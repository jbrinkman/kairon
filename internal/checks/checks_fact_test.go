package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSimLog writes content to <dir>/.eval/sim-log.jsonl and returns dir.
func writeSimLog(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".eval"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".eval", "sim-log.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const twoDecisionLog = `{"turn":1,"question":"which format?","fact":"f1","discoverable":false}
{"turn":2,"question":"anything else?","fact":"none"}
`

func factCheck(typ CheckType, fact string) Check {
	return Check{Criterion: "c", Type: typ, Fact: fact}
}

func evalOneFact(t *testing.T, dir string, c Check) Result {
	t.Helper()
	res := Evaluate([]Check{c}, Input{Dir: dir})
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	return res[0]
}

func TestFactChecks_PassAndFailFromLog(t *testing.T) {
	dir := writeSimLog(t, twoDecisionLog)
	cases := []struct {
		name       string
		check      Check
		wantPass   bool
		wantDetail []string
	}{
		{"asked f1 passes", factCheck(CheckFactAsked, "f1"), true, nil},
		{"not_asked f2 passes", factCheck(CheckFactNotAsked, "f2"), true, nil},
		{"asked f2 fails", factCheck(CheckFactAsked, "f2"), false, []string{"f2", "f1", "none"}},
		{"not_asked f1 fails", factCheck(CheckFactNotAsked, "f1"), false, []string{"f1", "turn 1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := evalOneFact(t, dir, tc.check)
			if res.Passed != tc.wantPass {
				t.Fatalf("passed = %v, want %v (detail %q)", res.Passed, tc.wantPass, res.Detail)
			}
			for _, w := range tc.wantDetail {
				if !strings.Contains(res.Detail, w) {
					t.Errorf("detail %q does not contain %q", res.Detail, w)
				}
			}
		})
	}
}

func TestFactChecks_AskedNoDecisions(t *testing.T) {
	dir := writeSimLog(t, "")
	res := evalOneFact(t, dir, factCheck(CheckFactAsked, "f1"))
	if res.Passed || !strings.Contains(res.Detail, "asked: none") {
		t.Errorf("got passed=%v detail=%q, want fail with 'asked: none'", res.Passed, res.Detail)
	}
	if res := evalOneFact(t, dir, factCheck(CheckFactNotAsked, "f1")); !res.Passed {
		t.Errorf("fact_not_asked on an empty (present) log must pass, got %q", res.Detail)
	}
}

func TestFactChecks_UnreadableLogFails(t *testing.T) {
	big := strings.Repeat(`{"turn":1,"question":"q","fact":"none"}`+"\n", (10<<20)/40+10)

	mk := map[string]struct {
		setup  func(t *testing.T) string
		detail string
	}{
		"missing": {func(t *testing.T) string { return t.TempDir() }, "sim log missing"},
		"directory": {func(t *testing.T) string {
			d := t.TempDir()
			if err := os.MkdirAll(filepath.Join(d, ".eval", "sim-log.jsonl"), 0o755); err != nil {
				t.Fatal(err)
			}
			return d
		}, "not a regular file"},
		"symlink": {func(t *testing.T) string {
			d := writeSimLog(t, twoDecisionLog)
			target := filepath.Join(d, "real.jsonl")
			p := filepath.Join(d, ".eval", "sim-log.jsonl")
			if err := os.Rename(p, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, p); err != nil {
				t.Skipf("symlinks unsupported: %v", err)
			}
			return d
		}, "symlink"},
		"oversized": {func(t *testing.T) string { return writeSimLog(t, big) }, "10 MiB"},
		"malformed": {func(t *testing.T) string {
			return writeSimLog(t, `{"turn":1,"question":"q","fact":"f1"}`+"\n"+`{"turn":2,"question":`+"\n")
		}, "sim log line 2: invalid JSON"},
	}
	for name, m := range mk {
		for _, typ := range []CheckType{CheckFactAsked, CheckFactNotAsked} {
			t.Run(name+"/"+string(typ), func(t *testing.T) {
				dir := m.setup(t)
				res := evalOneFact(t, dir, factCheck(typ, "f1"))
				if res.Passed {
					t.Fatalf("%s on a %s log must fail, passed", typ, name)
				}
				if !strings.Contains(res.Detail, m.detail) {
					t.Errorf("detail %q does not contain %q", res.Detail, m.detail)
				}
			})
		}
	}
}

func TestFactChecks_ValidateFields(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // "" = valid
	}{
		{"asked ok", "{criterion: c, type: fact_asked, fact: f1}", ""},
		{"not_asked ok", "{criterion: c, type: fact_not_asked, fact: f1}", ""},
		{"asked requires fact", "{criterion: c, type: fact_asked}", "fact is required"},
		{"not_asked requires fact", "{criterion: c, type: fact_not_asked}", "fact is required"},
		{"blank fact", "{criterion: c, type: fact_asked, fact: '  '}", "fact is required"},
		{"turn on asked", "{criterion: c, type: fact_asked, fact: f1, turn: 1}", "turn is not valid for type fact_asked"},
		{"turn on not_asked", "{criterion: c, type: fact_not_asked, fact: f1, turn: 1}", "turn is not valid for type fact_not_asked"},
		{"fact on output_contains", "{criterion: c, type: output_contains, pattern: x, fact: f1}", "fact is not valid for type output_contains"},
		{"fact on command", "{criterion: c, type: command, run: 'true', fact: f1}", "fact is not valid for type command"},
		{"fact on judge", "{criterion: c, type: judge, question: q, fact: f1}", "fact is not valid for type judge"},
		{"pattern on asked", "{criterion: c, type: fact_asked, fact: f1, pattern: x}", "pattern is not valid for type fact_asked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate("f.yaml", decodeChecks(t, "- "+tc.src+"\n"), ValidateOptions{})
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", tc.want)
			case tc.want != "" && !strings.Contains(err.Error(), tc.want):
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestFactChecks_Label(t *testing.T) {
	if got, want := checkLabel(2, factCheck(CheckFactNotAsked, "f1")), "#2 fact_not_asked fact=f1"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
	if got, want := checkLabel(1, factCheck(CheckFactAsked, "f1")), "#1 fact_asked fact=f1"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
	// Existing labels are unchanged.
	if got, want := checkLabel(3, Check{Type: CheckFileExists, Path: "a.txt"}), "#3 file_exists path=a.txt"; got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
}

func TestSimDecisionJSONKeys(t *testing.T) {
	dir := writeSimLog(t, `{"turn":3,"question":"q","fact":"f9","discoverable":true}`+"\n")
	decisions, err := ReadSimLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("got %d decisions, want 1", len(decisions))
	}
	d := decisions[0]
	if d.Turn != 3 || d.Question != "q" || d.Fact != "f9" || d.Discoverable == nil || !*d.Discoverable {
		t.Errorf("decoded %+v", d)
	}
}
