package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// runWithSummary runs agent with the stub backend, natively, against a copy of
// the checked-in fixtures and returns the saved <agent>.json and summary.json
// together with the evals dir used.
func runWithSummary(t *testing.T, agent string) (AgentResult, Summary, string) {
	t.Helper()
	t.Cleanup(resetConfig)
	evalsDir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, evalsDir)
	resultsDir := filepath.Join(evalsDir, "results")
	before := snapshotDirs(t, resultsDir)
	res := runEvalIn(t, evalsDir, agent, RunOptions{NoSandbox: true})
	added := newRunDirs(t, resultsDir, before)
	if len(added) != 1 {
		t.Fatalf("run created %d result dirs, want 1: %v", len(added), added)
	}
	data, err := os.ReadFile(filepath.Join(resultsDir, added[0], "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sum Summary
	if err := json.Unmarshal(data, &sum); err != nil {
		t.Fatalf("parsing summary.json: %v", err)
	}
	return res, sum, evalsDir
}

// AC1: the selftest verdict lists every non-cost criterion with values equal to
// the sums over the saved cases.
func TestSelfTestVerdictListsCriteria(t *testing.T) {
	res, sum, _ := runWithSummary(t, "selftest")
	v, ok := sum.AgentVerdicts["selftest"]
	if !ok {
		t.Fatal("summary.json has no agent_verdicts.selftest")
	}

	type pts struct{ score, max int }
	want := map[string]pts{}
	for _, c := range res.Cases {
		for _, s := range c.Scores {
			p := want[s.Name]
			if !s.Skipped {
				p.score += s.Score
			}
			p.max += s.MaxScore
			want[s.Name] = p
		}
	}
	if len(v.Criteria) != len(want) {
		t.Fatalf("criteria = %+v, want names %v", v.Criteria, want)
	}
	got := map[string]CriterionTotal{}
	for _, c := range v.Criteria {
		got[c.Name] = c
	}
	for _, name := range []string{"structural_completeness", "clarity"} {
		c, ok := got[name]
		if !ok {
			t.Fatalf("criteria %+v lack %s", v.Criteria, name)
		}
		if w := want[name]; c.Score != w.score || c.MaxScore != w.max {
			t.Errorf("%s = %d/%d, want %d/%d", name, c.Score, c.MaxScore, w.score, w.max)
		}
		if c.MaxScore == 0 || c.Percent != float64(c.Score)*100/float64(c.MaxScore) {
			t.Errorf("%s percent = %v for %d/%d", name, c.Percent, c.Score, c.MaxScore)
		}
	}
	if _, ok := got["cost_efficiency"]; ok {
		t.Error("cost criterion cost_efficiency must not be listed")
	}

	// AC3: the criteria are additive. The agent score and verdict are as before.
	if !v.Passed {
		t.Errorf("selftest verdict = %+v, want passed", v)
	}
	score, max := agentScoreTotals(res)
	var cscore, cmax int
	for _, c := range v.Criteria {
		cscore += c.Score
		cmax += c.MaxScore
	}
	if float64(cscore) != score || float64(cmax) != max {
		t.Errorf("criteria sum %d/%d != agent totals %v/%v", cscore, cmax, score, max)
	}
	if want := score / max; sum.AgentScores["selftest"] != want {
		t.Errorf("agent_scores = %v, want %v", sum.AgentScores["selftest"], want)
	}
}

// AC4: under checks_only a criterion's maximum is its number of checks across
// the cases, counted here from the loaded case files.
func TestSelfTestChecksCriterionMaxEqualsCheckCount(t *testing.T) {
	_, sum, evalsDir := runWithSummary(t, "selftest-checks")
	v := sum.AgentVerdicts["selftest-checks"]

	entries, err := os.ReadDir(filepath.Join(evalsDir, "cases", "selftest-checks"))
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]int{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(evalsDir, "cases", "selftest-checks", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var tc TestCase
		if err := yaml.Unmarshal(data, &tc); err != nil {
			t.Fatal(err)
		}
		for _, c := range tc.Checks {
			checks[c.Criterion]++
		}
	}
	if checks["file_written"] != 2 || checks["report_marker"] != 1 {
		t.Fatalf("counted checks %v, want file_written 2 and report_marker 1", checks)
	}
	if len(v.Criteria) != len(checks) {
		t.Fatalf("criteria = %+v, want one per checked criterion %v", v.Criteria, checks)
	}
	for _, c := range v.Criteria {
		if c.MaxScore != checks[c.Name] {
			t.Errorf("%s max_score = %d, want its %d checks", c.Name, c.MaxScore, checks[c.Name])
		}
		if c.Score != c.MaxScore || c.Percent != 100 {
			t.Errorf("%s = %d/%d (%v%%), want all checks passing", c.Name, c.Score, c.MaxScore, c.Percent)
		}
	}
}
