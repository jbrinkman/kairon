package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/github/githubtest"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/planner/plannertest"
)

func validateTestModel(t *testing.T, mgr *agent.Manager) model {
	t.Helper()
	cfg := &config.Config{Repo: "o/r", Label: "kairon", LoadedTheme: createMinimalTheme()}
	if mgr == nil {
		mgr = agent.NewManager(cfg)
	}
	return newModel(nil, mgr, cfg, nil, nil)
}

func activityText(m model) string { return strings.Join(m.activityLines, "\n") }

// AC 10: the command returns at once with a command that does the work; no
// run starts on the UI goroutine.
func TestValidateCommand_RunsOffTheUIGoroutine(t *testing.T) {
	m := validateTestModel(t, nil)
	var calls atomic.Int32
	m.validate = func(_ context.Context, issue int) agent.ValidateResult {
		calls.Add(1)
		return agent.ValidateResult{Status: agent.ValidateAlreadyValid, Issue: issue, Hash: strings.Repeat("a", 64)}
	}

	m, cmd := m.executeCommand("validate 42")

	if cmd == nil {
		t.Fatal("executeCommand returned no command")
	}
	if calls.Load() != 0 {
		t.Fatal("validate ran before executeCommand returned")
	}
	if !strings.Contains(activityText(m), "Validating issue #42") {
		t.Errorf("console = %q, want a validating line", activityText(m))
	}
	msg, ok := cmd().(validateDoneMsg)
	if !ok || calls.Load() != 1 || msg.res.Issue != 42 {
		t.Fatalf("cmd() = %#v after %d calls, want one validateDoneMsg for #42", msg, calls.Load())
	}
	next, _ := m.Update(msg)
	if got := activityText(next.(model)); !strings.Contains(got, "Issue #42 is already valid (sha256 aaaaaaaaaaaa)") {
		t.Errorf("console = %q, want the already-valid line", got)
	}
}

func TestValidateCommand_Usage(t *testing.T) {
	for _, in := range []string{"validate", "validate abc", "validate 0", "validate 1 2"} {
		m := validateTestModel(t, nil)
		m.validate = func(context.Context, int) agent.ValidateResult {
			t.Fatalf("%q ran validate", in)
			return agent.ValidateResult{}
		}
		m, cmd := m.executeCommand(in)
		if cmd != nil || !strings.Contains(activityText(m), "Usage: validate <issue-number>") {
			t.Errorf("%q: cmd = %v, console = %q; want the usage line and no command", in, cmd != nil, activityText(m))
		}
	}
}

func TestValidateDone_ConsoleLines(t *testing.T) {
	hash := strings.Repeat("b", 64)
	cases := []struct {
		name string
		res  agent.ValidateResult
		want []string
	}{
		{"committed, label removed", agent.ValidateResult{Status: agent.ValidateFinished, Issue: 7, HadFailedLabel: true, LabelRemoved: true,
			Intake: agent.IntakeResult{Outcome: agent.IntakeCommitted, Hash: hash, Attempts: 2}},
			[]string{"Issue #7 validated and stamped (sha256 bbbbbbbbbbbb) after 2 attempt(s)", "Removed `kairon-failed` from #7"}},
		{"committed, label removal failed", agent.ValidateResult{Status: agent.ValidateFinished, Issue: 7, HadFailedLabel: true, LabelErr: errors.New("gh 502"),
			Intake: agent.IntakeResult{Outcome: agent.IntakeCommitted, Hash: hash, Attempts: 1}},
			[]string{"validated and stamped", "removing `kairon-failed` failed: gh 502", "Remove the label by hand"}},
		{"punched out", agent.ValidateResult{Status: agent.ValidateFinished, Issue: 7,
			Intake: agent.IntakeResult{Outcome: agent.IntakePunchedOut, Failure: &guard.FailureContext{Guard: "intake"}}},
			[]string{"punched out issue #7 (guard intake); see the comment on the issue"}},
		{"changed", agent.ValidateResult{Status: agent.ValidateFinished, Issue: 7, Intake: agent.IntakeResult{Outcome: agent.IntakeChanged}},
			[]string{"edited during validation; nothing was written", "`validate 7`"}},
		{"stopped", agent.ValidateResult{Status: agent.ValidateFinished, Issue: 7, Intake: agent.IntakeResult{Outcome: agent.IntakeStopped}},
			[]string{"Validation of #7 was stopped"}},
		{"run failed", agent.ValidateResult{Status: agent.ValidateFinished, Issue: 7, Intake: agent.IntakeResult{Outcome: agent.IntakeFailed, Err: errors.New("gh down")}},
			[]string{"Validation of #7 failed: gh down"}},
		{"already valid with fail label", agent.ValidateResult{Status: agent.ValidateAlreadyValid, Issue: 7, Hash: hash, HadFailedLabel: true},
			[]string{"already valid", "remove `kairon-failed` to restart it"}},
		{"refused", agent.ValidateResult{Status: agent.ValidateRefused, Issue: 7, Reason: "the issue is closed"},
			[]string{"Not validating issue #7: the issue is closed"}},
		{"already running", agent.ValidateResult{Status: agent.ValidateAlreadyRunning, Issue: 7},
			[]string{"Validation already running for #7"}},
		{"error", agent.ValidateResult{Status: agent.ValidateError, Issue: 7, Err: errors.New("rate limited")},
			[]string{"Validate #7 failed: rate limited"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validateTestModel(t, nil)
			next, _ := m.Update(validateDoneMsg{res: tc.res})
			got := activityText(next.(model))
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("console = %q, want it to contain %q", got, w)
				}
			}
		})
	}
}

// AC 13: status lists a running intake run with its kind.
func TestStatus_ListsIntakeRuns(t *testing.T) {
	t.Chdir(t.TempDir())
	gh := githubtest.New()
	gh.Put(42, "Export is slow", "## Problem\n\nSlow.", "kairon")
	dir := t.TempDir()
	release := filepath.Join(dir, "release")
	cfg := &config.Config{Repo: "o/r", Label: "kairon", MaxRetries: 0, LoadedTheme: createMinimalTheme()}
	mgr := agent.NewManagerWithOptions(cfg, agent.Options{
		GitHub: gh,
		IntakeCommand: func(string) *exec.Cmd {
			return plannertest.BlockingCommand(dir, plannertest.CanonicalReply("o/r", 42, plannertest.ValidBody("Dependencies: none")), release)
		},
	})
	done := make(chan agent.ValidateResult, 1)
	go func() { done <- mgr.Validate(context.Background(), 42) }()
	deadline := time.Now().Add(5 * time.Second)
	for !mgr.IntakeActive(42) {
		if time.Now().After(deadline) {
			t.Fatal("intake did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	m := validateTestModel(t, mgr)
	m, _ = m.handleStatus()
	status := strings.Join(m.overlayContent.content, "\n")
	if !strings.Contains(status, "Issue #42") || !strings.Contains(status, "intake, running") {
		t.Errorf("status = %q, want issue #42 listed as intake, running", status)
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if res := <-done; res.Intake.Outcome != agent.IntakeCommitted {
		t.Errorf("validate = %+v, want committed", res)
	}
}

func TestCommandRegistry_HasValidate(t *testing.T) {
	r := NewCommandRegistry(nil)
	c, ok := r.commands["validate"]
	if !ok || !c.HasArgs || c.ArgPattern != "<issue>" {
		t.Fatalf("validate command = %+v (registered %v), want one taking <issue>", c, ok)
	}
	m := validateTestModel(t, nil)
	m, _ = m.handleHelp()
	if !strings.Contains(strings.Join(m.overlayContent.content, "\n"), "validate <issue>") {
		t.Error("help does not list validate <issue>")
	}
}
