package failure

import "time"

// Comment is an issue comment as the failure contract sees it.
type Comment struct {
	Body      string
	CreatedAt time.Time
	ByViewer  bool // authored by the authenticated user
}

// IssueState is the GitHub state the failure contract decides from.
type IssueState struct {
	Labels             []string
	Comments           []Comment
	FailLabelRemovedAt time.Time // most recent removal of the fail-state label; zero if none or unknown
}

// Selection is how the watcher treats an issue.
type Selection int

// Selections.
const (
	SelectEligible Selection = iota
	SelectDone
	SelectFailed
	SelectTerminalPending
)

// Classify decides how the watcher treats the issue.
//
// Done and failed labels win. Otherwise the issue is terminal-pending when a
// marker comment authored by the viewer exists and the fail-state label has not
// been removed at or after the newest such marker. A tie counts as removed,
// because the label can only be applied after the comment. Markers not authored
// by the viewer, or not on the first line, are ignored.
func (s IssueState) Classify(doneLabel, failedLabel string) Selection {
	var hasDone, hasFailed bool
	for _, l := range s.Labels {
		switch l {
		case doneLabel:
			hasDone = true
		case failedLabel:
			hasFailed = true
		}
	}
	switch {
	case hasDone:
		return SelectDone
	case hasFailed:
		return SelectFailed
	}

	var newest time.Time
	var found bool
	for _, c := range s.Comments {
		if !c.ByViewer {
			continue
		}
		if _, ok := ParseMarker(c.Body); !ok {
			continue
		}
		if !found || c.CreatedAt.After(newest) {
			newest, found = c.CreatedAt, true
		}
	}
	if !found {
		return SelectEligible
	}
	if !s.FailLabelRemovedAt.IsZero() && !s.FailLabelRemovedAt.Before(newest) {
		return SelectEligible
	}
	return SelectTerminalPending
}

// Excluded reports whether the selection keeps the issue out of processing.
func (s Selection) Excluded() bool { return s != SelectEligible }

// Issues is the GitHub port. github.CLI implements it; tests fake it.
type Issues interface {
	// State returns labels and comments, and FailLabelRemovedAt only when needed.
	State(repo string, issue int, failLabel string) (IssueState, error)
	PostComment(repo string, issue int, body string) error
	AddLabel(repo string, issue int, label string) error
}

// Target identifies the issue and fail-state label a punch-out acts on.
type Target struct {
	Repo      string
	Issue     int
	FailLabel string
}
