package plan

import (
	"strings"
	"testing"
)

// TestCompatibility_LegacySpecWithoutPlan verifies specs without plan artifacts are handled gracefully
func TestCompatibility_LegacySpecWithoutPlan(t *testing.T) {
	// Legacy spec format - no plan artifact
	legacySpec := `# Design Specification: Legacy Feature

## Solution Approach
Implement feature using traditional sequential workflow.

## Implementation Details
The architect will delegate to builder sequentially, then validator will verify.

## Files to Create
- internal/feature/handler.go
- internal/feature/handler_test.go

## Files to Modify
- internal/router/routes.go

## Validation Commands
` + "```bash" + `
go test ./internal/feature/...
go test ./...
` + "```" + `
`

	// Parse plan from legacy spec
	plan, err := ParsePlanFromMarkdown([]byte(legacySpec))

	// Should not error - absence of plan is not an error
	if err != nil {
		t.Errorf("ParsePlanFromMarkdown should not error on legacy spec, got: %v", err)
	}

	// Should return nil plan to signal fallback to legacy workflow
	if plan != nil {
		t.Error("Expected nil plan for legacy spec, got non-nil plan")
	}
}

// TestCompatibility_SpecWithNonPlanCodeBlocks verifies other code blocks don't interfere
func TestCompatibility_SpecWithNonPlanCodeBlocks(t *testing.T) {
	spec := `# Design Specification

## Solution Approach
Use legacy workflow.

## Example Code

` + "```go" + `
func Example() {
    fmt.Println("Not a plan")
}
` + "```" + `

## Configuration

` + "```yaml" + `
config:
  setting: value
` + "```" + `

## Commands

` + "```bash" + `
go test ./...
` + "```" + `
`

	plan, err := ParsePlanFromMarkdown([]byte(spec))

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if plan != nil {
		t.Error("Expected nil plan when no kiro-plan block exists")
	}
}

// TestCompatibility_MixedPlanAndLegacyContent verifies plan-based specs can coexist with legacy content
func TestCompatibility_MixedPlanAndLegacyContent(t *testing.T) {
	spec := `# Design Specification: Mixed Format

## Solution Approach
This spec has both plan artifact and traditional sections.

## Machine-Readable Plan

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Implement feature"
    dependencies: []
    acceptance_criteria:
      - "Tests pass"
    validation_commands:
      - "go test ./..."
` + "```" + `

## Implementation Details
Additional details here...

## Files to Create
- some/file.go

## Traditional Validation

` + "```bash" + `
go test ./...
` + "```" + `
`

	plan, err := ParsePlanFromMarkdown([]byte(spec))

	if err != nil {
		t.Fatalf("ParsePlanFromMarkmark failed: %v", err)
	}

	if plan == nil {
		t.Fatal("Expected plan to be parsed from kiro-plan block")
	}

	// Verify plan was parsed correctly
	if plan.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", plan.Version)
	}

	if len(plan.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(plan.Tasks))
	}

	if plan.Tasks[0].ID != "task-1" {
		t.Errorf("Expected task-1, got %s", plan.Tasks[0].ID)
	}
}

// TestCompatibility_ParserRobustness tests parser handles various edge cases
func TestCompatibility_ParserRobustness(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantPlan  bool
		wantError bool
	}{
		{
			name:      "empty content",
			content:   "",
			wantPlan:  false,
			wantError: false,
		},
		{
			name:      "only whitespace",
			content:   "   \n\n\t\n   ",
			wantPlan:  false,
			wantError: false,
		},
		{
			name:      "no markdown structure",
			content:   "Just plain text without any structure",
			wantPlan:  false,
			wantError: false,
		},
		{
			name:      "kiro-plan in inline code",
			content:   "This mentions `kiro-plan` in inline code but has no block",
			wantPlan:  false,
			wantError: false,
		},
		{
			name:      "kiro-plan as heading",
			content:   "# kiro-plan\nThis is a heading, not a code block",
			wantPlan:  false,
			wantError: false,
		},
		{
			name:      "empty kiro-plan block",
			content:   "```kiro-plan\n```",
			wantPlan:  false,
			wantError: true, // Empty YAML should error
		},
		{
			name:      "whitespace-only kiro-plan block",
			content:   "```kiro-plan\n   \n\t\n```",
			wantPlan:  false,
			wantError: true, // Whitespace-only should error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ParsePlanFromMarkdown([]byte(tt.content))

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			if tt.wantPlan && plan == nil {
				t.Error("Expected plan, got nil")
			}

			if !tt.wantPlan && plan != nil {
				t.Error("Expected nil plan, got non-nil")
			}
		})
	}
}

// TestCompatibility_ValidationBackwardCompatibility verifies validation logic
func TestCompatibility_ValidationBackwardCompatibility(t *testing.T) {
	// Create a validator with known agents
	registry := &AgentRegistry{
		agents: map[string]string{
			"architect":  ".kiro/agents/architect.json",
			"builder":    ".kiro/agents/builder.json",
			"validator":  ".kiro/agents/validator.json",
			"documenter": ".kiro/agents/documenter.json",
		},
	}

	validator := NewValidator(registry)

	// Test that validation works with known agents
	validPlan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "Build something",
				Dependencies:       []string{},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
			{
				ID:                 "task-2",
				Agent:              "validator",
				Description:        "Validate something",
				Dependencies:       []string{"task-1"},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
		},
	}

	if err := validator.ValidatePlan(validPlan); err != nil {
		t.Errorf("Valid plan should pass validation, got error: %v", err)
	}

	// Test that validation rejects unknown agents
	invalidPlan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "unknown-agent",
				Description:  "Use unknown agent",
				Dependencies: []string{},
			},
		},
	}

	err := validator.ValidatePlan(invalidPlan)
	if err == nil {
		t.Error("Expected validation error for unknown agent")
	}

	if !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("Expected 'unknown agent' error, got: %v", err)
	}
}

// TestCompatibility_ExecutorEmptyPlan verifies executor handles edge cases
func TestCompatibility_ExecutorEmptyPlan(t *testing.T) {
	spawner := func(task Task) (int, error) {
		return 0, nil
	}

	executor := NewExecutor(spawner)

	// Test nil plan
	t.Run("nil plan", func(t *testing.T) {
		summary, err := executor.ExecutePlan(nil)
		if err == nil {
			t.Error("Expected error for nil plan")
		}
		if summary != nil {
			t.Error("Expected nil summary for nil plan")
		}
	})

	// Test empty plan (no tasks)
	t.Run("empty plan", func(t *testing.T) {
		emptyPlan := &Plan{
			Version: "1.0",
			Tasks:   []Task{},
		}

		summary, err := executor.ExecutePlan(emptyPlan)
		if err != nil {
			t.Errorf("Empty plan should not error, got: %v", err)
		}

		if summary == nil {
			t.Fatal("Expected summary, got nil")
		}

		if summary.TotalTasks != 0 {
			t.Errorf("Expected 0 total tasks, got %d", summary.TotalTasks)
		}

		if len(summary.Results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(summary.Results))
		}
	})
}

// TestCompatibility_LegacyWorkflowSimulation simulates legacy workflow without plans
func TestCompatibility_LegacyWorkflowSimulation(t *testing.T) {
	// Simulate the legacy workflow detection logic
	legacySpec := `# Design Specification

Implement feature X without a plan artifact.

## Implementation Details
Builder will implement sequentially.
`

	// Parse spec
	plan, err := ParsePlanFromMarkdown([]byte(legacySpec))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Simulate krew-lead logic
	var workflowType string
	if plan == nil {
		workflowType = "legacy-sequential"
		t.Log("Detected legacy workflow - no plan artifact found")
	} else {
		workflowType = "plan-based"
		t.Log("Detected plan-based workflow")
	}

	// Verify correct workflow type detected
	if workflowType != "legacy-sequential" {
		t.Errorf("Expected legacy-sequential workflow, got %s", workflowType)
	}
}

// TestCompatibility_PlanBasedWorkflowSimulation simulates plan-based workflow
func TestCompatibility_PlanBasedWorkflowSimulation(t *testing.T) {
	// Simulate the plan-based workflow detection logic
	planSpec := `# Design Specification

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Implement feature"
    dependencies: []
    acceptance_criteria:
      - "Tests pass"
    validation_commands:
      - "go test ./..."
` + "```" + `
`

	// Parse spec
	plan, err := ParsePlanFromMarkdown([]byte(planSpec))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Simulate krew-lead logic
	var workflowType string
	if plan == nil {
		workflowType = "legacy-sequential"
	} else {
		workflowType = "plan-based"
		t.Logf("Detected plan-based workflow with %d tasks", len(plan.Tasks))
	}

	// Verify correct workflow type detected
	if workflowType != "plan-based" {
		t.Errorf("Expected plan-based workflow, got %s", workflowType)
	}

	// Verify plan parsed correctly
	if plan == nil {
		t.Fatal("Expected plan, got nil")
	}

	if len(plan.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(plan.Tasks))
	}
}

// TestCompatibility_RegistryWithNoAgents verifies handling of empty agent directory
func TestCompatibility_RegistryWithNoAgents(t *testing.T) {
	// Create registry from non-existent directory
	registry, err := DiscoverAgents("/nonexistent/path")

	// Should not panic, should return empty registry or error
	if err == nil && registry != nil && len(registry.agents) > 0 {
		t.Error("Expected empty registry or error for non-existent path")
	}
}

// TestCompatibility_SchemaValidationErrors verifies error messages are actionable
func TestCompatibility_SchemaValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		plan           *Plan
		expectedErrMsg string
	}{
		{
			name: "invalid version",
			plan: &Plan{
				Version: "2.0",
				Tasks: []Task{
					{ID: "task-1", Agent: "builder", Description: "Test"},
				},
			},
			expectedErrMsg: "invalid plan version",
		},
		{
			name: "duplicate task ID",
			plan: &Plan{
				Version: "1.0",
				Tasks: []Task{
					{ID: "task-1", Agent: "builder", Description: "Test 1", AcceptanceCriteria: []string{"Done"}, ValidationCommands: []string{"go test"}},
					{ID: "task-1", Agent: "builder", Description: "Test 2", AcceptanceCriteria: []string{"Done"}, ValidationCommands: []string{"go test"}},
				},
			},
			expectedErrMsg: "duplicate task ID",
		},
		{
			name: "missing agent",
			plan: &Plan{
				Version: "1.0",
				Tasks: []Task{
					{ID: "task-1", Agent: "", Description: "Test"},
				},
			},
			expectedErrMsg: "missing agent",
		},
		{
			name: "empty plan",
			plan: &Plan{
				Version: "1.0",
				Tasks:   []Task{},
			},
			expectedErrMsg: "plan has no tasks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plan.Validate()
			if err == nil {
				t.Fatal("Expected validation error, got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectedErrMsg, err)
			}
		})
	}
}

// TestCompatibility_TopologicalSortStability verifies consistent ordering
func TestCompatibility_TopologicalSortStability(t *testing.T) {
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{ID: "task-c", Agent: "builder", Description: "C", Dependencies: []string{"task-a", "task-b"}},
			{ID: "task-a", Agent: "builder", Description: "A", Dependencies: []string{}},
			{ID: "task-b", Agent: "builder", Description: "B", Dependencies: []string{}},
		},
	}

	// Run topological sort multiple times
	for i := 0; i < 10; i++ {
		layers, err := TopologicalSort(plan)
		if err != nil {
			t.Fatalf("TopologicalSort failed: %v", err)
		}

		// Verify structure: 2 layers
		if len(layers) != 2 {
			t.Errorf("Expected 2 layers, got %d", len(layers))
		}

		// Layer 0 should contain task-a and task-b
		if len(layers[0]) != 2 {
			t.Errorf("Layer 0: expected 2 tasks, got %d", len(layers[0]))
		}

		// Layer 0 must be in a stable, deterministic order (sorted by task ID)
		if layers[0][0].ID != "task-a" || layers[0][1].ID != "task-b" {
			t.Errorf("Layer 0: expected exact order [task-a, task-b], got [%s, %s]",
				layers[0][0].ID, layers[0][1].ID)
		}

		// Layer 1 should contain task-c
		if len(layers[1]) != 1 {
			t.Errorf("Layer 1: expected 1 task, got %d", len(layers[1]))
		}

		if layers[1][0].ID != "task-c" {
			t.Errorf("Layer 1: expected task-c, got %s", layers[1][0].ID)
		}
	}
}
