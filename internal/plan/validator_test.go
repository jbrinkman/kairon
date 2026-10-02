package plan

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatorWithValidPlan(t *testing.T) {
	// Create test registry
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder":   "builder.json",
		"validator": "validator.json",
	}

	validator := NewValidator(registry)

	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "First task",
				Dependencies:       []string{},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
			{
				ID:                 "task-2",
				Agent:              "builder",
				Description:        "Second task",
				Dependencies:       []string{"task-1"},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err != nil {
		t.Errorf("Expected valid plan to pass validation, got error: %v", err)
	}
}

func TestValidatorWithUnknownAgent(t *testing.T) {
	// Create test registry with only builder
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "unknown-agent",
				Description:        "Task with unknown agent",
				Dependencies:       []string{},
				AcceptanceCriteria: []string{"Done"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err == nil {
		t.Fatal("Expected validation to fail for unknown agent")
	}

	if !strings.Contains(err.Error(), "not a trusted delegatable agent") {
		t.Errorf("Expected error to mention agent is not trusted, got: %v", err)
	}

	if !strings.Contains(err.Error(), "unknown-agent") {
		t.Errorf("Expected error to include agent name 'unknown-agent', got: %v", err)
	}
}

func TestValidatorWithMissingDependency(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "Task with missing dependency",
				Dependencies:       []string{"task-99"},
				AcceptanceCriteria: []string{"Done"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err == nil {
		t.Fatal("Expected validation to fail for missing dependency")
	}

	if !strings.Contains(err.Error(), "non-existent task") {
		t.Errorf("Expected error to mention non-existent task, got: %v", err)
	}

	if !strings.Contains(err.Error(), "task-99") {
		t.Errorf("Expected error to include missing task ID 'task-99', got: %v", err)
	}
}

func TestValidatorDetectsSimpleCycle(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	// Simple cycle: task-1 -> task-2 -> task-1
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "First task",
				Dependencies:       []string{"task-2"},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
			{
				ID:                 "task-2",
				Agent:              "builder",
				Description:        "Second task",
				Dependencies:       []string{"task-1"},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err == nil {
		t.Fatal("Expected validation to fail for circular dependency")
	}

	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("Expected error to mention circular dependency, got: %v", err)
	}

	// The reported path must start and end on the same node with no spurious
	// self-repeat in the middle. Depending on DFS start order the cycle reads
	// either "task-1 → task-2 → task-1" or "task-2 → task-1 → task-2"; both are
	// valid rotations, but "task-x → task-y → task-y" (the old bug) is not.
	msg := err.Error()
	okPath := strings.Contains(msg, "task-1 → task-2 → task-1") ||
		strings.Contains(msg, "task-2 → task-1 → task-2")
	if !okPath {
		t.Errorf("Expected a well-formed cycle path (a → b → a), got: %v", msg)
	}
	for _, bad := range []string{"task-1 → task-1 → task-1", "task-2 → task-1 → task-1", "task-1 → task-2 → task-2"} {
		if strings.Contains(msg, bad) {
			t.Errorf("Got malformed cycle path %q in: %v", bad, msg)
		}
	}
}

func TestValidatorDetectsComplexCycle(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	// Complex cycle: task-1 -> task-2 -> task-3 -> task-1
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "First task",
				Dependencies: []string{"task-3"},
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Second task",
				Dependencies: []string{"task-1"},
			},
			{
				ID:           "task-3",
				Agent:        "builder",
				Description:  "Third task",
				Dependencies: []string{"task-2"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err == nil {
		t.Fatal("Expected validation to fail for circular dependency")
	}

	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("Expected error to mention circular dependency, got: %v", err)
	}

	// Verify the cycle path is included
	errMsg := err.Error()
	if !strings.Contains(errMsg, "task-1") || !strings.Contains(errMsg, "task-2") || !strings.Contains(errMsg, "task-3") {
		t.Errorf("Expected error to include cycle path with all tasks, got: %v", errMsg)
	}
}

func TestValidatorDetectsSelfCycle(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	// Self-cycle: task depends on itself
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "Self-referencing task",
				Dependencies: []string{"task-1"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err == nil {
		t.Fatal("Expected validation to fail for self-referencing task")
	}

	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("Expected error to mention circular dependency, got: %v", err)
	}
}

func TestValidatorWithMultipleErrors(t *testing.T) {
	// Create test registry with limited agents
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	// Plan with multiple errors: unknown agent, missing dependency, and cycle
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "unknown-agent", // Error 1: unknown agent
				Description:  "Task with errors",
				Dependencies: []string{"task-99"}, // Error 2: missing dependency
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Another task",
				Dependencies: []string{"task-1"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err == nil {
		t.Fatal("Expected validation to fail with multiple errors")
	}

	errMsg := err.Error()

	// Check that error contains reference to unknown agent
	if !strings.Contains(errMsg, "unknown-agent") && !strings.Contains(errMsg, "unknown agent") {
		t.Errorf("Expected error to mention unknown agent, got: %v", errMsg)
	}

	// Check that error contains reference to missing dependency
	if !strings.Contains(errMsg, "task-99") && !strings.Contains(errMsg, "non-existent") {
		t.Errorf("Expected error to mention missing dependency, got: %v", errMsg)
	}
}

func TestValidatorWithValidDAG(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	// Valid DAG with diamond pattern:
	//     task-1
	//    /      \
	// task-2  task-3
	//    \      /
	//     task-4
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "Root task",
				Dependencies:       []string{},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
			{
				ID:                 "task-2",
				Agent:              "builder",
				Description:        "Branch 1",
				Dependencies:       []string{"task-1"},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
			{
				ID:                 "task-3",
				Agent:              "builder",
				Description:        "Branch 2",
				Dependencies:       []string{"task-1"},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
			{
				ID:                 "task-4",
				Agent:              "builder",
				Description:        "Merge",
				Dependencies:       []string{"task-2", "task-3"},
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err != nil {
		t.Errorf("Expected valid DAG to pass validation, got error: %v", err)
	}
}

func TestValidatorWithNilPlan(t *testing.T) {
	registry := NewAgentRegistry()
	validator := NewValidator(registry)

	err := validator.ValidatePlan(nil)
	if err == nil {
		t.Fatal("Expected validation to fail for nil plan")
	}

	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("Expected error to mention nil plan, got: %v", err)
	}
}

func TestValidatorWithSchemaErrors(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"builder": "builder.json",
	}

	validator := NewValidator(registry)

	// Plan with schema errors (invalid version, duplicate IDs)
	plan := &Plan{
		Version: "2.0", // Invalid version
		Tasks: []Task{
			{
				ID:          "task-1",
				Agent:       "builder",
				Description: "First task",
			},
			{
				ID:          "task-1", // Duplicate ID
				Agent:       "builder",
				Description: "Second task with same ID",
			},
		},
	}

	err := validator.ValidatePlan(plan)
	if err == nil {
		t.Fatal("Expected validation to fail for schema errors")
	}

	// Should catch version or duplicate ID error
	errMsg := err.Error()
	if !strings.Contains(errMsg, "version") && !strings.Contains(errMsg, "duplicate") {
		t.Errorf("Expected error to mention schema violations, got: %v", errMsg)
	}
}

func TestValidatePlanWithRegistry(t *testing.T) {
	// This is an integration-style test but with mocked file system
	// For now, we'll just test that the function signature works
	// Full integration would require actual agent config files

	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:          "task-1",
				Agent:       "builder",
				Description: "Test task",
			},
		},
	}

	// This will fail since the directory doesn't exist, but we're testing
	// the function exists and handles errors properly
	err := ValidatePlanWithRegistry(plan, "/nonexistent/directory")
	if err == nil {
		t.Fatal("Expected error for non-existent agent directory")
	}

	if !strings.Contains(err.Error(), "discover") {
		t.Errorf("Expected error to mention discovery failure, got: %v", err)
	}
}

func TestValidationErrorsFormatting(t *testing.T) {
	// Test single error formatting
	singleErr := ValidationErrors{
		ValidationError{Field: "test", Message: "test error"},
	}
	if !strings.Contains(singleErr.Error(), "test error") {
		t.Errorf("Expected single error to contain message, got: %v", singleErr.Error())
	}

	// Test multiple errors formatting
	multiErr := ValidationErrors{
		ValidationError{Field: "field1", Message: "error 1"},
		ValidationError{Field: "field2", Message: "error 2"},
	}
	errMsg := multiErr.Error()
	if !strings.Contains(errMsg, "2 validation errors") {
		t.Errorf("Expected multiple errors header, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "error 1") || !strings.Contains(errMsg, "error 2") {
		t.Errorf("Expected both errors in output, got: %v", errMsg)
	}

	// Test empty errors
	emptyErr := ValidationErrors{}
	if !strings.Contains(emptyErr.Error(), "no validation errors") {
		t.Errorf("Expected empty error message, got: %v", emptyErr.Error())
	}
}

// TestPlanValidate_AccumulatesErrors verifies that Plan.Validate reports every
// per-task schema failure in a single pass (not just the first), and that
// errors.Is still matches sentinels carried inside the aggregated result.
func TestPlanValidate_AccumulatesErrors(t *testing.T) {
	// task-1 is missing its agent, its acceptance criteria, and its validation
	// commands all at once. task-2 duplicates task-1's ID. A first-error-wins
	// validator would report only one problem; the accumulate contract must
	// surface all of them in one Validate call.
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:          "task-1",
				Agent:       "",
				Description: "Missing agent and lists",
			},
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "Duplicate ID",
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
			},
		},
	}

	err := p.Validate()
	if err == nil {
		t.Fatal("expected validation errors, got nil")
	}

	msg := err.Error()
	for _, want := range []string{
		"missing agent",
		"no acceptance criteria",
		"no validation commands",
		"duplicate task ID",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected aggregated error to contain %q, got:\n%s", want, msg)
		}
	}

	// Sentinels must remain matchable through the aggregated collection.
	if !errors.Is(err, ErrMissingAgent) {
		t.Errorf("expected errors.Is(err, ErrMissingAgent) to be true, got: %v", err)
	}
	if !errors.Is(err, ErrDuplicateTaskID) {
		t.Errorf("expected errors.Is(err, ErrDuplicateTaskID) to be true, got: %v", err)
	}
}

// TestValidatorRejectsUntrustedAgents verifies that a task assigned to an agent
// not in the trusted registry (which is populated from krew-lead.json's
// trustedAgents) fails validation — including planner and krew-lead, which are
// never in the trusted set — while a trusted agent passes. The registry is the
// single source of truth; there is no separate denylist.
func TestValidatorRejectsUntrustedAgents(t *testing.T) {
	// Registry represents the orchestrator's trusted set (as DiscoverAgents
	// would build it from krew-lead.json). Note planner/krew-lead are NOT here.
	registry := NewAgentRegistry()
	registry.agents = map[string]string{
		"architect":  "architect.json",
		"builder":    "builder.json",
		"validator":  "validator.json",
		"documenter": "documenter.json",
	}
	validator := NewValidator(registry)

	for _, agent := range []string{"planner", "krew-lead", "security-reviewer"} {
		t.Run(agent, func(t *testing.T) {
			plan := &Plan{
				Version: "1.0",
				Tasks: []Task{
					{
						ID:                 "task-1",
						Agent:              agent,
						Description:        "Should be rejected",
						Dependencies:       []string{},
						AcceptanceCriteria: []string{"Done"},
						ValidationCommands: []string{"go test"},
					},
				},
			}

			err := validator.ValidatePlan(plan)
			if err == nil {
				t.Fatalf("expected validation to reject untrusted agent %q", agent)
			}
			if !strings.Contains(err.Error(), "not a trusted delegatable agent") {
				t.Errorf("expected not-trusted message for %q, got: %v", agent, err)
			}
		})
	}

	// A trusted agent must still pass.
	t.Run("builder passes", func(t *testing.T) {
		plan := &Plan{
			Version: "1.0",
			Tasks: []Task{
				{
					ID:                 "task-1",
					Agent:              "builder",
					Description:        "Normal task",
					Dependencies:       []string{},
					AcceptanceCriteria: []string{"Done"},
					ValidationCommands: []string{"go test"},
				},
			},
		}
		if err := validator.ValidatePlan(plan); err != nil {
			t.Errorf("expected builder task to pass, got: %v", err)
		}
	})
}
