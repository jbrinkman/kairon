package watcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/github"
)

type Watcher struct {
	config              *config.Config
	manager             *agent.Manager
	user                string // GitHub username for assignee filtering
	stop                chan struct{}
	tracked             map[int]bool
	mu                  sync.RWMutex
	started             bool
	backoffTracker      *BackoffTracker
	dependencyValidator *DependencyValidator

	// spawnAgent starts the implementation agent for an issue. It is set to
	// the manager's Spawn in New; tests replace it.
	spawnAgent func(issueNumber int, repo string) (*agent.Agent, error)

	// classifier evaluates an issue body's planner provenance. nil means no
	// classifier is wired and every issue is treated as ProvenanceValid.
	classifier ProvenanceClassifier
	// planner canonicalizes issues whose provenance is none or mismatch. New
	// sets the production kiro-cli runner; tests replace it.
	planner PlannerRunner
	// plannerInFlight and plannerAttempts are guarded by mu and created lazily
	// so a zero-value Watcher stays usable.
	plannerInFlight map[int]struct{} // issues with a planner run in progress
	plannerAttempts map[int]int      // planner runs started per issue in this process
}

func New(cfg *config.Config, mgr *agent.Manager, user string) *Watcher {
	return &Watcher{
		config:              cfg,
		manager:             mgr,
		user:                user,
		tracked:             make(map[int]bool),
		backoffTracker:      NewBackoffTracker(),
		dependencyValidator: NewDependencyValidator(),
		spawnAgent:          mgr.Spawn,
		planner:             newKiroPlannerRunner(cfg.Repo),
	}
}

// SetProvenanceClassifier wires the classifier that decides whether a polled
// issue goes to the implementation pipeline or to the planner. Passing nil
// restores the default of treating every issue as ProvenanceValid.
func (w *Watcher) SetProvenanceClassifier(c ProvenanceClassifier) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.classifier = c
}

func (w *Watcher) Start() {
	if w.started {
		return
	}
	w.cleanupOrphanedWorktrees()
	w.stop = make(chan struct{})
	w.started = true

	if w.user != "" {
		log.Printf("[watcher] started — polling %s every %s for label %q (assignee: %s)", w.config.Repo, w.config.PollInterval, w.config.Label, w.user)
	} else {
		log.Printf("[watcher] started — polling %s every %s for label %q", w.config.Repo, w.config.PollInterval, w.config.Label)
	}

	go w.pollLoop()
}

func (w *Watcher) Stop() {
	if !w.started {
		return
	}
	close(w.stop)
	w.started = false
	log.Printf("[watcher] stopped")
}

func (w *Watcher) pollLoop() {
	// Run immediately on start, then on interval
	w.checkIssues()

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.checkIssues()
		}
	}
}

func (w *Watcher) checkIssues() {
	// Increment polling round for backoff tracking
	w.backoffTracker.IncrementRound()

	log.Printf("[watcher] polling for issues...")
	issues, err := github.ListIssues(w.config.Repo, w.config.Label, w.user)
	if err != nil {
		log.Printf("[watcher] error fetching issues: %v", err)
		return
	}

	for _, issue := range issues {
		w.processIssue(issue)
	}
}

// processIssue evaluates one polled issue and spawns the implementation agent
// when every criterion passes.
func (w *Watcher) processIssue(issue github.Issue) {
	w.mu.RLock()
	tracked := w.tracked[issue.Number]
	w.mu.RUnlock()

	if tracked {
		return
	}

	if w.hasActiveWorktree(issue.Number) {
		log.Printf("[watcher] issue #%d already has active worktree, skipping", issue.Number)
		return
	}

	if w.hasExceededGlobalRetries(issue.Number) {
		log.Printf("[watcher] issue #%d exceeded retry limit (%d attempts), skipping", issue.Number, w.config.MaxRetries)
		return
	}

	// Check dependency validation with backoff
	if !w.backoffTracker.ShouldCheck(issue.Number) {
		roundsRemaining := w.backoffTracker.GetRoundsUntilCheck(issue.Number)
		log.Printf("[watcher] issue #%d in backoff, checking again in %d rounds", issue.Number, roundsRemaining)
		return
	}

	validationResult, err := w.dependencyValidator.ValidateIssue(w.config.Repo, issue.Number, issue.Body)
	if err != nil {
		log.Printf("[watcher] dependency validation error for issue #%d: %v", issue.Number, err)
		w.backoffTracker.RecordFailure(issue.Number)
		return
	}

	// Log circular dependencies as warnings but don't block processing
	if len(validationResult.CircularDependencies) > 0 {
		log.Printf("[watcher] WARNING: circular dependency detected for issue #%d: %v", issue.Number, validationResult.CircularDependencies)
	}

	// Skip issues with unresolved dependencies
	if !validationResult.IsValid {
		log.Printf("[watcher] skipping issue #%d due to unresolved dependencies: %v", issue.Number, validationResult.UnresolvedDependencies)
		w.backoffTracker.RecordFailure(issue.Number)
		return
	}

	// Provenance is evaluated after every other criterion, so an unmarked issue
	// that is still waiting on dependencies never reaches the classifier.
	switch p := w.classify(issue.Body); p {
	case ProvenanceValid:
		w.mu.Lock()
		delete(w.plannerAttempts, issue.Number)
		w.mu.Unlock()
	case ProvenanceNone, ProvenanceMismatch:
		// Not tracked and not retry-counted: planner runs are not
		// implementation attempts, and the issue must reach step 7 once the
		// planner has made its body valid.
		w.routeToPlanner(issue, p)
		return
	default:
		log.Printf("[watcher] ERROR: unrecognised provenance %q for issue #%d, skipping", p, issue.Number)
		return
	}

	w.mu.Lock()
	w.tracked[issue.Number] = true
	w.mu.Unlock()

	w.incrementGlobalRetryCount(issue.Number)
	attempt := w.getCurrentRetryCount(issue.Number)
	log.Printf("[watcher] spawning agent for issue #%d %q (attempt %d)", issue.Number, issue.Title, attempt)
	if _, err := w.spawnAgent(issue.Number, w.config.Repo); err != nil {
		log.Printf("[watcher] failed to spawn agent for issue #%d: %v", issue.Number, err)
	}
}

// classify returns the provenance of an issue body. With no classifier wired
// every body is ProvenanceValid.
func (w *Watcher) classify(body string) Provenance {
	w.mu.RLock()
	c := w.classifier
	w.mu.RUnlock()
	if c == nil {
		return ProvenanceValid
	}
	return c.Classify(body)
}

// routeToPlanner starts a planner run for an issue whose body is not yet
// canonical. It starts at most one run per issue at a time and at most
// config.MaxRetries runs per issue per process (reset when the issue is seen
// ProvenanceValid), which bounds spend if the planner never fixes the body.
func (w *Watcher) routeToPlanner(issue github.Issue, p Provenance) {
	w.mu.Lock()
	planner := w.planner
	if planner == nil {
		w.mu.Unlock()
		log.Printf("[watcher] issue #%d has provenance %s but no planner is configured, skipping", issue.Number, p)
		return
	}
	if _, busy := w.plannerInFlight[issue.Number]; busy {
		w.mu.Unlock()
		log.Printf("[watcher] planner run already in flight for issue #%d, skipping", issue.Number)
		return
	}
	// Recheck backoff at admission. processIssue gates on ShouldCheck at the
	// top of its loop, but a later poll tick can reach routeToPlanner after a
	// prior run's failure was recorded, so re-evaluate here before admitting a
	// paid run. BackoffTracker has its own mutex, so this is safe under w.mu.
	if !w.backoffTracker.ShouldCheck(issue.Number) {
		w.mu.Unlock()
		log.Printf("[watcher] issue #%d is in backoff, not starting a planner run", issue.Number)
		return
	}
	if w.plannerAttempts[issue.Number] >= w.config.MaxRetries {
		w.mu.Unlock()
		log.Printf("[watcher] WARNING: issue #%d reached the planner attempt limit (%d), skipping", issue.Number, w.config.MaxRetries)
		return
	}
	if w.plannerInFlight == nil {
		w.plannerInFlight = make(map[int]struct{})
	}
	if w.plannerAttempts == nil {
		w.plannerAttempts = make(map[int]int)
	}
	w.plannerInFlight[issue.Number] = struct{}{}
	w.plannerAttempts[issue.Number]++
	attempt := w.plannerAttempts[issue.Number]
	w.mu.Unlock()

	log.Printf("[watcher] routing issue #%d %q to planner (provenance %s, run %d)", issue.Number, issue.Title, p, attempt)
	go func() {
		err := planner.Run(issue, p)

		// Record the failure before clearing the in-flight marker so a
		// concurrent poll cannot observe the issue as neither in-flight nor
		// backed-off and admit a fresh run in the gap.
		if err != nil {
			log.Printf("[watcher] planner run for issue #%d failed: %v", issue.Number, err)
			w.backoffTracker.RecordFailure(issue.Number)
		}

		w.mu.Lock()
		delete(w.plannerInFlight, issue.Number)
		w.mu.Unlock()
	}()
}

func (w *Watcher) cleanupOrphanedWorktrees() {
	worktreesDir := ".worktrees"
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "issue-") {
			continue
		}

		parts := strings.Split(name, "-")
		if len(parts) < 3 {
			continue
		}

		pidStr := parts[len(parts)-1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		if !w.isProcessRunning(pid) {
			worktreePath := filepath.Join(worktreesDir, name)
			log.Printf("[watcher] cleaning up orphaned worktree: %s (PID %d no longer running)", name, pid)
			os.RemoveAll(worktreePath)
		}
	}
}

func (w *Watcher) hasActiveWorktree(issueNumber int) bool {
	worktreesDir := ".worktrees"
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return false
	}

	prefix := fmt.Sprintf("issue-%d-", issueNumber)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}

		// Extract PID from worktree name and check if process is running
		parts := strings.Split(entry.Name(), "-")
		if len(parts) < 3 {
			continue
		}

		pidStr := parts[len(parts)-1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		if w.isProcessRunning(pid) {
			return true
		}
	}
	return false
}

func (w *Watcher) isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func (w *Watcher) hasExceededGlobalRetries(issueNumber int) bool {
	retryFile := fmt.Sprintf(".kairon/retries/issue-%d.count", issueNumber)
	data, err := os.ReadFile(retryFile)
	if err != nil {
		return false
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}

	return count >= w.config.MaxRetries
}

func (w *Watcher) incrementGlobalRetryCount(issueNumber int) {
	retryDir := ".kairon/retries"
	os.MkdirAll(retryDir, 0755)

	retryFile := fmt.Sprintf("%s/issue-%d.count", retryDir, issueNumber)

	count := 0
	if data, err := os.ReadFile(retryFile); err == nil {
		if c, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			count = c
		}
	}

	count++
	os.WriteFile(retryFile, []byte(fmt.Sprintf("%d\n", count)), 0644)
}

func (w *Watcher) getCurrentRetryCount(issueNumber int) int {
	retryFile := fmt.Sprintf(".kairon/retries/issue-%d.count", issueNumber)
	data, err := os.ReadFile(retryFile)
	if err != nil {
		return 0
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}

	return count
}

func (w *Watcher) Running() bool {
	return w.started
}
