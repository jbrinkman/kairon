package eval

import (
	"fmt"
	"io"
)

// scoreCase runs every non-cost rubric criterion against cr.ActualOutput and
// appends the CriterionScores to cr.Scores, adding LLM-judge cost to cr.JudgeCost.
// cr.Scores stays nil when the rubric has no non-cost criteria.
func scoreCase(rubric Rubric, tc TestCase, cr *CaseResult) {
	for _, criterion := range rubric.Criteria {
		if criterion.Type == "cost" {
			continue // cost is tracked separately
		}

		score := CriterionScore{
			Name:          criterion.Name,
			MaxScore:      parseMaxScore(criterion.Scoring),
			Deterministic: criterion.Deterministic,
		}

		if criterion.Deterministic {
			score.Score, score.Reasoning, score.Skipped = scoreDeterministic(criterion, tc, cr.ActualOutput, cr.WorkspaceDir, cr.containerWorkspaceDir)
		} else {
			// LLM-judged criteria
			if cr.ActualOutput == "" {
				score.Score = 0
				score.Skipped = true
				score.Reasoning = "no output available for LLM judging"
			} else {
				judgeCost, judgeScore, reasoning, skipped, rec := scoreLLMJudge(criterion, tc, cr.ActualOutput)
				cr.Calls = append(cr.Calls, rec)
				cr.JudgeCost.Add(judgeCost)
				score.Score = judgeScore
				score.Reasoning = reasoning
				score.Skipped = skipped
			}
		}

		cr.Scores = append(cr.Scores, score)
	}
}

// caseTotals returns the summed score and maximum score of a case's criteria.
// Every criterion counts, skipped or not: a skipped (errored, unscorable or
// no-output) criterion carries a Score of 0 and its MaxScore stays in the
// denominator, so a failure can never raise a score. A criterion whose
// MaxScore was never set adds nothing. This is the single place the per-case
// score arithmetic lives.
func caseTotals(cr CaseResult) (score, max int) {
	for _, s := range cr.Scores {
		score += s.Score
		max += s.MaxScore
	}
	return score, max
}

// agentScoreTotals returns the summed score and maximum score across every
// case of an agent, as floats, using caseTotals. The agent score is
// score/max when max > 0.
func agentScoreTotals(ar AgentResult) (score, max float64) {
	for _, c := range ar.Cases {
		cs, cm := caseTotals(c)
		score += float64(cs)
		max += float64(cm)
	}
	return score, max
}

// scorePercent returns score as a percent of max (0-100). It multiplies
// before dividing so that, for example, 19 of 20 is exactly 95 and meets a 95
// threshold under >=. It returns 0 when max is not positive; callers treat
// max <= 0 as a failure (see meetsThreshold).
func scorePercent(score, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return score * 100 / max
}

// meetsThreshold reports whether score of max reaches threshold percent.
// Nothing scorable (max <= 0) never meets a threshold.
func meetsThreshold(score, max, threshold float64) bool {
	return max > 0 && scorePercent(score, max) >= threshold
}

// floatRef returns a pointer to a copy of f.
func floatRef(f float64) *float64 { return &f }

// agentThreshold returns the agent's recorded threshold, or the default for a
// result written before thresholds were recorded.
func agentThreshold(ar AgentResult) float64 {
	if ar.Threshold != nil {
		return *ar.Threshold
	}
	return DefaultPassThreshold
}

// caseThreshold returns the case's recorded threshold, falling back to the
// agent threshold (legacy results).
func caseThreshold(cr CaseResult, agent float64) float64 {
	if cr.Threshold != nil {
		return *cr.Threshold
	}
	return agent
}

// agentVerdict derives the pass/fail verdict of an agent from its result
// alone. Passed depends on the aggregate score only; CasesFailed counts the
// cases below their own threshold (a case with nothing scorable counts as
// failed) and is informational.
func agentVerdict(ar AgentResult) AgentVerdict {
	threshold := agentThreshold(ar)
	score, max := agentScoreTotals(ar)

	v := AgentVerdict{
		Score:      scorePercent(score, max),
		Threshold:  threshold,
		Passed:     meetsThreshold(score, max, threshold),
		CasesTotal: len(ar.Cases),
	}
	for _, c := range ar.Cases {
		cs, cm := caseTotals(c)
		if !meetsThreshold(float64(cs), float64(cm), caseThreshold(c, threshold)) {
			v.CasesFailed++
		}
		v.Cost.TokensIn += c.AgentCost.TokensIn + c.JudgeCost.TokensIn
		v.Cost.TokensOut += c.AgentCost.TokensOut + c.JudgeCost.TokensOut
		v.Cost.EstimatedUSD += c.AgentCost.EstimatedUSD + c.JudgeCost.EstimatedUSD
	}
	return v
}

// printCaseResult writes the final status line for a scored case:
// "no output" / "no scored criteria" / pass (✅) / warn (⚠️, pct>=60) / fail (❌),
// plus the breakdown of criteria scoring below 3/4 of max when pct < threshold.
// The threshold is the one recorded on the result (see executeCase), else the
// case's min_score, else DefaultPassThreshold.
func printCaseResult(out io.Writer, tc TestCase, cr CaseResult) {
	if cr.ActualOutput != "" {
		totalScore, maxTotal := caseTotals(cr)
		if maxTotal == 0 {
			fmt.Fprintf(out, " ⚠️  no scored criteria\n")
		} else {
			pct := scorePercent(float64(totalScore), float64(maxTotal))
			threshold := DefaultPassThreshold
			switch {
			case cr.Threshold != nil:
				threshold = *cr.Threshold
			case tc.MinScore != nil:
				threshold = *tc.MinScore
			}

			if pct >= threshold {
				fmt.Fprintf(out, " ✅ %.0f%% (threshold: %.0f%%)\n", pct, threshold)
			} else if pct >= 60 {
				fmt.Fprintf(out, " ⚠️  %.0f%% (threshold: %.0f%%)\n", pct, threshold)
			} else {
				fmt.Fprintf(out, " ❌ %.0f%% (threshold: %.0f%%)\n", pct, threshold)
			}

			// Show criterion breakdown for scores below threshold
			if pct < threshold {
				for _, s := range cr.Scores {
					if s.Score < s.MaxScore*3/4 {
						fmt.Fprintf(out, "      %s: %d/%d\n", s.Name, s.Score, s.MaxScore)
					}
				}
			}
		}
	} else {
		fmt.Fprintf(out, " ❌ no output\n")
	}
}
