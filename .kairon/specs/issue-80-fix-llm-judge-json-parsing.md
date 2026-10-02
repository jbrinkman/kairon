# Fix LLM-as-a-Judge JSON Parsing Errors Design Specification

**Issue:** #80 - Fix LLM-as-a-Judge JSON parsing errors causing 13 skipped rubrics in evaluations  
**Closes:** #80

## Problem Analysis

The LLM-as-a-Judge evaluation system is failing to parse JSON responses from `kiro-cli chat` due to ANSI escape sequences contaminating the JSON content. This causes 13 out of 22 rubrics to be skipped with `JSON parse error: invalid character '\x1b' looking for beginning of value`.

### Root Cause Analysis

**Technical Issue:** The `kiro-cli chat --no-interactive` command outputs ANSI escape sequences (e.g., `\x1B[38;5;141m`, `\x1B[0m`) that are embedded within the JSON response content between the delimiters `===JSON_START===` and `===JSON_END===`.

**Evidence from hexdump:**
```
1b 5b 33 38 3b 35 3b 31 34 31 6d 3e 20 1b 5b 30 6d 3d 3d 3d 4a 53 4f 4e 5f 53 54 41 52 54 3d 3d
```
Shows ANSI sequences (`\x1B[38;5;141m`) contaminating JSON content.

**Affected Components:**
- `scoreLLMJudge()` function in `internal/eval/runner.go` (lines ~282-326)
- All non-deterministic rubric criteria across all agents
- JSON delimiter parsing logic that extracts content between `===JSON_START===` and `===JSON_END===`

### Impact Assessment

**Current State:**
- architect: 2/4 skipped (task_decomposition, acceptance_criteria_testability)
- builder: 2/4 skipped (spec_adherence, code_quality)
- documenter: 2/4 skipped (accuracy, practical_usage_guidance)
- planner: 3/4 skipped (requirement_clarity, scope_appropriateness, constraint_identification)  
- validator: 3/4 skipped (issue_coverage, defect_detection, actionable_feedback)
- krew-lead: 4/4 skipped (workflow_adherence, delegation_quality, retry_policy_compliance, error_handling)

**Business Impact:** 59% of evaluation criteria are non-functional, severely limiting evaluation system effectiveness.

## Solution Approach

### Core Strategy
Implement ANSI escape sequence sanitization in the JSON parsing pipeline while maintaining backward compatibility with the existing evaluation framework.

### Technical Approach
1. **ANSI Stripping Function:** Create utility to remove ANSI escape sequences using regex pattern `\x1b\[[0-9;]*m`
2. **Parsing Pipeline Enhancement:** Apply ANSI stripping before JSON parsing in the delimiter extraction logic
3. **Robust Error Handling:** Maintain existing fallback behavior for malformed responses
4. **Backward Compatibility:** Preserve all existing interfaces and data structures

### Architecture Considerations
- **Minimal Invasive Change:** Target only the JSON parsing logic to avoid regression risk
- **Performance:** ANSI stripping adds negligible overhead to evaluation runs
- **Future-Proof:** Solution handles both current ANSI sequences and potential future formatting changes
- **Testability:** Changes are isolated and easily unit-tested

## Relevant Files

### Files to Modify
- `internal/eval/runner.go` - Add ANSI stripping to `scoreLLMJudge` function (lines ~282-326)

### Files to Create (Optional Enhancement)
- `internal/eval/ansi.go` - Dedicated ANSI processing utilities (if complexity grows)

### Files to Reference
- `internal/eval/runner_test.go` - Existing test patterns for deterministic criteria
- `.kiro-krew/evals/results/667ef0c/` - Current failing evaluation results for testing

### Files Unchanged
- `internal/eval/types.go` - All data structures remain unchanged
- All rubric YAML files - No changes to evaluation criteria definitions
- All test case YAML files - Test data structure preserved
- JSON result output format - Maintains existing API compatibility

## Team Orchestration

**Single Component Fix:** This is a targeted fix to a specific parsing issue within the evaluation system. No cross-team coordination required.

**Integration Dependencies:**
- Must maintain compatibility with existing `kiro-cli chat` command interface
- Should preserve all existing evaluation framework contracts
- No changes required to TUI, agent management, or other system components

**Testing Strategy:**
- Unit tests for ANSI stripping function
- Integration tests with actual LLM responses containing ANSI sequences
- Regression verification on commit 667ef0c evaluation results

## Step-by-Step Task Breakdown

### Task 1: Implement ANSI Escape Sequence Stripping
**Acceptance Criteria:**
- Create `stripANSISequences(s string) string` function in `runner.go`
- Function removes all ANSI escape sequences matching pattern `\x1b\[[0-9;]*m`
- Handles edge cases: empty strings, strings without ANSI, malformed sequences
- Performance optimized for typical LLM response sizes

**Implementation Details:**
```go
func stripANSISequences(s string) string {
    ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
    return ansiRegex.ReplaceAllString(s, "")
}
```

**Testing:**
- Unit test with known ANSI sequences from `kiro-cli` output
- Verify preservation of JSON structure after stripping
- Test edge cases and performance with large responses

### Task 2: Integrate ANSI Stripping into JSON Parsing Pipeline
**Acceptance Criteria:**
- Modify `scoreLLMJudge` function to apply ANSI stripping before JSON parsing
- Apply stripping to extracted content between `===JSON_START===` and `===JSON_END===`
- Maintain existing error handling and fallback logic
- No changes to function signature or return values

**Implementation Location:**
In `scoreLLMJudge` function, modify the JSON extraction logic:
```go
jsonStr := raw[start+len("===JSON_START===") : end]
jsonStr = stripANSISequences(strings.TrimSpace(jsonStr))
```

**Error Handling:**
- Preserve existing JSON parsing error messages
- ANSI stripping failures should not cause evaluation crashes
- Maintain existing skip logic for unparseable responses

### Task 3: Add Regression Test for ANSI Handling
**Acceptance Criteria:**
- Create test case that reproduces the ANSI sequence contamination issue
- Test verifies successful JSON parsing after ANSI stripping
- Test covers common ANSI sequences found in `kiro-cli` output
- Integration test with actual `kiro-cli` command (if available in test environment)

**Test Structure:**
```go
func TestScoreLLMJudge_ANSISequences(t *testing.T) {
    // Test with ANSI-contaminated JSON response
    // Verify successful parsing and correct score extraction
}
```

**Test Data:**
- Use actual ANSI sequences captured from `kiro-cli` output
- Include various color codes and formatting sequences
- Test both simple and complex JSON structures with ANSI contamination

### Task 4: Validation on Historical Results
**Acceptance Criteria:**
- Re-run evaluation on commit 667ef0c shows 0 skipped rubrics
- All previously failing rubrics now produce valid scores
- Deterministic rubrics continue to work unchanged
- Total evaluation completeness reaches 100%

**Validation Process:**
1. Apply fix to current codebase
2. Re-run evaluation system on commit 667ef0c  
3. Verify all 22 rubrics execute successfully
4. Compare scores to ensure reasonable evaluation results
5. Check JSON output format integrity

### Task 5: Performance and Compatibility Testing
**Acceptance Criteria:**
- Evaluation runtime increase < 5% due to ANSI processing
- All existing evaluation result formats preserved
- No breaking changes to evaluation API
- Backward compatibility with existing result parsing tools

**Performance Testing:**
- Benchmark ANSI stripping function with typical response sizes
- Measure end-to-end evaluation time before and after changes
- Test with large rubric sets to identify scaling issues

## Validation Commands

### Basic Functionality Validation
```bash
# Test ANSI stripping with actual kiro-cli output
echo 'Test prompt requesting JSON' | kiro-cli chat --no-interactive | hexdump -C

# Run single agent evaluation to test fix
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval architect
```

### Regression Testing on Historical Commit
```bash
# Re-run evaluation on problematic commit 667ef0c
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval
jq '.Cases[].Scores[] | select(.Skipped == true)' evals/results/*/architect.json

# Verify 0 skipped rubrics after fix
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval
find evals/results -name "*.json" -exec jq -r '.Cases[].Scores[] | select(.Skipped == true) | .Name' {} \;
```

### JSON Structure Validation
```bash
# Verify JSON output structure integrity
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval planner
jq '.Cases[0].Scores[] | select(.Deterministic == false)' evals/results/*/planner.json

# Check that LLM judge scores are in valid range
jq '.Cases[].Scores[] | select(.Deterministic == false and .Skipped == false) | .Score' evals/results/*/planner.json
```

### Unit Test Execution
```bash
# Run unit tests for ANSI processing
go test -v ./internal/eval -run TestStripANSISequences
go test -v ./internal/eval -run TestScoreLLMJudge_ANSISequences

# Full test suite regression check
go test ./internal/eval/...
```

### Performance Validation
```bash
# Benchmark evaluation performance
cd .kiro-krew && time go run ../cmd/kiro-krew/main.go eval

# Test with multiple agents to check scaling
cd .kiro-krew && time go run ../cmd/kiro-krew/main.go eval architect builder documenter
```

## Implementation Notes

### ANSI Sequence Pattern Recognition
The regex pattern `\x1b\[[0-9;]*m` covers:
- `\x1b` - ESC character (start of ANSI sequence)
- `\[` - Literal bracket following ESC
- `[0-9;]*` - Zero or more digits and semicolons (color/format codes)
- `m` - Terminal character for color/formatting sequences

### Error Recovery Strategy
- ANSI stripping errors should not crash evaluations
- Fall back to original string if regex processing fails
- Preserve existing JSON parsing error messages for debugging
- Log ANSI processing issues for monitoring

### Backward Compatibility Guarantees
- All existing evaluation APIs remain unchanged
- JSON result structure preserved exactly
- Deterministic evaluation logic untouched
- Command-line interface compatibility maintained

### Future Enhancement Opportunities
- Consider `--no-color` flag for `kiro-cli` if supported
- Potential optimization: cache compiled regex for performance
- Monitor for other text contamination patterns beyond ANSI
- Consider broader text sanitization framework if issues expand

## Risk Assessment

**Low Risk Change:**
- Targeted fix to specific parsing issue
- No architectural changes required
- Minimal code surface area affected
- Easy to test and validate

**Potential Issues:**
- Regex compilation overhead (mitigated by simple pattern)
- ANSI detection false positives (unlikely with specific pattern)
- Performance impact (negligible for typical use cases)

**Mitigation Strategies:**
- Comprehensive unit test coverage
- Performance benchmarking before deployment  
- Gradual rollout with monitoring
- Quick rollback capability if issues arise
