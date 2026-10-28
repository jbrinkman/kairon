package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/github"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/planner"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// The planning handoff picks up the document an interactive planning session
// leaves behind. Kairon chooses a file path per planning session and tells the
// agent; the agent writes the approved stage-output document there. When the
// session ends, finishPlanning reads the file and sends it to the planning
// boundary on the create path, which is the only writer of GitHub issues.
//
// Nothing in this file names a front end. A front end passes a planningEnd and
// supplies its own Resume function, so removing one front end removes only its
// launch site.

// planningHandoffTimeout bounds one send to the planning boundary.
const planningHandoffTimeout = 2 * time.Minute

// planningSessionsDir holds the per-session document files.
const planningSessionsDir = ".kairon/sessions"

// planningBoundary is the part of planner.Boundary the handoff uses.
type planningBoundary interface {
	Process(ctx context.Context, req planner.Request) (planner.Result, error)
}

var _ planningBoundary = (*planner.Boundary)(nil)

// planningHandoffDoneMsg reports the end of one send to the planning boundary.
type planningHandoffDoneMsg struct {
	end  planningEnd
	path string
	// sent is the document that was sent. The file may hold a newer one by
	// the time the send ends.
	sent   []byte
	result planner.Result
	err    error

	// label is the trigger-label answer read together with the document, and
	// labelPath is the file it came from.
	label     labelAnswer
	labelPath string
	// labelName is the label the command tried to apply when labelTried is
	// set, and labelErr is how that failed.
	labelName  string
	labelTried bool
	labelErr   error
}

// planningLabeler is the part of the GitHub port the handoff uses to apply
// the trigger label. github.CLI satisfies it.
type planningLabeler interface {
	AddLabel(repo string, issue int, label string) error
}

var _ planningLabeler = github.CLI{}

// newPlanningLabeler builds the port the handoff uses to apply the trigger
// label. It returns nil when no repository is configured, like the boundary.
func newPlanningLabeler(cfg *config.Config) planningLabeler {
	if cfg == nil || cfg.Repo == "" {
		return nil
	}
	return github.CLI{}
}

// labelAnswerMaxBytes bounds the label file. The answer is one word.
const labelAnswerMaxBytes = 16

// labelAnswer is what the label file said when the document was read.
type labelAnswer struct {
	apply   bool   // the person answered yes
	problem string // non-empty: the file could not be used; apply is false
}

// readLabelAnswer reads the label file the planner wrote. A missing file is
// no answer, which means no label. The file must hold the single word yes or
// no, in any case; anything else is reported as a problem and applies nothing.
func readLabelAnswer(path string) labelAnswer {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return labelAnswer{}
		}
		return labelAnswer{problem: fmt.Sprintf("the label answer file %s cannot be checked (%v)", path, err)}
	}
	if !info.Mode().IsRegular() {
		return labelAnswer{problem: fmt.Sprintf("the label answer file %s is not a regular file", path)}
	}
	f, err := os.Open(path)
	if err != nil {
		return labelAnswer{problem: fmt.Sprintf("the label answer file %s cannot be read (%v)", path, err)}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, labelAnswerMaxBytes+1))
	if err != nil {
		return labelAnswer{problem: fmt.Sprintf("the label answer file %s cannot be read (%v)", path, err)}
	}
	if len(data) > labelAnswerMaxBytes {
		return labelAnswer{problem: fmt.Sprintf("the label answer file %s is longer than %d bytes", path, labelAnswerMaxBytes)}
	}
	switch strings.ToLower(strings.TrimSpace(string(data))) {
	case "yes":
		return labelAnswer{apply: true}
	case "no":
		return labelAnswer{}
	}
	return labelAnswer{problem: fmt.Sprintf("the label answer file %s says %q, not yes or no", path, string(data))}
}

// applyTriggerLabel adds the trigger label to the committed issue. It runs
// inside the handoff command, so gh never runs on the UI goroutine.
func applyTriggerLabel(labeler planningLabeler, repo, label string, issue int) error {
	if labeler == nil {
		return errors.New("no GitHub connection is configured")
	}
	if strings.TrimSpace(label) == "" {
		return errors.New("no trigger label is configured")
	}
	return labeler.AddLabel(repo, issue, label)
}

// newPlanningBoundary builds the boundary the handoff uses, with the same
// collaborators the intake runs use: the gh-backed issue store as the
// committer's store, the gh CLI for punch-outs, and a conflict router. It
// returns nil when no repository is configured.
func newPlanningBoundary(cfg *config.Config) planningBoundary {
	if cfg == nil || cfg.Repo == "" {
		return nil
	}
	target := failure.Target{Repo: cfg.Repo, FailLabel: failure.FailedLabel(cfg.Label)}
	issues := github.CLI{}
	return &planner.Boundary{
		Committer: &provenance.Committer{
			Store:  github.NewIssueStore(cfg.Repo),
			Router: &planner.ConflictRouter{Issues: issues, Target: target},
		},
		Issues: issues,
		Target: target,
	}
}

// planningSessionFile returns the path of a per-session file under the
// sessions directory, relative to the working directory. The session ID comes
// from a file name, so an ID that is empty or could leave the sessions
// directory is refused.
func planningSessionFile(sessionID, suffix string) (string, error) {
	if sessionID == "" {
		return "", errors.New("planning session has no ID")
	}
	if strings.ContainsAny(sessionID, `/\`) || strings.Contains(sessionID, "..") {
		return "", fmt.Errorf("planning session ID %q is not a plain file name", sessionID)
	}
	return path.Join(planningSessionsDir, sessionID+suffix), nil
}

// planningDocumentPath returns the document file for a planning session.
func planningDocumentPath(sessionID string) (string, error) {
	return planningSessionFile(sessionID, ".document.yaml")
}

// planningLabelPath returns the file where the planner records the person's
// answer to the trigger-label question, next to the document file.
func planningLabelPath(sessionID string) (string, error) {
	return planningSessionFile(sessionID, ".label")
}

// planningPrompt is the initial prompt for a planning session: the document
// path and the label answer path on the first two lines, one more sentence for
// a resumed session, then the person's own request. A session ID that has no
// valid path adds nothing, and the request is returned unchanged.
func planningPrompt(sessionID, request string, resumed bool) string {
	docPath, err := planningDocumentPath(sessionID)
	if err != nil {
		return request
	}
	labelPath, err := planningLabelPath(sessionID)
	if err != nil {
		return request
	}
	var sb strings.Builder
	sb.WriteString("Document path: " + docPath)
	sb.WriteString("\nLabel answer path: " + labelPath)
	if resumed {
		sb.WriteString("\nThis session is being resumed. The file may already hold the last document; read it before continuing.")
	}
	if request != "" {
		sb.WriteString("\n\n" + request)
	}
	return sb.String()
}

// planningResumePrompt is the message that resumes a session after the
// planning boundary rejected its document. It always carries the path.
func planningResumePrompt(sessionID string, fc guard.FailureContext) string {
	request := fmt.Sprintf("The document you wrote to that path was rejected by the %s check, and nothing was written to GitHub.\n"+
		"Expected: %s\nActual: %s\n"+
		"Revise the document with me if it needs my input, rewrite the file at that path, and return the document as your reply.",
		fc.Guard, fc.Expected, fc.Actual)
	return planningPrompt(sessionID, request, true)
}

// finishPlanning is the handoff. A front end calls it when a planning session
// ends. If the session's document file does not exist it does nothing. If a
// send for the session is already in flight it does nothing. Otherwise it
// reads the file and returns a command that sends the contents to the planning
// boundary off the UI goroutine; the model handles the planningHandoffDoneMsg
// that command returns.
func (m model) finishPlanning(end planningEnd) (model, tea.Cmd) {
	docPath, err := planningDocumentPath(end.SessionID)
	if err != nil {
		return m, nil
	}
	info, err := os.Stat(docPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m, nil
		}
		return m.handoffError(fmt.Sprintf("Cannot check the planning document %s: %v. The file was kept.", docPath, err)), nil
	}
	if m.planningHandoffs[end.SessionID] {
		return m, nil
	}
	if m.planBoundary == nil {
		return m.handoffError(fmt.Sprintf("Cannot send the planning document %s: no repository is configured. The file was kept.", docPath)), nil
	}
	if !info.Mode().IsRegular() {
		return m.handoffError(fmt.Sprintf("Cannot read the planning document %s: it is not a regular file. It was kept.", docPath)), nil
	}
	contents, err := os.ReadFile(docPath)
	if err != nil {
		return m.handoffError(fmt.Sprintf("Cannot read the planning document %s: %v. The file was kept.", docPath, err)), nil
	}
	// The answer is read with the document it belongs to, so a rewrite while
	// the send is in flight cannot change what this send does.
	labelPath, err := planningLabelPath(end.SessionID)
	if err != nil {
		return m, nil
	}
	answer := readLabelAnswer(labelPath)

	if m.planningHandoffs == nil {
		m.planningHandoffs = make(map[string]bool)
	}
	m.planningHandoffs[end.SessionID] = true

	boundary := m.planBoundary
	labeler := m.planLabeler
	repo, labelName := "", ""
	if m.config != nil {
		repo, labelName = m.config.Repo, m.config.Label
	}
	req := planner.Request{
		Reply:   string(contents),
		Attempt: guard.Attempt{Current: 1, Max: 1},
		// Number 0 is the create path: a new issue, never an update.
	}
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), planningHandoffTimeout)
		defer cancel()
		res, err := boundary.Process(ctx, req)
		done := planningHandoffDoneMsg{
			end: end, path: docPath, sent: contents, result: res, err: err,
			label: answer, labelPath: labelPath, labelName: labelName,
		}
		if err == nil && res.Outcome == planner.OutcomeCommitted && answer.apply {
			done.labelTried = true
			done.labelErr = applyTriggerLabel(labeler, repo, labelName, res.Number)
		}
		return done
	}
}

// handlePlanningHandoffDone applies the outcome of one send. The document file
// is deleted only after a commit, and only if it still holds the document that
// was sent; every other outcome keeps it. The agent can write a newer document
// while a send is in flight, and the in-flight guard drops that turn's end, so
// a newer document is sent now instead of being deleted or ignored.
func (m model) handlePlanningHandoffDone(msg planningHandoffDoneMsg) (model, tea.Cmd) {
	delete(m.planningHandoffs, msg.end.SessionID)

	current, readErr := os.ReadFile(msg.path)
	newer := readErr == nil && !bytes.Equal(current, msg.sent)
	sendNewer := func(m model) (model, tea.Cmd) {
		m = m.appendActivity(fmt.Sprintf(
			"A newer planning document was written to %s while the last one was being sent. Sending it now.", msg.path))
		return m.finishPlanning(msg.end)
	}

	if msg.err != nil {
		m = m.handoffError(fmt.Sprintf("Planning document %s was not sent: %v. The file was kept.", msg.path, msg.err))
		if newer {
			return sendNewer(m)
		}
		return m, nil
	}

	switch msg.result.Outcome {
	case planner.OutcomeCommitted:
		n := msg.result.Number
		repo := ""
		if m.config != nil {
			repo = m.config.Repo
		}
		m = m.appendActivity(m.styles.Success.Render(fmt.Sprintf(
			"Created issue #%d from the planning session: https://github.com/%s/issues/%d", n, repo, n)))
		m = m.reportTriggerLabel(msg, n)
		if newer {
			// The label file may belong to the newer document, so keep it.
			return sendNewer(m)
		}
		if err := os.Remove(msg.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			m = m.handoffError(fmt.Sprintf(
				"Issue #%d exists, but the planning document %s could not be removed (%v). To avoid a duplicate issue, delete the file by hand.",
				n, msg.path, err))
		}
		if msg.labelPath != "" {
			// A stale yes would label the next issue of this session.
			if err := os.Remove(msg.labelPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				m = m.handoffError(fmt.Sprintf(
					"Issue #%d exists, but the label answer file %s could not be removed (%v). Delete it by hand, or the next issue from this session may get the label.",
					n, msg.labelPath, err))
			}
		}
		return m, nil

	case planner.OutcomeRejected:
		fc := planner.RejectionFailure(msg.result, guard.Attempt{Current: 1, Max: 1})
		// The console line stays even if a later rejection replaces the dialog.
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf(
			"Planning document rejected by the %s check. Nothing was written to GitHub. The file was kept at %s", fc.Guard, msg.path)))
		if newer {
			// The rejection is for a document the agent has already replaced.
			return sendNewer(m)
		}
		return m.openPlanningRejection(msg.end, fc, msg.path), nil
	}

	m = m.handoffError(fmt.Sprintf("Planning document %s got an unexpected outcome (%s). The file was kept.",
		msg.path, msg.result.Outcome))
	if newer {
		return sendNewer(m)
	}
	return m, nil
}

// reportTriggerLabel adds the console line for the label step of a committed
// issue. No answer and a no answer add nothing.
func (m model) reportTriggerLabel(msg planningHandoffDoneMsg, issue int) model {
	switch {
	case msg.label.problem != "":
		return m.handoffError(fmt.Sprintf("Issue #%d was created without the trigger label: %s. Add it by hand if you want the watcher to pick it up.",
			issue, msg.label.problem))
	case msg.labelErr != nil:
		return m.handoffError(fmt.Sprintf("Issue #%d was created, but adding the label `%s` failed: %v. Add the label by hand if you want the watcher to pick it up.",
			issue, msg.labelName, msg.labelErr))
	case msg.labelTried:
		return m.appendActivity(m.styles.Success.Render(fmt.Sprintf(
			"Added the label `%s` to issue #%d.", msg.labelName, issue)))
	}
	return m
}

func (m model) handoffError(line string) model {
	return m.appendActivity(m.styles.Error.Render(line))
}
