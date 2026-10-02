# Design Specification: Replace Alphanumeric Shortcut Keys

**Closes #62**

## Problem Statement

The TUI currently uses alphanumeric keys as shortcuts, specifically the 'o' key for tab toggling. This creates conflicts with normal REPL usage where users need to type alphanumeric characters for commands and input without triggering unintended UI actions.

## Current State Analysis

### Affected Files
- `internal/tui/tui.go`: Contains the main keyboard event handling logic
- `internal/tui/output_view.go`: Contains agent output view with vim-style navigation shortcuts

### Current Conflicting Shortcuts
1. **Tab Toggle**: `"f2", "o"` - toggles between main and agent tabs
2. **Agent Output Navigation**: `"k", "g", "G", "j"` - vim-style scrolling in agent output views

### Current Implementation
```go
// In tui.go line ~348
case "f2", "o":
    // Toggle between main and agent tabs
    m.tabManager.ToggleView()
    return m, nil

// In output_view.go
case "up", "k":     // Scroll up
case "down", "j":   // Scroll down  
case "home", "g":   // Go to top
case "end", "G":    // Go to bottom
```

## Solution Approach

### Strategy
Replace all alphanumeric keyboard shortcuts with non-conflicting alternatives that preserve functionality while allowing normal typing in the REPL.

### Design Principles
1. **Non-interference**: No alphanumeric keys should trigger shortcuts
2. **Intuitive**: New shortcuts should be easy to remember and commonly used
3. **Terminal-safe**: Avoid sequences that conflict with common terminal control codes
4. **Accessibility**: Maintain keyboard-only navigation capabilities

### Proposed Shortcut Replacements

#### Primary Tab Navigation
- **Current**: `"f2", "o"` → **New**: `"f2", "tab"`
- **Rationale**: F2 remains unchanged for consistency; Tab key is intuitive for tab switching and commonly used in other applications

#### Secondary Tab Navigation (retain existing)
- **Previous Tab**: `"["` (unchanged)
- **Next Tab**: `"]"` (unchanged) 
- **Close Tab**: `"ctrl+w"` (unchanged)

#### Agent Output View Navigation
Replace vim-style keys with arrow-based alternatives:
- **Scroll Up**: `"up"` only (remove `"k"`)
- **Scroll Down**: `"down"` only (remove `"j"`)
- **Go to Top**: `"home"` only (remove `"g"`)
- **Go to Bottom**: `"end"` only (remove `"G"`)

## Relevant Files

### Files to Modify
1. **`internal/tui/tui.go`**
   - Update keyboard event handling for tab toggle
   - Line ~348: Replace `case "f2", "o":` with `case "f2", "tab":`

2. **`internal/tui/output_view.go`**
   - Remove vim-style navigation shortcuts
   - Keep only non-alphabetic alternatives for scrolling

### Files for Reference
- `internal/tui/tab_manager.go`: Contains `ToggleView()` method (no changes needed)
- `internal/tui/integration_test.go`: May need test updates
- `internal/tui/tab_manager_test.go`: May need test updates

## Team Orchestration

### Implementation Sequence
This is a single-developer task with minimal coordination needed:

1. **Code Changes**: Update keyboard event handling
2. **Testing**: Verify functionality with manual testing
3. **Documentation**: Update any help text or documentation

### Dependencies
- No external dependencies
- No API changes
- No database migrations needed

## Step-by-Step Task Breakdown

### Task 1: Update Main Tab Toggle Shortcut
**Acceptance Criteria:**
- [ ] Remove 'o' key from tab toggle case statement
- [ ] Add 'tab' key to existing F2 case
- [ ] Verify F2 continues to work
- [ ] Verify Tab key now toggles tabs
- [ ] Verify 'o' can be typed in REPL without triggering tab switch

**Implementation:**
```go
// Change in internal/tui/tui.go around line 348
// FROM:
case "f2", "o":
// TO:  
case "f2", "tab":
```

### Task 2: Update Agent Output View Navigation
**Acceptance Criteria:**
- [ ] Remove 'k', 'j', 'g', 'G' from keyboard shortcuts
- [ ] Retain arrow keys and page navigation
- [ ] Verify all letters can be typed without triggering scrolling
- [ ] Verify arrow keys still work for navigation

**Implementation:**
```go
// Changes in internal/tui/output_view.go
// FROM:
case "up", "k":
case "down", "j": 
case "home", "g":
case "end", "G":
// TO:
case "up":
case "down":
case "home":
case "end":
```

### Task 3: Verify No Other Alphanumeric Shortcuts
**Acceptance Criteria:**
- [ ] Scan codebase for any other alphanumeric shortcuts
- [ ] Document any found shortcuts that need attention
- [ ] Ensure comprehensive coverage of the issue

### Task 4: Update Help Documentation
**Acceptance Criteria:**
- [ ] Update any help text mentioning old shortcuts
- [ ] Verify help command shows correct shortcuts
- [ ] Update any inline documentation or comments

## Validation Commands

### Manual Testing Sequence
```bash
# 1. Build and run the application
go build -o kiro-krew ./cmd/kiro-krew
./kiro-krew chat

# 2. Test tab functionality
# - Press F2 → should toggle tabs
# - Press Tab → should toggle tabs  
# - Type 'o' in REPL → should not trigger tab toggle
# - Type 'status' → should execute command normally

# 3. Test agent output navigation (if agents are running)
# - Use arrow keys to scroll in agent tab
# - Type 'kjgG' → should not trigger scrolling
# - Verify normal typing works in all contexts

# 4. Test all other shortcuts remain functional
# - [ and ] for tab navigation
# - Ctrl+W for closing tabs
# - All other existing functionality
```

### Automated Testing
```bash
# Run existing tests to ensure no regressions
go test ./internal/tui/...

# Specific tests to verify:
# - Tab switching functionality
# - Keyboard input handling
# - REPL input processing
```

### Regression Testing Checklist
- [ ] All existing functionality works unchanged
- [ ] No new conflicts introduced
- [ ] REPL accepts all alphanumeric input
- [ ] All non-alphanumeric shortcuts remain functional
- [ ] Tab navigation works with both F2 and Tab
- [ ] Agent output navigation works with arrow keys
- [ ] Help text reflects new shortcuts

## Risk Assessment

### Low Risk Changes
- Keyboard shortcut mappings are isolated and well-contained
- Changes are additive/subtractive with no complex logic
- Easy to revert if issues arise

### Testing Considerations
- Focus on manual testing for user experience
- Ensure comprehensive testing of REPL input
- Test with multiple agents running for agent tab functionality

### Rollback Plan
If issues arise, simply revert the keyboard case statements to their original values. The changes are minimal and contained to specific switch statements.
