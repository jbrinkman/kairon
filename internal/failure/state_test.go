package failure

import (
	"testing"
	"time"
)

var (
	t0 = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	t1 = t0.Add(time.Minute)
	t2 = t0.Add(2 * time.Minute)
)

const (
	doneL   = "kairon-done"
	failedL = "kairon-failed"
)

func viewerMarker(runID string, at time.Time) Comment {
	return Comment{Body: Marker(runID) + "\nexplanation", CreatedAt: at, ByViewer: true}
}

func TestClassify_NoMarkerIsEligible(t *testing.T) {
	s := IssueState{Labels: []string{"kairon"}, Comments: []Comment{{Body: "hello", CreatedAt: t0, ByViewer: true}}}
	if got := s.Classify(doneL, failedL); got != SelectEligible {
		t.Errorf("Classify = %v, want SelectEligible", got)
	}
	if got := (IssueState{}).Classify(doneL, failedL); got != SelectEligible {
		t.Errorf("empty state Classify = %v, want SelectEligible", got)
	}
}

func TestClassify_TerminalPendingIsExcluded(t *testing.T) {
	s := IssueState{
		Labels:   []string{"kairon"},
		Comments: []Comment{viewerMarker("r1", t0)},
	}
	got := s.Classify(doneL, failedL)
	if got != SelectTerminalPending {
		t.Fatalf("Classify = %v, want SelectTerminalPending", got)
	}
	if !got.Excluded() {
		t.Error("TerminalPending must be excluded")
	}
}

func TestClassify_LabelRemovedBeforeMarkerStaysPending(t *testing.T) {
	// A removal older than the newest marker belongs to an earlier run.
	s := IssueState{
		Labels:             []string{"kairon"},
		Comments:           []Comment{viewerMarker("r1", t1)},
		FailLabelRemovedAt: t0,
	}
	if got := s.Classify(doneL, failedL); got != SelectTerminalPending {
		t.Errorf("Classify = %v, want SelectTerminalPending", got)
	}
}

func TestClassify_LabelClearedAfterMarkerIsEligible(t *testing.T) {
	for name, removed := range map[string]time.Time{"after": t2, "tie": t1} {
		s := IssueState{
			Labels:             []string{"kairon"},
			Comments:           []Comment{viewerMarker("r1", t1)},
			FailLabelRemovedAt: removed,
		}
		got := s.Classify(doneL, failedL)
		if got != SelectEligible {
			t.Errorf("%s: Classify = %v, want SelectEligible", name, got)
		}
		if got.Excluded() {
			t.Errorf("%s: Eligible must not be excluded", name)
		}
	}
}

func TestClassify_NewMarkerAfterRestartIsPending(t *testing.T) {
	// Run r1 punched out at t0, a human cleared the label at t1, run r2
	// punched out again at t2 and its label has not been removed since.
	s := IssueState{
		Labels:             []string{"kairon"},
		Comments:           []Comment{viewerMarker("r1", t0), viewerMarker("r2", t2)},
		FailLabelRemovedAt: t1,
	}
	if got := s.Classify(doneL, failedL); got != SelectTerminalPending {
		t.Errorf("Classify = %v, want SelectTerminalPending", got)
	}
}

func TestClassify_UsesNewestMarkerRegardlessOfOrder(t *testing.T) {
	s := IssueState{
		Labels:             []string{"kairon"},
		Comments:           []Comment{viewerMarker("r2", t2), viewerMarker("r1", t0)},
		FailLabelRemovedAt: t1,
	}
	if got := s.Classify(doneL, failedL); got != SelectTerminalPending {
		t.Errorf("Classify = %v, want SelectTerminalPending", got)
	}
}

func TestClassify_IgnoresMarkerNotByViewer(t *testing.T) {
	c := viewerMarker("r1", t0)
	c.ByViewer = false
	s := IssueState{Labels: []string{"kairon"}, Comments: []Comment{c}}
	if got := s.Classify(doneL, failedL); got != SelectEligible {
		t.Errorf("Classify = %v, want SelectEligible for a marker not by the viewer", got)
	}
}

func TestClassify_IgnoresViewerCommentWithMarkerNotOnFirstLine(t *testing.T) {
	s := IssueState{
		Labels:   []string{"kairon"},
		Comments: []Comment{{Body: "quoting:\n" + Marker("r1"), CreatedAt: t0, ByViewer: true}},
	}
	if got := s.Classify(doneL, failedL); got != SelectEligible {
		t.Errorf("Classify = %v, want SelectEligible", got)
	}
}

func TestClassify_UntrustedMarkerDoesNotHideTrustedOne(t *testing.T) {
	untrusted := viewerMarker("evil", t2)
	untrusted.ByViewer = false
	s := IssueState{
		Labels:   []string{"kairon"},
		Comments: []Comment{viewerMarker("r1", t0), untrusted},
	}
	if got := s.Classify(doneL, failedL); got != SelectTerminalPending {
		t.Errorf("Classify = %v, want SelectTerminalPending", got)
	}
}

func TestClassify_DoneAndFailed(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		want   Selection
	}{
		{"done", []string{"kairon", doneL}, SelectDone},
		{"failed", []string{"kairon", failedL}, SelectFailed},
		{"failed wins over pending", []string{failedL}, SelectFailed},
	}
	for _, c := range cases {
		s := IssueState{Labels: c.labels, Comments: []Comment{viewerMarker("r1", t0)}}
		got := s.Classify(doneL, failedL)
		if got != c.want {
			t.Errorf("%s: Classify = %v, want %v", c.name, got, c.want)
		}
		if !got.Excluded() {
			t.Errorf("%s: must be excluded", c.name)
		}
	}
	// Labels alone, with no comments.
	if got := (IssueState{Labels: []string{doneL}}).Classify(doneL, failedL); got != SelectDone {
		t.Errorf("done without comments = %v", got)
	}
}

func TestSelectionExcluded(t *testing.T) {
	want := map[Selection]bool{
		SelectEligible: false, SelectDone: true, SelectFailed: true, SelectTerminalPending: true,
	}
	for s, w := range want {
		if got := s.Excluded(); got != w {
			t.Errorf("Selection(%d).Excluded() = %v, want %v", s, got, w)
		}
	}
}
