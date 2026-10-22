package planner

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoForbiddenImports keeps the boundary free of process, network and key
// access. It checks the direct-import list (go list .Imports, which ignores
// test files): os/exec is reachable transitively through internal/failure's
// config dependency, so the full closure is not checked. Anything that needs
// a process or the network (gh, git) lives behind the provenance.IssueStore,
// provenance.Router and failure.Issues interfaces.
func TestNoForbiddenImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports: %v\n%s", err, out)
	}
	imports := strings.Fields(string(out))
	if len(imports) == 0 {
		t.Fatal("go list printed no imports")
	}
	forbidden := map[string]bool{"os/exec": true, "net/http": true, "crypto/hmac": true}
	for _, imp := range imports {
		if forbidden[imp] {
			t.Errorf("internal/planner directly imports %q; the boundary must stay free of process, network and key access", imp)
		}
	}
}

func TestDisallowedImportsListIsExercised(t *testing.T) {
	// Guard against the test above silently checking nothing: the package
	// does import the packages it is built on.
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports: %v\n%s", err, out)
	}
	for _, want := range []string{
		"github.com/jbrinkman/kairon/internal/guard",
		"github.com/jbrinkman/kairon/internal/provenance",
		"github.com/jbrinkman/kairon/internal/failure",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("expected internal/planner to import %s", want)
		}
	}
}
