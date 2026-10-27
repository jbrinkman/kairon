package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/guard"
)

// planningEnd is what a front end hands over when a planning session ends.
type planningEnd struct {
	SessionID string
	// Resume reopens the session in the front end that ran it and gives the
	// agent the failure context. Each front end supplies its own.
	Resume func(m model, fc guard.FailureContext) (model, tea.Cmd)
}

// planningRejection is the state behind the planning-rejection overlay.
type planningRejection struct {
	end     planningEnd
	failure guard.FailureContext
	path    string
}

const planningRejectionTitle = "Planning document rejected"

// openPlanningRejection opens the rejection dialog for a document the
// planning boundary refused. path is the kept document file. The dialog never
// touches the file: both choices leave it in place.
func (m model) openPlanningRejection(end planningEnd, fc guard.FailureContext, path string) model {
	lines := []string{
		"The planning boundary rejected the document. Nothing was written to GitHub.",
		"",
		"Guard: " + fc.Guard,
	}
	lines = append(lines, labelledLines("Expected", fc.Expected)...)
	lines = append(lines, labelledLines("Actual", fc.Actual)...)
	lines = append(lines,
		"",
		"Kept file: "+path,
		"",
		"R  Resume planning    D / Esc  Dismiss",
	)
	if len(lines) > maxOverlayLines {
		lines = lines[len(lines)-maxOverlayLines:]
	}

	m.planningRejection = &planningRejection{end: end, failure: fc, path: path}
	m.activeOverlay = overlayPlanningRejection
	m.overlayContent = overlayContent{title: planningRejectionTitle, content: lines}
	return m
}

// labelledLines renders "Label: text". Multi-line text continues on indented
// lines; nothing is truncated.
func labelledLines(label, text string) []string {
	parts := strings.Split(text, "\n")
	out := []string{label + ": " + parts[0]}
	for _, p := range parts[1:] {
		out = append(out, "  "+p)
	}
	return out
}

// handlePlanningRejectionKey handles keys while the rejection dialog is open.
// The bool reports whether the key was consumed; unhandled keys fall through
// to the generic overlay rules, which swallow them.
func (m model) handlePlanningRejectionKey(key string) (model, tea.Cmd, bool) {
	if m.activeOverlay != overlayPlanningRejection || m.planningRejection == nil {
		return m, nil, false
	}
	switch key {
	case "d", "esc":
		m = m.closePlanningRejection()
		return m, nil, true
	case "r", "enter":
		pr := *m.planningRejection
		m = m.closePlanningRejection()
		if pr.end.Resume == nil {
			m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf(
				"Cannot resume planning session %s: its front end offered no way to resume. The document was kept at %s",
				pr.end.SessionID, pr.path)))
			return m, nil, true
		}
		m, cmd := pr.end.Resume(m, pr.failure)
		return m, cmd, true
	}
	return m, nil, false
}

func (m model) closePlanningRejection() model {
	m.planningRejection = nil
	return m.clearOverlay()
}
