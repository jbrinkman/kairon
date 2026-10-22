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

func TestPolicyFromRetries(t *testing.T) {
	cases := []struct{ retries, want int }{
		{0, 1}, {1, 2}, {3, 4}, {10, 11}, {-1, 1}, {-100, 1},
	}
	for _, c := range cases {
		if got := PolicyFromRetries(c.retries).MaxAttempts; got != c.want {
			t.Errorf("PolicyFromRetries(%d).MaxAttempts = %d, want %d", c.retries, got, c.want)
		}
	}
}

func TestPolicyAttempt(t *testing.T) {
	got := PolicyFromRetries(2).Attempt(2)
	if want := (guard.Attempt{Current: 2, Max: 3}); got != want {
		t.Errorf("Attempt(2) = %+v, want %+v", got, want)
	}
	if !PolicyFromRetries(0).Attempt(1).Valid() {
		t.Error("Attempt(1) under zero retries should be a valid guard.Attempt")
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

func TestDecide_UnrecoverableAlwaysPunchesOut(t *testing.T) {
	for _, fc := range []guard.FailureContext{fcAt(1, 4), fcAt(2, 4), fcAt(1, 1), fcAt(4, 4)} {
		if got := Decide(fc, Unrecoverable); got != DecisionPunchOut {
			t.Errorf("Decide(%+v, Unrecoverable) = %v, want PunchOut", fc.Attempt, got)
		}
	}
}

func TestDecide_RecoverableRetriesWhileAttemptsRemain(t *testing.T) {
	for cur := 1; cur < 4; cur++ {
		if got := Decide(fcAt(cur, 4), Recoverable); got != DecisionRetry {
			t.Errorf("Decide(%d of 4, Recoverable) = %v, want Retry", cur, got)
		}
	}
}

func TestDecide_RecoverablePunchesOutAtMax(t *testing.T) {
	if got := Decide(fcAt(4, 4), Recoverable); got != DecisionPunchOut {
		t.Errorf("Decide(4 of 4) = %v, want PunchOut", got)
	}
	if got := Decide(fcAt(1, 1), Recoverable); got != DecisionPunchOut {
		t.Errorf("Decide(1 of 1) = %v, want PunchOut", got)
	}
}

func TestDecide_InvalidAttemptFailsToPunchOut(t *testing.T) {
	// An unusable attempt must never produce an unbounded retry.
	for _, a := range []guard.Attempt{{}, {Current: 5, Max: 4}, {Current: 1, Max: 0}} {
		fc := fcAt(1, 1)
		fc.Attempt = a
		if got := Decide(fc, Recoverable); got != DecisionPunchOut {
			t.Errorf("Decide(%+v) = %v, want PunchOut", a, got)
		}
	}
}

func TestDispositionZeroValueIsRecoverable(t *testing.T) {
	var d Disposition
	if d != Recoverable {
		t.Error("zero Disposition must be Recoverable")
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
