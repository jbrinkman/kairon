package provenance

import (
	"regexp"
	"strings"
	"time"
)

// State is the provenance classification of an issue body.
type State int

const (
	// StateInvalid is the zero value and is never returned by Classify.
	StateInvalid State = iota
	// StateValid means the body carries one well-formed v1 marker, as its last
	// non-empty line, whose hash matches the body.
	StateValid
	// StateNone means the body has no marker line.
	StateNone
	// StateMismatch means a marker line is present but the body is not valid:
	// the marker is malformed, has an unknown version, is duplicated, is not
	// last, or carries the wrong hash.
	StateMismatch
)

// String returns "valid", "none" or "mismatch". StateInvalid and out-of-range
// values return "invalid".
func (s State) String() string {
	switch s {
	case StateValid:
		return "valid"
	case StateNone:
		return "none"
	case StateMismatch:
		return "mismatch"
	default:
		return "invalid"
	}
}

var markerV1Re = regexp.MustCompile(`^<!-- planner-provenance: v1 sha256=([0-9a-f]{64}) generated=(\S+) -->$`)

// Classify reports whether body is valid, none or mismatch. It uses Hash, and
// through it Canonicalize, so the reader and the writer share one
// normalization path. A marker that is present but not exactly right is a
// mismatch, never valid and never none.
func Classify(body string) State {
	lines := strings.Split(normalizeLineEndings(body), "\n")

	var candidates []int
	last := -1 // index of the last non-empty line
	for i, line := range lines {
		if isMarkerCandidate(line) {
			candidates = append(candidates, i)
		}
		if strings.TrimSpace(line) != "" {
			last = i
		}
	}
	if len(candidates) == 0 {
		return StateNone
	}
	if len(candidates) != 1 || candidates[0] != last {
		return StateMismatch
	}

	m := markerV1Re.FindStringSubmatch(strings.TrimSpace(lines[last]))
	if m == nil {
		return StateMismatch
	}
	if _, err := time.Parse(time.RFC3339, m[2]); err != nil {
		return StateMismatch
	}
	if m[1] != Hash(body) {
		return StateMismatch
	}
	return StateValid
}
