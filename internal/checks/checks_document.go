package checks

import (
	"regexp"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// wrappingYAMLFence matches a reply that is, in its entirety, one ```yaml (or
// ```yml) fenced block. Surrounding whitespace is tolerated. A fence that does
// not wrap the whole reply, or a fence inside the document body, does not match.
var wrappingYAMLFence = regexp.MustCompile("(?s)\\A\\s*```ya?ml[ \\t]*\\r?\\n(.*?)\\r?\\n```\\s*\\z")

// unwrapYAMLFence removes exactly one wrapping yaml fence, if the whole text is
// that fence. This is transport tolerance, not document grammar: the document
// itself is judged only by stageoutput.Parse.
func unwrapYAMLFence(s string) string {
	if m := wrappingYAMLFence.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// evalDocument runs the real stage-output parser over text. The failure detail
// is the parser's own message, so a reader sees which rule the document broke
// (for example an RFC 3339 timestamp) rather than a regex mismatch.
func evalDocument(text string) (bool, string) {
	if _, err := stageoutput.Parse([]byte(unwrapYAMLFence(text))); err != nil {
		return false, "output is not a valid stage-output document: " + err.Error()
	}
	return true, ""
}
