package guard

import (
	"fmt"
	"reflect"
	"strings"
)

// Outcome is the overall result of running a chain.
type Outcome int

// Outcomes Run may report.
const (
	OutcomeInvalid       Outcome = iota // zero value; never returned with a nil error
	OutcomeReadyToCommit                // every guard passed
	OutcomeRejected                     // a guard failed and its OnFailure was invoked
)

// Result is what Run reports when the chain ran.
type Result struct {
	Outcome    Outcome
	Failure    *FailureContext // set iff Outcome is OutcomeRejected
	FailureErr error           // what the failing guard's OnFailure returned
}

// ContractError reports a guard that broke the Guard contract. It matches
// ErrInvalid.
type ContractError struct {
	// GuardIndex is the position of the offending guard in the chain.
	GuardIndex int
	Problems   []string
}

func (e *ContractError) Error() string {
	return fmt.Sprintf("%s: guard %d broke the admission contract: %s",
		ErrInvalid.Error(), e.GuardIndex, strings.Join(e.Problems, "; "))
}

// Is makes errors.Is(err, ErrInvalid) true.
func (e *ContractError) Is(target error) bool { return target == ErrInvalid }

// Run applies guards to in, in slice order, and returns what happened.
//
// It calls each guard's Admit once. On the first failure it calls that guard's
// OnFailure exactly once with the FailureContext from its AdmissionResult,
// stops, and returns OutcomeRejected with a nil error. When every guard passes
// it returns OutcomeReadyToCommit and performs no side effect.
//
// The returned error means the chain could not be run (ErrEmptyChain, or
// ErrInvalid for a nil guard or invalid Input) or a guard broke the contract
// (*ContractError). In those cases no OnFailure is called. Result.FailureErr
// is separate: the chain ran, a guard rejected, and its OnFailure returned an
// error.
//
// Run keeps no state, does not retry, does not count attempts and does not
// recover panics from guards.
func Run(in Input, guards []Guard) (Result, error) {
	if len(guards) == 0 {
		return Result{}, ErrEmptyChain
	}
	if err := in.Validate(); err != nil {
		return Result{}, err
	}
	for i, g := range guards {
		if isNil(g) {
			return Result{}, fmt.Errorf("%w: guard %d is nil", ErrInvalid, i)
		}
	}

	for i, g := range guards {
		res := g.Admit(in)
		if p := res.problems(); len(p) > 0 {
			return Result{}, &ContractError{GuardIndex: i, Problems: p}
		}
		if res.Pass {
			continue
		}
		fc := *res.Failure
		failErr := g.OnFailure(fc)
		out := fc // the handler got its own copy; Result reports the original
		return Result{Outcome: OutcomeRejected, Failure: &out, FailureErr: failErr}, nil
	}
	return Result{Outcome: OutcomeReadyToCommit}, nil
}

// isNil reports whether g is a nil interface or an interface holding a nil
// pointer, map, slice, func or channel.
func isNil(g Guard) bool {
	if g == nil {
		return true
	}
	switch v := reflect.ValueOf(g); v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return v.IsNil()
	}
	return false
}

// DefaultChain returns the hard-coded, ordered guard chain for a stage
// boundary. It is the single place the order is written; Run still takes a
// plain slice. The handler is the failure action the guards invoke through
// OnFailure; a nil handler makes OnFailure return ErrNoFailureHandler.
func DefaultChain(handler FailureHandler) []Guard {
	return []Guard{NewStructural(handler)}
}
