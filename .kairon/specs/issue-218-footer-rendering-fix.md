# Design Specification: Fix Footer Rendering Inconsistency

**Issue:** #218 - Fix footer rendering inconsistency between main and planning tabs  
**Repository:** jbrinkman/kiro-krew  
Closes #218

## Problem Analysis

The footer on the planning tab displays an extra blank line compared to the main tab, creating visual inconsistency in the UI. This manifests as additional spacing between the tab content and the footer status row.

### Root Cause

The issue is in the `renderTabContentWithFooter` function in `internal/tui/tui.go` at line 611:

```go
return tabContent + "\n" + footerWithDropdown
```

This function unconditionally adds a newline between tab content and footer. However:

1. **Main tab content** (from `consoleViewport.View()`) does not end with a trailing newline
2. **Planning tab content** (from `lipgloss.JoinVertical()`) does end with a trailing newline

This creates a double newline (`content\n` + `\n` + footer) for planning tabs but single newline (`content` + `\n` + footer) for main tabs.

## Solution Approach

Implement intelligent newline handling in `renderTabContentWithFooter` to normalize tab content before adding the footer. This approach:

1. **Preserves existing functionality** - no changes to individual tab rendering logic
2. **Maintains unified footer system** - all fixes contained within the rendering pipeline  
3. **Handles edge cases** - works with empty content, content with multiple trailing newlines
4. **Future-proofs** - automatically handles any new tab types with different content ending patterns

## Relevant Files

- **`internal/tui/tui.go`** - Contains `renderTabContentWithFooter` function (primary modification)
- **`internal/tui/planning_tab.go`** - Planning tab View method (validation/testing)
- **`internal/tui/footer.go`** - Footer rendering system (validation/testing)

## Team Orchestration  

This is a single-component fix that can be completed in parallel with other development work:

- **Frontend rendering fix**: Modify `renderTabContentWithFooter` to normalize newlines
- **Testing validation**: Verify footer consistency across all tab types
- **No dependencies**: Independent of other features or ongoing work

## Step-by-Step Task Breakdown

### Task 1: Implement Intelligent Newline Handling
**Acceptance Criteria**:
- Modify `renderTabContentWithFooter` function to normalize tab content
- Remove trailing newlines from tab content before adding footer separator
- Ensure empty content is handled correctly (no extra newlines)
- Preserve all existing footer functionality
**Dependencies**: None

### Task 2: Validate Footer Consistency  
**Acceptance Criteria**:
- Footer spacing is identical between main and planning tabs
- No extra blank lines appear in any tab's footer area
- Footer height calculations remain accurate
- All tab types render with consistent footer spacing
**Dependencies**: Task 1

### Task 3: Verify Edge Cases
**Acceptance Criteria**:  
- Empty tab content renders footer correctly
- Content with multiple trailing newlines is normalized properly
- Footer dropdown functionality works correctly on all tabs
- Tab switching maintains consistent footer behavior
**Dependencies**: Task 1, Task 2

## Implementation Details

### Core Fix (Task 1)

Replace the simple concatenation in `renderTabContentWithFooter`:

```go
// Before (line 611)
return tabContent + "\n" + footerWithDropdown

// After  
func (m model) renderTabContentWithFooter(tabContent string, tabType TabType) string {
	// Render footer using the footer system
	footerWithDropdown, _ := m.footerManager.RenderDropdownWithFooter(tabType)
	
	// Normalize tab content by removing trailing newlines
	normalizedContent := strings.TrimRight(tabContent, "\n")
	
	// Compose the complete view with consistent newline separation
	return normalizedContent + "\n" + footerWithDropdown
}
```

### Alternative Implementation Approach

If the above approach affects other functionality, consider checking for existing newlines:

```go
// Conditional newline approach
func (m model) renderTabContentWithFooter(tabContent string, tabType TabType) string {
	footerWithDropdown, _ := m.footerManager.RenderDropdownWithFooter(tabType)
	
	// Add newline only if content doesn't already end with one
	separator := "\n"
	if strings.HasSuffix(tabContent, "\n") {
		separator = ""
	}
	
	return tabContent + separator + footerWithDropdown
}
```

## Validation Commands

### Visual Verification
```bash
# Build and run the application
go build ./cmd/kiro-krew
./kiro-krew

# In the TUI:
# 1. Switch to planning tab (Ctrl+Alt+P) 
# 2. Switch back to main tab (Ctrl+Alt+P)
# 3. Verify footer spacing is identical
# 4. Check various planning tab states (idle, active, completed)
```

### Automated Testing  
```bash
# Run existing tests to ensure no regression
go test ./internal/tui/...

# Test footer height calculations
go test -run TestFooter ./internal/tui/...

# Test tab rendering consistency  
go test -run TestTab ./internal/tui/...
```

## Success Criteria

- ✅ Footer renders with identical spacing on main and planning tabs
- ✅ No extra blank lines appear after status row on planning tab  
- ✅ Footer height calculations remain consistent across all tabs
- ✅ All existing footer functionality continues to work (dropdown, context display, etc.)
- ✅ Tab switching maintains consistent visual behavior
- ✅ Edge cases handled properly (empty content, multiple trailing newlines)

## Risk Mitigation

**Low Risk Changes**: Modification is contained within a single function with well-defined input/output behavior.

**Backward Compatibility**: Changes only affect visual rendering, no API or data structure changes.

**Testing Strategy**: Visual verification combined with existing automated test suite provides comprehensive coverage.

## Notes

This fix addresses a UI consistency issue that affects user experience but does not impact functionality. The solution normalizes content formatting within the existing unified footer rendering system, ensuring consistent visual presentation across all tab types while maintaining the current architecture.