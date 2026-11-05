package planner

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

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
