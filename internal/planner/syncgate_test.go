package planner

import (
	"io/fs"
	"os"
	"path/filepath"
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

func TestSyncGate_TemplatesHaveNoPlannerConventionsSkill(t *testing.T) {
	root := filepath.Join(repoRoot(t), "cmd", "kairon", "templates")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(filepath.ToSlash(p), "planner-conventions") {
			t.Errorf("templates must not contain a planner-conventions path: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
}

func TestSyncGate_BuilderConventionsListsSharedSkill(t *testing.T) {
	doc := string(readRepoFile(t, ".kiro/skills/builder-conventions/SKILL.md"))
	row := "| `.kiro/skills/planner-shared/*` | `cmd/kairon/templates/kiro/skills/planner-shared/` |"
	if !strings.Contains(doc, row) {
		t.Errorf("builder-conventions must contain the mapping row %q", row)
	}
	cmd := "cp .kiro/skills/planner-shared/* cmd/kairon/templates/kiro/skills/planner-shared/"
	if !strings.Contains(doc, cmd) {
		t.Errorf("builder-conventions must contain the sync command %q", cmd)
	}
}

func TestSyncGate_SharedSkillTemplateInSync(t *testing.T) {
	live := readRepoFile(t, ".kiro/skills/planner-shared/SKILL.md")
	tmpl := readRepoFile(t, "cmd/kairon/templates/kiro/skills/planner-shared/SKILL.md")
	if string(live) != string(tmpl) {
		t.Error("live planner-shared skill and its template copy differ")
	}
}

// TestSyncGate_DriftDetected proves the scoped diff is a real gate: it passes
// when the files match, fails when one byte is appended to a copy, and passes
// again after the copy is restored. It works on temp copies, never the repo.
func TestSyncGate_DriftDetected(t *testing.T) {
	live := readRepoFile(t, ".kiro/skills/planner-shared/SKILL.md")
	dir := t.TempDir()
	a := filepath.Join(dir, "live")
	b := filepath.Join(dir, "tmpl")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	liveFile := filepath.Join(a, "SKILL.md")
	tmplFile := filepath.Join(b, "SKILL.md")
	if err := os.WriteFile(liveFile, live, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmplFile, live, 0o644); err != nil {
		t.Fatal(err)
	}
	if !dirsMatch(t, a, b) {
		t.Fatal("identical copies must match")
	}
	if err := os.WriteFile(liveFile, append(append([]byte{}, live...), 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirsMatch(t, a, b) {
		t.Fatal("appending a byte must be detected as drift")
	}
	if err := os.WriteFile(liveFile, live, 0o644); err != nil {
		t.Fatal(err)
	}
	if !dirsMatch(t, a, b) {
		t.Fatal("restoring the file must pass again")
	}
}

func dirsMatch(t *testing.T, a, b string) bool {
	t.Helper()
	x, err := os.ReadFile(filepath.Join(a, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	y, err := os.ReadFile(filepath.Join(b, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(x) == string(y)
}
