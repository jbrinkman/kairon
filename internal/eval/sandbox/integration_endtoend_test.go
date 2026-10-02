package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEndFlow validates the complete unified container creation flow:
// generate → build → create → verify
func TestEndToEndFlow(t *testing.T) {
	skipIfNoDocker(t)

	hostPlatform, err := DetectHostArchitecture()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	startTime := time.Now()

	// Phase 1: Generate - Generate Dockerfile with kiro-cli pre-installed
	t.Log("Phase 1: Generate Dockerfile")
	generateStart := time.Now()

	c, err := NewContainerWithDebug("", false) // Non-debug mode for performance measurement
	require.NoError(t, err)
	defer c.Close()

	dockerfile, err := c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
	require.NoError(t, err)

	generateDuration := time.Since(generateStart)
	t.Logf("  Generate phase: %v", generateDuration)

	// Validate Dockerfile content
	assert.Contains(t, dockerfile, "FROM alpine:3.19")
	assert.Contains(t, dockerfile, "# Install kiro-cli")
	assert.Contains(t, dockerfile, "WORKDIR /workspace")

	// Phase 2: Build - Build custom image from generated Dockerfile
	t.Log("Phase 2: Build custom image")
	buildStart := time.Now()

	customImageName := c.GetCustomImageName(hostPlatform)
	require.NotEmpty(t, customImageName)
	assert.Contains(t, customImageName, "kairon-eval:")

	err = c.BuildImageFromDockerfile(ctx, dockerfile, customImageName, hostPlatform)
	require.NoError(t, err)

	buildDuration := time.Since(buildStart)
	t.Logf("  Build phase: %v", buildDuration)

	// Phase 3: Create - Create container with custom image
	t.Log("Phase 3: Create container")
	createStart := time.Now()

	limits := DefaultLimits()
	hostConfig := NewHostConfigWithLimits(limits)

	containerCfg := &container.Config{
		Image:      customImageName, // Use custom image, not alpine:3.19
		Cmd:        []string{"sleep", "300"},
		WorkingDir: "/workspace",
		Env:        []string{"KIRO_CLI_DISABLE_TELEMETRY=1"},
	}

	err = c.CreateWithPlatform(ctx, containerCfg, hostConfig, hostPlatform)
	require.NoError(t, err)

	defer func() {
		if cleanupErr := c.Cleanup(ctx); cleanupErr != nil {
			t.Logf("Cleanup warning: %v", cleanupErr)
		}
	}()

	err = c.Start(ctx)
	require.NoError(t, err)

	createDuration := time.Since(createStart)
	t.Logf("  Create phase: %v", createDuration)

	// Phase 4: Verify - Validate kiro-cli is pre-installed and functional
	t.Log("Phase 4: Verify pre-installed kiro-cli")
	verifyStart := time.Now()

	err = c.ValidateKiroCLI(ctx, hostPlatform)
	require.NoError(t, err)

	// Additional verification - test actual kiro-cli execution
	output, err := c.ExecWithOutput(ctx, []string{"kiro-cli", "--version"})
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(output), "kiro-cli")

	verifyDuration := time.Since(verifyStart)
	t.Logf("  Verify phase: %v", verifyDuration)

	totalDuration := time.Since(startTime)
	t.Logf("Total end-to-end flow: %v", totalDuration)

	// Performance validation - ensure reasonable execution time
	assert.Less(t, totalDuration, 5*time.Minute, "End-to-end flow should complete within 5 minutes")

	// Verify all phases completed successfully
	t.Log("✅ All phases completed successfully:")
	t.Logf("  1. Generate: %v", generateDuration)
	t.Logf("  2. Build: %v", buildDuration)
	t.Logf("  3. Create: %v", createDuration)
	t.Logf("  4. Verify: %v", verifyDuration)
	t.Logf("  Total: %v", totalDuration)
}

// TestEndToEndFlowWithDebug validates debug mode includes proper artifact saving
func TestEndToEndFlowWithDebug(t *testing.T) {
	skipIfNoDocker(t)

	hostPlatform, err := DetectHostArchitecture()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Create container with debug mode enabled
	c, err := NewContainerWithDebug("", true)
	require.NoError(t, err)
	defer c.Close()

	// Generate and build
	dockerfile, err := c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
	require.NoError(t, err)

	customImageName := c.GetCustomImageName(hostPlatform)
	assert.Contains(t, customImageName, "kairon-eval-debug:", "Debug mode should use debug image naming")

	err = c.BuildImageFromDockerfile(ctx, dockerfile, customImageName, hostPlatform)
	require.NoError(t, err)

	// Create and start container
	limits := DefaultLimits()
	hostConfig := NewHostConfigWithLimits(limits)

	containerCfg := &container.Config{
		Image:      customImageName,
		Cmd:        []string{"sleep", "300"},
		WorkingDir: "/workspace",
	}

	err = c.CreateWithPlatform(ctx, containerCfg, hostConfig, hostPlatform)
	require.NoError(t, err)

	defer func() {
		// In debug mode, container should be preserved for inspection
		if cleanupErr := c.CleanupWithDebugInfo(ctx, false); cleanupErr != nil {
			t.Logf("Cleanup warning: %v", cleanupErr)
		}
	}()

	err = c.Start(ctx)
	require.NoError(t, err)

	// Verify the complete flow works in debug mode too
	err = c.ValidateKiroCLI(ctx, hostPlatform)
	require.NoError(t, err)

	// Verify container info is available for debug artifact saving
	shortID, imageName := c.GetContainerInfo()
	assert.NotEmpty(t, shortID, "Debug mode should provide container ID")
	assert.NotEmpty(t, imageName, "Debug mode should provide image name")

	// Exercise the artifact-saving path (same as runner.go debug flow)
	debugDir := filepath.Join(os.TempDir(), "kairon-eval-debug-test")
	err = os.MkdirAll(debugDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(debugDir)

	// Save debug artifacts similar to saveDebugContainerInfo in runner.go
	infoFile := filepath.Join(debugDir, fmt.Sprintf("container-%s.json", shortID))
	info := map[string]interface{}{
		"container_id": shortID,
		"image_name":   imageName,
		"custom_image": customImageName,
		"platform":     hostPlatform,
	}
	data, err := json.MarshalIndent(info, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(infoFile, data, 0644)
	require.NoError(t, err)

	// Verify artifact was written correctly
	savedData, err := os.ReadFile(infoFile)
	require.NoError(t, err)
	assert.Contains(t, string(savedData), shortID)
	assert.Contains(t, string(savedData), customImageName)
}

// TestFlowConsistencyBetweenTestAndProduction validates that test and production
// paths use the same container creation flow
func TestFlowConsistencyBetweenTestAndProduction(t *testing.T) {
	skipIfNoDocker(t)

	hostPlatform, err := DetectHostArchitecture()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Test that both test and production code paths use the same methods:
	// 1. GenerateDockerfileWithPlatform()
	// 2. BuildImageFromDockerfile()
	// 3. CreateWithPlatform() with custom image
	// 4. ValidateKiroCLI() for verification

	c, err := NewContainer("") // Empty image - should use generated custom image
	require.NoError(t, err)
	defer c.Close()

	// Verify GenerateDockerfileWithPlatform works (used by both test and production)
	dockerfile, err := c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
	require.NoError(t, err)
	assert.NotEmpty(t, dockerfile)

	// Verify BuildImageFromDockerfile works (used by both test and production)
	customImageName := c.GetCustomImageName(hostPlatform)
	err = c.BuildImageFromDockerfile(ctx, dockerfile, customImageName, hostPlatform)
	require.NoError(t, err)

	// Verify CreateWithPlatform works with custom image (used by both test and production)
	limits := DefaultLimits()
	hostConfig := NewHostConfigWithLimits(limits)

	containerCfg := &container.Config{
		Image:      customImageName, // Custom image, not alpine:3.19
		Cmd:        []string{"sleep", "300"},
		WorkingDir: "/workspace",
	}

	err = c.CreateWithPlatform(ctx, containerCfg, hostConfig, hostPlatform)
	require.NoError(t, err)

	defer func() {
		if cleanupErr := c.Cleanup(ctx); cleanupErr != nil {
			t.Logf("Cleanup warning: %v", cleanupErr)
		}
	}()

	err = c.Start(ctx)
	require.NoError(t, err)

	// Verify ValidateKiroCLI works (used by both test and production)
	err = c.ValidateKiroCLI(ctx, hostPlatform)
	require.NoError(t, err)

	// Note: This is a unit-level validation that the sandbox helpers work correctly
	// in the same sequence used by the production runner (internal/eval/runner.go:562-639).
	// It does not call the runner directly — changes to runner orchestration order
	// should be caught by runner-level integration tests.
	t.Log("✅ Unit validation: sandbox helpers support the generate → build → create → verify flow")
}
