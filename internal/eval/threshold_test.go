package eval

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRubricThreshold(t *testing.T) {
	if got := (Rubric{}).Threshold(); got != 95.0 {
		t.Errorf("default rubric threshold = %v, want 95", got)
	}
	if DefaultPassThreshold != 95.0 {
		t.Errorf("DefaultPassThreshold = %v, want 95", DefaultPassThreshold)
	}
	if got := (Rubric{PassThreshold: floatPtr(90)}).Threshold(); got != 90.0 {
		t.Errorf("rubric threshold = %v, want 90", got)
	}
}

func TestCaseThreshold(t *testing.T) {
	tests := []struct {
		name   string
		tc     TestCase
		rubric Rubric
		want   float64
	}{
		{"no min_score and no pass_threshold defaults to 95", TestCase{Name: "c"}, Rubric{}, 95},
		{"no min_score inherits rubric pass_threshold", TestCase{Name: "c"}, Rubric{PassThreshold: floatPtr(88)}, 88},
		{"min_score below rubric threshold overrides it", TestCase{Name: "c", MinScore: floatPtr(60)}, Rubric{PassThreshold: floatPtr(90)}, 60},
		{"min_score above rubric threshold overrides it", TestCase{Name: "c", MinScore: floatPtr(99)}, Rubric{PassThreshold: floatPtr(80)}, 99},
		{"min_score overrides the default rubric threshold", TestCase{Name: "c", MinScore: floatPtr(50)}, Rubric{}, 50},
		{"min_score 0 is an explicit override", TestCase{Name: "c", MinScore: floatPtr(0)}, Rubric{PassThreshold: floatPtr(90)}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caseThreshold(tt.tc, tt.rubric); got != tt.want {
				t.Errorf("caseThreshold = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadRubricsPassThreshold(t *testing.T) {
	load := func(t *testing.T, passThreshold string) ([]Rubric, error) {
		t.Helper()
		dir := chdirTemp(t)
		body := "agent: a1\n" + passThreshold + "criteria:\n  - name: c\n    scoring: \"1-5\"\n"
		writeCfgFile(t, filepath.Join(dir, "alt", "rubrics", "r.yaml"), body)
		if err := configure(RunOptions{EvalsDir: "alt"}); err != nil {
			t.Fatal(err)
		}
		return loadRubrics("")
	}

	t.Run("absent defaults to 95", func(t *testing.T) {
		rubrics, err := load(t, "")
		if err != nil || len(rubrics) != 1 {
			t.Fatalf("loadRubrics: %v, %d rubrics", err, len(rubrics))
		}
		if rubrics[0].PassThreshold != nil {
			t.Errorf("PassThreshold = %v, want nil", *rubrics[0].PassThreshold)
		}
		if got := rubrics[0].Threshold(); got != 95 {
			t.Errorf("Threshold() = %v, want 95", got)
		}
	})

	for _, tt := range []struct {
		name, yaml string
		want       float64
	}{
		{"value", "pass_threshold: 90\n", 90},
		{"fractional", "pass_threshold: 97.5\n", 97.5},
		{"upper bound 100", "pass_threshold: 100\n", 100},
		{"small positive", "pass_threshold: 0.5\n", 0.5},
	} {
		t.Run("accepts "+tt.name, func(t *testing.T) {
			rubrics, err := load(t, tt.yaml)
			if err != nil || len(rubrics) != 1 {
				t.Fatalf("loadRubrics: %v, %d rubrics", err, len(rubrics))
			}
			if got := rubrics[0].Threshold(); got != tt.want {
				t.Errorf("Threshold() = %v, want %v", got, tt.want)
			}
		})
	}

	for _, tt := range []struct{ name, yaml string }{
		{"zero", "pass_threshold: 0\n"},
		{"negative", "pass_threshold: -5\n"},
		{"above 100", "pass_threshold: 100.5\n"},
		{"NaN", "pass_threshold: .nan\n"},
		{"positive infinity", "pass_threshold: .inf\n"},
	} {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			_, err := load(t, tt.yaml)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "r.yaml") || !strings.Contains(err.Error(), "pass_threshold") {
				t.Errorf("error %q should name the rubric file and pass_threshold", err)
			}
		})
	}
}
