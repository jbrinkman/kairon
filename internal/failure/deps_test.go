package failure

import (
	"os/exec"
	"strings"
	"testing"
)

// disallowedDirectImport reports whether a direct import violates the package
// boundary: the standard library plus internal/guard, internal/stageoutput and
// internal/config only.
func disallowedDirectImport(imp string) bool {
	switch imp {
	case "github.com/jbrinkman/kairon/internal/guard",
		"github.com/jbrinkman/kairon/internal/stageoutput",
		"github.com/jbrinkman/kairon/internal/config":
		return false
	}
	first := imp
	if i := strings.IndexByte(imp, '/'); i >= 0 {
		first = imp[:i]
	}
	return strings.Contains(first, ".") // module domain: third-party or other internal
}

func TestDisallowedDirectImport(t *testing.T) {
	cases := []struct {
		imp  string
		want bool
	}{
		{"fmt", false},
		{"crypto/rand", false},
		{"github.com/jbrinkman/kairon/internal/guard", false},
		{"github.com/jbrinkman/kairon/internal/stageoutput", false},
		{"github.com/jbrinkman/kairon/internal/config", false},
		{"github.com/jbrinkman/kairon/internal/github", true},
		{"github.com/jbrinkman/kairon/internal/agent", true},
		{"gopkg.in/yaml.v3", true},
	}
	for _, c := range cases {
		if got := disallowedDirectImport(c.imp); got != c.want {
			t.Errorf("disallowedDirectImport(%q) = %v, want %v", c.imp, got, c.want)
		}
	}
}

// TestNoForbiddenImports keeps the package free of process, file and network
// access. Those packages are reachable transitively through fmt and config, so
// the direct-import list (go list .Imports) is checked, not the full closure.
func TestNoForbiddenImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports: %v\n%s", err, out)
	}
	forbidden := map[string]bool{"os": true, "os/exec": true, "net": true, "net/http": true}
	imports := strings.Fields(string(out))
	if len(imports) == 0 {
		t.Fatal("go list printed no imports")
	}
	for _, imp := range imports {
		if forbidden[imp] {
			t.Errorf("internal/failure directly imports %q; it must stay free of process, file and network access", imp)
		}
		if disallowedDirectImport(imp) {
			t.Errorf("internal/failure directly imports %q; allowed: stdlib, internal/guard, internal/stageoutput, internal/config", imp)
		}
	}
}
