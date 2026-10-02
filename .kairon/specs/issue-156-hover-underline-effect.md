# Design Specification: Add Hover Underline Effect for Inactive TUI Tabs

**Issue:** #156  
**Title:** Add hover underline effect for inactive tabs in TUI  
**Closes:** #156

## Solution Approach

Implement mouse motion tracking for TUI tab headers to provide visual hover feedback on inactive tabs. This enhances user experience by clearly indicating clickable tabs without interfering with existing functionality.

**Key Architectural Decisions:**
1. Enable global mouse motion events via `tea.WithMouseAllMotion()` program option
2. Add hover state tracking to `TabManager` for inactive tabs only
3. Extend tab styling system to support hover effects via lipgloss underline
4. Maintain existing active tab styling precedence over hover states

## Relevant Files

### Files to Modify:
- `internal/tui/tab_manager.go` - Add hover state tracking and update rendering logic
- `internal/tui/tui.go` - Enable mouse motion events and handle `tea.MouseMotionMsg`
- `internal/tui/styles.go` - Add hover styling for inactive tabs

### Reference Files:
- Existing mouse handling patterns in `tui.go` (`tea.MouseClickMsg`, `tea.MouseWheelMsg`)
- Current tab click detection in `TabManager.HandleTabHeaderClick()`
- Tab styling in `Styles` struct with `TabInactive` style

## Team Orchestration

**Builder Agent:** Implement changes sequentially across the three files, ensuring each modification preserves existing functionality while adding hover capabilities.

**Validator Agent:** Verify that hover effects work correctly, active tab styling is unaffected, and mouse click functionality remains intact.

## Step-by-Step Task Breakdown

### Task 1: Add Hover State Tracking to TabManager
**Acceptance Criteria:**
- Add `hoveredTab` field to `TabManager` struct to track which tab (by index) is currently hovered
- Initialize hover state to -1 (no tab hovered) in `NewTabManager()`
- Add `SetHoveredTab(index int)` method to update hover state
- Add `GetHoveredTab()` method to retrieve current hovered tab index
- Add `ClearHover()` method to reset hover state

### Task 2: Enable Mouse Motion Events in TUI
**Acceptance Criteria:**
- Add `tea.WithMouseAllMotion()` to program options in `Run()` function
- Add `tea.MouseMotionMsg` case to `Update()` method in `tui.go`
- Handle mouse motion events to detect hover over tab headers
- Only process hover when `mouse.Y < tabHeaderHeight` (within tab area)
- Call appropriate `TabManager` hover methods based on mouse position

### Task 3: Extend Tab Rendering with Hover Effects
**Acceptance Criteria:**
- Modify `RenderTabHeaders()` method to apply hover styling to inactive tabs
- Add hover underline effect using lipgloss `Underline(true)` when tab is hovered
- Ensure active tab styling takes precedence (no hover effect on active tab)
- Maintain existing tab padding, close button, and separator rendering

### Task 4: Add Hover Styling Support
**Acceptance Criteria:**
- Create `TabInactiveHover` style in `Styles` struct extending `TabInactive` with underline
- Update `NewStyles()` function to initialize hover style with theme colors
- Ensure hover style maintains existing inactive tab colors while adding underline decoration

### Task 5: Implement Hover Position Calculation
**Acceptance Criteria:**
- Create helper method in `TabManager` to calculate which tab is under mouse cursor
- Reuse existing position calculation logic from `HandleTabHeaderClick()` method
- Account for tab padding, separators, and close button positioning
- Handle edge cases (mouse outside tab area, no tabs, terminal width constraints)

## Validation Commands

```bash
# Build and test the application
go build ./cmd/kiro-krew
./kiro-krew

# Manual testing steps:
# 1. Start application and verify multiple tabs are visible
# 2. Hover mouse over inactive tabs - should see underline effect
# 3. Hover over active tab - should NOT see underline effect  
# 4. Click on tabs to ensure existing functionality works
# 5. Verify hover effect clears when mouse moves away
# 6. Test with terminal resizing and tab overflow scenarios
```

## Implementation Notes

**Mouse Event Flow:**
1. `tea.MouseMotionMsg` received in `tui.Update()`
2. Check if `mouse.Y < tabHeaderHeight` (within tab header area)
3. Calculate which tab is under cursor using position logic
4. Update `TabManager` hover state via `SetHoveredTab()` or `ClearHover()`
5. Re-render tab headers with hover styling applied

**Performance Considerations:**
- Mouse motion events are frequent; keep processing lightweight
- Only update hover state when it actually changes to minimize re-renders
- Reuse existing tab position calculation logic to avoid duplication

**Styling Hierarchy:**
1. Active tab: Uses `TabActive` style (no hover effect)
2. Inactive tab hovered: Uses `TabInactiveHover` style (with underline)
3. Inactive tab normal: Uses `TabInactive` style (no decoration)

**Error Handling:**
- Gracefully handle invalid tab indices in hover methods
- Ensure hover state is properly cleared when tabs are closed/removed
- Handle edge cases with empty tab list or single tab scenarios