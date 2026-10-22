package planner

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// fakeStore is an in-memory provenance.IssueStore that records every call.
type fakeStore struct {
	mu        sync.Mutex
	revisions map[int]string
	bodies    map[int]string
	titles    map[int]string
	nextNum   int
	calls     []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		revisions: map[int]string{},
		bodies:    map[int]string{},
		titles:    map[int]string{},
		nextNum:   400,
	}
}

func (f *fakeStore) seed(number int, revision, body string) {
	f.revisions[number] = revision
	f.bodies[number] = body
}

func (f *fakeStore) Revision(_ context.Context, number int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Revision")
	return f.revisions[number], nil
}

func (f *fakeStore) Create(_ context.Context, title, body string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Create")
	f.nextNum++
	f.titles[f.nextNum] = title
	f.bodies[f.nextNum] = body
	f.revisions[f.nextNum] = "rev-created"
	return f.nextNum, nil
}

func (f *fakeStore) Update(_ context.Context, number int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Update")
	f.bodies[number] = body
	return nil
}

func (f *fakeStore) count(op string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == op {
			n++
		}
	}
	return n
}

func (f *fakeStore) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// fakeRouter records conflicts routed by provenance.
type fakeRouter struct {
	intake, punchOut int
}

func (r *fakeRouter) RouteToIntake(_ context.Context, _ provenance.Conflict) error {
	r.intake++
	return nil
}

func (r *fakeRouter) PunchOut(_ context.Context, _ provenance.Conflict) error {
	r.punchOut++
	return nil
}

// fakeIssues is a failure.Issues that records the order of calls.
type fakeIssues struct {
	mu       sync.Mutex
	calls    []string
	labels   []string
	comments []failure.Comment
	bodies   []string
	clock    time.Time
}

func newFakeIssues() *fakeIssues {
	return &fakeIssues{clock: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (f *fakeIssues) State(_ string, _ int, _ string) (failure.IssueState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "State")
	return failure.IssueState{
		Labels:   slices.Clone(f.labels),
		Comments: slices.Clone(f.comments),
	}, nil
}

func (f *fakeIssues) PostComment(_ string, _ int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "PostComment")
	f.clock = f.clock.Add(time.Second)
	f.comments = append(f.comments, failure.Comment{Body: body, CreatedAt: f.clock, ByViewer: true})
	f.bodies = append(f.bodies, body)
	return nil
}

func (f *fakeIssues) AddLabel(_ string, _ int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "AddLabel "+label)
	f.labels = append(f.labels, label)
	return nil
}

// writes returns the recorded calls other than State reads.
func (f *fakeIssues) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var w []string
	for _, c := range f.calls {
		if c != "State" {
			w = append(w, c)
		}
	}
	return w
}

const testBodyTemplate = `# %s

## Problem Statement
The export has no retry, so one transient error loses the run.

## Context
Export code lives in internal/export/export.go:10.

## Scope
### In Scope
- Retry failed export requests
### Out of Scope
- Changing the export format

## Acceptance Criteria
1. A failed request is retried up to 3 times. Verify: ` + "`go test ./internal/export -run TestRetry`" + `

## Dependencies
Dependencies: %s
`

// canonicalBody returns a body that satisfies the default structural guard.
func canonicalBody(title, deps string) string {
	return fmt.Sprintf(testBodyTemplate, title, deps)
}

func marshalDoc(t *testing.T, d stageoutput.Document) string {
	t.Helper()
	raw, err := stageoutput.Marshal(&d)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	return string(raw)
}

const testGenerated = "2026-01-02T03:04:05Z"

// interactiveCanonical is a valid interactive canonical reply.
func interactiveCanonical(t *testing.T, body string) string {
	t.Helper()
	return marshalDoc(t, stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceDocument, Ref: "interactive-request"},
		Generated:     testGenerated,
		Body:          body,
	})
}

// automatedCanonical is a valid automated canonical reply for issue.
func automatedCanonical(t *testing.T, issue int, body string) string {
	t.Helper()
	return marshalDoc(t, stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: fmt.Sprint(issue)},
		Generated:     testGenerated,
		Body:          body,
	})
}

func punchOutDoc(t *testing.T, source stageoutput.Source, un []stageoutput.Unresolved) string {
	t.Helper()
	return marshalDoc(t, stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultPunchOut,
		Source:        source,
		Generated:     testGenerated,
		Unresolved:    un,
	})
}

func containsAll(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("expected %q to contain %q", haystack, n)
		}
	}
}
