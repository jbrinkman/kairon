package eval

import (
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

	if got, want := len(res.Cases), 5+11; got != want {
		t.Errorf("selftest has %d cases, want %d (5 legacy + 11 check cases)", got, want)
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

	// check-command also covers a non-zero expect_exit.
	cmd := checkedScore(t, caseByName(t, res, "check-command"))
	if len(cmd.Checks) != 2 {
		t.Errorf("check-command records %d checks, want 2 (exit 0 and expect_exit 3)", len(cmd.Checks))
	}
}

func TestSelfTestChecksFailingCases(t *testing.T) {
	res := runSelfTestAgent(t, "selftest-fail")

	if got, want := len(res.Cases), 10+1+1; got != want {
		t.Errorf("selftest-fail has %d cases, want %d (10 per-type + check-partial + stub-timeout)", got, want)
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

// TestSelfTestChecksRunExitsCleanly mirrors `task eval:selftest` and the
// selftest-fail run: failures are recorded in the results, not returned as an
// error.
func TestSelfTestChecksRunExitsCleanly(t *testing.T) {
	t.Cleanup(resetConfig)
	for _, agent := range []string{"selftest", "selftest-fail"} {
		evalsDir := filepath.Join(t.TempDir(), "evals")
		copyFixturesTo(t, evalsDir)
		if err := RunWithOptions(agent, "", RunOptions{Backend: "stub", NoSandbox: true, EvalsDir: evalsDir}); err != nil {
			t.Errorf("run of %s returned an error: %v", agent, err)
		}
	}
}
