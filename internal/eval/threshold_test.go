package eval

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetThreshold(t *testing.T) {
	// No min_score and no rubric pass_threshold: the rubric default.
	if got := getThreshold(Rubric{}, TestCase{Name: "default-test"}); got != DefaultPassThreshold {
		t.Errorf("default threshold = %v, want %v", got, DefaultPassThreshold)
	}

	// No min_score: inherits the rubric's pass_threshold.
	if got := getThreshold(Rubric{PassThreshold: floatPtr(70)}, TestCase{Name: "inherit"}); got != 70 {
		t.Errorf("inherited threshold = %v, want 70", got)
	}

	// min_score overrides the rubric, higher or lower.
	for _, min := range []float64{90, 60, 0} {
		tc := TestCase{Name: "custom", MinScore: floatPtr(min)}
		if got := getThreshold(Rubric{PassThreshold: floatPtr(70)}, tc); got != min {
			t.Errorf("min_score %v: threshold = %v", min, got)
		}
	}
}

func TestRubricThresholdDefaultAndOverride(t *testing.T) {
	if got := (Rubric{}).Threshold(); got != 95 {
		t.Errorf("default Threshold() = %v, want 95", got)
	}
	if got := (Rubric{PassThreshold: floatPtr(80)}).Threshold(); got != 80 {
		t.Errorf("Threshold() = %v, want 80", got)
	}
	// An explicit 0 is a real value, not "unset".
	if got := (Rubric{PassThreshold: floatPtr(0)}).Threshold(); got != 0 {
		t.Errorf("Threshold() with pass_threshold 0 = %v, want 0", got)
	}
}

func writeRubricFile(t *testing.T, name, body string) {
	t.Helper()
	dir := evalsPath("rubrics")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const rubricBody = "agent: a1\ncriteria:\n  - name: x\n    scoring: \"1-5\"\n"

func TestLoadRubricsPassThreshold(t *testing.T) {
	chdirTemp(t)

	writeRubricFile(t, "none.yaml", rubricBody)
	rs, err := loadRubrics("a1")
	if err != nil || len(rs) != 1 {
		t.Fatalf("loadRubrics: %v, %d rubrics", err, len(rs))
	}
	if rs[0].PassThreshold != nil || rs[0].Threshold() != 95 {
		t.Errorf("unset pass_threshold: field %v, Threshold() %v", rs[0].PassThreshold, rs[0].Threshold())
	}

	writeRubricFile(t, "none.yaml", "pass_threshold: 82.5\n"+rubricBody)
	rs, err = loadRubrics("a1")
	if err != nil || len(rs) != 1 {
		t.Fatalf("loadRubrics: %v, %d rubrics", err, len(rs))
	}
	if rs[0].Threshold() != 82.5 {
		t.Errorf("Threshold() = %v, want 82.5", rs[0].Threshold())
	}

	for _, ok := range []string{"0", "100"} {
		writeRubricFile(t, "none.yaml", "pass_threshold: "+ok+"\n"+rubricBody)
		if _, err := loadRubrics("a1"); err != nil {
			t.Errorf("pass_threshold %s rejected: %v", ok, err)
		}
	}
}

func TestLoadRubricsRejectsBadPassThreshold(t *testing.T) {
	for _, bad := range []string{"-1", "100.5", "101", ".nan", ".inf", "-.inf"} {
		t.Run(bad, func(t *testing.T) {
			chdirTemp(t)
			writeRubricFile(t, "bad-rubric.yaml", "pass_threshold: "+bad+"\n"+rubricBody)
			_, err := loadRubrics("")
			if err == nil {
				t.Fatalf("pass_threshold %s accepted", bad)
			}
			if !strings.Contains(err.Error(), "bad-rubric.yaml") || !strings.Contains(err.Error(), "pass_threshold") {
				t.Errorf("error %q does not name the file and field", err)
			}
		})
	}
}

func TestScorePercentIsExactAtBoundary(t *testing.T) {
	if got := scorePercent(19, 20); got != 95 {
		t.Errorf("scorePercent(19, 20) = %v, want exactly 95", got)
	}
	if got := scorePercent(19, 20); !(got >= 95.0) {
		t.Errorf("19/20 does not meet a 95 bar: %v", got)
	}
	if got := scorePercent(0, 0); got != 0 || math.IsNaN(got) {
		t.Errorf("scorePercent(0, 0) = %v, want 0", got)
	}
}
