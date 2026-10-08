package eval

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/stretchr/testify/require"
)

// The sandbox image is the persistent tools-only base image, so the flow worth
// measuring is: ensure image (reused after the first build) -> create -> start
// -> verify. These checks build the real base image (downloads kiro-cli and
// gh), so they only run with a container daemon and KAIRON_EVAL_SANDBOX_SELFTEST=1.
func skipUnlessBaseImageGate(tb testing.TB) {
	tb.Helper()
	if testing.Short() {
		tb.Skip("Skipping performance check in short mode")
	}
	if os.Getenv(sandboxSelftestEnv) != "1" {
		tb.Skipf("set %s=1 (see `task eval:selftest:sandbox`) to build the base image", sandboxSelftestEnv)
	}
	if err := checkDockerAvailability(); err != nil {
		tb.Skipf("container daemon not available (Podman or Docker): %v", err)
	}
}

// BenchmarkBaseImageFlow measures ensure-image (a cache hit after the first
// iteration) plus container create/start/verify.
func BenchmarkBaseImageFlow(b *testing.B) {
	skipUnlessBaseImageGate(b)

	hostPlatform, err := sandbox.DetectHostArchitecture()
	require.NoError(b, err)

	im, err := sandbox.NewImageManager("", false)
	require.NoError(b, err)
	defer im.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ensureStart := time.Now()
		tag, built, err := im.EnsureBaseImage(ctx, hostPlatform)
		require.NoError(b, err)
		ensureDuration := time.Since(ensureStart)

		createStart := time.Now()
		c, err := sandbox.NewContainer("")
		require.NoError(b, err)

		hostConfig, err := sandbox.NewHostConfigWithMounts(sandbox.DefaultLimits(), nil)
		require.NoError(b, err)
		err = c.CreateWithPlatform(ctx, &container.Config{
			Image:      tag,
			Cmd:        []string{"sleep", "3600"},
			Env:        []string{"KIRO_CLI_DISABLE_TELEMETRY=1"},
			WorkingDir: "/workspace",
		}, hostConfig, hostPlatform)
		require.NoError(b, err)
		require.NoError(b, c.Start(ctx))
		createDuration := time.Since(createStart)

		verifyStart := time.Now()
		require.NoError(b, c.ValidateKiroCLI(ctx, hostPlatform))
		verifyDuration := time.Since(verifyStart)

		b.StopTimer()
		b.Logf("iteration %d: ensure=%v (built=%v) create=%v verify=%v", i+1, ensureDuration, built, createDuration, verifyDuration)
		if cleanupErr := c.Cleanup(ctx); cleanupErr != nil {
			b.Logf("Cleanup warning: %v", cleanupErr)
		}
		c.Close()
		b.StartTimer()
	}
}

// TestPerformanceRegression runs the whole flow once and logs (never fails on)
// wall-clock thresholds, which are environment-dependent.
func TestPerformanceRegression(t *testing.T) {
	skipUnlessBaseImageGate(t)

	hostPlatform, err := sandbox.DetectHostArchitecture()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	start := time.Now()

	im, err := sandbox.NewImageManager("", false)
	require.NoError(t, err)
	defer im.Close()

	tag, built, err := im.EnsureBaseImage(ctx, hostPlatform)
	require.NoError(t, err)

	c, err := sandbox.NewContainer("")
	require.NoError(t, err)
	defer c.Close()

	hostConfig, err := sandbox.NewHostConfigWithMounts(sandbox.DefaultLimits(), nil)
	require.NoError(t, err)
	err = c.CreateWithPlatform(ctx, &container.Config{
		Image:      tag,
		Cmd:        []string{"sleep", "3600"},
		Env:        []string{"KIRO_CLI_DISABLE_TELEMETRY=1"},
		WorkingDir: "/workspace",
	}, hostConfig, hostPlatform)
	require.NoError(t, err)
	defer func() {
		if cleanupErr := c.Cleanup(ctx); cleanupErr != nil {
			t.Logf("Cleanup warning: %v", cleanupErr)
		}
	}()

	require.NoError(t, c.Start(ctx))
	require.NoError(t, c.ValidateKiroCLI(ctx, hostPlatform))

	totalTime := time.Since(start)
	if built {
		t.Logf("base image %s was built in this run (first run on this machine)", tag)
	} else if totalTime > 2*time.Minute {
		t.Logf("⚠️ Warning: flow took %v with a cached base image (>2 minutes)", totalTime)
	}
	t.Logf("✅ flow finished in %v (image %s, built=%v)", totalTime, tag, built)
}
