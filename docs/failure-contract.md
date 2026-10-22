# Failure Contract

When a workflow stage fails, it ends in one of two ways. Either the stage is re-run automatically within a fixed
budget (Tier 1), or the workflow stops and tells a human why (Tier 2). This page describes that contract and the code
that enforces it in `internal/failure`.

The design background is `.kairon/specs/maturity-model/planner-requirements.md` §14 (failure contract), §4.1
(`FailureContext`) and §3.5 (a planner punch-out is a `FailureContext`). The package builds on `internal/guard`
([guards.md](guards.md)), which defines `FailureContext`, `Attempt` and `FailureHandler`.

The mechanism is deterministic Go. It does not call an agent or a model, and `internal/failure` does no I/O itself:
every GitHub effect goes through the small `failure.Issues` port, which `internal/github.CLI` implements with the `gh`
CLI.

## The two tiers

| Tier | Name | What happens | GitHub writes |
|------|------|--------------|---------------|
| Tier 1 | Bounded retry | The stage runs again while attempts remain. | None |
| Tier 2 | Punch-out | The workflow stops, posts a comment that says why, then adds the `kairon-failed` label. | One comment, then one label |

A failure goes to Tier 2 when either condition holds:

- The attempts are used up.
- The failing guard declared the failure unrecoverable. A planner punch-out is the main case (#381 builds on this).

After a punch-out, nothing re-enters the work by itself. Only a human removing the label restarts the issue.

## Attempt semantics

`max_retries` is the number of re-runs after the first run. It is not the number of runs. This matches what the agent
manager did before the contract existed: one initial run plus up to `max_retries` retries.

`guard.Attempt` requires `1 <= Current <= Max`, so the failure package counts in attempts:

| Config key | Default | Boundary | Policy |
|------------|---------|----------|--------|
| `max_retries` | 3 | `BoundaryImplementation` | `MaxAttempts = max_retries + 1` |
| `max_qa_retries` | 3 | `BoundaryQA` | `MaxAttempts = max_qa_retries + 1` |

`failure.PolicyFromRetries(n)` is the only place the `+1` is written. A negative value counts as zero retries, so one
attempt. `failure.PolicyFor(cfg, boundary)` reads the matching key from the config, so the limits are never hard-coded.
A nil config or an unknown boundary allows a single attempt, so a misconfiguration never grants a larger budget.

The comment a human reads says "Attempts made N of M". With the default `max_retries` of 3, a punched-out run reads
"4 of 4".

`BoundaryQA` has a policy but no Go caller yet. The builder-validator QA loop is still driven by the krew-lead
prompt, so `max_qa_retries` is read by `PolicyFor` and is not yet enforced by Go code on that loop.

## Deciding: retry or punch out

`failure.Decide(fc, disposition)` is a pure function and the single place the tier is chosen:

| Disposition | Attempts remaining | Decision |
|-------------|--------------------|----------|
| `Unrecoverable` | any | `DecisionPunchOut` |
| `Recoverable` | `Attempt.Current < Attempt.Max` | `DecisionRetry` |
| `Recoverable` | `Attempt.Current == Attempt.Max` | `DecisionPunchOut` |

An attempt that is not valid (for example `Max` of zero) fails toward a punch-out, never toward an extra retry.

There are two ways to reach `Decide`, and both share it and `PunchOut`:

- `failure.Runner.Run` is a synchronous loop. Use it at a boundary that can call a stage and wait.
- The agent `Manager` is event-driven. A `kiro-cli` process exits, then `HandleExit` decides whether to start a new
  process. It cannot use a loop, so it calls `Decide` and `PunchOut` directly (see
  [Where the manager fits](#where-the-manager-fits)).

## Routing a guard into the mechanism: Route

`guard.FailureHandler` is `func(FailureContext) error`, so the choice between recoverable and unrecoverable cannot
travel inside the context. The guard owns that choice by installing one of two handlers from a `failure.Route`:

- `route.Recoverable()` records the failure as one that may be retried.
- `route.Terminal(remedy)` records it as one that must punch out immediately. A planner punch-out calls it with
  `Attempt{1, 1}`.

The `Route` only records. The stage returns `route.Result()` to the `Runner`, which enacts it. Use a new `Route` for
each execution of a stage.

## Running a stage: Runner

```go
runner := failure.Runner{
	Issues: github.CLI{},
	Target: failure.Target{Repo: "owner/repo", Issue: 379, FailLabel: failure.FailedLabel("kairon")},
	Policy: failure.PolicyFor(cfg, failure.BoundaryImplementation),
	RunID:  failure.NewRunID(),
}

res, err := runner.Run(func(a guard.Attempt) failure.StageResult {
	route := &failure.Route{} // one Route per execution of the stage
	out := runStage()         // the stage's raw output
	_, _ = guard.Run(
		guard.Input{Stage: stageoutput.StagePlanner, Raw: out, Attempt: a},
		[]guard.Guard{guard.NewStructural(route.Recoverable())}, // route.Terminal("...") to punch out at once
	)
	return route.Result() // Failure is nil when no guard rejected the output
})
// res.Outcome is one of OutcomeSucceeded, OutcomePunchedOut,
// OutcomePunchOutPending and OutcomeHalted.
```

`Runner.Run` behaves as follows:

1. It reads the issue first. If the issue carries `-done` or `-failed`, or is terminal-pending, it returns
   `OutcomeHalted` without calling the stage.
2. The attempt counter starts at 1 on every call and lives only inside the call. A restarted issue is a fresh call
   with a fresh budget.
3. After each failure the runner overwrites `Failure.Attempt` with its own counter, so a stage cannot misreport it,
   and validates the context. An invalid context returns an error that wraps `guard.ErrInvalid`, with no GitHub write.
4. A retry writes nothing to GitHub. A punch-out calls `PunchOut` and the stage is never called again.
5. If `PunchOut` fails part way, the result is `OutcomePunchOutPending` plus the error. Whether `Reconcile` can
   finish it depends on how far `PunchOut` got: if the comment landed but the label write failed, the issue is
   terminal-pending and `Reconcile` applies the label later; if the state read or the comment write failed,
   nothing was written, so the issue stays eligible and re-runs with a fresh budget rather than being reconciled.

The caller must supply a run id (see below). There is no fallback.

## Tier 2: PunchOut

`failure.PunchOut(iss, target, runID, fc, remedy)` does these steps:

1. It validates the run id, the `FailureContext` and the target. Invalid input returns an error and writes nothing.
2. It reads the issue state.
3. If no comment by the authenticated user carries the marker for this run id, it posts the explanatory comment. If
   that write fails, it returns the error and does not touch the label.
4. If the `kairon-failed` label is already present, it does nothing more. Otherwise it adds the label.

### Comment before the label

The punch-out always writes the comment before the label, never after and never in parallel. The label is what takes
the issue out of the polling set, so an issue that has the label but no comment would sit there with no stated reason.
Posting the comment before the label means the worst partial failure is a comment with no label, which the next poll repairs.

### Idempotent terminal writes

`PunchOut` is idempotent for one run id. A call after a partial failure (comment posted, label write failed) posts
no second comment and applies only the label. A call when both are already in place is a no-op. It returns nil only
when the comment and the label are both in place.

### What the comment contains

`RenderComment` builds the body from the `FailureContext`. The first line is the run's marker. The rest names the
stage and guard, the attempts made, what was expected, what was actually found, the remedy, and how to restart.

````markdown
<!-- kairon:punch-out run=3f9c2a71b0d44e5a8c61d2e7 -->

**Kairon stopped work on this issue and needs a human.**

- Stage: `krew-lead`
- Guard: `implementation-run`
- Attempts made: 4 of 4

**Expected**

```
krew-lead exits 0 and a pull request exists for issue #379
```

**Actual**

```
krew-lead exited with code 1
```

**Remedy**

Read the agent log at .kairon/logs/issue-379.log to see why the run failed, fix the cause, then remove the `kairon-failed` label.

To restart this issue, fix the cause and remove the `kairon-failed` label. The workflow picks the issue up again with a fresh retry budget.
````

If the caller supplies no remedy, the default is "Make this true, then remove the label" followed by the expected
value.

Expected, Actual and the default remedy can carry agent output, so they are treated as untrusted text. Each is
truncated to 2000 runes and placed in a code fence that is longer than any run of backticks inside it, so the text
cannot close the fence. The stage, guard and label are collapsed to one line with backticks removed.

## Run id and marker

The marker is the line `<!-- kairon:punch-out run=<id> -->`, and it is always the first line of a punch-out comment.
The run id is generated once per terminal run by `failure.NewRunID()`. It is made of the characters `A-Z a-z 0-9 . _ -`
and is at most 128 characters long. The agent `Manager` generates one per `Agent` at `Spawn` and reuses it for every
retry of that agent.

The marker is scoped to a run id, not to a digest of the failure. A digest would be identical when the same problem
recurs after a human restart, and it would wrongly suppress the new explanation. With a run id, each terminal run
posts its own comment, and retries inside one run post exactly one.

Anyone who can comment on an issue could paste a marker, so a marker only counts when both are true:

- The comment was written by the authenticated `gh` user (`Comment.ByViewer`).
- The marker is the entire first line of the body (`ParseMarker` rejects it anywhere else).

## Terminal-pending selection

An issue is terminal-pending when a punch-out started but did not finish: the explanatory comment exists and the
`kairon-failed` label does not. `IssueState.Classify` decides this from GitHub state:

1. If the issue has the `-done` label, it is done. If it has the `-failed` label, it is failed. Either is excluded.
2. Otherwise, if there is no trusted marker comment, the issue is eligible.
3. Otherwise the issue is terminal-pending unless the failed label was removed at or after the newest trusted marker.
   A tie counts as removed, because the label can only be applied after the comment.

"Marker present and label absent" is not enough on its own. After a human clears the label, the old marker comment
remains, and it would read as pending forever. The removal time comes from the issue events API (`unlabeled` events).
Both timestamps are GitHub server times, so clock skew does not matter. The events are fetched only for an issue that
has a trusted marker and neither excluded label, and they are filtered by label name in Go, never interpolated into a
`jq` program.

`github.SelectIssues` returns two slices, `Eligible` and `TerminalPending`. A terminal-pending issue is never started.
If the events lookup fails for one issue, that issue is left out of this poll and the error is logged. The poll fails
closed rather than risk starting a stage in the middle of a reconcile.

### Reconcile

`failure.Reconcile(iss, target)` finishes a half-done punch-out. For a terminal-pending issue it applies only the
missing label and reports that it did. In any other state it writes nothing. The watcher calls it for each
terminal-pending issue on every poll. When it succeeds, the watcher also removes the issue's
`.kairon/retries/issue-<number>.count` file. One failing issue is logged and does not stop the others.

## Restart by clearing the label

The label is the only way back in. To restart a punched-out issue:

1. Fix the cause named in the comment.
2. Remove the `kairon-failed` label from the issue.

The next poll sees an eligible issue, because the removal time is at or after the marker. Two pieces of state would
otherwise block the restart, and both are reset:

- The watcher keeps an in-memory set of tracked issues. After each successful poll it drops entries for issues absent
  from the eligible list, so a punched-out issue (absent because it is labeled) is forgotten and is picked up again
  once the label is gone. A poll that errors leaves the set untouched.
- The persisted `.kairon/retries/issue-<number>.count` file is removed when a punch-out completes (manager) or is
  reconciled (watcher). Without that, the issue would be skipped silently after `max_retries` restarts.

The restarted run starts at attempt 1 with a new run id, so if it fails again the human gets a new comment.

## Where the manager fits

`internal/agent.Manager.HandleExit` has two failure paths: a non-zero exit, and exit 0 with no pull request. Both build a
`FailureContext` (stage `krew-lead`, guard `implementation-run`, attempt `RetryCount+1` of `max_retries+1`) and call
`failure.Decide`.

- On a retry, the manager increments its counter and starts a new process. No label, no comment.
- On a punch-out, it releases its lock and then calls `failure.PunchOut` with a remedy that points at
  `.kairon/logs/issue-<number>.log`. On success it removes the retry count file. On error it logs that the punch-out
  will be reconciled on the next poll and leaves the retry count file alone.

The `-done` path is unchanged.

## Known limits

- The builder-validator QA loop is still driven by the krew-lead prompt. `max_qa_retries` has a policy
  (`BoundaryQA`) but no Go caller yet.
- Values in the comment are truncated to 2000 runes each.
- Selection fails closed. An issue whose events lookup fails is skipped for that poll and tried again on the next.
- If the human and Kairon use the same GitHub account, the human's comments also count as the authenticated user's. A
  marker on the first line is still required, which a person would not write by accident.
- The planner punch-out decision (#381) and the read/verify boundary (#382) are follow-ups. #382 will consume the
  terminal-pending rule.
