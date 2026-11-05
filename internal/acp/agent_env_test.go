package acp

import (
	"context"
	"testing"

	"github.com/jbrinkman/kairon/internal/kaironbin/kaironbintest"
)

// An ACP agent session reaches the kairon binary that started it.
func TestACPCommandPutsRunningKaironOnPath(t *testing.T) {
	t.Chdir(t.TempDir())
	c := NewClient(&ConnectionConfig{KiroCLIPath: "kiro-cli", Agent: "planner"})
	kaironbintest.AssertEnv(t, c.command(context.Background()).Env)
}
