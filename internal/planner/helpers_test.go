package planner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// repoRoot returns the repository root by walking up from the test's working
// directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// readRepoFile reads a repo-relative file and fails the test if it is missing.
func readRepoFile(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return data
}

// readManifest reads a repo-relative agent manifest as a generic JSON object.
func readManifest(t *testing.T, rel string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(readRepoFile(t, rel), &m); err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	return m
}
