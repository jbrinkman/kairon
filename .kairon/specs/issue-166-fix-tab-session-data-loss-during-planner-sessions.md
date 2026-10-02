# Fix Tab Session Data Loss During Planner Sessions

**Issue**: #166  
**Title**: Fix tab session data loss during planner sessions  
**Closes**: #166

## Problem Analysis

The tab session logic has a bug where session data for agent tabs and main TUI watcher logs gets lost during planner sessions due to output capture suspension/resume during mode switching.

### Root Cause

The issue stems from the **complete suspension** of output capture when entering planner mode:

1. In `switchToPlanningMode()` (internal/tui/commands.go:261), `m.manager.SuspendOutputCapture()` is called
2. `SuspendOutputCapture()` (internal/agent/manager.go:81) sets the `suspended` flag in `OutputCapture`
3. When `suspended = true`, `AddLine()` (internal/agent/output_capture.go:43) returns early, dropping all incoming output
4. Agent processes continue running and generating output, but it's completely discarded
5. When returning to console mode, `ResumeOutputCapture()` only resumes capture going forward - historical data lost during suspension is not recoverable

### Impact

- Agent tab logs show gaps during planner sessions
- Main TUI watcher logs miss critical events  
- Users lose context when switching back from planner mode
- History limits are still respected, but effective history is shorter due to gaps

## Solution Approach

**Modify the suspend/resume mechanism to pause terminal display while continuing data accumulation.**

The key insight is that suspension should only affect **display output** (terminal logging), not **data storage** (OutputCapture buffer and agent tab history).

### Architecture Changes

1. **Decouple Display from Storage**: Separate terminal output suspension from data capture suspension
2. **Conditional Terminal Output**: Use existing `terminalOutputEnabled()` mechanism for display control
3. **Continuous Data Accumulation**: Keep OutputCapture.AddLine() active during planner sessions
4. **Preserve Session History**: Maintain tab session data and watcher logs during mode switches

## Detailed Implementation Plan

### 1. Modify OutputCapture Behavior

**File**: `internal/agent/output_capture.go`

**Changes**:
- Remove suspension logic from `AddLine()` method
- Keep data accumulation active regardless of suspension state  
- Suspension only affects external consumers, not internal storage

**Rationale**: OutputCapture should be a pure data store, not concerned with display state.

### 2. Update Manager Suspend/Resume Logic

**File**: `internal/agent/manager.go`

**Changes**:
- `SuspendOutputCapture()`: Only suspend terminal output display, keep data capture active
- `ResumeOutputCapture()`: Only resume terminal output display
- Remove calls to `outputCapture.Suspend()/Resume()`
- Rely on `terminalOutputEnabled()` for display control

**Rationale**: The manager already has the correct conditional terminal output infrastructure.

### 3. Preserve Console State During Mode Switches  

**File**: `internal/tui/commands.go`

**Changes**:
- Ensure `consoleState` preservation includes all activity lines
- Maintain agent output generation counter state
- No changes to suspend/resume calls - they work correctly with the new approach

**Rationale**: Console state restoration already works, just needs the underlying data to persist.

### 4. Tab Session Data Continuity

**Files**: `internal/tui/tab_manager.go`, `internal/tui/agent_tab.go`

**Changes**:
- No direct changes needed - tabs automatically benefit from continuous data capture
- Agent tabs will maintain full history when output capture continues running
- Tab switching will show complete logs including planner session periods

**Rationale**: Tabs consume data from OutputCapture, so fixing the capture fixes the tabs.

## Step-by-Step Task Breakdown

### Task 1: Remove Suspension from OutputCapture.AddLine()

**File**: `internal/agent/output_capture.go`

**Acceptance Criteria**:
- `AddLine()` method continues storing data regardless of suspension state
- Remove the early return when `suspended = true`
- Data accumulation works continuously during planner sessions
- History limits (ring buffer) still enforced

### Task 2: Update Manager Suspend/Resume Methods

**File**: `internal/agent/manager.go`  

**Acceptance Criteria**:
- `SuspendOutputCapture()` only sets `terminalOutputPaused = true`
- Remove `outputCapture.Suspend()` call
- `ResumeOutputCapture()` only sets `terminalOutputPaused = false`  
- Remove `outputCapture.Resume()` call
- Terminal output correctly paused/resumed via `terminalOutputEnabled()`

### Task 3: Verify Console State Preservation

**File**: `internal/tui/commands.go`

**Acceptance Criteria**:
- `switchToPlanningMode()` preserves complete console state
- `switchToConsoleMode()` restores full activity history  
- Agent output generation counters maintain continuity
- No session data lost during mode transitions

### Task 4: Integration Testing

**Acceptance Criteria**:
- Start agents, switch to planner mode, return to console - full history visible
- Agent tabs show continuous logs with no gaps during planner sessions
- Main TUI watcher logs accumulate during planner sessions
- History limits respected but no artificial gaps from suspension
- Performance not impacted by continuous capture during planning

## Validation Commands

```bash
# Test scenario: Agent output continuity during planner sessions
go run ./cmd/kiro-krew
# In TUI:
# 1. watch start (start watcher)
# 2. Create labeled GitHub issue to spawn agent
# 3. Observe agent output accumulating 
# 4. Ctrl+Alt+P (switch to planner mode)
# 5. Wait in planner mode while agent continues
# 6. Ctrl+Alt+P (return to console mode)  
# 7. Open agent tab - should see continuous log history
# 8. Check main console - should see watcher logs from during planner session

# Verify no performance regression
go test -bench=. ./internal/agent/...
go test -bench=. ./internal/tui/...

# Edge case: Rapid mode switching
# Switch to planner and back quickly multiple times
# Verify no data corruption or loss
```

## Risk Assessment

**Low Risk Changes**:
- Removing early return from AddLine() - simple logic change
- Manager suspend/resume modification - uses existing terminalOutputEnabled() mechanism

**Potential Issues**:
- **Memory usage**: Continuous capture during long planner sessions could accumulate more data
  - **Mitigation**: Existing ring buffer limits prevent unbounded growth
- **Performance**: OutputCapture.AddLine() called more frequently  
  - **Mitigation**: Method is already optimized, no additional processing added

**Backwards Compatibility**:
- No breaking changes to APIs
- Existing session files remain compatible
- Terminal output behavior unchanged from user perspective

## Testing Strategy

1. **Unit Tests**: Verify OutputCapture continues accumulating when "suspended"
2. **Integration Tests**: Full planner mode switching with agent output verification  
3. **Manual Testing**: Long-running agents during extended planner sessions
4. **Performance Tests**: Ensure no regression in capture performance
5. **Edge Cases**: Rapid mode switching, agent startup/shutdown during planning

## Success Metrics

- ✅ Agent tabs retain full log history after planner sessions
- ✅ Main TUI watcher logs show no gaps during planner sessions  
- ✅ History limits still enforced (no unbounded memory growth)
- ✅ Terminal output correctly paused/resumed (user experience unchanged)
- ✅ No performance degradation in output capture or mode switching
- ✅ All existing functionality works normally