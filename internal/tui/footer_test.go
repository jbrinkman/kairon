package tui

import (
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

func TestFooterRendersExactly3Lines(t *testing.T) {
	cfg := &config.Config{Theme: "default"}
	manager := agent.NewManager(cfg)
	registry := NewCommandRegistry(manager)
	theme := &config.Theme{}
	styles := NewStyles(theme)
	autocomplete := NewAutocompleteInput(registry, styles)
	tabManager := NewTabManager()

	fm := NewFooterManager(styles, cfg, autocomplete, tabManager)
	fm.Resize(80, 24)

	tests := []struct {
		name    string
		tabType TabType
	}{
		{"main tab", TabTypeMain},
		{"planning tab", TabTypePlanning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := fm.RenderWithSeparator(tt.tabType)
			lines := strings.Split(rendered, "\n")
			expected := fm.GetFooterHeight()
			if len(lines) != expected {
				t.Errorf("RenderWithSeparator(%v) produced %d lines, expected %d\nContent: %q",
					tt.tabType, len(lines), expected, rendered)
			}
		})
	}
}

func TestFooterDropdownRendersExactly3LinesWithoutDropdown(t *testing.T) {
	cfg := &config.Config{Theme: "default"}
	manager := agent.NewManager(cfg)
	registry := NewCommandRegistry(manager)
	theme := &config.Theme{}
	styles := NewStyles(theme)
	autocomplete := NewAutocompleteInput(registry, styles)
	tabManager := NewTabManager()

	fm := NewFooterManager(styles, cfg, autocomplete, tabManager)
	fm.Resize(80, 24)

	tests := []struct {
		name    string
		tabType TabType
	}{
		{"main tab", TabTypeMain},
		{"planning tab", TabTypePlanning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := fm.RenderWithSeparator(tt.tabType)
			lines := strings.Split(rendered, "\n")
			expected := fm.GetFooterHeight()
			if len(lines) != expected {
				t.Errorf("RenderWithSeparator(%v) produced %d lines, expected %d\nContent: %q",
					tt.tabType, len(lines), expected, rendered)
			}
		})
	}
}
