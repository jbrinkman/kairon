# Design Specification: Simplify TUI Agent Tab Names to Show Issue Numbers Clearly

**Issue**: #84  
**Closes**: #84  
**Date**: 2026-06-10  
**Status**: Draft

## Problem Statement

Current TUI tab names display 'Agent agent-80-18752' format which is confusing and redundant. The format includes both "Agent" prefix and complex agent IDs containing timestamps that don't provide meaningful information to users. Users need to quickly identify which issue an agent is working on.

## Solution Approach

Simplify tab names to display 'Issue 80' format using the agent's `IssueNumber` field directly. This provides clear, concise identification of what each agent tab represents while maintaining backward compatibility.

### Design Goals
- **Clarity**: Tab names should immediately show the issue number
- **Conciseness**: Remove redundant "Agent" prefix and timestamp suffixes  
- **Maintainability**: Minimal code changes with clean implementation
- **Backward Compatibility**: No breaking changes to existing functionality

## Current Architecture Analysis

### Key Components

1. **AgentTab** (`internal/tui/agent_tab.go`)
   - Contains `agentID` field (format: "agent-{issueNumber}-{timestamp}")
   - `Title()` method currently returns `"Agent " + at.agentID`
   - Has access to agent manager through `outputView.manager`

2. **Agent Structure** (`internal/agent/manager.go`)
   ```go
   type Agent struct {
       ID          string  // "agent-{issueNumber}-{timestamp}"
       IssueNumber int     // The actual issue number
       IssueTitle  string  // "Issue #{issueNumber}"
       // ... other fields
   }
   ```

3. **Manager** (`internal/agent/manager.go`)
   - Maintains map of agents by ID: `agents map[string]*Agent`
   - No public Get method currently exists
   - `List()` method returns all agents

## Relevant Files

### Files to Modify
- **`internal/tui/agent_tab.go`**: Modify `Title()` method to access and display issue number
- **`internal/agent/manager.go`**: Add `GetAgent(id string)` method for accessing agent by ID

### Files for Reference
- **`internal/tui/tui.go`**: Contains agent tab creation logic at line 797
- **`internal/tui/output_view.go`**: Shows how agent manager is accessed from output view

## Step-by-Step Task Breakdown

### Task 1: Add GetAgent Method to Manager
**File**: `internal/agent/manager.go`
**Acceptance Criteria**:
- [ ] Add `GetAgent(id string) *Agent` method with proper locking
- [ ] Method returns nil for non-existent agent IDs
- [ ] Method is thread-safe using existing RLock pattern

**Implementation Details**:
```go
// GetAgent retrieves an agent by ID
func (m *Manager) GetAgent(id string) *Agent {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    return m.agents[id]
}
```

### Task 2: Update AgentTab Title Method
**File**: `internal/tui/agent_tab.go`
**Acceptance Criteria**:
- [ ] `Title()` method returns "Issue {number}" format
- [ ] Graceful fallback to current behavior if agent not found
- [ ] No changes to other AgentTab functionality

**Implementation Details**:
```go
// Title returns the tab title
func (at *AgentTab) Title() string {
    if agent := at.outputView.manager.GetAgent(at.agentID); agent != nil {
        return fmt.Sprintf("Issue %d", agent.IssueNumber)
    }
    // Fallback to current behavior if agent not found
    return "Agent " + at.agentID
}
```

### Task 3: Add Required Import
**File**: `internal/tui/agent_tab.go`
**Acceptance Criteria**:
- [ ] Add `fmt` import for string formatting
- [ ] Verify no import conflicts exist

## Team Orchestration

This is a minimal, self-contained change affecting only the display layer. No coordination with other teams required.

### Development Sequence
1. **Agent Manager Enhancement** (Task 1): Add GetAgent method
2. **Title Method Update** (Task 2): Modify AgentTab.Title()  
3. **Import Addition** (Task 3): Add fmt import if needed

### Risk Mitigation
- Fallback behavior preserves current tab names if agent lookup fails
- Changes are display-only with no functional impact
- Existing test coverage should continue to pass

## Validation Commands

### Build Verification
```bash
go build ./...
```

### Test Execution
```bash
go test ./internal/tui/...
go test ./internal/agent/...
```

### Manual Testing
1. Start kiro-krew TUI with active agents
2. Verify tab names show "Issue {number}" format
3. Verify tab functionality remains unchanged (switching, closing, etc.)
4. Test with multiple agents to ensure unique issue numbers display correctly

### Integration Testing
```bash
# Run the integration test to verify end-to-end functionality
./test_integration.sh
```

## Implementation Notes

### Backward Compatibility
- Fallback to current behavior ensures no breaking changes
- Agent lookup failure gracefully degrades to original tab naming

### Performance Considerations
- `GetAgent()` method uses read lock for minimal performance impact
- Title generation happens only during UI updates (infrequent)
- No impact on agent execution or output capture

### Edge Cases Handled
- **Agent not found**: Falls back to current "Agent {agentID}" format
- **Concurrent access**: Thread-safe via existing mutex patterns
- **Invalid issue numbers**: Agent.IssueNumber is always valid (set during spawn)

## Success Criteria

- [ ] Tab names display as "Issue {number}" for all active agents
- [ ] No functional regression in tab behavior
- [ ] All existing tests pass
- [ ] Manual testing confirms improved readability
- [ ] Fallback behavior works correctly when agent lookup fails
