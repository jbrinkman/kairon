package eval

import (
	"errors"
	"fmt"
	"strings"
)

// ErrThresholdFailed is the sentinel wrapped by *ThresholdError. Callers test
// for it with errors.Is to tell "an evaluated agent scored below its pass
// threshold" apart from a run that could not be completed.
var ErrThresholdFailed = errors.New("evaluation failed its pass threshold")

// ThresholdError is returned by a run (after all results are written) when one
// or more evaluated agents fail their pass threshold. It satisfies
// errors.Is(err, ErrThresholdFailed) and names the failing agents.
type ThresholdError struct {
	// Agents are the failing agents, in evaluation order.
	Agents []string
}

// Error returns a one-line message naming the failing agents.
func (e *ThresholdError) Error() string {
	return fmt.Sprintf("%s: %s", ErrThresholdFailed.Error(), strings.Join(e.Agents, ", "))
}

// Unwrap lets errors.Is(err, ErrThresholdFailed) match.
func (e *ThresholdError) Unwrap() error { return ErrThresholdFailed }

// formatVerdictLine renders the one-line verdict for an agent, always starting
// with "PASS " or "FAIL ", for example:
//
//	PASS selftest: 100.0% (threshold 95.0%, 0/5 cases failed)
func formatVerdictLine(agent string, v AgentVerdict) string {
	status := "FAIL"
	if v.Passed {
		status = "PASS"
	}
	return fmt.Sprintf("%s %s: %.1f%% (threshold %.1f%%, %d/%d cases failed)",
		status, agent, v.Score, v.Threshold, v.CasesFailed, v.CasesTotal)
}
