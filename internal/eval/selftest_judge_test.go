package eval

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

// judgeCalls returns the judge CallRecords a case recorded for the criterion
// that carries its judge checks. The selftest rubric's clarity criterion has
// no checks, so it still makes a legacy judge call that is not counted here.
func judgeCalls(c CaseResult) []inference.CallRecord {
	var out []inference.CallRecord
	for _, rec := range c.Calls {
		if rec.Role == string(inference.RoleJudge) && rec.Criterion == "structural_completeness" {
			out = append(out, rec)
		}
	}
	return out
}

// judgeChecks returns the judge-type results of a criterion score.
func judgeChecks(sc CriterionScore) []CheckResult {
	var out []CheckResult
	for _, cr := range sc.Checks {
		if cr.Type == CheckJudge {
			out = append(out, cr)
		}
	}
	return out
}

func TestSelfTestJudgePassingCases(t *testing.T) {
	res := runSelfTestAgent(t, "selftest")

	for _, name := range []string{"check-judge-yes", "check-judge-files", "check-judge-mixed"} {
		c := caseByName(t, res, name)
		if c.ErrorContext != nil {
			t.Errorf("%s: unexpected ErrorContext %+v", name, c.ErrorContext)
		}
		sc := checkedScore(t, c)
		if sc.Score != sc.MaxScore || sc.MaxScore == 0 || !sc.Deterministic || sc.Skipped {
			t.Errorf("%s: score = %d/%d (deterministic %v, skipped %v), want N/N", name, sc.Score, sc.MaxScore, sc.Deterministic, sc.Skipped)
		}
		jc := judgeChecks(sc)
		if len(jc) != 1 {
			t.Fatalf("%s: %d judge checks, want 1: %+v", name, len(jc), sc.Checks)
		}
		if !jc[0].Passed || !strings.Contains(jc[0].Detail, "judge answered yes: stub judge: yes") {
			t.Errorf("%s: judge check = %+v, want a pass with the judge's reasoning", name, jc[0])
		}
		if calls := judgeCalls(c); len(calls) != 1 || calls[0].Criterion != "structural_completeness" || calls[0].Error != "" {
			t.Errorf("%s: judge calls = %+v, want one clean call for structural_completeness", name, calls)
		}
	}

	mixed := checkedScore(t, caseByName(t, res, "check-judge-mixed"))
	if mixed.Score != 2 || mixed.MaxScore != 2 {
		t.Errorf("check-judge-mixed = %d/%d, want exactly 2/2", mixed.Score, mixed.MaxScore)
	}
	if len(mixed.Checks) != 2 || mixed.Checks[0].Type != CheckFileExists || mixed.Checks[1].Type != CheckJudge {
		t.Errorf("check-judge-mixed checks = %+v, want [file_exists, judge]", mixed.Checks)
	}
}

func TestSelfTestJudgeFilesReachTheJudge(t *testing.T) {
	res := runSelfTestAgent(t, "selftest")
	c := caseByName(t, res, "check-judge-files")
	sc := checkedScore(t, c)
	jc := judgeChecks(sc)
	if len(jc) != 1 || !jc[0].Passed {
		t.Fatalf("check-judge-files judge checks = %+v, want one pass", jc)
	}
	// The listed file is read from the agent's workspace; the stub wrote it
	// through its own command, so a pass proves it was found and read.
	if calls := judgeCalls(c); len(calls) != 1 {
		t.Errorf("judge calls = %d, want 1", len(calls))
	}
}

func TestSelfTestJudgeFailingCases(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")

	want := []struct {
		name      string
		detail    string
		judgeCall bool   // a judge backend call is recorded
		callError string // substring of the recorded call error, if any
	}{
		{"check-judge-no", "judge answered no: stub judge: no", true, ""},
		{"check-judge-garbage", "judge parse error", true, ""},
		{"check-judge-no-script", "judge call failed", true, "no stub.judge"},
		{"check-judge-missing-file", "file does not exist", false, ""},
	}
	for _, w := range want {
		c := caseByName(t, res, w.name)
		sc := checkedScore(t, c)
		if sc.Skipped {
			t.Errorf("%s: criterion was skipped; a judge failure must count", w.name)
		}
		if sc.MaxScore != 1 || sc.Score != 0 || !sc.Deterministic {
			t.Errorf("%s: score = %d/%d (deterministic %v), want 0/1", w.name, sc.Score, sc.MaxScore, sc.Deterministic)
		}
		jc := judgeChecks(sc)
		if len(jc) != 1 || jc[0].Passed || !strings.Contains(jc[0].Detail, w.detail) {
			t.Errorf("%s: judge checks = %+v, want one failure with detail containing %q", w.name, jc, w.detail)
		}
		if !strings.Contains(sc.Reasoning, jc[0].Label) {
			t.Errorf("%s: reasoning %q does not name failed check %q", w.name, sc.Reasoning, jc[0].Label)
		}
		calls := judgeCalls(c)
		if w.judgeCall && len(calls) != 1 {
			t.Errorf("%s: judge calls = %+v, want 1", w.name, calls)
		}
		if !w.judgeCall && len(calls) != 0 {
			t.Errorf("%s: judge calls = %+v, want none (fails before any model call)", w.name, calls)
		}
		if w.callError != "" && (len(calls) != 1 || !strings.Contains(calls[0].Error, w.callError)) {
			t.Errorf("%s: judge call error = %+v, want it to contain %q", w.name, calls, w.callError)
		}
	}
}

func TestSelfTestJudgeListIsOneOfTwo(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")
	c := caseByName(t, res, "check-judge-list")
	sc := checkedScore(t, c)
	if sc.Score != 1 || sc.MaxScore != 2 || sc.Skipped {
		t.Fatalf("check-judge-list = %d/%d (skipped %v), want exactly 1/2", sc.Score, sc.MaxScore, sc.Skipped)
	}
	jc := judgeChecks(sc)
	if len(jc) != 2 || !jc[0].Passed || jc[1].Passed {
		t.Fatalf("judge checks = %+v, want [pass, fail]", jc)
	}
	if !strings.Contains(jc[0].Detail, "judge answered yes") || !strings.Contains(jc[1].Detail, "judge answered no") {
		t.Errorf("details = %q / %q, want yes then no reasoning", jc[0].Detail, jc[1].Detail)
	}
	if got := len(judgeCalls(c)); got != 2 {
		t.Errorf("judge calls = %d, want 2", got)
	}
}

// loadSelfTestCase loads one case of agent from a temp copy of the fixtures,
// configured for the stub backend, so a test can run the same loaded
// TestCase more than once.
func loadSelfTestCase(t *testing.T, agent, name string) (Rubric, TestCase) {
	t.Helper()
	t.Cleanup(resetConfig)
	evalsDir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, evalsDir)
	if err := configure(RunOptions{Backend: "stub", EvalsDir: evalsDir, NoSandbox: true}); err != nil {
		t.Fatal(err)
	}
	rubrics, err := loadRubrics(agent)
	if err != nil || len(rubrics) != 1 {
		t.Fatalf("loadRubrics(%s) = %v, %v", agent, rubrics, err)
	}
	cases, err := loadCases(agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		if tc.Name == name {
			return rubrics[0], tc
		}
	}
	t.Fatalf("case %s/%s not found", agent, name)
	return Rubric{}, TestCase{}
}

func verdicts(t *testing.T, c CaseResult) []bool {
	t.Helper()
	var out []bool
	for _, cr := range judgeChecks(checkedScore(t, c)) {
		out = append(out, cr.Passed)
	}
	return out
}

// TestSelfTestJudgeRepeatContinuesCursor pins that re-running the same loaded
// case continues the stub.judge cursor instead of restarting it.
func TestSelfTestJudgeRepeatContinuesCursor(t *testing.T) {
	rubric, tc := loadSelfTestCase(t, "selftest-fail", "check-judge-list")
	if tc.Stub == nil || len(tc.Stub.Judge) != 2 || len(tc.Checks) != 2 {
		t.Fatalf("check-judge-list: stub %+v, %d checks; want a 2-answer script and 2 checks", tc.Stub, len(tc.Checks))
	}

	// Two answers for two checks wrap exactly once per run: every run sees
	// [yes, no].
	for i := 1; i <= 2; i++ {
		got := verdicts(t, executeCase(rubric, tc, nil, io.Discard, false))
		if len(got) != 2 || !got[0] || got[1] {
			t.Fatalf("run %d: verdicts = %v, want [true false]", i, got)
		}
	}

	// A 3-answer script against 2 checks per run offsets each run: the second
	// run starts where the first stopped. A restarting cursor would repeat
	// [true false] every time.
	tc.Stub.Judge = inference.StubJudge{"yes", "no", "yes"}
	// The cursor is at 0 after the two full wraps above (2 runs * 2 answers
	// over len 2), so it starts from the beginning of the new list.
	wants := [][]bool{{true, false}, {true, true}, {false, true}}
	for i, want := range wants {
		got := verdicts(t, executeCase(rubric, tc, nil, io.Discard, false))
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("run %d: verdicts = %v, want %v (cursor must continue, not restart)", i+1, got, want)
		}
	}
}

// TestSelfTestJudgeFreshLoadRestartsCursor documents the flip side: a fresh
// load of the case has a fresh cursor.
func TestSelfTestJudgeFreshLoadRestartsCursor(t *testing.T) {
	rubric, tc := loadSelfTestCase(t, "selftest-fail", "check-judge-list")
	tc.Stub.Judge = inference.StubJudge{"yes", "no", "yes"}
	first := verdicts(t, executeCase(rubric, tc, nil, io.Discard, false))
	second := verdicts(t, executeCase(rubric, tc, nil, io.Discard, false))
	if len(first) != 2 || len(second) != 2 || !first[0] || first[1] || !second[0] || !second[1] {
		t.Fatalf("setup: first %v second %v", first, second)
	}

	rubric2, fresh := loadSelfTestCase(t, "selftest-fail", "check-judge-list")
	fresh.Stub.Judge = inference.StubJudge{"yes", "no", "yes"}
	got := verdicts(t, executeCase(rubric2, fresh, nil, io.Discard, false))
	if len(got) != 2 || !got[0] || got[1] {
		t.Errorf("freshly loaded case verdicts = %v, want [true false]", got)
	}
}
