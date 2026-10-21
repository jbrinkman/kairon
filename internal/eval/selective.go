package eval

import (
	"fmt"
	"os"
	"strings"
)

// ListAvailableTestCases returns all test case names for an agent.
func ListAvailableTestCases(agent string) ([]string, error) {
	cases, err := loadCases(agent)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, tc := range cases {
		names = append(names, tc.Name)
	}
	return names, nil
}

// ValidateTestCase checks if a test case exists for the given agent.
func ValidateTestCase(agent, testcase string) error {
	cases, err := loadCases(agent)
	if err != nil {
		return fmt.Errorf("failed to load test cases for %s: %w", agent, err)
	}

	for _, tc := range cases {
		if tc.Name == testcase {
			return nil
		}
	}

	// Provide helpful error with suggestions
	var available []string
	for _, tc := range cases {
		available = append(available, tc.Name)
	}

	if len(available) == 0 {
		return fmt.Errorf("test case '%s' not found for agent '%s' (no test cases available)", testcase, agent)
	}

	return fmt.Errorf("test case '%s' not found for agent '%s'\nAvailable: %s", testcase, agent, strings.Join(available, ", "))
}

// GetTestCase retrieves a specific test case for an agent.
func GetTestCase(agent, testcase string) (*TestCase, error) {
	cases, err := loadCases(agent)
	if err != nil {
		return nil, err
	}

	for _, tc := range cases {
		if tc.Name == testcase {
			return &tc, nil
		}
	}

	return nil, fmt.Errorf("test case '%s' not found for agent '%s'", testcase, agent)
}

// PrintTestCaseList displays available test cases for an agent in a user-friendly format.
// It honors the run's --filter and shows each case's metadata.
func PrintTestCaseList(agent string) error {
	return printTestCaseList(os.Stdout, agent, cfg.filters)
}
