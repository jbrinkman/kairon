package eval

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const pinFakeKiroBody = `cat >/dev/null
case "$*" in
*--agent*) echo "agent output" ;;
*) printf '===JSON_START===\n{"score": 4, "reasoning": "fine", "pass": true}\n===JSON_END===' ;;
esac`

// writeProjectFile writes content to path (relative to cwd), creating parents.
func writeProjectFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// initTempGitRepo turns the current (temp) directory into a git repo with one
// commit, so getGitShortHash works.
func initTempGitRepo(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v: %s", err, out)
		}
	}
}

// setupPinProject builds a temp project in cwd with an architect agent, one
// LLM-judged criterion and one case. evalsYAML is the content of the `evals:`
// block ("" for no config file).
func setupPinProject(t *testing.T, agentJSON, evalsYAML string) {
	t.Helper()
	chdirTemp(t)
	initTempGitRepo(t)
	if agentJSON == "" {
		agentJSON = `{"name":"architect","model":"claude-sonnet-5.5","prompt":"inline"}`
	}
	writeProjectFile(t, ".kiro/agents/architect.json", agentJSON)
	writeProjectFile(t, ".kairon/evals/rubrics/architect.yaml",
		"agent: architect\ncriteria:\n  - name: clarity\n    description: d\n    scoring: 1-5\n")
	writeProjectFile(t, ".kairon/evals/cases/architect/c1.yaml",
		"name: c1\ndescription: d\nagent: architect\ninput: do it\n")
	if evalsYAML != "" {
		writeProjectFile(t, ".kairon/config.yaml", evalsYAML)
	}
	t.Cleanup(resetConfig)
}

func TestPinSingleCaseModelFlags(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantAgent  string
		wantJudge  string
		wantRecord string
	}{
		{"defaults", "", "claude-sonnet-5.5", "claude-sonnet-5.5", "claude-sonnet-5.5"},
		{"agent override", "evals:\n  agent_model: claude-sonnet-4.5\n", "claude-sonnet-4.5", "claude-sonnet-5.5", "claude-sonnet-4.5"},
		{"judge override", "evals:\n  judge_model: claude-haiku-4.5\n", "claude-sonnet-5.5", "claude-haiku-4.5", "claude-sonnet-5.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupPinProject(t, "", tt.config)
			_, calls := installFakeKiroCLI(t, pinFakeKiroBody)

			if err := RunWithOptions("architect", "c1", RunOptions{}); err != nil {
				t.Fatalf("run: %v", err)
			}

			var agentLines, judgeLines int
			for _, line := range readCalls(t, calls) {
				switch {
				case strings.Contains(line, "--agent architect"):
					agentLines++
					if !strings.HasSuffix(line, "--model "+tt.wantAgent) {
						t.Errorf("agent call %q lacks --model %s", line, tt.wantAgent)
					}
				case strings.HasPrefix(line, "chat --no-interactive"):
					judgeLines++
					if !strings.HasSuffix(line, "--model "+tt.wantJudge) {
						t.Errorf("judge call %q lacks --model %s", line, tt.wantJudge)
					}
				}
			}
			if agentLines != 1 || judgeLines != 1 {
				t.Fatalf("agent calls=%d judge calls=%d, want 1 each", agentLines, judgeLines)
			}

			// Single-case runs write <agent>.json and summary.json with provenance.
			dirs, _ := filepath.Glob(".kairon/evals/results/*")
			if len(dirs) != 1 {
				t.Fatalf("results dirs = %v", dirs)
			}
			var res AgentResult
			readJSON(t, filepath.Join(dirs[0], "architect.json"), &res)
			if res.AgentModel != tt.wantAgent || res.JudgeModel != tt.wantJudge || len(res.PromptSHA256) != 64 {
				t.Errorf("result provenance = %+v", res)
			}
			var sum Summary
			readJSON(t, filepath.Join(dirs[0], "summary.json"), &sum)
			if sum.AgentModel != tt.wantAgent || sum.JudgeModel != tt.wantJudge || sum.PromptSHA256 != res.PromptSHA256 || sum.ResourcesPresent == nil {
				t.Errorf("summary provenance = %+v", sum)
			}
			if len(res.Cases) != 1 || len(res.Cases[0].Calls) != 2 {
				t.Fatalf("calls = %+v", res.Cases)
			}
			if got := res.Cases[0].Calls[0]; got.Role != "agent" || got.Model != tt.wantRecord || got.PromptSHA256 != res.PromptSHA256 {
				t.Errorf("agent record = %+v", got)
			}
			if got := res.Cases[0].Calls[1]; got.Role != "judge" || got.Model != tt.wantJudge || got.Criterion != "clarity" || got.PromptSHA256 != "" {
				t.Errorf("judge record = %+v", got)
			}
		})
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestPinRefusesBeforeAnyCase(t *testing.T) {
	noModel := `{"name":"architect","prompt":"inline"}`
	tests := []struct {
		name    string
		agent   string
		config  string
		backend string
		want    []string // substrings the error must contain
	}{
		{"judge auto", "", "evals:\n  judge_model: auto\n", "", []string{`"auto"`, "allowed_models", "evals.judge_model"}},
		{"judge empty", "", "evals:\n  judge_model: \"\"\n", "", []string{"empty", "allowed_models"}},
		{"judge not allowed", "", "evals:\n  judge_model: gpt-9\n", "", []string{`"gpt-9"`, "allowed_models", "claude-sonnet-5.5"}},
		{"agent no model", noModel, "", "", []string{"empty", "allowed_models", "architect"}},
		{"agent override auto", "", "evals:\n  agent_model: AUTO\n", "", []string{`"AUTO"`, "evals.agent_model"}},
		{"agent override not allowed", "", "evals:\n  agent_model: gpt-9\n", "", []string{`"gpt-9"`, "allowed_models"}},
		{"stub backend refused too", "", "evals:\n  judge_model: auto\n", "stub", []string{`"auto"`, "allowed_models"}},
		{"all violations together", noModel, "evals:\n  judge_model: auto\n", "", []string{"evals.judge_model", `agent "architect"`, `"auto"`, "empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupPinProject(t, tt.agent, tt.config)
			_, calls := installFakeKiroCLI(t, pinFakeKiroBody)

			for _, tc := range []string{"", "c1"} {
				err := RunWithOptions("architect", tc, RunOptions{Backend: tt.backend})
				if err == nil {
					t.Fatalf("case %q: expected refusal", tc)
				}
				for _, w := range tt.want {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q lacks %q", err, w)
					}
				}
				if cfg.pins != nil {
					t.Error("cfg.pins set after refusal")
				}
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

func TestPinRefusalWithoutAgentCoversRubricAgents(t *testing.T) {
	setupPinProject(t, `{"name":"architect","prompt":"inline"}`, "")
	installFakeKiroCLI(t, pinFakeKiroBody)
	err := RunWithOptions("", "", RunOptions{})
	if err == nil || !strings.Contains(err.Error(), `agent "architect"`) {
		t.Fatalf("err = %v, want architect model refusal", err)
	}
}

func TestPinDoesNotBlockListOrCleanup(t *testing.T) {
	setupPinProject(t, "", "evals:\n  judge_model: auto\n")
	installFakeKiroCLI(t, pinFakeKiroBody)
	if err := RunWithOptions("architect", "", RunOptions{List: true}); err != nil {
		t.Errorf("--list blocked by pinning: %v", err)
	}
	// --cleanup returns before the pre-flight; it must not report a pinning error.
	if err := RunWithOptions("", "", RunOptions{Cleanup: true}); err != nil && strings.Contains(err.Error(), "pinning") {
		t.Errorf("--cleanup blocked by pinning: %v", err)
	}
}

func TestPinRunSandboxIgnoresAgentModel(t *testing.T) {
	setupPinProject(t, "", "evals:\n  agent_model: claude-sonnet-4.5\n")

	if err := pinRun("architect", RunOptions{}, true); err != nil {
		t.Fatalf("pinRun(container): %v", err)
	}
	pin, ok := cfg.pins.pinOf("architect")
	if !ok || pin.Model != "claude-sonnet-5.5" {
		t.Errorf("container pin = %+v, want config model claude-sonnet-5.5", pin)
	}
	if cfg.pins.judgeModel() != "claude-sonnet-5.5" {
		t.Errorf("judge = %q", cfg.pins.judgeModel())
	}

	// The config model is validated, not the ignored override.
	if err := configure(RunOptions{}); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, ".kiro/agents/architect.json", `{"name":"architect","model":"gpt-9","prompt":"inline"}`)
	err := pinRun("architect", RunOptions{}, true)
	if err == nil || !strings.Contains(err.Error(), `"gpt-9"`) {
		t.Fatalf("err = %v, want config-model refusal", err)
	}

	// Non-container: the override applies.
	if err := configure(RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := pinRun("architect", RunOptions{}, false); err != nil {
		t.Fatalf("pinRun(host): %v", err)
	}
	if got := cfg.pins.agentModel("architect"); got != "claude-sonnet-4.5" {
		t.Errorf("host agent model = %q", got)
	}
}

func TestPinUnpinnedCallsOmitModel(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "cat >/dev/null\necho ok")
	if _, _, rec, _, err := invokeAgent("builder", "p", nil, nil); err != nil || rec.Role != "agent" || rec.Model != "" {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	if got := readCalls(t, calls); len(got) != 1 || strings.Contains(got[0], "--model") {
		t.Errorf("calls = %q", got)
	}
}

func TestCallRecordsFailedCallStillRecorded(t *testing.T) {
	chdirTemp(t)
	cfg.pins = &runPins{Judge: "j", Agents: map[string]agentPin{"a": {Model: "m"}}}
	installFakeKiroCLI(t, "cat >/dev/null\nexit 3")
	_, _, rec, _, err := invokeAgent("a", "p", nil, nil)
	if err == nil || rec.Error == "" || rec.Model != "m" {
		t.Errorf("err=%v rec=%+v", err, rec)
	}
	_, _, _, skipped, jrec := scoreLLMJudge(Criterion{Name: "c"}, TestCase{}, "x")
	if !skipped || jrec.Error == "" || jrec.Model != "j" || jrec.Criterion != "c" {
		t.Errorf("judge rec=%+v", jrec)
	}
}

// copySelfTestEvals copies the fixtures to a temp dir and returns its path.
func copySelfTestEvals(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, dir)
	return dir
}

// runSelfTestIn runs the stub self-test against evalsDir from the package
// directory and returns the run dir.
func runSelfTestIn(t *testing.T, evalsDir string) string {
	t.Helper()
	t.Cleanup(resetConfig)
	if err := RunWithOptions("selftest", "", RunOptions{Backend: "stub", EvalsDir: evalsDir}); err != nil {
		t.Fatalf("selftest run: %v", err)
	}
	dirs, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*"))
	if len(dirs) == 0 {
		t.Fatalf("no results under %s", evalsDir)
	}
	return dirs[len(dirs)-1]
}

func TestCallRecordsSelfTestProvenance(t *testing.T) {
	runDir := runSelfTestIn(t, copySelfTestEvals(t))

	raw, err := os.ReadFile(filepath.Join(runDir, "selftest.json"))
	if err != nil {
		t.Fatal(err)
	}
	// resources_present must serialise as [] (not be absent) for selftest.
	if !strings.Contains(string(raw), `"resources_present": []`) {
		t.Errorf("selftest.json lacks empty resources_present:\n%s", raw)
	}
	var res AgentResult
	readJSON(t, filepath.Join(runDir, "selftest.json"), &res)
	if res.AgentModel != "claude-sonnet-5.5" || res.JudgeModel != "claude-sonnet-5.5" || len(res.PromptSHA256) != 64 {
		t.Errorf("provenance = %+v", res)
	}
	if res.ResourcesPresent == nil || len(res.ResourcesPresent) != 0 {
		t.Errorf("resources_present = %#v, want []", res.ResourcesPresent)
	}

	for _, c := range res.Cases {
		var agents, judges int
		for _, call := range c.Calls {
			if call.Model == "" || call.InputTokens == 0 || call.OutputTokens == 0 || call.CostUSD == 0 {
				t.Errorf("case %s incomplete record: %+v", c.CaseName, call)
			}
			switch call.Role {
			case "agent":
				agents++
				if call.PromptSHA256 != res.PromptSHA256 || call.Agent != "selftest" {
					t.Errorf("case %s agent record = %+v", c.CaseName, call)
				}
				wantEst := c.CaseName != "stub-usage"
				if call.Estimated != wantEst {
					t.Errorf("case %s agent estimated = %v, want %v", c.CaseName, call.Estimated, wantEst)
				}
			case "judge":
				judges++
				if call.PromptSHA256 != "" || call.Criterion != "clarity" || !call.Estimated {
					t.Errorf("case %s judge record = %+v", c.CaseName, call)
				}
			}
		}
		if agents != 1 || judges != 1 {
			t.Errorf("case %s: agent=%d judge=%d records, want 1 each", c.CaseName, agents, judges)
		}
	}
	rawCalls := string(raw)
	for _, f := range []string{`"duration_ms"`, `"input_tokens"`, `"output_tokens"`, `"cost_usd"`, `"estimated"`} {
		if !strings.Contains(rawCalls, f) {
			t.Errorf("selftest.json lacks %s", f)
		}
	}

	var sum Summary
	readJSON(t, filepath.Join(runDir, "summary.json"), &sum)
	if sum.JudgeModel != "claude-sonnet-5.5" || sum.AgentModel != "claude-sonnet-5.5" || sum.PromptSHA256 != res.PromptSHA256 {
		t.Errorf("summary = %+v", sum)
	}
	if sum.ResourcesPresent == nil || len(sum.ResourcesPresent) != 0 {
		t.Errorf("summary resources_present = %#v", sum.ResourcesPresent)
	}
	sraw, _ := os.ReadFile(filepath.Join(runDir, "summary.json"))
	if !strings.Contains(string(sraw), `"resources_present": []`) {
		t.Errorf("summary.json lacks empty resources_present:\n%s", sraw)
	}
	if a, ok := sum.Agents["selftest"]; !ok || a.PromptSHA256 != res.PromptSHA256 {
		t.Errorf("summary agents = %+v", sum.Agents)
	}
}

func TestProvenancePromptEditChangesHash(t *testing.T) {
	hashOf := func(evalsDir string) string {
		var res AgentResult
		readJSON(t, filepath.Join(runSelfTestIn(t, evalsDir), "selftest.json"), &res)
		return res.PromptSHA256
	}
	base := copySelfTestEvals(t)
	same := copySelfTestEvals(t)
	edited := copySelfTestEvals(t)
	promptPath := filepath.Join(edited, "agents", "selftest-prompt.md")
	data, _ := os.ReadFile(promptPath)
	if err := os.WriteFile(promptPath, append(data, []byte("\nExtra instruction.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	h1, h2, h3 := hashOf(base), hashOf(same), hashOf(edited)
	if h1 != h2 {
		t.Errorf("unedited copy changed hash: %s vs %s", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("editing the prompt did not change the hash (%s)", h1)
	}
}

func TestProvenanceMissingConventionsResourceOmitted(t *testing.T) {
	setupPinProject(t, `{"name":"architect","model":"claude-sonnet-5.5","prompt":"inline",
		"resources":["file://README.md","skill://.kiro/skills/architect-conventions/SKILL.md"]}`, "")
	writeProjectFile(t, "README.md", "hi")
	installFakeKiroCLI(t, pinFakeKiroBody)
	if err := RunWithOptions("architect", "c1", RunOptions{}); err != nil {
		t.Fatal(err)
	}
	dirs, _ := filepath.Glob(".kairon/evals/results/*")
	var res AgentResult
	readJSON(t, filepath.Join(dirs[0], "architect.json"), &res)
	if len(res.ResourcesPresent) != 1 || res.ResourcesPresent[0] != "file://README.md" {
		t.Errorf("resources_present = %v", res.ResourcesPresent)
	}
}

func TestSummaryMultiAgentOmitsTopLevelAgentFields(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "summary.json")
	mk := func(agent, sha string) AgentResult {
		return AgentResult{Agent: agent, AgentModel: "m-" + agent, JudgeModel: "j", PromptSHA256: sha, ResourcesPresent: []string{}}
	}
	if err := updateIncrementalSummary(file, mk("a", "h1"), "g"); err != nil {
		t.Fatal(err)
	}
	var s Summary
	readJSON(t, file, &s)
	if s.AgentModel != "m-a" || s.PromptSHA256 != "h1" {
		t.Errorf("single agent summary = %+v", s)
	}
	if err := updateIncrementalSummary(file, mk("b", "h2"), "g"); err != nil {
		t.Fatal(err)
	}
	s = Summary{}
	readJSON(t, file, &s)
	if s.AgentModel != "" || s.PromptSHA256 != "" || s.ResourcesPresent != nil {
		t.Errorf("multi-agent summary has top-level agent fields: %+v", s)
	}
	if s.JudgeModel != "j" || len(s.Agents) != 2 || s.Agents["b"].AgentModel != "m-b" {
		t.Errorf("multi-agent summary = %+v", s)
	}
	raw, _ := os.ReadFile(file)
	if strings.Contains(string(raw), `"resources_present"`) && !strings.Contains(string(raw), `"agents"`) {
		t.Errorf("unexpected summary JSON: %s", raw)
	}
}

func TestPinResumeRefusedWhenPromptOrModelsChanged(t *testing.T) {
	setupPinProject(t, `{"name":"architect","model":"claude-sonnet-5.5","prompt":"file://p.md"}`, "")
	writeProjectFile(t, ".kiro/agents/p.md", "v1")
	installFakeKiroCLI(t, pinFakeKiroBody)

	if err := RunWithOptions("architect", "", RunOptions{}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	dirs, _ := filepath.Glob(".kairon/evals/results/*")
	if len(dirs) != 1 {
		t.Fatalf("dirs = %v", dirs)
	}
	// Simulate an interrupted run.
	writeProjectFile(t, filepath.Join(dirs[0], ".progress"), "in-progress")

	// Unchanged: resume continues.
	if err := RunWithOptions("architect", "", RunOptions{Resume: true}); err != nil {
		t.Fatalf("unchanged resume: %v", err)
	}
	writeProjectFile(t, filepath.Join(dirs[0], ".progress"), "in-progress")

	// Prompt changed: refused, result file untouched.
	before, _ := os.ReadFile(filepath.Join(dirs[0], "architect.json"))
	writeProjectFile(t, ".kiro/agents/p.md", "v2")
	err := RunWithOptions("architect", "", RunOptions{Resume: true})
	if err == nil || !strings.Contains(err.Error(), "cannot resume") {
		t.Fatalf("err = %v, want resume refusal", err)
	}
	after, _ := os.ReadFile(filepath.Join(dirs[0], "architect.json"))
	if string(before) != string(after) {
		t.Error("result file changed despite refusal")
	}

	// Models changed: refused.
	writeProjectFile(t, ".kiro/agents/p.md", "v1")
	writeProjectFile(t, ".kairon/config.yaml", "evals:\n  judge_model: claude-haiku-4.5\n")
	err = RunWithOptions("architect", "", RunOptions{Resume: true})
	if err == nil || !strings.Contains(err.Error(), "cannot resume") {
		t.Fatalf("err = %v, want resume refusal on model change", err)
	}
}
