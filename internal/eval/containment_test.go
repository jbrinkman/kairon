package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/jbrinkman/kairon/internal/inference"
)

// pinnedAgentPins returns pins covering agent "builder" with a trust override.
func pinnedAgentPins(trust []string) *runPins {
	return &runPins{
		Judge: "judge-m",
		Agents: map[string]agentPin{
			"builder": {Model: "agent-m", Provenance: agentProvenance{
				PromptSHA256:     "sha-1",
				ResourcesPresent: []string{"file://a.md"},
			}},
		},
		TrustOverrides: map[string][]string{"builder": trust},
	}
}

func TestApplyRunContextContainerVsNative(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = pinnedAgentPins([]string{"read", "write"})

	want, err := resolveTrustSet("builder")
	if err != nil {
		t.Fatal(err)
	}

	var native, container AgentResult
	native.Agent, container.Agent = "builder", "builder"
	applyRunContext(&native, nil)
	applyRunContext(&container, &ContainerConfig{})

	if native.Sandbox == nil || *native.Sandbox || native.Containment != nil {
		t.Errorf("native: Sandbox=%v Containment=%+v, want false/nil", native.Sandbox, native.Containment)
	}
	if container.Sandbox == nil || !*container.Sandbox {
		t.Fatalf("container Sandbox = %v, want true", container.Sandbox)
	}
	wantC := Containment{ToolTrust: want.Names(), FakeGH: true, ReadOnlyFS: true, Network: "unrestricted"}
	if container.Containment == nil || !reflect.DeepEqual(*container.Containment, wantC) {
		t.Errorf("container Containment = %+v, want %+v", container.Containment, wantC)
	}

	// Provenance is identical across modes.
	if native.AgentModel != "agent-m" || native.JudgeModel != "judge-m" || native.PromptSHA256 != "sha-1" ||
		!reflect.DeepEqual(native.ResourcesPresent, []string{"file://a.md"}) {
		t.Fatalf("native provenance not applied: %+v", native)
	}
	if container.AgentModel != native.AgentModel || container.JudgeModel != native.JudgeModel ||
		container.PromptSHA256 != native.PromptSHA256 ||
		!reflect.DeepEqual(container.ResourcesPresent, native.ResourcesPresent) {
		t.Errorf("provenance differs: native=%+v container=%+v", native, container)
	}
}

func TestApplyRunContextClearsStaleContainment(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = pinnedAgentPins(nil)
	r := AgentResult{Agent: "builder", Containment: &Containment{Network: "unrestricted"}}
	applyRunContext(&r, nil)
	if r.Containment != nil || r.Sandbox == nil || *r.Sandbox {
		t.Errorf("native apply left Containment=%+v Sandbox=%v", r.Containment, r.Sandbox)
	}
}

func TestApplyRunContextUnpinnedStillRecordsMode(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = &runPins{TrustOverrides: map[string][]string{"builder": {}}}
	r := AgentResult{Agent: "builder"}
	applyRunContext(&r, &ContainerConfig{})
	if r.Sandbox == nil || !*r.Sandbox || r.Containment == nil {
		t.Errorf("unpinned container run: Sandbox=%v Containment=%v", r.Sandbox, r.Containment)
	}
	if r.AgentModel != "" || r.PromptSHA256 != "" {
		t.Errorf("unpinned run gained provenance: %+v", r)
	}
}

func TestContainmentToolTrustSerialisesEmptyArray(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = &runPins{TrustOverrides: map[string][]string{"builder": {}}}
	c := containmentFor("builder")
	if c.ToolTrust == nil || len(c.ToolTrust) != 0 {
		t.Fatalf("ToolTrust = %#v, want non-nil empty", c.ToolTrust)
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tool_trust":[]`) {
		t.Errorf("json = %s, want tool_trust:[]", data)
	}
	// A zero-value Containment still emits the tool_trust key (not omitempty).
	data, _ = json.Marshal(Containment{})
	if !strings.Contains(string(data), `"tool_trust"`) {
		t.Errorf("zero Containment dropped tool_trust: %s", data)
	}
}

func TestContainmentForUnresolvableTrustRecordsEmptyAndDoesNotFail(t *testing.T) {
	chdirTemp(t) // no agent config anywhere
	t.Cleanup(resetConfig)
	r := AgentResult{Agent: "ghost"}
	applyRunContext(&r, &ContainerConfig{})
	if r.Containment == nil || r.Containment.ToolTrust == nil || len(r.Containment.ToolTrust) != 0 {
		t.Fatalf("Containment = %+v, want tool_trust []", r.Containment)
	}
	if !r.Containment.FakeGH || !r.Containment.ReadOnlyFS || r.Containment.Network != "unrestricted" {
		t.Errorf("other containment facts wrong: %+v", r.Containment)
	}
}

// The recorded containment facts must equal what is actually enforced.
func TestContainmentMatchesEnforcement(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = &runPins{TrustOverrides: map[string][]string{"builder": {"read", "shell"}}}
	c := containmentFor("builder")

	hc, err := sandbox.NewHostConfigWithMounts(sandbox.ResourceLimits{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReadOnlyFS != hc.ReadonlyRootfs {
		t.Errorf("ReadOnlyFS = %t, HostConfig.ReadonlyRootfs = %t", c.ReadOnlyFS, hc.ReadonlyRootfs)
	}
	if hc.NetworkMode != "none" {
		t.Errorf("NetworkMode = %q, must stay unchanged (\"none\")", hc.NetworkMode)
	}

	ws := testWorkspaceDirs(t)
	ms, err := buildContainerMounts(ws, testContainerConfig(), inference.NameKiroCLI)
	if err != nil {
		t.Fatal(err)
	}
	_, hasBin := mountSummary(ms)[containerBinDir]
	if c.FakeGH != hasBin {
		t.Errorf("FakeGH = %t, bin mount present = %t", c.FakeGH, hasBin)
	}

	trust, _ := resolveTrustSet("builder")
	if !reflect.DeepEqual(c.ToolTrust, trust.Names()) {
		t.Errorf("ToolTrust = %v, want %v", c.ToolTrust, trust.Names())
	}
}

func boolPtr(b bool) *bool { return &b }

func containerResult(agent string, trust []string) AgentResult {
	return AgentResult{
		Agent: agent, Sandbox: boolPtr(true),
		Containment: &Containment{ToolTrust: trust, FakeGH: true, ReadOnlyFS: true, Network: "unrestricted"},
	}
}

func TestUpdateIncrementalSummaryRecordsModeAndContainment(t *testing.T) {
	dir := t.TempDir()

	cfile := filepath.Join(dir, "c", "summary.json")
	if err := os.MkdirAll(filepath.Dir(cfile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := updateIncrementalSummary(cfile, containerResult("builder", []string{"read"}), "g"); err != nil {
		t.Fatal(err)
	}
	var cs Summary
	readJSON(t, cfile, &cs)
	if cs.Sandbox != "container" {
		t.Errorf("Sandbox = %q, want container", cs.Sandbox)
	}
	if got := cs.Containment["builder"]; !reflect.DeepEqual(got, Containment{ToolTrust: []string{"read"}, FakeGH: true, ReadOnlyFS: true, Network: "unrestricted"}) {
		t.Errorf("containment[builder] = %+v", got)
	}

	nfile := filepath.Join(dir, "n", "summary.json")
	if err := os.MkdirAll(filepath.Dir(nfile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := updateIncrementalSummary(nfile, AgentResult{Agent: "builder", Sandbox: boolPtr(false)}, "g"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(nfile)
	if !strings.Contains(string(raw), `"sandbox": "native"`) || strings.Contains(string(raw), `"containment"`) {
		t.Errorf("native summary = %s", raw)
	}
}

func TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	if err := updateIncrementalSummary(file, containerResult("builder", []string{}), "g"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	if !strings.Contains(string(raw), `"tool_trust": []`) {
		t.Errorf("summary = %s, want tool_trust: []", raw)
	}
}

func TestUpdateIncrementalSummaryRefusesModeChange(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	if err := updateIncrementalSummary(file, containerResult("a", nil), "g"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(file)
	err := updateIncrementalSummary(file, AgentResult{Agent: "b", Sandbox: boolPtr(false)}, "g")
	if err == nil || !strings.Contains(err.Error(), "container") || !strings.Contains(err.Error(), "native") {
		t.Fatalf("err = %v, want refusal naming both modes", err)
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Errorf("summary was modified despite refusal:\n%s\n%s", before, after)
	}
}

func TestUpdateIncrementalSummaryNativeRemovesStaleContainment(t *testing.T) {
	// Defence in depth: a hand-edited/stale summary that says native but
	// carries a containment entry loses it on the next native update.
	file := filepath.Join(t.TempDir(), "summary.json")
	if err := os.WriteFile(file, []byte(`{"git_hash":"g","total_cost":{},"agent_scores":{},"sandbox":"native","containment":{"builder":{"tool_trust":[],"fake_gh":true,"read_only_fs":true,"network":"unrestricted"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := updateIncrementalSummary(file, AgentResult{Agent: "builder", Sandbox: boolPtr(false)}, "g"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	if strings.Contains(string(raw), `"containment"`) {
		t.Errorf("stale containment kept: %s", raw)
	}
}

func TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	if err := updateIncrementalSummary(file, AgentResult{Agent: "a"}, "g"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	if strings.Contains(string(raw), `"sandbox"`) || strings.Contains(string(raw), `"containment"`) {
		t.Errorf("result with unknown mode must not invent one: %s", raw)
	}
}

func TestLegacyJSONStillUnmarshals(t *testing.T) {
	var s Summary
	if err := json.Unmarshal([]byte(`{"git_hash":"g","total_cost":{},"agent_scores":{"a":0.5}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Sandbox != "" || s.Containment != nil {
		t.Errorf("legacy summary = %+v", s)
	}
	var a AgentResult
	if err := json.Unmarshal([]byte(`{"agent":"a","git_hash":"g","cases":[]}`), &a); err != nil {
		t.Fatal(err)
	}
	if a.Sandbox != nil || a.Containment != nil {
		t.Errorf("legacy agent = %+v", a)
	}
}

func TestCheckResumeIntegritySummaryModeMismatch(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = nil
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"git_hash":"g","total_cost":{},"agent_scores":{},"sandbox":"container"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// No agent file exists yet: only the summary can reveal the change.
	err := checkResumeIntegrity(dir, "builder", false)
	if err == nil || !strings.Contains(err.Error(), "container") || !strings.Contains(err.Error(), "native") {
		t.Fatalf("err = %v, want refusal naming both modes", err)
	}
	if err := checkResumeIntegrity(dir, "builder", true); err != nil {
		t.Errorf("same mode refused: %v", err)
	}

	// A legacy summary (no sandbox) is not mode-checked.
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"git_hash":"g","total_cost":{},"agent_scores":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []bool{false, true} {
		if err := checkResumeIntegrity(dir, "builder", mode); err != nil {
			t.Errorf("legacy summary (mode=%t) refused: %v", mode, err)
		}
	}
}

func TestEvaluateRecordsSandboxInResultAndSummary(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = &runPins{TrustOverrides: map[string][]string{execRubric().Agent: {}}}
	res := evaluate(execRubric(), nil, "abc", &strings.Builder{}, nil)
	if res.Sandbox == nil || *res.Sandbox || res.Containment != nil {
		t.Errorf("evaluate(native): Sandbox=%v Containment=%v", res.Sandbox, res.Containment)
	}
	file := filepath.Join(t.TempDir(), "summary.json")
	if err := updateIncrementalSummary(file, res, "abc"); err != nil {
		t.Fatal(err)
	}
	var s Summary
	readJSON(t, file, &s)
	if s.Sandbox != "native" {
		t.Errorf("summary sandbox = %q, want native", s.Sandbox)
	}
}

// The same cases scored through the shared totals give the same score and
// denominator whether the result was built natively or for a container run,
// both directly and through the summary.
func TestSharedTotalsEqualAcrossModes(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.pins = pinnedAgentPins([]string{"read"})

	cases := []CaseResult{
		{CaseName: "c1", Scores: []CriterionScore{
			{Name: "a", Score: 4, MaxScore: 4, Deterministic: true},
			{Name: "b", Score: 1, MaxScore: 4},
			{Name: "skip", Score: 0, MaxScore: 4, Skipped: true},
		}},
		{CaseName: "c2", Scores: []CriterionScore{{Name: "a", Score: 2, MaxScore: 4}}},
		{CaseName: "c3-timeout"}, // no scores at all
	}
	build := func(c *ContainerConfig) AgentResult {
		r := AgentResult{Agent: "builder", GitHash: "g", Cases: append([]CaseResult(nil), cases...)}
		applyRunContext(&r, c)
		return r
	}
	native, container := build(nil), build(&ContainerConfig{})

	ns, nm := agentScoreTotals(native)
	cs, cm := agentScoreTotals(container)
	if ns != cs || nm != cm {
		t.Fatalf("totals differ: native %v/%v, container %v/%v", ns, nm, cs, cm)
	}
	if ns != 7 || nm != 12 {
		t.Errorf("totals = %v/%v, want 7/12 (skipped criterion excluded)", ns, nm)
	}
	for i := range cases {
		a, am := caseTotals(native.Cases[i])
		b, bm := caseTotals(container.Cases[i])
		if a != b || am != bm {
			t.Errorf("case %s totals differ: %d/%d vs %d/%d", cases[i].CaseName, a, am, b, bm)
		}
	}

	dir := t.TempDir()
	nf, cf := filepath.Join(dir, "n", "summary.json"), filepath.Join(dir, "c", "summary.json")
	for _, x := range []struct {
		file string
		res  AgentResult
	}{{nf, native}, {cf, container}} {
		if err := os.MkdirAll(filepath.Dir(x.file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := updateIncrementalSummary(x.file, x.res, "g"); err != nil {
			t.Fatal(err)
		}
	}
	var nsum, csum Summary
	readJSON(t, nf, &nsum)
	readJSON(t, cf, &csum)
	if !reflect.DeepEqual(nsum.AgentScores, csum.AgentScores) {
		t.Errorf("agent_scores differ: native %v, container %v", nsum.AgentScores, csum.AgentScores)
	}
	if got, want := nsum.AgentScores["builder"], 7.0/12.0; got != want {
		t.Errorf("agent score = %v, want %v", got, want)
	}

	// Whole-Summary equality once the fields that are meant to differ are cleared.
	nsum.Sandbox, nsum.Containment = "", nil
	csum.Sandbox, csum.Containment = "", nil
	if !reflect.DeepEqual(nsum, csum) {
		t.Errorf("summaries differ beyond sandbox/containment:\nnative:    %+v\ncontainer: %+v", nsum, csum)
	}
}

// Fixture runs load with the current types and score through the shared totals.
func TestRunFixturesLoadAndScore(t *testing.T) {
	for _, fixture := range []string{"legacy-native", "native-pinned"} {
		t.Run(fixture, func(t *testing.T) {
			res, err := loadAgentResult(filepath.Join(runFixturesDir, fixture, "architect.json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, m := agentScoreTotals(res); m == 0 {
				t.Errorf("fixture %s has no scored criteria", fixture)
			}
		})
	}
}
