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
// touches the file: both choices leave it in place. The full Expected and
// Actual text also goes to the console, which scrolls, because the dialog
// shortens them to fit the screen.
func (m model) openPlanningRejection(end planningEnd, fc guard.FailureContext, path string) model {
	pr := &planningRejection{end: end, failure: fc, path: path}
	for _, line := range pr.detailLines() {
		m = m.appendActivity(line)
	}
	m.planningRejection = pr
	m.activeOverlay = overlayPlanningRejection
	m.overlayContent = overlayContent{title: planningRejectionTitle, content: pr.lines(0)}
	return m
}

// detailLines are the Expected and Actual text, one entry per line.
func (pr *planningRejection) detailLines() []string {
	lines := labelledLines("Expected", pr.failure.Expected)
	return append(lines, labelledLines("Actual", pr.failure.Actual)...)
}

// lines renders the dialog in at most height lines; height 0 means no limit.
// The summary, the guard, the kept file and the keys always show. When the
// Expected and Actual text does not fit, it is cut short with a note that the
// full text is in the console. Blank spacer lines go first when space is tight.
func (pr *planningRejection) lines(height int) []string {
	summary := "The planning boundary rejected the document. Nothing was written to GitHub."
	guardLine := "Guard: " + pr.failure.Guard
	keptLine := "Kept file: " + pr.path
	keys := "R  Resume planning    D / Esc  Dismiss"

	header := []string{summary, "", guardLine}
	footer := []string{"", keptLine, "", keys}
	details := pr.detailLines()
	if height <= 0 {
		height = maxOverlayLines
	}
	room := height - len(header) - len(footer)
	if room < len(details) && room < 2 {
		header = []string{summary, guardLine}
		footer = []string{keptLine, keys}
		room = height - len(header) - len(footer)
	}
	if room < len(details) {
		keep := max(room-1, 0)
		details = append(details[:keep:keep],
			fmt.Sprintf("… %d more lines (full text in the console)", len(details)-keep))
	}

	out := append(header, details...)
	return append(out, footer...)
}

// labelledLines renders "Label: text". Multi-line text continues on indented
// lines.
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
