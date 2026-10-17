package eval

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestSelfTestFloorFailsOnLowCriterion is the end-to-end proof of
// min_criterion_score: the selftest-floor case scores 2/3 = 66.7% overall,
// above its 60% threshold, but report_marker is 0/1 under a 50% floor, so the
// agent fails. Before the floor existed the key was ignored and the run passed.
func TestSelfTestFloorFailsOnLowCriterion(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	out, err := runStubCapturing(t, evalsDir, "selftest-floor", "", RunOptions{})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed (the floor must fail the agent)\n%s", err, out)
	}
	if !strings.Contains(err.Error(), "report_marker") {
		t.Errorf("error %q does not name report_marker", err)
	}
	if !strings.Contains(err.Error(), "selftest-floor (criterion report_marker 0.0% < floor 50.0%)") {
		t.Errorf("error %q does not carry the floor text", err)
	}

	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL selftest-floor: ") ||
		!strings.HasSuffix(lines[0], "; criterion below floor 50.0%: report_marker 0.0% (0/1)") {
		t.Fatalf("verdict lines = %q\n%s", lines, out)
	}

	runDir := onlyRunDir(t, evalsDir)
	var s Summary
	readJSON(t, filepath.Join(runDir, "summary.json"), &s)
	v, ok := s.AgentVerdicts["selftest-floor"]
	if !ok {
		t.Fatalf("summary has no selftest-floor verdict: %+v", s.AgentVerdicts)
	}
	if v.Passed {
		t.Error("verdict passed = true, want false")
	}
	if math.Abs(v.Score-200.0/3) > 1e-9 || v.Score < v.Threshold {
		t.Errorf("score = %v, threshold = %v, want 66.7 above the threshold", v.Score, v.Threshold)
	}
	if v.MinCriterionScore == nil || *v.MinCriterionScore != 50 {
		t.Errorf("verdict min_criterion_score = %v, want 50", v.MinCriterionScore)
	}
	if want := []string{"report_marker"}; !reflect.DeepEqual(v.CriteriaBelowFloor, want) {
		t.Errorf("criteria_below_floor = %v, want %v", v.CriteriaBelowFloor, want)
	}
	if v.CasesTotal != 1 || v.CasesFailed != 0 {
		t.Errorf("cases = %d total, %d failed, want 1, 0: the case itself passes", v.CasesTotal, v.CasesFailed)
	}
	wantCriteria := []CriterionTotal{
		{Name: "file_written", Score: 2, MaxScore: 2, Percent: 100},
		{Name: "report_marker", Score: 0, MaxScore: 1, Percent: 0},
	}
	if !reflect.DeepEqual(v.Criteria, wantCriteria) {
		t.Errorf("criteria = %+v, want %+v", v.Criteria, wantCriteria)
	}

	var res AgentResult
	readJSON(t, filepath.Join(runDir, "selftest-floor.json"), &res)
	if res.MinCriterionScore == nil || *res.MinCriterionScore != 50 {
		t.Errorf("selftest-floor.json min_criterion_score = %v, want 50", res.MinCriterionScore)
	}
}

// TestSelfTestFloorControlWithoutFloorPasses removes min_criterion_score from a
// copy of the rubric: the same run then passes with the same score, proving the
// floor is the only cause of the failure.
func TestSelfTestFloorControlWithoutFloorPasses(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	rubricPath := filepath.Join(evalsDir, "rubrics", "selftest-floor.yaml")
	data, err := os.ReadFile(rubricPath)
	if err != nil {
		t.Fatal(err)
	}
	stripped := regexp.MustCompile(`(?m)^min_criterion_score:.*\n`).ReplaceAllString(string(data), "")
	if stripped == string(data) {
		t.Fatal("rubric has no min_criterion_score line to remove")
	}
	if err := os.WriteFile(rubricPath, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runStubCapturing(t, evalsDir, "selftest-floor", "", RunOptions{})
	if err != nil {
		t.Fatalf("control run err = %v, want nil\n%s", err, out)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || lines[0] != "PASS selftest-floor: 66.7% (threshold 60.0%), 0/1 cases failed" {
		t.Fatalf("verdict lines = %q\n%s", lines, out)
	}
	var s Summary
	readJSON(t, filepath.Join(onlyRunDir(t, evalsDir), "summary.json"), &s)
	v := s.AgentVerdicts["selftest-floor"]
	if !v.Passed || v.MinCriterionScore != nil || v.CriteriaBelowFloor != nil {
		t.Errorf("control verdict = %+v, want passed with no floor fields", v)
	}
}

// TestSelfTestFloorResumeKeepsFloor resumes an interrupted selftest-floor run;
// the floor is re-stamped from the rubric, so the resumed agent still fails.
func TestSelfTestFloorResumeKeepsFloor(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	if _, err := runStubCapturing(t, evalsDir, "selftest-floor", "", RunOptions{}); !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("first run err = %v, want ErrThresholdFailed", err)
	}
	runDir := onlyRunDir(t, evalsDir)
	if err := os.WriteFile(filepath.Join(runDir, ".progress"), []byte("in-progress"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runStubCapturing(t, evalsDir, "selftest-floor", "", RunOptions{Resume: true})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("resume err = %v, want ErrThresholdFailed\n%s", err, out)
	}
	if lines := verdictLines(out); len(lines) != 1 || !strings.Contains(lines[0], "criterion below floor 50.0%: report_marker") {
		t.Fatalf("verdict lines = %q\n%s", lines, out)
	}
	var res AgentResult
	readJSON(t, filepath.Join(runDir, "selftest-floor.json"), &res)
	if res.MinCriterionScore == nil || *res.MinCriterionScore != 50 {
		t.Errorf("resumed min_criterion_score = %v, want 50", res.MinCriterionScore)
	}
}
