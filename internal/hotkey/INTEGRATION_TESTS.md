# Hotkey Toggle Integration Tests

## Overview

This document describes the comprehensive integration tests created for Phase 6 of the hotkey toggle implementation. These tests validate the complete end-to-end functionality of the hotkey toggle system.

## Test Coverage

### 1. Basic Hotkey Functionality (`TestHotkeyIntegrationEndToEnd`)

**What it tests:**
- Console to Planning mode switching via hotkey trigger
- Planning to Console mode switching via hotkey trigger  
- Error handling when hotkey is used outside kairon context

**Scenarios covered:**
- ✅ Starting in console mode and switching to planning
- ✅ Switching back from planning to console mode
- ✅ Proper error messaging when used outside kairon context

### 2. Session State Preservation (`TestSessionStatePreservation`)

**What it tests:**
- Session data persistence across mode switches
- Console session state preservation during planning mode
- Planning session state preservation during console mode

**Scenarios covered:**
- ✅ Console session history preserved after mode switch
- ✅ Planning session history preserved after mode switch
- ✅ Session data integrity maintained across switches

### 3. Process Management and Cleanup (`TestProcessManagementAndCleanup`)

**What it tests:**
- Session cleanup on application exit
- Orphaned session cleanup functionality
- Proper session lifecycle management

**Scenarios covered:**
- ✅ Clean exit without data loss
- ✅ Cleanup of old/orphaned sessions
- ✅ Session file management and corruption recovery

### 4. Error Handling and Validation (`TestHotkeyValidation`)

**What it tests:**
- Context validation for hotkey functionality
- Proper error responses for invalid contexts
- Environmental requirement validation

**Scenarios covered:**
- ✅ Valid context detection (`KAIRON_WATCHER_PID` set)
- ✅ Invalid context error handling (no environment variable)
- ✅ Appropriate error messages for different failure modes

### 5. Complete End-to-End Workflow (`TestCompleteWorkflow`)

**What it tests:**
- Full workflow from start to finish
- Integration of all components working together
- Real-world usage simulation

**Scenarios covered:**
- ✅ Start in console mode with populated session
- ✅ Switch to planning mode and populate planning session
- ✅ Switch back to console mode
- ✅ Verify both sessions maintained their state
- ✅ Test error conditions in complete workflow

## Test Architecture

### Mock TUI Model
The tests use a `mockTUIModel` that simulates the actual TUI behavior:

```go
type mockTUIModel struct {
    currentMode       session.SessionType
    sessionManager    *session.SessionManager
    hotkeyTriggered   bool
    modeSwitch        bool
    errorReceived     error
    processRunning    bool
    sessionPreserved  bool
    consoleState      map[string]interface{}
    planningState     map[string]interface{}
}
```

### Test Environment Setup
- Temporary directories for session storage
- Environment variable manipulation for context testing
- Session manager initialization and cleanup
- State verification mechanisms

## Integration Points Tested

### 1. Hotkey Detection → TUI Integration
- Hotkey detection triggers proper TUI messages
- TUI correctly processes `HotkeyTriggeredMsg` and `HotkeyErrorMsg`
- Mode switching occurs as expected

### 2. Session Management Integration
- Session creation, loading, and saving
- Session state preservation across mode switches
- Session cleanup and lifecycle management

### 3. Error Handling Integration
- Context validation integration
- Error propagation through the system
- User-friendly error messaging

### 4. Process Lifecycle Integration
- Startup and shutdown procedures
- Resource cleanup on exit
- Graceful handling of interruptions

## Usage

### Running All Integration Tests
```bash
cd internal/hotkey
go test -v
```

### Running Specific Test Suites
```bash
# Basic functionality
go test -run TestHotkeyIntegrationEndToEnd -v

# Session preservation
go test -run TestSessionStatePreservation -v

# Process management
go test -run TestProcessManagementAndCleanup -v

# Error handling
go test -run TestHotkeyValidation -v

# Complete workflow
go test -run TestCompleteWorkflow -v
```

### Running Integration Demonstration
```bash
./test_integration.sh
```

## Test Results

All integration tests pass, confirming:

- ✅ Hotkey detection and processing works correctly
- ✅ Mode switching (Console ↔ Planning) functions properly  
- ✅ Session state is preserved across mode switches
- ✅ Process management and cleanup work as expected
- ✅ Error handling provides appropriate feedback
- ✅ Complete end-to-end workflow operates correctly
- ✅ Cross-component integration is seamless
- ✅ Project builds successfully with all integration points

## Acceptance Criteria Met

✅ **Test hotkey functionality across different scenarios**
- Console to planning mode switching
- Planning to console mode switching
- Error scenarios (outside kairon context)

✅ **Validate session state preservation**
- Console session state maintained during planning mode
- Planning session state maintained during console mode
- Session data integrity across multiple switches

✅ **Test process management and cleanup**
- Session lifecycle management
- Cleanup on exit
- Orphaned session handling

✅ **Acceptance: Full hotkey toggle workflow works end-to-end**
- Complete workflow from console → planning → console
- All integration points working together
- Real-world usage scenarios validated

The hotkey toggle system is fully tested and ready for production use.