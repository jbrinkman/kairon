package guard

import (
	"regexp"
	"strconv"
	"strings"
)

// Finding kinds reported by CheckScope.
const (
	// ScopeUncovered: no Scope Coverage row quotes the item, or its rows name
	// no task.
	ScopeUncovered = "uncovered"
	// ScopeUnknownTask: a Scope Coverage row names a task ID that is not in
	// the spec's kiro-plan block.
	ScopeUnknownTask = "unknown_task"
	// ScopeDeferredSection: the spec's Deferred work section is missing, empty,
	// or says anything other than none.
	ScopeDeferredSection = "deferred_section"
)

const (
	inScope       = "In Scope"
	scopeCoverage = "Scope Coverage"
	deferredWork  = "Deferred work"
)

// taskIDRE is the plan's task ID shape (internal/plan taskIDPattern). The guard
// package may not import internal/plan, so the pattern is repeated here.
var taskIDRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var (
	tableSeparatorRE = regexp.MustCompile(`^\|?[ \t]*:?-+:?[ \t]*(\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)
	codeSpanRE       = regexp.MustCompile("`([^`]+)`")
)

// ScopeItem is one In Scope item (ID s-N) or acceptance criterion (ID ac-N).
// Text is the item's first line without its list marker.
type ScopeItem struct {
	ID   string
	Text string
}

// ScopeFinding is one reason a spec does not cover its issue. Line is the
// 1-based spec line, or 0 when the finding has no line (an uncovered item, a
// missing section).
type ScopeFinding struct {
	Kind string `json:"kind"`
	Item string `json:"item"`
	Line int    `json:"line,omitempty"`
	Text string `json:"text"`
}

// ScopeItems lists every In Scope item, then every acceptance criterion, in
// body order. Acceptance criteria are read exactly as the structural guard
// reads them, so the two never disagree about what an item is.
func ScopeItems(body string) []ScopeItem {
	var out []ScopeItem
	for _, it := range scopeItems(body) {
		out = append(out, it.ScopeItem)
	}
	return out
}

// scopeItem is a ScopeItem plus the item's whole text, normalized, so a row
// may quote the first line or more of the item.
type scopeItem struct {
	ScopeItem
	full string
}

func scopeItems(body string) []scopeItem {
	lines := scanLines(body)
	var out []scopeItem
	add := func(prefix, section string) {
		for i, it := range sectionListItems(lines, section) {
			out = append(out, scopeItem{
				ScopeItem: ScopeItem{ID: prefix + strconv.Itoa(i+1), Text: itemHeadline(it)},
				full:      itemText(it),
			})
		}
	}
	add("s-", inScope)
	add("ac-", acceptanceCriteria)
	return out
}

// CheckScope reports every way spec fails to cover the issue body: an item no
// Scope Coverage row quotes, a row naming a task not in taskIDs (the spec's
// kiro-plan task IDs), and a Deferred work section that is not exactly none.
// It is presence-only: it does not judge whether the named tasks deliver the
// item, and it never scans for deferral language, which needs judgment and is
// left to the validator.
func CheckScope(body, spec string, taskIDs []string) []ScopeFinding {
	known := make(map[string]bool, len(taskIDs))
	for _, id := range taskIDs {
		known[id] = true
	}
	lines := scanLines(spec)
	rows := coverageRows(lines)

	var findings []ScopeFinding
	matched := make([]bool, len(rows))
	reported := map[string]bool{}
	reportUnknown := func(r coverageRow, item string) {
		for _, id := range r.tasks {
			key := strconv.Itoa(r.line) + "\x00" + id
			if known[id] || reported[key] {
				continue
			}
			reported[key] = true
			findings = append(findings, ScopeFinding{Kind: ScopeUnknownTask, Item: item, Line: r.line, Text: id})
		}
	}

	for _, it := range scopeItems(body) {
		var found, hasKnown, hasUnknown bool
		for i, r := range rows {
			if !r.quotes(it) {
				continue
			}
			found = true
			matched[i] = true
			for _, id := range r.tasks {
				if known[id] {
					hasKnown = true
				} else {
					hasUnknown = true
				}
			}
		}
		switch {
		case hasUnknown:
			for i, r := range rows {
				if matched[i] && r.quotes(it) {
					reportUnknown(r, it.ID)
				}
			}
		case !found || !hasKnown:
			findings = append(findings, ScopeFinding{Kind: ScopeUncovered, Item: it.ID, Text: it.Text})
		}
	}
	for i, r := range rows {
		if !matched[i] {
			reportUnknown(r, r.label)
		}
	}

	if f, ok := checkDeferredSection(lines); !ok {
		findings = append(findings, f)
	}
	return findings
}

// coverageRow is one body row of the Scope Coverage table.
type coverageRow struct {
	line   int    // 1-based spec line
	label  string // first cell, used to name a row that quotes no item
	quoted string // the normalized text between the first and last double quote of the item cells
	tasks  []string
}

// quotes reports whether the row quotes the item: the quoted text holds at
// least the item's first line and nothing beyond the item's own text. A longer
// item that starts with a shorter one, or a task name, never covers the
// shorter item.
func (r coverageRow) quotes(it scopeItem) bool {
	headline := normalizeQuote(it.Text)
	return headline != "" && len(r.quoted) >= len(headline) && strings.HasPrefix(normalizeQuote(it.full), r.quoted)
}

// coverageRows returns the body rows of every table in the Scope Coverage
// section. A row's tasks are the code spans in its last cell that have the
// plan's task ID shape and sit outside parentheses; parentheses hold
// commentary such as test names.
func coverageRows(lines []bodyLine) []coverageRow {
	var rows []coverageRow
	for _, start := range findSections(lines, scopeCoverage) {
		end := sectionEnd(lines, start)
		for i := start + 1; i < end; i++ {
			l := lines[i]
			text := strings.TrimSpace(l.text)
			if l.inFence || !strings.HasPrefix(text, "|") || tableSeparatorRE.MatchString(text) {
				continue
			}
			// The row directly above a separator is the header.
			if i+1 < end && tableSeparatorRE.MatchString(strings.TrimSpace(lines[i+1].text)) {
				continue
			}
			cells := splitCells(text)
			if len(cells) == 0 {
				continue
			}
			rows = append(rows, coverageRow{
				line:   i + 1,
				label:  strings.TrimSpace(cells[0]),
				quoted: quotedText(normalizeQuote(strings.Join(cells[:len(cells)-1], "|"))),
				tasks:  cellTaskIDs(cells[len(cells)-1]),
			})
		}
	}
	return rows
}

// splitCells splits a table row on pipes that are not backslash-escaped. It
// drops only the cells outside the leading and trailing pipes, so an empty
// column stays in place.
func splitCells(row string) []string {
	var cells []string
	var cur strings.Builder
	escaped := false
	for _, r := range row {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			cur.WriteRune(r)
			escaped = true
		case r == '|':
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	cells = append(cells, cur.String())
	if strings.HasPrefix(row, "|") {
		cells = cells[1:]
	}
	if len(cells) > 0 && strings.HasSuffix(row, "|") && !strings.HasSuffix(row, `\|`) {
		cells = cells[:len(cells)-1]
	}
	return cells
}

// cellTaskIDs returns the task IDs a "Delivered by" cell names.
func cellTaskIDs(cell string) []string {
	var outside strings.Builder
	depth := 0
	for _, r := range cell {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case depth == 0:
			outside.WriteRune(r)
		}
	}
	var ids []string
	for _, m := range codeSpanRE.FindAllStringSubmatch(outside.String(), -1) {
		if id := strings.TrimSpace(m[1]); taskIDRE.MatchString(id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// checkDeferredSection requires a Deferred work section whose only non-blank
// content is none.
func checkDeferredSection(lines []bodyLine) (ScopeFinding, bool) {
	start, ok := findSection(lines, deferredWork)
	if !ok {
		return ScopeFinding{Kind: ScopeDeferredSection, Item: deferredWork, Text: "missing"}, false
	}
	sawNone := false
	for i := start + 1; i < sectionEnd(lines, start); i++ {
		text := strings.TrimSpace(lines[i].text)
		if text == "" {
			continue
		}
		if !sawNone && isNone(text) {
			sawNone = true
			continue
		}
		return ScopeFinding{Kind: ScopeDeferredSection, Item: deferredWork, Line: i + 1, Text: text}, false
	}
	if !sawNone {
		return ScopeFinding{Kind: ScopeDeferredSection, Item: deferredWork, Line: start + 1, Text: "empty"}, false
	}
	return ScopeFinding{}, true
}

// isNone reports whether a line says none, allowing emphasis, a code span and
// a trailing period.
func isNone(text string) bool {
	t := strings.Trim(strings.ToLower(text), "*_` \t.")
	return t == "none"
}

// sectionListItems returns the list items of every section the heading names,
// in order.
func sectionListItems(lines []bodyLine, name string) []string {
	var items []string
	for _, start := range findSections(lines, name) {
		items = append(items, listItems(lines[start+1:sectionEnd(lines, start)])...)
	}
	return items
}

// itemHeadline returns a list item's first line without its marker.
func itemHeadline(item string) string {
	first, _, _ := strings.Cut(item, "\n")
	if m := listRE.FindString(first); m != "" {
		first = first[len(m):]
	}
	return strings.Join(strings.Fields(first), " ")
}

// quotedText returns the text between the first and the last double quote of
// s, or "" when s has fewer than two.
func quotedText(s string) string {
	first, last := strings.Index(s, `"`), strings.LastIndex(s, `"`)
	if first < 0 || last <= first {
		return ""
	}
	return strings.TrimSpace(s[first+1 : last])
}

// itemText returns a list item's whole text without its marker, with
// whitespace collapsed.
func itemText(item string) string {
	if m := listRE.FindString(item); m != "" {
		item = item[len(m):]
	}
	return strings.Join(strings.Fields(item), " ")
}

// normalizeQuote undoes the escapes a quote needs inside a table cell (\" and
// \|) and collapses whitespace, so a quoted item matches its source text.
func normalizeQuote(s string) string {
	s = strings.NewReplacer(`\"`, `"`, `\|`, `|`).Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
