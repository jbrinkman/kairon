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
