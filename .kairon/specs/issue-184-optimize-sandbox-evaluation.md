# Design Specification: Optimize Sandbox Evaluation System

**Issue**: #184 - Optimize sandbox evaluation: image reuse, clean output, writable workspace  
**Closes**: #184

## Problem Analysis

The current sandbox evaluation system has three critical performance and usability issues:

1. **Performance**: Container images are rebuilt for each test case, causing unnecessary overhead (~5 minutes per image build)
2. **Output Clarity**: Docker build logs contain JSON stream entries with control characters that clutter debug output
3. **Filesystem Access**: Workspace directory is read-only, causing "mkdir /workspace: read-only file system" errors during GitHub mocking setup

## Solution Approach

### 1. Image Reuse Strategy
Implement per-evaluation-run image caching to build images once and reuse across all test cases:
- Move image building to evaluation run startup, before test case execution
- Cache built images with evaluation-run-scoped names  
- Clean up images at evaluation run completion (preserve in debug mode)
- Maintain platform-specific image names for cross-architecture compatibility

### 2. Clean Docker Build Output
Parse Docker build stream JSON and extract clean text messages:
- Implement JSON stream parser for Docker build output
- Extract `stream` field content from JSON entries
- Strip ANSI control characters from extracted text
- Preserve debug capability with raw JSON output option

### 3. Writable Workspace Configuration
Fix workspace filesystem permissions to enable file operations:
- The current `tmpfs` mount in `resource_limits.go` already provides writable workspace
- Issue stems from incorrect workspace path handling in mock setup
- Ensure container operations use `/workspace` consistently
- Validate GitHub mocking works with writable tmpfs mount

## Architecture Changes

### Core Components Modified

#### 1. Image Lifecycle Management (`internal/eval/sandbox/container.go`)
- Add `ImageManager` struct to handle evaluation-run-scoped image caching
- Implement `BuildOnceForRun()` method for image building at evaluation start
- Add image cleanup at evaluation completion
- Maintain existing per-test-case container creation

#### 2. Build Output Processing (`internal/eval/sandbox/build_output.go` - new file)
- Create `DockerBuildOutputParser` to process JSON stream
- Implement clean text extraction with ANSI stripping
- Provide debug mode for raw output preservation

#### 3. Evaluation Runner Updates (`internal/eval/runner.go`)
- Move image building to `Run()` function startup
- Pass cached image name to test case execution
- Add evaluation-level cleanup

## Relevant Files

### Files to Create:
- `internal/eval/sandbox/build_output.go` - Docker build output parser
- `internal/eval/sandbox/image_manager.go` - Image lifecycle management

### Files to Modify:
- `internal/eval/sandbox/container.go` - Update BuildImageFromDockerfile method
- `internal/eval/runner.go` - Move image building to evaluation startup
- `internal/eval/types.go` - Add ImageManager field to ContainerConfig

### Files Referenced:
- `internal/eval/sandbox/resource_limits.go` - Workspace tmpfs configuration (already correct)
- `internal/eval/sandbox/mock_github.go` - GitHub mocking setup

## Team Orchestration

### Builder Agent Tasks:
1. **Create build output parser** (`build_output.go`)
   - JSON stream parsing
   - ANSI stripping  
   - Debug mode support

2. **Create image manager** (`image_manager.go`)
   - Evaluation-scoped image caching
   - Platform-specific naming
   - Cleanup coordination

3. **Update container implementation** (`container.go`)  
   - Integrate build output parser
   - Support image reuse workflow
   - Maintain existing API compatibility

4. **Update evaluation runner** (`runner.go`)
   - Move image building to evaluation startup
   - Pass cached images to test execution
   - Add evaluation-level cleanup

## Step-by-Step Task Breakdown

### Task 1: Docker Build Output Parser
**Acceptance Criteria:**
- Parse Docker JSON stream format  
- Extract clean text from `stream` fields
- Strip ANSI escape sequences
- Preserve debug mode with raw output
- Handle malformed JSON gracefully

**Implementation:**
- Create `DockerBuildOutputParser` struct
- Implement `ParseBuildStream(io.Reader) (string, error)` method
- Add ANSI regex for control character removal
- Support debug flag for dual output modes

### Task 2: Image Manager for Reuse
**Acceptance Criteria:** 
- Build images once per evaluation run
- Generate evaluation-scoped image names
- Support platform-specific caching
- Clean up images after evaluation (preserve in debug)
- Thread-safe for concurrent access

**Implementation:**
- Create `ImageManager` struct with mutex
- Implement `BuildForEvaluation(dockerfile, platform) (imageName, error)` method  
- Add evaluation ID to image naming
- Implement cleanup with debug mode awareness

### Task 3: Container Integration Updates
**Acceptance Criteria:**
- Use build output parser for clean logs
- Support image reuse workflow
- Maintain backward compatibility
- Preserve existing error handling
- Keep debug capabilities intact

**Implementation:**
- Update `BuildImageFromDockerfile` to use output parser
- Add image reuse methods to Container struct
- Integrate with ImageManager
- Maintain existing API contracts

### Task 4: Runner Evaluation Updates  
**Acceptance Criteria:**
- Build images once at evaluation start
- Pass cached images to test cases
- Clean up images after evaluation
- Support debug mode preservation
- Maintain performance profiling

**Implementation:**
- Move image building to `Run()` function startup
- Update `invokeAgentInContainer` to accept pre-built image
- Add evaluation cleanup hook
- Preserve existing timing and metrics

### Task 5: Workspace Validation
**Acceptance Criteria:**
- Confirm `/workspace` is writable
- GitHub mocking setup succeeds
- File operations work correctly
- Error messages are clear
- Debug mode preserves failed containers

**Implementation:**
- Add workspace writability test to container validation
- Verify GitHub mocking in writable workspace
- Update error messages for clarity
- Ensure debug preservation works

## Validation Commands

### 1. Image Reuse Verification
```bash
# Run evaluation and verify image is built once
task dev
./kiro-krew eval architect --sandbox --debug

# Check that same image is reused across test cases
docker images | grep kiro-eval
```

### 2. Clean Output Verification
```bash
# Run evaluation with sandbox and check build output
./kiro-krew eval builder --sandbox 2>&1 | grep -v "\\x1b\\|{\"stream\""
```

### 3. Workspace Writable Test
```bash
# Run evaluation that requires GitHub mocking
./kiro-krew eval krew-lead --sandbox --debug

# Check for absence of "read-only file system" errors
./kiro-krew eval architect --sandbox 2>&1 | grep -c "read-only file system"
# Should return 0
```

### 4. Performance Improvement Test
```bash
# Time evaluation run with multiple test cases
time ./kiro-krew eval architect --sandbox

# Should show significant reduction in total time due to image reuse
```

### 5. Backward Compatibility Test  
```bash
# Existing evaluation commands should work unchanged
./kiro-krew eval builder
./kiro-krew eval --list architect
./kiro-krew eval architect specific-test-case
```

## Error Handling

### Build Output Parser Errors:
- Malformed JSON in Docker stream → log warning, continue with raw output
- Network/IO errors during parsing → propagate with context
- Empty build output → return appropriate error message

### Image Manager Errors:
- Docker daemon unavailable → early failure with clear message
- Image build failures → preserve error context with clean output
- Platform detection failures → fallback to amd64 with warning

### Workspace Access Errors:
- Tmpfs mount failures → validate Docker version compatibility  
- Permission errors → provide troubleshooting guidance
- GitHub mocking setup failures → clear error with workspace path

## Risk Mitigation

### Breaking Changes Prevention:
- Maintain all existing public APIs
- Keep original native execution path unchanged  
- Preserve debug output in debug mode
- Maintain existing error handling patterns

### Performance Regression Prevention:
- Profile image building vs. reuse timing
- Monitor memory usage with cached images
- Validate cleanup doesn't leave orphaned containers
- Test with large evaluation runs

### Debug Capability Preservation:
- Keep raw Docker output available in debug mode
- Preserve failed containers for inspection
- Maintain image preservation for debugging
- Keep all existing debug logging

