package planner

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

const (
	liveSkillPath     = ".kiro/skills/planner-shared/SKILL.md"
	templateSkillPath = "cmd/kairon/templates/kiro/skills/planner-shared/SKILL.md"
)

func readSkill(t *testing.T) string {
	t.Helper()
	return string(readRepoFile(t, liveSkillPath))
}

func TestSkillConstants(t *testing.T) {
	cases := []struct{ name, got, want string }{
		{"AgentInteractive", AgentInteractive, "planner"},
		{"AgentAutomated", AgentAutomated, "planner-automated"},
		{"SharedSkill", SharedSkill, "planner-shared"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestSkillFrontMatter(t *testing.T) {
	skill := readSkill(t)
	m := regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`).FindStringSubmatch(skill)
	if m == nil {
		t.Fatal("skill does not start with a front matter block")
	}
	if !regexp.MustCompile(`(?m)^name: planner-shared$`).MatchString(m[1]) {
		t.Errorf("front matter lacks 'name: planner-shared':\n%s", m[1])
	}
	desc := regexp.MustCompile(`(?m)^description: (\S.*)$`).FindStringSubmatch(m[1])
	if desc == nil {
		t.Fatalf("front matter lacks a one-line description:\n%s", m[1])
	}
}

func TestSkillSectionsInOrder(t *testing.T) {
	skill := readSkill(t)
	headings := []string{
		"## Restrictions",
		"## Output contract",
		"## Investigate, classify, cover",
		"## Issue-body standard",
		"## Hard bars",
	}
	last := -1
	for _, h := range headings {
		idx := strings.Index(skill, "\n"+h+"\n")
		if idx < 0 {
			t.Errorf("skill lacks section %q", h)
			continue
		}
		if idx < last {
			t.Errorf("section %q is out of order", h)
		}
		last = idx
	}
}

func TestSkillContractFields(t *testing.T) {
	skill := readSkill(t)
	for _, term := range []string{
		"schema_version: 1", "stage: planner", "result: canonical", "result: punch_out",
		"source", "github_issue", "document", "generated", "body", "unresolved",
		"slot", "reason", "inferences", "inferred", "evidence", "RFC 3339",
	} {
		if !strings.Contains(skill, term) {
			t.Errorf("skill does not mention %q", term)
		}
	}
	// The two variants are mutually exclusive, and the skill must say so.
	if !regexp.MustCompile(`(?i)mutually exclusive`).MatchString(skill) {
		t.Error("skill does not say the two result variants are mutually exclusive")
	}
}

var yamlFence = regexp.MustCompile("(?s)```yaml\n(.*?)\n```")

func TestSkillExamplesParseOnePerVariant(t *testing.T) {
	skill := readSkill(t)
	blocks := yamlFence.FindAllStringSubmatch(skill, -1)
	counts := map[stageoutput.Result]int{}
	for i, m := range blocks {
		doc, err := stageoutput.Parse([]byte(m[1]))
		if err != nil {
			t.Errorf("yaml example %d does not parse: %v\n%s", i+1, err, m[1])
			continue
		}
		if doc.Stage != stageoutput.StagePlanner {
			t.Errorf("yaml example %d has stage %q, want planner", i+1, doc.Stage)
		}
		counts[doc.Result]++
	}
	for _, r := range []stageoutput.Result{stageoutput.ResultCanonical, stageoutput.ResultPunchOut} {
		if counts[r] != 1 {
			t.Errorf("skill has %d complete %s examples, want exactly 1", counts[r], r)
		}
	}
}

func TestSkillCanonicalExampleFollowsOwnStandard(t *testing.T) {
	skill := readSkill(t)
	var body string
	for _, m := range yamlFence.FindAllStringSubmatch(skill, -1) {
		doc, err := stageoutput.Parse([]byte(m[1]))
		if err == nil && doc.Result == stageoutput.ResultCanonical {
			body = doc.Body
		}
	}
	if body == "" {
		t.Fatal("no canonical example found")
	}
	for _, want := range []string{
		"# ", "## Problem Statement", "## Context", "## Scope", "In Scope", "Out of Scope",
		"## Acceptance Criteria", "Verify:", "## Constraints", "Dependencies: none",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("canonical example body lacks %q", want)
		}
	}
	if regexp.MustCompile(`(?m)^#+ Dependencies`).MatchString(body) {
		t.Error("canonical example body uses a Dependencies header; it must be a single line")
	}
	for _, key := range []string{"hash", "sha256", "provenance"} {
		if strings.Contains(strings.ToLower(body), key) {
			t.Errorf("canonical example body contains %q", key)
		}
	}
}

func TestSkillInvestigateClassifyCover(t *testing.T) {
	skill := readSkill(t)
	for _, term := range []string{
		"Feature", "Bug", "Refactor",
		"Baseline & Target", "Security", "Migration", "Operational",
		"Functional Requirements", "Interface / Contract", "Edge Cases & Error Handling",
		"Example Interactions", "Reproduction", "Current / Target State",
		"Behavior-Preservation Contract", "Data / Migration",
		"problem and evidence", "desired behavior", "scope boundary", "out of scope",
		"acceptance criteria", "edge and error behavior", "constraints", "dependencies",
		"file reference",
	} {
		if !strings.Contains(skill, term) {
			t.Errorf("skill does not mention %q", term)
		}
	}
}

func TestSkillIssueBodyStandard(t *testing.T) {
	skill := readSkill(t)
	for _, term := range []string{
		"70 characters", "Problem Statement", "Context", "In Scope", "Out of Scope",
		"Acceptance Criteria", "Verify:", "black-box", "Constraints", "Dependencies",
		"User Story", "Verification", "6 In Scope", "8 acceptance criteria",
	} {
		if !strings.Contains(skill, term) {
			t.Errorf("skill does not mention %q", term)
		}
	}
}

func TestSkillDependenciesRule(t *testing.T) {
	skill := readSkill(t)
	for _, term := range []string{
		"Dependencies: #12, #15",
		"Dependencies: none",
		"#{key}",
		"[A-Za-z0-9][A-Za-z0-9_-]*",
		"## Dependencies",
		"bullet",
		"zero",
		"single line",
	} {
		if !strings.Contains(skill, term) {
			t.Errorf("skill does not state the dependencies rule term %q", term)
		}
	}
	// A "## Dependencies" header may only appear as a thing to avoid.
	for _, line := range strings.Split(skill, "\n") {
		if strings.HasPrefix(line, "## Dependencies") {
			t.Errorf("skill uses %q as a real heading; it may only be named as forbidden", line)
		}
	}
}

func TestSkillRestrictions(t *testing.T) {
	skill := readSkill(t)
	for _, term := range []string{
		"gh issue create", "gh issue edit", "gh issue comment", "gh api",
		"label", "provenance", "hash", "sha256",
		"cat", "ls", "grep", "rg", "git log", "git show", "git diff", "git status",
		"gh issue view", "gh issue list", "gh pr view", "gh pr list", ".kairon/config.yaml",
		"read-only", "decline",
	} {
		if !strings.Contains(skill, term) {
			t.Errorf("skill does not mention %q", term)
		}
	}
	section := sectionBetween(skill, "## Restrictions", "## Output contract")
	if !strings.Contains(section, "gh issue create") || !strings.Contains(section, "provenance") {
		t.Error("the prohibitions on gh issue create and provenance belong in the Restrictions section")
	}
}

func TestSkillHardBars(t *testing.T) {
	skill := readSkill(t)
	section := sectionBetween(skill, "## Hard bars", "")
	for _, term := range []string{
		"ungrounded", "properly", "appropriate", "gracefully", "fast", "robust",
		"as needed", "etc.", "should work", "Constraints", "nothing else",
	} {
		if !strings.Contains(section, term) {
			t.Errorf("Hard bars section does not mention %q", term)
		}
	}
}

// TestSkillHasNoModeSpecificText guards the single-skill-for-two-agents design:
// anything only one agent needs belongs in that agent's prompt.
func TestSkillHasNoModeSpecificText(t *testing.T) {
	skill := readSkill(t)
	forbidden := map[string]*regexp.Regexp{
		"one question":         regexp.MustCompile(`(?i)one question`),
		"question":             regexp.MustCompile(`(?i)\bquestions?\b`),
		"decision per turn":    regexp.MustCompile(`(?i)per turn`),
		"a) option list":       regexp.MustCompile(`(?m)(^|\s)[a-c]\) `),
		"Other option":         regexp.MustCompile(`\bOther\b`),
		"Recommended:":         regexp.MustCompile(`(?i)recommend`),
		"draft":                regexp.MustCompile(`(?i)\bdraft`),
		"approval":             regexp.MustCompile(`(?i)\bapprov`),
		"confirm defaults":     regexp.MustCompile(`(?i)confirm`),
		"interactive":          regexp.MustCompile(`(?i)interactive`),
		"automated":            regexp.MustCompile(`(?i)automated`),
		"unattended":           regexp.MustCompile(`(?i)unattended`),
		"watcher":              regexp.MustCompile(`(?i)watcher`),
		"mode":                 regexp.MustCompile(`(?i)\bmodes?\b`),
		"if you are running":   regexp.MustCompile(`(?i)if you are (running|invoked|launched)`),
		"when running":         regexp.MustCompile(`(?i)when (running|invoked|launched) `),
		"human present/absent": regexp.MustCompile(`(?i)\bhuman (is|present|absent)\b`),
		"interview":            regexp.MustCompile(`(?i)interview`),
		"label turn":           regexp.MustCompile(`(?i)label (question|turn)`),
	}
	for name, re := range forbidden {
		if loc := re.FindStringIndex(skill); loc != nil {
			t.Errorf("skill contains mode-specific text (%s): %q", name, skill[loc[0]:min(loc[1]+30, len(skill))])
		}
	}
}

func TestSkillPointsToPromptForVariantPermission(t *testing.T) {
	skill := readSkill(t)
	if !regexp.MustCompile(`(?i)your (agent )?prompt`).MatchString(skill) {
		t.Error("skill must say that the agent's prompt states which result variant it may emit")
	}
}

func TestTemplateSkillIsByteIdentical(t *testing.T) {
	live := readRepoFile(t, liveSkillPath)
	tmpl := readRepoFile(t, templateSkillPath)
	if !bytes.Equal(live, tmpl) {
		t.Errorf("%s differs from %s", templateSkillPath, liveSkillPath)
	}
}

func TestTemplateSkillDirHasOnlySkillFile(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "cmd", "kairon", "templates", "kiro", "skills", "planner-shared")
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if len(files) != 1 || files[0] != "SKILL.md" {
		t.Errorf("template planner-shared files = %v, want [SKILL.md]", files)
	}
}

// planner-conventions is project-owned. It must never be distributed in the
// templates, or kairon update would overwrite a project's local guidance.
func TestTemplatesContainNoPlannerConventionsPath(t *testing.T) {
	root := filepath.Join(repoRoot(t), "cmd", "kairon", "templates")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(p, "planner-conventions") {
			t.Errorf("templates contain a planner-conventions path: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

// sectionBetween returns the text from the start heading up to (not including)
// the end heading; an empty end means the rest of the document.
func sectionBetween(doc, start, end string) string {
	i := strings.Index(doc, "\n"+start+"\n")
	if i < 0 {
		return ""
	}
	rest := doc[i+1:]
	if end == "" {
		return rest
	}
	if j := strings.Index(rest, "\n"+end+"\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}
