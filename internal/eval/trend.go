package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// trendPromptLen is how many characters of prompt_sha256 a trend row shows.
const trendPromptLen = 8

// TrendOptions configures Trend.
type TrendOptions struct {
	EvalsDir string // "" => defaultEvalsDir
	JSON     bool
	Filters  []Filter // when set, each run is scored from its matching case results only
}

// TrendRow is one run of an agent in the trend. Optional facts the run does
// not record are "" (strings) or nil (numbers and booleans, written as null).
type TrendRow struct {
	Run          string   `json:"run"`
	Date         string   `json:"date"`          // RFC 3339 UTC; "" when the run name has no timestamp
	Score        float64  `json:"score"`         // percent, same value as the verdict score
	SuccessRate  *float64 `json:"success_rate"`  // percent; nil when not recorded
	AgentModel   string   `json:"agent_model"`   // "" when not recorded
	JudgeModel   string   `json:"judge_model"`   // "" when not recorded
	PromptSHA256 string   `json:"prompt_sha256"` // first 8 characters only; "" when not recorded
	Complete     *bool    `json:"complete"`      // nil when the run predates coverage tracking

	// CaseMetadata maps each metadata key to the sorted, distinct values
	// across the run's (filtered) case results. Omitted when no case carries
	// metadata.
	CaseMetadata map[string][]string `json:"case_metadata,omitempty"`
}

// LoadTrend returns one row per run under <evalsDir>/results that holds a
// score for agent, oldest first. The date comes from the run directory name
// (YYMMDD-HHMMSS, UTC); a name without a timestamp sorts first and has no
// date. Ties break on the run name. It never reads package state; the
// directory is explicit ("" => defaultEvalsDir).
func LoadTrend(agent, evalsDir string) ([]TrendRow, error) {
	return loadTrendFiltered(agent, evalsDir, nil)
}

// loadTrendFiltered is LoadTrend narrowed to the case results that match
// every filter.
func loadTrendFiltered(agent, evalsDir string, filters []Filter) ([]TrendRow, error) {
	if err := validatePathElement("agent name", agent); err != nil {
		return nil, err
	}
	if evalsDir == "" {
		evalsDir = defaultEvalsDir
	}
	resultsDir := filepath.Join(evalsDir, "results")
	entries, err := os.ReadDir(resultsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read results directory: %w", err)
	}

	type dated struct {
		when time.Time
		row  TrendRow
	}
	var runs []dated
	for _, e := range entries {
		name := e.Name()
		rec := loadRunRecord(resultsDir, name, agent)
		if !rec.exists {
			continue
		}
		when, _ := splitRunDirName(name)
		result, resErr := loadAgentResult(filepath.Join(resultsDir, name, agent+".json"))
		if resErr != nil {
			result = AgentResult{}
		}
		if len(filters) > 0 {
			// Score the run from the matching case results only. A run
			// with none has nothing to report.
			result = reduceResult(result, agent, filters)
			if len(result.Cases) == 0 {
				continue
			}
			v := agentVerdict(result)
			rec.score = v.Score
			rec.successRate = nil
			if v.AttemptsTotal > 0 {
				rate := v.SuccessRate
				rec.successRate = &rate
			}
		} else if !rec.hasScore {
			continue
		}
		row := TrendRow{
			Run:          name,
			Score:        rec.score,
			SuccessRate:  rec.successRate,
			AgentModel:   rec.agentModel,
			JudgeModel:   rec.judgeModel,
			PromptSHA256: trendTruncate(rec.promptSHA, trendPromptLen),
			CaseMetadata: trendCaseMetadata(result),
		}
		if !when.IsZero() {
			row.Date = when.UTC().Format(time.RFC3339)
		}
		if rec.hasComplete {
			c := rec.complete
			row.Complete = &c
		}
		runs = append(runs, dated{when: when, row: row})
	}
	if len(runs) == 0 {
		if len(filters) > 0 {
			return nil, fmt.Errorf("no eval results for agent %q matching --filter %s under %s", agent, FormatFilters(filters), resultsDir)
		}
		return nil, fmt.Errorf("no eval results for agent %q under %s", agent, resultsDir)
	}

	sort.SliceStable(runs, func(i, j int) bool {
		if !runs[i].when.Equal(runs[j].when) {
			return runs[i].when.Before(runs[j].when)
		}
		return runs[i].row.Run < runs[j].row.Run
	})
	rows := make([]TrendRow, len(runs))
	for i, r := range runs {
		rows[i] = r.row
	}
	return rows, nil
}

// trendCaseMetadata maps each metadata key to the sorted, distinct values
// across r's case results. It is nil when no case carries metadata.
func trendCaseMetadata(r AgentResult) map[string][]string {
	seen := map[string]map[string]bool{}
	for _, c := range r.Cases {
		for k, v := range c.Metadata {
			if seen[k] == nil {
				seen[k] = map[string]bool{}
			}
			seen[k][v] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make(map[string][]string, len(seen))
	for k, vs := range seen {
		for v := range vs {
			out[k] = append(out[k], v)
		}
		sort.Strings(out[k])
	}
	return out
}

// trendMetadataSuffix renders case_metadata as "metadata k=v1,v2 k2=v", keys
// sorted, or "" when there is none.
func trendMetadataSuffix(m map[string][]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{"metadata"}
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(m[k], ","))
	}
	return strings.Join(parts, " ")
}

// trendTruncate returns the first n characters of s, or s when it is shorter.
func trendTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Trend prints the runs of agent, oldest first: one line per run, or with
// opts.JSON an indented JSON array of the same rows. Nothing is written when
// it returns an error. It never reads package state.
func Trend(w io.Writer, agent string, opts TrendOptions) error {
	rows, err := loadTrendFiltered(agent, opts.EvalsDir, opts.Filters)
	if err != nil {
		return err
	}
	if opts.JSON {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		_, err = w.Write(append(data, '\n'))
		return err
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(formatTrendRow(r))
		b.WriteByte('\n')
	}
	_, err = io.WriteString(w, b.String())
	return err
}

// formatTrendRow renders one text line. Fields are separated by two spaces and
// an unknown value is "-".
func formatTrendRow(r TrendRow) string {
	success := "-"
	if r.SuccessRate != nil {
		success = fmt.Sprintf("%.1f%%", *r.SuccessRate)
	}
	completeness := "completeness-unknown"
	if r.Complete != nil {
		completeness = "INCOMPLETE"
		if *r.Complete {
			completeness = "complete"
		}
	}
	fields := []string{
		r.Run,
		trendDash(r.Date),
		fmt.Sprintf("score %.1f%%", r.Score),
		"success " + success,
		"agent_model " + trendDash(r.AgentModel),
		"judge_model " + trendDash(r.JudgeModel),
		"prompt " + trendDash(r.PromptSHA256),
		completeness,
	}
	if suffix := trendMetadataSuffix(r.CaseMetadata); suffix != "" {
		fields = append(fields, suffix)
	}
	return strings.Join(fields, "  ")
}

func trendDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
