package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

// recordingBackend wraps another backend and records every agent request.
type recordingBackend struct {
	inference.Backend
	mu   sync.Mutex
	reqs []inference.Request
}

func (r *recordingBackend) Invoke(ctx context.Context, req inference.Request) (inference.Response, error) {
	if req.Role == inference.RoleAgent {
		r.mu.Lock()
		r.reqs = append(r.reqs, req)
		r.mu.Unlock()
	}
	return r.Backend.Invoke(ctx, req)
}

// useRecordingBackend swaps the configured backend for a recording wrapper
// around it. execEnv/configure resets cfg for the next test.
func useRecordingBackend(t *testing.T) *recordingBackend {
	t.Helper()
	rb := &recordingBackend{Backend: cfg.backend}
	cfg.backend = rb
	return rb
}

// turnsCase builds a hand-made multi-turn stub case.
func turnsCase(name string, turns []string, stubTurns ...inference.StubTurn) TestCase {
	return TestCase{
		Name:  name,
		Agent: "selftest",
		Turns: turns,
		Stub:  &inference.StubScript{Turns: stubTurns},
	}
}

func TestMultiTurnProducesTurnOutputsInOrder(t *testing.T) {
	execEnv(t, false)
	tc := turnsCase("three", []string{"one", "two", "three"},
		inference.StubTurn{Response: "R1", Usage: &inference.StubUsage{InputTokens: 10, OutputTokens: 1}},
		inference.StubTurn{Response: "R2", Usage: &inference.StubUsage{InputTokens: 20, OutputTokens: 2}},
		inference.StubTurn{Response: "R3", Usage: &inference.StubUsage{InputTokens: 30, OutputTokens: 3}},
	)
	// A check keeps the criterion off the judge, so calls holds agent calls only.
	tc.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "R3"}}

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)

	if want := []string{"R1", "R2", "R3"}; strings.Join(cr.TurnOutputs, "|") != strings.Join(want, "|") {
		t.Fatalf("TurnOutputs = %q, want %q\n%s", cr.TurnOutputs, want, out.String())
	}
	if cr.ActualOutput != "R3" {
		t.Errorf("ActualOutput = %q, want R3", cr.ActualOutput)
	}
	if len(cr.Calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(cr.Calls))
	}
	for i, c := range cr.Calls {
		if c.Turn != i+1 {
			t.Errorf("Calls[%d].Turn = %d, want %d", i, c.Turn, i+1)
		}
	}
	if cr.AgentCost.TokensIn != 60 || cr.AgentCost.TokensOut != 6 {
		t.Errorf("AgentCost = %+v, want summed 60/6", cr.AgentCost)
	}
	for _, want := range []string{"turn 1/3", "turn 2/3", "turn 3/3"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("progress output lacks %q:\n%s", want, out.String())
		}
	}

	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	if got, _ := generic["turn_outputs"].([]any); len(got) != 3 {
		t.Errorf("JSON turn_outputs = %v, want 3 entries", generic["turn_outputs"])
	}
}

func TestMultiTurnClassicCaseHasNoTurnFields(t *testing.T) {
	execEnv(t, false)
	var out bytes.Buffer
	cr := executeCase(execRubric(), stubCase("classic", "", "", "done"), nil, &out, false)

	if cr.ActualOutput != "done" {
		t.Fatalf("ActualOutput = %q\n%s", cr.ActualOutput, out.String())
	}
	if cr.TurnOutputs != nil {
		t.Errorf("TurnOutputs = %q, want nil for a classic case", cr.TurnOutputs)
	}
	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "turn_outputs") || strings.Contains(string(data), `"turn"`) {
		t.Errorf("classic result JSON has turn fields: %s", data)
	}
	if strings.Contains(out.String(), "turn 1/1") {
		t.Errorf("classic case prints turn progress:\n%s", out.String())
	}
	for _, c := range cr.Calls {
		if c.Turn != 0 {
			t.Errorf("call %+v has Turn set for a classic case", c)
		}
	}
}

func TestMultiTurnBackendSeesTurnIndexAndOnlyThatTurnsMessage(t *testing.T) {
	execEnv(t, false)
	rb := useRecordingBackend(t)

	tc := turnsCase("prompts", []string{"MSG-ONE", "MSG-TWO", "MSG-THREE"},
		inference.StubTurn{Response: "a"}, inference.StubTurn{Response: "b"}, inference.StubTurn{Response: "c"})
	tc.Setup = []SetupEntry{{Type: "text", Label: "Ctx", Content: "SETUP-CONTEXT"}}

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if cr.ErrorContext != nil || len(rb.reqs) != 3 {
		t.Fatalf("reqs = %d ec=%+v\n%s", len(rb.reqs), cr.ErrorContext, out.String())
	}
	msgs := []string{"MSG-ONE", "MSG-TWO", "MSG-THREE"}
	for i, req := range rb.reqs {
		if req.Turn != i {
			t.Errorf("request %d: Turn = %d, want %d", i, req.Turn, i)
		}
		for j, m := range msgs {
			if has := strings.Contains(req.Prompt, m); has != (i == j) {
				t.Errorf("request %d: prompt contains %q = %v; prompt=%q", i, m, has, req.Prompt)
			}
		}
		hasSetup := strings.Contains(req.Prompt, "SETUP-CONTEXT")
		if hasSetup != (i == 0) {
			t.Errorf("request %d: setup present = %v, want %v", i, hasSetup, i == 0)
		}
	}
	if rb.reqs[1].Prompt != "MSG-TWO" || rb.reqs[2].Prompt != "MSG-THREE" {
		t.Errorf("later turns must be sent verbatim: %q, %q", rb.reqs[1].Prompt, rb.reqs[2].Prompt)
	}
}

func TestMultiTurnClassicCaseSendsTurnZero(t *testing.T) {
	execEnv(t, false)
	rb := useRecordingBackend(t)
	var out bytes.Buffer
	executeCase(execRubric(), stubCase("classic", "", "", "done"), nil, &out, false)
	if len(rb.reqs) != 1 || rb.reqs[0].Turn != 0 || rb.reqs[0].Prompt != "do it" {
		t.Fatalf("reqs = %+v", rb.reqs)
	}
}

func TestMultiTurnGHLogChecksAreScopedToTurn(t *testing.T) {
	execEnv(t, false)
	tc := turnsCase("gates", []string{"draft", "approve", "create"},
		inference.StubTurn{Response: "draft"},
		inference.StubTurn{Response: "confirm"},
		inference.StubTurn{Response: "created", Commands: []string{`echo "gh issue create --title t" >> .eval/gh.log`}},
	)
	tc.Checks = []Check{
		{Criterion: "clarity", Type: "gh_log_not_contains", Pattern: "issue create", Turn: intp(1)},
		{Criterion: "clarity", Type: "gh_log_not_contains", Pattern: "issue create", Turn: intp(2)},
		{Criterion: "clarity", Type: "gh_log_contains", Pattern: "issue create", Turn: intp(3)},
		{Criterion: "clarity", Type: "gh_log_contains", Pattern: "issue create"},
		{Criterion: "clarity", Type: "output_contains", Pattern: "draft", Turn: intp(1)},
		{Criterion: "clarity", Type: "output_not_contains", Pattern: "draft"},
	}

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if len(cr.Scores) != 1 {
		t.Fatalf("scores = %+v\n%s", cr.Scores, out.String())
	}
	s := cr.Scores[0]
	if s.Score != s.MaxScore || s.MaxScore != 6 {
		t.Fatalf("score = %d/%d, reasoning = %s", s.Score, s.MaxScore, s.Reasoning)
	}

	// The same check pinned to the wrong turn fails: scoping is real.
	tc.Checks = []Check{{Criterion: "clarity", Type: "gh_log_contains", Pattern: "issue create", Turn: intp(1)}}
	cr = executeCase(execRubric(), tc, nil, &out, false)
	if s := cr.Scores[0]; s.Score != 0 || s.MaxScore != 1 {
		t.Fatalf("turn-1 gh_log_contains should fail: %d/%d %s", s.Score, s.MaxScore, s.Reasoning)
	}
}

func TestMultiTurnSharesWorkspaceAcrossTurns(t *testing.T) {
	execEnv(t, false)
	tc := turnsCase("shared", []string{"one", "two"},
		inference.StubTurn{Response: "r1", Commands: []string{"echo first > turn1.txt"}},
		inference.StubTurn{Response: "r2", Commands: []string{"cat turn1.txt && echo second > turn2.txt"}},
	)
	tc.Checks = []Check{{Criterion: "clarity", Type: "file_exists", Path: "turn2.txt"}}

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if cr.ErrorContext != nil || cr.ActualOutput != "r2" {
		t.Fatalf("output=%q ec=%+v\n%s", cr.ActualOutput, cr.ErrorContext, out.String())
	}
	if s := cr.Scores[0]; s.Score != 1 {
		t.Fatalf("score = %d/%d %s", s.Score, s.MaxScore, s.Reasoning)
	}
}

func TestMultiTurnFailureStopsRemainingTurns(t *testing.T) {
	execEnv(t, false)
	rb := useRecordingBackend(t)
	tc := turnsCase("fails", []string{"one", "two", "three"},
		inference.StubTurn{Response: "r1"},
		inference.StubTurn{Response: "r2", Commands: []string{"exit 3"}},
		inference.StubTurn{Response: "r3"},
	)
	tc.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "r1", Turn: intp(1)}}

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)

	if len(rb.reqs) != 2 {
		t.Fatalf("backend saw %d requests, want 2 (turn 3 must not run)", len(rb.reqs))
	}
	if cr.ActualOutput != "" {
		t.Errorf("ActualOutput = %q, want empty", cr.ActualOutput)
	}
	if len(cr.TurnOutputs) != 1 || cr.TurnOutputs[0] != "r1" {
		t.Errorf("TurnOutputs = %q, want [r1]", cr.TurnOutputs)
	}
	if len(cr.Calls) != 2 || cr.Calls[0].Error != "" || cr.Calls[1].Error == "" {
		t.Fatalf("calls = %+v, want 2 with the second failed", cr.Calls)
	}
	if cr.Calls[0].Turn != 1 || cr.Calls[1].Turn != 2 {
		t.Errorf("call turns = %d,%d", cr.Calls[0].Turn, cr.Calls[1].Turn)
	}
	if cr.ErrorContext == nil {
		t.Error("ErrorContext should be the failing turn's")
	}
	if !strings.Contains(out.String(), "Error: turn 2/3:") {
		t.Errorf("printed error should start with 'turn 2/3:':\n%s", out.String())
	}
	if len(cr.Scores) != 1 || cr.Scores[0].Score != 0 || !strings.Contains(cr.Scores[0].Reasoning, noOutputChecksReasoning) {
		t.Errorf("scores = %+v, want checks not run", cr.Scores)
	}
}

// Wiring of the turn contract through the real kiro-cli backend: turn 1 has no
// --resume, later turns do, and stdin is only that turn's message.
func TestMultiTurnKiroCLIResumeAndStdin(t *testing.T) {
	execEnv(t, false)
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil { // default kiro-cli backend
		t.Fatal(err)
	}
	body := `d=$(dirname "$0")
n=$(wc -l < "$d/calls.log" | tr -d ' ')
cat > "$d/stdin.$n"
printf 'out-%s' "$n"`
	dir, callsLog := installFakeKiroCLI(t, body)

	tc := TestCase{Name: "resume", Agent: "selftest", Turns: []string{"FIRST", "SECOND"}}
	tc.Setup = []SetupEntry{{Type: "text", Content: "SETUP-CONTEXT"}}
	tc.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "out-2"}} // no judge call
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if cr.ErrorContext != nil && cr.ActualOutput == "" {
		t.Fatalf("ec=%+v\n%s", cr.ErrorContext, out.String())
	}
	if want := "out-1|out-2"; strings.Join(cr.TurnOutputs, "|") != want {
		t.Fatalf("TurnOutputs = %q, want %q\n%s", cr.TurnOutputs, want, out.String())
	}

	calls := readCalls(t, callsLog)
	if len(calls) != 2 {
		t.Fatalf("kiro-cli calls = %q", calls)
	}
	if strings.Contains(calls[0], "--resume") {
		t.Errorf("first call must not resume: %s", calls[0])
	}
	if !strings.Contains(calls[1], "--resume") {
		t.Errorf("second call must resume: %s", calls[1])
	}
	in1 := readFileT(t, filepath.Join(dir, "stdin.1"))
	in2 := readFileT(t, filepath.Join(dir, "stdin.2"))
	if !strings.Contains(in1, "FIRST") || !strings.Contains(in1, "SETUP-CONTEXT") || strings.Contains(in1, "SECOND") {
		t.Errorf("stdin of call 1 = %q", in1)
	}
	if strings.TrimSpace(in2) != "SECOND" {
		t.Errorf("stdin of call 2 = %q, want only SECOND", in2)
	}
	_ = os.Remove(callsLog)
}
