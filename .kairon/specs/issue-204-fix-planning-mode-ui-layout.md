# Design Specification: Fix Planning Mode UI Layout - Tab Headers Misaligned When Planning Tab Active

**Issue**: #204  
**Repository**: jbrinkman/kiro-krew  
**Closes**: #204

## Solution Approach

The planning tab is violating the two-row footer structure established in Issue #195 by conditionally rendering status information at the top of its content area when terminal width is >100 characters. This causes status elements to appear in the tab headers area instead of strictly in Row 2 (Status Row) at the bottom of the screen.

The solution involves:
1. **Remove conditional status bar** from planning tab's View() method entirely
2. **Ensure FooterManager handles planning context** correctly for all terminal widths
3. **Validate status information only appears in Row 2** (Status Row) as defined in Issue #195 specification

## Root Cause Analysis

The issue is located in `internal/tui/planning_tab.go` in the `View()` method, specifically around lines 530-550:

```go
// Add status bar for wide terminals
var statusBar string
if pt.width > 100 && pt.contextTracker != nil {
    used, total := pt.contextTracker.GetUsage()
    model := pt.contextTracker.GetCurrentModel()

    statusInfo := fmt.Sprintf("Context: %d/%d", used, total)
    if model != "" {
        statusInfo += fmt.Sprintf(" | Model: %s", model)
    }
    statusInfo += fmt.Sprintf(" | State: %s", pt.getStateDisplay())

    statusBar = pt.styles.PlanningTimestamp.
        Width(pt.width).
        AlignHorizontal(lipgloss.Right).
        Render(statusInfo)
}

// Combine all parts with responsive layout
if statusBar != "" {
    return lipgloss.JoinVertical(
        lipgloss.Left,
        statusBar,        // <- PROBLEM: Status bar rendered at top of content
        messageArea,
        separator,
        inputArea,
    )
}
```

This conditional status rendering violates Issue #195's strict two-row footer layout where:
- **Tab headers**: ONLY tab names and close buttons - no status elements ever
- **Row 2 (Status Row)**: Shows status information for all tabs, with additional context/model/directory info when planning tab is active

## Relevant Files

### Files to Modify:
1. **`internal/tui/planning_tab.go`** - Remove conditional status bar from View() method
2. **`internal/tui/footer.go`** - Ensure FooterManager correctly shows planning context for all terminal widths

### Files for Reference:
- **`internal/tui/tui.go`** - Main TUI coordination and FooterManager usage
- **`internal/tui/tab_manager.go`** - Tab management and header rendering
- **`internal/tui/context_tracker.go`** - Context tracking functionality

## Team Orchestration

This is a focused UI layout fix with clear component boundaries:

- **Planning Tab Component**: Remove erroneous status rendering from content area
- **Footer Manager Component**: Ensure consistent status row rendering regardless of terminal width
- **Main TUI Coordinator**: Maintains existing integration between tab manager and footer manager

**Dependencies**: No dependencies between tasks - all changes are self-contained within their respective components.

## Step-by-Step Task Breakdown

### Task 1: Remove Conditional Status Bar from Planning Tab View Method
**Acceptance Criteria**:
- Remove the conditional status bar logic from `planning_tab.go` View() method (lines ~530-550)
- Remove the conditional return that includes statusBar in JoinVertical layout
- Ensure View() method only returns: messageArea, separator, inputArea (no status bar at top)
- Maintain all existing responsive styling and layout logic for message area and input
- All tab navigation, message handling, and ACP integration functionality remains intact

**Dependencies**: None

### Task 2: Verify FooterManager Shows Planning Context Consistently
**Acceptance Criteria**:
- FooterManager renders planning context (ctx/model/directory) in Row 2 for planning tabs regardless of terminal width
- Remove any width-based conditionals that might hide planning context information
- Ensure `renderPlanningInfo()` method returns context information for all screen sizes
- Theme information appears in Row 2 for all tabs as specified in Issue #195
- Row 2 shows enhanced planning context only when planning tab is active

**Dependencies**: None

### Task 3: Integration Testing and UI Layout Validation
**Acceptance Criteria**:
- Tab headers contain ONLY tab names and close buttons across all terminal widths
- Row 2 (Status Row) shows `theme: {theme}` for all tabs
- Row 2 additionally shows `ctx: {usage} | model: {model} | 📁 {directory}` when planning tab is active
- No status information appears anywhere except Row 2 (Status Row)
- Layout remains consistent across terminal widths (narrow <60, medium 60-100, wide >100)
- All existing planning tab functionality (message sending, streaming, tab switching) works correctly
- Switching between console and planning tabs shows appropriate status information in Row 2

**Dependencies**: Task 1, Task 2

## Validation Commands

### UI Layout Verification:
```bash
# Start kiro-krew and test the UI layout
./kiro-krew

# In the REPL, test planning tab creation:
plan Test planning tab layout

# Verify tab headers show only tab names and close buttons
# Verify Row 2 shows theme + planning context when planning tab is active
# Test with different terminal widths:

# Narrow terminal (< 60 chars)
resize 50 20

# Medium terminal (60-100 chars) 
resize 80 24

# Wide terminal (> 100 chars)
resize 120 30
```

### Functional Regression Testing:
```bash
# Verify all existing functionality works
plan Create a test feature     # Test message sending
# Test tab navigation with [ ] keys
# Test mouse clicking on tabs
# Test tab closing with Ctrl+W
# Test streaming responses
# Test context tracking updates
```

### Code Review Validation:
```bash
# Check that planning_tab.go View() method no longer conditionally renders status
grep -n "statusBar" internal/tui/planning_tab.go
# Should return no results

# Verify footer manager handles planning context consistently
grep -A 10 -B 5 "renderPlanningInfo" internal/tui/footer.go
```

## Implementation Notes

### Critical Requirements:
- **Zero Breaking Changes**: All existing planning tab functionality must remain intact
- **Strict Layout Compliance**: Must adhere exactly to Issue #195's two-row footer specification  
- **Cross-Width Consistency**: Status information behavior must be identical across all terminal widths
- **Component Isolation**: Changes are localized to UI rendering, no changes to business logic

### UI Layout Specification Compliance:
This fix ensures the UI strictly follows Issue #195's layout:
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [Console] [Planning Tab ×] [Agent Tab]                                     │  ← Tab headers ONLY
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Content area... (NO STATUS ELEMENTS HERE)                                 │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ kiro-krew> Type your command here...                                        │  ← Row 1 (Input Row)  
│ theme: dark │ ctx: 45k/200k │ model: claude-sonnet-4 │ 📁 /projects/myapp  │  ← Row 2 (Status Row)
└─────────────────────────────────────────────────────────────────────────────┘
```

### Testing Strategy:
- Manual UI testing across different terminal widths
- Functional regression testing of all planning tab features  
- Visual inspection that tab headers remain clean
- Verification that all status information appears only in Row 2

The implementation is straightforward UI refactoring that removes problematic conditional rendering while maintaining all existing functionality and ensuring consistent status display through the established FooterManager system.