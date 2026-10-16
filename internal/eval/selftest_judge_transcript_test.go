package eval

import (
	"io"
	"strings"
	"testing"
)

// judgePrompts splits captured --debug output into the judge-check prompts it
// holds, one per "Debug: judge prompt (criterion=" header, in print order.
// Simulated-user routing prompts use a different header and are not included.
func judgePrompts(debug string) []string {
	const header = "Debug: judge prompt (criterion="
	parts := strings.Split(debug, header)
	if len(parts) < 2 {
		return nil
	}
	return parts[1:]
}

// runDebugSelfTestCase executes one stub self-test case with --debug on and
// returns its result and the judge prompts printed while scoring it.
func runDebugSelfTestCase(t *testing.T, name string) (CaseResult, []string) {
	t.Helper()
	rubric, tc := loadSelfTestCase(t, "selftest", name)
	cfg.debug = true
	buf := captureDebug(t)
	cr := executeCase(rubric, tc, nil, io.Discard, false)
	t.Cleanup(func() { cleanupWorkspace(t, cr) })
	return cr, judgePrompts(buf.String())
}

// requireInOrder fails unless every want appears in s, each after the previous.
func requireInOrder(t *testing.T, label, s string, wants ...string) {
	t.Helper()
	pos := 0
	for _, w := range wants {
		i := strings.Index(s[pos:], w)
		if i < 0 {
			t.Errorf("%s: %q not found (in order) after offset %d:\n%s", label, w, pos, s)
			return
		}
		pos += i + len(w)
	}
}

func TestSelfTestMultiTurnJudgePrompts(t *testing.T) {
	cr, prompts := runDebugSelfTestCase(t, "multi-turn-judge")
	if cr.ErrorContext != nil {
		t.Fatalf("unexpected ErrorContext %+v", cr.ErrorContext)
	}
	if len(prompts) != 2 {
		t.Fatalf("got %d judge prompts, want 2 (one per judge check)", len(prompts))
	}

	// Check 1 has turn: 1 — it sees turn 1 only.
	for _, want := range []string{"USER-ONE-MARKER", "REPLY-ONE-MARKER", "===BEGIN TURN 1 USER MESSAGE===", "===END TURN 1 AGENT REPLY==="} {
		if !strings.Contains(prompts[0], want) {
			t.Errorf("prompt #1 lacks %q:\n%s", want, prompts[0])
		}
	}
	for _, leak := range []string{"USER-TWO-MARKER", "REPLY-TWO-MARKER", "TURN 2"} {
		if strings.Contains(prompts[0], leak) {
			t.Errorf("prompt #1 (turn: 1) leaks %q:\n%s", leak, prompts[0])
		}
	}

	// Check 2 has no turn — the whole conversation, in order, each fenced.
	requireInOrder(t, "prompt #2", prompts[1],
		"===BEGIN TURN 1 USER MESSAGE===", "USER-ONE-MARKER", "===END TURN 1 USER MESSAGE===",
		"===BEGIN TURN 1 AGENT REPLY===", "REPLY-ONE-MARKER", "===END TURN 1 AGENT REPLY===",
		"===BEGIN TURN 2 USER MESSAGE===", "USER-TWO-MARKER", "===END TURN 2 USER MESSAGE===",
		"===BEGIN TURN 2 AGENT REPLY===", "REPLY-TWO-MARKER", "===END TURN 2 AGENT REPLY===")

	sc := checkedScore(t, cr)
	jc := judgeChecks(sc)
	if len(jc) != 2 {
		t.Fatalf("%d judge checks, want 2: %+v", len(jc), sc.Checks)
	}
	for i, r := range jc {
		if !r.Passed || !strings.Contains(r.Detail, "judge answered yes: stub judge: yes") {
			t.Errorf("judge check %d = %+v, want a pass with `judge answered yes: stub judge: yes`", i+1, r)
		}
	}
	if sc.Score != sc.MaxScore || sc.MaxScore != 2 {
		t.Errorf("score = %d/%d, want 2/2", sc.Score, sc.MaxScore)
	}
}

func TestSelfTestSimJudgePrompt(t *testing.T) {
	cr, prompts := runDebugSelfTestCase(t, "sim-judge")
	if cr.ErrorContext != nil {
		t.Fatalf("unexpected ErrorContext %+v", cr.ErrorContext)
	}
	if len(prompts) != 1 {
		t.Fatalf("got %d judge prompts, want 1", len(prompts))
	}
	if len(cr.UserTurns) != 3 || len(cr.TurnOutputs) != 3 {
		t.Fatalf("user turns %q, turn outputs %d; want 3 of each", cr.UserTurns, len(cr.TurnOutputs))
	}

	// The messages actually sent (opener, the verbatim fact answer, the default
	// reply) and every stub reply, interleaved and labelled by turn.
	requireInOrder(t, "sim-judge prompt", prompts[0],
		"===BEGIN TURN 1 USER MESSAGE===", selftestSimOpener, "===END TURN 1 USER MESSAGE===",
		"===BEGIN TURN 1 AGENT REPLY===", "SIM-TURN-ONE-MARKER", "===END TURN 1 AGENT REPLY===",
		"===BEGIN TURN 2 USER MESSAGE===", selftestSimF1, "===END TURN 2 USER MESSAGE===",
		"===BEGIN TURN 2 AGENT REPLY===", "SIM-TURN-TWO-MARKER", "===END TURN 2 AGENT REPLY===",
		"===BEGIN TURN 3 USER MESSAGE===", selftestSimReply, "===END TURN 3 USER MESSAGE===",
		"===BEGIN TURN 3 AGENT REPLY===", "SIM-TURN-THREE-MARKER", "===END TURN 3 AGENT REPLY===")

	jc := judgeChecks(checkedScore(t, cr))
	if len(jc) != 1 || !jc[0].Passed || !strings.Contains(jc[0].Detail, "judge answered yes: stub judge: yes") {
		t.Errorf("judge checks = %+v, want one pass with the stub judge's reasoning", jc)
	}
}

// A judge check whose turn is beyond the case's turns never loads, so a case
// file cannot exercise the run-time guard; the loader names the turn count.
func TestLoadCasesJudgeTurnBeyondCaseTurns(t *testing.T) {
	_, err := loadTurnsCase(t, `name: judge-turn-9
agent: a1
turns:
  - one
  - two
checks:
  - {criterion: c, type: judge, question: q, turn: 9}
`)
	if err == nil {
		t.Fatal("expected a load error for a judge check with turn 9 on a 2-turn case")
	}
	for _, want := range []string{"judge-turn-9", "check 1 (judge)", "turn 9 is beyond the case's 2 turn(s)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
