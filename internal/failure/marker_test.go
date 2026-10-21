package failure

import (
	"strings"
	"testing"
)

func TestNewRunID_ValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := NewRunID()
		if !ValidRunID(id) {
			t.Fatalf("NewRunID() = %q is not a valid run id", id)
		}
		if seen[id] {
			t.Fatalf("NewRunID() repeated %q", id)
		}
		seen[id] = true
	}
}

func TestValidRunID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"abc123", true},
		{"A.b_c-9", true},
		{"", false},
		{"has space", false},
		{"new\nline", false},
		{"semi;colon", false},
		{"-->", false},
		{"a>b", false},
		{"ünï", false},
		{strings.Repeat("a", 129), false},
	}
	for _, c := range cases {
		if got := ValidRunID(c.id); got != c.want {
			t.Errorf("ValidRunID(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestMarker_Format(t *testing.T) {
	if got, want := Marker("abc"), "<!-- kairon:punch-out run=abc -->"; got != want {
		t.Errorf("Marker = %q, want %q", got, want)
	}
}

func TestMarker_RoundTrip(t *testing.T) {
	for _, id := range []string{"abc", "A.b_c-9", NewRunID()} {
		got, ok := ParseMarker(Marker(id))
		if !ok || got != id {
			t.Errorf("ParseMarker(Marker(%q)) = %q, %v", id, got, ok)
		}
		// With a body after the marker line, LF and CRLF.
		for _, nl := range []string{"\n", "\r\n"} {
			got, ok = ParseMarker(Marker(id) + nl + "body text")
			if !ok || got != id {
				t.Errorf("ParseMarker with body (%q) = %q, %v", nl, got, ok)
			}
		}
	}
}

func TestParseMarker_RejectsNonFirstLine(t *testing.T) {
	m := Marker("abc")
	bodies := []string{
		"text\n" + m,
		"\n" + m,
		" " + m,
		"quote: " + m,
		"> " + m,
		"intro\n\n" + m + "\nmore",
	}
	for _, b := range bodies {
		if id, ok := ParseMarker(b); ok {
			t.Errorf("ParseMarker(%q) = %q, true; want rejection", b, id)
		}
	}
}

func TestParseMarker_RejectsMalformed(t *testing.T) {
	bodies := []string{
		"",
		"hello",
		"<!-- kairon:punch-out -->",
		"<!-- kairon:punch-out run= -->",
		"<!-- kairon:punch-out run=has space -->",
		"<!-- kairon:punch-out run=abc",
		"<!-- kairon:punch-out run=abc --> trailing",
		"<!--kairon:punch-out run=abc-->",
		"<!-- kairon:other run=abc -->",
		"<!-- kairon:punch-out run=" + strings.Repeat("a", 129) + " -->",
	}
	for _, b := range bodies {
		if id, ok := ParseMarker(b); ok {
			t.Errorf("ParseMarker(%q) = %q, true; want rejection", b, id)
		}
	}
}
