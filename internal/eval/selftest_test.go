package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selftestEvalsDir is the checked-in self-test fixture set, relative to this
// package directory (the working directory of `go test`).
const selftestEvalsDir = "testdata/evals"

// snapshotDirs returns the names of the entries directly under dir. A missing
// dir yields an empty set.
func snapshotDirs(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}
		}
		t.Fatal(err)
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

// registerResultsCleanup removes every entry that appears under resultsDir
// after this call (and resultsDir itself if it did not exist beforehand), so
// the self-test leaves the working tree exactly as it found it. Registered
// before the run so it still fires if the run fails or the test aborts.
func registerResultsCleanup(t *testing.T, resultsDir string) {
	t.Helper()
	_, statErr := os.Stat(resultsDir)
	existed := statErr == nil
	before := snapshotDirs(t, resultsDir)
	t.Cleanup(func() {
		for name := range snapshotDirs(t, resultsDir) {
			if !before[name] {
				_ = os.RemoveAll(filepath.Join(resultsDir, name))
			}
		}
		if !existed {
			_ = os.Remove(resultsDir) // only succeeds when empty
		}
	})
}

// newRunDirs returns the entries under resultsDir that were not in before.
func newRunDirs(t *testing.T, resultsDir string, before map[string]bool) []string {
	t.Helper()
	var added []string
	for name := range snapshotDirs(t, resultsDir) {
		if !before[name] {
			added = append(added, name)
		}
	}
	return added
}

// pathWithoutKiroCLI returns PATH with every directory that holds a kiro-cli
// executable removed, so neither exec nor exec.LookPath can find a real one.
func pathWithoutKiroCLI(t *testing.T) string {
	t.Helper()
	var kept []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(d, "kiro-cli")); err == nil {
			continue
		}
		kept = append(kept, d)
	}
	return strings.Join(kept, string(os.PathListSeparator))
}

func readSelfTestResult(t *testing.T, path string) AgentResult {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading results JSON: %v", err)
	}
	var res AgentResult
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return res
}

// assertSelfTestResults checks the written selftest.json: the stub-usage case
// carries the reported usage from its stub turn, and every score (including
// LLM-judged ones) is at the maximum.
func assertSelfTestResults(t *testing.T, res AgentResult) {
	t.Helper()
	byName := map[string]CaseResult{}
	for _, c := range res.Cases {
		byName[c.CaseName] = c
	}
	for _, name := range []string{"stub-basic", "stub-usage"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("case %q missing from results (have %v)", name, byName)
		}
	}

	usage := byName["stub-usage"]
	if usage.AgentCost.TokensIn != 123 || usage.AgentCost.TokensOut != 45 {
		t.Errorf("stub-usage agent tokens = %d/%d, want 123/45",
			usage.AgentCost.TokensIn, usage.AgentCost.TokensOut)
	}
	if usage.AgentCost.UsageSource != "reported" {
		t.Errorf("stub-usage usage_source = %q, want reported", usage.AgentCost.UsageSource)
	}
	if usage.AgentCost.Model != "stub-model" {
		t.Errorf("stub-usage model = %q, want stub-model", usage.AgentCost.Model)
	}

	basic := byName["stub-basic"]
	if basic.AgentCost.UsageSource != "estimated" {
		t.Errorf("stub-basic usage_source = %q, want estimated", basic.AgentCost.UsageSource)
	}

	for _, c := range res.Cases {
		if c.ErrorContext != nil {
			t.Errorf("case %s has ErrorContext: %+v", c.CaseName, c.ErrorContext)
		}
		var judged int
		for _, s := range c.Scores {
			if s.Skipped {
				t.Errorf("case %s criterion %s skipped: %s", c.CaseName, s.Name, s.Reasoning)
				continue
			}
			if s.MaxScore == 0 || s.Score != s.MaxScore {
				t.Errorf("case %s criterion %s score = %d/%d, want maximum",
					c.CaseName, s.Name, s.Score, s.MaxScore)
			}
			if s.Name == "clarity" {
				judged++
			}
		}
		if judged != 1 {
			t.Errorf("case %s: found %d judge-scored clarity criteria, want 1", c.CaseName, judged)
		}
		if c.JudgeCost.Model != "stub" {
			t.Errorf("case %s judge model = %q, want stub", c.CaseName, c.JudgeCost.Model)
		}
	}
}

// TestSelfTestStubRunEndToEnd runs the selftest agent through RunWithOptions
// with the stub backend while a recording fake kiro-cli is first on PATH. Any
// kiro-cli invocation (agent, judge, or the --version startup probe) lands in
// the fake's log and fails the test.
func TestSelfTestStubRunEndToEnd(t *testing.T) {
	t.Cleanup(resetConfig)
	_, calls := installFakeKiroCLI(t, "echo 'kiro-cli must not run' >&2\nexit 99")

	resultsDir := filepath.Join(selftestEvalsDir, "results")
	registerResultsCleanup(t, resultsDir)
	before := snapshotDirs(t, resultsDir)

	err := RunWithOptions("selftest", "", RunOptions{Backend: "stub", EvalsDir: selftestEvalsDir})
	if err != nil {
		t.Fatalf("stub run failed: %v", err)
	}

	if got := readCalls(t, calls); len(got) != 0 {
		t.Fatalf("kiro-cli was invoked %d time(s) during a stub run: %q", len(got), got)
	}

	added := newRunDirs(t, resultsDir, before)
	if len(added) == 0 {
		t.Fatalf("no new run directory appeared under %s", resultsDir)
	}

	// Results must land under the evals dir, never under the default one.
	resultFile := filepath.Join(resultsDir, added[0], "selftest.json")
	assertSelfTestResults(t, readSelfTestResult(t, resultFile))
}

// TestSelfTestStubRunDoesNotConsultPath runs the stub backend against a copy
// of the fixtures with no kiro-cli anywhere on PATH. This catches a
// LookPath-style availability check that a recording fake cannot observe
// (LookPath only stats the file, it never executes it).
func TestSelfTestStubRunDoesNotConsultPath(t *testing.T) {
	t.Cleanup(resetConfig)

	evalsDir := filepath.Join(t.TempDir(), "evals")
	if err := os.CopyFS(evalsDir, os.DirFS(selftestEvalsDir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathWithoutKiroCLI(t))

	err := RunWithOptions("selftest", "", RunOptions{Backend: "stub", EvalsDir: evalsDir})
	if err != nil {
		t.Fatalf("stub run without kiro-cli on PATH failed: %v", err)
	}

	resultsDir := filepath.Join(evalsDir, "results")
	added := newRunDirs(t, resultsDir, map[string]bool{})
	if len(added) == 0 {
		t.Fatalf("no run directory appeared under %s", resultsDir)
	}
	assertSelfTestResults(t, readSelfTestResult(t, filepath.Join(resultsDir, added[0], "selftest.json")))
}

func TestSelfTestRejectsUnknownBackend(t *testing.T) {
	t.Cleanup(resetConfig)
	_, calls := installFakeKiroCLI(t, "")

	resultsDir := filepath.Join(selftestEvalsDir, "results")
	registerResultsCleanup(t, resultsDir)
	before := snapshotDirs(t, resultsDir)

	err := RunWithOptions("selftest", "", RunOptions{Backend: "nope", EvalsDir: selftestEvalsDir})
	if err == nil {
		t.Fatal("expected an error for unknown backend")
	}
	for _, want := range []string{`"nope"`, "kiro-cli", "stub"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
	if added := newRunDirs(t, resultsDir, before); len(added) != 0 {
		t.Errorf("rejected run still created result dirs: %v", added)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}

func TestSelfTestRejectsStubWithSandbox(t *testing.T) {
	t.Cleanup(resetConfig)
	_, calls := installFakeKiroCLI(t, "")

	resultsDir := filepath.Join(selftestEvalsDir, "results")
	registerResultsCleanup(t, resultsDir)
	before := snapshotDirs(t, resultsDir)

	err := RunWithOptions("selftest", "", RunOptions{Backend: "stub", Sandbox: true, EvalsDir: selftestEvalsDir})
	if err == nil {
		t.Fatal("expected stub+sandbox to be rejected")
	}
	if !strings.Contains(err.Error(), "sandbox") || !strings.Contains(err.Error(), "stub") {
		t.Errorf("error %q should mention stub and sandbox", err)
	}
	if added := newRunDirs(t, resultsDir, before); len(added) != 0 {
		t.Errorf("rejected run still created result dirs: %v", added)
	}
	if got := readCalls(t, calls); len(got) != 0 {
		t.Errorf("kiro-cli invoked: %q", got)
	}
}
