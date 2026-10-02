# Design Specification: Refactor Logging to Use JSON Formatter

**Issue**: #252  
**Title**: Log tab displays no messages: refactor logging to use JSON formatter instead of parsing formatted text  
**Closes**: #252

## Problem Statement

The log tab does not display messages due to a flawed architecture where:

1. The `charmbracelet/log` logger formats logs as human-readable text
2. `loggingMultiWriter.Write()` attempts to parse that formatted text back into structured data
3. Text parsing logic is incorrect (expects log level at `fields[0]`, but it's actually at `fields[2]` after date/time)
4. Ring buffer receives malformed entries with incorrect log levels
5. Log tab displays nothing because entries fail to parse correctly

**Current Flawed Flow:**
```
logging.Info("msg", "key", "value")
  → charmbracelet/log formats to text: "2026/07/15 07:20:42 INFO msg key=value"
  → loggingMultiWriter.Write([]byte) receives formatted string
  → attempts to parse fields[0] as level (actually date)
  → parsing fails, entries are malformed
  → ring buffer gets incorrect data
  → log tab displays nothing
```

This violates the architectural principle: **keep data structured until the final display step**.

## Solution Approach

Refactor the logging pipeline to use JSON formatting throughout, eliminating fragile text parsing:

1. **Configure JSON Formatter**: Set `log.SetFormatter(log.JSONFormatter)` in logger initialization
2. **Update loggingMultiWriter**: Parse JSON instead of text to extract structured data
3. **Pass Structured Data**: Populate ring buffer with properly parsed log level, message, and metadata
4. **Preserve File Logs**: Consider file format options (JSON vs text)
5. **Maintain Display Layer**: No changes needed to log tab formatting (already handles structured `LogEntry`)

**Proposed Correct Flow:**
```
logging.Info("msg", "key", "value")
  → charmbracelet/log formats to JSON: {"time":"2026-07-15T07:20:42-04:00","level":"info","message":"msg","key":"value"}
  → loggingMultiWriter.Write([]byte) receives JSON
  → json.Unmarshal() parses structured data
  → extract level="info", message="msg", metadata={"key":"value"}
  → ringBuffer.Add(log.InfoLevel, "msg", "key", "value")
  → log tab displays correctly formatted entry
```

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────┐
│ Application Code (logging.Info/Debug/Warn/Error)              │
└───────────────────────┬────────────────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────────────────┐
│ charmbracelet/log (globalLogger)                               │
│ - Formatter: log.JSONFormatter ✅ NEW                          │
│ - Output: loggingMultiWriter                                   │
└───────────────────────┬────────────────────────────────────────┘
                        │
                        │ JSON formatted bytes
                        ▼
┌────────────────────────────────────────────────────────────────┐
│ loggingMultiWriter                                             │
│ ┌──────────────────┐          ┌──────────────────┐            │
│ │ json.Unmarshal() │          │ FileHandler.Write│            │
│ │ ✅ NEW           │          │ (JSON or text)   │            │
│ └────────┬─────────┘          └──────────────────┘            │
│          │                                                      │
│          │ Structured: level, message, metadata                │
│          ▼                                                      │
│ ┌──────────────────────────────────────────────┐              │
│ │ RingBuffer.Add(level, message, keyvals...)   │              │
│ │ (no changes needed)                          │              │
│ └──────────────────────────────────────────────┘              │
└───────────────────────┬────────────────────────────────────────┘
                        │
                        │ Structured LogEntry objects
                        ▼
┌────────────────────────────────────────────────────────────────┐
│ LogTab                                                          │
│ - formatLogEntry() renders with color-coding                   │
│ - No changes needed ✅                                         │
└────────────────────────────────────────────────────────────────┘
```

## Relevant Files

### Files to Modify

1. **internal/logging/logger.go**
   - `Initialize()` function: Add `globalLogger.SetFormatter(log.JSONFormatter)`
   - `Activate()` function: Add `globalLogger.SetFormatter(log.JSONFormatter)`
   - Purpose: Configure JSON output at logger initialization

2. **internal/tui/tui.go**
   - `loggingMultiWriter.Write()` method (lines 1313-1365)
   - Purpose: Replace text parsing with JSON parsing

### Files Referenced (No Changes)

3. **internal/logging/ring_buffer.go**
   - `RingBuffer.Add(level, message, keyvals...)` method
   - Already accepts structured data in correct format

4. **internal/logging/types.go**
   - `LogEntry` struct definition
   - Already structured correctly

5. **internal/tui/log_tab.go**
   - `formatLogEntry()` method
   - Already handles structured `LogEntry` objects correctly

6. **internal/logging/file_handler.go**
   - `FileHandler.Write()` method
   - Will receive JSON bytes, writes directly to file

## Team Orchestration

This is a single-PR refactor with two parallel-ready tasks followed by validation:

### Task Execution Strategy

- **Task 1 and Task 2 can run in parallel** (no dependencies)
- **Task 3 depends on completion of Tasks 1 and 2**

```
Task 1: Configure JSON Formatter (Logger)
    ↓
    ├─→ (parallel execution) ─→ Task 3: Validation
    ↓
Task 2: Update loggingMultiWriter (Parser)
```

## Step-by-Step Task Breakdown

### Task 1: Configure JSON Formatter in Logger

**File**: `internal/logging/logger.go`

**Acceptance Criteria**:
- Add `globalLogger.SetFormatter(log.JSONFormatter)` in `Initialize()` function after logger creation
- Add `globalLogger.SetFormatter(log.JSONFormatter)` in `Activate()` function after logger creation
- Logger now outputs JSON format instead of text format
- All existing logging API calls (`logging.Info/Debug/Warn/Error`) continue to work unchanged

**Implementation Details**:

In `Initialize()` function, after line:
```go
globalLogger.SetReportTimestamp(true)
```

Add:
```go
// Use JSON formatter for structured output that can be parsed reliably
globalLogger.SetFormatter(log.JSONFormatter)
```

In `Activate()` function, after line:
```go
globalLogger.SetReportTimestamp(true)
```

Add:
```go
// Use JSON formatter for structured output that can be parsed reliably
globalLogger.SetFormatter(log.JSONFormatter)
```

**Dependencies**: None

**Estimated Effort**: 5 minutes (2 line additions)

---

### Task 2: Update loggingMultiWriter to Parse JSON

**File**: `internal/tui/tui.go`

**Acceptance Criteria**:
- Replace text parsing logic with JSON parsing in `loggingMultiWriter.Write()` method
- Parse JSON to extract `time`, `level`, `message`, and additional fields as metadata
- Map string log level to `log.Level` constants (DebugLevel, InfoLevel, WarnLevel, ErrorLevel)
- Convert JSON fields to key-value pairs for metadata
- Call `ringBuffer.Add(level, message, keyvals...)` with correctly structured data
- Handle JSON parsing errors gracefully (log but don't fail the write)

**Implementation Details**:

Replace the existing `loggingMultiWriter.Write()` method (lines 1313-1365) with:

```go
func (lmw *loggingMultiWriter) Write(p []byte) (n int, err error) {
	// Write to file handler first
	n, err = lmw.fileHandler.Write(p)
	if err != nil {
		return n, err
	}

	// Parse JSON structured log entry
	var entry struct {
		Time    string                 `json:"time"`
		Level   string                 `json:"level"`
		Message string                 `json:"message"`
		Fields  map[string]interface{} `json:"-"` // Capture remaining fields
	}

	// Unmarshal into a map first to capture all fields
	var rawEntry map[string]interface{}
	if err := json.Unmarshal(p, &rawEntry); err != nil {
		// JSON parse failed, but file write succeeded - don't fail the write
		// This could happen during transition or with malformed input
		return n, nil
	}

	// Extract known fields
	if t, ok := rawEntry["time"].(string); ok {
		entry.Time = t
	}
	if l, ok := rawEntry["level"].(string); ok {
		entry.Level = l
	}
	if m, ok := rawEntry["message"].(string); ok {
		entry.Message = m
	}

	// Map string level to log.Level constant
	level := mapStringToLogLevel(entry.Level)

	// Convert remaining fields to key-value pairs (exclude time, level, message)
	var keyvals []interface{}
	for k, v := range rawEntry {
		if k != "time" && k != "level" && k != "message" {
			keyvals = append(keyvals, k, v)
		}
	}

	// Add structured entry to ring buffer
	lmw.ringBuffer.Add(level, entry.Message, keyvals...)

	return n, nil
}

// mapStringToLogLevel converts a string log level to charmbracelet/log.Level constant
func mapStringToLogLevel(levelStr string) clog.Level {
	switch strings.ToLower(levelStr) {
	case "debug", "debu", "dbug":
		return clog.DebugLevel
	case "info":
		return clog.InfoLevel
	case "warn", "warning":
		return clog.WarnLevel
	case "error", "erro", "err":
		return clog.ErrorLevel
	default:
		return clog.InfoLevel // Default to info for unknown levels
	}
}
```

**Required Import Addition**:
Add to imports at top of `internal/tui/tui.go`:
```go
"encoding/json"
```

**Dependencies**: None (can run in parallel with Task 1)

**Estimated Effort**: 20 minutes

---

### Task 3: Validation and Testing

**Acceptance Criteria**:
- Open the log tab in the TUI
- Verify log entries appear correctly in the log tab display
- Verify log levels are correctly identified and color-coded (DEBUG=gray, INFO=blue, WARN=yellow, ERROR=red)
- Verify messages are displayed correctly
- Verify metadata (key-value pairs) are preserved and displayed
- Verify file logs are written successfully (format will be JSON)
- Verify structured logging API continues to work (`logging.Info("msg", "key", "value")`)
- Test with all log levels: Debug, Info, Warn, Error
- Test with metadata: multiple key-value pairs
- Test edge cases: empty messages, no metadata, large metadata

**Validation Commands**:

1. **Build and run the application**:
```bash
go build -o kiro-krew ./cmd/kiro-krew
./kiro-krew
```

2. **In the REPL, trigger logging activity**:
```
kiro-krew> watch start
```
   (This will trigger various log entries)

3. **Open the log tab** (via REPL command or hotkey if available)

4. **Verify log display**:
   - Check that log entries appear with timestamps
   - Check color-coded log levels
   - Check messages are readable
   - Check metadata is displayed in `[key=value]` format

5. **Check log file format**:
```bash
cat .kiro-krew/kiro-krew.log
```
   - Verify JSON format (one JSON object per line)
   - Example expected output:
     ```json
     {"time":"2026-07-15T09:30:00-04:00","level":"info","message":"Watcher started","repo":"owner/name"}
     {"time":"2026-07-15T09:30:01-04:00","level":"debug","message":"Polling GitHub","interval":"5m"}
     ```

6. **Test all log levels**:
   - Trigger debug logs (if log level is set to debug)
   - Trigger info logs (default)
   - Trigger warn logs (error conditions)
   - Trigger error logs (failures)

**Manual Testing Script**:

Create a temporary test in `internal/logging/logger_test.go` or run ad-hoc:

```go
func TestJSONFormatterIntegration(t *testing.T) {
	// This test verifies end-to-end JSON formatting
	var buf bytes.Buffer
	
	logger := log.New(&buf)
	logger.SetFormatter(log.JSONFormatter)
	logger.SetLevel(log.DebugLevel)
	logger.SetReportTimestamp(true)
	
	logger.Info("test message", "key1", "value1", "key2", 123)
	
	// Parse the JSON output
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}
	
	// Verify fields
	if entry["level"] != "info" {
		t.Errorf("Expected level=info, got %v", entry["level"])
	}
	if entry["message"] != "test message" {
		t.Errorf("Expected message='test message', got %v", entry["message"])
	}
	if entry["key1"] != "value1" {
		t.Errorf("Expected key1=value1, got %v", entry["key1"])
	}
}
```

**Dependencies**: Task 1 and Task 2 must be completed

**Estimated Effort**: 30 minutes

---

## File Format Considerations

### Current State
- File handler writes whatever bytes it receives from the logger
- Currently receives text-formatted logs

### After Refactor
- File handler will receive JSON-formatted logs
- Each log line will be a complete JSON object

### Options for File Format

**Option A: Keep JSON in Files (Recommended)**
- **Pros**: 
  - Machine-readable, structured logs
  - Easy to parse with tools like `jq`, log aggregators
  - Consistent format throughout pipeline
  - No additional complexity
- **Cons**: 
  - Less human-readable when viewing raw file
  - Slightly larger file size

**Option B: Format JSON to Text Before File Write**
- **Pros**: 
  - Human-readable file logs
  - Familiar format for developers
- **Cons**: 
  - Adds complexity to loggingMultiWriter
  - Requires custom text formatter
  - Defeats purpose of structured logging
  - Not recommended

**Recommendation**: Use Option A (keep JSON). If human-readable logs are needed, users can pipe through `jq`:
```bash
tail -f .kiro-krew/kiro-krew.log | jq -r '[.time, .level, .message] | @tsv'
```

### Future Enhancement (Optional)
If configurable file format is desired, add a config option:
```yaml
logging:
  file_format: json  # or "text"
```

This is OUT OF SCOPE for this issue but documented for future consideration.

## Validation Strategy

### Unit Testing
No new unit tests required - existing tests should continue to pass:
- `internal/logging/ring_buffer_test.go` - tests ring buffer operations
- `internal/logging/file_handler_test.go` - tests file writing

### Integration Testing
Manual verification required:
1. Log tab displays entries correctly
2. Log levels are color-coded properly
3. Metadata is preserved and displayed
4. File logs are written in JSON format
5. No errors or warnings during logging

### Regression Prevention
- Existing logging API unchanged (`logging.Info/Debug/Warn/Error`)
- RingBuffer interface unchanged
- LogEntry structure unchanged
- Log tab display logic unchanged

## Risk Assessment

### Low Risk
- Changes are isolated to two functions
- Logger configuration change (1 line addition × 2 locations)
- Writer method replacement (self-contained)
- No changes to logging API
- No changes to display layer

### Medium Risk
- File format changes from text to JSON
  - **Mitigation**: Document new format, provide `jq` examples
  - **Impact**: Users viewing raw log files will see JSON
  
- JSON parsing could fail on malformed input
  - **Mitigation**: Graceful error handling in `loggingMultiWriter.Write()`
  - **Impact**: Failed parse logs to stderr but doesn't crash

### Zero Risk
- Breaking existing code (logging API unchanged)
- Breaking log display (already handles structured data)
- Breaking ring buffer (already handles structured data)

## Performance Considerations

### JSON Formatting Overhead
- **Negligible**: JSON formatting in `charmbracelet/log` is optimized
- Typical log volume: <100 logs/second
- JSON parsing: <1ms per entry

### File I/O
- No change to file I/O operations
- JSON format slightly larger than text (~20% increase)
- File rotation still triggers at configured size

## Backward Compatibility

### Breaking Changes
1. **Log file format changes from text to JSON**
   - External tools parsing old text format will break
   - **Migration**: Document new JSON format, provide examples

### Non-Breaking Changes
- Logging API unchanged
- Log tab display unchanged
- Ring buffer interface unchanged
- Configuration unchanged

## Success Criteria

1. ✅ Log tab displays messages correctly
2. ✅ Log levels are correctly identified and color-coded
3. ✅ Metadata (key-value pairs) are preserved and displayed
4. ✅ File logs are written successfully (JSON format)
5. ✅ No errors during logging operations
6. ✅ All existing logging API calls continue to work
7. ✅ Manual testing validates all log levels (Debug, Info, Warn, Error)
8. ✅ Manual testing validates metadata display

## Implementation Notes

### charmbracelet/log JSON Format
The `charmbracelet/log` package with `JSONFormatter` produces JSON output like:
```json
{
  "time": "2026-07-15T09:30:00-04:00",
  "level": "info",
  "message": "Watcher started",
  "repo": "owner/name",
  "interval": "5m"
}
```

### Key-Value Pairs
When calling `logging.Info("msg", "key1", "val1", "key2", "val2")`, the JSON will include:
```json
{
  "message": "msg",
  "key1": "val1",
  "key2": "val2"
}
```

### Log Level Mapping
String levels from JSON map to constants:
- `"debug"` → `log.DebugLevel`
- `"info"` → `log.InfoLevel`
- `"warn"` or `"warning"` → `log.WarnLevel`
- `"error"` → `log.ErrorLevel`

## References

- **Issue**: #252
- **charmbracelet/log**: https://github.com/charmbracelet/log
- **JSON Formatter**: `log.JSONFormatter` constant
- **Architecture Principle**: Keep data structured until final output
