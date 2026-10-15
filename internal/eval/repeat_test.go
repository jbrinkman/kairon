package eval

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// ranScored builds a result of repeat number run whose single criterion
// scored score/max.
func ranScored(name string, run, score, max int) CaseResult {
	c := scored(name, score, max)
	c.Run = run
	return c
}

func TestRepeatVerdictLegacyAndSingleRepeatUnchanged(t *testing.T) {
	yes := true
	stamped := scored("stamped", 19, 20)
	stamped.Passed = &yes
	legacy := scored("legacy", 19, 20) // Passed == nil, Run == 0

	tests := []struct {
		name string
		ar   AgentResult
	}{
		{"legacy: Repeat 0, Run 0, Passed nil", AgentResult{Agent: "a", Threshold: 95, Cases: []CaseResult{legacy}}},
		{"repeat 1, run 1", AgentResult{Agent: "a", Threshold: 95, Repeat: 1, Cases: []CaseResult{ranScored("c", 1, 19, 20)}}},
		{"stamped and legacy mixed", AgentResult{Agent: "a", Threshold: 95, Cases: []CaseResult{stamped, legacy}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := agentVerdict(tt.ar)
			score, max := agentScoreTotals(tt.ar)
			if v.Score != score*100/max {
				t.Errorf("Score = %v, want %v", v.Score, score*100/max)
			}
			if !v.Passed || v.Threshold != 95 {
				t.Errorf("verdict = %+v, want passed at threshold 95 (19/20 is exactly 95)", v)
			}
			if v.Repeat != 1 {
				t.Errorf("Repeat = %d, want 1", v.Repeat)
			}
			if v.ScoreMin != v.Score || v.ScoreMax != v.Score {
				t.Errorf("ScoreMin/ScoreMax = %v/%v, want both == Score %v", v.ScoreMin, v.ScoreMax, v.Score)
			}
			if !reflect.DeepEqual(v.RepeatScores, []float64{v.Score}) {
				t.Errorf("RepeatScores = %v, want [%v]", v.RepeatScores, v.Score)
			}
			if v.CasesTotal != len(tt.ar.Cases) {
				t.Errorf("CasesTotal = %d, want %d", v.CasesTotal, len(tt.ar.Cases))
			}
		})
	}
}

func TestRepeatVerdictLegacyCaseAccountingUnchanged(t *testing.T) {
	no := false
	failed := scored("stamped-fail", 5, 5)
	failed.Passed = &no
	legacyFail := scored("legacy-fail", 4, 5)
	ar := AgentResult{Agent: "a", Cases: []CaseResult{scored("ok", 5, 5), failed, legacyFail}}
	v := agentVerdict(ar)
	if v.CasesTotal != 3 || v.CasesFailed != 2 {
		t.Errorf("CasesTotal/CasesFailed = %d/%d, want 3/2", v.CasesTotal, v.CasesFailed)
	}
}

func TestRepeatVerdictMinMaxMean(t *testing.T) {
	ar := AgentResult{Agent: "a", Threshold: 85, Repeat: 2, Cases: []CaseResult{
		ranScored("c", 1, 20, 20),
		ranScored("c", 2, 16, 20),
	}}
	v := agentVerdict(ar)
	if v.Score != 90 || v.ScoreMin != 80 || v.ScoreMax != 100 {
		t.Errorf("score/min/max = %v/%v/%v, want 90/80/100", v.Score, v.ScoreMin, v.ScoreMax)
	}
	if !reflect.DeepEqual(v.RepeatScores, []float64{100, 80}) {
		t.Errorf("RepeatScores = %v, want [100 80]", v.RepeatScores)
	}
	if v.Repeat != 2 {
		t.Errorf("Repeat = %d, want 2", v.Repeat)
	}
	if !v.Passed {
		t.Errorf("Passed = false, want true (mean 90 >= 85)")
	}
}

func TestRepeatVerdictRunOrderIndependentOfStorageOrder(t *testing.T) {
	// Case-major storage interleaves runs; scores are reported in run order.
	ar := AgentResult{Agent: "a", Threshold: 50, Repeat: 2, Cases: []CaseResult{
		ranScored("a", 1, 10, 10), ranScored("a", 2, 0, 10),
		ranScored("b", 1, 10, 10), ranScored("b", 2, 5, 10),
	}}
	v := agentVerdict(ar)
	if !reflect.DeepEqual(v.RepeatScores, []float64{100, 25}) {
		t.Errorf("RepeatScores = %v, want [100 25]", v.RepeatScores)
	}
	if v.ScoreMin != 25 || v.ScoreMax != 100 || v.Score != 62.5 {
		t.Errorf("score/min/max = %v/%v/%v, want 62.5/25/100", v.Score, v.ScoreMin, v.ScoreMax)
	}
}

func TestRepeatVerdictMeanDecidesPass(t *testing.T) {
	tests := []struct {
		name       string
		a, b       int // scores out of 20 for run 1 and run 2
		wantPassed bool
	}{
		{"100 and 90 at 95: mean is exactly 95, passes", 20, 18, true},
		{"100 and 80 at 95: fails although the best repeat is above", 20, 16, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := agentVerdict(AgentResult{Agent: "a", Threshold: 95, Repeat: 2, Cases: []CaseResult{
				ranScored("c", 1, tt.a, 20), ranScored("c", 2, tt.b, 20),
			}})
			if v.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v (score %v, max repeat %v)", v.Passed, tt.wantPassed, v.Score, v.ScoreMax)
			}
		})
	}
}

func TestRepeatVerdictMeanEqualsMeanOfRepeatScores(t *testing.T) {
	ar := AgentResult{Agent: "a", Threshold: 50, Repeat: 3, Cases: []CaseResult{
		ranScored("a", 1, 7, 9), ranScored("a", 2, 3, 9), ranScored("a", 3, 9, 9),
		ranScored("b", 1, 1, 3), ranScored("b", 2, 2, 3), ranScored("b", 3, 0, 3),
	}}
	v := agentVerdict(ar)
	var sum float64
	for _, s := range v.RepeatScores {
		sum += s
	}
	if len(v.RepeatScores) != 3 {
		t.Fatalf("RepeatScores = %v, want 3 entries", v.RepeatScores)
	}
	if math.Abs(v.Score-sum/3) > 1e-9 {
		t.Errorf("Score = %v, want mean of repeat scores %v", v.Score, sum/3)
	}
}

func TestRepeatVerdictNothingScoredFailsClosed(t *testing.T) {
	for name, ar := range map[string]AgentResult{
		"no cases":        {Agent: "a", Threshold: 95, Repeat: 2},
		"zero max":        {Agent: "a", Threshold: 95, Repeat: 2, Cases: []CaseResult{{CaseName: "c", Run: 1}, {CaseName: "c", Run: 2}}},
		"legacy no cases": {Agent: "a"},
	} {
		t.Run(name, func(t *testing.T) {
			v := agentVerdict(ar)
			if v.Passed || v.Score != 0 || v.ScoreMin != 0 || v.ScoreMax != 0 {
				t.Errorf("verdict = %+v, want passed false and score/min/max all 0", v)
			}
		})
	}
}

func TestRepeatVerdictCasesTotalCountsDistinctNames(t *testing.T) {
	var cases []CaseResult
	for _, name := range []string{"a", "b"} {
		for run := 1; run <= 3; run++ {
			cases = append(cases, ranScored(name, run, 10, 10))
		}
	}
	v := agentVerdict(AgentResult{Agent: "a", Threshold: 95, Repeat: 3, Cases: cases})
	if v.CasesTotal != 2 || v.CasesFailed != 0 {
		t.Errorf("CasesTotal/CasesFailed = %d/%d, want 2/0", v.CasesTotal, v.CasesFailed)
	}
}

func TestRepeatVerdictCasesFailedUsesPooledPercentPerCase(t *testing.T) {
	no := false
	// "flaky" passes 10/10 then 0/10 (pooled 50% < 95): failed. "steady" is
	// 19/20 pooled over two runs (95% >= 95): not failed. Stamped Passed on a
	// single run does not decide a multi-run case.
	flaky1, flaky2 := ranScored("flaky", 1, 10, 10), ranScored("flaky", 2, 0, 10)
	steady1, steady2 := ranScored("steady", 1, 10, 10), ranScored("steady", 2, 9, 10)
	steady1.Passed = &no // ignored: pooled rule applies to multiple runs
	v := agentVerdict(AgentResult{Agent: "a", Threshold: 95, Repeat: 2, Cases: []CaseResult{flaky1, flaky2, steady1, steady2}})
	if v.CasesTotal != 2 || v.CasesFailed != 1 {
		t.Errorf("CasesTotal/CasesFailed = %d/%d, want 2/1", v.CasesTotal, v.CasesFailed)
	}

	// A case's own threshold (CaseResult.Threshold) applies when set.
	lenient1, lenient2 := ranScored("lenient", 1, 10, 10), ranScored("lenient", 2, 0, 10)
	lenient1.Threshold, lenient2.Threshold = 40, 40
	v = agentVerdict(AgentResult{Agent: "a", Threshold: 95, Repeat: 2, Cases: []CaseResult{lenient1, lenient2}})
	if v.CasesFailed != 0 {
		t.Errorf("CasesFailed = %d, want 0 (pooled 50%% >= case threshold 40%%)", v.CasesFailed)
	}
}

func TestRepeatVerdictJSONFieldsAlwaysWritten(t *testing.T) {
	// A score_min of 0 (and an unset repeat) must still be written.
	b, err := json.Marshal(AgentVerdict{})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"repeat", "score_min", "score_max", "repeat_scores"} {
		if _, ok := m[key]; !ok {
			t.Errorf("AgentVerdict JSON lacks %q: %s", key, b)
		}
	}

	v := agentVerdict(AgentResult{Agent: "a", Threshold: 95, Repeat: 2, Cases: []CaseResult{
		ranScored("c", 1, 10, 10), ranScored("c", 2, 0, 10),
	}})
	b, err = json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"repeat":2`, `"score_min":0`, `"score_max":100`, `"repeat_scores":[100,0]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("verdict JSON lacks %s: %s", want, b)
		}
	}
}

func TestRepeatVerdictResultFieldsOmitEmpty(t *testing.T) {
	b, err := json.Marshal(AgentResult{Agent: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"repeat"`) {
		t.Errorf("AgentResult.Repeat 0 must be omitted: %s", b)
	}
	b, err = json.Marshal(CaseResult{CaseName: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"run"`) {
		t.Errorf("CaseResult.Run 0 must be omitted: %s", b)
	}
	b, err = json.Marshal(AgentResult{Agent: "a", Repeat: 3, Cases: []CaseResult{{CaseName: "c", Run: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"repeat":3`, `"run":2`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON lacks %s: %s", want, b)
		}
	}
}

func TestPrintAgentVerdictRepeatSuffix(t *testing.T) {
	tests := []struct {
		name string
		v    AgentVerdict
		want string
	}{
		{"repeat 0 unchanged", AgentVerdict{Score: 100, Threshold: 95, Passed: true, CasesTotal: 4, ScoreMin: 100, ScoreMax: 100},
			"PASS selftest: 100.0% (threshold 95.0%), 0/4 cases failed\n"},
		{"repeat 1 unchanged", AgentVerdict{Repeat: 1, Score: 100, Threshold: 95, Passed: true, CasesTotal: 4, ScoreMin: 100, ScoreMax: 100},
			"PASS selftest: 100.0% (threshold 95.0%), 0/4 cases failed\n"},
		{"repeat 2 fail", AgentVerdict{Repeat: 2, Score: 50, Threshold: 95, CasesTotal: 1, CasesFailed: 1, ScoreMin: 0, ScoreMax: 100},
			"FAIL selftest: 50.0% (threshold 95.0%), 1/1 cases failed [repeat 2: min 0.0%, max 100.0%]\n"},
		{"repeat 3 pass", AgentVerdict{Repeat: 3, Score: 96.7, Threshold: 95, Passed: true, CasesTotal: 5, ScoreMin: 95, ScoreMax: 100},
			"PASS selftest: 96.7% (threshold 95.0%), 0/5 cases failed [repeat 3: min 95.0%, max 100.0%]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printAgentVerdict(&buf, "selftest", tt.v)
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}
