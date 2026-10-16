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
//
// With repeats, Score is the mean over the repeats and the verdict is decided
// on it; ScoreMin, ScoreMax and RepeatScores show the spread. They are not
// omitempty: a legitimate score_min of 0 must be written.
type AgentVerdict struct {
	Score        float64   `json:"score"`
	Threshold    float64   `json:"threshold"`
	Passed       bool      `json:"passed"`
	CasesTotal   int       `json:"cases_total"`
	CasesFailed  int       `json:"cases_failed"`
	Cost         CostInfo  `json:"cost"`
	Repeat       int       `json:"repeat"`
	ScoreMin     float64   `json:"score_min"`
	ScoreMax     float64   `json:"score_max"`
	RepeatScores []float64 `json:"repeat_scores"`
	// Criteria is every non-cost criterion scored in any case, pooled over
	// every case and repeat, in first-seen order. Always written ([] when
	// nothing is scored).
	Criteria []CriterionTotal `json:"criteria"`
	// MinCriterionScore echoes the rubric's per-criterion floor (percent);
	// absent when the rubric sets none.
	MinCriterionScore *float64 `json:"min_criterion_score,omitempty"`
	// CriteriaBelowFloor names the criteria whose pooled percent is below
	// MinCriterionScore, in Criteria order; absent when there are none. Any
	// entry fails the agent.
	CriteriaBelowFloor []string `json:"criteria_below_floor,omitempty"`
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
//
// When the result carries a per-criterion floor (MinCriterionScore) the agent
// also fails if any criterion's pooled percent is below it, even when the
// score meets the threshold. The score itself is never changed by the floor.
//
// With repeats (CaseResult.Run) the aggregate score is the mean over the
// repeats, computed as the pooled sum of scores over the sum of maxima (every
// repeat has the same denominator, so this is the arithmetic mean of the
// per-repeat scores while keeping the multiply-before-divide boundary
// behaviour). ScoreMin, ScoreMax and RepeatScores describe the per-repeat
// spread; the verdict is decided on the mean, so a best repeat above the
// threshold does not rescue a mean below it. A legacy result (Repeat 0, Run 0)
// reads as a single repeat.
func agentVerdict(ar AgentResult) AgentVerdict {
	threshold := agentThreshold(ar)
	v := AgentVerdict{
		Threshold:    threshold,
		Cost:         agentCostTotals(ar),
		Repeat:       max(ar.Repeat, 1),
		RepeatScores: []float64{},
		Criteria:     []CriterionTotal{},
	}
	if criteria := agentCriterionTotals(ar); len(criteria) > 0 {
		v.Criteria = criteria
	}
	if ar.MinCriterionScore != nil {
		floor := *ar.MinCriterionScore
		v.MinCriterionScore = &floor
		for _, c := range v.Criteria {
			if !percentAtLeast(c.Score, c.MaxScore, floor) {
				v.CriteriaBelowFloor = append(v.CriteriaBelowFloor, c.Name)
			}
		}
	}
	v.CasesTotal, v.CasesFailed = caseAccounting(ar, threshold)

	score, maxScore := agentScoreTotals(ar)
	if maxScore > 0 {
		v.Score = score * 100 / maxScore
		v.Passed = score*100/maxScore >= threshold && len(v.CriteriaBelowFloor) == 0
	}

	for i, rt := range repeatTotals(ar) {
		var pct float64
		if rt.Max > 0 {
			pct = rt.Score * 100 / rt.Max
		}
		v.RepeatScores = append(v.RepeatScores, pct)
		if i == 0 || pct < v.ScoreMin {
			v.ScoreMin = pct
		}
		if i == 0 || pct > v.ScoreMax {
			v.ScoreMax = pct
		}
	}
	if maxScore <= 0 {
		// Nothing scored: fail closed with the whole spread at 0.
		v.ScoreMin, v.ScoreMax = 0, 0
		for i := range v.RepeatScores {
			v.RepeatScores[i] = 0
		}
	}
	if len(v.RepeatScores) == 0 {
		v.RepeatScores = append(v.RepeatScores, v.Score)
		v.ScoreMin, v.ScoreMax = v.Score, v.Score
	}
	return v
}

// caseAccounting returns the number of distinct cases and how many of them
// failed. A case with a single result fails per casePassed (a stamped Passed
// wins, a legacy case is recomputed). A case with several results (repeats)
// fails when its pooled percent over those results is below the case's
// threshold: the CaseResult.Threshold of a stamped case (authoritative,
// including an explicit 0), else the agent's.
func caseAccounting(ar AgentResult, agentThreshold float64) (total, failed int) {
	groups := map[string][]CaseResult{}
	var order []string
	for _, c := range ar.Cases {
		if _, ok := groups[c.CaseName]; !ok {
			order = append(order, c.CaseName)
		}
		groups[c.CaseName] = append(groups[c.CaseName], c)
	}
	for _, name := range order {
		g := groups[name]
		if len(g) == 1 {
			if !casePassed(g[0], agentThreshold) {
				failed++
			}
			continue
		}
		threshold := agentThreshold
		for _, c := range g {
			// A stamped case (Passed != nil) carries an authoritative
			// Threshold, including an explicit 0 (min_score: 0); an unstamped
			// legacy case has Threshold 0 meaning "unset" and falls back to
			// the agent threshold. Guarding on Threshold > 0 would miscount a
			// case with an explicit zero threshold against the agent's.
			if c.Passed != nil {
				threshold = c.Threshold
				break
			}
		}
		var score, maxScore int
		for _, c := range g {
			cs, cm := caseTotals(c)
			score += cs
			maxScore += cm
		}
		if !percentAtLeast(score, maxScore, threshold) {
			failed++
		}
	}
	return len(order), failed
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
	// Floor and BelowFloor are set when criteria fell below the rubric's
	// min_criterion_score floor.
	Floor      float64
	BelowFloor []CriterionTotal
}

// failureOf builds the AgentFailure of an agent whose verdict did not pass,
// carrying the criteria below the floor when there are any.
func failureOf(agent string, v AgentVerdict) AgentFailure {
	f := AgentFailure{Agent: agent, Score: v.Score, Threshold: v.Threshold}
	if len(v.CriteriaBelowFloor) == 0 || v.MinCriterionScore == nil {
		return f
	}
	f.Floor = *v.MinCriterionScore
	for _, c := range v.Criteria {
		if !percentAtLeast(c.Score, c.MaxScore, f.Floor) {
			f.BelowFloor = append(f.BelowFloor, c)
		}
	}
	return f
}

// floorDetail renders criteria as "a 10.0% (1/10), b 0.0% (0/1)".
func floorDetail(criteria []CriterionTotal) string {
	parts := make([]string, 0, len(criteria))
	for _, c := range criteria {
		parts = append(parts, fmt.Sprintf("%s %.1f%% (%d/%d)", c.Name, c.Percent, c.Score, c.MaxScore))
	}
	return strings.Join(parts, ", ")
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
		if len(f.BelowFloor) > 0 {
			// Name the criteria instead of a score < threshold comparison: the
			// overall score may have met its threshold.
			names := make([]string, 0, len(f.BelowFloor))
			for _, c := range f.BelowFloor {
				names = append(names, fmt.Sprintf("%s %.1f%%", c.Name, c.Percent))
			}
			noun := "criterion"
			if len(names) > 1 {
				noun = "criteria"
			}
			parts = append(parts, fmt.Sprintf("%s (%s %s < floor %.1f%%)", f.Agent, noun, strings.Join(names, ", "), f.Floor))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%.1f%% < %.1f%%)", f.Agent, f.Score, f.Threshold))
	}
	return fmt.Sprintf("%s: %d of %d agents below their pass threshold: %s",
		ErrThresholdFailed, len(e.Failed), e.Total, strings.Join(parts, ", "))
}

func (e *ThresholdError) Unwrap() error { return ErrThresholdFailed }

// printAgentVerdict writes the single PASS/FAIL line for an agent. The line
// starts with the verdict word so it can be matched with grep '^PASS \|^FAIL '.
// When criteria are below the floor, they are named after the existing text; with
// repeats (Repeat > 1) the spread is appended last.
func printAgentVerdict(w io.Writer, agent string, v AgentVerdict) {
	word := "FAIL"
	if v.Passed {
		word = "PASS"
	}
	suffix := ""
	if len(v.CriteriaBelowFloor) > 0 && v.MinCriterionScore != nil {
		var below []CriterionTotal
		for _, c := range v.Criteria {
			if !percentAtLeast(c.Score, c.MaxScore, *v.MinCriterionScore) {
				below = append(below, c)
			}
		}
		noun := "criterion"
		if len(below) > 1 {
			noun = "criteria"
		}
		suffix = fmt.Sprintf("; %s below floor %.1f%%: %s", noun, *v.MinCriterionScore, floorDetail(below))
	}
	if v.Repeat > 1 {
		suffix += fmt.Sprintf(" [repeat %d: min %.1f%%, max %.1f%%]", v.Repeat, v.ScoreMin, v.ScoreMax)
	}
	fmt.Fprintf(w, "%s %s: %.1f%% (threshold %.1f%%), %d/%d cases failed%s\n",
		word, agent, v.Score, v.Threshold, v.CasesFailed, v.CasesTotal, suffix)
	for _, c := range v.Criteria {
		fmt.Fprintf(w, "  %s: %d/%d (%.1f%%)\n", c.Name, c.Score, c.MaxScore, c.Percent)
	}
}

// thresholdFailure returns a *ThresholdError for the failed agents, or nil when
// none failed. total is the number of agents evaluated.
func thresholdFailure(failed []AgentFailure, total int) error {
	if len(failed) == 0 {
		return nil
	}
	return &ThresholdError{Failed: failed, Total: total}
}
