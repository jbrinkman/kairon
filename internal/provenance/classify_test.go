package provenance

import (
	"strings"
	"testing"
	"time"
)

func TestState_String(t *testing.T) {
	cases := map[State]string{StateValid: "valid", StateNone: "none", StateMismatch: "mismatch"}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", int(s), got, want)
		}
	}
	if StateInvalid != 0 {
		t.Error("StateInvalid must be the zero value")
	}
}

func TestClassify_ThreeStates(t *testing.T) {
	body := "# Issue\n\nDo the thing.\n"
	marked := Mark(body, fixedTime)
	cases := []struct {
		name, in string
		want     State
	}{
		{"unmarked", body, StateNone},
		{"empty", "", StateNone},
		{"marked", marked, StateValid},
		{"marked with CRLF endings", strings.ReplaceAll(marked, "\n", "\r\n"), StateValid},
		{"marked with trailing newline", marked + "\n", StateValid},
		{"marked with trailing blank lines", marked + "\n\n  \n", StateValid},
		{"marked with other timestamp", Mark(body, fixedTime.Add(time.Hour)), StateValid},
		{"marked then edited", strings.Replace(marked, "Do the thing.", "Do another thing.", 1), StateMismatch},
		{"marked then appended text", marked + "\nextra", StateMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.in); got != c.want {
				t.Errorf("Classify = %v, want %v", got, c.want)
			}
		})
	}
}

func TestClassify_MalformedMarkerIsMismatch(t *testing.T) {
	body := "# Issue\n\ntext"
	h := Hash(body)
	const ts = "2026-03-04T05:06:07Z"
	valid := "<!-- planner-provenance: v1 sha256=" + h + " generated=" + ts + " -->"
	cases := []struct {
		name, in string
	}{
		{"hex too short", body + "\n<!-- planner-provenance: v1 sha256=" + h[:63] + " generated=" + ts + " -->"},
		{"hex too long", body + "\n<!-- planner-provenance: v1 sha256=" + h + "0 generated=" + ts + " -->"},
		{"uppercase hex", body + "\n<!-- planner-provenance: v1 sha256=" + strings.ToUpper(h) + " generated=" + ts + " -->"},
		{"non-hex characters", body + "\n<!-- planner-provenance: v1 sha256=" + strings.Repeat("g", 64) + " generated=" + ts + " -->"},
		{"unknown version v2", body + "\n<!-- planner-provenance: v2 sha256=" + h + " generated=" + ts + " -->"},
		{"missing version", body + "\n<!-- planner-provenance: sha256=" + h + " generated=" + ts + " -->"},
		{"missing generated", body + "\n<!-- planner-provenance: v1 sha256=" + h + " -->"},
		{"unparsable timestamp", body + "\n<!-- planner-provenance: v1 sha256=" + h + " generated=yesterday -->"},
		{"date only timestamp", body + "\n<!-- planner-provenance: v1 sha256=" + h + " generated=2026-03-04 -->"},
		{"missing closing", body + "\n<!-- planner-provenance: v1 sha256=" + h + " generated=" + ts},
		{"extra field", body + "\n<!-- planner-provenance: v1 sha256=" + h + " generated=" + ts + " extra=1 -->"},
		{"bare prefix", body + "\n<!-- planner-provenance:"},
		{"duplicate marker", body + "\n" + valid + "\n" + valid},
		{"duplicate marker with different timestamp", body + "\n" + valid + "\n<!-- planner-provenance: v1 sha256=" + h + " generated=2027-01-01T00:00:00Z -->"},
		{"marker not last", body + "\n" + valid + "\nmore text"},
		{"marker first", valid + "\n" + body},
		{"wrong hash", body + "\n<!-- planner-provenance: v1 sha256=" + strings.Repeat("0", 64) + " generated=" + ts + " -->"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.in)
			if got != StateMismatch {
				t.Errorf("Classify = %v, want mismatch", got)
			}
		})
	}
	// Guard the fixture: the unmodified well-formed marker is valid.
	if got := Classify(body + "\n" + valid); got != StateValid {
		t.Fatalf("fixture marker classified %v, want valid", got)
	}
}

func TestClassify_ClassifyAgreesWithMarkRoundTrip(t *testing.T) {
	for _, b := range []string{"", "x", "a\n\nb  \n", "```\ncode\n```\n"} {
		if got := Classify(Mark(b, fixedTime)); got != StateValid {
			t.Errorf("Classify(Mark(%q)) = %v, want valid", b, got)
		}
	}
}
