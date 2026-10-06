package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests exercise .kairon/scripts/worktree-create.sh against throw-away
// git repositories created under t.TempDir(). Every git/bash invocation runs
// with an isolated environment (no host git config, fixed identity) and only
// talks to local file-system remotes, so no network access is needed.

const worktreeCreateTimeout = 60 * time.Second

// worktreeCreateScript returns the absolute path of the script under test,
// resolved relative to this package directory.
func worktreeCreateScript(t *testing.T) string {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", ".kairon", "scripts", "worktree-create.sh"))
	if err != nil {
		t.Fatalf("resolve script path: %v", err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("script under test not found: %v", err)
	}
	return script
}

// worktreeFixture is a bare origin plus a "local" clone (the repo the script
// runs in) and an "other" clone used to push commits that local has not fetched.
type worktreeFixture struct {
	t             *testing.T
	env           []string
	root          string
	origin        string
	other         string
	local         string
	defaultBranch string
}

// isolatedGitEnv builds an environment that ignores host git configuration and
// pins the commit identity. Inherited GIT_* and KAIRON_* variables are dropped.
func isolatedGitEnv(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "KAIRON_") ||
			strings.HasPrefix(kv, "HOME=") || strings.HasPrefix(kv, "XDG_CONFIG_HOME=") ||
			strings.HasPrefix(kv, "LC_ALL=") || strings.HasPrefix(kv, "PWD=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Kairon Test",
		"GIT_AUTHOR_EMAIL=kairon-test@example.invalid",
		"GIT_COMMITTER_NAME=Kairon Test",
		"GIT_COMMITTER_EMAIL=kairon-test@example.invalid",
		"LC_ALL=C",
	)
}

// requireWorktreeTools skips the test when git or bash are unavailable.
func requireWorktreeTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"git", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found on PATH: %v", tool, err)
		}
	}
}

// run executes a command and returns stdout, stderr and the run error.
func (f *worktreeFixture) run(dir string, extraEnv []string, name string, args ...string) (string, string, error) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), worktreeCreateTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(append([]string{}, f.env...), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// git runs git in dir, failing the test on error, and returns trimmed stdout.
func (f *worktreeFixture) git(dir string, args ...string) string {
	f.t.Helper()
	stdout, stderr, err := f.run(dir, nil, "git", args...)
	if err != nil {
		f.t.Fatalf("git %s (in %s) failed: %v\nstderr: %s", strings.Join(args, " "), dir, err, stderr)
	}
	return strings.TrimSpace(stdout)
}

// gitTry runs git in dir and returns trimmed stdout and the error (no failure).
func (f *worktreeFixture) gitTry(dir string, args ...string) (string, error) {
	f.t.Helper()
	stdout, _, err := f.run(dir, nil, "git", args...)
	return strings.TrimSpace(stdout), err
}

// commit writes a file in repo and commits it, returning the new commit SHA.
func (f *worktreeFixture) commit(repo, file, msg string) string {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(repo, file), []byte(msg+"\n"), 0o644); err != nil {
		f.t.Fatalf("write %s: %v", file, err)
	}
	f.git(repo, "add", file)
	f.git(repo, "commit", "--quiet", "-m", msg)
	return f.git(repo, "rev-parse", "HEAD")
}

// newWorktreeFixture builds origin (default branch defaultBranch, one commit),
// an "other" working clone, and a "local" clone that is initially in sync.
func newWorktreeFixture(t *testing.T, defaultBranch string) *worktreeFixture {
	t.Helper()
	requireWorktreeTools(t)

	// Resolve symlinks (e.g. /var -> /private/var on macOS) so paths the
	// script prints via `pwd` compare equal to the paths built here.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create home: %v", err)
	}

	f := &worktreeFixture{
		t:             t,
		env:           isolatedGitEnv(home),
		root:          root,
		origin:        filepath.Join(root, "origin.git"),
		other:         filepath.Join(root, "other"),
		local:         filepath.Join(root, "local"),
		defaultBranch: defaultBranch,
	}

	f.git(root, "init", "--quiet", "--bare", "--initial-branch="+defaultBranch, f.origin)
	f.git(root, "init", "--quiet", "--initial-branch="+defaultBranch, f.other)
	f.git(f.other, "remote", "add", "origin", f.origin)
	f.commit(f.other, "README.md", "initial commit")
	f.git(f.other, "push", "--quiet", "origin", defaultBranch)
	f.git(root, "clone", "--quiet", f.origin, f.local)
	return f
}

// pushUpstreamCommit pushes a new commit to origin's default branch from the
// "other" clone (local does not fetch it) and returns the new tip.
func (f *worktreeFixture) pushUpstreamCommit(msg string) string {
	f.t.Helper()
	f.git(f.other, "checkout", "--quiet", f.defaultBranch)
	tip := f.commit(f.other, "upstream-"+msg+".txt", msg)
	f.git(f.other, "push", "--quiet", "origin", f.defaultBranch)
	return tip
}

// pushBranch creates branch on origin (from the default branch, with one
// distinct commit) via the "other" clone and returns its tip. local does not
// learn about the branch until the script fetches it.
func (f *worktreeFixture) pushBranch(branch string) string {
	f.t.Helper()
	f.git(f.other, "checkout", "--quiet", f.defaultBranch)
	f.git(f.other, "checkout", "--quiet", "-b", branch)
	tip := f.commit(f.other, "branch-"+strings.ReplaceAll(branch, "/", "-")+".txt", "commit on "+branch)
	f.git(f.other, "push", "--quiet", "origin", branch)
	f.git(f.other, "checkout", "--quiet", f.defaultBranch)
	return tip
}

// writeConfig writes .kairon/config.yaml in the local clone.
func (f *worktreeFixture) writeConfig(content string) {
	f.t.Helper()
	dir := filepath.Join(f.local, ".kairon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatalf("create %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		f.t.Fatalf("write config: %v", err)
	}
}

// scriptResult is the outcome of one script run.
type scriptResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// runScript runs worktree-create.sh <name> via bash with cwd = local clone.
func (f *worktreeFixture) runScript(name string, extraEnv ...string) scriptResult {
	f.t.Helper()
	stdout, stderr, err := f.run(f.local, extraEnv, "bash", worktreeCreateScript(f.t), name)
	res := scriptResult{stdout: stdout, stderr: stderr}
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			f.t.Fatalf("run script: %v", err)
		}
		res.exitCode = exitErr.ExitCode()
	}
	return res
}

// mustSucceed fails the test unless the script exited 0.
func (r scriptResult) mustSucceed(t *testing.T) {
	t.Helper()
	if r.exitCode != 0 {
		t.Fatalf("script exited %d, want 0\nstdout: %s\nstderr: %s", r.exitCode, r.stdout, r.stderr)
	}
}

// worktreePath is the expected absolute worktree path for a spec name.
func (f *worktreeFixture) worktreePath(name string) string {
	return filepath.Join(f.local, ".worktrees", name)
}

// assertMergeBase asserts merge-base(spec/<name>, ref) == wantTip.
func (f *worktreeFixture) assertMergeBase(name, ref, wantTip string) {
	f.t.Helper()
	got := f.git(f.local, "merge-base", "spec/"+name, ref)
	if got != wantTip {
		f.t.Errorf("merge-base(spec/%s, %s) = %s, want %s", name, ref, got, wantTip)
	}
}

func TestWorktreeCreate(t *testing.T) {
	requireWorktreeTools(t)
	// Fail early (not skip) if the script itself is missing.
	worktreeCreateScript(t)

	const name = "issue-288-1234"

	// Scenarios 1 and 2.
	t.Run("LocalBehindOrigin_BranchesFromOriginTip", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		oldTip := f.git(f.local, "rev-parse", "main")
		newTip := f.pushUpstreamCommit("upstream work merged after clone")
		if oldTip == newTip {
			t.Fatal("fixture error: origin tip did not move")
		}

		// Preconditions: local is genuinely behind and has not fetched.
		if got := f.git(f.local, "rev-parse", "origin/main"); got != oldTip {
			t.Fatalf("precondition: local origin/main = %s, want stale %s", got, oldTip)
		}

		res := f.runScript(name)
		res.mustSucceed(t)

		// Acceptance criterion: merge-base equals origin/main's new tip.
		mergeBase := f.git(f.local, "merge-base", "spec/"+name, "origin/main")
		if mergeBase != newTip {
			t.Errorf("git merge-base spec/%s origin/main = %s, want origin/main new tip %s", name, mergeBase, newTip)
		}
		if got := f.git(f.local, "rev-parse", "spec/"+name); got != newTip {
			t.Errorf("spec/%s tip = %s, want %s", name, got, newTip)
		}

		// Local main is untouched (still at the stale tip).
		if got := f.git(f.local, "rev-parse", "main"); got != oldTip {
			t.Errorf("local main = %s, want untouched %s", got, oldTip)
		}

		// The worktree checkout itself is at the fresh tip.
		if got := f.git(f.worktreePath(name), "rev-parse", "HEAD"); got != newTip {
			t.Errorf("worktree HEAD = %s, want %s", got, newTip)
		}
	})

	t.Run("ScriptFetchUpdatesOriginRemoteRef", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		oldTip := f.git(f.local, "rev-parse", "origin/main")
		newTip := f.pushUpstreamCommit("fetched by script")

		// No manual fetch: the stale remote-tracking ref must be the only
		// thing local knows before the script runs.
		if got := f.git(f.local, "rev-parse", "origin/main"); got != oldTip {
			t.Fatalf("precondition: origin/main = %s, want %s", got, oldTip)
		}

		f.runScript(name).mustSucceed(t)

		if got := f.git(f.local, "rev-parse", "origin/main"); got != newTip {
			t.Errorf("origin/main after script = %s, want %s (script must fetch)", got, newTip)
		}
	})

	// Scenario 3.
	t.Run("StdoutIsExactlyAbsoluteWorktreePath", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		f.pushUpstreamCommit("something new")

		res := f.runScript(name)
		res.mustSucceed(t)

		want := f.worktreePath(name) + "\n"
		if res.stdout != want {
			t.Errorf("stdout = %q, want exactly %q", res.stdout, want)
		}
		if !filepath.IsAbs(strings.TrimSpace(res.stdout)) {
			t.Errorf("stdout %q is not an absolute path", res.stdout)
		}
		if info, err := os.Stat(strings.TrimSpace(res.stdout)); err != nil || !info.IsDir() {
			t.Errorf("worktree directory %q missing (err=%v)", res.stdout, err)
		}
		if res.stderr == "" {
			t.Error("expected progress logging on stderr, got none")
		}
	})

	// Scenario 4.
	t.Run("BranchHasNoUpstream", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		f.pushUpstreamCommit("something new")

		f.runScript(name).mustSucceed(t)

		if out, err := f.gitTry(f.local, "config", "--get", "branch.spec/"+name+".remote"); err == nil || out != "" {
			t.Errorf("branch.spec/%s.remote = %q (err=%v), want unset", name, out, err)
		}
		if out, err := f.gitTry(f.local, "config", "--get", "branch.spec/"+name+".merge"); err == nil || out != "" {
			t.Errorf("branch.spec/%s.merge = %q (err=%v), want unset", name, out, err)
		}
		if out, err := f.gitTry(f.local, "rev-parse", "--abbrev-ref", "spec/"+name+"@{upstream}"); err == nil {
			t.Errorf("spec/%s unexpectedly has upstream %q", name, out)
		}
	})

	// Scenario 5.
	t.Run("ConfigBaseBranchSelectsOriginDevelop", func(t *testing.T) {
		cases := []struct {
			name string
			line string
		}{
			{"Plain", "base_branch: develop"},
			{"DoubleQuoted", `base_branch: "develop"`},
			{"SingleQuoted", `base_branch: 'develop'`},
			{"TrailingComment", "base_branch: develop # integration branch"},
			{"QuotedWithTrailingComment", `base_branch: "develop"   # integration branch`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := newWorktreeFixture(t, "main")
				mainTip := f.pushUpstreamCommit("main moved")
				developTip := f.pushBranch("develop")
				if developTip == mainTip {
					t.Fatal("fixture error: develop and main tips are identical")
				}
				// A nested base_branch key must not be picked up: only the
				// top-level (column 0) key counts.
				f.writeConfig("repo: owner/name\nnested:\n  base_branch: wrong\n" + tc.line + "\nlabel: kairon\n")

				f.runScript(name).mustSucceed(t)

				f.assertMergeBase(name, "origin/develop", developTip)
				if got := f.git(f.local, "rev-parse", "spec/"+name); got != developTip {
					t.Errorf("spec/%s tip = %s, want origin/develop tip %s", name, got, developTip)
				}
				if got := f.git(f.local, "rev-parse", "origin/develop"); got != developTip {
					t.Errorf("origin/develop = %s, want %s (script must fetch it)", got, developTip)
				}
			})
		}
	})

	// Scenario 6.
	t.Run("EnvOverridesConfigBaseBranch", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		developTip := f.pushBranch("develop")
		releaseTip := f.pushBranch("release/1.x")
		f.writeConfig("base_branch: develop\n")

		f.runScript(name, "KAIRON_BASE_BRANCH=release/1.x").mustSucceed(t)

		if got := f.git(f.local, "rev-parse", "spec/"+name); got != releaseTip {
			t.Errorf("spec/%s tip = %s, want origin/release/1.x tip %s", name, got, releaseTip)
		}
		if got := f.git(f.local, "rev-parse", "spec/"+name); got == developTip {
			t.Errorf("spec/%s was based on config base_branch, env override ignored", name)
		}
	})

	// Scenario 7.
	t.Run("AutoDetectsOriginDefaultBranchTrunk", func(t *testing.T) {
		cases := []struct {
			name     string
			dropHEAD bool
		}{
			{"FromOriginHEADSymref", false},
			{"FromLsRemoteWhenOriginHEADUnset", true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := newWorktreeFixture(t, "trunk")
				// A decoy "main" branch proves "main" is not hard-coded.
				decoyTip := f.pushBranch("main")
				trunkTip := f.pushUpstreamCommit("trunk moved")
				if tc.dropHEAD {
					f.git(f.local, "remote", "set-head", "origin", "--delete")
					if out, err := f.gitTry(f.local, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
						t.Fatalf("precondition: origin/HEAD still set to %q", out)
					}
				}

				// No config and no env override.
				f.runScript(name).mustSucceed(t)

				f.assertMergeBase(name, "origin/trunk", trunkTip)
				if got := f.git(f.local, "rev-parse", "spec/"+name); got != trunkTip {
					t.Errorf("spec/%s tip = %s, want origin/trunk tip %s (decoy main %s)", name, got, trunkTip, decoyTip)
				}
			})
		}
	})

	// Scenario 8.
	t.Run("NoOriginRemoteBranchesFromLocalHEAD", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		localTip := f.commit(f.local, "local-only.txt", "local only commit")
		f.git(f.local, "remote", "remove", "origin")

		res := f.runScript(name)
		res.mustSucceed(t)

		if got := f.git(f.local, "rev-parse", "spec/"+name); got != localTip {
			t.Errorf("spec/%s tip = %s, want local HEAD %s", name, got, localTip)
		}
		if want := f.worktreePath(name) + "\n"; res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
		if !strings.Contains(res.stderr, "no 'origin' remote") {
			t.Errorf("stderr should warn about missing origin remote, got:\n%s", res.stderr)
		}
	})

	// Scenario 9.
	t.Run("FetchFailureUsesCachedOriginWithWarning", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		cachedTip := f.git(f.local, "rev-parse", "origin/main")
		// Origin moves on, but local can no longer reach it.
		f.pushUpstreamCommit("unreachable upstream work")
		f.git(f.local, "remote", "set-url", "origin", filepath.Join(f.root, "does-not-exist.git"))

		res := f.runScript(name)
		res.mustSucceed(t)

		if got := f.git(f.local, "rev-parse", "spec/"+name); got != cachedTip {
			t.Errorf("spec/%s tip = %s, want cached origin/main %s", name, got, cachedTip)
		}
		f.assertMergeBase(name, "origin/main", cachedTip)
		if !strings.Contains(res.stderr, "Warning: fetch failed") || !strings.Contains(res.stderr, "origin/main") {
			t.Errorf("stderr should warn that fetch failed and cached origin/main is used, got:\n%s", res.stderr)
		}
		if want := f.worktreePath(name) + "\n"; res.stdout != want {
			t.Errorf("stdout = %q, want %q", res.stdout, want)
		}
	})

	// Scenario 10.
	t.Run("MissingConfiguredBranchFailsWithoutSideEffects", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		f.writeConfig("base_branch: does-not-exist\n")

		res := f.runScript(name)

		if res.exitCode == 0 {
			t.Fatalf("script exited 0, want non-zero\nstderr: %s", res.stderr)
		}
		if res.stdout != "" {
			t.Errorf("stdout = %q, want empty on failure", res.stdout)
		}
		if !strings.Contains(res.stderr, "does-not-exist") {
			t.Errorf("stderr should name the missing branch, got:\n%s", res.stderr)
		}
		if _, err := os.Stat(f.worktreePath(name)); !os.IsNotExist(err) {
			t.Errorf("worktree dir %s should not exist (stat err=%v)", f.worktreePath(name), err)
		}
		if out := f.git(f.local, "branch", "--list", "spec/"+name); out != "" {
			t.Errorf("branch spec/%s should not exist, got %q", name, out)
		}
		if out := f.git(f.local, "worktree", "list", "--porcelain"); strings.Contains(out, name) {
			t.Errorf("worktree list should not mention %s:\n%s", name, out)
		}
		if out := f.git(f.local, "branch", "--list", "does-not-exist"); out != "" {
			t.Errorf("script must not fall back to local branch, got %q", out)
		}
	})

	// Scenario 11.
	t.Run("RerunIsIdempotent", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		f.pushUpstreamCommit("first")

		first := f.runScript(name)
		first.mustSucceed(t)
		tipAfterFirst := f.git(f.local, "rev-parse", "spec/"+name)

		// Origin moves again; an existing worktree must be left as-is.
		f.pushUpstreamCommit("second")

		second := f.runScript(name)
		second.mustSucceed(t)

		if second.stdout != first.stdout {
			t.Errorf("second run stdout = %q, want same as first %q", second.stdout, first.stdout)
		}
		if want := f.worktreePath(name) + "\n"; second.stdout != want {
			t.Errorf("second run stdout = %q, want %q", second.stdout, want)
		}
		if !strings.Contains(second.stderr, "already exists") {
			t.Errorf("second run stderr should report existing worktree, got:\n%s", second.stderr)
		}
		if got := f.git(f.local, "rev-parse", "spec/"+name); got != tipAfterFirst {
			t.Errorf("existing branch moved on re-run: %s, want %s", got, tipAfterFirst)
		}
	})

	t.Run("StaleBranchWithoutWorktreeIsReplaced", func(t *testing.T) {
		f := newWorktreeFixture(t, "main")
		staleTip := f.git(f.local, "rev-parse", "main")
		f.git(f.local, "branch", "spec/"+name, staleTip)
		newTip := f.pushUpstreamCommit("fresh work")

		res := f.runScript(name)
		res.mustSucceed(t)

		if got := f.git(f.local, "rev-parse", "spec/"+name); got != newTip {
			t.Errorf("spec/%s tip = %s, want fresh origin/main tip %s (stale was %s)", name, got, newTip, staleTip)
		}
		f.assertMergeBase(name, "origin/main", newTip)
		// stdout must be exactly the absolute worktree path, even when a
		// stale branch was deleted (git's "Deleted branch ..." must not leak).
		if want := f.worktreePath(name) + "\n"; res.stdout != want {
			t.Errorf("stdout = %q, want exactly %q", res.stdout, want)
		}
		if !strings.Contains(res.stderr, "stale branch") {
			t.Errorf("stderr should mention stale branch removal, got:\n%s", res.stderr)
		}
	})
}
