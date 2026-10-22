package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const (
	// MarkerPrefix starts every provenance marker line, in any version.
	MarkerPrefix = "<!-- planner-provenance:"
	// VersionV1 is the only marker version this package writes or accepts.
	VersionV1 = "v1"
)

// Hash returns the lower-case hex SHA-256 of Canonicalize(body). The marker
// line is not part of the hash input because Canonicalize removes it.
func Hash(body string) string {
	sum := sha256.Sum256([]byte(Canonicalize(body)))
	return hex.EncodeToString(sum[:])
}

// Marker renders the v1 marker line for hash and generated. The timestamp is
// RFC 3339 in UTC, for example 2006-01-02T15:04:05Z.
func Marker(hash string, generated time.Time) string {
	return MarkerPrefix + " " + VersionV1 + " sha256=" + hash +
		" generated=" + generated.UTC().Format(time.RFC3339) + " -->"
}

// Mark returns Canonicalize(body), a newline, and the v1 marker for that
// body. The marker is the last line and there is no trailing newline. If the
// canonical body is empty, Mark returns the marker alone. Marking an already
// marked body yields the same hash, and Canonicalize(Mark(b, t)) equals
// Canonicalize(b).
func Mark(body string, generated time.Time) string {
	canonical := Canonicalize(body)
	marker := Marker(Hash(canonical), generated)
	if canonical == "" {
		return marker
	}
	return canonical + "\n" + marker
}
