package eval

import (
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

	cfg = runConfig{evalsDir: dir, backend: backend} // pins reset to nil
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
