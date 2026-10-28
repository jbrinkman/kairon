package session

import (
	"os"
	"testing"
	"time"
)

func TestSessionManagerCRUD(t *testing.T) {
	// Use temp directory for testing
	tempDir := t.TempDir()
	sm := NewSessionManager()
	sm.sessionsDir = tempDir

	// Test Create
	id, err := sm.Create(Console)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned empty ID")
	}

	// Test Load
	state, err := sm.Load(id)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if state.Type != Console {
		t.Errorf("Expected session type %v, got %v", Console, state.Type)
	}

	// Test Save with modifications
	state.AddMessage("user", "test message")
	err = sm.Save(id, state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify modifications persisted
	state2, err := sm.Load(id)
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	if len(state2.History) != 1 {
		t.Errorf("Expected 1 message, got %d", len(state2.History))
	} else if state2.History[0].Role != "user" || state2.History[0].Content != "test message" {
		t.Errorf("First message not restored correctly: got role %q, content %q", state2.History[0].Role, state2.History[0].Content)
	}
	if state2.Type != Console {
		t.Errorf("Expected reloaded session type %v, got %v", Console, state2.Type)
	}

	// Test List
	sessions, err := sm.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(sessions) != 1 || sessions[0] != id {
		t.Errorf("Expected sessions [%s], got %v", id, sessions)
	}

	// Test Delete
	err = sm.Delete(id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	sessions, err = sm.List()
	if err != nil {
		t.Fatalf("List after delete failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Expected no sessions after delete, got %v", sessions)
	}
}

func TestSessionManagerCleanup(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewSessionManager()
	sm.sessionsDir = tempDir

	// Create two sessions
	id1, _ := sm.Create(Console)
	id2, _ := sm.Create(Planning)

	// Make first session appear old by modifying its file timestamp
	filename := tempDir + "/" + id1 + ".json"
	oldTime := time.Now().Add(-25 * time.Hour)
	os.Chtimes(filename, oldTime, oldTime)

	// Cleanup sessions older than 24 hours
	err := sm.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify only new session remains
	sessions, err := sm.List()
	if err != nil {
		t.Fatalf("List after cleanup failed: %v", err)
	}
	if len(sessions) != 1 || sessions[0] != id2 {
		t.Errorf("Expected sessions [%s], got %v", id2, sessions)
	}
}
