package failure

import (
	"fmt"

	"github.com/jbrinkman/kairon/internal/guard"
)

// Stage runs one attempt and reports how it went. The attempt it receives is
// the Runner's position, so a stage can pass it on to guard.Input.
type Stage func(attempt guard.Attempt) StageResult

// Outcome is how a Runner.Run ended. Zero is deliberately not an outcome.
type Outcome int

// Outcomes.
const (
	// OutcomeSucceeded: the stage succeeded, possibly after retries.
	OutcomeSucceeded Outcome = iota + 1
	// OutcomePunchedOut: the comment and label are both in place.
	OutcomePunchedOut
	// OutcomePunchOutPending: a punch-out was decided but a GitHub call failed.
	// If the comment landed and the label did not, the issue is terminal-pending
	// and Reconcile finishes the label. If the state read or the comment write
	// failed, nothing was written and the issue stays eligible.
	OutcomePunchOutPending
	// OutcomeHalted: the issue is labeled or terminal-pending, so the stage
	// was not called.
	OutcomeHalted
)

// Result is what Runner.Run reports.
type Result struct {
	Outcome Outcome
	// Attempts is how many times the stage ran in this call.
	Attempts int
	// Failure is the last failure, with the Runner's attempt counter. Nil when
	// the stage succeeded or the Run halted before calling it.
	Failure *guard.FailureContext
}

// Runner is the synchronous Tier 1 loop and Tier 2 trigger. It re-runs a
// failing stage while attempts remain, and punches out when they are used up
// or the failure is unrecoverable. Event-driven callers (the agent Manager)
// share Decide and PunchOut instead of using a loop.
type Runner struct {
	Issues Issues
	Target Target
	Policy Policy
	// RunID scopes the punch-out marker to this terminal run. The caller
	// supplies it (see NewRunID); there is no fallback.
	RunID string
}

// Run executes stage under the retry policy.
//
// It reads the issue first. Unless the issue is eligible (no done or failed
// label, and not terminal-pending) it returns OutcomeHalted without calling
// the stage, so nothing re-enters work after a punch-out. The attempt counter
// starts at 1 on every call and lives only inside it; clearing the label and
// calling Run again is a fresh run with a fresh budget.
//
// After each failure the Runner overwrites Failure.Attempt with its own
// counter, so a stage cannot misreport attempts, and validates the context. An
// invalid one returns an error wrapping guard.ErrInvalid with no GitHub write.
// A retry writes nothing. A punch-out writes the comment and then the label,
// and the stage is not called again. If a punch-out write fails, Run returns
// OutcomePunchOutPending and that error.
func (r Runner) Run(stage Stage) (Result, error) {
	if err := r.validate(stage); err != nil {
		return Result{}, err
	}

	st, err := r.Issues.State(r.Target.Repo, r.Target.Issue, r.Target.FailLabel)
	if err != nil {
		return Result{}, fmt.Errorf("read issue state: %w", err)
	}
	if st.Classify(r.Target.doneLabel(), r.Target.FailLabel) != SelectEligible {
		return Result{Outcome: OutcomeHalted}, nil
	}

	for n := 1; ; n++ {
		out := stage(r.Policy.Attempt(n))
		if out.Failure == nil {
			return Result{Outcome: OutcomeSucceeded, Attempts: n}, nil
		}

		fc := *out.Failure
		fc.Attempt = r.Policy.Attempt(n)
		if err := fc.Validate(); err != nil {
			return Result{Attempts: n}, err
		}

		if Decide(fc, out.Disposition) == DecisionRetry {
			continue
		}
		if err := PunchOut(r.Issues, r.Target, r.RunID, fc, out.Remedy); err != nil {
			return Result{Outcome: OutcomePunchOutPending, Attempts: n, Failure: &fc}, err
		}
		return Result{Outcome: OutcomePunchedOut, Attempts: n, Failure: &fc}, nil
	}
}

func (r Runner) validate(stage Stage) error {
	if stage == nil {
		return fmt.Errorf("%w: stage is nil", guard.ErrInvalid)
	}
	if err := validateTarget(r.Issues, r.Target); err != nil {
		return err
	}
	if !ValidRunID(r.RunID) {
		return fmt.Errorf("%w: run id %q is not valid", guard.ErrInvalid, r.RunID)
	}
	if r.Policy.MaxAttempts < 1 {
		return fmt.Errorf("%w: policy allows %d attempts, need at least 1", guard.ErrInvalid, r.Policy.MaxAttempts)
	}
	return nil
}
