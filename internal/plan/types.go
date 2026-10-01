package plan

import (
	"fmt"
)

// Plan represents a machine-readable execution plan for an issue
type Plan struct {
	Version string `yaml:"version"`
	Tasks   []Task `yaml:"tasks"`
}

// Task represents a single unit of work within a plan
type Task struct {
	ID                 string   `yaml:"id"`
	Agent              string   `yaml:"agent"`
	Description        string   `yaml:"description"`
	Dependencies       []string `yaml:"dependencies"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	ValidationCommands []string `yaml:"validation_commands"`
}

// Error types for validation failures
var (
	ErrInvalidVersion  = fmt.Errorf("invalid plan version")
	ErrDuplicateTaskID = fmt.Errorf("duplicate task ID")
	ErrMissingAgent    = fmt.Errorf("missing agent assignment")
	ErrMissingTaskID   = fmt.Errorf("missing task ID")
	ErrEmptyPlan       = fmt.Errorf("plan has no tasks")
)

// Validate performs schema validation on the plan
func (p *Plan) Validate() error {
	if p == nil {
		return fmt.Errorf("plan is nil")
	}

	// Check version
	if p.Version != "1.0" {
		return fmt.Errorf("%w: expected '1.0', got '%s'", ErrInvalidVersion, p.Version)
	}

	// Check for empty plan
	if len(p.Tasks) == 0 {
		return ErrEmptyPlan
	}

	// Track task IDs for uniqueness check
	taskIDs := make(map[string]bool)

	for i, task := range p.Tasks {
		// Check for missing task ID
		if task.ID == "" {
			return fmt.Errorf("%w: task at index %d", ErrMissingTaskID, i)
		}

		// Check for duplicate task IDs
		if taskIDs[task.ID] {
			return fmt.Errorf("%w: '%s'", ErrDuplicateTaskID, task.ID)
		}
		taskIDs[task.ID] = true

		// Check for missing agent
		if task.Agent == "" {
			return fmt.Errorf("%w: task '%s'", ErrMissingAgent, task.ID)
		}

		// Check for empty description
		if task.Description == "" {
			return fmt.Errorf("task '%s' has empty description", task.ID)
		}

		// Each task must define at least one acceptance criterion and one
		// validation command so execution has an explicit success boundary and
		// a task-level verification step (see spec issue-273).
		if len(task.AcceptanceCriteria) == 0 {
			return fmt.Errorf("task '%s' has no acceptance criteria", task.ID)
		}
		if len(task.ValidationCommands) == 0 {
			return fmt.Errorf("task '%s' has no validation commands", task.ID)
		}
	}

	return nil
}

// GetTaskByID returns a task by its ID, or nil if not found
func (p *Plan) GetTaskByID(id string) *Task {
	for i := range p.Tasks {
		if p.Tasks[i].ID == id {
			return &p.Tasks[i]
		}
	}
	return nil
}

// GetTaskIDs returns a slice of all task IDs in the plan
func (p *Plan) GetTaskIDs() []string {
	ids := make([]string, len(p.Tasks))
	for i, task := range p.Tasks {
		ids[i] = task.ID
	}
	return ids
}
