//go:build unix

package eval

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func intp(n int) *int { return &n }

func TestEvaluateChecks_CommandExitCodes(t *testing.T) {
	in := CheckInput{Dir: t.TempDir()}
	wantPass(t, evalOne(t, Check{Type: CheckCommand, Run: "true"}, in))
	wantPass(t, evalOne(t, Check{Type: CheckCommand, Run: "exit 3", ExpectExit: intp(3)}, in))
	wantPass(t, evalOne(t, Check{Type: CheckCommand, Run: "false", ExpectExit: intp(1)}, in))

	r := evalOne(t, Check{Type: CheckCommand, Run: "echo boom-output; exit 1"}, in)
	wantFail(t, r, "exit status 1")
	wantFail(t, r, "boom-output")
	wantFail(t, evalOne(t, Check{Type: CheckCommand, Run: "true", ExpectExit: intp(1)}, in), "want 1")
	wantFail(t, evalOne(t, Check{Type: CheckCommand, Run: "kill -9 $$"}, in), "signal")
}

func TestEvaluateChecks_CommandRunsInDirWithoutStdin(t *testing.T) {
	dir := t.TempDir()
	in := CheckInput{Dir: dir}
	wantPass(t, evalOne(t, Check{Type: CheckCommand, Run: "pwd -P > where.txt; cat > /dev/null"}, in))
	got, err := os.ReadFile(filepath.Join(dir, "where.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("cwd = %q, want %q", got, want)
	}
}

func TestEvaluateChecks_CommandEnvironment(t *testing.T) {
	t.Setenv("GH_TOKEN", "secret-a")
	t.Setenv("GITHUB_TOKEN", "secret-b")
	t.Setenv("GH_ENTERPRISE_TOKEN", "secret-c")
	t.Setenv("KAIRON_CHECK_TEST", "yes")
	in := CheckInput{Dir: t.TempDir()}
	wantPass(t, evalOne(t, Check{Type: CheckCommand,
		Run: `test -z "$GH_TOKEN" && test -z "$GITHUB_TOKEN" && test -z "$GH_ENTERPRISE_TOKEN" && test "$KAIRON_CHECK_TEST" = yes`}, in))
}

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func readPID(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if !processAlive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("process %d is still running", pid)
}

func TestEvaluateChecks_CommandTimeoutKillsProcessGroup(t *testing.T) {
	old := checkCommandTimeout
	checkCommandTimeout = 400 * time.Millisecond
	defer func() { checkCommandTimeout = old }()
	dir := t.TempDir()

	start := time.Now()
	r := evalOne(t, Check{Type: CheckCommand, Run: "sleep 60 & echo $! > pid.txt; wait"}, CheckInput{Dir: dir})
	if el := time.Since(start); el > 15*time.Second {
		t.Fatalf("timeout not enforced: took %s", el)
	}
	wantFail(t, r, "timed out")
	waitDead(t, readPID(t, filepath.Join(dir, "pid.txt")))
}

func TestEvaluateChecks_CommandStrayBackgroundProcessKilled(t *testing.T) {
	dir := t.TempDir()
	r := evalOne(t, Check{Type: CheckCommand, Run: "sleep 60 & echo $! > pid.txt"}, CheckInput{Dir: dir})
	wantPass(t, r)
	waitDead(t, readPID(t, filepath.Join(dir, "pid.txt")))
}

func TestEvaluateChecks_NonCommandChecksRunBeforeCommands(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "keep.txt", "k")
	rs := EvaluateChecks([]Check{
		{Criterion: "c", Type: CheckCommand, Run: "touch made.txt; rm keep.txt"},
		{Criterion: "c", Type: CheckFileAbsent, Path: "made.txt"},
		{Criterion: "c", Type: CheckFileExists, Path: "keep.txt"},
	}, CheckInput{Dir: dir})
	for i, r := range rs {
		if r.Index != i+1 || !r.Passed {
			t.Fatalf("result %d = %+v", i, r)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "made.txt")); err != nil {
		t.Fatal("command did not run")
	}
}
