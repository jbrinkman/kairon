package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jbrinkman/kairon/internal/acp"
	"github.com/jbrinkman/kairon/internal/logging"
	"github.com/jbrinkman/kairon/internal/session"
)

// PlanningMessage represents a single message in the planning conversation
type PlanningMessage struct {
	Role      string    // "user" or "assistant"
	Content   string    // message content
	Timestamp time.Time // when the message was created
}

// PlanningTab implements a chat-like interface for planning sessions
type PlanningTab struct {
	id     string
	title  string
	width  int
	height int
	state  session.PlanningTabState

	// Chat components
	viewport  viewport.Model  // Scrollable message history
	textinput textinput.Model // Simple terminal-style message input
	messages  []PlanningMessage

	// ACP integration
	acpClient acp.Client

	// UI state
	styles      *Styles
	focusTarget FocusTarget // Which input has focus
	inputHeight int         // Height reserved for input area

	// Streaming state
	streamingResponse bool
	currentResponse   strings.Builder
	streamChan        <-chan *acp.StreamingResponse
	streamCancel      context.CancelFunc

	// Lifecycle context - cancelled when tab is closed
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc

	// Context tracking
	contextTracker *ContextTracker

	// Session management
	sessionManager *session.SessionManager
	sessionID      string
}

// sanitizeErrorMessage parses and formats error messages into user-friendly text.
// It handles JSON-RPC error structures and plain text errors, truncating very long messages.
func sanitizeErrorMessage(rawError string) string {
	if rawError == "" {
		return "An unknown error occurred"
	}

	// Strip any prefix before JSON (e.g., "failed to send prompt: {...}")
	// Look for the first '{' to find embedded JSON
	jsonStart := strings.Index(rawError, "{")
	var jsonStr string
	if jsonStart >= 0 {
		jsonStr = rawError[jsonStart:]
	} else {
		jsonStr = rawError
	}

	// Try to parse as JSON-RPC error structure
	var jsonError map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonError); err == nil {
		// Check for data field (can be string or object in JSON-RPC)
		if data, ok := jsonError["data"]; ok && data != nil {
			// Handle data as string (most common case)
			if dataStr, ok := data.(string); ok && dataStr != "" {
				return dataStr
			}
			// Handle data as object
			if dataObj, ok := data.(map[string]interface{}); ok {
				if message, ok := dataObj["message"].(map[string]interface{}); ok {
					if code, ok := message["code"].(string); ok && code != "" {
						return code
					}
				}
				// Check for direct message in data
				if message, ok := dataObj["message"].(string); ok && message != "" {
					return message
				}
			}
		}
		// Check for direct error message field
		if message, ok := jsonError["message"].(string); ok && message != "" {
			return message
		}
		if errorMsg, ok := jsonError["error"].(string); ok && errorMsg != "" {
			return errorMsg
		}
	}

	// Use as plain text error, cleaning up common patterns
	cleanError := strings.TrimSpace(rawError)

	// Remove "Error: " prefix if present to avoid duplication
	cleanError = strings.TrimPrefix(cleanError, "Error: ")
	cleanError = strings.TrimPrefix(cleanError, "error: ")

	// Truncate very long errors
	const maxLength = 500
	if len(cleanError) > maxLength {
		cleanError = cleanError[:maxLength] + "... (truncated)"
	}

	if cleanError == "" {
		return "An unknown error occurred"
	}

	return cleanError
}

// Planning tab messages
type planningResponseMsg struct {
	content    string
	isError    bool
	isComplete bool
}

type planningStreamMsg struct {
	response *acp.StreamingResponse
}

type planningStreamStartMsg struct {
	streamChan   <-chan *acp.StreamingResponse
	streamCancel context.CancelFunc
}

// Focus transfer message for coordinating between message input and footer input
type focusTransferMsg struct {
	target string // "message" or "footer"
}

// NewPlanningTab creates a new planning tab
func NewPlanningTab(id, title string, styles *Styles, contextTracker *ContextTracker) *PlanningTab {
	return NewPlanningTabWithSession(id, title, styles, contextTracker, nil, nil)
}

// NewPlanningTabWithSession creates a new planning tab with session management
func NewPlanningTabWithSession(id, title string, styles *Styles, contextTracker *ContextTracker, sessionManager *session.SessionManager, acpClient acp.Client) *PlanningTab {
	logging.Info("creating planning tab", "tab_id", id, "title", title)

	// Create viewport for message history
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.KeyMap = viewport.KeyMap{} // Disable built-in keybindings - we'll handle them

	// Create simple textinput for message input with terminal prompt style
	ti := textinput.New()
	ti.Placeholder = "" // No placeholder — avoids virtual cursor rendering first char as cursor glyph
	ti.Prompt = ""      // We'll render the prompt ourselves for consistent styling
	ti.CharLimit = 4000 // Reasonable message limit

	// Configure solid cursor (non-blinking)
	currentStyles := ti.Styles()
	currentStyles.Cursor.Blink = false
	ti.SetStyles(currentStyles)

	ti.Focus() // Start focused since focusTarget defaults to FocusTargetMessage

	pt := &PlanningTab{
		id:             id,
		title:          title,
		state:          session.PlanningStateIdle,
		viewport:       vp,
		textinput:      ti,
		messages:       make([]PlanningMessage, 0),
		styles:         styles,
		focusTarget:    FocusTargetMessage, // Start with message input focused
		inputHeight:    1,                  // Height for prompt line (separator handled by footer system)
		contextTracker: contextTracker,
		sessionManager: sessionManager,
	}

	// Initialize ACP client with provided client or create default
	if acpClient == nil {
		logging.Debug("creating default ACP client for planner agent", "tab_id", id)

		// Get current working directory for ACP session
		cwd, err := os.Getwd()
		if err != nil {
			logging.Warn("failed to get current working directory, using fallback", "tab_id", id, "error", err)
			// Use absolute fallback when Getwd fails
			cwd = os.TempDir()
		}

		// Ensure absolute path unconditionally
		if !filepath.IsAbs(cwd) {
			absCwd, err := filepath.Abs(cwd)
			if err != nil {
				// Final fallback to temp dir if Abs fails
				logging.Warn("failed to convert to absolute path, using temp dir", "tab_id", id, "error", err)
				cwd = os.TempDir()
			} else {
				cwd = absCwd
			}
		}

		logging.Debug("initializing ACP client with working directory", "tab_id", id, "cwd", cwd)

		config := acp.DefaultConnectionConfig()
		config.Agent = "planner"
		config.Cwd = cwd
		acpClient = acp.NewClient(config)
	} else {
		logging.Debug("using provided ACP client", "tab_id", id)
	}
	pt.acpClient = acpClient

	// Create or load session if session manager is provided
	if sessionManager != nil {
		logging.Debug("initializing session", "tab_id", id)
		pt.initializeSession()
	}

	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	pt.lifecycleCtx = lifecycleCtx
	pt.lifecycleCancel = lifecycleCancel

	logging.Info("planning tab created", "tab_id", id, "state", pt.state)
	return pt
}

// ID returns the tab identifier
func (pt *PlanningTab) ID() string {
	return pt.id
}

// Type returns the tab type
func (pt *PlanningTab) Type() TabType {
	return TabTypePlanning
}

// Title returns the tab title with state indicator
func (pt *PlanningTab) Title() string {
	switch pt.state {
	case session.PlanningStateActive:
		return pt.title + "*"
	case session.PlanningStateCompleted:
		return pt.title + "✓"
	case session.PlanningStateFailed:
		return pt.title + "✗"
	case session.PlanningStateReadOnly:
		return pt.title + " (RO)"
	default:
		return pt.title
	}
}

// IsClosable returns whether this tab can be closed
func (pt *PlanningTab) IsClosable() bool {
	return pt.state != session.PlanningStateActive
}

// GetState returns the current planning state
func (pt *PlanningTab) GetState() session.PlanningTabState {
	return pt.state
}

// Session Management Methods

// initializeSession creates or loads a planning session
func (pt *PlanningTab) initializeSession() {
	if pt.sessionManager == nil {
		return
	}

	// Try to find existing session for this tab
	existingSessionID, existingState, err := pt.sessionManager.FindPlanningSessionByTabID(pt.id)
	if err == nil {
		// Load existing session
		pt.sessionID = existingSessionID
		pt.loadSessionState(existingState)
		return
	}

	// Create new session
	sessionID, _, err := pt.sessionManager.CreatePlanningSession(pt.id, pt.title)
	if err != nil {
		// Handle error but continue without session persistence
		return
	}

	pt.sessionID = sessionID
	pt.saveSessionState()
}

// loadSessionState restores tab state from session data
func (pt *PlanningTab) loadSessionState(state *session.SessionState) {
	if !state.IsPlanning() {
		return
	}

	// Restore basic tab info
	pt.title = state.PlanningData.Title
	pt.state = state.PlanningData.State

	// Restore conversation history
	pt.messages = make([]PlanningMessage, 0, len(state.History))
	for _, msg := range state.History {
		pt.messages = append(pt.messages, PlanningMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		})
	}

	// Update context tracker if available
	if pt.contextTracker != nil {
		pt.contextTracker.UpdateContextUsage(state.PlanningData.ContextUsage.Used)
		if state.PlanningData.ACPConnection.Model != "" {
			pt.contextTracker.UpdateModel(state.PlanningData.ACPConnection.Model)
		}
	}

	// Update viewport content
	pt.updateViewportContent()

	// Set read-only state if needed
	if pt.state == session.PlanningStateReadOnly {
		pt.focusTarget = FocusTargetFooter
	}
}

// toSessionMessages converts internal PlanningMessage slice to session.Message format
func (pt *PlanningTab) toSessionMessages() []session.Message {
	messages := make([]session.Message, 0, len(pt.messages))
	for _, msg := range pt.messages {
		messages = append(messages, session.Message{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		})
	}
	return messages
}

// saveSessionState persists current tab state to session
func (pt *PlanningTab) saveSessionState() {
	if pt.sessionManager == nil || pt.sessionID == "" {
		return
	}

	// Convert messages to session format
	messages := pt.toSessionMessages()

	// Load current session state
	state, err := pt.sessionManager.LoadPlanningSession(pt.sessionID)
	if err != nil {
		return
	}

	// Update session with current tab state
	state.History = messages
	state.PlanningData.Title = pt.title
	state.PlanningData.State = pt.state

	// Save quietly (background persistence)
	pt.sessionManager.SaveQuiet(pt.sessionID, state)
}

// SaveSession forces an immediate session save (for important state changes)
func (pt *PlanningTab) SaveSession() {
	if pt.sessionManager == nil || pt.sessionID == "" {
		return
	}

	state, err := pt.sessionManager.LoadPlanningSession(pt.sessionID)
	if err != nil {
		return
	}

	// Convert and update messages
	messages := pt.toSessionMessages()

	state.History = messages
	state.PlanningData.Title = pt.title
	state.PlanningData.State = pt.state

	// Full save with validation
	pt.sessionManager.Save(pt.sessionID, state)
}

// UpdateSessionACP updates ACP connection metadata in session
func (pt *PlanningTab) UpdateSessionACP(connected bool, agent, model string) {
	if pt.sessionManager == nil || pt.sessionID == "" {
		return
	}

	pt.sessionManager.UpdatePlanningSessionACP(pt.sessionID, connected, agent, model)
}

// UpdateSessionContext updates context usage in session
func (pt *PlanningTab) UpdateSessionContext(used, total int) {
	if pt.sessionManager == nil || pt.sessionID == "" {
		return
	}

	pt.sessionManager.UpdatePlanningSessionContext(pt.sessionID, used, total)
}

// CleanupSession removes the session when tab is closed
func (pt *PlanningTab) CleanupSession() {
	if pt.sessionManager == nil || pt.sessionID == "" {
		return
	}

	// Only cleanup if not in a state that should be preserved
	if pt.state != session.PlanningStateCompleted && pt.state != session.PlanningStateFailed {
		pt.sessionManager.Delete(pt.sessionID)
	} else {
		// Mark as completed in session for potential cleanup later
		pt.sessionManager.UpdatePlanningSessionState(pt.sessionID, pt.state)
	}
}

// AddMessage adds a message to the conversation history
func (pt *PlanningTab) AddMessage(role, content string) {
	logging.Debug("adding message to conversation", "tab_id", pt.id, "role", role, "content_length", len(content))

	message := PlanningMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
	pt.messages = append(pt.messages, message)
	pt.updateViewportContent()

	// Save to session
	pt.saveSessionState()

	logging.Info("message added", "tab_id", pt.id, "role", role, "message_count", len(pt.messages))
}

// updateViewportContent rebuilds the viewport content from messages with minimal styling
func (pt *PlanningTab) updateViewportContent() {
	var content strings.Builder

	for i, msg := range pt.messages {
		if i > 0 {
			content.WriteString("\n")
		}

		// Get minimal message style
		messageStyle := pt.styles.GetPlanningMessageStyle(msg.Role, pt.width)

		// Format message based on role with clean minimal styling
		if msg.Role == "user" {
			// User messages with simple prefix
			prefix := pt.styles.PlanningPrompt.Render("[planner]")
			userContent := prefix + " " + msg.Content
			content.WriteString(messageStyle.Render(userContent))
		} else {
			// Assistant messages with minimal formatting
			assistantContent := msg.Content

			// Add error styling for error messages (case-insensitive)
			contentLower := strings.ToLower(assistantContent)
			if strings.Contains(contentLower, "error:") {
				assistantContent = pt.styles.PlanningError.Render(assistantContent)
			} else {
				assistantContent = messageStyle.Render(assistantContent)
			}

			content.WriteString(assistantContent)
		}
	}

	// Add current streaming response with minimal indicator if active
	if pt.streamingResponse && pt.currentResponse.Len() > 0 {
		if len(pt.messages) > 0 {
			content.WriteString("\n")
		}

		// Minimal streaming indicator
		streamingText := pt.currentResponse.String()
		indicator := pt.styles.PlanningStreamingIndicator.Render("● ")

		styledResponse := pt.styles.PlanningAssistant.Render(streamingText)
		content.WriteString(indicator + styledResponse)
	}

	pt.viewport.SetContent(content.String())

	// Auto-scroll to bottom for new messages
	if pt.viewport.ScrollPercent() >= 0.85 || len(pt.messages) == 1 {
		pt.viewport.GotoBottom()
	}
}

// sendMessage sends a message to the agent via ACP
func (pt *PlanningTab) sendMessage(message string) tea.Cmd {
	logging.Info("sending message to ACP", "tab_id", pt.id, "message_length", len(message))

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(pt.lifecycleCtx, 60*time.Second)

		// Ensure ACP connection
		if !pt.acpClient.IsConnected() {
			logging.Warn("ACP not connected, attempting connection", "tab_id", pt.id)
			if err := pt.acpClient.Connect(ctx); err != nil {
				cancel()
				logging.Error("failed to connect to planner agent via ACP", "tab_id", pt.id, "error", err)
				return planningResponseMsg{
					content:    fmt.Sprintf("Failed to connect to planner agent: %v", err),
					isError:    true,
					isComplete: true,
				}
			}
			logging.Info("ACP connection established for planner agent", "tab_id", pt.id)
		}

		// Create message request
		req := &acp.MessageRequest{
			Agent:          "", // Agent field deprecated - agent context established at connection level
			Message:        message,
			Streaming:      true,
			ResponseFormat: "text",
			Timeout:        60 * time.Second,
		}

		logging.Debug("creating ACP stream", "tab_id", pt.id, "agent", "planner")

		// Send streaming request
		streamChan, err := pt.acpClient.StreamMessage(ctx, req)
		if err != nil {
			cancel()
			logging.Error("failed to send ACP message", "tab_id", pt.id, "error", err)
			return planningResponseMsg{
				content:    fmt.Sprintf("Failed to send message: %v", err),
				isError:    true,
				isComplete: true,
			}
		}

		logging.Info("ACP stream created", "tab_id", pt.id)

		// Return streaming start message instead of storing directly
		return planningStreamStartMsg{
			streamChan:   streamChan,
			streamCancel: cancel,
		}
	}
}

// listenToStream creates a command that reads the next message from the stream channel.
// Each invocation reads exactly one message; the Update handler issues continuation
// commands to read subsequent messages until "done" or "error" arrives.
func (pt *PlanningTab) listenToStream() tea.Cmd {
	ch := pt.streamChan
	return func() tea.Msg {
		if ch == nil {
			return planningResponseMsg{
				content:    "",
				isError:    false,
				isComplete: true,
			}
		}
		response, ok := <-ch
		if !ok {
			// Channel closed — stream ended without explicit done signal
			return planningResponseMsg{
				content:    "",
				isError:    false,
				isComplete: true,
			}
		}
		return planningStreamMsg{response: response}
	}
}

// View renders the planning tab content with minimal clean styling
// This method renders only the tab's content area - the unified rendering system
// in tui.go will add the footer below this content
func (pt *PlanningTab) View() string {
	if pt.height == 0 || pt.width == 0 {
		return ""
	}

	// Calculate dimensions for clean minimal layout
	messageHeight := pt.height - pt.inputHeight
	if messageHeight < 1 {
		messageHeight = 1
	}

	// Update viewport dimensions to use full available width
	pt.viewport.SetWidth(pt.width)
	pt.viewport.SetHeight(messageHeight)

	// Render message history directly without any containers or borders
	messageArea := pt.viewport.View()

	// Render input area with minimal terminal-style prompt
	inputArea := pt.renderInputArea()

	// Combine all parts with minimal clean layout - no separator
	// The unified rendering system in tui.go will add the footer below this content
	return lipgloss.JoinVertical(
		lipgloss.Left,
		messageArea,
		inputArea,
	)
}

// renderInputArea renders the clean terminal-style input prompt without borders or padding
func (pt *PlanningTab) renderInputArea() string {
	// Build the minimal terminal-style prompt based on state
	var prompt string
	var promptStyle lipgloss.Style

	switch pt.state {
	case session.PlanningStateActive:
		prompt = "[planner] ● "
		promptStyle = pt.styles.PlanningStreamingIndicator
	case session.PlanningStateReadOnly:
		prompt = "[planner] 🔒 "
		promptStyle = pt.styles.Warning
	case session.PlanningStateCompleted:
		prompt = "[planner] ✓ "
		promptStyle = pt.styles.Success
	case session.PlanningStateFailed:
		prompt = "[planner] ✗ "
		promptStyle = pt.styles.Error
	default:
		prompt = "[planner] > "
		promptStyle = pt.styles.PlanningPrompt
	}

	// Render clean prompt - no borders, no padding, no background
	styledPrompt := promptStyle.Render(prompt)

	// Only show textinput if not in read-only state
	if pt.state == session.PlanningStateReadOnly {
		return styledPrompt
	}

	return styledPrompt + pt.textinput.View()
}

// Update handles messages for the planning tab
func (pt *PlanningTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Handle key presses based on current state
		if pt.state == session.PlanningStateReadOnly {
			// In read-only mode, only allow scrolling
			switch msg.String() {
			case "up":
				pt.viewport.ScrollUp(1)
			case "down":
				pt.viewport.ScrollDown(1)
			case "pgup":
				pt.viewport.HalfPageUp()
			case "pgdown":
				pt.viewport.HalfPageDown()
			case "home":
				pt.viewport.GotoTop()
			case "end":
				pt.viewport.GotoBottom()
			}
			return pt, nil
		}

		// Handle input focus and message sending
		switch msg.String() {
		case "enter":
			if pt.focusTarget == FocusTargetMessage && pt.state != session.PlanningStateActive {
				// Send message
				message := strings.TrimSpace(pt.textinput.Value())
				if message != "" {
					logging.Info("user message submitted", "tab_id", pt.id, "message_length", len(message))

					// Add user message
					pt.AddMessage("user", message)

					// Clear input
					pt.textinput.SetValue("")

					// Start streaming response
					oldState := pt.state
					pt.state = session.PlanningStateActive
					pt.streamingResponse = true
					pt.currentResponse.Reset()

					logging.Info("state transition", "tab_id", pt.id, "from", oldState, "to", pt.state)

					// Save state change
					pt.saveSessionState()

					// Send message to agent
					cmds = append(cmds, pt.sendMessage(message))
				}
			}
		case "up":
			if pt.focusTarget != FocusTargetMessage {
				pt.viewport.ScrollUp(1)
			}
		case "down":
			if pt.focusTarget != FocusTargetMessage {
				pt.viewport.ScrollDown(1)
			}
		case "pgup":
			pt.viewport.HalfPageUp()
			pt.focusTarget = FocusTargetFooter
			pt.textinput.Blur()
		case "pgdown":
			pt.viewport.HalfPageDown()
			pt.focusTarget = FocusTargetFooter
			pt.textinput.Blur()
		case "home":
			pt.viewport.GotoTop()
			pt.focusTarget = FocusTargetFooter
			pt.textinput.Blur()
		case "end":
			pt.viewport.GotoBottom()
			pt.focusTarget = FocusTargetFooter
			pt.textinput.Blur()
		case "esc":
			// Transfer focus from message input to footer
			if pt.focusTarget == FocusTargetMessage {
				logging.Debug("focus transfer to footer", "tab_id", pt.id)
				pt.focusTarget = FocusTargetFooter
				pt.textinput.Blur()
				// Send focus transfer message to coordinate with parent
				cmds = append(cmds, func() tea.Msg {
					return focusTransferMsg{target: "footer"}
				})
			}

		default:
			if pt.focusTarget == FocusTargetMessage {
				// Forward to textinput
				var cmd tea.Cmd
				pt.textinput, cmd = pt.textinput.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

	case planningStreamStartMsg:
		logging.Info("streaming start", "tab_id", pt.id)
		pt.streamChan = msg.streamChan
		pt.streamCancel = msg.streamCancel
		// Start listening to the stream
		cmds = append(cmds, pt.listenToStream())

	case planningStreamMsg:
		// Handle streaming response
		response := msg.response
		switch response.Type {
		case "start":
			// Stream started — issue continuation to read the first real chunk
			logging.Debug("stream start event received", "tab_id", pt.id)
			if pt.streamingResponse {
				cmds = append(cmds, pt.listenToStream())
			}

		case "text":
			// Append text to current response
			logging.Debug("stream text received", "tab_id", pt.id, "content_length", len(response.Content))
			pt.currentResponse.WriteString(response.Content)
			pt.updateViewportContent()

			// Issue continuation command to read the next chunk
			if pt.streamingResponse {
				cmds = append(cmds, pt.listenToStream())
			}

		case "error":
			// Handle error response
			logging.Error("stream error received", "tab_id", pt.id, "error", response.Error)
			pt.streamingResponse = false
			oldState := pt.state
			pt.cancelStream()

			// Sanitize error message for user display
			sanitizedError := sanitizeErrorMessage(response.Error)
			errorMessage := fmt.Sprintf("Error: %s", sanitizedError)

			// Transition to Idle state BEFORE AddMessage to ensure correct state persistence
			pt.state = session.PlanningStateIdle
			logging.Info("state transition on error", "tab_id", pt.id, "from", oldState, "to", pt.state)

			// Preserve accumulated response before displaying error
			accumulatedResponse := pt.currentResponse.String()
			if accumulatedResponse != "" {
				pt.AddMessage("assistant", accumulatedResponse)
			}
			pt.currentResponse.Reset()

			// Add sanitized error message
			pt.AddMessage("assistant", errorMessage)

			// Force viewport to scroll to error message
			pt.viewport.GotoBottom()

		case "done":
			// Complete the response
			logging.Info("stream done event received", "tab_id", pt.id, "response_length", pt.currentResponse.Len())
			pt.streamingResponse = false
			oldState := pt.state
			pt.state = session.PlanningStateIdle
			pt.cancelStream()

			logging.Info("state transition on completion", "tab_id", pt.id, "from", oldState, "to", pt.state)

			// Finalize the assistant message
			if pt.currentResponse.Len() > 0 {
				pt.AddMessage("assistant", pt.currentResponse.String())
				pt.currentResponse.Reset()
			}
		}

	case planningResponseMsg:
		// Handle direct response (non-streaming)
		logging.Debug("direct response received", "tab_id", pt.id, "is_error", msg.isError, "is_complete", msg.isComplete)
		pt.streamingResponse = false

		if msg.isError {
			oldState := pt.state

			// Sanitize error message for user display
			sanitizedError := sanitizeErrorMessage(msg.content)
			errorMessage := fmt.Sprintf("Error: %s", sanitizedError)

			// Transition to Idle state BEFORE AddMessage to ensure correct state persistence
			pt.state = session.PlanningStateIdle
			logging.Info("state transition on error response", "tab_id", pt.id, "from", oldState, "to", pt.state)

			// Add sanitized error message
			pt.AddMessage("assistant", errorMessage)

			// Force viewport to scroll to error message
			pt.viewport.GotoBottom()
		} else if msg.isComplete {
			oldState := pt.state
			pt.state = session.PlanningStateCompleted
			logging.Info("state transition on completion", "tab_id", pt.id, "from", oldState, "to", pt.state)
		} else {
			pt.AddMessage("assistant", msg.content)
		}

	case tea.MouseClickMsg:
		// Handle mouse clicks on message input area
		mouse := msg.Mouse()

		// Calculate message input area boundaries
		// Input is at the bottom of the planning tab content area
		// The input line starts after the viewport (messageHeight) and has inputHeight lines
		inputStartY := pt.height - pt.inputHeight

		// Check if click is in the message input area
		if mouse.Y >= inputStartY && mouse.Y < pt.height {
			// Click is in the message input area
			if pt.focusTarget != FocusTargetMessage && pt.state != session.PlanningStateReadOnly {
				logging.Debug("mouse click focus transfer to message input",
					"tab_id", pt.id,
					"mouse_x", mouse.X,
					"mouse_y", mouse.Y,
					"input_start_y", inputStartY)

				pt.focusTarget = FocusTargetMessage
				// Send focus transfer message to update centralized state
				focusCmd := func() tea.Msg {
					return focusTransferMsg{target: "message"}
				}
				return pt, tea.Batch(pt.textinput.Focus(), focusCmd)
			}
		}
	}

	return pt, tea.Batch(cmds...)
}

// Resize updates the tab dimensions with footer-aware height calculation
// The height parameter represents the available space for the tab content only (footer excluded)
func (pt *PlanningTab) Resize(width, height int) {
	pt.width = width
	pt.height = height

	// Update component dimensions with footer space already excluded from height
	if width > 4 {
		pt.viewport.SetWidth(width)
	}

	// Recalculate message area height using the footer-aware height
	messageHeight := height - pt.inputHeight
	if messageHeight > 0 {
		pt.viewport.SetHeight(messageHeight)
	}
}

// SetCompleted sets the tab to completed state (successful GitHub issue creation)
func (pt *PlanningTab) SetCompleted() {
	pt.state = session.PlanningStateCompleted
	pt.focusTarget = FocusTargetFooter
	pt.SaveSession() // Persist important state change
}

// SetFailed sets the tab to failed state
func (pt *PlanningTab) SetFailed() {
	pt.state = session.PlanningStateFailed
	pt.focusTarget = FocusTargetFooter
	pt.SaveSession() // Persist important state change
}

// SetReadOnly sets the tab to read-only mode
func (pt *PlanningTab) SetReadOnly() {
	pt.state = session.PlanningStateReadOnly
	pt.focusTarget = FocusTargetFooter
	pt.SaveSession() // Persist important state change
}

// SetActive sets the tab to active mode
func (pt *PlanningTab) SetActive() {
	if pt.state == session.PlanningStateReadOnly {
		return // Can't change from read-only
	}
	pt.state = session.PlanningStateActive
	pt.saveSessionState()
}

// Reset clears the conversation and resets the tab state
func (pt *PlanningTab) Reset() {
	if pt.state == session.PlanningStateActive {
		return // Can't reset while active
	}

	pt.messages = make([]PlanningMessage, 0)
	pt.state = session.PlanningStateIdle
	pt.streamingResponse = false
	pt.currentResponse.Reset()
	pt.textinput.SetValue("")
	pt.focusTarget = FocusTargetMessage
	pt.updateViewportContent()
	pt.SaveSession() // Persist state reset
}

// Close cleans up resources when the tab is closed
func (pt *PlanningTab) Close() {
	pt.lifecycleCancel()
	pt.cancelStream()

	if pt.acpClient != nil {
		pt.acpClient.Close()
	}

	// Cleanup session
	pt.CleanupSession()
}

// cancelStream cancels the active stream context and clears stream state
func (pt *PlanningTab) cancelStream() {
	if pt.streamCancel != nil {
		pt.streamCancel()
		pt.streamCancel = nil
	}
	pt.streamChan = nil
}

// UpdateContextUsage updates the context usage for this planning session
func (pt *PlanningTab) UpdateContextUsage(used int) {
	if pt.contextTracker != nil {
		pt.contextTracker.UpdateContextUsage(used)
	}

	// Update session context usage
	total := 200000 // Default context limit
	if pt.contextTracker != nil && pt.contextTracker.IsActive() {
		if planningContext := pt.contextTracker.GetPlanningContext(); planningContext != nil {
			total = planningContext.Usage.Total
		}
	}
	pt.UpdateSessionContext(used, total)
}

// SetACPClient sets the ACP client for this planning tab
func (pt *PlanningTab) SetACPClient(client acp.Client) {
	if pt.acpClient != nil && pt.acpClient != client {
		pt.acpClient.Close()
	}
	pt.acpClient = client
}

// GetMessageCount returns the number of messages in the conversation
func (pt *PlanningTab) GetMessageCount() int {
	return len(pt.messages)
}

// GetLastMessage returns the last message in the conversation
func (pt *PlanningTab) GetLastMessage() *PlanningMessage {
	if len(pt.messages) == 0 {
		return nil
	}
	return &pt.messages[len(pt.messages)-1]
}

// SetTitle updates the tab title
func (pt *PlanningTab) SetTitle(title string) {
	pt.title = title
}

// IsActive returns true if the tab is currently processing a request
func (pt *PlanningTab) IsActive() bool {
	return pt.state == session.PlanningStateActive
}

// SetFocusInput sets the planning tab's focus target based on focused boolean.
// This method exists for backward compatibility and will be deprecated.
func (pt *PlanningTab) SetFocusInput(focused bool) {
	if focused {
		pt.focusTarget = FocusTargetMessage
	} else {
		pt.focusTarget = FocusTargetFooter
	}
}

// CaptureFocusState returns the current focus state for the planning tab
func (pt *PlanningTab) CaptureFocusState() FocusTarget {
	return pt.focusTarget
}

// RestoreFocusState restores the focus state for the planning tab
func (pt *PlanningTab) RestoreFocusState(target FocusTarget) tea.Cmd {
	switch target {
	case FocusTargetMessage:
		pt.focusTarget = FocusTargetMessage
		return pt.textinput.Focus()
	case FocusTargetFooter:
		pt.focusTarget = FocusTargetFooter
		pt.textinput.Blur()
		return nil
	default:
		// Invalid target, default to footer focus
		pt.focusTarget = FocusTargetFooter
		pt.textinput.Blur()
		return nil
	}
}

// RestoreFocus is kept for backward compatibility
// It now delegates to RestoreFocusState
func (pt *PlanningTab) RestoreFocus() tea.Cmd {
	target := pt.CaptureFocusState()
	return pt.RestoreFocusState(target)
}
