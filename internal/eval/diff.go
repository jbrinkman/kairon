package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// resolveRunDirectory handles both old and new format detection
func resolveRunDirectory(runName string) (string, error) {
	resultsDir := evalsPath("results")

	// If directory exists as-is, use it
	fullPath := filepath.Join(resultsDir, runName)
	if _, err := os.Stat(fullPath); err == nil {
		return runName, nil
	}

	// If runName looks like a hash only, search for timestamped version
	hash := parseDirectoryName(runName)
	if hash == runName {
		// This is just a hash, look for a timestamped directory with this hash
		entries, err := os.ReadDir(resultsDir)
		if err != nil {
			return "", fmt.Errorf("failed to read results directory: %w", err)
		}

		var latest string
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			dirHash := parseDirectoryName(entry.Name())
			if dirHash == hash {
				latest = entry.Name()
			}
		}
		if latest != "" {
			return latest, nil
		}
	}

	return "", fmt.Errorf("run directory %s not found", runName)
}

// DiffWithOptions is like Diff but first applies opts (notably EvalsDir) so
// that results are read from the configured evals directory.
func DiffWithOptions(runA, runB string, opts RunOptions) error {
	if err := configure(opts); err != nil {
		return err
	}
	return Diff(runA, runB)
}

// Diff compares two eval runs and prints provenance and score/cost deltas to
// stdout.
func Diff(runA, runB string) error {
	return diffTo(os.Stdout, runA, runB)
}

// diffTo is Diff writing to w. It writes nothing anywhere else.
func diffTo(w io.Writer, runA, runB string) error {
	resultsDir := evalsPath("results")

	resolvedA, err := resolveRunDirectory(runA)
	if err != nil {
		return fmt.Errorf("failed to resolve run %s: %w", runA, err)
	}

	resolvedB, err := resolveRunDirectory(runB)
	if err != nil {
		return fmt.Errorf("failed to resolve run %s: %w", runB, err)
	}

	summaryA, err := loadSummary(filepath.Join(resultsDir, resolvedA, "summary.json"))
	if err != nil {
		return fmt.Errorf("failed to load run %s: %w", runA, err)
	}

	summaryB, err := loadSummary(filepath.Join(resultsDir, resolvedB, "summary.json"))
	if err != nil {
		return fmt.Errorf("failed to load run %s: %w", runB, err)
	}

	fmt.Fprintf(w, "Eval Diff: %s → %s\n", runA, runB)
	fmt.Fprintln(w, strings.Repeat("─", 60))

	// Provenance and execution mode, before any deltas.
	printProvenance(w, runA, runB,
		loadRunInfo(summaryA, filepath.Join(resultsDir, resolvedA)),
		loadRunInfo(summaryB, filepath.Join(resultsDir, resolvedB)))

	// Per-agent, per-criterion deltas
	allAgents := mergeKeys(summaryA.AgentScores, summaryB.AgentScores)
	for _, agent := range allAgents {
		resultA, errA := loadAgentResult(filepath.Join(resultsDir, resolvedA, agent+".json"))
		resultB, errB := loadAgentResult(filepath.Join(resultsDir, resolvedB, agent+".json"))

		scoreA := summaryA.AgentScores[agent]
		scoreB := summaryB.AgentScores[agent]
		delta := scoreB - scoreA
		indicator := "→"
		if delta > 0.001 {
			indicator = "↑"
		} else if delta < -0.001 {
			indicator = "↓"
		}
		fmt.Fprintf(w, "\n%s: %.3f → %.3f  %s %+.3f\n", agent, scoreA, scoreB, indicator, delta)

		if errA != nil || errB != nil {
			continue
		}

		// Build criterion averages for each run
		avgA := criterionAverages(resultA)
		avgB := criterionAverages(resultB)
		allCriteria := mergeKeys(avgA, avgB)

		for _, crit := range allCriteria {
			cA, hasA := avgA[crit]
			cB, hasB := avgB[crit]
			if !hasA && !hasB {
				continue
			}
			cDelta := cB - cA
			cInd := "→"
			if cDelta > 0.001 {
				cInd = "↑"
			} else if cDelta < -0.001 {
				cInd = "↓"
			}
			if !hasA {
				fmt.Fprintf(w, "  %-30s  [new]  %.3f\n", crit, cB)
			} else if !hasB {
				fmt.Fprintf(w, "  %-30s  %.3f  [removed]\n", crit, cA)
			} else {
				fmt.Fprintf(w, "  %-30s  %.3f → %.3f  %s %+.3f\n", crit, cA, cB, cInd, cDelta)
			}
		}

		// Show output changes for significant score deltas
		if delta > 0.05 || delta < -0.05 {
			showOutputChanges(w, resultA, resultB)
		}
	}

	// Cost delta
	fmt.Fprintf(w, "\nCost Delta:\n")
	costDelta := summaryB.TotalCost.EstimatedUSD - summaryA.TotalCost.EstimatedUSD
	tokenDelta := (summaryB.TotalCost.TokensIn + summaryB.TotalCost.TokensOut) -
		(summaryA.TotalCost.TokensIn + summaryA.TotalCost.TokensOut)
	fmt.Fprintf(w, "  Tokens: %+d\n", tokenDelta)
	fmt.Fprintf(w, "  Cost:   %+.6f USD\n", costDelta)

	// Cost trends (agent vs judge)
	fmt.Fprintf(w, "\nCost Trends:\n")
	showCostTrends(w, summaryA, summaryB, resolvedA, resolvedB, resultsDir)

	// Quality per dollar
	fmt.Fprintf(w, "\nQuality per Dollar:\n")
	avgA := avgScore(summaryA.AgentScores)
	avgB := avgScore(summaryB.AgentScores)
	if summaryA.TotalCost.EstimatedUSD > 0 {
		fmt.Fprintf(w, "  %s: %.2f quality/$ \n", runA, avgA/summaryA.TotalCost.EstimatedUSD)
	}
	if summaryB.TotalCost.EstimatedUSD > 0 {
		fmt.Fprintf(w, "  %s: %.2f quality/$\n", runB, avgB/summaryB.TotalCost.EstimatedUSD)
	}

	return nil
}

func loadSummary(path string) (Summary, error) {
	var s Summary
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func loadAgentResult(path string) (AgentResult, error) {
	var r AgentResult
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(data, &r)
	return r, err
}

func criterionAverages(result AgentResult) map[string]float64 {
	totals := make(map[string]float64)
	counts := make(map[string]int)
	for _, c := range result.Cases {
		for _, sc := range c.Scores {
			if sc.Skipped || sc.MaxScore == 0 {
				continue
			}
			totals[sc.Name] += float64(sc.Score) / float64(sc.MaxScore)
			counts[sc.Name]++
		}
	}
	avgs := make(map[string]float64)
	for name, total := range totals {
		if counts[name] > 0 {
			avgs[name] = total / float64(counts[name])
		}
	}
	return avgs
}

func mergeKeys(a, b map[string]float64) []string {
	seen := make(map[string]bool)
	var keys []string
	for k := range a {
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for k := range b {
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	return keys
}

func avgScore(scores map[string]float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, v := range scores {
		sum += v
	}
	return sum / float64(len(scores))
}

func showOutputChanges(w io.Writer, resultA, resultB AgentResult) {
	// Compare outputs for cases with same name
	outputsA := make(map[string]string)
	outputsB := make(map[string]string)

	for _, c := range resultA.Cases {
		outputsA[c.CaseName] = c.ActualOutput
	}
	for _, c := range resultB.Cases {
		outputsB[c.CaseName] = c.ActualOutput
	}

	for caseName, outputA := range outputsA {
		if outputB, exists := outputsB[caseName]; exists && outputA != outputB {
			fmt.Fprintf(w, "    Case '%s' output changed:\n", caseName)

			// Truncate long outputs for side-by-side display
			linesA := strings.Split(strings.TrimSpace(outputA), "\n")
			linesB := strings.Split(strings.TrimSpace(outputB), "\n")

			maxLines := 3
			if len(linesA) > maxLines {
				linesA = append(linesA[:maxLines], "...")
			}
			if len(linesB) > maxLines {
				linesB = append(linesB[:maxLines], "...")
			}

			fmt.Fprintf(w, "      Before: %s\n", strings.Join(linesA, " "))
			fmt.Fprintf(w, "      After:  %s\n", strings.Join(linesB, " "))
		}
	}
}

func showCostTrends(w io.Writer, summaryA, summaryB Summary, runA, runB, resultsDir string) {
	agentCostA, judgeCostA := calculateAgentJudgeCosts(runA, resultsDir)
	agentCostB, judgeCostB := calculateAgentJudgeCosts(runB, resultsDir)

	fmt.Fprintf(w, "  Agent tokens:  %d → %d (%+d)\n",
		agentCostA.TokensIn+agentCostA.TokensOut,
		agentCostB.TokensIn+agentCostB.TokensOut,
		(agentCostB.TokensIn+agentCostB.TokensOut)-(agentCostA.TokensIn+agentCostA.TokensOut))

	fmt.Fprintf(w, "  Judge tokens:  %d → %d (%+d)\n",
		judgeCostA.TokensIn+judgeCostA.TokensOut,
		judgeCostB.TokensIn+judgeCostB.TokensOut,
		(judgeCostB.TokensIn+judgeCostB.TokensOut)-(judgeCostA.TokensIn+judgeCostA.TokensOut))
}

func calculateAgentJudgeCosts(runDir, resultsDir string) (CostInfo, CostInfo) {
	var agentCost, judgeCost CostInfo

	entries, err := os.ReadDir(filepath.Join(resultsDir, runDir))
	if err != nil {
		return agentCost, judgeCost
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "summary.json" {
			continue
		}

		result, err := loadAgentResult(filepath.Join(resultsDir, runDir, entry.Name()))
		if err != nil {
			continue
		}

		for _, c := range result.Cases {
			agentCost.TokensIn += c.AgentCost.TokensIn
			agentCost.TokensOut += c.AgentCost.TokensOut
			agentCost.EstimatedUSD += c.AgentCost.EstimatedUSD

			judgeCost.TokensIn += c.JudgeCost.TokensIn
			judgeCost.TokensOut += c.JudgeCost.TokensOut
			judgeCost.EstimatedUSD += c.JudgeCost.EstimatedUSD
		}
	}

	return agentCost, judgeCost
}
