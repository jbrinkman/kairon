package planner

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const (
	interactivePromptPath     = ".kiro/agents/planner-prompt.md"
	interactiveTmplManifest   = "cmd/kairon/templates/kiro/agents/planner.json"
	interactiveTmplPrompt     = "cmd/kairon/templates/kiro/agents/planner-prompt.md"
	interactivePromptFileURI  = "file://./planner-prompt.md"
	interactiveManifestModel  = "claude-sonnet-5.5"
	interactiveWriteScopePath = ".worktrees/planning-*"
)

func readInteractivePrompt(t *testing.T) string {
	t.Helper()
	return string(readRepoFile(t, interactivePromptPath))
}

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
	if len(paths) != 1 || paths[0] != interactiveWriteScopePath {
		t.Errorf("toolsSettings.write.allowedPaths = %v, want [%s]", paths, interactiveWriteScopePath)
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

func TestInteractivePromptHasRTCCHeadings(t *testing.T) {
	prompt := readInteractivePrompt(t)
	last := -1
	for _, h := range []string{"## Role", "## Task", "## Context", "## Constraints"} {
		idx := strings.Index(prompt, "\n"+h+"\n")
		if idx < 0 {
			t.Errorf("prompt lacks heading %q", h)
			continue
		}
		if idx < last {
			t.Errorf("heading %q is out of order", h)
		}
		last = idx
	}
}

// Every element of the elicitation flow must be stated in the prompt.
func TestInteractivePromptEncodesElicitationFlow(t *testing.T) {
	prompt := readInteractivePrompt(t)
	required := map[string]*regexp.Regexp{
		"investigate before asking":          regexp.MustCompile(`(?i)investigate before (you )?ask`),
		"repository answers are not asked":   regexp.MustCompile(`(?i)file reference`),
		"coverage map":                       regexp.MustCompile(`(?i)coverage map`),
		"one decision per turn":              regexp.MustCompile(`(?i)one decision per turn`),
		"a/b/c/Other options":                regexp.MustCompile(`(?s)a\).*b\).*c\).*Other`),
		"single-commitment option rule":      regexp.MustCompile(`(?i)single,? unambiguous commitment`),
		"and is allowed":                     regexp.MustCompile(`(?i)joined by \*?and\*?`),
		"or is forbidden":                    regexp.MustCompile(`(?i)never (by|joined by|with) \*?or\*?`),
		"split genuine alternatives":         regexp.MustCompile(`(?i)separate options`),
		"recommend exactly one option":       regexp.MustCompile(`(?i)recommend exactly one`),
		"no conditional recommendation":      regexp.MustCompile(`(?i)never a (branching|conditional)`),
		"no recommendation when none chosen": regexp.MustCompile(`(?i)without a recommendation`),
		"constraints asked not invented":     regexp.MustCompile(`(?i)constraints? (is|are) asked,? never invented`),
		"no preference means no constraint":  regexp.MustCompile(`(?i)no preference.{0,40}no constraint`),
		"escape hatch":                       regexp.MustCompile(`(?i)escape hatch`),
		"confirm defaults turn":              regexp.MustCompile(`(?i)confirm(-| )defaults`),
		"draft review":                       regexp.MustCompile(`(?i)draft review`),
		"coverage summary outside the issue": regexp.MustCompile(`(?i)coverage summary.{0,60}outside`),
		"one approval question":              regexp.MustCompile(`(?i)(one|single) approval question`),
		"label turn":                         regexp.MustCompile(`(?i)label (question|turn)`),
		"return the document":                regexp.MustCompile(`(?i)return the (stage-output )?document`),
		"oversize split":                     regexp.MustCompile(`(?i)split`),
		"one-line summaries":                 regexp.MustCompile(`(?i)one-line summar`),
		"dependency order":                   regexp.MustCompile(`(?i)dependency order`),
		"continue with only the first":       regexp.MustCompile(`(?i)only the first`),
		"short key per summary":              regexp.MustCompile(`(?i)kebab-case .{0,10}key`),
		"root cause versus symptom":          regexp.MustCompile(`(?i)root cause`),
	}
	for name, re := range required {
		if !re.MatchString(prompt) {
			t.Errorf("prompt does not state: %s", name)
		}
	}
}

func TestInteractivePromptNeverPunchesOut(t *testing.T) {
	prompt := readInteractivePrompt(t)
	if !regexp.MustCompile("(?i)never (return|emit|write)s? `?(result: )?punch_out").MatchString(prompt) {
		t.Error("prompt must say the interactive planner never returns punch_out")
	}
	if !regexp.MustCompile(`(?i)open required slot`).MatchString(prompt) {
		t.Error("prompt must say an open required slot is a question")
	}
	// punch_out may only appear in a prohibition.
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.Contains(line, "punch_out") {
			continue
		}
		if !regexp.MustCompile(`(?i)\b(never|not|no)\b`).MatchString(line) {
			t.Errorf("punch_out appears outside a prohibition: %q", line)
		}
	}
}

func TestInteractivePromptDoesNotPublishOrHash(t *testing.T) {
	prompt := readInteractivePrompt(t)
	if !strings.Contains(prompt, "gh issue create") {
		t.Error("prompt must tell the agent not to run gh issue create")
	}
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.Contains(line, "gh issue create") {
			continue
		}
		if !regexp.MustCompile(`(?i)\b(never|not|no)\b`).MatchString(line) {
			t.Errorf("gh issue create appears outside a prohibition: %q", line)
		}
	}
	if !regexp.MustCompile(`(?i)(never|not|no) (write|compute|add)s? (a )?(provenance|hash)`).MatchString(prompt) &&
		!regexp.MustCompile(`(?i)never .{0,40}\bhash\b`).MatchString(prompt) {
		t.Error("prompt must tell the agent not to write a hash")
	}
	if regexp.MustCompile("(?m)^\\s*```(bash|sh)?\\s*\\n\\s*gh issue").MatchString(prompt) {
		t.Error("prompt must not contain a gh issue command block")
	}
	if strings.Contains(prompt, "--body-file") || strings.Contains(prompt, "/tmp/issue-body.md") {
		t.Error("prompt must not describe writing an issue body file")
	}
}

// The contract and the body spine live once, in the shared skill, and the
// prompt must not choose between behaviours at runtime.
func TestInteractivePromptDoesNotRestateSharedContractOrSelectMode(t *testing.T) {
	prompt := readInteractivePrompt(t)
	for _, term := range []string{
		"schema_version", "source.kind", "kind: github_issue", "generated:", "body: |",
		"Problem Statement", "In Scope", "Out of Scope", "Acceptance Criteria",
		"Verify:", "Dependencies:", "70 characters", "User Story",
	} {
		if strings.Contains(prompt, term) {
			t.Errorf("prompt restates shared content %q", term)
		}
	}
	if !strings.Contains(prompt, "planner-shared") {
		t.Error("prompt must point at the planner-shared skill")
	}
	forbidden := map[string]*regexp.Regexp{
		"mode":               regexp.MustCompile(`(?i)\bmodes?\b`),
		"automated":          regexp.MustCompile(`(?i)\bautomated\b`),
		"unattended":         regexp.MustCompile(`(?i)\bunattended\b`),
		"watcher":            regexp.MustCompile(`(?i)\bwatcher\b`),
		"if you are running": regexp.MustCompile(`(?i)if you are (running|invoked|launched)`),
		"when running":       regexp.MustCompile(`(?i)when (running|invoked|launched)\b`),
		"no human":           regexp.MustCompile(`(?i)\bno human\b`),
	}
	for name, re := range forbidden {
		if loc := re.FindStringIndex(prompt); loc != nil {
			t.Errorf("prompt contains mode-selection text (%s): %q", name, prompt[loc[0]:min(loc[1]+30, len(prompt))])
		}
	}
}

func TestInteractiveFilesMatchTemplates(t *testing.T) {
	for _, pair := range [][2]string{
		{interactiveManifest, interactiveTmplManifest},
		{interactivePromptPath, interactiveTmplPrompt},
	} {
		live, tmpl := readRepoFile(t, pair[0]), readRepoFile(t, pair[1])
		if !bytes.Equal(live, tmpl) {
			t.Errorf("%s differs from %s", pair[1], pair[0])
		}
	}
}
