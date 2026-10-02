# Design Specification: Focus State Tracking Across All Tab Types

**Issue**: #256  
**Title**: Add focus state tracking to all tab types  
**Closes**: #256

## Problem Statement

Currently, focus management in the kiro-krew TUI is inconsistent across tab types. When switching between tabs, cursor focus is not preserved correctly, forcing users to perform manual workarounds to restore proper input focus.

**Root Cause Analysis**:
- Only planning tabs implement focus state tracking via `focusInput` boolean and `RestoreFocus()` method
- `switchActiveTab()` in `tui.go` (lines 1103-1125) handles focus restoration asymmetrically:
  - **Planning tabs**: Restore preserved state via `pt.RestoreFocus()`
  - **Main/Agent/Log tabs**: Always call `m.input.SetFocus(true)` regardless of previous state
- Focus state is **not captured** before switching away from non-planning tabs
- No mouse click handling for input field focus recovery

## Solution Approach

Implement a **unified focus state tracking system** that:

1. **Extends the Tab interface** with focus capture/restore methods
2. **Implements focus state tracking** in all tab types (Main, Agent, Log, Planning)
3. **Centralizes focus management** in `switchActiveTab()` to eliminate special-case logic
4. **Adds mouse click support** for input field focus recovery
5. **Maintains backward compatibility** with existing behavior

### Architectural Decisions

**Focus State Representation**:
- Use a simple enum-based `FocusTarget` type to represent which input field should have focus
- For tabs with single input (Main/Agent/Log), focus state returns constant "footer"
- For planning tabs, track "footer" vs "message" input focus

**Interface Design**:
- Add two methods to `Tab` interface: `CaptureFocusState()` and `RestoreFocusState(state)`
- Keep methods lightweight - capture returns current state, restore applies saved state
- Return `tea.Cmd` from restore method to support focus commands

**Centralized Management**:
- `switchActiveTab()` becomes the single point for all focus state transitions
- Tab-specific logic moves into tab implementations
- Eliminates type assertions and special cases in main model

## Relevant Files

### Core Files (Must Modify)
- **internal/tui/tabs.go**: Tab interface extension with focus methods
- **internal/tui/tui.go**: Centralized focus management in `switchActiveTab()`, mouse click handling
- **internal/tui/focus_state.go**: New file - `FocusTarget` type and focus state utilities
- **internal/tui/main_tab.go**: Implement focus state tracking (footer-only)
- **internal/tui/agent_tab.go**: Implement focus state tracking (footer-only)
- **internal/tui/log_tab.go**: Implement focus state tracking (footer-only)
- **internal/tui/planning_tab.go**: Refactor existing focus tracking to new interface

### Testing Files (May Add)
- **internal/tui/focus_state_test.go**: Unit tests for focus state logic
- **internal/tui/tabs_test.go**: Integration tests for tab focus transitions

## Team Orchestration

### Task Dependencies

```
Task 1: Define Focus State Types
    ↓
Task 2: Extend Tab Interface
    ↓
Task 3: Implement Focus State in Simple Tabs (Main/Agent/Log)
    ↓
Task 4: Refactor Planning Tab Focus Logic
    ↓
Task 5: Centralize Focus Management in switchActiveTab()
    ↓
Task 6: Add Mouse Click Focus Recovery
    ↓
Task 7: Testing and Validation
```

**Parallelization**: Tasks 3 and 4 can run in parallel after Task 2 completes.

### Implementation Sequence

All tasks contribute to complete issue resolution in one pull request. Each task builds upon the previous to create a unified focus management system.

## Step-by-Step Task Breakdown

### Task 1: Define Focus State Types

**File**: `internal/tui/focus_state.go` (new file)

**Acceptance Criteria**:
- Create `FocusTarget` enum type with constants: `FocusTargetFooter`, `FocusTargetMessage`, `FocusTargetNone`
- Type is string-based for easy debugging and logging
- Include helper methods: `String()`, `IsValid()`

**Implementation Details**:
```go
package tui

// FocusTarget represents which input field should have focus
type FocusTarget string

const (
    // FocusTargetFooter indicates the footer input should have focus
    FocusTargetFooter FocusTarget = "footer"
    
    // FocusTargetMessage indicates the planning tab message input should have focus
    FocusTargetMessage FocusTarget = "message"
    
    // FocusTargetNone indicates no input should have focus (viewport scroll mode)
    FocusTargetNone FocusTarget = "none"
)

// String returns the string representation of the focus target
func (ft FocusTarget) String() string {
    return string(ft)
}

// IsValid checks if the focus target is a valid value
func (ft FocusTarget) IsValid() bool {
    switch ft {
    case FocusTargetFooter, FocusTargetMessage, FocusTargetNone:
        return true
    default:
        return false
    }
}
```

**Dependencies**: None

---

### Task 2: Extend Tab Interface

**File**: `internal/tui/tabs.go`

**Acceptance Criteria**:
- Add `CaptureFocusState() FocusTarget` method to Tab interface
- Add `RestoreFocusState(target FocusTarget) tea.Cmd` method to Tab interface
- Interface remains backward compatible (implementations will be added in subsequent tasks)

**Implementation Details**:
```go
// Tab interface defines the contract for all tab implementations
type Tab interface {
    ID() string
    Type() TabType
    Title() string
    IsClosable() bool
    View() string
    Update(tea.Msg) (Tab, tea.Cmd)
    Resize(width, height int)
    
    // Focus state management
    CaptureFocusState() FocusTarget
    RestoreFocusState(target FocusTarget) tea.Cmd
}
```

**Dependencies**: Task 1 (requires `FocusTarget` type)

---

### Task 3: Implement Focus State in Simple Tabs (Main/Agent/Log)

**Files**: 
- `internal/tui/main_tab.go`
- `internal/tui/agent_tab.go`
- `internal/tui/log_tab.go`

**Acceptance Criteria**:
- Each tab implements `CaptureFocusState()` returning `FocusTargetFooter`
- Each tab implements `RestoreFocusState(target)` as no-op (returns nil)
- Simple tabs always indicate footer should have focus (stateless)
- No changes to existing tab behavior or fields

**Implementation Details**:

For **main_tab.go**:
```go
// CaptureFocusState returns the current focus state for the main tab
func (mt *MainTab) CaptureFocusState() FocusTarget {
    // Main tab always uses footer input
    return FocusTargetFooter
}

// RestoreFocusState restores the focus state for the main tab
func (mt *MainTab) RestoreFocusState(target FocusTarget) tea.Cmd {
    // Main tab doesn't manage focus directly - handled by parent model
    return nil
}
```

For **agent_tab.go**:
```go
// CaptureFocusState returns the current focus state for the agent tab
func (at *AgentTab) CaptureFocusState() FocusTarget {
    // Agent tab always uses footer input
    return FocusTargetFooter
}

// RestoreFocusState restores the focus state for the agent tab
func (at *AgentTab) RestoreFocusState(target FocusTarget) tea.Cmd {
    // Agent tab doesn't manage focus directly - handled by parent model
    return nil
}
```

For **log_tab.go**:
```go
// CaptureFocusState returns the current focus state for the log tab
func (lt *LogTab) CaptureFocusState() FocusTarget {
    // Log tab always uses footer input
    return FocusTargetFooter
}

// RestoreFocusState restores the focus state for the log tab
func (lt *LogTab) RestoreFocusState(target FocusTarget) tea.Cmd {
    // Log tab doesn't manage focus directly - handled by parent model
    return nil
}
```

**Dependencies**: Task 2 (requires extended Tab interface)

---

### Task 4: Refactor Planning Tab Focus Logic

**File**: `internal/tui/planning_tab.go`

**Acceptance Criteria**:
- Implement `CaptureFocusState()` returning current focus state based on `focusInput` field
- Implement `RestoreFocusState(target)` to replace existing `RestoreFocus()` method
- Convert `focusInput` boolean to use `FocusTarget` internally (or map from boolean)
- Existing `RestoreFocus()` method kept for backward compatibility during transition
- All existing focus behavior preserved (viewport scrolling, Esc key handling, etc.)

**Implementation Details**:
```go
// CaptureFocusState returns the current focus state for the planning tab
func (pt *PlanningTab) CaptureFocusState() FocusTarget {
    if pt.focusInput {
        return FocusTargetMessage
    }
    // When focusInput is false, check if in scroll mode (viewport navigation)
    // If neither message input nor footer has focus, we're in scroll/none mode
    return FocusTargetNone
}

// RestoreFocusState restores the focus state for the planning tab
func (pt *PlanningTab) RestoreFocusState(target FocusTarget) tea.Cmd {
    switch target {
    case FocusTargetMessage:
        pt.focusInput = true
        return pt.textinput.Focus()
    case FocusTargetFooter:
        pt.focusInput = false
        pt.textinput.Blur()
        return nil
    case FocusTargetNone:
        pt.focusInput = false
        pt.textinput.Blur()
        return nil
    default:
        // Invalid target, default to no focus
        pt.focusInput = false
        pt.textinput.Blur()
        return nil
    }
}

// RestoreFocus is kept for backward compatibility
// It now delegates to RestoreFocusState
func (pt *PlanningTab) RestoreFocus() tea.Cmd {
    target := pt.CaptureFocusState()
    return pt.RestoreFocusState(target)
}
```

**Note**: The `focusInput` boolean field is retained for internal state tracking. Converting to `FocusTarget` would require broader refactoring of planning tab logic.

**Dependencies**: Task 2 (requires extended Tab interface)

---

### Task 5: Centralize Focus Management in switchActiveTab()

**File**: `internal/tui/tui.go`

**Acceptance Criteria**:
- Add focus state storage map to model: `tabFocusStates map[string]FocusTarget`
- Capture focus state from current tab before switching
- Store captured state in map keyed by tab ID
- Retrieve and restore focus state for new active tab
- Apply focus to footer input based on restored target
- Remove all tab-type-specific focus logic (planning tab special case)
- Handle case where no previous state exists (default to footer focus)

**Implementation Details**:

1. **Add field to model struct**:
```go
type model struct {
    // ... existing fields ...
    
    // Focus state tracking per tab
    tabFocusStates map[string]FocusTarget
}
```

2. **Initialize in newModel**:
```go
func newModel(...) model {
    // ... existing initialization ...
    
    return model{
        // ... existing fields ...
        tabFocusStates: make(map[string]FocusTarget),
    }
}
```

3. **Refactor switchActiveTab**:
```go
// switchActiveTab switches to a tab by index and updates context tracking
func (m model) switchActiveTab(index int) (model, tea.Cmd) {
    var cmds []tea.Cmd
    
    // Capture focus state from current active tab before switching
    if currentTab := m.tabManager.GetActiveTab(); currentTab != nil {
        currentFocus := currentTab.CaptureFocusState()
        m.tabFocusStates[currentTab.ID()] = currentFocus
        logging.Debug("captured focus state", 
            "tab_id", currentTab.ID(), 
            "focus_target", currentFocus)
    }
    
    // Switch to new tab
    m.tabManager.SetActiveTab(index)
    
    // Restore focus state for newly active tab
    if activeTab := m.tabManager.GetActiveTab(); activeTab != nil {
        // Handle planning context tracking
        if activeTab.Type() == TabTypePlanning {
            if !m.footerManager.GetContextTracker().IsActive() {
                m.footerManager.GetContextTracker().StartPlanningSession("claude-sonnet-4")
            }
        } else {
            if m.footerManager.GetContextTracker().IsActive() {
                m.footerManager.GetContextTracker().StopPlanningSession()
            }
        }
        
        // Retrieve previous focus state or default to footer
        previousFocus, exists := m.tabFocusStates[activeTab.ID()]
        if !exists {
            // Default: footer focus for new tabs
            previousFocus = FocusTargetFooter
        }
        
        logging.Debug("restoring focus state", 
            "tab_id", activeTab.ID(), 
            "focus_target", previousFocus)
        
        // Apply focus state to the tab
        if cmd := activeTab.RestoreFocusState(previousFocus); cmd != nil {
            cmds = append(cmds, cmd)
        }
        
        // Apply footer input focus based on target
        if previousFocus == FocusTargetFooter {
            m.input.SetFocus(true)
            cmds = append(cmds, m.input.Focus())
        } else {
            m.input.SetFocus(false)
        }
    }
    
    return m, tea.Batch(cmds...)
}
```

**Dependencies**: Task 3, Task 4 (requires all tabs to implement focus interface)

---

### Task 6: Add Mouse Click Focus Recovery

**File**: `internal/tui/tui.go`

**Acceptance Criteria**:
- Detect mouse clicks on footer input area and transfer focus
- Detect mouse clicks on planning tab message input area and transfer focus
- Update tab's focus state when mouse click changes focus
- Mouse clicks work regardless of previous focus state
- Clicks on other UI areas (viewport, tab headers) don't affect input focus

**Implementation Details**:

1. **Calculate input field boundaries**:
```go
// isClickInFooterInput checks if mouse click is in the footer input area
func (m model) isClickInFooterInput(mouseX, mouseY int) bool {
    // Footer is at the bottom of the screen
    // Input area starts after prompt text and spans footer width
    footerY := m.height - footerHeight
    if mouseY < footerY {
        return false
    }
    
    // Check if click is on the input line (not help text line)
    inputLineY := footerY + 1 // Adjust based on actual footer layout
    return mouseY == inputLineY && mouseX >= len(m.footerManager.GetPrompt())
}
```

2. **Add mouse click handler in Update method**:
```go
case tea.MouseClickMsg:
    // Handle mouse clicks on tab headers when no overlay active
    if m.activeOverlay == overlayNone {
        mouse := msg.Mouse()
        
        // Check if click is in the tab header area (first line)
        if mouse.Y < tabHeaderHeight {
            m.tabManager.HandleTabHeaderClick(mouse.X)
            m.checkLogTabClosed()
            return m, nil
        }
        
        // Check if click is in footer input area
        if m.isClickInFooterInput(mouse.X, mouse.Y) {
            activeTab := m.tabManager.GetActiveTab()
            if activeTab != nil {
                // Update tab's focus state
                m.tabFocusStates[activeTab.ID()] = FocusTargetFooter
                
                // Apply focus to footer input
                m.input.SetFocus(true)
                
                // If on planning tab, update its focus state
                if activeTab.Type() == TabTypePlanning {
                    if pt, ok := activeTab.(*PlanningTab); ok {
                        pt.SetFocusInput(false)
                        pt.textinput.Blur()
                    }
                }
                
                return m, m.input.Focus()
            }
        }
    }
    
    // Forward to active tab (for planning tab message input clicks)
    if cmd := m.tabManager.Update(msg); cmd != nil {
        return m, cmd
    }
    
    m.checkLogTabClosed()
    return m, nil
```

3. **Add mouse click handling in PlanningTab.Update**:
```go
case tea.MouseClickMsg:
    mouse := msg.Mouse()
    
    // Calculate message input area boundaries
    // Input is at the bottom of the planning tab content area
    inputY := pt.height - pt.inputHeight
    
    if mouse.Y >= inputY && mouse.Y < pt.height {
        // Click is in the message input area
        if !pt.focusInput {
            logging.Debug("mouse click focus transfer to message input", "tab_id", pt.id)
            pt.focusInput = true
            return pt, pt.textinput.Focus()
        }
    }
```

**Dependencies**: Task 5 (requires centralized focus management and state tracking)

---

### Task 7: Testing and Validation

**Files**: 
- `internal/tui/focus_state_test.go` (new)
- Manual testing procedures

**Acceptance Criteria**:
- Unit tests for `FocusTarget` type helpers
- Manual test: Switch from planning tab (message input focused) → main tab → footer has focus
- Manual test: Switch from main tab → planning tab → message input focus restored
- Manual test: Switch between agent tabs → footer focus preserved
- Manual test: Click on footer input → footer receives focus
- Manual test: Click on planning message input → message input receives focus
- Manual test: Focus persists through multiple tab switches (main → planning → agent → planning)
- Manual test: Viewport scrolling (pgup/pgdown/home/end) correctly removes focus
- No regressions in existing tab switching behavior

**Implementation Details**:

1. **Unit tests** (`focus_state_test.go`):
```go
package tui

import "testing"

func TestFocusTargetString(t *testing.T) {
    tests := []struct {
        target   FocusTarget
        expected string
    }{
        {FocusTargetFooter, "footer"},
        {FocusTargetMessage, "message"},
        {FocusTargetNone, "none"},
    }
    
    for _, tt := range tests {
        if got := tt.target.String(); got != tt.expected {
            t.Errorf("FocusTarget.String() = %v, want %v", got, tt.expected)
        }
    }
}

func TestFocusTargetIsValid(t *testing.T) {
    tests := []struct {
        target   FocusTarget
        expected bool
    }{
        {FocusTargetFooter, true},
        {FocusTargetMessage, true},
        {FocusTargetNone, true},
        {FocusTarget("invalid"), false},
        {FocusTarget(""), false},
    }
    
    for _, tt := range tests {
        if got := tt.target.IsValid(); got != tt.expected {
            t.Errorf("FocusTarget(%q).IsValid() = %v, want %v", 
                tt.target, got, tt.expected)
        }
    }
}
```

2. **Manual Testing Procedure**:
```markdown
# Focus State Testing Checklist

## Basic Tab Switching
- [ ] Start on main tab, type in footer input
- [ ] Switch to planning tab - footer input should be blurred
- [ ] Planning tab message input should NOT have focus by default
- [ ] Switch back to main tab - footer input should have focus

## Planning Tab Focus Persistence
- [ ] On planning tab, press Enter to focus message input
- [ ] Type some text (don't send)
- [ ] Switch to main tab
- [ ] Switch back to planning tab
- [ ] Message input should still have focus with text preserved

## Viewport Scroll Mode
- [ ] On planning tab with message input focused
- [ ] Press PgDown to enter scroll mode
- [ ] Message input should lose focus (no cursor visible)
- [ ] Switch to main tab, then back to planning tab
- [ ] Should restore "no focus" state (scroll mode)

## Agent Tab Focus
- [ ] Switch to an agent tab
- [ ] Footer input should have focus
- [ ] Switch to different agent tab
- [ ] Footer input should maintain focus

## Mouse Click Focus Recovery
- [ ] On planning tab with message input focused
- [ ] Click on footer input area
- [ ] Footer input should receive focus, message input should blur
- [ ] Click on message input area
- [ ] Message input should receive focus, footer should blur

## Complex Tab Switching Sequence
- [ ] Main tab (footer focused) → Planning tab (message focused) → Agent tab
- [ ] Agent tab footer should have focus
- [ ] Switch back to planning tab
- [ ] Message input focus should be restored
- [ ] Switch back to main tab
- [ ] Footer focus should be restored

## ESC Key Focus Transfer
- [ ] Planning tab with message input focused
- [ ] Press ESC
- [ ] Focus should transfer to footer
- [ ] Press ESC again
- [ ] Focus should transfer back to message input
```

**Dependencies**: Tasks 1-6 (requires complete implementation)

---

## Validation Commands

### Build and Run
```bash
# Build the project
go build ./cmd/kiro-krew

# Run the TUI
./kiro-krew
```

### Unit Tests
```bash
# Run focus state tests
go test -v ./internal/tui -run TestFocusTarget

# Run all TUI tests
go test -v ./internal/tui
```

### Manual Testing
Follow the manual testing procedure in Task 7 to verify all acceptance criteria.

### Integration Verification
```bash
# Start kiro-krew and verify focus behavior
./kiro-krew

# In the REPL:
# 1. Type "plan test" and press Enter to create planning tab
# 2. Press Enter to focus message input
# 3. Press [ to switch to main tab
# 4. Verify footer has focus (you can type immediately)
# 5. Press ] to switch back to planning tab
# 6. Verify message input has focus (cursor in message area)
```

## Implementation Notes

### Backward Compatibility
- Existing `RestoreFocus()` method in `PlanningTab` is retained and delegates to new interface
- No changes to public APIs outside of Tab interface extension
- Existing tab behavior preserved (viewport scrolling, Esc key handling)

### Performance Considerations
- Focus state map is lightweight (string keys, enum values)
- No additional allocations during tab switching
- Mouse click detection uses simple boundary checks

### Edge Cases Handled
- **New tabs without previous state**: Default to `FocusTargetFooter`
- **Invalid focus targets**: Restore method defaults to no focus
- **Tab closure**: Focus state is preserved but becomes stale (harmless)
- **Multiple rapid tab switches**: Each switch correctly captures/restores state

### Logging Strategy
Add debug logging for focus state transitions:
- Capture: Log tab ID and captured focus target
- Restore: Log tab ID and restored focus target
- Mouse clicks: Log focus transfer events

### Future Enhancements (Out of Scope)
- Focus state persistence across kiro-krew restarts
- Keyboard shortcuts for explicit focus transfer (beyond Esc key)
- Focus indicators in tab titles (e.g., "[F]" suffix for footer focus)

## Constraints Verification

✅ **Maintains backward compatibility**: Existing code continues to work, new interface methods added  
✅ **Transparent focus restoration**: Users see seamless focus preservation  
✅ **No performance degradation**: Minimal overhead (map lookup, enum comparison)  
✅ **Complete issue resolution**: All acceptance criteria addressed in single PR

## References

- **GitHub Issue**: #256
- **switchActiveTab()**: `internal/tui/tui.go` lines 1103-1125
- **Tab Interface**: `internal/tui/tabs.go`
- **Planning Tab Focus**: `internal/tui/planning_tab.go` (`focusInput`, `RestoreFocus()`)
- **Tab Implementations**: `main_tab.go`, `agent_tab.go`, `log_tab.go`
- **Focus Transfer Message**: `internal/tui/planning_tab.go` line 153 (`focusTransferMsg`)

---

**End of Specification**
