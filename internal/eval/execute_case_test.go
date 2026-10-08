package eval

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// execEnv prepares a temp git repo as cwd (standing in for the repository
// root), configures the stub backend with the given keep setting, and returns
// the repo path.
func execEnv(t *testing.T, keep bool) string {
	t.Helper()
	cwd := wsEnv(t)
	wsGit(t, cwd, "init", "-q", "-b", "main")
	wsGit(t, cwd, "add", "-A")
	wsGit(t, cwd, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")
	if err := configure(RunOptions{Backend: inference.NameStub, EvalsDir: "evals", KeepWorkspaces: keep}); err != nil {
		t.Fatal(err)
	}
	return cwd
}

func execRubric() Rubric {
	return Rubric{Agent: "selftest", Criteria: []Criterion{
		{Name: "clarity", Description: "clear", Scoring: "1-5"},
	}}
}

func stubCase(name, workspace, timeout, response string, commands ...string) TestCase {
	return TestCase{
		Name:      name,
		Agent:     "selftest",
		Input:     "do it",
		Workspace: workspace,
		Timeout:   timeout,
		Stub:      &inference.StubScript{Turns: []inference.StubTurn{{Response: response, Commands: commands}}},
	}
}

func TestExecuteCaseNativeMarkerLeavesRepoRootUntouched(t *testing.T) {
	cwd := execEnv(t, true)
	before := wsGit(t, cwd, "status", "--porcelain")

	var out bytes.Buffer
	res := evaluate(execRubric(), []TestCase{stubCase("marker", "", "", "done", "echo hi > marker.txt")}, "abc", &out, nil)
	if len(res.Cases) != 1 {
		t.Fatalf("cases = %d", len(res.Cases))
	}
	cr := res.Cases[0]
	if cr.ActualOutput != "done" || cr.ErrorContext != nil {
		t.Fatalf("output=%q ec=%+v\n%s", cr.ActualOutput, cr.ErrorContext, out.String())
	}
	if cr.WorkspaceDir == "" {
		t.Fatal("WorkspaceDir not recorded")
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(cr.WorkspaceDir)) })

	if got := readFileT(t, filepath.Join(cr.WorkspaceDir, "marker.txt")); got != "hi\n" {
		t.Fatalf("marker.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(cwd, "marker.txt")); err == nil {
		t.Fatal("marker.txt leaked into the repository root")
	}
	if after := wsGit(t, cwd, "status", "--porcelain"); after != before {
		t.Fatalf("repo root status changed:\nbefore=%q\nafter=%q", before, after)
	}
	// The native agent ran in the workspace.
	if cr.Calls[0].Role != "agent" {
		t.Fatalf("calls = %+v", cr.Calls)
	}
}

func TestExecuteCaseKeepVersusRemove(t *testing.T) {
	for _, keep := range []bool{false, true} {
		execEnv(t, keep)
		var out bytes.Buffer
		cr := executeCase(execRubric(), stubCase("k", "", "", "ok", "echo hi > marker.txt"), nil, &out, keep)
		if cr.WorkspaceDir == "" {
			t.Fatalf("keep=%v: WorkspaceDir empty", keep)
		}
		_, err := os.Stat(cr.WorkspaceDir)
		if !keep {
			if err == nil {
				t.Fatalf("keep=false: %s still exists", cr.WorkspaceDir)
			}
			continue
		}
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(cr.WorkspaceDir)) })
		if err != nil {
			t.Fatalf("keep=true: workspace gone: %v", err)
		}
		if _, err := os.Stat(filepath.Join(cr.WorkspaceDir, ".git")); err != nil {
			t.Fatalf("kept workspace is not a git repo: %v", err)
		}
		if fi, err := os.Stat(filepath.Join(cr.WorkspaceDir, ".eval")); err != nil || !fi.IsDir() {
			t.Fatalf("kept workspace lacks .eval/: %v", err)
		}
	}
}

func TestExecuteCaseWorkspaceExistsWhileScoring(t *testing.T) {
	execEnv(t, false)
	var seen string
	var existed bool
	orig := scoreCaseFn
	t.Cleanup(func() { scoreCaseFn = orig })
	scoreCaseFn = func(r Rubric, tc TestCase, cr *CaseResult) {
		seen = cr.WorkspaceDir
		_, err := os.Stat(filepath.Join(cr.WorkspaceDir, "marker.txt"))
		existed = err == nil
		orig(r, tc, cr)
	}

	var out bytes.Buffer
	cr := executeCase(execRubric(), stubCase("s", "", "", "ok", "echo hi > marker.txt"), nil, &out, false)
	if seen == "" || seen != cr.WorkspaceDir {
		t.Fatalf("WorkspaceDir during scoring = %q, recorded = %q", seen, cr.WorkspaceDir)
	}
	if !existed {
		t.Fatal("workspace (or its marker.txt) did not exist while scoring")
	}
	if _, err := os.Stat(cr.WorkspaceDir); err == nil {
		t.Fatal("workspace not removed after scoring")
	}
}

func TestExecuteCaseTimeoutRecordedNatively(t *testing.T) {
	execEnv(t, false)
	var out bytes.Buffer
	start := time.Now()
	cr := executeCase(execRubric(), stubCase("slow", "", "1s", "never", "sleep 3"), nil, &out, false)
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Fatalf("timeout case took %v, want < 2.5s", d)
	}
	if cr.ActualOutput != "" {
		t.Fatalf("ActualOutput = %q, want empty", cr.ActualOutput)
	}
	if cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, "timeout after 1s") {
		t.Fatalf("ErrorContext = %+v", cr.ErrorContext)
	}
	if len(cr.Calls) != 1 || !strings.Contains(cr.Calls[0].Error, "timeout") {
		t.Fatalf("call record = %+v", cr.Calls)
	}
}

func TestInvokeAgentCaseTimeoutPrecedence(t *testing.T) {
	execEnv(t, false)
	t.Setenv("KAIRON_EVAL_TIMEOUT", "5m")
	ws := newWS(t, TestCase{Name: "p"})
	stub := &inference.StubScript{Turns: []inference.StubTurn{{Response: "x", Commands: []string{"sleep 3"}}}}
	start := time.Now()
	_, _, _, ec, err := invokeAgent("selftest", "p", nil, callOpts{Stub: stub, Workspace: ws, Timeout: time.Second})
	if !errors.Is(err, inference.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if time.Since(start) > 2500*time.Millisecond {
		t.Fatal("case timeout did not override KAIRON_EVAL_TIMEOUT")
	}
	if ec == nil || ec.WorkingDir != ws.Dir {
		t.Fatalf("ErrorContext.WorkingDir = %+v, want workspace %s", ec, ws.Dir)
	}
}

func TestExecuteCaseSeededFixtureShowsModifiedFile(t *testing.T) {
	execEnv(t, true)
	writeCfgFile(t, "evals/fixtures/workspaces/seeded/README.md", "# seeded\n")
	writeCfgFile(t, "evals/fixtures/workspaces/seeded/docs/note.txt", "note\n")

	var out bytes.Buffer
	cr := executeCase(execRubric(), stubCase("seeded", "seeded", "", "ok", "echo more >> README.md"), nil, &out, true)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(cr.WorkspaceDir)) })
	if cr.ErrorContext != nil || cr.ActualOutput != "ok" {
		t.Fatalf("output=%q ec=%+v", cr.ActualOutput, cr.ErrorContext)
	}
	if st := wsGit(t, cr.WorkspaceDir, "status", "--porcelain"); st != "M README.md" {
		t.Fatalf("git status = %q, want ' M README.md'", st)
	}
	if n := wsGit(t, cr.WorkspaceDir, "rev-list", "--count", "HEAD"); n != "1" {
		t.Fatalf("commit count = %s, want 1", n)
	}
}

func TestExecuteCaseWorkspaceFailureIsRecorded(t *testing.T) {
	execEnv(t, false)
	var out bytes.Buffer
	// A fixture that does not exist (case built by hand, bypassing loadCases).
	cr := executeCase(execRubric(), stubCase("bad", "missing", "", "ok"), nil, &out, false)
	if cr.ActualOutput != "" || cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, "workspace") {
		t.Fatalf("result = %+v", cr)
	}
	if cr.WorkspaceDir != "" {
		t.Fatalf("WorkspaceDir = %q, want empty", cr.WorkspaceDir)
	}
}

func TestConfigureKeepWorkspaces(t *testing.T) {
	chdirTemp(t)
	if err := configure(RunOptions{KeepWorkspaces: true}); err != nil {
		t.Fatal(err)
	}
	if !cfg.keepWorkspaces {
		t.Fatal("keepWorkspaces not applied")
	}
	if err := configure(RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if cfg.keepWorkspaces {
		t.Fatal("keepWorkspaces leaked into the next configure")
	}
}

// evaluateProgressive goes through the same executeCase path.
func TestEvaluateProgressiveUsesWorkspace(t *testing.T) {
	execEnv(t, true)
	resultsDir := t.TempDir()
	var out bytes.Buffer
	res := evaluateProgressive(execRubric(), []TestCase{stubCase("prog", "", "", "ok", "echo hi > marker.txt")},
		"abc", &out, resultsDir, false, nil)
	if len(res.Cases) != 1 || res.Cases[0].WorkspaceDir == "" {
		t.Fatalf("result = %+v", res.Cases)
	}
	dir := res.Cases[0].WorkspaceDir
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(dir)) })
	if got := readFileT(t, filepath.Join(dir, "marker.txt")); got != "hi\n" {
		t.Fatalf("marker.txt = %q", got)
	}
}

// A requires_sandbox case run natively must fail before creating a workspace
// or invoking the agent.
func TestExecuteCaseRequiresSandboxFailsFastNatively(t *testing.T) {
	execEnv(t, true)
	root := os.Getenv(workspaceRootEnv)

	tc := stubCase("sandbox-only", "", "", "never", "touch invoked.txt")
	tc.RequiresSandbox = true
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, true)

	if cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, "requires --sandbox") {
		t.Fatalf("ErrorContext = %+v\n%s", cr.ErrorContext, out.String())
	}
	if !strings.Contains(out.String(), "requires --sandbox") {
		t.Errorf("output lacks the reason: %s", out.String())
	}
	if cr.ActualOutput != "" || len(cr.Calls) != 0 {
		t.Errorf("agent must not be invoked: output=%q calls=%d", cr.ActualOutput, len(cr.Calls))
	}
	if cr.WorkspaceDir != "" {
		t.Errorf("WorkspaceDir = %q, want none", cr.WorkspaceDir)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("workspace root not empty: %v", entries)
	}
}
