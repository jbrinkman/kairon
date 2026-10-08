package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
	"gopkg.in/yaml.v3"
)

// chdirTemp moves into a fresh temp dir for the test and restores cwd and
// the default run configuration afterwards.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	resetConfig()
	t.Cleanup(func() {
		_ = os.Chdir(orig)
		resetConfig()
	})
	return dir
}

func writeCfgFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultConfig(t *testing.T) {
	resetConfig()
	t.Cleanup(resetConfig)

	if cfg.evalsDir != ".kairon/evals" {
		t.Errorf("evalsDir = %q, want .kairon/evals", cfg.evalsDir)
	}
	if cfg.backend == nil || cfg.backend.Name() != "kiro-cli" {
		t.Errorf("backend = %v, want kiro-cli", cfg.backend)
	}

	want := map[string]string{
		"rubrics": filepath.Join(".kairon", "evals", "rubrics"),
		"cases":   filepath.Join(".kairon", "evals", "cases"),
		"results": filepath.Join(".kairon", "evals", "results"),
	}
	for part, w := range want {
		if got := evalsPath(part); got != w {
			t.Errorf("evalsPath(%q) = %q, want %q", part, got, w)
		}
	}
}

func TestConfigureDefaultsAndOverrides(t *testing.T) {
	t.Cleanup(resetConfig)

	if err := configure(RunOptions{}); err != nil {
		t.Fatalf("configure(empty): %v", err)
	}
	if cfg.evalsDir != ".kairon/evals" || cfg.backend.Name() != "kiro-cli" {
		t.Errorf("empty options should give defaults, got %q / %s", cfg.evalsDir, cfg.backend.Name())
	}

	if err := configure(RunOptions{Backend: "stub", EvalsDir: "custom/evals"}); err != nil {
		t.Fatalf("configure(stub): %v", err)
	}
	if cfg.evalsDir != "custom/evals" || cfg.backend.Name() != "stub" {
		t.Errorf("got %q / %s", cfg.evalsDir, cfg.backend.Name())
	}
	if got, want := evalsPath("results", "x"), filepath.Join("custom", "evals", "results", "x"); got != want {
		t.Errorf("evalsPath = %q, want %q", got, want)
	}

	// A later call with empty options must reset to defaults.
	if err := configure(RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if cfg.evalsDir != ".kairon/evals" || cfg.backend.Name() != "kiro-cli" {
		t.Errorf("configure did not reset to defaults: %q / %s", cfg.evalsDir, cfg.backend.Name())
	}
}

func TestConfigureUnknownBackend(t *testing.T) {
	t.Cleanup(resetConfig)
	resetConfig()

	err := configure(RunOptions{Backend: "nope", EvalsDir: "x"})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	for _, s := range []string{`"nope"`, "kiro-cli", "stub"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q missing %q", err, s)
		}
	}
	// State must be untouched on failure.
	if cfg.evalsDir != ".kairon/evals" {
		t.Errorf("evalsDir changed on failed configure: %q", cfg.evalsDir)
	}
}

func TestRunWithOptionsRejectsUnknownBackend(t *testing.T) {
	t.Cleanup(resetConfig)
	err := RunWithOptions("a", "", RunOptions{Backend: "nope"})
	if err == nil || !strings.Contains(err.Error(), "valid backends") {
		t.Fatalf("expected unknown backend error, got %v", err)
	}
}

func TestRebaseEvalsPath(t *testing.T) {
	t.Cleanup(resetConfig)

	// Default evals dir: nothing is rebased.
	resetConfig()
	p := ".kairon/evals/fixtures/a.md"
	if got := rebaseEvalsPath(p); got != p {
		t.Errorf("default dir rebased %q to %q", p, got)
	}

	if err := configure(RunOptions{EvalsDir: "internal/eval/testdata/evals"}); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ in, want string }{
		{".kairon/evals/fixtures/a.md", filepath.Join("internal/eval/testdata/evals", "fixtures", "a.md")},
		{"docs/readme.md", "docs/readme.md"},
		{".kairon/other/x.md", ".kairon/other/x.md"},
		{".kairon/evals", ".kairon/evals"}, // no trailing slash: not inside the dir
		{".kairon/evalsx/a.md", ".kairon/evalsx/a.md"},
		{"/abs/.kairon/evals/a.md", "/abs/.kairon/evals/a.md"},
	}
	for _, tt := range tests {
		if got := rebaseEvalsPath(tt.in); got != tt.want {
			t.Errorf("rebaseEvalsPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAssemblePromptRebasesSetupFiles(t *testing.T) {
	dir := chdirTemp(t)
	writeCfgFile(t, filepath.Join(dir, "alt", "fixtures", "f.md"), "ALT FIXTURE")
	writeCfgFile(t, filepath.Join(dir, ".kairon", "evals", "fixtures", "f.md"), "DEFAULT FIXTURE")
	writeCfgFile(t, filepath.Join(dir, "plain.md"), "PLAIN")

	setup := []SetupEntry{
		{Type: "file", Label: "fx", Path: ".kairon/evals/fixtures/f.md"},
		{Type: "file", Label: "plain", Path: "plain.md"},
	}

	// Default: reads the path as written.
	got, err := assemblePrompt(setup, "task")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "DEFAULT FIXTURE") || strings.Contains(got, "ALT FIXTURE") {
		t.Errorf("default run should read default fixture:\n%s", got)
	}

	// Non-default evals dir: prefixed path rebased, other path untouched.
	if err := configure(RunOptions{EvalsDir: "alt"}); err != nil {
		t.Fatal(err)
	}
	got, err = assemblePrompt(setup, "task")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ALT FIXTURE") || strings.Contains(got, "DEFAULT FIXTURE") {
		t.Errorf("rebased run should read alt fixture:\n%s", got)
	}
	if !strings.Contains(got, "PLAIN") {
		t.Errorf("unprefixed path should be untouched:\n%s", got)
	}
}

func TestLoadRubricsAndCasesUseEvalsDir(t *testing.T) {
	dir := chdirTemp(t)
	writeCfgFile(t, filepath.Join(dir, "alt", "rubrics", "r.yaml"), "agent: a1\ncriteria:\n  - name: c\n    scoring: \"1-5\"\n")
	writeCfgFile(t, filepath.Join(dir, "alt", "cases", "a1", "c1.yaml"), "name: case-one\nagent: a1\ninput: hi\n")

	// Default dir has nothing: loading fails.
	if _, err := loadRubrics(""); err == nil {
		t.Error("expected error loading rubrics from missing default dir")
	}
	if _, err := loadCases("a1"); err == nil {
		t.Error("expected error loading cases from missing default dir")
	}

	if err := configure(RunOptions{EvalsDir: "alt"}); err != nil {
		t.Fatal(err)
	}
	rubrics, err := loadRubrics("a1")
	if err != nil || len(rubrics) != 1 {
		t.Fatalf("loadRubrics: %v, %d rubrics", err, len(rubrics))
	}
	cases, err := loadCases("a1")
	if err != nil || len(cases) != 1 || cases[0].Name != "case-one" {
		t.Fatalf("loadCases: %v, %+v", err, cases)
	}
}

func TestRunUsesEvalsDirForRubricsCheck(t *testing.T) {
	chdirTemp(t)
	err := configure(RunOptions{EvalsDir: "missing-dir"})
	if err != nil {
		t.Fatal(err)
	}
	err = Run("a1", nil)
	if err == nil || !strings.Contains(err.Error(), filepath.Join("missing-dir", "rubrics")) {
		t.Fatalf("expected rubrics-dir-not-found error naming the configured dir, got %v", err)
	}
}

func TestDiffResolvesUnderEvalsDir(t *testing.T) {
	dir := chdirTemp(t)
	for _, run := range []string{"run-a", "run-b"} {
		base := filepath.Join(dir, "alt", "results", run)
		writeCfgFile(t, filepath.Join(base, "summary.json"), `{"git_hash":"abc","total_cost":{},"agent_scores":{"a1":0.5}}`)
		writeCfgFile(t, filepath.Join(base, "a1.json"), `{"agent":"a1","git_hash":"abc","cases":[]}`)
	}

	if err := configure(RunOptions{EvalsDir: "alt"}); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveRunDirectory("run-a"); err != nil || got != "run-a" {
		t.Fatalf("resolveRunDirectory under alt: %q, %v", got, err)
	}
	if err := Diff("run-a", "run-b"); err != nil {
		t.Fatalf("Diff under alt evals dir: %v", err)
	}

	// With the default dir the same runs are not found.
	resetConfig()
	if _, err := resolveRunDirectory("run-a"); err == nil {
		t.Error("run should not resolve under the default evals dir")
	}
}

func TestRunWithResumeUsesEvalsDir(t *testing.T) {
	chdirTemp(t)
	if err := configure(RunOptions{EvalsDir: "alt"}); err != nil {
		t.Fatal(err)
	}
	err := runWithResume("a1", nil)
	if err == nil || !strings.Contains(err.Error(), "results directory") {
		t.Fatalf("expected results-directory read error, got %v", err)
	}
	// It must not have created or touched the default location.
	if _, statErr := os.Stat(".kairon"); !os.IsNotExist(statErr) {
		t.Error("default .kairon dir should not exist")
	}
}

func TestCostInfoAdd(t *testing.T) {
	rep := func(in, out int, model string) CostInfo {
		return costFromUsage(model, inference.Usage{InputTokens: in, OutputTokens: out, Source: inference.UsageReported})
	}
	est := func(in, out int) CostInfo {
		return costFromUsage("", inference.Usage{InputTokens: in, OutputTokens: out, Source: inference.UsageEstimated})
	}

	t.Run("all reported stays reported", func(t *testing.T) {
		var c CostInfo
		c.Add(rep(10, 5, "m"))
		c.Add(rep(20, 7, "m"))
		if c.UsageSource != "reported" || c.TokensIn != 30 || c.TokensOut != 12 || c.Model != "m" {
			t.Errorf("got %+v", c)
		}
	})
	t.Run("any estimated makes estimated", func(t *testing.T) {
		var c CostInfo
		c.Add(rep(10, 5, "m"))
		c.Add(est(20, 7))
		if c.UsageSource != "estimated" {
			t.Errorf("got %q", c.UsageSource)
		}
		// Order independence.
		var d CostInfo
		d.Add(est(20, 7))
		d.Add(rep(10, 5, "m"))
		if d.UsageSource != "estimated" {
			t.Errorf("got %q", d.UsageSource)
		}
	})
	t.Run("all estimated", func(t *testing.T) {
		var c CostInfo
		c.Add(est(4, 4))
		c.Add(est(4, 4))
		if c.UsageSource != "estimated" {
			t.Errorf("got %q", c.UsageSource)
		}
	})
	t.Run("empty parts do not downgrade reported", func(t *testing.T) {
		c := rep(1, 1, "m")
		c.Add(CostInfo{})
		if c.UsageSource != "reported" {
			t.Errorf("got %q", c.UsageSource)
		}
	})
	t.Run("costs sum", func(t *testing.T) {
		var c CostInfo
		c.Add(est(1000, 1000))
		c.Add(est(1000, 1000))
		if want := 2 * (1000*3.0/1e6 + 1000*15.0/1e6); c.EstimatedUSD < want-1e-12 || c.EstimatedUSD > want+1e-12 {
			t.Errorf("EstimatedUSD = %v, want %v", c.EstimatedUSD, want)
		}
	})
}

func TestCostFromUsage(t *testing.T) {
	c := costFromUsage("mdl", inference.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, Source: inference.UsageReported})
	if c.TokensIn != 1_000_000 || c.TokensOut != 1_000_000 || c.Model != "mdl" || c.UsageSource != "reported" {
		t.Errorf("got %+v", c)
	}
	if c.EstimatedUSD != 18.0 {
		t.Errorf("EstimatedUSD = %v, want 18", c.EstimatedUSD)
	}
}

func TestCostInfoJSONOmitsEmptyNewFields(t *testing.T) {
	b, err := json.Marshal(CostInfo{TokensIn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "model") || strings.Contains(string(b), "usage_source") {
		t.Errorf("empty fields should be omitted: %s", b)
	}

	b, _ = json.Marshal(CostInfo{Model: "m", UsageSource: "reported"})
	if !strings.Contains(string(b), `"model":"m"`) || !strings.Contains(string(b), `"usage_source":"reported"`) {
		t.Errorf("fields missing: %s", b)
	}
}

func TestTestCaseParsesStubFromYAML(t *testing.T) {
	src := `
name: stub-usage
agent: selftest
input: hello
stub:
  turns:
    - response: "## A\n### B"
      model: stub-model
      usage:
        input_tokens: 123
        output_tokens: 45
    - response: second
`
	var tc TestCase
	if err := yaml.Unmarshal([]byte(src), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Stub == nil || len(tc.Stub.Turns) != 2 {
		t.Fatalf("stub not parsed: %+v", tc.Stub)
	}
	turn := tc.Stub.Turns[0]
	if turn.Response != "## A\n### B" || turn.Model != "stub-model" {
		t.Errorf("turn = %+v", turn)
	}
	if turn.Usage == nil || turn.Usage.InputTokens != 123 || turn.Usage.OutputTokens != 45 {
		t.Errorf("usage = %+v", turn.Usage)
	}
	if tc.Stub.Turns[1].Usage != nil {
		t.Error("second turn should have no usage")
	}

	// Cases without a stub leave it nil.
	var plain TestCase
	if err := yaml.Unmarshal([]byte("name: n\nagent: a\ninput: i\n"), &plain); err != nil {
		t.Fatal(err)
	}
	if plain.Stub != nil {
		t.Error("Stub should be nil when absent")
	}
}
