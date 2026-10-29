package failure

import (
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

func renderFC() guard.FailureContext {
	return guard.FailureContext{
		Stage:    stageoutput.StageKrewLead,
		Guard:    "implementation-run",
		Attempt:  guard.Attempt{Current: 4, Max: 4},
		Expected: "krew-lead exits 0 and a pull request exists",
		Actual:   "exit code 1",
	}
}

func TestRenderComment_ContainsStageAttemptsExpectedActualRemedy(t *testing.T) {
	fc := renderFC()
	fc.Attempt = guard.Attempt{Current: 2, Max: 3}
	got := RenderComment(fc, "run-1", "Read .kairon/logs/issue-5.log, fix the cause.", "kairon-failed")

	if first, _, _ := strings.Cut(got, "\n"); first != Marker("run-1") {
		t.Errorf("first line = %q, want the marker", first)
	}
	if id, ok := ParseMarker(got); !ok || id != "run-1" {
		t.Errorf("ParseMarker(rendered) = %q, %v", id, ok)
	}
	for _, want := range []string{
		"krew-lead",
		"implementation-run",
		"2 of 3",
		fc.Expected,
		fc.Actual,
		"Read .kairon/logs/issue-5.log, fix the cause.",
		"kairon-failed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered comment missing %q:\n%s", want, got)
		}
	}
}

func TestRenderComment_DefaultRemedy(t *testing.T) {
	fc := renderFC()
	got := RenderComment(fc, "r", "", "kairon-failed")
	if !strings.Contains(got, "Make this true, then remove the label") {
		t.Errorf("missing default remedy:\n%s", got)
	}
	if strings.Count(got, fc.Expected) < 2 {
		t.Errorf("default remedy should repeat the expected value:\n%s", got)
	}
}

// testBacktickRun returns the length of the longest run of backticks in s.
func testBacktickRun(s string) int {
	best, cur := 0, 0
	for _, r := range s {
		if r == '`' {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

// fenceOpeners returns the backtick-only fence lines of a rendered comment.
func fenceLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Trim(l, "`") == "" && l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestRenderComment_NeutralizesUntrustedValues(t *testing.T) {
	t.Run("fence breakout", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = "before\n```\n## injected heading\n``````\nafter"
		got := RenderComment(fc, "r", "x", "kairon-failed")
		fences := fenceLines(got)
		if len(fences) < 4 {
			t.Fatalf("want at least 4 fence lines, got %v in:\n%s", fences, got)
		}
		// The fence that wraps Actual must be longer than its longest run (6).
		found := false
		for _, f := range fences {
			if len(f) > 6 {
				found = true
			}
		}
		if !found {
			t.Errorf("no fence longer than the 6-backtick run in the value:\n%s", got)
		}
		// Every fence is at least 3 long.
		for _, f := range fences {
			if len(f) < 3 {
				t.Errorf("fence %q shorter than 3", f)
			}
		}
	})

	t.Run("fence is longer than any run in each value", func(t *testing.T) {
		fc := renderFC()
		fc.Expected = "a `` b ````` c"
		fc.Actual = "```` d"
		got := RenderComment(fc, "r", "x", "kairon-failed")
		// Locate each value and check the fence lines directly around it.
		lines := strings.Split(got, "\n")
		for i, l := range lines {
			if strings.Contains(l, "``` ") || l == fc.Expected || l == fc.Actual {
				open, closeL := lines[i-1], lines[i+1]
				if open != closeL || strings.Trim(open, "`") != "" {
					t.Errorf("value %q not wrapped by matching fences: %q ... %q", l, open, closeL)
				}
				if len(open) <= testBacktickRun(l) {
					t.Errorf("fence %q not longer than run in %q", open, l)
				}
			}
		}
	})

	t.Run("injected marker is not the first line", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = Marker("attacker") + "\nmore"
		fc.Expected = "\n" + Marker("attacker2")
		got := RenderComment(fc, "real", "x", "kairon-failed")
		id, ok := ParseMarker(got)
		if !ok || id != "real" {
			t.Errorf("ParseMarker = %q, %v; want real", id, ok)
		}
	})

	t.Run("truncated to 2000 runes", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = strings.Repeat("é", 5000)
		fc.Expected = strings.Repeat("x", 2000)
		got := RenderComment(fc, "r", "x", "kairon-failed")
		if n := strings.Count(got, "é"); n != 2000 {
			t.Errorf("é count = %d, want 2000 (truncated)", n)
		}
		if n := strings.Count(got, "x"); n < 2000 {
			t.Errorf("exact-limit value was truncated: x count = %d", n)
		}
		if !strings.Contains(got, "truncated") {
			t.Errorf("truncated value should say so:\n%.400s", got)
		}
	})

	t.Run("not truncated at the limit", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = strings.Repeat("é", 2000)
		got := RenderComment(fc, "r", "x", "kairon-failed")
		if n := strings.Count(got, "é"); n != 2000 {
			t.Errorf("é count = %d, want 2000", n)
		}
		if strings.Contains(got, "truncated") {
			t.Error("value at the limit must not be marked truncated")
		}
	})

	t.Run("truncation happens before fence sizing", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = strings.Repeat("a", 2000) + strings.Repeat("`", 50)
		got := RenderComment(fc, "r", "x", "kairon-failed")
		for _, f := range fenceLines(got) {
			if len(f) > 10 {
				t.Errorf("fence of %d backticks sized from text that was cut", len(f))
			}
		}
	})

	t.Run("single-line fields cannot break structure", func(t *testing.T) {
		fc := renderFC()
		fc.Guard = "evil`\n## heading"
		got := RenderComment(fc, "r", "x", "kairon-failed`\n## label")
		if strings.Contains(got, "\n## heading") || strings.Contains(got, "\n## label") {
			t.Errorf("guard or label injected a line:\n%s", got)
		}
	})
}
