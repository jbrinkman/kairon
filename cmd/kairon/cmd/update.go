package cmd

import (
	"github.com/jbrinkman/kairon/internal/templates"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update project templates (force overwrite)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return templates.Extract("templates", ".", true)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
