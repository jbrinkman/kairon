# Help Command Support for kiro-krew CLI

**Issue:** #33 - Add help command support to kiro-krew CLI  
**Closes:** #33

## Solution Approach

Implement a minimal help system that extends the existing switch-based command routing in `main.go` to support help flags (`--help`, `-h`) for both general and command-specific help. The solution preserves all existing behavior while adding standard CLI help conventions.

### Architectural Decisions

1. **Minimal Implementation**: Extend existing switch statement rather than introducing a CLI framework to maintain simplicity
2. **Help Data Structure**: Define help content as structured data in the main package for maintainability
3. **Flag Detection**: Parse help flags before command routing to catch both global and command-specific help requests
4. **Backwards Compatibility**: All existing commands and default TUI behavior remain unchanged

## Relevant Files

### Files to Modify
- `cmd/kiro-krew/main.go` - Add help flag parsing and help display functions

### Files Referenced
- `internal/eval/runner.go` - Understanding eval command structure
- `internal/eval/diff.go` - Understanding eval diff subcommand

## Team Orchestration

This is a single-file change requiring no coordination between components. The help system is self-contained within the main CLI entry point.

## Step-by-Step Task Breakdown

### Task 1: Define Help Data Structure
**Acceptance Criteria:**
- Create structured help data containing command descriptions, usage, and examples
- Include all existing commands: init, update, eval (with diff subcommand)
- Define both brief (for general help) and detailed (for command-specific help) descriptions

### Task 2: Implement Help Flag Detection
**Acceptance Criteria:**
- Parse `--help` and `-h` flags before command routing
- Support both global help (`kiro-krew --help`) and command-specific help (`kiro-krew eval --help`)
- Preserve existing behavior when no help flags are present

### Task 3: Add Help Display Functions
**Acceptance Criteria:**
- `showGeneralHelp()` displays all commands with brief descriptions and usage
- `showCommandHelp(command)` displays detailed help for specific commands
- Include proper formatting following CLI conventions
- Handle unknown commands gracefully

### Task 4: Update Command Routing Logic
**Acceptance Criteria:**
- Modify existing switch statement to check for help flags first
- Route to appropriate help function when help flags detected
- Maintain all existing command functionality unchanged
- Preserve default TUI launch behavior for `kiro-krew` with no arguments

### Task 5: Add Help Content
**Acceptance Criteria:**
- **General help** shows: program description, usage syntax, available commands list
- **init command help** shows: purpose (extract templates), usage, behavior
- **update command help** shows: purpose (update templates with force), usage, force flag explanation  
- **eval command help** shows: purpose (run evaluations), usage, optional agent parameter, diff subcommand
- **eval diff help** shows: purpose (compare runs), usage, required parameters

## Validation Commands

```bash
# Test general help
kiro-krew --help
kiro-krew -h

# Test command-specific help  
kiro-krew init --help
kiro-krew init -h
kiro-krew update --help
kiro-krew update -h
kiro-krew eval --help
kiro-krew eval -h

# Test existing functionality preservation
kiro-krew                    # Should launch TUI
kiro-krew init              # Should extract templates
kiro-krew update            # Should update templates  
kiro-krew eval              # Should run evaluation
kiro-krew eval diff run1 run2  # Should show diff

# Test error handling
kiro-krew invalid --help    # Should show general help or error
kiro-krew --invalid         # Should show existing behavior
```

## Implementation Notes

- **Minimal Code Changes**: Extend existing patterns rather than restructuring
- **Standard Conventions**: Follow typical CLI help formatting and exit codes
- **Error Handling**: Help requests should exit with code 0, errors with code 1
- **Future Extensibility**: Help data structure should make adding new commands straightforward
