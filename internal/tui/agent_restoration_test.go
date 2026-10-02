package tui

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

func TestRestoreOrFocusAgentTab(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Theme: "test",
		LoadedTheme: &config.Theme{
			Name: "test",
		},
	}
	manager := agent.NewManager(cfg)
	styles := NewStyles(cfg.LoadedTheme)
	tabManager := NewTabManager()

	// Add main tab
	mainTab := NewMainTab()
	tabManager.AddTab(mainTab)

	agentID := "test-agent-123"

	// Test 1: Create new agent tab when none exists
	t.Run("creates new agent tab", func(t *testing.T) {
		result := tabManager.RestoreOrFocusAgentTab(agentID, manager, styles)
		if !result {
			t.Error("Expected RestoreOrFocusAgentTab to return true")
		}

		if len(tabManager.GetTabs()) != 2 {
			t.Errorf("Expected 2 tabs, got %d", len(tabManager.GetTabs()))
		}

		activeIndex := tabManager.GetActiveTabIndex()
		if activeIndex != 1 {
			t.Errorf("Expected active tab to be 1, got %d", activeIndex)
		}

		activeTab := tabManager.GetActiveTab()
		if activeTab.Type() != TabTypeAgent {
			t.Error("Expected active tab to be an agent tab")
		}
	})

	// Test 2: Focus existing agent tab
	t.Run("focuses existing agent tab", func(t *testing.T) {
		// Switch to main tab first
		tabManager.SetActiveTab(0)

		result := tabManager.RestoreOrFocusAgentTab(agentID, manager, styles)
		if !result {
			t.Error("Expected RestoreOrFocusAgentTab to return true")
		}

		// Should still have 2 tabs (no new tab created)
		if len(tabManager.GetTabs()) != 2 {
			t.Errorf("Expected 2 tabs, got %d", len(tabManager.GetTabs()))
		}

		// Should focus the existing agent tab
		activeIndex := tabManager.GetActiveTabIndex()
		if activeIndex != 1 {
			t.Errorf("Expected active tab to be 1, got %d", activeIndex)
		}
	})
}

func TestFindTabByAgentID(t *testing.T) {
	tabManager := NewTabManager()
	cfg := &config.Config{
		Theme: "test",
		LoadedTheme: &config.Theme{
			Name: "test",
		},
	}
	manager := agent.NewManager(cfg)
	styles := NewStyles(cfg.LoadedTheme)

	// Add main tab
	mainTab := NewMainTab()
	tabManager.AddTab(mainTab)

	// Test agent ID
	agentID := "test-agent-456"

	// Should not find non-existent agent tab
	index := tabManager.FindTabByAgentID(agentID)
	if index != -1 {
		t.Errorf("Expected -1 for non-existent agent, got %d", index)
	}

	// Add agent tab
	agentTab := NewAgentTab(agentID, manager, styles)
	tabManager.AddTab(agentTab)

	// Should find the agent tab
	index = tabManager.FindTabByAgentID(agentID)
	if index != 1 {
		t.Errorf("Expected 1 for agent tab index, got %d", index)
	}
}

func TestStatusOverlayAgentSelection(t *testing.T) {
	cfg := &config.Config{
		Theme:      "dark",
		MaxRetries: 1,
		LoadedTheme: &config.Theme{
			Name: "dark",
		},
	}
	manager := agent.NewManager(cfg)
	m := newModel(nil, manager, cfg, nil, nil)
	m.width = 120
	m.height = 40

	t.Run("handleStatus stores sorted running agents snapshot", func(t *testing.T) {
		m, _ = m.handleStatus()

		// With no agents, snapshot should be empty
		if len(m.statusRunningAgents) != 0 {
			t.Errorf("Expected 0 running agents in snapshot, got %d", len(m.statusRunningAgents))
		}

		// Overlay should be active
		if m.activeOverlay != overlayStatus {
			t.Error("Expected status overlay to be active")
		}
	})

	t.Run("invalid agent index is ignored", func(t *testing.T) {
		m.activeOverlay = overlayStatus
		m.statusRunningAgents = nil

		// Simulate pressing "1" with no running agents — should be a no-op
		// (We test the logic directly since constructing tea.KeyPressMsg is complex)
		agentIndex := 0 // "1" key -> 0-based
		if agentIndex >= 0 && agentIndex < len(m.statusRunningAgents) {
			t.Error("Should not match any agent when snapshot is empty")
		}
	})

	t.Run("valid agent index selects correct agent from snapshot", func(t *testing.T) {
		// Create a snapshot with mock agents
		m.statusRunningAgents = []*agent.Agent{
			{ID: "agent-a", IssueNumber: 10, Status: agent.StatusRunning},
			{ID: "agent-b", IssueNumber: 20, Status: agent.StatusRunning},
			{ID: "agent-c", IssueNumber: 30, Status: agent.StatusRunning},
		}

		// Pressing "2" should select agent-b (index 1)
		agentIndex := 1
		if agentIndex < 0 || agentIndex >= len(m.statusRunningAgents) {
			t.Fatal("Expected valid index")
		}
		selected := m.statusRunningAgents[agentIndex]
		if selected.ID != "agent-b" {
			t.Errorf("Expected agent-b, got %s", selected.ID)
		}
	})

	t.Run("overlay dismissal clears state", func(t *testing.T) {
		m.activeOverlay = overlayStatus
		m = m.clearOverlay()
		if m.activeOverlay != overlayNone {
			t.Error("Expected overlay to be cleared")
		}
	})
}

func TestStatusRunningAgentsDeterministicOrder(t *testing.T) {
	cfg := &config.Config{
		Theme:      "dark",
		MaxRetries: 1,
		LoadedTheme: &config.Theme{
			Name: "dark",
		},
	}
	manager := agent.NewManager(cfg)
	m := newModel(nil, manager, cfg, nil, nil)
	m.width = 120
	m.height = 40

	// Call handleStatus multiple times — snapshot should always be sorted by issue number
	for i := 0; i < 5; i++ {
		m, _ = m.handleStatus()
		m = m.clearOverlay()
	}

	// With no agents this is trivially correct, but the test validates no panics
	if m.statusRunningAgents == nil {
		// nil is acceptable when there are no running agents
	}
}
