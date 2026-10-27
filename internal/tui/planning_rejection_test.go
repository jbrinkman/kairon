package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

func rejectionFailure() guard.FailureContext {
	return guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    "extract",
		Attempt:  guard.Attempt{Current: 1, Max: 1},
		Expected: "a stage-output document in YAML",
		Actual:   "line one of actual\nline two of actual",
	}
}

// rejectionFixture returns a model with an on-disk document file and the
// dialog open for it. calls records every Resume invocation.
func rejectionFixture(t *testing.T, resume func(model, guard.FailureContext) (model, tea.Cmd)) (model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s1.document.yaml")
	if err := os.WriteFile(path, []byte("doc: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := validateTestModel(t, nil)
	m = m.openPlanningRejection(planningEnd{SessionID: "s1", Resume: resume}, rejectionFailure(), path)
	return m, path
}

func overlayText(m model) string {
	return m.overlayContent.title + "\n" + strings.Join(m.overlayContent.content, "\n")
}

func pressKey(t *testing.T, m model, k tea.Key) (model, tea.Cmd) {
	t.Helper()
	out, cmd := m.Update(tea.KeyPressMsg(k))
	return out.(model), cmd
}

func TestRejectionDialogShowsGuardExpectedActual(t *testing.T) {
	m, path := rejectionFixture(t, nil)

	if m.activeOverlay != overlayPlanningRejection {
		t.Fatalf("activeOverlay = %v, want overlayPlanningRejection", m.activeOverlay)
	}
	text := overlayText(m)
	for _, want := range []string{
		"Guard: extract",
		"Expected: a stage-output document in YAML",
		"line one of actual",
		"line two of actual",
		path,
		"Resume planning",
		"Dismiss",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("overlay content missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Press ESC to close") {
		t.Errorf("generic close footer must not be shown:\n%s", text)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("opening the dialog must keep the file: %v", err)
	}
}

func TestRejectionDialogDismissKeepsFile(t *testing.T) {
	keys := map[string]tea.Key{
		"d":   {Code: 'd', Text: "d"},
		"esc": {Code: tea.KeyEscape},
	}
	for name, k := range keys {
		t.Run(name, func(t *testing.T) {
			var calls int
			m, path := rejectionFixture(t, func(m model, _ guard.FailureContext) (model, tea.Cmd) {
				calls++
				return m, nil
			})

			m, _ = pressKey(t, m, k)

			if m.activeOverlay != overlayNone {
				t.Errorf("activeOverlay = %v, want overlayNone", m.activeOverlay)
			}
			if m.planningRejection != nil {
				t.Error("planningRejection state not cleared")
			}
			if calls != 0 {
				t.Errorf("Resume called %d times on dismiss", calls)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("dismiss must keep the file: %v", err)
			}
		})
	}
}

func TestRejectionDialogResumeCallsFrontEnd(t *testing.T) {
	keys := map[string]tea.Key{
		"r":     {Code: 'r', Text: "r"},
		"enter": {Code: tea.KeyEnter},
	}
	for name, k := range keys {
		t.Run(name, func(t *testing.T) {
			var got []guard.FailureContext
			sentinel := struct{ tea.Msg }{}
			m, path := rejectionFixture(t, func(m model, fc guard.FailureContext) (model, tea.Cmd) {
				got = append(got, fc)
				return m, func() tea.Msg { return sentinel }
			})

			m, cmd := pressKey(t, m, k)

			if m.activeOverlay != overlayNone {
				t.Errorf("activeOverlay = %v, want overlayNone", m.activeOverlay)
			}
			if len(got) != 1 {
				t.Fatalf("Resume called %d times, want 1", len(got))
			}
			if got[0] != rejectionFailure() {
				t.Errorf("Resume got %+v, want %+v", got[0], rejectionFailure())
			}
			if cmd == nil {
				t.Fatal("Resume's command was dropped")
			}
			if msg := cmd(); msg != sentinel {
				t.Errorf("returned command yielded %v, want Resume's command", msg)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("resume must keep the file: %v", err)
			}
		})
	}
}

func TestRejectionDialogNilResumeShowsError(t *testing.T) {
	m, path := rejectionFixture(t, nil)

	m, _ = pressKey(t, m, tea.Key{Code: 'r', Text: "r"})

	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone", m.activeOverlay)
	}
	if got := activityText(m); !strings.Contains(strings.ToLower(got), "cannot resume") || !strings.Contains(got, path) {
		t.Errorf("want a console error naming the kept file, got %q", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file must be kept: %v", err)
	}
}

func TestRejectionDialogSwallowsOtherKeys(t *testing.T) {
	var calls int
	m, _ := rejectionFixture(t, func(m model, _ guard.FailureContext) (model, tea.Cmd) {
		calls++
		return m, nil
	})

	m, _ = pressKey(t, m, tea.Key{Code: 'x', Text: "x"})

	if m.activeOverlay != overlayPlanningRejection {
		t.Errorf("an unrelated key closed the dialog: %v", m.activeOverlay)
	}
	if calls != 0 {
		t.Errorf("Resume called %d times for an unrelated key", calls)
	}
}
