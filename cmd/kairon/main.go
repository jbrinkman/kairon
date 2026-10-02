package main

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/jbrinkman/kairon/cmd/kairon/cmd"
	"github.com/jbrinkman/kairon/internal/logging"
	"github.com/jbrinkman/kairon/internal/templates"
)

//go:embed templates
var Templates embed.FS

func main() {
	// Initialize logging subsystem early (inactive state - no handlers)
	// This ensures the logger is available throughout the application lifecycle
	if err := logging.Initialize("info"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize logging subsystem: %v\n", err)
		// Continue execution - this is not a fatal error
	}

	// Normalize the subcommand to lowercase for case-insensitive matching.
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		os.Args[1] = strings.ToLower(os.Args[1])
	}
	cmd.SetTemplates(Templates)
	templates.SetTemplates(Templates)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
