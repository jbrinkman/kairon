package eval

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// checksOnlyRubric has a deterministic criterion, an LLM-judged criterion and
// a cost criterion; checks_only is set by the caller.
func checksOnlyRubric(checksOnly bool) Rubric {
	return Rubric{Agent: "a", ChecksOnly: checksOnly, Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-5"},
		{Name: "cost_efficiency", Type: "cost"},
	}}
}

func hasJudgeCall(cr CaseResult) bool {
	for _, c := range cr.Calls {
		if c.Role == "judge" {
			return true
		}
	}
	return false
}

func TestRubricChecksOnlyYAML(t *testing.T) {
	for name, tt := range map[string]struct {
		yaml string
		want bool
	}{
		"set":    {"agent: a\nchecks_only: true\ncriteria: []\n", true},
		"unset":  {"agent: a\ncriteria: []\n", false},
		"false":  {"agent: a\nchecks_only: false\ncriteria: []\n", false},
		"nested": {"agent: a\ncriteria:\n  - name: x\n    scoring: 1-5\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			var r Rubric
			if err := yaml.Unmarshal([]byte(tt.yaml), &r); err != nil {
				t.Fatal(err)
			}
			if r.ChecksOnly != tt.want {
				t.Errorf("ChecksOnly = %v, want %v", r.ChecksOnly, tt.want)
			}
		})
	}
}

func TestScoreCaseChecksOnlySkipsUncheckedCriteria(t *testing.T) {
	chdirTemp(t)
	_, calls := installFakeKiroCLI(t, "")
	useStubBackend(t)

	dir := t.TempDir()
	writeEvalFile(t, dir, "a.txt", "hello\n")
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "clarity", Type: CheckFileExists, Path: "a.txt"},
		{Criterion: "clarity", Type: CheckFileContains, Path: "a.txt", Pattern: "hello"},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S\ntext", WorkspaceDir: dir}
	scoreCase(checksOnlyRubric(true), tc, &cr)

	if len(cr.Scores) != 1 {
		t.Fatalf("scores = %+v, want only the checked criterion", cr.Scores)
	}
	if s := cr.Scores[0]; s.Name != "clarity" || s.Score != 2 || s.MaxScore != 2 || !s.Deterministic || s.Skipped {
		t.Errorf("clarity = %+v, want 2/2 deterministic", s)
	}
	for _, s := range cr.Scores {
		if s.Name == "structural_completeness" || s.Name == "cost_efficiency" {
			t.Errorf("unchecked/cost criterion %q has a score: %+v", s.Name, s)
		}
	}
	if score, max := caseTotals(cr); score != 2 || max != 2 {
		t.Errorf("caseTotals = %d/%d, want 2/2", score, max)
	}
	as, am := agentScoreTotals(AgentResult{Cases: []CaseResult{cr}})
	if as != 2 || am != 2 {
		t.Errorf("agentScoreTotals = %v/%v, want 2/2", as, am)
	}
	if len(cr.Calls) != 0 || !reflect.DeepEqual(cr.JudgeCost, CostInfo{}) {
		t.Errorf("calls = %+v, judge cost = %+v, want none", cr.Calls, cr.JudgeCost)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestScoreCaseChecksOnlyNeverJudgesOrRunsHeuristic(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	dir := t.TempDir()
	writeEvalFile(t, dir, "a.txt", "x\n")
	// The only check is on a criterion that is not clarity or
	// structural_completeness, so both of those are unchecked.
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "extra", Type: CheckFileExists, Path: "a.txt"},
	}}
	rubric := Rubric{Agent: "a", ChecksOnly: true, Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-5"},
		{Name: "extra", Scoring: "1-5"},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S", WorkspaceDir: dir}
	scoreCase(rubric, tc, &cr)

	if hasJudgeCall(cr) || len(cr.Calls) != 0 {
		t.Errorf("calls = %+v, want no judge call", cr.Calls)
	}
	if len(cr.Scores) != 1 || cr.Scores[0].Name != "extra" {
		t.Errorf("scores = %+v, want only extra", cr.Scores)
	}
}

func TestScoreCaseChecksOnlyEmptyOutputScoresZeroOfChecks(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "clarity", Type: CheckFileExists, Path: "a.txt"},
		{Criterion: "clarity", Type: CheckOutputContains, Pattern: "x"},
		{Criterion: "structural_completeness", Type: CheckOutputContains, Pattern: "y"},
	}}
	cr := CaseResult{ActualOutput: "", WorkspaceDir: t.TempDir()}
	scoreCase(checksOnlyRubric(true), tc, &cr)

	if len(cr.Scores) != 2 {
		t.Fatalf("scores = %+v, want 2 checked criteria", cr.Scores)
	}
	for name, max := range map[string]int{"structural_completeness": 1, "clarity": 2} {
		s := scoreByName(t, cr, name)
		if s.Score != 0 || s.MaxScore != max || s.Skipped || s.Reasoning != noOutputChecksReasoning {
			t.Errorf("%s = %+v, want non-skipped 0/%d (%s)", name, s, max, noOutputChecksReasoning)
		}
	}
	if score, max := caseTotals(cr); score != 0 || max != 3 {
		t.Errorf("caseTotals = %d/%d, want 0/3", score, max)
	}
}

// Without checks_only the unchecked criteria are scored exactly as before:
// the heuristic for the deterministic one, the judge for the other.
func TestScoreCaseWithoutChecksOnlyStillScoresUncheckedCriteria(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)

	dir := t.TempDir()
	writeEvalFile(t, dir, "a.txt", "x\n")
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "extra", Type: CheckFileExists, Path: "a.txt"},
	}}
	rubric := Rubric{Agent: "a", Criteria: []Criterion{
		{Name: "structural_completeness", Scoring: "1-5", Deterministic: true},
		{Name: "clarity", Scoring: "1-5"},
		{Name: "extra", Scoring: "1-5"},
	}}
	cr := CaseResult{ActualOutput: "## H\n### S", WorkspaceDir: dir}
	scoreCase(rubric, tc, &cr)

	if len(cr.Scores) != 3 {
		t.Fatalf("scores = %+v, want 3", cr.Scores)
	}
	sc := scoreByName(t, cr, "structural_completeness")
	if sc.Score != 5 || sc.MaxScore != 5 || sc.Reasoning != "found 2/2 expected structural elements" {
		t.Errorf("structural_completeness = %+v, want legacy heuristic 5/5", sc)
	}
	cl := scoreByName(t, cr, "clarity")
	if cl.Score != 5 || cl.MaxScore != 5 || cl.Deterministic {
		t.Errorf("clarity = %+v, want legacy judge 5/5", cl)
	}
	if !hasJudgeCall(cr) {
		t.Errorf("calls = %+v, want a legacy judge call for clarity", cr.Calls)
	}
	if s := scoreByName(t, cr, "extra"); s.Score != 1 || s.MaxScore != 1 {
		t.Errorf("extra = %+v, want 1/1", s)
	}
}

// writeChecksOnlyEvals writes a minimal evals dir for one agent: a rubric
// (checks_only as given) and the given cases (file name -> yaml body).
func writeChecksOnlyEvals(t *testing.T, root, agent string, checksOnly bool, cases map[string]string) {
	t.Helper()
	rubric := "agent: " + agent + "\n"
	if checksOnly {
		rubric += "checks_only: true\n"
	}
	rubric += "criteria:\n  - name: clarity\n    description: d\n    scoring: 1-5\n"
	writeCfgFile(t, filepath.Join(root, "rubrics", agent+".yaml"), rubric)
	for file, body := range cases {
		writeCfgFile(t, filepath.Join(root, "cases", agent, file), body)
	}
}

const checkedCaseYAML = `name: has-checks
description: d
input: hi
expected_output: out
stub:
  turns:
    - response: "## H"
checks:
  - criterion: clarity
    type: output_contains
    pattern: "## H"
`

const checkLessCaseYAML = `name: no-checks
description: d
input: hi
expected_output: out
stub:
  turns:
    - response: "## H"
`

func TestLoadCasesChecksOnlyRejectsCheckLessCase(t *testing.T) {
	root := t.TempDir()
	writeChecksOnlyEvals(t, root, "agent-x", true, map[string]string{
		"good.yaml":                 checkedCaseYAML,
		"zz-missing-checks-it.yaml": checkLessCaseYAML,
	})
	if err := configure(RunOptions{Backend: "stub", EvalsDir: root}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	_, err := loadCases("agent-x")
	if err == nil {
		t.Fatal("loadCases succeeded, want an error for the check-less case")
	}
	if !errors.Is(err, errInvalidChecks) {
		t.Errorf("error %v does not wrap errInvalidChecks", err)
	}
	if !strings.Contains(err.Error(), "zz-missing-checks-it.yaml") {
		t.Errorf("error %q does not name the case file", err)
	}
}

func TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase(t *testing.T) {
	root := t.TempDir()
	writeChecksOnlyEvals(t, root, "agent-x", false, map[string]string{
		"good.yaml":                 checkedCaseYAML,
		"zz-missing-checks-it.yaml": checkLessCaseYAML,
	})
	if err := configure(RunOptions{Backend: "stub", EvalsDir: root}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	cases, err := loadCases("agent-x")
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	if len(cases) != 2 {
		t.Errorf("loaded %d cases, want 2", len(cases))
	}
}

func TestLoadCasesChecksOnlyAllowsFullyCheckedCases(t *testing.T) {
	root := t.TempDir()
	writeChecksOnlyEvals(t, root, "agent-x", true, map[string]string{"good.yaml": checkedCaseYAML})
	if err := configure(RunOptions{Backend: "stub", EvalsDir: root}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	if cases, err := loadCases("agent-x"); err != nil || len(cases) != 1 {
		t.Fatalf("loadCases = %d cases, err %v; want 1, nil", len(cases), err)
	}
}

// An invalid checks_only agent must fail the run before any agent's first
// case runs, even when a valid agent sorts (and so would run) first.
func TestRunProgressiveEvaluationChecksOnlyPreflightRunsNoCase(t *testing.T) {
	root := t.TempDir()
	writeChecksOnlyEvals(t, root, "a-valid", false, map[string]string{"good.yaml": checkedCaseYAML})
	writeChecksOnlyEvals(t, root, "b-invalid", true, map[string]string{
		"good.yaml":                checkedCaseYAML,
		"preflight-no-checks.yaml": checkLessCaseYAML,
	})
	if err := configure(RunOptions{Backend: "stub", EvalsDir: root, NoSandbox: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	resultsDir := filepath.Join(root, "results", "run")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := runProgressiveEvaluation("", resultsDir, false, nil)
	if !errors.Is(err, errInvalidChecks) {
		t.Fatalf("error = %v, want errInvalidChecks", err)
	}
	if !strings.Contains(err.Error(), "preflight-no-checks.yaml") {
		t.Errorf("error %q does not name the case file", err)
	}
	if _, statErr := os.Stat(filepath.Join(resultsDir, "a-valid.json")); !os.IsNotExist(statErr) {
		t.Errorf("a-valid.json exists (stat err %v): a case ran before the checks_only agent was rejected", statErr)
	}
}
