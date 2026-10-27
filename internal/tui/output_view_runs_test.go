package tui

import (
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

// An intake run and a krew-lead run on the same issue each show only their own
// output in their tabs.
func TestOutputView_IntakeAndKrewLeadTabsKeepTheirOwnOutput(t *testing.T) {
	cfg := &config.Config{LoadedTheme: &config.Theme{}}
	manager := agent.NewManager(cfg)
	manager.RegisterAgent("intake-42-1", 42)
	intake := manager.GetAgent("intake-42-1")
	intake.Kind = agent.KindIntake
	manager.RegisterAgent("agent-42-1", 42)
	manager.GetAgent("agent-42-1").Kind = agent.KindImplementation

	manager.CaptureAgentLine(intake, "planning the issue")
	manager.CaptureOutputLine(42, "building the feature")

	for id, want := range map[string][2]string{
		"intake-42-1": {"planning the issue", "building the feature"},
		"agent-42-1":  {"building the feature", "planning the issue"},
	} {
		view := NewOutputViewForAgent(id, manager, NewStyles(cfg.LoadedTheme))
		view.Resize(120, 20)
		got := view.View()
		if !strings.Contains(got, want[0]) {
			t.Errorf("tab %s does not show %q:\n%s", id, want[0], got)
		}
		if strings.Contains(got, want[1]) {
			t.Errorf("tab %s shows the other run's %q:\n%s", id, want[1], got)
		}
	}
}
