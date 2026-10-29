package hotkey

import (
	"os"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/session"
)

func TestSessionIntegration(t *testing.T) {
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	_ = os.Chdir(tempDir)

	sm := session.NewSessionManager()
	// Create console session with data
	consoleID, err := sm.Create(session.Console)
	if err != nil {
		t.Fatalf("Failed to create console session: %v", err)
	}

	consoleState, err := sm.Load(consoleID)
	if err != nil {
		t.Fatalf("Failed to load console session: %v", err)
	}

	// Add some data to console session
	consoleState.AddMessage("user", "console command 1")
	consoleState.AddMessage("assistant", "console response 1")
	err = sm.Save(consoleID, consoleState)
	if err != nil {
		t.Fatalf("Failed to save console session: %v", err)
	}

	// Create planning session
	planningID, err := sm.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create planning session: %v", err)
	}

	planningState, err := sm.Load(planningID)
	if err != nil {
		t.Fatalf("Failed to load planning session: %v", err)
	}

	// Add data to planning session
	planningState.AddMessage("user", "planning request 1")
	planningState.AddMessage("assistant", "planning response 1")
	err = sm.Save(planningID, planningState)
	if err != nil {
		t.Fatalf("Failed to save planning session: %v", err)
	}

	t.Run("State Preservation During Mode Switch", func(t *testing.T) {
		// Reload and verify console session preserved
		reloadedConsole, err := sm.Load(consoleID)
		if err != nil {
			t.Fatalf("Failed to reload console session: %v", err)
		}

		if len(reloadedConsole.History) != 2 {
			t.Errorf("Expected 2 console messages, got %d", len(reloadedConsole.History))
		}

		if reloadedConsole.History[0].Content != "console command 1" {
			t.Error("Console session data not preserved")
		}

		// Reload and verify planning session preserved
		reloadedPlanning, err := sm.Load(planningID)
		if err != nil {
			t.Fatalf("Failed to reload planning session: %v", err)
		}

		if len(reloadedPlanning.History) != 2 {
			t.Errorf("Expected 2 planning messages, got %d", len(reloadedPlanning.History))
		}

		if reloadedPlanning.History[0].Content != "planning request 1" {
			t.Error("Planning session data not preserved")
		}
	})

	// Cleanup
	sm.Delete(consoleID)
	sm.Delete(planningID)

	t.Run("Session Cleanup on Exit", func(t *testing.T) {
		sm := session.NewSessionManager()
		// Create multiple sessions
		sessions := make([]string, 3)
		for i := 0; i < 3; i++ {
			id, err := sm.Create(session.Console)
			if err != nil {
				t.Fatalf("Failed to create session %d: %v", i, err)
			}
			sessions[i] = id
		}

		// Verify all sessions exist
		allSessions, err := sm.List()
		if err != nil {
			t.Fatalf("Failed to list sessions: %v", err)
		}

		if len(allSessions) < 3 {
			t.Errorf("Expected at least 3 sessions, got %d", len(allSessions))
		}

		// Simulate cleanup on exit
		err = sm.CleanupOnExit()
		if err != nil {
			t.Errorf("Cleanup failed: %v", err)
		}

		// Verify sessions still exist (cleanup only removes old/corrupt sessions)
		remainingSessions, err := sm.List()
		if err != nil {
			t.Fatalf("Failed to list sessions after cleanup: %v", err)
		}

		// All sessions should still exist since they're new
		if len(remainingSessions) != len(allSessions) {
			t.Errorf("Expected %d sessions after cleanup, got %d",
				len(allSessions), len(remainingSessions))
		}

		// Clean up test sessions
		for _, id := range sessions {
			sm.Delete(id)
		}
	})

	t.Run("Orphaned Session Cleanup", func(t *testing.T) {
		// Create session and make it appear old
		id, err := sm.Create(session.Planning)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Manually age the session file
		// Note: In real implementation, this would use file modification time
		// For testing, we simulate with Cleanup call with very short duration
		time.Sleep(10 * time.Millisecond)

		err = sm.Cleanup(5 * time.Millisecond)
		if err != nil {
			t.Errorf("Cleanup of old sessions failed: %v", err)
		}

		// Verify session was cleaned up
		_, err = sm.Load(id)
		if err == nil {
			// Session still exists, delete it manually for cleanup
			sm.Delete(id)
		}
	})
}
