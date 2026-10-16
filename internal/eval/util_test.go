package eval

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMakeUniqueResultsDirAvoidsSecondGranularityCollision pins the reliability
// contract that two runs starting within the same wall-clock second must still
// get two distinct results directories. generateTimestampPrefix has
// one-second granularity (YYMMDD-HHMMSS), so a plain MkdirAll on
// results/<timestamp>-<hash> silently reuses the first run's directory when the
// second run starts in the same second. That made the self-test's two-run tests
// (e.g. TestSelfTestKeptWorkspacesAreClean) flaky in CI: the second run added no
// new directory, so "created 0 result dirs, want 1" fired. makeUniqueResultsDir
// must create a brand-new directory on every call.
func TestMakeUniqueResultsDirAvoidsSecondGranularityCollision(t *testing.T) {
	base := t.TempDir()
	// Same timestamp+hash on both calls, simulating two runs in one second.
	name := "060102-150405-abc123"

	first, err := makeUniqueResultsDir(base, name)
	if err != nil {
		t.Fatalf("first makeUniqueResultsDir: %v", err)
	}
	second, err := makeUniqueResultsDir(base, name)
	if err != nil {
		t.Fatalf("second makeUniqueResultsDir: %v", err)
	}

	if first == second {
		t.Fatalf("two runs in the same second reused one directory: %q", first)
	}
	for _, d := range []string{first, second} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("expected created directory at %q (stat err %v)", d, err)
		}
		if filepath.Dir(d) != base {
			t.Errorf("results dir %q is not under base %q", d, base)
		}
	}

	// Both directories must still be discoverable as timestamped run dirs so
	// diff/latest-run tooling continues to recognise them.
	for _, d := range []string{first, second} {
		if parseDirectoryName(filepath.Base(d)) != "abc123" {
			t.Errorf("dir %q does not parse back to its git hash", filepath.Base(d))
		}
	}
}
