package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAgentRegistry(t *testing.T) {
	registry := NewAgentRegistry()
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if registry.Count() != 0 {
		t.Errorf("expected empty registry, got count: %d", registry.Count())
	}
}

// writeLeadConfig writes a krew-lead.json with the given trustedAgents list
// into agentDir and returns agentDir, for exercising DiscoverAgents.
func writeLeadConfig(t *testing.T, agentDir string, trusted []string) {
	t.Helper()
	quoted := make([]string, len(trusted))
	for i, a := range trusted {
		quoted[i] = `"` + a + `"`
	}
	content := `{
  "name": "krew-lead",
  "toolsSettings": { "subagent": { "trustedAgents": [` + strings.Join(quoted, ", ") + `] } }
}`
	path := filepath.Join(agentDir, "krew-lead.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write krew-lead.json: %v", err)
	}
}

// TestDiscoverAgents_FromTrustedList verifies the registry is built from the
// orchestrator's trustedAgents list, including an agent added purely by editing
// that list (no recompile), and that non-trusted names are absent.
func TestDiscoverAgents_FromTrustedList(t *testing.T) {
	tmpDir := t.TempDir()
	// Includes a newly-added specialized agent to prove the no-recompile goal.
	writeLeadConfig(t, tmpDir, []string{"architect", "builder", "validator", "documenter", "security-reviewer"})

	registry, err := DiscoverAgents(tmpDir)
	if err != nil {
		t.Fatalf("expected successful discovery, got error: %v", err)
	}

	if registry.Count() != 5 {
		t.Errorf("expected 5 trusted agents, got %d", registry.Count())
	}
	for _, name := range []string{"architect", "builder", "validator", "documenter", "security-reviewer"} {
		if !registry.Contains(name) {
			t.Errorf("expected registry to contain trusted agent '%s'", name)
		}
		path, ok := registry.GetConfigPath(name)
		if !ok || path == "" {
			t.Errorf("expected a config path for trusted agent '%s'", name)
		}
	}
	// A name not in the trusted list (even krew-lead/planner themselves) must
	// not be in the registry.
	for _, name := range []string{"planner", "krew-lead", "kiro_default"} {
		if registry.Contains(name) {
			t.Errorf("did not expect non-trusted agent '%s' in registry", name)
		}
	}
}

func TestDiscoverAgents_MissingLeadConfig(t *testing.T) {
	// Empty dir: no krew-lead.json -> hard error (orchestrator is broken).
	tmpDir := t.TempDir()
	if _, err := DiscoverAgents(tmpDir); err == nil {
		t.Error("expected error when krew-lead.json is missing")
	}
}

func TestDiscoverAgents_NonexistentDirectory(t *testing.T) {
	_, err := DiscoverAgents("/nonexistent/directory/path")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestDiscoverAgents_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "krew-lead.json"), []byte(`{invalid json`), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if _, err := DiscoverAgents(tmpDir); err == nil {
		t.Error("expected error for invalid krew-lead.json")
	}
}

func TestDiscoverAgents_EmptyTrustedList(t *testing.T) {
	tmpDir := t.TempDir()
	writeLeadConfig(t, tmpDir, []string{})
	if _, err := DiscoverAgents(tmpDir); err == nil {
		t.Error("expected error when trustedAgents is empty")
	}
}

func TestAgentRegistry_Contains(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents["builder"] = "/path/to/builder.json"
	registry.agents["validator"] = "/path/to/validator.json"

	tests := []struct {
		name     string
		expected bool
	}{
		{"builder", true},
		{"validator", true},
		{"architect", false},
		{"", false},
	}

	for _, tt := range tests {
		result := registry.Contains(tt.name)
		if result != tt.expected {
			t.Errorf("Contains(%q) = %v, expected %v", tt.name, result, tt.expected)
		}
	}
}

func TestAgentRegistry_GetConfigPath(t *testing.T) {
	registry := NewAgentRegistry()
	expectedPath := "/path/to/builder.json"
	registry.agents["builder"] = expectedPath

	// Test existing agent
	path, exists := registry.GetConfigPath("builder")
	if !exists {
		t.Error("expected to find builder")
	}
	if path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, path)
	}

	// Test non-existent agent
	path, exists = registry.GetConfigPath("nonexistent")
	if exists {
		t.Error("expected not to find nonexistent agent")
	}
	if path != "" {
		t.Errorf("expected empty path for non-existent agent, got %s", path)
	}
}

func TestAgentRegistry_GetAgentNames(t *testing.T) {
	registry := NewAgentRegistry()
	registry.agents["builder"] = "/path/to/builder.json"
	registry.agents["validator"] = "/path/to/validator.json"
	registry.agents["architect"] = "/path/to/architect.json"

	names := registry.GetAgentNames()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}

	// Check that all expected names are present (order doesn't matter)
	expectedNames := map[string]bool{
		"builder":   false,
		"validator": false,
		"architect": false,
	}

	for _, name := range names {
		if _, ok := expectedNames[name]; ok {
			expectedNames[name] = true
		} else {
			t.Errorf("unexpected agent name: %s", name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("expected to find agent name: %s", name)
		}
	}
}

func TestAgentRegistry_Count(t *testing.T) {
	registry := NewAgentRegistry()
	if registry.Count() != 0 {
		t.Errorf("expected count 0, got %d", registry.Count())
	}

	registry.agents["builder"] = "/path/to/builder.json"
	if registry.Count() != 1 {
		t.Errorf("expected count 1, got %d", registry.Count())
	}

	registry.agents["validator"] = "/path/to/validator.json"
	registry.agents["architect"] = "/path/to/architect.json"
	if registry.Count() != 3 {
		t.Errorf("expected count 3, got %d", registry.Count())
	}
}
