package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

func TestEvalFlagsRegistered(t *testing.T) {
	backend := evalCmd.Flags().Lookup("backend")
	if backend == nil {
		t.Fatal("--backend flag not registered on eval")
	}
	if backend.DefValue != inference.NameKiroCLI {
		t.Errorf("--backend default = %q, want %q", backend.DefValue, inference.NameKiroCLI)
	}
	for _, name := range inference.Names() {
		if !strings.Contains(backend.Usage, name) {
			t.Errorf("--backend usage %q does not list backend %q", backend.Usage, name)
		}
	}

	evalsDir := evalCmd.PersistentFlags().Lookup("evals-dir")
	if evalsDir == nil {
		t.Fatal("--evals-dir must be a persistent flag on eval")
	}
	if evalsDir.DefValue != "" {
		t.Errorf("--evals-dir default = %q, want empty (use package default)", evalsDir.DefValue)
	}
	// Persistent flags are inherited, so `eval diff` honours --evals-dir too.
	if diffCmd.InheritedFlags().Lookup("evals-dir") == nil {
		t.Error("diff subcommand does not inherit --evals-dir")
	}
}

func TestEvalUnknownBackendRejected(t *testing.T) {
	orig := evalBackend
	t.Cleanup(func() { evalBackend = orig })
	evalBackend = "nope"

	err := evalCmd.RunE(evalCmd, []string{"selftest"})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	for _, want := range []string{"nope", inference.NameKiroCLI, inference.NameStub} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestEvalUnknownBackendRejectedForPerfAndCleanup(t *testing.T) {
	origBackend, origPerf, origCleanup := evalBackend, evalPerf, evalCleanup
	t.Cleanup(func() { evalBackend, evalPerf, evalCleanup = origBackend, origPerf, origCleanup })
	evalBackend = "nope"

	// --perf and --cleanup go through RunWithOptions, so configuration (and
	// backend validation) applies before any work is done.
	for name, set := range map[string]func(){
		"perf":    func() { evalPerf, evalCleanup = true, false },
		"cleanup": func() { evalPerf, evalCleanup = false, true },
	} {
		set()
		if err := evalCmd.RunE(evalCmd, nil); err == nil || !strings.Contains(err.Error(), "unknown backend") {
			t.Errorf("--%s with bad backend: err = %v, want unknown backend error", name, err)
		}
	}
}

// evalSelfTestProject chdirs into a temp git repo whose .kairon/config.yaml is
// configYAML ("" for none) and returns a writable copy of the self-test
// fixtures. Eval flags are restored on cleanup.
func evalSelfTestProject(t *testing.T, configYAML string) (evalsDir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := filepath.Abs(filepath.Join("..", "..", "..", "internal", "eval", "testdata", "evals"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	origBackend, origEvals := evalBackend, evalEvalsDir
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		evalBackend, evalEvalsDir = origBackend, origEvals
	})

	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v: %s", err, out)
		}
	}
	if configYAML != "" {
		if err := os.MkdirAll(".kairon", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".kairon", "config.yaml"), []byte(configYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	evalsDir = filepath.Join(dir, "evals-copy")
	if err := os.CopyFS(evalsDir, os.DirFS(fixtures)); err != nil {
		t.Fatal(err)
	}
	// results/ is git-ignored run output (written by `task eval:selftest`);
	// a copy must not inherit stale runs from the developer's working tree.
	if err := os.RemoveAll(filepath.Join(evalsDir, "results")); err != nil {
		t.Fatal(err)
	}
	evalBackend, evalEvalsDir = "stub", evalsDir
	return evalsDir
}

func TestEvalRejectsUnpinnedJudgeModel(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "evals:\n  judge_model: auto\n")

	err := evalCmd.RunE(evalCmd, []string{"selftest"})
	if err == nil {
		t.Fatal("expected the run to be refused for judge_model auto")
	}
	for _, want := range []string{"auto", "allowed_models"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
		t.Errorf("results dir exists after refusal (stat err = %v)", statErr)
	}
}

func TestEvalSelfTestSucceedsWithDefaults(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")

	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("self-test with default evals config failed: %v", err)
	}
	runs, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*", "summary.json"))
	if len(runs) != 1 {
		t.Fatalf("summary.json files = %v, want 1", runs)
	}
}

func TestEvalPromptFileFlagRegistered(t *testing.T) {
	f := evalCmd.Flags().Lookup("prompt-file")
	if f == nil {
		t.Fatal("--prompt-file flag not registered on eval")
	}
	if f.DefValue != "" {
		t.Errorf("--prompt-file default = %q, want empty", f.DefValue)
	}
	if f.Value.Type() != "string" {
		t.Errorf("--prompt-file type = %q, want string", f.Value.Type())
	}
	if !strings.Contains(strings.ToLower(f.Usage), "agent") {
		t.Errorf("--prompt-file usage %q does not name the agent requirement", f.Usage)
	}
}

// setEvalPromptFile sets the package flag var and restores it (and the other
// vars the candidate tests touch) on cleanup.
func setEvalPromptFile(t *testing.T, path string, keep bool) {
	t.Helper()
	origPrompt, origKeep := evalPromptFile, evalKeepWorkspaces
	t.Cleanup(func() { evalPromptFile, evalKeepWorkspaces = origPrompt, origKeep })
	evalPromptFile, evalKeepWorkspaces = path, keep
}

// AC4: --prompt-file without an agent is refused with an "agent is required" error.
func TestEvalPromptFileWithoutAgentRejected(t *testing.T) {
	setEvalPromptFile(t, "x.md", false)

	err := evalCmd.RunE(evalCmd, nil)
	if err == nil {
		t.Fatal("expected error for --prompt-file without an agent")
	}
	for _, want := range []string{"agent is required", "--prompt-file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// AC5: a missing candidate is refused before any case runs; no results dir.
func TestEvalPromptFileMissingRejectedWithoutResults(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")
	setEvalPromptFile(t, filepath.Join(t.TempDir(), "missing.md"), false)

	err := evalCmd.RunE(evalCmd, []string{"selftest"})
	if err == nil {
		t.Fatal("expected error for a missing candidate prompt file")
	}
	if !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("error %q does not name the missing file", err)
	}
	if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
		t.Errorf("results dir exists after refusal (stat err = %v)", statErr)
	}
}

// gitOut runs git in dir and returns its trimmed stdout.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commitSelftestAgent commits a copy of the selftest agent (config + prompt)
// under the project's .kiro/agents so that nothing writing to it is observable
// through git.
func commitSelftestAgent(t *testing.T, evalsDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(".kiro", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"selftest.json", "selftest-prompt.md"} {
		b, err := os.ReadFile(filepath.Join(evalsDir, "agents", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".kiro", "agents", name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	gitOut(t, wd, "add", ".kiro/agents")
	gitOut(t, wd, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "agent")
}

// makeTreeWritable lets t.TempDir cleanup remove read-only kept workspaces.
func makeTreeWritable(root string) {
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
}

type evalRunFiles struct {
	Summary map[string]any
	Agent   struct {
		Cases []struct {
			WorkspaceDir string `json:"workspace_dir"`
		} `json:"cases"`
	}
}

// newestRun returns the results dir of the only run not in seen.
func newestRun(t *testing.T, evalsDir string, seen map[string]bool) string {
	t.Helper()
	runs, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*", "summary.json"))
	var fresh []string
	for _, r := range runs {
		if !seen[r] {
			fresh = append(fresh, r)
			seen[r] = true
		}
	}
	if len(fresh) != 1 {
		t.Fatalf("new summary.json files = %v, want exactly 1", fresh)
	}
	return filepath.Dir(fresh[0])
}

func loadRun(t *testing.T, dir string) evalRunFiles {
	t.Helper()
	var r evalRunFiles
	b, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &r.Summary); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(filepath.Join(dir, "selftest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &r.Agent); err != nil {
		t.Fatal(err)
	}
	return r
}

// AC1-AC3 end to end: a baseline run and a candidate run with the stub backend.
func TestEvalPromptFileEndToEnd(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")
	commitSelftestAgent(t, evalsDir)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	livePrompt, err := os.ReadFile(filepath.Join(".kiro", "agents", "selftest-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}

	wsRoot := t.TempDir()
	t.Setenv("KAIRON_EVAL_WORKSPACE_ROOT", wsRoot)
	t.Cleanup(func() { makeTreeWritable(wsRoot) })

	candidate := filepath.Join(t.TempDir(), "candidate.md")
	candidateBody := []byte("# Candidate selftest\n\nA different prompt under evaluation.\n")
	if err := os.WriteFile(candidate, candidateBody, 0o644); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}

	// Baseline: no candidate.
	setEvalPromptFile(t, "", false)
	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("baseline run failed: %v", err)
	}
	baseline := loadRun(t, newestRun(t, evalsDir, seen))
	if _, ok := baseline.Summary["prompt_file"]; ok {
		t.Errorf("baseline summary has prompt_file = %v, want absent", baseline.Summary["prompt_file"])
	}
	baseSHA, _ := baseline.Summary["prompt_sha256"].(string)
	if baseSHA == "" {
		t.Fatal("baseline summary has no prompt_sha256")
	}

	// Run directories are named to the second; make sure the next one is distinct.
	time.Sleep(1100 * time.Millisecond)

	// Candidate run, keeping workspaces.
	setEvalPromptFile(t, candidate, true)
	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("candidate run failed: %v", err)
	}
	run := loadRun(t, newestRun(t, evalsDir, seen))

	// AC3: prompt_file and a different prompt_sha256.
	if got := run.Summary["prompt_file"]; got != filepath.ToSlash(candidate) {
		t.Errorf("summary prompt_file = %v, want %q", got, filepath.ToSlash(candidate))
	}
	if sha, _ := run.Summary["prompt_sha256"].(string); sha == "" || sha == baseSHA {
		t.Errorf("candidate prompt_sha256 = %q, baseline = %q; want non-empty and different", sha, baseSHA)
	}
	agents, _ := run.Summary["agents"].(map[string]any)
	if entry, _ := agents["selftest"].(map[string]any); entry["prompt_file"] != filepath.ToSlash(candidate) {
		t.Errorf("summary agents.selftest.prompt_file = %v, want %q", entry["prompt_file"], filepath.ToSlash(candidate))
	}

	// AC1: every kept workspace carries the candidate.
	if len(run.Agent.Cases) == 0 {
		t.Fatal("candidate run has no cases")
	}
	for _, c := range run.Agent.Cases {
		if c.WorkspaceDir == "" {
			t.Fatalf("case has no workspace_dir: %+v", run.Agent.Cases)
		}
		got, err := os.ReadFile(filepath.Join(c.WorkspaceDir, ".kiro", "agents", "selftest-prompt.md"))
		if err != nil {
			t.Fatalf("reading staged prompt in %s: %v", c.WorkspaceDir, err)
		}
		if string(got) != string(candidateBody) {
			t.Errorf("workspace %s prompt = %q, want candidate %q", c.WorkspaceDir, got, candidateBody)
		}
	}

	// AC2: the live agent is untouched, both on disk and in git.
	if st := gitOut(t, wd, "status", "--porcelain", ".kiro/agents"); st != "" {
		t.Errorf("git status --porcelain .kiro/agents = %q, want empty", st)
	}
	if got, _ := os.ReadFile(filepath.Join(".kiro", "agents", "selftest-prompt.md")); string(got) != string(livePrompt) {
		t.Errorf("live prompt changed: %q", got)
	}
}
