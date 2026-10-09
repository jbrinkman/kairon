package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeCandidate writes a candidate prompt file under a fresh temp dir and
// returns its path.
func writeCandidate(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cand.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadCandidatePrompt(t *testing.T) {
	t.Run("no prompt file is no candidate", func(t *testing.T) {
		c, err := loadCandidatePrompt("a", RunOptions{})
		if err != nil || c != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", c, err)
		}
		c, err = loadCandidatePrompt("", RunOptions{})
		if err != nil || c != nil {
			t.Fatalf("empty agent without prompt file: got (%v, %v), want (nil, nil)", c, err)
		}
	})

	t.Run("agent required", func(t *testing.T) {
		_, err := loadCandidatePrompt("", RunOptions{PromptFile: writeCandidate(t, "x")})
		if err == nil || !strings.Contains(err.Error(), "agent is required") {
			t.Fatalf("err = %v, want 'agent is required'", err)
		}
	})

	t.Run("missing file names the path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing.md")
		_, err := loadCandidatePrompt("a", RunOptions{PromptFile: missing})
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("err = %v, want it to name %s", err, missing)
		}
	})

	t.Run("directory names the path", func(t *testing.T) {
		dir := t.TempDir()
		_, err := loadCandidatePrompt("a", RunOptions{PromptFile: dir})
		if err == nil || !strings.Contains(err.Error(), dir) {
			t.Fatalf("err = %v, want it to name %s", err, dir)
		}
	})

	t.Run("empty file names the path", func(t *testing.T) {
		p := writeCandidate(t, "")
		_, err := loadCandidatePrompt("a", RunOptions{PromptFile: p})
		if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("err = %v, want an 'empty' error naming %s", err, p)
		}
	})

	t.Run("conflicts with list perf cleanup", func(t *testing.T) {
		p := writeCandidate(t, "x")
		for name, o := range map[string]RunOptions{
			"list":    {List: true, PromptFile: p},
			"perf":    {Perf: true, PromptFile: p},
			"cleanup": {Cleanup: true, PromptFile: p},
		} {
			if _, err := loadCandidatePrompt("a", o); err == nil || !strings.Contains(err.Error(), "--prompt-file") {
				t.Errorf("%s: err = %v, want a --prompt-file conflict error", name, err)
			}
		}
	})

	t.Run("valid file", func(t *testing.T) {
		p := writeCandidate(t, "hello candidate")
		c, err := loadCandidatePrompt("a", RunOptions{PromptFile: p})
		if err != nil || c == nil {
			t.Fatalf("got (%v, %v)", c, err)
		}
		if string(c.Content) != "hello candidate" {
			t.Errorf("Content = %q", c.Content)
		}
		if want := filepath.ToSlash(filepath.Clean(p)); c.Path != want {
			t.Errorf("Path = %q, want %q", c.Path, want)
		}
		if c.Agent != "a" {
			t.Errorf("Agent = %q", c.Agent)
		}
	})

	t.Run("path is cleaned and slash-normalised", func(t *testing.T) {
		p := writeCandidate(t, "x")
		c, err := loadCandidatePrompt("a", RunOptions{PromptFile: filepath.Dir(p) + "//./cand.md"})
		if err != nil || c == nil {
			t.Fatalf("got (%v, %v)", c, err)
		}
		if want := filepath.ToSlash(p); c.Path != want {
			t.Errorf("Path = %q, want %q", c.Path, want)
		}
	})
}

func TestRunWithOptionsPromptFileRejected(t *testing.T) {
	// copyFixturesTo's source is relative to the package dir, so copy the
	// fixtures before chdirTemp moves the working directory.
	evals := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, evals)
	dir := chdirTemp(t)
	resultsDir := filepath.Join(evals, "results")
	good := writeCandidate(t, "candidate")

	// Sentinel state: a rejected run must not touch cfg (configure not called).
	cfg.evalsDir = "sentinel-evals-dir"
	cfg.candidate = nil

	cases := map[string]struct {
		agent string
		opts  RunOptions
		want  string
	}{
		"agent empty":  {"", RunOptions{PromptFile: good}, "agent is required"},
		"missing":      {"selftest", RunOptions{PromptFile: filepath.Join(dir, "nope.md")}, "nope.md"},
		"directory":    {"selftest", RunOptions{PromptFile: dir}, dir},
		"empty file":   {"selftest", RunOptions{PromptFile: writeCandidate(t, "")}, "empty"},
		"with list":    {"selftest", RunOptions{List: true, PromptFile: good}, "--prompt-file"},
		"with perf":    {"selftest", RunOptions{Perf: true, PromptFile: good}, "--prompt-file"},
		"with cleanup": {"selftest", RunOptions{Cleanup: true, PromptFile: good}, "--prompt-file"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.opts.Backend = "stub"
			tc.opts.EvalsDir = evals
			err := RunWithOptions(tc.agent, "", tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
			if _, statErr := os.Stat(resultsDir); !os.IsNotExist(statErr) {
				t.Errorf("results dir exists after rejected run (stat err = %v)", statErr)
			}
			if cfg.evalsDir != "sentinel-evals-dir" || cfg.candidate != nil || cfg.pins != nil {
				t.Errorf("cfg changed by rejected run: %+v", cfg)
			}
		})
	}
}

// candProject is provProject plus the helpers to hash a candidate.
func provWithCandidate(t *testing.T, content string) agentProvenance {
	t.Helper()
	cfg.candidate = &candidatePrompt{Agent: "a", Path: "cand.md", Content: []byte(content)}
	t.Cleanup(func() { cfg.candidate = nil })
	return mustProv(t, "a", false)
}

func TestProvenanceCandidateHash(t *testing.T) {
	t.Run("differs from baseline when content differs", func(t *testing.T) {
		provProject(t, `[]`)
		base := mustProv(t, "a", false)
		cand := provWithCandidate(t, "prompt v2")
		if cand.PromptSHA256 == base.PromptSHA256 {
			t.Error("candidate with different content must change prompt_sha256")
		}
		if cand.PromptFile != "cand.md" {
			t.Errorf("PromptFile = %q", cand.PromptFile)
		}
		if base.PromptFile != "" {
			t.Errorf("baseline PromptFile = %q, want empty", base.PromptFile)
		}
	})

	t.Run("equals baseline when bytes are identical", func(t *testing.T) {
		provProject(t, `[]`)
		base := mustProv(t, "a", false)
		cand := provWithCandidate(t, "prompt v1")
		if cand.PromptSHA256 != base.PromptSHA256 {
			t.Errorf("identical bytes: %s != %s", cand.PromptSHA256, base.PromptSHA256)
		}
	})

	t.Run("equals hash of project whose live prompt was replaced", func(t *testing.T) {
		provProject(t, `[]`)
		cand := provWithCandidate(t, "prompt v2")
		cfg.candidate = nil
		writeProvFile(t, ".kiro/agents/a-prompt.md", "prompt v2")
		if replaced := mustProv(t, "a", false); replaced.PromptSHA256 != cand.PromptSHA256 {
			t.Errorf("replaced-live hash %s != candidate hash %s", replaced.PromptSHA256, cand.PromptSHA256)
		}
	})

	t.Run("live prompt file not required", func(t *testing.T) {
		provProject(t, `[]`)
		base := mustProv(t, "a", false)
		if err := os.Remove(".kiro/agents/a-prompt.md"); err != nil {
			t.Fatal(err)
		}
		cand := provWithCandidate(t, "prompt v1")
		if cand.PromptSHA256 != base.PromptSHA256 {
			t.Errorf("hash with absent live prompt %s != baseline %s", cand.PromptSHA256, base.PromptSHA256)
		}
	})

	t.Run("candidate for another agent is ignored", func(t *testing.T) {
		provProject(t, `[]`)
		base := mustProv(t, "a", false)
		cfg.candidate = &candidatePrompt{Agent: "other", Path: "c.md", Content: []byte("zzz")}
		t.Cleanup(func() { cfg.candidate = nil })
		got := mustProv(t, "a", false)
		if got.PromptSHA256 != base.PromptSHA256 || got.PromptFile != "" {
			t.Errorf("candidate for another agent leaked: %+v", got)
		}
	})
}

// TestProvenanceCandidateResourceAliasUsesCandidateBytes covers the case where
// a resources entry resolves to the very file --prompt-file replaces. The
// staged workspace serves the candidate for both the prompt and that resource,
// so provenance must hash the candidate for the aliasing resource too;
// otherwise editing the untouched live file changes prompt_sha256 and blocks
// --resume even though the agent saw identical content.
func TestProvenanceCandidateResourceAliasUsesCandidateBytes(t *testing.T) {
	// prompt file://./a-prompt.md and a resource that resolves to the same
	// on-disk file (.kiro/agents/a-prompt.md).
	setup := func(t *testing.T) {
		t.Helper()
		chdirTemp(t)
		writeProvFile(t, ".kiro/agents/a-prompt.md", "live prompt v1")
		writeProvFile(t, ".kiro/agents/a.json",
			`{"name":"a","model":"m","prompt":"file://./a-prompt.md","resources":["file://.kiro/agents/a-prompt.md"]}`)
	}

	t.Run("aliasing resource ignores live edits", func(t *testing.T) {
		setup(t)
		cfg.candidate = &candidatePrompt{Agent: "a", Path: "cand.md", Content: []byte("candidate bytes")}
		t.Cleanup(func() { cfg.candidate = nil })

		before := mustProv(t, "a", false).PromptSHA256
		// Edit the live file the candidate replaces: the hash must not move,
		// because both the prompt and the aliasing resource hash the candidate.
		writeProvFile(t, ".kiro/agents/a-prompt.md", "live prompt v2 (totally different)")
		after := mustProv(t, "a", false).PromptSHA256
		if before != after {
			t.Errorf("editing the replaced live file changed the hash: %s -> %s", before, after)
		}
		// And the hash must reflect the candidate content, not the live bytes.
		cfg.candidate = &candidatePrompt{Agent: "a", Path: "cand.md", Content: []byte("other candidate")}
		if other := mustProv(t, "a", false).PromptSHA256; other == after {
			t.Error("changing candidate content did not change the hash")
		}
	})

	t.Run("non-aliasing resource still reads the live file", func(t *testing.T) {
		chdirTemp(t)
		writeProvFile(t, ".kiro/agents/a-prompt.md", "live prompt")
		writeProvFile(t, "res.md", "res v1")
		writeProvFile(t, ".kiro/agents/a.json",
			`{"name":"a","model":"m","prompt":"file://./a-prompt.md","resources":["file://res.md"]}`)
		cfg.candidate = &candidatePrompt{Agent: "a", Path: "cand.md", Content: []byte("candidate bytes")}
		t.Cleanup(func() { cfg.candidate = nil })

		before := mustProv(t, "a", false).PromptSHA256
		writeProvFile(t, "res.md", "res v2")
		if after := mustProv(t, "a", false).PromptSHA256; after == before {
			t.Error("editing a non-aliasing resource must still change the hash")
		}
	})
}

func TestProvenanceCandidateRejectsUnsubstitutablePrompt(t *testing.T) {
	cases := map[string]struct {
		prompt string
		want   string
	}{
		"inline prompt":     {`"You are a helpful agent"`, "file://"},
		"absolute file ref": {`"file:///etc/prompt.md"`, "absolute"},
		"escapes .kiro":     {`"file://../../outside.md"`, ".kiro"},
		"empty file ref":    {`"file://"`, "file://"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			chdirTemp(t)
			writeProvFile(t, ".kiro/agents/a.json", `{"name":"a","model":"m","prompt":`+tc.prompt+`}`)
			cfg.candidate = &candidatePrompt{Agent: "a", Path: "c.md", Content: []byte("x")}
			t.Cleanup(func() { cfg.candidate = nil })
			_, err := resolveAgentProvenance("a", false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}

	t.Run("same configs are fine without a candidate", func(t *testing.T) {
		chdirTemp(t)
		writeProvFile(t, ".kiro/agents/a.json", `{"name":"a","model":"m","prompt":"inline"}`)
		if _, err := resolveAgentProvenance("a", false); err != nil {
			t.Fatalf("inline prompt without candidate must still work: %v", err)
		}
	})
}

func TestApplyToRecordsPromptFile(t *testing.T) {
	pins := &runPins{Judge: "j", Agents: map[string]agentPin{
		"a": {Model: "m", Provenance: agentProvenance{PromptSHA256: "h", PromptFile: "iter/v2.md"}},
		"b": {Model: "m", Provenance: agentProvenance{PromptSHA256: "h"}},
	}}

	r := AgentResult{Agent: "a"}
	pins.applyTo(&r)
	if r.PromptFile != "iter/v2.md" {
		t.Errorf("PromptFile = %q, want iter/v2.md", r.PromptFile)
	}
	raw, _ := json.Marshal(r)
	if !strings.Contains(string(raw), `"prompt_file":"iter/v2.md"`) {
		t.Errorf("json missing prompt_file: %s", raw)
	}

	r = AgentResult{Agent: "b"}
	pins.applyTo(&r)
	raw, _ = json.Marshal(r)
	if strings.Contains(string(raw), "prompt_file") {
		t.Errorf("prompt_file key must be absent without a candidate: %s", raw)
	}
}

func TestUpdateIncrementalSummaryPromptFile(t *testing.T) {
	dir := t.TempDir()

	with := filepath.Join(dir, "with.json")
	err := updateIncrementalSummary(with, AgentResult{
		Agent: "a", AgentModel: "m", PromptSHA256: "h", PromptFile: "iter/v2.md",
	}, "g")
	if err != nil {
		t.Fatal(err)
	}
	var s Summary
	readJSON(t, with, &s)
	if s.Agents["a"].PromptFile != "iter/v2.md" {
		t.Errorf("agents.a.prompt_file = %q", s.Agents["a"].PromptFile)
	}
	if s.PromptFile != "iter/v2.md" {
		t.Errorf("top-level prompt_file = %q (single agent)", s.PromptFile)
	}

	without := filepath.Join(dir, "without.json")
	if err := updateIncrementalSummary(without, AgentResult{Agent: "a", AgentModel: "m", PromptSHA256: "h"}, "g"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(without)
	if strings.Contains(string(raw), "prompt_file") {
		t.Errorf("prompt_file key must be absent without a candidate: %s", raw)
	}

	// Two agents: no ambiguous top-level prompt_file, per-agent kept.
	multi := filepath.Join(dir, "multi.json")
	if err := updateIncrementalSummary(multi, AgentResult{Agent: "a", PromptSHA256: "h", PromptFile: "x.md"}, "g"); err != nil {
		t.Fatal(err)
	}
	if err := updateIncrementalSummary(multi, AgentResult{Agent: "b", PromptSHA256: "h2"}, "g"); err != nil {
		t.Fatal(err)
	}
	var m Summary
	readJSON(t, multi, &m)
	if m.PromptFile != "" || m.Agents["a"].PromptFile != "x.md" || m.Agents["b"].PromptFile != "" {
		t.Errorf("multi-agent summary = top %q, a %q, b %q", m.PromptFile, m.Agents["a"].PromptFile, m.Agents["b"].PromptFile)
	}
}

// stageEnv extends wsEnv with two agents that reference a prompt file: "pa"
// (repo config .kiro/agents/pa.json) and "ov" (evals-dir overlay config
// evals/agents/ov.json, whose prompt lives next to it in the overlay). It
// returns the cwd. cfg.candidate is cleared on cleanup.
func stageEnv(t *testing.T) string {
	t.Helper()
	cwd := wsEnv(t)
	t.Cleanup(resetConfig)
	writeCfgFile(t, ".kiro/agents/pa.json", `{"name":"pa","prompt":"file://./pa-prompt.md"}`)
	writeCfgFile(t, ".kiro/agents/pa-prompt.md", "live pa prompt")
	writeCfgFile(t, "evals/agents/ov.json", `{"name":"ov","prompt":"file://./ov-prompt.md"}`)
	writeCfgFile(t, "evals/agents/ov-prompt.md", "overlay ov prompt")
	return cwd
}

func setCandidate(agent, content string) {
	cfg.candidate = &candidatePrompt{Agent: agent, Path: "cand.md", Content: []byte(content)}
}

func assertNotGroupOtherWritable(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o022 != 0 || perm&0o444 != 0o444 {
		t.Errorf("%s: mode %o, want world readable and not group/other writable", p, perm)
	}
}

func TestStageKiroCandidateRepoConfig(t *testing.T) {
	cwd := stageEnv(t)
	setCandidate("pa", "candidate pa prompt")

	livePrompt := filepath.Join(cwd, ".kiro", "agents", "pa-prompt.md")
	// Put the live prompt in the past so an accidental rewrite changes mtime.
	past := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(livePrompt, past, past); err != nil {
		t.Fatal(err)
	}
	liveBefore := snapshotTree(t, filepath.Join(cwd, ".kiro"))
	evalsBefore := snapshotTree(t, filepath.Join(cwd, "evals"))

	ws := newWS(t, TestCase{Name: "c"})
	staged := filepath.Join(ws.KiroDir, "agents", "pa-prompt.md")

	if got := readFileT(t, staged); got != "candidate pa prompt" {
		t.Errorf("staged prompt = %q, want the candidate bytes", got)
	}
	assertNotGroupOtherWritable(t, staged)

	// Everything else staged is the live agent.
	agents := filepath.Join(ws.KiroDir, "agents")
	if got := readFileT(t, filepath.Join(agents, "pa.json")); got != `{"name":"pa","prompt":"file://./pa-prompt.md"}` {
		t.Errorf("staged config changed: %q", got)
	}
	if got := readFileT(t, filepath.Join(agents, "other.json")); got != `{"from":"project-other"}` {
		t.Errorf("other.json = %q", got)
	}

	assertTreeEqual(t, liveBefore, snapshotTree(t, filepath.Join(cwd, ".kiro")), "live .kiro")
	assertTreeEqual(t, evalsBefore, snapshotTree(t, filepath.Join(cwd, "evals")), "evals dir")
	if got := readFileT(t, livePrompt); got != "live pa prompt" {
		t.Errorf("live prompt = %q", got)
	}
	info, err := os.Stat(livePrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Errorf("live prompt mtime = %v, want unchanged %v", info.ModTime(), past)
	}
}

func TestStageKiroCandidateEvalsOverlayConfig(t *testing.T) {
	cwd := stageEnv(t)
	setCandidate("ov", "candidate ov prompt")

	overlayPrompt := filepath.Join(cwd, "evals", "agents", "ov-prompt.md")
	past := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(overlayPrompt, past, past); err != nil {
		t.Fatal(err)
	}
	kiroBefore := snapshotTree(t, filepath.Join(cwd, ".kiro"))
	evalsBefore := snapshotTree(t, filepath.Join(cwd, "evals"))

	ws := newWS(t, TestCase{Name: "c"})
	staged := filepath.Join(ws.KiroDir, "agents", "ov-prompt.md")

	if got := readFileT(t, staged); got != "candidate ov prompt" {
		t.Errorf("staged overlay prompt = %q, want the candidate bytes", got)
	}
	assertNotGroupOtherWritable(t, staged)
	// A prompt of another agent is not touched.
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "pa-prompt.md")); got != "live pa prompt" {
		t.Errorf("other agent's prompt = %q, want live", got)
	}

	assertTreeEqual(t, kiroBefore, snapshotTree(t, filepath.Join(cwd, ".kiro")), "live .kiro")
	assertTreeEqual(t, evalsBefore, snapshotTree(t, filepath.Join(cwd, "evals")), "evals dir")
	info, err := os.Stat(overlayPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Errorf("evals-dir prompt mtime = %v, want unchanged %v", info.ModTime(), past)
	}
}

func TestStageKiroCandidateOverridesFixtureFile(t *testing.T) {
	stageEnv(t)
	writeCfgFile(t, "evals/fixtures/workspaces/f/.kiro/agents/pa-prompt.md", "fixture pa prompt")
	setCandidate("pa", "candidate pa prompt")

	ws := newWS(t, TestCase{Name: "c", Workspace: "f"})
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "pa-prompt.md")); got != "candidate pa prompt" {
		t.Errorf("candidate must win over the fixture file, got %q", got)
	}
}

func TestStageKiroWithoutCandidateUsesLivePrompt(t *testing.T) {
	stageEnv(t)
	ws := newWS(t, TestCase{Name: "c"})
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "pa-prompt.md")); got != "live pa prompt" {
		t.Errorf("pa-prompt.md = %q, want the live prompt", got)
	}
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "ov-prompt.md")); got != "overlay ov prompt" {
		t.Errorf("ov-prompt.md = %q, want the overlay prompt", got)
	}
}

func TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	stageEnv(t)
	kiro := filepath.Join(t.TempDir(), ".kiro")
	if err := os.MkdirAll(filepath.Join(kiro, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.md")
	if err := os.WriteFile(target, []byte("symlink target"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(kiro, "agents", "pa-prompt.md")
	if err := os.Symlink(target, dest); err != nil {
		t.Fatal(err)
	}

	w := &caseWorkspace{KiroDir: kiro}
	cand := &candidatePrompt{Agent: "pa", Path: "cand.md", Content: []byte("candidate pa prompt")}
	if err := stageCandidatePrompt(w, cand); err != nil {
		t.Fatalf("stageCandidatePrompt: %v", err)
	}

	if got := readFileT(t, target); got != "symlink target" {
		t.Errorf("symlink target was written through: %q", got)
	}
	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("destination mode %v, want a regular file replacing the symlink", info.Mode())
	}
	if got := readFileT(t, dest); got != "candidate pa prompt" {
		t.Errorf("destination = %q, want the candidate bytes", got)
	}
}

func TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough(t *testing.T) {
	stageEnv(t)
	kiro := filepath.Join(t.TempDir(), ".kiro")
	if err := os.MkdirAll(filepath.Join(kiro, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other.md")
	if err := os.WriteFile(other, []byte("hardlink peer"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(kiro, "agents", "pa-prompt.md")
	if err := os.Link(other, dest); err != nil {
		t.Skipf("hard links unsupported: %v", err)
	}

	w := &caseWorkspace{KiroDir: kiro}
	cand := &candidatePrompt{Agent: "pa", Path: "cand.md", Content: []byte("candidate pa prompt")}
	if err := stageCandidatePrompt(w, cand); err != nil {
		t.Fatalf("stageCandidatePrompt: %v", err)
	}
	if got := readFileT(t, other); got != "hardlink peer" {
		t.Errorf("hardlink peer was written through: %q", got)
	}
	if got := readFileT(t, dest); got != "candidate pa prompt" {
		t.Errorf("destination = %q, want the candidate bytes", got)
	}
}

func TestStageCandidatePromptRejectsUnsubstitutablePrompt(t *testing.T) {
	stageEnv(t)
	writeCfgFile(t, ".kiro/agents/inl.json", `{"name":"inl","prompt":"inline text"}`)
	w := &caseWorkspace{KiroDir: filepath.Join(t.TempDir(), ".kiro")}
	err := stageCandidatePrompt(w, &candidatePrompt{Agent: "inl", Path: "c.md", Content: []byte("x")})
	if err == nil || !strings.Contains(err.Error(), "inline prompt") {
		t.Errorf("err = %v, want an inline prompt error", err)
	}
	if _, serr := os.Stat(w.KiroDir); serr == nil {
		t.Errorf("nothing should be written on rejection")
	}
}
