//go:build !windows

package inference

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeScript is a fake kiro-cli. It records argv, cwd and stdin into
// $FAKE_KIRO_DIR and behaves according to $FAKE_KIRO_MODE.
const fakeScript = `#!/bin/sh
printf '%s\n' "$*" > "$FAKE_KIRO_DIR/args"
pwd > "$FAKE_KIRO_DIR/pwd"
if [ "$1" = "--version" ]; then
  echo "kiro-cli 0.0.0"
  exit 0
fi
cat > "$FAKE_KIRO_DIR/stdin"
case "$FAKE_KIRO_MODE" in
  ansi)
    printf '\033[1mhello\033[0m \033[32;1mworld\033[0m'
    ;;
  stderr)
    echo "a warning" >&2
    printf 'ok'
    ;;
  fail)
    echo "boom" >&2
    exit 3
    ;;
  sleep)
    exec sleep 5
    ;;
  overlay)
    find .kiro/agents -type f | sort > "$FAKE_KIRO_DIR/agents.txt"
    cat .kiro/agents/prompt.md
    ;;
  *)
    printf 'default output'
    ;;
esac
`

// installFakeKiro puts a fake kiro-cli first on PATH and returns the dir it
// records into.
func installFakeKiro(t *testing.T, mode string) string {
	t.Helper()
	bin := t.TempDir()
	rec := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "kiro-cli"), []byte(fakeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_KIRO_DIR", rec)
	t.Setenv("FAKE_KIRO_MODE", mode)
	return rec
}

func readRec(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(b)
}

func newKiro(t *testing.T) Backend {
	t.Helper()
	b, err := New("kiro-cli")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestKiroCLI_AvailableUsesPath(t *testing.T) {
	installFakeKiro(t, "")
	if err := newKiro(t).Available(); err != nil {
		t.Errorf("Available() with fake on PATH = %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := newKiro(t).Available(); err == nil {
		t.Error("Available() should fail when kiro-cli is not on PATH")
	}
}

func TestKiroCLI_StartupProbeRunsVersion(t *testing.T) {
	rec := installFakeKiro(t, "")
	d := newKiro(t).StartupProbe()
	if d <= 0 {
		t.Errorf("StartupProbe() = %v, want > 0", d)
	}
	if got := strings.TrimSpace(readRec(t, rec, "args")); got != "--version" {
		t.Errorf("probe argv = %q, want --version", got)
	}
}

func TestKiroCLI_AgentArgvStdinAndANSI(t *testing.T) {
	rec := installFakeKiro(t, "ansi")
	resp, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "my-agent", Prompt: "do the thing\nline two", Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readRec(t, rec, "args")); got != "chat --agent my-agent --no-interactive --trust-all-tools" {
		t.Errorf("argv = %q", got)
	}
	if got := readRec(t, rec, "stdin"); got != "do the thing\nline two" {
		t.Errorf("stdin = %q", got)
	}
	if resp.Text != "hello world" {
		t.Errorf("Text = %q, want ANSI-stripped %q", resp.Text, "hello world")
	}
	if resp.Command != "kiro-cli chat --agent my-agent --no-interactive --trust-all-tools" {
		t.Errorf("Command = %q", resp.Command)
	}
	if resp.Model != "" {
		t.Errorf("Model = %q, want empty", resp.Model)
	}
	want := EstimateUsage("do the thing\nline two", "hello world")
	if resp.Usage != want || resp.Usage.Source != UsageEstimated {
		t.Errorf("Usage = %+v, want %+v", resp.Usage, want)
	}
	if resp.Duration <= 0 {
		t.Errorf("Duration = %v", resp.Duration)
	}
	// No AgentConfigDir => no cmd.Dir => fake runs in our cwd.
	cwd, _ := os.Getwd()
	gotPwd := strings.TrimSpace(readRec(t, rec, "pwd"))
	if a, _ := filepath.EvalSymlinks(gotPwd); a != mustEval(t, cwd) {
		t.Errorf("cwd = %q, want %q", gotPwd, cwd)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestKiroCLI_AgentCapturesStderrSeparately(t *testing.T) {
	installFakeKiro(t, "stderr")
	resp, err := newKiro(t).Invoke(context.Background(), Request{Role: RoleAgent, Agent: "a", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q (stderr must not leak into stdout)", resp.Text)
	}
	if resp.Stderr != "a warning\n" {
		t.Errorf("Stderr = %q", resp.Stderr)
	}
}

func TestKiroCLI_AgentFailure(t *testing.T) {
	installFakeKiro(t, "fail")
	resp, err := newKiro(t).Invoke(context.Background(), Request{Role: RoleAgent, Agent: "a", Prompt: "p"})
	if err == nil {
		t.Fatal("expected error")
	}
	if want := "kiro-cli invocation failed: exit status 3"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if errors.Is(err, ErrTimeout) {
		t.Error("failure must not match ErrTimeout")
	}
	if resp.ExitCode != 3 {
		t.Errorf("ExitCode = %d", resp.ExitCode)
	}
	if resp.Stderr != "boom\n" {
		t.Errorf("Stderr = %q", resp.Stderr)
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty on failure", resp.Text)
	}
}

func TestKiroCLI_AgentTimeout(t *testing.T) {
	installFakeKiro(t, "sleep")
	resp, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "a", Prompt: "p", Timeout: 300 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = false; err = %v", err)
	}
	if want := "kiro-cli timeout after 300ms"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if resp.Command == "" {
		t.Error("Command should be populated on error")
	}
}

func TestKiroCLI_AgentNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := newKiro(t).Invoke(context.Background(), Request{Role: RoleAgent, Agent: "a", Prompt: "p"})
	if err == nil || !strings.HasPrefix(err.Error(), "kiro-cli invocation failed: ") {
		t.Errorf("error = %v", err)
	}
}

func TestKiroCLI_JudgeArgvStdinRawOutput(t *testing.T) {
	rec := installFakeKiro(t, "ansi")
	resp, err := newKiro(t).Invoke(context.Background(), Request{Role: RoleJudge, Prompt: "judge prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readRec(t, rec, "args")); got != "chat --no-interactive" {
		t.Errorf("argv = %q", got)
	}
	if got := readRec(t, rec, "stdin"); got != "judge prompt" {
		t.Errorf("stdin = %q", got)
	}
	// Judge output is raw: ANSI must NOT be stripped.
	if !strings.Contains(resp.Text, "\x1b[1m") {
		t.Errorf("judge Text should be raw, got %q", resp.Text)
	}
	if resp.Command != "kiro-cli chat --no-interactive" {
		t.Errorf("Command = %q", resp.Command)
	}
	if want := EstimateUsage("judge prompt", resp.Text); resp.Usage != want {
		t.Errorf("Usage = %+v, want %+v", resp.Usage, want)
	}
}

func TestKiroCLI_JudgeFailureWording(t *testing.T) {
	installFakeKiro(t, "fail")
	resp, err := newKiro(t).Invoke(context.Background(), Request{Role: RoleJudge, Prompt: "p"})
	if err == nil {
		t.Fatal("expected error")
	}
	if want := "kiro-cli chat failed: exit status 3"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if resp.ExitCode != 3 || resp.Stderr != "boom\n" {
		t.Errorf("ExitCode=%d Stderr=%q", resp.ExitCode, resp.Stderr)
	}
}

func TestKiroCLI_JudgeTimeout(t *testing.T) {
	installFakeKiro(t, "sleep")
	_, err := newKiro(t).Invoke(context.Background(), Request{Role: RoleJudge, Prompt: "p", Timeout: 300 * time.Millisecond})
	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
	if err != nil && !strings.HasPrefix(err.Error(), "kiro-cli chat failed: ") {
		t.Errorf("error wording = %q", err.Error())
	}
}

func TestKiroCLI_UnsupportedRole(t *testing.T) {
	if _, err := newKiro(t).Invoke(context.Background(), Request{Role: ""}); err == nil {
		t.Error("expected error for empty role")
	}
}

func writeAgentDir(t *testing.T, agent string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "agents")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		agent + ".json": `{"name":"` + agent + `","prompt":"file://./prompt.md"}`,
		"prompt.md":     "overlay prompt body",
		"sub/extra.md":  "nested",
	}
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestKiroCLI_AgentConfigDirOverlay(t *testing.T) {
	rec := installFakeKiro(t, "overlay")
	dir := writeAgentDir(t, "selftest")

	resp, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "selftest", Prompt: "p", AgentConfigDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "overlay prompt body" {
		t.Errorf("Text = %q (overlay prompt not visible to agent)", resp.Text)
	}
	listing := readRec(t, rec, "agents.txt")
	for _, want := range []string{"selftest.json", "prompt.md", "sub/extra.md"} {
		if !strings.Contains(listing, ".kiro/agents/"+want) {
			t.Errorf("overlay listing missing %s:\n%s", want, listing)
		}
	}
	// Ran in a temp dir, which is gone now.
	pwd := strings.TrimSpace(readRec(t, rec, "pwd"))
	cwd, _ := os.Getwd()
	if mustEval(t, cwd) == func() string { r, _ := filepath.EvalSymlinks(pwd); return r }() {
		t.Error("overlay run should not use the caller's cwd")
	}
	if _, statErr := os.Stat(pwd); !os.IsNotExist(statErr) {
		t.Errorf("overlay dir %s should be cleaned up (stat err = %v)", pwd, statErr)
	}
	// Source dir untouched.
	if _, statErr := os.Stat(filepath.Join(dir, "selftest.json")); statErr != nil {
		t.Errorf("source agent dir modified: %v", statErr)
	}
}

func TestKiroCLI_AgentConfigDirWithoutMatchingAgentNoOverlay(t *testing.T) {
	rec := installFakeKiro(t, "")
	dir := writeAgentDir(t, "other")
	if _, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "selftest", Prompt: "p", AgentConfigDir: dir,
	}); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	gotPwd := mustEval(t, strings.TrimSpace(readRec(t, rec, "pwd")))
	if gotPwd != mustEval(t, cwd) {
		t.Errorf("cwd = %q, want unchanged %q", gotPwd, cwd)
	}
}

func TestKiroCLI_OverlayCleanedUpOnFailure(t *testing.T) {
	rec := installFakeKiro(t, "fail")
	dir := writeAgentDir(t, "selftest")
	_, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "selftest", Prompt: "p", AgentConfigDir: dir,
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	pwd := strings.TrimSpace(readRec(t, rec, "pwd"))
	if _, statErr := os.Stat(pwd); !os.IsNotExist(statErr) {
		t.Errorf("overlay dir %s should be cleaned up after failure", pwd)
	}
}
