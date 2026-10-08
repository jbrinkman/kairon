package eval

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exitEvalsDir copies the self-test fixtures to a temp evals dir keeping only
// the stub-backend agents that run natively (selftest passes, selftest-fail
// fails), so a run over all agents has a deterministic verdict.
func exitEvalsDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, dir)
	for _, n := range []string{"selftest-sandbox", "selftest-sandbox-ro"} {
		_ = os.Remove(filepath.Join(dir, "rubrics", n+".yaml"))
		_ = os.RemoveAll(filepath.Join(dir, "cases", n))
	}
	return dir
}

// tolerateThresholdFailure turns a *ThresholdError into nil. Tests that inspect
// the results of a run (not its verdict) use it for agents that legitimately
// fail their pass threshold; any other error is returned unchanged.
func tolerateThresholdFailure(err error) error {
	if errors.Is(err, ErrThresholdFailed) {
		return nil
	}
	return err
}

// verdictLines returns the lines of out that start with PASS or FAIL.
func verdictLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "PASS ") || strings.HasPrefix(l, "FAIL ") {
			lines = append(lines, l)
		}
	}
	return lines
}

// onlyRunDir returns the single run directory under evalsDir/results.
func onlyRunDir(t *testing.T, evalsDir string) string {
	t.Helper()
	added := newRunDirs(t, filepath.Join(evalsDir, "results"), map[string]bool{})
	if len(added) != 1 {
		t.Fatalf("run dirs = %v, want exactly one", added)
	}
	return filepath.Join(evalsDir, "results", added[0])
}

func TestThresholdErrorWrapsSentinelAndNamesAgents(t *testing.T) {
	var err error = &ThresholdError{Agents: []string{"alpha", "beta"}}
	if !errors.Is(err, ErrThresholdFailed) {
		t.Errorf("errors.Is(err, ErrThresholdFailed) = false for %v", err)
	}
	var te *ThresholdError
	if !errors.As(err, &te) || len(te.Agents) != 2 {
		t.Errorf("errors.As failed: %v", te)
	}
	msg := err.Error()
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %s", msg, want)
		}
	}
	if strings.Contains(msg, "\n") {
		t.Errorf("error %q must be one line", msg)
	}
}

func TestRunFailingAgentReturnsThresholdError(t *testing.T) {
	t.Cleanup(resetConfig)
	evalsDir := exitEvalsDir(t)

	var err error
	out := captureStdout(t, func() {
		err = RunWithOptions("selftest-fail", "", RunOptions{Backend: "stub", EvalsDir: evalsDir})
	})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	if !strings.Contains(err.Error(), "selftest-fail") {
		t.Errorf("error %q does not name the failing agent", err)
	}

	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL ") {
		t.Fatalf("verdict lines = %q, want exactly one FAIL line\n%s", lines, out)
	}
	for _, want := range []string{"selftest-fail", "0.0%", "95.0%", "2/2"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("FAIL line %q lacks %q", lines[0], want)
		}
	}

	// The error is returned only after the results, summary and progress-file
	// cleanup are done.
	runDir := onlyRunDir(t, evalsDir)
	if _, statErr := os.Stat(filepath.Join(runDir, "selftest-fail.json")); statErr != nil {
		t.Errorf("result file missing: %v", statErr)
	}
	s, _ := readSummary(t, filepath.Join(runDir, "summary.json"))
	if v, ok := s.AgentVerdicts["selftest-fail"]; !ok || v.Passed {
		t.Errorf("summary verdict = %+v ok=%v, want recorded and not passed", v, ok)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, ".progress")); !os.IsNotExist(statErr) {
		t.Errorf(".progress still present (stat err=%v)", statErr)
	}
	if !strings.Contains(out, "Evaluation complete") {
		t.Errorf("output lacks completion message:\n%s", out)
	}
}

func TestRunPassingAgentReturnsNil(t *testing.T) {
	t.Cleanup(resetConfig)
	evalsDir := exitEvalsDir(t)

	var err error
	out := captureStdout(t, func() {
		err = RunWithOptions("selftest", "", RunOptions{Backend: "stub", EvalsDir: evalsDir})
	})
	if err != nil {
		t.Fatalf("passing run returned %v", err)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PASS ") {
		t.Fatalf("verdict lines = %q, want exactly one PASS line\n%s", lines, out)
	}
	for _, want := range []string{"selftest", "100.0%", "95.0%", "0/5"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("PASS line %q lacks %q", lines[0], want)
		}
	}
}

func TestRunAllAgentsOneLineEachAndErrorNamesOnlyFailing(t *testing.T) {
	t.Cleanup(resetConfig)
	evalsDir := exitEvalsDir(t)

	var err error
	out := captureStdout(t, func() {
		err = RunWithOptions("", "", RunOptions{Backend: "stub", EvalsDir: evalsDir})
	})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	if !strings.Contains(err.Error(), "selftest-fail") {
		t.Errorf("error %q does not name selftest-fail", err)
	}
	if strings.Contains(strings.ReplaceAll(err.Error(), "selftest-fail", ""), "selftest") {
		t.Errorf("error %q names the passing agent", err)
	}

	lines := verdictLines(out)
	if len(lines) != 2 {
		t.Fatalf("verdict lines = %q, want one per agent\n%s", lines, out)
	}
	var pass, fail int
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "PASS selftest:"):
			pass++
		case strings.HasPrefix(l, "FAIL selftest-fail:"):
			fail++
		}
	}
	if pass != 1 || fail != 1 {
		t.Errorf("lines = %q, want PASS selftest and FAIL selftest-fail", lines)
	}

	// Every agent was still written before the error was returned.
	runDir := onlyRunDir(t, evalsDir)
	s, _ := readSummary(t, filepath.Join(runDir, "summary.json"))
	if len(s.AgentVerdicts) != 2 {
		t.Errorf("summary verdicts = %+v, want both agents", s.AgentVerdicts)
	}
}

func TestRunSingleCaseVerdictLineAndError(t *testing.T) {
	t.Cleanup(resetConfig)
	evalsDir := exitEvalsDir(t)

	t.Run("failing", func(t *testing.T) {
		var err error
		out := captureStdout(t, func() {
			err = RunWithOptions("selftest-fail", "stub-timeout", RunOptions{Backend: "stub", EvalsDir: evalsDir})
		})
		if !errors.Is(err, ErrThresholdFailed) {
			t.Fatalf("err = %v, want ErrThresholdFailed", err)
		}
		if !strings.Contains(err.Error(), "selftest-fail") {
			t.Errorf("error %q does not name the agent", err)
		}
		lines := verdictLines(out)
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL ") {
			t.Fatalf("verdict lines = %q, want exactly one FAIL line\n%s", lines, out)
		}
		// The result and summary are written before the error is returned.
		added := newRunDirs(t, filepath.Join(evalsDir, "results"), map[string]bool{})
		if len(added) == 0 {
			t.Fatal("no run directory written")
		}
		var found bool
		for _, d := range added {
			if _, statErr := os.Stat(filepath.Join(evalsDir, "results", d, "summary.json")); statErr == nil {
				found = true
			}
		}
		if !found {
			t.Error("summary.json not written for the failing single case")
		}
		if !strings.Contains(out, "Performance Summary") {
			t.Errorf("single-case output lacks the performance summary:\n%s", out)
		}
	})

	t.Run("passing", func(t *testing.T) {
		var err error
		out := captureStdout(t, func() {
			err = RunWithOptions("selftest", "stub-basic", RunOptions{Backend: "stub", EvalsDir: evalsDir})
		})
		if err != nil {
			t.Fatalf("passing single case returned %v", err)
		}
		lines := verdictLines(out)
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "PASS ") {
			t.Fatalf("verdict lines = %q, want exactly one PASS line\n%s", lines, out)
		}
		if !strings.Contains(lines[0], "0/1") {
			t.Errorf("PASS line %q lacks failed/total 0/1", lines[0])
		}
	})
}

func TestRunAgentWithoutCasesIsSkippedAndDoesNotAffectVerdict(t *testing.T) {
	t.Cleanup(resetConfig)
	evalsDir := exitEvalsDir(t)
	// A rubric whose agent has no cases directory.
	rubric := "agent: nocases\ncriteria:\n  - name: q\n    description: d\n    scoring: 1-5\n    deterministic: true\n"
	if err := os.WriteFile(filepath.Join(evalsDir, "rubrics", "nocases.yaml"), []byte(rubric), 0644); err != nil {
		t.Fatal(err)
	}
	agentCfg, err := os.ReadFile(filepath.Join(evalsDir, "agents", "selftest.json"))
	if err != nil {
		t.Fatal(err)
	}
	agentCfg = []byte(strings.Replace(string(agentCfg), "selftest", "nocases", 1))
	if err := os.WriteFile(filepath.Join(evalsDir, "agents", "nocases.json"), agentCfg, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(evalsDir, "rubrics", "selftest-fail.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(evalsDir, "cases", "selftest-fail")); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		err = RunWithOptions("", "", RunOptions{Backend: "stub", EvalsDir: evalsDir})
	})
	if err != nil {
		t.Fatalf("run with a skipped agent returned %v, want nil", err)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PASS selftest:") {
		t.Errorf("verdict lines = %q, want only the PASS selftest line", lines)
	}
	for _, l := range lines {
		if strings.Contains(l, "nocases") {
			t.Errorf("skipped agent got a verdict line: %q", l)
		}
	}
}
