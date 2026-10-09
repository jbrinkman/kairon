package eval

import (
	"encoding/json"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/jbrinkman/kairon/internal/inference"
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

	// Stub scripts the agent's response for the stub inference backend.
	// Other backends ignore it.
	Stub *inference.StubScript `yaml:"stub,omitempty" json:"stub,omitempty"`

	// Workspace names a fixture directory under <evals-dir>/fixtures/workspaces/
	// that seeds the case's per-case workspace. Empty means an empty workspace.
	Workspace string `yaml:"workspace,omitempty" json:"workspace,omitempty"`

	// Timeout overrides the agent-call timeout for this case. It is a Go
	// duration string (e.g. "30s") and must be positive when set.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// GHIssue is the data the sandbox's fake gh answers `gh issue view` with.
	// It requires RequiresSandbox. number defaults to 1; title is required.
	GHIssue *sandbox.GHIssue `yaml:"gh_issue,omitempty" json:"gh_issue,omitempty"`

	// RequiresSandbox refuses to run the case natively (without --sandbox).
	RequiresSandbox bool `yaml:"requires_sandbox,omitempty" json:"requires_sandbox,omitempty"`

	// Mocks places author-supplied scripts on the container PATH (in the
	// read-only /opt/kairon/bin mount that also holds the fake gh), so a bare
	// command resolves to the mock instead of a real tool. It requires
	// RequiresSandbox: a native run has no such directory on PATH and would
	// call the real tool.
	Mocks []CaseMock `yaml:"mocks,omitempty" json:"mocks,omitempty"`

	// Checks are deterministic pass/fail assertions on the workspace, the
	// agent output and the gh log. A criterion with checks is scored from them.
	Checks []Check `yaml:"checks,omitempty" json:"checks,omitempty"`
}

// CaseMock declares one mocked command for a case.
type CaseMock struct {
	// Command is the bare command name the script is installed as
	// (for example "aws"). "gh" is reserved for the harness's fake gh.
	Command string `yaml:"command" json:"command"`
	// Script is a path, relative to the evals directory, of the file that
	// implements the command.
	Script string `yaml:"script" json:"script"`
}

// CostInfo tracks token usage and estimated cost.
type CostInfo struct {
	TokensIn     int     `json:"tokens_in"`
	TokensOut    int     `json:"tokens_out"`
	EstimatedUSD float64 `json:"estimated_usd"`

	// Model is the model that served the request, when known.
	Model string `json:"model,omitempty"`
	// UsageSource is "reported" or "estimated" (see inference.UsageSource).
	UsageSource string `json:"usage_source,omitempty"`
}

// Add accumulates other into c. Model is kept if c has none yet. The merged
// UsageSource is "reported" only if every contributing part is "reported";
// a part with no source contributes nothing when the receiver is empty, and
// counts as not reported otherwise.
func (c *CostInfo) Add(other CostInfo) {
	empty := c.TokensIn == 0 && c.TokensOut == 0 && c.EstimatedUSD == 0 && c.UsageSource == ""

	c.TokensIn += other.TokensIn
	c.TokensOut += other.TokensOut
	c.EstimatedUSD += other.EstimatedUSD

	if c.Model == "" {
		c.Model = other.Model
	}

	switch {
	case empty:
		c.UsageSource = other.UsageSource
	case other.UsageSource == "" && other.TokensIn == 0 && other.TokensOut == 0 && other.EstimatedUSD == 0:
		// other is an empty cost; it does not change the merged source.
	case c.UsageSource == string(inference.UsageReported) && other.UsageSource == string(inference.UsageReported):
		c.UsageSource = string(inference.UsageReported)
	default:
		c.UsageSource = string(inference.UsageEstimated)
	}
}

// costFromUsage converts inference usage to a CostInfo using the Claude
// Sonnet pricing estimate ($3/M input, $15/M output tokens).
func costFromUsage(model string, u inference.Usage) CostInfo {
	cost := (float64(u.InputTokens) * 3.0 / 1_000_000) + (float64(u.OutputTokens) * 15.0 / 1_000_000)
	return CostInfo{
		TokensIn:     u.InputTokens,
		TokensOut:    u.OutputTokens,
		EstimatedUSD: cost,
		Model:        model,
		UsageSource:  string(u.Source),
	}
}

// CriterionScore is the score for a single criterion on a single test case.
type CriterionScore struct {
	Name          string `json:"name"`
	Score         int    `json:"score"`
	MaxScore      int    `json:"max_score"`
	Deterministic bool   `json:"deterministic"`
	Skipped       bool   `json:"skipped,omitempty"`
	Reasoning     string `json:"reasoning,omitempty"`

	// Checks holds the per-check results when the criterion is scored by checks.
	Checks []CheckResult `json:"checks,omitempty"`
}

// CaseResult holds scores and cost for one test case.
type CaseResult struct {
	CaseName     string           `json:"case_name"`
	ActualOutput string           `json:"actual_output"`
	Scores       []CriterionScore `json:"scores"`
	AgentCost    CostInfo         `json:"agent_cost"`
	JudgeCost    CostInfo         `json:"judge_cost"`
	ErrorContext *ErrorContext    `json:"error_context,omitempty"`
	// WorkspaceDir is the host path of the case's workspace. It is recorded
	// whether or not the directory is removed after the case.
	WorkspaceDir string `json:"workspace_dir,omitempty"`
	// containerWorkspaceDir is the workspace path *inside* the container
	// (e.g. "/workspace") for a sandbox run, empty for a native run. It lets
	// deterministic scoring rewrite an absolute path the agent reported from
	// inside the container back to the host workspace. Not serialized.
	containerWorkspaceDir string
	// baseCommit is the workspace's fixture commit; changed_files diffs
	// against it so a change the agent committed is still seen. Not serialized.
	baseCommit string
	// Calls holds one record per agent call and per judge call, in execution order.
	Calls []inference.CallRecord `json:"calls,omitempty"`
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
	Agent        string `json:"agent"`
	GitHash      string `json:"git_hash"`
	AgentModel   string `json:"agent_model,omitempty"`
	JudgeModel   string `json:"judge_model,omitempty"`
	PromptSHA256 string `json:"prompt_sha256,omitempty"`
	// PromptFile is the candidate prompt path given via --prompt-file; absent
	// when the live prompt was evaluated.
	PromptFile string `json:"prompt_file,omitempty"`
	// Sandbox records whether the run was containerised (--sandbox). It is a
	// pointer so a result written before sandbox-mode tracking (legacy, field
	// absent) is distinguishable (nil) from an explicitly native run (false):
	// resume refuses a mode change, and refuses a legacy file with cases
	// outright since its mode is unknown. Resuming with the wrong mode would
	// score requires_sandbox cases under a different execution model and mark
	// them completed, so a correct later resume would skip them.
	Sandbox *bool `json:"sandbox,omitempty"`
	// Containment records what contained this agent's cases. It is set for a
	// container run and nil for a native run (nothing is contained), so
	// native files keep their previous shape.
	Containment *Containment `json:"containment,omitempty"`
	// ResourcesPresent lists the config resources that existed when the hash
	// was computed. It is not omitempty: a resolved-but-empty list serialises
	// as [] so that "the missing resource was omitted" is observable.
	ResourcesPresent []string     `json:"resources_present"`
	Cases            []CaseResult `json:"cases"`
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
	Backend       string            // Inference backend name (default "kiro-cli")
	EvalsDir      string            // Evals directory (default ".kairon/evals")
	Perf          bool              // Run performance investigation
	PromptFile    string            // Candidate prompt file to evaluate instead of the live prompt (requires an agent)

	// KeepWorkspaces keeps per-case workspaces after the run instead of
	// removing them.
	KeepWorkspaces bool
}

// Summary holds aggregate results for an eval run.
type Summary struct {
	GitHash     string             `json:"git_hash"`
	TotalCost   CostInfo           `json:"total_cost"`
	AgentScores map[string]float64 `json:"agent_scores"` // agent -> average score

	// Provenance. JudgeModel and Agents are always set for a pinned run; the
	// top-level agent fields are populated only when the run covers exactly
	// one agent (len(Agents) == 1), since several agents would make a single
	// value ambiguous.
	JudgeModel       string                     `json:"judge_model,omitempty"`
	AgentModel       string                     `json:"agent_model,omitempty"`
	PromptSHA256     string                     `json:"prompt_sha256,omitempty"`
	PromptFile       string                     `json:"prompt_file,omitempty"`
	ResourcesPresent []string                   `json:"resources_present,omitempty"`
	Agents           map[string]AgentProvenance `json:"agents,omitempty"`

	// Sandbox is the execution mode of the whole run: "native" or "container".
	// <agent>.json spells the same fact as a boolean (AgentResult.Sandbox,
	// true = container). Empty for a run that predates mode tracking.
	Sandbox string `json:"sandbox,omitempty"`
	// Containment is keyed by agent, like Agents, because the tool-trust set
	// is per agent. Absent for native runs.
	Containment map[string]Containment `json:"containment,omitempty"`
}

// RunMode is the execution mode of an eval run, as recorded in summary.json.
type RunMode string

const (
	// RunModeNative runs the agent directly on the host.
	RunModeNative RunMode = "native"
	// RunModeContainer runs the agent in the sandbox container (--sandbox).
	RunModeContainer RunMode = "container"
)

// runModeOf returns the mode for a sandbox flag.
func runModeOf(sandbox bool) RunMode {
	if sandbox {
		return RunModeContainer
	}
	return RunModeNative
}

// Containment summarises what contained a container run of one agent.
type Containment struct {
	// ToolTrust is the normalised --trust-tools name set. [] means trust
	// nothing, so it is deliberately not omitempty.
	ToolTrust  []string `json:"tool_trust"`
	FakeGH     bool     `json:"fake_gh"`
	ReadOnlyFS bool     `json:"read_only_fs"`
	// Network is always "unrestricted": Kairon makes no network containment
	// guarantee. It is not the container runtime's NetworkMode.
	Network string `json:"network"`
}

// MarshalJSON emits resources_present whenever the top-level agent fields are
// populated (single-agent run), including as [] when the list is empty, and
// omits it otherwise. A plain omitempty tag would drop the empty list, hiding
// "every declared resource was missing".
func (s Summary) MarshalJSON() ([]byte, error) {
	type plain Summary
	aux := struct {
		plain
		ResourcesPresent *[]string `json:"resources_present,omitempty"`
	}{plain: plain(s)}
	if s.PromptSHA256 != "" {
		rp := s.ResourcesPresent
		if rp == nil {
			rp = []string{}
		}
		aux.ResourcesPresent = &rp
	}
	return json.Marshal(aux)
}

// AgentProvenance is the per-agent provenance recorded in Summary.Agents.
type AgentProvenance struct {
	AgentModel       string   `json:"agent_model"`
	PromptSHA256     string   `json:"prompt_sha256"`
	PromptFile       string   `json:"prompt_file,omitempty"`
	ResourcesPresent []string `json:"resources_present"`
}

// ContainerConfig configures containerized execution
type ContainerConfig struct {
	Platform        string                 `json:"platform"`
	ResourceLimits  sandbox.ResourceLimits `json:"resource_limits"`
	Environment     map[string]string      `json:"environment"`
	WorkspaceDir    string                 `json:"workspace_dir"`
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
