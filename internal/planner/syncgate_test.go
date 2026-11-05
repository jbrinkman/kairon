package planner

import (
	"strings"
	"testing"
)

const sharedSkillDiff = "diff -rq .kiro/skills/planner-shared/ cmd/kairon/templates/kiro/skills/planner-shared/ || error=1"

// syncCheckBody returns the text of the sync:check task in Taskfile.yml.
func syncCheckBody(t *testing.T) string {
	t.Helper()
	taskfile := string(readRepoFile(t, "Taskfile.yml"))
	start := strings.Index(taskfile, "  sync:check:")
	if start < 0 {
		t.Fatal("Taskfile.yml has no sync:check task")
	}
	body := taskfile[start+len("  sync:check:"):]
	// The task ends at the next top-level task (two-space indent key).
	lines := strings.Split(body, "\n")
	var out []string
	for i, l := range lines {
		if i > 0 && strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && strings.HasSuffix(strings.TrimSpace(l), ":") {
			break
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func TestSyncGate_TaskfileDiffsSharedSkill(t *testing.T) {
	if !strings.Contains(syncCheckBody(t), sharedSkillDiff) {
		t.Errorf("sync:check must contain %q", sharedSkillDiff)
	}
}

func TestSyncGate_TaskfileExcludesPlannerConventions(t *testing.T) {
	if strings.Contains(string(readRepoFile(t, "Taskfile.yml")), "planner-conventions") {
		t.Error("Taskfile.yml must not mention planner-conventions (project-owned, never templated)")
	}
}
