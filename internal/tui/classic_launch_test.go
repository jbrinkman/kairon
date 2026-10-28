package tui

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/session"
)

// classicLaunch records one call to the execPlanning seam.
type classicLaunch struct {
	cmd       *exec.Cmd
	sessionID string
}

// classicFixture runs in a temp working directory, points the session manager
// at its .kairon/sessions directory and replaces the execPlanning seam with a
// recorder, so no process starts.
func classicFixture(t *testing.T) (model, *[]classicLaunch) {
	t.Helper()
	t.Chdir(t.TempDir())
	m := validateTestModel(t, nil)
	m.sessionManager = session.NewSessionManagerWithDir(".kairon/sessions")
	var launches []classicLaunch
	m.execPlanning = func(cmd *exec.Cmd, sessionID string) tea.Cmd {
		launches = append(launches, classicLaunch{cmd: cmd, sessionID: sessionID})
		return nil
	}
	return m, &launches
}

// planningArgs returns the arguments handed to kiro-cli: everything after the
// "sh -c <script> sh" wrapper.
func planningArgs(t *testing.T, cmd *exec.Cmd) []string {
	t.Helper()
	if len(cmd.Args) < 4 {
		t.Fatalf("command args = %q, want sh -c <script> sh ...", cmd.Args)
	}
	return cmd.Args[4:]
}

// collectMsgs runs cmd and every command inside a batch, and returns the
// messages that arrive within the timeout. Commands that wait (cursor blink
// ticks) are abandoned.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	out := make(chan tea.Msg, 32)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			msg := c()
			if b, ok := msg.(tea.BatchMsg); ok {
				for _, sub := range b {
					run(sub)
				}
				return
			}
			out <- msg
		}()
	}
	run(cmd)
	var msgs []tea.Msg
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case msg := <-out:
			msgs = append(msgs, msg)
		case <-deadline:
			return msgs
		}
	}
}

// applyExecDone applies an execDoneMsg, runs the commands it returns and
// applies any handoff message they produce.
func applyExecDone(t *testing.T, m model, msg execDoneMsg) model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(model)
	for _, got := range collectMsgs(cmd) {
		if done, ok := got.(planningHandoffDoneMsg); ok {
			next, _ := m.Update(done)
			m = next.(model)
		}
	}
	return m
}

func TestExecDoneRunsHandoffWithDocument(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(55)}
	m, path := handoffFixture(t, fb)
	doc := "schema_version: 1\nstage: planner\n"
	writeDocument(t, path, doc)

	m = applyExecDone(t, m, execDoneMsg{sessionID: "s1"})

	if fb.calls() != 1 {
		t.Fatalf("boundary calls = %d, want 1", fb.calls())
	}
	if fb.reqs[0].Reply != doc {
		t.Errorf("Reply = %q, want the file contents %q", fb.reqs[0].Reply, doc)
	}
	if fb.reqs[0].Target.Number != 0 {
		t.Errorf("Target.Number = %d, want 0", fb.reqs[0].Target.Number)
	}
	if fileExists(path) {
		t.Error("the document file still exists after a commit")
	}
	if !strings.Contains(activityText(m), "#55") {
		t.Errorf("console = %q, want the new issue number", activityText(m))
	}
}

func TestExecDoneHandoffRunsWhenProcessExitedWithError(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(56)}
	m, path := handoffFixture(t, fb)
	writeDocument(t, path, "schema_version: 1\n")

	m = applyExecDone(t, m, execDoneMsg{sessionID: "s1", err: os.ErrClosed})

	if fb.calls() != 1 {
		t.Fatalf("boundary calls = %d, want 1 (the user may quit after approving)", fb.calls())
	}
	if fileExists(path) {
		t.Error("the document file still exists after a commit")
	}
}

func TestExecDoneWithoutDocumentDoesNothing(t *testing.T) {
	fb := &fakeBoundary{res: committedResult(57)}
	m, _ := handoffFixture(t, fb)

	m = applyExecDone(t, m, execDoneMsg{sessionID: "s1"})

	if fb.calls() != 0 {
		t.Errorf("boundary calls = %d, want 0", fb.calls())
	}
	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone", m.activeOverlay)
	}
}

func TestClassicPlanningCmdArgvCarriesPrompt(t *testing.T) {
	prompt := planningPrompt("abc123", "plan a thing", false)
	cmd := classicPlanningCmd("abc123", prompt)

	args := planningArgs(t, cmd)
	if len(args) == 0 || args[len(args)-1] != prompt {
		t.Fatalf("args = %q, want the prompt as the last argument", args)
	}
	if !strings.Contains(prompt, "Document path: .kairon/sessions/abc123.document.yaml") {
		t.Errorf("prompt = %q, want the document path line", prompt)
	}
}

func TestClassicNewSessionPromptHasDocumentPath(t *testing.T) {
	m, launches := classicFixture(t)

	m, _ = m.handlePlanClassic("plan a thing")

	if len(*launches) != 1 {
		t.Fatalf("launches = %d, want 1", len(*launches))
	}
	l := (*launches)[0]
	want := "Document path: .kairon/sessions/" + l.sessionID + ".document.yaml"
	joined := strings.Join(planningArgs(t, l.cmd), "\n")
	if !strings.Contains(joined, want) {
		t.Errorf("args = %q, want %q", joined, want)
	}
	if !strings.Contains(joined, "plan a thing") {
		t.Errorf("args = %q, want the user's request", joined)
	}
	if strings.Contains(joined, "being resumed") {
		t.Errorf("args = %q, a new session must not carry the resumed sentence", joined)
	}
	ids, err := m.sessionManager.List()
	if err != nil || len(ids) != 1 || ids[0] != l.sessionID {
		t.Errorf("sessions = %v (err %v), want exactly the launched session %s", ids, err, l.sessionID)
	}
}

func TestClassicReusesClassicSessionAndMarksResumed(t *testing.T) {
	m, launches := classicFixture(t)
	id, err := m.sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatal(err)
	}

	m, _ = m.handlePlanClassic("")

	if len(*launches) != 1 || (*launches)[0].sessionID != id {
		t.Fatalf("launches = %+v, want one launch of session %s", *launches, id)
	}
	joined := strings.Join(planningArgs(t, (*launches)[0].cmd), "\n")
	if !strings.Contains(joined, "Document path: .kairon/sessions/"+id+".document.yaml") {
		t.Errorf("args = %q, want the document path line", joined)
	}
	if !strings.Contains(joined, "being resumed") {
		t.Errorf("args = %q, want the resumed sentence", joined)
	}
}

func TestClassicDoesNotReuseACPTabSession(t *testing.T) {
	m, launches := classicFixture(t)
	acpID, _, err := m.sessionManager.CreatePlanningSession("tab-1", "ACP tab")
	if err != nil {
		t.Fatal(err)
	}

	m, _ = m.handlePlanClassic("")

	if len(*launches) != 1 {
		t.Fatalf("launches = %d, want 1", len(*launches))
	}
	if got := (*launches)[0].sessionID; got == acpID || got == "" {
		t.Errorf("launched session %q, want a new classic session distinct from the ACP tab session %q", got, acpID)
	}
	joined := strings.Join(planningArgs(t, (*launches)[0].cmd), "\n")
	if strings.Contains(joined, acpID) {
		t.Errorf("args = %q, must not name the ACP tab session", joined)
	}
}

func TestClassicCreateFailureShowsErrorAndDoesNotLaunch(t *testing.T) {
	m, launches := classicFixture(t)
	// A read-only sessions directory lets List succeed and makes Create fail.
	if err := os.MkdirAll(".kairon/sessions", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(".kairon/sessions", 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(".kairon/sessions", 0o755) })
	if f, err := os.Create(".kairon/sessions/probe"); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced (running as root?)")
	}

	m, cmd := m.handlePlanClassic("plan")

	if len(*launches) != 0 {
		t.Fatalf("launches = %d, want 0", len(*launches))
	}
	if cmd != nil {
		t.Error("handlePlanClassic returned a command after a Create failure")
	}
	if !strings.Contains(activityText(m), "Failed to create planning session") {
		t.Errorf("console = %q, want an error line for the failed Create", activityText(m))
	}
	if m.currentMode != session.Console {
		t.Errorf("currentMode = %v, want console (nothing launched)", m.currentMode)
	}
}

func TestClassicResumeFromDialogRelaunchesSameSession(t *testing.T) {
	m, launches := classicFixture(t)
	path, err := planningDocumentPath("sess9")
	if err != nil {
		t.Fatal(err)
	}
	writeDocument(t, path, "doc: 1\n")
	m = m.openPlanningRejection(planningEnd{SessionID: "sess9", Resume: classicResume("sess9")}, rejectionFailure(), path)

	m, _ = pressKey(t, m, tea.Key{Code: 'r', Text: "r"})

	if len(*launches) != 1 {
		t.Fatalf("launches = %d, want 1", len(*launches))
	}
	l := (*launches)[0]
	if l.sessionID != "sess9" {
		t.Errorf("sessionID = %q, want sess9", l.sessionID)
	}
	joined := strings.Join(planningArgs(t, l.cmd), "\n")
	for _, want := range []string{
		"Document path: .kairon/sessions/sess9.document.yaml",
		"extract",
		"a stage-output document in YAML",
		"line one of actual",
		"line two of actual",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("resume args missing %q:\n%s", want, joined)
		}
	}
	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone after Resume", m.activeOverlay)
	}
	if !fileExists(path) {
		t.Error("Resume removed the document file")
	}
}

func TestClassicDismissFromDialogKeepsFile(t *testing.T) {
	m, launches := classicFixture(t)
	path, _ := planningDocumentPath("sess9")
	writeDocument(t, path, "doc: 1\n")
	m = m.openPlanningRejection(planningEnd{SessionID: "sess9", Resume: classicResume("sess9")}, rejectionFailure(), path)

	m, _ = pressKey(t, m, tea.Key{Code: 'd', Text: "d"})

	if m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone", m.activeOverlay)
	}
	if len(*launches) != 0 {
		t.Errorf("launches = %d, want 0", len(*launches))
	}
	if !fileExists(path) {
		t.Error("Dismiss removed the document file")
	}
}

func TestClassicExecPlanningSeamDefaultIsSet(t *testing.T) {
	t.Chdir(t.TempDir())
	m := validateTestModel(t, nil)
	if m.execPlanning == nil {
		t.Fatal("newModel left the execPlanning seam nil; the default must wrap tea.ExecProcess")
	}
}
