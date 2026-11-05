package agent

import (
	"io"
	"testing"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/kaironbin/kaironbintest"
)

// The implementation agent reaches the kairon binary that started it.
func TestAgentCommandPutsRunningKaironOnPath(t *testing.T) {
	t.Chdir(t.TempDir())
	m := NewManager(&config.Config{})
	cmd := m.agentCommand(7, "o/r", "issue-7", t.TempDir(), "/in.yaml", io.Discard)
	kaironbintest.AssertEnv(t, cmd.Env)
}
