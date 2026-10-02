# Design Specification: Add Dependency Validation with Exponential Backoff to Watcher

**Issue**: #91  
**Closes**: #91  
**Date**: 2026-06-13  
**Status**: Draft

## Problem Statement

The kiro-krew watcher currently spawns agents for any issue with the configured label, regardless of whether prerequisite work is complete. This leads to agent failures when implementing features that depend on incomplete foundations, causing cascading failures and inefficient resource usage.

## Solution Approach

Implement dependency parsing and validation with exponential backoff strategy to ensure agents only work on issues when all prerequisite work is completed. This involves:

1. **Dependency Parsing**: Extract issue dependencies from issue descriptions using multiple supported formats
2. **Validation Logic**: Check dependency states before spawning agents using GitHub API calls
3. **Exponential Backoff**: Reduce API calls for blocked issues using in-memory tracking with progressive delays
4. **Integration**: Seamlessly integrate with existing watcher, retry, and labeling systems

### Design Goals
- **Robust Parsing**: Support multiple dependency declaration formats
- **Efficient API Usage**: Minimize GitHub API calls through intelligent backoff
- **Clean Integration**: No breaking changes to existing functionality
- **Clear Logging**: Comprehensive debugging information for dependency states
- **Graceful Degradation**: Handle edge cases like circular dependencies

## Current Architecture Analysis

### Key Components

1. **Watcher** (`internal/watcher/watcher.go`)
   - Current flow: `checkIssues()` → `ListIssues()` → spawn agents directly
   - Existing retry logic: Global retry counts in `.kiro-krew/retries/`
   - Missing: Dependency validation step before spawning

2. **GitHub Client** (`internal/github/client.go`)
   - Current API: `ListIssues()` fetches `number,title,labels`
   - Missing: Individual issue details with `body,state` for dependency validation

3. **Config Structure** (`internal/config/config.go`)
   - Existing fields: `PollInterval`, `MaxRetries`, etc.
   - No dependency-related configuration needed (backoff is algorithmic)

## Relevant Files

### Files to Modify
- **`internal/watcher/watcher.go`**: Add dependency validation logic and backoff tracking
- **`internal/github/client.go`**: Add `GetIssueDetails()` function for dependency validation
- **`internal/watcher/dependencies.go`**: New file for dependency parsing and validation logic

### Files to Reference
- **Existing specs**: Review patterns in `.kiro-krew/specs/` for integration approaches
- **Config file**: `.kiro-krew/config.yaml` for understanding current watcher configuration

## Team Orchestration

### Implementation Phases
1. **Phase 1**: Add dependency parsing functionality (pure functions, no side effects)
2. **Phase 2**: Extend GitHub client with issue details API
3. **Phase 3**: Integrate dependency validation into watcher loop
4. **Phase 4**: Add exponential backoff tracking and logic

### Component Interactions
- **Watcher**: Orchestrates dependency checking before agent spawning
- **GitHub Client**: Provides both issue listing and individual issue details
- **Dependency Parser**: Pure functions for extracting dependencies from issue text
- **Backoff Tracker**: In-memory state for managing retry delays

## Step-by-Step Task Breakdown

### Task 1: Implement Dependency Parsing
**File**: `internal/watcher/dependencies.go`
**Acceptance Criteria**:
- [ ] Parse "Depends on Issue #N" format
- [ ] Parse "Dependencies: #N, #M" format  
- [ ] Parse "Blocked by: #N" format
- [ ] Parse markdown links "Depends on [Issue #88](url)"
- [ ] Return slice of issue numbers from any text input
- [ ] Handle malformed input gracefully (empty slice, no errors)

**Implementation Details**:
```go
// DependencyParser extracts issue numbers from issue body text
type DependencyParser struct{}

func (dp *DependencyParser) ParseDependencies(issueBody string) []int
func (dp *DependencyParser) extractIssueNumbers(text string) []int
```

### Task 2: Extend GitHub Client for Issue Details
**File**: `internal/github/client.go`
**Acceptance Criteria**:
- [ ] Add `GetIssueDetails(repo string, number int)` function
- [ ] Return issue body and state using `gh issue view <number> --json body,state`
- [ ] Handle API errors gracefully (return error, don't crash)
- [ ] Parse JSON response into structured data

**Implementation Details**:
```go
type IssueDetails struct {
    Body  string `json:"body"`
    State string `json:"state"`
}

func GetIssueDetails(repo string, number int) (*IssueDetails, error)
```

### Task 3: Add Backoff Tracking Structure
**File**: `internal/watcher/dependencies.go`
**Acceptance Criteria**:
- [ ] Define `DependencyBackoff` struct with issue number, failure count, next check round
- [ ] Implement in-memory tracking (map keyed by issue number)
- [ ] Calculate backoff delay: 2^failure_count polling rounds (max 16x)
- [ ] Reset backoff on watcher restart (intentionally non-persistent)
- [ ] Provide logging for backoff status

**Implementation Details**:
```go
type DependencyBackoff struct {
    issueNumber   int
    failureCount  int
    nextCheckRound int
}

type BackoffTracker struct {
    backoffs    map[int]*DependencyBackoff
    currentRound int
    mu          sync.RWMutex
}

func (bt *BackoffTracker) ShouldCheck(issueNumber int) bool
func (bt *BackoffTracker) RecordFailure(issueNumber int)
func (bt *BackoffTracker) IncrementRound()
```

### Task 4: Implement Dependency Validation
**File**: `internal/watcher/dependencies.go`
**Acceptance Criteria**:
- [ ] Check all dependencies are in "closed" state
- [ ] Return validation result with details of unresolved dependencies
- [ ] Handle GitHub API errors gracefully
- [ ] Detect circular dependencies and log warnings
- [ ] Integrate with existing logging patterns

**Implementation Details**:
```go
type ValidationResult struct {
    IsValid             bool
    UnresolvedDependencies []int
    CircularDependencies   []int
}

type DependencyValidator struct {
    parser  *DependencyParser
    visited map[int]bool
}

func (dv *DependencyValidator) ValidateIssue(repo string, issueNumber int) (*ValidationResult, error)
func (dv *DependencyValidator) checkCircularDependencies(repo string, issueNumber int, visited map[int]bool) []int
```

### Task 5: Integrate with Watcher Main Loop
**File**: `internal/watcher/watcher.go`
**Acceptance Criteria**:
- [ ] Add dependency validation before agent spawning
- [ ] Skip issues that fail dependency validation
- [ ] Update backoff tracker on each polling round
- [ ] Log dependency check results for debugging
- [ ] Preserve existing functionality for issues without dependencies

**Implementation Details**:
- Modify `checkIssues()` to call dependency validation
- Add dependency validation step between existing checks and agent spawning
- Initialize `BackoffTracker` in watcher constructor
- Call `IncrementRound()` at start of each polling cycle

### Task 6: Add Comprehensive Logging
**Files**: `internal/watcher/watcher.go`, `internal/watcher/dependencies.go`
**Acceptance Criteria**:
- [ ] Log when issues are skipped due to unresolved dependencies
- [ ] Log backoff status: "Issue #88 in backoff, checking again in 4 rounds"
- [ ] Log circular dependency warnings
- [ ] Log dependency parsing results (debug level)
- [ ] Maintain existing log format and verbosity

## Validation Commands

### Unit Tests
```bash
# Test dependency parsing with various formats
go test ./internal/watcher -run TestParseDependencies

# Test backoff calculation logic
go test ./internal/watcher -run TestBackoffTracker

# Test GitHub client extension
go test ./internal/github -run TestGetIssueDetails
```

### Integration Tests
```bash
# Create test issues with dependencies
gh issue create --title "Dependency Test Parent" --body "Parent issue for testing"
gh issue create --title "Dependency Test Child" --body "Depends on Issue #<parent_number>"

# Start watcher and verify dependency validation
./kiro-krew
# In REPL: watch start
# Verify child issue is not processed until parent is closed
```

### Manual Verification
```bash
# Test dependency parsing patterns
echo "Depends on Issue #88" | # should extract [88]
echo "Dependencies: #88, #89" | # should extract [88, 89]
echo "Blocked by: #90" | # should extract [90]

# Test backoff progression with mock polling rounds
# Round 1: First failure → skip next 2 rounds
# Round 4: Check again, second failure → skip next 4 rounds  
# Round 9: Check again, third failure → skip next 8 rounds
```

### Error Scenarios to Test
- [ ] Malformed dependency declarations (should be ignored)
- [ ] Non-existent dependency issue numbers (should fail validation)
- [ ] GitHub API errors during dependency checking (should retry with backoff)
- [ ] Circular dependencies (should log warning and process anyway)
- [ ] Mixed resolved/unresolved dependencies (should wait for all)

## Integration Considerations

### Backward Compatibility
- Issues without dependencies should process immediately (no behavior change)
- Existing retry logic should remain unchanged
- Current labeling system (`kiro-krew-done`, `kiro-krew-failed`) unchanged
- No configuration changes required

### Performance Impact
- Additional GitHub API call per issue with dependencies
- In-memory backoff tracking has minimal overhead
- Backoff strategy reduces API calls over time for blocked issues
- Dependency parsing is CPU-only (no I/O impact)

### Error Handling
- GitHub API failures should not crash watcher
- Malformed dependency syntax should be ignored (graceful degradation)
- Circular dependencies should be detected and logged but not block processing
- Network timeouts should be handled with existing retry mechanisms

## Future Enhancements (Out of Scope)

- Cross-repository dependency support (requires authentication changes)
- Persistent backoff state (would complicate restart behavior)
- Dependency visualization in TUI (separate feature)
- Automatic dependency creation from code analysis (separate feature)
- Webhook-based dependency resolution (architectural change)
