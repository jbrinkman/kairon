package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	filters, err := ParseFilters(opts.Filters)
	if err != nil {
		return err
	}
	return diffFiltered(os.Stdout, runA, runB, filters)
}

// Diff compares two eval runs and prints provenance and score/cost deltas to
// stdout.
func Diff(runA, runB string) error {
	return diffTo(os.Stdout, runA, runB)
}

// diffTo is Diff writing to w. It writes nothing anywhere else.
func diffTo(w io.Writer, runA, runB string) error {
	return diffFiltered(w, runA, runB, nil)
}

// diffFiltered is diffTo narrowed to the case results that match every filter.
// With no filters it reports the recorded summaries. With filters it recomputes
// every figure from the matching recorded case results, because the summary
// totals cannot be filtered.
func diffFiltered(w io.Writer, runA, runB string, filters []Filter) error {
	resultsDir := evalsPath("results")
	filtered := len(filters) > 0

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
	if filtered {
		fmt.Fprintf(w, "Filter: %s\n", FormatFilters(filters))
	}

	// Provenance and execution mode, before any deltas.
	printProvenance(w, runA, runB,
		loadRunInfo(summaryA, filepath.Join(resultsDir, resolvedA)),
		loadRunInfo(summaryB, filepath.Join(resultsDir, resolvedB)))

	allAgents := mergeKeys(summaryA.AgentScores, summaryB.AgentScores)
	if filtered {
		sort.Strings(allAgents)
	}

	// Load every agent's recorded results once. The Cases block depends on
	// whether any result in either run carries metadata.
	type agentPair struct {
		a, b       AgentResult
		errA, errB error
	}
	pairs := make(map[string]agentPair, len(allAgents))
	hasMetadata := false
	for _, agent := range allAgents {
		var p agentPair
		p.a, p.errA = loadAgentResult(filepath.Join(resultsDir, resolvedA, agent+".json"))
		p.b, p.errB = loadAgentResult(filepath.Join(resultsDir, resolvedB, agent+".json"))
		pairs[agent] = p
		hasMetadata = hasMetadata || resultsHaveMetadata(p.a) || resultsHaveMetadata(p.b)
	}

	sideA := diffSide{scores: map[string]float64{}}
	sideB := diffSide{scores: map[string]float64{}}

	// Per-agent, per-criterion deltas
	for _, agent := range allAgents {
		p := pairs[agent]
		resultA, resultB := p.a, p.b
		errA, errB := p.errA, p.errB

		scoreA := summaryA.AgentScores[agent]
		scoreB := summaryB.AgentScores[agent]
		hasSideA, hasSideB := true, true

		if filtered {
			// An agent present in only one run has no <agent>.json on the
			// other side. Treat that one missing file (os.ErrNotExist) as an
			// empty side, so the agent is still shown with "-" on the empty
			// side rather than skipped. If BOTH sides are missing, or a file
			// is genuinely unreadable, there is nothing to match: skip.
			missingA := errors.Is(errA, os.ErrNotExist)
			missingB := errors.Is(errB, os.ErrNotExist)
			if missingA && !missingB && errB == nil {
				resultA, errA = AgentResult{}, nil
			}
			if missingB && !missingA && errA == nil {
				resultB, errB = AgentResult{}, nil
			}
			if errA != nil || errB != nil {
				fmt.Fprintf(w, "\n%s: no case results to filter, skipped\n", agent)
				continue
			}
			resultA = reduceResult(resultA, agent, filters)
			resultB = reduceResult(resultB, agent, filters)
			hasSideA, hasSideB = len(resultA.Cases) > 0, len(resultB.Cases) > 0
			if !hasSideA && !hasSideB {
				continue
			}
			scoreA, scoreB = fractionScore(resultA), fractionScore(resultB)
			if hasSideA {
				sideA.scores[agent] = scoreA
				sideA.accumulate(resultA)
			}
			if hasSideB {
				sideB.scores[agent] = scoreB
				sideB.accumulate(resultB)
			}
		}

		delta := scoreB - scoreA
		indicator := "→"
		if delta > 0.001 {
			indicator = "↑"
		} else if delta < -0.001 {
			indicator = "↓"
		}
		switch {
		case !hasSideA:
			fmt.Fprintf(w, "\n%s: - → %.3f\n", agent, scoreB)
		case !hasSideB:
			fmt.Fprintf(w, "\n%s: %.3f → -\n", agent, scoreA)
		default:
			fmt.Fprintf(w, "\n%s: %.3f → %.3f  %s %+.3f\n", agent, scoreA, scoreB, indicator, delta)
		}

		if errA != nil || errB != nil {
			continue
		}

		// Build criterion averages for each run
		avgA := criterionAverages(resultA)
		avgB := criterionAverages(resultB)
		allCriteria := mergeKeys(avgA, avgB)
		if filtered {
			sort.Strings(allCriteria)
		}

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

		if hasMetadata {
			showCaseScores(w, resultA, resultB)
		}

		// Show output changes for significant score deltas
		if hasSideA && hasSideB && (delta > 0.05 || delta < -0.05) {
			showOutputChanges(w, resultA, resultB)
		}
	}

	// Cost delta. The summary totals cover every case, so a filtered diff
	// sums the matching case results instead.
	totalA, totalB := summaryA.TotalCost, summaryB.TotalCost
	scoresA, scoresB := summaryA.AgentScores, summaryB.AgentScores
	if filtered {
		totalA, totalB = sideA.total(), sideB.total()
		scoresA, scoresB = sideA.scores, sideB.scores
	}
	fmt.Fprintf(w, "\nCost Delta:\n")
	costDelta := totalB.EstimatedUSD - totalA.EstimatedUSD
	tokenDelta := (totalB.TokensIn + totalB.TokensOut) -
		(totalA.TokensIn + totalA.TokensOut)
	fmt.Fprintf(w, "  Tokens: %+d\n", tokenDelta)
	var incompleteSides []string
	if totalA.CostIncomplete {
		incompleteSides = append(incompleteSides, runA)
	}
	if totalB.CostIncomplete {
		incompleteSides = append(incompleteSides, runB)
	}
	anyIncomplete := len(incompleteSides) > 0
	costNote := ""
	qualityNote := ""
	if anyIncomplete {
		costNote = " (incomplete: " + strings.Join(incompleteSides, ", ") + ")"
		qualityNote = " (cost incomplete)"
	}
	fmt.Fprintf(w, "  Cost:   %+.6f USD%s\n", costDelta, costNote)

	// Cost trends (agent vs judge)
	fmt.Fprintf(w, "\nCost Trends:\n")
	if filtered {
		printCostTrends(w, sideA.agentCost, sideA.judges, sideB.agentCost, sideB.judges)
	} else {
		showCostTrends(w, summaryA, summaryB, resolvedA, resolvedB, resultsDir)
	}

	// Quality per dollar
	fmt.Fprintf(w, "\nQuality per Dollar:\n")
	avgA := avgScore(scoresA)
	avgB := avgScore(scoresB)
	if totalA.EstimatedUSD > 0 {
		fmt.Fprintf(w, "  %s: %.2f quality/$%s\n", runA, avgA/totalA.EstimatedUSD, qualityNote)
	}
	if totalB.EstimatedUSD > 0 {
		fmt.Fprintf(w, "  %s: %.2f quality/$%s\n", runB, avgB/totalB.EstimatedUSD, qualityNote)
	}

	return nil
}

// diffSide is what one run contributes to the filtered sections of a diff: the
// score of each agent that has a matching case, and the cost of those cases.
type diffSide struct {
	scores            map[string]float64
	agentCost, judges CostInfo
}

// accumulate adds the agent and judge cost of every case in r.
func (s *diffSide) accumulate(r AgentResult) {
	for _, c := range r.Cases {
		s.agentCost.addTotals(c.AgentCost)
		s.judges.addTotals(c.JudgeCost)
	}
}

// total is the agent and judge cost together.
func (s diffSide) total() CostInfo {
	var t CostInfo
	t.addTotals(s.agentCost)
	t.addTotals(s.judges)
	return t
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

// runOneOutputs maps case name to the output of its first run.
func runOneOutputs(r AgentResult) map[string]string {
	outputs := make(map[string]string)
	runs := make(map[string]int)
	for _, c := range r.Cases {
		run := max(1, c.Run)
		if prev, seen := runs[c.CaseName]; seen && prev <= run {
			continue
		}
		runs[c.CaseName] = run
		outputs[c.CaseName] = c.ActualOutput
	}
	return outputs
}

func showOutputChanges(w io.Writer, resultA, resultB AgentResult) {
	// Compare outputs for cases with same name
	// A repeated case has several results; compare run 1 (a legacy result
	// without a run is run 1) so the comparison does not depend on how many
	// repeats either side ran.
	outputsA := runOneOutputs(resultA)
	outputsB := runOneOutputs(resultB)
	metaA := caseMetadataByName(resultA)
	metaB := caseMetadataByName(resultB)

	names := make([]string, 0, len(outputsA))
	for caseName := range outputsA {
		names = append(names, caseName)
	}
	sort.Strings(names)

	for _, caseName := range names {
		outputA := outputsA[caseName]
		if outputB, exists := outputsB[caseName]; exists && outputA != outputB {
			// Run B's metadata wins; run A's is the fallback.
			marker := formatMetadata(metaB[caseName])
			if marker == "" {
				marker = formatMetadata(metaA[caseName])
			}
			if marker != "" {
				marker = " " + marker
			}
			fmt.Fprintf(w, "    Case '%s' output changed:%s\n", caseName, marker)

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

// reduceResult returns r holding only the case results that match every
// filter. agent is the agent the result belongs to.
func reduceResult(r AgentResult, agent string, filters []Filter) AgentResult {
	var kept []CaseResult
	for _, c := range r.Cases {
		if matchAll(filters, resultLookup(agent, c)) {
			kept = append(kept, c)
		}
	}
	r.Cases = kept
	return r
}

// fractionScore is the agent score as a 0-1 fraction (score/max), 0 when
// nothing was scored.
func fractionScore(r AgentResult) float64 {
	score, max := agentScoreTotals(r)
	if max <= 0 {
		return 0
	}
	return score / max
}

// resultsHaveMetadata reports whether any case result records metadata.
func resultsHaveMetadata(r AgentResult) bool {
	for _, c := range r.Cases {
		if len(c.Metadata) > 0 {
			return true
		}
	}
	return false
}

// formatMetadata renders metadata as "[k=v, k=v]" with keys sorted, or "" when
// there is none.
func formatMetadata(m CaseMetadata) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k]
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// caseMetadataByName maps each case name to the first non-empty metadata
// recorded for it.
func caseMetadataByName(r AgentResult) map[string]CaseMetadata {
	out := make(map[string]CaseMetadata)
	for _, c := range r.Cases {
		if len(out[c.CaseName]) == 0 && len(c.Metadata) > 0 {
			out[c.CaseName] = c.Metadata
		}
	}
	return out
}

// casePooledScore returns the pooled score/max over every result of a case
// name, and whether the case has any result.
func casePooledScore(r AgentResult, name string) (float64, bool) {
	var score, max float64
	found := false
	for _, c := range r.Cases {
		if c.CaseName != name {
			continue
		}
		found = true
		cs, cm := caseTotals(c)
		score += float64(cs)
		max += float64(cm)
	}
	if !found || max <= 0 {
		return 0, found
	}
	return score / max, true
}

// showCaseScores prints the Cases block: one line per case name seen in either
// run, with its metadata marker and its pooled score on each side.
func showCaseScores(w io.Writer, resultA, resultB AgentResult) {
	seen := map[string]bool{}
	var names []string
	for _, r := range []AgentResult{resultA, resultB} {
		for _, c := range r.Cases {
			if !seen[c.CaseName] {
				seen[c.CaseName] = true
				names = append(names, c.CaseName)
			}
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	metaA := caseMetadataByName(resultA)
	metaB := caseMetadataByName(resultB)

	score := func(r AgentResult, name string) string {
		if v, ok := casePooledScore(r, name); ok {
			return fmt.Sprintf("%.3f", v)
		}
		return "-"
	}
	fmt.Fprintln(w, "  Cases:")
	for _, name := range names {
		marker := formatMetadata(metaB[name])
		if marker == "" {
			marker = formatMetadata(metaA[name])
		}
		label := name
		if marker != "" {
			label += " " + marker
		}
		fmt.Fprintf(w, "    %s  %s → %s\n", label, score(resultA, name), score(resultB, name))
	}
}

func showCostTrends(w io.Writer, summaryA, summaryB Summary, runA, runB, resultsDir string) {
	agentCostA, judgeCostA := calculateAgentJudgeCosts(runA, resultsDir)
	agentCostB, judgeCostB := calculateAgentJudgeCosts(runB, resultsDir)
	printCostTrends(w, agentCostA, judgeCostA, agentCostB, judgeCostB)
}

func printCostTrends(w io.Writer, agentCostA, judgeCostA, agentCostB, judgeCostB CostInfo) {
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
			agentCost.addTotals(c.AgentCost)
			judgeCost.addTotals(c.JudgeCost)
		}
	}

	return agentCost, judgeCost
}
