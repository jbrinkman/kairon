package cmd

import (
	"strings"
	"testing"

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
