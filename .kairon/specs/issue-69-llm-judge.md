# LLM-as-a-Judge Implementation Design Specification

**Issue:** #69 - Implement LLM-as-a-Judge functionality for eval framework  
**Closes:** #69

## Problem Analysis

The kiro-krew eval framework currently has a stub `scoreLLMJudge` function that returns skipped results for all non-deterministic criteria. This prevents evaluation of subjective criteria like "code quality", "spec adherence", and "requirement clarity" that require LLM judgment rather than deterministic checks.

**Current State:**
- `scoreLLMJudge()` in `internal/eval/runner.go` line 118 always returns `(0, "LLM judge not configured — criterion skipped", true)`
- Rubrics contain both deterministic and LLM-judged criteria
- Framework supports 1-5 scoring scale that needs normalization to 0-1 internally
- No integration with kiro CLI's chat functionality

## Solution Approach

### Architecture Strategy
1. **Replace stub with working LLM judge:** Implement `scoreLLMJudge` using kiro CLI chat mode
2. **JSON-structured responses:** Follow Promptfoo's llm-rubric pattern with `{score, reasoning, pass}` structure
3. **Scale normalization:** Accept 1-5 input from rubrics, normalize to 0-1 for internal processing
4. **Error handling:** Graceful fallbacks with appropriate reasoning when LLM calls fail
5. **Future-ready:** Include TODO placeholders for chain-of-thought evaluation enhancements

### Integration Pattern
- Route all LLM judge calls through `kiro chat` for consistency
- Use structured prompts that request JSON responses
- Parse and validate JSON responses with fallback to score extraction from text
- Maintain existing evaluation framework interfaces without breaking changes

## Relevant Files

### Files to Modify
- `internal/eval/runner.go` - Replace `scoreLLMJudge` stub with working implementation
- `internal/eval/types.go` - Add LLM response structures if needed

### Files to Reference  
- `.kiro-krew/evals/rubrics/*.yaml` - Existing rubric structure with LLM-judged criteria
- `.kiro-krew/evals/cases/*/` - Test cases that provide context for LLM judging

### Files Unchanged
- All deterministic evaluation logic remains intact
- Existing rubric YAML format preserved
- Test case loading and result output structures unchanged

## Team Orchestration

**Single Implementation Focus:** This is a targeted enhancement to one function that integrates with existing kiro CLI infrastructure. No multi-team coordination required.

**Integration Points:**
- Must work with existing eval framework without breaking deterministic evaluations
- Should follow established patterns from libraries like Promptfoo for LLM judging
- Must maintain compatibility with current agent rubrics

## Step-by-Step Task Breakdown

### Task 1: Implement Core LLM Judge Function
**Acceptance Criteria:**
- Replace `scoreLLMJudge` stub with working implementation
- Function signature matches existing interface: `(criterion Criterion, tc TestCase) (int, string, bool)`
- Routes calls through `kiro chat` command for consistency
- Returns proper error handling when LLM unavailable

**Implementation Steps:**
1. Create LLM prompt template incorporating criterion description and test case context
2. Execute `kiro chat` with structured JSON request
3. Parse JSON response to extract score, reasoning, and pass/fail
4. Implement fallback parsing for non-JSON responses
5. Add error handling for command failures

### Task 2: JSON Response Structure Implementation  
**Acceptance Criteria:**
- Follows Promptfoo llm-rubric pattern: `{"score": number, "reasoning": string, "pass": boolean}`
- Handles 1-5 scale input from rubrics, normalizes appropriately
- Graceful parsing with fallbacks when JSON is malformed
- Reasoning field populated with LLM's evaluation explanation

**Implementation Steps:**
1. Define JSON response structure matching Promptfoo pattern
2. Create prompt that requests specific JSON format
3. Implement JSON parsing with error recovery
4. Handle score normalization (1-5 scale to internal representation)
5. Extract pass/fail logic based on score thresholds

### Task 3: Error Handling and Fallback Strategy
**Acceptance Criteria:**
- Graceful handling when kiro CLI unavailable or fails
- Appropriate reasoning messages for different failure modes
- Skipped flag set correctly for failures vs. actual scores
- No crashes or panics under error conditions

**Implementation Steps:**
1. Add timeout handling for kiro chat commands
2. Implement fallback score extraction from plain text responses
3. Create descriptive error messages for different failure types
4. Add logging for debugging LLM judge issues
5. Test error scenarios (command not found, malformed JSON, etc.)

### Task 4: Chain-of-Thought Preparation
**Acceptance Criteria:**
- TODO placeholders added for future chain-of-thought evaluation
- Current implementation structured to easily add CoT enhancement
- Documentation outlines where CoT would be integrated
- No functional changes, just structural preparation

**Implementation Steps:**
1. Add TODO comments indicating CoT integration points
2. Structure prompt building to easily add CoT steps
3. Document CoT enhancement approach in code comments
4. Ensure current implementation won't conflict with future CoT addition

### Task 5: Integration Testing
**Acceptance Criteria:**
- LLM judge works with all existing rubric criteria
- Deterministic evaluations continue working unchanged  
- Results properly formatted in existing JSON output structure
- Performance acceptable for typical rubric evaluation runs

**Implementation Steps:**
1. Test with each rubric's LLM-judged criteria
2. Verify deterministic criteria still work correctly
3. Check JSON output format matches existing structure
4. Performance test with multiple criteria and cases
5. Validate error handling with various failure scenarios

## Validation Commands

### Basic Functionality Test
```bash
# Run eval on single agent to test LLM judge integration
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval architect
```

### Full Integration Test  
```bash
# Run complete evaluation to ensure no regression
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval
```

### Error Handling Test
```bash
# Test with kiro CLI unavailable (rename binary temporarily)
mv $(which kiro) $(which kiro).bak && cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval planner
mv $(which kiro).bak $(which kiro)
```

### Response Format Verification
```bash
# Check that results maintain expected JSON structure
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval architect
jq '.Cases[0].Scores[] | select(.Deterministic == false)' evals/results/*/architect.json
```

### Scoring Scale Validation
```bash
# Verify 1-5 scale scoring works correctly with normalization
cd .kiro-krew && go run ../cmd/kiro-krew/main.go eval planner
# Check that LLM scores are in valid range and normalized properly
```

## Implementation Notes

**Prompt Design:** Follow Promptfoo's established patterns for LLM rubric evaluation, requesting specific JSON format and providing clear evaluation context.

**Command Execution:** Use `exec.Command` to call `kiro chat` with appropriate arguments and timeout handling.

**JSON Parsing:** Use Go's standard `encoding/json` with custom unmarshaling for robust response handling.

**Backward Compatibility:** Ensure all existing deterministic evaluation logic continues working unchanged.

**Future Enhancements:** Structure implementation to easily add chain-of-thought reasoning, multi-step evaluation, and other advanced LLM judging techniques.
