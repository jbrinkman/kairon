package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateChecks_ChangedFilesClean(t *testing.T) {
	dir, base := evalRepo(t)
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: dir, Base: base}))
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: dir})) // Base defaults to HEAD
}

func TestEvaluateChecks_ChangedFilesKinds(t *testing.T) {
	dir, base := evalRepo(t)
	writeEvalFile(t, dir, "README.md", "changed\n")                         // modified
	if err := os.Remove(filepath.Join(dir, "docs/notes.txt")); err != nil { // deleted
		t.Fatal(err)
	}
	writeEvalFile(t, dir, "staged.txt", "s\n") // added (staged)
	chkGit(t, dir, "add", "staged.txt")
	writeEvalFile(t, dir, "extra/new.txt", "n\n") // untracked
	in := CheckInput{Dir: dir, Base: base}

	r := evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, in)
	wantFail(t, r, "README.md")
	for _, want := range []string{"modified README.md", "deleted docs/notes.txt", "added staged.txt", "untracked extra/new.txt"} {
		wantFail(t, r, want)
	}

	// Allow globs cover exactly what they match.
	r = evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{"README.md", "docs/**", "staged.txt"}}, in)
	wantFail(t, r, "extra/new.txt")
	if got := r.Detail; contains(got, "modified README.md") || contains(got, "deleted docs/notes.txt") || contains(got, "added staged.txt") {
		t.Fatalf("allowed paths must not be reported: %q", got)
	}
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{"README.md", "docs/**", "staged.txt", "extra/**"}}, in))
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{"**"}}, in))
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestEvaluateChecks_ChangedFilesIgnoresEvalDir(t *testing.T) {
	dir, base := evalRepo(t)
	writeEvalFile(t, dir, ".eval/gh.log", "log\n")
	writeEvalFile(t, dir, ".eval/note.txt", "n\n")
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: dir, Base: base}))
	// Even when .eval/ is not excluded by git and gets staged.
	chkGit(t, dir, "add", "-f", ".eval")
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: dir, Base: base}))
}

func TestEvaluateChecks_ChangedFilesDetectsAgentCommit(t *testing.T) {
	dir, base := evalRepo(t)
	writeEvalFile(t, dir, "README.md", "committed change\n")
	chkGit(t, dir, "add", "-A")
	chkGit(t, dir, "-c", "user.name=a", "-c", "user.email=a@example.com", "commit", "-q", "-m", "agent")

	wantFail(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: dir, Base: base}), "modified README.md")
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{"README.md"}}, CheckInput{Dir: dir, Base: base}))
	// Without Base the default is HEAD, which already contains the commit.
	wantPass(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: dir}))
}

func TestEvaluateChecks_ChangedFilesGitErrors(t *testing.T) {
	// Not a git repository.
	wantFail(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: t.TempDir()}), "git")

	dir, _ := evalRepo(t)
	// A directory inside a repo that is not itself the repo root.
	sub := filepath.Join(dir, "docs")
	wantFail(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{"**"}}, CheckInput{Dir: sub}), "repository root")
	// Unknown base revision and option-looking base.
	wantFail(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{"**"}}, CheckInput{Dir: dir, Base: "no-such-rev"}), "git")
	wantFail(t, evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{"**"}}, CheckInput{Dir: dir, Base: "--output=pwned"}), "base")
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Fatal("option injection via Base wrote a file")
	}
}

func TestEvaluateChecks_ChangedFilesIgnoresRepoConfigCommands(t *testing.T) {
	dir, base := evalRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	// Agent-controlled repo config must not get to execute anything.
	chkGit(t, dir, "config", "core.fsmonitor", "touch "+marker+"; echo")
	chkGit(t, dir, "config", "diff.external", "touch "+marker)
	writeEvalFile(t, dir, "README.md", "changed\n")
	evalOne(t, Check{Type: CheckChangedFiles, Allow: []string{}}, CheckInput{Dir: dir, Base: base})
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("repository config command was executed")
	}
}
