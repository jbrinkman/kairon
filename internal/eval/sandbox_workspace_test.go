package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

// workspaceCaseNames are the self-test cases that exercise the host-built
// per-case workspace: the two `selftest` cases plus the `selftest-fail`
// timeout case.
const (
	markerCase  = "stub-marker"
	seededCase  = "stub-seeded-workspace"
	timeoutCase = "stub-timeout"
)

// runEvalCopy runs agent with the stub backend against a fresh copy of the
// checked-in self-test fixtures (so results never land in the source tree and
// consecutive runs never share a results directory) and returns the parsed
// result and the evals dir used.
func runEvalCopy(t *testing.T, agent string, opts RunOptions) (AgentResult, string) {
	t.Helper()
	evalsDir := copyEvalsFromRepoRoot(t)
	return runEvalIn(t, evalsDir, agent, opts), evalsDir
}

// copyEvalsFromRepoRoot copies the checked-in self-test fixtures into a fresh
// temp dir. Unlike copyFixturesTo it resolves the source from the module root,
// so it works after the test has changed into the repository root.
func copyEvalsFromRepoRoot(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "evals")
	src := filepath.Join(repoRoot(t), "internal", "eval", "testdata", "evals")
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dst, "results")); err != nil {
		t.Fatal(err)
	}
	return dst
}

// runEvalIn runs agent against an existing evals dir and returns its result.
func runEvalIn(t *testing.T, evalsDir, agent string, opts RunOptions) AgentResult {
	t.Helper()
	resultsDir := filepath.Join(evalsDir, "results")
	before := snapshotDirs(t, resultsDir)

	opts.Backend = "stub"
	opts.EvalsDir = evalsDir
	err := RunWithOptions(agent, "", opts)
	if wantThresholdFailure(agent, opts) {
		if !errors.Is(err, ErrThresholdFailed) {
			t.Fatalf("run of %s (sandbox=%v, keep=%v) = %v, want ErrThresholdFailed", agent, opts.Sandbox, opts.KeepWorkspaces, err)
		}
	} else if err != nil {
		t.Fatalf("run of %s (sandbox=%v, keep=%v) failed: %v", agent, opts.Sandbox, opts.KeepWorkspaces, err)
	}
	added := newRunDirs(t, resultsDir, before)
	if len(added) != 1 {
		t.Fatalf("run of %s created %d result dirs, want 1: %v", agent, len(added), added)
	}
	return readSelfTestResult(t, filepath.Join(resultsDir, added[0], agent+".json"))
}

// wantThresholdFailure reports whether a self-test run of agent is expected to
// finish below its pass threshold: selftest-fail always does, and the
// containment agents do when run natively (every case is refused with
// "requires --sandbox"). Everything else must pass.
func wantThresholdFailure(agent string, opts RunOptions) bool {
	switch agent {
	case "selftest-fail":
		return true
	case "selftest-sandbox", "selftest-sandbox-ro":
		return !opts.Sandbox
	}
	return false
}

func caseByName(t *testing.T, res AgentResult, name string) CaseResult {
	t.Helper()
	for _, c := range res.Cases {
		if c.CaseName == name {
			return c
		}
	}
	t.Fatalf("case %q missing from %s results", name, res.Agent)
	return CaseResult{}
}

// gitIn runs git in dir with a hermetic configuration and returns stdout.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "safe.directory=*"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v in %s: %v: %s", args, dir, err, stderr.String())
	}
	return string(out)
}

// snapshotHostTree maps every path under root (relative, slash-separated) to its
// content, "<dir>" for directories. The top-level .git and .kiro entries are
// harness-owned and excluded.
func snapshotHostTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if top := strings.Split(filepath.ToSlash(rel), "/")[0]; top == ".git" || top == ".kiro" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			tree[filepath.ToSlash(rel)] = "<dir>"
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		tree[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return tree
}

// cleanupWorkspace removes a kept workspace (and its private parent) when the
// test ends.
func cleanupWorkspace(t *testing.T, c CaseResult) {
	t.Helper()
	if c.WorkspaceDir == "" {
		return
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(c.WorkspaceDir)) })
}

// assertWorkspaceCases checks what every workspace case must show on the host
// regardless of backend: marker.txt, the seeded fixture modified once, and a
// single fixture commit. The workspaces must be kept.
func assertWorkspaceCases(t *testing.T, label string, agentRes, failRes AgentResult) {
	t.Helper()

	marker := caseByName(t, agentRes, markerCase)
	if marker.WorkspaceDir == "" {
		t.Fatalf("%s: marker case has no workspace_dir", label)
	}
	data, err := os.ReadFile(filepath.Join(marker.WorkspaceDir, "marker.txt"))
	if err != nil {
		t.Fatalf("%s: reading marker.txt from the host workspace: %v", label, err)
	}
	if string(data) != "hi\n" {
		t.Errorf("%s: marker.txt = %q, want %q", label, data, "hi\n")
	}
	if got := strings.TrimSpace(gitIn(t, marker.WorkspaceDir, "rev-list", "--count", "HEAD")); got != "1" {
		t.Errorf("%s: marker workspace has %s commits, want exactly 1", label, got)
	}
	if st, err := os.Stat(filepath.Join(marker.WorkspaceDir, ".eval")); err != nil || !st.IsDir() {
		t.Errorf("%s: marker workspace has no .eval/ directory (err=%v)", label, err)
	}

	seeded := caseByName(t, agentRes, seededCase)
	if got := gitIn(t, seeded.WorkspaceDir, "status", "--porcelain"); got != " M README.md\n" {
		t.Errorf("%s: seeded workspace status = %q, want %q", label, got, " M README.md\n")
	}
	if got := strings.TrimSpace(gitIn(t, seeded.WorkspaceDir, "rev-list", "--count", "HEAD")); got != "1" {
		t.Errorf("%s: seeded workspace has %s commits, want exactly 1", label, got)
	}
	if _, err := os.Stat(filepath.Join(seeded.WorkspaceDir, "docs", "notes.txt")); err != nil {
		t.Errorf("%s: seeded fixture file docs/notes.txt missing: %v", label, err)
	}

	timeout := caseByName(t, failRes, timeoutCase)
	if timeout.ActualOutput != "" {
		t.Errorf("%s: timeout case actual_output = %q, want empty", label, timeout.ActualOutput)
	}
	if timeout.ErrorContext == nil || !strings.Contains(timeout.ErrorContext.Stderr, "timeout after 1s") {
		t.Errorf("%s: timeout case error_context = %+v, want stderr containing 'timeout after 1s'", label, timeout.ErrorContext)
	}

	for _, res := range []AgentResult{agentRes, failRes} {
		for _, c := range res.Cases {
			raw, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(bytes.ToLower(raw), []byte("permission denied")) {
				t.Errorf("%s: case %s contains 'Permission denied': %s", label, c.CaseName, raw)
			}
		}
	}
}

// TestSelfTestWorkspaceCasesNative runs the checked-in workspace cases with the
// stub backend natively (no daemon needed) and checks the host-side workspace:
// marker.txt, the seeded fixture, the single fixture commit, the timeout
// failure, keep/no-keep semantics and that the repository root is untouched.
func TestSelfTestWorkspaceCasesNative(t *testing.T) {
	t.Cleanup(resetConfig)
	t.Chdir(repoRoot(t))
	rootStatusBefore := gitIn(t, ".", "status", "--porcelain")

	start := time.Now()
	agentRes, _ := runEvalCopy(t, "selftest", RunOptions{NoSandbox: true, KeepWorkspaces: true})
	failRes, _ := runEvalCopy(t, "selftest-fail", RunOptions{NoSandbox: true, KeepWorkspaces: true})
	for _, res := range []AgentResult{agentRes, failRes} {
		for _, c := range res.Cases {
			cleanupWorkspace(t, c)
		}
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("native workspace runs took %v, want well under 30s", elapsed)
	}

	assertWorkspaceCases(t, "native", agentRes, failRes)
	assertSelfTestResults(t, agentRes)

	if _, err := os.Stat("marker.txt"); err == nil {
		t.Error("marker.txt appeared in the repository root")
	}
	if got := gitIn(t, ".", "status", "--porcelain"); got != rootStatusBefore {
		t.Errorf("repo root status changed\nbefore:\n%s\nafter:\n%s", rootStatusBefore, got)
	}

	// Without --keep-workspaces the recorded directory is gone after the run.
	noKeep, _ := runEvalCopy(t, "selftest", RunOptions{NoSandbox: true})
	c := caseByName(t, noKeep, markerCase)
	if c.WorkspaceDir == "" {
		t.Fatal("workspace_dir must be recorded even when the workspace is removed")
	}
	if _, err := os.Stat(c.WorkspaceDir); !os.IsNotExist(err) {
		t.Errorf("workspace %s still exists without --keep-workspaces (err=%v)", c.WorkspaceDir, err)
	}
}

// TestSandboxWorkspace runs the workspace cases natively and under --sandbox
// (stub backend inside the container) with kept workspaces and requires the
// same host-side result, no Permission denied failures, an untouched
// repository root, correct keep/no-keep behavior, a recorded timeout failure
// and base-image reuse after editing an agent config and a case.
//
// Gated like TestSelftestSandbox: needs KAIRON_EVAL_SANDBOX_SELFTEST=1 and a
// reachable Podman or Docker daemon; otherwise it skips without starting
// anything.
func TestSandboxWorkspace(t *testing.T) {
	if os.Getenv(sandboxSelftestEnv) != "1" {
		t.Skipf("sandbox workspace test is opt-in: set %s=1 or run `task eval:selftest:sandbox` (needs Podman or Docker)", sandboxSelftestEnv)
	}
	if err := sandbox.EnsureContainerDaemon(); err != nil {
		t.Skipf("no container daemon reachable (tried Podman and Docker); start Podman or Docker to run the sandbox workspace test: %v", err)
	}

	t.Cleanup(resetConfig)
	t.Chdir(repoRoot(t))

	platform, err := sandbox.DetectHostArchitecture()
	if err != nil {
		t.Fatal(err)
	}
	im, err := sandbox.NewImageManager("sandbox-workspace-test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer im.Close()

	// Build (or reuse) the base image up front so the timing assertion below
	// measures container start plus the case, not an image build.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	tag, _, err := im.EnsureBaseImage(ctx, platform)
	if err != nil {
		t.Fatalf("ensuring base image: %v", err)
	}
	idBefore, err := im.ImageID(ctx, tag)
	if err != nil {
		t.Fatal(err)
	}

	rootStatusBefore := gitIn(t, ".", "status", "--porcelain")

	// Native reference run.
	nativeAgent, _ := runEvalCopy(t, "selftest", RunOptions{NoSandbox: true, KeepWorkspaces: true})
	nativeFail, _ := runEvalCopy(t, "selftest-fail", RunOptions{NoSandbox: true, KeepWorkspaces: true})

	// Sandbox runs, kept.
	var sandboxAgent, sandboxFail AgentResult
	sandboxOut := captureStdout(t, func() {
		sandboxAgent, _ = runEvalCopy(t, "selftest", RunOptions{Sandbox: true, KeepWorkspaces: true})
	})
	failStart := time.Now()
	sandboxFail, _ = runEvalCopy(t, "selftest-fail", RunOptions{Sandbox: true, KeepWorkspaces: true})
	failElapsed := time.Since(failStart)

	for _, res := range []AgentResult{nativeAgent, nativeFail, sandboxAgent, sandboxFail} {
		for _, c := range res.Cases {
			cleanupWorkspace(t, c)
		}
	}

	if !strings.Contains(sandboxOut, "Base image reused: "+tag) {
		t.Errorf("sandbox run output does not report reusing %s:\n%s", tag, sandboxOut)
	}

	assertWorkspaceCases(t, "native", nativeAgent, nativeFail)
	assertWorkspaceCases(t, "sandbox", sandboxAgent, sandboxFail)
	assertSelfTestResults(t, sandboxAgent)

	// Native and sandbox host workspaces are identical for all three cases.
	pairs := []struct {
		name           string
		native, sboxed CaseResult
	}{
		{markerCase, caseByName(t, nativeAgent, markerCase), caseByName(t, sandboxAgent, markerCase)},
		{seededCase, caseByName(t, nativeAgent, seededCase), caseByName(t, sandboxAgent, seededCase)},
		{timeoutCase, caseByName(t, nativeFail, timeoutCase), caseByName(t, sandboxFail, timeoutCase)},
	}
	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			if p.native.WorkspaceDir == p.sboxed.WorkspaceDir {
				t.Fatalf("native and sandbox share workspace %s", p.native.WorkspaceDir)
			}
			nTree, sTree := snapshotHostTree(t, p.native.WorkspaceDir), snapshotHostTree(t, p.sboxed.WorkspaceDir)
			if len(nTree) != len(sTree) {
				t.Errorf("tree sizes differ: native %d, sandbox %d\nnative: %v\nsandbox: %v", len(nTree), len(sTree), nTree, sTree)
			}
			for path, content := range nTree {
				if got, ok := sTree[path]; !ok {
					t.Errorf("%s missing from the sandbox workspace", path)
				} else if got != content {
					t.Errorf("%s differs\n native: %q\nsandbox: %q", path, content, got)
				}
			}
			nStatus := gitIn(t, p.native.WorkspaceDir, "status", "--porcelain")
			sStatus := gitIn(t, p.sboxed.WorkspaceDir, "status", "--porcelain")
			if nStatus != sStatus {
				t.Errorf("git status differs\n native: %q\nsandbox: %q", nStatus, sStatus)
			}
			if _, err := os.Stat(filepath.Join(p.sboxed.WorkspaceDir, ".eval")); err != nil {
				t.Errorf("sandbox workspace has no .eval/: %v", err)
			}
		})
	}

	// The sandboxed timeout case is recorded as a timeout failure, quickly.
	if failElapsed > 30*time.Second {
		t.Errorf("sandboxed timeout run took %v, want under ~30s including container start", failElapsed)
	}

	// Repo root untouched by the sandbox (and native) runs.
	if got := gitIn(t, ".", "status", "--porcelain"); got != rootStatusBefore {
		t.Errorf("repo root status changed\nbefore:\n%s\nafter:\n%s", rootStatusBefore, got)
	}
	if _, err := os.Stat("marker.txt"); err == nil {
		t.Error("marker.txt appeared in the repository root")
	}

	// Kept: exists and holds .eval/. (Verified above for every case.)
	// Not kept: the recorded workspace is gone after the sandbox run.
	noKeep, _ := runEvalCopy(t, "selftest", RunOptions{Sandbox: true})
	nk := caseByName(t, noKeep, markerCase)
	if nk.WorkspaceDir == "" {
		t.Fatal("workspace_dir must be recorded even when the workspace is removed")
	}
	if _, err := os.Stat(nk.WorkspaceDir); !os.IsNotExist(err) {
		t.Errorf("sandbox workspace %s still exists without --keep-workspaces (err=%v)", nk.WorkspaceDir, err)
	}

	// Base image reuse: edit an agent config and a case in a copy of the
	// evals dir; the next --sandbox run must reuse the same image.
	evalsDir := copyEvalsFromRepoRoot(t)
	editFile(t, filepath.Join(evalsDir, "agents", "selftest.json"),
		`"description": "Minimal agent`, `"description": "Edited minimal agent`)
	editFile(t, filepath.Join(evalsDir, "cases", "selftest", "stub-basic.yaml"),
		"description: \"Self-test: stub backend", "description: \"Edited self-test: stub backend")
	var edited AgentResult
	editedOut := captureStdout(t, func() {
		edited = runEvalIn(t, evalsDir, "selftest", RunOptions{Sandbox: true})
	})
	if len(edited.Cases) != 16 {
		t.Errorf("edited run has %d cases, want 16", len(edited.Cases))
	}
	if strings.Contains(editedOut, "Base image built") {
		t.Errorf("editing an agent config and a case rebuilt the base image:\n%s", editedOut)
	}
	if !strings.Contains(editedOut, "Base image reused: "+tag) {
		t.Errorf("edited run did not report reusing %s:\n%s", tag, editedOut)
	}
	tagAfter, built, err := im.EnsureBaseImage(ctx, platform)
	if err != nil {
		t.Fatal(err)
	}
	idAfter, err := im.ImageID(ctx, tagAfter)
	if err != nil {
		t.Fatal(err)
	}
	if built || tagAfter != tag || idAfter != idBefore {
		t.Errorf("base image changed: built=%v tag %s -> %s, id %s -> %s", built, tag, tagAfter, idBefore, idAfter)
	}
}

// editFile replaces old with new in path, failing if old is not present (a
// silent no-op would make the reuse assertion meaningless).
func editFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}
