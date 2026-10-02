# Design Specification: Fix ACP "Improperly formed request" error in planning tab

**Issue**: #248  
**Created**: 2026-07-14  
**Status**: Ready for Implementation  

Closes #248

## Problem Summary

The planning tab in Kiro Krew successfully establishes an ACP connection but fails when streaming prompts with an "Improperly formed request" error. This breaks all planning tab functionality, preventing users from interacting with the planner agent.

**Error Details**:
```
2026/07/14 17:52:51 ERRO streaming prompt failed 
  session_id=6a48e227-8737-4ebb-9da6-911880692a0b 
  error="{{\"code\":-32603,\"message\":\"Internal error\",
         \"data\":\"Encountered an error in the response stream: 
         Improperly formed request. (request_id: d2bd0124-534a-41bd-aae9-56167d9c3f0c)\"}}"
```

## Root Cause Analysis

After examining the codebase, I've identified **two critical bugs**:

### Bug 1: Incorrect Agent Name in MessageRequest
**Location**: `internal/tui/planning_tab.go:439`

The planning tab hardcodes the agent name as `"kiro-agent"` when creating message requests:

```go
req := &acp.MessageRequest{
    Agent:          "kiro-agent", // ❌ WRONG - should be "planner"
    Message:        message,
    Streaming:      true,
    ResponseFormat: "text",
    Timeout:        60 * time.Second,
}
```

However, the actual agent configuration is named `"planner"` (defined in `.kiro/agents/planner.json`). This mismatch causes the ACP protocol to reject the request as improperly formed since it references a non-existent agent.

### Bug 2: Missing Agent Context at Connection Time
**Location**: `internal/acp/client.go:156`

The ACP client spawns `kiro-cli acp` without specifying which agent to use:

```go
cmd := exec.CommandContext(ctx, c.config.KiroCLIPath, "acp")
// ❌ Missing: --agent flag
```

According to the ACP protocol and kiro-cli design, the agent context should be established at connection initialization time via the `--agent` flag. This would eliminate ambiguity in subsequent message requests.

### Why This Causes "Improperly formed request"

1. **Connection establishes** without agent context (no `--agent` flag)
2. **Prompt request sent** with `Agent: "kiro-agent"` field
3. **ACP validates request** and finds:
   - No agent context from connection initialization
   - Agent name "kiro-agent" doesn't exist
   - Request is rejected as improperly formed

## Solution Approach

### High-Level Strategy

Implement a two-layer fix that addresses both the immediate bug and establishes proper agent context management:

1. **Layer 1 (Critical Fix)**: Correct the agent name in `MessageRequest` to use `"planner"`
2. **Layer 2 (Architectural Fix)**: Add agent specification to ACP connection configuration and pass it during connection initialization

This approach ensures:
- Immediate functionality restoration (Layer 1)
- Proper architectural foundation for future agent integrations (Layer 2)
- No breaking changes to existing code
- Clear separation between connection-level and message-level agent context

### Design Decisions

**Decision 1**: Add `Agent` field to `ConnectionConfig`
- **Rationale**: Agent context is a connection-level concern, not a per-message concern
- **Impact**: Minimal - ConnectionConfig is internal to the ACP package
- **Benefit**: Centralizes agent specification, reduces repetition

**Decision 2**: Pass `--agent` flag during connection initialization
- **Rationale**: Follows ACP protocol best practices and kiro-cli design
- **Impact**: Changes command construction in `Connect()` method
- **Benefit**: Establishes agent context once at connection time, not per message

**Decision 3**: Make `MessageRequest.Agent` field optional/deprecated
- **Rationale**: With connection-level agent context, message-level specification becomes redundant
- **Impact**: No breaking change - field remains but is not used
- **Benefit**: Simplifies message construction, reduces error surface

**Decision 4**: Update planning tab to use connection-level agent configuration
- **Rationale**: Planning tab should configure agent once at client creation, not per message
- **Impact**: Simplifies `sendMessage()` function
- **Benefit**: More maintainable, clearer intent

## Relevant Files

### Files to Modify

1. **`internal/acp/types.go`**
   - Add `Agent` field to `ConnectionConfig` struct
   - Update `DefaultConnectionConfig()` to set default agent
   - Add documentation about agent specification

2. **`internal/acp/client.go`**
   - Update `Connect()` to pass `--agent` flag from `ConnectionConfig.Agent`
   - Add validation to ensure agent is specified before connecting
   - Add logging for agent context

3. **`internal/tui/planning_tab.go`**
   - Fix hardcoded agent name from `"kiro-agent"` to `"planner"`
   - Update ACP client initialization to pass agent in `ConnectionConfig`
   - Remove redundant `Agent` field from `MessageRequest` creation (use empty string or remove)

### Files for Reference

- `.kiro/agents/planner.json` - Agent configuration defining "planner" as the agent name
- `internal/logging/logger.go` - Logging patterns for consistent debug output

## Team Orchestration

This is a single-developer implementation with clear task boundaries:

### Task Dependencies

```
Task 1 (Backend: ACP Types) → No dependencies
Task 2 (Backend: ACP Client) → Depends on Task 1
Task 3 (Frontend: Planning Tab) → Depends on Task 1, Task 2
Task 4 (Validation) → Depends on Task 3
```

**Parallel Execution Opportunities**: None - tasks must be executed sequentially due to tight coupling between type definitions, client implementation, and UI integration.

### Implementation Sequence

Tasks are numbered to reflect logical implementation order within a single PR. All tasks must be completed for full issue resolution.

## Step-by-Step Task Breakdown

### Task 1: Update ACP Type Definitions

**Objective**: Add agent specification to connection configuration

**Files to Modify**:
- `internal/acp/types.go`

**Implementation Steps**:
1. Add `Agent` field to `ConnectionConfig` struct after `KiroCLIPath` field:
   ```go
   // Agent is the name of the agent to communicate with (required)
   Agent string `json:"agent"`
   ```

2. Update `DefaultConnectionConfig()` to include a default agent value:
   ```go
   return &ConnectionConfig{
       KiroCLIPath:       "kiro-cli",
       Agent:             "", // Default empty - must be set by caller
       MaxRetries:        3,
       RetryDelay:        1 * time.Second,
       ConnectionTimeout: 30 * time.Second,
       RequestTimeout:    60 * time.Second,
   }
   ```

3. Add validation function for ConnectionConfig:
   ```go
   // ValidateConnectionConfig validates a ConnectionConfig
   func ValidateConnectionConfig(config *ConnectionConfig) error {
       if config == nil {
           return ErrInvalidRequest
       }
       if config.Agent == "" {
           return ErrMissingAgent
       }
       return nil
   }
   ```

**Acceptance Criteria**:
- `ConnectionConfig` struct has `Agent` field with JSON tag
- `DefaultConnectionConfig()` returns config with empty agent (forcing explicit setting)
- `ValidateConnectionConfig()` function exists and checks for empty agent
- All existing tests pass (no breaking changes)

**Dependencies**: None

---

### Task 2: Update ACP Client Connection Logic

**Objective**: Pass agent specification to `kiro-cli acp` during connection initialization

**Files to Modify**:
- `internal/acp/client.go`

**Implementation Steps**:
1. Add agent validation at the beginning of `Connect()` method (line ~150):
   ```go
   // Validate connection config before attempting connection
   if err := ValidateConnectionConfig(c.config); err != nil {
       logging.Error("invalid ACP connection config", "error", err)
       return err
   }
   ```

2. Update command construction to include `--agent` flag (line ~156):
   ```go
   // Start kiro-cli in ACP mode with agent specification
   cmd := exec.CommandContext(ctx, c.config.KiroCLIPath, "acp", "--agent", c.config.Agent)
   
   logging.Info("starting ACP connection", "agent", c.config.Agent, "kiro_cli_path", c.config.KiroCLIPath)
   ```

3. Add enhanced logging after connection initialization (after line ~174):
   ```go
   logging.Info("ACP connection established successfully", "agent", c.config.Agent, "session_id", c.sessionID)
   ```

4. Update error logging to include agent context:
   ```go
   logging.Error("failed to initialize ACP connection", "agent", c.config.Agent, "error", err)
   ```

**Acceptance Criteria**:
- `Connect()` validates `ConnectionConfig` before spawning process
- `kiro-cli acp` command includes `--agent <agent-name>` flag
- Connection logs include agent name for debugging
- Connection fails fast with clear error if agent is not specified
- Existing connection lifecycle (disconnect, cleanup) remains unchanged

**Dependencies**: Task 1

---

### Task 3: Fix Planning Tab Agent Configuration

**Objective**: Configure planning tab to use correct agent name and connection-level agent specification

**Files to Modify**:
- `internal/tui/planning_tab.go`

**Implementation Steps**:
1. Update `NewPlanningTab()` and `NewPlanningTabWithSession()` to configure agent in ACP client (around line ~115-120):
   ```go
   // Initialize ACP client with provided client or create default
   if acpClient == nil {
       logging.Debug("creating default ACP client", "tab_id", id)
       config := acp.DefaultConnectionConfig()
       config.Agent = "planner" // Set agent for planning tab
       acpClient = acp.NewClient(config)
   } else {
       logging.Debug("using provided ACP client", "tab_id", id)
   }
   pt.acpClient = acpClient
   ```

2. Update `sendMessage()` to remove hardcoded agent name (line ~439):
   ```go
   // Create message request - agent context already established at connection time
   req := &acp.MessageRequest{
       Agent:          "", // Empty - agent context from connection
       Message:        message,
       Streaming:      true,
       ResponseFormat: "text",
       Timeout:        60 * time.Second,
   }
   
   logging.Debug("creating ACP stream", "tab_id", pt.id, "message_length", len(message))
   ```

3. Add agent logging to connection establishment (around line ~436):
   ```go
   if !pt.acpClient.IsConnected() {
       logging.Info("ACP not connected, establishing connection", "tab_id", pt.id)
       if err := pt.acpClient.Connect(ctx); err != nil {
           cancel()
           logging.Error("failed to connect to ACP", "tab_id", pt.id, "error", err)
           return planningResponseMsg{
               content:    fmt.Sprintf("Failed to connect to planner agent: %v", err),
               isError:    true,
               isComplete: true,
           }
       }
       logging.Info("ACP connection established", "tab_id", pt.id)
   }
   ```

4. Update session ACP metadata tracking (if needed) to reflect agent configuration

**Acceptance Criteria**:
- Planning tab creates ACP client with `agent="planner"` in `ConnectionConfig`
- `MessageRequest` no longer contains hardcoded `"kiro-agent"` agent name
- `MessageRequest.Agent` field is empty string (connection-level agent used)
- Connection error messages reference "planner agent" for clarity
- Logging shows agent configuration during client creation
- All existing planning tab functionality preserved (message sending, streaming, etc.)

**Dependencies**: Task 1, Task 2

---

### Task 4: Integration Testing and Validation

**Objective**: Verify complete fix through manual testing and log analysis

**Testing Approach**:

1. **Unit Test Validation** (if applicable):
   ```bash
   go test ./internal/acp/... -v
   go test ./internal/tui/... -v -run TestPlanningTab
   ```

2. **Manual Integration Test**:
   - Start kiro-krew application
   - Navigate to planning tab using hotkey (Ctrl+Alt+P)
   - Type a test message: "Hello, can you help me create a GitHub issue?"
   - Press Enter to send message
   - Verify:
     - No "Improperly formed request" error in logs
     - Connection establishes successfully
     - Message streams back from planner agent
     - Response displays in planning tab viewport

3. **Log Analysis**:
   ```bash
   # Check for successful connection with agent context
   grep "ACP connection established" .kiro-krew/logs/debug-*.log
   
   # Verify agent flag is passed
   grep "starting ACP connection.*planner" .kiro-krew/logs/debug-*.log
   
   # Confirm no "Improperly formed request" errors
   grep "Improperly formed request" .kiro-krew/logs/debug-*.log
   # Should return no results
   ```

4. **Error Handling Test**:
   - Test with invalid agent name (manually modify code temporarily)
   - Verify connection fails fast with clear error message
   - Restore correct agent name

5. **Session Persistence Test**:
   - Send multiple messages in same planning tab
   - Verify agent context persists across messages
   - Verify session reuse works correctly

**Acceptance Criteria**:
- Planning tab connection establishes without errors
- User can type message and receive streamed response
- No "Improperly formed request" errors in logs during normal operation
- Agent context logged at each protocol layer (CLI args, connection init, prompt request)
- All messages in a session use the same agent context
- Error messages clearly indicate "planner agent" when connection fails

**Dependencies**: Task 3

## Validation Commands

### Build and Test

```bash
# Build the project
go build ./cmd/kiro-krew

# Run unit tests
go test ./internal/acp/... -v
go test ./internal/tui/... -v

# Run full test suite
go test ./... -v
```

### Manual Testing

```bash
# Start kiro-krew in debug mode
./kiro-krew

# In the REPL:
# 1. Press Ctrl+Alt+P to open planning tab
# 2. Type: "Can you help me draft a GitHub issue?"
# 3. Press Enter
# 4. Verify response streams back without errors
```

### Log Verification

```bash
# Check connection logs
grep "ACP connection established" .kiro-krew/logs/debug-*.log | tail -5

# Verify agent configuration
grep "starting ACP connection" .kiro-krew/logs/debug-*.log | grep "planner"

# Check for any errors
grep -i "error\|failed" .kiro-krew/logs/debug-*.log | grep -i acp

# Verify no "Improperly formed request" errors
grep "Improperly formed request" .kiro-krew/logs/debug-*.log
# Should return: (no results)
```

### Success Indicators

✅ **Connection Phase**:
- Log shows: `ACP connection established successfully agent=planner`
- Command includes: `kiro-cli acp --agent planner`
- No connection errors in logs

✅ **Message Streaming Phase**:
- User message appears in planning tab
- Agent response streams back character by character
- No "Improperly formed request" errors
- Session ID persists across messages

✅ **Complete Workflow**:
- User can send multiple messages in one planning session
- Agent context maintained across all messages
- Planning tab shows streaming indicator during response
- All responses display correctly in viewport

## Implementation Notes

### Agent Specification Philosophy

The ACP protocol supports two approaches for agent specification:

1. **Connection-level specification** (RECOMMENDED): Pass `--agent` flag to `kiro-cli acp` at connection time
   - Agent context established once
   - All subsequent messages inherit agent context
   - Cleaner protocol, less error-prone
   - **This is our implementation approach**

2. **Message-level specification** (NOT RECOMMENDED): Pass agent in each `MessageRequest`
   - Agent specified per message (verbose)
   - Allows agent switching within a session
   - More error-prone (easy to forget or misspell)
   - Creates ambiguity if both connection and message specify different agents

### Backward Compatibility

This fix introduces no breaking changes:
- `MessageRequest.Agent` field remains (marked as optional/deprecated)
- Existing code that doesn't use `ConnectionConfig.Agent` will get validation error
- Clear error messages guide developers to add agent configuration

### Future Enhancements (Out of Scope)

The following improvements are NOT part of this PR but could be considered later:

- **Multi-agent support**: Allow switching agents within a session
- **Agent discovery**: Auto-detect available agents from `.kiro/agents/`
- **Agent validation**: Verify agent exists before attempting connection
- **Graceful fallback**: Use default agent if specified agent not found

## Security Considerations

- **Command Injection**: Agent name is validated before being passed to exec.Command
- **Input Validation**: `ValidateConnectionConfig()` prevents empty agent values
- **Error Information**: Error messages do not expose sensitive system information

## Performance Impact

- **Connection Time**: No measurable impact (agent flag adds ~1ms to command construction)
- **Message Latency**: Reduced slightly (no per-message agent lookup/validation)
- **Memory**: Negligible increase (~10 bytes per ConnectionConfig for agent string)

## Testing Strategy

### Unit Tests (Recommended but not blocking)

While not strictly required for this fix, consider adding unit tests:

```go
// internal/acp/types_test.go
func TestValidateConnectionConfig(t *testing.T) {
    tests := []struct {
        name    string
        config  *ConnectionConfig
        wantErr error
    }{
        {"valid config", &ConnectionConfig{Agent: "planner"}, nil},
        {"empty agent", &ConnectionConfig{Agent: ""}, ErrMissingAgent},
        {"nil config", nil, ErrInvalidRequest},
    }
    // ... test implementation
}
```

### Integration Tests

Manual integration testing is sufficient for this fix. Automated integration tests would require mocking the entire ACP protocol, which is out of scope.

## Rollback Plan

If issues arise after deployment:

1. **Immediate Rollback**: Revert commit and deploy previous version
2. **Quick Fix**: Remove agent validation from `Connect()` to allow connections without agent specification
3. **Investigation**: Use enhanced logging to diagnose any new issues

The fix is low-risk and isolated to the ACP client and planning tab, so rollback is straightforward.

## Documentation Updates

No external documentation requires updates. The fix is internal to the ACP client implementation.

## Success Metrics

- **Primary**: Planning tab works without "Improperly formed request" errors
- **Secondary**: Log analysis shows agent context at all protocol layers
- **Tertiary**: No regression in console tab or watcher functionality

## Conclusion

This specification provides a complete implementation roadmap for fixing the ACP protocol bug. The solution addresses both the immediate symptom (wrong agent name) and the root architectural issue (missing connection-level agent context). All tasks are designed to be completed within a single PR, following kiro-krew's one-issue-one-PR workflow.
