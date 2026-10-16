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

	// A classic case is a case with one turn. The turn plan decides each next
	// user message: the next scripted turn, or a reply from the simulated
	// user's fact bank. Setup context is part of the first turn's prompt only;
	// later turns are sent verbatim.
	simulated := tc.SimulatedUser != nil
	multi := tc.Turns != nil || simulated // these cases record per-turn results
	var plan *turnPlan
	prompt, err := assemblePrompt(tc.Setup, openingMessage(tc))
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

	// A pinned run records, per case, what the case's own agent sees: the
	// provenance is resolved from the staged workspace, so a fixture that
	// supplies a *-conventions skill (or a prompt) is reflected. Unpinned
	// runs record nothing, as before.
	if _, pinned := cfg.pins.pinOf(rubric.Agent); pinned {
		prov, perr := resolveProvenanceInRoot(ws.Dir, rubric.Agent, cfg.candidate)
		if perr != nil {
			_ = ws.Remove()
			fmt.Fprintf(out, " ❌ (workspace failed)\n")
			fmt.Fprintf(out, "      Error: %v\n", perr)
			cr.ActualOutput = ""
			cr.ErrorContext = &ErrorContext{Stderr: fmt.Sprintf("workspace setup failed: resolving provenance: %v", perr)}
			scoreCaseFn(rubric, tc, &cr)
			return cr
		}
		cr.PromptSHA256 = prov.PromptSHA256
		cr.ResourcesPresent = prov.ResourcesPresent
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

	// A simulated case's decision log starts empty, so "no decision yet" is
	// distinguishable from "no log". The harness keeps the decisions in memory
	// and rewrites the file before scoring, because the agent can write .eval/.
	persist := func(ds []SimDecision) error { return rewriteSimLog(ws.Dir, ds) }
	if simulated {
		if err := persist(nil); err != nil {
			fmt.Fprintf(out, " ❌ (workspace failed)\n")
			fmt.Fprintf(out, "      Error: %v\n", err)
			cr.ActualOutput = ""
			cr.ErrorContext = &ErrorContext{Stderr: fmt.Sprintf("workspace setup failed: %v", err)}
			scoreCaseFn(rubric, tc, &cr)
			return cr
		}
	}
	plan = newTurnPlan(tc, nil, persist)
	if simulated {
		plan.route = newRouter(tc, &cr)
	}

	// score scores the case. A simulated case's log is first rewritten from
	// memory, and the judge sees the conversation that actually happened.
	score := func() {
		view := tc
		if simulated {
			if err := persist(plan.decisions); err != nil && cr.ErrorContext == nil {
				cr.ActualOutput = ""
				cr.ErrorContext = &ErrorContext{Stderr: err.Error()}
			}
			if len(cr.UserTurns) > 0 {
				view.Turns = cr.UserTurns
			}
		}
		scoreCaseFn(rubric, view, &cr)
	}

	// A multi-turn sandbox case keeps one container for all its turns, so the
	// agent's conversation state under the container's $HOME survives between
	// turns. One-turn and classic cases keep the one-shot container path.
	var session containerSession
	failed := false
	if cConfig != nil && (simulated || len(tc.userTurns()) > 1) {
		session = &lazyContainerSession{cConfig: cConfig, ws: ws}
		defer func() { session.Close(failed) }()
	}

	// Every turn runs against the same workspace; the backend owns
	// conversation continuity (Request.Turn > 0 continues it).
	userMsg := plan.first()
	for k := 1; ; k++ {
		if simulated {
			cr.UserTurns = append(cr.UserTurns, userMsg)
		}
		if k > 1 {
			prompt = userMsg
		}
		switch {
		case simulated:
			fmt.Fprintf(out, " → turn %d...", k)
		case multi:
			fmt.Fprintf(out, " → turn %d/%d...", k, plan.total())
		default:
			fmt.Fprintf(out, " → running agent...")
		}
		actualOutput, cost, rec, errorContext, err := invokeAgent(rubric.Agent, prompt, cConfig,
			callOpts{Stub: tc.Stub, Workspace: ws, Timeout: caseTimeout(tc), Turn: k, Session: session})
		if multi {
			rec.Turn = k
		}
		if cr.PromptSHA256 != "" {
			// The call record must not disagree with the case it belongs to.
			rec.PromptSHA256 = cr.PromptSHA256
		}
		cr.Calls = append(cr.Calls, rec)
		cr.AgentCost.Add(cost)
		if err != nil {
			failed = true
			fmt.Fprintf(out, " ❌ (agent failed)\n")
			switch {
			case simulated:
				fmt.Fprintf(out, "      Error: turn %d: %v\n", k, err)
			case multi:
				fmt.Fprintf(out, "      Error: turn %d/%d: %v\n", k, plan.total(), err)
			default:
				fmt.Fprintf(out, "      Error: %v\n", err)
			}
			cr.ActualOutput = ""
			cr.ErrorContext = errorContext
			score()
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

		next, more, nerr := plan.next(k, actualOutput)
		if nerr != nil {
			// The simulated user could not decide (routing failed). Fail
			// closed: no default_reply, nothing more is sent.
			failed = true
			msg := fmt.Sprintf("turn %d: %v", k, nerr)
			fmt.Fprintf(out, " ❌ (simulated user failed)\n")
			fmt.Fprintf(out, "      Error: %s\n", msg)
			cr.ActualOutput = ""
			cr.ErrorContext = &ErrorContext{Stderr: msg}
			score()
			return cr
		}
		if !more {
			break
		}
		userMsg = next
	}
	fmt.Fprintf(out, " → evaluating...")

	// Close the container before scoring reads the final workspace, so a
	// detaching backend can't keep changing scored files while final-workspace
	// checks (file_*, changed_files, command, default output_*/gh_log_*) run.
	// The deferred Close above remains as a fallback for the early-return
	// failure paths; a second Close on an already-closed session is a no-op.
	if session != nil {
		session.Close(failed)
	}

	score()
	return cr
}

// openingMessage is the first user message of a case.
func openingMessage(tc TestCase) string {
	if tc.SimulatedUser != nil {
		return tc.Input
	}
	return tc.userTurns()[0]
}
