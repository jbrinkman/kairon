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

func TestKiroCLIExecution_SandboxUser(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create container with the sandbox host config (read-only rootfs + tmpfs).
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "300"},
		User:  "root",
	}

	limits := DefaultLimits()
	hostConfig := mustHostConfig(t, limits)

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

	// The sandbox runs with a read-only root filesystem, so a runtime
	// `adduser` (which writes /etc/passwd) is deliberately impossible — the
	// real base image bakes the sandbox user in. This test runs on bare
	// alpine:3.19, so it exercises the kiro-cli probes directly as the
	// container's user rather than creating one at runtime.

	// kiro-cli --version fails because kiro-cli isn't installed in base Alpine.
	// Run directly (no shell): the exec reports exit code 127 (command not
	// found) with empty stderr, so assert on that signal rather than parsing a
	// shell's error text.
	output, err := c.ExecWithOutput(ctx, []string{"kiro-cli", "--version"})
	if err != nil {
		assert.True(t, kiroCLIMissing(err), "Error should indicate kiro-cli is missing: %v", err)
		t.Logf("Expected failure - kiro-cli not installed: %v", err)
	} else {
		// If somehow it works, verify output.
		assert.NotEmpty(t, output)
		t.Logf("Unexpected success - kiro-cli found: %s", output)
	}

	// kiro-cli chat --help also fails without an installation.
	output, err = c.ExecWithOutput(ctx, []string{"kiro-cli", "chat", "--help"})
	if err != nil {
		assert.True(t, kiroCLIMissing(err), "Error should indicate kiro-cli is missing: %v", err)
		t.Logf("Expected failure - kiro-cli chat not available: %v", err)
	} else {
		assert.Contains(t, strings.ToLower(output), "help")
		t.Logf("Unexpected success - kiro-cli chat help: %s", output)
	}
}

// kiroCLIMissing reports whether err indicates kiro-cli is absent. Running a
// missing binary directly via exec yields exit code 127 (command not found)
// with empty stderr, so the exit code is the reliable signal; the name/text
// checks also catch a shell-mediated "not found" message.
func kiroCLIMissing(err error) bool {
	s := err.Error()
	return strings.Contains(s, "exit code 127") ||
		strings.Contains(s, "kiro-cli") ||
		strings.Contains(s, "not found") ||
		strings.Contains(s, "No such file")
}
