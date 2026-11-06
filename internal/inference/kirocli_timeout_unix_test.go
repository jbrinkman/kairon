//go:build unix

package inference

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
)

// Issue #420: a timed-out kiro-cli call must return promptly and must not
// leave the descendants of kiro-cli running. These tests use syscall.Kill, so
// they live in a unix-only file. The fake kiro-cli modes `orphan` and
// `leftover` (see fakeScript in kirocli_test.go) start `sleep 5 &`, which
// inherits stdout and stderr and therefore holds the output pipes open.

// readChildPID reads the PID the fake kiro-cli wrote to child.pid and
// registers a cleanup that SIGKILLs it, so a red run leaves no sleep process.
func readChildPID(t *testing.T, recDir string) int {
	t.Helper()
	path := filepath.Join(recDir, "child.pid")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fake kiro-cli did not record a child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("child.pid = %q is not an integer: %v", b, err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return pid
}

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// waitGone polls every 25ms for at most 1s for pid to disappear. It must not
// poll longer: a WaitDelay-only fix returns at about 1.3s with the child
// still alive, and the 5s sleep must still be running when the poll ends so
// that fix fails here.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if !processAlive(pid) {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("child process %d is still running 1s after the call returned", pid)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestKiroCLI_AgentTimeoutKillsChild(t *testing.T) {
	rec := installFakeKiro(t, "orphan")
	start := time.Now()
	_, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "a", Prompt: "p", Timeout: 300 * time.Millisecond,
	})
	elapsed := time.Since(start)
	pid := readChildPID(t, rec)

	if elapsed >= timeoutReturnBound {
		t.Errorf("timed-out call took %v, want < %v", elapsed, timeoutReturnBound)
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = false; err = %v", err)
	}
	if want := "kiro-cli timeout after 300ms"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	waitGone(t, pid)
}

func TestKiroCLI_JudgeTimeoutKillsChild(t *testing.T) {
	rec := installFakeKiro(t, "orphan")
	start := time.Now()
	_, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleJudge, Prompt: "p", Timeout: 300 * time.Millisecond,
	})
	elapsed := time.Since(start)
	pid := readChildPID(t, rec)

	if elapsed >= timeoutReturnBound {
		t.Errorf("timed-out call took %v, want < %v", elapsed, timeoutReturnBound)
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = false; err = %v", err)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "kiro-cli chat failed: ") {
		t.Errorf("error wording = %v, want prefix %q", err, "kiro-cli chat failed: ")
	}
	waitGone(t, pid)
}

// A kiro-cli that exits 0 while a descendant still holds the output pipes has
// finished its work. Bounding Wait with WaitDelay must not turn that into a
// failure.
func TestKiroCLI_AgentLeftoverChildNotAFailure(t *testing.T) {
	rec := installFakeKiro(t, "leftover")
	start := time.Now()
	resp, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "a", Prompt: "p",
	})
	elapsed := time.Since(start)
	readChildPID(t, rec) // registers the SIGKILL cleanup for the leftover sleep

	if elapsed >= timeoutReturnBound {
		t.Errorf("call took %v, want < %v", elapsed, timeoutReturnBound)
	}
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q, want %q", resp.Text, "ok")
	}
	if resp.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", resp.ExitCode)
	}
}

func TestKiroCLI_JudgeLeftoverChildNotAFailure(t *testing.T) {
	rec := installFakeKiro(t, "leftover")
	start := time.Now()
	resp, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleJudge, Prompt: "p",
	})
	elapsed := time.Since(start)
	readChildPID(t, rec) // registers the SIGKILL cleanup for the leftover sleep

	if elapsed >= timeoutReturnBound {
		t.Errorf("call took %v, want < %v", elapsed, timeoutReturnBound)
	}
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q, want %q", resp.Text, "ok")
	}
	if resp.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", resp.ExitCode)
	}
}
