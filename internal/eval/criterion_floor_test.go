package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// floorResult builds an agent result whose cases score the given criteria.
func floorResult(floor *float64, threshold float64, cases ...CaseResult) AgentResult {
	return AgentResult{Agent: "floor", Threshold: threshold, MinCriterionScore: floor, Cases: cases}
}

func floorCase(name string, run int, scores ...CriterionScore) CaseResult {
	return CaseResult{CaseName: name, Run: run, ActualOutput: "out", Scores: scores}
}

// manyChecks is the issue's scenario: 19 passing checks on one criterion and a
// single failing check on another, 19/20 = 95% overall.
func manyChecks() CaseResult {
	return floorCase("c", 0, cs("big", 19, 19), cs("tiny", 0, 1))
}

func TestAgentVerdictFloorUnsetIsUnchanged(t *testing.T) {
	v := agentVerdict(floorResult(nil, 95, manyChecks()))
	if !v.Passed || v.Score != 95 {
		t.Errorf("Passed = %v, Score = %v, want true, 95 (a 0%% criterion without a floor must still pass)", v.Passed, v.Score)
	}
	if v.MinCriterionScore != nil || v.CriteriaBelowFloor != nil {
		t.Errorf("floor fields = %v, %v, want nil, nil", v.MinCriterionScore, v.CriteriaBelowFloor)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "min_criterion_score") || strings.Contains(string(out), "criteria_below_floor") {
		t.Errorf("verdict JSON %s must omit the floor fields when no floor is set", out)
	}
}

func TestAgentVerdictFloorFailsLowCriterionDespiteScore(t *testing.T) {
	v := agentVerdict(floorResult(floatPtr(50), 95, manyChecks()))
	if v.Passed {
		t.Error("Passed = true, want false: tiny is 0% under a 50% floor")
	}
	if v.Score != 95 {
		t.Errorf("Score = %v, want 95 (the floor never changes the score)", v.Score)
	}
	if !reflect.DeepEqual(v.CriteriaBelowFloor, []string{"tiny"}) {
		t.Errorf("CriteriaBelowFloor = %v, want [tiny]", v.CriteriaBelowFloor)
	}
	if v.MinCriterionScore == nil || *v.MinCriterionScore != 50 {
		t.Errorf("MinCriterionScore = %v, want 50", v.MinCriterionScore)
	}
	if v.CasesFailed != 0 {
		t.Errorf("CasesFailed = %d, want 0 (the floor does not fail a case)", v.CasesFailed)
	}
}

func TestAgentVerdictFloorBoundary(t *testing.T) {
	tests := []struct {
		name       string
		score, max int
		wantPassed bool
		wantBelow  []string
		floor      float64
	}{
		{"exactly at the floor passes", 1, 2, true, nil, 50},
		{"49 of 100 fails a floor of 50", 49, 100, false, []string{"x"}, 50},
		{"50 of 100 passes", 50, 100, true, nil, 50},
		{"fractional floor, exact", 19, 20, true, nil, 95},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := floorResult(floatPtr(tt.floor), 1, floorCase("c", 0, cs("x", tt.score, tt.max)))
			v := agentVerdict(ar)
			if v.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", v.Passed, tt.wantPassed)
			}
			if !reflect.DeepEqual(v.CriteriaBelowFloor, tt.wantBelow) {
				t.Errorf("CriteriaBelowFloor = %v, want %v", v.CriteriaBelowFloor, tt.wantBelow)
			}
		})
	}
}

func TestAgentVerdictFloorNamesSeveralCriteriaInOrder(t *testing.T) {
	ar := floorResult(floatPtr(50), 1,
		floorCase("c", 0, cs("a", 1, 10), cs("ok", 5, 5), cs("b", 0, 1)))
	v := agentVerdict(ar)
	if want := []string{"a", "b"}; !reflect.DeepEqual(v.CriteriaBelowFloor, want) {
		t.Errorf("CriteriaBelowFloor = %v, want %v (criteria order)", v.CriteriaBelowFloor, want)
	}
	if v.Passed {
		t.Error("Passed = true, want false")
	}
}

func TestAgentVerdictFloorIsJudgedOnPooledPercentAcrossRepeats(t *testing.T) {
	// "a" is 0/2 in run 1 and 2/2 in runs 2 and 3: pooled 4/6 = 66.7% >= 50
	// although one repeat alone is 0%. "b" is 1/2, 0/2, 0/2: pooled 1/6 < 50.
	ar := floorResult(floatPtr(50), 1,
		floorCase("c", 1, cs("a", 0, 2), cs("b", 1, 2)),
		floorCase("c", 2, cs("a", 2, 2), cs("b", 0, 2)),
		floorCase("c", 3, cs("a", 2, 2), cs("b", 0, 2)))
	ar.Repeat = 3
	v := agentVerdict(ar)
	if want := []string{"b"}; !reflect.DeepEqual(v.CriteriaBelowFloor, want) {
		t.Errorf("CriteriaBelowFloor = %v, want %v", v.CriteriaBelowFloor, want)
	}
}

func TestAgentVerdictFloorWithScoreBelowThresholdStillListsCriteria(t *testing.T) {
	v := agentVerdict(floorResult(floatPtr(50), 95, floorCase("c", 0, cs("a", 1, 10), cs("b", 5, 5))))
	if v.Passed {
		t.Error("Passed = true, want false")
	}
	if want := []string{"a"}; !reflect.DeepEqual(v.CriteriaBelowFloor, want) {
		t.Errorf("CriteriaBelowFloor = %v, want %v", v.CriteriaBelowFloor, want)
	}
}

func TestAgentVerdictFloorNothingScoredFailsClosed(t *testing.T) {
	v := agentVerdict(floorResult(floatPtr(50), 95))
	if v.Passed {
		t.Error("Passed = true, want false (fail closed)")
	}
	if len(v.CriteriaBelowFloor) != 0 {
		t.Errorf("CriteriaBelowFloor = %v, want none: nothing was measured", v.CriteriaBelowFloor)
	}
}

func TestPrintAgentVerdictFloorSuffix(t *testing.T) {
	floor := floatPtr(50)
	one := AgentVerdict{
		Score: 66.7, Threshold: 60, CasesTotal: 1, AttemptsTotal: 1, AttemptsPassed: 1, SuccessRate: 100, MinCriterionScore: floor,
		Criteria: []CriterionTotal{
			{Name: "file_written", Score: 2, MaxScore: 2, Percent: 100},
			{Name: "report_marker", Score: 0, MaxScore: 1, Percent: 0},
		},
		CriteriaBelowFloor: []string{"report_marker"},
	}
	several := AgentVerdict{
		Score: 80.4, Threshold: 60, CasesTotal: 2, AttemptsTotal: 4, AttemptsPassed: 3, SuccessRate: 75, MinCriterionScore: floor, Repeat: 2, ScoreMin: 80.4, ScoreMax: 80.4,
		Criteria: []CriterionTotal{
			{Name: "a", Score: 2, MaxScore: 20, Percent: 10},
			{Name: "b", Score: 0, MaxScore: 2, Percent: 0},
		},
		CriteriaBelowFloor: []string{"a", "b"},
	}
	tests := []struct {
		name string
		v    AgentVerdict
		want string
	}{
		{"one criterion", one,
			"FAIL selftest-floor: 66.7% (threshold 60.0%), 0/1 cases failed, success rate 100.0% (1/1 attempts); criterion below floor 50.0%: report_marker 0.0% (0/1)\n" +
				"  file_written: 2/2 (100.0%)\n  report_marker: 0/1 (0.0%)\n"},
		{"several with repeat suffix last", several,
			"FAIL selftest-floor: 80.4% (threshold 60.0%), 0/2 cases failed, success rate 75.0% (3/4 attempts); criteria below floor 50.0%: a 10.0% (2/20), b 0.0% (0/2) [repeat 2: min 80.4%, max 80.4%]\n" +
				"  a: 2/20 (10.0%)\n  b: 0/2 (0.0%)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printAgentVerdict(&buf, "selftest-floor", tt.v)
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestThresholdErrorFloorFailure(t *testing.T) {
	below := []CriterionTotal{{Name: "report_marker", Score: 0, MaxScore: 1, Percent: 0}}
	err := &ThresholdError{Total: 2, Failed: []AgentFailure{
		{Agent: "selftest-floor", Score: 97, Threshold: 95, Floor: 50, BelowFloor: below},
	}}
	got := err.Error()
	if !strings.Contains(got, "selftest-floor (criterion report_marker 0.0% < floor 50.0%)") {
		t.Errorf("error %q does not name the criterion and floor", got)
	}
	if strings.Contains(got, "97.0% < 95.0%") {
		t.Errorf("error %q prints a misleading score < threshold comparison", got)
	}
	if !errors.Is(err, ErrThresholdFailed) {
		t.Error("errors.Is(err, ErrThresholdFailed) = false")
	}

	// Several criteria are joined with ", ".
	multi := &ThresholdError{Total: 1, Failed: []AgentFailure{
		{Agent: "x", Floor: 50, BelowFloor: []CriterionTotal{
			{Name: "a", Percent: 10}, {Name: "b", Percent: 0},
		}},
	}}
	if want := "x (criteria a 10.0%, b 0.0% < floor 50.0%)"; !strings.Contains(multi.Error(), want) {
		t.Errorf("error %q does not contain %q", multi.Error(), want)
	}

	// A threshold-only failure is printed exactly as before.
	plain := &ThresholdError{Total: 1, Failed: []AgentFailure{{Agent: "alpha", Score: 12.5, Threshold: 95}}}
	if want := "eval failed: 1 of 1 agents below their pass threshold: alpha (12.5% < 95.0%)"; plain.Error() != want {
		t.Errorf("plain error = %q, want %q", plain.Error(), want)
	}
}

func TestFailureOf(t *testing.T) {
	v := AgentVerdict{
		Score: 66.7, Threshold: 60, MinCriterionScore: floatPtr(50),
		Criteria: []CriterionTotal{
			{Name: "a", Score: 2, MaxScore: 2, Percent: 100},
			{Name: "b", Score: 0, MaxScore: 1, Percent: 0},
		},
		CriteriaBelowFloor: []string{"b"},
	}
	got := failureOf("agent", v)
	want := AgentFailure{
		Agent: "agent", Score: 66.7, Threshold: 60, Floor: 50,
		BelowFloor: []CriterionTotal{{Name: "b", Score: 0, MaxScore: 1, Percent: 0}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("failureOf = %+v, want %+v", got, want)
	}
	if got := failureOf("agent", AgentVerdict{Score: 1, Threshold: 95}); got.BelowFloor != nil || got.Floor != 0 {
		t.Errorf("threshold-only failureOf = %+v, want no floor data", got)
	}
}

// floorRubric is a one-criterion rubric with a floor for the stamping tests.
func floorRubric(floor *float64) Rubric {
	r := verdictRubric()
	r.MinCriterionScore = floor
	return r
}

func TestEvaluateStampsMinCriterionScore(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)

	var out strings.Builder
	for label, res := range map[string]AgentResult{
		"evaluate":            evaluate(floorRubric(floatPtr(50)), verdictCases(), "abc", &out, nil),
		"evaluateProgressive": evalProgressiveOK(t, floorRubric(floatPtr(50)), verdictCases(), "abc", &out, t.TempDir(), false, nil),
	} {
		if res.MinCriterionScore == nil || *res.MinCriterionScore != 50 {
			t.Errorf("%s: MinCriterionScore = %v, want 50", label, res.MinCriterionScore)
		}
	}
	if res := evaluate(floorRubric(nil), verdictCases(), "abc", &out, nil); res.MinCriterionScore != nil {
		t.Errorf("no floor: MinCriterionScore = %v, want nil", *res.MinCriterionScore)
	}
}

func TestEvaluateProgressiveResumeRestampsMinCriterionScore(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)

	dir := t.TempDir()
	// A saved result with a stale floor of 10 and one completed case.
	stale := AgentResult{Agent: "selftest", GitHash: "abc", MinCriterionScore: floatPtr(10), Cases: []CaseResult{scored("done-case", 5, 5)}}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(filepath.Join(dir, "selftest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	cases := append([]TestCase{{Name: "done-case", Input: "do it"}}, verdictCases()...)
	res := evalProgressiveOK(t, floorRubric(floatPtr(50)), cases, "abc", &out, dir, true, nil)
	if res.MinCriterionScore == nil || *res.MinCriterionScore != 50 {
		t.Errorf("resume: MinCriterionScore = %v, want the current rubric's 50", res.MinCriterionScore)
	}

	// Removing the floor from the rubric clears a stale saved one.
	if err := os.WriteFile(filepath.Join(dir, "selftest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	res = evalProgressiveOK(t, floorRubric(nil), cases, "abc", &out, dir, true, nil)
	if res.MinCriterionScore != nil {
		t.Errorf("resume without floor: MinCriterionScore = %v, want nil", *res.MinCriterionScore)
	}
}
