package eval

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeTrendFilterRun writes run under evalsDir with a summary whose verdict
// score (50) is far from what the cases below add up to, so a filtered trend
// that read it instead of the cases would show.
func writeTrendFilterRun(t *testing.T, evalsDir, run string, cases []CaseResult) {
	t.Helper()
	dir := filepath.Join(evalsDir, "results", run)
	trendWrite(t, filepath.Join(dir, "summary.json"),
		`{"agent_verdicts": {"selftest": {"score": 50, "success_rate": 50, "attempts_total": 2, "attempts_passed": 1}}}`)
	data, err := json.Marshal(AgentResult{Agent: trendAgent, Threshold: 70, Cases: cases})
	if err != nil {
		t.Fatal(err)
	}
	trendWrite(t, filepath.Join(dir, "selftest.json"), string(data))
}

func trendCase(name string, meta CaseMetadata, score, max int) CaseResult {
	return CaseResult{CaseName: name, Metadata: meta, Scores: []CriterionScore{{Name: "acc", Score: score, MaxScore: max}}}
}

// trendFilterEvals holds three runs: run 1 mixes fast and slow cases, run 2
// only slow, run 3 has no metadata at all.
func trendFilterEvals(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTrendFilterRun(t, dir, "260701-100000-aaaaaaa", []CaseResult{
		trendCase("fast-1", CaseMetadata{"speed": "fast", "skill": "cli"}, 4, 4),
		trendCase("slow-1", CaseMetadata{"speed": "slow"}, 0, 4),
	})
	writeTrendFilterRun(t, dir, "260702-100000-bbbbbbb", []CaseResult{
		trendCase("slow-1", CaseMetadata{"speed": "slow"}, 2, 4),
	})
	writeTrendFilterRun(t, dir, "260703-100000-ccccccc", []CaseResult{
		trendCase("fast-1", nil, 3, 4),
		trendCase("slow-1", nil, 4, 4),
	})
	return dir
}

func mustFilters(t *testing.T, exprs ...string) []Filter {
	t.Helper()
	fs, err := ParseFilters(exprs)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestTrendRowCaseMetadataFromRunCases(t *testing.T) {
	rows, err := LoadTrend(trendAgent, trendFilterEvals(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	want := map[string][]string{"skill": {"cli"}, "speed": {"fast", "slow"}}
	if !reflect.DeepEqual(rows[0].CaseMetadata, want) {
		t.Errorf("run 1 case_metadata = %v, want %v", rows[0].CaseMetadata, want)
	}
	if want := map[string][]string{"speed": {"slow"}}; !reflect.DeepEqual(rows[1].CaseMetadata, want) {
		t.Errorf("run 2 case_metadata = %v, want %v", rows[1].CaseMetadata, want)
	}
	if rows[2].CaseMetadata != nil {
		t.Errorf("run 3 case_metadata = %v, want nil", rows[2].CaseMetadata)
	}
	// Unfiltered scores still come from the summary.
	if rows[0].Score != 50 {
		t.Errorf("unfiltered score = %v, want the summary's 50", rows[0].Score)
	}
}

func TestTrendJSONIncludesCaseMetadataOnlyWhenPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: trendFilterEvals(t), JSON: true}); err != nil {
		t.Fatal(err)
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw[0]["case_metadata"]); !strings.Contains(got, `"speed"`) || !strings.Contains(got, `"fast"`) {
		t.Errorf("run 1 case_metadata = %s", got)
	}
	if _, ok := raw[2]["case_metadata"]; ok {
		t.Errorf("run 3 has no metadata, case_metadata must be omitted: %v", raw[2])
	}
}

func TestTrendTextAppendsMetadataSuffixOnlyWhenPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: trendFilterEvals(t)}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines: %q", len(lines), lines)
	}
	if !strings.HasSuffix(lines[0], "  metadata skill=cli speed=fast,slow") {
		t.Errorf("run 1 line = %q, want the metadata suffix", lines[0])
	}
	if strings.Contains(lines[2], "metadata") {
		t.Errorf("run 3 line = %q, want no metadata suffix", lines[2])
	}
	if want := formatTrendRow(TrendRow{Run: "260703-100000-ccccccc", Date: "2026-07-03T10:00:00Z", Score: 50, SuccessRate: ptr(50.0)}); lines[2] != want {
		t.Errorf("run 3 line = %q, want the unchanged format %q", lines[2], want)
	}
}

func ptr[T any](v T) *T { return &v }

func TestTrendFilterRecomputesScoreAndSuccessFromMatchingCases(t *testing.T) {
	rows, err := loadTrendFiltered(trendAgent, trendFilterEvals(t), mustFilters(t, "speed==fast"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := trendNames(rows), []string{"260701-100000-aaaaaaa"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %v, want %v (runs without a match are dropped)", got, want)
	}
	r := rows[0]
	if r.Score != 100 {
		t.Errorf("score = %v, want 100 from fast-1 alone", r.Score)
	}
	if r.SuccessRate == nil || *r.SuccessRate != 100 {
		t.Errorf("success rate = %v, want 100", r.SuccessRate)
	}
	if want := map[string][]string{"skill": {"cli"}, "speed": {"fast"}}; !reflect.DeepEqual(r.CaseMetadata, want) {
		t.Errorf("case_metadata = %v, want %v (matching cases only)", r.CaseMetadata, want)
	}
}

func TestTrendFilterBuiltInKeysReachResultsWithoutMetadata(t *testing.T) {
	rows, err := loadTrendFiltered(trendAgent, trendFilterEvals(t), mustFilters(t, "name==fast-1", "agent==selftest"))
	if err != nil {
		t.Fatal(err)
	}
	// Runs 1 and 3 have a fast-1 result; run 3 carries no metadata.
	if got, want := trendNames(rows), []string{"260701-100000-aaaaaaa", "260703-100000-ccccccc"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	if rows[1].Score != 75 {
		t.Errorf("run 3 score = %v, want 75 (3/4)", rows[1].Score)
	}
	if rows[1].SuccessRate == nil || *rows[1].SuccessRate != 100 {
		t.Errorf("run 3 success rate = %v, want 100 (75%% >= threshold 70)", rows[1].SuccessRate)
	}
	if rows[1].CaseMetadata != nil {
		t.Errorf("run 3 case_metadata = %v, want nil", rows[1].CaseMetadata)
	}
}

func TestTrendFilterMetadataKeyDoesNotMatchResultsWithoutMetadata(t *testing.T) {
	rows, err := loadTrendFiltered(trendAgent, trendFilterEvals(t), mustFilters(t, "speed<>fast"))
	if err != nil {
		t.Fatal(err)
	}
	// Run 3 has no speed key, so even <> does not match it.
	if got, want := trendNames(rows), []string{"260701-100000-aaaaaaa", "260702-100000-bbbbbbb"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	if rows[0].Score != 0 || rows[1].Score != 50 {
		t.Errorf("scores = %v, %v; want 0 and 50 from the slow cases", rows[0].Score, rows[1].Score)
	}
}

func TestTrendFilterNoRemainingRunNamesAgentAndFilter(t *testing.T) {
	var buf bytes.Buffer
	err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: trendFilterEvals(t), Filters: mustFilters(t, "speed==nothing", "skill==cli")})
	if err == nil {
		t.Fatal("want an error when no run has a matching case")
	}
	for _, want := range []string{`"selftest"`, "skill==cli AND speed==nothing", "--filter"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %q on error, want nothing", buf.String())
	}
}

func TestTrendOptionsFiltersApplyToTrend(t *testing.T) {
	var buf bytes.Buffer
	if err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: trendFilterEvals(t), JSON: true, Filters: mustFilters(t, "speed==fast")}); err != nil {
		t.Fatal(err)
	}
	var rows []TrendRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Score != 100 {
		t.Fatalf("rows = %+v, want the single run scored 100", rows)
	}
}
