package tui

import (
	"context"
	"errors"
	"fmt"
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
	end    planningEnd
	path   string
	result planner.Result
	err    error
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

// planningDocumentPath returns the document file for a planning session,
// relative to the working directory. The session ID comes from a file name, so
// an ID that is empty or could leave the sessions directory is refused.
func planningDocumentPath(sessionID string) (string, error) {
	if sessionID == "" {
		return "", errors.New("planning session has no ID")
	}
	if strings.ContainsAny(sessionID, `/\`) || strings.Contains(sessionID, "..") {
		return "", fmt.Errorf("planning session ID %q is not a plain file name", sessionID)
	}
	return path.Join(planningSessionsDir, sessionID+".document.yaml"), nil
}

// planningPrompt is the initial prompt for a planning session: the document
// path on the first line, one more sentence for a resumed session, then the
// person's own request. A session ID that has no valid path adds nothing, and
// the request is returned unchanged.
func planningPrompt(sessionID, request string, resumed bool) string {
	docPath, err := planningDocumentPath(sessionID)
	if err != nil {
		return request
	}
	var sb strings.Builder
	sb.WriteString("Document path: " + docPath)
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

	if m.planningHandoffs == nil {
		m.planningHandoffs = make(map[string]bool)
	}
	m.planningHandoffs[end.SessionID] = true

	boundary := m.planBoundary
	req := planner.Request{
		Reply:   string(contents),
		Attempt: guard.Attempt{Current: 1, Max: 1},
		// Number 0 is the create path: a new issue, never an update.
	}
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), planningHandoffTimeout)
		defer cancel()
		res, err := boundary.Process(ctx, req)
		return planningHandoffDoneMsg{end: end, path: docPath, result: res, err: err}
	}
}

// handlePlanningHandoffDone applies the outcome of one send. The document file
// is deleted only after a commit; every other outcome keeps it.
func (m model) handlePlanningHandoffDone(msg planningHandoffDoneMsg) model {
	delete(m.planningHandoffs, msg.end.SessionID)

	if msg.err != nil {
		return m.handoffError(fmt.Sprintf("Planning document %s was not sent: %v. The file was kept.", msg.path, msg.err))
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
		if err := os.Remove(msg.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			m = m.handoffError(fmt.Sprintf(
				"Issue #%d exists, but the planning document %s could not be removed (%v). To avoid a duplicate issue, delete the file by hand.",
				n, msg.path, err))
		}
		return m

	case planner.OutcomeRejected:
		fc := planner.RejectionFailure(msg.result, guard.Attempt{Current: 1, Max: 1})
		// The console line stays even if a later rejection replaces the dialog.
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf(
			"Planning document rejected by the %s check. Nothing was written to GitHub. The file was kept at %s", fc.Guard, msg.path)))
		return m.openPlanningRejection(msg.end, fc, msg.path)
	}

	return m.handoffError(fmt.Sprintf("Planning document %s got an unexpected outcome (%s). The file was kept.",
		msg.path, msg.result.Outcome))
}

func (m model) handoffError(line string) model {
	return m.appendActivity(m.styles.Error.Render(line))
}
