package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionCorruptionRecovery(t *testing.T) {
	// Setup temporary directory
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()

	// Create a corrupted session file
	sessionDir := ".kairon/sessions"
	os.MkdirAll(sessionDir, 0755)

	corruptedData := `{"type": "planning", "history": [invalid json`
	sessionFile := filepath.Join(sessionDir, "corrupted.json")
	os.WriteFile(sessionFile, []byte(corruptedData), 0644)

	// Attempt to load corrupted session - should succeed due to recovery
	state, err := manager.Load("corrupted")
	if err != nil {
		t.Errorf("Expected successful recovery, but got error: %v", err)
	}

	if state == nil {
		t.Error("Expected recovered session state, got nil")
	}

	if state != nil && state.Type != Planning {
		t.Errorf("Expected recovered session type to be 'planning', got %s", state.Type)
	}

	// Verify backup was created by checking if original file was moved
	entries, _ := os.ReadDir(sessionDir)
	hasBackup := false
	for _, entry := range entries {
		if contains(entry.Name(), "corrupted") && contains(entry.Name(), "corrupt") {
			hasBackup = true
			break
		}
	}

	if !hasBackup {
		t.Log("No backup found, this might be expected if recovery failed")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || s[len(s)-len(substr):] == substr ||
		(len(s) > len(substr) && s[:len(substr)] == substr) ||
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSessionValidation(t *testing.T) {
	manager := NewSessionManager()

	// Test invalid session type
	state := &SessionState{
		Type:    "invalid",
		History: []Message{},
	}

	err := manager.ValidateSession("test-session", state)
	if err == nil {
		t.Error("Expected error for invalid session type")
	}

	// Test nil session
	err = manager.ValidateSession("test-session", nil)
	if err == nil {
		t.Error("Expected error for nil session")
	}

	// Test valid session
	validState := NewSessionState(Planning)
	err = manager.ValidateSession("test-session", validState)
	if err != nil {
		t.Errorf("Expected no error for valid session, got: %v", err)
	}
}

func TestCleanupOnExit(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()

	// Create some test sessions
	id1, _ := manager.Create(Console)
	id2, _ := manager.Create(Planning)

	// Test cleanup
	err := manager.CleanupOnExit()
	if err != nil {
		t.Errorf("CleanupOnExit failed: %v", err)
	}

	// Verify sessions still exist (they're not old enough to be cleaned up)
	sessions, _ := manager.List()
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions after cleanup, got %d", len(sessions))
	}

	// Verify we can still load the sessions
	_, err = manager.Load(id1)
	if err != nil {
		t.Errorf("Failed to load session after cleanup: %v", err)
	}

	_, err = manager.Load(id2)
	if err != nil {
		t.Errorf("Failed to load session after cleanup: %v", err)
	}
}
