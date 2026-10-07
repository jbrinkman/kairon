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
		if len(turn.Commands) > 0 {
			if cmdResp, err := runStubCommands(ctx, req, turn.Commands); err != nil {
				cmdResp.Command = resp.Command
				return cmdResp, err
			}
		}
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

// runStubCommands runs each command with `sh -c` in req.WorkDir, in order, under
// one deadline of req.Timeout (DefaultTimeout when zero). It stops at the first
// failure. On failure the returned Response carries Stderr/ExitCode/Duration and
// no Text.
func runStubCommands(parent context.Context, req Request, commands []string) (Response, error) {
	if req.WorkDir == "" {
		return Response{}, errors.New("stub turn has commands but the request has no WorkDir; refusing to run them in the process working directory")
	}

	timeout := timeoutOrDefault(req.Timeout)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	start := time.Now()
	for _, command := range commands {
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = req.WorkDir
		cmd.WaitDelay = stubCommandWaitDelay
		setProcessGroup(cmd)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		runErr := cmd.Run()
		if runErr == nil {
			continue
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
	return Response{}, nil
}
