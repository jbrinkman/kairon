package plan

import (
	"fmt"
	"sort"
)

// TopologicalSort performs topological sorting on plan tasks using Kahn's algorithm.
// Returns tasks grouped into layers where each layer contains tasks with no unfulfilled dependencies.
// Tasks within a layer can be executed in parallel.
//
// Returns an error if the graph contains a cycle (should not happen if plan is validated).
func TopologicalSort(plan *Plan) ([][]Task, error) {
	if plan == nil || len(plan.Tasks) == 0 {
		return [][]Task{}, nil
	}

	// Build task lookup map
	taskMap := make(map[string]*Task)
	for i := range plan.Tasks {
		taskMap[plan.Tasks[i].ID] = &plan.Tasks[i]
	}

	// Calculate in-degree for each task (number of dependencies)
	inDegree := make(map[string]int)
	for _, task := range plan.Tasks {
		if _, exists := inDegree[task.ID]; !exists {
			inDegree[task.ID] = 0
		}
		for range task.Dependencies {
			inDegree[task.ID]++
		}
	}

	// Build adjacency list (task -> tasks that depend on it)
	dependents := make(map[string][]string)
	for _, task := range plan.Tasks {
		for _, depID := range task.Dependencies {
			dependents[depID] = append(dependents[depID], task.ID)
		}
	}

	// Find all tasks with in-degree 0 (no dependencies) - Layer 0
	var layers [][]Task
	currentLayer := []string{}
	for taskID, degree := range inDegree {
		if degree == 0 {
			currentLayer = append(currentLayer, taskID)
		}
	}
	// Sort for deterministic, stable ordering (map iteration order is random)
	sort.Strings(currentLayer)

	processed := 0

	// Process layers until no more tasks remain
	for len(currentLayer) > 0 {
		// Convert task IDs to Task objects for this layer
		layer := []Task{}
		for _, taskID := range currentLayer {
			if task, exists := taskMap[taskID]; exists {
				layer = append(layer, *task)
			}
		}
		layers = append(layers, layer)
		processed += len(currentLayer)

		// Find next layer: tasks whose dependencies are all satisfied
		nextLayer := []string{}
		for _, taskID := range currentLayer {
			// For each task that depends on this completed task
			for _, dependentID := range dependents[taskID] {
				inDegree[dependentID]--
				if inDegree[dependentID] == 0 {
					nextLayer = append(nextLayer, dependentID)
				}
			}
		}

		// Sort for deterministic, stable ordering across runs
		sort.Strings(nextLayer)
		currentLayer = nextLayer
	}

	// If not all tasks were processed, there's a cycle
	// (This shouldn't happen if plan is validated, but check anyway)
	if processed != len(plan.Tasks) {
		return nil, fmt.Errorf("cycle detected: only processed %d of %d tasks", processed, len(plan.Tasks))
	}

	return layers, nil
}
