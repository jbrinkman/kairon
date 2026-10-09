package eval

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

// scored builds a case whose single criterion scored score/max.
func scored(name string, score, max int) CaseResult {
	return CaseResult{CaseName: name, ActualOutput: "out", Scores: []CriterionScore{{Name: "x", Score: score, MaxScore: max}}}
}

func TestAgentVerdictThresholdBoundary(t *testing.T) {
	tests := []struct {
		name      string
		score     int
		threshold float64
		wantPass  bool
		wantScore float64
	}{
		{"exactly at threshold passes", 19, 95, true, 95},
		{"just below threshold fails", 18, 95, false, 90},
		{"perfect passes", 20, 95, true, 100},
		{"custom threshold", 16, 80, true, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := agentVerdict(AgentResult{Agent: "a", Threshold: tt.threshold, Cases: []CaseResult{scored("c", tt.score, 20)}})
			if v.Passed != tt.wantPass {
				t.Errorf("Passed = %v, want %v", v.Passed, tt.wantPass)
			}
			if v.Score != tt.wantScore {
				t.Errorf("Score = %v, want %v (percent)", v.Score, tt.wantScore)
			}
			if v.Threshold != tt.threshold {
				t.Errorf("Threshold = %v, want %v", v.Threshold, tt.threshold)
			}
		})
	}
}

func TestAgentVerdictZeroDenominatorFailsClosed(t *testing.T) {
	for name, ar := range map[string]AgentResult{
		"no cases":        {Agent: "a", Threshold: 95},
		"zero max":        {Agent: "a", Threshold: 95, Cases: []CaseResult{{CaseName: "c", Scores: []CriterionScore{{Name: "x", MaxScore: 0}}}}},
		"no scores":       {Agent: "a", Threshold: 95, Cases: []CaseResult{{CaseName: "c"}}},
		"zero thresh too": {Agent: "a", Cases: []CaseResult{{CaseName: "c"}}},
	} {
		t.Run(name, func(t *testing.T) {
			v := agentVerdict(ar)
			if v.Passed || v.Score != 0 {
				t.Errorf("verdict = %+v, want score 0 and passed false", v)
			}
		})
	}
}

func TestAgentVerdictCasesTotalAndFailed(t *testing.T) {
	yes, no := true, false
	pass := scored("stamped-pass", 5, 5)
	pass.Passed = &yes
	fail := scored("stamped-fail", 1, 5)
	fail.Passed = &no
	// A stamped verdict wins over a recompute from the scores.
	stampedFailDespiteScore := scored("stamped-fail-high", 5, 5)
	stampedFailDespiteScore.Passed = &no
	legacyPass := scored("legacy-pass", 5, 5)           // Passed == nil, 100% >= 95
	legacyFail := scored("legacy-fail", 4, 5)           // Passed == nil, 80% < 95
	legacyEmpty := CaseResult{CaseName: "legacy-empty"} // Passed == nil, nothing measured

	v := agentVerdict(AgentResult{Agent: "a", Cases: []CaseResult{pass, fail, stampedFailDespiteScore, legacyPass, legacyFail, legacyEmpty}})
	if v.CasesTotal != 6 {
		t.Errorf("CasesTotal = %d, want 6", v.CasesTotal)
	}
	// stamped-fail, stamped-fail-high, legacy-fail (threshold falls back to 95), legacy-empty.
	if v.CasesFailed != 4 {
		t.Errorf("CasesFailed = %d, want 4", v.CasesFailed)
	}

	// A legacy case is recomputed against the agent's recorded threshold.
	v = agentVerdict(AgentResult{Agent: "a", Threshold: 80, Cases: []CaseResult{legacyFail}})
	if v.CasesFailed != 0 {
		t.Errorf("CasesFailed at threshold 80 = %d, want 0 (4/5 = 80%%)", v.CasesFailed)
	}
}

func TestAgentVerdictDoesNotFailAgentOnCaseFailuresAlone(t *testing.T) {
	no := false
	c := scored("c", 5, 5)
	c.Passed = &no
	v := agentVerdict(AgentResult{Agent: "a", Threshold: 95, Cases: []CaseResult{c, scored("d", 95, 95), scored("e", 100, 100)}})
	if !v.Passed || v.CasesFailed != 1 {
		t.Errorf("verdict = %+v, want passed with 1 failed case (cases_failed is informational)", v)
	}
}

func TestAgentCostTotalsIncludesAgentAndJudge(t *testing.T) {
	ar := AgentResult{Agent: "a", Cases: []CaseResult{
		{AgentCost: CostInfo{TokensIn: 10, TokensOut: 5, EstimatedUSD: 0.5, Model: "m"}, JudgeCost: CostInfo{TokensIn: 3, TokensOut: 2, EstimatedUSD: 0.25}},
		{AgentCost: CostInfo{TokensIn: 1, TokensOut: 1, EstimatedUSD: 0.125}},
	}}
	got := agentCostTotals(ar)
	if got.TokensIn != 14 || got.TokensOut != 8 || math.Abs(got.EstimatedUSD-0.875) > 1e-9 {
		t.Errorf("agentCostTotals = %+v, want 14 in / 8 out / 0.875 USD", got)
	}
	if got.Model != "" || got.UsageSource != "" {
		t.Errorf("model/usage_source must not be aggregated, got %+v", got)
	}
}

func verdictRubric() Rubric {
	return Rubric{Agent: "selftest", PassThreshold: floatPtr(80), Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
	}}
}

func verdictCases() []TestCase {
	stub := func(resp string) *inference.StubScript {
		return &inference.StubScript{Turns: []inference.StubTurn{{Response: resp}}}
	}
	return []TestCase{
		{Name: "pass-case", Input: "do it", Stub: stub("## H\n### S\ntext")},               // 100%
		{Name: "fail-case", Input: "do it", Stub: stub("## only one section")},             // 40% < 80
		{Name: "lenient-case", Input: "do it", MinScore: floatPtr(30), Stub: stub("## x")}, // 40% >= 30
	}
}

func checkStamped(t *testing.T, label string, res AgentResult) {
	t.Helper()
	if res.Threshold != 80 {
		t.Errorf("%s: AgentResult.Threshold = %v, want 80", label, res.Threshold)
	}
	want := map[string]struct {
		threshold float64
		passed    bool
	}{"pass-case": {80, true}, "fail-case": {80, false}, "lenient-case": {30, true}}
	for _, c := range res.Cases {
		w, ok := want[c.CaseName]
		if !ok {
			continue
		}
		if c.Threshold != w.threshold {
			t.Errorf("%s: case %s Threshold = %v, want %v", label, c.CaseName, c.Threshold, w.threshold)
		}
		if c.Passed == nil || *c.Passed != w.passed {
			t.Errorf("%s: case %s Passed = %v, want %v", label, c.CaseName, c.Passed, w.passed)
		}
	}
}

func TestEvaluateStampsThresholdAndPassed(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)

	var out strings.Builder
	checkStamped(t, "evaluate", evaluate(verdictRubric(), verdictCases(), "abc", &out, nil))
	checkStamped(t, "evaluateProgressive", evaluateProgressive(verdictRubric(), verdictCases(), "abc", &out, t.TempDir(), false, nil))
}

func TestEvaluateProgressiveResumeStampsThreshold(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)

	dir := t.TempDir()
	// A result written before verdicts existed: one completed case, no thresholds.
	legacy := AgentResult{Agent: "selftest", GitHash: "abc", Cases: []CaseResult{scored("done-case", 5, 5)}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "selftest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	cases := append([]TestCase{{Name: "done-case", Input: "do it"}}, verdictCases()...)
	res := evaluateProgressive(verdictRubric(), cases, "abc", &out, dir, true, nil)
	checkStamped(t, "resume", res)

	// The agent file that was saved carries the threshold too.
	var saved AgentResult
	readJSON(t, filepath.Join(dir, "selftest.json"), &saved)
	if saved.Threshold != 80 {
		t.Errorf("saved selftest.json threshold = %v, want 80", saved.Threshold)
	}
}

func verdictAgent(agent string, score, max int, cost CostInfo) AgentResult {
	cr := scored("c", score, max)
	cr.Threshold, cr.AgentCost = 95, cost
	cr.JudgeCost = CostInfo{TokensIn: 1, TokensOut: 1, EstimatedUSD: 0.01}
	p := score*100 >= 95*max
	cr.Passed = &p
	return AgentResult{Agent: agent, Threshold: 95, Cases: []CaseResult{cr}}
}

func TestUpdateIncrementalSummaryWritesAgentVerdicts(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	for _, ar := range []AgentResult{
		verdictAgent("a", 19, 20, CostInfo{TokensIn: 10, TokensOut: 4, EstimatedUSD: 0.1}),
		verdictAgent("b", 10, 20, CostInfo{TokensIn: 20, TokensOut: 8, EstimatedUSD: 0.2}),
	} {
		if err := updateIncrementalSummary(file, ar, "g"); err != nil {
			t.Fatal(err)
		}
	}
	var s Summary
	readJSON(t, file, &s)

	a, b := s.AgentVerdicts["a"], s.AgentVerdicts["b"]
	if a.Score != 95 || a.Threshold != 95 || !a.Passed || a.CasesTotal != 1 || a.CasesFailed != 0 {
		t.Errorf("verdict a = %+v", a)
	}
	if b.Score != 50 || b.Threshold != 95 || b.Passed || b.CasesTotal != 1 || b.CasesFailed != 1 {
		t.Errorf("verdict b = %+v (a's verdict must survive b's save)", b)
	}
	// agent_scores stays a 0-1 fraction.
	if s.AgentScores["a"] != 0.95 || s.AgentScores["b"] != 0.5 {
		t.Errorf("agent_scores = %v, want fractions 0.95 / 0.5", s.AgentScores)
	}

	var raw struct {
		Verdicts map[string]map[string]any `json:"agent_verdicts"`
	}
	data, _ := os.ReadFile(file)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"score", "threshold", "passed", "cases_total", "cases_failed", "cost"} {
		if _, ok := raw.Verdicts["a"][k]; !ok {
			t.Errorf("summary.json agent_verdicts.a missing %q", k)
		}
	}
}

func costClose(t *testing.T, label string, got, want CostInfo) {
	t.Helper()
	if got.TokensIn != want.TokensIn || got.TokensOut != want.TokensOut {
		t.Errorf("%s tokens = %d/%d, want %d/%d", label, got.TokensIn, got.TokensOut, want.TokensIn, want.TokensOut)
	}
	if math.Abs(got.EstimatedUSD-want.EstimatedUSD) > 1e-9 {
		t.Errorf("%s USD = %v, want %v", label, got.EstimatedUSD, want.EstimatedUSD)
	}
}

func TestSummaryTotalCostAccumulatesAcrossAgents(t *testing.T) {
	a := verdictAgent("a", 19, 20, CostInfo{TokensIn: 10, TokensOut: 4, EstimatedUSD: 0.1})
	b := verdictAgent("b", 10, 20, CostInfo{TokensIn: 20, TokensOut: 8, EstimatedUSD: 0.2})
	c := verdictAgent("c", 20, 20, CostInfo{TokensIn: 7, TokensOut: 3, EstimatedUSD: 0.3})
	want := CostInfo{}
	for _, ar := range []AgentResult{a, b, c} {
		want.TokensIn += agentCostTotals(ar).TokensIn
		want.TokensOut += agentCostTotals(ar).TokensOut
		want.EstimatedUSD += agentCostTotals(ar).EstimatedUSD
	}
	if want.TokensIn != 40 { // 10+1 + 20+1 + 7+1: judge cost included
		t.Fatalf("test setup: want tokens_in 40, got %d", want.TokensIn)
	}

	save := func(order ...AgentResult) Summary {
		file := filepath.Join(t.TempDir(), "summary.json")
		for _, ar := range order {
			if err := updateIncrementalSummary(file, ar, "g"); err != nil {
				t.Fatal(err)
			}
		}
		var s Summary
		readJSON(t, file, &s)
		return s
	}

	forward := save(a, b, c)
	costClose(t, "forward", forward.TotalCost, want)
	costClose(t, "reverse order", save(c, b, a).TotalCost, want)
	// Repeated progressive saves of the same agent must not change the total.
	costClose(t, "repeated saves", save(a, a, b, b, b, c, a).TotalCost, want)

	// Each agent's own cost is recorded in its verdict.
	costClose(t, "verdict a cost", forward.AgentVerdicts["a"].Cost, agentCostTotals(a))
}

func TestBuildSummaryWritesVerdictsAndAccumulatesCost(t *testing.T) {
	a := verdictAgent("a", 19, 20, CostInfo{TokensIn: 10, TokensOut: 4, EstimatedUSD: 0.1})
	b := verdictAgent("b", 10, 20, CostInfo{TokensIn: 20, TokensOut: 8, EstimatedUSD: 0.2})
	zero := AgentResult{Agent: "zero", Threshold: 95, Cases: []CaseResult{{CaseName: "c"}}}

	s := buildSummary([]AgentResult{a, b, zero}, "g")
	if !s.AgentVerdicts["a"].Passed || s.AgentVerdicts["b"].Passed {
		t.Errorf("verdicts = %+v", s.AgentVerdicts)
	}
	if v, ok := s.AgentVerdicts["zero"]; !ok || v.Passed || v.Score != 0 {
		t.Errorf("zero-denominator agent verdict = %+v (present %v), want present, failed, score 0", v, ok)
	}
	if _, ok := s.AgentScores["zero"]; ok {
		t.Errorf("zero-denominator agent must still be omitted from agent_scores")
	}
	want := CostInfo{}
	for _, ar := range []AgentResult{a, b, zero} {
		c := agentCostTotals(ar)
		want.TokensIn += c.TokensIn
		want.TokensOut += c.TokensOut
		want.EstimatedUSD += c.EstimatedUSD
	}
	costClose(t, "buildSummary total", s.TotalCost, want)
	if s.TotalCost.TokensIn != 32 {
		t.Errorf("total tokens_in = %d, want 32 (agent + judge)", s.TotalCost.TokensIn)
	}
}

// A summary.json written before verdicts existed still loads and still accepts
// a progressive save.
func TestSummaryWithoutAgentVerdictsStillLoads(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	legacy := `{"git_hash":"old","total_cost":{"tokens_in":5,"tokens_out":2,"estimated_usd":0.5},"agent_scores":{"architect":0.75}}`
	if err := os.WriteFile(file, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadSummary(file)
	if err != nil {
		t.Fatalf("loadSummary: %v", err)
	}
	if s.AgentVerdicts != nil || s.AgentScores["architect"] != 0.75 {
		t.Errorf("legacy summary = %+v", s)
	}
	if err := updateIncrementalSummary(file, verdictAgent("builder", 20, 20, CostInfo{TokensIn: 1}), "g"); err != nil {
		t.Fatal(err)
	}
	var after Summary
	readJSON(t, file, &after)
	if _, ok := after.AgentVerdicts["builder"]; !ok || after.AgentScores["architect"] != 0.75 {
		t.Errorf("after save = %+v", after)
	}
}

// The checked-in legacy result directories predate verdicts and must still diff.
func TestDiffLegacyResultDirectoriesStillLoad(t *testing.T) {
	repoResults := filepath.Join(packageDir, "..", "..", ".kairon", "evals", "results")
	runs := []string{"260620-200919-e369501", "260621-160207-8a19eb2"}
	for _, r := range runs {
		if _, err := os.Stat(filepath.Join(repoResults, r, "summary.json")); err != nil {
			t.Skipf("checked-in legacy result directory %s not present: %v", r, err)
		}
	}

	chdirTemp(t)
	dst := filepath.Join(".kairon", "evals", "results")
	for _, r := range runs {
		if err := os.CopyFS(filepath.Join(dst, r), os.DirFS(filepath.Join(repoResults, r))); err != nil {
			t.Fatal(err)
		}
		s, err := loadSummary(filepath.Join(dst, r, "summary.json"))
		if err != nil {
			t.Fatalf("loadSummary(%s): %v", r, err)
		}
		if s.AgentVerdicts != nil {
			t.Errorf("%s: legacy summary unexpectedly has agent_verdicts", r)
		}
	}
	var buf bytes.Buffer
	if err := diffTo(&buf, runs[0], runs[1]); err != nil {
		t.Fatalf("diffTo: %v", err)
	}
	if !strings.Contains(buf.String(), "Eval Diff:") {
		t.Errorf("diff output missing header:\n%s", buf.String())
	}
}
