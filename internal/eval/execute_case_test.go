package eval

import (
	"bytes"
	"encoding/json"
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
	t.Run("keep workspaces", func(t *testing.T) {
		chdirTemp(t)
		t.Cleanup(resetConfig)
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
	})

	t.Run("debug flag", func(t *testing.T) {
		t.Cleanup(resetConfig)
		if err := configure(RunOptions{Backend: "stub", Debug: true}); err != nil {
			t.Fatal(err)
		}
		if !cfg.debug {
			t.Error("configure(Debug: true) did not set cfg.debug")
		}
		if err := configure(RunOptions{Backend: "stub"}); err != nil {
			t.Fatal(err)
		}
		if cfg.debug {
			t.Error("configure without Debug left cfg.debug set")
		}
	})
}

// evaluateProgressive goes through the same executeCase path.
func TestEvaluateProgressiveUsesWorkspace(t *testing.T) {
	execEnv(t, true)
	resultsDir := t.TempDir()
	var out bytes.Buffer
	res := evalProgressiveOK(t, execRubric(), []TestCase{stubCase("prog", "", "", "ok", "echo hi > marker.txt")},
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

// provEnv prepares a git project whose selftest agent declares a conventions
// resource, a workspace fixture "conv" that supplies it, and pins the run.
func provEnv(t *testing.T) {
	t.Helper()
	execEnv(t, false)
	writeCfgFile(t, ".kiro/agents/selftest.json",
		`{"name":"selftest","model":"claude-sonnet-5.5","prompt":"inline","resources":["skill://.kiro/skills/x-conventions/SKILL.md"]}`)
	gitTrack(t, ".kiro/agents/selftest.json")
	writeCfgFile(t, "evals/fixtures/workspaces/conv/.kiro/skills/x-conventions/SKILL.md", "fixture override\n")
	writeCfgFile(t, "evals/fixtures/workspaces/conv/README.md", "# conv\n")
	if err := pinRun("selftest", RunOptions{}, false); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteCasePerCaseProvenanceFromStagedWorkspace(t *testing.T) {
	provEnv(t)
	const res = "skill://.kiro/skills/x-conventions/SKILL.md"
	pin, ok := cfg.pins.pinOf("selftest")
	if !ok {
		t.Fatal("run is not pinned")
	}

	var out bytes.Buffer
	plain := executeCase(execRubric(), stubCase("plain", "", "", "ok"), nil, &out, false)
	withFix := executeCase(execRubric(), stubCase("fix", "conv", "", "ok"), nil, &out, false)

	// A case without the fixture runs the baseline workspace.
	if plain.PromptSHA256 != pin.Provenance.PromptSHA256 {
		t.Errorf("plain prompt_sha256 = %q, want baseline %q", plain.PromptSHA256, pin.Provenance.PromptSHA256)
	}
	if plain.ResourcesPresent == nil || len(plain.ResourcesPresent) != 0 {
		t.Errorf("plain resources_present = %#v, want []", plain.ResourcesPresent)
	}
	// A case with the fixture hashes and lists what its agent actually sees.
	if len(withFix.ResourcesPresent) != 1 || withFix.ResourcesPresent[0] != res {
		t.Errorf("fixture resources_present = %v, want [%s]", withFix.ResourcesPresent, res)
	}
	if withFix.PromptSHA256 == "" || withFix.PromptSHA256 == plain.PromptSHA256 {
		t.Errorf("fixture prompt_sha256 = %q, must differ from the no-fixture %q", withFix.PromptSHA256, plain.PromptSHA256)
	}
	// The agent-level pin is untouched.
	if got, _ := cfg.pins.pinOf("selftest"); got.Provenance.PromptSHA256 != pin.Provenance.PromptSHA256 || len(got.Provenance.ResourcesPresent) != 0 {
		t.Errorf("agent-level pin changed: %+v", got.Provenance)
	}
	// Every agent call record carries the case's hash.
	for _, c := range []CaseResult{plain, withFix} {
		var agents int
		for _, rec := range c.Calls {
			if rec.Role != "agent" {
				continue
			}
			agents++
			if rec.PromptSHA256 != c.PromptSHA256 {
				t.Errorf("case %s call prompt_sha256 = %q, want case value %q", c.CaseName, rec.PromptSHA256, c.PromptSHA256)
			}
		}
		if agents == 0 {
			t.Errorf("case %s has no agent call record", c.CaseName)
		}
	}
}

func TestExecuteCaseUnpinnedOmitsProvenance(t *testing.T) {
	execEnv(t, false)
	var out bytes.Buffer
	cr := executeCase(execRubric(), stubCase("u", "", "", "ok"), nil, &out, false)
	if cr.PromptSHA256 != "" || cr.ResourcesPresent != nil {
		t.Fatalf("unpinned case recorded provenance: %q %v", cr.PromptSHA256, cr.ResourcesPresent)
	}
	b, err := json.Marshal(cr)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"prompt_sha256"`, `"resources_present"`} {
		if strings.Contains(string(b), key) {
			t.Errorf("unpinned case JSON contains %s: %s", key, b)
		}
	}
}

func TestExecuteCaseProvenanceErrorIsWorkspaceFailure(t *testing.T) {
	provEnv(t)
	// The pin exists, but the staged workspace has no agent config to hash.
	if err := os.Remove(".kiro/agents/selftest.json"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cr := executeCase(execRubric(), stubCase("broken", "", "", "ok"), nil, &out, false)
	if cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, "workspace") {
		t.Fatalf("ErrorContext = %+v, want a workspace failure", cr.ErrorContext)
	}
	if !strings.Contains(out.String(), "workspace failed") {
		t.Errorf("output lacks the workspace failure marker: %q", out.String())
	}
	if cr.ActualOutput != "" || len(cr.Calls) != 0 || cr.PromptSHA256 != "" {
		t.Errorf("agent ran or provenance set despite the error: %+v", cr)
	}
	if cr.WorkspaceDir != "" {
		if _, err := os.Stat(cr.WorkspaceDir); err == nil {
			t.Errorf("workspace %s was not removed", cr.WorkspaceDir)
		}
	}
}

func TestCaseResultJSONProvenance(t *testing.T) {
	has := func(t *testing.T, cr CaseResult, key string) bool {
		t.Helper()
		b, err := json.Marshal(cr)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		_, ok := m[key]
		return ok
	}
	// Old shape: neither field.
	old := CaseResult{CaseName: "c"}
	if has(t, old, "prompt_sha256") || has(t, old, "resources_present") {
		t.Error("a case without provenance must omit both fields")
	}
	// prompt_sha256 set with an empty list: resources_present is still emitted, as [].
	cr := CaseResult{CaseName: "c", PromptSHA256: strings.Repeat("a", 64)}
	b, _ := json.Marshal(cr)
	if !strings.Contains(string(b), `"resources_present":[]`) || !strings.Contains(string(b), `"prompt_sha256"`) {
		t.Errorf("JSON = %s, want prompt_sha256 and resources_present:[]", b)
	}
	// Round trip.
	cr.ResourcesPresent = []string{"skill://a"}
	b, _ = json.Marshal(cr)
	var back CaseResult
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.PromptSHA256 != cr.PromptSHA256 || len(back.ResourcesPresent) != 1 || back.ResourcesPresent[0] != "skill://a" {
		t.Errorf("round trip = %+v", back)
	}
}
