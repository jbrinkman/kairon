# Design Specification: Fix Log Tab JSON Field Parsing

**Issue**: #257  
**Title**: Log tab not displaying messages due to incorrect JSON field parsing  
**Closes**: #257

## Problem Summary

The log tab viewport in the TUI remains empty even though log entries are successfully written to disk in `.kiro-krew/logs/`. This occurs because the `loggingMultiWriter.Write()` function parses incoming JSON log entries looking for a field named `"message"`, but `charmbracelet/log` with `JSONFormatter` actually uses the field name `"msg"` according to the library's convention.

### Root Cause Analysis

**Location**: `internal/tui/tui.go`, lines 1314-1355, specifically line 1337

**Current Implementation**:
```go
if m, ok := rawEntry["message"].(string); ok {
    message = m
}
```

**Issue**: The field name mismatch causes all log messages to be extracted as empty strings, resulting in empty `LogEntry.Message` values being added to the ring buffer.

**Evidence**: Testing confirms that `charmbracelet/log` version 0.4.0 with `JSONFormatter` produces JSON output with the field `"msg"`:
```json
{"key1":"value1","level":"info","msg":"test message"}
```

### Impact

- Users see an empty log tab viewport despite logs being written to disk
- Log metadata (timestamp, level, key-value pairs) are parsed correctly
- Only the message field is affected
- File-based logging works correctly (no impact on log files)

## Solution Approach

This is a single-line fix to correct the JSON field name from `"message"` to `"msg"` to match the `charmbracelet/log` library's JSON formatter output.

### Architectural Context

The logging flow in Kiro Krew:

1. **Logger Initialization**: `internal/logging/logger.go` creates a `charmbracelet/log.Logger` with `JSONFormatter`
2. **Multi-Writer Pattern**: `loggingMultiWriter` in `internal/tui/tui.go` implements `io.Writer` to write to both:
   - `FileHandler` (writes raw JSON to disk) ✅ Working
   - `RingBuffer` (parses JSON and stores structured entries) ❌ Broken due to field name mismatch
3. **Log Tab Display**: `LogTab` polls the ring buffer and formats entries for display

The fix addresses step 2, ensuring the ring buffer receives correct message content.

## Relevant Files

### Files to Modify

- **`internal/tui/tui.go`** (line ~1337): Change JSON field name from `"message"` to `"msg"`

### Related Files (No Changes Required)

- `internal/tui/log_tab.go`: Log tab display logic (works correctly once ring buffer has data)
- `internal/logging/ring_buffer.go`: Ring buffer implementation (works correctly)
- `internal/logging/logger.go`: Logger configuration using `JSONFormatter` (correct as-is)
- `internal/logging/file_handler.go`: File writing (works correctly)

## Implementation Details

### Task 1: Fix JSON Field Name in loggingMultiWriter

**File**: `internal/tui/tui.go`  
**Line**: ~1337

**Current Code**:
```go
// Extract level and message
var levelStr, message string
if l, ok := rawEntry["level"].(string); ok {
    levelStr = l
}
if m, ok := rawEntry["message"].(string); ok {
    message = m
}
```

**Fixed Code**:
```go
// Extract level and message
var levelStr, message string
if l, ok := rawEntry["level"].(string); ok {
    levelStr = l
}
if m, ok := rawEntry["msg"].(string); ok {
    message = m
}
```

**Change Summary**: Replace `"message"` with `"msg"` on line 1337.

**Additional Context**: Line 1346 also references `"message"` in the metadata exclusion logic:
```go
for k, v := range rawEntry {
    if k != "time" && k != "level" && k != "message" {
```

This should be updated to `"msg"` as well to maintain consistency, ensuring the message field is properly excluded from the metadata map.

**Complete Fix**:
- Line 1337: Change `rawEntry["message"]` to `rawEntry["msg"]`
- Line 1346: Change condition from `k != "message"` to `k != "msg"`

### Acceptance Criteria Verification

**Criterion 1**: Log tab displays all log messages that are written to log files  
**Verification**: Messages will be extracted correctly from JSON and added to ring buffer with non-empty `Message` field

**Criterion 2**: Messages appear with correct timestamp, level, and metadata  
**Status**: Already working correctly; timestamp and metadata parsing are unaffected

**Criterion 3**: Log levels (debug, info, warn, error) are properly color-coded  
**Status**: Already working correctly; level parsing and colorization are unaffected

**Criterion 4**: Auto-scroll to bottom works as logs stream in  
**Status**: Already working correctly; auto-scroll logic is independent of message content

**Criterion 5**: Existing log file writing functionality remains unchanged  
**Status**: File handler writes raw JSON unmodified; this change only affects ring buffer parsing

## Validation Commands

### Manual Testing

1. **Start the TUI with logging enabled**:
   ```bash
   ./kiro-krew
   ```

2. **Open the log viewer tab**:
   - In the REPL, trigger actions that generate log messages (e.g., `watch start`, `status`)
   - Observe that log messages now appear in the log tab viewport

3. **Verify log content**:
   - Check that messages are displayed with correct timestamps
   - Verify log level colors (DEBUG=gray, INFO=blue, WARN=yellow, ERROR=red)
   - Confirm metadata key-value pairs are shown in brackets

4. **Verify file logging unchanged**:
   ```bash
   cat .kiro-krew/logs/debug-*.log
   ```
   - Confirm JSON format remains unchanged
   - Verify `"msg"` field is present in JSON output

### Automated Testing

Since this is a JSON parsing fix in the TUI layer, automated testing options:

1. **Unit Test for loggingMultiWriter** (if test coverage is added):
   ```go
   func TestLoggingMultiWriterParsesMsg(t *testing.T) {
       rb := logging.NewRingBuffer(10)
       fh := &mockFileHandler{}
       
       lmw := &loggingMultiWriter{
           ringBuffer: rb,
           fileHandler: fh,
       }
       
       jsonLog := []byte(`{"level":"info","msg":"test message","key":"value"}`)
       lmw.Write(jsonLog)
       
       entries := rb.Get()
       if len(entries) != 1 {
           t.Fatal("expected 1 entry")
       }
       if entries[0].Message != "test message" {
           t.Errorf("expected 'test message', got '%s'", entries[0].Message)
       }
   }
   ```

2. **Integration Test**: Run the TUI in test mode and verify log tab content appears

### Regression Prevention

- **Before Fix**: Log tab viewport remains empty despite successful file writes
- **After Fix**: Log tab displays messages matching file content
- **No Side Effects**: File logging, metadata parsing, and level parsing remain unaffected

## Team Orchestration

This is a single-developer task with no dependencies:

- **Task 1**: Builder makes one-line change (two lines actually: line 1337 and 1346)
- **Validation**: Validator verifies log tab displays messages correctly

**Estimated Effort**: < 5 minutes  
**Risk Level**: Low (isolated change, no API modifications)  
**Testing Strategy**: Manual verification via TUI, visual confirmation of log tab content

## Implementation Notes

### Why Both Lines Need Changing

1. **Line 1337**: Extracts the message from JSON into the `message` variable
   - Without this change, messages remain empty (primary bug)

2. **Line 1346**: Excludes message field from metadata map
   - Should be updated to exclude `"msg"` instead of `"message"`
   - Without this change, `"msg"` would incorrectly appear in metadata
   - This is a consistency fix to prevent metadata pollution

### Alternative Approaches Considered

**Option A**: Modify `charmbracelet/log` to use `"message"` field  
**Rejected**: Would require forking the library or upstream change; unnecessary when we can easily adapt to the library's convention

**Option B**: Support both `"message"` and `"msg"` field names  
**Rejected**: Unnecessary complexity; `charmbracelet/log` consistently uses `"msg"`

**Option C**: Current fix - adapt to library's field name  
**Selected**: Minimal change, aligns with library conventions, no external dependencies

### Backward Compatibility

This fix has no backward compatibility concerns:
- No API changes
- No configuration changes
- No file format changes
- Only affects in-memory log parsing for TUI display

## Dependencies

- No external dependencies
- No new libraries required
- No configuration changes needed
- No database migrations

## Rollout Plan

1. Merge PR with fix
2. Users update to new version
3. Log tab immediately displays messages correctly
4. No configuration or data migration required

## Success Metrics

- Log tab viewport displays non-empty messages
- Message content matches log file entries
- All acceptance criteria verified
- No regression in file logging functionality
