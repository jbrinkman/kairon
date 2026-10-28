package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	ctx := context.Background()

	// ValidateKiroCLI only verifies; installation happens at image build time.
	// There is no running container with kiro-cli here, so verification fails.
	err = c.ValidateKiroCLI(ctx, "linux/amd64")
	assert.Error(t, err, "Should fail verification when kiro-cli not installed")

	t.Run("verifyKiroCLIInstallation error names kiro-cli", func(t *testing.T) {
		err := c.verifyKiroCLIInstallation(ctx)
		require.Error(t, err, "Should fail when no container is running")
		assert.Contains(t, err.Error(), "kiro-cli")
	})
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
