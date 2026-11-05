package tui

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/kaironbin/kaironbintest"
)

// The interactive classic planner reaches the kairon binary that started it.
func TestClassicPlanningCmdPutsRunningKaironOnPath(t *testing.T) {
	t.Chdir(t.TempDir())
	kaironbintest.AssertEnv(t, classicPlanningCmd("s1", "p").Env)
}
