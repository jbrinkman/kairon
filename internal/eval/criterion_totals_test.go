package eval

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func cs(name string, score, max int) CriterionScore {
	return CriterionScore{Name: name, Score: score, MaxScore: max}
}

func TestAgentCriterionTotals(t *testing.T) {
	tests := []struct {
		name string
		ar   AgentResult
		want []CriterionTotal
	}{
		{
			name: "no cases",
			ar:   AgentResult{},
			want: nil,
		},
		{
			name: "pooled over several cases in first-seen order",
			ar: AgentResult{Cases: []CaseResult{
				{CaseName: "one", Scores: []CriterionScore{cs("b", 3, 5), cs("a", 5, 5)}},
				{CaseName: "two", Scores: []CriterionScore{cs("a", 4, 5), cs("c", 1, 2), cs("b", 5, 5)}},
			}},
			want: []CriterionTotal{
				{Name: "b", Score: 8, MaxScore: 10, Percent: 80},
				{Name: "a", Score: 9, MaxScore: 10, Percent: 90},
				{Name: "c", Score: 1, MaxScore: 2, Percent: 50},
			},
		},
		{
			name: "pooled over repeats",
			ar: AgentResult{Repeat: 3, Cases: []CaseResult{
				{CaseName: "one", Run: 1, Scores: []CriterionScore{cs("a", 5, 5)}},
				{CaseName: "one", Run: 2, Scores: []CriterionScore{cs("a", 0, 5)}},
				{CaseName: "one", Run: 3, Scores: []CriterionScore{cs("a", 5, 5)}},
			}},
			want: []CriterionTotal{{Name: "a", Score: 10, MaxScore: 15, Percent: 10 * 100.0 / 15}},
		},
		{
			name: "skipped counts zero and keeps its maximum",
			ar: AgentResult{Cases: []CaseResult{
				{CaseName: "one", Scores: []CriterionScore{{Name: "a", Score: 4, MaxScore: 5, Skipped: true}}},
				{CaseName: "two", Scores: []CriterionScore{cs("a", 5, 5)}},
			}},
			want: []CriterionTotal{{Name: "a", Score: 5, MaxScore: 10, Percent: 50}},
		},
		{
			name: "criterion in only some cases has the maximum of the cases that scored it",
			ar: AgentResult{Cases: []CaseResult{
				{CaseName: "one", Scores: []CriterionScore{cs("a", 2, 2)}},
				{CaseName: "two", Scores: []CriterionScore{cs("a", 1, 1), cs("b", 1, 1)}},
			}},
			want: []CriterionTotal{
				{Name: "a", Score: 3, MaxScore: 3, Percent: 100},
				{Name: "b", Score: 1, MaxScore: 1, Percent: 100},
			},
		},
		{
			name: "zero maximum entries are dropped",
			ar: AgentResult{Cases: []CaseResult{
				{CaseName: "one", Scores: []CriterionScore{cs("zero", 0, 0), cs("a", 1, 2)}},
			}},
			want: []CriterionTotal{{Name: "a", Score: 1, MaxScore: 2, Percent: 50}},
		},
		{
			name: "percent is exact at the boundary",
			ar: AgentResult{Cases: []CaseResult{
				{CaseName: "one", Scores: []CriterionScore{cs("a", 19, 20)}},
			}},
			want: []CriterionTotal{{Name: "a", Score: 19, MaxScore: 20, Percent: 95}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentCriterionTotals(tt.ar)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("agentCriterionTotals = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The criteria must add up to exactly the totals the agent score is built from.
func TestAgentCriterionTotalsMatchAgentScoreTotals(t *testing.T) {
	ar := AgentResult{Repeat: 2, Cases: []CaseResult{
		{CaseName: "one", Run: 1, Scores: []CriterionScore{cs("a", 5, 5), {Name: "b", Score: 3, MaxScore: 5, Skipped: true}}},
		{CaseName: "one", Run: 2, Scores: []CriterionScore{cs("a", 4, 5), cs("b", 2, 5)}},
		{CaseName: "two", Run: 1, Scores: []CriterionScore{cs("b", 1, 1), cs("c", 7, 9)}},
		{CaseName: "two", Run: 2, Scores: []CriterionScore{cs("c", 0, 9)}},
	}}
	var score, max int
	for _, c := range agentCriterionTotals(ar) {
		score += c.Score
		max += c.MaxScore
	}
	wantScore, wantMax := agentScoreTotals(ar)
	if float64(score) != wantScore || float64(max) != wantMax {
		t.Errorf("criteria sum = %d/%d, agentScoreTotals = %v/%v", score, max, wantScore, wantMax)
	}
}

func TestAgentVerdictCriteria(t *testing.T) {
	ar := AgentResult{Threshold: 90, Cases: []CaseResult{
		{CaseName: "one", ActualOutput: "x", Scores: []CriterionScore{cs("a", 5, 5), cs("b", 0, 1)}},
	}}
	v := agentVerdict(ar)
	want := []CriterionTotal{
		{Name: "a", Score: 5, MaxScore: 5, Percent: 100},
		{Name: "b", Score: 0, MaxScore: 1, Percent: 0},
	}
	if !reflect.DeepEqual(v.Criteria, want) {
		t.Errorf("Criteria = %+v, want %+v", v.Criteria, want)
	}
	// The criteria are additive: score, pass and case accounting are as before.
	if v.Passed || v.CasesTotal != 1 || v.CasesFailed != 1 {
		t.Errorf("verdict = %+v, want unchanged Passed false (83.3 < 90), 1 case, 1 failed", v)
	}
	if got := 500.0 / 6; v.Score != got {
		t.Errorf("Score = %v, want %v", v.Score, got)
	}
}

func TestAgentVerdictCriteriaEmptyIsJSONArray(t *testing.T) {
	data, err := json.Marshal(agentVerdict(AgentResult{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"criteria":[]`) {
		t.Errorf("empty verdict JSON = %s, want criteria written as []", data)
	}
}

func TestPrintAgentVerdictCriteria(t *testing.T) {
	v := AgentVerdict{
		Score: 66.7, Threshold: 60, Passed: true, CasesTotal: 2,
		Criteria: []CriterionTotal{
			{Name: "file_written", Score: 2, MaxScore: 2, Percent: 100},
			{Name: "report_marker", Score: 0, MaxScore: 1, Percent: 0},
		},
	}
	var buf bytes.Buffer
	printAgentVerdict(&buf, "selftest-checks", v)
	want := "PASS selftest-checks: 66.7% (threshold 60.0%), 0/2 cases failed\n" +
		"  file_written: 2/2 (100.0%)\n" +
		"  report_marker: 0/1 (0.0%)\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}

	// With no criteria the output is exactly the single verdict line.
	v.Criteria = nil
	buf.Reset()
	printAgentVerdict(&buf, "selftest-checks", v)
	if want := "PASS selftest-checks: 66.7% (threshold 60.0%), 0/2 cases failed\n"; buf.String() != want {
		t.Errorf("no criteria: got %q, want %q", buf.String(), want)
	}
}
