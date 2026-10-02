# Design Specification: Add --version/-v Command Line Flag

**Issue**: #97 - Add --version/-v command line flag to display version without entering REPL

**Closes #97**

## Solution Approach

Add `--version` and `-v` flags to the root Cobra command that display the version number and exit immediately, bypassing the REPL. This leverages the existing `internal/version` package which already handles version information and build-time embedding.

## Architecture Analysis

The current architecture already supports this change cleanly:

1. **Existing Version Package**: `internal/version` already exists with:
   - JSON-based version embedding (`version.json`)
   - Build-time variable injection support (`Version`, `BuildDate` variables)
   - `Info()` method for structured version data
   - Proper semver validation in tests

2. **Cobra CLI Structure**: Root command in `cmd/kiro-krew/cmd/root.go` uses Cobra which has built-in flag support

3. **Current REPL Usage**: TUI already uses `version.Info()` in the about command, so the version display logic is proven

## Relevant Files

### Files to Modify
- `cmd/kiro-krew/cmd/root.go` - Add version flag and handler
- `internal/version/version.go` - Add simple version string method

### Files Referenced (No Changes)
- `internal/version/version.json` - Current version data
- `internal/version/version_test.go` - Existing version validation
- `internal/tui/commands.go` - Reference for how version is currently displayed

## Team Orchestration

This is a single-file change with minimal coordination required:
- **Builder**: Implements the Cobra flag and version output logic
- **Validator**: Verifies the flag works correctly and doesn't interfere with existing functionality

## Step-by-Step Task Breakdown

### Task 1: Add Version String Method
**File**: `internal/version/version.go`
**Action**: Add a simple `String()` method that returns just the version number
**Acceptance Criteria**:
- Method returns clean version string (e.g., "0.5.0") without extra formatting
- Handles prerelease versions correctly (e.g., "0.5.0-beta.1")
- No breaking changes to existing API

### Task 2: Add Cobra Version Flag
**File**: `cmd/kiro-krew/cmd/root.go`
**Action**: Add `--version`/`-v` flag to root command
**Acceptance Criteria**:
- Flag is added as a persistent flag to root command
- When flag is present, print version to stdout and exit with code 0
- Flag takes precedence over other arguments and subcommands
- Uses standard Cobra patterns for version flags
- Output is minimal (version string only, no extra text)

### Task 3: Test Integration
**Action**: Verify the implementation works correctly
**Acceptance Criteria**:
- `kiro-krew --version` displays version and exits
- `kiro-krew -v` displays version and exits  
- Version output goes to stdout, not stderr
- Exit code is 0
- Flag works with other arguments (version takes precedence)
- Existing functionality remains unaffected
- No conflicts with current CLI parsing

## Implementation Details

### Version Output Format
- Output should be just the version number: `0.5.0`
- No additional text like "kiro-krew version 0.5.0"
- Newline after version number for proper terminal display

### Cobra Integration
- Use `cmd.Flags().BoolP("version", "v", false, "display version and exit")`
- Check flag in `RunE` function before normal execution
- Use early return pattern to bypass REPL startup

### Build Integration
The existing version system already supports build-time injection:
```bash
go build -ldflags "-X github.com/jbrinkman/kiro-krew/internal/version.Version=1.2.3"
```

## Validation Commands

### Basic Functionality Tests
```bash
# Test long form
kiro-krew --version
# Expected: prints "0.5.0" and exits with code 0

# Test short form  
kiro-krew -v
# Expected: prints "0.5.0" and exits with code 0

# Test precedence
kiro-krew --version init
# Expected: prints "0.5.0" and exits (ignores init command)
```

### Regression Tests  
```bash
# Verify normal operation still works
kiro-krew init
kiro-krew help
kiro-krew

# Verify about command still works in REPL
echo "about" | kiro-krew
```

### Build-time Version Tests
```bash
# Test custom version injection
go build -ldflags "-X github.com/jbrinkman/kiro-krew/internal/version.Version=1.2.3-custom" -o kiro-krew-test ./cmd/kiro-krew
./kiro-krew-test --version
# Expected: prints "1.2.3-custom"
```

## Edge Cases Handled

1. **Version flag with other flags**: Version takes precedence
2. **Invalid version data**: Falls back to "dev" (existing behavior)
3. **Build without version injection**: Uses version.json content (existing behavior)
4. **Prerelease versions**: Correctly formats with hyphen (existing behavior)

## Non-Goals

- No changes to existing about command in REPL
- No additional version information (build date, etc.) in flag output
- No version checking/update functionality in the flag
- No breaking changes to existing version package API
