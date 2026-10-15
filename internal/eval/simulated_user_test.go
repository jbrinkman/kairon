package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/jbrinkman/kairon/internal/inference"
)

const (
	simAnswerF1 = "  Comma separated, UTF-8.  " // surrounding spaces: replies are verbatim
	simDefault  = "Use your best judgement."
)

// simFacts returns the two-fact bank used by most tests.
func simFacts() []Fact {
	return []Fact{
		{ID: "f1", Answer: simAnswerF1, Discoverable: boolPtr(false)},
		{ID: "f2", Answer: "Written in Go.", Discoverable: boolPtr(true)},
	}
}

// simCase builds a hand-made simulated-user stub case. responses are the stub
// agent turns; routes the stub routing decisions.
func simCase(name string, maxTurns int, routes []string, responses ...string) TestCase {
	su := &SimulatedUser{Facts: simFacts(), DefaultReply: simDefault}
	if maxTurns > 0 {
		su.MaxTurns = &maxTurns
	}
	stub := &inference.StubScript{Routes: routes}
	for _, r := range responses {
		stub.Turns = append(stub.Turns, inference.StubTurn{Response: r})
	}
	return TestCase{Name: name, Agent: "selftest", Input: "OPENING", SimulatedUser: su, Stub: stub}
}

// routeBackend wraps a backend and records agent and routing requests. When
// routeText or routeErr is set, routing requests are answered with them
// instead of being passed on.
type routeBackend struct {
	inference.Backend
	mu        sync.Mutex
	agent     []inference.Request
	routes    []inference.Request
	routeText string
	routeErr  error
}

func (r *routeBackend) Invoke(ctx context.Context, req inference.Request) (inference.Response, error) {
	r.mu.Lock()
	switch {
	case req.Role == inference.RoleAgent:
		r.agent = append(r.agent, req)
	case req.Route:
		r.routes = append(r.routes, req)
		if r.routeErr != nil {
			r.mu.Unlock()
			return inference.Response{Usage: inference.Usage{InputTokens: 5, OutputTokens: 5}}, r.routeErr
		}
		if r.routeText != "" {
			r.mu.Unlock()
			return inference.Response{Text: r.routeText, Usage: inference.Usage{InputTokens: 5, OutputTokens: 5}}, nil
		}
	}
	r.mu.Unlock()
	return r.Backend.Invoke(ctx, req)
}

func useRouteBackend(t *testing.T) *routeBackend {
	t.Helper()
	rb := &routeBackend{Backend: cfg.backend}
	cfg.backend = rb
	return rb
}

// simChecks are checks that keep the criterion off the model judge.
func simChecks(cs ...Check) []Check {
	for i := range cs {
		cs[i].Criterion = "clarity"
	}
	return cs
}

func readLogLines(t *testing.T, dir string) []SimDecision {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".eval", "sim-log.jsonl"))
	if err != nil {
		t.Fatalf("read sim log: %v", err)
	}
	var out []SimDecision
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var d SimDecision
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("bad sim log line %q: %v", line, err)
		}
		out = append(out, d)
	}
	return out
}

// AC 2: replies come from the bank verbatim, then default_reply.
func TestSimulatedUserRepliesFromBank(t *testing.T) {
	execEnv(t, true)
	rb := useRouteBackend(t)
	tc := simCase("replies", 3, []string{"f1", "none"}, "A1 ask format?", "A2 ask more?", "A3 done")
	tc.Setup = []SetupEntry{{Type: "text", Label: "Ctx", Content: "SETUP-CONTEXT"}}
	tc.Checks = simChecks(
		Check{Type: "fact_asked", Fact: "f1"},
		Check{Type: "fact_not_asked", Fact: "f2"},
		Check{Type: "output_contains", Pattern: "A2", Turn: intp(2)},
	)

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, true)

	if len(rb.agent) != 3 {
		t.Fatalf("agent turns = %d, want 3\n%s", len(rb.agent), out.String())
	}
	if rb.agent[1].Prompt != simAnswerF1 {
		t.Errorf("turn 2 prompt = %q, want f1's answer verbatim %q", rb.agent[1].Prompt, simAnswerF1)
	}
	if rb.agent[2].Prompt != simDefault {
		t.Errorf("turn 3 prompt = %q, want default_reply %q", rb.agent[2].Prompt, simDefault)
	}
	for i, req := range rb.agent {
		if req.Turn != i {
			t.Errorf("agent request %d: Turn = %d, want %d", i, req.Turn, i)
		}
		if has := strings.Contains(req.Prompt, "SETUP-CONTEXT"); has != (i == 0) {
			t.Errorf("agent request %d: setup present = %v, want %v", i, has, i == 0)
		}
	}
	if want := []string{"OPENING", simAnswerF1, simDefault}; strings.Join(cr.UserTurns, "|") != strings.Join(want, "|") {
		t.Errorf("UserTurns = %q, want %q", cr.UserTurns, want)
	}
	if got := strings.Join(cr.TurnOutputs, "|"); got != "A1 ask format?|A2 ask more?|A3 done" {
		t.Errorf("TurnOutputs = %q", got)
	}
	if cr.ActualOutput != "A3 done" {
		t.Errorf("ActualOutput = %q", cr.ActualOutput)
	}

	// Two routing calls, interleaved with the agent calls in execution order.
	var roles []string
	for _, c := range cr.Calls {
		r := c.Role
		if c.Purpose != "" {
			r += ":" + c.Purpose
		}
		roles = append(roles, r)
	}
	want := "agent,judge:simulated_user,agent,judge:simulated_user,agent"
	if strings.Join(roles, ",") != want {
		t.Errorf("call order = %v, want %s", roles, want)
	}
	for i, req := range rb.routes {
		if req.RouteSeq != i || !req.Route || req.Role != inference.RoleJudge {
			t.Errorf("route request %d = %+v", i, req)
		}
		if req.Timeout != judgeCheckTimeout {
			t.Errorf("route request %d timeout = %v, want %v", i, req.Timeout, judgeCheckTimeout)
		}
		if req.Model != cfg.pins.judgeModel() {
			t.Errorf("route request %d model = %q, want the judge pin %q", i, req.Model, cfg.pins.judgeModel())
		}
	}
	if cr.JudgeCost.TokensIn == 0 {
		t.Errorf("JudgeCost = %+v, want the routing cost on success", cr.JudgeCost)
	}
	for _, c := range cr.Calls {
		if c.Role == "agent" && c.Purpose != "" {
			t.Errorf("agent call has purpose %q", c.Purpose)
		}
	}

	// AC 5 end to end: the fact checks score from the log.
	if len(cr.Scores) != 1 || cr.Scores[0].Score != 3 || cr.Scores[0].MaxScore != 3 {
		t.Fatalf("scores = %+v", cr.Scores)
	}

	log := readLogLines(t, cr.WorkspaceDir)
	if len(log) != 2 || log[0].Turn != 1 || log[0].Fact != "f1" || log[1].Turn != 2 || log[1].Fact != "none" {
		t.Fatalf("sim log = %+v", log)
	}
	if log[0].Discoverable == nil || *log[0].Discoverable || log[1].Discoverable != nil {
		t.Errorf("discoverable = %v / %v, want false / absent", log[0].Discoverable, log[1].Discoverable)
	}
	if log[0].Question != "A1 ask format?" {
		t.Errorf("question = %q", log[0].Question)
	}
}

func TestSimulatedUserProgressShowsTurnWithoutTotal(t *testing.T) {
	execEnv(t, false)
	tc := simCase("progress", 2, []string{"none"}, "a", "b")
	tc.Checks = simChecks(Check{Type: "output_contains", Pattern: "b"})
	var out bytes.Buffer
	executeCase(execRubric(), tc, nil, &out, false)
	for _, want := range []string{"turn 1...", "turn 2..."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("progress lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "turn 1/") {
		t.Errorf("simulated progress must not print a total:\n%s", out.String())
	}
}

// AC 3: stop_when on turn 2 ends the phase; scripted turns become turns 3, 4.
func TestSimulatedUserStopWhenThenScriptedTurns(t *testing.T) {
	execEnv(t, true)
	rb := useRouteBackend(t)
	tc := simCase("stop", 5, []string{"f1"}, "A1 question?", "A2 PLAN-READY", "A3", "A4")
	tc.Turns = []string{"Please summarise.", "Thanks."}
	tc.SimulatedUser.StopWhen = "PLAN-READY"
	tc.Checks = simChecks(Check{Type: "output_contains", Pattern: "A4", Turn: intp(4)})

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, true)

	if want := []string{"OPENING", simAnswerF1, "Please summarise.", "Thanks."}; strings.Join(cr.UserTurns, "|") != strings.Join(want, "|") {
		t.Fatalf("UserTurns = %q, want %q\n%s", cr.UserTurns, want, out.String())
	}
	if len(cr.TurnOutputs) != 4 || len(rb.agent) != 4 {
		t.Fatalf("turn outputs = %d agent calls = %d, want 4", len(cr.TurnOutputs), len(rb.agent))
	}
	if len(rb.routes) != 1 {
		t.Errorf("routing calls = %d, want 1 (the stop turn is not routed)", len(rb.routes))
	}
	for i, req := range rb.agent {
		if req.Turn != i {
			t.Errorf("agent %d: Turn = %d", i, req.Turn)
		}
	}
	var turns []int
	for _, c := range cr.Calls {
		if c.Role == "agent" {
			turns = append(turns, c.Turn)
		}
	}
	if len(turns) != 4 || turns[0] != 1 || turns[3] != 4 {
		t.Errorf("agent call turns = %v, want 1..4", turns)
	}
	if log := readLogLines(t, cr.WorkspaceDir); len(log) != 1 || log[0].Turn != 1 {
		t.Errorf("sim log = %+v, want one decision on turn 1", log)
	}
	if len(cr.Scores) != 1 || cr.Scores[0].Score != 1 {
		t.Errorf("scores = %+v", cr.Scores)
	}
}

func TestSimulatedUserMaxTurnsEndsWithoutAnsweringLastTurn(t *testing.T) {
	execEnv(t, false)
	rb := useRouteBackend(t)
	tc := simCase("max", 2, []string{"none"}, "A1?", "A2?", "A3-never")
	tc.Turns = []string{"scripted"}
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)

	// 2 simulated turns, then one scripted turn; turn 2's question gets no routed reply.
	if want := []string{"OPENING", simDefault, "scripted"}; strings.Join(cr.UserTurns, "|") != strings.Join(want, "|") {
		t.Fatalf("UserTurns = %q, want %q\n%s", cr.UserTurns, want, out.String())
	}
	if len(rb.agent) != 3 || len(rb.routes) != 1 {
		t.Errorf("agent = %d routes = %d, want 3 and 1", len(rb.agent), len(rb.routes))
	}
}

func TestSimulatedUserMaxTurnsDefaultsToTen(t *testing.T) {
	execEnv(t, false)
	rb := useRouteBackend(t)
	var responses []string
	for i := 0; i < 10; i++ {
		responses = append(responses, "turn output")
	}
	routes := make([]string, 9)
	for i := range routes {
		routes[i] = "none"
	}
	tc := simCase("default-max", 0, routes, responses...)
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if len(rb.agent) != 10 || len(rb.routes) != 9 {
		t.Fatalf("agent = %d routes = %d, want 10 and 9\n%s", len(rb.agent), len(rb.routes), out.String())
	}
	if len(cr.UserTurns) != 10 {
		t.Errorf("UserTurns = %d, want 10", len(cr.UserTurns))
	}
}

func TestSimulatedUserStopOnFirstTurnMakesNoRoutingCall(t *testing.T) {
	execEnv(t, true)
	rb := useRouteBackend(t)
	tc := simCase("stop-1", 5, nil, "all done READY")
	tc.SimulatedUser.StopWhen = "READY"
	tc.Checks = simChecks(Check{Type: "fact_not_asked", Fact: "f1"})
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, true)

	if len(rb.routes) != 0 {
		t.Fatalf("routing calls = %d, want 0", len(rb.routes))
	}
	if len(cr.UserTurns) != 1 || len(cr.TurnOutputs) != 1 {
		t.Errorf("UserTurns = %q TurnOutputs = %q", cr.UserTurns, cr.TurnOutputs)
	}
	// The log exists (created empty at case start) so fact_not_asked passes.
	if len(cr.Scores) != 1 || cr.Scores[0].Score != 1 {
		t.Errorf("scores = %+v, want fact_not_asked to pass on an empty log\n%s", cr.Scores, out.String())
	}
	if got := readLogLines(t, cr.WorkspaceDir); len(got) != 0 {
		t.Errorf("sim log = %+v, want empty", got)
	}
}

func TestSimulatedUserStopWhenIsCaseSensitive(t *testing.T) {
	execEnv(t, false)
	rb := useRouteBackend(t)
	tc := simCase("case", 2, []string{"none"}, "ready", "b")
	tc.SimulatedUser.StopWhen = "READY"
	var out bytes.Buffer
	executeCase(execRubric(), tc, nil, &out, false)
	if len(rb.agent) != 2 {
		t.Errorf("agent turns = %d, want 2 (stop_when does not match \"ready\")", len(rb.agent))
	}
}

func TestSimulatedUserLogExcerpt(t *testing.T) {
	execEnv(t, true)
	long := strings.Repeat("x", 300) + "\n\n  Which   format\tdo you want?  \n"
	tc := simCase("excerpt", 2, []string{"f1"}, long, "end")
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, true)
	log := readLogLines(t, cr.WorkspaceDir)
	if len(log) != 1 {
		t.Fatalf("log = %+v\n%s", log, out.String())
	}
	q := log[0].Question
	if utf8.RuneCountInString(q) != 200 {
		t.Errorf("question has %d runes, want 200", utf8.RuneCountInString(q))
	}
	if !strings.HasSuffix(q, " Which format do you want?") || strings.ContainsAny(q, "\n\t") || strings.Contains(q, "  ") {
		t.Errorf("question = %q, want collapsed whitespace and the tail of the output", q)
	}
}

// AC 4: the agent can write .eval/, so the log is rewritten from memory.
func TestSimulatedUserLogIsTamperResistant(t *testing.T) {
	execEnv(t, true)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}
	tc := simCase("tamper", 3, []string{"f1", "none"})
	tc.Stub.Turns = []inference.StubTurn{
		{Response: "A1?", Commands: []string{": > .eval/sim-log.jsonl"}},
		{Response: "A2?", Commands: []string{"rm -f .eval/sim-log.jsonl; ln -s '" + outside + "' .eval/sim-log.jsonl"}},
		{Response: "A3", Commands: []string{"rm -f .eval/sim-log.jsonl; echo garbage > .eval/sim-log.jsonl"}},
	}
	tc.Checks = simChecks(Check{Type: "fact_asked", Fact: "f1"}, Check{Type: "fact_not_asked", Fact: "f2"})

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, true)

	if len(cr.Scores) != 1 || cr.Scores[0].Score != 2 || cr.Scores[0].MaxScore != 2 {
		t.Fatalf("scores = %+v\n%s", cr.Scores, out.String())
	}
	if log := readLogLines(t, cr.WorkspaceDir); len(log) != 2 || log[0].Fact != "f1" || log[1].Fact != "none" {
		t.Errorf("sim log = %+v, want the two decisions from memory", log)
	}
	if got, _ := os.ReadFile(outside); string(got) != "untouched" {
		t.Errorf("the harness wrote through a symlink planted by the agent: %q", got)
	}
}

func TestSimulatedUserRoutingFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		tc       func() TestCase
		setup    func(*routeBackend)
		wantTurn string
		wantAgnt int
		wantErr  string
	}{
		{
			name:     "exhausted stub.routes",
			tc:       func() TestCase { return simCase("exhausted", 4, []string{"f1"}, "a?", "b?", "c?", "d") },
			wantTurn: "turn 2: simulated user:",
			wantAgnt: 2,
			wantErr:  "stub.routes[1]",
		},
		{
			name: "unknown fact id",
			tc:   func() TestCase { return simCase("unknown", 3, []string{"f1", "f1"}, "a?", "b?", "c") },
			setup: func(rb *routeBackend) {
				rb.routeText = "===JSON_START===\n{\"reasoning\":\"x\",\"fact\":\"zzz\"}\n===JSON_END==="
			},
			wantTurn: "turn 1: simulated user:",
			wantAgnt: 1,
			wantErr:  "zzz",
		},
		{
			name:     "judge error",
			tc:       func() TestCase { return simCase("judge-error", 3, []string{"f1", "f1"}, "a?", "b?", "c") },
			setup:    func(rb *routeBackend) { rb.routeErr = errors.New("judge exploded") },
			wantTurn: "turn 1: simulated user:",
			wantAgnt: 1,
			wantErr:  "judge exploded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execEnv(t, false)
			rb := useRouteBackend(t)
			if tt.setup != nil {
				tt.setup(rb)
			}
			tc := tt.tc()
			tc.Checks = simChecks(Check{Type: "output_contains", Pattern: "a"})
			var out bytes.Buffer
			cr := executeCase(execRubric(), tc, nil, &out, false)

			if cr.ActualOutput != "" {
				t.Errorf("ActualOutput = %q, want empty on a routing failure", cr.ActualOutput)
			}
			if cr.ErrorContext == nil || !strings.Contains(cr.ErrorContext.Stderr, tt.wantTurn) || !strings.Contains(cr.ErrorContext.Stderr, tt.wantErr) {
				t.Errorf("ErrorContext = %+v, want stderr with %q and %q", cr.ErrorContext, tt.wantTurn, tt.wantErr)
			}
			if !strings.Contains(out.String(), tt.wantTurn) {
				t.Errorf("progress output lacks %q:\n%s", tt.wantTurn, out.String())
			}
			if len(rb.agent) != tt.wantAgnt {
				t.Errorf("agent turns = %d, want %d", len(rb.agent), tt.wantAgnt)
			}
			for _, req := range rb.agent {
				if req.Prompt == simDefault {
					t.Errorf("default_reply was sent after a routing failure")
				}
			}
			for _, u := range cr.UserTurns {
				if u == simDefault {
					t.Errorf("UserTurns contains default_reply: %q", cr.UserTurns)
				}
			}
			if len(cr.TurnOutputs) != tt.wantAgnt {
				t.Errorf("TurnOutputs = %q, want %d recorded", cr.TurnOutputs, tt.wantAgnt)
			}
			// The failed routing call is recorded, but its cost is not in JudgeCost.
			var routed int
			for _, c := range cr.Calls {
				if c.Purpose == "simulated_user" && c.Error != "" {
					routed++
				}
			}
			if tt.name != "unknown fact id" && routed != 1 {
				t.Errorf("failed routing call records = %d, want 1; calls = %+v", routed, cr.Calls)
			}
			// The exhausted case first routed once successfully (its cost counts);
			// in the others the only routing call failed, so JudgeCost stays zero.
			if zero := tt.name != "exhausted stub.routes"; zero == (cr.JudgeCost.TokensIn != 0 || cr.JudgeCost.TokensOut != 0) {
				t.Errorf("JudgeCost = %+v, want zero = %v (only successful routing calls are costed)", cr.JudgeCost, zero)
			}
		})
	}
}

// AC 6: the stub run spawns no kiro-cli.
func TestSimulatedUserMakesNoModelCalls(t *testing.T) {
	_, callsLog := installFakeKiroCLI(t, "echo should-not-run")
	execEnv(t, false)
	tc := simCase("no-model", 3, []string{"f1", "none"}, "A1?", "A2?", "A3")
	tc.Checks = simChecks(Check{Type: "fact_asked", Fact: "f1"})
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)
	if len(cr.UserTurns) != 3 {
		t.Fatalf("case did not run: %q\n%s", cr.UserTurns, out.String())
	}
	if calls := readCalls(t, callsLog); len(calls) != 0 {
		t.Errorf("kiro-cli was called: %v", calls)
	}
}

// A simulated case keeps one container session for all its turns.
func TestSimulatedUserUsesOneContainerSession(t *testing.T) {
	sessionEnv(t)
	f := &fakeSession{}
	opens := useFakeSession(t, f)
	tc := simCase("container", 3, []string{"f1", "none"})
	tc.Timeout = "45s"
	tc.Stub.Turns = nil
	tc.Checks = simChecks(Check{Type: "fact_asked", Fact: "f1"})

	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, testContainerConfig(), &out, false)

	if len(*opens) != 1 {
		t.Fatalf("opens = %d, want 1\n%s", len(*opens), out.String())
	}
	if len(f.reqs) != 3 {
		t.Fatalf("Run calls = %d, want 3\n%s", len(f.reqs), out.String())
	}
	for i, req := range f.reqs {
		if req.Turn != i {
			t.Errorf("Run %d: Turn = %d, want %d", i, req.Turn, i)
		}
	}
	if f.reqs[1].Prompt != simAnswerF1 || f.reqs[2].Prompt != simDefault {
		t.Errorf("prompts = %q, %q", f.reqs[1].Prompt, f.reqs[2].Prompt)
	}
	if len(f.closed) != 1 || f.closed[0] {
		t.Errorf("Close calls = %v, want exactly one with failed=false", f.closed)
	}
	if len(cr.Scores) != 1 || cr.Scores[0].Score != 1 {
		t.Errorf("scores = %+v", cr.Scores)
	}
}

func TestSimulatedUserSessionClosedBeforeLogRewrite(t *testing.T) {
	sessionEnv(t)
	f := &fakeSession{}
	useFakeSession(t, f)
	tc := simCase("container-order", 2, []string{"f1"})
	tc.Timeout = "45s"
	tc.Stub.Turns = nil
	tc.Checks = simChecks(Check{Type: "fact_asked", Fact: "f1"})

	orig := scoreCaseFn
	t.Cleanup(func() { scoreCaseFn = orig })
	closedAtScoring, logAtScoring := -1, []SimDecision(nil)
	scoreCaseFn = func(r Rubric, tc TestCase, cr *CaseResult) {
		closedAtScoring = len(f.closed)
		logAtScoring, _ = readSimLog(cr.WorkspaceDir)
		orig(r, tc, cr)
	}
	var out bytes.Buffer
	executeCase(execRubric(), tc, testContainerConfig(), &out, false)
	if closedAtScoring != 1 {
		t.Errorf("session closes seen at scoring = %d, want 1", closedAtScoring)
	}
	if len(logAtScoring) != 1 || logAtScoring[0].Fact != "f1" {
		t.Errorf("sim log at scoring = %+v", logAtScoring)
	}
}

func TestOneTurnClassicCaseStillUsesOneShotContainerPath(t *testing.T) {
	sessionEnv(t)
	opened := 0
	origOpen, origInvoke := openContainerSession, invokeInContainer
	openContainerSession = func(inference.Request, *ContainerConfig, *caseWorkspace) (containerSession, error) {
		opened++
		return &fakeSession{}, nil
	}
	oneShot := 0
	invokeInContainer = func(inference.Request, *ContainerConfig, *caseWorkspace) (inference.Response, *ErrorContext, error) {
		oneShot++
		return inference.Response{Text: "done"}, nil, nil
	}
	t.Cleanup(func() { openContainerSession, invokeInContainer = origOpen, origInvoke })

	tc := stubCase("classic", "", "45s", "done")
	tc.Checks = simChecks(Check{Type: "output_contains", Pattern: "done"})
	var out bytes.Buffer
	executeCase(execRubric(), tc, testContainerConfig(), &out, false)
	if opened != 0 || oneShot != 1 {
		t.Errorf("opened = %d one-shot = %d, want 0 and 1", opened, oneShot)
	}
}

// Classic and #317 turns cases keep their result shape.
func TestSimulatedUserFieldsAbsentFromOtherCases(t *testing.T) {
	execEnv(t, false)
	for _, tc := range []TestCase{
		stubCase("classic", "", "", "done"),
		turnsCase("turns", []string{"one", "two"}, inference.StubTurn{Response: "a"}, inference.StubTurn{Response: "b"}),
	} {
		tc.Checks = simChecks(Check{Type: "output_contains", Pattern: "."})
		var out bytes.Buffer
		cr := executeCase(execRubric(), tc, nil, &out, false)
		data, err := json.Marshal(cr)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"user_turns", "purpose"} {
			if strings.Contains(string(data), banned) {
				t.Errorf("%s: result JSON contains %q: %s", tc.Name, banned, data)
			}
		}
		if cr.UserTurns != nil {
			t.Errorf("%s: UserTurns = %q, want nil", tc.Name, cr.UserTurns)
		}
	}
}

// The judge sees the conversation as it happened, not just the opening message.
func TestSimulatedUserJudgeViewListsRealReplies(t *testing.T) {
	execEnv(t, false)
	tc := simCase("judge-view", 2, []string{"f1"}, "A1?", "A2")
	var seen TestCase
	orig := scoreCaseFn
	t.Cleanup(func() { scoreCaseFn = orig })
	scoreCaseFn = func(r Rubric, tc TestCase, cr *CaseResult) {
		seen = tc
		orig(r, tc, cr)
	}
	var out bytes.Buffer
	cr := executeCase(execRubric(), tc, nil, &out, false)

	if strings.Join(seen.Turns, "|") != strings.Join(cr.UserTurns, "|") {
		t.Fatalf("scored case Turns = %q, want UserTurns %q", seen.Turns, cr.UserTurns)
	}
	input := judgeCaseInput(seen)
	for _, want := range []string{"Turn 1 (user): OPENING", "Turn 2 (user): " + simAnswerF1} {
		if !strings.Contains(input, want) {
			t.Errorf("judge input lacks %q:\n%s", want, input)
		}
	}
	if tc.Turns != nil {
		t.Errorf("the caller's case was mutated: Turns = %q", tc.Turns)
	}
}

func TestParseRoute(t *testing.T) {
	ids := []string{"f1", "f2"}
	wrap := func(body string) string { return "preamble\n===JSON_START===\n" + body + "\n===JSON_END===\ntrailer" }
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"valid id", wrap(`{"reasoning":"r","fact":"f2"}`), "f2", false},
		{"id with spaces", wrap(`{"reasoning":"r","fact":" f1 "}`), "f1", false},
		{"none", wrap(`{"reasoning":"r","fact":"none"}`), "none", false},
		{"None", wrap(`{"reasoning":"r","fact":"None"}`), "none", false},
		{"ansi stripped", "===JSON_START===\n\x1b[0m" + `{"fact":"f1"}` + "\n===JSON_END===", "f1", false},
		{"last start wins", `===JSON_START==={"fact":"f1"}===JSON_END===` + "\n" + wrap(`{"fact":"f2"}`), "f2", false},
		{"unknown id", wrap(`{"reasoning":"r","fact":"f3"}`), "", true},
		{"id compared exactly", wrap(`{"fact":"F1"}`), "", true},
		{"missing key", wrap(`{"reasoning":"r"}`), "", true},
		{"non-string", wrap(`{"fact":["f1"]}`), "", true},
		{"null", wrap(`{"fact":null}`), "", true},
		{"bad json", wrap(`{"fact":`), "", true},
		{"no delimiters", `{"fact":"f1"}`, "", true},
		{"no end delimiter", "===JSON_START===\n" + `{"fact":"f1"}`, "", true},
		{"blank", wrap(`{"fact":"  "}`), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRoute(tt.raw, ids)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildRoutePrompt(t *testing.T) {
	p := buildRoutePrompt(simFacts(), "Which format do you need?")
	for _, want := range []string{"f1", strings.TrimSpace(simAnswerF1), "f2", "Written in Go.", "none", "untrusted",
		"===BEGIN AGENT OUTPUT===", "===END AGENT OUTPUT===", "Which format do you need?", "===JSON_START==="} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
	if strings.Index(p, "untrusted") > strings.Index(p, "===BEGIN AGENT OUTPUT===") {
		t.Errorf("the untrusted-data warning must come before the fenced output")
	}

	// Questions come last: only the tail of a long output is kept, on a rune boundary.
	long := "HEAD-MARKER" + strings.Repeat("é", 20000) + "TAIL-MARKER"
	p = buildRoutePrompt(simFacts(), long)
	if strings.Contains(p, "HEAD-MARKER") || !strings.Contains(p, "TAIL-MARKER") {
		t.Errorf("prompt must keep the tail of a long output")
	}
	if !utf8.ValidString(p) {
		t.Errorf("prompt is not valid UTF-8 after truncation")
	}
	fenced := p[strings.Index(p, "===BEGIN AGENT OUTPUT===")+len("===BEGIN AGENT OUTPUT===") : strings.Index(p, "===END AGENT OUTPUT===")]
	if len(fenced) > routeOutputCap+16 {
		t.Errorf("fenced output is %d bytes, want <= ~%d", len(fenced), routeOutputCap)
	}
}
