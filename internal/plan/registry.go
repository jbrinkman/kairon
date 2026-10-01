package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AgentRegistry maintains a mapping of agent names to their configuration file paths
type AgentRegistry struct {
	agents map[string]string // agent name -> config file path
	mu     sync.RWMutex
}

// NewAgentRegistry creates a new empty agent registry
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]string),
	}
}

// DiscoverAgents builds the agent registry from the orchestrator's trusted-agent
// list in <agentDir>/krew-lead.json (toolsSettings.subagent.trustedAgents).
//
// The trusted list is the single source of truth for which agents may be
// assigned workflow tasks: a user adds a new agent by creating its config and
// adding its name to krew-lead's trustedAgents, with no recompile. We
// deliberately do NOT glob and trust arbitrary *.json configs — trusting any
// discovered config is an injection surface that needs a dedicated design first.
//
// krew-lead.json is required: without it the orchestrator cannot delegate at
// all, so a missing/unreadable file or an empty trusted list is a hard error.
func DiscoverAgents(agentDir string) (*AgentRegistry, error) {
	registry := NewAgentRegistry()

	leadConfigPath := filepath.Join(agentDir, "krew-lead.json")
	data, err := os.ReadFile(leadConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read orchestrator config %s: %w", leadConfigPath, err)
	}

	var leadConfig struct {
		ToolsSettings struct {
			Subagent struct {
				TrustedAgents []string `json:"trustedAgents"`
			} `json:"subagent"`
		} `json:"toolsSettings"`
	}
	if err := json.Unmarshal(data, &leadConfig); err != nil {
		return nil, fmt.Errorf("failed to parse orchestrator config %s: %w", leadConfigPath, err)
	}

	trusted := leadConfig.ToolsSettings.Subagent.TrustedAgents
	if len(trusted) == 0 {
		return nil, fmt.Errorf("orchestrator config %s has no toolsSettings.subagent.trustedAgents; the workflow cannot delegate without a trusted-agent list", leadConfigPath)
	}

	// Register each trusted agent by name, mapping to its expected config path
	// in the same directory (the file need not exist here — a trusted agent
	// that cannot actually be spawned will fail at delegation time).
	for _, name := range trusted {
		if name == "" {
			continue
		}
		registry.agents[name] = filepath.Join(agentDir, name+".json")
	}

	if len(registry.agents) == 0 {
		return nil, fmt.Errorf("orchestrator config %s trustedAgents contained no usable agent names", leadConfigPath)
	}

	return registry, nil
}

// Contains checks if an agent with the given name exists in the registry
func (r *AgentRegistry) Contains(agentName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.agents[agentName]
	return exists
}

// GetConfigPath returns the configuration file path for a given agent name
func (r *AgentRegistry) GetConfigPath(agentName string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	path, exists := r.agents[agentName]
	return path, exists
}

// GetAgentNames returns a slice of all registered agent names
func (r *AgentRegistry) GetAgentNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered agents
func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}
