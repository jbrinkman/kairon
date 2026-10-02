# Hotkey Toggle Between Planning Mode and Console - Design Specification

**Closes #47**

## Solution Approach

Implement a hotkey (`ctrl+option+p`) toggle system that allows seamless switching between planning mode (kiro-cli chat with planner agent) and console mode (main kiro-krew TUI) while preserving session state for both modes.

### Architecture Strategy

1. **Session State Management**: Create a session manager to persist and restore conversation history and context for both planning and console modes.

2. **Hotkey Integration**: Extend the existing Bubble Tea TUI with hotkey detection and mode switching logic.

3. **Process Management**: Implement background process handling to suspend/resume planning sessions while maintaining console responsiveness.

4. **State Persistence**: Store session data in `.kiro-krew/sessions/` directory with JSON serialization for conversation history and mode state.

## Relevant Files

### Files to Create
- `internal/session/manager.go` - Core session management and state persistence
- `internal/session/types.go` - Session data structures and constants  
- `internal/session/planner.go` - Planning session management (suspend/resume)
- `internal/hotkey/detector.go` - Hotkey detection and validation logic

### Files to Modify
- `internal/tui/tui.go` - Add hotkey handling and session integration
- `internal/tui/commands.go` - Update plan command to use session manager
- `internal/config/config.go` - Add session configuration options
- `.kiro-krew/config.yaml` - Session settings (history limits, auto-save)

### Files Referenced/Relevant
- `cmd/kiro-krew/cmd/root.go` - Main TUI entry point
- `.kiro/agents/planner.json` - Planner agent configuration
- `internal/agent/manager.go` - For process management patterns

## Team Orchestration

### Session Management Component
- Handles state persistence across mode toggles
- Manages conversation history serialization/deserialization
- Provides session lifecycle management (create, suspend, resume, cleanup)

### Hotkey Detection Component  
- Intercepts `ctrl+option+p` key combinations in TUI
- Validates hotkey context (only in kiro-krew sessions)
- Triggers mode switching operations

### TUI Integration Component
- Extends existing Bubble Tea model with session awareness
- Coordinates between console and planning mode transitions
- Maintains user interface consistency during mode switches

### Process Management Component
- Handles planning session process suspension/resumption
- Manages background kiro-cli process lifecycle
- Ensures clean process termination and resource cleanup

## Step-by-Step Task Breakdown

### Phase 1: Core Session Infrastructure
1. **Create Session Data Structures** [2 hours]
   - Define SessionType enum (Console, Planning)
   - Create SessionState struct with conversation history
   - Implement JSON marshaling/unmarshaling
   - **Acceptance:** Session data can be serialized and restored

2. **Implement Session Manager** [3 hours]
   - Create SessionManager with CRUD operations
   - Add session persistence to `.kiro-krew/sessions/`
   - Implement session lifecycle management
   - **Acceptance:** Sessions can be created, saved, loaded, and cleaned up

### Phase 2: Hotkey Detection System
3. **Create Hotkey Detection** [2 hours]
   - Implement hotkey pattern matching for `ctrl+option+p`
   - Add validation to ensure hotkey only works in kiro-krew terminals
   - Create hotkey event types for Bubble Tea
   - **Acceptance:** Hotkey detection triggers events only in kiro-krew context

4. **Integrate Hotkey with TUI** [2 hours]
   - Extend tui.model to handle hotkey events
   - Add hotkey processing to Update() method
   - Implement mode toggle logic
   - **Acceptance:** Pressing `ctrl+option+p` triggers mode switch attempt

### Phase 3: Planning Session Management
5. **Implement Planning Process Control** [4 hours]
   - Create planning session subprocess management
   - Add session suspend/resume with process control
   - Implement conversation history capture from kiro-cli
   - **Acceptance:** Planning sessions can be suspended and resumed with history intact

6. **Update Plan Command Integration** [2 hours]
   - Modify handlePlan() to use session manager
   - Add session restoration for existing planning sessions
   - Integrate with existing kiro-cli execution
   - **Acceptance:** Plan command creates or resumes sessions properly

### Phase 4: Mode Switching Logic
7. **Implement Console to Planning Toggle** [2 hours]
   - Add logic to switch from console to planning mode
   - Handle case where no planning session exists
   - Preserve console state during switch
   - **Acceptance:** Console→Planning switch works with proper state management

8. **Implement Planning to Console Toggle** [2 hours]
   - Add logic to switch from planning to console mode
   - Suspend planning session while preserving state
   - Restore console interface and activity history
   - **Acceptance:** Planning→Console switch preserves both session states

### Phase 5: Configuration and Polish
9. **Add Configuration Support** [1 hour]
   - Add session configuration to config.yaml
   - Implement session history limits and cleanup policies
   - Add configuration validation
   - **Acceptance:** Session behavior is configurable

10. **Error Handling and Edge Cases** [2 hours]
    - Handle session corruption and recovery
    - Add proper error messages for failed toggles
    - Implement session cleanup on exit
    - **Acceptance:** System handles edge cases gracefully

### Phase 6: Testing and Documentation
11. **Integration Testing** [3 hours]
    - Test hotkey functionality across different scenarios
    - Validate session state preservation
    - Test process management and cleanup
    - **Acceptance:** Full hotkey toggle workflow works end-to-end

12. **Documentation Updates** [1 hour]
    - Update help command with hotkey information
    - Add configuration documentation
    - Update README with feature description
    - **Acceptance:** Users can discover and understand the feature

## Validation Commands

### Basic Functionality Tests
```bash
# Start kiro-krew console
kiro-krew

# In console, start planning session
plan "test feature"

# Test hotkey toggle (manual verification)
# Press Ctrl+Option+P to switch to console
# Press Ctrl+Option+P again to return to planning

# Verify session persistence
exit
kiro-krew
plan  # Should resume previous session
```

### Session State Verification
```bash
# Check session files are created
ls -la .kiro-krew/sessions/

# Verify session content structure
cat .kiro-krew/sessions/planning-*.json | jq '.'

# Test session cleanup
kiro-krew
# Exit cleanly and verify sessions are cleaned up
ls -la .kiro-krew/sessions/
```

### Error Condition Testing
```bash
# Test hotkey outside kiro-krew context
# Should not respond to Ctrl+Option+P

# Test toggle with no planning session
kiro-krew
# Press Ctrl+Option+P from console
# Should display "No active planning session"

# Test session corruption recovery
# Corrupt a session file and verify graceful handling
```

### Configuration Testing
```bash
# Test with different session limits
echo "session_history_limit: 50" >> .kiro-krew/config.yaml
kiro-krew

# Verify configuration is respected
# Create long conversation and check truncation
```
