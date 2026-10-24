package boundary

import (
	"errors"
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

// Intake routes an unverified issue back through the planner. It is
// synchronous: it returns nil only after the issue body has been re-committed
// with a provenance marker. It returns ErrIntakePunchedOut if intake itself
// punched the issue out. Any other error means intake failed.
type Intake interface {
	Route(repo string, issue int) error
}

// ErrIntakePunchedOut means intake punched the issue out (Tier 2 already
// enacted), so the boundary must not spawn and must not punch out again.
var ErrIntakePunchedOut = errors.New("boundary: intake punched the issue out")

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
	SkipDone             = "done"
	SkipFailed           = "failed"
	SkipClosed           = "closed"
	SkipTerminalPending  = "terminal-pending"
	SkipIntakePunchedOut = "intake-punched-out"
)

// Boundary loads and verifies an issue before implementation. It never writes
// to GitHub; a punch-out is returned for the caller to enact.
type Boundary struct {
	Reader Reader
	// Intake is nil when no automated intake is wired; non-valid issues then
	// punch out.
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
	ViaIntake  bool                  // OutcomeAdmitted only: verified after an intake pass
	SkipReason string                // OutcomeSkipped only
	Failure    *guard.FailureContext // OutcomePunchOut only
	Remedy     string                // OutcomePunchOut only
}

// Admit reads the issue from GitHub and decides whether it may be implemented.
// It takes no body: the body always comes from a fresh read.
//
// It returns a non-nil error, and a zero Result, only when it cannot decide:
// invalid arguments (matching guard.ErrInvalid) or a failed read. It fails
// closed: an error means do not spawn and write nothing.
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

	var snap Snapshot
	viaIntake := false
	for {
		var err error
		snap, err = b.Reader.ReadIssue(repo, issue, failedLabel)
		if err != nil {
			return Result{}, fmt.Errorf("boundary: read issue #%d in %s: %w", issue, repo, err)
		}
		if reason := skipReason(snap, doneLabel, failedLabel); reason != "" {
			return Result{Outcome: OutcomeSkipped, SkipReason: reason}, nil
		}

		state := classify(snap.Body)
		if state == provenance.StateValid {
			break
		}
		if viaIntake {
			return provenancePunchOut(state, failedLabel,
				fmt.Sprintf("intake reported success but the body is still %s", describe(state))), nil
		}
		if b.Intake == nil {
			return provenancePunchOut(state, failedLabel,
				fmt.Sprintf("%s; no automated intake is configured", describe(state))), nil
		}
		if err := b.Intake.Route(repo, issue); err != nil {
			if errors.Is(err, ErrIntakePunchedOut) {
				return Result{Outcome: OutcomeSkipped, SkipReason: SkipIntakePunchedOut}, nil
			}
			return provenancePunchOut(state, failedLabel,
				fmt.Sprintf("%s; intake failed: %s", describe(state), truncate(err.Error()))), nil
		}
		viaIntake = true // re-read and re-verify exactly once
	}

	canonical := provenance.Canonicalize(snap.Body)
	if err := screen(canonical); err != nil {
		return screenPunchOut("body", err, failedLabel), nil
	}
	if err := screen(snap.Title); err != nil {
		return screenPunchOut("title", err, failedLabel), nil
	}

	return Result{
		Outcome:   OutcomeAdmitted,
		ViaIntake: viaIntake,
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

func provenancePunchOut(state provenance.State, failedLabel, actual string) Result {
	remedy := fmt.Sprintf("The body changed after it was planned. Re-run the planner on the issue "+
		"(or restore the planned text), then remove the `%s` label.", failedLabel)
	if state == provenance.StateNone {
		remedy = fmt.Sprintf("Run the planner on this issue so it is canonicalized and stamped, "+
			"then remove the `%s` label.", failedLabel)
	}
	return punchOut(guardProvenance, expectedMarker, actual, remedy)
}

func screenPunchOut(what string, err error, failedLabel string) Result {
	return punchOut(guardScreen, expectedScreen,
		fmt.Sprintf("the screen rejected the issue %s: %s", what, truncate(err.Error())),
		fmt.Sprintf("The issue %s did not pass the content screen. Fix the %s, then re-run the "+
			"planner on the issue so it is re-canonicalized and stamped (a hand edit to the hashed "+
			"body would fail provenance on the next run), then remove the `%s` label.",
			what, what, failedLabel))
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
