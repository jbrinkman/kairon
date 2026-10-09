package eval

import "testing"

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// Literal / slash-less patterns match only a top-level path.
		{"literal top-level", "README.md", "README.md", true},
		{"literal not nested", "README.md", "docs/README.md", false},
		{"literal different file", "README.md", "README.txt", false},
		{"literal is whole-path", "README.md", "xREADME.md", false},
		{"literal is whole-path suffix", "README.md", "README.md.bak", false},
		{"literal dot is not wildcard", "a.md", "aXmd", false},

		// *
		{"star ext top-level", "*.md", "a.md", true},
		{"star ext not nested", "*.md", "docs/a.md", false},
		{"star matches empty", "*.md", ".md", true},
		{"star does not cross slash", "docs/*", "docs/a/b", false},
		{"star in dir segment", "docs/*/x.md", "docs/a/x.md", true},
		{"star in dir segment no cross", "docs/*/x.md", "docs/a/b/x.md", false},
		{"star one level", "docs/*", "docs/a.md", true},

		// ?
		{"question one char", "a?c", "abc", true},
		{"question not empty", "a?c", "ac", false},
		{"question not slash", "a?c", "a/c", false},
		{"question not two chars", "a?c", "abbc", false},
		{"question multibyte rune", "a?c", "a\u00e9c", true},

		// [..]
		{"class member", "[abc].go", "b.go", true},
		{"class non member", "[abc].go", "d.go", false},
		{"class range", "file[0-9].txt", "file7.txt", true},
		{"class range miss", "file[0-9].txt", "filex.txt", false},
		{"class negated bang", "[!a].go", "b.go", true},
		{"class negated bang miss", "[!a].go", "a.go", false},
		{"class negated caret", "[^a].go", "b.go", true},
		{"class negated caret miss", "[^a].go", "a.go", false},
		{"negated class never matches slash", "a[!x]b", "a/b", false},
		{"class literal dash first", "[-a].go", "-.go", true},
		{"class literal dash last", "[a-].go", "-.go", true},
		{"class escaped bracket", `[\]].go`, "].go", true},
		{"class holds special regex chars", "[.+].go", "+.go", true},
		{"class holds special regex chars miss", "[.+].go", "x.go", false},
		{"class bang not first is literal", "[a!].go", "!.go", true},

		// escapes
		{"escaped star is literal", `a\*b`, "a*b", true},
		{"escaped star not wildcard", `a\*b`, "axb", false},

		// **
		{"doublestar below dir", "docs/**", "docs/a.md", true},
		{"doublestar deep below dir", "docs/**", "docs/x/y/z.md", true},
		{"doublestar below dir other dir", "docs/**", "other/a.md", false},
		{"doublestar below dir prefix sibling", "docs/**", "docs2/a.md", false},
		{"doublestar below dir not top-level", "docs/**", "x/docs/a.md", false},
		{"doublestar any depth go top-level", "**/*.go", "main.go", true},
		{"doublestar any depth go one", "**/*.go", "cmd/main.go", true},
		{"doublestar any depth go deep", "**/*.go", "a/b/c/d.go", true},
		{"doublestar any depth go wrong ext", "**/*.go", "a/b/c/d.md", false},
		{"doublestar any depth go name suffix", "**/*.go", "a/b/xgo", false},
		{"doublestar mid zero dirs", "a/**/b", "a/b", true},
		{"doublestar mid many dirs", "a/**/b", "a/x/y/b", true},
		{"doublestar mid one dir", "a/**/b", "a/x/b", true},
		{"doublestar mid wrong tail", "a/**/b", "a/x/c", false},
		{"doublestar mid not segment-glued", "a/**/b", "a/xb", false},
		{"doublestar mid wrong head", "a/**/b", "z/a/x/b", false},
		{"doublestar alone matches all", "**", "a/b/c.txt", true},
		{"doublestar alone matches top-level", "**", "a.txt", true},
		{"doublestar prefix name", "src/**/test_*.go", "src/pkg/sub/test_x.go", true},
		{"doublestar prefix name zero dirs", "src/**/test_*.go", "src/test_x.go", true},
		{"doublestar prefix name miss", "src/**/test_*.go", "src/pkg/x.go", false},
		{"doublestar glued crosses slash", "a**b", "a/x/b", true},

		// Awkward but legal literal characters.
		{"regex metachars are literal", "a+b(c).md", "a+b(c).md", true},
		{"regex metachars are literal miss", "a+b(c).md", "aab(c).md", false},
		{"dollar caret are literal", "a$b^c", "a$b^c", true},
		{"newline in path", "docs/**", "docs/a\nb", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := globMatch(tt.pattern, tt.path)
			if err != nil {
				t.Fatalf("globMatch(%q, %q) unexpected error: %v", tt.pattern, tt.path, err)
			}
			if got != tt.want {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestGlobInvalidPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{"empty", ""},
		{"unterminated class", "[abc"},
		{"unterminated class in path", "docs/[a-z"},
		{"unterminated negated class", "[!abc"},
		{"empty class", "[]"},
		{"empty negated class", "[!]"},
		{"unterminated class after escape", `[\]`},
		{"reversed range", "[z-a]"},
		{"trailing backslash", `abc\`},
		{"absolute pattern", "/etc/passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := compileGlob(tt.pattern); err == nil {
				t.Fatalf("compileGlob(%q) returned nil error, want error", tt.pattern)
			}
			if _, err := globMatch(tt.pattern, "x"); err == nil {
				t.Fatalf("globMatch(%q, ...) returned nil error, want error", tt.pattern)
			}
		})
	}
}

func TestGlobCompiledReuse(t *testing.T) {
	g, err := compileGlob("**/*.go")
	if err != nil {
		t.Fatalf("compileGlob: %v", err)
	}
	for path, want := range map[string]bool{
		"main.go":      true,
		"a/b/main.go":  true,
		"a/b/main.txt": false,
	} {
		if got := g.match(path); got != want {
			t.Errorf("match(%q) = %v, want %v", path, got, want)
		}
	}
}
