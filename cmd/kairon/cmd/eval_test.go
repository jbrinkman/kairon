package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
	"github.com/spf13/cobra"
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

// verifyLogSub returns the registered `eval verify-log` subcommand, failing the
// test when it is not registered.
func verifyLogSub(t *testing.T) *cobra.Command {
	t.Helper()
	for _, c := range evalCmd.Commands() {
		if c.Name() == "verify-log" {
			return c
		}
	}
	t.Fatal("verify-log subcommand not registered on eval")
	return nil
}

func TestEvalFlagsVerifyLogRegistered(t *testing.T) {
	sub := verifyLogSub(t)

	if err := sub.Args(sub, nil); err == nil {
		t.Error("verify-log accepted zero arguments, want exactly one")
	}
	if err := sub.Args(sub, []string{"a", "b"}); err == nil {
		t.Error("verify-log accepted two arguments, want exactly one")
	}
	if err := sub.Args(sub, []string{"selftest"}); err != nil {
		t.Errorf("verify-log rejected one argument: %v", err)
	}

	iterDir := sub.Flags().Lookup("iterations-dir")
	if iterDir == nil {
		t.Fatal("--iterations-dir flag not registered on verify-log")
	}
	if iterDir.DefValue != ".kairon/iterations" {
		t.Errorf("--iterations-dir default = %q, want %q", iterDir.DefValue, ".kairon/iterations")
	}
	minIter := sub.Flags().Lookup("min-iterations")
	if minIter == nil {
		t.Fatal("--min-iterations flag not registered on verify-log")
	}
	if minIter.DefValue != "2" {
		t.Errorf("--min-iterations default = %q, want %q", minIter.DefValue, "2")
	}
	// --evals-dir is the persistent flag of eval, inherited by verify-log.
	if sub.InheritedFlags().Lookup("evals-dir") == nil {
		t.Error("verify-log does not inherit --evals-dir")
	}
}

// runVerifyLog runs the verify-log subcommand against a fixture set under
// internal/eval/testdata/verifylog and returns the error and captured stderr.
func runVerifyLog(t *testing.T, set string, minIterations int) (error, string) {
	t.Helper()
	sub := verifyLogSub(t)
	root := filepath.Join("..", "..", "..", "internal", "eval", "testdata", "verifylog")

	origEvals := evalEvalsDir
	t.Cleanup(func() {
		evalEvalsDir = origEvals
		_ = sub.Flags().Set("iterations-dir", sub.Flags().Lookup("iterations-dir").DefValue)
		_ = sub.Flags().Set("min-iterations", sub.Flags().Lookup("min-iterations").DefValue)
		sub.SetErr(nil)
		sub.SetOut(nil)
	})
	evalEvalsDir = filepath.Join(root, "evals")
	if err := sub.Flags().Set("iterations-dir", filepath.Join(root, set)); err != nil {
		t.Fatal(err)
	}
	if err := sub.Flags().Set("min-iterations", strconv.Itoa(minIterations)); err != nil {
		t.Fatal(err)
	}

	var stderr, stdout bytes.Buffer
	sub.SetErr(&stderr)
	sub.SetOut(&stdout)
	err := sub.RunE(sub, []string{"selftest"})
	return err, stderr.String()
}

func TestEvalVerifyLogValidSet(t *testing.T) {
	err, stderr := runVerifyLog(t, "valid", 2)
	if err != nil {
		t.Fatalf("valid set returned error: %v (stderr: %s)", err, stderr)
	}
	if stderr != "" {
		t.Errorf("valid set wrote to stderr: %q", stderr)
	}
}

func TestEvalVerifyLogViolations(t *testing.T) {
	tests := []struct {
		set  string
		file string
		rule string
	}{
		{"missing-run", "iteration-01.md", "run-missing"},
		{"score-mismatch", "iteration-00.md", "score-mismatch"},
		{"gap", "iteration-02.md", "numbering"},
		{"broken-chain", "iteration-01.md", "chain"},
		{"no-prompt-change", "iteration-00.md", "prompt-unchanged"},
	}
	for _, tt := range tests {
		t.Run(tt.set, func(t *testing.T) {
			err, stderr := runVerifyLog(t, tt.set, 2)
			if err == nil {
				t.Fatalf("expected error for %s set (stderr: %s)", tt.set, stderr)
			}
			if !strings.Contains(err.Error(), "verify-log failed") {
				t.Errorf("error %q does not mention verify-log failed", err)
			}
			line := tt.file + ": [" + tt.rule + "]"
			if !strings.Contains(stderr, line) {
				t.Errorf("stderr %q does not contain %q", stderr, line)
			}
		})
	}
}

func TestEvalVerifyLogMinIterations(t *testing.T) {
	err, stderr := runVerifyLog(t, "valid", 5)
	if err == nil {
		t.Fatal("expected error for valid set with --min-iterations 5")
	}
	if !strings.Contains(stderr, "[min-iterations]") {
		t.Errorf("stderr %q does not contain the min-iterations rule", stderr)
	}
	if !strings.Contains(err.Error()+stderr, "min-iterations") {
		t.Errorf("error %q / stderr %q do not mention min-iterations", err, stderr)
	}
}
