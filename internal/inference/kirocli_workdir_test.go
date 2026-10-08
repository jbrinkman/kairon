//go:build !windows

package inference

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKiroCLI_WorkDirSetsCmdDir(t *testing.T) {
	rec := installFakeKiro(t, "")
	wd := t.TempDir()

	if _, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "a", Prompt: "p", WorkDir: wd, Timeout: 10 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(readRec(t, rec, "pwd"))
	if mustEval(t, got) != mustEval(t, wd) {
		t.Errorf("cwd = %q, want %q", got, wd)
	}
}

func TestKiroCLI_WorkDirTakesPrecedenceOverOverlay(t *testing.T) {
	rec := installFakeKiro(t, "")
	wd := t.TempDir()
	dir := writeAgentDir(t, "selftest")

	if _, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "selftest", Prompt: "p", WorkDir: wd, AgentConfigDir: dir,
		Timeout: 10 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(readRec(t, rec, "pwd"))
	if mustEval(t, got) != mustEval(t, wd) {
		t.Errorf("cwd = %q, want WorkDir %q (overlay temp dir must not be used)", got, wd)
	}
	if _, err := os.Stat(filepath.Join(wd, ".kiro")); err == nil {
		t.Error("overlay must not write into WorkDir")
	}
}

func TestKiroCLI_EmptyWorkDirKeepsOverlayBehavior(t *testing.T) {
	rec := installFakeKiro(t, "")
	dir := writeAgentDir(t, "selftest")

	if _, err := newKiro(t).Invoke(context.Background(), Request{
		Role: RoleAgent, Agent: "selftest", Prompt: "p", AgentConfigDir: dir, Timeout: 10 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(readRec(t, rec, "pwd"))
	// The overlay temp dir is removed after the call, so it cannot be
	// symlink-resolved; compare the raw path against our cwd instead.
	cwd, _ := os.Getwd()
	if got == cwd || got == mustEval(t, cwd) {
		t.Errorf("overlay should run in a temp dir, ran in cwd %q", got)
	}
	if !strings.Contains(got, "kairon-inference-") {
		t.Errorf("cwd = %q, want an overlay temp dir", got)
	}
}
