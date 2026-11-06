//go:build unix

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// TestCheckCommand_SIGINTKillsKiroCLIGroup: SIGINT sent to `kairon check`
// while a judge kiro-cli call is in flight cancels the call, which kills the
// kiro-cli process group (kiro-cli and its sleep child). The command prints a
// FAIL line for the judge check and returns an error (issue #490).
func TestCheckCommand_SIGINTKillsKiroCLIGroup(t *testing.T) {
	guardSignals(t, syscall.SIGINT, syscall.SIGTERM)
	readyFile, pidFile := installBlockingFakeKiro(t)

	work := t.TempDir()
	inDir(t, work)
	writeEvalsConfig(t, "evals:\n  judge_model: claude-sonnet-5.5\n")
	checksFile := filepath.Join(work, "checks.yaml")
	body := "- type: judge\n  question: Does it greet the user?\n  files: [hello.txt]\n"
	if err := os.WriteFile(checksFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldChecks, oldDir, oldBackend, oldNew := checkChecks, checkDir, checkBackend, checkNewBackend
	t.Cleanup(func() {
		checkChecks, checkDir, checkBackend, checkNewBackend = oldChecks, oldDir, oldBackend, oldNew
		checkCmd.SilenceUsage = false
		checkCmd.SetOut(nil)
		checkCmd.SetErr(nil)
	})
	checkChecks, checkDir, checkBackend, checkNewBackend = checksFile, work, inference.NameKiroCLI, inference.New

	var stdout, stderr bytes.Buffer
	checkCmd.SetOut(&stdout)
	checkCmd.SetErr(&stderr)

	done := make(chan error, 1)
	go func() { done <- runCheckCommand(checkCmd, nil) }()

	waitForFile(t, readyFile)
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(5 * time.Second):
		// Not cancelled. Kill the fake processes so the goroutine can finish
		// before the cleanups run, then fail on the assertion.
		killRecordedPIDs(pidFile)
		<-done
		t.Fatal("runCheckCommand did not return within 5s of SIGINT")
	}
	if runErr == nil {
		t.Error("runCheckCommand error = nil, want a non-nil error after the judge call was cancelled")
	}
	if lines := outputLines(stdout.String()); len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL") {
		t.Errorf("want one FAIL line for the judge check, got:\n%s", stdout.String())
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	pids := strings.Fields(string(raw))
	if len(pids) != 2 {
		t.Fatalf("pid file = %q, want two PIDs (kiro-cli, sleep child)", raw)
	}
	for _, f := range pids {
		pid, err := strconv.Atoi(f)
		if err != nil {
			t.Fatalf("bad pid %q: %v", f, err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for processAlive(pid) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if processAlive(pid) {
			t.Errorf("process %d is still alive after SIGINT", pid)
		}
	}
}

// killRecordedPIDs SIGKILLs every PID the fake kiro-cli recorded.
func killRecordedPIDs(pidFile string) {
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	for _, f := range strings.Fields(string(raw)) {
		if pid, err := strconv.Atoi(f); err == nil && pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
