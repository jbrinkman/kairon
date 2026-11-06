package failure

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/jbrinkman/kairon/internal/guard"
)

// maxValueRunes caps each rendered value so a huge agent output cannot flood
// the issue.
const maxValueRunes = 2000

// RenderComment renders the punch-out comment for fc. The first line is
// Marker(runID). Expected, Actual and the default remedy may carry agent
// output, so each is truncated to 2000 runes and rendered by renderValue as
// Markdown text that cannot act: mentions, references, URLs, links and images
// show as typed and HTML is escaped. A value that is preformatted (a line
// starts with a tab or two or more spaces) goes in one code fence longer than
// any backtick run it contains. A caller-supplied remedy is truncated and
// written as is. Guard and failLabel are collapsed to one safe line. runID must
// already be valid; PunchOut validates it.
func RenderComment(fc guard.FailureContext, runID, remedy, failLabel string) string {
	var b strings.Builder
	b.WriteString(Marker(runID))
	b.WriteString("\n\n")
	b.WriteString("**Kairon stopped work on this issue and needs a human.**\n\n")
	fmt.Fprintf(&b, "- Stage: `%s`\n", inlineSafe(string(fc.Stage)))
	fmt.Fprintf(&b, "- Guard: `%s`\n", inlineSafe(fc.Guard))
	fmt.Fprintf(&b, "- Attempts made: %d of %d\n\n", fc.Attempt.Current, fc.Attempt.Max)

	b.WriteString("**Expected**\n\n")
	b.WriteString(renderValue(fc.Expected))
	b.WriteString("\n**Actual**\n\n")
	b.WriteString(renderValue(fc.Actual))

	b.WriteString("\n**Remedy**\n\n")
	if strings.TrimSpace(remedy) == "" {
		fmt.Fprintf(&b, "Make this true, then remove the label `%s`:\n\n", inlineSafe(failLabel))
		b.WriteString(renderValue(fc.Expected))
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
// close it early. renderValue uses it for a preformatted value, which it has
// already cut to maxValueRunes, so fenced adds no truncation note on that path.
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

// truncationNote follows a truncated value, after a blank line.
const truncationNote = "\n_(truncated to 2000 characters)_\n"

// renderValue renders an untrusted value as Markdown text that ends with a
// newline. The value is cut to 2000 runes, its line breaks are normalized and
// blank lines at either end are dropped. A value with a nonblank line outside
// its own fences that starts with a tab or two or more spaces, or that is a GFM
// table delimiter row (a table would split code spans at |), is preformatted
// and goes in one code fence. A fence
// the value opens itself (0 or 1 leading space) is kept, and closed if the cut
// left it open. Everything else is text: each run of consecutive non-blank
// lines is one paragraph, rendered by renderRun. A truncated value is followed
// by a blank line and the truncation note on every path. An empty value renders
// as "".
func renderValue(s string) string {
	text, truncated := truncateRunes(s)
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text)
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		if truncated {
			// No body to separate the note from, so drop its leading blank line.
			return strings.TrimPrefix(truncationNote, "\n")
		}
		return ""
	}

	// Classify the lines. inFence[i] is true for a line of a fence the value
	// opens (opener, content and closer).
	inFence := make([]bool, len(lines))
	preformatted := false
	open := false
	var fenceCh byte
	fenceLen := 0
	for i, l := range lines {
		if open {
			inFence[i] = true
			if valueFenceCloser(l, fenceCh, fenceLen) {
				open = false
			}
			continue
		}
		if ch, n, ok := valueFenceOpener(l); ok {
			inFence[i], open, fenceCh, fenceLen = true, true, ch, n
			continue
		}
		if l != "" && (l[0] == '\t' || strings.HasPrefix(l, "  ")) {
			preformatted = true
		}
		if isTableDelimiterRow(l) {
			preformatted = true
		}
	}

	var body strings.Builder
	if preformatted {
		body.WriteString(fenced(strings.Join(lines, "\n")))
	} else {
		for i := 0; i < len(lines); {
			switch {
			case inFence[i]:
				body.WriteString(lines[i])
				body.WriteString("\n")
				i++
			case lines[i] == "":
				body.WriteString("\n")
				i++
			default:
				j := i
				for j < len(lines) && lines[j] != "" && !inFence[j] {
					j++
				}
				body.WriteString(renderRun(strings.Join(lines[i:j], "\n")))
				body.WriteString("\n")
				i = j
			}
		}
		if open {
			body.WriteString(strings.Repeat(string(fenceCh), fenceLen))
			body.WriteString("\n")
		}
	}
	if truncated {
		body.WriteString(truncationNote)
	}
	return body.String()
}

// isTableDelimiterRow reports whether l has the shape of a GFM table delimiter
// row: cells of hyphens with an optional colon at either end, separated by |,
// with at least one |. Without a | the line is a thematic break or setext
// underline, not a table. The column count is not checked against a header row,
// so this matches every delimiter row GitHub accepts and some it does not.
func isTableDelimiterRow(l string) bool {
	t := strings.TrimSpace(l)
	if !strings.Contains(t, "|") {
		return false
	}
	t = strings.TrimSuffix(strings.TrimPrefix(t, "|"), "|")
	for _, cell := range strings.Split(t, "|") {
		cell = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(cell), ":"), ":")
		if cell == "" || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

// valueFenceOpener reports whether l opens a code fence the renderer keeps: 0 or 1
// leading space (never a tab), then 3 or more backticks or tildes, and for a
// backtick fence an info string without a backtick. A line with 0 or 1 leading
// space is always a top-level block, so a fence it opens ends at its own closer.
// A fence-looking line indented by 2 or more spaces is not an opener: under a
// list item it would end early.
func valueFenceOpener(l string) (ch byte, n int, ok bool) {
	rest := strings.TrimPrefix(l, " ")
	if rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return 0, 0, false
	}
	ch = rest[0]
	for n < len(rest) && rest[n] == ch {
		n++
	}
	if n < 3 || (ch == '`' && strings.Contains(rest[n:], "`")) {
		return 0, 0, false
	}
	return ch, n, true
}

// valueFenceCloser reports whether l closes a fence opened with n characters ch:
// 0 to 3 leading spaces, then a run of ch at least n long, then only spaces or
// tabs. A closer may be indented further than an opener because that is how a
// Markdown renderer closes a fence; a stricter rule would leave later lines
// written as fence content after the renderer had already closed it.
func valueFenceCloser(l string, ch byte, n int) bool {
	sp := 0
	for sp < len(l) && l[sp] == ' ' {
		sp++
	}
	if sp > 3 {
		return false
	}
	rest := l[sp:]
	k := 0
	for k < len(rest) && rest[k] == ch {
		k++
	}
	return k >= n && strings.Trim(rest[k:], " \t") == ""
}

// renderRun renders one paragraph (consecutive non-blank text lines joined with
// "\n", outside any fence) so that Markdown shows it as typed and nothing in it
// acts. It scans left to right and applies the first rule that matches:
//
//   - a backtick run that pairs on the same line is a user code span, kept
//     verbatim; an unpaired backtick is written \`
//   - an image, link or reference link, matched with bracket-balanced text and
//     an inline or reference tail, and a bare URL, GH-N, owner/repo#N, #N or
//     @mention are wrapped in inline code
//   - < is written &lt; and \ is written \\
//   - a [ that no link or image matched is written \[, so no unescaped [ is left
//     outside code and no link, reference link or image can form
//
// A token that crosses a line break is wrapped in one code span per line.
func renderRun(run string) string {
	r := []rune(run)
	out := make([]rune, 0, len(r)+len(r)/4)
	put := func(s string) { out = append(out, []rune(s)...) }
	// wrapTok writes r[from:to] as inline code. Touching backtick runs would
	// merge, so a space separates the wrapper from a backtick on either side.
	// codeEnd is the index in r where the last code span written ended. GitHub
	// reads mentions and references per text node, so a token that starts there
	// is at a boundary whatever rune precedes it in r.
	codeEnd := -1
	wrapTok := func(from, to int) {
		if len(out) > 0 && out[len(out)-1] == '`' {
			out = append(out, ' ')
		}
		put(wrap(string(r[from:to])))
		if to < len(r) && r[to] == '`' {
			out = append(out, ' ')
		}
		codeEnd = to
	}

	for i := 0; i < len(r); {
		c := r[i]
		if c == '`' {
			n := backtickRunLen(r, i)
			if end, ok := codeSpanEnd(r, i); ok {
				out = append(out, r[i:end]...)
				i = end
				codeEnd = end
			} else {
				for k := 0; k < n; k++ {
					put("\\`")
				}
				i += n
			}
			continue
		}
		if c == '!' && i+1 < len(r) && r[i+1] == '[' {
			if end, ok := linkEnd(r, i+1); ok {
				wrapTok(i, end)
				i = end
				continue
			}
		}
		if c == '[' {
			if end, ok := linkEnd(r, i); ok {
				wrapTok(i, end)
				i = end
				continue
			}
			put("\\[")
			i++
			continue
		}
		end := tokenEnd(r, i)
		if i == codeEnd {
			// Match from a fresh start so no left-boundary check sees r[i-1].
			end = i + tokenEnd(r[i:], 0)
		}
		if end > i {
			wrapTok(i, end)
			i = end
			continue
		}
		switch c {
		case '<':
			put("&lt;")
		case '\\':
			put("\\\\")
		default:
			out = append(out, c)
		}
		i++
	}
	return string(out)
}

// tokenEnd returns the end of the URL, GH-N, owner/repo#N, #N or mention that
// starts at r[i], or i if none does.
func tokenEnd(r []rune, i int) int {
	for _, f := range []func([]rune, int) int{urlEnd, ghEnd, ownerRepoEnd, hashEnd, mentionEnd} {
		if end := f(r, i); end > i {
			return end
		}
	}
	return i
}

func isWordRune(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' }

func isASCIIAlnum(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// prevIs reports whether i > 0 and r[i-1] satisfies bad.
func prevIs(r []rune, i int, bad func(rune) bool) bool { return i > 0 && bad(r[i-1]) }

func nextIsWord(r []rune, i int) bool { return i < len(r) && isWordRune(r[i]) }

func digitsEnd(r []rune, i int) int {
	for i < len(r) && r[i] >= '0' && r[i] <= '9' {
		i++
	}
	return i
}

// hasPrefixFold reports whether r[i:] starts with the lower-case ASCII string
// p, ignoring ASCII case.
func hasPrefixFold(r []rune, i int, p string) bool {
	if i+len(p) > len(r) {
		return false
	}
	for k := 0; k < len(p); k++ {
		c := r[i+k]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != rune(p[k]) {
			return false
		}
	}
	return true
}

// urlEnd matches a bare URL: http://, https:// or www. (case-insensitive) after
// a rune that is not a letter or digit, up to white space or <, without
// trailing punctuation.
func urlEnd(r []rune, i int) int {
	if prevIs(r, i, func(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }) {
		return i
	}
	start := i
	switch {
	case hasPrefixFold(r, i, "https://"):
		start = i + len("https://")
	case hasPrefixFold(r, i, "http://"):
		start = i + len("http://")
	case hasPrefixFold(r, i, "www."):
		start = i + len("www.")
		if start >= len(r) || !(unicode.IsLetter(r[start]) || unicode.IsDigit(r[start])) {
			return i
		}
	default:
		return i
	}
	end := start
	for end < len(r) && !unicode.IsSpace(r[end]) && r[end] != '<' {
		end++
	}
	for end > start && strings.ContainsRune(".,;:!?\"')]}", r[end-1]) {
		end--
	}
	return end
}

// ghEnd matches GH-N (case-insensitive).
func ghEnd(r []rune, i int) int {
	if !hasPrefixFold(r, i, "gh-") ||
		prevIs(r, i, func(c rune) bool { return isWordRune(c) || c == '-' }) {
		return i
	}
	end := digitsEnd(r, i+3)
	if end == i+3 || nextIsWord(r, end) {
		return i
	}
	return end
}

func isRepoRune(c rune) bool { return isASCIIAlnum(c) || c == '_' || c == '.' || c == '-' }

// ownerRepoEnd matches owner/repo#N.
func ownerRepoEnd(r []rune, i int) int {
	if i >= len(r) || !isRepoRune(r[i]) ||
		prevIs(r, i, func(c rune) bool { return isWordRune(c) || c == '.' || c == '-' || c == '/' }) {
		return i
	}
	j := i
	for j < len(r) && isRepoRune(r[j]) {
		j++
	}
	if j >= len(r) || r[j] != '/' {
		return i
	}
	k := j + 1
	for k < len(r) && isRepoRune(r[k]) {
		k++
	}
	if k == j+1 || k >= len(r) || r[k] != '#' {
		return i
	}
	end := digitsEnd(r, k+1)
	if end == k+1 || nextIsWord(r, end) {
		return i
	}
	return end
}

// hashEnd matches #N.
func hashEnd(r []rune, i int) int {
	if r[i] != '#' ||
		prevIs(r, i, func(c rune) bool { return isWordRune(c) || c == '&' || c == '/' || c == '#' }) {
		return i
	}
	end := digitsEnd(r, i+1)
	if end == i+1 || nextIsWord(r, end) {
		return i
	}
	return end
}

// mentionEnd matches @name and @org/team.
func mentionEnd(r []rune, i int) int {
	if r[i] != '@' || prevIs(r, i, isWordRune) || i+1 >= len(r) || !isASCIIAlnum(r[i+1]) {
		return i
	}
	isName := func(c rune) bool { return isASCIIAlnum(c) || c == '_' || c == '-' }
	end := i + 2
	for end < len(r) && isName(r[end]) {
		end++
	}
	if end+1 < len(r) && r[end] == '/' && isName(r[end+1]) {
		end += 2
		for end < len(r) && isName(r[end]) {
			end++
		}
	}
	return end
}

func backtickRunLen(r []rune, i int) int {
	n := 0
	for i+n < len(r) && r[i+n] == '`' {
		n++
	}
	return n
}

// codeSpanEnd reports where the code span that starts with the backtick run at
// r[i] ends: just after a later run of exactly the same length on the same
// line. A user code span never crosses a line break.
func codeSpanEnd(r []rune, i int) (int, bool) {
	n := backtickRunLen(r, i)
	for j := i + n; j < len(r) && r[j] != '\n'; {
		if r[j] == '`' {
			m := backtickRunLen(r, j)
			if m == n {
				return j + m, true
			}
			j += m
			continue
		}
		j++
	}
	return 0, false
}

// closeBracket returns the index of the ] that closes the link text opened by
// the [ at r[i]. Brackets nest, line breaks are ordinary runes, and a code span
// is skipped as a unit because it binds tighter than link text. A backslash is
// an ordinary rune: renderRun writes every backslash outside code as \\, so no
// bracket in its output is escaped.
func closeBracket(r []rune, i int) (int, bool) {
	depth := 1
	for j := i + 1; j < len(r); {
		switch r[j] {
		case '`':
			if end, ok := codeSpanEnd(r, j); ok {
				j = end
			} else {
				j += backtickRunLen(r, j)
			}
			continue
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return j, true
			}
		}
		j++
	}
	return 0, false
}

// linkEnd returns the end of the link, reference link or image text and tail
// that starts at the [ at r[i].
func linkEnd(r []rune, i int) (int, bool) {
	cb, ok := closeBracket(r, i)
	if !ok || cb+1 >= len(r) {
		return 0, false
	}
	switch r[cb+1] {
	case '(':
		return inlineTailEnd(r, cb+1)
	case '[':
		for j := cb + 2; j < len(r); j++ {
			if r[j] == ']' {
				return j + 1, true
			}
		}
	}
	return 0, false
}

func isTailSpace(c rune) bool { return c == ' ' || c == '\t' || c == '\n' }

// inlineTailEnd returns the end of the inline tail whose ( is at r[k]. A
// backtick is an ordinary rune here, because CommonMark reads the destination
// and title as plain text: a backtick must not hide the ) that closes the link.
func inlineTailEnd(r []rune, k int) (int, bool) {
	// Depth scan: balanced parentheses, an angle-bracket destination and a
	// quoted title, whose parentheses do not count.
	j := k + 1
	p := j
	for p < len(r) && isTailSpace(r[p]) {
		p++
	}
	if p < len(r) && r[p] == '<' {
		if q := indexRune(r, '>', p+1); q >= 0 {
			j = q + 1
		}
	}
	depth := 1
	for ; j < len(r); j++ {
		switch c := r[j]; c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j + 1, true
			}
		case '"', '\'':
			if isTailSpace(r[j-1]) {
				if q := indexRune(r, c, j+1); q >= 0 {
					j = q
				}
			}
		}
	}
	// The depth scan did not close, for example because CommonMark honours a
	// backslash escape that the scan does not. Take the first ) so the match
	// stays a superset of what a renderer reads as the link.
	if q := indexRune(r, ')', k+1); q >= 0 {
		return q + 1, true
	}
	return 0, false
}

func indexRune(r []rune, c rune, from int) int {
	for j := from; j < len(r); j++ {
		if r[j] == c {
			return j
		}
	}
	return -1
}

// wrap writes a token as inline code. A token that holds a backtick gets a
// longer backtick run, and one that starts or ends with a backtick gets a space
// inside each side. A code span would turn a line break into a space and leave a
// bracket outside code, so a token that crosses a line break is wrapped one
// line at a time with the line breaks kept; a line's leading spaces stay
// outside its span.
func wrap(tok string) string {
	var b strings.Builder
	for n, piece := range strings.Split(tok, "\n") {
		if n > 0 {
			b.WriteString("\n")
		}
		text := strings.TrimLeft(piece, " ")
		b.WriteString(piece[:len(piece)-len(text)])
		fence := strings.Repeat("`", longestBacktickRun(text)+1)
		if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
			text = " " + text + " "
		}
		b.WriteString(fence)
		b.WriteString(text)
		b.WriteString(fence)
	}
	return b.String()
}
