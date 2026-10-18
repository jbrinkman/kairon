package issueprov

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	base       = "## Summary\n\nAdd a widget.\n\n## Acceptance Criteria\n1. It works."
	baseDigest = "ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13"
)

var (
	t1 = time.Date(2026, 10, 18, 19, 15, 34, 0, time.UTC)
	t2 = time.Date(2026, 10, 19, 8, 0, 1, 0, time.UTC)

	sumRe = regexp.MustCompile(`sha256=([0-9a-f]{64})`)
)

func markerCount(body string) int {
	n := 0
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "<!-- planner-provenance:") {
			n++
		}
	}
	return n
}

func withTrailingWS(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = l + " \t "
	}
	return strings.Join(lines, "\n")
}

// AC 1: digest of the base fixture and marker line format.
func TestHashBaseFixture(t *testing.T) {
	if got := Hash(base); got != baseDigest {
		t.Fatalf("Hash(base) = %s, want %s", got, baseDigest)
	}
}

func TestHashIsCanonicalizationInvariant(t *testing.T) {
	variants := map[string]string{
		"trailing newline":    base + "\n",
		"trailing blanks":     base + "\n\n\n",
		"crlf":                strings.ReplaceAll(base, "\n", "\r\n"),
		"lone cr":             strings.ReplaceAll(base, "\n", "\r"),
		"trailing whitespace": withTrailingWS(base),
	}
	for name, v := range variants {
		t.Run(name, func(t *testing.T) {
			if got := Hash(v); got != baseDigest {
				t.Errorf("Hash = %s, want %s", got, baseDigest)
			}
		})
	}
}

func TestCanonicalize(t *testing.T) {
	got := Canonicalize("a  \r\nb\t\r\n<!-- planner-provenance: junk -->\r\n\r\n")
	if want := "a\nb"; got != want {
		t.Errorf("Canonicalize = %q, want %q", got, want)
	}
	if got := Canonicalize(""); got != "" {
		t.Errorf("Canonicalize(\"\") = %q, want empty", got)
	}
}

func TestMarkerLineFormat(t *testing.T) {
	want := "<!-- planner-provenance: v1 sha256=" + baseDigest + " generated=2026-10-18T19:15:34Z -->"
	if got := MarkerLine(base, t1); got != want {
		t.Errorf("MarkerLine = %q, want %q", got, want)
	}
}

func TestMarkerLineRendersUTC(t *testing.T) {
	est := time.FixedZone("EST", -5*3600)
	local := time.Date(2026, 10, 18, 14, 15, 34, 0, est) // == t1
	if got, want := MarkerLine(base, local), MarkerLine(base, t1); got != want {
		t.Errorf("MarkerLine(non-UTC) = %q, want %q", got, want)
	}
}

// AC 2: three fixtures classify as none, valid, mismatch.
func TestClassifyFixtures(t *testing.T) {
	tests := []struct {
		name string
		body string
		want State
	}{
		{"unmarked", base, StateNone},
		{"marked", Append(base, t1), StateValid},
		{"marked then edited", strings.Replace(Append(base, t1), "widget", "widget extra", 1), StateMismatch},
		{"empty", "", StateNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.body); got != tt.want {
				t.Errorf("Classify = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateValues(t *testing.T) {
	if StateNone != "none" || StateValid != "valid" || StateMismatch != "mismatch" {
		t.Errorf("unexpected state values: %q %q %q", StateNone, StateValid, StateMismatch)
	}
	if Version != "v1" {
		t.Errorf("Version = %q, want v1", Version)
	}
}

// AC 3: Append then Classify is valid; marker is the last line, no trailing newline.
func TestAppendMarkerIsLastLine(t *testing.T) {
	got := Append(base, t1)
	if Classify(got) != StateValid {
		t.Fatalf("Classify(Append(base)) = %q, want valid", Classify(got))
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("Append result has trailing newline: %q", got)
	}
	lines := strings.Split(got, "\n")
	if last := lines[len(lines)-1]; last != MarkerLine(base, t1) {
		t.Errorf("last line = %q, want marker line", last)
	}
	if want := base + "\n\n" + MarkerLine(base, t1); got != want {
		t.Errorf("Append = %q, want %q", got, want)
	}
}

func TestAppendEmptyBody(t *testing.T) {
	got := Append("", t1)
	if want := MarkerLine("", t1); got != want {
		t.Errorf("Append(\"\") = %q, want %q", got, want)
	}
	if Classify(got) != StateValid {
		t.Errorf("Classify = %q, want valid", Classify(got))
	}
}

func TestAppendTrimsTrailingBlankLines(t *testing.T) {
	got := Append(base+"\n\n\n  \n", t1)
	if want := Append(base, t1); got != want {
		t.Errorf("Append = %q, want %q", got, want)
	}
}

// AC 4: whitespace and line-ending changes keep a marked body valid.
func TestClassifyToleratesWhitespaceAndLineEndings(t *testing.T) {
	marked := Append(base, t1)
	variants := map[string]string{
		"trailing whitespace": withTrailingWS(marked),
		"crlf":                strings.ReplaceAll(marked, "\n", "\r\n"),
		"trailing newline":    marked + "\n",
	}
	for name, v := range variants {
		t.Run(name, func(t *testing.T) {
			if got := Classify(v); got != StateValid {
				t.Errorf("Classify = %q, want valid", got)
			}
		})
	}
}

// AC 5: re-marking is idempotent on the hash and leaves a single marker.
func TestRemarkKeepsHashAndSingleMarker(t *testing.T) {
	once := Append(base, t1)
	twice := Append(once, t2)
	if once == twice {
		t.Fatal("re-marking with a different timestamp should change the marker")
	}
	a := sumRe.FindStringSubmatch(once)
	b := sumRe.FindStringSubmatch(twice)
	if a == nil || b == nil || a[1] != b[1] {
		t.Errorf("sha256 differs: %v vs %v", a, b)
	}
	if n := markerCount(twice); n != 1 {
		t.Errorf("marker lines = %d, want 1", n)
	}
	if Classify(twice) != StateValid {
		t.Errorf("Classify(twice) = %q, want valid", Classify(twice))
	}
	if !strings.HasSuffix(twice, "generated=2026-10-19T08:00:01Z -->") {
		t.Errorf("twice should carry the new timestamp: %q", twice)
	}
}

// AC 5 (replacement path): Append strips pre-existing markers of various
// shapes before adding a fresh one, leaving exactly one valid marker. This
// exercises Append directly, not only Classify, closing the gap where a
// regression in Append's marker-removal loop would go uncaught.
func TestAppendReplacesExistingMarkers(t *testing.T) {
	malformed := "<!-- planner-provenance: v1 sha256=abc -->"
	valid := MarkerLine(base, t1)
	tests := map[string]string{
		"malformed marker, LF":     base + "\n\n" + malformed,
		"valid + malformed, LF":    base + "\n\n" + valid + "\n" + malformed,
		"well-formed marker, CRLF": strings.ReplaceAll(base+"\n\n"+valid, "\n", "\r\n"),
		// Lone-CR (old-Mac) endings must be split the same way Classify splits
		// them. Append must normalize via splitLines, not strings.Split on "\n",
		// or it leaves the pre-existing marker in place and emits two markers.
		"well-formed marker, lone CR": strings.ReplaceAll(base+"\n\n"+valid, "\n", "\r"),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			got := Append(body, t2)
			if n := markerCount(got); n != 1 {
				t.Errorf("marker lines = %d, want 1\ngot: %q", n, got)
			}
			if Classify(got) != StateValid {
				t.Errorf("Classify = %q, want valid\ngot: %q", Classify(got), got)
			}
			if !strings.Contains(got, "Add a widget.") {
				t.Errorf("original content lost: %q", got)
			}
		})
	}
}

// AC 6: malformed markers classify as mismatch.
func TestClassifyMalformedMarkersAreMismatch(t *testing.T) {
	const ts = "generated=2026-10-18T19:15:34Z"
	upper := strings.ToUpper(baseDigest)
	markers := map[string]string{
		"truncated":           "<!-- planner-provenance: v1 sha256=abc -->",
		"short digest":        "<!-- planner-provenance: v1 sha256=" + baseDigest[:63] + " " + ts + " -->",
		"long digest":         "<!-- planner-provenance: v1 sha256=" + baseDigest + "0 " + ts + " -->",
		"non-hex digest":      "<!-- planner-provenance: v1 sha256=" + strings.Repeat("z", 64) + " " + ts + " -->",
		"uppercase digest":    "<!-- planner-provenance: v1 sha256=" + upper + " " + ts + " -->",
		"missing generated":   "<!-- planner-provenance: v1 sha256=" + baseDigest + " -->",
		"non-RFC3339":         "<!-- planner-provenance: v1 sha256=" + baseDigest + " generated=yesterday -->",
		"date only":           "<!-- planner-provenance: v1 sha256=" + baseDigest + " generated=2026-10-18 -->",
		"missing close":       "<!-- planner-provenance: v1 sha256=" + baseDigest + " " + ts,
		"corrupted version":   "<!-- planner-provenance: vX sha256=" + baseDigest + " " + ts + " -->",
		"missing version":     "<!-- planner-provenance: sha256=" + baseDigest + " " + ts + " -->",
		"prefix only":         "<!-- planner-provenance:",
		"trailing garbage":    "<!-- planner-provenance: v1 sha256=" + baseDigest + " " + ts + " --> x",
		"extra field":         "<!-- planner-provenance: v1 sha256=" + baseDigest + " " + ts + " extra=1 -->",
		"double space":        "<!-- planner-provenance: v1  sha256=" + baseDigest + " " + ts + " -->",
		"wrong digest, valid": "<!-- planner-provenance: v1 sha256=" + strings.Repeat("0", 64) + " " + ts + " -->",
	}
	for name, m := range markers {
		t.Run(name, func(t *testing.T) {
			body := base + "\n\n" + m
			if got := Classify(body); got != StateMismatch {
				t.Errorf("Classify = %q, want mismatch", got)
			}
		})
	}
}

func TestClassifyUnknownVersionIsMismatch(t *testing.T) {
	m := "<!-- planner-provenance: v2 sha256=" + baseDigest + " generated=2026-10-18T19:15:34Z -->"
	if got := Classify(base + "\n\n" + m); got != StateMismatch {
		t.Errorf("Classify(v2) = %q, want mismatch", got)
	}
}

func TestClassifyTwoMarkersIsMismatch(t *testing.T) {
	m := MarkerLine(base, t1)
	if got := Classify(base + "\n\n" + m + "\n" + m); got != StateMismatch {
		t.Errorf("Classify(two markers) = %q, want mismatch", got)
	}
	m2 := MarkerLine(base, t2)
	if got := Classify(base + "\n\n" + m + "\n" + m2); got != StateMismatch {
		t.Errorf("Classify(two different markers) = %q, want mismatch", got)
	}
}

func TestClassifyMarkerAtTopIsValid(t *testing.T) {
	// Moving the marker removes its line entirely. A blank line left behind
	// would be a leading blank line, which canonicalization keeps (only
	// trailing blank lines are dropped), so the marker goes directly above.
	body := MarkerLine(base, t1) + "\n" + base
	if got := Classify(body); got != StateValid {
		t.Errorf("Classify(marker at top) = %q, want valid", got)
	}
}

func TestClassifyEditedAfterMarkingIsMismatch(t *testing.T) {
	body := Append(base, t1) + "\n\nAppended note."
	if got := Classify(body); got != StateMismatch {
		t.Errorf("Classify(edited) = %q, want mismatch", got)
	}
}

func TestClassifyIndentedMarkerIsRecognized(t *testing.T) {
	body := base + "\n\n   " + MarkerLine(base, t1) + "  "
	if got := Classify(body); got != StateValid {
		t.Errorf("Classify(indented marker) = %q, want valid", got)
	}
}
