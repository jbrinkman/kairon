# TUI Agent Output View with Planning Mode Compatibility - Design Specification

**Closes #45**

## Solution Approach

Add a toggleable agent output view within the existing TUI that captures and displays real-time agent progress without ANSI control characters. The solution integrates with the existing planning mode workflow to ensure terminal interface compatibility and proper output suspension/resumption during mode transitions.

### Architecture Strategy

1. **Output Capture System**: Extend the existing `prefixedWriter` in `internal/agent/manager.go` to multiplex output between console, file logging, and a structured capture buffer that strips ANSI sequences.

2. **Dual-View TUI**: Add a second view mode to the existing TUI that displays captured agent output with scrolling and text wrapping, toggled via key binding.

3. **Planning Mode Integration**: Hook into existing session management to suspend agent output capture during planning mode and resume when returning to console mode.

4. **State Preservation**: Maintain separate state for both main TUI view and agent output view to preserve scroll position and content when switching.

## Relevant Files

### Files to Create
- `internal/tui/output_view.go` - Agent output view component with scrolling and text wrapping
- `internal/agent/output_capture.go` - ANSI-stripping output capture system
- `internal/tui/view_state.go` - State management for view switching

### Files to Modify
- `internal/tui/tui.go` - Add view toggling, state management, and planning mode hooks
- `internal/agent/manager.go` - Extend output routing to support capture buffer
- `internal/tui/styles.go` - Add styling for agent output view
- `internal/session/planner.go` - Add output capture suspend/resume hooks

### Files Referenced/Relevant  
- `internal/tui/commands.go` - Existing TUI command handling patterns
- `internal/config/config.go` - Console logging configuration
- `internal/hotkey/detector.go` - Key binding detection patterns

## Team Orchestration

### Output Capture Component
- Implements ANSI sequence stripping using regex patterns
- Provides thread-safe buffer management for multi-agent scenarios  
- Integrates with existing file logging without disruption
- Handles output routing based on console logging and capture state

### TUI View Management Component
- Manages view state switching between main and agent output views
- Handles key binding for view toggle (e.g., 'o' for output)
- Preserves scroll positions and content state across view switches
- Implements agent output display with proper text wrapping and scrolling

### Planning Mode Integration Component
- Hooks into existing session suspend/resume lifecycle
- Coordinates output capture suspension before planning mode entry
- Ensures clean terminal handoff to planning interface
- Restores output capture when returning from planning mode

## Step-by-Step Task Breakdown

### Task 1: Implement Output Capture System
**Acceptance Criteria:**
- ANSI control sequences are stripped from agent output
- Output is captured in thread-safe buffer with configurable size limits
- Existing file logging continues unchanged
- Console logging behavior respects existing configuration

**Implementation Steps:**
1. Create `internal/agent/output_capture.go` with `OutputCapture` struct
2. Implement ANSI stripping using regex: `\x1b\[[0-9;]*[mK]`
3. Add thread-safe circular buffer for captured lines
4. Create `CaptureWriter` that wraps existing `prefixedWriter`

### Task 2: Create Agent Output View Component  
**Acceptance Criteria:**
- Scrollable view displays captured agent output with text wrapping
- View handles terminal resize gracefully
- Multiple agents' output is clearly distinguished
- Empty state shows helpful message when no agents are running

**Implementation Steps:**
1. Create `internal/tui/output_view.go` with Bubble Tea model
2. Implement scrolling with viewport component from Bubble Tea
3. Add text wrapping using lipgloss width constraints
4. Style agent output with prefixes and timestamps

### Task 3: Implement View State Management
**Acceptance Criteria:**
- Smooth transitions between main TUI and agent output views  
- State preservation maintains scroll positions and content
- Key binding toggles views without breaking TUI flow
- View state survives terminal resize events

**Implementation Steps:**
1. Create `internal/tui/view_state.go` with view types and state structs
2. Extend main TUI model to track current view and preserve states
3. Add key binding handler for view toggle
4. Implement view rendering delegation based on current view

### Task 4: Integrate with Planning Mode Workflow
**Acceptance Criteria:**
- Agent output capture suspends cleanly before planning mode entry
- Planning mode terminal interface operates without interference
- Output capture resumes automatically when returning to console mode
- No agent output corruption or loss during mode transitions

**Implementation Steps:**
1. Add suspend/resume methods to `OutputCapture` 
2. Hook suspend call into existing planning mode entry points
3. Hook resume call into planning mode exit handlers
4. Add planning mode state tracking to prevent output interference

### Task 5: Extend TUI with View Toggle and Display
**Acceptance Criteria:**
- Key binding ('o') toggles between main and agent output views
- Agent output view integrates seamlessly with existing TUI styling
- View toggle works in both console and planning mode contexts
- Help system documents the new key binding

**Implementation Steps:**
1. Modify `internal/tui/tui.go` to handle view state switching
2. Add key binding detection for output view toggle
3. Integrate output view rendering with existing TUI structure
4. Update help command to document new functionality
5. Apply consistent theming to agent output view

### Task 6: Testing and Integration Validation
**Acceptance Criteria:**
- Multiple agents running simultaneously display properly
- Output capture works with existing console logging settings
- Planning mode transitions work without terminal corruption
- Performance remains acceptable with high-volume agent output

**Implementation Steps:**
1. Test with multiple concurrent agents
2. Validate ANSI stripping with various terminal outputs
3. Test planning mode integration thoroughly
4. Verify memory usage with long-running agents
5. Test terminal resize handling in both views

## Validation Commands

```bash
# Start kiro-krew TUI
kiro-krew watch

# In TUI console:
watch start                    # Start watching for issues
status                        # Verify agents are running  

# Test output view toggle
# Press 'o' to switch to agent output view
# Press 'o' again to return to main view

# Test with planning mode
# Press Ctrl+Alt+P to enter planning mode
# Verify no agent output interference
# Press Ctrl+Alt+P to return to console mode
# Press 'o' to verify output view still works

# Test multiple agents
# Create issues with kiro-krew label to spawn multiple agents
# Verify output from all agents appears in output view

# Test console logging compatibility  
# Set console_logging: true in config
# Verify output appears in both terminal and capture view
```

## Configuration Integration

The agent output view respects existing configuration settings:

- **console_logging**: When enabled, output goes to terminal, file, AND capture buffer
- **max_activity_lines**: Applied to both main activity and agent output buffers  
- **theme**: Agent output view uses same theming as main TUI
- **planning mode**: Existing hotkey and session management integration

## Planning Mode Compatibility Details

The implementation ensures clean integration with the existing planning mode workflow:

1. **Output Suspension**: When `switchToPlanningMode()` is called, agent output capture is suspended before terminal handoff
2. **Clean Terminal**: Planning mode receives a clean terminal without background agent output
3. **State Preservation**: Agent output buffer content is preserved during planning sessions
4. **Automatic Resumption**: Returning to console mode via `switchToConsoleMode()` automatically resumes agent output capture
5. **No Interference**: Suspended output capture prevents any terminal conflicts with planning agent interface

This design ensures the agent output view enhances the existing TUI without disrupting the established planning mode workflow or terminal management.
