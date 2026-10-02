package tui

import (
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/version"
)

func TestFormatCommitHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"full hash", "a76a9a4b8c3d2e1f", "a76a9a4"},
		{"exactly 7 chars", "a76a9a4", "a76a9a4"},
		{"short hash", "abc12", "abc12"},
		{"empty string", "", "unknown"},
		{"unknown value", "unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := version.FormatCommitHash(tt.input)
			if got != tt.want {
				t.Errorf("FormatCommitHash(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAboutDialog(t *testing.T) {
	dialog := NewAboutDialog()

	// Test BuildContent returns expected structure
	content := dialog.BuildContent()
	if len(content) == 0 {
		t.Error("BuildContent() returned empty content")
	}

	// Verify expected fields are present
	foundVersion := false
	for _, line := range content {
		if strings.Contains(line, "Version:") {
			foundVersion = true
			break
		}
	}
	if !foundVersion {
		t.Error("BuildContent() missing Version field")
	}

	// Test UpdateStatusLine and GetFullContent
	statusLines := []string{"Status: OK", "Last check: now"}
	dialog.UpdateStatusLine(statusLines)

	fullContent := dialog.GetFullContent()
	if len(fullContent) <= len(content) {
		t.Error("GetFullContent() should include status lines")
	}

	// Verify status lines are appended
	foundStatus := false
	for _, line := range fullContent {
		if strings.Contains(line, "Status: OK") {
			foundStatus = true
			break
		}
	}
	if !foundStatus {
		t.Error("GetFullContent() missing appended status lines")
	}
}
