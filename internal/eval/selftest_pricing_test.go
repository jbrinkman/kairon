package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

// Self-test cases that pin per-model pricing end to end (issue #340, AC 2-4).
const (
	priceSonnetCase  = "stub-price-sonnet"
	priceUnknownCase = "stub-price-unknown"
)

// selftestPriceRun is the evidence a single-case stub run leaves behind.
type selftestPriceRun struct {
	Case    CaseResult
	Verdict AgentVerdict
	Total   CostInfo
	// RawTotal is summary.json's total_cost as written, to pin field names.
	RawTotal map[string]any
}

// runSelftestPriceCase runs one selftest case on the stub backend and reads
// back <agent>.json and summary.json.
func runSelftestPriceCase(t *testing.T, caseName string) selftestPriceRun {
	t.Helper()
	t.Cleanup(resetConfig)

	evalsDir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, evalsDir)
	resultsDir := filepath.Join(evalsDir, "results")

	err := RunWithOptions("selftest", caseName, RunOptions{Backend: "stub", EvalsDir: evalsDir, NoSandbox: true})
	if err != nil {
		t.Fatalf("stub run of selftest %s failed: %v", caseName, err)
	}
	added := newRunDirs(t, resultsDir, map[string]bool{})
	if len(added) != 1 {
		t.Fatalf("run of %s created %d result dirs, want 1: %v", caseName, len(added), added)
	}
	runDir := filepath.Join(resultsDir, added[0])

	res := readSelfTestResult(t, filepath.Join(runDir, "selftest.json"))
	if len(res.Cases) != 1 || res.Cases[0].CaseName != caseName {
		t.Fatalf("results = %+v, want exactly case %s", res.Cases, caseName)
	}

	data, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
	if err != nil {
		t.Fatalf("reading summary.json: %v", err)
	}
	var sum Summary
	if err := json.Unmarshal(data, &sum); err != nil {
		t.Fatalf("parsing summary.json: %v", err)
	}
	var raw struct {
		TotalCost map[string]any `json:"total_cost"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return selftestPriceRun{
		Case:     res.Cases[0],
		Verdict:  sum.AgentVerdicts["selftest"],
		Total:    sum.TotalCost,
		RawTotal: raw.TotalCost,
	}
}

// priceAgentCall returns the single agent call record of a case.
func priceAgentCall(t *testing.T, c CaseResult) inference.CallRecord {
	t.Helper()
	var agent []inference.CallRecord
	for _, call := range c.Calls {
		if call.Role == "agent" {
			agent = append(agent, call)
		}
	}
	if len(agent) != 1 {
		t.Fatalf("case %s has %d agent calls, want 1: %+v", c.CaseName, len(agent), c.Calls)
	}
	return agent[0]
}

// rawAgentCall marshals the agent call so the written JSON field names are
// what is asserted (cost_unknown is omitted entirely for a known cost).
func rawAgentCall(t *testing.T, c CaseResult) map[string]any {
	t.Helper()
	data, err := json.Marshal(priceAgentCall(t, c))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// nearUSD compares dollar amounts that come from float64 arithmetic
// (1000*3/1e6 + 500*15/1e6 is 0.010499999999999999, not 0.0105).
func nearUSD(got, want float64) bool {
	d := got - want
	return d < 1e-12 && d > -1e-12
}

// AC 2: a configured model is priced at its price x tokens.
func TestSelftestPriceSonnetIsPricedPerModel(t *testing.T) {
	r := runSelftestPriceCase(t, priceSonnetCase)

	call := priceAgentCall(t, r.Case)
	if call.Model != "claude-sonnet-5.5" || call.InputTokens != 1000 || call.OutputTokens != 500 {
		t.Errorf("agent call = %s %d/%d, want claude-sonnet-5.5 1000/500", call.Model, call.InputTokens, call.OutputTokens)
	}
	if !nearUSD(call.CostUSD, 0.0105) {
		t.Errorf("agent call cost_usd = %v, want 0.0105", call.CostUSD)
	}
	if call.CostUnknown {
		t.Errorf("agent call cost_unknown = true, want false: %+v", call)
	}
	if _, present := rawAgentCall(t, r.Case)["cost_unknown"]; present {
		t.Errorf("a known cost must omit cost_unknown from the call record")
	}

	ac := r.Case.AgentCost
	if !nearUSD(ac.EstimatedUSD, 0.0105) {
		t.Errorf("agent_cost.estimated_usd = %v, want 0.0105", ac.EstimatedUSD)
	}
	if ac.CostIncomplete || len(ac.UnpricedModels) != 0 {
		t.Errorf("agent_cost marked incomplete: %+v", ac)
	}
	if ac.UsageSource != "reported" {
		t.Errorf("agent_cost.usage_source = %q, want reported", ac.UsageSource)
	}

	// The stub judge reports the pinned (priced) judge model, so nothing in
	// the run is unpriced: AC 4's negative case.
	if r.Case.JudgeCost.CostIncomplete {
		t.Errorf("judge_cost marked incomplete: %+v", r.Case.JudgeCost)
	}
	if r.Verdict.Cost.CostIncomplete {
		t.Errorf("agent verdict cost marked incomplete: %+v", r.Verdict.Cost)
	}
	if r.Total.CostIncomplete || len(r.Total.UnpricedModels) != 0 {
		t.Errorf("summary total_cost marked incomplete: %+v", r.Total)
	}
	if _, present := r.RawTotal["cost_incomplete"]; present {
		t.Errorf("summary total_cost must omit cost_incomplete when complete: %v", r.RawTotal)
	}
}

// AC 3: a model with no configured price records an unknown cost, not zero
// dollars and not Sonnet's price.
func TestSelftestPriceUnknownMarksCostUnknown(t *testing.T) {
	r := runSelftestPriceCase(t, priceUnknownCase)

	call := priceAgentCall(t, r.Case)
	if call.Model != "claude-haiku-4.5" || call.InputTokens != 1000 || call.OutputTokens != 500 {
		t.Errorf("agent call = %s %d/%d, want claude-haiku-4.5 1000/500", call.Model, call.InputTokens, call.OutputTokens)
	}
	if !call.CostUnknown || call.CostUSD != 0 {
		t.Errorf("agent call cost = %v unknown=%v, want 0 and cost_unknown", call.CostUSD, call.CostUnknown)
	}
	if v, ok := rawAgentCall(t, r.Case)["cost_unknown"]; !ok || v != true {
		t.Errorf("agent call JSON cost_unknown = %v (present=%v), want true", v, ok)
	}

	ac := r.Case.AgentCost
	if ac.EstimatedUSD != 0 || !ac.CostIncomplete {
		t.Errorf("agent_cost = %+v, want 0 USD and cost_incomplete", ac)
	}
	if want := []string{"claude-haiku-4.5"}; !reflect.DeepEqual(ac.UnpricedModels, want) {
		t.Errorf("agent_cost.unpriced_models = %v, want %v", ac.UnpricedModels, want)
	}
	// Tokens are still reported even though the dollars are unknown.
	if ac.TokensIn != 1000 || ac.TokensOut != 500 {
		t.Errorf("agent_cost tokens = %d/%d, want 1000/500", ac.TokensIn, ac.TokensOut)
	}
}

// AC 4: the run total says it is a lower bound when any call was unpriced.
func TestSelftestPriceUnknownMakesSummaryTotalIncomplete(t *testing.T) {
	r := runSelftestPriceCase(t, priceUnknownCase)

	if !r.Total.CostIncomplete {
		t.Errorf("summary total_cost.cost_incomplete = false, want true: %+v", r.Total)
	}
	if want := []string{"claude-haiku-4.5"}; !reflect.DeepEqual(r.Total.UnpricedModels, want) {
		t.Errorf("summary total_cost.unpriced_models = %v, want %v", r.Total.UnpricedModels, want)
	}
	if v, ok := r.RawTotal["cost_incomplete"]; !ok || v != true {
		t.Errorf("summary.json total_cost.cost_incomplete = %v (present=%v), want true", v, ok)
	}
	if !r.Verdict.Cost.CostIncomplete {
		t.Errorf("agent verdict cost.cost_incomplete = false, want true: %+v", r.Verdict.Cost)
	}
	// Judge calls ran on the priced pinned model: only the agent is unpriced.
	if r.Case.JudgeCost.CostIncomplete {
		t.Errorf("judge_cost marked incomplete: %+v", r.Case.JudgeCost)
	}
}

// The two new cases must be loadable stub scripts, so the container/parity
// helpers that read selftest fixtures by name (loadSelftestStub) can use them.
func TestSelftestPriceFixturesDefineStubUsage(t *testing.T) {
	for name, want := range map[string]struct {
		model string
		in    int
		out   int
	}{
		priceSonnetCase:  {"claude-sonnet-5.5", 1000, 500},
		priceUnknownCase: {"claude-haiku-4.5", 1000, 500},
	} {
		stub := loadSelftestStub(t, name)
		if len(stub.Turns) != 1 {
			t.Fatalf("%s has %d stub turns, want 1", name, len(stub.Turns))
		}
		turn := stub.Turns[0]
		if turn.Model != want.model || turn.Usage == nil ||
			turn.Usage.InputTokens != want.in || turn.Usage.OutputTokens != want.out {
			t.Errorf("%s turn = model %q usage %+v, want %s %d/%d", name, turn.Model, turn.Usage, want.model, want.in, want.out)
		}
	}
}
