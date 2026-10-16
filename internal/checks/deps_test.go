package checks

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoHarnessDeps keeps the check engine independent of the eval harness:
// internal/checks must not link internal/eval or internal/eval/sandbox, so the
// workflow engine can reuse the checks without the harness.
func TestNoHarnessDeps(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	const mod = "github.com/jbrinkman/kairon/internal/eval"
	var found bool
	for _, pkg := range strings.Fields(string(out)) {
		found = true
		if pkg == mod || strings.HasPrefix(pkg, mod+"/") {
			t.Errorf("internal/checks depends on %s; the check engine must not link the eval harness", pkg)
		}
	}
	if !found {
		t.Fatal("go list -deps printed no packages")
	}
}
