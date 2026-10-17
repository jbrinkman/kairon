package eval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wireEnv stages a workflow project in a temp git repo: a dispatcher "wf"
// trusting "step-a", whose config lists a skill resource that only a fixture
// workspace supplies. The run is pinned for "wf".
func wireEnv(t *testing.T) {
	t.Helper()
	execEnv(t, false)
	writeCfgFile(t, ".kiro/agents/wf.json",
		`{"name":"wf","model":"claude-sonnet-5.5","prompt":"inline","toolsSettings":{"subagent":{"trustedAgents":["step-a"]}}}`)
	writeCfgFile(t, ".kiro/agents/step-a.json",
		`{"name":"step-a","model":"claude-haiku-4.5","prompt":"inline step","resources":["skill://.kiro/skills/step-conventions/SKILL.md"]}`)
	gitTrack(t, ".kiro/agents/wf.json", ".kiro/agents/step-a.json")
	writeCfgFile(t, "evals/fixtures/workspaces/conv/.kiro/skills/step-conventions/SKILL.md", "fixture override\n")
	writeCfgFile(t, "evals/fixtures/workspaces/conv/README.md", "# conv\n")
	if err := pinRun("wf", RunOptions{}, false); err != nil {
		t.Fatal(err)
	}
}

func wireRubric() Rubric {
	return Rubric{Agent: "wf", Criteria: []Criterion{{Name: "clarity", Description: "clear", Scoring: "1-5"}}}
}

func TestExecuteCaseWorkflowProvenanceFromStagedWorkspace(t *testing.T) {
	wireEnv(t)
	pin, ok := cfg.pins.pinOf("wf")
	if !ok {
		t.Fatal("run is not pinned")
	}

	var out bytes.Buffer
	plain := executeCase(wireRubric(), stubCase("plain", "", "", "ok"), nil, &out, false)
	withFix := executeCase(wireRubric(), stubCase("fix", "conv", "", "ok"), nil, &out, false)

	if plain.WorkflowSHA256 == "" || len(plain.SubAgents) != 1 {
		t.Fatalf("plain case: workflow_sha256=%q sub_agents=%v\n%s", plain.WorkflowSHA256, plain.SubAgents, out.String())
	}
	if plain.WorkflowSHA256 != pin.Provenance.WorkflowSHA256 {
		t.Errorf("plain workflow_sha256 = %q, want baseline %q", plain.WorkflowSHA256, pin.Provenance.WorkflowSHA256)
	}
	if got := plain.SubAgents["step-a"]; got.AgentModel != "claude-haiku-4.5" || got.PromptSHA256 == "" ||
		got.ResourcesPresent == nil || len(got.ResourcesPresent) != 0 {
		t.Errorf("plain step-a = %+v, want haiku model, a hash and resources_present []", got)
	}

	// The fixture supplies the sub-agent's skill, so what that step sees (and
	// the workflow hash) differs from a case without it.
	if len(withFix.SubAgents["step-a"].ResourcesPresent) != 1 {
		t.Errorf("fixture step-a resources_present = %v, want the fixture skill", withFix.SubAgents["step-a"].ResourcesPresent)
	}
	if withFix.WorkflowSHA256 == "" || withFix.WorkflowSHA256 == plain.WorkflowSHA256 {
		t.Errorf("fixture workflow_sha256 = %q, must differ from the no-fixture %q", withFix.WorkflowSHA256, plain.WorkflowSHA256)
	}
	if withFix.SubAgents["step-a"].PromptSHA256 == plain.SubAgents["step-a"].PromptSHA256 {
		t.Error("fixture did not change the sub-agent prompt_sha256")
	}
	// The dispatcher's own hash is unaffected by the sub-agent's fixture.
	if withFix.PromptSHA256 != plain.PromptSHA256 {
		t.Errorf("dispatcher prompt_sha256 changed: %q vs %q", withFix.PromptSHA256, plain.PromptSHA256)
	}
}

func TestRunPinsApplyToCopiesWorkflowProvenance(t *testing.T) {
	subs := map[string]SubAgentProvenance{"s": {AgentModel: "m", PromptSHA256: "h", ResourcesPresent: []string{}}}
	pins := &runPins{Judge: "j", Agents: map[string]agentPin{
		"wf": {Model: "m", Provenance: agentProvenance{PromptSHA256: "p", SubAgents: subs, WorkflowSHA256: "w"}},
		"pl": {Model: "m", Provenance: agentProvenance{PromptSHA256: "p"}},
	}}

	wf := AgentResult{Agent: "wf"}
	pins.applyTo(&wf)
	if wf.WorkflowSHA256 != "w" || wf.SubAgents["s"].PromptSHA256 != "h" {
		t.Errorf("workflow AgentResult = %+v", wf)
	}
	pl := AgentResult{Agent: "pl"}
	pins.applyTo(&pl)
	if pl.WorkflowSHA256 != "" || pl.SubAgents != nil {
		t.Errorf("plain AgentResult carries workflow provenance: %+v", pl)
	}
}

func TestUpdateSummaryCarriesWorkflowProvenance(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	subs := map[string]SubAgentProvenance{"s": {AgentModel: "m", PromptSHA256: "h", ResourcesPresent: []string{}}}
	wf := AgentResult{Agent: "wf", AgentModel: "m", PromptSHA256: "p", ResourcesPresent: []string{}, SubAgents: subs, WorkflowSHA256: "w"}
	pl := AgentResult{Agent: "pl", AgentModel: "m", PromptSHA256: "p", ResourcesPresent: []string{}}
	for _, r := range []AgentResult{wf, pl} {
		if err := updateIncrementalSummary(file, r, "abc"); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var s Summary
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if got := s.Agents["wf"]; got.WorkflowSHA256 != "w" || got.SubAgents["s"].AgentModel != "m" {
		t.Errorf("summary agents.wf = %+v", got)
	}
	if got := s.Agents["pl"]; got.WorkflowSHA256 != "" || got.SubAgents != nil {
		t.Errorf("summary agents.pl = %+v", got)
	}
}

// TestWorkflowStubRunSurfacesSubAgentProvenance runs the checked-in
// selftest-workflow and selftest agents through RunWithOptions (stub backend).
func TestWorkflowStubRunSurfacesSubAgentProvenance(t *testing.T) {
	t.Cleanup(resetConfig)
	evalsDir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, evalsDir)
	t.Setenv("PATH", pathWithoutKiroCLI(t))
	resultsDir := filepath.Join(evalsDir, "results")

	var wfErr error
	stdout := captureStdout(t, func() {
		wfErr = RunWithOptions("selftest-workflow", "", RunOptions{Backend: "stub", EvalsDir: evalsDir})
	})
	if wfErr != nil {
		t.Fatalf("workflow stub run failed: %v\n%s", wfErr, stdout)
	}
	added := newRunDirs(t, resultsDir, map[string]bool{})
	if len(added) != 1 {
		t.Fatalf("run dirs = %v", added)
	}
	runDir := filepath.Join(resultsDir, added[0])

	// The pin line names each sub-agent, its model and a short prompt hash, and
	// carries the workflow hash.
	var pinLine string
	for _, l := range strings.Split(stdout, "\n") {
		if strings.Contains(l, "sub-agents:") {
			pinLine = l
		}
	}
	if pinLine == "" {
		t.Fatalf("no sub-agents pin line in output:\n%s", stdout)
	}
	for _, want := range []string{"selftest-step-a=claude-sonnet-5.5 (prompt sha256 ", "selftest-step-b=", "workflow sha256 "} {
		if !strings.Contains(pinLine, want) {
			t.Errorf("pin line %q lacks %q", pinLine, want)
		}
	}

	res := readSelfTestResult(t, filepath.Join(runDir, "selftest-workflow.json"))
	if res.WorkflowSHA256 == "" || len(res.SubAgents) != 2 {
		t.Fatalf("selftest-workflow.json: workflow_sha256=%q sub_agents=%v", res.WorkflowSHA256, res.SubAgents)
	}
	for _, n := range []string{"selftest-step-a", "selftest-step-b"} {
		sa, ok := res.SubAgents[n]
		if !ok || sa.AgentModel == "" || len(sa.PromptSHA256) != 64 {
			t.Errorf("sub_agents[%s] = %+v", n, sa)
		}
	}
	if len(res.Cases) == 0 {
		t.Fatal("no cases in result")
	}
	for _, c := range res.Cases {
		if c.WorkflowSHA256 != res.WorkflowSHA256 || len(c.SubAgents) != 2 {
			t.Errorf("case %s: workflow_sha256=%q sub_agents=%v, want the baseline's", c.CaseName, c.WorkflowSHA256, c.SubAgents)
		}
	}

	var sum Summary
	raw, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &sum); err != nil {
		t.Fatal(err)
	}
	ap := sum.Agents["selftest-workflow"]
	if ap.WorkflowSHA256 != res.WorkflowSHA256 || len(ap.SubAgents) != 2 ||
		ap.SubAgents["selftest-step-a"].PromptSHA256 != res.SubAgents["selftest-step-a"].PromptSHA256 {
		t.Errorf("summary agents.selftest-workflow = %+v, want %+v / %q", ap, res.SubAgents, res.WorkflowSHA256)
	}
	if _, isStep := sum.Agents["selftest-step-a"]; isStep {
		t.Error("sub-agent must not be a key of summary agents")
	}

	// A plain agent's JSON contains neither key.
	if err := RunWithOptions("selftest", "", RunOptions{Backend: "stub", EvalsDir: evalsDir}); err != nil {
		t.Fatalf("plain stub run failed: %v", err)
	}
	plainDirs := newRunDirs(t, resultsDir, map[string]bool{added[0]: true})
	if len(plainDirs) != 1 {
		t.Fatalf("plain run dirs = %v", plainDirs)
	}
	for _, f := range []string{"selftest.json", "summary.json"} {
		b, err := os.ReadFile(filepath.Join(resultsDir, plainDirs[0], f))
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{`"sub_agents"`, `"workflow_sha256"`} {
			if strings.Contains(string(b), key) {
				t.Errorf("plain %s contains %s", f, key)
			}
		}
	}
}

func TestPinRunPrintsSubAgentsLineOnlyForWorkflowAgents(t *testing.T) {
	wireEnv(t)
	writeCfgFile(t, ".kiro/agents/plain.json", `{"name":"plain","model":"claude-sonnet-5.5","prompt":"inline"}`)
	gitTrack(t, ".kiro/agents/plain.json")

	out := captureStdout(t, func() {
		if err := pinRun("wf", RunOptions{}, false); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "sub-agents:") || !strings.Contains(out, "step-a=claude-haiku-4.5 (prompt sha256 ") ||
		!strings.Contains(out, "workflow sha256 ") {
		t.Errorf("workflow pin output lacks sub-agents line:\n%s", out)
	}
	out = captureStdout(t, func() {
		if err := pinRun("plain", RunOptions{}, false); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "sub-agents:") {
		t.Errorf("plain agent pin output has a sub-agents line:\n%s", out)
	}
}

func TestCheckResumeIntegrityGuardsWorkflowSHA256(t *testing.T) {
	setupPinProject(t, "", "")
	resultsDir := ".kairon/evals/results/run1"
	pins := func(workflow string) *runPins {
		return &runPins{
			Judge: "claude-sonnet-5.5",
			Agents: map[string]agentPin{"architect": {
				Model:      "claude-sonnet-5.5",
				Provenance: agentProvenance{PromptSHA256: "h", WorkflowSHA256: workflow},
			}},
		}
	}
	record := func(workflow string) {
		extra := ""
		if workflow != "" {
			extra = `"workflow_sha256":"` + workflow + `",`
		}
		writeProjectFile(t, filepath.Join(resultsDir, "architect.json"),
			`{"agent":"architect","git_hash":"g","agent_model":"claude-sonnet-5.5","judge_model":"claude-sonnet-5.5","prompt_sha256":"h",`+extra+`"sandbox":false,"cases":[{"case_name":"c1"}]}`)
	}

	// Same workflow hash: resume allowed.
	cfg.pins = pins("w1")
	record("w1")
	if err := checkResumeIntegrity(resultsDir, "architect", false); err != nil {
		t.Fatalf("unchanged workflow should resume: %v", err)
	}

	// A sub-agent edit changes workflow_sha256 only: refused, naming it.
	cfg.pins = pins("w2")
	err := checkResumeIntegrity(resultsDir, "architect", false)
	if err == nil || !strings.Contains(err.Error(), "cannot resume") || !strings.Contains(err.Error(), "workflow_sha256") {
		t.Fatalf("err = %v, want a resume refusal mentioning workflow_sha256", err)
	}

	// A recorded file without workflow_sha256 for a workflow agent: refused.
	record("")
	err = checkResumeIntegrity(resultsDir, "architect", false)
	if err == nil || !strings.Contains(err.Error(), "workflow_sha256") {
		t.Fatalf("err = %v, want refusal for a recorded file lacking workflow_sha256", err)
	}

	// A non-workflow agent is unaffected when the file has none.
	cfg.pins = pins("")
	if err := checkResumeIntegrity(resultsDir, "architect", false); err != nil {
		t.Fatalf("plain agent resume: %v", err)
	}
}
