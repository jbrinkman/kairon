package planner

import (
	"reflect"
	"strings"
	"testing"
)

const (
	automatedManifestPath  = ".kiro/agents/planner-automated.json"
	interactiveManifest    = ".kiro/agents/planner.json"
	conventionsSkillURI    = "skill://.kiro/skills/planner-conventions/SKILL.md"
	sharedSkillURI         = "skill://.kiro/skills/planner-shared/SKILL.md"
	automatedPromptFileURI = "file://./planner-automated-prompt.md"
)

// stringList reads a JSON array of strings from a manifest field.
func stringList(t *testing.T, m map[string]any, key string) []string {
	t.Helper()
	raw, ok := m[key].([]any)
	if !ok {
		t.Fatalf("manifest field %q is not an array: %v", key, m[key])
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("manifest field %q has a non-string entry: %v", key, v)
		}
		out = append(out, s)
	}
	return out
}

func TestAutomatedManifest(t *testing.T) {
	m := readManifest(t, automatedManifestPath)

	if got := m["name"]; got != AgentAutomated {
		t.Errorf("name = %v, want %q", got, AgentAutomated)
	}
	if got := m["prompt"]; got != automatedPromptFileURI {
		t.Errorf("prompt = %v, want %q", got, automatedPromptFileURI)
	}
	if d, _ := m["description"].(string); strings.TrimSpace(d) == "" {
		t.Error("description is empty")
	}
	if got := m["model"]; got != "claude-sonnet-5.5" {
		t.Errorf("model = %v, want claude-sonnet-5.5", got)
	}

	wantTools := []string{"read", "shell"}
	for _, key := range []string{"tools", "allowedTools"} {
		if got := stringList(t, m, key); !reflect.DeepEqual(got, wantTools) {
			t.Errorf("%s = %v, want exactly %v", key, got, wantTools)
		}
	}

	wantResources := []string{conventionsSkillURI, sharedSkillURI}
	if got := stringList(t, m, "resources"); !reflect.DeepEqual(got, wantResources) {
		t.Errorf("resources = %v, want %v", got, wantResources)
	}
}

// The unattended agent has no write tool, no web tools, and none of the
// pipeline-injected credential plumbing.
func TestAutomatedManifestHasNoInjectedOrWriteCapabilities(t *testing.T) {
	raw := string(readRepoFile(t, automatedManifestPath))
	for _, forbidden := range []string{"creds-agent", "mcpServers", "\"write\"", "web_search", "web_fetch", "--trust-all-tools"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("planner-automated.json must not contain %q", forbidden)
		}
	}
}

func TestAutomatedManifestShellIsReadOnly(t *testing.T) {
	m := readManifest(t, automatedManifestPath)
	settings, _ := m["toolsSettings"].(map[string]any)
	shell, _ := settings["shell"].(map[string]any)
	if shell["autoAllowReadonly"] != true {
		t.Errorf("toolsSettings.shell.autoAllowReadonly = %v, want true", shell["autoAllowReadonly"])
	}
}

// Both agents load the one shared skill through the same URI.
func TestPlannerManifestsListSameSharedSkill(t *testing.T) {
	for _, path := range []string{interactiveManifest, automatedManifestPath} {
		found := false
		for _, r := range stringList(t, readManifest(t, path), "resources") {
			if r == sharedSkillURI {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not list %s", path, sharedSkillURI)
		}
	}
}
