package boundary

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/provenance"
)

var fixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

const (
	testRepo  = "owner/name"
	testIssue = 382
	testLabel = "kairon"
)

// callLog is a shared, ordered record of calls made by the fakes.
type callLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *callLog) add(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func (l *callLog) count(prefix string) int {
	n := 0
	for _, e := range l.all() {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

// fakeReader returns snapshots in sequence (the last one repeats) and logs
// every call, including the arguments it received.
type fakeReader struct {
	log       *callLog
	snapshots []Snapshot
	err       error
	calls     int
	gotRepo   string
	gotIssue  int
	gotFail   string
}

func (r *fakeReader) ReadIssue(repo string, issue int, failLabel string) (Snapshot, error) {
	r.calls++
	r.gotRepo, r.gotIssue, r.gotFail = repo, issue, failLabel
	if r.log != nil {
		r.log.add("ReadIssue")
	}
	if r.err != nil {
		return Snapshot{}, r.err
	}
	i := r.calls - 1
	if i >= len(r.snapshots) {
		i = len(r.snapshots) - 1
	}
	return r.snapshots[i], nil
}

// fakeIntake counts calls and returns status (IntakeStarted when unset) and err.
type fakeIntake struct {
	log    *callLog
	status IntakeStatus
	err    error
	calls  int
}

func (f *fakeIntake) Start(repo string, issue int) (IntakeStatus, error) {
	f.calls++
	if f.log != nil {
		f.log.add("Intake")
	}
	if f.err != nil {
		return 0, f.err
	}
	if f.status == 0 {
		return IntakeStarted, nil
	}
	return f.status, nil
}

// fakeStore is an in-memory provenance.IssueStore that records written bodies.
type fakeStore struct {
	bodies map[int]string
	next   int
}

func newFakeStore() *fakeStore { return &fakeStore{bodies: map[int]string{}, next: testIssue - 1} }

func (s *fakeStore) Revision(context.Context, int) (string, error) { return "rev", nil }

func (s *fakeStore) Create(_ context.Context, _ string, body string) (int, error) {
	s.next++
	s.bodies[s.next] = body
	return s.next, nil
}

func (s *fakeStore) Update(_ context.Context, n int, body string) error {
	s.bodies[n] = body
	return nil
}

var _ provenance.IssueStore = (*fakeStore)(nil)

// openSnapshot is an open, label-free issue with the given body.
func openSnapshot(body string) Snapshot {
	return Snapshot{Title: "Add the boundary", Body: body, Open: true}
}

// newBoundary wires a Boundary over r with the standard test label.
func newBoundary(r Reader, in Intake) Boundary {
	return Boundary{Reader: r, Intake: in, Label: testLabel}
}

// terminalPendingState is the state of an issue whose run-scoped punch-out
// comment exists (authored by the viewer) while the fail label has not been
// applied.
func terminalPendingState(commentAt time.Time) failure.IssueState {
	return failure.IssueState{
		Comments: []failure.Comment{{
			Body:      failure.Marker(failure.NewRunID()) + "\nKairon stopped.",
			CreatedAt: commentAt,
			ByViewer:  true,
		}},
	}
}
