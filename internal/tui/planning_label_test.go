package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/planner"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
)

// labelCall is one AddLabel call the handoff made.
type labelCall struct {
	repo  string
	issue int
	label string
}

// fakeLabeler records every AddLabel call and returns err.
type fakeLabeler struct {
	mu    sync.Mutex
	calls []labelCall
	err   error
}

func (f *fakeLabeler) AddLabel(repo string, issue int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, labelCall{repo: repo, issue: issue, label: label})
	return f.err
}

func (f *fakeLabeler) recorded() []labelCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// writeLabelAnswer writes the label file for a session, as the planner does.
func writeLabelAnswer(t *testing.T, sessionID, content string) string {
	t.Helper()
	path, err := planningLabelPath(sessionID)
	if err != nil {
		t.Fatalf("planningLabelPath: %v", err)
	}
	writeDocument(t, path, content)
	return path
}

// labelFixture is handoffFixture plus a fake labeler, a document file and, when
// answer is not empty, a label file for session "s1". It returns the model,
// the document path, the label path and the labeler.
func labelFixture(t *testing.T, b planningBoundary, answer string) (model, string, string, *fakeLabeler) {
	t.Helper()
	m, docPath := handoffFixture(t, b)
	lab := &fakeLabeler{}
	m.planLabeler = lab
	writeDocument(t, docPath, "doc\n")
	labelPath, err := planningLabelPath("s1")
	if err != nil {
		t.Fatalf("planningLabelPath: %v", err)
	}
	if answer != "" {
		writeLabelAnswer(t, "s1", answer)
	}
	return m, docPath, labelPath, lab
}

func assertLabelCalls(t *testing.T, lab *fakeLabeler, want ...labelCall) {
	t.Helper()
	got := lab.recorded()
	if len(got) != len(want) {
		t.Fatalf("label calls = %d (%+v), want %d (%+v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label call %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// lineWith returns the first console line that contains every part, or "".
func lineWith(m model, parts ...string) string {
	for _, line := range m.activityLines {
		ok := true
		for _, p := range parts {
			if !strings.Contains(line, p) {
				ok = false
				break
			}
		}
		if ok {
			return line
		}
	}
	return ""
}

func TestHandoffLabelYesAppliesConfiguredLabel(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(77)}
	m, docPath, labelPath, lab := labelFixture(t, fb, "yes\n")

	m, cmd := m.finishPlanning(s1End())
	if cmd == nil {
		t.Fatal("finishPlanning returned no command")
	}
	if n := len(lab.recorded()); n != 0 {
		t.Fatalf("label calls before the command ran = %d, want 0; gh must not run on the UI goroutine", n)
	}
	next, _ := m.Update(cmd().(planningHandoffDoneMsg))
	m = next.(model)

	assertLabelCalls(t, lab, labelCall{repo: "o/r", issue: 77, label: "kairon"})
	if fb.calls() != 1 {
		t.Errorf("boundary calls = %d, want 1", fb.calls())
	}
	if fileExists(docPath) {
		t.Error("the document file must be deleted after a commit")
	}
	if fileExists(labelPath) {
		t.Error("the label file must be deleted after a commit")
	}
	if lineWith(m, "Added the label `kairon` to issue #77.") == "" {
		t.Errorf("console = %q, want the success line for the label", activityText(m))
	}
}

func TestHandoffLabelUsesConfiguredLabelName(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(77)}
	m, _, _, lab := labelFixture(t, fb, "yes")
	m.config.Label = "my-label"

	m = runHandoff(t, m)

	assertLabelCalls(t, lab, labelCall{repo: "o/r", issue: 77, label: "my-label"})
	if lineWith(m, "`my-label`", "#77") == "" {
		t.Errorf("console = %q, want a line naming my-label and #77", activityText(m))
	}
}

func TestHandoffLabelYesWithRealBoundary(t *testing.T) {
	store := &handoffStore{}
	m, docPath, labelPath, lab := labelFixture(t, realBoundary(t, store), "yes")
	writeDocument(t, docPath, plannertest.CanonicalReply("o/r", 1, plannertest.ValidBody("Dependencies: none")))

	m = runHandoff(t, m)

	if store.createCount() != 1 {
		t.Fatalf("creates = %d, want 1; console: %q", store.createCount(), activityText(m))
	}
	assertLabelCalls(t, lab, labelCall{repo: "o/r", issue: 321, label: "kairon"})
	if fileExists(docPath) || fileExists(labelPath) {
		t.Error("both session files must be deleted after a commit")
	}
}

func TestHandoffLabelNoOrMissingAnswerAppliesNothing(t *testing.T) {
	cases := map[string]string{
		"no":           "no",
		"uppercase NO": "NO\n",
		"no file":      "",
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			fb := &fakeBoundary{res: committedResult(77)}
			m, docPath, labelPath, lab := labelFixture(t, fb, answer)

			m = runHandoff(t, m)

			assertLabelCalls(t, lab)
			if fb.calls() != 1 {
				t.Errorf("boundary calls = %d, want 1; the issue is still created", fb.calls())
			}
			if strings.Contains(activityText(m), "label") {
				t.Errorf("console = %q, want no line about a label", activityText(m))
			}
			if lineWith(m, "Created issue #77") == "" {
				t.Errorf("console = %q, want the creation reported", activityText(m))
			}
			if fileExists(docPath) || fileExists(labelPath) {
				t.Error("both session files must be deleted after a commit")
			}
		})
	}
}

func TestHandoffLabelYesIsCaseAndWhitespaceTolerant(t *testing.T) {
	for _, answer := range []string{"YES", " yes \n", "Yes"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			fb := &fakeBoundary{res: committedResult(77)}
			m, _, _, lab := labelFixture(t, fb, answer)

			runHandoff(t, m)

			assertLabelCalls(t, lab, labelCall{repo: "o/r", issue: 77, label: "kairon"})
		})
	}
}

func TestHandoffLabelNotAppliedWithoutCommit(t *testing.T) {
	fc := rejectionFailure()
	cases := map[string]*fakeBoundary{
		"rejected with failure":    {res: planner.Result{Outcome: planner.OutcomeRejected, Failure: &fc}},
		"rejected without failure": {res: planner.Result{Outcome: planner.OutcomeRejected}},
		"process error":            {err: errors.New("gh: network is down")},
		"routed to intake":         {res: planner.Result{Outcome: planner.OutcomeRoutedToIntake, Number: 5}},
		"punched out":              {res: planner.Result{Outcome: planner.OutcomePunchedOut, Number: 5}},
	}
	for name, fb := range cases {
		t.Run(name, func(t *testing.T) {
			m, docPath, labelPath, lab := labelFixture(t, fb, "yes")

			m = runHandoff(t, m)

			assertLabelCalls(t, lab)
			if !fileExists(docPath) {
				t.Error("the document file must be kept")
			}
			if !fileExists(labelPath) {
				t.Error("the label file must be kept with the document")
			}
			if strings.Contains(activityText(m), "Added the label") {
				t.Errorf("console = %q, must not report a label", activityText(m))
			}
		})
	}
}

func TestHandoffLabelFailureNamesIssueAndDoesNotRecreate(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(77)}
	m, docPath, labelPath, lab := labelFixture(t, fb, "yes")
	lab.err = errors.New("gh issue edit failed: boom")

	m = runHandoff(t, m)

	if fb.calls() != 1 {
		t.Errorf("boundary calls = %d, want 1", fb.calls())
	}
	assertLabelCalls(t, lab, labelCall{repo: "o/r", issue: 77, label: "kairon"})
	line := lineWith(m, "#77", "boom")
	if line == "" {
		t.Fatalf("console = %q, want one line with the issue number and the error", activityText(m))
	}
	for _, want := range []string{"was created", "`kairon`", "by hand"} {
		if !strings.Contains(line, want) {
			t.Errorf("line = %q, want it to contain %q", line, want)
		}
	}
	if fileExists(docPath) {
		t.Error("the document file must be deleted, or the issue would be created again")
	}
	if fileExists(labelPath) {
		t.Error("the label file must be deleted with the document")
	}

	m2, cmd := m.finishPlanning(s1End())
	if cmd != nil {
		t.Error("a second end after a failed label returned a command")
	}
	if fb.calls() != 1 {
		t.Errorf("boundary calls after a second end = %d, want 1", fb.calls())
	}
	if len(m2.activityLines) != len(m.activityLines) {
		t.Errorf("a second end added console lines: %q", m2.activityLines)
	}
}

func TestHandoffLabelInvalidAnswerIsReported(t *testing.T) {
	cases := map[string]string{
		"maybe":          "maybe\n",
		"100 byte file":  strings.Repeat("y", 100),
		"yes and more":   "yes please",
		"directory file": "",
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			fb := &fakeBoundary{res: committedResult(77)}
			m, _, labelPath, lab := labelFixture(t, fb, answer)
			if name == "directory file" {
				if err := os.MkdirAll(labelPath, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			m = runHandoff(t, m)

			assertLabelCalls(t, lab)
			if fb.calls() != 1 {
				t.Errorf("boundary calls = %d, want 1", fb.calls())
			}
			if lineWith(m, "Created issue #77") == "" {
				t.Errorf("console = %q, want the creation reported", activityText(m))
			}
			if lineWith(m, "#77", "without the trigger label", "by hand") == "" {
				t.Errorf("console = %q, want a line naming #77 that says the label was not applied", activityText(m))
			}
		})
	}
}

func TestHandoffLabelMissingPortOrNameIsReported(t *testing.T) {
	t.Run("nil port", func(t *testing.T) {
		fb := &fakeBoundary{res: committedResult(77)}
		m, _, _, lab := labelFixture(t, fb, "yes")
		m.planLabeler = nil

		m = runHandoff(t, m)

		assertLabelCalls(t, lab)
		if fb.calls() != 1 {
			t.Errorf("boundary calls = %d, want 1", fb.calls())
		}
		if lineWith(m, "#77", "label", "failed") == "" {
			t.Errorf("console = %q, want a line naming #77 about the label", activityText(m))
		}
	})
	t.Run("empty label name", func(t *testing.T) {
		fb := &fakeBoundary{res: committedResult(77)}
		m, _, _, lab := labelFixture(t, fb, "yes")
		m.config.Label = ""

		m = runHandoff(t, m)

		assertLabelCalls(t, lab)
		if lineWith(m, "#77", "no trigger label is configured") == "" {
			t.Errorf("console = %q, want a line naming #77 and the missing label name", activityText(m))
		}
	})
}

func TestHandoffLabelKeptWhenNewerDocumentWritten(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(77)}
	m, docPath, labelPath, lab := labelFixture(t, fb, "yes")

	m, follow := rewriteDuringSend(t, m, docPath, "second\n")

	assertLabelCalls(t, lab, labelCall{repo: "o/r", issue: 77, label: "kairon"})
	if !fileExists(labelPath) {
		t.Fatal("the label file was deleted while a newer document is pending")
	}
	if !fileExists(docPath) {
		t.Fatal("the newer document was deleted")
	}
	if follow == nil {
		t.Fatal("the newer document was not sent")
	}
	done := follow().(planningHandoffDoneMsg)
	if fb.calls() != 2 || fb.reqs[1].Reply != "second\n" {
		t.Fatalf("requests = %+v, want a second send of the newer document", fb.reqs)
	}
	next, _ := m.Update(done)
	m = next.(model)
	if fileExists(docPath) || fileExists(labelPath) {
		t.Error("both session files must be deleted once the newer document is committed")
	}
}

func TestHandoffLabelFileRemovalFailureIsReported(t *testing.T) {
	m, _ := handoffFixture(t, &fakeBoundary{})
	// A non-empty directory cannot be removed with os.Remove.
	stuck := filepath.Join(t.TempDir(), "stuck.label")
	if err := os.MkdirAll(filepath.Join(stuck, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	seen, err := os.Stat(stuck)
	if err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(t.TempDir(), "gone.document.yaml")

	next, _ := m.Update(planningHandoffDoneMsg{end: s1End(), path: docPath, labelPath: stuck,
		label: labelAnswer{seen: seen}, result: committedResult(12)})
	m = next.(model)

	if lineWith(m, "#12", stuck) == "" {
		t.Errorf("console = %q, want a line naming #12 and %s", activityText(m), stuck)
	}
}

// The planner writes the label answer before the document. A commit that ends
// between those two writes sees the old document bytes, so the answer just
// written for the next document must survive the cleanup.
func TestHandoffLabelAnswerRewrittenDuringSendIsKept(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(77)}
	m, docPath, labelPath, lab := labelFixture(t, fb, "yes\n")

	m, cmd := m.finishPlanning(s1End())
	if cmd == nil {
		t.Fatal("finishPlanning returned no command")
	}
	done := cmd().(planningHandoffDoneMsg)
	// Same bytes, new write: the answer for the next document.
	writeLabelAnswer(t, "s1", "yes\n")
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(labelPath, later, later); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(done)
	m = next.(model)

	assertLabelCalls(t, lab, labelCall{repo: "o/r", issue: 77, label: "kairon"})
	if fileExists(docPath) {
		t.Error("the committed document file must be deleted")
	}
	if !fileExists(labelPath) {
		t.Error("the label answer written during the send was deleted")
	}
}

// A label file that did not exist when the document was read belongs to a
// later document and is kept.
func TestHandoffLabelAnswerCreatedDuringSendIsKept(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(77)}
	m, _, labelPath, lab := labelFixture(t, fb, "")

	m, cmd := m.finishPlanning(s1End())
	done := cmd().(planningHandoffDoneMsg)
	writeLabelAnswer(t, "s1", "yes\n")
	next, _ := m.Update(done)
	m = next.(model)

	assertLabelCalls(t, lab)
	if !fileExists(labelPath) {
		t.Error("the label answer written during the send was deleted")
	}
}

// blockingLabeler never returns from AddLabel until release is closed.
type blockingLabeler struct{ release chan struct{} }

func (b *blockingLabeler) AddLabel(string, int, string) error {
	<-b.release
	return nil
}

// A stalled label call must not hold the handoff: the created issue is still
// reported, with the label failure, and the document is removed.
func TestHandoffLabelCallThatStallsTimesOut(t *testing.T) {
	old := planningLabelTimeout
	planningLabelTimeout = 50 * time.Millisecond
	t.Cleanup(func() { planningLabelTimeout = old })

	fb := &fakeBoundary{res: committedResult(77)}
	m, docPath, _, _ := labelFixture(t, fb, "yes")
	stall := &blockingLabeler{release: make(chan struct{})}
	t.Cleanup(func() { close(stall.release) })
	m.planLabeler = stall

	m, cmd := m.finishPlanning(s1End())
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()
	var done planningHandoffDoneMsg
	select {
	case msg := <-got:
		done = msg.(planningHandoffDoneMsg)
	case <-time.After(5 * time.Second):
		t.Fatal("the handoff command did not return while the label call stalled")
	}
	if !done.labelTried || !errors.Is(done.labelErr, context.DeadlineExceeded) {
		t.Fatalf("labelTried = %v, labelErr = %v; want a tried call that hit its deadline", done.labelTried, done.labelErr)
	}
	next, _ := m.Update(done)
	m = next.(model)
	if lineWith(m, "Created issue #77") == "" || lineWith(m, "#77", "adding the label `kairon` failed") == "" {
		t.Errorf("console = %q, want the created issue and the label failure", activityText(m))
	}
	if fileExists(docPath) {
		t.Error("the committed document file must be deleted")
	}
}
