package watcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jbrinkman/kairon/internal/github"
)

// kiroPlannerRunner is the production PlannerRunner. It starts the planner
// agent through kiro-cli, the same way the manager starts krew-lead, and logs
// its output to .kairon/logs/planner-issue-<n>.log.
type kiroPlannerRunner struct {
	repo string
	// run executes the built command and blocks until it exits. Tests replace
	// it so no real kiro-cli is started.
	run func(*exec.Cmd) error
}

// newKiroPlannerRunner returns a runner for repo that executes commands with
// (*exec.Cmd).Run.
func newKiroPlannerRunner(repo string) *kiroPlannerRunner {
	return &kiroPlannerRunner{
		repo: repo,
		run:  (*exec.Cmd).Run,
	}
}

// command builds, without starting, the kiro-cli invocation that asks the
// planner to canonicalize the issue.
func (r *kiroPlannerRunner) command(issue github.Issue, p Provenance) *exec.Cmd {
	cmd := exec.Command("kiro-cli", "chat",
		"--agent", "planner",
		"--no-interactive",
		"--trust-all-tools",
		r.prompt(issue, p))
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("ISSUE_NUMBER=%d", issue.Number),
		fmt.Sprintf("REPO=%s", r.repo),
		fmt.Sprintf("KAIRON_WATCHER_PID=%d", os.Getpid()))
	return cmd
}

// prompt describes the issue, why it needs the planner, and what to do about it.
func (r *kiroPlannerRunner) prompt(issue github.Issue, p Provenance) string {
	var reason string
	switch p {
	case ProvenanceNone:
		reason = "it has no planner marker"
	case ProvenanceMismatch:
		reason = "its body was edited after generation"
	default:
		reason = "its planner provenance could not be verified"
	}
	return fmt.Sprintf(
		"Canonicalize issue #%d from repo %s. Provenance: %s (%s). "+
			"Rewrite the issue into canonical planner form so its body carries a valid planner hash.",
		issue.Number, r.repo, p, reason)
}

// Run executes one planner run for the issue and returns the run error. Output
// from the planner is appended to .kairon/logs/planner-issue-<n>.log.
func (r *kiroPlannerRunner) Run(issue github.Issue, p Provenance) error {
	logDir := filepath.Join(".kairon", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("failed to create planner log directory: %w", err)
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("planner-issue-%d.log", issue.Number))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open planner log file: %w", err)
	}
	defer logFile.Close()

	cmd := r.command(issue, p)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	return r.run(cmd)
}
