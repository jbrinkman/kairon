package failure

import (
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/guard"
)

// Policy bounds how many times a stage may execute.
type Policy struct {
	// MaxAttempts is the total number of executions of the stage: the first
	// run plus the configured retries.
	MaxAttempts int
}

// PolicyFromRetries builds a Policy from a configured retry count (the number
// of re-runs after the first run). It is the only place the +1 is written. A
// negative count is treated as zero retries.
func PolicyFromRetries(maxRetries int) Policy {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return Policy{MaxAttempts: maxRetries + 1}
}

// Boundary names a retry boundary that has its own configured budget.
type Boundary int

// Boundaries. Zero is deliberately not a boundary.
const (
	// BoundaryImplementation is governed by config max_retries.
	BoundaryImplementation Boundary = iota + 1
	// BoundaryQA is governed by config max_qa_retries.
	BoundaryQA
)

// PolicyFor returns the Policy for boundary b from cfg. A nil cfg or an unknown
// boundary allows a single attempt, so a misconfiguration never grants an
// unbounded or larger retry budget.
func PolicyFor(cfg *config.Config, b Boundary) Policy {
	if cfg == nil {
		return PolicyFromRetries(0)
	}
	switch b {
	case BoundaryImplementation:
		return PolicyFromRetries(cfg.MaxRetries)
	case BoundaryQA:
		return PolicyFromRetries(cfg.MaxQARetries)
	}
	return PolicyFromRetries(0)
}

// Attempt returns the guard.Attempt for the n-th execution (1-based) under p.
func (p Policy) Attempt(n int) guard.Attempt {
	return guard.Attempt{Current: n, Max: p.MaxAttempts}
}

// Disposition is how the failing guard classifies a failure.
type Disposition int

// Dispositions. Recoverable is the zero value.
const (
	// Recoverable failures are retried while attempts remain.
	Recoverable Disposition = iota
	// Unrecoverable failures punch out immediately, whatever the attempt.
	Unrecoverable
)

// Decision is what the boundary does next.
type Decision int

// Decisions. Zero is deliberately not a decision.
const (
	DecisionRetry Decision = iota + 1
	DecisionPunchOut
)

// Decide chooses between another attempt and a punch-out. Unrecoverable
// failures always punch out. Recoverable failures retry while
// fc.Attempt.Current < fc.Attempt.Max, so an unusable attempt (for example
// Max of zero) fails toward a punch-out and never toward an extra retry.
func Decide(fc guard.FailureContext, d Disposition) Decision {
	if d == Unrecoverable {
		return DecisionPunchOut
	}
	if fc.Attempt.Valid() && fc.Attempt.Current < fc.Attempt.Max {
		return DecisionRetry
	}
	return DecisionPunchOut
}

// FailedLabel returns the fail-state label for a trigger label.
func FailedLabel(triggerLabel string) string { return triggerLabel + "-failed" }

// DoneLabel returns the done-state label for a trigger label.
func DoneLabel(triggerLabel string) string { return triggerLabel + "-done" }
