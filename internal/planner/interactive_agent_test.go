package planner

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

const (
	interactiveTmplManifest   = "cmd/kairon/templates/kiro/agents/planner.json"
	interactivePromptFileURI  = "file://./planner-prompt.md"
	interactiveManifestModel  = "claude-sonnet-5.5"
	interactiveWriteScopePath = ".worktrees/planning-*"
	// interactiveSessionDocumentPath is where Kairon asks the planner to write
	// the approved document, so the end of a planning session can commit it.
	interactiveSessionDocumentPath = ".kairon/sessions/*.document.yaml"
	// interactiveSessionLabelPath is where Kairon asks the planner to record the
	// person's answer to the trigger-label question (yes or no).
	interactiveSessionLabelPath = ".kairon/sessions/*.label"
)

func TestInteractiveManifest(t *testing.T) {
	m := readManifest(t, interactiveManifest)

	if got := m["name"]; got != AgentInteractive {
		t.Errorf("name = %v, want %q", got, AgentInteractive)
	}
	if got := m["prompt"]; got != interactivePromptFileURI {
		t.Errorf("prompt = %v, want %q", got, interactivePromptFileURI)
	}
	if d, _ := m["description"].(string); strings.TrimSpace(d) == "" {
		t.Error("description is empty")
	}
	if got := m["model"]; got != interactiveManifestModel {
		t.Errorf("model = %v, want %s", got, interactiveManifestModel)
	}

	wantResources := []string{conventionsSkillURI, sharedSkillURI}
	if got := stringList(t, m, "resources"); !reflect.DeepEqual(got, wantResources) {
		t.Errorf("resources = %v, want %v", got, wantResources)
	}

	wantTools := []string{"read", "write", "shell", "web_search", "web_fetch"}
	for _, key := range []string{"tools", "allowedTools"} {
		if got := stringList(t, m, key); !reflect.DeepEqual(got, wantTools) {
			t.Errorf("%s = %v, want %v", key, got, wantTools)
		}
	}

	settings, _ := m["toolsSettings"].(map[string]any)
	write, _ := settings["write"].(map[string]any)
	paths, _ := write["allowedPaths"].([]any)
	wantPaths := []any{interactiveWriteScopePath, interactiveSessionDocumentPath, interactiveSessionLabelPath}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Errorf("toolsSettings.write.allowedPaths = %v, want %v", paths, wantPaths)
	}
}

// writeAllowedPaths returns toolsSettings.write.allowedPaths of a manifest. It
// reads only that field, so it is not affected by other fields the pipeline
// injects into working-tree manifests.
func writeAllowedPaths(t *testing.T, rel string) []string {
	t.Helper()
	m := readManifest(t, rel)
	settings, _ := m["toolsSettings"].(map[string]any)
	write, _ := settings["write"].(map[string]any)
	raw, ok := write["allowedPaths"].([]any)
	if !ok {
		t.Fatalf("%s: toolsSettings.write.allowedPaths is not an array: %v", rel, write["allowedPaths"])
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("%s: allowedPaths has a non-string entry: %v", rel, v)
		}
		out = append(out, s)
	}
	return out
}

// The interactive planner writes the approved document and the label answer
// under .kairon/sessions/ so Kairon can commit the document and apply the
// label when the session ends.
func TestInteractiveManifestAllowsSessionDocument(t *testing.T) {
	got := writeAllowedPaths(t, interactiveManifest)
	for _, want := range []string{interactiveWriteScopePath, interactiveSessionDocumentPath, interactiveSessionLabelPath} {
		if !slices.Contains(got, want) {
			t.Errorf("toolsSettings.write.allowedPaths = %v, missing %q", got, want)
		}
	}
}

func TestInteractiveTemplateManifestAllowedPathsMatchLive(t *testing.T) {
	live := writeAllowedPaths(t, interactiveManifest)
	tmpl := writeAllowedPaths(t, interactiveTmplManifest)
	if !reflect.DeepEqual(live, tmpl) {
		t.Errorf("template allowedPaths = %v, live = %v", tmpl, live)
	}
}

// The pipeline injects a credentials tool and an MCP server block into the
// working-tree manifests. They are setup, not part of the agent.
func TestInteractiveManifestHasNoInjectedCapabilities(t *testing.T) {
	raw := string(readRepoFile(t, interactiveManifest))
	for _, forbidden := range []string{"creds-agent", "mcpServers"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("planner.json must not contain %q", forbidden)
		}
	}
}
