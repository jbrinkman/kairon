package failure

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

func renderFC() guard.FailureContext {
	return guard.FailureContext{
		Stage:    stageoutput.StageKrewLead,
		Guard:    "implementation-run",
		Attempt:  guard.Attempt{Current: 4, Max: 4},
		Expected: "krew-lead exits 0 and a pull request exists",
		Actual:   "exit code 1",
	}
}

func TestRenderComment_ContainsStageAttemptsExpectedActualRemedy(t *testing.T) {
	fc := renderFC()
	fc.Attempt = guard.Attempt{Current: 2, Max: 3}
	got := RenderComment(fc, "run-1", "Read .kairon/logs/issue-5.log, fix the cause.", "kairon-failed")

	if first, _, _ := strings.Cut(got, "\n"); first != Marker("run-1") {
		t.Errorf("first line = %q, want the marker", first)
	}
	if id, ok := ParseMarker(got); !ok || id != "run-1" {
		t.Errorf("ParseMarker(rendered) = %q, %v", id, ok)
	}
	for _, want := range []string{
		"krew-lead",
		"implementation-run",
		"2 of 3",
		fc.Expected,
		fc.Actual,
		"Read .kairon/logs/issue-5.log, fix the cause.",
		"kairon-failed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered comment missing %q:\n%s", want, got)
		}
	}
}

func TestRenderComment_DefaultRemedy(t *testing.T) {
	fc := renderFC()
	got := RenderComment(fc, "r", "", "kairon-failed")
	if !strings.Contains(got, "Make this true, then remove the label") {
		t.Errorf("missing default remedy:\n%s", got)
	}
	if strings.Count(got, fc.Expected) < 2 {
		t.Errorf("default remedy should repeat the expected value:\n%s", got)
	}
}

// testBacktickRun returns the length of the longest run of backticks in s.
func testBacktickRun(s string) int {
	best, cur := 0, 0
	for _, r := range s {
		if r == '`' {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

// fenceOpeners returns the backtick-only fence lines of a rendered comment.
func fenceLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Trim(l, "`") == "" && l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestRenderComment_NeutralizesUntrustedValues(t *testing.T) {
	t.Run("fence breakout", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = "before\n```\n## injected heading\n``````\nafter"
		got := RenderComment(fc, "r", "x", "kairon-failed")
		// The value's own fence lines are kept verbatim, and the heading sits
		// between its opener and its closer, so it cannot act as a heading.
		if sec := section(got, "Actual"); sec != fc.Actual {
			t.Errorf("Actual section = %q, want the value verbatim %q", sec, fc.Actual)
		}
		if strings.Contains(outsideFences(got), "## injected heading") {
			t.Errorf("injected heading is outside a fence:\n%s", got)
		}
		if why := fenceProblem(got); why != "" {
			t.Errorf("fences not balanced: %s\n%s", why, got)
		}
	})

	t.Run("value fence is kept and a backtick run is escaped, not fenced", func(t *testing.T) {
		fc := renderFC()
		fc.Expected = "a `` b ````` c"
		fc.Actual = "```` d"
		got := RenderComment(fc, "r", "x", "kairon-failed")
		// Expected holds only unmatched backtick runs, so every backtick is
		// escaped and no live backtick run is left.
		wantExpected := strings.ReplaceAll(fc.Expected, "`", "\\`")
		if sec := section(got, "Expected"); sec != wantExpected {
			t.Errorf("Expected section = %q, want %q", sec, wantExpected)
		}
		// Actual's 4-backtick opener is kept and closed with a 4-backtick line.
		if sec, want := section(got, "Actual"), "```` d\n````"; sec != want {
			t.Errorf("Actual section = %q, want %q", sec, want)
		}
		if why := fenceProblem(got); why != "" {
			t.Errorf("fences not balanced: %s\n%s", why, got)
		}
	})

	t.Run("injected marker is not the first line", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = Marker("attacker") + "\nmore"
		fc.Expected = "\n" + Marker("attacker2")
		got := RenderComment(fc, "real", "x", "kairon-failed")
		id, ok := ParseMarker(got)
		if !ok || id != "real" {
			t.Errorf("ParseMarker = %q, %v; want real", id, ok)
		}
	})

	t.Run("truncated to 2000 runes", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = strings.Repeat("é", 5000)
		fc.Expected = strings.Repeat("x", 2000)
		got := RenderComment(fc, "r", "x", "kairon-failed")
		if n := strings.Count(got, "é"); n != 2000 {
			t.Errorf("é count = %d, want 2000 (truncated)", n)
		}
		if n := strings.Count(got, "x"); n < 2000 {
			t.Errorf("exact-limit value was truncated: x count = %d", n)
		}
		if !strings.Contains(got, "truncated") {
			t.Errorf("truncated value should say so:\n%.400s", got)
		}
	})

	t.Run("not truncated at the limit", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = strings.Repeat("é", 2000)
		got := RenderComment(fc, "r", "x", "kairon-failed")
		if n := strings.Count(got, "é"); n != 2000 {
			t.Errorf("é count = %d, want 2000", n)
		}
		if strings.Contains(got, "truncated") {
			t.Error("value at the limit must not be marked truncated")
		}
	})

	t.Run("truncation happens before fence sizing", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = strings.Repeat("a", 2000) + strings.Repeat("`", 50)
		got := RenderComment(fc, "r", "x", "kairon-failed")
		for _, f := range fenceLines(got) {
			if len(f) > 10 {
				t.Errorf("fence of %d backticks sized from text that was cut", len(f))
			}
		}
	})

	t.Run("single-line fields cannot break structure", func(t *testing.T) {
		fc := renderFC()
		fc.Guard = "evil`\n## heading"
		got := RenderComment(fc, "r", "x", "kairon-failed`\n## label")
		if strings.Contains(got, "\n## heading") || strings.Contains(got, "\n## label") {
			t.Errorf("guard or label injected a line:\n%s", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Markdown rendering of Expected, Actual and the default remedy (issue #485).
//
// Every test below drives RenderComment only. None calls renderValue or any
// other function the implementation adds, so the package compiles against the
// current render.go and the red run fails on assertions.
// ---------------------------------------------------------------------------

const bt = "`"

// renderWith renders a comment with the given Actual, Expected and remedy.
func renderWith(actual, expected, remedy string) string {
	fc := guard.FailureContext{
		Stage:    stageoutput.StageKrewLead,
		Guard:    "implementation-run",
		Attempt:  guard.Attempt{Current: 1, Max: 3},
		Expected: expected,
		Actual:   actual,
	}
	return RenderComment(fc, "r", remedy, "kairon-failed")
}

// renderActual renders in as the Actual value (Expected "ok", default remedy)
// and returns the whole comment and the Actual section.
func renderActual(in string) (got, sec string) {
	got = renderWith(in, "ok", "")
	return got, section(got, "Actual")
}

// renderExpected renders in as the Expected value (Actual "ok", default
// remedy) and returns the whole comment and the Expected section.
func renderExpected(in string) (got, sec string) {
	got = renderWith("ok", in, "")
	return got, section(got, "Expected")
}

// section returns the text between "**<name>**\n\n" and the next heading, with
// the final newline removed. Expected ends at "\n**Actual**\n\n" and Actual at
// "\n**Remedy**\n\n".
func section(got, name string) string {
	start := "**" + name + "**\n\n"
	end := map[string]string{"Expected": "\n**Actual**\n\n", "Actual": "\n**Remedy**\n\n"}[name]
	i := strings.Index(got, start)
	if i < 0 || end == "" {
		return "\x00missing section " + name
	}
	rest := got[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return "\x00missing end of section " + name
	}
	return strings.TrimSuffix(rest[:j], "\n")
}

// remedySection returns the text between "**Remedy**\n\n" and the restart line.
func remedySection(got string) string {
	start := "**Remedy**\n\n"
	i := strings.Index(got, start)
	if i < 0 {
		return "\x00missing remedy section"
	}
	rest := got[i+len(start):]
	j := strings.Index(rest, "\nTo restart this issue")
	if j < 0 {
		return "\x00missing restart line"
	}
	return rest[:j]
}

// ---- fence scanner (an independent reading of the spec's step 3) ----

func leadingSpaces(l string) int {
	n := 0
	for n < len(l) && l[n] == ' ' {
		n++
	}
	return n
}

// fenceOpener reports whether l opens a fence: 0 or 1 leading space (never a
// tab), then 3 or more backticks or tildes, and for a backtick fence an info
// string without a backtick.
func fenceOpener(l string) (ch byte, n int, ok bool) {
	sp := leadingSpaces(l)
	if sp > 1 {
		return 0, 0, false
	}
	rest := l[sp:]
	if rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return 0, 0, false
	}
	ch = rest[0]
	for n < len(rest) && rest[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	if ch == '`' && strings.Contains(rest[n:], "`") {
		return 0, 0, false
	}
	return ch, n, true
}

// fenceCloser reports whether l closes a fence opened with n characters ch:
// 0 to 3 leading spaces, a run of ch at least n long, then only spaces or tabs.
func fenceCloser(l string, ch byte, n int) bool {
	sp := leadingSpaces(l)
	if sp > 3 {
		return false
	}
	rest := l[sp:]
	k := 0
	for k < len(rest) && rest[k] == ch {
		k++
	}
	if k < n {
		return false
	}
	return strings.Trim(rest[k:], " \t") == ""
}

type scannedLine struct {
	text string
	in   bool // part of a fence: its opener, its content or its closer
}

// scanFences classifies every line of s and reports whether a fence is still
// open at the end.
func scanFences(s string) (lines []scannedLine, open bool) {
	var ch byte
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if open {
			lines = append(lines, scannedLine{l, true})
			if fenceCloser(l, ch, n) {
				open = false
			}
			continue
		}
		if c, k, ok := fenceOpener(l); ok {
			ch, n, open = c, k, true
			lines = append(lines, scannedLine{l, true})
			continue
		}
		lines = append(lines, scannedLine{l, false})
	}
	return lines, open
}

// outsideFences returns s with every fenced region removed.
func outsideFences(s string) string {
	lines, _ := scanFences(s)
	var out []string
	for _, l := range lines {
		if !l.in {
			out = append(out, l.text)
		}
	}
	return strings.Join(out, "\n")
}

// fenceProblem returns why a whole comment is not fence-balanced, or "".
// (1) the scan ends outside any fence; (2) no line outside a fence starts with
// a tab or two or more spaces; (3) the Expected, Actual and Remedy headings and
// the restart line are each outside every fence.
func fenceProblem(got string) string {
	lines, open := scanFences(got)
	if open {
		return "a fence is still open at the end of the comment"
	}
	seen := map[string]bool{}
	for _, l := range lines {
		if l.in {
			continue
		}
		if strings.HasPrefix(l.text, "\t") || strings.HasPrefix(l.text, "  ") {
			return fmt.Sprintf("line outside a fence starts with a tab or 2+ spaces: %q", l.text)
		}
		switch {
		case l.text == "**Expected**", l.text == "**Actual**", l.text == "**Remedy**":
			seen[l.text] = true
		case strings.HasPrefix(l.text, "To restart this issue"):
			seen["restart"] = true
		}
	}
	for _, k := range []string{"**Expected**", "**Actual**", "**Remedy**", "restart"} {
		if !seen[k] {
			return fmt.Sprintf("%s is inside a fence or missing", k)
		}
	}
	return ""
}

func fencesBalanced(got string) bool { return fenceProblem(got) == "" }

// hasFenceLine reports whether any line of s starts a fence: 0 to 3 leading
// spaces, then 3 or more backticks or tildes (for backticks, an info string
// without a backtick, so a 3-backtick token wrapper at the start of a line is not one).
func hasFenceLine(s string) bool {
	for _, l := range strings.Split(s, "\n") {
		sp := leadingSpaces(l)
		if sp > 3 {
			continue
		}
		rest := l[sp:]
		if rest == "" || (rest[0] != '`' && rest[0] != '~') {
			continue
		}
		k := 0
		for k < len(rest) && rest[k] == rest[0] {
			k++
		}
		if k >= 3 && !(rest[0] == '`' && strings.Contains(rest[k:], "`")) {
			return true
		}
	}
	return false
}

// valueProblem returns the first structural problem in a rendered value, or "".
// It is the check every row of the fence, token and sweep tests applies.
func valueProblem(got, sec string) string {
	if why := fenceProblem(got); why != "" {
		return "fences not balanced: " + why
	}
	out := outsideFences(sec)
	if hasLiveLink(out) {
		return fmt.Sprintf("live link outside fences in section %q", sec)
	}
	// Deviates from spec "Test design" (TestRenderMarkdown_FenceStructure and
	// TestRenderMarkdown_HTMLEscapedOutsideCode: no "<" in outsideFences(section)):
	// the requirement (issue FR4, spec algorithm) is that "<" is written &lt;
	// only outside code, and the spec's own exact want for
	// "[a](<u`v>) `x`" keeps "<" raw inside a renderer-written code span, so a
	// check on outsideFences alone flags correct output. This ignores "<" that
	// is inside an inline code span (CommonMark pairing, as in hasLiveLink: a
	// backslash and the rune after it are literal, a backtick run pairs with the
	// next run of equal length in the same paragraph, an unpaired run is kept as
	// text). A "<" in ordinary text still fails.
	var plain strings.Builder
	for _, para := range paragraphsOf(out) {
		p := []rune(para)
		for i := 0; i < len(p); {
			if p[i] == '\\' {
				for k := i; k < i+2 && k < len(p); k++ {
					plain.WriteRune(p[k])
				}
				i += 2
				continue
			}
			if p[i] != '`' {
				plain.WriteRune(p[i])
				i++
				continue
			}
			j := i
			for j < len(p) && p[j] == '`' {
				j++
			}
			end := -1
			for k := j; k < len(p); {
				if p[k] != '`' {
					k++
					continue
				}
				m := k
				for m < len(p) && p[m] == '`' {
					m++
				}
				if m-k == j-i {
					end = m
					break
				}
				k = m
			}
			if end < 0 {
				for ; i < j; i++ {
					plain.WriteRune('`')
				}
				continue
			}
			i = end
		}
		plain.WriteRune('\n')
	}
	if strings.Contains(plain.String(), "<") {
		return fmt.Sprintf("raw < outside code in section %q", sec)
	}
	return ""
}

// ---- live-link oracle ----

type liveRune struct {
	idx int
	r   rune
	lit bool // escaped by a backslash, a backslash itself, or a literal backtick
}

// paragraphsOf splits s at blank lines (empty or white-space-only).
func paragraphsOf(s string) []string {
	var out, cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) == "" {
			flush()
			continue
		}
		cur = append(cur, l)
	}
	flush()
	return out
}

// hasLiveLink is an independent oracle for "would a CommonMark renderer turn
// something in s into a link". It is looser than the renderer on purpose.
//
// Step 1 finds each unescaped "[" and its bracket-balanced "]" after dropping
// code spans (a backtick run of length n with a later run of exactly n in the
// paragraph is dropped with everything between). A backslash and the rune after
// it are literal. Step 2 reads the tail from the raw paragraph text, without
// dropping code spans and without honouring escapes: "(" after the "]" is live
// when a ")" occurs later in the paragraph, "[" is live when a "]" occurs later.
func hasLiveLink(s string) bool {
	for _, para := range paragraphsOf(s) {
		if paragraphHasLiveLink([]rune(para)) {
			return true
		}
	}
	return false
}

func paragraphHasLiveLink(p []rune) bool {
	var kept []liveRune
	for i := 0; i < len(p); {
		switch {
		case p[i] == '\\':
			kept = append(kept, liveRune{i, p[i], true})
			if i+1 < len(p) {
				kept = append(kept, liveRune{i + 1, p[i+1], true})
			}
			i += 2
		case p[i] == '`':
			j := i
			for j < len(p) && p[j] == '`' {
				j++
			}
			n := j - i
			end := -1
			for k := j; k < len(p); {
				if p[k] != '`' {
					k++
					continue
				}
				m := k
				for m < len(p) && p[m] == '`' {
					m++
				}
				if m-k == n {
					end = m
					break
				}
				k = m
			}
			if end >= 0 {
				i = end
			} else {
				for ; i < j; i++ {
					kept = append(kept, liveRune{i, '`', true})
				}
			}
		default:
			kept = append(kept, liveRune{i, p[i], false})
			i++
		}
	}
	for a := range kept {
		if kept[a].lit || kept[a].r != '[' {
			continue
		}
		depth := 1
		for b := a + 1; b < len(kept); b++ {
			if kept[b].lit {
				continue
			}
			switch kept[b].r {
			case '[':
				depth++
			case ']':
				depth--
			}
			if depth > 0 {
				continue
			}
			next := kept[b].idx + 1
			if next < len(p) {
				switch p[next] {
				case '(':
					if slices.Contains(p[next+1:], ')') {
						return true
					}
				case '[':
					if slices.Contains(p[next+1:], ']') {
						return true
					}
				}
			}
			break
		}
	}
	return false
}

// ---- the #419 fixture ----

const (
	fixture419Runes = 1886
	fixture419SHA   = "03840b619ebd80414a7267cb7e1bf370606ad12e2f7f805aebe43cc36e37fdba"
	fixture419Exp   = "every required slot is fillable from the request and the repository"
)

// fixture419 reads the Actual text of the punch-out comment on #419. It fails
// the test unless the text has exactly 1886 runes, no newline and the recorded
// SHA-256, so a bad copy cannot hide behind the red failures.
func fixture419(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "punchout-419-actual.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	s := strings.TrimSuffix(string(b), "\n")
	if n := utf8.RuneCountInString(s); n != fixture419Runes {
		t.Fatalf("fixture has %d runes, want %d", n, fixture419Runes)
	}
	if strings.Contains(s, "\n") {
		t.Fatal("fixture contains a newline")
	}
	sum := sha256.Sum256([]byte(s))
	if got := hex.EncodeToString(sum[:]); got != fixture419SHA {
		t.Fatalf("fixture SHA-256 = %s, want %s", got, fixture419SHA)
	}
	return s
}

func render419(actual string) string {
	fc := guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    "intake",
		Attempt:  guard.Attempt{Current: 1, Max: 1},
		Expected: fixture419Exp,
		Actual:   actual,
	}
	return RenderComment(fc, "r", "", "kairon-failed")
}

// TestRenderMarkdown_Paragraph419 renders the real Actual text of the #419
// punch-out comment.
//
// Hand derivation (rules of renderRun in the spec): the value is one line with
// no leading white space, no tab and no value fence, so it is one text run. The
// backtick at rune offset 250 starts a run of one backtick whose codeSpanEnd is
// the backtick at offset 332 on the same line (row 1), so offsets 250 to 332,
// with their [a-z-], \. and parentheses, are written verbatim. Outside that
// span there is no backtick, [, ], backslash, <, @, #, !, &, http, www. or GH-,
// so every other rune is written as itself (row 13). The Actual section is
// therefore the fixture, and it is not truncated (1886 runes < 2000).
func TestRenderMarkdown_Paragraph419(t *testing.T) {
	fix := fixture419(t)
	got := render419(fix)
	sec := section(got, "Actual")
	if sec != fix {
		t.Errorf("Actual section differs from the fixture:\n got: %q\nwant: %q", sec, fix)
	}
	if hasFenceLine(sec) {
		t.Errorf("Actual section has a fence line: %q", sec)
	}
	if strings.Contains(sec, "\n") {
		t.Errorf("Actual section is not one line: %q", sec)
	}
	if strings.Contains(got, "truncated") {
		t.Errorf("comment has a truncation note:\n%s", got)
	}
	if why := fenceProblem(got); why != "" {
		t.Errorf("fences not balanced: %s\n%s", why, got)
	}
}

// TestRenderMarkdown_Paragraph419StrayBacktick removes the closing backtick of
// the real text (rune offset 332).
//
// Hand derivation: the backtick at offset 250 now has no later backtick on its
// line, so row 2 writes it as backslash-backtick. The text that was the span is
// now outside code: [a-z-] has a balanced ] followed by +, which is neither (
// nor [, so rows 3 and 4 do not match and row 12 writes the [ as backslash-[.
// The backslash at offset 283 is doubled by row 11. Parentheses, |, ', * and _
// are written as themselves (row 13). No token of rows 5 to 9 occurs. The
// expected value is the modified text with exactly those three replacements,
// built with one NewReplacer call: a NewReplacer makes one pass over the
// original text and never rescans its own output, which three chained
// strings.Replace calls in the order backtick, bracket, backslash would (the
// first call inserts a backslash that the third call then doubles).
func TestRenderMarkdown_Paragraph419StrayBacktick(t *testing.T) {
	fix := fixture419(t)
	r := []rune(fix)
	if r[250] != '`' || r[332] != '`' {
		t.Fatalf("fixture offsets 250/332 are %q/%q, want backticks", r[250], r[332])
	}
	modified := string(append(append([]rune{}, r[:332]...), r[333:]...))
	want := strings.NewReplacer("`", "\\`", "[", "\\[", "\\", "\\\\").Replace(modified)
	got := render419(modified)
	sec := section(got, "Actual")
	if sec != want {
		t.Errorf("Actual section:\n got: %q\nwant: %q", sec, want)
	}
	if hasFenceLine(sec) {
		t.Errorf("Actual section has a fence line: %q", sec)
	}
	if strings.Contains(sec, "\n") {
		t.Errorf("Actual section is not one line: %q", sec)
	}
	if why := fenceProblem(got); why != "" {
		t.Errorf("fences not balanced: %s", why)
	}
}

func TestRenderMarkdown_ParagraphsAndLineBreaks(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"blank line gives two paragraphs", "one\n\ntwo", "one\n\ntwo"},
		{"single newline is a line break", "one\ntwo", "one\ntwo"},
		{"white-space-only line is a blank line", "one\n \t\ntwo", "one\n\ntwo"},
		{"leading and trailing blank lines dropped", "\n\none\n\n", "one"},
		{"trailing spaces trimmed", "one  \ntwo\t", "one\ntwo"},
		{"CRLF becomes LF", "one\r\ntwo\r\n\r\nthree", "one\ntwo\n\nthree"},
		{"several blank lines stay paragraph breaks", "one\n\n\ntwo", "one\n\n\ntwo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, sec := renderActual(c.in)
			if sec != c.want {
				t.Errorf("section:\n got: %q\nwant: %q", sec, c.want)
			}
			if hasFenceLine(sec) {
				t.Errorf("section has a fence line: %q", sec)
			}
			if why := fenceProblem(got); why != "" {
				t.Errorf("fences not balanced: %s", why)
			}
		})
	}
}

func TestRenderMarkdown_KeepsOwnFence(t *testing.T) {
	// No case has a fence opener indented by 2 or 3 spaces: that is an indented
	// line and the whole value is fenced (see IndentedValueIsFenced).
	cases := []struct{ name, in string }{
		{"backtick fence", "before\n```\nx := 1\n```\nafter"},
		{"tilde fence", "before\n~~~\nx := 1\n~~~\nafter"},
		{"4-backtick fence holding a 3-backtick line", "````\n```\nx\n````"},
		{"opener and closer with one leading space", " ```\nx\n ```"},
		{"closer with 3 leading spaces", "```\nx\n   ```\ny"},
		{"one-space opener after text", "a\n ```\nx\n```"},
		{"one-space opener after a list item", "- a\n ```\nx\n```"},
		{"indented lines inside the fence", "```\n  indented\n\tmore\n```"},
		{"fence with an info string", "```go\nx := 1\n```"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, sec := renderActual(c.in)
			if sec != c.in {
				t.Errorf("section:\n got: %q\nwant: %q", sec, c.in)
			}
			if why := fenceProblem(got); why != "" {
				t.Errorf("fences not balanced: %s\n%s", why, got)
			}
		})
	}
}

func TestRenderMarkdown_IndentedValueIsFenced(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"Go stack trace with a tab-indented line",
			"panic: x\n\ngoroutine 1 [running]:\nmain.main()\n\t/src/main.go:10 +0x1d",
			"```\npanic: x\n\ngoroutine 1 [running]:\nmain.main()\n\t/src/main.go:10 +0x1d\n```",
		},
		{
			"fence longer than a 5-backtick run in the trace",
			"panic: x `````\n\tframe",
			"``````\npanic: x `````\n\tframe\n``````",
		},
		{"two-space-indented line", "a\n  b", "```\na\n  b\n```"},
		{"tab-indented single line", "\tx", "```\n\tx\n```"},
		{"own fence plus an indented line outside it", "```\nx\n```\n  y", "````\n```\nx\n```\n  y\n````"},
		{"list item with a 2-space fence", "- a\n  ```\n  x", "````\n- a\n  ```\n  x\n````"},
		{"3-space fence line after text", "a\n   ```\nx\n```", "````\na\n   ```\nx\n```\n````"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, sec := renderActual(c.in)
			if sec != c.want {
				t.Errorf("section:\n got: %q\nwant: %q", sec, c.want)
			}
			if why := valueProblem(got, sec); why != "" {
				t.Error(why)
			}
			first, _, _ := strings.Cut(sec, "\n")
			if strings.Trim(first, "`") != "" || len(first) <= testBacktickRun(c.in) {
				t.Errorf("opening fence %q is not a backtick fence longer than any run in %q", first, c.in)
			}
		})
	}

	t.Run("one leading space does not fence the value", func(t *testing.T) {
		got, sec := renderActual("a\n b")
		if sec != "a\n b" {
			t.Errorf("section = %q, want %q", sec, "a\n b")
		}
		if hasFenceLine(sec) {
			t.Errorf("section has a fence line: %q", sec)
		}
		if why := fenceProblem(got); why != "" {
			t.Error(why)
		}
	})
}

// fenceRow is one exact row of TestRenderMarkdown_FenceStructure.
type fenceRow struct {
	name, in, want string
	remedyAfter    bool // the Remedy heading is followed by the default remedy outside every fence
}

func TestRenderMarkdown_FenceStructure(t *testing.T) {
	cutPrefix := "1. Run this:\n   ```\n   go test ./...\n   "
	cutIn := cutPrefix + strings.Repeat("y", 2100)
	cutWant := "````\n" + cutPrefix + strings.Repeat("y", 2000-utf8.RuneCountInString(cutPrefix)) +
		"\n````\n\n_(truncated to 2000 characters)_"

	rows := []fenceRow{
		// Fenced as a whole (4 backticks, each value holds a run of 3).
		{"list-item fence indented 2, value ends inside it", "- a\n  ```\n  x", "````\n- a\n  ```\n  x\n````", false},
		{"fence indented 3 after text, closer at column 0", "a\n   ```\nx\n```", "````\na\n   ```\nx\n```\n````", false},
		{
			"unclosed list-item fence", "- step\n  ```\n  go test ./...",
			"````\n- step\n  ```\n  go test ./...\n````", true,
		},
		{
			"closed list-item fence then a link, a mention and HTML",
			"- step\n  ```\nnote <details> @bob [a](https://x.y)\n```",
			"````\n- step\n  ```\nnote <details> @bob [a](https://x.y)\n```\n````", true,
		},
		{"list-item fence cut at 2000 runes", cutIn, cutWant, true},
		// Kept as the value wrote it (opener with 0 or 1 leading space).
		{
			"two-space line closes the fence, the rest is text",
			"```\nx\n  ```\n<details> @bob [a](https://x.y)",
			"```\nx\n  ```\n&lt;details> `@bob` `[a](https://x.y)`", false,
		},
		{"one-space opener and closer", " ```\nx\n ```", " ```\nx\n ```", false},
		{"one-space opener after text", "a\n ```\nx\n```", "a\n ```\nx\n```", false},
		{"one-space opener after a list item", "- a\n ```\nx\n```", "- a\n ```\nx\n```", false},
		{"closer with 3 leading spaces", "```\nx\n   ```\ny", "```\nx\n   ```\ny", false},
		{"unclosed fence is closed", "```\nx", "```\nx\n```", false},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got, sec := renderActual(r.in)
			if sec != r.want {
				t.Errorf("section:\n got: %q\nwant: %q", sec, r.want)
			}
			if why := valueProblem(got, sec); why != "" {
				t.Error(why)
			}
			if r.remedyAfter {
				out := outsideFences(got)
				if !strings.Contains(out, "**Remedy**\n\nMake this true") {
					t.Errorf("the Remedy heading is not followed by the default remedy outside every fence:\n%s", got)
				}
				if !strings.Contains(out, "To restart this issue") {
					t.Errorf("the restart line is inside a fence:\n%s", got)
				}
			}
		})
	}

	t.Run("sweep", func(t *testing.T) {
		rng := rand.New(rand.NewSource(485))
		for i := 0; i < 20000; i++ {
			in := sweepValue(rng)
			got, sec := renderActual(in)
			if why := valueProblem(got, sec); why != "" {
				t.Fatalf("value #%d %q: %s", i, in, why)
			}
		}
	})
}

// sweepValue builds a value of 1 to 7 lines from list-prefixed lines, fence
// lines at several indents, indented text and blank lines; 15% get a long tail
// so the 2000-rune cut can fall inside a fence.
func sweepValue(rng *rand.Rand) string {
	prefixes := []string{"- ", "* ", "+ ", "1. ", "10. ", "> ", "# ", "| a | ", "--- ", "", ""}
	texts := []string{
		"step", "go test ./...", "x := 1", "plain text", "see #12 and GH-3", "[a](u)",
		"![i](https://x.y/i.png)", "[ref]: https://x.y", "note <details> @bob [a](https://x.y)", "<!-- x",
	}
	fences := []string{"```", "~~~", "````", "~~~~", "```go", "~~~x"}
	indents := []string{"", " ", "  ", "   ", "    ", "\t"}
	var lines []string
	for n := 1 + rng.Intn(7); n > 0; n-- {
		switch rng.Intn(4) {
		case 0, 1:
			lines = append(lines, prefixes[rng.Intn(len(prefixes))]+texts[rng.Intn(len(texts))])
		case 2:
			if rng.Intn(2) == 0 {
				lines = append(lines, indents[rng.Intn(len(indents))]+fences[rng.Intn(len(fences))])
			} else {
				lines = append(lines, indents[rng.Intn(len(indents))]+texts[rng.Intn(len(texts))])
			}
		default:
			lines = append(lines, []string{"", "  ", "\t"}[rng.Intn(3)])
		}
	}
	v := strings.Join(lines, "\n")
	if rng.Intn(100) < 15 {
		v += "\n" + strings.Repeat("y", rng.Intn(3000))
	}
	return v
}

// ---- exact rows (spec: "Exact rows for tails, escaped brackets and definitions") ----

// exactRow is one in/want pair copied from the spec: in is the Expected value
// and want the exact Expected section (without the final newline). Where a
// pair seems to disagree with the spec's prose, the pair is the contract.
type exactRow struct{ group, in, want string }

var exactRows = []exactRow{
	{"tail-backtick", "[a](u`)b` c", "``[a](u`)``b\\` c"},
	{"tail-backtick", "[t](https://x.y/a`b) see `code`", "``[t](https://x.y/a`b)`` see `code`"},
	{"tail-backtick", "[a](u`)", "``[a](u`)``"},
	{"tail-backtick", "[a](u`b`)", "``[a](u`b`)``"},
	{"tail-backtick", "[a](<u`v>) `x`", "``[a](<u`v>)`` `x`"},
	{"tail-backtick", "[a](b\\()x", "`[a](b\\()`x"},
	{"tail-backtick", "[a](u \"x)y\")", "`[a](u \"x)y\")`"},
	{"linktext-backtick", "[a`](u)b`", "\\[a`](u)b`"},
	{"bracket", "[[](!])x](y)", "\\[`[](!])`x](y)"},
	{"bracket", "![[](!])x](y)", "!\\[`[](!])`x](y)"},
	{"bracket", "[#12]", "\\[`#12`]"},
	{"bracket", "[a [b](u)", "\\[a `[b](u)`"},
	{"bracket", "a [b] c", "a \\[b] c"},
	{"bracket", "x ![a", "x !\\[a"},
	{"bracket", "- [ ] todo", "- \\[ ] todo"},
	{"definition", "[ref]: https://x.y", "\\[ref]: `https://x.y`"},
	{"definition", "[^1]: note", "\\[^1]: note"},
	{"definition", "[foo\nbar]: https://x.y", "\\[foo\nbar]: `https://x.y`"},
	{"definition", "> [x]: u", "> \\[x]: u"},
	{"definition", "- [x]: u", "- \\[x]: u"},
	{"backslash", "\\[a](u)", "\\\\`[a](u)`"},
}

func exactRowsIn(groups ...string) []exactRow {
	var out []exactRow
	for _, r := range exactRows {
		if slices.Contains(groups, r.group) {
			out = append(out, r)
		}
	}
	return out
}

// checkExpectedRow renders in as the Expected value and checks the exact
// section and the structural properties.
func checkExpectedRow(t *testing.T, in, want string) {
	t.Helper()
	got, sec := renderExpected(in)
	if sec != want {
		t.Errorf("section:\n  in: %q\n got: %q\nwant: %q", in, sec, want)
	}
	if why := valueProblem(got, sec); why != "" {
		t.Errorf("in %q: %s", in, why)
	}
}

func TestRenderMarkdown_RawTokens(t *testing.T) {
	rows := []struct{ name, in, want string }{
		{"mention", "see @name now", "see `@name` now"},
		{"team mention", "see @org/team now", "see `@org/team` now"},
		{"issue reference", "see #123 now", "see `#123` now"},
		{"issue reference before a period", "see #123.", "see `#123`."},
		{"GH reference", "see GH-123 now", "see `GH-123` now"},
		{"lower-case gh reference", "see gh-1 now", "see `gh-1` now"},
		{"owner/repo reference", "see owner/repo#123 now", "see `owner/repo#123` now"},
		{"https URL", "see https://x.y/z now", "see `https://x.y/z` now"},
		{"http URL", "see http://x.y now", "see `http://x.y` now"},
		{"www URL", "see www.x.y now", "see `www.x.y` now"},
		{"inline link", "see [t](https://x.y) now", "see `[t](https://x.y)` now"},
		{"reference link", "see [t][r] now", "see `[t][r]` now"},
		{"collapsed reference link", "see [t][] now", "see `[t][]` now"},
		{"image", "see ![a](https://x.y/i.png) now", "see `![a](https://x.y/i.png)` now"},
		{"reference in brackets", "[#12]", "\\[`#12`]"},
		{"link text holding a mention is wrapped once", "[hi @bob](https://x.y)", "`[hi @bob](https://x.y)`"},
		// Nested brackets.
		{"nested-bracket link", "see [a [b] c](https://x.y) now", "see `[a [b] c](https://x.y)` now"},
		{"nested-bracket image", "see ![a [b]](https://x.y/i.png) now", "see `![a [b]](https://x.y/i.png)` now"},
		{"nested-bracket reference link", "[a [b] c][r]", "`[a [b] c][r]`"},
		{"inner link inside an outer link", "[a [b](u) c](v)", "`[a [b](u) c](v)`"},
		{"link text with a code span holding a bracket", "[a `]` b](u)", "``[a `]` b](u)``"},
		{"title holding a parenthesis", "[a](u \"x)y\")", "`[a](u \"x)y\")`"},
		// Links and images that cross a line break: one code span per line.
		{"link over two lines", "see [a\nb](https://x.y) now", "see `[a`\n`b](https://x.y)` now"},
		{"reference link over two lines", "[a\nb][r]", "`[a`\n`b][r]`"},
		{"image over two lines", "![a\nb](https://x.y/i.png)", "`![a`\n`b](https://x.y/i.png)`"},
		{"link over three lines", "[a\nmiddle\nb](u)", "`[a`\n`middle`\n`b](u)`"},
		{"second line with one leading space", "![a\n b](u)", "`![a`\n `b](u)`"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) { checkExpectedRow(t, r.in, r.want) })
	}
	for _, r := range exactRows {
		t.Run("exact/"+r.group+"/"+r.in, func(t *testing.T) { checkExpectedRow(t, r.in, r.want) })
	}
}

func TestRenderMarkdown_LiveLinkOracle(t *testing.T) {
	live := []string{
		"[a [b] c](u)",
		"![a [b]](u)",
		"[a\nb](u)",
		"[a][b]",
		"\\\\[a](u)", // an escaped backslash, then a live link
		"[a](u" + bt + ")b" + bt,
		"[t](https://x.y/a" + bt + "b) see " + bt + "code" + bt,
		"[[](!])x](y)",
	}
	for _, s := range live {
		if !hasLiveLink(s) {
			t.Errorf("hasLiveLink(%q) = false, want true", s)
		}
	}
	notLive := []string{
		bt + "[a](u)" + bt,
		"\\[a](u)",
		"[a]",
		"[a](u",
		"[a" + bt + "](u)b" + bt, // a code span hides the ] of the link text
		"one\n\n[a](u",
		"plain text (with parentheses) and [brackets]",
		"``[a](u`)``b\\` c",
		"``[t](https://x.y/a`b)`` see `code`",
	}
	for _, s := range notLive {
		if hasLiveLink(s) {
			t.Errorf("hasLiveLink(%q) = true, want false", s)
		}
	}
	for _, r := range exactRows {
		if hasLiveLink(r.want) {
			t.Errorf("hasLiveLink(want %q) = true, want false", r.want)
		}
	}
}

func TestRenderMarkdown_NoLiveLinkProperty(t *testing.T) {
	check := func(t *testing.T, in string, skipOracle bool) {
		t.Helper()
		got, sec := renderActual(in)
		if why := fenceProblem(got); why != "" {
			t.Fatalf("input %q: fences not balanced: %s", in, why)
		}
		if skipOracle {
			return
		}
		if hasFenceLine(sec) {
			t.Fatalf("input %q: Actual section has a fence line: %q", in, sec)
		}
		if hasLiveLink(outsideFences(sec)) {
			t.Fatalf("input %q: live link in section %q", in, sec)
		}
	}
	skip := func(in string) bool {
		return strings.TrimSpace(in) == "" || strings.Contains(in, "```")
	}
	syms := []string{"[", "]", "(", ")", "!", "\\", bt, "a", "\n"}

	t.Run("fixed inputs", func(t *testing.T) {
		for _, in := range []string{
			"[[](!])x](y)",
			"![[](!])x](y)",
			"[a](u" + bt + ")b" + bt,
			"[t](https://x.y/a" + bt + "b) see " + bt + "code" + bt,
			"[\n](" + bt + "\\()" + bt + "a\\]]",
		} {
			check(t, in, false)
		}
	})
	t.Run("all strings of length 1 to 5", func(t *testing.T) {
		for n := 1; n <= 5; n++ {
			total := 1
			for i := 0; i < n; i++ {
				total *= len(syms)
			}
			for k := 0; k < total; k++ {
				var b strings.Builder
				for v, i := k, 0; i < n; i, v = i+1, v/len(syms) {
					b.WriteString(syms[v%len(syms)])
				}
				check(t, b.String(), skip(b.String()))
			}
		}
	})
	t.Run("100000 random strings of length 6 to 16", func(t *testing.T) {
		rng := rand.New(rand.NewSource(485))
		for i := 0; i < 100000; i++ {
			var b strings.Builder
			for n := 6 + rng.Intn(11); n > 0; n-- {
				b.WriteString(syms[rng.Intn(len(syms))])
			}
			check(t, b.String(), skip(b.String()))
		}
	})
}

func TestRenderMarkdown_RawTokensWithBackticks(t *testing.T) {
	rows := []struct{ name, in, want string }{
		{"backtick in link text", "[a" + bt + "b](https://x.y)", "``[a`b](https://x.y)``"},
		{"URL ending in a backtick", "see https://x.y/a" + bt, "see `` https://x.y/a` ``"},
		{"token with a 2-backtick run", "[a" + bt + bt + "b](u)", "```[a``b](u)```"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) { checkExpectedRow(t, r.in, r.want) })
	}
	for _, r := range exactRowsIn("tail-backtick", "linktext-backtick") {
		t.Run("exact/"+r.in, func(t *testing.T) { checkExpectedRow(t, r.in, r.want) })
	}
	// A backtick in the destination does not hide the tail: the link opener
	// stays at the start of the want, inside a code span, not escaped.
	for _, in := range []string{"[a](u" + bt + ")b" + bt, "[t](https://x.y/a" + bt + "b) see " + bt + "code" + bt} {
		_, sec := renderExpected(in)
		if !strings.HasPrefix(sec, "``[") {
			t.Errorf("render of %q = %q, want it to start with a double-backtick wrapper and an unescaped [", in, sec)
		}
	}
}

func TestRenderMarkdown_RawTokensLeftAlone(t *testing.T) {
	for _, in := range []string{
		"a@b.com",
		"go install x@latest",
		"C#",
		"foo#1",
		"&#35;",
		"version 1.2.3",
		"a plain sentence: with a colon; a semicolon (and parentheses).",
	} {
		t.Run(in, func(t *testing.T) {
			got, sec := renderActual(in)
			if sec != in {
				t.Errorf("section = %q, want %q", sec, in)
			}
			if why := fenceProblem(got); why != "" {
				t.Error(why)
			}
		})
	}
}

func TestRenderMarkdown_UserCodeSpansUntouched(t *testing.T) {
	rows := []struct{ name, in, want string }{
		{"mention in a code span", "`@name`", "`@name`"},
		{"HTML in a code span", "`<details>`", "`<details>`"},
		{"double-backtick span holding a backtick", "``a ` b``", "``a ` b``"},
		{"token next to a span is still wrapped", "`x` @name", "`x` `@name`"},
		{"a span does not cross a line break", "a `b\nc` @x", "a \\`b\nc\\` `@x`"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got, sec := renderActual(r.in)
			if sec != r.want {
				t.Errorf("section:\n got: %q\nwant: %q", sec, r.want)
			}
			if hasFenceLine(sec) {
				t.Errorf("section has a fence line: %q", sec)
			}
			if why := fenceProblem(got); why != "" {
				t.Error(why)
			}
		})
	}
}

func TestRenderMarkdown_StrayBackticksAndBackslashes(t *testing.T) {
	rows := []struct{ name, in, want string }{
		{"lone backtick", "`", "\\`"},
		{"stray backtick next to a mention", "a ` b @name", "a \\` b `@name`"},
		{"backslash before a quote", "\\\"", "\\\\\""},
		{"backslash before <", "\\<x>", "\\\\&lt;x>"},
		{"backslash before a link", "\\[a](u)", "\\\\`[a](u)`"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got, sec := renderActual(r.in)
			if sec != r.want {
				t.Errorf("section:\n got: %q\nwant: %q", sec, r.want)
			}
			if hasFenceLine(sec) {
				t.Errorf("section has a fence line: %q", sec)
			}
			if why := valueProblem(got, sec); why != "" {
				t.Error(why)
			}
		})
	}
}

// codeSpanTexts returns the contents of the code spans in s, read as
// CommonMark does: a backtick run pairs with the next run of the same length.
func codeSpanTexts(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		n := j - i
		end := -1
		for k := j; k < len(s); {
			if s[k] != '`' {
				k++
				continue
			}
			m := k
			for m < len(s) && s[m] == '`' {
				m++
			}
			if m-k == n {
				end = k
				break
			}
			k = m
		}
		if end < 0 {
			i = j
			continue
		}
		out = append(out, strings.Trim(s[j:end], " "))
		i = end + n
	}
	return out
}

func TestRenderMarkdown_AdjacentBackticks(t *testing.T) {
	rows := []struct {
		name, in, want string
		spans          []string
	}{
		{"token then a user span", "@a`x`", "`@a` `x`", []string{"@a", "x"}},
		{"user span then a token", "`x`@a", "`x` `@a`", []string{"x", "@a"}},
		{"two adjacent links", "[a](b)[c](d)", "`[a](b)` `[c](d)`", []string{"[a](b)", "[c](d)"}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			_, sec := renderActual(r.in)
			if sec != r.want {
				t.Errorf("section:\n got: %q\nwant: %q", sec, r.want)
			}
			if got := codeSpanTexts(sec); !slices.Equal(got, r.spans) {
				t.Errorf("code spans in %q = %q, want %q", sec, got, r.spans)
			}
		})
	}
}

func TestRenderMarkdown_DefinitionLines(t *testing.T) {
	rows := []struct{ name, in, want string }{
		{"label that crosses a line break", "[foo\nbar]: https://x.y", "\\[foo\nbar]: `https://x.y`"},
		{"shortcut reference with no definition", "[x] y", "\\[x] y"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) { checkExpectedRow(t, r.in, r.want) })
	}
	for _, r := range exactRowsIn("definition") {
		t.Run("exact/"+r.in, func(t *testing.T) { checkExpectedRow(t, r.in, r.want) })
	}
	// "- [ ] todo" starts no definition, but its [ is written \[ all the same.
	checkExpectedRow(t, "- [ ] todo", "- \\[ ] todo")
}

func TestRenderMarkdown_HTMLEscapedOutsideCode(t *testing.T) {
	t.Run("outside code", func(t *testing.T) {
		for _, c := range []struct{ in, want string }{
			{"see issue-<number>-<slug>.md", "see issue-&lt;number>-&lt;slug>.md"},
			{"<details>", "&lt;details>"},
			{"<!-- x", "&lt;!-- x"},
		} {
			got, sec := renderActual(c.in)
			if sec != c.want {
				t.Errorf("section of %q:\n got: %q\nwant: %q", c.in, sec, c.want)
			}
			if strings.Contains(outsideFences(sec), "<") {
				t.Errorf("section of %q has a raw < outside code: %q", c.in, sec)
			}
			if why := fenceProblem(got); why != "" {
				t.Error(why)
			}
		}
	})

	t.Run("unchanged inside a value fence", func(t *testing.T) {
		in := "```\n<details>\n<!-- x\n```"
		_, sec := renderActual(in)
		if sec != in {
			t.Errorf("section:\n got: %q\nwant: %q", sec, in)
		}
		if strings.Contains(sec, "&lt;") {
			t.Errorf("a value fence must keep < as typed: %q", sec)
		}
	})

	t.Run("unchanged inside a whole-value fence", func(t *testing.T) {
		in := "\t<details>\n<!-- x"
		want := "```\n" + in + "\n```"
		_, sec := renderActual(in)
		if sec != want {
			t.Errorf("section:\n got: %q\nwant: %q", sec, want)
		}
		if strings.Contains(sec, "&lt;") {
			t.Errorf("a whole-value fence must keep < as typed: %q", sec)
		}
	})

	t.Run("list-item fence followed by a details line and a comment-open line", func(t *testing.T) {
		in := "- step\n  ```\nnote <details> @bob [a](https://x.y)\n```\n<!-- x"
		want := "````\n" + in + "\n````"
		got, sec := renderActual(in)
		if sec != want {
			t.Errorf("section:\n got: %q\nwant: %q", sec, want)
		}
		if why := valueProblem(got, sec); why != "" {
			t.Error(why)
		}
	})

	t.Run("hostile marker in a value", func(t *testing.T) {
		fc := renderFC()
		fc.Actual = "<!-- kairon:punch-out run=evil -->\n<script>"
		fc.Expected = "<!-- kairon:punch-out run=evil2 -->"
		got := RenderComment(fc, "real", "x", "kairon-failed")
		if id, ok := ParseMarker(got); !ok || id != "real" {
			t.Errorf("ParseMarker = %q, %v; want real", id, ok)
		}
		if n := strings.Count(got, "<!--"); n != 1 {
			t.Errorf("comment has %d occurrences of <!--, want only the marker on line 1:\n%s", n, got)
		}
		if strings.Contains(got, "<script>") {
			t.Errorf("comment holds a raw <script>:\n%s", got)
		}
	})
}

func TestRenderMarkdown_TruncationClosesFence(t *testing.T) {
	const note = "\n\n_(truncated to 2000 characters)_"
	ys := func(n int) string { return strings.Repeat("y", n) }

	cases := []struct{ name, in, want string }{
		{
			"backtick fence", "intro\n```\n" + ys(2100),
			"intro\n```\n" + ys(2000-10) + "\n```\n" + note[1:],
		},
		{
			"tilde fence", "intro\n~~~\n" + ys(2100),
			"intro\n~~~\n" + ys(2000-10) + "\n~~~\n" + note[1:],
		},
		{
			"4-backtick fence", "intro\n````\n" + ys(2100),
			"intro\n````\n" + ys(2000-11) + "\n````\n" + note[1:],
		},
		{
			"whole-value (indented) value", "\tx\n" + ys(2100),
			"```\n\tx\n" + ys(2000-3) + "\n```\n" + note[1:],
		},
		{
			"list-item fence cut at the limit",
			"1. Run this:\n   ```\n   go test ./...\n   " + ys(2100),
			"````\n1. Run this:\n   ```\n   go test ./...\n   " +
				ys(2000-utf8.RuneCountInString("1. Run this:\n   ```\n   go test ./...\n   ")) + "\n````\n" + note[1:],
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, sec := renderActual(c.in)
			if sec != c.want {
				t.Errorf("Actual section:\n got: %.120q ... %.60q\nwant: %.120q ... %.60q",
					sec, sec[max(0, len(sec)-60):], c.want, c.want[max(0, len(c.want)-60):])
			}
			if why := fenceProblem(got); why != "" {
				t.Errorf("fences not balanced: %s", why)
			}
			closeAt := strings.Index(got, "\n**Remedy**")
			noteAt := strings.Index(got, "_(truncated to 2000 characters)_")
			if noteAt < 0 || noteAt > closeAt {
				t.Errorf("the truncation note is missing or after the Remedy heading")
			}
		})
	}

	t.Run("Expected cut inside a fence", func(t *testing.T) {
		in := "intro\n```\n" + ys(2100)
		got, sec := renderExpected(in)
		want := "intro\n```\n" + ys(2000-10) + "\n```\n" + note[1:]
		if sec != want {
			t.Errorf("Expected section differs: got %d bytes, want %d", len(sec), len(want))
		}
		if why := fenceProblem(got); why != "" {
			t.Errorf("fences not balanced: %s", why)
		}
	})

	t.Run("text cut in the middle of a link, a code span or a mention", func(t *testing.T) {
		for _, in := range []string{
			"x [a](https://x.y/" + ys(2100),
			"x " + bt + "code " + ys(2100),
			"x @" + ys(2100),
			"x [a " + ys(2100),
		} {
			got, sec := renderActual(in)
			if hasFenceLine(sec) {
				t.Errorf("section has a fence line: %.80q", sec)
			}
			if !strings.HasSuffix(sec, "_(truncated to 2000 characters)_") {
				t.Errorf("section lacks the truncation note: %.80q", sec)
			}
			if why := valueProblem(got, sec); why != "" {
				t.Errorf("input %.40q: %s", in, why)
			}
		}
	})
}

func TestRenderMarkdown_TruncationNote(t *testing.T) {
	t.Run("note follows a blank line and 2000 runes are kept", func(t *testing.T) {
		_, sec := renderActual(strings.Repeat("é", 5000))
		want := strings.Repeat("é", 2000) + "\n\n_(truncated to 2000 characters)_"
		if sec != want {
			t.Errorf("section: got %d runes, want %d with a note after a blank line", utf8.RuneCountInString(sec), utf8.RuneCountInString(want))
		}
	})
	t.Run("exactly 2000 runes has no note", func(t *testing.T) {
		_, sec := renderActual(strings.Repeat("é", 2000))
		if sec != strings.Repeat("é", 2000) {
			t.Errorf("section = %.60q..., want 2000 é and no note", sec)
		}
	})
}

func TestRenderMarkdown_DefaultRemedy(t *testing.T) {
	const lead = "Make this true, then remove the label `kairon-failed`:\n\n"
	in := "set <placeholder> per #12 now"
	want := lead + "set &lt;placeholder> per `#12` now\n"
	for _, remedy := range []string{"", "  \n\t "} {
		got := renderWith("none", in, remedy)
		if r := remedySection(got); r != want {
			t.Errorf("remedy %q: remedy section:\n got: %q\nwant: %q", remedy, r, want)
		}
		if hasFenceLine(remedySection(got)) {
			t.Errorf("default remedy has a fence line:\n%s", got)
		}
	}
}

func TestRenderMarkdown_CallerRemedyUnchanged(t *testing.T) {
	fc := renderFC()
	fc.Attempt = guard.Attempt{Current: 1, Max: 3}
	fc.Expected = "see #1 now"
	fc.Actual = "none"
	got := RenderComment(fc, "r", "do <x> and #1", "kairon-failed")
	want := Marker("r") + "\n\n**Kairon stopped work on this issue and needs a human.**\n\n" +
		"- Stage: `krew-lead`\n- Guard: `implementation-run`\n- Attempts made: 1 of 3\n\n" +
		"**Expected**\n\nsee `#1` now\n\n**Actual**\n\nnone\n\n**Remedy**\n\ndo <x> and #1\n\n" +
		"To restart this issue, fix the cause and remove the `kairon-failed` label. " +
		"The pipeline picks the issue up again with a fresh retry budget.\n"
	if got != want {
		t.Errorf("comment:\n got: %q\nwant: %q", got, want)
	}

	long := strings.Repeat("r", 2100)
	got = RenderComment(fc, "r", long, "kairon-failed")
	if r := remedySection(got); r != strings.Repeat("r", 2000)+"\n" {
		t.Errorf("caller remedy not cut to 2000 runes without a note: %d bytes", len(r))
	}
}

// TestRenderMarkdown_GoldenExample holds a whole-comment golden string derived
// by hand from the spec rules. update-failure-docs copies this string into
// docs/failure-contract.md as the example.
//
// Derivation: the marker, intro, list and headings are unchanged. Expected is
// one text run: "#42" follows a space and is followed by a space, so row 8
// wraps it; "<" becomes &lt; (row 10); ">" and "." stay. Actual has two lines
// and so one run with a line break kept as a single newline; "@alice" follows a
// space (row 9) and is wrapped. The caller remedy is written raw and followed by
// a newline, then the restart line.
func TestRenderMarkdown_GoldenExample(t *testing.T) {
	fc := guard.FailureContext{
		Stage:    stageoutput.StageKrewLead,
		Guard:    "implementation-run",
		Attempt:  guard.Attempt{Current: 2, Max: 3},
		Expected: "A pull request exists for issue #42 and docs/issue-<number>.md is written.",
		Actual:   "krew-lead exited 1 after @alice restarted it.\nThe log is .kairon/logs/issue-42.log.",
	}
	got := RenderComment(fc, "run-1", "Read .kairon/logs/issue-42.log and fix the cause.", "kairon-failed")
	want := "<!-- kairon:punch-out run=run-1 -->\n" +
		"\n" +
		"**Kairon stopped work on this issue and needs a human.**\n" +
		"\n" +
		"- Stage: `krew-lead`\n" +
		"- Guard: `implementation-run`\n" +
		"- Attempts made: 2 of 3\n" +
		"\n" +
		"**Expected**\n" +
		"\n" +
		"A pull request exists for issue `#42` and docs/issue-&lt;number>.md is written.\n" +
		"\n" +
		"**Actual**\n" +
		"\n" +
		"krew-lead exited 1 after `@alice` restarted it.\n" +
		"The log is .kairon/logs/issue-42.log.\n" +
		"\n" +
		"**Remedy**\n" +
		"\n" +
		"Read .kairon/logs/issue-42.log and fix the cause.\n" +
		"\n" +
		"To restart this issue, fix the cause and remove the `kairon-failed` label. " +
		"The pipeline picks the issue up again with a fresh retry budget.\n"
	if got != want {
		t.Errorf("golden comment:\n got: %q\nwant: %q", got, want)
	}
}
