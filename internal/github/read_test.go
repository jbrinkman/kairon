package github

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/boundary"
	"github.com/jbrinkman/kairon/internal/failure"
)

var _ boundary.Reader = CLI{}

// readJSON builds the output of `gh issue view --json title,body,state,labels,comments`.
// body is JSON-encoded by the caller via %q-compatible escapes, so it is passed
// already quoted.
func readJSON(title, quotedBody, state string, labels []string, comments ...string) string {
	var ls []string
	for _, l := range labels {
		ls = append(ls, `{"name":"`+l+`"}`)
	}
	return `{"title":"` + title + `","body":` + quotedBody + `,"state":"` + state +
		`","labels":[` + strings.Join(ls, ",") + `],"comments":[` + strings.Join(comments, ",") + `]}`
}

func TestCLI_ReadIssue_ExactArgs(t *testing.T) {
	g := &fakeGH{view: map[int]string{9: readJSON("T", `"B"`, "OPEN", nil)}}
	if _, err := (CLI{run: g.run}).ReadIssue(testRepo, 9, "kairon-failed"); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 1 {
		t.Fatalf("gh calls = %d, want exactly 1: %v", len(g.calls), g.calls)
	}
	want := []string{"issue", "view", "9", "--repo", testRepo, "--json", "title,body,state,labels,comments"}
	got := g.calls[0].args
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}
	if g.calls[0].stdin != "" {
		t.Errorf("stdin = %q, want empty", g.calls[0].stdin)
	}
}

func TestCLI_ReadIssue_PopulatesSnapshot(t *testing.T) {
	g := &fakeGH{view: map[int]string{9: readJSON("My title", `"line1\r\nline2\r\n"`, "OPEN",
		[]string{"kairon", "bug"},
		plainComment("alice", "hello", "2026-01-01T10:00:00Z"))}}
	snap, err := CLI{run: g.run}.ReadIssue(testRepo, 9, "kairon-failed")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Title != "My title" {
		t.Errorf("Title = %q", snap.Title)
	}
	if snap.Body != "line1\r\nline2\r\n" {
		t.Errorf("Body = %q, want raw CRLF preserved", snap.Body)
	}
	if !snap.Open {
		t.Error("Open = false, want true for OPEN")
	}
	if len(snap.State.Labels) != 2 || snap.State.Labels[0] != "kairon" || snap.State.Labels[1] != "bug" {
		t.Errorf("Labels = %v", snap.State.Labels)
	}
	if len(snap.State.Comments) != 1 || snap.State.Comments[0].Body != "hello" || snap.State.Comments[0].ByViewer {
		t.Errorf("Comments = %+v", snap.State.Comments)
	}
	if n := g.count("api "); n != 0 {
		t.Errorf("api calls = %d, want 0 without a marker comment", n)
	}
}

func TestCLI_ReadIssue_OpenOnlyForStateOpen(t *testing.T) {
	cases := map[string]bool{"OPEN": true, "CLOSED": false, "open": false, "": false, "MERGED": false}
	for state, want := range cases {
		g := &fakeGH{view: map[int]string{9: readJSON("t", `"b"`, state, nil)}}
		snap, err := CLI{run: g.run}.ReadIssue(testRepo, 9, "kairon-failed")
		if err != nil {
			t.Fatalf("state %q: %v", state, err)
		}
		if snap.Open != want {
			t.Errorf("state %q: Open = %v, want %v", state, snap.Open, want)
		}
	}
}

func TestCLI_ReadIssue_ViewerAndEventsWhenViewerMarker(t *testing.T) {
	view := readJSON("t", `"b"`, "OPEN", []string{"kairon"},
		markerComment(testViewer, "run1", "2026-01-01T10:00:00Z"))
	g := &fakeGH{
		viewer: testViewer,
		view:   map[int]string{9: view},
		events: map[int]string{9: `{"label":"kairon-failed","at":"2026-01-01T12:00:00Z"}` + "\n"},
	}
	snap, err := CLI{run: g.run}.ReadIssue(testRepo, 9, "kairon-failed")
	if err != nil {
		t.Fatal(err)
	}
	if g.count("api user") != 1 || g.count("api repos/") != 1 {
		t.Errorf("viewer lookups = %d, events lookups = %d, want 1 and 1", g.count("api user"), g.count("api repos/"))
	}
	if !snap.State.FailLabelRemovedAt.Equal(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("FailLabelRemovedAt = %v", snap.State.FailLabelRemovedAt)
	}
	if !snap.State.Comments[0].ByViewer {
		t.Error("marker comment by the viewer must be ByViewer")
	}
	if got := snap.State.Classify("kairon-done", "kairon-failed"); got != failure.SelectEligible {
		t.Errorf("Classify = %v, want eligible (label cleared after marker)", got)
	}
}

func TestCLI_ReadIssue_NoEventsWithoutViewerMarker(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		view: map[int]string{9: readJSON("t", `"b"`, "OPEN", nil,
			markerComment("mallory", "r", "2026-01-01T10:00:00Z"))},
	}
	snap, err := CLI{run: g.run}.ReadIssue(testRepo, 9, "kairon-failed")
	if err != nil {
		t.Fatal(err)
	}
	if g.count("api user") != 1 {
		t.Errorf("viewer lookups = %d, want 1 (a marker comment exists)", g.count("api user"))
	}
	if g.count("api repos/") != 0 {
		t.Errorf("events lookups = %d, want 0 (marker not by viewer)", g.count("api repos/"))
	}
	if snap.State.Comments[0].ByViewer {
		t.Error("comment by another author must not be ByViewer")
	}
}

func TestCLI_ReadIssue_NoEventsWhenFailLabelPresent(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		view: map[int]string{9: readJSON("t", `"b"`, "OPEN", []string{"kairon-failed"},
			markerComment(testViewer, "r", "2026-01-01T10:00:00Z"))},
	}
	if _, err := (CLI{run: g.run}).ReadIssue(testRepo, 9, "kairon-failed"); err != nil {
		t.Fatal(err)
	}
	if n := g.count("api repos/"); n != 0 {
		t.Errorf("events lookups = %d, want 0 when the fail label is applied", n)
	}
}

func TestCLI_ReadIssue_ErrorsFailClosed(t *testing.T) {
	marker := markerComment(testViewer, "r", "2026-01-01T10:00:00Z")
	good := readJSON("t", `"b"`, "OPEN", nil, marker)

	t.Run("gh issue view error", func(t *testing.T) {
		run := func(string, ...string) ([]byte, error) { return nil, errors.New("boom") }
		snap, err := CLI{run: run}.ReadIssue(testRepo, 9, "kairon-failed")
		if err == nil {
			t.Fatal("want error")
		}
		if snap.Body != "" || snap.Open || snap.Title != "" || len(snap.State.Comments) != 0 {
			t.Errorf("partial snapshot returned: %+v", snap)
		}
	})
	t.Run("unparsable JSON", func(t *testing.T) {
		g := &fakeGH{view: map[int]string{9: `{"title": "x", "body": `}}
		snap, err := CLI{run: g.run}.ReadIssue(testRepo, 9, "kairon-failed")
		if err == nil {
			t.Fatal("want error")
		}
		if snap.Title != "" || snap.Body != "" || snap.Open {
			t.Errorf("partial snapshot returned: %+v", snap)
		}
	})
	t.Run("viewer lookup error", func(t *testing.T) {
		g := &fakeGH{view: map[int]string{9: good}, viewerErr: errors.New("no auth")}
		snap, err := CLI{run: g.run}.ReadIssue(testRepo, 9, "kairon-failed")
		if err == nil {
			t.Fatal("want error")
		}
		if snap.Body != "" || snap.Open || len(snap.State.Comments) != 0 {
			t.Errorf("partial snapshot returned: %+v", snap)
		}
	})
	t.Run("events lookup error", func(t *testing.T) {
		g := &fakeGH{view: map[int]string{9: good}, viewer: testViewer, eventsErr: map[int]error{9: errors.New("boom")}}
		snap, err := CLI{run: g.run}.ReadIssue(testRepo, 9, "kairon-failed")
		if err == nil {
			t.Fatal("want error")
		}
		if snap.Body != "" || snap.Open || len(snap.State.Comments) != 0 {
			t.Errorf("partial snapshot returned: %+v", snap)
		}
	})
}

func TestCLI_ReadIssue_RejectsInvalidArguments(t *testing.T) {
	g := &fakeGH{}
	if _, err := (CLI{run: g.run}).ReadIssue("", 9, "f"); err == nil {
		t.Error("want error for empty repo")
	}
	if _, err := (CLI{run: g.run}).ReadIssue(testRepo, 0, "f"); err == nil {
		t.Error("want error for issue 0")
	}
	if len(g.calls) != 0 {
		t.Errorf("gh called %d times for invalid arguments", len(g.calls))
	}
}
