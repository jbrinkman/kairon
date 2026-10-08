package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// KiroCLIAgentCommand returns the kiro-cli arguments (without the program name)
// and the human-readable command line for an agent request:
//
//	chat --agent <agent> --no-interactive --trust-all-tools [--model <m>]
//
// When req.ToolTrust is non-nil, --trust-all-tools is replaced by the single
// argument --trust-tools=<csv> (--trust-tools= when the set is empty). A nil
// ToolTrust keeps the historical --trust-all-tools form byte-identical.
//
// --model is appended only when req.Model is non-empty. The prompt is never
// part of the arguments; callers deliver it on stdin.
//
// It is shared by the native kiro-cli backend and by any transport (such as
// the eval container sandbox) that executes kiro-cli elsewhere, so there is a
// single definition of the command.
func KiroCLIAgentCommand(req Request) (args []string, command string) {
	trustArg := "--trust-all-tools"
	if req.ToolTrust != nil {
		trustArg = "--trust-tools=" + req.ToolTrust.CSV()
	}
	args = []string{"chat", "--agent", req.Agent, "--no-interactive", trustArg}
	command = fmt.Sprintf("kiro-cli chat --agent %s --no-interactive %s", req.Agent, trustArg)
	if req.Model != "" {
		args = append(args, "--model", req.Model)
		command += " --model " + req.Model
	}
	return args, command
}

// KiroCLIAgentResponse builds the Response for a successful kiro-cli agent
// run: ANSI sequences are stripped from stdout, Model echoes the requested
// model (kiro-cli cannot report the one that served the call) and Usage is
// estimated from the prompt and output text.
func KiroCLIAgentResponse(req Request, stdout, stderr string, exitCode int, d time.Duration) Response {
	_, command := KiroCLIAgentCommand(req)
	text := stripANSI(stdout)
	return Response{
		Text:     text,
		Model:    req.Model,
		Usage:    EstimateUsage(req.Prompt, text),
		Command:  command,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: d,
	}
}

// ExecEnvelope is the wire format written by ServeExec: the Response plus the
// backend error, flattened so it survives a process boundary.
type ExecEnvelope struct {
	Response Response `json:"response"`
	Error    string   `json:"error,omitempty"`
	// Timeout is true when the backend error satisfied errors.Is(err, ErrTimeout).
	Timeout bool `json:"timeout,omitempty"`
}

// ServeExec reads one JSON Request from r, runs it on the backend registered
// under name, and writes one JSON ExecEnvelope to w.
//
// A backend error is reported in the envelope (and ServeExec returns nil);
// only I/O, decode and unknown-backend errors are returned.
func ServeExec(ctx context.Context, name string, r io.Reader, w io.Writer) error {
	backend, err := New(name)
	if err != nil {
		return err
	}

	var req Request
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return fmt.Errorf("decoding inference request: %w", err)
	}

	resp, invokeErr := backend.Invoke(ctx, req)
	env := ExecEnvelope{Response: resp}
	if invokeErr != nil {
		env.Error = invokeErr.Error()
		env.Timeout = errors.Is(invokeErr, ErrTimeout)
	}

	if err := json.NewEncoder(w).Encode(env); err != nil {
		return fmt.Errorf("writing inference result: %w", err)
	}
	return nil
}

// DecodeExecResult turns the stdout of ServeExec back into (Response, error).
// A reported timeout yields an error satisfying errors.Is(err, ErrTimeout)
// whose message is the original backend message.
func DecodeExecResult(stdout []byte) (Response, error) {
	var env ExecEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &env); err != nil {
		return Response{}, fmt.Errorf("decoding inference result: %w", err)
	}

	switch {
	case env.Timeout:
		msg := env.Error
		if strings.TrimSpace(msg) == "" {
			msg = ErrTimeout.Error()
		}
		return env.Response, &invokeError{msg: msg, errs: []error{ErrTimeout}}
	case env.Error != "":
		return env.Response, errors.New(env.Error)
	}
	return env.Response, nil
}
