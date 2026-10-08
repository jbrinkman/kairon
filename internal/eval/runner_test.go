package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScoreDeterministic_AcceptanceCriteriaQuality(t *testing.T) {
	criterion := Criterion{
		Name:    "acceptance_criteria_quality",
		Scoring: "1-5",
	}

	tests := []struct {
		name          string
		output        string
		expectedScore int
		expectedSkip  bool
	}{
		{
			name:          "high quality - checkboxes and commands",
			output:        "- [ ] Run `go build ./...` and verify exit code 0\n- [ ] Run `go test ./...` returns no failures\n- [ ] `curl /api/health` returns status code 200",
			expectedScore: 5,
			expectedSkip:  false,
		},
		{
			name:          "medium quality - some testable criteria",
			output:        "- [ ] The API returns 200 on success\n- The implementation should be clean",
			expectedScore: 3,
			expectedSkip:  false,
		},
		{
			name:          "low quality - vague criteria",
			output:        "The feature should work well and be performant.",
			expectedScore: 1,
			expectedSkip:  false,
		},
		{
			name:          "no output",
			output:        "",
			expectedScore: 0,
			expectedSkip:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TestCase{}
			score, _, skipped := scoreDeterministic(criterion, tc, tt.output, "", "")
			if score != tt.expectedScore {
				t.Errorf("expected score %d, got %d", tt.expectedScore, score)
			}
			if skipped != tt.expectedSkip {
				t.Errorf("expected skipped %v, got %v", tt.expectedSkip, skipped)
			}
		})
	}
}

func TestScoreDeterministic_TestExecution(t *testing.T) {
	criterion := Criterion{
		Name:    "test_execution",
		Scoring: "1-5",
	}

	tests := []struct {
		name          string
		output        string
		expectedScore int
		expectedSkip  bool
	}{
		{
			name:          "clear execution with exit code and test output",
			output:        "$ go test ./...\nok  \tgithub.com/example/pkg\t0.042s\nexit code 0",
			expectedScore: 5,
			expectedSkip:  false,
		},
		{
			name:          "execution with PASS/FAIL markers",
			output:        "$ go test -v ./...\n--- PASS: TestFeature (0.01s)\nPASS",
			expectedScore: 5,
			expectedSkip:  false,
		},
		{
			name:          "single execution indicator",
			output:        "Verified by checking exit code of the build step.",
			expectedScore: 2,
			expectedSkip:  false,
		},
		{
			name:          "no execution evidence",
			output:        "The implementation looks correct based on code review.",
			expectedScore: 1,
			expectedSkip:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TestCase{}
			score, _, skipped := scoreDeterministic(criterion, tc, tt.output, "", "")
			if score != tt.expectedScore {
				t.Errorf("expected score %d, got %d", tt.expectedScore, score)
			}
			if skipped != tt.expectedSkip {
				t.Errorf("expected skipped %v, got %v", tt.expectedSkip, skipped)
			}
		})
	}
}

func TestScoreDeterministic_CodeCorrectness(t *testing.T) {
	criterion := Criterion{
		Name:    "code_correctness",
		Scoring: "1-5",
	}

	tests := []struct {
		name          string
		output        string
		expectedScore int
		expectedSkip  bool
	}{
		{
			name:          "build passes with no errors",
			output:        "Build passes: go build ./...\nno errors found\nexit code 0",
			expectedScore: 5,
			expectedSkip:  false,
		},
		{
			name:          "compiled successfully",
			output:        "The code compiled successfully and all tests pass.",
			expectedScore: 5,
			expectedSkip:  false,
		},
		{
			name:          "syntax error present",
			output:        "Build failed: syntax error in cmd/main.go:15",
			expectedScore: 1,
			expectedSkip:  false,
		},
		{
			name:          "no build evidence",
			output:        "The feature has been implemented as requested.",
			expectedScore: 2,
			expectedSkip:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TestCase{}
			score, _, skipped := scoreDeterministic(criterion, tc, tt.output, "", "")
			if score != tt.expectedScore {
				t.Errorf("expected score %d, got %d", tt.expectedScore, score)
			}
			if skipped != tt.expectedSkip {
				t.Errorf("expected skipped %v, got %v", tt.expectedSkip, skipped)
			}
		})
	}
}

func TestScoreDeterministic_TestCoverage(t *testing.T) {
	criterion := Criterion{
		Name:    "test_coverage",
		Scoring: "1-5",
	}

	tests := []struct {
		name          string
		output        string
		expectedScore int
		expectedSkip  bool
	}{
		{
			name:          "test files and execution",
			output:        "Created internal/auth/handler_test.go with func TestHandleLogin.\nRan go test ./internal/auth/...",
			expectedScore: 5,
			expectedSkip:  false,
		},
		{
			name:          "test file reference only",
			output:        "Added user_test.go with unit tests for the new service.",
			expectedScore: 2,
			expectedSkip:  false,
		},
		{
			name:          "no test indicators",
			output:        "The feature has been implemented with proper error handling.",
			expectedScore: 1,
			expectedSkip:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TestCase{}
			score, _, skipped := scoreDeterministic(criterion, tc, tt.output, "", "")
			if score != tt.expectedScore {
				t.Errorf("expected score %d, got %d", tt.expectedScore, score)
			}
			if skipped != tt.expectedSkip {
				t.Errorf("expected skipped %v, got %v", tt.expectedSkip, skipped)
			}
		})
	}
}

func TestScoreDeterministic_ExistingCheckers(t *testing.T) {
	tests := []struct {
		name       string
		criterion  Criterion
		output     string
		expectSkip bool
	}{
		{
			name: "completeness checker",
			criterion: Criterion{
				Name:    "completeness",
				Scoring: "1-5",
			},
			output:     "## Overview\n### Details",
			expectSkip: false,
		},
		{
			name: "file_naming checker",
			criterion: Criterion{
				Name:    "file_naming",
				Scoring: "1-5",
			},
			output:     "Created app_docs/feature-auth.md",
			expectSkip: false,
		},
		{
			name: "unknown checker skips",
			criterion: Criterion{
				Name:    "unknown_checker",
				Scoring: "1-5",
			},
			output:     "some output",
			expectSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TestCase{}
			_, _, skipped := scoreDeterministic(tt.criterion, tc, tt.output, "", "")
			if skipped != tt.expectSkip {
				t.Errorf("expected skipped %v, got %v", tt.expectSkip, skipped)
			}
		})
	}
}

func TestScoreLLMJudge_ANSISequences(t *testing.T) {
	tests := []struct {
		name        string
		rawResponse string
		expectError bool
	}{
		{
			name:        "valid JSON with ANSI color sequences",
			rawResponse: "===JSON_START===\n\x1B[38;5;141m{\"score\": 4, \"reasoning\": \"Good implementation\", \"pass\": true}\x1B[0m\n===JSON_END===",
			expectError: false,
		},
		{
			name:        "valid JSON with multiple ANSI sequences",
			rawResponse: "===JSON_START===\n\x1B[1m\x1B[32m{\"score\": 5, \"reasoning\": \"Excellent work\", \"pass\": true}\x1B[0m\x1B[39m\n===JSON_END===",
			expectError: false,
		},
		{
			name:        "valid JSON with bold and reset ANSI",
			rawResponse: "===JSON_START===\n\x1B[1m{\"score\": 3, \"reasoning\": \"Adequate\", \"pass\": true}\x1B[0m\n===JSON_END===",
			expectError: false,
		},
		{
			// Verifies ANSI sequences inside JSON string values are also stripped.
			// This intentionally mutates content—acceptable since ANSI codes in
			// LLM output are rendering artifacts, not meaningful data.
			name:        "ANSI sequences within JSON string values",
			rawResponse: "===JSON_START===\n{\"score\": 2, \"reasoning\": \"\x1B[31mNeeds improvement\x1B[0m\", \"pass\": false}\n===JSON_END===",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract JSON from the mock response
			start := strings.Index(tt.rawResponse, "===JSON_START===")
			end := strings.Index(tt.rawResponse, "===JSON_END===")
			if start == -1 || end == -1 {
				t.Fatal("Invalid test data: missing JSON delimiters")
			}
			jsonStr := tt.rawResponse[start+len("===JSON_START===") : end]
			jsonStr = stripANSISequences(strings.TrimSpace(jsonStr))

			var response struct {
				Score     int    `json:"score"`
				Reasoning string `json:"reasoning"`
				Pass      bool   `json:"pass"`
			}

			err := json.Unmarshal([]byte(jsonStr), &response)

			if tt.expectError && err == nil {
				t.Error("expected JSON parse error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("unexpected JSON parse error: %v", err)
			} else if !tt.expectError {
				// Verify we got valid parsed data
				if response.Score < 1 || response.Score > 5 {
					t.Errorf("invalid score: %d", response.Score)
				}
				if response.Reasoning == "" {
					t.Error("empty reasoning")
				}
			}
		})
	}
}

// TestScoreDeterministic_FileReferenceWorkspace verifies that the
// file_reference scorer resolves references against the case workspace (where
// the agent runs) — relative references natively, and absolute references
// reported under the container workspace path for a sandbox run — not against
// the harness process directory.
func TestScoreDeterministic_FileReferenceWorkspace(t *testing.T) {
	criterion := Criterion{Name: "file_reference_accuracy", Scoring: "1-5"}

	// Workspace that contains the referenced file.
	wsWith := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsWith, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsWith, "docs", "result.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Workspace that does not.
	wsWithout := t.TempDir()

	tc := TestCase{}
	const containerWS = "/workspace"
	// "1-5" scoring => max score 5.

	// Relative reference (native run): resolved against the host workspace.
	relOut := "I created the design at `docs/result.md` as requested."
	if score, _, _ := scoreDeterministic(criterion, tc, relOut, wsWith, ""); score != 5 {
		t.Errorf("relative ref, file present: score = %d, want 5", score)
	}
	if score, _, _ := scoreDeterministic(criterion, tc, relOut, wsWithout, ""); score != 1 {
		t.Errorf("relative ref, file absent: score = %d, want 1", score)
	}

	// Absolute container reference (sandbox run): /workspace/... is rewritten
	// onto the host workspace, not stat'd on the host as a literal path.
	absOut := "Wrote the design to `/workspace/docs/result.md` for review."
	if score, _, _ := scoreDeterministic(criterion, tc, absOut, wsWith, containerWS); score != 5 {
		t.Errorf("container-absolute ref, file present: score = %d, want 5", score)
	}
	if score, _, _ := scoreDeterministic(criterion, tc, absOut, wsWithout, containerWS); score != 1 {
		t.Errorf("container-absolute ref, file absent: score = %d, want 1", score)
	}
}
