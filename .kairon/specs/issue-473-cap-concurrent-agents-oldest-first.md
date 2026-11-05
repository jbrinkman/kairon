# Design Spec: Cap concurrent agents and start the oldest issues first

Closes #473

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/config`: a `max_agents` setting in `.kairon/config.yaml`, with a finite default. Kairon rejects zero or negative values at load time and offers no way to set it to unlimited." | `config-max-agents` |
| S2 | In Scope: "`internal/watcher` and `internal/agent` (`Manager`): never run more than `max_agents` agents at once. Implementation runs, retries started by `retryAgent`, and intake runs all count toward the limit." | `manager-slot-core`, `manager-intake-slots`, `watcher-cap-oldest-first` |
| S3 | In Scope: "`internal/watcher`: when a slot is free, start the oldest eligible issue (lowest issue number) first. An issue that waits for a slot is not charged a retry attempt, and the next poll considers it again." | `watcher-cap-oldest-first` |
| S4 | In Scope: "`internal/watcher`: log each poll's waiting issues once at info level (\"issue #N waiting for an agent slot (M/M running)\"), not as a failure." | `watcher-cap-oldest-first` |
| S5 | In Scope: "Documentation: document `max_agents` in `README.md` / `docs/` and in the config template under `cmd/kairon/templates/`." | `config-max-agents`, `document-max-agents` |
| AC1 | "Kairon never runs more than `max_agents` agents at once." | `manager-slot-core` (`TestSpawn_NeverExceedsMaxAgentsConcurrently`), `watcher-cap-oldest-first` (`TestCheckIssues_TwoSlotsFiveIssuesNeverMoreThanTwoRunning`) |
| AC2 | "Free slots go to the oldest eligible issue." | `watcher-cap-oldest-first` (`TestCheckIssues_OneSlotStartsOldestIssueFirst`) |
| AC3 | "Retries and intake runs count toward the limit." | `manager-slot-core` (`TestRetryHoldsSlotAcrossFailedGap`), `manager-intake-slots` (`TestIntakeRunHoldsSlot`), `watcher-cap-oldest-first` (`TestCheckIssues_RetryHoldsOnlySlot`, `TestCheckIssues_IntakeHoldsOnlySlot`) |
| AC4 | "An issue that waits for a slot keeps its retry budget and is considered again on the next poll." | `watcher-cap-oldest-first` (`TestCheckIssues_WaitingIssueKeepsRetryBudget`) |
| AC5 | "Unlimited is not configurable." | `config-max-agents` (`TestLoad_MaxAgentsDefaultIsFinitePositive`, `TestLoad_MaxAgentsRejectsNonPositive`) |
| AC6 | "Each poll logs its waiting issues once, at info level." | `watcher-cap-oldest-first` (`TestCheckIssues_LogsWaitingIssueOnceAtInfo`) |
| AC7 | "The setting is documented, and the config template is in sync." | `config-max-agents`, `document-max-agents`, `validate-all` |
| AC8 | "The gate passes." | `validate-all` |

## Deferred work

none

## Solution Approach

### What is wrong today

- `checkIssues` (`internal/watcher/watcher.go`) walks `sel.Eligible` and calls `w.spawn` (which is `Manager.Spawn`) for every issue that passes its filters. Nothing counts running agents.
- `Manager.Spawn` (`internal/agent/manager.go`) never refuses for capacity.
- Retries are the hard case. `HandleExit` sets the agent's `Status` to `StatusFailed`, then `decideFailureLocked` runs `go m.retry(a)`. `retryAgent` sleeps `RetryCount` seconds and only then sets `Status = StatusRunning`. During that gap the issue has no `StatusRunning` agent, so a capacity count derived from `Status` would show a free slot and a second issue would start. When the retry then starts, the cap is exceeded.
- Intake runs are `Agent` entries of `Kind == KindIntake` created by `startIntake` (`internal/agent/intake.go`). They are started from two places: the boundary, inside `Spawn` (`admitIssue` -> `watcherIntake.Start` -> `startIntake`), and the TUI `validate` command (`Manager.Validate` -> `startIntake`).
- The watcher has a second, separate gap: `checkIssues` relies on `selectIssues` for order. The watcher test seam (`w.selectIssues`) can return any order, so ordering is not guaranteed at the point where it matters.

### Chosen design: an issue-keyed slot table in the Manager, reserved under `m.mu`

The Manager owns slot accounting. The watcher asks it; it does not keep its own count.

1. **One slot per issue, held for the whole life of the run.** `Manager` gets `maxAgents int` and `slots map[int]struct{}` (issue number -> held). Both are guarded by the existing `m.mu`. A slot is not derived from `Agent.Status`, so the `StatusFailed` gap between a crash and its retry does not free it.
2. **Reserve before anything is created.** `reserveSlotLocked(issue)` runs under `m.mu`, in the same critical section that checks `intakeOwnsIssueLocked` and before `createWorktree`, `agentLogFile` and `m.start`. The count check and the insert are one step under one lock, so two concurrent `Spawn` calls cannot both take the last slot. This is the race-free reservation.
3. **Release exactly where a run ends for good.**

   | Run | Acquired | Released |
   |-----|----------|----------|
   | Implementation | `Spawn`, after the intake-owns check, under `m.mu` | `Spawn` failure after reserve (worktree, log file or `m.start` error); `HandleExit` when the agent is marked `StatusCompleted`; `decideFailureLocked` on the punch-out branch; `retryAgent`'s `failRetry` (every terminal exit of a retry: refused admission, punch-out, worktree/log/start failure, intake took the issue) |
   | Retry | Not acquired. It keeps the slot of the run it retries, across the `StatusFailed` gap and the sleep in `retryAgent`. | Same as above |
   | Intake | `startIntake`, in its first critical section | `endIntake`, right after it moves the agent out of `StatusRunning` (that function already returns early when the status is not running, so release happens once) |

   Releasing a slot not held is a no-op, so a duplicated exit callback cannot double-release.
4. **No second run per issue.** `reserveSlotLocked` reports `errSlotHeld` when the issue already holds a slot. `Spawn` maps that to `ErrNotAdmitted` ("a run for the issue is already going"). `startIntake` maps it to `started == false`, the existing "a run is already going" answer. This also closes the gap where `Validate` could start intake on an issue whose retry is sleeping.
5. **Count runs started by an earlier process.** An intake run that outlived a previous watcher is visible only through its PID marker (`livePIDMarker`, `.kairon/intake/issue-<n>.pid`). `usedLocked()` is `len(slots)` plus the number of live PID markers whose issue is not already in `slots`. A cap that ignored those would let Kairon run more than `max_agents` agents after a restart. This reuses `livePIDMarker`, which `activeRunLocked` already calls under `m.mu`.
6. **Capacity errors.** New `var ErrAtCapacity = errors.New("agent: no free agent slot")`. `reserveSlotLocked` returns `fmt.Errorf("%w (%d/%d running)", ErrAtCapacity, used, max)`.
   - `Spawn` has a cheap early check (read lock, no reservation) before `admitIssue`, so a poll that races a full table does no GitHub read, writes no input document and cannot start intake. The authoritative reservation is the locked one after admission.
   - Intake started from inside `Spawn`'s admission fails with a wrapped `ErrAtCapacity`. `boundary.Boundary.startIntake` already wraps the `Intake.Start` error with `%w`. `admitIssue` currently wraps every admission error as `ErrAdmissionUnavailable` with `%v`, which would hide it, so `admitIssue` returns the error unchanged when `errors.Is(err, ErrAtCapacity)`.
   - `Validate` at capacity returns `ValidateError` whose `Err` wraps `ErrAtCapacity`; the TUI already prints `Validate #N failed: <err>`.
7. **Non-positive limits never mean unlimited.** `config.Load` rejects them. A hand-built `config.Config` (the many test fixtures write `&config.Config{...}` with no `MaxAgents`) goes through a new `(*Config).EffectiveMaxAgents()`, which returns `MaxAgents` when positive and `DefaultMaxAgents` otherwise. `NewManagerWithOptions` and the watcher use it. No path produces an unbounded limit, and existing fixtures keep working.
8. **Test seams.** `agent.Options` gets `PRExists func(repo string, issue int) (bool, error)` (nil keeps `github.PRExistsForIssue`). `HandleExit` currently adds the done label with the package function `github.AddLabel`, which shells out to `gh`; it changes to `m.issues.AddLabel` (the `failure.Issues` port every other label write already uses) so a watcher-level test can drive a real exit-0 completion without `gh`. Production behavior is the same: `ghPorts` embeds `github.CLI`, whose `AddLabel` runs the same `gh issue edit --add-label`.

### Watcher: sort, then fill free slots

In `checkIssues`:

- **Sort first.** Copy `sel.Eligible` and sort ascending by `Number` (`slices.SortStableFunc`). #470 makes `selectIssues` return oldest-first, but the acceptance test for criterion 2 feeds #12, #10, #11 through the `selectIssues` seam, and the loop must not depend on the caller's order. `untrackAbsent` still gets the original `sel.Eligible`.
- **Capacity gate position.** The gate goes after every existing filter, including dependency validation, and before `w.tracked[...] = true` and `incrementGlobalRetryCount`. So only an issue that would start right now can be reported as waiting. An older issue that is blocked by an open dependency, in backoff, over its retry limit or already running does not take a slot and is not logged as waiting, and a younger eligible issue starts in its place. That is what "oldest eligible issue" means.
- **Gate.** `used, max := w.slots()`. If `used >= max` the issue is waiting: log it, `continue`. The retry counter is never touched on this path, so the budget is unchanged (criterion 4), and `tracked` is not set, so the next poll considers it again.
- **Race with another starter.** `Spawn` can still return `ErrAtCapacity` if `Validate` took the last slot between the gate and the reservation. The existing `ErrNotAdmitted` branch already does "untrack and refund"; `ErrAtCapacity` joins it and is logged as waiting, not as `failed to spawn`.
- **Free slots are re-read for each issue.** One `Spawn` can change the count (an intake start takes a slot too), so the gate reads fresh numbers before every issue. When the first issue of a poll takes the last slot, the rest wait in the same poll.
- **Log.** Each waiting issue gets exactly one line per poll: `log.Printf("[watcher] INFO: issue #%d waiting for an agent slot (%d/%d running)", n, used, max)`. The watcher has no leveled logger: it writes with the standard `log` package to `.kairon/kairon.log` and tags warnings in the message text (`WARNING:`). `logging.Info` (the leveled `internal/logging` logger) is a no-op unless the log viewer is open, so it would drop the message in normal runs. The line carries the `INFO:` tag, has no `WARNING:`, `ERROR:` or `failed` wording, and `used == max` whenever it is written, so it reads `(M/M running)`.
- **Seam.** `Watcher` gets `slots func() (used, max int)`. `New` sets it to `mgr.Slots` when `mgr != nil`, and otherwise to `func() (int, int) { return 0, cfg.EffectiveMaxAgents() }` (the nil-manager watchers in the existing tests stub `spawn` and never register agents).
- **Poll cost.** At capacity, every remaining eligible issue still goes through dependency validation so that only genuinely startable issues are reported as waiting. That is the same per-issue work every poll did before this change.

### Alternatives considered

| Option | Verdict |
|--------|---------|
| Count `StatusRunning` agents in `m.agents` | Rejected. Misses the `StatusFailed` window before a retry starts (retries must count), and leaves a gap between the count and the insert in `Spawn`. |
| Counting semaphore (`chan struct{}`) in the Manager | Rejected. Needs a token per run handed through `Spawn`, `retryAgent`, `startIntake` and the exit callbacks, cannot report `used` for the log line, and cannot count orphan intake runs. An issue-keyed map does all three and makes double release harmless. |
| Count in the watcher | Rejected. The watcher does not see retries (`retryAgent` runs inside the Manager) or the TUI `validate` path. |
| Unlimited when `max_agents` is `0` | Rejected by the issue. |
| Reserve before admission in `Spawn` | Rejected. Admission may start intake, which needs the same issue's slot; a reservation made earlier would have to be handed over. The early read-only check plus the authoritative locked reservation after admission gives the same protection with no hand-over. |

### Default value

`DefaultMaxAgents = 3`: enough for independent issues to overlap, small enough to bound token spend. Documented in the README and docs, and in the config template.

## Relevant Files

| File | Change |
|------|--------|
| `internal/config/config.go` | `MaxAgents` field (`yaml:"max_agents"`), `DefaultMaxAgents`, default in `Load`, validation error naming `max_agents`, `EffectiveMaxAgents()` |
| `internal/config/config_test.go` | `max_agents` tests |
| `.kairon/config.yaml`, `cmd/kairon/templates/kairon/config.yaml` | `max_agents: 3` with an explanatory comment (the template is what `kairon init` writes) |
| `internal/agent/manager.go` | `maxAgents`, `slots`, `ErrAtCapacity`, `errSlotHeld`, `reserveSlotLocked`, `releaseSlotLocked`, `usedLocked`, `Slots()`, `Options.PRExists`; reserve in `Spawn`, release in `Spawn` failure paths, `HandleExit`, `decideFailureLocked`, `retryAgent.failRetry`; `admitIssue` passes `ErrAtCapacity` through; done label via `m.issues.AddLabel` |
| `internal/agent/intake.go` | reserve in `startIntake`, release in `endIntake`, orphan PID markers in the count, `Validate` at capacity |
| `internal/agent/manager_capacity_test.go` (new) | slot tests for `Spawn`, exits, retries |
| `internal/agent/intake_capacity_test.go` (new) | slot tests for intake, `Validate`, orphans |
| `internal/watcher/watcher.go` | sort, `slots` seam, capacity gate, waiting log, `ErrAtCapacity` branch |
| `internal/watcher/watcher_capacity_test.go` (new) | acceptance tests for criteria 1-4 and 6 |
| `README.md` | config table row and sample, scheduling paragraph |
| `docs/concurrency.md` (new), `docs/boundary.md` | reference for `max_agents`, slot rules, the waiting log line, `validate` at capacity |

Not template-synchronized: `config.yaml` is not in the `sync:check` mapping (`Taskfile.yml`), and no agent, script, theme, eval or skill file changes. `task sync:check` still runs as the gate.

## Team Orchestration

```
config-max-agents --> manager-slot-core --> manager-intake-slots --> watcher-cap-oldest-first --+--> validate-all
                                                                                               |
config-max-agents --> document-max-agents -----------------------------------------------------+
```

- `config-max-agents` goes first: both the Manager and the watcher read `EffectiveMaxAgents()`.
- `manager-slot-core` and `manager-intake-slots` edit the same two files and share the slot table, so they run in sequence.
- `document-max-agents` only needs the final names and behavior, which this spec fixes, so it can run in parallel with the Go tasks. It waits for `config-max-agents` because it cites the default.
- `watcher-cap-oldest-first` needs the Manager API (`Slots`, `ErrAtCapacity`, `Options.PRExists`) from both manager tasks.
- `validate-all` runs last, read-only.

## Step-by-Step Task Breakdown

Every Go task follows AGENTS.md TDD: write the tests first against a signature-only stub, run them, confirm they fail for the expected reason (a missing or wrong behavior, not a compile error), then implement. Tests and implementation go in one commit, and the commit message names the test seen failing first. All `go test` commands include `-race`.

### Task 1: `config-max-agents`

- Add `MaxAgents int \`yaml:"max_agents"\`` to `Config` and `const DefaultMaxAgents = 3`. `Load` sets the default before unmarshalling, so an absent key keeps it.
- After unmarshalling, `if cfg.MaxAgents <= 0` returns `fmt.Errorf("max_agents must be greater than 0 (got %d); there is no unlimited setting", cfg.MaxAgents)`. Place it with the other validation, before the theme load.
- `func (c *Config) EffectiveMaxAgents() int`: `MaxAgents` when positive, else `DefaultMaxAgents`.
- Add a `max_agents: 3` line with a comment to `.kairon/config.yaml` and `cmd/kairon/templates/kairon/config.yaml`: what it caps (implementation runs, retries, intake), that the oldest issues start first, that the value must be a positive integer, and that there is no unlimited setting.
- Tests in `internal/config/config_test.go`, using the existing `writeProjectConfig`/`runScalarCases` helpers: `TestLoad_MaxAgentsDefaultIsFinitePositive` (key absent gives `DefaultMaxAgents`, and `DefaultMaxAgents > 0`; also asserts the shipped template config parses to the default when `repo` is filled in), `TestLoad_MaxAgentsAcceptsPositive`, `TestLoad_MaxAgentsRejectsNonPositive` (`0`, `-1` fail and the message contains `max_agents`), `TestEffectiveMaxAgents` (0 and negative give the default, never an unbounded value).

### Task 2: `manager-slot-core`

- Slot table, `ErrAtCapacity`, `errSlotHeld`, `reserveSlotLocked`, `releaseSlotLocked`, `usedLocked` (in-process slots only in this task; orphan intake markers are added in task 3), `Slots() (used, max int)`.
- `NewManagerWithOptions`: `m.maxAgents = cfg.EffectiveMaxAgents()`, `Options.PRExists`.
- `Spawn`: early read-only capacity check before `admitIssue`; reserve under `m.mu` after the `intakeOwnsIssueLocked` check; release on every error return after the reserve (a deferred release guarded by a success flag runs while `m.mu` is still held); keep the slot on success.
- `HandleExit`: release when the agent is marked `StatusCompleted`; `m.issues.AddLabel` for the done label.
- `decideFailureLocked`: release on the punch-out branch; the retry branch releases nothing.
- `retryAgent`: release inside `failRetry`; no other change to the retry flow.
- `admitIssue`: return `ErrAtCapacity` unchanged.
- Tests in `internal/agent/manager_capacity_test.go` (use `NewManagerWithOptions` with `githubtest.Fake`, `CreateWorktree`, `StartAgent`, `MonitorAgent` stubs as `manager_boundary_test.go` does):
  - `TestSpawn_NeverExceedsMaxAgentsConcurrently`: `MaxAgents: 2`, eight distinct valid issues spawned from eight goroutines at once; exactly two succeed, six return `ErrAtCapacity`, `Slots()` reports `2/2`, and the stub `StartAgent` was called twice. Run under `-race`.
  - `TestSpawn_ExitFreesSlotForNextIssue`: third issue refused, then `HandleExit(id, 0)` with `PRExists` true, then the third issue spawns.
  - `TestSpawn_AtCapacityDoesNoAdmissionWork`: with the table full, `Spawn` makes no GitHub read, writes no input document and calls no `CreateWorktree`.
  - `TestSpawn_FailurePathsReleaseSlot`: worktree error, log-file error and `StartAgent` error each leave `Slots()` at 0.
  - `TestRetryHoldsSlotAcrossFailedGap`: `MaxAgents: 1`; `HandleExit(id, 1)` with the `m.retry` seam replaced by a function that blocks on a channel (the agent's `Status` is `StatusFailed`); a `Spawn` for another issue returns `ErrAtCapacity`; releasing the gate runs the real `retryAgent`; the other issue is still refused while the retry runs, and is accepted after the retried run ends.
  - `TestRetryTerminalFailureReleasesSlot`: a retry whose re-admission is refused or whose worktree creation fails frees the slot (covers `failRetry`).
  - `TestPunchOutReleasesSlot`: an exit with the retry budget spent punches out and frees the slot.
  - `TestSpawn_SecondRunForHeldIssueRefused`: a second `Spawn` for an issue that holds a slot returns `ErrNotAdmitted`.
  - `TestNewManager_NonPositiveMaxAgentsUsesDefault`: a `config.Config{}` manager reports `max == DefaultMaxAgents`.

### Task 3: `manager-intake-slots`

- `startIntake`: reserve in the first critical section after `activeRunLocked`; `errSlotHeld` gives `(nil, false, nil)`; `ErrAtCapacity` is returned as the error with nothing written (no agent entry, no files).
- `endIntake`: `releaseSlotLocked(run.issue)` in the same critical section that moves the status off `StatusRunning`, so every intake exit (committed, punched out, changed, stopped, failed) releases once, and the slot stays held across `intakeAttemptFailed` retries.
- `usedLocked`: add live orphan PID markers. Enumerate `.kairon/intake/issue-*.pid`, parse the issue number, skip issues already in `slots`, count those where `livePIDMarker` is true.
- `Validate`: when `startIntake` returns `ErrAtCapacity`, `Validate` returns `ValidateError` with `Err` wrapping `ErrAtCapacity` (the existing `err != nil` branch already does this once `startIntake` returns the error unchanged). The existing call order (`clearIntakeRecords`, then `startIntake`) stays: an explicit `validate` request already resets the loop guard, and a refused start writes nothing else.
- Tests in `internal/agent/intake_capacity_test.go` (use the harness in `intake_test.go`):
  - `TestIntakeRunHoldsSlot`: `MaxAgents: 1`; an intake run is in flight (its process blocked); `Spawn` for another issue returns `ErrAtCapacity`; when the run ends (each of committed, punched out, stopped, failed) the slot frees and `Spawn` succeeds.
  - `TestIntakeAttemptRetryKeepsSlot`: a failed attempt followed by a retry attempt never shows a free slot in between.
  - `TestIntakeStartAtCapacityStartsNothing`: `startIntake` at capacity returns `ErrAtCapacity`, creates no agent entry and no `.kairon/intake` or log file.
  - `TestSpawnAdmissionIntakeAtCapacityReturnsErrAtCapacity`: an unmarked issue whose admission would start intake, with the slot table full by a race, returns an error satisfying `errors.Is(err, ErrAtCapacity)`, not `ErrAdmissionUnavailable`.
  - `TestValidate_AtCapacityReportsError`.
  - `TestOrphanIntakeMarkerCountsTowardLimit`: a PID marker naming a live process (the test's own PID) for an issue not owned by the manager makes `Slots()` report it, and with `MaxAgents: 1` `Spawn` returns `ErrAtCapacity`; a marker naming a dead PID does not count.

### Task 4: `watcher-cap-oldest-first`

- `checkIssues`: sorted copy of `sel.Eligible`; capacity gate after dependency validation and before `tracked`/`incrementGlobalRetryCount`; waiting log; `ErrAtCapacity` handled in the same branch as `ErrNotAdmitted`/`ErrAdmissionUnavailable` (untrack, refund, log as waiting once, `continue`).
- `Watcher.slots` seam and its default in `New`.
- Tests in `internal/watcher/watcher_capacity_test.go`, using a production-constructed `agent.Manager` over `githubtest.Fake` with `StartAgent`/`MonitorAgent` stubs as `watcher_intake_test.go` does (`newPipelineHere`-style, with `MaxAgents` set; add a parameter or a sibling constructor, do not change the existing fixtures' behavior):
  - `TestCheckIssues_TwoSlotsFiveIssuesNeverMoreThanTwoRunning` (criterion 1): `MaxAgents: 2`, five eligible issues. After poll 1 exactly two agents are running and three issues are waiting. After each of several polls the running count is at most two. The third issue starts only after one of the first two exits (`HandleExit`), never before.
  - `TestCheckIssues_OneSlotStartsOldestIssueFirst` (criterion 2): `MaxAgents: 1`, `selectIssues` returns #12, #10, #11 in that order. Poll, exit, poll, exit, poll: start order is #10, #11, #12.
  - `TestCheckIssues_RetryHoldsOnlySlot` (criterion 3): the only slot is held by a run in the failed-then-retry gap; polls start nothing until the retry run ends.
  - `TestCheckIssues_IntakeHoldsOnlySlot` (criterion 3): the only slot is held by an in-flight intake run; polls start nothing else until it ends.
  - `TestCheckIssues_WaitingIssueKeepsRetryBudget` (criterion 4): `MaxAgents: 1`, one running agent, a waiting issue with an existing retry count file. After three consecutive polls the file content is unchanged (and absent if it was absent), the issue is not in `tracked`, and the first poll after the slot frees starts it with `attempt` = old count + 1.
  - `TestCheckIssues_LogsWaitingIssueOnceAtInfo` (criterion 6): `MaxAgents: 1`, two eligible issues; capture the standard logger (`log.SetOutput` to a buffer, restored in `t.Cleanup`) for one `checkIssues` call. Exactly one line contains `issue #<waiting> waiting for an agent slot (1/1 running)`, it contains `INFO:`, and no line mentioning that issue contains `WARNING:`, `ERROR:` or `failed to spawn`. A second identical poll logs it once more (once per poll).
  - `TestCheckIssues_BlockedOlderIssueDoesNotTakeSlot`: the lowest-numbered issue has an open dependency; the next-lowest eligible issue starts and the blocked one is not logged as waiting.
  - `TestCheckIssues_SpawnAtCapacityUntracksAndRefunds`: the `spawn` seam returns `ErrAtCapacity`; the issue ends untracked, its retry count is back to its value before the poll, and one waiting line is logged.
  - All existing watcher tests keep passing without assertion changes.

### Task 5: `document-max-agents`

- `README.md`: add `max_agents` to the config sample and the config table (default `3`, positive integer, no unlimited), and a short paragraph under Agent Workflow or Issue Lifecycle: at most `max_agents` agents run at once, implementation runs, retries and intake all count, the oldest issue starts first, a waiting issue is not charged a retry and is logged at info level as `issue #N waiting for an agent slot (M/M running)`.
- `docs/concurrency.md` (new): the setting and its default, what holds a slot and when it is released (the table in this spec), oldest-first order and why, waiting issues and the retry budget, the log line, what `validate` does at capacity, runs left by a previous watcher, and how to raise the limit.
- `docs/boundary.md`: one sentence in the `validate` section that it reports an error when no slot is free.
- Every statement matches the final code. No "Known gaps" entry is added: nothing in this issue is deferred.

### Task 6: `validate-all`

Read-only verification of every criterion (see the plan's commands): gate, template sync, grep for `max_agents`, scope table and plan parse, and a check of the AGENTS.md rules (TDD evidence in commit messages, no `Deferred work` other than `none`, no new file under `.kairon/specs/` other than this spec).

## Validation Commands

```bash
go build ./...
go vet ./...
test -z "$(gofmt -l .)"
go test -race -count=1 ./internal/config/... ./internal/agent/... ./internal/watcher/...
task test
task lint
task fmt:check
task sync:check
grep -rn max_agents README.md docs cmd/kairon/templates
go run ./cmd/kairon plan parse .kairon/specs/issue-473-cap-concurrent-agents-oldest-first.md
go run ./cmd/kairon scope check --input "$KAIRON_INPUT_DOCUMENT" --spec .kairon/specs/issue-473-cap-concurrent-agents-oldest-first.md
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "config-max-agents"
    agent: "builder"
    description: "In internal/config/config.go add the MaxAgents field (yaml max_agents), const DefaultMaxAgents = 3 set as the default in Load, a validation error 'max_agents must be greater than 0 (got N); there is no unlimited setting' for zero or negative values, and (*Config).EffectiveMaxAgents() that returns MaxAgents when positive and DefaultMaxAgents otherwise. Add a max_agents: 3 line with an explanatory comment to .kairon/config.yaml and cmd/kairon/templates/kairon/config.yaml. Write the tests in internal/config/config_test.go first (signature-only stub for the new field and function), see them fail for a behavioral reason, then implement. Commit tests and implementation together and name the test seen failing first in the commit message."
    dependencies: []
    acceptance_criteria:
      - "TestLoad_MaxAgentsDefaultIsFinitePositive passes: with no max_agents key, Load returns DefaultMaxAgents, and DefaultMaxAgents is greater than 0"
      - "TestLoad_MaxAgentsRejectsNonPositive passes: max_agents 0 and max_agents -1 each make Load return an error whose message contains max_agents"
      - "TestLoad_MaxAgentsAcceptsPositive passes: max_agents 5 loads as 5"
      - "TestEffectiveMaxAgents passes: 0 and negative values return DefaultMaxAgents, a positive value is returned unchanged, no input yields an unbounded value"
      - "grep -n max_agents .kairon/config.yaml cmd/kairon/templates/kairon/config.yaml matches in both files, and the template still loads with repo filled in"
      - "The new tests were seen failing for a behavioral reason before the implementation, and that test name is in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/config/..."
      - "test -z \"$(gofmt -l internal/config)\""
      - "go test -race -count=1 ./internal/config/..."
      - "grep -n max_agents .kairon/config.yaml cmd/kairon/templates/kairon/config.yaml"
      - "task sync:check"

  - id: "manager-slot-core"
    agent: "builder"
    description: "In internal/agent/manager.go add the issue-keyed slot table guarded by m.mu: maxAgents (from cfg.EffectiveMaxAgents()), slots map[int]struct{}, ErrAtCapacity, errSlotHeld, reserveSlotLocked (count check and insert under one lock, returns ErrAtCapacity wrapped with the used/max numbers), releaseSlotLocked (no-op when not held), usedLocked (in-process slots), and Manager.Slots() (used, max int). Add Options.PRExists (nil keeps github.PRExistsForIssue) and switch HandleExit's done-label write from github.AddLabel to m.issues.AddLabel. In Spawn: early read-only capacity check before admitIssue, reserve under m.mu right after the intakeOwnsIssueLocked check and before createWorktree, deferred release on every error return after the reserve, keep the slot on success, map errSlotHeld to ErrNotAdmitted. Release when HandleExit marks StatusCompleted, in decideFailureLocked on the punch-out branch only (the retry branch keeps the slot), and inside retryAgent's failRetry. admitIssue must return an ErrAtCapacity error unchanged instead of wrapping it as ErrAdmissionUnavailable. Write internal/agent/manager_capacity_test.go first against signature-only stubs, see the tests fail for a behavioral reason, then implement. Commit tests and implementation together."
    dependencies: ["config-max-agents"]
    acceptance_criteria:
      - "TestSpawn_NeverExceedsMaxAgentsConcurrently passes under -race: max 2, eight goroutines spawning eight valid issues, exactly two succeed, six return ErrAtCapacity, Slots() is 2/2, StartAgent ran twice"
      - "TestSpawn_ExitFreesSlotForNextIssue passes: after HandleExit(id, 0) with PRExists true the previously refused issue spawns"
      - "TestSpawn_AtCapacityDoesNoAdmissionWork passes: a full table means no GitHub read, no input document, no CreateWorktree call"
      - "TestSpawn_FailurePathsReleaseSlot passes for worktree, log file and StartAgent errors"
      - "TestRetryHoldsSlotAcrossFailedGap passes: with the agent in StatusFailed and the retry blocked, another issue gets ErrAtCapacity; it is still refused while the retry runs and accepted after the retried run ends"
      - "TestRetryTerminalFailureReleasesSlot and TestPunchOutReleasesSlot pass"
      - "TestSpawn_SecondRunForHeldIssueRefused passes: ErrNotAdmitted"
      - "TestNewManager_NonPositiveMaxAgentsUsesDefault passes: a manager built from config.Config{} reports max DefaultMaxAgents"
      - "Every pre-existing test in internal/agent passes with no change to its assertions"
      - "HandleExit's done-label write goes through m.issues.AddLabel and no test in this package invokes gh"
      - "The new tests were seen failing for a behavioral reason before the implementation, and that test name is in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/agent/..."
      - "test -z \"$(gofmt -l internal/agent)\""
      - "go test -race -count=1 ./internal/agent/..."
      - "go test -race -count=20 -run 'TestSpawn_NeverExceedsMaxAgentsConcurrently' ./internal/agent/..."

  - id: "manager-intake-slots"
    agent: "builder"
    description: "In internal/agent/intake.go make intake runs hold a slot: startIntake reserves in its first critical section after activeRunLocked (errSlotHeld returns nil,false,nil; ErrAtCapacity is returned with no agent entry and no files written), endIntake releases in the same critical section that moves the status off StatusRunning so every intake exit releases once and attempt retries keep the slot. Extend usedLocked to count live orphan intake PID markers (.kairon/intake/issue-*.pid, issues not already in slots, livePIDMarker true). Make Validate report ValidateError wrapping ErrAtCapacity when no slot is free. Write internal/agent/intake_capacity_test.go first using the intake_test.go harness, see the tests fail for a behavioral reason, then implement. Commit tests and implementation together."
    dependencies: ["manager-slot-core"]
    acceptance_criteria:
      - "TestIntakeRunHoldsSlot passes: max 1, an in-flight intake run makes Spawn for another issue return ErrAtCapacity, and the slot frees after each of committed, punched out, stopped and failed"
      - "TestIntakeAttemptRetryKeepsSlot passes: no free slot is visible between a failed attempt and its retry attempt"
      - "TestIntakeStartAtCapacityStartsNothing passes: ErrAtCapacity, no agent entry, no files under .kairon/intake or the intake log"
      - "TestSpawnAdmissionIntakeAtCapacityReturnsErrAtCapacity passes: errors.Is(err, ErrAtCapacity) and not ErrAdmissionUnavailable"
      - "TestValidate_AtCapacityReportsError passes: Status ValidateError with Err wrapping ErrAtCapacity"
      - "TestOrphanIntakeMarkerCountsTowardLimit passes: a live PID marker for an unowned issue counts toward Slots() and blocks Spawn at max 1, a dead PID marker does not"
      - "Every pre-existing test in internal/agent passes with no change to its assertions, under -race"
      - "The new tests were seen failing for a behavioral reason before the implementation, and that test name is in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/agent/..."
      - "test -z \"$(gofmt -l internal/agent)\""
      - "go test -race -count=1 ./internal/agent/..."

  - id: "watcher-cap-oldest-first"
    agent: "builder"
    description: "In internal/watcher/watcher.go: sort a copy of sel.Eligible ascending by issue number at the top of checkIssues (untrackAbsent keeps the original slice); add the Watcher.slots seam (New sets mgr.Slots when mgr is non-nil, otherwise 0 and cfg.EffectiveMaxAgents()); add the capacity gate after dependency validation and before w.tracked is set and incrementGlobalRetryCount runs, re-reading slots for every issue, logging log.Printf('[watcher] INFO: issue #%d waiting for an agent slot (%d/%d running)') once per waiting issue per poll and continuing without touching tracked or the retry count; handle agent.ErrAtCapacity from the spawn seam in the same branch as ErrNotAdmitted (untrack, refund, log the waiting line once, continue) so it never logs 'failed to spawn'. Write internal/watcher/watcher_capacity_test.go first using a production-constructed Manager over githubtest.Fake with StartAgent and MonitorAgent stubs, see the tests fail for a behavioral reason, then implement. Commit tests and implementation together."
    dependencies: ["manager-slot-core", "manager-intake-slots"]
    acceptance_criteria:
      - "TestCheckIssues_TwoSlotsFiveIssuesNeverMoreThanTwoRunning passes: max 2 and five eligible issues, never more than two running after any poll, the third starts only after one of the first two exits"
      - "TestCheckIssues_OneSlotStartsOldestIssueFirst passes: max 1, selectIssues returns #12, #10, #11, start order is #10, #11, #12"
      - "TestCheckIssues_RetryHoldsOnlySlot and TestCheckIssues_IntakeHoldsOnlySlot pass: no new issue starts while a retry (in the failed gap) or an intake run holds the only slot, and the oldest waiting issue starts after it ends"
      - "TestCheckIssues_WaitingIssueKeepsRetryBudget passes: the retry count file is unchanged after three consecutive polls at capacity, the issue is untracked, and the first poll after a slot frees starts it as attempt old+1"
      - "TestCheckIssues_LogsWaitingIssueOnceAtInfo passes: exactly one line with 'issue #N waiting for an agent slot (1/1 running)' and the INFO: tag per poll, and no WARNING:, ERROR: or 'failed to spawn' line for that issue"
      - "TestCheckIssues_BlockedOlderIssueDoesNotTakeSlot passes: an older issue blocked by an open dependency is neither started nor logged as waiting, and the next eligible issue starts"
      - "TestCheckIssues_SpawnAtCapacityUntracksAndRefunds passes"
      - "Every pre-existing test in internal/watcher passes with no change to its assertions, under -race"
      - "The new tests were seen failing for a behavioral reason before the implementation, and that test name is in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/watcher/..."
      - "test -z \"$(gofmt -l internal/watcher)\""
      - "go test -race -count=1 ./internal/watcher/..."
      - "go test -race -count=1 ./internal/agent/... ./internal/config/..."

  - id: "document-max-agents"
    agent: "documenter"
    description: "Document max_agents. README.md: add max_agents to the config sample and the config table (default 3, positive integer, no unlimited setting) and a short paragraph that at most max_agents agents run at once, that implementation runs, retries and intake runs all count, that the oldest issue starts first, and that a waiting issue is not charged a retry and is logged at info level as 'issue #N waiting for an agent slot (M/M running)'. Create docs/concurrency.md covering the setting and default, what holds a slot and when it is released, oldest-first order, waiting issues and the retry budget, the log line, validate at capacity, intake runs left by a previous watcher, and how to change the limit. Add one sentence to the validate section of docs/boundary.md that validate reports an error when no slot is free. Statements must match the code from the builder tasks. Do not add a Known gaps entry."
    dependencies: ["config-max-agents"]
    acceptance_criteria:
      - "README.md config sample and config table contain max_agents with default 3"
      - "docs/concurrency.md exists and describes the setting, the slot rules, oldest-first order, the retry-budget rule and the exact log line"
      - "docs/boundary.md validate section says validate reports an error when no agent slot is free"
      - "grep -rn max_agents README.md docs cmd/kairon/templates matches in README.md, docs/concurrency.md and cmd/kairon/templates/kairon/config.yaml"
      - "No document uses the words deferred, follow-up or future work about max_agents behavior"
    validation_commands:
      - "grep -n max_agents README.md"
      - "grep -n max_agents docs/concurrency.md"
      - "grep -rn max_agents README.md docs cmd/kairon/templates"
      - "test -f docs/concurrency.md"
      - "task sync:check"

  - id: "validate-all"
    agent: "validator"
    description: "Verify the whole change against issue 473: each acceptance criterion has its named passing test, slot accounting is reserved under m.mu before any worktree, file or process is created, released on every terminal path, held across the retry gap and across intake attempts, the watcher sorts oldest-first, gates after dependency validation and before the retry counter, logs the waiting line once per poll with the INFO: tag, max_agents rejects zero and negative values with a message containing max_agents, docs and the config template mention max_agents, task sync:check passes, task test passes, and the spec deviations (if any) are recorded in code comments and the sentinel."
    dependencies: ["watcher-cap-oldest-first", "document-max-agents"]
    acceptance_criteria:
      - "task test exits 0"
      - "task lint, task fmt:check and task sync:check exit 0"
      - "grep -rn max_agents README.md docs cmd/kairon/templates finds a match in the README config reference, docs/concurrency.md and the config template"
      - "Each of the eight acceptance criteria of issue 473 maps to a named test that passes"
      - "go test -race -count=5 on internal/agent and internal/watcher shows no data race"
      - "The spec's Deferred work section says none and no changed file mentions deferred max_agents work"
    validation_commands:
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "grep -rn max_agents README.md docs cmd/kairon/templates"
      - "go test -race -count=5 ./internal/agent/... ./internal/watcher/... ./internal/config/..."
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-473-cap-concurrent-agents-oldest-first.md"
```
