package guard

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoForbiddenImports keeps the guard package deterministic and free of
// agent-spawn, GitHub and shared-state write paths. It may import only the
// standard library and internal/stageoutput, and must not import os, os/exec,
// net or net/http directly. Those four are reachable transitively through fmt
// and yaml.v3, so the direct-import list (go list .Imports) is checked rather
// than the full dependency set.
// disallowedDirectImport reports whether a direct import of imp violates the
// guard package's boundary: it may import only the standard library and
// internal/stageoutput. Standard-library paths have no dot in their first path
// segment (no module domain like "github.com"); everything else is a
// third-party or disallowed internal import.
func disallowedDirectImport(imp string) bool {
	if imp == "github.com/jbrinkman/kairon/internal/stageoutput" {
		return false
	}
	first := imp
	if i := strings.IndexByte(imp, '/'); i >= 0 {
		first = imp[:i]
	}
	if strings.Contains(first, ".") {
		return true // has a module domain: third-party or non-allowlisted internal
	}
	return false // standard library
}

func TestDisallowedDirectImport(t *testing.T) {
	cases := []struct {
		imp  string
		want bool
	}{
		{"fmt", false},
		{"os/exec", false}, // stdlib by path shape; the targeted check below rejects it
		{"regexp", false},
		{"github.com/jbrinkman/kairon/internal/stageoutput", false},
		{"github.com/jbrinkman/kairon/internal/guard", true},
		{"github.com/example/thirdparty", true},
		{"gopkg.in/yaml.v3", true},
	}
	for _, c := range cases {
		if got := disallowedDirectImport(c.imp); got != c.want {
			t.Errorf("disallowedDirectImport(%q) = %v, want %v", c.imp, got, c.want)
		}
	}
}

func TestNoForbiddenImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports: %v\n%s", err, out)
	}
	forbidden := map[string]bool{"os": true, "os/exec": true, "net": true, "net/http": true}
	for _, imp := range strings.Fields(string(out)) {
		if forbidden[imp] {
			t.Errorf("internal/guard directly imports %q; it must stay free of process, file and network access", imp)
		}
		if disallowedDirectImport(imp) {
			t.Errorf("internal/guard directly imports %q; it may import only the standard library and internal/stageoutput", imp)
		}
	}

	out, err = exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	const (
		internalPrefix = "github.com/jbrinkman/kairon/internal/"
		self           = "github.com/jbrinkman/kairon/internal/guard"
		allowed        = "github.com/jbrinkman/kairon/internal/stageoutput"
	)
	var found bool
	for _, pkg := range strings.Fields(string(out)) {
		found = true
		if pkg == self || pkg == allowed {
			continue
		}
		if strings.HasPrefix(pkg, internalPrefix) {
			t.Errorf("internal/guard depends on %s; only internal/stageoutput is allowed among internal packages", pkg)
		}
	}
	if !found {
		t.Fatal("go list -deps printed no packages")
	}
}
