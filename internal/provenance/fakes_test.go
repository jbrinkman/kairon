package provenance

import (
	"context"
	"sync"
)

// fakeStore is an in-memory IssueStore that counts calls and records bodies.
type fakeStore struct {
	mu sync.Mutex

	revisions map[int]string // current revision per issue
	bodies    map[int]string // current body per issue
	titles    map[int]string
	nextNum   int

	revisionCalls int
	createCalls   int
	updateCalls   int

	// Optional injected failures.
	revisionErr error
	createErr   error
	updateErr   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		revisions: map[int]string{},
		bodies:    map[int]string{},
		titles:    map[int]string{},
		nextNum:   100,
	}
}

// seed adds an existing issue.
func (f *fakeStore) seed(number int, revision, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revisions[number] = revision
	f.bodies[number] = body
}

func (f *fakeStore) Revision(_ context.Context, number int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revisionCalls++
	if f.revisionErr != nil {
		return "", f.revisionErr
	}
	return f.revisions[number], nil
}

func (f *fakeStore) Create(_ context.Context, title, body string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.nextNum++
	f.bodies[f.nextNum] = body
	f.titles[f.nextNum] = title
	f.revisions[f.nextNum] = "rev-created"
	return f.nextNum, nil
}

func (f *fakeStore) Update(_ context.Context, number int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls++
	if f.updateErr != nil {
		return f.updateErr
	}
	f.bodies[number] = body
	return nil
}

// totalCalls is the number of store calls of any kind.
func (f *fakeStore) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.revisionCalls + f.createCalls + f.updateCalls
}

func (f *fakeStore) body(number int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[number]
}

// fakeRouter is a Router that counts calls and records the conflicts.
type fakeRouter struct {
	intakeCalls   int
	punchOutCalls int
	intakeErr     error
	punchOutErr   error
	conflicts     []Conflict
}

func (r *fakeRouter) RouteToIntake(_ context.Context, c Conflict) error {
	r.intakeCalls++
	r.conflicts = append(r.conflicts, c)
	return r.intakeErr
}

func (r *fakeRouter) PunchOut(_ context.Context, c Conflict) error {
	r.punchOutCalls++
	r.conflicts = append(r.conflicts, c)
	return r.punchOutErr
}
