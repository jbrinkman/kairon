# Design Specification: Configurable Sandbox Resource Limits

**Issue**: #163  
**Title**: Add configurable sandbox resource limits to project configuration  
**Closes**: #163

## Solution Approach

Extend the existing configuration system to support optional sandbox resource limits while maintaining backward compatibility. The solution leverages the established YAML configuration pattern and adds a new `sandbox` section with sensible defaults.

### Key Design Principles
1. **Backward Compatibility**: All sandbox fields are optional with current hardcoded values as defaults
2. **Configuration-Driven**: Use existing config loading and validation patterns
3. **Type Safety**: Leverage Go's duration parsing and validation for robust config handling
4. **Minimal Surface Area**: Focus only on the core resource limit parameters

## Relevant Files

### Files to Modify
- `internal/config/config.go` - Add SandboxConfig struct and validation
- `internal/eval/runner.go` - Update createContainerConfig() to use config values
- `.kiro-krew/config.yaml` - Add commented sandbox section example

### Files to Create
- `internal/config/sandbox_test.go` - Unit tests for sandbox configuration
- `internal/eval/runner_config_test.go` - Tests for config integration

### Files for Reference
- `internal/config/config_test.go` - Testing patterns and setup utilities
- `internal/eval/runner_test.go` - Existing test structure

## Team Orchestration

This is a single-agent implementation focused on configuration extension:
1. **Config Layer**: Extend existing config struct and validation
2. **Runner Integration**: Modify container config creation to use config values
3. **Testing**: Add comprehensive unit tests for new functionality

## Step-by-Step Task Breakdown

### Task 1: Extend Configuration Structure
**Acceptance Criteria**:
- [ ] Add SandboxConfig struct with Image, WorkspaceDir, CPUCores, MemoryMB, Timeout fields
- [ ] Integrate SandboxConfig into main Config struct with proper defaults
- [ ] Support standard Go duration formats (e.g., "5m", "300s")
- [ ] Validate positive values for numeric fields

### Task 2: Update Container Configuration
**Acceptance Criteria**:
- [ ] Modify createContainerConfig() to accept config parameter
- [ ] Use config values with fallback to current defaults
- [ ] Convert CPUCores (float64) to CPUQuota (int64) microseconds correctly
- [ ] Convert MemoryMB (int) to Memory (int64) bytes correctly

### Task 3: Add Configuration Example
**Acceptance Criteria**:
- [ ] Add commented sandbox section to config.yaml template
- [ ] Include all available fields with current default values
- [ ] Add clear documentation comments

### Task 4: Comprehensive Testing
**Acceptance Criteria**:
- [ ] Unit tests for sandbox config parsing and validation
- [ ] Tests for config defaults and fallback behavior
- [ ] Tests for duration parsing and validation
- [ ] Integration tests for createContainerConfig() with various config inputs

## Implementation Details

### Configuration Structure
```go
type SandboxConfig struct {
    Image        string        `yaml:"image"`
    WorkspaceDir string        `yaml:"workspace_dir"`
    CPUCores     float64       `yaml:"cpu_cores"`
    MemoryMB     int           `yaml:"memory_mb"`
    Timeout      time.Duration `yaml:"timeout"`
}
```

### Default Values
- Image: `"alpine:3.19"`
- WorkspaceDir: `"/workspace"`
- CPUCores: `1.0`
- MemoryMB: `1024`
- Timeout: `5 * time.Minute`

### Validation Rules
- CPUCores > 0
- MemoryMB > 0
- Timeout > 0
- Image and WorkspaceDir non-empty when specified

## Validation Commands

```bash
# Build and test
go build ./...
go test ./internal/config/...
go test ./internal/eval/...

# Test config loading with sandbox section
echo 'sandbox:
  image: "alpine:3.20"
  cpu_cores: 2.0
  memory_mb: 2048
  timeout: 10m' >> test-config.yaml

# Verify backward compatibility
go test -run TestLoad_AllDefaultValues ./internal/config/

# Test config validation
go test -run TestSandboxConfig ./internal/config/
```

## Risk Assessment

**Low Risk**: This change only adds optional configuration fields with safe defaults, maintaining full backward compatibility. The implementation follows established patterns in the codebase and focuses on a well-defined, minimal scope.