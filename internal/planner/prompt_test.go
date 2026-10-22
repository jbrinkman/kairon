package planner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// promptEnv names a prompt file to check instead of the live one, so a
// candidate can be checked before it is promoted:
//
//	KAIRON_PLANNER_PROMPT=$PWD/.kairon/iterations/planner/v1.md \
//	    go test ./internal/planner -run TestPlannerPrompt
const promptEnv = "KAIRON_PLANNER_PROMPT"

// livePromptRel is the live planner prompt, relative to this package.
var livePromptRel = filepath.Join("..", "..", ".kiro", "agents", "planner-prompt.md")

// forbiddenInPrompt are strings whose presence means the prompt tells the
// planner to write to GitHub itself, which is the defect issue #381 fixes: the
// planner returns a document and the boundary (Boundary.Handle) writes.
var forbiddenInPrompt = []string{
	"gh issue create",
	"--body-file",
	"/tmp/issue-body.md",
}

// requiredInPrompt are anchors the new prompt must carry: the mode line, the
// envelope version, the punch-out result, the two contract keys and the
// placeholder form for split chains.
var requiredInPrompt = []string{
	AutomatedIntakeLine,
	"schema_version: 1",
	"result: punch_out",
	"unresolved",
	"inferences",
	"#{issue-",
}

// loadPlannerPrompt reads the prompt under test. A missing or empty file is a
// failure, never a skip: a guard that silently checks nothing is worse than
// none.
func loadPlannerPrompt(t *testing.T) (path, text string) {
	t.Helper()
	path = livePromptRel
	if p := os.Getenv(promptEnv); p != "" {
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read planner prompt %s: %v", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Fatalf("planner prompt %s is empty", path)
	}
	return path, string(data)
}

// TestPlannerPromptHasNoGitHubWrite is the regression guard for the original
// defect: the prompt instructed the planner to run gh issue create.
func TestPlannerPromptHasNoGitHubWrite(t *testing.T) {
	path, text := loadPlannerPrompt(t)
	for _, s := range forbiddenInPrompt {
		if strings.Contains(text, s) {
			t.Errorf("%s contains %q: the planner must return a stage-output document and leave the GitHub write to the boundary", path, s)
		}
	}
}

// TestPlannerPromptCarriesContractAnchors checks the prompt still names the
// pieces of the output contract. It is deliberately a presence check only; the
// eval suite certifies behavior.
func TestPlannerPromptCarriesContractAnchors(t *testing.T) {
	path, text := loadPlannerPrompt(t)
	for _, s := range requiredInPrompt {
		if !strings.Contains(text, s) {
			t.Errorf("%s does not contain %q", path, s)
		}
	}
}
