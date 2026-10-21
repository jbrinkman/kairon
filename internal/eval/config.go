package eval

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jbrinkman/kairon/internal/inference"
)

const (
	// defaultEvalsDir is the repo-relative directory holding rubrics, cases,
	// fixtures and results when no --evals-dir is given.
	defaultEvalsDir = ".kairon/evals"

	// defaultBackend is the inference backend used when none is requested.
	defaultBackend = inference.NameKiroCLI
)

// runConfig holds package-level settings for an eval run. The eval package
// already relies on package state (e.g. globalProfiler), and loadCases /
// loadRubrics / GetTestCase are called from many places with fixed
// signatures, so configuration is kept here rather than threaded through.
type runConfig struct {
	evalsDir string
	backend  inference.Backend
	// pins is the run's pinned models and prompt provenance, set by pinRun.
	// nil means unpinned (direct calls that bypass RunWithOptions): empty
	// Request.Model and no provenance fields.
	pins *runPins
	// candidate is the --prompt-file candidate prompt under evaluation, set by
	// RunWithOptions after configure. nil means the live prompt is evaluated.
	candidate *candidatePrompt
	// keepWorkspaces keeps per-case workspaces after the run (--keep-workspaces).
	keepWorkspaces bool
	// debug prints each judge-check prompt to debugWriter (--debug).
	debug bool
	// repeat is the number of times every case is executed (--repeat). Zero
	// (unset, e.g. a direct call that bypasses RunWithOptions) means 1; read
	// it through runRepeat.
	repeat int
	// suites maps each agent in scope to its suite_sha256, set by RunWithOptions.
	// nil means coverage is not tracked (direct calls that bypass it), like pins.
	suites map[string]string
	// caseFilter is the --case value of the run; "" for a full run.
	caseFilter string
	// filters are the parsed --filter expressions of the run; nil when none.
	filters []Filter
	// scopeAgents narrows agentsInScope to the agents that have a matching
	// case; nil means no narrowing.
	scopeAgents []string
}

// cfg is the active run configuration. Call configure before use; tests
// reset it with resetConfig.
//
// Invariant: configure runs once before a run starts and cfg is only read
// (never reassigned) during the run. InvestigateParallelExecution reads
// cfg.backend from parallel goroutines, so configuring while a run is in
// flight would be a data race.
var cfg = newDefaultConfig()

func newDefaultConfig() runConfig {
	b, err := inference.New(defaultBackend)
	if err != nil {
		// The default backend is always registered; this cannot happen.
		panic(err)
	}
	return runConfig{evalsDir: defaultEvalsDir, backend: b}
}

// resetConfig restores the default configuration (used by tests).
func resetConfig() {
	cfg = newDefaultConfig()
}

// configure applies opts to the package configuration. Empty values select
// the defaults, so repeated calls never leak settings from a previous run.
// An unknown backend is rejected before any state is changed.
func configure(opts RunOptions) error {
	name := opts.Backend
	if name == "" {
		name = defaultBackend
	}
	backend, err := inference.New(name)
	if err != nil {
		return err
	}

	dir := opts.EvalsDir
	if dir == "" {
		dir = defaultEvalsDir
	}

	cfg = runConfig{evalsDir: dir, backend: backend, keepWorkspaces: opts.KeepWorkspaces, debug: opts.Debug, repeat: opts.Repeat} // pins reset to nil
	return nil
}

// runRepeat is the number of times each case runs: the configured --repeat,
// at least 1.
func runRepeat() int {
	return max(1, cfg.repeat)
}

// validateRepeat rejects --repeat values that cannot be honoured. Zero means
// "default 1" for programmatic callers. A repeat above 1 is refused for the
// modes that never run cases (list, cleanup, perf) rather than silently doing
// nothing with it. It changes no state, so RunWithOptions calls it first.
func validateRepeat(opts RunOptions) error {
	if opts.Repeat < 0 {
		return fmt.Errorf("❌ invalid repeat %d: must be at least 1", opts.Repeat)
	}
	if opts.Repeat > 1 {
		for _, m := range []struct {
			flag string
			set  bool
		}{{"--list", opts.List}, {"--cleanup", opts.Cleanup}, {"--perf", opts.Perf}} {
			if m.set {
				return fmt.Errorf("❌ repeat %d cannot be combined with %s: it does not run any cases", opts.Repeat, m.flag)
			}
		}
	}
	return nil
}

// validateFilterOptions rejects --filter and --strict for the modes that never
// run cases (--perf, --cleanup), rather than silently ignoring them, like
// validateRepeat. --strict without --filter is accepted and has no effect.
func validateFilterOptions(opts RunOptions, hasFilters bool) error {
	var used []string
	if hasFilters {
		used = append(used, "--filter")
	}
	if opts.Strict {
		used = append(used, "--strict")
	}
	if len(used) == 0 {
		return nil
	}
	for _, m := range []struct {
		flag string
		set  bool
	}{{"--cleanup", opts.Cleanup}, {"--perf", opts.Perf}} {
		if m.set {
			return fmt.Errorf("❌ %s cannot be combined with %s: it does not run any cases", strings.Join(used, " and "), m.flag)
		}
	}
	return nil
}

// evalsPath joins parts under the configured evals directory.
func evalsPath(parts ...string) string {
	return filepath.Join(append([]string{cfg.evalsDir}, parts...)...)
}

// usingDefaultEvalsDir reports whether the evals dir is the default one.
func usingDefaultEvalsDir() bool {
	return filepath.Clean(cfg.evalsDir) == filepath.Clean(defaultEvalsDir)
}

// rebaseEvalsPath rebases a repo-relative path prefixed with ".kairon/evals/"
// onto the configured evals dir. Paths without that prefix, and all paths when
// the default evals dir is in use, are returned unchanged.
func rebaseEvalsPath(p string) string {
	if usingDefaultEvalsDir() {
		return p
	}
	slashed := filepath.ToSlash(p)
	const prefix = defaultEvalsDir + "/"
	if !strings.HasPrefix(slashed, prefix) {
		return p
	}
	return filepath.Join(cfg.evalsDir, filepath.FromSlash(strings.TrimPrefix(slashed, prefix)))
}
