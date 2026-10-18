package issueprov

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

const modPath = "github.com/jbrinkman/kairon/internal/"

// forbiddenDeps returns the packages in deps that are internal/watcher or
// internal/eval, or one of their subpackages.
func forbiddenDeps(deps []string) []string {
	var bad []string
	for _, pkg := range deps {
		for _, f := range []string{"watcher", "eval"} {
			root := modPath + f
			if pkg == root || strings.HasPrefix(pkg, root+"/") {
				bad = append(bad, pkg)
			}
		}
	}
	return bad
}

func TestForbiddenDeps(t *testing.T) {
	deps := []string{
		"strings",
		modPath + "watcher",
		modPath + "watcher/sub",
		modPath + "eval",
		modPath + "eval/sandbox",
		modPath + "evaluator", // sibling with a shared prefix, not a subpackage
		modPath + "checks",
	}
	want := []string{modPath + "watcher", modPath + "watcher/sub", modPath + "eval", modPath + "eval/sandbox"}
	if got := forbiddenDeps(deps); !reflect.DeepEqual(got, want) {
		t.Errorf("forbiddenDeps = %v, want %v", got, want)
	}
}

// TestNoWatcherOrEvalDeps keeps the marker package a leaf that the planner and
// the watcher can both import: it must not link internal/watcher or
// internal/eval.
func TestNoWatcherOrEvalDeps(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	deps := strings.Fields(string(out))
	if len(deps) == 0 {
		t.Fatal("go list -deps printed no packages")
	}
	for _, pkg := range forbiddenDeps(deps) {
		t.Errorf("internal/issueprov depends on %s; it must not link internal/watcher or internal/eval", pkg)
	}
}
