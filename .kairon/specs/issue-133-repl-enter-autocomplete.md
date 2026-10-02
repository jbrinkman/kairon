# Issue #133: REPL: Allow Enter to execute selected autocomplete suggestion

**Closes #133**

## Problem Statement

Currently in the REPL autocomplete system, the Enter key executes the partial command that the user has typed instead of applying the currently selected autocomplete suggestion. This creates inconsistent UX where:

- Tab key correctly applies the selected suggestion
- Enter key ignores the selected suggestion and executes partial input
- Users expect Enter to work like Tab for completion

## Solution Approach

Modify the `handleKeyMsg` function in `/internal/tui/autocomplete.go` to make Enter behave consistently with Tab completion when a dropdown is visible and a suggestion is selected.

The fix requires minimal changes to the existing autocomplete logic while preserving all current functionality for other key interactions.

## Architecture Analysis

### Current Flow
1. User types partial command (e.g., "w")
2. Autocomplete shows dropdown with suggestions (e.g., "watch start", "watch stop")  
3. User navigates with Up/Down arrows to select suggestion
4. **Current behavior**: Enter executes "w" (partial input)
5. **Expected behavior**: Enter executes "watch start" (selected suggestion)

### Key Components
- `AutocompleteInput.handleKeyMsg()` - Main key event handler
- `AutocompleteState` - Tracks dropdown visibility and selected index
- Parent TUI - Handles command execution after autocomplete processing

## Relevant Files

### Files to Modify
- `internal/tui/autocomplete.go` - Update Enter key handling in `handleKeyMsg` function

### Files for Reference
- `internal/tui/tui.go` - Parent command execution flow (lines 469-485)
- `internal/tui/autocomplete_integration_test.go` - Existing test coverage
- `internal/tui/command_registry.go` - Available commands for testing

## Team Orchestration

This is a single-component fix that requires:
1. **Builder**: Implement the minimal Enter key fix in autocomplete.go
2. **Validator**: Verify the fix works correctly and doesn't break existing behavior

No coordination with external systems or complex dependencies required.

## Step-by-Step Task Breakdown

### Task 1: Implement Enter Key Fix
**Acceptance Criteria:**
- Modify `handleKeyMsg` function in `autocomplete.go`
- When Enter is pressed AND dropdown is visible AND a suggestion is selected:
  - Apply the selected suggestion (same logic as Tab key)
  - Set cursor to end of completed text
  - Update autocomplete state
  - Return without passing through to parent
- When Enter is pressed with no dropdown/selection:
  - Preserve existing behavior (pass through to parent for command execution)

**Technical Details:**
- Add condition check before existing Enter handling
- Reuse existing Tab completion logic for consistency
- Ensure proper state updates after completion

### Task 2: Verify Backward Compatibility
**Acceptance Criteria:**
- Tab key continues to work exactly as before
- Up/Down arrow navigation works exactly as before  
- ESC key continues to hide dropdown as before
- Enter with no dropdown executes command as before
- All existing autocomplete tests pass

### Task 3: Test Edge Cases
**Acceptance Criteria:**
- Enter works when dropdown has 1 suggestion
- Enter works when dropdown has multiple suggestions
- Enter works after using arrow keys to change selection
- Enter behavior is correct when ghost text is present
- Enter behavior is correct when no ghost text is present

## Validation Commands

```bash
# Run existing autocomplete tests
go test -v ./internal/tui -run TestAutocomplete

# Run integration tests
go test -v ./internal/tui -run TestAutocompleteIntegration

# Build and manual testing
go build ./cmd/kiro-krew
./kiro-krew
# In REPL: type "w", use arrows to select, press Enter - should complete to selected suggestion
# In REPL: type "invalid", press Enter - should execute partial command as before
```

## Implementation Notes

### Minimal Code Changes Required
The fix requires adding approximately 8-10 lines of code to the existing `handleKeyMsg` function's Enter case, reusing the exact same logic already proven to work for Tab completion.

### Risk Assessment
- **Low Risk**: Change is isolated to single function  
- **Low Complexity**: Reuses existing, tested Tab logic
- **High Testability**: Existing comprehensive test suite covers this scenario
- **No Breaking Changes**: Preserves all existing behavior

### Performance Impact  
None - the change adds a simple condition check with no additional computational overhead.