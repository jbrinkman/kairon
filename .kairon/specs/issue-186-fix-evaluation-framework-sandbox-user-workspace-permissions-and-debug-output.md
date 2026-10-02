# Fix Evaluation Framework: Sandbox User Workspace Permissions and Debug Output

**Issue**: #186  
**Agent**: architect  
**Status**: Design Complete  

Closes #186

## Problem Analysis

The evaluation framework has two critical issues preventing proper operation:

1. **Debug Output Issue**: Raw JSON stream objects displayed instead of clean formatted Docker build progress
2. **Workspace Permission Issue**: `/workspace` directory owned by root, but sandbox user (who runs tests) cannot write to it

### Root Cause Analysis

#### Debug Output Problem
- `DockerBuildOutputParser.ParseBuildStream()` in `internal/eval/sandbox/build_output.go` has incorrect debug logic
- When `p.Debug` is true, it bypasses JSON parsing and outputs raw Docker stream lines
- Expected: Clean formatted build progress like `Step 1/8: FROM alpine:3.19 → 6baf43584bcb`
- Actual: Raw JSON like `{"stream":"Step 1/8 : FROM alpine:3.19"}` and `{"stream":"\n"}`

#### Workspace Permission Problem  
- `container.go:396` correctly sets `RUN mkdir -p /workspace && chown sandbox:sandbox /workspace`
- However, execution happens as root during container build, so `/workspace` gets created properly
- The issue occurs during GitHub mocking setup when the sandbox user tries to create directories in `/workspace`
- This is a timing/execution issue, not a Dockerfile issue

## Solution Approach

### 1. Debug Output Fix
**Strategy**: Fix the debug flag logic to format JSON streams properly in debug mode

**Implementation**: 
- Modify `DockerBuildOutputParser.ParseBuildStream()` to parse JSON even in debug mode
- Add debug-specific formatting that shows both raw JSON (for debugging) and clean output
- Maintain backward compatibility for non-debug mode

### 2. Workspace Permissions Fix  
**Strategy**: Ensure workspace permissions are properly set at container runtime, not just build time

**Implementation**:
- Add explicit permission verification step after container creation
- Ensure GitHub mocking setup runs with proper workspace access
- Add runtime validation of sandbox user workspace access

### 3. Integration Test for Regression Prevention
**Strategy**: Create dedicated test that validates workspace permissions independently

**Implementation**:
- Create integration test that spawns container and validates file creation as sandbox user
- Test must run in CI to catch regressions
- Validate both local and CI environments

## Relevant Files

### Files to Modify
- `internal/eval/sandbox/build_output.go` - Fix debug output parsing
- `internal/eval/sandbox/container.go` - Add runtime workspace permission validation
- `internal/eval/sandbox/container_test.go` - Add workspace permission integration test
- `.github/workflows/ci.yml` - Add workspace permission test to CI pipeline

### Files to Reference  
- `internal/eval/runner.go` - Understand container execution flow
- `internal/eval/dockerfile/base.Dockerfile` - Base container setup
- `internal/eval/debug/dockerfile.go` - Debug artifact management

## Team Orchestration

### Single Implementation Phase
Since both issues are in the evaluation framework sandbox module, they can be addressed in one coordinated effort:

1. **Builder Agent**: Fix debug output parsing and add workspace permission validation
2. **Builder Agent**: Create integration test for workspace permissions  
3. **Builder Agent**: Update CI configuration to run workspace permission tests
4. **Validator Agent**: Verify fixes work in both debug and non-debug modes
5. **Validator Agent**: Confirm integration test catches permission regressions

## Step-by-Step Task Breakdown

### Task 1: Fix Debug Output Parsing
**Acceptance Criteria**:
- [ ] Docker build progress shows clean formatted output in debug mode
- [ ] Raw JSON still available for debugging but formatted nicely
- [ ] Non-debug mode behavior unchanged
- [ ] Debug output shows: `Step 1/8: FROM alpine:3.19 → 6baf43584bcb`

**Implementation**:
1. Modify `DockerBuildOutputParser.ParseBuildStream()` in `internal/eval/sandbox/build_output.go`
2. Change debug logic to parse JSON first, then format for display
3. Add formatted output that shows step progression clearly
4. Keep raw JSON in debug artifacts if needed

### Task 2: Add Runtime Workspace Permission Validation
**Acceptance Criteria**:
- [ ] `/workspace` directory is writable by sandbox user after container start
- [ ] GitHub mocking setup completes successfully  
- [ ] All 7 evaluation test cases can execute and produce results
- [ ] Container setup logs show workspace ownership confirmation

**Implementation**:
1. Add `ValidateWorkspacePermissions()` method to `Container` struct
2. Call validation after container start in `internal/eval/sandbox/container.go`
3. Add retry logic for permission setup if needed
4. Log workspace permission status during container startup

### Task 3: Create Workspace Permission Integration Test
**Acceptance Criteria**:
- [ ] Integration test creates evaluation container and validates workspace writability
- [ ] Test runs as sandbox user (not root) and verifies file creation succeeds
- [ ] Test runs locally on-demand and automatically in CI pipeline
- [ ] Test fails if workspace permissions are incorrect

**Implementation**:
1. Create `TestWorkspacePermissions` in `internal/eval/sandbox/container_test.go`
2. Test should create container, switch to sandbox user, attempt file creation
3. Validate both directory creation and file writing succeed
4. Add test to existing test suite

### Task 4: Update CI Pipeline  
**Acceptance Criteria**:
- [ ] Workspace permission test runs in CI pipeline
- [ ] CI fails if workspace permissions are broken
- [ ] Test runs with Docker available in CI environment
- [ ] CI logs show workspace permission validation results

**Implementation**:
1. Ensure Docker is available in CI environment (already configured)
2. Add specific test run for workspace permissions if needed
3. Verify test runs as part of `task test` command
4. Add Docker service to CI if not already present

## Validation Commands

### Debug Output Validation
```bash
# Test debug output formatting
kiro-krew eval architect --sandbox --debug

# Expected output should show:
# 🔧 Debug: Build output:
# Step 1/8: FROM alpine:3.19 → 6baf43584bcb
# Step 2/8: RUN apk add --no-cache git curl bash ca-certificates → 7f2a4b5c8d9e
# ...
```

### Workspace Permission Validation  
```bash
# Run integration test for workspace permissions
go test -v ./internal/eval/sandbox -run TestWorkspacePermissions

# Run full evaluation to verify all test cases work
kiro-krew eval architect --sandbox

# Expected: All 7 test cases should complete successfully
```

### CI Integration Validation
```bash
# Run full test suite including Docker tests
task test

# Expected: All tests pass including workspace permission validation
```

## Technical Implementation Details

### Debug Output Fix Details
The current logic in `build_output.go:47-52` bypasses JSON parsing when debug is enabled:
```go
if p.Debug {
    result.WriteString(line + "\n")  // Raw output
    parsed++
    continue
}
```

**Fix**: Parse JSON first, then format appropriately for debug mode:
```go
var msg dockerStreamMessage
if err := json.Unmarshal([]byte(line), &msg); err != nil {
    if p.Debug {
        result.WriteString(line + "\n")  // Fallback for non-JSON
    }
    continue
}

if p.Debug {
    // Format for readability while preserving info
    if msg.Stream != "" {
        formatted := formatBuildStep(msg.Stream)
        result.WriteString(formatted)
    }
} else {
    // Existing clean output logic
    if msg.Stream != "" {
        clean := ansiRegex.ReplaceAllString(msg.Stream, "")
        result.WriteString(clean)
    }
}
```

### Workspace Permission Fix Details
The container setup correctly creates workspace with proper ownership during build, but we need runtime validation:

```go
// Add to Container struct
func (c *Container) ValidateWorkspacePermissions(ctx context.Context) error {
    // Test write access as sandbox user
    testCmd := []string{"sh", "-c", "touch /workspace/.permission_test && rm /workspace/.permission_test"}
    if _, err := c.ExecWithOutput(ctx, testCmd); err != nil {
        return fmt.Errorf("workspace not writable by sandbox user: %w", err)
    }
    
    if c.debugMode {
        fmt.Printf("🔧 Debug: Workspace permissions validated for sandbox user\n")
    }
    return nil
}
```

### Integration Test Structure
```go
func TestWorkspacePermissions(t *testing.T) {
    skipIfNoDocker(t)
    
    ctx := context.Background()
    container, err := NewContainerWithDebug("test-workspace-perms", true)
    require.NoError(t, err)
    defer container.Close()
    
    // Build container with workspace setup
    dockerfile := generateTestDockerfile()
    imageName := container.GetCustomImageName("linux/amd64")
    err = container.BuildImageFromDockerfile(ctx, dockerfile, imageName, "linux/amd64")
    require.NoError(t, err)
    
    // Create and start container
    config := &container.Config{Image: imageName, Cmd: []string{"sleep", "30"}}
    hostConfig := &container.HostConfig{}
    
    err = container.Create(ctx, config, hostConfig)
    require.NoError(t, err)
    
    err = container.Start(ctx)
    require.NoError(t, err)
    
    // Validate workspace permissions
    err = container.ValidateWorkspacePermissions(ctx)
    assert.NoError(t, err, "Sandbox user should be able to write to /workspace")
}
```

## Success Metrics

### Primary Metrics
1. **Debug Output Quality**: Debug mode shows formatted Docker build progress instead of raw JSON
2. **Workspace Access**: All 7 evaluation test cases complete successfully with sandbox user
3. **Integration Test Coverage**: New test prevents workspace permission regressions
4. **CI Reliability**: Pipeline fails fast when workspace permissions are broken

### Regression Prevention
1. Integration test catches workspace permission issues before deployment
2. CI pipeline validates Docker container setup in automated environment  
3. Debug output formatting tested in both modes (debug and non-debug)
4. Documentation shows expected vs actual behavior for future debugging

This comprehensive fix addresses both immediate issues while building robust prevention mechanisms for future regressions.

