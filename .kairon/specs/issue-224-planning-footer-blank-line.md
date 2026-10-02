# Design Specification: Fix Extra Blank Line in Planning Tab Footer

**Issue:** #224 - Planning tab footer shows extra blank line below status row  
**Repository:** jbrinkman/kiro-krew  
Closes #224

## Problem Analysis

The planning tab footer displays an extra blank line below the status row, creating unwanted visual spacing. This appears to be a regression introduced after the recent footer rendering bug fix in commit 34fd48f.

### Root Cause Investigation

After analyzing the codebase and recent changes, the issue stems from the interaction between:

1. **Planning Tab Content Generation**: The `PlanningTab.View()` method uses `lipgloss.JoinVertical()` which may add trailing newlines
2. **Unified Footer Rendering**: The `renderTabContentWithFooter()` method was recently modified to handle empty content, but may not properly handle content that already has trailing newlines
3. **Footer Structure**: The footer should consist of exactly 3 lines (separator + input row + status row), but an extra newline is being added somewhere in the rendering pipeline

### Likely Sources

Based on the code analysis, the extra blank line is likely caused by:

- **Footer Rendering Logic**: The `RenderWithSeparator` or `RenderDropdownWithFooter` methods in `FooterManager` may be adding extra newlines
- **Lipgloss Style Rendering**: The `ThemeLabel.Render()` call on the status row may be adding padding or extra newlines
- **String Join Logic**: The `strings.Join(result, "\n")` in footer rendering may be creating double newlines when combined with existing newlines from tab content

## Solution Approach

Implement a focused fix that identifies and eliminates the source of the extra blank line while maintaining the unified footer rendering system. This approach:

1. **Preserves Recent Fixes**: Maintains the existing footer rendering improvements from PR #219
2. **Targeted Investigation**: Focuses on the specific footer rendering pipeline for planning tabs
3. **Maintains Consistency**: Ensures all tabs render footers with identical spacing
4. **Future-Proofs**: Prevents similar issues in other tab types

## Relevant Files

- **`internal/tui/footer.go`** - Footer rendering system (primary investigation area)
- **`internal/tui/tui.go`** - Unified footer integration with `renderTabContentWithFooter`
- **`internal/tui/planning_tab.go`** - Planning tab content generation (validation)
- **`internal/tui/styles.go`** - Style definitions that may affect footer rendering

## Team Orchestration

This is a single-component debugging and fix task that can be completed independently:

- **Root Cause Investigation**: Debug the exact source of the extra blank line
- **Footer Rendering Fix**: Modify footer rendering logic to eliminate extra newlines  
- **Validation Testing**: Verify footer consistency across all tab types
- **No Dependencies**: Independent of other features or ongoing development

## Step-by-Step Task Breakdown

### Task 1: Debug Root Cause of Extra Blank Line
**Acceptance Criteria**:
- Investigate footer rendering pipeline to identify exact source of extra newline
- Analyze `RenderWithSeparator` and `RenderDropdownWithFooter` methods
- Check if `ThemeLabel.Render()` is adding extra spacing/padding
- Verify if `strings.Join(result, "\n")` is causing double newlines
- Document the precise location where the extra blank line is introduced
**Dependencies**: None

### Task 2: Implement Targeted Footer Rendering Fix
**Acceptance Criteria**:
- Modify the identified source to eliminate the extra blank line
- Ensure footer structure remains exactly 3 lines (separator + input + status)
- Preserve all existing footer functionality (dropdown, context display, etc.)
- Maintain footer height calculation accuracy (should return 3)
- Fix should work for both `RenderWithSeparator` and `RenderDropdownWithFooter`
**Dependencies**: Task 1

### Task 3: Validate Footer Consistency Across All Tab Types
**Acceptance Criteria**:
- Planning tab footer shows no extra blank lines below status row
- Main tab footer spacing remains unchanged  
- Agent tab footers render consistently (if applicable)
- Footer height calculations match actual rendered content
- Tab switching maintains consistent footer behavior
- Dropdown functionality works correctly on all tabs
**Dependencies**: Task 1, Task 2

### Task 4: Verify Edge Cases and Regression Prevention
**Acceptance Criteria**:
- Empty status row handling works correctly (no blank line where status should be)
- Long status content that wraps behaves properly
- Footer rendering with and without dropdown maintains consistency
- All footer rendering code paths produce identical line counts
- No regression in previous footer fixes from PR #219 and PR #210
**Dependencies**: Task 1, Task 2, Task 3

## Implementation Details

### Investigation Areas (Task 1)

1. **Footer Structure Analysis**:
   ```go
   // Current footer should be:
   // Line 1: separator (─────────)
   // Line 2: input row (command prompt)
   // Line 3: status row (theme, planning info)
   // Total: 3 lines
   ```

2. **Check `RenderWithSeparator` Method**:
   ```go
   // Verify this logic in footer.go:
   var result []string
   result = append(result, separator)
   result = append(result, footer.InputRow)
   if strings.TrimSpace(footer.StatusRow) != "" {
       result = append(result, fm.styles.ThemeLabel.Render(footer.StatusRow))
   }
   return strings.Join(result, "\n")
   ```

3. **Check `ThemeLabel` Style**:
   ```go
   // Verify if ThemeLabel style adds extra padding/newlines:
   ThemeLabel: lipgloss.NewStyle().
       Foreground(lipgloss.Color(theme.Colors.TextMuted)).
       Italic(true),
   ```

### Potential Fix Approaches (Task 2)

**Option 1: Fix String Join Logic**
```go
// If the issue is in the join operation:
result = strings.Join(result, "\n")
// Ensure no trailing newlines are added
return strings.TrimRight(strings.Join(result, "\n"), "\n")
```

**Option 2: Fix Style Rendering**
```go
// If ThemeLabel style is adding extra newlines:
return strings.TrimSpace(fm.styles.ThemeLabel.Render(footer.StatusRow))
```

**Option 3: Fix Status Row Handling**
```go
// If empty status row handling is incorrect:
if strings.TrimSpace(footer.StatusRow) != "" {
    statusRow := strings.TrimSpace(fm.styles.ThemeLabel.Render(footer.StatusRow))
    if statusRow != "" {
        result = append(result, statusRow)
    }
}
```

## Validation Commands

### Visual Testing
```bash
# Build and run application
go build ./cmd/kiro-krew
./kiro-krew

# In the TUI:
# 1. Use Ctrl+Alt+P to switch to planning tab
# 2. Observe footer area - count the lines
# 3. Switch back to main tab with Ctrl+Alt+P  
# 4. Compare footer line counts visually
# 5. Try different planning tab states (idle, active, etc.)
```

### Debug Output Testing
```bash
# If needed, add temporary debug output to identify the issue:
# In footer.go, add logging to see exact footer structure
```

### Automated Testing
```bash
# Run existing tests to ensure no regression
go test ./internal/tui/...

# Specifically test footer components
go test -run TestFooter ./internal/tui/...
```

## Success Criteria

- ✅ Planning tab footer displays exactly 3 lines (separator + input + status)
- ✅ No extra blank line appears below the status row on planning tab
- ✅ Footer spacing is identical between main and planning tabs
- ✅ Footer height calculation returns 3 and matches actual rendered content
- ✅ All existing footer functionality continues to work properly
- ✅ Dropdown functionality works correctly on all tab types
- ✅ No regression from previous footer rendering fixes

## Risk Mitigation

**Low Risk Changes**: Targeted fix in footer rendering system with well-defined boundaries.

**Regression Prevention**: Thorough testing across all tab types to ensure consistent behavior.

**Backward Compatibility**: Changes only affect visual rendering, no API changes.

## Notes

This issue represents a visual regression that affects user experience in the planning tab interface. The fix should be surgical, targeting the specific source of the extra blank line while preserving all recent footer improvements and maintaining consistency across the entire TUI interface.

The investigation should focus on the footer rendering pipeline, particularly the interaction between content normalization in `renderTabContentWithFooter` and the footer structure generation in `FooterManager`. The goal is to ensure the footer consistently renders as exactly 3 lines across all tab types.
