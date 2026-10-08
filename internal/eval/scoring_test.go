package eval

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

func floatPtr(f float64) *float64 { return &f }

func TestPrintCaseResultStatuses(t *testing.T) {
	tests := []struct {
		name string
		tc   TestCase
		cr   CaseResult
		want string
	}{
		{
			name: "no output",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{Scores: []CriterionScore{
				{Name: "a", Score: 0, MaxScore: 5, Skipped: true},
			}},
			want: " ❌ no output\n",
		},
		{
			name: "no scored criteria when no scores at all",
			tc:   TestCase{Name: "c"},
			cr:   CaseResult{ActualOutput: "out"},
			want: " ⚠️  no scored criteria\n",
		},
		{
			name: "all skipped criteria score 0 over their maximum",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 0, MaxScore: 5, Skipped: true},
				{Name: "b", Score: 0, MaxScore: 5, Skipped: true},
			}},
			want: " ❌ 0% (threshold: 95%)\n      a: 0/5\n      b: 0/5\n",
		},
		{
			name: "pass at default threshold",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 5, MaxScore: 5},
			}},
			want: " ✅ 100% (threshold: 95%)\n",
		},
		{
			name: "19 of 20 passes the 95 bar exactly",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 19, MaxScore: 20},
			}},
			want: " ✅ 95% (threshold: 95%)\n",
		},
		{
			name: "80% no longer passes the default",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 4, MaxScore: 5},
			}},
			want: " ⚠️  80% (threshold: 95%)\n",
		},
		{
			name: "recorded case threshold is used",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Threshold: floatPtr(70), Scores: []CriterionScore{
				{Name: "a", Score: 4, MaxScore: 5},
			}},
			want: " ✅ 80% (threshold: 70%)\n",
		},
		{
			name: "warn between 60 and threshold",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 4, MaxScore: 5},
				{Name: "b", Score: 2, MaxScore: 5},
			}},
			want: " ⚠️  60% (threshold: 95%)\n      b: 2/5\n",
		},
		{
			name: "fail below 60",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 2, MaxScore: 5},
			}},
			want: " ❌ 40% (threshold: 95%)\n      a: 2/5\n",
		},
		{
			name: "custom min_score lowers threshold so 40% passes",
			tc:   TestCase{Name: "c", MinScore: floatPtr(40)},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 2, MaxScore: 5},
			}},
			want: " ✅ 40% (threshold: 40%)\n",
		},
		{
			name: "fail at 50% with no breakdown when criterion is at 3/4 of max",
			tc:   TestCase{Name: "c", MinScore: floatPtr(90)},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 1, MaxScore: 2},
			}},
			// 50% < 60 => fail; max 2 => 2*3/4 == 1, so 1 < 1 is false => no breakdown line.
			want: " ❌ 50% (threshold: 90%)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			printCaseResult(&sb, tt.tc, tt.cr)
			if got := sb.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintCaseResultBreakdownOnlyBelowThreshold(t *testing.T) {
	scores := []CriterionScore{
		{Name: "low", Score: 1, MaxScore: 5},                        // 1 < 3 => shown
		{Name: "edge", Score: 3, MaxScore: 5},                       // 3 < 5*3/4=3 false => omitted
		{Name: "high", Score: 5, MaxScore: 5},                       // omitted
		{Name: "skipped-low", Score: 0, MaxScore: 5, Skipped: true}, // counts as 0 => shown
		{Name: "just-under", Score: 2, MaxScore: 5},                 // 2 < 3 => shown
	}
	// total = 1+3+5+0+2 = 11 / 25 = 44% (skipped counted as 0) -> fail, below threshold.
	cr := CaseResult{ActualOutput: "out", Scores: scores}

	var sb strings.Builder
	printCaseResult(&sb, TestCase{Name: "c"}, cr)
	got := sb.String()

	want := " ❌ 44% (threshold: 95%)\n      low: 1/5\n      skipped-low: 0/5\n      just-under: 2/5\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	for _, absent := range []string{"edge", "high"} {
		if strings.Contains(got, absent) {
			t.Errorf("breakdown unexpectedly contains %q: %q", absent, got)
		}
	}
}

func TestPrintCaseResultNoBreakdownWhenPassing(t *testing.T) {
	// 17/20 = 85% >= 80% threshold, even though "weak" is below 3/4 of its max.
	cr := CaseResult{ActualOutput: "out", Threshold: floatPtr(80), Scores: []CriterionScore{
		{Name: "weak", Score: 2, MaxScore: 5},
		{Name: "strong", Score: 5, MaxScore: 5},
		{Name: "strong2", Score: 5, MaxScore: 5},
		{Name: "strong3", Score: 5, MaxScore: 5},
	}}
	var sb strings.Builder
	printCaseResult(&sb, TestCase{Name: "c"}, cr)
	want := " ✅ 85% (threshold: 80%)\n"
	if got := sb.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestScoreCaseSkipsCostCriterion(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	rubric := Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "cost_budget", Scoring: "1-5", Type: "cost"},
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S\ntext"}
	scoreCase(rubric, TestCase{Name: "c"}, &cr)

	if len(cr.Scores) != 1 {
		t.Fatalf("scores = %+v, want exactly 1 (cost criterion skipped)", cr.Scores)
	}
	if cr.Scores[0].Name != "structural_completeness" {
		t.Errorf("scored criterion = %q", cr.Scores[0].Name)
	}
}

func TestScoreCaseDeterministicPath(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	rubric := Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S\ntext"}
	scoreCase(rubric, TestCase{Name: "c"}, &cr)

	want := []CriterionScore{{
		Name:          "structural_completeness",
		Score:         5,
		MaxScore:      5,
		Deterministic: true,
		Reasoning:     "found 2/2 expected structural elements",
	}}
	if !reflect.DeepEqual(cr.Scores, want) {
		t.Errorf("scores = %+v, want %+v", cr.Scores, want)
	}
	// Deterministic scoring must not incur judge cost.
	if cr.JudgeCost != (CostInfo{}) {
		t.Errorf("JudgeCost = %+v, want zero", cr.JudgeCost)
	}
}

func TestScoreCaseDeterministicEmptyOutputSkipped(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	rubric := Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
	}}
	cr := CaseResult{}
	scoreCase(rubric, TestCase{Name: "c"}, &cr)

	if len(cr.Scores) != 1 || !cr.Scores[0].Skipped || cr.Scores[0].Score != 0 {
		t.Errorf("scores = %+v, want one skipped zero score", cr.Scores)
	}
}

func TestScoreCaseLLMJudgeEmptyOutputSkipped(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	rubric := Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "clarity", Scoring: "1-7"},
	}}
	cr := CaseResult{}
	scoreCase(rubric, TestCase{Name: "c"}, &cr)

	want := []CriterionScore{{
		Name:      "clarity",
		Score:     0,
		MaxScore:  7,
		Skipped:   true,
		Reasoning: "no output available for LLM judging",
	}}
	if !reflect.DeepEqual(cr.Scores, want) {
		t.Errorf("scores = %+v, want %+v", cr.Scores, want)
	}
	if cr.JudgeCost != (CostInfo{}) {
		t.Errorf("JudgeCost = %+v, want zero (judge must not run)", cr.JudgeCost)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestScoreCaseLLMJudgeAccumulatesJudgeCost(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	rubric := Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "clarity", Scoring: "1-5"},
		{Name: "depth", Scoring: "1-5"},
	}}
	cr := CaseResult{ActualOutput: "some output"}
	scoreCase(rubric, TestCase{Name: "c", Input: "in"}, &cr)

	if len(cr.Scores) != 2 {
		t.Fatalf("scores = %+v", cr.Scores)
	}
	for _, s := range cr.Scores {
		if s.Skipped || s.Score != 5 || s.MaxScore != 5 || s.Deterministic {
			t.Errorf("score %+v, want unskipped 5/5 non-deterministic", s)
		}
	}
	if cr.JudgeCost.Model != "stub" || cr.JudgeCost.UsageSource != string(inference.UsageEstimated) || cr.JudgeCost.TokensIn == 0 {
		t.Errorf("JudgeCost = %+v, want accumulated stub estimate", cr.JudgeCost)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestScoreCaseNilScoresWhenNoNonCostCriteria(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	for name, rubric := range map[string]Rubric{
		"empty rubric": {Agent: "a"},
		"cost only": {Agent: "a", Criteria: []Criterion{
			{Name: "cost_budget", Scoring: "1-5", Type: "cost"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			cr := CaseResult{ActualOutput: "out"}
			scoreCase(rubric, TestCase{Name: "c"}, &cr)
			if cr.Scores != nil {
				t.Errorf("Scores = %#v, want nil", cr.Scores)
			}
		})
	}
}

// TestScoringParityEvaluateVsEvaluateProgressive runs identical stub-backed cases through
// evaluate (single-case/--case path) and evaluateProgressive (full/resume
// path) and requires identical scores and identical status-line output.
func TestScoringParityEvaluateVsEvaluateProgressive(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	rubric := Rubric{Agent: "selftest", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-5"},
		{Name: "cost_budget", Scoring: "1-5", Type: "cost"},
	}}
	stub := func(resp string) *inference.StubScript {
		return &inference.StubScript{Turns: []inference.StubTurn{{
			Response: resp,
			Usage:    &inference.StubUsage{InputTokens: 10, OutputTokens: 5},
		}}}
	}
	cases := []TestCase{
		{Name: "pass-case", Input: "do it", Stub: stub("## H\n### S\ntext")},               // 100% -> pass
		{Name: "warn-case", Input: "do it", Stub: stub("## only one section")},             // 7/10 -> warn + breakdown
		{Name: "lenient-case", Input: "do it", MinScore: floatPtr(50), Stub: stub("## x")}, // 70% >= 50 -> pass
	}

	var singleOut, progOut strings.Builder
	single := evaluate(rubric, cases, "abc", &singleOut, nil)
	prog := evaluateProgressive(rubric, cases, "abc", &progOut, t.TempDir(), false, nil)

	if len(single.Cases) != len(cases) || len(prog.Cases) != len(cases) {
		t.Fatalf("case counts: single=%d progressive=%d, want %d", len(single.Cases), len(prog.Cases), len(cases))
	}
	for i := range cases {
		s, p := single.Cases[i], prog.Cases[i]
		if s.CaseName != p.CaseName {
			t.Errorf("case %d name: %q vs %q", i, s.CaseName, p.CaseName)
		}
		if len(s.Scores) == 0 {
			t.Errorf("case %q: no scores produced", s.CaseName)
		}
		if !reflect.DeepEqual(s.Scores, p.Scores) {
			t.Errorf("case %q scores differ:\n single:      %+v\n progressive: %+v", s.CaseName, s.Scores, p.Scores)
		}
		if s.JudgeCost != p.JudgeCost {
			t.Errorf("case %q judge cost differ: %+v vs %+v", s.CaseName, s.JudgeCost, p.JudgeCost)
		}
	}

	// Identical status-line output (progress text, status line, breakdown).
	if singleOut.String() != progOut.String() {
		t.Errorf("output differs:\n single:\n%s\n progressive:\n%s", singleOut.String(), progOut.String())
	}

	// Sanity: the outputs contain the expected status lines so parity is not vacuous.
	got := singleOut.String()
	for _, want := range []string{
		" ✅ 100% (threshold: 95%)\n",
		" ⚠️  70% (threshold: 95%)\n      structural_completeness: 2/5\n",
		" ✅ 70% (threshold: 50%)\n", // passing: breakdown omitted
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}

	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestCaseTotals(t *testing.T) {
	tests := []struct {
		name      string
		cr        CaseResult
		wantScore int
		wantMax   int
	}{
		{
			name:      "no scores",
			cr:        CaseResult{},
			wantScore: 0,
			wantMax:   0,
		},
		{
			name: "all scored",
			cr: CaseResult{Scores: []CriterionScore{
				{Name: "a", Score: 4, MaxScore: 5},
				{Name: "b", Score: 3, MaxScore: 5},
			}},
			wantScore: 7,
			wantMax:   10,
		},
		{
			name: "skipped criteria count in the denominator with their score",
			cr: CaseResult{Scores: []CriterionScore{
				{Name: "a", Score: 4, MaxScore: 5},
				{Name: "b", Score: 0, MaxScore: 5, Skipped: true},
				{Name: "c", Score: 0, MaxScore: 10, Skipped: true},
			}},
			wantScore: 4,
			wantMax:   20,
		},
		{
			name: "all skipped scores 0 over a non-zero maximum",
			cr: CaseResult{Scores: []CriterionScore{
				{Name: "a", Score: 0, MaxScore: 5, Skipped: true},
				{Name: "b", Score: 0, MaxScore: 5, Skipped: true},
			}},
			wantScore: 0,
			wantMax:   10,
		},
		{
			name: "scored criterion with zero max",
			cr: CaseResult{Scores: []CriterionScore{
				{Name: "a", Score: 0, MaxScore: 0},
			}},
			wantScore: 0,
			wantMax:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotMax := caseTotals(tt.cr)
			if gotScore != tt.wantScore || gotMax != tt.wantMax {
				t.Errorf("caseTotals() = (%d, %d), want (%d, %d)", gotScore, gotMax, tt.wantScore, tt.wantMax)
			}
		})
	}
}

func TestAgentScoreTotals(t *testing.T) {
	tests := []struct {
		name      string
		ar        AgentResult
		wantScore float64
		wantMax   float64
	}{
		{
			name:      "no cases",
			ar:        AgentResult{Agent: "x"},
			wantScore: 0,
			wantMax:   0,
		},
		{
			name: "case with no scores contributes nothing",
			ar: AgentResult{Agent: "x", Cases: []CaseResult{
				{CaseName: "empty"},
				{CaseName: "scored", Scores: []CriterionScore{{Name: "a", Score: 3, MaxScore: 5}}},
			}},
			wantScore: 3,
			wantMax:   5,
		},
		{
			name: "skipped criteria counted as 0 across cases",
			ar: AgentResult{Agent: "x", Cases: []CaseResult{
				{CaseName: "c1", Scores: []CriterionScore{
					{Name: "a", Score: 5, MaxScore: 5},
					{Name: "b", Score: 0, MaxScore: 5, Skipped: true},
				}},
				{CaseName: "c2", Scores: []CriterionScore{
					{Name: "a", Score: 2, MaxScore: 5},
					{Name: "b", Score: 0, MaxScore: 10, Skipped: true},
				}},
			}},
			wantScore: 7,
			wantMax:   25,
		},
		{
			name: "everything skipped scores 0 over a non-zero maximum",
			ar: AgentResult{Agent: "x", Cases: []CaseResult{
				{CaseName: "c1", Scores: []CriterionScore{{Name: "a", Score: 0, MaxScore: 5, Skipped: true}}},
			}},
			wantScore: 0,
			wantMax:   5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotMax := agentScoreTotals(tt.ar)
			if gotScore != tt.wantScore || gotMax != tt.wantMax {
				t.Errorf("agentScoreTotals() = (%v, %v), want (%v, %v)", gotScore, gotMax, tt.wantScore, tt.wantMax)
			}
		})
	}
}

// A zero denominator must not produce an agent score entry in the summary
// (previous behaviour: only totalMax > 0 writes AgentScores).
func TestBuildSummaryZeroDenominatorOmitsAgentScore(t *testing.T) {
	results := []AgentResult{
		{Agent: "zero", Cases: []CaseResult{
			{CaseName: "c", Scores: []CriterionScore{{Name: "a", Score: 0, MaxScore: 0}}},
		}},
		{Agent: "scored", Cases: []CaseResult{
			{CaseName: "c", Scores: []CriterionScore{
				{Name: "a", Score: 3, MaxScore: 4},
				{Name: "b", Score: 1, MaxScore: 4},
			}},
		}},
	}
	s := buildSummary(results, "abc")
	if _, ok := s.AgentScores["zero"]; ok {
		t.Errorf("agent with zero denominator should have no score, got %v", s.AgentScores["zero"])
	}
	if got := s.AgentScores["scored"]; got != 0.5 {
		t.Errorf("AgentScores[scored] = %v, want 0.5", got)
	}
}

// A skipped (errored / no-output) criterion must lower the case and agent
// score instead of vanishing from the denominator.
func TestSkippedCriterionLowersCaseAndAgentScore(t *testing.T) {
	scored := CaseResult{CaseName: "c", Scores: []CriterionScore{
		{Name: "a", Score: 5, MaxScore: 5},
	}}
	withSkip := CaseResult{CaseName: "c", Scores: []CriterionScore{
		{Name: "a", Score: 5, MaxScore: 5},
		{Name: "b", Score: 0, MaxScore: 5, Skipped: true},
	}}
	s1, m1 := caseTotals(scored)
	s2, m2 := caseTotals(withSkip)
	if p1, p2 := scorePercent(float64(s1), float64(m1)), scorePercent(float64(s2), float64(m2)); p1 != 100 || p2 != 50 {
		t.Errorf("case percent without skip %v, with skip %v; want 100 and 50", p1, p2)
	}

	ok := AgentResult{Agent: "x", Cases: []CaseResult{scored}}
	bad := AgentResult{Agent: "x", Cases: []CaseResult{withSkip}}
	if v1, v2 := agentVerdict(ok), agentVerdict(bad); v2.Score >= v1.Score {
		t.Errorf("agent with a skipped criterion scored %v, not below %v", v2.Score, v1.Score)
	}
}

func TestNoOutputCaseScoresZeroOverNonZeroMax(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	rubric := Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-7"},
	}}
	cr := CaseResult{} // no agent output
	scoreCase(rubric, TestCase{Name: "c"}, &cr)

	score, max := caseTotals(cr)
	if score != 0 || max != 12 {
		t.Errorf("caseTotals = %d/%d, want 0/12", score, max)
	}
}

func TestVerdictBoundaryAndFailures(t *testing.T) {
	mk := func(score, max int, thr *float64) CaseResult {
		return CaseResult{CaseName: "c", Threshold: thr, Scores: []CriterionScore{{Name: "a", Score: score, MaxScore: max}}}
	}

	// 19 of 20 is exactly 95 and passes the default bar.
	v := agentVerdict(AgentResult{Agent: "x", Cases: []CaseResult{mk(19, 20, nil)}})
	if v.Score != 95 || v.Threshold != 95 || !v.Passed || v.CasesTotal != 1 || v.CasesFailed != 0 {
		t.Errorf("19/20 verdict = %+v", v)
	}

	// 18 of 20 fails.
	v = agentVerdict(AgentResult{Agent: "x", Cases: []CaseResult{mk(18, 20, nil)}})
	if v.Score != 90 || v.Passed || v.CasesFailed != 1 {
		t.Errorf("18/20 verdict = %+v", v)
	}

	// An agent threshold of its own, and per-case thresholds for cases_failed:
	// the agent passes on the aggregate while one case is below its own bar.
	ar := AgentResult{Agent: "x", Threshold: floatPtr(80), Cases: []CaseResult{
		mk(20, 20, nil),          // 100 >= 80
		mk(12, 20, nil),          // 60 < 80  -> failed
		mk(12, 20, floatPtr(50)), // 60 >= its own 50 -> not failed
		mk(20, 20, nil),
	}}
	v = agentVerdict(ar)
	if v.Score != 80 || v.Threshold != 80 || !v.Passed || v.CasesTotal != 4 || v.CasesFailed != 1 {
		t.Errorf("verdict = %+v, want score 80 threshold 80 passed total 4 failed 1", v)
	}
}

func TestVerdictZeroMaxFails(t *testing.T) {
	// No cases at all.
	v := agentVerdict(AgentResult{Agent: "x", Threshold: floatPtr(0)})
	if v.Passed || v.Score != 0 || v.CasesTotal != 0 {
		t.Errorf("no-case verdict = %+v, want failed with score 0", v)
	}

	// A case with nothing scorable fails even at threshold 0, and so does the agent.
	ar := AgentResult{Agent: "x", Threshold: floatPtr(0), Cases: []CaseResult{{CaseName: "empty"}}}
	v = agentVerdict(ar)
	if v.Passed || v.CasesFailed != 1 || v.CasesTotal != 1 {
		t.Errorf("zero-max verdict = %+v, want failed with 1/1 cases failed", v)
	}
}

func TestVerdictLegacyResultFallsBackTo95(t *testing.T) {
	ar := AgentResult{Agent: "x", Cases: []CaseResult{
		{CaseName: "c", Scores: []CriterionScore{{Name: "a", Score: 9, MaxScore: 10}}},
	}}
	v := agentVerdict(ar)
	if v.Threshold != 95 || v.Passed || v.CasesFailed != 1 {
		t.Errorf("legacy verdict = %+v, want threshold 95, failed", v)
	}
}

func TestVerdictCostSumsAgentAndJudge(t *testing.T) {
	ar := AgentResult{Agent: "x", Threshold: floatPtr(0), Cases: []CaseResult{
		{CaseName: "a", Scores: []CriterionScore{{Name: "k", Score: 1, MaxScore: 1}},
			AgentCost: CostInfo{TokensIn: 10, TokensOut: 5, EstimatedUSD: 0.5, Model: "m", UsageSource: "reported"},
			JudgeCost: CostInfo{TokensIn: 1, TokensOut: 2, EstimatedUSD: 0.25}},
		{CaseName: "b", Scores: []CriterionScore{{Name: "k", Score: 1, MaxScore: 1}},
			AgentCost: CostInfo{TokensIn: 20, TokensOut: 10, EstimatedUSD: 1}},
	}}
	want := CostInfo{TokensIn: 31, TokensOut: 17, EstimatedUSD: 1.75}
	if got := agentVerdict(ar).Cost; got != want {
		t.Errorf("verdict cost = %+v, want %+v (tokens and usd only)", got, want)
	}
}

func TestExecuteCaseRecordsEffectiveThreshold(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	rubric := Rubric{Agent: "a", PassThreshold: floatPtr(70), Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
	}}
	stub := &inference.StubScript{Turns: []inference.StubTurn{{Response: "## H\n### S\ntext"}}}

	cr := executeCase(rubric, TestCase{Name: "inherits", Input: "x", Stub: stub}, nil, &strings.Builder{}, false)
	if cr.Threshold == nil || *cr.Threshold != 70 {
		t.Errorf("inherited Threshold = %v, want 70", cr.Threshold)
	}
	cr = executeCase(rubric, TestCase{Name: "overrides", Input: "x", MinScore: floatPtr(40), Stub: stub}, nil, &strings.Builder{}, false)
	if cr.Threshold == nil || *cr.Threshold != 40 {
		t.Errorf("min_score Threshold = %v, want 40", cr.Threshold)
	}
	// Recorded on early-failure paths too.
	cr = executeCase(rubric, TestCase{Name: "sandbox-only", RequiresSandbox: true}, nil, &strings.Builder{}, false)
	if cr.Threshold == nil || *cr.Threshold != 70 {
		t.Errorf("requires-sandbox Threshold = %v, want 70", cr.Threshold)
	}
}

func TestEvaluateStampsAgentThreshold(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	rubric := Rubric{Agent: "a", PassThreshold: floatPtr(66)}
	for name, res := range map[string]AgentResult{
		"evaluate":    evaluate(rubric, nil, "abc", &strings.Builder{}, nil),
		"progressive": evaluateProgressive(rubric, nil, "abc", &strings.Builder{}, t.TempDir(), false, nil),
	} {
		if res.Threshold == nil || *res.Threshold != 66 {
			t.Errorf("%s: AgentResult.Threshold = %v, want 66", name, res.Threshold)
		}
	}
}

func TestCriterionAveragesCountSkippedAsZero(t *testing.T) {
	ar := AgentResult{Agent: "x", Cases: []CaseResult{
		{CaseName: "c1", Scores: []CriterionScore{{Name: "a", Score: 4, MaxScore: 4}}},
		{CaseName: "c2", Scores: []CriterionScore{{Name: "a", Score: 0, MaxScore: 4, Skipped: true}}},
		{CaseName: "c3", Scores: []CriterionScore{{Name: "unset-max", Score: 0, MaxScore: 0}}},
	}}
	avgs := criterionAverages(ar)
	if got := avgs["a"]; got != 0.5 {
		t.Errorf("average for a = %v, want 0.5 (skipped counted as 0)", got)
	}
	if _, ok := avgs["unset-max"]; ok {
		t.Errorf("criterion with no max_score should not be averaged: %v", avgs)
	}
}
