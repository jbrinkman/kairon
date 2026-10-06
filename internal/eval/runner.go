package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// checkDockerAvailability verifies Docker daemon is running and accessible
func checkDockerAvailability() error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := cli.Ping(ctx); err != nil {
		return fmt.Errorf("Docker is not running. Start Docker and try again: %w", err)
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

	// The sandbox path is hard-coded to run kiro-cli inside a container, so it
	// is only valid for the kiro-cli backend. Reject --sandbox for every other
	// backend (an allowlist, not a stub denylist) so a future backend used with
	// --sandbox fails loudly instead of silently running kiro-cli.
	if options.Sandbox && !options.NoSandbox && cfg.backend.Name() != inference.NameKiroCLI {
		return fmt.Errorf("❌ --backend %s cannot be combined with --sandbox: the sandbox runs kiro-cli in a container", cfg.backend.Name())
	}

	// Handle cleanup operation early
	if options.Cleanup {
		return RunCleanup()
	}

	// Handle performance investigation
	if options.Perf {
		return RunPerformanceInvestigation(agent)
	}

	// Configure container sandboxing
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

	// Task 4: Build images once at evaluation start if using containers
	if cConfig != nil {
		// Generate evaluation ID for image scoping
		gitHash, err := getGitShortHash()
		if err != nil {
			return fmt.Errorf("failed to get git hash: %w", err)
		}
		timestamp := generateTimestampPrefix()
		evaluationID := fmt.Sprintf("%s-%s", timestamp, gitHash)

		fmt.Println("🔨 Building evaluation images...")
		imageManager, err := sandbox.NewImageManager(evaluationID, cConfig.Debug)
		if err != nil {
			return fmt.Errorf("failed to create image manager: %w", err)
		}
		defer func() {
			if err := imageManager.Cleanup(context.Background()); err != nil {
				fmt.Printf("⚠️ Warning: Failed to cleanup images: %v\n", err)
			}
			imageManager.Close()
		}()

		// Pre-build the image for this platform
		buildStart := time.Now()
		c, err := sandbox.NewContainerWithDebug("", cConfig.Debug)
		if err != nil {
			return fmt.Errorf("creating container for image build: %w", err)
		}
		defer c.Close()

		dockerfile, err := c.GenerateDockerfileWithPlatform(cConfig.WorkspaceDir, cConfig.Platform)
		if err != nil {
			return fmt.Errorf("generating dockerfile: %w", err)
		}

		imageName, err := imageManager.BuildForEvaluation(context.Background(), dockerfile, cConfig.Platform)
		if err != nil {
			return fmt.Errorf("building evaluation image: %w", err)
		}
		fmt.Printf("✅ Images built: %v\n", time.Since(buildStart))

		// Add image manager and cached image name to config for test cases
		cConfig.ImageManager = imageManager
		cConfig.CachedImageName = imageName
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

	for i, tc := range cases {
		fmt.Fprintf(out, "   [%d/%d] %s", i+1, len(cases), tc.Name)

		testStart := time.Now()
		cr := CaseResult{CaseName: tc.Name}

		// Task 4: Structured output - Agent → Case execution
		prompt, err := assemblePrompt(tc.Setup, tc.Input)
		if err != nil {
			fmt.Fprintf(out, " ❌ (prompt error)\n")
			fmt.Fprintf(out, "      Error: %v\n", err)
			cr.ActualOutput = ""
		} else {
			fmt.Fprintf(out, " → running agent...")
			actualOutput, cost, errorContext, err := invokeAgent(rubric.Agent, prompt, cConfig, tc.Stub)
			if err != nil {
				fmt.Fprintf(out, " ❌ (agent failed)\n")
				fmt.Fprintf(out, "      Error: %v\n", err)
				cr.ActualOutput = ""
				cr.ErrorContext = errorContext
			} else {
				cr.ActualOutput = actualOutput
				cr.AgentCost = cost
				cr.ErrorContext = errorContext
				fmt.Fprintf(out, " → evaluating...")
			}
		}

		// Task 4: Criterion-by-criterion evaluation display
		scoreCase(rubric, tc, &cr)

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
// With a container config it runs in Docker; otherwise it goes through the
// configured inference backend. stub is the case's scripted response, used
// only by the stub backend.
func invokeAgent(agent, prompt string, cConfig *ContainerConfig, stub *inference.StubScript) (string, CostInfo, *ErrorContext, error) {
	// Use container execution if configured
	if cConfig != nil {
		return invokeAgentInContainer(agent, prompt, cConfig)
	}

	return invokeAgentViaBackend(agent, prompt, stub)
}

// createContainerConfig builds container configuration from CLI options
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

// invokeAgentInContainer executes kiro-cli in a Docker container with cached images
func invokeAgentInContainer(agent, prompt string, cConfig *ContainerConfig) (string, CostInfo, *ErrorContext, error) {
	ctx := context.Background()

	c, err := sandbox.NewContainerWithDebug("", cConfig.Debug)
	if err != nil {
		return "", CostInfo{}, nil, fmt.Errorf("creating container: %w", err)
	}
	defer c.Close()

	// Use pre-built image or build one individually
	var customImageName string
	if cConfig.CachedImageName != "" {
		customImageName = cConfig.CachedImageName
	} else {
		// Fallback to individual build for compatibility
		buildCtx, buildCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer buildCancel()

		buildStart := time.Now()
		dockerfile, err := c.GenerateDockerfileWithPlatform(cConfig.WorkspaceDir, cConfig.Platform)
		if err != nil {
			return "", CostInfo{}, nil, fmt.Errorf("generating dockerfile: %w", err)
		}

		customImageName = c.GetCustomImageName(cConfig.Platform)
		if err := c.BuildImageFromDockerfile(buildCtx, dockerfile, customImageName, cConfig.Platform); err != nil {
			return "", CostInfo{}, nil, fmt.Errorf("building custom image: %w", err)
		}
		fmt.Printf("  Image build: %v\n", time.Since(buildStart))

		// Clean up image after use (preserve in debug mode)
		if !cConfig.Debug {
			defer func() {
				if _, err := c.RemoveImage(ctx, customImageName); err != nil {
					fmt.Printf("⚠️ Warning: Failed to remove custom image %s: %v\n", customImageName, err)
				}
			}()
		}
	}

	// Container startup (same as before)
	createStart := time.Now()

	hostConfig := sandbox.NewHostConfigWithLimits(cConfig.ResourceLimits)

	// Configure environment variables from host system and container config
	envVars := []string{
		"KIRO_CLI_DISABLE_TELEMETRY=1",
	}
	for key, value := range cConfig.Environment {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	containerCfg := &container.Config{
		Image:      customImageName,
		Cmd:        []string{"sleep", "3600"},
		Env:        envVars,
		WorkingDir: cConfig.WorkspaceDir,
	}

	// Use platform-aware container creation
	if err := c.CreateWithPlatform(ctx, containerCfg, hostConfig, cConfig.Platform); err != nil {
		return "", CostInfo{}, nil, fmt.Errorf("creating container: %w", err)
	}
	containerFailed := false
	defer func() {
		c.CleanupWithDebugInfo(ctx, containerFailed)
	}()

	if err := c.Start(ctx); err != nil {
		return "", CostInfo{}, nil, fmt.Errorf("starting container: %w", err)
	}

	c.LogStartup(cConfig.ResourceLimits)
	fmt.Printf("  Container startup: %v\n", time.Since(createStart))

	// Debug mode: Save container registry information
	if cConfig.Debug {
		shortID, imageName := c.GetContainerInfo()
		if err := saveDebugContainerInfo(shortID, imageName, customImageName, cConfig.Platform, agent); err != nil {
			fmt.Printf("⚠️ Warning: Failed to save debug container info: %v\n", err)
		}
	}

	// Container setup (same as before)
	setupStart := time.Now()

	// Verify kiro-cli is pre-installed and functional
	if err := c.ValidateKiroCLI(ctx, cConfig.Platform); err != nil {
		return "", CostInfo{}, nil, fmt.Errorf("validating kiro-cli: %w", err)
	}

	// Setup GitHub mocking if enabled
	if cConfig.MockGitHub {
		if err := c.SetupGitHubMocking(ctx, cConfig.WorkspaceDir); err != nil {
			return "", CostInfo{}, nil, fmt.Errorf("setting up GitHub mocking: %w", err)
		}

		if err := c.ConfigureMockGitHubPath(ctx); err != nil {
			return "", CostInfo{}, nil, fmt.Errorf("configuring mock GitHub PATH: %w", err)
		}
	}

	fmt.Printf("  Container setup: %v\n", time.Since(setupStart))

	// Command execution (same as before)
	timeoutCtx, cancel := context.WithTimeout(ctx, cConfig.ResourceLimits.Timeout)
	defer cancel()

	cmd := []string{"kiro-cli", "chat", "--agent", agent, "--no-interactive", "--trust-all-tools"}

	executionStart := time.Now()
	output, err := c.ExecWithOutput(timeoutCtx, cmd)
	executionDuration := time.Since(executionStart)

	if err != nil {
		// Enhanced error context with container information
		shortID, imageName := c.GetContainerInfo()

		errorContext := &ErrorContext{
			Command:        strings.Join(cmd, " "),
			WorkingDir:     cConfig.WorkspaceDir,
			Environment:    cConfig.Environment,
			Stderr:         err.Error(),
			ContainerID:    shortID,
			ContainerImage: imageName,
			Platform:       cConfig.Platform,
			DockerError:    err.Error(),
		}

		// In debug mode, preserve the failed container
		if cConfig.Debug {
			containerFailed = true
		}

		// Provide actionable error messages for common container issues
		if strings.Contains(err.Error(), "timeout") || timeoutCtx.Err() == context.DeadlineExceeded {
			return "", CostInfo{}, errorContext, fmt.Errorf("⏱️ Container execution timeout after %v. Consider increasing --resource-limit timeout=", cConfig.ResourceLimits.Timeout)
		}
		if strings.Contains(err.Error(), "out of memory") || strings.Contains(err.Error(), "OOMKilled") {
			memoryMB := cConfig.ResourceLimits.Memory / (1024 * 1024)
			return "", CostInfo{}, errorContext, fmt.Errorf("💾 Container ran out of memory (%dMB limit). Consider increasing --resource-limit memory=", memoryMB)
		}
		if strings.Contains(err.Error(), "no such image") || strings.Contains(err.Error(), "pull access denied") {
			return "", CostInfo{}, errorContext, fmt.Errorf("❌ Failed to pull image %s: %v. Check internet connection", imageName, err)
		}

		return "", CostInfo{}, errorContext, fmt.Errorf("container execution failed: %w", err)
	}

	fmt.Printf("  Execution time: %v\n", executionDuration)

	result := stripANSISequences(output)
	cost := estimateCost(prompt, result)

	return result, cost, nil, nil
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

// invokeAgentViaBackend runs the agent through the configured inference backend.
func invokeAgentViaBackend(agent, prompt string, stub *inference.StubScript) (string, CostInfo, *ErrorContext, error) {
	timeoutStr := os.Getenv("KAIRON_EVAL_TIMEOUT")
	timeout := 2 * time.Minute
	if timeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsedTimeout
		}
	}

	// Capture working directory
	workingDir, _ := os.Getwd()

	// Capture relevant environment variables
	envVars := make(map[string]string)
	for _, key := range []string{"KAIRON_EVAL_TIMEOUT"} {
		if val := os.Getenv(key); val != "" {
			envVars[key] = val
		}
	}

	resp, err := cfg.backend.Invoke(context.Background(), inference.Request{
		Role:           inference.RoleAgent,
		Agent:          agent,
		Prompt:         prompt,
		Timeout:        timeout,
		AgentConfigDir: agentConfigDir(agent),
		Stub:           stub,
		// Turn is left 0: multi-turn stub selection lands with E9 (multi-turn
		// cases). StubScript.Turns is a slice for that future, not yet wired.
	})

	if resp.Duration > 30*time.Second {
		fmt.Printf(" (>30s)")
	}

	// Create error context for any execution issues
	var errorContext *ErrorContext
	if err != nil || len(resp.Stderr) > 0 {
		errorContext = &ErrorContext{
			Command:     resp.Command,
			WorkingDir:  workingDir,
			Environment: envVars,
			Stderr:      resp.Stderr,
			ExitCode:    resp.ExitCode,
		}
	}

	if err != nil {
		if errors.Is(err, inference.ErrTimeout) && errorContext != nil {
			errorContext.Stderr = fmt.Sprintf("timeout after %v\n%s", timeout, errorContext.Stderr)
		}
		return "", CostInfo{}, errorContext, err
	}

	return resp.Text, costFromUsage(resp.Model, resp.Usage), errorContext, nil
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

func scoreLLMJudge(criterion Criterion, tc TestCase, actualOutput string) (CostInfo, int, string, bool) {
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

	resp, err := cfg.backend.Invoke(context.Background(), inference.Request{
		Role:    inference.RoleJudge,
		Prompt:  prompt,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		// The backend's error already carries the historical wording
		// (e.g. "kiro-cli chat failed: ...").
		return CostInfo{}, 0, err.Error(), true
	}

	raw := resp.Text
	start := strings.Index(raw, "===JSON_START===")
	end := strings.Index(raw, "===JSON_END===")
	if start == -1 || end == -1 || end <= start {
		return CostInfo{}, 0, fmt.Sprintf("JSON delimiters not found in output"), true
	}
	jsonStr := raw[start+len("===JSON_START===") : end]
	jsonStr = stripANSISequences(strings.TrimSpace(jsonStr))

	var response struct {
		Score     int    `json:"score"`
		Reasoning string `json:"reasoning"`
		Pass      bool   `json:"pass"`
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonStr)), &response); err != nil {
		return CostInfo{}, 0, fmt.Sprintf("JSON parse error: %v", err), true
	}

	score := max(1, min(response.Score, 5))
	cost := costFromUsage(resp.Model, resp.Usage)
	return cost, score, response.Reasoning, false
}

func estimateCost(input, output string) CostInfo {
	return costFromUsage("", inference.EstimateUsage(input, output))
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

// evaluateProgressive runs evaluation with progressive result saving after each test case.
func evaluateProgressive(rubric Rubric, cases []TestCase, gitHash string, out io.Writer, resultsDir string, isResume bool, cConfig *ContainerConfig) AgentResult {
	result := AgentResult{
		Agent:   rubric.Agent,
		GitHash: gitHash,
	}

	// If resuming, load existing results
	if isResume {
		if existingData, err := os.ReadFile(filepath.Join(resultsDir, rubric.Agent+".json")); err == nil {
			var existing AgentResult
			if json.Unmarshal(existingData, &existing) == nil {
				result = existing
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

		cr := CaseResult{CaseName: tc.Name}

		// Execute test case
		prompt, err := assemblePrompt(tc.Setup, tc.Input)
		if err != nil {
			fmt.Fprintf(out, " ❌ (prompt error)\n")
			fmt.Fprintf(out, "      Error: %v\n", err)
			cr.ActualOutput = ""
		} else {
			fmt.Fprintf(out, " → running agent...")
			actualOutput, cost, errorContext, err := invokeAgent(rubric.Agent, prompt, cConfig, tc.Stub)
			if err != nil {
				fmt.Fprintf(out, " ❌ (agent failed)\n")
				fmt.Fprintf(out, "      Error: %v\n", err)
				cr.ActualOutput = ""
				cr.ErrorContext = errorContext
			} else {
				cr.ActualOutput = actualOutput
				cr.AgentCost = cost
				cr.ErrorContext = errorContext
				fmt.Fprintf(out, " → evaluating...")
			}
		}

		// Score the test case
		scoreCase(rubric, tc, &cr)

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
