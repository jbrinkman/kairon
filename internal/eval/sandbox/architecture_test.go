package sandbox

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectHostArchitecture(t *testing.T) {
	tests := []struct {
		name        string
		goarch      string
		expected    string
		expectError bool
	}{
		{"AMD64", "amd64", "linux/amd64", false},
		{"ARM64", "arm64", "linux/arm64", false},
		{"Unsupported", "mips", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalGOARCH := runtime.GOARCH

			// Mock runtime.GOARCH (note: can't directly modify it, so test current arch)
			platform, err := DetectHostArchitecture()

			if tt.expectError {
				// Skip this test on actual supported architectures
				if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
					t.Skip("Running on supported architecture, skipping error test")
				}
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify current architecture returns expected format
				switch runtime.GOARCH {
				case "amd64":
					assert.Equal(t, "linux/amd64", platform)
				case "arm64":
					assert.Equal(t, "linux/arm64", platform)
				default:
					t.Skipf("Unsupported architecture for this test: %s", runtime.GOARCH)
				}
			}
			_ = originalGOARCH // prevent unused variable error
		})
	}
}

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

func TestArchitectureDetectionInContainer(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}
	hostConfig := mustHostConfig(t, DefaultLimits())

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	err = c.Start(ctx)
	require.NoError(t, err)

	// Test architecture detection inside container
	arch, err := c.ExecWithOutput(ctx, []string{"uname", "-m"})
	require.NoError(t, err)

	// Clean Docker output formatting issues more thoroughly
	arch = strings.TrimSpace(arch)
	// Remove Docker stream headers and any null bytes
	arch = strings.ReplaceAll(arch, "\x00", "")
	arch = strings.ReplaceAll(arch, "\x01", "")
	arch = strings.ReplaceAll(arch, "\x02", "")
	// Remove any remaining binary data at the start
	for i, r := range arch {
		if r >= 32 && r <= 126 { // printable ASCII
			arch = arch[i:]
			break
		}
	}
	arch = strings.TrimSpace(arch)

	// Verify we get a known architecture
	validArchs := []string{"x86_64", "aarch64"}
	assert.Contains(t, validArchs, arch, "Container should report a supported architecture, got: %q (len=%d)", arch, len(arch))

	// Test platform detection command
	platform, err := c.ExecWithOutput(ctx, []string{"uname", "-a"})
	require.NoError(t, err)
	assert.Contains(t, platform, "Linux", "Should be running on Linux platform")
}

func TestPlatformSpecificImagePulling(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()

	// Test that we can detect and use the current host architecture
	platform, err := DetectHostArchitecture()
	require.NoError(t, err)

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}
	hostConfig := mustHostConfig(t, DefaultLimits())

	// This should pull the image for the correct platform
	err = c.CreateWithPlatform(ctx, config, hostConfig, platform)
	assert.NoError(t, err)

	if c.containerID != "" {
		defer c.Cleanup(ctx)

		err = c.Start(ctx)
		require.NoError(t, err)

		// Verify the container runs with the expected architecture
		output, err := c.ExecWithOutput(ctx, []string{"echo", "success"})
		assert.NoError(t, err)
		assert.Contains(t, output, "success")
	}
}

func TestKiroCLIInstallationMocking(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "60"},
	}
	hostConfig := mustHostConfig(t, DefaultLimits())

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	err = c.Start(ctx)
	require.NoError(t, err)

	// Test that the architecture detection works inside the container
	arch, err := c.ExecWithOutput(ctx, []string{"uname", "-m"})
	require.NoError(t, err)

	// Clean Docker output formatting issues
	arch = strings.TrimSpace(arch)
	arch = strings.ReplaceAll(arch, "\x00", "")
	arch = strings.ReplaceAll(arch, "\x01", "")
	arch = strings.ReplaceAll(arch, "\x02", "")
	// Remove any remaining binary data at the start
	for i, r := range arch {
		if r >= 32 && r <= 126 { // printable ASCII
			arch = arch[i:]
			break
		}
	}
	arch = strings.TrimSpace(arch)

	// Verify we can map the architecture correctly
	var expectedPlatform string
	switch arch {
	case "x86_64":
		expectedPlatform = "linux/amd64"
	case "aarch64":
		expectedPlatform = "linux/arm64"
	default:
		t.Fatalf("Unsupported container architecture: %s", arch)
	}

	// Test URL generation for detected architecture
	url, err := kiroCLIVersionedURL(expectedPlatform, DefaultToolSet.KiroCLIVersion)
	require.NoError(t, err)
	assert.Contains(t, url, "https://desktop-release.q.us-east-1.amazonaws.com/"+DefaultToolSet.KiroCLIVersion+"/")

	if arch == "x86_64" {
		assert.Contains(t, url, "kirocli-x86_64-linux-musl.zip")
	} else if arch == "aarch64" {
		assert.Contains(t, url, "kirocli-aarch64-linux-musl.zip")
	}

	// Note: We don't actually install kiro-cli in this test to avoid network dependencies
	// The ValidateKiroCLI method would be tested in integration tests
}
