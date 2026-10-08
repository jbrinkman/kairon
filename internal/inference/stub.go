package inference

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// stubJudgeOutput is what every judge call returns: the maximum score of the
// judge scale (1-5) and pass=true, wrapped in the delimiters the eval harness
// parses.
const stubJudgeOutput = "===JSON_START===\n" +
	`{"score": 5, "reasoning": "stub judge: always passes", "pass": true}` + "\n" +
	"===JSON_END==="

// stubBackend is deterministic and never spawns a process or touches the network.
type stubBackend struct{}

func newStub() *stubBackend { return &stubBackend{} }

func (*stubBackend) Name() string { return NameStub }

func (*stubBackend) Available() error { return nil }

func (*stubBackend) StartupProbe() time.Duration { return 0 }

func (*stubBackend) Invoke(ctx context.Context, req Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	switch req.Role {
	case RoleJudge:
		return Response{
			Text:    stubJudgeOutput,
			Model:   NameStub,
			Usage:   EstimateUsage(req.Prompt, stubJudgeOutput),
			Command: "stub judge",
		}, nil

	case RoleAgent:
		if req.Stub == nil || req.Turn < 0 || req.Turn >= len(req.Stub.Turns) ||
			req.Stub.Turns[req.Turn].Response == "" {
			return Response{Command: "stub agent"}, fmt.Errorf("case has no stub.turns[%d].response", req.Turn)
		}
		turn := req.Stub.Turns[req.Turn]
		resp := Response{
			Text:    turn.Response,
			Model:   turn.Model,
			Command: "stub agent",
		}
		// Commands are ungated scripted environment actions and run first.
		// Tool calls run after them, in order, each gated by the trust set: a
		// denied call is recorded and skipped, an allowed call runs. Denials
		// are recorded only for calls execution actually reaches — if an
		// earlier command or allowed call fails and halts the turn, calls after
		// it (and their denials) are never recorded, so the failed-call record
		// is not padded with denials that never occurred.
		denials, cmdResp, runErr := runStubAgentTurn(ctx, req, turn)
		if runErr != nil {
			cmdResp.Command = resp.Command
			cmdResp.ToolDenials = denials
			return cmdResp, runErr
		}
		resp.ToolDenials = denials
		if turn.Usage != nil {
			resp.Usage = Usage{
				InputTokens:  turn.Usage.InputTokens,
				OutputTokens: turn.Usage.OutputTokens,
				Source:       UsageReported,
			}
		} else {
			resp.Usage = EstimateUsage(req.Prompt, turn.Response)
		}
		return resp, nil

	default:
		return Response{}, fmt.Errorf("stub backend: unsupported role %q", req.Role)
	}
}

// stubCommandWaitDelay bounds how long Wait lingers for output pipes to close
// after a command was killed.
const stubCommandWaitDelay = time.Second

// runStubAgentTurn executes one stub agent turn: the ungated environment
// Commands first, then each ToolCall in order, gated by the trust set. An
// allowed call's command runs; a denied call is recorded and skipped. All run
// under one shared deadline (req.Timeout, DefaultTimeout when zero) and stop at
// the first failure. Denials are recorded only for tool calls execution
// actually reaches, so a turn that fails partway is not reported as having
// denied calls it never got to. On success it returns the full denial list, a
// zero Response and nil error; on failure it returns the denials reached so
// far, a Response carrying Stderr/ExitCode/Duration, and the error.
func runStubAgentTurn(parent context.Context, req Request, turn StubTurn) ([]ToolDenial, Response, error) {
	// WorkDir is only required when a command actually runs; a turn whose tool
	// calls are all denied (and that has no environment commands) records its
	// denials without ever touching the filesystem.
	needsRun := len(turn.Commands) > 0
	for _, call := range turn.ToolCalls {
		if req.ToolTrust.Allows(NormalizeToolName(call.Tool)) {
			needsRun = true
			break
		}
	}
	if needsRun && req.WorkDir == "" {
		return nil, Response{}, errors.New("stub turn has commands but the request has no WorkDir; refusing to run them in the process working directory")
	}
	if len(turn.Commands) == 0 && len(turn.ToolCalls) == 0 {
		return nil, Response{}, nil
	}

	timeout := timeoutOrDefault(req.Timeout)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	start := time.Now()

	// Environment commands run before any tool call. If one fails, no tool
	// call is reached, so no denials are recorded.
	for _, command := range turn.Commands {
		if resp, err := runOneStubCommand(ctx, parent, req, command, timeout, start); err != nil {
			return nil, resp, err
		}
	}

	var denials []ToolDenial
	for _, call := range turn.ToolCalls {
		tool := NormalizeToolName(call.Tool)
		if !req.ToolTrust.Allows(tool) {
			denials = append(denials, ToolDenial{
				Tool:    tool,
				Command: call.Command,
				Reason:  ReasonToolNotTrusted,
			})
			continue
		}
		if resp, err := runOneStubCommand(ctx, parent, req, call.Command, timeout, start); err != nil {
			// Reached this allowed call and it failed; denials recorded so far
			// are exactly those for calls before this point.
			return denials, resp, err
		}
	}
	return denials, Response{}, nil
}

// runOneStubCommand runs a single command under the already-deadlined ctx. start
// anchors the reported Duration across a sequence of calls sharing one ctx, so
// callers that interleave commands (environment actions then gated tool calls)
// still report one cumulative duration and one shared deadline. On success it
// returns a zero Response and nil error.
func runOneStubCommand(ctx, parent context.Context, req Request, command string, timeout time.Duration, start time.Time) (Response, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = req.WorkDir
	cmd.WaitDelay = stubCommandWaitDelay
	setProcessGroup(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr == nil {
		return Response{}, nil
	}

	resp := Response{Stderr: stderr.String(), Duration: time.Since(start)}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		resp.ExitCode = exitErr.ExitCode()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return resp, &invokeError{
			msg:  fmt.Sprintf("stub timeout after %v", timeout),
			errs: []error{ErrTimeout, runErr},
		}
	}
	if parent.Err() != nil {
		return resp, parent.Err()
	}
	return resp, fmt.Errorf("stub command %q failed: %w: %s", command, runErr, strings.TrimSpace(resp.Stderr))
}
