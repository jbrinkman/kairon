package cmd

import (
	"bytes"
	"strings"
	"testing"
)

const (
	verifyLogFixtures = "../../../internal/eval/testdata/verifylog"
	verifyLogEvalsDir = verifyLogFixtures + "/evals"
)

// runVerifyLog runs verify-log for the selftest agent against a fixture set
// with the given --min-iterations, restoring the package flag vars afterwards.
func runVerifyLog(t *testing.T, set string, minIterations int) (string, error) {
	t.Helper()
	origIters, origMin, origEvals := verifyLogIterationsDir, verifyLogMinIterations, evalEvalsDir
	t.Cleanup(func() {
		verifyLogIterationsDir, verifyLogMinIterations, evalEvalsDir = origIters, origMin, origEvals
	})
	verifyLogIterationsDir = verifyLogFixtures + "/" + set
	verifyLogMinIterations = minIterations
	evalEvalsDir = verifyLogEvalsDir

	var out bytes.Buffer
	verifyLogCmd.SetOut(&out)
	t.Cleanup(func() { verifyLogCmd.SetOut(nil) })
	err := verifyLogCmd.RunE(verifyLogCmd, []string{"selftest"})
	return out.String(), err
}

func TestVerifyLogFlagsRegistered(t *testing.T) {
	found := false
	for _, c := range evalCmd.Commands() {
		if c == verifyLogCmd {
			found = true
		}
	}
	if !found {
		t.Fatal("verify-log is not registered on eval")
	}
	if verifyLogCmd.Name() != "verify-log" {
		t.Errorf("command name = %q, want verify-log", verifyLogCmd.Name())
	}

	iters := verifyLogCmd.Flags().Lookup("iterations-dir")
	if iters == nil {
		t.Fatal("--iterations-dir flag not registered")
	}
	if iters.DefValue != ".kairon/iterations" {
		t.Errorf("--iterations-dir default = %q, want .kairon/iterations", iters.DefValue)
	}
	min := verifyLogCmd.Flags().Lookup("min-iterations")
	if min == nil {
		t.Fatal("--min-iterations flag not registered")
	}
	if min.DefValue != "2" {
		t.Errorf("--min-iterations default = %q, want 2", min.DefValue)
	}
	// --evals-dir is the persistent flag from eval, not redefined locally.
	if verifyLogCmd.LocalFlags().Lookup("evals-dir") != nil {
		t.Error("--evals-dir must be inherited from eval, not redefined")
	}
	if verifyLogCmd.InheritedFlags().Lookup("evals-dir") == nil {
		t.Error("verify-log does not inherit --evals-dir")
	}
}

func TestVerifyLogValidSet(t *testing.T) {
	out, err := runVerifyLog(t, "valid", 2)
	if err != nil {
		t.Fatalf("valid set: unexpected error: %v", err)
	}
	if !strings.Contains(out, "OK selftest: 3 iterations verified") {
		t.Errorf("output = %q, want OK line", out)
	}
}

func TestVerifyLogFailingSets(t *testing.T) {
	tests := []struct {
		set  string
		file string
		rule string
	}{
		{"missing-run", "iteration-01.md", "run-exists"},
		{"score-mismatch", "iteration-01.md", "score-match"},
		{"gap", "iteration-02.md", "sequence"},
		{"broken-chain", "iteration-01.md", "chain"},
		{"no-prompt-change", "iteration-01.md", "prompt-change"},
	}
	for _, tt := range tests {
		t.Run(tt.set, func(t *testing.T) {
			_, err := runVerifyLog(t, tt.set, 2)
			if err == nil {
				t.Fatalf("%s: expected verification error", tt.set)
			}
			if !strings.Contains(err.Error(), tt.file) {
				t.Errorf("error %q does not name file %q", err.Error(), tt.file)
			}
			if !strings.Contains(err.Error(), tt.rule) {
				t.Errorf("error %q does not name rule %q", err.Error(), tt.rule)
			}
		})
	}
}

func TestVerifyLogMinIterations(t *testing.T) {
	_, err := runVerifyLog(t, "valid", 5)
	if err == nil || !strings.Contains(err.Error(), "min-iterations") {
		t.Fatalf("--min-iterations 5 on valid set: err = %v, want min-iterations violation", err)
	}
}

func TestVerifyLogNegativeMinIterationsRejected(t *testing.T) {
	_, err := runVerifyLog(t, "valid", -1)
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("negative --min-iterations: err = %v, want a 'negative' error", err)
	}
}

func TestVerifyLogMissingAgentIsUsageError(t *testing.T) {
	if err := verifyLogCmd.Args(verifyLogCmd, nil); err == nil {
		t.Error("expected arg error when agent is missing")
	}
	if err := verifyLogCmd.Args(verifyLogCmd, []string{"a", "b"}); err == nil {
		t.Error("expected arg error for extra arguments")
	}
	if err := verifyLogCmd.Args(verifyLogCmd, []string{"selftest"}); err != nil {
		t.Errorf("one argument should be accepted: %v", err)
	}
}

func TestVerifyLogSilencesUsageOnVerificationFailure(t *testing.T) {
	t.Cleanup(func() { verifyLogCmd.SilenceUsage = false })
	verifyLogCmd.SilenceUsage = false
	if _, err := runVerifyLog(t, "gap", 2); err == nil {
		t.Fatal("expected verification error")
	}
	if !verifyLogCmd.SilenceUsage {
		t.Error("usage must be silenced once arguments are parsed")
	}
}
