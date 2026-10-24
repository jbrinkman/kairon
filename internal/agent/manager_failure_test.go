package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/failure"
)

const (
	testRepo  = "owner/repo"
	testIssue = 42
	testLabel = "kairon"
)

// fakeIssues is a call-recording failure.Issues. It reflects posted comments
// and added labels back through State. When mgr is set it also records whether
// the manager lock was held during any call, so tests can prove GitHub writes
// happen outside m.mu.
type fakeIssues struct {
	mu       sync.Mutex
	calls    []string
	comments []failure.Comment
	labels   []string
	failOp   map[string]error
	mgr      *Manager
	lockHeld bool
}

func newFakeIssues() *fakeIssues { return &fakeIssues{failOp: map[string]error{}} }

func (f *fakeIssues) probe() {
	if f.mgr == nil {
		return
	}
	if f.mgr.mu.TryLock() {
		f.mgr.mu.Unlock()
		return
	}
	f.lockHeld = true
}

func (f *fakeIssues) State(repo string, issue int, failLabel string) (failure.IssueState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probe()
	f.calls = append(f.calls, "State")
	if err := f.failOp["State"]; err != nil {
		return failure.IssueState{}, err
	}
	return failure.IssueState{Labels: slices.Clone(f.labels), Comments: slices.Clone(f.comments)}, nil
}

func (f *fakeIssues) PostComment(repo string, issue int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probe()
	f.calls = append(f.calls, "PostComment")
	if err := f.failOp["PostComment"]; err != nil {
		return err
	}
	f.comments = append(f.comments, failure.Comment{Body: body, CreatedAt: time.Now(), ByViewer: true})
	return nil
}

func (f *fakeIssues) AddLabel(repo string, issue int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probe()
	f.calls = append(f.calls, "AddLabel "+label)
	if err := f.failOp["AddLabel"]; err != nil {
		return err
	}
	f.labels = append(f.labels, label)
	return nil
}

// writes returns the recorded calls that change GitHub, in order.
func (f *fakeIssues) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var w []string
	for _, c := range f.calls {
		if c != "State" && c != "RemoveWorktree" {
			w = append(w, c)
		}
	}
	return w
}

// orderedCalls returns every recorded call (GitHub writes and the worktree
// removal), in order, so a test can assert the worktree is removed before the
// fail label is applied.
func (f *fakeIssues) orderedCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var w []string
	for _, c := range f.calls {
		if c != "State" {
			w = append(w, c)
		}
	}
	return w
}

// harness wires a Manager to fakes. No test using it can reach gh, kiro-cli or git.
type harness struct {
	m         *Manager
	issues    *fakeIssues
	agent     *Agent
	retried   chan *Agent
	cleaned   chan int
	removed   chan int
	untracked chan int
}

func newHarness(t *testing.T, maxRetries, retryCount int, prExists bool) *harness {
	t.Helper()
	t.Chdir(t.TempDir()) // retry count files live under ./.kairon/retries

	cfg := &config.Config{Repo: testRepo, Label: testLabel, MaxRetries: maxRetries}

	h := &harness{
		m:         NewManager(cfg),
		issues:    newFakeIssues(),
		retried:   make(chan *Agent, 32),
		cleaned:   make(chan int, 32),
		removed:   make(chan int, 32),
		untracked: make(chan int, 32),
	}
	h.issues.mgr = h.m
	h.m.issues = h.issues
	h.m.prExists = func(repo string, issue int) (bool, error) { return prExists, nil }
	h.m.retry = func(a *Agent) { h.retried <- a }
	h.m.cleanup = func(issue, pid int) { h.cleaned <- issue }
	h.m.untrack = func(issue int) { h.untracked <- issue }
	h.m.removeWorktree = func(issue, pid int) error {
		// Record ordering against the GitHub writes so a test can prove the
		// worktree is removed before the fail label is applied.
		h.issues.mu.Lock()
		h.issues.calls = append(h.issues.calls, "RemoveWorktree")
		h.issues.mu.Unlock()
		h.removed <- issue
		return nil
	}

	h.agent = &Agent{
		ID:          "agent-42-1",
		IssueNumber: testIssue,
		Status:      StatusRunning,
		RetryCount:  retryCount,
		RunID:       failure.NewRunID(),
	}
	h.m.agents[h.agent.ID] = h.agent
	return h
}

// retryCalls waits briefly for the (asynchronous) retry seam and returns how
// many times it ran.
func (h *harness) retryCalls() int {
	time.Sleep(50 * time.Millisecond)
	return len(h.retried)
}

func (h *harness) writeRetryFile(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(".kairon", "retries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "issue-42.count")
	if err := os.WriteFile(p, []byte("3"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// assertCommentThenLabel checks the fake saw exactly one comment followed by
// the failed label, and that the comment names the stage and the attempts.
func assertCommentThenLabel(t *testing.T, h *harness, wantAttempts string) {
	t.Helper()
	want := []string{"PostComment", "AddLabel kairon-failed"}
	if got := h.issues.writes(); !slices.Equal(got, want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
	body := h.issues.comments[0].Body
	for _, s := range []string{"krew-lead", wantAttempts, "issue #42", ".kairon/logs/issue-42.log", "kairon-failed"} {
		if !strings.Contains(body, s) {
			t.Errorf("comment missing %q:\n%s", s, body)
		}
	}
	if id, ok := failure.ParseMarker(body); !ok || id != h.agent.RunID {
		t.Errorf("comment marker run id = %q (ok=%v), want %q", id, ok, h.agent.RunID)
	}
	if h.issues.lockHeld {
		t.Error("m.mu was held during a GitHub call")
	}
}

func TestHandleExit_ExhaustedRetriesPostsCommentThenLabel(t *testing.T) {
	h := newHarness(t, 3, 3, true)

	h.m.HandleExit(h.agent.ID, 1)

	assertCommentThenLabel(t, h, "Attempts made: 4 of 4")
	if n := h.retryCalls(); n != 0 {
		t.Errorf("retry called %d times, want 0", n)
	}
	if h.agent.Status != StatusFailed {
		t.Errorf("status = %s, want %s", h.agent.Status, StatusFailed)
	}
}

func TestHandleExit_NoPRExhaustedPostsCommentThenLabel(t *testing.T) {
	h := newHarness(t, 2, 2, false)

	h.m.HandleExit(h.agent.ID, 0)

	assertCommentThenLabel(t, h, "Attempts made: 3 of 3")
	if !strings.Contains(h.issues.comments[0].Body, "no pull request") {
		t.Errorf("comment does not say no PR was found:\n%s", h.issues.comments[0].Body)
	}
	if n := h.retryCalls(); n != 0 {
		t.Errorf("retry called %d times, want 0", n)
	}
}

func TestHandleExit_RetriesRemainNoWrites(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exit     int
		prExists bool
	}{
		{"non-zero exit", 1, true},
		{"exit 0 without PR", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, 3, 1, tc.prExists)

			h.m.HandleExit(h.agent.ID, tc.exit)

			if n := h.retryCalls(); n != 1 {
				t.Fatalf("retry called %d times, want 1", n)
			}
			if got := <-h.retried; got != h.agent {
				t.Error("retry seam received a different agent")
			}
			if w := h.issues.writes(); len(w) != 0 {
				t.Errorf("writes = %v, want none while retries remain", w)
			}
			if h.agent.RetryCount != 2 {
				t.Errorf("RetryCount = %d, want 2", h.agent.RetryCount)
			}
			// Cleanup must NOT run while a retry is scheduled: it would delete
			// the retry worktree and reset the persisted retry budget.
			if n := len(h.cleaned); n != 0 {
				t.Errorf("cleanup seam called %d times on a retry, want 0", n)
			}
		})
	}
}

func TestHandleExit_RetryBudgetFollowsConfig(t *testing.T) {
	for _, maxRetries := range []int{0, 1, 2, 5} {
		h := newHarness(t, maxRetries, 0, true)

		for i := 0; i <= maxRetries; i++ {
			h.m.HandleExit(h.agent.ID, 1)
		}

		if n := h.retryCalls(); n != maxRetries {
			t.Errorf("MaxRetries=%d: retry called %d times, want %d", maxRetries, n, maxRetries)
		}
		want := []string{"PostComment", "AddLabel kairon-failed"}
		if got := h.issues.writes(); !slices.Equal(got, want) {
			t.Errorf("MaxRetries=%d: writes = %v, want %v", maxRetries, got, want)
		}
		wantAttempts := "Attempts made: " + strconv.Itoa(maxRetries+1) + " of " + strconv.Itoa(maxRetries+1)
		if len(h.issues.comments) == 1 && !strings.Contains(h.issues.comments[0].Body, wantAttempts) {
			t.Errorf("MaxRetries=%d: comment missing %q", maxRetries, wantAttempts)
		}
	}
}

func TestHandleExit_PunchOutSuccessRemovesRetryFile(t *testing.T) {
	h := newHarness(t, 1, 1, true)
	p := h.writeRetryFile(t)

	h.m.HandleExit(h.agent.ID, 1)

	if exists(p) {
		t.Error("retry count file still exists after a completed punch-out")
	}
}

func TestHandleExit_PunchOutErrorKeepsRetryFile(t *testing.T) {
	for _, op := range []string{"State", "PostComment", "AddLabel"} {
		t.Run(op, func(t *testing.T) {
			h := newHarness(t, 1, 1, true)
			h.issues.failOp[op] = errors.New("gh unavailable")
			p := h.writeRetryFile(t)

			h.m.HandleExit(h.agent.ID, 1)

			if !exists(p) {
				t.Error("retry count file was removed although the punch-out failed")
			}
			if op == "PostComment" {
				if w := h.issues.writes(); !slices.Equal(w, []string{"PostComment"}) {
					t.Errorf("writes = %v, want only the failed comment (no label)", w)
				}
			}
		})
	}
}

func TestHandleExit_NoPRPunchOutErrorKeepsRetryFile(t *testing.T) {
	h := newHarness(t, 1, 1, false)
	h.issues.failOp["AddLabel"] = errors.New("gh unavailable")
	p := h.writeRetryFile(t)

	h.m.HandleExit(h.agent.ID, 0)

	if !exists(p) {
		t.Error("retry count file was removed although the punch-out failed")
	}
}

// removedCount waits briefly for the (synchronous) worktree-removal seam and
// returns how many times it ran.
func (h *harness) removedCount() int {
	time.Sleep(50 * time.Millisecond)
	return len(h.removed)
}

func TestPunchOut_RemovesWorktreeBeforeApplyingFailLabel(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exit     int
		prExists bool
	}{
		{"non-zero exit", 1, true},
		{"exit 0 without PR", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, 1, 1, tc.prExists)

			h.m.HandleExit(h.agent.ID, tc.exit)

			if n := h.removedCount(); n != 1 {
				t.Fatalf("worktree removal called %d times, want 1", n)
			}
			if issue := <-h.removed; issue != testIssue {
				t.Errorf("removed worktree for issue %d, want %d", issue, testIssue)
			}
			// The worktree must be gone before the fail label is applied, so a
			// human who clears the label gets a clean restart (hasActiveWorktree
			// no longer sees a leftover directory named with the live watcher PID).
			want := []string{"RemoveWorktree", "PostComment", "AddLabel kairon-failed"}
			if got := h.issues.orderedCalls(); !slices.Equal(got, want) {
				t.Fatalf("call order = %v, want %v", got, want)
			}
		})
	}
}

func TestPunchOut_RetriesRemainDoesNotRemoveWorktree(t *testing.T) {
	h := newHarness(t, 3, 1, true) // retries remain: not a punch-out

	h.m.HandleExit(h.agent.ID, 1)

	if n := h.removedCount(); n != 0 {
		t.Errorf("worktree removal called %d times on a retry, want 0", n)
	}
}

// TestHandleExit_ZeroExitWithoutPRDoesNotRunGenericCleanup pins that the
// generic cleanup seam does not run on the exit-0/no-PR path. That path is a
// failure: it either retries (cleanup would delete the retry worktree) or
// punches out (punchOut removes its own worktree first). Either way the
// unconditional success cleanup must not fire here.
func TestHandleExit_ZeroExitWithoutPRDoesNotRunGenericCleanup(t *testing.T) {
	h := newHarness(t, 1, 1, false) // retries exhausted: terminal punch-out

	h.m.HandleExit(h.agent.ID, 0)

	// Punch-out removes the worktree through removeWorktree, not the generic
	// cleanup seam.
	if n := h.removedCount(); n != 1 {
		t.Errorf("worktree removal called %d times on terminal no-PR, want 1", n)
	}
	if n := len(h.cleaned); n != 0 {
		t.Errorf("generic cleanup seam called %d times on no-PR path, want 0", n)
	}
}

// TestHandleExit_ZeroExitWithPRRunsCleanup pins that a successful exit-0 run
// with a PR runs the generic cleanup seam (worktree + retry file).
func TestHandleExit_ZeroExitWithPRRunsCleanup(t *testing.T) {
	h := newHarness(t, 1, 0, true) // PR exists: success path

	h.m.HandleExit(h.agent.ID, 0)

	select {
	case issue := <-h.cleaned:
		if issue != testIssue {
			t.Errorf("cleanup issue = %d, want %d", issue, testIssue)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup seam was not called on the success path")
	}
}

// TestPerformCleanup_RemovesWorktreeAndRetryFileUnconditionally pins that the
// success-path cleanup no longer guards on a PR existing: a worktree is cleaned
// up because the work is done, not because a PR was found. It drives the real
// performCleanup with a fake removeWorktree so no gh call is reachable.
func TestPerformCleanup_RemovesWorktreeAndRetryFileUnconditionally(t *testing.T) {
	h := newHarness(t, 1, 0, false) // prExists=false: cleanup must still run

	p := h.writeRetryFile(t)

	h.m.performCleanup(testIssue, 1234)

	if n := h.removedCount(); n != 1 {
		t.Fatalf("worktree removal called %d times, want 1 (cleanup must not depend on a PR)", n)
	}
	if issue := <-h.removed; issue != testIssue {
		t.Errorf("removed worktree for issue %d, want %d", issue, testIssue)
	}
	if exists(p) {
		t.Error("retry count file still exists after performCleanup")
	}
}
