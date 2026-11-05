// Package kaironbintest holds assertions shared by the tests of every place
// that starts an agent process, so each spawn site checks the same contract.
package kaironbintest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/kaironbin"
)

// AssertEnv fails t unless env has the Kairon bin directory as its first PATH
// entry, that directory's kairon resolves to the running binary, and
// KAIRON_BIN is the running binary's resolved path. Each variable must appear
// exactly once.
func AssertEnv(t testing.TB, env []string) {
	t.Helper()
	dir, err := kaironbin.Dir()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := kaironbin.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path, ok := single(t, env, "PATH")
	if ok {
		if first := strings.SplitN(path, string(os.PathListSeparator), 2)[0]; first != dir {
			t.Errorf("first PATH entry = %q, want the Kairon bin directory %q", first, dir)
		}
	}
	if bin, ok := single(t, env, kaironbin.EnvVar); ok && bin != exe {
		t.Errorf("%s = %q, want the running binary %q", kaironbin.EnvVar, bin, exe)
	}
	if err := kaironbin.Install(); err != nil {
		t.Fatal(err)
	}
	AssertLinkTargetsRunningBinary(t, filepath.Join(dir, "kairon"))
}

// AssertLinkTargetsRunningBinary fails t unless link resolves to the running
// binary.
func AssertLinkTargetsRunningBinary(t testing.TB, link string) {
	t.Helper()
	exe, err := kaironbin.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("resolving %s: %v", link, err)
	}
	if got != exe {
		t.Errorf("%s resolves to %q, want the running binary %q", link, got, exe)
	}
}

func single(t testing.TB, env []string, key string) (string, bool) {
	t.Helper()
	var vals []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			vals = append(vals, v)
		}
	}
	if len(vals) != 1 {
		t.Errorf("env has %d %s entries %q, want exactly 1", len(vals), key, vals)
		return "", false
	}
	return vals[0], true
}
