package watcher

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/github"
	"github.com/jbrinkman/kairon/internal/github/githubtest"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// newCapPipeline is newPipelineHere with the agent limit set, a PR check that
// always finds a PR (so HandleExit(id, 0) completes a run without gh) and a
// retry budget of 2. It is a sibling of newPipelineHere so the existing
// fixtures keep their behavior. The manager is the production constructor;
// only the worktree, the process start and the process monitor are stubbed.
func newCapPipeline(t *testing.T, maxAgents int) *pipeline {
	t.Helper()
	t.Chdir(t.TempDir())
	gh := githubtest.New()
	p := &pipeline{t: t, gh: gh, dir: t.TempDir()}
	p.replyWith(plannertest.ValidBody("Dependencies: none"))
	cfg := &config.Config{Repo: e2eRepo, Label: e2eLabel, MaxRetries: 2, MaxAgents: maxAgents, PollInterval: time.Minute}
	worktree := t.TempDir()
	p.m = agent.NewManagerWithOptions(cfg, agent.Options{
		GitHub: gh,
		IntakeCommand: func(prompt string) *exec.Cmd {
			p.mu.Lock()
			p.prompts = append(p.prompts, prompt)
			next := p.next
			p.mu.Unlock()
			return next(prompt)
		},
		RetryDelay:     func(int) time.Duration { return 0 },
		CreateWorktree: func(string) (string, error) { return worktree, nil },
		StartAgent: func(cmd *exec.Cmd) error {
			p.mu.Lock()
			p.krewLead = append(p.krewLead, cmd)
			p.mu.Unlock()
			return nil
		},
		MonitorAgent: func(*agent.Agent, *exec.Cmd) {},
		PRExists:     func(string, int) (bool, error) { return true, nil },
	})
	p.w = New(cfg, p.m, "")
	p.w.selectIssues = func(_, label, _ string) (github.Selection, error) { return gh.Select(label), nil }
	p.w.reconcile = func(repo string, issue int, failLabel string) error {
		_, err := failure.Reconcile(gh, failure.Target{Repo: repo, Issue: issue, FailLabel: failLabel})
		return err
	}
	p.w.dependencyValidator.getIssueDetails = gh.Details
	return p
}

// putStamped stores open, stamped, dependency-free issues the real boundary
// admits for implementation.
func (p *pipeline) putStamped(issues ...int) {
	p.t.Helper()
	for _, n := range issues {
		body := provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now())
		p.gh.Put(n, fmt.Sprintf("Issue %d", n), body, e2eLabel)
	}
}

// starts is the issue number of every krew-lead process start, in order.
func (p *pipeline) starts() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	var got []int
	for _, cmd := range p.krewLead {
		// The process environment may already carry ISSUE_NUMBER; the
		// manager's value is appended last and wins.
		for i := len(cmd.Env) - 1; i >= 0; i-- {
			if v, ok := strings.CutPrefix(cmd.Env[i], "ISSUE_NUMBER="); ok {
				n, err := strconv.Atoi(v)
				if err != nil {
					p.t.Fatalf("ISSUE_NUMBER = %q", v)
				}
				got = append(got, n)
				break
			}
		}
	}
	return got
}

// implAgentID is the id of the issue's krew-lead agent.
func (p *pipeline) implAgentID(issue int) string {
	p.t.Helper()
	for _, a := range p.m.List() {
		if a.IssueNumber == issue && a.Kind == agent.KindImplementation {
			return a.ID
		}
	}
	p.t.Fatalf("no krew-lead agent for issue #%d", issue)
	return ""
}

func (p *pipeline) wantSlots(used, max int) {
	p.t.Helper()
	if u, m := p.m.Slots(); u != used || m != max {
		p.t.Fatalf("Slots() = %d/%d, want %d/%d", u, m, used, max)
	}
}

func (p *pipeline) wantStarts(want ...int) {
	p.t.Helper()
	if got := p.starts(); !slices.Equal(got, want) {
		p.t.Fatalf("starts = %v, want %v", got, want)
	}
}

func (p *pipeline) wantTracked(want ...int) {
	p.t.Helper()
	if got := trackedKeys(p.w); !slices.Equal(got, want) {
		p.t.Fatalf("tracked = %v, want %v", got, want)
	}
}

// syncBuffer is a bytes.Buffer safe for the manager's goroutines to log into.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Split(strings.TrimRight(s.b.String(), "\n"), "\n")
}

func (s *syncBuffer) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}

// captureLog sends the standard logger to a buffer until the test ends.
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

// waitStarts waits until issue has been started n times in total.
func (p *pipeline) waitStarts(issue, n int) {
	p.t.Helper()
	deadline := time.Now().Add(pollWithin)
	for {
		count := 0
		for _, s := range p.starts() {
			if s == issue {
				count++
			}
		}
		if count >= n {
			return
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("issue #%d was started %d times, want %d", issue, count, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// AC1: with two slots and five eligible issues, never more than two run, and
// the third starts only after one of the first two exits.
func TestCheckIssues_TwoSlotsFiveIssuesNeverMoreThanTwoRunning(t *testing.T) {
	p := newCapPipeline(t, 2)
	p.putStamped(10, 11, 12, 13, 14)

	p.w.checkIssues()

	p.wantStarts(10, 11)
	p.wantSlots(2, 2)
	// The three waiting issues are untracked and have spent no retry attempt.
	p.wantTracked(10, 11)
	for _, n := range []int{12, 13, 14} {
		if fileExists(fmt.Sprintf(".kairon/retries/issue-%d.count", n)) {
			t.Errorf("issue #%d waited but has a retry count file", n)
		}
	}

	for range 3 {
		p.w.checkIssues()
		p.wantStarts(10, 11) // the third never starts while both slots are held
		p.wantSlots(2, 2)
	}

	running := []int{10, 11}
	next := []int{12, 13, 14}
	for _, n := range next {
		p.m.HandleExit(p.implAgentID(running[0]), 0)
		running = running[1:]
		p.wantSlots(1, 2)

		p.w.checkIssues()

		running = append(running, n)
		wantStarts := []int{10, 11}
		for _, s := range next {
			wantStarts = append(wantStarts, s)
			if s == n {
				break
			}
		}
		p.wantStarts(wantStarts...)
		p.wantSlots(2, 2)

		p.w.checkIssues() // a further poll starts nothing more
		p.wantStarts(wantStarts...)
	}
}

// AC2: free slots go to the lowest issue number, whatever order the poll
// returns.
func TestCheckIssues_OneSlotStartsOldestIssueFirst(t *testing.T) {
	p := newCapPipeline(t, 1)
	p.putStamped(10, 11, 12)
	p.w.selectIssues = func(_, label, _ string) (github.Selection, error) {
		byNumber := map[int]github.Issue{}
		for _, is := range p.gh.Select(label).Eligible {
			byNumber[is.Number] = is
		}
		return github.Selection{Eligible: []github.Issue{byNumber[12], byNumber[10], byNumber[11]}}, nil
	}

	p.w.checkIssues()
	p.wantStarts(10)
	p.m.HandleExit(p.implAgentID(10), 0)

	p.w.checkIssues()
	p.wantStarts(10, 11)
	p.m.HandleExit(p.implAgentID(11), 0)

	p.w.checkIssues()
	p.wantStarts(10, 11, 12)
}

// AC3: a retry holds its slot across the failed gap (the agent has exited and
// the retry has not started yet), so no other issue starts.
func TestCheckIssues_RetryHoldsOnlySlot(t *testing.T) {
	p := newCapPipeline(t, 1)
	p.putStamped(10, 11)
	p.w.checkIssues()
	p.wantStarts(10)

	// Exit non-zero: the agent is failed and its retry sleeps one second
	// before it starts, so this poll runs inside the gap.
	p.m.HandleExit(p.implAgentID(10), 1)
	p.w.checkIssues()
	p.wantStarts(10)
	p.wantSlots(1, 1)
	p.wantTracked(10)
	if fileExists(".kairon/retries/issue-11.count") {
		t.Error("issue #11 waited during the retry gap but was charged a retry attempt")
	}

	p.waitStarts(10, 2) // the retry is running
	p.w.checkIssues()
	p.wantStarts(10, 10)
	p.wantTracked(10)
	if fileExists(".kairon/retries/issue-11.count") {
		t.Error("issue #11 waited during the retry but was charged a retry attempt")
	}

	p.m.HandleExit(p.implAgentID(10), 0)
	p.w.checkIssues()
	p.wantStarts(10, 10, 11)
}

// AC3: an intake run holds a slot, so no other issue starts until it ends, and
// then the oldest waiting issue starts.
func TestCheckIssues_IntakeHoldsOnlySlot(t *testing.T) {
	p := newCapPipeline(t, 1)
	const intakeIssue = 12
	p.gh.Put(intakeIssue, "Export is slow", handIssue, e2eLabel)
	release := filepath.Join(p.dir, "release")
	reply := plannertest.CanonicalReply(e2eRepo, intakeIssue, plannertest.ValidBody("Dependencies: none"))
	p.next = func(string) *exec.Cmd { return plannertest.BlockingCommand(p.dir, reply, release) }

	p.w.checkIssues() // starts intake on #12, which takes the only slot
	if n := p.intakeStarts(); n != 1 {
		t.Fatalf("intake starts = %d, want 1", n)
	}
	p.wantSlots(1, 1)

	// Younger and older issues become eligible while intake runs.
	p.putStamped(10, 11)
	for range 3 {
		p.w.checkIssues()
		p.wantStarts()
		p.wantSlots(1, 1)
		p.wantTracked(intakeIssue)
		for _, n := range []int{10, 11} {
			if fileExists(fmt.Sprintf(".kairon/retries/issue-%d.count", n)) {
				t.Errorf("issue #%d waited behind intake but was charged a retry attempt", n)
			}
		}
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p.waitIntakeEnded(intakeIssue)
	p.wantSlots(0, 1)

	p.w.checkIssues()
	p.wantStarts(10) // the oldest waiting issue; #11 and the stamped #12 wait
	p.wantSlots(1, 1)
}

// AC4: an issue that waits keeps its retry budget and is considered again.
func TestCheckIssues_WaitingIssueKeepsRetryBudget(t *testing.T) {
	p := newCapPipeline(t, 1)
	p.putStamped(10) // alone on poll 1, so it takes the only slot
	p.w.checkIssues()
	p.wantStarts(10)
	p.putStamped(11, 12)
	writeRetryFile(t, 11, 1) // one earlier real attempt

	retryFile := ".kairon/retries/issue-11.count"
	before, err := os.ReadFile(retryFile)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		p.w.checkIssues()
		p.wantStarts(10)
		after, err := os.ReadFile(retryFile)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("poll %d: retry file = %q (%v), want unchanged %q", i+1, after, err, before)
		}
		if fileExists(".kairon/retries/issue-12.count") {
			t.Fatalf("poll %d: issue #12 has a retry count file although it never started", i+1)
		}
		p.wantTracked(10)
	}

	p.m.HandleExit(p.implAgentID(10), 0)
	p.w.checkIssues()

	p.wantStarts(10, 11)
	if got := p.w.getCurrentRetryCount(11); got != 2 {
		t.Errorf("retry count for #11 = %d, want 2 (old 1 + this attempt)", got)
	}
	if fileExists(".kairon/retries/issue-12.count") {
		t.Error("issue #12 still waits but has a retry count file")
	}
	p.wantTracked(11)
}

// AC6: each poll logs a waiting issue once, at info level, not as a failure.
func TestCheckIssues_LogsWaitingIssueOnceAtInfo(t *testing.T) {
	p := newCapPipeline(t, 1)
	p.putStamped(10, 11)
	buf := captureLog(t)

	const waiting = "issue #11 waiting for an agent slot (1/1 running)"
	for poll := 1; poll <= 2; poll++ {
		buf.reset()
		p.w.checkIssues()

		var matching, mentioning []string
		for _, line := range buf.lines() {
			if strings.Contains(line, waiting) {
				matching = append(matching, line)
			}
			if strings.Contains(line, "issue #11 ") || strings.HasSuffix(line, "issue #11") {
				mentioning = append(mentioning, line)
			}
		}
		if len(matching) != 1 {
			t.Fatalf("poll %d: %d lines contain %q, want exactly 1\nlog:\n%s", poll, len(matching), waiting, strings.Join(buf.lines(), "\n"))
		}
		if !strings.Contains(matching[0], "INFO:") {
			t.Errorf("poll %d: waiting line %q has no INFO: tag", poll, matching[0])
		}
		for _, line := range mentioning {
			for _, bad := range []string{"WARNING:", "ERROR:", "failed to spawn"} {
				if strings.Contains(line, bad) {
					t.Errorf("poll %d: line about #11 contains %q: %s", poll, bad, line)
				}
			}
		}
	}
}

// An older issue held back by an open dependency is not eligible to start, so
// it neither takes the slot nor is reported as waiting for one.
func TestCheckIssues_BlockedOlderIssueDoesNotTakeSlot(t *testing.T) {
	p := newCapPipeline(t, 1)
	p.gh.Put(99, "Prerequisite", "still open")
	p.gh.Put(10, "Blocked", provenance.Mark(plannertest.ValidBody("Dependencies: #99"), time.Now()), e2eLabel)
	p.putStamped(11, 12)
	buf := captureLog(t)

	p.w.checkIssues()

	p.wantStarts(11)
	for _, line := range buf.lines() {
		if strings.Contains(line, "issue #10 ") && strings.Contains(line, "waiting for an agent slot") {
			t.Errorf("blocked issue #10 logged as waiting for a slot: %s", line)
		}
	}
	waiting12 := 0
	for _, line := range buf.lines() {
		if strings.Contains(line, "issue #12 waiting for an agent slot (1/1 running)") {
			waiting12++
		}
	}
	if waiting12 != 1 {
		t.Errorf("issue #12 logged as waiting %d times, want 1", waiting12)
	}
	if fileExists(".kairon/retries/issue-10.count") {
		t.Error("blocked issue #10 has a retry count file")
	}
}

// The slot can be taken between the watcher's gate and Spawn's reservation (a
// validate run). Spawn then returns ErrAtCapacity: the watcher untracks and
// refunds the attempt and logs a waiting line, not a spawn failure.
func TestCheckIssues_SpawnAtCapacityUntracksAndRefunds(t *testing.T) {
	w, spawned := newSpawnWatcher(t, fmt.Errorf("%w (1/1 running)", agent.ErrAtCapacity))
	writeRetryFile(t, 7, 1) // one earlier real attempt
	// The gate sees a free slot; by the time spawn runs, the slot is taken.
	var reads int
	w.slots = func() (int, int) {
		reads++
		if reads == 1 {
			return 0, 1
		}
		return 1, 1
	}
	buf := captureLog(t)

	w.checkIssues()

	if want := []int{7}; !reflect.DeepEqual(*spawned, want) {
		t.Fatalf("spawned = %v, want %v", *spawned, want)
	}
	if got := trackedKeys(w); len(got) != 0 {
		t.Errorf("tracked = %v, want untracked", got)
	}
	if got := w.getCurrentRetryCount(7); got != 1 {
		t.Errorf("retry count = %d, want 1 (this attempt refunded)", got)
	}
	var waiting int
	for _, line := range buf.lines() {
		if strings.Contains(line, "failed to spawn") {
			t.Errorf("ErrAtCapacity logged as a spawn failure: %s", line)
		}
		if strings.Contains(line, "issue #7 waiting for an agent slot (1/1 running)") {
			waiting++
			if !strings.Contains(line, "INFO:") {
				t.Errorf("waiting line %q has no INFO: tag", line)
			}
		}
	}
	if waiting != 1 {
		t.Errorf("waiting lines = %d, want 1", waiting)
	}
}
