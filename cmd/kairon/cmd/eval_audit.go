package cmd

import (
	"io/fs"

	"github.com/jbrinkman/kairon/internal/eval"
	"github.com/spf13/cobra"
)

var (
	auditIterationsDir string
	auditPromptFile    string
)

// auditSkillsFS returns the shipped engine skills, rooted so "<name>/SKILL.md"
// resolves. It is a variable so tests can substitute the templates, which are
// embedded in package main and are empty in package cmd tests.
var auditSkillsFS = func() (fs.FS, error) {
	return fs.Sub(Templates, "templates/kiro/skills")
}

var auditCmd = &cobra.Command{
	Use:   "audit <agent>",
	Short: "Audit an agent prompt for RTCC structure and load-bearing coverage",
	Long: `Audit an agent's prompt for RTCC structure (## Role, ## Task, ## Context,
## Constraints) and check that at least 90% of its instructions are
load-bearing, i.e. mapped to a rubric criterion in
<iterations-dir>/<agent>/load-bearing.md.

Every row's Evidence cell in the table must name a case of the agent as
case:<case-name>; the case must exist in <evals-dir>/cases/<agent>/ and have a
check on the row's Criterion. Repeated instructions fail the audit: a line whose
text repeats an earlier instruction in the prompt or its audited skills is a
"duplicate" violation.

Engine skills listed in the agent's resources are audited together with the
prompt; *-conventions skills are override hooks and are excluded. The audit is
deterministic and makes no model calls.

Every violation is reported as "<file>[:<line>]: <rule>: <message>". The
command exits non-zero if any rule fails.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Arguments are parsed; from here an error is an audit result,
		// not a usage mistake.
		cmd.SilenceUsage = true

		skills, err := auditSkillsFS()
		if err != nil {
			return err
		}

		return eval.Audit(cmd.OutOrStdout(), args[0], eval.AuditOptions{
			IterationsDir: auditIterationsDir,
			EvalsDir:      evalEvalsDir,
			PromptFile:    auditPromptFile,
			SkillsFS:      skills,
		})
	},
}

func init() {
	auditCmd.Flags().StringVar(&auditIterationsDir, "iterations-dir", eval.DefaultIterationsDir,
		"Directory holding <agent>/load-bearing.md")
	auditCmd.Flags().StringVar(&auditPromptFile, "prompt-file", "",
		"Audit this candidate prompt file in place of the agent's prompt")

	evalCmd.AddCommand(auditCmd)
}
