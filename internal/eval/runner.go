package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/jbrinkman/kairon/internal/inference"
	"gopkg.in/yaml.v3"
)

// ansiRegex matches all CSI (Control Sequence Introducer) escape sequences.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// checkDockerAvailability verifies a container daemon (Podman or Docker) is
// running and accessible. It delegates to sandbox.EnsureContainerDaemon so the
// eval runner and the sandbox constructors share one Podman-aware detection
// path and the same user-facing error message.
func checkDockerAvailability() error {
	if err := sandbox.EnsureContainerDaemon(); err != nil {
		return sandbox.DaemonNotRunningError(err)
	}
	return nil
}

// RunWithOptions executes evaluation with extended CLI options.
func RunWithOptions(agent string, testcase string, options RunOptions) error {
	// Apply backend / evals-dir configuration before doing any work so that
	// an unknown backend is rejected up front.
	if err := configure(options); err != nil {
		return err
	}

	// Handle cleanup operation early
	if options.Cleanup {
		return RunCleanup()
	}

	// Pre-flight: pin and validate the judge and agent models before any
	// case (or kiro-cli call) starts.
	if !options.List {
		// The overlay is always visible to the agent: a native run uses the
		// staged <evals-dir>/agents directly, and a --sandbox run now stages
		// it into the per-case workspace's .kiro, which is bind-mounted into
		// the container. So provenance must consider the overlay on every
		// path (there is no longer a kiro-cli container that cannot see it).
		if err := pinRun(agent, options, false); err != nil {
			return err
		}
	}

	// Handle performance investigation
	if options.Perf {
		return RunPerformanceInvestigation(agent)
	}

	// Configure container sandboxing. The single-case/resume paths below
	// build and then ignore cConfig, so this guard is broader than the set of
	// runs that actually containerise.
	var cConfig *ContainerConfig
	if options.Sandbox && !options.NoSandbox {
		// Early Docker availability check before any configuration work
		if err := checkDockerAvailability(); err != nil {
			return err
		}
		var sandboxCfg *config.SandboxConfig
		if cfg, err := config.Load(); err == nil {
			sandboxCfg = &cfg.Sandbox
		}
		cConfig = createContainerConfig(sandboxCfg, options.ResourceLimit, options.Debug)
	}

	// Start performance profiling
	StartProfiling()
	defer func() {
		profile := GenerateProfile()
		fmt.Println() // Add spacing before performance report
		PrintPerformanceReport(profile, nil)
	}()

	// Handle list command
	if options.List {
		return listTestCases(agent)
	}

	// Handle specific test case execution
	if testcase != "" {
		return runSingleTestCase(agent, testcase)
	}

	// Handle resume
	if options.Resume {
		return runWithResume(agent)
	}

	// Default to original behavior for backward compatibility
	return Run(agent, cConfig)
}

// RunCleanup stops and removes all tracked debug containers and cleans artifacts
func RunCleanup() error {
	fmt.Println("🧹 Starting cleanup of debug containers and artifacts...")

	// Initialize registry to get tracked containers
	registry, err := sandbox.NewRegistry()
	if err != nil {
		return fmt.Errorf("failed to access container registry: %w", err)
	}

	containers := registry.List()
	if len(containers) == 0 {
		fmt.Println("✅ No debug containers found to cleanup")
		return nil
	}

	fmt.Printf("📋 Found %d tracked containers to cleanup\n", len(containers))

	// Connect to Docker
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Cleanup containers
	cleaned := 0
	for _, entry := range containers {
		shortID := entry.ContainerID
		if len(shortID) > 7 {
			shortID = shortID[:7]
		}

		fmt.Printf("🐳 Stopping container %s (%s)... ", shortID, entry.Name)

		// Stop container
		timeout := 10 * time.Second
		timeoutSec := int(timeout.Seconds())
		if err := cli.ContainerStop(ctx, entry.ContainerID, container.StopOptions{Timeout: &timeoutSec}); err != nil {
			fmt.Printf("⚠️ Stop failed: %v\n", err)
		}

		// Remove container
		if err := cli.ContainerRemove(ctx, entry.ContainerID, container.RemoveOptions{Force: true}); err != nil {
			fmt.Printf("⚠️ Remove failed: %v\n", err)
		} else {
			fmt.Println("✅ Removed")
			cleaned++
		}

		// Remove from registry
		registry.Remove(entry.ContainerID)
	}

	// Clear the registry
	if err := registry.Clear(); err != nil {
		fmt.Printf("⚠️ Warning: Failed to clear registry: %v\n", err)
	}

	fmt.Printf("✅ Cleanup complete: %d/%d containers removed\n", cleaned, len(containers))

	// Prompt about Dockerfile preservation
	dockerfileDir := filepath.Join(".kairon", "evals", "tmp", "dockerfiles")
	if entries, err := os.ReadDir(dockerfileDir); err == nil && len(entries) > 0 {
		fmt.Printf("📁 Found %d preserved Dockerfiles in %s\n", len(entries), dockerfileDir)
		fmt.Println("💡 These are preserved for debugging. Remove manually if no longer needed.")
	}

	return nil
}

// listTestCases displays available test cases for an agent.
func listTestCases(agent string) error {
	if agent == "" {
		return fmt.Errorf("❌ agent name required for --list")
	}

	return PrintTestCaseList(agent)
}

// runSingleTestCase executes a single test case for an agent.
func runSingleTestCase(agent string, testcase string) error {
	// Start performance profiling for single test
	StartProfiling()
	startupTime := MeasureStartupOverhead()

	if agent == "" {
		return fmt.Errorf("❌ agent name required for test case execution")
	}

	// Validate test case exists using selective module
	if err := ValidateTestCase(agent, testcase); err != nil {
		return fmt.Errorf("❌ %w", err)
	}

	// Get the test case
	targetCase, err := GetTestCase(agent, testcase)
	if err != nil {
		return fmt.Errorf("❌ %w", err)
	}

	fmt.Printf("🚀 Running single test case: %s/%s\n", agent, testcase)
	fmt.Printf("📊 Startup overhead: %v\n", startupTime)

	// Load rubric for the agent
	rubrics, err := loadRubrics(agent)
	if err != nil {
		return fmt.Errorf("❌ failed to load rubrics for %s: %w", agent, err)
	}

	var rubric *Rubric
	for _, r := range rubrics {
		if r.Agent == agent {
			rubric = &r
			break
		}
	}

	if rubric == nil {
		return fmt.Errorf("❌ rubric not found for agent '%s'", agent)
	}

	// Setup results directory
	gitHash, err := getGitShortHash()
	if err != nil {
		return fmt.Errorf("failed to get git hash: %w", err)
	}

	timestamp := generateTimestampPrefix()
	resultsDir := evalsPath("results", timestamp)
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}

	// Run evaluation on single test case (without container config for now)
	result := evaluate(*rubric, []TestCase{*targetCase}, gitHash, os.Stdout, nil)

	// Write result file
	resultFile := filepath.Join(resultsDir, agent+".json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := os.WriteFile(resultFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write result file: %w", err)
	}

	// Single-case runs also get a summary.json carrying the provenance fields.
	if err := updateIncrementalSummary(filepath.Join(resultsDir, "summary.json"), result, gitHash); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	// Generate and display performance analysis for single test
	profile := GenerateProfile()

	// Save performance report
	if perfErr := SavePerformanceReport(resultsDir, profile, nil); perfErr != nil {
		fmt.Printf("⚠️  Failed to save performance report: %v\n", perfErr)
	}

	fmt.Printf("📂 Results: %s\n", resultsDir)

	// Display concise performance summary for single test
	fmt.Printf("\n📊 Performance Summary:\n")
	fmt.Printf("  Test execution: %v\n", profile.TestCaseTimings[targetCase.Name])
	fmt.Printf("  Startup overhead: %v\n", profile.StartupOverhead)
	if len(profile.Bottlenecks) > 0 {
		fmt.Printf("  Bottlenecks: %d identified (see performance.json)\n", len(profile.Bottlenecks))
	}

	return nil
}

// Run executes the evaluation for all agents (or a specific agent) and writes results.
func Run(agent string, cConfig *ContainerConfig) error {
	// Start performance profiling
	StartProfiling()

	fmt.Println("🚀 Starting evaluation framework...")

	// Obtain the tools-only base image once per run. It is cached by content
	// hash and persistent: nothing builds or removes an image per call.
	if cConfig != nil {
		imageManager, err := sandbox.NewImageManager("", cConfig.Debug)
		if err != nil {
			return fmt.Errorf("failed to create image manager: %w", err)
		}
		defer imageManager.Close()

		tag, built, err := imageManager.EnsureBaseImage(context.Background(), cConfig.Platform)
		if err != nil {
			return fmt.Errorf("preparing base image: %w", err)
		}
		if built {
			fmt.Printf("🔨 Base image built: %s\n", tag)
		} else {
			fmt.Printf("✅ Base image reused: %s\n", tag)
		}

		// Add image manager and cached image name to config for test cases
		cConfig.ImageManager = imageManager
		cConfig.CachedImageName = tag
	}

	// Measure startup overhead
	fmt.Printf("📊 Measuring %s startup overhead...", cfg.backend.Name())
	startupTime := MeasureStartupOverhead()
	fmt.Printf(" %v\n", startupTime)

	// Task 2: Validate rubrics directory exists
	rubricsDir := evalsPath("rubrics")
	if _, err := os.Stat(rubricsDir); os.IsNotExist(err) {
		return fmt.Errorf("❌ Fatal: rubrics directory not found at %s", rubricsDir)
	}

	// Task 2: Check backend availability
	if err := cfg.backend.Available(); err != nil {
		return fmt.Errorf("❌ Fatal: %s unavailable: %w", cfg.backend.Name(), err)
	}

	gitHash, err := getGitShortHash()
	if err != nil {
		return fmt.Errorf("failed to get git hash: %w", err)
	}

	rubrics, err := loadRubrics(agent)
	if err != nil {
		return err
	}

	if len(rubrics) == 0 {
		return fmt.Errorf("❌ Fatal: no rubrics found in %s/", evalsPath("rubrics"))
	}

	timestamp := generateTimestampPrefix()
	resultsDir := evalsPath("results", timestamp+"-"+gitHash)
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}

	// Create progress file to enable resumption
	progressFile := filepath.Join(resultsDir, ".progress")
	if err := os.WriteFile(progressFile, []byte("in-progress"), 0644); err != nil {
		return fmt.Errorf("failed to create progress file: %w", err)
	}

	// Use progressive evaluation
	err = runProgressiveEvaluation(agent, resultsDir, false, cConfig)

	// Generate performance analysis
	profile := GenerateProfile()

	// Save performance report
	if perfErr := SavePerformanceReport(resultsDir, profile, nil); perfErr != nil {
		fmt.Printf("⚠️  Failed to save performance report: %v\n", perfErr)
	}

	// Display performance analysis
	PrintPerformanceReport(profile, nil)

	return err
}

func evaluate(rubric Rubric, cases []TestCase, gitHash string, out io.Writer, cConfig *ContainerConfig) AgentResult {
	result := AgentResult{
		Agent:   rubric.Agent,
		GitHash: gitHash,
	}
	cfg.pins.applyTo(&result)

	for i, tc := range cases {
		fmt.Fprintf(out, "   [%d/%d] %s", i+1, len(cases), tc.Name)

		testStart := time.Now()

		// Task 4: Structured output - Agent → Case execution (workspace, agent
		// call and criterion-by-criterion scoring in one shared path).
		cr := executeCase(rubric, tc, cConfig, out, cfg.keepWorkspaces)

		// Task 4: Color-coded final status for the case
		printCaseResult(out, tc, cr)

		// Track test case completion for performance profiling
		testDuration := time.Since(testStart)
		TrackTestCase(tc.Name, testDuration)

		result.Cases = append(result.Cases, cr)
	}

	return result
}

// assemblePrompt combines setup entries and input into a complete agent prompt
func assemblePrompt(setup []SetupEntry, input string) (string, error) {
	var parts []string

	for _, entry := range setup {
		switch entry.Type {
		case "text":
			if entry.Label != "" {
				parts = append(parts, fmt.Sprintf("=== %s ===\n%s", entry.Label, entry.Content))
			} else {
				parts = append(parts, entry.Content)
			}
		case "file":
			if entry.Path == "" {
				return "", fmt.Errorf("setup entry '%s' has type 'file' but no path", entry.Label)
			}
			path := rebaseEvalsPath(entry.Path)
			content, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("failed to read setup file %s: %w", path, err)
			}
			label := entry.Label
			if label == "" {
				label = entry.Path
			}
			parts = append(parts, fmt.Sprintf("=== %s ===\n%s", label, string(content)))
		case "url":
			return "", fmt.Errorf("url setup entries not yet supported")
		default:
			return "", fmt.Errorf("unknown setup entry type: %s", entry.Type)
		}
	}

	if len(parts) > 0 {
		parts = append(parts, "=== Task ===")
	}
	parts = append(parts, input)

	return strings.Join(parts, "\n\n"), nil
}

// invokeAgent runs the agent with the given prompt, with timeout support.
// Both paths share one inference.Request (newAgentRequest) and one completion
// (completeAgentCall); they differ only in where the backend process runs:
// in-process through the configured backend, or inside a container when a
// container config is given. opts.Stub is the case's scripted response, used
// only by the stub backend.
//
// opts.Workspace, when set, is the native agent's working directory.
// opts.Timeout, when set, overrides the default timeout (native) and the
// sandbox resource-limit timeout (container).
//
// The returned CallRecord describes the call (also when it failed).
func invokeAgent(agent, prompt string, cConfig *ContainerConfig, opts callOpts) (string, CostInfo, inference.CallRecord, *ErrorContext, error) {
	req := newAgentRequest(agent, prompt, opts.Stub)

	if cConfig != nil {
		// The container exec is bounded by the sandbox timeout, and the
		// in-container backend must honour the same bound.
		if cConfig.ResourceLimits.Timeout > 0 {
			req.Timeout = cConfig.ResourceLimits.Timeout
		}
		// The agent config directory is a host path; nothing in the
		// container can read it.
		req.AgentConfigDir = ""
		// The case's own timeout wins over the sandbox default.
		if opts.Timeout > 0 {
			req.Timeout = opts.Timeout
		}
		// The agent works in the container-side workspace path (the bind
		// mount of the host workspace), not a host path.
		req.WorkDir = cConfig.WorkspaceDir

		start := time.Now()
		resp, baseEC, err := invokeInContainer(req, cConfig, opts.Workspace)
		return completeAgentCall(req, resp, err, time.Since(start), baseEC)
	}

	// The case's own timeout wins over KAIRON_EVAL_TIMEOUT and the default.
	if opts.Timeout > 0 {
		req.Timeout = opts.Timeout
	}

	// Capture working directory and relevant environment for error context.
	workingDir, _ := os.Getwd()
	if opts.Workspace != nil {
		// The agent runs in the case workspace, not the process cwd. The
		// staged .kiro/agents there already holds the evals-dir overlay, so
		// the host agent-config path is not needed.
		req.WorkDir = opts.Workspace.Dir
		req.AgentConfigDir = ""
		workingDir = opts.Workspace.Dir
	}
	envVars := make(map[string]string)
	for _, key := range []string{"KAIRON_EVAL_TIMEOUT"} {
		if val := os.Getenv(key); val != "" {
			envVars[key] = val
		}
	}

	start := time.Now()
	resp, err := cfg.backend.Invoke(context.Background(), req)
	return completeAgentCall(req, resp, err, time.Since(start), &ErrorContext{WorkingDir: workingDir, Environment: envVars})
}

// newAgentRequest builds the inference request for one agent call. It is
// shared by the native and container paths so both send the same model,
// timeout and stub script.
func newAgentRequest(agent, prompt string, stub *inference.StubScript) inference.Request {
	timeoutStr := os.Getenv("KAIRON_EVAL_TIMEOUT")
	timeout := 2 * time.Minute
	if timeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsedTimeout
		}
	}

	return inference.Request{
		Role:           inference.RoleAgent,
		Agent:          agent,
		Prompt:         prompt,
		Timeout:        timeout,
		AgentConfigDir: agentConfigDir(agent),
		Model:          cfg.pins.agentModel(agent),
		Stub:           stub,
		// Turn is left 0: multi-turn stub selection lands with E9 (multi-turn
		// cases). StubScript.Turns is a slice for that future, not yet wired.
	}
}

// completeAgentCall turns a backend result into invokeAgent's return values:
// cost from the reported/estimated usage, the audit CallRecord, and the
// ErrorContext. baseEC carries transport-specific context (working directory
// natively; container id/image in a container) and may be nil; the fields the
// backend reported (command, stderr, exit code) are filled into it.
func completeAgentCall(req inference.Request, resp inference.Response, err error, wall time.Duration, baseEC *ErrorContext) (string, CostInfo, inference.CallRecord, *ErrorContext, error) {
	cost := costFromUsage(resp.Model, resp.Usage)
	rec := newCallRecord(req, resp, err, wall, cost)
	rec.PromptSHA256 = cfg.pins.agentPromptSHA(req.Agent)

	if resp.Duration > 30*time.Second {
		fmt.Printf(" (>30s)")
	}

	// Create error context for any execution issues
	var errorContext *ErrorContext
	if err != nil || len(resp.Stderr) > 0 {
		errorContext = baseEC
		if errorContext == nil {
			errorContext = &ErrorContext{}
		}
		if errorContext.Command == "" {
			errorContext.Command = resp.Command
		}
		if errorContext.Stderr == "" {
			errorContext.Stderr = resp.Stderr
		}
		if errorContext.ExitCode == 0 {
			errorContext.ExitCode = resp.ExitCode
		}
	}

	if err != nil {
		if errors.Is(err, inference.ErrTimeout) && errorContext != nil {
			errorContext.Stderr = fmt.Sprintf("timeout after %v\n%s", req.Timeout, errorContext.Stderr)
		}
		return "", CostInfo{}, rec, errorContext, err
	}

	return resp.Text, cost, rec, errorContext, nil
}

// createContainerConfig builds container configuration from CLI options. It
// is backend-agnostic: which backend runs inside the container is decided by
// cfg.backend (see runAgentInContainer).
func createContainerConfig(sandboxCfg *config.SandboxConfig, resourceLimits map[string]string, debug bool) *ContainerConfig {
	// Detect host architecture for platform-aware container creation
	platform, err := sandbox.DetectHostArchitecture()
	if err != nil {
		// Fallback to amd64 if detection fails
		platform = "linux/amd64"
	}

	// Use config values with fallbacks to defaults
	workspaceDir := "/workspace"
	cpuCores := 1.0
	memoryMB := 1024
	timeout := 5 * time.Minute

	if sandboxCfg != nil {
		if sandboxCfg.WorkspaceDir != "" {
			workspaceDir = sandboxCfg.WorkspaceDir
		}
		if sandboxCfg.CPUCores > 0 {
			cpuCores = sandboxCfg.CPUCores
		}
		if sandboxCfg.MemoryMB > 0 {
			memoryMB = sandboxCfg.MemoryMB
		}
		if sandboxCfg.Timeout > 0 {
			timeout = sandboxCfg.Timeout
		}
	}

	config := &ContainerConfig{
		WorkspaceDir: workspaceDir,
		MockGitHub:   true,
		Platform:     platform,
		Debug:        debug,
		ImageManager: nil,
		Environment: map[string]string{
			"KIRO_CLI_DISABLE_TELEMETRY": "1",
		},
		ResourceLimits: sandbox.ResourceLimits{
			CPUQuota: int64(cpuCores * 1000000),     // Convert cores to microseconds
			Memory:   int64(memoryMB * 1024 * 1024), // Convert MB to bytes
			Timeout:  timeout,
		},
	}

	// Apply resource limit overrides (only positive values accepted)
	if resourceLimits != nil {
		if cpu := resourceLimits["cpu"]; cpu != "" {
			if cpuFloat, err := strconv.ParseFloat(cpu, 64); err == nil && cpuFloat > 0 {
				config.ResourceLimits.CPUQuota = int64(cpuFloat * 1000000)
			}
		}
		if memory := resourceLimits["memory"]; memory != "" {
			if memInt, err := strconv.ParseInt(memory, 10, 64); err == nil && memInt >= 256*1024*1024 {
				config.ResourceLimits.Memory = memInt
			}
		}
		if timeout := resourceLimits["timeout"]; timeout != "" {
			if timeoutDur, err := time.ParseDuration(timeout); err == nil && timeoutDur > 0 {
				config.ResourceLimits.Timeout = timeoutDur
			}
		}
	}

	return config
}

// containerHelperPath is where the kairon helper binary is bind-mounted
// (read-only) in the container for non-kiro-cli backends. Nothing is copied
// into the running container.
const containerHelperPath = "/opt/kairon/kairon"

// containerUser is the unprivileged user every container runs as.
const containerUser = "sandbox"

// containerHome is the sandbox user's home directory in the base image.
const containerHome = "/home/sandbox"

// helperExecGrace is added to the host exec deadline for helper backends, so
// the in-container backend normally reports its own timeout (an
// ErrTimeout-wrapped envelope) and the host deadline is only a backstop.
const helperExecGrace = 10 * time.Second

// buildContainerMounts returns the bind mounts for one case: the staged
// .kiro read-only, the workspace and its .eval/ read-write, and, for
// non-kiro-cli backends, the kairon helper read-only. There is deliberately
// no tmpfs at the workspace path.
func buildContainerMounts(ws *caseWorkspace, cConfig *ContainerConfig, backendName string) ([]sandbox.Mount, error) {
	if ws == nil {
		return nil, fmt.Errorf("container run needs a case workspace")
	}
	dir := cConfig.WorkspaceDir
	mounts := []sandbox.Mount{
		{HostPath: ws.KiroDir, ContainerPath: path.Join(dir, ".kiro"), ReadOnly: true},
		{HostPath: ws.Dir, ContainerPath: dir},
		{HostPath: ws.EvalDir, ContainerPath: path.Join(dir, ".eval")},
	}
	if backendName != inference.NameKiroCLI {
		bin, err := resolveLinuxBinary(cConfig.Platform)
		if err != nil {
			return nil, fmt.Errorf("preparing %s helper for the container: %w", backendName, err)
		}
		info, err := os.Stat(bin)
		if err != nil {
			return nil, fmt.Errorf("preparing %s helper for the container: %w", backendName, err)
		}
		// The sandbox user (uid 1000) is not the owner: it needs r-x for others.
		if info.Mode().Perm()&0o005 != 0o005 {
			return nil, fmt.Errorf("%s helper %s (mode %v) is not readable and executable by the container user; chmod 755 it", backendName, bin, info.Mode().Perm())
		}
		mounts = append(mounts, sandbox.Mount{HostPath: bin, ContainerPath: containerHelperPath, ReadOnly: true})
	}
	return mounts, nil
}

// containerEnv returns the container environment: the configured variables
// (sorted for determinism), then HOME and the git safe.directory setting. The
// latter is required because the mounted repository is owned by a different
// uid than the sandbox user, and git refuses it ("dubious ownership")
// otherwise. It is environment, not a file operation.
func containerEnv(cConfig *ContainerConfig) []string {
	env := []string{"KIRO_CLI_DISABLE_TELEMETRY=1"}
	keys := make([]string, 0, len(cConfig.Environment))
	for k := range cConfig.Environment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, fmt.Sprintf("%s=%s", k, cConfig.Environment[k]))
	}
	return append(env,
		"HOME="+containerHome,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*",
	)
}

// newContainerConfig is the container.Config for one case.
func newContainerConfig(image string, cConfig *ContainerConfig) *container.Config {
	return &container.Config{
		Image:      image,
		Cmd:        []string{"sleep", "3600"},
		Env:        containerEnv(cConfig),
		User:       containerUser,
		WorkingDir: cConfig.WorkspaceDir,
	}
}

// ensureBaseImageName returns the cached base image tag, resolving it through
// the (cached) EnsureBaseImage when Run() did not.
func ensureBaseImageName(ctx context.Context, cConfig *ContainerConfig) (string, error) {
	if cConfig.CachedImageName != "" {
		return cConfig.CachedImageName, nil
	}
	im, err := sandbox.NewImageManager("", cConfig.Debug)
	if err != nil {
		return "", fmt.Errorf("creating image manager: %w", err)
	}
	defer im.Close()
	tag, _, err := im.EnsureBaseImage(ctx, cConfig.Platform)
	if err != nil {
		return "", fmt.Errorf("preparing base image: %w", err)
	}
	return tag, nil
}

// invokeAgentInContainer runs one agent request inside a container created
// from the cached base image. Everything the agent sees is bind-mounted from
// the host-built case workspace; nothing is copied or installed in the running
// container. The container is only a transport: the backend selected by
// cfg.backend runs inside it (see runAgentInContainer) and the result is
// completed by the same logic as a native call.
func invokeAgentInContainer(req inference.Request, cConfig *ContainerConfig, ws *caseWorkspace) (inference.Response, *ErrorContext, error) {
	ctx := context.Background()
	backendName := cfg.backend.Name()

	mounts, err := buildContainerMounts(ws, cConfig, backendName)
	if err != nil {
		return inference.Response{}, nil, err
	}
	hostConfig, err := sandbox.NewHostConfigWithMounts(cConfig.ResourceLimits, mounts)
	if err != nil {
		return inference.Response{}, nil, fmt.Errorf("preparing container mounts: %w", err)
	}

	c, err := sandbox.NewContainerWithDebug("", cConfig.Debug)
	if err != nil {
		return inference.Response{}, nil, fmt.Errorf("creating container: %w", err)
	}
	defer c.Close()

	imageName, err := ensureBaseImageName(ctx, cConfig)
	if err != nil {
		return inference.Response{}, nil, err
	}

	createStart := time.Now()
	if err := c.CreateWithPlatform(ctx, newContainerConfig(imageName, cConfig), hostConfig, cConfig.Platform); err != nil {
		return inference.Response{}, nil, fmt.Errorf("creating container: %w", err)
	}
	containerFailed := false
	defer func() {
		c.CleanupWithDebugInfo(ctx, containerFailed)
	}()

	if err := c.Start(ctx); err != nil {
		return inference.Response{}, nil, fmt.Errorf("starting container: %w", err)
	}

	c.LogStartup(cConfig.ResourceLimits)
	fmt.Printf("  Container startup: %v\n", time.Since(createStart))

	// Debug mode: Save container registry information
	if cConfig.Debug {
		shortID, name := c.GetContainerInfo()
		if err := saveDebugContainerInfo(shortID, name, imageName, cConfig.Platform, req.Agent); err != nil {
			fmt.Printf("⚠️ Warning: Failed to save debug container info: %v\n", err)
		}
	}

	// A read-only check that the baked kiro-cli works (not an install).
	setupStart := time.Now()
	if backendName == inference.NameKiroCLI {
		if err := c.ValidateKiroCLI(ctx, cConfig.Platform); err != nil {
			return inference.Response{}, nil, fmt.Errorf("validating kiro-cli: %w", err)
		}
	}
	fmt.Printf("  Container setup: %v\n", time.Since(setupStart))

	executionStart := time.Now()
	resp, ec, err := runAgentInContainer(ctx, c, req, cConfig)
	executionDuration := time.Since(executionStart)

	if err != nil {
		// In debug mode, preserve the failed container
		if cConfig.Debug {
			containerFailed = true
		}
		return resp, ec, err
	}

	fmt.Printf("  Execution time: %v\n", executionDuration)
	return resp, ec, nil
}

// invokeInContainer is a seam so tests can run invokeAgent's container branch
// against a fake executor instead of a real container.
var invokeInContainer = invokeAgentInContainer

// agentExecer is the part of *sandbox.Container that runAgentInContainer
// needs. It is a seam so the container path is unit-testable without a
// container daemon. It deliberately has no way to copy files in.
type agentExecer interface {
	ExecWithStdin(ctx context.Context, cmd []string, stdin io.Reader) (sandbox.ExecResult, error)
}

// resolveLinuxBinary is a seam for tests; production uses sandbox.ResolveLinuxBinary.
var resolveLinuxBinary = sandbox.ResolveLinuxBinary

// containerError is an error with a fixed user-facing message that unwraps to
// sentinel errors (so a timeout still satisfies errors.Is(err, inference.ErrTimeout)).
type containerError struct {
	msg  string
	errs []error
}

func (e *containerError) Error() string   { return e.msg }
func (e *containerError) Unwrap() []error { return e.errs }

// runAgentInContainer executes one inference request inside the container x.
// The prompt is always delivered on stdin, never on argv.
//
//   - kiro-cli runs directly with the same argv as the native backend
//     (inference.KiroCLIAgentCommand) and its output is decoded by the same
//     function (inference.KiroCLIAgentResponse).
//   - every other backend runs in-process in the container through
//     "kairon inference-exec --backend <name>", fed the JSON request.
//
// The returned ErrorContext (non-nil on failure) holds container details.
func runAgentInContainer(ctx context.Context, x agentExecer, req inference.Request, cConfig *ContainerConfig) (inference.Response, *ErrorContext, error) {
	backendName := cfg.backend.Name()
	if cConfig.Debug {
		fmt.Printf("🔧 Debug: container invoke backend=%s agent=%s model=%s\n", backendName, req.Agent, req.Model)
	}

	// req.Timeout is the effective timeout (case timeout, else the sandbox
	// resource limit); invokeAgent has already resolved it.
	timeout := timeoutOrDefaultRequest(req)
	req.Timeout = timeout

	var (
		cmd     []string
		command string
		stdin   string
		// execTimeout is the host exec deadline: the effective timeout for
		// kiro-cli, plus a grace for helper backends so the in-container
		// backend normally reports the timeout itself.
		execTimeout = timeout
	)
	if backendName == inference.NameKiroCLI {
		args, c := inference.KiroCLIAgentCommand(req)
		cmd = append([]string{"kiro-cli"}, args...)
		command = c
		stdin = req.Prompt
	} else {
		payload, err := json.Marshal(req)
		if err != nil {
			return inference.Response{}, nil, fmt.Errorf("encoding inference request: %w", err)
		}
		cmd = []string{containerHelperPath, "inference-exec", "--backend", backendName}
		command = strings.Join(cmd, " ")
		stdin = string(payload)
		execTimeout = timeout + helperExecGrace
	}

	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	start := time.Now()
	res, execErr := x.ExecWithStdin(execCtx, sandbox.WithOpenUmask(cmd), strings.NewReader(stdin))
	elapsed := time.Since(start)

	resp := inference.Response{Command: command, Model: req.Model, Duration: elapsed}

	// Container details for the error context.
	var shortID, imageName string
	if info, ok := x.(interface{ GetContainerInfo() (string, string) }); ok {
		shortID, imageName = info.GetContainerInfo()
	}
	newEC := func(stderr string) *ErrorContext {
		return &ErrorContext{
			Command:        strings.Join(cmd, " "),
			WorkingDir:     cConfig.WorkspaceDir,
			Environment:    cConfig.Environment,
			Stderr:         stderr,
			ContainerID:    shortID,
			ContainerImage: imageName,
			Platform:       cConfig.Platform,
			DockerError:    stderr,
		}
	}

	// Transport failure (exec could not run, or the context ended).
	if execErr != nil {
		ec := newEC(execErr.Error())
		return resp, ec, mapContainerError(execErr, execCtx, imageName, cConfig, timeout, fmt.Errorf("container execution failed: %w", execErr))
	}

	resp.ExitCode = res.ExitCode
	if res.ExitCode != 0 {
		resp.Stderr = res.Stderr
		detail := fmt.Errorf("command failed with exit code %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
		var fallback error
		if backendName == inference.NameKiroCLI {
			fallback = fmt.Errorf("kiro-cli invocation failed: exit status %d", res.ExitCode)
		} else {
			fallback = fmt.Errorf("%s helper failed: exit status %d: %s", backendName, res.ExitCode, strings.TrimSpace(res.Stderr))
		}
		ec := newEC(detail.Error())
		ec.Stderr = res.Stderr
		return resp, ec, mapContainerError(detail, execCtx, imageName, cConfig, timeout, fallback)
	}

	if backendName == inference.NameKiroCLI {
		return inference.KiroCLIAgentResponse(req, res.Stdout, res.Stderr, 0, elapsed), nil, nil
	}

	out, decodeErr := inference.DecodeExecResult([]byte(res.Stdout))
	if decodeErr == nil {
		return out, nil, nil
	}

	if strings.HasPrefix(decodeErr.Error(), "decoding inference result") {
		// The helper did not produce a readable envelope.
		resp.Stderr = res.Stderr
		return resp, newEC(res.Stderr), fmt.Errorf("%s helper returned an unreadable result: %w", backendName, decodeErr)
	}

	// The in-container backend itself failed. decodeErr already carries the
	// backend-authored, user-facing message, so it is returned as-is rather
	// than through mapContainerError: the OOM/image-pull remapping applies to
	// transport failures, not to a backend that ran and reported an error, and
	// a reported timeout is already ErrTimeout-wrapped by DecodeExecResult.
	if out.Command == "" {
		out.Command = command
	}
	if out.Stderr == "" {
		out.Stderr = res.Stderr
	}
	return out, newEC(out.Stderr), decodeErr
}

// timeoutOrDefaultRequest returns req.Timeout, or the inference default when unset.
func timeoutOrDefaultRequest(req inference.Request) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}
	return inference.DefaultTimeout
}

// mapContainerError converts a container failure into the actionable
// user-facing message for the common cases (timeout, OOM, image pull) and
// falls back to the supplied error otherwise. timeout is the effective
// timeout of the call (the case timeout, else the sandbox limit), which is
// what the message names. Timeouts also satisfy
// errors.Is(err, inference.ErrTimeout).
func mapContainerError(err error, execCtx context.Context, imageName string, cConfig *ContainerConfig, timeout time.Duration, fallback error) error {
	msg := err.Error()
	if strings.Contains(msg, "timeout") || execCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
		return &containerError{
			msg:  fmt.Sprintf("⏱️ Container execution timeout after %v. Consider increasing the case timeout or --resource-limit timeout=", timeout),
			errs: []error{inference.ErrTimeout, err},
		}
	}
	if strings.Contains(msg, "out of memory") || strings.Contains(msg, "OOMKilled") {
		memoryMB := cConfig.ResourceLimits.Memory / (1024 * 1024)
		return fmt.Errorf("💾 Container ran out of memory (%dMB limit). Consider increasing --resource-limit memory=", memoryMB)
	}
	if strings.Contains(msg, "no such image") || strings.Contains(msg, "pull access denied") {
		return fmt.Errorf("❌ Failed to pull image %s: %v. Check internet connection", imageName, err)
	}
	return fallback
}

// agentConfigDir returns the evals-dir agents directory when it holds a config
// for agent, so it takes precedence over the repo's .kiro/agents. It returns ""
// otherwise, leaving backend behaviour unchanged.
func agentConfigDir(agent string) string {
	dir := evalsPath("agents")
	if _, err := os.Stat(filepath.Join(dir, agent+".json")); err != nil {
		return ""
	}
	return dir
}

func scoreDeterministic(criterion Criterion, tc TestCase, actualOutput string) (int, string, bool) {
	output := actualOutput
	if output == "" {
		return 0, "no output to evaluate", true
	}

	maxScore := parseMaxScore(criterion.Scoring)

	// Check against context facts if available
	contextViolations := 0
	if len(tc.Context) > 0 {
		lowerOutput := strings.ToLower(output)
		for _, fact := range tc.Context {
			lowerFact := strings.ToLower(fact)
			// Simple heuristic: if output contradicts a context fact
			if strings.Contains(lowerFact, "not") || strings.Contains(lowerFact, "no ") {
				// Look for positive assertions that contradict negative facts
				factWords := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(lowerFact, "not", ""), "no ", ""))
				for _, word := range factWords {
					if len(word) > 3 && strings.Contains(lowerOutput, word) {
						contextViolations++
						break
					}
				}
			}
		}
	}

	// Heuristic deterministic checks based on criterion name patterns
	switch {
	case strings.Contains(criterion.Name, "completeness"):
		// Check for required sections
		sections := []string{"## ", "### "}
		found := 0
		for _, s := range sections {
			if strings.Contains(output, s) {
				found++
			}
		}
		score := (found * maxScore) / len(sections)
		if contextViolations > 0 {
			score = max(1, score-contextViolations)
		}
		return score, fmt.Sprintf("found %d/%d expected structural elements", found, len(sections)), false

	case strings.Contains(criterion.Name, "file_reference"):
		// Extract candidate file paths and verify they exist
		lines := strings.Split(output, "\n")
		var candidates []string
		for _, word := range lines {
			for _, w := range strings.Fields(word) {
				w = strings.Trim(w, "`*_-•")
				if strings.Contains(w, "/") && (strings.HasSuffix(w, ".go") || strings.HasSuffix(w, ".ts") || strings.HasSuffix(w, ".yaml") || strings.HasSuffix(w, ".md") || strings.HasSuffix(w, ".json") || strings.HasSuffix(w, ".sh")) {
					candidates = append(candidates, w)
				}
			}
		}

		// Check against context facts for file existence
		if len(tc.Context) > 0 {
			for i, path := range candidates {
				for _, fact := range tc.Context {
					if strings.Contains(fact, path) && (strings.Contains(fact, "exists") || strings.Contains(fact, "present")) {
						candidates[i] = path + " (verified by context)"
					}
				}
			}
		}

		if len(candidates) == 0 {
			return 1, "no file references found", false
		}
		verified := 0
		for _, path := range candidates {
			cleanPath := strings.Split(path, " ")[0]
			if _, err := os.Stat(cleanPath); err == nil || strings.Contains(path, "verified by context") {
				verified++
			}
		}
		score := (verified * maxScore) / len(candidates)
		if score < 1 {
			score = 1
		}
		if contextViolations > 0 {
			score = max(1, score-contextViolations)
		}
		return score, fmt.Sprintf("%d/%d referenced files verified", verified, len(candidates)), false

	case strings.Contains(criterion.Name, "file_naming"):
		// Check documenter output references correct path format
		if strings.Contains(output, "app_docs/feature-") {
			return maxScore, "output references correct app_docs/feature-* path", false
		}
		return 1, "no app_docs/feature-* path found in output", false

	case strings.Contains(criterion.Name, "acceptance_criteria_quality"):
		// Check for testable acceptance criteria patterns
		testablePatterns := []string{"- [ ]", "- [x]", "```", "go test", "go build", "curl ", "exit code", "status code", "returns ", "outputs "}
		found := 0
		for _, pattern := range testablePatterns {
			if strings.Contains(strings.ToLower(output), pattern) {
				found++
			}
		}
		score := max(1, (found*maxScore)/3)
		if found >= 3 {
			score = maxScore
		}
		if contextViolations > 0 {
			score = max(1, score-contextViolations)
		}
		return score, fmt.Sprintf("found %d testable criteria indicators", found), false

	case strings.Contains(criterion.Name, "test_execution"):
		// Check for evidence of actual command execution and results
		executionIndicators := []string{"exit code", "$ ", "PASS", "FAIL", "ok  \t", "--- FAIL", "--- PASS", "go test", "npm test", "pytest"}
		found := 0
		for _, indicator := range executionIndicators {
			if strings.Contains(output, indicator) {
				found++
			}
		}
		score := max(1, (found*maxScore)/2)
		if found >= 2 {
			score = maxScore
		}
		if contextViolations > 0 {
			score = max(1, score-contextViolations)
		}
		return score, fmt.Sprintf("found %d execution evidence indicators", found), false

	case strings.Contains(criterion.Name, "code_correctness"):
		// Check for code compilation/execution success indicators
		lowerOutput := strings.ToLower(output)
		successIndicators := []string{"build passes", "compiled successfully", "no errors", "exit code 0", "ok  \t"}
		errorIndicators := []string{"compilation error", "syntax error", "build failed", "does not compile"}
		successCount := 0
		errorCount := 0
		for _, indicator := range successIndicators {
			if strings.Contains(lowerOutput, indicator) {
				successCount++
			}
		}
		for _, indicator := range errorIndicators {
			if strings.Contains(lowerOutput, indicator) {
				errorCount++
			}
		}

		score := maxScore / 2
		if successCount > 0 && errorCount == 0 {
			score = maxScore
		}
		if errorCount > 0 {
			score = 1
		}
		if contextViolations > 0 {
			score = max(1, score-contextViolations)
		}

		reasoning := "no clear success or error indicators"
		if successCount > 0 && errorCount == 0 {
			reasoning = "code appears to compile/run successfully"
		} else if errorCount > 0 {
			reasoning = "compilation or runtime errors detected"
		}

		return score, reasoning, false

	case strings.Contains(criterion.Name, "test_coverage"):
		// Check for test file references and test execution
		testPatterns := []string{"_test.go", ".test.js", ".spec.ts", "func Test", "go test", "npm test", "pytest", "describe(", "it("}
		found := 0
		for _, pattern := range testPatterns {
			if strings.Contains(output, pattern) {
				found++
			}
		}
		score := max(1, (found*maxScore)/2)
		if found >= 2 {
			score = maxScore
		}
		if contextViolations > 0 {
			score = max(1, score-contextViolations)
		}
		return score, fmt.Sprintf("found %d test coverage indicators", found), false

	default:
		// Unknown deterministic criterion — skip rather than award false credit
		return 0, fmt.Sprintf("no deterministic checker implemented for %q", criterion.Name), true
	}
}

// The returned CallRecord describes the judge call (also when it failed or
// its output could not be parsed, since tokens were spent).
func scoreLLMJudge(criterion Criterion, tc TestCase, actualOutput string) (CostInfo, int, string, bool, inference.CallRecord) {
	contextSection := ""
	if len(tc.Context) > 0 {
		contextSection = fmt.Sprintf("\nCONTEXT FACTS (use for hallucination detection — deduct for contradictions):\n%s\n", strings.Join(tc.Context, "\n"))
	}

	expectedSection := ""
	if tc.ExpectedOutput != "" {
		expectedSection = fmt.Sprintf("\nEXPECTED OUTPUT (compare similarity and completeness):\n%s\n", tc.ExpectedOutput)
	}

	prompt := fmt.Sprintf(`Evaluate this output against the criterion.
Wrap your JSON response between ===JSON_START=== and ===JSON_END=== delimiters.

{"score": <number 1-5>, "reasoning": "<explanation>", "pass": <boolean>}

CRITERION: %s
DESCRIPTION: %s

SCORING SCALE:
1 = Does not meet the criterion at all
2 = Minimally addresses the criterion with major gaps
3 = Partially meets the criterion with notable room for improvement
4 = Mostly meets the criterion with minor gaps
5 = Fully satisfies the criterion
%s%s
INPUT:
%s

ACTUAL OUTPUT TO EVALUATE:
%s`, criterion.Name, criterion.Description, contextSection, expectedSection, tc.Input, actualOutput)

	req := inference.Request{
		Role:    inference.RoleJudge,
		Prompt:  prompt,
		Timeout: 2 * time.Minute,
		Model:   cfg.pins.judgeModel(),
	}
	wallStart := time.Now()
	resp, err := cfg.backend.Invoke(context.Background(), req)
	rec := newCallRecord(req, resp, err, time.Since(wallStart), costFromUsage(resp.Model, resp.Usage))
	rec.Criterion = criterion.Name
	// On a failed or unparseable call this returns CostInfo{} (zero), so the
	// case's judge_cost deliberately keeps its historical zero-on-failure
	// behaviour. The tokens that were actually spent are not lost: rec already
	// carries the real cost and is appended to the case's calls[]. Do not add
	// rec's cost back into judge_cost — that would double-count against calls[].
	if err != nil {
		// The backend's error already carries the historical wording
		// (e.g. "kiro-cli chat failed: ...").
		return CostInfo{}, 0, err.Error(), true, rec
	}

	raw := resp.Text
	start := strings.Index(raw, "===JSON_START===")
	end := strings.Index(raw, "===JSON_END===")
	if start == -1 || end == -1 || end <= start {
		return CostInfo{}, 0, fmt.Sprintf("JSON delimiters not found in output"), true, rec
	}
	jsonStr := raw[start+len("===JSON_START===") : end]
	jsonStr = stripANSISequences(strings.TrimSpace(jsonStr))

	var response struct {
		Score     int    `json:"score"`
		Reasoning string `json:"reasoning"`
		Pass      bool   `json:"pass"`
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonStr)), &response); err != nil {
		return CostInfo{}, 0, fmt.Sprintf("JSON parse error: %v", err), true, rec
	}

	score := max(1, min(response.Score, 5))
	cost := costFromUsage(resp.Model, resp.Usage)
	return cost, score, response.Reasoning, false, rec
}

// newCallRecord builds the audit record for one backend call. Model is the
// served model when the backend reports one, else the pinned (requested) one;
// DurationMS prefers the backend's measurement over the wall clock (the stub
// reports 0). A failed call is still recorded, with Error set.
func newCallRecord(req inference.Request, resp inference.Response, err error, wall time.Duration, cost CostInfo) inference.CallRecord {
	rec := inference.CallRecord{
		Role:         string(req.Role),
		Model:        resp.Model,
		Agent:        req.Agent,
		InputTokens:  cost.TokensIn,
		OutputTokens: cost.TokensOut,
		CostUSD:      cost.EstimatedUSD,
		Estimated:    resp.Usage.Source != inference.UsageReported,
		DurationMS:   resp.Duration.Milliseconds(),
	}
	if rec.Model == "" {
		rec.Model = req.Model
	}
	if resp.Duration == 0 {
		rec.DurationMS = wall.Milliseconds()
	}
	if err != nil {
		rec.Error = err.Error()
	}
	return rec
}

func buildSummary(results []AgentResult, gitHash string) Summary {
	s := Summary{
		GitHash:     gitHash,
		AgentScores: make(map[string]float64),
	}

	for _, r := range results {
		var totalScore, totalMax float64
		for _, c := range r.Cases {
			for _, sc := range c.Scores {
				if sc.Skipped {
					continue
				}
				totalScore += float64(sc.Score)
				totalMax += float64(sc.MaxScore)
			}
			s.TotalCost.TokensIn += c.AgentCost.TokensIn
			s.TotalCost.TokensOut += c.AgentCost.TokensOut
			s.TotalCost.EstimatedUSD += c.AgentCost.EstimatedUSD
		}
		if totalMax > 0 {
			s.AgentScores[r.Agent] = totalScore / totalMax
		}
	}

	return s
}

func parseMaxScore(scoring string) int {
	// Parse "1-5" -> 5
	parts := strings.Split(scoring, "-")
	if len(parts) == 2 {
		var max int
		fmt.Sscanf(parts[1], "%d", &max)
		if max > 0 {
			return max
		}
	}
	return 5
}

func loadRubrics(agentFilter string) ([]Rubric, error) {
	dir := evalsPath("rubrics")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read rubrics directory: %w", err)
	}

	var rubrics []Rubric
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read rubric %s: %w", e.Name(), err)
		}

		var r Rubric
		if err := yaml.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("failed to parse rubric %s: %w", e.Name(), err)
		}

		if agentFilter == "" || r.Agent == agentFilter {
			rubrics = append(rubrics, r)
		}
	}

	return rubrics, nil
}

func loadCases(agent string) ([]TestCase, error) {
	dir := evalsPath("cases", agent)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cases directory for %s: %w", agent, err)
	}

	var cases []TestCase
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read case %s: %w", e.Name(), err)
		}

		var tc TestCase
		if err := yaml.Unmarshal(data, &tc); err != nil {
			return nil, fmt.Errorf("failed to parse case %s: %w", e.Name(), err)
		}

		if err := validateCaseFields(tc, e.Name()); err != nil {
			return nil, err
		}

		tc.Agent = agent
		cases = append(cases, tc)
	}

	return cases, nil
}

func stripANSISequences(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// getThreshold returns the success threshold for a test case (defaults to 80%).
func getThreshold(tc TestCase) float64 {
	if tc.MinScore != nil {
		return *tc.MinScore
	}
	return 80.0 // Default 80% threshold
}

func getGitShortHash() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runWithResume finds the most recent incomplete evaluation and resumes it.
func runWithResume(agent string) error {
	fmt.Println("🔄 Scanning for incomplete evaluations...")

	resultsBaseDir := evalsPath("results")
	entries, err := os.ReadDir(resultsBaseDir)
	if err != nil {
		return fmt.Errorf("❌ failed to read results directory: %w", err)
	}

	var latestDir string
	var latestTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		progressFile := filepath.Join(resultsBaseDir, entry.Name(), ".progress")
		if _, err := os.Stat(progressFile); err == nil {
			// Found incomplete evaluation
			info, err := entry.Info()
			if err == nil && info.ModTime().After(latestTime) {
				latestDir = filepath.Join(resultsBaseDir, entry.Name())
				latestTime = info.ModTime()
			}
		}
	}

	if latestDir == "" {
		fmt.Println("📄 No incomplete evaluations found, starting fresh...")
		return Run(agent, nil)
	}

	fmt.Printf("📂 Resuming evaluation from: %s\n", latestDir)
	return runProgressiveEvaluation(agent, latestDir, true, nil)
}

// runProgressiveEvaluation runs evaluation with progressive result saving.
func runProgressiveEvaluation(agent, resultsDir string, isResume bool, cConfig *ContainerConfig) error {
	gitHash, err := getGitShortHash()
	if err != nil {
		return fmt.Errorf("failed to get git hash: %w", err)
	}

	// Acquire lock to prevent concurrent evaluations
	lockFile, err := acquireResultLock(resultsDir)
	if err != nil {
		return err
	}
	defer releaseResultLock(lockFile)

	rubrics, err := loadRubrics(agent)
	if err != nil {
		return err
	}

	if len(rubrics) == 0 {
		return fmt.Errorf("❌ Fatal: no rubrics found")
	}

	for _, rubric := range rubrics {
		fmt.Printf("\n📋 Agent: %s\n", rubric.Agent)

		cases, err := loadCases(rubric.Agent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: no test cases for agent %s: %v\n", rubric.Agent, err)
			continue
		}

		if isResume {
			if err := checkResumeIntegrity(resultsDir, rubric.Agent); err != nil {
				return err
			}
		}

		result := evaluateProgressive(rubric, cases, gitHash, os.Stdout, resultsDir, isResume, cConfig)

		// Final save
		if err := saveProgressiveResult(resultsDir, rubric.Agent, result, gitHash); err != nil {
			return fmt.Errorf("failed to save final result: %w", err)
		}

		fmt.Printf("✅ %s: %d cases completed\n", rubric.Agent, len(result.Cases))
	}

	// Clean up progress file on completion
	progressFile := filepath.Join(resultsDir, ".progress")
	os.Remove(progressFile)

	fmt.Printf("\n🎉 Evaluation complete\n")
	fmt.Printf("📂 Results: %s\n", resultsDir)
	return nil
}

// checkResumeIntegrity refuses to resume into a result file that was written
// with a different prompt or different models, so one file never mixes two
// prompt/model versions. A legacy file (no recorded prompt_sha256) is refused
// when it already holds saved cases, since resuming would stamp the current
// models/hash onto scores produced under unknown inputs; an empty legacy file,
// and unpinned runs, are not checked.
func checkResumeIntegrity(resultsDir, agent string) error {
	pin, ok := cfg.pins.pinOf(agent)
	if !ok {
		return nil
	}

	// A resumed run must keep the judge model recorded for the run as a whole,
	// even for an agent that has no result file yet. In a multi-agent run
	// interrupted after one agent was saved but before this one's file exists,
	// the per-agent check below is a no-op (nothing to read), yet
	// updateIncrementalSummary would overwrite summary.judge_model with the new
	// value and leave the already-saved agent's scores attributed to a judge it
	// never ran on. Guard against a run-level judge change independently.
	if sumData, err := os.ReadFile(filepath.Join(resultsDir, "summary.json")); err == nil {
		var existingSum Summary
		if json.Unmarshal(sumData, &existingSum) == nil &&
			existingSum.JudgeModel != "" && existingSum.JudgeModel != cfg.pins.Judge {
			return fmt.Errorf("❌ cannot resume: judge_model changed since the interrupted run (summary recorded judge_model=%s; now %s)",
				existingSum.JudgeModel, cfg.pins.Judge)
		}
	}

	data, err := os.ReadFile(filepath.Join(resultsDir, agent+".json"))
	if err != nil {
		return nil
	}
	var existing AgentResult
	if json.Unmarshal(data, &existing) != nil {
		return nil
	}
	if existing.PromptSHA256 == "" {
		// Legacy result written before provenance existed. Resuming would stamp
		// the current models/hash onto its saved cases, claiming a provenance
		// they never had. Refuse when there is anything to mis-attribute; an
		// empty legacy file has no scores to protect.
		if len(existing.Cases) > 0 {
			return fmt.Errorf("❌ cannot resume: %s.json predates run provenance and has %d saved case(s); its scores cannot be attributed to the current prompt/models — start a fresh run (without --resume)",
				agent, len(existing.Cases))
		}
		return nil
	}
	if existing.PromptSHA256 != pin.Provenance.PromptSHA256 ||
		existing.AgentModel != pin.Model ||
		existing.JudgeModel != cfg.pins.Judge {
		return fmt.Errorf("❌ cannot resume: prompt or models changed since the interrupted run of %s (recorded agent_model=%s judge_model=%s prompt_sha256=%s; now agent_model=%s judge_model=%s prompt_sha256=%s)",
			agent, existing.AgentModel, existing.JudgeModel, existing.PromptSHA256,
			pin.Model, cfg.pins.Judge, pin.Provenance.PromptSHA256)
	}
	return nil
}

// evaluateProgressive runs evaluation with progressive result saving after each test case.
func evaluateProgressive(rubric Rubric, cases []TestCase, gitHash string, out io.Writer, resultsDir string, isResume bool, cConfig *ContainerConfig) AgentResult {
	result := AgentResult{
		Agent:   rubric.Agent,
		GitHash: gitHash,
	}
	cfg.pins.applyTo(&result)

	// If resuming, load existing results
	if isResume {
		if existingData, err := os.ReadFile(filepath.Join(resultsDir, rubric.Agent+".json")); err == nil {
			var existing AgentResult
			if json.Unmarshal(existingData, &existing) == nil {
				result = existing
				// Provenance is verified equal by checkResumeIntegrity (or the
				// file predates provenance); stamp it from the current pins.
				cfg.pins.applyTo(&result)
			}
		}
	}

	for i, tc := range cases {
		// Skip if already completed during resume
		if isResume && isTestCaseCompleted(resultsDir, rubric.Agent, tc.Name) {
			fmt.Fprintf(out, "   [%d/%d] %s ✅ (already completed)\n", i+1, len(cases), tc.Name)
			continue
		}

		fmt.Fprintf(out, "   [%d/%d] %s", i+1, len(cases), tc.Name)

		// Track test case start time for performance profiling
		testStart := time.Now()

		// Execute and score the test case (shared per-case path)
		cr := executeCase(rubric, tc, cConfig, out, cfg.keepWorkspaces)

		// Add or update the case result
		found := false
		for j, existing := range result.Cases {
			if existing.CaseName == tc.Name {
				result.Cases[j] = cr
				found = true
				break
			}
		}
		if !found {
			result.Cases = append(result.Cases, cr)
		}

		// Progressive save after each test case
		if err := saveProgressiveResult(resultsDir, rubric.Agent, result, gitHash); err != nil {
			fmt.Fprintf(out, " ⚠️ (save failed: %v)\n", err)
		}

		// Track test case completion time for performance profiling
		testDuration := time.Since(testStart)
		TrackTestCase(tc.Name, testDuration)

		// Display result
		printCaseResult(out, tc, cr)
	}

	return result
}

// acquireResultLock creates an exclusive lock file to prevent concurrent writes.
// Writes the current PID to detect stale locks from crashed processes.
func acquireResultLock(resultsDir string) (*os.File, error) {
	lockPath := filepath.Join(resultsDir, ".lock")

	// Check for stale lock
	if data, err := os.ReadFile(lockPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			// Check if process is still running
			proc, err := os.FindProcess(pid)
			if err != nil || proc.Signal(nil) != nil {
				// Process not running — remove stale lock
				os.Remove(lockPath)
			} else {
				return nil, fmt.Errorf("evaluation already running (PID %d)", pid)
			}
		} else {
			// Malformed lock file — remove it
			os.Remove(lockPath)
		}
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire result lock (evaluation may be running elsewhere): %w", err)
	}

	// Write PID for stale detection
	fmt.Fprintf(lockFile, "%d", os.Getpid())
	return lockFile, nil
}

// releaseResultLock removes the lock file.
func releaseResultLock(lockFile *os.File) {
	if lockFile != nil {
		lockFile.Close()
		os.Remove(lockFile.Name())
	}
}

// saveProgressiveResult immediately saves a test case result and updates summary.
func saveProgressiveResult(resultsDir, agent string, result AgentResult, gitHash string) error {
	// Write individual agent result
	agentFile := filepath.Join(resultsDir, agent+".json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal agent result: %w", err)
	}

	if err := os.WriteFile(agentFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write agent result: %w", err)
	}

	// Update incremental summary
	summaryFile := filepath.Join(resultsDir, "summary.json")
	return updateIncrementalSummary(summaryFile, result, gitHash)
}

// updateIncrementalSummary adds/updates an agent's results in the summary file.
func updateIncrementalSummary(summaryFile string, agentResult AgentResult, gitHash string) error {
	var summary Summary

	// Load existing summary if it exists
	if data, err := os.ReadFile(summaryFile); err == nil {
		json.Unmarshal(data, &summary)
	}

	// Initialize if empty
	if summary.AgentScores == nil {
		summary.AgentScores = make(map[string]float64)
		summary.GitHash = gitHash
	}

	// Calculate and update agent score
	var totalScore, totalMax float64
	var agentCost CostInfo

	for _, c := range agentResult.Cases {
		for _, sc := range c.Scores {
			if !sc.Skipped {
				totalScore += float64(sc.Score)
				totalMax += float64(sc.MaxScore)
			}
		}
		agentCost.TokensIn += c.AgentCost.TokensIn
		agentCost.TokensOut += c.AgentCost.TokensOut
		agentCost.EstimatedUSD += c.AgentCost.EstimatedUSD

		agentCost.TokensIn += c.JudgeCost.TokensIn
		agentCost.TokensOut += c.JudgeCost.TokensOut
		agentCost.EstimatedUSD += c.JudgeCost.EstimatedUSD
	}

	if totalMax > 0 {
		summary.AgentScores[agentResult.Agent] = totalScore / totalMax
	}

	// Update total cost (this is cumulative across all agents)
	summary.TotalCost = agentCost

	// Provenance. A multi-agent run cannot carry a single top-level agent
	// model/hash, so those are set only when exactly one agent is covered.
	if agentResult.JudgeModel != "" {
		summary.JudgeModel = agentResult.JudgeModel
	}
	if agentResult.PromptSHA256 != "" {
		if summary.Agents == nil {
			summary.Agents = make(map[string]AgentProvenance)
		}
		summary.Agents[agentResult.Agent] = AgentProvenance{
			AgentModel:       agentResult.AgentModel,
			PromptSHA256:     agentResult.PromptSHA256,
			ResourcesPresent: append([]string{}, agentResult.ResourcesPresent...),
		}
	}
	summary.AgentModel, summary.PromptSHA256, summary.ResourcesPresent = "", "", nil
	if len(summary.Agents) == 1 {
		for _, p := range summary.Agents {
			summary.AgentModel = p.AgentModel
			summary.PromptSHA256 = p.PromptSHA256
			summary.ResourcesPresent = p.ResourcesPresent
		}
	}

	// Write updated summary
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	return os.WriteFile(summaryFile, data, 0644)
}

// isTestCaseCompleted checks if a test case result already exists.
func isTestCaseCompleted(resultsDir, agent, testCaseName string) bool {
	agentFile := filepath.Join(resultsDir, agent+".json")
	data, err := os.ReadFile(agentFile)
	if err != nil {
		return false
	}

	var result AgentResult
	if err := json.Unmarshal(data, &result); err != nil {
		return false
	}

	for _, caseResult := range result.Cases {
		if caseResult.CaseName == testCaseName {
			return true
		}
	}

	return false
}

// RunPerformanceInvestigation conducts comprehensive performance analysis.
func RunPerformanceInvestigation(agent string) error {
	fmt.Println("🔍 Starting comprehensive performance investigation...")

	if agent == "" {
		return fmt.Errorf("❌ agent name required for performance investigation")
	}

	// Start profiling
	StartProfiling()

	// Measure startup overhead multiple times for accuracy
	fmt.Println("📊 Measuring startup overhead (3 samples)...")
	var startupTimes []time.Duration
	for i := 0; i < 3; i++ {
		startupTime := MeasureStartupOverhead()
		startupTimes = append(startupTimes, startupTime)
		fmt.Printf("  Sample %d: %v\n", i+1, startupTime)
	}

	// Calculate average startup time
	var totalStartup time.Duration
	for _, t := range startupTimes {
		totalStartup += t
	}
	avgStartup := totalStartup / time.Duration(len(startupTimes))
	fmt.Printf("  Average: %v\n", avgStartup)

	// Load test cases for analysis
	cases, err := loadCases(agent)
	if err != nil {
		return fmt.Errorf("failed to load test cases for %s: %w", agent, err)
	}

	if len(cases) == 0 {
		fmt.Printf("⚠️  No test cases found for agent %s\n", agent)
		return nil
	}

	fmt.Printf("📋 Found %d test cases for performance analysis\n", len(cases))

	// Run parallel execution benchmark
	fmt.Println("\n🚀 Benchmarking sequential vs parallel execution...")
	benchmark, err := InvestigateParallelExecution(agent)
	if err != nil {
		fmt.Printf("⚠️  Parallel execution benchmark failed: %v\n", err)
	}

	// Analyze test case complexity
	fmt.Println("\n📈 Analyzing test case complexity...")
	for i, tc := range cases {
		if i >= 3 { // Limit to first 3 cases for quick analysis
			fmt.Printf("  ... (analyzing remaining %d cases)\n", len(cases)-i)
			break
		}

		promptSize := len(tc.Input)
		if len(tc.Setup) > 0 {
			for _, setup := range tc.Setup {
				promptSize += len(setup.Content)
			}
		}

		fmt.Printf("  %s: %d chars input\n", tc.Name, promptSize)
	}

	// Create results directory for performance report
	timestamp := generateTimestampPrefix()
	gitHash, _ := getGitShortHash()
	resultsDir := evalsPath("results", timestamp+"-perf-"+gitHash)
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}

	// Generate comprehensive performance profile
	profile := GenerateProfile()

	// Add startup analysis to profile
	profile.StartupOverhead = avgStartup

	// Save detailed performance report
	if err := SavePerformanceReport(resultsDir, profile, benchmark); err != nil {
		fmt.Printf("⚠️  Failed to save performance report: %v\n", err)
	}

	// Display comprehensive analysis
	fmt.Println("\n🎯 Performance Investigation Results:")
	fmt.Println("=====================================")

	PrintPerformanceReport(profile, benchmark)

	// Additional recommendations specific to investigation mode
	fmt.Println("\n🔬 Investigation-Specific Findings:")
	if len(startupTimes) > 1 {
		// Calculate startup variance
		var variance time.Duration
		for _, t := range startupTimes {
			diff := t - avgStartup
			if diff < 0 {
				diff = -diff
			}
			variance += diff
		}
		variance /= time.Duration(len(startupTimes))

		if variance > avgStartup/10 { // More than 10% variance
			fmt.Printf("  ⚠️  High startup variance detected (%v), consider system load factors\n", variance)
		}
	}

	if benchmark != nil && benchmark.Speedup < 1.2 {
		fmt.Printf("  💡 Limited parallel benefits due to startup overhead\n")
	}

	fmt.Printf("\n📂 Detailed report saved to: %s/performance.json\n", resultsDir)

	return nil
}

// saveDebugContainerInfo saves container registry information for debug mode
func saveDebugContainerInfo(containerID, imageName, customImageName, platform, agent string) error {
	debugDir := filepath.Join(".kairon", "evals", "tmp", "debug")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		return fmt.Errorf("creating debug directory: %w", err)
	}

	// Create container info file
	timestamp := time.Now().Format("20060102-150405")

	infoFile := filepath.Join(debugDir, fmt.Sprintf("container-%s-%s.json", timestamp, containerID))

	info := map[string]interface{}{
		"container_id": containerID,
		"short_id":     containerID,
		"base_image":   imageName,
		"custom_image": customImageName,
		"platform":     platform,
		"agent":        agent,
		"timestamp":    time.Now().Format(time.RFC3339),
		"debug_commands": map[string]string{
			"inspect":    fmt.Sprintf("docker inspect %s", containerID),
			"logs":       fmt.Sprintf("docker logs %s", containerID),
			"exec":       fmt.Sprintf("docker exec -it %s /bin/bash", containerID),
			"image_info": fmt.Sprintf("docker image inspect %s", customImageName),
		},
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling container info: %w", err)
	}

	if err := os.WriteFile(infoFile, data, 0644); err != nil {
		return fmt.Errorf("writing container info to %s: %w", infoFile, err)
	}

	fmt.Printf("🔧 Debug: Container info saved to %s\n", infoFile)
	return nil
}
