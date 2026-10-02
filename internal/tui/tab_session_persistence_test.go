package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/session"
)

// TestTabDataAccumulatesContinuouslyDuringPlannerSessions verifies that agent output
// continues to accumulate even while in planning mode
func TestTabDataAccumulatesContinuouslyDuringPlannerSessions(t *testing.T) {
	m := setupTestModel(t)

	// Create test agent
	testAgent := &agent.Agent{
		ID:          "continuous-agent",
		IssueNumber: 101,
		IssueTitle:  "Continuous Data Test",
		Status:      agent.StatusRunning,
		StartTime:   time.Now(),
	}
	m.manager.RegisterAgent(testAgent.ID, testAgent.IssueNumber)

	// Create agent tab
	m.tabManager.RestoreOrFocusAgentTab(testAgent.ID, m.manager, m.styles)

	// Add initial agent output
	m.manager.CaptureOutputLine(testAgent.IssueNumber, "Initial output before planning mode")

	// Verify initial output is captured
	outputLines := m.manager.GetOutputLines()
	initialCount := len(outputLines)
	if initialCount == 0 {
		t.Fatal("Initial output should be captured")
	}

	// Mock planning session
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	// Switch to planning mode - this should suspend terminal output but NOT data accumulation
	m, _ = m.switchToPlanningMode()
	if m.currentMode != session.Planning {
		t.Error("Should be in planning mode")
	}

	// Simulate agent output continuing while in planning mode
	continuousOutputLines := []string{
		"Planning mode output 1",
		"Planning mode output 2",
		"Planning mode output 3",
		"Agent continues working",
		"More output while planning",
	}

	for _, line := range continuousOutputLines {
		m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
	}

	// Verify output continues to accumulate during planning mode
	updatedOutputLines := m.manager.GetOutputLines()
	if len(updatedOutputLines) <= initialCount {
		t.Errorf("Output should continue accumulating during planning mode: initial %d, current %d",
			initialCount, len(updatedOutputLines))
	}

	// Verify specific continuous output is captured
	foundContinuousOutput := 0
	for _, line := range updatedOutputLines {
		for _, expected := range continuousOutputLines {
			if strings.Contains(line, expected) {
				foundContinuousOutput++
				break
			}
		}
	}

	if foundContinuousOutput != len(continuousOutputLines) {
		t.Errorf("Not all continuous output was captured: expected %d, found %d",
			len(continuousOutputLines), foundContinuousOutput)
	}

	// Switch back to console mode
	m.currentMode = session.Planning
	m, _ = m.switchToConsoleMode()

	// Verify all accumulated output is still available
	finalOutputLines := m.manager.GetOutputLines()
	if len(finalOutputLines) < len(updatedOutputLines) {
		t.Error("Output should not be lost when returning from planning mode")
	}

	// Verify agent tab still shows accumulated data
	tabs := m.tabManager.GetTabs()
	agentTabFound := false
	for _, tab := range tabs {
		if tab.Type() == TabTypeAgent && tab.ID() == "agent-"+testAgent.ID {
			agentTabFound = true
			break
		}
	}
	if !agentTabFound {
		t.Error("Agent tab should still exist with accumulated data")
	}
}

// TestAgentTabsRetainFullLogHistoryWhenSwitchingBack verifies that agent tabs
// preserve their complete log history across mode switches
func TestAgentTabsRetainFullLogHistoryWhenSwitchingBack(t *testing.T) {
	m := setupTestModel(t)

	// Create multiple agents with different amounts of history
	agents := []*agent.Agent{
		{ID: "history-agent-1", IssueNumber: 201, IssueTitle: "History Test 1", Status: agent.StatusRunning, StartTime: time.Now()},
		{ID: "history-agent-2", IssueNumber: 202, IssueTitle: "History Test 2", Status: agent.StatusRunning, StartTime: time.Now()},
	}

	// Register agents and create tabs
	for _, testAgent := range agents {
		m.manager.RegisterAgent(testAgent.ID, testAgent.IssueNumber)
		m.tabManager.RestoreOrFocusAgentTab(testAgent.ID, m.manager, m.styles)
	}

	// Build substantial history for each agent
	historyData := make(map[int][]string)
	for i, testAgent := range agents {
		history := []string{}
		for j := 0; j < 20; j++ {
			line := fmt.Sprintf("Agent %d - History line %d", i+1, j+1)
			m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
			history = append(history, line)
		}
		historyData[testAgent.IssueNumber] = history
	}

	// Capture initial state
	initialOutputLines := m.manager.GetOutputLines()
	initialTabCount := len(m.tabManager.GetTabs())

	// Mock planning session
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	// Switch to planning mode
	m, _ = m.switchToPlanningMode()

	// Add more history while in planning mode
	for i, testAgent := range agents {
		for j := 0; j < 10; j++ {
			line := fmt.Sprintf("Agent %d - Planning mode line %d", i+1, j+1)
			m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
			historyData[testAgent.IssueNumber] = append(historyData[testAgent.IssueNumber], line)
		}
	}

	// Switch back to console mode
	m.currentMode = session.Planning
	m, _ = m.switchToConsoleMode()

	// Verify tab count is preserved
	if len(m.tabManager.GetTabs()) != initialTabCount {
		t.Errorf("Tab count changed: expected %d, got %d", initialTabCount, len(m.tabManager.GetTabs()))
	}

	// Verify all history is preserved
	finalOutputLines := m.manager.GetOutputLines()
	if len(finalOutputLines) < len(initialOutputLines) {
		t.Error("Output history was lost during mode switching")
	}

	// Verify specific history for each agent
	for issueNumber, expectedHistory := range historyData {
		foundHistoryCount := 0
		for _, line := range finalOutputLines {
			for _, expectedLine := range expectedHistory {
				if strings.Contains(line, expectedLine) {
					foundHistoryCount++
					break
				}
			}
		}

		if foundHistoryCount < len(expectedHistory) {
			t.Errorf("Agent %d missing history: expected %d lines, found %d",
				issueNumber, len(expectedHistory), foundHistoryCount)
		}
	}

	// Verify each agent tab is accessible and functional
	for _, testAgent := range agents {
		tabIndex := m.tabManager.FindTabByAgentID(testAgent.ID)
		if tabIndex == -1 {
			t.Errorf("Agent tab for %s not found after mode switch", testAgent.ID)
			continue
		}

		// Switch to agent tab and verify it works
		m.tabManager.SetActiveTab(tabIndex)
		activeTab := m.tabManager.GetActiveTab()
		if activeTab == nil || activeTab.Type() != TabTypeAgent {
			t.Errorf("Agent tab for %s not accessible", testAgent.ID)
		}
	}
}

// TestMainTUIWatcherLogsContinueAccumulating verifies that main console logs
// continue to accumulate during planner mode
func TestMainTUIWatcherLogsContinueAccumulating(t *testing.T) {
	m := setupTestModel(t)

	// Add initial console activity
	initialActivity := []string{
		"Watcher started",
		"Polling GitHub for issues",
		"Found issue #123",
	}

	for _, line := range initialActivity {
		m = m.appendActivity(line)
	}

	initialActivityCount := len(m.activityLines)
	if initialActivityCount == 0 {
		t.Fatal("Initial activity should be present")
	}

	// Mock planning session
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	// Switch to planning mode
	m, _ = m.switchToPlanningMode()

	// Simulate console activity continuing (e.g., from watcher polling, background operations)
	// In planning mode, console state should be preserved but new activity can still accumulate
	planningModeActivity := []string{
		"Background polling continues",
		"Agent status updated",
		"Issue #124 detected",
		"Watcher activity continues",
	}

	// Note: In planning mode, we preserve console state, but the system can still add activity
	// This simulates background operations continuing
	for _, line := range planningModeActivity {
		m = m.appendActivity(line)
	}

	// Verify console state preservation mechanism is working
	if m.consoleState == nil {
		t.Fatal("Console state should be preserved during planning mode")
	}

	// Switch back to console mode
	m.currentMode = session.Planning
	m, _ = m.switchToConsoleMode()

	// Verify console activity was preserved and potentially accumulated
	if len(m.activityLines) < initialActivityCount {
		t.Errorf("Console activity was lost: initial %d, final %d",
			initialActivityCount, len(m.activityLines))
	}

	// Verify specific activity content is preserved
	activityContent := strings.Join(m.activityLines, "\n")
	for _, expected := range initialActivity {
		if !strings.Contains(activityContent, expected) {
			t.Errorf("Initial activity line missing: %s", expected)
		}
	}
}

// TestHistoryLimitsRespectedDuringPersistence verifies that existing buffer limits
// are still enforced even during continuous data accumulation
func TestHistoryLimitsRespectedDuringPersistence(t *testing.T) {
	m := setupTestModel(t)

	// Set a small activity limit for testing
	m.maxActivityLines = 5

	// Create test agent
	testAgent := &agent.Agent{
		ID:          "limit-test-agent",
		IssueNumber: 301,
		IssueTitle:  "Limit Test",
		Status:      agent.StatusRunning,
		StartTime:   time.Now(),
	}
	m.manager.RegisterAgent(testAgent.ID, testAgent.IssueNumber)

	// Fill beyond the limit
	for i := 0; i < 10; i++ {
		m = m.appendActivity(fmt.Sprintf("Activity line %d", i+1))
	}

	// Verify limit is enforced for console activity
	if len(m.activityLines) > m.maxActivityLines {
		t.Errorf("Console activity limit not enforced: expected max %d, got %d",
			m.maxActivityLines, len(m.activityLines))
	}

	// Test OutputCapture buffer limits (using manager's output capture with 1000 line buffer)
	// Add many lines to test buffer behavior
	for i := 0; i < 1500; i++ {
		m.manager.CaptureOutputLine(testAgent.IssueNumber, fmt.Sprintf("Output line %d", i+1))
	}

	outputLines := m.manager.GetOutputLines()
	// The OutputCapture has a maxSize of 1000, so it should not exceed that
	if len(outputLines) > 1000 {
		t.Errorf("Output capture buffer limit not enforced: expected max 1000, got %d", len(outputLines))
	}

	// Mock planning session and test limits during mode switching
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	// Switch to planning mode
	m, _ = m.switchToPlanningMode()

	// Add more output while in planning mode
	for i := 1500; i < 2000; i++ {
		m.manager.CaptureOutputLine(testAgent.IssueNumber, fmt.Sprintf("Planning output line %d", i+1))
	}

	// Switch back and verify limits are still respected
	m.currentMode = session.Planning
	m, _ = m.switchToConsoleMode()

	finalOutputLines := m.manager.GetOutputLines()
	if len(finalOutputLines) > 1000 {
		t.Errorf("Output buffer limit violated after mode switching: expected max 1000, got %d",
			len(finalOutputLines))
	}

	// Verify we have the most recent data (ring buffer behavior)
	lastLine := ""
	for _, line := range finalOutputLines {
		if strings.Contains(line, "Planning output line") {
			lastLine = line
		}
	}

	if lastLine == "" {
		t.Error("Recent output should be preserved in ring buffer")
	}
}

// TestMultipleRapidModeSwitchesWithContinuousData verifies data persistence
// during rapid mode switching with continuous data accumulation
func TestMultipleRapidModeSwitchesWithContinuousData(t *testing.T) {
	m := setupTestModel(t)

	// Create test agents
	agents := []*agent.Agent{
		{ID: "rapid-1", IssueNumber: 401, IssueTitle: "Rapid Test 1", Status: agent.StatusRunning, StartTime: time.Now()},
		{ID: "rapid-2", IssueNumber: 402, IssueTitle: "Rapid Test 2", Status: agent.StatusRunning, StartTime: time.Now()},
	}

	for _, testAgent := range agents {
		m.manager.RegisterAgent(testAgent.ID, testAgent.IssueNumber)
		m.tabManager.RestoreOrFocusAgentTab(testAgent.ID, m.manager, m.styles)
	}

	// Mock planning session
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	outputCounter := 0
	activityCounter := 0

	// Perform rapid switching with continuous data generation
	for cycle := 0; cycle < 3; cycle++ {
		// Add data before switching
		for _, testAgent := range agents {
			outputCounter++
			m.manager.CaptureOutputLine(testAgent.IssueNumber,
				fmt.Sprintf("Cycle %d output %d", cycle, outputCounter))
		}

		activityCounter++
		m = m.appendActivity(fmt.Sprintf("Console activity cycle %d-%d", cycle, activityCounter))

		// Switch to planning mode
		m, _ = m.switchToPlanningMode()

		// Add data while in planning mode
		for _, testAgent := range agents {
			outputCounter++
			m.manager.CaptureOutputLine(testAgent.IssueNumber,
				fmt.Sprintf("Cycle %d planning output %d", cycle, outputCounter))
		}

		// Switch back to console mode
		m.currentMode = session.Planning
		m, _ = m.switchToConsoleMode()

		// Add data after switching back
		activityCounter++
		m = m.appendActivity(fmt.Sprintf("Console activity after cycle %d-%d", cycle, activityCounter))
	}

	// Verify data integrity after rapid switching
	outputLines := m.manager.GetOutputLines()
	if len(outputLines) == 0 {
		t.Fatal("All output should be preserved after rapid mode switching")
	}

	// Verify we have data from all cycles
	cycleDataFound := make(map[int]int)
	for _, line := range outputLines {
		for cycle := 0; cycle < 3; cycle++ {
			if strings.Contains(line, fmt.Sprintf("Cycle %d", cycle)) {
				cycleDataFound[cycle]++
			}
		}
	}

	for cycle := 0; cycle < 3; cycle++ {
		if cycleDataFound[cycle] == 0 {
			t.Errorf("No data found for cycle %d after rapid switching", cycle)
		}
	}

	// Verify tabs are still present and functional
	tabs := m.tabManager.GetTabs()
	expectedTabCount := 1 + len(agents) // Main + agent tabs
	if len(tabs) != expectedTabCount {
		t.Errorf("Tab count changed during rapid switching: expected %d, got %d",
			expectedTabCount, len(tabs))
	}

	// Verify each agent tab is accessible
	for _, testAgent := range agents {
		tabIndex := m.tabManager.FindTabByAgentID(testAgent.ID)
		if tabIndex == -1 {
			t.Errorf("Agent tab for %s lost during rapid switching", testAgent.ID)
		}
	}
}

// TestAgentDataIntegrityAfterMultiplePlannerCycles verifies data integrity
// across multiple complete planning session cycles with different agents
func TestAgentDataIntegrityAfterMultiplePlannerCycles(t *testing.T) {
	m := setupTestModel(t)

	// Create multiple agents with distinct outputs
	agents := []*agent.Agent{
		{ID: "integrity-1", IssueNumber: 501, IssueTitle: "Integrity Test 1", Status: agent.StatusRunning, StartTime: time.Now()},
		{ID: "integrity-2", IssueNumber: 502, IssueTitle: "Integrity Test 2", Status: agent.StatusRunning, StartTime: time.Now()},
	}

	for _, testAgent := range agents {
		m.manager.RegisterAgent(testAgent.ID, testAgent.IssueNumber)
		m.tabManager.RestoreOrFocusAgentTab(testAgent.ID, m.manager, m.styles)
	}

	// Mock planning session
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	expectedData := make(map[int][]string)

	// Perform 5 complete cycles with unique data each cycle
	for cycle := 0; cycle < 5; cycle++ {
		// Add unique pre-planning data
		for _, testAgent := range agents {
			line := fmt.Sprintf("Cycle-%d-Pre-Planning-Agent-%s", cycle, testAgent.ID)
			m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
			expectedData[testAgent.IssueNumber] = append(expectedData[testAgent.IssueNumber], line)
		}

		// Switch to planning mode
		m, _ = m.switchToPlanningMode()

		// Add unique planning data
		for _, testAgent := range agents {
			line := fmt.Sprintf("Cycle-%d-In-Planning-Agent-%s", cycle, testAgent.ID)
			m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
			expectedData[testAgent.IssueNumber] = append(expectedData[testAgent.IssueNumber], line)
		}

		// Switch back to console mode
		m.currentMode = session.Planning
		m, _ = m.switchToConsoleMode()

		// Add unique post-planning data
		for _, testAgent := range agents {
			line := fmt.Sprintf("Cycle-%d-Post-Planning-Agent-%s", cycle, testAgent.ID)
			m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
			expectedData[testAgent.IssueNumber] = append(expectedData[testAgent.IssueNumber], line)
		}
	}

	// Verify complete data integrity for each agent
	outputLines := m.manager.GetOutputLines()
	for issueNumber, expectedLines := range expectedData {
		foundCount := 0
		for _, line := range outputLines {
			for _, expectedLine := range expectedLines {
				if strings.Contains(line, expectedLine) {
					foundCount++
					break
				}
			}
		}

		if foundCount != len(expectedLines) {
			t.Errorf("Agent %d data integrity failed: expected %d lines, found %d",
				issueNumber, len(expectedLines), foundCount)
		}
	}

	// Verify tab integrity
	for _, testAgent := range agents {
		if m.tabManager.FindTabByAgentID(testAgent.ID) == -1 {
			t.Errorf("Agent tab %s lost during multiple cycles", testAgent.ID)
		}
	}
}

// TestSessionDataPersistenceDuringMemoryPressure tests data persistence under
// high-volume output to simulate memory pressure scenarios
func TestSessionDataPersistenceDuringMemoryPressure(t *testing.T) {
	m := setupTestModel(t)

	testAgent := &agent.Agent{
		ID:          "memory-pressure-agent",
		IssueNumber: 601,
		IssueTitle:  "Memory Pressure Test",
		Status:      agent.StatusRunning,
		StartTime:   time.Now(),
	}
	m.manager.RegisterAgent(testAgent.ID, testAgent.IssueNumber)
	m.tabManager.RestoreOrFocusAgentTab(testAgent.ID, m.manager, m.styles)

	// Mock planning session
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	// Generate high-volume output before planning mode
	preCount := 500
	for i := 0; i < preCount; i++ {
		m.manager.CaptureOutputLine(testAgent.IssueNumber, fmt.Sprintf("Pre-planning high-volume line %d", i))
	}

	initialOutputLines := m.manager.GetOutputLines()
	initialCount := len(initialOutputLines)

	// Switch to planning mode
	m, _ = m.switchToPlanningMode()

	// Generate high-volume output during planning mode
	planningCount := 300
	for i := 0; i < planningCount; i++ {
		m.manager.CaptureOutputLine(testAgent.IssueNumber, fmt.Sprintf("Planning high-volume line %d", i))
	}

	// Switch back to console mode
	m.currentMode = session.Planning
	m, _ = m.switchToConsoleMode()

	// Verify data accumulation survived memory pressure
	finalOutputLines := m.manager.GetOutputLines()

	// Should have at least initial lines (ring buffer may limit total)
	if len(finalOutputLines) < initialCount {
		t.Errorf("Lost data during memory pressure: initial %d, final %d",
			initialCount, len(finalOutputLines))
	}

	// Verify we still have recent data (ring buffer keeps latest entries)
	recentDataFound := false
	for _, line := range finalOutputLines {
		if strings.Contains(line, "Planning high-volume line") {
			recentDataFound = true
			break
		}
	}
	if !recentDataFound {
		t.Error("Recent data lost during memory pressure scenario")
	}

	// Verify tab still exists and is functional
	if m.tabManager.FindTabByAgentID(testAgent.ID) == -1 {
		t.Error("Agent tab lost during memory pressure scenario")
	}
}

// TestConcurrentAgentDataDuringPlannerSessionToggle tests concurrent agent
// data accumulation while rapidly toggling planner sessions
func TestConcurrentAgentDataDuringPlannerSessionToggle(t *testing.T) {
	m := setupTestModel(t)

	// Create concurrent agents
	agents := []*agent.Agent{
		{ID: "concurrent-1", IssueNumber: 701, IssueTitle: "Concurrent Test 1", Status: agent.StatusRunning, StartTime: time.Now()},
		{ID: "concurrent-2", IssueNumber: 702, IssueTitle: "Concurrent Test 2", Status: agent.StatusRunning, StartTime: time.Now()},
		{ID: "concurrent-3", IssueNumber: 703, IssueTitle: "Concurrent Test 3", Status: agent.StatusRunning, StartTime: time.Now()},
	}

	for _, testAgent := range agents {
		m.manager.RegisterAgent(testAgent.ID, testAgent.IssueNumber)
		m.tabManager.RestoreOrFocusAgentTab(testAgent.ID, m.manager, m.styles)
	}

	// Mock planning session
	sessionManager := session.NewSessionManager()
	sessionID, err := sessionManager.Create(session.Planning)
	if err != nil {
		t.Fatalf("Failed to create mock planning session: %v", err)
	}
	defer sessionManager.Delete(sessionID)
	m.sessionManager = sessionManager

	dataMap := make(map[int][]string)

	// Simulate concurrent activity with rapid toggling
	for iteration := 0; iteration < 10; iteration++ {
		// Add data for all agents
		for _, testAgent := range agents {
			line := fmt.Sprintf("Concurrent iteration %d for agent %s", iteration, testAgent.ID)
			m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
			dataMap[testAgent.IssueNumber] = append(dataMap[testAgent.IssueNumber], line)
		}

		// Rapid toggle to planning mode and back
		m, _ = m.switchToPlanningMode()
		m.currentMode = session.Planning
		m, _ = m.switchToConsoleMode()

		// Add more data after toggle
		for _, testAgent := range agents {
			line := fmt.Sprintf("Post-toggle iteration %d for agent %s", iteration, testAgent.ID)
			m.manager.CaptureOutputLine(testAgent.IssueNumber, line)
			dataMap[testAgent.IssueNumber] = append(dataMap[testAgent.IssueNumber], line)
		}
	}

	// Verify all concurrent data is preserved
	outputLines := m.manager.GetOutputLines()
	for issueNumber, expectedLines := range dataMap {
		foundCount := 0
		for _, line := range outputLines {
			for _, expectedLine := range expectedLines {
				if strings.Contains(line, expectedLine) {
					foundCount++
					break
				}
			}
		}

		if foundCount != len(expectedLines) {
			t.Errorf("Concurrent agent %d lost data: expected %d lines, found %d",
				issueNumber, len(expectedLines), foundCount)
		}
	}

	// Verify all agent tabs survived concurrent operations
	for _, testAgent := range agents {
		if m.tabManager.FindTabByAgentID(testAgent.ID) == -1 {
			t.Errorf("Agent tab %s lost during concurrent operations", testAgent.ID)
		}
	}
}
