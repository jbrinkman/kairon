package cmd

import (
	"github.com/jbrinkman/kairon/internal/eval"
	"github.com/spf13/cobra"
)

var trendJSON bool

var trendCmd = &cobra.Command{
	Use:   "trend <agent>",
	Short: "Show an agent's eval score and attempt success rate across runs",
	Long: `List the runs of <agent> under <evals-dir>/results, oldest first: run name,
date, score, attempt success rate, models, prompt hash and completeness.

Nothing is run: the command only reads recorded results. It exits non-zero if
no run has a result for the agent.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Arguments are parsed; from here an error is a result, not a usage
		// mistake.
		cmd.SilenceUsage = true

		return eval.Trend(cmd.OutOrStdout(), args[0], eval.TrendOptions{
			EvalsDir: evalEvalsDir,
			JSON:     trendJSON,
		})
	},
}

func init() {
	trendCmd.Flags().BoolVar(&trendJSON, "json", false, "Print the runs as a JSON array")

	evalCmd.AddCommand(trendCmd)
}
