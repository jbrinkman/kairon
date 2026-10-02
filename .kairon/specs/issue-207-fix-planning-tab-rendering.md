# Fix Planning Tab Rendering - Remove Tab-Type-Specific Styling Logic

**Issue:** #207  
**Closes:** #207

## Problem Analysis

The current tab rendering system in `internal/tui/tab_manager.go` (lines 263-307) contains extensive conditional branching based on `tab.Type()`, creating separate rendering paths for different tab types. This violates the design principle that tabs should have uniform basic rendering regardless of their content or type.

### Root Cause

The `RenderTabHeaders` method implements complex tab-type-specific logic:
- Planning tabs use `GetPlanningTabStyle()` with special border styling that causes layout breakage
- Agent tabs have separate status-based coloring logic
- Multiple conditional branches create inconsistent behavior

This architecture incorrectly mixes tab content concerns with basic tab rendering, resulting in planning tabs wrapping across multiple lines ("Plan" → "N" → "×") due to inappropriate styling.

## Solution Approach

**Unified Tab Rendering Architecture**: Refactor the tab rendering system to use consistent basic styles (`TabActive`, `TabInactive`, `TabInactiveHover`) for all tab types, while moving status/state information to appropriate areas (footer, content body) rather than tab headers.

### Core Principles

1. **Uniform Basic Rendering**: All tabs use the same fundamental styling logic
2. **Type-Agnostic Headers**: Tab headers display only title and close button with consistent formatting
3. **Status Separation**: Move status/state information from headers to appropriate display areas
4. **Preserve Functionality**: Maintain all existing tab management features (hover, click, close)

## Relevant Files

### Primary Implementation Files
- **`internal/tui/tab_manager.go`** (lines 263-307) - Remove tab-type-specific branching from `RenderTabHeaders`
- **`internal/tui/styles.go`** - Remove planning-specific tab header styles, keep base tab styles
- **`internal/tui/footer.go`** - Potentially enhanced to display planning tab status in footer

### Secondary Files (for validation/testing)
- **`internal/tui/planning_tab.go`** - Verify tab still functions after style changes
- **`internal/tui/agent_tab.go`** - Verify agent tabs maintain functionality
- **`internal/tui/main_tab.go`** - Ensure main tab remains unaffected
- Test files in `internal/tui/*_test.go` - Update any tests that depend on specific tab styling

## Team Orchestration

This is a focused refactoring that can be implemented as a single coordinated change:

1. **Rendering Logic Simplification** (Core Task) - Remove conditional branching, implement uniform styling
2. **Style Cleanup** (Parallel Task) - Remove unused planning tab header styles
3. **Status Information Relocation** (Dependent Task) - Move planning status display to appropriate areas
4. **Validation & Testing** (Final Task) - Verify all tab types work correctly

Tasks 1 and 2 can run in parallel since they modify different sections. Task 3 depends on Task 1 completion. Task 4 validates the complete solution.

## Step-by-Step Task Breakdown

### Task 1: Simplify Tab Rendering Logic
**File**: `internal/tui/tab_manager.go` (lines 263-307)
**Acceptance Criteria**:
- Replace all tab-type-specific conditional branches with uniform logic
- All tabs use only `TabActive`, `TabInactive`, or `TabInactiveHover` styles
- Remove calls to `GetPlanningTabStyle()` from `RenderTabHeaders` method
- Remove agent status-based coloring from tab headers
- Planning tabs render on single line with format: "Plan N ×"
- Agent tabs use basic styling without status-specific colors
- Main tab remains unchanged (already uses basic styling)
- Tab functionality (hover, click, close) preserved
**Dependencies**: None

### Task 2: Clean Up Planning Tab Header Styles  
**File**: `internal/tui/styles.go`
**Acceptance Criteria**:
- Remove planning-specific tab header styles: `PlanningTabActive`, `PlanningTabInactive`, `PlanningTabHover`, `PlanningTabProcessing`, `PlanningTabCompleted`, `PlanningTabFailed`, `PlanningTabReadOnly`
- Remove `GetPlanningTabStyle()` method since it's no longer used
- Keep basic tab styles: `TabActive`, `TabInactive`, `TabInactiveHover`, `TabClose`
- Keep all planning content styles (borders, messages, etc.) - only remove header styles
**Dependencies**: None (can run parallel with Task 1)

### Task 3: Relocate Planning Status Display
**File**: `internal/tui/footer.go` or `internal/tui/planning_tab.go`
**Acceptance Criteria**:
- Planning tab status/state information displayed in footer status area or tab content area
- Status information no longer affects tab header appearance
- All planning states (active, completed, failed, read-only) clearly indicated outside of tab headers
- Footer displays appropriate status for active planning tab
**Dependencies**: Task 1 (must complete rendering logic changes first)

### Task 4: Validation and Testing
**Files**: Test files and manual verification
**Acceptance Criteria**:
- All tab types render with consistent header formatting
- Planning tabs display properly on single line: "Plan N ×"
- Agent tabs work without special status coloring in headers
- Main tab unaffected (non-closable, always visible)
- All tab interactions work (hover effects, clicking, closing)
- Planning status visible in appropriate areas (footer/content)
- No layout wrapping or rendering issues
- Existing tests pass or are updated appropriately
**Dependencies**: Tasks 1, 2, 3

## Validation Commands

```bash
# Build and run to verify basic functionality
go build ./cmd/kiro-krew
./kiro-krew

# Run existing tests to ensure no regressions
go test ./internal/tui/... -v

# Run integration tests for tab management
go test ./internal/tui/ -run "TestTab" -v

# Specific tests for planning tab functionality
go test ./internal/tui/ -run "TestPlanning" -v

# Manual verification:
# 1. Create multiple planning tabs and verify single-line rendering
# 2. Switch between tab types and verify consistent styling
# 3. Test tab hover and click functionality
# 4. Verify planning status appears in footer/content area
# 5. Confirm agent tabs work without status-based header coloring
```

## Implementation Notes

### Current Problematic Code Pattern
```go
// REMOVE THIS PATTERN:
if tab.Type() == TabTypePlanning {
    if planningTab, ok := tab.(*PlanningTab); ok {
        styledTab = styles.GetPlanningTabStyle(planningTab.GetState(), true, false).Render(title)
    }
} else {
    styledTab = styles.TabActive.Render(title)
}
```

### Target Simplified Pattern
```go
// REPLACE WITH THIS PATTERN:
if i == tm.activeTab {
    styledTab = styles.TabActive.Render(title)
} else if i == tm.hoveredTab {
    styledTab = styles.TabInactiveHover.Render(title)  
} else {
    styledTab = styles.TabInactive.Render(title)
}
```

### Status Information Alternatives
Since tab headers will no longer show status/state information:
1. **Footer Status Row**: Display planning tab state in existing footer status area
2. **Tab Content Headers**: Show status within tab content area
3. **Visual Indicators**: Use subtle indicators in tab content rather than headers

The goal is consistent, clean tab headers with status information displayed in more appropriate locations.

## Constraints & Risks

### Must Preserve
- Main tab non-closable behavior
- All existing tab management functionality
- Planning tab state tracking (move display, don't remove functionality)
- Agent status information (move display location)

### Potential Risks
- Planning status information may be less visible if moved to footer
- Users accustomed to colored tab headers may need adjustment period
- Need to ensure new status display locations are intuitive

### Mitigation
- Thoroughly test all tab interactions after changes
- Ensure status information remains clearly visible in new locations
- Consider user feedback on new status display approach