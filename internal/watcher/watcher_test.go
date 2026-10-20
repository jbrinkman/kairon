package watcher

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/github"
)

// harness wires a Watcher to fakes so processIssue can be driven without gh,
// kiro-cli or the network. It also moves the test into an empty temp dir so
// .worktrees/ and .kairon/retries/ are isolated.
type harness struct {
	t *testing.T
	w *Watcher

	mu      sync.Mutex
	spawned []int

	planner *fakePlanner

	// depState is the state the fake dependency lookup reports for every
	// dependency ("closed" unless a test sets it).
	depState string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Chdir(t.TempDir())

	h := &harness{t: t, depState: "closed", planner: &fakePlanner{}}
	h.w = New(&config.Config{Repo: "o/r", MaxRetries: 3, PollInterval: time.Minute}, nil, "")
	h.w.spawnAgent = func(issueNumber int, repo string) (*agent.Agent, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.spawned = append(h.spawned, issueNumber)
		return nil, nil
	}
	h.w.planner = h.planner
	h.w.dependencyValidator.getIssueDetails = func(repo string, number int) (*github.IssueDetails, error) {
		return &github.IssueDetails{State: h.depState}, nil
	}
	return h
}

func (h *harness) spawnedIssues() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]int(nil), h.spawned...)
}

func (h *harness) isTracked(n int) bool {
	h.w.mu.RLock()
	defer h.w.mu.RUnlock()
	return h.w.tracked[n]
}

func retryFile(n int) string {
	return fmt.Sprintf(".kairon/retries/issue-%d.count", n)
}

func writeRetryCount(t *testing.T, n, count int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(".kairon/retries", 0o755))
	require.NoError(t, os.WriteFile(retryFile(n), []byte(strconv.Itoa(count)+"\n"), 0o644))
}

func readRetryCount(t *testing.T, n int) int {
	t.Helper()
	data, err := os.ReadFile(retryFile(n))
	require.NoError(t, err)
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return count
}

// --- Characterization tests: the pre-existing per-issue criteria ---

func TestProcessIssue_TrackedIssueIsSkipped(t *testing.T) {
	h := newHarness(t)
	h.w.tracked[7] = true

	h.w.processIssue(github.Issue{Number: 7, Title: "t", Body: "b"})

	assert.Empty(t, h.spawnedIssues())
	assert.NoFileExists(t, retryFile(7))
}

func TestProcessIssue_ActiveWorktreeIsSkipped(t *testing.T) {
	h := newHarness(t)
	// A worktree named for this process: its PID is running, so it is active.
	require.NoError(t, os.MkdirAll(fmt.Sprintf(".worktrees/issue-7-%d", os.Getpid()), 0o755))

	h.w.processIssue(github.Issue{Number: 7, Title: "t", Body: "b"})

	assert.Empty(t, h.spawnedIssues())
	assert.False(t, h.isTracked(7))
	assert.NoFileExists(t, retryFile(7))
}

func TestProcessIssue_RetryLimitIsSkipped(t *testing.T) {
	h := newHarness(t)
	writeRetryCount(t, 7, h.w.config.MaxRetries)

	h.w.processIssue(github.Issue{Number: 7, Title: "t", Body: "b"})

	assert.Empty(t, h.spawnedIssues())
	assert.False(t, h.isTracked(7))
	assert.Equal(t, h.w.config.MaxRetries, readRetryCount(t, 7), "count must not change")
}

func TestProcessIssue_UnresolvedDependencyIsSkippedWithBackoff(t *testing.T) {
	h := newHarness(t)
	h.depState = "open"

	h.w.processIssue(github.Issue{Number: 7, Title: "t", Body: "Depends on Issue #5"})

	assert.Empty(t, h.spawnedIssues())
	assert.False(t, h.isTracked(7))
	assert.NoFileExists(t, retryFile(7))
	assert.False(t, h.w.backoffTracker.ShouldCheck(7), "a failure must be recorded so the issue backs off")
	assert.Positive(t, h.w.backoffTracker.GetRoundsUntilCheck(7))
}

func TestProcessIssue_BackoffGateSkipsValidation(t *testing.T) {
	h := newHarness(t)
	h.w.backoffTracker.RecordFailure(7)

	h.w.processIssue(github.Issue{Number: 7, Title: "t", Body: "b"})

	assert.Empty(t, h.spawnedIssues())
	assert.False(t, h.isTracked(7))
}

func TestProcessIssue_CleanIssueSpawnsOnce(t *testing.T) {
	h := newHarness(t)
	issue := github.Issue{Number: 7, Title: "t", Body: "no dependencies"}

	h.w.processIssue(issue)
	h.w.processIssue(issue) // second poll: tracked, so no second spawn

	assert.Equal(t, []int{7}, h.spawnedIssues())
	assert.True(t, h.isTracked(7))
	assert.Equal(t, 1, readRetryCount(t, 7))
}

// --- Fakes ---

// fakeClassifier returns a fixed result and records the bodies it was given.
type fakeClassifier struct {
	mu     sync.Mutex
	result Provenance
	bodies []string
}

func (c *fakeClassifier) Classify(body string) Provenance {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, body)
	return c.result
}

func (c *fakeClassifier) callBodies() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.bodies...)
}

func (c *fakeClassifier) set(p Provenance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result = p
}

type plannerCall struct {
	issue github.Issue
	prov  Provenance
}

// fakePlanner records Run calls. When release is non-nil, Run blocks until it
// is closed. started receives one value per Run, as it begins.
type fakePlanner struct {
	mu      sync.Mutex
	calls   []plannerCall
	release chan struct{}
	err     error
	started chan struct{}
}

func (p *fakePlanner) Run(issue github.Issue, prov Provenance) error {
	p.mu.Lock()
	p.calls = append(p.calls, plannerCall{issue, prov})
	release, err, started := p.release, p.err, p.started
	p.mu.Unlock()

	if started != nil {
		started <- struct{}{}
	}
	if release != nil {
		<-release
	}
	return err
}

func (p *fakePlanner) recorded() []plannerCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]plannerCall(nil), p.calls...)
}

// waitForPlannerStart blocks until the fake planner's Run has begun, failing
// the test (rather than hanging it) if no run starts.
func (h *harness) waitForPlannerStart() {
	h.t.Helper()
	select {
	case <-h.planner.started:
	case <-time.After(5 * time.Second):
		h.t.Fatal("planner run never started")
	}
}

// waitForPlannerIdle blocks until no planner run is in flight for the issue.
// The in-flight entry is cleared by the run goroutine after Run returns, so
// this is the only observable "run finished" signal.
func (h *harness) waitForPlannerIdle(n int) {
	h.t.Helper()
	require.Eventually(h.t, func() bool {
		h.w.mu.RLock()
		defer h.w.mu.RUnlock()
		_, busy := h.w.plannerInFlight[n]
		return !busy
	}, 5*time.Second, time.Millisecond, "planner run for #%d never finished", n)
}

// --- Provenance routing ---

func TestProcessIssue_ClassifiesEachState(t *testing.T) {
	tests := []struct {
		name        string
		prov        Provenance
		wantSpawn   bool
		wantPlanner bool
	}{
		{"valid", ProvenanceValid, true, false},
		{"none", ProvenanceNone, false, true},
		{"mismatch", ProvenanceMismatch, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			c := &fakeClassifier{result: tc.prov}
			h.w.SetProvenanceClassifier(c)
			issue := github.Issue{Number: 11, Title: "t", Body: "body for " + tc.name}

			h.w.processIssue(issue)
			h.waitForPlannerIdle(11)

			assert.Equal(t, []string{issue.Body}, c.callBodies(), "classifier must receive the issue body")
			assert.Equal(t, tc.wantSpawn, len(h.spawnedIssues()) == 1)
			assert.Equal(t, tc.wantPlanner, len(h.planner.recorded()) == 1)
		})
	}
}

func TestProcessIssue_ValidSpawns(t *testing.T) {
	h := newHarness(t)
	h.w.SetProvenanceClassifier(&fakeClassifier{result: ProvenanceValid})

	h.w.processIssue(github.Issue{Number: 11, Title: "t", Body: "b"})

	assert.Equal(t, []int{11}, h.spawnedIssues())
	assert.Empty(t, h.planner.recorded())
	assert.True(t, h.isTracked(11))
	assert.Equal(t, 1, readRetryCount(t, 11))
}

func TestProcessIssue_NilClassifierTreatedAsValid(t *testing.T) {
	h := newHarness(t)
	require.Nil(t, h.w.classifier, "New must not wire a classifier")

	h.w.processIssue(github.Issue{Number: 11, Title: "t", Body: "no marker at all"})

	assert.Equal(t, []int{11}, h.spawnedIssues())
	assert.Empty(t, h.planner.recorded())
	assert.True(t, h.isTracked(11))
}

func TestProcessIssue_NoneRoutesToPlanner(t *testing.T) {
	testRoutesToPlanner(t, ProvenanceNone)
}

func TestProcessIssue_MismatchRoutesToPlanner(t *testing.T) {
	testRoutesToPlanner(t, ProvenanceMismatch)
}

func testRoutesToPlanner(t *testing.T, prov Provenance) {
	t.Helper()
	h := newHarness(t)
	h.w.SetProvenanceClassifier(&fakeClassifier{result: prov})
	issue := github.Issue{Number: 11, Title: "t", Body: "b"}

	h.w.processIssue(issue)
	h.waitForPlannerIdle(11)

	assert.Equal(t, []plannerCall{{issue: issue, prov: prov}}, h.planner.recorded())
	assert.Empty(t, h.spawnedIssues())
	assert.False(t, h.isTracked(11), "routing to the planner must not track the issue")
	assert.NoFileExists(t, retryFile(11), "planner runs must not consume implementation retries")
}

func TestProcessIssue_PlannerNotRepeatedWhileInFlight(t *testing.T) {
	h := newHarness(t)
	h.w.SetProvenanceClassifier(&fakeClassifier{result: ProvenanceNone})
	h.planner.release = make(chan struct{})
	h.planner.started = make(chan struct{}, 8)
	issue := github.Issue{Number: 11, Title: "t", Body: "b"}

	h.w.processIssue(issue)
	h.waitForPlannerStart() // first run is now blocked inside Run
	h.w.processIssue(issue)

	close(h.planner.release)
	h.waitForPlannerIdle(11)
	assert.Len(t, h.planner.recorded(), 1, "second poll must not start a second run")

	h.w.processIssue(issue)
	h.waitForPlannerIdle(11)
	assert.Len(t, h.planner.recorded(), 2, "a poll after the run ended starts a new run")
	assert.Empty(t, h.spawnedIssues())
}

func TestProcessIssue_DependenciesGateBeforeProvenance(t *testing.T) {
	h := newHarness(t)
	h.depState = "open"
	c := &fakeClassifier{result: ProvenanceNone}
	h.w.SetProvenanceClassifier(c)

	h.w.processIssue(github.Issue{Number: 11, Title: "t", Body: "no marker\nDepends on Issue #5"})

	assert.Empty(t, c.callBodies(), "classifier must not run before dependencies resolve")
	assert.Empty(t, h.planner.recorded())
	assert.Empty(t, h.spawnedIssues())
	assert.False(t, h.w.backoffTracker.ShouldCheck(11), "dependency failure must be recorded for backoff")
}

func TestProcessIssue_UnknownProvenanceFailsClosed(t *testing.T) {
	h := newHarness(t)
	h.w.SetProvenanceClassifier(&fakeClassifier{result: ProvenanceUnknown})

	h.w.processIssue(github.Issue{Number: 11, Title: "t", Body: "b"})

	assert.Empty(t, h.spawnedIssues())
	assert.Empty(t, h.planner.recorded())
	assert.False(t, h.isTracked(11))
	assert.NoFileExists(t, retryFile(11))
}

func TestRouteToPlanner_AttemptCap(t *testing.T) {
	h := newHarness(t)
	c := &fakeClassifier{result: ProvenanceNone}
	h.w.SetProvenanceClassifier(c)
	issue := github.Issue{Number: 11, Title: "t", Body: "b"}
	max := h.w.config.MaxRetries

	for i := 1; i <= max; i++ {
		h.w.processIssue(issue)
		h.waitForPlannerIdle(11)
		require.Len(t, h.planner.recorded(), i)
	}

	h.w.processIssue(issue)
	h.waitForPlannerIdle(11)
	assert.Len(t, h.planner.recorded(), max, "no run may start after MaxRetries runs")

	// Observing the issue Valid resets the counter.
	c.set(ProvenanceValid)
	h.w.processIssue(issue)
	require.Equal(t, []int{11}, h.spawnedIssues())

	// Simulate the issue later being edited again: untrack it and clear the
	// implementation retry count, then classify it as unmarked once more.
	h.w.mu.Lock()
	delete(h.w.tracked, 11)
	h.w.mu.Unlock()
	require.NoError(t, os.Remove(retryFile(11)))
	c.set(ProvenanceMismatch)

	h.w.processIssue(issue)
	h.waitForPlannerIdle(11)
	assert.Len(t, h.planner.recorded(), max+1, "counter must have been reset by the Valid observation")
}

func TestRouteToPlanner_FailureRecordsBackoff(t *testing.T) {
	h := newHarness(t)
	h.w.SetProvenanceClassifier(&fakeClassifier{result: ProvenanceNone})
	h.planner.err = fmt.Errorf("planner exploded")
	require.True(t, h.w.backoffTracker.ShouldCheck(11))

	h.w.processIssue(github.Issue{Number: 11, Title: "t", Body: "b"})
	h.waitForPlannerIdle(11)

	assert.Len(t, h.planner.recorded(), 1)
	assert.Eventually(t, func() bool { return !h.w.backoffTracker.ShouldCheck(11) },
		5*time.Second, time.Millisecond, "planner error must record a backoff failure")
}

// TestRouteToPlanner_BackoffBlocksRepeatRun guards the admission check: an
// issue already in backoff must not start another (paid) planner run even when
// routeToPlanner is reached directly, as a later poll tick can be between
// processIssue's top-of-loop ShouldCheck gate and admission.
func TestRouteToPlanner_BackoffBlocksRepeatRun(t *testing.T) {
	h := newHarness(t)
	h.w.backoffTracker.RecordFailure(11)
	require.False(t, h.w.backoffTracker.ShouldCheck(11),
		"precondition: issue must be in backoff")

	h.w.routeToPlanner(github.Issue{Number: 11, Title: "t", Body: "b"}, ProvenanceNone)

	assert.Empty(t, h.planner.recorded(),
		"no planner run may start while the issue is in backoff")
	h.w.mu.RLock()
	_, busy := h.w.plannerInFlight[11]
	h.w.mu.RUnlock()
	assert.False(t, busy, "no in-flight marker may be set for a backed-off issue")
}

func TestRouteToPlanner_NilPlannerDoesNothing(t *testing.T) {
	h := newHarness(t)
	h.w.planner = nil
	h.w.SetProvenanceClassifier(&fakeClassifier{result: ProvenanceNone})

	h.w.processIssue(github.Issue{Number: 11, Title: "t", Body: "b"})

	assert.Empty(t, h.spawnedIssues())
	assert.False(t, h.isTracked(11))
	assert.NoFileExists(t, retryFile(11))
}

func TestWatcher_ZeroValueIsSafe(t *testing.T) {
	w := &Watcher{}
	assert.NotPanics(t, func() {
		w.SetProvenanceClassifier(nil)
		w.SetProvenanceClassifier(ProvenanceClassifierFunc(func(string) Provenance { return ProvenanceValid }))
		w.routeToPlanner(github.Issue{Number: 1}, ProvenanceNone)
		_ = w.Running()
	})
}
