package stageoutput

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoInternalDeps keeps this package a leaf: it must not link any other
// internal/* package (in particular internal/watcher or internal/agent), so
// stages and guards can import it without an import cycle.
func TestNoInternalDeps(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	const (
		internalPrefix = "github.com/jbrinkman/kairon/internal/"
		self           = "github.com/jbrinkman/kairon/internal/stageoutput"
	)
	var found bool
	for _, pkg := range strings.Fields(string(out)) {
		found = true
		if pkg == self {
			continue
		}
		if strings.HasPrefix(pkg, internalPrefix) {
			t.Errorf("internal/stageoutput depends on %s; it must not link any other internal package", pkg)
		}
	}
	if !found {
		t.Fatal("go list -deps printed no packages")
	}
}
