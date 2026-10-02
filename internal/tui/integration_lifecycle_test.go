package tui

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

func TestAgentTabIntegration(t *testing.T) {
	// Test TabManager integration with agent tabs
	tm := NewTabManager()

	// Add main tab
	mainTab := NewMainTab()
	tm.AddTab(mainTab)

	if len(tm.GetTabs()) != 1 {
		t.Errorf("Expected 1 tab, got %d", len(tm.GetTabs()))
	}

	// Simulate adding an agent tab (this would happen in updateAgentTabs)
	cfg := &config.Config{}
	m := agent.NewManager(cfg)
	styles := &Styles{} // Use empty styles for test

	agentTab := NewAgentTab("test-123", m, styles)
	tm.AddTab(agentTab)

	if len(tm.GetTabs()) != 2 {
		t.Errorf("Expected 2 tabs after adding agent tab, got %d", len(tm.GetTabs()))
	}

	// Test tab switching
	activeTab := tm.GetActiveTab()
	if activeTab.Type() != TabTypeMain {
		t.Error("Expected main tab to be active initially")
	}

	tm.ToggleView()
	activeTab = tm.GetActiveTab()
	if activeTab.Type() != TabTypeAgent {
		t.Error("Expected agent tab to be active after toggle")
	}

	// Test finding agent by ID
	index := tm.FindTabByAgentID("test-123")
	if index != 1 {
		t.Errorf("Expected agent tab at index 1, got %d", index)
	}

	// Test tab removal
	removed := tm.RemoveTab("agent-test-123")
	if !removed {
		t.Error("Expected tab to be removed successfully")
	}

	if len(tm.GetTabs()) != 1 {
		t.Errorf("Expected 1 tab after removal, got %d", len(tm.GetTabs()))
	}
}
