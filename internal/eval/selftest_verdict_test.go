package eval

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// verdictRun runs agent (all agents when "") against the stub backend in a
// fresh copy of the self-test fixtures and returns the evals dir, the single
// run directory, the run error and the captured stdout.
func verdictRun(t *testing.T, agent string) (evalsDir, runDir string, err error, out string) {
	t.Helper()
	t.Cleanup(resetConfig)
	evalsDir = exitEvalsDir(t)
	out = captureStdout(t, func() {
		err = RunWithOptions(agent, "", RunOptions{Backend: "stub", EvalsDir: evalsDir})
	})
	return evalsDir, onlyRunDir(t, evalsDir), err, out
}

// The selftest-fail stub-empty-response case scripts an empty response, so the
// agent produces no output and every non-cost criterion must score 0 over a
// non-zero maximum (a failure cannot be excluded from the denominator).
func TestSelftestFailEmptyResponseCaseScoresZeroOverMax(t *testing.T) {
	_, runDir, err, _ := verdictRun(t, "selftest-fail")
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	res := readSelfTestResult(t, filepath.Join(runDir, "selftest-fail.json"))

	var found bool
	for _, c := range res.Cases {
		if c.CaseName != "stub-empty-response" {
			continue
		}
		found = true
		if c.ActualOutput != "" {
			t.Errorf("actual_output = %q, want empty", c.ActualOutput)
		}
		var scored int
		for _, s := range c.Scores {
			if s.Name == "cost_efficiency" {
				continue
			}
			scored++
			if s.Score != 0 || s.MaxScore <= 0 {
				t.Errorf("criterion %s = %d/%d, want 0 over a non-zero max", s.Name, s.Score, s.MaxScore)
			}
		}
		if scored == 0 {
			t.Error("case has no scored criteria")
		}
	}
	if !found {
		t.Fatalf("stub-empty-response missing from results: %+v", res.Cases)
	}
}

func TestSelftestVerdictPassesAndExitsClean(t *testing.T) {
	_, runDir, err, out := verdictRun(t, "selftest")
	if err != nil {
		t.Fatalf("selftest run returned %v, want nil", err)
	}
	s, raw := readSummary(t, filepath.Join(runDir, "summary.json"))
	if _, ok := raw["agent_verdicts"]; !ok {
		t.Fatalf("summary.json lacks agent_verdicts: %s", raw)
	}
	v, ok := s.AgentVerdicts["selftest"]
	if !ok {
		t.Fatalf("agent_verdicts lacks selftest: %+v", s.AgentVerdicts)
	}
	if !v.Passed || v.Score != 100 || v.Threshold != 95 || v.CasesTotal != 5 || v.CasesFailed != 0 {
		t.Errorf("verdict = %+v, want passed, score 100, threshold 95, 0/5 failed", v)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || lines[0][:5] != "PASS " {
		t.Errorf("verdict lines = %q, want one PASS line", lines)
	}
}

func TestSelftestFailVerdictFailsEveryCase(t *testing.T) {
	_, runDir, err, out := verdictRun(t, "selftest-fail")
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	s, _ := readSummary(t, filepath.Join(runDir, "summary.json"))
	v, ok := s.AgentVerdicts["selftest-fail"]
	if !ok {
		t.Fatalf("agent_verdicts lacks selftest-fail: %+v", s.AgentVerdicts)
	}
	if v.Passed || v.Score >= v.Threshold || v.CasesTotal == 0 || v.CasesFailed != v.CasesTotal {
		t.Errorf("verdict = %+v, want not passed, score < threshold and every case failed", v)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || lines[0][:5] != "FAIL " {
		t.Errorf("verdict lines = %q, want one FAIL line", lines)
	}
}

// Running selftest and selftest-fail together accumulates the cost across
// both agents instead of keeping only the agent saved last.
func TestSelftestCostAccumulatesAcrossAgents(t *testing.T) {
	_, runDir, err, _ := verdictRun(t, "")
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed (selftest-fail fails)", err)
	}
	s, _ := readSummary(t, filepath.Join(runDir, "summary.json"))

	var want CostInfo
	for _, agent := range []string{"selftest", "selftest-fail"} {
		res := readSelfTestResult(t, filepath.Join(runDir, agent+".json"))
		for _, c := range res.Cases {
			want.TokensIn += c.AgentCost.TokensIn + c.JudgeCost.TokensIn
			want.TokensOut += c.AgentCost.TokensOut + c.JudgeCost.TokensOut
			want.EstimatedUSD += c.AgentCost.EstimatedUSD + c.JudgeCost.EstimatedUSD
		}
	}
	if want.TokensIn == 0 {
		t.Fatal("fixture runs produced no token usage; the cost assertion would be vacuous")
	}
	if s.TotalCost.TokensIn != want.TokensIn || s.TotalCost.TokensOut != want.TokensOut ||
		math.Abs(s.TotalCost.EstimatedUSD-want.EstimatedUSD) > 1e-9 {
		t.Errorf("total_cost = %+v, want sum of both agents %+v", s.TotalCost, want)
	}

	var verdictSum CostInfo
	for _, v := range s.AgentVerdicts {
		verdictSum.TokensIn += v.Cost.TokensIn
		verdictSum.TokensOut += v.Cost.TokensOut
		verdictSum.EstimatedUSD += v.Cost.EstimatedUSD
	}
	if len(s.AgentVerdicts) != 2 || verdictSum.TokensIn != s.TotalCost.TokensIn ||
		verdictSum.TokensOut != s.TotalCost.TokensOut ||
		math.Abs(verdictSum.EstimatedUSD-s.TotalCost.EstimatedUSD) > 1e-9 {
		t.Errorf("verdict costs %+v do not sum to total_cost %+v", verdictSum, s.TotalCost)
	}
}

// Pre-change result directories (no verdicts, no thresholds, skipped criteria)
// and a new-format run both load through eval diff without error.
func TestDiffLegacyFixturesAgainstNewFormatSummary(t *testing.T) {
	evalsDir, newRun, err, _ := verdictRun(t, "selftest")
	if err != nil {
		t.Fatalf("selftest run returned %v", err)
	}
	results := filepath.Join(evalsDir, "results")
	for _, fixture := range []string{"legacy-native", "native-pinned"} {
		src := filepath.Join(packageDir, runFixturesDir, fixture)
		if err := os.CopyFS(filepath.Join(results, fixture), os.DirFS(src)); err != nil {
			t.Fatal(err)
		}
	}

	newName := filepath.Base(newRun)
	pairs := [][2]string{
		{"legacy-native", newName},
		{newName, "legacy-native"},
		{"native-pinned", newName},
		{"legacy-native", "native-pinned"},
	}
	for _, p := range pairs {
		var buf bytes.Buffer
		if err := diffTo(&buf, p[0], p[1]); err != nil {
			t.Errorf("diffTo(%s, %s): %v", p[0], p[1], err)
		}
		if buf.Len() == 0 {
			t.Errorf("diffTo(%s, %s) wrote nothing", p[0], p[1])
		}
	}
}
