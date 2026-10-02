# Design Specification: Convert Autocomplete Menu to Non-blocking Overlay Dialog

**Issue:** #229  
**Title:** Convert autocomplete menu from inline footer component to non-blocking overlay dialog  
**Closes:** #229

## Problem Analysis

The current autocomplete implementation causes visual problems due to dynamic footer resizing when suggestions appear/disappear. This creates a jarring user experience, especially noticeable on planning and agent tabs.

### Root Cause Investigation

**Current Architecture Issues:**
1. **Dynamic Height Calculation**: `GetFooterHeightWithDropdown()` returns different heights based on dropdown visibility
2. **Inline Rendering**: `RenderDropdownWithFooter()` adds dropdown content directly to footer layout
3. **Layout Propagation**: Height changes cascade through the entire view system, causing visible resizing
4. **Custom Implementation**: Using custom dropdown instead of Bubble Tea v2's built-in capabilities

**Current Flow:**
```
User Types → AutocompleteInput.Update() → Footer.RenderDropdownWithFooter() → TUI.renderTabContentWithFooter() → Dynamic Height Calculation → Layout Resize
```

## Solution Approach

Replace the current inline dropdown system with Bubble Tea v2's native autocomplete overlay capabilities, utilizing `textinput.ShowSuggestions` and `lipgloss.Place()` for positioning.

### High-Level Strategy

1. **Eliminate Dynamic Footer Height**: Remove `GetFooterHeightWithDropdown()` and maintain constant footer dimensions
2. **Replace Custom Dropdown**: Use `textinput.ShowSuggestions = true` for native autocomplete behavior
3. **Overlay Positioning**: Use `lipgloss.Place()` to position suggestions as absolute overlay
4. **Layout Isolation**: Decouple autocomplete rendering from footer layout system

### Architecture Decision: Built-in vs Custom Overlay

**Decision: Use Bubble Tea v2 Built-in Capabilities**

- Bubble Tea v2.0.6 includes native `textinput.ShowSuggestions` functionality
- Lipgloss v2.0.3 provides `lipgloss.Place()` for precise overlay positioning
- Built-in implementation handles keyboard navigation, focus management, and rendering optimization
- Reduces custom code maintenance and leverages framework capabilities

## Relevant Files

### Primary Implementation Files
- `internal/tui/autocomplete.go` — Current custom autocomplete implementation
- `internal/tui/footer.go` — Footer system with dynamic height calculations  
- `internal/tui/tui.go` — Main rendering logic with layout calculations

### Supporting Files
- `internal/tui/styles.go` — Style definitions for autocomplete elements
- `go.mod` — Confirms Bubble Tea v2.0.6 and Lipgloss v2.0.3 availability

## Team Orchestration

This implementation follows a **layered refactoring approach** where each component is updated to work with the new overlay system:

### Task Dependencies
1. **Tasks 1 & 2**: Can run in parallel (different files, no direct dependencies)
2. **Task 3**: Depends on Tasks 1 & 2 completion (integrates both changes)
3. **Task 4**: Depends on Task 3 completion (requires integrated system for testing)

## Step-by-Step Task Breakdown

### Task 1: Replace Custom Autocomplete with Built-in Implementation
**File:** `internal/tui/autocomplete.go`  
**Acceptance Criteria:**
- Remove custom `AutocompleteState` struct and dropdown rendering logic
- Replace with Bubble Tea v2's `textinput.ShowSuggestions = true` 
- Configure `textinput.SetSuggestions()` with command registry integration
- Maintain existing keyboard shortcuts (up/down navigation, tab completion, escape dismissal)
- Preserve ghost text functionality and command validation
- Remove `ViewDropdown()`, `IsDropdownVisible()` and related dropdown methods

**Dependencies:** None

### Task 2: Remove Dynamic Footer Height System
**File:** `internal/tui/footer.go`  
**Acceptance Criteria:**
- Remove `GetFooterHeightWithDropdown()` method entirely
- Remove `RenderDropdownWithFooter()` method 
- Update `GetFooterHeight()` to return constant height (3 lines: separator + input + status)
- Simplify footer rendering to use only `RenderWithSeparator()`
- Remove dropdown-related logic from footer rendering pipeline

**Dependencies:** None

### Task 3: Update Main Rendering Pipeline
**File:** `internal/tui/tui.go`  
**Acceptance Criteria:**
- Update `renderBaseView()` to use constant `GetFooterHeight()` instead of `GetFooterHeightWithDropdown()`
- Update `renderTabContentWithFooter()` to use `RenderWithSeparator()` instead of `RenderDropdownWithFooter()`
- Remove dropdown height calculations from viewport sizing logic
- Update window resize handling to use constant footer height
- Ensure all tab types (main, planning, agent) use consistent footer height calculations

**Dependencies:** Tasks 1, 2

### Task 4: Integration Testing and Validation
**Acceptance Criteria:**
- Verify autocomplete appears as overlay without affecting layout on all tab types
- Confirm footer height remains constant during autocomplete usage
- Test keyboard navigation (up/down arrows, tab, escape) works correctly
- Validate styling matches current theme system
- Verify no performance regression in typing responsiveness
- Test terminal resize handling with overlay active
- Confirm ghost text functionality preserved

**Dependencies:** Task 3

## Validation Commands

### Manual Testing
```bash
# Build and run the application
go build ./cmd/kiro-krew
./kiro-krew

# Test scenarios:
# 1. Type partial commands to trigger autocomplete
# 2. Navigate suggestions with arrow keys 
# 3. Use tab to complete suggestions
# 4. Press escape to dismiss menu
# 5. Switch between tabs while autocomplete is active
# 6. Resize terminal while autocomplete is visible
```

### Automated Testing
```bash
# Run existing tests to ensure no regressions
go test ./internal/tui/...

# Run integration tests
go test ./...
```

### Layout Validation
```bash
# Test footer height consistency
# Before: Footer height changes when autocomplete appears
# After: Footer height remains constant (3 lines) regardless of autocomplete state
```

## Technical Implementation Notes

### Bubble Tea v2 Native Autocomplete Pattern
```go
// In AutocompleteInput.Init()
ti := textinput.New()
ti.ShowSuggestions = true
ti.SetSuggestions(suggestionList)

// Automatic overlay positioning and keyboard handling
// No custom dropdown rendering required
```

### Lipgloss v2 Overlay Positioning (if needed)
```go
// Alternative approach for custom positioning
overlay := lipgloss.Place(
    width, height,           // Canvas dimensions
    lipgloss.Left,          // Horizontal alignment  
    lipgloss.Bottom,        // Vertical alignment
    suggestionContent,      // Content to overlay
)
```

### Migration Strategy
1. **Preserve Interface**: Keep `AutocompleteInput` public interface unchanged
2. **Internal Refactoring**: Replace implementation while maintaining external behavior
3. **Gradual Removal**: Remove unused methods after confirming no external dependencies

## Success Criteria

### Functional Requirements
- [ ] Autocomplete menu renders as overlay without layout shifts
- [ ] Footer height remains constant (3 lines) in all states
- [ ] All keyboard shortcuts work identically to current implementation
- [ ] Works consistently across all tab types (main, planning, agent)
- [ ] Theme styling preserved for autocomplete elements

### Technical Requirements  
- [ ] No `GetFooterHeightWithDropdown()` method exists
- [ ] No `RenderDropdownWithFooter()` method exists
- [ ] All layout calculations use constant footer height
- [ ] Built-in `textinput.ShowSuggestions` used instead of custom dropdown

### User Experience Requirements
- [ ] Smooth autocomplete appearance without visual jumps
- [ ] Consistent interface behavior across all usage scenarios
- [ ] No performance degradation in typing responsiveness
- [ ] Graceful handling of terminal resize with overlay active
