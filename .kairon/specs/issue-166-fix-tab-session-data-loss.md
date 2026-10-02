# Fix Tab Session Data Loss During Planner Sessions

**Issue**: #166  
**Title**: Fix tab session data loss during planner sessions  
**Closes**: #166

## Problem Analysis

The root cause of session data loss is that `SuspendOutputCapture()` and `ResumeOutputCapture()` in the agent manager only controls terminal display output, but the output capture system (`OutputCapture`) continues to receive and store data. However, the session management system may be interfering with data accumulation during mode switching.

After analyzing the codebase, the issue appears to be in the interaction between:

1. **Agent Output Capture**: `OutputCapture` in `internal/agent/output_capture.go` has `Suspend()` and `Resume()` methods that are **not used** by the agent manager
2. **Agent Manager**: `SuspendOutputCapture()` and `ResumeOutputCapture()` only control terminal display via the `terminalOutputPaused` flag
3. **Mode Switching**: The TUI commands properly call suspend/resume on the manager but data should continue accumulating

The problem is that data **should** continue accumulating during planner sessions, but there may be edge cases where the capture writers or tab state is being affected.

## Solution Approach

Implement a **two-tier separation** between display suspension and data accumulation:

1. **Display Suspension**: Continue using the existing `terminalOutputPaused` mechanism to control terminal output display
2. **Data Accumulation**: Ensure that `OutputCapture.AddLine()` continues working during planner sessions and that tab session data persists correctly

The key insight is that **data capture should never be suspended** - only the terminal display should be suspended during planner mode.

## Relevant Files

### Files to Modify
- `internal/agent/manager.go` - Update suspend/resume logic to ensure data continues accumulating
- `internal/agent/output_capture.go` - Remove or clarify unused suspend/resume methods 
- `internal/tui/commands.go` - Verify mode switching preserves session data
- `internal/tui/tab_manager.go` - Ensure tabs maintain their data during mode switches

### Files to Review/Test
- `internal/session/manager.go` - Session persistence during mode changes
- `internal/session/planner.go` - Planning mode session handling
- `internal/tui/agent_tab.go` - Agent tab data handling
- `internal/tui/main_tab.go` - Main tab data handling

## Team Orchestration

This is a **data integrity** fix that requires careful coordination:

1. **Agent Output System**: Ensure output capture continues during display suspension
2. **Tab Management**: Verify tab data persistence across mode switches  
3. **Session Management**: Ensure planning sessions don't interfere with console data
4. **Mode Switching**: Preserve all session data during transitions

## Step-by-Step Task Breakdown

### Task 1: Clarify Output Capture Suspend/Resume Semantics
**Acceptance Criteria**:
- Remove unused `Suspend()` and `Resume()` methods from `OutputCapture` struct
- Document that `OutputCapture` should always accumulate data regardless of display state
- Ensure `AddLine()` is never blocked during planner sessions

### Task 2: Verify Agent Manager Suspend/Resume Logic
**Acceptance Criteria**:
- Confirm `SuspendOutputCapture()` only affects `terminalOutputPaused` flag
- Verify that `CaptureWriter` continues calling `AddLine()` during suspension
- Ensure agent log files continue being written during planner sessions
- Test that `GetOutputLines()` returns complete data after resume

### Task 3: Audit Mode Switching Data Preservation  
**Acceptance Criteria**:
- Verify `switchToPlanningMode()` doesn't lose console output history
- Verify `switchToConsoleMode()` doesn't lose agent output history
- Ensure tab state is preserved during mode transitions
- Test rapid mode switching doesn't cause data loss

### Task 4: Strengthen Tab Session Data Persistence
**Acceptance Criteria**:
- Verify tab data accumulates continuously during planner sessions
- Ensure agent tabs retain full log history when switching back from planner
- Test that main TUI watcher logs continue accumulating during planner mode
- Validate history limits are still respected

### Task 5: Add Session Data Validation
**Acceptance Criteria**:
- Add validation to ensure session data integrity across mode switches
- Implement recovery mechanisms for corrupted session states
- Add logging to track session data flow during suspend/resume cycles
- Test edge cases like rapid mode switching

## Validation Commands

```bash
# Test basic functionality
kiro-krew
# In TUI: watch start, then Ctrl+Alt+P to switch to planning mode
# Verify agents continue logging in background
# Switch back with Ctrl+Alt+P and check tab data is preserved

# Test session data persistence
ls .kiro-krew/sessions/  # Check session files are maintained
ls .kiro-krew/logs/      # Check agent logs continue during planner mode

# Test tab data integrity  
# Start multiple agents, switch to planner mode, wait, switch back
# Verify all agent tabs retain their complete log history

# Test rapid mode switching
# Rapidly switch between console and planning modes
# Verify no data loss occurs

# Test with existing sessions
# Create planning session, suspend, start agents, resume planning
# Verify both planning and agent data are preserved
```

## Technical Implementation Notes

### Key Insights
1. The `OutputCapture.suspended` field is **not used** by the agent manager
2. Data accumulation should **never stop** - only display should be suspended
3. Session files in `.kiro-krew/sessions/` should persist independently of mode switches
4. Tab state should be preserved by the tab manager regardless of current mode

### Risk Areas
- Race conditions during rapid mode switching
- Session corruption during unexpected termination  
- Memory usage if history accumulates indefinitely during long planner sessions
- Edge cases where suspend/resume is called multiple times

### Testing Strategy
- Unit tests for `OutputCapture` data integrity during suspend/resume cycles
- Integration tests for mode switching with active agents
- End-to-end tests for complete session data preservation workflows
- Performance tests for memory usage during extended planner sessions