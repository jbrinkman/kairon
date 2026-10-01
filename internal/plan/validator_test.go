package plan

import (
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

	if !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("Expected error to mention unknown agent, got: %v", err)
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
				ID:           "task-1",
				Agent:        "builder",
				Description:  "First task",
				Dependencies: []string{"task-2"},
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Second task",
				Dependencies: []string{"task-1"},
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
