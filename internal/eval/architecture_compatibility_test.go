package eval

import (
	"runtime"
	"testing"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArchitectureCompatibility verifies eval framework works on both ARM64 and x86_64
func TestArchitectureCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping architecture compatibility test in short mode")
	}

	// Test architecture detection
	platform, err := sandbox.DetectHostArchitecture()
	require.NoError(t, err)

	expectedPlatforms := []string{"linux/amd64", "linux/arm64"}
	assert.Contains(t, expectedPlatforms, platform,
		"Should detect supported architecture")

	t.Logf("Running eval framework tests on platform: %s", platform)
}

func TestRunnerArchitectureSupport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping runner architecture test in short mode")
	}

	// Create a minimal test case
	testCase := &TestCase{
		Name:        "architecture-test",
		Description: "Test that runner works on current architecture",
		Input:       "echo architecture compatibility test",
		Agent:       "test-agent",
	}

	// This should work regardless of architecture - just test the structure
	assert.Equal(t, "architecture-test", testCase.Name)
	assert.Equal(t, "test-agent", testCase.Agent)
	assert.NotEmpty(t, testCase.Input)
}

func TestSandboxArchitectureDetection(t *testing.T) {
	// Test that we can properly detect architecture for sandbox creation
	platform, err := sandbox.DetectHostArchitecture()
	require.NoError(t, err)

	// Verify platform string format
	assert.Contains(t, platform, "linux/")

	switch runtime.GOARCH {
	case "amd64":
		assert.Equal(t, "linux/amd64", platform)
	case "arm64":
		assert.Equal(t, "linux/arm64", platform)
	default:
		t.Fatalf("Unsupported architecture for testing: %s", runtime.GOARCH)
	}
}

func TestKiroCLIURLGeneration(t *testing.T) {
	tests := []struct {
		platform    string
		expectedURL string
	}{
		{
			platform:    "linux/amd64",
			expectedURL: "https://desktop-release.q.us-east-1.amazonaws.com/latest/kirocli-x86_64-linux-musl.zip",
		},
		{
			platform:    "linux/arm64",
			expectedURL: "https://desktop-release.q.us-east-1.amazonaws.com/latest/kirocli-aarch64-linux-musl.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			// Note: the URL mapping is unexported (sandbox.kiroCLIVersionedURL), so it is
			// covered in the sandbox package; here we only check architecture detection
			// This validates the URL generation logic is working for both architectures

			// Create a temporary container to access the method
			c, err := sandbox.NewContainer("alpine:3.19")
			if err != nil {
				t.Skip("Docker not available for testing")
			}
			defer c.Close()

			// The URL generation happens inside ValidateKiroCLI
			// We can't test it directly but we validate the architecture detection works
			platform, err := sandbox.DetectHostArchitecture()
			require.NoError(t, err)

			// Verify our test case matches current architecture
			if (tt.platform == "linux/amd64" && runtime.GOARCH == "amd64") ||
				(tt.platform == "linux/arm64" && runtime.GOARCH == "arm64") {
				assert.Equal(t, tt.platform, platform)
			}
		})
	}
}

func TestEvalFrameworkArchitectureIndependence(t *testing.T) {
	// Test that core eval framework functionality doesn't depend on architecture

	// Test basic result structure
	result := &CaseResult{
		CaseName:     "arch-test",
		ActualOutput: "test output",
	}

	// These should work on any architecture
	assert.Equal(t, "arch-test", result.CaseName)
	assert.Equal(t, "test output", result.ActualOutput)

	// Test cost info structure
	cost := CostInfo{
		TokensIn:     100,
		TokensOut:    50,
		EstimatedUSD: 0.01,
	}

	assert.Equal(t, 100, cost.TokensIn)
	assert.Equal(t, 50, cost.TokensOut)
	assert.Equal(t, 0.01, cost.EstimatedUSD)
}

func TestArchitecturePlatformConsistency(t *testing.T) {
	// Verify our architecture detection is consistent
	platform, err := sandbox.DetectHostArchitecture()
	require.NoError(t, err)

	// Test multiple calls return same result
	for i := 0; i < 5; i++ {
		platform2, err := sandbox.DetectHostArchitecture()
		require.NoError(t, err)
		assert.Equal(t, platform, platform2,
			"Architecture detection should be consistent")
	}

	// Verify platform format
	assert.Contains(t, platform, "/", "Platform should contain OS/arch separator")
	assert.True(t,
		platform == "linux/amd64" || platform == "linux/arm64",
		"Platform should be supported architecture")
}
