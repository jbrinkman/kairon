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
			score.Score, score.Reasoning, score.Skipped = scoreDeterministic(criterion, tc, cr.ActualOutput)
		} else {
			// LLM-judged criteria
			if cr.ActualOutput == "" {
				score.Score = 0
				score.Skipped = true
				score.Reasoning = "no output available for LLM judging"
			} else {
				judgeCost, judgeScore, reasoning, skipped := scoreLLMJudge(criterion, tc, cr.ActualOutput)
				cr.JudgeCost.Add(judgeCost)
				score.Score = judgeScore
				score.Reasoning = reasoning
				score.Skipped = skipped
			}
		}

		cr.Scores = append(cr.Scores, score)
	}
}

// printCaseResult writes the final status line for a scored case:
// "no output" / "no scored criteria" / pass (✅) / warn (⚠️, pct>=60) / fail (❌),
// plus the breakdown of criteria scoring below 3/4 of max when pct < threshold.
func printCaseResult(out io.Writer, tc TestCase, cr CaseResult) {
	if cr.ActualOutput != "" {
		totalScore := 0
		maxTotal := 0
		for _, s := range cr.Scores {
			if !s.Skipped {
				totalScore += s.Score
				maxTotal += s.MaxScore
			}
		}
		if maxTotal == 0 {
			fmt.Fprintf(out, " ⚠️  no scored criteria\n")
		} else {
			pct := float64(totalScore) / float64(maxTotal) * 100
			threshold := getThreshold(tc)

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
					if !s.Skipped && s.Score < s.MaxScore*3/4 {
						fmt.Fprintf(out, "      %s: %d/%d\n", s.Name, s.Score, s.MaxScore)
					}
				}
			}
		}
	} else {
		fmt.Fprintf(out, " ❌ no output\n")
	}
}
