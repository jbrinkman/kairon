//go:build unix

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// installBlockingFakeKiro puts a fake `kiro-cli` first on PATH and returns the
// files it uses to report progress.
//
// The script answers `--version` with exit 0. For any other invocation it
// drains stdin, starts a background `sleep 300`, writes its own PID (line 1)
// and the background sleep PID (line 2) to pidFile, touches readyFile, and then
// execs a foreground `sleep 300`, so the first PID stays valid and no process
// is left unrecorded. A call therefore never finishes on its own, so a test
// can prove that cancelling it kills the whole process group. A t.Cleanup
// SIGKILLs both recorded PIDs, so a red run leaves nothing behind.
func installBlockingFakeKiro(t *testing.T) (readyFile, pidFile string) {
	t.Helper()
	dir := t.TempDir()
	readyFile = filepath.Join(dir, "ready")
	pidFile = filepath.Join(dir, "pids")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  exit 0
fi
cat >/dev/null
sleep 300 &
printf '%%s\n%%s\n' "$$" "$!" > %[1]q.tmp
mv %[1]q.tmp %[1]q
: > %[2]q
exec sleep 300
`, pidFile, readyFile)
	if err := os.WriteFile(filepath.Join(dir, "kiro-cli"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kiro-cli: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		for _, f := range strings.Fields(string(b)) {
			if pid, err := strconv.Atoi(f); err == nil && pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	return readyFile, pidFile
}

// waitForFile polls for path to exist, for at most 5 seconds.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after 5s waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// processAlive reports whether pid names a running process (signal 0).
func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }
