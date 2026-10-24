package boundary

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix   = "github.com/jbrinkman/kairon/"
	internalPrefix = modulePrefix + "internal/"
	selfPkg        = internalPrefix + "boundary"
)

// allowedInternal is every internal package non-test code may import.
var allowedInternal = map[string]bool{
	internalPrefix + "provenance":  true,
	internalPrefix + "failure":     true,
	internalPrefix + "guard":       true,
	internalPrefix + "stageoutput": true,
}

// disallowedDirectImport reports whether a direct import of imp violates the
// package boundary: the standard library, gopkg.in/yaml.v3 and the allowed
// internal packages only. Standard-library paths have no dot in their first
// path segment.
func disallowedDirectImport(imp string) bool {
	if allowedInternal[imp] || imp == "gopkg.in/yaml.v3" {
		return false
	}
	first := imp
	if i := strings.IndexByte(imp, '/'); i >= 0 {
		first = imp[:i]
	}
	return strings.Contains(first, ".")
}

func TestDisallowedDirectImport(t *testing.T) {
	cases := []struct {
		imp  string
		want bool
	}{
		{"fmt", false},
		{"errors", false},
		{"unicode/utf8", false},
		{"gopkg.in/yaml.v3", false},
		{internalPrefix + "provenance", false},
		{internalPrefix + "failure", false},
		{internalPrefix + "guard", false},
		{internalPrefix + "stageoutput", false},
		{internalPrefix + "github", true},
		{internalPrefix + "agent", true},
		{internalPrefix + "watcher", true},
		{internalPrefix + "config", true},
		{"github.com/example/thirdparty", true},
	}
	for _, c := range cases {
		if got := disallowedDirectImport(c.imp); got != c.want {
			t.Errorf("disallowedDirectImport(%q) = %v, want %v", c.imp, got, c.want)
		}
	}
}

// TestNoForbiddenImports keeps the boundary pure: no process, file or network
// access, and no dependency on the packages that call it. Direct imports come
// from go list .Imports (test files excluded) because os and net are reachable
// transitively through fmt.
func TestNoForbiddenImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports: %v\n%s", err, out)
	}
	forbidden := map[string]bool{"os": true, "os/exec": true, "net": true, "net/http": true}
	for _, imp := range strings.Fields(string(out)) {
		if forbidden[imp] {
			t.Errorf("internal/boundary directly imports %q; it must stay free of process, file and network access", imp)
		}
		if disallowedDirectImport(imp) {
			t.Errorf("internal/boundary directly imports %q; it may import only the standard library, gopkg.in/yaml.v3, internal/provenance, internal/failure, internal/guard and internal/stageoutput", imp)
		}
	}

	out, err = exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	banned := []string{internalPrefix + "github", internalPrefix + "agent", internalPrefix + "watcher"}
	var found bool
	for _, pkg := range strings.Fields(string(out)) {
		found = true
		for _, b := range banned {
			if pkg == b {
				t.Errorf("internal/boundary depends (transitively) on %s", b)
			}
		}
	}
	if !found {
		t.Fatal("go list -deps printed no packages")
	}
}
