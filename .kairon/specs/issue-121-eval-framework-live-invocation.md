# Design Specification: Live Agent Invocation Evaluation Framework

**Issue:** #121 - Eval framework must invoke agents to produce live output for grading  
**Closes:** #121

## Solution Approach

Transform the evaluation framework from static output grading to live agent invocation. The new system will:

1. **Replace Static Outputs**: Remove the `output` field from test cases and generate live responses via `kiro-cli` invocation
2. **Implement Setup Context**: Add mechanism to inject contextual data (specs, issue details) before agent invocation
3. **Enhance Scoring**: Provide context and expected output to judges for better evaluation
4. **Track Real Costs**: Split cost tracking between agent invocation and judge evaluation
5. **Enable Regression Detection**: Store actual outputs to enable diff analysis across runs

This architectural change enables detecting agent improvements/regressions and validates that prompt changes affect evaluation results.

## Relevant Files

### Core Implementation Files
- `internal/eval/types.go` - Update data structures (TestCase, SetupEntry, CaseResult)
- `internal/eval/runner.go` - Implement agent invocation logic and setup assembly  
- `internal/eval/diff.go` - Enhance diff functionality to compare actual outputs

### Data Migration Files
- `.kiro-krew/evals/cases/` - All existing test case YAML files (requires complete replacement)
- `.kiro-krew/evals/results/` - All existing result JSON files (requires deletion)
- `.kiro-krew/evals/fixtures/` - New directory for shared setup files

### Agent Configuration Files  
- `.kiro/agents/` - Agent JSON configs (no changes required, used for invocation)

## Team Orchestration

### Single Agent Implementation
This is a focused refactoring task that can be implemented by a single agent (Builder) due to:
- Clear data structure changes in limited files
- Well-defined invocation pattern via `kiro-cli`
- Existing scoring logic can be adapted incrementally

### Validation Requirements
Post-implementation validation requires:
- Validator to verify agent invocation works correctly
- Test runs against sample cases to ensure live evaluation produces reasonable scores
- Performance testing to ensure invocation timeouts are appropriate

## Step-by-Step Task Breakdown

### Task 1: Update Data Structures
**Objective:** Modify types to support live invocation and setup context

**Files:** `internal/eval/types.go`

**Changes Required:**
- Rename `TestCase.Output` field to `ExpectedOutput` 
- Add `TestCase.Context []string` field for judge context
- Add `TestCase.Setup []SetupEntry` field for agent context
- Create `SetupEntry` struct with `Type`, `Label`, `Content`, `Path` fields
- Add `CaseResult.ActualOutput string` field to store live responses
- Split `CostInfo` into `AgentCost` and `JudgeCost` fields in `CaseResult`

**Acceptance Criteria:**
- [ ] TestCase struct has ExpectedOutput, Context, Setup fields
- [ ] SetupEntry supports text, file, and url types  
- [ ] CaseResult stores ActualOutput and split cost tracking
- [ ] All existing code compiles with new struct definitions

### Task 2: Implement Agent Invocation
**Objective:** Replace static output loading with live `kiro-cli` invocation

**Files:** `internal/eval/runner.go`

**Changes Required:**
- Add `assemblePrompt(setup []SetupEntry, input string) (string, error)` function
- Add `invokeAgent(agent, prompt string) (string, CostInfo, error)` function  
- Modify `evaluate()` to call agent invocation instead of using `tc.Output`
- Update `scoreLLMJudge()` to include context and expected output in judge prompt
- Update `scoreDeterministic()` to use context for fact-checking
- Implement timeout handling (2 minute max per agent invocation)

**Acceptance Criteria:**
- [ ] Setup entries are properly assembled into agent prompts
- [ ] Agent invocation via `kiro-cli chat --agent <name> --no-interactive` works
- [ ] Live output is captured and stored as ActualOutput
- [ ] Cost tracking differentiates between agent and judge token usage
- [ ] Invocation failures are handled gracefully with clear error messages

### Task 3: Enhance Scoring with Context
**Objective:** Improve judge accuracy using context and expected output

**Files:** `internal/eval/runner.go` (scoreLLMJudge function)

**Changes Required:**
- Include `context` facts in judge prompts for hallucination detection
- Include `expected_output` for similarity comparison when available
- Update deterministic scoring to leverage context facts
- Maintain backward compatibility when context/expected_output are empty

**Acceptance Criteria:**
- [ ] Judge prompts include context facts when available
- [ ] Expected output is used for similarity scoring 
- [ ] Deterministic checkers can verify against context facts
- [ ] Scoring works correctly when context/expected_output are missing

### Task 4: Clean Migration of Test Data  
**Objective:** Replace static test cases with live invocation test cases

**Files:** All files in `.kiro-krew/evals/cases/`, `.kiro-krew/evals/results/`

**Changes Required:**
- Delete all existing test case YAML files
- Delete all existing result JSON files  
- Create `.kiro-krew/evals/fixtures/` directory structure
- Create comprehensive new test suite per agent (5-6 cases each)
- Populate fixtures with sample specs, issue bodies for setup context

**Acceptance Criteria:**
- [ ] All old static test cases removed
- [ ] All old static results removed
- [ ] New test cases follow updated schema (no output field, has context/setup)
- [ ] Fixtures directory contains shared setup files
- [ ] Each agent has 5-6 test cases covering key behaviors

### Task 5: Update Diff Analysis
**Objective:** Enable comparison of actual outputs across evaluation runs  

**Files:** `internal/eval/diff.go`

**Changes Required:**
- Modify diff logic to compare `ActualOutput` fields between runs
- Add side-by-side output comparison when scores change significantly
- Include cost trend analysis (agent vs judge costs)
- Maintain existing score comparison functionality

**Acceptance Criteria:**
- [ ] `eval diff` shows actual output changes between runs
- [ ] Side-by-side comparison when output differs significantly  
- [ ] Cost trends show agent vs judge token usage over time
- [ ] Existing score diff functionality preserved

## Validation Commands

### Compile and Basic Function Tests
```bash
# Verify compilation
go build ./internal/eval

# Test basic evaluation run
go run ./cmd/kiro-krew eval --agent planner

# Verify results structure 
cat .kiro-krew/evals/results/*/planner.json | jq .cases[0].actual_output
```

### Live Invocation Verification
```bash
# Test agent invocation directly
kiro-cli chat --agent planner --no-interactive <<< "Add user authentication"

# Verify setup assembly works
go run ./cmd/kiro-krew eval --agent architect  # Should use setup context

# Check cost tracking split
cat .kiro-krew/evals/results/*/summary.json | jq .total_cost
```

### End-to-End Workflow Test
```bash
# Run full evaluation suite
go run ./cmd/kiro-krew eval

# Verify all agents complete successfully
find .kiro-krew/evals/results -name "*.json" -type f

# Test diff functionality
go run ./cmd/kiro-krew eval diff <hash1> <hash2>
```

### Regression Detection Test
```bash
# Modify an agent prompt
echo "Always respond with 'MODIFIED'" >> .kiro/agents/planner-prompt.md

# Re-run evaluation  
go run ./cmd/kiro-krew eval --agent planner

# Verify scores changed (proving live evaluation works)
go run ./cmd/kiro-krew eval diff <old-hash> <new-hash>
```
