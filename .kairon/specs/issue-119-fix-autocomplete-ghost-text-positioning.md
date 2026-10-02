# Fix Autocomplete Ghost Text Positioning and Enable Overwrite Mode

**Issue:** #119  
**Closes:** #119

## Problem Statement

The TUI autocomplete feature has two critical UX issues:

1. **Ghost text positioning**: Ghost text appears after the cursor instead of underneath/overlaying it
2. **Insert vs overwrite mode**: Typing operates in insert mode rather than overwrite mode when ghost text is present

The current implementation in `./internal/tui/autocomplete.go` (lines 159-163) appends ghost text after the base textinput view, causing it to appear adjacent to the cursor rather than overlaying the cursor position.

## Solution Approach

The core issue is architectural: the current approach concatenates ghost text after the textinput view, but what's needed is overlaying ghost text at the cursor position within the textinput itself. This requires modifying the textinput's internal value temporarily during rendering while preserving the actual typed value for editing operations.

### Technical Strategy

1. **Cursor position overlay**: Modify the View() method to manipulate the textinput's display value to include ghost text at cursor position
2. **Overwrite mode simulation**: Implement character-by-character typing that replaces ghost text characters instead of inserting
3. **State preservation**: Maintain separation between actual typed value and display value with ghost text

## Relevant Files

### Files to Modify

- `./internal/tui/autocomplete.go` - Primary implementation file
  - `View()` method (lines 159-163): Core ghost text rendering logic
  - `handleKeyMsg()` method: Add overwrite mode for regular typing
  - `updateGhostText()` method: May need adjustment for cursor positioning

### Files for Reference

- `./internal/tui/tui.go` - Integration context, understand rendering flow
- `./internal/tui/autocomplete_integration_test.go` - Test behavior expectations
- `./internal/tui/styles.go` - Ghost text styling configuration

## Team Orchestration

This is a focused UI/UX fix within a single component. No cross-team coordination required.

**Dependencies**: None - self-contained within autocomplete component  
**Impact**: Affects only autocomplete ghost text behavior, preserves all existing functionality

## Step-by-Step Task Breakdown

### Task 1: Implement Cursor Position Ghost Text Rendering
**Acceptance Criteria:**
- Ghost text appears at cursor position, not after typed text
- Cursor remains visually positioned at boundary between typed and ghost text
- Existing tab completion and dropdown functionality unaffected

**Implementation Details:**
- Modify `View()` method in autocomplete.go
- Instead of concatenating ghost text after base view, temporarily set textinput value to include ghost text
- Use textinput styling to differentiate ghost vs typed text
- Restore original value after rendering to preserve editing state

### Task 2: Implement Overwrite Mode for Ghost Text
**Acceptance Criteria:**  
- Typing regular characters over ghost text replaces those characters
- Backspace and cursor movement work normally
- Tab completion continues to work for full completion

**Implementation Details:**
- Modify `handleKeyMsg()` method to detect regular character input
- When ghost text is present and cursor is at boundary, implement overwrite logic
- Update textinput value by replacing ghost text characters rather than inserting
- Ensure cursor advances correctly after each character

### Task 3: Update Ghost Text State Management
**Acceptance Criteria:**
- Ghost text updates correctly as user types in overwrite mode
- Dropdown selections continue to work properly
- State remains consistent between different input modes

**Implementation Details:**
- Adjust `updateGhostText()` if needed for cursor-based positioning
- Ensure ghost text remains synchronized with current selection
- Verify state transitions between insert mode (no ghost) and overwrite mode (with ghost)

### Task 4: Preserve Existing Functionality
**Acceptance Criteria:**
- Tab key still completes full suggestion
- Arrow keys navigate dropdown suggestions  
- Escape key clears ghost text and dropdown
- Enter key executes command without interference

**Implementation Details:**
- Verify all existing key handlers remain functional
- Test dropdown navigation with new ghost text positioning
- Ensure no regressions in command execution flow

## Validation Commands

```bash
# Build and test the application
go build ./cmd/kiro-krew
./kiro-krew

# In the TUI, test autocomplete behavior:
# 1. Type 'w' - ghost text should appear at cursor, not after
# 2. Type additional characters - should overwrite ghost text
# 3. Press Tab - should complete full suggestion  
# 4. Use arrow keys - should navigate dropdown
# 5. Press Escape - should clear ghost text

# Run existing tests to ensure no regressions
go test ./internal/tui/...
go test ./internal/tui/ -run TestAutocompleteIntegration -v

# Run specific autocomplete tests
go test ./internal/tui/ -run TestAutocompleteInput -v
```

## Implementation Notes

### Critical Constraints

- **Preserve textinput API**: Don't modify the underlying textinput.Model API
- **Maintain state separation**: Keep actual typed value separate from display value  
- **No performance impact**: Ghost text rendering should not affect input responsiveness
- **Style consistency**: Use existing AutocompleteGhost style from theme

### Edge Cases to Handle

- Empty input (no ghost text)
- Ghost text shorter than or equal to typed text
- Rapid typing that outpaces ghost text updates
- Switching between suggestions while typing

### Testing Strategy

- Verify existing integration tests continue to pass
- Test cursor positioning visually in TUI
- Test overwrite mode with various input patterns
- Verify no regressions in tab completion and dropdown navigation
