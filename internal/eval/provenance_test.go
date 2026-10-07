package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/config"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func writeProvFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// provProject creates a temp project with .kiro/agents/a.json using an
// external prompt file and the given JSON resources array body.
func provProject(t *testing.T, resources string) string {
	t.Helper()
	dir := chdirTemp(t)
	writeProvFile(t, ".kiro/agents/a-prompt.md", "prompt v1")
	writeProvFile(t, ".kiro/agents/a.json",
		`{"name":"a","model":" claude-sonnet-5.5 ","prompt":"file://./a-prompt.md","resources":`+resources+`}`)
	return dir
}

func mustProv(t *testing.T, agent string, container bool) agentProvenance {
	t.Helper()
	p, err := resolveAgentProvenance(agent, container)
	if err != nil {
		t.Fatalf("resolveAgentProvenance: %v", err)
	}
	return p
}

func TestProvenanceHashShapeAndStability(t *testing.T) {
	provProject(t, `[]`)
	p1 := mustProv(t, "a", false)
	p2 := mustProv(t, "a", false)
	if !hex64.MatchString(p1.PromptSHA256) {
		t.Errorf("hash %q is not 64 lowercase hex", p1.PromptSHA256)
	}
	if p1.PromptSHA256 != p2.PromptSHA256 {
		t.Error("hash not stable across runs")
	}
	if p1.Model != "claude-sonnet-5.5" {
		t.Errorf("Model = %q, want trimmed config model", p1.Model)
	}
	if p1.ResourcesPresent == nil || len(p1.ResourcesPresent) != 0 {
		t.Errorf("ResourcesPresent = %#v, want non-nil empty", p1.ResourcesPresent)
	}
	b, _ := json.Marshal(AgentResult{ResourcesPresent: p1.ResourcesPresent})
	if !strings.Contains(string(b), `"resources_present":[]`) {
		t.Errorf("resources_present should serialise as []: %s", b)
	}
}

func TestProvenanceHashChangesWithPromptResourceAndConfig(t *testing.T) {
	provProject(t, `["file://res.md"]`)
	writeProvFile(t, "res.md", "res v1")
	base := mustProv(t, "a", false).PromptSHA256

	writeProvFile(t, ".kiro/agents/a-prompt.md", "prompt v2")
	if got := mustProv(t, "a", false).PromptSHA256; got == base {
		t.Error("hash unchanged after prompt edit")
	}
	writeProvFile(t, ".kiro/agents/a-prompt.md", "prompt v1")
	if got := mustProv(t, "a", false).PromptSHA256; got != base {
		t.Error("hash not restored after reverting prompt")
	}

	writeProvFile(t, "res.md", "res v2")
	if got := mustProv(t, "a", false).PromptSHA256; got == base {
		t.Error("hash unchanged after resource edit")
	}
	writeProvFile(t, "res.md", "res v1")

	writeProvFile(t, ".kiro/agents/a.json",
		`{"name":"a","model":"claude-sonnet-4","prompt":"file://./a-prompt.md","resources":["file://res.md"]}`)
	if got := mustProv(t, "a", false).PromptSHA256; got == base {
		t.Error("hash unchanged after config edit")
	}
}

func TestProvenanceHashResourceOrderMatters(t *testing.T) {
	provProject(t, `["file://one.md","file://two.md"]`)
	writeProvFile(t, "one.md", "1")
	writeProvFile(t, "two.md", "2")
	first := mustProv(t, "a", false).PromptSHA256

	writeProvFile(t, ".kiro/agents/a.json",
		`{"name":"a","model":" claude-sonnet-5.5 ","prompt":"file://./a-prompt.md","resources":["file://two.md","file://one.md"]}`)
	if mustProv(t, "a", false).PromptSHA256 == first {
		t.Error("swapping resources did not change the hash")
	}
}

func TestProvenanceHashFramingIsUnambiguous(t *testing.T) {
	// Moving a byte between two adjacent resources must change the hash.
	provProject(t, `["file://one.md","file://two.md"]`)
	writeProvFile(t, "one.md", "ab")
	writeProvFile(t, "two.md", "c")
	first := mustProv(t, "a", false).PromptSHA256
	writeProvFile(t, "one.md", "a")
	writeProvFile(t, "two.md", "bc")
	if mustProv(t, "a", false).PromptSHA256 == first {
		t.Error("shifting bytes between parts did not change the hash")
	}
}

func TestProvenanceMissingResourceSkipped(t *testing.T) {
	provProject(t, `["skill://.kiro/skills/x-conventions/SKILL.md","file://real.md"]`)
	writeProvFile(t, "real.md", "r")
	before := mustProv(t, "a", false)
	want := []string{"file://real.md"}
	if strings.Join(before.ResourcesPresent, ",") != strings.Join(want, ",") {
		t.Errorf("ResourcesPresent = %v, want %v", before.ResourcesPresent, want)
	}

	// Creating the missing file later changes the hash and lists it.
	writeProvFile(t, ".kiro/skills/x-conventions/SKILL.md", "conv")
	after := mustProv(t, "a", false)
	if after.PromptSHA256 == before.PromptSHA256 {
		t.Error("hash unchanged when a previously missing resource appeared")
	}
	if len(after.ResourcesPresent) != 2 || after.ResourcesPresent[0] != "skill://.kiro/skills/x-conventions/SKILL.md" {
		t.Errorf("ResourcesPresent = %v, want both in config order", after.ResourcesPresent)
	}
}

func TestProvenanceNonStringAndOtherSchemeResourcesIgnored(t *testing.T) {
	provProject(t, `[{"type":"knowledgeBase","source":"x"}, 42, "https://example.com/x.md", "file://real.md"]`)
	writeProvFile(t, "real.md", "r")
	p := mustProv(t, "a", false)
	if len(p.ResourcesPresent) != 1 || p.ResourcesPresent[0] != "file://real.md" {
		t.Errorf("ResourcesPresent = %v", p.ResourcesPresent)
	}
}

func TestProvenanceGlobResourcesExpanded(t *testing.T) {
	provProject(t, `["file://docs/*.md","file://nomatch/*.md"]`)
	writeProvFile(t, "docs/a.md", "A")
	writeProvFile(t, "docs/b.md", "B")
	first := mustProv(t, "a", false)
	if len(first.ResourcesPresent) != 1 || first.ResourcesPresent[0] != "file://docs/*.md" {
		t.Errorf("ResourcesPresent = %v", first.ResourcesPresent)
	}
	writeProvFile(t, "docs/b.md", "B2")
	if mustProv(t, "a", false).PromptSHA256 == first.PromptSHA256 {
		t.Error("editing a glob match did not change the hash")
	}
	writeProvFile(t, "docs/b.md", "B")
	writeProvFile(t, "docs/c.md", "C")
	if mustProv(t, "a", false).PromptSHA256 == first.PromptSHA256 {
		t.Error("adding a glob match did not change the hash")
	}
}

func TestProvenanceMissingPromptFileErrors(t *testing.T) {
	provProject(t, `[]`)
	if err := os.Remove(".kiro/agents/a-prompt.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveAgentProvenance("a", false); err == nil || !strings.Contains(err.Error(), "a-prompt.md") {
		t.Errorf("err = %v, want error naming the prompt file", err)
	}
}

func TestProvenanceInlinePromptNeedsNoFile(t *testing.T) {
	chdirTemp(t)
	writeProvFile(t, ".kiro/agents/i.json", `{"name":"i","prompt":"be helpful"}`)
	p := mustProv(t, "i", false)
	if !hex64.MatchString(p.PromptSHA256) || p.Model != "" {
		t.Errorf("unexpected provenance %+v", p)
	}
}

func TestProvenanceConfigLookupPrecedence(t *testing.T) {
	provProject(t, `[]`)
	// Evals-dir overlay with a different model and its own prompt.
	writeProvFile(t, "ev/agents/a.json", `{"name":"a","model":"claude-haiku-4.5","prompt":"overlay prompt"}`)
	cfg.evalsDir = "ev"

	overlay := mustProv(t, "a", false)
	if overlay.Model != "claude-haiku-4.5" || overlay.ConfigPath != filepath.Join("ev", "agents", "a.json") {
		t.Errorf("overlay not preferred: %+v", overlay)
	}
	container := mustProv(t, "a", true)
	if container.Model != "claude-sonnet-5.5" || container.ConfigPath != filepath.Join(".kiro", "agents", "a.json") {
		t.Errorf("container should ignore overlay: %+v", container)
	}
	if container.PromptSHA256 == overlay.PromptSHA256 {
		t.Error("overlay and repo configs produced the same hash")
	}
}

func TestProvenanceFallsBackToKiroAgents(t *testing.T) {
	provProject(t, `[]`)
	cfg.evalsDir = "ev" // no overlay present
	if p := mustProv(t, "a", false); p.ConfigPath != filepath.Join(".kiro", "agents", "a.json") {
		t.Errorf("ConfigPath = %q", p.ConfigPath)
	}
}

func TestProvenanceMissingConfigNamesBothPaths(t *testing.T) {
	chdirTemp(t)
	cfg.evalsDir = "ev"
	for _, container := range []bool{false, true} {
		_, err := resolveAgentProvenance("ghost", container)
		if err == nil {
			t.Fatalf("container=%v: expected error", container)
		}
		for _, want := range []string{filepath.Join("ev", "agents", "ghost.json"), filepath.Join(".kiro", "agents", "ghost.json")} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("container=%v: error %q does not name %s", container, err, want)
			}
		}
	}
}

func TestProvenanceMalformedConfigErrors(t *testing.T) {
	chdirTemp(t)
	writeProvFile(t, ".kiro/agents/bad.json", `{not json`)
	if _, err := resolveAgentProvenance("bad", false); err == nil {
		t.Error("expected parse error")
	}
}

func TestProvenanceTypesSerialisation(t *testing.T) {
	s := Summary{
		JudgeModel: "j",
		Agents:     map[string]AgentProvenance{"a": {AgentModel: "m", PromptSHA256: "h", ResourcesPresent: []string{}}},
	}
	b, _ := json.Marshal(s)
	for _, want := range []string{`"judge_model":"j"`, `"agents":{"a":{"agent_model":"m","prompt_sha256":"h","resources_present":[]}}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("summary JSON %s missing %s", b, want)
		}
	}
	if strings.Contains(string(b), `"agent_model":"m","prompt_sha256":"h","resources_present":[]},`) {
		t.Errorf("unexpected top-level fields: %s", b)
	}
	var cr CaseResult
	cr.Calls = nil
	if b, _ := json.Marshal(cr); strings.Contains(string(b), "calls") {
		t.Errorf("empty Calls should be omitted: %s", b)
	}
}

func TestProvenanceSelfTestFixture(t *testing.T) {
	// Run from the package dir: the fixture lives in testdata.
	resetConfig()
	cfg.evalsDir = filepath.Join("testdata", "evals")
	t.Cleanup(resetConfig)

	data, err := os.ReadFile(filepath.Join("testdata", "evals", "agents", "selftest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var conf struct {
		Model     string   `json:"model"`
		Resources []string `json:"resources"`
	}
	if err := json.Unmarshal(data, &conf); err != nil {
		t.Fatal(err)
	}
	if err := config.DefaultEvalsConfig().CheckModel(conf.Model); err != nil {
		t.Errorf("fixture model not in default allowlist: %v", err)
	}
	if conf.Model != "claude-sonnet-5.5" {
		t.Errorf("fixture model = %q", conf.Model)
	}
	var found bool
	for _, r := range conf.Resources {
		if strings.Contains(r, "-conventions") {
			found = true
			files, err := resolveResourceFiles(r)
			if err != nil || len(files) != 0 {
				t.Errorf("resource %q should not exist (files=%v err=%v)", r, files, err)
			}
		}
	}
	if !found {
		t.Error("fixture has no *-conventions resource")
	}

	p := mustProv(t, "selftest", false)
	if !hex64.MatchString(p.PromptSHA256) || p.Model != "claude-sonnet-5.5" {
		t.Errorf("unexpected provenance %+v", p)
	}
	if p.ResourcesPresent == nil || len(p.ResourcesPresent) != 0 {
		t.Errorf("ResourcesPresent = %#v, want []", p.ResourcesPresent)
	}
}
