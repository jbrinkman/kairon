package boundary

import (
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// Reader is the GitHub read port. github.CLI implements it.
type Reader interface {
	ReadIssue(repo string, issue int, failLabel string) (Snapshot, error)
}

// Snapshot is one consistent read of an issue.
type Snapshot struct {
	Title string
	Body  string             // raw, exactly as GitHub returned it (may contain CRLF)
	Open  bool               // the issue is open
	State failure.IssueState // labels, comments, FailLabelRemovedAt
}

// Intake starts planner intake for an issue whose body is not verified. Start
// does not wait for the planner: it reports whether a run was started, one is
// already running, intake has already committed this issue since it was last
// admitted (IntakeSpent), or intake may not start again (IntakeExhausted). A
// non-nil error means intake could not be started; nothing was written.
type Intake interface {
	Start(repo string, issue int) (IntakeStatus, error)
}

// IntakeStatus is what Intake.Start reports. The zero value is not a status.
type IntakeStatus int

// Intake statuses.
const (
	// IntakeStarted: a new intake run is going. A later poll sees its result.
	IntakeStarted IntakeStatus = iota + 1
	// IntakeRunning: a run for this issue was already going; none was started.
	IntakeRunning
	// IntakeSpent: intake already committed this issue and the body is still
	// not valid, so another run would loop. The boundary punches out.
	IntakeSpent
	// IntakeExhausted: a human stopped intake for this issue, or its runs
	// failed on every start allowed, so another run must wait for a human. The
	// boundary punches out.
	IntakeExhausted
)

// Guard names carried in the FailureContext.
const (
	guardProvenance = "provenance"
	guardScreen     = "screen"

	expectedMarker = "issue body ends with one valid `planner-provenance: v1` marker whose sha256 equals the hash of the canonical body"
	expectedScreen = "issue title and body pass the content screen"

	maxDetail = 300 // longest error text copied into a FailureContext
)

// Skip reasons reported in Result.SkipReason.
const (
	SkipDone            = "done"
	SkipFailed          = "failed"
	SkipClosed          = "closed"
	SkipTerminalPending = "terminal-pending"
	SkipIntakeStarted   = "intake-started"
	SkipIntakeRunning   = "intake-running"
)

// Boundary loads and verifies an issue before implementation. It never writes
// to GitHub; a punch-out is returned for the caller to enact.
type Boundary struct {
	Reader Reader
	// Intake is nil when no automated intake is wired; non-valid issues then
	// punch out. Otherwise a non-valid issue starts intake and is skipped.
	Intake Intake
	// Label is the trigger label; the done and failed labels derive from it.
	Label string
	// Classify is a seam for tests. Nil means provenance.Classify.
	Classify func(body string) provenance.State
	// Screen checks untrusted content. Nil means DefaultScreen.
	Screen func(body string) error
}

// Outcome is what Admit decided. The zero value is not an outcome.
type Outcome int

// Outcomes.
const (
	// OutcomeAdmitted: the body is verified; Result.Document is set.
	OutcomeAdmitted Outcome = iota + 1
	// OutcomeSkipped: do not spawn and write nothing; see Result.SkipReason.
	OutcomeSkipped
	// OutcomePunchOut: do not spawn; enact Result.Failure and Result.Remedy.
	OutcomePunchOut
)

// Result is what Admit reports.
type Result struct {
	Outcome    Outcome
	Document   *InputDocument        // OutcomeAdmitted only
	SkipReason string                // OutcomeSkipped only
	Failure    *guard.FailureContext // OutcomePunchOut only
	Remedy     string                // OutcomePunchOut only
}

// Admit reads the issue from GitHub and decides whether it may be implemented.
// It takes no body: the body always comes from a fresh read.
//
// It returns a non-nil error, and a zero Result, only when it cannot decide:
// invalid arguments (matching guard.ErrInvalid), a failed read, or intake that
// could not be started. It fails closed: an error means do not spawn and write
// nothing.
func (b Boundary) Admit(repo string, issue int) (Result, error) {
	switch {
	case repo == "":
		return Result{}, fmt.Errorf("%w: boundary: repo is empty", guard.ErrInvalid)
	case issue <= 0:
		return Result{}, fmt.Errorf("%w: boundary: issue number %d is not positive", guard.ErrInvalid, issue)
	case b.Reader == nil:
		return Result{}, fmt.Errorf("%w: boundary: no reader", guard.ErrInvalid)
	case b.Label == "":
		return Result{}, fmt.Errorf("%w: boundary: no trigger label", guard.ErrInvalid)
	}
	classify := b.Classify
	if classify == nil {
		classify = provenance.Classify
	}
	screen := b.Screen
	if screen == nil {
		screen = DefaultScreen
	}
	doneLabel, failedLabel := failure.DoneLabel(b.Label), failure.FailedLabel(b.Label)

	snap, err := b.Reader.ReadIssue(repo, issue, failedLabel)
	if err != nil {
		return Result{}, fmt.Errorf("boundary: read issue #%d in %s: %w", issue, repo, err)
	}
	if reason := skipReason(snap, doneLabel, failedLabel); reason != "" {
		return Result{Outcome: OutcomeSkipped, SkipReason: reason}, nil
	}
	if state := classify(snap.Body); state != provenance.StateValid {
		return b.startIntake(repo, issue, state, failedLabel)
	}

	canonical := provenance.Canonicalize(snap.Body)
	if err := screen(canonical); err != nil {
		return screenPunchOut(issue, "body", err, failedLabel), nil
	}
	if err := screen(snap.Title); err != nil {
		return screenPunchOut(issue, "title", err, failedLabel), nil
	}

	return Result{
		Outcome: OutcomeAdmitted,
		Document: &InputDocument{
			SchemaVersion: SchemaVersion,
			Repo:          repo,
			Issue:         issue,
			Title:         snap.Title,
			Provenance:    Provenance{State: StateValid, SHA256: provenance.Hash(snap.Body)},
			Body:          canonical,
		},
	}, nil
}

// startIntake handles a body that is not valid. With no intake wired the issue
// punches out. Otherwise intake is started (or found running) and the issue is
// skipped: a later poll re-reads the stamped body and runs every pre-check on
// it again. IntakeSpent means a commit already happened and the body is still
// not valid, so it punches out instead of looping.
func (b Boundary) startIntake(repo string, issue int, state provenance.State, failedLabel string) (Result, error) {
	if b.Intake == nil {
		return provenancePunchOut(issue, state, failedLabel,
			fmt.Sprintf("%s; no automated intake is configured", describe(state))), nil
	}
	status, err := b.Intake.Start(repo, issue)
	if err != nil {
		return Result{}, fmt.Errorf("boundary: start intake for issue #%d: %w", issue, err)
	}
	switch status {
	case IntakeStarted:
		return Result{Outcome: OutcomeSkipped, SkipReason: SkipIntakeStarted}, nil
	case IntakeRunning:
		return Result{Outcome: OutcomeSkipped, SkipReason: SkipIntakeRunning}, nil
	case IntakeSpent:
		return provenancePunchOut(issue, state, failedLabel,
			fmt.Sprintf("%s; the body changed after intake committed it", describe(state))), nil
	case IntakeExhausted:
		return provenancePunchOut(issue, state, failedLabel,
			fmt.Sprintf("%s; intake was stopped, or failed on every start allowed", describe(state))), nil
	}
	return Result{}, fmt.Errorf("%w: boundary: unknown intake status %d for issue #%d", guard.ErrInvalid, status, issue)
}

// skipReason reports why the issue must not be spawned, or "" if it is eligible.
func skipReason(snap Snapshot, doneLabel, failedLabel string) string {
	if !snap.Open {
		return SkipClosed
	}
	switch snap.State.Classify(doneLabel, failedLabel) {
	case failure.SelectDone:
		return SkipDone
	case failure.SelectFailed:
		return SkipFailed
	case failure.SelectTerminalPending:
		return SkipTerminalPending
	case failure.SelectEligible:
		return ""
	default:
		return SkipTerminalPending // unknown selection: fail closed
	}
}

// describe names a non-valid provenance state for a FailureContext.
func describe(s provenance.State) string {
	switch s {
	case provenance.StateNone:
		return "none: no provenance marker line"
	case provenance.StateMismatch:
		return "mismatch: marker present but the body does not match it"
	default:
		return fmt.Sprintf("unrecognized provenance state %q", s.String())
	}
}

// provenancePunchOut builds the punch-out for a body that is not valid. The
// remedy names the TUI command `validate <issue>` with the real number: it
// canonicalizes and stamps the issue and removes the failed label on commit.
func provenancePunchOut(issue int, state provenance.State, failedLabel, actual string) Result {
	remedy := fmt.Sprintf("The body changed after it was planned. Run `validate %d` in the Kairon TUI to plan and "+
		"stamp it again (or restore the planned text and remove the `%s` label). Validate removes the `%s` label "+
		"when it commits.", issue, failedLabel, failedLabel)
	if state == provenance.StateNone {
		remedy = fmt.Sprintf("Run `validate %d` in the Kairon TUI so the issue is canonicalized and stamped. "+
			"Validate removes the `%s` label when it commits.", issue, failedLabel)
	}
	return punchOut(guardProvenance, expectedMarker, actual, remedy)
}

func screenPunchOut(issue int, what string, err error, failedLabel string) Result {
	remedy := fmt.Sprintf("The issue title did not pass the content screen. Fix the title, then remove the `%s` "+
		"label. The title is not hashed, so it does not need planning again.", failedLabel)
	if what == "body" {
		remedy = fmt.Sprintf("The issue body did not pass the content screen. Fix the body, then run `validate %d` "+
			"in the Kairon TUI so it is re-canonicalized and stamped (a hand edit to the hashed body fails "+
			"provenance otherwise). Validate removes the `%s` label when it commits.", issue, failedLabel)
	}
	return punchOut(guardScreen, expectedScreen,
		fmt.Sprintf("the screen rejected the issue %s: %s", what, truncate(err.Error())), remedy)
}

// punchOut builds a terminal result: attempt 1 of 1, because no retry can fix a
// human edit or a missing marker.
func punchOut(guardName, expected, actual, remedy string) Result {
	return Result{
		Outcome: OutcomePunchOut,
		Failure: &guard.FailureContext{
			Stage:    stageoutput.StageKrewLead,
			Guard:    guardName,
			Attempt:  guard.Attempt{Current: 1, Max: 1},
			Expected: expected,
			Actual:   actual,
		},
		Remedy: remedy,
	}
}

// truncate flattens s to one line and caps its length on a rune boundary.
func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > maxDetail {
		return string(r[:maxDetail]) + "..."
	}
	return s
}
