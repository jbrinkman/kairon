package plan

import (
	"testing"
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
