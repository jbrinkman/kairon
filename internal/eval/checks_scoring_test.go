package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChecksScoringCharacterizationLegacySelftestCases pins the scores of the
// five pre-existing, unchecked selftest cases. It must pass before and after
// checks are integrated into scoreCase: criteria without checks are scored
// exactly as before.
func TestChecksScoringCharacterizationLegacySelftestCases(t *testing.T) {
	t.Cleanup(resetConfig)

	evalsDir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, evalsDir)
	if err := RunWithOptions("selftest", "", RunOptions{Backend: "stub", EvalsDir: evalsDir}); err != nil {
		t.Fatalf("selftest run failed: %v", err)
	}
	resultsDir := filepath.Join(evalsDir, "results")
	added := newRunDirs(t, resultsDir, map[string]bool{})
	if len(added) == 0 {
		t.Fatalf("no run directory appeared under %s", resultsDir)
	}
	res := readSelfTestResult(t, filepath.Join(resultsDir, added[0], "selftest.json"))

	for _, name := range []string{"stub-basic", "stub-usage", "stub-quoted-input", markerCase, seededCase} {
		c := caseByName(t, res, name)
		if len(c.Scores) != 2 {
			t.Fatalf("%s: scores = %+v, want 2", name, c.Scores)
		}
		sc, cl := c.Scores[0], c.Scores[1]
		if sc.Name != "structural_completeness" || sc.Score != 5 || sc.MaxScore != 5 || !sc.Deterministic ||
			sc.Skipped || sc.Reasoning != "found 2/2 expected structural elements" || sc.Checks != nil {
			t.Errorf("%s: structural_completeness = %+v", name, sc)
		}
		if cl.Name != "clarity" || cl.Score != 5 || cl.MaxScore != 5 || cl.Deterministic ||
			cl.Skipped || cl.Checks != nil {
			t.Errorf("%s: clarity = %+v", name, cl)
		}
	}
}

// checkedRubric has a deterministic and an LLM-judged criterion.
func checkedRubric() Rubric {
	return Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-5"},
	}}
}

func scoreByName(t *testing.T, cr CaseResult, name string) CriterionScore {
	t.Helper()
	for _, s := range cr.Scores {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no score for %q in %+v", name, cr.Scores)
	return CriterionScore{}
}

func TestChecksScoringCheckedCriterionScoresPassedOverTotal(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	dir := t.TempDir()
	writeEvalFile(t, dir, "a.txt", "hello\n")
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "clarity", Type: CheckFileExists, Path: "a.txt"},
		{Criterion: "clarity", Type: CheckFileContains, Path: "a.txt", Pattern: "hello"},
		{Criterion: "clarity", Type: CheckOutputContains, Pattern: "## "},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S\ntext", WorkspaceDir: dir}
	scoreCase(checkedRubric(), tc, &cr)

	clarity := scoreByName(t, cr, "clarity")
	if clarity.Score != 3 || clarity.MaxScore != 3 || !clarity.Deterministic || clarity.Skipped {
		t.Errorf("clarity = %+v, want 3/3 deterministic", clarity)
	}
	if clarity.Reasoning != "3/3 checks passed" {
		t.Errorf("reasoning = %q", clarity.Reasoning)
	}
	if len(clarity.Checks) != 3 {
		t.Fatalf("checks = %+v, want 3 results", clarity.Checks)
	}
	// The unchecked criterion keeps its legacy scoring.
	sc := scoreByName(t, cr, "structural_completeness")
	if sc.Score != 5 || sc.MaxScore != 5 || sc.Reasoning != "found 2/2 expected structural elements" || sc.Checks != nil {
		t.Errorf("structural_completeness = %+v, want unchanged legacy score", sc)
	}
	// No judge call for the checked LLM criterion.
	if len(cr.Calls) != 0 {
		t.Errorf("calls = %+v, want none", cr.Calls)
	}
	if cr.JudgeCost != (CostInfo{}) {
		t.Errorf("JudgeCost = %+v, want zero", cr.JudgeCost)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestChecksScoringPartialNamesFailedCheck(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	dir := t.TempDir()
	writeEvalFile(t, dir, "marker.txt", "m\n")
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "structural_completeness", Type: CheckFileExists, Path: "marker.txt"},
		{Criterion: "structural_completeness", Type: CheckFileExists, Path: "missing.txt"},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S", WorkspaceDir: dir}
	scoreCase(checkedRubric(), tc, &cr)

	s := scoreByName(t, cr, "structural_completeness")
	if s.Score != 1 || s.MaxScore != 2 || !s.Deterministic {
		t.Fatalf("score = %+v, want 1/2 deterministic", s)
	}
	for _, want := range []string{"1/2 checks passed", "#2 file_exists path=missing.txt", "file does not exist"} {
		if !strings.Contains(s.Reasoning, want) {
			t.Errorf("reasoning %q missing %q", s.Reasoning, want)
		}
	}
	if strings.Contains(s.Reasoning, "#1") {
		t.Errorf("reasoning %q names the passing check", s.Reasoning)
	}
	if len(s.Checks) != 2 || !s.Checks[0].Passed || s.Checks[1].Passed {
		t.Errorf("checks = %+v, want [pass, fail]", s.Checks)
	}
}

func TestChecksScoringReasoningTruncatesLongDetail(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "structural_completeness", Type: CheckOutputContains, Pattern: "zzz"},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S", WorkspaceDir: t.TempDir()}
	// Replace the detail with an overlong one via the formatter.
	got := checksReasoning([]CheckResult{{Index: 1, Type: CheckCommand, Label: "#1 command \"x\"", Detail: strings.Repeat("d", 500)}})
	if !strings.Contains(got, strings.Repeat("d", 200)+"…") || strings.Contains(got, strings.Repeat("d", 201)) {
		t.Errorf("reasoning detail not truncated to 200 chars: %q", got)
	}
	scoreCase(checkedRubric(), tc, &cr)
	if s := scoreByName(t, cr, "structural_completeness"); s.Score != 0 || s.MaxScore != 1 {
		t.Errorf("score = %+v", s)
	}
}

func TestChecksScoringUnknownCriterionIsVisible(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "no_such_criterion", Type: CheckOutputContains, Pattern: "## "},
		{Criterion: "no_such_criterion", Type: CheckOutputContains, Pattern: "###"},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)

	s := scoreByName(t, cr, "no_such_criterion")
	if s.Score != 0 || s.MaxScore != 2 || s.Skipped {
		t.Errorf("score = %+v, want visible 0/2", s)
	}
	if !strings.Contains(s.Reasoning, `criterion "no_such_criterion" is not in the rubric`) {
		t.Errorf("reasoning = %q", s.Reasoning)
	}
	if got := len(cr.Scores); got != 3 {
		t.Errorf("scores = %+v, want legacy 2 + the extra", cr.Scores)
	}
}

func TestChecksScoringNoOutputOrWorkspaceScoresZero(t *testing.T) {
	for name, cr := range map[string]CaseResult{
		"empty output": {ActualOutput: "", WorkspaceDir: "WS"},
		"no workspace": {ActualOutput: "## H\n### S", WorkspaceDir: ""},
	} {
		t.Run(name, func(t *testing.T) {
			chdirTemp(t)
			useStubBackend(t)

			dir := t.TempDir()
			if cr.WorkspaceDir == "WS" {
				cr.WorkspaceDir = dir
			}
			tc := TestCase{Name: "c", Checks: []Check{
				{Criterion: "clarity", Type: CheckCommand, Run: "touch " + filepath.Join(dir, "ran.txt")},
				{Criterion: "clarity", Type: CheckFileAbsent, Path: "x"},
			}}
			scoreCase(checkedRubric(), tc, &cr)

			s := scoreByName(t, cr, "clarity")
			if s.Score != 0 || s.MaxScore != 2 || s.Skipped || !s.Deterministic {
				t.Errorf("clarity = %+v, want non-skipped 0/2", s)
			}
			if s.Reasoning != "agent produced no output; checks not run" {
				t.Errorf("reasoning = %q", s.Reasoning)
			}
			if _, err := os.Stat(filepath.Join(dir, "ran.txt")); err == nil {
				t.Error("command check was executed")
			}
		})
	}
}

func TestChecksScoringNoCommandRunWithoutOutput(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	orig := runCheckCommand
	t.Cleanup(func() { runCheckCommand = orig })
	ran := false
	runCheckCommand = func(dir, run string) commandOutcome {
		ran = true
		return commandOutcome{}
	}
	tc := TestCase{Name: "c", Checks: []Check{{Criterion: "clarity", Type: CheckCommand, Run: "true"}}}
	cr := CaseResult{WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)
	if ran {
		t.Error("runCheckCommand called for a case with no output")
	}
}

func TestChecksScoringReadsGHLog(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	dir := t.TempDir()
	writeEvalFile(t, dir, ".eval/gh.log", "gh issue create --title t\n")
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "clarity", Type: CheckGHLogContains, Pattern: `(?m)^gh issue create`},
		{Criterion: "clarity", Type: CheckGHLogNotContains, Pattern: `gh pr merge`},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S", WorkspaceDir: dir}
	scoreCase(checkedRubric(), tc, &cr)
	if s := scoreByName(t, cr, "clarity"); s.Score != 2 || s.MaxScore != 2 {
		t.Errorf("clarity = %+v, want 2/2", s)
	}

	// A missing log is empty: gh_log_contains fails, gh_log_not_contains passes.
	cr2 := CaseResult{ActualOutput: "## H\n### S", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr2)
	if s := scoreByName(t, cr2, "clarity"); s.Score != 1 || s.MaxScore != 2 {
		t.Errorf("clarity (no log) = %+v, want 1/2", s)
	}
}

func TestChecksScoringUsesFixtureCommitAsBase(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	ws, err := newCaseWorkspace(TestCase{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Remove() })
	if ws.BaseCommit == "" {
		t.Fatal("caseWorkspace.BaseCommit is empty")
	}
	// The agent commits a change; HEAD now equals the change.
	writeEvalFile(t, ws.Dir, "agent.txt", "x\n")
	chkGit(t, ws.Dir, "add", "-A")
	chkGit(t, ws.Dir, "-c", "user.name=a", "-c", "user.email=a@example.com", "commit", "-q", "-m", "agent")

	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{}},
	}}
	cr := CaseResult{ActualOutput: "## H", WorkspaceDir: ws.Dir, baseCommit: ws.BaseCommit}
	scoreCase(checkedRubric(), tc, &cr)
	s := scoreByName(t, cr, "clarity")
	if s.Score != 0 || s.MaxScore != 1 || !strings.Contains(s.Reasoning, "agent.txt") {
		t.Errorf("clarity = %+v, want failed changed_files naming agent.txt", s)
	}
}

func TestChecksScoringExecuteCaseRecordsBaseCommit(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	orig := scoreCaseFn
	t.Cleanup(func() { scoreCaseFn = orig })
	var seen CaseResult
	scoreCaseFn = func(r Rubric, tc TestCase, cr *CaseResult) {
		seen = *cr
		orig(r, tc, cr)
	}
	tc := TestCase{Name: "c", Agent: "a", Input: "hi"}
	_ = executeCase(checkedRubric(), tc, nil, &strings.Builder{}, false)
	if seen.baseCommit == "" {
		t.Error("executeCase did not set CaseResult.baseCommit before scoring")
	}
}

func TestPrintCaseResultFailedChecksListed(t *testing.T) {
	cr := CaseResult{ActualOutput: "out", Scores: []CriterionScore{{
		Name: "c", Score: 1, MaxScore: 2, Deterministic: true,
		Checks: []CheckResult{
			{Index: 1, Type: CheckFileExists, Label: "#1 file_exists path=ok.txt", Passed: true},
			{Index: 2, Type: CheckFileExists, Label: "#2 file_exists path=missing.txt", Detail: "file does not exist"},
		},
	}}}
	var sb strings.Builder
	printCaseResult(&sb, TestCase{Name: "c"}, Rubric{}, cr)
	want := " ❌ 50% (threshold: 95%)\n      ✗ #2 file_exists path=missing.txt: file does not exist\n"
	if sb.String() != want {
		t.Errorf("output = %q, want %q", sb.String(), want)
	}

	// Failed-check lines appear even when the percentage meets the threshold.
	cr.Scores = append(cr.Scores, CriterionScore{Name: "big", Score: 98, MaxScore: 98})
	sb.Reset()
	printCaseResult(&sb, TestCase{Name: "c", MinScore: floatPtr(50)}, Rubric{}, cr)
	want = " ✅ 99% (threshold: 50%)\n      ✗ #2 file_exists path=missing.txt: file does not exist\n"
	if sb.String() != want {
		t.Errorf("output = %q, want %q", sb.String(), want)
	}
}

func TestPrintCaseResultFailedCheckWithoutDetail(t *testing.T) {
	cr := CaseResult{ActualOutput: "out", Scores: []CriterionScore{{
		Name: "c", Score: 0, MaxScore: 1,
		Checks: []CheckResult{{Index: 1, Type: CheckCommand, Label: `#1 command "x"`}},
	}}}
	var sb strings.Builder
	printCaseResult(&sb, TestCase{Name: "c", MinScore: floatPtr(0)}, Rubric{}, cr)
	want := " ✅ 0% (threshold: 0%)\n      ✗ #1 command \"x\"\n"
	if sb.String() != want {
		t.Errorf("output = %q, want %q", sb.String(), want)
	}
}

func TestPrintCaseResultNoChecksUnchanged(t *testing.T) {
	cr := CaseResult{ActualOutput: "out", Scores: []CriterionScore{{Name: "low", Score: 1, MaxScore: 5}}}
	var sb strings.Builder
	printCaseResult(&sb, TestCase{Name: "c"}, Rubric{}, cr)
	want := " ❌ 20% (threshold: 95%)\n      low: 1/5\n"
	if sb.String() != want {
		t.Errorf("output = %q, want %q", sb.String(), want)
	}
}
