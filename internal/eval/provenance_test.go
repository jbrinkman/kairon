package eval

import (
	"crypto/sha256"
	"encoding/hex"
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
	provProject(t, `["file://.kiro/res.md"]`)
	writeProvFile(t, ".kiro/res.md", "res v1")
	base := mustProv(t, "a", false).PromptSHA256

	writeProvFile(t, ".kiro/agents/a-prompt.md", "prompt v2")
	if got := mustProv(t, "a", false).PromptSHA256; got == base {
		t.Error("hash unchanged after prompt edit")
	}
	writeProvFile(t, ".kiro/agents/a-prompt.md", "prompt v1")
	if got := mustProv(t, "a", false).PromptSHA256; got != base {
		t.Error("hash not restored after reverting prompt")
	}

	writeProvFile(t, ".kiro/res.md", "res v2")
	if got := mustProv(t, "a", false).PromptSHA256; got == base {
		t.Error("hash unchanged after resource edit")
	}
	writeProvFile(t, ".kiro/res.md", "res v1")

	writeProvFile(t, ".kiro/agents/a.json",
		`{"name":"a","model":"claude-sonnet-4","prompt":"file://./a-prompt.md","resources":["file://.kiro/res.md"]}`)
	if got := mustProv(t, "a", false).PromptSHA256; got == base {
		t.Error("hash unchanged after config edit")
	}
}

func TestProvenanceHashResourceOrderMatters(t *testing.T) {
	provProject(t, `["file://.kiro/one.md","file://.kiro/two.md"]`)
	writeProvFile(t, ".kiro/one.md", "1")
	writeProvFile(t, ".kiro/two.md", "2")
	first := mustProv(t, "a", false).PromptSHA256

	writeProvFile(t, ".kiro/agents/a.json",
		`{"name":"a","model":" claude-sonnet-5.5 ","prompt":"file://./a-prompt.md","resources":["file://.kiro/two.md","file://.kiro/one.md"]}`)
	if mustProv(t, "a", false).PromptSHA256 == first {
		t.Error("swapping resources did not change the hash")
	}
}

func TestProvenanceHashFramingIsUnambiguous(t *testing.T) {
	// Moving a byte between two adjacent resources must change the hash.
	provProject(t, `["file://.kiro/one.md","file://.kiro/two.md"]`)
	writeProvFile(t, ".kiro/one.md", "ab")
	writeProvFile(t, ".kiro/two.md", "c")
	first := mustProv(t, "a", false).PromptSHA256
	writeProvFile(t, ".kiro/one.md", "a")
	writeProvFile(t, ".kiro/two.md", "bc")
	if mustProv(t, "a", false).PromptSHA256 == first {
		t.Error("shifting bytes between parts did not change the hash")
	}
}

func TestProvenanceMissingResourceSkipped(t *testing.T) {
	provProject(t, `["skill://.kiro/skills/x/SKILL.md","file://.kiro/real.md"]`)
	writeProvFile(t, ".kiro/real.md", "r")
	before := mustProv(t, "a", false)
	want := []string{"file://.kiro/real.md"}
	if strings.Join(before.ResourcesPresent, ",") != strings.Join(want, ",") {
		t.Errorf("ResourcesPresent = %v, want %v", before.ResourcesPresent, want)
	}

	// Creating the missing file later changes the hash and lists it. (A
	// *-conventions override never does: see
	// TestProvenanceProjectConventionsDoNotAffectAgentHash.)
	writeProvFile(t, ".kiro/skills/x/SKILL.md", "skill")
	after := mustProv(t, "a", false)
	if after.PromptSHA256 == before.PromptSHA256 {
		t.Error("hash unchanged when a previously missing resource appeared")
	}
	if len(after.ResourcesPresent) != 2 || after.ResourcesPresent[0] != "skill://.kiro/skills/x/SKILL.md" {
		t.Errorf("ResourcesPresent = %v, want both in config order", after.ResourcesPresent)
	}
}

func TestProvenanceNonStringAndOtherSchemeResourcesIgnored(t *testing.T) {
	provProject(t, `[{"type":"knowledgeBase","source":"x"}, 42, "https://example.com/x.md", "file://.kiro/real.md"]`)
	writeProvFile(t, ".kiro/real.md", "r")
	p := mustProv(t, "a", false)
	if len(p.ResourcesPresent) != 1 || p.ResourcesPresent[0] != "file://.kiro/real.md" {
		t.Errorf("ResourcesPresent = %v", p.ResourcesPresent)
	}
}

func TestProvenanceGlobResourcesExpanded(t *testing.T) {
	provProject(t, `["file://.kiro/docs/*.md","file://.kiro/nomatch/*.md"]`)
	writeProvFile(t, ".kiro/docs/a.md", "A")
	writeProvFile(t, ".kiro/docs/b.md", "B")
	first := mustProv(t, "a", false)
	if len(first.ResourcesPresent) != 1 || first.ResourcesPresent[0] != "file://.kiro/docs/*.md" {
		t.Errorf("ResourcesPresent = %v", first.ResourcesPresent)
	}
	writeProvFile(t, ".kiro/docs/b.md", "B2")
	if mustProv(t, "a", false).PromptSHA256 == first.PromptSHA256 {
		t.Error("editing a glob match did not change the hash")
	}
	writeProvFile(t, ".kiro/docs/b.md", "B")
	writeProvFile(t, ".kiro/docs/c.md", "C")
	if mustProv(t, "a", false).PromptSHA256 == first.PromptSHA256 {
		t.Error("adding a glob match did not change the hash")
	}
}

// A workspace root containing a literal backslash (possible on Linux) must not
// hide a plain, non-glob resource. escapeGlobMeta is only for building a glob
// pattern; the direct os.Stat lookup needs the real, unescaped path.
func TestResolveResourceFilesInBackslashRootFindsPlainFile(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("backslash is the path separator on this platform")
	}
	root := filepath.Join(t.TempDir(), `weird\dir`)
	writeProvFile(t, filepath.Join(root, "res.md"), "resource body")

	files, err := resolveResourceFilesIn(root, "file://res.md")
	if err != nil {
		t.Fatalf("resolveResourceFilesIn: %v", err)
	}
	want := filepath.Join(root, "res.md")
	if len(files) != 1 || files[0] != want {
		t.Errorf("files = %v, want [%s]", files, want)
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

// refProvHash recomputes a provenance hash from raw parts using the documented
// framing (kind, decimal length, bytes), independent of resolveProvenance*.
func refProvHash(config, prompt string, resources ...string) string {
	h := sha256.New()
	writeHashPart(h, "config", []byte(config))
	writeHashPart(h, "prompt", []byte(prompt))
	for _, r := range resources {
		writeHashPart(h, "resource", []byte(r))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AC4: a project *-conventions file is never staged, so creating, editing or
// deleting it must not move the agent-level hash or resources_present.
func TestProvenanceProjectConventionsDoNotAffectAgentHash(t *testing.T) {
	provProject(t, `["skill://.kiro/skills/a-conventions/SKILL.md","file://.kiro/skills/engine/SKILL.md"]`)
	writeProvFile(t, ".kiro/skills/engine/SKILL.md", "engine")
	base := mustProv(t, "a", false)
	if want := []string{"file://.kiro/skills/engine/SKILL.md"}; strings.Join(base.ResourcesPresent, ",") != strings.Join(want, ",") {
		t.Fatalf("baseline ResourcesPresent = %v, want %v", base.ResourcesPresent, want)
	}

	conv := ".kiro/skills/a-conventions/SKILL.md"
	steps := []struct {
		name string
		do   func()
	}{
		{"create", func() { writeProvFile(t, conv, "override v1") }},
		{"edit", func() { writeProvFile(t, conv, "override v2") }},
		{"delete", func() {
			if err := os.Remove(conv); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, s := range steps {
		s.do()
		got := mustProv(t, "a", false)
		if got.PromptSHA256 != base.PromptSHA256 {
			t.Errorf("after %s of project conventions file: prompt_sha256 %s != baseline %s", s.name, got.PromptSHA256, base.PromptSHA256)
		}
		if strings.Join(got.ResourcesPresent, ",") != strings.Join(base.ResourcesPresent, ",") {
			t.Errorf("after %s: ResourcesPresent = %v, want %v", s.name, got.ResourcesPresent, base.ResourcesPresent)
		}
	}
}

// resolveProvenanceInRoot resolves config, prompt and resources against the
// given root, so a conventions file staged there is listed and hashed, and the
// process working directory is irrelevant.
func TestResolveProvenanceInRootHashesStagedConventions(t *testing.T) {
	root := t.TempDir()
	config := `{"name":"a","model":"m","prompt":"file://./a-prompt.md","resources":["skill://.kiro/skills/a-conventions/SKILL.md","file://.kiro/skills/engine/SKILL.md","skill://.kiro/skills/gone/SKILL.md"]}`
	writeProvFile(t, filepath.Join(root, ".kiro/agents/a.json"), config)
	writeProvFile(t, filepath.Join(root, ".kiro/agents/a-prompt.md"), "staged prompt")
	writeProvFile(t, filepath.Join(root, ".kiro/skills/a-conventions/SKILL.md"), "staged conventions")
	writeProvFile(t, filepath.Join(root, ".kiro/skills/engine/SKILL.md"), "staged engine")

	// A decoy under the cwd at the same relative paths must be ignored.
	chdirTemp(t)
	writeProvFile(t, ".kiro/agents/a-prompt.md", "decoy prompt")
	writeProvFile(t, ".kiro/skills/a-conventions/SKILL.md", "decoy conventions")
	writeProvFile(t, ".kiro/skills/gone/SKILL.md", "decoy gone")

	p, err := resolveProvenanceInRoot(root, "a", nil)
	if err != nil {
		t.Fatalf("resolveProvenanceInRoot: %v", err)
	}
	wantRes := []string{"skill://.kiro/skills/a-conventions/SKILL.md", "file://.kiro/skills/engine/SKILL.md"}
	if strings.Join(p.ResourcesPresent, ",") != strings.Join(wantRes, ",") {
		t.Errorf("ResourcesPresent = %v, want %v", p.ResourcesPresent, wantRes)
	}
	if want := refProvHash(config, "staged prompt", "staged conventions", "staged engine"); p.PromptSHA256 != want {
		t.Errorf("PromptSHA256 = %s, want %s", p.PromptSHA256, want)
	}
	if p.Model != "m" {
		t.Errorf("Model = %q", p.Model)
	}
}

// Untracked project files are not staged, so they cannot affect the hash; the
// same file once tracked does.
func TestProvenanceUntrackedProjectFilesIgnored(t *testing.T) {
	provProject(t, `["file://.kiro/skills/engine/SKILL.md"]`)
	gitInitCwd(t)
	gitTrack(t, ".kiro/agents")
	writeProvFile(t, ".kiro/skills/engine/SKILL.md", "engine v1")

	base := mustProv(t, "a", false)
	if len(base.ResourcesPresent) != 0 {
		t.Fatalf("untracked resource listed: %v", base.ResourcesPresent)
	}
	writeProvFile(t, ".kiro/skills/engine/SKILL.md", "engine v2")
	if got := mustProv(t, "a", false); got.PromptSHA256 != base.PromptSHA256 {
		t.Error("editing an untracked project file changed the hash")
	}

	gitTrack(t, ".kiro/skills/engine/SKILL.md")
	tracked := mustProv(t, "a", false)
	if len(tracked.ResourcesPresent) != 1 || tracked.PromptSHA256 == base.PromptSHA256 {
		t.Errorf("tracked resource not picked up: %+v", tracked)
	}
}

// For a tree with no conventions or untracked files the hash is the legacy
// framing over config, prompt and each resource in config order.
func TestProvenanceHashMatchesLegacyFraming(t *testing.T) {
	provProject(t, `["file://.kiro/skills/one/SKILL.md","file://.kiro/docs/*.md","skill://.kiro/skills/missing/SKILL.md"]`)
	writeProvFile(t, ".kiro/skills/one/SKILL.md", "one")
	writeProvFile(t, ".kiro/docs/b.md", "B")
	writeProvFile(t, ".kiro/docs/a.md", "A")
	config, err := os.ReadFile(".kiro/agents/a.json")
	if err != nil {
		t.Fatal(err)
	}
	want := refProvHash(string(config), "prompt v1", "one", "A", "B")
	if got := mustProv(t, "a", false).PromptSHA256; got != want {
		t.Errorf("PromptSHA256 = %s, want legacy-framed %s", got, want)
	}
}

// The throwaway baseline directory is removed on success and on error.
func TestProvenanceBaselineTempDirRemoved(t *testing.T) {
	provProject(t, `[]`)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	leftovers := func() []string {
		entries, err := os.ReadDir(tmp)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}

	if _, err := resolveAgentProvenance("a", false); err != nil {
		t.Fatal(err)
	}
	if n := leftovers(); len(n) != 0 {
		t.Errorf("temp entries left after success: %v", n)
	}

	// Candidate validation fails after staging has started (inline prompt).
	writeProvFile(t, ".kiro/agents/inline.json", `{"name":"inline","prompt":"inline"}`)
	cand := &candidatePrompt{Agent: "inline", Path: "c.md", Content: []byte("x")}
	if _, err := resolveAgentProvenanceWith("inline", false, cand); err == nil {
		t.Fatal("expected candidate validation error")
	}
	// Missing prompt file fails inside the root-based resolution.
	if err := os.Remove(".kiro/agents/a-prompt.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveAgentProvenance("a", false); err == nil {
		t.Fatal("expected missing prompt error")
	}
	if n := leftovers(); len(n) != 0 {
		t.Errorf("temp entries left after errors: %v", n)
	}
}
