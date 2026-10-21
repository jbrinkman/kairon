package provenance

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrUnknownPlaceholder is returned when a dependencies line refers to a
	// "#{key}" that has no entry in the numbers map. That covers both an
	// unknown key and a forward reference to a chain item not yet committed.
	ErrUnknownPlaceholder = errors.New("provenance: unknown dependency placeholder")

	// ErrBadDependencyLine is returned when a dependencies line that contains
	// a placeholder has a part that is neither "#<digits>" nor "#{key}".
	ErrBadDependencyLine = errors.New("provenance: malformed dependencies line")
)

var (
	// dependenciesKeywordRe finds the keyword on a line. The parser that
	// consumes the output matches it case-insensitively.
	dependenciesKeywordRe = regexp.MustCompile(`(?i)dependencies:`)
	// placeholderAnywhereRe reports whether text contains a "#{key}" token.
	placeholderAnywhereRe = regexp.MustCompile(`#\{[A-Za-z0-9][A-Za-z0-9_-]*\}`)
	// placeholderPartRe and numberPartRe match one whole comma-separated part.
	placeholderPartRe = regexp.MustCompile(`^#\{([A-Za-z0-9][A-Za-z0-9_-]*)\}$`)
	numberPartRe      = regexp.MustCompile(`^#([0-9]+)$`)
)

// SubstituteDependencies replaces "#{key}" placeholders on dependencies lines
// of body with the issue numbers in numbers.
//
// A dependencies line is any line containing "dependencies:" (case
// insensitive). The text after the first occurrence is split on commas and
// every part must be "#<digits>" or "#{key}". Placeholders are replaced,
// duplicate numbers are dropped (first occurrence wins) and the line is
// rewritten in the single-line form "<prefix>dependencies: #N, #M".
//
// A dependencies line with no placeholder is returned unchanged, and
// "#{...}" on any other line is never touched. It returns ErrUnknownPlaceholder
// for a key missing from numbers and ErrBadDependencyLine for a malformed part
// on a line that has a placeholder.
func SubstituteDependencies(body string, numbers map[string]int) (string, error) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		out, err := substituteLine(line, numbers)
		if err != nil {
			return "", fmt.Errorf("line %d: %w", i+1, err)
		}
		lines[i] = out
	}
	return strings.Join(lines, "\n"), nil
}

// substituteLine rewrites one line (without its "\n") or returns it as is.
func substituteLine(line string, numbers map[string]int) (string, error) {
	loc := dependenciesKeywordRe.FindStringIndex(line)
	if loc == nil {
		return line, nil
	}
	prefix, rest := line[:loc[1]], line[loc[1]:]
	if !placeholderAnywhereRe.MatchString(rest) {
		return line, nil
	}

	// Keep a trailing carriage return out of the parts and put it back.
	cr := ""
	if strings.HasSuffix(rest, "\r") {
		cr, rest = "\r", strings.TrimSuffix(rest, "\r")
	}

	seen := make(map[int]bool)
	var refs []string
	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		var n int
		if m := placeholderPartRe.FindStringSubmatch(part); m != nil {
			v, ok := numbers[m[1]]
			if !ok {
				return "", fmt.Errorf("%w: %q", ErrUnknownPlaceholder, m[1])
			}
			n = v
		} else if m := numberPartRe.FindStringSubmatch(part); m != nil {
			v, err := strconv.Atoi(m[1])
			if err != nil {
				return "", fmt.Errorf("%w: reference %q is out of range", ErrBadDependencyLine, part)
			}
			n = v
		} else {
			return "", fmt.Errorf("%w: part %q is neither #<number> nor #{key}", ErrBadDependencyLine, part)
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		refs = append(refs, "#"+strconv.Itoa(n))
	}
	return prefix + " " + strings.Join(refs, ", ") + cr, nil
}
