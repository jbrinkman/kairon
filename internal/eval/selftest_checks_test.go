package eval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selftestCheckTypes maps each of the ten check types to the suffix used in the
// self-test case names: `check-<suffix>` exists under both `selftest` (passes)
// and `selftest-fail` (fails).
var selftestCheckTypes = []struct {
	typ    CheckType
	suffix string
	// failWant is a substring that the failing case's reasoning must contain
	// besides the failed check's label (the check detail or a named path).
	failWant []string
}{
	{CheckCommand, "command", []string{"exit"}},
	{CheckFileExists, "file-exists", []string{"missing.txt"}},
	{CheckFileAbsent, "file-absent", []string{"marker.txt"}},
	{CheckFileContains, "file-contains", []string{"marker.txt"}},
	{CheckFileNotContains, "file-not-contains", []string{"marker.txt"}},
	{CheckChangedFiles, "changed-files", []string{"docs/notes.txt", "extra.txt"}},
	{CheckOutputContains, "output-contains", []string{"output_contains"}},
	{CheckOutputNotContains, "output-not-contains", []string{"output_not_contains"}},
	{CheckGHLogContains, "gh-log-contains", []string{"gh_log_contains"}},
	{CheckGHLogNotContains, "gh-log-not-contains", []string{"gh pr merge"}},
}

// runSelfTestAgent runs agent with the stub backend, natively, against a copy
// of the checked-in fixtures and keeps the case workspaces (removed when the
// test ends). It returns the parsed result.
func runSelfTestAgent(t *testing.T, agent string) AgentResult {
	t.Helper()
	t.Cleanup(resetConfig)
	evalsDir := filepath.Join(t.TempDir(), "evals")
	copyFixturesTo(t, evalsDir)
	res := runEvalIn(t, evalsDir, agent, RunOptions{NoSandbox: true, KeepWorkspaces: true})
	for _, c := range res.Cases {
		cleanupWorkspace(t, c)
	}
	return res
}

// checkedScore returns the structural_completeness score of a case, which every
// self-test check case attaches its checks to.
func checkedScore(t *testing.T, c CaseResult) CriterionScore {
	t.Helper()
	for _, s := range c.Scores {
		if s.Name == "structural_completeness" {
			return s
		}
	}
	t.Fatalf("case %s has no structural_completeness score: %+v", c.CaseName, c.Scores)
	return CriterionScore{}
}

func TestSelfTestChecksPassingCases(t *testing.T) {
	res := runSelfTestAgent(t, "selftest")

	if got, want := len(res.Cases), 5+1+11+2+3+2+2; got != want {
		t.Errorf("selftest has %d cases, want %d (5 legacy + 1 conventions-fixture + 11 check + 2 multi-turn + 3 judge + 2 simulated-user + 2 judge-transcript)", got, want)
	}

	seen := map[CheckType]bool{}
	for _, tt := range selftestCheckTypes {
		c := caseByName(t, res, "check-"+tt.suffix)
		if c.ErrorContext != nil {
			t.Errorf("%s: unexpected ErrorContext %+v", c.CaseName, c.ErrorContext)
		}
		sc := checkedScore(t, c)
		if len(sc.Checks) == 0 {
			t.Errorf("%s: no checks recorded", c.CaseName)
			continue
		}
		if sc.Score != sc.MaxScore || sc.MaxScore != len(sc.Checks) || !sc.Deterministic || sc.Skipped {
			t.Errorf("%s: score = %d/%d (checks %d, deterministic %v, skipped %v), want N/N",
				c.CaseName, sc.Score, sc.MaxScore, len(sc.Checks), sc.Deterministic, sc.Skipped)
		}
		if want := fmt.Sprintf("%d/%d checks passed", sc.MaxScore, sc.MaxScore); sc.Reasoning != want {
			t.Errorf("%s: reasoning = %q, want %q", c.CaseName, sc.Reasoning, want)
		}
		for _, cr := range sc.Checks {
			if !cr.Passed {
				t.Errorf("%s: check %s failed: %s", c.CaseName, cr.Label, cr.Detail)
			}
			if cr.Type == tt.typ {
				seen[tt.typ] = true
			}
		}
		// The unchecked clarity criterion is untouched by checks.
		for _, s := range c.Scores {
			if s.Name == "clarity" && (s.Checks != nil || s.Score != s.MaxScore) {
				t.Errorf("%s: clarity = %+v, want an unchecked maximum score", c.CaseName, s)
			}
		}
	}
	for _, tt := range selftestCheckTypes {
		if !seen[tt.typ] {
			t.Errorf("no passing selftest case exercises check type %s", tt.typ)
		}
	}

	// The multi-turn cases pass every turn-scoped and default-last check.
	for _, name := range []string{multiTurnGHLogCase, multiTurnWorkspaceCase} {
		c := caseByName(t, res, name)
		if c.ErrorContext != nil {
			t.Errorf("%s: unexpected ErrorContext %+v", name, c.ErrorContext)
		}
		sc := checkedScore(t, c)
		if len(sc.Checks) == 0 || sc.Score != sc.MaxScore || sc.MaxScore != len(sc.Checks) {
			t.Errorf("%s: score = %d/%d (checks %d), want N/N: %s", name, sc.Score, sc.MaxScore, len(sc.Checks), sc.Reasoning)
		}
		for _, cr := range sc.Checks {
			if !cr.Passed {
				t.Errorf("%s: check %s failed: %s", name, cr.Label, cr.Detail)
			}
		}
	}
	gh := caseByName(t, res, multiTurnGHLogCase)
	assertMultiTurnGHLogResult(t, gh)
	var scoped int
	for _, cr := range checkedScore(t, gh).Checks {
		if strings.Contains(cr.Label, "turn=") {
			scoped++
		}
	}
	if scoped != 5 {
		t.Errorf("%s has %d turn-scoped checks, want 5", multiTurnGHLogCase, scoped)
	}
	ws := caseByName(t, res, multiTurnWorkspaceCase)
	if len(ws.Calls) < 2 {
		t.Errorf("%s has %d calls, want at least 2 agent calls", multiTurnWorkspaceCase, len(ws.Calls))
	}

	// check-command also covers a non-zero expect_exit.
	cmd := checkedScore(t, caseByName(t, res, "check-command"))
	if len(cmd.Checks) != 2 {
		t.Errorf("check-command records %d checks, want 2 (exit 0 and expect_exit 3)", len(cmd.Checks))
	}
}

func TestSelfTestChecksFailingCases(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")

	if got, want := len(res.Cases), 10+1+1+1+1+6+1+1; got != want {
		t.Errorf("selftest-fail has %d cases, want %d (10 per-type + check-partial + stub-timeout + stub-empty-response + multi-turn-wrong-turn + 6 judge + sim-fact-not-asked + calls-malformed-line)", got, want)
	}

	for _, tt := range selftestCheckTypes {
		c := caseByName(t, res, "check-"+tt.suffix)
		sc := checkedScore(t, c)
		if sc.MaxScore == 0 || sc.Score >= sc.MaxScore || !sc.Deterministic || sc.Skipped {
			t.Errorf("%s: score = %d/%d (deterministic %v, skipped %v), want < N/N",
				c.CaseName, sc.Score, sc.MaxScore, sc.Deterministic, sc.Skipped)
		}
		var failedOfType bool
		for _, cr := range sc.Checks {
			if cr.Passed {
				continue
			}
			if cr.Type == tt.typ {
				failedOfType = true
			}
			if !strings.Contains(sc.Reasoning, cr.Label) {
				t.Errorf("%s: reasoning %q does not name failed check %q", c.CaseName, sc.Reasoning, cr.Label)
			}
		}
		if !failedOfType {
			t.Errorf("%s: no failed %s check recorded: %+v", c.CaseName, tt.typ, sc.Checks)
		}
		for _, want := range tt.failWant {
			if !strings.Contains(sc.Reasoning, want) {
				t.Errorf("%s: reasoning %q does not mention %q", c.CaseName, sc.Reasoning, want)
			}
		}
	}
}

// TestSelfTestChecksWrongTurnFails proves turn scoping is real: the turn-1
// gh_log_contains check names a call made in turn 3, so it fails even though
// the final log contains it.
func TestSelfTestChecksWrongTurnFails(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")
	c := caseByName(t, res, multiTurnWrongTurnCase)
	if len(c.TurnOutputs) != 3 {
		t.Errorf("turn_outputs = %q, want 3 entries", c.TurnOutputs)
	}
	sc := checkedScore(t, c)
	if sc.Score >= sc.MaxScore || sc.MaxScore == 0 {
		t.Fatalf("score = %d/%d, want a failed case: %s", sc.Score, sc.MaxScore, sc.Reasoning)
	}
	var failed int
	for _, cr := range sc.Checks {
		if cr.Passed {
			continue
		}
		failed++
		if cr.Type != CheckGHLogContains || !strings.Contains(cr.Label, "turn=1") {
			t.Errorf("failed check %q (%s), want the gh_log_contains turn=1 check", cr.Label, cr.Type)
		}
		if !strings.Contains(sc.Reasoning, cr.Label) {
			t.Errorf("reasoning %q does not name failed check %q", sc.Reasoning, cr.Label)
		}
	}
	if failed != 1 {
		t.Errorf("%d checks failed, want exactly 1: %+v", failed, sc.Checks)
	}
	if !strings.Contains(sc.Reasoning, "gh_log_contains") || !strings.Contains(sc.Reasoning, "turn=1") {
		t.Errorf("reasoning %q must name the gh_log_contains check with turn=1", sc.Reasoning)
	}
}

// TestSelfTestEmptyResponseCountsAsZero: the stub-empty-response case gets no
// agent output, so every criterion scores 0 out of a positive maximum, stays in
// the denominator, and drags the selftest-fail agent score down.
func TestSelfTestEmptyResponseCountsAsZero(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")
	c := caseByName(t, res, "stub-empty-response")

	if c.ActualOutput != "" {
		t.Errorf("ActualOutput = %q, want empty", c.ActualOutput)
	}
	if len(c.Scores) == 0 {
		t.Fatal("stub-empty-response recorded no scores")
	}
	for _, s := range c.Scores {
		if s.Score != 0 || s.MaxScore <= 0 || !s.Skipped {
			t.Errorf("criterion %s = %d/%d (skipped %v), want a skipped 0 with MaxScore > 0",
				s.Name, s.Score, s.MaxScore, s.Skipped)
		}
	}
	cs, cm := caseTotals(c)
	if cs != 0 || cm == 0 {
		t.Errorf("caseTotals = (%d, %d), want (0, >0): the case must stay in the denominator", cs, cm)
	}

	// The agent score with the case is lower than without it.
	withScore, withMax := agentScoreTotals(res)
	without := res
	without.Cases = nil
	for _, other := range res.Cases {
		if other.CaseName != c.CaseName {
			without.Cases = append(without.Cases, other)
		}
	}
	woScore, woMax := agentScoreTotals(without)
	if withScore != woScore || withMax != woMax+float64(cm) {
		t.Errorf("totals with = %v/%v, without = %v/%v, want the case to add 0/%d", withScore, withMax, woScore, woMax, cm)
	}
	if with, wo := withScore/withMax, woScore/woMax; !(with < wo) {
		t.Errorf("agent score with the empty-response case = %v, want lower than %v without it", with, wo)
	}
}

func TestSelfTestChecksPartialIsOneOfTwo(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")
	sc := checkedScore(t, caseByName(t, res, "check-partial"))
	if sc.Score != 1 || sc.MaxScore != 2 {
		t.Fatalf("check-partial = %d/%d, want exactly 1/2", sc.Score, sc.MaxScore)
	}
	if !strings.Contains(sc.Reasoning, "1/2 checks passed") ||
		!strings.Contains(sc.Reasoning, "#2 file_exists path=missing.txt") {
		t.Errorf("reasoning = %q, want it to name '#2 file_exists path=missing.txt'", sc.Reasoning)
	}
	if len(sc.Checks) != 2 || !sc.Checks[0].Passed || sc.Checks[1].Passed {
		t.Errorf("checks = %+v, want [#1 passed, #2 failed]", sc.Checks)
	}
}

// TestSelfTestChecksInjectIsHiddenFromAgent is the end-to-end AC4 test: the
// stub's own `ls -R` (run while the agent works) must not see the injected
// hidden_test.go, the command check must pass with the file present, and the
// kept workspace must not contain it afterwards.
func TestSelfTestChecksInjectIsHiddenFromAgent(t *testing.T) {
	res := runSelfTestAgent(t, "selftest")
	c := caseByName(t, res, "check-command-inject")
	if c.WorkspaceDir == "" {
		t.Fatal("no workspace_dir recorded")
	}

	seen, err := os.ReadFile(filepath.Join(c.WorkspaceDir, ".eval", "seen.txt"))
	if err != nil {
		t.Fatalf("reading .eval/seen.txt: %v", err)
	}
	if len(seen) == 0 {
		t.Error(".eval/seen.txt is empty; the stub listing did not run")
	}
	if strings.Contains(string(seen), "hidden_test.go") {
		t.Errorf("the agent saw the injected file:\n%s", seen)
	}

	sc := checkedScore(t, c)
	var cmdPassed bool
	for _, cr := range sc.Checks {
		if cr.Type == CheckCommand {
			cmdPassed = cr.Passed
		}
	}
	if !cmdPassed {
		t.Errorf("the command check did not pass with the injected file present: %+v", sc.Checks)
	}
	if sc.Score != sc.MaxScore {
		t.Errorf("score = %d/%d, want N/N: %s", sc.Score, sc.MaxScore, sc.Reasoning)
	}

	if _, err := os.Lstat(filepath.Join(c.WorkspaceDir, "hidden_test.go")); !os.IsNotExist(err) {
		t.Errorf("hidden_test.go is still in the kept workspace (err=%v)", err)
	}
}

// TestSelfTestChecksLegacyScoresUnchanged pins that the five pre-existing
// selftest cases still score exactly as before (no checks, rubrics unchanged).
func TestSelfTestChecksLegacyScoresUnchanged(t *testing.T) {
	res := runSelfTestAgent(t, "selftest")
	for _, name := range []string{"stub-basic", "stub-usage", "stub-quoted-input", markerCase, seededCase} {
		c := caseByName(t, res, name)
		if len(c.Scores) != 2 {
			t.Fatalf("%s: scores = %+v, want 2", name, c.Scores)
		}
		sc, cl := c.Scores[0], c.Scores[1]
		if sc.Name != "structural_completeness" || sc.Score != 5 || sc.MaxScore != 5 ||
			sc.Reasoning != "found 2/2 expected structural elements" || sc.Checks != nil {
			t.Errorf("%s: structural_completeness = %+v", name, sc)
		}
		if cl.Name != "clarity" || cl.Score != 5 || cl.MaxScore != 5 || cl.Checks != nil {
			t.Errorf("%s: clarity = %+v", name, cl)
		}
	}
}

// TestSelfTestChecksRunExitStatus mirrors `task eval:selftest` (exits 0) and
// the selftest-fail run (exits non-zero with the threshold error).
func TestSelfTestChecksRunExitStatus(t *testing.T) {
	t.Cleanup(resetConfig)
	for agent, wantFail := range map[string]bool{"selftest": false, "selftest-fail": true} {
		evalsDir := filepath.Join(t.TempDir(), "evals")
		copyFixturesTo(t, evalsDir)
		err := RunWithOptions(agent, "", RunOptions{Backend: "stub", NoSandbox: true, EvalsDir: evalsDir})
		if wantFail != errors.Is(err, ErrThresholdFailed) {
			t.Errorf("run of %s: err = %v, want ErrThresholdFailed=%v", agent, err, wantFail)
		}
		if !wantFail && err != nil {
			t.Errorf("run of %s returned an error: %v", agent, err)
		}
	}
}

// checksOnlyInvalidDir is the deliberately invalid evals dir: its checks_only
// rubric owns a case without checks.
const checksOnlyInvalidDir = "testdata/checks-only-invalid"

// criterionByName returns the score recorded for the named criterion and
// whether the case recorded one.
func criterionByName(c CaseResult, name string) (CriterionScore, bool) {
	for _, s := range c.Scores {
		if s.Name == name {
			return s, true
		}
	}
	return CriterionScore{}, false
}

// TestSelfTestChecksOnlyPasses: selftest-checks scores two cases from their
// checks alone. check-one-criterion has checks on file_written only, so
// report_marker is absent; check-both-criteria scores both in full. Neither
// case makes a judge call.
func TestSelfTestChecksOnlyPasses(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-checks")

	if len(res.Cases) != 2 {
		t.Fatalf("selftest-checks has %d cases, want 2", len(res.Cases))
	}
	for _, c := range res.Cases {
		if c.ErrorContext != nil {
			t.Errorf("%s: unexpected ErrorContext %+v", c.CaseName, c.ErrorContext)
		}
		if c.Passed == nil || !*c.Passed {
			t.Errorf("%s: Passed = %v, want true", c.CaseName, c.Passed)
		}
		for _, call := range c.Calls {
			if call.Role == "judge" {
				t.Errorf("%s: unexpected judge call %+v", c.CaseName, call)
			}
		}
		for _, s := range c.Scores {
			if s.Skipped || len(s.Checks) == 0 || s.Score != s.MaxScore || s.MaxScore != len(s.Checks) {
				t.Errorf("%s: criterion %s = %d/%d (checks %d, skipped %v), want N/N from checks",
					c.CaseName, s.Name, s.Score, s.MaxScore, len(s.Checks), s.Skipped)
			}
			for _, cr := range s.Checks {
				if !cr.Passed {
					t.Errorf("%s: check %s failed: %s", c.CaseName, cr.Label, cr.Detail)
				}
			}
		}
	}

	one := caseByName(t, res, "check-one-criterion")
	if len(one.Scores) != 1 {
		t.Fatalf("check-one-criterion scores = %+v, want exactly file_written", one.Scores)
	}
	if _, ok := criterionByName(one, "file_written"); !ok {
		t.Errorf("check-one-criterion has no file_written score: %+v", one.Scores)
	}
	if _, ok := criterionByName(one, "report_marker"); ok {
		t.Errorf("check-one-criterion scored report_marker, which has no checks: %+v", one.Scores)
	}

	both := caseByName(t, res, "check-both-criteria")
	if len(both.Scores) != 2 {
		t.Fatalf("check-both-criteria scores = %+v, want file_written and report_marker", both.Scores)
	}
	for _, name := range []string{"file_written", "report_marker"} {
		if _, ok := criterionByName(both, name); !ok {
			t.Errorf("check-both-criteria has no %s score", name)
		}
	}

	// The case totals equal the number of checks: nothing unchecked adds to
	// the numerator or the denominator.
	for _, c := range res.Cases {
		var n int
		for _, s := range c.Scores {
			n += len(s.Checks)
		}
		if cs, cm := caseTotals(c); cs != n || cm != n {
			t.Errorf("%s: caseTotals = (%d, %d), want (%d, %d)", c.CaseName, cs, cm, n, n)
		}
	}
	if as, am := agentScoreTotals(res); as != am || am == 0 {
		t.Errorf("agentScoreTotals = (%v, %v), want a full non-zero score", as, am)
	}
}

// TestSelfTestChecksOnlyNoJudgeCallsInResultFile: the persisted results JSON
// of the selftest-checks run holds no calls[] record with role judge.
func TestSelfTestChecksOnlyNoJudgeCallsInResultFile(t *testing.T) {
	t.Cleanup(resetConfig)
	evalsDir := copySelfTestEvals(t)
	if _, err := runStubCapturing(t, evalsDir, "selftest-checks", "", RunOptions{}); err != nil {
		t.Fatalf("selftest-checks run: %v", err)
	}
	res := readSelfTestResult(t, filepath.Join(onlyRunDir(t, evalsDir), "selftest-checks.json"))
	var agentCalls int
	for _, c := range res.Cases {
		for _, call := range c.Calls {
			switch call.Role {
			case "judge":
				t.Errorf("%s: judge call recorded: %+v", c.CaseName, call)
			case "agent":
				agentCalls++
			}
		}
	}
	if agentCalls == 0 {
		t.Error("no agent calls recorded; the no-judge assertion would be vacuous")
	}
}

// TestSelfTestChecksOnlyFailRecordsZeroPerCheckedCriterion: an empty stub
// response gives no output, so each checked criterion scores 0 out of its check
// count and the run fails the
// agent threshold.
func TestSelfTestChecksOnlyFailRecordsZeroPerCheckedCriterion(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-checks-fail")

	c := caseByName(t, res, "stub-empty-response")
	if c.ActualOutput != "" {
		t.Errorf("ActualOutput = %q, want empty", c.ActualOutput)
	}
	if len(c.Scores) != 2 {
		t.Fatalf("scores = %+v, want file_written and report_marker", c.Scores)
	}
	want := map[string]int{"file_written": 2, "report_marker": 1}
	for _, s := range c.Scores {
		n, ok := want[s.Name]
		if !ok {
			t.Errorf("unexpected criterion %s", s.Name)
			continue
		}
		if s.Score != 0 || s.MaxScore != n || s.Skipped || s.Reasoning != noOutputChecksReasoning {
			t.Errorf("%s = %d/%d (skipped %v, %q), want 0/%d with %q",
				s.Name, s.Score, s.MaxScore, s.Skipped, s.Reasoning, n, noOutputChecksReasoning)
		}
		for _, call := range c.Calls {
			if call.Role == "judge" {
				t.Errorf("unexpected judge call %+v", call)
			}
		}
	}
	if cs, cm := caseTotals(c); cs != 0 || cm != 3 {
		t.Errorf("caseTotals = (%d, %d), want (0, 3)", cs, cm)
	}
	if c.Passed == nil || *c.Passed {
		t.Errorf("Passed = %v, want false", c.Passed)
	}
}

// TestSelfTestChecksOnlyRunExitStatus: selftest-checks exits zero and
// selftest-checks-fail returns ErrThresholdFailed (non-zero exit).
func TestSelfTestChecksOnlyRunExitStatus(t *testing.T) {
	for agent, wantFail := range map[string]bool{"selftest-checks": false, "selftest-checks-fail": true} {
		evalsDir := copySelfTestEvals(t)
		_, err := runStubCapturing(t, evalsDir, agent, "", RunOptions{})
		if wantFail != errors.Is(err, ErrThresholdFailed) {
			t.Errorf("run of %s: err = %v, want ErrThresholdFailed=%v", agent, err, wantFail)
		}
		if !wantFail && err != nil {
			t.Errorf("run of %s returned an error: %v", agent, err)
		}
	}
}

// TestSelfTestChecksOnlyInvalidRejectedBeforeAnyCase: the checks-only-invalid
// evals dir holds a checks_only rubric whose only case has no checks. The run
// fails with an invalid-checks error naming the case file (not a threshold
// failure) and no case runs, so no agent result file is written.
func TestSelfTestChecksOnlyInvalidRejectedBeforeAnyCase(t *testing.T) {
	evalsDir := filepath.Join(t.TempDir(), "evals")
	if err := os.CopyFS(evalsDir, os.DirFS(checksOnlyInvalidDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(evalsDir, "results")); err != nil {
		t.Fatal(err)
	}
	_, err := runStubCapturing(t, evalsDir, "selftest-checks", "", RunOptions{})
	if err == nil {
		t.Fatal("run of the checks-only-invalid dir succeeded, want an error")
	}
	if errors.Is(err, ErrThresholdFailed) {
		t.Errorf("err = %v, want a load error rather than a threshold failure", err)
	}
	if !errors.Is(err, errInvalidChecks) {
		t.Errorf("err = %v, want errInvalidChecks", err)
	}
	if !strings.Contains(err.Error(), "no-checks.yaml") {
		t.Errorf("err = %q, want it to name no-checks.yaml", err.Error())
	}
	matches, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*", "selftest-checks*.json"))
	if len(matches) != 0 {
		t.Errorf("agent result files written for a rejected run: %v", matches)
	}
}
