package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/jbrinkman/kairon/internal/boundary"
	"github.com/jbrinkman/kairon/internal/failure"
)

// ghFunc runs `gh` with an argument list (never a shell) and optional stdin.
// Tests replace it; production uses execGH.
type ghFunc func(stdin string, args ...string) ([]byte, error)

// execGH runs the real gh CLI. A non-zero exit returns an error that wraps the
// *exec.ExitError (so isRateLimited still works) and includes stderr.
func execGH(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
				return out, fmt.Errorf("%w: %s", err, msg)
			}
		}
		return out, err
	}
	return out, nil
}

// rawComment is a comment as `gh issue view --json comments` returns it, or as
// fetchOpenIssues converts it from a GraphQL comment node.
type rawComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// rawIssue is an issue as fetchOpenIssues returns it from the GraphQL issues
// connection.
type rawIssue struct {
	Issue
	Comments []rawComment `json:"comments"`
}

// listPageSize is the number of issues one `gh api graphql` page requests. It
// is a page size, not a cap on the total: fetchOpenIssues follows the cursor
// until every open labeled issue has been read.
const listPageSize = 50

// listCommentCap is the number of comments the issues query requests per issue
// at most (oldest first). An issue whose list page is at the
// cap is re-read in full so a newer punch-out marker is not missed.
const listCommentCap = 100

// issueState converts decoded gh JSON into the failure contract's view of an
// issue. It is pure. A comment is ByViewer only when viewer is non-empty and
// equals the comment author's login. FailLabelRemovedAt is left zero.
func issueState(labels []Label, comments []rawComment, viewer string) failure.IssueState {
	st := failure.IssueState{}
	for _, l := range labels {
		st.Labels = append(st.Labels, l.Name)
	}
	for _, c := range comments {
		st.Comments = append(st.Comments, failure.Comment{
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
			ByViewer:  viewer != "" && c.Author.Login == viewer,
		})
	}
	return st
}

// hasMarkerComment reports whether any comment's first line is a punch-out
// marker, regardless of author. It decides whether the viewer lookup is needed.
func hasMarkerComment(comments []rawComment) bool {
	for _, c := range comments {
		if _, ok := failure.ParseMarker(c.Body); ok {
			return true
		}
	}
	return false
}

// unlabeledEvent is one line of the events call's output.
type unlabeledEvent struct {
	Label string    `json:"label"`
	At    time.Time `json:"at"`
}

// eventsJQ selects unlabeled events. It is a constant: the label being looked
// for is matched in Go by parseUnlabeledEvents, never placed in this program.
const eventsJQ = `.[] | select(.event=="unlabeled") | {label: .label.name, at: .created_at}`

// parseUnlabeledEvents returns the most recent time an event removed label, or
// the zero time if none did. out is a stream of JSON objects, compact or
// pretty-printed. Matching is an exact string comparison.
func parseUnlabeledEvents(out []byte, label string) (time.Time, error) {
	var latest time.Time
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var ev unlabeledEvent
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				return latest, nil
			}
			return time.Time{}, fmt.Errorf("failed to parse label events: %w", err)
		}
		if ev.Label == label && ev.At.After(latest) {
			latest = ev.At
		}
	}
}

func validateTarget(repo string, issue int) error {
	if strings.TrimSpace(repo) == "" {
		return errors.New("repo must not be empty")
	}
	if issue <= 0 {
		return fmt.Errorf("invalid issue number %d", issue)
	}
	return nil
}

func viewerLogin(run ghFunc) (string, error) {
	out, err := run("", "api", "user", "--jq", ".login")
	if err != nil {
		return "", fmt.Errorf("gh api user failed: %w", err)
	}
	user := strings.TrimSpace(string(out))
	if user == "" {
		return "", errors.New("gh api user returned an empty login")
	}
	return user, nil
}

func fetchFailLabelRemovedAt(run ghFunc, repo string, issue int, failLabel string) (time.Time, error) {
	out, err := run("", "api", fmt.Sprintf("repos/%s/issues/%d/events", repo, issue),
		"--paginate", "--jq", eventsJQ)
	if err != nil {
		return time.Time{}, fmt.Errorf("gh api issue events failed: %w", err)
	}
	return parseUnlabeledEvents(out, failLabel)
}

// CLI implements failure.Issues with the gh CLI. The zero value runs gh.
type CLI struct{ run ghFunc }

var _ failure.Issues = CLI{}

func (c CLI) gh() ghFunc {
	if c.run != nil {
		return c.run
	}
	return execGH
}

// State returns the labels and comments of an issue. The authenticated user is
// looked up only if a marker comment exists, and the label events only if a
// marker comment by that user exists and failLabel is not currently applied.
func (c CLI) State(repo string, issue int, failLabel string) (failure.IssueState, error) {
	if err := validateTarget(repo, issue); err != nil {
		return failure.IssueState{}, err
	}
	run := c.gh()
	out, err := run("", "issue", "view", fmt.Sprintf("%d", issue), "--repo", repo, "--json", "labels,comments")
	if err != nil {
		return failure.IssueState{}, fmt.Errorf("gh issue view failed: %w", err)
	}
	var view struct {
		Labels   []Label      `json:"labels"`
		Comments []rawComment `json:"comments"`
	}
	if err := json.Unmarshal(out, &view); err != nil {
		return failure.IssueState{}, fmt.Errorf("failed to parse issue: %w", err)
	}
	return resolveState(run, repo, issue, failLabel, view.Labels, view.Comments)
}

// resolveState turns decoded labels and comments into a failure.IssueState. It
// is the tail shared by State and ReadIssue. The authenticated user is looked
// up only if a marker comment exists, and the label events only if a marker
// comment by that user exists and failLabel is not currently applied.
func resolveState(run ghFunc, repo string, issue int, failLabel string, labels []Label, comments []rawComment) (failure.IssueState, error) {
	viewer := ""
	if hasMarkerComment(comments) {
		var err error
		if viewer, err = viewerLogin(run); err != nil {
			return failure.IssueState{}, err
		}
	}
	st := issueState(labels, comments, viewer)

	if hasViewerMarker(st) && !containsLabel(st.Labels, failLabel) {
		at, err := fetchFailLabelRemovedAt(run, repo, issue, failLabel)
		if err != nil {
			return failure.IssueState{}, err
		}
		st.FailLabelRemovedAt = at
	}
	return st, nil
}

var _ boundary.Reader = CLI{}

// ReadIssue reads title, body, open state, labels and comments of an issue in a
// single `gh issue view` call, then resolves the failure-contract state with the
// same logic State uses. The body is returned raw, exactly as GitHub returned it.
// Any error returns a zero Snapshot: a partial read is never trusted.
func (c CLI) ReadIssue(repo string, issue int, failLabel string) (boundary.Snapshot, error) {
	if err := validateTarget(repo, issue); err != nil {
		return boundary.Snapshot{}, err
	}
	run := c.gh()
	out, err := run("", "issue", "view", fmt.Sprintf("%d", issue), "--repo", repo,
		"--json", "title,body,state,labels,comments")
	if err != nil {
		return boundary.Snapshot{}, fmt.Errorf("gh issue view failed: %w", err)
	}
	var view struct {
		Title    string       `json:"title"`
		Body     string       `json:"body"`
		State    string       `json:"state"`
		Labels   []Label      `json:"labels"`
		Comments []rawComment `json:"comments"`
	}
	if err := json.Unmarshal(out, &view); err != nil {
		return boundary.Snapshot{}, fmt.Errorf("failed to parse issue: %w", err)
	}
	st, err := resolveState(run, repo, issue, failLabel, view.Labels, view.Comments)
	if err != nil {
		return boundary.Snapshot{}, err
	}
	return boundary.Snapshot{
		Title: view.Title,
		Body:  view.Body,
		Open:  view.State == "OPEN",
		State: st,
	}, nil
}

func hasViewerMarker(st failure.IssueState) bool {
	for _, cm := range st.Comments {
		if !cm.ByViewer {
			continue
		}
		if _, ok := failure.ParseMarker(cm.Body); ok {
			return true
		}
	}
	return false
}

func containsLabel(labels []string, name string) bool {
	for _, l := range labels {
		if l == name {
			return true
		}
	}
	return false
}

// PostComment posts body as a comment. The body travels on stdin so it can
// never be read as a flag or interpreted by a shell.
func (c CLI) PostComment(repo string, issue int, body string) error {
	if err := validateTarget(repo, issue); err != nil {
		return err
	}
	if _, err := c.gh()(body, "issue", "comment", fmt.Sprintf("%d", issue), "--repo", repo, "--body-file", "-"); err != nil {
		return fmt.Errorf("gh issue comment failed: %w", err)
	}
	return nil
}

// AddLabel adds label to the issue.
func (c CLI) AddLabel(repo string, issue int, label string) error {
	if err := validateTarget(repo, issue); err != nil {
		return err
	}
	if strings.TrimSpace(label) == "" {
		return errors.New("label must not be empty")
	}
	if _, err := c.gh()("", "issue", "edit", fmt.Sprintf("%d", issue), "--repo", repo, "--add-label", label); err != nil {
		return fmt.Errorf("gh issue edit failed: %w", err)
	}
	return nil
}

// RemoveLabel removes label from the issue.
func (c CLI) RemoveLabel(repo string, issue int, label string) error {
	if err := validateTarget(repo, issue); err != nil {
		return err
	}
	if strings.TrimSpace(label) == "" {
		return errors.New("label must not be empty")
	}
	if _, err := c.gh()("", "issue", "edit", fmt.Sprintf("%d", issue), "--repo", repo, "--remove-label", label); err != nil {
		return fmt.Errorf("gh issue edit failed: %w", err)
	}
	return nil
}

// Selection is the result of SelectIssues.
type Selection struct {
	// Eligible issues may be processed.
	Eligible []Issue
	// TerminalPending issues carry a trusted punch-out comment but not yet the
	// fail-state label. They are excluded from processing and need Reconcile.
	TerminalPending []Issue
}

// SelectIssues lists open issues with label and splits them into those that may
// be processed and those awaiting a completed punch-out. Issues labeled done or
// failed are in neither.
func SelectIssues(repo, label, assignee string) (Selection, error) {
	return selectIssues(execGH, repo, label, assignee)
}

func selectIssues(run ghFunc, repo, label, assignee string) (Selection, error) {
	// fetchOpenIssues pages through every open labeled issue and returns them
	// ascending by number. It maps a rate limit to ErrRateLimited and returns
	// nothing from pages that succeeded when a later page fails, so a failed
	// poll never yields a partial Selection.
	issues, err := fetchOpenIssues(run, repo, label, assignee)
	if err != nil {
		return Selection{}, err
	}

	done, failed := failure.DoneLabel(label), failure.FailedLabel(label)

	// The authenticated user is needed only if some issue has a marker comment.
	// Look it up once. If that fails, no marker can be trusted, so issues that
	// carry one are skipped for this poll (fail closed).
	var (
		viewer    string
		viewerErr error
		viewerSet bool
	)
	lookupViewer := func() (string, error) {
		if !viewerSet {
			viewer, viewerErr = viewerLogin(run)
			viewerSet = true
		}
		return viewer, viewerErr
	}

	var sel Selection
	for _, ri := range issues {
		if len(ri.Comments) >= listCommentCap {
			// The issues query returns only the oldest listCommentCap
			// comments, so a punch-out marker (always one of the newest) can be
			// hidden on a long thread. Re-read the issue in full and classify
			// from that. Fail closed: skip this poll on error rather than
			// re-spawning what may be a terminal-pending issue.
			st, err := CLI{run: run}.State(repo, ri.Number, failed)
			if err != nil {
				log.Printf("[github] issue #%d: full state lookup failed, skipping this poll: %v", ri.Number, err)
				continue
			}
			switch st.Classify(done, failed) {
			case failure.SelectEligible:
				sel.Eligible = append(sel.Eligible, ri.Issue)
			case failure.SelectTerminalPending:
				sel.TerminalPending = append(sel.TerminalPending, ri.Issue)
			}
			continue
		}
		who := ""
		if hasMarkerComment(ri.Comments) {
			if who, err = lookupViewer(); err != nil {
				log.Printf("[github] issue #%d: cannot verify punch-out marker, skipping this poll: %v", ri.Number, err)
				continue
			}
		}
		st := issueState(ri.Labels, ri.Comments, who)
		class := st.Classify(done, failed)
		if class == failure.SelectTerminalPending {
			at, err := fetchFailLabelRemovedAt(run, repo, ri.Number, failed)
			if err != nil {
				log.Printf("[github] issue #%d: label events lookup failed, skipping this poll: %v", ri.Number, err)
				continue
			}
			st.FailLabelRemovedAt = at
			class = st.Classify(done, failed)
		}
		switch class {
		case failure.SelectEligible:
			sel.Eligible = append(sel.Eligible, ri.Issue)
		case failure.SelectTerminalPending:
			sel.TerminalPending = append(sel.TerminalPending, ri.Issue)
		}
	}
	return sel, nil
}

func listIssues(run ghFunc, repo, label, assignee string) ([]Issue, error) {
	sel, err := selectIssues(run, repo, label, assignee)
	if err != nil {
		return nil, err
	}
	return sel.Eligible, nil
}
