# Fix Universal Input Focus and Remove Persistent UI Separators

**Issue:** #216 - Fix Universal Input Focus and Remove Persistent UI Separators  
**Closes:** #216

## Problem Analysis

### Root Cause 1: Architectural Flaw in Key Event Routing

The key event routing system in `/internal/tui/tui.go` has hardcoded tab-type restrictions that prevent universal input handling:

**Lines 535-543:** Only `TabTypeAgent` tabs receive forwarded key events
```go
if activeTab != nil && activeTab.Type() == TabTypeAgent {
    if cmd := m.tabManager.Update(msg); cmd != nil {
        return m, cmd
    }
}
```

**Lines 546-552:** Only `TabTypeMain` tabs can update input fields
```go
if m.activeOverlay == overlayNone && activeTab != nil && activeTab.Type() == TabTypeMain {
    var cmd tea.Cmd
    m.input, cmd = m.input.Update(msg)
    return m, cmd
}
```

This architectural design assumes different tab types need different input handling, but now that all tabs use the unified footer system, they should all support universal input.

### Root Cause 2: Persistent Separator Rendering

In `/internal/tui/planning_tab.go` at line 481, the `View()` method unconditionally renders a separator:

```go
// Render minimal separator - single line only
separator := strings.Repeat("─", pt.width)

// Combine all parts with minimal clean layout
return lipgloss.JoinVertical(
    lipgloss.Left,
    messageArea,
    separator,    // <-- This creates unwanted separator lines
    inputArea,
)
```

This creates persistent unwanted borders that have been reported and "fixed" multiple times, suggesting previous fixes addressed symptoms rather than the root architectural problem.

### Impact Assessment

1. **Planning Tab Input Failure**: Users cannot type in planning tab inputs or use tab key for focus switching
2. **Inconsistent UX**: Different behaviors across tab types creating confusion
3. **Broken Focus Management**: Tab key doesn't work for focus switching in planning tabs
4. **UI Pollution**: Unwanted separator lines creating visual noise
5. **Technical Debt**: Recurring issues indicate architectural problems

## Solution Approach

### Strategy: Universal Input Architecture

Transform the key event routing from tab-type-specific restrictions to a universal input system where:

1. **All tabs receive key events** regardless of type
2. **Individual tabs decide** how to handle their specific events
3. **Input focus works consistently** across all tab types
4. **Clean UI rendering** without unnecessary separators

### Design Principles

- **Universal Access**: No arbitrary tab-type restrictions on input handling
- **Delegation Pattern**: Forward events to active tab, let tab decide handling
- **Consistent Focus**: Same focus behavior across all tab types
- **Minimal UI**: Remove decorative elements that add visual noise
- **Backward Compatibility**: Maintain all existing functionality

## Relevant Files

### Files to Modify
- `internal/tui/tui.go` - Remove tab-type restrictions in key event routing
- `internal/tui/planning_tab.go` - Remove unconditional separator rendering

### Files to Review
- `internal/tui/main_tab.go` - Ensure compatibility with universal input
- `internal/tui/agent_tab.go` - Ensure compatibility with universal input
- `internal/tui/tabs.go` - Tab interface and type definitions
- `internal/tui/tab_manager.go` - Tab management and event forwarding

## Team Orchestration

### Single Development Cycle
All components will be updated in one coordinated implementation to ensure:
- **Consistent Behavior**: Universal input works across all tab types
- **No Regression**: Existing functionality remains intact
- **Complete Solution**: Both input focus and separator issues resolved

### Task Dependencies
- **Task 1**: Remove key routing restrictions (no dependencies)
- **Task 2**: Clean up separator rendering (no dependencies) 
- **Task 3**: Validation and testing (depends on Tasks 1 & 2)

Tasks 1 and 2 can be implemented in parallel as they address independent issues.

## Step-by-Step Task Breakdown

### Task 1: Implement Universal Key Event Routing
**File**: `internal/tui/tui.go`
**Acceptance Criteria**:
- Remove `TabTypeAgent` restriction in key event forwarding (lines 535-543)
- Remove `TabTypeMain` restriction in input handling (lines 546-552)
- Forward ALL key events to active tab regardless of type
- Let individual tabs decide how to handle their specific events
- Maintain backward compatibility with existing tab functionality

**Dependencies**: None

### Task 2: Remove Persistent Separator Rendering
**File**: `internal/tui/planning_tab.go`
**Acceptance Criteria**:
- Remove unconditional separator rendering in `View()` method (around line 481)
- Clean up layout to remove unnecessary visual elements
- Maintain clean terminal-style `[planner] >` prompt
- Preserve all other UI functionality and styling

**Dependencies**: None

### Task 3: Integration Testing and Validation
**Files**: All modified files
**Acceptance Criteria**:
- Planning tab input field receives focus and accepts typing
- Tab key toggles focus between viewport scrolling and input typing
- Footer command input works from all tab types regardless of active tab
- No separator lines above planning tab input areas
- No regression in existing main tab and agent tab functionality
- Focus state is visually clear to users
- All tab types process key events correctly

**Dependencies**: Task 1, Task 2

## Validation Commands

### Manual Testing Script
```bash
# Build the application
go build ./cmd/kiro-krew

# Start the application
./kiro-krew

# In the REPL, test universal input:
# 1. Press Ctrl+Alt+P to switch to planning mode
# 2. Verify you can type in the [planner] > prompt
# 3. Verify Tab key switches focus between viewport and input
# 4. Press Ctrl+Alt+P to switch back to console mode  
# 5. Verify footer input still works
# 6. Switch to agent tabs and verify input still works
```

### Automated Testing
```bash
# Run existing tests to ensure no regression
go test ./internal/tui/...

# Run integration tests
go test ./internal/tui/ -run TestPlanningTabInput
go test ./internal/tui/ -run TestUniversalInput
go test ./internal/tui/ -run TestModeSwitching
```

### UI Validation
```bash
# Visual inspection checklist:
# □ No separator lines above planning tab input areas
# □ Clean [planner] > prompt without extra borders
# □ Consistent input behavior across all tab types
# □ Focus switching works with Tab key in planning tabs
# □ Footer command input accessible from all tabs
```

## Technical Implementation Notes

### Key Event Routing Changes
Replace tab-type-specific restrictions with universal forwarding:

```go
// OLD (restrictive):
if activeTab != nil && activeTab.Type() == TabTypeAgent {
    if cmd := m.tabManager.Update(msg); cmd != nil {
        return m, cmd
    }
}

// NEW (universal):
if activeTab != nil {
    if cmd := m.tabManager.Update(msg); cmd != nil {
        return m, cmd
    }
}
```

### Input Handling Changes
Remove main tab restriction on input updates:

```go
// OLD (restrictive):
if m.activeOverlay == overlayNone && activeTab != nil && activeTab.Type() == TabTypeMain {
    var cmd tea.Cmd
    m.input, cmd = m.input.Update(msg)
    return m, cmd
}

// NEW (universal):
if m.activeOverlay == overlayNone {
    // Let active tab handle input, fallback to global input if needed
    if activeTab != nil {
        if cmd := m.tabManager.Update(msg); cmd != nil {
            return m, cmd
        }
    }
    // Handle global input (footer commands) for main tab
    if activeTab != nil && activeTab.Type() == TabTypeMain {
        var cmd tea.Cmd
        m.input, cmd = m.input.Update(msg)
        return m, cmd
    }
}
```

### Separator Removal
Remove unnecessary separator rendering in planning tab:

```go
// OLD (with separator):
return lipgloss.JoinVertical(
    lipgloss.Left,
    messageArea,
    separator,  // <-- Remove this
    inputArea,
)

// NEW (clean):
return lipgloss.JoinVertical(
    lipgloss.Left,
    messageArea,
    inputArea,
)
```

## Risk Assessment

### Low Risk
- Separator removal is purely cosmetic
- Key routing changes are additive (removing restrictions)
- Existing tests will catch regressions

### Mitigation Strategies
- Comprehensive testing on all tab types
- Incremental validation during implementation
- Rollback plan via git if issues arise

## Success Criteria

### Functional Requirements
- [ ] Planning tab input field receives focus and accepts typing
- [ ] Tab key works for focus switching in planning tabs
- [ ] Footer command input works from all tab types
- [ ] Input field focus behaves consistently across all tab types
- [ ] No arbitrary tab-type-based input restrictions

### UI Requirements  
- [ ] No separator lines above planning tab input areas
- [ ] No extra borders or padding around input prompts
- [ ] Clean terminal-style `[planner] >` prompt without decorative elements
- [ ] Consistent minimal styling across all input areas

### Compatibility Requirements
- [ ] No regression in existing main tab functionality
- [ ] No regression in existing agent tab functionality
- [ ] All existing keyboard shortcuts continue to work
- [ ] Focus management remains intuitive for users

This design specification addresses both root causes identified in the issue: the architectural key routing restrictions and the persistent separator rendering. The solution implements a universal input system that treats all tab types equally while removing unnecessary UI elements, resulting in a consistent and clean user experience across the entire application.