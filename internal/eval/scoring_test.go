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
			name: "all skipped criteria score 0 and are listed in the breakdown",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 0, MaxScore: 5, Skipped: true},
				{Name: "b", Score: 0, MaxScore: 5, Skipped: true},
			}},
			want: " ❌ 0% (threshold: 95%)\n      a: 0/5\n      b: 0/5\n",
		},
		{
			name: "pass exactly at default threshold of 95%",
			tc:   TestCase{Name: "c"},
			cr: CaseResult{ActualOutput: "out", Scores: []CriterionScore{
				{Name: "a", Score: 5, MaxScore: 5},
				{Name: "b", Score: 5, MaxScore: 5},
				{Name: "c", Score: 5, MaxScore: 5},
				{Name: "d", Score: 4, MaxScore: 5},
			}},
			want: " ✅ 95% (threshold: 95%)\n",
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
			printCaseResult(&sb, tt.tc, Rubric{}, tt.cr)
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
		{Name: "skipped-low", Score: 0, MaxScore: 5, Skipped: true}, // skipped counts as 0/5 => shown
		{Name: "just-under", Score: 2, MaxScore: 5},                 // 2 < 3 => shown
	}
	// total = 1+3+5+0+2 = 11 / 25 = 44% (skipped counted as 0/5) -> fail, below threshold.
	cr := CaseResult{ActualOutput: "out", Scores: scores}

	var sb strings.Builder
	printCaseResult(&sb, TestCase{Name: "c"}, Rubric{}, cr)
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
	// 17/20 = 85% >= the rubric's 80% pass_threshold, even though "weak" is below 3/4 of its max.
	cr := CaseResult{ActualOutput: "out", Scores: []CriterionScore{
		{Name: "weak", Score: 2, MaxScore: 5},
		{Name: "strong", Score: 5, MaxScore: 5},
		{Name: "strong2", Score: 5, MaxScore: 5},
		{Name: "strong3", Score: 5, MaxScore: 5},
	}}
	var sb strings.Builder
	printCaseResult(&sb, TestCase{Name: "c"}, Rubric{PassThreshold: floatPtr(80)}, cr)
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

	rubric := Rubric{Agent: "selftest", PassThreshold: floatPtr(80), Criteria: []Criterion{
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
		" ✅ 100% (threshold: 80%)\n",
		" ⚠️  70% (threshold: 80%)\n      structural_completeness: 2/5\n",
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
			name: "skipped criteria score 0 and stay in the denominator",
			cr: CaseResult{Scores: []CriterionScore{
				{Name: "a", Score: 4, MaxScore: 5},
				{Name: "b", Score: 0, MaxScore: 5, Skipped: true},
				{Name: "c", Score: 0, MaxScore: 10, Skipped: true},
			}},
			wantScore: 4,
			wantMax:   20,
		},
		{
			name: "skipped criterion with a stray score still counts as 0",
			cr: CaseResult{Scores: []CriterionScore{
				{Name: "a", Score: 5, MaxScore: 5},
				{Name: "b", Score: 5, MaxScore: 5, Skipped: true},
			}},
			wantScore: 5,
			wantMax:   10,
		},
		{
			name: "all skipped keeps the full denominator at score 0",
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
			name: "skipped criteria stay in the denominator across cases",
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
			name: "everything skipped scores 0 over the full denominator",
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

// A zero denominator (no scored criteria at all) must not produce an agent
// score entry in the summary. Skipped criteria no longer cause one: they stay in
// the denominator as 0, so an agent whose every criterion was skipped scores 0.
func TestBuildSummaryZeroDenominatorOmitsAgentScore(t *testing.T) {
	results := []AgentResult{
		{Agent: "zero", Cases: []CaseResult{
			{CaseName: "c", Scores: []CriterionScore{{Name: "a", Score: 0, MaxScore: 0}}},
			{CaseName: "empty"},
		}},
		{Agent: "scored", Cases: []CaseResult{
			{CaseName: "c", Scores: []CriterionScore{
				{Name: "a", Score: 3, MaxScore: 4},
				{Name: "b", Score: 0, MaxScore: 4, Skipped: true},
			}},
		}},
		{Agent: "all-skipped", Cases: []CaseResult{
			{CaseName: "c", Scores: []CriterionScore{{Name: "a", Score: 0, MaxScore: 5, Skipped: true}}},
		}},
	}
	s := buildSummary(results, "abc")
	if _, ok := s.AgentScores["zero"]; ok {
		t.Errorf("agent with zero denominator should have no score, got %v", s.AgentScores["zero"])
	}
	if got := s.AgentScores["scored"]; got != 0.375 {
		t.Errorf("AgentScores[scored] = %v, want 0.375 (3/8: the skipped criterion counts as 0/4)", got)
	}
	if got, ok := s.AgentScores["all-skipped"]; !ok || got != 0 {
		t.Errorf("AgentScores[all-skipped] = %v (present %v), want an explicit 0", got, ok)
	}
}

// A skipped criterion lowers the agent score rather than raising it.
func TestBuildSummarySkippedCriterionLowersAgentScore(t *testing.T) {
	full := AgentResult{Agent: "a", Cases: []CaseResult{
		{CaseName: "ok", Scores: []CriterionScore{{Name: "x", Score: 5, MaxScore: 5}}},
	}}
	withFailure := AgentResult{Agent: "a", Cases: []CaseResult{
		{CaseName: "ok", Scores: []CriterionScore{{Name: "x", Score: 5, MaxScore: 5}}},
		{CaseName: "no-output", Scores: []CriterionScore{{Name: "x", Score: 0, MaxScore: 5, Skipped: true}}},
	}}
	before := buildSummary([]AgentResult{full}, "h").AgentScores["a"]
	after := buildSummary([]AgentResult{withFailure}, "h").AgentScores["a"]
	if before != 1 || after != 0.5 {
		t.Errorf("agent score before/after a no-output case = %v/%v, want 1/0.5", before, after)
	}
}

// emptyOutputRubric has one deterministic and one LLM-judged criterion.
func emptyOutputRubric() Rubric {
	return Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-7"},
	}}
}

// requireCountedZero asserts every score is a skipped 0 with a positive
// MaxScore and that caseTotals counts the full MaxScore in the denominator.
func requireCountedZero(t *testing.T, cr CaseResult, wantMax int) {
	t.Helper()
	for _, s := range cr.Scores {
		if s.Score != 0 || s.MaxScore <= 0 || !s.Skipped {
			t.Errorf("score %+v, want a skipped 0 with MaxScore > 0", s)
		}
	}
	if got, gotMax := caseTotals(cr); got != 0 || gotMax != wantMax {
		t.Errorf("caseTotals = (%d, %d), want (0, %d)", got, gotMax, wantMax)
	}
}

func TestCaseTotalsCountsNoOutputCriteria(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	// Deterministic and LLM-judged criteria with no agent output.
	cr := CaseResult{}
	scoreCase(emptyOutputRubric(), TestCase{Name: "c"}, &cr)
	if len(cr.Scores) != 2 {
		t.Fatalf("scores = %+v, want 2", cr.Scores)
	}
	requireCountedZero(t, cr, 5+7)
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestCaseTotalsCountsLLMJudgeErrorAndNoUsableJudge(t *testing.T) {
	rubric := Rubric{Agent: "a", Criteria: []Criterion{{Name: "clarity", Scoring: "1-7"}}}
	for name, script := range map[string]string{
		"judge error":                  "cat >/dev/null\nexit 2",
		"judge returned no JSON block": "cat >/dev/null\nprintf 'no verdict here'",
	} {
		t.Run(name, func(t *testing.T) {
			chdirTemp(t)
			installFakeKiroCLI(t, script)

			cr := CaseResult{ActualOutput: "some output"}
			scoreCase(rubric, TestCase{Name: "c", Input: "in"}, &cr)
			if len(cr.Scores) != 1 {
				t.Fatalf("scores = %+v, want 1", cr.Scores)
			}
			requireCountedZero(t, cr, 7)
			if cr.Scores[0].Reasoning == "" {
				t.Error("skipped score lost its reason")
			}
		})
	}
}

func TestCaseTotalsCountsCheckedCriteriaWithNoOutput(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "structural_completeness", Type: CheckFileExists, Path: "a.txt"},
		{Criterion: "structural_completeness", Type: CheckFileExists, Path: "b.txt"},
	}}
	cr := CaseResult{WorkspaceDir: t.TempDir()} // empty output: the checks do not run
	scoreCase(emptyOutputRubric(), tc, &cr)

	checked := scoreByName(t, cr, "structural_completeness")
	if checked.Score != 0 || checked.MaxScore != 2 {
		t.Errorf("checked criterion = %+v, want 0/2", checked)
	}
	// 0/2 for the checked criterion plus the skipped, uncounted-before clarity 0/7.
	if got, gotMax := caseTotals(cr); got != 0 || gotMax != 2+7 {
		t.Errorf("caseTotals = (%d, %d), want (0, 9)", got, gotMax)
	}
}
