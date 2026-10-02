# Design Specification: Add Autocomplete Support for "plan classic" Command

**Issue**: #233 - Add autocomplete support for "plan classic" command  
**Repository**: jbrinkman/kiro-krew  
**Closes**: #233

## Problem Statement

The footer command input does not include autocomplete suggestions for the `plan classic` command. While the command is implemented and functional (as shown in `internal/tui/tui.go` and documented in the help text), it's missing from the command registry used for autocomplete.

Users typing "plan" currently see autocomplete for `plan [desc]` but not for the `plan classic [desc]` variant, creating inconsistent user experience where a documented command lacks autocomplete support.

## Solution Approach

Update the `CommandRegistry` in `internal/tui/command_registry.go` to include "classic" as a subcommand for the `plan` command. This follows the existing pattern used by the `watch` command which has "start" and "stop" subcommands.

The flattened command matching system already supports compound commands and will automatically generate "plan classic" suggestions once the subcommand is registered.

## Architecture Analysis

### Current Command Registry Structure
- Commands are defined in `NewCommandRegistry()` function in `internal/tui/command_registry.go`
- Each command has optional `Subcommands []string` field for compound commands
- The `buildFlattenedCommands()` method creates compound command strings like "watch start", "watch stop"
- Autocomplete uses `GetFlattenedMatches()` to provide suggestions including compound commands

### Current Plan Command Registration
```go
registry.register(&Command{
    Name:        "plan",
    Description: "Start interactive planning session",
    HasArgs:     true,
    ArgPattern:  "[desc]",
})
```

### Command Execution Flow
- Command input is processed in `executeCommand()` in `internal/tui/tui.go` lines 1029-1045
- The "plan classic" detection already exists: `if len(parts) >= 2 && strings.ToLower(parts[1]) == "classic"`
- No changes needed to command execution logic

## Relevant Files

### Files to Modify
- `internal/tui/command_registry.go` - Add "classic" subcommand to plan command registration
- `internal/tui/command_registry_test.go` - Add test coverage for plan classic autocomplete

### Files for Reference (No Changes Required)
- `internal/tui/tui.go` - Contains existing command execution logic (lines 1029-1045)
- `internal/tui/autocomplete_test.go` - Existing autocomplete test patterns

## Team Orchestration

This is a straightforward single-file change with accompanying test updates. No coordination between different components is required since:

- Command execution logic already exists in `tui.go`
- Only the command registry needs updating to expose the subcommand for autocomplete
- Testing can be done immediately after the change

## Step-by-Step Task Breakdown

### Task 1: Update Command Registry for Plan Classic Autocomplete
**Acceptance Criteria**:
- Add "classic" to the Subcommands field of the plan command in `command_registry.go`
- Verify that `buildFlattenedCommands()` automatically includes "plan classic" in flattened commands
- Ensure autocomplete suggestions include "plan classic [desc]" pattern
**Dependencies**: None

### Task 2: Add Comprehensive Test Coverage
**Acceptance Criteria**:
- Add test case for "plan classic" command validation in `command_registry_test.go`
- Add test case for "plan classic" appearing in flattened command matches
- Add test case for "plan c" prefix matching to "plan classic"
- Add test case for subcommand filtering of "classic" under "plan"
- Verify all existing tests continue to pass
**Dependencies**: Task 1

### Task 3: Validate Complete Autocomplete Behavior
**Acceptance Criteria**:
- Typing "plan" shows both "plan" and "plan classic" options
- Typing "plan c" or "plan cl" filters to show "plan classic [desc]"
- Tab completion works correctly for "plan classic"
- Command execution continues to work exactly as before
- All validation commands pass
**Dependencies**: Task 1, Task 2

## Implementation Details

### Required Code Change in command_registry.go
```go
registry.register(&Command{
    Name:        "plan",
    Description: "Start interactive planning session",
    Subcommands: []string{"classic"},
    HasArgs:     true,
    ArgPattern:  "[desc]",
})
```

### Expected Autocomplete Behavior After Implementation
- `GetFlattenedMatches("plan")` returns: `["plan", "plan classic"]`
- `GetFlattenedMatches("plan c")` returns: `["plan classic"]`
- `GetSubcommands("plan")` returns: `["classic"]`
- `IsValidCommand("plan classic")` returns: `true`

### Test Cases to Add
```go
// Test plan classic subcommand exists
subcommands := registry.GetSubcommands("plan")
assert.Contains(t, subcommands, "classic")

// Test plan classic in flattened matches
matches := registry.GetFlattenedMatches("plan")
assert.Contains(t, matches, "plan classic")

// Test plan classic validation
assert.True(t, registry.IsValidCommand("plan classic"))
assert.True(t, registry.IsValidCommand("plan classic some description"))

// Test prefix filtering for plan classic
matches = registry.GetFlattenedMatches("plan c")
assert.Contains(t, matches, "plan classic")
```

## Validation Commands

After implementation, verify the following behaviors work correctly:

```bash
# Build and run tests
go test ./internal/tui/... -v

# Manual testing of autocomplete (if running kiro-krew interactively)
# 1. Type "plan" and verify both "plan" and "plan classic" appear in suggestions
# 2. Type "plan c" and verify "plan classic" is suggested
# 3. Tab-complete "plan cl" should complete to "plan classic"
# 4. Verify "plan classic test" command still executes the classic planning mode

# Verify existing functionality unchanged
# 1. "plan some description" should work as before
# 2. "watch start" and "watch stop" autocomplete should still work
# 3. All other commands should autocomplete normally
```

## Backward Compatibility

This change is fully backward compatible:
- No existing command behavior is modified
- No existing API contracts are changed
- Only adds autocomplete functionality for an already-working command
- All existing tests should continue to pass

## Risk Assessment

**Low Risk**: This is a safe, additive change that only affects autocomplete behavior. The command execution logic remains unchanged, and the registry pattern is well-established.

**Potential Issues**:
- None anticipated - the implementation follows the exact same pattern as existing subcommands

**Rollback Plan**: 
- Remove "classic" from the Subcommands array to revert the change