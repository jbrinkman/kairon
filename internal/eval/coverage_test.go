package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

const testSuiteHash = "1111111111111111111111111111111111111111111111111111111111111111"

func boolOf(t *testing.T, label string, p *bool) bool {
	t.Helper()
	if p == nil {
		t.Fatalf("%s = nil, want a recorded value", label)
	}
	return *p
}

func strPtr(s string) *string { return &s }

// covResults builds one result per (case, run) pair.
func covResults(runs map[string][]int) []CaseResult {
	var out []CaseResult
	for name, rs := range runs {
		for _, r := range rs {
			out = append(out, CaseResult{CaseName: name, Run: r})
		}
	}
	return out
}

func covCases(names ...string) []TestCase {
	var tcs []TestCase
	for _, n := range names {
		tcs = append(tcs, TestCase{Name: n})
	}
	return tcs
}

func TestCoverageCasesCovered(t *testing.T) {
	tests := []struct {
		name    string
		results []CaseResult
		cases   []TestCase
		repeat  int
		want    bool
	}{
		{"all cases single run", covResults(map[string][]int{"a": {1}, "b": {1}}), covCases("a", "b"), 1, true},
		{"one case missing", covResults(map[string][]int{"a": {1}}), covCases("a", "b"), 1, false},
		{"all cases all repeats", covResults(map[string][]int{"a": {1, 2}, "b": {1, 2}}), covCases("a", "b"), 2, true},
		{"one repeat missing", covResults(map[string][]int{"a": {1, 2}, "b": {1}}), covCases("a", "b"), 2, false},
		{"run 0 counts as run 1", covResults(map[string][]int{"a": {0}}), covCases("a"), 1, true},
		{"run 0 does not cover run 2", covResults(map[string][]int{"a": {0}}), covCases("a"), 2, false},
		{"run 0 plus run 2", covResults(map[string][]int{"a": {0, 2}}), covCases("a"), 2, true},
		{"zero cases is not covered", nil, nil, 1, false},
		{"zero cases with results is not covered", covResults(map[string][]int{"a": {1}}), nil, 1, false},
		{"extra results do not matter", covResults(map[string][]int{"a": {1}, "x": {1}}), covCases("a"), 1, true},
		{"zero repeat is one", covResults(map[string][]int{"a": {1}}), covCases("a"), 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := casesCovered(tt.results, tt.cases, tt.repeat); got != tt.want {
				t.Errorf("casesCovered = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoverageStampFullRun(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg = runConfig{suites: map[string]string{"selftest": testSuiteHash}}
	r := AgentResult{Agent: "selftest", Cases: covResults(map[string][]int{"a": {1}})}

	stampCoverage(&r, covCases("a"))

	if r.SuiteSHA256 != testSuiteHash {
		t.Errorf("SuiteSHA256 = %q, want %q", r.SuiteSHA256, testSuiteHash)
	}
	if !boolOf(t, "Complete", r.Complete) {
		t.Error("Complete = false, want true for a covered full run")
	}
	if r.CaseFilter == nil || *r.CaseFilter != "" {
		t.Errorf("CaseFilter = %v, want a recorded empty string", r.CaseFilter)
	}
}

func TestCoverageStampIncompleteRun(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg = runConfig{suites: map[string]string{"selftest": testSuiteHash}}
	r := AgentResult{Agent: "selftest", Cases: covResults(map[string][]int{"a": {1}})}

	stampCoverage(&r, covCases("a", "b"))

	if boolOf(t, "Complete", r.Complete) {
		t.Error("Complete = true with case b uncovered, want false")
	}
}

func TestCoverageStampCaseFilterIsNeverComplete(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg = runConfig{suites: map[string]string{"selftest": testSuiteHash}, caseFilter: "only"}
	// The agent has exactly one case and it is covered, yet a filtered run is
	// not a full run.
	r := AgentResult{Agent: "selftest", Cases: covResults(map[string][]int{"only": {1}})}

	stampCoverage(&r, covCases("only"))

	if boolOf(t, "Complete", r.Complete) {
		t.Error("Complete = true for a --case run, want false")
	}
	if r.CaseFilter == nil || *r.CaseFilter != "only" {
		t.Errorf("CaseFilter = %v, want only", r.CaseFilter)
	}
}

func TestCoverageStampNoOpWithoutSuiteEntry(t *testing.T) {
	t.Cleanup(resetConfig)
	for name, suites := range map[string]map[string]string{
		"nil map":      nil,
		"other agent":  {"someone-else": testSuiteHash},
		"empty entry?": {},
	} {
		t.Run(name, func(t *testing.T) {
			cfg = runConfig{suites: suites, caseFilter: "x"}
			r := AgentResult{Agent: "selftest", Cases: covResults(map[string][]int{"a": {1}})}

			stampCoverage(&r, covCases("a"))

			if r.SuiteSHA256 != "" || r.Complete != nil || r.CaseFilter != nil {
				t.Errorf("stampCoverage changed an unpinned result: %+v", r)
			}
		})
	}
}

func TestCoverageConfigureResetsRunScopedFields(t *testing.T) {
	t.Cleanup(resetConfig)
	cfg.suites = map[string]string{"a": testSuiteHash}
	cfg.caseFilter = "x"
	if err := configure(RunOptions{Backend: inference.NameStub}); err != nil {
		t.Fatal(err)
	}
	if cfg.suites != nil || cfg.caseFilter != "" {
		t.Errorf("configure left suites=%v caseFilter=%q", cfg.suites, cfg.caseFilter)
	}
}

func TestCoverageAgentResultJSON(t *testing.T) {
	t.Run("recorded false and empty are written", func(t *testing.T) {
		data, err := json.Marshal(AgentResult{Agent: "a", SuiteSHA256: testSuiteHash, Complete: boolPtr(false), CaseFilter: strPtr("")})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"suite_sha256":"` + testSuiteHash + `"`, `"complete":false`, `"case_filter":""`} {
			if !strings.Contains(string(data), want) {
				t.Errorf("JSON %s missing %s", data, want)
			}
		}
	})
	t.Run("not recorded is omitted", func(t *testing.T) {
		data, err := json.Marshal(AgentResult{Agent: "a"})
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"suite_sha256", "complete", "case_filter"} {
			if strings.Contains(string(data), key) {
				t.Errorf("JSON %s contains %s for an unrecorded result", data, key)
			}
		}
	})
	t.Run("legacy result decodes with nil Complete", func(t *testing.T) {
		var r AgentResult
		if err := json.Unmarshal([]byte(`{"agent":"a","git_hash":"g","cases":[]}`), &r); err != nil {
			t.Fatal(err)
		}
		if r.Complete != nil || r.CaseFilter != nil || r.SuiteSHA256 != "" {
			t.Errorf("legacy decode = %+v, want unrecorded coverage", r)
		}
	})
	t.Run("round trip keeps false and empty", func(t *testing.T) {
		data, _ := json.Marshal(AgentResult{Agent: "a", Complete: boolPtr(false), CaseFilter: strPtr("")})
		var r AgentResult
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		if r.Complete == nil || *r.Complete || r.CaseFilter == nil || *r.CaseFilter != "" {
			t.Errorf("round trip = %+v", r)
		}
	})
}

func TestCoverageSummaryJSON(t *testing.T) {
	t.Run("provenance writes false and empty", func(t *testing.T) {
		data, err := json.Marshal(Summary{Agents: map[string]AgentProvenance{
			"a": {PromptSHA256: "p", SuiteSHA256: testSuiteHash, Complete: boolPtr(false), CaseFilter: strPtr("")},
		}})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"suite_sha256":"` + testSuiteHash + `"`, `"complete":false`, `"case_filter":""`} {
			if !strings.Contains(string(data), want) {
				t.Errorf("JSON %s missing %s", data, want)
			}
		}
	})
	t.Run("provenance omits unrecorded", func(t *testing.T) {
		data, _ := json.Marshal(AgentProvenance{PromptSHA256: "p"})
		for _, key := range []string{"suite_sha256", "complete", "case_filter"} {
			if strings.Contains(string(data), key) {
				t.Errorf("JSON %s contains %s", data, key)
			}
		}
	})
	t.Run("legacy summary decodes with nil Complete", func(t *testing.T) {
		var s Summary
		legacy := `{"git_hash":"g","agent_scores":{},"agents":{"a":{"agent_model":"m","prompt_sha256":"p","resources_present":[]}}}`
		if err := json.Unmarshal([]byte(legacy), &s); err != nil {
			t.Fatal(err)
		}
		if p := s.Agents["a"]; p.Complete != nil || p.CaseFilter != nil || p.SuiteSHA256 != "" {
			t.Errorf("legacy provenance = %+v, want unrecorded coverage", p)
		}
	})
}

func TestCoverageUpdateIncrementalSummaryCopiesFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	res := AgentResult{Agent: "a", SuiteSHA256: testSuiteHash, Complete: boolPtr(false), CaseFilter: strPtr("")}
	// No PromptSHA256: the SuiteSHA256 alone must write the entry.
	if err := updateIncrementalSummary(file, res, "g"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"complete": false`, `"case_filter": ""`, `"suite_sha256": "` + testSuiteHash + `"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("summary missing %s:\n%s", want, raw)
		}
	}
	var s Summary
	readJSON(t, file, &s)
	p, ok := s.Agents["a"]
	if !ok {
		t.Fatalf("agents.a missing: %s", raw)
	}
	if p.SuiteSHA256 != testSuiteHash || p.Complete == nil || *p.Complete || p.CaseFilter == nil || *p.CaseFilter != "" {
		t.Errorf("agents.a = %+v", p)
	}
}

func TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.json")
	if err := updateIncrementalSummary(file, AgentResult{Agent: "a"}, "g"); err != nil {
		t.Fatal(err)
	}
	var s Summary
	readJSON(t, file, &s)
	if len(s.Agents) != 0 {
		t.Errorf("agents = %v, want none for an unpinned, unrecorded result", s.Agents)
	}
}

// covCase is a stub case whose shell command runs in the workspace.
func covCase(name string, commands ...string) TestCase {
	return TestCase{
		Name:  name,
		Input: "do it",
		Stub:  &inference.StubScript{Turns: []inference.StubTurn{{Response: "## H\n### S\ntext", Commands: commands}}},
	}
}

func TestCoverageEvaluateProgressiveStampsEverySave(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)
	cfg.suites = map[string]string{"selftest": testSuiteHash}

	dir := t.TempDir()
	// Cases 2 and 3 snapshot the agent file as it stood when they started,
	// i.e. the save made after the previous case.
	snap := func(n string) string {
		return "cp '" + filepath.Join(dir, "selftest.json") + "' '" + filepath.Join(dir, n) + "'"
	}
	cases := []TestCase{covCase("c1"), covCase("c2", snap("after1.json")), covCase("c3", snap("after2.json"))}

	var out strings.Builder
	res := evaluateProgressive(verdictRubric(), cases, "abc", &out, dir, false, nil)

	for _, name := range []string{"after1.json", "after2.json"} {
		var part AgentResult
		readJSON(t, filepath.Join(dir, name), &part)
		if part.SuiteSHA256 != testSuiteHash {
			t.Errorf("%s suite_sha256 = %q, want %q", name, part.SuiteSHA256, testSuiteHash)
		}
		if boolOf(t, name+" complete", part.Complete) {
			t.Errorf("%s (%d of 3 cases) complete = true, want false", name, len(part.Cases))
		}
	}

	if !boolOf(t, "returned Complete", res.Complete) {
		t.Error("returned result complete = false, want true for a finished run")
	}
	var saved AgentResult
	readJSON(t, filepath.Join(dir, "selftest.json"), &saved)
	if !boolOf(t, "saved Complete", saved.Complete) {
		t.Error("final selftest.json complete = false, want true")
	}
	var sum Summary
	readJSON(t, filepath.Join(dir, "summary.json"), &sum)
	if p := sum.Agents["selftest"]; p.SuiteSHA256 != testSuiteHash || p.Complete == nil || !*p.Complete || p.CaseFilter == nil || *p.CaseFilter != "" {
		t.Errorf("summary agents.selftest = %+v", p)
	}
}

func TestCoverageEvaluateProgressiveRepeat(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)
	cfg.suites = map[string]string{"selftest": testSuiteHash}
	cfg.repeat = 2

	res := evaluateProgressive(verdictRubric(), []TestCase{covCase("c1")}, "abc", &strings.Builder{}, t.TempDir(), false, nil)

	if len(res.Cases) != 2 {
		t.Fatalf("got %d results, want 2 (one per repeat)", len(res.Cases))
	}
	if !boolOf(t, "Complete", res.Complete) {
		t.Error("Complete = false with both repeats present, want true")
	}
}

func TestCoverageEvaluateProgressiveCaseFilterIncomplete(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)
	cfg.suites = map[string]string{"selftest": testSuiteHash}
	cfg.caseFilter = "c1"

	res := evaluateProgressive(verdictRubric(), []TestCase{covCase("c1")}, "abc", &strings.Builder{}, t.TempDir(), false, nil)

	if boolOf(t, "Complete", res.Complete) {
		t.Error("Complete = true for a filtered run, want false")
	}
	if res.CaseFilter == nil || *res.CaseFilter != "c1" {
		t.Errorf("CaseFilter = %v, want c1", res.CaseFilter)
	}
}

func TestCoverageEvaluateStampsSingleCaseRun(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)
	cfg.suites = map[string]string{"selftest": testSuiteHash}
	cfg.caseFilter = "only"

	res := evaluate(verdictRubric(), []TestCase{covCase("only")}, "abc", &strings.Builder{}, nil)

	if res.SuiteSHA256 != testSuiteHash {
		t.Errorf("SuiteSHA256 = %q", res.SuiteSHA256)
	}
	if boolOf(t, "Complete", res.Complete) {
		t.Error("Complete = true for a --case run whose agent has one case, want false")
	}
	if res.CaseFilter == nil || *res.CaseFilter != "only" {
		t.Errorf("CaseFilter = %v, want only", res.CaseFilter)
	}
}

func TestCoverageEvaluateUnpinnedLeavesFieldsUnset(t *testing.T) {
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	useStubBackend(t)

	for name, res := range map[string]AgentResult{
		"evaluate":            evaluate(verdictRubric(), []TestCase{covCase("c1")}, "abc", &strings.Builder{}, nil),
		"evaluateProgressive": evaluateProgressive(verdictRubric(), []TestCase{covCase("c1")}, "abc", &strings.Builder{}, t.TempDir(), false, nil),
	} {
		if res.SuiteSHA256 != "" || res.Complete != nil || res.CaseFilter != nil {
			t.Errorf("%s: unpinned result carries coverage: %+v", name, res)
		}
	}
}
