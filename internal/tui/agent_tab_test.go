package tui

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

func TestAgentTabTitle(t *testing.T) {
	cfg := &config.Config{
		MaxRetries: 1,
		LoadedTheme: &config.Theme{
			Name: "dark",
		},
	}
	manager := agent.NewManager(cfg)
	styles := NewStyles(cfg.LoadedTheme)

	t.Run("returns issue number when agent found", func(t *testing.T) {
		manager.RegisterAgent("agent-42", 42)
		tab := NewAgentTab("agent-42", manager, styles)

		title := tab.Title()
		if title != "Issue 42" {
			t.Errorf("expected 'Issue 42', got %q", title)
		}
	})

	t.Run("falls back to agent ID when agent not found", func(t *testing.T) {
		tab := NewAgentTab("nonexistent", manager, styles)

		title := tab.Title()
		if title != "Agent nonexistent" {
			t.Errorf("expected 'Agent nonexistent', got %q", title)
		}
	})
}
