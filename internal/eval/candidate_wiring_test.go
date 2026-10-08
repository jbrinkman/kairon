package eval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- workspace staging ------------------------------------------------------

// candWS sets up wsEnv plus a project prompt file and returns a candidate
// targeting it. cfg.candidate is set after wsEnv (configure resets it).
func candWS(t *testing.T, content string) *candidatePrompt {
	t.Helper()
	wsEnv(t)
	writeCfgFile(t, ".kiro/agents/p-prompt.md", "live prompt")
	c := &candidatePrompt{
		Path:    "iterations/p/v2.md",
		Content: []byte(content),
		Target:  filepath.Join("agents", "p-prompt.md"),
	}
	cfg.candidate = c
	t.Cleanup(resetConfig)
	return c
}

func TestCandidateWorkspaceStagesCandidateBytes(t *testing.T) {
	candWS(t, "candidate prompt\nline 2\n")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, filepath.Join(cwd, ".kiro"))

	ws := newWS(t, TestCase{Name: "c"})
	got, err := os.ReadFile(filepath.Join(ws.KiroDir, "agents", "p-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("candidate prompt\nline 2\n")) {
		t.Errorf("staged prompt = %q, want candidate bytes", got)
	}
	assertTreeEqual(t, before, snapshotTree(t, filepath.Join(cwd, ".kiro")), "project .kiro")
	// Other staged agent files are untouched.
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "other.json")); got != `{"from":"project-other"}` {
		t.Errorf("other.json = %q", got)
	}
}

func TestCandidateWorkspaceOverridesFixtureFile(t *testing.T) {
	candWS(t, "candidate")
	writeCfgFile(t, "evals/fixtures/workspaces/f/.kiro/agents/p-prompt.md", "fixture prompt")

	ws := newWS(t, TestCase{Name: "c", Workspace: "f"})
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "p-prompt.md")); got != "candidate" {
		t.Errorf("candidate must win over fixture, got %q", got)
	}
	if got := readFileT(t, "evals/fixtures/workspaces/f/.kiro/agents/p-prompt.md"); got != "fixture prompt" {
		t.Errorf("fixture file modified: %q", got)
	}
}

func TestCandidateWorkspaceOverridesOverlayFile(t *testing.T) {
	candWS(t, "candidate")
	writeCfgFile(t, "evals/agents/p-prompt.md", "overlay prompt")

	ws := newWS(t, TestCase{Name: "c"})
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "p-prompt.md")); got != "candidate" {
		t.Errorf("candidate must win over overlay, got %q", got)
	}
	if got := readFileT(t, "evals/agents/p-prompt.md"); got != "overlay prompt" {
		t.Errorf("overlay file modified: %q", got)
	}
	if got := readFileT(t, ".kiro/agents/p-prompt.md"); got != "live prompt" {
		t.Errorf("live prompt modified: %q", got)
	}
}

func TestCandidateWorkspaceOverlaySourcedAgentConfig(t *testing.T) {
	// The agent config and its prompt exist only in the evals overlay; the
	// project .kiro/agents has neither.
	wsEnv(t)
	writeCfgFile(t, "evals/agents/ov.json", `{"name":"ov","model":"claude-sonnet-5.5","prompt":"file://./ov-prompt.md"}`)
	writeCfgFile(t, "evals/agents/ov-prompt.md", "overlay prompt")
	writeCfgFile(t, "cand.md", "candidate for overlay agent")
	t.Cleanup(resetConfig)

	c, err := loadCandidatePrompt("ov", "cand.md")
	if err != nil {
		t.Fatalf("loadCandidatePrompt: %v", err)
	}
	cfg.candidate = c

	ws := newWS(t, TestCase{Name: "c"})
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "ov-prompt.md")); got != "candidate for overlay agent" {
		t.Errorf("staged overlay-agent prompt = %q", got)
	}
	if got := readFileT(t, "evals/agents/ov-prompt.md"); got != "overlay prompt" {
		t.Errorf("overlay prompt modified: %q", got)
	}
}

func TestCandidateWorkspaceNoCandidateUnchanged(t *testing.T) {
	wsEnv(t)
	writeCfgFile(t, ".kiro/agents/p-prompt.md", "live prompt")
	t.Cleanup(resetConfig)

	ws := newWS(t, TestCase{Name: "c"})
	if got := readFileT(t, filepath.Join(ws.KiroDir, "agents", "p-prompt.md")); got != "live prompt" {
		t.Errorf("without a candidate the live prompt must be staged, got %q", got)
	}
}

func TestCandidateWorkspaceStagedFileNotWorldWritable(t *testing.T) {
	candWS(t, "candidate")
	ws := newWS(t, TestCase{Name: "c"})
	info, err := os.Stat(filepath.Join(ws.KiroDir, "agents", "p-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o022 != 0 {
		t.Errorf("staged candidate mode = %v, want not group/world-writable", info.Mode().Perm())
	}
}

// --- RunWithOptions ---------------------------------------------------------

const (
	candLive      = "live prompt v1"
	candCandidate = "candidate prompt v2"
)

// candRunProject builds a temp project (two cases) whose agent uses a
// relative file:// prompt, a candidate file, and a fake kiro-cli that appends
// the prompt file it can see in its cwd to a log. It returns the calls log
// and the seen-prompts log.
func candRunProject(t *testing.T) (calls, seen string) {
	t.Helper()
	setupPinProject(t, `{"name":"architect","model":"claude-sonnet-5.5","prompt":"file://./p.md"}`, "")
	writeProjectFile(t, ".kiro/agents/p.md", candLive)
	writeProjectFile(t, ".kairon/evals/cases/architect/c2.yaml",
		"name: c2\ndescription: d\nagent: architect\ninput: do it too\n")
	writeProjectFile(t, ".kairon/iterations/architect/v2.md", candCandidate)

	seen = filepath.Join(t.TempDir(), "seen.log")
	body := `cat >/dev/null
case "$*" in
*--agent*) { cat .kiro/agents/p.md; echo "|"; } >> ` + seen + `; echo "agent output" ;;
*) printf '===JSON_START===\n{"score": 4, "reasoning": "fine", "pass": true}\n===JSON_END===' ;;
esac`
	_, calls = installFakeKiroCLI(t, body)
	return calls, seen
}

const candFile = ".kairon/iterations/architect/v2.md"

func latestRunDir(t *testing.T) string {
	t.Helper()
	dirs, _ := filepath.Glob(".kairon/evals/results/*")
	if len(dirs) == 0 {
		t.Fatal("no results dir")
	}
	return dirs[len(dirs)-1]
}

func TestCandidateRunUsesCandidateInEveryCase(t *testing.T) {
	_, seen := candRunProject(t)
	liveBefore := readFileT(t, ".kiro/agents/p.md")
	confBefore := readFileT(t, ".kiro/agents/architect.json")

	if err := RunWithOptions("architect", "", RunOptions{PromptFile: candFile}); err != nil {
		t.Fatalf("RunWithOptions: %v", err)
	}

	got := readFileT(t, seen)
	if want := strings.Repeat(candCandidate+"|\n", 2); got != want {
		t.Errorf("prompts seen by the agent = %q, want the candidate for both cases (%q)", got, want)
	}
	if readFileT(t, ".kiro/agents/p.md") != liveBefore || readFileT(t, ".kiro/agents/architect.json") != confBefore {
		t.Error("project .kiro/agents changed")
	}
}

func TestCandidateRunRecordsPromptFileAndHash(t *testing.T) {
	candRunProject(t)

	// Baseline (no candidate).
	if err := RunWithOptions("architect", "", RunOptions{}); err != nil {
		t.Fatalf("baseline run: %v", err)
	}
	baseDir := latestRunDir(t)
	for _, f := range []string{"architect.json", "summary.json"} {
		if raw := readFileT(t, filepath.Join(baseDir, f)); strings.Contains(raw, "prompt_file") {
			t.Errorf("baseline %s contains prompt_file:\n%s", f, raw)
		}
	}
	var baseRes AgentResult
	readJSON(t, filepath.Join(baseDir, "architect.json"), &baseRes)

	// Candidate run, in a fresh results dir.
	if err := os.RemoveAll(".kairon/evals/results"); err != nil {
		t.Fatal(err)
	}
	if err := RunWithOptions("architect", "", RunOptions{PromptFile: candFile}); err != nil {
		t.Fatalf("candidate run: %v", err)
	}
	dir := latestRunDir(t)

	want, err := resolveAgentProvenanceWith("architect", false, &candidatePrompt{Path: candFile, Content: []byte(candCandidate)})
	if err != nil {
		t.Fatal(err)
	}
	var res AgentResult
	readJSON(t, filepath.Join(dir, "architect.json"), &res)
	if res.PromptFile != candFile {
		t.Errorf("architect.json prompt_file = %q, want %q", res.PromptFile, candFile)
	}
	if res.PromptSHA256 != want.PromptSHA256 || res.PromptSHA256 == baseRes.PromptSHA256 {
		t.Errorf("architect.json prompt_sha256 = %s, want candidate hash %s (baseline %s)", res.PromptSHA256, want.PromptSHA256, baseRes.PromptSHA256)
	}

	var sum Summary
	readJSON(t, filepath.Join(dir, "summary.json"), &sum)
	if sum.PromptFile != candFile || sum.PromptSHA256 != want.PromptSHA256 {
		t.Errorf("summary top level = file %q sha %q", sum.PromptFile, sum.PromptSHA256)
	}
	if a := sum.Agents["architect"]; a.PromptFile != candFile || a.PromptSHA256 != want.PromptSHA256 {
		t.Errorf("summary.agents.architect = %+v", a)
	}
	// Each case's agent call record carries the candidate hash too.
	for _, c := range res.Cases {
		for _, call := range c.Calls {
			if call.Role == "agent" && call.PromptSHA256 != want.PromptSHA256 {
				t.Errorf("case %s call prompt_sha256 = %s", c.CaseName, call.PromptSHA256)
			}
		}
	}
}

func TestCandidateSingleCaseRunRecordsPromptFile(t *testing.T) {
	_, seen := candRunProject(t)

	if err := RunWithOptions("architect", "c1", RunOptions{PromptFile: candFile}); err != nil {
		t.Fatalf("single-case run: %v", err)
	}
	if got := readFileT(t, seen); got != candCandidate+"|\n" {
		t.Errorf("prompt seen = %q", got)
	}
	dir := latestRunDir(t)
	var res AgentResult
	readJSON(t, filepath.Join(dir, "architect.json"), &res)
	var sum Summary
	readJSON(t, filepath.Join(dir, "summary.json"), &sum)
	if res.PromptFile != candFile || sum.PromptFile != candFile || sum.Agents["architect"].PromptFile != candFile {
		t.Errorf("prompt_file: result=%q summary=%q agents=%q", res.PromptFile, sum.PromptFile, sum.Agents["architect"].PromptFile)
	}
}

func TestCandidateResumeRequiresSamePromptFile(t *testing.T) {
	candRunProject(t)

	if err := RunWithOptions("architect", "", RunOptions{PromptFile: candFile}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	dir := latestRunDir(t)
	simulateInterrupted := func() { writeProjectFile(t, filepath.Join(dir, ".progress"), "in-progress") }

	// Same candidate: resume continues and still records prompt_file.
	simulateInterrupted()
	if err := RunWithOptions("architect", "", RunOptions{Resume: true, PromptFile: candFile}); err != nil {
		t.Fatalf("resume with same candidate: %v", err)
	}
	var res AgentResult
	readJSON(t, filepath.Join(dir, "architect.json"), &res)
	var sum Summary
	readJSON(t, filepath.Join(dir, "summary.json"), &sum)
	if res.PromptFile != candFile || sum.PromptFile != candFile || sum.Agents["architect"].PromptFile != candFile {
		t.Errorf("prompt_file lost on resume: result=%q summary=%q agents=%q", res.PromptFile, sum.PromptFile, sum.Agents["architect"].PromptFile)
	}

	// Without the candidate: refused with the existing prompt-changed error.
	simulateInterrupted()
	before := readFileT(t, filepath.Join(dir, "architect.json"))
	err := RunWithOptions("architect", "", RunOptions{Resume: true})
	if err == nil || !strings.Contains(err.Error(), "cannot resume") || !strings.Contains(err.Error(), "prompt or models changed") {
		t.Fatalf("resume without --prompt-file: err = %v, want prompt-changed refusal", err)
	}
	if readFileT(t, filepath.Join(dir, "architect.json")) != before {
		t.Error("result file changed despite refusal")
	}
}

func TestCandidateRunRefusalsCreateNothing(t *testing.T) {
	tests := []struct {
		name    string
		agent   string
		opts    RunOptions
		setup   func(t *testing.T)
		wantErr []string
	}{
		{"no agent", "", RunOptions{PromptFile: candFile}, nil, []string{"an agent is required"}},
		{"missing file", "architect", RunOptions{PromptFile: "nope.md"}, nil, []string{"--prompt-file"}},
		{"empty file", "architect", RunOptions{PromptFile: candFile}, func(t *testing.T) {
			writeProjectFile(t, candFile, "  \n")
		}, []string{"empty"}},
		{"inline prompt", "architect", RunOptions{PromptFile: candFile}, func(t *testing.T) {
			writeProjectFile(t, ".kiro/agents/architect.json", `{"name":"architect","model":"claude-sonnet-5.5","prompt":"inline"}`)
		}, []string{"file://"}},
		{"with list", "architect", RunOptions{PromptFile: candFile, List: true}, nil, []string{"--list"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, _ := candRunProject(t)
			if tt.setup != nil {
				tt.setup(t)
			}
			for _, tc := range []string{"", "c1"} {
				err := RunWithOptions(tt.agent, tc, tt.opts)
				if err == nil {
					t.Fatalf("case %q: expected refusal", tc)
				}
				for _, w := range tt.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q lacks %q", err, w)
					}
				}
			}
			if cfg.pins != nil {
				t.Error("cfg.pins set after refusal")
			}
			if got := readCalls(t, calls); len(got) != 0 {
				t.Errorf("kiro-cli invoked: %q", got)
			}
			if _, err := os.Stat(".kairon/evals/results"); !os.IsNotExist(err) {
				t.Errorf("results dir exists after refusal (stat err=%v)", err)
			}
		})
	}
}

func TestCandidateOptionValidationRunsFirstAndLoadBeforePin(t *testing.T) {
	// Option validation precedes configure: a bad backend does not mask it.
	candRunProject(t)
	err := RunWithOptions("", "", RunOptions{PromptFile: candFile, Backend: "no-such-backend"})
	if err == nil || !strings.Contains(err.Error(), "an agent is required") {
		t.Errorf("err = %v, want agent-required refusal before backend configuration", err)
	}

	// Candidate load precedes pinRun: with a pinning violation as well, the
	// missing candidate is what is reported.
	setupPinProject(t, "", "evals:\n  judge_model: auto\n")
	err = RunWithOptions("architect", "", RunOptions{PromptFile: "nope.md"})
	if err == nil || !strings.Contains(err.Error(), "--prompt-file") || strings.Contains(err.Error(), "pinning") {
		t.Errorf("err = %v, want candidate refusal before pinning", err)
	}
}

func TestCandidatePinRunPrintsPromptFile(t *testing.T) {
	candRunProject(t)
	// pinRun reads cfg.candidate (set by RunWithOptions after configure).
	c, err := loadCandidatePrompt("architect", candFile)
	if err != nil {
		t.Fatal(err)
	}
	cfg.candidate = c

	out := captureStdout(t, func() {
		if err := pinRun("architect", RunOptions{}, false); err != nil {
			t.Fatalf("pinRun: %v", err)
		}
	})
	if !strings.Contains(out, "🔒 Models:") || !strings.Contains(out, candFile) {
		t.Errorf("pinRun output lacks the prompt file:\n%s", out)
	}
	if cfg.pins.Agents["architect"].Provenance.PromptFile != candFile {
		t.Errorf("pin PromptFile = %q", cfg.pins.Agents["architect"].Provenance.PromptFile)
	}

	// Without a candidate the line does not mention a prompt file.
	cfg.candidate = nil
	out = captureStdout(t, func() {
		if err := pinRun("architect", RunOptions{}, false); err != nil {
			t.Fatalf("pinRun: %v", err)
		}
	})
	if strings.Contains(out, "prompt file") || strings.Contains(out, candFile) {
		t.Errorf("baseline pinRun output mentions a prompt file:\n%s", out)
	}
}

func TestCandidateApplyToAndSummaryCarryPromptFile(t *testing.T) {
	p := &runPins{Judge: "j", Agents: map[string]agentPin{
		"a": {Model: "m", Provenance: agentProvenance{PromptSHA256: "abc", PromptFile: "x/v2.md", ResourcesPresent: []string{}}},
		"b": {Model: "m", Provenance: agentProvenance{PromptSHA256: "def", ResourcesPresent: []string{}}},
	}}
	var ra, rb AgentResult
	ra.Agent, rb.Agent = "a", "b"
	p.applyTo(&ra)
	p.applyTo(&rb)
	if ra.PromptFile != "x/v2.md" || rb.PromptFile != "" {
		t.Errorf("applyTo PromptFile: a=%q b=%q", ra.PromptFile, rb.PromptFile)
	}

	// Multi-agent: per-agent entries carry it, top level does not.
	summary := filepath.Join(t.TempDir(), "summary.json")
	if err := updateIncrementalSummary(summary, ra, "g"); err != nil {
		t.Fatal(err)
	}
	if err := updateIncrementalSummary(summary, rb, "g"); err != nil {
		t.Fatal(err)
	}
	var sum Summary
	readJSON(t, summary, &sum)
	if sum.Agents["a"].PromptFile != "x/v2.md" || sum.Agents["b"].PromptFile != "" || sum.PromptFile != "" {
		t.Errorf("summary = %+v", sum)
	}
}
