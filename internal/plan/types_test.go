package plan

import (
	"errors"
	"testing"
)

func TestPlanValidate_ValidPlan(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "Implement feature A",
				Dependencies:       []string{},
				AcceptanceCriteria: []string{"Feature works"},
				ValidationCommands: []string{"go test"},
			},
			{
				ID:                 "task-2",
				Agent:              "validator",
				Description:        "Validate feature A",
				Dependencies:       []string{"task-1"},
				AcceptanceCriteria: []string{"Tests pass"},
				ValidationCommands: []string{"go test"},
			},
		},
	}

	err := p.Validate()
	if err != nil {
		t.Errorf("expected valid plan, got error: %v", err)
	}
}

func TestPlanValidate_NilPlan(t *testing.T) {
	var p *Plan
	err := p.Validate()
	if err == nil {
		t.Error("expected error for nil plan")
	}
}

func TestPlanValidate_InvalidVersion(t *testing.T) {
	p := &Plan{
		Version: "2.0",
		Tasks: []Task{
			{
				ID:          "task-1",
				Agent:       "builder",
				Description: "Test task",
			},
		},
	}

	err := p.Validate()
	if err == nil {
		t.Error("expected error for invalid version")
	}
	if !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("expected ErrInvalidVersion, got: %v", err)
	}
}

func TestPlanValidate_EmptyPlan(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks:   []Task{},
	}

	err := p.Validate()
	if err == nil {
		t.Error("expected error for empty plan")
	}
	if !errors.Is(err, ErrEmptyPlan) {
		t.Errorf("expected ErrEmptyPlan, got: %v", err)
	}
}

func TestPlanValidate_DuplicateTaskID(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:                 "task-1",
				Agent:              "builder",
				Description:        "First task",
				AcceptanceCriteria: []string{"Done"},
				ValidationCommands: []string{"go test"},
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
		t.Error("expected error for duplicate task ID")
	}
	if !errors.Is(err, ErrDuplicateTaskID) {
		t.Errorf("expected ErrDuplicateTaskID, got: %v", err)
	}
}

func TestPlanValidate_MissingTaskID(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:          "",
				Agent:       "builder",
				Description: "Task with no ID",
			},
		},
	}

	err := p.Validate()
	if err == nil {
		t.Error("expected error for missing task ID")
	}
	if !errors.Is(err, ErrMissingTaskID) {
		t.Errorf("expected ErrMissingTaskID, got: %v", err)
	}
}

func TestPlanValidate_MissingAgent(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:          "task-1",
				Agent:       "",
				Description: "Task with no agent",
			},
		},
	}

	err := p.Validate()
	if err == nil {
		t.Error("expected error for missing agent")
	}
	if !errors.Is(err, ErrMissingAgent) {
		t.Errorf("expected ErrMissingAgent, got: %v", err)
	}
}

func TestPlanValidate_EmptyDescription(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:          "task-1",
				Agent:       "builder",
				Description: "",
			},
		},
	}

	err := p.Validate()
	if err == nil {
		t.Error("expected error for empty description")
	}
}

func TestPlanGetTaskByID(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{ID: "task-1", Agent: "builder", Description: "First"},
			{ID: "task-2", Agent: "validator", Description: "Second"},
		},
	}

	// Test finding existing task
	task := p.GetTaskByID("task-2")
	if task == nil {
		t.Error("expected to find task-2")
	} else if task.ID != "task-2" {
		t.Errorf("expected task-2, got %s", task.ID)
	}

	// Test non-existent task
	task = p.GetTaskByID("task-999")
	if task != nil {
		t.Error("expected nil for non-existent task")
	}
}

func TestPlanGetTaskIDs(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{ID: "task-1", Agent: "builder", Description: "First"},
			{ID: "task-2", Agent: "validator", Description: "Second"},
			{ID: "task-3", Agent: "builder", Description: "Third"},
		},
	}

	ids := p.GetTaskIDs()
	if len(ids) != 3 {
		t.Errorf("expected 3 task IDs, got %d", len(ids))
	}

	expected := []string{"task-1", "task-2", "task-3"}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("expected %s at index %d, got %s", expected[i], i, id)
		}
	}
}

func TestPlanGetTaskIDs_EmptyPlan(t *testing.T) {
	p := &Plan{
		Version: "1.0",
		Tasks:   []Task{},
	}

	ids := p.GetTaskIDs()
	if len(ids) != 0 {
		t.Errorf("expected 0 task IDs, got %d", len(ids))
	}
}
