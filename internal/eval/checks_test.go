package eval

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// checksEnv prepares a temp evals dir ("evals") with a rubric for the agent
// "chk" (criteria: completeness, clarity, cost_efficiency [cost]) and a hidden
// fixture directory containing hidden_test.go.
func checksEnv(t *testing.T) {
	t.Helper()
	chdirTemp(t)
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil {
		t.Fatal(err)
	}
	writeCfgFile(t, "evals/rubrics/chk.yaml", `agent: chk
criteria:
  - name: completeness
    description: d
    scoring: 1-5
  - name: clarity
    description: d
    scoring: 1-5
  - name: cost_efficiency
    description: d
    type: cost
`)
	writeCfgFile(t, "evals/fixtures/hidden/hidden_test.go", "package hidden\n")
	writeCfgFile(t, "evals/fixtures/hidden/sub/dir/x.txt", "x\n")
}

func writeChecksCase(t *testing.T, agent, file, body string) {
	t.Helper()
	writeCfgFile(t, filepath.Join("evals", "cases", agent, file), body)
}

func TestChecksYAMLDecodesFieldNames(t *testing.T) {
	const src = `name: n
input: x
checks:
  - criterion: completeness
    type: command
    run: "go test ./..."
    expect_exit: 3
    inject: [hidden_test.go]
  - criterion: completeness
    type: file_contains
    path: a.txt
    pattern: "(?m)^a"
  - criterion: clarity
    type: changed_files
    allow: [README.md, "docs/**"]
`
	var tc TestCase
	if err := yaml.Unmarshal([]byte(src), &tc); err != nil {
		t.Fatal(err)
	}
	if len(tc.Checks) != 3 {
		t.Fatalf("got %d checks, want 3", len(tc.Checks))
	}
	c := tc.Checks[0]
	if c.Criterion != "completeness" || c.Type != CheckCommand || c.Run != "go test ./..." ||
		c.ExpectExit == nil || *c.ExpectExit != 3 || len(c.Inject) != 1 || c.Inject[0] != "hidden_test.go" {
		t.Errorf("command check decoded wrong: %+v", c)
	}
	c = tc.Checks[1]
	if c.Type != CheckFileContains || c.Path != "a.txt" || c.Pattern != "(?m)^a" {
		t.Errorf("file_contains check decoded wrong: %+v", c)
	}
	c = tc.Checks[2]
	if c.Type != CheckChangedFiles || len(c.Allow) != 2 || c.Allow[1] != "docs/**" {
		t.Errorf("changed_files check decoded wrong: %+v", c)
	}
}

// TestChecksValidateRubricCrossCheck drives ValidateChecks' rubric cross-check:
// the engine's own rules live in internal/checks; here a check's criterion must
// be a non-cost criterion of the rubric, and errors wrap errInvalidChecks and
// name the case file, the check index and the type.
func TestChecksValidateRubricCrossCheck(t *testing.T) {
	checksEnv(t)
	rubric := &Rubric{Agent: "chk", Criteria: []Criterion{
		{Name: "completeness", Scoring: "1-5"},
		{Name: "clarity", Scoring: "1-5"},
		{Name: "cost_efficiency", Type: "cost"},
	}}
	hidden := evalsPath("fixtures", "hidden")

	tests := []struct {
		name    string
		check   Check
		wantErr string // substring besides file/index/type; "" = valid
	}{
		{"scored criterion", Check{Criterion: "completeness", Type: CheckCommand, Run: "true", Inject: []string{"hidden_test.go"}}, ""},
		{"scored criterion clarity", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a/b.txt"}, ""},
		{"missing criterion", Check{Type: CheckFileExists, Path: "a"}, "criterion is required"},
		{"missing path", Check{Criterion: "clarity", Type: CheckFileExists}, "path is required"},
		{"inject missing source", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"nope.go"}}, "inject"},
		{"unknown criterion", Check{Criterion: "nonexistent", Type: CheckFileExists, Path: "a"}, "nonexistent"},
		{"cost criterion", Check{Criterion: "cost_efficiency", Type: CheckFileExists, Path: "a"}, "cost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Two checks: a valid one first so the reported index is 2.
			checks := []Check{{Criterion: "clarity", Type: CheckFileExists, Path: "ok"}, tt.check}
			err := ValidateChecks("case-file.yaml", checks, hidden, rubric)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !errors.Is(err, errInvalidChecks) {
				t.Errorf("error does not wrap errInvalidChecks: %v", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "case-file.yaml") {
				t.Errorf("error does not name the case file: %v", err)
			}
			if !strings.Contains(msg, "#2") {
				t.Errorf("error does not name check index #2: %v", err)
			}
			if tt.check.Type != "" && !strings.Contains(msg, string(tt.check.Type)) {
				t.Errorf("error does not name the check type %q: %v", tt.check.Type, err)
			}
			if !strings.Contains(msg, tt.wantErr) {
				t.Errorf("error %q does not contain %q", msg, tt.wantErr)
			}
		})
	}
}

func TestChecksValidateNilRubricSkipsCrossCheck(t *testing.T) {
	checksEnv(t)
	checks := []Check{{Criterion: "anything", Type: CheckFileExists, Path: "a"}}
	if err := ValidateChecks("f.yaml", checks, evalsPath("fixtures", "hidden"), nil); err != nil {
		t.Fatalf("nil rubric must skip the criterion cross-check: %v", err)
	}
}

func TestLoadCasesChecks(t *testing.T) {
	checksEnv(t)
	writeChecksCase(t, "chk", "good.yaml", `name: good
input: x
checks:
  - criterion: completeness
    type: command
    run: "true"
    inject: [hidden_test.go]
  - criterion: clarity
    type: changed_files
    allow: []
`)
	got, err := loadCases("chk")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Checks) != 2 {
		t.Fatalf("loaded %+v", got)
	}
	c := got[0].Checks[0]
	if c.ExpectExit == nil || *c.ExpectExit != 0 {
		t.Errorf("expect_exit default not applied: %v", c.ExpectExit)
	}
	// The inject source dir is resolved at load (it is unexported now that the
	// engine lives in internal/checks), so a loaded check can still inject
	// hidden_test.go into a workspace; an unresolved one would fail to evaluate.
	probe := c
	probe.Run = "test -f hidden_test.go"
	if r := EvaluateChecks([]Check{probe}, CheckInput{Dir: t.TempDir()})[0]; !r.Passed {
		t.Errorf("inject source dir not resolved at load: %s", r.Detail)
	}
	if got[0].Checks[1].Allow == nil {
		t.Error("allow: [] must stay non-nil")
	}
}

func TestLoadCasesChecksRejected(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"unknown type", "checks:\n  - criterion: clarity\n    type: bogus\n", "unknown type"},
		{"missing allow", "checks:\n  - criterion: clarity\n    type: changed_files\n", "allow is required"},
		{"bad regex", "checks:\n  - criterion: clarity\n    type: output_contains\n    pattern: \"(\"\n", "pattern"},
		{"bad glob", "checks:\n  - criterion: clarity\n    type: changed_files\n    allow: [\"[\"]\n", "invalid glob"},
		{"dotdot path", "checks:\n  - criterion: clarity\n    type: file_exists\n    path: ../x\n", "path"},
		{"wrong-type field", "checks:\n  - criterion: clarity\n    type: file_exists\n    path: a\n    run: x\n", "run is not valid"},
		{"missing inject source", "checks:\n  - criterion: clarity\n    type: command\n    run: \"true\"\n    inject: [gone.go]\n", "inject"},
		{"unknown criterion", "checks:\n  - criterion: nope\n    type: file_exists\n    path: a\n", "nope"},
		{"cost criterion", "checks:\n  - criterion: cost_efficiency\n    type: file_exists\n    path: a\n", "cost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksEnv(t)
			writeChecksCase(t, "chk", "bad-case.yaml", "name: bad\ninput: x\n"+tt.yaml)
			_, err := loadCases("chk")
			if err == nil {
				t.Fatal("expected a load error")
			}
			if !errors.Is(err, errInvalidChecks) {
				t.Errorf("error does not wrap errInvalidChecks: %v", err)
			}
			for _, want := range []string{"bad-case.yaml", "#1", tt.wantErr} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// A case file with no checks loads exactly as before, even when the agent has
// no rubric at all or the hidden fixture directory does not exist.
func TestLoadCasesWithoutChecksUnchanged(t *testing.T) {
	chdirTemp(t)
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil {
		t.Fatal(err)
	}
	writeChecksCase(t, "norubric", "plain.yaml", "name: plain\ninput: x\n")
	got, err := loadCases("norubric")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "plain" || got[0].Checks != nil {
		t.Fatalf("got %+v", got)
	}
}

// When the agent's rubric cannot be loaded, the criterion cross-check is
// skipped (the rubric errors surface elsewhere); every other rule still holds.
func TestLoadCasesChecksSkipsRubricCrossCheckWithoutRubric(t *testing.T) {
	chdirTemp(t)
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil {
		t.Fatal(err)
	}
	writeChecksCase(t, "norubric", "c.yaml", "name: c\ninput: x\nchecks:\n  - criterion: whatever\n    type: file_exists\n    path: a\n")
	if _, err := loadCases("norubric"); err != nil {
		t.Fatalf("missing rubric must skip the cross-check: %v", err)
	}
	writeChecksCase(t, "norubric", "d.yaml", "name: d\ninput: x\nchecks:\n  - criterion: whatever\n    type: bogus\n")
	if _, err := loadCases("norubric"); !errors.Is(err, errInvalidChecks) {
		t.Fatalf("other rules must still apply, got %v", err)
	}
}

func TestChecksCriterionScoreJSON(t *testing.T) {
	cs := CriterionScore{Name: "n", Score: 1, MaxScore: 2, Checks: []CheckResult{
		{Index: 2, Type: CheckFileExists, Label: "#2 file_exists path=missing.txt", Passed: false, Detail: "file does not exist"},
	}}
	b, err := json.Marshal(cs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"checks":[`, `"index":2`, `"type":"file_exists"`, `"label":"#2 file_exists path=missing.txt"`, `"passed":false`, `"detail":"file does not exist"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON %s missing %s", b, want)
		}
	}
	// Additive: no checks → no "checks" key, so older results are unchanged.
	b, _ = json.Marshal(CriterionScore{Name: "n"})
	if strings.Contains(string(b), "checks") {
		t.Errorf("empty Checks must be omitted: %s", b)
	}
}

// initGitRepoHere makes the (temp) cwd a git repo with one commit so
// getGitShortHash succeeds.
func initGitRepoHere(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	writeCfgFile(t, "seed.txt", "seed\n")
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "seed.txt"},
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "seed"},
	} {
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestChecksRunProgressiveEvaluationInvalidChecksIsFatal(t *testing.T) {
	checksEnv(t)
	initGitRepoHere(t)
	writeChecksCase(t, "chk", "bad.yaml", "name: bad\ninput: x\nchecks:\n  - criterion: clarity\n    type: bogus\n")

	if err := os.MkdirAll(filepath.Join("evals", "results", "r1"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := runProgressiveEvaluation("chk", filepath.Join("evals", "results", "r1"), false, nil)
	if err == nil {
		t.Fatal("invalid checks must fail the run, got nil")
	}
	if !errors.Is(err, errInvalidChecks) {
		t.Fatalf("error does not wrap errInvalidChecks: %v", err)
	}
	if !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("error does not name the case file: %v", err)
	}
}

func TestChecksRunProgressiveEvaluationMalformedCaseIsFatal(t *testing.T) {
	checksEnv(t)
	initGitRepoHere(t)
	// A malformed value (expect_exit wants an int) fails during YAML decode in
	// loadCases, before ValidateChecks can wrap it with errInvalidChecks. The
	// run must still fail rather than skip the agent's cases and exit 0.
	writeChecksCase(t, "chk", "bad.yaml", "name: bad\ninput: x\nchecks:\n  - criterion: clarity\n    type: command\n    run: \"true\"\n    expect_exit: abc\n")

	if err := os.MkdirAll(filepath.Join("evals", "results", "rm"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := runProgressiveEvaluation("chk", filepath.Join("evals", "results", "rm"), false, nil)
	if err == nil {
		t.Fatal("a malformed case file must fail the run, got nil")
	}
	if !errors.Is(err, errMalformedCase) {
		t.Fatalf("error does not wrap errMalformedCase: %v", err)
	}
	if !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("error does not name the case file: %v", err)
	}
}

func TestChecksRunProgressiveEvaluationBadStubJudgeIsFatal(t *testing.T) {
	// A bad stub.judge (empty list or blank entry) must fail the run, not be
	// downgraded to a "no test cases for agent" warning that drops every case
	// and exits 0. validateCaseFields rejects it, so the error must wrap
	// errMalformedCase for the runner.go fatal gate to re-raise it.
	for name, stub := range map[string]string{
		"empty list":  "stub:\n  judge: []\n",
		"blank entry": "stub:\n  judge: [yes, \"  \"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			checksEnv(t)
			initGitRepoHere(t)
			writeChecksCase(t, "chk", "bad.yaml", "name: bad\ninput: x\n"+stub)
			if err := os.MkdirAll(filepath.Join("evals", "results", "rj"), 0o755); err != nil {
				t.Fatal(err)
			}
			err := runProgressiveEvaluation("chk", filepath.Join("evals", "results", "rj"), false, nil)
			if err == nil {
				t.Fatal("a bad stub.judge must fail the run, got nil")
			}
			if !errors.Is(err, errMalformedCase) {
				t.Fatalf("error does not wrap errMalformedCase: %v", err)
			}
			if !strings.Contains(err.Error(), "bad.yaml") {
				t.Errorf("error does not name the case file: %v", err)
			}
		})
	}
}

func TestChecksRunProgressiveEvaluationMissingCasesDirIsWarning(t *testing.T) {
	checksEnv(t)
	initGitRepoHere(t)
	// Rubric exists for agent "chk" but there is no evals/cases/chk directory.
	if err := os.MkdirAll(filepath.Join("evals", "results", "r2"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := runProgressiveEvaluation("chk", filepath.Join("evals", "results", "r2"), false, nil)
	if err != nil {
		t.Fatalf("a missing cases directory must stay a warning, got %v", err)
	}
}
