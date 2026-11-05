package session

import (
	"context"
	"testing"

	"github.com/jbrinkman/kairon/internal/kaironbin/kaironbintest"
)

// A planner session process reaches the kairon binary that started it.
func TestPlannerCommandPutsRunningKaironOnPath(t *testing.T) {
	t.Chdir(t.TempDir())
	kaironbintest.AssertEnv(t, plannerCommand(context.Background()).Env)
}
