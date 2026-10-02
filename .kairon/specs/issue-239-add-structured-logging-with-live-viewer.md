# Issue #239: Add Structured Logging with Live Viewer

**Closes #239**

## Executive Summary

Implement a structured logging infrastructure using `github.com/charmbracelet/log` with a live viewer tab for runtime debugging. The immediate goal is to diagnose planning tab ACP message flow issues where messages are sent but responses don't appear. The system provides on-demand logging (only when viewer tab is open), file rotation, and comprehensive instrumentation throughout the codebase.

## Problem Statement

The planning tab currently lacks visibility into the ACP message lifecycle. When a user sends a message:
1. The UI shows the prompt is added to history
2. The state changes to executing
3. No response text appears

Without structured logging, it's impossible to determine whether:
- The ACP connection is active
- The message was sent successfully
- The streaming response channel is receiving data
- Session updates are being processed
- Where in the pipeline the failure occurs

This affects not just the planning tab but debugging across the entire application.

## Solution Approach

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│ Application Components                                       │
│ (TUI, ACP Client, Session Manager, Watcher, etc.)          │
└───────────────┬─────────────────────────────────────────────┘
                │ log events
                ↓
┌─────────────────────────────────────────────────────────────┐
│ Global Logger (charmbracelet/log)                           │
│ - Configured level (debug/info/warn/error)                  │
│ - Multiple handlers (ring buffer + file)                    │
└───────┬─────────────────────────────────────────────────────┘
        │
        ├──────────────────┬──────────────────────────────────┐
        ↓                  ↓                                  ↓
┌──────────────┐  ┌───────────────┐              ┌──────────────────┐
│ Ring Buffer  │  │ File Handler  │              │ Log Viewer Tab   │
│ - FIFO queue │  │ - Rotation    │              │ - Streaming view │
│ - Max lines  │  │ - Timestamps  │←─────reads───│ - Scrolling      │
│ - O(1) ops   │  │ - .kiro-krew/ │              │ - Selection/copy │
└──────┬───────┘  │   logs/       │              └──────────────────┘
       │          └───────────────┘
       │ buffer read
       └──────────────────────────────────┐
                                          │
                            ┌─────────────┴──────────────┐
                            │ Log Viewer Tab             │
                            │ Consumes from ring buffer  │
                            └────────────────────────────┘
```

### Key Design Decisions

1. **On-Demand Logging**: Logging only occurs when the log viewer tab is open
   - Reduces performance impact during normal operation
   - File logging tied to viewer lifecycle
   - Ring buffer cleared when viewer closes

2. **Single Log Tab Constraint**: Only one log viewer tab can exist at a time
   - Simplifies buffer management
   - Prevents resource contention
   - Clear UX around logging state

3. **Ring Buffer Implementation**: In-memory circular buffer for efficient log storage
   - FIFO behavior with O(1) add/remove operations
   - Configurable max size (default 10,000 lines)
   - Thread-safe for concurrent access

4. **File Rotation Strategy**: Time-based rotation with minute resolution
   - Files named: `debug-YYYY-MM-DD-HHMM.log`
   - Rotation on size threshold (configurable MB limit)
   - Viewer display remains uninterrupted during rotation

5. **Command Parameter Overrides**: Session-only configuration overrides
   - `log [level] [size]` parameters override config temporarily
   - Closing and reopening reverts to config file defaults
   - Persistent changes require editing `.kiro-krew/config.yaml`

## Relevant Files

### New Files to Create

1. **`internal/logging/logger.go`**
   - Global logger initialization with charmbracelet/log
   - Multi-handler setup (ring buffer + file)
   - Level configuration and dynamic updates
   - Thread-safe logger access

2. **`internal/logging/ring_buffer.go`**
   - Circular buffer implementation with FIFO behavior
   - Thread-safe operations (Add, Get, Clear, Size)
   - Iterator interface for consumption
   - O(1) performance guarantees

3. **`internal/logging/file_handler.go`**
   - Custom charmbracelet/log handler for file output
   - File rotation logic (size-based)
   - Timestamp-based file naming
   - Directory creation and management

4. **`internal/tui/log_tab.go`**
   - Tab interface implementation for log viewer
   - Streaming log display with viewport
   - Free scrolling support (up/down/pgup/pgdown/home/end)
   - Text selection and clipboard copy
   - Auto-scroll behavior for new entries

5. **`internal/logging/types.go`**
   - LogEntry structure with timestamp, level, message, metadata
   - Configuration structures for ring buffer and file output
   - Constants for log levels and defaults

### Files to Modify

1. **`internal/config/config.go`**
   - Add `LoggingConfig` struct with fields:
     - `DefaultLevel string` (debug/info/warn/error)
     - `MaxBufferLines int` (ring buffer size)
     - `MaxFileSizeMB int` (rotation threshold)
     - `LogDir string` (output directory)
   - Update `Load()` to parse logging section
   - Validation for logging config fields

2. **`.kiro-krew/config.yaml`**
   - Add logging configuration section with defaults:
     ```yaml
     logging:
       default_level: "info"
       max_buffer_lines: 10000
       max_file_size_mb: 100
       log_dir: ".kiro-krew/logs"
     ```

3. **`internal/tui/command_registry.go`**
   - Register new `log` command with optional parameters
   - Support `log [level] [size]` syntax
   - Command description and help text

4. **`internal/tui/commands.go`**
   - Implement `handleLog()` function
   - Parse optional level and size parameters
   - Create/navigate to log viewer tab
   - Handle existing tab scenarios (prompt user)
   - Initialize logging subsystem when viewer opens

5. **`internal/tui/tui.go`**
   - Initialize logging subsystem on startup (but inactive)
   - Activate logging when log viewer tab opens
   - Deactivate logging when log viewer tab closes
   - Hook log tab lifecycle to global logger state

6. **`internal/tui/tab_manager.go`**
   - Add `TabTypeLog` constant
   - Enforce single log tab constraint
   - Handle log tab closure cleanup

7. **`internal/acp/client.go`**
   - Add structured logging throughout:
     - Connection lifecycle (Connect, Disconnect)
     - Message sending (SendMessage, StreamMessage)
     - Session creation and management
     - Error conditions and retries
   - Log with context: session IDs, agent names, timestamps

8. **`internal/tui/planning_tab.go`**
   - Add structured logging for:
     - Message submission
     - Stream start/chunk/done/error events
     - State transitions (idle → active → completed/failed)
     - Viewport updates and content rendering
   - Log with context: tab ID, message count, streaming state

9. **`internal/session/manager.go`**
   - Add logging for session lifecycle events
   - Log session creation, loading, saving, deletion
   - Log session state transitions

10. **`internal/session/planner.go`**
    - Add logging for planning session operations
    - Log ACP connection state changes
    - Log context usage updates

11. **`cmd/kiro-krew/main.go`**
    - Initialize logging subsystem early in startup
    - Configure default logger before TUI starts

12. **`go.mod`**
    - Add dependency: `github.com/charmbracelet/log v0.5.0`

## Team Orchestration

### Task Dependencies

```
Task 1 (Logging Core) ────┐
Task 2 (Ring Buffer)  ────┼──→ Task 4 (Log Viewer Tab)
Task 3 (File Handler) ────┘

Task 5 (Config + Commands) ──→ Task 4 (Log Viewer Tab)

Task 4 (Log Viewer Tab) ──→ Task 6 (TUI Integration)

Task 6 (TUI Integration) ──→ Task 7 (Instrumentation)
```

### Parallel Execution Groups

**Group 1 - Foundation (parallel execution)**
- Task 1: Logging Core Infrastructure
- Task 2: Ring Buffer Implementation  
- Task 3: File Handler with Rotation

**Group 2 - Configuration (parallel with Group 1)**
- Task 5: Configuration and Command Support

**Group 3 - UI Integration (depends on Groups 1 & 2)**
- Task 4: Log Viewer Tab Implementation
- Task 6: TUI Lifecycle Integration

**Group 4 - Instrumentation (depends on Group 3)**
- Task 7: Comprehensive Codebase Instrumentation

## Step-by-Step Task Breakdown

### Task 1: Logging Core Infrastructure
**File**: `internal/logging/logger.go`

**Acceptance Criteria**:
- Create global logger instance using `github.com/charmbracelet/log`
- Implement `Initialize(level, handlers)` function
- Implement `SetLevel(level)` for dynamic level changes
- Implement `GetLogger()` for global access
- Support multiple concurrent handlers
- Thread-safe logger access
- Log levels: debug, info, warn, error

**Dependencies**: None (can execute first)

**Implementation Notes**:
- Use singleton pattern for global logger
- Initialize as inactive (no handlers) by default
- Activate only when log viewer tab opens
- Provide helper functions: `Debug()`, `Info()`, `Warn()`, `Error()`
- Include structured fields support (key-value pairs)

---

### Task 2: Ring Buffer Implementation
**File**: `internal/logging/ring_buffer.go`

**Acceptance Criteria**:
- Implement circular buffer with configurable max size
- FIFO behavior when buffer is full
- Thread-safe operations: `Add()`, `Get()`, `Clear()`, `Size()`
- O(1) performance for add/remove operations
- Iterator interface for reading without consuming
- Memory-efficient storage of log entries

**Dependencies**: None (can execute in parallel with Task 1)

**Implementation Notes**:
- Use slice-based circular buffer with head/tail pointers
- Mutex protection for concurrent access
- Store `LogEntry` structs with timestamp, level, message, metadata
- Support read-only iteration for multiple consumers
- Implement `GetRecent(n)` for retrieving last N entries

---

### Task 3: File Handler with Rotation
**File**: `internal/logging/file_handler.go`

**Acceptance Criteria**:
- Custom handler implementing charmbracelet/log Handler interface
- Write logs to configurable directory (`.kiro-krew/logs/`)
- File naming: `debug-YYYY-MM-DD-HHMM.log` (minute resolution)
- Size-based rotation when max size exceeded
- Create new file with current timestamp on rotation
- Directory creation with proper permissions
- Thread-safe file writing

**Dependencies**: None (can execute in parallel with Tasks 1 & 2)

**Implementation Notes**:
- Monitor file size before each write
- Close current file and create new one on rotation
- Include timestamp, level, and message in log format
- Support structured fields in output format
- Handle file write errors gracefully
- Clean up old log files (optional, based on retention policy)

---

### Task 4: Log Viewer Tab Implementation
**File**: `internal/tui/log_tab.go`

**Acceptance Criteria**:
- Implement `Tab` interface for log viewer
- Stream log entries from ring buffer
- Scrollable viewport with free navigation
- Support keyboard navigation: up/down, pgup/pgdown, home/end
- Auto-scroll to bottom for new entries (with smart threshold)
- Text selection and clipboard copy support
- Display timestamp, level, and message
- Color-coded log levels (error=red, warn=yellow, info=blue, debug=gray)
- Clean tab closure with resource cleanup

**Dependencies**: Tasks 1, 2, 5 (requires logging core, ring buffer, and config)

**Implementation Notes**:
- Use `viewport.Model` from bubbletea for scrolling
- Poll ring buffer for new entries every 100ms
- Implement smart auto-scroll: only scroll if near bottom (>85%)
- Format log entries with level colors from styles
- Handle tab resize gracefully
- Clear ring buffer on tab closure
- Implement `IsClosable()` to return true (always closable)

---

### Task 5: Configuration and Command Support
**Files**: `internal/config/config.go`, `.kiro-krew/config.yaml`, `internal/tui/command_registry.go`, `internal/tui/commands.go`

**Acceptance Criteria**:
- Add `LoggingConfig` struct to `config.Config`
- Parse logging section from YAML with defaults
- Validate logging configuration fields
- Register `log [level] [size]` command in registry
- Implement `handleLog()` command handler
- Support optional level parameter (overrides config)
- Support optional size parameter (overrides config)
- Handle existing log tab scenario (prompt user)
- Parameter overrides are session-only (don't persist)

**Dependencies**: None (can execute in parallel with Group 1)

**Implementation Notes**:
- Default values: level="info", max_buffer_lines=10000, max_file_size_mb=100
- Validate level is one of: debug, info, warn, error
- Validate max_buffer_lines > 0
- Validate max_file_size_mb > 0
- Command parsing: split on whitespace, validate parameters
- Store override values in log tab instance, not global config
- Revert to config defaults on tab close/reopen

---

### Task 6: TUI Lifecycle Integration
**Files**: `internal/tui/tui.go`, `internal/tui/tab_manager.go`, `cmd/kiro-krew/main.go`

**Acceptance Criteria**:
- Initialize logging subsystem in `main.go` (inactive state)
- Add `TabTypeLog` constant to tab system
- Enforce single log tab constraint in `TabManager`
- Activate logging when log viewer tab opens
- Attach ring buffer handler to logger
- Attach file handler to logger
- Deactivate logging when log viewer tab closes
- Remove handlers from logger
- Stop file writing on tab closure
- Clear ring buffer on tab closure

**Dependencies**: Task 4 (requires log viewer tab implementation)

**Implementation Notes**:
- Logging system starts inactive (no handlers attached)
- Opening log tab triggers: create handlers, attach to logger, start file writing
- Closing log tab triggers: remove handlers, close file, clear buffer
- Enforce one log tab rule before creating new tab
- If log tab exists and user runs `log` command, navigate to existing tab
- If log tab exists with params, prompt: view existing or close & create new

---

### Task 7: Comprehensive Codebase Instrumentation
**Files**: `internal/acp/client.go`, `internal/tui/planning_tab.go`, `internal/session/manager.go`, `internal/session/planner.go`, and others as needed

**Acceptance Criteria**:
- Add structured logging to ACP client:
  - Connection lifecycle (connect, disconnect, reconnect)
  - Message sending (request, response, streaming)
  - Session creation and management
  - Permission requests and approvals
  - Error conditions with stack context
- Add structured logging to planning tab:
  - User message submission
  - Streaming response events (start, text, error, done)
  - State transitions (idle → active → completed/failed)
  - Viewport updates and rendering
  - Focus changes and input handling
- Add structured logging to session management:
  - Session creation, loading, saving, deletion
  - State transitions and persistence
  - Planning session lifecycle
  - ACP connection state tracking
- All logs include relevant context (IDs, timestamps, counts, states)
- Error logs include error messages and context
- Debug logs for detailed flow tracing

**Dependencies**: Task 6 (requires TUI integration complete)

**Implementation Notes**:
- Use structured fields for context: `log.Info("message", "key", value)`
- ACP client logging:
  - Debug: detailed message flow, session IDs, agent names
  - Info: connection state changes, message sent/received
  - Warn: retry attempts, connection issues
  - Error: connection failures, message send failures
- Planning tab logging:
  - Debug: viewport updates, input handling, focus changes
  - Info: message submission, streaming state changes
  - Warn: stream errors, incomplete responses
  - Error: ACP connection failures, critical errors
- Session management logging:
  - Debug: persistence operations, state serialization
  - Info: session lifecycle events, state transitions
  - Warn: cleanup failures, orphaned sessions
  - Error: persistence failures, corruption issues
- Focus on planning tab ACP flow as highest priority

---

## Validation Commands

### Build and Test
```bash
# Build the application
go build ./cmd/kiro-krew

# Run unit tests for logging package
go test -v ./internal/logging/...

# Run unit tests for log tab
go test -v ./internal/tui/log_tab_test.go

# Run integration tests
go test -v ./internal/tui/integration_test.go
```

### Manual Testing Scenarios

#### Scenario 1: Basic Log Viewing
```bash
# Start kiro-krew
./kiro-krew

# Open log viewer with default settings
kiro-krew> log

# Verify:
# - Log tab opens
# - Tab title shows "Logs"
# - Viewer displays live log stream
# - Timestamp, level, and message visible
# - File created: .kiro-krew/logs/debug-YYYY-MM-DD-HHMM.log
```

#### Scenario 2: Level Override
```bash
# Open log viewer with debug level
kiro-krew> log debug

# Verify:
# - Debug messages visible in viewer
# - More verbose output than default
# - Close tab and reopen
kiro-krew> log

# Verify:
# - Level reverted to config default (info)
# - Less verbose output
```

#### Scenario 3: Buffer Size Override
```bash
# Open log viewer with larger buffer
kiro-krew> log info 20000

# Generate many logs (trigger watcher, open planning tabs, etc.)
# Verify:
# - Buffer holds up to 20000 lines
# - FIFO behavior when buffer full
# - Close and reopen
kiro-krew> log

# Verify:
# - Buffer size reverted to config default (10000)
```

#### Scenario 4: Planning Tab ACP Flow
```bash
# Open log viewer with debug level
kiro-krew> log debug

# Open planning tab
kiro-krew> plan test-debug

# Send a message in planning tab
[planner] > Hello, can you help me create an issue?

# Monitor log viewer for:
# - "Planning tab: user message submitted" (info)
# - "ACP: sending message request" (debug)
# - "ACP: session created" (info)
# - "ACP: prompt sent" (debug)
# - "Planning tab: stream started" (info)
# - "ACP: text chunk received" (debug)
# - "Planning tab: appending text to response" (debug)
# - "Planning tab: stream completed" (info)
# - "Planning tab: assistant message added" (info)
```

#### Scenario 5: File Rotation
```bash
# Set low max_file_size_mb in config (e.g., 1 MB)
# Open log viewer
kiro-krew> log debug

# Generate logs rapidly (multiple planning sessions, watcher activity)
# Verify:
# - New log file created when size threshold exceeded
# - New filename has updated timestamp
# - Viewer display uninterrupted during rotation
# - Old log file remains intact
```

#### Scenario 6: Single Tab Constraint
```bash
# Open log viewer
kiro-krew> log

# Try to open another log viewer
kiro-krew> log debug 20000

# Verify:
# - Prompt appears: "Log viewer already open. View existing or start new?"
# - Options: "View existing" (navigate to tab) or "New session" (close old, open new)
```

#### Scenario 7: Scrolling and Navigation
```bash
# Open log viewer with debug level
kiro-krew> log debug

# Generate many logs to fill viewport
# Test keyboard navigation:
# - Up/Down arrows: scroll line by line
# - PgUp/PgDown: scroll by half page
# - Home: jump to top
# - End: jump to bottom

# Verify:
# - Smooth scrolling behavior
# - Auto-scroll stops when manually scrolling up
# - Auto-scroll resumes when scrolling to bottom (>85%)
```

#### Scenario 8: Tab Closure and Cleanup
```bash
# Open log viewer
kiro-krew> log

# Generate some logs
# Close log tab (Ctrl+W or close command)

# Verify:
# - Log file closed
# - No further file writing
# - Ring buffer cleared
# - Log handler removed from logger
# - No memory leaks (monitor with activity monitor)
```

## Configuration Schema

### `.kiro-krew/config.yaml` Addition
```yaml
# Structured logging configuration
logging:
  # Default log level: debug, info, warn, error
  default_level: "info"
  
  # Maximum lines in memory ring buffer (FIFO)
  max_buffer_lines: 10000
  
  # Maximum log file size in MB before rotation
  max_file_size_mb: 100
  
  # Directory for log files (relative to project root)
  log_dir: ".kiro-krew/logs"
```

## Performance Considerations

1. **Ring Buffer Efficiency**
   - O(1) add/remove operations using circular buffer
   - Pre-allocated slice to avoid frequent reallocations
   - Bounded memory usage (max_buffer_lines * avg_entry_size)

2. **File I/O Optimization**
   - Buffered writes to reduce syscall overhead
   - Async file writing with goroutine and channel
   - File descriptor caching (reuse open file handle)

3. **Log Level Filtering**
   - Check log level before expensive operations (string formatting)
   - Short-circuit when level below threshold
   - Minimize allocations in hot paths

4. **Viewer Update Rate**
   - Poll ring buffer every 100ms (configurable)
   - Batch updates when multiple entries available
   - Viewport rendering only when tab is visible

## Security Considerations

1. **Log Content Sanitization**
   - Avoid logging sensitive data (tokens, passwords, API keys)
   - Redact session IDs in production mode (optional config)
   - Truncate very long messages to prevent buffer exhaustion

2. **File Permissions**
   - Create log directory with restricted permissions (0700)
   - Write log files with owner-only read (0600)
   - Validate log directory path to prevent directory traversal

3. **Resource Limits**
   - Enforce max_buffer_lines to prevent memory exhaustion
   - Enforce max_file_size_mb to prevent disk exhaustion
   - Implement log rotation to manage disk usage

## Error Handling

1. **File Write Failures**
   - Log to stderr if file handler fails
   - Continue ring buffer operation even if file fails
   - Display warning in log viewer if file writing disabled

2. **Buffer Overflow**
   - Drop oldest entries when buffer full (FIFO)
   - Log warning when buffer capacity reached
   - Increase buffer size recommendation in warning

3. **Invalid Configuration**
   - Use default values for invalid config entries
   - Log warnings for invalid config values
   - Validate config on load, fail fast if critical errors

## Future Enhancements (Out of Scope for This PR)

1. **Log Filtering**
   - Filter by log level in viewer
   - Filter by component/package
   - Search within logs

2. **Log Export**
   - Export current buffer to file
   - Export with timestamp range
   - Export as JSON for analysis

3. **Log Analysis**
   - View log statistics (counts by level)
   - Highlight errors/warnings
   - Jump to next/previous error

4. **Distributed Logging**
   - Send logs to remote server (optional)
   - Support OpenTelemetry integration
   - Correlate logs across agent instances

## References

- [charmbracelet/log Documentation](https://github.com/charmbracelet/log)
- [Bubbletea Viewport Example](https://github.com/charmbracelet/bubbletea/tree/master/examples/views)
- Issue #239: Add structured logging with live viewer using charmbracelet/log

## Notes for Implementation

1. **Incremental Development**: Implement tasks in dependency order to enable early testing
2. **Testing Priority**: Focus on ring buffer and file rotation edge cases
3. **Documentation**: Include inline comments for complex buffer management logic
4. **Logging Philosophy**: Use structured logging throughout - avoid string concatenation
5. **Immediate Focus**: Prioritize planning tab and ACP client instrumentation for issue debugging
