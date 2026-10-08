package sandbox

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fakeGHScript is the POSIX sh fake `gh` placed first on PATH in a sandboxed
// run. It logs every call to $KAIRON_EVAL_DIR/gh.log, copies --body-file
// bodies to gh-body-<n>.md, answers `gh issue view` from gh-issue.json /
// gh-issue.txt and exits 1 for every command it does not simulate.
//
//go:embed fakegh/gh
var fakeGHScript []byte

// FakeGHFileName is the name of the fake gh script inside the directory passed
// to WriteFakeGH.
const FakeGHFileName = "gh"

// Defaults applied by RenderGHIssue when a GHIssue field is left empty.
const (
	DefaultGHIssueNumber = 1
	DefaultGHIssueState  = "OPEN"
	DefaultGHIssueAuthor = "fake-user"
)

// fakeGHRepoURL is the repository URL the fake gh reports for issues.
const fakeGHRepoURL = "https://github.com/fake-owner/fake-repo"

// GHIssue is the data a case configures for `gh issue view` under --sandbox.
type GHIssue struct {
	Number int      `yaml:"number,omitempty" json:"number,omitempty"`
	Title  string   `yaml:"title" json:"title"`
	Body   string   `yaml:"body,omitempty" json:"body,omitempty"`
	State  string   `yaml:"state,omitempty" json:"state,omitempty"`
	Author string   `yaml:"author,omitempty" json:"author,omitempty"`
	Labels []string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// WriteFakeGH writes the fake gh script to <dir>/gh with mode 0755, creating
// dir (0755) when needed. It installs nothing into a container: callers mount
// the directory read-only.
func WriteFakeGH(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create fake gh dir: %w", err)
	}
	path := filepath.Join(dir, FakeGHFileName)
	if err := os.WriteFile(path, fakeGHScript, 0o755); err != nil {
		return fmt.Errorf("write fake gh: %w", err)
	}
	// WriteFile does not change the mode of an existing file and honours the
	// umask for new ones, so set the mode explicitly.
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("chmod fake gh: %w", err)
	}
	return nil
}

// RenderGHIssue renders the issue for the fake gh. jsonDoc is valid JSON with
// exactly one top-level field per line (so the script can select --json fields
// without jq); text is the human `gh issue view` output. Empty Number, State
// and Author take their defaults; an empty Title is an error.
func RenderGHIssue(issue GHIssue) (jsonDoc []byte, text string, err error) {
	if strings.TrimSpace(issue.Title) == "" {
		return nil, "", fmt.Errorf("gh_issue: title is required")
	}
	if issue.Number < 0 {
		return nil, "", fmt.Errorf("gh_issue: number must be positive, got %d", issue.Number)
	}
	if issue.Number == 0 {
		issue.Number = DefaultGHIssueNumber
	}
	if issue.State == "" {
		issue.State = DefaultGHIssueState
	}
	if issue.Author == "" {
		issue.Author = DefaultGHIssueAuthor
	}

	type login struct {
		Login string `json:"login"`
	}
	type label struct {
		Name string `json:"name"`
	}
	labels := make([]label, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, label{Name: l})
	}

	// Field order is fixed so the output is deterministic.
	fields := []struct {
		name  string
		value any
	}{
		{"author", login{Login: issue.Author}},
		{"body", issue.Body},
		{"labels", labels},
		{"number", issue.Number},
		{"state", issue.State},
		{"title", issue.Title},
		{"url", fmt.Sprintf("%s/issues/%d", fakeGHRepoURL, issue.Number)},
	}

	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, f := range fields {
		val, mErr := marshalCompact(f.value)
		if mErr != nil {
			return nil, "", fmt.Errorf("gh_issue: encode %s: %w", f.name, mErr)
		}
		fmt.Fprintf(&buf, "  %q: %s", f.name, val)
		if i < len(fields)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")

	text = fmt.Sprintf("title:\t%s\nstate:\t%s\nauthor:\t%s\nlabels:\t%s\ncomments:\t0\nassignees:\t\nprojects:\t\nmilestone:\t\nnumber:\t%d\n--\n%s\n",
		singleLine(issue.Title), issue.State, issue.Author, strings.Join(issue.Labels, ", "), issue.Number, issue.Body)
	return buf.Bytes(), text, nil
}

// marshalCompact encodes v as single-line JSON without HTML escaping.
func marshalCompact(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// singleLine folds newlines so a header value stays on one line.
func singleLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}
