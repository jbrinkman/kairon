# Fix Broken Table Formatting in Status Overlay

**Issue**: #52  
**Closes**: #52

## Problem Analysis

The status overlay in `internal/tui/commands.go` has broken table formatting where hardcoded separator characters (`────`) display as broken lines instead of proper horizontal separators. The issue stems from:

1. **Hardcoded separator generation**: Uses `strings.Repeat("─", 70)` with a fixed-width Unicode character
2. **Fixed table width**: The 70-character width doesn't adapt to overlay dimensions
3. **Poor column alignment**: Static formatting doesn't account for variable content lengths within the overlay bounds
4. **Unicode rendering issues**: The `─` character may not render properly across all terminal configurations

## Current Implementation Issues

In `handleStatus()` function (internal/tui/commands.go:54-70):
```go
header := fmt.Sprintf("%-8s %-30s %-10s %s", "Issue", "Title", "Status", "Elapsed")
sep := strings.Repeat("─", 70)  // Problem: fixed width, Unicode char
content = append(content, m.styles.Prompt.Render(header), m.styles.Separator.Render(sep))
```

Problems:
- Fixed 70-character width doesn't match overlay width calculations
- Unicode `─` character causes rendering artifacts
- No consideration for overlay content width constraints

## Solution Approach

### 1. Dynamic Table Width Calculation
- Calculate table width based on overlay content area dimensions
- Use existing overlay width calculation from `renderOverlay()` function
- Ensure table fits within `maxContentWidth` bounds

### 2. ASCII-Safe Separator
- Replace Unicode `─` with ASCII `-` character for universal compatibility
- Maintain visual separation while ensuring proper rendering

### 3. Responsive Column Widths
- Adjust column widths proportionally to available space
- Ensure minimum widths for readability
- Handle content truncation gracefully within columns

### 4. Consistent Styling
- Leverage existing theme system for table elements
- Maintain separation between header styling (Prompt) and separator styling (Separator)

## Relevant Files

### Primary Changes Required
- **`internal/tui/commands.go`**: 
  - Modify `handleStatus()` function (lines 54-70)
  - Fix table width calculation and separator generation

### Supporting Files (No Changes)
- **`internal/tui/tui.go`**: Overlay rendering system (working correctly)
- **`internal/tui/styles.go`**: Theme styling system (working correctly) 
- **`.kiro-krew/themes/*.yaml`**: Theme definitions (compatible)

## Implementation Plan

### Step 1: Calculate Dynamic Table Width
```go
// Add helper function to calculate available content width
func (m model) getOverlayContentWidth() int {
    overlayWidth := int(float64(m.width) * 0.6)
    if overlayWidth < 40 {
        overlayWidth = 40
    }
    if overlayWidth >= m.width {
        overlayWidth = m.width - 2
    }
    return overlayWidth - 6 // Account for border + padding
}
```

### Step 2: Implement Responsive Column Layout
```go
// Update handleStatus() with dynamic calculations
func (m model) handleStatus() (model, tea.Cmd) {
    agents := m.manager.List()
    content := []string{}
    
    if len(agents) == 0 {
        content = append(content, m.styles.Warning.Render("No agents running"))
    } else {
        // Calculate available width for table
        contentWidth := m.getOverlayContentWidth()
        
        // Define proportional column widths (maintain existing ratios)
        issueWidth := max(5, contentWidth * 8 / 70)    // ~11% (was 8/70)
        titleWidth := max(10, contentWidth * 30 / 70)  // ~43% (was 30/70) 
        statusWidth := max(7, contentWidth * 10 / 70)  // ~14% (was 10/70)
        elapsedWidth := max(5, contentWidth * 22 / 70) // ~31% (remaining)
        
        // Create header with calculated widths
        header := fmt.Sprintf("%-*s %-*s %-*s %s", 
            issueWidth, "Issue",
            titleWidth, "Title", 
            statusWidth, "Status",
            "Elapsed")
            
        // Generate ASCII separator matching content width
        sep := strings.Repeat("-", contentWidth)
        
        content = append(content, 
            m.styles.Prompt.Render(header), 
            m.styles.Separator.Render(sep))

        // Format agent rows with same column widths
        for _, a := range agents {
            elapsed := time.Since(a.StartTime).Truncate(time.Second)
            line := fmt.Sprintf("%-*d %-*s %-*s %s",
                issueWidth, a.IssueNumber,
                titleWidth, truncate(a.IssueTitle, titleWidth),
                statusWidth, string(a.Status),
                elapsed)
            content = append(content, line)
        }
    }
    
    m = m.activateOverlay(overlayStatus, "Agent Status", content)
    return m, nil
}
```

### Step 3: Add Helper Functions
```go
// Add max function for width calculations
func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

// Update existing truncate function to handle edge cases
func truncate(s string, max int) string {
    if max <= 0 {
        return ""
    }
    if len(s) <= max {
        return s
    }
    if max <= 3 {
        return s[:max]
    }
    return s[:max-3] + "..."
}
```

## Team Orchestration

**Single Developer Task**: This is a focused fix within a single function that requires:
1. Understanding of Go string formatting and terminal display
2. Familiarity with the existing codebase architecture
3. Testing across different terminal sizes and themes

**No Cross-Team Dependencies**: The change is isolated to table formatting logic and doesn't affect:
- Agent management functionality
- Theme system architecture  
- Overlay rendering system
- Session management

## Step-by-Step Task Breakdown

### Task 1: Implement Dynamic Width Calculation
**Acceptance Criteria:**
- [ ] Add `getOverlayContentWidth()` helper method to model
- [ ] Method returns appropriate width based on overlay calculations
- [ ] Width respects minimum bounds and terminal constraints

### Task 2: Update Table Header Generation  
**Acceptance Criteria:**
- [ ] Replace hardcoded widths with proportional calculations
- [ ] Use ASCII `-` instead of Unicode `─` for separator
- [ ] Maintain visual hierarchy with existing styles
- [ ] Header columns align properly with content rows

### Task 3: Update Agent Row Formatting
**Acceptance Criteria:** 
- [ ] Agent data rows use same column widths as header
- [ ] Title truncation works correctly within calculated width
- [ ] All columns remain aligned regardless of content length
- [ ] No content overflow beyond overlay bounds

### Task 4: Add Utility Functions
**Acceptance Criteria:**
- [ ] Add `max()` function for width calculations  
- [ ] Update `truncate()` to handle edge cases (width <= 3)
- [ ] Functions are properly tested and handle boundary conditions

### Task 5: Integration Testing
**Acceptance Criteria:**
- [ ] Status overlay renders correctly on narrow terminals (< 80 cols)
- [ ] Status overlay renders correctly on wide terminals (> 120 cols) 
- [ ] Table formatting works with all existing themes
- [ ] No rendering artifacts or broken characters
- [ ] Agent data displays correctly with varying content lengths

## Validation Commands

### Build and Test
```bash
# Build the application
go build -o kiro-krew ./cmd/kiro-krew

# Run basic functionality test
./kiro-krew --help
```

### Manual Testing
```bash
# Start kiro-krew in different terminal sizes
./kiro-krew

# In the TUI, test status overlay:
status

# Test with running agents (if available):
# 1. Start some agents first, then check status
# 2. Verify table formatting at different terminal widths
```

### Automated Testing
```bash
# Run existing tests to ensure no regression
go test ./internal/tui/...

# Test theme compatibility
go test ./internal/config/...
```

### Edge Case Testing
1. **Narrow Terminal**: Resize terminal to 40 columns, verify status display
2. **Wide Terminal**: Resize terminal to 150+ columns, verify proportional scaling
3. **No Agents**: Verify warning message displays correctly
4. **Long Titles**: Test with agents having very long issue titles
5. **Theme Switching**: Change themes and verify table colors update correctly

## Risk Assessment

**Low Risk Change**: 
- Isolated to table formatting logic
- No changes to core agent management
- No changes to overlay system architecture
- Maintains existing functionality and information display

**Regression Prevention**:
- Existing theme system unchanged
- Overlay activation/dismissal unchanged  
- All agent data still displayed with same information
- ESC key dismissal still works

**Performance Impact**: Minimal - adds simple width calculations without affecting rendering performance.
