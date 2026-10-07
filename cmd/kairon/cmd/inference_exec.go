package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/jbrinkman/kairon/internal/inference"
)

var inferenceExecBackend string

// inferenceExecCmd is an internal protocol between the eval harness and its
// own binary: the container sandbox runs it inside the container so a
// non-kiro-cli inference backend executes there. It reads one JSON
// inference.Request on stdin and writes exactly one JSON inference.ExecEnvelope
// on stdout. Nothing else may be written to stdout: it is hidden from help and
// usage/error output is suppressed here (main prints the error to stderr).
var inferenceExecCmd = &cobra.Command{
	Use:           "inference-exec",
	Short:         "Run an inference backend on a JSON request from stdin (internal)",
	Hidden:        true,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		// InOrStdin/OutOrStdout default to os.Stdin/os.Stdout.
		return inference.ServeExec(ctx, inferenceExecBackend, cmd.InOrStdin(), cmd.OutOrStdout())
	},
}

func init() {
	inferenceExecCmd.Flags().StringVar(&inferenceExecBackend, "backend", inference.NameKiroCLI,
		"Inference backend to run")
	rootCmd.AddCommand(inferenceExecCmd)
}
