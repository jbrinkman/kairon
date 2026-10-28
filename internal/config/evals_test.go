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

func TestDefaultEvalsConfigReturnsFreshSlice(t *testing.T) {
	a := DefaultEvalsConfig()
	a.AllowedModels[0] = "mutated"
	b := DefaultEvalsConfig()
	if b.AllowedModels[0] != "claude-sonnet-5.5" {
		t.Errorf("mutation leaked into later defaults: %v", b.AllowedModels)
	}
}

// loadAny adapts a typed loader to a common signature so one table can cover
// LoadEvals and LoadPricing.
type loadAny func() (any, error)

func loadEvalsAny() (any, error)   { return LoadEvals() }
func loadPricingAny() (any, error) { return LoadPricing() }

func TestLoadEvalsAndPricing_MissingFileReturnsDefaults(t *testing.T) {
	tests := []struct {
		name string
		load loadAny
		want any
	}{
		{"LoadEvals", loadEvalsAny, DefaultEvalsConfig()},
		{"LoadPricing", loadPricingAny, DefaultPricing()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectConfig(t, nil)
			got, err := tt.load()
			if err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s = %+v, want defaults", tt.name, got)
			}
		})
	}
}

func TestLoadEvalsAndPricing_AbsentKeyReturnsDefaults(t *testing.T) {
	// No repo key in any row: the loaders must not require it.
	tests := []struct {
		name string
		yaml string
		load loadAny
		want any
	}{
		{"LoadEvals_NoEvalsKey", "label: foo\n", loadEvalsAny, DefaultEvalsConfig()},
		{"LoadPricing_NoPricingKeyWithoutRepo", "label: foo\n", loadPricingAny, DefaultPricing()},
		{"LoadPricing_NullKey", "pricing:\n", loadPricingAny, DefaultPricing()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectConfig(t, strPtr(tt.yaml))
			got, err := tt.load()
			if err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s = %+v, want defaults", tt.name, got)
			}
		})
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
	// One check per row: allowed_models: [] must keep its len == 0 assertion,
	// so rows are not forced into a single DeepEqual.
	tests := []struct {
		name  string
		yaml  string
		check func(t *testing.T, got EvalsConfig)
	}{
		{
			name: "partial_block",
			yaml: "evals:\n  agent_model: claude-sonnet-4.5\n",
			check: func(t *testing.T, got EvalsConfig) {
				if got.AgentModel != "claude-sonnet-4.5" {
					t.Errorf("AgentModel = %q", got.AgentModel)
				}
				if got.JudgeModel != DefaultJudgeModel {
					t.Errorf("JudgeModel = %q, want default", got.JudgeModel)
				}
				if !reflect.DeepEqual(got.AllowedModels, wantDefaultAllowed) {
					t.Errorf("AllowedModels = %v, want defaults", got.AllowedModels)
				}
			},
		},
		{
			name: "allowed_models_replaces_default",
			yaml: "evals:\n  allowed_models:\n    - my-model\n    - other-model\n",
			check: func(t *testing.T, got EvalsConfig) {
				want := []string{"my-model", "other-model"}
				if !reflect.DeepEqual(got.AllowedModels, want) {
					t.Errorf("AllowedModels = %v, want %v (replace, not append)", got.AllowedModels, want)
				}
			},
		},
		{
			// The empty judge model is rejected by CheckModel; that is the
			// TestCheckModel "empty" case, so it is not repeated here.
			name: "explicit_empty_judge_model",
			yaml: "evals:\n  judge_model: \"\"\n",
			check: func(t *testing.T, got EvalsConfig) {
				if got.JudgeModel != "" {
					t.Errorf("JudgeModel = %q, want empty", got.JudgeModel)
				}
			},
		},
		{
			name: "explicit_empty_allowed_models",
			yaml: "evals:\n  allowed_models: []\n",
			check: func(t *testing.T, got EvalsConfig) {
				if len(got.AllowedModels) != 0 {
					t.Errorf("AllowedModels = %v, want empty", got.AllowedModels)
				}
			},
		},
		{
			name: "trust_tools_defaults_nil",
			yaml: "evals:\n  judge_model: claude-sonnet-5.5\n",
			check: func(t *testing.T, got EvalsConfig) {
				if got.TrustTools != nil {
					t.Errorf("TrustTools = %#v, want nil", got.TrustTools)
				}
				if DefaultEvalsConfig().TrustTools != nil {
					t.Error("default TrustTools must be nil")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectConfig(t, strPtr(tt.yaml))
			got, err := LoadEvals()
			if err != nil {
				t.Fatalf("LoadEvals() error = %v", err)
			}
			tt.check(t, got)
		})
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

func TestLoad_EvalsPartialBlockAndNoModelRejection(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		check func(t *testing.T, cfg *Config)
	}{
		{
			// Load must not reject model values: enforcement is a run-time concern.
			name: "partial_block_and_no_model_rejection",
			yaml: `repo: owner/name
evals:
  judge_model: auto
  agent_model: not-a-real-model
`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Evals.JudgeModel != "auto" || cfg.Evals.AgentModel != "not-a-real-model" {
					t.Errorf("cfg.Evals = %+v", cfg.Evals)
				}
				if !reflect.DeepEqual(cfg.Evals.AllowedModels, wantDefaultAllowed) {
					t.Errorf("AllowedModels = %v, want defaults", cfg.Evals.AllowedModels)
				}
			},
		},
		{
			name: "allowed_models_replaces_default",
			yaml: "repo: owner/name\nevals:\n  allowed_models: [only-one]\n",
			check: func(t *testing.T, cfg *Config) {
				if !reflect.DeepEqual(cfg.Evals.AllowedModels, []string{"only-one"}) {
					t.Errorf("AllowedModels = %v", cfg.Evals.AllowedModels)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectConfig(t, strPtr(tt.yaml))
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			tt.check(t, cfg)
		})
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
