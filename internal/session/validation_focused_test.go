package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnhancedSessionValidation(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()

	tests := []struct {
		name        string
		state       *SessionState
		expectError bool
		errorField  string
	}{
		{
			name: "valid console session",
			state: &SessionState{
				Type: Console,
				History: []Message{
					{Role: "user", Content: "test", Timestamp: time.Now()},
				},
			},
			expectError: false,
		},
		{
			name: "valid planning session",
			state: &SessionState{
				Type:    Planning,
				History: []Message{},
			},
			expectError: false,
		},
		{
			name: "valid system message",
			state: &SessionState{
				Type: Planning,
				History: []Message{
					{Role: "system", Content: "Session terminated", Timestamp: time.Now()},
				},
			},
			expectError: false,
		},
		{
			name: "invalid session type",
			state: &SessionState{
				Type:    "invalid",
				History: []Message{},
			},
			expectError: true,
			errorField:  "type",
		},
		{
			name: "invalid message role",
			state: &SessionState{
				Type: Console,
				History: []Message{
					{Role: "invalid", Content: "test", Timestamp: time.Now()},
				},
			},
			expectError: true,
			errorField:  "history",
		},
		{
			name:        "nil session",
			state:       nil,
			expectError: true,
			errorField:  "state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateSession("test-session", tt.state)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected validation error for %s", tt.name)
					return
				}

				validationErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("Expected ValidationError, got %T: %v", err, err)
					return
				}

				if validationErr.Field != tt.errorField {
					t.Errorf("Expected error field %s, got %s", tt.errorField, validationErr.Field)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error for %s: %v", tt.name, err)
				}
			}
		})
	}
}

func TestSessionDataIntegrityDuringModeSwitch(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()

	// Create initial session with data
	sessionID, err := manager.Create(Console)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Load and add some history
	state, err := manager.Load(sessionID)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}

	originalMessages := []Message{
		{Role: "user", Content: "watch start", Timestamp: time.Now()},
		{Role: "assistant", Content: "Watcher started", Timestamp: time.Now()},
		{Role: "user", Content: "status", Timestamp: time.Now()},
	}

	for _, msg := range originalMessages {
		state.AddMessage(msg.Role, msg.Content)
	}

	if err := manager.Save(sessionID, state); err != nil {
		t.Fatalf("Failed to save session with history: %v", err)
	}

	// Log mode switch with preserved data
	preservedData := map[string]interface{}{
		"inputValue":    "test input",
		"activityLines": []string{"line1", "line2"},
	}
	manager.LogModeSwitch(sessionID, Console, Planning, preservedData)

	// Simulate mode switch by changing session type
	state.Type = Planning
	if err := manager.Save(sessionID, state); err != nil {
		t.Fatalf("Failed to save session after mode switch: %v", err)
	}

	// Validate session flow
	if err := manager.ValidateSessionFlow(sessionID); err != nil {
		t.Errorf("Session flow validation failed: %v", err)
	}

	// Switch back and verify data integrity
	state.Type = Console
	if err := manager.Save(sessionID, state); err != nil {
		t.Fatalf("Failed to save session after switch back: %v", err)
	}

	// Load and verify all data is preserved
	finalState, err := manager.Load(sessionID)
	if err != nil {
		t.Fatalf("Failed to load final session state: %v", err)
	}

	if finalState.Type != Console {
		t.Errorf("Session type not preserved: got %s, want %s", finalState.Type, Console)
	}

	if len(finalState.History) != len(originalMessages) {
		t.Errorf("History length not preserved: got %d, want %d", len(finalState.History), len(originalMessages))
	}

	for i, original := range originalMessages {
		if i >= len(finalState.History) {
			t.Errorf("Missing message %d in final state", i)
			continue
		}

		final := finalState.History[i]
		if final.Role != original.Role || final.Content != original.Content {
			t.Errorf("Message %d not preserved: got {%s: %s}, want {%s: %s}",
				i, final.Role, final.Content, original.Role, original.Content)
		}
	}
}

func TestSequentialModeSwitching(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()

	sessionID, err := manager.Create(Console)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Perform sequential mode switching
	modes := []SessionType{Planning, Console, Planning, Console, Planning}

	for i, mode := range modes {
		// Load session
		state, err := manager.Load(sessionID)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to load session: %v", i, err)
		}

		// Add a message to verify data persistence
		state.AddMessage("user", fmt.Sprintf("message from iteration %d", i))

		// Change mode
		state.Type = mode

		// Save session
		if err := manager.Save(sessionID, state); err != nil {
			t.Fatalf("Iteration %d: Failed to save session: %v", i, err)
		}

		// Validate flow
		if err := manager.ValidateSessionFlow(sessionID); err != nil {
			t.Errorf("Iteration %d: Session flow validation failed: %v", i, err)
		}
	}

	// Final validation - load session and verify integrity
	finalState, err := manager.Load(sessionID)
	if err != nil {
		t.Fatalf("Failed to load session after mode switching: %v", err)
	}

	if finalState.Type != Planning {
		t.Errorf("Final session type incorrect: got %s, want %s", finalState.Type, Planning)
	}

	// Should have messages from the mode switching (at least 5)
	if len(finalState.History) < 5 {
		t.Errorf("Expected at least 5 messages after mode switching, got %d", len(finalState.History))
	}
}

func TestSimpleCorruptionRecovery(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()
	sessionDir := ".kairon/sessions"
	os.MkdirAll(sessionDir, 0755)

	// Test recoverable corruption
	corruptData := `{"type": "console", "history": [{"role": "user", "content": "test"}]}INVALID`
	sessionFile := filepath.Join(sessionDir, "corrupt-test.json")
	os.WriteFile(sessionFile, []byte(corruptData), 0644)

	state, err := manager.Load("corrupt-test")
	if err != nil {
		t.Errorf("Expected recovery but got error: %v", err)
		return
	}
	if state == nil {
		t.Error("Expected recovered state but got nil")
		return
	}

	// Verify recovered state is valid
	if err := manager.ValidateSession("corrupt-test", state); err != nil {
		t.Errorf("Recovered state failed validation: %v", err)
	}

	// Cleanup
	os.Remove(sessionFile)
}

func TestExcessiveHistoryTrimming(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()

	sessionID, err := manager.Create(Console)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	state, err := manager.Load(sessionID)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}

	// Add excessive history (over 10000 messages)
	for i := 0; i < 12000; i++ {
		state.AddMessage("user", fmt.Sprintf("message %d", i))
	}

	// Save and reload to trigger trimming
	if err := manager.Save(sessionID, state); err != nil {
		t.Fatalf("Failed to save session with excessive history: %v", err)
	}

	trimmedState, err := manager.Load(sessionID)
	if err != nil {
		t.Fatalf("Failed to load session after trimming: %v", err)
	}

	if len(trimmedState.History) != 5000 {
		t.Errorf("Expected history to be trimmed to 5000 messages, got %d", len(trimmedState.History))
	}

	// Verify that the kept messages are the most recent ones
	if trimmedState.History[0].Content != "message 7000" {
		t.Errorf("Expected first kept message to be 'message 7000', got %s", trimmedState.History[0].Content)
	}

	if trimmedState.History[4999].Content != "message 11999" {
		t.Errorf("Expected last kept message to be 'message 11999', got %s", trimmedState.History[4999].Content)
	}
}

func TestModeSwitchLogging(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	manager := NewSessionManager()

	sessionID, err := manager.Create(Console)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Test logging with different data types
	preservedData := map[string]interface{}{
		"inputValue":    "test input",
		"activityLines": []string{"line1", "line2", "line3"},
	}

	// This should not panic or error - just logs
	manager.LogModeSwitch(sessionID, Console, Planning, preservedData)
	manager.LogModeSwitch(sessionID, Planning, Console, "simple string data")
	manager.LogModeSwitch(sessionID, Console, Planning, nil)

	// Test flow validation
	if err := manager.ValidateSessionFlow(sessionID); err != nil {
		t.Errorf("Session flow validation failed: %v", err)
	}
}
