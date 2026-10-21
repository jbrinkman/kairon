package eval

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Tests for --filter / --strict in the run path: pre-flight selection, the
// per-agent skip, the recorded case_filter label, the changed-filter resume
// refusal, --list and the metadata copied into case results. They use the
// stub backend and small temp evals directories.

// fcase builds a checks-only case file body. meta is a YAML metadata block
// body (without the "metadata:" line), or "" for an untagged case.
func fcase(name, meta string) string {
	body := "name: " + name + "\ndescription: desc " + name + "\ninput: \"please do the thing called " + name + "\"\n" +
		"expected_output: out\nstub:\n  turns:\n    - response: \"## H\"\n" +
		"checks:\n  - criterion: clarity\n    type: output_contains\n    pattern: \"## H\"\n"
	if meta != "" {
		body += "metadata:\n"
		for _, line := range strings.Split(strings.TrimRight(meta, "\n"), "\n") {
			body += "  " + line + "\n"
		}
	}
	return body
}

// filterEnv is a temp git project with a checks-only evals dir.
type filterEnv struct {
	evalsDir string
}

// newFilterEnv builds the project and writes the given agents: agent name ->
// (case file name -> body).
func newFilterEnv(t *testing.T, agents map[string]map[string]string) filterEnv {
	t.Helper()
	env := newCandidateE2EEnv(t)
	root := filepath.Join(t.TempDir(), "evals")
	for agent, cases := range agents {
		writeChecksOnlyEvals(t, root, agent, true, cases)
		writeProvFile(t, filepath.Join(root, "agents", agent+".json"),
			`{"name":"`+agent+`","prompt":"file://./`+agent+`-prompt.md","model":"claude-sonnet-5.5"}`)
		writeProvFile(t, filepath.Join(root, "agents", agent+"-prompt.md"), agent+" prompt\n")
	}
	_ = env
	return filterEnv{evalsDir: root}
}

// oneAgent is the common fixture: agent "alpha" with two fast cases, one
// slow case and one untagged case.
func oneAgent(t *testing.T) filterEnv {
	t.Helper()
	return newFilterEnv(t, map[string]map[string]string{
		"alpha": {
			"a1.yaml": fcase("a1", "speed: fast\nskill: cli\n"),
			"a2.yaml": fcase("a2", "speed: slow\n"),
			"a3.yaml": fcase("a3", ""),
			"a4.yaml": fcase("a4", "speed: fast\n"),
		},
	})
}

func (e filterEnv) opts(o RunOptions) RunOptions {
	o.Backend = "stub"
	o.NoSandbox = true
	o.EvalsDir = e.evalsDir
	return o
}

func (e filterEnv) resultsDir() string { return filepath.Join(e.evalsDir, "results") }

// runDirs lists the result directories a run created, if any.
func (e filterEnv) runDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(e.resultsDir())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
	}
	return names
}

// lastRun reads the only result directory.
func (e filterEnv) onlyRun(t *testing.T) string {
	t.Helper()
	dirs := e.runDirs(t)
	if len(dirs) != 1 {
		t.Fatalf("results dirs = %v, want exactly one", dirs)
	}
	return filepath.Join(e.resultsDir(), dirs[0])
}

func caseNames(r AgentResult) []string {
	seen := map[string]bool{}
	var names []string
	for _, c := range r.Cases {
		if !seen[c.CaseName] {
			seen[c.CaseName] = true
			names = append(names, c.CaseName)
		}
	}
	sort.Strings(names)
	return names
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	func() {
		defer func() {
			os.Stderr = orig
			_ = w.Close()
		}()
		fn()
	}()
	return <-done
}

func mustParseFilters(t *testing.T, exprs ...string) []Filter {
	t.Helper()
	fs, err := ParseFilters(exprs)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

// ---- label ----

func TestCaseFilterLabel(t *testing.T) {
	fs := mustParseFilters(t, "speed==fast", "skill<>cli,go")
	for _, tc := range []struct {
		name     string
		testcase string
		filters  []Filter
		want     string
	}{
		{"nothing", "", nil, ""},
		{"case only is the bare case name", "add-flag", nil, "add-flag"},
		{"filters only", "", fs, "speed==fast AND skill<>cli,go"},
		{"case and filters", "add-flag", fs, "case=add-flag AND speed==fast AND skill<>cli,go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := caseFilterLabel(tc.testcase, tc.filters); got != tc.want {
				t.Errorf("caseFilterLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---- run selection ----

func TestRunFilterRunsOnlyMatchingCases(t *testing.T) {
	env := oneAgent(t)
	if err := RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"speed==fast"}})); err != nil {
		t.Fatalf("filtered run: %v", err)
	}
	dir := env.onlyRun(t)
	res := readSelfTestResult(t, filepath.Join(dir, "alpha.json"))
	if got, want := caseNames(res), []string{"a1", "a4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cases run = %v, want %v", got, want)
	}
}

func TestRunFilterOnAllAgentsWithoutAgentArgument(t *testing.T) {
	env := newFilterEnv(t, map[string]map[string]string{
		"alpha": {"a1.yaml": fcase("a1", "speed: fast\n"), "a2.yaml": fcase("a2", "speed: slow\n")},
		"beta":  {"b1.yaml": fcase("b1", "speed: fast\n")},
	})
	if err := RunWithOptions("", "", env.opts(RunOptions{Filters: []string{"speed==fast"}})); err != nil {
		t.Fatalf("filtered run: %v", err)
	}
	dir := env.onlyRun(t)
	if got, want := caseNames(readSelfTestResult(t, filepath.Join(dir, "alpha.json"))), []string{"a1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("alpha cases = %v, want %v", got, want)
	}
	if got, want := caseNames(readSelfTestResult(t, filepath.Join(dir, "beta.json"))), []string{"b1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("beta cases = %v, want %v", got, want)
	}
}

func TestRunFilterANDsAgentFilterAndMultipleFilters(t *testing.T) {
	t.Run("two filters", func(t *testing.T) {
		env := oneAgent(t)
		// a1 is speed=fast and skill=cli; a4 is fast only.
		if err := RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"speed==fast", "skill==cli"}})); err != nil {
			t.Fatalf("run: %v", err)
		}
		res := readSelfTestResult(t, filepath.Join(env.onlyRun(t), "alpha.json"))
		if got, want := caseNames(res), []string{"a1"}; !reflect.DeepEqual(got, want) {
			t.Errorf("cases run = %v, want %v", got, want)
		}
	})
	t.Run("agent key", func(t *testing.T) {
		// The agent built-in key narrows too: agent==beta matches nothing in alpha.
		env := oneAgent(t)
		var err error
		stderr := captureStderr(t, func() {
			err = RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"agent==beta"}}))
		})
		if err != nil {
			t.Fatalf("no-match run returned %v, want nil", err)
		}
		if !strings.Contains(stderr, "agent==beta") {
			t.Errorf("warning %q does not name the filter", stderr)
		}
	})
}

func TestRunCaseAndFilterCombineWithAND(t *testing.T) {
	t.Run("both match", func(t *testing.T) {
		env := oneAgent(t)
		// a1 is fast: --case a1 AND speed==fast runs a1.
		if err := RunWithOptions("alpha", "a1", env.opts(RunOptions{Filters: []string{"speed==fast"}})); err != nil {
			t.Fatalf("run: %v", err)
		}
		res := readSelfTestResult(t, filepath.Join(env.onlyRun(t), "alpha.json"))
		if got, want := caseNames(res), []string{"a1"}; !reflect.DeepEqual(got, want) {
			t.Errorf("cases run = %v, want %v", got, want)
		}
	})
	t.Run("filter excludes the case", func(t *testing.T) {
		// a2 is slow: --case a2 AND speed==fast selects nothing.
		env := oneAgent(t)
		var err error
		stderr := captureStderr(t, func() {
			err = RunWithOptions("alpha", "a2", env.opts(RunOptions{Filters: []string{"speed==fast"}}))
		})
		if err != nil {
			t.Fatalf("no-match run returned %v, want nil", err)
		}
		if !strings.Contains(stderr, "speed==fast") || !strings.Contains(stderr, "a2") {
			t.Errorf("warning %q should name the filter and the --case scope", stderr)
		}
		if dirs := env.runDirs(t); len(dirs) != 0 {
			t.Errorf("no-match run created results dirs %v", dirs)
		}
	})
}

func TestRunCaseMissingStillGivesNotFoundWithFilters(t *testing.T) {
	env := oneAgent(t)
	err := RunWithOptions("alpha", "does-not-exist", env.opts(RunOptions{Filters: []string{"speed==fast"}, Strict: true}))
	if err == nil {
		t.Fatal("want the not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want the existing 'not found' error", err)
	}
	if strings.Contains(err.Error(), "no eval cases match") {
		t.Errorf("error = %q is the filter message, want the not-found error", err)
	}
	if dirs := env.runDirs(t); len(dirs) != 0 {
		t.Errorf("results dirs created: %v", dirs)
	}
}

func TestRunNoMatchWarnsAndRunsNothing(t *testing.T) {
	env := oneAgent(t)
	var err error
	stderr := captureStderr(t, func() {
		err = RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"speed==warp", "skill<>cli"}}))
	})
	if err != nil {
		t.Fatalf("non-strict no-match returned %v, want nil", err)
	}
	for _, want := range []string{"No eval cases match", "speed==warp", "skill<>cli", "alpha"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("warning %q does not contain %q", stderr, want)
		}
	}
	if _, statErr := os.Stat(env.resultsDir()); !os.IsNotExist(statErr) {
		t.Errorf("results directory exists after a no-match run (stat err = %v)", statErr)
	}
}

func TestRunNoMatchStrictErrorsNamingEveryFilter(t *testing.T) {
	env := oneAgent(t)
	err := RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"speed==warp", "skill<>cli"}, Strict: true}))
	if err == nil {
		t.Fatal("strict no-match returned nil, want an error")
	}
	for _, want := range []string{"no eval cases match", "speed==warp", "skill<>cli", "--strict"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if _, statErr := os.Stat(env.resultsDir()); !os.IsNotExist(statErr) {
		t.Errorf("results directory exists after a strict no-match (stat err = %v)", statErr)
	}
}

func TestRunMalformedFilterRejectedBeforeAnyCase(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "not strict", true: "strict"}[strict], func(t *testing.T) {
			env := oneAgent(t)
			err := RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"speed==fast", "no-operator"}, Strict: strict}))
			if err == nil {
				t.Fatal("malformed filter accepted")
			}
			if !strings.Contains(err.Error(), `invalid --filter "no-operator"`) || !strings.Contains(err.Error(), "expected <key><operator><value>") {
				t.Errorf("error = %q, want the usage-style filter error", err)
			}
			if _, statErr := os.Stat(env.resultsDir()); !os.IsNotExist(statErr) {
				t.Errorf("results directory exists (stat err = %v)", statErr)
			}
		})
	}
}

func TestRunMalformedFilterBeatsNotFoundCase(t *testing.T) {
	env := oneAgent(t)
	err := RunWithOptions("alpha", "does-not-exist", env.opts(RunOptions{Filters: []string{"=x"}}))
	if err == nil || !strings.Contains(err.Error(), "invalid --filter") {
		t.Fatalf("error = %v, want the invalid --filter error", err)
	}
}

func TestRunFilterRejectedWithPerfAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts RunOptions
		flag string
	}{
		{"filter+perf", RunOptions{Filters: []string{"speed==fast"}, Perf: true}, "--perf"},
		{"filter+cleanup", RunOptions{Filters: []string{"speed==fast"}, Cleanup: true}, "--cleanup"},
		{"strict+perf", RunOptions{Strict: true, Perf: true}, "--perf"},
		{"strict+cleanup", RunOptions{Strict: true, Cleanup: true}, "--cleanup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := oneAgent(t)
			err := RunWithOptions("alpha", "", env.opts(tc.opts))
			if err == nil {
				t.Fatal("combination accepted, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.flag) {
				t.Errorf("error %q does not name %s", err, tc.flag)
			}
			if !strings.Contains(err.Error(), "--filter") && !strings.Contains(err.Error(), "--strict") {
				t.Errorf("error %q does not name the filter flag", err)
			}
			if dirs := env.runDirs(t); len(dirs) != 0 {
				t.Errorf("results dirs created: %v", dirs)
			}
		})
	}
}

func TestRunStrictWithoutFilterChangesNothing(t *testing.T) {
	env := oneAgent(t)
	if err := RunWithOptions("alpha", "", env.opts(RunOptions{Strict: true})); err != nil {
		t.Fatalf("--strict alone: %v", err)
	}
	res := readSelfTestResult(t, filepath.Join(env.onlyRun(t), "alpha.json"))
	if got, want := caseNames(res), []string{"a1", "a2", "a3", "a4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cases run = %v, want all %v", got, want)
	}
	if !boolOf(t, "Complete", res.Complete) {
		t.Error("complete = false for an unfiltered run with --strict")
	}
	if res.CaseFilter == nil || *res.CaseFilter != "" {
		t.Errorf("case_filter = %v, want an explicit empty string", res.CaseFilter)
	}
}

// ---- several agents ----

func threeAgents(t *testing.T) filterEnv {
	return newFilterEnv(t, map[string]map[string]string{
		"alpha": {"a1.yaml": fcase("a1", "speed: fast\n")},
		"beta":  {"b1.yaml": fcase("b1", "speed: slow\n")},
		"gamma": {"g1.yaml": fcase("g1", "speed: fast\n")},
	})
}

func TestRunFilterSkipsAgentsWithNoMatches(t *testing.T) {
	env := threeAgents(t)
	if err := RunWithOptions("", "", env.opts(RunOptions{Filters: []string{"speed==fast"}})); err != nil {
		t.Fatalf("run: %v", err)
	}
	dir := env.onlyRun(t)

	for _, agent := range []string{"alpha", "gamma"} {
		if _, err := os.Stat(filepath.Join(dir, agent+".json")); err != nil {
			t.Errorf("%s.json missing: %v", agent, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "beta.json")); !os.IsNotExist(err) {
		t.Errorf("beta.json exists for an agent with no matching case (stat err = %v)", err)
	}

	var sum struct {
		Agents map[string]json.RawMessage `json:"agents"`
		Scores map[string]float64         `json:"agent_scores"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &sum); err != nil {
		t.Fatal(err)
	}
	if _, ok := sum.Agents["beta"]; ok {
		t.Error("summary.json agents has an entry for the skipped agent beta")
	}
	if _, ok := sum.Scores["beta"]; ok {
		t.Error("summary.json agent_scores has an entry for the skipped agent beta")
	}
	for _, agent := range []string{"alpha", "gamma"} {
		if _, ok := sum.Agents[agent]; !ok {
			t.Errorf("summary.json agents lacks %s", agent)
		}
	}
	if _, ok := cfg.suites["beta"]; ok {
		t.Error("a suite hash was pinned for the skipped agent beta")
	}
	for _, agent := range []string{"alpha", "gamma"} {
		if cfg.suites[agent] == "" {
			t.Errorf("no suite hash pinned for %s", agent)
		}
	}
}

func TestRunNoMatchRuleAppliesOnlyToWholeSelection(t *testing.T) {
	t.Run("not strict", func(t *testing.T) {
		env := threeAgents(t)
		var err error
		stderr := captureStderr(t, func() {
			err = RunWithOptions("", "", env.opts(RunOptions{Filters: []string{"speed==warp"}}))
		})
		if err != nil {
			t.Fatalf("non-strict empty selection returned %v, want nil", err)
		}
		if !strings.Contains(stderr, "speed==warp") {
			t.Errorf("warning %q does not name the filter", stderr)
		}
		if _, statErr := os.Stat(env.resultsDir()); !os.IsNotExist(statErr) {
			t.Errorf("results directory exists (stat err = %v)", statErr)
		}
	})
	t.Run("strict", func(t *testing.T) {
		env := threeAgents(t)
		err := RunWithOptions("", "", env.opts(RunOptions{Filters: []string{"speed==warp"}, Strict: true}))
		if err == nil || !strings.Contains(err.Error(), "speed==warp") {
			t.Fatalf("strict empty selection error = %v, want one naming the filter", err)
		}
	})
}

func TestRunSkippedAgentModelDoesNotBlockFilteredRun(t *testing.T) {
	// Only agents that will run are pinned, so narrowing must reach
	// agentsInScope.
	cfg.scopeAgents = []string{"alpha"}
	t.Cleanup(resetConfig)
	got := agentsInScope("", RunOptions{})
	if !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Errorf("agentsInScope with scopeAgents = %v, want [alpha]", got)
	}
	got = agentsInScope("zeta", RunOptions{})
	if !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Errorf("agentsInScope(zeta) with scopeAgents = %v, want scopeAgents first", got)
	}
	resetConfig()
	if got := agentsInScope("zeta", RunOptions{}); !reflect.DeepEqual(got, []string{"zeta"}) {
		t.Errorf("agentsInScope(zeta) without scopeAgents = %v, want [zeta]", got)
	}
}

func TestConfigureResetsFilterState(t *testing.T) {
	cfg.filters = mustParseFilters(t, "a==b")
	cfg.scopeAgents = []string{"x"}
	t.Cleanup(resetConfig)
	if err := configure(RunOptions{Backend: "stub"}); err != nil {
		t.Fatal(err)
	}
	if cfg.filters != nil || cfg.scopeAgents != nil {
		t.Errorf("configure left filters=%v scopeAgents=%v", cfg.filters, cfg.scopeAgents)
	}
}

// ---- selectForRun ----

func TestSelectForRun(t *testing.T) {
	env := oneAgent(t)
	if err := configure(env.opts(RunOptions{})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	opts := RunOptions{}

	t.Run("no filters proceeds silently", func(t *testing.T) {
		var buf bytes.Buffer
		ok, err := selectForRun(&buf, "alpha", "", nil, true, opts)
		if !ok || err != nil || buf.Len() != 0 {
			t.Errorf("= (%v, %v, %q), want (true, nil, empty)", ok, err, buf.String())
		}
	})
	t.Run("match narrows the scope", func(t *testing.T) {
		var buf bytes.Buffer
		ok, err := selectForRun(&buf, "", "", mustParseFilters(t, "speed==fast"), false, opts)
		if !ok || err != nil {
			t.Fatalf("= (%v, %v), want (true, nil)", ok, err)
		}
		if !reflect.DeepEqual(cfg.scopeAgents, []string{"alpha"}) {
			t.Errorf("scopeAgents = %v, want [alpha]", cfg.scopeAgents)
		}
	})
	t.Run("non-strict no match warns", func(t *testing.T) {
		var buf bytes.Buffer
		ok, err := selectForRun(&buf, "alpha", "", mustParseFilters(t, "speed==warp", "skill==x"), false, opts)
		if ok || err != nil {
			t.Fatalf("= (%v, %v), want (false, nil)", ok, err)
		}
		for _, want := range []string{"⚠️", "speed==warp AND skill==x", "agent: alpha"} {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("warning %q lacks %q", buf.String(), want)
			}
		}
	})
	t.Run("strict no match errors", func(t *testing.T) {
		var buf bytes.Buffer
		ok, err := selectForRun(&buf, "alpha", "a1", mustParseFilters(t, "speed==slow"), true, opts)
		if ok || err == nil {
			t.Fatalf("= (%v, %v), want (false, error)", ok, err)
		}
		for _, want := range []string{"❌ no eval cases match --filter speed==slow", "(--strict)", "case: a1"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lacks %q", err, want)
			}
		}
		if buf.Len() != 0 {
			t.Errorf("strict wrote %q to the warning writer", buf.String())
		}
	})
	t.Run("missing case defers to the not-found error", func(t *testing.T) {
		var buf bytes.Buffer
		ok, err := selectForRun(&buf, "alpha", "nope", mustParseFilters(t, "speed==fast"), true, opts)
		if !ok || err != nil || buf.Len() != 0 {
			t.Errorf("= (%v, %v, %q), want (true, nil, empty)", ok, err, buf.String())
		}
	})
}

func TestSelectForRunLeavesUnloadableAgentToTheRun(t *testing.T) {
	env := newFilterEnv(t, map[string]map[string]string{
		"alpha": {"a1.yaml": fcase("a1", "speed: fast\n")},
		"beta":  {"bad.yaml": "name: bad\nunknown_field: 1\n"},
	})
	if err := configure(env.opts(RunOptions{})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	var buf bytes.Buffer
	ok, err := selectForRun(&buf, "", "", mustParseFilters(t, "speed==fast"), true, RunOptions{})
	if !ok || err != nil {
		t.Fatalf("= (%v, %v), want (true, nil): a load error is reported later by the run", ok, err)
	}

	// The run itself still fails on the malformed case file.
	err = RunWithOptions("", "", env.opts(RunOptions{Filters: []string{"speed==fast"}}))
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Fatalf("run error = %v, want the malformed case error naming bad.yaml", err)
	}
}

// ---- recorded label ----

func TestFilteredRunRecordsIncompleteAndLabel(t *testing.T) {
	env := oneAgent(t)
	if err := RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"speed==fast", "skill==cli"}})); err != nil {
		t.Fatal(err)
	}
	dir := env.onlyRun(t)
	want := "speed==fast AND skill==cli"
	for label, m := range map[string]map[string]json.RawMessage{
		"alpha.json":   readRawJSON(t, filepath.Join(dir, "alpha.json")),
		"summary.json": summaryAgent(t, dir, "alpha"),
	} {
		if got := rawString(t, label, m, "case_filter"); got != want {
			t.Errorf("%s case_filter = %q, want %q", label, got, want)
		}
		if rawBool(t, label, m, "complete") {
			t.Errorf("%s complete = true for a filtered run", label)
		}
	}
}

func summaryAgent(t *testing.T, dir, agent string) map[string]json.RawMessage {
	t.Helper()
	sum := readRawJSON(t, filepath.Join(dir, "summary.json"))
	var agents map[string]map[string]json.RawMessage
	if err := json.Unmarshal(sum["agents"], &agents); err != nil {
		t.Fatalf("summary agents: %v", err)
	}
	return agents[agent]
}

func TestCaseAndFilterRunRecordsCombinedLabel(t *testing.T) {
	env := oneAgent(t)
	if err := RunWithOptions("alpha", "a1", env.opts(RunOptions{Filters: []string{"speed==fast"}})); err != nil {
		t.Fatal(err)
	}
	res := readSelfTestResult(t, filepath.Join(env.onlyRun(t), "alpha.json"))
	if res.CaseFilter == nil || *res.CaseFilter != "case=a1 AND speed==fast" {
		t.Errorf("case_filter = %v, want %q", res.CaseFilter, "case=a1 AND speed==fast")
	}
	if boolOf(t, "Complete", res.Complete) {
		t.Error("complete = true for a filtered --case run")
	}
}

func TestUnfilteredAndCaseRunsRecordAsBefore(t *testing.T) {
	t.Run("unfiltered", func(t *testing.T) {
		env := oneAgent(t)
		if err := RunWithOptions("alpha", "", env.opts(RunOptions{})); err != nil {
			t.Fatal(err)
		}
		res := readSelfTestResult(t, filepath.Join(env.onlyRun(t), "alpha.json"))
		if !boolOf(t, "Complete", res.Complete) || res.CaseFilter == nil || *res.CaseFilter != "" {
			t.Errorf("complete=%v case_filter=%v, want true and \"\"", res.Complete, res.CaseFilter)
		}
	})
	t.Run("--case", func(t *testing.T) {
		env := oneAgent(t)
		if err := RunWithOptions("alpha", "a2", env.opts(RunOptions{})); err != nil {
			t.Fatal(err)
		}
		res := readSelfTestResult(t, filepath.Join(env.onlyRun(t), "alpha.json"))
		if boolOf(t, "Complete", res.Complete) || res.CaseFilter == nil || *res.CaseFilter != "a2" {
			t.Errorf("complete=%v case_filter=%v, want false and \"a2\"", res.Complete, res.CaseFilter)
		}
	})
}

func TestRunSingleTestCaseGuardSkipsNonMatchingCase(t *testing.T) {
	env := oneAgent(t)
	if err := configure(env.opts(RunOptions{})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	cfg.filters = mustParseFilters(t, "speed==fast")

	var err error
	stderr := captureStderr(t, func() {
		err = runSingleTestCase("alpha", "a2", nil)
	})
	if err != nil {
		t.Fatalf("guard returned %v, want nil", err)
	}
	if !strings.Contains(stderr, "speed==fast") {
		t.Errorf("guard message %q does not name the filter", stderr)
	}
	if _, statErr := os.Stat(env.resultsDir()); !os.IsNotExist(statErr) {
		t.Errorf("results directory created by the guarded call (stat err = %v)", statErr)
	}
}

// ---- metadata in results ----

func TestResultsRecordMetadataForEveryRepeat(t *testing.T) {
	env := oneAgent(t)
	if err := RunWithOptions("alpha", "", env.opts(RunOptions{Repeat: 2})); err != nil {
		t.Fatal(err)
	}
	dir := env.onlyRun(t)
	res := readSelfTestResult(t, filepath.Join(dir, "alpha.json"))

	wantMeta := map[string]CaseMetadata{
		"a1": {"speed": "fast", "skill": "cli"},
		"a2": {"speed": "slow"},
		"a3": nil,
		"a4": {"speed": "fast"},
	}
	runs := map[string]int{}
	for _, c := range res.Cases {
		runs[c.CaseName]++
		if !reflect.DeepEqual(c.Metadata, wantMeta[c.CaseName]) && !(len(c.Metadata) == 0 && len(wantMeta[c.CaseName]) == 0) {
			t.Errorf("%s run %d metadata = %v, want %v", c.CaseName, c.Run, c.Metadata, wantMeta[c.CaseName])
		}
	}
	for name, n := range runs {
		if n != 2 {
			t.Errorf("case %s has %d results, want 2", name, n)
		}
	}

	// Raw JSON: tagged cases carry "metadata"; untagged ones omit the key.
	var raw struct {
		Cases []map[string]json.RawMessage `json:"cases"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "alpha.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, c := range raw.Cases {
		var name string
		_ = json.Unmarshal(c["case_name"], &name)
		_, has := c["metadata"]
		if name == "a3" && has {
			t.Errorf("untagged case a3 has a metadata key in the JSON")
		}
		if name != "a3" && !has {
			t.Errorf("tagged case %s has no metadata key in the JSON", name)
		}
	}
}

func TestStampCaseCopiesMetadata(t *testing.T) {
	tc := TestCase{Name: "c", Metadata: CaseMetadata{"speed": "fast"}}
	var cr CaseResult
	stampCase(&cr, tc, Rubric{})
	if !reflect.DeepEqual(cr.Metadata, CaseMetadata{"speed": "fast"}) {
		t.Fatalf("Metadata = %v, want speed=fast", cr.Metadata)
	}
	tc.Metadata["speed"] = "slow"
	if cr.Metadata["speed"] != "fast" {
		t.Error("CaseResult.Metadata aliases the case's map")
	}

	var plain CaseResult
	stampCase(&plain, TestCase{Name: "p"}, Rubric{})
	if plain.Metadata != nil {
		t.Errorf("untagged Metadata = %v, want nil", plain.Metadata)
	}
}

func TestStampResumedCasesKeepsRecordedMetadata(t *testing.T) {
	res := AgentResult{Cases: []CaseResult{{CaseName: "c"}}} // pre-verdict result: Passed == nil, no metadata
	stampResumedCases(&res, []TestCase{{Name: "c", Metadata: CaseMetadata{"speed": "fast"}}}, Rubric{})
	if res.Cases[0].Metadata != nil {
		t.Errorf("resume back-filled metadata %v onto a saved result", res.Cases[0].Metadata)
	}
	if res.Cases[0].Passed == nil {
		t.Error("resumed case was not stamped")
	}
}

// ---- resume ----

func filteredRunThenInterrupt(t *testing.T, env filterEnv, filter string) string {
	t.Helper()
	if err := RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{filter}})); err != nil {
		t.Fatal(err)
	}
	dir := env.onlyRun(t)
	markInterrupted(t, dir)
	return dir
}

func TestResumeSameFilterContinuesAndAppliesFilter(t *testing.T) {
	env := oneAgent(t)
	dir := filteredRunThenInterrupt(t, env, "speed==fast")

	// Drop one saved case so the resume has work to do.
	agentFile := filepath.Join(dir, "alpha.json")
	res := readSelfTestResult(t, agentFile)
	res.Cases = res.Cases[:1]
	data, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(agentFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunWithOptions("alpha", "", env.opts(RunOptions{Filters: []string{"speed==fast"}, Resume: true}))
	if err != nil {
		t.Fatalf("resume with the same filter: %v", err)
	}
	after := readSelfTestResult(t, agentFile)
	if got, want := caseNames(after), []string{"a1", "a4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cases after resume = %v, want %v (filter applied, no non-matching case added)", got, want)
	}
	if after.CaseFilter == nil || *after.CaseFilter != "speed==fast" {
		t.Errorf("case_filter after resume = %v, want speed==fast", after.CaseFilter)
	}
}

func TestResumeRefusesChangedOrMissingFilter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filters []string
		now     string
	}{
		{"different filter", []string{"speed==slow"}, "speed==slow"},
		{"filter dropped", nil, ""},
		{"filter added to it", []string{"speed==fast", "skill==cli"}, "speed==fast AND skill==cli"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := oneAgent(t)
			dir := filteredRunThenInterrupt(t, env, "speed==fast")
			agentFile := filepath.Join(dir, "alpha.json")
			before, err := os.ReadFile(agentFile)
			if err != nil {
				t.Fatal(err)
			}

			err = RunWithOptions("alpha", "", env.opts(RunOptions{Filters: tc.filters, Resume: true}))
			if err == nil {
				t.Fatal("resume with a changed filter was accepted")
			}
			for _, want := range []string{"cannot resume", "--filter changed", "alpha", `recorded case_filter="speed==fast"`, `now "` + tc.now + `"`} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
			after, _ := os.ReadFile(agentFile)
			if !bytes.Equal(before, after) {
				t.Error("refused resume modified the saved result")
			}
		})
	}
}

func TestResumeLegacyResultWithoutCaseFilterResumesAsBefore(t *testing.T) {
	env := oneAgent(t)
	if err := RunWithOptions("alpha", "", env.opts(RunOptions{})); err != nil {
		t.Fatal(err)
	}
	dir := env.onlyRun(t)
	markInterrupted(t, dir)

	// Make the saved result look like one written before case_filter existed.
	for _, f := range []string{"alpha.json"} {
		raw := readRawJSON(t, filepath.Join(dir, f))
		delete(raw, "case_filter")
		data, _ := json.MarshalIndent(raw, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunWithOptions("alpha", "", env.opts(RunOptions{Resume: true})); err != nil {
		t.Fatalf("legacy resume without filters was refused: %v", err)
	}

	// With a filter now, the legacy result (recorded as unfiltered) is refused.
	markInterrupted(t, dir)
	err := RunWithOptions("alpha", "", env.opts(RunOptions{Resume: true, Filters: []string{"speed==fast"}}))
	if err == nil || !strings.Contains(err.Error(), "--filter changed") {
		t.Fatalf("resume of an unfiltered result with a filter: err = %v, want the changed-filter refusal", err)
	}
}

// ---- --list ----

func TestPrintTestCaseListMetadataLinesSortedAndUnchangedWhenUntagged(t *testing.T) {
	env := newFilterEnv(t, map[string]map[string]string{
		"alpha": {
			"a1.yaml": fcase("a1", "speed: fast\nskill: cli\nretries: 3\n"),
			"a2.yaml": fcase("a2", ""),
		},
	})
	if err := configure(env.opts(RunOptions{})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	var buf bytes.Buffer
	if err := printTestCaseList(&buf, "alpha", nil); err != nil {
		t.Fatal(err)
	}
	want := "📋 Test cases for agent 'alpha':\n" +
		"   • a1 - desc a1\n" +
		"       metadata: retries=3, skill=cli, speed=fast\n" +
		"   • a2 - desc a2\n"
	if buf.String() != want {
		t.Errorf("list output:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestPrintTestCaseListHonorsFilters(t *testing.T) {
	env := oneAgent(t)
	if err := configure(env.opts(RunOptions{})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)

	var buf bytes.Buffer
	if err := printTestCaseList(&buf, "alpha", mustParseFilters(t, "speed==fast", "skill==cli")); err != nil {
		t.Fatal(err)
	}
	want := "📋 Test cases for agent 'alpha' (filter: speed==fast AND skill==cli):\n" +
		"   • a1 - desc a1\n" +
		"       metadata: skill=cli, speed=fast\n"
	if buf.String() != want {
		t.Errorf("list output:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestListWithFilterThroughRunWithOptions(t *testing.T) {
	env := oneAgent(t)

	var err error
	out := captureStdout(t, func() {
		err = RunWithOptions("alpha", "", env.opts(RunOptions{List: true, Filters: []string{"speed==slow"}}))
	})
	if err != nil {
		t.Fatalf("--list --filter: %v", err)
	}
	if !strings.Contains(out, "a2") || strings.Contains(out, "a1") || strings.Contains(out, "a4") {
		t.Errorf("--list --filter speed==slow output:\n%s\nwant only a2", out)
	}

	// No match: warning on stderr, nil; strict: error.
	var stderr string
	stderr = captureStderr(t, func() {
		err = RunWithOptions("alpha", "", env.opts(RunOptions{List: true, Filters: []string{"speed==warp"}}))
	})
	if err != nil || !strings.Contains(stderr, "speed==warp") {
		t.Errorf("non-strict --list no match: err=%v stderr=%q", err, stderr)
	}
	err = RunWithOptions("alpha", "", env.opts(RunOptions{List: true, Filters: []string{"speed==warp"}, Strict: true}))
	if err == nil || !strings.Contains(err.Error(), "speed==warp") {
		t.Errorf("strict --list no match: err = %v", err)
	}
	if dirs := env.runDirs(t); len(dirs) != 0 {
		t.Errorf("--list created results dirs %v", dirs)
	}
}

func TestListWithoutAgentStillRequiresAgent(t *testing.T) {
	env := oneAgent(t)
	err := RunWithOptions("", "", env.opts(RunOptions{List: true, Filters: []string{"speed==warp"}}))
	if err == nil || !strings.Contains(err.Error(), "agent name required for --list") {
		t.Fatalf("err = %v, want the existing agent-required error", err)
	}
}
