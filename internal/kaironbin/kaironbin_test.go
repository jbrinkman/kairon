package kaironbin_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jbrinkman/kairon/internal/kaironbin"
	"github.com/jbrinkman/kairon/internal/kaironbin/kaironbintest"
)

func TestInstallExposesOnlyTheRunningBinary(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := kaironbin.Install(); err != nil {
		t.Fatal(err)
	}
	dir, err := kaironbin.Dir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"kairon"}) {
		t.Fatalf("%s holds %q, want exactly [kairon]", dir, names)
	}
	kaironbintest.AssertLinkTargetsRunningBinary(t, filepath.Join(dir, "kairon"))
}

func TestInstallReplacesALinkLeftByAnOlderBinary(t *testing.T) {
	t.Chdir(t.TempDir())
	old := filepath.Join(t.TempDir(), "old-kairon")
	if err := os.WriteFile(old, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(".kairon", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(old, filepath.Join(".kairon", "bin", "kairon")); err != nil {
		t.Fatal(err)
	}
	if err := kaironbin.Install(); err != nil {
		t.Fatal(err)
	}
	dir, _ := kaironbin.Dir()
	kaironbintest.AssertLinkTargetsRunningBinary(t, filepath.Join(dir, "kairon"))
}

func TestApplyPutsBinDirFirstOnPathAndSetsKaironBin(t *testing.T) {
	t.Chdir(t.TempDir())
	env := kaironbin.Apply([]string{"HOME=/h", "PATH=/usr/bin:/bin", "KAIRON_BIN=/stale"})
	kaironbintest.AssertEnv(t, env)

	var paths []string
	for _, kv := range env {
		if len(kv) > 5 && kv[:5] == "PATH=" {
			paths = append(paths, kv)
		}
	}
	dir, _ := kaironbin.Dir()
	if want := "PATH=" + dir + string(os.PathListSeparator) + "/usr/bin:/bin"; !slices.Equal(paths, []string{want}) {
		t.Errorf("PATH entries = %q, want [%q]", paths, want)
	}
	if !slices.Contains(env, "HOME=/h") {
		t.Errorf("env = %q, other variables must be kept", env)
	}
}

func TestApplyWithoutPathSetsBinDirAlone(t *testing.T) {
	t.Chdir(t.TempDir())
	env := kaironbin.Apply(nil)
	dir, _ := kaironbin.Dir()
	if !slices.Contains(env, "PATH="+dir) {
		t.Errorf("env = %q, want PATH=%s", env, dir)
	}
}
