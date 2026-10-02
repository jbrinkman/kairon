# TUI Modular Refactoring Design Specification

## Problem Statement

The current TUI implementation in `internal/tui/` has grown complex with business logic mixed into rendering functions. The main `tui.go` file contains 860+ lines with UI rendering, business logic, event handling, and state management all tightly coupled together. This makes the codebase difficult to test, maintain, and extend.

## Solution Approach

Implement a clean separation of concerns using the Model-View-Controller (MVC) pattern adapted for terminal UIs:

1. **State Layer** - Pure data models and state management
2. **Business Logic Layer** - Command handlers and operations 
3. **UI Layer** - Pure rendering and input handling
4. **Event System** - Decoupled message passing between layers

### Architecture Principles

- **Single Responsibility**: Each component handles one specific concern
- **Dependency Inversion**: Business logic doesn't depend on UI details
- **Testability**: Business logic can be tested without TUI dependencies
- **Immutable State**: State changes return new state objects
- **Event-Driven**: Components communicate via well-defined events

## Relevant Files

### Files to Create
- `internal/tui/state/app_state.go` - Central application state
- `internal/tui/state/ui_state.go` - UI-specific state (overlays, inputs, etc.)
- `internal/tui/handlers/command_handler.go` - Business logic for commands
- `internal/tui/handlers/watcher_handler.go` - Watcher operations
- `internal/tui/handlers/agent_handler.go` - Agent management operations
- `internal/tui/events/events.go` - Event definitions
- `internal/tui/ui/renderer.go` - Pure rendering functions
- `internal/tui/ui/input_handler.go` - Input processing
- `internal/tui/controller.go` - Coordinates between layers

### Files to Modify
- `internal/tui/tui.go` - Simplified orchestration
- `internal/tui/commands.go` - Extract business logic to handlers
- `internal/tui/tab_manager.go` - Focus on UI concerns only
- Existing test files - Update for new structure

### Files to Preserve
- `internal/tui/styles.go` - UI styling (no changes needed)
- `internal/tui/about.go` - Self-contained component
- `internal/tui/autocomplete.go` - Input-specific functionality

## Team Orchestration

This refactoring can be implemented incrementally without breaking existing functionality:

1. **Phase 1**: Create state layer and event system
2. **Phase 2**: Extract business logic handlers 
3. **Phase 3**: Create pure rendering layer
4. **Phase 4**: Refactor main TUI orchestration
5. **Phase 5**: Update tests and validation

Each phase maintains backward compatibility while progressively improving the architecture.

## Step-by-Step Task Breakdown

### Task 1: Create State Management Layer
**Acceptance Criteria:**
- `AppState` struct holds all application data (agents, watcher, config, etc.)
- `UIState` struct holds UI-specific state (overlays, tabs, input values)
- State changes return new immutable state objects
- Clear separation between business and UI state

### Task 2: Define Event System
**Acceptance Criteria:**
- Event types for all user actions and system events
- Event dispatching mechanism
- Type-safe event handling

### Task 3: Extract Command Handlers
**Acceptance Criteria:**
- `CommandHandler` processes business commands without UI dependencies
- `WatcherHandler` manages watcher lifecycle
- `AgentHandler` manages agent operations
- All handlers return events, not direct state mutations

### Task 4: Create Pure Rendering Layer
**Acceptance Criteria:**
- `Renderer` takes state and returns rendered views
- No business logic in rendering functions
- Stateless rendering functions
- Consistent styling application

### Task 5: Implement Input Processing
**Acceptance Criteria:**
- `InputHandler` processes key events and returns commands/events
- Separated from business logic
- Handles autocomplete, navigation, and shortcuts

### Task 6: Create Controller Layer
**Acceptance Criteria:**
- `Controller` coordinates between state, handlers, and UI
- Implements the Bubble Tea model interface
- Minimal orchestration logic
- Clear event flow

### Task 7: Refactor Main TUI
**Acceptance Criteria:**
- `tui.go` becomes thin orchestration layer
- Uses controller for all operations
- Maintains existing public API
- All functionality preserved

### Task 8: Update Tests
**Acceptance Criteria:**
- Unit tests for state management
- Unit tests for command handlers
- Integration tests for controller
- Test coverage maintained or improved

## Validation Commands

```bash
# Build and ensure no compilation errors
go build ./cmd/kiro-krew

# Run all tests
go test ./internal/tui/...

# Test specific functionality
go test -run TestCommandHandling ./internal/tui/...
go test -run TestStateManagement ./internal/tui/...
go test -run TestUIRendering ./internal/tui/...

# Integration testing
./test_integration.sh

# Verify existing behavior preserved
go run ./cmd/kiro-krew
# Test all existing commands work: watch start/stop, status, plan, etc.
```

## Implementation Notes

### State Management Pattern
```go
// Immutable state updates
func (s AppState) WithWatcherRunning(running bool) AppState {
    newState := s
    newState.WatcherRunning = running
    return newState
}
```

### Event-Driven Architecture
```go
// Commands return events, not state changes
func (h *CommandHandler) HandleWatch(action string, state AppState) ([]Event, error) {
    switch action {
    case "start":
        return []Event{WatcherStartRequested{}}, nil
    case "stop": 
        return []Event{WatcherStopRequested{}}, nil
    }
}
```

### Testable Business Logic
```go
// Handlers can be tested without TUI
func TestWatcherStart(t *testing.T) {
    handler := NewWatcherHandler(mockWatcher)
    events, err := handler.Start(initialState)
    assert.NoError(t, err)
    assert.Contains(t, events, WatcherStarted{})
}
```

This architecture ensures clear separation of concerns, improved testability, and maintainable code while preserving all existing functionality.
