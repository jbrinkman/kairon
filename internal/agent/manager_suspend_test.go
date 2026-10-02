package agent

import (
	"bytes"
	"io"
	"testing"

	"github.com/jbrinkman/kairon/internal/config"
)

func TestManagerSuspendOnlyAffectsTerminalOutput(t *testing.T) {
	cfg := &config.Config{ConsoleLogging: true}
	manager := NewManager(cfg)

	// Verify initial state
	if manager.terminalOutputPaused {
		t.Fatal("Expected terminalOutputPaused to be false initially")
	}

	// Test SuspendOutputCapture only affects terminalOutputPaused flag
	manager.SuspendOutputCapture()
	if !manager.terminalOutputPaused {
		t.Fatal("Expected terminalOutputPaused to be true after suspend")
	}

	// Verify resume
	manager.ResumeOutputCapture()
	if manager.terminalOutputPaused {
		t.Fatal("Expected terminalOutputPaused to be false after resume")
	}
}

func TestCaptureWriterContinuesDuringsuspension(t *testing.T) {
	cfg := &config.Config{ConsoleLogging: true}
	manager := NewManager(cfg)

	// Create a buffer to capture terminal output
	var terminalBuffer bytes.Buffer

	// Create a capture writer that uses both terminal and capture
	captureWriter := NewCaptureWriter(&terminalBuffer, manager.outputCapture, "[agent issue-1] ")

	// Write data before suspension
	_, err := captureWriter.Write([]byte("before suspension\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify data appears in both places
	lines := manager.GetOutputLines()
	if len(lines) != 1 || lines[0] != "[agent issue-1] before suspension" {
		t.Fatalf("Expected capture to contain line, got: %v", lines)
	}
	if !bytes.Contains(terminalBuffer.Bytes(), []byte("before suspension")) {
		t.Fatal("Expected terminal buffer to contain output")
	}

	// Suspend output capture
	manager.SuspendOutputCapture()

	// Clear terminal buffer for next test
	terminalBuffer.Reset()

	// Write data during suspension - should still go to capture but not terminal
	_, err = captureWriter.Write([]byte("during suspension\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify capture still accumulates
	lines = manager.GetOutputLines()
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines in capture, got %d", len(lines))
	}
	if lines[1] != "[agent issue-1] during suspension" {
		t.Fatalf("Expected second line to be suspension message, got: %s", lines[1])
	}

	// Resume and verify
	manager.ResumeOutputCapture()

	// Write after resume
	_, err = captureWriter.Write([]byte("after resume\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify final state
	lines = manager.GetOutputLines()
	if len(lines) != 3 {
		t.Fatalf("Expected 3 lines total, got %d", len(lines))
	}

	expectedLines := []string{
		"[agent issue-1] before suspension",
		"[agent issue-1] during suspension",
		"[agent issue-1] after resume",
	}

	for i, expected := range expectedLines {
		if lines[i] != expected {
			t.Fatalf("Line %d: expected %q, got %q", i, expected, lines[i])
		}
	}
}

func TestGetOutputLinesReturnsCompleteDataAfterResume(t *testing.T) {
	cfg := &config.Config{ConsoleLogging: true}
	manager := NewManager(cfg)

	// Add lines before suspension
	manager.CaptureOutputLine(1, "line1")
	manager.CaptureOutputLine(1, "line2")

	initialGen := manager.GetOutputGeneration()

	// Suspend
	manager.SuspendOutputCapture()

	// Add lines during suspension
	manager.CaptureOutputLine(1, "line3")
	manager.CaptureOutputLine(1, "line4")

	suspendedGen := manager.GetOutputGeneration()
	if suspendedGen <= initialGen {
		t.Fatal("Expected generation to increase during suspension")
	}

	// Resume
	manager.ResumeOutputCapture()

	// Add lines after resume
	manager.CaptureOutputLine(1, "line5")

	finalGen := manager.GetOutputGeneration()
	if finalGen <= suspendedGen {
		t.Fatal("Expected generation to increase after resume")
	}

	// Verify complete data is available
	lines := manager.GetOutputLines()
	if len(lines) != 5 {
		t.Fatalf("Expected 5 lines total, got %d", len(lines))
	}

	expectedLines := []string{
		"[agent issue-1] line1",
		"[agent issue-1] line2",
		"[agent issue-1] line3",
		"[agent issue-1] line4",
		"[agent issue-1] line5",
	}

	for i, expected := range expectedLines {
		if lines[i] != expected {
			t.Fatalf("Line %d: expected %q, got %q", i, expected, lines[i])
		}
	}
}

func TestTerminalOutputEnabledFunction(t *testing.T) {
	cfg := &config.Config{ConsoleLogging: true}
	manager := NewManager(cfg)

	// Test initial state
	if !manager.terminalOutputEnabled() {
		t.Fatal("Expected terminal output to be enabled initially")
	}

	// Test after suspension
	manager.SuspendOutputCapture()
	if manager.terminalOutputEnabled() {
		t.Fatal("Expected terminal output to be disabled after suspension")
	}

	// Test after resume
	manager.ResumeOutputCapture()
	if !manager.terminalOutputEnabled() {
		t.Fatal("Expected terminal output to be enabled after resume")
	}
}

func TestConditionalWriterRespectsSuspension(t *testing.T) {
	cfg := &config.Config{ConsoleLogging: true}
	manager := NewManager(cfg)

	var buffer bytes.Buffer
	conditionalWriter := &conditionalWriter{
		writer:  &buffer,
		enabled: manager.terminalOutputEnabled,
	}

	// Write when enabled
	n, err := conditionalWriter.Write([]byte("enabled output"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len("enabled output") {
		t.Fatalf("Expected to write %d bytes, got %d", len("enabled output"), n)
	}
	if !bytes.Contains(buffer.Bytes(), []byte("enabled output")) {
		t.Fatal("Expected buffer to contain output when enabled")
	}

	// Suspend and write
	manager.SuspendOutputCapture()
	buffer.Reset()

	n, err = conditionalWriter.Write([]byte("suspended output"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len("suspended output") {
		t.Fatalf("Expected to report writing %d bytes, got %d", len("suspended output"), n)
	}
	if buffer.Len() != 0 {
		t.Fatal("Expected buffer to be empty when suspended")
	}

	// Resume and write
	manager.ResumeOutputCapture()
	n, err = conditionalWriter.Write([]byte("resumed output"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len("resumed output") {
		t.Fatalf("Expected to write %d bytes, got %d", len("resumed output"), n)
	}
	if !bytes.Contains(buffer.Bytes(), []byte("resumed output")) {
		t.Fatal("Expected buffer to contain output when resumed")
	}
}

// Test that agent log files continue being written during suspension
func TestAgentLogFilesContinueDuringSuspension(t *testing.T) {
	cfg := &config.Config{ConsoleLogging: true}
	manager := NewManager(cfg)

	// Create a mock log writer
	var logBuffer bytes.Buffer

	// Create writers similar to what's used in agent spawning
	captureWriter := manager.createCaptureWriter(123)
	prefixedWriter := manager.createPrefixedWriter(123)

	// Create multi-writer like in agent spawning
	multiWriter := io.MultiWriter(&logBuffer, captureWriter, prefixedWriter)

	// Write before suspension
	multiWriter.Write([]byte("log entry 1\n"))

	// Verify initial state
	if !bytes.Contains(logBuffer.Bytes(), []byte("log entry 1")) {
		t.Fatal("Expected log buffer to contain entry before suspension")
	}

	lines := manager.GetOutputLines()
	if len(lines) != 1 || !bytes.Contains([]byte(lines[0]), []byte("log entry 1")) {
		t.Fatal("Expected capture to contain entry before suspension")
	}

	// Suspend output
	manager.SuspendOutputCapture()
	logBuffer.Reset() // Clear to test new writes

	// Write during suspension
	multiWriter.Write([]byte("log entry during suspension\n"))

	// Verify log file continues to be written
	if !bytes.Contains(logBuffer.Bytes(), []byte("log entry during suspension")) {
		t.Fatal("Expected log buffer to continue receiving writes during suspension")
	}

	// Verify capture continues to accumulate
	lines = manager.GetOutputLines()
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines in capture during suspension, got %d", len(lines))
	}

	// Resume and verify
	manager.ResumeOutputCapture()
	multiWriter.Write([]byte("log entry after resume\n"))

	// Final verification
	lines = manager.GetOutputLines()
	if len(lines) != 3 {
		t.Fatalf("Expected 3 lines total after resume, got %d", len(lines))
	}
}
