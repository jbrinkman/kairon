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

// nonDelegatableAgents are discovered agents that must never be assigned a
// workflow task. The planner runs *before* a workflow (it creates the issue the
// Go poller later picks up), and krew-lead is the orchestrator that delegates
// tasks — it does not receive them. Both are auto-discovered from
// .kiro/agents/*.json, so validation must exclude them explicitly.
var nonDelegatableAgents = map[string]bool{
	"planner":   true,
	"krew-lead": true,
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
		} else if nonDelegatableAgents[task.Agent] {
			// The registry auto-discovers every .kiro/agents/*.json, which
			// includes agents that are not valid workflow task delegates:
			// the planner creates issues *before* a workflow starts, and
			// krew-lead is the orchestrator itself. Assigning a task to either
			// would pass a bare existence check but be rejected at execution by
			// the orchestrator's trusted-agent set — reject it here so the
			// failure is caught at validation with a clear message.
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("task[%s].agent", task.ID),
				Message: fmt.Sprintf("agent '%s' cannot be assigned tasks: it is not a delegatable workflow agent", task.Agent),
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
				// Cycle detected - reconstruct the path starting at `dependent`,
				// walking the parent chain from taskID back up to dependent, then
				// closing the loop. For a two-task cycle this reads
				// task-a → task-b → task-a (not task-b → task-a → task-a).
				var path []string
				for current := taskID; current != dependent; current = parent[current] {
					path = append(path, current)
				}
				cycle := []string{dependent}
				// path holds taskID..child-of-dependent; reverse it so the cycle
				// reads forward from dependent through its successors.
				for i := len(path) - 1; i >= 0; i-- {
					cycle = append(cycle, path[i])
				}
				cycle = append(cycle, dependent) // close the cycle
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
