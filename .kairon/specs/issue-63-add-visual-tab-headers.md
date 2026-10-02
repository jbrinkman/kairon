# Design Specification: Add Visual Tab Headers to TUI Interface

**Issue:** #63 - Add visual tab headers to TUI interface
**Closes:** #63

## Problem Statement

The current kiro-krew TUI has a fully functional tab system with keyboard navigation, but users cannot visually see which tabs are available or which tab is currently active. Users must cycle through tabs using `[` and `]` keys without knowing what tabs exist or their current position.

## Solution Approach

Add visual tab headers at the top of the TUI interface following the bubbletea tabs pattern. The solution will:

1. **Render tab headers horizontally** at the top of the screen
2. **Support mouse clicks** for tab switching
3. **Show close buttons** (×) for closable tabs
4. **Handle terminal width constraints** with truncation/scrolling
5. **Integrate seamlessly** with existing TabManager and keyboard navigation
6. **Use consistent lipgloss styling** with the current theme system

### Architecture Decision: Header Component

Create a new `TabHeaders` component that:
- Renders above the main content area
- Takes input from `TabManager` state
- Handles mouse events for tab switching and closing
- Manages responsive layout for narrow terminals
- Integrates with existing styles system

## Relevant Files

### Files to Create:
- `internal/tui/tab_headers.go` - New TabHeaders component

### Files to Modify:
- `internal/tui/tui.go` - Integrate TabHeaders into main view rendering and mouse event handling
- `internal/tui/styles.go` - Add tab header styles (active, inactive, close button)

### Files Referenced (No Changes):
- `internal/tui/tab_manager.go` - Existing tab management functionality
- `internal/tui/tabs.go` - Tab interface definition
- `internal/tui/main_tab.go` - Main tab implementation
- `internal/tui/agent_tab.go` - Agent tab implementation

## Team Orchestration

This is a single-component feature that requires:
1. **UI Component Development** - Create TabHeaders component with rendering logic
2. **Integration Work** - Wire into existing TUI model and event handling
3. **Styling** - Extend theme system with tab-specific styles
4. **Testing** - Verify mouse and keyboard interactions work together

No external team coordination required - all changes are within the TUI package.

## Step-by-Step Task Breakdown

### Task 1: Create Tab Header Styles
**Acceptance Criteria:**
- Add `ActiveTab`, `InactiveTab`, and `CloseButton` styles to Styles struct
- Follow bubbletea tabs example border patterns
- Use theme colors for consistency
- Support both light and dark themes

### Task 2: Implement TabHeaders Component
**Acceptance Criteria:**
- Create `TabHeaders` struct with `Render()` method
- Handle mouse click events for tab switching
- Show close buttons (×) only for closable tabs
- Truncate long tab titles to fit terminal width
- Return consistent height (2 lines: tabs + border)

### Task 3: Integrate Mouse Event Handling
**Acceptance Criteria:**
- Add mouse click detection in TUI Update() method
- Map click coordinates to tab indices
- Handle close button clicks separately from tab clicks
- Preserve existing keyboard shortcuts ([/], F2, Ctrl+W)

### Task 4: Modify Main View Rendering
**Acceptance Criteria:**
- Render tab headers at top of screen
- Reduce content area height by tab header height
- Maintain overlay positioning above tab headers
- Handle terminal resize events correctly

### Task 5: Handle Terminal Width Constraints
**Acceptance Criteria:**
- Truncate tab titles when terminal is too narrow
- Show scroll indicators (‹ ›) when not all tabs fit
- Ensure minimum tab width (8 characters)
- Handle single-tab case gracefully

### Task 6: Integration Testing
**Acceptance Criteria:**
- Test all mouse interactions (tab click, close button)
- Verify keyboard shortcuts still work
- Test terminal resizing behavior
- Verify styling consistency across themes

## Technical Implementation Details

### TabHeaders Component Structure:
```go
type TabHeaders struct {
    tabs       []Tab
    activeTab  int
    width      int
    styles     *Styles
}

func (th *TabHeaders) Render() string
func (th *TabHeaders) HandleClick(x, y int) (action string, tabIndex int)
func (th *TabHeaders) Update(tabs []Tab, activeTab int, width int)
```

### Mouse Event Integration:
- Extend `tea.MouseClickMsg` handling in TUI Update()
- Check if click is in tab header region (top 2 lines)
- Delegate to TabHeaders.HandleClick() for action determination
- Execute tab switch or close based on returned action

### Styling Pattern:
- Follow bubbletea example with connected borders
- Active tab: highlighted with no bottom border
- Inactive tabs: muted with full border
- Close button: positioned at right edge of closable tabs

### Width Handling Strategy:
- Calculate available width for tabs (screen width - padding)
- Distribute width evenly among visible tabs
- Show scroll indicators when tabs exceed available width
- Minimum tab width: 8 characters (title + close button)

## Validation Commands

```bash
# Build and test
go build ./cmd/kiro-krew
go test ./internal/tui/...

# Manual testing scenarios:
./kiro-krew  # Start with main tab
# - Verify tab header shows "Main TUI" tab
# - Start agent to create second tab
# - Click between tabs to verify switching
# - Click × button on agent tab to close
# - Test keyboard shortcuts still work: [, ], F2, Ctrl+W
# - Resize terminal to test width constraints
```

## Risk Assessment

**Low Risk Changes:**
- Adding new TabHeaders component
- Extending styles system
- Mouse event handling (additive)

**Medium Risk Changes:**
- Modifying main View() rendering logic
- Changing content area height calculation

**Mitigation Strategies:**
- Preserve all existing keyboard functionality
- Maintain backward compatibility with existing tab interface
- Test thoroughly with different terminal sizes
- Ensure graceful degradation if mouse events fail

## Dependencies

**Internal Dependencies:**
- Existing TabManager API (read-only access)
- Current Styles/theme system
- bubbletea mouse event system
- lipgloss styling library

**External Dependencies:**
- No new external dependencies required
- Leverages existing charm.land/lipgloss and bubbletea packages
