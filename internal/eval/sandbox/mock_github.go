package sandbox

import (
	"embed"
)

// MockGitHubSkill embeds the mock github-cli skill. Nothing installs it into a
// container any more (the harness no longer copies content into a running
// container); it is kept for the containment work in #298, which decides how a
// fake gh is provided.
//
//go:embed testdata/github-cli-mock/*
var MockGitHubSkill embed.FS

// GitHubMockResponse represents a mock GitHub API response
type GitHubMockResponse struct {
	IssueNumber int    `json:"issue_number,omitempty"`
	PRNumber    int    `json:"pr_number,omitempty"`
	URL         string `json:"url,omitempty"`
	Status      string `json:"status,omitempty"`
}

// SimulateGitHubResponse returns realistic GitHub API responses for testing
func SimulateGitHubResponse(operation string, args []string) GitHubMockResponse {
	switch operation {
	case "issue":
		if len(args) > 0 && args[0] == "create" {
			return GitHubMockResponse{
				IssueNumber: 12345,
				URL:         "https://github.com/test/test/issues/12345",
			}
		}
		return GitHubMockResponse{Status: "success"}

	case "pr":
		if len(args) > 0 && args[0] == "create" {
			return GitHubMockResponse{
				PRNumber: 42,
				URL:      "https://github.com/test/test/pull/42",
			}
		}
		return GitHubMockResponse{Status: "success"}

	default:
		return GitHubMockResponse{Status: "success"}
	}
}
