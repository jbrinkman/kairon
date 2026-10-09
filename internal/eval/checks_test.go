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

func TestChecksYAMLAllowEmptyVersusMissing(t *testing.T) {
	var empty, missing Check
	if err := yaml.Unmarshal([]byte("type: changed_files\nallow: []\n"), &empty); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte("type: changed_files\n"), &missing); err != nil {
		t.Fatal(err)
	}
	if empty.Allow == nil {
		t.Error("allow: [] decoded to nil; want non-nil empty slice")
	}
	if missing.Allow != nil {
		t.Error("missing allow decoded to non-nil")
	}
}

func TestChecksCheckTypeSet(t *testing.T) {
	want := []CheckType{
		"command", "file_exists", "file_absent", "file_contains", "file_not_contains",
		"changed_files", "output_contains", "output_not_contains",
		"gh_log_contains", "gh_log_not_contains",
	}
	got := []CheckType{
		CheckCommand, CheckFileExists, CheckFileAbsent, CheckFileContains, CheckFileNotContains,
		CheckChangedFiles, CheckOutputContains, CheckOutputNotContains,
		CheckGHLogContains, CheckGHLogNotContains,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("constant %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestChecksValidateTable drives ValidateChecks through every rule. Each
// invalid case must wrap errInvalidChecks and name the case file, the 1-based
// check index and the check type.
func TestChecksValidateTable(t *testing.T) {
	checksEnv(t)
	rubric := &Rubric{Agent: "chk", Criteria: []Criterion{
		{Name: "completeness", Scoring: "1-5"},
		{Name: "clarity", Scoring: "1-5"},
		{Name: "cost_efficiency", Type: "cost"},
	}}
	hidden := evalsPath("fixtures", "hidden")

	// An absolute symlink target and a symlinked file/dir inside hidden/.
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(hidden, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(hidden, "sub"), filepath.Join(hidden, "linkdir")); err != nil {
		t.Fatal(err)
	}

	zero, three, big, neg := 0, 3, 256, -1
	tests := []struct {
		name    string
		check   Check
		wantErr string // substring besides file/index/type; "" = valid
	}{
		// valid forms
		{"command ok", Check{Criterion: "completeness", Type: CheckCommand, Run: "true"}, ""},
		{"command expect_exit 0", Check{Criterion: "completeness", Type: CheckCommand, Run: "true", ExpectExit: &zero}, ""},
		{"command expect_exit 255", Check{Criterion: "completeness", Type: CheckCommand, Run: "true", ExpectExit: func() *int { v := 255; return &v }()}, ""},
		{"command with inject", Check{Criterion: "completeness", Type: CheckCommand, Run: "true", Inject: []string{"hidden_test.go"}}, ""},
		{"command nested inject", Check{Criterion: "completeness", Type: CheckCommand, Run: "true", Inject: []string{"sub/dir/x.txt"}}, ""},
		{"file_exists ok", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a/b.txt"}, ""},
		{"file_absent ok", Check{Criterion: "clarity", Type: CheckFileAbsent, Path: "a"}, ""},
		{"file_contains ok", Check{Criterion: "clarity", Type: CheckFileContains, Path: "a", Pattern: `(?m)^x+$`}, ""},
		{"file_not_contains ok", Check{Criterion: "clarity", Type: CheckFileNotContains, Path: "a", Pattern: "x"}, ""},
		{"changed_files ok", Check{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{"README.md", "docs/**", "**/*.go"}}, ""},
		{"changed_files empty allow", Check{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{}}, ""},
		{"output_contains ok", Check{Criterion: "clarity", Type: CheckOutputContains, Pattern: "## "}, ""},
		{"output_not_contains ok", Check{Criterion: "clarity", Type: CheckOutputNotContains, Pattern: "x"}, ""},
		{"gh_log_contains ok", Check{Criterion: "clarity", Type: CheckGHLogContains, Pattern: "gh issue"}, ""},
		{"gh_log_not_contains ok", Check{Criterion: "clarity", Type: CheckGHLogNotContains, Pattern: "pr merge"}, ""},

		// type / criterion / required fields
		{"unknown type", Check{Criterion: "clarity", Type: "bogus", Path: "a"}, "unknown type"},
		{"missing type", Check{Criterion: "clarity", Path: "a"}, "type is required"},
		{"missing criterion", Check{Type: CheckFileExists, Path: "a"}, "criterion is required"},
		{"command missing run", Check{Criterion: "clarity", Type: CheckCommand}, "run is required"},
		{"command blank run", Check{Criterion: "clarity", Type: CheckCommand, Run: "  "}, "run is required"},
		{"file_exists missing path", Check{Criterion: "clarity", Type: CheckFileExists}, "path is required"},
		{"file_absent missing path", Check{Criterion: "clarity", Type: CheckFileAbsent}, "path is required"},
		{"file_contains missing path", Check{Criterion: "clarity", Type: CheckFileContains, Pattern: "x"}, "path is required"},
		{"file_contains missing pattern", Check{Criterion: "clarity", Type: CheckFileContains, Path: "a"}, "pattern is required"},
		{"file_not_contains missing pattern", Check{Criterion: "clarity", Type: CheckFileNotContains, Path: "a"}, "pattern is required"},
		{"output_contains missing pattern", Check{Criterion: "clarity", Type: CheckOutputContains}, "pattern is required"},
		{"output_not_contains missing pattern", Check{Criterion: "clarity", Type: CheckOutputNotContains}, "pattern is required"},
		{"gh_log_contains missing pattern", Check{Criterion: "clarity", Type: CheckGHLogContains}, "pattern is required"},
		{"gh_log_not_contains missing pattern", Check{Criterion: "clarity", Type: CheckGHLogNotContains}, "pattern is required"},
		{"changed_files missing allow", Check{Criterion: "clarity", Type: CheckChangedFiles}, "allow is required"},

		// fields that do not belong to the type
		{"command with path", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Path: "a"}, "path is not valid"},
		{"command with pattern", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Pattern: "a"}, "pattern is not valid"},
		{"command with allow", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Allow: []string{"a"}}, "allow is not valid"},
		{"file_exists with run", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a", Run: "true"}, "run is not valid"},
		{"file_exists with pattern", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a", Pattern: "x"}, "pattern is not valid"},
		{"file_exists with expect_exit", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a", ExpectExit: &zero}, "expect_exit is not valid"},
		{"file_absent with pattern", Check{Criterion: "clarity", Type: CheckFileAbsent, Path: "a", Pattern: "x"}, "pattern is not valid"},
		{"file_contains with allow", Check{Criterion: "clarity", Type: CheckFileContains, Path: "a", Pattern: "x", Allow: []string{}}, "allow is not valid"},
		{"changed_files with path", Check{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{}, Path: "a"}, "path is not valid"},
		{"changed_files with pattern", Check{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{}, Pattern: "a"}, "pattern is not valid"},
		{"output_contains with path", Check{Criterion: "clarity", Type: CheckOutputContains, Pattern: "x", Path: "a"}, "path is not valid"},
		{"gh_log_contains with run", Check{Criterion: "clarity", Type: CheckGHLogContains, Pattern: "x", Run: "true"}, "run is not valid"},

		// regex / glob / path
		{"bad regex file_contains", Check{Criterion: "clarity", Type: CheckFileContains, Path: "a", Pattern: "(unclosed"}, "pattern"},
		{"bad regex output", Check{Criterion: "clarity", Type: CheckOutputContains, Pattern: "[a-"}, "pattern"},
		{"bad regex gh_log", Check{Criterion: "clarity", Type: CheckGHLogNotContains, Pattern: "*x"}, "pattern"},
		{"bad glob", Check{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{"ok", "docs/[a-"}}, "invalid glob"},
		{"empty glob", Check{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{""}}, "invalid glob"},
		{"absolute path", Check{Criterion: "clarity", Type: CheckFileExists, Path: "/etc/passwd"}, "path"},
		{"dotdot path", Check{Criterion: "clarity", Type: CheckFileExists, Path: "../x"}, "path"},
		{"embedded dotdot path", Check{Criterion: "clarity", Type: CheckFileContains, Path: "a/../../x", Pattern: "x"}, "path"},
		{"dotdot only path", Check{Criterion: "clarity", Type: CheckFileAbsent, Path: ".."}, "path"},

		// expect_exit range
		{"expect_exit negative", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", ExpectExit: &neg}, "expect_exit"},
		{"expect_exit too big", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", ExpectExit: &big}, "expect_exit"},
		{"expect_exit 3 ok", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", ExpectExit: &three}, ""},

		// inject
		{"inject on file_exists", Check{Criterion: "clarity", Type: CheckFileExists, Path: "a", Inject: []string{"hidden_test.go"}}, "inject is not valid"},
		{"inject on output_contains", Check{Criterion: "clarity", Type: CheckOutputContains, Pattern: "x", Inject: []string{"hidden_test.go"}}, "inject is not valid"},
		{"inject missing source", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"nope.go"}}, "inject"},
		{"inject absolute", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"/etc/passwd"}}, "inject"},
		{"inject dotdot", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"../rubrics/chk.yaml"}}, "inject"},
		{"inject empty entry", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{""}}, "inject"},
		{"inject directory", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"sub"}}, "regular file"},
		{"inject symlink file", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"link.txt"}}, "symlink"},
		{"inject via symlink dir", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"linkdir/dir/x.txt"}}, "symlink"},
		{"inject dest .git", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{".git/config"}}, "inject"},
		{"inject dest .eval", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{".eval/gh.log"}}, "inject"},
		{"inject dest .kiro", Check{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{".kiro/agents/x.json"}}, "inject"},

		// rubric cross-check
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

func TestChecksValidateDefaultsExpectExitToZero(t *testing.T) {
	checksEnv(t)
	checks := []Check{{Criterion: "clarity", Type: CheckCommand, Run: "true"}}
	if err := ValidateChecks("f.yaml", checks, evalsPath("fixtures", "hidden"), nil); err != nil {
		t.Fatal(err)
	}
	if checks[0].ExpectExit == nil || *checks[0].ExpectExit != 0 {
		t.Fatalf("ExpectExit = %v, want pointer to 0", checks[0].ExpectExit)
	}
	if got := checks[0].expectedExit(); got != 0 {
		t.Fatalf("expectedExit() = %d, want 0", got)
	}
	var hand Check
	if got := hand.expectedExit(); got != 0 {
		t.Fatalf("zero-value expectedExit() = %d, want 0", got)
	}
}

func TestChecksValidateCompilesAndResolves(t *testing.T) {
	checksEnv(t)
	checks := []Check{
		{Criterion: "clarity", Type: CheckFileContains, Path: "a", Pattern: "x+"},
		{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{"a", "b/**"}},
		{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"hidden_test.go"}},
	}
	hidden := evalsPath("fixtures", "hidden")
	if err := ValidateChecks("f.yaml", checks, hidden, nil); err != nil {
		t.Fatal(err)
	}
	if checks[0].re == nil || !checks[0].re.MatchString("xx") {
		t.Error("pattern not compiled onto the check")
	}
	if len(checks[1].globs) != 2 || !checks[1].globs[1].match("b/c/d") {
		t.Error("allow globs not compiled onto the check")
	}
	abs, _ := filepath.Abs(hidden)
	if checks[2].injectDir != abs {
		t.Errorf("injectDir = %q, want %q", checks[2].injectDir, abs)
	}
}

func TestChecksValidateNilRubricSkipsCrossCheck(t *testing.T) {
	checksEnv(t)
	checks := []Check{{Criterion: "anything", Type: CheckFileExists, Path: "a"}}
	if err := ValidateChecks("f.yaml", checks, evalsPath("fixtures", "hidden"), nil); err != nil {
		t.Fatalf("nil rubric must skip the criterion cross-check: %v", err)
	}
}

func TestChecksValidateEmptyList(t *testing.T) {
	if err := ValidateChecks("f.yaml", nil, "", nil); err != nil {
		t.Fatal(err)
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
	if c.injectDir == "" {
		t.Error("inject source dir not resolved at load")
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
