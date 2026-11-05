package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/boundary"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/github/githubtest"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
	"github.com/jbrinkman/kairon/internal/provenance"
)

const capWait = 10 * time.Second

// capHarness runs a production-constructed Manager over a githubtest.Fake with
// the worktree, process start, monitor and PR check replaced. Nothing here can
// reach gh, git or kiro-cli.
type capHarness struct {
	t         *testing.T
	m         *Manager
	gh        *githubtest.Fake
	wt        string
	prExists  atomic.Bool
	worktrees atomic.Int32
	starts    atomic.Int32
	monitored chan *Agent
	untracked chan int

	mu         sync.Mutex
	wtCalls    map[int]int // CreateWorktree calls per issue
	startCalls map[int]int // StartAgent calls per issue
	onWorktree func(issue, n int) (string, error)
	onStart    func(issue, n int) error
}

func newCapHarness(t *testing.T, maxAgents, maxRetries int) *capHarness {
	t.Helper()
	t.Chdir(t.TempDir()) // inputs, logs and retry files live under ./.kairon
	h := &capHarness{
		t:          t,
		gh:         githubtest.New(),
		wt:         t.TempDir(),
		monitored:  make(chan *Agent, 32),
		untracked:  make(chan int, 32),
		wtCalls:    map[int]int{},
		startCalls: map[int]int{},
	}
	h.prExists.Store(true)
	cfg := &config.Config{Repo: testRepo, Label: testLabel, MaxRetries: maxRetries, MaxAgents: maxAgents}
	h.m = NewManagerWithOptions(cfg, Options{
		GitHub:         h.gh,
		CreateWorktree: h.createWorktree,
		StartAgent:     h.startAgent,
		MonitorAgent:   func(a *Agent, _ *exec.Cmd) { h.monitored <- a },
		PRExists:       func(string, int) (bool, error) { return h.prExists.Load(), nil },
	})
	h.m.cleanup = func(int, int) {}
	h.m.removeWorktree = func(int, int) error { return nil }
	h.m.untrack = func(issue int) { h.untracked <- issue }
	return h
}

func (h *capHarness) createWorktree(name string) (string, error) {
	var issue int
	if _, err := fmt.Sscanf(name, "issue-%d-", &issue); err != nil {
		h.t.Errorf("unexpected worktree name %q", name)
	}
	h.worktrees.Add(1)
	h.mu.Lock()
	h.wtCalls[issue]++
	n, hook := h.wtCalls[issue], h.onWorktree
	h.mu.Unlock()
	if hook != nil {
		if path, err := hook(issue, n); err != nil || path != "" {
			return path, err
		}
	}
	return h.wt, nil
}

func (h *capHarness) startAgent(cmd *exec.Cmd) error {
	v, _ := envValue(cmd, "ISSUE_NUMBER")
	issue, err := strconv.Atoi(v)
	if err != nil {
		h.t.Errorf("ISSUE_NUMBER = %q", v)
	}
	h.starts.Add(1)
	h.mu.Lock()
	h.startCalls[issue]++
	n, hook := h.startCalls[issue], h.onStart
	h.mu.Unlock()
	if hook != nil {
		return hook(issue, n)
	}
	return nil
}

func (h *capHarness) setWorktreeHook(fn func(issue, n int) (string, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onWorktree = fn
}

func (h *capHarness) setStartHook(fn func(issue, n int) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onStart = fn
}

// put stores valid, stamped issues so the real boundary admits them.
func (h *capHarness) put(issues ...int) {
	for _, n := range issues {
		body := provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now())
		h.gh.Put(n, fmt.Sprintf("Issue %d", n), body, testLabel)
	}
}

func (h *capHarness) mustSpawn(issue int) *Agent {
	h.t.Helper()
	a, err := h.m.Spawn(issue, testRepo)
	if err != nil {
		h.t.Fatalf("Spawn(#%d): %v", issue, err)
	}
	return a
}

func (h *capHarness) wantAtCapacity(issue int) {
	h.t.Helper()
	if _, err := h.m.Spawn(issue, testRepo); !errors.Is(err, ErrAtCapacity) {
		h.t.Fatalf("Spawn(#%d) = %v, want ErrAtCapacity", issue, err)
	}
}

func (h *capHarness) wantSlots(used, max int) {
	h.t.Helper()
	if u, m := h.m.Slots(); u != used || m != max {
		h.t.Fatalf("Slots() = %d/%d, want %d/%d", u, m, used, max)
	}
}

func (h *capHarness) status(a *Agent) Status {
	h.m.mu.RLock()
	defer h.m.mu.RUnlock()
	return a.Status
}

func (h *capHarness) waitUntracked(issue int) {
	h.t.Helper()
	select {
	case got := <-h.untracked:
		if got != issue {
			h.t.Fatalf("untracked issue #%d, want #%d", got, issue)
		}
	case <-time.After(capWait):
		h.t.Fatalf("retry for issue #%d did not end", issue)
	}
}

func TestSpawn_NeverExceedsMaxAgentsConcurrently(t *testing.T) {
	h := newCapHarness(t, 2, 3)
	issues := []int{101, 102, 103, 104, 105, 106, 107, 108}
	h.put(issues...)

	begin := make(chan struct{})
	errs := make([]error, len(issues))
	var wg sync.WaitGroup
	for i, n := range issues {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-begin
			_, errs[i] = h.m.Spawn(n, testRepo)
		}()
	}
	close(begin)
	wg.Wait()

	var ok, atCap int
	for i, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrAtCapacity):
			atCap++
		default:
			t.Errorf("Spawn(#%d) = %v, want nil or ErrAtCapacity", issues[i], err)
		}
	}
	if ok != 2 || atCap != 6 {
		t.Fatalf("succeeded = %d, at capacity = %d, want 2 and 6", ok, atCap)
	}
	h.wantSlots(2, 2)
	if n := h.starts.Load(); n != 2 {
		t.Errorf("StartAgent ran %d times, want 2", n)
	}
	if n := h.worktrees.Load(); n != 2 {
		t.Errorf("CreateWorktree ran %d times, want 2", n)
	}
}

func TestSpawn_ExitFreesSlotForNextIssue(t *testing.T) {
	h := newCapHarness(t, 2, 3)
	h.put(1, 2, 3)
	a1 := h.mustSpawn(1)
	h.mustSpawn(2)
	h.wantAtCapacity(3)

	h.m.HandleExit(a1.ID, 0) // PRExists is true: the run completed

	h.wantSlots(1, 2)
	h.mustSpawn(3)
	h.wantSlots(2, 2)
}

func TestHandleExit_DoneLabelGoesThroughIssuesPort(t *testing.T) {
	h := newCapHarness(t, 2, 3)
	h.put(1)
	a1 := h.mustSpawn(1)

	h.m.HandleExit(a1.ID, 0)

	if !slices.Contains(h.gh.Get(1).Labels, testLabel+"-done") {
		t.Errorf("labels = %v, want %s-done added through the issues port", h.gh.Get(1).Labels, testLabel)
	}
}

func TestSpawn_AtCapacityDoesNoAdmissionWork(t *testing.T) {
	h := newCapHarness(t, 1, 3)
	h.put(1, 2)
	h.mustSpawn(1)
	calls, worktrees := len(h.gh.Calls()), h.worktrees.Load()

	h.wantAtCapacity(2)

	if got := h.gh.Calls(); len(got) != calls {
		t.Errorf("GitHub calls after a refusal = %v, want none beyond the first %d", got[calls:], calls)
	}
	if exists(inputDocumentPath(2)) {
		t.Error("input document written for a refused issue")
	}
	if n := h.worktrees.Load(); n != worktrees {
		t.Errorf("CreateWorktree ran %d more times for a refused issue", n-worktrees)
	}
}

func TestSpawn_FailurePathsReleaseSlot(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name    string
		arrange func(h *capHarness)
	}{
		{"worktree error", func(h *capHarness) {
			h.setWorktreeHook(func(int, int) (string, error) { return "", boom })
		}},
		{"log file error", func(h *capHarness) {
			// A directory where the log file belongs makes OpenFile fail.
			if err := os.MkdirAll(filepath.Join(".kairon", "logs", "issue-5.log"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"StartAgent error", func(h *capHarness) {
			h.setStartHook(func(int, int) error { return boom })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCapHarness(t, 1, 3)
			h.put(5, 6)
			tc.arrange(h)
			// CreateWorktree runs under m.mu, so usedLocked is safe to call
			// from it. Wrap whatever the case installed to observe the table.
			var usedAtWorktree atomic.Int32
			usedAtWorktree.Store(-1)
			h.mu.Lock()
			inner := h.onWorktree
			h.onWorktree = func(issue, n int) (string, error) {
				usedAtWorktree.Store(int32(h.m.usedLocked()))
				if inner != nil {
					return inner(issue, n)
				}
				return "", nil
			}
			h.mu.Unlock()

			if _, err := h.m.Spawn(5, testRepo); err == nil {
				t.Fatal("Spawn succeeded, want an error")
			}

			if got := usedAtWorktree.Load(); got != 1 {
				t.Errorf("slots held when the worktree was created = %d, want 1 (reserve before createWorktree)", got)
			}
			h.wantSlots(0, 1)

			// The slot is free again: another issue can take it.
			h.setWorktreeHook(nil)
			h.setStartHook(nil)
			h.mustSpawn(6)
			h.wantSlots(1, 1)
		})
	}
}

func TestSpawn_SecondRunForHeldIssueRefused(t *testing.T) {
	h := newCapHarness(t, 3, 3)
	h.put(1)
	h.mustSpawn(1)

	_, err := h.m.Spawn(1, testRepo)

	if !errors.Is(err, ErrNotAdmitted) {
		t.Fatalf("second Spawn = %v, want ErrNotAdmitted", err)
	}
	if n := h.starts.Load(); n != 1 {
		t.Errorf("StartAgent ran %d times, want 1", n)
	}
	h.wantSlots(1, 3)
	if !exists(inputDocumentPath(1)) {
		t.Error("the running agent's input document is gone")
	}
}

func TestRetryHoldsSlotAcrossFailedGap(t *testing.T) {
	h := newCapHarness(t, 1, 3)
	h.put(1, 2)
	a1 := h.mustSpawn(1)

	gate := make(chan struct{})
	h.m.retry = func(a *Agent) { <-gate; h.m.retryAgent(a) }
	startReached, startGate := make(chan struct{}), make(chan struct{})
	h.setStartHook(func(issue, n int) error {
		if issue == 1 && n == 2 { // the retry's process start
			close(startReached)
			<-startGate
		}
		return nil
	})

	h.m.HandleExit(a1.ID, 1)

	if got := h.status(a1); got != StatusFailed {
		t.Fatalf("status = %s, want %s", got, StatusFailed)
	}
	h.wantAtCapacity(2) // the failed gap: no StatusRunning agent, slot still held
	h.wantSlots(1, 1)

	close(gate)
	select {
	case <-startReached:
	case <-time.After(capWait):
		t.Fatal("retry did not reach the process start")
	}
	if got := h.status(a1); got != StatusRunning {
		t.Fatalf("status during retry = %s, want %s", got, StatusRunning)
	}
	h.wantAtCapacity(2) // the retry is running

	close(startGate)
	for range 2 { // the first run and the retried run were each monitored
		select {
		case <-h.monitored:
		case <-time.After(capWait):
			t.Fatal("retry did not start its monitor")
		}
	}
	h.wantAtCapacity(2)

	h.m.HandleExit(a1.ID, 0) // the retried run ends with a PR

	h.mustSpawn(2)
	h.wantSlots(1, 1)
}

func TestRetryTerminalFailureReleasesSlot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func(h *capHarness)
	}{
		{"re-admission refused", func(h *capHarness) { h.gh.Close(1) }},
		{"worktree creation fails", func(h *capHarness) {
			h.setWorktreeHook(func(issue, n int) (string, error) {
				if issue == 1 && n >= 2 { // the retry's worktree, not Spawn's
					return "", errors.New("no worktree")
				}
				return "", nil
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCapHarness(t, 1, 3)
			h.put(1, 2)
			a1 := h.mustSpawn(1)
			gate := make(chan struct{})
			h.m.retry = func(a *Agent) { <-gate; h.m.retryAgent(a) }

			h.m.HandleExit(a1.ID, 1)
			h.wantAtCapacity(2) // held while the retry is pending
			tc.arrange(h)
			close(gate)
			h.waitUntracked(1) // retryAgent gave up; failRetry ran before this

			h.wantSlots(0, 1)
			if got := h.status(a1); got != StatusFailed {
				t.Errorf("status = %s, want %s", got, StatusFailed)
			}
			h.mustSpawn(2)
		})
	}
}

func TestPunchOutReleasesSlot(t *testing.T) {
	for _, tc := range []struct {
		name string
		exit int
		pr   bool
	}{
		{"non-zero exit", 1, true},
		{"exit 0 without PR", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCapHarness(t, 1, 0) // no retry budget: the first failure punches out
			h.put(1, 2)
			h.prExists.Store(tc.pr)
			a1 := h.mustSpawn(1)
			h.wantAtCapacity(2)

			h.m.HandleExit(a1.ID, tc.exit)

			if !slices.Contains(h.gh.Get(1).Labels, testLabel+"-failed") {
				t.Fatalf("labels = %v, want the failed label (a punch-out)", h.gh.Get(1).Labels)
			}
			h.wantSlots(0, 1)
			h.mustSpawn(2)
		})
	}
}

// A duplicated exit callback must not free a slot that another run now holds.
func TestHandleExit_DuplicateExitDoesNotDoubleRelease(t *testing.T) {
	h := newCapHarness(t, 1, 0)
	h.put(1)
	a1 := h.mustSpawn(1)
	h.m.HandleExit(a1.ID, 1)
	h.wantSlots(0, 1)
	h.m.mu.Lock()
	if err := h.m.reserveSlotLocked(2); err != nil { // another run takes the slot
		h.m.mu.Unlock()
		t.Fatal(err)
	}
	h.m.mu.Unlock()

	h.m.HandleExit(a1.ID, 1)

	h.wantSlots(1, 1)
}

func TestNewManager_NonPositiveMaxAgentsUsesDefault(t *testing.T) {
	for _, cfg := range []*config.Config{{}, {MaxAgents: -3}} {
		_, max := NewManager(cfg).Slots()
		if max != config.DefaultMaxAgents {
			t.Errorf("MaxAgents=%d: max = %d, want %d", cfg.MaxAgents, max, config.DefaultMaxAgents)
		}
	}
}

func TestReserveSlotLocked(t *testing.T) {
	h := newCapHarness(t, 1, 3)
	m := h.m
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.reserveSlotLocked(1); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if err := m.reserveSlotLocked(1); !errors.Is(err, errSlotHeld) {
		t.Errorf("reserve of a held issue = %v, want errSlotHeld", err)
	}
	err := m.reserveSlotLocked(2)
	if !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("reserve on a full table = %v, want ErrAtCapacity", err)
	}
	if !strings.Contains(err.Error(), "(1/1 running)") {
		t.Errorf("error %q does not carry the used/max numbers", err)
	}
	m.releaseSlotLocked(3) // not held: no-op
	m.releaseSlotLocked(1)
	m.releaseSlotLocked(1) // already released: no-op
	if used := m.usedLocked(); used != 0 {
		t.Errorf("used = %d after release, want 0", used)
	}
	if err := m.reserveSlotLocked(2); err != nil {
		t.Errorf("reserve after release: %v", err)
	}
}

func TestSpawn_AdmissionErrAtCapacityPassesThrough(t *testing.T) {
	h := newCapHarness(t, 3, 3)
	h.m.admit = func(string, int) (boundary.Result, error) {
		return boundary.Result{}, fmt.Errorf("start intake: %w", ErrAtCapacity)
	}

	_, err := h.m.Spawn(1, testRepo)

	if !errors.Is(err, ErrAtCapacity) {
		t.Errorf("Spawn = %v, want an error satisfying ErrAtCapacity", err)
	}
	if errors.Is(err, ErrAdmissionUnavailable) {
		t.Errorf("Spawn = %v, ErrAtCapacity was hidden as ErrAdmissionUnavailable", err)
	}
}
