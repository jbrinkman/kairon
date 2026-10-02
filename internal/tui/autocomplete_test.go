package tui

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

func TestAutocompleteInput(t *testing.T) {
	cfg := &config.Config{}
	manager := agent.NewManager(cfg)
	registry := NewCommandRegistry(manager)
	theme := &config.Theme{} // Use default theme
	styles := NewStyles(theme)

	autocomplete := NewAutocompleteInput(registry, styles)

	// Test initial state
	if autocomplete.Value() != "" {
		t.Error("Expected empty initial value")
	}

	// Test setting value and suggestions update
	autocomplete.SetValue("w")
	suggestions := autocomplete.textinput.AvailableSuggestions()
	if len(suggestions) == 0 {
		t.Error("Expected suggestions to be available after typing 'w'")
	}

	// Test valid command detection
	autocomplete.SetValue("watch start")
	if !autocomplete.IsValidCommand() {
		t.Error("Expected 'watch start' to be valid command")
	}

	autocomplete.SetValue("invalid command")
	if autocomplete.IsValidCommand() {
		t.Error("Expected 'invalid command' to be invalid")
	}
}
