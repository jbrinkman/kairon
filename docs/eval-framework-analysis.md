# Complete Analysis and Recommendations for Kairon Evaluation Framework

## Current State Analysis

### Kairon's Current Framework
Your existing evaluation system is impressively well-designed and practical for your specific use case:

Strengths:
- **Simple, focused test case schema**: name, description, input, output with clear agent assignment
- **Hybrid scoring approach**: Deterministic scoring for objective criteria + LLM-judged scoring for subjective ones
- **Practical rubrics**: Agent-specific criteria like acceptance_criteria_quality, file_reference, test_execution
- **Cost tracking**: Built-in token estimation and cost monitoring
- **Git integration**: Version tracking with commit hashes
- **Results management**: Timestamped results with diff capabilities

Current Limitations:
- No support for conversational/multi-turn test cases
- Limited context injection capabilities
- No tool call evaluation mechanisms
- Basic metadata support for test organization

## Framework Comparison Analysis

### Promptfoo's Valuable Features

Test Case Organization:
- **Provider/prompt filtering**: providers: ["fast-model"] to run specific tests on specific models
- **Rich metadata system**: metadata: {category: math, difficulty: easy} with filtering capabilities
- **External data sources**: CSV/Excel with special columns (__expected, __metadata:*)
- **Dynamic test generation**: JavaScript/Python functions that generate test cases programmatically

Advanced Test Structure:
- **Multiple assertions per test**: __expected1, __expected2, __expected3
- **Test inheritance**: defaultTest configuration applied to all test cases
- **Media file support**: file://images/chart.png with automatic base64 conversion

### DeepEval's Valuable Features

Rich Context Support:
- **Multiple context types**: context, retrieval_context, tools_called, expected_tools
- **Multimodal support**: MLLMImage objects for vision model testing
- **Conversation support**: ConversationalTestCase for multi-turn scenarios
- **Performance tracking**: token_cost, completion_time built into test cases

Advanced Test Case Types:
- **Tool evaluation**: Structured ToolCall objects with reasoning, input/output tracking
- **RAG-specific parameters**: Separate context vs retrieval_context for ground truth vs actual retrieval
- **Labeling system**: name, tags for enterprise-level test management

## Recommended Framework Enhancements

### High-Value Additions (Align with Simplicity Preference)

1. Enhanced Context Support (Similar to DeepEval)
yaml
name: plan-with-context
input: "Add authentication to the API"
context:
  - "Current API has no auth middleware"
  - "User model exists in internal/models/user.go"
  - "JWT library already imported in go.mod"
expected_tools:
  - name: "gh"
    reasoning: "Create GitHub issue"
output: |
  I've created the issue...


2. Metadata and Filtering (From Promptfoo)
yaml
name: plan-complex-feature
metadata:
  category: "authentication"
  difficulty: "medium"
  priority: "high"
input: "Add JWT auth"
output: |
  Created issue with proper scope...


3. Tool Call Evaluation (DeepEval-inspired)
yaml
name: planner-tool-usage
input: "Create issue for new API endpoint"
expected_tools:
  - name: "gh issue create"
    parameters: 
      title: "feat: add user endpoint"
      label: "kairon"
actual_tools:  # Populated by test runner
  - name: "gh issue create"
    parameters:
      title: "feat: add user management endpoint"
      label: "kairon"


4. Multi-Agent Workflow Testing (Your unique need)
yaml
name: end-to-end-workflow
type: "workflow"
agents: ["planner", "architect", "builder", "validator"]
input: "Add status command"
checkpoints:
  - agent: "planner"
    expected_output_contains: "GitHub issue created"
  - agent: "architect"
    expected_files: [".kairon/specs/issue-*.md"]
  - agent: "builder" 
    expected_files_modified: ["internal/tui/commands.go"]


### Lower Priority (Complex but Potentially Valuable)

5. Dynamic Test Generation (From Promptfoo)
yaml
# tests/planner_tests.py
def generate_planner_tests(config):
    scenarios = [
        "Add authentication",
        "Create new API endpoint", 
        "Fix bug in validation"
    ]
    return [create_test_case(s) for s in scenarios]


6. External Test Data Support
yaml
tests: 
  - file://tests/planner_scenarios.csv
  - file://tests/generated_tests.py:create_tests


## Recommended Schema Evolution

### Enhanced TestCase Schema
yaml
# Current + new fields
name: string
description: string
input: string
output: string (optional if testing live)
agent: string

# NEW: Context and constraints
context: []string (optional)
constraints: []string (optional)
metadata:
  category: string
  difficulty: string
  priority: string

# NEW: Tool evaluation
expected_tools: []ToolCall (optional)
actual_tools: []ToolCall (populated by runner)

# NEW: Multi-agent workflow
type: "single" | "workflow" (default: "single")
checkpoints: []Checkpoint (for workflow tests)


### Enhanced Rubric Schema
yaml
agent: string
criteria:
  - name: string
    description: string
    scoring: string
    deterministic: bool
    type: string
    
    # NEW: Context-aware scoring
    requires_context: bool (optional)
    requires_tools: bool (optional)
    
    # NEW: Metadata filtering
    applies_to:
      categories: []string (optional)
      difficulties: []string (optional)


## Implementation Recommendations

### Phase 1: Core Context Support
1. Add context and metadata fields to TestCase
2. Enhance deterministic scoring to use context
3. Add metadata filtering to test runner
4. Update planner rubric to use context-aware criteria

### Phase 2: Tool Evaluation
1. Add ToolCall struct and related fields
2. Implement tool usage tracking in test runner
3. Create tool correctness criteria for planner
4. Add expected_tools vs actual_tools comparison logic

### Phase 3: Advanced Features
1. Multi-agent workflow test support
2. Dynamic test generation capabilities
3. External data source integration (CSV)
4. Enhanced reporting with context/tool usage insights
