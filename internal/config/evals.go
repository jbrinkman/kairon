package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultJudgeModel is the model used for LLM-judge calls unless overridden.
const DefaultJudgeModel = "claude-sonnet-5.5"

// EvalsConfig configures model pinning for `kairon eval`.
type EvalsConfig struct {
	AgentModel    string   `yaml:"agent_model"`    // optional override for the agent under test
	JudgeModel    string   `yaml:"judge_model"`    // default claude-sonnet-5.5
	AllowedModels []string `yaml:"allowed_models"` // models an eval run may use
}

// DefaultEvalsConfig returns the default evals configuration. The returned
// AllowedModels slice is freshly allocated on every call, so callers may
// mutate it.
func DefaultEvalsConfig() EvalsConfig {
	return EvalsConfig{
		AgentModel: "",
		JudgeModel: DefaultJudgeModel,
		AllowedModels: []string{
			"claude-sonnet-5.5",
			"claude-sonnet-5",
			"claude-sonnet-4.6",
			"claude-sonnet-4.5",
			"claude-sonnet-4",
			"claude-haiku-4.5",
		},
	}
}

// normalize trims whitespace from all fields and drops empty allowlist entries.
func (e *EvalsConfig) normalize() {
	e.AgentModel = strings.TrimSpace(e.AgentModel)
	e.JudgeModel = strings.TrimSpace(e.JudgeModel)
	allowed := make([]string, 0, len(e.AllowedModels))
	for _, m := range e.AllowedModels {
		if m = strings.TrimSpace(m); m != "" {
			allowed = append(allowed, m)
		}
	}
	e.AllowedModels = allowed
}

// LoadEvals reads only the `evals` key of ./.kairon/config.yaml. A missing file
// yields the defaults; any other read error or a YAML parse error is returned.
// Unlike Load it does not require `repo` and does not load a theme.
func LoadEvals() (EvalsConfig, error) {
	const path = ".kairon/config.yaml"

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultEvalsConfig(), nil
		}
		return EvalsConfig{}, fmt.Errorf("failed to read config file: %w", err)
	}

	file := struct {
		Evals EvalsConfig `yaml:"evals"`
	}{Evals: DefaultEvalsConfig()}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return EvalsConfig{}, fmt.Errorf("failed to parse config: %w", err)
	}

	file.Evals.normalize()
	return file.Evals, nil
}

// CheckModel returns nil only if model (trimmed) is non-empty, is not "auto"
// (case-insensitive) and is exactly one of AllowedModels. Every error names
// the rejected model (or says it is empty) and prints the allowlist.
func (e EvalsConfig) CheckModel(model string) error {
	model = strings.TrimSpace(model)
	allowlist := "[" + strings.Join(e.AllowedModels, ", ") + "]"

	if len(e.AllowedModels) == 0 {
		if model == "" {
			return fmt.Errorf("model is empty; allowed_models is empty: no model can be used")
		}
		return fmt.Errorf("model %q is not permitted; allowed_models is empty: no model can be used", model)
	}
	if model == "" {
		return fmt.Errorf("model is empty; allowed_models: %s", allowlist)
	}
	if strings.EqualFold(model, "auto") {
		return fmt.Errorf("model %q is not permitted; allowed_models: %s", model, allowlist)
	}
	for _, allowed := range e.AllowedModels {
		if model == allowed {
			return nil
		}
	}
	return fmt.Errorf("model %q is not permitted; allowed_models: %s", model, allowlist)
}
