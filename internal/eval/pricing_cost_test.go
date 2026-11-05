package eval

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// pricedPins pins a price table for the duration of a test.
func pricedPins(t *testing.T, table inference.PriceTable) {
	t.Helper()
	old := cfg.pins
	cfg.pins = &runPins{Prices: table}
	t.Cleanup(func() { cfg.pins = old })
}

var sonnetTable = inference.PriceTable{
	"sonnet-x": {InputUSDPerMTok: 3, OutputUSDPerMTok: 15},
	"cheap-y":  {InputUSDPerMTok: 1, OutputUSDPerMTok: 2},
}

// wantSonnetUSD is tokens x the sonnet-x price, evaluated at run time (as the
// implementation does) so float rounding matches.
func wantSonnetUSD(in, out int) float64 {
	return float64(in)*3/1e6 + float64(out)*15/1e6
}

func usage(in, out int) inference.Usage {
	return inference.Usage{InputTokens: in, OutputTokens: out, Source: inference.UsageReported}
}

func TestCostFromUsagePricedUsesModelPrice(t *testing.T) {
	pricedPins(t, sonnetTable)

	c := costFromUsage("cheap-y", usage(1_000_000, 1_000_000))
	if c.EstimatedUSD != 3.0 {
		t.Errorf("EstimatedUSD = %v, want 3 (1M*1 + 1M*2)", c.EstimatedUSD)
	}
	if c.CostIncomplete || len(c.UnpricedModels) != 0 {
		t.Errorf("priced call marked incomplete: %+v", c)
	}
	if c.Model != "cheap-y" || c.TokensIn != 1_000_000 || c.TokensOut != 1_000_000 {
		t.Errorf("got %+v", c)
	}

	s := costFromUsage("sonnet-x", usage(1000, 500))
	if want := wantSonnetUSD(1000, 500); s.EstimatedUSD != want {
		t.Errorf("EstimatedUSD = %v, want %v", s.EstimatedUSD, want)
	}
}

func TestCostFromUsageUnpricedIsIncomplete(t *testing.T) {
	pricedPins(t, sonnetTable)

	c := costFromUsage("haiku-z", usage(1000, 500))
	if c.EstimatedUSD != 0 {
		t.Errorf("EstimatedUSD = %v, want 0", c.EstimatedUSD)
	}
	if !c.CostIncomplete {
		t.Error("CostIncomplete = false, want true")
	}
	if !reflect.DeepEqual(c.UnpricedModels, []string{"haiku-z"}) {
		t.Errorf("UnpricedModels = %v, want [haiku-z]", c.UnpricedModels)
	}
	if c.TokensIn != 1000 || c.TokensOut != 500 {
		t.Errorf("tokens must still be counted: %+v", c)
	}

	// An empty model has no price and is not listed by name.
	e := costFromUsage("", usage(1, 1))
	if !e.CostIncomplete || len(e.UnpricedModels) != 0 {
		t.Errorf("empty model: %+v, want incomplete with no named models", e)
	}

	// No tokens means nothing was spent, so nothing is unknown.
	z := costFromUsage("haiku-z", inference.Usage{})
	if z.CostIncomplete {
		t.Errorf("zero-token call marked incomplete: %+v", z)
	}
}

func TestCostFromUsageUnpinnedPricesNothing(t *testing.T) {
	old := cfg.pins
	cfg.pins = nil
	t.Cleanup(func() { cfg.pins = old })

	c := costFromUsage("sonnet-x", usage(10, 10))
	if c.EstimatedUSD != 0 || !c.CostIncomplete {
		t.Errorf("unpinned run must not invent a price: %+v", c)
	}
}

func TestServedModel(t *testing.T) {
	tests := []struct {
		name      string
		req, resp string
		want      string
	}{
		{"response wins", "pinned", "served", "served"},
		{"falls back to request", "pinned", "", "pinned"},
		{"neither", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servedModel(inference.Request{Model: tt.req}, inference.Response{Model: tt.resp})
			if got != tt.want {
				t.Errorf("servedModel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCostInfoAddPropagatesIncomplete(t *testing.T) {
	pricedPins(t, sonnetTable)
	priced := costFromUsage("sonnet-x", usage(10, 10))
	unA := costFromUsage("b-model", usage(10, 10))
	unB := costFromUsage("a-model", usage(10, 10))

	t.Run("priced only stays unmarked", func(t *testing.T) {
		var c CostInfo
		c.Add(priced)
		c.Add(priced)
		if c.CostIncomplete || len(c.UnpricedModels) != 0 {
			t.Errorf("got %+v", c)
		}
	})
	t.Run("any unpriced marks and unions sorted deduped", func(t *testing.T) {
		var c CostInfo
		c.Add(priced)
		c.Add(unA)
		c.Add(unB)
		c.Add(unA)
		if !c.CostIncomplete {
			t.Error("want incomplete")
		}
		if want := []string{"a-model", "b-model"}; !reflect.DeepEqual(c.UnpricedModels, want) {
			t.Errorf("UnpricedModels = %v, want %v", c.UnpricedModels, want)
		}
		if c.EstimatedUSD != priced.EstimatedUSD {
			t.Errorf("unpriced calls must add 0 USD, got %v", c.EstimatedUSD)
		}
	})
	t.Run("addTotals sums and propagates without touching model or source", func(t *testing.T) {
		c := CostInfo{Model: "keep", UsageSource: "reported"}
		c.addTotals(unA)
		c.addTotals(priced)
		if c.Model != "keep" || c.UsageSource != "reported" {
			t.Errorf("addTotals changed Model/UsageSource: %+v", c)
		}
		if c.TokensIn != 20 || c.TokensOut != 20 || !c.CostIncomplete ||
			!reflect.DeepEqual(c.UnpricedModels, []string{"b-model"}) {
			t.Errorf("got %+v", c)
		}
	})
}

func TestNewCallRecordCostUnknownAndModelAgreement(t *testing.T) {
	pricedPins(t, sonnetTable)

	t.Run("unpriced call is cost_unknown with zero cost", func(t *testing.T) {
		req := inference.Request{Role: inference.RoleAgent, Model: "haiku-z"}
		resp := inference.Response{Usage: usage(100, 50)}
		cost := costFromUsage(servedModel(req, resp), resp.Usage)
		rec := newCallRecord(req, resp, nil, time.Second, cost)
		if !rec.CostUnknown || rec.CostUSD != 0 {
			t.Errorf("rec = %+v, want cost_unknown true and cost_usd 0", rec)
		}
		if rec.Model != "haiku-z" || cost.Model != rec.Model {
			t.Errorf("record model %q and priced model %q must be identical (haiku-z)", rec.Model, cost.Model)
		}
	})
	t.Run("priced call is known", func(t *testing.T) {
		req := inference.Request{Role: inference.RoleAgent}
		resp := inference.Response{Model: "sonnet-x", Usage: usage(1000, 500)}
		cost := costFromUsage(servedModel(req, resp), resp.Usage)
		rec := newCallRecord(req, resp, nil, time.Second, cost)
		if rec.CostUnknown || rec.CostUSD != wantSonnetUSD(1000, 500) {
			t.Errorf("rec = %+v", rec)
		}
		if rec.Model != cost.Model {
			t.Errorf("record model %q != priced model %q", rec.Model, cost.Model)
		}
	})
	t.Run("response without model prices under request model", func(t *testing.T) {
		req := inference.Request{Role: inference.RoleJudge, Model: "cheap-y"}
		resp := inference.Response{Usage: usage(1_000_000, 0)}
		cost := costFromUsage(servedModel(req, resp), resp.Usage)
		rec := newCallRecord(req, resp, nil, time.Second, cost)
		if rec.Model != "cheap-y" || cost.Model != "cheap-y" || rec.CostUSD != 1 || rec.CostUnknown {
			t.Errorf("rec = %+v cost = %+v", rec, cost)
		}
	})
}

func incompleteCase(name string, models ...string) CaseResult {
	return CaseResult{
		CaseName:     name,
		ActualOutput: "out",
		Scores:       []CriterionScore{{Name: "x", Score: 5, MaxScore: 5}},
		AgentCost:    CostInfo{TokensIn: 1, TokensOut: 1, CostIncomplete: len(models) > 0, UnpricedModels: models},
		JudgeCost:    CostInfo{TokensIn: 1, TokensOut: 1, EstimatedUSD: 0.5},
	}
}

func TestAgentCostTotalsAndVerdictPropagateIncomplete(t *testing.T) {
	clean := incompleteCase("a")
	un1 := incompleteCase("b", "m2")
	un2 := incompleteCase("c", "m1")

	t.Run("no unpriced call leaves totals unmarked", func(t *testing.T) {
		got := agentCostTotals(AgentResult{Agent: "x", Cases: []CaseResult{clean, clean}})
		if got.CostIncomplete || len(got.UnpricedModels) != 0 || got.EstimatedUSD != 1.0 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("any unpriced case marks agent cost", func(t *testing.T) {
		got := agentCostTotals(AgentResult{Agent: "x", Cases: []CaseResult{clean, un1, un2}})
		if !got.CostIncomplete || !reflect.DeepEqual(got.UnpricedModels, []string{"m1", "m2"}) {
			t.Errorf("got %+v", got)
		}
		if got.TokensIn != 6 {
			t.Errorf("tokens must still sum: %+v", got)
		}
	})
	t.Run("a judge-side unpriced model is carried too", func(t *testing.T) {
		c := clean
		c.JudgeCost = CostInfo{CostIncomplete: true, UnpricedModels: []string{"judge-m"}}
		got := agentCostTotals(AgentResult{Agent: "x", Cases: []CaseResult{c}})
		if !got.CostIncomplete || !reflect.DeepEqual(got.UnpricedModels, []string{"judge-m"}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("buildSummary total is incomplete iff any verdict is", func(t *testing.T) {
		s := buildSummary([]AgentResult{
			{Agent: "a1", Cases: []CaseResult{clean}},
			{Agent: "a2", Cases: []CaseResult{un1}},
		}, "g")
		if s.AgentVerdicts["a1"].Cost.CostIncomplete {
			t.Error("a1 should be unmarked")
		}
		if !s.AgentVerdicts["a2"].Cost.CostIncomplete {
			t.Error("a2 should be marked")
		}
		if !s.TotalCost.CostIncomplete || !reflect.DeepEqual(s.TotalCost.UnpricedModels, []string{"m2"}) {
			t.Errorf("TotalCost = %+v", s.TotalCost)
		}

		clean := buildSummary([]AgentResult{{Agent: "a1", Cases: []CaseResult{clean}}}, "g")
		if clean.TotalCost.CostIncomplete || len(clean.TotalCost.UnpricedModels) != 0 {
			t.Errorf("clean TotalCost = %+v", clean.TotalCost)
		}
	})
	t.Run("progressive saves stay idempotent", func(t *testing.T) {
		var s Summary
		ar := AgentResult{Agent: "a2", Cases: []CaseResult{un1}}
		setAgentVerdict(&s, ar)
		setAgentVerdict(&s, ar)
		if !reflect.DeepEqual(s.TotalCost.UnpricedModels, []string{"m2"}) {
			t.Errorf("TotalCost = %+v", s.TotalCost)
		}
	})
}

func TestPrintAgentVerdictCostIncompleteMarker(t *testing.T) {
	var buf bytes.Buffer
	printAgentVerdict(&buf, "a", AgentVerdict{Passed: true, Score: 100, Threshold: 95, CasesTotal: 1})
	if strings.Contains(buf.String(), "cost incomplete") {
		t.Errorf("unmarked verdict has marker: %q", buf.String())
	}

	buf.Reset()
	printAgentVerdict(&buf, "a", AgentVerdict{
		Passed: false, Score: 10, Threshold: 95, CasesTotal: 1, CasesFailed: 1,
		Cost: CostInfo{CostIncomplete: true, UnpricedModels: []string{"haiku-z", "m2"}},
	})
	line := buf.String()
	if !strings.HasPrefix(line, "FAIL ") {
		t.Errorf("line must start with FAIL: %q", line)
	}
	if !strings.Contains(line, "[cost incomplete: no price for haiku-z, m2]") {
		t.Errorf("missing marker: %q", line)
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("must stay a single line: %q", line)
	}
}

func TestDiffMarksIncompleteCost(t *testing.T) {
	chdirTemp(t)
	t.Cleanup(resetConfig)
	sum := func(incomplete bool) Summary {
		s := provSummary("", map[string]float64{"a1": 0.9})
		s.TotalCost = CostInfo{TokensIn: 10, TokensOut: 10, EstimatedUSD: 0.5}
		if incomplete {
			s.TotalCost.CostIncomplete = true
			s.TotalCost.UnpricedModels = []string{"haiku-z"}
		}
		return s
	}
	writeProvRun(t, "run-a", provRun{summary: sum(false)})
	writeProvRun(t, "run-b", provRun{summary: sum(true)})
	writeProvRun(t, "run-c", provRun{summary: sum(false)})

	var out bytes.Buffer
	if err := diffTo(&out, "run-a", "run-b"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "(incomplete: run-b)") {
		t.Errorf("Cost line lacks side marker:\n%s", text)
	}
	if !strings.Contains(text, "quality/$ (cost incomplete)") && strings.Count(text, "(cost incomplete)") < 1 {
		t.Errorf("quality/$ lines lack marker:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "quality/$") && !strings.Contains(line, "(cost incomplete)") {
			t.Errorf("quality/$ line lacks marker when either run is incomplete: %q", line)
		}
	}

	out.Reset()
	if err := diffTo(&out, "run-a", "run-c"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "incomplete") {
		t.Errorf("clean runs must not be annotated:\n%s", out.String())
	}

	out.Reset()
	if err := diffTo(&out, "run-b", "run-b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(incomplete: run-b, run-b)") {
		t.Errorf("both sides should be named:\n%s", out.String())
	}
}

func TestCalculateAgentJudgeCostsPropagatesIncomplete(t *testing.T) {
	chdirTemp(t)
	t.Cleanup(resetConfig)
	un := incompleteCase("b", "haiku-z")
	writeProvRun(t, "run-a", provRun{
		summary: provSummary("", map[string]float64{"a1": 1}),
		agents:  map[string]AgentResult{"a1": {Agent: "a1", Cases: []CaseResult{incompleteCase("a"), un}}},
	})
	agent, judge := calculateAgentJudgeCosts("run-a", evalsPath("results"))
	if !agent.CostIncomplete || !reflect.DeepEqual(agent.UnpricedModels, []string{"haiku-z"}) {
		t.Errorf("agent = %+v", agent)
	}
	if judge.CostIncomplete || judge.EstimatedUSD != 1.0 {
		t.Errorf("judge = %+v", judge)
	}
}

// TestLegacyResultJSONStillLoadsAndDiffs pins backwards compatibility: files
// written before cost_incomplete / unpriced_models / cost_unknown existed.
func TestLegacyResultJSONStillLoadsAndDiffs(t *testing.T) {
	const legacySummary = `{"git_hash":"abc","agent_scores":{"a1":0.8},` +
		`"total_cost":{"tokens_in":100,"tokens_out":50,"estimated_usd":0.00105,"usage_source":"estimated"}}`
	const legacyAgent = `{"agent":"a1","git_hash":"abc","cases":[{"case_name":"c1","actual_output":"o",` +
		`"scores":[{"name":"x","score":4,"max_score":5}],` +
		`"agent_cost":{"tokens_in":60,"tokens_out":30,"estimated_usd":0.00063,"model":"m"},` +
		`"judge_cost":{"tokens_in":40,"tokens_out":20,"estimated_usd":0.00042},` +
		`"calls":[{"role":"agent","model":"m","input_tokens":60,"output_tokens":30,"cost_usd":0.00063,"estimated":true,"duration_ms":1}]}]}`

	var s Summary
	if err := json.Unmarshal([]byte(legacySummary), &s); err != nil {
		t.Fatalf("legacy summary: %v", err)
	}
	if s.TotalCost.CostIncomplete || len(s.TotalCost.UnpricedModels) != 0 || s.TotalCost.EstimatedUSD != 0.00105 {
		t.Errorf("TotalCost = %+v", s.TotalCost)
	}
	var ar AgentResult
	if err := json.Unmarshal([]byte(legacyAgent), &ar); err != nil {
		t.Fatalf("legacy agent result: %v", err)
	}
	if ar.Cases[0].AgentCost.CostIncomplete || ar.Cases[0].Calls[0].CostUnknown {
		t.Errorf("legacy data must read as known cost: %+v", ar.Cases[0])
	}

	chdirTemp(t)
	t.Cleanup(resetConfig)
	writeProvRun(t, "old-a", provRun{raw: map[string]string{"summary.json": legacySummary, "a1.json": legacyAgent}})
	writeProvRun(t, "old-b", provRun{raw: map[string]string{"summary.json": legacySummary, "a1.json": legacyAgent}})
	var out bytes.Buffer
	if err := diffTo(&out, "old-a", "old-b"); err != nil {
		t.Fatalf("diff on legacy runs: %v", err)
	}
	if !strings.Contains(out.String(), "Cost:") || strings.Contains(out.String(), "incomplete") {
		t.Errorf("diff output:\n%s", out.String())
	}
}

func TestPinRunLoadsPricesAndRefusesInvalidPricing(t *testing.T) {
	t.Run("loads default prices", func(t *testing.T) {
		setupPinProject(t, "", "")
		if err := pinRun("architect", RunOptions{}, false); err != nil {
			t.Fatal(err)
		}
		if got := cfg.pins.prices().Cost("claude-sonnet-5.5", usage(1_000_000, 0)); !got.Known || got.USD != 3 {
			t.Errorf("sonnet cost = %+v, want known $3", got)
		}
		if got := cfg.pins.prices().Cost("claude-haiku-4.5", usage(1, 1)); got.Known {
			t.Errorf("haiku must be unpriced by default: %+v", got)
		}
	})
	t.Run("config pricing replaces defaults", func(t *testing.T) {
		setupPinProject(t, "", "pricing:\n  claude-haiku-4.5:\n    input_usd_per_mtok: 1\n    output_usd_per_mtok: 5\n")
		if err := pinRun("architect", RunOptions{}, false); err != nil {
			t.Fatal(err)
		}
		if got := cfg.pins.prices().Cost("claude-haiku-4.5", usage(1_000_000, 0)); !got.Known || got.USD != 1 {
			t.Errorf("haiku cost = %+v", got)
		}
		if got := cfg.pins.prices().Cost("claude-sonnet-5.5", usage(1, 1)); got.Known {
			t.Errorf("sonnet should be unpriced once the block replaces defaults: %+v", got)
		}
	})
	t.Run("invalid pricing refuses the run before any case", func(t *testing.T) {
		setupPinProject(t, "", "pricing:\n  claude-sonnet-5.5:\n    input_usd_per_mtok: -1\n    output_usd_per_mtok: 15\n")
		_, calls := installFakeKiroCLI(t, pinFakeKiroBody)
		err := RunWithOptions("architect", "c1", RunOptions{})
		if err == nil || !strings.Contains(err.Error(), "pricing") || !strings.Contains(err.Error(), "claude-sonnet-5.5") {
			t.Fatalf("err = %v, want a pricing refusal naming the model", err)
		}
		if got := readCalls(t, calls); len(got) != 0 {
			t.Errorf("no case may start, saw calls: %v", got)
		}
		if cfg.pins != nil {
			t.Errorf("pins must not be set on refusal: %+v", cfg.pins)
		}
	})
}
