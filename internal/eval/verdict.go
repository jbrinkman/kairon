package eval

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// AgentVerdict is the pass/fail judgement of one evaluated agent. Score and
// Threshold are percent (0-100); the legacy summary agent_scores is a 0-1
// fraction and is unchanged.
type AgentVerdict struct {
	Score       float64  `json:"score"`
	Threshold   float64  `json:"threshold"`
	Passed      bool     `json:"passed"`
	CasesTotal  int      `json:"cases_total"`
	CasesFailed int      `json:"cases_failed"`
	Cost        CostInfo `json:"cost"`
}

// agentThreshold returns the pass threshold recorded on the agent result,
// falling back to DefaultPassThreshold for a result that predates verdicts.
func agentThreshold(ar AgentResult) float64 {
	if ar.Threshold > 0 {
		return ar.Threshold
	}
	return DefaultPassThreshold
}

// percentAtLeast reports whether score/max, as a percent, is at least
// threshold. It multiplies before dividing so that an exact boundary (19/20 at
// 95) is not lost to float drift. A zero denominator never passes.
func percentAtLeast(score, max int, threshold float64) bool {
	return max > 0 && float64(score)*100/float64(max) >= threshold
}

// casePassed reports whether a case met its threshold. A stamped Passed wins;
// a legacy case (Passed == nil) is recomputed against the agent's threshold.
func casePassed(c CaseResult, agentThreshold float64) bool {
	if c.Passed != nil {
		return *c.Passed
	}
	score, max := caseTotals(c)
	return percentAtLeast(score, max, agentThreshold)
}

// stampCase records the case's threshold and whether it passed. A case with no
// output cannot pass.
func stampCase(cr *CaseResult, tc TestCase, rubric Rubric) {
	threshold := caseThreshold(tc, rubric)
	score, max := caseTotals(*cr)
	passed := cr.ActualOutput != "" && percentAtLeast(score, max, threshold)
	cr.Threshold = threshold
	cr.Passed = &passed
}

// agentVerdict computes the verdict of an agent from its result alone. The
// agent passes when its aggregate score (over every non-cost criterion of every
// case) is at least its threshold. With nothing measured it fails closed.
// CasesFailed is informational and does not by itself fail the agent.
func agentVerdict(ar AgentResult) AgentVerdict {
	threshold := agentThreshold(ar)
	v := AgentVerdict{
		Threshold:  threshold,
		CasesTotal: len(ar.Cases),
		Cost:       agentCostTotals(ar),
	}
	for _, c := range ar.Cases {
		if !casePassed(c, threshold) {
			v.CasesFailed++
		}
	}

	score, max := agentScoreTotals(ar)
	if max > 0 {
		v.Score = score * 100 / max
		v.Passed = score*100/max >= threshold
	}
	return v
}

// agentCostTotals returns the agent's cost: the agent and judge calls of every
// case. Model and UsageSource are deliberately not aggregated.
func agentCostTotals(ar AgentResult) CostInfo {
	var cost CostInfo
	for _, c := range ar.Cases {
		cost.TokensIn += c.AgentCost.TokensIn + c.JudgeCost.TokensIn
		cost.TokensOut += c.AgentCost.TokensOut + c.JudgeCost.TokensOut
		cost.EstimatedUSD += c.AgentCost.EstimatedUSD + c.JudgeCost.EstimatedUSD
	}
	return cost
}

// setAgentVerdict records the agent's verdict in the summary and recomputes
// TotalCost as the sum of every verdict's cost, in sorted agent order so the
// float sum is deterministic. Recomputing (rather than adding) keeps repeated
// progressive saves of the same agent idempotent.
func setAgentVerdict(s *Summary, ar AgentResult) {
	if s.AgentVerdicts == nil {
		s.AgentVerdicts = make(map[string]AgentVerdict)
	}
	s.AgentVerdicts[ar.Agent] = agentVerdict(ar)

	var total CostInfo
	for _, agent := range sortedVerdictAgents(s.AgentVerdicts) {
		c := s.AgentVerdicts[agent].Cost
		total.TokensIn += c.TokensIn
		total.TokensOut += c.TokensOut
		total.EstimatedUSD += c.EstimatedUSD
	}
	s.TotalCost = total
}

func sortedVerdictAgents(m map[string]AgentVerdict) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ErrThresholdFailed is the sentinel wrapped by ThresholdError.
var ErrThresholdFailed = errors.New("eval failed")

// AgentFailure describes one agent that finished below its pass threshold.
type AgentFailure struct {
	Agent     string
	Score     float64
	Threshold float64
}

// ThresholdError is returned when one or more evaluated agents fall below
// their pass threshold.
type ThresholdError struct {
	Failed []AgentFailure
	Total  int
}

func (e *ThresholdError) Error() string {
	parts := make([]string, 0, len(e.Failed))
	for _, f := range e.Failed {
		parts = append(parts, fmt.Sprintf("%s (%.1f%% < %.1f%%)", f.Agent, f.Score, f.Threshold))
	}
	return fmt.Sprintf("%s: %d of %d agents below their pass threshold: %s",
		ErrThresholdFailed, len(e.Failed), e.Total, strings.Join(parts, ", "))
}

func (e *ThresholdError) Unwrap() error { return ErrThresholdFailed }

// printAgentVerdict writes the single PASS/FAIL line for an agent. The line
// starts with the verdict word so it can be matched with grep '^PASS \|^FAIL '.
func printAgentVerdict(w io.Writer, agent string, v AgentVerdict) {
	word := "FAIL"
	if v.Passed {
		word = "PASS"
	}
	fmt.Fprintf(w, "%s %s: %.1f%% (threshold %.1f%%), %d/%d cases failed\n",
		word, agent, v.Score, v.Threshold, v.CasesFailed, v.CasesTotal)
}

// thresholdFailure returns a *ThresholdError for the failed agents, or nil when
// none failed. total is the number of agents evaluated.
func thresholdFailure(failed []AgentFailure, total int) error {
	if len(failed) == 0 {
		return nil
	}
	return &ThresholdError{Failed: failed, Total: total}
}
