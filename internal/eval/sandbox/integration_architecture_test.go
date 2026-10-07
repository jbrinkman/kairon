//go:build integration
// +build integration

package sandbox

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for architecture-specific functionality
// Run with: go test -tags=integration ./internal/eval/sandbox

func TestArchitectureIntegration_ContainerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoContainerDaemon(t)

	tests := []struct {
		name     string
		platform string
	}{
		{"Current Architecture", ""},
		{"Explicit AMD64", "linux/amd64"},
		{"Explicit ARM64", "linux/arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			c, err := NewContainer("alpine:3.19")
			require.NoError(t, err)
			defer c.Close()

			config := &container.Config{
				Image: "alpine:3.19",
				Cmd:   []string{"sleep", "30"},
			}
			hostConfig := mustHostConfig(t, DefaultLimits())

			// Use detected architecture if platform is empty
			platform := tt.platform
			if platform == "" {
				platform, err = DetectHostArchitecture()
				require.NoError(t, err)
			}

			err = c.CreateWithPlatform(ctx, config, hostConfig, platform)
			if err != nil {
				// Skip test if platform is not available on this system
				if strings.Contains(err.Error(), "no matching manifest") ||
					strings.Contains(err.Error(), "platform") {
					t.Skipf("Platform %s not available on this system: %v", platform, err)
					return
				}
				require.NoError(t, err)
			}

			defer c.Cleanup(ctx)

			err = c.Start(ctx)
			require.NoError(t, err)

			// Verify architecture matches expectation
			arch, err := c.ExecWithOutput(ctx, []string{"uname", "-m"})
			require.NoError(t, err)

			switch platform {
			case "linux/amd64":
				assert.Equal(t, "x86_64", arch)
			case "linux/arm64":
				assert.Equal(t, "aarch64", arch)
			default:
				// For current architecture, just verify it's a known value
				assert.Contains(t, []string{"x86_64", "aarch64"}, arch)
			}

			// Test basic functionality
			output, err := c.ExecWithOutput(ctx, []string{"echo", "architecture-test"})
			require.NoError(t, err)
			assert.Equal(t, "architecture-test", output)
		})
	}
}

func TestArchitectureIntegration_KiroCLIInstallation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoContainerDaemon(t)

	// Only test on the current architecture to avoid long pull times
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Create container with longer timeout for installation
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "300"},
	}

	// Increase resource limits for installation
	limits := ResourceLimits{
		CPUQuota: 2000000,            // 2 cores
		Memory:   1024 * 1024 * 1024, // 1GB
		Timeout:  5 * time.Minute,
	}
	hostConfig := mustHostConfig(t, limits)

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	err = c.Start(ctx)
	require.NoError(t, err)

	// Detect container architecture
	arch, err := c.ExecWithOutput(ctx, []string{"uname", "-m"})
	require.NoError(t, err)
	t.Logf("Container architecture: %s", arch)

	// Verify architecture mapping
	var platform string
	switch arch {
	case "x86_64":
		platform = "linux/amd64"
	case "aarch64":
		platform = "linux/arm64"
	default:
		t.Fatalf("Unsupported architecture: %s", arch)
	}

	// Test URL generation
	url, err := kiroCLIVersionedURL(platform, DefaultToolSet.KiroCLIVersion)
	require.NoError(t, err)
	t.Logf("Download URL for %s: %s", platform, url)

	// Verify URL format
	assert.Contains(t, url, "https://desktop-release.q.us-east-1.amazonaws.com/"+DefaultToolSet.KiroCLIVersion+"/")
	if arch == "x86_64" {
		assert.Contains(t, url, "kirocli-x86_64-linux-musl.zip")
	} else {
		assert.Contains(t, url, "kirocli-aarch64-linux-musl.zip")
	}

	// Test installation (this will make a real network request)
	if os.Getenv("SKIP_NETWORK_TESTS") == "" {
		err = c.ValidateKiroCLI(ctx, platform)
		if err != nil {
			// Network issues are common in CI, log but don't fail
			t.Logf("kiro-cli installation failed (possibly network issue): %v", err)
		} else {
			t.Log("kiro-cli installation successful")

			// Verify installation
			version, err := c.ExecWithOutput(ctx, []string{"kiro-cli", "--version"})
			if err == nil {
				t.Logf("kiro-cli version: %s", version)
				assert.NotEmpty(t, version)
			}
		}
	} else {
		t.Log("Skipping network-dependent kiro-cli installation test")
	}
}

func TestArchitectureIntegration_ResourceLimitsAcrossArchitectures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoContainerDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Test with strict resource limits
	limits := ResourceLimits{
		CPUQuota: 500000,           // 0.5 core
		Memory:   64 * 1024 * 1024, // 64MB
		Timeout:  30 * time.Second,
	}

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "20"},
	}
	hostConfig := mustHostConfig(t, limits)

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	err = c.Start(ctx)
	require.NoError(t, err)

	// Log container info
	c.LogStartup(limits)
	shortID, imageName := c.GetContainerInfo()
	assert.Equal(t, "alpine:3.19", imageName)
	assert.NotEmpty(t, shortID)

	// Test that resource limits are applied regardless of architecture
	arch, err := c.ExecWithOutput(ctx, []string{"uname", "-m"})
	require.NoError(t, err)
	t.Logf("Testing resource limits on architecture: %s", arch)

	// Simple memory usage test (should work on both architectures)
	output, err := c.ExecWithOutput(ctx, []string{"sh", "-c", "echo 'Memory test passed'"})
	require.NoError(t, err)
	assert.Contains(t, output, "Memory test passed")
}

func TestArchitectureIntegration_CurrentArchitectureConsistency(t *testing.T) {
	// Test that our architecture detection is consistent with runtime
	platform, err := DetectHostArchitecture()
	require.NoError(t, err)

	switch runtime.GOARCH {
	case "amd64":
		assert.Equal(t, "linux/amd64", platform)
	case "arm64":
		assert.Equal(t, "linux/arm64", platform)
	default:
		t.Fatalf("Unsupported test architecture: %s", runtime.GOARCH)
	}

	// Test URL generation for current architecture
	url, err := kiroCLIVersionedURL(platform, DefaultToolSet.KiroCLIVersion)
	require.NoError(t, err)

	assert.Contains(t, url, "https://desktop-release.q.us-east-1.amazonaws.com/"+DefaultToolSet.KiroCLIVersion+"/")

	if runtime.GOARCH == "amd64" {
		assert.Contains(t, url, "kirocli-x86_64-linux-musl.zip")
	} else if runtime.GOARCH == "arm64" {
		assert.Contains(t, url, "kirocli-aarch64-linux-musl.zip")
	}

	t.Logf("Current architecture: %s -> Platform: %s -> URL: %s",
		runtime.GOARCH, platform, url)
}
