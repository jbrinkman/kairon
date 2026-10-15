package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Execution-level tests for --repeat (issue #321). The verdict arithmetic is
// covered in repeat_test.go; these drive RunWithOptions end to end with the
// stub backend.

const judgeRepeatCase = "check-judge-repeat"

// runsByCase groups the run numbers recorded per case name, sorted.
func runsByCase(res AgentResult) map[string][]int {
	m := map[string][]int{}
	for _, c := range res.Cases {
		m[c.CaseName] = append(m[c.CaseName], c.Run)
	}
	for _, runs := range m {
		sort.Ints(runs)
	}
	return m
}

// runStubRepeat runs agent (optionally one testcase) against a fresh copy of
// the fixtures, returning console output, the error, the run directory and the
// parsed summary and agent result.
func runStubRepeat(t *testing.T, agent, testcase string, opts RunOptions) (out, runDir string, sum Summary, res AgentResult, err error) {
	t.Helper()
	evalsDir := copySelfTestEvals(t)
	return runStubRepeatIn(t, evalsDir, agent, testcase, opts)
}

func runStubRepeatIn(t *testing.T, evalsDir, agent, testcase string, opts RunOptions) (out, runDir string, sum Summary, res AgentResult, err error) {
	t.Helper()
	out, err = runStubCapturing(t, evalsDir, agent, testcase, opts)
	runDir = onlyRunDir(t, evalsDir)
	readJSON(t, filepath.Join(runDir, "summary.json"), &sum)
	readJSON(t, filepath.Join(runDir, agent+".json"), &res)
	return
}

func TestRepeatExecutionSelftestRecordsEveryRunInFreshWorkspace(t *testing.T) {
	_, _, sum, res, err := runStubRepeat(t, "selftest", "", RunOptions{Repeat: 3, KeepWorkspaces: true})
	if err != nil {
		t.Fatalf("selftest --repeat 3 failed: %v", err)
	}
	for _, c := range res.Cases {
		cleanupWorkspace(t, c)
	}

	if res.Repeat != 3 {
		t.Errorf("AgentResult.Repeat = %d, want 3", res.Repeat)
	}
	runs := runsByCase(res)
	if len(runs) == 0 {
		t.Fatal("no cases recorded")
	}
	for name, got := range runs {
		if !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Errorf("case %s runs = %v, want [1 2 3]", name, got)
		}
	}
	if want := len(runs) * 3; len(res.Cases) != want {
		t.Errorf("recorded %d results, want %d (3 per case)", len(res.Cases), want)
	}
	if v := sum.AgentVerdicts["selftest"]; v.Repeat != 3 || len(v.RepeatScores) != 3 || v.CasesTotal != len(runs) {
		t.Errorf("summary verdict = %+v, want repeat 3, 3 repeat scores, cases_total %d", v, len(runs))
	}

	// Every run owns a distinct, existing workspace; a file the stub wrote
	// appears once per workspace, never accumulated across runs.
	seen := map[string]bool{}
	for _, c := range res.Cases {
		if c.WorkspaceDir == "" {
			t.Errorf("%s run %d has no workspace_dir", c.CaseName, c.Run)
			continue
		}
		if seen[c.WorkspaceDir] {
			t.Errorf("%s run %d reuses workspace %s", c.CaseName, c.Run, c.WorkspaceDir)
		}
		seen[c.WorkspaceDir] = true
		if st, statErr := os.Stat(c.WorkspaceDir); statErr != nil || !st.IsDir() {
			t.Errorf("%s run %d workspace %s does not exist (%v)", c.CaseName, c.Run, c.WorkspaceDir, statErr)
		}
		if c.CaseName != markerCase {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(c.WorkspaceDir, "marker.txt"))
		if readErr != nil {
			t.Errorf("%s run %d: %v", c.CaseName, c.Run, readErr)
		} else if string(data) != "hi\n" {
			t.Errorf("%s run %d marker.txt = %q, want %q (written once)", c.CaseName, c.Run, data, "hi\n")
		}
	}
}

func TestRepeatExecutionJudgeCaseShowsSpread(t *testing.T) {
	out, _, sum, res, err := runStubRepeat(t, "selftest-fail", judgeRepeatCase, RunOptions{Repeat: 2})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed (mean 50 < 95)", err)
	}
	if len(res.Cases) != 2 {
		t.Fatalf("recorded %d results, want 2", len(res.Cases))
	}
	wantDetail := []string{"judge answered yes", "judge answered no"}
	wantScore := []int{1, 0}
	for i, c := range res.Cases {
		if c.Run != i+1 {
			t.Errorf("result %d run = %d, want %d", i, c.Run, i+1)
		}
		sc := checkedScore(t, c)
		if sc.Score != wantScore[i] || sc.MaxScore != 1 {
			t.Errorf("run %d score = %d/%d, want %d/1", c.Run, sc.Score, sc.MaxScore, wantScore[i])
		}
		jc := judgeChecks(sc)
		if len(jc) != 1 || !strings.Contains(jc[0].Detail, wantDetail[i]) {
			t.Errorf("run %d judge checks = %+v, want detail containing %q", c.Run, jc, wantDetail[i])
		}
	}

	v, ok := sum.AgentVerdicts["selftest-fail"]
	if !ok {
		t.Fatal("summary has no selftest-fail verdict")
	}
	if v.Repeat != 2 || v.ScoreMin != 0 || v.ScoreMax != 100 || v.Score != 50 || v.Passed ||
		!reflect.DeepEqual(v.RepeatScores, []float64{100, 0}) {
		t.Errorf("verdict = %+v, want repeat 2, min 0, max 100, score 50, repeat_scores [100 0], passed false", v)
	}
	lines := verdictLines(out)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL selftest-fail:") || !strings.Contains(lines[0], "min 0.0%, max 100.0%") {
		t.Errorf("verdict lines = %q, want one FAIL line with the min/max suffix", lines)
	}
}

func TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals(t *testing.T) {
	_, _, sum, res, err := runStubRepeat(t, "selftest-fail", "", RunOptions{Repeat: 2})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	v := sum.AgentVerdicts["selftest-fail"]
	if !(v.ScoreMin < v.ScoreMax) {
		t.Fatalf("score_min %v, score_max %v: want a visible spread", v.ScoreMin, v.ScoreMax)
	}

	// Recompute per-repeat totals from <agent>.json alone.
	score := map[int]int{}
	maxes := map[int]int{}
	for _, c := range res.Cases {
		s, m := caseTotals(c)
		score[c.Run] += s
		maxes[c.Run] += m
	}
	var per []float64
	for run := 1; run <= 2; run++ {
		if maxes[run] == 0 {
			t.Fatalf("run %d has no scored criteria", run)
		}
		per = append(per, float64(score[run])*100/float64(maxes[run]))
	}
	if !reflect.DeepEqual(v.RepeatScores, per) {
		t.Errorf("repeat_scores = %v, recomputed %v", v.RepeatScores, per)
	}
	wantMin, wantMax := math.Min(per[0], per[1]), math.Max(per[0], per[1])
	if v.ScoreMin != wantMin || v.ScoreMax != wantMax {
		t.Errorf("min/max = %v/%v, want %v/%v", v.ScoreMin, v.ScoreMax, wantMin, wantMax)
	}
	if mean := (per[0] + per[1]) / 2; math.Abs(v.Score-mean) > 1e-9 {
		t.Errorf("score = %v, want mean %v of the per-repeat totals", v.Score, mean)
	}
}

// TestRepeatExecutionMeanDecidesThePass: repeats scoring 100 and 80 against
// the 95 threshold fail, although the best repeat is above it.
func TestRepeatExecutionMeanDecidesThePass(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	var b strings.Builder
	b.WriteString("name: check-judge-mean\ndescription: \"five judge checks: all yes, then four yes and one no\"\nagent: selftest-fail\ninput: |\n  Review the work.\nexpected_output: |\n  A short report.\nstub:\n  judge: [yes, yes, yes, yes, yes, yes, yes, yes, yes, no]\n  turns:\n    - response: |\n        ## Checks\n\n        done\n\n        ### Details\n\n        done\nchecks:\n")
	for range 5 {
		b.WriteString("  - criterion: structural_completeness\n    type: judge\n    question: \"Did it finish?\"\n")
	}
	if err := os.WriteFile(filepath.Join(evalsDir, "cases", "selftest-fail", "check-judge-mean.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, sum, _, err := runStubRepeatIn(t, evalsDir, "selftest-fail", "check-judge-mean", RunOptions{Repeat: 2})
	if !errors.Is(err, ErrThresholdFailed) {
		t.Fatalf("err = %v, want ErrThresholdFailed", err)
	}
	v := sum.AgentVerdicts["selftest-fail"]
	if v.ScoreMax < v.Threshold || v.Score >= v.Threshold || v.Passed {
		t.Errorf("verdict = %+v, want max >= threshold, mean < threshold, passed false", v)
	}
	if !reflect.DeepEqual(v.RepeatScores, []float64{100, 80}) || v.Score != 90 {
		t.Errorf("repeat_scores/score = %v/%v, want [100 80]/90", v.RepeatScores, v.Score)
	}
}

func TestRepeatExecutionUnsetAndOneAreUnchanged(t *testing.T) {
	var outs [2]string
	for i, repeat := range []int{0, 1} {
		out, _, sum, res, err := runStubRepeat(t, "selftest", "", RunOptions{Repeat: repeat})
		if err != nil {
			t.Fatalf("Repeat %d: %v", repeat, err)
		}
		outs[i] = out
		if res.Repeat != 1 {
			t.Errorf("Repeat %d: AgentResult.Repeat = %d, want 1", repeat, res.Repeat)
		}
		for _, c := range res.Cases {
			if c.Run != 1 {
				t.Errorf("Repeat %d: %s run = %d, want 1", repeat, c.CaseName, c.Run)
			}
		}
		v := sum.AgentVerdicts["selftest"]
		if v.Repeat != 1 || v.ScoreMin != v.Score || v.ScoreMax != v.Score || !reflect.DeepEqual(v.RepeatScores, []float64{v.Score}) {
			t.Errorf("Repeat %d: verdict = %+v, want repeat 1 and min == max == score", repeat, v)
		}
		lines := verdictLines(out)
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "PASS selftest: 100.0% (threshold 95.0%), 0/") || strings.Contains(lines[0], "[repeat") {
			t.Errorf("Repeat %d: verdict lines = %q, want the plain PASS line without a repeat suffix", repeat, lines)
		}
		if strings.Contains(out, "run 1/") {
			t.Errorf("Repeat %d: console output carries per-run lines:\n%s", repeat, out)
		}
	}
	if strip(outs[0]) != strip(outs[1]) {
		t.Errorf("Repeat 0 and Repeat 1 console output differ:\n--- 0 ---\n%s\n--- 1 ---\n%s", outs[0], outs[1])
	}
}

// strip removes the lines of console output that legitimately differ between
// two runs (paths, timings, timestamps), keeping the case and verdict lines.
func strip(out string) string {
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "[") && strings.Contains(l, "/") && strings.Contains(l, "]") ||
			strings.HasPrefix(l, "PASS ") || strings.HasPrefix(l, "FAIL ") {
			// "   [i/M] name ... → running agent... → evaluating... ✅ 100% (threshold: 95%)"
			if i := strings.Index(l, "(workspace:"); i >= 0 {
				l = l[:i]
			}
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

func TestRepeatExecutionRepeatMultipliesCost(t *testing.T) {
	_, _, sum1, _, err := runStubRepeat(t, "selftest", "", RunOptions{Repeat: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, _, sum3, _, err := runStubRepeat(t, "selftest", "", RunOptions{Repeat: 3})
	if err != nil {
		t.Fatal(err)
	}
	c1, c3 := sum1.TotalCost, sum3.TotalCost
	if c1.TokensIn == 0 {
		t.Fatal("repeat 1 recorded no tokens; the comparison is meaningless")
	}
	if c3.TokensIn != 3*c1.TokensIn || c3.TokensOut != 3*c1.TokensOut {
		t.Errorf("tokens at repeat 3 = %d/%d, want 3x %d/%d", c3.TokensIn, c3.TokensOut, c1.TokensIn, c1.TokensOut)
	}
	if math.Abs(c3.EstimatedUSD-3*c1.EstimatedUSD) > 1e-9 {
		t.Errorf("cost at repeat 3 = %v, want 3x %v", c3.EstimatedUSD, c1.EstimatedUSD)
	}
	v1, v3 := sum1.AgentVerdicts["selftest"].Cost, sum3.AgentVerdicts["selftest"].Cost
	if v3.TokensIn != 3*v1.TokensIn || math.Abs(v3.EstimatedUSD-3*v1.EstimatedUSD) > 1e-9 {
		t.Errorf("verdict cost at repeat 3 = %+v, want 3x %+v", v3, v1)
	}
}

func TestRepeatExecutionLoadsCasesOncePerRun(t *testing.T) {
	// The stub.judge cursor lives on the loaded TestCase, so the [yes, no]
	// sequence of check-judge-repeat only continues across repeats when the
	// cases are loaded once and reused. Three repeats consume yes, no, then run
	// past the end of the list and keep the last answer.
	_, _, _, res, _ := runStubRepeat(t, "selftest-fail", judgeRepeatCase, RunOptions{Repeat: 3})
	var got []int
	for _, c := range res.Cases {
		s, _ := caseTotals(c)
		got = append(got, s)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 0 {
		t.Errorf("per-run scores = %v, want it to start 1, 0 (cursor continues, not restarts)", got)
	}
}

func TestRepeatExecutionRunOptionsValidation(t *testing.T) {
	tests := []struct {
		name string
		opts RunOptions
		want string
	}{
		{"negative repeat", RunOptions{Repeat: -1}, "repeat"},
		{"repeat with list", RunOptions{Repeat: 2, List: true}, "repeat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evalsDir := filepath.Join(t.TempDir(), "evals")
			copyFixturesTo(t, evalsDir)
			opts := tt.opts
			opts.Backend, opts.EvalsDir, opts.NoSandbox = "stub", evalsDir, true
			t.Cleanup(resetConfig)
			err := RunWithOptions("selftest", "", opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want an error mentioning %q", err, tt.want)
			}
			if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
				t.Errorf("results directory was created (stat err = %v)", statErr)
			}
		})
	}
}

// TestRepeatExecutionRejectsRepeatWithCleanupAndPerf is separate because both
// modes act on process-wide state (the container registry, the performance
// investigation) when they are not rejected.
func TestRepeatExecutionRejectsRepeatWithCleanupAndPerf(t *testing.T) {
	for name, opts := range map[string]RunOptions{
		"cleanup": {Repeat: 2, Cleanup: true},
		"perf":    {Repeat: 2, Perf: true},
	} {
		t.Run(name, func(t *testing.T) {
			evalsDir := filepath.Join(t.TempDir(), "evals")
			copyFixturesTo(t, evalsDir)
			opts.Backend, opts.EvalsDir, opts.NoSandbox = "stub", evalsDir, true
			t.Cleanup(resetConfig)
			err := RunWithOptions("selftest", "", opts)
			if err == nil || !strings.Contains(err.Error(), "repeat") {
				t.Fatalf("err = %v, want an error mentioning repeat", err)
			}
			if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
				t.Errorf("results directory was created (stat err = %v)", statErr)
			}
		})
	}
}

// interruptAfterFirstCase rewrites the saved <agent>.json of a finished run so
// that it looks interrupted after its first case (all of that case's runs are
// kept), and restores the .progress marker. It returns the kept results.
func interruptAfterFirstCase(t *testing.T, runDir, agent string, mutate func(*AgentResult)) []CaseResult {
	t.Helper()
	path := filepath.Join(runDir, agent+".json")
	var res AgentResult
	readJSON(t, path, &res)
	first := res.Cases[0].CaseName
	var kept []CaseResult
	for _, c := range res.Cases {
		if c.CaseName == first {
			kept = append(kept, c)
		}
	}
	res.Cases = kept
	if mutate != nil {
		mutate(&res)
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, ".progress"), []byte("in-progress"), 0o644); err != nil {
		t.Fatal(err)
	}
	return kept
}

func TestRepeatExecutionResumeSkipsCompletedCaseWithoutDuplicating(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	if _, _, _, _, err := runStubRepeatIn(t, evalsDir, "selftest", "", RunOptions{Repeat: 2}); err != nil {
		t.Fatal(err)
	}
	runDir := onlyRunDir(t, evalsDir)
	kept := interruptAfterFirstCase(t, runDir, "selftest", nil)
	if len(kept) != 2 {
		t.Fatalf("kept %d results of the first case, want 2", len(kept))
	}

	out, err := runStubCapturing(t, evalsDir, "selftest", "", RunOptions{Repeat: 2, Resume: true})
	if err != nil {
		t.Fatalf("resume with the same repeat: %v\n%s", err, out)
	}
	var res AgentResult
	readJSON(t, filepath.Join(runDir, "selftest.json"), &res)
	for name, runs := range runsByCase(res) {
		if !reflect.DeepEqual(runs, []int{1, 2}) {
			t.Errorf("case %s runs = %v, want [1 2] (no duplicates)", name, runs)
		}
	}
	if !strings.Contains(out, "already completed") {
		t.Errorf("resume output does not report the skipped case:\n%s", out)
	}
	// The completed case was not re-run: its results (workspace dirs are
	// unique per execution) are exactly the ones that were saved.
	var got []CaseResult
	for _, c := range res.Cases {
		if c.CaseName == kept[0].CaseName {
			got = append(got, c)
		}
	}
	if len(got) != 2 || got[0].WorkspaceDir != kept[0].WorkspaceDir || got[1].WorkspaceDir != kept[1].WorkspaceDir {
		t.Errorf("completed case was re-run: workspaces %v, want the saved %v", workspaces(got), workspaces(kept))
	}
}

func workspaces(cs []CaseResult) []string {
	var dirs []string
	for _, c := range cs {
		dirs = append(dirs, c.WorkspaceDir)
	}
	return dirs
}

func TestRepeatExecutionResumeRefusesChangedRepeat(t *testing.T) {
	for _, now := range []int{1, 3} {
		evalsDir := copySelfTestEvals(t)
		if _, _, _, _, err := runStubRepeatIn(t, evalsDir, "selftest", "", RunOptions{Repeat: 2}); err != nil {
			t.Fatal(err)
		}
		runDir := onlyRunDir(t, evalsDir)
		interruptAfterFirstCase(t, runDir, "selftest", nil)
		before, _ := os.ReadFile(filepath.Join(runDir, "selftest.json"))

		_, err := runStubCapturing(t, evalsDir, "selftest", "", RunOptions{Repeat: now, Resume: true})
		if err == nil || !strings.Contains(err.Error(), "repeat changed") ||
			!strings.Contains(err.Error(), "repeat=2") {
			t.Fatalf("resume with Repeat %d: err = %v, want a 'repeat changed' refusal naming repeat=2", now, err)
		}
		after, _ := os.ReadFile(filepath.Join(runDir, "selftest.json"))
		if !bytes.Equal(before, after) {
			t.Errorf("refused resume with Repeat %d modified selftest.json", now)
		}
	}
}

func TestRepeatExecutionLegacyResultResumesAsRepeatOne(t *testing.T) {
	evalsDir := copySelfTestEvals(t)
	if _, _, _, _, err := runStubRepeatIn(t, evalsDir, "selftest", "", RunOptions{}); err != nil {
		t.Fatal(err)
	}
	runDir := onlyRunDir(t, evalsDir)
	// A legacy file records neither repeat nor run.
	legacy := func(r *AgentResult) {
		r.Repeat = 0
		for i := range r.Cases {
			r.Cases[i].Run = 0
		}
	}
	interruptAfterFirstCase(t, runDir, "selftest", legacy)

	// Resuming it at repeat 2 is a change from the implied repeat 1.
	if _, err := runStubCapturing(t, evalsDir, "selftest", "", RunOptions{Repeat: 2, Resume: true}); err == nil ||
		!strings.Contains(err.Error(), "repeat changed") || !strings.Contains(err.Error(), "repeat=1") {
		t.Fatalf("legacy resume at repeat 2: err = %v, want a 'repeat changed' refusal naming repeat=1", err)
	}

	for _, repeat := range []int{0, 1} {
		interruptAfterFirstCase(t, runDir, "selftest", legacy)
		if out, err := runStubCapturing(t, evalsDir, "selftest", "", RunOptions{Repeat: repeat, Resume: true}); err != nil {
			t.Fatalf("legacy resume at Repeat %d: %v\n%s", repeat, err, out)
		}
		var res AgentResult
		readJSON(t, filepath.Join(runDir, "selftest.json"), &res)
		if res.Repeat != 1 {
			t.Errorf("Repeat %d: AgentResult.Repeat = %d, want 1", repeat, res.Repeat)
		}
		for name, runs := range runsByCase(res) {
			if len(runs) != 1 {
				t.Errorf("Repeat %d: case %s has %d results, want 1", repeat, name, len(runs))
			}
		}
	}
}

func TestRepeatDiffAgainstRepeatOneRunUsesRunOneOutput(t *testing.T) {
	evalsA := copySelfTestEvals(t)
	if _, _, _, _, err := runStubRepeatIn(t, evalsA, "selftest", "", RunOptions{Repeat: 3}); err != nil {
		t.Fatal(err)
	}
	dirA := onlyRunDir(t, evalsA)
	evalsB := copySelfTestEvals(t)
	if _, _, _, _, err := runStubRepeatIn(t, evalsB, "selftest", "", RunOptions{Repeat: 1}); err != nil {
		t.Fatal(err)
	}
	dirB := onlyRunDir(t, evalsB)
	// Bring both runs under one results directory under distinct names.
	if err := os.Rename(dirB, filepath.Join(evalsA, "results", "run-b")); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(resetConfig)
	if err := configure(RunOptions{Backend: "stub", EvalsDir: evalsA}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := diffTo(&buf, filepath.Base(dirA), "run-b"); err != nil {
		t.Fatalf("diff of a repeat-3 against a repeat-1 run: %v", err)
	}
}

func TestRepeatShowOutputChangesKeepsRunOneOutput(t *testing.T) {
	a := AgentResult{Cases: []CaseResult{
		{CaseName: "c", Run: 1, ActualOutput: "alpha"},
		{CaseName: "c", Run: 2, ActualOutput: "beta"},
		{CaseName: "c", Run: 3, ActualOutput: "gamma"},
	}}
	b := AgentResult{Cases: []CaseResult{{CaseName: "c", Run: 1, ActualOutput: "alpha"}}}

	// Run 1 of A equals B's only output: nothing changed. Keeping the last
	// output (gamma) would report a change.
	var buf bytes.Buffer
	showOutputChanges(&buf, a, b)
	if buf.Len() != 0 {
		t.Errorf("run-1 outputs are equal but a change was reported:\n%s", buf.String())
	}

	buf.Reset()
	showOutputChanges(&buf, b, a)
	if buf.Len() != 0 {
		t.Errorf("run-1 outputs are equal but a change was reported (swapped):\n%s", buf.String())
	}

	// A legacy result (Run 0) is run 1.
	buf.Reset()
	showOutputChanges(&buf, AgentResult{Cases: []CaseResult{{CaseName: "c", ActualOutput: "alpha"}}}, a)
	if buf.Len() != 0 {
		t.Errorf("legacy run-0 output equals run 1 but a change was reported:\n%s", buf.String())
	}
}
