package cmd

import (
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jbrinkman/kairon/internal/agent"
	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/tui"
	"github.com/jbrinkman/kairon/internal/version"
	"github.com/jbrinkman/kairon/internal/watcher"
)

var Templates embed.FS

var rootCmd = &cobra.Command{
	Use:     "kairon",
	Short:   "Multi-agent development tool",
	Long:    "kairon is a multi-agent development tool that watches GitHub issues and spawns agents to work on them.",
	Version: version.String(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		about, _ := cmd.Flags().GetBool("about")
		if about {
			info := version.Info()
			out := cmd.OutOrStdout()

			fmt.Fprintf(out, "  Version:    %s\n", info["version"])
			fmt.Fprintf(out, "  Build Date: %s\n", info["build_date"])
			fmt.Fprintf(out, "  Commit:     %s\n", version.ShortCommitHash())
			fmt.Fprintf(out, "  Go Version: %s\n", info["go_version"])
			fmt.Fprintf(out, "  Arch:       %s\n", info["arch"])
			os.Exit(0)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		manager := agent.NewManager(cfg)
		w := watcher.New(cfg, manager)

		defer manager.StopAll()
		defer w.Stop()

		return tui.Run(w, manager, cfg)
	},
}

func init() {
	// Cobra automatically adds --version/-v flags when Version is set
	// Customize the version template to show just the version number
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	// Add --about/-a flag
	rootCmd.PersistentFlags().BoolP("about", "a", false, "display comprehensive version information")
}

func SetTemplates(templates embed.FS) {
	Templates = templates
}

func Execute() error {
	return rootCmd.Execute()
}
