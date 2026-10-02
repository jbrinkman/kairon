package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jbrinkman/kairon/internal/incidents"
)

var logIncidentCmd = &cobra.Command{
	Use:   "log-incident <issue> <attempt> [content]",
	Short: "Log an incident for an issue and attempt",
	Long:  "Log an incident with content. If content is not provided, it will be read from stdin.",
	Args:  cobra.RangeArgs(2, 3),
	RunE:  runLogIncident,
}

func init() {
	rootCmd.AddCommand(logIncidentCmd)
}

func runLogIncident(cmd *cobra.Command, args []string) error {
	issueNum, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue number: %s", args[0])
	}

	attempt, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attempt number: %s", args[1])
	}

	var content string
	if len(args) >= 3 {
		content = args[2]
	} else {
		var err error
		content, err = readFromStdin()
		if err != nil {
			return fmt.Errorf("failed to read content from stdin: %w", err)
		}
	}

	logger, err := incidents.NewIncidentLogger()
	if err != nil {
		return fmt.Errorf("failed to initialize incident logger: %w", err)
	}

	if err := logger.LogIncident(issueNum, attempt, content); err != nil {
		return fmt.Errorf("failed to log incident: %w", err)
	}

	fmt.Printf("Incident logged for issue %d, attempt %d\n", issueNum, attempt)
	return nil
}

func readFromStdin() (string, error) {
	var lines []string
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}
