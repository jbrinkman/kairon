package provenance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// Sentinel errors returned (wrapped) by Commit. Match them with errors.Is.
var (
	// ErrNotReady means the guard result was not an all-pass
	// guard.OutcomeReadyToCommit. Commit made no store call.
	ErrNotReady = errors.New("provenance: admission result is not ready to commit")
	// ErrInvalid means the request was malformed. Commit made no store call.
	ErrInvalid = errors.New("provenance: invalid commit request")
	// ErrRevisionUnavailable means the store could not report the issue's
	// current revision, so the commit could not prove the issue was
	// unchanged. Nothing was written.
	ErrRevisionUnavailable = errors.New("provenance: issue revision unavailable")
	// ErrUnroutable means the issue changed under the planner and neither
	// RouteToIntake nor PunchOut succeeded. Nothing was written.
	ErrUnroutable = errors.New("provenance: conflicting issue could not be routed")
)

// Commit step. This is not a guard: the boundary calls Commit after
// guard.Run reports guard.OutcomeReadyToCommit.

// IssueStore is the GitHub boundary. Create and Update are the only writes.
// A concrete implementation (for example one built on gh) lives outside this
// package, which keeps it free of process, file and network access.
type IssueStore interface {
	// Revision returns the issue's current revision token (GitHub updatedAt
	// or an equivalent opaque token).
	Revision(ctx context.Context, number int) (string, error)
	// Create makes a new issue and returns its number. This is a GitHub write.
	Create(ctx context.Context, title, body string) (int, error)
	// Update replaces an existing issue's body. This is a GitHub write.
	Update(ctx context.Context, number int, body string) error
}

// Conflict describes an issue whose revision changed between the planner's
// read (Observed) and the commit (Current).
type Conflict struct {
	Number            int
	Observed, Current string
}

// Router receives an issue whose revision changed between planner read and
// commit.
type Router interface {
	// RouteToIntake sends the issue back for re-planning, so the human's
	// edit is not lost. It is the preferred route.
	RouteToIntake(ctx context.Context, c Conflict) error
	// PunchOut is used only if RouteToIntake fails.
	PunchOut(ctx context.Context, c Conflict) error
}

// Target says where the body goes. Number 0 means create a new issue.
// Otherwise the issue Number is updated, and ObservedRevision, recorded when
// the planner read the issue, is required.
type Target struct {
	Number           int
	ObservedRevision string
}

// Request is the input to Commit.
type Request struct {
	// Doc must validate, have Result == canonical and a non-empty body.
	Doc *stageoutput.Document
	// Title is required when Target.Number is 0.
	Title  string
	Target Target
	// Admission is the result from guard.Run.
	Admission guard.Result
}

// Outcome is what Commit did.
type Outcome int

// Outcomes Commit may report. OutcomeInvalid is the zero value and is
// returned alongside every error.
const (
	OutcomeInvalid        Outcome = iota
	OutcomeCommitted              // the body and marker were written
	OutcomeRoutedToIntake         // the issue changed; nothing written; sent to intake
	OutcomePunchedOut             // the issue changed; nothing written; punched out
)

// String returns a short name for o.
func (o Outcome) String() string {
	switch o {
	case OutcomeCommitted:
		return "committed"
	case OutcomeRoutedToIntake:
		return "routed_to_intake"
	case OutcomePunchedOut:
		return "punched_out"
	}
	return "invalid"
}

// Result is what Commit reports.
//
// For OutcomeCommitted, Number is the written issue, Hash is the sha256 in
// the marker and Body is the exact text written. For the two routed outcomes
// Number is the conflicting issue and Hash and Body are empty, because
// nothing was written.
type Result struct {
	Outcome Outcome
	Number  int
	Hash    string
	Body    string
}

// Committer performs the commit step.
type Committer struct {
	Store  IssueStore
	Router Router
	// Now supplies the marker's generated time. Nil means time.Now.
	Now func() time.Time
}

// Commit writes the validated body plus a v1 provenance marker to GitHub, once.
//
// Order of operations:
//
//  1. Gate: unless req.Admission is an all-pass OutcomeReadyToCommit with no
//     Failure, return ErrNotReady. No store method is called.
//  2. Validate the request; violations match ErrInvalid. No store call.
//  3. Compute body := Mark(doc.Body, now). This is the only place the marker
//     is produced.
//  4. Create path: one Store.Create.
//  5. Update path: read Store.Revision. If that fails, return
//     ErrRevisionUnavailable and write nothing. If the revision differs from
//     Target.ObservedRevision, write nothing and route the conflict
//     (RouteToIntake, then PunchOut if that fails, else ErrUnroutable).
//     Otherwise one Store.Update.
//
// Commit never retries; bounded retry belongs to the caller. It spawns
// nothing: advancing to the next stage is the caller's reaction to
// OutcomeCommitted.
func (c *Committer) Commit(ctx context.Context, req Request) (Result, error) {
	if req.Admission.Outcome != guard.OutcomeReadyToCommit || req.Admission.Failure != nil {
		return Result{}, ErrNotReady
	}
	if err := c.validate(req); err != nil {
		return Result{}, err
	}

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	body := Mark(req.Doc.Body, now())
	hash := Hash(req.Doc.Body)

	if req.Target.Number == 0 {
		number, err := c.Store.Create(ctx, req.Title, body)
		if err != nil {
			return Result{}, fmt.Errorf("provenance: create issue: %w", err)
		}
		return Result{Outcome: OutcomeCommitted, Number: number, Hash: hash, Body: body}, nil
	}

	number := req.Target.Number
	current, err := c.Store.Revision(ctx, number)
	if err != nil {
		return Result{}, fmt.Errorf("%w: issue #%d: %w", ErrRevisionUnavailable, number, err)
	}
	if current != req.Target.ObservedRevision {
		return c.route(ctx, Conflict{Number: number, Observed: req.Target.ObservedRevision, Current: current})
	}
	if err := c.Store.Update(ctx, number, body); err != nil {
		return Result{}, fmt.Errorf("provenance: update issue #%d: %w", number, err)
	}
	return Result{Outcome: OutcomeCommitted, Number: number, Hash: hash, Body: body}, nil
}

// validate checks the request after the admission gate has passed.
func (c *Committer) validate(req Request) error {
	if c.Store == nil {
		return fmt.Errorf("%w: no issue store", ErrInvalid)
	}
	if req.Doc == nil {
		return fmt.Errorf("%w: no document", ErrInvalid)
	}
	if err := req.Doc.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if req.Doc.Result != stageoutput.ResultCanonical {
		return fmt.Errorf("%w: document result is %q, only %q can be committed", ErrInvalid, req.Doc.Result, stageoutput.ResultCanonical)
	}
	if Canonicalize(req.Doc.Body) == "" {
		return fmt.Errorf("%w: document body is empty after canonicalization", ErrInvalid)
	}
	switch {
	case req.Target.Number < 0:
		return fmt.Errorf("%w: target issue number %d is negative", ErrInvalid, req.Target.Number)
	case req.Target.Number == 0 && strings.TrimSpace(req.Title) == "":
		return fmt.Errorf("%w: title is required to create an issue", ErrInvalid)
	case req.Target.Number > 0 && req.Target.ObservedRevision == "":
		return fmt.Errorf("%w: observed revision is required to update issue #%d", ErrInvalid, req.Target.Number)
	}
	return nil
}

// route hands a conflicting issue to the router: intake first, punch-out if
// intake fails, ErrUnroutable if both fail. Nothing is written.
func (c *Committer) route(ctx context.Context, conflict Conflict) (Result, error) {
	if c.Router == nil {
		return Result{}, fmt.Errorf("%w: issue #%d changed and no router is configured", ErrUnroutable, conflict.Number)
	}
	intakeErr := c.Router.RouteToIntake(ctx, conflict)
	if intakeErr == nil {
		return Result{Outcome: OutcomeRoutedToIntake, Number: conflict.Number}, nil
	}
	punchErr := c.Router.PunchOut(ctx, conflict)
	if punchErr == nil {
		return Result{Outcome: OutcomePunchedOut, Number: conflict.Number}, nil
	}
	return Result{}, fmt.Errorf("%w: issue #%d: route to intake: %w; punch out: %w",
		ErrUnroutable, conflict.Number, intakeErr, punchErr)
}
