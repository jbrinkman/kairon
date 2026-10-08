package eval

import (
	"fmt"
	"io"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// callOpts carries the per-call inputs of invokeAgent that are not part of
// the agent name, the prompt or the container configuration.
type callOpts struct {
	// Stub is the case's scripted response, used only by the stub backend.
	Stub *inference.StubScript
	// Workspace is the case's host-side workspace. Natively it becomes the
	// agent's working directory. Nil runs the agent without a workspace (the
	// process working directory), as direct callers such as the performance
	// investigation do.
	Workspace *caseWorkspace
	// Timeout is the case's own timeout. It takes precedence over the
	// environment/default (native) and the sandbox resource limit
	// (container). Zero leaves the existing precedence unchanged.
	Timeout time.Duration
}

// scoreCaseFn is the scoring step of executeCase; tests replace it to observe
// the workspace while a case is scored.
var scoreCaseFn = scoreCase

// caseTimeout returns the case's own timeout, or 0 when it has none. The
// value is validated by loadCases; an unparsable value (a hand-built case)
// is treated as unset.
func caseTimeout(tc TestCase) time.Duration {
	if tc.Timeout == "" {
		return 0
	}
	d, err := time.ParseDuration(tc.Timeout)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// executeCase is the one per-case code path shared by evaluate,
// evaluateProgressive (and through them the single-case and resume paths):
// workspace -> invoke -> score (workspace still present) -> keep or remove.
//
// The order is load-bearing: CaseResult.WorkspaceDir is set before scoring
// and the workspace is only removed after scoring, so checks that read the
// workspace see it. WorkspaceDir is recorded whether or not the directory is
// later removed.
func executeCase(rubric Rubric, tc TestCase, cConfig *ContainerConfig, out io.Writer, keep bool) CaseResult {
	cr := CaseResult{CaseName: tc.Name}

	prompt, err := assemblePrompt(tc.Setup, tc.Input)
	if err != nil {
		fmt.Fprintf(out, " ❌ (prompt error)\n")
		fmt.Fprintf(out, "      Error: %v\n", err)
		cr.ActualOutput = ""
		scoreCaseFn(rubric, tc, &cr)
		return cr
	}

	ws, wsErr := newCaseWorkspace(tc)
	if wsErr != nil {
		fmt.Fprintf(out, " ❌ (workspace failed)\n")
		fmt.Fprintf(out, "      Error: %v\n", wsErr)
		cr.ActualOutput = ""
		cr.ErrorContext = &ErrorContext{Stderr: fmt.Sprintf("workspace setup failed: %v", wsErr)}
		scoreCaseFn(rubric, tc, &cr)
		return cr
	}
	cr.WorkspaceDir = ws.Dir
	if cConfig != nil {
		// Sandbox run: the agent sees the workspace at the container path, so
		// absolute references it reports are rewritten to the host dir for scoring.
		cr.containerWorkspaceDir = cConfig.WorkspaceDir
	}
	if keep {
		fmt.Fprintf(out, " (workspace: %s)", ws.Dir)
	} else {
		defer ws.Remove() // after scoring; Remove never fails the run
	}

	fmt.Fprintf(out, " → running agent...")
	actualOutput, cost, rec, errorContext, err := invokeAgent(rubric.Agent, prompt, cConfig,
		callOpts{Stub: tc.Stub, Workspace: ws, Timeout: caseTimeout(tc)})
	cr.Calls = append(cr.Calls, rec)
	if err != nil {
		fmt.Fprintf(out, " ❌ (agent failed)\n")
		fmt.Fprintf(out, "      Error: %v\n", err)
		cr.ActualOutput = ""
		cr.ErrorContext = errorContext
	} else {
		cr.ActualOutput = actualOutput
		cr.AgentCost = cost
		cr.ErrorContext = errorContext
		fmt.Fprintf(out, " → evaluating...")
	}

	scoreCaseFn(rubric, tc, &cr)
	return cr
}
