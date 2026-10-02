# Fix Evaluation Framework Silent Failures

**Issue**: #123  
**Title**: fix: eval framework creates empty results folders with no logging output

## Problem Analysis

The evaluation framework in `internal/eval/runner.go` creates timestamped result directories but fails to populate them with evaluation results. The command runs silently without progress indicators, making debugging impossible.

### Root Causes Identified

1. **Silent Error Handling**: The `Run()` function uses `fmt.Fprintf(os.Stderr, "Warning: ...")` for errors but continues execution, leading to empty result folders with only summary.json containing empty data structures.

2. **Missing Console Feedback**: No progress indicators during evaluation execution, so users can't tell if the process is working or hanging.

3. **Inadequate Error Propagation**: When `loadCases()` fails or agent invocation fails, warnings are logged but execution continues, resulting in empty results.

4. **No Validation of Prerequisites**: The system doesn't verify that rubrics and test cases exist before starting evaluation.

## Solution Approach

Implement comprehensive logging and error handling to provide clear feedback during evaluation execution while maintaining backwards compatibility with existing evaluation data structures.

## Relevant Files

### Files to Modify
- `internal/eval/runner.go` - Add progress logging and improve error handling
- `cmd/kiro-krew/cmd/eval.go` - Add validation and user feedback

### Files Referenced
- `.kiro-krew/evals/rubrics/*.yaml` - Rubric definitions (read-only)
- `.kiro-krew/evals/cases/*/` - Test case directories (read-only)
- `.kiro-krew/evals/results/` - Output directory structure

## Team Orchestration

This is a single-component fix focused on the evaluation runner with no cross-agent dependencies. The builder agent will implement all changes in sequence.

## Step-by-Step Task Breakdown

### Task 1: Add Progress Logging to Run Function
**Acceptance Criteria:**
- Console shows "Starting evaluation for agent X..." messages
- Progress indicators for each test case execution
- Clear success/failure messages for each step
- Final summary shows total cases processed and results location

### Task 2: Improve Error Handling and Validation
**Acceptance Criteria:**
- Validate that rubrics directory exists before starting
- Validate that test cases exist for each agent before evaluation
- Fatal errors (no rubrics, no kiro-cli) should exit with clear error messages
- Non-fatal errors (missing cases for one agent) should warn but continue with other agents

### Task 3: Add Timeout and Hang Detection
**Acceptance Criteria:**
- Show timeout indicator when kiro-cli commands take >30 seconds
- Allow configurable timeout via environment variable KIRO_KREW_EVAL_TIMEOUT
- Graceful handling of timeout with partial results saved

### Task 4: Enhanced Console Output Format
**Acceptance Criteria:**
- Structured output showing: Agent → Case → Criterion evaluation
- Real-time status updates during long-running evaluations
- Color-coded success/warning/error messages where supported
- Preserve existing JSON output format in result files

## Validation Commands

```bash
# Test basic evaluation with progress output
./kiro-krew eval architect

# Test specific agent evaluation
./kiro-krew eval builder

# Test evaluation with missing components (should show clear errors)
rm -rf .kiro-krew/evals/cases/architect
./kiro-krew eval architect

# Test evaluation with invalid rubric (should show clear errors)
echo "invalid yaml content" > .kiro-krew/evals/rubrics/test.yaml
./kiro-krew eval test

# Verify results are populated (should contain actual data, not empty structures)
cat .kiro-krew/evals/results/*/summary.json | jq '.agent_scores'
```

## Implementation Notes

- Maintain backwards compatibility with existing JSON result format
- Use standard Go logging patterns for consistency
- Add progress dots or spinner for long-running operations
- Ensure error messages guide users toward resolution (e.g., "Run 'kiro-krew init' to set up evaluation framework")

Closes #123