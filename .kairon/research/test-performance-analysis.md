# Analysis: `task test` Performance

Status: research / plan input. Not a design spec.
Audience: Kairon maintainers deciding how to make the test gate fast enough that nobody skips it.
Measured: 2026-10-28 on a 12-core Apple Silicon Mac (Go 1.26.5, Docker 29.9.2 running), and from the
GitHub Actions CI run for PR #416 (`ubuntu-latest`).

## 1. Summary

`task test` (`go test -v -race -coverprofile=coverage.out ./...`) takes **7–8 minutes** on a developer Mac
whenever a change touches the eval harness or anything it depends on. That is true of more than half of
recent commits. The builder, the validator and the PR-comment and merge-conflict skills all gate on it, and a
single issue run can pay it several times. CI on Linux takes 3m55s for the same step.

The time is very concentrated:

- **One package sets the wall time.** Packages run in parallel, and `internal/eval` finishes at 417s.
  Every other package is done by 146s. Nothing outside `internal/eval` shortens the gate until
  `internal/eval` itself gets faster.
- **Within that package, 48 of 807 tests take 345s of its 414s.** Each one runs a whole stub eval suite end
  to end, often the full 22-case `selftest` suite and often several times per test. Its tests are also fully
  serial: the package keeps its configuration in a global, and its tests swap `PATH`, the working directory
  and function variables.
- **Each suite run is about 165 subprocesses, mostly `git`.** On macOS, process creation and file-system
  work cost about 2.3× what they cost on Linux. System time (212s) exceeds user time (158s).
- **The second-longest package, `internal/eval/sandbox` (145s), mostly waits on timeouts.** 14 container
  tests each wait out a 10-second `docker stop` grace period. The container runs `sleep` as PID 1, and
  `sleep` ignores SIGTERM. Production eval runs use the same container setup, so they probably pay the same
  10 seconds per container teardown.

**Audit of whether each test is load-bearing (§8):** about one in four tests is a deletion candidate, and
about one in nine is a copy-paste variant that belongs in a table. The suite isn't empty filler: nearly every
test asserts something. The weakness is redundancy, tests of trivia or the wrong layer, and slow setup. The
audit also found two real bugs: `kiro-cli` calls can hang past their timeout, and the real hotkey handler
has no test.

**Targets (maintainer decision, 2026-10-28):**
- **Required:** the full `task test` finishes in **under 2 minutes** on a local dev machine.
- **Goal:** it finishes in **under 1 minute**. For now that may be met by a subset of the tests, while the
  full suite works its way down to the same mark.
- **Evals are never part of `task test`.** Evals call real models, so their results vary from run to run, and
  they run through `kairon eval`. The stub-backend suite runs inside `internal/eval`'s tests are a different
  thing: they are deterministic tests of the eval harness, they make no model calls, and they stay in
  `task test`.

**North star: every test runs in every dev loop.** The full `task test`, with `-race` and every package,
must be cheap enough to run on every change. There is no separate fast tier and no "the slow ones run in CI".
Moving slow tests out of the dev loop doesn't fix them. It hides them in CI, where waiting on a run holds up
dependent issues and the slowness never feels urgent enough to fix. When every dev loop pays for the full
suite, a slow test gets noticed the day it is added, and the suite stays tight.

The plan has two tiers:

- **Tier 1 is low-effort.** It should take the uncached gate from about 7.5 minutes to an estimated 1.5–2.5
  minutes on a Mac, and it adds a time budget that fails the gate when the suite gets slow again.
- **Tier 2 changes how the tests are structured.** It is aimed at an uncached full `task test` of about
  45–75 seconds on a Mac, with every test still included.

## 2. How the measurements were made

| Run | Command | Wall | Notes |
|-----|---------|------|-------|
| Cold, no cache | `go test -race -coverprofile=… -count=1 -json ./...` | **421s** | user 158s, sys 212s; per-test timings below come from this run |
| Gate flags, cold | `go test -v -race -coverprofile=… ./...` | **468s** | 9 packages cached from earlier runs |
| Gate flags again | same | **436s** | 0 packages cached (see §4.4) |
| Exact `task test` twice in a row | `go test -v -race -coverprofile=coverage.out ./...` ×2 | **469s, then 43s** | second run: everything cached except `cmd/kairon/cmd` |
| CI (`ubuntu-latest`) | `task test` | **235s** | `internal/eval` 178s, `internal/eval/sandbox` 169s |

Package timeline from the cold run. All packages start within the first second:

| Package | Ends at | Sum of its tests | Tests | Where the time goes |
|---------|--------:|-----------------:|------:|---------------------|
| `internal/eval` | **416.8s** | 414s | 807 | 48 end-to-end suite runs = 345s |
| `internal/eval/sandbox` | 146.4s | 144s | 100 | 14 container tests × ~10s = 142s |
| `cmd/kairon/cmd` | 43.4s | 41s | 72 | 4 `kairon eval` suite runs = 36s |
| `internal/agent` | 22.8s | 20s | 61 | `TestWorktreeCreate` = 18s (14 serial git-fixture subtests) |
| everything else | ≤ 13.6s | | | |

Totals: 237 test files, about 70k lines of test code against about 105k lines of production code, and 57
`t.Parallel()` calls, 56 of them in `internal/planner`.

## 3. Root causes

### 3.1 The sandbox container ignores SIGTERM, so every teardown waits 10 seconds (measured)

`Container.Cleanup` (`internal/eval/sandbox/container.go:445`) calls `ContainerStop` with a 10-second
timeout. The test containers run `sleep 30`, and the production runner's container runs
`sleep 2147483647` (`internal/eval/runner.go:982`). Neither starts an init process, so `sleep` is PID 1. PID 1
ignores SIGTERM unless it installs a handler, so Docker always waits the full grace period and then sends
SIGKILL.

Measured directly:

| Command | Time |
|---------|-----:|
| `docker stop` on `alpine sleep 30` | **10.14s** |
| `docker rm -f` on `alpine sleep 30` | **10.13s** |
| `docker stop` on `alpine sleep 30` started with `--init` | **0.11s** |

14 sandbox tests each pay it, about 140s of the package's 145s. Production sandboxed eval runs probably pay
the same 10 seconds per container teardown, since the runner uses the same pattern.

### 3.2 `internal/eval` runs whole eval suites end to end, and runs them often

There are 87 call sites of `RunWithOptions` and the related helpers in `internal/eval`'s tests. 42 of them
pass an empty case filter, so they run the whole suite. The `selftest` suite has 28 case files and
`selftest-fail` has 22. A full stub `selftest` run costs **4.3s**. A one-case run with three repeats
(`TestRepeatExecutionLoadsCasesOncePerRun`) costs **0.48s**.

Many slow tests assert something that one or two cases would show. Examples:

| Test | Time | What it runs | What it asserts |
|------|-----:|--------------|-----------------|
| `TestRepeatExecutionRepeatMultipliesCost` | 15.5s | full `selftest` at repeat 1, then at repeat 3 | cost at repeat 3 is 3× cost at repeat 1 |
| `TestRepeatDiffAgainstRepeatOneRunUsesRunOneOutput` | 16.7s | full suite, several runs | diff picks run 1's output |
| `TestRepeatExecutionResume*` (3 tests) | 14–15s each | full suite, interrupted and resumed | resume skips or refuses correctly |
| `TestProvenancePromptEditChangesHash` | 14.6s | full suite twice | a prompt edit changes the hash |
| `TestCandidateE2E*` (5 tests) | 5–11s each | full suite | candidate provenance recorded |

`cmd/kairon/cmd` repeats the pattern through the CLI. `TestEvalRepeatPassedToRun` (13.4s),
`TestEvalPromptFileRecorded` (9.7s), `TestEvalSelfTestSucceedsWithDefaults` (8.2s) and
`TestEvalSelfTestFailExitsWithThresholdError` (4.7s) all run whole suites. `internal/eval` already covers what
those suites exercise. The CLI tests only need to prove that the flags are wired through.

### 3.3 Every eval case workspace costs about 7 git processes (measured)

With a `PATH` shim that logs every `git`, `sh`, `gh` and `kiro-cli` call, one `TestSelfTestStubRunEndToEnd`
run made **165 subprocess calls**:

| Calls | Command | Where |
|------:|---------|-------|
| 28 | `git init` | `caseWorkspace.gitInit` (`internal/eval/workspace.go:540`) |
| 28 | `git add --all --force` | same |
| 28 | `git -c … commit` | same |
| 30 | `git rev-parse` | base commit, and the run's commit hash (`runner.go:1869`) |
| 29 | `git ls-files` | `.kiro` tracking check (`workspace.go:674`) |
| 19 | `sh -c` | `command` checks |

Across the roughly 85 suite runs in the package, that is on the order of 14,000 processes per `task test`.
Each one is a fork and exec from a large, race-instrumented test binary. Every case also builds a fresh
workspace on disk and walks it twice to set permissions (`workspace.go:700–703`).

### 3.4 `internal/eval`'s tests cannot run in parallel

- **Configuration lives in a global.** `cfg` is a package-level variable (`internal/eval/config.go:61`).
  `configure()` overwrites it on every run, and 31 test files call `resetConfig` (94 uses).
- **Tests swap global function variables.** Examples are `openContainerSession`, `invokeInContainer` and
  `resolveLinuxBinary` (`runner.go:1020–1166`).
- **Tests change process state.** 11 files call `t.Setenv` (21 uses), mostly to put a fake `kiro-cli` on
  `PATH`. 10 files change the working directory (17 uses). Go panics if `t.Setenv` is used in a parallel test.
- **Tests share fixture directories.** Many runs write into the shared
  `internal/eval/testdata/evals/results/` directory, and `runner.go:1931` takes a lock against concurrent
  evaluations.

The 807 tests therefore run one after another on 1 of 12 cores. The other packages finish in the first
2.5 minutes and the machine then sits mostly idle.

### 3.5 Tests write into the repository and defeat the test cache

Go caches a package's test result and replays it unless the package's code or a file its tests opened has
changed. Several tests write into the source tree:

| Writer | Path written | Effect |
|--------|--------------|--------|
| `cmd/kairon/cmd` tests | `internal/eval/testdata/check-cli/dir` | invalidates `internal/checks` and `cmd/kairon/cmd` itself, so `cmd` reruns (36s) on **every** run, even with no changes |
| `internal/eval` tests | `internal/eval/testdata/evals/results/` | 5 leaked run directories are present now; any test that lists this directory can miss the cache |
| `internal/tui` tests | `internal/tui/.kairon/sessions/<random>.json` | new randomly named files on every run |
| `internal/eval/sandbox` tests | `internal/eval/sandbox/.kairon/evals/tmp/containers.json` | rewritten on every run |
| (several) | `.kairon/kairon.log` at the repo root | appended on every run |

### 3.6 Smaller costs

| Item | Measured cost | Note |
|------|--------------:|------|
| `-race` | about +25–30% | 4.2s against 3.2s for one selftest run. It finds real bugs, so keep it in CI. |
| `-coverprofile` and `coverage.html` | about 1s | negligible |
| `-v` | negligible | about 160 KB of output |
| `TestWorktreeCreate` | 18s | 14 independent git-fixture subtests, each with its own temp repos, run serially |
| `time.Sleep` in tests | under 0.5s in the gate | the one 1.1s sleep is in a sandbox-only self-test that is not in the gate |
| `go list` subprocesses in `deps_test.go` | about 0.3s each, 8 packages | small, and not on the critical path |
| macOS against Linux | 2.3× for `internal/eval` | from process creation and file-system cost; any reduction in subprocesses saves about twice as much on a Mac |

### 3.7 How often the slow packages rerun

Of the last 80 commits on `main`:
- **44** changed Go code that `internal/eval`'s tests depend on (`internal/eval`, `checks`, `config`,
  `inference`, `logging`, `stageoutput`, `eval/sandbox`, `eval/dockerfile`). Each of these pays the full 7
  minutes.
- **61** changed code that `cmd/kairon/cmd`'s tests depend on (25 internal packages).

The test cache is not a fix on its own: the slow package is also the one that changes most often.

## 4. Plan

The ordering rule: until `internal/eval` gets faster, no other fix changes the wall time. After that comes
`internal/eval/sandbox` at 146s, then `cmd/kairon/cmd` at 43s, then `internal/agent` at 23s. Each tier is
ordered by wall-time impact.

The gate times below are **estimates** from the measurements above, not measured results. Re-measure after
each step.

### Tier 1: high impact, low effort

| # | Change | Effort | Expected effect |
|---|--------|--------|-----------------|
| 1.1 | **Shrink suite runs in tests that do not need the whole suite.** Pass a one-case filter (or a small fixture suite) wherever the assertion is about the mechanism (repeat, resume, diff, provenance, cost arithmetic, CLI flag wiring), not about the suite's contents. Keep a small number of full-suite runs (`TestSelfTestStubRunEndToEnd`, the passing and failing selftest verdict tests) as the end-to-end guard. Do the same in `cmd/kairon/cmd`. | Low to medium: mechanical, but each of the ~48 tests needs a short review of what it asserts | `internal/eval`: about 345s of e2e time down to an estimated 60–100s. `cmd/kairon/cmd`: 36s down to about 5s. |
| 1.2 | **Start sandbox containers with an init process.** Set `HostConfig.Init = true` in the sandbox's container configuration (`internal/eval/sandbox/mounts.go:108`) and in the test helpers that build their own `HostConfig`, or make `Cleanup` kill instead of stop. | Low: a one-line change plus a test that times `Cleanup` | `internal/eval/sandbox`: 145s down to about 10s. CI: 169s down to an estimated 30s. Production sandboxed eval runs probably lose 10s per container teardown too. |
| 1.3 | **Stop writing into the source tree.** Point every test's results, sessions and logs at `t.TempDir()`. Fix `cmd/kairon/cmd`'s write to `internal/eval/testdata/check-cli/dir`. Delete the leaked `testdata/evals/results/*` directories, and add a test that fails if the run leaves anything behind. | Low | `cmd/kairon/cmd` (36s) and `internal/checks` come from the cache when unchanged. Reruns after a change in an unrelated package drop to a few seconds. |
| 1.4 | **Run `TestWorktreeCreate`'s subtests in parallel.** Each one already builds its own temp repos, and none uses `t.Setenv` or `Chdir`. | Low | `internal/agent`: 23s down to about 7s. This only matters once the items above are done. |
| 1.5 | **Enforce a time budget in `task test` itself.** Run the suite with `-json` and check the result: fail the gate when the whole run, any package, or any single test goes past its budget. The whole-run budget ends at the targets in §1: 2 minutes required, 1 minute as the goal. Report the slowest tests on failure. Start the budgets just above the measured times after each step and tighten them as the work lands. An over-budget test is fixed or justified with a raised budget in the same change, never skipped or moved out of the gate. | Low: a small script or Go program wrapping `go test -json` | The suite can no longer slowly drift back to minutes. A slow test fails the dev loop the day it is added, which is the point of the north star. |
| 1.6 | **Require a running Docker or Podman for `task test`** (maintainer decision, 2026-10-28). (1) Add a preflight step to `task test` that checks for a daemon the same way the sandbox does (`DOCKER_HOST`, the default endpoint, then the known Podman sockets) and stops with one clear message that names Podman and Docker. (2) Change `skipIfNoContainerDaemon` (`internal/eval/sandbox/container_daemon_test.go:62`), and any other daemon skip, from `t.Skip` to `t.Fatal`, so a bare `go test ./...` enforces the same rule. Every machine then gets the same coverage. **CI needs no change:** `ubuntu-latest` runners have Docker installed and running, and the container tests already run there (`TestContainer_Lifecycle` passed in 10.35s on PR #416). One CI cost to watch: images are pulled fresh on every run (`alpine:3.19` and the base image; `TestCreateWithPlatform` spent 6.6s pulling the amd64 image). If pulls start to matter, cache the images or point the tests at an image already on the runner. | Low | Same coverage on every machine. No speed effect. |

**Estimated result after Tier 1:** the uncached gate drops from about 7.5 minutes to about 1.5–2.5 minutes on
a Mac. Tier 1 alone may not meet the 2-minute requirement. Tier 1 plus the pruning in §8 should, because the
rewrites in §8.3 are mostly the same tests as item 1.1. `internal/eval` is still the critical path at roughly 90–150s. A rerun after a change outside the eval
dependencies takes seconds.

### Tier 2: high impact, more effort or a change to the test approach

| # | Change | Effort | Expected effect |
|---|--------|--------|-----------------|
| 2.1 | **Make an eval run reentrant, then run `internal/eval`'s tests in parallel.** Replace the global `cfg` and the swappable function variables with a runner value that carries its configuration and dependencies. Have `RunOptions` take the `kiro-cli` path and the working directory instead of reading `PATH` and the current directory. Give each run its own results directory, so the lock in `runner.go:1931` no longer serializes tests. Then add `t.Parallel()`. This also opens the way to running cases in parallel in real eval runs. | High: touches the runner's core and about 30 test files | `internal/eval` uses all cores instead of one: an estimated 90–150s down to 20–40s. This is the largest single structural win. |
| 2.2 | **Build each eval workspace from a cached template instead of running git per case.** Run `git init`, `add` and `commit` once per fixture per test process, then copy the directory, including `.git`, for each case. Or write the single fixture commit in process. Compute the `.kiro` tracking check and the base commit from what was written, instead of asking `git`. | Medium | Removes most of the roughly 146 git processes per suite run. Worth about 2× on `internal/eval` on a Mac, and speeds up real eval runs. |
| 2.3 | **Split `internal/eval` into smaller packages.** For example: suite and case loading, the runner, scoring and results, provenance and repeat. `go test` runs packages in parallel, so the split parallelizes without making every test safe for `t.Parallel()`. It also narrows how much code each change reruns (§3.7). | High: a package restructure, interacting with the planned move of the eval harness to a standalone app | Spreads `internal/eval`'s time across cores, and fewer commits rerun all of it. Best done together with that move. |

**Estimated result after Tier 2:** the full `task test`, with every test and `-race`, takes about 45–75s
uncached on a Mac. That meets the 1-minute goal at the low end of the estimate, and comes close at the high
end. The run takes and a few seconds when cached. CI takes well under 2 minutes. The dev loop and CI run the same
command.

### Rejected: making the dev loop cheaper by running less of it

These would cut dev-loop time quickly, but they go against the north star, so they are not part of the plan:

- **A permanent fast tier that replaces the full suite in the dev loop** (`testing.Short()`, build tags, or a
  `task test:fast` for the skills' gates). It hides the slow tests instead of fixing them, and moves the wait
  into CI. A subset may meet the 1-minute goal for now (§1). That is a step toward the full suite meeting the
  same mark, not a substitute for it. The full suite still has to finish in under 2 minutes, and it still has
  to run before a PR is opened.
- **Dropping `-race` from the dev loop only.** It would save about 25%, but a race found only in CI is found
  late. The cost of `-race` is part of what the time budget measures.
- **Skipping container tests in the gate on purpose.** They should become fast (1.2), not optional. They
  already skip on a machine with no daemon; see the open question in §6.

## 5. Recommended order

0. **Prune first (§8).** Delete the tests the audit found carry no weight, merge copy-paste variants into
   tables, and fix the two real bugs (§8.4) before optimizing anything. There's no point making a test
   faster when it should be deleted, and every later step then works on a smaller suite.
1. **1.5 (time budget)**, set just above today's measured times. It records the baseline that every later
   step is measured against, and it stops anything new from getting slower while the rest lands.
2. **1.2 (sandbox init)** and **1.3 (no writes into the tree)**. Each is a small change with a clear test,
   and both pay off right away in CI and in cache hits.
3. **1.1 (narrow suite runs)**. This is the change that actually moves the wall time. Do it in batches by
   test file, re-measure after each batch, and tighten the budget as it drops.
4. **1.4 (worktree subtests in parallel)**, once `internal/agent` is near the critical path.
5. **2.1 (reentrant runner and parallel eval tests)**, then **2.2 (workspace templates)**. Consider **2.3**
   as part of the eval harness's planned move out of Kairon.

## 6. Open questions for the maintainer

- **Opt-in sandbox self-tests:** 9 tests skip in every `task test` run, CI included, unless
  `KAIRON_EVAL_SANDBOX_SELFTEST=1` is set. They run through `task eval:selftest:sandbox` instead. Examples are
  `TestSelftestSandbox`, `TestSandboxWorkspace` and the containment and parity tests. They are deterministic
  (stub backend in a container), so the north star argues for bringing them into `task test`. They were made
  opt-in because they are slow. Decide once items 1.2 and 1.6 have made container tests fast, and measure
  them then.
- **CI timeout:** The CI job's 10-minute timeout leaves room today (3m55s for tests), but the trend should
  be watched until Tier 1 lands.

## 7. Caveats

- **Test cache anomaly:** one run (the third row in §2) cached zero packages on an unchanged tree. Two
  back-to-back runs of the exact `task test` command later behaved as expected, and the anomaly did not
  reproduce. §3.5 lists the known invalidators.
- **Measurement noise:** timings come from one machine with other light work running, and differ by about
  ±10% between runs. The ratios are stable: which packages and tests dominate, and the cost of a full suite
  run against a one-case run.

## 8. Supplemental audit: is each test load-bearing?

Before making the tests faster, we checked whether each one earns its place: does it cover code nothing
else covers, and does it check behaviour that matters? The per-test verdicts are in
[test-audit-verdicts.md](test-audit-verdicts.md).

### 8.1 Method

1. **Per-test coverage.** Each of the 1,996 top-level tests was run alone, in its own process, with
   `-coverpkg=./...`. That records exactly which statement blocks each test reaches across the whole module.
2. **Static scan.** An AST pass over every test function counted its assertions, both direct and through
   helpers. It looked for tautologies, smoke-only tests and tests that read docs or prompts, and it grouped
   tests whose bodies have the same shape.
3. **Judged review.** Every test the measurement flagged was read next to the test flagged as its partner,
   1,021 tests in all. A test was flagged when it covers no Go code, covers only lines a single other test
   also covers, or is slow and adds no unique coverage. Each got one verdict: KEEP, MERGE (fold into a
   table with a named partner), REWRITE-CHEAPER (keep the assertion, cheaper setup) or DELETE (with a
   stated reason). I re-checked a random sample of the DELETE verdicts and the dead-code claims myself.

Line coverage can't judge a test on its own. Two tests can cover the same lines and assert different things.
Coverage is also blind to tests of shell scripts, prompts and YAML: `TestWorktreeCreate` covers 0 Go
blocks, but it is a real contract test of `worktree-create.sh`. That is why every flagged test was reviewed
by hand rather than deleted on the numbers.

### 8.2 What the measurement shows

- **Every test asserts something.** Only 2 of 2,000 tests have no assertion at all, no tautologies were
  found, and only 2 tests are smoke-only. Where the suite is weak, the cause is redundancy, and tests that
  check trivia or the wrong layer. The tests are not empty.
- **A third of the tests reach all the coverage.** 629 tests (124s serial) reach all 6,751 blocks that the
  full 1,987 passing tests (677s serial) reach. 871 tests cover only a subset of what one other test in the
  same package covers.
- **Coverage is uneven.** The suite reaches 74.7% of statements. `internal/eval` is at 87.6% and carries
  most of the redundancy, while `internal/acp` is at 2.6%, `internal/session` at 35% and `internal/tui` at
  51%. Some live files have no coverage at all: `internal/session/planner.go` (216 statements),
  `internal/eval/progress.go`, `internal/eval/sandbox/build_output.go` and `internal/templates/extract.go`.
  Test effort has gone where it was least needed.

### 8.3 Verdicts

Of the 1,021 flagged tests:

| Verdict | Tests | Seconds (each run alone) |
|---------|------:|-------------------------:|
| KEEP | 491 | 66 |
| MERGE into a table | 225 | 53 |
| REWRITE-CHEAPER | 54 | 289 |
| DELETE | 251 | 102 |

Combined with the 966 tests that were not flagged, the proposal shrinks the suite from 1,996 top-level tests
to about 1,745. About 225 of those become rows in existing tables, which leaves roughly 1,520 test functions.
The seconds column comes from running each test in its own process, so for fast tests it includes about 0.5s
of start-up on each package's first test. Most of the time saved comes from the 54 rewrites, not the
deletions.

Recurring patterns:

1. **Whole-suite runs to check one fact** (REWRITE-CHEAPER, most of the 289s). Each of these tests runs the
   whole 22–28-case stub suite, sometimes several times, then checks one case or one counter. Examples:
   - `TestSelfTestChecksWrongTurnFails` and `TestSelfTestJudgeListIsOneOfTwo` each check a single case.
   - The repeat, resume and diff tests multiply the full suite. The slowest,
     `TestRepeatDiffAgainstRepeatOneRunUsesRunOneOutput` (12–16s), only asserts that `diffTo` returns nil.
   - The CLI tests `TestEvalPromptFileRecorded` and `TestEvalRepeatPassedToRun` run whole suites to prove
     one flag is wired.

   The fix is a one-case filter, a two-case fixture suite, or one shared run per test file. Keep
   `TestSelfTestStubRunEndToEnd` as the single full-suite guard. This is item 1.1, now with a per-test
   list.
2. **Container tests that repeat the same lifecycle** (DELETE or MERGE). Several sandbox tests each start
   their own container, about 10s each, just to re-check `uname`, `echo` or a URL mapping that a unit test
   already covers. Examples are `TestPlatformSpecificImagePulling`, `TestArchitectureDetectionInContainer`
   and `TestKiroCLIInstallationMocking`, which mocks nothing. The few checks that are distinct, such as the
   cgroup memory limit and the kiro-cli not-found message, should move into containers that other tests
   already start.
3. **Copy-paste duplicates and one input per test** (MERGE or DELETE). Examples:
   - `TestFooterRendersExactly3Lines` is byte-identical to its partner.
   - `TestIsKaironContext` is a line-for-line copy of `TestHotkeyErrorHandling` (I verified this).
   - Each `TestEnterKey*Tab*` test repeats one row of `TestEnterKeyCommandExecutionInAllTabs`.
   - `TestPlanValidate_*`, `TestParsePlanFromMarkdown_*` and the six `PriceTable_Cost` tests each test one
     input in its own function, where a table would do.
4. **Tests that check Go, a library, constants, a test double or their own setup** (DELETE). Examples:
   - The five `internal/eval` architecture tests, `TestChecksCheckTypeSet` (constants compared with
     themselves) and `TestTabInterfaceDefinition` (a test double's hard-coded returns).
   - `TestPrereleaseConcatenation` and `TestEmptyPrerelease`, which copy `init()`'s logic into the test.
   - `TestWorkspacesAreIndependent`, which can't fail (I verified this).
   - `TestStatusRunningAgentsDeterministicOrder`, whose only check is an empty `if`.
   - `TestIssueStoreImplementsProvenanceStore`, which should become a compile-time
     `var _ provenance.IssueStore = IssueStore{}`.
5. **Tests that pin documentation, prompt or skill text** (DELETE, all of them; see §8.6). The reviewers
   kept some of these under the old rules, such as docs tests that run documented YAML examples and prompt
   tests for tokens like `Document path:`. The maintainer's rule now overrides those verdicts.
6. **Tests of dead or test-only code** (DELETE together with the code).
   - `internal/plan`'s `Executor` (`NewExecutor`, `NewExecutorWithLimit`, `ExecutePlan` and its helpers)
     has no caller outside its own tests (I verified this). Delete that code and its tests. Don't delete
     the whole of `executor.go`: the same file defines `TopologicalSort`, which `kairon plan parse` calls
     (`cmd/kairon/cmd/plan.go:77`). `TopologicalSort` and its tests stay, moved to their own file if
     `executor.go` is removed.
   - `tui.ViewManager` has no production use.
   - The Podman-discovery tests exercise a helper that exists only for tests.
7. **The same check in two places.**
   - Live/template byte-equality tests repeat what `task sync:check` already enforces in CI, and two of
     them are the same test.
   - `internal/tui` tests re-test `agent.OutputCapture`, which `internal/agent` already covers.

### 8.4 Real bugs found

- **`kiro-cli` calls can hang past their timeout.** `internal/inference/kirocli.go:63` and `:125` start
  `kiro-cli` with `exec.CommandContext`, with no `WaitDelay` and no process-group kill. If a timed-out
  `kiro-cli` leaves a child process that still holds stdout, `Wait` blocks until that child exits.
  `TestInvokeAgentDefaultBackendTimeout` sets a 200ms timeout and takes 5.2s, and it passes only because it
  never checks elapsed time. The stub backend already handles this (`stub.go:234`, `stubcmd_unix.go`).
  Fix: apply the same `WaitDelay` and process-group cancel to the `kiro-cli` path, and make the test assert
  that the call returns soon after the timeout.
- **`internal/hotkey`'s real handler is untested.** All six of its tests compare literals, drive a mock or
  repeat each other, so `hotkey.HandleKeyMsg`, which the TUI uses, has no real test. Replace them with one
  test that sends ctrl+alt+p both inside and outside a kairon context.
- **A docs test can pass vacuously.** stageoutput's `TestDocs_InvalidExamplesAreRejected` passes when the
  doc contains no invalid examples. Add a check that at least one exists.
- **Fixed waits stand in for synchronization.** The ACP test fixture's `collectWithin` waits a fixed 60ms per
  message hop, adding 0.4–0.8s per test. Command-timeout tests use 400ms–1s where much less would do.

### 8.5 Where test effort should go instead

Pruning frees up test effort for the code that has little or none: `internal/acp` (2.6%),
`internal/session/planner.go` (0%), `internal/tui` (51%), `internal/hotkey`'s real handler, and
`internal/eval/progress.go`. Add those tests under the time budget (1.5), so the gaps get closed without
slowing the suite down again.

### 8.6 Tests that pin documentation, prompt or skill text: complete inventory

**Rule (maintainer decision, 2026-10-28):**
- No test may pin documentation to anything. Docs are temporary and get regenerated.
- No test may pin the wording of an agent prompt or a skill. Prompts and skills are code for the model:
  test what they produce, not their text.
- Tests of the pipeline's own output (stage-output documents, input documents, the planner's document) are
  behaviour tests, and they stay.

The audit in §8.3 reviewed only tests that the coverage measurement flagged. So this inventory traced every
test instead: each of the 1,996 top-level tests was run alone with Go's test log
(`-test.testlogfile`, the file-access record the test cache uses), and every repository file it opened was
recorded. 66 tests open documentation, prompt or skill files.

| Group | Tests | Action |
|-------|------:|--------|
| **Documentation:** `docs/*.md`, plus two examples parsed from `.kairon/research/` | 26 | Delete. These are `TestDocs_*` in `boundary`, `failure`, `guard`, `provenance` and `stageoutput`; `TestChecksDocs*` in `checks`; and `TestWorkflowDocs*` and `TestSimulatedUserDoc*` in `eval`. The docs tests that run YAML examples go too. Two parser tests in `stageoutput` (`TestParse_CanonicalExampleFromSpec`, `TestParse_PunchOutExampleFromSpec`) check parser behaviour, but they read their input from a research doc. Keep the behaviour by moving those two inputs into `testdata/`, and stop reading the doc. |
| **Prompt and skill wording:** `internal/planner`'s `TestAutomatedPrompt*`, `TestInteractivePrompt*` (except the template-match ones) and `TestSkill*` | 26 | Delete. This includes the tests for tokens that the code reads from agent output (`Document path:`, `Label answer path:`, the yes/no label answer), the Dependencies line format, the examples embedded in the skill, and skill front matter. What those tests protected is now covered by checking the agent's output (see below). |
| **Live/template copies of prompts and skills:** `TestAutomatedFilesMatchTemplates`, `TestInteractiveFilesMatchTemplates`, `TestInteractiveTemplatePromptMatchesLive`, `TestTemplateSkillIsByteIdentical`, `TestTemplateSkillDirHasOnlySkillFile`, `TestSyncGate_*` | 9 | Delete. They repeat `task sync:check`, which CI runs. `TestSyncGate_TemplatesHaveNoPlannerConventionsSkill` is a copy of `TestTemplatesContainNoPlannerConventionsPath`, which stays (next row). |
| **Templates ship no `planner-conventions` skill:** `TestTemplatesContainNoPlannerConventionsPath` | 1 | Keep. It enforces a packaging rule that `task sync:check` does not check: the templates must never ship the project-owned `planner-conventions` skill, because shipping it would let `kairon update` overwrite a project's local guidance. It checks that a path is absent and pins no prompt or skill text. |
| **CLI audit run against the real prompts:** `TestAuditRealBuilderListsSkillsAndHooks`, `TestAuditRealPlannerHasRTCCHeadings` | 2 | Delete. They check the structure of the live prompts through `kairon eval audit`. The audit command's own behaviour is already tested against fixtures. |
| **Code that reads prompts as input:** `TestSelfTestWorkspaceCasesNative` (copies the real `.kiro/` into a workspace) and `TestEvalDriverOverlayAndResults` (the planner eval driver's overlay) | 2 | Keep. They test what the code does with the files, not what the files say. (`TestSelfTestWorkspaceCasesNative` is still a REWRITE-CHEAPER candidate in §8.3.) |

In total, 61 tests are deleted and 2 parser tests are rewritten to use `testdata/`. Both of those are counted in the documentation row. 3 tests stay. The
companion verdicts file reflects the earlier rules; where it conflicts with this table, this table wins.

**Where the planner's behaviour is tested instead.** The planner eval cases are in `.kairon/evals/cases/planner/`:
`automated-best-effort-fill`, `automated-bug-classification`, `automated-punch-out`, `interactive-option-rules`,
`interactive-return-document`, `interactive-single-decision` and `multi-issue-split`. They check what the planner
produces. Before deleting the prompt tests, map each one to the eval case that checks the same behaviour. These
behaviours must have an eval case:
- the interactive planner writes its document to the given path and records the label answer
- the automated planner punches out instead of asking
- inferences carry evidence
- the Dependencies line uses the right format

Add an eval case for any behaviour that has none, so nothing the prompt tests protected stops being checked.
Eval cases run through `kairon eval`, never through `task test` (§1). Evals call real models and their results
vary from run to run, so they can't serve as a deterministic gate. Run the evals for a prompt whenever the
prompt changes.
