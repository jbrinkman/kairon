package planner

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const (
	automatedManifestPath  = ".kiro/agents/planner-automated.json"
	automatedPromptPath    = ".kiro/agents/planner-automated-prompt.md"
	automatedTmplManifest  = "cmd/kairon/templates/kiro/agents/planner-automated.json"
	automatedTmplPrompt    = "cmd/kairon/templates/kiro/agents/planner-automated-prompt.md"
	interactiveManifest    = ".kiro/agents/planner.json"
	conventionsSkillURI    = "skill://.kiro/skills/planner-conventions/SKILL.md"
	sharedSkillURI         = "skill://.kiro/skills/planner-shared/SKILL.md"
	automatedPromptFileURI = "file://./planner-automated-prompt.md"
)

func readAutomatedPrompt(t *testing.T) string {
	t.Helper()
	return string(readRepoFile(t, automatedPromptPath))
}

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

func TestAutomatedPromptHasRTCCHeadings(t *testing.T) {
	prompt := readAutomatedPrompt(t)
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

func TestAutomatedPromptNeverAsksOffersOrDrafts(t *testing.T) {
	prompt := readAutomatedPrompt(t)
	required := map[string]*regexp.Regexp{
		"never asks a question":         regexp.MustCompile(`(?i)never ask(s)? (a|the requester|anyone|any) ?(question|anything)`),
		"never offers options":          regexp.MustCompile(`(?i)never offer(s)? (the requester )?(an? )?options?`),
		"never drafts for approval":     regexp.MustCompile(`(?i)never writes? a draft for approval`),
		"reply is the document at once": regexp.MustCompile(`(?i)the reply is the document`),
	}
	for name, re := range required {
		if !re.MatchString(prompt) {
			t.Errorf("prompt does not state: %s", name)
		}
	}
}

func TestAutomatedPromptRequiresInferenceEvidence(t *testing.T) {
	prompt := readAutomatedPrompt(t)
	for _, term := range []string{"inferences", "slot", "inferred", "evidence"} {
		if !strings.Contains(prompt, term) {
			t.Errorf("prompt does not mention %q", term)
		}
	}
	if !regexp.MustCompile(`(?i)every inference`).MatchString(prompt) {
		t.Error("prompt must require an inferences entry for every inference")
	}
	if !regexp.MustCompile(`(?i)competent engineer reading the same issue and repository`).MatchString(prompt) {
		t.Error("prompt must define a reasonable inference as one a competent engineer reading the same issue and repository would reach")
	}
	if !regexp.MustCompile(`(?i)coin-flip`).MatchString(prompt) {
		t.Error("prompt must say a coin-flip among materially different options is not reasonable")
	}
}

func TestAutomatedPromptRequiresPunchOut(t *testing.T) {
	prompt := readAutomatedPrompt(t)
	for _, term := range []string{"result: punch_out", "unresolved", "reason"} {
		if !strings.Contains(prompt, term) {
			t.Errorf("prompt does not mention %q", term)
		}
	}
	if !regexp.MustCompile(`(?i)every (required )?slot`).MatchString(prompt) {
		t.Error("prompt must require the unresolved list to name every unresolvable required slot")
	}
	if !regexp.MustCompile(`(?i)never (a|return a|emit a|write a) partial canonical`).MatchString(prompt) {
		t.Error("prompt must forbid a partial canonical document")
	}
}

// The agent has no one to ask, so none of the elicitation machinery belongs in
// its prompt, and the prompt must not choose between behaviours at runtime.
func TestAutomatedPromptHasNoInteractiveOrModeText(t *testing.T) {
	prompt := readAutomatedPrompt(t)
	forbidden := map[string]*regexp.Regexp{
		"one question per turn": regexp.MustCompile(`(?i)one question`),
		"per turn":              regexp.MustCompile(`(?i)per turn`),
		"a) option list":        regexp.MustCompile(`(?m)(^|\s)[a-c]\) `),
		"Other option":          regexp.MustCompile(`\bOther\b`),
		"Recommended:":          regexp.MustCompile(`(?i)recommend`),
		"draft review":          regexp.MustCompile(`(?i)draft review`),
		"approval gate":         regexp.MustCompile(`(?i)\bapprov(e|al) (question|gate|turn)`),
		"confirm defaults":      regexp.MustCompile(`(?i)confirm`),
		"label question":        regexp.MustCompile(`(?i)label (question|turn)`),
		"interactive":           regexp.MustCompile(`(?i)interactive`),
		"mode":                  regexp.MustCompile(`(?i)\bmodes?\b`),
		"if you are running":    regexp.MustCompile(`(?i)if you are (running|invoked|launched)`),
		"when running":          regexp.MustCompile(`(?i)when (running|invoked|launched) `),
		"human present":         regexp.MustCompile(`(?i)\bhuman (is|present|absent)\b`),
		"gh issue create run":   regexp.MustCompile("(?i)`gh issue create"),
	}
	for name, re := range forbidden {
		if loc := re.FindStringIndex(prompt); loc != nil {
			t.Errorf("prompt contains forbidden text (%s): %q", name, prompt[loc[0]:min(loc[1]+30, len(prompt))])
		}
	}
}

// The contract and the body spine live once, in the shared skill.
func TestAutomatedPromptDoesNotRestateSharedContract(t *testing.T) {
	prompt := readAutomatedPrompt(t)
	for _, term := range []string{
		"schema_version", "source.kind", "kind: github_issue", "generated:", "body: |",
		"Problem Statement", "In Scope", "Out of Scope", "Acceptance Criteria",
		"Verify:", "Dependencies:", "70 characters",
	} {
		if strings.Contains(prompt, term) {
			t.Errorf("prompt restates shared content %q", term)
		}
	}
	if !strings.Contains(prompt, "planner-shared") {
		t.Error("prompt must point at the planner-shared skill")
	}
}

func TestAutomatedFilesMatchTemplates(t *testing.T) {
	for _, pair := range [][2]string{
		{automatedManifestPath, automatedTmplManifest},
		{automatedPromptPath, automatedTmplPrompt},
	} {
		live, tmpl := readRepoFile(t, pair[0]), readRepoFile(t, pair[1])
		if !bytes.Equal(live, tmpl) {
			t.Errorf("%s differs from %s", pair[1], pair[0])
		}
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
