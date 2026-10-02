package plan

import (
	"fmt"
	"regexp"
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
	ErrInvalidTaskID   = fmt.Errorf("invalid task ID")
	ErrEmptyPlan       = fmt.Errorf("plan has no tasks")
)

// taskIDPattern is the safe grammar for task IDs: lowercase alphanumerics in
// single-hyphen-separated groups (kebab-case), e.g. "task-1", "implement-api".
// Task IDs are interpolated verbatim into sentinel paths and shell commands
// (e.g. .kiro-krew/artifacts/<agent>-<issue>-<task-id>.md and test -f checks),
// so disallowing '/', '..', whitespace, and shell metacharacters prevents path
// traversal and command injection via a crafted or malformed ID.
var taskIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate performs schema validation on the plan.
//
// Version and empty-plan problems are whole-plan errors and fail fast (there is
// nothing meaningful to check per task once they trip). All per-task schema
// failures are accumulated and returned together as ValidationErrors, so a
// single call reports every problem rather than stopping at the first — this
// gives the architect complete, actionable feedback in one pass. Each entry
// wraps its sentinel (ErrMissingTaskID, etc.) so errors.Is still matches.
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

	var errs ValidationErrors

	// Track task IDs for uniqueness check
	taskIDs := make(map[string]bool)

	for i, task := range p.Tasks {
		// Check for missing task ID
		if task.ID == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("task[%d].id", i),
				Message: fmt.Sprintf("%v: task at index %d", ErrMissingTaskID, i),
				Err:     ErrMissingTaskID,
			})
			// Without an ID the remaining per-task checks can't reference the
			// task meaningfully; skip to the next task.
			continue
		}

		// Enforce the safe task-ID grammar before the ID is ever interpolated
		// into sentinel paths or shell commands downstream.
		if !taskIDPattern.MatchString(task.ID) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("task[%s].id", task.ID),
				Message: fmt.Sprintf("%v: '%s' must be kebab-case (lowercase alphanumerics separated by single hyphens)", ErrInvalidTaskID, task.ID),
				Err:     ErrInvalidTaskID,
			})
			// A malformed ID can't be trusted in paths/commands or as a map key
			// for duplicate detection; skip the remaining checks for this task.
			continue
		}

		// Check for duplicate task IDs
		if taskIDs[task.ID] {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("task[%s].id", task.ID),
				Message: fmt.Sprintf("%v: '%s'", ErrDuplicateTaskID, task.ID),
				Err:     ErrDuplicateTaskID,
			})
		}
		taskIDs[task.ID] = true

		// Check for missing agent
		if task.Agent == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("task[%s].agent", task.ID),
				Message: fmt.Sprintf("%v: task '%s'", ErrMissingAgent, task.ID),
				Err:     ErrMissingAgent,
			})
		}

		// Check for empty description
		if task.Description == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("task[%s].description", task.ID),
				Message: fmt.Sprintf("task '%s' has empty description", task.ID),
			})
		}

		// Each task must define at least one acceptance criterion and one
		// validation command so execution has an explicit success boundary and
		// a task-level verification step (see spec issue-273).
		if len(task.AcceptanceCriteria) == 0 {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("task[%s].acceptance_criteria", task.ID),
				Message: fmt.Sprintf("task '%s' has no acceptance criteria", task.ID),
			})
		}
		if len(task.ValidationCommands) == 0 {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("task[%s].validation_commands", task.ID),
				Message: fmt.Sprintf("task '%s' has no validation commands", task.ID),
			})
		}
	}

	if len(errs) > 0 {
		return errs
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
