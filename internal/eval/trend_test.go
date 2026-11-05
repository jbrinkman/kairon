package eval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	trendAgent   = "selftest"
	trendFixture = "testdata/trend"
)

// trendNames returns the run names of rows, in order.
func trendNames(rows []TrendRow) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Run
	}
	return names
}

// trendWrite writes content to path, creating parent directories.
func trendWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// trendRun writes a run directory under evalsDir/results holding a summary with
// the given agent_verdicts JSON object body for agent selftest and an empty
// selftest.json (no case results).
func trendRun(t *testing.T, evalsDir, run, verdict string) {
	t.Helper()
	dir := filepath.Join(evalsDir, "results", run)
	trendWrite(t, filepath.Join(dir, "summary.json"), `{"agent_verdicts": {"selftest": `+verdict+`}}`)
	trendWrite(t, filepath.Join(dir, "selftest.json"), `{"agent": "selftest", "cases": []}`)
}

func TestTrendFixtureOrder(t *testing.T) {
	rows, err := LoadTrend(trendAgent, trendFixture)
	if err != nil {
		t.Fatalf("LoadTrend: %v", err)
	}
	want := []string{
		"260701-100000-aaaaaaa",
		"260702-100000-bbbbbbb",
		"260703-100000-ccccccc",
		"260704-100000-ddddddd",
	}
	if got := trendNames(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %v, want %v (the run of agent \"other\" must be excluded)", got, want)
	}
	for i, r := range rows {
		wantDate := "2026-07-0" + string(rune('1'+i)) + "T10:00:00Z"
		if r.Date != wantDate {
			t.Errorf("%s: date = %q, want %q", r.Run, r.Date, wantDate)
		}
	}
	if rows[0].Score != 80 || rows[3].Score != 87.5 {
		t.Errorf("scores = %v, %v; want 80 and 87.5", rows[0].Score, rows[3].Score)
	}
	for i, r := range rows {
		if r.Complete == nil {
			t.Fatalf("%s: Complete is nil, want a recorded value", r.Run)
		}
		if want := i != 2; *r.Complete != want {
			t.Errorf("%s: complete = %v, want %v", r.Run, *r.Complete, want)
		}
	}
}

func TestTrendFixtureCoverage(t *testing.T) {
	rec := loadRunRecord(filepath.Join(trendFixture, "results"), "260703-100000-ccccccc", trendAgent)
	if !rec.hasComplete || rec.complete {
		t.Errorf("third run: hasComplete=%v complete=%v, want recorded false", rec.hasComplete, rec.complete)
	}
	if rec.caseFilter != "stub-basic" {
		t.Errorf("third run case_filter = %q, want stub-basic", rec.caseFilter)
	}
	other := loadRunRecord(filepath.Join(trendFixture, "results"), "260705-100000-eeeeeee", "other")
	if !other.hasScore {
		t.Error("fixture run 260705-100000-eeeeeee must hold a score for agent other")
	}
}

func TestTrendTextOutput(t *testing.T) {
	var buf bytes.Buffer
	if err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: trendFixture}); err != nil {
		t.Fatalf("Trend: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("output must end with a newline: %q", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want one per run (4):\n%s", len(lines), out)
	}
	want := []string{
		"260701-100000-aaaaaaa  2026-07-01T10:00:00Z  score 80.0%  success 75.0%  agent_model agent-m  judge_model judge-m  prompt aaaaaaaa  complete",
		"260702-100000-bbbbbbb  2026-07-02T10:00:00Z  score 100.0%  success 100.0%  agent_model agent-m  judge_model judge-m  prompt bbbbbbbb  complete",
		"260703-100000-ccccccc  2026-07-03T10:00:00Z  score 100.0%  success 100.0%  agent_model agent-m  judge_model judge-m  prompt cccccccc  INCOMPLETE",
		"260704-100000-ddddddd  2026-07-04T10:00:00Z  score 87.5%  success 75.0%  agent_model agent-m2  judge_model judge-m2  prompt dddddddd  complete",
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, lines[i], want[i])
		}
	}
	incomplete := 0
	for _, l := range lines {
		if strings.Contains(l, "INCOMPLETE") {
			incomplete++
		}
	}
	if incomplete != 1 || !strings.Contains(lines[2], "INCOMPLETE") {
		t.Errorf("want exactly the third line marked INCOMPLETE, got %d marked", incomplete)
	}
}

func TestTrendJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	if err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: trendFixture, JSON: true}); err != nil {
		t.Fatalf("Trend: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "[\n  {\n    \"run\"") || !strings.HasSuffix(out, "]\n") {
		t.Errorf("output is not an indented JSON array ending in a newline:\n%s", out)
	}
	var objs []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &objs); err != nil {
		t.Fatalf("output is not a JSON array of objects: %v\n%s", err, out)
	}
	if len(objs) != 4 {
		t.Fatalf("got %d objects, want 4", len(objs))
	}
	keys := []string{"run", "date", "score", "success_rate", "agent_model", "judge_model", "prompt_sha256", "complete"}
	for i, o := range objs {
		if len(o) != len(keys) {
			t.Errorf("object %d has %d keys, want %d: %v", i, len(o), len(keys), o)
		}
		for _, k := range keys {
			if _, ok := o[k]; !ok {
				t.Errorf("object %d lacks key %q", i, k)
			}
		}
	}
	if objs[0]["run"] != "260701-100000-aaaaaaa" || objs[3]["run"] != "260704-100000-ddddddd" {
		t.Errorf("objects not in date order: %v, %v", objs[0]["run"], objs[3]["run"])
	}
	if objs[0]["success_rate"] != float64(75) || objs[2]["complete"] != false {
		t.Errorf("success_rate/complete = %v / %v, want 75 / false", objs[0]["success_rate"], objs[2]["complete"])
	}
	if objs[0]["prompt_sha256"] != "aaaaaaaa" {
		t.Errorf("prompt_sha256 = %v, want the 8-character prefix", objs[0]["prompt_sha256"])
	}
}

func TestTrendUnrecordedFacts(t *testing.T) {
	dir := t.TempDir()
	// A legacy run: a verdict with a score only, no cases, no provenance.
	trendRun(t, dir, "260701-100000-aaaaaaa", `{"score": 60.0, "threshold": 70.0, "passed": false, "cases_total": 1, "cases_failed": 1, "cost": {}}`)

	rows, err := LoadTrend(trendAgent, dir)
	if err != nil {
		t.Fatalf("LoadTrend: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.SuccessRate != nil {
		t.Errorf("SuccessRate = %v, want nil (no usable attempts)", *r.SuccessRate)
	}
	if r.Complete != nil {
		t.Errorf("Complete = %v, want nil (not recorded)", *r.Complete)
	}
	if r.AgentModel != "" || r.JudgeModel != "" || r.PromptSHA256 != "" {
		t.Errorf("unrecorded strings = %q / %q / %q, want empty", r.AgentModel, r.JudgeModel, r.PromptSHA256)
	}

	var jbuf bytes.Buffer
	if err := Trend(&jbuf, trendAgent, TrendOptions{EvalsDir: dir, JSON: true}); err != nil {
		t.Fatal(err)
	}
	var objs []map[string]any
	if err := json.Unmarshal(jbuf.Bytes(), &objs); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"success_rate", "complete"} {
		if v, ok := objs[0][k]; !ok || v != nil {
			t.Errorf("%s = %v (present=%v), want null", k, v, ok)
		}
	}
	for _, k := range []string{"agent_model", "judge_model", "prompt_sha256"} {
		if objs[0][k] != "" {
			t.Errorf("%s = %v, want empty string", k, objs[0][k])
		}
	}

	var tbuf bytes.Buffer
	if err := Trend(&tbuf, trendAgent, TrendOptions{EvalsDir: dir}); err != nil {
		t.Fatal(err)
	}
	want := "260701-100000-aaaaaaa  2026-07-01T10:00:00Z  score 60.0%  success -  agent_model -  judge_model -  prompt -  completeness-unknown\n"
	if tbuf.String() != want {
		t.Errorf("text line:\n got %q\nwant %q", tbuf.String(), want)
	}
}

func TestTrendRecordedZeroSuccessRate(t *testing.T) {
	dir := t.TempDir()
	trendRun(t, dir, "260701-100000-aaaaaaa", `{"score": 10.0, "threshold": 70.0, "passed": false, "cases_total": 2, "cases_failed": 2, "attempts_total": 2, "attempts_passed": 0, "success_rate": 0, "cost": {}}`)
	rows, err := LoadTrend(trendAgent, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].SuccessRate == nil || *rows[0].SuccessRate != 0 {
		t.Errorf("SuccessRate = %v, want a recorded 0", rows[0].SuccessRate)
	}
}

func TestTrendHashOnlyRunSortsFirst(t *testing.T) {
	dir := t.TempDir()
	v := `{"score": 50.0, "threshold": 70.0, "passed": false, "cases_total": 1, "cases_failed": 1, "cost": {}}`
	trendRun(t, dir, "260702-100000-bbbbbbb", v)
	trendRun(t, dir, "abc1234", v)
	trendRun(t, dir, "260701-100000-aaaaaaa", v)
	trendRun(t, dir, "0ldhash", v)

	rows, err := LoadTrend(trendAgent, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"0ldhash", "abc1234", "260701-100000-aaaaaaa", "260702-100000-bbbbbbb"}
	if got := trendNames(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	if len(rows) < 3 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	if rows[0].Date != "" || rows[1].Date != "" {
		t.Errorf("hash-only runs have dates %q, %q; want empty", rows[0].Date, rows[1].Date)
	}
	if rows[2].Date == "" {
		t.Error("dated run has an empty date")
	}

	var buf bytes.Buffer
	if err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: dir}); err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(buf.String(), "\n", 2)[0]
	if !strings.HasPrefix(first, "0ldhash  -  score 50.0%") {
		t.Errorf("undated line = %q, want the date shown as -", first)
	}
}

func TestTrendSameTimestampSortsByName(t *testing.T) {
	dir := t.TempDir()
	v := `{"score": 50.0, "threshold": 70.0, "passed": false, "cases_total": 1, "cases_failed": 1, "cost": {}}`
	trendRun(t, dir, "260701-100000-bbbbbbb", v)
	trendRun(t, dir, "260701-100000-aaaaaaa", v)
	rows, err := LoadTrend(trendAgent, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"260701-100000-aaaaaaa", "260701-100000-bbbbbbb"}
	if got := trendNames(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}

func TestTrendSkipsNonDirsAndRunsWithoutAgent(t *testing.T) {
	dir := t.TempDir()
	v := `{"score": 50.0, "threshold": 70.0, "passed": false, "cases_total": 1, "cases_failed": 1, "cost": {}}`
	trendRun(t, dir, "260701-100000-aaaaaaa", v)
	// A plain file named like a run.
	trendWrite(t, filepath.Join(dir, "results", "260702-100000-bbbbbbb"), "not a directory")
	// A directory with no data at all.
	if err := os.MkdirAll(filepath.Join(dir, "results", "260703-100000-ccccccc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A run of another agent only.
	trendWrite(t, filepath.Join(dir, "results", "260704-100000-ddddddd", "summary.json"),
		`{"agent_verdicts": {"other": {"score": 90.0, "threshold": 70.0, "passed": true, "cases_total": 1, "cases_failed": 0, "cost": {}}}}`)
	// A stray file at the top of results.
	trendWrite(t, filepath.Join(dir, "results", "notes.txt"), "hello")

	rows, err := LoadTrend(trendAgent, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"260701-100000-aaaaaaa"}
	if got := trendNames(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}

func TestTrendMissingResultsDir(t *testing.T) {
	tests := []struct {
		name string
		// evalsDir returns the evals directory to read results from.
		evalsDir func(t *testing.T) string
		agent    string
		json     bool
		// needle must appear in the error from both Trend and LoadTrend.
		needle string
	}{
		{
			name:     "missing results dir",
			evalsDir: func(t *testing.T) string { return t.TempDir() },
			agent:    trendAgent,
			needle:   trendAgent,
		},
		{
			name: "empty results dir",
			evalsDir: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.MkdirAll(filepath.Join(dir, "results"), 0o755); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			agent:  trendAgent,
			needle: trendAgent,
		},
		{
			name:     "unknown agent text",
			evalsDir: func(*testing.T) string { return trendFixture },
			agent:    "nope",
			needle:   `"nope"`,
		},
		{
			name:     "unknown agent json",
			evalsDir: func(*testing.T) string { return trendFixture },
			agent:    "nope",
			json:     true,
			needle:   `"nope"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.evalsDir(t)
			var buf bytes.Buffer
			err := Trend(&buf, tc.agent, TrendOptions{EvalsDir: dir, JSON: tc.json})
			if err == nil {
				t.Fatal("Trend: want an error for an agent with no results")
			}
			if !strings.Contains(err.Error(), tc.needle) {
				t.Errorf("Trend error %q does not contain %s", err, tc.needle)
			}
			if buf.Len() != 0 {
				t.Errorf("wrote %q on error, want nothing", buf.String())
			}
			if _, err := LoadTrend(tc.agent, dir); err == nil {
				t.Error("LoadTrend: want an error for an agent with no results")
			} else if !strings.Contains(err.Error(), tc.needle) {
				t.Errorf("LoadTrend error %q does not contain %s", err, tc.needle)
			}
		})
	}
}

func TestTrendRejectsPathLikeAgent(t *testing.T) {
	for _, agent := range []string{"", "a/b", `a\b`, "..", "../selftest", "a..b"} {
		var buf bytes.Buffer
		if err := Trend(&buf, agent, TrendOptions{EvalsDir: trendFixture}); err == nil {
			t.Errorf("agent %q: want an error", agent)
		}
		if buf.Len() != 0 {
			t.Errorf("agent %q: wrote %q on error, want nothing", agent, buf.String())
		}
		if _, err := LoadTrend(agent, trendFixture); err == nil {
			t.Errorf("LoadTrend agent %q: want an error", agent)
		}
	}
}

func TestTrendDoesNotUsePackageConfig(t *testing.T) {
	saved := cfg
	t.Cleanup(func() { cfg = saved })
	cfg = newDefaultConfig()
	cfg.evalsDir = filepath.Join(t.TempDir(), "elsewhere")
	cfg.caseFilter = "sentinel"
	before := cfg

	var buf bytes.Buffer
	if err := Trend(&buf, trendAgent, TrendOptions{EvalsDir: trendFixture}); err != nil {
		t.Fatalf("Trend must use the explicit EvalsDir, not cfg.evalsDir: %v", err)
	}
	if got := strings.Count(buf.String(), "\n"); got != 4 {
		t.Errorf("got %d lines, want 4", got)
	}
	if !reflect.DeepEqual(cfg, before) {
		t.Error("Trend modified the package-level cfg")
	}
}
