package planner

import (
	"context"
	"fmt"
	"io"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

var _ provenance.Router = (*DefaultRouter)(nil)

// DefaultRouter is the provenance.Router the boundary uses when the caller
// has no other.
//
// RouteToIntake only logs. An issue edited while the planner worked on it no
// longer matches the planner's marker (or has none), so the next poll
// classifies it none or mismatch and plans it again; nothing has to be
// written for that to happen, and the human's edit is not lost.
//
// PunchOut is the fallback and goes to failure.PunchOut with an "edited while
// being planned" context, so the issue never gets a fail label without a
// reason. It needs Issues, Repo, FailLabel and RunID; without them it returns
// an error wrapping guard.ErrInvalid and writes nothing.
type DefaultRouter struct {
	// Issues is the GitHub port for a punch-out.
	Issues failure.Issues
	// Repo, FailLabel and RunID address the punch-out.
	Repo, FailLabel, RunID string
	// Log receives one line per routed conflict. Nil discards it.
	Log io.Writer
}

// RouteToIntake logs the conflict and returns nil.
func (r *DefaultRouter) RouteToIntake(_ context.Context, c provenance.Conflict) error {
	if r.Log != nil {
		fmt.Fprintf(r.Log, "issue #%d was edited while being planned (revision %s, now %s); it will be planned again on the next poll\n",
			c.Number, c.Observed, c.Current)
	}
	return nil
}

// PunchOut posts the failure comment, then the fail label.
func (r *DefaultRouter) PunchOut(_ context.Context, c provenance.Conflict) error {
	fc := guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    IntakeGuard,
		Attempt:  guard.Attempt{Current: 1, Max: 1},
		Expected: "the issue is unchanged between the planner reading it and the planner writing it",
		Actual: fmt.Sprintf("the issue was edited while being planned (revision %s when read, %s at commit) and could not be sent back for planning",
			c.Observed, c.Current),
	}
	target := failure.Target{Repo: r.Repo, Issue: c.Number, FailLabel: r.FailLabel}
	return failure.PunchOut(r.Issues, target, r.RunID, fc, "")
}
