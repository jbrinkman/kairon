package planner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Intake runs in parallel, so two planners can create a planning worktree in
// the same second. Each must get its own worktree, and cleanup must accept
// every name create produces.
func TestPlanningWorktreeScripts_ParallelRunsGetDistinctWorktrees(t *testing.T) {
	bash := requireBash(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	scripts, err := filepath.Abs("../../.kairon/scripts")
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Start both creates together so they land in the same second.
	type result struct {
		path string
		err  error
		out  string
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			cmd := exec.Command(bash, filepath.Join(scripts, "planning-worktree-create.sh"))
			cmd.Dir = repo
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			results <- result{strings.TrimSpace(string(out)), err, stderr.String()}
		}()
	}
	var paths []string
	for range 2 {
		r := <-results
		if r.err != nil {
			t.Fatalf("planning-worktree-create.sh: %v\n%s", r.err, r.out)
		}
		paths = append(paths, r.path)
	}
	if paths[0] == paths[1] {
		t.Fatalf("both runs got %s", paths[0])
	}

	for _, p := range paths {
		rel, err := filepath.Rel(repo, p)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bash, filepath.Join(scripts, "planning-worktree-cleanup.sh"), rel)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cleanup %s: %v\n%s", rel, err, out)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists after cleanup", p)
		}
	}
}
