# Issue #113: Fix Autocomplete UX Issues

**Status**: Ready for Implementation  
**Closes**: #113  
**Author**: Architect Agent  
**Created**: 2026-06-18

## Problem Summary

The autocomplete functionality in the kiro-krew REPL has four critical UX issues:

1. **Ghost text spacing**: Extra space appears in ghost text rendering (`w atch` instead of `watch`)
2. **Cursor positioning**: After Tab completion, cursor doesn't move to end of completed text
3. **Missing dropdown menu**: Dropdown exists but isn't being displayed properly in main TUI
4. **Command granularity**: Complex commands like "watch start" should be single autocomplete units

## Solution Approach

### High-Level Strategy

Fix the autocomplete system by addressing each UX issue systematically:

1. **Ghost Text Rendering**: Fix the `View()` method in `AutocompleteInput` to properly render ghost text without extra spacing
2. **Cursor Management**: Update Tab handling in `handleKeyMsg()` to properly position cursor after completion
3. **Dropdown Display**: Fix integration between `ViewDropdown()` and main TUI rendering in `renderBaseView()`
4. **Command Flattening**: Enhance `CommandRegistry` to treat compound commands as atomic units

### Architecture Decisions

- **Minimal Changes**: Focus on fixing existing code rather than major refactoring
- **Preserve Existing API**: Maintain compatibility with current `AutocompleteInput` interface
- **Theme Integration**: Ensure fixes work with existing theme system
- **Performance**: Maintain responsive autocomplete performance

## Relevant Files

### Core Implementation Files
- `internal/tui/autocomplete.go` - Main autocomplete logic (PRIMARY)
- `internal/tui/command_registry.go` - Command definitions and matching
- `internal/tui/tui.go` - TUI integration and rendering
- `internal/tui/styles.go` - Styling definitions (minimal changes)

### Testing Files
- Test files may need creation for autocomplete behavior verification

## Team Orchestration

### Single Developer Workflow
This is a focused UI fix that can be completed by a single developer in sequence:

1. **Analysis Phase**: Understand current autocomplete flow and identify exact failure points
2. **Implementation Phase**: Apply targeted fixes to each component
3. **Testing Phase**: Verify each fix resolves its specific UX issue
4. **Integration Phase**: Ensure all fixes work together harmoniously

### Dependencies
- No external dependencies required
- Changes are isolated to existing autocomplete system
- Integration points are well-defined in current codebase

## Step-by-Step Task Breakdown

### Task 1: Fix Ghost Text Spacing
**File**: `internal/tui/autocomplete.go`  
**Method**: `View()`  
**Issue**: Ghost text includes extra space due to cursor positioning logic

**Acceptance Criteria**:
- Ghost text renders immediately after current input without spaces
- Typing `w` shows `atch start` ghost text with cursor on `a`
- No visual artifacts or text overlap

**Implementation Notes**:
- Current logic: `inputView += a.styles.AutocompleteGhost.Render(ghost)`
- Issue likely in how `ghost` substring is calculated from `a.state.ghostText[len(a.textinput.Value()):]`
- Need to ensure cursor position aligns with ghost text start

### Task 2: Fix Tab Completion Cursor Positioning  
**File**: `internal/tui/autocomplete.go`  
**Method**: `handleKeyMsg()` - Tab case  
**Issue**: Cursor doesn't move to end after Tab completion

**Acceptance Criteria**:
- After Tab completion, cursor moves to end of completed text
- Text doesn't shift unexpectedly during completion
- Subsequent typing continues from cursor position correctly

**Implementation Notes**:
- Current: `a.textinput.SetValue(a.state.ghostText)` or `a.textinput.SetValue(selected)`
- Need to explicitly set cursor position to end after SetValue
- May require additional textinput method calls or position management

### Task 3: Fix Dropdown Menu Display
**File**: `internal/tui/tui.go`  
**Method**: `renderBaseView()`  
**Issue**: Dropdown renders but positioning/display needs improvement

**Acceptance Criteria**:
- Dropdown appears above input line when multiple options exist
- Menu shows after typing first character of any command
- Arrow keys navigate menu and update selection visually
- Menu positioning adapts to available terminal space

**Implementation Notes**:
- Current: dropdown appends to baseView with newline
- Need better positioning logic for dropdown placement
- Consider terminal height constraints and upward vs downward rendering
- Ensure dropdown doesn't interfere with other TUI elements

### Task 4: Enhance Command Registry for Compound Commands
**File**: `internal/tui/command_registry.go`  
**Methods**: `FilterCommands()`, `updateAutocomplete()` in autocomplete.go  
**Issue**: Commands like "watch start" should be treated as single units

**Acceptance Criteria**:
- Typing `w` shows `atch start` as first complete match, not just `atch`
- Menu shows complete commands: `watch start`, `watch stop`, not hierarchical structure
- Autocomplete treats compound commands as atomic units
- Unknown arguments (like issue numbers) still handled separately

**Implementation Notes**:
- Create flattened command list including full compound commands
- Update `FilterCommands()` to return compound commands as single entries
- Modify `updateAutocomplete()` to prioritize complete command matches
- Maintain backward compatibility with existing command structure

### Task 5: Integration Testing and Polish
**Files**: All autocomplete-related files  
**Issue**: Ensure all fixes work together seamlessly

**Acceptance Criteria**:
- All four original UX issues are resolved
- No new UX regressions introduced
- Performance remains responsive
- Theme integration works correctly
- Edge cases handled (empty input, invalid commands, etc.)

**Implementation Notes**:
- Test complete user workflows from typing to completion
- Verify interaction between ghost text, dropdown, and cursor positioning
- Ensure proper cleanup and state management
- Test with different terminal sizes and themes

## Validation Commands

### Unit Testing
```bash
# Run existing tests to ensure no regressions
go test ./internal/tui/...

# Test autocomplete functionality specifically
go test -v ./internal/tui/ -run ".*Autocomplete.*"
```

### Manual Testing Scenarios
```bash
# Start kiro-krew in test mode
go run ./cmd/kiro-krew

# Test ghost text - type 'w' and verify ghost text shows "atch start"
# Test Tab completion - press Tab and verify cursor moves to end
# Test dropdown menu - verify menu appears above input with multiple options
# Test arrow navigation - use up/down arrows to navigate menu
# Test compound commands - verify "watch start" appears as single unit
```

### Specific Test Cases
1. **Ghost Text**: Type `w` → expect `atch start` ghost text, cursor on `a`
2. **Tab Completion**: Type `w`, press Tab → expect `watch start`, cursor at end
3. **Dropdown Menu**: Type `w` → expect menu showing `watch start`, `watch stop`
4. **Arrow Navigation**: Type `w`, press down arrow → expect selection to change
5. **Command Granularity**: Verify compound commands appear as complete units

## Technical Implementation Details

### Current Flow Analysis
1. User types character → `handleKeyMsg()` → `updateAutocomplete()` → `updateGhostText()`
2. `View()` renders input + ghost text
3. `ViewDropdown()` creates dropdown content
4. `renderBaseView()` integrates dropdown into main TUI

### Key Fix Points
1. **Ghost Text Calculation**: Fix substring logic in `updateGhostText()`
2. **Cursor Management**: Add cursor positioning after `SetValue()`
3. **Dropdown Integration**: Improve positioning in `renderBaseView()`
4. **Command Flattening**: Enhance registry to return compound commands

### State Management
- Ensure `AutocompleteState` correctly tracks selection and visibility
- Maintain consistency between ghost text and dropdown selection
- Handle state transitions cleanly (typing, navigation, completion)

## Constraints and Considerations

### Performance
- Autocomplete must remain responsive during typing
- Dropdown rendering should be efficient for large command lists
- Ghost text updates should not cause flicker

### Compatibility
- Must work with existing theme system
- Should not break current keyboard shortcuts
- Maintain API compatibility with existing command registry

### User Experience
- Autocomplete behavior should feel natural and predictable
- Error states should be handled gracefully
- Terminal resize should not break autocomplete display

### Edge Cases
- Very narrow terminal windows
- Commands with many subcommands
- Rapid typing and navigation
- Theme switching during active autocomplete

## Success Metrics

### Functional Success
- [ ] Ghost text appears without extra spaces
- [ ] Tab completion moves cursor to correct position
- [ ] Dropdown menu displays properly positioned
- [ ] Compound commands work as single units
- [ ] All existing functionality preserved

### User Experience Success
- [ ] Autocomplete feels responsive and smooth
- [ ] Behavior matches modern terminal expectations
- [ ] No visual glitches or artifacts
- [ ] Keyboard navigation is intuitive
- [ ] Error recovery is graceful
