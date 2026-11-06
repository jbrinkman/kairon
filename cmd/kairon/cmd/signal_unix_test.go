//go:build unix

package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const signalNoticeText = "kairon: signal received, cancelling in-flight calls (send it again to exit immediately)\n"

// lockedBuffer is a bytes.Buffer safe for the notice goroutine to write while
// the test reads.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// guardSignals registers a signal.Notify channel for sig before the test sends
// that signal to itself. If signalContext does not handle the signal, the
// guard still does, so a red run fails an assertion and the go test process is
// not killed.
func guardSignals(t *testing.T, sigs ...os.Signal) {
	t.Helper()
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, sigs...)
	t.Cleanup(func() { signal.Stop(ch) })
}

func waitDone(t *testing.T, ctx context.Context) bool {
	t.Helper()
	select {
	case <-ctx.Done():
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

// waitNotice polls up to 2s for the notice goroutine to write its line.
func waitNotice(buf *lockedBuffer) string {
	deadline := time.Now().Add(2 * time.Second)
	for buf.String() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return buf.String()
}

func testSignalCancels(t *testing.T, sig syscall.Signal) {
	guardSignals(t, syscall.SIGINT, syscall.SIGTERM)
	var buf lockedBuffer
	ctx, cancel := signalContext(context.Background(), &buf)
	defer cancel()

	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatalf("send %v: %v", sig, err)
	}
	if !waitDone(t, ctx) {
		t.Fatalf("context not cancelled within 2s of %v", sig)
	}
	if got := waitNotice(&buf); got != signalNoticeText {
		t.Errorf("notice = %q, want %q", got, signalNoticeText)
	}
	// Give a second (wrong) notice time to appear, then check there is one.
	time.Sleep(50 * time.Millisecond)
	if got := buf.String(); strings.Count(got, "\n") != 1 {
		t.Errorf("want exactly one notice line, got %q", got)
	}
}

func TestSignalContext_SIGINTCancels(t *testing.T)  { testSignalCancels(t, syscall.SIGINT) }
func TestSignalContext_SIGTERMCancels(t *testing.T) { testSignalCancels(t, syscall.SIGTERM) }

func TestSignalContext_ParentCancelDoesNotNotify(t *testing.T) {
	var buf lockedBuffer
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, cancel := signalContext(parent, &buf)
	defer cancel()

	cancelParent()
	if !waitDone(t, ctx) {
		t.Fatal("cancelling the parent did not cancel the signal context")
	}
	time.Sleep(100 * time.Millisecond)
	if got := buf.String(); got != "" {
		t.Errorf("parent cancellation wrote a notice: %q", got)
	}
}

func TestSignalContext_CancelIsIdempotentAndQuiet(t *testing.T) {
	var buf lockedBuffer
	ctx, cancel := signalContext(context.Background(), &buf)

	cancel()
	cancel()
	if !waitDone(t, ctx) {
		t.Fatal("cancel did not cancel the context")
	}
	time.Sleep(100 * time.Millisecond)
	if got := buf.String(); got != "" {
		t.Errorf("normal release wrote a notice: %q", got)
	}
}

func TestSignalContext_NilParent(t *testing.T) {
	var buf lockedBuffer
	//nolint:staticcheck // a nil parent is the case under test.
	ctx, cancel := signalContext(nil, &buf)
	if ctx == nil {
		t.Fatal("signalContext(nil, ...) returned a nil context")
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("fresh context Err() = %v, want nil", err)
	}
	cancel()
	if !waitDone(t, ctx) {
		t.Error("cancel did not cancel the context built from a nil parent")
	}
}

// TestSignalContext_SecondSignalTerminates proves the handler is released after
// the first signal. The scenario runs in a child copy of the test binary so
// that the second signal can actually kill a process.
func TestSignalContext_SecondSignalTerminates(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^TestSignalContextHelperProcess$")
	c.Env = append(os.Environ(), "KAIRON_SIGNAL_CONTEXT_HELPER=1")
	var stdout, stderr lockedBuffer
	c.Stdout, c.Stderr = &stdout, &stderr
	if err := c.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	t.Cleanup(func() { _ = c.Process.Kill() })

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("helper process still running after 10s\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}

	if !strings.Contains(stdout.String(), "first-signal-cancelled") {
		t.Errorf("first signal did not cancel the context\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), strings.TrimSuffix(signalNoticeText, "\n")) {
		t.Errorf("helper stderr lacks the notice: %q", stderr.String())
	}
	ws, ok := c.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGINT {
		t.Errorf("second SIGINT did not terminate the helper with default handling; state = %v", c.ProcessState)
	}
	if strings.Contains(stdout.String(), "survived-second-signal") {
		t.Error("helper survived the second signal")
	}
}

// TestSignalContextHelperProcess is the child side of
// TestSignalContext_SecondSignalTerminates. It does nothing in a normal run.
func TestSignalContextHelperProcess(t *testing.T) {
	if os.Getenv("KAIRON_SIGNAL_CONTEXT_HELPER") != "1" {
		t.Skip("helper process for TestSignalContext_SecondSignalTerminates")
	}
	ctx, cancel := signalContext(nil, os.Stderr)
	defer cancel()
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	select {
	case <-ctx.Done():
		os.Stdout.WriteString("first-signal-cancelled\n")
	case <-time.After(2 * time.Second):
		os.Stdout.WriteString("first-signal-ignored\n")
		os.Exit(3)
	}
	// Let the release goroutine finish: it writes the notice, then restores
	// default handling.
	time.Sleep(300 * time.Millisecond)
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	time.Sleep(3 * time.Second)
	os.Stdout.WriteString("survived-second-signal\n")
	os.Exit(0)
}
