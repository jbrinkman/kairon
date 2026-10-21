package failure

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/guard"
)

// failing returns a StageResult that reports a recoverable failure. The
// reported attempt is deliberately wrong so tests prove the runner overwrites it.
func failing() StageResult {
	fc := testFC()
	fc.Attempt = guard.Attempt{Current: 99, Max: 99}
	return StageResult{Failure: &fc}
}

func newRunner(f *fakeIssues, maxRetries int) Runner {
	return Runner{Issues: f, Target: testTarget, Policy: PolicyFromRetries(maxRetries), RunID: "run1"}
}

func TestRunner_RecoversAfterTwoFailures(t *testing.T) {
	f := newFake()
	runs := 0
	stage := func(a guard.Attempt) StageResult {
		runs++
		if runs <= 2 {
			return failing()
		}
		return StageResult{}
	}
	res, err := newRunner(f, 2).Run(stage)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != OutcomeSucceeded || res.Attempts != 3 {
		t.Fatalf("result = %+v, want succeeded after 3 attempts", res)
	}
	if runs != 3 {
		t.Fatalf("stage ran %d times, want 3", runs)
	}
	if n := f.count("AddLabel"); n != 0 {
		t.Errorf("AddLabel called %d times, want 0", n)
	}
	if n := f.count("PostComment"); n != 0 {
		t.Errorf("PostComment called %d times, want 0", n)
	}
}

func TestRunner_ExhaustionPunchesOut(t *testing.T) {
	f := newFake()
	r := newRunner(f, 3)
	runs := 0
	stage := func(a guard.Attempt) StageResult { runs++; return failing() }

	res, err := r.Run(stage)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runs != r.Policy.MaxAttempts {
		t.Fatalf("stage ran %d times, want Policy.MaxAttempts = %d", runs, r.Policy.MaxAttempts)
	}
	if res.Outcome != OutcomePunchedOut {
		t.Fatalf("Outcome = %v, want OutcomePunchedOut", res.Outcome)
	}
	if n := f.count("PostComment"); n != 1 {
		t.Errorf("comments = %d, want exactly 1", n)
	}
	if n := f.count("AddLabel " + testFailLabel); n != 1 {
		t.Errorf("AddLabel(%s) calls = %d, want 1; calls = %v", testFailLabel, n, f.calls)
	}
	if res.Failure == nil || res.Failure.Attempt != (guard.Attempt{Current: 4, Max: 4}) {
		t.Errorf("Failure = %+v, want attempt 4 of 4", res.Failure)
	}

	// The stage is not called again after the punch-out.
	if res2, err := r.Run(stage); err != nil || res2.Outcome != OutcomeHalted {
		t.Fatalf("second Run = (%+v, %v), want halted", res2, err)
	}
	if runs != r.Policy.MaxAttempts {
		t.Fatalf("stage ran again after punch-out: %d runs", runs)
	}
}

func TestRunner_UnrecoverableSkipsRetries(t *testing.T) {
	f := newFake()
	runs := 0
	stage := func(a guard.Attempt) StageResult {
		runs++
		var route Route
		fc := testFC()
		fc.Attempt = a
		if err := route.Terminal("rewrite the body")(fc); err != nil {
			t.Fatalf("handler: %v", err)
		}
		return route.Result()
	}
	res, err := newRunner(f, 5).Run(stage)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runs != 1 {
		t.Fatalf("stage ran %d times, want 1 (zero retries)", runs)
	}
	if res.Outcome != OutcomePunchedOut || res.Attempts != 1 {
		t.Fatalf("result = %+v, want punched out on attempt 1", res)
	}
	if f.count("PostComment") != 1 || f.count("AddLabel "+testFailLabel) != 1 {
		t.Fatalf("calls = %v", f.calls)
	}
	if !strings.Contains(f.comments[0].Body, "rewrite the body") {
		t.Errorf("comment does not carry the route's remedy:\n%s", f.comments[0].Body)
	}
}

func TestRunner_RetryCountFollowsConfig(t *testing.T) {
	for _, retries := range []int{0, 1, 2, 5} {
		f := newFake()
		cfg := &config.Config{MaxRetries: retries}
		r := Runner{Issues: f, Target: testTarget, Policy: PolicyFor(cfg, BoundaryImplementation), RunID: "run1"}
		runs := 0
		if _, err := r.Run(func(guard.Attempt) StageResult { runs++; return failing() }); err != nil {
			t.Fatalf("retries=%d: %v", retries, err)
		}
		if runs != retries+1 {
			t.Errorf("max_retries=%d: stage ran %d times, want %d", retries, runs, retries+1)
		}
	}
}

func TestRunner_RefusesWhileLabeled(t *testing.T) {
	for _, label := range []string{testFailLabel, "kairon-done"} {
		t.Run(label, func(t *testing.T) {
			f := newFake()
			f.seedLabel(label)
			called := false
			res, err := newRunner(f, 3).Run(func(guard.Attempt) StageResult { called = true; return StageResult{} })
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if res.Outcome != OutcomeHalted || res.Attempts != 0 {
				t.Fatalf("result = %+v, want halted with 0 attempts", res)
			}
			if called {
				t.Fatal("stage was called while the issue is labeled")
			}
			if w := f.writes(); len(w) != 0 {
				t.Fatalf("writes = %v", w)
			}
		})
	}
}

func TestRunner_RefusesWhileTerminalPending(t *testing.T) {
	f := newFake()
	f.seedComment(Marker("earlier")+"\n", true) // comment landed, label did not
	called := false
	res, err := newRunner(f, 3).Run(func(guard.Attempt) StageResult { called = true; return StageResult{} })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != OutcomeHalted || called {
		t.Fatalf("result = %+v, called = %v; want halted without calling the stage", res, called)
	}
}

func TestRunner_FreshRunStartsAtAttemptOne(t *testing.T) {
	f := newFake()
	r := newRunner(f, 1)
	var seen []guard.Attempt
	stage := func(a guard.Attempt) StageResult { seen = append(seen, a); return failing() }

	if res, err := r.Run(stage); err != nil || res.Outcome != OutcomePunchedOut {
		t.Fatalf("first Run = (%+v, %v)", res, err)
	}
	f.clearLabel(testFailLabel) // human restart
	r.RunID = "run2"
	seen = nil
	if res, err := r.Run(stage); err != nil || res.Outcome != OutcomePunchedOut {
		t.Fatalf("second Run = (%+v, %v)", res, err)
	}
	want := []guard.Attempt{{Current: 1, Max: 2}, {Current: 2, Max: 2}}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("attempts seen after restart = %v, want %v", seen, want)
	}
	if got := f.postedRunIDs(); !reflect.DeepEqual(got, []string{"run1", "run2"}) {
		t.Fatalf("posted run ids = %v", got)
	}
}

func TestRunner_NoAutoRetryAfterTerminal(t *testing.T) {
	f := newFake()
	r := newRunner(f, 0)
	runs := 0
	stage := func(guard.Attempt) StageResult { runs++; return failing() }
	if _, err := r.Run(stage); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		res, err := r.Run(stage)
		if err != nil || res.Outcome != OutcomeHalted {
			t.Fatalf("Run #%d after punch-out = (%+v, %v), want halted", i+2, res, err)
		}
	}
	if runs != 1 {
		t.Fatalf("stage ran %d times, want 1", runs)
	}
	if n := f.count("PostComment"); n != 1 {
		t.Fatalf("comments = %d, want 1", n)
	}
}

func TestRunner_OverwritesStageAttempt(t *testing.T) {
	f := newFake()
	r := newRunner(f, 1)
	res, err := r.Run(func(guard.Attempt) StageResult { return failing() })
	if err != nil {
		t.Fatal(err)
	}
	// The stage reported 99 of 99; the runner counted 2 of 2.
	if res.Failure == nil || res.Failure.Attempt != (guard.Attempt{Current: 2, Max: 2}) {
		t.Fatalf("Failure = %+v, want attempt 2 of 2", res.Failure)
	}
	if !strings.Contains(f.comments[0].Body, "2 of 2") || strings.Contains(f.comments[0].Body, "99") {
		t.Fatalf("comment used the stage's attempt:\n%s", f.comments[0].Body)
	}
}

func TestRunner_InvalidFailureContext(t *testing.T) {
	f := newFake()
	runs := 0
	stage := func(guard.Attempt) StageResult {
		runs++
		fc := testFC()
		fc.Actual = "" // invalid
		return StageResult{Failure: &fc}
	}
	res, err := newRunner(f, 3).Run(stage)
	if !errors.Is(err, guard.ErrInvalid) {
		t.Fatalf("err = %v, want wrapping guard.ErrInvalid", err)
	}
	if res.Outcome == OutcomeSucceeded || res.Outcome == OutcomePunchedOut {
		t.Errorf("Outcome = %v on an invalid context", res.Outcome)
	}
	if runs != 1 {
		t.Errorf("stage ran %d times, want 1", runs)
	}
	if w := f.writes(); len(w) != 0 {
		t.Fatalf("invalid context caused GitHub writes: %v", w)
	}
}

func TestRunner_PunchOutErrorIsPending(t *testing.T) {
	f := newFake()
	f.failNext("AddLabel", 1)
	r := newRunner(f, 0)
	runs := 0
	stage := func(guard.Attempt) StageResult { runs++; return failing() }
	res, err := r.Run(stage)
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
	if res.Outcome != OutcomePunchOutPending {
		t.Fatalf("Outcome = %v, want OutcomePunchOutPending", res.Outcome)
	}
	// The issue is now terminal-pending: no re-entry, and Reconcile finishes it.
	if res2, err := r.Run(stage); err != nil || res2.Outcome != OutcomeHalted || runs != 1 {
		t.Fatalf("Run while pending = (%+v, %v), runs = %d", res2, err, runs)
	}
	if applied, err := Reconcile(f, testTarget); err != nil || !applied {
		t.Fatalf("Reconcile = (%v, %v)", applied, err)
	}
	if n := f.count("PostComment"); n != 1 {
		t.Fatalf("comments = %d, want 1", n)
	}
}

func TestRunner_InvalidSetupDoesNothing(t *testing.T) {
	cases := map[string]Runner{
		"empty run id":      {Target: testTarget, Policy: PolicyFromRetries(1)},
		"zero max attempts": {Target: testTarget, RunID: "run1"},
		"invalid target":    {Policy: PolicyFromRetries(1), RunID: "run1"},
		"nil issues":        {Target: testTarget, Policy: PolicyFromRetries(1), RunID: "run1"},
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			if name != "nil issues" {
				r.Issues = f
			}
			called := false
			_, err := r.Run(func(guard.Attempt) StageResult { called = true; return StageResult{} })
			if !errors.Is(err, guard.ErrInvalid) {
				t.Fatalf("err = %v, want wrapping guard.ErrInvalid", err)
			}
			if called || len(f.calls) != 0 {
				t.Fatalf("stage called = %v, GitHub calls = %v", called, f.calls)
			}
		})
	}
}

func TestRunner_StateErrorDoesNotRunStage(t *testing.T) {
	f := newFake()
	f.failNext("State", 1)
	called := false
	_, err := newRunner(f, 1).Run(func(guard.Attempt) StageResult { called = true; return StageResult{} })
	if !errors.Is(err, errBoom) || called {
		t.Fatalf("err = %v, called = %v; want errBoom and no stage call", err, called)
	}
}

func TestRunner_StageAttemptArgumentCountsFromOne(t *testing.T) {
	f := newFake()
	var seen []guard.Attempt
	_, _ = newRunner(f, 2).Run(func(a guard.Attempt) StageResult { seen = append(seen, a); return failing() })
	want := []guard.Attempt{{Current: 1, Max: 3}, {Current: 2, Max: 3}, {Current: 3, Max: 3}}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("attempts = %v, want %v", seen, want)
	}
}
