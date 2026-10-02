# Design Specification: Add Timestamp Prefix to Eval Result Folders

**Issue:** #101  
**Title:** Add timestamp prefix to eval result folders for multiple runs per commit  
**Closes:** #101

## Solution Approach

The current eval system stores results in folders named only by git commit hash (e.g., `6a131f0`), preventing multiple evaluation runs per commit. We will implement UTC timestamp prefixes in the format `mmddhh-hhmmss-{commit-hash}` to enable multiple evaluations per commit while maintaining backward compatibility.

The solution involves:
1. **Timestamp Generation**: Create UTC timestamps in `mmddhh-hhmmss` format
2. **Directory Creation**: Update result directory naming in runner
3. **Migration Logic**: Rename existing directories using git commit timestamps
4. **Backward Compatibility**: Update diff command to handle both old and new formats
5. **Folder Matching**: Smart pattern matching for existing folders

## Relevant Files

### Core Implementation Files
- `internal/eval/runner.go` - Lines ~30-35: Update `resultsDir` creation logic
- `internal/eval/diff.go` - Update folder resolution for backward compatibility
- `internal/eval/types.go` - No changes needed to data structures

### Supporting Files
- `cmd/kiro-krew/cmd/eval.go` - No changes needed to CLI interface

### New Files
- `internal/eval/migration.go` - Migration logic for existing directories
- `internal/eval/util.go` - Utility functions for timestamp and folder handling

## Team Orchestration

This is a self-contained change that affects only the eval system:
- **Builder**: Implements timestamp generation, migration logic, and backward compatibility
- **Validator**: Verifies new timestamp format works and old folders still accessible
- No coordination with other components needed

## Step-by-Step Task Breakdown

### Task 1: Implement Timestamp Utilities
**File:** `internal/eval/util.go`
**Acceptance Criteria:**
- Function `generateTimestampPrefix()` returns UTC timestamp in `mmddhh-hhmmss` format
- Function `parseDirectoryName(string)` extracts commit hash from both old and new formats
- Function `getCommitTimestamp(hash string)` retrieves Unix timestamp for migration

### Task 2: Update Result Directory Creation
**File:** `internal/eval/runner.go`
**Acceptance Criteria:**
- Line ~32: Replace `resultsDir := filepath.Join(".kiro-krew", "evals", "results", gitHash)` with timestamped version
- New directory format: `{timestamp}-{gitHash}` where timestamp is `mmddhh-hhmmss`
- Preserve all existing functionality

### Task 3: Implement Migration Logic  
**File:** `internal/eval/migration.go`
**Acceptance Criteria:**
- Function `migrateExistingDirectories()` renames old format directories
- Uses git commit timestamps converted to `mmddhh-hhmmss` format
- Handles migration errors gracefully (skip problematic directories)
- Called automatically on first run with new timestamp format

### Task 4: Update Diff Command Backward Compatibility
**File:** `internal/eval/diff.go`
**Acceptance Criteria:**
- Accept folder names in both `{hash}` and `{timestamp}-{hash}` formats
- Function `resolveRunDirectory(runName string)` handles format detection
- Existing `kiro-krew eval diff 6a131f0 78cdf37` commands continue working
- New commands like `kiro-krew eval diff 061800-120000-6a131f0 061801-120000-78cdf37` work

### Task 5: Integration and Migration
**File:** `internal/eval/runner.go`  
**Acceptance Criteria:**
- Call migration logic on first execution
- Handle migration failures gracefully (log warnings, continue)
- Ensure migration is idempotent (safe to run multiple times)

## Validation Commands

### Test New Timestamp Format
```bash
# Run evaluation to create new timestamped directory
kiro-krew eval

# Verify directory created with timestamp format
ls .kiro-krew/evals/results/ | grep -E '^[0-9]{6}-[0-9]{6}-[a-f0-9]+$'
```

### Test Multiple Runs Per Commit
```bash
# Run eval twice on same commit
kiro-krew eval
sleep 1
kiro-krew eval

# Verify two different timestamped directories exist for same commit
ls .kiro-krew/evals/results/ | grep "$(git rev-parse --short HEAD)" | wc -l
# Should output: 2
```

### Test Backward Compatibility
```bash
# Test diff with old format (if migration successful)
kiro-krew eval diff 6a131f0 78cdf37

# Test diff with new format
kiro-krew eval diff 061800-120000-6a131f0 061801-120000-78cdf37

# Test mixed formats
kiro-krew eval diff 6a131f0 061801-120000-78cdf37
```

### Test Migration Logic
```bash
# Check migration converted existing directories
ls .kiro-krew/evals/results/ | grep -E '^[0-9]{6}-[0-9]{6}-[a-f0-9]+$' | wc -l
# Should equal number of originally existing directories

# Verify old directory names no longer exist
ls .kiro-krew/evals/results/ | grep -E '^[a-f0-9]{7}$' | wc -l
# Should output: 0
```

### Test Timestamp Format
```bash
# Verify timestamp format is correct UTC
echo "Check timestamp format matches mmddhh-hhmmss:"
ls .kiro-krew/evals/results/ | grep -o '^[0-9]\{6\}-[0-9]\{6\}' | head -1
# Should show format like: 061800-120000
```

### Build and Test Validation
```bash
# Ensure code compiles
go build ./cmd/kiro-krew

# Run any existing tests
go test ./internal/eval/...

# Verify eval commands still work
./kiro-krew eval --help
./kiro-krew eval diff --help
```
