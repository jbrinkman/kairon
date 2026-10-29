package failure

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

func fcAt(cur, max int) guard.FailureContext {
	return guard.FailureContext{
		Stage:    stageoutput.StageKrewLead,
		Guard:    "implementation-run",
		Attempt:  guard.Attempt{Current: cur, Max: max},
		Expected: "exit 0",
		Actual:   "exit 1",
	}
}

func TestPolicyFor_FollowsConfig(t *testing.T) {
	cfg := &config.Config{MaxRetries: 2, MaxQARetries: 5}
	if got := PolicyFor(cfg, BoundaryImplementation).MaxAttempts; got != 3 {
		t.Errorf("implementation MaxAttempts = %d, want 3", got)
	}
	if got := PolicyFor(cfg, BoundaryQA).MaxAttempts; got != 6 {
		t.Errorf("QA MaxAttempts = %d, want 6", got)
	}

	// The two knobs are independent.
	cfg.MaxRetries = 7
	if got := PolicyFor(cfg, BoundaryImplementation).MaxAttempts; got != 8 {
		t.Errorf("implementation MaxAttempts after override = %d, want 8", got)
	}
	if got := PolicyFor(cfg, BoundaryQA).MaxAttempts; got != 6 {
		t.Errorf("QA MaxAttempts changed with MaxRetries: got %d, want 6", got)
	}
	cfg.MaxQARetries = 0
	if got := PolicyFor(cfg, BoundaryQA).MaxAttempts; got != 1 {
		t.Errorf("QA MaxAttempts with zero retries = %d, want 1", got)
	}
	if got := PolicyFor(cfg, BoundaryImplementation).MaxAttempts; got != 8 {
		t.Errorf("implementation MaxAttempts changed with MaxQARetries: got %d, want 8", got)
	}

	cfg.MaxRetries = -4
	if got := PolicyFor(cfg, BoundaryImplementation).MaxAttempts; got != 1 {
		t.Errorf("negative MaxRetries MaxAttempts = %d, want 1", got)
	}
}

func TestPolicyFor_NilConfigAndUnknownBoundaryAllowOneAttempt(t *testing.T) {
	if got := PolicyFor(nil, BoundaryImplementation).MaxAttempts; got != 1 {
		t.Errorf("nil config MaxAttempts = %d, want 1", got)
	}
	if got := PolicyFor(&config.Config{MaxRetries: 9, MaxQARetries: 9}, Boundary(99)).MaxAttempts; got != 1 {
		t.Errorf("unknown boundary MaxAttempts = %d, want 1", got)
	}
}

// TestDecide_InvalidAttemptFailsToPunchOut is the Decide decision table:
// unrecoverable failures always punch out, recoverable ones retry while
// attempts remain and punch out at the maximum, and an unusable attempt must
// never produce an unbounded retry.
func TestDecide_InvalidAttemptFailsToPunchOut(t *testing.T) {
	cases := []struct {
		name    string
		attempt guard.Attempt
		disp    Disposition
		want    Decision
	}{
		{"unrecoverable 1 of 4", guard.Attempt{Current: 1, Max: 4}, Unrecoverable, DecisionPunchOut},
		{"unrecoverable 2 of 4", guard.Attempt{Current: 2, Max: 4}, Unrecoverable, DecisionPunchOut},
		{"unrecoverable 1 of 1", guard.Attempt{Current: 1, Max: 1}, Unrecoverable, DecisionPunchOut},
		{"unrecoverable 4 of 4", guard.Attempt{Current: 4, Max: 4}, Unrecoverable, DecisionPunchOut},
		{"recoverable 1 of 4", guard.Attempt{Current: 1, Max: 4}, Recoverable, DecisionRetry},
		{"recoverable 2 of 4", guard.Attempt{Current: 2, Max: 4}, Recoverable, DecisionRetry},
		{"recoverable 3 of 4", guard.Attempt{Current: 3, Max: 4}, Recoverable, DecisionRetry},
		{"recoverable 4 of 4", guard.Attempt{Current: 4, Max: 4}, Recoverable, DecisionPunchOut},
		{"recoverable 1 of 1", guard.Attempt{Current: 1, Max: 1}, Recoverable, DecisionPunchOut},
		{"invalid zero attempt", guard.Attempt{}, Recoverable, DecisionPunchOut},
		{"invalid current above max", guard.Attempt{Current: 5, Max: 4}, Recoverable, DecisionPunchOut},
		{"invalid zero max", guard.Attempt{Current: 1, Max: 0}, Recoverable, DecisionPunchOut},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fc := fcAt(1, 1)
			fc.Attempt = c.attempt
			if got := Decide(fc, c.disp); got != c.want {
				t.Errorf("Decide(%+v, %v) = %v, want %v", c.attempt, c.disp, got, c.want)
			}
		})
	}
}

func TestLabels(t *testing.T) {
	if got := FailedLabel("kairon"); got != "kairon-failed" {
		t.Errorf("FailedLabel = %q", got)
	}
	if got := DoneLabel("my-label"); got != "my-label-done" {
		t.Errorf("DoneLabel = %q", got)
	}
}
