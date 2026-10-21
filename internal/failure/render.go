package failure

import (
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/guard"
)

// maxValueRunes caps each rendered value so a huge agent output cannot flood
// the issue.
const maxValueRunes = 2000

// RenderComment renders the punch-out comment for fc. The first line is
// Marker(runID). Expected, Actual and the default remedy may carry agent
// output, so they are truncated to 2000 runes and placed in a code fence longer
// than any backtick run they contain. Guard and failLabel are collapsed to one
// safe line. runID must already be valid; PunchOut validates it.
func RenderComment(fc guard.FailureContext, runID, remedy, failLabel string) string {
	var b strings.Builder
	b.WriteString(Marker(runID))
	b.WriteString("\n\n")
	b.WriteString("**Kairon stopped work on this issue and needs a human.**\n\n")
	fmt.Fprintf(&b, "- Stage: `%s`\n", inlineSafe(string(fc.Stage)))
	fmt.Fprintf(&b, "- Guard: `%s`\n", inlineSafe(fc.Guard))
	fmt.Fprintf(&b, "- Attempts made: %d of %d\n\n", fc.Attempt.Current, fc.Attempt.Max)

	b.WriteString("**Expected**\n\n")
	b.WriteString(fenced(fc.Expected))
	b.WriteString("\n**Actual**\n\n")
	b.WriteString(fenced(fc.Actual))

	b.WriteString("\n**Remedy**\n\n")
	if strings.TrimSpace(remedy) == "" {
		fmt.Fprintf(&b, "Make this true, then remove the label `%s`:\n\n", inlineSafe(failLabel))
		b.WriteString(fenced(fc.Expected))
	} else {
		trimmed, _ := truncateRunes(remedy)
		b.WriteString(trimmed)
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "\nTo restart this issue, fix the cause and remove the `%s` label. "+
		"The pipeline picks the issue up again with a fresh retry budget.\n", inlineSafe(failLabel))
	return b.String()
}

// fenced returns s in a code fence, followed by a newline. The fence is longer
// than the longest backtick run in the (truncated) text, so the text cannot
// close it early.
func fenced(s string) string {
	text, truncated := truncateRunes(s)
	text = strings.TrimRight(text, "\r\n")
	fence := strings.Repeat("`", max(3, longestBacktickRun(text)+1))
	var b strings.Builder
	b.WriteString(fence)
	b.WriteString("\n")
	b.WriteString(text)
	b.WriteString("\n")
	b.WriteString(fence)
	b.WriteString("\n")
	if truncated {
		b.WriteString("_(truncated to 2000 characters)_\n")
	}
	return b.String()
}

// truncateRunes cuts s to maxValueRunes runes and reports whether it cut.
func truncateRunes(s string) (string, bool) {
	r := []rune(s)
	if len(r) <= maxValueRunes {
		return s, false
	}
	return string(r[:maxValueRunes]), true
}

// longestBacktickRun returns the length of the longest run of backticks in s.
func longestBacktickRun(s string) int {
	best, cur := 0, 0
	for _, r := range s {
		if r == '`' {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

// inlineSafe collapses s to one line and removes backticks so it can sit
// inside an inline code span without ending it or starting a new line.
func inlineSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '`':
			return '\''
		case r == '\n' || r == '\r' || r < 0x20 || r == 0x7f:
			return ' '
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}
