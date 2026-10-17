package eval

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadGHLogDetectsOversize verifies readGHLog flags a log past the cap and
// returns only the truncated prefix, while an at-cap log is not flagged.
func TestReadGHLogDetectsOversize(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, ".eval", "gh.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}

	// Over the cap by one byte: flagged, content truncated to the cap.
	if err := os.WriteFile(logPath, make([]byte, maxGHLogBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	got, oversized := readGHLog(dir)
	if !oversized {
		t.Fatal("over-cap log not flagged oversized")
	}
	if len(got) != maxGHLogBytes {
		t.Fatalf("truncated length = %d, want %d", len(got), maxGHLogBytes)
	}

	// Exactly at the cap: not flagged.
	if err := os.WriteFile(logPath, make([]byte, maxGHLogBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, oversized := readGHLog(dir); oversized {
		t.Fatal("at-cap log wrongly flagged oversized")
	}
}
