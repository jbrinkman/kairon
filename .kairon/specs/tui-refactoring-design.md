# TUI Refactoring Design Specification

## Problem Statement

The current TUI code has grown complex with business logic mixed into rendering functions. The main issues identified:

1. **Large monolithic model**: The `model` struct in `tui.go` (600+ lines) contains too many responsibilities
2. **Mixed concerns**: Business logic for command handling, state management, and UI rendering are intertwined
3. **Testability**: Hard to unit test individual components due to tight coupling
4. **State management**: Complex state synchronization between different views and modes
5. **Command handling**: Business logic embedded directly in UI update methods

## Solution Approach

Implement a **Model-View-Controller (MVC) architecture** with clear separation of concerns:

- **Model Layer**: Pure business logic for state management and command execution
- **View Layer**: UI rendering and layout logic only  
- **Controller Layer**: Event routing and coordination between model and view

### Key Architectural Changes

1. **Extract Business Logic**: Move all command handling and state management to dedicated services
2. **Component-based UI**: Break down monolithic model into focused, composable UI components
3. **Event-driven Architecture**: Use message passing for loose coupling between components
4. **State Management Service**: Centralized state coordination with clear interfaces

## Relevant Files

### Files to Create
- `internal/tui/controller/` - Command controllers and event handlers
- `internal/tui/model/` - Business logic and state management  
- `internal/tui/components/` - Reusable UI components
- `internal/tui/services/` - Application services (command execution, state sync)

### Files to Modify
- `internal/tui/tui.go` - Simplified main model focusing on UI coordination
- `internal/tui/commands.go` - Extract business logic to controller layer
- `internal/tui/tab_manager.go` - Convert to pure UI component
- `internal/tui/output_view.go` - Simplify to focus on rendering
- `internal/tui/autocomplete.go` - Extract validation logic

### Files to Keep
- `internal/tui/styles.go` - UI styling (already separated)
- `internal/tui/*_test.go` - Update tests to use new architecture

## Team Orchestration

### Phase 1: Foundation (Service Layer)
- Create service interfaces and base implementations
- Extract command execution logic from UI layer
- Set up event system for component communication

### Phase 2: Model Extraction  
- Move state management logic to dedicated model layer
- Create state synchronization services
- Implement business logic separation

### Phase 3: UI Component Refactoring
- Break down monolithic model into focused components  
- Implement component-based rendering
- Create reusable UI primitives

### Phase 4: Integration & Testing
- Wire up new architecture
- Migrate existing functionality
- Add comprehensive unit tests for separated components

## Step-by-Step Task Breakdown

### Task 1: Create Service Layer Architecture
**Acceptance Criteria:**
- [ ] Create `internal/tui/services/command_service.go` with interface for command execution
- [ ] Create `internal/tui/services/state_service.go` with centralized state management
- [ ] Create `internal/tui/services/event_service.go` for component communication
- [ ] All services use dependency injection and are independently testable

### Task 2: Extract Command Controllers  
**Acceptance Criteria:**
- [ ] Create `internal/tui/controller/watch_controller.go` - handles watch start/stop logic
- [ ] Create `internal/tui/controller/status_controller.go` - handles status display logic
- [ ] Create `internal/tui/controller/agent_controller.go` - handles agent management
- [ ] Create `internal/tui/controller/planning_controller.go` - handles planning mode
- [ ] Controllers contain only business logic, no UI rendering

### Task 3: Create Application State Model
**Acceptance Criteria:**
- [ ] Create `internal/tui/model/app_state.go` - centralized application state
- [ ] Create `internal/tui/model/view_state.go` - UI-specific state (current tab, overlays)
- [ ] Create `internal/tui/model/session_state.go` - session and mode management
- [ ] State changes trigger events, no direct UI manipulation

### Task 4: Component-based UI Architecture
**Acceptance Criteria:**
- [ ] Create `internal/tui/components/console.go` - main console component
- [ ] Create `internal/tui/components/overlay.go` - reusable overlay component  
- [ ] Create `internal/tui/components/tab_bar.go` - tab header rendering
- [ ] Create `internal/tui/components/input.go` - input handling component
- [ ] Each component handles only rendering and local UI state

### Task 5: Refactor Main TUI Model
**Acceptance Criteria:**  
- [ ] Reduce `tui.go` to ~200 lines focused on coordination
- [ ] Replace direct command handling with controller delegation
- [ ] Use component composition for rendering
- [ ] Remove all business logic from Update() method

### Task 6: Event System Integration
**Acceptance Criteria:**
- [ ] Implement event-driven communication between components
- [ ] Remove direct dependencies between UI components
- [ ] Use message passing for state synchronization
- [ ] Add event debugging and tracing capabilities

### Task 7: Extract and Enhance Testing
**Acceptance Criteria:**
- [ ] Create unit tests for all service layer components
- [ ] Create unit tests for controllers with mocked dependencies
- [ ] Create integration tests for component interactions
- [ ] Achieve >80% test coverage for new architecture

### Task 8: Migration and Validation  
**Acceptance Criteria:**
- [ ] All existing functionality works identically
- [ ] No regression in performance or user experience
- [ ] Clean up old code and update documentation
- [ ] Validate improved testability with new test suite

## Validation Commands

```bash
# Build and verify compilation
go build ./cmd/kiro-krew

# Run comprehensive test suite
go test ./internal/tui/... -v -race -coverprofile=coverage.out

# Check test coverage 
go tool cover -html=coverage.out

# Integration test - start TUI and verify all commands work
./kiro-krew
# In TUI: test watch start/stop, status, plan, help, theme, logs, exit

# Performance validation - should start within same time bounds
time ./kiro-krew --version

# Verify no memory leaks in long-running operations
go test -memprofile=mem.prof ./internal/tui/...
go tool pprof mem.prof
```

## Implementation Notes

### Key Design Patterns

1. **Dependency Injection**: All services receive dependencies via constructor injection
2. **Event Sourcing**: State changes generate events that components can subscribe to  
3. **Command Pattern**: Each user action becomes a command object with execute() method
4. **Component Pattern**: UI elements are self-contained with clear interfaces

### Backwards Compatibility

- Maintain identical user-facing behavior during refactoring
- Preserve all existing keyboard shortcuts and commands
- Keep configuration file format unchanged
- Ensure no breaking changes to integration points

### Error Handling Strategy

- Services return structured errors with context
- UI components handle errors gracefully with user feedback
- Failed operations don't crash the application
- Add proper error logging for debugging

### Testing Strategy

- **Unit Tests**: Test business logic in isolation with mocks
- **Component Tests**: Test UI components with simulated events  
- **Integration Tests**: Test full workflows end-to-end
- **Property Tests**: Use randomized testing for state transitions

This refactoring will significantly improve code maintainability, testability, and extensibility while preserving all existing functionality.
