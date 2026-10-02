package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

// TestEnterKeyCommandExecutionInAllTabs tests that command execution works
// when footer is focused, regardless of which tab is active
func TestEnterKeyCommandExecutionInAllTabs(t *testing.T) {
	tests := []struct {
		name          string
		tabType       TabType
		footerFocused bool
		inputValue    string
		expectCommand bool
	}{
		{"main tab footer focused", TabTypeMain, true, "help", true},
		{"planning tab footer focused", TabTypePlanning, true, "status", true},
		{"agent tab footer focused", TabTypeAgent, true, "theme", true},
		{"log tab footer focused", TabTypeLog, true, "about", true},
		{"planning tab content focused", TabTypePlanning, false, "message", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test model with minimal configuration
			m := createTestModelWithTab(t, tt.tabType)

			// Set footer focus state
			if tt.footerFocused {
				m.input.SetFocus(true)
			} else {
				m.input.SetFocus(false)
			}

			// Set input value
			m.input.SetValue(tt.inputValue)

			// Create enter key message
			enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})

			// Process the enter key
			updatedModel, _ := m.Update(enterMsg)
			updated := updatedModel.(model)

			if tt.expectCommand {
				// When footer is focused, input should be cleared after command execution
				if updated.input.Value() != "" {
					t.Errorf("Expected input to be cleared after command execution, got %q", updated.input.Value())
				}

				// The command should have been processed
				// We can't directly verify command execution without exposing internals,
				// but we verify that the input was cleared which indicates the command path was taken
			} else {
				// When footer is NOT focused, input should remain unchanged
				// (enter was forwarded to tab for tab-specific handling)
				if updated.input.Value() != tt.inputValue {
					t.Errorf("Expected input value to remain %q when footer not focused, got %q",
						tt.inputValue, updated.input.Value())
				}
			}
		})
	}
}

// TestEnterKeyFooterFocusPriority verifies that footer focus state
// takes priority over tab type when determining command execution
func TestEnterKeyFooterFocusPriority(t *testing.T) {
	tests := []struct {
		name      string
		tabType   TabType
		focused   bool
		wantClear bool // Should input be cleared (command executed)?
	}{
		{"main tab focused", TabTypeMain, true, true},
		{"agent tab focused", TabTypeAgent, true, true},
		{"planning tab focused", TabTypePlanning, true, true},
		{"log tab focused", TabTypeLog, true, true},
		{"planning tab unfocused", TabTypePlanning, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createTestModelWithTab(t, tt.tabType)
			m.input.SetFocus(tt.focused)
			m.input.SetValue("test command")

			enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
			updatedModel, _ := m.Update(enterMsg)
			updated := updatedModel.(model)

			inputCleared := updated.input.Value() == ""
			if inputCleared != tt.wantClear {
				if tt.wantClear {
					t.Error("Expected input to be cleared (command executed)")
				} else {
					t.Error("Expected input to remain (enter forwarded to tab)")
				}
			}
		})
	}
}

// TestEnterKeyEmptyInput verifies that pressing enter with empty input
// does not execute a command but still clears autocomplete state
func TestEnterKeyEmptyInput(t *testing.T) {
	m := createTestModelWithTab(t, TabTypeMain)
	m.input.SetFocus(true)
	m.input.SetValue("") // Empty input

	enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
	updatedModel, _ := m.Update(enterMsg)
	updated := updatedModel.(model)

	// Input should remain empty
	if updated.input.Value() != "" {
		t.Errorf("Expected empty input to remain empty, got %q", updated.input.Value())
	}
}

// TestEnterKeyWhitespaceOnly verifies that whitespace-only input
// is treated as empty and does not execute a command
func TestEnterKeyWhitespaceOnly(t *testing.T) {
	whitespaceInputs := []string{
		"   ",
		"\t",
		"\n",
		"  \t  ",
	}

	for _, input := range whitespaceInputs {
		t.Run("whitespace: "+strings.ReplaceAll(input, " ", "·"), func(t *testing.T) {
			m := createTestModelWithTab(t, TabTypeMain)
			m.input.SetFocus(true)
			m.input.SetValue(input)

			enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
			updatedModel, _ := m.Update(enterMsg)
			updated := updatedModel.(model)

			// Input should be cleared (trimmed whitespace is empty)
			if updated.input.Value() != "" {
				t.Errorf("Expected whitespace-only input to be cleared, got %q", updated.input.Value())
			}
		})
	}
}

// TestEnterKeyAgentTabCommandExecution specifically tests the bug fix:
// commands should execute in agent tabs when footer is focused
func TestEnterKeyAgentTabCommandExecution(t *testing.T) {
	m := createTestModelWithTab(t, TabTypeAgent)
	m.input.SetFocus(true)
	m.input.SetValue("help")

	enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
	updatedModel, _ := m.Update(enterMsg)
	updated := updatedModel.(model)

	// Input should be cleared, indicating command was executed
	if updated.input.Value() != "" {
		t.Error("Agent tab should execute commands when footer is focused (bug #258)")
	}
}

// TestEnterKeyLogTabCommandExecution specifically tests the bug fix:
// commands should execute in log tabs when footer is focused
func TestEnterKeyLogTabCommandExecution(t *testing.T) {
	m := createTestModelWithTab(t, TabTypeLog)
	m.input.SetFocus(true)
	m.input.SetValue("status")

	enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
	updatedModel, _ := m.Update(enterMsg)
	updated := updatedModel.(model)

	// Input should be cleared, indicating command was executed
	if updated.input.Value() != "" {
		t.Error("Log tab should execute commands when footer is focused (bug #258)")
	}
}

// TestEnterKeyPlanningTabForwarding verifies that enter key is forwarded
// to planning tab when footer is NOT focused (for sending messages).
// Note: This test only verifies the negative case (command not executed).
// Full forwarding behavior is verified through integration tests.
func TestEnterKeyPlanningTabForwarding(t *testing.T) {
	m := createTestModelWithTab(t, TabTypePlanning)
	m.input.SetFocus(false) // Footer NOT focused
	m.input.SetValue("Hello, planning agent")

	enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
	updatedModel, _ := m.Update(enterMsg)
	updated := updatedModel.(model)

	// Input should NOT be cleared because enter was forwarded to planning tab
	// for its internal message handling (not command execution)
	if updated.input.Value() == "" {
		t.Error("Planning tab should receive enter key for message sending when footer not focused")
	}
}

// TestEnterKeyCommandExecutionWithSequentialInput verifies that command execution
// works correctly when characters are typed sequentially before pressing enter
func TestEnterKeyCommandExecutionWithSequentialInput(t *testing.T) {
	m := createTestModelWithTab(t, TabTypeMain)
	m.input.SetFocus(true)

	// Type partial command to trigger autocomplete
	m.input.SetValue("hel")

	// Update to trigger autocomplete suggestions
	updatedModel, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "p", Code: 'p'}))
	m = updatedModel.(model)

	// Now the input should be "help" and autocomplete may be showing suggestions
	enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
	updatedModel, _ = m.Update(enterMsg)
	updated := updatedModel.(model)

	// Command should execute, clearing the input
	if updated.input.Value() != "" {
		t.Error("Command should execute after sequential input")
	}
}

// TestEnterKeyAllTabTypes verifies the fix works across all four tab types
func TestEnterKeyAllTabTypes(t *testing.T) {
	allTabTypes := []TabType{
		TabTypeMain,
		TabTypeAgent,
		TabTypePlanning,
		TabTypeLog,
	}

	for _, tabType := range allTabTypes {
		t.Run(tabType.String(), func(t *testing.T) {
			m := createTestModelWithTab(t, tabType)
			m.input.SetFocus(true)
			m.input.SetValue("test")

			enterMsg := tea.KeyPressMsg(tea.Key{Code: 13})
			updatedModel, _ := m.Update(enterMsg)
			updated := updatedModel.(model)

			// All tab types should execute commands when footer is focused
			if updated.input.Value() != "" {
				t.Errorf("%s should execute commands when footer is focused", tabType)
			}
		})
	}
}

// Helper function to create a test model with a specific active tab type
func createTestModelWithTab(t *testing.T, tabType TabType) model {
	t.Helper()

	// Create minimal test configuration
	cfg := &config.Config{
		LoadedTheme: createMinimalTheme(),
	}

	// Create model using newModel - we need to provide nil for watcher and log files for testing
	m := newModel(nil, agent.NewManager(cfg), cfg, nil, nil)

	// Create and add the appropriate tab type
	switch tabType {
	case TabTypeMain:
		// Main tab already exists by default
	case TabTypeAgent:
		// Create and add an agent tab
		agentTab := NewAgentTab("test-agent-123", m.manager, m.styles)
		m.tabManager.AddTab(agentTab)
		// Switch to the agent tab
		tabs := m.tabManager.GetTabs()
		for i, tab := range tabs {
			if tab.Type() == TabTypeAgent {
				m.tabManager.SetActiveTab(i)
				break
			}
		}
	case TabTypePlanning:
		// Create and add a planning tab (without ACP connection for testing)
		contextTracker := NewContextTracker()
		planningTab := NewPlanningTabWithSession(
			"test-plan-1",
			"Test Plan",
			m.styles,
			contextTracker,
			nil, // sessionManager not needed for this test
			nil, // acpConn not needed for this test
		)
		m.tabManager.AddTab(planningTab)
		// Switch to the planning tab
		tabs := m.tabManager.GetTabs()
		for i, tab := range tabs {
			if tab.Type() == TabTypePlanning {
				m.tabManager.SetActiveTab(i)
				break
			}
		}
	case TabTypeLog:
		// Create and add a log tab
		logTab := NewLogTab("test-log", "info", 1000, m.styles)
		m.tabManager.AddTab(logTab)
		// Switch to the log tab
		tabs := m.tabManager.GetTabs()
		for i, tab := range tabs {
			if tab.Type() == TabTypeLog {
				m.tabManager.SetActiveTab(i)
				break
			}
		}
	}

	return m
}

// Helper function to create a minimal theme for testing
func createMinimalTheme() *config.Theme {
	theme := &config.Theme{}
	theme.Colors.Primary = "#00AAFF"
	theme.Colors.Secondary = "#0088CC"
	theme.Colors.Success = "#00AA00"
	theme.Colors.Warning = "#FFAA00"
	theme.Colors.Error = "#FF0000"
	theme.Colors.TextPrimary = "#FFFFFF"
	theme.Colors.TextSecondary = "#CCCCCC"
	theme.Colors.TextMuted = "#888888"
	theme.Colors.Prompt = "#00AAFF"
	theme.Colors.Separator = "#00AAFF"
	theme.Colors.Activity = "#FFFFFF"
	theme.Colors.Background = "#000000"
	theme.Colors.Surface = "#111111"
	return theme
}
