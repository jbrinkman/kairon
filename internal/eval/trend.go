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
}

// LoadTrend returns one row per run under <evalsDir>/results that holds a
// score for agent, oldest first. The date comes from the run directory name
// (YYMMDD-HHMMSS, UTC); a name without a timestamp sorts first and has no
// date. Ties break on the run name. It never reads package state; the
// directory is explicit ("" => defaultEvalsDir).
func LoadTrend(agent, evalsDir string) ([]TrendRow, error) {
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
		if !rec.exists || !rec.hasScore {
			continue
		}
		when, _ := splitRunDirName(name)
		row := TrendRow{
			Run:          name,
			Score:        rec.score,
			SuccessRate:  trendSuccessRate(filepath.Join(resultsDir, name), agent),
			AgentModel:   rec.agentModel,
			JudgeModel:   rec.judgeModel,
			PromptSHA256: trendTruncate(rec.promptSHA, trendPromptLen),
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

// trendSuccessRate returns the attempt success rate (percent) of agent in the
// run directory dir: the summary verdict's value when it counted attempts,
// else recomputed from <agent>.json when that holds case results, else nil.
func trendSuccessRate(dir, agent string) *float64 {
	if summary, err := loadSummary(filepath.Join(dir, "summary.json")); err == nil {
		if v, ok := summary.AgentVerdicts[agent]; ok && v.AttemptsTotal > 0 {
			rate := v.SuccessRate
			return &rate
		}
	}
	if result, err := loadAgentResult(filepath.Join(dir, agent+".json")); err == nil && len(result.Cases) > 0 {
		rate := agentVerdict(result).SuccessRate
		return &rate
	}
	return nil
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
	rows, err := LoadTrend(agent, opts.EvalsDir)
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
	return strings.Join([]string{
		r.Run,
		trendDash(r.Date),
		fmt.Sprintf("score %.1f%%", r.Score),
		"success " + success,
		"agent_model " + trendDash(r.AgentModel),
		"judge_model " + trendDash(r.JudgeModel),
		"prompt " + trendDash(r.PromptSHA256),
		completeness,
	}, "  ")
}

func trendDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
