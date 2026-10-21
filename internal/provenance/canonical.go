package provenance

import "strings"

// normalizeLineEndings converts CRLF and lone CR line endings to LF.
func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// isMarkerCandidate reports whether line looks like a provenance marker of any
// version, well-formed or not.
func isMarkerCandidate(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), MarkerPrefix)
}

// Canonicalize returns the canonical form of an issue body. It is the single
// normalization used by both the writer (Mark) and the reader (Classify).
//
// In order, it:
//  1. converts CRLF and lone CR line endings to LF;
//  2. drops every line that starts (after trimming) with MarkerPrefix, in any
//     version, well-formed or not;
//  3. strips trailing spaces, tabs, form feeds and vertical tabs from each
//     remaining line;
//  4. drops trailing empty lines and joins the lines with LF, so the result
//     has no trailing newline.
//
// Canonicalize is idempotent.
func Canonicalize(body string) string {
	lines := strings.Split(normalizeLineEndings(body), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if isMarkerCandidate(line) {
			continue
		}
		kept = append(kept, strings.TrimRight(line, " \t\f\v"))
	}
	end := len(kept)
	for end > 0 && kept[end-1] == "" {
		end--
	}
	return strings.Join(kept[:end], "\n")
}
