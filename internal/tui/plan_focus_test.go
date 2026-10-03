package tui

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/watcher"
)

// setupPlanFocusTestModel creates a test model for plan focus testing
func setupPlanFocusTestModel(t *testing.T) model {
	t.Helper()

	// Create test config with theme
	cfg := &config.Config{
		Theme: "default",
		LoadedTheme: &config.Theme{
			Name: "default",
		},
		MaxActivityLines: 100,
		Logging: config.LoggingConfig{
			DefaultLevel:   "info",
			MaxBufferLines: 1000,
		},
	}

	// Create test manager and watcher
	manager := agent.NewManager(cfg)
	w := &watcher.Watcher{}

	// Create temp files for logging
	logFile, err := os.CreateTemp("", "test-log")
	if err != nil {
		t.Fatalf("Failed to create temp log file: %v", err)
	}
	t.Cleanup(func() { logFile.Close(); os.Remove(logFile.Name()) })

	logReader, err := os.Open(logFile.Name())
	if err != nil {
		t.Fatalf("Failed to open log reader: %v", err)
	}
	t.Cleanup(func() { logReader.Close() })

	m := newModel(w, manager, cfg, logFile, logReader)

	// Initialize the model
	m.Init()

	return m
}

// TestPlanWithDescriptionFocusesMessageInput tests T1: plan [description] focuses message input
func TestPlanWithDescriptionFocusesMessageInput(t *testing.T) {
	m := setupPlanFocusTestModel(t)

	// Execute plan command with description through full Update routing
	description := "implement user authentication"
	m, _ = m.executeCommand("plan " + description)

	// Verify a planning tab was created
	tabs := m.tabManager.GetTabs()
	if len(tabs) != 2 {
		t.Fatalf("Expected 2 tabs (main + planning), got %d", len(tabs))
	}

	activeTab := m.tabManager.GetActiveTab()
	if activeTab == nil {
		t.Fatal("Active tab is nil")
	}

	if activeTab.Type() != TabTypePlanning {
		t.Errorf("Expected active tab to be Planning, got %v", activeTab.Type())
	}

	planningTab, ok := activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab")
	}

	// Verify focus state in tabFocusStates map
	focusState, exists := m.tabFocusStates[planningTab.ID()]
	if !exists {
		t.Error("Focus state not found in tabFocusStates map")
	}

	if focusState != FocusTargetMessage {
		t.Errorf("Expected focus state to be FocusTargetMessage, got %v", focusState)
	}

	// Verify actual component focus state
	if planningTab.focusTarget != FocusTargetMessage {
		t.Errorf("Expected planning tab focusTarget to be FocusTargetMessage, got %v", planningTab.focusTarget)
	}

	if !planningTab.textinput.Focused() {
		t.Error("Expected planning tab textinput to be focused, but it is not")
	}

	// Verify footer input is NOT focused
	if m.input.Focused() {
		t.Error("Expected footer input to NOT be focused when message input should have focus")
	}
}

// TestPlanEmptyFocusesMessageInput tests T2: empty plan command focuses message input
func TestPlanEmptyFocusesMessageInput(t *testing.T) {
	m := setupPlanFocusTestModel(t)

	// Execute plan command without description
	m, _ = m.executeCommand("plan")

	// Verify a planning tab was created
	tabs := m.tabManager.GetTabs()
	if len(tabs) != 2 {
		t.Fatalf("Expected 2 tabs (main + planning), got %d", len(tabs))
	}

	activeTab := m.tabManager.GetActiveTab()
	if activeTab == nil {
		t.Fatal("Active tab is nil")
	}

	if activeTab.Type() != TabTypePlanning {
		t.Errorf("Expected active tab to be Planning, got %v", activeTab.Type())
	}

	planningTab, ok := activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab")
	}

	// Verify focus state in tabFocusStates map
	focusState, exists := m.tabFocusStates[planningTab.ID()]
	if !exists {
		t.Error("Focus state not found in tabFocusStates map")
	}

	if focusState != FocusTargetMessage {
		t.Errorf("Expected focus state to be FocusTargetMessage, got %v", focusState)
	}

	// Verify actual component focus state
	if planningTab.focusTarget != FocusTargetMessage {
		t.Errorf("Expected planning tab focusTarget to be FocusTargetMessage, got %v", planningTab.focusTarget)
	}

	if !planningTab.textinput.Focused() {
		t.Error("Expected planning tab textinput to be focused, but it is not")
	}

	// Verify footer input is NOT focused
	if m.input.Focused() {
		t.Error("Expected footer input to NOT be focused when message input should have focus")
	}
}

// TestPlanDescriptionCursorAtEnd tests T3: cursor positioned at end of description
func TestPlanDescriptionCursorAtEnd(t *testing.T) {
	m := setupPlanFocusTestModel(t)

	// Execute plan command with description
	description := "add payment processing"
	m, _ = m.executeCommand("plan " + description)

	activeTab := m.tabManager.GetActiveTab()
	planningTab, ok := activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab")
	}

	// Verify the description was added as a message to the history
	// (not to the textinput field, which remains empty for user to type)
	if len(planningTab.messages) < 1 {
		t.Fatal("Expected at least one message in planning tab history")
	}

	// Find the user message with the description
	foundDescription := false
	for _, msg := range planningTab.messages {
		if msg.Role == "user" && msg.Content == description {
			foundDescription = true
			break
		}
	}

	if !foundDescription {
		t.Errorf("Expected to find user message with description %q in message history", description)
	}

	// Verify textinput is empty and ready for user input
	inputValue := planningTab.textinput.Value()
	if inputValue != "" {
		t.Errorf("Expected textinput to be empty for user input, got %q", inputValue)
	}

	// Verify cursor is at position 0 (empty input, ready to type)
	cursorPosition := planningTab.textinput.Position()
	if cursorPosition != 0 {
		t.Errorf("Expected cursor at position 0 (empty input), got %d", cursorPosition)
	}
}

// TestTabToggleAfterPlanFocus tests T4: Tab toggle works after initial focus
// Note: The actual implementation uses ESC to transfer from message to footer,
// and mouse click or explicit focus commands to return to message input.
func TestTabToggleAfterPlanFocus(t *testing.T) {
	m := setupPlanFocusTestModel(t)

	// Execute plan command with description
	m, _ = m.executeCommand("plan implement search")

	activeTab := m.tabManager.GetActiveTab()
	planningTab, ok := activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab")
	}

	// Verify initial focus on message input
	if planningTab.focusTarget != FocusTargetMessage {
		t.Errorf("Expected initial focus on message input, got %v", planningTab.focusTarget)
	}

	if !planningTab.textinput.Focused() {
		t.Error("Expected textinput to be focused initially")
	}

	// Simulate ESC key press to transfer focus to footer (full routing through model.Update)
	escMsg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	updatedModel, cmd := m.Update(escMsg)
	m = updatedModel.(model)

	// Process the focusTransferMsg command
	if cmd != nil {
		msg := cmd()
		updatedModel, _ = m.Update(msg)
		m = updatedModel.(model)
	}

	// Verify focus transferred to footer
	focusState := m.tabFocusStates[planningTab.ID()]
	if focusState != FocusTargetFooter {
		t.Errorf("Expected focus state to transfer to FocusTargetFooter after ESC, got %v", focusState)
	}

	if planningTab.focusTarget != FocusTargetFooter {
		t.Errorf("Expected planning tab focusTarget to be FocusTargetFooter after ESC, got %v", planningTab.focusTarget)
	}

	if planningTab.textinput.Focused() {
		t.Error("Expected textinput to be blurred after ESC")
	}

	if !m.input.Focused() {
		t.Error("Expected footer input to be focused after ESC")
	}

	// Verify that focus can be restored by simulating ESC again when footer has focus
	// (ESC on footer with planning tab should transfer to message)
	updatedModel, cmd = m.Update(escMsg)
	m = updatedModel.(model)

	// Process the focusTransferMsg command
	if cmd != nil {
		msg := cmd()
		updatedModel, _ = m.Update(msg)
		m = updatedModel.(model)
	}

	// Get fresh reference to planning tab
	activeTab = m.tabManager.GetActiveTab()
	planningTab, ok = activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab after second ESC")
	}

	// Verify focus transferred back to message input
	focusState = m.tabFocusStates[planningTab.ID()]
	if focusState != FocusTargetMessage {
		t.Errorf("Expected focus state to transfer back to FocusTargetMessage after second ESC, got %v", focusState)
	}

	if planningTab.focusTarget != FocusTargetMessage {
		t.Errorf("Expected planning tab focusTarget to be FocusTargetMessage after second ESC, got %v", planningTab.focusTarget)
	}

	if !planningTab.textinput.Focused() {
		t.Error("Expected textinput to be focused after second ESC")
	}

	if m.input.Focused() {
		t.Error("Expected footer input to NOT be focused after second ESC")
	}
}

// TestPlanningTabSwitchPreservesDefaultFocus tests T5: other planning tab entry paths unchanged
func TestPlanningTabSwitchPreservesDefaultFocus(t *testing.T) {
	m := setupPlanFocusTestModel(t)

	// Create a planning tab through normal command (empty)
	m, _ = m.executeCommand("plan")

	activeTab := m.tabManager.GetActiveTab()
	planningTab, ok := activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab")
	}

	planningTabID := planningTab.ID()

	// Initial state should be message input focused (from plan command)
	if m.tabFocusStates[planningTabID] != FocusTargetMessage {
		t.Errorf("Expected initial focus state to be FocusTargetMessage, got %v", m.tabFocusStates[planningTabID])
	}

	// Transfer focus to footer using ESC (when message input has focus, ESC transfers to footer)
	escMsg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	updatedModel, cmd := m.Update(escMsg)
	m = updatedModel.(model)

	// Process the focusTransferMsg command returned by ESC
	if cmd != nil {
		msg := cmd()
		updatedModel, _ = m.Update(msg)
		m = updatedModel.(model)
	}

	// Get fresh reference to planning tab after update
	activeTab = m.tabManager.GetActiveTab()
	planningTab, ok = activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab after ESC")
	}

	// Verify footer focus was set
	if m.tabFocusStates[planningTabID] != FocusTargetFooter {
		t.Errorf("Expected footer focus after ESC, got %v", m.tabFocusStates[planningTabID])
	}

	if planningTab.focusTarget != FocusTargetFooter {
		t.Errorf("Expected planningTab.focusTarget to be FocusTargetFooter after ESC, got %v", planningTab.focusTarget)
	}

	// Switch away to main tab
	m, _ = m.switchActiveTab(0)

	// Switch back to planning tab by finding it
	tabIndex := -1
	tabs := m.tabManager.GetTabs()
	for i, tab := range tabs {
		if tab.ID() == planningTabID {
			tabIndex = i
			break
		}
	}
	if tabIndex < 0 {
		t.Fatal("Could not find planning tab")
	}
	m, _ = m.switchActiveTab(tabIndex)

	// Get fresh reference to planning tab after switching back
	activeTab = m.tabManager.GetActiveTab()
	planningTab, ok = activeTab.(*PlanningTab)
	if !ok {
		t.Fatal("Active tab is not a PlanningTab after switch back")
	}

	// Verify focus state was preserved (should be footer)
	focusState := m.tabFocusStates[planningTabID]
	if focusState != FocusTargetFooter {
		t.Errorf("Expected preserved focus state to be FocusTargetFooter, got %v", focusState)
	}

	if planningTab.focusTarget != FocusTargetFooter {
		t.Errorf("Expected planning tab to restore FocusTargetFooter, got %v", planningTab.focusTarget)
	}

	if !m.input.Focused() {
		t.Error("Expected footer input to be focused after restoration")
	}
}

// TestPlanClassicUnchanged tests T6: plan classic behavior unchanged
func TestPlanClassicUnchanged(t *testing.T) {
	m := setupPlanFocusTestModel(t)

	// Execute plan classic command
	m, _ = m.executeCommand("plan classic test description")

	// Verify mode switched to planning (classic mode)
	if m.currentMode != "planning" {
		t.Errorf("Expected current mode to be 'planning', got %v", m.currentMode)
	}

	// Verify console state was preserved (classic planning preserves state)
	if m.consoleState == nil {
		t.Error("Expected console state to be preserved for classic planning")
	}

	// Verify footer input was blurred (classic planning behavior)
	if m.input.Focused() {
		t.Error("Expected footer input to be blurred in classic planning mode")
	}

	// Verify no new planning tab was created (classic uses subprocess)
	tabs := m.tabManager.GetTabs()
	planningTabCount := 0
	for _, tab := range tabs {
		if tab.Type() == TabTypePlanning {
			planningTabCount++
		}
	}

	if planningTabCount != 0 {
		t.Errorf("Expected no planning tabs for classic mode, got %d", planningTabCount)
	}
}
