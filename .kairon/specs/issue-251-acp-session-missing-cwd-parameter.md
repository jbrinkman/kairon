# Design Specification: Fix ACP Session Creation Missing Required `cwd` Parameter

**Issue**: #251  
**Title**: ACP session creation missing required cwd parameter causing "Improperly formed request" error  
**Repository**: jbrinkman/kiro-krew  
**Closes**: #251

## Problem Statement

The planning tab's ACP session creation fails with an "Improperly formed request" error because the `NewSessionRequest` is missing the required `cwd` (current working directory) parameter as specified in the ACP protocol specification.

### Error Manifestation

```
2026/07/15 07:21:24 ERRO streaming prompt failed session_id=ef5b2781-c9e0-41a8-936d-e87004e4ac33 error="{\"code\":-32603,\"message\":\"Internal error\",\"data\":\"Encountered an error in the response stream: Improperly formed request. (request_id: 332a05f2-73ca-46df-91e3-2d99069066c2)\"}"
```

### Root Cause

According to the [ACP protocol documentation](https://agentclientprotocol.com/protocol/v1/session-setup#creating-a-session), the `session/new` method requires:
- `cwd`: The working directory for the session (must be an absolute path)
- `mcpServers`: List of MCP servers (can be empty slice)

The current implementation at `internal/acp/client.go` lines 320 and 407 only provides `McpServers` and omits the required `Cwd` field.

## Solution Approach

Add the `cwd` parameter to the ACP connection configuration and ensure it's passed to all `NewSession` calls (both streaming and non-streaming). The solution follows the pattern established in the reference implementation at `/Users/jbrinkman/projects/kiro-acp-client/main.go`.

### Design Principles

1. **Protocol Compliance**: Strictly follow ACP protocol specification requirements
2. **Minimal Changes**: Add only what's required without refactoring unrelated code
3. **Consistency**: Apply the fix uniformly to both streaming and non-streaming paths
4. **Validation**: Ensure the `cwd` is an absolute path before use
5. **Backward Compatibility**: Default to current working directory if not explicitly set

## Relevant Files

### Files to Modify

1. **`internal/acp/types.go`** (line ~88)
   - Add `Cwd string` field to `ConnectionConfig` struct
   - Update `DefaultConnectionConfig()` to initialize `Cwd` with current working directory
   - Update `ValidateConnectionConfig()` to validate `Cwd` is absolute path

2. **`internal/acp/client.go`** (lines 320, 407)
   - Update `NewSession` call on line 320 (non-streaming) to include `Cwd: c.config.Cwd`
   - Update `NewSession` call on line 407 (streaming) to include `Cwd: c.config.Cwd`

3. **`internal/tui/planning_tab.go`** (line ~131)
   - When creating ACP client, set `config.Cwd` to project root (use `os.Getwd()`)
   - Add logging to record the working directory being used

### Files for Reference

- `/Users/jbrinkman/projects/kiro-acp-client/main.go` - Reference implementation showing correct usage

## Team Orchestration

This is a straightforward bug fix with minimal dependencies:

1. **Task 1**: Update `ConnectionConfig` type and validation (types.go)
   - **Dependencies**: None
   - Can proceed immediately

2. **Task 2**: Update both `NewSession` calls in ACP client (client.go)
   - **Dependencies**: Task 1 (needs updated `ConnectionConfig` structure)
   - Sequential after Task 1

3. **Task 3**: Initialize `Cwd` in planning tab (planning_tab.go)
   - **Dependencies**: Task 1 (needs updated `ConnectionConfig` structure)
   - Can run in parallel with Task 2

All tasks contribute to complete issue resolution in one PR. Tasks 2 and 3 can be parallelized after Task 1 completes.

## Step-by-Step Task Breakdown

### Task 1: Update ConnectionConfig Structure

**File**: `internal/acp/types.go`

**Changes Required**:

1. Add `Cwd` field to `ConnectionConfig` struct (around line 88):
```go
type ConnectionConfig struct {
    // KiroCLIPath is the path to the Kiro CLI executable
    KiroCLIPath string `json:"kiro_cli_path,omitempty"`

    // Agent is the name of the agent to connect to
    Agent string `json:"agent,omitempty"`

    // Cwd is the working directory for the ACP session (must be absolute path)
    Cwd string `json:"cwd,omitempty"`

    // MaxRetries is the maximum number of retry attempts
    MaxRetries int `json:"max_retries,omitempty"`
    
    // ... rest of fields
}
```

2. Update `DefaultConnectionConfig()` function to initialize `Cwd`:
```go
func DefaultConnectionConfig() *ConnectionConfig {
    cwd, err := os.Getwd()
    if err != nil {
        cwd = "." // Fallback to current directory
    }
    // Convert to absolute path
    if !filepath.IsAbs(cwd) {
        if absCwd, err := filepath.Abs(cwd); err == nil {
            cwd = absCwd
        }
    }
    
    return &ConnectionConfig{
        KiroCLIPath:       "kiro-cli",
        Agent:             "", // Must be set explicitly by caller
        Cwd:               cwd,
        MaxRetries:        3,
        RetryDelay:        1 * time.Second,
        ConnectionTimeout: 30 * time.Second,
        RequestTimeout:    60 * time.Second,
    }
}
```

3. Update `ValidateConnectionConfig()` to validate `Cwd`:
```go
func ValidateConnectionConfig(config *ConnectionConfig) error {
    if config == nil {
        return ErrInvalidConfig
    }

    if config.Agent == "" {
        return ErrMissingConfigAgent
    }

    if config.KiroCLIPath == "" {
        return errors.New("kiro-cli path is required")
    }

    if config.Cwd == "" {
        return errors.New("cwd (working directory) is required")
    }

    // Validate Cwd is absolute path
    if !filepath.IsAbs(config.Cwd) {
        return errors.New("cwd must be an absolute path")
    }

    if config.ConnectionTimeout <= 0 || config.RequestTimeout <= 0 {
        return errors.New("timeouts must be positive")
    }

    return nil
}
```

**Required Imports**: Add `path/filepath` and `os` if not already imported

**Acceptance Criteria**:
- `ConnectionConfig` struct has `Cwd string` field
- `DefaultConnectionConfig()` initializes `Cwd` with absolute path to current working directory
- `ValidateConnectionConfig()` validates `Cwd` is non-empty and absolute path
- All validations pass with appropriate error messages

### Task 2: Update NewSession Calls in ACP Client

**File**: `internal/acp/client.go`

**Changes Required**:

1. Update non-streaming `NewSession` call (line ~320):
```go
// Before:
sessionResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
    McpServers: []acp.McpServer{},
})

// After:
logging.Debug("creating ACP session", "cwd", c.config.Cwd)
sessionResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
    Cwd:        c.config.Cwd,
    McpServers: []acp.McpServer{},
})
```

2. Update streaming `NewSession` call (line ~407):
```go
// Before:
sessionResp, err := conn.NewSession(streamCtx, acp.NewSessionRequest{
    McpServers: []acp.McpServer{},
})

// After:
logging.Debug("creating streaming ACP session", "cwd", c.config.Cwd)
sessionResp, err := conn.NewSession(streamCtx, acp.NewSessionRequest{
    Cwd:        c.config.Cwd,
    McpServers: []acp.McpServer{},
})
```

**Acceptance Criteria**:
- Both `NewSession` calls (streaming and non-streaming) include `Cwd` field
- Debug logging includes the working directory being used
- No changes to surrounding error handling or session management logic
- All existing tests pass

### Task 3: Initialize Cwd in Planning Tab

**File**: `internal/tui/planning_tab.go`

**Changes Required**:

Update the ACP client creation section (around line 131 in `NewPlanningTabWithSession`):

```go
// Initialize ACP client with provided client or create default
if acpClient == nil {
    logging.Debug("creating default ACP client for planner agent", "tab_id", id)
    
    // Get current working directory for ACP session
    cwd, err := os.Getwd()
    if err != nil {
        logging.Warn("failed to get current working directory, using fallback", "tab_id", id, "error", err)
        cwd = "."
    }
    
    // Ensure absolute path
    if !filepath.IsAbs(cwd) {
        if absCwd, err := filepath.Abs(cwd); err == nil {
            cwd = absCwd
        }
    }
    
    logging.Info("initializing ACP client with working directory", "tab_id", id, "cwd", cwd)
    
    config := acp.DefaultConnectionConfig()
    config.Agent = "planner"
    config.Cwd = cwd
    acpClient = acp.NewClient(config)
} else {
    logging.Debug("using provided ACP client", "tab_id", id)
}
```

**Required Imports**: Add `os` and `path/filepath` if not already imported

**Acceptance Criteria**:
- Planning tab sets `Cwd` when creating ACP client
- Working directory is logged for debugging
- Absolute path is ensured before setting
- Error handling for `os.Getwd()` failure with appropriate fallback
- Existing ACP client functionality remains unchanged

## Validation Commands

### Unit Testing

```bash
# Run ACP-related tests
go test ./internal/acp/... -v

# Run planning tab tests
go test ./internal/tui/... -run TestPlanning -v
```

### Integration Testing

```bash
# Build the project
task build

# Run the TUI and test planning tab
./kiro-krew

# In the REPL:
# 1. Press Ctrl+Alt+P to switch to planning mode
# 2. Type a simple prompt like "hello" and press Enter
# 3. Verify no "Improperly formed request" error appears
# 4. Verify the planner responds successfully
```

### Manual Verification Checklist

1. **Start kiro-krew and open planning tab**
   - No errors in logs during ACP connection
   - `cwd` appears in debug logs during session creation

2. **Send a prompt in planning tab**
   - No "Improperly formed request" error
   - Session ID appears in logs
   - Streaming response received successfully
   - No JSON-RPC protocol errors

3. **Check logs for working directory**
   - Look for log entries containing `cwd` parameter
   - Verify path is absolute
   - Verify path matches current working directory

### Expected Log Output

```
INFO initializing ACP client with working directory tab_id=planning-1 cwd=/Users/username/projects/kiro-krew
DEBUG creating ACP session cwd=/Users/username/projects/kiro-krew
INFO streaming ACP session created session_id=<uuid>
DEBUG sending prompt for streaming session_id=<uuid>
INFO streaming completed session_id=<uuid>
```

### Regression Testing

Verify existing functionality still works:
- Non-streaming message requests work correctly
- Session reuse across multiple prompts works
- Error handling for connection failures remains functional
- Planning tab state management unaffected

## Constraints

1. **Protocol Compliance**: Must match ACP protocol specification exactly
2. **No Breaking Changes**: Existing ACP functionality must continue working
3. **Path Safety**: `cwd` must always be an absolute path
4. **Error Handling**: Gracefully handle failures to determine working directory
5. **Logging**: Add appropriate debug/info logging without cluttering logs

## Technical Notes

### Why This Fix Works

The ACP protocol requires `cwd` for session creation because:
1. Agents need context about where to resolve relative file paths
2. The working directory affects tool execution and file operations
3. The protocol enforces this as a mandatory field for session context

### Reference Implementation Analysis

The working example at `/Users/jbrinkman/projects/kiro-acp-client/main.go` shows:
```go
cwd, err := filepath.Abs(os.Args[1])
if err != nil {
    log.Fatal(err)
}

// ...

sess, err := conn.NewSession(ctx, acp.NewSessionRequest{
    Cwd:        cwd,  // ✅ Required field included
    McpServers: []acp.McpServer{},
})
```

Our implementation follows the same pattern but uses `os.Getwd()` instead of requiring it as a parameter.

### Potential Edge Cases

1. **Symbolic Links**: `filepath.Abs()` resolves symlinks, which is the correct behavior
2. **Permission Issues**: If `os.Getwd()` fails due to permissions, we fall back to "." and let `filepath.Abs()` resolve it
3. **Windows Paths**: `filepath.IsAbs()` and `filepath.Abs()` are cross-platform and handle Windows paths correctly

## Success Criteria

The fix is complete when:

1. ✅ All three files are updated with the required changes
2. ✅ Unit tests pass for modified code
3. ✅ Planning tab can send prompts without protocol errors
4. ✅ Debug logs show the `cwd` parameter being used
5. ✅ Both streaming and non-streaming requests work correctly
6. ✅ No regression in existing ACP functionality
7. ✅ Code follows existing patterns and style conventions

## Risk Assessment

**Risk Level**: Low

**Rationale**:
- Small, focused change to well-defined types and call sites
- Following established pattern from reference implementation
- No complex logic or state management changes
- Clear validation and error handling

**Mitigation**:
- Comprehensive testing of both streaming and non-streaming paths
- Debug logging to verify correct behavior
- Validation to catch configuration errors early
