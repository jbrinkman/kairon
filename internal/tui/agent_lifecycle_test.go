package tui

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

func TestAgentLifecycleIntegration(t *testing.T) {
	cfg := &config.Config{
		Theme:      "dark",
		MaxRetries: 1,
		LoadedTheme: &config.Theme{
			Name: "dark",
		},
	}
	manager := agent.NewManager(cfg)

	// Create a mock TUI model
	model := newModel(nil, manager, cfg, nil, nil)

	// Initially should have only the main tab
	if len(model.tabManager.GetTabs()) != 1 {
		t.Errorf("Expected 1 tab initially, got %d", len(model.tabManager.GetTabs()))
	}

	// Test that updateAgentTabs doesn't crash with empty agent list
	model = model.updateAgentTabs()

	// Should still have only main tab
	if len(model.tabManager.GetTabs()) != 1 {
		t.Errorf("Expected 1 tab after updateAgentTabs, got %d", len(model.tabManager.GetTabs()))
	}

	// Verify knownAgents is initialized
	if model.knownAgents == nil {
		t.Error("knownAgents map should be initialized")
	}

	// Test tab creation manually (simulating what would happen when agent starts)
	agentTab := NewAgentTab("test-agent-123", manager, model.styles)
	model.tabManager.AddTab(agentTab)

	if len(model.tabManager.GetTabs()) != 2 {
		t.Errorf("Expected 2 tabs after manual tab addition, got %d", len(model.tabManager.GetTabs()))
	}

	// Test tab removal
	removed := model.tabManager.RemoveTab("agent-test-agent-123")
	if !removed {
		t.Error("Failed to remove agent tab manually")
	}

	if len(model.tabManager.GetTabs()) != 1 {
		t.Errorf("Expected 1 tab after manual removal, got %d", len(model.tabManager.GetTabs()))
	}
}
