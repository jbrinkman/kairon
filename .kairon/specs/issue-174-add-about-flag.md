# Design Specification: Add --about (-a) Flag

**Issue:** #174  
**Title:** Add --about (-a) flag to display comprehensive version information  
**Closes:** #174

## Solution Approach

Add a new `--about` / `-a` flag to the root Cobra command that displays comprehensive version information using the existing `version.Info()` function. The implementation will reuse the formatting logic from `internal/tui/about.go` to ensure consistency across the CLI and TUI interfaces.

The solution leverages Cobra's persistent flag system and exits immediately after displaying the information, similar to how `--version` works but with more detailed output.

## Architecture Overview

The implementation follows the existing CLI patterns in the codebase:
- Add flag definition to `root.go` in the `init()` function
- Create a pre-run hook that checks for the flag and handles the output
- Reuse existing `version.Info()` and `formatCommitHash()` functions
- Follow Cobra best practices for immediate exit flags

## Relevant Files

### Files to Modify
- `cmd/kiro-krew/cmd/root.go` - Add --about flag and handler logic

### Files Referenced (No Changes)
- `internal/version/version.go` - Contains `version.Info()` function
- `internal/tui/about.go` - Contains formatting reference and `formatCommitHash()` function

## Implementation Details

### Flag Integration
- Add `--about` and `-a` flags using Cobra's `PersistentFlags().BoolP()` 
- Place flag definition in `init()` function alongside existing setup
- Use a pre-run hook (`PersistentPreRunE`) to check flag and handle output before normal command execution

### Output Formatting
Reuse the exact formatting pattern from `internal/tui/about.go`:
```
  Version:    <version>
  Build Date: <build_date>
  Commit:     <short_hash>
  Go Version: <go_version>
  Arch:       <arch>
```

### Flag Precedence
The `--about` flag should work independently and exit immediately, similar to `--version`. If both flags are present, `--about` takes precedence as it provides more comprehensive information.

## Step-by-Step Task Breakdown

### Task 1: Add Flag Definition
**Acceptance Criteria:**
- Add `--about` and `-a` flags to root command
- Flag should be a boolean type
- Should not conflict with existing `--version/-v` flag

### Task 2: Implement Flag Handler
**Acceptance Criteria:**
- Create pre-run hook that checks for `--about` flag
- When flag is present, display version information and exit with code 0
- Use existing `version.Info()` function to get data
- Import and use `formatCommitHash()` from `internal/tui/about.go` for consistent formatting

### Task 3: Format Output
**Acceptance Criteria:**
- Output format matches TUI AboutDialog exactly
- Proper alignment with consistent spacing (2 spaces indent, colon alignment)
- Short commit hash (7 characters) using existing `formatCommitHash()` function
- No trailing newlines or extra spacing

## Team Orchestration

This is a single-file change that requires no coordination between teams. The implementation is self-contained within the CLI command structure and reuses existing version infrastructure.

## Validation Commands

```bash
# Test the new flag works
./kiro-krew --about
./kiro-krew -a

# Verify it doesn't interfere with existing functionality  
./kiro-krew --version
./kiro-krew -v

# Test flag precedence
./kiro-krew --about --version  # Should show about info

# Test with subcommands (should show about info and exit)
./kiro-krew init --about

# Verify normal operation still works
./kiro-krew init
./kiro-krew
```

## Technical Notes

- The implementation uses Cobra's `PersistentPreRunE` to ensure the flag works with any subcommand
- Exit with code 0 after displaying information (standard CLI behavior)
- No changes needed to version package or TUI components
- Minimal code changes focused only on the root command