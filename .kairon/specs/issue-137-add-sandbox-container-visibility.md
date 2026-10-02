# Add Sandbox Container Visibility to Evaluation Output

**Issue**: #137  
**Closes**: #137  
**Created**: 2026-06-21  

## Solution Approach

Enhance the evaluation framework's container sandbox mode with comprehensive logging to provide users clear visibility into container lifecycle and execution status. The solution adds structured logging at key container operations without impacting native evaluation mode performance.

### Key Design Principles

1. **Non-intrusive Integration** - Container logging only activates when sandbox mode is enabled
2. **Progressive Information Display** - Show container setup details, execution status, and cleanup progress 
3. **Performance Impact Awareness** - Distinguish container overhead from test execution time
4. **Consistent Formatting** - Use structured log format with timestamps and clear visual indicators
5. **Graceful Degradation** - Handle container failures with informative error messages

## Relevant Files

### Files to Modify

- `internal/eval/runner.go` 
  - **Primary Focus**: `invokeAgentInContainer()` function - add logging around container operations
  - Add container startup logging with resource limits display
  - Add execution status messages during test runs
  - Add cleanup logging with error handling
  - Integrate `[SANDBOX]` indicators in test result output

- `internal/eval/sandbox/container.go`
  - Add logging methods for container lifecycle events
  - Enhance `NewContainer()`, `Create()`, `Start()`, and `Cleanup()` with structured logging
  - Add container ID extraction and resource limit display methods
  - Add timing measurements for container operations

- `internal/eval/types.go`
  - Add `SandboxMetrics` field to `CaseResult` for container overhead tracking
  - Add logging configuration options to `ContainerConfig`
  - Extend `ErrorContext` with container-specific error fields

- `cmd/kiro-krew/cmd/eval.go`
  - Add `--verbose` flag for enhanced container debugging information
  - Extend help documentation for sandbox logging options

### Files for Reference

- `internal/eval/sandbox/resource_limits.go` - For displaying resource limit values
- `internal/eval/progress.go` - For consistent progress indicator patterns
- `docs/evaluation.md` - For documenting new logging behavior

## Team Orchestration

### Container Infrastructure Team
- Implement structured logging in `sandbox.Container` methods
- Add container metrics collection (startup time, resource usage)
- Ensure Docker error handling provides actionable feedback

### Evaluation Framework Team  
- Integrate container logging into `invokeAgentInContainer()`
- Add sandbox indicators to test result formatting
- Preserve existing native evaluation mode behavior

### CLI/UX Team
- Add `--verbose` flag for detailed container debugging
- Update help documentation and error messages
- Ensure log formatting consistency with existing TUI patterns

## Step-by-Step Task Breakdown

### Phase 1: Container Lifecycle Logging
**Acceptance Criteria**: Container startup displays detailed information

**Tasks**:
1. **Add container startup logging** in `invokeAgentInContainer()`
   - Display image name, short container ID, resource limits
   - Format: `Starting sandbox container: alpine:3.19 [abc123] (1.0 CPU, 1GB RAM, 2m timeout)`
   - Show timing for container creation and startup

2. **Enhance `sandbox.Container` with logging methods**
   - Add `LogStartup()` method to display container creation details
   - Add `GetContainerInfo()` for ID and resource extraction
   - Add timing measurement utilities

3. **Add structured error handling**
   - Capture Docker errors with context (image pull failures, resource constraints)
   - Provide actionable error messages for common issues

**Validation Commands**:
```bash
# Test basic container startup logging
kiro-cli eval builder --sandbox --case simple-task

# Verify resource limit display
kiro-cli eval builder --sandbox --resource-limit cpu=0.5,memory=512MB --case simple-task

# Test error conditions
kiro-cli eval builder --sandbox --resource-limit memory=999999GB --case simple-task
```

### Phase 2: Execution Status Logging  
**Acceptance Criteria**: Clear status messages during test execution

**Tasks**:
1. **Add execution status messages** in test case evaluation loop
   - Display "Running test in sandbox..." when invoking kiro-cli
   - Add progress indicators for long-running tests (>10 seconds)
   - Show intermediate status without disrupting test output capture

2. **Integrate sandbox indicators** in test result display
   - Add `[SANDBOX]` prefix to test case names/results 
   - Distinguish sandbox vs native execution in result output
   - Maintain existing result formatting structure

3. **Add container overhead tracking**
   - Measure container startup time vs test execution time
   - Include overhead metrics in performance data
   - Display timing breakdown in verbose mode

**Validation Commands**:
```bash
# Test execution status display
kiro-cli eval architect --sandbox --case code-analysis

# Verify sandbox indicators in output
kiro-cli eval validator --sandbox | grep "\[SANDBOX\]"

# Check performance overhead tracking
kiro-cli eval builder --sandbox --perf --case build-verification
```

### Phase 3: Cleanup and Error Handling
**Acceptance Criteria**: Comprehensive cleanup logging and error recovery

**Tasks**:
1. **Add cleanup status logging** in container lifecycle
   - Display "Shutting down sandbox container..." message
   - Show "Sandbox cleanup complete" on success
   - Handle cleanup failures with retry logic and error reporting

2. **Enhance error context** for container failures
   - Extend `ErrorContext` with container ID, image, and Docker error details
   - Provide debugging hints for common container issues
   - Log container state during failures for troubleshooting

3. **Add graceful failure handling**
   - Ensure containers are cleaned up even after test failures
   - Provide clear error messages for Docker daemon issues
   - Fall back to informative error when container cleanup fails

**Validation Commands**:
```bash
# Test normal cleanup
kiro-cli eval builder --sandbox --case quick-test

# Test cleanup after failure (simulate Docker issue)
docker stop $(docker ps -q) && kiro-cli eval builder --sandbox --case simple-task

# Test error recovery
kiro-cli eval builder --sandbox --resource-limit timeout=1s --case long-running-task
```

### Phase 4: Verbose Mode and Documentation
**Acceptance Criteria**: Enhanced debugging mode and complete documentation

**Tasks**:
1. **Implement `--verbose` flag** for detailed container debugging
   - Show Docker commands being executed
   - Display container environment variables and mounted paths
   - Include detailed resource usage statistics

2. **Add comprehensive logging configuration**
   - Allow selective logging levels (startup, execution, cleanup)
   - Provide JSON output mode for programmatic consumption
   - Add timestamp precision options

3. **Update documentation** and help text
   - Document new logging behavior in evaluation docs
   - Add troubleshooting guide for container issues
   - Update CLI help with sandbox logging examples

**Validation Commands**:
```bash
# Test verbose mode
kiro-cli eval builder --sandbox --verbose --case integration-test

# Test selective logging (if implemented)
kiro-cli eval validator --sandbox --log-level=startup,cleanup --case validation-test

# Verify help documentation
kiro-cli eval --help | grep -A5 sandbox
```

## Container Information Display Format

### Startup Message Template
```
Starting sandbox container: {image} [{container_id}] ({cpu} CPU, {memory}, {timeout} timeout)
  Resource Limits: CPU={cpu_quota}μs Memory={memory_bytes}B Timeout={timeout}
  Workspace: {workspace_dir}
  Environment: KIRO_CLI_DISABLE_TELEMETRY=1
```

### Execution Status Template  
```
Running test in sandbox... [progress indicators for >10s tests]
```

### Result Display Template
```
[SANDBOX] test_case_name ✅ 85% (threshold: 80%)
Container overhead: 2.1s, Test execution: 8.4s
```

### Cleanup Message Template
```
Shutting down sandbox container... ✅ Sandbox cleanup complete (0.8s)
```

## Error Handling Strategy

### Container Creation Failures
- **Docker not running**: "❌ Docker is not running. Start Docker and try again"
- **Image pull failure**: "❌ Failed to pull image {image}: {error}. Check internet connection"
- **Resource constraints**: "❌ Invalid resource limits: {details}. Adjust --resource-limit values"

### Execution Failures
- **Timeout**: "⏱️ Container execution timeout after {timeout}. Consider increasing --resource-limit timeout="
- **OOM kill**: "💾 Container ran out of memory ({memory} limit). Consider increasing --resource-limit memory="
- **Command failure**: Include full command, exit code, and stderr in ErrorContext

### Cleanup Failures  
- **Container stop failure**: "⚠️ Failed to stop container gracefully, forcing removal"
- **Cleanup timeout**: "⚠️ Container cleanup timed out, container may need manual removal"
- **Docker daemon issue**: "❌ Docker daemon error during cleanup: {error}"

## Performance Considerations

### Minimal Logging Overhead
- Use structured logging only when sandbox mode is enabled
- Buffer log messages to avoid interrupting test execution
- Measure and report container overhead separately from test timing

### Resource Usage Display
- Show human-readable resource limits (1.5 CPU, 2GB RAM)  
- Convert microsecond CPU quotas to fractional core display
- Display timeout in user-friendly format (2m30s vs 150000ms)

### Progress Indicators
- Show progress for container operations taking >3 seconds
- Use spinner/dots for operations without measurable progress
- Avoid excessive log output that might overwhelm TUI

## Validation Commands

### Integration Testing
```bash
# Run complete sandbox evaluation with logging
kiro-cli eval architect --sandbox --case comprehensive-analysis

# Test resource limit combinations
kiro-cli eval builder --sandbox --resource-limit cpu=0.5,memory=1GB,timeout=3m --case build-test

# Test failure scenarios
kiro-cli eval validator --sandbox --resource-limit timeout=5s --case long-validation

# Test verbose mode
kiro-cli eval documenter --sandbox --verbose --case documentation-generation
```

### Regression Testing  
```bash
# Ensure native mode unchanged
kiro-cli eval builder --case simple-task

# Verify no-sandbox flag works
kiro-cli eval architect --no-sandbox --case quick-analysis  

# Test with existing performance investigation
kiro-cli eval validator --perf --case validation-suite
```

### Error Condition Testing
```bash
# Test without Docker running
sudo systemctl stop docker && kiro-cli eval builder --sandbox --case simple-task

# Test with invalid resource limits
kiro-cli eval builder --sandbox --resource-limit cpu=999,memory=-1 --case simple-task

# Test cleanup after process kill
kiro-cli eval builder --sandbox --case long-task & kill %1
```

## Constraints Compliance

### No Impact on Native Mode
- All container logging code paths are conditional on `cConfig != nil`
- Zero performance overhead when `--sandbox` flag is not used
- Existing evaluation test cases continue to work unchanged

### Informative but Not Excessive
- Container startup info fits on 2-3 lines maximum
- Execution status updates only for tests >10 seconds
- Cleanup messages are single line unless errors occur

### Security Considerations
- Never log sensitive information (API keys, tokens)
- Container IDs shown are short form (first 7 characters)
- Docker errors are sanitized to avoid information disclosure

### Minimal Performance Impact
- Container logging operations add <100ms total overhead
- Progress indicators don't interfere with test output capture
- Resource usage display calculated once at startup