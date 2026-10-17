package checks

import "regexp"

// ansiRegex matches all CSI (Control Sequence Introducer) escape sequences.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes CSI escape sequences from s.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}
