package planner

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
)

var testConflict = provenance.Conflict{Number: 42, Observed: "rev-1", Current: "rev-2"}

func TestDefaultRouterRouteToIntakeLogsAndWritesNothing(t *testing.T) {
	issues := newFakeIssues()
	var log bytes.Buffer
	r := &DefaultRouter{Issues: issues, Repo: testRepo, FailLabel: testLabel, RunID: testRunID, Log: &log}

	if err := r.RouteToIntake(context.Background(), testConflict); err != nil {
		t.Fatalf("RouteToIntake() error: %v", err)
	}
	containsAll(t, log.String(), "#42", "rev-1", "rev-2")
	if len(issues.calls) != 0 {
		t.Errorf("RouteToIntake touched the issue: %v", issues.calls)
	}
}

func TestDefaultRouterRouteToIntakeWithNilLogIsSilent(t *testing.T) {
	r := &DefaultRouter{}
	if err := r.RouteToIntake(context.Background(), testConflict); err != nil {
		t.Fatalf("RouteToIntake() error: %v", err)
	}
}

func TestDefaultRouterPunchOutPostsCommentThenLabel(t *testing.T) {
	issues := newFakeIssues()
	r := &DefaultRouter{Issues: issues, Repo: testRepo, FailLabel: testLabel, RunID: testRunID}

	if err := r.PunchOut(context.Background(), testConflict); err != nil {
		t.Fatalf("PunchOut() error: %v", err)
	}
	w := issues.writes()
	if len(w) != 2 || w[0] != "PostComment" || w[1] != "AddLabel "+testLabel {
		t.Fatalf("writes = %v, want comment then label", w)
	}
	containsAll(t, issues.bodies[0], "edited while being planned", "rev-1", "rev-2")
}

func TestDefaultRouterPunchOutNeedsATarget(t *testing.T) {
	issues := newFakeIssues()
	r := &DefaultRouter{Issues: issues}
	err := r.PunchOut(context.Background(), testConflict)
	if !errors.Is(err, guard.ErrInvalid) {
		t.Fatalf("PunchOut() error = %v, want guard.ErrInvalid", err)
	}
	if w := issues.writes(); len(w) != 0 {
		t.Errorf("wrote without a target: %v", w)
	}
}

func TestDefaultRouterPunchOutUsesConflictIssue(t *testing.T) {
	var gotIssue int
	r := &DefaultRouter{Issues: recordIssue{&gotIssue}, Repo: testRepo, FailLabel: testLabel, RunID: testRunID}
	if err := r.PunchOut(context.Background(), testConflict); err != nil {
		t.Fatalf("PunchOut() error: %v", err)
	}
	if gotIssue != 42 {
		t.Errorf("punched out issue %d, want 42", gotIssue)
	}
}

func TestDefaultRouterIsTheBoundaryRouterForAConflict(t *testing.T) {
	h := newHarness()
	var log bytes.Buffer
	h.b.Router = &DefaultRouter{Issues: h.issues, Repo: testRepo, FailLabel: testLabel, RunID: testRunID, Log: &log}
	h.store.seed(42, "rev-2-edited-by-human", "human edit")
	reply := automatedCanonical(t, 42, canonicalBody("Add retry", "none"))
	res, err := h.b.Handle(context.Background(), h.automated(42, reply))
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if res.Outcome != OutcomeRoutedToIntake {
		t.Fatalf("outcome = %v, want routed to intake", res.Outcome)
	}
	if !strings.Contains(log.String(), "#42") {
		t.Errorf("log = %q, want it to name #42", log.String())
	}
	if h.store.bodies[42] != "human edit" {
		t.Errorf("human edit overwritten: %q", h.store.bodies[42])
	}
}

// recordIssue is a failure.Issues that records which issue was addressed.
type recordIssue struct{ issue *int }

func (r recordIssue) State(_ string, issue int, _ string) (is failure.IssueState, err error) {
	*r.issue = issue
	return is, nil
}
func (r recordIssue) PostComment(string, int, string) error { return nil }
func (r recordIssue) AddLabel(string, int, string) error    { return nil }
