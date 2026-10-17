package cmd

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const auditFixtures = "../../../internal/eval/testdata/audit"

// useShippedSkills points the audit command at the real shipped templates on
// disk. Package cmd tests never call SetTemplates, so the embedded Templates
// FS is empty here; this seam stands in for it without importing package main.
func useShippedSkills(t *testing.T) {
	t.Helper()
	// Absolute: tests chdir to the repo root after this, and DirFS resolves lazily.
	dir, err := filepath.Abs("../templates/kiro/skills")
	if err != nil {
		t.Fatal(err)
	}
	orig := auditSkillsFS
	auditSkillsFS = func() (fs.FS, error) { return os.DirFS(dir), nil }
	t.Cleanup(func() { auditSkillsFS = orig })
}

// runAudit runs the audit command with explicit flag values, restoring the
// package flag vars afterwards. Empty strings leave the defaults in place.
func runAudit(t *testing.T, agent, evalsDir, itersDir, promptFile string) (string, error) {
	t.Helper()
	origIters, origPrompt, origEvals := auditIterationsDir, auditPromptFile, evalEvalsDir
	t.Cleanup(func() {
		auditIterationsDir, auditPromptFile, evalEvalsDir = origIters, origPrompt, origEvals
		auditCmd.SilenceUsage = false
		auditCmd.SetOut(nil)
	})
	if itersDir != "" {
		auditIterationsDir = itersDir
	}
	auditPromptFile = promptFile
	evalEvalsDir = evalsDir

	var out bytes.Buffer
	auditCmd.SetOut(&out)
	err := auditCmd.RunE(auditCmd, []string{agent})
	return out.String(), err
}

func runAuditFixture(t *testing.T, set string) (string, error) {
	t.Helper()
	return runAudit(t, "demo", auditFixtures+"/"+set+"/evals", auditFixtures+"/"+set+"/iterations", "")
}

// runAuditFlatFixture audits agent "selftest" against one of the flat-layout
// fixtures, where --iterations-dir and --evals-dir are the same directory
// (the form used by the issue #338 verify commands).
func runAuditFlatFixture(t *testing.T, set string) (string, error) {
	t.Helper()
	dir := auditFixtures + "/" + set
	return runAudit(t, "selftest", dir, dir, "")
}

func TestAuditFlatFixtureEvidenceAndDuplicates(t *testing.T) {
	const table = "selftest/load-bearing.md"
	const prompt = "agents/selftest-prompt.md"

	tests := []struct {
		fixture string
		// wantErr must all appear in the returned error text.
		wantErr []string
		// wantOut must all appear on stdout.
		wantOut []string
	}{
		{
			// Row 3 has an empty Evidence cell, row 4 has text without case:.
			fixture: "evidence-missing",
			wantErr: []string{
				table + ":3: table:",
				table + ":4: table:",
				"Evidence must name at least one case",
				"case:<case-name>",
			},
			wantOut: []string{"Audit: selftest"},
		},
		{
			fixture: "evidence-unknown-case",
			wantErr: []string{
				table + ":4: table:",
				`"ghost"`,
				"not a case of agent",
			},
			wantOut: []string{"Audit: selftest"},
		},
		{
			fixture: "evidence-unchecked-criterion",
			wantErr: []string{
				table + ":5: table:",
				`"alpha-only"`,
				`"beta"`,
				"no check on criterion",
			},
			wantOut: []string{"Audit: selftest"},
		},
		{
			// The repeat is on line 9 and refers to the first occurrence on line 8.
			fixture: "duplicate-line",
			wantErr: []string{
				prompt + ":9: duplicate:",
				"duplicate of",
				prompt + ":8:",
				"Never skip validation.",
			},
			wantOut: []string{"duplicates=1", "total=5", "mapped=5"},
		},
		{
			// Two non-cost criteria plus a type: cost entry: the rubric is
			// named as the violation's file and the cost entry is not counted.
			fixture: "two-criteria",
			wantErr: []string{
				"rubrics/selftest.yaml: rubric:",
				"2 distinct non-cost criteria",
				"need at least 3",
			},
			wantOut: []string{"Criteria: non-cost=2 (need at least 3)", "alpha: checks=1 cases=1"},
		},
		{
			fixture: "unchecked-criterion",
			wantErr: []string{
				"rubrics/selftest.yaml: unchecked:",
				`"gamma"`,
				"no check in any case",
			},
			wantOut: []string{"gamma: checks=0 cases=0"},
		},
		{
			fixture: "identical-criteria",
			wantErr: []string{
				"rubrics/selftest.yaml: identical:",
				`"beta"`,
				`"gamma"`,
				"same checks",
			},
			wantOut: []string{"Criteria: non-cost=3", "beta: checks=1 cases=1", "gamma: checks=1 cases=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			out, err := runAuditFlatFixture(t, tt.fixture)
			if err == nil {
				t.Fatalf("%s: expected a non-nil audit error, got nil\noutput:\n%s", tt.fixture, out)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error missing %q:\n%s", want, err)
				}
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out, want) {
					t.Errorf("stdout missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestAuditHelpDescribesEvidenceAndDuplicates(t *testing.T) {
	long := auditCmd.Long
	for _, want := range []string{"case:<case-name>", "Evidence", "Repeated instructions fail"} {
		if !strings.Contains(long, want) {
			t.Errorf("audit help missing %q:\n%s", want, long)
		}
	}
}

func TestAuditThreeCriteriaOKFixturePasses(t *testing.T) {
	out, err := runAuditFlatFixture(t, "three-criteria-ok")
	if err != nil {
		t.Fatalf("three-criteria-ok: unexpected error: %v\noutput:\n%s", err, out)
	}
	for _, want := range []string{
		"Criteria: non-cost=3 (need at least 3)",
		"alpha: checks=3 cases=2",
		"beta: checks=1 cases=1",
		"gamma: checks=1 cases=1",
		"OK selftest",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
	// The type: cost criterion is never listed.
	if strings.Contains(out, "cost:") {
		t.Errorf("cost criterion must not be listed:\n%s", out)
	}
}

func TestAuditHelpDescribesRubricRules(t *testing.T) {
	long := auditCmd.Long
	for _, want := range []string{
		"at least three non-cost criteria",
		"no check in any case",
		"identical checks",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("audit help missing %q:\n%s", want, long)
		}
	}
}

func TestAuditFlagsRegistered(t *testing.T) {
	found := false
	for _, c := range evalCmd.Commands() {
		if c == auditCmd {
			found = true
		}
	}
	if !found {
		t.Fatal("audit is not registered on eval")
	}
	if auditCmd.Name() != "audit" {
		t.Errorf("command name = %q, want audit", auditCmd.Name())
	}
	if got, _, err := evalCmd.Find([]string{"audit", "builder"}); err != nil || got != auditCmd {
		t.Errorf("eval audit routes to %v (err %v), want auditCmd", got, err)
	}

	iters := auditCmd.Flags().Lookup("iterations-dir")
	if iters == nil {
		t.Fatal("--iterations-dir flag not registered")
	}
	if iters.DefValue != ".kairon/iterations" {
		t.Errorf("--iterations-dir default = %q, want .kairon/iterations", iters.DefValue)
	}
	if auditCmd.LocalFlags().Lookup("prompt-file") == nil {
		t.Error("--prompt-file must be a local flag of audit")
	}
	if auditCmd.LocalFlags().Lookup("evals-dir") != nil {
		t.Error("--evals-dir must be inherited from eval, not redefined")
	}
	if auditCmd.InheritedFlags().Lookup("evals-dir") == nil {
		t.Error("audit does not inherit --evals-dir")
	}
}

func TestAuditArgs(t *testing.T) {
	if err := auditCmd.Args(auditCmd, nil); err == nil {
		t.Error("expected arg error when agent is missing")
	}
	if err := auditCmd.Args(auditCmd, []string{"a", "b"}); err == nil {
		t.Error("expected arg error for extra arguments")
	}
	if err := auditCmd.Args(auditCmd, []string{"builder"}); err != nil {
		t.Errorf("one argument should be accepted: %v", err)
	}
}

func TestAuditSilencesUsageOnAuditFailure(t *testing.T) {
	auditCmd.SilenceUsage = false
	if _, err := runAuditFixture(t, "coverage-89"); err == nil {
		t.Fatal("expected audit failure")
	}
	if !auditCmd.SilenceUsage {
		t.Error("usage must be silenced once arguments are parsed")
	}
}

func TestAuditCoverage90Passes(t *testing.T) {
	out, err := runAuditFixture(t, "coverage-90")
	if err != nil {
		t.Fatalf("90%% fixture: unexpected error: %v", err)
	}
	for _, want := range []string{"total=10 mapped=9", "90.0%", "OK demo"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func TestAuditCoverage89Fails(t *testing.T) {
	out, err := runAuditFixture(t, "coverage-89")
	if err == nil {
		t.Fatal("89% fixture: expected failure")
	}
	if !strings.Contains(err.Error(), "coverage") {
		t.Errorf("error %q does not name the coverage rule", err.Error())
	}
	if !strings.Contains(out, "89.0%") {
		t.Errorf("output %q does not show 89.0%%", out)
	}
}

func TestAuditPromptFileFlagIsPassedThrough(t *testing.T) {
	// The candidate has no RTCC headings in the fixture's live prompt only if
	// the flag is ignored; a missing candidate file proves the flag is wired.
	_, err := runAudit(t, "demo", auditFixtures+"/coverage-90/evals", auditFixtures+"/coverage-90/iterations", "does-not-exist.md")
	if err == nil || !strings.Contains(err.Error(), "does-not-exist.md") {
		t.Fatalf("err = %v, want an error naming the missing candidate file", err)
	}
}

func TestAuditSkillsFSWiredFromTemplates(t *testing.T) {
	sub, err := auditSkillsFS()
	if err != nil {
		t.Fatalf("auditSkillsFS: %v", err)
	}
	// In package cmd tests Templates is the zero embed.FS: the sub-FS must be
	// usable and simply have nothing in it, not fail.
	if _, err := fs.ReadFile(sub, "sentinel-protocol/SKILL.md"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("reading from sub FS: %v, want nil or ErrNotExist", err)
	}
}

// The real-repo tests run from the repository root and assert only on the
// resource listing and structure, so they hold whether or not a
// load-bearing table exists.
func TestAuditRealBuilderListsSkillsAndHooks(t *testing.T) {
	useShippedSkills(t)
	t.Chdir("../../..")
	out, _ := runAudit(t, "builder", "", "", "")

	audited, hooks, ok := strings.Cut(out, "Override hooks")
	if !ok {
		t.Fatalf("no Override hooks section in output:\n%s", out)
	}
	if !strings.Contains(audited, "Audited resources:") || !strings.Contains(audited, "skill sentinel-protocol") {
		t.Errorf("sentinel-protocol is not listed as audited:\n%s", out)
	}
	if !strings.Contains(hooks, "skill builder-conventions") {
		t.Errorf("builder-conventions is not listed as an override hook:\n%s", out)
	}
	if strings.Contains(audited, "builder-conventions") {
		t.Errorf("builder-conventions must not be audited:\n%s", out)
	}
}

func TestAuditRealPlannerNamesMissingHeadings(t *testing.T) {
	useShippedSkills(t)
	t.Chdir("../../..")
	_, err := runAudit(t, "planner", "", "", "")
	if err == nil {
		t.Fatal("planner: expected audit failure")
	}
	for _, h := range []string{"## Role", "## Task", "## Context", "## Constraints"} {
		if !strings.Contains(err.Error(), h) {
			t.Errorf("error does not name missing heading %q:\n%s", h, err)
		}
	}
}
