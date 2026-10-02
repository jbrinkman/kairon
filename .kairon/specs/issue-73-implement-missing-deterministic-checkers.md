# Design Specification: Implement Missing Deterministic Checkers

**Issue**: #73  
**Closes**: #73

## Problem Statement

The evaluation system currently has deterministic criteria defined in rubrics but lacks checker implementations for four key criteria: `acceptance_criteria_testability`, `test_execution`, `code_correctness`, and `test_coverage`. These criteria are marked as `deterministic: true` in their respective rubric YAML files but are not handled by the `scoreDeterministic` function, causing them to be skipped during evaluation.

## Solution Approach

### High-Level Strategy

Extend the existing `scoreDeterministic` function in `internal/eval/runner.go` to implement pattern-based heuristic checkers for the four missing deterministic criteria. Each checker will use content analysis patterns to determine scores based on observable characteristics in the test case output.

### Architectural Decisions

1. **Pattern-Based Heuristics**: Use content analysis patterns similar to existing checkers (completeness, file_reference, file_naming)
2. **Minimal False Positives**: Implement conservative scoring that avoids awarding points for borderline cases
3. **Robust Parsing**: Handle various output formats and edge cases gracefully
4. **Extensible Design**: Structure checkers to be easily maintainable and extendable

## Relevant Files

### Files to Modify
- `internal/eval/runner.go` - Add four new deterministic checker cases
- `internal/eval/runner_test.go` (new) - Unit tests for checker implementations

### Files Referenced for Context
- `.kiro-krew/evals/rubrics/planner.yaml` - Contains `acceptance_criteria_testability` criterion
- `.kiro-krew/evals/rubrics/validator.yaml` - Contains `test_execution` criterion
- `.kiro-krew/evals/rubrics/builder.yaml` - Contains `code_correctness` and `test_coverage` criteria
- `.kiro-krew/evals/cases/*/case-1.yaml` - Test case examples for validation

## Team Orchestration

This is a self-contained implementation task requiring:

1. **Single Developer**: Implement all checker logic in one cohesive update
2. **Testing**: Create comprehensive unit tests covering various scenarios
3. **Validation**: Run existing evaluation suite to ensure no regressions

No cross-team dependencies or coordination required.

## Step-by-Step Task Breakdown

### Task 1: Implement `acceptance_criteria_testability` Checker

**Acceptance Criteria**:
- Detects presence of testable acceptance criteria in planner output
- Identifies specific patterns: `- [ ]` checkboxes, testable assertions, measurable outcomes
- Awards partial credit based on testability indicators found
- Handles cases with no criteria gracefully

**Implementation Details**:
- Search for checkbox patterns (`- [ ]`, `- [x]`)
- Look for action verbs indicating testability ("should", "must", "returns", "accepts", "validates")
- Count measurable criteria vs. vague statements
- Score based on percentage of testable vs. total criteria

### Task 2: Implement `test_execution` Checker

**Acceptance Criteria**:
- Detects evidence that validation commands were actually executed
- Identifies command output, exit codes, test results
- Awards credit for documented command execution with results
- Distinguishes between commands mentioned vs. commands run

**Implementation Details**:
- Search for command execution indicators ("`exit code`", "`✓`", "`PASS`", "`FAIL`")
- Look for actual command output patterns (console output, file paths verified)
- Identify result reporting ("Commands Run:", "Build passes:", "Tests pass:")
- Score based on evidence of actual execution vs. theoretical validation

### Task 3: Implement `code_correctness` Checker

**Acceptance Criteria**:
- Detects evidence of code compilation/execution success
- Identifies build success indicators and error absence
- Awards credit for demonstrated working code
- Handles various language build patterns (Go, TypeScript, etc.)

**Implementation Details**:
- Search for build success patterns ("`go build`", "`npm run build`", "`✓`")
- Look for compilation success messages and absence of error indicators
- Identify explicit success statements ("Build passes", "Compiles successfully")
- Score based on build evidence and absence of error patterns

### Task 4: Implement `test_coverage` Checker

**Acceptance Criteria**:
- Detects presence of test implementations in builder output
- Identifies test files, test functions, and testing patterns
- Awards credit based on test coverage indicators
- Handles different testing frameworks and patterns

**Implementation Details**:
- Search for test file patterns (`*_test.go`, `*.test.js`, `*.spec.ts`)
- Look for test function indicators ("`func Test`", "`it(`", "`describe(`")
- Identify testing framework usage and test execution results
- Score based on test presence and execution evidence

### Task 5: Create Comprehensive Unit Tests

**Acceptance Criteria**:
- Test all four new checker implementations
- Cover positive, negative, and edge cases
- Verify scoring logic and reasoning output
- Ensure no regressions in existing checkers

**Test Cases to Implement**:
- `TestScoreDeterministic_AcceptanceCriteriaTestability`
- `TestScoreDeterministic_TestExecution`
- `TestScoreDeterministic_CodeCorrectness`
- `TestScoreDeterministic_TestCoverage`
- Integration tests with real test case data

### Task 6: Integration Testing and Validation

**Acceptance Criteria**:
- Existing evaluation suite runs without errors
- All deterministic criteria return actual scores instead of being skipped
- No false positives or negatives in scoring
- Performance remains acceptable

## Implementation Patterns

### Checker Structure Template

Each checker should follow this pattern:

```go
case strings.Contains(criterion.Name, "criterion_name"):
    // 1. Extract relevant content from tc.Output
    // 2. Apply scoring heuristics
    // 3. Calculate proportional score
    // 4. Return score, reasoning, skipped=false
```

### Scoring Guidelines

- Use proportional scoring: `(found * maxScore) / total`
- Ensure minimum score of 1 for partial credit when appropriate
- Provide detailed reasoning strings for debugging
- Return `skipped=false` for all implemented checkers

### Content Analysis Patterns

- Use case-insensitive matching where appropriate
- Look for multiple indicators to improve accuracy
- Weight different patterns based on reliability
- Handle various output formats robustly

## Validation Commands

### Unit Testing
```bash
go test ./internal/eval/... -v
```

### Integration Testing
```bash
# Run evaluation on existing test cases
go run ./cmd/kiro-krew eval

# Verify no criteria are skipped
grep -r "skipped.*true" .kiro-krew/evals/results/*/
```

### Regression Testing  
```bash
# Compare results before/after implementation
go run ./cmd/kiro-krew eval > before.txt
# (implement changes)
go run ./cmd/kiro-krew eval > after.txt
diff before.txt after.txt
```

### Performance Validation
```bash
time go run ./cmd/kiro-krew eval
```

## Success Metrics

- All 4 deterministic criteria implemented and functional
- Zero skipped deterministic criteria in evaluation results
- Unit test coverage >80% for new checker code
- No performance degradation >20% in evaluation runtime
- All existing functionality preserved

## Risk Mitigation

- **False Positives**: Use conservative scoring and multiple verification patterns
- **Performance**: Keep pattern matching efficient, avoid expensive operations
- **Maintainability**: Document scoring logic clearly, use consistent patterns
- **Compatibility**: Ensure backward compatibility with existing evaluation data

## Future Extensibility

The checker implementation should support:
- Easy addition of new deterministic criteria
- Configurable scoring weights
- Language-specific build/test pattern recognition
- Custom scoring logic per criterion type
