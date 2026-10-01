package plan

import (
	"fmt"
	"strings"
)

// ValidationError represents a single validation failure
type ValidationError struct {
	Field   string // Field or aspect that failed validation
	Message string // Human-readable error message
	Err     error  // Optional underlying sentinel error, for errors.Is traversal
}

// Error implements the error interface for ValidationError
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Unwrap exposes the underlying sentinel (if any) so errors.Is can match
// against sentinels such as ErrMissingTaskID even when the failure is carried
// inside an aggregated ValidationErrors collection.
func (e ValidationError) Unwrap() error {
	return e.Err
}

// ValidationErrors is a collection of validation failures
type ValidationErrors []ValidationError

// Error implements the error interface for ValidationErrors
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}
	if len(e) == 1 {
		return e[0].Error()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d validation errors:\n", len(e)))
	for i, err := range e {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// Unwrap returns the contained errors so errors.Is / errors.As can traverse the
// whole collection (Go 1.20+ multi-error unwrap). This lets callers match any
// accumulated sentinel, e.g. errors.Is(err, ErrDuplicateTaskID).
func (e ValidationErrors) Unwrap() []error {
	errs := make([]error, len(e))
	for i := range e {
		errs[i] = e[i]
	}
	return errs
}

// Validator validates plans against schema rules, dependency resolution, and agent availability
type Validator struct {
	registry *AgentRegistry
}

// NewValidator creates a new validator with the given agent registry
func NewValidator(registry *AgentRegistry) *Validator {
	return &Validator{
		registry: registry,
	}
}

// ValidatePlan performs comprehensive validation on a plan, checking:
// - Schema validity (version, task IDs, required fields)
// - Dependency resolution (all referenced tasks exist)
// - Agent resolution (all agents exist in registry)
// - DAG acyclicity (no circular dependencies)
//
// Returns all validation errors found, not just the first failure
func (v *Validator) ValidatePlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}

	var errors ValidationErrors

	// Schema validation (already handled by Plan.Validate())
	if err := plan.Validate(); err != nil {
		// If it's already ValidationErrors, append them
		if verrs, ok := err.(ValidationErrors); ok {
			errors = append(errors, verrs...)
		} else {
			errors = append(errors, ValidationError{
				Field:   "schema",
				Message: err.Error(),
			})
		}
	}

	// Build task ID set for dependency resolution
	taskIDs := make(map[string]bool)
	for _, task := range plan.Tasks {
		taskIDs[task.ID] = true
	}

	// Validate each task
	for _, task := range plan.Tasks {
		// Dependency resolution: check that all dependencies exist
		for _, depID := range task.Dependencies {
			if !taskIDs[depID] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("task[%s].dependencies", task.ID),
					Message: fmt.Sprintf("references non-existent task '%s'", depID),
				})
			}
		}

		// Agent resolution: check that agent exists in registry
		if !v.registry.Contains(task.Agent) {
			availableAgents := v.registry.GetAgentNames()
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("task[%s].agent", task.ID),
				Message: fmt.Sprintf("unknown agent '%s' (available: %s)", task.Agent, strings.Join(availableAgents, ", ")),
			})
		}
	}

	// DAG acyclicity check using DFS
	if cycle := v.detectCycle(plan); cycle != nil {
		errors = append(errors, ValidationError{
			Field:   "dependencies",
			Message: fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " → ")),
		})
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

// detectCycle uses depth-first search to detect cycles in the task dependency graph.
// Returns the cycle path if found, or nil if the graph is acyclic.
func (v *Validator) detectCycle(plan *Plan) []string {
	// Build adjacency list (task ID -> dependent task IDs)
	graph := make(map[string][]string)
	for _, task := range plan.Tasks {
		if _, exists := graph[task.ID]; !exists {
			graph[task.ID] = []string{}
		}
		for _, depID := range task.Dependencies {
			graph[depID] = append(graph[depID], task.ID)
		}
	}

	// Track visited nodes and recursion stack
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	parent := make(map[string]string)

	// DFS helper function
	var dfs func(string) *[]string
	dfs = func(taskID string) *[]string {
		visited[taskID] = true
		recStack[taskID] = true

		for _, dependent := range graph[taskID] {
			if !visited[dependent] {
				parent[dependent] = taskID
				if cycle := dfs(dependent); cycle != nil {
					return cycle
				}
			} else if recStack[dependent] {
				// Cycle detected - reconstruct the cycle path
				cycle := []string{dependent}
				current := taskID
				for current != dependent {
					cycle = append([]string{current}, cycle...)
					current = parent[current]
				}
				cycle = append(cycle, dependent) // Close the cycle
				return &cycle
			}
		}

		recStack[taskID] = false
		return nil
	}

	// Run DFS from each unvisited node
	for _, task := range plan.Tasks {
		if !visited[task.ID] {
			if cycle := dfs(task.ID); cycle != nil {
				return *cycle
			}
		}
	}

	return nil
}

// ValidatePlanWithRegistry is a convenience function that discovers agents and validates a plan
func ValidatePlanWithRegistry(plan *Plan, agentDir string) error {
	registry, err := DiscoverAgents(agentDir)
	if err != nil {
		return fmt.Errorf("failed to discover agents: %w", err)
	}

	validator := NewValidator(registry)
	return validator.ValidatePlan(plan)
}
