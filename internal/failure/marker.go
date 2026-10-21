package failure

import "strings"

const (
	markerPrefix = "<!-- kairon:punch-out run="
	markerSuffix = " -->"
)

// Marker returns the marker line for runID. It is the first line of every
// punch-out comment and scopes that comment to one terminal run.
func Marker(runID string) string { return markerPrefix + runID + markerSuffix }

// ParseMarker extracts the run id from body. The marker must be the entire
// first line (a trailing CR is tolerated); a marker anywhere else, or one with
// an invalid run id, is rejected. That keeps pasted or quoted text from
// reading as a marker.
func ParseMarker(body string) (runID string, ok bool) {
	line, _, _ := strings.Cut(body, "\n")
	line = strings.TrimSuffix(line, "\r")
	if !strings.HasPrefix(line, markerPrefix) || !strings.HasSuffix(line, markerSuffix) {
		return "", false
	}
	if len(line) < len(markerPrefix)+len(markerSuffix) {
		return "", false
	}
	id := line[len(markerPrefix) : len(line)-len(markerSuffix)]
	if !ValidRunID(id) {
		return "", false
	}
	return id, true
}
