package tui

import (
	"context"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/failure"
)

// validateDoneMsg carries the result of a validate request back to Update.
type validateDoneMsg struct {
	res agent.ValidateResult
}

// handleValidate starts `validate <issue>`: planner intake on one issue. The
// run is a managed intake run (its own log, agent tab and status entry); the
// returned command waits for it off the UI goroutine.
func (m model) handleValidate(args []string) (model, tea.Cmd) {
	issue := 0
	if len(args) == 1 {
		issue, _ = strconv.Atoi(args[0])
	}
	if issue <= 0 {
		m = m.appendActivity(m.styles.Error.Render("Usage: validate <issue-number>"))
		return m, nil
	}
	validate := m.validate
	if validate == nil {
		m = m.appendActivity(m.styles.Error.Render("Validate is unavailable: no agent manager"))
		return m, nil
	}
	m = m.appendActivity(m.styles.Activity.Render(fmt.Sprintf("Validating issue #%d with the automated planner...", issue)))
	return m, func() tea.Msg {
		return validateDoneMsg{res: validate(context.Background(), issue)}
	}
}

// validateLines renders a validate result as console lines.
func (m model) validateLines(res agent.ValidateResult) []string {
	failed := failure.FailedLabel(m.config.Label)
	n := res.Issue
	switch res.Status {
	case agent.ValidateAlreadyValid:
		if res.HadFailedLabel {
			return []string{m.styles.Warning.Render(fmt.Sprintf(
				"Issue #%d is already valid (sha256 %s); remove `%s` to restart it", n, shortHash(res.Hash), failed))}
		}
		return []string{m.styles.Success.Render(fmt.Sprintf("Issue #%d is already valid (sha256 %s)", n, shortHash(res.Hash)))}
	case agent.ValidateRefused:
		return []string{m.styles.Warning.Render(fmt.Sprintf("Not validating issue #%d: %s", n, res.Reason))}
	case agent.ValidateAlreadyRunning:
		return []string{m.styles.Warning.Render(fmt.Sprintf("Validation already running for #%d", n))}
	case agent.ValidateError:
		return []string{m.styles.Error.Render(fmt.Sprintf("Validate #%d failed: %v", n, res.Err))}
	}

	in := res.Intake
	switch in.Outcome {
	case agent.IntakeCommitted:
		lines := []string{m.styles.Success.Render(fmt.Sprintf(
			"Issue #%d validated and stamped (sha256 %s) after %d attempt(s)", n, shortHash(in.Hash), in.Attempts))}
		switch {
		case res.LabelErr != nil:
			lines = append(lines, m.styles.Error.Render(fmt.Sprintf(
				"Issue #%d is stamped, but removing `%s` failed: %v. Remove the label by hand.", n, failed, res.LabelErr)))
		case res.LabelRemoved:
			lines = append(lines, m.styles.Success.Render(fmt.Sprintf(
				"Removed `%s` from #%d; the watcher picks it up on its next poll", failed, n)))
		}
		return lines
	case agent.IntakePunchedOut:
		guard := "intake"
		if in.Failure != nil {
			guard = in.Failure.Guard
		}
		return []string{m.styles.Error.Render(fmt.Sprintf(
			"Validation punched out issue #%d (guard %s); see the comment on the issue", n, guard))}
	case agent.IntakeChanged:
		return []string{m.styles.Warning.Render(fmt.Sprintf(
			"Issue #%d was edited during validation; nothing was written. Run `validate %d` again.", n, n))}
	case agent.IntakeStopped:
		return []string{m.styles.Warning.Render(fmt.Sprintf("Validation of #%d was stopped; nothing was written", n))}
	}
	return []string{m.styles.Error.Render(fmt.Sprintf("Validation of #%d failed: %v", n, in.Err))}
}

// shortHash is the first 12 hex digits of a sha256, for console lines.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
