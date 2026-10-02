package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jbrinkman/kairon/internal/logging"
)

// SessionManager handles CRUD operations and persistence for sessions
type SessionManager struct {
	sessionsDir string
}

// ValidationError represents a session validation failure
type ValidationError struct {
	SessionID string
	Field     string
	Message   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("session %s: %s validation failed: %s", e.SessionID, e.Field, e.Message)
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessionsDir: ".kairon/sessions",
	}
}

// NewSessionManagerWithDir creates a SessionManager using the specified directory
func NewSessionManagerWithDir(dir string) *SessionManager {
	return &SessionManager{
		sessionsDir: dir,
	}
}

// Create creates a new session and returns its ID
func (sm *SessionManager) Create(sessionType SessionType) (string, error) {
	logging.Info("creating new session", "type", sessionType)

	// Generate session ID
	id, err := generateSessionID()
	if err != nil {
		logging.Error("failed to generate session ID", "error", err)
		return "", err
	}

	// Create session state
	state := NewSessionState(sessionType)

	// Save to disk
	err = sm.Save(id, state)
	if err != nil {
		logging.Error("failed to save new session", "session_id", id, "error", err)
		return "", err
	}

	logging.Info("session created", "session_id", id, "type", sessionType)
	return id, nil
}

// Save persists a session to disk with validation
func (sm *SessionManager) Save(id string, state *SessionState) error {
	logging.Debug("saving session", "session_id", id, "type", state.Type, "message_count", len(state.History))

	// Repair before validation to enforce limits
	sm.RepairSession(id, state)

	// Validate session before saving
	if err := sm.ValidateSession(id, state); err != nil {
		logging.Error("session validation failed", "session_id", id, "error", err)
		return fmt.Errorf("session validation failed: %w", err)
	}

	if err := sm.writeSession(id, state); err != nil {
		logging.Error("failed to write session", "session_id", id, "error", err)
		return err
	}

	logging.Debug("session saved", "session_id", id)
	return nil
}

// SaveQuiet persists a session to disk without full validation (for high-frequency background saves)
func (sm *SessionManager) SaveQuiet(id string, state *SessionState) error {
	if state == nil {
		return fmt.Errorf("cannot save nil session state")
	}
	return sm.writeSession(id, state)
}

// writeSession handles the atomic file write
func (sm *SessionManager) writeSession(id string, state *SessionState) error {
	// Ensure sessions directory exists
	err := os.MkdirAll(sm.sessionsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Serialize session
	data, err := state.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize session: %w", err)
	}

	// Write to file atomically (write to temp file first, then rename)
	filename := filepath.Join(sm.sessionsDir, id+".json")
	tempFile := filename + ".tmp"

	err = os.WriteFile(tempFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	err = os.Rename(tempFile, filename)
	if err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to finalize session file: %w", err)
	}

	return nil
}

// Load reads a session from disk with corruption recovery
func (sm *SessionManager) Load(id string) (*SessionState, error) {
	logging.Debug("loading session", "session_id", id)

	filename := filepath.Join(sm.sessionsDir, id+".json")

	data, err := os.ReadFile(filename)
	if err != nil {
		logging.Error("failed to read session file", "session_id", id, "error", err)
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	state, err := FromJSON(data)
	if err != nil {
		logging.Warn("session deserialization failed, attempting recovery", "session_id", id, "error", err)
		// Attempt corruption recovery
		if recovered := sm.recoverCorruptedSession(id, filename, data); recovered != nil {
			logging.Info("session recovered from corruption", "session_id", id)
			state = recovered
		} else {
			logging.Error("session recovery failed", "session_id", id, "error", err)
			return nil, fmt.Errorf("failed to deserialize session (corruption detected): %w", err)
		}
	}

	// Repair session integrity before validation so fixable fields
	// (e.g. empty TabID, missing ACP defaults) don't cause rejection.
	sm.RepairSession(id, state)

	if err := sm.ValidateSession(id, state); err != nil {
		logging.Error("session validation failed after load", "session_id", id, "error", err)
		return nil, fmt.Errorf("session validation failed: %w", err)
	}

	logging.Info("session loaded", "session_id", id, "type", state.Type, "message_count", len(state.History))
	return state, nil
}

// Delete removes a session from disk
func (sm *SessionManager) Delete(id string) error {
	logging.Info("deleting session", "session_id", id)

	filename := filepath.Join(sm.sessionsDir, id+".json")
	err := os.Remove(filename)
	if err != nil {
		logging.Error("failed to delete session file", "session_id", id, "error", err)
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	logging.Info("session deleted", "session_id", id)
	return nil
}

// List returns all session IDs
func (sm *SessionManager) List() ([]string, error) {
	entries, err := os.ReadDir(sm.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			id := strings.TrimSuffix(entry.Name(), ".json")
			sessions = append(sessions, id)
		}
	}

	return sessions, nil
}

// Cleanup removes sessions older than the specified duration
func (sm *SessionManager) Cleanup(maxAge time.Duration) error {
	sessions, err := sm.List()
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	var errors []string

	for _, id := range sessions {
		filename := filepath.Join(sm.sessionsDir, id+".json")
		info, err := os.Stat(filename)
		if err != nil {
			continue // Skip files we can't stat
		}

		if info.ModTime().Before(cutoff) {
			err = sm.Delete(id)
			if err != nil {
				errors = append(errors, fmt.Sprintf("session %s: %v", id, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}
	return nil
}

// CleanupOnExit performs graceful cleanup on system exit
func (sm *SessionManager) CleanupOnExit() error {
	// Clean up orphaned sessions (older than 24 hours)
	if err := sm.Cleanup(24 * time.Hour); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	// Remove corrupt session files
	sessions, err := sm.List()
	if err != nil {
		return err
	}

	for _, id := range sessions {
		if _, err := sm.Load(id); err != nil {
			if strings.Contains(err.Error(), "corruption") {
				_ = sm.Delete(id) // Best effort cleanup
			}
		}
	}

	return nil
}

// recoverCorruptedSession attempts to recover a corrupted session
func (sm *SessionManager) recoverCorruptedSession(id, filename string, data []byte) *SessionState {
	// Try to extract type from partial JSON - look for the first valid type pattern
	dataStr := string(data)

	// Look for type field patterns
	var sessionType SessionType
	if strings.Contains(dataStr, `"type": "console"`) || strings.Contains(dataStr, `"type":"console"`) {
		sessionType = Console
	} else if strings.Contains(dataStr, `"type": "planning"`) || strings.Contains(dataStr, `"type":"planning"`) {
		sessionType = Planning
	} else {
		// Try JSON unmarshaling for partial extraction as fallback
		var partial struct {
			Type SessionType `json:"type"`
		}
		if json.Unmarshal(data, &partial) == nil && partial.Type != "" {
			sessionType = partial.Type
		}
	}

	if sessionType != "" {
		var recovered *SessionState

		if sessionType == Planning {
			// Try to extract planning data for better recovery
			var planningData struct {
				PlanningData *PlanningSessionData `json:"planning_data"`
			}

			tabID := "recovered-" + id
			title := "Recovered Planning Tab"

			// Attempt to extract planning metadata if possible
			if json.Unmarshal(data, &planningData) == nil && planningData.PlanningData != nil {
				if planningData.PlanningData.TabID != "" {
					tabID = planningData.PlanningData.TabID
				}
				if planningData.PlanningData.Title != "" {
					title = planningData.PlanningData.Title
				}
			}

			recovered = NewPlanningSessionState(tabID, title)
		} else {
			recovered = NewSessionState(sessionType)
		}

		// Try to recover conversation history
		var historyData struct {
			History []Message `json:"history"`
		}
		if json.Unmarshal(data, &historyData) == nil && len(historyData.History) > 0 {
			recovered.History = historyData.History
		}

		// Create backup of corrupted file
		backupName := filename + ".corrupt." + time.Now().Format("20060102-150405")
		_ = os.Rename(filename, backupName)

		// Save recovered session
		if err := sm.writeSession(id, recovered); err != nil {
			return nil
		}

		return recovered
	}

	return nil
}

// ValidateSession performs read-only session integrity checks
func (sm *SessionManager) ValidateSession(id string, state *SessionState) error {
	if state == nil {
		return &ValidationError{SessionID: id, Field: "state", Message: "session state is nil"}
	}

	if state.Type != Console && state.Type != Planning {
		return &ValidationError{SessionID: id, Field: "type", Message: fmt.Sprintf("invalid session type: %s", state.Type)}
	}

	// Validate message integrity
	for i, msg := range state.History {
		if msg.Role != "user" && msg.Role != "assistant" && msg.Role != "system" {
			return &ValidationError{SessionID: id, Field: "history", Message: fmt.Sprintf("message %d has invalid role: %s", i, msg.Role)}
		}
	}

	// Validate planning-specific data
	if state.Type == Planning {
		// Planning data is only required for sessions created with NewPlanningSessionState
		// Sessions created with NewSessionState(Planning) don't have planning data by design
		if state.PlanningData != nil {
			if state.PlanningData.TabID == "" {
				return &ValidationError{SessionID: id, Field: "planning_data.tab_id", Message: "planning session missing tab ID"}
			}

			if state.PlanningData.Title == "" {
				return &ValidationError{SessionID: id, Field: "planning_data.title", Message: "planning session missing title"}
			}

			// Validate planning state enum
			planningState := state.PlanningData.State
			if planningState < PlanningStateIdle || planningState > PlanningStateReadOnly {
				return &ValidationError{SessionID: id, Field: "planning_data.state", Message: fmt.Sprintf("invalid planning state: %d", planningState)}
			}

			// Note: ACP connection fields (Agent, Model), context usage (Total, Used),
			// and timestamp fields (CreatedAt, LastActivity) are not validated here
			// as they can be repaired by RepairSession with appropriate defaults
		}
	}

	return nil
}

// RepairSession fixes recoverable issues in session state (nil history, zero timestamps, excessive history)
func (sm *SessionManager) RepairSession(id string, state *SessionState) {
	if state == nil {
		return
	}

	if state.History == nil {
		log.Printf("session %s: repaired nil history", id)
		state.History = make([]Message, 0)
	}

	// Fix zero timestamps in message history
	for i := range state.History {
		if state.History[i].Timestamp.IsZero() {
			log.Printf("session %s: repaired zero timestamp in message %d", id, i)
			state.History[i].Timestamp = time.Now()
		}
	}

	// Trim excessive history (memory protection)
	if len(state.History) > 10000 {
		log.Printf("session %s: truncated history from %d to 5000 messages", id, len(state.History))
		state.History = state.History[len(state.History)-5000:]
	}

	// Repair planning-specific data
	if state.Type == Planning {
		if state.PlanningData == nil {
			log.Printf("session %s: repaired missing planning data", id)
			// Reconstruct planning data with defaults if missing
			now := time.Now()
			state.PlanningData = &PlanningSessionData{
				TabID:         "recovered-" + id,
				Title:         "Recovered Planning Tab",
				State:         PlanningStateIdle,
				CreatedAt:     now,
				LastActivity:  now,
				ACPConnection: DefaultACPConnectionMetadata(),
				ContextUsage:  DefaultContextUsage(),
			}
		} else {
			// Fix individual fields in existing planning data
			if state.PlanningData.TabID == "" {
				log.Printf("session %s: repaired empty tab ID", id)
				state.PlanningData.TabID = "recovered-" + id
			}

			if state.PlanningData.Title == "" {
				log.Printf("session %s: repaired empty title", id)
				state.PlanningData.Title = "Recovered Planning Tab"
			}

			now := time.Now()
			if state.PlanningData.CreatedAt.IsZero() {
				log.Printf("session %s: repaired zero CreatedAt", id)
				state.PlanningData.CreatedAt = now
			}

			if state.PlanningData.LastActivity.IsZero() {
				log.Printf("session %s: repaired zero LastActivity", id)
				state.PlanningData.LastActivity = now
			}

			// Fix ACP connection defaults from canonical source
			defaults := DefaultACPConnectionMetadata()
			if state.PlanningData.ACPConnection.Agent == "" {
				log.Printf("session %s: repaired empty ACP agent", id)
				state.PlanningData.ACPConnection.Agent = defaults.Agent
			}

			if state.PlanningData.ACPConnection.Model == "" {
				log.Printf("session %s: repaired empty ACP model", id)
				state.PlanningData.ACPConnection.Model = defaults.Model
			}

			if state.PlanningData.ACPConnection.Timeout == 0 {
				log.Printf("session %s: repaired zero ACP timeout", id)
				state.PlanningData.ACPConnection.Timeout = defaults.Timeout
			}

			if state.PlanningData.ACPConnection.ResponseFormat == "" {
				log.Printf("session %s: repaired empty ACP response format", id)
				state.PlanningData.ACPConnection.ResponseFormat = defaults.ResponseFormat
			}

			if !state.PlanningData.ACPConnection.Streaming {
				log.Printf("session %s: repaired ACP streaming flag", id)
				state.PlanningData.ACPConnection.Streaming = defaults.Streaming
			}

			if state.PlanningData.ACPConnection.LastActivity.IsZero() {
				log.Printf("session %s: repaired zero ACP last activity", id)
				state.PlanningData.ACPConnection.LastActivity = now
			}

			// Fix context usage defaults from canonical source
			ctxDefaults := DefaultContextUsage()
			if state.PlanningData.ContextUsage.Total <= 0 {
				log.Printf("session %s: repaired invalid context total", id)
				state.PlanningData.ContextUsage.Total = ctxDefaults.Total
			}

			if state.PlanningData.ContextUsage.Used < 0 {
				log.Printf("session %s: repaired negative context used", id)
				state.PlanningData.ContextUsage.Used = 0
			}
		}
	}
}

// generateSessionID creates a random session identifier
func generateSessionID() (string, error) {
	bytes := make([]byte, 8)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// LogModeSwitch is a no-op hook for future diagnostics during mode transitions
func (sm *SessionManager) LogModeSwitch(sessionID string, fromMode, toMode SessionType, preservedData interface{}) {
	// Reserved for future diagnostic use
}

// ValidateSessionFlow ensures data integrity during rapid mode switching
func (sm *SessionManager) ValidateSessionFlow(sessionID string) error {
	state, err := sm.Load(sessionID)
	if err != nil {
		return fmt.Errorf("session flow validation failed: %w", err)
	}

	if err := sm.ValidateSession(sessionID, state); err != nil {
		return fmt.Errorf("session flow validation failed: %w", err)
	}

	return nil
}

// Planning Tab Session Management

// CreatePlanningSession creates a new planning session with ACP metadata
func (sm *SessionManager) CreatePlanningSession(tabID, title string) (string, *SessionState, error) {
	logging.Info("creating planning session", "tab_id", tabID, "title", title)

	// Generate session ID
	sessionID, err := generateSessionID()
	if err != nil {
		logging.Error("failed to generate planning session ID", "tab_id", tabID, "error", err)
		return "", nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Create planning session state
	state := NewPlanningSessionState(tabID, title)

	// Save to disk
	if err := sm.Save(sessionID, state); err != nil {
		logging.Error("failed to save planning session", "session_id", sessionID, "tab_id", tabID, "error", err)
		return "", nil, fmt.Errorf("failed to save planning session: %w", err)
	}

	logging.Info("planning session created", "session_id", sessionID, "tab_id", tabID)
	return sessionID, state, nil
}

// LoadPlanningSession loads a planning session by session ID
func (sm *SessionManager) LoadPlanningSession(sessionID string) (*SessionState, error) {
	logging.Debug("loading planning session", "session_id", sessionID)

	state, err := sm.Load(sessionID)
	if err != nil {
		return nil, err
	}

	if !state.IsPlanning() {
		logging.Error("session is not a planning session", "session_id", sessionID, "type", state.Type)
		return nil, fmt.Errorf("session %s is not a planning session", sessionID)
	}

	logging.Info("planning session loaded", "session_id", sessionID, "tab_id", state.GetTabID())
	return state, nil
}

// FindPlanningSessionByTabID finds a planning session by tab ID
func (sm *SessionManager) FindPlanningSessionByTabID(tabID string) (string, *SessionState, error) {
	logging.Debug("finding planning session by tab ID", "tab_id", tabID)

	sessions, err := sm.List()
	if err != nil {
		logging.Error("failed to list sessions for tab search", "tab_id", tabID, "error", err)
		return "", nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	for _, sessionID := range sessions {
		state, err := sm.Load(sessionID)
		if err != nil {
			continue // Skip corrupted sessions
		}

		if state.IsPlanning() && state.GetTabID() == tabID {
			logging.Info("found planning session for tab", "session_id", sessionID, "tab_id", tabID)
			return sessionID, state, nil
		}
	}

	logging.Debug("no planning session found for tab", "tab_id", tabID)
	return "", nil, fmt.Errorf("no planning session found for tab ID: %s", tabID)
}

// ListPlanningSessions returns all planning session IDs and their states
func (sm *SessionManager) ListPlanningSessions() (map[string]*SessionState, error) {
	sessions, err := sm.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	planningSessions := make(map[string]*SessionState)
	for _, sessionID := range sessions {
		state, err := sm.Load(sessionID)
		if err != nil {
			continue // Skip corrupted sessions
		}

		if state.IsPlanning() {
			planningSessions[sessionID] = state
		}
	}

	return planningSessions, nil
}

// UpdatePlanningSessionState updates the state of a planning session
func (sm *SessionManager) UpdatePlanningSessionState(sessionID string, state PlanningTabState) error {
	sessionState, err := sm.LoadPlanningSession(sessionID)
	if err != nil {
		return err
	}

	sessionState.UpdatePlanningState(state)
	return sm.SaveQuiet(sessionID, sessionState)
}

// UpdatePlanningSessionACP updates ACP connection metadata for a planning session
func (sm *SessionManager) UpdatePlanningSessionACP(sessionID string, connected bool, agent, model string) error {
	sessionState, err := sm.LoadPlanningSession(sessionID)
	if err != nil {
		return err
	}

	sessionState.UpdateACPConnection(connected, agent, model)
	return sm.SaveQuiet(sessionID, sessionState)
}

// UpdatePlanningSessionContext updates context usage for a planning session
func (sm *SessionManager) UpdatePlanningSessionContext(sessionID string, used, total int) error {
	sessionState, err := sm.LoadPlanningSession(sessionID)
	if err != nil {
		return err
	}

	sessionState.UpdateContextUsage(used, total)
	return sm.SaveQuiet(sessionID, sessionState)
}

// CleanupCompletedPlanningSessions removes completed/failed planning sessions older than maxAge
func (sm *SessionManager) CleanupCompletedPlanningSessions(maxAge time.Duration) (int, error) {
	sessions, err := sm.ListPlanningSessions()
	if err != nil {
		return 0, fmt.Errorf("failed to list planning sessions: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	cleaned := 0

	for sessionID, state := range sessions {
		planningState := state.GetPlanningState()

		// Only cleanup completed or failed sessions
		if planningState == PlanningStateCompleted || planningState == PlanningStateFailed {
			if state.PlanningData.LastActivity.Before(cutoff) {
				if err := sm.Delete(sessionID); err == nil {
					cleaned++
				}
			}
		}
	}

	return cleaned, nil
}

// CleanupOrphanedPlanningSessions removes planning sessions for tabs that no longer exist
func (sm *SessionManager) CleanupOrphanedPlanningSessions(activeTabIDs []string) (int, error) {
	sessions, err := sm.ListPlanningSessions()
	if err != nil {
		return 0, fmt.Errorf("failed to list planning sessions: %w", err)
	}

	// Create lookup map of active tab IDs
	activeMap := make(map[string]bool)
	for _, tabID := range activeTabIDs {
		activeMap[tabID] = true
	}

	cleaned := 0
	for sessionID, state := range sessions {
		tabID := state.GetTabID()
		if !activeMap[tabID] {
			// This session belongs to a tab that no longer exists
			if err := sm.Delete(sessionID); err == nil {
				cleaned++
			}
		}
	}

	return cleaned, nil
}
