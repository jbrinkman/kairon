package cmd

import (
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/eval"
	"github.com/jbrinkman/kairon/internal/inference"
	"github.com/spf13/cobra"
)

var (
	evalList           bool
	evalResume         bool
	evalCase           string
	evalPerf           bool
	evalSandbox        bool
	evalNoSandbox      bool
	evalResourceLimit  []string
	evalDebug          bool
	evalCleanup        bool
	evalBackend        string
	evalEvalsDir       string
	evalKeepWorkspaces bool
	evalPromptFile     string
	evalRepeat         int
	evalFilter         []string
	evalStrict         bool
)

var evalCmd = &cobra.Command{
	Use:   "eval [agent] [testcase]",
	Short: "Run evaluations or show diff between runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reject a bad --repeat before any work (and before SilenceUsage, so
		// usage is shown as for other argument mistakes). The check lives here
		// because eval.RunOptions.Repeat == 0 must keep meaning "default 1" for
		// programmatic callers.
		if evalRepeat < 1 {
			return fmt.Errorf("--repeat must be at least 1 (got %d)", evalRepeat)
		}

		// Reject a malformed --filter the same way, with or without
		// --strict. RunWithOptions parses the expressions again; this early
		// check is what keeps the usage visible for the mistake.
		if _, err := eval.ParseFilters(evalFilter); err != nil {
			return err
		}

		// Arguments are parsed; from here an error is a run result (for
		// example an agent below its pass threshold), not a usage mistake.
		cmd.SilenceUsage = true

		var agent, testcase string
		if len(args) > 0 {
			agent = args[0]
		}
		if len(args) > 1 {
			testcase = args[1]
		}

		// Use --case flag if provided
		if evalCase != "" {
			testcase = evalCase
		}

		// Parse resource limits
		resourceLimits := make(map[string]string)
		for _, limit := range evalResourceLimit {
			parts := strings.SplitN(limit, "=", 2)
			if len(parts) == 2 {
				resourceLimits[parts[0]] = parts[1]
			}
		}

		return eval.RunWithOptions(agent, testcase, eval.RunOptions{
			List:           evalList,
			Resume:         evalResume,
			Sandbox:        evalSandbox,
			NoSandbox:      evalNoSandbox,
			ResourceLimit:  resourceLimits,
			Debug:          evalDebug,
			Cleanup:        evalCleanup,
			Perf:           evalPerf,
			Backend:        evalBackend,
			EvalsDir:       evalEvalsDir,
			KeepWorkspaces: evalKeepWorkspaces,
			PromptFile:     evalPromptFile,
			Repeat:         evalRepeat,
			Filters:        evalFilter,
			Strict:         evalStrict,
		})
	},
}

var diffCmd = &cobra.Command{
	Use:   "diff <runA> <runB>",
	Short: "Compare two evaluation runs",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return eval.DiffWithOptions(args[0], args[1], eval.RunOptions{EvalsDir: evalEvalsDir, Filters: evalFilter})
	},
}

func init() {
	evalCmd.Flags().BoolVar(&evalList, "list", false, "List available test cases for the agent")
	evalCmd.Flags().BoolVar(&evalResume, "resume", false, "Resume interrupted evaluation from last completed test")
	evalCmd.Flags().StringVar(&evalCase, "case", "", "Run specific test case")
	evalCmd.Flags().BoolVar(&evalPerf, "perf", false, "Run performance investigation and profiling")
	evalCmd.Flags().BoolVar(&evalSandbox, "sandbox", false, "Enable container sandboxing for agent execution")
	evalCmd.Flags().BoolVar(&evalNoSandbox, "no-sandbox", false, "Explicitly disable container sandboxing")
	evalCmd.Flags().StringSliceVar(&evalResourceLimit, "resource-limit", nil, "Override resource limits (cpu=1.0, memory=1073741824, timeout=5m)")
	evalCmd.Flags().BoolVarP(&evalDebug, "debug", "d", false, "Enable debug mode with verbose logging and container persistence")
	evalCmd.Flags().BoolVar(&evalCleanup, "cleanup", false, "Stop and remove all tracked debug containers and clean artifacts")
	evalCmd.Flags().StringVar(&evalBackend, "backend", inference.NameKiroCLI,
		fmt.Sprintf("Inference backend for agent and judge calls (%s)", strings.Join(inference.Names(), ", ")))
	evalCmd.Flags().BoolVar(&evalKeepWorkspaces, "keep-workspaces", false, "Keep per-case workspaces after the run (their paths are recorded as workspace_dir in the results)")
	evalCmd.Flags().StringVar(&evalPromptFile, "prompt-file", "",
		"Evaluate a candidate prompt file in place of the agent's prompt, in per-case workspaces only (requires an agent; the live .kiro/agents is not modified)")
	evalCmd.Flags().IntVar(&evalRepeat, "repeat", 1,
		"Run every case N times, each in a fresh workspace; the agent score is the mean over runs, with min/max reported (must be at least 1)")
	evalCmd.Flags().BoolVar(&evalStrict, "strict", false,
		"Fail when --filter matches no case (default: warn and exit 0)")
	evalCmd.PersistentFlags().StringArrayVar(&evalFilter, "filter", nil,
		"Select cases by metadata: <key><op><value> with op one of ==, <>, !=, <, <=, >, >= (e.g. speed==fast,medium); repeat to require all. Also narrows eval diff and eval trend (built-in keys: agent, name)")
	evalCmd.PersistentFlags().StringVar(&evalEvalsDir, "evals-dir", "",
		"Evals directory holding rubrics, cases, fixtures and results (default \".kairon/evals\")")

	evalCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(evalCmd)
}
