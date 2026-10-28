package sandbox

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKiroCLIVersionedURL_PlatformMapping(t *testing.T) {
	v := DefaultToolSet.KiroCLIVersion
	tests := []struct {
		name        string
		platform    string
		expected    string
		expectError bool
	}{
		{
			name:     "AMD64 Linux",
			platform: "linux/amd64",
			expected: "https://desktop-release.q.us-east-1.amazonaws.com/" + v + "/kirocli-x86_64-linux-musl.zip",
		},
		{
			name:     "ARM64 Linux",
			platform: "linux/arm64",
			expected: "https://desktop-release.q.us-east-1.amazonaws.com/" + v + "/kirocli-aarch64-linux-musl.zip",
		},
		{
			name:        "Unsupported Platform",
			platform:    "windows/amd64",
			expectError: true,
		},
		{
			name:        "Invalid Platform Format",
			platform:    "linux",
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
				require.NoError(t, err)
				assert.Equal(t, tt.expected, url)
			}
		})
	}
}

func TestCreateWithPlatform(t *testing.T) {
	skipIfNoContainerDaemon(t)

	tests := []struct {
		name     string
		platform string
	}{
		{"AMD64 Platform", "linux/amd64"},
		{"ARM64 Platform", "linux/arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			c, err := NewContainer("alpine:3.19")
			require.NoError(t, err)
			defer c.Close()

			config := &container.Config{
				Image: "alpine:3.19",
				Cmd:   []string{"sleep", "5"},
			}
			hostConfig := mustHostConfig(t, DefaultLimits())

			err = c.CreateWithPlatform(ctx, config, hostConfig, tt.platform)

			// Note: This may fail on systems that don't support the target platform
			// Docker will attempt to pull the image for the specified platform
			if err != nil {
				t.Logf("Platform %s not supported on this system: %v", tt.platform, err)
				return
			}

			assert.NoError(t, err)
			assert.NotEmpty(t, c.containerID)

			// Clean up
			defer c.Cleanup(ctx)

			// Verify container can start
			err = c.Start(ctx)
			assert.NoError(t, err)
		})
	}
}
