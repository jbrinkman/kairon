package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlanFromMarkdown_ValidPlan(t *testing.T) {
	markdown := []byte(`
# Design Specification

This is a test specification.

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Implement feature A"
    dependencies: []
    acceptance_criteria:
      - "Feature works"
    validation_commands:
      - "go test"
` + "```" + `

More content after the plan.
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected successful parse, got error: %v", err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if plan.Version != "1.0" {
		t.Errorf("expected version '1.0', got '%s'", plan.Version)
	}

	if len(plan.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(plan.Tasks))
	}

	if plan.Tasks[0].ID != "task-1" {
		t.Errorf("expected task ID 'task-1', got '%s'", plan.Tasks[0].ID)
	}
}

func TestParsePlanFromMarkdown_NoPlan(t *testing.T) {
	markdown := []byte(`
# Design Specification

This is a specification without a plan.

Some code example:
` + "```go" + `
func main() {
    fmt.Println("Hello")
}
` + "```" + `
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err != nil {
		t.Errorf("expected no error for missing plan, got: %v", err)
	}

	if plan != nil {
		t.Error("expected nil plan when no plan block exists")
	}
}

func TestParsePlanFromMarkdown_MultiplePlans(t *testing.T) {
	markdown := []byte(`
# Design Specification

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "First plan"
    dependencies: []
` + "```" + `

Some text.

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-2"
    agent: "builder"
    description: "Second plan"
    dependencies: []
` + "```" + `
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err == nil {
		t.Error("expected error for multiple plan blocks")
	}

	if plan != nil {
		t.Error("expected nil plan when multiple blocks exist")
	}
}

func TestParsePlanFromMarkdown_UnclosedBlock(t *testing.T) {
	markdown := []byte(`
# Design Specification

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Unclosed plan"
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err == nil {
		t.Error("expected error for unclosed plan block")
	}

	if plan != nil {
		t.Error("expected nil plan for unclosed block")
	}
}

func TestParsePlanFromMarkdown_EmptyBlock(t *testing.T) {
	markdown := []byte(`
# Design Specification

` + "```kiro-plan" + `
` + "```" + `
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err == nil {
		t.Error("expected error for empty plan block")
	}

	if plan != nil {
		t.Error("expected nil plan for empty block")
	}
}

func TestParsePlanFromMarkdown_InvalidYAML(t *testing.T) {
	markdown := []byte(`
# Design Specification

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: [invalid yaml syntax
    description: "Invalid"
` + "```" + `
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}

	if plan != nil {
		t.Error("expected nil plan for invalid YAML")
	}
}

func TestParsePlanFromMarkdown_MultipleTasksPlan(t *testing.T) {
	markdown := []byte(`
# Design Specification

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "First task"
    dependencies: []
    acceptance_criteria:
      - "Criterion 1"
    validation_commands:
      - "command 1"
  
  - id: "task-2"
    agent: "validator"
    description: "Second task"
    dependencies: ["task-1"]
    acceptance_criteria:
      - "Criterion 2"
    validation_commands:
      - "command 2"
  
  - id: "task-3"
    agent: "builder"
    description: "Third task"
    dependencies: ["task-1", "task-2"]
    acceptance_criteria:
      - "Criterion 3"
    validation_commands:
      - "command 3"
` + "```" + `
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected successful parse, got error: %v", err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if len(plan.Tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(plan.Tasks))
	}

	// Verify task 1
	if plan.Tasks[0].ID != "task-1" {
		t.Errorf("expected task-1, got %s", plan.Tasks[0].ID)
	}
	if len(plan.Tasks[0].Dependencies) != 0 {
		t.Errorf("expected 0 dependencies for task-1, got %d", len(plan.Tasks[0].Dependencies))
	}

	// Verify task 2
	if plan.Tasks[1].ID != "task-2" {
		t.Errorf("expected task-2, got %s", plan.Tasks[1].ID)
	}
	if len(plan.Tasks[1].Dependencies) != 1 || plan.Tasks[1].Dependencies[0] != "task-1" {
		t.Errorf("expected task-2 to depend on task-1, got %v", plan.Tasks[1].Dependencies)
	}

	// Verify task 3
	if plan.Tasks[2].ID != "task-3" {
		t.Errorf("expected task-3, got %s", plan.Tasks[2].ID)
	}
	if len(plan.Tasks[2].Dependencies) != 2 {
		t.Errorf("expected 2 dependencies for task-3, got %d", len(plan.Tasks[2].Dependencies))
	}
}

func TestParsePlanFromMarkdown_WhitespaceHandling(t *testing.T) {
	markdown := []byte(`
# Design Specification

` + "```kiro-plan    " + `

version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Test task"
    dependencies: []

` + "```" + `
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected successful parse with whitespace, got error: %v", err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if len(plan.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(plan.Tasks))
	}
}

func TestParsePlanFromFile_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "spec.md")

	content := []byte(`
# Specification

` + "```kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Test task"
    dependencies: []
` + "```" + `
`)

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	plan, err := ParsePlanFromFile(filePath)
	if err != nil {
		t.Fatalf("expected successful parse, got error: %v", err)
	}

	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if plan.Tasks[0].ID != "task-1" {
		t.Errorf("expected task-1, got %s", plan.Tasks[0].ID)
	}
}

func TestParsePlanFromFile_NonexistentFile(t *testing.T) {
	_, err := ParsePlanFromFile("/nonexistent/file.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParsePlanFromFile_NoPlan(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "spec.md")

	content := []byte(`
# Specification

This file has no plan.
`)

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	plan, err := ParsePlanFromFile(filePath)
	if err != nil {
		t.Errorf("expected no error for missing plan, got: %v", err)
	}

	if plan != nil {
		t.Error("expected nil plan when no plan block exists")
	}
}

// TestParsePlanFromMarkdown_TildeFence verifies a ~~~kiro-plan block is parsed
// (previously the backtick-only opening regex silently skipped it).
func TestParsePlanFromMarkdown_TildeFence(t *testing.T) {
	markdown := []byte(`
# Design Specification

~~~kiro-plan
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: "Tilde-fenced plan"
    dependencies: []
    acceptance_criteria:
      - "works"
    validation_commands:
      - "go test"
~~~
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected successful parse of tilde fence, got error: %v", err)
	}
	if plan == nil {
		t.Fatal("expected non-nil plan from tilde fence")
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].ID != "task-1" {
		t.Errorf("expected one task 'task-1', got %+v", plan.Tasks)
	}
}

// TestParsePlanFromMarkdown_ClosingFenceBoundary verifies that a shorter inner
// backtick fence does NOT prematurely close a longer (4-backtick) opening fence.
func TestParsePlanFromMarkdown_ClosingFenceBoundary(t *testing.T) {
	markdown := []byte(`
# Design Specification

` + "````kiro-plan" + `
version: "1.0"
tasks:
  - id: "task-1"
    agent: "builder"
    description: |
      An inner shorter fence should not close the block:
      ` + "```" + `
      still inside the plan
      ` + "```" + `
    dependencies: []
    acceptance_criteria:
      - "works"
    validation_commands:
      - "go test"
` + "````" + `
`)

	plan, err := ParsePlanFromMarkdown(markdown)
	if err != nil {
		t.Fatalf("expected successful parse, got error: %v", err)
	}
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].ID != "task-1" {
		t.Errorf("expected one task 'task-1' (block not closed early), got %+v", plan.Tasks)
	}
}
