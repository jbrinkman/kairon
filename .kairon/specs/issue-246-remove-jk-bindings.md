# Design Specification: Remove j/k Key Bindings from Planning Tab

**Issue:** #246 - Remove j/k key bindings from planning tab to allow typing these characters
**Closes:** #246

## Problem Statement

Users cannot type 'j' or 'k' characters in the planning tab text input field because these keys are bound as vim-style navigation shortcuts. When users attempt to type these common characters (e.g., "project", "keyboard", "just"), the keys are intercepted by the keyboard handler's case statements before they can reach the text input component, causing the viewport to scroll instead of inserting the characters.

This creates a significant usability problem for planning tab input, as 'j' and 'k' are frequently used letters in English text. While vim-style navigation is convenient, it conflicts with the primary function of the planning tab's text input field.

## Root Cause Analysis

Investigation of `internal/tui/planning_tab.go` reveals the problematic code in the `Update` method's keyboard handling logic:

**Lines 621-627 (Problematic Code):**
```go
case "up", "k":
    if !pt.focusInput {
        pt.viewport.ScrollUp(1)
    }
case "down", "j":
    if !pt.focusInput {
        pt.viewport.ScrollDown(1)
    }
```

**Current Flow:**
1. User types 'j' or 'k' in the text input field
2. Keyboard event reaches `Update` method
3. Switch statement matches `"j"` or `"k"` case statements
4. Code checks `!pt.focusInput` condition
5. If input has focus, nothing happens (j/k are blocked)
6. Character never reaches the `default` case that forwards to textinput
7. Result: Character is silently dropped, viewport doesn't scroll

The issue occurs in two code paths within the `Update` method:
1. **Normal state handling** (lines 621-627): When `pt.state != session.PlanningStateReadOnly`
2. **Read-only state handling** (lines 595-608): When `pt.state == session.PlanningStateReadOnly`

## Solution Approach

Remove the 'j' and 'k' bindings from the case statements while preserving the 'up' and 'down' arrow key navigation. This allows:
- Arrow keys, pgup/pgdown, home/end continue to work for scrolling
- 'j' and 'k' fall through to the default case, forwarding them to textinput
- Users can type these characters naturally in the text input field
- Read-only mode continues to work with arrow-based navigation

This approach:
- **Minimal Change**: Only removes two string literals from case statements
- **No API Changes**: No function signatures or interfaces modified
- **Backward Compatible**: All other keyboard shortcuts remain functional
- **Low Risk**: Self-contained change with clear behavioral impact
- **User-Friendly**: Prioritizes text input over vim-style navigation

## Relevant Files

### Files to Modify
- `internal/tui/planning_tab.go` - Remove 'j' and 'k' from keyboard case statements

### Files Referenced (No Changes)
- `go.mod` - Current bubbletea/v2 dependency version
- `internal/tui/integration_test.go` - Existing TUI test infrastructure

## Team Orchestration

This is a single-file fix that can be implemented independently without coordination with other components. The change is:
- **Low Risk**: Only affects keyboard event routing, no state changes
- **Self-Contained**: No dependencies on other tasks or components
- **Backward Compatible**: Removes vim-style shortcuts but preserves arrow key navigation
- **Testable**: Can be validated manually and with existing test infrastructure

## Step-by-Step Task Breakdown

### Task 1: Remove j/k Bindings from Normal State Handler
**Acceptance Criteria:**
- Modify case statement at lines 621-627 to remove `"k"` and `"j"` strings
- Keep `"up"` and `"down"` arrow key bindings
- Ensure case statements maintain correct syntax (proper comma placement)
- No changes to conditional logic or behavior within case blocks

**Dependencies:** None

**Implementation Details:**
```go
// Before:
case "up", "k":
    if !pt.focusInput {
        pt.viewport.ScrollUp(1)
    }
case "down", "j":
    if !pt.focusInput {
        pt.viewport.ScrollDown(1)
    }

// After:
case "up":
    if !pt.focusInput {
        pt.viewport.ScrollUp(1)
    }
case "down":
    if !pt.focusInput {
        pt.viewport.ScrollDown(1)
    }
```

### Task 2: Remove j/k Bindings from Read-Only State Handler
**Acceptance Criteria:**
- Modify case statement at lines 595-608 to remove `"k"` and `"j"` strings
- Keep `"up"` and `"down"` arrow key bindings
- Ensure case statements maintain correct syntax (proper comma placement)
- No changes to conditional logic or behavior within case blocks

**Dependencies:** None (can run in parallel with Task 1)

**Implementation Details:**
```go
// Before (read-only mode):
switch msg.String() {
case "up", "k":
    pt.viewport.ScrollUp(1)
case "down", "j":
    pt.viewport.ScrollDown(1)
// ... other cases
}

// After (read-only mode):
switch msg.String() {
case "up":
    pt.viewport.ScrollUp(1)
case "down":
    pt.viewport.ScrollDown(1)
// ... other cases
}
```

### Task 3: Validate Fix with Testing
**Acceptance Criteria:**
- Build application successfully with no compilation errors
- Run existing tests to ensure no regression
- Manually verify 'j' and 'k' characters can be typed in planning tab input
- Verify arrow keys, pgup/pgdown, home/end still work for scrolling
- Confirm no regression in other keyboard shortcuts
- Test both normal and read-only planning states

**Dependencies:** Task 1, Task 2

**Implementation Details:**
- Run `task build` to compile the application
- Run `task test` to execute the test suite
- Manual testing of keyboard input in planning tab
- Manual testing of navigation keys in both states

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

# 2. Press Ctrl+Alt+P to enter planning mode
# 3. Create or open a planning tab

# 4. Test character input (normal state):
#    - Type "just a quick test" - verify 'j' and 'k' appear in input
#    - Type "keyboard project" - verify 'k' characters work
#    - Type "making progress" - verify no characters are blocked

# 5. Test arrow key navigation (normal state):
#    - Press ESC to unfocus input (or use pgup/pgdown/home/end)
#    - Press UP arrow - viewport should scroll up
#    - Press DOWN arrow - viewport should scroll down
#    - Press pgup/pgdown - should scroll by pages
#    - Press home/end - should jump to top/bottom

# 6. Test read-only mode:
#    - Open a completed/failed planning tab (read-only state)
#    - Press UP arrow - viewport should scroll up
#    - Press DOWN arrow - viewport should scroll down
#    - Verify j/k keys do nothing (no input field in read-only mode)

# 7. Test focus behavior:
#    - Return focus to input
#    - Verify typing works normally with j/k characters
#    - Verify arrow keys don't scroll when input has focus
```

### Regression Testing Checklist
- [ ] 'j' and 'k' characters can be typed in planning tab input field
- [ ] Arrow keys (up/down) work for scrolling when input is not focused
- [ ] Arrow keys don't scroll when input has focus (existing behavior)
- [ ] pgup/pgdown/home/end navigation continues to work
- [ ] Read-only state: arrow keys work for scrolling
- [ ] Read-only state: j/k keys have no effect (expected - no input field)
- [ ] Enter key sends messages normally
- [ ] ESC key transfers focus to footer normally
- [ ] Other keyboard shortcuts (esc, enter, etc.) work normally
- [ ] Tab switching and management work normally
- [ ] No compilation errors or test failures

## Implementation Notes

### Code Locations

**Primary Change - Normal State Handler (~lines 621-627):**
```go
// File: internal/tui/planning_tab.go
// Function: Update(msg tea.Msg)
// Context: Keyboard handling when state != PlanningStateReadOnly

case "up":           // Remove "k" from this line
    if !pt.focusInput {
        pt.viewport.ScrollUp(1)
    }
case "down":         // Remove "j" from this line
    if !pt.focusInput {
        pt.viewport.ScrollDown(1)
    }
```

**Secondary Change - Read-Only State Handler (~lines 595-608):**
```go
// File: internal/tui/planning_tab.go
// Function: Update(msg tea.Msg)
// Context: Keyboard handling when state == PlanningStateReadOnly

if pt.state == session.PlanningStateReadOnly {
    switch msg.String() {
    case "up":       // Remove "k" from this line
        pt.viewport.ScrollUp(1)
    case "down":     // Remove "j" from this line
        pt.viewport.ScrollDown(1)
    // ... other cases
    }
    return pt, nil
}
```

### Key Behavior After Fix

| Key | When Input Focused | When Input Not Focused | Read-Only Mode |
|-----|-------------------|------------------------|----------------|
| j   | Types 'j' character | Types 'j' character | No effect |
| k   | Types 'k' character | Types 'k' character | No effect |
| ↑   | No effect | Scrolls viewport up | Scrolls viewport up |
| ↓   | No effect | Scrolls viewport down | Scrolls viewport down |
| pgup | Scrolls + unfocuses input | Scrolls viewport up | Scrolls viewport up |
| pgdown | Scrolls + unfocuses input | Scrolls viewport down | Scrolls viewport down |
| home | Scrolls + unfocuses input | Jumps to top | Jumps to top |
| end | Scrolls + unfocuses input | Jumps to bottom | Jumps to bottom |

### Design Rationale

**Why Remove j/k Instead of Conditional Logic:**
- Planning tab's primary purpose is text input, not document navigation
- vim-style navigation is a power-user feature, not essential functionality
- Arrow keys provide sufficient navigation without blocking text input
- Simpler code is better - removes two string literals vs. adding complex conditionals
- Follows principle of least surprise - 'j' and 'k' should type like other letters

**Alternative Approaches Considered:**
1. **Modal editing (vim-style)**: Too complex, confusing for non-vim users
2. **Modifier keys (Ctrl+j/k)**: Inconsistent with other shortcuts, breaks muscle memory
3. **Context-aware routing**: Over-engineered for a simple text input issue
4. **Toggle between modes**: Adds UI complexity without clear benefit

**Chosen Approach Benefits:**
- Zero learning curve - j/k "just work"
- No new UI elements or state management
- Consistent with user expectations for text input
- Still provides navigation via arrow keys
- Minimal code change, minimal risk

### Risk Assessment
- **Risk Level:** Very Low
- **Blast Radius:** Single component keyboard handling
- **Rollback:** Simple - restore the removed string literals
- **Testing Requirements:** Manual verification + existing automated tests
- **User Impact:** Positive - removes confusing behavior

### Success Criteria
- Users can type 'j' and 'k' characters in planning tab text input without issues
- Arrow keys continue to provide viewport scrolling when input is not focused
- All other keyboard shortcuts remain functional
- No regression in planning tab functionality (message sending, focus handling, etc.)
- Fix works correctly in both normal and read-only planning states
- Behavior is intuitive and matches user expectations

## Related Issues

This fix is similar in nature to issue #227 (textinput "T" character), which also addressed planning tab text input issues. Both fixes share characteristics:
- Single-file changes
- Focus on text input usability
- Minimal code modifications
- Low risk, high user impact
- Similar validation approaches

This demonstrates the planning tab's ongoing evolution toward better text input UX, prioritizing the primary use case (typing messages) over secondary features (vim-style navigation).
