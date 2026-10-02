# Design Specification: Restore Autocomplete Dropdown Menu

**Issue:** #235  
**Title:** PR 230 broke autocomplete menu - only inline ghost text remains, dropdown menu no longer renders  
**Closes:** #235

## Problem Analysis

PR #230 successfully converted the autocomplete system to use Bubble Tea v2's built-in `textinput.ShowSuggestions` feature, which fixed the footer resizing issue from #229. However, this caused a regression: the visual dropdown menu was completely removed, leaving only inline ghost text.

### What Works (Must Preserve)
- ✅ Inline ghost text showing current suggestion after cursor
- ✅ Tab/Enter to accept suggestions
- ✅ Arrow key navigation between suggestions (updates ghost text)
- ✅ Footer height remains constant (no dynamic resizing)
- ✅ PR #230's footer height fix from issue #229

### What's Broken (Must Restore)
- ❌ Visual dropdown menu showing all matched suggestions
- ❌ Ability to see multiple command options at once
- ❌ Visual highlight of currently selected suggestion in menu
- ❌ Menu overlay positioned above input field

### Root Cause

The built-in `textinput.ShowSuggestions` feature only provides **inline ghost text completion**, not a dropdown menu. PR #230 removed the custom `ViewDropdown()` method and dropdown rendering logic (~150 lines), assuming the built-in feature would provide both capabilities. It does not.

**Previous Architecture (pre-PR #230):**
```text
AutocompleteInput.ViewDropdown() → Footer.RenderDropdownWithFooter() → inline in footer layout
```

**Current Architecture (post-PR #230):**
```text
textinput with ShowSuggestions=true → ghost text only → NO DROPDOWN RENDERING
```

**Required Architecture (hybrid solution):**
```text
textinput with ShowSuggestions=true → ghost text ✅
+ Custom overlay menu → lipgloss.Place() → rendered as overlay ✅
```

## Solution Approach

Implement a **hybrid approach** that combines the working built-in ghost text feature with a custom overlay menu component. This preserves PR #230's footer height fix while restoring the visual dropdown menu.

### High-Level Strategy

1. **Keep `textinput.ShowSuggestions = true`** — Provides working inline ghost text and keyboard navigation
2. **Add custom overlay menu component** — Provides visual dropdown showing all suggestions
3. **Render menu as overlay using `lipgloss.Place()`** — Non-blocking, doesn't affect footer dimensions
4. **Synchronize menu highlight with textinput's current suggestion** — Both features work together

### Architecture Decision: Hybrid Built-in + Custom Overlay

**Decision: Keep textinput.ShowSuggestions + Add Custom Menu Overlay**

Rationale:
- Built-in `ShowSuggestions` provides ghost text and keyboard navigation handling (working correctly)
- Custom overlay menu provides visual feedback of all suggestions (missing functionality)
- `lipgloss.Place()` enables overlay positioning without affecting footer height
- Maintains PR #230's footer height fix while restoring full autocomplete UX
- Minimal code addition (~50-80 lines vs. 150+ lines removed in PR #230)

## Relevant Files

### Primary Implementation Files
- `internal/tui/autocomplete.go` — Add `RenderSuggestionsMenu()` method for dropdown overlay
- `internal/tui/tui.go` — Integrate menu overlay rendering in `View()` method
- `internal/tui/styles.go` — Already has `AutocompleteDropdown` and `AutocompleteSelected` styles

### Supporting Files
- `internal/tui/autocomplete_overlay_validation_test.go` — Existing tests verify footer height stability
- `internal/tui/autocomplete_test.go` — Basic autocomplete functionality tests

## Team Orchestration

This implementation follows a **sequential approach** where the menu component is built first, then integrated into the rendering pipeline.

### Task Dependencies
1. **Task 1**: Independent (add method to existing component)
2. **Task 2**: Depends on Task 1 (integrates the new method)
3. **Task 3**: Depends on Task 2 (validates complete integration)

All tasks must be completed within this single PR to fully resolve issue #235.

## Step-by-Step Task Breakdown

### Task 1: Add Menu Overlay Rendering Method to AutocompleteInput
**File:** `internal/tui/autocomplete.go`  
**Acceptance Criteria:**
- Add `RenderSuggestionsMenu()` method that returns styled dropdown overlay string
- Method checks `HasMatchedSuggestions()` — returns empty string if no suggestions
- Use `textinput.MatchedSuggestions()` to get suggestion list
- Use `textinput.CurrentSuggestionIndex()` to determine which item to highlight
- Render each suggestion with appropriate styling (highlighted vs. normal)
- Apply `styles.AutocompleteSelected` to currently selected suggestion
- Apply `styles.AutocompleteDropdown` border/padding to menu box
- Limit display to first 10 suggestions to prevent overly large menus
- Return complete styled menu string ready for overlay positioning

**Implementation Pattern:**
```go
func (a *AutocompleteInput) RenderSuggestionsMenu() string {
    if !a.HasMatchedSuggestions() {
        return ""
    }
    
    suggestions := a.textinput.MatchedSuggestions()
    currentIndex := a.textinput.CurrentSuggestionIndex()
    
    var menuItems []string
    maxItems := 10
    if len(suggestions) < maxItems {
        maxItems = len(suggestions)
    }
    
    for i := 0; i < maxItems; i++ {
        var style lipgloss.Style
        if i == currentIndex {
            style = a.styles.AutocompleteSelected.Padding(0, 1)
        } else {
            style = lipgloss.NewStyle().Padding(0, 1)
        }
        menuItems = append(menuItems, style.Render(suggestions[i]))
    }
    
    menuBox := strings.Join(menuItems, "\n")
    return a.styles.AutocompleteDropdown.Render(menuBox)
}
```

**Dependencies:** None

### Task 2: Integrate Menu Overlay into Main View Rendering
**File:** `internal/tui/tui.go`  
**Acceptance Criteria:**
- Modify `View()` method to render autocomplete menu as overlay
- Check `m.input.HasMatchedSuggestions()` to determine if menu should appear
- Call `m.input.RenderSuggestionsMenu()` to get menu overlay content
- Position menu overlay above footer at bottom-left using `lipgloss.Place()`
- Menu appears after footer is rendered but before overlays (dialogs, help, etc.)
- Menu works consistently across all tab types (main, planning, agent)
- Menu does not interfere with existing overlay system (status, help, about, logs)
- Ensure menu disappears when no suggestions match (automatic via `HasMatchedSuggestions()`)

**Implementation Pattern:**
```go
func (m model) View() tea.View {
    // ... existing rendering logic ...
    
    // Combine tab headers with content
    content = tabHeaders + "\n" + content
    
    // Render autocomplete menu overlay BEFORE other overlays
    if m.input.HasMatchedSuggestions() {
        menuOverlay := m.input.RenderSuggestionsMenu()
        if menuOverlay != "" {
            // Position menu at bottom-left, above footer
            // Calculate position: leave room for footer (3 lines) + menu height
            menuLines := strings.Split(menuOverlay, "\n")
            menuHeight := len(menuLines)
            
            // Place menu above footer using lipgloss.Place
            content = m.layerOverlay(content, menuOverlay)
        }
    }
    
    // Compose other overlays if active (status, help, about, logs)
    if m.activeOverlay != overlayNone {
        overlay := m.renderOverlay()
        content = m.layerOverlay(content, overlay)
    }
    
    // ... rest of View() method ...
}
```

**Note on Positioning:**
The menu should be positioned above the footer input line. The `layerOverlay()` method already exists in `tui.go` and handles overlay positioning. We may need to adjust positioning parameters to ensure the menu appears directly above the footer without obscuring content.

**Dependencies:** Task 1

### Task 3: Validation and Testing
**Acceptance Criteria:**
- Verify both ghost text AND dropdown menu appear when typing commands
- Confirm menu shows all matched suggestions (up to 10)
- Validate currently selected item is visually highlighted in menu
- Test arrow key navigation updates both ghost text and menu highlight
- Verify Tab/Enter accepts current suggestion from both ghost text and menu
- Confirm Escape dismisses menu (handled by textinput clearing suggestions)
- Test menu appears/disappears correctly as user types
- Verify footer height remains constant (existing tests pass)
- Validate menu rendering across all tab types (main, planning, agent)
- Test terminal resize with menu active
- Run existing test suite: `go test ./internal/tui/...` — all tests pass
- Manual testing: Type partial commands, navigate with arrows, accept with Tab/Enter

**Manual Test Scenarios:**
1. Type "wa" → verify ghost text shows "watch" AND menu shows ["watch"]
2. Type "w" → verify menu shows ["watch"] and any other "w" commands
3. Type "help" → verify exact match shows in ghost text and menu
4. Press down arrow → verify menu highlight changes and ghost text updates
5. Press up arrow → verify wraparound works (last → first)
6. Press Tab → verify suggestion accepted, menu dismissed
7. Press Escape → verify menu dismissed (suggestions cleared)
8. Type "xyz" → verify no ghost text, no menu (no matches)
9. Switch tabs with menu active → verify menu appears consistently
10. Resize terminal with menu active → verify menu repositions correctly

**Dependencies:** Task 2

## Validation Commands

### Build and Manual Testing
```bash
# Build the application
go build ./cmd/kiro-krew

# Run and test autocomplete functionality
./kiro-krew

# Test scenarios in REPL:
# 1. Type "w" — should show ghost text AND dropdown menu
# 2. Use arrow keys — menu highlight and ghost text should change together
# 3. Press Tab — should accept suggestion and dismiss menu
# 4. Type "st" — should show "status" and "stop <issue>" in menu
# 5. Press Escape — should dismiss menu
# 6. Switch to planning tab — autocomplete should work identically
# 7. Resize terminal — menu should reposition correctly
```

### Automated Testing
```bash
# Run all TUI tests (including existing footer height validation tests)
go test ./internal/tui/... -v

# Existing tests should all pass, confirming:
# - Footer height remains constant (Task 4 validation from PR #230)
# - No layout regressions
# - Autocomplete functionality preserved
```

### Regression Testing (Critical)
```bash
# Verify PR #230's footer height fix is not broken
# Footer must remain exactly 3 lines in all states:
# - No suggestions: 3 lines
# - Suggestions visible: 3 lines (menu is overlay, doesn't affect footer)
# - After accepting suggestion: 3 lines

# The test TestTask4IntegrationValidation.FooterHeightRemainsConstantDuringAutocompleteUsage
# validates this requirement and MUST continue to pass
```

## Technical Implementation Notes

### Synchronization with textinput State

The custom menu overlay stays synchronized with textinput's internal state:
- `textinput.MatchedSuggestions()` — provides current filtered suggestion list
- `textinput.CurrentSuggestionIndex()` — indicates which suggestion is selected
- `textinput.ShowSuggestions` remains `true` — enables ghost text and keyboard navigation
- Arrow keys handled by textinput update both ghost text and current index
- Menu reads this state each render cycle to display correct highlight

### Overlay Positioning Strategy

Use existing `layerOverlay()` method in `tui.go` for positioning:
```go
func (m model) layerOverlay(base, overlay string) string {
    baseLines := strings.Split(base, "\n")
    overlayLines := strings.Split(overlay, "\n")
    
    // Calculate position for menu above footer
    // Footer is at bottom (last 3 lines), menu should appear above it
    startRow := len(baseLines) - 3 - len(overlayLines) - 1
    startCol := 0 // Left-aligned with input prompt
    
    // ... overlay rendering logic ...
}
```

**Alternative:** If `layerOverlay()` doesn't provide sufficient control, use `lipgloss.Place()` directly:
```go
menuOverlay := lipgloss.Place(
    m.width, m.height,
    lipgloss.Left,     // Horizontal alignment
    lipgloss.Bottom,   // Vertical alignment
    menuContent,       // Menu string
    lipgloss.WithWhitespaceChars(" "),
)
```

### Style Reuse from PR #230

The styles removed in PR #230 but still defined in `styles.go`:
- `AutocompleteDropdown` — Border and background for menu box
- `AutocompleteSelected` — Highlight style for selected item

These styles are already available and theme-integrated. No new styles required.

### Reference Implementation

The old `ViewDropdown()` method (removed in PR #230) provides reference for:
- Menu item styling and layout
- Selected item highlighting
- 10-item limit for menu size
- Menu box border and padding

Key differences from old implementation:
1. **Data source:** Use `textinput.MatchedSuggestions()` instead of custom state
2. **Selection index:** Use `textinput.CurrentSuggestionIndex()` instead of custom tracking
3. **Rendering location:** Overlay via `lipgloss.Place()` instead of inline in footer
4. **Lifecycle:** Tied to `HasMatchedSuggestions()` not custom `showDropdown` flag

## Success Criteria

### Functional Requirements
- [x] Ghost text shows current suggestion inline after cursor (preserved from PR #230)
- [ ] Dropdown menu shows all matched suggestions as visual overlay
- [ ] Currently selected suggestion is highlighted in menu
- [ ] Menu and ghost text stay synchronized during arrow key navigation
- [ ] Menu appears automatically when suggestions match user input
- [ ] Menu dismisses automatically when no suggestions match
- [ ] Tab/Enter accepts current suggestion (existing behavior preserved)
- [ ] Escape dismisses suggestions and menu (existing behavior preserved)
- [ ] Works identically across main, planning, and agent tabs

### Layout Requirements (Critical - Must Not Break PR #230's Fix)
- [ ] Footer height remains constant at 3 lines in all states
- [ ] Menu renders as overlay, not inline in footer layout
- [ ] Menu does not cause visual jumps or layout shifts
- [ ] Menu positioned above footer, not overlapping input line
- [ ] Menu handles terminal resize gracefully

### Technical Requirements
- [ ] `RenderSuggestionsMenu()` method added to `AutocompleteInput`
- [ ] Menu rendering integrated into `tui.go` `View()` method
- [ ] Uses `lipgloss.Place()` or existing `layerOverlay()` for positioning
- [ ] Existing `AutocompleteDropdown` and `AutocompleteSelected` styles utilized
- [ ] No modifications to `textinput.ShowSuggestions` configuration (keep = true)
- [ ] No new state tracking in `AutocompleteInput` (uses textinput's internal state)

### Testing Requirements
- [ ] All existing tests pass (especially `TestTask4IntegrationValidation`)
- [ ] `FooterHeightRemainsConstantDuringAutocompleteUsage` test passes
- [ ] Manual testing confirms both ghost text and menu work together
- [ ] No performance degradation in typing responsiveness
- [ ] Terminal resize with active menu works correctly

## Risk Mitigation

### Risk: Breaking PR #230's Footer Height Fix
**Mitigation:** 
- Menu MUST be rendered as overlay using `lipgloss.Place()` or `layerOverlay()`
- Menu MUST NOT affect footer height calculations
- Existing test `TestTask4IntegrationValidation.FooterHeightRemainsConstantDuringAutocompleteUsage` MUST pass
- Footer height MUST remain 3 lines in all autocomplete states

### Risk: Menu Positioning Conflicts with Other Overlays
**Mitigation:**
- Render autocomplete menu BEFORE checking `m.activeOverlay != overlayNone`
- Autocomplete menu is transient and auto-dismisses on certain inputs
- Dialog overlays (status, help, about) should take precedence (render on top)
- Order: base content → autocomplete menu → dialog overlays

### Risk: Ghost Text and Menu Desynchronization
**Mitigation:**
- Do NOT maintain separate suggestion state in `AutocompleteInput`
- Always read from `textinput.MatchedSuggestions()` and `textinput.CurrentSuggestionIndex()`
- Let textinput handle all keyboard navigation and state updates
- Menu is purely a visual representation of textinput's internal state

## Implementation Estimate

- **Task 1 (Add RenderSuggestionsMenu):** ~30-50 lines, straightforward method addition
- **Task 2 (Integrate menu overlay):** ~15-25 lines, modify existing `View()` method
- **Task 3 (Testing):** Validation using existing test suite + manual testing

**Total Implementation:** ~50-80 lines of new code (vs. 150+ lines removed in PR #230)

This is a minimal addition that restores critical UX functionality while preserving all benefits of PR #230.
