package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// candProject creates a temp project with .kiro/agents/a.json using the
// relative file:// prompt ref and a live prompt, and returns a helper-written
// candidate file path.
func candProject(t *testing.T, candidate string) string {
	t.Helper()
	chdirTemp(t)
	writeProvFile(t, ".kiro/agents/a-prompt.md", "live prompt")
	writeProvFile(t, ".kiro/agents/a.json",
		`{"name":"a","model":"claude-sonnet-5.5","prompt":"file://./a-prompt.md","resources":[]}`)
	path := filepath.Join("iterations", "a", "v2.md")
	writeProvFile(t, path, candidate)
	return path
}

func TestCandidateLoadSuccess(t *testing.T) {
	path := candProject(t, "candidate prompt")

	c, err := loadCandidatePrompt("a", path)
	if err != nil {
		t.Fatalf("loadCandidatePrompt: %v", err)
	}
	if c.Path != filepath.ToSlash(path) {
		t.Errorf("Path = %q, want %q", c.Path, filepath.ToSlash(path))
	}
	if string(c.Content) != "candidate prompt" {
		t.Errorf("Content = %q", c.Content)
	}
	if want := filepath.Join("agents", "a-prompt.md"); c.Target != want {
		t.Errorf("Target = %q, want %q", c.Target, want)
	}
}

func TestCandidateLoadPathSlashNormalised(t *testing.T) {
	candProject(t, "x")
	c, err := loadCandidatePrompt("a", "./iterations/a/../a/v2.md")
	if err != nil {
		t.Fatalf("loadCandidatePrompt: %v", err)
	}
	if strings.Contains(c.Path, `\`) {
		t.Errorf("Path %q is not slash-normalised", c.Path)
	}
	// "as given": no absolutising.
	if filepath.IsAbs(c.Path) {
		t.Errorf("Path %q was made absolute", c.Path)
	}
}

func TestCandidateLoadRejections(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (agent, path string)
		wantErr string
	}{
		{
			name: "missing file",
			setup: func(t *testing.T) (string, string) {
				candProject(t, "x")
				return "a", "iterations/a/nope.md"
			},
			wantErr: "nope.md",
		},
		{
			name: "directory",
			setup: func(t *testing.T) (string, string) {
				candProject(t, "x")
				return "a", "iterations/a"
			},
			wantErr: "regular file",
		},
		{
			name: "empty file",
			setup: func(t *testing.T) (string, string) {
				return "a", candProject(t, "")
			},
			wantErr: "empty",
		},
		{
			name: "whitespace-only file",
			setup: func(t *testing.T) (string, string) {
				return "a", candProject(t, " \n\t\n")
			},
			wantErr: "empty",
		},
		{
			name: "missing agent config",
			setup: func(t *testing.T) (string, string) {
				return "ghost", candProject(t, "x")
			},
			wantErr: "ghost",
		},
		{
			name: "inline prompt",
			setup: func(t *testing.T) (string, string) {
				p := candProject(t, "x")
				writeProvFile(t, ".kiro/agents/a.json", `{"name":"a","prompt":"You are an agent."}`)
				return "a", p
			},
			wantErr: "file://",
		},
		{
			name: "no prompt at all",
			setup: func(t *testing.T) (string, string) {
				p := candProject(t, "x")
				writeProvFile(t, ".kiro/agents/a.json", `{"name":"a"}`)
				return "a", p
			},
			wantErr: "file://",
		},
		{
			name: "absolute file:// prompt",
			setup: func(t *testing.T) (string, string) {
				p := candProject(t, "x")
				writeProvFile(t, ".kiro/agents/a.json", `{"name":"a","prompt":"file:///etc/a-prompt.md"}`)
				return "a", p
			},
			wantErr: "relative",
		},
		{
			name: "ref escaping the .kiro tree",
			setup: func(t *testing.T) (string, string) {
				p := candProject(t, "x")
				writeProvFile(t, ".kiro/agents/a.json", `{"name":"a","prompt":"file://../../outside.md"}`)
				return "a", p
			},
			wantErr: ".kiro",
		},
		{
			name: "ref resolving to the agents dir itself",
			setup: func(t *testing.T) (string, string) {
				p := candProject(t, "x")
				writeProvFile(t, ".kiro/agents/a.json", `{"name":"a","prompt":"file://."}`)
				return "a", p
			},
			wantErr: "file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, path := tt.setup(t)
			c, err := loadCandidatePrompt(agent, path)
			if err == nil {
				t.Fatalf("expected error, got candidate %+v", c)
			}
			if c != nil {
				t.Errorf("candidate = %+v, want nil on error", c)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestCandidateLoadRefStayingInsideKiroIsAllowed(t *testing.T) {
	// ../prompts/p.md from .kiro/agents resolves to .kiro/prompts/p.md: still inside .kiro.
	p := candProject(t, "x")
	writeProvFile(t, ".kiro/agents/a.json", `{"name":"a","prompt":"file://../prompts/p.md"}`)
	c, err := loadCandidatePrompt("a", p)
	if err != nil {
		t.Fatalf("loadCandidatePrompt: %v", err)
	}
	if want := filepath.Join("prompts", "p.md"); c.Target != want {
		t.Errorf("Target = %q, want %q", c.Target, want)
	}
}

func TestCandidateLoadUsesOverlayConfigFirst(t *testing.T) {
	p := candProject(t, "x")
	writeProvFile(t, ".kairon/evals/agents/a.json", `{"name":"a","prompt":"file://./overlay-prompt.md"}`)
	c, err := loadCandidatePrompt("a", p)
	if err != nil {
		t.Fatalf("loadCandidatePrompt: %v", err)
	}
	if want := filepath.Join("agents", "overlay-prompt.md"); c.Target != want {
		t.Errorf("Target = %q, want %q (overlay config)", c.Target, want)
	}
}

func TestCandidateValidateOptions(t *testing.T) {
	if err := validateCandidateOptions("", RunOptions{}); err != nil {
		t.Errorf("no PromptFile, no agent: unexpected error %v", err)
	}
	if err := validateCandidateOptions("a", RunOptions{List: true, Perf: true}); err != nil {
		t.Errorf("no PromptFile: unexpected error %v", err)
	}
	if err := validateCandidateOptions("a", RunOptions{PromptFile: "p.md"}); err != nil {
		t.Errorf("PromptFile with agent: unexpected error %v", err)
	}

	err := validateCandidateOptions("", RunOptions{PromptFile: "p.md"})
	if err == nil || !strings.Contains(err.Error(), "an agent is required") {
		t.Errorf("no agent: error = %v, want 'an agent is required'", err)
	} else if !strings.Contains(err.Error(), "--prompt-file") {
		t.Errorf("error %q should name the --prompt-file flag", err)
	}

	for name, opts := range map[string]RunOptions{
		"--list":    {PromptFile: "p.md", List: true},
		"--perf":    {PromptFile: "p.md", Perf: true},
		"--cleanup": {PromptFile: "p.md", Cleanup: true},
	} {
		err := validateCandidateOptions("a", opts)
		if err == nil {
			t.Errorf("%s: expected error", name)
		} else if !strings.Contains(err.Error(), name) {
			t.Errorf("%s: error %q should name the conflicting flag", name, err)
		}
	}
}

func TestCandidateStageWritesTarget(t *testing.T) {
	kiro := filepath.Join(t.TempDir(), ".kiro")
	c := &candidatePrompt{Path: "p.md", Content: []byte("cand"), Target: filepath.Join("agents", "a-prompt.md")}

	if err := c.stage(kiro); err != nil {
		t.Fatalf("stage: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(kiro, "agents", "a-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cand" {
		t.Errorf("staged content = %q", got)
	}
}

func TestCandidateStageOverridesExistingFile(t *testing.T) {
	kiro := t.TempDir()
	writeProvFile(t, filepath.Join(kiro, "agents", "a-prompt.md"), "fixture version")
	c := &candidatePrompt{Content: []byte("cand"), Target: filepath.Join("agents", "a-prompt.md")}

	if err := c.stage(kiro); err != nil {
		t.Fatalf("stage: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(kiro, "agents", "a-prompt.md"))
	if string(got) != "cand" {
		t.Errorf("staged content = %q, want candidate to win", got)
	}
}

func TestCandidateStageNeverWritesThroughSymlink(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeProvFile(t, outside, "untouched")
	kiro := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kiro, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(kiro, "agents", "a-prompt.md")
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	c := &candidatePrompt{Content: []byte("cand"), Target: filepath.Join("agents", "a-prompt.md")}

	if err := c.stage(kiro); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != "untouched" {
		t.Errorf("symlink target was written through: %q", got)
	}
	if got, _ := os.ReadFile(target); string(got) != "cand" {
		t.Errorf("staged content = %q", got)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
		t.Errorf("staged target should be a regular file, got %v (err %v)", info, err)
	}
}

func TestCandidateStageNilIsNoop(t *testing.T) {
	kiro := t.TempDir()
	var c *candidatePrompt
	if err := c.stage(kiro); err != nil {
		t.Fatalf("nil stage: %v", err)
	}
	entries, _ := os.ReadDir(kiro)
	if len(entries) != 0 {
		t.Errorf("nil candidate wrote %d entries", len(entries))
	}
}

func TestCandidateStageRejectsEscapingTarget(t *testing.T) {
	kiro := filepath.Join(t.TempDir(), ".kiro")
	for _, target := range []string{"../evil.md", filepath.Join("agents", "..", "..", "evil.md"), "/abs/evil.md", ""} {
		c := &candidatePrompt{Content: []byte("x"), Target: target}
		if err := c.stage(kiro); err == nil {
			t.Errorf("target %q: expected error", target)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(kiro), "evil.md")); err == nil {
		t.Error("stage wrote outside the .kiro dir")
	}
}

// ---- provenance override ----

func TestCandidateProvenanceOverride(t *testing.T) {
	path := candProject(t, "candidate prompt")
	live := mustProv(t, "a", false)

	c, err := loadCandidatePrompt("a", path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveAgentProvenanceWith("a", false, c)
	if err != nil {
		t.Fatalf("resolveAgentProvenanceWith: %v", err)
	}
	if !hex64.MatchString(got.PromptSHA256) {
		t.Errorf("hash %q is not 64 lowercase hex", got.PromptSHA256)
	}
	if got.PromptSHA256 == live.PromptSHA256 {
		t.Error("candidate with different content must change prompt_sha256")
	}
	if got.PromptFile != c.Path {
		t.Errorf("PromptFile = %q, want %q", got.PromptFile, c.Path)
	}
	if got.Model != live.Model || got.ConfigPath != live.ConfigPath {
		t.Errorf("model/config path changed: %+v vs %+v", got, live)
	}
	if live.PromptFile != "" {
		t.Errorf("live provenance PromptFile = %q, want empty", live.PromptFile)
	}
}

func TestCandidateProvenanceIdenticalContentSameHash(t *testing.T) {
	path := candProject(t, "live prompt") // byte-identical to the live prompt
	live := mustProv(t, "a", false)

	c, err := loadCandidatePrompt("a", path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveAgentProvenanceWith("a", false, c)
	if err != nil {
		t.Fatal(err)
	}
	if got.PromptSHA256 != live.PromptSHA256 {
		t.Errorf("identical content: sha %s != live %s", got.PromptSHA256, live.PromptSHA256)
	}
}

func TestCandidateProvenanceDoesNotReadLivePrompt(t *testing.T) {
	path := candProject(t, "candidate prompt")
	c, err := loadCandidatePrompt("a", path)
	if err != nil {
		t.Fatal(err)
	}
	withLive, err := resolveAgentProvenanceWith("a", false, c)
	if err != nil {
		t.Fatal(err)
	}
	// Without the override a missing live prompt is an error (existing behaviour)...
	if err := os.Remove(".kiro/agents/a-prompt.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveAgentProvenance("a", false); err == nil {
		t.Fatal("expected error resolving provenance with a missing live prompt")
	}
	// ...but with the override it is never read, and the hash is unchanged.
	got, err := resolveAgentProvenanceWith("a", false, c)
	if err != nil {
		t.Fatalf("live prompt must not be read with an override: %v", err)
	}
	if got.PromptSHA256 != withLive.PromptSHA256 {
		t.Error("hash depends on the live prompt file")
	}
}

func TestCandidateProvenanceNilOverrideMatchesPlain(t *testing.T) {
	candProject(t, "x")
	plain := mustProv(t, "a", false)
	with, err := resolveAgentProvenanceWith("a", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain.PromptSHA256 != with.PromptSHA256 || plain.PromptFile != with.PromptFile ||
		plain.Model != with.Model || plain.ConfigPath != with.ConfigPath {
		t.Errorf("nil override differs from plain: %+v vs %+v", with, plain)
	}
}

// ---- types / config ----

func TestCandidatePromptFileJSONFieldsOmitEmpty(t *testing.T) {
	for name, v := range map[string]any{
		"AgentResult":     AgentResult{},
		"AgentProvenance": AgentProvenance{},
		"Summary":         Summary{},
	} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "prompt_file") {
			t.Errorf("%s: prompt_file must be omitted when empty: %s", name, b)
		}
	}

	for name, v := range map[string]any{
		"AgentResult":     AgentResult{PromptFile: "p.md"},
		"AgentProvenance": AgentProvenance{PromptFile: "p.md"},
		"Summary":         Summary{PromptFile: "p.md"},
	} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"prompt_file":"p.md"`) {
			t.Errorf("%s: prompt_file missing when set: %s", name, b)
		}
	}
}

func TestCandidateRunConfigField(t *testing.T) {
	resetConfig()
	t.Cleanup(resetConfig)
	if cfg.candidate != nil {
		t.Error("default config must have no candidate")
	}
	cfg.candidate = &candidatePrompt{Path: "p.md"}
	if err := configure(RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if cfg.candidate != nil {
		t.Error("configure must reset candidate (no leakage between runs)")
	}
}
