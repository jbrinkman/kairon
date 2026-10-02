# Design Specification: Enhance Evaluation Framework

**Issue**: #125 - Enhance evaluation framework with debugging, selective testing, and progress visibility  
**Closes**: #125

## Solution Approach

The current evaluation framework in `internal/eval/runner.go` will be enhanced with debugging capabilities, selective execution, real-time progress feedback, configurable thresholds, and progressive result saving. The approach focuses on extending existing structures while maintaining backward compatibility.

### Key Architectural Decisions

1. **Progressive Execution Model**: Transform the current batch execution into incremental processing with immediate result saving
2. **Enhanced Error Capture**: Capture both stdout and stderr from kiro-cli with structured error context
3. **Flexible Command Interface**: Extend CLI to support selective test execution and listing
4. **Real-time Progress**: Add live progress reporting with time estimates and result paths
5. **Configurable Thresholds**: Support per-test-case scoring thresholds in YAML configuration

## Relevant Files

### Files to Modify

- `cmd/kiro-krew/cmd/eval.go` - Enhanced CLI command interface
- `internal/eval/runner.go` - Core evaluation engine with progressive execution
- `internal/eval/types.go` - Extended data structures for enhanced functionality
- `internal/eval/progress.go` - **NEW** - Progress tracking and display
- `internal/eval/selective.go` - **NEW** - Single test case execution logic

### Files to Reference

- `.kiro-krew/evals/cases/*/` - Test case YAML files (for schema extension)
- `.kiro-krew/evals/results/` - Result directory structure
- `cmd/kiro-krew/cmd/root.go` - CLI command registration

## Team Orchestration

This enhancement requires coordinated changes across multiple components:

1. **CLI Layer**: Extended command parsing and validation
2. **Execution Engine**: Progressive evaluation with error capture
3. **Progress System**: Real-time feedback and time estimation
4. **Result Management**: Incremental saving and resumption logic
5. **Configuration**: Extended YAML schema for test cases

## Step-by-Step Task Breakdown

### Task 1: Extend CLI Command Interface

**Acceptance Criteria**:
- Support `kiro-krew eval agent testcase` for single test execution
- Support `kiro-krew eval agent --case=testcase` alternative syntax
- Support `kiro-krew eval agent --list` to show available test cases
- Support `kiro-krew eval agent --resume` for interrupted evaluation resumption
- Maintain backward compatibility with `kiro-krew eval agent`

**Implementation**:
- Modify `cmd/kiro-krew/cmd/eval.go` to parse new flags and arguments
- Add validation for test case names and agent names
- Implement list functionality to enumerate available tests

### Task 2: Create Progress Tracking System

**Acceptance Criteria**:
- Display results directory path at evaluation start
- Show current test progress (e.g., "[2/5] test-name")
- Display elapsed time for current test (updating every 5-10s)
- Show estimated time remaining based on previous test durations
- Preserve progress state for resumption

**Implementation**:
- Create `internal/eval/progress.go` with progress tracking structures
- Implement time estimation based on historical test durations
- Add progress display with periodic updates during long-running tests

### Task 3: Enhance Error Handling and Debugging

**Acceptance Criteria**:
- Capture and preserve kiro-cli stderr output
- Include execution context (command, working directory, environment)
- Store error details in result files even for failed tests
- Provide structured error information for debugging

**Implementation**:
- Modify `invokeAgent()` function to capture both stdout and stderr
- Extend `CaseResult` structure to include error context and stderr
- Ensure error details are written to result files immediately

### Task 4: Implement Progressive Result Saving

**Acceptance Criteria**:
- Write individual test results immediately after completion
- Update summary file after each test case
- Preserve partial results if evaluation is interrupted
- Enable resumption from last completed test case

**Implementation**:
- Modify evaluation loop to write results after each test case
- Implement result file locking to prevent corruption
- Add resumption logic to skip already completed test cases
- Maintain incremental summary updates

### Task 5: Add Configurable Success Thresholds

**Acceptance Criteria**:
- Support `min_score` field in test case YAML files
- Default to 80% threshold with per-test override capability
- Display thresholds in output: "✅ 85% (threshold: 80%)"
- Backward compatibility for test cases without explicit thresholds

**Implementation**:
- Extend `TestCase` structure to include optional `MinScore` field
- Modify scoring display logic to show thresholds
- Update evaluation logic to use per-test thresholds

### Task 6: Implement Selective Test Execution

**Acceptance Criteria**:
- Execute single test cases by name
- Validate test case existence before execution
- Maintain same result format for single and batch execution
- Support case name completion and validation

**Implementation**:
- Create `internal/eval/selective.go` for single test execution
- Add test case enumeration and validation functions
- Modify main evaluation flow to support selective execution

### Task 7: Performance Optimization Investigation

**Acceptance Criteria**:
- Profile current evaluation bottlenecks
- Investigate parallel execution opportunities
- Consider result caching for unchanged test cases
- Document performance findings and recommendations

**Implementation**:
- Add performance profiling to identify bottlenecks
- Evaluate kiro-cli startup overhead and optimization opportunities
- Design safe parallel execution strategy for independent test cases

## Validation Commands

```bash
# Test selective execution
kiro-krew eval architect basic-spec-generation
kiro-krew eval architect --case=basic-spec-generation

# Test list functionality
kiro-krew eval architect --list

# Test progress and error handling
kiro-krew eval architect  # Should show progress and handle errors gracefully

# Test resumption
# (Interrupt evaluation mid-run)
kiro-krew eval architect --resume

# Verify result files contain error details
cat .kiro-krew/evals/results/*/architect.json | jq '.cases[] | select(.case_name == "basic-spec-generation") | .error_context'

# Test configurable thresholds
# (Add min_score: 90 to a test case YAML)
kiro-krew eval architect basic-spec-generation  # Should show custom threshold

# Verify progressive saving
# (Check results directory during evaluation)
ls -la .kiro-krew/evals/results/*/
```

## Implementation Priority

1. **High Priority**: Error handling, progressive saving, selective execution
2. **Medium Priority**: Progress visibility, configurable thresholds
3. **Low Priority**: Performance optimization, result caching

## Backward Compatibility Notes

- All existing evaluation commands continue to work unchanged
- Current result file formats are preserved and extended
- Test case YAML files without new fields work with defaults
- Existing rubrics and evaluation logic remain functional

## Performance Considerations

- Progressive result saving adds minimal I/O overhead
- Progress display updates should be throttled to avoid performance impact
- Single test execution should be significantly faster than full suite
- Consider async result writing to avoid blocking evaluation

## Error Recovery Strategy

- Incomplete evaluations preserve all completed test results
- Resumption skips successfully completed tests
- Result file corruption detection and recovery
- Graceful handling of interrupted kiro-cli processes