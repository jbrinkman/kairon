package planner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

const (
	// BoundaryGuard is FailureContext.Guard for a rejection this package
	// makes itself (extraction, mode contract, title, placeholders).
	BoundaryGuard = "planner-boundary"
	// IntakeGuard is FailureContext.Guard for a planner punch-out.
	IntakeGuard = "planner-intake"

	intakeExpected = "the required slots were fillable from the issue and the repository, directly or by grounded inference"
)

// Request is one call to Boundary.Handle.
type Request struct {
	Mode Mode
	// Replies is the final reply of each planner turn, in dependency order.
	// Automated mode requires exactly one.
	Replies []string
	// Attempt is the caller's retry position, normally from failure.Runner.
	// The zero value means {1, 1}.
	Attempt guard.Attempt
	// Known maps the keys of issues already committed in this split chain
	// (for example "issue-1") to their issue numbers. New documents continue
	// the numbering after them.
	Known map[string]int
	// Issue is the existing issue an automated intake works on. Interactive
	// requests leave it 0: every document creates an issue.
	Issue int
	// ObservedRevision is the issue revision recorded when the planner read
	// the issue. Automated canonical updates require it.
	ObservedRevision string
	// Repo, FailLabel and RunID address an automated punch-out.
	Repo, FailLabel, RunID string
}

// Outcome is what Handle did.
type Outcome int

// Outcomes. OutcomeInvalid is the zero value and accompanies every error
// that is not a failed GitHub write.
const (
	OutcomeInvalid Outcome = iota
	// OutcomeCommitted: every document was written (Result.Committed).
	OutcomeCommitted
	// OutcomePunchedOut: automated punch-out; the comment and label are in
	// place, or the issue changed under the planner and was punched out.
	OutcomePunchedOut
	// OutcomePunchOutPending: a punch-out was decided but a GitHub write
	// failed. The error is returned with it.
	OutcomePunchOutPending
	// OutcomeRoutedToIntake: the issue changed under the planner; nothing was
	// written and the issue was sent back for re-planning.
	OutcomeRoutedToIntake
	// OutcomeRejected: the reply broke the contract. Nothing was written.
	// Result.Failure says why and Result.Disposition is Recoverable.
	OutcomeRejected
)

// String returns a short name for o.
func (o Outcome) String() string {
	switch o {
	case OutcomeCommitted:
		return "committed"
	case OutcomePunchedOut:
		return "punched_out"
	case OutcomePunchOutPending:
		return "punch_out_pending"
	case OutcomeRoutedToIntake:
		return "routed_to_intake"
	case OutcomeRejected:
		return "rejected"
	}
	return "invalid"
}

// Result is what Handle reports.
type Result struct {
	Outcome Outcome
	// Committed lists the issues written, in order. It can be non-empty with
	// a non-committed Outcome only if the chain stopped part way; the keys
	// can be passed back as Request.Known.
	Committed []provenance.Committed
	// Failure is set for OutcomeRejected, and for a punch-out (where it is
	// the context that was posted, with Attempt{1,1}).
	Failure *guard.FailureContext
	// Disposition is failure.Recoverable for OutcomeRejected, so a
	// failure.Runner can retry the planner.
	Disposition failure.Disposition
	// Index is the position in Request.Replies of the rejected reply.
	Index int
}

// Boundary drives a planner reply through the guard chain to GitHub.
type Boundary struct {
	// Store creates and updates issues.
	Store provenance.IssueStore
	// Router receives an issue that changed under the planner.
	Router provenance.Router
	// Issues is the GitHub port for a punch-out.
	Issues failure.Issues
	// Now supplies the marker time. Nil means time.Now.
	Now func() time.Time
}

// prepared is one reply that passed extraction, the mode contract and the
// guard chain.
type prepared struct {
	doc   *stageoutput.Document
	title string // canonical only; "" when the issue keeps its GitHub title
}

// Handle runs the planner's final replies through the boundary.
//
// Every reply is extracted, checked against the mode contract and run through
// the guard chain before anything is written, so a bad later document cannot
// leave an earlier one committed. Then:
//
//   - a punch_out (automated only) goes to failure.PunchOut with a
//     FailureContext built from its unresolved list, Attempt{1,1}: comment
//     first, label second, no IssueStore call;
//   - canonical documents lose their H1 (the issue title), get the keys
//     issue-(len(Known)+i+1) and commit through CommitChainWith, whose admit
//     step runs the guard chain again on the substituted text.
//
// A reply that breaks the contract returns OutcomeRejected with a nil error.
// A non-nil error means the request was malformed (guard.ErrInvalid), a
// GitHub call failed, or the chain could not run.
func (b *Boundary) Handle(ctx context.Context, req Request) (Result, error) {
	attempt := req.Attempt
	if attempt == (guard.Attempt{}) {
		attempt = guard.Attempt{Current: 1, Max: 1}
	}
	if err := validateRequest(req, attempt); err != nil {
		return Result{}, err
	}

	rejectAt := func(index int, expected, actual string) Result {
		return Result{
			Outcome:     OutcomeRejected,
			Disposition: failure.Recoverable,
			Index:       index,
			Failure: &guard.FailureContext{
				Stage:    stageoutput.StagePlanner,
				Guard:    BoundaryGuard,
				Attempt:  attempt,
				Expected: expected,
				Actual:   actual,
			},
		}
	}

	if req.Mode == ModeAutomated && len(req.Replies) != 1 {
		return rejectAt(0, "automated intake returns exactly one stage-output document",
			fmt.Sprintf("%d replies", len(req.Replies))), nil
	}
	if len(req.Replies) == 0 {
		return rejectAt(0, "at least one reply containing a stage-output document", "no replies"), nil
	}

	docs := make([]prepared, 0, len(req.Replies))
	for i, reply := range req.Replies {
		p, rejected, err := b.prepare(req, attempt, i, reply)
		if err != nil {
			return Result{}, err
		}
		if rejected != nil {
			return *rejected, nil
		}
		docs = append(docs, p)
	}

	if docs[0].doc.Result == stageoutput.ResultPunchOut {
		return b.punchOut(req, docs[0].doc)
	}

	// Resolve placeholders on a dry run so an unknown or forward reference is
	// a rejection before the first write rather than a half-written chain.
	items := make([]provenance.ChainItem, len(docs))
	numbers := make(map[string]int, len(req.Known)+len(docs))
	for k, n := range req.Known {
		numbers[k] = n
	}
	for i, p := range docs {
		key := "issue-" + strconv.Itoa(len(req.Known)+i+1)
		if _, err := provenance.SubstituteDependencies(p.doc.Body, numbers); err != nil {
			if errors.Is(err, provenance.ErrUnknownPlaceholder) || errors.Is(err, provenance.ErrBadDependencyLine) {
				return rejectAt(i,
					"dependencies lines list #<number> or #{issue-N} for an earlier issue in this split (keys available here: "+availableKeys(numbers)+")",
					err.Error()), nil
			}
			return Result{}, err
		}
		numbers[key] = 1 // placeholder number: later documents may refer to this key
		items[i] = provenance.ChainItem{
			Key:   key,
			Title: p.title,
			Doc:   *p.doc,
			Target: provenance.Target{
				Number:           req.Issue,
				ObservedRevision: req.ObservedRevision,
			},
		}
	}

	var lastRoute *failure.Route
	admit := func(raw string) (guard.Result, error) {
		route := &failure.Route{}
		lastRoute = route
		return guard.Run(guard.Input{Stage: stageoutput.StagePlanner, Raw: raw, Attempt: attempt},
			guard.DefaultChain(route.Recoverable()))
	}
	committer := provenance.Committer{Store: b.Store, Router: b.Router, Now: b.Now}
	chain, err := committer.CommitChainWith(ctx, items, admit, req.Known)
	res := Result{Committed: chain.Committed}
	if err != nil {
		res.Index = max(chain.StoppedAt, 0)
		return res, err
	}
	if chain.Stopped == nil {
		res.Outcome = OutcomeCommitted
		return res, nil
	}

	res.Index = chain.StoppedAt
	switch chain.Stopped.Commit.Outcome {
	case provenance.OutcomeRoutedToIntake:
		res.Outcome = OutcomeRoutedToIntake
	case provenance.OutcomePunchedOut:
		res.Outcome = OutcomePunchedOut
	default:
		// The admit step rejected the substituted document.
		res.Outcome = OutcomeRejected
		res.Failure = chain.Stopped.Admission.Failure
		if lastRoute != nil {
			res.Disposition = lastRoute.Result().Disposition
		}
	}
	return res, nil
}

// prepare extracts one reply and applies the mode contract and the guard
// chain. It returns a non-nil *Result for a rejection.
func (b *Boundary) prepare(req Request, attempt guard.Attempt, index int, reply string) (prepared, *Result, error) {
	reject := func(guardName, expected, actual string) (prepared, *Result, error) {
		return prepared{}, &Result{
			Outcome:     OutcomeRejected,
			Disposition: failure.Recoverable,
			Index:       index,
			Failure: &guard.FailureContext{
				Stage:    stageoutput.StagePlanner,
				Guard:    guardName,
				Attempt:  attempt,
				Expected: expected,
				Actual:   actual,
			},
		}, nil
	}

	raw, err := Extract(reply)
	if err != nil {
		return reject(BoundaryGuard, "the whole reply is one stage-output document", err.Error())
	}

	// Mode contract. A document that does not parse skips it: the guard chain
	// below rejects it with the parser's message.
	doc, perr := stageoutput.Parse([]byte(raw))
	if perr == nil {
		if expected, actual, bad := modeContract(req, doc); bad {
			return reject(BoundaryGuard, expected, actual)
		}
	}

	route := &failure.Route{}
	gres, err := guard.Run(guard.Input{Stage: stageoutput.StagePlanner, Raw: raw, Attempt: attempt},
		guard.DefaultChain(route.Recoverable()))
	if err != nil {
		return prepared{}, nil, fmt.Errorf("planner: guard chain: %w", err)
	}
	if gres.Outcome != guard.OutcomeReadyToCommit {
		sr := route.Result()
		return prepared{}, &Result{
			Outcome:     OutcomeRejected,
			Failure:     gres.Failure,
			Disposition: sr.Disposition,
			Index:       index,
		}, nil
	}

	p := prepared{doc: doc}
	if doc.Result != stageoutput.ResultCanonical {
		return p, nil, nil
	}
	title, rest, ok := SplitTitle(doc.Body)
	switch {
	case !ok && req.Issue == 0:
		return reject(BoundaryGuard,
			"a body that starts with an H1 line (# Title); it becomes the issue title",
			"the first non-blank line of the body is not an H1")
	case ok:
		if provenance.Canonicalize(rest) == "" {
			return reject(BoundaryGuard, "a body with content after the H1 title", "the body is only the H1 title")
		}
		d := *doc
		d.Body = rest
		p.doc = &d
		if req.Issue == 0 {
			p.title = title
		}
	}
	return p, nil, nil
}

// modeContract checks the per-mode rules for a parsed document.
func modeContract(req Request, doc *stageoutput.Document) (expected, actual string, bad bool) {
	if doc.Stage != stageoutput.StagePlanner {
		return "stage: planner", fmt.Sprintf("stage: %s", doc.Stage), true
	}
	switch req.Mode {
	case ModeInteractive:
		if doc.Result == stageoutput.ResultPunchOut {
			return "a canonical document in interactive mode (a human is present, so there is no issue to comment on; ask the human instead)",
				"result: punch_out", true
		}
	case ModeAutomated:
		wantRef := strconv.Itoa(req.Issue)
		if doc.Source.Kind != stageoutput.SourceGitHubIssue || doc.Source.Ref != wantRef {
			return fmt.Sprintf("source kind github_issue with ref %q", wantRef),
				fmt.Sprintf("source kind %q with ref %q", doc.Source.Kind, doc.Source.Ref), true
		}
	}
	if doc.Result == stageoutput.ResultCanonical && strings.Contains(doc.Body, provenance.MarkerPrefix) {
		return "a body without a provenance marker (the boundary writes the marker, not the planner)",
			"the body contains " + provenance.MarkerPrefix, true
	}
	return "", "", false
}

// punchOut sends an automated punch_out document to Tier 2.
func (b *Boundary) punchOut(req Request, doc *stageoutput.Document) (Result, error) {
	lines := make([]string, len(doc.Unresolved))
	for i, u := range doc.Unresolved {
		lines[i] = u.Slot + ": " + u.Reason
	}
	fc := guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    IntakeGuard,
		Attempt:  guard.Attempt{Current: 1, Max: 1},
		Expected: intakeExpected,
		Actual:   strings.Join(lines, "\n"),
	}
	target := failure.Target{Repo: req.Repo, Issue: req.Issue, FailLabel: req.FailLabel}
	if err := failure.PunchOut(b.Issues, target, req.RunID, fc, ""); err != nil {
		if errors.Is(err, guard.ErrInvalid) {
			return Result{Failure: &fc}, err
		}
		return Result{Outcome: OutcomePunchOutPending, Failure: &fc}, err
	}
	return Result{Outcome: OutcomePunchedOut, Failure: &fc}, nil
}

func validateRequest(req Request, attempt guard.Attempt) error {
	var p []string
	switch req.Mode {
	case ModeInteractive:
		if req.Issue != 0 {
			p = append(p, "interactive mode creates issues, so Issue must be 0")
		}
	case ModeAutomated:
		if req.Issue <= 0 {
			p = append(p, "automated mode needs the issue number")
		}
	default:
		p = append(p, fmt.Sprintf("unknown mode %d", int(req.Mode)))
	}
	if !attempt.Valid() {
		p = append(p, fmt.Sprintf("attempt %d of %d is not 1 <= current <= max", attempt.Current, attempt.Max))
	}
	if len(p) > 0 {
		return fmt.Errorf("%w: planner request: %s", guard.ErrInvalid, strings.Join(p, "; "))
	}
	return nil
}

// availableKeys lists the placeholder keys a document may refer to.
func availableKeys(numbers map[string]int) string {
	if len(numbers) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(numbers))
	for k := range numbers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
