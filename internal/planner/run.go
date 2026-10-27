package planner

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// InvokeGuardName is reported in guard.FailureContext.Guard when the planner
// agent could not be run or exited with an error, so there was no reply to
// check.
const InvokeGuardName = "invoke"

// KiroCommand is the executable the unattended planner runs as.
const KiroCommand = "kiro-cli"

// KiroArgs returns the kiro-cli arguments for one unattended run of agent:
// chat --agent <agent> --no-interactive <prompt>.
//
// It never includes the trust-everything flag. An unattended run is bounded by
// the agent manifest's allowedTools, so a planner can read the repository but
// has no way to widen that on its own.
func KiroArgs(agent, prompt string) []string {
	return []string{"chat", "--agent", agent, "--no-interactive", prompt}
}

// AutomatedCommand is the unattended invocation site: the one place that knows
// the run has no human present, so it always selects AgentAutomated. Nothing
// here chooses between agents; the interactive launch sites name their own.
//
// It returns, without starting it, the command for one intake attempt: `kiro-cli
// chat --agent planner-automated --no-interactive <prompt>` in dir. The caller
// owns the process: it starts it, collects standard output as the reply, strips
// terminal escapes with StripANSI and hands the reply to Boundary.Process. The
// agent manager runs each attempt this way, asynchronously, and owns retries.
func AutomatedCommand(prompt, dir string) *exec.Cmd {
	cmd := exec.Command(KiroCommand, KiroArgs(AgentAutomated, prompt)...)
	cmd.Dir = dir
	return cmd
}

var (
	// csiSequence matches a CSI sequence: colors, cursor moves, erase, and
	// private modes such as ESC[?25l.
	csiSequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	// oscSequence matches an OSC sequence (window title, hyperlinks) ended by
	// BEL or ST.
	oscSequence = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	// charsetSequence matches the two-byte character-set selects ESC ( B and
	// the like.
	charsetSequence = regexp.MustCompile(`\x1b[()][0-9A-Za-z]`)
)

// StripANSI removes terminal escape sequences from s. kiro-cli colors its
// output even without a terminal, and a stage-output document with escape
// bytes in it is not a document.
func StripANSI(s string) string {
	s = oscSequence.ReplaceAllString(s, "")
	s = csiSequence.ReplaceAllString(s, "")
	return charsetSequence.ReplaceAllString(s, "")
}

// AutomatedPrompt is the one message sent to planner-automated for an attempt.
// It names the issue and the attempt, and on a retry says what was wrong with
// the previous reply. It does not describe the output contract: that lives in
// the planner-shared skill, which the agent loads.
func AutomatedPrompt(repo string, issue int, attempt guard.Attempt, prev *guard.FailureContext) string {
	ref := fmt.Sprintf("%s#%d", repo, issue)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Canonicalize GitHub issue %s. Read it with `gh issue view %d --repo %s`. ", ref, issue, repo)
	fmt.Fprintf(&sb, "Use source.kind github_issue and source.ref %s. Reply with the stage-output document and nothing else.", ref)
	fmt.Fprintf(&sb, "\n\nThis is attempt %d of %d.", attempt.Current, attempt.Max)
	if prev != nil {
		fmt.Fprintf(&sb, " Your previous reply was rejected by the %s check.\nExpected: %s\nActual: %s\nReturn a corrected document.",
			prev.Guard, prev.Expected, prev.Actual)
	}
	return sb.String()
}

// RejectionFailure returns the failure to retry or punch out on for a rejected
// Process result. A rejection always carries a failure; if one is somehow
// missing, a synthesized one keeps the caller from reading the rejection as
// success.
func RejectionFailure(out Result, attempt guard.Attempt) guard.FailureContext {
	if out.Failure != nil {
		return *out.Failure
	}
	return guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    ExtractGuardName,
		Attempt:  attempt,
		Expected: "the reply is exactly one stage-output document and nothing else",
		Actual:   "the reply was rejected without a recorded reason",
	}
}

// InvokeFailure is the failure for an attempt whose agent could not be run or
// exited with an error. It is recoverable: the caller retries it like a
// rejected reply.
func InvokeFailure(attempt guard.Attempt, err error) guard.FailureContext {
	return guard.FailureContext{
		Stage:    stageoutput.StagePlanner,
		Guard:    InvokeGuardName,
		Attempt:  attempt,
		Expected: "the planner agent runs and returns a reply",
		Actual:   err.Error(),
	}
}
