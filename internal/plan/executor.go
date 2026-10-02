package plan

import (
	"fmt"
	"sort"
	"sync"
)

// TaskStatus represents the execution status of a task
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending" // Task not yet started
	TaskStatusRunning TaskStatus = "running" // Task currently executing
	TaskStatusSuccess TaskStatus = "success" // Task completed successfully
	TaskStatusFailed  TaskStatus = "failed"  // Task failed
	TaskStatusSkipped TaskStatus = "skipped" // Task skipped due to dependency failure
)

// TaskResult represents the outcome of a task execution
type TaskResult struct {
	TaskID   string     // ID of the executed task
	Status   TaskStatus // Execution status
	ExitCode int        // Exit code from agent process (0 = success)
	Error    error      // Error if task failed to spawn or execute
}

// ExecutionSummary contains the results of plan execution
type ExecutionSummary struct {
	Results    []TaskResult // Results for each task
	TotalTasks int          // Total number of tasks
	Successful int          // Number of successfully completed tasks
	Failed     int          // Number of failed tasks
	Skipped    int          // Number of skipped tasks
}

// TaskSpawner is a function type that spawns an agent to execute a task
// It takes the task and returns an exit code (0 for success, non-zero for failure)
type TaskSpawner func(task Task) (exitCode int, err error)

// Executor executes validated plans with topological sorting and parallelization
// DefaultMaxParallelTasks bounds how many tasks within a single layer may be
// spawned concurrently when no explicit limit is given. Matches the
// max_parallel_tasks default in the plan-and-execute spec (issue-273).
const DefaultMaxParallelTasks = 4

type Executor struct {
	spawner     TaskSpawner // Function to spawn agent processes
	maxParallel int         // Max concurrent task spawns per layer (semaphore size)
}

// NewExecutor creates a new plan executor with the given task spawner, using
// the default per-layer concurrency limit (DefaultMaxParallelTasks).
func NewExecutor(spawner TaskSpawner) *Executor {
	return &Executor{
		spawner:     spawner,
		maxParallel: DefaultMaxParallelTasks,
	}
}

// NewExecutorWithLimit creates an executor with an explicit per-layer
// concurrency limit. A non-positive limit falls back to DefaultMaxParallelTasks
// so the executor never runs unbounded.
func NewExecutorWithLimit(spawner TaskSpawner, maxParallel int) *Executor {
	if maxParallel < 1 {
		maxParallel = DefaultMaxParallelTasks
	}
	return &Executor{
		spawner:     spawner,
		maxParallel: maxParallel,
	}
}

// ExecutePlan executes a validated plan with parallel task execution.
// Tasks are executed in topologically sorted layers, where all tasks in a layer
// execute concurrently. The executor waits for each layer to complete before
// proceeding to the next.
//
// Task failures are handled gracefully:
// - Failed tasks don't crash the executor
// - Tasks dependent on failed tasks are skipped
// - Independent tasks continue execution regardless of failures
//
// Returns an ExecutionSummary with the status of each task.
func (e *Executor) ExecutePlan(plan *Plan) (*ExecutionSummary, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is nil")
	}

	if len(plan.Tasks) == 0 {
		return &ExecutionSummary{
			Results:    []TaskResult{},
			TotalTasks: 0,
		}, nil
	}

	// Perform topological sort to determine execution order
	layers, err := TopologicalSort(plan)
	if err != nil {
		return nil, fmt.Errorf("topological sort failed: %w", err)
	}

	// Track task completion status with mutex for concurrent access
	var statusMu sync.RWMutex
	taskStatus := make(map[string]TaskStatus)
	for _, task := range plan.Tasks {
		taskStatus[task.ID] = TaskStatusPending
	}

	// Execute layers sequentially, tasks within a layer in parallel
	var allResults []TaskResult

	for layerNum, layer := range layers {
		// Check if any tasks in this layer should be skipped due to failed dependencies
		executableTasks := []Task{}
		for _, task := range layer {
			statusMu.RLock()
			shouldSkip := e.shouldSkipTask(task, taskStatus)
			statusMu.RUnlock()

			if shouldSkip {
				// Mark as skipped
				statusMu.Lock()
				taskStatus[task.ID] = TaskStatusSkipped
				statusMu.Unlock()

				allResults = append(allResults, TaskResult{
					TaskID:   task.ID,
					Status:   TaskStatusSkipped,
					ExitCode: -1,
				})
			} else {
				executableTasks = append(executableTasks, task)
			}
		}

		// Execute all executable tasks in this layer concurrently
		if len(executableTasks) > 0 {
			layerResults := e.executeLayer(layerNum, executableTasks, taskStatus, &statusMu)
			allResults = append(allResults, layerResults...)
		}
	}

	// Compile execution summary
	summary := &ExecutionSummary{
		Results:    allResults,
		TotalTasks: len(plan.Tasks),
	}

	for _, result := range allResults {
		switch result.Status {
		case TaskStatusSuccess:
			summary.Successful++
		case TaskStatusFailed:
			summary.Failed++
		case TaskStatusSkipped:
			summary.Skipped++
		}
	}

	return summary, nil
}

// shouldSkipTask determines if a task should be skipped based on dependency status
func (e *Executor) shouldSkipTask(task Task, taskStatus map[string]TaskStatus) bool {
	for _, depID := range task.Dependencies {
		status := taskStatus[depID]
		if status == TaskStatusFailed || status == TaskStatusSkipped {
			return true
		}
	}
	return false
}

// executeLayer executes the tasks in a layer concurrently using a WaitGroup,
// bounded by a semaphore so no more than e.maxParallel task agents are spawned
// at once. A layer may contain more tasks than the limit; the excess queue on
// the semaphore rather than forking unbounded agent processes.
func (e *Executor) executeLayer(layerNum int, tasks []Task, taskStatus map[string]TaskStatus, statusMu *sync.RWMutex) []TaskResult {
	var wg sync.WaitGroup
	results := make([]TaskResult, len(tasks))

	// Semaphore limiting concurrent spawns. maxParallel is always >= 1 (set by
	// the constructors), so this channel is never zero-capacity.
	sem := make(chan struct{}, e.maxParallel)

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()

			// Acquire a semaphore slot before spawning; release on completion.
			sem <- struct{}{}
			defer func() { <-sem }()

			// Mark task as running
			statusMu.Lock()
			taskStatus[t.ID] = TaskStatusRunning
			statusMu.Unlock()

			// Spawn the task agent
			exitCode, err := e.spawner(t)

			// Determine status based on exit code and error
			var status TaskStatus
			if err != nil {
				status = TaskStatusFailed
			} else if exitCode == 0 {
				status = TaskStatusSuccess
			} else {
				status = TaskStatusFailed
			}

			statusMu.Lock()
			taskStatus[t.ID] = status
			statusMu.Unlock()

			results[idx] = TaskResult{
				TaskID:   t.ID,
				Status:   status,
				ExitCode: exitCode,
				Error:    err,
			}
		}(i, task)
	}

	wg.Wait()
	return results
}

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
