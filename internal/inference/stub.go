package inference

import (
	"context"
	"fmt"
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
