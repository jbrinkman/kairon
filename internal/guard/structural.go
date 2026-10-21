package guard

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// StructuralName is the name the structural guard reports in
// FailureContext.Guard.
const StructuralName = "structural"

// acceptanceCriteria is the section whose list items must carry a Verify:
// marker.
const acceptanceCriteria = "Acceptance Criteria"

// FailureHandler enacts a guard's retry-versus-punch-out decision. The
// boundary supplies it; this package does not implement one.
type FailureHandler func(FailureContext) error

// DefaultRequiredSections are the sections a canonical planner body must
// contain. Treat it as read-only.
var DefaultRequiredSections = []string{"Problem", "Context", "Scope", "Acceptance Criteria", "Dependencies"}

var (
	headingRE = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*)$`)
	fenceRE   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	listRE    = regexp.MustCompile(`^([ \t]*)([-*+]|\d+[.)])[ \t]+`)
	verifyRE  = regexp.MustCompile(`(?i)\bverify(\*\*|__)?:`)
)

// Structural is the reference guard. It checks that a stage-output document
// parses under the strict schema and, for canonical documents, that the body
// has the required sections and that every acceptance criterion carries a
// Verify: marker. punch_out documents pass once they parse.
type Structural struct {
	// RequiredSections lists the section names the body must contain. A nil
	// slice means DefaultRequiredSections.
	RequiredSections []string

	handler FailureHandler
}

// NewStructural returns a Structural guard that uses DefaultRequiredSections
// and calls handler from OnFailure. A nil handler makes OnFailure return
// ErrNoFailureHandler.
func NewStructural(handler FailureHandler) *Structural {
	return &Structural{handler: handler}
}

// Admit implements Guard. It is pure: it reads in and returns a result.
func (s *Structural) Admit(in Input) AdmissionResult {
	doc, err := in.Document()
	if err != nil {
		return in.Reject(StructuralName,
			"document parses under the strict stage-output schema (stageoutput.Parse)",
			err.Error())
	}
	if doc.Result != stageoutput.ResultCanonical {
		return Admitted()
	}

	lines := scanLines(doc.Body)

	required := s.RequiredSections
	if required == nil {
		required = DefaultRequiredSections
	}
	var missing []string
	for _, name := range required {
		if _, ok := findSection(lines, name); !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return in.Reject(StructuralName,
			"sections present: "+strings.Join(required, ", "),
			"missing sections: "+strings.Join(missing, ", "))
	}

	start, ok := findSection(lines, acceptanceCriteria)
	if !ok {
		return Admitted()
	}
	items := listItems(lines[start+1 : sectionEnd(lines, start)])
	if len(items) == 0 {
		return in.Reject(StructuralName,
			"every acceptance criterion carries a Verify: line",
			"Acceptance Criteria section has no list items")
	}
	var bad []string
	for i, item := range items {
		if !verifyRE.MatchString(item) {
			bad = append(bad, strconv.Itoa(i+1))
		}
	}
	if len(bad) > 0 {
		return in.Reject(StructuralName,
			"every acceptance criterion carries a Verify: line",
			"criteria without Verify: "+strings.Join(bad, ", ")+" (of "+strconv.Itoa(len(items))+")")
	}
	return Admitted()
}

// OnFailure implements Guard. It passes fc to the injected handler and returns
// the handler's error, or ErrNoFailureHandler when none was supplied so a
// failure is never swallowed silently.
func (s *Structural) OnFailure(fc FailureContext) error {
	if s.handler == nil {
		return ErrNoFailureHandler
	}
	return s.handler(fc)
}

// bodyLine is one line of a markdown body with what the scanner learned about
// it.
type bodyLine struct {
	text    string
	inFence bool // inside a fenced code block, or a fence delimiter itself
	level   int  // heading level 1-6, or 0 when the line is not a heading
	heading string
}

// scanLines splits body into lines and marks fenced code and headings. A line
// inside a fence is never a heading.
func scanLines(body string) []bodyLine {
	raw := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	out := make([]bodyLine, len(raw))
	var fenceMark string // the opening fence run, "" when outside a fence
	for i, text := range raw {
		l := bodyLine{text: text}
		if m := fenceRE.FindStringSubmatch(text); m != nil {
			run := m[1]
			switch {
			case fenceMark == "":
				fenceMark = run
				l.inFence = true
			case run[0] == fenceMark[0] && len(run) >= len(fenceMark) &&
				strings.TrimSpace(text[len(m[0]):]) == "":
				fenceMark = ""
				l.inFence = true
			default:
				l.inFence = true
			}
		} else if fenceMark != "" {
			l.inFence = true
		} else if m := headingRE.FindStringSubmatch(text); m != nil {
			l.level = len(m[1])
			l.heading = normalizeHeading(m[2])
		}
		out[i] = l
	}
	return out
}

// normalizeHeading lower-cases heading text and trims whitespace, closing
// hashes, and leading or trailing punctuation such as ':' or '*'.
func normalizeHeading(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "# \t")
	s = strings.TrimFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return strings.ToLower(s)
}

// headingMatches reports whether a normalized heading names the section: equal
// to it, or starting with it followed by a non-alphanumeric character.
func headingMatches(heading, name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || !strings.HasPrefix(heading, n) {
		return false
	}
	rest := heading[len(n):]
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// findSection returns the index of the first heading line that names the
// section.
func findSection(lines []bodyLine, name string) (int, bool) {
	for i, l := range lines {
		if l.level > 0 && headingMatches(l.heading, name) {
			return i, true
		}
	}
	return 0, false
}

// sectionEnd returns the index of the first line after the section that starts
// at lines[start]: the next heading of the same or higher level, or the end.
func sectionEnd(lines []bodyLine, start int) int {
	level := lines[start].level
	for i := start + 1; i < len(lines); i++ {
		if lines[i].level > 0 && lines[i].level <= level {
			return i
		}
	}
	return len(lines)
}

// listItems splits a section into list items and returns each item's text,
// including its continuation lines. An item starts at a list marker whose
// indent is at most the first item's indent; deeper markers and plain lines
// continue the current item. Lines inside a fenced code block never start an
// item.
func listItems(lines []bodyLine) []string {
	var items []string
	var cur *strings.Builder
	base := -1
	for _, l := range lines {
		if !l.inFence {
			if m := listRE.FindStringSubmatch(l.text); m != nil {
				indent := indentWidth(m[1])
				if base < 0 || indent <= base {
					if base < 0 {
						base = indent
					}
					if cur != nil {
						items = append(items, cur.String())
					}
					cur = &strings.Builder{}
					cur.WriteString(l.text)
					continue
				}
			}
		}
		if cur != nil {
			cur.WriteString("\n")
			cur.WriteString(l.text)
		}
	}
	if cur != nil {
		items = append(items, cur.String())
	}
	return items
}

// indentWidth counts leading whitespace, with a tab as four columns.
func indentWidth(ws string) int {
	n := 0
	for _, r := range ws {
		if r == '\t' {
			n += 4
		} else {
			n++
		}
	}
	return n
}
