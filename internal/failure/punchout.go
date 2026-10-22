package failure

import (
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/guard"
)

// PunchOut is Tier 2: it tells a human why the pipeline stopped, then removes
// the issue from the polling set by applying the fail-state label.
//
// The order is fixed. The explanatory comment is written first, and the label
// only after the comment is in place, so an issue is never labeled without a
// stated reason. PunchOut is idempotent for one runID: it reads the issue
// first, skips the comment when a comment authored by the authenticated user
// already carries Marker(runID), and skips the label when it is present. A
// retry after a partial failure therefore never posts a second comment. A new
// runID (a new terminal run after a human restart) posts its own comment.
//
// It returns nil only when both the comment and the label are in place. Invalid
// input returns an error wrapping guard.ErrInvalid and writes nothing.
func PunchOut(iss Issues, t Target, runID string, fc guard.FailureContext, remedy string) error {
	if err := validateTarget(iss, t); err != nil {
		return err
	}
	if !ValidRunID(runID) {
		return fmt.Errorf("%w: run id %q is not valid", guard.ErrInvalid, runID)
	}
	if err := fc.Validate(); err != nil {
		return err
	}

	st, err := iss.State(t.Repo, t.Issue, t.FailLabel)
	if err != nil {
		return fmt.Errorf("read issue state: %w", err)
	}

	if !hasViewerMarker(st, runID) {
		body := RenderComment(fc, runID, remedy, t.FailLabel)
		if err := iss.PostComment(t.Repo, t.Issue, body); err != nil {
			// Do not touch the label: a label without a reason is the failure
			// mode this contract exists to prevent.
			return fmt.Errorf("post punch-out comment: %w", err)
		}
	}

	if hasLabel(st, t.FailLabel) {
		return nil
	}
	if err := iss.AddLabel(t.Repo, t.Issue, t.FailLabel); err != nil {
		return fmt.Errorf("add label %q: %w", t.FailLabel, err)
	}
	return nil
}

// Reconcile finishes a half-done punch-out. When the issue is terminal-pending
// (a trusted marker comment exists and the fail-state label is missing) it
// applies only the label and reports applied. In every other state it writes
// nothing.
func Reconcile(iss Issues, t Target) (applied bool, err error) {
	if err := validateTarget(iss, t); err != nil {
		return false, err
	}
	st, err := iss.State(t.Repo, t.Issue, t.FailLabel)
	if err != nil {
		return false, fmt.Errorf("read issue state: %w", err)
	}
	if st.Classify(t.doneLabel(), t.FailLabel) != SelectTerminalPending {
		return false, nil
	}
	if err := iss.AddLabel(t.Repo, t.Issue, t.FailLabel); err != nil {
		return false, fmt.Errorf("add label %q: %w", t.FailLabel, err)
	}
	return true, nil
}

// validateTarget checks the port and the target. The error wraps guard.ErrInvalid.
func validateTarget(iss Issues, t Target) error {
	var p []string
	if iss == nil {
		p = append(p, "issues port is nil")
	}
	if t.Repo == "" {
		p = append(p, "repo is empty")
	}
	if t.Issue <= 0 {
		p = append(p, fmt.Sprintf("issue %d is not positive", t.Issue))
	}
	if t.FailLabel == "" {
		p = append(p, "fail label is empty")
	}
	if len(p) > 0 {
		return fmt.Errorf("%w: target: %s", guard.ErrInvalid, strings.Join(p, "; "))
	}
	return nil
}

// doneLabel derives the done-state label that pairs with the fail-state label.
// FailedLabel(trigger) is trigger+"-failed" and DoneLabel(trigger) is
// trigger+"-done", so the trigger is recovered by trimming the suffix.
func (t Target) doneLabel() string {
	return DoneLabel(strings.TrimSuffix(t.FailLabel, "-failed"))
}

// hasViewerMarker reports whether a comment authored by the authenticated user
// carries Marker(runID) as its first line. Comments from anyone else are
// untrusted and ignored.
func hasViewerMarker(st IssueState, runID string) bool {
	for _, c := range st.Comments {
		if !c.ByViewer {
			continue
		}
		if id, ok := ParseMarker(c.Body); ok && id == runID {
			return true
		}
	}
	return false
}

func hasLabel(st IssueState, label string) bool {
	for _, l := range st.Labels {
		if l == label {
			return true
		}
	}
	return false
}
