package cmd

import (
	"fmt"

	"github.com/jbrinkman/kairon/internal/eval"
	"github.com/spf13/cobra"
)

var (
	verifyLogIterationsDir string
	verifyLogMinIterations int
)

var verifyLogCmd = &cobra.Command{
	Use:   "verify-log <agent>",
	Short: "Verify an agent's iteration logs against recorded eval results",
	Long: `Verify <iterations-dir>/<agent>/iteration-NN.md against the eval results
committed under <evals-dir>/results.

Every violation is reported as "<file>: <rule>: <message>". The command exits
non-zero if any rule fails.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Arguments are parsed; from here an error is a verification result,
		// not a usage mistake.
		cmd.SilenceUsage = true

		if verifyLogMinIterations < 0 {
			return fmt.Errorf("--min-iterations must not be negative (got %d)", verifyLogMinIterations)
		}

		return eval.VerifyLog(cmd.OutOrStdout(), args[0], eval.VerifyLogOptions{
			IterationsDir: verifyLogIterationsDir,
			EvalsDir:      evalEvalsDir,
			MinIterations: verifyLogMinIterations,
		})
	},
}

func init() {
	verifyLogCmd.Flags().StringVar(&verifyLogIterationsDir, "iterations-dir", eval.DefaultIterationsDir,
		"Directory holding <agent>/iteration-NN.md logs")
	verifyLogCmd.Flags().IntVar(&verifyLogMinIterations, "min-iterations", eval.DefaultMinIterations,
		"Minimum number of iteration logs required (0 disables the minimum)")

	evalCmd.AddCommand(verifyLogCmd)
}
