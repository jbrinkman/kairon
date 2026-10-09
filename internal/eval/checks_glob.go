package eval

import (
	"fmt"
	"regexp"
	"strings"
)

// glob is a compiled path glob used by changed_files allow patterns.
//
// Dialect (matched against the whole slash-separated, repo-relative path):
//
//   - any run of characters except '/'
//     ?      exactly one character except '/'
//     [...]  one character from a class; ranges (a-z) and negation ([!...] or
//     [^...]) are supported; a class never matches '/'
//     **     any run of characters including '/'; as a whole segment
//     ("a/**/b", "**/x") it also matches zero directories
//     \c     the character c, literally
//
// A pattern without '/' therefore matches only a top-level path.
type glob struct {
	re *regexp.Regexp
}

// compileGlob validates and compiles pattern. It returns an error for an empty
// or absolute pattern, an unterminated or empty or invalid character class, and
// a trailing backslash.
func compileGlob(pattern string) (glob, error) {
	if pattern == "" {
		return glob{}, fmt.Errorf("invalid glob %q: empty pattern", pattern)
	}
	if pattern[0] == '/' {
		return glob{}, fmt.Errorf("invalid glob %q: patterns are repo-relative and must not start with '/'", pattern)
	}

	rs := []rune(pattern)
	var b strings.Builder
	b.WriteString(`(?s)\A`)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; r {
		case '\\':
			if i+1 >= len(rs) {
				return glob{}, fmt.Errorf("invalid glob %q: trailing backslash", pattern)
			}
			i++
			b.WriteString(regexp.QuoteMeta(string(rs[i])))
		case '?':
			b.WriteString(`[^/]`)
		case '*':
			j := i
			for j+1 < len(rs) && rs[j+1] == '*' {
				j++
			}
			if j == i {
				b.WriteString(`[^/]*`)
				break
			}
			// "**" as a whole segment followed by '/' also matches zero directories.
			if (i == 0 || rs[i-1] == '/') && j+1 < len(rs) && rs[j+1] == '/' {
				b.WriteString(`(?:.*/)?`)
				j++
			} else {
				b.WriteString(`.*`)
			}
			i = j
		case '[':
			end, class, err := compileGlobClass(rs, i)
			if err != nil {
				return glob{}, fmt.Errorf("invalid glob %q: %w", pattern, err)
			}
			b.WriteString(class)
			i = end
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString(`\z`)

	re, err := regexp.Compile(b.String())
	if err != nil { // defensive: the translation above should always be valid RE2
		return glob{}, fmt.Errorf("invalid glob %q: %w", pattern, err)
	}
	return glob{re: re}, nil
}

// compileGlobClass translates the character class starting at rs[start] ('[')
// into an RE2 class. It returns the index of the closing ']'.
func compileGlobClass(rs []rune, start int) (int, string, error) {
	i := start + 1
	negate := false
	if i < len(rs) && (rs[i] == '!' || rs[i] == '^') {
		negate = true
		i++
	}

	// next reads one (possibly escaped) class character.
	next := func() (rune, error) {
		if i >= len(rs) {
			return 0, fmt.Errorf("unterminated character class")
		}
		r := rs[i]
		i++
		if r == '\\' {
			if i >= len(rs) {
				return 0, fmt.Errorf("unterminated character class")
			}
			r = rs[i]
			i++
		}
		return r, nil
	}

	var b strings.Builder
	b.WriteByte('[')
	if negate {
		b.WriteByte('^')
	}
	items := 0
	for {
		if i >= len(rs) {
			return 0, "", fmt.Errorf("unterminated character class")
		}
		if rs[i] == ']' {
			break
		}
		lo, err := next()
		if err != nil {
			return 0, "", err
		}
		hi := lo
		// "x-y" is a range unless '-' is the last character before ']'.
		if i+1 < len(rs) && rs[i] == '-' && rs[i+1] != ']' {
			i++
			if hi, err = next(); err != nil {
				return 0, "", err
			}
			if hi < lo {
				return 0, "", fmt.Errorf("invalid character range %c-%c", lo, hi)
			}
		}
		if !negate && lo <= '/' && '/' <= hi {
			return 0, "", fmt.Errorf("character class must not match '/'")
		}
		if lo == hi {
			fmt.Fprintf(&b, `\x{%x}`, lo)
		} else {
			fmt.Fprintf(&b, `\x{%x}-\x{%x}`, lo, hi)
		}
		items++
	}
	if items == 0 {
		return 0, "", fmt.Errorf("empty character class")
	}
	if negate {
		b.WriteString(`\x{2f}`) // a negated class must still not match '/'
	}
	b.WriteByte(']')
	return i, b.String(), nil
}

// match reports whether path (slash-separated, repo-relative) matches the whole glob.
func (g glob) match(path string) bool {
	return g.re != nil && g.re.MatchString(path)
}

// globMatch compiles pattern and matches path against it. It returns an error
// for an invalid pattern.
func globMatch(pattern, path string) (bool, error) {
	g, err := compileGlob(pattern)
	if err != nil {
		return false, err
	}
	return g.match(path), nil
}
