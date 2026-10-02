# Design Specification: Improve Planner Agent Question Clarity and Add Evaluation Workflow

**Issue:** #107  
**Closes:** #107

## Solution Approach

This issue addresses two critical problems with the planner agent:

1. **Question Ambiguity**: Users get confused when presented with ambiguous yes/no questions or multiple questions in one response
2. **Missing Evaluation Process**: No systematic process exists to validate prompt changes before deployment

The solution involves updating the planner agent prompt to enforce clear question patterns and establishing an evaluation workflow for all prompt modifications.

## Relevant Files

### Files to Modify
- `.kiro/agents/planner-prompt.md` - Update prompt with clear questioning guidelines
- `.kiro-krew/evals/rubrics/planner.yaml` - Add new evaluation criteria for question clarity
- `.kiro-krew/evals/cases/planner/` - Add new test cases for problematic question patterns

### Files to Create
- `.kiro-krew/evals/cases/planner/case-ambiguous-questions.yaml` - Test case for ambiguous question detection
- `.kiro-krew/evals/cases/planner/case-multiple-questions.yaml` - Test case for multiple question violations
- `.kiro-krew/evals/cases/planner/case-option-selection.yaml` - Test case for proper option presentation

### Files Referenced for Context
- `internal/session/planner.go` - Planner session management (no changes needed)
- `cmd/kiro-krew/cmd/eval.go` - Evaluation command implementation (no changes needed)
- `internal/eval/runner.go` - Evaluation execution logic (no changes needed)

## Team Orchestration

This is a single-developer task requiring:
1. Prompt engineering expertise to eliminate ambiguous questioning patterns
2. Evaluation framework knowledge to create effective test cases
3. Understanding of agent behavioral requirements

No coordination with external teams is required as this is an internal agent improvement.

## Step-by-Step Task Breakdown

### Task 1: Update Planner Agent Prompt
**Acceptance Criteria:**
- [ ] Add explicit prohibition against multiple questions per response
- [ ] Add structured guidelines for option presentation (a, b, c format with "other" option)
- [ ] Add examples of problematic vs. improved question patterns
- [ ] Maintain existing workflow and collaborative nature
- [ ] Preserve all current functionality and restrictions

**Implementation Notes:**
- Add "Question Format Rules" section to planner-prompt.md
- Include specific examples from the issue description
- Add instruction to wait for response before asking follow-up questions

### Task 2: Enhance Evaluation Rubric
**Acceptance Criteria:**
- [ ] Add `question_clarity` criterion to planner rubric (1-5 scoring)
- [ ] Add `single_question_adherence` deterministic criterion
- [ ] Maintain existing criteria (requirement_clarity, scope_appropriateness, etc.)
- [ ] Update criterion descriptions for clarity

**Implementation Notes:**
- Add deterministic check for multiple question marks in single response
- Score question_clarity based on ambiguity detection patterns

### Task 3: Create Adversarial Test Cases
**Acceptance Criteria:**
- [ ] Create test case that triggers ambiguous yes/no questions
- [ ] Create test case that attempts to elicit multiple questions per response
- [ ] Create test case for proper option selection format
- [ ] Each test case includes input that would historically cause problems
- [ ] Test cases cover edge cases (complex requirements, unclear user input)

**Implementation Notes:**
- Use input patterns known to cause problematic agent behavior
- Include expected output demonstrating correct question handling

### Task 4: Implement Evaluation Workflow Documentation
**Acceptance Criteria:**
- [ ] Document requirement to run `kiro-krew eval` before prompt changes (baseline)
- [ ] Document requirement to run `kiro-krew eval` after prompt changes (verification)
- [ ] Add guidance on creating test cases for specific behavioral changes
- [ ] Include evaluation workflow in development process documentation

**Implementation Notes:**
- Update existing documentation or create new process guide
- Establish evaluation as equivalent to unit testing for prompt engineering

### Task 5: Validation and Testing
**Acceptance Criteria:**
- [ ] Run baseline evaluation before any changes
- [ ] Apply prompt improvements
- [ ] Run post-change evaluation to verify improvements
- [ ] Demonstrate improved scores on question_clarity criteria
- [ ] Verify no regression on existing criteria

## Validation Commands

```bash
# Baseline evaluation before changes
kiro-krew eval planner

# Test specific problematic patterns
kiro-krew eval planner case-ambiguous-questions
kiro-krew eval planner case-multiple-questions

# Full evaluation after changes
kiro-krew eval planner

# Compare baseline vs. improved results
kiro-krew eval diff <baseline-hash> <improved-hash>

# Test planner agent with sample input to verify behavior
echo "Add user authentication" | kiro-cli chat --agent planner --no-interactive
```

## Risk Mitigation

- **Risk**: Prompt changes may break existing functionality
  - **Mitigation**: Comprehensive evaluation before/after with comparison
- **Risk**: Over-constraining questions may reduce planner effectiveness
  - **Mitigation**: Maintain collaborative workflow, only eliminate ambiguity
- **Risk**: New test cases may not catch all problematic patterns
  - **Mitigation**: Include adversarial cases designed to trigger known issues

## Success Metrics

1. **Zero ambiguous yes/no questions** when presenting options
2. **Single question per response** enforced consistently
3. **Improved evaluation scores** on question_clarity criteria
4. **No regression** on existing planner functionality
5. **Documented evaluation workflow** for future prompt changes
