package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
	"time"
)

var markerRe = regexp.MustCompile(`^<!-- planner-provenance: v1 sha256=[0-9a-f]{64} generated=\S+ -->$`)

var fixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func TestHash_IsSHA256OfCanonicalBody(t *testing.T) {
	body := "hello\r\nworld  \n"
	sum := sha256.Sum256([]byte("hello\nworld"))
	want := hex.EncodeToString(sum[:])
	if got := Hash(body); got != want {
		t.Errorf("Hash = %q, want %q", got, want)
	}
}

func TestHash_ExcludesMarker(t *testing.T) {
	body := "# Issue\n\ntext"
	marked := Mark(body, fixedTime)
	if Hash(marked) != Hash(body) {
		t.Errorf("Hash(Mark(b)) = %q, want Hash(b) = %q", Hash(marked), Hash(body))
	}
	other := Mark(body, fixedTime.Add(48*time.Hour))
	if Hash(other) != Hash(body) {
		t.Error("generated timestamp must not affect the hash")
	}
}

func TestMarker_Format(t *testing.T) {
	h := strings.Repeat("ab", 32)
	got := Marker(h, fixedTime)
	want := "<!-- planner-provenance: v1 sha256=" + h + " generated=2026-03-04T05:06:07Z -->"
	if got != want {
		t.Errorf("Marker = %q, want %q", got, want)
	}
	if !markerRe.MatchString(got) {
		t.Errorf("Marker %q does not match the documented form", got)
	}
}

func TestMarker_ConvertsToUTC(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	got := Marker(strings.Repeat("0", 64), time.Date(2026, 3, 4, 0, 6, 7, 0, loc))
	if !strings.HasSuffix(got, "generated=2026-03-04T05:06:07Z -->") {
		t.Errorf("Marker = %q, want UTC timestamp", got)
	}
}

func TestMark_MarkerIsLastLineWithNoTrailingNewline(t *testing.T) {
	got := Mark("line one  \r\nline two\r\n\r\n", fixedTime)
	if strings.HasSuffix(got, "\n") {
		t.Errorf("Mark output has a trailing newline: %q", got)
	}
	lines := strings.Split(got, "\n")
	last := lines[len(lines)-1]
	if !markerRe.MatchString(last) {
		t.Errorf("last line %q does not match the marker form", last)
	}
	if last != Marker(Hash("line one\nline two"), fixedTime) {
		t.Errorf("last line = %q", last)
	}
	if rest := strings.Join(lines[:len(lines)-1], "\n"); rest != "line one\nline two" {
		t.Errorf("body before marker = %q", rest)
	}
}

func TestMark_MarkingTwiceYieldsSameHash(t *testing.T) {
	body := "# Issue\n\nDependencies: #1\n"
	once := Mark(body, fixedTime)
	twice := Mark(once, fixedTime.Add(time.Hour))
	hashOf := func(s string) string {
		lines := strings.Split(s, "\n")
		m := regexp.MustCompile(`sha256=([0-9a-f]{64})`).FindStringSubmatch(lines[len(lines)-1])
		if m == nil {
			t.Fatalf("no sha256 in last line of %q", s)
		}
		return m[1]
	}
	if hashOf(once) != hashOf(twice) {
		t.Errorf("sha256 changed on re-marking: %s vs %s", hashOf(once), hashOf(twice))
	}
	if hashOf(once) != Hash(body) {
		t.Errorf("marker sha256 = %s, want Hash(body) = %s", hashOf(once), Hash(body))
	}
	if n := strings.Count(twice, MarkerPrefix); n != 1 {
		t.Errorf("re-marked body has %d marker lines, want 1", n)
	}
	if Canonicalize(twice) != Canonicalize(body) {
		t.Error("Canonicalize(Mark(b)) != Canonicalize(b)")
	}
}

func TestMark_EmptyBodyReturnsMarkerAlone(t *testing.T) {
	got := Mark("\n  \n", fixedTime)
	if got != Marker(Hash(""), fixedTime) {
		t.Errorf("Mark(empty) = %q, want the marker alone", got)
	}
}
