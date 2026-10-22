# Issue #379: Uniform failure contract: bounded retry, then punch-out with comment and label

Closes #379

Design source: `.kairon/specs/maturity-model/planner-requirements.md` §14 (failure contract), §4.1
(`FailureContext`), §3.5 (planner punch-out maps onto a `FailureContext`).
Depends on #377 (closed): `internal/guard` (`Guard`, `FailureContext`, `Attempt`, `FailureHandler`, `Run`).

## 1. Problem

A failed stage must end one of two ways: a bounded automatic retry (Tier 1), or a punch-out (Tier 2) that tells a
human why and removes the issue from the polling set. Today only `internal/agent/manager.go` has this shape, and its
terminal path is a bare best-effort `github.AddLabel(..., "<label>-failed")`. The issue ends up labeled with no
stated reason (§14.5). Nothing exists for a guard's failure method (#377) to route into, so the planner punch-out
(#381) and gate/validation failures have nowhere uniform to land.

## 2. Solution approach

### 2.1 Shape

Add one new package, `internal/failure`, that owns the whole contract. It does no I/O itself: every GitHub effect goes
through a small injected port (`failure.Issues`). That keeps the mechanism deterministic Go (not prompt text),
unit-testable with a fake, and consistent with the rule that `internal/guard` never touches GitHub.

```
guard.Run ──reject──▶ guard.OnFailure(fc)           (guard decides recoverable vs terminal)
                          │
                          ▼
              failure.Route  (records fc + disposition; a guard.FailureHandler)
                          │
                          ▼
              failure.Runner (Tier 1 loop / Tier 2 trigger)  ──▶ failure.PunchOut ──▶ failure.Issues (port)
                                                                                         ▲
internal/agent Manager (async process lifecycle) ── failure.Decide + failure.PunchOut ───┘
internal/github.CLI  implements failure.Issues with the `gh` CLI
internal/github.SelectIssues / watcher use failure.IssueState.Classify for selection and Reconcile for repair
```

`internal/guard` is **not modified** (its `TestNoForbiddenImports` allows only stdlib and `internal/stageoutput`).
`internal/failure` imports `internal/guard` for `FailureContext`/`Attempt`/`FailureHandler`.

### 2.2 Decisions

1. **Two entry styles, one decision function.** `failure.Runner.Run` is a synchronous loop (used by synchronous
   boundaries and by tests; satisfies "re-run the stage, increment the counter"). The agent `Manager` is
   event-driven (process exits, then `retryAgent` spawns a new process), so it cannot use a loop. Both call the same
   pure `failure.Decide(fc, disposition)` and the same `failure.PunchOut`, so the tier logic exists once.
2. **Attempt semantics are preserved.** `max_retries` is the number of *re-runs after the first run*, as in
   `manager.go` today (`RetryCount < MaxRetries` then retry; 1 initial run + `MaxRetries` retries). `guard.Attempt`
   requires `1 <= Current <= Max`, so the failure package works in *attempts*: `Policy.MaxAttempts = maxRetries + 1`.
   `PolicyFromRetries` is the only place the `+1` is written. A negative config value is treated as 0 retries.
   `max_qa_retries` gets its own policy through `PolicyFor(cfg, BoundaryQA)`. Today the builder-validator QA loop is
   still driven by the krew-lead prompt, so no Go code consumes `BoundaryQA` yet. That is documented, not hidden.
3. **The guard declares recoverable versus terminal.** `guard.FailureHandler` is `func(FailureContext) error`, so the
   disposition cannot travel in the context. `failure.Route` supplies two handlers, `Recoverable()` and
   `Terminal(remedy)`. A guard's `OnFailure` calls whichever it owns; the route records the context and disposition;
   the `Runner` enacts it. A planner punch-out calls `Terminal` with `Attempt{1,1}` (§3.5).
4. **Run identity, not a content digest, scopes the marker.** A digest of the `FailureContext` alone would be identical
   when the same problem recurs after a human restart, and would wrongly suppress the new explanation (AC 5). The
   marker therefore carries a **run id**: `failure.NewRunID()` is generated once per terminal run (per `Runner.Run`,
   and per `agent.Agent` at `Spawn`) and is stable across retries of the same punch-out. A caller must supply one;
   there is no digest fallback.
5. **Terminal-pending is decided from GitHub state, using the label timeline.** "Marker present and label absent"
   is not enough: after a human clears the label the old marker comment remains and would read as pending forever.
   Rule (`IssueState.Classify`): an issue is *terminal-pending* when (a) neither `-done` nor `-failed` is present,
   (b) a marker comment **authored by the authenticated user** exists, and (c) the fail-state label has **not** been
   removed at or after the newest such marker's `createdAt`. The removal time comes from the issue events API
   (`unlabeled` events), fetched lazily, only for an issue that has a trusted marker and no excluded label.
   Both timestamps are GitHub server times, so no clock skew. A tie (equal seconds) counts as removed, since the
   label can only be applied after the comment.
6. **Untrusted text never becomes control.** Anyone who can comment could paste a marker. Only comments whose
   author equals the authenticated `gh` user count (`Comment.ByViewer`), the marker must be the **first line** of the
   comment body, and `Expected`/`Actual` (which may contain agent output) are rendered inside a code fence that is
   longer than any backtick run they contain, truncated to 2000 runes each. `gh` is invoked with an argument list
   (never a shell), and the comment body is passed on stdin (`--body-file -`).
7. **Fail closed.** If the events lookup fails for one issue, `SelectIssues` excludes that issue for this poll and logs,
   rather than risk starting a stage mid-reconciliation. If the comment write fails, the label is not written.
8. **Reconcile finishes a half-done punch-out.** `failure.Reconcile` (called by the watcher for each terminal-pending
   issue) applies only the missing label. Because selection excludes pending issues, repair is a separate step, not a
   side effect of selecting.
9. **Restart really works end to end (§14.3).** Two existing behaviors would break "clear the label and the pipeline
   resumes with a fresh budget" and are fixed here with minimal change:
   - `watcher.tracked` is never cleared, so a running watcher would ignore a restarted issue. The watcher drops
     tracked entries for issues absent from the eligible list on a successful poll (a punched-out issue is absent
     because it is labeled).
   - `.kairon/retries/issue-N.count` persists, so after `max_retries` restarts the watcher would skip the issue
     silently. The count file is removed when a punch-out completes (manager) or is reconciled (watcher).
   - If a poll errors, no un-tracking happens.

### 2.3 Alternatives considered

| Option | Why not |
|--------|---------|
| Put the mechanism in `internal/guard` | Breaks its no-I/O import rule and mixes admission with handling. |
| Digest of `FailureContext` as the marker | Suppresses a later identical failure after a restart (violates AC 5). |
| Edit the comment to "complete" after the label lands | Extra write that can itself fail, and needs comment ids. The label timeline already gives the same fact. |
| Convert the manager to a synchronous loop | Large behavior change to process supervision; out of scope. Sharing `Decide` and `PunchOut` gives uniformity without it. |
| Hard-code 3 retries | Violates the constraint; limits come from `config.MaxRetries` / `MaxQARetries`. |

## 3. Relevant files

New:
- `internal/failure/doc.go`, `policy.go`, `runid.go`, `marker.go`, `state.go`, `render.go`, `punchout.go`,
  `route.go`, `runner.go`, and `*_test.go` for each, plus `deps_test.go` and `docs_test.go`
- `docs/failure-contract.md`

Modified:
- `internal/github/client.go` (new `CLI` adapter, `SelectIssues`, `ListIssues` delegates to it)
- `internal/github/*_test.go` (new tests)
- `internal/agent/manager.go` (retrofit; seams `issues`, `prExists`, `retry`, `cleanup`; `Agent.RunID`)
- `internal/agent/manager_failure_test.go` (new)
- `internal/watcher/watcher.go` (reconcile pending, un-track, reset retry count) and `watcher_reconcile_test.go` (new)
- `README.md` (Issue Lifecycle table row for Failed; Retry Logic section)
- `docs/guards.md` (point the "#379 adds ..." paragraph at the new doc; keep the terms `#378` and `#379`, which
  `internal/guard/docs_test.go` requires)

Not changed: `internal/guard/*` (code), `.kiro/agents/*`, `.kairon/scripts/*`, any template-synchronized file, label
strings, `CHANGELOG.md`, anything under `.kairon/specs/` other than this file (AGENTS.md: historical artifacts).
`task sync:check` is still run as a regression gate; no sync commands should be needed.

## 4. API (target shapes; unexported names are the builder's choice)

```go
package failure

// Policy and decisions
type Policy struct{ MaxAttempts int }                        // total executions of the stage
func PolicyFromRetries(maxRetries int) Policy                // MaxAttempts = max(maxRetries,0) + 1
type Boundary int                                            // BoundaryImplementation, BoundaryQA
func PolicyFor(cfg *config.Config, b Boundary) Policy        // MaxRetries / MaxQARetries
func (p Policy) Attempt(n int) guard.Attempt                 // {Current: n, Max: p.MaxAttempts}

type Disposition int                                         // Recoverable (zero value), Unrecoverable
type Decision int                                            // DecisionRetry, DecisionPunchOut
func Decide(fc guard.FailureContext, d Disposition) Decision // Unrecoverable -> PunchOut; Current<Max -> Retry; else PunchOut

// Labels (single source for the strings; unchanged values)
func FailedLabel(triggerLabel string) string                 // triggerLabel + "-failed"
func DoneLabel(triggerLabel string) string                   // triggerLabel + "-done"

// Run identity and marker
func NewRunID() string                                       // [A-Za-z0-9._-]+, unique per call
func ValidRunID(id string) bool
func Marker(runID string) string                             // "<!-- kairon:punch-out run=<id> -->"
func ParseMarker(body string) (runID string, ok bool)        // marker must be the FIRST line

// Issue state and selection (pure)
type Comment struct{ Body string; CreatedAt time.Time; ByViewer bool }
type IssueState struct {
    Labels             []string
    Comments           []Comment
    FailLabelRemovedAt time.Time // most recent removal of the fail-state label; zero if none/unknown
}
type Selection int // SelectEligible, SelectDone, SelectFailed, SelectTerminalPending
func (s IssueState) Classify(doneLabel, failedLabel string) Selection
func (s Selection) Excluded() bool

// Port (implemented by github.CLI; faked in tests)
type Issues interface {
    State(repo string, issue int, failLabel string) (IssueState, error) // labels + comments; events lookup only if needed
    PostComment(repo string, issue int, body string) error
    AddLabel(repo string, issue int, label string) error
}
type Target struct{ Repo string; Issue int; FailLabel string }

// Tier 2
func RenderComment(fc guard.FailureContext, runID, remedy, failLabel string) string
func PunchOut(iss Issues, t Target, runID string, fc guard.FailureContext, remedy string) error
func Reconcile(iss Issues, t Target) (applied bool, err error)

// Routing a guard into the mechanism, and the synchronous Tier 1 loop
type Route struct{ /* records fc + disposition + remedy */ }
func (r *Route) Recoverable() guard.FailureHandler
func (r *Route) Terminal(remedy string) guard.FailureHandler
func (r *Route) Result() StageResult

type StageResult struct { Failure *guard.FailureContext; Disposition Disposition; Remedy string } // Failure nil = success
type Stage func(attempt guard.Attempt) StageResult
type Outcome int // OutcomeSucceeded, OutcomePunchedOut, OutcomePunchOutPending, OutcomeHalted
type Result struct { Outcome Outcome; Attempts int; Failure *guard.FailureContext }
type Runner struct { Issues Issues; Target Target; Policy Policy; RunID string }
func (r Runner) Run(stage Stage) (Result, error)
```

### 4.1 Behavior contract

`PunchOut` (idempotent, comment-first):
1. Validate `runID`, `fc.Validate()`, and the target. Invalid input returns an error and writes nothing.
2. `State(...)`. On error return it; write nothing.
3. If no `ByViewer` comment carries `Marker(runID)`: `PostComment(RenderComment(...))`. If that fails, return the
   error and do **not** touch the label.
4. If `FailLabel` is already in `State.Labels`: no-op. Otherwise `AddLabel`.
5. Return nil only when both the comment and the label are in place.

`Runner.Run`:
1. Reads `State` first. If `Classify` is anything but `SelectEligible`, return `OutcomeHalted` without calling the
   stage (no auto-re-entry while labeled or terminal-pending).
2. Attempt counter starts at 1 on every call and lives only inside the call (a restarted issue is a fresh call).
3. Calls `stage(Policy.Attempt(n))`. A nil `Failure` is `OutcomeSucceeded`.
4. The runner overwrites `Failure.Attempt` with its own counter (a stage cannot misreport attempts), then validates
   it; an invalid context returns an error wrapping `guard.ErrInvalid`, with no writes.
5. `Decide`: `Retry` loops (no label, no comment). `PunchOut` calls `PunchOut(...)` and returns. A nil error gives
   `OutcomePunchedOut`; a non-nil error gives `OutcomePunchOutPending` and that error (the watcher's `Reconcile`
   finishes the label). The stage is never called again after a punch-out decision.

`RenderComment` first line is `Marker(runID)`. The visible text names the stage and guard, "attempts made N of M",
the expected and actual values (fenced, truncated), the remedy (default: "Make this true, then remove the label:
<expected>"), and the restart instruction naming `failLabel`.

## 5. Team orchestration

Single PR. `failure-core` is the foundation (types, port, pure logic). Then three tasks run in parallel because
they touch different packages and share only the `internal/failure` API from `failure-core` (or `failure-punchout`).

```
failure-core ─┬─▶ failure-punchout ─┬─▶ manager-retrofit ──┐
              │                      └─▶ watcher-restart ───┼─▶ failure-docs ─▶ validate-all
              └─▶ github-adapter ───────┬─▶ manager-retrofit  │
                                        └─▶ watcher-restart ──┘
```

- Documentation is assigned to `builder`, not `documenter`: the documenter agent writes only to `app_docs/`, cannot
  run shell, and the issue requires `docs/failure-contract.md` plus a docs test and README/guards.md edits.
- Do not edit `.kiro/agents/*` (unrelated sandbox changes). No template sync is expected.
- TDD per AGENTS.md: each task writes its failing test first, confirms it fails for the expected reason, then writes
  the minimal code. Commit tests with the implementation. The commit message names the test seen failing first.
- Commit only; never push (git steering).

## 6. Step-by-step task breakdown

### Task 1: `failure-core`: policy, decision, run id, marker, state classification, rendering, port
No dependencies.
- `policy.go`: `Policy`, `PolicyFromRetries`, `Boundary`, `PolicyFor`, `Disposition`, `Decision`, `Decide`,
  `FailedLabel`, `DoneLabel`.
- `runid.go`, `marker.go`: `NewRunID`, `ValidRunID`, `Marker`, `ParseMarker`. Marker must be the first line;
  malformed or non-first-line markers are rejected.
- `state.go`: `Comment`, `IssueState`, `Selection`, `Classify`, `Excluded`, and the `Issues` port and `Target`.
- `render.go`: `RenderComment` with fence-length escaping and truncation.
- `deps_test.go`: `internal/failure` must not directly import `os`, `os/exec`, `net`, `net/http` (mirrors
  `internal/guard/deps_test.go`). It may import `internal/guard`, `internal/stageoutput`, `internal/config`.
- Tests (names are prescribed so validation can target them): `TestDecide_*`, `TestPolicyFromRetries`,
  `TestPolicyFor_FollowsConfig` (override `MaxRetries` and `MaxQARetries` independently; `MaxAttempts` follows),
  `TestMarker_RoundTrip`, `TestParseMarker_RejectsNonFirstLine`, `TestRenderComment_ContainsStageAttemptsExpectedActualRemedy`
  (AC 4), `TestRenderComment_NeutralizesUntrustedValues` (fence breakout, injected marker, truncation),
  `TestClassify_TerminalPendingIsExcluded` (AC 6), `TestClassify_LabelClearedAfterMarkerIsEligible` (restart),
  `TestClassify_NewMarkerAfterRestartIsPending`, `TestClassify_IgnoresMarkerNotByViewer`, `TestClassify_DoneAndFailed`,
  `TestNoForbiddenImports`.

### Task 2: `failure-punchout`: PunchOut, Reconcile, Route, Runner
Depends on Task 1.
- `punchout.go`, `route.go`, `runner.go` per section 4.1.
- A fake `Issues` in tests records calls in order and reflects posted comments and labels back through `State`; it
  can be told to fail a specific call.
- Tests: `TestRunner_RecoversAfterTwoFailures` (AC 1: fails twice then succeeds, max retries 2 so MaxAttempts 3 gives
  exactly 3 stage runs, zero `AddLabel`, zero comments), `TestRunner_ExhaustionPunchesOut` (AC 2: always fails, retries
  3 gives 4 stage runs, exactly one comment, `AddLabel` with the fail label, stage not called again, `OutcomePunchedOut`),
  `TestRunner_UnrecoverableSkipsRetries` (AC 3: via `Route.Terminal`, stage runs once, immediate punch-out),
  `TestRunner_RetryCountFollowsConfig` (a config override changes the run count), `TestRunner_RefusesWhileLabeled`
  and `TestRunner_RefusesWhileTerminalPending` (AC 8: stage never called, `OutcomeHalted`),
  `TestRunner_FreshRunStartsAtAttemptOne` (AC 8: after the fake clears the label, a new `Run` sees attempt 1),
  `TestRunner_NoAutoRetryAfterTerminal` (AC 8: no exported method re-enters; a second `Run` without clearing the
  label halts), `TestRunner_OverwritesStageAttempt`, `TestRunner_InvalidFailureContext`,
  `TestPunchOut_CommentBeforeLabel` (call order), `TestPunchOut_CommentFailureLeavesNoLabel`,
  `TestPunchOut_RetryAfterPartialPostsNoDuplicate` (AC 5: comment ok, label fails, call again, exactly one comment,
  label applied), `TestPunchOut_LabelAlreadyPresentIsNoop`, `TestPunchOut_NewRunIDPostsFreshComment` (AC 5: after
  the fake simulates a human clearing the label, a new run id posts its own comment),
  `TestPunchOut_IgnoresMarkerNotByViewer`, `TestReconcile_AppliesMissingLabelOnly`, `TestReconcile_NoopWhenNotPending`,
  `TestRoute_*` (recoverable vs terminal recorded; a real `guard.Run` with `guard.NewStructural(route.Terminal(...))`
  routes through).

### Task 3: `github-adapter`: `CLI`, `SelectIssues`, trusted-marker selection
Depends on Task 1. Can run in parallel with Task 2.
- `type CLI struct{}` with `State`, `PostComment`, `AddLabel`; `var _ failure.Issues = CLI{}`.
  - `State`: `gh issue view N --repo R --json labels,comments`. `ByViewer` is true when the comment author login equals
    `GetAuthenticatedUser()` (fetched once and cached per call). Only when a `ByViewer` marker exists and neither
    excluded label is present, query
    `gh api repos/R/issues/N/events --paginate --jq '.[] | select(.event=="unlabeled") | [.label.name, .created_at] | @tsv'`
    and filter by label name **in Go** (never interpolate the label into the jq program). Fill `FailLabelRemovedAt`.
  - `PostComment`: `gh issue comment N --repo R --body-file -` with the body on stdin.
- `SelectIssues(repo, label, assignee) (Selection, error)` returning `Eligible` and `TerminalPending` issue slices.
  It requests `number,title,body,labels,comments` in the single list call, uses `failure.IssueState.Classify`, and
  fetches events only for candidates that need it. A per-issue lookup error excludes that issue and logs.
  `ListIssues` keeps its signature and returns `SelectIssues(...).Eligible`. Rate-limit handling is unchanged.
- Keep the pure part (decoded JSON in, classification out) separate from the `exec` calls so it is unit-testable
  without `gh`.
- Tests: `TestSelectIssues_ExcludesTerminalPending` (AC 6: comment with trusted marker, no label, so excluded from
  `Eligible`, present in `TerminalPending`), `TestSelectIssues_ExcludesDoneAndFailed` (existing behavior),
  `TestSelectIssues_RestartedIssueIsEligible` (marker plus later `unlabeled` event), `TestSelectIssues_IgnoresUntrustedMarker`,
  `TestSelectIssues_EventsLookupErrorFailsClosed`, `TestParseUnlabeledEvents`.

### Task 4: `manager-retrofit`: manager terminal path posts the explanation
Depends on Tasks 2 and 3.
- `Manager` gains seams with production defaults: `issues failure.Issues` (`github.CLI{}`), `prExists`
  (`github.PRExistsForIssue`), `retry func(*Agent)` (`m.retryAgent`), `cleanup func(issue, pid int)`
  (`m.performCleanup`). `Agent` gains `RunID` (set from `failure.NewRunID()` in `Spawn`; reused by retries).
- In `HandleExit`, replace both bare `github.AddLabel(..., failedLabel)` calls (exit 0 with no PR, and non-zero exit)
  with the shared path: build a `guard.FailureContext` (`Stage: stageoutput.StageKrewLead`, `Guard:
  "implementation-run"`, `Attempt{RetryCount+1, MaxRetries+1}` via `failure.PolicyFor(cfg, BoundaryImplementation)`,
  `Expected: "krew-lead exits 0 and a pull request exists for issue #N"`, `Actual:` the exit code, or "exited 0 but no
  pull request was found for branch spec/issue-N-*"), call `failure.Decide`, then either `m.retry` (existing counter
  and status handling, unchanged) or `failure.PunchOut` with a remedy that points at `.kairon/logs/issue-N.log`.
- The non-zero branch currently holds `m.mu` (via `defer`) across the label write. Decide and update status under the
  lock, release it, then do the GitHub writes.
- A `PunchOut` error is logged with "will be reconciled on the next poll"; it does not panic or retry in a loop.
  On success, remove `.kairon/retries/issue-N.count` via the existing `cleanupRetryFile`.
- Existing `-done` behavior and log lines for the success path are unchanged.
- Tests (`manager_failure_test.go`, fake `Issues`, stubbed seams, no real `gh`/`kiro-cli`/`git`):
  `TestHandleExit_ExhaustedRetriesPostsCommentThenLabel` (AC 7: non-zero exit with `RetryCount == MaxRetries`;
  asserts a comment call precedes `AddLabel(..., "<label>-failed")`, comment mentions stage and "N of M"),
  `TestHandleExit_NoPRExhaustedPostsCommentThenLabel` (exit 0, `prExists` false), `TestHandleExit_RetriesRemainNoWrites`
  (`retry` stub called, no comment or label), `TestHandleExit_RetryBudgetFollowsConfig` (change `MaxRetries`, the
  number of `retry` calls before punch-out follows it), `TestHandleExit_PunchOutErrorKeepsRetryFile`.

### Task 5: `watcher-restart`: reconcile pending, make restart work
Depends on Tasks 2 and 3. Can run in parallel with Task 4.
- `checkIssues` calls `github.SelectIssues`. For each `TerminalPending` issue, call an injectable
  `reconcile func(repo string, issue int, failLabel string) error` (default wraps `failure.Reconcile(github.CLI{}, ...)`);
  on success, remove the issue's retry count file. Log errors; continue.
- After a successful poll, delete `tracked` entries whose issue number is not in `Eligible`. On a poll error, leave
  `tracked` untouched.
- Keep this small and put the logic in two helpers (`reconcilePending`, `untrackAbsent`) so they can be tested
  without `gh`.
- Tests: `TestReconcilePending_CallsReconcileAndResetsRetryCount`, `TestReconcilePending_ErrorDoesNotStopOthers`,
  `TestUntrackAbsent_DropsExcludedKeepsEligible`, `TestRestartedIssueReentersWithFreshRetryBudget` (retry file
  removed, so `hasExceededGlobalRetries` is false).
- Out of scope: the read/verify boundary (#382) consumes the terminal-pending rule later.

### Task 6: `failure-docs`: reference docs
Depends on Tasks 4 and 5 (document what was built).
- `docs/failure-contract.md`: the two tiers; attempt semantics and the config knobs (`max_retries`,
  `max_qa_retries`, and the +1); disposition and how a guard routes with `Route` (a short Go example); punch-out steps
  and why comment-before-label; a rendered comment example; the run-scoped marker and trust rules; terminal-pending
  selection, the label-timeline rule and `Reconcile`; the restart-by-clearing-the-label flow (§14.3) including the
  watcher un-tracking and retry-count reset; the manager mapping; known limits (QA loop still prompt-driven,
  2000-rune cap, fail-closed polls).
- `internal/failure/docs_test.go`: `TestDocs_NamesRequiredTerms` fails if the doc omits terms such as `Tier 1`,
  `Tier 2`, `punch-out`, `FailureContext`, `terminal-pending`, `run id`, `max_retries`, `max_qa_retries`,
  `Reconcile`, `Route`, `Runner`, `kairon-failed`, and `#381`/`#382` (follow-ups). Follow the `internal/guard/docs_test.go` pattern.
  Do not use ` ```yaml ` fences in the new doc.
- `README.md`: Issue Lifecycle table (Failed row mentions the explanatory comment) and the Retry Logic section
  (link to `docs/failure-contract.md`). `docs/guards.md`: replace the sentence that says #379 "adds" the handling
  with a link; the terms `#378` and `#379` must remain.

### Task 7: `validate-all`: independent verification
Depends on Task 6. Read-only. Run every command below, map each acceptance criterion to a passing named test, and
confirm the work's commits touch no file under `.kiro/agents/` or `.kairon/specs/` (other than this spec). The
worktree already has unrelated uncommitted edits under `.kiro/agents/`; they are not part of this work.

## 7. Acceptance-criteria traceability

| AC | Proven by |
|----|-----------|
| 1 recoverable retries, no label | `TestRunner_RecoversAfterTwoFailures` |
| 2 exhaustion punches out once and halts | `TestRunner_ExhaustionPunchesOut` |
| 3 terminal skips retries | `TestRunner_UnrecoverableSkipsRetries`, `TestRoute_*` |
| 4 comment rendered from context | `TestRenderComment_ContainsStageAttemptsExpectedActualRemedy` |
| 5 comment-first, idempotent, run-scoped | `TestPunchOut_CommentBeforeLabel`, `..._RetryAfterPartialPostsNoDuplicate`, `..._NewRunIDPostsFreshComment`, `..._LabelAlreadyPresentIsNoop` |
| 6 selection skips terminal-pending | `TestClassify_TerminalPendingIsExcluded`, `TestSelectIssues_ExcludesTerminalPending` |
| 7 manager posts comment plus label | `TestHandleExit_ExhaustedRetriesPostsCommentThenLabel` |
| 8 label clear is the only restart, fresh budget | `TestRunner_RefusesWhileLabeled`, `TestRunner_FreshRunStartsAtAttemptOne`, `TestRunner_NoAutoRetryAfterTerminal`, `TestRestartedIssueReentersWithFreshRetryBudget` |
| Config-driven max | `TestPolicyFor_FollowsConfig`, `TestRunner_RetryCountFollowsConfig`, `TestHandleExit_RetryBudgetFollowsConfig` |
| Deterministic Go, no agent | `TestNoForbiddenImports` in `internal/failure` |

## 8. Validation commands

```bash
go build ./...
go vet ./...
test -z "$(gofmt -l internal cmd)"
go test ./internal/failure/... -race -count=1
go test ./internal/github/... -race -count=1
go test ./internal/agent/... -race -count=1
go test ./internal/watcher/... -race -count=1
go test ./internal/guard/... -count=1     # guard docs test still passes after the guards.md edit
task test
task lint
task fmt:check
task sync:check
go run ./cmd/kairon plan parse .kairon/specs/issue-379-uniform-failure-contract.md
```

## 9. Risks and assumptions

- **Attempts versus retries.** The comment says "attempts made N of M" where M is `max_retries + 1`. Preserving the
  existing manager behavior (one initial run plus `max_retries` retries) was chosen over changing run counts.
- **`gh` JSON shape.** The design relies on `comments` (with `author.login`, `body`, `createdAt`) being available in
  `gh issue list/view --json`, and on the issue events API for `unlabeled`. The pure parsing helpers are tested with
  recorded JSON so a field rename is caught by tests, not in production.
- **List size.** Requesting `comments` enlarges the list payload. The call count per poll is unchanged; events are
  fetched only for the rare marker-without-label candidate.
- **Single-account setups.** If the human and Kairon use the same GitHub account, the human's comments also count as
  `ByViewer`. A marker is still required on the first line, which a person would not write by accident.
- **Watcher un-tracking.** Dropping `tracked` entries absent from the poll relies on `hasActiveWorktree` to stop a
  duplicate spawn if an issue briefly drops out of a 30-item `gh issue list` page. This is unchanged existing
  behavior at the list-size limit.
- **Out of scope, as stated in the issue.** The planner punch-out decision (#381), the read/verify boundary (#382),
  the `-failed`/`-done` label strings, and any change to krew-lead's QA loop prompt.

## 10. Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "failure-core"
    agent: "builder"
    description: "Create internal/failure with policy (config-driven max attempts), Decide, run id and marker helpers, IssueState.Classify (terminal-pending selection rule), RenderComment, and the Issues port/Target. Pure Go, no I/O. Write failing tests first (AGENTS.md TDD)."
    dependencies: []
    acceptance_criteria:
      - "internal/failure exists and imports only stdlib, internal/guard, internal/stageoutput and internal/config; TestNoForbiddenImports forbids direct os, os/exec, net, net/http imports"
      - "PolicyFromRetries(n) yields MaxAttempts n+1 (negative n treated as 0 retries) and PolicyFor follows config.MaxRetries and config.MaxQARetries independently (TestPolicyFor_FollowsConfig)"
      - "Decide returns PunchOut for Unrecoverable regardless of attempts, Retry while Attempt.Current < Attempt.Max for Recoverable, and PunchOut at Current == Max"
      - "Marker/ParseMarker round-trip, and ParseMarker rejects a marker that is not on the first line of the body"
      - "RenderComment output starts with the marker and contains the stage, guard, 'N of M' attempts, expected, actual, remedy and the fail label; fence longer than any backtick run in values, values truncated to 2000 runes (TestRenderComment_*)"
      - "IssueState.Classify returns TerminalPending for a ByViewer marker with no done/failed label and no later label removal, Eligible once FailLabelRemovedAt is at or after the newest marker, and ignores markers not authored by the viewer (TestClassify_*)"
      - "Each new test was observed failing for the expected reason before the implementation was written"
    validation_commands:
      - "go build ./internal/failure/..."
      - "go test ./internal/failure/... -race -count=1"
      - "go vet ./internal/failure/..."
      - 'test -z "$(gofmt -l internal/failure)"'

  - id: "failure-punchout"
    agent: "builder"
    description: "Add PunchOut (idempotent, comment-before-label, run-scoped marker), Reconcile, Route (guard.FailureHandler adapters declaring recoverable vs terminal), and Runner (synchronous Tier 1 loop that refuses to run while labeled or terminal-pending). Tests use a call-recording fake Issues."
    dependencies: ["failure-core"]
    acceptance_criteria:
      - "TestRunner_RecoversAfterTwoFailures: stage fails twice then succeeds, stage ran 3 times, no AddLabel and no comment"
      - "TestRunner_ExhaustionPunchesOut: always-failing stage runs exactly Policy.MaxAttempts times, exactly one comment is posted, AddLabel is called with the fail label, the stage is not called again, Outcome is OutcomePunchedOut"
      - "TestRunner_UnrecoverableSkipsRetries: a guard failure method that calls Route.Terminal causes one stage run and an immediate punch-out with zero retries"
      - "TestPunchOut_CommentBeforeLabel asserts call order; TestPunchOut_CommentFailureLeavesNoLabel asserts the label is never written when the comment write fails"
      - "TestPunchOut_RetryAfterPartialPostsNoDuplicate: comment succeeds, label fails, second call posts no second comment and applies the label; an already-present label is a no-op"
      - "TestPunchOut_NewRunIDPostsFreshComment: after the fake simulates label removal, a new run id posts its own comment"
      - "Runner.Run returns OutcomeHalted without calling the stage while the issue is labeled or terminal-pending; each Run starts the attempt counter at 1; no exported API re-enters after a punch-out (TestRunner_RefusesWhileLabeled, TestRunner_FreshRunStartsAtAttemptOne, TestRunner_NoAutoRetryAfterTerminal)"
      - "Runner overwrites the stage-reported Attempt with its own counter and returns an error wrapping guard.ErrInvalid for an invalid FailureContext without any GitHub write"
      - "Reconcile applies only the missing label for a terminal-pending issue and does nothing otherwise"
      - "A real guard.Run with guard.NewStructural(route.Terminal(...)) routes into the mechanism"
    validation_commands:
      - "go test ./internal/failure/... -race -count=1"
      - "go test ./internal/failure -run 'TestRunner|TestPunchOut|TestReconcile|TestRoute' -race -count=1 -v"
      - "go vet ./internal/failure/..."
      - 'test -z "$(gofmt -l internal/failure)"'

  - id: "github-adapter"
    agent: "builder"
    description: "In internal/github add the CLI adapter implementing failure.Issues (State via gh issue view plus lazy events lookup, PostComment via stdin, AddLabel) and SelectIssues returning Eligible and TerminalPending using failure.IssueState.Classify. ListIssues keeps its signature and returns Eligible. Keep JSON-to-classification logic pure and tested without gh."
    dependencies: ["failure-core"]
    acceptance_criteria:
      - "var _ failure.Issues = CLI{} compiles"
      - "ListIssues keeps its signature and existing -done/-failed exclusion; it requests the comments field and delegates to SelectIssues"
      - "TestSelectIssues_ExcludesTerminalPending: an issue with a trusted marker comment and no fail-state label is absent from Eligible and present in TerminalPending"
      - "TestSelectIssues_RestartedIssueIsEligible: a marker followed by a later unlabeled event for the fail-state label is Eligible"
      - "TestSelectIssues_IgnoresUntrustedMarker: a marker comment not authored by the authenticated user does not exclude the issue"
      - "TestSelectIssues_EventsLookupErrorFailsClosed: an events lookup error excludes that issue for the poll without failing the whole listing"
      - "Events are filtered by label name in Go (no label text interpolated into a jq program); comment bodies are passed on stdin; gh is invoked with an argument list, never a shell"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/github/... -race -count=1"
      - "go vet ./internal/github/..."
      - 'test -z "$(gofmt -l internal/github)"'

  - id: "manager-retrofit"
    agent: "builder"
    description: "Retrofit internal/agent/manager.go so both terminal paths (non-zero exit and exit 0 with no PR) build a guard.FailureContext, use failure.Decide, and on punch-out call failure.PunchOut (comment then label) outside the manager lock, then clear the issue retry count file. Add test seams issues, prExists, retry, cleanup and Agent.RunID."
    dependencies: ["failure-punchout", "github-adapter"]
    acceptance_criteria:
      - "TestHandleExit_ExhaustedRetriesPostsCommentThenLabel: with RetryCount == MaxRetries and a non-zero exit, the fake Issues records a comment (naming the stage and attempts) and then AddLabel(..., '<label>-failed')"
      - "TestHandleExit_NoPRExhaustedPostsCommentThenLabel covers exit 0 with no PR"
      - "TestHandleExit_RetriesRemainNoWrites: while retries remain, the retry seam is called once and no comment or label is written"
      - "TestHandleExit_RetryBudgetFollowsConfig: changing config.MaxRetries changes the number of retries before punch-out"
      - "The non-zero exit branch no longer holds m.mu across GitHub writes"
      - "A PunchOut error is logged and does not remove the retry count file; success removes it"
      - "Existing behavior for successful completion (-done label) is unchanged and existing agent tests still pass"
      - "No test invokes real gh, kiro-cli or git"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/agent/... -race -count=1"
      - "go test ./internal/agent -run 'TestHandleExit' -race -count=1 -v"
      - "go vet ./internal/agent/..."
      - 'test -z "$(gofmt -l internal/agent)"'

  - id: "watcher-restart"
    agent: "builder"
    description: "Update internal/watcher/watcher.go to use github.SelectIssues, reconcile terminal-pending issues through an injectable reconcile func (default failure.Reconcile with github.CLI) and reset their retry count file, and drop tracked entries for issues absent from the eligible list after a successful poll so clearing the fail-state label restarts the issue with a fresh budget."
    dependencies: ["failure-punchout", "github-adapter"]
    acceptance_criteria:
      - "TestReconcilePending_CallsReconcileAndResetsRetryCount: each terminal-pending issue is reconciled once and its .kairon/retries count file is removed on success"
      - "TestReconcilePending_ErrorDoesNotStopOthers: one reconcile error is logged and the remaining issues are still processed"
      - "TestUntrackAbsent_DropsExcludedKeepsEligible: tracked entries not in the eligible list are dropped; entries still eligible are kept; a poll error leaves tracked unchanged"
      - "TestRestartedIssueReentersWithFreshRetryBudget: after reset, hasExceededGlobalRetries is false for the issue"
      - "Terminal-pending issues are never passed to Spawn"
      - "Existing watcher and dependency tests still pass"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/watcher/... -race -count=1"
      - "go vet ./internal/watcher/..."
      - 'test -z "$(gofmt -l internal/watcher)"'

  - id: "failure-docs"
    agent: "builder"
    description: "Write docs/failure-contract.md (two-tier model, idempotent terminal writes, run-scoped marker, terminal-pending selection, restart by clearing the label), add internal/failure/docs_test.go that fails if required terms are missing, and update README.md (Failed lifecycle row, Retry Logic) and docs/guards.md (link instead of 'adds')."
    dependencies: ["manager-retrofit", "watcher-restart"]
    acceptance_criteria:
      - "docs/failure-contract.md exists and covers Tier 1, Tier 2, attempt semantics with max_retries and max_qa_retries, Route and Runner usage, comment-before-label, run id and marker, terminal-pending selection and Reconcile, and the restart-by-clearing-the-label flow"
      - "TestDocs_NamesRequiredTerms in internal/failure fails when a required term is removed from the doc and passes now"
      - "README.md Issue Lifecycle and Retry Logic sections state that the explanatory comment is posted before the failed label and link to docs/failure-contract.md"
      - "docs/guards.md no longer says #379 will add the handling, still contains the terms #378 and #379, and internal/guard docs tests still pass"
      - "No ```yaml fences in the new doc; no edits to .kiro/agents, .kairon/specs (other than the spec), CHANGELOG.md"
    validation_commands:
      - "go test ./internal/failure -run 'TestDocs' -count=1"
      - "go test ./internal/guard/... -count=1"
      - "test -f docs/failure-contract.md"
      - "grep -q 'failure-contract.md' README.md"

  - id: "validate-all"
    agent: "validator"
    description: "Independently verify the whole change: build, vet, format, race tests for every touched package, task test/lint/fmt:check/sync:check, plan parse of this spec, and map each of the 8 acceptance criteria and the config-driven-max constraint to a named passing test. Confirm the work's commits did not touch .kiro/agents or historical specs."
    dependencies: ["failure-docs"]
    acceptance_criteria:
      - "go build ./... and go vet ./... succeed and gofmt reports no files"
      - "go test -race passes for internal/failure, internal/github, internal/agent, internal/watcher and internal/guard"
      - "task test, task lint, task fmt:check and task sync:check all succeed"
      - "Each of AC 1 to 8 is demonstrated by the named test in the traceability table of the spec, run with -run and -v, showing PASS"
      - "The work adds no commits touching .kiro/agents/ (the worktree already carries unrelated uncommitted sandbox edits there; leave them as they are) and none touching .kairon/specs/ except issue-379-uniform-failure-contract.md"
      - "internal/guard non-test code is unchanged"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - 'test -z "$(gofmt -l internal cmd)"'
      - "go test -race -count=1 ./internal/failure/... ./internal/github/... ./internal/agent/... ./internal/watcher/... ./internal/guard/..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-379-uniform-failure-contract.md"
```
