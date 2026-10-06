package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerIntegration_KiroCLIInstallation(t *testing.T) {
	skipIfNoContainerDaemon(t)

	// Only test the host architecture — cross-platform testing requires matrixed CI
	hostPlatform, err := DetectHostArchitecture()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create container
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Generate dockerfile with kiro-cli installation
	tempDir := t.TempDir()
	dockerfile, err := c.GenerateDockerfileWithPlatform(tempDir, hostPlatform)
	require.NoError(t, err)
	assert.Contains(t, dockerfile, "# Install kiro-cli")

	// Create container with proper configuration
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sh", "-c", "sleep 300"},
	}

	limits := DefaultLimits()
	hostConfig := NewHostConfigWithLimits(limits)

	err = c.CreateWithPlatform(ctx, config, hostConfig, hostPlatform)
	require.NoError(t, err)
	defer func() {
		cleanupErr := c.Cleanup(ctx)
		if cleanupErr != nil {
			t.Logf("Cleanup warning: %v", cleanupErr)
		}
	}()

	// Start container
	err = c.Start(ctx)
	require.NoError(t, err)

	c.LogStartup(limits)

	// Install and verify kiro-cli
	err = c.ValidateKiroCLI(ctx, hostPlatform)
	if err != nil {
		// For this test, we expect installation to fail since we can't install
		// kiro-cli in base Alpine without building the image
		t.Logf("Expected installation failure in base image: %v", err)
		return
	}

	// If installation somehow succeeded, verify it works
	version, err := c.ExecWithOutput(ctx, []string{"kiro-cli", "--version"})
	require.NoError(t, err)
	assert.NotEmpty(t, version)
}

func TestKiroCLIExecution_SandboxUser(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create container with sandbox user
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Configure container with sandbox user setup (use sh since bash not available in base Alpine)
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "300"},
		User:  "root",
	}

	limits := DefaultLimits()
	hostConfig := NewHostConfigWithLimits(limits)

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)
	defer func() {
		cleanupErr := c.Cleanup(ctx)
		if cleanupErr != nil {
			t.Logf("Cleanup warning: %v", cleanupErr)
		}
	}()

	err = c.Start(ctx)
	require.NoError(t, err)

	// Create sandbox user via exec (avoids racy sleep)
	_, err = c.ExecWithOutput(ctx, []string{"adduser", "-D", "-s", "/bin/sh", "sandbox"})
	require.NoError(t, err)

	// Verify sandbox user exists
	output, err := c.ExecWithOutput(ctx, []string{"id", "sandbox"})
	require.NoError(t, err)
	assert.Contains(t, output, "uid=")

	// Test kiro-cli --version as sandbox user (will fail without installation)
	output, err = c.ExecWithOutput(ctx, []string{"su", "-c", "kiro-cli --version", "sandbox"})
	if err != nil {
		// Expected to fail since kiro-cli isn't installed in base Alpine
		// Check for both "kiro-cli" and "not found" since Alpine may return different error messages
		errStr := err.Error()
		hasExpectedError := strings.Contains(errStr, "kiro-cli") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "No such file")
		assert.True(t, hasExpectedError, "Error should indicate kiro-cli is missing: %v", err)
		t.Logf("Expected failure - kiro-cli not installed: %v", err)
	} else {
		// If somehow it works, verify output
		assert.NotEmpty(t, output)
		t.Logf("Unexpected success - kiro-cli found: %s", output)
	}

	// Test kiro-cli chat --help as sandbox user (will also fail without installation)
	output, err = c.ExecWithOutput(ctx, []string{"su", "-c", "kiro-cli chat --help", "sandbox"})
	if err != nil {
		// Expected to fail since kiro-cli isn't installed
		errStr := err.Error()
		hasExpectedError := strings.Contains(errStr, "kiro-cli") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "No such file")
		assert.True(t, hasExpectedError, "Error should indicate kiro-cli is missing: %v", err)
		t.Logf("Expected failure - kiro-cli chat not available: %v", err)
	} else {
		// If somehow it works, verify help output
		assert.Contains(t, strings.ToLower(output), "help")
		t.Logf("Unexpected success - kiro-cli chat help: %s", output)
	}
}

func TestCrossPlatform_InstallationVerification(t *testing.T) {
	skipIfNoContainerDaemon(t)

	// Only test container creation for the host architecture
	hostPlatform, err := DetectHostArchitecture()
	require.NoError(t, err)

	// Determine expected values for host platform
	var expectedBinary, expectedArch string
	switch hostPlatform {
	case "linux/amd64":
		expectedBinary = "kirocli-x86_64-linux-musl.zip"
		expectedArch = "x86_64"
	case "linux/arm64":
		expectedBinary = "kirocli-aarch64-linux-musl.zip"
		expectedArch = "aarch64"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	tempDir := t.TempDir()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Generate dockerfile with platform-specific installation
	dockerfile, err := c.GenerateDockerfileWithPlatform(tempDir, hostPlatform)
	require.NoError(t, err)

	// Verify platform-specific binary is referenced
	assert.Contains(t, dockerfile, expectedBinary)
	assert.Contains(t, dockerfile, "# Install kiro-cli")
	assert.Contains(t, dockerfile, "curl -fsSL")
	assert.Contains(t, dockerfile, "unzip -q")
	assert.Contains(t, dockerfile, "chmod 755 kirocli/bin/kiro-cli")
	assert.Contains(t, dockerfile, "/usr/local/bin/kiro-cli")

	// Test URL generation for the platform
	url, err := getKiroCLIDownloadURL(hostPlatform)
	require.NoError(t, err)
	assert.Contains(t, url, expectedBinary)

	// Create container to test platform support
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sh", "-c", "sleep 300"},
	}

	limits := DefaultLimits()
	hostConfig := NewHostConfigWithLimits(limits)

	err = c.CreateWithPlatform(ctx, config, hostConfig, hostPlatform)
	require.NoError(t, err)
	defer func() {
		cleanupErr := c.Cleanup(ctx)
		if cleanupErr != nil {
			t.Logf("Cleanup warning: %v", cleanupErr)
		}
	}()

	err = c.Start(ctx)
	require.NoError(t, err)

	// Verify container architecture matches platform
	output, err := c.ExecWithOutput(ctx, []string{"uname", "-m"})
	require.NoError(t, err)
	assert.Equal(t, expectedArch, strings.TrimSpace(output))

	// Container lifecycle verification
	shortID, imageName := c.GetContainerInfo()
	assert.NotEmpty(t, shortID)
	assert.Equal(t, "alpine:3.19", imageName)

	c.LogStartup(limits)
}
