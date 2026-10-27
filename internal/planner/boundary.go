package planner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// ErrNoPunchOutTarget means the planner returned a punch_out document but the
// boundary has no issue to comment on. It matches guard.ErrInvalid. A
// punch-out is addressed to a human on an existing issue, so on the create
// path (no issue yet) a punch_out document is a contract violation, not
// something to swallow.
var ErrNoPunchOutTarget = fmt.Errorf("%w: planner punch_out has no target issue to comment on", guard.ErrInvalid)

// Names the boundary reports in guard.FailureContext.Guard for failures that
// no guard in the chain produced.
const (
	// ExtractGuardName is reported when the reply is not exactly one
	// stage-output document.
	ExtractGuardName = "extract"
	// IntakeGuardName is reported for a planner punch_out: the planner itself
	// decided a required slot could not be filled.
	IntakeGuardName = "intake"
	// ProvenanceGuardName is reported when the issue changed under the planner
	// and the conflict had to be punched out.
	ProvenanceGuardName = "provenance"
)

// Outcome is what Process did with an agent reply.
type Outcome int

// Outcomes Process may report. OutcomeInvalid is the zero value and is
// returned alongside every error.
const (
	OutcomeInvalid Outcome = iota
	// OutcomeCommitted: the body and its provenance marker were written once.
	OutcomeCommitted
	// OutcomeRejected: the reply failed extraction or a guard. Nothing was
	// written. Result.Failure says why and Result.Disposition says whether a
	// retry may help.
	OutcomeRejected
	// OutcomeRoutedToIntake: the issue changed under the planner. Nothing was
	// written; the issue goes back to intake.
	OutcomeRoutedToIntake
	// OutcomePunchedOut: a human was told why the pipeline stopped (a comment,
	// then the fail-state label). Nothing was written to the issue body.
	OutcomePunchedOut
)

// String returns a short name for o.
func (o Outcome) String() string {
	switch o {
	case OutcomeCommitted:
		return "committed"
	case OutcomeRejected:
		return "rejected"
	case OutcomeRoutedToIntake:
		return "routed_to_intake"
	case OutcomePunchedOut:
		return "punched_out"
	}
	return "invalid"
}

// Boundary is the planner output boundary. The agent never writes to GitHub:
// it returns a stage-output document, and Process validates it, then hands it
// to the commit step (canonical) or to the Tier 2 failure contract
// (punch_out). Boundary is mode-agnostic: the interactive and the automated
// agent return the same document and take the same path.
type Boundary struct {
	// Committer performs the provenance commit for a canonical document. Its
	// Router decides what happens when the issue changed under the planner.
	Committer *provenance.Committer
	// Issues is the punch-out port (internal/github.CLI satisfies it).
	Issues failure.Issues
	// Target is the issue and fail-state label a planner punch_out acts on.
	// Target.Issue is 0 on the create path, where a punch_out is a contract
	// error.
	Target failure.Target
	// Guards builds the guard chain from the failure handler it should invoke.
	// Nil means guard.DefaultChain followed by DependenciesForm.
	Guards func(guard.FailureHandler) []guard.Guard
	// NewRunID supplies the run id that scopes a punch-out comment. Nil means
	// failure.NewRunID.
	NewRunID func() string
}

// Request is one agent reply to process.
type Request struct {
	// Reply is the agent's whole reply.
	Reply string
	// Attempt is the caller's retry position. This package never counts
	// attempts.
	Attempt guard.Attempt
	// Target says where a canonical body goes: Number 0 creates a new issue,
	// otherwise Number is updated if its revision still equals
	// ObservedRevision.
	Target provenance.Target
	// Title is the issue title for the create path. Empty means the first
	// "# " heading of the body.
	Title string
}

// Result is what Process reports.
type Result struct {
	Outcome Outcome
	// Number is the issue written (OutcomeCommitted) or the conflicting issue
	// (OutcomeRoutedToIntake, and OutcomePunchedOut after a conflict).
	Number int
	// Hash and Body are what the committer wrote. Empty unless
	// OutcomeCommitted.
	Hash string
	Body string
	// Failure is why a reply was rejected or punched out. Nil when committed
	// or routed to intake.
	Failure *guard.FailureContext
	// Disposition says whether a rejection may be retried. It is Recoverable
	// for every rejection and Unrecoverable for a planner punch_out.
	Disposition failure.Disposition
	// Document is the parsed document, when the reply parsed.
	Document *stageoutput.Document
}

// defaultGuards is the chain Process runs when Boundary.Guards is nil: the
// merged default chain, then the planner-side dependencies-form guard.
func defaultGuards(handler guard.FailureHandler) []guard.Guard {
	return append(guard.DefaultChain(handler), NewDependenciesForm(handler))
}

// Process turns one agent reply into at most one GitHub write.
//
//  1. Extract the document from the reply. A reply that is not exactly one
//     document is rejected (recoverable); nothing is written.
//  2. Parse it. A document that does not parse is run through the guard chain
//     so the structural guard produces the FailureContext, and is rejected.
//  3. Route on the document's result:
//     canonical: run the guard chain. A guard failure is rejected
//     (recoverable) with its FailureContext and nothing is written. When every
//     guard passes, Committer.Commit writes the body with its provenance
//     marker, or routes a conflict.
//     punch_out: require Target.Issue, build the FailureContext and call
//     failure.PunchOut: the explanatory comment, then the fail-state label.
//     The issue body is never written.
//
// Process has no mode parameter: it routes on the document's result only. It
// calls neither gh nor any hashing code; the only writers are Committer and
// failure.PunchOut. A non-nil error means Process could not run (an invalid
// attempt, a missing collaborator, or a GitHub call that failed); the Result
// is then the zero Result.
func (b *Boundary) Process(ctx context.Context, req Request) (Result, error) {
	if !req.Attempt.Valid() {
		return Result{}, fmt.Errorf("%w: attempt %d of %d is not 1 <= current <= max",
			guard.ErrInvalid, req.Attempt.Current, req.Attempt.Max)
	}
	route := &failure.Route{}

	text, err := Extract(req.Reply)
	if err != nil {
		fc := guard.FailureContext{
			Stage:    stageoutput.StagePlanner,
			Guard:    ExtractGuardName,
			Attempt:  req.Attempt,
			Expected: "the reply is exactly one stage-output document and nothing else",
			Actual:   err.Error(),
		}
		if err := route.Recoverable()(fc); err != nil {
			return Result{}, err
		}
		return rejected(route.Result(), nil), nil
	}

	doc, err := stageoutput.Parse([]byte(text))
	if err != nil {
		// The structural guard produces the FailureContext for a document
		// that does not parse, so the failure reads the same as any other.
		gres, gerr := b.runGuards(route, text, req.Attempt)
		if gerr != nil {
			return Result{}, gerr
		}
		if gres.Outcome != guard.OutcomeRejected {
			return Result{}, fmt.Errorf("planner: document does not parse (%w) but the guard chain admitted it", err)
		}
		return rejected(route.Result(), nil), gres.FailureErr
	}

	switch doc.Result {
	case stageoutput.ResultCanonical:
		return b.processCanonical(ctx, route, text, doc, req)
	case stageoutput.ResultPunchOut:
		return b.processPunchOut(doc)
	}
	return Result{}, fmt.Errorf("%w: document result %q is not routable", guard.ErrInvalid, doc.Result)
}

// processCanonical runs the guard chain and, when every guard passes, the
// commit step.
func (b *Boundary) processCanonical(ctx context.Context, route *failure.Route, text string, doc *stageoutput.Document, req Request) (Result, error) {
	gres, err := b.runGuards(route, text, req.Attempt)
	if err != nil {
		return Result{}, err
	}
	if gres.Outcome != guard.OutcomeReadyToCommit {
		out := rejected(route.Result(), doc)
		if out.Failure == nil && gres.Failure != nil {
			fc := *gres.Failure
			out.Failure = &fc
		}
		return out, gres.FailureErr
	}

	if b.Committer == nil {
		return Result{}, fmt.Errorf("%w: boundary has no committer", provenance.ErrInvalid)
	}
	title := req.Title
	if strings.TrimSpace(title) == "" {
		title = titleFromBody(doc.Body)
	}
	committed, err := b.Committer.Commit(ctx, provenance.Request{
		Doc:       doc,
		Title:     title,
		Target:    req.Target,
		Admission: gres,
	})
	if err != nil {
		return Result{}, err
	}
	out := Result{Number: committed.Number, Document: doc}
	switch committed.Outcome {
	case provenance.OutcomeCommitted:
		out.Outcome, out.Hash, out.Body = OutcomeCommitted, committed.Hash, committed.Body
	case provenance.OutcomeRoutedToIntake:
		out.Outcome = OutcomeRoutedToIntake
	case provenance.OutcomePunchedOut:
		out.Outcome = OutcomePunchedOut
	default:
		return Result{}, fmt.Errorf("planner: committer reported outcome %v", committed.Outcome)
	}
	return out, nil
}

// processPunchOut sends a planner punch_out to the Tier 2 failure contract.
func (b *Boundary) processPunchOut(doc *stageoutput.Document) (Result, error) {
	if b.Target.Issue <= 0 {
		return Result{}, ErrNoPunchOutTarget
	}
	fc := guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    IntakeGuardName,
		Attempt:  guard.Attempt{Current: 1, Max: 1},
		Expected: "every required slot is fillable from the request and the repository",
		Actual:   unresolvedList(doc.Unresolved),
	}
	remedy := fmt.Sprintf("Add the missing information named above to the issue, then remove the `%s` label to run intake again.",
		b.Target.FailLabel)
	if err := failure.PunchOut(b.Issues, b.Target, b.runID(), fc, remedy); err != nil {
		return Result{}, fmt.Errorf("planner: punch out issue #%d: %w", b.Target.Issue, err)
	}
	return Result{
		Outcome:     OutcomePunchedOut,
		Number:      b.Target.Issue,
		Failure:     &fc,
		Disposition: failure.Unrecoverable,
		Document:    doc,
	}, nil
}

// runGuards runs the guard chain over text. The failure handler is the
// route's Recoverable handler, so a rejection can be retried.
func (b *Boundary) runGuards(route *failure.Route, text string, attempt guard.Attempt) (guard.Result, error) {
	build := b.Guards
	if build == nil {
		build = defaultGuards
	}
	return guard.Run(guard.Input{Stage: stageoutput.StagePlanner, Raw: text, Attempt: attempt}, build(route.Recoverable()))
}

func (b *Boundary) runID() string {
	if b.NewRunID != nil {
		return b.NewRunID()
	}
	return failure.NewRunID()
}

// rejected builds an OutcomeRejected result from what the route recorded.
func rejected(sr failure.StageResult, doc *stageoutput.Document) Result {
	return Result{Outcome: OutcomeRejected, Failure: sr.Failure, Disposition: sr.Disposition, Document: doc}
}

// unresolvedList renders a punch_out's unresolved slots, one "slot: reason"
// per line.
func unresolvedList(items []stageoutput.Unresolved) string {
	lines := make([]string, 0, len(items))
	for _, u := range items {
		lines = append(lines, u.Slot+": "+u.Reason)
	}
	return strings.Join(lines, "\n")
}

// titleFromBody returns the text of the first level-1 markdown heading outside
// a fenced code block, or "" when there is none.
func titleFromBody(body string) string {
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

// ConflictRouter is the provenance.Router for the boundary. When the issue
// changed under the planner nothing was written, so RouteToIntake has nothing
// to undo: the issue is still unmarked or mismatched and the watcher's own
// routing picks it up again. PunchOut is the fallback and uses the Tier 2
// failure contract.
type ConflictRouter struct {
	Issues failure.Issues
	// Target supplies the repo and the fail-state label. Its Issue is replaced
	// by the conflicting issue.
	Target failure.Target
	// NewRunID supplies the punch-out run id. Nil means failure.NewRunID.
	NewRunID func() string
}

var _ provenance.Router = (*ConflictRouter)(nil)

// RouteToIntake implements provenance.Router. It writes nothing.
func (r *ConflictRouter) RouteToIntake(context.Context, provenance.Conflict) error { return nil }

// PunchOut implements provenance.Router.
func (r *ConflictRouter) PunchOut(_ context.Context, c provenance.Conflict) error {
	if r == nil {
		return errors.New("planner: no conflict router")
	}
	t := r.Target
	t.Issue = c.Number
	runID := failure.NewRunID
	if r.NewRunID != nil {
		runID = r.NewRunID
	}
	fc := guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    ProvenanceGuardName,
		Attempt:  guard.Attempt{Current: 1, Max: 1},
		Expected: "the issue is unchanged between the planner's read and the commit",
		Actual: fmt.Sprintf("issue #%d changed under the planner (revision %s when read, %s at commit); nothing was written",
			c.Number, c.Observed, c.Current),
	}
	remedy := fmt.Sprintf("The issue was edited while it was being planned. Review the edit, then remove the `%s` label to plan it again.",
		t.FailLabel)
	return failure.PunchOut(r.Issues, t, runID(), fc, remedy)
}
