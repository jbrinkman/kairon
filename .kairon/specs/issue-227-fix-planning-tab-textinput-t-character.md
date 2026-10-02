# Design Specification: Fix Planning Tab Textinput "T" Character on Initialization

**Issue:** #227 - Fix planning tab textinput showing "T" character on initialization
**Closes:** #227

## Problem Statement

When creating a new planning tab in the Kiro Krew TUI, the message input box displays a spurious "T" character at the cursor position after the `[planner] >` prompt, instead of appearing empty as expected. This UI artifact appears consistently across different builds and tab creation scenarios, indicating it's a specific default value in the textinput library component rather than random state or session restoration issues.

## Root Cause Analysis

Investigation reveals that the `textinput.New()` call in the `NewPlanningTabWithSession` function (lines 97-106 of `internal/tui/planning_tab.go`) creates a textinput component that contains a default "T" character. While the prompt display is handled separately in the rendering logic, the textinput value itself is never explicitly cleared during initialization.

The issue is specific to planning tabs because:
1. The `textinput.New()` call doesn't explicitly clear the initial value
2. Other textinput usages in the codebase (like autocomplete.go) set prompts but may not exhibit this issue due to different initialization order or usage patterns
3. The bubbles/v2 textinput component version 2.1.0 appears to have this default state behavior

## Solution Approach

The fix is straightforward and low-risk: add an explicit `ti.SetValue("")` call immediately after creating the textinput component in `NewPlanningTabWithSession` function to ensure the input starts with a completely clean empty state.

This approach:
- Directly addresses the root cause by explicitly clearing any default value
- Follows existing patterns seen in the autocomplete component which uses `SetValue` for state management
- Is minimal and focused, affecting only the initialization sequence
- Preserves all existing functionality including message sending, focus handling, and session restoration

## Relevant Files

### Files to Modify
- `internal/tui/planning_tab.go` - Add explicit value clearing in textinput initialization

### Files Referenced (No Changes)
- `internal/tui/autocomplete.go` - Reference for SetValue usage patterns
- `go.mod` - Current bubbles/v2 dependency version (2.1.0)
- `internal/tui/integration_test.go` - Existing planning tab test infrastructure

## Team Orchestration

This is a single-file fix that can be implemented independently without coordination with other components. The change is:
- **Low Risk**: Only affects textinput initialization, no API changes
- **Self-Contained**: No dependencies on other tasks or components
- **Backward Compatible**: No changes to public interfaces or behavior
- **Testable**: Can be validated with existing test infrastructure

## Step-by-Step Task Breakdown

### Task 1: Implement Textinput Value Clearing
**Acceptance Criteria:**
- Add `ti.SetValue("")` call after `textinput.New()` in `NewPlanningTabWithSession` function
- Ensure the call is placed after all textinput configuration but before the component is used
- Maintain existing focus behavior and configuration

**Dependencies:** None

**Implementation Details:**
- Locate the textinput initialization block (lines 97-106)
- Add the `SetValue("")` call after the `SetStyles()` call but before `Focus()`
- Ensure proper code formatting and maintain existing comment structure

### Task 2: Validate Fix with Testing
**Acceptance Criteria:**
- Build application successfully with no compilation errors
- Run existing tests to ensure no regression
- Manually verify new planning tabs start with empty input (no "T" character)
- Verify cursor positioning remains correct
- Confirm existing functionality (typing, sending messages, focus handling) works normally

**Dependencies:** Task 1

**Implementation Details:**
- Run `task build` to compile the application
- Run `task test` to execute the test suite
- Create test planning tabs manually to verify the fix
- Test various tab creation scenarios (new tabs, restored sessions)

## Validation Commands

### Build and Test Commands
```bash
# Build the application
task build

# Run full test suite
task test

# Development build for quick testing
task dev
```

### Manual Validation Steps
```bash
# 1. Start the application
./kiro-krew

# 2. In the REPL, press Ctrl+Alt+P to enter planning mode
# 3. Create a new planning tab
# 4. Verify the message input shows only "[planner] > " with cursor, no "T" character
# 5. Test typing functionality works normally
# 6. Test message sending works normally
# 7. Create multiple planning tabs to verify consistency
```

### Regression Testing
```bash
# Ensure existing functionality remains intact
# - Message input and sending
# - Focus handling and navigation
# - Session persistence and restoration
# - Tab switching and management
# - Cursor positioning and text editing
```

## Implementation Notes

### Code Location
The fix targets the `NewPlanningTabWithSession` function in `internal/tui/planning_tab.go`, specifically the textinput initialization block around lines 97-106:

```go
// Current code:
ti := textinput.New()
ti.Placeholder = "Type your message here..."
ti.Prompt = ""      // We'll render the prompt ourselves for consistent styling
ti.CharLimit = 4000 // Reasonable message limit

// Configure solid cursor (non-blinking)
currentStyles := ti.Styles()
currentStyles.Cursor.Blink = false
ti.SetStyles(currentStyles)

ti.Focus() // Start focused since focusInput defaults to true

// Add after SetStyles and before Focus:
ti.SetValue("") // Ensure clean empty state
```

### Risk Assessment
- **Risk Level:** Very Low
- **Blast Radius:** Single component initialization
- **Rollback:** Simple - remove the added line
- **Testing Requirements:** Manual verification + existing automated tests

### Success Criteria
- New planning tabs display completely empty message input (no "T" character)
- Cursor appears at the correct position without any preceding characters
- Fix doesn't affect existing functionality like message sending, focus handling, or session restoration
- Behavior is consistent across all tab creation scenarios (new tabs, restored sessions, etc.)

This fix addresses the specific UI bug while maintaining full backward compatibility and following established patterns in the codebase.
