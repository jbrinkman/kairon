package planner

import (
	"errors"
	"regexp"
	"strings"
)

var (
	// ErrNoDocument means the reply holds no stage-output document: it is
	// empty, or it opens a fence that is never closed or holds nothing.
	ErrNoDocument = errors.New("planner: reply contains no stage-output document")

	// ErrExtraneousText means the reply holds something other than exactly one
	// stage-output document: prose before or after it, a second fence, or a
	// second document.
	ErrExtraneousText = errors.New("planner: reply contains text other than one stage-output document")
)

const (
	fenceMarker = "```"
	// documentStart is the key every stage-output document opens with. It is
	// what separates a document from prose that merely looks like YAML.
	documentStart = "schema_version:"
)

// openFence matches the opening line of a yaml or yml fence.
var openFence = regexp.MustCompile("^```(?:yaml|yml)[ \t]*$")

// Extract returns the stage-output document text from an agent reply: the
// whole reply, or the content of one yaml or yml fence that is the whole
// reply. Blank lines around the document or the fence are ignored. Anything
// else before or after it (prose, a second fence, a second document) returns
// ErrExtraneousText, and an empty reply or a fence that is never closed
// returns ErrNoDocument. Extract does not parse the document; that is
// stageoutput.Parse's job.
func Extract(reply string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(reply, "\r\n", "\n"), "\n")
	lines = trimBlankLines(lines)
	if len(lines) == 0 {
		return "", ErrNoDocument
	}

	if strings.HasPrefix(lines[0], fenceMarker) {
		if !openFence.MatchString(lines[0]) {
			return "", ErrExtraneousText
		}
		closing := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimRight(lines[i], " \t") == fenceMarker {
				closing = i
				break
			}
		}
		if closing < 0 {
			return "", ErrNoDocument
		}
		if len(trimBlankLines(lines[closing+1:])) > 0 {
			return "", ErrExtraneousText
		}
		lines = trimBlankLines(lines[1:closing])
		if len(lines) == 0 {
			return "", ErrNoDocument
		}
	}

	if err := checkSingleDocument(lines); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// checkSingleDocument verifies that lines hold one YAML mapping that opens with
// schema_version. Only column-0 lines are inspected: block scalar content
// (including a fenced code block inside the body) is indented.
func checkSingleDocument(lines []string) error {
	if !strings.HasPrefix(lines[0], documentStart) {
		return ErrExtraneousText
	}
	for i, line := range lines {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		switch {
		case strings.TrimRight(line, " \t") == "---", strings.TrimRight(line, " \t") == "...":
			// A document marker starts or ends a document.
			return ErrExtraneousText
		case i > 0 && strings.HasPrefix(line, documentStart):
			// A second document pasted straight after the first.
			return ErrExtraneousText
		case line[0] == '-' && len(line) > 1 && (line[1] == ' ' || line[1] == '\t'):
			// A sequence item at the top level of a key's value.
		case isEnvelopeKey(line):
		default:
			return ErrExtraneousText
		}
	}
	return nil
}

// envelopeKeys are the top-level keys of a stage-output document. A column-0
// "word: text" line whose key is not one of these is prose that happens to look
// like YAML ("Note: that is all"), not part of the document. stageoutput.Parse
// rejects unknown keys anyway, so this set only decides which error is reported.
var envelopeKeys = map[string]bool{
	"schema_version": true,
	"stage":          true,
	"result":         true,
	"source":         true,
	"generated":      true,
	"body":           true,
	"unresolved":     true,
	"inferences":     true,
}

// isEnvelopeKey reports whether a column-0 line is "key:" for an envelope key.
func isEnvelopeKey(line string) bool {
	key, _, ok := strings.Cut(line, ":")
	return ok && envelopeKeys[key] && (len(line) == len(key)+1 || line[len(key)+1] == ' ' || line[len(key)+1] == '\t')
}

// trimBlankLines drops leading and trailing lines that are empty or only
// whitespace.
func trimBlankLines(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}
