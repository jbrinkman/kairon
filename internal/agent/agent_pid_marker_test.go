package agent

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// An implementation run left by a previous watcher (one that crashed or was
// killed) is known only by its PID marker. While that process is alive it takes
// a slot, so a restarted watcher cannot run more than max_agents agents; a dead
// one does not.
func TestOrphanImplementationMarkerCountsTowardLimit(t *testing.T) {
	const orphan = 9

	t.Run("live marker", func(t *testing.T) {
		h := newCapHarness(t, 1, 3)
		if err := writeAgentPIDMarker(orphan, os.Getpid()); err != nil {
			t.Fatal(err)
		}
		if used, _ := h.m.Slots(); used != 1 {
			t.Errorf("used = %d, want 1 (the orphan run)", used)
		}
		h.put(10)
		h.wantAtCapacity(10)
	})

	t.Run("dead marker", func(t *testing.T) {
		h := newCapHarness(t, 1, 3)
		dead := exec.Command("true")
		if err := dead.Run(); err != nil {
			t.Fatal(err)
		}
		if err := writeAgentPIDMarker(orphan, dead.ProcessState.Pid()); err != nil {
			t.Fatal(err)
		}
		h.put(10)
		h.mustSpawn(10)
		if exists(agentPIDPath(orphan)) {
			t.Error("a marker for a dead process must be removed")
		}
	})

	t.Run("an issue with both markers counts once", func(t *testing.T) {
		h := newCapHarness(t, 2, 3)
		if err := writeAgentPIDMarker(orphan, os.Getpid()); err != nil {
			t.Fatal(err)
		}
		if err := writePIDMarker(orphan, os.Getpid()); err != nil {
			t.Fatal(err)
		}
		if used, _ := h.m.Slots(); used != 1 {
			t.Errorf("used = %d, want 1", used)
		}
	})
}

// A started implementation run records its process, and the marker goes when
// the process exits.
func TestImplementationRunRecordsAndClearsPIDMarker(t *testing.T) {
	h := newCapHarness(t, 1, 3)
	var started *exec.Cmd
	h.m.start = func(cmd *exec.Cmd) error {
		sleep, err := exec.LookPath("sleep")
		if err != nil {
			return err
		}
		cmd.Path, cmd.Args = sleep, []string{"sleep", "30"}
		if err := cmd.Start(); err != nil {
			return err
		}
		started = cmd
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		return nil
	}
	h.put(10)
	a := h.mustSpawn(10)

	data, err := os.ReadFile(agentPIDPath(10))
	if err != nil {
		t.Fatalf("no PID marker after start: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != strconv.Itoa(started.Process.Pid) {
		t.Errorf("marker = %q, want pid %d", got, started.Process.Pid)
	}

	if err := started.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	h.m.waitAgent(a, started)
	if exists(agentPIDPath(10)) {
		t.Error("the PID marker must be removed when the run exits")
	}
}
