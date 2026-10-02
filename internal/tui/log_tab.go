package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/log"

	"github.com/jbrinkman/kairon/internal/logging"
)

const (
	// Poll interval for checking new log entries from ring buffer
	logPollInterval = 100 * time.Millisecond
	// Auto-scroll threshold - scroll to bottom only if viewport is within this percentage of bottom
	autoScrollThreshold = 0.85
)

// TickMsg is sent periodically to poll the ring buffer for new entries
type TickMsg time.Time

// LogTab implements the Tab interface for displaying live log streams
type LogTab struct {
	id       string
	viewport viewport.Model
	width    int
	height   int
	styles   *Styles

	// Ring buffer for log entries
	ringBuffer *logging.RingBuffer

	// Last known write counter to detect new entries (monotonic)
	lastWriteCounter int64

	// Track whether user has scrolled away from bottom (disable auto-scroll)
	userScrolled bool

	// Configuration
	level      string
	bufferSize int
}

// NewLogTab creates a new log viewer tab with the specified configuration
func NewLogTab(id string, level string, bufferSize int, styles *Styles) *LogTab {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))
	vp.MouseWheelEnabled = true

	// Create ring buffer with configured size
	if bufferSize <= 0 {
		bufferSize = logging.DefaultMaxBufferLines
	}
	ringBuffer := logging.NewRingBuffer(bufferSize)

	return &LogTab{
		id:               id,
		viewport:         vp,
		styles:           styles,
		ringBuffer:       ringBuffer,
		lastWriteCounter: 0,
		userScrolled:     false,
		level:            level,
		bufferSize:       bufferSize,
	}
}

// ID returns the tab identifier
func (lt *LogTab) ID() string {
	return lt.id
}

// Type returns the tab type
func (lt *LogTab) Type() TabType {
	return TabTypeLog
}

// Title returns the tab title
func (lt *LogTab) Title() string {
	return "Logs"
}

// IsClosable returns whether this tab can be closed
func (lt *LogTab) IsClosable() bool {
	return true // Log viewer is always closable
}

// View returns the tab's rendered content
func (lt *LogTab) View() string {
	return lt.viewport.View()
}

// Update handles messages for the log tab
func (lt *LogTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Track user scrolling to disable auto-scroll
		switch msg.String() {
		case "up", "k", "pgup", "home":
			lt.userScrolled = true
		case "down", "j", "pgdown":
			lt.userScrolled = true
		case "end":
			// Jump to bottom and re-enable auto-scroll
			lt.userScrolled = false
		}

		// Pass key event to viewport for scrolling
		lt.viewport, cmd = lt.viewport.Update(msg)

		// Re-enable auto-scroll if user has scrolled to near bottom (check after viewport update)
		if lt.userScrolled && lt.isNearBottom() {
			lt.userScrolled = false
		}

	case TickMsg:
		// Poll ring buffer for new entries
		lt.refreshContent()

		// Schedule next tick
		return lt, tea.Batch(cmd, tea.Tick(logPollInterval, func(t time.Time) tea.Msg {
			return TickMsg(t)
		}))

	case tea.WindowSizeMsg:
		// Handle resize in Resize method
	}

	return lt, cmd
}

// Resize updates the tab dimensions
func (lt *LogTab) Resize(width, height int) {
	lt.width = width
	lt.height = height
	lt.viewport.SetWidth(width)
	lt.viewport.SetHeight(height)
}

// refreshContent polls the ring buffer and updates the viewport with new log entries
func (lt *LogTab) refreshContent() {
	currentCounter := lt.ringBuffer.WriteCounter()

	// Check if there are new entries (write counter is monotonic)
	if currentCounter == lt.lastWriteCounter {
		return // No new entries
	}

	// Get all log entries from buffer
	entries := lt.ringBuffer.Get()

	// Format all entries for display
	var content strings.Builder
	for _, entry := range entries {
		content.WriteString(lt.formatLogEntry(entry))
		content.WriteString("\n")
	}

	// Update viewport content
	lt.viewport.SetContent(content.String())

	// Auto-scroll to bottom if user hasn't scrolled away
	if !lt.userScrolled || lt.isNearBottom() {
		lt.viewport.GotoBottom()
		lt.userScrolled = false // Re-enable auto-scroll
	}

	// Update last known write counter
	lt.lastWriteCounter = currentCounter
}

// formatLogEntry formats a single log entry with color-coded level
func (lt *LogTab) formatLogEntry(entry logging.LogEntry) string {
	// Format timestamp
	timestamp := entry.Timestamp.Format("15:04:05.000")

	// Color-code log level
	levelStr := lt.colorizeLevel(entry.Level)

	// Build formatted message
	var msg strings.Builder
	msg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(timestamp))
	msg.WriteString(" ")
	msg.WriteString(levelStr)
	msg.WriteString(" ")
	msg.WriteString(entry.Message)

	// Append metadata if present
	if len(entry.Metadata) > 0 {
		msg.WriteString(" ")
		msg.WriteString(lt.formatMetadata(entry.Metadata))
	}

	return msg.String()
}

// colorizeLevel returns a color-coded level string
func (lt *LogTab) colorizeLevel(level log.Level) string {
	var style lipgloss.Style
	var levelName string

	switch level {
	case log.DebugLevel:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")) // gray
		levelName = "DEBUG"
	case log.InfoLevel:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00AAFF")) // blue
		levelName = "INFO "
	case log.WarnLevel:
		style = lt.styles.Warning // yellow
		levelName = "WARN "
	case log.ErrorLevel:
		style = lt.styles.Error // red
		levelName = "ERROR"
	default:
		style = lipgloss.NewStyle()
		levelName = "UNKNOWN"
	}

	return style.Render(levelName)
}

// formatMetadata formats key-value metadata for display
func (lt *LogTab) formatMetadata(metadata map[string]interface{}) string {
	if len(metadata) == 0 {
		return ""
	}

	// Collect and sort keys for deterministic ordering
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	// Sort keys alphabetically
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	var pairs []string
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, metadata[k]))
	}

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	return style.Render(fmt.Sprintf("[%s]", strings.Join(pairs, " ")))
}

// isNearBottom returns true if the viewport is within the auto-scroll threshold of the bottom
func (lt *LogTab) isNearBottom() bool {
	if lt.viewport.TotalLineCount() == 0 {
		return true
	}

	// Calculate percentage of scroll position
	scrollPercentage := float64(lt.viewport.YOffset()+lt.viewport.Height()) / float64(lt.viewport.TotalLineCount())
	return scrollPercentage >= autoScrollThreshold
}

// GetRingBuffer returns the ring buffer for this log tab
// This is used by the TUI to write log entries to the buffer
func (lt *LogTab) GetRingBuffer() *logging.RingBuffer {
	return lt.ringBuffer
}

// StartPolling returns a command to start polling the ring buffer
func (lt *LogTab) StartPolling() tea.Cmd {
	return tea.Tick(logPollInterval, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Cleanup performs resource cleanup when the tab is closed
func (lt *LogTab) Cleanup() {
	// Clear the ring buffer
	if lt.ringBuffer != nil {
		lt.ringBuffer.Clear()
	}
}

// CaptureFocusState returns the current focus state for the log tab
func (lt *LogTab) CaptureFocusState() FocusTarget {
	// Log tab always uses footer input
	return FocusTargetFooter
}

// RestoreFocusState restores the focus state for the log tab
func (lt *LogTab) RestoreFocusState(target FocusTarget) tea.Cmd {
	// Log tab doesn't manage focus directly - handled by parent model
	return nil
}
