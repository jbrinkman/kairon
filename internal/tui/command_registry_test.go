package tui

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
)

func TestCommandRegistry(t *testing.T) {
	cfg := &config.Config{}
	manager := agent.NewManager(cfg)
	registry := NewCommandRegistry(manager)

	// Test command filtering
	matches := registry.FilterCommands("w")
	if len(matches) != 1 || matches[0].Name != "watch" {
		t.Errorf("Expected 1 match for 'w', got %d", len(matches))
	}

	// Test subcommand filtering
	subcommands := registry.GetSubcommands("watch")
	if len(subcommands) != 2 {
		t.Errorf("Expected 2 subcommands for 'watch', got %d", len(subcommands))
	}

	// Test best match
	match := registry.GetBestMatch("wat")
	if match != "watch" {
		t.Errorf("Expected 'watch' for 'wat', got '%s'", match)
	}

	// Test valid command
	if !registry.IsValidCommand("watch start") {
		t.Error("Expected 'watch start' to be valid")
	}

	if registry.IsValidCommand("invalid command") {
		t.Error("Expected 'invalid command' to be invalid")
	}
}

func TestPlanClassicAutocomplete(t *testing.T) {
	cfg := &config.Config{}
	manager := agent.NewManager(cfg)
	registry := NewCommandRegistry(manager)

	// Test plan classic subcommand exists
	subcommands := registry.GetSubcommands("plan")
	if len(subcommands) != 1 || subcommands[0] != "classic" {
		t.Errorf("Expected plan to have 'classic' subcommand, got %v", subcommands)
	}

	// Test plan classic in flattened matches
	matches := registry.GetFlattenedMatches("plan")
	foundPlan := false
	foundPlanClassic := false
	for _, match := range matches {
		if match == "plan" {
			foundPlan = true
		}
		if match == "plan classic" {
			foundPlanClassic = true
		}
	}
	if !foundPlan {
		t.Error("Expected 'plan' in flattened matches")
	}
	if !foundPlanClassic {
		t.Error("Expected 'plan classic' in flattened matches")
	}

	// Test plan classic validation
	if !registry.IsValidCommand("plan classic") {
		t.Error("Expected 'plan classic' to be valid")
	}

	if !registry.IsValidCommand("plan classic some description") {
		t.Error("Expected 'plan classic some description' to be valid")
	}

	// Test backward compatibility for base plan command with description
	if !registry.IsValidCommand("plan some description") {
		t.Error("Expected 'plan some description' to be valid")
	}

	if !registry.IsValidCommand("plan implement feature X") {
		t.Error("Expected 'plan implement feature X' to be valid")
	}

	// Test prefix filtering for plan classic
	matches = registry.GetFlattenedMatches("plan c")
	foundPlanClassic = false
	for _, match := range matches {
		if match == "plan classic" {
			foundPlanClassic = true
			break
		}
	}
	if !foundPlanClassic {
		t.Error("Expected 'plan classic' in matches for 'plan c'")
	}

	// Test partial subcommand filtering
	subcommands = registry.GetSubcommands("plan c")
	if len(subcommands) != 1 || subcommands[0] != "classic" {
		t.Errorf("Expected 'plan c' to filter to 'classic' subcommand, got %v", subcommands)
	}

	// Test best match for plan classic
	match := registry.GetBestMatch("plan cl")
	if match != "plan classic" {
		t.Errorf("Expected 'plan classic' for 'plan cl', got '%s'", match)
	}
}
