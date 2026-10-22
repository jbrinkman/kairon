package planner

import (
	"fmt"
	"strings"
)

// Mode says whether a human is present while the planner runs.
type Mode int

// Modes. Interactive is the zero value.
const (
	// ModeInteractive: a human is in the conversation. The reply is a
	// canonical document (a punch_out has nobody to comment on) and the
	// boundary creates a new issue per document.
	ModeInteractive Mode = iota
	// ModeAutomated: intake of an existing issue with no human. The reply is
	// exactly one document, canonical or punch_out.
	ModeAutomated
)

// String returns a short name for m.
func (m Mode) String() string {
	switch m {
	case ModeInteractive:
		return "interactive"
	case ModeAutomated:
		return "automated"
	}
	return fmt.Sprintf("mode(%d)", int(m))
}

// AutomatedIntakeLine is the first line of the prompt that selects automated
// mode. The planner treats its absence as interactive mode. The eval cases
// under .kairon/evals/cases/planner use the same line; a test pins them to
// this constant.
const AutomatedIntakeLine = "AUTOMATED INTAKE: non-interactive, no human is available to answer questions. Do not ask questions and do not offer options."

// AutomatedPrompt builds the prompt for an automated intake of issue number
// issue in repo: AutomatedIntakeLine, the rest of the preamble, then the issue
// text.
func AutomatedPrompt(repo string, issue int, title, body string) string {
	var b strings.Builder
	b.WriteString(AutomatedIntakeLine)
	b.WriteString("\n")
	b.WriteString("Read the issue and the repository context below, fill every slot they ground by reasonable inference and record each inference with its evidence.\n")
	b.WriteString("Return exactly one stage-output document as the whole reply, with nothing before or after it.\n")
	b.WriteString("If a required slot cannot be reasonably inferred, return a punch_out document instead of guessing.\n")
	fmt.Fprintf(&b, "Use source kind github_issue with ref \"%d\".\n\n", issue)
	fmt.Fprintf(&b, "Repository: %s\n", repo)
	fmt.Fprintf(&b, "Issue: #%d\n", issue)
	fmt.Fprintf(&b, "Title: %s\n\n", title)
	b.WriteString("Issue body:\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}
