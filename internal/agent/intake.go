package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jbrinkman/kairon/internal/boundary"
	"github.com/jbrinkman/kairon/internal/failure"
	"github.com/jbrinkman/kairon/internal/guard"
	"github.com/jbrinkman/kairon/internal/planner"
	"github.com/jbrinkman/kairon/internal/provenance"
)

// An intake run canonicalizes and stamps one issue whose body is not verified.
// It runs like krew-lead: each attempt is one planner-automated process,
// started without waiting, and a monitor goroutine handles its exit. The reply
// goes to planner.Boundary.Process, which commits the body with its provenance
// marker, rejects it (retried with the failure in the next prompt), or punches
// the issue out. Two callers start runs: the watcher boundary, through
// watcherIntake, and the TUI's validate command, through Validate.

// IntakeOutcome is how an intake run ended. The zero value is not an outcome.
type IntakeOutcome int

// Intake outcomes.
const (
	// IntakeCommitted: the body and its provenance marker were written.
	IntakeCommitted IntakeOutcome = iota + 1
	// IntakePunchedOut: a human was told why (a comment, then the fail label).
	IntakePunchedOut
	// IntakeChanged: the issue was edited during the run; nothing was written.
	IntakeChanged
	// IntakeStopped: the run was stopped; nothing was written.
	IntakeStopped
	// IntakeFailed: the run could not finish (a GitHub call failed).
	IntakeFailed
)

// String returns a short name for o.
func (o IntakeOutcome) String() string {
	switch o {
	case IntakeCommitted:
		return "committed"
	case IntakePunchedOut:
		return "punched out"
	case IntakeChanged:
		return "issue changed during the run"
	case IntakeStopped:
		return "stopped"
	case IntakeFailed:
		return "failed"
	}
	return "unknown"
}

// IntakeResult is what an intake run reports when it ends.
type IntakeResult struct {
	Outcome  IntakeOutcome
	Issue    int
	Attempts int
	Hash     string                // IntakeCommitted only
	Failure  *guard.FailureContext // IntakePunchedOut only
	Err      error                 // IntakeFailed only
}

// intakeRun is the state of one intake run: every attempt for one issue. Its
// mutable fields are guarded by Manager.mu.
type intakeRun struct {
	issue    int
	revision string // read before attempt 1; every commit attempt checks it
	attempt  int    // the current attempt, 1-based
	policy   failure.Policy
	last     *guard.FailureContext // the previous attempt's failure
	stopped  bool
	logFile  *os.File
	done     chan struct{} // closed when the run ends
	result   IntakeResult  // set before done is closed
}

// Paths an intake run keeps under the working directory. The PID marker lets a
// restarted watcher see a run whose process outlived the previous one. The
// committed record is the loop guard: it says intake committed the issue and
// the issue has not been admitted or punched out since.
func intakeLogPath(issue int) string {
	return filepath.Join(".kairon", "logs", fmt.Sprintf("issue-%d-intake.log", issue))
}

func intakePIDPath(issue int) string {
	return filepath.Join(".kairon", "intake", fmt.Sprintf("issue-%d.pid", issue))
}

func intakeCommittedPath(issue int) string {
	return filepath.Join(".kairon", "intake", fmt.Sprintf("issue-%d.committed", issue))
}

// watcherIntake is the boundary.Intake the watcher's boundary uses.
type watcherIntake struct{ m *Manager }

var _ boundary.Intake = watcherIntake{}

// Start implements boundary.Intake. A running run wins over the loop guard, so
// a run that is about to commit is never punched out under it.
func (w watcherIntake) Start(_ string, issue int) (boundary.IntakeStatus, error) {
	if w.m.IntakeActive(issue) {
		return boundary.IntakeRunning, nil
	}
	if w.m.hasCommitted(issue) {
		return boundary.IntakeSpent, nil
	}
	_, started, err := w.m.startIntake(issue)
	if err != nil {
		return 0, err
	}
	if !started {
		return boundary.IntakeRunning, nil
	}
	return boundary.IntakeStarted, nil
}

// IntakeActive reports whether an intake run for issue is going: one this
// manager started, or one whose PID marker names a live process (a run that
// outlived a previous watcher).
func (m *Manager) IntakeActive(issue int) bool {
	return m.OwnsIntake(issue) || livePIDMarker(issue)
}

// OwnsIntake reports whether this manager has an intake run going for issue,
// so it will call the release callback when the run ends.
func (m *Manager) OwnsIntake(issue int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.agents {
		if a.IssueNumber == issue && a.Kind == KindIntake && a.Status == StatusRunning {
			return true
		}
	}
	return false
}

// activeRunLocked reports whether any run for issue is going: krew-lead or
// intake, here or (for intake) in a previous process. m.mu must be held.
func (m *Manager) activeRunLocked(issue int) bool {
	for _, a := range m.agents {
		if a.IssueNumber == issue && a.Status == StatusRunning {
			return true
		}
	}
	return livePIDMarker(issue)
}

// startIntake starts an intake run for issue and returns without waiting for
// it. started is false, with no error, when a run for the issue is already
// going. An error means the run could not start; nothing was written.
func (m *Manager) startIntake(issue int) (run *intakeRun, started bool, err error) {
	m.mu.Lock()
	if m.activeRunLocked(issue) {
		m.mu.Unlock()
		return nil, false, nil
	}
	run = &intakeRun{
		issue:   issue,
		attempt: 1,
		policy:  failure.PolicyFor(m.config, failure.BoundaryImplementation),
		done:    make(chan struct{}),
	}
	a := &Agent{
		ID:          fmt.Sprintf("intake-%d-%d", issue, time.Now().UnixNano()),
		IssueNumber: issue,
		Kind:        KindIntake,
		IssueTitle:  fmt.Sprintf("Issue #%d (intake)", issue),
		Status:      StatusRunning,
		StartTime:   time.Now(),
		RunID:       failure.NewRunID(),
		intake:      run,
	}
	m.agents[a.ID] = a
	m.statusGen.Add(1)
	m.mu.Unlock()

	// GitHub and file I/O happen outside the lock.
	revision, err := m.github.Revision(context.Background(), issue)
	if err != nil {
		err = fmt.Errorf("read revision of issue #%d: %w", issue, err)
		m.endIntake(a, IntakeResult{Outcome: IntakeFailed, Err: err})
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(intakeLogPath(issue)), 0o755); err != nil {
		m.endIntake(a, IntakeResult{Outcome: IntakeFailed, Err: err})
		return nil, false, err
	}
	logFile, err := os.OpenFile(intakeLogPath(issue), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		err = fmt.Errorf("open intake log for issue #%d: %w", issue, err)
		m.endIntake(a, IntakeResult{Outcome: IntakeFailed, Err: err})
		return nil, false, err
	}
	m.mu.Lock()
	run.revision = revision
	run.logFile = logFile
	m.mu.Unlock()

	log.Printf("[intake] started for issue #%d (log: %s)", issue, intakeLogPath(issue))
	m.launchIntakeAttempt(a)
	return run, true, nil
}

// launchIntakeAttempt starts the current attempt's process and its monitor. A
// process that cannot start is a failed attempt, retried like any other.
func (m *Manager) launchIntakeAttempt(a *Agent) {
	run := a.intake
	m.mu.RLock()
	attempt := run.policy.Attempt(run.attempt)
	prompt := planner.AutomatedPrompt(m.config.Repo, run.issue, attempt, run.last)
	m.mu.RUnlock()

	cmd := m.intakeCommand(prompt)
	var reply bytes.Buffer
	capture := NewCaptureWriter(nil, m.outputCapture, fmt.Sprintf("[agent issue-%d] intake: ", run.issue))
	fmt.Fprintf(run.logFile, "=== intake attempt %d of %d ===\n", attempt.Current, attempt.Max)
	cmd.Stdout = io.MultiWriter(&reply, run.logFile, capture)
	cmd.Stderr = io.MultiWriter(run.logFile, capture)

	if err := cmd.Start(); err != nil {
		m.intakeAttemptFailed(a, planner.InvokeFailure(attempt, err), failure.Recoverable)
		return
	}
	if err := writePIDMarker(run.issue, cmd.Process.Pid); err != nil {
		log.Printf("[intake] failed to write PID marker for issue #%d: %v", run.issue, err)
	}
	m.mu.Lock()
	a.Process = cmd.Process
	m.statusGen.Add(1)
	m.mu.Unlock()

	go m.monitorIntake(a, cmd, &reply)
}

// monitorIntake waits for one attempt and acts on its reply.
func (m *Manager) monitorIntake(a *Agent, cmd interface{ Wait() error }, reply *bytes.Buffer) {
	run := a.intake
	werr := cmd.Wait()
	removePIDMarker(run.issue)

	m.mu.RLock()
	stopped := run.stopped
	attempt := run.policy.Attempt(run.attempt)
	revision := run.revision
	m.mu.RUnlock()

	if stopped {
		m.endIntake(a, IntakeResult{Outcome: IntakeStopped})
		return
	}
	if werr != nil {
		err := fmt.Errorf("%s exited: %w (see %s)", planner.KiroCommand, werr, intakeLogPath(run.issue))
		m.intakeAttemptFailed(a, planner.InvokeFailure(attempt, err), failure.Recoverable)
		return
	}

	out, err := m.plannerBoundary(run.issue).Process(context.Background(), planner.Request{
		Reply:   planner.StripANSI(reply.String()),
		Attempt: attempt,
		Target:  provenance.Target{Number: run.issue, ObservedRevision: revision},
	})
	if err != nil {
		m.endIntake(a, IntakeResult{Outcome: IntakeFailed, Err: err})
		return
	}
	switch out.Outcome {
	case planner.OutcomeRejected:
		m.intakeAttemptFailed(a, planner.RejectionFailure(out, attempt), out.Disposition)
	case planner.OutcomeCommitted:
		if err := writeCommitted(run.issue, out.Hash); err != nil {
			log.Printf("[intake] failed to record the commit for issue #%d: %v", run.issue, err)
		}
		m.endIntake(a, IntakeResult{Outcome: IntakeCommitted, Hash: out.Hash})
	case planner.OutcomeRoutedToIntake:
		m.endIntake(a, IntakeResult{Outcome: IntakeChanged})
	case planner.OutcomePunchedOut:
		m.endIntake(a, IntakeResult{Outcome: IntakePunchedOut, Failure: out.Failure})
	default:
		m.endIntake(a, IntakeResult{Outcome: IntakeFailed, Err: fmt.Errorf("planner reported outcome %v", out.Outcome)})
	}
}

// intakeAttemptFailed retries a failed attempt while attempts remain, with the
// failure in the next prompt. Otherwise it punches the issue out once.
func (m *Manager) intakeAttemptFailed(a *Agent, fc guard.FailureContext, d failure.Disposition) {
	run := a.intake
	m.mu.Lock()
	fc.Attempt = run.policy.Attempt(run.attempt)
	if failure.Decide(fc, d) == failure.DecisionRetry {
		run.last = &fc
		run.attempt++
		a.RetryCount++
		next := run.attempt
		m.mu.Unlock()
		log.Printf("[intake] attempt %d for issue #%d failed (%s); retrying", next-1, run.issue, fc.Guard)
		go func() {
			time.Sleep(m.intakeDelay(next))
			m.mu.RLock()
			stopped := run.stopped
			m.mu.RUnlock()
			if stopped {
				m.endIntake(a, IntakeResult{Outcome: IntakeStopped})
				return
			}
			m.launchIntakeAttempt(a)
		}()
		return
	}
	runID := a.RunID
	m.mu.Unlock()

	target := failure.Target{Repo: m.config.Repo, Issue: run.issue, FailLabel: failure.FailedLabel(m.config.Label)}
	remedy := fmt.Sprintf("Read the intake log at %s to see why planning failed, fix the cause (the issue text, "+
		"or the kiro-cli login), then run `validate %d` in the Kairon TUI.", intakeLogPath(run.issue), run.issue)
	if err := failure.PunchOut(m.github, target, runID, fc, remedy); err != nil {
		m.endIntake(a, IntakeResult{Outcome: IntakeFailed, Err: fmt.Errorf("punch out issue #%d: %w", run.issue, err)})
		return
	}
	m.endIntake(a, IntakeResult{Outcome: IntakePunchedOut, Failure: &fc})
}

// endIntake records how the run ended, once, and hands the issue back to the
// watcher.
func (m *Manager) endIntake(a *Agent, res IntakeResult) {
	run := a.intake
	m.mu.Lock()
	if a.Status != StatusRunning {
		m.mu.Unlock()
		return
	}
	if res.Outcome == IntakeCommitted {
		a.Status = StatusCompleted
	} else {
		a.Status = StatusFailed
	}
	res.Issue, res.Attempts = run.issue, run.attempt
	run.result = res
	m.statusGen.Add(1)
	logFile := run.logFile
	m.mu.Unlock()

	if logFile != nil {
		fmt.Fprintf(logFile, "=== intake %s ===\n", res.Outcome)
		logFile.Close()
	}
	if res.Err != nil {
		log.Printf("[intake] issue #%d: %s after %d attempt(s): %v", run.issue, res.Outcome, res.Attempts, res.Err)
	} else {
		log.Printf("[intake] issue #%d: %s after %d attempt(s)", run.issue, res.Outcome, res.Attempts)
	}
	close(run.done)
	m.release(run.issue)
}

// plannerBoundary is the planner output boundary for one existing issue. It is
// the only writer of the issue body.
func (m *Manager) plannerBoundary(issue int) *planner.Boundary {
	target := failure.Target{Repo: m.config.Repo, Issue: issue, FailLabel: failure.FailedLabel(m.config.Label)}
	return &planner.Boundary{
		Committer: &provenance.Committer{
			Store:  m.github,
			Router: &planner.ConflictRouter{Issues: m.github, Target: target},
		},
		Issues: m.github,
		Target: target,
	}
}

// ValidateStatus is how a validate request ended.
type ValidateStatus int

// Validate statuses.
const (
	// ValidateAlreadyValid: the body is verified; no run was started.
	ValidateAlreadyValid ValidateStatus = iota + 1
	// ValidateRefused: the issue is closed or done; no run was started.
	ValidateRefused
	// ValidateAlreadyRunning: a run for the issue is already going.
	ValidateAlreadyRunning
	// ValidateFinished: an intake run ran to its end; see Intake.
	ValidateFinished
	// ValidateError: the request could not be carried out; see Err.
	ValidateError
)

// ValidateResult is what Validate reports.
type ValidateResult struct {
	Status ValidateStatus
	Issue  int
	// Hash is the verified body's hash (ValidateAlreadyValid).
	Hash string
	// Reason says why the request was refused (ValidateRefused).
	Reason string
	// HadFailedLabel says the issue carried the fail label when validate read it.
	HadFailedLabel bool
	// Intake is how the run ended (ValidateFinished).
	Intake IntakeResult
	// LabelRemoved says the fail label was removed after a commit; LabelErr
	// says why that removal failed.
	LabelRemoved bool
	LabelErr     error
	Err          error
}

// Validate runs intake on issue at a user's request and waits for the run to
// end. Unlike the watcher, it runs on an issue that carries the fail label,
// and removes that label after a commit so the watcher picks the issue up.
// Callers run it off the UI goroutine.
func (m *Manager) Validate(ctx context.Context, issue int) ValidateResult {
	failedLabel, doneLabel := failure.FailedLabel(m.config.Label), failure.DoneLabel(m.config.Label)
	snap, err := m.github.ReadIssue(m.config.Repo, issue, failedLabel)
	if err != nil {
		return ValidateResult{Status: ValidateError, Issue: issue, Err: fmt.Errorf("read issue #%d: %w", issue, err)}
	}
	switch {
	case !snap.Open:
		return ValidateResult{Status: ValidateRefused, Issue: issue, Reason: "the issue is closed"}
	case slices.Contains(snap.State.Labels, doneLabel):
		return ValidateResult{Status: ValidateRefused, Issue: issue, Reason: fmt.Sprintf("the issue is done (`%s`)", doneLabel)}
	}
	hadFailed := slices.Contains(snap.State.Labels, failedLabel)
	if provenance.Classify(snap.Body) == provenance.StateValid {
		return ValidateResult{Status: ValidateAlreadyValid, Issue: issue, Hash: provenance.Hash(snap.Body), HadFailedLabel: hadFailed}
	}

	// An explicit request starts fresh: it is not a loop.
	m.clearCommitted(issue)
	run, started, err := m.startIntake(issue)
	if err != nil {
		return ValidateResult{Status: ValidateError, Issue: issue, Err: err}
	}
	if !started {
		return ValidateResult{Status: ValidateAlreadyRunning, Issue: issue}
	}
	select {
	case <-run.done:
	case <-ctx.Done():
		return ValidateResult{Status: ValidateError, Issue: issue, Err: ctx.Err()}
	}
	m.mu.RLock()
	res := run.result
	m.mu.RUnlock()

	out := ValidateResult{Status: ValidateFinished, Issue: issue, HadFailedLabel: hadFailed, Intake: res}
	if res.Outcome == IntakeCommitted && hadFailed {
		if err := m.github.RemoveLabel(m.config.Repo, issue, failedLabel); err != nil {
			out.LabelErr = err
		} else {
			out.LabelRemoved = true
		}
	}
	return out
}

// writePIDMarker records the intake process for issue.
func writePIDMarker(issue, pid int) error {
	if err := os.MkdirAll(filepath.Dir(intakePIDPath(issue)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(intakePIDPath(issue), []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

func removePIDMarker(issue int) {
	if err := os.Remove(intakePIDPath(issue)); err != nil && !os.IsNotExist(err) {
		log.Printf("[intake] failed to remove PID marker for issue #%d: %v", issue, err)
	}
}

// livePIDMarker reports whether the issue's PID marker names a live process. A
// marker for a dead process is stale and is removed.
func livePIDMarker(issue int) bool {
	data, err := os.ReadFile(intakePIDPath(issue))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err == nil && pid > 0 && processAlive(pid) {
		return true
	}
	removePIDMarker(issue)
	return false
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// writeCommitted records that intake committed issue with hash.
func writeCommitted(issue int, hash string) error {
	if err := os.MkdirAll(filepath.Dir(intakeCommittedPath(issue)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(intakeCommittedPath(issue), []byte(hash+"\n"), 0o644)
}

func (m *Manager) hasCommitted(issue int) bool {
	_, err := os.Stat(intakeCommittedPath(issue))
	return err == nil
}

func (m *Manager) clearCommitted(issue int) {
	if err := os.Remove(intakeCommittedPath(issue)); err != nil && !os.IsNotExist(err) {
		log.Printf("[intake] failed to clear the commit record for issue #%d: %v", issue, err)
	}
}
