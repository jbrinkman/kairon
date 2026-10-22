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

var _ provenance.IssueStore = IssueStore{}

// IssueStore implements provenance.IssueStore with the gh CLI. Every call is an
// argument list (never a shell), and bodies travel on stdin with
// `--body-file -`, so they can never be read as a flag or interpreted by a
// shell. The zero value for run uses the real gh.
type IssueStore struct {
	// Repo is the target repository as owner/name.
	Repo string
	// Labels are applied to issues made by Create. Update never changes labels.
	Labels []string

	run ghFunc
}

func (s IssueStore) gh() ghFunc {
	if s.run != nil {
		return s.run
	}
	return execGH
}

// Revision returns the issue's updatedAt timestamp as its revision token.
func (s IssueStore) Revision(ctx context.Context, number int) (string, error) {
	if err := validateTarget(s.Repo, number); err != nil {
		return "", fmt.Errorf("revision of issue #%d: %w", number, err)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("revision of issue #%d: %w", number, err)
	}
	out, err := s.gh()("", "issue", "view", strconv.Itoa(number), "--repo", s.Repo, "--json", "updatedAt")
	if err != nil {
		return "", fmt.Errorf("gh issue view failed (revision of issue #%d): %w", number, err)
	}
	var v struct {
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("revision of issue #%d: failed to parse gh output: %w", number, err)
	}
	if v.UpdatedAt == "" {
		return "", fmt.Errorf("revision of issue #%d: gh output has no updatedAt", number)
	}
	return v.UpdatedAt, nil
}

// issueURLNumber matches the trailing issue number of the URL gh prints.
var issueURLNumber = regexp.MustCompile(`/issues/(\d+)\s*$`)

// Create makes a new issue with the configured labels and returns its number.
// This is a GitHub write.
func (s IssueStore) Create(ctx context.Context, title, body string) (int, error) {
	if strings.TrimSpace(s.Repo) == "" {
		return 0, errors.New("create issue: repo must not be empty")
	}
	if strings.TrimSpace(title) == "" {
		return 0, errors.New("create issue: title must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("create issue: %w", err)
	}
	args := []string{"issue", "create", "--repo", s.Repo, "--title", title, "--body-file", "-"}
	for _, l := range s.Labels {
		args = append(args, "--label", l)
	}
	out, err := s.gh()(body, args...)
	if err != nil {
		return 0, fmt.Errorf("gh issue create failed: %w", err)
	}
	m := issueURLNumber.FindStringSubmatch(string(out))
	if m == nil {
		return 0, fmt.Errorf("create issue: could not find an issue number in gh output %q", strings.TrimSpace(string(out)))
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("create issue: invalid issue number in gh output %q", strings.TrimSpace(string(out)))
	}
	return n, nil
}

// Update replaces an existing issue's body. This is a GitHub write.
func (s IssueStore) Update(ctx context.Context, number int, body string) error {
	if err := validateTarget(s.Repo, number); err != nil {
		return fmt.Errorf("update issue #%d: %w", number, err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update issue #%d: %w", number, err)
	}
	if _, err := s.gh()(body, "issue", "edit", strconv.Itoa(number), "--repo", s.Repo, "--body-file", "-"); err != nil {
		return fmt.Errorf("gh issue edit failed (update of issue #%d): %w", number, err)
	}
	return nil
}
