// Package githubtest is an in-memory GitHub for tests that drive the watcher,
// the agent manager and the planner boundary together. One Fake satisfies
// every GitHub port those packages use, so a test sees the same issue through
// the boundary's read, the commit step's write and the failure contract's
// comment and label. Production code never imports it.
package githubtest

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jbrinkman/kairon/internal/boundary"
	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/github"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// Issue is one issue held by a Fake.
type Issue struct {
	Title    string
	Body     string
	Open     bool
	Labels   []string
	Comments []failure.Comment
	rev      int
	removed  map[string]time.Time // when each label was last removed
}

// Fake is an in-memory GitHub. Every mutation bumps the issue's revision, as
// GitHub's updatedAt does. It is safe for concurrent use.
type Fake struct {
	mu     sync.Mutex
	issues map[int]*Issue
	calls  []string
	// Fail makes the named operation (for example "Update" or "RemoveLabel")
	// return the error.
	Fail map[string]error
}

var (
	_ failure.Issues        = (*Fake)(nil)
	_ boundary.Reader       = (*Fake)(nil)
	_ provenance.IssueStore = (*Fake)(nil)
)

// New returns an empty Fake.
func New() *Fake { return &Fake{issues: map[int]*Issue{}, Fail: map[string]error{}} }

// Put stores an open issue with the given title, body and labels.
func (f *Fake) Put(number int, title, body string, labels ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues[number] = &Issue{Title: title, Body: body, Open: true, Labels: slices.Clone(labels), rev: 1}
}

// Close marks the issue closed.
func (f *Fake) Close(number int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.must(number).Open = false
	f.must(number).rev++
}

// Edit replaces the body as a human would, outside every Kairon port.
func (f *Fake) Edit(number int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.must(number).Body = body
	f.must(number).rev++
}

// Get returns a copy of the issue.
func (f *Fake) Get(number int) Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	is := *f.must(number)
	is.Labels = slices.Clone(is.Labels)
	is.Comments = slices.Clone(is.Comments)
	is.removed = nil
	return is
}

// Calls returns every recorded port call, in order, as "Op #N [arg]".
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Count returns how many recorded calls start with op.
func (f *Fake) Count(op string) int {
	n := 0
	for _, c := range f.Calls() {
		if len(c) >= len(op) && c[:len(op)] == op && (len(c) == len(op) || c[len(op)] == ' ') {
			n++
		}
	}
	return n
}

// Writes returns the recorded calls that change GitHub, in order.
func (f *Fake) Writes() []string {
	var w []string
	for _, c := range f.Calls() {
		for _, op := range []string{"Update", "Create", "PostComment", "AddLabel", "RemoveLabel"} {
			if len(c) >= len(op) && c[:len(op)] == op {
				w = append(w, c)
			}
		}
	}
	return w
}

// Select is the watcher's issue selection over the Fake: open issues carrying
// label, split into eligible and terminal-pending exactly as
// github.SelectIssues does. Issues come back in number order.
func (f *Fake) Select(label string) github.Selection {
	f.mu.Lock()
	defer f.mu.Unlock()
	var sel github.Selection
	numbers := make([]int, 0, len(f.issues))
	for n := range f.issues {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	for _, n := range numbers {
		is := f.issues[n]
		if !is.Open || !slices.Contains(is.Labels, label) {
			continue
		}
		gi := github.Issue{Number: n, Title: is.Title, Body: is.Body}
		for _, l := range is.Labels {
			gi.Labels = append(gi.Labels, github.Label{Name: l})
		}
		switch f.state(is, failure.FailedLabel(label)).Classify(failure.DoneLabel(label), failure.FailedLabel(label)) {
		case failure.SelectEligible:
			sel.Eligible = append(sel.Eligible, gi)
		case failure.SelectTerminalPending:
			sel.TerminalPending = append(sel.TerminalPending, gi)
		}
	}
	return sel
}

// Details is the dependency lookup: an issue's body and its OPEN or CLOSED state.
func (f *Fake) Details(_ string, number int) (*github.IssueDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	is, ok := f.issues[number]
	if !ok {
		return nil, fmt.Errorf("issue #%d not found", number)
	}
	state := "OPEN"
	if !is.Open {
		state = "CLOSED"
	}
	return &github.IssueDetails{Body: is.Body, State: state}, nil
}

// ReadIssue implements boundary.Reader.
func (f *Fake) ReadIssue(_ string, number int, failLabel string) (boundary.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("ReadIssue", number, ""); err != nil {
		return boundary.Snapshot{}, err
	}
	is, ok := f.issues[number]
	if !ok {
		return boundary.Snapshot{}, fmt.Errorf("issue #%d not found", number)
	}
	return boundary.Snapshot{Title: is.Title, Body: is.Body, Open: is.Open, State: f.state(is, failLabel)}, nil
}

// State implements failure.Issues.
func (f *Fake) State(_ string, number int, failLabel string) (failure.IssueState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("State", number, ""); err != nil {
		return failure.IssueState{}, err
	}
	return f.state(f.must(number), failLabel), nil
}

// PostComment implements failure.Issues. Comments are authored by the viewer.
func (f *Fake) PostComment(_ string, number int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("PostComment", number, ""); err != nil {
		return err
	}
	is := f.must(number)
	is.Comments = append(is.Comments, failure.Comment{Body: body, CreatedAt: time.Now(), ByViewer: true})
	is.rev++
	return nil
}

// AddLabel implements failure.Issues.
func (f *Fake) AddLabel(_ string, number int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("AddLabel", number, label); err != nil {
		return err
	}
	is := f.must(number)
	if !slices.Contains(is.Labels, label) {
		is.Labels = append(is.Labels, label)
	}
	is.rev++
	return nil
}

// RemoveLabel matches github.CLI.RemoveLabel.
func (f *Fake) RemoveLabel(_ string, number int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("RemoveLabel", number, label); err != nil {
		return err
	}
	is := f.must(number)
	if slices.Contains(is.Labels, label) {
		is.Labels = slices.DeleteFunc(is.Labels, func(l string) bool { return l == label })
		if is.removed == nil {
			is.removed = map[string]time.Time{}
		}
		at := time.Now()
		for _, c := range is.Comments {
			if !at.After(c.CreatedAt) {
				at = c.CreatedAt.Add(time.Nanosecond)
			}
		}
		is.removed[label] = at
	}
	is.rev++
	return nil
}

// Revision implements provenance.IssueStore.
func (f *Fake) Revision(_ context.Context, number int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("Revision", number, ""); err != nil {
		return "", err
	}
	is, ok := f.issues[number]
	if !ok {
		return "", fmt.Errorf("issue #%d not found", number)
	}
	return "rev-" + strconv.Itoa(is.rev), nil
}

// Create implements provenance.IssueStore.
func (f *Fake) Create(_ context.Context, title, body string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 1
	for k := range f.issues {
		if k >= n {
			n = k + 1
		}
	}
	if err := f.op("Create", n, ""); err != nil {
		return 0, err
	}
	f.issues[n] = &Issue{Title: title, Body: body, Open: true, rev: 1}
	return n, nil
}

// Update implements provenance.IssueStore.
func (f *Fake) Update(_ context.Context, number int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("Update", number, ""); err != nil {
		return err
	}
	is := f.must(number)
	is.Body = body
	is.rev++
	return nil
}

// op records a call and returns the configured failure for it. f.mu is held.
func (f *Fake) op(name string, number int, arg string) error {
	c := fmt.Sprintf("%s #%d", name, number)
	if arg != "" {
		c += " " + arg
	}
	f.calls = append(f.calls, c)
	return f.Fail[name]
}

// must returns the issue or panics: a test that names a missing issue is wrong.
func (f *Fake) must(number int) *Issue {
	is, ok := f.issues[number]
	if !ok {
		panic(fmt.Sprintf("githubtest: issue #%d not found", number))
	}
	return is
}

// state is the issue's failure-contract state. FailLabelRemovedAt is set the
// way github.CLI resolves it: only for failLabel, from its last removal.
func (f *Fake) state(is *Issue, failLabel string) failure.IssueState {
	return failure.IssueState{
		Labels:             slices.Clone(is.Labels),
		Comments:           slices.Clone(is.Comments),
		FailLabelRemovedAt: is.removed[failLabel],
	}
}
