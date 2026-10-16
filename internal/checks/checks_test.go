package checks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// hiddenFixtures builds a hidden fixture directory containing hidden_test.go
// and sub/dir/x.txt, the sources the inject cases below resolve.
func hiddenFixtures(t *testing.T) string {
	t.Helper()
	hidden := t.TempDir()
	for rel, content := range map[string]string{
		"hidden_test.go": "package hidden\n",
		"sub/dir/x.txt":  "x\n",
	} {
		p := filepath.Join(hidden, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return hidden
}

// stubRubric stands in for eval's rubric cross-check: completeness and clarity
// are scored criteria, cost_efficiency is a cost criterion, anything else is
// unknown.
func stubRubric(name string) error {
	switch name {
	case "completeness", "clarity":
		return nil
	case "cost_efficiency":
		return fmt.Errorf("criterion %q is a cost criterion and cannot have checks", name)
	}
	return fmt.Errorf("criterion %q is not in the rubric", name)
}

// TestChecksYAMLDecodesChecksList decodes a top-level list of checks (the value
// of a case's `checks:` key) into []Check.
func TestChecksYAMLDecodesChecksList(t *testing.T) {
	const src = `- criterion: completeness
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
	var list []Check
	if err := yaml.Unmarshal([]byte(src), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d checks, want 3", len(list))
	}
	c := list[0]
	if c.Criterion != "completeness" || c.Type != CheckCommand || c.Run != "go test ./..." ||
		c.ExpectExit == nil || *c.ExpectExit != 3 || len(c.Inject) != 1 || c.Inject[0] != "hidden_test.go" {
		t.Errorf("command check decoded wrong: %+v", c)
	}
	c = list[1]
	if c.Type != CheckFileContains || c.Path != "a.txt" || c.Pattern != "(?m)^a" {
		t.Errorf("file_contains check decoded wrong: %+v", c)
	}
	c = list[2]
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
		"gh_log_contains", "gh_log_not_contains", "judge",
	}
	got := []CheckType{
		CheckCommand, CheckFileExists, CheckFileAbsent, CheckFileContains, CheckFileNotContains,
		CheckChangedFiles, CheckOutputContains, CheckOutputNotContains,
		CheckGHLogContains, CheckGHLogNotContains, CheckJudge,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("constant %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestChecksValidateTable drives Validate through every rule. Each
// invalid case must wrap ErrInvalid and name the case file, the 1-based
// check index and the check type.
func TestChecksValidateTable(t *testing.T) {
	hidden := hiddenFixtures(t)

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

		// Criterion callback (eval passes the rubric cross-check)
		{"unknown criterion", Check{Criterion: "nonexistent", Type: CheckFileExists, Path: "a"}, "nonexistent"},
		{"cost criterion", Check{Criterion: "cost_efficiency", Type: CheckFileExists, Path: "a"}, "cost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Two checks: a valid one first so the reported index is 2.
			checks := []Check{{Criterion: "clarity", Type: CheckFileExists, Path: "ok"}, tt.check}
			err := Validate("case-file.yaml", checks, ValidateOptions{HiddenDir: hidden, Criterion: stubRubric})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error does not wrap ErrInvalid: %v", err)
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
	checks := []Check{{Criterion: "clarity", Type: CheckCommand, Run: "true"}}
	if err := Validate("f.yaml", checks, ValidateOptions{HiddenDir: hiddenFixtures(t)}); err != nil {
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
	checks := []Check{
		{Criterion: "clarity", Type: CheckFileContains, Path: "a", Pattern: "x+"},
		{Criterion: "clarity", Type: CheckChangedFiles, Allow: []string{"a", "b/**"}},
		{Criterion: "clarity", Type: CheckCommand, Run: "true", Inject: []string{"hidden_test.go"}},
	}
	hidden := hiddenFixtures(t)
	if err := Validate("f.yaml", checks, ValidateOptions{HiddenDir: hidden}); err != nil {
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

func TestChecksValidateNilCriterionSkipsCrossCheck(t *testing.T) {
	checks := []Check{{Criterion: "anything", Type: CheckFileExists, Path: "a"}}
	if err := Validate("f.yaml", checks, ValidateOptions{}); err != nil {
		t.Fatalf("nil Criterion must skip the criterion cross-check: %v", err)
	}
}

func TestChecksValidateEmptyList(t *testing.T) {
	if err := Validate("f.yaml", nil, ValidateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestChecksValidateOptionalCriterion(t *testing.T) {
	c := Check{Type: CheckFileExists, Path: "a"}

	err := Validate("f.yaml", []Check{c}, ValidateOptions{})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "invalid checks: f.yaml: check #1 (file_exists): criterion is required") {
		t.Errorf("required criterion: err = %v", err)
	}
	if err := Validate("f.yaml", []Check{c}, ValidateOptions{OptionalCriterion: true}); err != nil {
		t.Errorf("OptionalCriterion: unexpected error %v", err)
	}

	// The callback sees every criterion, including the empty one when optional.
	var seen []string
	opts := ValidateOptions{OptionalCriterion: true, Criterion: func(name string) error {
		seen = append(seen, name)
		return nil
	}}
	if err := Validate("f.yaml", []Check{c, {Criterion: "clarity", Type: CheckFileExists, Path: "b"}}, opts); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "" || seen[1] != "clarity" {
		t.Errorf("Criterion callback saw %q, want [\"\" \"clarity\"]", seen)
	}
}
