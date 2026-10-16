package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jbrinkman/kairon/internal/checks"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/inference"
)

// maxCheckGHLogBytes caps what --gh-log reads, like the eval harness: a larger
// log sets Input.GHLogOversized so the gh_log checks fail explicitly.
const maxCheckGHLogBytes = 10 << 20

var (
	checkChecks    string
	checkDir       string
	checkOutput    string
	checkGHLog     string
	checkHiddenDir string
	checkBackend   string
)

// checkNewBackend builds the inference backend answering judge checks; tests
// replace it to record requests.
var checkNewBackend = inference.New

var checkCmd = &cobra.Command{
	Use:   "check --checks <file> --dir <dir>",
	Short: "Run a checks file against a directory",
	Long: `Run a YAML list of checks (the format of an eval case's checks: key; criterion
is optional) against a directory and print one PASS or FAIL line per check.

The whole file is validated before any check runs. The command exits 0 only if
every check passes. Judge checks are answered by --backend, with the model
pinned by evals.judge_model in .kairon/config.yaml (checked against
evals.allowed_models, exactly as in kairon eval); that config is read only when
the file contains a judge check. With --backend stub every judge check is
answered "yes" by a deterministic test double and no model is called.

command checks run "sh -c" on this host, in --dir. Treat a checks file as
trusted input.`,
	Args: cobra.NoArgs,
	RunE: runCheckCommand,
}

func runCheckCommand(cmd *cobra.Command, _ []string) error {
	// Arguments are parsed; from here an error is a result, not a usage mistake.
	cmd.SilenceUsage = true

	backend, err := checkNewBackend(checkBackend)
	if err != nil {
		return err
	}

	dir, err := filepath.Abs(checkDir)
	if err != nil {
		return fmt.Errorf("--dir %s: %w", checkDir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("--dir %s: %w", checkDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--dir %s: not a directory", checkDir)
	}

	var in checks.Input
	in.Dir = dir
	if checkOutput != "" {
		if in.Output, err = readCheckFile("--output", checkOutput); err != nil {
			return err
		}
	}
	if checkGHLog != "" {
		if in.GHLog, in.GHLogOversized, err = readCheckGHLog(checkGHLog); err != nil {
			return err
		}
	}
	hidden := ""
	if checkHiddenDir != "" {
		hidden, err = filepath.Abs(checkHiddenDir)
		if err != nil {
			return fmt.Errorf("--hidden-dir %s: %w", checkHiddenDir, err)
		}
		hi, err := os.Stat(hidden)
		if err != nil {
			return fmt.Errorf("--hidden-dir %s: %w", checkHiddenDir, err)
		}
		if !hi.IsDir() {
			return fmt.Errorf("--hidden-dir %s: not a directory", checkHiddenDir)
		}
	}

	// Load and validate everything before any check runs.
	list, err := checks.LoadFile(checkChecks)
	if err != nil {
		return err
	}
	if err := checks.Validate(checkChecks, list, checks.ValidateOptions{
		HiddenDir:         hidden,
		OptionalCriterion: true,
	}); err != nil {
		return err
	}

	// Pin the judge model like kairon eval, but only when a judge check exists.
	if hasJudgeCheck(list) {
		ev, err := config.LoadEvals()
		if err != nil {
			return fmt.Errorf("❌ cannot pin the judge model: %w", err)
		}
		if err := ev.CheckModel(ev.JudgeModel); err != nil {
			return fmt.Errorf("❌ eval model pinning refused the run before any check started:\n  - evals.judge_model: %v\n"+
				"Edit the evals block in .kairon/config.yaml (judge_model, allowed_models)", err)
		}
		judge := checks.InferenceJudge{Backend: backend, Model: ev.JudgeModel}
		if backend.Name() == inference.NameStub {
			// Test double: answer every judge check "yes" with no model call.
			judge.Stub = &inference.StubScript{Judge: inference.StubJudge{"yes"}}
		}
		in.Judge = judge.Func()
	}

	results := checks.Evaluate(list, in)

	out := cmd.OutOrStdout()
	failed := 0
	for _, r := range results {
		fmt.Fprintln(out, formatCheckResult(r))
		if !r.Passed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d checks failed", failed, len(results))
	}
	return nil
}

func hasJudgeCheck(list []checks.Check) bool {
	for _, c := range list {
		if c.Type == checks.CheckJudge {
			return true
		}
	}
	return false
}

// formatCheckResult renders one result as a single line.
func formatCheckResult(r checks.Result) string {
	label := oneLine(r.Label)
	detail := oneLine(r.Detail)
	switch {
	case r.Passed && detail == "":
		return "PASS " + label
	case r.Passed:
		return "PASS " + label + ": " + detail
	case detail == "":
		return "FAIL " + label
	default:
		return "FAIL " + label + ": " + detail
	}
}

// oneLine collapses all whitespace runs (including newlines) to single spaces.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// readCheckFile reads a regular file named by flag.
func readCheckFile(flag, path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", flag, path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s %s: not a regular file", flag, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", flag, path, err)
	}
	return string(data), nil
}

// readCheckGHLog reads the gh log, capped at maxCheckGHLogBytes. oversized is
// true when the file is larger than the cap (the text is then the prefix).
func readCheckGHLog(path string) (text string, oversized bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("--gh-log %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("--gh-log %s: not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, fmt.Errorf("--gh-log %s: %w", path, err)
	}
	defer f.Close()
	// Read one byte past the cap so an exactly-cap-sized log is not oversized.
	data, err := io.ReadAll(io.LimitReader(f, maxCheckGHLogBytes+1))
	if err != nil {
		return "", false, fmt.Errorf("--gh-log %s: %w", path, err)
	}
	if len(data) > maxCheckGHLogBytes {
		return string(data[:maxCheckGHLogBytes]), true, nil
	}
	return string(data), false, nil
}

func init() {
	checkCmd.Flags().StringVar(&checkChecks, "checks", "", "YAML file holding the list of checks to run (required)")
	checkCmd.Flags().StringVar(&checkDir, "dir", "", "Directory the checks look at; must exist (required)")
	checkCmd.Flags().StringVar(&checkOutput, "output", "", "File whose text output_contains / output_not_contains checks read (default: empty text)")
	checkCmd.Flags().StringVar(&checkGHLog, "gh-log", "", "File whose text gh_log_contains / gh_log_not_contains checks read, capped at 10 MiB (default: empty text)")
	checkCmd.Flags().StringVar(&checkHiddenDir, "hidden-dir", "", "Directory inject entries are resolved against; without it a check using inject is invalid")
	checkCmd.Flags().StringVar(&checkBackend, "backend", inference.NameKiroCLI,
		fmt.Sprintf("Inference backend answering judge checks (%s)", strings.Join(inference.Names(), ", ")))
	_ = checkCmd.MarkFlagRequired("checks")
	_ = checkCmd.MarkFlagRequired("dir")

	rootCmd.AddCommand(checkCmd)
}
