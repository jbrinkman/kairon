package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/session"
)

// TabManager manages the lifecycle and state of all tabs
type TabManager struct {
	tabs          []Tab
	activeTab     int
	hoveredTab    int
	width         int
	height        int
	contentHeight int // height allocated to tab content (excludes footer + header)

	// Planning tab management
	planningTabCounter int // Counter for unique planning tab IDs

	// Session management
	sessionManager *session.SessionManager
}

// NewTabManager creates a new tab manager
func NewTabManager() *TabManager {
	return &TabManager{
		tabs:               make([]Tab, 0),
		activeTab:          0,
		hoveredTab:         -1,
		planningTabCounter: 0,
		sessionManager:     session.NewSessionManager(),
	}
}

// AddTab adds a new tab to the manager
func (tm *TabManager) AddTab(tab Tab) {
	tm.tabs = append(tm.tabs, tab)
	if tm.width > 0 && tm.contentHeight > 0 {
		tab.Resize(tm.width, tm.contentHeight)
	} else if tm.width > 0 && tm.height > 0 {
		tab.Resize(tm.width, tm.height)
	}
	// If this is the first tab, set it as active
	if len(tm.tabs) == 1 {
		tm.activeTab = 0
	}
}

// RemoveTab removes a tab by ID if it's closable
func (tm *TabManager) RemoveTab(id string) bool {
	for i, tab := range tm.tabs {
		if tab.ID() == id && tab.IsClosable() {
			tm.tabs = append(tm.tabs[:i], tm.tabs[i+1:]...)
			if tm.activeTab >= len(tm.tabs) && len(tm.tabs) > 0 {
				tm.activeTab = len(tm.tabs) - 1
			}
			return true
		}
	}
	return false
}

// SetActiveTab sets the active tab by index
func (tm *TabManager) SetActiveTab(index int) {
	if index >= 0 && index < len(tm.tabs) {
		tm.activeTab = index
	}
}

// GetActiveTab returns the currently active tab
func (tm *TabManager) GetActiveTab() Tab {
	if len(tm.tabs) == 0 || tm.activeTab >= len(tm.tabs) {
		return nil
	}
	return tm.tabs[tm.activeTab]
}

// GetTabs returns all tabs
func (tm *TabManager) GetTabs() []Tab {
	return tm.tabs
}

// Update forwards update messages to the active tab
func (tm *TabManager) Update(msg tea.Msg) tea.Cmd {
	if activeTab := tm.GetActiveTab(); activeTab != nil {
		updated, cmd := activeTab.Update(msg)
		if updated != nil {
			tm.tabs[tm.activeTab] = updated
		}
		return cmd
	}
	return nil
}

// ResizeForFooter resizes all tabs accounting for footer height
func (tm *TabManager) ResizeForFooter(width, height int, footerHeight int) {
	tm.width = width
	tm.height = height
	// Calculate content area height excluding tab header and footer
	contentHeight := height - footerHeight - tabHeaderHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	tm.contentHeight = contentHeight

	// Resize tabs with adjusted content height
	for _, tab := range tm.tabs {
		tab.Resize(width, contentHeight)
	}
}

// RenderCurrentView renders the active tab's view
func (tm *TabManager) RenderCurrentView() string {
	if activeTab := tm.GetActiveTab(); activeTab != nil {
		return activeTab.View()
	}
	return ""
}

// NextTab switches to the next tab
func (tm *TabManager) NextTab() {
	if len(tm.tabs) > 1 {
		tm.activeTab = (tm.activeTab + 1) % len(tm.tabs)
	}
}

// PreviousTab switches to the previous tab
func (tm *TabManager) PreviousTab() {
	if len(tm.tabs) > 1 {
		tm.activeTab = (tm.activeTab - 1 + len(tm.tabs)) % len(tm.tabs)
	}
}

// FindTabByAgentID finds tab index by agent ID
func (tm *TabManager) FindTabByAgentID(agentID string) int {
	for i, tab := range tm.tabs {
		if tab.Type() == TabTypeAgent && tab.ID() == "agent-"+agentID {
			return i
		}
	}
	return -1
}

// CloseTab closes tab at index
func (tm *TabManager) CloseTab(index int) bool {
	if index < 0 || index >= len(tm.tabs) || !tm.tabs[index].IsClosable() {
		return false
	}

	// Clean up resources for planning tabs before removing
	if planningTab, ok := tm.tabs[index].(*PlanningTab); ok {
		planningTab.Close()
	}

	// Clean up resources for log tabs before removing
	if logTab, ok := tm.tabs[index].(*LogTab); ok {
		logTab.Cleanup()
	}

	tm.tabs = append(tm.tabs[:index], tm.tabs[index+1:]...)
	tm.ClearHover()

	// Maintain active tab index
	if tm.activeTab >= len(tm.tabs) && len(tm.tabs) > 0 {
		tm.activeTab = len(tm.tabs) - 1
	} else if tm.activeTab > index {
		tm.activeTab--
	}

	return true
}

// CloseTabByID closes tab by ID
func (tm *TabManager) CloseTabByID(tabID string) bool {
	for i, tab := range tm.tabs {
		if tab.ID() == tabID {
			return tm.CloseTab(i)
		}
	}
	return false
}

// GetActiveTabIndex returns the index of the currently active tab
func (tm *TabManager) GetActiveTabIndex() int {
	return tm.activeTab
}

// SetHoveredTab updates the hover state to the specified tab index
func (tm *TabManager) SetHoveredTab(index int) {
	if index >= 0 && index < len(tm.tabs) {
		tm.hoveredTab = index
	}
}

// GetHoveredTab returns the current hovered tab index (-1 if no tab is hovered)
func (tm *TabManager) GetHoveredTab() int {
	return tm.hoveredTab
}

// ClearHover resets the hover state to no tab hovered
func (tm *TabManager) ClearHover() {
	tm.hoveredTab = -1
}

// CloseCurrentTab closes the currently active tab if it's closable
func (tm *TabManager) CloseCurrentTab() bool {
	return tm.CloseTab(tm.activeTab)
}

// ToggleView switches between main tab and first agent tab (for F2 compatibility)
func (tm *TabManager) ToggleView() {
	if len(tm.tabs) < 2 {
		return
	}

	// If on main tab, switch to first agent tab
	if tm.activeTab == 0 {
		for i, tab := range tm.tabs {
			if tab.Type() == TabTypeAgent {
				tm.activeTab = i
				return
			}
		}
	} else {
		// Switch back to main tab
		tm.activeTab = 0
	}
}

// tabPadding is the horizontal padding applied to each tab by lipgloss (Padding(0, 1) = 1 char each side)
const tabPadding = 2

// closeBtnText is the close button suffix for closable tabs
const closeBtnText = " ×"

// RenderTabHeaders renders visual tab headers showing all tabs with active highlighting and close buttons.
// The width parameter controls overflow — tabs exceeding width are truncated with an indicator.
func (tm *TabManager) RenderTabHeaders(width int, styles *Styles) string {
	if len(tm.tabs) == 0 {
		return ""
	}

	var tabHeaders []string
	usedWidth := 0
	separatorWidth := 1 // "│"

	for i, tab := range tm.tabs {
		title := tab.Title()

		// Truncate long titles
		if len(title) > 15 {
			title = title[:12] + "..."
		}

		// Calculate rendered width of this tab: title + optional close btn + padding
		renderedWidth := len(title) + tabPadding
		if tab.IsClosable() {
			renderedWidth += len(closeBtnText)
		}

		// Check if adding this tab would overflow terminal width
		needed := renderedWidth
		if len(tabHeaders) > 0 {
			needed += separatorWidth
		}
		if width > 0 && usedWidth+needed > width {
			break
		}

		// Render tab title with unified style logic - no tab-type-specific conditional branching
		var styledTab string
		if i == tm.activeTab {
			styledTab = styles.TabActive.Render(title)
		} else if i == tm.hoveredTab {
			styledTab = styles.TabInactiveHover.Render(title)
		} else {
			styledTab = styles.TabInactive.Render(title)
		}

		if tab.IsClosable() {
			styledTab += styles.TabClose.Render(closeBtnText)
		}

		tabHeaders = append(tabHeaders, styledTab)
		usedWidth += needed
	}

	return strings.Join(tabHeaders, styles.Separator.Render("│"))
}

// Planning Tab Management

// MaxPlanningTabs defines the maximum concurrent planning tabs allowed
const MaxPlanningTabs = 10

// GetPlanningTabCount returns the current number of planning tabs
func (tm *TabManager) GetPlanningTabCount() int {
	count := 0
	for _, tab := range tm.tabs {
		if tab.Type() == TabTypePlanning {
			count++
		}
	}
	return count
}

// CanCreatePlanningTab checks if a new planning tab can be created
func (tm *TabManager) CanCreatePlanningTab() bool {
	return tm.GetPlanningTabCount() < MaxPlanningTabs
}

// CreatePlanningTab creates a new planning tab if within limits
func (tm *TabManager) CreatePlanningTab(styles *Styles, contextTracker *ContextTracker) (*PlanningTab, error) {
	return tm.CreateAndAddPlanningTab(styles, contextTracker, nil)
}

// CreateAndAddPlanningTab creates a new planning tab with session management if within limits
func (tm *TabManager) CreateAndAddPlanningTab(styles *Styles, contextTracker *ContextTracker, sessionManager *session.SessionManager) (*PlanningTab, error) {
	if !tm.CanCreatePlanningTab() {
		return nil, fmt.Errorf("maximum %d concurrent planning tabs reached", MaxPlanningTabs)
	}

	// Use provided session manager or fallback to tab manager's session manager
	if sessionManager == nil {
		sessionManager = tm.sessionManager
	}

	// Generate unique ID and title
	tm.planningTabCounter++
	id := fmt.Sprintf("planning-%d-%d", tm.planningTabCounter, time.Now().Unix())
	title := fmt.Sprintf("Plan %d", tm.planningTabCounter)

	// Create the planning tab (constructor creates ACP client internally)
	planningTab := NewPlanningTabWithSession(id, title, styles, contextTracker, sessionManager, nil)

	// Verify tab creation was successful
	if planningTab == nil {
		return nil, fmt.Errorf("failed to create planning tab instance")
	}

	// Add to manager (don't set as active here - let the caller decide)
	tm.AddTab(planningTab)

	return planningTab, nil
}

// ForceNewPlanningTab creates a new planning tab for subsequent sessions after completion
func (tm *TabManager) ForceNewPlanningTab(styles *Styles, contextTracker *ContextTracker) (*PlanningTab, error) {
	// First try normal creation
	if tm.CanCreatePlanningTab() {
		planningTab, err := tm.CreateAndAddPlanningTab(styles, contextTracker, tm.sessionManager)
		if err != nil {
			return nil, err
		}
		// Set as active tab for forced creation
		tm.SetActiveTab(len(tm.tabs) - 1)
		return planningTab, nil
	}

	// Find and close a completed or failed planning tab to make room
	for i := len(tm.tabs) - 1; i >= 0; i-- {
		if tab := tm.tabs[i]; tab.Type() == TabTypePlanning {
			if planningTab, ok := tab.(*PlanningTab); ok {
				state := planningTab.GetState()
				if state == session.PlanningStateCompleted || state == session.PlanningStateFailed || state == session.PlanningStateReadOnly {
					// Close this tab to make room
					tm.CloseTab(i)
					break
				}
			}
		}
	}

	// Now create the new tab
	planningTab, err := tm.CreateAndAddPlanningTab(styles, contextTracker, tm.sessionManager)
	if err != nil {
		return nil, err
	}

	// Set as active tab for forced creation
	tm.SetActiveTab(len(tm.tabs) - 1)
	return planningTab, nil
}

// Session Management Methods

// CleanupSessionsOnExit performs session cleanup when the application exits
func (tm *TabManager) CleanupSessionsOnExit() error {
	// Get list of active planning tab IDs
	activeTabIDs := make([]string, 0)
	for _, tab := range tm.tabs {
		if tab.Type() == TabTypePlanning {
			if planningTab, ok := tab.(*PlanningTab); ok {
				if planningTab.sessionID != "" {
					activeTabIDs = append(activeTabIDs, planningTab.id)
				}
			}
		}
	}

	// Cleanup orphaned sessions
	orphanedCount, err := tm.sessionManager.CleanupOrphanedPlanningSessions(activeTabIDs)
	if err != nil {
		return fmt.Errorf("failed to cleanup orphaned planning sessions: %w", err)
	}

	// Cleanup old completed sessions (older than 7 days)
	completedCount, err := tm.sessionManager.CleanupCompletedPlanningSessions(7 * 24 * time.Hour)
	if err != nil {
		return fmt.Errorf("failed to cleanup completed planning sessions: %w", err)
	}

	// General session cleanup
	if err := tm.sessionManager.CleanupOnExit(); err != nil {
		return fmt.Errorf("failed to perform general session cleanup: %w", err)
	}

	if orphanedCount > 0 || completedCount > 0 {
		// Session cleanup occurred
	}

	return nil
}

// GetPlanningTabByID finds a planning tab by ID
func (tm *TabManager) GetPlanningTabByID(id string) *PlanningTab {
	for _, tab := range tm.tabs {
		if tab.Type() == TabTypePlanning && tab.ID() == id {
			if planningTab, ok := tab.(*PlanningTab); ok {
				return planningTab
			}
		}
	}
	return nil
}

// GetActivePlanningTabs returns all planning tabs that are currently active (processing)
func (tm *TabManager) GetActivePlanningTabs() []*PlanningTab {
	var activeTabs []*PlanningTab
	for _, tab := range tm.tabs {
		if tab.Type() == TabTypePlanning {
			if planningTab, ok := tab.(*PlanningTab); ok {
				if planningTab.GetState() == session.PlanningStateActive {
					activeTabs = append(activeTabs, planningTab)
				}
			}
		}
	}
	return activeTabs
}

// MarkPlanningTabCompleted marks a planning tab as completed (successful GitHub issue creation)
func (tm *TabManager) MarkPlanningTabCompleted(tabID string) bool {
	if planningTab := tm.GetPlanningTabByID(tabID); planningTab != nil {
		// Set to read-only state to prevent further interaction
		planningTab.SetReadOnly()
		// Note: The color will be handled by RenderTabHeaders based on state
		return true
	}
	return false
}

// MarkPlanningTabFailed marks a planning tab as failed
func (tm *TabManager) MarkPlanningTabFailed(tabID string) bool {
	if planningTab := tm.GetPlanningTabByID(tabID); planningTab != nil {
		planningTab.SetFailed()
		return true
	}
	return false
}

// CleanupCompletedPlanningTabs removes completed/failed planning tabs to free up slots
func (tm *TabManager) CleanupCompletedPlanningTabs() int {
	cleaned := 0
	for i := len(tm.tabs) - 1; i >= 0; i-- {
		if tab := tm.tabs[i]; tab.Type() == TabTypePlanning {
			if planningTab, ok := tab.(*PlanningTab); ok {
				state := planningTab.GetState()
				if state == session.PlanningStateCompleted || state == session.PlanningStateFailed {
					if tm.CloseTab(i) {
						cleaned++
					}
				}
			}
		}
	}
	return cleaned
}

// HasReadOnlyPlanningTabs checks if there are any read-only planning tabs
func (tm *TabManager) HasReadOnlyPlanningTabs() bool {
	for _, tab := range tm.tabs {
		if tab.Type() == TabTypePlanning {
			if planningTab, ok := tab.(*PlanningTab); ok {
				if planningTab.GetState() == session.PlanningStateReadOnly {
					return true
				}
			}
		}
	}
	return false
}

// RestoreOrFocusAgentTab creates a new agent tab if one doesn't exist, or focuses existing tab
func (tm *TabManager) RestoreOrFocusAgentTab(agentID string, manager *agent.Manager, styles *Styles) bool {
	// Check if tab already exists
	existingIndex := tm.FindTabByAgentID(agentID)
	if existingIndex >= 0 {
		tm.SetActiveTab(existingIndex)
		return true
	}

	// Create new agent tab
	agentTab := NewAgentTab(agentID, manager, styles)
	tm.AddTab(agentTab)
	// Set the new tab as active (it will be the last one added)
	tm.SetActiveTab(len(tm.tabs) - 1)
	return true
}

// FindLogTab returns the index of the log viewer tab if it exists, or -1 if not found
func (tm *TabManager) FindLogTab() int {
	for i, tab := range tm.tabs {
		if tab.Type() == TabTypeLog {
			return i
		}
	}
	return -1
}

// GetLogTab returns the log viewer tab if it exists, or nil if not found
func (tm *TabManager) GetLogTab() *LogTab {
	for _, tab := range tm.tabs {
		if tab.Type() == TabTypeLog {
			if logTab, ok := tab.(*LogTab); ok {
				return logTab
			}
		}
	}
	return nil
}

// HasLogTab returns whether a log viewer tab currently exists
func (tm *TabManager) HasLogTab() bool {
	return tm.FindLogTab() >= 0
}

// HandleTabHeaderHover handles mouse hover over tab headers.
// Sets hover state based on mouse position using same logic as HandleTabHeaderClick.
func (tm *TabManager) HandleTabHeaderHover(x int) {
	if len(tm.tabs) == 0 {
		tm.ClearHover()
		return
	}

	position := 0
	separatorWidth := 1 // "│"

	for i, tab := range tm.tabs {
		title := tab.Title()
		if len(title) > 15 {
			title = title[:12] + "..."
		}

		// Calculate tab width including padding and optional close button
		tabWidth := len(title) + tabPadding
		if tab.IsClosable() {
			tabWidth += len(closeBtnText)
		}

		// Check if mouse is within this tab
		if x >= position && x < position+tabWidth {
			tm.SetHoveredTab(i)
			return
		}

		position += tabWidth + separatorWidth
	}

	// Mouse is not over any tab
	tm.ClearHover()
}

// HandleTabHeaderClick handles mouse clicks on tab headers.
// Positions account for lipgloss padding applied during rendering.
func (tm *TabManager) HandleTabHeaderClick(x int) bool {
	if len(tm.tabs) == 0 {
		return false
	}

	position := 0
	separatorWidth := 1 // "│"

	for i, tab := range tm.tabs {
		title := tab.Title()
		if len(title) > 15 {
			title = title[:12] + "..."
		}

		// Each tab has padding (1 char each side) around the title
		tabWidth := len(title) + tabPadding
		closeButtonStart := position + tabWidth

		if tab.IsClosable() {
			tabWidth += len(closeBtnText)
		}

		// Check if click is within this tab
		if x >= position && x < position+tabWidth {
			if tab.IsClosable() && x >= closeButtonStart {
				tm.CloseTab(i)
			} else {
				tm.SetActiveTab(i)
			}
			return true
		}

		position += tabWidth + separatorWidth
	}

	return false
}
