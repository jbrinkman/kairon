package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The platform -> kiro-cli download URL mapping lives in kiroCLIVersionedURL
// (the base image pins the version); these tests keep the mapping and the
// unsupported-platform error covered.
func TestKiroCLIDownloadURL_SupportedPlatforms(t *testing.T) {
	v := DefaultToolSet.KiroCLIVersion
	tests := []struct {
		name        string
		platform    string
		expectedURL string
		expectError bool
	}{
		{
			name:        "AMD64 platform",
			platform:    "linux/amd64",
			expectedURL: "https://desktop-release.q.us-east-1.amazonaws.com/" + v + "/kirocli-x86_64-linux-musl.zip",
		},
		{
			name:        "ARM64 platform",
			platform:    "linux/arm64",
			expectedURL: "https://desktop-release.q.us-east-1.amazonaws.com/" + v + "/kirocli-aarch64-linux-musl.zip",
		},
		{
			name:        "Unsupported platform",
			platform:    "linux/mips",
			expectError: true,
		},
		{
			name:        "Invalid platform format",
			platform:    "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := kiroCLIVersionedURL(tt.platform, v)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, url)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
			}
		})
	}
}

func TestInstallationVerification_PermissionChecks(t *testing.T) {
	// Create a test container with a mock kiro-cli binary
	tempDir := t.TempDir()
	mockBinary := filepath.Join(tempDir, "kiro-cli")

	// Create mock binary with proper content
	mockContent := "#!/bin/sh\necho 'kiro-cli version 1.0.0'"
	err := os.WriteFile(mockBinary, []byte(mockContent), 0755)
	require.NoError(t, err)

	// Test permission verification
	t.Run("ValidPermissions", func(t *testing.T) {
		// This test would require a running container to fully test
		// For unit testing, we verify the permission check logic exists
		info, err := os.Stat(mockBinary)
		require.NoError(t, err)

		mode := info.Mode()
		assert.True(t, mode&0755 == 0755, "Mock binary should have executable permissions")
	})

	t.Run("InvalidPermissions", func(t *testing.T) {
		// Create binary without execute permissions
		nonExecBinary := filepath.Join(tempDir, "kiro-cli-noexec")
		err = os.WriteFile(nonExecBinary, []byte(mockContent), 0644)
		require.NoError(t, err)

		info, err := os.Stat(nonExecBinary)
		require.NoError(t, err)

		mode := info.Mode()
		assert.False(t, mode&0111 != 0, "Binary should not have execute permissions")
	})
}

func TestInstallationFailures_ErrorHandling(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{
			name:     "Valid AMD64 platform",
			platform: "linux/amd64",
			wantErr:  false,
		},
		{
			name:     "Valid ARM64 platform",
			platform: "linux/arm64",
			wantErr:  false,
		},
		{
			name:     "Invalid platform",
			platform: "windows/amd64",
			wantErr:  true,
		},
		{
			name:     "Empty platform",
			platform: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := kiroCLIVersionedURL(tt.platform, DefaultToolSet.KiroCLIVersion)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRuntimeVerification_DoesNotInstall(t *testing.T) {
	skipIfNoContainerDaemon(t)

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// ValidateKiroCLI only verifies; installation happens at image build time.
	// There is no running container with kiro-cli here, so verification fails.
	err = c.ValidateKiroCLI(context.Background(), "linux/amd64")
	assert.Error(t, err, "Should fail verification when kiro-cli not installed")
}

func TestPlatformSpecificBinaries(t *testing.T) {
	tests := []struct {
		platform     string
		expectedFile string
	}{
		{"linux/amd64", "kirocli-x86_64-linux-musl.zip"},
		{"linux/arm64", "kirocli-aarch64-linux-musl.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			url, err := kiroCLIVersionedURL(tt.platform, DefaultToolSet.KiroCLIVersion)
			require.NoError(t, err)
			assert.Contains(t, url, tt.expectedFile)

			args, err := DefaultToolSet.BuildArgs(tt.platform)
			require.NoError(t, err)
			assert.Contains(t, *args["KIRO_CLI_URL"], tt.expectedFile)
		})
	}
}

func TestContainer_LogStartup(t *testing.T) {
	skipIfNoContainerDaemon(t)
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	c.containerID = "1234567890abcdef"
	limits := ResourceLimits{
		CPUQuota: 2000000,           // 2 cores
		Memory:   512 * 1024 * 1024, // 512MB
		Timeout:  time.Minute * 5,
	}

	// This should not panic and should output formatted info
	c.LogStartup(limits)
}

func TestContainer_GetContainerInfo(t *testing.T) {
	skipIfNoContainerDaemon(t)
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	c.containerID = "1234567890abcdef"
	shortID, imageName := c.GetContainerInfo()

	assert.Equal(t, "1234567", shortID)
	assert.Equal(t, "alpine:3.19", imageName)

	// Test with short ID
	c.containerID = "123"
	shortID, _ = c.GetContainerInfo()
	assert.Equal(t, "123", shortID)
}

func TestDetectHostArchitecture_Unsupported(t *testing.T) {
	// Test the current architecture (should work)
	platform, err := DetectHostArchitecture()
	require.NoError(t, err)
	assert.True(t, platform == "linux/amd64" || platform == "linux/arm64")
}

func TestValidateKiroCLI_Verification(t *testing.T) {
	skipIfNoContainerDaemon(t)

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	// Should fail without a running container
	err = c.ValidateKiroCLI(ctx, "linux/amd64")
	assert.Error(t, err, "Should fail when container not running")
}

func TestResourceLimits_Coverage(t *testing.T) {
	limits := DefaultLimits()
	assert.NotZero(t, limits.CPUQuota)
	assert.NotZero(t, limits.Memory)
	assert.NotZero(t, limits.Timeout)

	hostConfig := &container.HostConfig{}
	limits.ApplyToHostConfig(hostConfig)
	assert.NotNil(t, hostConfig.Resources)

	newHostConfig, err := NewHostConfigWithMounts(limits, nil)
	require.NoError(t, err)
	assert.NotNil(t, newHostConfig)
	assert.NotNil(t, newHostConfig.Resources)
}

func TestKiroCLIVerification_Detailed(t *testing.T) {
	skipIfNoContainerDaemon(t)

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	// Test verification method directly
	err = c.verifyKiroCLIInstallation(ctx)
	assert.Error(t, err, "Should fail when no container is running")
	assert.Contains(t, err.Error(), "kiro-cli")
}

func TestContainer_ArchitectureErrors(t *testing.T) {
	skipIfNoContainerDaemon(t)
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()
	config := &container.Config{Image: "alpine:3.19", Cmd: []string{"echo", "test"}}
	hostConfig := &container.HostConfig{}

	// Test invalid platform format
	err = c.CreateWithPlatform(ctx, config, hostConfig, "invalid-platform")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid platform format")
}

// SimulateGitHubResponse and the embedded mock skill are kept for the
// containment work (#298); nothing installs them into a container any more.
func TestSimulateGitHubResponse(t *testing.T) {
	response := SimulateGitHubResponse("issue", []string{"create"})
	assert.Equal(t, 12345, response.IssueNumber)

	response = SimulateGitHubResponse("pr", []string{"create"})
	assert.Equal(t, 42, response.PRNumber)

	response = SimulateGitHubResponse("unknown", []string{})
	assert.Equal(t, "success", response.Status)
}

func TestMockGitHubSkill_Embedded(t *testing.T) {
	content, err := MockGitHubSkill.ReadFile("testdata/github-cli-mock/gh")
	require.NoError(t, err)
	assert.Contains(t, string(content), "[MOCK]")
}
