package provenance

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix   = "github.com/jbrinkman/kairon/"
	internalPrefix = modulePrefix + "internal/"
	selfPkg        = internalPrefix + "provenance"
	guardPkg       = internalPrefix + "guard"
	stageoutputPkg = internalPrefix + "stageoutput"
)

// disallowedDirectImport reports whether a direct import of imp violates the
// package boundary: it may import only the standard library, internal/guard
// and internal/stageoutput. Standard-library paths have no dot in their first
// path segment (no module domain like "github.com"); everything else is a
// third-party or non-allowlisted internal import.
func disallowedDirectImport(imp string) bool {
	if imp == guardPkg || imp == stageoutputPkg {
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
		{"crypto/sha256", false},
		{"regexp", false},
		{guardPkg, false},
		{stageoutputPkg, false},
		{internalPrefix + "github", true},
		{internalPrefix + "watcher", true},
		{internalPrefix + "agent", true},
		{"github.com/example/thirdparty", true},
		{"gopkg.in/yaml.v3", true},
	}
	for _, c := range cases {
		if got := disallowedDirectImport(c.imp); got != c.want {
			t.Errorf("disallowedDirectImport(%q) = %v, want %v", c.imp, got, c.want)
		}
	}
}

// TestNoForbiddenImports keeps the provenance package pure: it must not read
// the environment, files, the network or key material. The direct-import list
// (go list .Imports, which ignores test files) is checked for the forbidden
// packages, because os and net are reachable transitively through fmt. The
// full dependency set is checked for stray internal packages.
func TestNoForbiddenImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports: %v\n%s", err, out)
	}
	forbidden := map[string]bool{
		"os": true, "os/exec": true, "net": true, "net/http": true, "crypto/hmac": true,
	}
	for _, imp := range strings.Fields(string(out)) {
		if forbidden[imp] {
			t.Errorf("internal/provenance directly imports %q; it must stay free of process, file, network and key access", imp)
		}
		if disallowedDirectImport(imp) {
			t.Errorf("internal/provenance directly imports %q; it may import only the standard library, internal/guard and internal/stageoutput", imp)
		}
	}

	out, err = exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	var found bool
	for _, pkg := range strings.Fields(string(out)) {
		found = true
		if pkg == selfPkg || pkg == guardPkg || pkg == stageoutputPkg {
			continue
		}
		if strings.HasPrefix(pkg, internalPrefix) {
			t.Errorf("internal/provenance depends on %s; only internal/guard and internal/stageoutput are allowed among internal packages", pkg)
		}
	}
	if !found {
		t.Fatal("go list -deps printed no packages")
	}
}
