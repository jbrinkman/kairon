package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/session"
)

func TestMultipleAgentsOutputCapture(t *testing.T) {
	capture := agent.NewOutputCapture(1000)

	// Simulate multiple agents producing output
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Simulate agent output
			for j := 0; j < 10; j++ {
				output := fmt.Sprintf("Agent %d output line %d", id, j)
				capture.AddLine(output)
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify output was captured
	lines := capture.GetLines()
	if len(lines) == 0 {
		t.Error("No output captured from multiple agents")
	}

	// Check that output from different agents is present
	found := make(map[int]bool)
	for _, line := range lines {
		for i := 0; i < 5; i++ {
			if strings.Contains(line, fmt.Sprintf("Agent %d", i)) {
				found[i] = true
			}
		}
	}

	if len(found) < 3 { // At least 3 agents should have output
		t.Errorf("Expected output from at least 3 agents, found %d", len(found))
	}
}

func TestANSIStripping(t *testing.T) {
	capture := agent.NewOutputCapture(100)

	testCases := []struct {
		input    string
		expected string
	}{
		{"\033[31mRed text\033[0m", "Red text"},
		{"\033[1;32mBold Green\033[0m", "Bold Green"},
		{"\033[2K\rClearing line", "\rClearing line"}, // \r is not stripped by ANSI regex
		{"Normal text", "Normal text"},
		{"\033[38;5;196mExtended color\033[0m", "Extended color"},
	}

	for _, tc := range testCases {
		capture.AddLine(tc.input)
	}

	lines := capture.GetLines()
	for i, tc := range testCases {
		if i < len(lines) {
			if lines[i] != tc.expected {
				t.Errorf("ANSI stripping failed: expected %q, got %q", tc.expected, lines[i])
			}
		}
	}
}

func TestViewManagerTransitions(t *testing.T) {
	vm := NewViewManager()

	// Test initial state
	if vm.CurrentView() != ViewConsole {
		t.Error("Expected initial view to be console")
	}

	// Test toggle to agent output
	vm.ToggleView()
	if vm.CurrentView() != ViewAgentOutput {
		t.Error("Failed to toggle to agent output view")
	}

	// Test toggle back to console
	vm.ToggleView()
	if vm.CurrentView() != ViewConsole {
		t.Error("Failed to toggle back to console view")
	}

	// Test explicit set
	vm.SetView(ViewAgentOutput)
	if vm.CurrentView() != ViewAgentOutput {
		t.Error("Failed to explicitly set agent output view")
	}
}

func TestHighVolumeOutput(t *testing.T) {
	capture := agent.NewOutputCapture(500) // Smaller buffer for test

	// Generate high volume output
	for i := 0; i < 1000; i++ {
		output := fmt.Sprintf("Line %d: %s", i, strings.Repeat("x", 50))
		capture.AddLine(output)
	}

	// Verify buffer size is maintained
	lines := capture.GetLines()
	if len(lines) > 500 {
		t.Errorf("Buffer exceeded max size: %d > 500", len(lines))
	}

	// Verify we have the most recent lines (buffer rotates)
	if len(lines) == 500 {
		lastLine := lines[len(lines)-1]
		if !strings.Contains(lastLine, "Line 999") {
			t.Error("Buffer rotation not working correctly - missing recent data")
		}
	}
}

func TestTerminalResizeHandling(t *testing.T) {
	vm := NewViewManager()

	// Test resize message
	resizeMsg := tea.WindowSizeMsg{Width: 120, Height: 30}
	_ = vm.Update(resizeMsg)

	// Verify dimensions are stored (indirectly through no panic/error)
	// The actual width/height are private, so we test the behavior works
	if vm.CurrentView() != ViewConsole {
		t.Error("View manager state corrupted after resize")
	}
}

func TestOutputAlwaysAccumulates(t *testing.T) {
	capture := agent.NewOutputCapture(100)

	// Add some initial output
	capture.AddLine("Initial line")

	// Add more output - data is always accumulated
	capture.AddLine("Second line - always captured")

	// Add even more output
	capture.AddLine("Third line")

	lines := capture.GetLines()

	// Should have all lines since data is always accumulated
	found := make(map[string]bool)
	for _, line := range lines {
		found[line] = true
	}

	if !found["Initial line"] {
		t.Error("Expected 'Initial line' not found")
	}

	if !found["Second line - always captured"] {
		t.Error("Second line was not captured (should always be captured)")
	}

	if !found["Third line"] {
		t.Error("Expected 'Third line' not found")
	}
}

func BenchmarkConcurrentOutput(b *testing.B) {
	capture := agent.NewOutputCapture(10000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		lineNum := 0
		for pb.Next() {
			output := fmt.Sprintf("Benchmark output %d", lineNum)
			capture.AddLine(output)
			lineNum++
		}
	})
}

func TestOutputViewRendersCapturedManagerOutput(t *testing.T) {
	cfg := &config.Config{LoadedTheme: &config.Theme{}}
	cfg.LoadedTheme.Colors.Primary = "#ffffff"
	cfg.LoadedTheme.Colors.Secondary = "#cccccc"
	cfg.LoadedTheme.Colors.Success = "#00ff00"
	cfg.LoadedTheme.Colors.Warning = "#ffff00"
	cfg.LoadedTheme.Colors.Error = "#ff0000"
	cfg.LoadedTheme.Colors.TextPrimary = "#ffffff"
	cfg.LoadedTheme.Colors.TextSecondary = "#cccccc"
	cfg.LoadedTheme.Colors.TextMuted = "#999999"
	cfg.LoadedTheme.Colors.Prompt = "#ffffff"
	cfg.LoadedTheme.Colors.Separator = "#666666"
	cfg.LoadedTheme.Colors.Activity = "#00ffff"
	cfg.LoadedTheme.Colors.Background = "#000000"
	cfg.LoadedTheme.Colors.Surface = "#111111"
	manager := agent.NewManager(cfg)
	for i := 0; i < 30; i++ {
		manager.CaptureOutputLine(42, fmt.Sprintf("captured line %d", i))
	}

	view := NewOutputView(manager, NewStyles(cfg.LoadedTheme))
	view.Resize(80, 6)

	rendered := view.View()
	if !strings.Contains(rendered, "captured line") {
		t.Fatalf("expected rendered output to include captured lines, got %q", rendered)
	}

	view.viewport.ScrollDown(1)
	scrolled := view.View()
	if rendered == scrolled {
		t.Fatal("expected view content to change after scrolling")
	}
}

// TestTask8Integration tests the complete integration of all Task 8 components
func TestTask8Integration(t *testing.T) {
	// Skip if in short mode to avoid long-running tests
	if testing.Short() {
		t.Skip("Skipping Task 8 integration test in short mode")
	}

	// Create test config
	theme, err := config.LoadTheme("default")
	if err != nil {
		// Fallback to creating a minimal theme for testing
		theme = &config.Theme{}
		theme.Colors.Primary = "#00AAFF"
		theme.Colors.Success = "#00AA00"
		theme.Colors.Warning = "#FFAA00"
		theme.Colors.Error = "#FF0000"
		theme.Colors.TextPrimary = "#FFFFFF"
		theme.Colors.TextSecondary = "#CCCCCC"
		theme.Colors.TextMuted = "#888888"
		theme.Colors.Prompt = "#00AAFF"
		theme.Colors.Separator = "#00AAFF"
		theme.Colors.Activity = "#FFFFFF"
		theme.Colors.Background = "#000000"
		theme.Colors.Surface = "#111111"
	}
	// Create styles (use theme directly since cfg is only needed for config loading)
	styles := NewStyles(theme)

	t.Run("PlanningTabCreation", func(t *testing.T) {
		// Test ACP-based planning tab creation with session management
		contextTracker := NewContextTracker()
		sessionManager := createTestSessionManager()

		tabManager := NewTabManager()

		// Test planning tab creation with session management
		planningTab, err := tabManager.CreateAndAddPlanningTab(
			styles,
			contextTracker,
			sessionManager,
		)

		if err != nil {
			// This is expected to fail in test environment without ACP
			if !strings.Contains(err.Error(), "ACP") && !strings.Contains(err.Error(), "kiro-cli") {
				t.Errorf("Unexpected error creating planning tab: %v", err)
				return
			}
			t.Logf("Expected ACP connection error in test environment: %v", err)

			// Create planning tab directly without ACP connection for testing
			planningTab := NewPlanningTabWithSession("test-plan-1", "Test Plan", styles, contextTracker, sessionManager, nil)
			if planningTab == nil {
				t.Error("Direct planning tab creation failed")
				return
			}

			// Continue with testing tab behavior independently
			// Test tab properties
			if planningTab.Type() != TabTypePlanning {
				t.Error("Expected TabTypePlanning")
			}

			if !strings.Contains(planningTab.Title(), "Plan") {
				t.Error("Expected planning tab title to contain 'Plan'")
			}

			// Test initial state
			if planningTab.GetMessageCount() != 0 {
				t.Error("Expected new planning tab to have no messages")
			}

			// Test textinput starts empty (no placeholder artifact like "T")
			if planningTab.textinput.Value() != "" {
				t.Errorf("Expected empty textinput value, got %q", planningTab.textinput.Value())
			}
			if planningTab.textinput.Placeholder != "" {
				t.Errorf("Expected empty placeholder to avoid virtual cursor artifact, got %q", planningTab.textinput.Placeholder)
			}

			// Test adding messages
			planningTab.AddMessage("user", "Test message")
			if planningTab.GetMessageCount() != 1 {
				t.Error("Expected 1 message after adding user message")
			}

			// Test graceful degradation
			planningTab.SetReadOnly()
			if planningTab.IsActive() {
				t.Error("Expected tab to not be active in read-only mode")
			}
			return
		}

		if planningTab == nil {
			t.Error("Planning tab creation returned nil")
			return
		}

		// Test tab properties
		if planningTab.Type() != TabTypePlanning {
			t.Error("Expected TabTypePlanning")
		}

		if !strings.Contains(planningTab.Title(), "Plan") {
			t.Error("Expected planning tab title to contain 'Plan'")
		}

		// Test initial state
		if planningTab.GetMessageCount() != 0 {
			t.Error("Expected new planning tab to have no messages")
		}

		// Test adding messages
		planningTab.AddMessage("user", "Test message")
		if planningTab.GetMessageCount() != 1 {
			t.Error("Expected 1 message after adding user message")
		}

		// Test session state management
		planningTab.SaveSession()

		// Test graceful degradation
		planningTab.SetReadOnly()
		if planningTab.IsActive() {
			t.Error("Expected tab to not be active in read-only mode")
		}
	})

	t.Run("ErrorHandlingAndGracefulDegradation", func(t *testing.T) {
		contextTracker := NewContextTracker()

		// Test context tracker validation
		err := contextTracker.StartPlanningSessionWithValidation("")
		if err == nil {
			t.Error("Expected error for empty model name")
		}

		err = contextTracker.StartPlanningSessionWithValidation("claude-sonnet-4")
		if err != nil {
			t.Errorf("Unexpected error for valid model: %v", err)
		}

		// Test context tracking functionality
		if !contextTracker.IsActive() {
			t.Error("Expected context tracker to be active")
		}

		usage := contextTracker.FormatContextUsage()
		if usage == "" {
			t.Error("Expected non-empty context usage format")
		}

		// Test cleanup
		contextTracker.StopPlanningSession()
		if contextTracker.IsActive() {
			t.Error("Expected context tracker to be inactive after stop")
		}
	})

	t.Run("TabManagerIntegration", func(t *testing.T) {
		tabManager := NewTabManager()

		// Add main tab
		mainTab := NewMainTab()
		tabManager.AddTab(mainTab)

		// Test tab limit enforcement
		for i := 0; i < MaxPlanningTabs+1; i++ {
			_, err := tabManager.CreateAndAddPlanningTab(
				styles,
				NewContextTracker(),
				createTestSessionManager(),
			)

			if i < MaxPlanningTabs {
				// Should succeed for first MaxPlanningTabs attempts
				if err != nil && !strings.Contains(err.Error(), "ACP") {
					t.Errorf("Unexpected error creating planning tab %d: %v", i, err)
				}
			} else {
				// Should fail when limit exceeded
				if err == nil {
					t.Error("Expected error when exceeding planning tab limit")
				} else if !strings.Contains(err.Error(), "maximum") {
					t.Errorf("Expected limit error, got: %v", err)
				}
			}
		}

		// Test tab navigation
		tabs := tabManager.GetTabs()
		if len(tabs) < 1 {
			t.Error("Expected at least main tab to be present")
		}

		// Test active tab switching
		if tabManager.GetActiveTabIndex() != 0 {
			t.Error("Expected first tab to be active initially")
		}
	})

	t.Run("ResourceCleanupAndRecovery", func(t *testing.T) {
		// Test session cleanup scenarios
		sessionManager := createTestSessionManager()

		// Test cleanup on exit
		err := sessionManager.CleanupOnExit()
		if err != nil {
			t.Logf("Session cleanup warning (expected in test): %v", err)
		}

		// Test corrupted session handling
		sessions, err := sessionManager.List()
		if err != nil && !strings.Contains(err.Error(), "no sessions") {
			t.Errorf("Unexpected error listing sessions: %v", err)
		}

		// Should have no sessions in clean test environment
		if len(sessions) > 0 {
			t.Logf("Found %d existing sessions in test environment", len(sessions))
		}

		// Verify graceful handling of non-existent sessions
		_, err = sessionManager.Load("non-existent-session")
		if err == nil {
			t.Error("Expected error loading non-existent session")
		}
	})

	t.Run("BackwardCompatibility", func(t *testing.T) {
		// Test that classic planning functionality still works
		sessionManager := createTestSessionManager()

		// This should not break existing session types
		_, err := sessionManager.List()
		if err != nil && !strings.Contains(err.Error(), "no sessions") {
			t.Errorf("Session manager should handle empty session list gracefully: %v", err)
		}

		// Test that existing command structure is preserved
		contextTracker := NewContextTracker()
		if contextTracker == nil {
			t.Error("Context tracker initialization failed")
		}
	})
}

// Helper function to create a test session manager
func createTestSessionManager() *session.SessionManager {
	dir, err := os.MkdirTemp("", "kairon-test-sessions-*")
	if err != nil {
		// Fallback to default if temp dir creation fails
		return session.NewSessionManager()
	}
	return session.NewSessionManagerWithDir(dir)
}
