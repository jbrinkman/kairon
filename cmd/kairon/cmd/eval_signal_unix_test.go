//go:build unix

package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// TestEvalCommand_SIGINTKillsKiroCLIGroup: SIGINT sent to `kairon eval` while
// an agent kiro-cli call is in flight cancels the call, which kills the
// kiro-cli process group (kiro-cli and its sleep child). The run returns an
// error wrapping context.Canceled and saves no result for the interrupted
// case (issue #490).
func TestEvalCommand_SIGINTKillsKiroCLIGroup(t *testing.T) {
	runEvalInterruptTest(t, syscall.SIGINT)
}

// TestEvalCommand_SIGTERMKillsKiroCLIGroup is the SIGTERM counterpart.
func TestEvalCommand_SIGTERMKillsKiroCLIGroup(t *testing.T) {
	runEvalInterruptTest(t, syscall.SIGTERM)
}

func runEvalInterruptTest(t *testing.T, sig syscall.Signal) {
	t.Helper()
	guardSignals(t, syscall.SIGINT, syscall.SIGTERM)
	readyFile, pidFile := installBlockingFakeKiro(t)

	// The selftest fixtures need a pinned judge model to run with the
	// kiro-cli backend; the project config supplies it.
	evalsDir := evalSelfTestProject(t, "evals:\n  judge_model: claude-sonnet-5.5\n")
	evalBackend = inference.NameKiroCLI

	origSilence := evalCmd.SilenceUsage
	t.Cleanup(func() { evalCmd.SilenceUsage = origSilence })

	done := make(chan error, 1)
	go func() { done <- evalCmd.RunE(evalCmd, []string{"selftest"}) }()

	// Wait for the agent call to be in flight. If the run ends first, report
	// its error instead of timing out on the ready file.
	readyDeadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("eval returned before the fake kiro-cli was called: %v", err)
		default:
		}
		if time.Now().After(readyDeadline) {
			killRecordedGroups(pidFile)
			t.Fatalf("timed out after 5s waiting for %s", readyFile)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatalf("send %v: %v", sig, err)
	}

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(5 * time.Second):
		// Not cancelled. Killing the fake kiro-cli only fails one case; an
		// uncancelled run then starts the next. Keep killing the recorded
		// processes until the run ends, so the goroutine finishes before the
		// cleanups run, then fail on the assertion.
		t.Errorf("evalCmd.RunE did not return within 5s of %v", sig)
		for drained := false; !drained; {
			killRecordedGroups(pidFile)
			select {
			case <-done:
				drained = true
			case <-time.After(50 * time.Millisecond):
			}
		}
		return
	}
	if !errors.Is(runErr, context.Canceled) {
		t.Errorf("evalCmd.RunE error = %v, want an error wrapping context.Canceled", runErr)
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
			t.Errorf("process %d is still alive after %v", pid, sig)
		}
	}

	// The interrupted case must not be saved: no agent JSON in any results
	// directory (summary.json is the only file a run may have written).
	agentJSON, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*", "selftest*.json"))
	if len(agentJSON) != 0 {
		t.Errorf("saved agent results after interrupt: %v", agentJSON)
	}
}

// killRecordedGroups SIGKILLs every recorded PID and the process group of the
// first one (the fake kiro-cli is its group leader), so a case that was cut
// off before its sleep child was recorded leaves nothing behind.
func killRecordedGroups(pidFile string) {
	killRecordedPIDs(pidFile)
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	if f := strings.Fields(string(raw)); len(f) > 0 {
		if pid, err := strconv.Atoi(f[0]); err == nil && pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}
