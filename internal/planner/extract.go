package planner

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNotOneDocument is matched (errors.Is) by every Extract failure.
var ErrNotOneDocument = errors.New("planner: reply is not exactly one stage-output document")

// topLevelKeyRe matches a column-0 line that starts a top-level YAML key of the
// envelope (all keys are lower snake case). Everything else at column 0 is
// text outside the document.
var topLevelKeyRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*:`)

// Extract returns the stage-output document in a planner reply.
//
// The whole reply must be one document: either bare, or wrapped in exactly one
// ```yaml fence whose closing fence is the last line. Leading and trailing
// blank lines are ignored. Prose before or after, a second document (a "---"
// separator or a second fence) and an unclosed fence are errors matching
// ErrNotOneDocument. Extract does not parse the YAML; stageoutput.Parse does
// that, and still rejects anything Extract lets through.
func Extract(reply string) (string, error) {
	text := strings.TrimSpace(strings.ReplaceAll(reply, "\r\n", "\n"))
	if text == "" {
		return "", fmt.Errorf("%w: the reply is empty", ErrNotOneDocument)
	}
	lines := strings.Split(text, "\n")

	if strings.HasPrefix(lines[0], "```") {
		open := strings.TrimSpace(lines[0])
		if open != "```yaml" && open != "```yml" {
			return "", fmt.Errorf("%w: the opening fence must be ```yaml, got %q", ErrNotOneDocument, open)
		}
		if len(lines) < 3 || strings.TrimSpace(lines[len(lines)-1]) != "```" {
			return "", fmt.Errorf("%w: the opening yaml fence has no closing fence as the last line of the reply", ErrNotOneDocument)
		}
		lines = lines[1 : len(lines)-1]
	}

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "```"):
			return "", fmt.Errorf("%w: a second fence at line %d (one document, one fence)", ErrNotOneDocument, i+1)
		case line == "---" || strings.HasPrefix(line, "--- ") || line == "...":
			return "", fmt.Errorf("%w: document separator %q at line %d", ErrNotOneDocument, line, i+1)
		case line == "" || line[0] == ' ' || line[0] == '\t':
			// Blank, or indented (nested key or body block content).
		case topLevelKeyRe.MatchString(line):
		default:
			return "", fmt.Errorf("%w: text outside the document at line %d: %q", ErrNotOneDocument, i+1, truncate(line, 60))
		}
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
