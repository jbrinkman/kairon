package failure

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

func TestRoute_RecoverableRecordsContext(t *testing.T) {
	var r Route
	fc := testFC()
	if err := r.Recoverable()(fc); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got := r.Result()
	if got.Failure == nil || *got.Failure != fc {
		t.Fatalf("Failure = %+v, want %+v", got.Failure, fc)
	}
	if got.Disposition != Recoverable {
		t.Errorf("Disposition = %v, want Recoverable", got.Disposition)
	}
}

func TestRoute_TerminalRecordsDispositionAndRemedy(t *testing.T) {
	var r Route
	fc := testFC()
	if err := r.Terminal("do the thing")(fc); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got := r.Result()
	if got.Failure == nil || *got.Failure != fc {
		t.Fatalf("Failure = %+v, want %+v", got.Failure, fc)
	}
	if got.Disposition != Unrecoverable {
		t.Errorf("Disposition = %v, want Unrecoverable", got.Disposition)
	}
	if got.Remedy != "do the thing" {
		t.Errorf("Remedy = %q", got.Remedy)
	}
}

func TestRoute_ResultIsSuccessWhenNoFailureRecorded(t *testing.T) {
	var r Route
	if got := r.Result(); got.Failure != nil {
		t.Fatalf("Failure = %+v, want nil", got.Failure)
	}
}

func TestRoute_LastHandlerCallWins(t *testing.T) {
	var r Route
	fc := testFC()
	_ = r.Terminal("x")(fc)
	_ = r.Recoverable()(fc)
	got := r.Result()
	if got.Disposition != Recoverable || got.Remedy != "" {
		t.Fatalf("Result = %+v, want Recoverable with empty remedy", got)
	}
}

func TestRoute_ResultDoesNotAliasRecordedContext(t *testing.T) {
	var r Route
	fc := testFC()
	_ = r.Recoverable()(fc)
	fc.Guard = "mutated"
	if got := r.Result(); got.Failure.Guard != "structural" {
		t.Fatalf("recorded context changed with the caller's copy: %+v", got.Failure)
	}
}

// A real guard.Run, with the structural guard's failure method routed to
// Route.Terminal, lands in the mechanism: one stage run, then a punch-out.
func TestRoute_GuardRunRoutesIntoMechanism(t *testing.T) {
	f := newFake()
	runs := 0
	stage := func(a guard.Attempt) StageResult {
		runs++
		var route Route
		g := guard.NewStructural(route.Terminal("rewrite the issue body"))
		in := guard.Input{Stage: stageoutput.StagePlanner, Raw: "this is not a stage-output document", Attempt: a}
		res, err := guard.Run(in, []guard.Guard{g})
		if err != nil {
			t.Fatalf("guard.Run: %v", err)
		}
		if res.Outcome != guard.OutcomeRejected {
			t.Fatalf("outcome = %v, want rejected", res.Outcome)
		}
		return route.Result()
	}
	r := Runner{Issues: f, Target: testTarget, Policy: PolicyFromRetries(3), RunID: "run1"}
	res, err := r.Run(stage)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runs != 1 || res.Outcome != OutcomePunchedOut {
		t.Fatalf("runs = %d, outcome = %v; want 1 run and OutcomePunchedOut", runs, res.Outcome)
	}
	if f.count("PostComment") != 1 || f.count("AddLabel "+testFailLabel) != 1 {
		t.Fatalf("calls = %v", f.calls)
	}
	if res.Failure == nil || res.Failure.Guard != guard.StructuralName {
		t.Fatalf("Failure = %+v", res.Failure)
	}
}
