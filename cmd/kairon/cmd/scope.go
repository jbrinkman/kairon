package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jbrinkman/kairon/internal/boundary"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/plan"
)

// Exit codes of kairon scope check.
const (
	scopeExitFindings  = 1
	scopeExitCannotRun = 3
)

// ExitError makes main exit with Code instead of the default 1.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

var (
	scopeInput string
	scopeSpec  string
)

var scopeCmd = &cobra.Command{
	Use:   "scope",
	Short: "Check that a spec covers its issue's scope",
}

var scopeCheckCmd = &cobra.Command{
	Use:   "check --input <input.yaml> --spec <spec.md>",
	Short: "Check a spec's Scope Coverage table and Deferred work section",
	Long: `Read the issue body from a verified input document and check the spec against it.

Every In Scope item (s-N) and acceptance criterion (ac-N) must have a row in the
spec's "## Scope Coverage" table that quotes the item's text and names, in
backticks in its last cell, at least one task ID from the spec's kiro-plan block.
Every task ID a row names must exist. The spec must have a "## Deferred work"
section whose only content is none.

The check is presence-only and deterministic. It does not judge whether the
tasks deliver an item, and it does not scan for deferral language: the
validator judges that.

Prints one JSON object: {"status":"pass"} or {"status":"fail","findings":[...]}.
Exits 0 on pass, 1 when there is at least one finding, and 3 when the check
cannot run (a usage error, a missing file, or an input document that does not
verify).`,
	// A usage error means the check did not run, so it exits 3 like any other
	// run that could not start; exit 1 is kept for findings.
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return scopeCannotRun(err)
		}
		return nil
	},
	RunE: runScopeCheckCommand,
}

func scopeCannotRun(err error) error { return &ExitError{Code: scopeExitCannotRun, Err: err} }

type scopeResult struct {
	Status   string               `json:"status"`
	Findings []guard.ScopeFinding `json:"findings,omitempty"`
}

func runScopeCheckCommand(cmd *cobra.Command, _ []string) error {
	// Arguments are parsed; from here an error is a result, not a usage
	// mistake. main prints the error, so cobra does not print it again.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cannotRun := scopeCannotRun
	// The flags are checked here, not with MarkFlagRequired, whose error cobra
	// raises before RunE as a plain error that exits 1.
	if scopeInput == "" || scopeSpec == "" {
		return cannotRun(fmt.Errorf("--input and --spec are required"))
	}
	data, err := os.ReadFile(scopeInput)
	if err != nil {
		return cannotRun(fmt.Errorf("--input: %w", err))
	}
	doc, err := boundary.ParseInputDocument(data)
	if err != nil {
		return cannotRun(fmt.Errorf("--input %s does not verify: %w", scopeInput, err))
	}
	spec, err := os.ReadFile(scopeSpec)
	if err != nil {
		return cannotRun(fmt.Errorf("--spec: %w", err))
	}

	// A missing or malformed plan leaves no known task IDs, so every row's
	// tasks are reported unknown: that is a spec defect the architect fixes,
	// not a reason the check cannot run. A plan finding comes first so the
	// architect sees the cause of the unknown_task findings.
	var taskIDs []string
	var findings []guard.ScopeFinding
	planFinding := func(text string) guard.ScopeFinding {
		return guard.ScopeFinding{Kind: guard.ScopePlan, Item: "kiro-plan", Text: text}
	}
	p, err := plan.ParsePlanFromMarkdown(spec)
	switch {
	case err != nil:
		findings = append(findings, planFinding(err.Error()))
	case p == nil:
		findings = append(findings, planFinding("missing"))
	default:
		taskIDs = p.GetTaskIDs()
	}

	findings = append(findings, guard.CheckScope(doc.Body, string(spec), taskIDs)...)
	res := scopeResult{Status: "pass"}
	if len(findings) > 0 {
		res = scopeResult{Status: "fail", Findings: findings}
	}
	out, err := json.Marshal(res)
	if err != nil {
		return cannotRun(err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
	if len(findings) > 0 {
		return &ExitError{Code: scopeExitFindings, Err: fmt.Errorf("scope check found %d finding(s)", len(findings))}
	}
	return nil
}

func init() {
	scopeCheckCmd.Flags().StringVar(&scopeInput, "input", "", "Verified input document holding the issue body (required)")
	scopeCheckCmd.Flags().StringVar(&scopeSpec, "spec", "", "Spec file to check (required)")
	scopeCheckCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return scopeCannotRun(err) })

	scopeCmd.AddCommand(scopeCheckCmd)
	rootCmd.AddCommand(scopeCmd)
}
