package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/boundary"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/github/githubtest"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
	"github.com/jbrinkman/kairon/internal/provenance"
)

const handWritten = "## Problem\n\nExport is slow.\n\nMake it faster please."

// intakeHarness runs the real boundary, intake, commit and failure code over a
// githubtest.Fake. Each intake attempt is a real `sh` process that prints the
// reply the test chose, so nothing reaches gh, git or kiro-cli.
type intakeHarness struct {
	t    *testing.T
	m    *Manager
	gh   *githubtest.Fake
	dir  string
	mu   sync.Mutex
	cmds []string // the prompt of every attempt, in order
	// next returns the command for the n-th attempt (0-based) of issue.
	next func(issue, n int, prompt string) *exec.Cmd
}

func newIntakeHarness(t *testing.T, maxRetries int) *intakeHarness {
	t.Helper()
	t.Chdir(t.TempDir())
	h := &intakeHarness{t: t, gh: githubtest.New(), dir: t.TempDir()}
	cfg := &config.Config{Repo: testRepo, Label: testLabel, MaxRetries: maxRetries}
	h.m = NewManagerWithOptions(cfg, Options{
		GitHub:        h.gh,
		IntakeCommand: h.command,
		RetryDelay:    func(int) time.Duration { return 0 },
	})
	h.replyAlways(plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none")))
	return h
}

func (h *intakeHarness) command(prompt string) *exec.Cmd {
	h.mu.Lock()
	issue := 0
	fmt.Sscanf(prompt[strings.Index(prompt, testRepo+"#")+len(testRepo)+1:], "%d", &issue)
	n := 0
	for _, p := range h.cmds {
		if strings.Contains(p, fmt.Sprintf("%s#%d.", testRepo, issue)) {
			n++
		}
	}
	h.cmds = append(h.cmds, prompt)
	next := h.next
	h.mu.Unlock()
	return next(issue, n, prompt)
}

func (h *intakeHarness) replyAlways(reply string) {
	h.next = func(int, int, string) *exec.Cmd { return plannertest.ReplyCommand(h.dir, reply) }
}

func (h *intakeHarness) prompts() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.cmds)
}

// waitIdle waits until no intake run for issue is going.
func (h *intakeHarness) waitIdle(issue int) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for h.m.IntakeActive(issue) {
		if time.Now().After(deadline) {
			h.t.Fatalf("intake for issue #%d did not end", issue)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *intakeHarness) intakeAgent(issue int) *Agent {
	h.t.Helper()
	for _, a := range h.m.List() {
		if a.IssueNumber == issue && a.Kind == KindIntake {
			return a
		}
	}
	h.t.Fatalf("no intake run for issue #%d", issue)
	return nil
}

func (h *intakeHarness) admit(issue int) boundary.Result {
	h.t.Helper()
	res, err := boundary.Boundary{Reader: h.gh, Label: testLabel}.Admit(testRepo, issue)
	if err != nil {
		h.t.Fatalf("Admit: %v", err)
	}
	return res
}

// AC 6: validate stamps an unmarked issue, and the stored body is then admitted.
func TestValidate_StampsUnmarkedIssueSoItIsAdmitted(t *testing.T) {
	h := newIntakeHarness(t, 1)
	h.gh.Put(testIssue, "Export is slow", handWritten, testLabel)

	res := h.m.Validate(context.Background(), testIssue)

	if res.Status != ValidateFinished || res.Intake.Outcome != IntakeCommitted {
		t.Fatalf("Validate = %+v, want a committed run", res)
	}
	if got := provenance.Classify(h.gh.Get(testIssue).Body); got != provenance.StateValid {
		t.Fatalf("stored body classifies as %v, want valid", got)
	}
	if adm := h.admit(testIssue); adm.Outcome != boundary.OutcomeAdmitted {
		t.Errorf("Admit after validate = %+v, want admitted", adm)
	}
	if res.Intake.Hash == "" || res.Intake.Hash != provenance.Hash(h.gh.Get(testIssue).Body) {
		t.Errorf("Hash = %q, want the stored body's hash", res.Intake.Hash)
	}
	if got := h.prompts(); len(got) != 1 || !strings.Contains(got[0], fmt.Sprintf("%s#%d", testRepo, testIssue)) {
		t.Errorf("prompts = %q, want one naming the issue", got)
	}
	if !exists(intakeLogPath(testIssue)) {
		t.Errorf("intake log %s not written", intakeLogPath(testIssue))
	}
	if exists(intakePIDPath(testIssue)) {
		t.Error("PID marker left behind after the run ended")
	}
}

// AC 8: on a commit, validate removes the fail label after the body update.
func TestValidate_CommitRemovesFailedLabelAfterUpdate(t *testing.T) {
	h := newIntakeHarness(t, 1)
	failed := failure.FailedLabel(testLabel)
	h.gh.Put(testIssue, "Export is slow", handWritten, testLabel, failed)

	res := h.m.Validate(context.Background(), testIssue)

	if res.Intake.Outcome != IntakeCommitted || !res.HadFailedLabel || !res.LabelRemoved || res.LabelErr != nil {
		t.Fatalf("Validate = %+v, want committed with the fail label removed", res)
	}
	writes := h.gh.Writes()
	upd := slices.Index(writes, fmt.Sprintf("Update #%d", testIssue))
	rm := slices.Index(writes, fmt.Sprintf("RemoveLabel #%d %s", testIssue, failed))
	if upd < 0 || rm < 0 || rm < upd || h.gh.Count("RemoveLabel") != 1 {
		t.Errorf("writes = %q, want one update then one label removal", writes)
	}
	if slices.Contains(h.gh.Get(testIssue).Labels, failed) {
		t.Error("fail label still present")
	}
}

// A punch-out that posted its comment but not its label leaves the issue
// terminal-pending. A commit by validate releases it, so the watcher admits the
// stamped issue instead of finishing the old punch-out.
func TestValidate_CommitReleasesTerminalPendingIssue(t *testing.T) {
	h := newIntakeHarness(t, 1)
	failed := failure.FailedLabel(testLabel)
	h.gh.Put(testIssue, "Export is slow", handWritten, testLabel)
	if err := h.gh.PostComment(testRepo, testIssue, failure.Marker(failure.NewRunID())+"\n\nPlanning failed."); err != nil {
		t.Fatal(err)
	}

	res := h.m.Validate(context.Background(), testIssue)

	if res.Intake.Outcome != IntakeCommitted || !res.LabelRemoved || res.LabelErr != nil {
		t.Fatalf("Validate = %+v, want committed with the issue released", res)
	}
	st, err := h.gh.State(testRepo, testIssue, failed)
	if err != nil {
		t.Fatal(err)
	}
	if sel := st.Classify(failure.DoneLabel(testLabel), failed); sel != failure.SelectEligible {
		t.Errorf("selection = %v, want eligible", sel)
	}
	if got := h.admit(testIssue); got.Outcome != boundary.OutcomeAdmitted {
		t.Errorf("admit = %+v, want admitted", got)
	}
}

// AC 8: punch-out, changed-during-run and error outcomes keep the fail label.
func TestValidate_OtherOutcomesKeepFailedLabel(t *testing.T) {
	failed := failure.FailedLabel(testLabel)
	cases := []struct {
		name  string
		setup func(h *intakeHarness)
		want  IntakeOutcome
	}{
		{"planner punch_out", func(h *intakeHarness) {
			h.replyAlways(plannertest.PunchOutReply(testRepo, testIssue))
		}, IntakePunchedOut},
		{"edited during the run", func(h *intakeHarness) {
			reply := plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none"))
			h.next = func(int, int, string) *exec.Cmd {
				h.gh.Edit(testIssue, handWritten+"\n\nA human edit.")
				return plannertest.ReplyCommand(h.dir, reply)
			}
		}, IntakeChanged},
		{"body write fails", func(h *intakeHarness) {
			h.gh.Fail["Update"] = errors.New("gh: 502")
		}, IntakeFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newIntakeHarness(t, 1)
			h.gh.Put(testIssue, "Export is slow", handWritten, testLabel, failed)
			tc.setup(h)

			res := h.m.Validate(context.Background(), testIssue)

			if res.Status != ValidateFinished || res.Intake.Outcome != tc.want {
				t.Fatalf("Validate = %+v, want outcome %v", res, tc.want)
			}
			if h.gh.Count("RemoveLabel") != 0 || res.LabelRemoved {
				t.Errorf("writes = %q, want no label removal", h.gh.Writes())
			}
			if !slices.Contains(h.gh.Get(testIssue).Labels, failed) {
				t.Error("fail label was removed")
			}
		})
	}
}

// AC 9: a valid, closed or done issue starts no run and writes nothing.
func TestValidate_RefusalsStartNothing(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(h *intakeHarness)
		status ValidateStatus
	}{
		{"already valid", func(h *intakeHarness) {
			h.gh.Put(testIssue, "t", provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now()), testLabel)
		}, ValidateAlreadyValid},
		{"closed", func(h *intakeHarness) {
			h.gh.Put(testIssue, "t", handWritten, testLabel)
			h.gh.Close(testIssue)
		}, ValidateRefused},
		{"done", func(h *intakeHarness) {
			h.gh.Put(testIssue, "t", handWritten, testLabel, failure.DoneLabel(testLabel))
		}, ValidateRefused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newIntakeHarness(t, 1)
			tc.setup(h)

			res := h.m.Validate(context.Background(), testIssue)

			if res.Status != tc.status {
				t.Fatalf("Validate = %+v, want status %v", res, tc.status)
			}
			if n := len(h.prompts()); n != 0 {
				t.Errorf("%d intake attempts started, want 0", n)
			}
			if w := h.gh.Writes(); len(w) != 0 {
				t.Errorf("writes = %q, want none", w)
			}
		})
	}
}

// A valid issue that carries the fail label keeps it: the label may be there
// for a krew-lead failure.
func TestValidate_AlreadyValidLeavesFailedLabel(t *testing.T) {
	h := newIntakeHarness(t, 1)
	failed := failure.FailedLabel(testLabel)
	h.gh.Put(testIssue, "t", provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now()), testLabel, failed)

	res := h.m.Validate(context.Background(), testIssue)

	if res.Status != ValidateAlreadyValid || !res.HadFailedLabel || res.Hash == "" {
		t.Fatalf("Validate = %+v, want already valid with the fail label noted", res)
	}
	if h.gh.Count("RemoveLabel") != 0 {
		t.Error("validate removed the fail label from an already-valid issue")
	}
}

// AC 7: two issues validate at once with separate logs; a second request for
// an issue whose intake is running reports it and starts nothing.
func TestValidate_ParallelRunsKeepSeparateLogs(t *testing.T) {
	h := newIntakeHarness(t, 1)
	release := filepath.Join(h.dir, "release")
	h.next = func(issue, _ int, _ string) *exec.Cmd {
		reply := plannertest.CanonicalReply(testRepo, issue, plannertest.ValidBody("Dependencies: none"))
		cmd := plannertest.BlockingCommand(h.dir, reply, release)
		cmd.Args[2] = fmt.Sprintf(`echo "progress for issue %d" >&2; `, issue) + cmd.Args[2]
		return cmd
	}
	for _, n := range []int{41, 42} {
		h.gh.Put(n, "t", handWritten, testLabel)
	}

	var wg sync.WaitGroup
	results := make([]ValidateResult, 2)
	for i, n := range []int{41, 42} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = h.m.Validate(context.Background(), n)
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for !(h.m.IntakeActive(41) && h.m.IntakeActive(42)) {
		if time.Now().After(deadline) {
			t.Fatal("both runs did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.m.Validate(context.Background(), 42); got.Status != ValidateAlreadyRunning {
		t.Errorf("second validate = %+v, want already running", got)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	for i, n := range []int{41, 42} {
		if results[i].Intake.Outcome != IntakeCommitted {
			t.Errorf("issue #%d: %+v, want committed", n, results[i])
		}
		data, err := os.ReadFile(intakeLogPath(n))
		if err != nil {
			t.Fatal(err)
		}
		other := 83 - n
		if !strings.Contains(string(data), fmt.Sprintf("progress for issue %d", n)) ||
			strings.Contains(string(data), fmt.Sprintf("progress for issue %d", other)) {
			t.Errorf("log for #%d = %q, want only its own output", n, data)
		}
	}
	if n := len(h.prompts()); n != 2 {
		t.Errorf("%d attempts, want 2 (the overlapping request started none)", n)
	}
}

// AC 11: rejected replies are retried with the previous failure in the prompt,
// and running out of attempts punches the issue out once.
func TestIntake_RetriesWithPreviousFailureThenPunchesOut(t *testing.T) {
	h := newIntakeHarness(t, 2)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.replyAlways(plannertest.CanonicalReply(testRepo, testIssue, plannertest.RejectedBody()))

	res := h.m.Validate(context.Background(), testIssue)

	if res.Intake.Outcome != IntakePunchedOut || res.Intake.Attempts != 3 {
		t.Fatalf("Validate = %+v, want punched out after 3 attempts", res)
	}
	prompts := h.prompts()
	if len(prompts) != 3 {
		t.Fatalf("%d attempts, want max_retries + 1 = 3", len(prompts))
	}
	if strings.Contains(prompts[0], "rejected") {
		t.Errorf("first prompt mentions a rejection:\n%s", prompts[0])
	}
	for i, p := range prompts[1:] {
		if !strings.Contains(p, "rejected by the "+res.Intake.Failure.Guard+" check") {
			t.Errorf("retry prompt %d does not carry the previous failure:\n%s", i+2, p)
		}
		if !strings.Contains(p, fmt.Sprintf("attempt %d of 3", i+2)) {
			t.Errorf("retry prompt %d does not name its attempt:\n%s", i+2, p)
		}
	}
	want := []string{fmt.Sprintf("PostComment #%d", testIssue), fmt.Sprintf("AddLabel #%d %s", testIssue, failure.FailedLabel(testLabel))}
	if got := h.gh.Writes(); !slices.Equal(got, want) {
		t.Errorf("writes = %q, want %q", got, want)
	}
	if c := h.gh.Get(testIssue).Comments; len(c) != 1 || !strings.Contains(c[0].Body, fmt.Sprintf("`validate %d`", testIssue)) {
		t.Errorf("comment = %+v, want one naming `validate %d`", c, testIssue)
	}
}

// A process that fails (kiro-cli missing or not logged in) is retried as an
// invoke failure, then punched out with that error.
func TestIntake_InvokeFailureIsRetriedThenPunchedOut(t *testing.T) {
	h := newIntakeHarness(t, 1)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.next = func(int, int, string) *exec.Cmd { return plannertest.FailingCommand("not logged in") }

	res := h.m.Validate(context.Background(), testIssue)

	if res.Intake.Outcome != IntakePunchedOut || res.Intake.Failure.Guard != "invoke" {
		t.Fatalf("Validate = %+v, want punched out on the invoke guard", res)
	}
	if n := len(h.prompts()); n != 2 {
		t.Errorf("%d attempts, want 2", n)
	}
	data, _ := os.ReadFile(intakeLogPath(testIssue))
	if !strings.Contains(string(data), "not logged in") {
		t.Errorf("intake log = %q, want the process's stderr", data)
	}
}

// AC 12: stop ends an intake run with no retry, no write and no punch-out.
func TestIntake_StopEndsWithoutRetryOrWrite(t *testing.T) {
	h := newIntakeHarness(t, 3)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.next = func(int, int, string) *exec.Cmd { return exec.Command("sleep", "30") }

	if _, started, err := h.m.startIntake(testIssue); !started || err != nil {
		t.Fatalf("startIntake = %v, %v", started, err)
	}
	a := h.intakeAgent(testIssue)
	if err := h.m.Stop(a.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	h.waitIdle(testIssue)

	if got := a.intake.result.Outcome; got != IntakeStopped {
		t.Errorf("outcome = %v, want stopped", got)
	}
	if n := len(h.prompts()); n != 1 {
		t.Errorf("%d attempts, want 1 (no retry after stop)", n)
	}
	if w := h.gh.Writes(); len(w) != 0 {
		t.Errorf("writes = %q, want none", w)
	}
}

// A stop that lands while the commit reads the revision prevents the body
// write that would follow it.
func TestIntake_StopDuringCommitWritesNothing(t *testing.T) {
	h := newIntakeHarness(t, 1)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	revisions := 0
	h.gh.OnCall = func(op string) {
		if op != "Revision" {
			return
		}
		// The first read is the run's own; the second is the commit's check.
		if revisions++; revisions == 2 {
			if err := h.m.Stop(h.intakeAgent(testIssue).ID); err != nil {
				t.Errorf("Stop: %v", err)
			}
		}
	}

	res := h.m.Validate(context.Background(), testIssue)

	if res.Intake.Outcome != IntakeStopped {
		t.Fatalf("Validate = %+v, want stopped", res)
	}
	if w := h.gh.Writes(); len(w) != 0 {
		t.Errorf("writes = %q, want none", w)
	}
}

// A stop that lands while the final punch-out reads the issue prevents the
// comment and the label.
func TestIntake_StopDuringPunchOutWritesNothing(t *testing.T) {
	h := newIntakeHarness(t, 0)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.replyAlways(plannertest.CanonicalReply(testRepo, testIssue, plannertest.RejectedBody()))
	h.gh.OnCall = func(op string) {
		if op == "State" {
			if err := h.m.Stop(h.intakeAgent(testIssue).ID); err != nil {
				t.Errorf("Stop: %v", err)
			}
		}
	}

	res := h.m.Validate(context.Background(), testIssue)

	if res.Intake.Outcome != IntakeStopped {
		t.Fatalf("Validate = %+v, want stopped", res)
	}
	if w := h.gh.Writes(); len(w) != 0 {
		t.Errorf("writes = %q, want none", w)
	}
}

// The watcher's intake reports a running run, starts at most one, and refuses
// to loop after a commit.
func TestWatcherIntake_RunningSpentAndStarted(t *testing.T) {
	h := newIntakeHarness(t, 1)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	release := filepath.Join(h.dir, "release")
	reply := plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none"))
	h.next = func(int, int, string) *exec.Cmd { return plannertest.BlockingCommand(h.dir, reply, release) }
	in := watcherIntake{h.m}

	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeStarted || err != nil {
		t.Fatalf("first Start = %v, %v; want started", st, err)
	}
	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeRunning || err != nil {
		t.Fatalf("second Start = %v, %v; want running", st, err)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.waitIdle(testIssue)
	if !h.m.hasCommitted(testIssue) {
		t.Fatal("no commit record after a committed run")
	}
	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeSpent || err != nil {
		t.Errorf("Start after a commit = %v, %v; want spent", st, err)
	}
	if n := len(h.prompts()); n != 1 {
		t.Errorf("%d attempts, want 1", n)
	}
}

// A live PID marker from a previous process counts as a running intake; a
// dead one is stale and removed.
func TestIntakeActive_PIDMarker(t *testing.T) {
	h := newIntakeHarness(t, 1)
	if err := writePIDMarker(testIssue, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if !h.m.IntakeActive(testIssue) || h.m.OwnsIntake(testIssue) {
		t.Error("a live marker should be active but not owned")
	}

	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	if err := writePIDMarker(testIssue, dead.ProcessState.Pid()); err != nil {
		t.Fatal(err)
	}
	if h.m.IntakeActive(testIssue) {
		t.Error("a dead marker counts as active")
	}
	if exists(intakePIDPath(testIssue)) {
		t.Error("stale marker not removed")
	}
}

// A failed revision read means intake could not start: an error, nothing written.
func TestWatcherIntake_StartErrorWritesNothing(t *testing.T) {
	h := newIntakeHarness(t, 1)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.gh.Fail["Revision"] = errors.New("gh: rate limited")

	if _, err := (watcherIntake{h.m}).Start(testRepo, testIssue); err == nil {
		t.Fatal("Start succeeded without a revision")
	}
	if h.m.IntakeActive(testIssue) || len(h.prompts()) != 0 || len(h.gh.Writes()) != 0 {
		t.Errorf("active = %v, attempts = %d, writes = %q; want nothing", h.m.IntakeActive(testIssue), len(h.prompts()), h.gh.Writes())
	}
}

// Spawn on an issue whose intake is running admits nothing and reads nothing.
func TestSpawn_IntakeRunningIsPending(t *testing.T) {
	h := newIntakeHarness(t, 1)
	if err := writePIDMarker(testIssue, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Spawn(testIssue, testRepo); !errors.Is(err, ErrIntakePending) {
		t.Fatalf("Spawn err = %v, want ErrIntakePending", err)
	}
	if n := h.gh.Count("ReadIssue"); n != 0 {
		t.Errorf("Spawn read the issue %d times, want 0", n)
	}
}

// A punch-out that fails before its comment keeps the commit record, so the
// next poll punches out again rather than starting intake on an issue intake
// already committed.
func TestSpawn_FailedPunchOutKeepsCommitRecord(t *testing.T) {
	h := newIntakeHarness(t, 1)
	stamped := provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now())
	h.gh.Put(testIssue, "t", stamped+"\n\nEdited after the commit.", testLabel)
	if err := writeCommitted(testIssue, "hash"); err != nil {
		t.Fatal(err)
	}
	h.gh.Fail["PostComment"] = errors.New("gh: rate limited")

	if _, err := h.m.Spawn(testIssue, testRepo); !errors.Is(err, ErrNotAdmitted) {
		t.Fatalf("Spawn = %v, want not admitted", err)
	}
	if !h.m.hasCommitted(testIssue) {
		t.Fatal("commit record cleared by a punch-out that failed")
	}

	delete(h.gh.Fail, "PostComment")
	if _, err := h.m.Spawn(testIssue, testRepo); !errors.Is(err, ErrNotAdmitted) {
		t.Fatalf("second Spawn = %v, want not admitted", err)
	}
	if n := len(h.prompts()); n != 0 {
		t.Errorf("%d intake attempts, want none", n)
	}
	if h.m.hasCommitted(testIssue) {
		t.Error("commit record kept after the punch-out succeeded")
	}
	if c := h.gh.Get(testIssue).Comments; len(c) != 1 {
		t.Errorf("%d comments, want 1", len(c))
	}
}

// stubKrewLead replaces krew-lead's worktree, process start and monitor, and
// counts the starts.
func (h *intakeHarness) stubKrewLead() *int {
	starts := 0
	h.m.createWorktree = func(name string) (string, error) { return filepath.Join(h.dir, name), nil }
	h.m.removeWorktree = func(int, int) error { return nil }
	h.m.start = func(*exec.Cmd) error { starts++; return nil }
	h.m.monitor = func(*Agent, *exec.Cmd) {}
	return &starts
}

// startIntakeOnRead makes the boundary's issue read look like a validate run
// starting between admission and krew-lead's start.
func (h *intakeHarness) startIntakeOnRead() {
	h.gh.OnCall = func(op string) {
		if op == "ReadIssue" {
			if err := writePIDMarker(testIssue, os.Getpid()); err != nil {
				h.t.Error(err)
			}
		}
	}
}

// An intake run that starts while Spawn admits the issue keeps krew-lead from
// starting on the document read before it.
func TestSpawn_IntakeStartedDuringAdmissionIsPending(t *testing.T) {
	h := newIntakeHarness(t, 1)
	starts := h.stubKrewLead()
	h.gh.Put(testIssue, "t", provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now()), testLabel)
	h.startIntakeOnRead()

	if _, err := h.m.Spawn(testIssue, testRepo); !errors.Is(err, ErrIntakePending) {
		t.Fatalf("Spawn err = %v, want ErrIntakePending", err)
	}
	if *starts != 0 {
		t.Errorf("krew-lead started %d times, want 0", *starts)
	}
	if exists(inputDocumentPath(testIssue)) {
		t.Error("input document left behind")
	}
}

// The same holds for a krew-lead retry, which re-admits the issue first.
func TestRetry_IntakeStartedDuringAdmissionStartsNothing(t *testing.T) {
	h := newIntakeHarness(t, 1)
	starts := h.stubKrewLead()
	untracked := 0
	h.m.untrack = func(int) { untracked++ }
	h.gh.Put(testIssue, "t", provenance.Mark(plannertest.ValidBody("Dependencies: none"), time.Now()), testLabel)
	a := &Agent{ID: "agent-retry", IssueNumber: testIssue, Kind: KindImplementation, Status: StatusFailed}
	h.m.agents[a.ID] = a
	h.startIntakeOnRead()

	h.m.retryAgent(a)

	if *starts != 0 {
		t.Errorf("krew-lead started %d times, want 0", *starts)
	}
	if a.Status != StatusFailed || untracked != 1 {
		t.Errorf("status = %v, untracked = %d; want failed and handed back once", a.Status, untracked)
	}
}

// A human stop of a watcher intake run is final: the next poll punches the
// issue out instead of starting intake again.
func TestWatcherIntake_StopIsFinal(t *testing.T) {
	h := newIntakeHarness(t, 3)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.next = func(int, int, string) *exec.Cmd { return exec.Command("sleep", "30") }
	in := watcherIntake{h.m}

	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeStarted || err != nil {
		t.Fatalf("Start = %v, %v; want started", st, err)
	}
	if err := h.m.Stop(h.intakeAgent(testIssue).ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	h.waitIdle(testIssue)

	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeExhausted || err != nil {
		t.Errorf("Start after a stop = %v, %v; want exhausted", st, err)
	}
	if n := len(h.prompts()); n != 1 {
		t.Errorf("%d attempts, want 1", n)
	}
}

// Shutting the watcher down stops intake without making the stop final.
func TestWatcherIntake_StopAllIsNotFinal(t *testing.T) {
	h := newIntakeHarness(t, 3)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.next = func(int, int, string) *exec.Cmd { return exec.Command("sleep", "30") }
	in := watcherIntake{h.m}

	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeStarted || err != nil {
		t.Fatalf("Start = %v, %v; want started", st, err)
	}
	h.m.StopAll()
	h.waitIdle(testIssue)

	h.replyAlways(plannertest.CanonicalReply(testRepo, testIssue, plannertest.ValidBody("Dependencies: none")))
	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeStarted || err != nil {
		t.Errorf("Start after StopAll = %v, %v; want started", st, err)
	}
	h.waitIdle(testIssue)
}

// Intake runs that fail are bounded: after max_retries + 1 failed starts the
// next poll punches out. Validate starts fresh.
func TestWatcherIntake_FailuresAreBounded(t *testing.T) {
	h := newIntakeHarness(t, 1)
	h.gh.Put(testIssue, "t", handWritten, testLabel)
	h.gh.Fail["Revision"] = errors.New("gh: rate limited")
	in := watcherIntake{h.m}

	for i := range 2 {
		if _, err := in.Start(testRepo, testIssue); err == nil {
			t.Fatalf("start %d succeeded without a revision", i+1)
		}
	}
	if st, err := in.Start(testRepo, testIssue); st != boundary.IntakeExhausted || err != nil {
		t.Fatalf("Start after 2 failed runs = %v, %v; want exhausted", st, err)
	}

	delete(h.gh.Fail, "Revision")
	if res := h.m.Validate(context.Background(), testIssue); res.Intake.Outcome != IntakeCommitted {
		t.Errorf("Validate = %+v, want a fresh run that commits", res)
	}
}
