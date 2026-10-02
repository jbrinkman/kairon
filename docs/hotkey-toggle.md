# Hotkey Toggle Feature

The hotkey toggle feature allows you to quickly switch between console mode and planning mode using a keyboard shortcut while working in Kairon.

## Quick Start

Press **Ctrl+Alt+P** (or **Ctrl+Option+P** on macOS) to toggle between modes:

- **Console Mode** → **Planning Mode**: Switch from the main Kairon interface to interactive planning
- **Planning Mode** → **Console Mode**: Return from planning session back to the main interface

## How It Works

### Console Mode
The default mode where you can:
- Start/stop the watcher
- View agent status
- Manage running agents
- Execute other Kairon commands

### Planning Mode
Interactive planning session where you can:
- Create GitHub issues with AI assistance
- Refine requirements and acceptance criteria
- Structure project specifications
- Optionally apply labels for immediate automation

### Mode Switching
The hotkey provides seamless switching between these modes:

1. **From Console**: Press `Ctrl+Alt+P` to enter planning mode
2. **From Planning**: Press `Ctrl+Alt+P` to return to console mode
3. **Session Preservation**: Both modes maintain their state when you switch

## Context Requirements

The hotkey toggle only works when:
- Running inside a Kairon terminal session
- The `KAIRON_WATCHER_PID` environment variable is set
- You're in an interactive TUI session

If used outside this context, you'll receive an error message: "hotkey toggle not available outside kairon context"

## Usage Examples

### Starting a Planning Session
```
kairon> status
No agents running

# Press Ctrl+Alt+P to switch to planning mode
# Planning session starts with banner and AI assistant
```

### Returning to Console
```
# While in planning mode, press Ctrl+Alt+P
# Returns to console with previous state preserved

kairon> watch start
Watcher started
```

### Alternative: Command-Based Planning
You can also access planning mode via command:
```
kairon> plan Create user authentication system
```

## Session Management

### State Preservation
- **Console Session**: Command history, activity log, and current status are preserved
- **Planning Session**: Conversation history and context are maintained across switches
- **Automatic Resume**: Existing planning sessions are automatically resumed when switching modes

### Session Cleanup
- Sessions are automatically cleaned up on application exit
- Orphaned sessions from crashed processes are detected and cleaned up
- Session data is stored in `.kairon/sessions/`

## Technical Details

### Hotkey Detection
- Cross-platform keyboard shortcut detection
- Integrated with Bubble Tea event system
- Validates execution context before processing

### Integration Points
- TUI message handling for mode switches
- Session manager integration for state preservation
- Error handling for invalid contexts
- Process lifecycle management

For more technical details, see the integration test documentation in `internal/hotkey/INTEGRATION_TESTS.md`.