package eval

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// resumePromptFileFixture writes an <agent>.json with saved cases, the given
// recorded prompt_file (omitted when empty) and a pin whose hash/model match
// it, so only prompt_file can drive a refusal.
func resumePromptFileFixture(t *testing.T, recordedFile, pinFile string) string {
	t.Helper()
	setupPinProject(t, "", "")
	resultsDir := ".kairon/evals/results/run1"
	recorded := ""
	if recordedFile != "" {
		recorded = `"prompt_file":"` + recordedFile + `",`
	}
	writeProjectFile(t, filepath.Join(resultsDir, "architect.json"),
		`{"agent":"architect","git_hash":"g","sandbox":false,`+
			`"agent_model":"claude-sonnet-5.5","judge_model":"claude-sonnet-5.5","prompt_sha256":"samehash",`+
			recorded+`"cases":[{"case_name":"c1"}]}`)
	cfg.pins = &runPins{
		Judge: "claude-sonnet-5.5",
		Agents: map[string]agentPin{"architect": {
			Model:      "claude-sonnet-5.5",
			Provenance: agentProvenance{PromptSHA256: "samehash", PromptFile: pinFile},
		}},
	}
	return resultsDir
}

func TestCheckResumeIntegrityPromptFile(t *testing.T) {
	t.Run("candidate run resumed without candidate is refused", func(t *testing.T) {
		dir := resumePromptFileFixture(t, "iter/a.md", "")
		err := checkResumeIntegrity(dir, "architect", false)
		if err == nil || !strings.Contains(err.Error(), "cannot resume") ||
			!strings.Contains(err.Error(), "iter/a.md") {
			t.Fatalf("err = %v, want refusal naming the recorded prompt_file iter/a.md", err)
		}
	})

	t.Run("different prompt_file with identical hash is refused", func(t *testing.T) {
		dir := resumePromptFileFixture(t, "iter/a.md", "iter/b.md")
		err := checkResumeIntegrity(dir, "architect", false)
		if err == nil || !strings.Contains(err.Error(), "cannot resume") ||
			!strings.Contains(err.Error(), "iter/a.md") || !strings.Contains(err.Error(), "iter/b.md") {
			t.Fatalf("err = %v, want refusal naming iter/a.md and iter/b.md", err)
		}
	})

	t.Run("legacy file without prompt_file resumed with a candidate is refused", func(t *testing.T) {
		dir := resumePromptFileFixture(t, "", "iter/b.md")
		err := checkResumeIntegrity(dir, "architect", false)
		if err == nil || !strings.Contains(err.Error(), "cannot resume") ||
			!strings.Contains(err.Error(), "iter/b.md") {
			t.Fatalf("err = %v, want refusal naming the current prompt_file iter/b.md", err)
		}
	})

	t.Run("identical prompt_file and hash resumes", func(t *testing.T) {
		dir := resumePromptFileFixture(t, "iter/a.md", "iter/a.md")
		if err := checkResumeIntegrity(dir, "architect", false); err != nil {
			t.Fatalf("identical prompt_file should resume: %v", err)
		}
	})

	t.Run("legacy file without prompt_file and no candidate resumes", func(t *testing.T) {
		dir := resumePromptFileFixture(t, "", "")
		if err := checkResumeIntegrity(dir, "architect", false); err != nil {
			t.Fatalf("legacy file without prompt_file should resume: %v", err)
		}
	})
}

func TestLoadRunInfoCarriesPromptFile(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, filepath.Join(dir, "architect.json"),
		`{"agent":"architect","agent_model":"m","prompt_sha256":"h1","prompt_file":"iter/from-agent-file.md"}`)

	// From the summary.
	info := loadRunInfo(Summary{Agents: map[string]AgentProvenance{
		"builder": {AgentModel: "m", PromptSHA256: "h2", PromptFile: "iter/from-summary.md"},
	}}, dir)
	if got := info.agents["builder"].promptFile; got != "iter/from-summary.md" {
		t.Errorf("summary promptFile = %q", got)
	}

	// From the agent file when the summary lacks it.
	if got := info.agents["architect"].promptFile; got != "iter/from-agent-file.md" {
		t.Errorf("agent-file promptFile = %q", got)
	}
}

func TestPrintProvenancePromptFileDifference(t *testing.T) {
	render := func(a, b runInfo) string {
		var buf bytes.Buffer
		printProvenance(&buf, "runA", "runB", a, b)
		return buf.String()
	}
	mk := func(file string) runInfo {
		return runInfo{
			mode: "native",
			agents: map[string]agentProv{
				"architect": {model: "m", sha: "same", promptFile: file},
			},
		}
	}

	t.Run("reports a difference", func(t *testing.T) {
		out := render(mk(""), mk("iter/v2.md"))
		if !strings.Contains(out, "architect prompt_file: "+notRecorded+" → iter/v2.md") {
			t.Errorf("missing prompt_file difference line:\n%s", out)
		}
		if strings.Contains(out, "No provenance differences") {
			t.Errorf("prompt_file difference not counted:\n%s", out)
		}
	})

	t.Run("two different candidates", func(t *testing.T) {
		out := render(mk("iter/v1.md"), mk("iter/v2.md"))
		if !strings.Contains(out, "architect prompt_file: iter/v1.md → iter/v2.md") {
			t.Errorf("missing prompt_file difference line:\n%s", out)
		}
	})

	t.Run("runs lacking the field are not reported", func(t *testing.T) {
		out := render(mk(""), mk(""))
		if strings.Contains(out, "prompt_file") {
			t.Errorf("unexpected prompt_file line for runs without the field:\n%s", out)
		}
		if !strings.Contains(out, "No provenance differences") {
			t.Errorf("want 'No provenance differences':\n%s", out)
		}
	})

	t.Run("identical prompt_file is not reported", func(t *testing.T) {
		out := render(mk("iter/v1.md"), mk("iter/v1.md"))
		if strings.Contains(out, "prompt_file") {
			t.Errorf("unexpected prompt_file line:\n%s", out)
		}
	})
}
