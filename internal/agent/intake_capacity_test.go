package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// otherIssue is the issue that competes with testIssue's intake run for a slot.
const otherIssue = 43

// newIntakeSlotHarness is newIntakeHarness with a manager that allows
// maxAgents agents, and krew-lead stubbed so Spawn can take a slot without
// starting a process. otherIssue is stored with a valid stamp, so the real
// boundary admits it.
func newIntakeSlotHarness(t *testing.T, maxAgents, maxRetries int) *intakeHarness {
	t.Helper()
	h := newIntakeHarness(t, maxRetries)
	cfg := &config.Config{Repo: testRepo, Label: testLabel, MaxRetries: maxRetries, MaxAgents: maxAgents}
	h.m = NewManagerWithOptions(cfg, Options{
		GitHub:        h.gh,
		IntakeCommand: h.command,
		RetryDelay:    func(int) time.Duration { return 0 },
	})
	h.stubKrewLead()
	h.gh.Put(testIssue, "Export is slow", handWritten, testLabel)
	h.gh.Put(otherIssue, "Other", provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now()), testLabel)
	return h
}

func (h *intakeHarness) wantSlots(used, max int) {
	h.t.Helper()
	if u, m := h.m.Slots(); u != used || m != max {
		h.t.Fatalf("Slots() = %d/%d, want %d/%d", u, m, used, max)
	}
}

func (h *intakeHarness) wantOtherAtCapacity() {
	h.t.Helper()
	if _, err := h.m.Spawn(otherIssue, testRepo); !errors.Is(err, ErrAtCapacity) {
		h.t.Fatalf("Spawn(#%d) = %v, want ErrAtCapacity", otherIssue, err)
	}
}

func (h *intakeHarness) outcome(a *Agent) IntakeOutcome {
	h.m.mu.RLock()
	defer h.m.mu.RUnlock()
	return a.intake.result.Outcome
}

// An intake run holds an agent slot from its start to its end, whichever way
// it ends, and Spawn for another issue is refused while it is in flight.
func TestIntakeRunHoldsSlot(t *testing.T) {
	cases := []struct {
		name  string
		want  IntakeOutcome
		setup func(h *intakeHarness, release string)
		// finish ends the run the way the case names.
		finish func(h *intakeHarness, release string)
	}{
		{"committed", IntakeCommitted, func(h *intakeHarness, release string) {
			reply := plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none"))
			h.next = func(int, int, string) *exec.Cmd { return plannertest.BlockingCommand(h.dir, reply, release) }
		}, func(_ *intakeHarness, release string) { writeFile(release) }},
		{"punched out", IntakePunchedOut, func(h *intakeHarness, release string) {
			reply := plannertest.PunchOutReply(testRepo, testIssue)
			h.next = func(int, int, string) *exec.Cmd { return plannertest.BlockingCommand(h.dir, reply, release) }
		}, func(_ *intakeHarness, release string) { writeFile(release) }},
		{"stopped", IntakeStopped, func(h *intakeHarness, _ string) {
			h.next = func(int, int, string) *exec.Cmd { return exec.Command("sleep", "30") }
		}, func(h *intakeHarness, _ string) {
			if err := h.m.Stop(h.intakeAgent(testIssue).ID); err != nil {
				h.t.Errorf("Stop: %v", err)
			}
		}},
		{"failed", IntakeFailed, func(h *intakeHarness, release string) {
			reply := plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none"))
			h.next = func(int, int, string) *exec.Cmd { return plannertest.BlockingCommand(h.dir, reply, release) }
			h.gh.Fail["Update"] = errors.New("gh: 502")
		}, func(_ *intakeHarness, release string) { writeFile(release) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newIntakeSlotHarness(t, 1, 1)
			release := filepath.Join(h.dir, "release")
			tc.setup(h, release)

			if _, started, err := h.m.startIntake(testIssue); !started || err != nil {
				t.Fatalf("startIntake = %v, %v", started, err)
			}
			a := h.intakeAgent(testIssue)
			h.wantSlots(1, 1)
			h.wantOtherAtCapacity()

			tc.finish(h, release)
			h.waitIdle(testIssue)

			if got := h.outcome(a); got != tc.want {
				t.Fatalf("outcome = %v, want %v", got, tc.want)
			}
			h.wantSlots(0, 1)
			if _, err := h.m.Spawn(otherIssue, testRepo); err != nil {
				t.Errorf("Spawn(#%d) after the run ended = %v, want a free slot", otherIssue, err)
			}
		})
	}
}

func writeFile(path string) {
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		panic(err)
	}
}

// A failed attempt keeps the run's slot while it waits to retry, and the retry
// attempt keeps it too: no free slot is visible in between.
func TestIntakeAttemptRetryKeepsSlot(t *testing.T) {
	h := newIntakeSlotHarness(t, 1, 1)
	release := filepath.Join(h.dir, "release")
	reply := plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none"))
	h.next = func(_, n int, _ string) *exec.Cmd {
		if n == 0 {
			return plannertest.FailingCommand("not logged in")
		}
		return plannertest.BlockingCommand(h.dir, reply, release)
	}
	inGap, gate := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h.m.intakeDelay = func(int) time.Duration {
		once.Do(func() { close(inGap) })
		<-gate
		return 0
	}

	if _, started, err := h.m.startIntake(testIssue); !started || err != nil {
		t.Fatalf("startIntake = %v, %v", started, err)
	}
	select {
	case <-inGap: // attempt 1 failed; the retry waits at the gate
	case <-time.After(10 * time.Second):
		t.Fatal("attempt 1 did not fail into a retry")
	}
	h.wantSlots(1, 1)
	h.wantOtherAtCapacity()

	close(gate)
	deadline := time.Now().Add(10 * time.Second)
	for len(h.prompts()) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the retry attempt did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.wantSlots(1, 1)
	h.wantOtherAtCapacity()

	writeFile(release)
	h.waitIdle(testIssue)
	h.wantSlots(0, 1)
}

// With no free slot startIntake returns ErrAtCapacity and starts nothing: no
// agent entry, no directory or file, no GitHub read.
func TestIntakeStartAtCapacityStartsNothing(t *testing.T) {
	h := newIntakeSlotHarness(t, 1, 1)
	if _, err := h.m.Spawn(otherIssue, testRepo); err != nil {
		t.Fatalf("Spawn(#%d): %v", otherIssue, err)
	}
	h.wantSlots(1, 1)
	// Keep a run that wrongly starts from touching the disk after the test.
	h.next = func(int, int, string) *exec.Cmd { return exec.Command("true") }
	reads := h.gh.Count("Revision")

	run, started, err := h.m.startIntake(testIssue)

	if !errors.Is(err, ErrAtCapacity) || started || run != nil {
		t.Fatalf("startIntake = %v, %v, %v; want nil, false, ErrAtCapacity", run, started, err)
	}
	for _, a := range h.m.List() {
		if a.IssueNumber == testIssue {
			t.Errorf("agent entry %s for issue #%d left behind", a.ID, testIssue)
		}
	}
	if exists(filepath.Join(".kairon", "intake")) {
		t.Error(".kairon/intake created")
	}
	if exists(intakeLogPath(testIssue)) {
		t.Errorf("intake log %s created", intakeLogPath(testIssue))
	}
	if n := h.gh.Count("Revision") - reads; n != 0 {
		t.Errorf("%d revision reads, want 0", n)
	}
	h.wantSlots(1, 1)
}

// An issue that already holds a slot (a krew-lead retry waiting to start, say)
// has a run going: startIntake reports that and starts nothing.
func TestIntakeStartSlotHeldReportsRunning(t *testing.T) {
	h := newIntakeSlotHarness(t, 2, 1)
	h.m.mu.Lock()
	err := h.m.reserveSlotLocked(testIssue)
	h.m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	h.next = func(int, int, string) *exec.Cmd { return exec.Command("true") }

	run, started, err := h.m.startIntake(testIssue)

	if run != nil || started || err != nil {
		t.Fatalf("startIntake = %v, %v, %v; want nil, false, nil", run, started, err)
	}
	if exists(intakeLogPath(testIssue)) || exists(filepath.Join(".kairon", "intake")) {
		t.Error("a refused start wrote files")
	}
	h.wantSlots(1, 2)
}

// Intake that Spawn's admission starts, when the last slot goes to another run
// first, fails with ErrAtCapacity itself, not as an unavailable admission.
func TestSpawnAdmissionIntakeAtCapacityReturnsErrAtCapacity(t *testing.T) {
	h := newIntakeSlotHarness(t, 1, 1)
	h.next = func(int, int, string) *exec.Cmd { return exec.Command("true") }
	// The table is free for Spawn's early check; another run takes the last
	// slot while the boundary reads the issue.
	var once sync.Once
	h.gh.OnCall = func(op string) {
		if op != "ReadIssue" {
			return
		}
		once.Do(func() {
			h.m.mu.Lock()
			defer h.m.mu.Unlock()
			if err := h.m.reserveSlotLocked(999); err != nil {
				h.t.Errorf("reserve: %v", err)
			}
		})
	}

	_, err := h.m.Spawn(testIssue, testRepo)

	if !errors.Is(err, ErrAtCapacity) || errors.Is(err, ErrAdmissionUnavailable) {
		t.Fatalf("Spawn = %v, want ErrAtCapacity and not ErrAdmissionUnavailable", err)
	}
	for _, a := range h.m.List() {
		if a.IssueNumber == testIssue {
			t.Errorf("agent entry %s for issue #%d left behind", a.ID, testIssue)
		}
	}
	if n := len(h.prompts()); n != 0 {
		t.Errorf("%d intake attempts started, want 0", n)
	}
	if exists(intakeLogPath(testIssue)) {
		t.Error("intake log created")
	}
}

// Validate at capacity reports an error that wraps ErrAtCapacity and starts
// nothing.
func TestValidate_AtCapacityReportsError(t *testing.T) {
	h := newIntakeSlotHarness(t, 1, 1)
	if _, err := h.m.Spawn(otherIssue, testRepo); err != nil {
		t.Fatalf("Spawn(#%d): %v", otherIssue, err)
	}
	h.next = func(int, int, string) *exec.Cmd { return exec.Command("true") }

	res := h.m.Validate(context.Background(), testIssue)

	if res.Status != ValidateError || !errors.Is(res.Err, ErrAtCapacity) {
		t.Fatalf("Validate = %+v, want ValidateError wrapping ErrAtCapacity", res)
	}
	if n := len(h.prompts()); n != 0 {
		t.Errorf("%d intake attempts started, want 0", n)
	}
	if w := h.gh.Writes(); len(w) != 0 {
		t.Errorf("writes = %q, want none", w)
	}
	h.wantSlots(1, 1)
}

// An intake process left by a previous watcher is known only by its PID
// marker. While that process is alive it takes a slot; a dead one does not.
func TestOrphanIntakeMarkerCountsTowardLimit(t *testing.T) {
	const orphan = 77

	t.Run("live marker", func(t *testing.T) {
		h := newIntakeSlotHarness(t, 1, 1)
		if err := writePIDMarker(orphan, os.Getpid()); err != nil {
			t.Fatal(err)
		}
		h.wantSlots(1, 1)
		h.wantOtherAtCapacity()
	})

	t.Run("dead marker", func(t *testing.T) {
		h := newIntakeSlotHarness(t, 1, 1)
		dead := exec.Command("true")
		if err := dead.Run(); err != nil {
			t.Fatal(err)
		}
		if err := writePIDMarker(orphan, dead.ProcessState.Pid()); err != nil {
			t.Fatal(err)
		}
		h.wantSlots(0, 1)
		if _, err := h.m.Spawn(otherIssue, testRepo); err != nil {
			t.Errorf("Spawn(#%d) = %v, want a free slot", otherIssue, err)
		}
	})

	t.Run("marker file names that are not an issue are ignored", func(t *testing.T) {
		h := newIntakeSlotHarness(t, 1, 1)
		for _, name := range []string{"issue-x.pid", "issue-.pid", "notes.pid"} {
			if err := os.MkdirAll(filepath.Join(".kairon", "intake"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(".kairon", "intake", name), []byte("1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		h.wantSlots(0, 1)
	})

	// The run's own marker names the live process of an issue that already
	// holds a slot. It is one agent, not two.
	t.Run("a marker for an issue that holds a slot counts once", func(t *testing.T) {
		h := newIntakeSlotHarness(t, 2, 1)
		release := filepath.Join(h.dir, "release")
		reply := plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none"))
		h.next = func(int, int, string) *exec.Cmd { return plannertest.BlockingCommand(h.dir, reply, release) }

		if _, started, err := h.m.startIntake(testIssue); !started || err != nil {
			t.Fatalf("startIntake = %v, %v", started, err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for !exists(intakePIDPath(testIssue)) {
			if time.Now().After(deadline) {
				t.Fatal("the run wrote no PID marker")
			}
			time.Sleep(5 * time.Millisecond)
		}
		h.wantSlots(1, 2)

		writeFile(release)
		h.waitIdle(testIssue)
		h.wantSlots(0, 2)
	})
}
