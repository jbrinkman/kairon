package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

// installFakeKiroCLI puts a fake kiro-cli first on PATH. Every invocation
// appends its argv to <dir>/calls.log; stdout/stderr/exit code come from the
// script body. It returns the directory and the calls log path.
func installFakeKiroCLI(t *testing.T, body string) (dir, callsLog string) {
	t.Helper()
	dir = t.TempDir()
	callsLog = filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$@\" >> " + callsLog + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kiro-cli"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir, callsLog
}

func readCalls(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func useStubBackend(t *testing.T) {
	t.Helper()
	if err := configure(RunOptions{Backend: "stub"}); err != nil {
		t.Fatal(err)
	}
}

func TestInvokeAgentDefaultBackendCommandLine(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "cat >/dev/null\nprintf '\\033[1mhello\\033[0m'")

	out, cost, ec, err := invokeAgent("builder", "prompt text here", nil, nil)
	if err != nil {
		t.Fatalf("invokeAgent: %v", err)
	}
	if out != "hello" {
		t.Errorf("output = %q, want ANSI-stripped %q", out, "hello")
	}
	if ec != nil {
		t.Errorf("ErrorContext = %+v, want nil on clean success", ec)
	}
	if cost.UsageSource != "estimated" || cost.TokensIn != len("prompt text here")/4 {
		t.Errorf("cost = %+v", cost)
	}
	got := readCalls(t, calls)
	want := "chat --agent builder --no-interactive --trust-all-tools"
	if len(got) != 1 || got[0] != want {
		t.Errorf("calls = %q, want [%q]", got, want)
	}
}

func TestInvokeAgentDefaultBackendErrorContext(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "cat >/dev/null\necho boom >&2\nexit 3")

	_, _, ec, err := invokeAgent("builder", "p", nil, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "kiro-cli invocation failed: ") {
		t.Fatalf("err = %v, want kiro-cli invocation failed prefix", err)
	}
	if ec == nil {
		t.Fatal("ErrorContext is nil")
	}
	if ec.Command != "kiro-cli chat --agent builder --no-interactive --trust-all-tools" {
		t.Errorf("Command = %q", ec.Command)
	}
	if ec.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", ec.ExitCode)
	}
	if strings.TrimSpace(ec.Stderr) != "boom" {
		t.Errorf("Stderr = %q", ec.Stderr)
	}
	if wd, _ := os.Getwd(); ec.WorkingDir != wd {
		t.Errorf("WorkingDir = %q, want %q", ec.WorkingDir, wd)
	}
}

func TestInvokeAgentDefaultBackendTimeout(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "sleep 5")
	t.Setenv("KAIRON_EVAL_TIMEOUT", "200ms")

	_, _, ec, err := invokeAgent("builder", "p", nil, nil)
	if err == nil || err.Error() != "kiro-cli timeout after 200ms" {
		t.Fatalf("err = %v, want %q", err, "kiro-cli timeout after 200ms")
	}
	if ec == nil {
		t.Fatal("ErrorContext is nil")
	}
	if !strings.HasPrefix(ec.Stderr, "timeout after 200ms\n") {
		t.Errorf("Stderr = %q, want timeout prefix", ec.Stderr)
	}
	if ec.Environment["KAIRON_EVAL_TIMEOUT"] != "200ms" {
		t.Errorf("Environment = %v", ec.Environment)
	}
}

func TestInvokeAgentStderrOnSuccessKeepsErrorContext(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "cat >/dev/null\necho warn >&2\necho ok")

	out, _, ec, err := invokeAgent("builder", "p", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Errorf("out = %q", out)
	}
	if ec == nil || strings.TrimSpace(ec.Stderr) != "warn" || ec.ExitCode != 0 {
		t.Errorf("ErrorContext = %+v", ec)
	}
}

func TestScoreLLMJudgeDefaultBackend(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, `cat >/dev/null
printf 'noise ===JSON_START===\n{"score": 4, "reasoning": "fine", "pass": true}\n===JSON_END==='`)

	cost, score, reasoning, skipped := scoreLLMJudge(
		Criterion{Name: "clarity", Description: "d"}, TestCase{Input: "in"}, "actual")
	if skipped || score != 4 || reasoning != "fine" {
		t.Fatalf("got score=%d reasoning=%q skipped=%v", score, reasoning, skipped)
	}
	if cost.UsageSource != "estimated" || cost.TokensIn == 0 {
		t.Errorf("cost = %+v", cost)
	}
	got := readCalls(t, calls)
	if len(got) != 1 || got[0] != "chat --no-interactive" {
		t.Errorf("calls = %q", got)
	}
}

func TestScoreLLMJudgeFailureWording(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "cat >/dev/null\nexit 2")

	_, score, reasoning, skipped := scoreLLMJudge(Criterion{Name: "c"}, TestCase{}, "actual")
	if !skipped || score != 0 {
		t.Errorf("score=%d skipped=%v", score, skipped)
	}
	if reasoning != "kiro-cli chat failed: exit status 2" {
		t.Errorf("reasoning = %q", reasoning)
	}
}

func TestStubBackendNeverStartsKiroCLI(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	stub := &inference.StubScript{Turns: []inference.StubTurn{{
		Response: "## A\n### B",
		Model:    "stub-model",
		Usage:    &inference.StubUsage{InputTokens: 123, OutputTokens: 45},
	}}}

	out, cost, ec, err := invokeAgent("selftest", "prompt", nil, stub)
	if err != nil || ec != nil {
		t.Fatalf("err=%v ec=%+v", err, ec)
	}
	if out != "## A\n### B" {
		t.Errorf("out = %q", out)
	}
	if cost.TokensIn != 123 || cost.TokensOut != 45 || cost.UsageSource != "reported" || cost.Model != "stub-model" {
		t.Errorf("cost = %+v", cost)
	}

	jc, score, _, skipped := scoreLLMJudge(Criterion{Name: "clarity"}, TestCase{}, out)
	if skipped || score != 5 {
		t.Errorf("judge score=%d skipped=%v", score, skipped)
	}
	if jc.Model != "stub" || jc.UsageSource != "estimated" {
		t.Errorf("judge cost = %+v", jc)
	}

	StartProfiling() // clear any cached startup measurement
	if d := MeasureStartupOverhead(); d != 0 {
		t.Errorf("stub startup overhead = %v, want 0", d)
	}

	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli was invoked: %q", got)
	}
}

func TestStubAgentMissingScriptIsError(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	_, _, ec, err := invokeAgent("a", "p", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "stub.turns[0].response") {
		t.Fatalf("err = %v", err)
	}
	if ec == nil {
		t.Error("expected ErrorContext on error")
	}
}

func TestMeasureStartupOverheadUsesBackendProbe(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)
	StartProfiling()

	if d := MeasureStartupOverhead(); d != 0 {
		t.Errorf("overhead = %v, want 0", d)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("process started: %q", got)
	}

	// Default backend still probes `kiro-cli --version`.
	resetConfig()
	StartProfiling()
	MeasureStartupOverhead()
	got := readCalls(t, calls)
	if len(got) != 1 || got[0] != "--version" {
		t.Errorf("calls = %q, want [--version]", got)
	}
}

func TestRunStubBackendSkipsPathCheck(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	t.Setenv("PATH", t.TempDir()) // no kiro-cli anywhere

	// Rubrics dir missing: Run should fail on that, not on kiro-cli lookup.
	err := Run("x", nil)
	if err == nil || !strings.Contains(err.Error(), "rubrics directory not found") {
		t.Fatalf("err = %v", err)
	}

	// Default backend consults PATH and reports the historical message.
	resetConfig()
	writeCfgFile(t, filepath.Join(".kairon", "evals", "rubrics", "x.yaml"), "agent: x\ncriteria: []\n")
	err = Run("x", nil)
	if err == nil || !strings.Contains(err.Error(), "❌ Fatal: kiro-cli unavailable:") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunWithOptionsRejectsStubWithSandbox(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")

	err := RunWithOptions("x", "", RunOptions{Backend: "stub", Sandbox: true})
	if err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("err = %v, want sandbox rejection", err)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("work was done before rejection: %q", got)
	}
}

func TestRunWithOptionsPerfRoutesToInvestigation(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	// Empty agent is rejected by RunPerformanceInvestigation itself, which
	// proves Perf is dispatched from RunWithOptions.
	err := RunWithOptions("", "", RunOptions{Backend: "stub", Perf: true})
	if err == nil || !strings.Contains(err.Error(), "agent name required for performance investigation") {
		t.Fatalf("err = %v", err)
	}
}

func TestEvaluatePassesCaseStubAndMergesJudgeCost(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	rubric := Rubric{Agent: "selftest", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-5"},
		{Name: "clarity2", Scoring: "1-5"},
	}}
	cases := []TestCase{{
		Name:  "c1",
		Input: "do it",
		Stub: &inference.StubScript{Turns: []inference.StubTurn{{
			Response: "## H\n### S\ntext",
			Usage:    &inference.StubUsage{InputTokens: 10, OutputTokens: 5},
		}}},
	}}

	res := evaluate(rubric, cases, "abc", &strings.Builder{}, nil)
	if len(res.Cases) != 1 {
		t.Fatalf("cases = %d", len(res.Cases))
	}
	cr := res.Cases[0]
	if cr.ActualOutput != "## H\n### S\ntext" {
		t.Errorf("output = %q", cr.ActualOutput)
	}
	if cr.AgentCost.UsageSource != "reported" || cr.AgentCost.TokensIn != 10 {
		t.Errorf("agent cost = %+v", cr.AgentCost)
	}
	if cr.JudgeCost.UsageSource != "estimated" || cr.JudgeCost.Model != "stub" || cr.JudgeCost.TokensIn == 0 {
		t.Errorf("judge cost = %+v", cr.JudgeCost)
	}
	for _, s := range cr.Scores {
		if s.Skipped || s.Score != s.MaxScore {
			t.Errorf("score %+v not max", s)
		}
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestAgentConfigDirOnlyWhenAgentConfigExists(t *testing.T) {
	chdirTemp(t)
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil {
		t.Fatal(err)
	}
	if got := agentConfigDir("selftest"); got != "" {
		t.Errorf("agentConfigDir = %q, want empty when config missing", got)
	}
	writeCfgFile(t, filepath.Join("evals", "agents", "selftest.json"), "{}")
	if got := agentConfigDir("selftest"); got != filepath.Join("evals", "agents") {
		t.Errorf("agentConfigDir = %q", got)
	}
}

func TestNoKiroCLIProcessOutsideDockerPath(t *testing.T) {
	// Guard rail for the acceptance criterion: perf.go must not spawn kiro-cli.
	data, err := os.ReadFile("perf.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `exec.Command("kiro-cli"`) {
		t.Error(`perf.go contains exec.Command("kiro-cli"...)`)
	}
}
