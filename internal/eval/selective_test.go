package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTestCase(t *testing.T) {
	// Create a temporary test structure
	tempDir, err := os.MkdirTemp("", "kairon-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Create test case structure
	casesDir := filepath.Join(".kairon", "evals", "cases", "test-agent")
	if err := os.MkdirAll(casesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test case file
	testCaseContent := `name: test-case-1
description: A test case
input: Test input
agent: test-agent
`
	if err := os.WriteFile(filepath.Join(casesDir, "test-case-1.yaml"), []byte(testCaseContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test ValidateTestCase with existing case
	err = ValidateTestCase("test-agent", "test-case-1")
	if err != nil {
		t.Errorf("Expected nil error for existing test case, got: %v", err)
	}

	// Test ValidateTestCase with non-existing case
	err = ValidateTestCase("test-agent", "non-existing")
	if err == nil {
		t.Error("Expected error for non-existing test case, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected error message to contain 'not found', got: %v", err)
	}

	// Test ValidateTestCase with non-existing agent
	err = ValidateTestCase("non-existing-agent", "test-case-1")
	if err == nil {
		t.Error("Expected error for non-existing agent, got nil")
	}
}

func TestGetTestCase(t *testing.T) {
	// Create a temporary test structure
	tempDir, err := os.MkdirTemp("", "kairon-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Create test case structure
	casesDir := filepath.Join(".kairon", "evals", "cases", "test-agent")
	if err := os.MkdirAll(casesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test case file
	testCaseContent := `name: test-case-1
description: A test case
input: Test input
agent: test-agent
`
	if err := os.WriteFile(filepath.Join(casesDir, "test-case-1.yaml"), []byte(testCaseContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test GetTestCase with existing case
	testCase, err := GetTestCase("test-agent", "test-case-1")
	if err != nil {
		t.Errorf("Expected nil error for existing test case, got: %v", err)
	}
	if testCase == nil {
		t.Error("Expected test case, got nil")
	}
	if testCase != nil && testCase.Name != "test-case-1" {
		t.Errorf("Expected test case name 'test-case-1', got: %s", testCase.Name)
	}

	// Test GetTestCase with non-existing case
	testCase, err = GetTestCase("test-agent", "non-existing")
	if err == nil {
		t.Error("Expected error for non-existing test case, got nil")
	}
	if testCase != nil {
		t.Error("Expected nil test case for non-existing case, got non-nil")
	}
}

func TestListAvailableTestCases(t *testing.T) {
	// Create a temporary test structure
	tempDir, err := os.MkdirTemp("", "kairon-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Create test case structure
	casesDir := filepath.Join(".kairon", "evals", "cases", "test-agent")
	if err := os.MkdirAll(casesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple test case files
	for i := 1; i <= 3; i++ {
		testCaseContent := fmt.Sprintf(`name: test-case-%d
description: Test case %d
input: Test input %d
agent: test-agent
`, i, i, i)
		filename := fmt.Sprintf("test-case-%d.yaml", i)
		if err := os.WriteFile(filepath.Join(casesDir, filename), []byte(testCaseContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Test ListAvailableTestCases
	names, err := ListAvailableTestCases("test-agent")
	if err != nil {
		t.Errorf("Expected nil error, got: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("Expected 3 test cases, got: %d", len(names))
	}

	// Verify names are correct
	expectedNames := []string{"test-case-1", "test-case-2", "test-case-3"}
	for _, expected := range expectedNames {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find test case '%s' in list: %v", expected, names)
		}
	}
}
