package tui

import (
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/github"
	"github.com/jbrinkman/kairon/internal/incidents"
	"github.com/jbrinkman/kairon/internal/session"
)

type runningWatcher interface {
	Running() bool
}

func watcherIsRunning(w any) bool {
	rw, ok := w.(runningWatcher)
	return ok && rw.Running()
}

func (m model) handleWatch(action string) (model, tea.Cmd) {
	switch strings.ToLower(action) {
	case "start":
		if watcherIsRunning(any(m.watcher)) {
			m = m.appendActivity(m.styles.Warning.Render("Watcher already running"))
			return m, nil
		}
		// Update log position to current end before starting watcher
		if info, err := m.logReader.Stat(); err == nil {
			m.lastLogPos = info.Size()
		}
		m.watcher.Start()
		m = m.appendActivity(m.styles.Success.Render("Watcher started"))
	case "stop":
		if !watcherIsRunning(any(m.watcher)) {
			m = m.appendActivity(m.styles.Warning.Render("Watcher not running"))
			return m, nil
		}
		m.watcher.Stop()
		m = m.appendActivity(m.styles.Success.Render("Watcher stopped"))
	default:
		m = m.appendActivity(m.styles.Error.Render("Usage: watch start|stop"))
	}
	return m, nil
}

func (m model) handleStatus() (model, tea.Cmd) {
	agents := m.manager.List()
	content := []string{}

	// Add tab information section
	tabs := m.tabManager.GetTabs()
	activeTabIndex := m.tabManager.GetActiveTabIndex()

	content = append(content, m.styles.Prompt.Render("Active Tab"))
	if len(tabs) > 0 {
		activeTab := m.tabManager.GetActiveTab()
		content = append(content, fmt.Sprintf("  Current: %s", activeTab.Title()))
		content = append(content, fmt.Sprintf("  Type: %s", getTabTypeName(activeTab.Type())))
	} else {
		content = append(content, "  No tabs")
	}

	content = append(content, "")
	content = append(content, m.styles.Prompt.Render(fmt.Sprintf("Tabs (%d open)", len(tabs))))

	for i, tab := range tabs {
		indicator := "  "
		if i == activeTabIndex {
			indicator = "* "
		}
		closable := ""
		if tab.IsClosable() {
			closable = " (closable)"
		}
		content = append(content, fmt.Sprintf("%s%s%s", indicator, tab.Title(), closable))
	}

	if len(tabs) > 1 {
		content = append(content, "")
		content = append(content, m.styles.Prompt.Render("Navigation"))
		content = append(content, "  F2 - Toggle between main and first agent tab")
		content = append(content, "  [ - Previous tab")
		content = append(content, "  ] - Next tab")
	}

	// Filter running agents for interactive selection
	runningAgents := []*agent.Agent{}
	stoppedAgents := []*agent.Agent{}
	for _, a := range agents {
		if a.Status == agent.StatusRunning {
			runningAgents = append(runningAgents, a)
		} else {
			stoppedAgents = append(stoppedAgents, a)
		}
	}

	// Sort running agents by issue number for deterministic ordering
	sort.Slice(runningAgents, func(i, j int) bool {
		return runningAgents[i].IssueNumber < runningAgents[j].IssueNumber
	})

	// Store snapshot so number key selection references the same order
	m.statusRunningAgents = runningAgents

	if len(runningAgents) > 0 {
		content = append(content, "", m.styles.Prompt.Render("Running Agents"))
		content = append(content, "Press number to open view:")
		content = append(content, "")

		// Scale title truncation to available overlay width
		titleMax := m.getOverlayContentWidth() - 25 // Reserve space for number, issue#, status, elapsed
		if titleMax < 15 {
			titleMax = 15
		}

		// Display up to 9 running agents with numbers
		for i, a := range runningAgents {
			if i >= 9 { // Only support 1-9 for simplicity
				break
			}
			elapsed := time.Since(a.StartTime).Truncate(time.Second)
			line := fmt.Sprintf("  %d. Issue #%d: %s (%s, %s)",
				i+1, a.IssueNumber, truncate(a.IssueTitle, titleMax), string(a.Status), elapsed)
			content = append(content, line)
		}

		if len(runningAgents) > 9 {
			content = append(content, fmt.Sprintf("  ... and %d more", len(runningAgents)-9))
		}
	}

	if len(stoppedAgents) > 0 {
		content = append(content, "", m.styles.Prompt.Render("Stopped Agents:"))
		titleMax := m.getOverlayContentWidth() - 22 // Reserve space for issue#, status, elapsed
		if titleMax < 15 {
			titleMax = 15
		}
		for _, a := range stoppedAgents {
			elapsed := time.Since(a.StartTime).Truncate(time.Second)
			line := fmt.Sprintf("   Issue #%d: %s (%s, %s)",
				a.IssueNumber, truncate(a.IssueTitle, titleMax), string(a.Status), elapsed)
			content = append(content, line)
		}
	}

	if len(agents) == 0 {
		content = append(content, "", m.styles.Warning.Render("No agents running"))
	}

	m = m.activateOverlay(overlayStatus, "System Status", content)
	return m, nil
}

func (m model) handleStop(issueStr string) (model, tea.Cmd) {
	issueNum, err := strconv.Atoi(issueStr)
	if err != nil {
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Invalid issue number: %s", issueStr)))
		return m, nil
	}

	agents := m.manager.List()
	for _, a := range agents {
		if a.IssueNumber == issueNum {
			if err := m.manager.Stop(a.ID); err != nil {
				m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Error stopping agent: %v", err)))
			} else {
				m = m.appendActivity(m.styles.Success.Render(fmt.Sprintf("Stopped agent for issue %d", issueNum)))
			}
			return m, nil
		}
	}
	m = m.appendActivity(m.styles.Warning.Render(fmt.Sprintf("No agent running for issue %d", issueNum)))
	return m, nil
}

func (m model) handleHelp() (model, tea.Cmd) {
	content := []string{
		m.styles.Prompt.Render("Available commands:"),
		"  watch start    - Start watching for labeled issues",
		"  watch stop     - Stop watching",
		"  status         - List all agents with details",
		"  stop <issue>   - Stop agent for specific issue number",
		"  plan [desc]    - Create new ACP-based Planning tab",
		"  plan classic [desc] - Start legacy subprocess planning session",
		"  log [level] [size] - Open log viewer (level: debug/info/warn/error, size: buffer lines)",
		"  logs           - View incident logs",
		"  theme          - Show current theme",
		"  theme <name>   - Switch to theme",
		"  about          - Show version information and check for updates",
		"  exit           - Exit (Ctrl+C also works)",
		"  help           - Show this help message",
		"",
		m.styles.Prompt.Render("Hotkeys:"),
		"  F2             - Toggle between console and agent output views",
		"  Ctrl+Alt+P     - Toggle between console and planning modes",
	}

	m = m.activateOverlay(overlayHelp, "Help", content)
	return m, nil
}

func (m model) handlePlan(description string) (model, tea.Cmd) {
	// Check tab limit before creating new planning tab
	if !m.tabManager.CanCreatePlanningTab() {
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Maximum %d concurrent planning tabs reached", MaxPlanningTabs)))
		return m, nil
	}

	// Generate tab title based on description or use default
	var tabTitle string
	if description != "" {
		// Use first few words of description as title
		words := strings.Fields(description)
		if len(words) > 3 {
			tabTitle = strings.Join(words[:3], " ") + "..."
		} else {
			tabTitle = description
		}
	} else {
		// Count existing planning tabs for naming
		existingCount := 0
		for _, tab := range m.tabManager.GetTabs() {
			if tab.Type() == TabTypePlanning {
				existingCount++
			}
		}
		tabTitle = fmt.Sprintf("Planning %d", existingCount+1)
	}

	// Create ACP-based planning tab with comprehensive error handling
	planningTab, err := m.tabManager.CreateAndAddPlanningTab(
		m.styles,
		m.footerManager.GetContextTracker(),
		m.sessionManager,
	)
	if err != nil {
		// Provide detailed error message based on error type
		var errorMsg string
		if strings.Contains(err.Error(), "ACP") {
			errorMsg = fmt.Sprintf("ACP connection failed: %v\nNote: Kiro CLI must be installed and accessible for ACP-based planning.", err)
		} else if strings.Contains(err.Error(), "session") {
			errorMsg = fmt.Sprintf("Session management failed: %v\nPlanning tab may not persist across restarts.", err)
		} else {
			errorMsg = fmt.Sprintf("Failed to create planning tab: %v", err)
		}

		m = m.appendActivity(m.styles.Error.Render(errorMsg))

		// Offer fallback to classic planning if ACP is unavailable
		if strings.Contains(err.Error(), "ACP") || strings.Contains(err.Error(), "kiro-cli") {
			m = m.appendActivity(m.styles.Warning.Render("Consider using 'plan classic [description]' for subprocess-based planning"))
		}
		return m, nil
	}

	// Pre-seed focus state to target message input when tab opens
	m.tabFocusStates[planningTab.ID()] = FocusTargetMessage

	// Set the tab title if description was provided
	if description != "" {
		planningTab.SetTitle(tabTitle)
	}

	// Start context tracking for the new planning session with error handling
	// NOTE: Tab switch will handle context tracking automatically

	// Switch to the newly created tab (already added by CreateAndAddPlanningTab)
	var focusCmd tea.Cmd
	m, focusCmd = m.switchActiveTab(len(m.tabManager.GetTabs()) - 1)

	// Pre-fill the message input with the description (if any) so the user
	// lands in the input ready to type/edit, cursor at end — mirroring a
	// fresh kiro-cli chat session. The description is staged in the input,
	// NOT added to conversation history: nothing is sent until the user
	// submits it.
	planningTab.textinput.SetValue(description)
	planningTab.textinput.CursorEnd()

	// Add connection status feedback as a system message
	if description != "" {
		planningTab.AddMessage("system", "💡 Planning tab ready. ACP connection will be established when you send your first message.")
	} else {
		planningTab.AddMessage("system", "🚀 ACP-based Planning Tab ready. Type your message to start planning.")
		planningTab.AddMessage("system", "📝 Use Tab to switch focus between message history and input area.")
	}

	// Update session with initial state
	planningTab.SaveSession()

	m = m.appendActivity(m.styles.Success.Render(fmt.Sprintf("✅ Created planning tab: %s", planningTab.Title())))

	return m, focusCmd
}

func (m model) handlePlanClassic(description string) (model, tea.Cmd) {
	// Use the legacy subprocess-based planning functionality
	return m.handlePlanSubprocess(description)
}

func (m model) handlePlanSubprocess(description string) (model, tea.Cmd) {
	// Suspend agent output capture before entering planning mode
	m.manager.SuspendOutputCapture()

	// Preserve current console state
	m.consoleState.inputValue = m.input.Value()
	m.consoleState.activityLines = make([]string, len(m.activityLines))
	copy(m.consoleState.activityLines, m.activityLines)
	if activeTab := m.tabManager.GetActiveTab(); activeTab != nil {
		m.consoleState.activeTabID = activeTab.ID()
	}

	// Check for existing planning sessions with error recovery
	sessions, err := m.sessionManager.List()
	if err != nil {
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Failed to check sessions: %v", err)))
		return m, nil
	}

	// Look for existing planning session with corruption handling
	var planningSessionID string
	for _, sessionID := range sessions {
		state, err := m.sessionManager.Load(sessionID)
		if err != nil {
			if strings.Contains(err.Error(), "corruption") {
				m = m.appendActivity(m.styles.Warning.Render(fmt.Sprintf("Removing corrupted session %s", sessionID[:8])))
				_ = m.sessionManager.Delete(sessionID)
			}
			continue
		}
		if state.Type == session.Planning {
			planningSessionID = sessionID
			break
		}
	}

	var sessionMsg string
	if planningSessionID == "" {
		sessionMsg = "Starting new classic planning session"
	} else {
		sessionMsg = fmt.Sprintf("Resuming planning session %s...", planningSessionID[:8])
	}

	// Start context tracking for planning mode with default model
	m.footerManager.GetContextTracker().StartPlanningSession("claude-sonnet-4")

	m = m.appendActivity(m.styles.Success.Render(sessionMsg))
	m.currentMode = session.Planning
	m.input.Blur()

	// Execute the kiro-cli subprocess for classic planning with shell wrapper
	args := []string{"chat", "--classic", "--agent", "planner"}
	if description != "" {
		args = append(args, description)
	}

	// Wrap in shell with clear and centered ASCII art banner
	banner := `cols=$(tput cols 2>/dev/null || echo 80)
art1="  _  ___              _  __                   "
art2=" | |/ (_)_ __ ___    | |/ /_ __ _____      __"
art3=" | ' /| | '__/ _ \   | ' /| '__/ _ \ \ /\ / /"
art4=" | . \| | | | (_) |  | . \| | |  __/\ V  V / "
art5=" |_|\_\_|_|  \___/   |_|\_\_|  \___| \_/\_/  "
pad() { w=${#1}; p=$(( (cols - w) / 2 )); [ "$p" -lt 0 ] && p=0; printf "%*s%s\n" "$p" "" "$1"; }
echo ""
pad "$art1"
pad "$art2"
pad "$art3"
pad "$art4"
pad "$art5"
echo ""`
	script := "clear && " + banner + " && exec kiro-cli \"$@\""
	cmd := exec.Command("sh", append([]string{"-c", script, "sh"}, args...)...)

	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

type execDoneMsg struct {
	err error
}

type updateCheckMsg struct {
	release *github.Release
	err     error
}

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

func (m model) getOverlayContentWidth() int {
	overlayWidth := int(float64(m.width) * 0.6)
	if overlayWidth < 40 {
		overlayWidth = 40
	}

	// Ensure overlay doesn't exceed screen bounds (mirrors renderOverlay()).
	if overlayWidth >= m.width {
		overlayWidth = m.width - 2
	}

	contentWidth := overlayWidth - 6 // Account for border and padding
	if contentWidth < 1 {
		contentWidth = 1
	}
	return contentWidth
}

func (m model) handleAbout() (model, tea.Cmd) {
	m.aboutDialog.BuildContent()
	m.aboutDialog.UpdateStatusLine([]string{"Checking for updates..."})

	m = m.activateOverlay(overlayAbout, "Kairon Version Information", m.aboutDialog.GetFullContent())
	return m, checkForUpdateCmd()
}

func (m model) handleTheme(args []string) (model, tea.Cmd) {
	if len(args) == 0 {
		// No longer show overlay for current theme - persistent display handles this
		return m, nil
	}

	if len(args) > 1 {
		m = m.appendActivity(m.styles.Error.Render("Usage: theme [name]"))
		return m, nil
	}

	themeName := args[0]

	// Try to load the theme (this handles validation)
	theme, err := config.LoadTheme(themeName)
	if err != nil {
		available := config.GetAvailableThemes()
		m = m.appendActivity(
			m.styles.Error.Render(fmt.Sprintf("Failed to load theme '%s': %v", themeName, err)),
			m.styles.Warning.Render(fmt.Sprintf("Available themes: %s", strings.Join(available, ", "))),
		)
		return m, nil
	}

	previousTheme := m.config.Theme
	previousLoadedTheme := m.config.LoadedTheme

	// Update config
	m.config.Theme = themeName
	m.config.LoadedTheme = theme

	// Save config
	if err := m.config.Save(); err != nil {
		m.config.Theme = previousTheme
		m.config.LoadedTheme = previousLoadedTheme
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Failed to save config: %v", err)))
		return m, nil
	}

	// Update styles with new theme
	m.styles = NewStyles(theme)

	m = m.appendActivity(m.styles.Success.Render(fmt.Sprintf("Theme changed to: %s", themeName)))
	return m, tea.ClearScreen
}

func checkForUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		release, err := github.GetLatestRelease("jbrinkman/kairon")
		return updateCheckMsg{release: release, err: err}
	}
}

// getTabTypeName converts TabType enum to readable string
func getTabTypeName(tabType TabType) string {
	switch tabType {
	case TabTypeMain:
		return "Main Console"
	case TabTypeAgent:
		return "Agent Output"
	case TabTypePlanning:
		return "Planning"
	case TabTypeLog:
		return "Log Viewer"
	default:
		return "Unknown"
	}
}

// switchToPlanningMode switches from console to planning mode while preserving console state
func (m model) switchToPlanningMode() (model, tea.Cmd) {
	// Suspend agent output capture before entering planning mode
	m.manager.SuspendOutputCapture()

	// Preserve current console state
	m.consoleState.inputValue = m.input.Value()
	m.consoleState.activityLines = make([]string, len(m.activityLines))
	copy(m.consoleState.activityLines, m.activityLines)
	if activeTab := m.tabManager.GetActiveTab(); activeTab != nil {
		m.consoleState.activeTabID = activeTab.ID()
	}

	// Check if there are any active planning tabs first
	activePlanningTabs := 0
	var lastPlanningTab *PlanningTab
	for _, tab := range m.tabManager.GetTabs() {
		if tab.Type() == TabTypePlanning {
			activePlanningTabs++
			if planningTab, ok := tab.(*PlanningTab); ok {
				lastPlanningTab = planningTab
			}
		}
	}

	// If there are active planning tabs, switch to the most recent one
	if activePlanningTabs > 0 && lastPlanningTab != nil {
		// Find the index of the last planning tab and switch to it
		for i, tab := range m.tabManager.GetTabs() {
			if tab == lastPlanningTab {
				m.tabManager.SetActiveTab(i)
				break
			}
		}

		// Start context tracking if not already active
		if !m.footerManager.GetContextTracker().IsActive() {
			if err := m.footerManager.GetContextTracker().StartPlanningSessionWithValidation("claude-sonnet-4"); err != nil {
				m = m.appendActivity(m.styles.Warning.Render(fmt.Sprintf("Context tracking warning: %v", err)))
			}
		}

		m = m.appendActivity(m.styles.Success.Render(fmt.Sprintf("Switched to active planning tab (found %d planning tabs)", activePlanningTabs)))
		m.currentMode = session.Planning
		m.input.Blur()
		return m, tea.ClearScreen
	}

	// Check for existing planning sessions with error recovery
	sessions, err := m.sessionManager.List()
	if err != nil {
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Failed to check sessions: %v", err)))
		// Resume output capture on failure
		m.manager.ResumeOutputCapture()
		return m, nil
	}

	// Look for existing planning session with corruption handling
	var planningSessionID string
	for _, sessionID := range sessions {
		state, err := m.sessionManager.Load(sessionID)
		if err != nil {
			if strings.Contains(err.Error(), "corruption") {
				m = m.appendActivity(m.styles.Warning.Render(fmt.Sprintf("Removing corrupted session %s", sessionID[:8])))
				_ = m.sessionManager.Delete(sessionID)
			}
			continue
		}
		if state.Type == session.Planning {
			planningSessionID = sessionID
			break
		}
	}

	var sessionMsg string
	if planningSessionID == "" {
		m = m.appendActivity(m.styles.Warning.Render("No active planning session or tabs"))
		m = m.appendActivity(m.styles.Warning.Render("Use 'plan [description]' to create a new planning tab"))
		// Resume output capture on failure
		m.manager.ResumeOutputCapture()
		return m, nil
	}
	sessionMsg = fmt.Sprintf("Resuming planning session %s...", planningSessionID[:8])

	// Start context tracking for planning mode with default model
	if err := m.footerManager.GetContextTracker().StartPlanningSessionWithValidation("claude-sonnet-4"); err != nil {
		m = m.appendActivity(m.styles.Warning.Render(fmt.Sprintf("Context tracking warning: %v", err)))
	}

	m = m.appendActivity(m.styles.Success.Render(sessionMsg))
	m.currentMode = session.Planning
	m.input.Blur()

	return m, tea.ClearScreen
}

// switchToConsoleMode switches from planning to console mode while preserving planning state
func (m model) switchToConsoleMode() (model, tea.Cmd) {
	// Save any active planning tab states
	activePlanningTabs := 0
	for _, tab := range m.tabManager.GetTabs() {
		if planningTab, ok := tab.(*PlanningTab); ok && planningTab.Type() == TabTypePlanning {
			activePlanningTabs++
			// Force save session state when switching away
			planningTab.SaveSession()
		}
	}

	// Handle legacy planning session cleanup
	if m.activePlanningSession != nil {
		// Suspend and preserve planning session state with error handling
		if err := m.activePlanningSession.SuspendAndDetach(); err != nil {
			m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Failed to suspend planning session: %v", err)))
			log.Printf("Planning session suspend error: %v", err)
			// Continue anyway, don't block mode switch
		} else {
			m = m.appendActivity(m.styles.Success.Render("Planning session suspended"))
		}
		m.activePlanningSession = nil
	}

	// Stop context tracking when exiting planning mode
	m.footerManager.GetContextTracker().StopPlanningSession()

	// Resume agent output capture when returning to console mode
	m.manager.ResumeOutputCapture()

	// Switch to console mode and restore console state
	m.currentMode = session.Console
	m = m.restoreConsoleState()

	// Restore previously active tab by ID
	restored := false
	if m.consoleState.activeTabID != "" {
		for i, tab := range m.tabManager.GetTabs() {
			if tab.ID() == m.consoleState.activeTabID {
				m, _ = m.switchActiveTab(i)
				restored = true
				break
			}
		}
	}
	if !restored {
		// Fallback to main tab
		for i, tab := range m.tabManager.GetTabs() {
			if tab.Type() == TabTypeMain {
				m, _ = m.switchActiveTab(i)
				break
			}
		}
	}

	m.input.Focus()

	// Provide user feedback about mode switch
	if activePlanningTabs > 0 {
		m = m.appendActivity(m.styles.Success.Render(fmt.Sprintf("Switched to console mode (%d planning tabs preserved)", activePlanningTabs)))
	} else {
		m = m.appendActivity(m.styles.Success.Render("Switched to console mode"))
	}

	return m, tea.Batch(m.input.Focus(), tea.ClearScreen)
}

// restoreConsoleState restores the console state after returning from planning mode
func (m model) restoreConsoleState() model {
	if m.consoleState != nil {
		m.input.SetValue(m.consoleState.inputValue)
		m.activityLines = make([]string, len(m.consoleState.activityLines))
		copy(m.activityLines, m.consoleState.activityLines)
	}
	m.currentMode = session.Console
	return m
}

func (m model) handleLogs() (model, tea.Cmd) {
	logger, err := incidents.NewIncidentLogger()
	if err != nil {
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Failed to initialize logger: %v", err)))
		return m, nil
	}

	incidents, err := logger.ListIncidents()
	if err != nil {
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Failed to list incidents: %v", err)))
		return m, nil
	}

	content := []string{}
	if len(incidents) == 0 {
		content = append(content, m.styles.Warning.Render("No incident logs found"))
	} else {
		content = append(content, m.styles.Prompt.Render(fmt.Sprintf("Found %d incident logs:", len(incidents))))
		content = append(content, "")

		for _, incident := range incidents {
			timestamp := incident.Timestamp.Format("Jan 02 15:04:05")
			line := fmt.Sprintf("Issue #%d (attempt %d) - %s", incident.IssueNumber, incident.Attempt, timestamp)
			content = append(content, line)
		}

		content = append(content, "")
		content = append(content, m.styles.Prompt.Render("Log files location:"))
		content = append(content, fmt.Sprintf("~/.kairon/logs/%s/incidents/", logger.RepoName()))
	}

	m = m.activateOverlay(overlayLogs, "Incident Logs", content)
	return m, nil
}

// handleLog opens the log viewer tab with optional level and size parameters
func (m model) handleLog(args []string) (model, tea.Cmd) {
	// Parse optional parameters: log [level] [size]
	var level string
	var bufferSize int
	var err error

	// Use config defaults
	level = m.config.Logging.DefaultLevel
	bufferSize = m.config.Logging.MaxBufferLines

	// Parse level parameter if provided
	if len(args) > 0 {
		providedLevel := strings.ToLower(args[0])
		validLevels := map[string]bool{
			"debug": true,
			"info":  true,
			"warn":  true,
			"error": true,
		}
		if !validLevels[providedLevel] {
			m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Invalid log level '%s'. Valid levels: debug, info, warn, error", args[0])))
			return m, nil
		}
		level = providedLevel
	}

	// Parse size parameter if provided
	if len(args) > 1 {
		bufferSize, err = strconv.Atoi(args[1])
		if err != nil || bufferSize <= 0 {
			m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Invalid buffer size '%s'. Must be a positive integer", args[1])))
			return m, nil
		}
	}

	// Check if a log viewer tab already exists (enforce single tab constraint)
	if m.tabManager.HasLogTab() {
		existingIndex := m.tabManager.FindLogTab()
		if len(args) > 0 {
			// User provided parameters - prompt for action
			m = m.appendActivity(
				m.styles.Warning.Render("Log viewer tab already exists"),
				m.styles.Warning.Render("Close the existing tab (Ctrl+W) first to create a new one with different parameters"),
			)
		} else {
			// No parameters provided - just navigate to existing tab
			m = m.appendActivity(m.styles.Success.Render("Switched to existing log viewer tab"))
		}
		// Switch to the existing log tab
		var cmd tea.Cmd
		m, cmd = m.switchActiveTab(existingIndex)
		return m, cmd
	}

	// Create new log viewer tab
	logTab := NewLogTab("log-viewer", level, bufferSize, m.styles)

	// Activate the logging subsystem with this log tab
	if err := m.activateLogging(logTab); err != nil {
		m = m.appendActivity(m.styles.Error.Render(fmt.Sprintf("Failed to activate logging: %v", err)))
		return m, nil
	}

	// Add the log tab to the tab manager
	m.tabManager.AddTab(logTab)

	// Switch to the newly created log tab
	var switchCmd tea.Cmd
	m, switchCmd = m.switchActiveTab(len(m.tabManager.GetTabs()) - 1)

	// Start polling for log entries
	pollCmd := logTab.StartPolling()

	m = m.appendActivity(m.styles.Success.Render(fmt.Sprintf("Log viewer opened: level=%s, buffer_size=%d", level, bufferSize)))

	return m, tea.Batch(switchCmd, pollCmd)
}
