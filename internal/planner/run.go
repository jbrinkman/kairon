package planner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/provenance"
	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// InvokeGuardName is reported in guard.FailureContext.Guard when the planner
// agent could not be run or exited with an error, so there was no reply to
// check.
const InvokeGuardName = "invoke"

// KiroCommand is the executable KiroInvoker runs.
const KiroCommand = "kiro-cli"

// maxStderr bounds how much of a failed run's stderr goes into an error, and
// so into a punch-out comment.
const maxStderr = 2000

// Invoker runs one planner agent and returns its reply. agent is the agent
// name, prompt the single message to send, and dir the working directory. The
// production implementation is KiroInvoker; tests inject a fake.
type Invoker func(ctx context.Context, agent, prompt, dir string) (reply string, err error)

// KiroArgs returns the kiro-cli arguments for one unattended run of agent:
// chat --agent <agent> --no-interactive <prompt>.
//
// It never includes the trust-everything flag. An unattended run is bounded by
// the agent manifest's allowedTools, so a planner can read the repository but
// has no way to widen that on its own.
func KiroArgs(agent, prompt string) []string {
	return []string{"chat", "--agent", agent, "--no-interactive", prompt}
}

// KiroInvoker is the production Invoker. It runs `kiro-cli chat --agent
// <agent> --no-interactive <prompt>` in dir and returns its standard output.
// A non-zero exit is an error that carries the end of stderr.
func KiroInvoker(ctx context.Context, agent, prompt, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, KiroCommand, KiroArgs(agent, prompt)...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(StripANSI(stderr.String()))
		if len(msg) > maxStderr {
			msg = "..." + msg[len(msg)-maxStderr:]
		}
		if msg == "" {
			return "", fmt.Errorf("planner: %s --agent %s: %w", KiroCommand, agent, err)
		}
		return "", fmt.Errorf("planner: %s --agent %s: %w: %s", KiroCommand, agent, err, msg)
	}
	return stdout.String(), nil
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

// AutomatedRequest is one unattended intake run: one existing issue,
// canonicalized by planner-automated.
type AutomatedRequest struct {
	// Repo is the owner/name of the repository holding the issue.
	Repo string
	// Issue is the number of the issue to canonicalize.
	Issue int
	// FailLabel is the fail-state label a punch-out adds (for example
	// "kairon-failed").
	FailLabel string
	// ObservedRevision is the issue revision the planner read. The commit
	// step refuses to write if the issue moved on.
	ObservedRevision string
	// MaxRetries is the configured max_retries: re-runs after the first run.
	MaxRetries int
	// Dir is the working directory for the agent. Empty means the current
	// directory.
	Dir string
}

// AutomatedResult is how an unattended run ended.
type AutomatedResult struct {
	// Outcome is OutcomeCommitted, OutcomeRoutedToIntake, OutcomePunchedOut
	// or OutcomeHalted.
	Outcome Outcome
	// Attempts is how many times the agent was invoked.
	Attempts int
	// Number is the issue written or punched out.
	Number int
	// Hash is the provenance hash, set when committed.
	Hash string
	// Failure is why the run punched out. Nil otherwise.
	Failure *guard.FailureContext
}

// RunAutomated is the unattended invocation site: the one place that knows the
// run has no human present, so it always selects AgentAutomated. Nothing here
// chooses between agents; the interactive launch sites name their own agent.
//
// Each attempt invokes the agent through invoke, strips terminal escape
// sequences from the reply, and hands the reply to b.Process. The attempts
// run under failure.Runner:
//
//   - a recoverable rejection (the reply was not a valid document, or a guard
//     refused it) is retried, as a new attempt, up to req.MaxRetries more
//     times; after that Runner punches out with an explanatory comment and
//     then the fail label;
//   - a failed invocation is treated as a recoverable rejection;
//   - a punch_out reply is terminal. Process has already commented and
//     labeled, so it is not retried and not punched out a second time;
//   - a commit or a routed-to-intake conflict ends the run;
//   - an issue that is already labeled done or failed is not run at all.
//
// b is not modified: its Target is replaced by a copy built from req. A
// non-nil error means the run could not complete (an invalid request, a
// GitHub call that failed, or a cancelled context).
func RunAutomated(ctx context.Context, b *Boundary, invoke Invoker, req AutomatedRequest) (AutomatedResult, error) {
	if err := req.validate(); err != nil {
		return AutomatedResult{}, err
	}
	if b == nil {
		return AutomatedResult{}, fmt.Errorf("%w: no boundary", guard.ErrInvalid)
	}
	if invoke == nil {
		return AutomatedResult{}, fmt.Errorf("%w: no invoker", guard.ErrInvalid)
	}

	bound := *b
	bound.Target = failure.Target{Repo: req.Repo, Issue: req.Issue, FailLabel: req.FailLabel}
	runner := failure.Runner{
		Issues: bound.Issues,
		Target: bound.Target,
		Policy: failure.PolicyFromRetries(req.MaxRetries),
		RunID:  bound.runID(),
	}

	var (
		done    *Result               // set when Process ended the run
		last    *guard.FailureContext // previous rejection, fed back on retry
		fatal   error                 // set when the run cannot continue
		invoked int
	)
	res, err := runner.Run(func(attempt guard.Attempt) failure.StageResult {
		invoked++
		reply, ierr := invoke(ctx, AgentAutomated, automatedPrompt(req, attempt, last), req.Dir)
		if ierr != nil {
			if ctx.Err() != nil {
				fatal = ctx.Err()
				return failure.StageResult{}
			}
			fc := guard.FailureContext{
				Stage:    stageoutput.StagePlanner,
				Guard:    InvokeGuardName,
				Attempt:  attempt,
				Expected: "the planner agent runs and returns a reply",
				Actual:   ierr.Error(),
			}
			last = &fc
			return failure.StageResult{Failure: &fc, Disposition: failure.Recoverable}
		}

		out, perr := bound.Process(ctx, Request{
			Reply:   StripANSI(reply),
			Attempt: attempt,
			Target:  provenance.Target{Number: req.Issue, ObservedRevision: req.ObservedRevision},
		})
		if perr != nil {
			fatal = perr
			return failure.StageResult{}
		}
		if out.Outcome == OutcomeRejected {
			fc := rejectionFailure(out, attempt)
			last = &fc
			return failure.StageResult{Failure: &fc, Disposition: out.Disposition}
		}
		// Committed, routed to intake, or punched out by the planner itself.
		// All three end the run, and none of them goes back to Runner as a
		// failure: a punch_out reply has already commented and labeled.
		done = &out
		return failure.StageResult{}
	})
	if fatal != nil {
		return AutomatedResult{Attempts: invoked}, fatal
	}
	if err != nil {
		return AutomatedResult{Attempts: res.Attempts, Failure: res.Failure}, err
	}

	switch res.Outcome {
	case failure.OutcomeHalted:
		return AutomatedResult{Outcome: OutcomeHalted}, nil
	case failure.OutcomePunchedOut:
		return AutomatedResult{Outcome: OutcomePunchedOut, Attempts: res.Attempts, Number: req.Issue, Failure: res.Failure}, nil
	case failure.OutcomeSucceeded:
		if done == nil {
			return AutomatedResult{}, errors.New("planner: stage succeeded without a result")
		}
		return AutomatedResult{
			Outcome:  done.Outcome,
			Attempts: res.Attempts,
			Number:   done.Number,
			Hash:     done.Hash,
			Failure:  done.Failure,
		}, nil
	}
	return AutomatedResult{}, fmt.Errorf("planner: unexpected runner outcome %v", res.Outcome)
}

func (r AutomatedRequest) validate() error {
	switch {
	case strings.TrimSpace(r.Repo) == "":
		return fmt.Errorf("%w: repo is empty", guard.ErrInvalid)
	case r.Issue <= 0:
		return fmt.Errorf("%w: issue %d is not a positive number", guard.ErrInvalid, r.Issue)
	case strings.TrimSpace(r.FailLabel) == "":
		return fmt.Errorf("%w: fail label is empty", guard.ErrInvalid)
	case strings.TrimSpace(r.ObservedRevision) == "":
		return fmt.Errorf("%w: observed revision is empty; the commit step needs it to detect a concurrent edit", guard.ErrInvalid)
	case r.MaxRetries < 0:
		return fmt.Errorf("%w: max retries %d is negative", guard.ErrInvalid, r.MaxRetries)
	}
	return nil
}

// rejectionFailure returns the failure to hand to Runner for a rejected
// Process result. A rejection always carries a failure; if one is somehow
// missing, a synthesized one keeps Runner from reading the rejection as
// success.
func rejectionFailure(out Result, attempt guard.Attempt) guard.FailureContext {
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

// automatedPrompt is the one message sent to the agent. It names the issue and
// the attempt, and on a retry says what was wrong with the previous reply. It
// does not describe the output contract: that lives in the planner-shared
// skill, which the agent loads.
func automatedPrompt(req AutomatedRequest, attempt guard.Attempt, prev *guard.FailureContext) string {
	ref := fmt.Sprintf("%s#%d", req.Repo, req.Issue)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Canonicalize GitHub issue %s. Read it with `gh issue view %d --repo %s`. ", ref, req.Issue, req.Repo)
	fmt.Fprintf(&sb, "Use source.kind github_issue and source.ref %s. Reply with the stage-output document and nothing else.", ref)
	fmt.Fprintf(&sb, "\n\nThis is attempt %d of %d.", attempt.Current, attempt.Max)
	if prev != nil {
		fmt.Fprintf(&sb, " Your previous reply was rejected by the %s check.\nExpected: %s\nActual: %s\nReturn a corrected document.",
			prev.Guard, prev.Expected, prev.Actual)
	}
	return sb.String()
}
