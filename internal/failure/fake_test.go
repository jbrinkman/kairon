package failure

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

var errBoom = errors.New("boom")

// fakeIssues is a call-recording Issues. It reflects posted comments and added
// labels back through State, and keeps a logical clock so comment times and
// label-removal times order the way GitHub's server times would.
type fakeIssues struct {
	mu        sync.Mutex
	calls     []string
	labels    []string
	comments  []Comment
	removedAt time.Time
	clock     time.Time
	failures  map[string]int // op name -> number of upcoming calls that fail
}

func newFake() *fakeIssues {
	return &fakeIssues{
		clock:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		failures: map[string]int{},
	}
}

func (f *fakeIssues) tick() time.Time {
	f.clock = f.clock.Add(time.Second)
	return f.clock
}

// failNext makes the next n calls of op ("State", "PostComment", "AddLabel") fail.
func (f *fakeIssues) failNext(op string, n int) { f.failures[op] = n }

func (f *fakeIssues) record(op, detail string) error {
	if detail != "" {
		f.calls = append(f.calls, op+" "+detail)
	} else {
		f.calls = append(f.calls, op)
	}
	if f.failures[op] > 0 {
		f.failures[op]--
		return errBoom
	}
	return nil
}

func (f *fakeIssues) State(repo string, issue int, failLabel string) (IssueState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("State", ""); err != nil {
		return IssueState{}, err
	}
	return IssueState{
		Labels:             slices.Clone(f.labels),
		Comments:           slices.Clone(f.comments),
		FailLabelRemovedAt: f.removedAt,
	}, nil
}

func (f *fakeIssues) PostComment(repo string, issue int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("PostComment", ""); err != nil {
		return err
	}
	f.comments = append(f.comments, Comment{Body: body, CreatedAt: f.tick(), ByViewer: true})
	return nil
}

func (f *fakeIssues) AddLabel(repo string, issue int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("AddLabel", label); err != nil {
		return err
	}
	if !slices.Contains(f.labels, label) {
		f.labels = append(f.labels, label)
	}
	return nil
}

// seedLabel puts a label on the issue without recording a call.
func (f *fakeIssues) seedLabel(label string) { f.labels = append(f.labels, label) }

// seedComment adds a comment without recording a call.
func (f *fakeIssues) seedComment(body string, byViewer bool) {
	f.comments = append(f.comments, Comment{Body: body, CreatedAt: f.tick(), ByViewer: byViewer})
}

// clearLabel simulates a human removing a label: the label disappears and the
// removal time is recorded after every earlier comment.
func (f *fakeIssues) clearLabel(label string) {
	f.labels = slices.DeleteFunc(f.labels, func(l string) bool { return l == label })
	f.removedAt = f.tick()
}

// count returns how many recorded calls start with prefix.
func (f *fakeIssues) count(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// writes returns the recorded calls that change GitHub.
func (f *fakeIssues) writes() []string {
	var w []string
	for _, c := range f.calls {
		if !strings.HasPrefix(c, "State") {
			w = append(w, c)
		}
	}
	return w
}

// postedRunIDs returns the run id in the marker of every comment, in order.
func (f *fakeIssues) postedRunIDs() []string {
	var ids []string
	for _, c := range f.comments {
		if id, ok := ParseMarker(c.Body); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (f *fakeIssues) String() string { return fmt.Sprint(f.calls) }
