package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// writeWFAgent writes <root>/.kiro/agents/<name>.json (+ <name>-prompt.md).
// trusted is the raw JSON for toolsSettings.subagent.trustedAgents ("" omits
// toolsSettings entirely). resources is the raw JSON array body.
func writeWFAgent(t *testing.T, root, name, model, prompt, trusted, resources string) {
	t.Helper()
	cfgJSON := `{"name":"` + name + `","model":"` + model + `","prompt":"file://./` + name + `-prompt.md","resources":` + resources
	if trusted != "" {
		cfgJSON += `,"toolsSettings":{"subagent":{"trustedAgents":` + trusted + `}}`
	}
	cfgJSON += `}`
	writeProvFile(t, filepath.Join(root, ".kiro/agents", name+".json"), cfgJSON)
	writeProvFile(t, filepath.Join(root, ".kiro/agents", name+"-prompt.md"), prompt)
}

// wfRoot stages a dispatcher "lead" trusting step-b then step-a (config order
// deliberately not sorted). step-a declares one absent and one present resource.
func wfRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeWFAgent(t, root, "lead", "model-lead", "lead prompt", `["step-b","step-a"]`, `[]`)
	writeWFAgent(t, root, "step-a", " model-a ", "step a prompt", "", `["skill://.kiro/skills/a/SKILL.md","file://.kiro/present.md"]`)
	writeWFAgent(t, root, "step-b", "model-b", "step b prompt", "", `[]`)
	writeProvFile(t, filepath.Join(root, ".kiro/present.md"), "present")
	return root
}

func mustProvInRoot(t *testing.T, root, agent string) agentProvenance {
	t.Helper()
	p, err := resolveProvenanceInRoot(root, agent, nil)
	if err != nil {
		t.Fatalf("resolveProvenanceInRoot(%s): %v", agent, err)
	}
	return p
}

// refWorkflowHash recomputes workflow_sha256 from the documented framing:
// the dispatcher first, then sub-agents sorted by name, each as a
// "workflow-agent" part (name) and a "prompt-sha256" part (hex hash).
func refWorkflowHash(dispatcher, dispatcherSHA string, subs map[string]string) string {
	h := sha256.New()
	writeHashPart(h, "workflow-agent", []byte(dispatcher))
	writeHashPart(h, "prompt-sha256", []byte(dispatcherSHA))
	names := make([]string, 0, len(subs))
	for n := range subs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		writeHashPart(h, "workflow-agent", []byte(n))
		writeHashPart(h, "prompt-sha256", []byte(subs[n]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestProvenanceSubAgentsResolved(t *testing.T) {
	root := wfRoot(t)
	p := mustProvInRoot(t, root, "lead")

	if len(p.SubAgents) != 2 {
		t.Fatalf("SubAgents = %v, want step-a and step-b", p.SubAgents)
	}
	a, ok := p.SubAgents["step-a"]
	if !ok {
		t.Fatal("step-a missing")
	}
	if a.AgentModel != "model-a" {
		t.Errorf("step-a AgentModel = %q, want trimmed model-a", a.AgentModel)
	}
	if !hex64.MatchString(a.PromptSHA256) {
		t.Errorf("step-a PromptSHA256 %q is not 64 hex", a.PromptSHA256)
	}
	if want := []string{"file://.kiro/present.md"}; strings.Join(a.ResourcesPresent, ",") != strings.Join(want, ",") {
		t.Errorf("step-a ResourcesPresent = %v, want %v", a.ResourcesPresent, want)
	}
	b := p.SubAgents["step-b"]
	if b.AgentModel != "model-b" {
		t.Errorf("step-b AgentModel = %q", b.AgentModel)
	}
	if b.ResourcesPresent == nil || len(b.ResourcesPresent) != 0 {
		t.Errorf("step-b ResourcesPresent = %#v, want non-nil empty", b.ResourcesPresent)
	}
	if _, dup := p.SubAgents["lead"]; dup {
		t.Error("dispatcher must not be listed as its own sub-agent")
	}

	// A sub-agent's hash is the one it would have as a dispatching agent.
	if want := mustProvInRoot(t, root, "step-a").PromptSHA256; a.PromptSHA256 != want {
		t.Errorf("step-a PromptSHA256 = %s, want %s", a.PromptSHA256, want)
	}

	if !hex64.MatchString(p.WorkflowSHA256) {
		t.Fatalf("WorkflowSHA256 %q is not 64 hex", p.WorkflowSHA256)
	}
	want := refWorkflowHash("lead", p.PromptSHA256, map[string]string{
		"step-a": a.PromptSHA256, "step-b": b.PromptSHA256,
	})
	if p.WorkflowSHA256 != want {
		t.Errorf("WorkflowSHA256 = %s, want %s", p.WorkflowSHA256, want)
	}
	if again := mustProvInRoot(t, root, "lead"); again.WorkflowSHA256 != p.WorkflowSHA256 {
		t.Error("WorkflowSHA256 not stable across runs")
	}

	// The SubAgents JSON keeps resources_present as [] and the documented keys.
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"agent_model":"model-b"`, `"prompt_sha256":"`, `"resources_present":[]`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("SubAgentProvenance JSON %s missing %s", raw, key)
		}
	}
}

func TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash(t *testing.T) {
	root := wfRoot(t)
	base := mustProvInRoot(t, root, "lead")

	writeProvFile(t, filepath.Join(root, ".kiro/agents/step-a-prompt.md"), "step a prompt EDITED")
	edited := mustProvInRoot(t, root, "lead")

	if edited.PromptSHA256 != base.PromptSHA256 {
		t.Error("dispatching agent's PromptSHA256 changed after a sub-agent edit")
	}
	if edited.WorkflowSHA256 == base.WorkflowSHA256 {
		t.Error("WorkflowSHA256 unchanged after a sub-agent prompt edit")
	}
	if edited.SubAgents["step-a"].PromptSHA256 == base.SubAgents["step-a"].PromptSHA256 {
		t.Error("step-a PromptSHA256 unchanged after its prompt edit")
	}
	if edited.SubAgents["step-b"].PromptSHA256 != base.SubAgents["step-b"].PromptSHA256 {
		t.Error("step-b PromptSHA256 changed by an unrelated edit")
	}

	// A sub-agent resource (staged workspace bytes) is part of its hash.
	writeProvFile(t, filepath.Join(root, ".kiro/present.md"), "present EDITED")
	if again := mustProvInRoot(t, root, "lead"); again.WorkflowSHA256 == edited.WorkflowSHA256 {
		t.Error("WorkflowSHA256 unchanged after a sub-agent resource edit")
	}
}

func TestProvenanceDispatcherEditChangesBothHashes(t *testing.T) {
	root := wfRoot(t)
	base := mustProvInRoot(t, root, "lead")
	writeProvFile(t, filepath.Join(root, ".kiro/agents/lead-prompt.md"), "lead prompt EDITED")
	edited := mustProvInRoot(t, root, "lead")
	if edited.PromptSHA256 == base.PromptSHA256 || edited.WorkflowSHA256 == base.WorkflowSHA256 {
		t.Error("dispatcher edit must change prompt_sha256 and workflow_sha256")
	}
}

func TestWorkflowHashSwappingStepPromptsChangesHash(t *testing.T) {
	root := wfRoot(t)
	base := mustProvInRoot(t, root, "lead")
	// Swap the two steps' prompts: same set of hashes, different names.
	writeProvFile(t, filepath.Join(root, ".kiro/agents/step-a-prompt.md"), "step b prompt")
	writeProvFile(t, filepath.Join(root, ".kiro/agents/step-b-prompt.md"), "step a prompt")
	swapped := mustProvInRoot(t, root, "lead")
	if swapped.WorkflowSHA256 == base.WorkflowSHA256 {
		t.Error("swapping the sub-agents' prompts did not change workflow_sha256")
	}
}

func TestWorkflowHashIndependentOfTrustedAgentsOrder(t *testing.T) {
	root := wfRoot(t)
	base := mustProvInRoot(t, root, "lead")
	// The config bytes change (so the dispatcher hash does too) but sub-agents
	// are sorted by name, so the sub-agent part of the workflow is stable.
	writeWFAgent(t, root, "lead", "model-lead", "lead prompt", `["step-a","step-b"]`, `[]`)
	reordered := mustProvInRoot(t, root, "lead")
	want := refWorkflowHash("lead", reordered.PromptSHA256, map[string]string{
		"step-a": base.SubAgents["step-a"].PromptSHA256, "step-b": base.SubAgents["step-b"].PromptSHA256,
	})
	if reordered.WorkflowSHA256 != want {
		t.Errorf("WorkflowSHA256 = %s, want %s (sub-agents sorted by name)", reordered.WorkflowSHA256, want)
	}
}

func TestProvenanceWithoutTrustedAgentsHasNoSubAgents(t *testing.T) {
	for name, trusted := range map[string]string{"absent": "", "empty list": `[]`} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeWFAgent(t, root, "solo", "m", "solo prompt", trusted, `[]`)
			p := mustProvInRoot(t, root, "solo")
			if p.SubAgents != nil {
				t.Errorf("SubAgents = %v, want nil", p.SubAgents)
			}
			if p.WorkflowSHA256 != "" {
				t.Errorf("WorkflowSHA256 = %q, want empty", p.WorkflowSHA256)
			}
			cfgBytes, err := os.ReadFile(filepath.Join(root, ".kiro/agents/solo.json"))
			if err != nil {
				t.Fatal(err)
			}
			if want := refProvHash(string(cfgBytes), "solo prompt"); p.PromptSHA256 != want {
				t.Errorf("PromptSHA256 = %s, want legacy framing %s", p.PromptSHA256, want)
			}
		})
	}
}

func TestProvenanceDispatcherHashIsItsOwnFilesOnly(t *testing.T) {
	root := wfRoot(t)
	p := mustProvInRoot(t, root, "lead")
	cfgBytes, err := os.ReadFile(filepath.Join(root, ".kiro/agents/lead.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := refProvHash(string(cfgBytes), "lead prompt"); p.PromptSHA256 != want {
		t.Errorf("dispatcher PromptSHA256 = %s, want hash over its own config+prompt %s", p.PromptSHA256, want)
	}
}

func TestProvenanceSubAgentsTransitiveAndCycleSafe(t *testing.T) {
	root := t.TempDir()
	// lead -> mid -> leaf, leaf -> mid (cycle), mid -> lead (back to the dispatcher), leaf -> leaf.
	writeWFAgent(t, root, "lead", "m", "lead", `["mid"]`, `[]`)
	writeWFAgent(t, root, "mid", "m", "mid", `["leaf","lead"]`, `[]`)
	writeWFAgent(t, root, "leaf", "m", "leaf", `["mid","leaf"]`, `[]`)

	p := mustProvInRoot(t, root, "lead")
	if len(p.SubAgents) != 2 || p.SubAgents["mid"].PromptSHA256 == "" || p.SubAgents["leaf"].PromptSHA256 == "" {
		t.Fatalf("SubAgents = %v, want mid and leaf (transitive, each once)", p.SubAgents)
	}
	if _, ok := p.SubAgents["lead"]; ok {
		t.Error("dispatcher listed as its own sub-agent")
	}

	// An agent trusting only itself has no sub-agents at all.
	writeWFAgent(t, root, "self", "m", "self", `["self"]`, `[]`)
	s := mustProvInRoot(t, root, "self")
	if s.SubAgents != nil || s.WorkflowSHA256 != "" {
		t.Errorf("self-trusting agent: SubAgents=%v WorkflowSHA256=%q, want none", s.SubAgents, s.WorkflowSHA256)
	}
}

func TestProvenanceSubAgentErrors(t *testing.T) {
	t.Run("missing sub-agent config names both agents", func(t *testing.T) {
		root := t.TempDir()
		writeWFAgent(t, root, "lead", "m", "lead", `["ghost"]`, `[]`)
		_, err := resolveProvenanceInRoot(root, "lead", nil)
		if err == nil {
			t.Fatal("expected an error for a trusted agent without a config")
		}
		if !strings.Contains(err.Error(), "ghost") || !strings.Contains(err.Error(), "lead") {
			t.Errorf("error %q must name sub-agent ghost and dispatcher lead", err)
		}
	})

	for _, bad := range []string{"../evil", "a/b", "..", ".hidden", "", "x y"} {
		t.Run("unsafe name "+bad, func(t *testing.T) {
			root := t.TempDir()
			// A decoy outside .kiro/agents must never be read.
			writeProvFile(t, filepath.Join(root, ".kiro/evil.json"), `{"model":"x"}`)
			writeWFAgent(t, root, "lead", "m", "lead", `[`+mustJSONString(t, bad)+`]`, `[]`)
			_, err := resolveProvenanceInRoot(root, "lead", nil)
			if err == nil {
				t.Fatalf("expected an error for trusted agent name %q", bad)
			}
			if !strings.Contains(err.Error(), "trustedAgents") && !strings.Contains(err.Error(), "unsafe") {
				t.Errorf("error %q should say the sub-agent name is unsafe", err)
			}
		})
	}

	for _, bad := range []string{`"step-a"`, `{"a":1}`, `[1,2]`, `5`} {
		t.Run("non-list trustedAgents "+bad, func(t *testing.T) {
			root := t.TempDir()
			writeWFAgent(t, root, "lead", "m", "lead", bad, `[]`)
			_, err := resolveProvenanceInRoot(root, "lead", nil)
			if err == nil {
				t.Fatalf("expected an error for trustedAgents %s", bad)
			}
			if !strings.Contains(err.Error(), "lead.json") {
				t.Errorf("error %q should name the config", err)
			}
		})
	}
}

func mustJSONString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProvenanceCandidateAffectsOnlyDispatchingAgent(t *testing.T) {
	root := wfRoot(t)
	base := mustProvInRoot(t, root, "lead")

	// The caller has already staged the candidate bytes over the lead prompt.
	writeProvFile(t, filepath.Join(root, ".kiro/agents/lead-prompt.md"), "candidate prompt")
	cand := &candidatePrompt{Agent: "lead", Path: "cand.md", Content: []byte("candidate prompt")}
	got, err := resolveProvenanceInRoot(root, "lead", cand)
	if err != nil {
		t.Fatal(err)
	}
	if got.PromptFile != "cand.md" {
		t.Errorf("PromptFile = %q, want cand.md", got.PromptFile)
	}
	for name, sub := range base.SubAgents {
		if got.SubAgents[name].PromptSHA256 != sub.PromptSHA256 {
			t.Errorf("candidate changed sub-agent %s", name)
		}
	}

	// A candidate aimed at a sub-agent name does not apply to the dispatcher.
	other := &candidatePrompt{Agent: "step-a", Path: "cand.md", Content: []byte("x")}
	p2, err := resolveProvenanceInRoot(root, "lead", other)
	if err != nil {
		t.Fatal(err)
	}
	if p2.PromptFile != "" {
		t.Errorf("PromptFile = %q for a candidate of another agent, want empty", p2.PromptFile)
	}
	for name, sub := range base.SubAgents {
		if p2.SubAgents[name].PromptSHA256 != sub.PromptSHA256 {
			t.Errorf("candidate for another agent changed sub-agent %s", name)
		}
	}
}

func TestAgentConfigFileTrustedAgents(t *testing.T) {
	var ok agentConfigFile
	if err := json.Unmarshal([]byte(`{"toolsSettings":{"subagent":{"trustedAgents":["a","b"]}}}`), &ok); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ok.ToolsSettings.Subagent.TrustedAgents, ",") != "a,b" {
		t.Errorf("TrustedAgents = %v", ok.ToolsSettings.Subagent.TrustedAgents)
	}
	var bad agentConfigFile
	if err := json.Unmarshal([]byte(`{"toolsSettings":{"subagent":{"trustedAgents":"a"}}}`), &bad); err == nil {
		t.Error("a non-list trustedAgents must be a decode error")
	}
}

// The staged-workspace rule: sub-agents come from the evals-dir overlay /
// project .kiro that stageKiroInto produces, not from the working directory.
func TestProvenanceSubAgentsFromStagedOverlay(t *testing.T) {
	provProject(t, `[]`) // chdir into a temp project; a.json is unrelated
	evals := filepath.Join(t.TempDir(), "evals")
	cfg.evalsDir = evals
	t.Cleanup(resetConfig)

	// Overlay layout is <evals>/agents/<name>.json with a sibling prompt.
	writeProvFile(t, filepath.Join(evals, "agents/wf.json"),
		`{"name":"wf","model":"m","prompt":"file://./wf-prompt.md","toolsSettings":{"subagent":{"trustedAgents":["wf-step"]}}}`)
	writeProvFile(t, filepath.Join(evals, "agents/wf-prompt.md"), "wf")
	writeProvFile(t, filepath.Join(evals, "agents/wf-step.json"),
		`{"name":"wf-step","model":"step-model","prompt":"file://./wf-step-prompt.md"}`)
	writeProvFile(t, filepath.Join(evals, "agents/wf-step-prompt.md"), "step v1")

	p1 := mustProv(t, "wf", false)
	if p1.SubAgents["wf-step"].AgentModel != "step-model" {
		t.Fatalf("SubAgents = %v, want overlay sub-agent wf-step", p1.SubAgents)
	}
	writeProvFile(t, filepath.Join(evals, "agents/wf-step-prompt.md"), "step v2")
	p2 := mustProv(t, "wf", false)
	if p2.WorkflowSHA256 == p1.WorkflowSHA256 || p2.PromptSHA256 != p1.PromptSHA256 {
		t.Error("overlay sub-agent edit must change workflow_sha256 only")
	}
}

// The shipped selftest-workflow fixture trusts selftest-step-a and
// selftest-step-b; selftest-step-a's only resource is absent, and an agent
// without trusted agents (selftest) has no workflow provenance.
func TestProvenanceSelftestWorkflowFixtureAgents(t *testing.T) {
	abs, err := filepath.Abs("testdata/evals")
	if err != nil {
		t.Fatal(err)
	}
	provProject(t, `[]`) // chdir into a temp project (also resets cfg)
	cfg.evalsDir = abs
	t.Cleanup(resetConfig)

	p := mustProv(t, "selftest-workflow", false)
	if len(p.SubAgents) != 2 {
		t.Fatalf("SubAgents = %v, want selftest-step-a and selftest-step-b", p.SubAgents)
	}
	a, b := p.SubAgents["selftest-step-a"], p.SubAgents["selftest-step-b"]
	if a.PromptSHA256 == "" || b.PromptSHA256 == "" || a.PromptSHA256 == b.PromptSHA256 {
		t.Errorf("sub-agent prompts must hash distinctly: a=%q b=%q", a.PromptSHA256, b.PromptSHA256)
	}
	if a.ResourcesPresent == nil || len(a.ResourcesPresent) != 0 {
		t.Errorf("selftest-step-a ResourcesPresent = %#v, want non-nil empty (resource is absent)", a.ResourcesPresent)
	}
	if !hex64.MatchString(p.WorkflowSHA256) {
		t.Errorf("WorkflowSHA256 = %q", p.WorkflowSHA256)
	}
	for _, sub := range []string{"selftest-step-a", "selftest-step-b"} {
		if _, err := os.Stat(filepath.Join(abs, "rubrics", sub+".yaml")); err == nil {
			t.Errorf("sub-agent %s must not have a rubric", sub)
		}
	}

	plain := mustProv(t, "selftest", false)
	if plain.SubAgents != nil || plain.WorkflowSHA256 != "" {
		t.Errorf("selftest: SubAgents=%v WorkflowSHA256=%q, want none", plain.SubAgents, plain.WorkflowSHA256)
	}
}
