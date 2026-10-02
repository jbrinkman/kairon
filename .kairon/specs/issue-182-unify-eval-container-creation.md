# Design Specification: Unify Eval Container Creation Paths

**Issue:** #182 - fix: unify eval container creation paths and rename misleading InstallKiroCLI function

**Closes #182**

## Problem Statement

The eval system currently has two completely separate container creation paths that violate the core testing principle of exercising the same code paths:

### Current Architecture Issues

1. **Tests Path**: Uses `GenerateDockerfileWithPlatform()` → builds custom images with kiro-cli pre-installed
2. **Production Path**: Uses `CreateWithPlatform()` → spawns plain `alpine:3.19` containers → calls `InstallKiroCLI()` for verification (which fails since nothing was installed)

3. **Misleading Function Name**: `InstallKiroCLI()` performs validation only, not installation, misleading developers about actual behavior

4. **Debug Mode Issue**: `--debug` mode saves `containers.json` but no Dockerfile because `GenerateDockerfileWithPlatform()` is never called in production flow

## Solution Approach

Unify both paths to use the **generate → build → create → verify** flow consistently:

1. **Production eval runner** will use `GenerateDockerfileWithPlatform()` to build custom images
2. **Rename** `InstallKiroCLI()` to `ValidateKiroCLI()` to reflect actual behavior
3. **Enhanced debug mode** saves both Dockerfile and container registry info
4. **Remove dead code paths** that bypass Dockerfile generation

## Relevant Files

### Core Files to Modify
- `internal/eval/runner.go` - Main eval execution logic (production flow)
- `internal/eval/sandbox/container.go` - Container management and creation methods
- `internal/eval/debug/dockerfile.go` - Debug artifact management

### Test Files to Update
- `internal/eval/sandbox/container_test.go` - Unit tests for container methods
- `internal/eval/sandbox/integration_installation_test.go` - Integration tests
- `internal/eval/sandbox/integration_architecture_test.go` - Architecture tests
- `internal/eval/sandbox/installation_test.go` - Installation validation tests
- `internal/eval/runner_test.go` - Runner flow tests

### Configuration Files
- `internal/eval/types.go` - Type definitions (may need updates for new flow)

## Step-by-Step Task Breakdown

### Task 1: Rename InstallKiroCLI to ValidateKiroCLI
**Acceptance Criteria:**
- All references to `InstallKiroCLI` renamed to `ValidateKiroCLI`
- Function comments updated to reflect validation-only behavior
- Tests updated to use new function name

**Files:**
- `internal/eval/sandbox/container.go`: Rename function and update comments
- `internal/eval/runner.go`: Update function call
- All test files: Update test method calls and assertions

### Task 2: Modify Production Flow to Use Dockerfile Generation
**Acceptance Criteria:**
- Production eval runner calls `GenerateDockerfileWithPlatform()` before container creation
- Generated Dockerfile is built into a custom image
- Custom image is used in `CreateWithPlatform()` call
- `ValidateKiroCLI()` verifies the pre-installed kiro-cli

**Implementation:**
1. In `runTestCaseInContainer()` function (around line 570 in runner.go):
   - Add Dockerfile generation step before container creation
   - Build custom image from generated Dockerfile
   - Use custom image name instead of `alpine:3.19`
   - Update container configuration to use custom image

### Task 3: Enhanced Debug Mode Support
**Acceptance Criteria:**
- Debug mode saves generated Dockerfile using existing `debug.SaveDockerfile()`
- Debug mode saves container registry information
- Custom image names include debug identifiers for easy cleanup
- Container cleanup preserves debug containers when needed

**Files:**
- `internal/eval/runner.go`: Add debug artifact saving
- `internal/eval/debug/dockerfile.go`: Ensure Dockerfile saving works with custom images

### Task 4: Update Container Creation Logic
**Acceptance Criteria:**
- Add new method `BuildImageFromDockerfile()` to container.go
- Modify `GenerateDockerfileWithPlatform()` to return image name for building
- Update container configuration to use dynamically built images
- Ensure proper image cleanup in debug and non-debug modes

**New Methods:**
```go
// BuildImageFromDockerfile builds a Docker image from generated Dockerfile content
func (c *Container) BuildImageFromDockerfile(ctx context.Context, dockerfile string, imageName string, platform string) error

// GetCustomImageName generates a unique image name for the eval session
func (c *Container) GetCustomImageName(platform string) string
```

### Task 5: Remove Dead Code Paths
**Acceptance Criteria:**
- Identify and remove any container creation code that bypasses Dockerfile generation
- Ensure all eval flows go through unified path
- Clean up unused imports and helper functions

### Task 6: End-to-End Flow Analysis
**Acceptance Criteria:**
- Document complete flow: generate → build → create → verify
- Verify all steps are actually performed in both test and production
- Add integration test that validates complete flow
- Performance analysis to ensure new flow doesn't significantly impact execution time

## Team Orchestration

### Phase 1: Function Renaming (Low Risk)
- Rename `InstallKiroCLI` → `ValidateKiroCLI` across all files
- Update function documentation and comments
- Update all test cases

### Phase 2: Core Flow Modification (Medium Risk)
- Modify production eval runner to use Dockerfile generation
- Implement image building logic
- Update container creation to use custom images

### Phase 3: Debug Enhancement (Low Risk)
- Enhance debug mode to save all artifacts
- Improve debug output and container preservation

### Phase 4: Cleanup and Testing (Medium Risk)
- Remove dead code paths
- Comprehensive end-to-end testing
- Performance validation

## Validation Commands

### Unit Tests
```bash
# Test container creation methods
go test ./internal/eval/sandbox -v -run TestContainer

# Test runner flow
go test ./internal/eval -v -run TestRunner
```

### Integration Tests
```bash
# Test complete eval flow
go test ./internal/eval/sandbox -v -run TestContainerIntegration

# Test architecture compatibility
go test ./internal/eval/sandbox -v -run TestArchitecture
```

### Manual Validation
```bash
# Test production eval with debug mode
kiro-cli eval --debug --sandbox architect simple-task

# Verify Dockerfile is generated and saved
ls -la .kiro-krew/evals/tmp/dockerfiles/

# Verify containers.json is updated
cat .kiro-krew/evals/tmp/containers.json

# Test container cleanup
kiro-cli eval --cleanup
```

### End-to-End Flow Verification
```bash
# Run eval with verbose logging to verify flow steps
kiro-cli eval --debug --sandbox architect simple-task 2>&1 | grep -E "(Generate|Build|Create|Verify)"

# Check that custom image is created and used
docker images | grep kiro-eval

# Verify kiro-cli is pre-installed in custom image
docker run --rm <custom-image> kiro-cli --version
```

## Implementation Priority

1. **High Priority**: Task 1 (Function renaming) - Low risk, enables accurate communication
2. **High Priority**: Task 2 (Production flow modification) - Core requirement
3. **Medium Priority**: Task 3 (Debug enhancement) - Improves developer experience
4. **Medium Priority**: Task 4 (Container logic updates) - Technical implementation details
5. **Low Priority**: Task 5 (Dead code removal) - Code quality improvement
6. **Critical**: Task 6 (End-to-end validation) - Ensures solution works correctly

## Risk Assessment

### High Risk Areas
- Modifying production eval flow could break existing evaluations
- Image building adds complexity and potential failure points
- Container resource management needs careful handling

### Mitigation Strategies
- Comprehensive testing at each phase
- Backward compatibility verification
- Performance monitoring during implementation
- Rollback plan using feature flags if needed

## Success Metrics

1. **Functional**: Tests and production use identical container creation flow
2. **Debug**: `--debug` mode saves both Dockerfile and container registry info
3. **Performance**: Unified flow completes within acceptable time limits; Docker layer caching mitigates cold-build overhead after first run
4. **Reliability**: Zero regression in existing eval test success rates
5. **Maintainability**: Single code path reduces maintenance complexity
