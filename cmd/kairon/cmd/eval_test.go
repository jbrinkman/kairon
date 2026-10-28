package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/eval"
	"github.com/jbrinkman/kairon/internal/inference"
	"github.com/spf13/cobra"
)

// flagLookup selects which flag set of the owning command a flagCase reads.
type flagLookup int

const (
	// lookupFlags reads cmd.Flags(): local and persistent flags together.
	lookupFlags flagLookup = iota
	// lookupPersistent reads cmd.PersistentFlags().
	lookupPersistent
	// lookupLocal reads cmd.LocalFlags(): flags the command declares itself.
	lookupLocal
	// lookupInherited reads cmd.InheritedFlags(): persistent flags of a parent.
	//
	// Deviates from spec "Shared flag-table shape", which lists only Flags(),
	// PersistentFlags() and LocalFlags(): cobra merges a parent's persistent
	// flags into cmd.Flags() only as a side effect of another call, so a
	// Flags() lookup of an inherited flag passes or fails depending on which
	// rows ran first (audit:--evals-dir failed when run alone). The spec's
	// assertion for these rows is "InheritedFlags() has it"; this reads it
	// directly and is order-independent.
	lookupInherited
)

// flagCase is one row of a flag-registration table. Only the fields a row
// sets are checked. TestEvalFlagsRegistered and TestVerifyLogFlags share it so
// the package has one flag-row shape.
type flagCase struct {
	// name is the subtest name. It contains no spaces.
	name string
	// cmd resolves the command that owns the flag when the row runs, so a
	// command looked up through rootCmd.Find can fail the row itself.
	cmd func(t *testing.T) *cobra.Command
	// flag is the flag name without the leading dashes.
	flag string
	// lookup selects the flag set that must contain the flag.
	lookup flagLookup
	// def, when non-nil, is the expected DefValue. defHint is appended to the
	// failure message.
	def     *string
	defHint string
	// valueType, when set, is the expected Value.Type().
	valueType string
	// usage lists strings the flag's Usage must contain.
	usage []string
	// required asserts the flag carries cobra's required annotation.
	required bool
	// inherits lists commands whose InheritedFlags() must contain the flag.
	inherits []*cobra.Command
	// notInherited lists commands whose InheritedFlags() must not contain it.
	notInherited []*cobra.Command
	// notLocal asserts the owner's LocalFlags() does not contain the flag.
	notLocal bool
	// notPersistent asserts the owner's PersistentFlags() does not contain it.
	notPersistent bool
}

// ownedBy returns a flagCase.cmd resolver for a command held in a variable.
func ownedBy(c *cobra.Command) func(t *testing.T) *cobra.Command {
	return func(*testing.T) *cobra.Command { return c }
}

// strPtr returns a pointer to s, for flagCase.def.
func strPtr(s string) *string { return &s }

// runFlagCase runs one row as a subtest named tc.name.
func runFlagCase(t *testing.T, tc flagCase) {
	t.Helper()
	t.Run(tc.name, func(t *testing.T) {
		c := tc.cmd(t)
		if c == nil {
			t.Fatalf("no command owns --%s", tc.flag)
		}

		// Method values avoid naming the pflag type: every lookup has the
		// same signature.
		lookup := c.Flags().Lookup
		switch tc.lookup {
		case lookupPersistent:
			lookup = c.PersistentFlags().Lookup
		case lookupLocal:
			lookup = c.LocalFlags().Lookup
		case lookupInherited:
			lookup = c.InheritedFlags().Lookup
		}
		f := lookup(tc.flag)
		if f == nil {
			if tc.lookup == lookupPersistent {
				t.Fatalf("--%s must be a persistent flag on %s", tc.flag, c.Name())
			}
			if tc.lookup == lookupInherited {
				t.Fatalf("%s does not inherit --%s", c.Name(), tc.flag)
			}
			t.Fatalf("--%s flag not registered on %s", tc.flag, c.Name())
		}

		if tc.def != nil && f.DefValue != *tc.def {
			msg := fmt.Sprintf("--%s default = %q, want %q", tc.flag, f.DefValue, *tc.def)
			if tc.defHint != "" {
				msg += " (" + tc.defHint + ")"
			}
			t.Error(msg)
		}
		if tc.valueType != "" && f.Value.Type() != tc.valueType {
			t.Errorf("--%s type = %q, want %s", tc.flag, f.Value.Type(), tc.valueType)
		}
		for _, want := range tc.usage {
			if !strings.Contains(f.Usage, want) {
				// Same text as the old backend assertions; other flags drop the noun.
				if tc.flag == "backend" {
					t.Errorf("--%s usage %q does not list backend %q", tc.flag, f.Usage, want)
				} else {
					t.Errorf("--%s usage %q does not list %q", tc.flag, f.Usage, want)
				}
			}
		}
		if tc.required {
			if got := f.Annotations[cobraRequiredAnnotation]; len(got) != 1 || got[0] != "true" {
				t.Errorf("--%s must be required, annotations = %v", tc.flag, f.Annotations)
			}
		}
		if tc.notLocal && c.LocalFlags().Lookup(tc.flag) != nil {
			t.Errorf("--%s must not be defined locally on %s", tc.flag, c.Name())
		}
		if tc.notPersistent && c.PersistentFlags().Lookup(tc.flag) != nil {
			t.Errorf("--%s must be a local flag on %s, not persistent", tc.flag, c.Name())
		}
		for _, other := range tc.inherits {
			if other.InheritedFlags().Lookup(tc.flag) == nil {
				t.Errorf("%s subcommand does not inherit --%s", other.Name(), tc.flag)
			}
		}
		for _, other := range tc.notInherited {
			if other.InheritedFlags().Lookup(tc.flag) != nil {
				t.Errorf("%s subcommand must not inherit --%s", other.Name(), tc.flag)
			}
		}
	})
}

func TestEvalFlagsRegistered(t *testing.T) {
	cases := []flagCase{
		{
			name: "eval:--backend", cmd: ownedBy(evalCmd), flag: "backend",
			lookup: lookupFlags, def: strPtr(inference.NameKiroCLI), usage: inference.Names(),
		},
		{
			// Persistent flags are inherited, so `eval diff` honours --evals-dir too.
			name: "eval:--evals-dir", cmd: ownedBy(evalCmd), flag: "evals-dir",
			lookup: lookupPersistent, def: strPtr(""), defHint: "use package default",
			inherits: []*cobra.Command{diffCmd},
		},
		{
			// Local to eval: it must not be a persistent flag, so diff does not inherit it.
			name: "eval:--prompt-file", cmd: ownedBy(evalCmd), flag: "prompt-file",
			lookup: lookupFlags, notPersistent: true, notInherited: []*cobra.Command{diffCmd},
		},
		{
			// Local to eval: diff must not inherit it.
			name: "eval:--repeat", cmd: ownedBy(evalCmd), flag: "repeat",
			lookup: lookupFlags, def: strPtr("1"), notInherited: []*cobra.Command{diffCmd},
		},
		{
			// Persistent: diff and trend inherit it from the one declaration.
			name: "eval:--filter", cmd: ownedBy(evalCmd), flag: "filter",
			lookup: lookupPersistent, inherits: []*cobra.Command{diffCmd, trendCmd},
		},
		{
			name: "eval:--strict", cmd: ownedBy(evalCmd), flag: "strict",
			lookup: lookupFlags, valueType: "bool", def: strPtr("false"), notPersistent: true,
			notInherited: []*cobra.Command{diffCmd, trendCmd},
		},
		{
			name: "check:--checks", cmd: findCheckCmd, flag: "checks",
			lookup: lookupFlags, required: true,
		},
		{
			name: "check:--dir", cmd: findCheckCmd, flag: "dir",
			lookup: lookupFlags, required: true,
		},
		{
			name: "check:--backend", cmd: findCheckCmd, flag: "backend",
			lookup: lookupFlags, def: strPtr(inference.NameKiroCLI), usage: inference.Names(),
		},
		{
			name: "audit:--iterations-dir", cmd: ownedBy(auditCmd), flag: "iterations-dir",
			lookup: lookupFlags, def: strPtr(".kairon/iterations"),
		},
		{
			name: "audit:--prompt-file", cmd: ownedBy(auditCmd), flag: "prompt-file",
			lookup: lookupLocal,
		},
		{
			// Not redefined locally: audit inherits the persistent flag from eval.
			// Inheritance fails if audit is not a child of eval.
			name: "audit:--evals-dir", cmd: ownedBy(auditCmd), flag: "evals-dir",
			lookup: lookupInherited, notLocal: true,
		},
		{
			name: "trend:--json", cmd: ownedBy(trendCmd), flag: "json",
			lookup: lookupLocal, def: strPtr("false"),
		},
		{
			// Not redefined locally: trend inherits the persistent flag from eval.
			name: "trend:--evals-dir", cmd: ownedBy(trendCmd), flag: "evals-dir",
			lookup: lookupInherited, notLocal: true,
		},
		{
			name: "inference-exec:--backend", cmd: ownedBy(inferenceExecCmd), flag: "backend",
			lookup: lookupFlags,
		},
	}
	for _, tc := range cases {
		runFlagCase(t, tc)
	}

	// Deviates from spec "Table: TestEvalFlagsRegistered", which lists
	// inference-exec:hidden as a row of the table. The hidden check is not a
	// flag lookup (it reads Command.Hidden and the root help text), so it is a
	// plain subtest next to the table rather than a flagCase field. The
	// subtest name and assertions are the ones the spec gives.
	t.Run("inference-exec:hidden", func(t *testing.T) {
		if !inferenceExecCmd.Hidden {
			t.Fatal("inference-exec must be Hidden")
		}

		var help bytes.Buffer
		rootCmd.SetOut(&help)
		t.Cleanup(func() { rootCmd.SetOut(nil) })
		if err := rootCmd.Help(); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(help.String(), "inference-exec") {
			t.Errorf("root help lists inference-exec:\n%s", help.String())
		}
	})
}

// findCheckCmd resolves the check command through rootCmd, so a row fails if
// check is not registered on the root.
func findCheckCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cc, _, err := rootCmd.Find([]string{"check"})
	if err != nil || cc.Name() != "check" {
		t.Fatalf("kairon check command is not registered (Find: %v)", err)
	}
	return cc
}

// TestEvalAgentSubcommandArgs pins the arity of the per-agent eval
// subcommands: each takes exactly one agent argument.
func TestEvalAgentSubcommandArgs(t *testing.T) {
	cases := []struct {
		name string
		cmd  *cobra.Command
		one  string // a valid single argument
	}{
		{name: "audit", cmd: auditCmd, one: "builder"},
		{name: "trend", cmd: trendCmd, one: "selftest"},
		{name: "verify-log", cmd: verifyLogCmd, one: "selftest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cmd.Args(tc.cmd, nil); err == nil {
				t.Error("expected arg error when agent is missing")
			}
			if err := tc.cmd.Args(tc.cmd, []string{"a", "b"}); err == nil {
				t.Error("expected arg error for extra arguments")
			}
			if err := tc.cmd.Args(tc.cmd, []string{tc.one}); err != nil {
				t.Errorf("one argument should be accepted: %v", err)
			}
		})
	}
}

func TestEvalUnknownBackendRejected(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		perf  bool
		clean bool
		// flag, when set, names the --flag in the failure message, as the
		// former TestEvalUnknownBackendRejectedForPerfAndCleanup did.
		flag string
		// want lists substrings the error must contain.
		want []string
	}{
		{
			name: "selftest", args: []string{"selftest"},
			want: []string{"nope", inference.NameKiroCLI, inference.NameStub},
		},
		{
			// --perf and --cleanup go through RunWithOptions, so configuration
			// (and backend validation) applies before any work is done.
			name: "perf", perf: true, flag: "perf",
			want: []string{"unknown backend"},
		},
		{
			name: "cleanup", clean: true, flag: "cleanup",
			want: []string{"unknown backend"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origBackend, origPerf, origCleanup := evalBackend, evalPerf, evalCleanup
			t.Cleanup(func() { evalBackend, evalPerf, evalCleanup = origBackend, origPerf, origCleanup })
			evalBackend, evalPerf, evalCleanup = "nope", tc.perf, tc.clean

			err := evalCmd.RunE(evalCmd, tc.args)
			if tc.flag != "" {
				for _, want := range tc.want {
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Errorf("--%s with bad backend: err = %v, want unknown backend error", tc.flag, err)
					}
				}
				return
			}
			if err == nil {
				t.Fatal("expected error for unknown backend")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err.Error(), want)
				}
			}
		})
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
