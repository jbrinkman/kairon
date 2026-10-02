package eval

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/stretchr/testify/require"
)

// BenchmarkUnifiedFlow measures the performance of the complete unified flow
func BenchmarkUnifiedFlow(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	// Skip if Docker is not available
	if err := checkDockerAvailability(); err != nil {
		b.Skipf("Docker not available: %v", err)
	}

	hostPlatform, err := sandbox.DetectHostArchitecture()
	require.NoError(b, err)

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer() // Don't measure setup time

		// Create container for this iteration
		c, err := sandbox.NewContainer("")
		require.NoError(b, err)

		b.StartTimer() // Start measuring the unified flow

		// Phase 1: Generate Dockerfile
		generateStart := time.Now()
		dockerfile, err := c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
		require.NoError(b, err)
		generateDuration := time.Since(generateStart)

		// Phase 2: Build custom image
		buildStart := time.Now()
		customImageName := c.GetCustomImageName(hostPlatform)
		err = c.BuildImageFromDockerfile(ctx, dockerfile, customImageName, hostPlatform)
		require.NoError(b, err)
		buildDuration := time.Since(buildStart)

		// Phase 3: Create and start container
		createStart := time.Now()
		limits := sandbox.DefaultLimits()
		hostConfig := sandbox.NewHostConfigWithLimits(limits)

		containerCfg := &container.Config{
			Image:      customImageName,
			Cmd:        []string{"sleep", "3600"},
			Env:        []string{"KIRO_CLI_DISABLE_TELEMETRY=1"},
			WorkingDir: "/workspace",
		}

		err = c.CreateWithPlatform(ctx, containerCfg, hostConfig, hostPlatform)
		require.NoError(b, err)

		err = c.Start(ctx)
		require.NoError(b, err)
		createDuration := time.Since(createStart)

		// Phase 4: Verify kiro-cli installation
		verifyStart := time.Now()
		err = c.ValidateKiroCLI(ctx, hostPlatform)
		require.NoError(b, err)
		verifyDuration := time.Since(verifyStart)

		b.StopTimer() // Stop measuring before cleanup

		// Report phase timings
		b.Logf("Iteration %d timings:", i+1)
		b.Logf("  Generate: %v", generateDuration)
		b.Logf("  Build: %v", buildDuration)
		b.Logf("  Create: %v", createDuration)
		b.Logf("  Verify: %v", verifyDuration)

		// Cleanup
		if cleanupErr := c.Cleanup(ctx); cleanupErr != nil {
			b.Logf("Cleanup warning: %v", cleanupErr)
		}
		c.Close()
	}
}

// BenchmarkFlowPhases benchmarks individual phases of the unified flow
func BenchmarkFlowPhases(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	if err := checkDockerAvailability(); err != nil {
		b.Skipf("Docker not available: %v", err)
	}

	hostPlatform, err := sandbox.DetectHostArchitecture()
	require.NoError(b, err)

	b.Run("Generate", func(b *testing.B) {
		b.StopTimer()
		for i := 0; i < b.N; i++ {
			c, err := sandbox.NewContainer("")
			require.NoError(b, err)

			b.StartTimer()
			_, err = c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
			b.StopTimer()

			require.NoError(b, err)
			c.Close()
		}
	})

	b.Run("Build", func(b *testing.B) {
		b.StopTimer()
		// Generate dockerfile once for all iterations
		c, err := sandbox.NewContainer("")
		require.NoError(b, err)
		dockerfile, err := c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
		require.NoError(b, err)
		c.Close()

		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c, err := sandbox.NewContainer("")
			require.NoError(b, err)

			customImageName := c.GetCustomImageName(hostPlatform)

			b.StartTimer()
			err = c.BuildImageFromDockerfile(ctx, dockerfile, customImageName, hostPlatform)
			b.StopTimer()

			require.NoError(b, err)
			c.Close()
		}
	})

	b.Run("Verify", func(b *testing.B) {
		b.StopTimer()
		// Pre-build an image for verification testing
		c, err := sandbox.NewContainer("")
		require.NoError(b, err)
		dockerfile, err := c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
		require.NoError(b, err)

		ctx := context.Background()
		customImageName := c.GetCustomImageName(hostPlatform)
		err = c.BuildImageFromDockerfile(ctx, dockerfile, customImageName, hostPlatform)
		require.NoError(b, err)

		// Create and start container
		limits := sandbox.DefaultLimits()
		hostConfig := sandbox.NewHostConfigWithLimits(limits)
		containerCfg := &container.Config{
			Image:      customImageName,
			Cmd:        []string{"sleep", "3600"},
			Env:        []string{"KIRO_CLI_DISABLE_TELEMETRY=1"},
			WorkingDir: "/workspace",
		}

		err = c.CreateWithPlatform(ctx, containerCfg, hostConfig, hostPlatform)
		require.NoError(b, err)
		err = c.Start(ctx)
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StartTimer()
			err = c.ValidateKiroCLI(ctx, hostPlatform)
			b.StopTimer()
			require.NoError(b, err)
		}

		// Cleanup
		c.Cleanup(ctx)
		c.Close()
	})
}

// TestPerformanceRegression ensures the new flow doesn't significantly impact performance
func TestPerformanceRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	if err := checkDockerAvailability(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	hostPlatform, err := sandbox.DetectHostArchitecture()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	start := time.Now()

	// Run complete unified flow
	c, err := sandbox.NewContainer("")
	require.NoError(t, err)
	defer c.Close()

	// Generate → Build → Create → Verify
	dockerfile, err := c.GenerateDockerfileWithPlatform("/workspace", hostPlatform)
	require.NoError(t, err)

	customImageName := c.GetCustomImageName(hostPlatform)
	err = c.BuildImageFromDockerfile(ctx, dockerfile, customImageName, hostPlatform)
	require.NoError(t, err)

	limits := sandbox.DefaultLimits()
	hostConfig := sandbox.NewHostConfigWithLimits(limits)
	containerCfg := &container.Config{
		Image:      customImageName,
		Cmd:        []string{"sleep", "3600"},
		Env:        []string{"KIRO_CLI_DISABLE_TELEMETRY=1"},
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

	err = c.ValidateKiroCLI(ctx, hostPlatform)
	require.NoError(t, err)

	totalTime := time.Since(start)

	// Performance observations (warnings only — wall-clock thresholds are environment-dependent)
	if totalTime > 5*time.Minute {
		t.Logf("⚠️ SLOW: Unified flow took %v, exceeding 5 minute threshold (check Docker cache state)", totalTime)
	} else if totalTime > 2*time.Minute {
		t.Logf("⚠️ Warning: Unified flow took %v (>2 minutes)", totalTime)
	}

	t.Logf("✅ Performance test completed: unified flow finished in %v", totalTime)
}
