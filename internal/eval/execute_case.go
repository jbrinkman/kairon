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
	// Turn is the 1-based number of the user turn being sent (the numbering of
	// case YAML and results). It becomes the 0-based inference.Request.Turn.
	// Zero means the first turn.
	Turn int
	// Session, when set together with a container config, runs the call in
	// the case's open container session instead of a one-shot container.
	Session containerSession
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

	// Fail fast, before any workspace or agent call, for cases that are only
	// safe inside the sandbox.
	if tc.RequiresSandbox && cConfig == nil {
		fmt.Fprintf(out, " ❌ (requires --sandbox)\n")
		fmt.Fprintf(out, "      Error: case %q requires --sandbox\n", tc.Name)
		cr.ActualOutput = ""
		cr.ErrorContext = &ErrorContext{Stderr: fmt.Sprintf("case %q requires --sandbox", tc.Name)}
		scoreCaseFn(rubric, tc, &cr)
		return cr
	}

	// A classic case is a case with one turn. Setup context is part of the
	// first turn's prompt only; later turns are sent verbatim.
	turns := tc.userTurns()
	multi := tc.Turns != nil // only turns cases record per-turn results
	prompt, err := assemblePrompt(tc.Setup, turns[0])
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
	cr.baseCommit = ws.BaseCommit
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

	// A multi-turn sandbox case keeps one container for all its turns, so the
	// agent's conversation state under the container's $HOME survives between
	// turns. One-turn and classic cases keep the one-shot container path.
	var session containerSession
	failed := false
	if cConfig != nil && len(turns) > 1 {
		session = &lazyContainerSession{cConfig: cConfig, ws: ws}
		defer func() { session.Close(failed) }()
	}

	// Every turn runs against the same workspace; the backend owns
	// conversation continuity (Request.Turn > 0 continues it).
	for i, userTurn := range turns {
		k := i + 1
		if i > 0 {
			prompt = userTurn
		}
		if multi {
			fmt.Fprintf(out, " → turn %d/%d...", k, len(turns))
		} else {
			fmt.Fprintf(out, " → running agent...")
		}
		actualOutput, cost, rec, errorContext, err := invokeAgent(rubric.Agent, prompt, cConfig,
			callOpts{Stub: tc.Stub, Workspace: ws, Timeout: caseTimeout(tc), Turn: k, Session: session})
		if multi {
			rec.Turn = k
		}
		cr.Calls = append(cr.Calls, rec)
		cr.AgentCost.Add(cost)
		if err != nil {
			failed = true
			fmt.Fprintf(out, " ❌ (agent failed)\n")
			if multi {
				fmt.Fprintf(out, "      Error: turn %d/%d: %v\n", k, len(turns), err)
			} else {
				fmt.Fprintf(out, "      Error: %v\n", err)
			}
			cr.ActualOutput = ""
			cr.ErrorContext = errorContext
			scoreCaseFn(rubric, tc, &cr)
			return cr
		}
		cr.ActualOutput = actualOutput
		cr.ErrorContext = errorContext
		if multi {
			// Snapshot the cumulative gh log before the next turn can change it.
			cr.TurnOutputs = append(cr.TurnOutputs, actualOutput)
			ghLog, oversized := readGHLog(ws.Dir)
			cr.turnGHLogs = append(cr.turnGHLogs, ghLogSnapshot{Content: ghLog, Oversized: oversized})
		}
	}
	fmt.Fprintf(out, " → evaluating...")

	scoreCaseFn(rubric, tc, &cr)
	return cr
}
