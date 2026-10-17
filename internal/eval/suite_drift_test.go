package eval

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

// pinAlpha configures the suiteEvals directory, pins agent alpha the way
// RunWithOptions does and returns the directory and the pinned hash.
func pinAlpha(t *testing.T) (dir, pinned string) {
	t.Helper()
	dir = suiteEvals(t)
	opts := RunOptions{Backend: inference.NameStub, EvalsDir: dir}
	if err := configure(opts); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	if err := pinSuites("alpha", opts); err != nil {
		t.Fatal(err)
	}
	pinned = cfg.suites["alpha"]
	if pinned == "" {
		t.Fatal("pinSuites pinned no suite for alpha")
	}
	return dir, pinned
}

func appendToFile(t *testing.T, path, extra string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSuiteUnchangedNoDrift(t *testing.T) {
	pinAlpha(t)
	if err := checkSuiteUnchanged("alpha"); err != nil {
		t.Fatalf("checkSuiteUnchanged on an untouched suite = %v, want nil", err)
	}
}

func TestCheckSuiteUnchangedRefusesEdits(t *testing.T) {
	tests := []struct {
		name string
		edit string // path relative to the evals directory
		add  string
	}{
		{"case file", "cases/alpha/one.yaml", "# edited mid-run\n"},
		{"rubric", "rubrics/alpha.yaml", "# edited mid-run\n"},
		{"referenced fixture", "fixtures/workspaces/shared/README.md", "edited mid-run\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, pinned := pinAlpha(t)
			appendToFile(t, filepath.Join(dir, tt.edit), tt.add)
			now, err := suiteSHA256("alpha")
			if err != nil {
				t.Fatal(err)
			}
			if now == pinned {
				t.Fatalf("test setup: editing %s did not change the suite hash", tt.edit)
			}

			err = checkSuiteUnchanged("alpha")
			if err == nil {
				t.Fatal("checkSuiteUnchanged = nil after the suite was edited, want a refusal")
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "❌") {
				t.Errorf("error %q does not start with ❌", msg)
			}
			for _, want := range []string{"alpha", "suite_sha256 at run start=" + pinned, "now " + now} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
		})
	}
}

func TestCheckSuiteUnchangedNotPinnedIsNoOp(t *testing.T) {
	dir, _ := pinAlpha(t)
	appendToFile(t, filepath.Join(dir, "rubrics/alpha.yaml"), "# edited\n")
	for name, suites := range map[string]map[string]string{
		"nil map":     nil,
		"other agent": {"beta": testSuiteHash},
	} {
		t.Run(name, func(t *testing.T) {
			cfg.suites = suites
			if err := checkSuiteUnchanged("alpha"); err != nil {
				t.Fatalf("checkSuiteUnchanged for an unpinned agent = %v, want nil", err)
			}
		})
	}
}

func TestCheckSuiteUnchangedHashErrorRefuses(t *testing.T) {
	dir, _ := pinAlpha(t)
	if err := os.WriteFile(filepath.Join(dir, "cases/alpha/one.yaml"), []byte("name: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := suiteSHA256("alpha"); err == nil {
		t.Fatal("test setup: suiteSHA256 still succeeds with a corrupt case file")
	}

	err := checkSuiteUnchanged("alpha")
	if err == nil {
		t.Fatal("checkSuiteUnchanged = nil when the suite cannot be re-hashed, want a refusal")
	}
	if !strings.HasPrefix(err.Error(), "❌ cannot re-hash") || !strings.Contains(err.Error(), "alpha") {
		t.Errorf("error = %q, want ❌ cannot re-hash ... naming alpha", err)
	}
}

// driftSuite builds a real two-case suite for agent "drift" in a temp evals
// directory, makes it the active one, pins it and returns the directory, the
// pinned hash and a marker path that case two creates when it runs.
func driftSuite(t *testing.T) (dir, pinned, marker string) {
	t.Helper()
	chdirTemp(t)
	installFakeKiroCLI(t, "")
	initGitRepoHere(t)

	dir = t.TempDir()
	marker = filepath.Join(t.TempDir(), "case-two-ran")
	rubric := filepath.Join(dir, "rubrics", "drift.yaml")
	caseYAML := func(name, command string) string {
		return "name: " + name + "\ndescription: d\nagent: drift\ninput: do it\nstub:\n  turns:\n    - commands:\n        - \"" + command + "\"\n      response: |\n        ## H\n\n        ### S\n\n        text\n"
	}
	writeEvalFile(t, dir, "rubrics/drift.yaml", "agent: drift\ncriteria:\n  - name: structural_completeness\n    description: d\n    scoring: 1-5\n    deterministic: true\n")
	writeEvalFile(t, dir, "cases/drift/c1.yaml", caseYAML("c1", "echo '# edited mid-run' >> '"+rubric+"'"))
	writeEvalFile(t, dir, "cases/drift/c2.yaml", caseYAML("c2", "touch '"+marker+"'"))

	opts := RunOptions{Backend: inference.NameStub, EvalsDir: dir}
	if err := configure(opts); err != nil {
		t.Fatal(err)
	}
	if err := pinSuites("drift", opts); err != nil {
		t.Fatal(err)
	}
	pinned = cfg.suites["drift"]
	if pinned == "" {
		t.Fatal("pinSuites pinned no suite for drift")
	}
	return dir, pinned, marker
}

// driftRubric is the in-memory rubric matching the on-disk drift rubric.
func driftRubric() Rubric {
	r := verdictRubric()
	r.Agent = "drift"
	return r
}

var driftErrRE = regexp.MustCompile(`^❌ eval suite of drift changed while the run was in progress \(suite_sha256 at run start=([0-9a-f]{64}); now ([0-9a-f]{64})\)`)

func assertDriftError(t *testing.T, err error, pinned, now string) {
	t.Helper()
	if err == nil {
		t.Fatal("got nil error after the suite was edited mid-run, want a refusal")
	}
	m := driftErrRE.FindStringSubmatch(err.Error())
	if m == nil {
		t.Fatalf("error %q does not match %s", err, driftErrRE)
	}
	if m[1] != pinned || m[2] != now {
		t.Errorf("error hashes = %s / %s, want pinned %s and now %s", m[1], m[2], pinned, now)
	}
}

func TestEvaluateProgressiveRefusesMidRunEdit(t *testing.T) {
	dir, pinned, marker := driftSuite(t)
	rubricPath := filepath.Join(dir, "rubrics", "drift.yaml")
	cases := []TestCase{
		covCase("c1", "echo '# edited mid-run' >> '"+rubricPath+"'"),
		covCase("c2", "touch '"+marker+"'"),
	}
	resultsDir := t.TempDir()

	_, err := evaluateProgressive(driftRubric(), cases, "abc", &strings.Builder{}, resultsDir, false, nil)

	now, hashErr := suiteSHA256("drift")
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	assertDriftError(t, err, pinned, now)
	if _, statErr := os.Stat(filepath.Join(resultsDir, "drift.json")); !os.IsNotExist(statErr) {
		t.Errorf("drift.json was written for the case that finished after the edit (stat err = %v)", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(resultsDir, "summary.json")); !os.IsNotExist(statErr) {
		t.Errorf("summary.json was written for the case that finished after the edit (stat err = %v)", statErr)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Errorf("case two ran after the drift was detected (stat err = %v)", statErr)
	}
}

// An edit made during the second case leaves the first save in place, and that
// save still carries a hash that matches the suite it was saved under.
func TestEvaluateProgressiveMidRunEditKeepsEarlierSaves(t *testing.T) {
	dir, pinned, marker := driftSuite(t)
	rubricPath := filepath.Join(dir, "rubrics", "drift.yaml")
	cases := []TestCase{
		covCase("c1"),
		covCase("c2", "echo '# edited mid-run' >> '"+rubricPath+"'"),
		covCase("c3", "touch '"+marker+"'"),
	}
	resultsDir := t.TempDir()

	_, err := evaluateProgressive(driftRubric(), cases, "abc", &strings.Builder{}, resultsDir, false, nil)

	now, hashErr := suiteSHA256("drift")
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	assertDriftError(t, err, pinned, now)
	var saved AgentResult
	readJSON(t, filepath.Join(resultsDir, "drift.json"), &saved)
	if saved.SuiteSHA256 != pinned {
		t.Errorf("saved suite_sha256 = %q, want the pinned %q", saved.SuiteSHA256, pinned)
	}
	for _, cr := range saved.Cases {
		if cr.CaseName != "c1" {
			t.Errorf("saved case %q, want only c1 (c2 finished after the edit)", cr.CaseName)
		}
	}
	if len(saved.Cases) != 1 {
		t.Errorf("saved %d case results, want 1", len(saved.Cases))
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Errorf("case three ran after the drift was detected (stat err = %v)", statErr)
	}
}

func TestRunProgressiveEvaluationMidRunEditFails(t *testing.T) {
	_, pinned, marker := driftSuite(t)
	resultsDir := t.TempDir()
	progress := filepath.Join(resultsDir, ".progress")
	if err := os.WriteFile(progress, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runProgressiveEvaluation("drift", resultsDir, false, nil)

	now, hashErr := suiteSHA256("drift")
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	assertDriftError(t, err, pinned, now)
	if _, statErr := os.Stat(progress); statErr != nil {
		t.Errorf(".progress marker was removed: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(resultsDir, "drift.json")); !os.IsNotExist(statErr) {
		t.Errorf("a result file was saved for the case that finished after the edit (stat err = %v)", statErr)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Errorf("case two ran after the drift was detected (stat err = %v)", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(resultsDir, ".lock")); !os.IsNotExist(statErr) {
		t.Errorf("result lock was not released (stat err = %v)", statErr)
	}
}

func TestRunSingleTestCaseMidRunEditFails(t *testing.T) {
	dir, pinned, _ := driftSuite(t)

	err := runSingleTestCase("drift", "c1", nil)

	now, hashErr := suiteSHA256("drift")
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	assertDriftError(t, err, pinned, now)
	var written []string
	_ = filepath.Walk(filepath.Join(dir, "results"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".json") && filepath.Base(p) != "performance.json" {
			written = append(written, p)
		}
		return nil
	})
	if len(written) != 0 {
		t.Errorf("result files written despite the drift: %v", written)
	}
}

// On a resumed run where every case is already completed, the per-case loop
// skips every iteration, so the per-case checkSuiteUnchanged never runs. The
// suite must still be re-verified before evaluateProgressive returns success,
// otherwise the caller's final save records the pinned (stale) hash for a
// suite that was edited after the pin.
func TestEvaluateProgressiveAllSkippedResumeRefusesDrift(t *testing.T) {
	dir, pinned, _ := driftSuite(t)
	rubricPath := filepath.Join(dir, "rubrics", "drift.yaml")
	// Harmless cases: the first (clean) pass must not itself edit the suite.
	cases := []TestCase{covCase("c1"), covCase("c2")}
	resultsDir := t.TempDir()

	// First pass: a clean run that completes both cases and saves drift.json
	// under the pinned hash.
	if _, err := evaluateProgressive(driftRubric(), cases, "abc", &strings.Builder{}, resultsDir, false, nil); err != nil {
		t.Fatalf("clean first pass: %v", err)
	}
	for _, tc := range cases {
		if !isTestCaseCompleted(resultsDir, "drift", tc.Name) {
			t.Fatalf("test setup: %s not marked completed after the clean pass", tc.Name)
		}
	}

	// Edit the suite after the pin, the way a concurrent agent's case would.
	appendToFile(t, rubricPath, "# edited after the pin\n")

	// Resume: every case is already completed, so the loop skips them all and
	// no per-case checkSuiteUnchanged runs.
	_, err := evaluateProgressive(driftRubric(), cases, "abc", &strings.Builder{}, resultsDir, true, nil)

	now, hashErr := suiteSHA256("drift")
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	assertDriftError(t, err, pinned, now)
}
