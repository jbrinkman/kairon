package provenance

import "testing"

func TestCanonicalize_RemovesMarkerNormalizesEndingsStripsTrailingWhitespace(t *testing.T) {
	marker := "<!-- planner-provenance: v1 sha256=abc generated=2026-01-02T03:04:05Z -->"
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"only newlines", "\n\n\r\n", ""},
		{"crlf", "a\r\nb\r\n", "a\nb"},
		{"lone cr", "a\rb\r", "a\nb"},
		{"trailing spaces and tabs", "a  \nb\t\n c \t \n", "a\nb\n c"},
		{"form feed and vertical tab", "a\f\v\nb", "a\nb"},
		{"trailing blank lines", "a\n\n\n", "a"},
		{"interior blank lines kept", "a\n\nb\n", "a\n\nb"},
		{"leading whitespace kept", "  a\n\tb", "  a\n\tb"},
		{"leading blank line kept", "\na", "\na"},
		{"marker removed", "a\n" + marker, "a"},
		{"marker removed with indentation", "a\n  " + marker + "  \n", "a"},
		{"marker removed from the middle", "a\n" + marker + "\nb", "a\nb"},
		{"every marker removed", "a\n" + marker + "\n" + marker + "\n", "a"},
		{"unknown version marker removed", "a\n<!-- planner-provenance: v2 foo -->\n", "a"},
		{"malformed marker removed", "a\n<!-- planner-provenance: garbage\n", "a"},
		{"marker prefix mid-line kept", "a <!-- planner-provenance: v1 -->", "a <!-- planner-provenance: v1 -->"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Canonicalize(c.in); got != c.want {
				t.Errorf("Canonicalize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCanonicalize_EquivalentBodiesMatch(t *testing.T) {
	base := "# Title\n\nSome text.\n\n- item\n"
	variants := []string{
		base,
		"# Title\r\n\r\nSome text.\r\n\r\n- item\r\n",
		"# Title\r\rSome text.\r\r- item\r",
		"# Title  \n\t\nSome text. \t\n\n- item   \n",
		base + "\n\n\n",
		"# Title\n\nSome text.\n\n- item\n<!-- planner-provenance: v1 sha256=00 generated=x -->",
		"# Title\n\nSome text.\n\n- item\n<!-- planner-provenance: v1 sha256=00 generated=x -->\r\n",
	}
	want := Canonicalize(base)
	if want != "# Title\n\nSome text.\n\n- item" {
		t.Fatalf("Canonicalize(base) = %q", want)
	}
	for i, v := range variants {
		got := Canonicalize(v)
		if got != want {
			t.Errorf("variant %d: Canonicalize = %q, want %q", i, got, want)
		}
		if again := Canonicalize(got); again != got {
			t.Errorf("variant %d: not idempotent: %q then %q", i, got, again)
		}
	}
}
