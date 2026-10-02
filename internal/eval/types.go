package eval

import (
	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

// Rubric defines scoring criteria for an agent.
type Rubric struct {
	Agent    string      `yaml:"agent" json:"agent"`
	Criteria []Criterion `yaml:"criteria" json:"criteria"`
}

// Criterion is a single scoring dimension within a rubric.
type Criterion struct {
	Name          string `yaml:"name" json:"name"`
	Description   string `yaml:"description" json:"description"`
	Scoring       string `yaml:"scoring" json:"scoring"` // e.g. "1-5"
	Deterministic bool   `yaml:"deterministic,omitempty" json:"deterministic,omitempty"`
	Type          string `yaml:"type,omitempty" json:"type,omitempty"` // e.g. "cost"
}

// SetupEntry provides context or files for agent setup.
type SetupEntry struct {
	Type    string `yaml:"type" json:"type"`                     // text, file, or url
	Label   string `yaml:"label" json:"label"`                   // descriptive label
	Content string `yaml:"content" json:"content"`               // text content or file path or url
	Path    string `yaml:"path,omitempty" json:"path,omitempty"` // optional path for file entries
}

// TestCase defines input and expected characteristics for an agent evaluation.
type TestCase struct {
	Name           string       `yaml:"name" json:"name"`
	Description    string       `yaml:"description" json:"description"`
	Input          string       `yaml:"input" json:"input"`
	ExpectedOutput string       `yaml:"expected_output,omitempty" json:"expected_output,omitempty"`
	Context        []string     `yaml:"context,omitempty" json:"context,omitempty"`
	Setup          []SetupEntry `yaml:"setup,omitempty" json:"setup,omitempty"`
	Agent          string       `yaml:"agent" json:"agent"`
	MinScore       *float64     `yaml:"min_score,omitempty" json:"min_score,omitempty"` // Success threshold (0-100), defaults to 80%
}

// CostInfo tracks token usage and estimated cost.
type CostInfo struct {
	TokensIn     int     `json:"tokens_in"`
	TokensOut    int     `json:"tokens_out"`
	EstimatedUSD float64 `json:"estimated_usd"`
}

// CriterionScore is the score for a single criterion on a single test case.
type CriterionScore struct {
	Name          string `json:"name"`
	Score         int    `json:"score"`
	MaxScore      int    `json:"max_score"`
	Deterministic bool   `json:"deterministic"`
	Skipped       bool   `json:"skipped,omitempty"`
	Reasoning     string `json:"reasoning,omitempty"`
}

// CaseResult holds scores and cost for one test case.
type CaseResult struct {
	CaseName     string           `json:"case_name"`
	ActualOutput string           `json:"actual_output"`
	Scores       []CriterionScore `json:"scores"`
	AgentCost    CostInfo         `json:"agent_cost"`
	JudgeCost    CostInfo         `json:"judge_cost"`
	ErrorContext *ErrorContext    `json:"error_context,omitempty"`
}

// ErrorContext captures execution details for debugging failed tests.
type ErrorContext struct {
	Command        string            `json:"command"`
	WorkingDir     string            `json:"working_dir"`
	Environment    map[string]string `json:"environment,omitempty"`
	Stderr         string            `json:"stderr,omitempty"`
	ExitCode       int               `json:"exit_code,omitempty"`
	ContainerID    string            `json:"container_id,omitempty"`    // Container short ID for sandbox mode
	ContainerImage string            `json:"container_image,omitempty"` // Container image name
	Platform       string            `json:"platform,omitempty"`        // Container platform
	DockerError    string            `json:"docker_error,omitempty"`    // Docker-specific error details
}

// AgentResult holds all case results for one agent.
type AgentResult struct {
	Agent   string       `json:"agent"`
	GitHash string       `json:"git_hash"`
	Cases   []CaseResult `json:"cases"`
}

// RunOptions configures evaluation execution.
type RunOptions struct {
	List          bool              // List available test cases
	Resume        bool              // Resume from interrupted evaluation
	Sandbox       bool              // Enable container sandboxing
	NoSandbox     bool              // Explicitly disable sandboxing
	ResourceLimit map[string]string // Resource limit overrides (cpu, memory, timeout)
	Debug         bool              // Enable debug mode with verbose logging
	Cleanup       bool              // Stop and remove tracked containers
}

// Summary holds aggregate results for an eval run.
type Summary struct {
	GitHash     string             `json:"git_hash"`
	TotalCost   CostInfo           `json:"total_cost"`
	AgentScores map[string]float64 `json:"agent_scores"` // agent -> average score
}

// ContainerConfig configures containerized execution
type ContainerConfig struct {
	Platform        string                 `json:"platform"`
	ResourceLimits  sandbox.ResourceLimits `json:"resource_limits"`
	Environment     map[string]string      `json:"environment"`
	WorkspaceDir    string                 `json:"workspace_dir"`
	MockGitHub      bool                   `json:"mock_github"`
	Debug           bool                   `json:"debug"`
	ImageManager    *sandbox.ImageManager  `json:"-"`
	CachedImageName string                 `json:"-"`
}

// ProjectDetection holds results from project type detection
type ProjectDetection struct {
	ProjectType  string            `json:"project_type"`
	ConfigFiles  []string          `json:"config_files"`
	Dependencies map[string]string `json:"dependencies"`
}
