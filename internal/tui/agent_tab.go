package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/agent"
)

// AgentTab implements Tab interface for individual agent views
type AgentTab struct {
	agentID    string
	outputView *OutputView
}

// NewAgentTab creates a new agent tab
func NewAgentTab(agentID string, manager *agent.Manager, styles *Styles) *AgentTab {
	return &AgentTab{
		agentID:    agentID,
		outputView: NewOutputViewForAgent(agentID, manager, styles),
	}
}

// ID returns the tab identifier
func (at *AgentTab) ID() string {
	return "agent-" + at.agentID
}

// Type returns the tab type
func (at *AgentTab) Type() TabType {
	return TabTypeAgent
}

// Title returns the tab title
func (at *AgentTab) Title() string {
	if agent := at.outputView.manager.GetAgent(at.agentID); agent != nil {
		return fmt.Sprintf("Issue %d", agent.IssueNumber)
	}
	// Fallback to current behavior if agent not found
	return "Agent " + at.agentID
}

// IsClosable returns whether this tab can be closed
func (at *AgentTab) IsClosable() bool {
	return true
}

// View returns the tab's rendered content
func (at *AgentTab) View() string {
	return at.outputView.View()
}

// Update handles messages for the agent tab
func (at *AgentTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	var cmd tea.Cmd
	at.outputView, cmd = at.outputView.Update(msg)
	return at, cmd
}

// Resize updates the tab dimensions
func (at *AgentTab) Resize(width, height int) {
	at.outputView.Resize(width, height)
}

// GetStatus returns the current status of the associated agent
func (at *AgentTab) GetStatus() agent.Status {
	if ag := at.outputView.manager.GetAgent(at.agentID); ag != nil {
		return ag.Status
	}
	return agent.StatusRunning // Default to running if agent not found
}

// CaptureFocusState returns the current focus state for the agent tab
func (at *AgentTab) CaptureFocusState() FocusTarget {
	// Agent tab always uses footer input
	return FocusTargetFooter
}

// RestoreFocusState restores the focus state for the agent tab
func (at *AgentTab) RestoreFocusState(target FocusTarget) tea.Cmd {
	// Agent tab doesn't manage focus directly - handled by parent model
	return nil
}
