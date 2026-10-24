package boundary

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// MaxBodyBytes is the largest body DefaultScreen accepts. GitHub's own limit is
// 65536 characters, so this only trips on abnormal input.
const MaxBodyBytes = 262144

// DefaultScreen is the deterministic backstop applied to untrusted issue
// content on every path, including a body whose provenance hash is valid.
//
// It rejects invalid UTF-8, control characters other than newline and tab (this
// includes NUL, carriage return, DEL and the C1 range), and content longer than
// MaxBodyBytes. It does not detect prompt injection: the defense against that is
// structural, because the body is only ever carried as a data field of the
// input document. A richer security guard can replace this function through
// Boundary.Screen without changing Admit.
//
// The returned error names the problem and never echoes the content.
func DefaultScreen(body string) error {
	if len(body) > MaxBodyBytes {
		return fmt.Errorf("content is %d bytes, over the %d byte limit", len(body), MaxBodyBytes)
	}
	if !utf8.ValidString(body) {
		return errors.New("content is not valid UTF-8")
	}
	for i, r := range body {
		if r == '\n' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return fmt.Errorf("content contains a control character (U+%04X) at byte %d", r, i)
		}
	}
	return nil
}
