package eval

// Helpers the harness tests share with the check-engine tests that moved to
// internal/checks (test helpers cannot be imported across packages).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// evalOne evaluates a single check and returns its only result.
func evalOne(t *testing.T, c Check, in CheckInput) CheckResult {
	t.Helper()
	if c.Criterion == "" {
		c.Criterion = "crit"
	}
	rs := EvaluateChecks([]Check{c}, in)
	if len(rs) != 1 {
		t.Fatalf("want 1 result, got %d", len(rs))
	}
	return rs[0]
}

func wantPass(t *testing.T, r CheckResult) {
	t.Helper()
	if !r.Passed {
		t.Fatalf("want pass, got fail: %q (%s)", r.Detail, r.Label)
	}
}

func wantFail(t *testing.T, r CheckResult, detailSub string) {
	t.Helper()
	if r.Passed {
		t.Fatalf("want fail, got pass (%s)", r.Label)
	}
	if r.Detail == "" || !strings.Contains(r.Detail, detailSub) {
		t.Fatalf("detail %q does not contain %q", r.Detail, detailSub)
	}
}

func writeEvalFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func chkGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = hermeticGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func turnPtr(n int) *int { return &n }

func intp(n int) *int { return &n }
