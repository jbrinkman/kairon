package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// attemptCase builds a stamped case result for run `run` (0 = no repeat).
func attemptCase(name string, run int, passed bool) CaseResult {
	cr := scored(name, 1, 1)
	cr.Run = run
	cr.Threshold = 95
	cr.Passed = boolPtr(passed)
	return cr
}

func TestAttemptAccountingOnePerCase(t *testing.T) {
	ar := AgentResult{Agent: "a", Threshold: 95, Cases: []CaseResult{
		attemptCase("c1", 0, true),
		attemptCase("c2", 0, false),
		attemptCase("c3", 0, true),
	}}
	total, passed := attemptAccounting(ar, 95)
	if total != 3 || passed != 2 {
		t.Errorf("attemptAccounting = %d, %d; want 3, 2", total, passed)
	}
}

func TestAttemptAccountingRepeats(t *testing.T) {
	ar := AgentResult{Agent: "a", Threshold: 95, Repeat: 3}
	// c1 passes in runs 1 and 3; c2 passes only in run 2.
	pass := map[string][3]bool{"c1": {true, false, true}, "c2": {false, true, false}}
	for _, name := range []string{"c1", "c2"} {
		for run := 1; run <= 3; run++ {
			ar.Cases = append(ar.Cases, attemptCase(name, run, pass[name][run-1]))
		}
	}
	total, passed := attemptAccounting(ar, 95)
	if total != 6 || passed != 3 {
		t.Errorf("attemptAccounting = %d, %d; want 6, 3", total, passed)
	}
	v := agentVerdict(ar)
	if v.AttemptsTotal != 6 || v.AttemptsPassed != 3 {
		t.Errorf("verdict attempts = %d/%d, want 3/6", v.AttemptsPassed, v.AttemptsTotal)
	}
	if v.CasesTotal != 2 {
		t.Errorf("cases_total = %d, want 2 (repeats are attempts, not cases)", v.CasesTotal)
	}
	if v.SuccessRate != 50 {
		t.Errorf("success_rate = %v, want 50", v.SuccessRate)
	}
}

func TestAttemptAccountingLegacyResultRecomputed(t *testing.T) {
	pass := scored("pass", 19, 20) // 95% meets threshold 95
	fail := scored("fail", 18, 20) // 90% does not
	if pass.Passed != nil || fail.Passed != nil {
		t.Fatal("test setup: legacy results must have Passed == nil")
	}
	total, passed := attemptAccounting(AgentResult{Agent: "a", Cases: []CaseResult{pass, fail}}, 95)
	if total != 2 || passed != 1 {
		t.Errorf("attemptAccounting = %d, %d; want 2, 1", total, passed)
	}
}

func TestAttemptAccountingNoOutputIsFailedAttempt(t *testing.T) {
	// stampCase marks a case with no output as not passed even with full marks.
	cr := scored("empty", 1, 1)
	cr.ActualOutput = ""
	stampCase(&cr, TestCase{}, Rubric{})
	total, passed := attemptAccounting(AgentResult{Agent: "a", Cases: []CaseResult{cr}}, 95)
	if total != 1 || passed != 0 {
		t.Errorf("attemptAccounting = %d, %d; want 1, 0", total, passed)
	}
}

func TestAttemptAccountingZeroResults(t *testing.T) {
	total, passed := attemptAccounting(AgentResult{Agent: "a"}, 95)
	if total != 0 || passed != 0 {
		t.Errorf("attemptAccounting = %d, %d; want 0, 0", total, passed)
	}
	v := agentVerdict(AgentResult{Agent: "a"})
	if v.AttemptsTotal != 0 || v.AttemptsPassed != 0 || v.SuccessRate != 0 {
		t.Errorf("zero-result verdict = %+v, want zero attempts and success_rate 0", v)
	}
	if math.IsNaN(v.SuccessRate) {
		t.Error("success_rate is NaN for zero attempts")
	}
}

func TestVerdictPassesWithSuccessRateBelow100(t *testing.T) {
	// Pooled score 99/100 = 99% meets the 95% threshold although one of the
	// three attempts failed its own threshold.
	good1 := scored("c1", 33, 33)
	good2 := scored("c2", 33, 33)
	bad := scored("c3", 33, 34)
	for i, c := range []*CaseResult{&good1, &good2, &bad} {
		c.Threshold = 100
		c.Passed = boolPtr(i < 2)
	}
	ar := AgentResult{Agent: "a", Threshold: 95, Cases: []CaseResult{good1, good2, bad}}
	v := agentVerdict(ar)
	if !v.Passed {
		t.Fatalf("verdict must pass on the pooled score (score %v, threshold %v)", v.Score, v.Threshold)
	}
	if v.AttemptsTotal != 3 || v.AttemptsPassed != 2 {
		t.Errorf("attempts = %d/%d, want 2/3", v.AttemptsPassed, v.AttemptsTotal)
	}
	if want := 200.0 / 3; math.Abs(v.SuccessRate-want) > 1e-9 {
		t.Errorf("success_rate = %v, want %v", v.SuccessRate, want)
	}
}

func TestAttemptCountsIndependentOfFloorAndThreshold(t *testing.T) {
	cases := []CaseResult{attemptCase("c1", 0, true), attemptCase("c2", 0, false)}
	base := agentVerdict(AgentResult{Agent: "a", Threshold: 95, Cases: cases})

	floor := 100.0
	withFloor := agentVerdict(AgentResult{Agent: "a", Threshold: 95, MinCriterionScore: &floor, Cases: cases})
	lowThreshold := agentVerdict(AgentResult{Agent: "a", Threshold: 1, Cases: cases})
	for name, v := range map[string]AgentVerdict{"floor": withFloor, "threshold": lowThreshold} {
		if v.AttemptsTotal != base.AttemptsTotal || v.AttemptsPassed != base.AttemptsPassed || v.SuccessRate != base.SuccessRate {
			t.Errorf("%s changed the attempt counts: %+v vs %+v", name, v, base)
		}
	}
}

func TestAgentVerdictAttemptKeysAlwaysWritten(t *testing.T) {
	// success_rate 0 must be written, not omitted.
	ar := AgentResult{Agent: "a", Threshold: 95, Cases: []CaseResult{attemptCase("c1", 0, false)}}
	data, err := json.Marshal(agentVerdict(ar))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]float64{"attempts_total": 1, "attempts_passed": 0, "success_rate": 0} {
		got, ok := raw[k]
		if !ok {
			t.Errorf("verdict JSON lacks %q: %s", k, data)
			continue
		}
		if got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}

	// An agent with no results also writes all three.
	data, err = json.Marshal(agentVerdict(AgentResult{Agent: "a"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"attempts_total":0`, `"attempts_passed":0`, `"success_rate":0`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("empty verdict JSON lacks %s: %s", want, data)
		}
	}
}

func TestSuccessRateIsPercentNotFraction(t *testing.T) {
	ar := AgentResult{Agent: "a", Threshold: 95, Cases: []CaseResult{
		attemptCase("c1", 0, true), attemptCase("c2", 0, true), attemptCase("c3", 0, true), attemptCase("c4", 0, false),
	}}
	if v := agentVerdict(ar); v.SuccessRate != 75 {
		t.Errorf("success_rate = %v, want 75 (percent)", v.SuccessRate)
	}
}

func TestPrintAgentVerdictSuccessRate(t *testing.T) {
	floor := 50.0
	tests := []struct {
		name string
		v    AgentVerdict
		want string
	}{
		{"pass",
			AgentVerdict{Score: 100, Threshold: 95, Passed: true, CasesTotal: 25, AttemptsTotal: 25, AttemptsPassed: 25, SuccessRate: 100},
			"PASS selftest: 100.0% (threshold 95.0%), 0/25 cases failed, success rate 100.0% (25/25 attempts)\n"},
		{"fail with repeat",
			AgentVerdict{Score: 8.1, Threshold: 95, CasesTotal: 21, CasesFailed: 21, Repeat: 2, ScoreMin: 6.5, ScoreMax: 9.7,
				AttemptsTotal: 42, AttemptsPassed: 1, SuccessRate: 100.0 / 42},
			"FAIL selftest: 8.1% (threshold 95.0%), 21/21 cases failed, success rate 2.4% (1/42 attempts) [repeat 2: min 6.5%, max 9.7%]\n"},
		{"zero attempts",
			AgentVerdict{Threshold: 95},
			"FAIL selftest: 0.0% (threshold 95.0%), 0/0 cases failed, success rate 0.0% (0/0 attempts)\n"},
		{"rate before floor, repeat and cost-incomplete",
			AgentVerdict{Score: 80, Threshold: 60, CasesTotal: 2, Repeat: 2, ScoreMin: 70, ScoreMax: 90,
				AttemptsTotal: 4, AttemptsPassed: 3, SuccessRate: 75,
				MinCriterionScore: &floor, CriteriaBelowFloor: []string{"a"},
				Criteria: []CriterionTotal{{Name: "a", Score: 1, MaxScore: 10, Percent: 10}},
				Cost:     CostInfo{CostIncomplete: true, UnpricedModels: []string{"m"}}},
			"FAIL selftest: 80.0% (threshold 60.0%), 0/2 cases failed, success rate 75.0% (3/4 attempts); criterion below floor 50.0%: a 10.0% (1/10) [repeat 2: min 70.0%, max 90.0%] [cost incomplete: no price for m]\n" +
				"  a: 1/10 (10.0%)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printAgentVerdict(&buf, "selftest", tt.v)
			if buf.String() != tt.want {
				t.Errorf("got  %q\nwant %q", buf.String(), tt.want)
			}
		})
	}
}

// TestSelftestFailRepeatAttempts runs selftest-fail with the stub backend and
// two repeats and checks the recorded attempt counts against the case results.
func TestSelftestFailRepeatAttempts(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	out, err := runStubCapturing(t, evalsDir, "selftest-fail", "", RunOptions{Repeat: 2})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed\n%s", err, out)
	}
	runDir := onlyRunDir(t, evalsDir)

	var res AgentResult
	readJSON(t, filepath.Join(runDir, "selftest-fail.json"), &res)
	var sum Summary
	readJSON(t, filepath.Join(runDir, "summary.json"), &sum)
	v := sum.AgentVerdicts["selftest-fail"]

	wantPassed := 0
	for _, c := range res.Cases {
		if c.Passed != nil && *c.Passed {
			wantPassed++
		}
	}
	if len(res.Cases) == 0 {
		t.Fatal("no case results written")
	}
	if v.AttemptsTotal != len(res.Cases) {
		t.Errorf("attempts_total = %d, want %d case results", v.AttemptsTotal, len(res.Cases))
	}
	if v.AttemptsPassed != wantPassed {
		t.Errorf("attempts_passed = %d, want %d", v.AttemptsPassed, wantPassed)
	}
	if v.AttemptsTotal <= v.CasesTotal {
		t.Errorf("attempts_total %d must exceed cases_total %d with --repeat 2", v.AttemptsTotal, v.CasesTotal)
	}
	if want := float64(wantPassed) * 100 / float64(len(res.Cases)); math.Abs(v.SuccessRate-want) > 1e-9 {
		t.Errorf("success_rate = %v, want %v", v.SuccessRate, want)
	}

	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL selftest-fail: ") {
		t.Fatalf("verdict lines = %q\n%s", lines, out)
	}
	wantText := fmt.Sprintf("cases failed, success rate %.1f%% (%d/%d attempts)", v.SuccessRate, wantPassed, len(res.Cases))
	if !strings.Contains(lines[0], wantText) {
		t.Errorf("FAIL line %q lacks %q", lines[0], wantText)
	}
	if !regexp.MustCompile(`cases failed, success rate \d+\.\d%`).MatchString(lines[0]) {
		t.Errorf("FAIL line %q does not match the success-rate pattern", lines[0])
	}

	// The summary on disk carries the three keys.
	raw, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{`"attempts_total"`, `"attempts_passed"`, `"success_rate"`} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("summary.json lacks %s", k)
		}
	}
}
