package plan

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTopologicalSortWithSimpleChain(t *testing.T) {
	// Linear dependency: task-1 -> task-2 -> task-3
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "First task",
				Dependencies: []string{},
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

	layers, err := TopologicalSort(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(layers) != 3 {
		t.Errorf("Expected 3 layers, got %d", len(layers))
	}

	// Verify layer 0 contains task-1
	if len(layers[0]) != 1 || layers[0][0].ID != "task-1" {
		t.Errorf("Expected layer 0 to contain task-1, got %v", layers[0])
	}

	// Verify layer 1 contains task-2
	if len(layers[1]) != 1 || layers[1][0].ID != "task-2" {
		t.Errorf("Expected layer 1 to contain task-2, got %v", layers[1])
	}

	// Verify layer 2 contains task-3
	if len(layers[2]) != 1 || layers[2][0].ID != "task-3" {
		t.Errorf("Expected layer 2 to contain task-3, got %v", layers[2])
	}
}

func TestTopologicalSortWithParallelTasks(t *testing.T) {
	// All tasks independent (can run in parallel)
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "First task",
				Dependencies: []string{},
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Second task",
				Dependencies: []string{},
			},
			{
				ID:           "task-3",
				Agent:        "builder",
				Description:  "Third task",
				Dependencies: []string{},
			},
		},
	}

	layers, err := TopologicalSort(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(layers) != 1 {
		t.Errorf("Expected 1 layer (all parallel), got %d", len(layers))
	}

	if len(layers[0]) != 3 {
		t.Errorf("Expected layer 0 to contain 3 tasks, got %d", len(layers[0]))
	}
}

func TestTopologicalSortWithDiamondPattern(t *testing.T) {
	// Diamond pattern:
	//     task-1
	//    /      \
	// task-2  task-3
	//    \      /
	//     task-4
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "Root",
				Dependencies: []string{},
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Branch 1",
				Dependencies: []string{"task-1"},
			},
			{
				ID:           "task-3",
				Agent:        "builder",
				Description:  "Branch 2",
				Dependencies: []string{"task-1"},
			},
			{
				ID:           "task-4",
				Agent:        "builder",
				Description:  "Merge",
				Dependencies: []string{"task-2", "task-3"},
			},
		},
	}

	layers, err := TopologicalSort(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(layers) != 3 {
		t.Errorf("Expected 3 layers, got %d", len(layers))
	}

	// Layer 0: task-1
	if len(layers[0]) != 1 {
		t.Errorf("Expected layer 0 to have 1 task, got %d", len(layers[0]))
	}

	// Layer 1: task-2 and task-3 (parallel)
	if len(layers[1]) != 2 {
		t.Errorf("Expected layer 1 to have 2 tasks, got %d", len(layers[1]))
	}

	// Layer 2: task-4
	if len(layers[2]) != 1 {
		t.Errorf("Expected layer 2 to have 1 task, got %d", len(layers[2]))
	}
}

func TestTopologicalSortWithEmptyPlan(t *testing.T) {
	plan := &Plan{
		Version: "1.0",
		Tasks:   []Task{},
	}

	layers, err := TopologicalSort(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(layers) != 0 {
		t.Errorf("Expected 0 layers for empty plan, got %d", len(layers))
	}
}

func TestTopologicalSortWithNilPlan(t *testing.T) {
	layers, err := TopologicalSort(nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(layers) != 0 {
		t.Errorf("Expected 0 layers for nil plan, got %d", len(layers))
	}
}

func TestExecutorWithSuccessfulTasks(t *testing.T) {
	// Mock spawner that always succeeds
	spawner := func(task Task) (int, error) {
		return 0, nil // Exit code 0 = success
	}

	executor := NewExecutor(spawner)

	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "Task 1",
				Dependencies: []string{},
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Task 2",
				Dependencies: []string{},
			},
		},
	}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if summary.TotalTasks != 2 {
		t.Errorf("Expected 2 total tasks, got %d", summary.TotalTasks)
	}

	if summary.Successful != 2 {
		t.Errorf("Expected 2 successful tasks, got %d", summary.Successful)
	}

	if summary.Failed != 0 {
		t.Errorf("Expected 0 failed tasks, got %d", summary.Failed)
	}

	if summary.Skipped != 0 {
		t.Errorf("Expected 0 skipped tasks, got %d", summary.Skipped)
	}
}

func TestExecutorWithFailedTask(t *testing.T) {
	// Mock spawner that fails for task-1
	spawner := func(task Task) (int, error) {
		if task.ID == "task-1" {
			return 1, fmt.Errorf("task failed")
		}
		return 0, nil
	}

	executor := NewExecutor(spawner)

	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "Failing task",
				Dependencies: []string{},
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Dependent task",
				Dependencies: []string{"task-1"},
			},
		},
	}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if summary.Failed != 1 {
		t.Errorf("Expected 1 failed task, got %d", summary.Failed)
	}

	if summary.Skipped != 1 {
		t.Errorf("Expected 1 skipped task (dependent on failed task), got %d", summary.Skipped)
	}

	// Verify task-2 was skipped
	for _, result := range summary.Results {
		if result.TaskID == "task-2" && result.Status != TaskStatusSkipped {
			t.Errorf("Expected task-2 to be skipped, got status %s", result.Status)
		}
	}
}

func TestExecutorWithPartialFailure(t *testing.T) {
	// Mock spawner that fails for task-2
	spawner := func(task Task) (int, error) {
		if task.ID == "task-2" {
			return 1, nil // Non-zero exit code
		}
		return 0, nil
	}

	executor := NewExecutor(spawner)

	// Diamond pattern where task-2 fails but task-3 succeeds
	//     task-1
	//    /      \
	// task-2  task-3
	//    \      /
	//     task-4
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{
				ID:           "task-1",
				Agent:        "builder",
				Description:  "Root",
				Dependencies: []string{},
			},
			{
				ID:           "task-2",
				Agent:        "builder",
				Description:  "Branch 1 (fails)",
				Dependencies: []string{"task-1"},
			},
			{
				ID:           "task-3",
				Agent:        "builder",
				Description:  "Branch 2 (succeeds)",
				Dependencies: []string{"task-1"},
			},
			{
				ID:           "task-4",
				Agent:        "builder",
				Description:  "Merge",
				Dependencies: []string{"task-2", "task-3"},
			},
		},
	}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// task-1 succeeds, task-2 fails, task-3 succeeds, task-4 skipped
	if summary.Successful != 2 {
		t.Errorf("Expected 2 successful tasks, got %d", summary.Successful)
	}

	if summary.Failed != 1 {
		t.Errorf("Expected 1 failed task, got %d", summary.Failed)
	}

	if summary.Skipped != 1 {
		t.Errorf("Expected 1 skipped task, got %d", summary.Skipped)
	}
}

func TestExecutorParallelExecution(t *testing.T) {
	// Track concurrent execution
	var mu sync.Mutex
	concurrent := 0
	maxConcurrent := 0
	executionOrder := []string{}

	// Mock spawner that simulates work and tracks concurrency
	spawner := func(task Task) (int, error) {
		mu.Lock()
		concurrent++
		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}
		executionOrder = append(executionOrder, task.ID)
		mu.Unlock()

		// Simulate work
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		concurrent--
		mu.Unlock()

		return 0, nil
	}

	executor := NewExecutor(spawner)

	// Three independent tasks that should run in parallel
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{ID: "task-1", Agent: "builder", Description: "Task 1", Dependencies: []string{}},
			{ID: "task-2", Agent: "builder", Description: "Task 2", Dependencies: []string{}},
			{ID: "task-3", Agent: "builder", Description: "Task 3", Dependencies: []string{}},
		},
	}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if summary.Successful != 3 {
		t.Errorf("Expected 3 successful tasks, got %d", summary.Successful)
	}

	// Verify tasks ran concurrently (maxConcurrent should be > 1)
	if maxConcurrent < 2 {
		t.Errorf("Expected concurrent execution (maxConcurrent >= 2), got %d", maxConcurrent)
	}
}

func TestExecutorWithEmptyPlan(t *testing.T) {
	spawner := func(task Task) (int, error) {
		return 0, nil
	}

	executor := NewExecutor(spawner)

	plan := &Plan{
		Version: "1.0",
		Tasks:   []Task{},
	}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if summary.TotalTasks != 0 {
		t.Errorf("Expected 0 total tasks, got %d", summary.TotalTasks)
	}
}

func TestExecutorWithNilPlan(t *testing.T) {
	spawner := func(task Task) (int, error) {
		return 0, nil
	}

	executor := NewExecutor(spawner)

	_, err := executor.ExecutePlan(nil)
	if err == nil {
		t.Fatal("Expected error for nil plan")
	}

	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("Expected error to mention nil plan, got: %v", err)
	}
}

func TestExecutorLayerSequencing(t *testing.T) {
	// Track execution order to verify layers execute sequentially
	var mu sync.Mutex
	executionOrder := []string{}
	taskCompletions := make(map[string]time.Time)

	spawner := func(task Task) (int, error) {
		mu.Lock()
		executionOrder = append(executionOrder, task.ID)
		mu.Unlock()

		// Simulate work
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		taskCompletions[task.ID] = time.Now()
		mu.Unlock()

		return 0, nil
	}

	executor := NewExecutor(spawner)

	// Linear dependency chain
	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{ID: "task-1", Agent: "builder", Description: "Task 1", Dependencies: []string{}},
			{ID: "task-2", Agent: "builder", Description: "Task 2", Dependencies: []string{"task-1"}},
			{ID: "task-3", Agent: "builder", Description: "Task 3", Dependencies: []string{"task-2"}},
		},
	}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if summary.Successful != 3 {
		t.Errorf("Expected 3 successful tasks, got %d", summary.Successful)
	}

	// Verify task-1 completed before task-2 started
	// and task-2 completed before task-3 started
	// (Since we track completion times and each task takes 10ms)
	if len(executionOrder) < 3 {
		t.Fatalf("Expected 3 tasks in execution order, got %d", len(executionOrder))
	}

	// First task should be task-1
	if executionOrder[0] != "task-1" {
		t.Errorf("Expected first execution to be task-1, got %s", executionOrder[0])
	}

	// task-2 should execute after task-1
	idx1 := -1
	idx2 := -1
	for i, id := range executionOrder {
		if id == "task-1" {
			idx1 = i
		}
		if id == "task-2" {
			idx2 = i
		}
	}
	if idx1 >= idx2 {
		t.Errorf("Expected task-1 to execute before task-2")
	}
}

func TestExecutorTaskResultMapping(t *testing.T) {
	spawner := func(task Task) (int, error) {
		if task.ID == "task-fail" {
			return 1, fmt.Errorf("intentional failure")
		}
		return 0, nil
	}

	executor := NewExecutor(spawner)

	plan := &Plan{
		Version: "1.0",
		Tasks: []Task{
			{ID: "task-success", Agent: "builder", Description: "Success", Dependencies: []string{}},
			{ID: "task-fail", Agent: "builder", Description: "Fail", Dependencies: []string{}},
		},
	}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Find results for each task
	var successResult, failResult *TaskResult
	for i := range summary.Results {
		if summary.Results[i].TaskID == "task-success" {
			successResult = &summary.Results[i]
		}
		if summary.Results[i].TaskID == "task-fail" {
			failResult = &summary.Results[i]
		}
	}

	if successResult == nil {
		t.Fatal("Expected to find result for task-success")
	}
	if failResult == nil {
		t.Fatal("Expected to find result for task-fail")
	}

	// Verify success task
	if successResult.Status != TaskStatusSuccess {
		t.Errorf("Expected task-success to have success status, got %s", successResult.Status)
	}
	if successResult.ExitCode != 0 {
		t.Errorf("Expected task-success to have exit code 0, got %d", successResult.ExitCode)
	}

	// Verify failed task
	if failResult.Status != TaskStatusFailed {
		t.Errorf("Expected task-fail to have failed status, got %s", failResult.Status)
	}
	if failResult.ExitCode == 0 {
		t.Errorf("Expected task-fail to have non-zero exit code, got %d", failResult.ExitCode)
	}
	if failResult.Error == nil {
		t.Error("Expected task-fail to have an error")
	}
}

// TestExecutorRespectsConcurrencyLimit verifies that a layer wider than the
// configured limit never spawns more than maxParallel task agents at once, and
// that all tasks still run and succeed (the excess queue on the semaphore).
func TestExecutorRespectsConcurrencyLimit(t *testing.T) {
	const limit = 2
	const taskCount = 6

	var inFlight int64
	var peak int64

	spawner := func(task Task) (int, error) {
		cur := atomic.AddInt64(&inFlight, 1)
		// Record the high-water mark of concurrent spawns.
		for {
			old := atomic.LoadInt64(&peak)
			if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
				break
			}
		}
		// Hold the slot briefly so overlap is observable.
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		return 0, nil
	}

	executor := NewExecutorWithLimit(spawner, limit)

	// Single layer of independent tasks (no dependencies), wider than the limit.
	tasks := make([]Task, taskCount)
	for i := 0; i < taskCount; i++ {
		tasks[i] = Task{
			ID:           fmt.Sprintf("task-%d", i+1),
			Agent:        "builder",
			Description:  fmt.Sprintf("Task %d", i+1),
			Dependencies: []string{},
		}
	}
	plan := &Plan{Version: "1.0", Tasks: tasks}

	summary, err := executor.ExecutePlan(plan)
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}

	if summary.Successful != taskCount {
		t.Errorf("Expected %d successful tasks, got %d", taskCount, summary.Successful)
	}

	if got := atomic.LoadInt64(&peak); got > limit {
		t.Errorf("Concurrency limit exceeded: peak %d concurrent spawns, limit %d", got, limit)
	}
}

// TestNewExecutorWithLimit_NonPositiveFallsBackToDefault verifies that a bad
// limit does not disable bounding (which would reintroduce the unbounded fork).
func TestNewExecutorWithLimit_NonPositiveFallsBackToDefault(t *testing.T) {
	for _, bad := range []int{0, -1, -100} {
		e := NewExecutorWithLimit(func(Task) (int, error) { return 0, nil }, bad)
		if e.maxParallel != DefaultMaxParallelTasks {
			t.Errorf("limit %d: expected fallback to %d, got %d", bad, DefaultMaxParallelTasks, e.maxParallel)
		}
	}
}
