package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/acp"
	"github.com/jbrinkman/kairon/internal/session"
)

// fakeACPClient is an acp.Client that records every message it is sent and
// answers each one with a canned stream (start, a text chunk, then "done").
type fakeACPClient struct {
	mu        sync.Mutex
	connected bool
	connects  int
	messages  []string
	reply     string
}

func (f *fakeACPClient) Connect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = true
	f.connects++
	return nil
}

func (f *fakeACPClient) Disconnect() error { return nil }

func (f *fakeACPClient) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *fakeACPClient) SendMessage(context.Context, *acp.MessageRequest) (*acp.MessageResponse, error) {
	return &acp.MessageResponse{}, nil
}

func (f *fakeACPClient) StreamMessage(_ context.Context, req *acp.MessageRequest) (<-chan *acp.StreamingResponse, error) {
	f.mu.Lock()
	f.messages = append(f.messages, req.Message)
	reply := f.reply
	f.mu.Unlock()
	ch := make(chan *acp.StreamingResponse, 3)
	ch <- &acp.StreamingResponse{Type: "start"}
	ch <- &acp.StreamingResponse{Type: "text", Content: reply}
	ch <- &acp.StreamingResponse{Type: "done"}
	return ch, nil
}

func (f *fakeACPClient) Close() error { return nil }

// disconnect simulates the connection dropping between messages.
func (f *fakeACPClient) disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = false
}

func (f *fakeACPClient) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

// acpFixture is a model with one planning tab backed by a fake ACP client and
// a fake planning boundary, running in a temp working directory.
type acpFixture struct {
	m      model
	tab    *PlanningTab
	client *fakeACPClient
	fb     *fakeBoundary
	doc    string // the tab's document path
}

func newACPFixture(t *testing.T) *acpFixture {
	t.Helper()
	t.Chdir(t.TempDir())
	m := validateTestModel(t, nil)
	fb := &fakeBoundary{res: committedResult(61)}
	m.planBoundary = fb
	sm := session.NewSessionManagerWithDir(".kairon/sessions")
	client := &fakeACPClient{reply: "Here is the plan."}
	tab := NewPlanningTabWithSession("planning-1", "Plan 1", m.styles, nil, sm, client)
	if tab.sessionID == "" {
		t.Fatal("planning tab has no session ID")
	}
	m.tabManager.AddTab(tab)
	doc, err := planningDocumentPath(tab.sessionID)
	if err != nil {
		t.Fatalf("planningDocumentPath: %v", err)
	}
	return &acpFixture{m: m, tab: tab, client: client, fb: fb, doc: doc}
}

// collectWithin runs cmd and every command inside a batch and returns the
// messages that arrive within d. Commands that wait (cursor blink ticks) are
// abandoned.
func collectWithin(cmd tea.Cmd, d time.Duration) []tea.Msg {
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
	deadline := time.After(d)
	for {
		select {
		case msg := <-out:
			msgs = append(msgs, msg)
		case <-deadline:
			return msgs
		}
	}
}

// pump feeds the planning messages cmd produces back through model.Update,
// and the messages that produces, until none are left.
func (f *acpFixture) pump(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	queue := collectWithin(cmd, 60*time.Millisecond)
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 60 {
			t.Fatal("message pump did not settle")
		}
		msg := queue[0]
		queue = queue[1:]
		switch msg.(type) {
		case planningStreamStartMsg, planningStreamMsg, planningResponseMsg,
			planningTurnEndedMsg, planningHandoffDoneMsg:
		default:
			continue
		}
		next, c := f.m.Update(msg)
		f.m = next.(model)
		queue = append(queue, collectWithin(c, 60*time.Millisecond)...)
	}
}

// send types a message into the tab, presses enter on the tab and pumps the
// whole turn through model.Update.
func (f *acpFixture) send(t *testing.T, text string) {
	t.Helper()
	f.tab.textinput.SetValue(text)
	_, cmd := f.tab.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("pressing enter in the planning tab returned no command")
	}
	f.pump(t, cmd)
}

func (f *acpFixture) writeDoc(t *testing.T, contents string) {
	t.Helper()
	writeDocument(t, f.doc, contents)
}

// TestRoutingStreamMessagesReachPlanningTab drives the real model.Update with
// stream events for a planning tab. Before the fix, model.Update had no case
// for them: they fell through to the footer input and the tab never saw them.
func TestRoutingStreamMessagesReachPlanningTab(t *testing.T) {
	f := newACPFixture(t)
	f.tab.streamingResponse = true

	next, _ := f.m.Update(planningStreamMsg{tabID: f.tab.ID(), response: &acp.StreamingResponse{Type: "text", Content: "hello"}})
	f.m = next.(model)

	if got := f.tab.currentResponse.String(); got != "hello" {
		t.Fatalf("planning tab received %q from model.Update, want %q: stream messages never reach the tab", got, "hello")
	}
}

func TestRoutingStreamMessageForClosedTabIsDropped(t *testing.T) {
	f := newACPFixture(t)
	f.tab.streamingResponse = true

	next, cmd := f.m.Update(planningStreamMsg{tabID: "planning-gone", response: &acp.StreamingResponse{Type: "text", Content: "stray"}})
	f.m = next.(model)

	if got := f.tab.currentResponse.String(); got != "" {
		t.Errorf("another tab received %q for a message addressed to a closed tab", got)
	}
	if f.m.input.Value() != "" {
		t.Errorf("footer input received the stray message: %q", f.m.input.Value())
	}
	for _, msg := range collectWithin(cmd, 30*time.Millisecond) {
		if _, ok := msg.(planningTurnEndedMsg); ok {
			t.Error("a message for a closed tab produced a turn end")
		}
	}
}

func TestRoutingStartAndResponseMessagesReachTab(t *testing.T) {
	f := newACPFixture(t)

	ch := make(chan *acp.StreamingResponse)
	cancelled := false
	next, _ := f.m.Update(planningStreamStartMsg{tabID: f.tab.ID(), streamChan: ch, streamCancel: func() { cancelled = true }})
	f.m = next.(model)
	if f.tab.streamChan == nil {
		t.Error("planningStreamStartMsg never reached the tab")
	}
	f.tab.cancelStream()
	if !cancelled {
		t.Error("the stream cancel func the tab received was not the one sent")
	}

	next, _ = f.m.Update(planningResponseMsg{tabID: f.tab.ID(), content: "plain reply"})
	f.m = next.(model)
	if last := f.tab.GetLastMessage(); last == nil || last.Content != "plain reply" {
		t.Errorf("planningResponseMsg never reached the tab; last message = %+v", last)
	}
}

// AC2: a stream "done" delivered through model.Update, with the document file
// present, makes exactly one call to the same handoff the classic site uses.
func TestACPTurnEndRunsHandoffWithDocument(t *testing.T) {
	f := newACPFixture(t)
	doc := "schema_version: 1\nstage: planner\nresult: canonical\n"
	f.writeDoc(t, doc)

	f.send(t, "plan a thing")

	if got := f.fb.calls(); got != 1 {
		t.Fatalf("Process calls = %d, want 1", got)
	}
	if got := f.fb.reqs[0].Reply; got != doc {
		t.Errorf("Reply = %q, want the file contents %q", got, doc)
	}
	if n := f.fb.reqs[0].Target.Number; n != 0 {
		t.Errorf("Target.Number = %d, want 0 (create path)", n)
	}
	if fileExists(f.doc) {
		t.Error("document file was not deleted after the commit")
	}
	if !strings.Contains(activityText(f.m), "#61") {
		t.Errorf("console has no success line with the issue number:\n%s", activityText(f.m))
	}
}

// AC3: with no document file, the turn end makes no call and opens no dialog.
func TestACPTurnEndWithoutDocumentDoesNothing(t *testing.T) {
	f := newACPFixture(t)

	f.send(t, "plan a thing")

	if got := f.fb.calls(); got != 0 {
		t.Errorf("Process calls = %d, want 0", got)
	}
	if f.m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want overlayNone", f.m.activeOverlay)
	}
	if last := f.tab.GetLastMessage(); last == nil || last.Content != "Here is the plan." {
		t.Errorf("tab did not receive the streamed reply; last message = %+v", last)
	}
}

func TestACPCompleteResponseEndsTheTurn(t *testing.T) {
	f := newACPFixture(t)

	_, cmd := f.tab.Update(planningResponseMsg{tabID: f.tab.ID(), isComplete: true})
	var ended *planningTurnEndedMsg
	for _, msg := range collectWithin(cmd, 30*time.Millisecond) {
		if e, ok := msg.(planningTurnEndedMsg); ok {
			ended = &e
		}
	}
	if ended == nil {
		t.Fatal("a non-error complete response did not produce a planningTurnEndedMsg")
	}
	if ended.tabID != f.tab.ID() || ended.sessionID != f.tab.sessionID {
		t.Errorf("turn end = %+v, want tab %q session %q", *ended, f.tab.ID(), f.tab.sessionID)
	}
}

func TestACPErrorsDoNotEndTheTurn(t *testing.T) {
	f := newACPFixture(t)
	f.tab.streamingResponse = true

	for name, msg := range map[string]tea.Msg{
		"error response": planningResponseMsg{tabID: f.tab.ID(), content: "boom", isError: true, isComplete: true},
		"stream error":   planningStreamMsg{tabID: f.tab.ID(), response: &acp.StreamingResponse{Type: "error", Error: "boom"}},
	} {
		_, cmd := f.tab.Update(msg)
		for _, got := range collectWithin(cmd, 30*time.Millisecond) {
			if _, ok := got.(planningTurnEndedMsg); ok {
				t.Errorf("%s produced a planningTurnEndedMsg", name)
			}
		}
	}
}

// AC6 for ACP: the first message over a connection carries the document path;
// the text shown in the tab stays the user's own message.
func TestACPFirstMessageAnnouncesDocumentPath(t *testing.T) {
	f := newACPFixture(t)

	f.send(t, "plan a thing")

	sent := f.client.sent()
	if len(sent) != 1 {
		t.Fatalf("client received %d messages, want 1", len(sent))
	}
	wantPrefix := "Document path: " + f.doc
	if !strings.HasPrefix(sent[0], wantPrefix) {
		t.Errorf("first message = %q, want prefix %q", sent[0], wantPrefix)
	}
	if !strings.HasPrefix(f.doc, ".kairon/sessions/") {
		t.Errorf("document path %q is not under .kairon/sessions/", f.doc)
	}
	if want := "Label answer path: " + strings.TrimSuffix(f.doc, ".document.yaml") + ".label"; !strings.Contains(sent[0], want) {
		t.Errorf("first message = %q, want the line %q", sent[0], want)
	}
	if !strings.HasSuffix(sent[0], "plan a thing") {
		t.Errorf("first message = %q, want the user's request at the end", sent[0])
	}
	if strings.Contains(sent[0], "resumed") {
		t.Errorf("a new tab's first message says it is resumed: %q", sent[0])
	}
	var shown string
	for _, msg := range f.tab.messages {
		if msg.Role == "user" {
			shown = msg.Content
		}
	}
	if shown != "plan a thing" {
		t.Errorf("tab shows the user message %q, want %q", shown, "plan a thing")
	}
}

func TestACPSecondMessageOnSameConnectionIsPlain(t *testing.T) {
	f := newACPFixture(t)

	f.send(t, "first")
	f.send(t, "second")

	sent := f.client.sent()
	if len(sent) != 2 {
		t.Fatalf("client received %d messages, want 2", len(sent))
	}
	if sent[1] != "second" {
		t.Errorf("second message = %q, want the plain text %q", sent[1], "second")
	}
}

func TestACPMessageAfterReconnectAnnouncesAgain(t *testing.T) {
	f := newACPFixture(t)

	f.send(t, "first")
	f.client.disconnect()
	f.send(t, "third")

	sent := f.client.sent()
	if len(sent) != 2 {
		t.Fatalf("client received %d messages, want 2", len(sent))
	}
	if !strings.HasPrefix(sent[1], "Document path: "+f.doc) {
		t.Errorf("message after reconnect = %q, want the document path again", sent[1])
	}
	if !strings.Contains(sent[1], "resumed") {
		t.Errorf("message after reconnect = %q, want the resumed sentence: the tab has earlier messages", sent[1])
	}
}

func TestACPRestoredTabAddsResumedSentence(t *testing.T) {
	f := newACPFixture(t)
	f.tab.AddMessage("user", "an earlier request")
	f.tab.AddMessage("assistant", "an earlier reply")

	f.send(t, "continue")

	sent := f.client.sent()
	if len(sent) != 1 {
		t.Fatalf("client received %d messages, want 1", len(sent))
	}
	if !strings.HasPrefix(sent[0], "Document path: "+f.doc) || !strings.Contains(sent[0], "resumed") {
		t.Errorf("message = %q, want the path line and the resumed sentence", sent[0])
	}
	if !strings.HasSuffix(sent[0], "continue") {
		t.Errorf("message = %q, want the user's request at the end", sent[0])
	}
}

func TestACPSendFailureAnnouncesPathAgain(t *testing.T) {
	f := newACPFixture(t)
	f.tab.pathAnnounced = true

	f.tab.Update(planningResponseMsg{tabID: f.tab.ID(), content: "Failed to send message: x", isError: true, isComplete: true})

	if f.tab.pathAnnounced {
		t.Error("a failed send left the path marked as announced; the agent never saw it")
	}
}

func TestACPStreamErrorAnnouncesPathAgain(t *testing.T) {
	f := newACPFixture(t)
	f.send(t, "first")

	f.tab.Update(planningStreamMsg{tabID: f.tab.ID(), response: &acp.StreamingResponse{Type: "error", Error: "failed to create session: x"}})
	f.send(t, "retry")

	sent := f.client.sent()
	if len(sent) != 2 {
		t.Fatalf("client received %d messages, want 2", len(sent))
	}
	if !strings.HasPrefix(sent[1], "Document path: "+f.doc) {
		t.Errorf("message after a stream error = %q, want it to announce the document path again", sent[1])
	}
}

func TestACPTabWithoutSessionAddsNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	m := validateTestModel(t, nil)
	client := &fakeACPClient{reply: "ok"}
	tab := NewPlanningTabWithSession("planning-9", "Plan 9", m.styles, nil, nil, client)
	f := &acpFixture{m: m, tab: tab, client: client, fb: &fakeBoundary{}}
	f.m.tabManager.AddTab(tab)
	f.m.planBoundary = f.fb

	f.send(t, "plain request")

	sent := client.sent()
	if len(sent) != 1 || sent[0] != "plain request" {
		t.Errorf("client received %q, want the plain request with no path", sent)
	}
	if f.fb.calls() != 0 {
		t.Errorf("Process calls = %d, want 0 for a tab with no session", f.fb.calls())
	}
}

// AC5 for ACP: Resume sends the path and the failure to the originating tab
// and makes it the active tab.
func TestACPResumeSendsFailureToOriginatingTab(t *testing.T) {
	f := newACPFixture(t)
	f.writeDoc(t, "schema_version: 1\n")
	f.m.tabManager.SetActiveTab(0)
	fc := rejectionFailure()
	f.m = f.m.openPlanningRejection(planningEnd{SessionID: f.tab.sessionID, Resume: acpResume(f.tab.ID())}, fc, f.doc)

	next, cmd := pressKey(t, f.m, tea.Key{Code: 'r', Text: "r"})
	f.m = next
	if !fileExists(f.doc) {
		t.Error("resuming deleted the document file")
	}
	f.pump(t, cmd)

	sent := f.client.sent()
	if len(sent) != 1 {
		t.Fatalf("client received %d messages, want 1", len(sent))
	}
	for _, want := range []string{f.doc, fc.Guard, fc.Expected, "line one of actual", "line two of actual"} {
		if !strings.Contains(sent[0], want) {
			t.Errorf("resume message lacks %q:\n%s", want, sent[0])
		}
	}
	if active := f.m.tabManager.GetActiveTab(); active == nil || active.ID() != f.tab.ID() {
		t.Errorf("active tab = %v, want the originating planning tab", active)
	}
	if f.m.activeOverlay != overlayNone {
		t.Errorf("activeOverlay = %v, want the dialog closed", f.m.activeOverlay)
	}
	var system bool
	for _, msg := range f.tab.messages {
		if msg.Role == "system" {
			system = true
		}
	}
	if !system {
		t.Error("resuming added no system message to the tab")
	}
}

func TestACPResumeWithClosedTabShowsErrorAndKeepsFile(t *testing.T) {
	f := newACPFixture(t)
	f.writeDoc(t, "schema_version: 1\n")
	end := planningEnd{SessionID: f.tab.sessionID, Resume: acpResume("planning-gone")}
	f.m = f.m.openPlanningRejection(end, rejectionFailure(), f.doc)

	next, cmd := pressKey(t, f.m, tea.Key{Code: 'r', Text: "r"})
	f.m = next

	if cmd != nil {
		t.Error("resume for a closed tab returned a command")
	}
	text := activityText(f.m)
	if !strings.Contains(text, "planning-gone") && !strings.Contains(strings.ToLower(text), "closed") {
		t.Errorf("console has no error line about the closed tab:\n%s", text)
	}
	if !fileExists(f.doc) {
		t.Error("document file was removed")
	}
	if len(f.client.sent()) != 0 {
		t.Error("a message was sent for a closed tab")
	}
}

func TestACPResumeWhileTabBusyShowsErrorAndKeepsFile(t *testing.T) {
	f := newACPFixture(t)
	f.writeDoc(t, "schema_version: 1\n")
	f.tab.state = session.PlanningStateActive
	end := planningEnd{SessionID: f.tab.sessionID, Resume: acpResume(f.tab.ID())}
	f.m = f.m.openPlanningRejection(end, rejectionFailure(), f.doc)

	next, cmd := pressKey(t, f.m, tea.Key{Code: 'r', Text: "r"})
	f.m = next

	if cmd != nil || len(f.client.sent()) != 0 {
		t.Error("resume started a second turn on a busy tab")
	}
	if !strings.Contains(activityText(f.m), "busy") {
		t.Errorf("console has no error line saying the tab is busy:\n%s", activityText(f.m))
	}
	if !fileExists(f.doc) {
		t.Error("document file was removed")
	}
}
