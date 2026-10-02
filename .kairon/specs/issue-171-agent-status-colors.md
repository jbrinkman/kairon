# Design Specification: Add Agent Status Colors to Tab Display

**Issue**: #171  
**Title**: Add agent status colors to tab display  
**Closes**: #171

## Solution Approach

The implementation will add visual feedback for agent completion status by introducing new theme colors and integrating them into the tab rendering system. This uses the existing status change notification mechanism (`m.statusGen.Add(1)`) to trigger UI updates when agent status changes occur.

The approach leverages:
1. **Theme System Extension**: Add `agent_success` and `agent_fail` colors to all themes
2. **Styles System Integration**: Expose new colors through existing `Styles` struct
3. **Tab Status Tracking**: Enhance tab rendering to use agent status for color selection
4. **Status Change Integration**: Use existing `statusGen` atomic counter to trigger tab updates

## Relevant Files

### Files to be modified:
- `.kiro-krew/themes/default.yaml` - Add agent status colors
- `.kiro-krew/themes/light.yaml` - Add agent status colors  
- `.kiro-krew/themes/high-contrast.yaml` - Add agent status colors
- `internal/config/themes.go` - Add validation for new color fields
- `internal/tui/styles.go` - Add new style definitions for agent status
- `internal/tui/tab_manager.go` - Enhance tab rendering with status-based colors
- `internal/tui/agent_tab.go` - Add status lookup method for tabs

## Team Orchestration

This is a focused UI enhancement that can be implemented in a single session:
- **Theme updates** first to establish color definitions
- **Backend changes** to expose colors through styles system
- **Frontend rendering** to apply colors based on agent status
- **Integration testing** to verify status changes trigger visual updates

## Step-by-Step Task Breakdown

### Task 1: Update Theme Definitions
**Acceptance Criteria:**
- All three theme files contain `agent_success` and `agent_fail` color definitions
- Colors are visually distinct and accessibility-compliant for each theme
- Default theme uses green for success, red for failure
- Light theme uses darker colors appropriate for light backgrounds
- High-contrast theme uses bright, high-contrast colors

**Implementation:**
```yaml
# Add to colors section of each theme:
agent_success: "#00AA00"    # Default theme
agent_fail: "#FF0000"       # Default theme
```

### Task 2: Extend Theme Validation
**Acceptance Criteria:**
- Theme validation recognizes `agent_success` and `agent_fail` as valid fields
- Invalid color values are properly rejected
- Themes without these fields continue to work (backwards compatibility)

**Implementation:**
- Update `validateTheme()` function in `internal/config/themes.go`
- Add new fields to `colorFields` map for validation

### Task 3: Update Theme Structure
**Acceptance Criteria:**
- Theme struct includes new agent status color fields
- Default theme fallback includes sensible status colors

**Implementation:**
- Add `AgentSuccess` and `AgentFail` fields to Theme.Colors struct
- Update `getDefaultTheme()` function with default values

### Task 4: Extend Styles System  
**Acceptance Criteria:**
- `Styles` struct exposes `AgentSuccess` and `AgentFail` lipgloss styles
- Styles are created from theme colors in `NewStyles()` function
- Backwards compatibility maintained for themes without agent colors

**Implementation:**
```go
// Add to Styles struct:
AgentSuccess lipgloss.Style
AgentFail    lipgloss.Style

// Add to NewStyles() function:
AgentSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.AgentSuccess)),
AgentFail:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Colors.AgentFail)),
```

### Task 5: Add Agent Status Lookup Method
**Acceptance Criteria:**
- Agent tab can determine its associated agent's current status
- Method handles cases where agent no longer exists
- Returns appropriate status for tab rendering

**Implementation:**
- Add `GetStatus()` method to `AgentTab` struct
- Method queries agent manager for current agent status
- Returns `StatusCompleted`, `StatusFailed`, or `StatusRunning`

### Task 6: Enhance Tab Header Rendering
**Acceptance Criteria:**
- Tab title color reflects agent completion status (success/fail/running)
- Only agent tabs use status-based coloring (main tab unchanged)
- Status changes are reflected immediately in tab display
- Hover and active states work correctly with status colors

**Implementation:**
- Modify `RenderTabHeaders()` method in `tab_manager.go`
- Add status-based style selection for agent tabs
- Preserve existing styling logic for non-agent tabs
- Use agent status to determine base tab style before applying active/hover states

### Task 7: Integration with Status Changes
**Acceptance Criteria:**
- Tab colors update automatically when agent status changes
- Uses existing `statusGen.Add(1)` mechanism in agent manager
- No additional polling or performance overhead
- Visual feedback appears within 200ms of status change (existing tick interval)

**Implementation:**
- No changes needed - existing `updateAgentTabs()` already responds to `statusGen` changes
- Tab rendering will automatically use new status-based colors
- Existing 200ms tick ensures responsive updates

## Validation Commands

### Theme Validation
```bash
# Verify theme files are valid YAML
yq eval '.colors.agent_success' .kiro-krew/themes/default.yaml
yq eval '.colors.agent_fail' .kiro-krew/themes/default.yaml

# Test theme loading
go run ./cmd/kiro-krew init  # Should not error on theme validation
```

### UI Testing
```bash
# Start kiro-krew and verify tab colors
go run ./cmd/kiro-krew

# In TUI:
# 1. Start watcher with labeled GitHub issues
# 2. Observe agent tabs are created with default colors (running)
# 3. Wait for agent completion and verify color changes to success/fail
# 4. Test theme switching with 'theme' command
# 5. Verify status colors change appropriately with different themes
```

### Integration Testing
```bash
# Test status change integration
go test ./internal/tui -v -run TestTabColorUpdates
go test ./internal/config -v -run TestThemeValidation
```

## Color Specifications

### Default Theme (Dark)
- `agent_success`: `#00AA00` (bright green)
- `agent_fail`: `#FF0000` (bright red)

### Light Theme  
- `agent_success`: `#007700` (dark green for contrast)
- `agent_fail`: `#CC0000` (dark red for contrast)

### High-Contrast Theme
- `agent_success`: `#00FF00` (bright green)
- `agent_fail`: `#FF0044` (bright red)

## Implementation Notes

- **Minimal Change Approach**: Leverages existing architecture patterns
- **Backwards Compatibility**: Themes without agent colors continue working
- **Performance**: No additional polling - uses existing status generation counter
- **Accessibility**: Colors chosen for visibility and contrast in each theme
- **Future Extension**: Pattern supports additional agent states if needed

## Technical Details

The implementation relies on the existing status notification system in `agent.Manager`:
1. Agent status changes call `m.statusGen.Add(1)`
2. TUI's 200ms tick checks for status generation changes
3. `updateAgentTabs()` is called on any status changes
4. Tab rendering uses agent status to select appropriate colors

This ensures immediate visual feedback without additional performance overhead.