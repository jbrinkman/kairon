package planner

import (
	"testing"

	"github.com/jbrinkman/kairon/internal/kaironbin/kaironbintest"
)

// The automated planner (intake) reaches the kairon binary that started it.
func TestAutomatedCommandPutsRunningKaironOnPath(t *testing.T) {
	t.Chdir(t.TempDir())
	kaironbintest.AssertEnv(t, AutomatedCommand("p", t.TempDir()).Env)
}
