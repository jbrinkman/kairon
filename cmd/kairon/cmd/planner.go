package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/github"
	"github.com/jbrinkman/kairon/internal/planner"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// plannerNewStore builds the store that creates the issues. Tests replace it
// with a fake so no test reaches the real gh.
var plannerNewStore = func(repo string, labels []string) provenance.IssueStore {
	return github.IssueStore{Repo: repo, Labels: labels}
}

// plannerNewIssues builds the GitHub port the default router punches out
// through. Interactive submissions create issues, so nothing is edited under
// the planner and the router is not reached; it is wired so a conflict is
// never an ErrUnroutable.
var plannerNewIssues = func() failure.Issues { return github.CLI{} }

// plannerKnownKey is the shape of a --known key: the boundary numbers a split
// chain issue-1, issue-2, and so on.
var plannerKnownKey = regexp.MustCompile(`^issue-[1-9][0-9]*$`)

var plannerCmd = &cobra.Command{
	Use:   "planner",
	Short: "Planner output utilities",
	Long: `Utilities for the output document the planner agent returns.

The planner never writes to GitHub. Its final reply is a stage-output document
(docs/stage-output.md); "kairon planner submit" runs that reply through the
output boundary and creates the issue.`,
}

var plannerSubmitCmd = &cobra.Command{
	Use:   "submit [--file <file>] --repo <owner/name> [flags]",
	Short: "Submit interactive planner replies as GitHub issues",
	Long: `Read one or more planner replies, run each through the output boundary (extract,
mode contract, guard chain) and create one GitHub issue per document.

Replies come from --file, or from stdin when --file is not given. Separate
several documents with a line containing only ---, in dependency order. Every
reply is checked before anything is written, so a bad later reply leaves no
earlier issue behind.

A later document may depend on an earlier one in the same input with
#{issue-1}, #{issue-2}, and so on. To submit a later document on its own, pass
the issues already created with --known issue-1=412 (repeatable); the numbers
continue after them.

On success one line per document is printed:

  committed issue-1 -> #412

On rejection the guard's Expected and Actual text are printed and the command
exits 1. Nothing is written for a rejected reply.

--dry-run runs extraction, the mode contract and the guard chain only. It
writes nothing, needs no --repo and does not run gh.

Issues get no label unless --label is given (repeatable). This command is for
interactive planning only.`,
	Args: cobra.NoArgs,
	RunE: runPlannerSubmit,
}

func init() {
	f := plannerSubmitCmd.Flags()
	f.String("file", "", "File holding the replies (default: stdin)")
	f.String("repo", "", "Target repository as owner/name (required unless --dry-run)")
	f.StringArray("label", nil, "Label to apply to each created issue (repeatable; default: none)")
	f.StringArray("known", nil, "Issue already created in this split, as issue-N=<number> (repeatable)")
	f.Bool("dry-run", false, "Check the replies without writing anything")
	plannerCmd.AddCommand(plannerSubmitCmd)
	rootCmd.AddCommand(plannerCmd)
}

func runPlannerSubmit(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	file, _ := f.GetString("file")
	repo, _ := f.GetString("repo")
	labels, _ := f.GetStringArray("label")
	knownArgs, _ := f.GetStringArray("known")
	dryRun, _ := f.GetBool("dry-run")

	known, err := parseKnown(knownArgs)
	if err != nil {
		return err
	}
	if repo == "" && !dryRun {
		return fmt.Errorf("--repo is required (owner/name), unless --dry-run")
	}

	// Arguments are parsed; from here an error is a result, not a usage mistake.
	// main prints the returned error, so cobra stays quiet to avoid a repeat.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	var raw []byte
	if file != "" {
		raw, err = os.ReadFile(file)
	} else {
		raw, err = io.ReadAll(cmd.InOrStdin())
	}
	if err != nil {
		return fmt.Errorf("read replies: %w", err)
	}
	replies := splitReplies(string(raw))
	if len(replies) == 0 {
		return fmt.Errorf("no replies in the input")
	}

	req := planner.Request{Mode: planner.ModeInteractive, Replies: replies, Known: known}
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()

	if dryRun {
		res, err := (&planner.Boundary{}).Check(req)
		if err != nil {
			return fmt.Errorf("check replies: %w", err)
		}
		if res.Outcome == planner.OutcomeRejected {
			return reportPlannerRejection(errOut, res, len(replies))
		}
		fmt.Fprintf(out, "dry run: %d reply(ies) pass the output boundary; nothing was written\n", len(replies))
		return nil
	}

	b := &planner.Boundary{
		Store: plannerNewStore(repo, labels),
		Router: &planner.DefaultRouter{
			Issues:    plannerNewIssues(),
			Repo:      repo,
			FailLabel: failure.FailedLabel("kairon"),
			RunID:     failure.NewRunID(),
			Log:       errOut,
		},
		Issues: plannerNewIssues(),
	}
	res, err := b.Handle(cmd.Context(), req)

	// Print what was written first, even on failure, so the user can resume
	// with --known.
	for _, c := range res.Committed {
		fmt.Fprintf(out, "committed %s -> #%d\n", c.Key, c.Result.Number)
	}
	if err != nil {
		return fmt.Errorf("submit replies: %w", err)
	}
	switch res.Outcome {
	case planner.OutcomeCommitted:
		return nil
	case planner.OutcomeRejected:
		return reportPlannerRejection(errOut, res, len(replies))
	default:
		return fmt.Errorf("submit stopped at reply %d: %s", res.Index+1, res.Outcome)
	}
}

// reportPlannerRejection prints the guard's Expected and Actual text and
// returns the error that makes the exit code non-zero.
func reportPlannerRejection(w io.Writer, res planner.Result, total int) error {
	fc := res.Failure
	if fc == nil {
		return fmt.Errorf("reply %d of %d rejected", res.Index+1, total)
	}
	fmt.Fprintf(w, "reply %d of %d rejected by %s\n", res.Index+1, total, fc.Guard)
	fmt.Fprintf(w, "Expected: %s\n", fc.Expected)
	fmt.Fprintf(w, "Actual: %s\n", fc.Actual)
	if len(res.Committed) > 0 {
		fmt.Fprintf(w, "Replies before it were committed; resume with --known for them.\n")
	}
	return fmt.Errorf("reply %d of %d rejected by %s", res.Index+1, total, fc.Guard)
}

// parseKnown turns --known issue-N=<number> arguments into a map.
func parseKnown(args []string) (map[string]int, error) {
	if len(args) == 0 {
		return nil, nil
	}
	known := make(map[string]int, len(args))
	for _, a := range args {
		key, val, ok := strings.Cut(a, "=")
		if !ok {
			return nil, fmt.Errorf("--known %q: want issue-N=<number>", a)
		}
		if !plannerKnownKey.MatchString(key) {
			return nil, fmt.Errorf("--known %q: key %q is not of the form issue-N", a, key)
		}
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("--known %q: %q is not a positive issue number", a, val)
		}
		if _, dup := known[key]; dup {
			return nil, fmt.Errorf("--known %q: key %q given twice", a, key)
		}
		known[key] = n
	}
	return known, nil
}

// splitReplies splits input into replies at each line that is only "---"
// (trailing whitespace allowed). Empty replies are dropped. An indented ---
// or a longer rule is content, so a document body holding a markdown rule is
// not split.
func splitReplies(input string) []string {
	var replies []string
	var cur []string
	flush := func() {
		if r := strings.TrimSpace(strings.Join(cur, "\n")); r != "" {
			replies = append(replies, r)
		}
		cur = nil
	}
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "---" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return replies
}
