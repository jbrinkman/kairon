package session

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/jbrinkman/kairon/internal/logging"
)

// PlannerProcess manages a kiro-cli subprocess for planning sessions
type PlannerProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	output chan string
	ctx    context.Context
	cancel context.CancelFunc
}

// NewPlannerProcess starts a new kiro-cli subprocess for planning
func NewPlannerProcess() (*PlannerProcess, error) {
	logging.Info("starting new planner process")

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "kiro-cli", "chat")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		logging.Error("failed to create stdin pipe for planner", "error", err)
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		cancel()
		logging.Error("failed to create stdout pipe for planner", "error", err)
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		cancel()
		logging.Error("failed to create stderr pipe for planner", "error", err)
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Set process group for clean termination
	setSysProcAttr(cmd)

	process := &PlannerProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		output: make(chan string, 100),
		ctx:    ctx,
		cancel: cancel,
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		cancel()
		logging.Error("failed to start planner process", "error", err)
		return nil, fmt.Errorf("failed to start kiro-cli: %w", err)
	}

	logging.Info("planner process started", "pid", cmd.Process.Pid)

	go process.captureOutput()

	return process, nil
}

// SendMessage sends input to the planning subprocess with error recovery
func (p *PlannerProcess) SendMessage(message string) error {
	select {
	case <-p.ctx.Done():
		logging.Warn("attempted to send message to stopped planner process")
		return fmt.Errorf("process stopped")
	default:
	}

	// Validate process is still running
	if p.cmd.Process == nil {
		logging.Error("planner subprocess terminated unexpectedly")
		return fmt.Errorf("subprocess terminated unexpectedly")
	}

	logging.Debug("sending message to planner", "message_length", len(message))

	_, err := p.stdin.Write([]byte(message + "\n"))
	if err != nil {
		// Check if process died
		if !p.IsRunning() {
			logging.Error("planner subprocess died during message send", "error", err)
			return fmt.Errorf("subprocess died: %w", err)
		}
		logging.Error("failed to write to planner subprocess", "error", err)
		return fmt.Errorf("failed to write to subprocess: %w", err)
	}

	logging.Debug("message sent to planner successfully")
	return nil
}

// GetOutput returns a channel for reading subprocess output
func (p *PlannerProcess) GetOutput() <-chan string {
	return p.output
}

// Suspend suspends the subprocess by sending SIGSTOP
func (p *PlannerProcess) Suspend() error {
	if p.cmd.Process == nil {
		logging.Error("cannot suspend, planner process not running")
		return fmt.Errorf("process not running")
	}

	logging.Info("suspending planner process", "pid", p.cmd.Process.Pid)

	if err := suspendProcess(p.cmd.Process.Pid); err != nil {
		logging.Error("failed to suspend planner process", "pid", p.cmd.Process.Pid, "error", err)
		return err
	}

	logging.Info("planner process suspended", "pid", p.cmd.Process.Pid)
	return nil
}

// Resume resumes the suspended subprocess by sending SIGCONT
func (p *PlannerProcess) Resume() error {
	if p.cmd.Process == nil {
		logging.Error("cannot resume, planner process not running")
		return fmt.Errorf("process not running")
	}

	logging.Info("resuming planner process", "pid", p.cmd.Process.Pid)

	if err := resumeProcess(p.cmd.Process.Pid); err != nil {
		logging.Error("failed to resume planner process", "pid", p.cmd.Process.Pid, "error", err)
		return err
	}

	logging.Info("planner process resumed", "pid", p.cmd.Process.Pid)
	return nil
}

// Stop terminates the subprocess gracefully
func (p *PlannerProcess) Stop() error {
	logging.Info("stopping planner process")

	p.cancel()

	if p.cmd.Process != nil {
		pid := p.cmd.Process.Pid
		logging.Debug("terminating planner process", "pid", pid)

		// Send SIGTERM to the process group
		err := terminateProcess(pid)
		if err != nil {
			// Force kill if graceful termination fails
			logging.Warn("graceful termination failed, force killing planner", "pid", pid)
			_ = p.cmd.Process.Kill()
		}
	}

	// Close pipes
	_ = p.stdin.Close()
	_ = p.stdout.Close()
	_ = p.stderr.Close()

	// Wait for process to finish
	_ = p.cmd.Wait()

	logging.Info("planner process stopped")

	// p.output is closed by captureOutput once stdout/stderr readers have exited.
	return nil
}

// IsRunning returns true if the subprocess is still running
func (p *PlannerProcess) IsRunning() bool {
	if p.cmd.Process == nil {
		return false
	}

	select {
	case <-p.ctx.Done():
		return false
	default:
		return true
	}
}

// captureOutput reads from stdout/stderr and forwards to output channel
func (p *PlannerProcess) captureOutput() {
	var wg sync.WaitGroup
	wg.Add(2)

	forwardOutput := func(reader io.Reader, prefix string) {
		defer wg.Done()

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if prefix != "" {
				line = prefix + line
			}

			select {
			case p.output <- line:
			case <-p.ctx.Done():
				return
			}
		}
	}

	go forwardOutput(p.stdout, "")
	go forwardOutput(p.stderr, "[ERROR] ")

	wg.Wait()
	close(p.output)
}

// PlanningSession manages a complete planning session with subprocess and history
type PlanningSession struct {
	ID        string
	Process   *PlannerProcess
	State     *SessionState
	Manager   *SessionManager
	Suspended bool
}

// NewPlanningSession creates a new planning session
func NewPlanningSession(manager *SessionManager) (*PlanningSession, error) {
	logging.Info("creating new planning session")

	id, err := manager.Create(Planning)
	if err != nil {
		logging.Error("failed to create planning session", "error", err)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	state, err := manager.Load(id)
	if err != nil {
		logging.Error("failed to load planning session", "session_id", id, "error", err)
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	process, err := NewPlannerProcess()
	if err != nil {
		logging.Error("failed to start planner process", "session_id", id, "error", err)
		return nil, fmt.Errorf("failed to start planner process: %w", err)
	}

	session := &PlanningSession{
		ID:      id,
		Process: process,
		State:   state,
		Manager: manager,
	}

	logging.Info("planning session created", "session_id", id)

	// Start capturing conversation history
	go session.captureHistory()

	return session, nil
}

// SendMessage sends a message and captures it in history with error handling
func (ps *PlanningSession) SendMessage(content string) error {
	logging.Debug("sending message in planning session", "session_id", ps.ID, "content_length", len(content))

	// Validate session state
	if ps.State == nil {
		logging.Error("planning session state corrupted", "session_id", ps.ID)
		return fmt.Errorf("session state corrupted")
	}

	ps.State.AddMessage("user", content)
	if err := ps.Manager.Save(ps.ID, ps.State); err != nil {
		logging.Error("failed to save planning session state", "session_id", ps.ID, "error", err)
		return fmt.Errorf("failed to save session state: %w", err)
	}

	err := ps.Process.SendMessage(content)
	if err != nil {
		// Log the error but don't fail the session
		ps.State.AddMessage("system", fmt.Sprintf("Error sending message: %v", err))
		_ = ps.Manager.Save(ps.ID, ps.State)
		logging.Error("failed to send message in planning session", "session_id", ps.ID, "error", err)
		return err
	}

	logging.Info("message sent in planning session", "session_id", ps.ID)
	return nil
}

// Suspend suspends the planning session with error recovery
func (ps *PlanningSession) Suspend() error {
	logging.Info("suspending planning session", "session_id", ps.ID)

	if ps.State == nil {
		logging.Error("planning session state corrupted, cannot suspend", "session_id", ps.ID)
		return fmt.Errorf("session state corrupted, cannot suspend")
	}

	if err := ps.Manager.Save(ps.ID, ps.State); err != nil {
		logging.Error("failed to save planning session before suspend", "session_id", ps.ID, "error", err)
		return fmt.Errorf("failed to save session state before suspend: %w", err)
	}

	ps.Suspended = true
	if err := ps.Process.Suspend(); err != nil {
		ps.Suspended = false
		logging.Error("failed to suspend planning session process", "session_id", ps.ID, "error", err)
		return fmt.Errorf("failed to suspend process: %w", err)
	}

	logging.Info("planning session suspended", "session_id", ps.ID)
	return nil
}

// SuspendAndDetach suspends the planning session and detaches from process with error handling
func (ps *PlanningSession) SuspendAndDetach() error {
	logging.Info("suspending and detaching planning session", "session_id", ps.ID)

	if ps.State == nil {
		logging.Error("planning session state corrupted, cannot suspend", "session_id", ps.ID)
		return fmt.Errorf("session state corrupted, cannot suspend")
	}

	if err := ps.Manager.Save(ps.ID, ps.State); err != nil {
		logging.Error("failed to save planning session before detach", "session_id", ps.ID, "error", err)
		return fmt.Errorf("failed to save session state before suspend: %w", err)
	}

	ps.Suspended = true
	if err := ps.Process.Suspend(); err != nil {
		ps.Suspended = false
		logging.Error("failed to suspend planning session for detach", "session_id", ps.ID, "error", err)
		return fmt.Errorf("failed to suspend process: %w", err)
	}

	logging.Info("planning session suspended and detached", "session_id", ps.ID)
	return nil
}

// Resume resumes the planning session with validation
func (ps *PlanningSession) Resume() error {
	logging.Info("resuming planning session", "session_id", ps.ID)

	if !ps.Suspended {
		logging.Warn("attempted to resume non-suspended planning session", "session_id", ps.ID)
		return fmt.Errorf("session not suspended")
	}

	if err := ps.Process.Resume(); err != nil {
		logging.Error("failed to resume planning session process", "session_id", ps.ID, "error", err)
		return fmt.Errorf("failed to resume process: %w", err)
	}

	ps.Suspended = false
	logging.Info("planning session resumed", "session_id", ps.ID)
	return nil
}

// Stop terminates the planning session with graceful cleanup
func (ps *PlanningSession) Stop() error {
	logging.Info("stopping planning session", "session_id", ps.ID)

	if ps.State != nil {
		ps.State.AddMessage("system", "Session terminated")
		_ = ps.Manager.Save(ps.ID, ps.State)
	}

	if ps.Process != nil {
		if err := ps.Process.Stop(); err != nil {
			logging.Error("failed to stop planning session process", "session_id", ps.ID, "error", err)
			return fmt.Errorf("failed to stop process: %w", err)
		}
	}

	logging.Info("planning session stopped", "session_id", ps.ID)
	return nil
}

// Recover attempts to recover from a corrupted planning session
func (ps *PlanningSession) Recover() error {
	logging.Warn("recovering planning session", "session_id", ps.ID)

	// Try to reload session state
	state, err := ps.Manager.Load(ps.ID)
	if err != nil {
		logging.Warn("failed to reload session, creating new state", "session_id", ps.ID, "error", err)
		// Create new state if recovery failed
		state = NewSessionState(Planning)
		state.AddMessage("system", "Session recovered from corruption")
	}

	ps.State = state
	if err := ps.Manager.Save(ps.ID, ps.State); err != nil {
		logging.Error("failed to save recovered planning session", "session_id", ps.ID, "error", err)
		return err
	}

	logging.Info("planning session recovered", "session_id", ps.ID)
	return nil
}

// captureHistory captures subprocess output and adds to conversation history
func (ps *PlanningSession) captureHistory() {
	for output := range ps.Process.GetOutput() {
		ps.State.AddMessage("assistant", output)
		// Periodically save state
		_ = ps.Manager.SaveQuiet(ps.ID, ps.State)
	}
}

// SaveState saves the current session state
func (ps *PlanningSession) SaveState() error {
	if ps.State == nil {
		return fmt.Errorf("session state is nil")
	}
	if ps.Manager == nil {
		return fmt.Errorf("session manager is nil")
	}

	return ps.Manager.Save(ps.ID, ps.State)
}

// Cleanup performs comprehensive cleanup of session resources
func (ps *PlanningSession) Cleanup() error {
	var cleanupErrs []string

	// Save final state
	if ps.State != nil && ps.Manager != nil {
		if err := ps.SaveState(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Sprintf("failed to save state: %v", err))
		}
	}

	// Stop the process
	if ps.Process != nil {
		if err := ps.Process.Stop(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Sprintf("failed to stop process: %v", err))
		}
	}

	// Clear references
	ps.Process = nil
	ps.State = nil

	if len(cleanupErrs) > 0 {
		return fmt.Errorf("cleanup errors: %v", cleanupErrs)
	}

	return nil
}
