# Design Specification: Debug Mode and Container Management for kiro-krew eval

**Issue Reference**: Closes #180

## Solution Approach

This enhancement adds comprehensive debugging capabilities to the `kiro-krew eval` command by implementing:

1. **Debug Mode (`--debug` flag)** - Enables verbose logging and container persistence for failed runs
2. **Container Registry** - Persistent tracking of all containers created during evaluation 
3. **Container Cleanup (`--cleanup` flag)** - Managed cleanup of debug containers and artifacts
4. **Enhanced Error Reporting** - Detailed container information in error messages

The solution extends the existing container sandbox infrastructure with minimal performance impact on normal operations while providing powerful debugging capabilities when needed.

## Architecture Overview

### Core Components

- **Enhanced CLI Interface** (`cmd/kiro-krew/cmd/eval.go`)
- **Debug-Aware Container Manager** (`internal/eval/sandbox/container.go`)
- **Container Registry** (new: `internal/eval/sandbox/registry.go`)
- **Debug Artifact Management** (new: `internal/eval/debug/`)

### Data Flow

```
eval --debug → Enhanced Container Creation → Registry Tracking → Verbose Logging
                     ↓                           ↓
             Failed Container Kept → Debug Commands Displayed
                     ↓                           ↓
             Cleanup Command → Registry Cleanup → Artifact Management
```

## Relevant Files

### Files to Modify
- `cmd/kiro-krew/cmd/eval.go` - Add debug and cleanup flags
- `internal/eval/types.go` - Extend RunOptions with debug fields
- `internal/eval/runner.go` - Pass debug options to container creation
- `internal/eval/sandbox/container.go` - Add debug logging and container persistence
- `.gitignore` - Add eval temporary artifacts exclusion

### Files to Create
- `internal/eval/sandbox/registry.go` - Container registry implementation
- `internal/eval/debug/artifacts.go` - Debug artifact management
- `internal/eval/debug/dockerfile.go` - Dockerfile preservation logic

### Configuration Files
- `.kiro-krew/evals/tmp/containers.json` - Runtime container registry
- `.kiro-krew/evals/tmp/dockerfiles/` - Timestamped Dockerfile storage

## Team Orchestration

This is a self-contained enhancement that:
- Extends existing sandbox infrastructure without breaking changes
- Uses established patterns for CLI flag handling
- Follows existing error handling and logging conventions
- Maintains backward compatibility for all existing functionality

## Step-by-Step Task Breakdown

### Task 1: Extend CLI Interface
**Acceptance Criteria:**
- Add `--debug` (`-d`) flag to eval command
- Add `--cleanup` flag to eval command  
- Flags are mutually exclusive with existing operations when appropriate
- Help text clearly describes new functionality

**Implementation:**
- Modify `cmd/kiro-krew/cmd/eval.go` to add new flags
- Update `RunOptions` in `internal/eval/types.go` with `Debug` and `Cleanup` fields
- Pass flags through to evaluation runner

### Task 2: Container Registry Implementation
**Acceptance Criteria:**
- Registry stored at `.kiro-krew/evals/tmp/containers.json`
- Tracks container ID, name, timestamp, eval run, status
- Thread-safe operations for concurrent access
- Handles missing/corrupted registry files gracefully
- Human-readable JSON format

**Implementation:**
- Create `internal/eval/sandbox/registry.go`
- Define `ContainerEntry` struct with required fields
- Implement `Registry` with Add, Remove, List, Clear methods
- Add file locking for concurrent access safety

### Task 3: Debug-Aware Container Management
**Acceptance Criteria:**
- Verbose logging for all Docker operations when debug enabled
- Generated Dockerfiles saved with timestamps
- Failed containers kept running in debug mode
- Debug commands printed for container inspection
- Container info included in error messages

**Implementation:**
- Extend `Container` struct in `container.go` with debug options
- Add `SetDebugMode(bool)` method
- Modify container lifecycle methods to respect debug flag
- Add verbose logging throughout container operations
- Integrate with container registry

### Task 4: Dockerfile Preservation
**Acceptance Criteria:**
- Dockerfiles saved to `.kiro-krew/evals/tmp/dockerfiles/`
- Filename format: `dockerfile-{timestamp}-{container-short-id}`
- Only saved when debug mode enabled
- Directory created automatically
- Reasonable disk space usage (consider rotation)

**Implementation:**
- Create `internal/eval/debug/dockerfile.go`
- Add `SaveDockerfile()` function with timestamp generation
- Integrate with `GenerateDockerfileWithPlatform()` method
- Add rotation logic for old files (keep last 50 or 7 days)

### Task 5: Enhanced Error Reporting
**Acceptance Criteria:**
- Error messages include container ID, image, platform
- kiro-cli installation errors show file permissions and sizes  
- Download progress visible during kiro-cli installation
- Checksum verification when available
- Debug commands displayed for failed containers

**Implementation:**
- Extend `ErrorContext` in `types.go` with container fields
- Modify `verifyKiroCLIInstallation()` for detailed errors
- Add progress reporting to `addKiroCLIToDockerfile()`
- Update error formatting throughout sandbox package

### Task 6: Container Cleanup Implementation
**Acceptance Criteria:**
- `--cleanup` stops and removes all tracked containers
- Clears container registry after successful cleanup
- Prompts user about Dockerfile preservation
- Handles missing/already removed containers gracefully
- Reports cleanup progress and results

**Implementation:**
- Add `RunCleanup()` function in runner.go
- Implement registry-driven container cleanup
- Add user prompt for Dockerfile preservation
- Robust error handling for missing containers
- Progress reporting during cleanup

### Task 7: Git Integration
**Acceptance Criteria:**
- `.kiro-krew/evals/tmp/` added to `.gitignore`
- Covers `containers.json`, `dockerfiles/`, and future artifacts
- Existing gitignore patterns preserved

**Implementation:**
- Update `.gitignore` with new exclusion pattern
- Verify pattern covers all temporary eval artifacts

## Validation Commands

### Debug Mode Verification
```bash
# Test debug mode with verbose output
kiro-krew eval --debug test-agent simple-case

# Verify container registry creation
ls -la .kiro-krew/evals/tmp/containers.json

# Check Dockerfile preservation
ls -la .kiro-krew/evals/tmp/dockerfiles/

# Test container persistence on failure
docker ps -a | grep kiro-eval
```

### Container Management Verification
```bash
# Test cleanup functionality
kiro-krew eval --cleanup

# Verify registry clearing
cat .kiro-krew/evals/tmp/containers.json

# Test cleanup with Dockerfile preservation
kiro-krew eval --cleanup  # Should prompt about Dockerfiles
```

### Error Reporting Verification
```bash
# Generate container error and verify enhanced reporting
kiro-krew eval --debug broken-test-case

# Verify error contains container ID, image, platform info
# Check that debug commands are displayed for failed containers
```

### Integration Testing
```bash
# Test normal mode performance (should be unaffected)
time kiro-krew eval test-agent benchmark-case

# Test flag combinations
kiro-krew eval --debug --list    # Should work
kiro-krew eval --cleanup --list  # Should error appropriately

# Test concurrent executions with registry
kiro-krew eval --debug agent1 & kiro-krew eval --debug agent2 &
```

## Implementation Notes

### Performance Considerations
- Debug logging uses conditional checks to avoid overhead in normal mode
- Dockerfile saving only occurs when debug flag is set
- Container registry operations are O(1) for add/remove operations
- File I/O for registry is minimized through in-memory caching

### Error Handling Strategy
- Registry corruption is handled by recreating empty registry
- Missing containers during cleanup are logged but don't fail the operation
- Dockerfile save failures are logged but don't abort evaluation
- All debug features degrade gracefully

### Security Considerations
- Container registry file permissions restricted to owner
- Dockerfile content is already safe (generated internally)
- No external input validation required for debug features
- Cleanup operations verify container ownership through registry

### Future Extensions
- Debug mode could be extended to other kiro-krew commands
- Container registry could track additional metadata
- Dockerfile preservation could include build context
- Cleanup could support selective removal by age/pattern