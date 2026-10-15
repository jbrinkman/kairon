package eval

import (
	"reflect"
	"strings"
	"testing"
)

// The checked-in answers and replies of the simulated-user self-test cases.
const (
	simRoutesCase     = "sim-routes"
	simStopWhenCase   = "sim-stop-when"
	simFactNotAsked   = "sim-fact-not-asked"
	selftestSimF1     = "SIM-F1-ANSWER: comma separated, UTF-8, one header row."
	selftestSimReply  = "SIM-DEFAULT-REPLY: I don't know, use your best judgement."
	selftestSimOpener = "I want a feature that exports reports as CSV"
)

// kiroCLICalls installs a recording fake kiro-cli first on PATH and returns a
// function reporting the calls it has seen.
func kiroCLICalls(t *testing.T) func() []string {
	t.Helper()
	_, callsLog := installFakeKiroCLI(t, "echo should-not-run")
	return func() []string { return readCalls(t, callsLog) }
}

// assertCaseFullMarks requires a simulated self-test case to run clean and
// score the maximum on every check.
func assertCaseFullMarks(t *testing.T, c CaseResult) {
	t.Helper()
	if c.ErrorContext != nil {
		t.Errorf("%s: unexpected ErrorContext %+v", c.CaseName, c.ErrorContext)
	}
	sc := checkedScore(t, c)
	if len(sc.Checks) == 0 || sc.Score != sc.MaxScore || sc.MaxScore != len(sc.Checks) {
		t.Errorf("%s: score = %d/%d (checks %d), want N/N: %s", c.CaseName, sc.Score, sc.MaxScore, len(sc.Checks), sc.Reasoning)
	}
	for _, cr := range sc.Checks {
		if !cr.Passed {
			t.Errorf("%s: check %s failed: %s", c.CaseName, cr.Label, cr.Detail)
		}
	}
}

// assertSimulatedSelfTestResults holds the evidence the passing simulated
// self-test cases must show; it is shared by the native and sandbox runs.
// AC 2 (verbatim answer, then default_reply), AC 3 (stop_when, then the
// scripted turns) and AC 4 (a sim log with every decision).
func assertSimulatedSelfTestResults(t *testing.T, res AgentResult) {
	t.Helper()

	routes := caseByName(t, res, simRoutesCase)
	assertCaseFullMarks(t, routes)
	wantUser := []string{selftestSimOpener, selftestSimF1, selftestSimReply}
	if !reflect.DeepEqual(routes.UserTurns, wantUser) {
		t.Errorf("%s user_turns = %q, want %q", simRoutesCase, routes.UserTurns, wantUser)
	}
	if len(routes.TurnOutputs) != 3 {
		t.Errorf("%s turn_outputs has %d entries, want 3", simRoutesCase, len(routes.TurnOutputs))
	}
	var routeCalls int
	for _, call := range routes.Calls {
		if call.Role == "judge" && call.Purpose == "simulated_user" {
			routeCalls++
		}
	}
	if routeCalls != 2 {
		t.Errorf("%s has %d simulated_user routing records, want 2", simRoutesCase, routeCalls)
	}
	if routes.JudgeCost.TokensIn == 0 {
		t.Errorf("%s routing and judge calls cost nothing: %+v", simRoutesCase, routes.JudgeCost)
	}

	stop := caseByName(t, res, simStopWhenCase)
	assertCaseFullMarks(t, stop)
	wantUser = []string{selftestSimOpener, selftestSimF1, "Please summarise.", "Thanks."}
	if !reflect.DeepEqual(stop.UserTurns, wantUser) {
		t.Errorf("%s user_turns = %q, want %q", simStopWhenCase, stop.UserTurns, wantUser)
	}
	if len(stop.TurnOutputs) != 4 {
		t.Errorf("%s turn_outputs has %d entries, want 4 (2 simulated + 2 scripted)", simStopWhenCase, len(stop.TurnOutputs))
	}
	var agentTurns []int
	for _, call := range stop.Calls {
		if call.Role == "agent" {
			agentTurns = append(agentTurns, call.Turn)
		}
	}
	if !reflect.DeepEqual(agentTurns, []int{1, 2, 3, 4}) {
		t.Errorf("%s agent call turns = %v, want [1 2 3 4]", simStopWhenCase, agentTurns)
	}
}

// readCaseSimLog returns the decisions of the kept workspace of c.
func readCaseSimLog(t *testing.T, c CaseResult) []SimDecision {
	t.Helper()
	if c.WorkspaceDir == "" {
		t.Fatalf("%s: no workspace_dir recorded (workspaces not kept?)", c.CaseName)
	}
	return readLogLines(t, c.WorkspaceDir)
}

func TestSelfTestSimulatedPassingCases(t *testing.T) {
	calls := kiroCLICalls(t)
	res := runSelfTestAgent(t, "selftest")
	assertSimulatedSelfTestResults(t, res)

	// AC 4: the kept workspace holds the sim log with both decisions.
	decisions := readCaseSimLog(t, caseByName(t, res, simRoutesCase))
	if len(decisions) != 2 {
		t.Fatalf("%s sim log has %d decisions, want 2: %+v", simRoutesCase, len(decisions), decisions)
	}
	if d := decisions[0]; d.Turn != 1 || d.Fact != "f1" || d.Discoverable == nil || *d.Discoverable ||
		!strings.Contains(d.Question, "SIM-TURN-ONE-MARKER") {
		t.Errorf("decision 1 = %+v, want turn 1 fact f1 (discoverable false) with the turn-1 question", d)
	}
	if d := decisions[1]; d.Turn != 2 || d.Fact != "none" || d.Discoverable != nil ||
		!strings.Contains(d.Question, "SIM-TURN-TWO-MARKER") {
		t.Errorf("decision 2 = %+v, want turn 2 fact none with the turn-2 question", d)
	}

	stop := readCaseSimLog(t, caseByName(t, res, simStopWhenCase))
	if len(stop) != 1 || stop[0].Turn != 1 || stop[0].Fact != "f1" {
		t.Errorf("%s sim log = %+v, want exactly turn 1 -> f1", simStopWhenCase, stop)
	}

	// AC 6: the stub run makes no model call.
	if got := calls(); len(got) != 0 {
		t.Errorf("kiro-cli was called during the stub self-test: %v", got)
	}
}

func TestSelfTestSimulatedFailingCase(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")
	c := caseByName(t, res, simFactNotAsked)
	if len(c.UserTurns) != 3 || c.UserTurns[1] != selftestSimF1 {
		t.Errorf("%s user_turns = %q, want the f1 answer as the second message", simFactNotAsked, c.UserTurns)
	}
	sc := checkedScore(t, c)
	if sc.Score >= sc.MaxScore || sc.MaxScore == 0 {
		t.Fatalf("%s score = %d/%d, want a failed case: %s", simFactNotAsked, sc.Score, sc.MaxScore, sc.Reasoning)
	}
	if !strings.Contains(sc.Reasoning, "fact_not_asked fact=f1") {
		t.Errorf("reasoning %q must name 'fact_not_asked fact=f1'", sc.Reasoning)
	}
	var failed int
	for _, cr := range sc.Checks {
		if cr.Passed {
			continue
		}
		failed++
		if cr.Type != CheckFactNotAsked || !strings.Contains(cr.Label, "fact=f1") {
			t.Errorf("failed check %q (%s), want the fact_not_asked fact=f1 check", cr.Label, cr.Type)
		}
	}
	if failed != 1 {
		t.Errorf("%d checks failed, want exactly 1: %+v", failed, sc.Checks)
	}
}
