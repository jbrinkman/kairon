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

	// Create container with sandbox user
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Configure container with sandbox user setup (use sh since bash not available in base Alpine)
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

	// Create sandbox user via exec (avoids racy sleep)
	_, err = c.ExecWithOutput(ctx, []string{"adduser", "-D", "-s", "/bin/sh", "sandbox"})
	require.NoError(t, err)

	// Verify sandbox user exists
	output, err := c.ExecWithOutput(ctx, []string{"id", "sandbox"})
	require.NoError(t, err)
	assert.Contains(t, output, "uid=")

	// Test kiro-cli --version as sandbox user (will fail without installation)
	output, err = c.ExecWithOutput(ctx, []string{"su", "-c", "kiro-cli --version", "sandbox"})
	if err != nil {
		// Expected to fail since kiro-cli isn't installed in base Alpine
		// Check for both "kiro-cli" and "not found" since Alpine may return different error messages
		errStr := err.Error()
		hasExpectedError := strings.Contains(errStr, "kiro-cli") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "No such file")
		assert.True(t, hasExpectedError, "Error should indicate kiro-cli is missing: %v", err)
		t.Logf("Expected failure - kiro-cli not installed: %v", err)
	} else {
		// If somehow it works, verify output
		assert.NotEmpty(t, output)
		t.Logf("Unexpected success - kiro-cli found: %s", output)
	}

	// Test kiro-cli chat --help as sandbox user (will also fail without installation)
	output, err = c.ExecWithOutput(ctx, []string{"su", "-c", "kiro-cli chat --help", "sandbox"})
	if err != nil {
		// Expected to fail since kiro-cli isn't installed
		errStr := err.Error()
		hasExpectedError := strings.Contains(errStr, "kiro-cli") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "No such file")
		assert.True(t, hasExpectedError, "Error should indicate kiro-cli is missing: %v", err)
		t.Logf("Expected failure - kiro-cli chat not available: %v", err)
	} else {
		// If somehow it works, verify help output
		assert.Contains(t, strings.ToLower(output), "help")
		t.Logf("Unexpected success - kiro-cli chat help: %s", output)
	}
}
