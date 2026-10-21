// Package guard defines the deterministic checks that sit between pipeline
// stages and the ordered runner that applies them.
//
// A guard has exactly two methods: Admit, a pure pass/fail judgement of a
// stage's output, and OnFailure, the guard's own retry-or-punch-out action.
// Admit returns only an AdmissionResult. When it fails, the result carries a
// FailureContext that the runner hands to the same guard's OnFailure.
//
// Run applies a slice of guards in order, stops at the first failure and
// routes that failure to the owning guard exactly once. The runner is a
// stateless function with no side effect of its own; committing the output
// and counting attempts belong to the caller.
//
// Guards are deterministic Go and never spawn an agent. To keep that
// checkable, this package imports only the standard library and
// internal/stageoutput, and does not directly import os, os/exec, net or
// net/http (enforced by TestNoForbiddenImports).
package guard
