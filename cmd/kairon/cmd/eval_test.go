package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/eval"
	"github.com/jbrinkman/kairon/internal/inference"
)

func TestEvalFlagsRegistered(t *testing.T) {
	backend := evalCmd.Flags().Lookup("backend")
	if backend == nil {
		t.Fatal("--backend flag not registered on eval")
	}
	if backend.DefValue != inference.NameKiroCLI {
		t.Errorf("--backend default = %q, want %q", backend.DefValue, inference.NameKiroCLI)
	}
	for _, name := range inference.Names() {
		if !strings.Contains(backend.Usage, name) {
			t.Errorf("--backend usage %q does not list backend %q", backend.Usage, name)
		}
	}

	evalsDir := evalCmd.PersistentFlags().Lookup("evals-dir")
	if evalsDir == nil {
		t.Fatal("--evals-dir must be a persistent flag on eval")
	}
	if evalsDir.DefValue != "" {
		t.Errorf("--evals-dir default = %q, want empty (use package default)", evalsDir.DefValue)
	}
	// Persistent flags are inherited, so `eval diff` honours --evals-dir too.
	if diffCmd.InheritedFlags().Lookup("evals-dir") == nil {
		t.Error("diff subcommand does not inherit --evals-dir")
	}
}

func TestEvalUnknownBackendRejected(t *testing.T) {
	orig := evalBackend
	t.Cleanup(func() { evalBackend = orig })
	evalBackend = "nope"

	err := evalCmd.RunE(evalCmd, []string{"selftest"})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	for _, want := range []string{"nope", inference.NameKiroCLI, inference.NameStub} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestEvalUnknownBackendRejectedForPerfAndCleanup(t *testing.T) {
	origBackend, origPerf, origCleanup := evalBackend, evalPerf, evalCleanup
	t.Cleanup(func() { evalBackend, evalPerf, evalCleanup = origBackend, origPerf, origCleanup })
	evalBackend = "nope"

	// --perf and --cleanup go through RunWithOptions, so configuration (and
	// backend validation) applies before any work is done.
	for name, set := range map[string]func(){
		"perf":    func() { evalPerf, evalCleanup = true, false },
		"cleanup": func() { evalPerf, evalCleanup = false, true },
	} {
		set()
		if err := evalCmd.RunE(evalCmd, nil); err == nil || !strings.Contains(err.Error(), "unknown backend") {
			t.Errorf("--%s with bad backend: err = %v, want unknown backend error", name, err)
		}
	}
}

// evalSelfTestProject chdirs into a temp git repo whose .kairon/config.yaml is
// configYAML ("" for none) and returns a writable copy of the self-test
// fixtures. Eval flags are restored on cleanup.
func evalSelfTestProject(t *testing.T, configYAML string) (evalsDir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := filepath.Abs(filepath.Join("..", "..", "..", "internal", "eval", "testdata", "evals"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	origBackend, origEvals := evalBackend, evalEvalsDir
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		evalBackend, evalEvalsDir = origBackend, origEvals
	})

	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v: %s", err, out)
		}
	}
	if configYAML != "" {
		if err := os.MkdirAll(".kairon", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".kairon", "config.yaml"), []byte(configYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	evalsDir = filepath.Join(dir, "evals-copy")
	if err := os.CopyFS(evalsDir, os.DirFS(fixtures)); err != nil {
		t.Fatal(err)
	}
	// results/ is git-ignored run output (written by `task eval:selftest`);
	// a copy must not inherit stale runs from the developer's working tree.
	if err := os.RemoveAll(filepath.Join(evalsDir, "results")); err != nil {
		t.Fatal(err)
	}
	evalBackend, evalEvalsDir = "stub", evalsDir
	return evalsDir
}

func TestEvalRejectsUnpinnedJudgeModel(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "evals:\n  judge_model: auto\n")

	err := evalCmd.RunE(evalCmd, []string{"selftest"})
	if err == nil {
		t.Fatal("expected the run to be refused for judge_model auto")
	}
	for _, want := range []string{"auto", "allowed_models"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
		t.Errorf("results dir exists after refusal (stat err = %v)", statErr)
	}
}

func TestEvalSelfTestSucceedsWithDefaults(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")

	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("self-test with default evals config failed: %v", err)
	}
	runs, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*", "summary.json"))
	if len(runs) != 1 {
		t.Fatalf("summary.json files = %v, want 1", runs)
	}
}

// A failed verdict is a result, not a usage error: the eval command must
// silence cobra's usage dump (this is set when RunE starts).
func TestEvalSilencesUsageOnRunError(t *testing.T) {
	orig := evalCmd.SilenceUsage
	t.Cleanup(func() { evalCmd.SilenceUsage = orig })
	evalCmd.SilenceUsage = false

	origBackend := evalBackend
	t.Cleanup(func() { evalBackend = origBackend })
	evalBackend = "nope"

	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err == nil {
		t.Fatal("expected an error")
	}
	if !evalCmd.SilenceUsage {
		t.Error("eval RunE did not set SilenceUsage")
	}
}

func TestEvalSelfTestFailExitsWithThresholdError(t *testing.T) {
	evalSelfTestProject(t, "")
	orig := evalCmd.SilenceUsage
	t.Cleanup(func() { evalCmd.SilenceUsage = orig })
	evalCmd.SilenceUsage = false

	err := evalCmd.RunE(evalCmd, []string{"selftest-fail"})
	if !errors.Is(err, eval.ErrThresholdFailed) {
		t.Fatalf("err = %v, want eval.ErrThresholdFailed", err)
	}
	if !evalCmd.SilenceUsage {
		t.Error("usage would be printed for a threshold failure")
	}
}

func TestEvalPromptFileFlagRegistered(t *testing.T) {
	f := evalCmd.Flags().Lookup("prompt-file")
	if f == nil {
		t.Fatal("--prompt-file flag not registered on eval")
	}
	if f.DefValue != "" {
		t.Errorf("--prompt-file default = %q, want empty", f.DefValue)
	}
	if !strings.Contains(strings.ToLower(f.Usage), "candidate") {
		t.Errorf("--prompt-file usage %q does not describe the candidate prompt", f.Usage)
	}
	// Local to eval: it must not be a persistent flag, so diff does not inherit it.
	if evalCmd.PersistentFlags().Lookup("prompt-file") != nil {
		t.Error("--prompt-file must be a local flag on eval, not persistent")
	}
	if diffCmd.InheritedFlags().Lookup("prompt-file") != nil {
		t.Error("diff subcommand must not inherit --prompt-file")
	}
}

func TestEvalPromptFileWithoutAgentRejected(t *testing.T) {
	orig := evalPromptFile
	t.Cleanup(func() { evalPromptFile = orig })
	evalPromptFile = "x.md"

	err := evalCmd.RunE(evalCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "agent is required") {
		t.Fatalf("err = %v, want error containing 'agent is required'", err)
	}
}

func TestEvalPromptFileMissingRejected(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")
	orig := evalPromptFile
	t.Cleanup(func() { evalPromptFile = orig })
	evalPromptFile = filepath.Join(t.TempDir(), "missing.md")

	err := evalCmd.RunE(evalCmd, []string{"selftest"})
	if err == nil {
		t.Fatal("expected error for missing candidate file")
	}
	if !strings.Contains(err.Error(), "missing.md") {
		t.Errorf("error %q does not name the candidate path", err)
	}
	if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
		t.Errorf("results dir exists after rejection (stat err = %v)", statErr)
	}
}

func TestEvalPromptFileRecorded(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")
	orig := evalPromptFile
	t.Cleanup(func() { evalPromptFile = orig })

	readSummary := func() map[string]any {
		t.Helper()
		matches, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*", "summary.json"))
		if len(matches) != 1 {
			t.Fatalf("summary.json files = %v, want 1", matches)
		}
		data, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		var s map[string]any
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		// Clear so the next run starts from a clean results dir.
		if err := os.RemoveAll(filepath.Join(evalsDir, "results")); err != nil {
			t.Fatal(err)
		}
		return s
	}
	selftestAgent := func(s map[string]any) map[string]any {
		t.Helper()
		agents, _ := s["agents"].(map[string]any)
		a, _ := agents["selftest"].(map[string]any)
		if a == nil {
			t.Fatalf("summary has no agents.selftest: %v", s)
		}
		return a
	}

	// Baseline: no candidate.
	evalPromptFile = ""
	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("baseline run: %v", err)
	}
	base := readSummary()
	baseAgent := selftestAgent(base)
	if _, ok := baseAgent["prompt_file"]; ok {
		t.Errorf("baseline agents.selftest has prompt_file: %v", baseAgent["prompt_file"])
	}
	baseSHA, _ := baseAgent["prompt_sha256"].(string)
	if baseSHA == "" {
		t.Fatal("baseline run did not record prompt_sha256")
	}

	// Candidate run.
	cand := filepath.Join(t.TempDir(), "cand.md")
	if err := os.WriteFile(cand, []byte("# Candidate\n\nAnswer concisely.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evalPromptFile = cand
	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("candidate run: %v", err)
	}
	got := readSummary()
	want := filepath.ToSlash(filepath.Clean(cand))
	if got["prompt_file"] != want {
		t.Errorf("summary.prompt_file = %v, want %q", got["prompt_file"], want)
	}
	candAgent := selftestAgent(got)
	if candAgent["prompt_file"] != want {
		t.Errorf("agents.selftest.prompt_file = %v, want %q", candAgent["prompt_file"], want)
	}
	candSHA, _ := candAgent["prompt_sha256"].(string)
	if candSHA == "" || candSHA == baseSHA {
		t.Errorf("candidate prompt_sha256 = %q, want non-empty and different from baseline %q", candSHA, baseSHA)
	}
}

func TestEvalRepeatFlagRegistered(t *testing.T) {
	f := evalCmd.Flags().Lookup("repeat")
	if f == nil {
		t.Fatal("--repeat flag not registered on eval")
	}
	if f.DefValue != "1" {
		t.Errorf("--repeat default = %q, want %q", f.DefValue, "1")
	}
	if f.Value.Type() != "int" {
		t.Errorf("--repeat type = %q, want int", f.Value.Type())
	}
	// Local to eval: diff must not inherit it.
	if diffCmd.InheritedFlags().Lookup("repeat") != nil {
		t.Error("diff subcommand must not inherit --repeat")
	}
	if !strings.Contains(evalCmd.UsageString(), "--repeat") {
		t.Error("eval usage (--help) does not list --repeat")
	}
}

// --repeat below 1 is refused before any work: the error names the flag, the
// usage is still shown (SilenceUsage not yet set), no results directory is
// created and the backend is never reached. The backend is deliberately set to
// an unknown name: if RunE got as far as configuring the backend, the error
// would be "unknown backend" instead of the --repeat message.
func TestEvalRepeatBelowOneRejected(t *testing.T) {
	for _, n := range []int{0, -1} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			evalsDir := evalSelfTestProject(t, "")
			origRepeat, origSilence := evalRepeat, evalCmd.SilenceUsage
			t.Cleanup(func() { evalRepeat, evalCmd.SilenceUsage = origRepeat, origSilence })
			evalRepeat = n
			evalCmd.SilenceUsage = false
			evalBackend = "nope" // restored by evalSelfTestProject's cleanup

			err := evalCmd.RunE(evalCmd, []string{"selftest"})
			if err == nil {
				t.Fatalf("--repeat %d: expected an error", n)
			}
			if !strings.Contains(err.Error(), "--repeat") {
				t.Errorf("error %q does not name --repeat", err)
			}
			if strings.Contains(err.Error(), "unknown backend") {
				t.Errorf("error %q: the backend was reached before --repeat was validated", err)
			}
			if evalCmd.SilenceUsage {
				t.Error("usage must still be shown for an argument mistake (SilenceUsage was set)")
			}
			if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
				t.Errorf("results dir exists after rejection (stat err = %v)", statErr)
			}
		})
	}
}

// With the stub backend, --repeat 3 reaches RunOptions.Repeat: selftest.json
// has three results per case and summary.json records repeat 3.
func TestEvalRepeatPassedToRun(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")
	origRepeat := evalRepeat
	t.Cleanup(func() { evalRepeat = origRepeat })
	evalRepeat = 3

	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("selftest --repeat 3: %v", err)
	}
	runs, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*"))
	if len(runs) != 1 {
		t.Fatalf("run directories = %v, want 1", runs)
	}

	var agentRes struct {
		Repeat int `json:"repeat"`
		Cases  []struct {
			CaseName string `json:"case_name"`
			Run      int    `json:"run"`
		} `json:"cases"`
	}
	readJSON(t, filepath.Join(runs[0], "selftest.json"), &agentRes)
	if agentRes.Repeat != 3 {
		t.Errorf("selftest.json repeat = %d, want 3", agentRes.Repeat)
	}
	if len(agentRes.Cases) == 0 {
		t.Fatal("selftest.json has no cases")
	}
	perCase := map[string]int{}
	for _, c := range agentRes.Cases {
		perCase[c.CaseName]++
	}
	for name, n := range perCase {
		if n != 3 {
			t.Errorf("case %q has %d results, want 3", name, n)
		}
	}
	if len(agentRes.Cases) != 3*len(perCase) {
		t.Errorf("results = %d, want 3 × %d cases", len(agentRes.Cases), len(perCase))
	}

	var summary struct {
		AgentVerdicts map[string]struct {
			Repeat int `json:"repeat"`
		} `json:"agent_verdicts"`
	}
	readJSON(t, filepath.Join(runs[0], "summary.json"), &summary)
	if got := summary.AgentVerdicts["selftest"].Repeat; got != 3 {
		t.Errorf("summary.json agent_verdicts.selftest.repeat = %d, want 3", got)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestEvalFilterAndStrictFlagsRegistered(t *testing.T) {
	filter := evalCmd.PersistentFlags().Lookup("filter")
	if filter == nil {
		t.Fatal("--filter must be a persistent flag on eval")
	}
	if filter.Value.Type() != "stringArray" {
		t.Errorf("--filter type = %q, want stringArray (stringSlice would split on commas)", filter.Value.Type())
	}
	// Persistent: diff and trend inherit it from the one declaration.
	if diffCmd.InheritedFlags().Lookup("filter") == nil {
		t.Error("diff subcommand does not inherit --filter")
	}
	if trendCmd.InheritedFlags().Lookup("filter") == nil {
		t.Error("trend subcommand does not inherit --filter")
	}

	strict := evalCmd.Flags().Lookup("strict")
	if strict == nil {
		t.Fatal("--strict flag not registered on eval")
	}
	if strict.Value.Type() != "bool" || strict.DefValue != "false" {
		t.Errorf("--strict = type %q default %q, want bool false", strict.Value.Type(), strict.DefValue)
	}
	if evalCmd.PersistentFlags().Lookup("strict") != nil {
		t.Error("--strict must be local to eval, not persistent")
	}
	if diffCmd.InheritedFlags().Lookup("strict") != nil {
		t.Error("diff subcommand must not inherit --strict")
	}
	if trendCmd.InheritedFlags().Lookup("strict") != nil {
		t.Error("trend subcommand must not inherit --strict")
	}

	usage := evalCmd.UsageString()
	for _, name := range []string{"--filter", "--strict"} {
		if !strings.Contains(usage, name) {
			t.Errorf("eval usage (--help) does not list %s", name)
		}
	}
}

// Repeating --filter collects every value; a value containing commas stays a
// single value.
func TestEvalFilterFlagRepeatedAndCommas(t *testing.T) {
	flag := evalCmd.PersistentFlags().Lookup("filter")
	if flag == nil {
		t.Fatal("--filter flag not registered on eval")
	}
	orig := evalFilter
	t.Cleanup(func() { evalFilter = orig })
	evalFilter = nil
	t.Cleanup(func() { _ = flag.Value.(interface{ Replace([]string) error }).Replace(nil) })

	for _, v := range []string{"speed==fast,medium", "skill==cli"} {
		if err := evalCmd.PersistentFlags().Set("filter", v); err != nil {
			t.Fatalf("set --filter %q: %v", v, err)
		}
	}
	want := []string{"speed==fast,medium", "skill==cli"}
	if len(evalFilter) != len(want) {
		t.Fatalf("evalFilter = %q, want %q", evalFilter, want)
	}
	for i := range want {
		if evalFilter[i] != want[i] {
			t.Errorf("evalFilter[%d] = %q, want %q", i, evalFilter[i], want[i])
		}
	}
}

// A malformed --filter is refused before any work, with or without --strict:
// the usage is still shown (SilenceUsage not yet set) and the unknown backend
// is never reached.
func TestEvalMalformedFilterRejectedBeforeWork(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run("strict="+strconv.FormatBool(strict), func(t *testing.T) {
			evalsDir := evalSelfTestProject(t, "")
			origFilter, origStrict, origSilence := evalFilter, evalStrict, evalCmd.SilenceUsage
			t.Cleanup(func() { evalFilter, evalStrict, evalCmd.SilenceUsage = origFilter, origStrict, origSilence })
			evalFilter = []string{"speed"} // no operator
			evalStrict = strict
			evalCmd.SilenceUsage = false
			evalBackend = "nope" // restored by evalSelfTestProject's cleanup

			err := evalCmd.RunE(evalCmd, []string{"selftest"})
			if err == nil {
				t.Fatal("expected an error for a malformed filter")
			}
			if strings.Contains(err.Error(), "unknown backend") {
				t.Errorf("error %q: the backend was reached before --filter was parsed", err)
			}
			if !strings.Contains(err.Error(), "speed") {
				t.Errorf("error %q does not mention the malformed expression", err)
			}
			if evalCmd.SilenceUsage {
				t.Error("usage must still be shown for a malformed filter (SilenceUsage was set)")
			}
			if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
				t.Errorf("results dir exists after rejection (stat err = %v)", statErr)
			}
		})
	}
}

// --filter and --strict reach eval.RunOptions: a filter that matches nothing
// is a warning without --strict and an error with it.
func TestEvalStrictNoMatch(t *testing.T) {
	for _, tc := range []struct {
		strict  bool
		wantErr bool
	}{{false, false}, {true, true}} {
		t.Run("strict="+strconv.FormatBool(tc.strict), func(t *testing.T) {
			evalsDir := evalSelfTestProject(t, "")
			origFilter, origStrict := evalFilter, evalStrict
			t.Cleanup(func() { evalFilter, evalStrict = origFilter, origStrict })
			evalFilter = []string{"name==nope"}
			evalStrict = tc.strict

			err := evalCmd.RunE(evalCmd, []string{"selftest"})
			if tc.wantErr {
				if err == nil {
					t.Fatal("--strict with no matching case: expected an error")
				}
				if !strings.Contains(err.Error(), "name==nope") {
					t.Errorf("error %q does not name the filter", err)
				}
			} else if err != nil {
				t.Fatalf("no matching case without --strict: err = %v, want nil", err)
			}
			if _, statErr := os.Stat(filepath.Join(evalsDir, "results")); !os.IsNotExist(statErr) {
				t.Errorf("results dir exists although no case matched (stat err = %v)", statErr)
			}
		})
	}
}

// A matching filter narrows the run to the selected case.
func TestEvalFilterNarrowsRun(t *testing.T) {
	evalsDir := evalSelfTestProject(t, "")
	origFilter := evalFilter
	t.Cleanup(func() { evalFilter = origFilter })
	evalFilter = []string{"name==stub-basic"}

	if err := evalCmd.RunE(evalCmd, []string{"selftest"}); err != nil {
		t.Fatalf("selftest --filter name==stub-basic: %v", err)
	}
	runs, _ := filepath.Glob(filepath.Join(evalsDir, "results", "*"))
	if len(runs) != 1 {
		t.Fatalf("run directories = %v, want 1", runs)
	}
	var agentRes struct {
		Cases []struct {
			CaseName string `json:"case_name"`
		} `json:"cases"`
	}
	readJSON(t, filepath.Join(runs[0], "selftest.json"), &agentRes)
	if len(agentRes.Cases) != 1 || agentRes.Cases[0].CaseName != "stub-basic" {
		t.Errorf("cases = %+v, want only stub-basic", agentRes.Cases)
	}
}

// diff receives the filters: a malformed one is refused before SilenceUsage,
// and a valid one is applied (the report carries a Filter: line).
func TestEvalDiffReceivesFilters(t *testing.T) {
	origFilter, origEvals, origSilence := evalFilter, evalEvalsDir, diffCmd.SilenceUsage
	t.Cleanup(func() { evalFilter, evalEvalsDir, diffCmd.SilenceUsage = origFilter, origEvals, origSilence })
	evalEvalsDir = trendFixtureEvalsDir
	runs := []string{"260701-100000-aaaaaaa", "260702-100000-bbbbbbb"}

	evalFilter = []string{"speed"}
	diffCmd.SilenceUsage = false
	err := diffCmd.RunE(diffCmd, runs)
	if err == nil {
		t.Fatal("diff with a malformed filter: expected an error")
	}
	if diffCmd.SilenceUsage {
		t.Error("usage must still be shown for a malformed filter (SilenceUsage was set)")
	}

	evalFilter = []string{"agent==selftest"}
	out := captureStdout(t, func() {
		if err := diffCmd.RunE(diffCmd, runs); err != nil {
			t.Errorf("diff with a valid filter: %v", err)
		}
	})
	if !strings.Contains(out, "Filter: agent==selftest") {
		t.Errorf("diff output has no Filter line:\n%s", out)
	}
}
