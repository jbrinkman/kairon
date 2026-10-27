package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/planner"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// fakeBoundary records every request and returns a canned result.
type fakeBoundary struct {
	mu   sync.Mutex
	reqs []planner.Request
	res  planner.Result
	err  error
}

func (f *fakeBoundary) Process(_ context.Context, req planner.Request) (planner.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	return f.res, f.err
}

func (f *fakeBoundary) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

// handoffFixture runs in a temp working directory and returns a model that
// uses the given boundary, plus the document path for session "s1".
func handoffFixture(t *testing.T, b planningBoundary) (model, string) {
	t.Helper()
	t.Chdir(t.TempDir())
	m := validateTestModel(t, nil)
	m.planBoundary = b
	path, err := planningDocumentPath("s1")
	if err != nil {
		t.Fatalf("planningDocumentPath: %v", err)
	}
	return m, path
}

func writeDocument(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func s1End() planningEnd { return planningEnd{SessionID: "s1"} }

// runHandoff ends session "s1", runs the returned command and applies the
// done message it produces.
func runHandoff(t *testing.T, m model) model {
	t.Helper()
	m, cmd := m.finishPlanning(s1End())
	if cmd == nil {
		t.Fatal("finishPlanning returned no command for a present document")
	}
	msg := cmd()
	done, ok := msg.(planningHandoffDoneMsg)
	if !ok {
		t.Fatalf("command returned %#v, want planningHandoffDoneMsg", msg)
	}
	next, _ := m.Update(done)
	return next.(model)
}

func committedResult(n int) planner.Result {
	return planner.Result{Outcome: planner.OutcomeCommitted, Number: n}
}

func TestHandoffCommittedProcessesOnceAndDeletesFile(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(77)}
	m, path := handoffFixture(t, fb)
	doc := "schema_version: 1\nstage: planner\n"
	writeDocument(t, path, doc)

	m, cmd := m.finishPlanning(s1End())
	if cmd == nil {
		t.Fatal("finishPlanning returned no command")
	}
	if fb.calls() != 0 {
		t.Fatal("the boundary ran on the caller's goroutine; it must run inside the returned command")
	}
	done := cmd().(planningHandoffDoneMsg)
	next, _ := m.Update(done)
	m = next.(model)

	if fb.calls() != 1 {
		t.Fatalf("boundary calls = %d, want 1", fb.calls())
	}
	req := fb.reqs[0]
	if req.Reply != doc {
		t.Errorf("Reply = %q, want the file contents %q", req.Reply, doc)
	}
	if req.Target.Number != 0 {
		t.Errorf("Target.Number = %d, want 0 (create path)", req.Target.Number)
	}
	if req.Attempt != (guard.Attempt{Current: 1, Max: 1}) {
		t.Errorf("Attempt = %+v, want 1 of 1", req.Attempt)
	}
	if fileExists(path) {
		t.Error("the document file must be deleted after a commit")
	}
	got := activityText(m)
	for _, want := range []string{"#77", "https://github.com/o/r/issues/77"} {
		if !strings.Contains(got, want) {
			t.Errorf("console = %q, want it to contain %q", got, want)
		}
	}
	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone", m.activeOverlay)
	}
	if m.planningHandoffs["s1"] {
		t.Error("the in-flight marker must be cleared")
	}
}

func TestHandoffNoFileDoesNothing(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(1)}
	m, _ := handoffFixture(t, fb)

	m2, cmd := m.finishPlanning(s1End())

	if cmd != nil {
		t.Error("finishPlanning returned a command with no document file")
	}
	if fb.calls() != 0 {
		t.Errorf("boundary calls = %d, want 0", fb.calls())
	}
	if len(m2.activityLines) != 0 {
		t.Errorf("console lines = %q, want none", m2.activityLines)
	}
	if m2.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone", m2.activeOverlay)
	}
}

func TestHandoffProcessErrorKeepsFile(t *testing.T) {
	fb := &fakeBoundary{err: errors.New("gh: network is down")}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "doc\n")

	m = runHandoff(t, m)

	if !fileExists(path) {
		t.Error("the file must be kept when the boundary returns an error")
	}
	got := activityText(m)
	if !strings.Contains(got, "gh: network is down") {
		t.Errorf("console = %q, want the error cause", got)
	}
	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone", m.activeOverlay)
	}
	if m.planningHandoffs["s1"] {
		t.Error("the in-flight marker must be cleared after an error")
	}
}

func TestHandoffRejectedOpensDialogAndKeepsFile(t *testing.T) {
	fc := rejectionFailure()
	fb := &fakeBoundary{res: planner.Result{Outcome: planner.OutcomeRejected, Failure: &fc}}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "doc\n")

	m = runHandoff(t, m)

	if m.activeOverlay != overlayPlanningRejection {
		t.Fatalf("activeOverlay = %v, want overlayPlanningRejection", m.activeOverlay)
	}
	text := overlayText(m)
	for _, want := range []string{"Guard: extract", "Expected: a stage-output document in YAML", "line two of actual", path} {
		if !strings.Contains(text, want) {
			t.Errorf("dialog missing %q:\n%s", want, text)
		}
	}
	if m.planningRejection == nil || m.planningRejection.end.SessionID != "s1" {
		t.Errorf("planningRejection = %+v, want the end for session s1", m.planningRejection)
	}
	if !fileExists(path) {
		t.Error("the file must be kept after a rejection")
	}
	console := activityText(m)
	if !strings.Contains(console, "extract") || !strings.Contains(console, path) {
		t.Errorf("console = %q, want a line naming the guard and the kept file", console)
	}
}

func TestHandoffRejectedWithoutFailureStillOpensDialog(t *testing.T) {
	fb := &fakeBoundary{res: planner.Result{Outcome: planner.OutcomeRejected}}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "doc\n")

	m = runHandoff(t, m)

	if m.activeOverlay != overlayPlanningRejection {
		t.Fatalf("activeOverlay = %v, want overlayPlanningRejection", m.activeOverlay)
	}
	if !fileExists(path) {
		t.Error("the file must be kept")
	}
}

func TestHandoffUnexpectedOutcomeIsAnErrorAndKeepsFile(t *testing.T) {
	fb := &fakeBoundary{res: planner.Result{Outcome: planner.OutcomeRoutedToIntake, Number: 5}}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "doc\n")

	m = runHandoff(t, m)

	if !fileExists(path) {
		t.Error("the file must be kept for an unexpected outcome")
	}
	got := activityText(m)
	if !strings.Contains(got, "unexpected outcome") {
		t.Errorf("console = %q, want 'unexpected outcome'", got)
	}
	if strings.Contains(got, "https://github.com") {
		t.Errorf("console = %q, an unexpected outcome must not read as success", got)
	}
}

func TestHandoffSecondEndAfterCommitDoesNothing(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(9)}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "doc\n")
	m = runHandoff(t, m)

	m2, cmd := m.finishPlanning(s1End())

	if cmd != nil {
		t.Error("a second end after a commit returned a command")
	}
	if fb.calls() != 1 {
		t.Errorf("boundary calls = %d, want 1", fb.calls())
	}
	if len(m2.activityLines) != len(m.activityLines) {
		t.Errorf("a second end added console lines: %q", m2.activityLines)
	}
}

func TestHandoffInFlightGuardSendsOnce(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(9)}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "doc\n")

	m, first := m.finishPlanning(s1End())
	m, second := m.finishPlanning(s1End())

	if first == nil {
		t.Fatal("the first end returned no command")
	}
	if second != nil {
		t.Error("the second end, while the first is in flight, returned a command")
	}
	msg := first()
	if fb.calls() != 1 {
		t.Errorf("boundary calls = %d, want 1", fb.calls())
	}

	// Once the first handoff is done and the document was rejected, the same
	// session can be handed off again.
	fb.res = planner.Result{Outcome: planner.OutcomeRejected}
	done := msg.(planningHandoffDoneMsg)
	done.result = fb.res
	next, _ := m.Update(done)
	m = next.(model)
	if _, again := m.finishPlanning(s1End()); again == nil {
		t.Error("after the first handoff finished with the file kept, a new end must run again")
	}
}

// rewriteDuringSend ends session "s1", writes newer to the document file
// before the send's done message is applied, and returns the model after that
// message plus the command it returned.
func rewriteDuringSend(t *testing.T, m model, path, newer string) (model, tea.Cmd) {
	t.Helper()
	m, cmd := m.finishPlanning(s1End())
	if cmd == nil {
		t.Fatal("finishPlanning returned no command for a present document")
	}
	done := cmd().(planningHandoffDoneMsg)
	writeDocument(t, path, newer)
	next, follow := m.Update(done)
	return next.(model), follow
}

func TestHandoffCommitKeepsAndSendsNewerDocument(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(5)}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "first\n")

	m, follow := rewriteDuringSend(t, m, path, "second\n")

	if !fileExists(path) {
		t.Fatal("a commit of the earlier document deleted the newer one")
	}
	if !strings.Contains(activityText(m), "#5") {
		t.Errorf("console = %q, want the commit of the earlier document reported", activityText(m))
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
	if fileExists(path) {
		t.Error("the newer document must be deleted once it is committed")
	}
}

func TestHandoffStaleRejectionSendsNewerDocumentWithoutDialog(t *testing.T) {
	fb := &fakeBoundary{res: planner.Result{Outcome: planner.OutcomeRejected}}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "first\n")

	m, follow := rewriteDuringSend(t, m, path, "second\n")

	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone for a rejection of a replaced document", m.activeOverlay)
	}
	if follow == nil {
		t.Fatal("the newer document was not sent")
	}
	follow()
	if fb.calls() != 2 || fb.reqs[1].Reply != "second\n" {
		t.Fatalf("requests = %+v, want a second send of the newer document", fb.reqs)
	}
}

func TestHandoffErrorSendsNewerDocument(t *testing.T) {
	fb := &fakeBoundary{err: errors.New("gh failed")}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "first\n")

	_, follow := rewriteDuringSend(t, m, path, "second\n")

	if follow == nil {
		t.Fatal("the newer document was not sent")
	}
	follow()
	if fb.calls() != 2 || fb.reqs[1].Reply != "second\n" {
		t.Fatalf("requests = %+v, want a second send of the newer document", fb.reqs)
	}
}

func TestHandoffRemovalFailureTellsUserToDeleteFile(t *testing.T) {
	m, _ := handoffFixture(t, &fakeBoundary{})
	// A non-empty directory cannot be removed with os.Remove.
	stuck := filepath.Join(t.TempDir(), "stuck")
	if err := os.MkdirAll(filepath.Join(stuck, "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	next, _ := m.Update(planningHandoffDoneMsg{end: s1End(), path: stuck, result: committedResult(12)})
	m = next.(model)

	got := activityText(m)
	for _, want := range []string{"#12", "delete", stuck, "duplicate"} {
		if !strings.Contains(got, want) {
			t.Errorf("console = %q, want it to contain %q", got, want)
		}
	}
}

func TestHandoffNilBoundaryShowsErrorAndKeepsFile(t *testing.T) {
	m, path := handoffFixture(t, nil)
	writeDocument(t, path, "doc\n")

	m2, cmd := m.finishPlanning(s1End())

	if cmd != nil {
		t.Error("finishPlanning returned a command with no boundary")
	}
	if !fileExists(path) {
		t.Error("the file must be kept")
	}
	if got := activityText(m2); !strings.Contains(got, path) {
		t.Errorf("console = %q, want an error line naming the kept file", got)
	}
}

func TestHandoffReadErrorShowsErrorAndKeepsFile(t *testing.T) {
	fb := &fakeBoundary{}
	m, path := handoffFixture(t, fb)
	// A directory at the document path exists but cannot be read as a file.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}

	m2, cmd := m.finishPlanning(s1End())

	if cmd != nil || fb.calls() != 0 {
		t.Errorf("cmd = %v, calls = %d, want no command and no call", cmd, fb.calls())
	}
	if len(m2.activityLines) == 0 {
		t.Error("a read error must produce a console error line")
	}
	if m2.planningHandoffs["s1"] {
		t.Error("a read error must not leave the in-flight marker set")
	}
}

func TestHandoffInvalidSessionIDDoesNothing(t *testing.T) {
	fb := &fakeBoundary{}
	m, _ := handoffFixture(t, fb)

	for _, id := range []string{"", "../x", "a/b"} {
		m2, cmd := m.finishPlanning(planningEnd{SessionID: id})
		if cmd != nil || fb.calls() != 0 || len(m2.activityLines) != 0 {
			t.Errorf("id %q: cmd=%v calls=%d lines=%q, want nothing", id, cmd, fb.calls(), m2.activityLines)
		}
	}
}

// --- the real planner.Boundary, with a fake store -----------------------

type handoffStore struct {
	mu      sync.Mutex
	creates []string // titles
	bodies  []string
}

func (s *handoffStore) Revision(context.Context, int) (string, error) { return "r", nil }
func (s *handoffStore) Update(context.Context, int, string) error     { return nil }
func (s *handoffStore) Create(_ context.Context, title, body string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates = append(s.creates, title)
	s.bodies = append(s.bodies, body)
	return 321, nil
}

func (s *handoffStore) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.creates)
}

// punchOutRecorder fails the test if anything is written to GitHub issues.
type punchOutRecorder struct{ t *testing.T }

func (p punchOutRecorder) State(string, int, string) (failure.IssueState, error) {
	p.t.Error("the create path must not read issue state")
	return failure.IssueState{}, nil
}
func (p punchOutRecorder) PostComment(string, int, string) error {
	p.t.Error("the create path must not post a comment")
	return nil
}
func (p punchOutRecorder) AddLabel(string, int, string) error {
	p.t.Error("the create path must not add a label")
	return nil
}

func realBoundary(t *testing.T, store *handoffStore) *planner.Boundary {
	t.Helper()
	target := failure.Target{Repo: "o/r", FailLabel: "kairon-failed"}
	issues := punchOutRecorder{t}
	return &planner.Boundary{
		Committer: &provenance.Committer{
			Store:  store,
			Router: &planner.ConflictRouter{Issues: issues, Target: target},
		},
		Issues: issues,
		Target: target,
	}
}

func TestHandoffRealBoundaryRejectsEmptyAndNonYAMLFiles(t *testing.T) {
	cases := map[string]string{
		"empty":    "",
		"non-yaml": "I could not decide, sorry.\nplain prose, not a document\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			store := &handoffStore{}
			m, path := handoffFixture(t, realBoundary(t, store))
			writeDocument(t, path, contents)

			m = runHandoff(t, m)

			if m.activeOverlay != overlayPlanningRejection {
				t.Fatalf("activeOverlay = %v, want overlayPlanningRejection; console: %q", m.activeOverlay, activityText(m))
			}
			if got := m.planningRejection.failure.Guard; got != "extract" {
				t.Errorf("guard = %q, want extract", got)
			}
			if !strings.Contains(overlayText(m), "Guard: extract") {
				t.Errorf("dialog = %q, want the extract guard", overlayText(m))
			}
			if store.createCount() != 0 {
				t.Error("a rejected document must write nothing to GitHub")
			}
			if !fileExists(path) {
				t.Error("the file must be kept")
			}
		})
	}
}

func TestHandoffRealBoundaryPunchOutIsAnErrorAndKeepsFile(t *testing.T) {
	store := &handoffStore{}
	m, path := handoffFixture(t, realBoundary(t, store))
	writeDocument(t, path, plannertest.PunchOutReply("o/r", 1))

	m = runHandoff(t, m)

	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone for a process error", m.activeOverlay)
	}
	if !fileExists(path) {
		t.Error("the file must be kept")
	}
	if got := activityText(m); !strings.Contains(got, planner.ErrNoPunchOutTarget.Error()) {
		t.Errorf("console = %q, want %q", got, planner.ErrNoPunchOutTarget.Error())
	}
	if store.createCount() != 0 {
		t.Error("a punch_out must create no issue")
	}
}

func TestHandoffRealBoundaryCommitsCanonicalDocument(t *testing.T) {
	store := &handoffStore{}
	m, path := handoffFixture(t, realBoundary(t, store))
	writeDocument(t, path, plannertest.CanonicalReply("o/r", 1, plannertest.ValidBody("Dependencies: none")))

	m = runHandoff(t, m)

	if store.createCount() != 1 {
		t.Fatalf("creates = %d, want 1; console: %q", store.createCount(), activityText(m))
	}
	if store.creates[0] != "Add a JSON Lines exporter" {
		t.Errorf("title = %q, want the first heading of the body", store.creates[0])
	}
	if fileExists(path) {
		t.Error("the file must be deleted after a commit")
	}
	if got := activityText(m); !strings.Contains(got, "https://github.com/o/r/issues/321") {
		t.Errorf("console = %q, want the issue URL", got)
	}
}

// --- path and prompt builders -------------------------------------------

func TestPlanningDocumentPath(t *testing.T) {
	got, err := planningDocumentPath("abc-123")
	if err != nil {
		t.Fatalf("planningDocumentPath: %v", err)
	}
	if want := ".kairon/sessions/abc-123.document.yaml"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, ".kairon/sessions/") {
		t.Errorf("path %q must be under .kairon/sessions/", got)
	}

	for _, id := range []string{"", "a/b", `a\b`, "..", "a..b", "../x", "/abs"} {
		if p, err := planningDocumentPath(id); err == nil {
			t.Errorf("planningDocumentPath(%q) = %q, want an error", id, p)
		}
	}
}

func TestPlanningPromptNewSession(t *testing.T) {
	got := planningPrompt("s1", "Plan an exporter", false)

	if want := "Document path: .kairon/sessions/s1.document.yaml\n"; !strings.HasPrefix(got, want) {
		t.Errorf("prompt = %q, want it to begin with %q", got, want)
	}
	if !strings.HasSuffix(got, "\n\nPlan an exporter") {
		t.Errorf("prompt = %q, want the request after a blank line", got)
	}
	if strings.Contains(got, "read it before continuing") {
		t.Errorf("a new session must not carry the resumed sentence: %q", got)
	}
}

func TestPlanningPromptResumedSessionAddsSentence(t *testing.T) {
	got := planningPrompt("s1", "Continue", true)

	if !strings.HasPrefix(got, "Document path: .kairon/sessions/s1.document.yaml\n") {
		t.Errorf("prompt = %q, want the path line first", got)
	}
	if !strings.Contains(got, "may already hold the last document") || !strings.Contains(got, "read it before continuing") {
		t.Errorf("prompt = %q, want the resumed sentence", got)
	}
	if !strings.HasSuffix(got, "\n\nContinue") {
		t.Errorf("prompt = %q, want the request last", got)
	}
}

func TestPlanningPromptWithoutRequestIsJustThePath(t *testing.T) {
	got := planningPrompt("s1", "", false)
	if got != "Document path: .kairon/sessions/s1.document.yaml" {
		t.Errorf("prompt = %q, want only the path line", got)
	}
}

func TestPlanningPromptInvalidSessionAddsNothing(t *testing.T) {
	if got := planningPrompt("../x", "Plan it", false); got != "Plan it" {
		t.Errorf("prompt = %q, want the request unchanged", got)
	}
}

func TestPlanningResumePromptCarriesFailureAndPath(t *testing.T) {
	got := planningResumePrompt("s1", rejectionFailure())

	for _, want := range []string{
		"Document path: .kairon/sessions/s1.document.yaml",
		"rejected",
		"extract",
		"a stage-output document in YAML",
		"line one of actual\nline two of actual",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("resume prompt missing %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "Document path: ") {
		t.Errorf("resume prompt = %q, want the path line first", got)
	}
}

// A compile-time check that the real boundary satisfies the port.
var _ planningBoundary = (*planner.Boundary)(nil)
