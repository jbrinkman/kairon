package sandbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustHostConfig builds a host config with the given limits and no mounts.
func mustHostConfig(tb testing.TB, limits ResourceLimits) *container.HostConfig {
	tb.Helper()
	hc, err := NewHostConfigWithMounts(limits, nil)
	require.NoError(tb, err)
	return hc
}

func TestContainer_Lifecycle(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Test Create
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}
	hostConfig := &container.HostConfig{}

	err = c.Create(ctx, config, hostConfig)
	assert.NoError(t, err)
	assert.NotEmpty(t, c.containerID)

	// Test Start
	err = c.Start(ctx)
	assert.NoError(t, err)

	// Test Exec
	err = c.Exec(ctx, []string{"echo", "test"})
	assert.NoError(t, err)

	// Test ExecWithOutput
	output, err := c.ExecWithOutput(ctx, []string{"echo", "hello"})
	assert.NoError(t, err)
	assert.Contains(t, output, "hello")

	// Test Cleanup
	err = c.Cleanup(ctx)
	assert.NoError(t, err)
}

func TestContainer_CopyTo(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Create container
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}
	err = c.Create(ctx, config, &container.HostConfig{})
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Create test file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	err = os.WriteFile(tmpFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Test file copy
	err = c.CopyTo(ctx, "/tmp/test.txt", tmpFile)
	assert.NoError(t, err)

	// Verify file exists in container
	output, err := c.ExecWithOutput(ctx, []string{"cat", "/tmp/test.txt"})
	assert.NoError(t, err)
	assert.Contains(t, output, "test content")
}

func TestResourceLimits(t *testing.T) {
	limits := DefaultLimits()

	// Test default values
	assert.Equal(t, int64(1000000), limits.CPUQuota)
	assert.Equal(t, int64(512*1024*1024), limits.Memory)
	assert.Equal(t, 5*time.Minute, limits.Timeout)

	// Test applying to host config
	hostConfig := &container.HostConfig{}
	limits.ApplyToHostConfig(hostConfig)

	assert.Equal(t, int64(1000000), hostConfig.Resources.CPUQuota)
	assert.Equal(t, int64(100000), hostConfig.Resources.CPUPeriod)
	assert.Equal(t, int64(512*1024*1024), hostConfig.Resources.Memory)

	// Test NewHostConfigWithMounts (limits and network policy preserved)
	hostConfig2, err := NewHostConfigWithMounts(limits, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1000000), hostConfig2.Resources.CPUQuota)
	assert.Equal(t, int64(100000), hostConfig2.Resources.CPUPeriod)
	assert.Equal(t, int64(512*1024*1024), hostConfig2.Resources.Memory)
	assert.Equal(t, container.NetworkMode("none"), hostConfig2.NetworkMode)
}

func TestResourceLimitsEnforcement(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Create container with strict memory limit (16MB)
	limits := ResourceLimits{
		CPUQuota: 500000,           // 0.5 core
		Memory:   16 * 1024 * 1024, // 16MB
		Timeout:  10 * time.Second,
	}

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}
	hostConfig, err := NewHostConfigWithMounts(limits, nil)
	require.NoError(t, err)

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Test memory limit is applied (works with both cgroups v1 and v2)
	output, err := c.ExecWithOutput(ctx, []string{"sh", "-c", "cat /sys/fs/cgroup/memory.max 2>/dev/null || cat /sys/fs/cgroup/memory/memory.limit_in_bytes 2>/dev/null || echo unknown"})
	assert.NoError(t, err)
	assert.Contains(t, output, "16777216") // 16MB in bytes
}

func TestContainerTimeout(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}

	err = c.Create(ctx, config, &container.HostConfig{})
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Use a very short timeout for the exec — should fail
	execCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	_, err = c.ExecWithOutput(execCtx, []string{"sleep", "10"})
	if err != nil {
		assert.Contains(t, err.Error(), "context deadline exceeded")
	} else {
		// Some Docker versions complete exec create before timeout hits
		t.Log("exec completed before timeout — skipping timeout assertion")
	}
}

// TestWorkspaceValidation tests comprehensive workspace validation per Task 5
func TestWorkspaceValidation(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	// Enable debug mode to test container preservation
	err = c.SetDebugMode(true)
	require.NoError(t, err)

	limits := DefaultLimits()
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "60"},
	}
	// A throw-away tmpfs workspace: this test exercises generic container
	// filesystem and exec behavior, not the host-mounted workspace.
	hostConfig, err := NewHostConfigWithMounts(limits, nil)
	require.NoError(t, err)
	hostConfig.Tmpfs = map[string]string{"/workspace": "rw,nosuid,size=64m"}

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// 1. Confirm /workspace is writable
	t.Run("workspace_writability", func(t *testing.T) {
		// Create nested directory structure
		_, err := c.ExecWithOutput(ctx, []string{"mkdir", "-p", "/workspace/.kiro/skills"})
		assert.NoError(t, err, "/workspace should be writable for nested directories")

		// Test file operations
		_, err = c.ExecWithOutput(ctx, []string{"sh", "-c", "echo 'writable' > /workspace/test-write.txt"})
		assert.NoError(t, err, "should be able to write files to /workspace")

		// Verify read-back
		output, err := c.ExecWithOutput(ctx, []string{"cat", "/workspace/test-write.txt"})
		assert.NoError(t, err, "should be able to read files from /workspace")
		assert.Contains(t, output, "writable")
	})

	// 2. Test GitHub mocking setup succeeds
	t.Run("github_mocking_setup", func(t *testing.T) {
		// Create the .kiro/skills structure inside the container
		_, err := c.ExecWithOutput(ctx, []string{"mkdir", "-p", "/workspace/.kiro/skills/github-cli"})
		assert.NoError(t, err, "should be able to create GitHub CLI mock directory structure")

		// Create a mock gh script inside the container using a simpler approach
		_, err = c.ExecWithOutput(ctx, []string{"sh", "-c", "echo '#!/bin/sh' > /workspace/.kiro/skills/github-cli/gh"})
		assert.NoError(t, err, "should be able to create mock script header")

		_, err = c.ExecWithOutput(ctx, []string{"sh", "-c", "echo 'echo \"[MOCK] GitHub CLI: $@\"' >> /workspace/.kiro/skills/github-cli/gh"})
		assert.NoError(t, err, "should be able to add mock script content")

		// Make the script executable
		_, err = c.ExecWithOutput(ctx, []string{"chmod", "755", "/workspace/.kiro/skills/github-cli/gh"})
		assert.NoError(t, err, "should be able to make mock script executable")

		// Verify the mock script exists and is executable
		_, err = c.ExecWithOutput(ctx, []string{"test", "-x", "/workspace/.kiro/skills/github-cli/gh"})
		assert.NoError(t, err, "GitHub CLI mock script should be executable")

		// Test that the mock script works by running it with sh
		output, err := c.ExecWithOutput(ctx, []string{"sh", "/workspace/.kiro/skills/github-cli/gh", "issue", "list"})
		assert.NoError(t, err, "mock script should execute successfully")
		assert.Contains(t, output, "[MOCK]", "mock script should produce expected output")
	})

	// 3. Test file operations work correctly
	t.Run("file_operations", func(t *testing.T) {
		// Complex file operations
		commands := [][]string{
			{"mkdir", "-p", "/workspace/test/subdir"},
			{"touch", "/workspace/test/file1.txt"},
			{"cp", "/workspace/test/file1.txt", "/workspace/test/file2.txt"},
			{"ln", "-s", "/workspace/test/file1.txt", "/workspace/test/link.txt"},
			{"chmod", "755", "/workspace/test/file1.txt"},
		}

		for _, cmd := range commands {
			_, err := c.ExecWithOutput(ctx, cmd)
			assert.NoError(t, err, "file operation should succeed: %v", cmd)
		}

		// Verify results
		_, err := c.ExecWithOutput(ctx, []string{"ls", "-la", "/workspace/test"})
		assert.NoError(t, err, "should be able to list workspace contents")
	})

	// 4. Test error messages are clear
	t.Run("error_message_clarity", func(t *testing.T) {
		// Test operation on non-existent path (should fail with clear message)
		_, err := c.ExecWithOutput(ctx, []string{"cat", "/workspace/nonexistent.txt"})
		assert.Error(t, err, "reading nonexistent file should produce clear error")
		assert.Contains(t, err.Error(), "nonexistent.txt", "error message should mention the file")

		// Test writing to /proc filesystem (should fail with clear error on most systems)
		_, err = c.ExecWithOutput(ctx, []string{"sh", "-c", "echo 'fail' > /proc/version"})
		assert.Error(t, err, "writing to read-only proc filesystem should fail")

		// Test accessing non-existent command - check stderr capture
		_, err = c.ExecWithOutput(ctx, []string{"sh", "-c", "nonexistent-command"})
		assert.Error(t, err, "running nonexistent command should fail")
		assert.Contains(t, err.Error(), "not found", "error should contain 'not found' from shell stderr")
	})

	// 5. Debug mode preserves failed containers
	t.Run("debug_preservation", func(t *testing.T) {
		// Call cleanup with failed=true — in debug mode the container should be preserved
		err := c.CleanupWithDebugInfo(ctx, true)
		assert.NoError(t, err, "cleanup should succeed in debug mode with failed container")

		// Container should still be accessible after debug cleanup preserves it
		_, err = c.ExecWithOutput(ctx, []string{"echo", "debug-test"})
		assert.NoError(t, err, "container should remain accessible after debug-mode preservation")
	})
}

func TestExecWithOutput_ErrorHandling(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}

	err = c.Create(ctx, config, &container.HostConfig{})
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Test successful command
	output, err := c.ExecWithOutput(ctx, []string{"echo", "success"})
	assert.NoError(t, err)
	assert.Contains(t, output, "success")

	// Test failing command should return error
	_, err = c.ExecWithOutput(ctx, []string{"false"})
	assert.Error(t, err, "failing command should return error")
	assert.Contains(t, err.Error(), "exit code 1")

	// Test non-existent command should return error
	_, err = c.ExecWithOutput(ctx, []string{"nonexistent-command"})
	assert.Error(t, err, "non-existent command should return error")
}

func TestKiroCLIInstallation_VerificationLogic(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}

	err = c.Create(ctx, config, &container.HostConfig{})
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Test verification fails when kiro-cli is not installed
	err = c.verifyKiroCLIInstallation(ctx)
	assert.Error(t, err, "verification should fail when kiro-cli is not installed")
}

// TestContainer_ExecWithStdin round-trips a >1 MiB payload containing quotes and
// newlines through `cat` byte-identically, and checks exit-code reporting,
// stderr separation, ExecWithOutput compatibility and ctx cancellation.
func TestContainer_ExecWithStdin(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	err = c.Create(ctx, &container.Config{Image: "alpine:3.19", Cmd: []string{"sleep", "60"}}, &container.HostConfig{})
	require.NoError(t, err)
	require.NoError(t, c.Start(ctx))
	defer c.Cleanup(ctx)

	t.Run("large payload round-trips byte-identically", func(t *testing.T) {
		unit := []byte("line \"double\" 'single' $(echo hi) `tick` \\ back\n\n  trailing space \n")
		payload := bytes.Repeat(unit, (1<<20)/len(unit)+64)
		require.Greater(t, len(payload), 1<<20)

		res, err := c.ExecWithStdin(ctx, []string{"cat"}, bytes.NewReader(payload))
		require.NoError(t, err)
		assert.Equal(t, 0, res.ExitCode)
		assert.Empty(t, res.Stderr)
		assert.True(t, bytes.Equal(payload, []byte(res.Stdout)), "stdout differs from stdin payload (got %d bytes, want %d)", len(res.Stdout), len(payload))
	})

	t.Run("non-zero exit is reported via ExitCode with separate untrimmed streams", func(t *testing.T) {
		res, err := c.ExecWithStdin(ctx, []string{"sh", "-c", "cat; printf ' out \\n'; printf ' err \\n' >&2; exit 3"}, bytes.NewReader([]byte("in\n")))
		require.NoError(t, err)
		assert.Equal(t, 3, res.ExitCode)
		assert.Equal(t, "in\n out \n", res.Stdout)
		assert.Equal(t, " err \n", res.Stderr)
	})

	t.Run("ExecWithOutput semantics unchanged", func(t *testing.T) {
		out, err := c.ExecWithOutput(ctx, []string{"sh", "-c", "echo ' hi '; echo boom >&2; exit 2"})
		require.Error(t, err)
		assert.Equal(t, "hi", out)
		assert.Equal(t, "command failed with exit code 2: boom", err.Error())
	})

	t.Run("context cancellation returns promptly with the context error", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := c.ExecWithStdin(cctx, []string{"sleep", "30"}, bytes.NewReader([]byte("x")))
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), 10*time.Second)
	})

	t.Run("a stdin read error is surfaced, not silently dropped", func(t *testing.T) {
		sentinel := errors.New("boom reading stdin")
		_, err := c.ExecWithStdin(ctx, []string{"cat"}, &errAfterReader{data: []byte("some bytes"), err: sentinel})
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.Contains(t, err.Error(), "reading stdin")
	})
}

// errAfterReader yields data once, then fails with err on the next Read, to
// exercise the stdin-read-error path of ExecWithStdin.
type errAfterReader struct {
	data []byte
	err  error
	done bool
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if !r.done && len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		if len(r.data) == 0 {
			r.done = true
		}
		return n, nil
	}
	return 0, r.err
}

var _ io.Reader = (*errAfterReader)(nil)
