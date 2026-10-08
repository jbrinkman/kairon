package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var wantDefaultAllowed = []string{
	"claude-sonnet-5.5",
	"claude-sonnet-5",
	"claude-sonnet-4.6",
	"claude-sonnet-4.5",
	"claude-sonnet-4",
	"claude-haiku-4.5",
}

// writeProjectConfig chdirs into a fresh temp dir and, when content is
// non-nil, writes it to .kairon/config.yaml.
func writeProjectConfig(t *testing.T, content *string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if content == nil {
		return
	}
	if err := os.MkdirAll(".kairon", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".kairon", "config.yaml"), []byte(*content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func strPtr(s string) *string { return &s }

func TestDefaultEvalsConfig(t *testing.T) {
	d := DefaultEvalsConfig()
	if d.AgentModel != "" {
		t.Errorf("AgentModel = %q, want empty", d.AgentModel)
	}
	if d.JudgeModel != "claude-sonnet-5.5" {
		t.Errorf("JudgeModel = %q, want claude-sonnet-5.5", d.JudgeModel)
	}
	if !reflect.DeepEqual(d.AllowedModels, wantDefaultAllowed) {
		t.Errorf("AllowedModels = %v, want %v", d.AllowedModels, wantDefaultAllowed)
	}
}

func TestDefaultEvalsConfigReturnsFreshSlice(t *testing.T) {
	a := DefaultEvalsConfig()
	a.AllowedModels[0] = "mutated"
	b := DefaultEvalsConfig()
	if b.AllowedModels[0] != "claude-sonnet-5.5" {
		t.Errorf("mutation leaked into later defaults: %v", b.AllowedModels)
	}
}

func TestLoadEvals_MissingFileReturnsDefaults(t *testing.T) {
	writeProjectConfig(t, nil)
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	if !reflect.DeepEqual(got, DefaultEvalsConfig()) {
		t.Errorf("LoadEvals() = %+v, want defaults", got)
	}
}

func TestLoadEvals_NoEvalsKeyReturnsDefaults(t *testing.T) {
	// No repo key: LoadEvals must not require it.
	writeProjectConfig(t, strPtr("label: foo\n"))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	if !reflect.DeepEqual(got, DefaultEvalsConfig()) {
		t.Errorf("LoadEvals() = %+v, want defaults", got)
	}
}

func TestLoadEvals_MalformedYAMLErrors(t *testing.T) {
	writeProjectConfig(t, strPtr("evals:\n  judge_model: [unterminated\n"))
	if _, err := LoadEvals(); err == nil {
		t.Fatal("LoadEvals() error = nil, want parse error")
	}
}

func TestLoadEvals_ReadErrorOtherThanNotExist(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// A directory where the file should be: read fails with a non-NotExist error.
	if err := os.MkdirAll(filepath.Join(".kairon", "config.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEvals(); err == nil {
		t.Fatal("LoadEvals() error = nil, want read error")
	}
}

func TestLoadEvals_PartialBlockKeepsOtherDefaults(t *testing.T) {
	writeProjectConfig(t, strPtr("evals:\n  agent_model: claude-sonnet-4.5\n"))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	if got.AgentModel != "claude-sonnet-4.5" {
		t.Errorf("AgentModel = %q", got.AgentModel)
	}
	if got.JudgeModel != DefaultJudgeModel {
		t.Errorf("JudgeModel = %q, want default", got.JudgeModel)
	}
	if !reflect.DeepEqual(got.AllowedModels, wantDefaultAllowed) {
		t.Errorf("AllowedModels = %v, want defaults", got.AllowedModels)
	}
}

func TestLoadEvals_AllowedModelsReplacesDefault(t *testing.T) {
	writeProjectConfig(t, strPtr("evals:\n  allowed_models:\n    - my-model\n    - other-model\n"))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	want := []string{"my-model", "other-model"}
	if !reflect.DeepEqual(got.AllowedModels, want) {
		t.Errorf("AllowedModels = %v, want %v (replace, not append)", got.AllowedModels, want)
	}
}

func TestLoadEvals_TrimsWhitespaceAndDropsEmptyEntries(t *testing.T) {
	writeProjectConfig(t, strPtr(`evals:
  agent_model: "  claude-sonnet-4.5 "
  judge_model: " claude-sonnet-5 "
  allowed_models:
    - " claude-sonnet-5 "
    - ""
    - "   "
    - claude-sonnet-4.5
`))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	if got.AgentModel != "claude-sonnet-4.5" {
		t.Errorf("AgentModel = %q", got.AgentModel)
	}
	if got.JudgeModel != "claude-sonnet-5" {
		t.Errorf("JudgeModel = %q", got.JudgeModel)
	}
	want := []string{"claude-sonnet-5", "claude-sonnet-4.5"}
	if !reflect.DeepEqual(got.AllowedModels, want) {
		t.Errorf("AllowedModels = %v, want %v", got.AllowedModels, want)
	}
}

func TestLoadEvals_ExplicitEmptyJudgeModelStaysEmpty(t *testing.T) {
	writeProjectConfig(t, strPtr("evals:\n  judge_model: \"\"\n"))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	if got.JudgeModel != "" {
		t.Errorf("JudgeModel = %q, want empty", got.JudgeModel)
	}
	if err := got.CheckModel(got.JudgeModel); err == nil {
		t.Error("CheckModel(empty judge model) = nil, want error")
	}
}

func TestLoadEvals_ExplicitEmptyAllowedModels(t *testing.T) {
	writeProjectConfig(t, strPtr("evals:\n  allowed_models: []\n"))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	if len(got.AllowedModels) != 0 {
		t.Errorf("AllowedModels = %v, want empty", got.AllowedModels)
	}
}

func TestCheckModel(t *testing.T) {
	cfg := DefaultEvalsConfig()
	allowlist := "[" + strings.Join(wantDefaultAllowed, ", ") + "]"

	for _, m := range wantDefaultAllowed {
		if err := cfg.CheckModel(m); err != nil {
			t.Errorf("CheckModel(%q) = %v, want nil", m, err)
		}
	}
	if err := cfg.CheckModel("  claude-sonnet-4.5 "); err != nil {
		t.Errorf("CheckModel with surrounding whitespace = %v, want nil", err)
	}

	tests := []struct {
		name        string
		cfg         EvalsConfig
		model       string
		wantContain []string
	}{
		{"empty", cfg, "", []string{"model is empty", "allowed_models: " + allowlist}},
		{"whitespace only", cfg, "   ", []string{"model is empty", "allowed_models: " + allowlist}},
		{"auto", cfg, "auto", []string{`"auto"`, "allowed_models: " + allowlist}},
		{"AUTO", cfg, "AUTO", []string{`"AUTO"`, "allowed_models: " + allowlist}},
		{"Auto", cfg, "Auto", []string{`"Auto"`, "allowed_models: " + allowlist}},
		{"unknown", cfg, "gpt-9", []string{`"gpt-9"`, "allowed_models: " + allowlist}},
		{"case differs from allowlist", cfg, "Claude-Sonnet-5.5", []string{`"Claude-Sonnet-5.5"`, "allowed_models: " + allowlist}},
		{"prefix only", cfg, "claude-sonnet", []string{`"claude-sonnet"`, "allowed_models: " + allowlist}},
		{"auto even when allowlisted", EvalsConfig{AllowedModels: []string{"auto"}}, "auto", []string{`"auto"`, "allowed_models: [auto]"}},
		{"empty allowlist", EvalsConfig{}, "claude-sonnet-5.5", []string{`"claude-sonnet-5.5"`, "allowed_models is empty", "no model can be used"}},
		{"empty allowlist and empty model", EvalsConfig{}, "", []string{"model is empty", "allowed_models is empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.CheckModel(tt.model)
			if err == nil {
				t.Fatalf("CheckModel(%q) = nil, want error", tt.model)
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

func TestLoad_ExposesEvalsDefaults(t *testing.T) {
	writeProjectConfig(t, strPtr("repo: owner/name\n"))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(cfg.Evals, DefaultEvalsConfig()) {
		t.Errorf("cfg.Evals = %+v, want defaults", cfg.Evals)
	}
}

func TestLoad_EvalsPartialBlockAndNoModelRejection(t *testing.T) {
	// Load must not reject model values: enforcement is a run-time concern.
	writeProjectConfig(t, strPtr(`repo: owner/name
evals:
  judge_model: auto
  agent_model: not-a-real-model
`))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() rejected model values: %v", err)
	}
	if cfg.Evals.JudgeModel != "auto" || cfg.Evals.AgentModel != "not-a-real-model" {
		t.Errorf("cfg.Evals = %+v", cfg.Evals)
	}
	if !reflect.DeepEqual(cfg.Evals.AllowedModels, wantDefaultAllowed) {
		t.Errorf("AllowedModels = %v, want defaults", cfg.Evals.AllowedModels)
	}
}

func TestLoad_EvalsAllowedModelsReplacesDefault(t *testing.T) {
	writeProjectConfig(t, strPtr("repo: owner/name\nevals:\n  allowed_models: [only-one]\n"))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(cfg.Evals.AllowedModels, []string{"only-one"}) {
		t.Errorf("AllowedModels = %v", cfg.Evals.AllowedModels)
	}
}

// evalsBlock returns the documented commented evals block of a config file:
// everything from the "# Eval model pinning" header line to the end.
func evalsBlock(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s := string(data)
	idx := strings.Index(s, "# Eval model pinning")
	if idx < 0 {
		t.Fatalf("%s does not contain the \"# Eval model pinning\" block", path)
	}
	return strings.TrimSpace(s[idx:])
}

func TestConfigFilesDocumentEvalsBlock(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", ".kairon", "config.yaml"),
		filepath.Join("..", "..", "cmd", "kairon", "templates", "kairon", "config.yaml"),
	}

	required := []string{"# evals:", "agent_model", "judge_model", "allowed_models", DefaultJudgeModel}
	required = append(required, wantDefaultAllowed...)

	blocks := make([]string, len(paths))
	for i, p := range paths {
		block := evalsBlock(t, p)
		blocks[i] = block
		for _, want := range required {
			if !strings.Contains(block, want) {
				t.Errorf("%s: evals block missing %q", p, want)
			}
		}
		// Every line of the block must be a comment: defaults apply when absent.
		for _, line := range strings.Split(block, "\n") {
			if line != "" && !strings.HasPrefix(line, "#") {
				t.Errorf("%s: evals block has non-comment line %q", p, line)
			}
		}
	}
	if blocks[0] != blocks[1] {
		t.Errorf("evals blocks differ between %s and %s:\n--- live ---\n%s\n--- template ---\n%s",
			paths[0], paths[1], blocks[0], blocks[1])
	}
}

func TestConfigFilesEvalsBlockMatchesDefaults(t *testing.T) {
	// Uncommenting the documented block must yield exactly the defaults.
	block := evalsBlock(t, filepath.Join("..", "..", ".kairon", "config.yaml"))
	var yamlLines []string
	inEvals := false
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "# evals:") {
			inEvals = true
		}
		if inEvals {
			yamlLines = append(yamlLines, strings.TrimPrefix(line, "# "))
		}
	}
	if len(yamlLines) == 0 {
		t.Fatal("no evals: section found in block")
	}
	writeProjectConfig(t, strPtr(strings.Join(yamlLines, "\n")+"\n"))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() on uncommented block: %v", err)
	}
	if !reflect.DeepEqual(got, DefaultEvalsConfig()) {
		t.Errorf("documented block = %+v, want defaults %+v", got, DefaultEvalsConfig())
	}
}

func TestLoadEvals_TrustToolsDefaultsNil(t *testing.T) {
	writeProjectConfig(t, strPtr("evals:\n  judge_model: claude-sonnet-5.5\n"))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	if got.TrustTools != nil {
		t.Errorf("TrustTools = %#v, want nil", got.TrustTools)
	}
	if DefaultEvalsConfig().TrustTools != nil {
		t.Error("default TrustTools must be nil")
	}
}

func TestLoadEvals_TrustToolsParsedAndTrimmed(t *testing.T) {
	writeProjectConfig(t, strPtr(`evals:
  trust_tools:
    " builder ": [" read ", "write", "@srv/tool"]
    "  ": [read]
    reviewer: []
`))
	got, err := LoadEvals()
	if err != nil {
		t.Fatalf("LoadEvals() error = %v", err)
	}
	want := map[string][]string{
		"builder":  {"read", "write", "@srv/tool"},
		"reviewer": {},
	}
	if !reflect.DeepEqual(got.TrustTools, want) {
		t.Errorf("TrustTools = %#v, want %#v", got.TrustTools, want)
	}
}

func TestLoadEvals_TrustToolsRejectsWildcardAndEmpty(t *testing.T) {
	for name, body := range map[string]string{
		"wildcard":        `["read", "*"]`,
		"padded wildcard": `[" * "]`,
		"empty entry":     `["read", ""]`,
		"blank entry":     `["  "]`,
	} {
		t.Run(name, func(t *testing.T) {
			writeProjectConfig(t, strPtr("evals:\n  trust_tools:\n    builder: "+body+"\n"))
			_, err := LoadEvals()
			if err == nil {
				t.Fatal("LoadEvals() error = nil, want rejection")
			}
			if !strings.Contains(err.Error(), `"builder"`) {
				t.Errorf("error %q does not name the agent", err)
			}
		})
	}
}
