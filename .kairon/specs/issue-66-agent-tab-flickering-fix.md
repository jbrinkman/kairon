# Design Specification: Fix Agent Tab Log Flickering

**Issue**: #66  
**Closes**: #66  
**Date**: 2026-06-07  
**Status**: Draft

## Problem Statement

Agent tabs in the TUI currently show logs from all agents instead of filtering to their specific agent ID, causing visual flickering between different agents' output. The core issue is in the `refreshContent()` method of `OutputView` which displays all agents indiscriminately.

## Current Architecture Analysis

### Key Components

1. **OutputView** (`internal/tui/output_view.go`)
   - Shared by all tabs (main view and agent tabs)
   - `refreshContent()` method displays all agents from `manager.List()`
   - Uses global output capture via `manager.GetOutputLines()`

2. **AgentTab** (`internal/tui/agent_tab.go`)
   - Contains an `agentID` field but doesn't use it for filtering
   - Creates a standard `OutputView` without agent-specific configuration
   - No isolation between different agent tabs

3. **OutputCapture** (`internal/agent/output_capture.go`)
   - Captures all agent output in a single global buffer
   - Lines prefixed with `[agent issue-{number}] `
   - No per-agent filtering capability

### Root Cause

The `refreshContent()` method in `OutputView` iterates through **all** agents and displays their logs regardless of which specific agent tab is being viewed. Agent tabs share the same `OutputView` logic as the main view.

## Solution Approach

Create agent-specific output views that filter logs by agent ID while maintaining the existing architecture patterns. This approach:

1. **Preserves existing scroll behavior and formatting**
2. **Maintains compatibility with the main "all agents" view**  
3. **Adds minimal new code with focused changes**
4. **Follows existing patterns in the codebase**

### Core Strategy

- Extend `OutputView` to support optional agent filtering
- Modify `AgentTab` to pass its agent ID to the output view
- Filter captured lines by agent prefix during rendering
- Preserve all existing UI behaviors (scrolling, formatting, etc.)

## Relevant Files

### Files to Modify

1. **`internal/tui/output_view.go`**
   - Add optional `filterAgentID` field to `OutputView` struct
   - Modify `refreshContent()` to filter by agent ID when specified
   - Add `NewAgentOutputView()` constructor for agent-specific views

2. **`internal/tui/agent_tab.go`**
   - Update `NewAgentTab()` to use agent-specific output view
   - Pass agent ID to output view for filtering

### Files Referenced (No Changes)

- `internal/tui/tab_manager.go` - Tab management logic (unchanged)
- `internal/agent/manager.go` - Agent management (unchanged)  
- `internal/agent/output_capture.go` - Output capture (unchanged)

## Team Orchestration

This is a focused TUI fix that can be implemented by a single developer without coordination requirements. The changes are:

- **Self-contained** within the TUI package
- **Backwards compatible** with existing functionality
- **Non-breaking** to other components

No coordination needed with:
- Agent spawning/management logic
- Output capture mechanisms  
- GitHub integration
- Configuration systems

## Step-by-Step Task Breakdown

### Task 1: Extend OutputView for Agent Filtering
**File**: `internal/tui/output_view.go`
**Acceptance Criteria**:
- [ ] Add `filterAgentID *string` field to `OutputView` struct
- [ ] Create `NewAgentOutputView(manager, styles, agentID)` constructor
- [ ] Modify `refreshContent()` to filter by agent ID when `filterAgentID` is set
- [ ] Preserve all existing behavior when `filterAgentID` is nil (main view)
- [ ] Filter shows only logs with matching `[agent issue-{issueNumber}]` prefix

**Implementation Details**:
```go
type OutputView struct {
    // existing fields...
    filterAgentID *string  // nil = show all agents, non-nil = filter by agent ID
}

func NewAgentOutputView(manager *agent.Manager, styles *Styles, agentID string) *OutputView {
    ov := NewOutputView(manager, styles)
    ov.filterAgentID = &agentID
    return ov
}
```

### Task 2: Update refreshContent() Logic  
**File**: `internal/tui/output_view.go`
**Acceptance Criteria**:
- [ ] When `filterAgentID` is set, show only matching agent's logs
- [ ] When `filterAgentID` is nil, preserve existing "show all agents" behavior
- [ ] Maintain existing formatting, headers, and separators
- [ ] Preserve scroll position behavior
- [ ] Handle case where filtered agent doesn't exist gracefully

**Implementation Strategy**:
- Add agent filtering logic before the main display loop
- Filter `agents` list to single matching agent when `filterAgentID` is set
- Reuse existing output formatting logic unchanged

### Task 3: Update AgentTab Constructor
**File**: `internal/tui/agent_tab.go`  
**Acceptance Criteria**:
- [ ] Modify `NewAgentTab()` to use `NewAgentOutputView()` instead of `NewOutputView()`
- [ ] Pass stored `agentID` to the output view constructor
- [ ] Maintain all existing AgentTab interface methods unchanged
- [ ] Preserve existing tab behavior (scrolling, resizing, etc.)

**Implementation**:
```go
func NewAgentTab(agentID string, manager *agent.Manager, styles *Styles) *AgentTab {
    return &AgentTab{
        agentID:    agentID,
        outputView: NewAgentOutputView(manager, styles, agentID),
    }
}
```

### Task 4: Verification and Testing
**Acceptance Criteria**:
- [ ] Start 2+ agents processing different issues
- [ ] Open agent tabs for different agents  
- [ ] Verify each tab shows only its specific agent's logs
- [ ] Verify main tab still shows all agents
- [ ] Verify no flickering occurs when switching between agent tabs
- [ ] Verify scroll positions are maintained independently per tab
- [ ] Test agent tab behavior when agent completes/fails

## Validation Commands

```bash
# Terminal 1: Start kiro-krew in TUI mode
kiro-krew tui

# Terminal 2: Trigger multiple agent spawns
kiro-krew watch start
# Wait for multiple issues to be processed simultaneously

# Manual validation steps in TUI:
# 1. Press Tab to cycle through agent tabs
# 2. Verify each agent tab shows only its specific logs
# 3. Verify main tab (first tab) shows all agents
# 4. Scroll within agent tabs - verify independent scroll positions
# 5. Let agents complete - verify tab content remains stable
```

**Expected Results**:
- Agent tab titles show "Agent {agentID}"
- Each agent tab displays only logs from that specific agent
- Main tab continues to show aggregated view of all agents
- No cross-contamination or flickering between agent views
- Scroll positions remain independent between tabs

**Test Scenarios**:
1. **2 agents running**: Tabs should show distinct, non-flickering content
2. **Agent completion**: Completed agent tab should show final state, no flickering
3. **Agent failure**: Failed agent tab should show error state, remain stable
4. **Tab switching**: Rapid tab switching should not cause visual flicker
5. **Scrolling**: Each tab maintains its own scroll position independently

## Technical Notes

### Key Implementation Principles

1. **Minimal Code Changes**: Extend existing `OutputView` rather than create entirely new components
2. **Backwards Compatibility**: Main view (filterAgentID = nil) preserves exact existing behavior  
3. **Consistent Patterns**: Follow existing constructor and filtering patterns in codebase
4. **Performance**: Filtering happens during render, not during capture (preserves capture performance)

### Edge Cases Handled

- **Agent doesn't exist**: Show "Agent not found" message instead of empty/error state
- **No logs yet**: Show existing "No captured output yet" message  
- **Agent ID format**: Match existing agent ID patterns used throughout codebase
- **Dynamic agent creation**: New agents automatically appear in main view, can get dedicated tabs

### Architecture Benefits

- **Isolated Views**: Each agent tab operates independently
- **Preserved Main View**: All-agents overview remains unchanged
- **Scalable**: Pattern works for any number of concurrent agents
- **Maintainable**: Changes are localized to TUI display logic only
