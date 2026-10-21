package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrRateLimited is returned when the GitHub API rate limit is exceeded.
var ErrRateLimited = errors.New("GitHub API rate limit exceeded")

// ErrNoReleases indicates no releases exist for the repository
var ErrNoReleases = errors.New("no releases found")

type Label struct {
	Name string `json:"name"`
}

type Issue struct {
	Number int     `json:"number"`
	Title  string  `json:"title"`
	Body   string  `json:"body"`
	Labels []Label `json:"labels"`
}

type IssueDetails struct {
	Body  string `json:"body"`
	State string `json:"state"`
}

func GetToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetAuthenticatedUser returns the username of the authenticated GitHub user.
func GetAuthenticatedUser() (string, error) {
	cmd := exec.Command("gh", "api", "user", "--jq", ".login")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh api user failed: %w", err)
	}
	user := strings.TrimSpace(string(output))
	if user == "" {
		return "", fmt.Errorf("gh api user returned an empty login")
	}
	return user, nil
}

func GetIssueDetails(repo string, number int) (*IssueDetails, error) {
	cmd := exec.Command("gh", "issue", "view", fmt.Sprintf("%d", number), "--repo", repo, "--json", "body,state")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh issue view failed: %w", err)
	}

	var details IssueDetails
	if err := json.Unmarshal(output, &details); err != nil {
		return nil, fmt.Errorf("failed to parse issue details: %w", err)
	}

	return &details, nil
}

// ListIssues returns the open issues with label that may be processed: issues
// labeled <label>-done or <label>-failed, and issues awaiting a completed
// punch-out, are excluded. See SelectIssues to also get the latter.
func ListIssues(repo, label, assignee string) ([]Issue, error) {
	return listIssues(execGH, repo, label, assignee)
}

func AddLabel(repo string, issueNumber int, label string) error {
	cmd := exec.Command("gh", "issue", "edit", fmt.Sprintf("%d", issueNumber), "--repo", repo, "--add-label", label)
	return cmd.Run()
}

func PRExistsForIssue(repo string, issueNumber int) (bool, error) {
	cmd := exec.Command("gh", "pr", "list", "--repo", repo, "--search", fmt.Sprintf("head:spec/issue-%d-", issueNumber), "--json", "number")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("gh pr list failed: %w", err)
	}

	var prs []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(output, &prs); err != nil {
		return false, fmt.Errorf("failed to parse gh output: %w", err)
	}

	return len(prs) > 0, nil
}

type PullRequest struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
}

// VerifyPRExists checks if a PR exists with the expected branch name pattern
func VerifyPRExists(repo string, issueNumber, pid int) (bool, error) {
	expectedBranch := fmt.Sprintf("spec/issue-%d-%d", issueNumber, pid)

	cmd := exec.Command("gh", "pr", "list", "--repo", repo, "--head", expectedBranch, "--json", "number,headRefName")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("gh pr list failed: %w", err)
	}

	var prs []PullRequest
	if err := json.Unmarshal(output, &prs); err != nil {
		return false, fmt.Errorf("failed to parse gh output: %w", err)
	}

	return len(prs) > 0, nil
}

type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

// GetLatestRelease fetches the latest release from GitHub
func GetLatestRelease(repo string) (*Release, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "api", fmt.Sprintf("repos/%s/releases", repo), "--jq", ".[0] | {tag_name, name}")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("gh api releases timed out after 10 seconds")
		}
		return nil, fmt.Errorf("gh api releases failed: %w", err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" {
		return nil, ErrNoReleases
	}

	var release Release
	if err := json.Unmarshal([]byte(trimmed), &release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	if strings.TrimSpace(release.TagName) == "" {
		return nil, ErrNoReleases
	}

	return &release, nil
}

// isRateLimited checks if an exec error from the gh CLI indicates a rate limit.
func isRateLimited(err error) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr := string(exitErr.Stderr)
		return strings.Contains(stderr, "rate limit") || strings.Contains(stderr, "API rate limit exceeded")
	}
	return false
}
