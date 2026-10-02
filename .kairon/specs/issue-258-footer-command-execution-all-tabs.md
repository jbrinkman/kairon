# Design Specification: Footer Command Execution in Agent and Log Tabs

**Issue**: #258  
**Title**: Footer command execution fails in agent and log tabs  
**Closes**: #258

## Problem Summary

When users type commands in the footer input field while viewing agent tabs or log tabs, the autocomplete suggestions display correctly, but pressing Enter does not execute the command. The command only executes when switching back to the main tab. This breaks user workflow and forces unnecessary tab switching.

### Root Cause

The enter key handler in `internal/tui/tui.go` (lines 611-647) uses hardcoded tab type logic that only executes footer commands for:
- **Main tab (TabTypeMain)**: Always executes commands
- **Planning tab (TabTypePlanning)**: Executes commands when footer is focused

For agent tabs (TabTypeAgent) and log tabs (TabTypeLog), the enter key is forwarded to the tab's Update method but never reaches command execution logic, even when the footer input has focus.

The current logic checks tab type first, then footer focus. It should check footer focus first, then handle tab-specific behavior only when footer is NOT focused.

## Solution Approach

Refactor the enter key handler to **prioritize footer focus state over tab type** when determining command execution. The logic should be:

1. **If footer is focused**: Execute the command regardless of tab type
2. **If footer is NOT focused**: Forward the enter key to the active tab for tab-specific handling

This approach ensures consistent command execution across all tab types while preserving tab-specific enter key behavior (e.g., sending messages in planning tabs when the tab content has focus, not the footer).

## Architecture Overview

### Current Flow (Problematic)
```
Enter Key Pressed
  ├─ Check: Is active tab Main?
  │    └─ YES → Execute command
  │    └─ NO → Check: Is active tab Planning AND footer focused?
  │         └─ YES → Execute command
  │         └─ NO → Forward to tab's Update method (command lost)
  └─ Agent/Log tabs never execute commands
```

### New Flow (Correct)
```
Enter Key Pressed
  ├─ Check: Is footer input focused?
  │    └─ YES → Execute command (all tab types)
  │    └─ NO → Forward to active tab's Update method
  └─ Tab-specific enter handling only when footer NOT focused
```

## Relevant Files

### Files to Modify
- **`internal/tui/tui.go`** (lines 611-647): Refactor enter key handler logic
  - Move footer focus check to top of conditional logic
  - Remove tab-type-specific command execution branches
  - Consolidate command execution into single path

### Files Referenced (No Changes)
- **`internal/tui/autocomplete.go`**: Contains `AutocompleteInput.Focused()` method used for focus detection
- **`internal/tui/tabs.go`**: Defines `TabType` constants (TabTypeMain, TabTypeAgent, TabTypePlanning, TabTypeLog)
- **`internal/tui/planning_tab.go`**: Example of tab that needs enter forwarding when footer NOT focused
- **`internal/tui/footer.go`**: Footer management system

## Team Orchestration

This is a single-file refactoring with no dependencies:
- **Task 1**: Refactor enter key handler (can be completed independently)
- **Task 2**: Add test coverage (depends on Task 1)

No parallel execution required - tasks must run sequentially.

## Step-by-Step Task Breakdown

### Task 1: Refactor Enter Key Handler Logic
**File**: `internal/tui/tui.go` (lines 611-647)  
**Acceptance Criteria**:
1. Enter key handler checks `m.input.Focused()` as first condition
2. If footer focused: Execute command immediately (remove tab type checks)
3. If footer NOT focused: Forward to active tab's Update method
4. Preserve existing behavior for main tab (always has footer focused)
5. Preserve existing behavior for planning tab content interaction (enter forwarding when footer NOT focused)
6. All four tab types (Main, Planning, Agent, Log) execute commands when footer focused

**Implementation Details**:
```go
case "enter":
    // Refactored logic: Check footer focus FIRST
    activeTab := m.tabManager.GetActiveTab()
    
    // If footer is focused, execute command regardless of tab type
    if m.input.Focused() {
        var cmd tea.Cmd
        m.input, cmd = m.input.Update(msg)
        
        input := strings.TrimSpace(m.input.Value())
        m.input.SetValue("")
        if input == "" {
            return m, cmd
        }
        
        return m.executeCommand(input)
    }
    
    // Footer NOT focused: Forward to active tab for tab-specific handling
    if activeTab != nil {
        if cmd := m.tabManager.Update(msg); cmd != nil {
            return m, cmd
        }
    }
    return m, nil
```

**Key Changes**:
- Remove hardcoded `activeTab.Type() != TabTypeMain` check
- Remove nested `activeTab.Type() == TabTypePlanning && m.input.Focused()` check
- Single path for command execution when footer focused
- Single path for tab forwarding when footer NOT focused

### Task 2: Add Test Coverage
**File**: Create `internal/tui/enter_key_test.go`  
**Dependencies**: Task 1  
**Acceptance Criteria**:
1. Test command execution in main tab with footer focused
2. Test command execution in planning tab with footer focused
3. Test command execution in agent tab with footer focused
4. Test command execution in log tab with footer focused
5. Test planning tab message sending when footer NOT focused (forwarding behavior)
6. Verify autocomplete continues to work across all tab types

**Implementation Details**:
```go
// Test structure for each tab type
func TestEnterKeyCommandExecutionInAllTabs(t *testing.T) {
    tests := []struct {
        name        string
        tabType     TabType
        footerFocused bool
        inputValue  string
        expectCommand bool
    }{
        {"main tab footer focused", TabTypeMain, true, "help", true},
        {"planning tab footer focused", TabTypePlanning, true, "status", true},
        {"agent tab footer focused", TabTypeAgent, true, "logs", true},
        {"log tab footer focused", TabTypeLog, true, "theme", true},
        {"planning tab content focused", TabTypePlanning, false, "message", false},
    }
    // ... test implementation
}
```

## Validation Commands

### Manual Testing
1. **Start the TUI**: `go run ./cmd/kiro-krew`
2. **Create an agent tab**: Execute `watch start` then `status` to spawn an agent
3. **Test agent tab**: Switch to agent tab, type `help` in footer, press Enter
   - **Expected**: Help command executes and displays in main tab activity
4. **Create a log tab**: Execute `log view`
5. **Test log tab**: Type `status` in footer, press Enter
   - **Expected**: Status display appears
6. **Test planning tab**: Execute `plan`, type command in footer, press Enter
   - **Expected**: Command executes (existing behavior preserved)
7. **Test main tab**: Type `about` in footer, press Enter
   - **Expected**: About dialog appears (existing behavior preserved)

### Automated Testing
```bash
# Run tests
go test ./internal/tui -v -run TestEnterKey

# Run all TUI tests
go test ./internal/tui -v

# Check for regressions
go test ./... -v
```

### Acceptance Verification
All five acceptance criteria must pass:
1. ✅ Footer commands work in agent tabs
2. ✅ Footer commands work in log tabs  
3. ✅ Main/planning tab behavior unchanged
4. ✅ Autocomplete still works (verified by existing autocomplete tests)
5. ✅ Enter forwarding for planning tab content still works (tab content sends messages, not commands)

## Edge Cases and Considerations

### 1. Autocomplete Menu Interaction
- Autocomplete menu should continue to intercept Enter key before command execution
- Current behavior: `m.input.Update(msg)` handles autocomplete selection
- Preserved in refactored code: autocomplete update happens before command execution check

### 2. Empty Input Handling
- Pressing Enter with empty footer input should not execute command
- Current behavior: `if input == "" { return m, cmd }`
- Preserved in refactored code

### 3. Tab Switching During Command Execution
- Commands execute in the context of the current model state
- Tab switching may occur as result of command (e.g., `status` switches to main tab)
- No special handling needed - existing behavior correct

### 4. Planning Tab Message Sending
- Planning tab has dual enter behavior:
  - Footer focused: Execute command
  - Content focused: Send message to agent
- Footer focus state correctly distinguishes these cases
- Refactored logic preserves this by forwarding to tab when footer NOT focused

### 5. Focus State Consistency
- Footer focus state managed by `AutocompleteInput.Focused()` method
- Focus changes handled by existing focus management system
- No changes needed to focus management

## Non-Goals

This specification does NOT include:
- Changes to command parsing or execution logic
- Changes to autocomplete behavior
- Changes to tab switching logic  
- Changes to footer rendering or display
- Changes to focus management system
- New commands or command enhancements

## Success Metrics

1. **Functional**: All acceptance criteria pass manual and automated testing
2. **Behavioral**: No regressions in existing tab or command behavior
3. **Code Quality**: Refactored code is simpler and more maintainable than original
4. **User Experience**: Users can execute commands from any tab without tab switching

## Implementation Notes

### Why This Approach?
- **Minimal change**: Single file, ~30 lines of refactored code
- **Clear logic**: Footer focus check at top of conditional makes intent obvious
- **Maintainable**: Removes nested tab-type checks that obscured the logic
- **Extensible**: Future tab types automatically inherit correct command execution

### Alternative Approaches Considered
1. **Add agent/log tab checks to existing logic**: Rejected because it perpetuates the wrong pattern (tab-type-first vs. focus-first)
2. **Modify tab Update methods**: Rejected because it scatters command execution logic across multiple files
3. **Create separate enter handler per tab type**: Rejected because it duplicates logic and increases maintenance burden

The chosen approach (focus-first refactoring) is the simplest and most correct solution.
