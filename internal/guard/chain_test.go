package guard

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

func TestRun_StopsAtFirstFailure(t *testing.T) {
	var log []string
	g1 := &fakeGuard{name: "one", log: &log}
	g2 := &fakeGuard{name: "two", reject: true, log: &log}
	g3 := &fakeGuard{name: "three", log: &log}

	res, err := Run(validInput(), []Guard{g1, g2, g3})
	require.NoError(t, err)

	assert.Equal(t, OutcomeRejected, res.Outcome)
	assert.Equal(t, 1, g1.admitCalls)
	assert.Equal(t, 1, g2.admitCalls)
	assert.Equal(t, 0, g3.admitCalls, "later guards must not run after a failure")
	assert.Equal(t, []string{"admit:one", "admit:two", "fail:two"}, log)
}

func TestRun_AllPassReadyToCommit(t *testing.T) {
	var log []string
	gs := []*fakeGuard{
		{name: "one", log: &log},
		{name: "two", log: &log},
		{name: "three", log: &log},
	}

	res, err := Run(validInput(), []Guard{gs[0], gs[1], gs[2]})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReadyToCommit, res.Outcome)
	assert.Nil(t, res.Failure)
	assert.NoError(t, res.FailureErr)
	for _, g := range gs {
		assert.Equal(t, 1, g.admitCalls, g.name)
		assert.Equal(t, 0, g.failCalls, g.name)
	}
	assert.Equal(t, []string{"admit:one", "admit:two", "admit:three"}, log,
		"the runner must add no log entries of its own")
}

func TestRun_FailureRoutedToOwningGuardOnce(t *testing.T) {
	g1 := &fakeGuard{name: "one"}
	g2 := &fakeGuard{name: "two", reject: true}
	g3 := &fakeGuard{name: "three"}
	in := validInput()

	res, err := Run(in, []Guard{g1, g2, g3})
	require.NoError(t, err)

	want := in.Reject("two", "expected-two", "actual-two").Failure
	require.NotNil(t, want)

	require.Equal(t, 1, g2.failCalls, "the failing guard's OnFailure must run exactly once")
	assert.Equal(t, *want, g2.failCtxs[0])
	assert.Equal(t, 0, g1.failCalls)
	assert.Equal(t, 0, g3.failCalls)
	assert.Equal(t, 0, g3.admitCalls)

	assert.Equal(t, OutcomeRejected, res.Outcome)
	require.NotNil(t, res.Failure)
	assert.Equal(t, *want, *res.Failure)
	assert.NoError(t, res.FailureErr)
}

func TestRun_ResultFailureIsACopy(t *testing.T) {
	shared := validInput().Reject("g", "e", "a")
	g := &fakeGuard{name: "g", result: &shared}

	res, err := Run(validInput(), []Guard{g})
	require.NoError(t, err)
	require.NotNil(t, res.Failure)
	assert.NotSame(t, shared.Failure, res.Failure, "Result must not alias the guard's FailureContext")
}

func TestRun_FailureErrPassedThrough(t *testing.T) {
	boom := errors.New("boom")
	g := &fakeGuard{name: "g", reject: true, failureErr: boom}

	res, err := Run(validInput(), []Guard{g})
	require.NoError(t, err)
	assert.Equal(t, OutcomeRejected, res.Outcome)
	assert.Same(t, boom, res.FailureErr)
	assert.Equal(t, 1, g.failCalls)
}

func TestRun_PassesInputToEveryGuard(t *testing.T) {
	g1 := &fakeGuard{name: "one"}
	g2 := &fakeGuard{name: "two"}
	in := validInput()

	_, err := Run(in, []Guard{g1, g2})
	require.NoError(t, err)
	assert.Equal(t, []Input{in}, g1.admitted)
	assert.Equal(t, []Input{in}, g2.admitted)
}

func TestRun_RejectsBadChain(t *testing.T) {
	var log []string
	good := &fakeGuard{name: "good", log: &log}

	t.Run("nil slice", func(t *testing.T) {
		res, err := Run(validInput(), nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyChain)
		assert.ErrorIs(t, err, ErrInvalid)
		assert.Equal(t, Result{}, res)
	})
	t.Run("empty slice", func(t *testing.T) {
		_, err := Run(validInput(), []Guard{})
		assert.ErrorIs(t, err, ErrEmptyChain)
		assert.ErrorIs(t, err, ErrInvalid)
	})
	t.Run("nil element", func(t *testing.T) {
		_, err := Run(validInput(), []Guard{good, nil})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalid)
		assert.NotErrorIs(t, err, ErrEmptyChain)
		assert.Contains(t, err.Error(), "1", "error should name the index")
	})
	t.Run("nil pointer element", func(t *testing.T) {
		var typedNil *fakeGuard
		_, err := Run(validInput(), []Guard{typedNil})
		assert.ErrorIs(t, err, ErrInvalid)
	})

	bad := []struct {
		name string
		in   Input
	}{
		{"unknown stage", Input{Stage: "nope", Attempt: Attempt{1, 3}}},
		{"empty stage", Input{Attempt: Attempt{1, 3}}},
		{"attempt 0 of 3", Input{Stage: stageoutput.StagePlanner, Attempt: Attempt{0, 3}}},
		{"attempt 4 of 3", Input{Stage: stageoutput.StagePlanner, Attempt: Attempt{4, 3}}},
	}
	for _, tt := range bad {
		t.Run("invalid input "+tt.name, func(t *testing.T) {
			res, err := Run(tt.in, []Guard{good})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalid)
			assert.Equal(t, Result{}, res)
		})
	}

	assert.Equal(t, 0, good.admitCalls, "no guard may run when the chain or input is rejected")
	assert.Equal(t, 0, good.failCalls)
	assert.Empty(t, log)
}

func TestRun_ContractViolation(t *testing.T) {
	valid := validInput().Reject("bad", "e", "a")
	require.NotNil(t, valid.Failure)
	noExpected := valid
	fcNoExpected := *valid.Failure
	fcNoExpected.Expected = ""
	noExpected.Failure = &fcNoExpected

	tests := []struct {
		name string
		res  AdmissionResult
	}{
		{"fail with nil failure", AdmissionResult{Pass: false}},
		{"pass with failure", AdmissionResult{Pass: true, Failure: valid.Failure}},
		{"failure with empty expected", noExpected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := &fakeGuard{name: "first"}
			broken := &fakeGuard{name: "bad", result: &tt.res}
			later := &fakeGuard{name: "later"}

			res, err := Run(validInput(), []Guard{first, broken, later})
			require.Error(t, err)
			assert.Equal(t, Result{}, res)
			assert.ErrorIs(t, err, ErrInvalid)

			var ce *ContractError
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, 1, ce.GuardIndex)
			assert.NotEmpty(t, ce.Problems)
			assert.NotEmpty(t, ce.Error())

			assert.Equal(t, 1, broken.admitCalls)
			assert.Equal(t, 0, first.failCalls)
			assert.Equal(t, 0, broken.failCalls, "no OnFailure on a contract violation")
			assert.Equal(t, 0, later.admitCalls, "no later guard after a contract violation")
			assert.Equal(t, 0, later.failCalls)
		})
	}
}

func TestDefaultChain_Order(t *testing.T) {
	chain := DefaultChain(nil)
	require.Len(t, chain, 1)
	assert.IsType(t, &Structural{}, chain[0])
}
