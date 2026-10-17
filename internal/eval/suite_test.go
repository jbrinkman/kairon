package eval

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

const (
	suiteRubricAlpha = `agent: alpha
criteria:
  - name: crit
    description: "A criterion"
    scoring: 1-5
`
	suiteRubricBeta = `agent: beta
criteria:
  - name: crit
    description: "A criterion"
    scoring: 1-5
`
	// suiteCaseOne seeds the shared workspace and reads a setup file through
	// the default-evals-dir form, which must be rebased onto --evals-dir.
	suiteCaseOne = `name: one
description: "uses the shared workspace and a setup file"
agent: alpha
workspace: shared
setup:
  - type: file
    label: input
    path: .kairon/evals/fixtures/input.md
input: "do the first thing"
`
	// suiteCaseTwo shares the workspace with case one and injects a hidden file.
	suiteCaseTwo = `name: two
description: "shares the workspace and injects a hidden file"
agent: alpha
workspace: shared
input: "do the second thing"
checks:
  - criterion: crit
    type: command
    run: "true"
    inject: [hidden_test.go]
`
	suiteCaseThree = `name: three
description: "uses a mock script"
agent: alpha
requires_sandbox: true
mocks:
  - command: aws
    script: fixtures/mock.sh
input: "do the third thing"
`
	suiteCaseBeta = `name: b
description: "another agent"
agent: beta
input: "beta input"
`
)

// suiteEvals builds a small evals directory under t.TempDir() and returns its
// path. Agent alpha has three cases that reference a shared workspace fixture
// (used by two cases), a hidden file, a mock script and a setup file.
func suiteEvals(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"rubrics/alpha.yaml":                      suiteRubricAlpha,
		"rubrics/beta.yaml":                       suiteRubricBeta,
		"cases/alpha/one.yaml":                    suiteCaseOne,
		"cases/alpha/two.yaml":                    suiteCaseTwo,
		"cases/alpha/three.yaml":                  suiteCaseThree,
		"cases/beta/b.yaml":                       suiteCaseBeta,
		"fixtures/workspaces/shared/README.md":    "# shared\n",
		"fixtures/workspaces/shared/sub/data.txt": "data\n",
		"fixtures/workspaces/unused/other.txt":    "nobody references me\n",
		"fixtures/hidden/hidden_test.go":          "package x\n",
		"fixtures/mock.sh":                        "#!/bin/sh\necho mock\n",
		"fixtures/input.md":                       "setup input\n",
		"agents/alpha.json":                       `{"name":"alpha"}` + "\n",
	}
	for rel, content := range files {
		writeEvalFile(t, dir, rel, content)
	}
	// An executable fixture, so the execute bit is part of the identity.
	script := filepath.Join(dir, "fixtures/workspaces/shared/run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrue\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// suiteHashAt hashes agent's suite with dir as the active evals directory.
func suiteHashAt(t *testing.T, dir, agent string) string {
	t.Helper()
	if err := configure(RunOptions{Backend: "stub", EvalsDir: dir}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetConfig)
	got, err := suiteSHA256(agent)
	if err != nil {
		t.Fatalf("suiteSHA256(%q) in %s: %v", agent, dir, err)
	}
	return got
}

func TestSuiteSHA256Format(t *testing.T) {
	got := suiteHashAt(t, suiteEvals(t), "alpha")
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(got) {
		t.Fatalf("suiteSHA256 = %q, want 64 lowercase hex characters", got)
	}
}

func TestSuiteSHA256DeterministicAcrossCallsAndCopies(t *testing.T) {
	dir := suiteEvals(t)
	first := suiteHashAt(t, dir, "alpha")
	if again := suiteHashAt(t, dir, "alpha"); again != first {
		t.Fatalf("second call = %s, want %s", again, first)
	}

	// A copy of the evals directory at a different path hashes the same:
	// the hash is relative to the evals directory.
	copyDir := filepath.Join(t.TempDir(), "elsewhere", "evals-copy")
	if err := copyTree(dir, copyDir, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := suiteHashAt(t, copyDir, "alpha"); got != first {
		t.Fatalf("copy at %s = %s, want %s", copyDir, got, first)
	}
}

func TestSuiteSHA256DiffersPerAgent(t *testing.T) {
	dir := suiteEvals(t)
	if a, b := suiteHashAt(t, dir, "alpha"), suiteHashAt(t, dir, "beta"); a == b {
		t.Fatalf("alpha and beta share a hash %s", a)
	}
}

// TestSuiteSHA256ChangesWhenSuiteInputChanges covers every kind of input the
// suite identity is built from.
func TestSuiteSHA256ChangesWhenSuiteInputChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
	}{
		{"rubric", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "rubrics/alpha.yaml", suiteRubricAlpha+"# edited\n")
		}},
		{"case file", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "cases/alpha/one.yaml", suiteCaseOne+"# edited\n")
		}},
		{"added case file", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "cases/alpha/four.yaml", "name: four\nagent: alpha\ninput: x\n")
		}},
		{"referenced workspace fixture", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "fixtures/workspaces/shared/sub/data.txt", "changed\n")
		}},
		{"added workspace fixture file", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "fixtures/workspaces/shared/new.txt", "new\n")
		}},
		{"hidden file", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "fixtures/hidden/hidden_test.go", "package changed\n")
		}},
		{"mock script", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "fixtures/mock.sh", "#!/bin/sh\necho changed\n")
		}},
		{"setup file", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "fixtures/input.md", "changed setup input\n")
		}},
		{"fixture execute bit", func(t *testing.T, dir string) {
			if err := os.Chmod(filepath.Join(dir, "fixtures/workspaces/shared/run.sh"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := suiteEvals(t)
			before := suiteHashAt(t, dir, "alpha")
			tt.mutate(t, dir)
			if after := suiteHashAt(t, dir, "alpha"); after == before {
				t.Fatalf("hash did not change after editing the %s", tt.name)
			}
		})
	}
}

// TestSuiteSHA256IgnoresUnrelatedFiles: only the agent's own suite counts.
func TestSuiteSHA256IgnoresUnrelatedFiles(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
	}{
		{"unreferenced workspace fixture", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "fixtures/workspaces/unused/other.txt", "changed\n")
			writeEvalFile(t, dir, "fixtures/workspaces/unused/added.txt", "added\n")
		}},
		{"referenced fixture .git contents", func(t *testing.T, dir string) {
			// A fixture that is a git checkout: newCaseWorkspace skips .git
			// via copyTree and runs a fresh gitInit, so the agent never sees
			// the fixture's own repo. The suite hash must ignore it too.
			writeEvalFile(t, dir, "fixtures/workspaces/shared/.git/HEAD", "ref: refs/heads/main\n")
			writeEvalFile(t, dir, "fixtures/workspaces/shared/.git/objects/ab/cdef", "object\n")
		}},
		{"another agent's case", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "cases/beta/b.yaml", suiteCaseBeta+"# edited\n")
		}},
		{"another agent's rubric", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "rubrics/beta.yaml", suiteRubricBeta+"# edited\n")
		}},
		{"agent config", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "agents/alpha.json", `{"name":"alpha","model":"other"}`+"\n")
		}},
		{"agent prompt", func(t *testing.T, dir string) {
			writeEvalFile(t, dir, "agents/alpha-prompt.md", "a prompt\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := suiteEvals(t)
			before := suiteHashAt(t, dir, "alpha")
			tt.mutate(t, dir)
			if after := suiteHashAt(t, dir, "alpha"); after != before {
				t.Fatalf("hash changed (%s -> %s) after editing an unrelated %s", before, after, tt.name)
			}
		})
	}
}

// TestSuiteSHA256SharedFixtureCountedOnce: two cases name the same workspace;
// its files must appear once among the hashed inputs.
func TestSuiteSHA256SharedFixtureCountedOnce(t *testing.T) {
	dir := suiteEvals(t)
	suiteHashAt(t, dir, "alpha") // configures the evals dir
	inputs, err := collectSuiteInputs("alpha")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, in := range inputs {
		seen[in.kind+" "+in.name]++
	}
	for _, key := range []string{
		"file fixtures/workspaces/shared/README.md",
		"file fixtures/workspaces/shared/sub/data.txt",
		"file+x fixtures/workspaces/shared/run.sh",
	} {
		if seen[key] != 1 {
			t.Errorf("%q counted %d times, want 1 (inputs: %v)", key, seen[key], seen)
		}
	}
}

// TestSuiteSHA256MissingReferencedFile: a referenced file that is absent is
// hashed as "missing", differently from an empty file, and is not an error.
func TestSuiteSHA256MissingReferencedFile(t *testing.T) {
	dir := suiteEvals(t)
	setup := filepath.Join(dir, "fixtures/input.md")

	withContent := suiteHashAt(t, dir, "alpha")

	if err := os.Remove(setup); err != nil {
		t.Fatal(err)
	}
	missing := suiteHashAt(t, dir, "alpha") // fails the test on error

	writeEvalFile(t, dir, "fixtures/input.md", "")
	empty := suiteHashAt(t, dir, "alpha")

	if missing == empty {
		t.Fatalf("missing and empty setup file hash alike (%s)", missing)
	}
	if missing == withContent || empty == withContent {
		t.Fatalf("hash did not change when the setup file was removed or emptied")
	}
}

// TestSuiteSHA256SelfTestSuite: the committed self-test evals hash cleanly.
func TestSuiteSHA256SelfTestSuite(t *testing.T) {
	root := filepath.Join("testdata", "evals")
	entries, err := os.ReadDir(filepath.Join(root, "cases"))
	if err != nil {
		t.Fatal(err)
	}
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n++
		got := suiteHashAt(t, root, e.Name())
		if !hex64.MatchString(got) {
			t.Errorf("agent %s: suiteSHA256 = %q, want 64 lowercase hex characters", e.Name(), got)
		}
	}
	if n == 0 {
		t.Fatal("no agents found under testdata/evals/cases")
	}
}
