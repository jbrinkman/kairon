# Issue #195: Implement ACP-based Planning Tab for Kiro CLI v3.0 Compatibility

**Closes #195**

## Solution Approach

This specification outlines the complete replacement of the current subprocess-based planning system with an ACP (Agent Communication Protocol) based Planning Tab implementation. The solution maintains full backward compatibility while preparing for Kiro CLI v3.0 migration by:

1. **ACP Integration**: Using the official `github.com/coder/acp-go-sdk` package exclusively for all agent communication
2. **Tab-based UI**: Creating dedicated Planning Tabs with chat-like interfaces embedded within the main TUI
3. **Enhanced Footer Design**: Implementing a two-row footer system for better context information display
4. **Lifecycle Management**: Enforcing a maximum of 10 concurrent Planning tabs with proper resource cleanup
5. **Backward Compatibility**: Preserving the existing `plan classic` command for spawnSync-based planning

## Relevant Files

### New Files to Create
- `internal/acp/client.go` - ACP client wrapper using official SDK
- `internal/acp/types.go` - ACP message and response type definitions
- `internal/tui/planning_tab.go` - Planning Tab implementation with chat interface
- `internal/tui/footer.go` - Two-row footer management system
- `internal/tui/context_tracker.go` - Real-time context information tracking
- `go.mod` - Add ACP SDK dependency: `github.com/coder/acp-go-sdk`

### Files to Modify
- `internal/tui/tui.go` - Integrate Planning Tab support and new footer system
- `internal/tui/tab_manager.go` - Add Planning Tab lifecycle management and 10-tab limit
- `internal/tui/commands.go` - Modify `plan` command to create ACP tabs, add `plan classic`
- `internal/tui/styles.go` - Add Planning Tab specific styling
- `internal/session/types.go` - Add ACP session state management
- `internal/session/manager.go` - Support Planning Tab session persistence

### Files for Reference (No Changes)
- `internal/session/planner.go` - Preserved for `plan classic` command
- `.kiro/agents/planner.json` - Agent configuration remains unchanged

## Team Orchestration

The implementation involves three key coordination areas:

1. **ACP Layer Integration**: The ACP client layer communicates directly with Kiro CLI via the official SDK, handling connection management, authentication, and message routing
2. **TUI Layer Enhancement**: The TUI system manages Planning Tabs as first-class tab types alongside Agent tabs, with proper lifecycle and resource management
3. **Session Persistence**: The session management layer handles Planning Tab state persistence, enabling proper resume/restore functionality

Dependencies between components:
- ACP Client → Official SDK (external dependency)
- Planning Tab → ACP Client → TUI Integration
- Footer System → Context Tracker → Tab Manager
- Session Management ← Planning Tab State

## Step-by-Step Task Breakdown

### Task 1: ACP Integration Layer
**Acceptance Criteria**:
- Add `github.com/coder/acp-go-sdk` dependency to go.mod
- Implement `internal/acp/client.go` using official SDK exclusively
- Create ACP message type definitions in `internal/acp/types.go`
- Implement connection management with retry logic via SDK
- Handle authentication using existing Kiro CLI credentials through SDK
- Support streaming and JSON response handling via SDK API
**Dependencies**: None (can run in parallel with Task 2)

### Task 2: Footer System Enhancement
**Acceptance Criteria**:
- Implement two-row footer design in `internal/tui/footer.go`
- Row 1: Command entry area (existing prompt functionality)
- Row 2: Contextual information display (theme, context usage, model, directory)
- Create `internal/tui/context_tracker.go` for real-time context updates
- Integrate footer system into main TUI rendering pipeline
- Ensure responsive behavior for narrow terminal widths
**Dependencies**: None (can run in parallel with Task 1)

### Task 3: Planning Tab Implementation
**Acceptance Criteria**:
- Create `internal/tui/planning_tab.go` with chat-like scrollable interface
- Implement embedded message input area within tab content
- Support streaming response rendering for text content
- Handle structured JSON responses with completion-wait logic
- Display user messages as `[planner] > {message}` format
- Display agent responses without prefix
- Enter key sends messages from embedded input area
- Integration with existing tab navigation system (mouse, `[`, `]`, `f2`)
**Dependencies**: Task 1 (ACP Layer), Task 2 (Footer System)

### Task 4: Tab Manager Enhancement
**Acceptance Criteria**:
- Extend `internal/tui/tab_manager.go` to support Planning Tab type
- Implement maximum 10 concurrent Planning tabs limit enforcement
- Add Planning Tab lifecycle management (create, active, completed, failed)
- Apply existing agent tab color system to Planning tabs
- Success color when GitHub issue created, error color on failure
- Read-only state after completion with forced new tab for subsequent sessions
**Dependencies**: Task 3 (Planning Tab Implementation)

### Task 5: Command System Integration
**Acceptance Criteria**:
- Modify `plan` command in `internal/tui/commands.go` to create ACP-based Planning tabs
- Add `plan classic` command that preserves existing spawnSync behavior
- Ensure existing planner.go functionality remains unchanged
- Implement proper error handling for ACP connection failures
- Add retry mechanisms with maximum retry cap using SDK connection management
**Dependencies**: Task 3 (Planning Tab Implementation), Task 4 (Tab Manager Enhancement)

### Task 6: Session State Management
**Acceptance Criteria**:
- Extend session types in `internal/session/types.go` for ACP session state
- Modify `internal/session/manager.go` to support Planning Tab session persistence
- Enable proper resume/restore functionality for Planning tabs
- Maintain conversation history across tab lifecycle events
- Clean up session state when tabs are closed or completed
**Dependencies**: Task 3 (Planning Tab Implementation)

### Task 7: Styling and UI Polish
**Acceptance Criteria**:
- Add Planning Tab specific styles to `internal/tui/styles.go`
- Ensure theme compatibility across all existing themes
- Implement proper hover and selection states for Planning tabs
- Verify responsive behavior and terminal compatibility
- Test mouse interaction and keyboard navigation
**Dependencies**: Task 3 (Planning Tab Implementation), Task 4 (Tab Manager Enhancement)

### Task 8: Integration and Error Handling
**Acceptance Criteria**:
- Integrate all components in `internal/tui/tui.go`
- Implement graceful degradation when ACP unavailable
- Add comprehensive error handling with inline error messages
- Test tab switching, concurrent planning sessions, and resource cleanup
- Verify backward compatibility with `plan classic` command
- Ensure existing functionality remains unaffected
**Dependencies**: All previous tasks (1-7)

## Validation Commands

```bash
# Build and verify compilation
go build ./cmd/kiro-krew

# Test basic functionality
./kiro-krew help

# Test ACP dependency integration
go mod tidy
go mod verify

# Test planning command structure
echo "plan" | ./kiro-krew
echo "plan classic" | ./kiro-krew

# Test tab limits
for i in {1..12}; do echo "plan Test session $i" & done | ./kiro-krew

# Test footer display
TERM=xterm-256color ./kiro-krew

# Verify session persistence
./kiro-krew
# (create planning tab, exit, restart)
./kiro-krew
# (verify planning session can be resumed)

# Test backward compatibility
echo "plan classic Test classic planning" | ./kiro-krew

# Test error handling
# (disconnect from network, test ACP failures)
```

## Technical Implementation Notes

### ACP SDK Integration
- **Mandatory Requirement**: All ACP communication MUST use `github.com/coder/acp-go-sdk`
- No custom ACP protocol implementation permitted
- SDK handles connection management, authentication, and message routing
- Leverage SDK's retry mechanisms and error handling
- Use existing Kiro CLI authentication without additional setup

### UI Layout Reference
```text
┌─────────────────────────────────────────────────────────────────────────────┐
│ [Console] [Planning 1*] [Planning 2] [Agent #123]                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  [planner] > Create a login feature                                         │
│                                                                             │
│  I'll help you plan a login feature. Let me start by understanding your    │
│  requirements better.                                                       │
│                                                                             │
│  What authentication method should users use to log in?                     │
│  a) JWT tokens with login form                                             │
│  b) OAuth with GitHub/Google                                               │
│  c) API keys for service accounts                                          │
│  d) Other (please specify)                                                 │
│                                                                             │
│  [planner] > █                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ kiro-krew> Type your command here...                                        │
│ theme: dark │ ctx: 45k/200k │ model: claude-sonnet-4 │ 📁 /projects/myapp  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Context Information Display
- **All Tabs**: `theme: {theme}`
- **Planning Tabs Additionally**: `theme: {theme} | ctx: {usage} | model: {model} | 📁 {directory}`
- Context info updates in real-time during ACP conversation
- Responsive design for narrow terminals

### Error Handling Strategy
- ACP connection failures display inline error messages from SDK
- Retry mechanism using SDK connection management
- Graceful degradation when ACP unavailable
- Comprehensive logging for debugging ACP integration issues

### Resource Management
- Maximum 10 concurrent Planning tabs enforced
- Automatic cleanup of completed/failed sessions
- Memory management for chat history and context tracking
- Proper tab lifecycle management with state persistence

This comprehensive implementation will provide a seamless transition to ACP-based planning while maintaining full backward compatibility and preparing for Kiro CLI v3.0 compatibility requirements.