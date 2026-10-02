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
	stop                chan struct{}
	tracked             map[int]bool
	mu                  sync.RWMutex
	started             bool
	backoffTracker      *BackoffTracker
	dependencyValidator *DependencyValidator
}

func New(cfg *config.Config, mgr *agent.Manager) *Watcher {
	return &Watcher{
		config:              cfg,
		manager:             mgr,
		tracked:             make(map[int]bool),
		backoffTracker:      NewBackoffTracker(),
		dependencyValidator: NewDependencyValidator(),
	}
}

func (w *Watcher) Start() {
	if w.started {
		return
	}
	w.cleanupOrphanedWorktrees()
	w.stop = make(chan struct{})
	w.started = true
	log.Printf("[watcher] started — polling %s every %s for label %q", w.config.Repo, w.config.PollInterval, w.config.Label)
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
	issues, err := github.ListIssues(w.config.Repo, w.config.Label)
	if err != nil {
		log.Printf("[watcher] error fetching issues: %v", err)
		return
	}

	for _, issue := range issues {
		w.mu.RLock()
		tracked := w.tracked[issue.Number]
		w.mu.RUnlock()

		if tracked {
			continue
		}

		if w.hasActiveWorktree(issue.Number) {
			log.Printf("[watcher] issue #%d already has active worktree, skipping", issue.Number)
			continue
		}

		if w.hasExceededGlobalRetries(issue.Number) {
			log.Printf("[watcher] issue #%d exceeded retry limit (%d attempts), skipping", issue.Number, w.config.MaxRetries)
			continue
		}

		// Check dependency validation with backoff
		if !w.backoffTracker.ShouldCheck(issue.Number) {
			roundsRemaining := w.backoffTracker.GetRoundsUntilCheck(issue.Number)
			log.Printf("[watcher] issue #%d in backoff, checking again in %d rounds", issue.Number, roundsRemaining)
			continue
		}

		validationResult, err := w.dependencyValidator.ValidateIssue(w.config.Repo, issue.Number, issue.Body)
		if err != nil {
			log.Printf("[watcher] dependency validation error for issue #%d: %v", issue.Number, err)
			w.backoffTracker.RecordFailure(issue.Number)
			continue
		}

		// Log circular dependencies as warnings but don't block processing
		if len(validationResult.CircularDependencies) > 0 {
			log.Printf("[watcher] WARNING: circular dependency detected for issue #%d: %v", issue.Number, validationResult.CircularDependencies)
		}

		// Skip issues with unresolved dependencies
		if !validationResult.IsValid {
			log.Printf("[watcher] skipping issue #%d due to unresolved dependencies: %v", issue.Number, validationResult.UnresolvedDependencies)
			w.backoffTracker.RecordFailure(issue.Number)
			continue
		}

		w.mu.Lock()
		w.tracked[issue.Number] = true
		w.mu.Unlock()

		w.incrementGlobalRetryCount(issue.Number)
		attempt := w.getCurrentRetryCount(issue.Number)
		log.Printf("[watcher] spawning agent for issue #%d %q (attempt %d)", issue.Number, issue.Title, attempt)
		if _, err := w.manager.Spawn(issue.Number, w.config.Repo); err != nil {
			log.Printf("[watcher] failed to spawn agent for issue #%d: %v", issue.Number, err)
		}
	}
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
