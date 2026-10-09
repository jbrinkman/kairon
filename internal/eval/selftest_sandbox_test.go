package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

// sandboxSelftestEnv gates TestSelftestSandbox. The sandbox run builds a
// container image and starts containers, so it must never happen as a side
// effect of a plain `go test ./...`. `task eval:selftest:sandbox` sets it.
const sandboxSelftestEnv = "KAIRON_EVAL_SANDBOX_SELFTEST"

// repoRoot returns the kairon module root (the directory holding go.mod),
// found by walking up from the package directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the working directory")
		}
		dir = parent
	}
}

// runSelftestInto runs the selftest agent with the stub backend (optionally in
// the container sandbox) and returns the parsed selftest.json it wrote.
func runSelftestInto(t *testing.T, evalsDir string, useSandbox bool) AgentResult {
	t.Helper()
	resultsDir := filepath.Join(evalsDir, "results")
	registerResultsCleanup(t, resultsDir)
	before := snapshotDirs(t, resultsDir)

	err := RunWithOptions("selftest", "", RunOptions{
		Backend:   "stub",
		EvalsDir:  evalsDir,
		Sandbox:   useSandbox,
		NoSandbox: !useSandbox,
	})
	if err != nil {
		t.Fatalf("selftest run (sandbox=%v) failed: %v", useSandbox, err)
	}
	added := newRunDirs(t, resultsDir, before)
	if len(added) != 1 {
		t.Fatalf("selftest run (sandbox=%v) created %d result dirs, want 1: %v", useSandbox, len(added), added)
	}
	return readSelfTestResult(t, filepath.Join(resultsDir, added[0], "selftest.json"))
}

// TestSelftestSandbox runs the self-test natively and again under
// `--sandbox --backend stub` and requires identical agent output, agent cost
// and recorded call model. It is gated: it only runs with
// KAIRON_EVAL_SANDBOX_SELFTEST=1 (see `task eval:selftest:sandbox`) and skips,
// without starting anything, when no Podman or Docker daemon is reachable.
func TestSelftestSandbox(t *testing.T) {
	if os.Getenv(sandboxSelftestEnv) != "1" {
		t.Skipf("sandbox self-test is opt-in: set %s=1 or run `task eval:selftest:sandbox` (needs Podman or Docker)", sandboxSelftestEnv)
	}
	if err := sandbox.EnsureContainerDaemon(); err != nil {
		t.Skipf("no container daemon reachable (tried Podman and Docker); start Podman or Docker to run the sandbox self-test: %v", err)
	}

	t.Cleanup(resetConfig)
	t.Chdir(repoRoot(t))
	const evalsDir = "internal/eval/testdata/evals"

	native := runSelftestInto(t, evalsDir, false)
	sandboxed := runSelftestInto(t, evalsDir, true)

	index := func(res AgentResult) map[string]CaseResult {
		m := map[string]CaseResult{}
		for _, c := range res.Cases {
			m[c.CaseName] = c
		}
		return m
	}
	nativeCases, sandboxCases := index(native), index(sandboxed)

	var names []string
	for name := range nativeCases {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("native selftest produced no cases")
	}
	if len(sandboxCases) != len(nativeCases) {
		t.Fatalf("sandbox run has %d cases, native has %d", len(sandboxCases), len(nativeCases))
	}

	for _, name := range names {
		n := nativeCases[name]
		s, ok := sandboxCases[name]
		if !ok {
			t.Errorf("case %q missing from the sandbox run", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			if s.ErrorContext != nil {
				t.Fatalf("sandbox case has ErrorContext: %+v", s.ErrorContext)
			}
			if s.ActualOutput == "" {
				t.Fatal("sandbox actual_output is empty")
			}
			if s.ActualOutput != n.ActualOutput {
				t.Errorf("actual_output differs\n native: %q\nsandbox: %q", n.ActualOutput, s.ActualOutput)
			}
			if s.AgentCost != n.AgentCost {
				t.Errorf("agent_cost differs\n native: %+v\nsandbox: %+v", n.AgentCost, s.AgentCost)
			}
			if got, want := agentCallModel(s), agentCallModel(n); got != want {
				t.Errorf("agent call model = %q, want %q (native)", got, want)
			}
		})
	}

	// The multi-turn cases run in one container per case under --sandbox:
	// every turn's output, the call records and the check results must match
	// the native run.
	for _, name := range []string{multiTurnGHLogCase, multiTurnWorkspaceCase} {
		n, s := nativeCases[name], sandboxCases[name]
		if len(n.TurnOutputs) == 0 {
			t.Errorf("native %s has no turn_outputs", name)
		}
		if strings.Join(n.TurnOutputs, "\x00") != strings.Join(s.TurnOutputs, "\x00") {
			t.Errorf("%s turn_outputs differ\n native: %q\nsandbox: %q", name, n.TurnOutputs, s.TurnOutputs)
		}
		var nTurns, sTurns []int
		for _, c := range n.Calls {
			if c.Role == "agent" {
				nTurns = append(nTurns, c.Turn)
			}
		}
		for _, c := range s.Calls {
			if c.Role == "agent" {
				sTurns = append(sTurns, c.Turn)
			}
		}
		if fmt.Sprint(nTurns) != fmt.Sprint(sTurns) {
			t.Errorf("%s agent call turns differ: native %v, sandbox %v", name, nTurns, sTurns)
		}
		if len(n.Scores) != len(s.Scores) {
			t.Fatalf("%s has %d native scores, %d sandbox scores", name, len(n.Scores), len(s.Scores))
		}
		for i := range n.Scores {
			if n.Scores[i].Score != s.Scores[i].Score || n.Scores[i].MaxScore != s.Scores[i].MaxScore {
				t.Errorf("%s criterion %s: native %d/%d, sandbox %d/%d", name, n.Scores[i].Name,
					n.Scores[i].Score, n.Scores[i].MaxScore, s.Scores[i].Score, s.Scores[i].MaxScore)
			}
		}
	}

	// The sandbox run must satisfy the same expectations as the native one.
	assertSelfTestResults(t, sandboxed)
}

// agentCallModel returns the model recorded on the case's agent call.
func agentCallModel(c CaseResult) string {
	for _, call := range c.Calls {
		if call.Role == "agent" {
			return call.Model
		}
	}
	return "<no agent call>"
}
