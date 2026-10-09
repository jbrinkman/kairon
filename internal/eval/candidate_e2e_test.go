package eval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// End-to-end tests for `kairon eval <agent> --prompt-file` using the stub
// backend. They need neither kiro-cli on PATH nor a container daemon: every
// run is native (NoSandbox) and the stub backend never starts an agent.

const (
	e2eAgent      = "selftest"
	e2ePromptName = "selftest-prompt.md"
	e2eCandidate  = "# Candidate prompt (issue 318 e2e)\n\nAnswer concisely with ## and ### headings.\n"
)

// candidateE2EEnv is an isolated project: a temp git repo (the working
// directory) holding a committed .kiro/agents, plus a separate evals dir copied
// from the self-test fixtures.
type candidateE2EEnv struct {
	repo     string // working directory, a git repo with a clean .kiro/agents
	fixtures string // absolute path of the checked-in self-test fixtures
	evalsDir string // absolute path of the copied fixtures
	wsRoot   string // workspace root, so kept workspaces never reach os.TempDir
}

func e2eGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "user.name=kairon-test", "-c", "user.email=kairon-test@example.invalid",
		"-c", "commit.gpgsign=false",
	}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// newCandidateE2EEnv builds a project with one committed live agent under
// .kiro/agents and a fresh copy of the fixtures, and chdirs into the project.
func newCandidateE2EEnv(t *testing.T) candidateE2EEnv {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	fixtures, err := filepath.Abs(selftestEvalsDir)
	if err != nil {
		t.Fatal(err)
	}
	repo := chdirTemp(t) // also restores cwd and cfg afterwards
	repo, err = filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	writeProvFile(t, filepath.Join(repo, ".kiro", "agents", "live.json"),
		`{"name":"live","prompt":"file://./live-prompt.md","model":"claude-sonnet-5.5"}`)
	writeProvFile(t, filepath.Join(repo, ".kiro", "agents", "live-prompt.md"), "live agent prompt\n")
	e2eGit(t, repo, "init", "-q")
	e2eGit(t, repo, "add", ".kiro")
	e2eGit(t, repo, "commit", "-q", "-m", "live agents")

	env := candidateE2EEnv{repo: repo, fixtures: fixtures, evalsDir: newE2EEvalsDir(t, fixtures), wsRoot: t.TempDir()}
	t.Setenv(workspaceRootEnv, env.wsRoot)
	return env
}

// newE2EEvalsDir returns a fresh, absolute copy of the self-test fixtures at
// src (an absolute path; the working directory is the temp project by now).
func newE2EEvalsDir(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "evals")
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dst, "results")); err != nil {
		t.Fatal(err)
	}
	return dst
}

// writeE2ECandidate writes content to a fresh temp file and returns its path.
func writeE2ECandidate(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "candidate.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// treeSnapshot maps the slash path of every regular file under root to its
// sha256. A missing root yields an empty snapshot.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == root {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		snap[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotting %s: %v", root, err)
	}
	return snap
}

func assertSnapshotsEqual(t *testing.T, label string, before, after map[string]string) {
	t.Helper()
	var names []string
	seen := map[string]bool{}
	for k := range before {
		seen[k] = true
		names = append(names, k)
	}
	for k := range after {
		if !seen[k] {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		b, bok := before[n]
		a, aok := after[n]
		switch {
		case bok && !aok:
			t.Errorf("%s: %s was removed", label, n)
		case !bok && aok:
			t.Errorf("%s: %s was added", label, n)
		case a != b:
			t.Errorf("%s: %s changed (sha256 %s -> %s)", label, n, b, a)
		}
	}
}

// removeKeptWorkspace removes a kept workspace's private parent at test end.
// The staged .kiro is read-only, so directories are made writable first.
func removeKeptWorkspace(t *testing.T, c CaseResult) {
	t.Helper()
	if c.WorkspaceDir == "" {
		return
	}
	parent := filepath.Dir(c.WorkspaceDir)
	t.Cleanup(func() {
		_ = filepath.WalkDir(parent, func(p string, d fs.DirEntry, err error) error {
			if d != nil && d.IsDir() {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
		if err := os.RemoveAll(parent); err != nil {
			t.Errorf("removing kept workspace %s: %v", parent, err)
		}
		if _, err := os.Stat(parent); !os.IsNotExist(err) {
			t.Errorf("kept workspace %s still exists after cleanup (stat err = %v)", parent, err)
		}
	})
}

// e2eRun is the output of one RunWithOptions call.
type e2eRun struct {
	dir     string // <evals-dir>/results/<run>
	agent   AgentResult
	rawAgnt map[string]json.RawMessage // raw <agent>.json, to assert on key presence
	summary Summary
	rawSum  map[string]json.RawMessage // raw summary.json
	rawSumA map[string]json.RawMessage // raw summary.json agents.<agent>
}

func readRawJSON(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return raw
}

// runE2E performs one stub-backend run of the selftest agent against
// evalsDir and loads the single run directory it produced.
func runE2E(t *testing.T, evalsDir string, opts RunOptions) e2eRun {
	t.Helper()
	resultsDir := filepath.Join(evalsDir, "results")
	before := snapshotDirs(t, resultsDir)

	opts.Backend = "stub"
	opts.NoSandbox = true
	opts.EvalsDir = evalsDir
	if err := RunWithOptions(e2eAgent, "", opts); err != nil {
		t.Fatalf("stub run (prompt-file=%q keep=%v) failed: %v", opts.PromptFile, opts.KeepWorkspaces, err)
	}
	added := newRunDirs(t, resultsDir, before)
	if len(added) != 1 {
		t.Fatalf("run created %d result dirs, want 1: %v", len(added), added)
	}
	dir := filepath.Join(resultsDir, added[0])

	run := e2eRun{dir: dir}
	run.agent = readSelfTestResult(t, filepath.Join(dir, e2eAgent+".json"))
	run.rawAgnt = readRawJSON(t, filepath.Join(dir, e2eAgent+".json"))
	run.rawSum = readRawJSON(t, filepath.Join(dir, "summary.json"))
	sumData, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sumData, &run.summary); err != nil {
		t.Fatalf("parsing summary.json: %v", err)
	}
	var agents map[string]map[string]json.RawMessage
	if raw, ok := run.rawSum["agents"]; ok {
		if err := json.Unmarshal(raw, &agents); err != nil {
			t.Fatalf("parsing summary.json agents: %v", err)
		}
	}
	run.rawSumA = agents[e2eAgent]
	return run
}

// liveEvalsPrompt returns the bytes of the prompt the selftest config points
// at (the evals-dir overlay copy).
func liveEvalsPrompt(t *testing.T, evalsDir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(evalsDir, "agents", e2ePromptName))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestCandidateE2EKeptWorkspacesHoldCandidate is AC1: with --keep-workspaces
// every case's workspace has the candidate bytes at the agent's prompt path,
// and the staged file is read-only like the rest of .kiro.
func TestCandidateE2EKeptWorkspacesHoldCandidate(t *testing.T) {
	env := newCandidateE2EEnv(t)
	t.Setenv("PATH", pathWithoutKiroCLI(t)) // no kiro-cli anywhere
	cand := writeE2ECandidate(t, e2eCandidate)

	live := liveEvalsPrompt(t, env.evalsDir)
	if bytes.Equal(live, []byte(e2eCandidate)) {
		t.Fatal("test candidate must differ from the live prompt")
	}

	run := runE2E(t, env.evalsDir, RunOptions{PromptFile: cand, KeepWorkspaces: true})
	for _, c := range run.agent.Cases {
		removeKeptWorkspace(t, c)
	}
	// Guard against a vacuous pass: the per-case loop below only proves
	// anything if the run actually produced cases. The exact count is
	// deliberately not asserted so adding selftest cases never breaks this.
	if len(run.agent.Cases) == 0 {
		t.Fatal("selftest run produced no cases; the per-case checks below would pass vacuously")
	}

	for _, c := range run.agent.Cases {
		if c.WorkspaceDir == "" {
			t.Errorf("case %s: no workspace_dir (workspaces were not kept)", c.CaseName)
			continue
		}
		staged := filepath.Join(c.WorkspaceDir, ".kiro", "agents", e2ePromptName)
		got, err := os.ReadFile(staged)
		if err != nil {
			t.Errorf("case %s: reading staged prompt: %v", c.CaseName, err)
			continue
		}
		if string(got) != e2eCandidate {
			t.Errorf("case %s: staged prompt = %q, want the candidate %q", c.CaseName, got, e2eCandidate)
		}
		info, err := os.Stat(staged)
		if err == nil && info.Mode().Perm()&0o022 != 0 {
			t.Errorf("case %s: staged prompt mode %v is group/other-writable", c.CaseName, info.Mode().Perm())
		}
	}
}

// TestCandidateE2ELiveAgentsUnchanged is AC2: a candidate run leaves the
// project's .kiro/agents tree and the evals-dir agents/ directory byte for
// byte identical, and git sees no change under .kiro/agents.
func TestCandidateE2ELiveAgentsUnchanged(t *testing.T) {
	env := newCandidateE2EEnv(t)
	cand := writeE2ECandidate(t, e2eCandidate)

	projectAgents := filepath.Join(env.repo, ".kiro", "agents")
	evalsAgents := filepath.Join(env.evalsDir, "agents")

	if out := e2eGit(t, env.repo, "status", "--porcelain", ".kiro/agents"); out != "" {
		t.Fatalf("git status --porcelain .kiro/agents not clean before the run:\n%s", out)
	}
	projBefore := treeSnapshot(t, projectAgents)
	evalsBefore := treeSnapshot(t, evalsAgents)
	if len(projBefore) == 0 || len(evalsBefore) == 0 {
		t.Fatalf("empty snapshots (project %d files, evals %d files)", len(projBefore), len(evalsBefore))
	}

	run := runE2E(t, env.evalsDir, RunOptions{PromptFile: cand, KeepWorkspaces: true})
	for _, c := range run.agent.Cases {
		removeKeptWorkspace(t, c)
	}
	if run.agent.PromptFile == "" {
		t.Fatal("candidate run did not record prompt_file; the run did not use the candidate")
	}

	assertSnapshotsEqual(t, "project .kiro/agents", projBefore, treeSnapshot(t, projectAgents))
	assertSnapshotsEqual(t, "evals-dir agents/", evalsBefore, treeSnapshot(t, evalsAgents))
	if out := e2eGit(t, env.repo, "status", "--porcelain", ".kiro/agents"); out != "" {
		t.Errorf("git status --porcelain .kiro/agents not clean after the run:\n%s", out)
	}
	if out := e2eGit(t, env.repo, "status", "--porcelain"); out != "" {
		t.Errorf("git status --porcelain not clean after the run (the candidate must not be written into the repo):\n%s", out)
	}
}

// TestCandidateE2EProvenanceRecorded is AC3: the baseline run has no
// prompt_file key; the candidate run records prompt_file and a different
// prompt_sha256, in <agent>.json and summary.json (agents.<agent> and, for a
// single-agent run, the top level).
func TestCandidateE2EProvenanceRecorded(t *testing.T) {
	env := newCandidateE2EEnv(t)
	cand := writeE2ECandidate(t, e2eCandidate)

	base := runE2E(t, env.evalsDir, RunOptions{})
	// A separate fixtures copy, so each run dir is the only one under its
	// evals dir.
	with := runE2E(t, newE2EEvalsDir(t, env.fixtures), RunOptions{PromptFile: cand})

	// Baseline: no prompt_file key anywhere.
	for label, raw := range map[string]map[string]json.RawMessage{
		"baseline selftest.json":                base.rawAgnt,
		"baseline summary.json":                 base.rawSum,
		"baseline summary.json agents.selftest": base.rawSumA,
	} {
		if raw == nil {
			t.Errorf("%s: missing", label)
			continue
		}
		if _, ok := raw["prompt_file"]; ok {
			t.Errorf("%s: has a prompt_file key, want none for a run without a candidate", label)
		}
	}

	want := filepath.ToSlash(filepath.Clean(cand))
	if with.agent.PromptFile != want {
		t.Errorf("selftest.json prompt_file = %q, want %q", with.agent.PromptFile, want)
	}
	if with.summary.PromptFile != want {
		t.Errorf("summary.json prompt_file = %q, want %q", with.summary.PromptFile, want)
	}
	if got := with.summary.Agents[e2eAgent].PromptFile; got != want {
		t.Errorf("summary.json agents.selftest.prompt_file = %q, want %q", got, want)
	}
	for label, raw := range map[string]map[string]json.RawMessage{
		"candidate selftest.json":                with.rawAgnt,
		"candidate summary.json":                 with.rawSum,
		"candidate summary.json agents.selftest": with.rawSumA,
	} {
		if _, ok := raw["prompt_file"]; !ok {
			t.Errorf("%s: no prompt_file key", label)
		}
	}

	if !hex64.MatchString(base.agent.PromptSHA256) {
		t.Fatalf("baseline prompt_sha256 = %q, want 64 hex chars", base.agent.PromptSHA256)
	}
	if with.agent.PromptSHA256 == base.agent.PromptSHA256 {
		t.Errorf("selftest.json prompt_sha256 equals the baseline (%s) for a different candidate", base.agent.PromptSHA256)
	}
	if got := with.summary.Agents[e2eAgent].PromptSHA256; got != with.agent.PromptSHA256 {
		t.Errorf("summary agents.selftest.prompt_sha256 = %q, want %q (selftest.json)", got, with.agent.PromptSHA256)
	}
	if got := base.summary.Agents[e2eAgent].PromptSHA256; got != base.agent.PromptSHA256 {
		t.Errorf("baseline summary agents.selftest.prompt_sha256 = %q, want %q", got, base.agent.PromptSHA256)
	}
	if with.summary.Agents[e2eAgent].PromptSHA256 == base.summary.Agents[e2eAgent].PromptSHA256 {
		t.Error("summary.json prompt_sha256 equals the baseline for a different candidate")
	}
	if with.summary.PromptSHA256 == base.summary.PromptSHA256 {
		t.Error("summary.json top-level prompt_sha256 equals the baseline for a different candidate")
	}
}

// TestCandidateE2ESameContentCandidateHashEqualsBaseline: a candidate with
// the same bytes as the live prompt hashes identically to the baseline, while
// still recording prompt_file. Provenance keys on content, not on the path.
func TestCandidateE2ESameContentCandidateHashEqualsBaseline(t *testing.T) {
	env := newCandidateE2EEnv(t)
	same := writeE2ECandidate(t, string(liveEvalsPrompt(t, env.evalsDir)))

	base := runE2E(t, env.evalsDir, RunOptions{})
	with := runE2E(t, newE2EEvalsDir(t, env.fixtures), RunOptions{PromptFile: same})

	if with.agent.PromptSHA256 != base.agent.PromptSHA256 {
		t.Errorf("same-content candidate prompt_sha256 = %s, want the baseline %s",
			with.agent.PromptSHA256, base.agent.PromptSHA256)
	}
	if got := with.summary.Agents[e2eAgent].PromptSHA256; got != base.summary.Agents[e2eAgent].PromptSHA256 {
		t.Errorf("summary same-content candidate prompt_sha256 = %s, want the baseline %s",
			got, base.summary.Agents[e2eAgent].PromptSHA256)
	}
	if with.agent.PromptFile == "" {
		t.Error("same-content candidate run did not record prompt_file")
	}
	if with.summary.Agents[e2eAgent].PromptFile == "" {
		t.Error("same-content candidate summary did not record agents.selftest.prompt_file")
	}
}

// TestCandidateE2EResumeRefusedWithoutCandidate: resuming an interrupted
// candidate run without the candidate is refused, naming the recorded
// prompt_file, and does not modify the saved results. Resuming with the same
// candidate is accepted.
func TestCandidateE2EResumeRefusedWithoutCandidate(t *testing.T) {
	env := newCandidateE2EEnv(t)
	cand := writeE2ECandidate(t, e2eCandidate)

	run := runE2E(t, env.evalsDir, RunOptions{PromptFile: cand})
	// Mark the finished run as interrupted so --resume picks it up.
	if err := os.WriteFile(filepath.Join(run.dir, ".progress"), []byte("interrupted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agentFile := filepath.Join(run.dir, e2eAgent+".json")
	savedBefore, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatal(err)
	}

	err = RunWithOptions(e2eAgent, "", RunOptions{
		Backend: "stub", NoSandbox: true, EvalsDir: env.evalsDir, Resume: true,
	})
	if err == nil {
		t.Fatal("resuming a candidate run without the candidate succeeded, want a refusal")
	}
	want := filepath.ToSlash(filepath.Clean(cand))
	for _, s := range []string{"cannot resume", "prompt_file=" + want, "prompt_file=(none)"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("refusal %q does not contain %q", err, s)
		}
	}
	savedAfter, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(savedBefore, savedAfter) {
		t.Error("refused resume modified the saved selftest.json")
	}

	// Positive control: the same candidate resumes.
	if err := RunWithOptions(e2eAgent, "", RunOptions{
		Backend: "stub", NoSandbox: true, EvalsDir: env.evalsDir, Resume: true, PromptFile: cand,
	}); err != nil {
		t.Errorf("resuming with the same candidate was refused: %v", err)
	}
}
