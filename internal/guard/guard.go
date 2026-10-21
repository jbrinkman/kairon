package guard

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// Sentinel errors. ErrEmptyChain and *ContractError match ErrInvalid.
var (
	ErrInvalid          = errors.New("guard: invalid")
	ErrEmptyChain       = fmt.Errorf("%w: empty chain", ErrInvalid)
	ErrNoFailureHandler = errors.New("guard: no failure handler")
)

// Attempt is the retry position for a boundary. The boundary supplies it; this
// package never counts attempts.
type Attempt struct {
	Current, Max int
}

// Input is what a guard's Admit method receives. It is a value type with no
// pointer, slice or map fields, so a guard cannot mutate the document or
// anything the runner holds.
type Input struct {
	// Stage is the stage whose output is being admitted. The boundary supplies
	// it, so it is known even when Raw does not parse.
	Stage stageoutput.Stage
	// Raw is the exact text the stage returned: a stage-output document (YAML).
	Raw string
	// Attempt is the boundary's current retry position.
	Attempt Attempt
}

// FailureContext describes why a guard rejected a stage's output.
type FailureContext struct {
	Stage    stageoutput.Stage
	Guard    string  // name of the guard whose admission failed
	Attempt  Attempt // current and max
	Expected string  // what admission required
	Actual   string  // what it observed
}

// AdmissionResult is the outcome of Guard.Admit. Failure is non-nil if and
// only if Pass is false.
type AdmissionResult struct {
	Pass    bool
	Failure *FailureContext
}

// Guard is one deterministic check at a stage boundary.
type Guard interface {
	// Admit judges the stage output. It is pure: no document mutation, no
	// GitHub or shared-state write, no failure action.
	Admit(in Input) AdmissionResult
	// OnFailure receives the FailureContext that Admit produced and enacts this
	// guard's retry-versus-punch-out decision. The runner calls it at most once
	// per run, and only on the guard whose Admit failed.
	OnFailure(fc FailureContext) error
}

// Admitted returns a passing AdmissionResult.
func Admitted() AdmissionResult { return AdmissionResult{Pass: true} }

// Reject returns a failing AdmissionResult whose FailureContext takes Stage and
// Attempt from in, so a guard cannot forget them.
func (in Input) Reject(guard, expected, actual string) AdmissionResult {
	return AdmissionResult{Failure: &FailureContext{
		Stage:    in.Stage,
		Guard:    guard,
		Attempt:  in.Attempt,
		Expected: expected,
		Actual:   actual,
	}}
}

// Valid reports whether a is a usable retry position: 1 <= Current <= Max.
func (a Attempt) Valid() bool { return a.Current >= 1 && a.Max >= a.Current }

// Validate reports whether in is usable. The returned error matches ErrInvalid.
func (in Input) Validate() error {
	return problems(in.problems())
}

func (in Input) problems() []string {
	var p []string
	if !in.Stage.Valid() {
		p = append(p, fmt.Sprintf("stage %q is not a known stage", string(in.Stage)))
	}
	if !in.Attempt.Valid() {
		p = append(p, attemptProblem(in.Attempt))
	}
	return p
}

// Validate reports whether fc is complete. Every field must be set and valid.
// The returned error matches ErrInvalid.
func (fc FailureContext) Validate() error {
	return problems(fc.problems())
}

func (fc FailureContext) problems() []string {
	var p []string
	if !fc.Stage.Valid() {
		p = append(p, fmt.Sprintf("stage %q is not a known stage", string(fc.Stage)))
	}
	if fc.Guard == "" {
		p = append(p, "guard is empty")
	}
	if !fc.Attempt.Valid() {
		p = append(p, attemptProblem(fc.Attempt))
	}
	if fc.Expected == "" {
		p = append(p, "expected is empty")
	}
	if fc.Actual == "" {
		p = append(p, "actual is empty")
	}
	return p
}

// Validate reports whether a obeys the rule that Failure is present if and
// only if Pass is false, and that a present Failure is itself valid. The
// returned error matches ErrInvalid.
func (a AdmissionResult) Validate() error {
	return problems(a.problems())
}

func (a AdmissionResult) problems() []string {
	switch {
	case a.Pass && a.Failure != nil:
		return []string{"pass is true but failure is set"}
	case !a.Pass && a.Failure == nil:
		return []string{"pass is false but failure is nil"}
	case a.Failure != nil:
		return a.Failure.problems()
	}
	return nil
}

func attemptProblem(a Attempt) string {
	return fmt.Sprintf("attempt %d of %d is not 1 <= current <= max", a.Current, a.Max)
}

// problems turns a problem list into an error that matches ErrInvalid, or nil.
func problems(p []string) error {
	if len(p) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(p, "; "))
}

// Document parses in.Raw under the strict stage-output schema. It parses on
// every call and is deliberately not memoized, so each caller gets its own copy
// and there is no hidden shared state.
func (in Input) Document() (*stageoutput.Document, error) {
	return stageoutput.Parse([]byte(in.Raw))
}
