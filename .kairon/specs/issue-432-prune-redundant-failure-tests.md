# Prune redundant internal/failure tests

**Issue**: #432 - Prune redundant internal/failure tests
**Closes**: #432

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/failure`: `marker_test.go`, `policy_test.go`, `punchout_test.go`, `render_test.go`, `route_test.go`, `runner_test.go`, `state_test.go`" | `prune-marker-policy`, `prune-punchout-render`, `prune-route-runner`, `prune-state` |
| AC1 | "The 9 tests marked Delete no longer exist." | `prune-marker-policy` (`TestDispositionZeroValueIsRecoverable`, `TestPolicyAttempt`, `TestPolicyFromRetries`), `prune-punchout-render` (`TestPunchOut_NewRunIDPostsFreshComment`, `TestRenderComment_FirstLineIsOnlyMarker`), `prune-route-runner` (`TestRoute_ResultDoesNotAliasRecordedContext`, `TestRunner_NoAutoRetryAfterTerminal`, `TestRunner_StageAttemptArgumentCountsFromOne`), `prune-state` (`TestSelectionExcluded`), `validate-rows` |
| AC2 | "Each of the 15 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-marker-policy`, `prune-punchout-render`, `prune-route-runner`, `prune-state`, `validate-rows` |
| AC3 | "Package coverage does not drop." | `prune-marker-policy`, `prune-punchout-render`, `prune-route-runner`, `prune-state`, `validate-gate` |
| AC4 | "Every remaining test passes unchanged, and the gate passes." | `prune-marker-policy`, `prune-punchout-render`, `prune-route-runner`, `prune-state`, `validate-gate` |
| AC5 | "No production code changes." | `prune-marker-policy`, `prune-punchout-render`, `prune-route-runner`, `prune-state`, `validate-gate` |

## Deferred work

none

## Problem

The test audit found 24 tests in `internal/failure` that carry no weight: 9 duplicate another test or check a
constant or their own setup (Delete), and 15 are one-input copies of a test that belongs in a table (Merge). This
spec removes the 9 and folds the 15 into their named partners. Only `internal/failure/*_test.go` files change.
Sources read: the seven test files, `fake_test.go`, `.kairon/research/test-audit-verdicts.md` (the `failure` rows)
and `.kairon/research/test-performance-analysis.md` §8.

## Solution Approach

Treat this as a refactor of test code. The existing tests are the characterization suite: each builder runs
`go test ./internal/failure` before editing (must pass) and after (must pass).

1. Work file by file. The four builder tasks touch disjoint files, so they run in parallel and cannot conflict.
2. A Delete removes the whole function. A Merge turns the partner into a table (or adds a subtest) whose rows carry
   the merged test's distinct input and every assertion the merged test made. Anything the partner already asserts
   is dropped, as the issue says.
3. Partner function names do not change (AC2 verifies them with `go test -run '^<partner>$' -v`), even where the
   name now undersells the content (`TestClassify_DoneAndFailed`, `TestDecide_InvalidAttemptFailsToPunchOut`,
   `TestRoute_TerminalRecordsDispositionAndRemedy`). Update the doc comment above each such partner to say what
   the table covers.
4. Do not edit helpers in `fake_test.go`, `renderFC`, `testFC`, `fcAt`, `failing`, `newRunner`. Other tests that
   are not in the table depend on them.
5. Remove imports that a deletion leaves unused (`regexp` in `render_test.go` is the known one). `go vet` and
   `gofmt -l` must stay clean.

### Coverage measurements taken while designing

Baseline `go test -cover ./internal/failure` is exactly 98.1%, so there is no slack.

- Skipping only the 9 Delete tests (`go test -cover -skip '^(...)$'`) still reports 98.1%. No Delete is
  coverage-critical.
- Skipping `TestPunchOut_StateErrorWritesNothing` alone drops the package to 97.6%. It is the only test that
  reaches the `State` error return inside `PunchOut`. The `state error` row added to
  `TestPunchOut_InvalidInputWritesNothing` must therefore call `PunchOut` with a fake whose `State` fails, so that
  branch stays covered.
- Skipping `TestParseMarker_RejectsNonFirstLine` alone or `TestRunner_OverwritesStageAttempt` alone leaves
  coverage at 98.1%.

## Relevant Files

All paths are under `internal/failure/`. Nothing else may change.

| File | Change |
|------|--------|
| `marker_test.go` | merge 1 test |
| `policy_test.go` | delete 3, merge 3 |
| `punchout_test.go` | delete 1, merge 1 |
| `render_test.go` | delete 1, merge 1; remove the `regexp` import |
| `route_test.go` | delete 1, merge 2 |
| `runner_test.go` | delete 2, merge 2 |
| `state_test.go` | delete 1, merge 5 |

Read-only context: `fake_test.go`, `deps_test.go`, `docs_test.go`, and the production files (`marker.go`,
`policy.go`, `punchout.go`, `render.go`, `route.go`, `runner.go`, `state.go`), which must not be edited.

The `.kiro/agents/*.json` files show as modified in the worktree before any work begins. They are sandbox credential
injection (the MCP creds-agent server config) and are specific to this environment, so they must never be committed.
They are not part of this issue. Builders must not edit, stage or commit them. Stage test files by explicit path
(`git add internal/failure/<file>_test.go`), never `git add -A` or `git commit -a`.

The set of paths that may appear in this branch's final commit is exactly:

- `internal/failure/*_test.go` (the seven files above), and
- the issue's own spec file `.kairon/specs/issue-432-*.md`. This is the one allowed extra path: AGENTS.md
  "Exceptions" lets the architect write the spec for the issue being processed, and it is normal pipeline output.
  The issue's AC5 `Verify:` line (`git diff --name-only <base-branch>...HEAD` lists only
  `internal/failure/*_test.go`) is read with that single exception.

The modified `.kiro/agents/*.json` files are **excluded paths**: `krew-lead` does not stage them (see Team
Orchestration, "Final commit").

## Row-by-row design (all 24 rows of the Current / Target State table)

### `marker_test.go` and `policy_test.go` (task `prune-marker-policy`)

| # | Test | Action | Design |
|---|------|--------|--------|
| 1 | `TestParseMarker_RejectsNonFirstLine` | Merge into `TestParseMarker_RejectsMalformed` | Delete the function. In `TestParseMarker_RejectsMalformed`, add `m := Marker("abc")` before the list and append the six bodies: `"text\n"+m`, `"\n"+m`, `" "+m`, `"quote: "+m`, `"> "+m`, `"intro\n\n"+m+"\nmore"`. One loop, one `Errorf`. |
| 2 | `TestPolicyFromRetries` | Delete | Remove the function. `PolicyFromRetries` is still exercised by `newRunner`, `TestRoute_GuardRunRoutesIntoMechanism` and `TestRunner_RetryCountFollowsConfig`. |
| 3 | `TestPolicyAttempt` | Delete | Remove the function. `guard` is still imported by `fcAt`. |
| 4 | `TestDecide_UnrecoverableAlwaysPunchesOut` | Merge into `TestDecide_InvalidAttemptFailsToPunchOut` | See the Decide table below. |
| 5 | `TestDecide_RecoverableRetriesWhileAttemptsRemain` | Merge into `TestDecide_InvalidAttemptFailsToPunchOut` | See below. |
| 6 | `TestDecide_RecoverablePunchesOutAtMax` | Merge into `TestDecide_InvalidAttemptFailsToPunchOut` | See below. |
| 7 | `TestDispositionZeroValueIsRecoverable` | Delete | Remove the function. |

Decide table. Convert `TestDecide_InvalidAttemptFailsToPunchOut` to
`cases := []struct{ name string; attempt guard.Attempt; disp Disposition; want Decision }` and run each row as
`t.Run(c.name, ...)` with `fc := fcAt(1, 1); fc.Attempt = c.attempt; Decide(fc, c.disp)`. Keep the comment that an
unusable attempt must never produce an unbounded retry. Rows, with the `t.Run` names to use:

| name | attempt | disposition | want |
|------|---------|-------------|------|
| `unrecoverable 1 of 4` | {1,4} | Unrecoverable | DecisionPunchOut |
| `unrecoverable 2 of 4` | {2,4} | Unrecoverable | DecisionPunchOut |
| `unrecoverable 1 of 1` | {1,1} | Unrecoverable | DecisionPunchOut |
| `unrecoverable 4 of 4` | {4,4} | Unrecoverable | DecisionPunchOut |
| `recoverable 1 of 4` | {1,4} | Recoverable | DecisionRetry |
| `recoverable 2 of 4` | {2,4} | Recoverable | DecisionRetry |
| `recoverable 3 of 4` | {3,4} | Recoverable | DecisionRetry |
| `recoverable 4 of 4` | {4,4} | Recoverable | DecisionPunchOut |
| `recoverable 1 of 1` | {1,1} | Recoverable | DecisionPunchOut |
| `invalid zero attempt` | {} | Recoverable | DecisionPunchOut |
| `invalid current above max` | {5,4} | Recoverable | DecisionPunchOut |
| `invalid zero max` | {1,0} | Recoverable | DecisionPunchOut |

This keeps all 4 + 3 + 2 + 3 inputs and their expected decisions. Error messages print the attempt, disposition,
got and want.

### `punchout_test.go` and `render_test.go` (task `prune-punchout-render`)

| # | Test | Action | Design |
|---|------|--------|--------|
| 8 | `TestPunchOut_NewRunIDPostsFreshComment` | Delete | Remove the function. `TestRunner_FreshRunStartsAtAttemptOne` (must stay untouched) covers the same path and asserts posted IDs `[run1 run2]`. |
| 9 | `TestPunchOut_StateErrorWritesNothing` | Merge into `TestPunchOut_InvalidInputWritesNothing` | Add a row `state error` and delete the function. See below. |
| 10 | `TestRenderComment_AttemptsReflectContext` | Merge into `TestRenderComment_ContainsStageAttemptsExpectedActualRemedy` | In the main test, after `fc := renderFC()`, set `fc.Attempt = guard.Attempt{Current: 2, Max: 3}` and replace `"4 of 4"` with `"2 of 3"` in the `want` list. Do not change `renderFC()` itself: other tests use it. Delete the function. |
| 11 | `TestRenderComment_FirstLineIsOnlyMarker` | Delete | Remove the function and the `regexp` import. The main render test already checks first line == `Marker(run)` and the `ParseMarker` round trip. |

`state error` row. `TestPunchOut_InvalidInputWritesNothing` currently asserts "any call to the fake is a failure"
(`len(f.calls) != 0`). A State-failure row has to make one `State` call, so extend the case struct with a field
(for example `wantErr error`, or a `stateErr bool` that does `f.failNext("State", 1)` when the fake is built).
Rules that keep every assertion of both tests:

- Invalid-input rows keep today's check: an error is returned and `len(f.calls) == 0`.
- The `state error` row builds a fake with `failNext("State", 1)`, calls `PunchOut(f, testTarget, "run1", testFC(), "")`,
  asserts `errors.Is(err, errBoom)`, and asserts `len(f.writes()) == 0` (a `State` read is expected, so the
  "no calls at all" check does not apply to this row). `errors` is already imported.
- This row is the only thing that keeps the `State` error branch of `PunchOut` covered (see the measurements
  above).

### `route_test.go` and `runner_test.go` (task `prune-route-runner`)

| # | Test | Action | Design |
|---|------|--------|--------|
| 12 | `TestRoute_RecoverableRecordsContext` | Merge into `TestRoute_TerminalRecordsDispositionAndRemedy` | See Route table below. |
| 13 | `TestRoute_ResultIsSuccessWhenNoFailureRecorded` | Merge into `TestRoute_TerminalRecordsDispositionAndRemedy` | See below. |
| 14 | `TestRoute_ResultDoesNotAliasRecordedContext` | Delete | Remove the function. |
| 15 | `TestRunner_RefusesWhileTerminalPending` | Merge into `TestRunner_RefusesWhileLabeled` | Add a third subtest. See below. |
| 16 | `TestRunner_NoAutoRetryAfterTerminal` | Delete | Remove the function. `TestRunner_ExhaustionPunchesOut` re-runs after the punch-out and asserts halted with no extra stage call. |
| 17 | `TestRunner_OverwritesStageAttempt` | Merge into `TestRunner_ExhaustionPunchesOut` | Delete the function; add the comment check below. |
| 18 | `TestRunner_StageAttemptArgumentCountsFromOne` | Delete | Remove the function. `TestRunner_FreshRunStartsAtAttemptOne` already `DeepEqual`s the attempts passed to the stage. |

Route table. Convert `TestRoute_TerminalRecordsDispositionAndRemedy` to a table over three rows run with
`t.Run`. Each row builds `var r Route`, `fc := testFC()`, optionally invokes a handler, then asserts on
`r.Result()`:

| name | handler call | want Failure | want Disposition | want Remedy |
|------|--------------|--------------|------------------|-------------|
| `recoverable` | `r.Recoverable()(fc)`, error is fatal | `*got.Failure == fc` | Recoverable | `""` |
| `terminal` | `r.Terminal("do the thing")(fc)`, error is fatal | `*got.Failure == fc` | Unrecoverable | `"do the thing"` |
| `nothing recorded` | none | `nil` | not asserted | not asserted |

Do not assert `Disposition` on the `nothing recorded` row. The issue deletes
`TestDispositionZeroValueIsRecoverable` because production never relies on the zero value, and asserting it here
would bring that dependence back. For the two handler rows keep the `got.Failure == nil || *got.Failure != fc`
check, so a nil `Failure` fails instead of panicking. Use a function field (for example
`call func(r *Route, fc guard.FailureContext) error`, nil for the last row). `TestRoute_LastHandlerCallWins` and
`TestRoute_GuardRunRoutesIntoMechanism` stay unchanged, and their imports (`guard`, `stageoutput`) stay in use.

`TestRunner_RefusesWhileLabeled`. Change the loop over `[]string{testFailLabel, "kairon-done"}` into a table of
`{name string; seed func(*fakeIssues)}`:

| name | seed |
|------|------|
| `kairon-failed` (keep the current subtest name `testFailLabel`) | `f.seedLabel(testFailLabel)` |
| `kairon-done` | `f.seedLabel("kairon-done")` |
| `terminal pending marker` | `f.seedComment(Marker("earlier")+"\n", true)` with the comment "comment landed, label did not" |

All three subtests run the existing assertions (no error, `OutcomeHalted`, `Attempts == 0`, stage not called, no
writes). The old terminal-pending test asserted a subset of these, so the stricter shared assertions must also
hold for the marker row. If one does not, that exposes a production behavior difference. Do not weaken the
assertion and do not change production code. Stop and report BLOCKED in the sentinel.

`TestRunner_ExhaustionPunchesOut`. After the existing `res.Failure` attempt check (4 of 4), add:

```go
body := f.comments[0].Body
if !strings.Contains(body, "4 of 4") || strings.Contains(body, "99") {
    t.Errorf("comment used the stage's attempt, not the runner's:\n%s", body)
}
```

`failing()` reports 99 of 99, so this keeps the "comment says N of N, not 99" check. `strings` is already
imported in `runner_test.go`. Keep the existing re-run assertions.

### `state_test.go` (task `prune-state`)

| # | Test | Action | Design |
|---|------|--------|--------|
| 19 | `TestClassify_NoMarkerIsEligible` | Merge into `TestClassify_DoneAndFailed` | Rows `no marker` and `empty state`. |
| 20 | `TestClassify_LabelRemovedBeforeMarkerStaysPending` | Merge into `TestClassify_DoneAndFailed` | Row `label removed before marker stays pending`. |
| 21 | `TestClassify_NewMarkerAfterRestartIsPending` | Merge into `TestClassify_DoneAndFailed` | Row `new marker after restart is pending`. |
| 22 | `TestClassify_UsesNewestMarkerRegardlessOfOrder` | Merge into `TestClassify_DoneAndFailed` | Row `newest marker regardless of order`. |
| 23 | `TestClassify_IgnoresMarkerNotByViewer` | Merge into `TestClassify_DoneAndFailed` | Row `marker not by viewer is ignored`. |
| 24 | `TestSelectionExcluded` | Delete | Remove the function; its content is covered by the `excluded` column below. |

Table. Change `TestClassify_DoneAndFailed` from `{name, labels, want}` with a fixed comment list to
`{name string; state IssueState; want Selection; excluded bool}`, run each row with `t.Run(c.name, ...)`, and
assert both `Classify(doneL, failedL) == want` and `Excluded() == c.excluded` (an explicit column, not
`want != SelectEligible`, so the check is not a copy of the implementation). Build comments with the existing
`viewerMarker(runID, at)` helper. Rows:

| name | state | want | excluded |
|------|-------|------|----------|
| `done` | labels {kairon, doneL}; comments [viewerMarker r1 t0] | SelectDone | true |
| `failed` | labels {kairon, failedL}; comments [viewerMarker r1 t0] | SelectFailed | true |
| `failed wins over pending` | labels {failedL}; comments [viewerMarker r1 t0] | SelectFailed | true |
| `done without comments` | labels {doneL}; no comments | SelectDone | true |
| `no marker` | labels {kairon}; comments [{Body "hello", CreatedAt t0, ByViewer true}] | SelectEligible | false |
| `empty state` | `IssueState{}` | SelectEligible | false |
| `label removed before marker stays pending` | labels {kairon}; comments [viewerMarker r1 t1]; FailLabelRemovedAt t0 (comment: a removal older than the newest marker belongs to an earlier run) | SelectTerminalPending | true |
| `new marker after restart is pending` | labels {kairon}; comments [viewerMarker r1 t0, viewerMarker r2 t2]; FailLabelRemovedAt t1 (r1 punched out at t0, a human cleared the label at t1, r2 punched out again at t2) | SelectTerminalPending | true |
| `newest marker regardless of order` | same as the previous row with the comments reversed: [r2 t2, r1 t0] | SelectTerminalPending | true |
| `marker not by viewer is ignored` | labels {kairon}; one comment `viewerMarker("r1", t0)` with `ByViewer = false` | SelectEligible | false |

All four `Selection` values appear with the right `excluded` flag, which is what `TestSelectionExcluded` checked.
The other `Classify` tests (`TerminalPendingIsExcluded`, `LabelClearedAfterMarkerIsEligible`,
`IgnoresViewerCommentWithMarkerNotOnFirstLine`, `UntrustedMarkerDoesNotHideTrustedOne`) are not in the table and
stay unchanged.

## Team Orchestration

- `prune-marker-policy`, `prune-punchout-render`, `prune-route-runner` and `prune-state` have no dependencies
  and edit disjoint files. They can run in parallel.
- Parallel builders share one git index. Each commits only its own files by explicit path and retries if it
  hits `index.lock`. A builder does not run `git add -A`, `git commit -a`, `git stash` or `git checkout --`, so
  it cannot sweep up the modified `.kiro/agents/*.json` files or another builder's in-progress work.
- Commit messages are conventional (`test(failure): ...`). Name the tests the builder relied on as the
  characterization suite (a refactor, per AGENTS.md). No test is added first because no behavior changes.
- `validate-rows` and `validate-gate` depend on all four builder tasks and are read-only. They can run in
  parallel with each other.

### Final commit (instruction for `krew-lead`)

The `.kiro/agents/*.json` modifications are environment-specific sandbox credential injection and must never reach
a commit. `krew-lead` therefore stages the final commit **by explicit path only** and does not run `git add -A`
(step 7.1 of its prompt) or `git commit -a` for this issue:

```bash
git add -- internal/failure/*_test.go .kairon/specs/issue-432-*.md
git commit -m "test(failure): prune redundant tests ..."
```

Those two path patterns are the whole set that may be committed. The builders have normally committed the test
files already, so this step mostly stages the spec file. `krew-lead` leaves the `.kiro/agents/*.json` files as
unstaged working-tree changes. It still applies its binary-file check to what it staged. If `git status --porcelain`
shows any other path, `krew-lead` does not stage it and reports it.

`validate-gate` checks this same set before `krew-lead` commits (AC5 below), so the check covers what will actually
be committed: committed changes, staged changes, unstaged changes and untracked files.

### Environment: how AC4 is verified

The worktree is not a clean tree. The modified `.kiro/agents/*.json` files no longer match
`cmd/kairon/templates/kiro/agents/`, so `internal/planner` tests that compare the live configs with the templates
fail in the worktree whatever this issue changes. Which tests fail depends on which agent files the environment
has modified, so this spec does not list them.

**AC4's pass/fail evidence is the exit code of `task test` in a git-preserving clean export.** The export is a
`git clone --no-hardlinks` of the worktree, with the working tree's `internal/failure/*_test.go` copied over it:

- The clone holds the committed tree and no uncommitted edits, so the `.kiro/agents/*.json` modifications are not in
  it. It contains the change under test because the builders' test files are copied in.
- The clone keeps `.git`. A plain `git archive | tar -x` export has no `.git`, and then about 55 `internal/eval`
  tests fail with `failed to get git hash: exit status 128` because they run `git rev-parse`. Do not use a
  `.git`-less export.
- A non-zero `task test` exit code in the export is a FAIL for AC4. There is no carve-out.
- A zero exit code in the export is a PASS for AC4, whatever the worktree shows.
- Run time: `task test` runs `go test -v -race ./...`. A parallel full run of `internal/eval` takes about 3
  minutes, and a loaded machine about 8. The whole run took 10 minutes 9 seconds on this machine (`internal/eval`
  531 s, `internal/eval/sandbox` 146 s, exit code 0, on the unchanged HEAD). The validator allows at least 15
  minutes and does not stop early or fall back to a weaker check.
- Read the exit code, not the text. The verbose log contains lines such as `FAIL selftest-fail: 8.3% ...` that
  come from passing eval self-tests. A failing package prints `FAIL\tgithub.com/...` on its own line and makes the
  exit code non-zero.

The worktree run is informational only. `validate-gate` may run `go test -count=1 ./...` in the worktree and
reports the failures it actually sees (package and test names). For each worktree failure it confirms that the same
test passes in the export (`--- PASS: <name>` in the export log), and states that the cause is the environment's
modified agent files. A worktree failure is never a reason to mark AC4 failed, and a clean worktree run never
replaces the export. `go test -race -count=1 ./internal/failure` still runs directly in the worktree and must pass.

## Step-by-Step Task Breakdown

1. `prune-marker-policy` (builder): rows 1-7 in `marker_test.go` and `policy_test.go`.
2. `prune-punchout-render` (builder): rows 8-11 in `punchout_test.go` and `render_test.go`.
3. `prune-route-runner` (builder): rows 12-18 in `route_test.go` and `runner_test.go`.
4. `prune-state` (builder): rows 19-24 in `state_test.go`.
5. `validate-rows` (validator): AC1 and AC2, plus a review that every merged assertion survives.
6. `validate-gate` (validator): AC3, AC4 (`task test` in a git-preserving clean export, 3 to 10 minutes), AC5
   (the set of paths that will be committed).

## Validation Commands

Run from the repository root (worktree).

```bash
# AC1: prints nothing (grep exits 1)
! grep -nE 'func (TestDispositionZeroValueIsRecoverable|TestPolicyAttempt|TestPolicyFromRetries|TestPunchOut_NewRunIDPostsFreshComment|TestRenderComment_FirstLineIsOnlyMarker|TestRoute_ResultDoesNotAliasRecordedContext|TestRunner_NoAutoRetryAfterTerminal|TestRunner_StageAttemptArgumentCountsFromOne|TestSelectionExcluded)\(' internal/failure/*_test.go

# AC2: prints nothing
! grep -nE 'func (TestClassify_IgnoresMarkerNotByViewer|TestClassify_LabelRemovedBeforeMarkerStaysPending|TestClassify_NewMarkerAfterRestartIsPending|TestClassify_NoMarkerIsEligible|TestClassify_UsesNewestMarkerRegardlessOfOrder|TestDecide_RecoverablePunchesOutAtMax|TestDecide_RecoverableRetriesWhileAttemptsRemain|TestDecide_UnrecoverableAlwaysPunchesOut|TestParseMarker_RejectsNonFirstLine|TestPunchOut_StateErrorWritesNothing|TestRenderComment_AttemptsReflectContext|TestRoute_RecoverableRecordsContext|TestRoute_ResultIsSuccessWhenNoFailureRecorded|TestRunner_OverwritesStageAttempt|TestRunner_RefusesWhileTerminalPending)\(' internal/failure/*_test.go

# AC2: each partner passes, and its merged cases show up as subtests
go test -count=1 -run '^TestClassify_DoneAndFailed$' -v ./internal/failure
go test -count=1 -run '^TestDecide_InvalidAttemptFailsToPunchOut$' -v ./internal/failure
go test -count=1 -run '^TestPunchOut_InvalidInputWritesNothing$' -v ./internal/failure
go test -count=1 -run '^TestRoute_TerminalRecordsDispositionAndRemedy$' -v ./internal/failure
go test -count=1 -run '^TestRunner_RefusesWhileLabeled$' -v ./internal/failure
go test -count=1 -run '^(TestParseMarker_RejectsMalformed|TestRenderComment_ContainsStageAttemptsExpectedActualRemedy|TestRunner_ExhaustionPunchesOut)$' -v ./internal/failure

# AC3: reports at least 98.1%
go test -count=1 -cover ./internal/failure

# AC4: the gate. Pass/fail evidence is this command's exit code; see "Environment: how AC4 is verified".
# Clean export WITH git metadata (git clone, not git archive); allow 15 minutes (measured 10 min 9 s; 3 to 8 min when less loaded).
D=$(mktemp -d); L=$(mktemp)
git clone -q --no-hardlinks "$PWD" "$D" && rm -f "$D"/internal/failure/*_test.go && cp internal/failure/*_test.go "$D/internal/failure/" && (cd "$D" && task test) >"$L" 2>&1
rc=$?; tail -n 30 "$L"; echo "export task test exit code: $rc (log: $L)"
rm -rf "$D"            # keep "$L" until the worktree failures have been checked against it, then rm -f "$L"
test "$rc" -eq 0

# AC5: every path that will be committed. Run from the repository root before krew-lead commits.
# (a) committed: only test files plus the issue's own spec file
test -z "$(git diff --name-only origin/main...HEAD | grep -vE '^(internal/failure/[A-Za-z0-9_]+_test\.go|\.kairon/specs/issue-432-[^/]+\.md)$')"
test -z "$(git diff --name-only origin/main...HEAD -- .kiro)"
# (b) staged, unstaged and untracked: same allowed set; unstaged-only .kiro/agents/*.json are reported as excluded
git status --porcelain --untracked-files=all
git status --porcelain --untracked-files=all | awk '
  { xy = substr($0, 1, 2); p = substr($0, 4) }
  p ~ /^internal\/failure\/[A-Za-z0-9_]+_test\.go$/ { next }
  p ~ /^\.kairon\/specs\/issue-432-[^\/]+\.md$/ { next }
  xy == " M" && p ~ /^\.kiro\/agents\/[^\/]+\.json$/ { print "EXCLUDED (krew-lead will not stage): " p > "/dev/stderr"; next }
  { print "NOT ALLOWED: " xy " " p; bad = 1 }
  END { exit bad }'
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "prune-marker-policy"
    agent: "builder"
    description: "In internal/failure/marker_test.go and policy_test.go: merge TestParseMarker_RejectsNonFirstLine into TestParseMarker_RejectsMalformed (append its six bodies to the one input list); delete TestPolicyFromRetries, TestPolicyAttempt and TestDispositionZeroValueIsRecoverable; merge TestDecide_UnrecoverableAlwaysPunchesOut, TestDecide_RecoverableRetriesWhileAttemptsRemain and TestDecide_RecoverablePunchesOutAtMax into TestDecide_InvalidAttemptFailsToPunchOut as a t.Run table with the 12 rows in the spec. Edit only these two files. Do not touch .kiro/agents/*.json. Commit only these two files by explicit path."
    dependencies: []
    acceptance_criteria:
      - "TestParseMarker_RejectsNonFirstLine, TestPolicyFromRetries, TestPolicyAttempt, TestDispositionZeroValueIsRecoverable, TestDecide_UnrecoverableAlwaysPunchesOut, TestDecide_RecoverableRetriesWhileAttemptsRemain and TestDecide_RecoverablePunchesOutAtMax no longer exist as functions"
      - "TestParseMarker_RejectsMalformed rejects all 10 original malformed bodies and the 6 former non-first-line bodies in one loop"
      - "TestDecide_InvalidAttemptFailsToPunchOut runs the 12 spec rows as subtests (4 unrecoverable, 3 recoverable retry, 2 recoverable punch-out, 3 invalid attempts) with the same expected decisions as the removed tests"
      - "go test ./internal/failure passes and gofmt and go vet report nothing for the package"
      - "Only internal/failure/marker_test.go and internal/failure/policy_test.go are modified and committed; no production file and no .kiro/agents/*.json file is staged"
    validation_commands:
      - "! grep -nE 'func (TestParseMarker_RejectsNonFirstLine|TestPolicyFromRetries|TestPolicyAttempt|TestDispositionZeroValueIsRecoverable|TestDecide_UnrecoverableAlwaysPunchesOut|TestDecide_RecoverableRetriesWhileAttemptsRemain|TestDecide_RecoverablePunchesOutAtMax)\\(' internal/failure/*_test.go"
      - "go test -count=1 -run '^(TestParseMarker_RejectsMalformed|TestDecide_InvalidAttemptFailsToPunchOut)$' -v ./internal/failure"
      - "go test -count=1 ./internal/failure"
      - "go vet ./internal/failure"
      - "test -z \"$(gofmt -l internal/failure)\""
      - "go test -count=1 -cover ./internal/failure"

  - id: "prune-punchout-render"
    agent: "builder"
    description: "In internal/failure/punchout_test.go and render_test.go: delete TestPunchOut_NewRunIDPostsFreshComment and TestRenderComment_FirstLineIsOnlyMarker (remove the now-unused regexp import); merge TestPunchOut_StateErrorWritesNothing into TestPunchOut_InvalidInputWritesNothing as a 'state error' row whose fake has failNext(\"State\", 1), asserts errors.Is(err, errBoom) and zero writes (this row keeps the PunchOut State-error branch covered); merge TestRenderComment_AttemptsReflectContext into TestRenderComment_ContainsStageAttemptsExpectedActualRemedy by setting fc.Attempt to 2 of 3 inside that test and asserting \"2 of 3\" in place of \"4 of 4\" (leave renderFC unchanged). Edit only these two files. Do not touch .kiro/agents/*.json. Commit only these two files by explicit path."
    dependencies: []
    acceptance_criteria:
      - "TestPunchOut_NewRunIDPostsFreshComment, TestRenderComment_FirstLineIsOnlyMarker, TestPunchOut_StateErrorWritesNothing and TestRenderComment_AttemptsReflectContext no longer exist as functions"
      - "render_test.go no longer imports regexp and compiles"
      - "TestPunchOut_InvalidInputWritesNothing has a 'state error' subtest that asserts errors.Is(err, errBoom) and len(f.writes()) == 0, while the seven invalid-input rows still assert an error and zero calls"
      - "TestRenderComment_ContainsStageAttemptsExpectedActualRemedy uses a 2 of 3 attempt and asserts \"2 of 3\" together with all its previous assertions (first line is the marker, ParseMarker round trip, stage, guard, expected, actual, remedy, label); renderFC is unchanged"
      - "go test -cover ./internal/failure still reports at least 98.1%"
      - "Only internal/failure/punchout_test.go and internal/failure/render_test.go are modified and committed; no production file and no .kiro/agents/*.json file is staged"
    validation_commands:
      - "! grep -nE 'func (TestPunchOut_NewRunIDPostsFreshComment|TestRenderComment_FirstLineIsOnlyMarker|TestPunchOut_StateErrorWritesNothing|TestRenderComment_AttemptsReflectContext)\\(' internal/failure/*_test.go"
      - "! grep -n 'regexp' internal/failure/render_test.go"
      - "go test -count=1 -run '^(TestPunchOut_InvalidInputWritesNothing|TestRenderComment_ContainsStageAttemptsExpectedActualRemedy)$' -v ./internal/failure"
      - "go test -count=1 ./internal/failure"
      - "go vet ./internal/failure"
      - "test -z \"$(gofmt -l internal/failure)\""
      - "go test -count=1 -cover ./internal/failure"

  - id: "prune-route-runner"
    agent: "builder"
    description: "In internal/failure/route_test.go and runner_test.go: delete TestRoute_ResultDoesNotAliasRecordedContext, TestRunner_NoAutoRetryAfterTerminal and TestRunner_StageAttemptArgumentCountsFromOne; merge TestRoute_RecoverableRecordsContext and TestRoute_ResultIsSuccessWhenNoFailureRecorded into TestRoute_TerminalRecordsDispositionAndRemedy as a three-row t.Run table (recoverable, terminal, nothing recorded; no Disposition assertion on the last row); merge TestRunner_RefusesWhileTerminalPending into TestRunner_RefusesWhileLabeled as a third subtest 'terminal pending marker' seeded with f.seedComment(Marker(\"earlier\")+\"\\n\", true) under the shared assertions; merge TestRunner_OverwritesStageAttempt into TestRunner_ExhaustionPunchesOut by asserting the comment body contains \"4 of 4\" and not \"99\". Leave TestRunner_FreshRunStartsAtAttemptOne untouched. Edit only these two files. Do not touch .kiro/agents/*.json. Commit only these two files by explicit path."
    dependencies: []
    acceptance_criteria:
      - "TestRoute_ResultDoesNotAliasRecordedContext, TestRunner_NoAutoRetryAfterTerminal, TestRunner_StageAttemptArgumentCountsFromOne, TestRoute_RecoverableRecordsContext, TestRoute_ResultIsSuccessWhenNoFailureRecorded, TestRunner_RefusesWhileTerminalPending and TestRunner_OverwritesStageAttempt no longer exist as functions"
      - "TestRoute_TerminalRecordsDispositionAndRemedy has subtests recoverable, terminal and nothing recorded; the first two assert Failure equals the context, the Disposition and the Remedy, the last asserts Failure is nil"
      - "TestRunner_RefusesWhileLabeled has three subtests (the fail label, kairon-done and a terminal-pending marker comment) that each assert halted, 0 attempts, stage not called and no writes"
      - "TestRunner_ExhaustionPunchesOut asserts the posted comment contains \"4 of 4\" and does not contain \"99\", and keeps its existing assertions"
      - "TestRunner_FreshRunStartsAtAttemptOne, TestRoute_LastHandlerCallWins and TestRoute_GuardRunRoutesIntoMechanism are unchanged"
      - "go test ./internal/failure passes and gofmt and go vet report nothing for the package"
      - "Only internal/failure/route_test.go and internal/failure/runner_test.go are modified and committed; no production file and no .kiro/agents/*.json file is staged"
    validation_commands:
      - "! grep -nE 'func (TestRoute_ResultDoesNotAliasRecordedContext|TestRunner_NoAutoRetryAfterTerminal|TestRunner_StageAttemptArgumentCountsFromOne|TestRoute_RecoverableRecordsContext|TestRoute_ResultIsSuccessWhenNoFailureRecorded|TestRunner_RefusesWhileTerminalPending|TestRunner_OverwritesStageAttempt)\\(' internal/failure/*_test.go"
      - "go test -count=1 -run '^(TestRoute_TerminalRecordsDispositionAndRemedy|TestRunner_RefusesWhileLabeled|TestRunner_ExhaustionPunchesOut)$' -v ./internal/failure"
      - "go test -count=1 ./internal/failure"
      - "go vet ./internal/failure"
      - "test -z \"$(gofmt -l internal/failure)\""
      - "go test -count=1 -cover ./internal/failure"

  - id: "prune-state"
    agent: "builder"
    description: "In internal/failure/state_test.go: delete TestSelectionExcluded; merge TestClassify_NoMarkerIsEligible, TestClassify_LabelRemovedBeforeMarkerStaysPending, TestClassify_NewMarkerAfterRestartIsPending, TestClassify_UsesNewestMarkerRegardlessOfOrder and TestClassify_IgnoresMarkerNotByViewer into TestClassify_DoneAndFailed by converting it to a t.Run table of {name, state IssueState, want Selection, excluded bool} with the ten rows in the spec (done, failed, failed wins over pending, done without comments, no marker, empty state, label removed before marker stays pending, new marker after restart is pending, newest marker regardless of order, marker not by viewer is ignored). Assert both Classify and Excluded() against the explicit excluded column. Leave the other Classify tests unchanged. Edit only this file. Do not touch .kiro/agents/*.json. Commit only this file by explicit path."
    dependencies: []
    acceptance_criteria:
      - "TestSelectionExcluded, TestClassify_NoMarkerIsEligible, TestClassify_LabelRemovedBeforeMarkerStaysPending, TestClassify_NewMarkerAfterRestartIsPending, TestClassify_UsesNewestMarkerRegardlessOfOrder and TestClassify_IgnoresMarkerNotByViewer no longer exist as functions"
      - "TestClassify_DoneAndFailed runs ten subtests whose states, times and expected Selection values match the spec table, and each asserts Excluded() against an explicit excluded field"
      - "All four Selection values (SelectEligible, SelectDone, SelectFailed, SelectTerminalPending) appear among the rows with the correct excluded flag"
      - "TestClassify_TerminalPendingIsExcluded, TestClassify_LabelClearedAfterMarkerIsEligible, TestClassify_IgnoresViewerCommentWithMarkerNotOnFirstLine and TestClassify_UntrustedMarkerDoesNotHideTrustedOne are unchanged"
      - "go test ./internal/failure passes and gofmt and go vet report nothing for the package"
      - "Only internal/failure/state_test.go is modified and committed; no production file and no .kiro/agents/*.json file is staged"
    validation_commands:
      - "! grep -nE 'func (TestSelectionExcluded|TestClassify_NoMarkerIsEligible|TestClassify_LabelRemovedBeforeMarkerStaysPending|TestClassify_NewMarkerAfterRestartIsPending|TestClassify_UsesNewestMarkerRegardlessOfOrder|TestClassify_IgnoresMarkerNotByViewer)\\(' internal/failure/*_test.go"
      - "go test -count=1 -run '^TestClassify_DoneAndFailed$' -v ./internal/failure"
      - "go test -count=1 ./internal/failure"
      - "go vet ./internal/failure"
      - "test -z \"$(gofmt -l internal/failure)\""
      - "go test -count=1 -cover ./internal/failure"

  - id: "validate-rows"
    agent: "validator"
    description: "Read-only check of AC1 and AC2 and of assertion preservation. Confirm all 9 Delete tests are gone and all 15 Merge tests are no longer functions. Confirm each of the 6 named partners passes and shows the merged cases as subtests or list entries (TestClassify_DoneAndFailed, TestDecide_InvalidAttemptFailsToPunchOut, TestPunchOut_InvalidInputWritesNothing, TestRoute_TerminalRecordsDispositionAndRemedy, TestRunner_RefusesWhileLabeled as subtests; TestParseMarker_RejectsMalformed, TestRenderComment_ContainsStageAttemptsExpectedActualRemedy, TestRunner_ExhaustionPunchesOut with the merged input or assertion). Walk all 24 rows of the Current / Target State table against git diff origin/main...HEAD for internal/failure/*_test.go and confirm every merged test's distinct input and assertion appears in its partner, that the state error row asserts errors.Is(err, errBoom) and zero writes, and that no test outside the table was changed or removed."
    dependencies: ["prune-marker-policy", "prune-punchout-render", "prune-route-runner", "prune-state"]
    acceptance_criteria:
      - "The AC1 grep prints nothing"
      - "The AC2 grep prints nothing"
      - "Each partner passes under go test -run '^<partner>$' -v and its merged case is visible as a subtest or as an entry in the partner's input list"
      - "Each of the 24 table rows is accounted for with a PASS or FAIL line citing the file and the partner; any FAIL names the missing assertion"
      - "No Test function outside the 24 listed was removed or renamed (compare the function lists of origin/main and HEAD)"
    validation_commands:
      - "! grep -nE 'func (TestDispositionZeroValueIsRecoverable|TestPolicyAttempt|TestPolicyFromRetries|TestPunchOut_NewRunIDPostsFreshComment|TestRenderComment_FirstLineIsOnlyMarker|TestRoute_ResultDoesNotAliasRecordedContext|TestRunner_NoAutoRetryAfterTerminal|TestRunner_StageAttemptArgumentCountsFromOne|TestSelectionExcluded)\\(' internal/failure/*_test.go"
      - "! grep -nE 'func (TestClassify_IgnoresMarkerNotByViewer|TestClassify_LabelRemovedBeforeMarkerStaysPending|TestClassify_NewMarkerAfterRestartIsPending|TestClassify_NoMarkerIsEligible|TestClassify_UsesNewestMarkerRegardlessOfOrder|TestDecide_RecoverablePunchesOutAtMax|TestDecide_RecoverableRetriesWhileAttemptsRemain|TestDecide_UnrecoverableAlwaysPunchesOut|TestParseMarker_RejectsNonFirstLine|TestPunchOut_StateErrorWritesNothing|TestRenderComment_AttemptsReflectContext|TestRoute_RecoverableRecordsContext|TestRoute_ResultIsSuccessWhenNoFailureRecorded|TestRunner_OverwritesStageAttempt|TestRunner_RefusesWhileTerminalPending)\\(' internal/failure/*_test.go"
      - "go test -count=1 -run '^TestClassify_DoneAndFailed$' -v ./internal/failure | grep -q -- '--- PASS: TestClassify_DoneAndFailed/'"
      - "go test -count=1 -run '^TestDecide_InvalidAttemptFailsToPunchOut$' -v ./internal/failure | grep -q -- '--- PASS: TestDecide_InvalidAttemptFailsToPunchOut/'"
      - "go test -count=1 -run '^TestPunchOut_InvalidInputWritesNothing$' -v ./internal/failure | grep -q -- '--- PASS: TestPunchOut_InvalidInputWritesNothing/state_error'"
      - "go test -count=1 -run '^TestRoute_TerminalRecordsDispositionAndRemedy$' -v ./internal/failure | grep -q -- '--- PASS: TestRoute_TerminalRecordsDispositionAndRemedy/'"
      - "go test -count=1 -run '^TestRunner_RefusesWhileLabeled$' -v ./internal/failure | grep -q -- '--- PASS: TestRunner_RefusesWhileLabeled/'"
      - "go test -count=1 -run '^(TestParseMarker_RejectsMalformed|TestRenderComment_ContainsStageAttemptsExpectedActualRemedy|TestRunner_ExhaustionPunchesOut)$' -v ./internal/failure"

  - id: "validate-gate"
    agent: "validator"
    description: |-
      Read-only check of AC3, AC4 and AC5. (1) AC3: go test -count=1 -cover ./internal/failure must report at least 98.1%, and go test -race -count=1 ./internal/failure must pass in the worktree. (2) AC4: the pass/fail evidence is the exit code of task test in a git-preserving clean export. Make the export with git clone -q --no-hardlinks "$PWD" "$D" (not git archive | tar: that export has no .git, and about 55 internal/eval tests then fail on git rev-parse), delete "$D"/internal/failure/*_test.go, copy the worktree's internal/failure/*_test.go into "$D"/internal/failure/, run task test in "$D" with the output saved to a log, and remove "$D". A non-zero exit code is a FAIL for AC4 and there is no carve-out; a zero exit code is a PASS. task test runs go test -v -race ./...: allow at least 15 minutes (about 3 minutes for a parallel run of internal/eval, about 8 minutes on a loaded machine; 10 minutes 9 seconds measured on this machine on the unchanged HEAD) and do not stop early or substitute a weaker check. Judge the result by the exit code only: the log has lines like FAIL selftest-fail from passing eval self-tests. The worktree run is informational only: if you run go test -count=1 ./... in the worktree, report every failure you actually see (package and test names; do not assume a list), confirm from the export log that each one passes there (--- PASS: <name>), and say that the cause is the environment's modified .kiro/agents/*.json files. A worktree failure never fails AC4. (3) AC5: check the set of paths that will be committed. Allowed paths are exactly internal/failure/*_test.go and the issue's own spec file .kairon/specs/issue-432-*.md (the only allowed extra path, AGENTS.md Exceptions). Check (a) git diff --name-only origin/main...HEAD lists only allowed paths and nothing under .kiro, and no internal/failure/*.go that is not a _test.go differs; (b) git status --porcelain --untracked-files=all (staged, unstaged and untracked paths) lists only allowed paths. The only paths tolerated outside the allowed set are .kiro/agents/*.json files that are unstaged working-tree modifications (status " M") and absent from every commit: they are environment-specific sandbox credential injection, krew-lead stages the final commit by explicit path only and never stages them, so report them as excluded paths, listing each one you see. If any such file is staged, committed or untracked, or any other path outside the allowed set appears, report AC5 as FAIL with the exact paths (or write Status BLOCKED with the exact paths if a human must clean the tree). Never count a staged or committed .kiro file as a pass.
    dependencies: ["prune-marker-policy", "prune-punchout-render", "prune-route-runner", "prune-state"]
    acceptance_criteria:
      - "go test -count=1 -cover ./internal/failure reports at least 98.1% of statements"
      - "go test -race -count=1 ./internal/failure passes in the worktree"
      - "task test exits 0 in the git-preserving clean export (git clone --no-hardlinks of the worktree with the worktree's internal/failure/*_test.go copied over it); this exit code is the only evidence for AC4, and a non-zero exit code is a FAIL for AC4"
      - "The export keeps .git (it is a git clone, not a git archive extract), and the report gives the run time of task test in the export"
      - "Any failures seen in the worktree run are reported as informational with their actual package and test names, and each is confirmed to pass in the export; none of them changes the AC4 result"
      - "git diff --name-only origin/main...HEAD lists only paths matching internal/failure/*_test.go, plus the issue's own spec file .kairon/specs/issue-432-*.md as the only allowed extra path; nothing under .kiro appears in any commit of the branch, and no production file (internal/failure/*.go that is not a _test.go) differs from origin/main"
      - "git status --porcelain --untracked-files=all (staged, unstaged and untracked paths) lists only internal/failure/*_test.go paths and .kairon/specs/issue-432-*.md, apart from .kiro/agents/*.json files that are unstaged working-tree modifications; the report lists each such file as an excluded path that krew-lead will not stage"
      - "AC5 is reported FAIL (or Status BLOCKED with the exact paths) if any .kiro/agents/*.json file is staged, committed or untracked, or if any other path outside the allowed set appears"
    validation_commands:
      - "go test -count=1 -cover ./internal/failure"
      - "go test -race -count=1 ./internal/failure"
      - |-
        D=$(mktemp -d); L=$(mktemp)
        git clone -q --no-hardlinks "$PWD" "$D" && rm -f "$D"/internal/failure/*_test.go && cp internal/failure/*_test.go "$D/internal/failure/" && (cd "$D" && task test) >"$L" 2>&1
        rc=$?; tail -n 30 "$L"; echo "export task test exit code: $rc (log: $L)"
        rm -rf "$D"
        exit $rc
      - "test -z \"$(git diff --name-only origin/main...HEAD | grep -vE '^(internal/failure/[A-Za-z0-9_]+_test\\.go|\\.kairon/specs/issue-432-[^/]+\\.md)$')\""
      - "test -z \"$(git diff --name-only origin/main...HEAD -- internal/failure ':!internal/failure/*_test.go')\""
      - "test -z \"$(git diff --name-only origin/main...HEAD -- .kiro)\""
      - |-
        git status --porcelain --untracked-files=all | awk '
          { xy = substr($0, 1, 2); p = substr($0, 4) }
          p ~ /^internal\/failure\/[A-Za-z0-9_]+_test\.go$/ { next }
          p ~ /^\.kairon\/specs\/issue-432-[^\/]+\.md$/ { next }
          xy == " M" && p ~ /^\.kiro\/agents\/[^\/]+\.json$/ { print "EXCLUDED (krew-lead will not stage): " p > "/dev/stderr"; next }
          { print "NOT ALLOWED: " xy " " p; bad = 1 }
          END { exit bad }'
```
