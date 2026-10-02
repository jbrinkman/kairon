# Design Specification: Add Autocomplete Functionality to REPL Commands

**Issue**: #105  
**Title**: Add autocomplete functionality to REPL commands  
**Closes**: #105

## Solution Approach

Enhance the existing REPL with modern autocomplete functionality using a layered approach that extends the current Charm `textinput` component while maintaining compatibility with existing command execution logic.

### Key Architectural Decisions

1. **Custom Autocomplete Component**: Create a wrapper around the existing `textinput.Model` that adds autocomplete functionality
2. **Command Registry**: Implement a centralized command registry with completion metadata
3. **Non-intrusive Integration**: Layer autocomplete on top of existing command parsing without breaking current functionality
4. **Bubble Tea Integration**: Use Bubble Tea's message passing for autocomplete interactions

## Relevant Files

### Files to Create
- `internal/tui/autocomplete.go` - Core autocomplete component and logic
- `internal/tui/command_registry.go` - Command definitions and completion metadata

### Files to Modify
- `internal/tui/tui.go` - Integrate autocomplete component and handle new key events
- `internal/tui/commands.go` - Extract command definitions to registry
- `internal/tui/styles.go` - Add autocomplete-specific styles

### Files for Reference
- `go.mod` - Check Bubble Tea/Bubbles version compatibility
- `internal/tui/output_view.go` - Reference for popup rendering patterns

## Team Orchestration

### Builder Tasks
1. Implement command registry system
2. Create autocomplete component
3. Integrate with existing TUI
4. Add visual styling and interactions
5. Implement error handling and validation

### Validator Tasks
1. Test autocomplete behavior across all supported commands
2. Verify keyboard navigation works correctly
3. Validate ghost text rendering
4. Test error indicators
5. Ensure no regression in existing command execution

## Step-by-Step Task Breakdown

### Task 1: Command Registry Implementation
**Acceptance Criteria:**
- Create `CommandRegistry` struct with command metadata
- Include command names, descriptions, argument patterns
- Support for subcommands (e.g., "watch start", "watch stop")
- Export method to get all commands and filtered matches

### Task 2: Autocomplete Component Core
**Acceptance Criteria:**
- Create `AutocompleteInput` struct wrapping `textinput.Model`
- Implement suggestion filtering based on current input
- Add state management for dropdown visibility and selection
- Support arrow key navigation through suggestions

### Task 3: Ghost Text Prediction
**Acceptance Criteria:**
- Implement ghost text rendering with visual distinction (dimmed style)
- Show best match prediction as user types
- Tab key completes ghost text without executing
- Ghost text updates dynamically with typing

### Task 4: Dropdown Menu Implementation
**Acceptance Criteria:**
- Render dropdown below input with available commands
- Show after first keypress with matching commands
- Use arrow keys (↑/↓) for navigation
- Highlight selected option visually
- Auto-hide when no matches or input is empty

### Task 5: Selection and Execution Logic
**Acceptance Criteria:**
- Enter key selects highlighted option and executes command
- Tab key completes current selection without executing
- Escape key closes dropdown without selection
- Maintain existing command execution flow

### Task 6: Error Handling and Validation
**Acceptance Criteria:**
- Visual indicator for invalid commands (red highlighting)
- Audio indicator (system beep) for invalid command attempts
- Prevent execution of unrecognized commands
- Clear error state when valid input is entered

### Task 7: Integration with Existing TUI
**Acceptance Criteria:**
- Replace existing textinput usage with autocomplete component
- Preserve all existing keyboard shortcuts and behaviors
- Handle mouse wheel and click events appropriately
- Maintain tab switching and overlay functionality

### Task 8: Styling and Theme Support
**Acceptance Criteria:**
- Add autocomplete styles to theme system
- Dropdown styling matches existing overlay patterns
- Ghost text styling is visually distinct but readable
- Error indicators use existing error style patterns

## Command Support Requirements

The autocomplete system must support all current REPL commands:

### Base Commands
- `watch` (with subcommands: `start`, `stop`)
- `status`
- `stop` (with `<issue>` parameter)
- `plan` (with optional `[desc]` parameter)
- `theme` (with optional `<name>` parameter)
- `about`
- `exit`
- `help`
- `logs`

### Command Metadata Structure
```go
type Command struct {
    Name        string
    Description string
    Subcommands []string
    HasArgs     bool
    ArgPattern  string
}
```

## Technical Implementation Details

### Autocomplete State Management
```go
type AutocompleteState struct {
    suggestions     []string
    selectedIndex   int
    showDropdown    bool
    ghostText       string
}
```

### Key Event Handling
- Override key handling in main TUI update loop
- Route autocomplete-specific keys to autocomplete component
- Fall back to existing input handling for other keys

### Rendering Strategy
- Render dropdown as overlay similar to existing popup system
- Position dropdown relative to input field
- Handle terminal resize gracefully

## Validation Commands

### Unit Tests
```bash
go test ./internal/tui -v -run TestAutocomplete
```

### Integration Tests
```bash
# Test basic autocomplete functionality
echo "wa" | kiro-krew # Should show "watch" suggestion

# Test command completion
# Type "w" + Tab, should complete to "watch"

# Test subcommand completion  
# Type "watch " + any key, should show "start" and "stop"

# Test invalid command handling
echo "invalid" | kiro-krew # Should show error indicator

# Test existing functionality preservation
kiro-krew status # Should work exactly as before
```

### Manual Testing Scenarios
1. Start kiro-krew REPL
2. Type single character - verify dropdown appears
3. Use arrow keys to navigate suggestions
4. Press Tab to complete without executing
5. Press Enter to execute selected command
6. Type invalid command - verify error indicators
7. Test ghost text with partial matches
8. Verify existing shortcuts (F2, [, ]) still work
9. Test with different themes
10. Verify mouse interactions don't break

## Performance Considerations

- Command filtering should have O(n) complexity or better
- Dropdown rendering should not cause noticeable lag
- Ghost text updates should be immediate (< 50ms)
- Memory usage should remain bounded (no leaks from suggestion lists)

## Backward Compatibility

- All existing commands must work exactly as before
- All existing keyboard shortcuts must be preserved
- Theme system integration must not break existing themes
- No changes to command execution logic or output formats
