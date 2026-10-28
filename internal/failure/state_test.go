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
	notByViewer := viewerMarker("r1", t0)
	notByViewer.ByViewer = false

	cases := []struct {
		name     string
		state    IssueState
		want     Selection
		excluded bool
	}{
		{
			name:     "done",
			state:    IssueState{Labels: []string{"kairon", doneL}, Comments: []Comment{viewerMarker("r1", t0)}},
			want:     SelectDone,
			excluded: true,
		},
		{
			name:     "failed",
			state:    IssueState{Labels: []string{"kairon", failedL}, Comments: []Comment{viewerMarker("r1", t0)}},
			want:     SelectFailed,
			excluded: true,
		},
		{
			name:     "failed wins over pending",
			state:    IssueState{Labels: []string{failedL}, Comments: []Comment{viewerMarker("r1", t0)}},
			want:     SelectFailed,
			excluded: true,
		},
		{
			// Labels alone, with no comments.
			name:     "done without comments",
			state:    IssueState{Labels: []string{doneL}},
			want:     SelectDone,
			excluded: true,
		},
		{
			name: "no marker",
			state: IssueState{
				Labels:   []string{"kairon"},
				Comments: []Comment{{Body: "hello", CreatedAt: t0, ByViewer: true}},
			},
			want:     SelectEligible,
			excluded: false,
		},
		{
			name:     "empty state",
			state:    IssueState{},
			want:     SelectEligible,
			excluded: false,
		},
		{
			// A removal older than the newest marker belongs to an earlier run.
			name: "label removed before marker stays pending",
			state: IssueState{
				Labels:             []string{"kairon"},
				Comments:           []Comment{viewerMarker("r1", t1)},
				FailLabelRemovedAt: t0,
			},
			want:     SelectTerminalPending,
			excluded: true,
		},
		{
			// Run r1 punched out at t0, a human cleared the label at t1, run r2
			// punched out again at t2 and its label has not been removed since.
			name: "new marker after restart is pending",
			state: IssueState{
				Labels:             []string{"kairon"},
				Comments:           []Comment{viewerMarker("r1", t0), viewerMarker("r2", t2)},
				FailLabelRemovedAt: t1,
			},
			want:     SelectTerminalPending,
			excluded: true,
		},
		{
			name: "newest marker regardless of order",
			state: IssueState{
				Labels:             []string{"kairon"},
				Comments:           []Comment{viewerMarker("r2", t2), viewerMarker("r1", t0)},
				FailLabelRemovedAt: t1,
			},
			want:     SelectTerminalPending,
			excluded: true,
		},
		{
			name:     "marker not by viewer is ignored",
			state:    IssueState{Labels: []string{"kairon"}, Comments: []Comment{notByViewer}},
			want:     SelectEligible,
			excluded: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.state.Classify(doneL, failedL)
			if got != c.want {
				t.Errorf("Classify = %v, want %v", got, c.want)
			}
			if got.Excluded() != c.excluded {
				t.Errorf("Excluded() = %v, want %v", got.Excluded(), c.excluded)
			}
		})
	}
}
