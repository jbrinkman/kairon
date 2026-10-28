package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

const (
	verifyLogFixtures = "../../../internal/eval/testdata/verifylog"
	verifyLogEvalsDir = verifyLogFixtures + "/evals"
)

// runVerifyLog runs verify-log for the selftest agent against a fixture set
// with the given --min-iterations and --require-cycle, restoring the package
// flag vars afterwards.
func runVerifyLog(t *testing.T, set string, minIterations int, requireCycle bool) (string, error) {
	t.Helper()
	origIters, origMin, origCycle, origEvals := verifyLogIterationsDir, verifyLogMinIterations, verifyLogRequireCycle, evalEvalsDir
	t.Cleanup(func() {
		verifyLogIterationsDir, verifyLogMinIterations, verifyLogRequireCycle, evalEvalsDir = origIters, origMin, origCycle, origEvals
	})
	verifyLogIterationsDir = verifyLogFixtures + "/" + set
	verifyLogMinIterations = minIterations
	verifyLogRequireCycle = requireCycle
	evalEvalsDir = verifyLogEvalsDir

	var out bytes.Buffer
	verifyLogCmd.SetOut(&out)
	t.Cleanup(func() { verifyLogCmd.SetOut(nil) })
	err := verifyLogCmd.RunE(verifyLogCmd, []string{"selftest"})
	return out.String(), err
}

func TestVerifyLogFlags(t *testing.T) {
	t.Run("registered", func(t *testing.T) {
		cases := []flagCase{
			{
				name: "iterations-dir", cmd: ownedBy(verifyLogCmd), flag: "iterations-dir",
				lookup: lookupFlags, def: strPtr(".kairon/iterations"),
			},
			{
				name: "min-iterations", cmd: ownedBy(verifyLogCmd), flag: "min-iterations",
				lookup: lookupFlags, def: strPtr("2"),
			},
			{
				name: "require-cycle", cmd: ownedBy(verifyLogCmd), flag: "require-cycle",
				lookup: lookupFlags, def: strPtr("true"), usage: []string{"--require-cycle=false"},
			},
			{
				// --evals-dir is the persistent flag from eval, not redefined locally.
				// lookupInherited keeps the row order-independent (see eval_test.go).
				name: "evals-dir", cmd: ownedBy(verifyLogCmd), flag: "evals-dir",
				lookup: lookupInherited, notLocal: true,
			},
		}
		for _, tc := range cases {
			runFlagCase(t, tc)
		}
	})

	t.Run("require-cycle-bound", func(t *testing.T) {
		// The flag must reach the package var: parse it through pflag.
		flag := verifyLogCmd.Flags().Lookup("require-cycle")
		if flag == nil {
			t.Fatal("--require-cycle flag not registered")
		}
		orig := verifyLogRequireCycle
		t.Cleanup(func() {
			verifyLogRequireCycle = orig
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		})
		if err := flag.Value.Set("false"); err != nil {
			t.Fatalf("setting --require-cycle=false: %v", err)
		}
		if verifyLogRequireCycle {
			t.Error("--require-cycle=false did not update verifyLogRequireCycle")
		}
	})
}

func TestVerifyLogValidSet(t *testing.T) {
	for _, requireCycle := range []bool{true, false} {
		t.Run(fmt.Sprintf("require-cycle=%v", requireCycle), func(t *testing.T) {
			out, err := runVerifyLog(t, "valid", 2, requireCycle)
			if err != nil {
				t.Fatalf("valid set (require-cycle=%v): unexpected error: %v", requireCycle, err)
			}
			if !strings.Contains(out, "OK selftest: 4 iterations verified") {
				t.Errorf("output = %q, want OK line", out)
			}
		})
	}
}

func TestVerifyLogFailingSets(t *testing.T) {
	tests := []struct {
		set      string
		file     string
		rule     string
		mentions []string
	}{
		{"missing-run", "iteration-01.md", "run-exists", nil},
		{"score-mismatch", "iteration-01.md", "score-match", nil},
		{"gap", "iteration-02.md", "sequence", nil},
		{"broken-chain", "iteration-01.md", "chain", nil},
		{"no-prompt-change", "iteration-01.md", "prompt-change", nil},
		{"partial-run", "iteration-01.md", "run-complete", nil},
		{"model-mismatch", "iteration-01.md", "run-config", []string{"agent_model"}},
		{"sandbox-mismatch", "iteration-01.md", "run-config", []string{"sandbox"}},
		{"judge-mismatch", "iteration-01.md", "run-config", []string{"judge_model"}},
		{"suite-changed", "iteration-01.md", "suite-change", nil},
		{"eval-prompt-changed", "iteration-01.md", "prompt-change", nil},
		{"eval-nothing-changed", "iteration-01.md", "suite-change", nil},
		{"legacy-run", "iteration-01.md", "run-data", []string{"suite_sha256", "complete"}},
		{"missing-phase", "iteration-00.md", "phase", nil},
		{"baseline-not-first", "iteration-01.md", "phase", []string{"baseline"}},
		{"red-no-new-failure", "iteration-01.md", "red", nil},
		{"green-no-fix", "iteration-02.md", "green", nil},
		{"green-score-drop", "iteration-02.md", "green", []string{"score"}},
		{"refactor-score-drop", "iteration-03.md", "refactor", nil},
	}
	for _, tt := range tests {
		t.Run(tt.set, func(t *testing.T) {
			// Fails with and without the default --min-iterations (2), and
			// with --require-cycle on (the default) and off: the defect is
			// reported either way.
			for _, min := range []int{0, 2} {
				for _, cycle := range []bool{true, false} {
					assertVerifyLogFailure(t, tt.set, min, cycle, tt.file, tt.rule, tt.mentions)
				}
			}
		})
	}
}

func assertVerifyLogFailure(t *testing.T, set string, min int, cycle bool, file, rule string, mentions []string) {
	t.Helper()
	_, err := runVerifyLog(t, set, min, cycle)
	if err == nil {
		t.Fatalf("%s (min=%d, cycle=%v): expected verification error", set, min, cycle)
	}
	if !strings.Contains(err.Error(), file) {
		t.Errorf("%s (min=%d, cycle=%v): error %q does not name file %q", set, min, cycle, err.Error(), file)
	}
	if !strings.Contains(err.Error(), rule) {
		t.Errorf("%s (min=%d, cycle=%v): error %q does not name rule %q", set, min, cycle, err.Error(), rule)
	}
	for _, m := range mentions {
		if !strings.Contains(err.Error(), m) {
			t.Errorf("%s (min=%d, cycle=%v): error %q does not mention %q", set, min, cycle, err.Error(), m)
		}
	}
}

func TestVerifyLogNoRefactorFailsByDefault(t *testing.T) {
	t.Run("require-cycle=true", func(t *testing.T) {
		_, err := runVerifyLog(t, "no-refactor", 2, true)
		if err == nil {
			t.Fatal("no-refactor with --require-cycle: expected a cycle violation")
		}
		if !strings.Contains(err.Error(), "cycle") {
			t.Errorf("error %q does not name the cycle rule", err.Error())
		}
	})
	t.Run("require-cycle=false", func(t *testing.T) {
		out, err := runVerifyLog(t, "no-refactor", 2, false)
		if err != nil {
			t.Fatalf("no-refactor with --require-cycle=false: unexpected error: %v", err)
		}
		if !strings.Contains(out, "OK selftest: 3 iterations verified") {
			t.Errorf("output = %q, want OK line with 3 iterations", out)
		}
	})
}

func TestVerifyLogMinIterations(t *testing.T) {
	_, err := runVerifyLog(t, "valid", 5, true)
	if err == nil || !strings.Contains(err.Error(), "min-iterations") {
		t.Fatalf("--min-iterations 5 on valid set: err = %v, want min-iterations violation", err)
	}
}

func TestVerifyLogNegativeMinIterationsRejected(t *testing.T) {
	_, err := runVerifyLog(t, "valid", -1, true)
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("negative --min-iterations: err = %v, want a 'negative' error", err)
	}
}

func TestVerifyLogSilencesUsageOnVerificationFailure(t *testing.T) {
	t.Cleanup(func() { verifyLogCmd.SilenceUsage = false })
	verifyLogCmd.SilenceUsage = false
	if _, err := runVerifyLog(t, "gap", 2, true); err == nil {
		t.Fatal("expected verification error")
	}
	if !verifyLogCmd.SilenceUsage {
		t.Error("usage must be silenced once arguments are parsed")
	}
}
