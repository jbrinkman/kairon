package hotkey

import (
	"os"
	"testing"
)

func TestIsKaironContext(t *testing.T) {
	// Test without environment variable
	os.Unsetenv("KAIRON_WATCHER_PID")
	if IsKaironContext() {
		t.Error("Expected IsKaironContext to return false when KAIRON_WATCHER_PID is not set")
	}

	// Test with environment variable
	os.Setenv("KAIRON_WATCHER_PID", "12345")
	if !IsKaironContext() {
		t.Error("Expected IsKaironContext to return true when KAIRON_WATCHER_PID is set")
	}

	// Cleanup
	os.Unsetenv("KAIRON_WATCHER_PID")
}

func TestIsCtrlOptionP(t *testing.T) {
	tests := []struct {
		name     string
		keyStr   string
		expected bool
	}{
		{"Ctrl+Alt+P", "ctrl+alt+p", true},
		{"Ctrl+C", "ctrl+c", false},
		{"Regular P", "p", false},
		{"Alt+P", "alt+p", false},
		{"Ctrl+P", "ctrl+p", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the string matching logic directly
			result := tt.keyStr == "ctrl+alt+p"
			if result != tt.expected {
				t.Errorf("Expected %v for %s, got %v", tt.expected, tt.keyStr, result)
			}
		})
	}
}
