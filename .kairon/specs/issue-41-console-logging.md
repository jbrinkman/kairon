# Console Logging Feature Design Specification

**Issue**: #41 - Add optional console logging for agent output with config toggle  
**Closes**: #41

## Solution Approach

Implement optional console logging using Go's `io.MultiWriter` to tee agent stdout/stderr to both existing log files and console output when enabled. The solution maintains full backward compatibility by defaulting console logging to false.

### Key Design Decisions

1. **Minimal Performance Impact**: Use conditional writer creation to avoid overhead when console logging is disabled
2. **ANSI Preservation**: Direct passthrough of agent output preserves rich formatting 
3. **Contextual Prefixing**: Add agent identification prefixes to distinguish console output from manager logs
4. **Backward Compatibility**: New config field defaults to false, existing behavior unchanged

## Relevant Files

### Files to Modify
- `internal/config/config.go` - Add ConsoleLogging field to Config struct
- `internal/agent/manager.go` - Implement tee writer logic in Spawn() and retryAgent() methods
- `.kiro-krew/config.yaml` (template) - Document new console_logging option

### Files Referenced (No Changes)
- `cmd/kiro-krew/main.go` - Uses config.Load(), will automatically pick up new field
- `internal/tui/tui.go` - Consumes manager, no changes needed
- `internal/watcher/watcher.go` - Uses manager, no changes needed

## Team Orchestration

This is a self-contained feature requiring coordination between:
1. **Config System** - Extend configuration structure
2. **Agent Manager** - Implement console output logic
3. **Documentation** - Update config template

No external API changes or database modifications required.

## Step-by-Step Task Breakdown

### Task 1: Extend Configuration System
**File**: `internal/config/config.go`
**Changes**:
- Add `ConsoleLogging bool` field to Config struct with `yaml:"console_logging"` tag
- Add ConsoleLogging to default config initialization (set to false)

**Acceptance Criteria**:
- Config struct includes new field
- Field defaults to false for backward compatibility
- YAML unmarshaling works correctly

### Task 2: Implement Console Tee Writer
**File**: `internal/agent/manager.go` 
**Changes**:
- Import `io` package for MultiWriter
- In `Spawn()` method (lines 85-86):
  - Create conditional writer based on `m.config.ConsoleLogging`
  - If enabled: use `io.MultiWriter(agentLogFile, prefixedConsoleWriter)`
  - If disabled: use `agentLogFile` directly
- In `retryAgent()` method (lines 333-334):
  - Apply same conditional writer logic
- Add helper function to create prefixed console writer:
  ```go
  func (m *Manager) createPrefixedWriter(issueNumber int) io.Writer
  ```

**Technical Implementation**:
- Use `fmt.Sprintf("[agent issue-%d] ", issueNumber)` as prefix format
- Implement line-by-line prefixing to handle multiline output correctly
- Preserve ANSI escape sequences by avoiding string manipulation of content

**Acceptance Criteria**:
- When `console_logging: false`, behavior identical to current implementation
- When `console_logging: true`, agent output appears in console with prefixes
- ANSI formatting preserved in console output
- Log file output unchanged
- No performance impact when console logging disabled

### Task 3: Update Configuration Template
**File**: Template config file for `kiro-krew init`
**Changes**:
- Add commented example: `# console_logging: false  # Enable real-time agent output in console`

**Acceptance Criteria**:
- New installations have documented console_logging option
- Default remains false for backward compatibility

## Validation Commands

```bash
# Test 1: Verify config loading with new field
cd test-project
echo "console_logging: true" >> .kiro-krew/config.yaml
kiro-krew # Should start without config errors

# Test 2: Test console output enabled
# (Configure console_logging: true, spawn agent, verify output appears in console)

# Test 3: Test backward compatibility  
# (Remove console_logging field, verify agent works normally)

# Test 4: Test ANSI preservation
# (Enable console logging, verify colored output in console)

# Test 5: Performance baseline
# (Measure agent spawn time with console_logging: false vs true)
```

## Implementation Notes

### Prefix Writer Implementation
```go
type prefixWriter struct {
    writer io.Writer
    prefix string
    atLineStart bool
}

func (pw *prefixWriter) Write(p []byte) (n int, err error) {
    // Handle line-by-line prefixing while preserving ANSI sequences
}
```

### MultiWriter Usage
```go
var cmdWriter io.Writer = agentLogFile
if m.config.ConsoleLogging {
    prefixed := &prefixWriter{
        writer: os.Stdout,
        prefix: fmt.Sprintf("[agent issue-%d] ", issueNumber),
        atLineStart: true,
    }
    cmdWriter = io.MultiWriter(agentLogFile, prefixed)
}
cmd.Stdout = cmdWriter
cmd.Stderr = cmdWriter
```

This design ensures minimal code changes while providing the requested functionality with full backward compatibility and optimal performance characteristics.
