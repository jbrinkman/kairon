package sandbox

import (
	"sort"
	"testing"

	"github.com/jbrinkman/kairon/internal/config"
)

// TestReservedWorkspacePathsMatchTmpfs guards against drift between the tmpfs
// mount targets this package lays down and the reserved-path list the config
// package rejects for sandbox.workspace_dir. If a new tmpfs mount is added
// here without updating config.ReservedSandboxWorkspacePaths (or vice versa),
// a workspace_dir colliding with it would no longer be caught at config load.
func TestReservedWorkspacePathsMatchTmpfs(t *testing.T) {
	tmpfs := tmpfsMounts()
	got := make([]string, 0, len(tmpfs))
	for p := range tmpfs {
		got = append(got, p)
	}
	want := config.ReservedSandboxWorkspacePaths()

	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("tmpfs mount targets %v and config reserved paths %v differ in length", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("tmpfs mount targets %v do not match config reserved paths %v (first diff at %d: %q vs %q)",
				got, want, i, got[i], want[i])
		}
	}
}
