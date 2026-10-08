package eval

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jbrinkman/kairon/internal/inference"
)

// resolveTrustSet returns the whole-tool trust set for a container run of
// agent. It is used only for sandbox runs; native runs keep --trust-all-tools.
//
// First match wins:
//  1. evals.trust_tools[agent] from .kairon/config.yaml (may be empty =
//     trust nothing);
//  2. the agent config's allowedTools (the same file resolveAgentProvenance
//     reads: <evals-dir>/agents/<agent>.json, else .kiro/agents/<agent>.json);
//  3. neither: an empty set (fail closed).
//
// The result is never nil. When no override applies, a missing or unreadable
// agent config is an error (listing the paths tried) rather than a silent
// empty set.
func resolveTrustSet(agent string) (*inference.ToolTrust, error) {
	if tools, ok := cfg.pins.trustOverride(agent); ok {
		return inference.NewToolTrust(tools), nil
	}

	// The overlay is always visible to the container agent (it is staged into
	// the workspace .kiro), so it is considered here too.
	path, err := locateAgentConfig(agent, false)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading agent config %s: %w", path, err)
	}
	var conf agentConfigFile
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("parsing agent config %s: %w", path, err)
	}
	return inference.NewToolTrust(conf.AllowedTools), nil
}
