package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jbrinkman/kairon/internal/provenance"
)

// IssueStore implements provenance.IssueStore with the gh CLI. The zero run
// field runs the real gh. Bodies travel on stdin (--body-file -), never in
// argv, so they cannot be read as flags or interpreted by a shell.
type IssueStore struct {
	// Repo is the target repository as owner/name.
	Repo string
	run  ghFunc
}

var _ provenance.IssueStore = IssueStore{}

// NewIssueStore returns an IssueStore for repo that runs the real gh CLI.
func NewIssueStore(repo string) IssueStore { return IssueStore{Repo: repo} }

func (s IssueStore) gh() ghFunc {
	if s.run != nil {
		return s.run
	}
	return execGH
}

// issueURLNumber matches the issue URL `gh issue create` prints.
var issueURLNumber = regexp.MustCompile(`/issues/(\d+)\s*$`)

// Revision returns the issue's updatedAt timestamp as the revision token.
// The context is accepted for the provenance.IssueStore contract; the injected
// gh runner does not take one.
func (s IssueStore) Revision(_ context.Context, number int) (string, error) {
	if err := validateTarget(s.Repo, number); err != nil {
		return "", err
	}
	out, err := s.gh()("", "issue", "view", strconv.Itoa(number), "--repo", s.Repo, "--json", "updatedAt")
	if err != nil {
		return "", fmt.Errorf("gh issue view failed: %w", err)
	}
	var view struct {
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.Unmarshal(out, &view); err != nil {
		return "", fmt.Errorf("failed to parse issue revision: %w", err)
	}
	if view.UpdatedAt == "" {
		return "", errors.New("gh issue view returned no updatedAt")
	}
	return view.UpdatedAt, nil
}

// Create makes a new issue and returns its number, parsed from the URL gh
// prints. This is a GitHub write.
func (s IssueStore) Create(_ context.Context, title, body string) (int, error) {
	if err := validateTarget(s.Repo, 1); err != nil {
		return 0, err
	}
	if strings.TrimSpace(title) == "" {
		return 0, errors.New("title must not be empty")
	}
	out, err := s.gh()(body, "issue", "create", "--repo", s.Repo, "--title", title, "--body-file", "-")
	if err != nil {
		return 0, fmt.Errorf("gh issue create failed: %w", err)
	}
	m := issueURLNumber.FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return 0, fmt.Errorf("could not parse issue number from gh output %q", strings.TrimSpace(string(out)))
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid issue number in gh output %q", strings.TrimSpace(string(out)))
	}
	return n, nil
}

// Update replaces an existing issue's body. This is a GitHub write.
func (s IssueStore) Update(_ context.Context, number int, body string) error {
	if err := validateTarget(s.Repo, number); err != nil {
		return err
	}
	if _, err := s.gh()(body, "issue", "edit", strconv.Itoa(number), "--repo", s.Repo, "--body-file", "-"); err != nil {
		return fmt.Errorf("gh issue edit failed: %w", err)
	}
	return nil
}
