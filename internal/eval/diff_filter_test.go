package eval

import (
	"bytes"
	"strings"
	"testing"
)

// filterCase builds a recorded case result with one "acc" criterion.
func filterCase(name string, meta CaseMetadata, score, max int, output string, agentTok, judgeTok int, usd float64) CaseResult {
	return CaseResult{
		CaseName:     name,
		Metadata:     meta,
		ActualOutput: output,
		Scores:       []CriterionScore{{Name: "acc", Score: score, MaxScore: max}},
		AgentCost:    CostInfo{TokensIn: agentTok, TokensOut: agentTok, EstimatedUSD: usd},
		JudgeCost:    CostInfo{TokensIn: judgeTok, TokensOut: judgeTok},
	}
}

var (
	metaFast = CaseMetadata{"speed": "fast", "skill": "cli"}
	metaSlow = CaseMetadata{"speed": "slow"}
)

// writeFilterRuns writes runs "r1" and "r2" for agent a1. The summary totals
// are deliberately far from the sum of the case costs, so a filtered diff that
// read them would show it.
func writeFilterRuns(t *testing.T, mf, ms CaseMetadata) {
	t.Helper()
	sum := func(score float64) Summary {
		s := provSummary("native", map[string]float64{"a1": score})
		s.TotalCost = CostInfo{TokensIn: 9000, TokensOut: 9000, EstimatedUSD: 90}
		return s
	}
	writeProvRun(t, "r1", provRun{summary: sum(0.75), agents: map[string]AgentResult{"a1": {Agent: "a1", Cases: []CaseResult{
		filterCase("fast-1", mf, 2, 4, "old fast", 10, 1, 0.01),
		filterCase("slow-1", ms, 4, 4, "same", 100, 0, 1),
	}}}})
	writeProvRun(t, "r2", provRun{summary: sum(0.5), agents: map[string]AgentResult{"a1": {Agent: "a1", Cases: []CaseResult{
		filterCase("fast-1", mf, 4, 4, "new fast", 20, 2, 0.02),
		filterCase("slow-1", ms, 0, 4, "same", 500, 0, 5),
	}}}})
}

func runDiffFiltered(t *testing.T, a, b string, exprs ...string) string {
	t.Helper()
	filters, err := ParseFilters(exprs)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := diffFiltered(&buf, a, b, filters); err != nil {
		t.Fatalf("diffFiltered: %v", err)
	}
	return buf.String()
}

func requireContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func requireAbsent(t *testing.T, out string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(out, bad) {
			t.Errorf("output must not contain %q:\n%s", bad, out)
		}
	}
}

// Without filters and without metadata the output is exactly what it was
// before filtering existed.
func TestDiffUnfilteredWithoutMetadataIsUnchanged(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, nil, nil)

	var buf bytes.Buffer
	if err := diffTo(&buf, "r1", "r2"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	requireAbsent(t, out, "Filter:", "Cases:", "[speed", "[skill")
	requireContains(t, out,
		"a1: 0.750 → 0.500  ↓ -0.250\n",
		"  acc                             0.750 → 0.500  ↓ -0.250\n",
		"    Case 'fast-1' output changed:\n",
		"  Tokens: +0\n",
		"  Cost:   +0.000000 USD\n",
	)
	// diffFiltered with no filters is the same code path.
	if got := runDiffFiltered(t, "r1", "r2"); got != out {
		t.Errorf("diffFiltered(nil) differs from diffTo:\n%s\nvs\n%s", got, out)
	}
}

func TestDiffFilterReportsOnlyMatchingCases(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, metaFast, metaSlow)

	out := runDiffFiltered(t, "r1", "r2", "speed==fast")

	requireContains(t, out,
		"Filter: speed==fast\n",
		// fast-1 only: 2/4 -> 4/4 (the summary says 0.750 -> 0.500).
		"a1: 0.500 → 1.000  ↑ +0.500\n",
		"acc                             0.500 → 1.000  ↑ +0.500",
		"Case 'fast-1' output changed:",
		// Costs from the fast case alone: agent 30 -> 60, judge 2 -> 4.
		"  Tokens: +22\n",
		"  Cost:   +0.010000 USD\n",
		"  Agent tokens:  20 → 40 (+20)\n",
		"  Judge tokens:  2 → 4 (+2)\n",
		// Quality per dollar from the filtered cost: 0.500 / 0.01 = 50.00 and
		// 1.000 / 0.02 = 50.00.
		"  r1: 50.00 quality/$\n",
		"  r2: 50.00 quality/$\n",
		// The Cases block lists only the matching case.
		"  Cases:\n",
		"    fast-1 [skill=cli, speed=fast]  0.500 → 1.000\n",
	)
	requireAbsent(t, out, "slow-1", "0.750", "+0.250")
}

func TestDiffFilterCostSectionsIgnoreSummaryTotals(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, metaFast, metaSlow)

	out := runDiffFiltered(t, "r1", "r2", "speed==slow")

	// slow-1: 4/4 -> 0/4, agent tokens 200 -> 1000, judge 0.
	requireContains(t, out,
		"a1: 1.000 → 0.000  ↓ -1.000\n",
		"  Tokens: +800\n",
		"  Cost:   +4.000000 USD\n",
		"  Agent tokens:  200 → 1000 (+800)\n",
	)
	requireAbsent(t, out, "fast-1", "90.0", "+18000")
}

func TestDiffFilterSideWithNoMatchesPrintsDash(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, metaFast, metaSlow)
	// r3 holds one extra case that r1 does not have.
	writeProvRun(t, "r3", provRun{summary: provSummary("native", map[string]float64{"a1": 1}), agents: map[string]AgentResult{"a1": {Agent: "a1", Cases: []CaseResult{
		filterCase("extra", CaseMetadata{"speed": "medium"}, 4, 4, "x", 1, 1, 0),
	}}}})

	out := runDiffFiltered(t, "r1", "r3", "speed==medium")

	requireContains(t, out, "a1: - → 1.000\n")
	requireAbsent(t, out, "↑", "↓", "output changed")
}

func TestDiffFilterBuiltInAgentAndNameKeys(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, metaFast, metaSlow)

	byName := runDiffFiltered(t, "r1", "r2", "name==fast-1")
	requireContains(t, byName, "a1: 0.500 → 1.000  ↑ +0.500\n")

	byAgent := runDiffFiltered(t, "r1", "r2", "agent==a1", "speed==fast")
	requireContains(t, byAgent, "Filter: agent==a1 AND speed==fast\n", "a1: 0.500 → 1.000")

	// A filter that matches nothing on either side omits the agent but still
	// prints the filter line and the (zero) cost delta.
	t.Run("no-match-agent-key", func(t *testing.T) {
		out := runDiffFiltered(t, "r1", "r2", "agent==zz")
		requireContains(t, out, "Filter: agent==zz\n", "Cost Delta:", "  Tokens: +0\n")
		requireAbsent(t, out, "a1:", "acc")
	})
	t.Run("no-match-metadata-key", func(t *testing.T) {
		out := runDiffFiltered(t, "r1", "r2", "speed==nothing")
		requireContains(t, out, "Filter: speed==nothing\n", "Cost Delta:", "  Tokens: +0\n")
		requireAbsent(t, out, "a1:", "acc")
	})
}

// Results saved before metadata existed match only agent and name filters.
func TestDiffFilterResultsWithoutMetadataMatchOnlyBuiltIns(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, nil, nil)

	byMeta := runDiffFiltered(t, "r1", "r2", "speed==fast")
	requireAbsent(t, byMeta, "a1:")
	byNotEqual := runDiffFiltered(t, "r1", "r2", "speed<>slow")
	requireAbsent(t, byNotEqual, "a1:")

	byName := runDiffFiltered(t, "r1", "r2", "name==slow-1")
	requireContains(t, byName, "a1: 1.000 → 0.000  ↓ -1.000\n")
	byAgent := runDiffFiltered(t, "r1", "r2", "agent==a1")
	requireContains(t, byAgent, "a1: 0.750 → 0.500")
}

func TestDiffFilterAgentFileMissingIsNotedAndSkipped(t *testing.T) {
	chdirTemp(t)
	writeProvRun(t, "r1", provRun{summary: provSummary("native", map[string]float64{"a1": 0.5})})
	writeProvRun(t, "r2", provRun{summary: provSummary("native", map[string]float64{"a1": 0.6})})

	out := runDiffFiltered(t, "r1", "r2", "speed==fast")

	requireContains(t, out, "a1: no case results to filter, skipped\n")
	requireAbsent(t, out, "0.500")
}

// TestDiffFilterAgentPresentOnOneSideShownWithDash pins that an agent present
// in only one run is shown with "-" on the side whose <agent>.json is absent,
// not skipped. r1 has a1's results; r2's summary names a1 but has no a1.json.
// A missing file (os.ErrNotExist) is an empty side, so the matching cases from
// r1 are still shown and the missing side reads "-". Skipping is reserved for a
// genuinely unreadable file or both sides failing to load.
func TestDiffFilterAgentPresentOnOneSideShownWithDash(t *testing.T) {
	chdirTemp(t)
	writeProvRun(t, "r1", provRun{
		summary: provSummary("native", map[string]float64{"a1": 0.5}),
		agents: map[string]AgentResult{"a1": {Agent: "a1", Cases: []CaseResult{
			filterCase("fast-1", metaFast, 4, 4, "out", 10, 1, 0.01),
		}}},
	})
	// r2 names a1 in its summary but has no a1.json (agent removed between runs).
	writeProvRun(t, "r2", provRun{summary: provSummary("native", map[string]float64{"a1": 0.6})})

	out := runDiffFiltered(t, "r1", "r2", "speed==fast")

	requireAbsent(t, out, "no case results to filter, skipped")
	// a1 matched on side A only (score 1.000), so side B shows "-".
	requireContains(t, out, "a1: 1.000 → -")
}

func TestDiffShowsCasesBlockAndMarkersWhenMetadataPresent(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, metaFast, metaSlow)

	out := runDiffFiltered(t, "r1", "r2")

	requireContains(t, out,
		"  Cases:\n",
		"    fast-1 [skill=cli, speed=fast]  0.500 → 1.000\n",
		"    slow-1 [speed=slow]  1.000 → 0.000\n",
		"    Case 'fast-1' output changed: [skill=cli, speed=fast]\n",
	)
	// slow-1's output did not change.
	requireAbsent(t, out, "Case 'slow-1' output changed")
}

func TestDiffCasesBlockMarksMissingSideAndFallsBackToOtherMetadata(t *testing.T) {
	chdirTemp(t)
	// r1 has no metadata on c1; r2 tags it. Metadata comes from r2.
	writeProvRun(t, "r1", provRun{summary: provSummary("native", map[string]float64{"a1": 0.5}), agents: map[string]AgentResult{"a1": {Agent: "a1", Cases: []CaseResult{
		filterCase("c1", nil, 2, 4, "a", 1, 1, 0),
		filterCase("gone", metaSlow, 4, 4, "g", 1, 1, 0),
	}}}})
	writeProvRun(t, "r2", provRun{summary: provSummary("native", map[string]float64{"a1": 1}), agents: map[string]AgentResult{"a1": {Agent: "a1", Cases: []CaseResult{
		filterCase("c1", metaFast, 4, 4, "b", 1, 1, 0),
	}}}})

	out := runDiffFiltered(t, "r1", "r2")

	requireContains(t, out,
		"    c1 [skill=cli, speed=fast]  0.500 → 1.000\n",
		"    gone [speed=slow]  1.000 → -\n",
		"    Case 'c1' output changed: [skill=cli, speed=fast]\n",
	)
}

func TestDiffWithOptionsRejectsMalformedFilter(t *testing.T) {
	chdirTemp(t)
	writeFilterRuns(t, nil, nil)

	err := DiffWithOptions("r1", "r2", RunOptions{Filters: []string{"speed"}})
	if err == nil || !strings.Contains(err.Error(), "invalid --filter") {
		t.Fatalf("err = %v, want an invalid --filter error", err)
	}
}
