package eval

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// noOutputChecksReasoning is recorded for checked criteria when the agent
// produced no output or there is no workspace, so the checks were not run.
const noOutputChecksReasoning = "agent produced no output; checks not run"

// maxGHLogBytes caps how much of .eval/gh.log is read.
const maxGHLogBytes = 10 << 20

// maxReasoningDetail is the length a check's detail is truncated to in reasoning.
const maxReasoningDetail = 200

// scoreCase runs every non-cost rubric criterion against cr.ActualOutput and
// appends the CriterionScores to cr.Scores, adding LLM-judge cost to cr.JudgeCost.
// cr.Scores stays nil when the rubric has no non-cost criteria.
//
// A criterion that has checks in tc is scored from them (passed out of total)
// instead of by the heuristic or the judge; the checks are evaluated once for
// the whole case. Criteria without checks are scored as before.
func scoreCase(rubric Rubric, tc TestCase, cr *CaseResult) {
	checkTotals := map[string]int{}
	var checkOrder []string // criteria named by checks, first-seen order
	for _, c := range tc.Checks {
		if _, ok := checkTotals[c.Criterion]; !ok {
			checkOrder = append(checkOrder, c.Criterion)
		}
		checkTotals[c.Criterion]++
	}

	// results holds the per-criterion check results; nil when checks did not
	// run because the agent produced nothing to check.
	var results map[string][]CheckResult
	checksRan := len(tc.Checks) > 0 && cr.ActualOutput != "" && cr.WorkspaceDir != ""
	if checksRan {
		ghLog, ghLogOversized := readGHLog(cr.WorkspaceDir)
		all := EvaluateChecks(tc.Checks, CheckInput{
			Dir:            cr.WorkspaceDir,
			Output:         cr.ActualOutput,
			GHLog:          ghLog,
			GHLogOversized: ghLogOversized,
			Base:           cr.baseCommit,
			Turns:          caseTurnEvidence(cr),
		})
		results = make(map[string][]CheckResult, len(checkTotals))
		for i, r := range all {
			name := tc.Checks[i].Criterion
			results[name] = append(results[name], r)
		}
	}
	// checkedScore builds the score of a criterion that has checks.
	checkedScore := func(name string) CriterionScore {
		total := checkTotals[name]
		score := CriterionScore{Name: name, MaxScore: total, Deterministic: true}
		if !checksRan {
			score.Reasoning = noOutputChecksReasoning
			return score
		}
		score.Checks = results[name]
		for _, r := range score.Checks {
			if r.Passed {
				score.Score++
			}
		}
		score.Reasoning = checksReasoning(score.Checks)
		return score
	}

	scored := map[string]bool{}
	for _, criterion := range rubric.Criteria {
		if criterion.Type == "cost" {
			continue // cost is tracked separately
		}
		scored[criterion.Name] = true

		if checkTotals[criterion.Name] > 0 {
			cr.Scores = append(cr.Scores, checkedScore(criterion.Name))
			continue
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

	// A check naming a criterion that is not scored above (a hand-built case
	// that bypassed the loader's validation) is never dropped silently.
	for _, name := range checkOrder {
		if scored[name] {
			continue
		}
		score := checkedScore(name)
		score.Score = 0
		score.Reasoning = fmt.Sprintf("criterion %q is not in the rubric", name)
		cr.Scores = append(cr.Scores, score)
	}
}

// caseTurnEvidence builds the per-turn check evidence of a multi-turn case from the
// recorded outputs and the gh-log snapshots taken at the end of each turn. It
// returns nil for a classic case (or when the snapshots are missing), so
// checks fall back to the final Output/GHLog.
func caseTurnEvidence(cr *CaseResult) []TurnEvidence {
	if len(cr.TurnOutputs) == 0 || len(cr.turnGHLogs) != len(cr.TurnOutputs) {
		return nil
	}
	ev := make([]TurnEvidence, len(cr.TurnOutputs))
	for i, out := range cr.TurnOutputs {
		ev[i] = TurnEvidence{
			Output:         out,
			GHLog:          cr.turnGHLogs[i].Content,
			GHLogOversized: cr.turnGHLogs[i].Oversized,
		}
	}
	return ev
}

// readGHLog returns the contents of <dir>/.eval/gh.log and whether it exceeds
// maxGHLogBytes. It returns "" (and false) when the log is absent, not a
// regular file (the agent can write .eval/) or unreadable. When the log is
// larger than the cap it returns the truncated content and oversized=true, so
// callers fail the gh_log checks rather than scoring incomplete text.
func readGHLog(dir string) (string, bool) {
	p := filepath.Join(dir, ".eval", "gh.log")
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	f, err := os.Open(p)
	if err != nil {
		return "", false
	}
	defer f.Close()
	// Read one byte past the cap so an exactly-cap-sized log is not mistaken
	// for an oversized one.
	data, err := io.ReadAll(io.LimitReader(f, maxGHLogBytes+1))
	if err != nil {
		return "", false
	}
	if len(data) > maxGHLogBytes {
		return string(data[:maxGHLogBytes]), true
	}
	return string(data), false
}

// checksReasoning summarizes check results: "3/3 checks passed", or
// "1/2 checks passed; failed: <label> (<detail>); ..." naming every failed check.
func checksReasoning(results []CheckResult) string {
	passed := 0
	var failed []string
	for _, r := range results {
		if r.Passed {
			passed++
			continue
		}
		f := r.Label
		if r.Detail != "" {
			f += " (" + shorten(r.Detail, maxReasoningDetail) + ")"
		}
		failed = append(failed, f)
	}
	reasoning := fmt.Sprintf("%d/%d checks passed", passed, len(results))
	if len(failed) > 0 {
		reasoning += "; failed: " + strings.Join(failed, "; ")
	}
	return reasoning
}

// caseTotals returns the summed score and maximum score of a case's criteria.
// Skipped criteria contribute to neither the numerator nor the denominator.
// This is the single place the per-case score arithmetic lives.
func caseTotals(cr CaseResult) (score, max int) {
	for _, s := range cr.Scores {
		if s.Skipped {
			continue
		}
		score += s.Score
		max += s.MaxScore
	}
	return score, max
}

// agentScoreTotals returns the summed score and maximum score across every
// case of an agent, as floats, using caseTotals so skipped criteria are
// excluded from both. The agent score is score/max when max > 0.
func agentScoreTotals(ar AgentResult) (score, max float64) {
	for _, c := range ar.Cases {
		cs, cm := caseTotals(c)
		score += float64(cs)
		max += float64(cm)
	}
	return score, max
}

// printCaseResult writes the final status line for a scored case:
// "no output" / "no scored criteria" / pass (✅) / warn (⚠️, pct>=60) / fail (❌),
// plus the breakdown of criteria scoring below 3/4 of max when pct < threshold.
func printCaseResult(out io.Writer, tc TestCase, cr CaseResult) {
	if cr.ActualOutput != "" {
		totalScore, maxTotal := caseTotals(cr)
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

		// Failed checks are listed whatever the percentage.
		for _, s := range cr.Scores {
			for _, c := range s.Checks {
				if c.Passed {
					continue
				}
				if c.Detail != "" {
					fmt.Fprintf(out, "      ✗ %s: %s\n", c.Label, c.Detail)
				} else {
					fmt.Fprintf(out, "      ✗ %s\n", c.Label)
				}
			}
		}
	} else {
		fmt.Fprintf(out, " ❌ no output\n")
	}
}
