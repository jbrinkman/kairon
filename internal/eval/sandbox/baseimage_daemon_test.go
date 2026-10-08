package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBaseImage_ToolsOnlyNoMounts (AC 1) starts a container from the base
// image with NO mounts and checks what the image alone provides: the
// unprivileged sandbox user, the stable tool set, and an empty /workspace with
// no .kiro or Kairon/agent configuration baked in.
//
// Gated: it builds the real base image (downloads kiro-cli and gh), so it only
// runs with a Podman or Docker daemon and KAIRON_EVAL_SANDBOX_SELFTEST=1.
func TestBaseImage_ToolsOnlyNoMounts(t *testing.T) {
	if os.Getenv("KAIRON_EVAL_SANDBOX_SELFTEST") != "1" {
		t.Skip("set KAIRON_EVAL_SANDBOX_SELFTEST=1 (see `task eval:selftest:sandbox`) to build the base image")
	}
	skipIfNoContainerDaemon(t)

	platform, err := DetectHostArchitecture()
	require.NoError(t, err)

	im, err := NewImageManager("base-image-tools-only", false)
	require.NoError(t, err)
	defer im.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	tag, _, err := im.EnsureBaseImage(ctx, platform)
	require.NoError(t, err)

	c, err := NewContainer("")
	require.NoError(t, err)
	defer c.Close()

	hostConfig, err := NewHostConfigWithMounts(DefaultLimits(), nil) // no mounts
	require.NoError(t, err)
	require.Empty(t, hostConfig.Mounts, "this test must run without any mounts")
	require.Empty(t, hostConfig.Binds, "this test must run without any binds")

	require.NoError(t, c.CreateWithPlatform(ctx, &container.Config{
		Image: tag,
		Cmd:   []string{"sleep", "600"},
	}, hostConfig, platform))
	defer func() { _ = c.Cleanup(context.Background()) }()
	require.NoError(t, c.Start(ctx))

	sh := func(script string) string {
		t.Helper()
		out, err := c.ExecWithOutput(ctx, []string{"sh", "-c", script})
		require.NoError(t, err, "script %q", script)
		return out
	}

	// Inspect the pristine filesystem first: running kiro-cli or gh below may
	// create their own state directories.
	assert.Equal(t, "sandbox", sh("id -un"), "container must run as the sandbox user")
	assert.Equal(t, "1000", sh("id -u"))
	assert.Empty(t, sh("ls -A /workspace"), "/workspace must be empty in the image")
	sh("test ! -e /workspace/.kiro")
	assert.Empty(t, sh("find /workspace /home/sandbox -name '.kiro*' -o -name '.kairon*' 2>/dev/null"),
		"no .kiro or .kairon content under the workspace or the sandbox home")
	assert.Empty(t, sh("find / -xdev -name '.kairon' -not -path '/proc/*' 2>/dev/null"),
		"no Kairon project config anywhere in the image")

	// Stable tool set on PATH.
	for _, tool := range []string{"kiro-cli", "gh", "git", "sh"} {
		path := sh("command -v " + tool)
		assert.True(t, strings.HasPrefix(path, "/"), "%s should resolve to a path, got %q", tool, path)
	}
	assert.Contains(t, sh("gh --version"), "gh version")
	assert.NotEmpty(t, sh("git --version"))
}
