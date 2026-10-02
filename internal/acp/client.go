package acp

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/jbrinkman/kairon/internal/logging"
)

// KiroACPClient implements the Client interface using the official ACP SDK
type KiroACPClient struct {
	config    *ConnectionConfig
	conn      *acp.ClientSideConnection
	cmd       *exec.Cmd
	connected bool
	mu        sync.RWMutex
	client    acp.Client
	sessionID string
}

// KiroClient implements the acp.Client interface for permission handling
type KiroClient struct {
	autoApprove bool
	respChan    chan<- *StreamingResponse
	mu          sync.Mutex
}

// RequestPermission handles permission requests from agents
func (k *KiroClient) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	logging.Debug("permission request received", "options_count", len(params.Options))

	if len(params.Options) == 0 {
		logging.Error("no permission options available")
		return acp.RequestPermissionResponse{}, fmt.Errorf("no permission options available")
	}

	// Auto-approve: prefer "allow" option, fall back to first available
	for _, opt := range params.Options {
		if opt.Kind == "allow" {
			logging.Info("auto-approving permission", "option_id", opt.OptionId, "kind", opt.Kind)
			return acp.RequestPermissionResponse{
				Outcome: acp.RequestPermissionOutcome{
					Selected: &acp.RequestPermissionOutcomeSelected{
						OptionId: opt.OptionId,
						Outcome:  "selected",
					},
				},
			}, nil
		}
	}

	// No "allow" option found, use first available
	logging.Warn("no 'allow' option found, using first available", "option_id", params.Options[0].OptionId)
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{
				OptionId: params.Options[0].OptionId,
				Outcome:  "selected",
			},
		},
	}, nil
}

// ReadTextFile handles file read requests
func (k *KiroClient) ReadTextFile(ctx context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	// For now, return an error indicating this capability is not implemented
	// In a full implementation, this would read the requested file
	return acp.ReadTextFileResponse{}, fmt.Errorf("ReadTextFile not implemented in planning mode")
}

// WriteTextFile handles file write requests
func (k *KiroClient) WriteTextFile(ctx context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	// For now, return an error indicating this capability is not implemented
	// In a full implementation, this would write to the requested file
	return acp.WriteTextFileResponse{}, fmt.Errorf("WriteTextFile not implemented in planning mode")
}

// SessionUpdate handles session update notifications
func (k *KiroClient) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	u := params.Update
	switch {
	case u.AgentMessageChunk != nil:
		content := u.AgentMessageChunk.Content
		if content.Text != nil {
			logging.Debug("agent message chunk received", "text_length", len(content.Text.Text))
			// Forward text content to the response channel if available
			k.mu.Lock()
			ch := k.respChan
			k.mu.Unlock()
			if ch != nil {
				ch <- &StreamingResponse{
					Type:      "text",
					Content:   content.Text.Text,
					Timestamp: time.Now(),
				}
			} else {
				logging.Warn("agent message chunk received but no response channel available")
			}
		}
	case u.ToolCall != nil:
		logging.Debug("tool call received")
	case u.ToolCallUpdate != nil:
		logging.Debug("tool call update received")
	case u.Plan != nil:
		logging.Debug("plan update received")
	case u.AgentThoughtChunk != nil:
		logging.Debug("agent thought chunk received")
	}
	return nil
}

// CreateTerminal handles terminal creation requests
func (k *KiroClient) CreateTerminal(ctx context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	// For planning mode, we don't support terminal operations
	return acp.CreateTerminalResponse{}, fmt.Errorf("terminal operations not supported in planning mode")
}

// KillTerminal handles terminal kill requests
func (k *KiroClient) KillTerminal(ctx context.Context, params acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, fmt.Errorf("terminal operations not supported in planning mode")
}

// TerminalOutput handles terminal output requests
func (k *KiroClient) TerminalOutput(ctx context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, fmt.Errorf("terminal operations not supported in planning mode")
}

// ReleaseTerminal handles terminal release requests
func (k *KiroClient) ReleaseTerminal(ctx context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, fmt.Errorf("terminal operations not supported in planning mode")
}

// WaitForTerminalExit handles terminal exit wait requests
func (k *KiroClient) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, fmt.Errorf("terminal operations not supported in planning mode")
}

// NewClient creates a new ACP client instance
func NewClient(config *ConnectionConfig) *KiroACPClient {
	if config == nil {
		config = DefaultConnectionConfig()
	}

	return &KiroACPClient{
		config:    config,
		connected: false,
		client:    &KiroClient{autoApprove: true},
	}
}

// Connect establishes a connection to the ACP server via Kiro CLI
func (c *KiroACPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate connection configuration including agent field
	if err := ValidateConnectionConfig(c.config); err != nil {
		logging.Error("invalid connection configuration", "error", err)
		return err
	}

	logging.Info("attempting ACP connection", "kiro_cli_path", c.config.KiroCLIPath, "agent", c.config.Agent)

	if c.connected {
		logging.Warn("already connected to ACP", "agent", c.config.Agent)
		return ErrAlreadyConnected
	}

	// Start kiro-cli in ACP mode with agent flag
	args := []string{"acp", "--agent", c.config.Agent}
	cmd := exec.CommandContext(ctx, c.config.KiroCLIPath, args...)

	// Get pipes for stdin/stdout communication
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logging.Error("failed to create stdin pipe", "agent", c.config.Agent, "error", err)
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		logging.Error("failed to create stdout pipe", "agent", c.config.Agent, "error", err)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	logging.Debug("starting kiro-cli process", "agent", c.config.Agent)

	// Start the command
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		logging.Error("failed to start kiro-cli", "agent", c.config.Agent, "error", err)
		return fmt.Errorf("failed to start kiro-cli: %w", err)
	}

	logging.Debug("creating ACP connection", "agent", c.config.Agent, "protocol_version", acp.ProtocolVersionNumber)

	// Create ACP connection
	conn := acp.NewClientSideConnection(c.client, stdin, stdout)

	// Initialize the connection
	_, err = conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapabilities{
				ReadTextFile:  false,
				WriteTextFile: false,
			},
		},
	})
	if err != nil {
		// Kill the process and wait - this also closes the pipes
		cmd.Process.Kill()
		cmd.Wait()
		logging.Error("failed to initialize ACP connection", "agent", c.config.Agent, "error", err)
		return fmt.Errorf("failed to initialize ACP connection: %w", err)
	}

	c.conn = conn
	c.cmd = cmd
	c.connected = true

	logging.Info("ACP connection established successfully", "agent", c.config.Agent)
	return nil
}

// Disconnect closes the connection to the ACP server
func (c *KiroACPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logging.Info("disconnecting ACP connection")

	if !c.connected {
		logging.Debug("already disconnected")
		return nil
	}

	if c.cmd != nil && c.cmd.Process != nil {
		logging.Debug("terminating ACP process", "pid", c.cmd.Process.Pid)

		// Attempt graceful shutdown with SIGTERM
		c.cmd.Process.Signal(syscall.SIGTERM)

		// Wait up to 3 seconds for graceful exit
		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil {
				logging.Warn("process exited with error", "error", err)
			} else {
				logging.Debug("process exited gracefully")
			}
		case <-time.After(3 * time.Second):
			// Force kill after timeout
			logging.Warn("graceful shutdown timeout, force killing process")
			c.cmd.Process.Kill()
			<-done
		}

		c.cmd = nil
	}

	c.conn = nil
	c.sessionID = ""
	c.connected = false

	logging.Info("ACP connection closed")
	return nil
}

// IsConnected returns true if connected to the ACP server
func (c *KiroACPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SendMessage sends a message to an agent and returns the response
func (c *KiroACPClient) SendMessage(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	logging.Info("sending ACP message", "agent", req.Agent, "message_length", len(req.Message), "streaming", req.Streaming)

	if err := ValidateMessageRequest(req); err != nil {
		logging.Error("invalid message request", "error", err)
		return nil, err
	}

	c.mu.RLock()
	if !c.connected || c.conn == nil {
		c.mu.RUnlock()
		logging.Error("not connected to ACP")
		return nil, ErrNotConnected
	}
	conn := c.conn
	sessionID := c.sessionID
	c.mu.RUnlock()

	// Apply request timeout if specified
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Create a new session or reuse existing one
	if sessionID == "" {
		c.mu.Lock()
		if c.sessionID == "" {
			logging.Debug("creating ACP session", "cwd", c.config.Cwd)
			sessionResp, err := conn.NewSession(ctx, acp.NewSessionRequest{
				Cwd:        c.config.Cwd,
				McpServers: []acp.McpServer{},
			})
			if err != nil {
				c.mu.Unlock()
				logging.Error("failed to create ACP session", "error", err)
				return nil, fmt.Errorf("failed to create session: %w", err)
			}
			c.sessionID = string(sessionResp.SessionId)
			logging.Info("ACP session created", "session_id", c.sessionID)
		}
		sessionID = c.sessionID
		c.mu.Unlock()
	} else {
		logging.Debug("reusing existing ACP session", "session_id", sessionID)
	}

	logging.Debug("sending prompt to ACP", "session_id", sessionID)

	// Send the prompt - responses will come via SessionUpdate
	_, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: acp.SessionId(sessionID),
		Prompt:    []acp.ContentBlock{acp.TextBlock(req.Message)},
	})
	if err != nil {
		logging.Error("failed to send prompt", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("failed to send prompt: %w", err)
	}

	logging.Info("ACP message sent successfully", "session_id", sessionID)

	// For now, return a basic response indicating the message was sent
	// In a full implementation, this would collect responses from SessionUpdate callbacks
	response := &MessageResponse{
		Success:   true,
		Message:   "Message sent successfully. Response will be received via session updates.",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"session_id": sessionID,
			"agent":      req.Agent,
		},
	}

	return response, nil
}

// StreamMessage sends a message to an agent and returns a streaming response
func (c *KiroACPClient) StreamMessage(ctx context.Context, req *MessageRequest) (<-chan *StreamingResponse, error) {
	logging.Info("starting ACP stream message", "agent", req.Agent, "message_length", len(req.Message))

	if err := ValidateMessageRequest(req); err != nil {
		logging.Error("invalid stream message request", "error", err)
		return nil, err
	}

	c.mu.RLock()
	if !c.connected || c.conn == nil {
		c.mu.RUnlock()
		logging.Error("not connected to ACP for streaming")
		return nil, ErrNotConnected
	}
	conn := c.conn
	sessionID := c.sessionID
	c.mu.RUnlock()

	// Create response channel
	respChan := make(chan *StreamingResponse, 10)

	go func() {
		defer func() {
			logging.Debug("closing stream response channel")
			close(respChan)
		}()

		// Apply request timeout if specified
		streamCtx := ctx
		if req.Timeout > 0 {
			var cancel context.CancelFunc
			streamCtx, cancel = context.WithTimeout(ctx, req.Timeout)
			defer cancel()
		}

		// Create a new session or reuse existing one
		if sessionID == "" {
			c.mu.Lock()
			if c.sessionID == "" {
				logging.Debug("creating streaming ACP session", "cwd", c.config.Cwd)
				sessionResp, err := conn.NewSession(streamCtx, acp.NewSessionRequest{
					Cwd:        c.config.Cwd,
					McpServers: []acp.McpServer{},
				})
				if err != nil {
					c.mu.Unlock()
					logging.Error("failed to create streaming session", "error", err)
					respChan <- &StreamingResponse{
						Type:      "error",
						Error:     fmt.Sprintf("failed to create session: %v", err),
						Timestamp: time.Now(),
					}
					return
				}
				c.sessionID = string(sessionResp.SessionId)
				logging.Info("streaming ACP session created", "session_id", c.sessionID)
			}
			sessionID = c.sessionID
			c.mu.Unlock()
		} else {
			logging.Debug("reusing existing session for streaming", "session_id", sessionID)
		}

		// Set the response channel on the client before sending the prompt
		if kiroClient, ok := c.client.(*KiroClient); ok {
			kiroClient.mu.Lock()
			kiroClient.respChan = respChan
			kiroClient.mu.Unlock()
			defer func() {
				kiroClient.mu.Lock()
				kiroClient.respChan = nil
				kiroClient.mu.Unlock()
			}()
		}

		// Send streaming response indicating start
		logging.Debug("sending stream start event", "session_id", sessionID)
		respChan <- &StreamingResponse{
			Type:      "start",
			Content:   "Starting streaming response...",
			Timestamp: time.Now(),
		}

		logging.Debug("sending prompt for streaming", "session_id", sessionID)

		// Send the prompt - real responses will come via SessionUpdate
		_, promptErr := conn.Prompt(streamCtx, acp.PromptRequest{
			SessionId: acp.SessionId(sessionID),
			Prompt:    []acp.ContentBlock{acp.TextBlock(req.Message)},
		})
		if promptErr != nil {
			logging.Error("streaming prompt failed", "session_id", sessionID, "error", promptErr)
			respChan <- &StreamingResponse{
				Type:      "error",
				Error:     fmt.Sprintf("failed to send prompt: %v", promptErr),
				Timestamp: time.Now(),
			}
			return
		}

		// The ACP SDK's notification barrier guarantees all SessionUpdate
		// callbacks (which forward text chunks to respChan) have completed
		// by the time Prompt() returns. Send the completion signal.
		logging.Info("streaming completed", "session_id", sessionID)
		respChan <- &StreamingResponse{
			Type:      "done",
			Content:   "",
			Timestamp: time.Now(),
		}
	}()

	return respChan, nil
}

// Close closes the client and cleans up resources
func (c *KiroACPClient) Close() error {
	return c.Disconnect()
}

// Ensure KiroACPClient implements Client interface
var _ Client = (*KiroACPClient)(nil)
var _ acp.Client = (*KiroClient)(nil)
