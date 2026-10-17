package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// End-to-end tests for the run-coverage fields (suite_sha256, complete,
// case_filter) written by RunWithOptions, and for the resume refusal that
// keeps one result file from mixing two eval suites. They use the stub
// backend and a temp copy of the self-test evals directory.

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// rawString decodes a raw JSON string value, failing when it is absent.
func rawString(t *testing.T, label string, m map[string]json.RawMessage, key string) string {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("%s has no %q key", label, key)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("%s %q is not a string: %v", label, key, err)
	}
	return s
}

// rawBool decodes a raw JSON bool value, failing when it is absent.
func rawBool(t *testing.T, label string, m map[string]json.RawMessage, key string) bool {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		t.Fatalf("%s has no %q key", label, key)
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("%s %q is not a bool: %v", label, key, err)
	}
	return b
}

// runE2ECase is runE2E for a run that names a testcase (--case).
func runE2ECase(t *testing.T, evalsDir, testcase string, opts RunOptions) e2eRun {
	t.Helper()
	resultsDir := filepath.Join(evalsDir, "results")
	before := snapshotDirs(t, resultsDir)

	opts.Backend = "stub"
	opts.NoSandbox = true
	opts.EvalsDir = evalsDir
	if err := RunWithOptions(e2eAgent, testcase, opts); err != nil {
		t.Fatalf("stub run of case %q failed: %v", testcase, err)
	}
	added := newRunDirs(t, resultsDir, before)
	if len(added) != 1 {
		t.Fatalf("run created %d result dirs, want 1: %v", len(added), added)
	}
	dir := filepath.Join(resultsDir, added[0])
	run := e2eRun{dir: dir}
	run.agent = readSelfTestResult(t, filepath.Join(dir, e2eAgent+".json"))
	run.rawAgnt = readRawJSON(t, filepath.Join(dir, e2eAgent+".json"))
	run.rawSum = readRawJSON(t, filepath.Join(dir, "summary.json"))
	var agents map[string]map[string]json.RawMessage
	if err := json.Unmarshal(run.rawSum["agents"], &agents); err != nil {
		t.Fatalf("parsing summary.json agents: %v", err)
	}
	run.rawSumA = agents[e2eAgent]
	return run
}

// TestCoverageRunFullRunRecordsSuiteAndComplete: a full stub run records
// suite_sha256, complete:true and an empty case_filter under agents.<agent>
// in summary.json and in <agent>.json.
func TestCoverageRunFullRunRecordsSuiteAndComplete(t *testing.T) {
	env := newCandidateE2EEnv(t)
	run := runE2E(t, env.evalsDir, RunOptions{})

	for label, m := range map[string]map[string]json.RawMessage{
		"<agent>.json":          run.rawAgnt,
		"summary.json agents.*": run.rawSumA,
	} {
		suite := rawString(t, label, m, "suite_sha256")
		if !sha256Hex.MatchString(suite) {
			t.Errorf("%s suite_sha256 = %q, want 64 lowercase hex chars", label, suite)
		}
		if !rawBool(t, label, m, "complete") {
			t.Errorf("%s complete = false, want true for a full run", label)
		}
		if got := rawString(t, label, m, "case_filter"); got != "" {
			t.Errorf("%s case_filter = %q, want an explicit empty string", label, got)
		}
	}
	if a, b := rawString(t, "<agent>.json", run.rawAgnt, "suite_sha256"),
		rawString(t, "summary.json", run.rawSumA, "suite_sha256"); a != b {
		t.Errorf("suite_sha256 differs between files: %s vs %s", a, b)
	}
	want, err := func() (string, error) {
		if err := configure(RunOptions{EvalsDir: env.evalsDir}); err != nil {
			return "", err
		}
		return suiteSHA256(e2eAgent)
	}()
	if err != nil {
		t.Fatal(err)
	}
	if got := run.agent.SuiteSHA256; got != want {
		t.Errorf("recorded suite_sha256 = %s, want suiteSHA256(%s) = %s", got, e2eAgent, want)
	}
}

// TestCoverageRunCaseEditChangesSuiteNotPrompt: editing one case file between
// two runs changes suite_sha256 and leaves prompt_sha256 unchanged.
func TestCoverageRunCaseEditChangesSuiteNotPrompt(t *testing.T) {
	env := newCandidateE2EEnv(t)
	first := runE2E(t, env.evalsDir, RunOptions{})

	caseFile := filepath.Join(env.evalsDir, "cases", e2eAgent, "stub-basic.yaml")
	data, err := os.ReadFile(caseFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caseFile, append(data, []byte("# edited between runs\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	second := runE2E(t, env.evalsDir, RunOptions{})

	if first.agent.SuiteSHA256 == "" || second.agent.SuiteSHA256 == "" {
		t.Fatalf("suite_sha256 not recorded: first=%q second=%q", first.agent.SuiteSHA256, second.agent.SuiteSHA256)
	}
	if first.agent.SuiteSHA256 == second.agent.SuiteSHA256 {
		t.Errorf("suite_sha256 unchanged (%s) after editing a case file", first.agent.SuiteSHA256)
	}
	if first.agent.PromptSHA256 == "" || first.agent.PromptSHA256 != second.agent.PromptSHA256 {
		t.Errorf("prompt_sha256 changed or empty: %q -> %q", first.agent.PromptSHA256, second.agent.PromptSHA256)
	}
}

// TestCoverageRunCaseFilterRecorded: a --case run records the case name in
// case_filter and complete:false, even though the case ran fully.
func TestCoverageRunCaseFilterRecorded(t *testing.T) {
	env := newCandidateE2EEnv(t)
	full := runE2E(t, env.evalsDir, RunOptions{})
	run := runE2ECase(t, env.evalsDir, "stub-basic", RunOptions{})

	for label, m := range map[string]map[string]json.RawMessage{
		"<agent>.json":          run.rawAgnt,
		"summary.json agents.*": run.rawSumA,
	} {
		if got := rawString(t, label, m, "case_filter"); got != "stub-basic" {
			t.Errorf("%s case_filter = %q, want stub-basic", label, got)
		}
		if rawBool(t, label, m, "complete") {
			t.Errorf("%s complete = true for a --case run, want false", label)
		}
	}
	// A filtered run and a full run of the same suite hash alike.
	if run.agent.SuiteSHA256 != full.agent.SuiteSHA256 {
		t.Errorf("--case suite_sha256 = %s, full run = %s; want equal", run.agent.SuiteSHA256, full.agent.SuiteSHA256)
	}
}

// TestCoverageRunRepeatTwoCompleteWhenBothRunsExist: with --repeat 2 the run
// is complete, and only because both runs of every case exist.
func TestCoverageRunRepeatTwoCompleteWhenBothRunsExist(t *testing.T) {
	env := newCandidateE2EEnv(t)
	run := runE2E(t, env.evalsDir, RunOptions{Repeat: 2})

	if !boolOf(t, "Complete", run.agent.Complete) {
		t.Fatal("complete = false after a finished --repeat 2 run")
	}
	runs := map[string]map[int]bool{}
	for _, c := range run.agent.Cases {
		if runs[c.CaseName] == nil {
			runs[c.CaseName] = map[int]bool{}
		}
		runs[c.CaseName][c.Run] = true
	}
	if len(runs) == 0 {
		t.Fatal("run has no cases")
	}
	for name, rs := range runs {
		if !rs[1] || !rs[2] {
			t.Errorf("case %s has runs %v, want 1 and 2", name, rs)
		}
	}

	// Drop one run of one case: the same result is no longer covered.
	var trimmed []CaseResult
	dropped := false
	for _, c := range run.agent.Cases {
		if !dropped && c.Run == 2 {
			dropped = true
			continue
		}
		trimmed = append(trimmed, c)
	}
	if casesCovered(trimmed, caseListOf(runs), 2) {
		t.Error("a result missing run 2 of one case counts as covered")
	}
}

func caseListOf(runs map[string]map[int]bool) []TestCase {
	var tcs []TestCase
	for name := range runs {
		tcs = append(tcs, TestCase{Name: name})
	}
	return tcs
}

// markInterrupted flags a finished run as interrupted so --resume picks it up.
func markInterrupted(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".progress"), []byte("interrupted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resumeE2E(evalsDir string) error {
	return RunWithOptions(e2eAgent, "", RunOptions{Backend: "stub", NoSandbox: true, EvalsDir: evalsDir, Resume: true})
}

// TestCoverageResumeUnchangedSuiteContinues: resuming with the suite as it was
// is allowed and keeps the recorded suite hash.
func TestCoverageResumeUnchangedSuiteContinues(t *testing.T) {
	env := newCandidateE2EEnv(t)
	run := runE2E(t, env.evalsDir, RunOptions{})
	markInterrupted(t, run.dir)

	if err := resumeE2E(env.evalsDir); err != nil {
		t.Fatalf("resume with an unchanged suite was refused: %v", err)
	}
	after := readSelfTestResult(t, filepath.Join(run.dir, e2eAgent+".json"))
	if after.SuiteSHA256 != run.agent.SuiteSHA256 {
		t.Errorf("suite_sha256 after resume = %s, want %s", after.SuiteSHA256, run.agent.SuiteSHA256)
	}
	if !boolOf(t, "Complete", after.Complete) {
		t.Error("complete = false after a resume that finished every case")
	}
}

// TestCoverageResumeChangedSuiteRefused: editing a case file after the
// interrupted run refuses the resume, naming the agent and both hashes, and
// leaves the saved result untouched.
func TestCoverageResumeChangedSuiteRefused(t *testing.T) {
	env := newCandidateE2EEnv(t)
	run := runE2E(t, env.evalsDir, RunOptions{})
	markInterrupted(t, run.dir)
	agentFile := filepath.Join(run.dir, e2eAgent+".json")
	before, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatal(err)
	}

	caseFile := filepath.Join(env.evalsDir, "cases", e2eAgent, "stub-basic.yaml")
	data, err := os.ReadFile(caseFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caseFile, append(data, []byte("# edited after the interruption\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := configure(RunOptions{EvalsDir: env.evalsDir}); err != nil {
		t.Fatal(err)
	}
	now, err := suiteSHA256(e2eAgent)
	if err != nil {
		t.Fatal(err)
	}

	err = resumeE2E(env.evalsDir)
	if err == nil {
		t.Fatal("resume with a changed suite succeeded, want a refusal")
	}
	for _, s := range []string{"cannot resume", "eval suite changed", e2eAgent, run.agent.SuiteSHA256, now} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("refusal %q does not contain %q", err, s)
		}
	}
	after, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("refused resume modified the saved result")
	}
}

// writeResumeFile writes <dir>/<agent>.json with the given suite_sha256
// fragment (e.g. `"suite_sha256":"…",`) and cases JSON.
func writeResumeFile(t *testing.T, dir, suiteFragment, casesJSON string) {
	t.Helper()
	body := `{"agent":"architect","git_hash":"g","sandbox":false,` + suiteFragment + `"cases":` + casesJSON + `}`
	if err := os.WriteFile(filepath.Join(dir, "architect.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageResumeIntegritySuite(t *testing.T) {
	const (
		saved = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		now   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	const withCases = `[{"case_name":"c1"}]`

	t.Run("same suite passes", func(t *testing.T) {
		t.Cleanup(resetConfig)
		cfg = runConfig{suites: map[string]string{"architect": saved}}
		dir := t.TempDir()
		writeResumeFile(t, dir, `"suite_sha256":"`+saved+`",`, withCases)
		if err := checkResumeIntegrity(dir, "architect", false); err != nil {
			t.Fatalf("unchanged suite refused: %v", err)
		}
	})

	t.Run("changed suite is refused naming agent and both hashes", func(t *testing.T) {
		t.Cleanup(resetConfig)
		cfg = runConfig{suites: map[string]string{"architect": now}}
		dir := t.TempDir()
		writeResumeFile(t, dir, `"suite_sha256":"`+saved+`",`, withCases)
		err := checkResumeIntegrity(dir, "architect", false)
		if err == nil {
			t.Fatal("changed suite was not refused")
		}
		for _, s := range []string{"cannot resume", "eval suite changed", "architect", saved, now} {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("refusal %q does not contain %q", err, s)
			}
		}
	})

	t.Run("saved cases without suite_sha256 is refused", func(t *testing.T) {
		t.Cleanup(resetConfig)
		cfg = runConfig{suites: map[string]string{"architect": now}}
		dir := t.TempDir()
		writeResumeFile(t, dir, "", withCases)
		err := checkResumeIntegrity(dir, "architect", false)
		if err == nil {
			t.Fatal("a result with cases but no suite_sha256 was not refused")
		}
		for _, s := range []string{"cannot resume", "architect", now} {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("refusal %q does not contain %q", err, s)
			}
		}
	})

	t.Run("empty file is not refused", func(t *testing.T) {
		t.Cleanup(resetConfig)
		cfg = runConfig{suites: map[string]string{"architect": now}}
		dir := t.TempDir()
		writeResumeFile(t, dir, "", `[]`)
		if err := checkResumeIntegrity(dir, "architect", false); err != nil {
			t.Fatalf("empty file refused: %v", err)
		}
		writeResumeFile(t, dir, `"suite_sha256":"`+saved+`",`, `[]`)
		if err := checkResumeIntegrity(dir, "architect", false); err != nil {
			t.Fatalf("empty file with a different suite refused: %v", err)
		}
	})

	t.Run("nothing checked without a suites entry", func(t *testing.T) {
		t.Cleanup(resetConfig)
		dir := t.TempDir()
		writeResumeFile(t, dir, "", withCases)
		cfg = runConfig{} // suites == nil
		if err := checkResumeIntegrity(dir, "architect", false); err != nil {
			t.Fatalf("nil suites refused: %v", err)
		}
		cfg = runConfig{suites: map[string]string{"other": now}}
		if err := checkResumeIntegrity(dir, "architect", false); err != nil {
			t.Fatalf("suites without an entry for the agent refused: %v", err)
		}
	})
}

// TestCoverageRunSuiteHashErrorStopsBeforeAnyCase: a suite input that cannot
// be read (a setup path that is a directory) returns the hashing error before
// the run creates its results directory or starts a case.
func TestCoverageRunSuiteHashErrorStopsBeforeAnyCase(t *testing.T) {
	setupPinProject(t, "", "")
	writeProjectFile(t, ".kairon/evals/cases/architect/c1.yaml",
		"name: c1\ndescription: d\nagent: architect\ninput: do it\nsetup:\n  - type: file\n    label: L\n    path: .kairon/evals/fixtures/a-directory\nstub:\n  turns:\n    - response: ok\n")
	if err := os.MkdirAll(filepath.Join(".kairon", "evals", "fixtures", "a-directory"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := RunWithOptions("architect", "", RunOptions{Backend: "stub", NoSandbox: true})
	if err == nil {
		t.Fatal("run succeeded, want the suite hashing error")
	}
	if !strings.Contains(err.Error(), "a-directory") {
		t.Errorf("error %q does not name the unreadable suite input", err)
	}
	if entries, rerr := os.ReadDir(filepath.Join(".kairon", "evals", "results")); rerr == nil && len(entries) > 0 {
		t.Errorf("run created results %v before failing; want none (no case should start)", entries)
	}
}
