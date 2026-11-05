package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// identityPlatform reports whether this OS records a process start time in PID
// markers.
func identityPlatform() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "linux" }

func writeRawMarker(t *testing.T, k pidMarker, issue int, content string) {
	t.Helper()
	if err := os.MkdirAll(k.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(k.path(issue), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A PID marker records the process's start time with its PID, so a PID the OS
// reused for an unrelated process after a crash is not taken for the run.
func TestPIDMarkerIdentity(t *testing.T) {
	for _, k := range []pidMarker{intakeMarkers, agentMarkers} {
		t.Run(filepath.Base(k.dir), func(t *testing.T) {
			t.Run("marker for a live process records its start and is live", func(t *testing.T) {
				t.Chdir(t.TempDir())
				if err := k.write(5, os.Getpid()); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(k.path(5))
				if err != nil {
					t.Fatal(err)
				}
				fields := strings.Fields(string(data))
				if identityPlatform() && len(fields) != 2 {
					t.Errorf("marker = %q, want \"<pid> <start>\" on %s", data, runtime.GOOS)
				}
				if !k.live(5) {
					t.Error("marker for the running test process is not live")
				}
			})

			t.Run("reused PID is stale and removed", func(t *testing.T) {
				if !identityPlatform() {
					t.Skipf("no process start time on %s", runtime.GOOS)
				}
				t.Chdir(t.TempDir())
				writeRawMarker(t, k, 5, strconv.Itoa(os.Getpid())+" 1\n")
				if k.live(5) {
					t.Error("a live PID whose start time differs from the marker must not count")
				}
				if _, err := os.Stat(k.path(5)); !os.IsNotExist(err) {
					t.Error("a stale marker must be removed")
				}
			})

			t.Run("PID-only marker falls back to liveness", func(t *testing.T) {
				t.Chdir(t.TempDir())
				writeRawMarker(t, k, 5, strconv.Itoa(os.Getpid())+"\n")
				if !k.live(5) {
					t.Error("a PID-only marker for a live process must count")
				}
			})
		})
	}
}

// A reused PID does not hold an agent slot.
func TestReusedPIDMarkerDoesNotHoldSlot(t *testing.T) {
	if !identityPlatform() {
		t.Skipf("no process start time on %s", runtime.GOOS)
	}
	h := newCapHarness(t, 1, 3)
	writeRawMarker(t, agentMarkers, 9, strconv.Itoa(os.Getpid())+" 1\n")
	if used, _ := h.m.Slots(); used != 0 {
		t.Errorf("used = %d, want 0 (the marker's PID belongs to another process)", used)
	}
}
