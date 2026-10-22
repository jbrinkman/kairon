package planner

import (
	"regexp"
	"strings"
)

var (
	h1Re            = regexp.MustCompile(`^ {0,3}#[ \t]+(.*)$`)
	closingHashesRe = regexp.MustCompile(`[ \t]+#+[ \t]*$`)
)

// SplitTitle splits the H1 title line off a canonical body.
//
// The title is the first non-blank line, and only if it is a level-1 heading
// ("# Title"). The returned rest is the body without that line and without the
// single blank line that follows it; blank lines before the title are dropped
// too. ok is false when the body has no such title or the title is empty, in
// which case title and rest are empty.
//
// The GitHub issue title comes from this line (provenance.Request.Title) and
// the envelope has no title key, so the boundary removes it before the guard
// chain and the hash: the hash covers exactly the body that is committed.
func SplitTitle(body string) (title, rest string, ok bool) {
	lines := strings.Split(body, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i == len(lines) {
		return "", "", false
	}
	m := h1Re.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
	if m == nil {
		return "", "", false
	}
	title = strings.TrimSpace(closingHashesRe.ReplaceAllString(m[1], ""))
	if title == "" || strings.Trim(title, "# \t") == "" {
		return "", "", false
	}
	next := i + 1
	if next < len(lines) && strings.TrimSpace(lines[next]) == "" {
		next++
	}
	return title, strings.Join(lines[next:], "\n"), true
}
