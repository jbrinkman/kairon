# Fix sandbox evaluation regression: kiro-cli installation and read-only filesystem errors

Closes #159

## Problem Analysis

The sandbox evaluation system has three critical regressions:

1. **ExecWithOutput Error Handling**: `ExecWithOutput` method doesn't distinguish between command execution errors and actual output, causing success messages to display error content
2. **Container Workspace Permissions**: Container filesystem is read-only at `/workspace`, preventing GitHub mocking setup from creating directories
3. **Installation Verification Logic**: `verifyKiroCLIInstallation` reports success even when kiro-cli installation fails

## Solution Approach

### High-level Strategy

1. **Fix ExecWithOutput Method**: Enhance error handling to properly distinguish command output from execution errors
2. **Container Configuration**: Configure container with writable workspace directory and proper user permissions
3. **Installation Verification**: Improve `verifyKiroCLIInstallation` to only report success on actual installation success
4. **Error Context Enhancement**: Provide better error messages and debugging information

### Architecture Changes

- **Container Setup**: Move from read-only root filesystem to properly configured writable workspace
- **Error Propagation**: Ensure Docker exec exit codes are properly captured and surfaced
- **User Permissions**: Configure container user and file permissions for workspace operations

## Relevant Files

### Files to Modify

1. **internal/eval/sandbox/container.go** - Core container management and command execution
2. **internal/eval/sandbox/resource_limits.go** - Container host configuration for writable workspace
3. **internal/eval/sandbox/container_test.go** - Add regression tests for fixed functionality

### Files Referenced but Not Modified

- **internal/eval/runner.go** - Container configuration and error handling (integration point)
- **internal/eval/sandbox/mock_github.go** - GitHub mocking setup (benefits from fixes)

## Team Orchestration

### Builder Focus Areas

1. **Phase 1**: Fix ExecWithOutput method error handling
2. **Phase 2**: Configure container workspace permissions 
3. **Phase 3**: Improve installation verification logic
4. **Phase 4**: Add comprehensive tests

### Validator Verification Points

- Container workspace directory is writable
- ExecWithOutput properly distinguishes errors from output
- Installation verification only succeeds on actual success
- GitHub mocking setup completes without filesystem errors

## Step-by-Step Task Breakdown

### Task 1: Fix ExecWithOutput Error Handling
**File**: `internal/eval/sandbox/container.go`
**Acceptance Criteria**:
- ExecWithOutput captures Docker exec exit codes properly
- Command output and error information are separated  
- Method returns error only on actual execution failures
- Success output doesn't contain error messages

**Implementation Details**:
- Use `ContainerExecInspect` to get exit code after command execution
- Return error if exit code != 0
- Clean up hijacked connection properly
- Strip Docker stream headers from output

### Task 2: Configure Container Workspace Permissions  
**Files**: `internal/eval/sandbox/resource_limits.go`, `internal/eval/sandbox/container.go`
**Acceptance Criteria**:
- Container `/workspace` directory is writable by sandbox user
- GitHub mocking can create directories and files
- Container user has proper permissions for file operations
- No "read-only file system" errors during setup

**Implementation Details**:
- Remove read-only filesystem restrictions from container config
- Configure proper user ownership of workspace directory  
- Add tmpfs or bind mount for writable workspace if needed
- Update NewHostConfigWithLimits to allow filesystem writes

### Task 3: Fix Installation Verification Logic
**File**: `internal/eval/sandbox/container.go` 
**Acceptance Criteria**:
- `verifyKiroCLIInstallation` only prints success on actual success
- Installation failures result in error return, not success message
- Version output is captured and displayed only on success
- Clear error messages for installation failures

**Implementation Details**:
- Check ExecWithOutput return values properly in verification
- Only print success message after confirming no errors
- Enhance error messages with specific failure details
- Move success logging to after all verification passes

### Task 4: Add Container Configuration Tests
**File**: `internal/eval/sandbox/container_test.go`
**Acceptance Criteria**:
- Test verifies container workspace is writable
- Test confirms ExecWithOutput error handling works correctly
- Test validates kiro-cli installation verification logic
- Test covers GitHub mocking setup in writable workspace

**Implementation Details**:
- Add TestContainer_WorkspacePermissions test
- Add TestExecWithOutput_ErrorHandling test  
- Add TestKiroCLIInstallation_VerificationLogic test
- Add TestContainer_GitHubMockingSetup test

### Task 5: Integration Testing
**Acceptance Criteria**:
- `kiro-krew eval architect --sandbox` runs successfully end-to-end
- No "read-only file system" errors in container logs
- No false success messages for failed installations  
- Container logs show actual kiro-cli version on successful installation

## Validation Commands

### Unit Testing
```bash
# Run container-specific tests
go test ./internal/eval/sandbox -v

# Run integration tests with Docker
go test ./internal/eval/sandbox -v -tags=integration
```

### Manual Validation  
```bash
# Test sandbox evaluation end-to-end
kiro-krew eval architect --sandbox

# Verify no error messages in success output
kiro-krew eval architect --sandbox 2>&1 | grep -E "(installation verified|❌|Error:)"

# Check container workspace permissions
docker run --rm alpine:3.19 sh -c "mkdir -p /workspace/test && echo 'success'"
```

### Expected Success Indicators
- Container logs show "✅ kiro-cli installation verified: v..." with actual version
- No "OCI runtime exec failed" messages in success output  
- No "read-only file system" errors during GitHub mocking
- Evaluation completes with proper agent output

### Expected Error Cases (Should Be Fixed)
- No more: "✅ kiro-cli installation verified: OCI runtime exec failed..."
- No more: "Error: setting up GitHub mocking: mkdir /workspace: read-only file system"
- No more: False positive success messages when commands fail

## Technical Implementation Notes

### ExecWithOutput Error Handling Pattern
```go
// Check execution result properly
inspect, err := c.client.ContainerExecInspect(ctx, resp.ID)
if err != nil {
    return "", err
}

if inspect.ExitCode != 0 {
    return "", fmt.Errorf("command failed with exit code %d", inspect.ExitCode)
}
```

### Container Workspace Configuration
```go
// Ensure workspace is writable
hostConfig := &container.HostConfig{
    // Remove read-only restrictions
    ReadonlyRootfs: false,
    // Configure tmpfs for writable workspace
    Tmpfs: map[string]string{
        "/workspace": "rw,noexec,nosuid,size=512m",
    },
}
```

### Installation Verification Flow
```go  
// Only succeed on actual success
if err := c.ExecWithOutput(ctx, []string{"kiro-cli", "--version"}); err != nil {
    return fmt.Errorf("kiro-cli installation failed: %w", err) 
}
// Success message only here after verification passes
fmt.Printf("✅ kiro-cli installation verified: %s\n", version)
```

## Risk Assessment

### Low Risk
- ExecWithOutput error handling improvements (isolated method change)
- Installation verification logic updates (contained function)

### Medium Risk  
- Container configuration changes (could affect existing workflows)
- Workspace permission modifications (filesystem behavior changes)

### Mitigation Strategies
- Comprehensive unit and integration testing
- Backward compatibility preservation for non-sandbox modes
- Clear error messages for debugging container issues
- Docker availability checks before attempting container operations

This design maintains existing container isolation while fixing the critical regressions that prevent sandbox evaluation from functioning properly.