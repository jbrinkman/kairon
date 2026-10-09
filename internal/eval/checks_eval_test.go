package eval

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// evalOne evaluates a single check and returns its only result.
func evalOne(t *testing.T, c Check, in CheckInput) CheckResult {
	t.Helper()
	if c.Criterion == "" {
		c.Criterion = "crit"
	}
	rs := EvaluateChecks([]Check{c}, in)
	if len(rs) != 1 {
		t.Fatalf("want 1 result, got %d", len(rs))
	}
	return rs[0]
}

func wantPass(t *testing.T, r CheckResult) {
	t.Helper()
	if !r.Passed {
		t.Fatalf("want pass, got fail: %q (%s)", r.Detail, r.Label)
	}
}

func wantFail(t *testing.T, r CheckResult, detailSub string) {
	t.Helper()
	if r.Passed {
		t.Fatalf("want fail, got pass (%s)", r.Label)
	}
	if r.Detail == "" || !strings.Contains(r.Detail, detailSub) {
		t.Fatalf("detail %q does not contain %q", r.Detail, detailSub)
	}
}

func writeEvalFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func chkGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = hermeticGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// evalRepo creates a git repo with README.md and docs/notes.txt committed and
// returns its directory and the base commit.
func evalRepo(t *testing.T) (dir, base string) {
	t.Helper()
	dir = t.TempDir()
	chkGit(t, dir, "init", "-q")
	writeEvalFile(t, dir, "README.md", "readme\n")
	writeEvalFile(t, dir, "docs/notes.txt", "notes\n")
	chkGit(t, dir, "add", "-A")
	chkGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "base")
	return dir, chkGit(t, dir, "rev-parse", "HEAD")
}

func TestEvaluateChecks_FileExists(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "a/b.txt", "x")
	in := CheckInput{Dir: dir}
	wantPass(t, evalOne(t, Check{Type: CheckFileExists, Path: "a/b.txt"}, in))
	wantPass(t, evalOne(t, Check{Type: CheckFileExists, Path: "a"}, in)) // any entry type
	wantFail(t, evalOne(t, Check{Type: CheckFileExists, Path: "missing.txt"}, in), "does not exist")
	// A dangling symlink still counts as existing.
	if err := os.Symlink(filepath.Join(dir, "nowhere"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	wantPass(t, evalOne(t, Check{Type: CheckFileExists, Path: "dangling"}, in))
}

func TestEvaluateChecks_FileAbsent(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "marker.txt", "x")
	in := CheckInput{Dir: dir}
	wantPass(t, evalOne(t, Check{Type: CheckFileAbsent, Path: "missing.txt"}, in))
	wantFail(t, evalOne(t, Check{Type: CheckFileAbsent, Path: "marker.txt"}, in), "exists")
	// A path below a regular file cannot exist.
	wantPass(t, evalOne(t, Check{Type: CheckFileAbsent, Path: "marker.txt/x"}, in))
}

func TestEvaluateChecks_PathThroughSymlinkRefused(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writeEvalFile(t, outside, "secret.txt", "secret")
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	in := CheckInput{Dir: dir}
	for _, c := range []Check{
		{Type: CheckFileExists, Path: "link/secret.txt"},
		{Type: CheckFileAbsent, Path: "link/nothing.txt"},
		{Type: CheckFileContains, Path: "link/secret.txt", Pattern: "secret"},
		{Type: CheckFileNotContains, Path: "link/secret.txt", Pattern: "zzz"},
	} {
		wantFail(t, evalOne(t, c, in), "symlink")
	}
}

func TestEvaluateChecks_FileContains(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "out.txt", "hello\nworld 42\n")
	in := CheckInput{Dir: dir}
	wantPass(t, evalOne(t, Check{Type: CheckFileContains, Path: "out.txt", Pattern: `world \d+`}, in))
	wantPass(t, evalOne(t, Check{Type: CheckFileContains, Path: "out.txt", Pattern: `(?m)^hello$`}, in))
	wantFail(t, evalOne(t, Check{Type: CheckFileContains, Path: "out.txt", Pattern: `absent`}, in), "not found")
	wantFail(t, evalOne(t, Check{Type: CheckFileContains, Path: "nope.txt", Pattern: `x`}, in), "does not exist")
	wantFail(t, evalOne(t, Check{Type: CheckFileContains, Path: ".", Pattern: `x`}, in), "regular file")
}

func TestEvaluateChecks_FileNotContains(t *testing.T) {
	dir := t.TempDir()
	writeEvalFile(t, dir, "out.txt", "hello world\n")
	in := CheckInput{Dir: dir}
	wantPass(t, evalOne(t, Check{Type: CheckFileNotContains, Path: "out.txt", Pattern: `secret`}, in))
	wantFail(t, evalOne(t, Check{Type: CheckFileNotContains, Path: "out.txt", Pattern: `wor.d`}, in), "matched")
	// A missing file must not pass vacuously.
	wantFail(t, evalOne(t, Check{Type: CheckFileNotContains, Path: "nope.txt", Pattern: `x`}, in), "does not exist")
}

func TestEvaluateChecks_FileContentRefusesSymlinkAndHugeFiles(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writeEvalFile(t, outside, "host.txt", "host secret")
	if err := os.Symlink(filepath.Join(outside, "host.txt"), filepath.Join(dir, "out.txt")); err != nil {
		t.Fatal(err)
	}
	in := CheckInput{Dir: dir}
	wantFail(t, evalOne(t, Check{Type: CheckFileContains, Path: "out.txt", Pattern: `host`}, in), "symlink")
	wantFail(t, evalOne(t, Check{Type: CheckFileNotContains, Path: "out.txt", Pattern: `zzz`}, in), "symlink")

	f, err := os.Create(filepath.Join(dir, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(10<<20 + 1); err != nil { // sparse
		t.Fatal(err)
	}
	f.Close()
	wantFail(t, evalOne(t, Check{Type: CheckFileContains, Path: "big.bin", Pattern: `x`}, in), "10 MiB")
	wantFail(t, evalOne(t, Check{Type: CheckFileNotContains, Path: "big.bin", Pattern: `x`}, in), "10 MiB")

	// Exactly at the cap is read.
	f, err = os.Create(filepath.Join(dir, "cap.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(10 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	wantPass(t, evalOne(t, Check{Type: CheckFileNotContains, Path: "cap.bin", Pattern: `x`}, in))
}

func TestEvaluateChecks_OutputAndGHLog(t *testing.T) {
	in := CheckInput{Output: "## Done\nall good\n", GHLog: "gh issue create --title t\n"}
	wantPass(t, evalOne(t, Check{Type: CheckOutputContains, Pattern: `## Done`}, in))
	wantFail(t, evalOne(t, Check{Type: CheckOutputContains, Pattern: `PASS`}, in), "not found")
	wantPass(t, evalOne(t, Check{Type: CheckOutputNotContains, Pattern: `PASS`}, in))
	wantFail(t, evalOne(t, Check{Type: CheckOutputNotContains, Pattern: `all good`}, in), "matched")

	wantPass(t, evalOne(t, Check{Type: CheckGHLogContains, Pattern: `(?m)^gh issue create`}, in))
	wantFail(t, evalOne(t, Check{Type: CheckGHLogContains, Pattern: `gh pr merge`}, in), "not found")
	wantPass(t, evalOne(t, Check{Type: CheckGHLogNotContains, Pattern: `gh pr merge`}, in))
	wantFail(t, evalOne(t, Check{Type: CheckGHLogNotContains, Pattern: `issue create`}, in), "matched")
	// Empty gh log: not_contains passes vacuously, contains fails.
	empty := CheckInput{}
	wantPass(t, evalOne(t, Check{Type: CheckGHLogNotContains, Pattern: `x`}, empty))
	wantFail(t, evalOne(t, Check{Type: CheckGHLogContains, Pattern: `x`}, empty), "not found")
}

func TestEvaluateChecks_InvalidCheckFailsWithDetail(t *testing.T) {
	in := CheckInput{Dir: t.TempDir(), Output: "x"}
	for name, c := range map[string]Check{
		"unknown type": {Type: "bogus"},
		"empty":        {},
		"bad regex":    {Type: CheckOutputContains, Pattern: "("},
		"missing pat":  {Type: CheckOutputContains},
		"dotdot path":  {Type: CheckFileExists, Path: "../x"},
		"abs path":     {Type: CheckFileExists, Path: "/etc/passwd"},
		"no allow":     {Type: CheckChangedFiles},
		"bad glob":     {Type: CheckChangedFiles, Allow: []string{"["}},
		"no run":       {Type: CheckCommand},
		"stray field":  {Type: CheckFileExists, Path: "a", Run: "true"},
	} {
		t.Run(name, func(t *testing.T) {
			r := evalOne(t, c, in)
			if r.Passed || r.Detail == "" {
				t.Fatalf("invalid check must fail with detail, got %+v", r)
			}
		})
	}
}

func TestEvaluateChecks_ResultsKeepListOrderAndLabels(t *testing.T) {
	rs := EvaluateChecks([]Check{
		{Criterion: "c", Type: CheckOutputContains, Pattern: "x"},
		{Criterion: "c", Type: CheckFileExists, Path: "missing.txt"},
	}, CheckInput{Dir: t.TempDir(), Output: "x"})
	if len(rs) != 2 || rs[0].Index != 1 || rs[1].Index != 2 {
		t.Fatalf("results = %+v", rs)
	}
	if rs[1].Type != CheckFileExists || rs[1].Label != "#2 file_exists path=missing.txt" {
		t.Fatalf("label = %q", rs[1].Label)
	}
	if !rs[0].Passed || rs[1].Passed {
		t.Fatalf("passed = %v %v", rs[0].Passed, rs[1].Passed)
	}
	if got := EvaluateChecks(nil, CheckInput{}); got == nil || len(got) != 0 {
		t.Fatalf("nil checks -> %#v", got)
	}
}

func TestEvaluateChecks_MissingDirFailsFileChecks(t *testing.T) {
	wantFail(t, evalOne(t, Check{Type: CheckFileExists, Path: "a"}, CheckInput{}), "directory")
	wantFail(t, evalOne(t, Check{Type: CheckCommand, Run: "true"}, CheckInput{}), "directory")
	wantFail(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: filepath.Join(t.TempDir(), "nope")}), "directory")
}
