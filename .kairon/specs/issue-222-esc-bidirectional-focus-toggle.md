# Design Specification: Esc Bidirectional Focus Toggle in Planning Tabs

**Issue**: #222 - feat(tui): use Esc as bidirectional focus toggle in planning tabs  
**Closes**: #222

## Problem Statement

The Tab key currently serves conflicting roles in the TUI:
- **In main tab**: Tab-completion of commands (autocomplete)
- **In planning tab**: Toggles focus between message input and footer input
- This creates routing complexity and prevents tab-completion from working when a planning tab is active

## Solution Approach

Replace the Tab key focus toggle with **Esc** as a context-sensitive bidirectional focus switch, following a priority-based layered dismissal system:

1. **Message input focused** → Esc moves focus to footer command input
2. **Footer focused (on a planning tab)** → Esc moves focus to message input  
3. **Autocomplete dropdown visible** → Esc dismisses dropdown first (existing behavior takes priority)
4. **Overlay active** → Esc dismisses overlay (existing behavior takes priority)

This maintains existing behaviors while resolving the Tab key conflict and enabling consistent autocomplete functionality across all tab types.

## Relevant Files

### Files to Modify

1. **`internal/tui/planning_tab.go`**
   - Remove Tab key handling logic (lines 617-629)
   - Add Esc key handling for focus transfer from message input to footer
   - Implement priority checking for autocomplete dropdown visibility

2. **`internal/tui/tui.go`**
   - Extend existing Esc key routing logic to handle planning tab focus transfers
   - Add context-aware routing for Esc when planning tab is active
   - Ensure overlay dismissal takes highest priority

3. **`internal/tui/autocomplete.go`** (Reference only - no changes needed)
   - Current Esc behavior for dropdown dismissal is correct
   - Should continue to dismiss dropdown when visible

### Files for Reference/Context

- **`internal/tui/footer.go`** - Footer focus management  
- **`internal/tui/tab_manager.go`** - Tab switching logic
- **`internal/tui/styles.go`** - Visual styling (no changes expected)

## Team Orchestration

The implementation follows a layered priority system where Esc key handling cascades through different contexts:

1. **Overlay Layer** (highest priority) - handled in `tui.go`
2. **Autocomplete Layer** - handled in `autocomplete.go` 
3. **Focus Transfer Layer** - coordinated between `tui.go` and `planning_tab.go`

### Task Dependencies

- **Task 1** and **Task 2** can run in parallel (independent file modifications)
- **Task 3** depends on Task 1 and Task 2 (integration testing requires both changes)
- **Task 4** depends on Task 3 (validation requires working implementation)

## Step-by-Step Task Breakdown

### Task 1: Remove Tab Key Handling from Planning Tab
**Acceptance Criteria**:
- Remove Tab key case block from `planning_tab.go` Update method (lines 617-629)
- Ensure no focus transfer logic remains tied to Tab key
- Tab key events now pass through to parent for autocomplete handling

**Implementation Location**: `internal/tui/planning_tab.go`  
**Dependencies**: None

### Task 2: Implement Esc Focus Transfer in Planning Tab  
**Acceptance Criteria**:
- Add Esc key handling in planning tab Update method
- When message input has focus and no autocomplete dropdown is visible, Esc shifts focus to footer
- Send `focusTransferMsg{target: "footer"}` to coordinate with parent
- Preserve existing behavior when autocomplete dropdown is visible

**Implementation Location**: `internal/tui/planning_tab.go`  
**Dependencies**: None (can run parallel with Task 1)

### Task 3: Extend Esc Routing in Main TUI Controller
**Acceptance Criteria**:
- Extend existing Esc key routing logic in `tui.go` 
- Add planning tab context detection for footer-to-message focus transfers
- When footer has focus on planning tab and Esc is pressed, shift focus to message input
- Maintain priority: overlay dismissal → autocomplete dismissal → focus transfer
- Ensure all existing Esc behaviors (overlay dismissal) remain unchanged

**Implementation Location**: `internal/tui/tui.go`  
**Dependencies**: Task 1, Task 2 (requires focus transfer message handling)

### Task 4: Integration Testing and Validation
**Acceptance Criteria**:
- Tab key works for autocomplete in footer when on main tab
- Tab key works for autocomplete in footer when on planning tab  
- Esc dismisses autocomplete dropdown when visible (no focus change)
- Esc dismisses overlays when active (no focus change)
- Esc toggles focus bidirectionally in planning tabs when no dropdown/overlay
- All existing keyboard shortcuts and behaviors remain functional

**Implementation Location**: Manual testing and verification  
**Dependencies**: Task 1, Task 2, Task 3

## Validation Commands

Run these commands to verify the implementation works correctly:

```bash
# Build the application
go build ./cmd/kiro-krew

# Start the TUI for manual testing
./kiro-krew

# Test scenarios to verify:
# 1. In main tab: Tab key should trigger autocomplete
# 2. Create planning tab: plan "test session"  
# 3. In planning tab message input: Tab key should trigger autocomplete
# 4. In planning tab footer: Tab key should trigger autocomplete
# 5. In planning tab: Esc should toggle focus between message and footer
# 6. With autocomplete dropdown visible: Esc should dismiss dropdown first
# 7. With overlay active (status/help): Esc should dismiss overlay
```

### Manual Test Matrix

| Context | Dropdown | Overlay | Esc Behavior | Expected Result |
|---------|----------|---------|--------------|-----------------|
| Main tab | No | No | N/A | No focus transfer |
| Main tab | Yes | No | Dismiss dropdown | Dropdown closes |
| Planning message | No | No | Focus transfer | Focus → footer |
| Planning footer | No | No | Focus transfer | Focus → message |
| Planning message | Yes | No | Dismiss dropdown | Dropdown closes |
| Planning footer | Yes | No | Dismiss dropdown | Dropdown closes |
| Any context | Any | Yes | Dismiss overlay | Overlay closes |

## Implementation Notes

### Priority System Implementation
The Esc key handling follows this exact order:
1. **`tui.go`**: Check for active overlay → dismiss if present, stop processing
2. **`tui.go`**: Route to active tab (includes autocomplete check)
3. **`autocomplete.go`**: Check for visible dropdown → dismiss if present, stop processing  
4. **`tui.go`** + **`planning_tab.go`**: Handle focus transfers

### Focus State Management
- Footer focus state is managed by `AutocompleteInput.SetFocus()`
- Message input focus state is managed by `PlanningTab.focusInput` boolean
- Focus coordination uses existing `focusTransferMsg` message system

### Backward Compatibility
- All existing keyboard shortcuts remain functional
- Overlay dismissal behavior unchanged  
- Autocomplete behavior unchanged
- Only the Tab key conflict in planning tabs is resolved

## Benefits Achieved

1. **Unified Tab Behavior**: Tab key consistently triggers autocomplete across all tab types
2. **Intuitive Esc Logic**: Single key for "escape from current context" 
3. **Layered Priority**: Clear hierarchy for dismissing overlays, dropdowns, then focus transfer
4. **No MacOS Conflicts**: Avoids modifier+Tab combinations claimed by system
5. **Preserved Functionality**: All existing behaviors remain intact
6. **Reduced Complexity**: Eliminates Tab key routing complexity between tabs

The implementation maintains the mental model of Esc as "escape current context" while enabling consistent Tab-based autocomplete functionality across the entire TUI.