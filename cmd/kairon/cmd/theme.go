package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jbrinkman/kairon/internal/config"
)

var themeCmd = &cobra.Command{
	Use:   "theme [name]",
	Short: "Manage UI themes",
	Long:  "Set or display the current UI theme. Run without arguments to show current theme.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			fmt.Printf("Current theme: %s\n", cfg.Theme)
			return nil
		}

		themeName := args[0]
		_, err := config.LoadTheme(themeName)
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		cfg.Theme = themeName
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Theme set to: %s\n", themeName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(themeCmd)
}
