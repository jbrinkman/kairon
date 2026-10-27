package watcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/github"
	"github.com/jbrinkman/kairon/internal/github/githubtest"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
	"github.com/jbrinkman/kairon/internal/provenance"
)

const (
	e2eRepo    = "o/r"
	e2eLabel   = "kairon"
	e2eIssue   = 7
	handIssue  = "## Problem\n\nExport is slow.\n\nMake it faster please."
	pollWithin = 10 * time.Second
)

// pipeline is a watcher and a production-constructed manager over one
// githubtest.Fake. Intake attempts are real `sh` processes printing a chosen
// reply; krew-lead is recorded, never run. The boundary, intake, commit,
// failure contract and dependency checks are the real code.
type pipeline struct {
	t        *testing.T
	gh       *githubtest.Fake
	w        *Watcher
	m        *agent.Manager
	dir      string
	mu       sync.Mutex
	prompts  []string
	krewLead []*exec.Cmd
	next     func(prompt string) *exec.Cmd
}

func newPipeline(t *testing.T) *pipeline {
	t.Helper()
	t.Chdir(t.TempDir())
	return newPipelineHere(t, githubtest.New())
}

// newPipelineHere builds a pipeline in the current directory over gh, as a
// restarted watcher would.
func newPipelineHere(t *testing.T, gh *githubtest.Fake) *pipeline {
	t.Helper()
	p := &pipeline{t: t, gh: gh, dir: t.TempDir()}
	p.replyWith(plannertest.ValidBody("Dependencies: none"))
	cfg := &config.Config{Repo: e2eRepo, Label: e2eLabel, MaxRetries: 2, PollInterval: time.Minute}
	worktree := t.TempDir()
	p.m = agent.NewManagerWithOptions(cfg, agent.Options{
		GitHub: gh,
		IntakeCommand: func(prompt string) *exec.Cmd {
			p.mu.Lock()
			p.prompts = append(p.prompts, prompt)
			next := p.next
			p.mu.Unlock()
			return next(prompt)
		},
		RetryDelay:     func(int) time.Duration { return 0 },
		CreateWorktree: func(string) (string, error) { return worktree, nil },
		StartAgent: func(cmd *exec.Cmd) error {
			p.mu.Lock()
			p.krewLead = append(p.krewLead, cmd)
			p.mu.Unlock()
			return nil
		},
		MonitorAgent: func(*agent.Agent, *exec.Cmd) {},
	})
	p.w = New(cfg, p.m, "")
	p.w.selectIssues = func(_, label, _ string) (github.Selection, error) { return gh.Select(label), nil }
	p.w.reconcile = func(repo string, issue int, failLabel string) error {
		_, err := failure.Reconcile(gh, failure.Target{Repo: repo, Issue: issue, FailLabel: failLabel})
		return err
	}
	p.w.dependencyValidator.getIssueDetails = gh.Details
	return p
}

func (p *pipeline) replyWith(body string) {
	p.next = func(string) *exec.Cmd {
		return plannertest.ReplyCommand(p.dir, plannertest.CanonicalReply(e2eRepo, e2eIssue, body))
	}
}

func (p *pipeline) intakeStarts() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.prompts)
}

func (p *pipeline) krewLeadStarts() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.krewLead)
}

// waitIntakeEnded waits until the intake run has ended and released the issue.
func (p *pipeline) waitIntakeEnded(issue int) {
	p.t.Helper()
	deadline := time.Now().Add(pollWithin)
	for p.m.IntakeActive(issue) || slices.Contains(trackedKeys(p.w), issue) {
		if time.Now().After(deadline) {
			p.t.Fatalf("intake for issue #%d did not end (tracked %v)", issue, trackedKeys(p.w))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// AC 1 and AC 14: poll 1 starts intake on an unmarked issue, the run commits,
// and poll 2 admits the stamped issue and starts krew-lead. The manager comes
// from the production constructor, so this fails if Intake is not wired.
func TestPipeline_UnmarkedIssueIsStampedThenImplemented(t *testing.T) {
	p := newPipeline(t)
	p.gh.Put(e2eIssue, "Export is slow", handIssue, e2eLabel)

	p.w.checkIssues()
	if p.intakeStarts() != 1 || p.krewLeadStarts() != 0 {
		t.Fatalf("after poll 1: intake starts %d, krew-lead starts %d; want 1 and 0", p.intakeStarts(), p.krewLeadStarts())
	}
	p.waitIntakeEnded(e2eIssue)
	if got := provenance.Classify(p.gh.Get(e2eIssue).Body); got != provenance.StateValid {
		t.Fatalf("body after intake classifies %v, want valid", got)
	}

	p.w.checkIssues()

	if p.krewLeadStarts() != 1 {
		t.Fatalf("after poll 2: krew-lead starts %d, want 1", p.krewLeadStarts())
	}
	if p.intakeStarts() != 1 {
		t.Errorf("intake starts %d, want 1", p.intakeStarts())
	}
	if got := p.gh.Get(e2eIssue).Comments; len(got) != 0 {
		t.Errorf("comments = %+v, want none: nothing punched out", got)
	}
	data, err := os.ReadFile(filepath.Join(".kairon", "inputs", fmt.Sprintf("issue-%d.yaml", e2eIssue)))
	if err != nil || !strings.Contains(string(data), "Add a JSON Lines exporter") {
		t.Errorf("input document = %q (%v), want the stamped body", data, err)
	}
}

// AC 2: while intake runs, further polls start no second run, keep the issue
// tracked and burn no retry.
func TestPipeline_NoDuplicateIntakeWhileRunning(t *testing.T) {
	p := newPipeline(t)
	p.gh.Put(e2eIssue, "Export is slow", handIssue, e2eLabel)
	release := filepath.Join(p.dir, "release")
	reply := plannertest.CanonicalReply(e2eRepo, e2eIssue, plannertest.ValidBody("Dependencies: none"))
	p.next = func(string) *exec.Cmd { return plannertest.BlockingCommand(p.dir, reply, release) }

	for range 3 {
		p.w.checkIssues()
	}

	if n := p.intakeStarts(); n != 1 {
		t.Errorf("intake starts = %d, want 1", n)
	}
	if got := trackedKeys(p.w); !slices.Equal(got, []int{e2eIssue}) {
		t.Errorf("tracked = %v, want [%d] while intake runs", got, e2eIssue)
	}
	if got := p.w.getCurrentRetryCount(e2eIssue); got != 0 {
		t.Errorf("retry count = %d, want 0", got)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p.waitIntakeEnded(e2eIssue)
}

// AC 3: a new watcher does not start a second intake while a previous run's
// process is alive. With the process dead, the stale marker is ignored.
func TestPipeline_NoDuplicateIntakeAfterRestart(t *testing.T) {
	t.Chdir(t.TempDir())
	gh := githubtest.New()
	gh.Put(e2eIssue, "Export is slow", handIssue, e2eLabel)
	marker := filepath.Join(".kairon", "intake", fmt.Sprintf("issue-%d.pid", e2eIssue))
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newPipelineHere(t, gh)
	p.w.checkIssues()
	if n := p.intakeStarts(); n != 0 {
		t.Fatalf("intake starts = %d with a live marker, want 0", n)
	}
	if got := trackedKeys(p.w); len(got) != 0 {
		t.Errorf("tracked = %v, want none: no release would ever come", got)
	}

	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte(fmt.Sprintf("%d\n", dead.ProcessState.Pid())), 0o644); err != nil {
		t.Fatal(err)
	}
	p = newPipelineHere(t, gh)
	p.w.checkIssues()
	if n := p.intakeStarts(); n != 1 {
		t.Fatalf("intake starts = %d with a dead marker, want 1", n)
	}
	p.waitIntakeEnded(e2eIssue)
}

// AC 4: after intake commits, a body that is mismatched at the next poll is
// punched out instead of starting intake again.
func TestPipeline_LoopGuardPunchesOutAfterCommit(t *testing.T) {
	p := newPipeline(t)
	p.gh.Put(e2eIssue, "Export is slow", handIssue, e2eLabel)
	p.w.checkIssues()
	p.waitIntakeEnded(e2eIssue)

	p.gh.Edit(e2eIssue, strings.Replace(p.gh.Get(e2eIssue).Body, "jq", "jq and awk", 1))
	p.w.checkIssues()

	if n := p.intakeStarts(); n != 1 {
		t.Errorf("intake starts = %d, want 1 (no second run)", n)
	}
	if p.krewLeadStarts() != 0 {
		t.Error("krew-lead started on a mismatched body")
	}
	is := p.gh.Get(e2eIssue)
	if len(is.Comments) != 1 || !strings.Contains(is.Comments[0].Body, "the body changed after intake committed it") {
		t.Fatalf("comments = %+v, want one loop-guard punch-out", is.Comments)
	}
	if !slices.Contains(is.Labels, failure.FailedLabel(e2eLabel)) {
		t.Errorf("labels = %v, want the fail label", is.Labels)
	}
}

// AC 5: a body that intake rewrote with an unresolved dependency is held by
// dependency validation on the next poll, not admitted.
func TestPipeline_StampedBodyIsDependencyCheckedAgain(t *testing.T) {
	p := newPipeline(t)
	p.gh.Put(e2eIssue, "Export is slow", handIssue, e2eLabel)
	p.gh.Put(99, "Prerequisite", "still open")
	p.replyWith(plannertest.ValidBody("Dependencies: #99"))

	p.w.checkIssues()
	p.waitIntakeEnded(e2eIssue)
	if !strings.Contains(p.gh.Get(e2eIssue).Body, "Dependencies: #99") {
		t.Fatal("test setup: intake did not write the dependency")
	}
	p.w.checkIssues()

	if p.krewLeadStarts() != 0 {
		t.Error("krew-lead started although dependency #99 is open")
	}
	if p.gh.Count("ReadIssue") != 1 {
		t.Errorf("ReadIssue calls = %d, want 1: the second poll stops at dependency validation", p.gh.Count("ReadIssue"))
	}
}

// Watcher intake never runs on an issue with the fail label: it is not even
// selected.
func TestPipeline_FailedIssueGetsNoIntake(t *testing.T) {
	p := newPipeline(t)
	p.gh.Put(e2eIssue, "Export is slow", handIssue, e2eLabel, failure.FailedLabel(e2eLabel))

	p.w.checkIssues()

	if n := p.intakeStarts(); n != 0 {
		t.Errorf("intake starts = %d, want 0", n)
	}
}

// A validate run the TUI started is seen by the watcher as running intake.
func TestPipeline_WatcherSkipsIssueValidateIsRunning(t *testing.T) {
	p := newPipeline(t)
	p.gh.Put(e2eIssue, "Export is slow", handIssue, e2eLabel)
	release := filepath.Join(p.dir, "release")
	reply := plannertest.CanonicalReply(e2eRepo, e2eIssue, plannertest.ValidBody("Dependencies: none"))
	p.next = func(string) *exec.Cmd { return plannertest.BlockingCommand(p.dir, reply, release) }

	done := make(chan agent.ValidateResult, 1)
	go func() { done <- p.m.Validate(t.Context(), e2eIssue) }()
	deadline := time.Now().Add(pollWithin)
	for !p.m.IntakeActive(e2eIssue) {
		if time.Now().After(deadline) {
			t.Fatal("validate did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.w.checkIssues()
	if n := p.intakeStarts(); n != 1 {
		t.Errorf("intake starts = %d, want 1", n)
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if res := <-done; res.Intake.Outcome != agent.IntakeCommitted {
		t.Fatalf("validate = %+v, want committed", res)
	}
	p.w.checkIssues()
	if p.krewLeadStarts() != 1 {
		t.Errorf("krew-lead starts = %d after validate committed, want 1", p.krewLeadStarts())
	}
}
