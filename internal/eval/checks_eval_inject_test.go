package eval

import (
	"os"
	"path/filepath"
	"testing"
)

// injectCheck builds a command check with inject sources taken from hidden.
func injectCheck(hidden, run string, inject ...string) Check {
	return Check{Criterion: "c", Type: CheckCommand, Run: run, Inject: inject, injectDir: hidden}
}

func readInjT(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEvaluateChecks_InjectPresentDuringCommandGoneAfter(t *testing.T) {
	dir, hidden := t.TempDir(), t.TempDir()
	writeEvalFile(t, hidden, "hidden_test.go", "package hidden\n")
	writeEvalFile(t, hidden, "sub/dir/x.txt", "x\n")
	writeEvalFile(t, dir, "sub/keep.txt", "keep\n") // sub pre-exists, sub/dir does not

	r := evalOne(t, injectCheck(hidden,
		"grep -q hidden hidden_test.go && grep -q x sub/dir/x.txt && ls -R > seen.txt",
		"hidden_test.go", "sub/dir/x.txt"), CheckInput{Dir: dir})
	wantPass(t, r)

	if _, err := os.Lstat(filepath.Join(dir, "hidden_test.go")); !os.IsNotExist(err) {
		t.Fatalf("injected file still present: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "sub/dir")); !os.IsNotExist(err) {
		t.Fatalf("injected dir still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub/keep.txt")); err != nil {
		t.Fatalf("pre-existing content removed: %v", err)
	}
	if got := readInjT(t, filepath.Join(dir, "seen.txt")); !contains(got, "hidden_test.go") {
		t.Fatalf("command did not see injected file: %q", got)
	}
}

func TestEvaluateChecks_InjectRestoredOnFailureAndTimeout(t *testing.T) {
	dir, hidden := t.TempDir(), t.TempDir()
	writeEvalFile(t, hidden, "h.txt", "hidden\n")
	in := CheckInput{Dir: dir}

	wantFail(t, evalOne(t, injectCheck(hidden, "exit 5", "h.txt"), in), "exit status 5")
	if _, err := os.Lstat(filepath.Join(dir, "h.txt")); !os.IsNotExist(err) {
		t.Fatalf("not restored after failing command: %v", err)
	}

	old := checkCommandTimeout
	checkCommandTimeout = 300e6
	defer func() { checkCommandTimeout = old }()
	wantFail(t, evalOne(t, injectCheck(hidden, "sleep 30", "h.txt"), in), "timed out")
	if _, err := os.Lstat(filepath.Join(dir, "h.txt")); !os.IsNotExist(err) {
		t.Fatalf("not restored after timeout: %v", err)
	}
}

func TestEvaluateChecks_InjectRestoresPreexistingFile(t *testing.T) {
	dir, hidden := t.TempDir(), t.TempDir()
	writeEvalFile(t, hidden, "h.txt", "FIXTURE\n")
	agent := filepath.Join(dir, "h.txt")
	if err := os.WriteFile(agent, []byte("agent wrote this\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	wantPass(t, evalOne(t, injectCheck(hidden, `test "$(cat h.txt)" = FIXTURE && echo mutated > h.txt`, "h.txt"), CheckInput{Dir: dir}))
	if got := readInjT(t, agent); got != "agent wrote this\n" {
		t.Fatalf("content = %q", got)
	}
	if fi, err := os.Stat(agent); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v err=%v", fi.Mode(), err)
	}
}

func TestEvaluateChecks_InjectCommandReplacesFileWithSymlink(t *testing.T) {
	dir, hidden, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeEvalFile(t, hidden, "h.txt", "FIXTURE\n")
	writeEvalFile(t, outside, "target.txt", "untouched\n")
	agent := filepath.Join(dir, "h.txt")
	if err := os.WriteFile(agent, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := "rm h.txt && ln -s " + filepath.Join(outside, "target.txt") + " h.txt"
	evalOne(t, injectCheck(hidden, run, "h.txt"), CheckInput{Dir: dir})

	if got := readInjT(t, filepath.Join(outside, "target.txt")); got != "untouched\n" {
		t.Fatalf("restore wrote through a symlink: %q", got)
	}
	if fi, err := os.Lstat(agent); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("symlink left behind: %v %v", fi, err)
	}
	if got := readInjT(t, agent); got != "original\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestEvaluateChecks_InjectRefusesSymlinkDestinations(t *testing.T) {
	dir, hidden, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeEvalFile(t, hidden, "out/h.txt", "FIXTURE\n")
	writeEvalFile(t, hidden, "top.txt", "FIXTURE\n")
	writeEvalFile(t, outside, "top.txt", "host file\n")
	if err := os.Symlink(outside, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "top.txt"), filepath.Join(dir, "top.txt")); err != nil {
		t.Fatal(err)
	}
	in := CheckInput{Dir: dir}
	ran := filepath.Join(dir, "ran.txt")

	r := evalOne(t, injectCheck(hidden, "touch ran.txt", "out/h.txt"), in)
	wantFail(t, r, "symlink")
	r = evalOne(t, injectCheck(hidden, "touch ran.txt", "top.txt"), in)
	wantFail(t, r, "symlink")

	if _, err := os.Lstat(filepath.Join(outside, "h.txt")); err == nil {
		t.Fatal("inject wrote through a directory symlink")
	}
	if got := readInjT(t, filepath.Join(outside, "top.txt")); got != "host file\n" {
		t.Fatalf("inject wrote through a file symlink: %q", got)
	}
	if _, err := os.Lstat(ran); err == nil {
		t.Fatal("command ran although injection was refused")
	}
}

func TestEvaluateChecks_InjectRefusesBadSources(t *testing.T) {
	dir, hidden := t.TempDir(), t.TempDir()
	writeEvalFile(t, hidden, ".git/config", "x")
	writeEvalFile(t, hidden, "real.txt", "x")
	in := CheckInput{Dir: dir}
	wantFail(t, evalOne(t, injectCheck(hidden, "true", ".git/config"), in), "reserved")
	wantFail(t, evalOne(t, injectCheck(hidden, "true", "missing.txt"), in), "source")
	wantFail(t, evalOne(t, injectCheck(hidden, "true", "../escape"), in), "inject")
	// Hand-built inject with no hidden directory cannot resolve.
	wantFail(t, evalOne(t, Check{Type: CheckCommand, Run: "true", Inject: []string{"real.txt"}}, in), "hidden")
	// Source swapped for a symlink after load is refused at run time.
	if err := os.Remove(filepath.Join(hidden, "real.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", filepath.Join(hidden, "real.txt")); err != nil {
		t.Fatal(err)
	}
	wantFail(t, evalOne(t, injectCheck(hidden, "true", "real.txt"), in), "symlink")
}
