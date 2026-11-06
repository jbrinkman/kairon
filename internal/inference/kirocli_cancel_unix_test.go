//go:build unix

package inference

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Issue #490: cancelling the parent context of an in-flight kiro-cli call must
// return promptly and must kill the kiro-cli process group, including a
// descendant that holds the output pipes. These are characterization tests:
// setProcessGroup already behaves this way, so they pass on first run. They
// differ from the timeout tests in kirocli_timeout_unix_test.go in that
// Request.Timeout is one minute, so only the parent cancellation can end the
// call.

// cancelRequestTimeout is long enough that Request.Timeout cannot fire during
// a test.
const cancelRequestTimeout = time.Minute

// invokeThenCancel runs backend.Invoke with a cancellable parent context. It
// cancels the parent once the fake kiro-cli has recorded its child PID, and
// returns the time from the cancel to the return, the child PID and the error.
func invokeThenCancel(t *testing.T, rec string, req Request) (sinceCancel time.Duration, pid int, err error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		err error
		at  time.Time
	}
	done := make(chan result, 1)
	go func() {
		_, err := newKiro(t).Invoke(ctx, req)
		done <- result{err: err, at: time.Now()}
	}()

	pidFile := filepath.Join(rec, "child.pid")
	ready := time.Now().Add(5 * time.Second)
	for {
		// The fake writes the PID with `echo $! > child.pid`, so require a
		// non-empty file to avoid reading it between create and write.
		if b, readErr := os.ReadFile(pidFile); readErr == nil && len(b) > 0 {
			break
		}
		if time.Now().After(ready) {
			cancel()
			t.Fatal("fake kiro-cli did not record a child pid within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancelledAt := time.Now()
	cancel()

	select {
	case r := <-done:
		return r.at.Sub(cancelledAt), readChildPID(t, rec), r.err
	case <-time.After(10 * time.Second):
		t.Fatal("Invoke did not return within 10s of the parent cancel")
		return 0, 0, nil
	}
}

func assertCancelled(t *testing.T, sinceCancel time.Duration, pid int, err error) {
	t.Helper()
	if sinceCancel >= timeoutReturnBound {
		t.Errorf("call returned %v after the cancel, want < %v", sinceCancel, timeoutReturnBound)
	}
	if err == nil {
		t.Error("err = nil, want a non-nil error after the parent was cancelled")
	}
	if errors.Is(err, ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = true; a parent cancel must not report a timeout; err = %v", err)
	}
	waitGone(t, pid)
}

func TestKiroCLI_AgentParentCancelKillsChild(t *testing.T) {
	rec := installFakeKiro(t, "orphan")
	sinceCancel, pid, err := invokeThenCancel(t, rec, Request{
		Role: RoleAgent, Agent: "a", Prompt: "p", Timeout: cancelRequestTimeout,
	})
	assertCancelled(t, sinceCancel, pid, err)
}

func TestKiroCLI_JudgeParentCancelKillsChild(t *testing.T) {
	rec := installFakeKiro(t, "orphan")
	sinceCancel, pid, err := invokeThenCancel(t, rec, Request{
		Role: RoleJudge, Prompt: "p", Timeout: cancelRequestTimeout,
	})
	assertCancelled(t, sinceCancel, pid, err)
}
