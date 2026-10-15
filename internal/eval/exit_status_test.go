package eval

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// verdictLines returns the lines of out that start with a PASS/FAIL verdict
// word, in order.
func verdictLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "PASS ") || strings.HasPrefix(l, "FAIL ") {
			lines = append(lines, l)
		}
	}
	return lines
}

// runStubCapturing runs RunWithOptions with the stub backend against evalsDir,
// returning what it wrote to stdout and its error.
func runStubCapturing(t *testing.T, evalsDir, agent, testcase string, opts RunOptions) (string, error) {
	t.Helper()
	t.Cleanup(resetConfig)
	opts.Backend = "stub"
	opts.EvalsDir = evalsDir
	opts.NoSandbox = true
	var err error
	out := captureStdout(t, func() {
		err = RunWithOptions(agent, testcase, opts)
	})
	return out, err
}

// onlyRunDir returns the single run directory under evalsDir/results.
func onlyRunDir(t *testing.T, evalsDir string) string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(evalsDir, "results", "*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("result dirs under %s = %v (err %v), want exactly 1", evalsDir, dirs, err)
	}
	return dirs[0]
}

func TestPrintAgentVerdictFormat(t *testing.T) {
	tests := []struct {
		name string
		v    AgentVerdict
		want string
	}{
		{"pass", AgentVerdict{Score: 100, Threshold: 95, Passed: true, CasesTotal: 16}, "PASS selftest: 100.0% (threshold 95.0%), 0/16 cases failed\n"},
		{"fail", AgentVerdict{Score: 4, Threshold: 95, Passed: false, CasesTotal: 13, CasesFailed: 13}, "FAIL selftest: 4.0% (threshold 95.0%), 13/13 cases failed\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printAgentVerdict(&buf, "selftest", tt.v)
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestThresholdErrorWrapsSentinelAndNamesAgents(t *testing.T) {
	err := error(&ThresholdError{
		Total: 3,
		Failed: []AgentFailure{
			{Agent: "alpha", Score: 12.5, Threshold: 95},
			{Agent: "beta", Score: 80, Threshold: 90},
		},
	})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("errors.Is(err, ErrThresholdFailed) = false for %v", err)
	}
	var te *ThresholdError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(*ThresholdError) = false")
	}
	for _, want := range []string{"2 of 3 agents", "alpha (12.5% < 95.0%)", "beta (80.0% < 90.0%)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestRunSelftestPassesAndPrintsOneVerdictLine(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	out, err := runStubCapturing(t, evalsDir, "selftest", "", RunOptions{})
	if err != nil {
		t.Fatalf("selftest must pass, got %v", err)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PASS selftest: 100.0% (threshold 95.0%), 0/") {
		t.Fatalf("verdict lines = %q\nfull output:\n%s", lines, out)
	}
}

func TestRunSelftestFailReturnsThresholdErrorAfterWritingResults(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	out, err := runStubCapturing(t, evalsDir, "selftest-fail", "", RunOptions{})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	if !strings.Contains(err.Error(), "selftest-fail (") || !strings.Contains(err.Error(), "< 95.0%") {
		t.Errorf("error text %q does not name the agent with score and threshold", err.Error())
	}
	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL selftest-fail: ") || !strings.Contains(lines[0], "(threshold 95.0%)") {
		t.Fatalf("verdict lines = %q\nfull output:\n%s", lines, out)
	}

	// Everything is written before the error is returned.
	runDir := onlyRunDir(t, evalsDir)
	var s Summary
	readJSON(t, filepath.Join(runDir, "summary.json"), &s)
	if v, ok := s.AgentVerdicts["selftest-fail"]; !ok || v.Passed {
		t.Errorf("summary verdict = %+v, want a recorded failure", v)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, ".progress")); !os.IsNotExist(statErr) {
		t.Errorf(".progress still present (stat err = %v)", statErr)
	}
	if !strings.Contains(out, "Evaluation complete") {
		t.Errorf("run did not complete before returning the error:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, "performance.json")); statErr != nil {
		t.Errorf("performance report not written before the error: %v", statErr)
	}
}

func TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	for _, name := range []string{"selftest-sandbox.yaml", "selftest-sandbox-ro.yaml"} {
		if err := os.Remove(filepath.Join(evalsDir, "rubrics", name)); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runStubCapturing(t, evalsDir, "", "", RunOptions{})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	if !strings.Contains(err.Error(), "1 of 2 agents") || !strings.Contains(err.Error(), "selftest-fail (") {
		t.Errorf("error %q must name only selftest-fail (1 of 2)", err.Error())
	}
	if strings.Contains(err.Error(), "selftest (") {
		t.Errorf("error %q names the passing agent", err.Error())
	}
	lines := verdictLines(out)
	if len(lines) != 2 {
		t.Fatalf("verdict lines = %q, want one per agent\n%s", lines, out)
	}
	seen := map[string]string{}
	for _, l := range lines {
		seen[strings.SplitN(strings.SplitN(l, ": ", 2)[0], " ", 2)[1]] = strings.SplitN(l, " ", 2)[0]
	}
	if seen["selftest"] != "PASS" || seen["selftest-fail"] != "FAIL" {
		t.Errorf("verdicts by agent = %v", seen)
	}

	runDir := onlyRunDir(t, evalsDir)
	var s Summary
	readJSON(t, filepath.Join(runDir, "summary.json"), &s)
	var want CostInfo
	for _, agent := range []string{"selftest", "selftest-fail"} {
		c := agentCostTotals(readSelfTestResult(t, filepath.Join(runDir, agent+".json")))
		want.TokensIn += c.TokensIn
		want.TokensOut += c.TokensOut
		want.EstimatedUSD += c.EstimatedUSD
	}
	if want.TokensIn == 0 && want.TokensOut == 0 {
		t.Fatal("fixture agents report no tokens; the cost-sum assertion would be vacuous")
	}
	costClose(t, "total_cost", s.TotalCost, want)
}

func TestSingleCaseRunPrintsVerdictAndReturnsThresholdError(t *testing.T) {
	t.Run("failing case", func(t *testing.T) {
		evalsDir := copySelfTestEvals(t)
		out, err := runStubCapturing(t, evalsDir, "selftest-fail", "stub-empty-response", RunOptions{})
		if !errors.Is(err, ErrThresholdFailed) {
			t.Fatalf("err = %v, want ErrThresholdFailed", err)
		}
		lines := verdictLines(out)
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL selftest-fail: 0.0% (threshold 95.0%), 1/1 cases failed") {
			t.Fatalf("verdict lines = %q\n%s", lines, out)
		}
		// Results and summary are written before the error is returned.
		runDir := onlyRunDir(t, evalsDir)
		var s Summary
		readJSON(t, filepath.Join(runDir, "summary.json"), &s)
		if v := s.AgentVerdicts["selftest-fail"]; v.Passed || v.CasesTotal != 1 {
			t.Errorf("summary verdict = %+v", v)
		}
		if _, statErr := os.Stat(filepath.Join(runDir, "selftest-fail.json")); statErr != nil {
			t.Errorf("agent result not written: %v", statErr)
		}
	})
	t.Run("passing case", func(t *testing.T) {
		evalsDir := copySelfTestEvals(t)
		out, err := runStubCapturing(t, evalsDir, "selftest", "stub-basic", RunOptions{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		lines := verdictLines(out)
		if len(lines) != 1 || lines[0] != "PASS selftest: 100.0% (threshold 95.0%), 0/1 cases failed" {
			t.Fatalf("verdict lines = %q\n%s", lines, out)
		}
	})
}

func TestResumeRunPrintsVerdictAndReturnsThresholdError(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	if _, err := runStubCapturing(t, evalsDir, "selftest-fail", "", RunOptions{}); !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("first run err = %v, want ErrThresholdFailed", err)
	}
	runDir := onlyRunDir(t, evalsDir)
	// Simulate an interrupted run.
	if err := os.WriteFile(filepath.Join(runDir, ".progress"), []byte("in-progress"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runStubCapturing(t, evalsDir, "selftest-fail", "", RunOptions{Resume: true})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("resume err = %v, want ErrThresholdFailed", err)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL selftest-fail: ") {
		t.Fatalf("verdict lines = %q\n%s", lines, out)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, ".progress")); !os.IsNotExist(statErr) {
		t.Errorf(".progress still present after resume (stat err = %v)", statErr)
	}
	var s Summary
	readJSON(t, filepath.Join(runDir, "summary.json"), &s)
	if v, ok := s.AgentVerdicts["selftest-fail"]; !ok || v.Passed {
		t.Errorf("summary verdict = %+v", v)
	}
}
