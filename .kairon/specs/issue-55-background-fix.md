# Design Specification: Fix Background Color Inconsistency in Status Dialog

**Issue:** #55 - Fix background color inconsistency in status dialog for high-contrast theme  
**Closes #55**

## Problem Analysis

The status dialog in the TUI REPL exhibits visual inconsistency in the high-contrast theme where:

1. **OverlayContent** style uses `text_primary` color (#FFFFFF) but lacks explicit background
2. **OverlayBorder** uses `surface` color (#222222) as background
3. This creates a visual mismatch where text content appears with no background while the border has the surface background
4. The issue is most visible in high-contrast theme due to bright white text on potentially transparent/terminal background

## Solution Approach

### Root Cause
In `internal/tui/styles.go`, the `OverlayContent` style only sets foreground color but doesn't inherit or explicitly set the background color from the theme's `surface` color.

### Strategy
Add explicit background color to `OverlayContent` style to match `OverlayBorder`'s surface background, ensuring visual consistency across all overlay dialogs.

### Design Principles
- **Minimal Change**: Single line modification to fix the issue
- **Theme Consistency**: Use existing theme color (`surface`) for background
- **Universal Fix**: Apply to all overlay types (status, help, about)
- **Backward Compatibility**: Maintain existing theme structure and behavior

## Relevant Files

### Primary Files (Modification Required)
- `internal/tui/styles.go` - **MODIFY**: Add background color to OverlayContent style

### Secondary Files (Validation/Testing)
- `internal/tui/tui.go` - Overlay rendering logic (no changes needed)
- `internal/tui/commands.go` - Status command implementation (no changes needed)
- `.kiro-krew/themes/high-contrast.yaml` - High-contrast theme definition (reference only)
- `.kiro-krew/themes/default.yaml` - Default theme definition (validation)
- `.kiro-krew/themes/light.yaml` - Light theme definition (validation)

## Team Orchestration

This is a single-developer task with no coordination requirements:

1. **Developer Role**: Implement the background color fix
2. **Testing Role**: Validate across all themes
3. **No External Dependencies**: Self-contained change within TUI module

## Step-by-Step Task Breakdown

### Task 1: Implement Background Color Fix
**Acceptance Criteria:**
- [ ] Modify `OverlayContent` style in `NewStyles()` function
- [ ] Add `.Background(lipgloss.Color(theme.Colors.Surface))` to the style chain
- [ ] Ensure change matches the pattern used in `OverlayBorder` style

**Implementation Details:**
```go
// In internal/tui/styles.go, NewStyles() function
OverlayContent: lipgloss.NewStyle().
    Foreground(lipgloss.Color(theme.Colors.TextPrimary)).
    Background(lipgloss.Color(theme.Colors.Surface)),
```

### Task 2: Validate Theme Consistency
**Acceptance Criteria:**
- [ ] Test status dialog with high-contrast theme (primary target)
- [ ] Test status dialog with default theme (regression check)
- [ ] Test status dialog with light theme (regression check)
- [ ] Verify other overlays (help, about) maintain consistency
- [ ] Confirm no visual artifacts or color bleeding

**Test Scenarios:**
1. Run `status` command in each theme
2. Run `help` command in each theme  
3. Run `about` command in each theme
4. Check overlay text readability and background consistency

### Task 3: Integration Testing
**Acceptance Criteria:**
- [ ] Build application successfully without errors
- [ ] Launch TUI and verify basic functionality
- [ ] Test overlay dismissal (ESC key) works correctly
- [ ] Verify overlay positioning and sizing unchanged

## Validation Commands

### Build and Basic Functionality
```bash
# Build the application
go build -o kiro-krew ./cmd/kiro-krew

# Launch TUI (requires terminal)
./kiro-krew
```

### Theme Testing Sequence
```bash
# In the TUI REPL, test each theme:

# Switch to high-contrast theme and test
theme high-contrast
status
# Press ESC to close overlay
help  
# Press ESC to close overlay
about
# Press ESC to close overlay

# Switch to default theme and test  
theme default
status
help
about

# Switch to light theme and test
theme light  
status
help
about

# Return to preferred theme
theme high-contrast
```

### Visual Validation Checklist
- [ ] Status dialog background is consistent (no transparent/mismatched areas)
- [ ] Text is clearly readable against the background
- [ ] Border and content backgrounds match visually
- [ ] No regression in other themes
- [ ] Overlay positioning and sizing unchanged
- [ ] ESC key dismissal works correctly

## Risk Assessment

**Low Risk Change:**
- Single line addition to existing style
- Uses existing theme color (`surface`)
- No changes to rendering logic or overlay system
- Backward compatible with all existing themes

**Potential Issues:**
- None identified - change follows established patterns in codebase

## Success Metrics

1. **Functional**: Status dialog displays with consistent background colors
2. **Visual**: No visible mismatch between border and content backgrounds  
3. **Compatibility**: All three themes (default, light, high-contrast) work correctly
4. **Regression**: No impact on other overlay types or TUI functionality
