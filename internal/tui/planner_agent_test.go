package tui

import (
	"os"
	"strings"
	"testing"
)

// TestInteractiveLaunchSitesUsePlannerAgentInteractive pins the two places
// that start the interactive planner to planner.AgentInteractive, so the
// agent name is defined once and cannot drift from the planner package.
func TestInteractiveLaunchSitesUsePlannerAgentInteractive(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"planning_tab.go", "commands.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if !strings.Contains(string(src), "planner.AgentInteractive") {
			t.Errorf("%s does not use planner.AgentInteractive", file)
		}
		for i, line := range strings.Split(string(src), "\n") {
			code, _, _ := strings.Cut(line, "//")
			if strings.Contains(code, `"planner"`) {
				t.Errorf("%s:%d uses the string literal \"planner\": %s", file, i+1, strings.TrimSpace(line))
			}
		}
	}
}
