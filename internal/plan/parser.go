package plan

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParsePlanFromMarkdown extracts and parses a plan artifact from markdown content.
// It searches for a fenced code block with the language identifier 'kiro-plan',
// extracts the YAML content, and unmarshals it into a Plan struct.
//
// Returns:
//   - (*Plan, nil) if a valid plan is found and parsed successfully
//   - (nil, nil) if no plan block is found (backward compatibility)
//   - (nil, error) if a plan block is found but is invalid or multiple blocks exist
func ParsePlanFromMarkdown(content []byte) (*Plan, error) {
	// Regex to match the opening fence with the 'kiro-plan' language identifier.
	// Per CommonMark, a fence is a run of at least three backticks or at least
	// three tildes; capture the exact fence so the closing fence can be paired
	// to it (same character, length >= opener).
	pattern := regexp.MustCompile("(?m)^([`~]{3,})\\s*kiro-plan\\s*$")
	matches := pattern.FindAllSubmatchIndex(content, -1)

	if len(matches) == 0 {
		// No plan block found - this is okay for backward compatibility
		return nil, nil
	}

	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple plan blocks found (expected at most 1, found %d)", len(matches))
	}

	// Capture the opening fence string (group 1) to pair the closing fence.
	openFence := string(content[matches[0][2]:matches[0][3]])
	fenceChar := openFence[0]
	fenceLen := len(openFence)

	// Extract the YAML content between the opening and closing fence
	startIdx := matches[0][1] // End of opening fence marker line

	// The closing fence must use the SAME character as the opener and be at
	// least as long (CommonMark). A shorter or different fence does not close.
	closingPattern := regexp.MustCompile(fmt.Sprintf("(?m)^%c{%d,}\\s*$", fenceChar, fenceLen))
	remaining := content[startIdx:]
	closingMatch := closingPattern.FindIndex(remaining)

	if closingMatch == nil {
		return nil, fmt.Errorf("unclosed plan code block (no closing fence found)")
	}

	endIdx := startIdx + closingMatch[0]
	yamlContent := content[startIdx:endIdx]

	// Trim leading/trailing whitespace from YAML content
	yamlStr := strings.TrimSpace(string(yamlContent))

	if yamlStr == "" {
		return nil, fmt.Errorf("empty plan block")
	}

	// Parse YAML into Plan struct.
	// Use a decoder with KnownFields(true) so unknown/misspelled keys (e.g. a
	// mistyped "dependencies") are rejected rather than silently dropped, which
	// would otherwise corrupt dependency ordering without the validator noticing.
	var plan Plan
	decoder := yaml.NewDecoder(strings.NewReader(yamlStr))
	decoder.KnownFields(true)
	if err := decoder.Decode(&plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan YAML: %w", err)
	}

	return &plan, nil
}

// ParsePlanFromFile reads a file and parses its plan artifact
func ParsePlanFromFile(filePath string) (*Plan, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return ParsePlanFromMarkdown(content)
}
