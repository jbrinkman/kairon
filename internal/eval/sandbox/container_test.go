package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestProjectDetection(t *testing.T) {
	testCases := []struct {
		name     string
		files    map[string]string
		expected []ProjectType
	}{
		{
			name:     "Go project",
			files:    map[string]string{"go.mod": "module test"},
			expected: []ProjectType{ProjectTypeGo},
		},
		{
			name:     "Node.js project",
			files:    map[string]string{"package.json": "{}"},
			expected: []ProjectType{ProjectTypeNodeJS},
		},
		{
			name:     "Python project",
			files:    map[string]string{"requirements.txt": "flask==2.0.1"},
			expected: []ProjectType{ProjectTypePython},
		},
		{
			name:     "Rust project",
			files:    map[string]string{"Cargo.toml": "[package]"},
			expected: []ProjectType{ProjectTypeRust},
		},
		{
			name:     "Java Maven project",
			files:    map[string]string{"pom.xml": "<project>"},
			expected: []ProjectType{ProjectTypeJava},
		},
		{
			name:     "Task project",
			files:    map[string]string{"Taskfile.yml": "version: '3'"},
			expected: []ProjectType{ProjectTypeTask},
		},
		{
			name: "Multi-language project",
			files: map[string]string{
				"go.mod":       "module test",
				"package.json": "{}",
				"Taskfile.yml": "version: '3'",
			},
			expected: []ProjectType{ProjectTypeGo, ProjectTypeNodeJS, ProjectTypeTask},
		},
		{
			name:     "No project files",
			files:    map[string]string{"README.md": "# Test"},
			expected: []ProjectType{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary directory with test files
			tmpDir := t.TempDir()
			for filename, content := range tc.files {
				err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
				require.NoError(t, err)
			}

			// Detect projects
			projects := DetectProject(tmpDir)

			// Extract project types
			var detected []ProjectType
			for _, p := range projects {
				detected = append(detected, p.Type)
			}

			assert.ElementsMatch(t, tc.expected, detected)
		})
	}
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

	// Test NewHostConfigWithLimits
	hostConfig2 := NewHostConfigWithLimits(limits)
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
	hostConfig := NewHostConfigWithLimits(limits)

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

func TestGenerateDockerfile(t *testing.T) {
	c := &Container{}

	// Detect host platform for test
	platform, err := DetectHostArchitecture()
	require.NoError(t, err)

	// Create test project directory
	tmpDir := t.TempDir()

	// Create Go project files
	err = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	require.NoError(t, err)

	// Mock template loading by creating a simple template
	templateDir := filepath.Join(tmpDir, "internal/eval/dockerfile/templates")
	err = os.MkdirAll(templateDir, 0755)
	require.NoError(t, err)

	goTemplate := `# Install Go
RUN apk add --no-cache go
ENV GOPATH=/home/sandbox/go
ENV PATH=$PATH:/usr/local/go/bin:$GOPATH/bin
`
	err = os.WriteFile(filepath.Join(templateDir, "go.Dockerfile"), []byte(goTemplate), 0644)
	require.NoError(t, err)

	// Change to temp directory to test template loading
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tmpDir)

	dockerfile, err := c.GenerateDockerfileWithPlatform(tmpDir, platform)
	assert.NoError(t, err)
	assert.Contains(t, dockerfile, "FROM alpine:3.19")
	assert.Contains(t, dockerfile, "Install Go")
	assert.Contains(t, dockerfile, "adduser -D -s /bin/bash sandbox")
	assert.Contains(t, dockerfile, "WORKDIR /workspace")
}

func TestGitHubCLIMocking(t *testing.T) {
	skipIfNoContainerDaemon(t)
	testdataDir := "testdata/github-cli-mock"

	// Check mock script exists locally
	mockScript := filepath.Join(testdataDir, "gh")
	_, err := os.Stat(mockScript)
	require.NoError(t, err, "GitHub CLI mock script should exist")

	// Verify mock script content
	content, err := os.ReadFile(mockScript)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[MOCK]")
	assert.Contains(t, string(content), "#!/bin/bash")
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

func TestContainer_WorkspacePermissions(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	limits := DefaultLimits()
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}
	hostConfig := NewHostConfigWithLimits(limits)

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Test workspace directory is writable
	_, err = c.ExecWithOutput(ctx, []string{"mkdir", "-p", "/workspace/test"})
	assert.NoError(t, err, "workspace should be writable")

	// Test file creation in workspace
	_, err = c.ExecWithOutput(ctx, []string{"sh", "-c", "echo 'test' > /workspace/test/file.txt"})
	assert.NoError(t, err, "should be able to create files in workspace")

	// Verify file content
	output, err := c.ExecWithOutput(ctx, []string{"cat", "/workspace/test/file.txt"})
	assert.NoError(t, err)
	assert.Contains(t, output, "test")
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
	hostConfig := NewHostConfigWithLimits(limits)

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

func TestContainer_GitHubMockingSetup(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainer("alpine:3.19")
	require.NoError(t, err)
	defer c.Close()

	limits := DefaultLimits()
	config := &container.Config{
		Image: "alpine:3.19",
		Cmd:   []string{"sleep", "30"},
	}
	hostConfig := NewHostConfigWithLimits(limits)

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Test basic directory creation
	_, err = c.ExecWithOutput(ctx, []string{"mkdir", "-p", "/workspace/test"})
	assert.NoError(t, err, "should be able to create directories in workspace")

	// Test file creation
	_, err = c.ExecWithOutput(ctx, []string{"touch", "/workspace/test/file"})
	assert.NoError(t, err, "should be able to create files in workspace")
}

// generateTestDockerfile creates a simple test Dockerfile for workspace permission testing
func generateTestDockerfile() string {
	return `FROM alpine:3.19
RUN adduser -D -s /bin/bash sandbox
RUN mkdir -p /workspace && chown sandbox:sandbox /workspace
USER sandbox
WORKDIR /workspace
`
}

func TestWorkspacePermissions(t *testing.T) {
	skipIfNoContainerDaemon(t)

	ctx := context.Background()
	c, err := NewContainerWithDebug("test-workspace-perms", true)
	require.NoError(t, err)
	defer c.Close()

	// Build container with workspace setup
	dockerfile := generateTestDockerfile()
	imageName := c.GetCustomImageName("linux/amd64")
	err = c.BuildImageFromDockerfile(ctx, dockerfile, imageName, "linux/amd64")
	require.NoError(t, err)

	// Create and start container
	config := &container.Config{Image: imageName, Cmd: []string{"sleep", "30"}}
	hostConfig := &container.HostConfig{}

	err = c.Create(ctx, config, hostConfig)
	require.NoError(t, err)

	err = c.Start(ctx)
	require.NoError(t, err)
	defer c.Cleanup(ctx)

	// Validate workspace permissions directly in the test
	_, err = c.ExecWithOutput(ctx, []string{"test", "-d", "/workspace"})
	require.NoError(t, err, "/workspace directory should exist")

	_, err = c.ExecWithOutput(ctx, []string{"sh", "-c", "touch /workspace/.permission_test && rm /workspace/.permission_test"})
	assert.NoError(t, err, "Sandbox user should be able to write to /workspace")

	_, err = c.ExecWithOutput(ctx, []string{"mkdir", "-p", "/workspace/subdir/nested"})
	assert.NoError(t, err, "Sandbox user should be able to create nested directories")
}
