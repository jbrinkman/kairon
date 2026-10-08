package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// verdictAgent builds an agent result with one case per entry in scores
// ({score, max} pairs), each carrying the given agent and judge cost.
func verdictAgent(name string, agentCost, judgeCost CostInfo, scores ...[2]int) AgentResult {
	ar := AgentResult{Agent: name, Threshold: floatPtr(50)}
	for i, s := range scores {
		ar.Cases = append(ar.Cases, CaseResult{
			CaseName:  string(rune('a' + i)),
			Scores:    []CriterionScore{{Name: "q", Score: s[0], MaxScore: s[1]}},
			AgentCost: agentCost,
			JudgeCost: judgeCost,
		})
	}
	return ar
}

func readSummary(t *testing.T, file string) (Summary, map[string]json.RawMessage) {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var s Summary
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw summary: %v", err)
	}
	return s, raw
}

func TestSummaryRecordsAgentVerdict(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	ar := verdictAgent("alpha",
		CostInfo{TokensIn: 10, TokensOut: 5, EstimatedUSD: 0.5},
		CostInfo{TokensIn: 1, TokensOut: 2, EstimatedUSD: 0.25},
		[2]int{9, 10}, [2]int{2, 10})
	if err := updateIncrementalSummary(file, ar, "g"); err != nil {
		t.Fatal(err)
	}
	s, _ := readSummary(t, file)

	v, ok := s.AgentVerdicts["alpha"]
	if !ok {
		t.Fatalf("agent_verdicts missing alpha: %+v", s.AgentVerdicts)
	}
	want := agentVerdict(ar)
	if v != want {
		t.Errorf("verdict = %+v, want %+v", v, want)
	}
	if v.Score != 55 || v.Threshold != 50 || !v.Passed || v.CasesTotal != 2 || v.CasesFailed != 1 {
		t.Errorf("verdict fields = %+v, want score 55 threshold 50 passed cases 2 failed 1", v)
	}
	if v.Cost.TokensIn != 22 || v.Cost.TokensOut != 14 || v.Cost.EstimatedUSD != 1.5 {
		t.Errorf("verdict cost = %+v", v.Cost)
	}
}

func TestSummaryTotalCostSumsAgents(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	a := verdictAgent("a", CostInfo{TokensIn: 10, TokensOut: 4, EstimatedUSD: 1}, CostInfo{}, [2]int{1, 1})
	b := verdictAgent("b", CostInfo{TokensIn: 7, TokensOut: 3, EstimatedUSD: 2}, CostInfo{TokensIn: 1, TokensOut: 1, EstimatedUSD: 0.5}, [2]int{1, 1})
	for _, ar := range []AgentResult{a, b} {
		if err := updateIncrementalSummary(file, ar, "g"); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := readSummary(t, file)
	want := CostInfo{TokensIn: 18, TokensOut: 8, EstimatedUSD: 3.5}
	if s.TotalCost != want {
		t.Errorf("total_cost = %+v, want %+v", s.TotalCost, want)
	}
}

func TestSummaryTotalCostNoDoubleCountOnRepeatedSave(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	cost := CostInfo{TokensIn: 10, TokensOut: 4, EstimatedUSD: 1}
	b := verdictAgent("b", CostInfo{TokensIn: 5, TokensOut: 5, EstimatedUSD: 2}, CostInfo{}, [2]int{1, 1})
	if err := updateIncrementalSummary(file, b, "g"); err != nil {
		t.Fatal(err)
	}
	// Agent "a" is saved after every case with the whole result so far.
	for n := 1; n <= 3; n++ {
		ar := AgentResult{Agent: "a", Threshold: floatPtr(50)}
		for i := 0; i < n; i++ {
			ar.Cases = append(ar.Cases, CaseResult{
				CaseName:  string(rune('a' + i)),
				Scores:    []CriterionScore{{Name: "q", Score: 1, MaxScore: 1}},
				AgentCost: cost,
			})
		}
		if err := updateIncrementalSummary(file, ar, "g"); err != nil {
			t.Fatal(err)
		}
		// Saving the same snapshot twice must not change anything either.
		if err := updateIncrementalSummary(file, ar, "g"); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := readSummary(t, file)
	want := CostInfo{TokensIn: 3*10 + 5, TokensOut: 3*4 + 5, EstimatedUSD: 3*1 + 2}
	if s.TotalCost != want {
		t.Errorf("total_cost = %+v, want %+v", s.TotalCost, want)
	}
}

func TestSummaryTotalCostJSONShape(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	ar := verdictAgent("a", CostInfo{TokensIn: 1, TokensOut: 1, EstimatedUSD: 1, Model: "m", UsageSource: "reported"}, CostInfo{}, [2]int{1, 1})
	if err := updateIncrementalSummary(file, ar, "g"); err != nil {
		t.Fatal(err)
	}
	_, raw := readSummary(t, file)
	var total map[string]json.RawMessage
	if err := json.Unmarshal(raw["total_cost"], &total); err != nil {
		t.Fatal(err)
	}
	for k := range total {
		if k != "tokens_in" && k != "tokens_out" && k != "estimated_usd" {
			t.Errorf("total_cost has unexpected key %q", k)
		}
	}
	if len(total) != 3 {
		t.Errorf("total_cost keys = %v, want tokens_in, tokens_out, estimated_usd", total)
	}
	var verdicts map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw["agent_verdicts"], &verdicts); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"score", "threshold", "passed", "cases_total", "cases_failed", "cost"} {
		if _, ok := verdicts["a"][k]; !ok {
			t.Errorf("agent_verdicts.a missing %q", k)
		}
	}
}

func TestSummaryAgentScoreFractionAndZeroDenominator(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	zero := AgentResult{Agent: "zero", Cases: []CaseResult{{CaseName: "c", Scores: []CriterionScore{{Name: "a"}}}}}
	scored := verdictAgent("scored", CostInfo{}, CostInfo{}, [2]int{1, 2})
	for _, ar := range []AgentResult{zero, scored} {
		if err := updateIncrementalSummary(file, ar, "g"); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := readSummary(t, file)
	if _, ok := s.AgentScores["zero"]; ok {
		t.Errorf("agent_scores has zero-denominator agent: %v", s.AgentScores)
	}
	if got := s.AgentScores["scored"]; got != 0.5 {
		t.Errorf("agent_scores[scored] = %v, want 0.5 (fraction)", got)
	}
	// The zero-denominator agent still gets a (failing) verdict.
	if v, ok := s.AgentVerdicts["zero"]; !ok || v.Passed {
		t.Errorf("verdict for zero-denominator agent = %+v, ok=%v; want present and failed", v, ok)
	}
}

func TestSummaryLegacyWithoutVerdictsLoadsAndUpdates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	legacy := `{"git_hash":"old","total_cost":{"tokens_in":100,"tokens_out":50,"estimated_usd":9},"agent_scores":{"old":0.8}}`
	if err := os.WriteFile(file, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	var s Summary
	if err := json.Unmarshal([]byte(legacy), &s); err != nil || s.AgentVerdicts != nil {
		t.Fatalf("legacy unmarshal: err=%v verdicts=%v", err, s.AgentVerdicts)
	}
	ar := verdictAgent("new", CostInfo{TokensIn: 3, TokensOut: 2, EstimatedUSD: 1}, CostInfo{}, [2]int{1, 1})
	if err := updateIncrementalSummary(file, ar, "g"); err != nil {
		t.Fatal(err)
	}
	got, _ := readSummary(t, file)
	if got.AgentScores["old"] != 0.8 {
		t.Errorf("legacy agent score lost: %v", got.AgentScores)
	}
	if _, ok := got.AgentVerdicts["new"]; !ok {
		t.Errorf("verdict for new agent missing: %+v", got.AgentVerdicts)
	}
	if got.TotalCost != (CostInfo{TokensIn: 3, TokensOut: 2, EstimatedUSD: 1}) {
		t.Errorf("total_cost = %+v, want the sum of recorded verdict costs", got.TotalCost)
	}
}

func TestBuildSummaryRecordsVerdictsAndSumsCost(t *testing.T) {
	a := verdictAgent("a", CostInfo{TokensIn: 4, TokensOut: 1, EstimatedUSD: 1}, CostInfo{TokensIn: 1, TokensOut: 1, EstimatedUSD: 1}, [2]int{1, 1})
	b := verdictAgent("b", CostInfo{TokensIn: 6, TokensOut: 2, EstimatedUSD: 2}, CostInfo{}, [2]int{0, 1})
	s := buildSummary([]AgentResult{a, b}, "g")
	if len(s.AgentVerdicts) != 2 || !s.AgentVerdicts["a"].Passed || s.AgentVerdicts["b"].Passed {
		t.Errorf("verdicts = %+v", s.AgentVerdicts)
	}
	if want := (CostInfo{TokensIn: 11, TokensOut: 4, EstimatedUSD: 4}); s.TotalCost != want {
		t.Errorf("total_cost = %+v, want %+v", s.TotalCost, want)
	}
}
