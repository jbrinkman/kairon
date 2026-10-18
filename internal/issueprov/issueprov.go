// Package issueprov reads and writes the planner provenance marker, a weak,
// tamper-evident content hash embedded in a GitHub issue body.
//
// The marker is a single HTML comment line:
//
//	<!-- planner-provenance: v1 sha256=<64 lowercase hex> generated=<RFC 3339 UTC> -->
//
// The SHA-256 covers the canonicalized body only (see [Canonicalize]), so the
// generated timestamp does not affect it. The hash has no secret. It detects
// edits made after generation; it does not prevent forgery. Callers should
// treat it as a routing hint and never as an authorization check.
//
// [Classify] reports exactly one of three states:
//
//   - [StateNone]: no marker line exists, so the issue was created manually.
//   - [StateValid]: exactly one well-formed v1 marker whose digest matches the body.
//   - [StateMismatch]: a marker exists but the body is not valid (edited after
//     marking, malformed marker, unknown version, bad timestamp, or several markers).
//
// This package is the shared reader and writer for the planner (which calls
// [Append]) and the watcher (which calls [Classify]). It depends only on the
// standard library.
//
// Marker detection is line-based and not markdown-aware: a line inside a fenced
// code block that starts with the marker prefix counts as a marker line.
package issueprov

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Version is the canonicalization and marker scheme written by this package.
const Version = "v1"

// State is the result of classifying an issue body.
type State string

const (
	// StateNone means the body contains no marker line.
	StateNone State = "none"
	// StateValid means the body has exactly one well-formed v1 marker whose
	// digest matches the body.
	StateValid State = "valid"
	// StateMismatch means a marker line exists but the body is not valid.
	StateMismatch State = "mismatch"
)

const markerPrefix = "<!-- planner-provenance:"

// markerRe is the strict form of a marker line, applied to the trimmed line.
var markerRe = regexp.MustCompile(`^<!-- planner-provenance: (v[0-9]+) sha256=([0-9a-f]{64}) generated=(\S+) -->$`)

// isMarkerLine reports whether line looks like a marker, well-formed or not.
func isMarkerLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), markerPrefix)
}

// parseMarker extracts the fields of a strictly well-formed marker line.
// ok is false when the line does not match the format or generated is not RFC 3339.
func parseMarker(line string) (version, sum, generated string, ok bool) {
	m := markerRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", "", "", false
	}
	if _, err := time.Parse(time.RFC3339, m[3]); err != nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

func splitLines(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.Split(body, "\n")
}

func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Canonicalize returns the v1 canonical form of body: line endings normalized
// to \n, marker lines dropped, trailing whitespace stripped from each line, and
// trailing empty lines removed. The result has no trailing newline.
func Canonicalize(body string) string {
	lines := splitLines(body)
	kept := lines[:0]
	for _, l := range lines {
		if isMarkerLine(l) {
			continue
		}
		kept = append(kept, strings.TrimRightFunc(l, unicode.IsSpace))
	}
	return strings.Join(trimTrailingEmpty(kept), "\n")
}

// Hash returns the lowercase hex SHA-256 of Canonicalize(body).
func Hash(body string) string {
	sum := sha256.Sum256([]byte(Canonicalize(body)))
	return hex.EncodeToString(sum[:])
}

// MarkerLine returns the marker line for body, without a trailing newline.
// generated is rendered in UTC.
func MarkerLine(body string, generated time.Time) string {
	return markerPrefix + " " + Version + " sha256=" + Hash(body) +
		" generated=" + generated.UTC().Format(time.RFC3339) + " -->"
}

// Append returns body with any existing marker lines removed, trailing blank
// lines trimmed, and a fresh marker line added as the last line. The result has
// no trailing newline. The body's other content, line endings and per-line
// whitespace are preserved.
func Append(body string, generated time.Time) string {
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if isMarkerLine(l) {
			continue
		}
		kept = append(kept, l)
	}
	// TrimSpace also absorbs the \r left on blank lines of CRLF input.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	marker := MarkerLine(body, generated)
	if len(kept) == 0 {
		return marker
	}
	return strings.Join(kept, "\n") + "\n\n" + marker
}

// Classify reports whether body carries no marker, a valid marker, or a marker
// that does not validate. See the package documentation for the rules.
func Classify(body string) State {
	var markers []string
	for _, l := range splitLines(body) {
		if isMarkerLine(l) {
			markers = append(markers, l)
		}
	}
	switch len(markers) {
	case 0:
		return StateNone
	case 1:
	default:
		return StateMismatch
	}
	version, sum, _, ok := parseMarker(markers[0])
	if !ok || version != Version || sum != Hash(body) {
		return StateMismatch
	}
	return StateValid
}
