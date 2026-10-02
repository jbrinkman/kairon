# Overlay Popup System Design Specification

**Issue**: #49 - Add overlay popups for status/help/about commands and persistent theme display  
**Closes**: #49  
**Author**: Kiro Architect Agent  
**Date**: 2025-06-03  

## Problem Statement

The current TUI displays command responses for informational commands (status, help, about, theme) inline with console activity, which interrupts the flow of ongoing logs and makes it difficult to distinguish between system activity and command responses. Users need:

1. Visual separation for informational commands via overlay popups
2. Persistent theme display on the command prompt
3. Non-blocking console activity while overlays are open
4. Intuitive overlay dismissal (Escape key)

## Solution Approach

### High-Level Strategy

Implement a layered overlay system within the existing Bubbletea TUI architecture that:
- Maintains the current console flow as the base layer
- Renders overlays on top using Bubbletea's layered rendering capabilities
- Preserves background console activity updates
- Uses modal-style interaction for overlay dismissal
- Enhances the prompt area with persistent theme display

### Architecture Decisions

1. **Overlay State Management**: Extend the existing `model` struct with overlay state fields
2. **View Composition**: Use conditional rendering in the `View()` method to layer overlays over base console
3. **Input Routing**: Route input based on overlay state - Escape dismisses, other keys depend on overlay type
4. **Non-blocking Updates**: Continue processing `tickMsg` and log updates when overlays are active
5. **Theme Integration**: Leverage existing theme system for overlay styling

## Technical Implementation Plan

### Data Structure Changes

#### Model Extension (`internal/tui/tui.go`)

```go
type overlayType int

const (
	overlayNone overlayType = iota
	overlayStatus
	overlayHelp  
	overlayAbout
)

type overlayContent struct {
	title   string
	content []string
}

// Add to existing model struct:
type model struct {
	// ... existing fields ...
	
	// Overlay system
	activeOverlay     overlayType
	overlayContent    overlayContent
	overlayWidth      int
	overlayHeight     int
}
```

#### Style Extensions (`internal/tui/styles.go`)

```go
// Add to existing Styles struct:
type Styles struct {
	// ... existing fields ...
	
	// Overlay styles
	OverlayBorder    lipgloss.Style
	OverlayTitle     lipgloss.Style
	OverlayContent   lipgloss.Style
	OverlayBackground lipgloss.Style
	ThemeLabel       lipgloss.Style
}

// Add overlay style initialization to NewStyles()
func NewStyles(theme *config.Theme) *Styles {
	return &Styles{
		// ... existing styles ...
		
		OverlayBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(theme.Colors.Primary)).
			Padding(1, 2),
		OverlayTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.Primary)).
			Bold(true),
		OverlayContent: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextPrimary)),
		OverlayBackground: lipgloss.NewStyle().
			Background(lipgloss.Color(theme.Colors.Surface)),
		ThemeLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Colors.TextMuted)).
			Italic(true),
	}
}
```

### Core Implementation Changes

#### 1. Update Handler (`internal/tui/tui.go`)

**Key Input Handling Changes:**
```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Handle overlay dismissal first
		if m.activeOverlay != overlayNone {
			if msg.String() == "esc" {
				m.activeOverlay = overlayNone
				return m, nil
			}
			// Block other input when overlay active
			return m, nil
		}
		
		// Existing input handling for base console...
		// ... rest of existing logic
	
	// Continue processing background updates even with overlays
	case tickMsg:
		newLines, newPos := m.readNewLogLines()
		if len(newLines) > 0 {
			m.lastLogPos = newPos
			m = m.appendActivity(newLines...)
		}
		return m, m.tickCmd()
	}
}
```

#### 2. View Composition (`internal/tui/tui.go`)

**Enhanced View Method:**
```go
func (m model) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}

	if m.height == 0 {
		v := tea.NewView(m.input.View())
		v.AltScreen = true
		return v
	}

	// Build base console view (existing logic)
	baseView := m.renderBaseView()
	
	// Add overlay if active
	if m.activeOverlay != overlayNone {
		overlayView := m.renderOverlay()
		baseView = m.layerOverlay(baseView, overlayView)
	}
	
	v := tea.NewView(baseView)
	v.AltScreen = true
	return v
}

func (m model) renderBaseView() string {
	// Extract existing View() logic for base console
	activityHeight := m.height - 2
	// ... existing activity rendering ...
	
	// Enhanced prompt with theme display
	promptText := m.input.View()
	themeLabel := m.styles.ThemeLabel.Render(fmt.Sprintf("theme: %s", m.config.Theme))
	
	// Right-align theme label
	promptWidth := m.width - lipgloss.Width(themeLabel) - 1
	if promptWidth < 20 {
		promptWidth = 20
	}
	
	prompt := lipgloss.JoinHorizontal(lipgloss.Top,
		m.styles.Prompt.Width(promptWidth).Render(promptText),
		themeLabel,
	)
	
	return m.styles.Activity.Render(activity) + "\n" + 
		   m.styles.Separator.Render(strings.Repeat("─", m.width)) + "\n" + 
		   prompt
}
```

#### 3. Overlay Rendering System (`internal/tui/tui.go`)

```go
func (m model) renderOverlay() string {
	// Calculate overlay dimensions (60% of screen, centered)
	m.overlayWidth = int(float64(m.width) * 0.6)
	m.overlayHeight = int(float64(m.height) * 0.6)
	
	if m.overlayWidth < 40 {
		m.overlayWidth = 40
	}
	if m.overlayHeight < 10 {
		m.overlayHeight = 10
	}
	
	// Create overlay content
	title := m.styles.OverlayTitle.Render(m.overlayContent.title)
	
	contentHeight := m.overlayHeight - 4 // Account for border + title + padding
	content := m.overlayContent.content
	if len(content) > contentHeight {
		content = content[len(content)-contentHeight:]
	}
	
	// Pad content to fill overlay
	for len(content) < contentHeight {
		content = append(content, "")
	}
	
	contentStr := strings.Join(content, "\n")
	
	// Apply overlay styling
	overlayContent := lipgloss.JoinVertical(lipgloss.Left, title, "", contentStr)
	
	return m.styles.OverlayBorder.
		Width(m.overlayWidth-4). // Account for border
		Height(m.overlayHeight-2).
		Render(m.styles.OverlayBackground.Render(overlayContent))
}

func (m model) layerOverlay(base, overlay string) string {
	// Center overlay on base view
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	
	startRow := (m.height - len(overlayLines)) / 2
	startCol := (m.width - m.overlayWidth) / 2
	
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}
	
	// Overlay the content
	result := make([]string, len(baseLines))
	copy(result, baseLines)
	
	for i, overlayLine := range overlayLines {
		targetRow := startRow + i
		if targetRow >= 0 && targetRow < len(result) {
			baseLine := result[targetRow]
			if len(baseLine) < startCol {
				baseLine += strings.Repeat(" ", startCol-len(baseLine))
			}
			
			beforeOverlay := baseLine[:startCol]
			afterOverlay := ""
			if len(baseLine) > startCol+lipgloss.Width(overlayLine) {
				afterOverlay = baseLine[startCol+lipgloss.Width(overlayLine):]
			}
			
			result[targetRow] = beforeOverlay + overlayLine + afterOverlay
		}
	}
	
	return strings.Join(result, "\n")
}
```

#### 4. Command Handler Updates (`internal/tui/commands.go`)

**Replace Inline Responses with Overlay Activation:**

```go
func (m model) handleStatus() (model, tea.Cmd) {
	agents := m.manager.List()
	
	var content []string
	if len(agents) == 0 {
		content = []string{"No agents currently running"}
	} else {
		header := fmt.Sprintf("%-8s %-30s %-10s %s", "Issue", "Title", "Status", "Elapsed")
		sep := strings.Repeat("─", 70)
		content = []string{header, sep}
		
		for _, a := range agents {
			elapsed := time.Since(a.StartTime).Truncate(time.Second)
			line := fmt.Sprintf("%-8d %-30s %-10s %s",
				a.IssueNumber,
				truncate(a.IssueTitle, 30),
				string(a.Status),
				elapsed)
			content = append(content, line)
		}
	}
	
	m.activeOverlay = overlayStatus
	m.overlayContent = overlayContent{
		title:   "Agent Status",
		content: content,
	}
	
	return m, nil
}

func (m model) handleHelp() (model, tea.Cmd) {
	help := []string{
		"Available commands:",
		"",
		"  watch start    - Start watching for labeled issues",
		"  watch stop     - Stop watching", 
		"  status         - List all agents with details",
		"  stop <issue>   - Stop agent for specific issue number",
		"  plan [desc]    - Start interactive planning session",
		"  theme          - Show current theme",
		"  theme <name>   - Switch to theme",
		"  about          - Show version information and check for updates",
		"  exit           - Exit (Ctrl+C also works)",
		"  help           - Show this help message",
		"",
		"Press ESC to close this overlay",
	}
	
	m.activeOverlay = overlayHelp
	m.overlayContent = overlayContent{
		title:   "Help",
		content: help,
	}
	
	return m, nil
}

func (m model) handleAbout() (model, tea.Cmd) {
	info := version.Info()
	
	about := []string{
		"Kiro-Krew Version Information:",
		"",
		fmt.Sprintf("Version:    %s", info["version"]),
		fmt.Sprintf("Build Date: %s", info["build_date"]),
		fmt.Sprintf("Go Version: %s", info["go_version"]),
		fmt.Sprintf("Arch:       %s", info["arch"]),
		"",
		"Checking for updates...",
		"",
		"Press ESC to close this overlay",
	}
	
	m.activeOverlay = overlayAbout
	m.overlayContent = overlayContent{
		title:   "About Kiro-Krew",
		content: about,
	}
	
	// Still trigger update check, but handle response differently
	return m, checkForUpdateCmd()
}

func (m model) handleTheme(args []string) (model, tea.Cmd) {
	if len(args) == 0 {
		// Theme info now shown in persistent display - no longer show overlay
		return m, nil
	}
	
	// Theme switching logic remains the same
	// ... existing theme switching implementation
}
```

**Enhanced Update Check Handler:**

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case updateCheckMsg:
		// If about overlay is active, update its content
		if m.activeOverlay == overlayAbout {
			content := m.overlayContent.content
			
			// Remove "Checking for updates..." line
			for i, line := range content {
				if strings.Contains(line, "Checking for updates") {
					content[i] = ""
					break
				}
			}
			
			// Add update status
			if msg.err != nil {
				content = append(content, 
					"Update Status: Unable to check for updates",
					fmt.Sprintf("Error: %v", msg.err))
			} else {
				// Existing version comparison logic
				// Add results to content instead of activityLines
			}
			
			m.overlayContent.content = content
		}
		return m, nil
	}
	// ... rest of existing update logic
}
```

## Team Orchestration

### Development Phases

1. **Phase 1: Overlay Infrastructure** (Builder Agent)
   - Extend model struct with overlay state
   - Implement base overlay rendering system
   - Add overlay style definitions

2. **Phase 2: Command Integration** (Builder Agent)  
   - Update status, help, about command handlers
   - Remove inline response logic
   - Implement overlay content generation

3. **Phase 3: Input Handling** (Builder Agent)
   - Add overlay-aware input routing
   - Implement Escape key dismissal
   - Ensure background activity continues

4. **Phase 4: Prompt Enhancement** (Builder Agent)
   - Add persistent theme display to prompt
   - Implement right-aligned theme label
   - Update theme command behavior

5. **Phase 5: Integration Testing** (Validator Agent)
   - Test overlay rendering across terminal sizes
   - Verify background activity continues
   - Validate Escape key functionality
   - Test theme display persistence

## Step-by-Step Task Breakdown

### Task 1: Overlay System Infrastructure
**Acceptance Criteria:**
- [ ] Model struct extended with overlay state fields (`activeOverlay`, `overlayContent`, etc.)
- [ ] `overlayType` enum defined with None, Status, Help, About variants  
- [ ] `overlayContent` struct created for title and content storage
- [ ] Overlay styles added to `Styles` struct and `NewStyles()` function
- [ ] Base overlay rendering functions implemented (`renderOverlay()`, `layerOverlay()`)

### Task 2: Input Handling Enhancement  
**Acceptance Criteria:**
- [ ] Input routing checks overlay state before processing commands
- [ ] Escape key dismisses active overlay and returns to normal mode
- [ ] Other keys are blocked when overlay is active
- [ ] Background `tickMsg` processing continues during overlay display
- [ ] Console activity updates continue while overlays are shown

### Task 3: Command Handler Conversion
**Acceptance Criteria:**  
- [ ] `handleStatus()` activates status overlay instead of appending to activity
- [ ] `handleHelp()` activates help overlay instead of appending to activity
- [ ] `handleAbout()` activates about overlay instead of appending to activity
- [ ] `handleTheme()` no longer shows overlay for current theme (persistent display handles this)
- [ ] Update check results update about overlay content when active
- [ ] All overlays include "Press ESC to close" instruction

### Task 4: View System Refactoring
**Acceptance Criteria:**
- [ ] `View()` method split into `renderBaseView()` and overlay composition
- [ ] Base console view renders normally when no overlay active
- [ ] Overlay is properly centered and sized (60% of screen dimensions)
- [ ] Overlay content is truncated/scrolled to fit overlay bounds
- [ ] Layered rendering preserves base console content visibility around overlay

### Task 5: Persistent Theme Display
**Acceptance Criteria:**  
- [ ] Command prompt enhanced with right-aligned theme label
- [ ] Theme label uses muted styling from theme system
- [ ] Theme label displays current theme name (e.g., "theme: default")
- [ ] Prompt text adjusts width to accommodate theme label
- [ ] Theme label remains visible during all console activity
- [ ] Layout gracefully handles narrow terminal widths

### Task 6: Integration and Polish
**Acceptance Criteria:**
- [ ] All overlay styles use theme system colors consistently
- [ ] Overlay borders and backgrounds provide clear visual separation
- [ ] Console activity scrolling continues normally with overlays displayed
- [ ] Terminal resize events properly recalculate overlay positioning
- [ ] Memory usage remains stable with frequent overlay usage
- [ ] No visual artifacts or rendering glitches during overlay transitions

## Validation Commands

### Build and Basic Functionality
```bash
# Verify build succeeds
go build ./cmd/kiro-krew

# Start application in test mode
./kiro-krew
```

### Overlay System Testing
```bash
# In kiro-krew TUI, test each overlay command:
help
status  
about

# Verify each overlay:
# 1. Displays as popup over console
# 2. Shows appropriate content
# 3. Dismisses with Escape key
# 4. Console activity continues in background
```

### Theme Display Testing  
```bash
# In kiro-krew TUI, verify theme display:
# 1. Theme name appears on right side of prompt
# 2. Uses muted styling
# 3. Persists during console activity
# 4. Updates when theme is changed

theme light
# Verify theme label updates to "theme: light"

theme default  
# Verify theme label reverts to "theme: default"
```

### Integration Testing
```bash
# Test overlay behavior during active console activity:
watch start  # Start background activity
help         # Open overlay
# Verify console continues updating behind overlay
# Press Escape to dismiss
status       # Test another overlay
# Verify console activity never stopped
```

### Responsive Layout Testing
```bash
# Test various terminal sizes:
# 1. Very narrow (< 40 cols) - overlay should use minimum width
# 2. Very short (< 10 rows) - overlay should use minimum height  
# 3. Standard sizes - overlay should be 60% of screen dimensions
# 4. Theme label should handle narrow prompts gracefully
```

## Risk Mitigation

### Performance Considerations
- Overlay rendering uses string composition, not pixel-level graphics
- Background activity processing unchanged to maintain responsiveness
- Overlay state adds minimal memory overhead

### Compatibility Considerations  
- All changes maintain existing Bubbletea architecture patterns
- No breaking changes to command interface or configuration
- Theme system integration uses existing theme loading infrastructure

### User Experience Considerations
- Overlays provide clear visual hierarchy with borders and backgrounds
- Escape key follows standard modal dialog conventions
- Persistent theme display doesn't interfere with normal prompt usage
- Console activity continues to show system status during information display
