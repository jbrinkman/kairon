package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// scalarCase is one config.yaml input and the expected value of one scalar
// field after Load(). get returns the field under test as a comparable value.
type scalarCase struct {
	name          string
	configContent string
	want          any
}

// runScalarCases writes each case's YAML with writeProjectConfig, calls Load()
// and compares the field selected by get with the expected value. It does not
// use t.Parallel because writeProjectConfig changes the working directory.
func runScalarCases(t *testing.T, field string, get func(*Config) any, cases []scalarCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			writeProjectConfig(t, strPtr(tt.configContent))

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if got := get(cfg); got != tt.want {
				t.Errorf("%s = %#v, expected %#v", field, got, tt.want)
			}
		})
	}
}

func TestLoad_ScalarFieldDecoding(t *testing.T) {
	t.Run("EnableCopilotReviewDefaults", func(t *testing.T) {
		runScalarCases(t, "EnableCopilotReview", func(c *Config) any { return c.EnableCopilotReview }, []scalarCase{
			{
				name: "enable_copilot_review not specified - should default to true",
				configContent: `repo: test/repo
label: test-label`,
				want: true,
			},
			{
				name: "enable_copilot_review explicitly set to true",
				configContent: `repo: test/repo
label: test-label
enable_copilot_review: true`,
				want: true,
			},
			{
				name: "enable_copilot_review explicitly set to false",
				configContent: `repo: test/repo
label: test-label
enable_copilot_review: false`,
				want: false,
			},
		})
	})

	t.Run("UserField", func(t *testing.T) {
		runScalarCases(t, "User", func(c *Config) any { return c.User }, []scalarCase{
			{
				name: "user field omitted - should default to empty (auto-detect sentinel)",
				configContent: `repo: test/repo
label: test-label`,
				want: "",
			},
			{
				name: "user field explicitly set",
				configContent: `repo: test/repo
label: test-label
user: octocat`,
				want: "octocat",
			},
		})
	})

	t.Run("BaseBranch", func(t *testing.T) {
		runScalarCases(t, "BaseBranch", func(c *Config) any { return c.BaseBranch }, []scalarCase{
			{
				name: "base_branch omitted - should default to empty (auto-detect)",
				configContent: `repo: test/repo
label: test-label`,
				want: "",
			},
			{
				name: "base_branch explicitly set",
				configContent: `repo: test/repo
label: test-label
base_branch: develop`,
				want: "develop",
			},
			{
				name: "base_branch only in a comment - should remain empty",
				configContent: `repo: test/repo
label: test-label
# base_branch: main`,
				want: "",
			},
		})
	})
}

func TestLoad_AllDefaultValues(t *testing.T) {
	// Test that all default values are set correctly
	tmpDir := t.TempDir()
	configDir := tmpDir + string(os.PathSeparator) + ".kairon"
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configContent := `repo: test/repo`
	configFile := configDir + string(os.PathSeparator) + "config.yaml"
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	})
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify all default values
	if cfg.Label != "kairon" {
		t.Errorf("Label = %s, expected kairon", cfg.Label)
	}
	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, expected 5m0s", cfg.PollInterval)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, expected 3", cfg.MaxRetries)
	}
	if cfg.MaxActivityLines != 1000 {
		t.Errorf("MaxActivityLines = %d, expected 1000", cfg.MaxActivityLines)
	}
	if cfg.ConsoleLogging != false {
		t.Errorf("ConsoleLogging = %v, expected false", cfg.ConsoleLogging)
	}
	if cfg.Theme != "default" {
		t.Errorf("Theme = %s, expected default", cfg.Theme)
	}
	if cfg.EnableCopilotReview != true {
		t.Errorf("EnableCopilotReview = %v, expected true", cfg.EnableCopilotReview)
	}

	// Shared-cfg subtests: the default Evals and Pricing blocks are loaded
	// alongside the scalar defaults when the config file has only a repo.
	t.Run("evals_defaults", func(t *testing.T) {
		if !reflect.DeepEqual(cfg.Evals, DefaultEvalsConfig()) {
			t.Errorf("Evals = %+v, expected %+v", cfg.Evals, DefaultEvalsConfig())
		}
	})
	t.Run("pricing_defaults", func(t *testing.T) {
		if !reflect.DeepEqual(cfg.Pricing, DefaultPricing()) {
			t.Errorf("Pricing = %+v, expected %+v", cfg.Pricing, DefaultPricing())
		}
	})
}

func TestLoad_MaxAgentsDefaultIsFinitePositive(t *testing.T) {
	if DefaultMaxAgents <= 0 {
		t.Fatalf("DefaultMaxAgents = %d, expected a positive number", DefaultMaxAgents)
	}

	t.Run("key absent", func(t *testing.T) {
		writeProjectConfig(t, strPtr("repo: test/repo\nlabel: test-label"))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.MaxAgents != DefaultMaxAgents {
			t.Errorf("MaxAgents = %d, expected DefaultMaxAgents (%d)", cfg.MaxAgents, DefaultMaxAgents)
		}
	})

	// The config that `kairon init` writes must load, with repo filled in, and
	// carry the default.
	t.Run("shipped template", func(t *testing.T) {
		tmpl, err := os.ReadFile("../../cmd/kairon/templates/kairon/config.yaml")
		if err != nil {
			t.Fatalf("read template config: %v", err)
		}
		content := strings.Replace(string(tmpl), `repo: ""`, `repo: test/repo`, 1)
		if content == string(tmpl) {
			t.Fatal(`template config has no 'repo: ""' line to fill in`)
		}
		writeProjectConfig(t, strPtr(content))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() of template error = %v", err)
		}
		if cfg.MaxAgents != DefaultMaxAgents {
			t.Errorf("template MaxAgents = %d, expected DefaultMaxAgents (%d)", cfg.MaxAgents, DefaultMaxAgents)
		}
	})
}

func TestLoad_MaxAgentsRejectsNonPositive(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		t.Run("max_agents "+value, func(t *testing.T) {
			writeProjectConfig(t, strPtr("repo: test/repo\nlabel: test-label\nmax_agents: "+value))
			cfg, err := Load()
			if err == nil {
				t.Fatalf("Load() = %+v, expected an error for max_agents: %s", cfg, value)
			}
			if !strings.Contains(err.Error(), "max_agents") {
				t.Errorf("error %q does not mention max_agents", err)
			}
		})
	}
}

func TestLoad_MaxAgentsAcceptsPositive(t *testing.T) {
	runScalarCases(t, "MaxAgents", func(c *Config) any { return c.MaxAgents }, []scalarCase{
		{
			name:          "max_agents 5",
			configContent: "repo: test/repo\nlabel: test-label\nmax_agents: 5",
			want:          5,
		},
	})
}

func TestEffectiveMaxAgents(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"zero uses the default", 0, DefaultMaxAgents},
		{"negative uses the default", -1, DefaultMaxAgents},
		{"large negative uses the default", -1 << 31, DefaultMaxAgents},
		{"one is unchanged", 1, 1},
		{"positive is unchanged", 7, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&Config{MaxAgents: tt.in}).EffectiveMaxAgents()
			if got != tt.want {
				t.Errorf("EffectiveMaxAgents() with MaxAgents=%d = %d, expected %d", tt.in, got, tt.want)
			}
			if got <= 0 {
				t.Errorf("EffectiveMaxAgents() = %d, expected a positive (bounded) value", got)
			}
		})
	}
}
