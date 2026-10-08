package eval

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

// wsEnv sets up a temp cwd with a project .kiro, an evals dir "evals" and a
// private workspace root, and returns the cwd.
func wsEnv(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cwd := chdirTemp(t)
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(workspaceRootEnv, root)

	writeCfgFile(t, ".kiro/agents/builder.json", `{"from":"project"}`)
	writeCfgFile(t, ".kiro/agents/other.json", `{"from":"project-other"}`)
	writeCfgFile(t, ".kiro/skills/s/SKILL.md", "skill")
	writeCfgFile(t, ".kiro/mcp.json", `{"mcpServers":{}}`)
	writeCfgFile(t, "evals/agents/builder.json", `{"from":"evals"}`)
	writeCfgFile(t, "evals/agents/extra.json", `{"from":"evals-extra"}`)
	return cwd
}

func wsGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = hermeticGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func readFileT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newWS(t *testing.T, tc TestCase) *caseWorkspace {
	t.Helper()
	ws, err := newCaseWorkspace(tc)
	if err != nil {
		t.Fatalf("newCaseWorkspace: %v", err)
	}
	t.Cleanup(func() { _ = ws.Remove() })
	return ws
}

func TestWorkspaceDefaultIsOneEmptyCommit(t *testing.T) {
	wsEnv(t)
	ws := newWS(t, TestCase{Name: "c"})

	if got := wsGit(t, ws.Dir, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("commit count = %s, want 1", got)
	}
	if got := wsGit(t, ws.Dir, "ls-tree", "-r", "HEAD"); got != "" {
		t.Fatalf("tree should be empty, got %q", got)
	}
	if got := wsGit(t, ws.Dir, "status", "--porcelain"); got != "" {
		t.Fatalf("status not clean: %q", got)
	}
	info, err := os.Stat(ws.EvalDir)
	if err != nil || !info.IsDir() {
		t.Fatalf(".eval missing: %v", err)
	}
}

func TestWorkspaceFixtureIsSingleCommitEqualToFixture(t *testing.T) {
	wsEnv(t)
	writeCfgFile(t, "evals/fixtures/workspaces/seeded/README.md", "hello\n")
	writeCfgFile(t, "evals/fixtures/workspaces/seeded/pkg/deep/file.txt", "deep\n")
	if err := os.WriteFile("evals/fixtures/workspaces/seeded/run.sh", []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	ws := newWS(t, TestCase{Name: "c", Workspace: "seeded"})

	if got := wsGit(t, ws.Dir, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("commit count = %s, want 1", got)
	}
	files := wsGit(t, ws.Dir, "ls-tree", "-r", "--name-only", "HEAD")
	want := "README.md\npkg/deep/file.txt\nrun.sh"
	if files != want {
		t.Fatalf("tree = %q, want %q", files, want)
	}
	if got := readFileT(t, filepath.Join(ws.Dir, "pkg/deep/file.txt")); got != "deep\n" {
		t.Fatalf("content = %q", got)
	}
	if got := wsGit(t, ws.Dir, "ls-tree", "HEAD", "run.sh"); !strings.HasPrefix(got, "100755") {
		t.Fatalf("exec bit lost: %q", got)
	}
	if got := wsGit(t, ws.Dir, "status", "--porcelain"); got != "" {
		t.Fatalf("status not clean: %q", got)
	}
}

func TestWorkspaceCommitIsHermetic(t *testing.T) {
	wsEnv(t)
	// A hostile global config must not leak in (identity, signing, hooks).
	gc := filepath.Join(t.TempDir(), "gitconfig")
	writeCfgFile(t, gc, "[commit]\n\tgpgsign = true\n[user]\n\tname = Someone Else\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)

	ws := newWS(t, TestCase{Name: "c"})
	if got := wsGit(t, ws.Dir, "log", "-1", "--format=%an <%ae>"); got != "kairon-eval <eval@kairon.invalid>" {
		t.Fatalf("author = %q", got)
	}
}

func TestWorkspaceNameValidation(t *testing.T) {
	wsEnv(t)
	writeCfgFile(t, "evals/fixtures/workspaces/ok/a.txt", "a")

	for _, name := range []string{"..", ".", "a/b", `a\b`, "../x", "/abs", "has space", "ok/"} {
		if err := validateWorkspaceName(name); err == nil {
			t.Errorf("validateWorkspaceName(%q) = nil, want error", name)
		}
		if _, err := newCaseWorkspace(TestCase{Name: "bad", Workspace: name}); err == nil {
			t.Errorf("newCaseWorkspace(%q) = nil error", name)
		} else if !strings.Contains(err.Error(), "bad") {
			t.Errorf("error for %q does not name the case: %v", name, err)
		}
	}
	for _, name := range []string{"", "ok", "my-fixture_1.v2"} {
		if err := validateWorkspaceName(name); err != nil {
			t.Errorf("validateWorkspaceName(%q) = %v", name, err)
		}
	}

	_, err := newCaseWorkspace(TestCase{Name: "gone", Workspace: "missing"})
	if err == nil || !strings.Contains(err.Error(), "gone") || !strings.Contains(err.Error(), filepath.Join("evals", "fixtures", "workspaces", "missing")) {
		t.Fatalf("missing fixture error should name case and path, got %v", err)
	}
}

func TestLoadCasesValidatesWorkspaceAndTimeout(t *testing.T) {
	wsEnv(t)
	writeCfgFile(t, "evals/fixtures/workspaces/ok/a.txt", "a")

	cases := []struct {
		name    string
		yaml    string
		wantErr string // empty = success
	}{
		{"valid", "name: good\nworkspace: ok\ntimeout: 1s\ninput: x\n", ""},
		{"no-fields", "name: plain\ninput: x\n", ""},
		{"escape", "name: esc\nworkspace: ../ok\ninput: x\n", "esc"},
		{"dotdot", "name: dd\nworkspace: ..\ninput: x\n", "dd"},
		{"missing", "name: miss\nworkspace: nope\ninput: x\n", "miss"},
		{"bad-timeout", "name: badt\ntimeout: soon\ninput: x\n", "badt"},
		{"zero-timeout", "name: zero\ntimeout: 0s\ninput: x\n", "zero"},
		{"negative-timeout", "name: neg\ntimeout: -5s\ninput: x\n", "neg"},
		{"bare-number", "name: bare\ntimeout: 30\ninput: x\n", "bare"},
		{"gh-issue-ok", "name: ghok\nrequires_sandbox: true\ngh_issue:\n  title: T\ninput: x\n", ""},
		{"gh-issue-no-sandbox", "name: ghns\ngh_issue:\n  title: T\ninput: x\n", "requires_sandbox"},
		{"gh-issue-no-title", "name: ghnt\nrequires_sandbox: true\ngh_issue:\n  number: 3\ninput: x\n", "title"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.Join("evals", "cases", "agent-"+c.name)
			writeCfgFile(t, filepath.Join(dir, "c.yaml"), c.yaml)
			got, err := loadCases("agent-" + c.name)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != 1 {
					t.Fatalf("got %d cases", len(got))
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want it to name %q", err, c.wantErr)
			}
		})
	}

	got, err := loadCases("agent-valid")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Workspace != "ok" || got[0].Timeout != "1s" {
		t.Fatalf("fields not loaded: %+v", got[0])
	}
}

// TestLoadCasesRejectsFixtureAgentOverride verifies that a workspace fixture
// providing the case's own agent config is refused at load, because provenance
// is pinned from <evals-dir>/agents or .kiro/agents and would not describe the
// fixture-provided config that actually ran.
func TestLoadCasesRejectsFixtureAgentOverride(t *testing.T) {
	wsEnv(t)
	// Fixture that overrides the agent config for agent "agent-fixoverride".
	writeCfgFile(t, "evals/fixtures/workspaces/ovr/.kiro/agents/agent-fixoverride.json", `{"name":"x"}`)
	writeCfgFile(t, filepath.Join("evals", "cases", "agent-fixoverride", "c.yaml"),
		"name: ovr-case\nworkspace: ovr\ninput: x\n")

	_, err := loadCases("agent-fixoverride")
	if err == nil || !strings.Contains(err.Error(), "ovr-case") {
		t.Fatalf("error = %v, want it to reject the fixture agent override and name the case", err)
	}

	// A fixture overriding a DIFFERENT agent's config (not the case's agent) or
	// non-agent .kiro content is fine.
	writeCfgFile(t, "evals/fixtures/workspaces/ok2/.kiro/agents/some-other.json", `{"name":"y"}`)
	writeCfgFile(t, filepath.Join("evals", "cases", "agent-fixok", "c.yaml"),
		"name: ok-case\nworkspace: ok2\ninput: x\n")
	if _, err := loadCases("agent-fixok"); err != nil {
		t.Fatalf("fixture overriding a different agent should be allowed, got: %v", err)
	}
}

func TestWorkspaceExcludesHarnessPathsFromStatus(t *testing.T) {
	wsEnv(t)
	ws := newWS(t, TestCase{Name: "c"})

	writeCfgFile(t, filepath.Join(ws.EvalDir, "out.txt"), "output")
	if got := wsGit(t, ws.Dir, "status", "--porcelain"); got != "" {
		t.Fatalf("harness paths leaked into status: %q", got)
	}
	// A regular file still shows up.
	writeCfgFile(t, filepath.Join(ws.Dir, "real.txt"), "x")
	if got := wsGit(t, ws.Dir, "status", "--porcelain"); got != "?? real.txt" {
		t.Fatalf("status = %q", got)
	}
}

func TestWorkspaceFixtureTrackedKiroStaysTracked(t *testing.T) {
	wsEnv(t)
	writeCfgFile(t, "evals/fixtures/workspaces/k/.kiro/tracked.md", "tracked")
	writeCfgFile(t, "evals/fixtures/workspaces/k/code.go", "package k")

	ws := newWS(t, TestCase{Name: "c", Workspace: "k"})

	if got := wsGit(t, ws.Dir, "ls-files"); got != ".kiro/tracked.md\ncode.go" {
		t.Fatalf("tracked files = %q", got)
	}
	// Staged, untracked project files are hidden; tracked ones stay clean.
	if got := wsGit(t, ws.Dir, "status", "--porcelain"); got != "" {
		t.Fatalf("status = %q", got)
	}
	// Modifying a tracked .kiro file is visible.
	if err := os.Chmod(filepath.Join(ws.KiroDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(ws.KiroDir, "tracked.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCfgFile(t, filepath.Join(ws.KiroDir, "tracked.md"), "changed")
	if got := wsGit(t, ws.Dir, "status", "--porcelain"); got != "M .kiro/tracked.md" && got != " M .kiro/tracked.md" {
		t.Fatalf("tracked .kiro edit not reported: %q", got)
	}
}

func TestWorkspaceKiroStagingPrecedence(t *testing.T) {
	cwd := wsEnv(t)
	// Fixture overrides one agent file from both lower layers and adds a skill.
	writeCfgFile(t, "evals/fixtures/workspaces/f/.kiro/agents/builder.json", `{"from":"fixture"}`)
	writeCfgFile(t, "evals/fixtures/workspaces/f/.kiro/skills/s/SKILL.md", "fixture skill")

	before := snapshotTree(t, filepath.Join(cwd, ".kiro"))
	evalsBefore := snapshotTree(t, filepath.Join(cwd, "evals"))

	ws := newWS(t, TestCase{Name: "c", Workspace: "f"})
	agents := filepath.Join(ws.KiroDir, "agents")

	if got := readFileT(t, filepath.Join(agents, "builder.json")); got != `{"from":"fixture"}` {
		t.Errorf("fixture must win, builder.json = %q", got)
	}
	if got := readFileT(t, filepath.Join(agents, "extra.json")); got != `{"from":"evals-extra"}` {
		t.Errorf("evals agent missing, extra.json = %q", got)
	}
	if got := readFileT(t, filepath.Join(agents, "other.json")); got != `{"from":"project-other"}` {
		t.Errorf("project agent missing, other.json = %q", got)
	}
	if got := readFileT(t, filepath.Join(ws.KiroDir, "skills/s/SKILL.md")); got != "fixture skill" {
		t.Errorf("fixture skill must win, got %q", got)
	}
	if got := readFileT(t, filepath.Join(ws.KiroDir, "mcp.json")); got != `{"mcpServers":{}}` {
		t.Errorf("mcp config not staged: %q", got)
	}

	assertTreeEqual(t, before, snapshotTree(t, filepath.Join(cwd, ".kiro")), "project .kiro")
	assertTreeEqual(t, evalsBefore, snapshotTree(t, filepath.Join(cwd, "evals")), "evals dir")
}

func TestWorkspaceKiroStagingEvalsOverridesProject(t *testing.T) {
	wsEnv(t)
	ws := newWS(t, TestCase{Name: "c"})
	agents := filepath.Join(ws.KiroDir, "agents")

	if got := readFileT(t, filepath.Join(agents, "builder.json")); got != `{"from":"evals"}` {
		t.Errorf("evals agents must override project, got %q", got)
	}
	if got := readFileT(t, filepath.Join(agents, "other.json")); got != `{"from":"project-other"}` {
		t.Errorf("other.json = %q", got)
	}
	if got := readFileT(t, filepath.Join(ws.KiroDir, "skills/s/SKILL.md")); got != "skill" {
		t.Errorf("skill = %q", got)
	}
}

func TestWorkspaceWithoutProjectKiroOrEvalAgents(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	chdirTemp(t)
	t.Setenv(workspaceRootEnv, t.TempDir())
	ws := newWS(t, TestCase{Name: "c"})
	if _, err := os.Stat(ws.KiroDir); err != nil {
		t.Fatalf(".kiro should exist even when empty: %v", err)
	}
}

func TestWorkspacePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	wsEnv(t)
	// Restrictive modes in the fixture must still end up open.
	writeCfgFile(t, "evals/fixtures/workspaces/p/secret.txt", "s")
	writeCfgFile(t, "evals/fixtures/workspaces/p/sub/inner.txt", "i")
	if err := os.Chmod("evals/fixtures/workspaces/p/secret.txt", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("evals/fixtures/workspaces/p/sub", 0o700); err != nil {
		t.Fatal(err)
	}
	ws := newWS(t, TestCase{Name: "c", Workspace: "p"})

	err := filepath.WalkDir(ws.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		inKiro := p == ws.KiroDir || strings.HasPrefix(p, ws.KiroDir+string(filepath.Separator))
		if inKiro {
			if perm&0o444 != 0o444 {
				t.Errorf("%s: mode %o not world readable", p, perm)
			}
			if perm&0o022 != 0 {
				t.Errorf("%s: mode %o must not be group/other writable", p, perm)
			}
			if info.IsDir() && perm&0o111 != 0o111 {
				t.Errorf("%s: dir mode %o not traversable", p, perm)
			}
			return nil
		}
		if perm&0o666 != 0o666 {
			t.Errorf("%s: mode %o is not world read/write", p, perm)
		}
		if info.IsDir() && perm&0o777 != 0o777 {
			t.Errorf("%s: dir mode %o is not a+rwx", p, perm)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Checked before any git command: git may rewrite .git/index (it is
	// replaced via rename, which the world-writable .git directory allows).
	for _, rel := range []string{".git", ".git/objects", ".git/HEAD", ".git/index"} {
		info, err := os.Stat(filepath.Join(ws.Dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o666 != 0o666 {
			t.Errorf("%s: mode %o is not world read/write", rel, info.Mode().Perm())
		}
	}

	// The tree can still be committed to (what another uid with these modes
	// would see), and mode changes leave status clean.
	if got := wsGit(t, ws.Dir, "status", "--porcelain"); got != "" {
		t.Fatalf("status after chmod = %q", got)
	}
	writeCfgFile(t, filepath.Join(ws.Dir, "new.txt"), "n")
	wsGit(t, ws.Dir, "add", "new.txt")
	wsGit(t, ws.Dir, "-c", "user.name=u", "-c", "user.email=u@x", "commit", "-q", "-m", "second")
	if got := wsGit(t, ws.Dir, "rev-list", "--count", "HEAD"); got != "2" {
		t.Fatalf("commit count after agent commit = %s", got)
	}
}

func TestWorkspaceDirAbsoluteAndSymlinkResolved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	wsEnv(t)
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workspaceRootEnv, link)

	ws := newWS(t, TestCase{Name: "c"})
	if !filepath.IsAbs(ws.Dir) {
		t.Fatalf("Dir not absolute: %s", ws.Dir)
	}
	resolved, err := filepath.EvalSymlinks(ws.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != ws.Dir {
		t.Fatalf("Dir not symlink-resolved: %s vs %s", ws.Dir, resolved)
	}
	realResolved, _ := filepath.EvalSymlinks(real)
	if !strings.HasPrefix(ws.Dir, realResolved+string(filepath.Separator)) {
		t.Fatalf("KAIRON_EVAL_WORKSPACE_ROOT not honored: %s not under %s", ws.Dir, realResolved)
	}
	if ws.EvalDir != filepath.Join(ws.Dir, ".eval") || ws.KiroDir != filepath.Join(ws.Dir, ".kiro") {
		t.Fatalf("unexpected sub paths: %s %s", ws.EvalDir, ws.KiroDir)
	}
}

func TestWorkspaceRelativeRootBecomesAbsolute(t *testing.T) {
	wsEnv(t)
	t.Setenv(workspaceRootEnv, "rel-root")
	ws := newWS(t, TestCase{Name: "c"})
	if !filepath.IsAbs(ws.Dir) {
		t.Fatalf("Dir not absolute: %s", ws.Dir)
	}
}

func TestWorkspaceDefaultsToOSTempDir(t *testing.T) {
	wsEnv(t)
	t.Setenv(workspaceRootEnv, "")
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ws := newWS(t, TestCase{Name: "c"})
	if !strings.HasPrefix(ws.Dir, tmp+string(filepath.Separator)) {
		t.Fatalf("%s not under %s", ws.Dir, tmp)
	}
}

func TestWorkspaceRemove(t *testing.T) {
	wsEnv(t)
	ws, err := newCaseWorkspace(TestCase{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists: %v", err)
	}
	if _, err := os.Stat(ws.root); !os.IsNotExist(err) {
		t.Fatalf("workspace root still exists: %v", err)
	}
	// Idempotent, and nil-safe.
	if err := ws.Remove(); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
	var nilWS *caseWorkspace
	if err := nilWS.Remove(); err != nil {
		t.Fatalf("nil Remove: %v", err)
	}
}

func TestWorkspaceRemoveToleratesUnreadableDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	wsEnv(t)
	ws, err := newCaseWorkspace(TestCase{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(ws.Dir, "locked")
	writeCfgFile(t, filepath.Join(locked, "f.txt"), "x")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	if err := ws.Remove(); err != nil {
		t.Fatalf("Remove must not fail a run: %v", err)
	}
	if _, err := os.Stat(ws.root); !os.IsNotExist(err) {
		t.Fatalf("workspace not removed: %v", err)
	}
}

func TestWorkspaceRemoveSwallowsFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	wsEnv(t)
	ws, err := newCaseWorkspace(TestCase{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	// Make the parent non-writable so the root cannot be unlinked.
	parent := filepath.Dir(ws.root)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0o755)
		_ = ws.Remove()
	})
	if err := ws.Remove(); err != nil {
		t.Fatalf("Remove returned %v; it must never fail a run", err)
	}
}

func TestWorkspaceCreationFailureLeavesNothingBehind(t *testing.T) {
	wsEnv(t)
	root := os.Getenv(workspaceRootEnv)
	// A fixture name that passes validation but is a file, not a directory.
	writeCfgFile(t, "evals/fixtures/workspaces/notadir", "file")
	if _, err := newCaseWorkspace(TestCase{Name: "c", Workspace: "notadir"}); err == nil {
		t.Fatal("expected error for non-directory fixture")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("leftover workspace entries: %v", entries)
	}
}

func TestWorkspacesAreIndependent(t *testing.T) {
	wsEnv(t)
	a := newWS(t, TestCase{Name: "a"})
	b := newWS(t, TestCase{Name: "b"})
	if a.Dir == b.Dir {
		t.Fatal("workspaces share a directory")
	}
}

func TestCaseResultWorkspaceDirJSON(t *testing.T) {
	data, err := json.Marshal(CaseResult{CaseName: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "workspace_dir") {
		t.Fatalf("workspace_dir must be omitted when empty: %s", data)
	}

	data, err = json.Marshal(CaseResult{CaseName: "c", WorkspaceDir: "/tmp/ws"})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["workspace_dir"] != "/tmp/ws" {
		t.Fatalf("workspace_dir = %v in %s", raw["workspace_dir"], data)
	}

	// Result files written before the field existed still decode.
	var old CaseResult
	if err := json.Unmarshal([]byte(`{"case_name":"c","actual_output":"o","scores":[],"agent_cost":{},"judge_cost":{}}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.WorkspaceDir != "" || old.CaseName != "c" {
		t.Fatalf("decoded = %+v", old)
	}
}

func TestRunOptionsKeepWorkspacesField(t *testing.T) {
	if !(RunOptions{KeepWorkspaces: true}).KeepWorkspaces {
		t.Fatal("KeepWorkspaces not settable")
	}
}

// snapshotTree maps relative file paths to their contents and modes.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[rel+"/"] = info.Mode().Perm().String()
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = info.Mode().Perm().String() + ":" + string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertTreeEqual(t *testing.T, want, got map[string]string, what string) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("%s changed: %d entries before, %d after", what, len(want), len(got))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %s changed (%q -> %q)", what, k, v, got[k])
		}
	}
}

func TestLoadCasesGHIssueDefaultsNumber(t *testing.T) {
	wsEnv(t)
	writeCfgFile(t, filepath.Join("evals", "cases", "agent-gh", "c.yaml"),
		"name: gh\nrequires_sandbox: true\ngh_issue:\n  title: T\n  body: B\n  labels: [bug]\ninput: x\n")
	writeCfgFile(t, filepath.Join("evals", "cases", "agent-gh", "d.yaml"),
		"name: gh2\nrequires_sandbox: true\ngh_issue:\n  number: 42\n  title: T\ninput: x\n")
	got, err := loadCases("agent-gh")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d cases", len(got))
	}
	byName := map[string]TestCase{}
	for _, c := range got {
		byName[c.Name] = c
	}
	if c := byName["gh"]; c.GHIssue == nil || c.GHIssue.Number != 1 || c.GHIssue.Title != "T" || !c.RequiresSandbox {
		t.Errorf("defaults not applied: %+v", c.GHIssue)
	}
	if c := byName["gh2"]; c.GHIssue == nil || c.GHIssue.Number != 42 {
		t.Errorf("explicit number lost: %+v", c.GHIssue)
	}
}

func TestNewCaseWorkspaceWritesFakeGHAndIssue(t *testing.T) {
	wsEnv(t)

	// Without gh_issue: fake gh is present, issue files are not.
	plain := newWS(t, TestCase{Name: "plain"})
	info, err := os.Stat(filepath.Join(plain.BinDir, "gh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("BinDir/gh mode = %v, want executable", info.Mode().Perm())
	}
	if filepath.Dir(plain.BinDir) != plain.root || strings.HasPrefix(plain.BinDir, plain.Dir+string(filepath.Separator)) {
		t.Errorf("BinDir %s must be a sibling of the workspace, not inside it", plain.BinDir)
	}
	for _, n := range []string{"gh-issue.json", "gh-issue.txt"} {
		if _, err := os.Stat(filepath.Join(plain.EvalDir, n)); !os.IsNotExist(err) {
			t.Errorf("%s must not exist without gh_issue (err=%v)", n, err)
		}
	}

	// With gh_issue: both rendered, JSON one top-level field per line.
	ws := newWS(t, TestCase{Name: "gh", RequiresSandbox: true, GHIssue: &sandbox.GHIssue{Title: "Add widget", Body: "b"}})
	raw := readFileT(t, filepath.Join(ws.EvalDir, "gh-issue.json"))
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("gh-issue.json is not valid JSON: %v\n%s", err, raw)
	}
	if m["title"] != "Add widget" || m["number"] != float64(1) {
		t.Errorf("rendered issue = %v", m)
	}
	if !strings.Contains(readFileT(t, filepath.Join(ws.EvalDir, "gh-issue.txt")), "Add widget") {
		t.Error("gh-issue.txt lacks the title")
	}

	// A gh_issue without a title cannot be rendered.
	if _, err := newCaseWorkspace(TestCase{Name: "bad", GHIssue: &sandbox.GHIssue{}}); err == nil {
		t.Error("expected an error for gh_issue without title")
	}
}

func TestRemoveDeletesBinDir(t *testing.T) {
	wsEnv(t)
	ws := newWS(t, TestCase{Name: "rm"})
	bin := ws.BinDir
	_ = ws.Remove()
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Errorf("BinDir survived Remove: %v", err)
	}
}
