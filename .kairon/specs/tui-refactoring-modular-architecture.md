# TUI Refactoring: Modular Architecture Design

## Problem Statement

The current TUI code in `internal/tui/` has grown complex with business logic mixed into rendering functions. The main `tui.go` file is over 650 lines and contains:

- **Mixed Concerns**: Business logic intertwined with UI rendering in the main model
- **Monolithic Structure**: Single large model handling multiple responsibilities
- **Testing Challenges**: Business logic coupled to UI makes unit testing difficult
- **State Management**: Complex state transitions scattered throughout the main update loop
- **Command Processing**: Command logic embedded in the main TUI model

## Solution Approach

Implement a **Model-View-Controller (MVC)** architecture with clear separation of concerns:

### Core Architectural Principles

1. **Separation of Concerns**: Isolate business logic from presentation logic
2. **Single Responsibility**: Each component handles one primary concern
3. **Dependency Injection**: Controllers receive dependencies via constructors
4. **Event-Driven Architecture**: Use message passing for component communication
5. **Immutable State**: State changes flow through well-defined channels

### High-Level Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│     View        │    │   Controller     │    │     Model       │
│  (UI Rendering) │◄──►│ (Business Logic) │◄──►│ (State & Data)  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
    ┌─────────┐              ┌─────────┐           ┌─────────────┐
    │ Styles  │              │Commands │           │   Events    │
    │Renderer │              │Registry │           │   System    │
    └─────────┘              └─────────┘           └─────────────┘
```

## Relevant Files

### Files to Create

- `internal/tui/mvc/controller.go` - Main TUI controller
- `internal/tui/mvc/model.go` - Application state model  
- `internal/tui/mvc/events.go` - Event system definitions
- `internal/tui/mvc/view.go` - View renderer
- `internal/tui/controllers/command_controller.go` - Command processing
- `internal/tui/controllers/agent_controller.go` - Agent lifecycle management
- `internal/tui/controllers/session_controller.go` - Session management
- `internal/tui/models/app_state.go` - Application state
- `internal/tui/models/ui_state.go` - UI-specific state
- `internal/tui/views/console_view.go` - Console view renderer
- `internal/tui/views/overlay_view.go` - Overlay rendering
- `internal/tui/views/tab_view.go` - Tab system renderer

### Files to Modify

- `internal/tui/tui.go` - Refactor to use MVC architecture
- `internal/tui/commands.go` - Extract command logic to controllers
- `internal/tui/tab_manager.go` - Simplify to focus on tab state only
- `internal/tui/main_tab.go` - Adapt to new architecture
- `internal/tui/agent_tab.go` - Adapt to new architecture

### Files to Keep (Minimal Changes)

- `internal/tui/styles.go` - Already well-separated
- `internal/tui/autocomplete.go` - Self-contained component
- `internal/tui/about.go` - Self-contained component
- `internal/tui/output_view.go` - Already focused on view concerns

## Team Orchestration

This refactoring requires careful coordination to maintain existing functionality:

1. **Phase 1 - Foundation**: Create MVC foundation and event system
2. **Phase 2 - Migration**: Gradually move business logic to controllers
3. **Phase 3 - Views**: Refactor rendering logic to dedicated view components
4. **Phase 4 - Integration**: Wire up new architecture and remove old code
5. **Phase 5 - Testing**: Ensure all functionality works and add tests

## Step-by-Step Task Breakdown

### Task 1: Create MVC Foundation
**Acceptance Criteria:**
- [ ] Create `internal/tui/mvc/` directory structure
- [ ] Implement base `Controller`, `Model`, and `View` interfaces
- [ ] Create event system for component communication
- [ ] Add basic dependency injection container

**Files Created:**
- `internal/tui/mvc/interfaces.go` - Core MVC interfaces
- `internal/tui/mvc/events.go` - Event system
- `internal/tui/mvc/container.go` - Dependency injection

### Task 2: Create Application State Models
**Acceptance Criteria:**
- [ ] Extract all state from main TUI model into dedicated state structs
- [ ] Create immutable state update patterns
- [ ] Implement state validation and transitions
- [ ] Add state serialization for debugging

**Files Created:**
- `internal/tui/models/app_state.go` - Core application state
- `internal/tui/models/ui_state.go` - UI-specific state
- `internal/tui/models/agent_state.go` - Agent-related state

### Task 3: Implement Command Controller
**Acceptance Criteria:**
- [ ] Extract all command handling logic from `commands.go`
- [ ] Implement command validation and execution
- [ ] Add command history and undo capabilities
- [ ] Create command middleware system

**Files Created:**
- `internal/tui/controllers/command_controller.go`
- `internal/tui/controllers/middleware/` - Command middleware

### Task 4: Implement Agent Controller
**Acceptance Criteria:**
- [ ] Extract agent lifecycle management
- [ ] Implement agent state synchronization
- [ ] Add agent output capture coordination
- [ ] Create agent event broadcasting

**Files Created:**
- `internal/tui/controllers/agent_controller.go`

### Task 5: Implement Session Controller  
**Acceptance Criteria:**
- [ ] Extract session management logic
- [ ] Implement session state persistence
- [ ] Add session mode switching coordination
- [ ] Create session cleanup automation

**Files Created:**
- `internal/tui/controllers/session_controller.go`

### Task 6: Create View Renderers
**Acceptance Criteria:**
- [ ] Extract all rendering logic from main TUI
- [ ] Implement composable view system
- [ ] Add view state management
- [ ] Create consistent styling application

**Files Created:**
- `internal/tui/views/console_view.go` - Console rendering
- `internal/tui/views/overlay_view.go` - Overlay system
- `internal/tui/views/tab_view.go` - Tab headers and management
- `internal/tui/views/status_view.go` - Status overlay

### Task 7: Refactor Main TUI Model
**Acceptance Criteria:**
- [ ] Replace monolithic model with MVC coordinator
- [ ] Migrate all business logic to appropriate controllers
- [ ] Simplify update loop to message routing
- [ ] Maintain exact same external behavior

**Files Modified:**
- `internal/tui/tui.go` - Main coordinator
- `internal/tui/tab_manager.go` - Simplify to state only

### Task 8: Update Tab System
**Acceptance Criteria:**
- [ ] Adapt tab system to work with new architecture
- [ ] Separate tab rendering from tab state
- [ ] Implement tab controller for lifecycle management
- [ ] Maintain all existing tab functionality

**Files Modified:**
- `internal/tui/main_tab.go`
- `internal/tui/agent_tab.go`
- `internal/tui/tabs.go`

### Task 9: Integration Testing
**Acceptance Criteria:**
- [ ] All existing TUI functionality works identically
- [ ] No visual changes to user interface
- [ ] All hotkeys and commands work as before
- [ ] Performance is equivalent or better

### Task 10: Add Controller Unit Tests
**Acceptance Criteria:**
- [ ] Create comprehensive controller unit tests
- [ ] Add state model unit tests
- [ ] Test event system isolation
- [ ] Achieve >80% test coverage for new components

**Files Created:**
- `internal/tui/controllers/*_test.go`
- `internal/tui/models/*_test.go`
- `internal/tui/mvc/*_test.go`

## Implementation Details

### Event System Design

```go
type EventBus interface {
    Subscribe(eventType string, handler EventHandler) error
    Publish(event Event) error
    Unsubscribe(eventType string, handler EventHandler) error
}

type Event struct {
    Type      string
    Payload   interface{}
    Timestamp time.Time
}
```

### Controller Interface

```go
type Controller interface {
    Initialize(ctx context.Context, deps *Dependencies) error
    HandleMessage(msg tea.Msg) ([]Event, error)
    Shutdown() error
}
```

### State Management Pattern

```go
type StateManager interface {
    GetState() AppState
    UpdateState(updater func(AppState) AppState) error
    Subscribe(listener StateListener) error
}
```

### View Renderer Interface

```go
type ViewRenderer interface {
    Render(state AppState, styles *Styles) (string, error)
    GetSize() (width int, height int)
    SetSize(width, height int)
}
```

## Validation Commands

### Functional Testing
```bash
# Test all existing functionality
go test ./internal/tui/... -v

# Run integration tests
go test ./internal/tui/integration_test.go -v

# Test TUI with real watcher
./kiro-krew
```

### Performance Testing
```bash
# Benchmark rendering performance
go test -bench=BenchmarkRender ./internal/tui/...

# Memory usage analysis
go test -memprofile=mem.prof ./internal/tui/...
go tool pprof mem.prof
```

### Architectural Validation
```bash
# Check cyclic dependencies
go mod graph | grep "internal/tui"

# Validate interfaces
go vet ./internal/tui/...

# Code complexity analysis
gocyclo -avg ./internal/tui/
```

## Risk Mitigation

### Backward Compatibility
- Maintain exact same external API
- All existing commands and hotkeys work identically  
- No changes to configuration or external dependencies

### Incremental Migration
- Implement new architecture alongside existing code
- Gradual migration with feature flags if needed
- Rollback capability at each phase

### Testing Strategy
- Comprehensive integration tests before refactoring
- Unit tests for all new components
- End-to-end testing with real scenarios

## Success Criteria

1. **Maintainability**: Business logic clearly separated from UI concerns
2. **Testability**: Controllers and models can be unit tested in isolation
3. **Modularity**: Components have single responsibilities and clear interfaces
4. **Performance**: No degradation in rendering or response times
5. **Functionality**: All existing features work exactly as before

## Additional Benefits

- **Easier Feature Development**: New features can be added to specific controllers
- **Better Error Handling**: Centralized error handling in the event system
- **Improved Debugging**: Clear component boundaries make issues easier to isolate
- **Future Extensibility**: Plugin system can be added via controller interfaces

---

**Closes #TUI-Refactoring-Request**
