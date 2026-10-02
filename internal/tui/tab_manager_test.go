package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/config"
)

func TestTabManager(t *testing.T) {
	tm := NewTabManager()

	// Test empty state
	if activeTab := tm.GetActiveTab(); activeTab != nil {
		t.Error("Expected no active tab in empty manager")
	}

	// Test adding main tab
	mainTab := NewMainTab()
	tm.AddTab(mainTab)

	if len(tm.GetTabs()) != 1 {
		t.Errorf("Expected 1 tab, got %d", len(tm.GetTabs()))
	}

	activeTab := tm.GetActiveTab()
	if activeTab == nil {
		t.Error("Expected active tab after adding main tab")
	}

	if activeTab.ID() != "main" {
		t.Errorf("Expected main tab ID 'main', got %s", activeTab.ID())
	}

	if activeTab.Type() != TabTypeMain {
		t.Errorf("Expected TabTypeMain, got %v", activeTab.Type())
	}

	if activeTab.IsClosable() {
		t.Error("Expected main tab to not be closable")
	}

	// Test tab removal (should fail for non-closable main tab)
	removed := tm.RemoveTab("main")
	if removed {
		t.Error("Should not be able to remove non-closable main tab")
	}

	if len(tm.GetTabs()) != 1 {
		t.Errorf("Expected 1 tab after failed removal, got %d", len(tm.GetTabs()))
	}
}

func TestMainTab(t *testing.T) {
	mainTab := NewMainTab()

	if mainTab.ID() != "main" {
		t.Errorf("Expected ID 'main', got %s", mainTab.ID())
	}

	if mainTab.Type() != TabTypeMain {
		t.Errorf("Expected TabTypeMain, got %v", mainTab.Type())
	}

	if mainTab.Title() != "Main TUI" {
		t.Errorf("Expected title 'Main TUI', got %s", mainTab.Title())
	}

	if mainTab.IsClosable() {
		t.Error("Main tab should not be closable")
	}

	// Test setting base view
	testView := "test view content"
	mainTab.SetBaseView(testView)

	if mainTab.View() != testView {
		t.Errorf("Expected view content '%s', got '%s'", testView, mainTab.View())
	}

	// Test resize
	mainTab.Resize(80, 24)
	// Should not error or panic
}

func TestTabNavigation(t *testing.T) {
	tm := NewTabManager()

	// Add some tabs
	mainTab := NewMainTab()
	// For testing, create mock agent tabs
	mockAgentTab1 := &mockAgentTab{id: "agent-agent1", agentID: "agent1"}
	mockAgentTab2 := &mockAgentTab{id: "agent-agent2", agentID: "agent2"}

	tm.AddTab(mainTab)
	tm.AddTab(mockAgentTab1)
	tm.AddTab(mockAgentTab2)

	// Test NextTab
	tm.NextTab()
	if tm.activeTab != 1 {
		t.Errorf("Expected active tab 1, got %d", tm.activeTab)
	}

	// Test PreviousTab
	tm.PreviousTab()
	if tm.activeTab != 0 {
		t.Errorf("Expected active tab 0, got %d", tm.activeTab)
	}

	// Test wrap around
	tm.PreviousTab()
	if tm.activeTab != 2 {
		t.Errorf("Expected active tab 2 (wrap around), got %d", tm.activeTab)
	}

	// Test FindTabByAgentID
	idx := tm.FindTabByAgentID("agent1")
	if idx != 1 {
		t.Errorf("Expected to find agent1 at index 1, got %d", idx)
	}

	idx = tm.FindTabByAgentID("nonexistent")
	if idx != -1 {
		t.Errorf("Expected -1 for nonexistent agent, got %d", idx)
	}

	// Test CloseTab
	success := tm.CloseTab(1) // Close agent1
	if !success {
		t.Error("Expected to successfully close tab")
	}

	if len(tm.tabs) != 2 {
		t.Errorf("Expected 2 tabs after closing, got %d", len(tm.tabs))
	}

	// Active tab should adjust
	if tm.activeTab != 1 { // Should now point to agent2
		t.Errorf("Expected active tab 1 after closing, got %d", tm.activeTab)
	}

	// Test CloseTabByID
	success = tm.CloseTabByID("agent-agent2")
	if !success {
		t.Error("Expected to successfully close tab by ID")
	}

	if len(tm.tabs) != 1 {
		t.Errorf("Expected 1 tab after closing by ID, got %d", len(tm.tabs))
	}
}

// Mock agent tab for testing
type mockAgentTab struct {
	id      string
	agentID string
}

func (m *mockAgentTab) ID() string                                   { return m.id }
func (m *mockAgentTab) Type() TabType                                { return TabTypeAgent }
func (m *mockAgentTab) Title() string                                { return "Agent " + m.agentID }
func (m *mockAgentTab) IsClosable() bool                             { return true }
func (m *mockAgentTab) View() string                                 { return "" }
func (m *mockAgentTab) Update(msg tea.Msg) (Tab, tea.Cmd)            { return m, nil }
func (m *mockAgentTab) Resize(width, height int)                     {}
func (m *mockAgentTab) CaptureFocusState() FocusTarget               { return FocusTargetFooter }
func (m *mockAgentTab) RestoreFocusState(target FocusTarget) tea.Cmd { return nil }

func TestTabToggle(t *testing.T) {
	tm := NewTabManager()
	mainTab := NewMainTab()
	tm.AddTab(mainTab)

	// With only main tab, toggle should do nothing
	tm.ToggleView()
	if tm.activeTab != 0 {
		t.Errorf("Expected active tab 0, got %d", tm.activeTab)
	}

	// Test with no agent tabs - toggle should still work
	tm.SetActiveTab(0)
	tm.ToggleView()
	if tm.activeTab != 0 {
		t.Errorf("Expected active tab to remain 0 with no agent tabs, got %d", tm.activeTab)
	}
}

func TestRenderTabHeaders(t *testing.T) {
	tm := NewTabManager()

	// Create a default theme for testing
	theme := &config.Theme{}
	theme.Colors.Primary = "#00FF00"
	theme.Colors.TextPrimary = "#FFFFFF"
	theme.Colors.TextMuted = "#888888"
	theme.Colors.Warning = "#FFFF00"
	theme.Colors.Surface = "#333333"
	theme.Colors.Separator = "#CCCCCC"

	styles := NewStyles(theme)

	// Test empty tab manager
	result := tm.RenderTabHeaders(80, styles)
	if result != "" {
		t.Errorf("Expected empty string for no tabs, got '%s'", result)
	}

	// Add main tab
	mainTab := NewMainTab()
	tm.AddTab(mainTab)

	result = tm.RenderTabHeaders(80, styles)
	if !strings.Contains(result, "Main TUI") {
		t.Errorf("Expected tab header to contain 'Main TUI', got '%s'", result)
	}

	// Add closable agent tab
	mockAgent := &mockAgentTab{id: "agent-test", agentID: "test"}
	tm.AddTab(mockAgent)

	result = tm.RenderTabHeaders(80, styles)
	if !strings.Contains(result, "Main TUI") {
		t.Errorf("Expected tab header to contain 'Main TUI', got '%s'", result)
	}
	if !strings.Contains(result, "Agent test") {
		t.Errorf("Expected tab header to contain 'Agent test', got '%s'", result)
	}
	if !strings.Contains(result, "×") {
		t.Errorf("Expected tab header to contain close button '×', got '%s'", result)
	}

	// Test long title truncation
	longTitleTab := &mockAgentTab{id: "agent-long", agentID: "verylongagentname"}
	tm.AddTab(longTitleTab)

	result = tm.RenderTabHeaders(80, styles)
	// Should truncate long titles
	if strings.Contains(result, "verylongagentname") {
		t.Errorf("Expected long title to be truncated, got '%s'", result)
	}

	// Test width overflow — narrow terminal should not render all tabs
	result = tm.RenderTabHeaders(20, styles)
	// With 20 chars, only the first tab (8 + 2 padding = 10) should fit
	if strings.Contains(result, "Agent test") {
		t.Errorf("Expected narrow width to omit later tabs, got '%s'", result)
	}
}

// tabClickPos calculates the click x-position for a tab at the given index.
// Accounts for padding (tabPadding) and separator width (1) between tabs.
func tabClickPos(tabs []Tab, targetIndex int, inCloseBtn bool) int {
	pos := 0
	for i, tab := range tabs {
		if i >= targetIndex {
			break
		}
		title := tab.Title()
		if len(title) > 15 {
			title = title[:12] + "..."
		}
		w := len(title) + tabPadding
		if tab.IsClosable() {
			w += len(closeBtnText)
		}
		pos += w + 1 // +1 for separator
	}
	if inCloseBtn {
		title := tabs[targetIndex].Title()
		if len(title) > 15 {
			title = title[:12] + "..."
		}
		pos += len(title) + tabPadding // jump past title+padding to close btn area
	} else {
		pos += 1 // click inside the padding/title area
	}
	return pos
}

func TestTabManager_HandleTabHeaderClick(t *testing.T) {
	tm := NewTabManager()

	// Add test tabs
	mainTab := NewMainTab()                                   // "Main TUI" = 8 chars + 2 padding = 10 rendered
	agentTab := &mockAgentTab{id: "agent-1", agentID: "test"} // "Agent test" = 10 chars + 2 padding + 2 close = 14 rendered

	tm.AddTab(mainTab)
	tm.AddTab(agentTab)

	tabs := tm.GetTabs()

	// Test clicking on first tab (inside title area)
	clickPos := tabClickPos(tabs, 0, false)
	tm.HandleTabHeaderClick(clickPos)
	if tm.GetActiveTabIndex() != 0 {
		t.Errorf("Expected active tab 0, got %d", tm.GetActiveTabIndex())
	}

	// Test clicking on second tab content (not close button)
	clickPos = tabClickPos(tabs, 1, false)
	tm.HandleTabHeaderClick(clickPos)
	if tm.GetActiveTabIndex() != 1 {
		t.Errorf("Expected active tab 1, got %d", tm.GetActiveTabIndex())
	}

	// Test clicking on close button area of second tab
	initialCount := len(tm.GetTabs())
	clickPos = tabClickPos(tabs, 1, true)
	tm.HandleTabHeaderClick(clickPos)
	if len(tm.GetTabs()) >= initialCount {
		t.Error("Expected closable tab to be closed when clicking close button")
	}
}
