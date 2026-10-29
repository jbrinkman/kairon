# Design Spec: Prune redundant internal/eval tests, part 2 of 5

Closes #424

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/eval`: `container_agent_test.go`, `container_runner_test.go`, `container_session_test.go`, `containment_test.go`, `coverage_run_test.go`, `coverage_test.go`, `criterion_floor_test.go`, `diff_filter_test.go`, `diff_provenance_test.go`, `exit_status_test.go`, `filter_run_test.go`" | `prune-container-agent-tests`, `prune-container-runner-tests`, `prune-container-session-tests`, `prune-containment-tests`, `prune-coverage-run-tests`, `merge-coverage-stamp-cases`, `merge-coverage-configure-case`, `prune-coverage-summary-tests`, `delete-coverage-evaluate-tests`, `prune-criterion-floor-tests`, `prune-diff-filter-tests`, `prune-diff-provenance-tests`, `prune-exit-status-tests`, `prune-filter-run-tests` |
| C1 | Contract: "No production code changes. Every test not listed passes unchanged. Merged partners keep every assertion they had." | `capture-baseline`, `prune-container-agent-tests`, `prune-container-runner-tests`, `prune-container-session-tests`, `prune-containment-tests`, `prune-coverage-run-tests`, `merge-coverage-stamp-cases`, `merge-coverage-configure-case`, `prune-coverage-summary-tests`, `delete-coverage-evaluate-tests`, `prune-criterion-floor-tests`, `prune-diff-filter-tests`, `prune-diff-provenance-tests`, `prune-exit-status-tests`, `prune-filter-run-tests`, `gate-coverage-and-task-test`, `validate-prune` |
| AC1 | "The 14 tests marked Delete no longer exist." | `prune-container-agent-tests`, `prune-container-runner-tests`, `prune-containment-tests`, `prune-coverage-run-tests`, `prune-coverage-summary-tests`, `delete-coverage-evaluate-tests`, `prune-diff-provenance-tests`, `prune-exit-status-tests`, `gate-coverage-and-task-test`, `validate-prune` |
| AC2 | "Each of the 22 tests marked Merge is a case or subtest of its partner, and no longer a separate function. A partner that is itself marked Merge (`TestCoverageStampCaseFilterIsNeverComplete`, `TestCoverageStampNoOpWithoutSuiteEntry`) is a case of `TestCoverageStampFullRun`." | `prune-container-agent-tests`, `prune-container-runner-tests`, `prune-container-session-tests`, `prune-containment-tests`, `prune-coverage-run-tests`, `merge-coverage-stamp-cases`, `merge-coverage-configure-case`, `prune-coverage-summary-tests`, `prune-criterion-floor-tests`, `prune-diff-filter-tests`, `prune-diff-provenance-tests`, `prune-filter-run-tests`, `gate-coverage-and-task-test`, `validate-prune` |
| AC3 | "Package coverage does not drop." | `capture-baseline`, `gate-coverage-and-task-test`, `validate-prune` |
| AC4 | "Every remaining test passes unchanged, and the gate passes." | `gate-coverage-and-task-test`, `validate-prune` |
| R1 | Merge `TestContainerKiroCLIPromptOnStdinNotArgv` into `TestContainerExecsAreWrappedAndPromptStaysOnStdin` | `prune-container-agent-tests` |
| R2 | Merge `TestContainerHelperPromptOnStdinNotArgv` into `TestContainerExecsAreWrappedAndPromptStaysOnStdin` | `prune-container-agent-tests` |
| R3 | Merge `TestContainerTimeoutMapping` into `TestContainerTimeoutNamesEffectiveTimeout` | `prune-container-agent-tests` |
| R4 | Delete `TestContainerHelperReportedTimeout` (partner `TestContainerTimeoutNamesEffectiveTimeout`) | `prune-container-agent-tests` |
| R5 | Merge `TestBuildContainerMountsHelper` into `TestBuildContainerMountsKiroCLI` | `prune-container-runner-tests` |
| R6 | Merge `TestBuildContainerMountsKaironBinaryOnPathForEveryBackend` into `TestBuildContainerMountsKiroCLI` | `prune-container-runner-tests` |
| R7 | Delete `TestMapContainerErrorNamesGivenTimeout` (partner `TestContainerTimeoutNamesEffectiveTimeout`) | `prune-container-runner-tests` |
| R8 | Merge `TestContainerEnvPathAndEvalDir` into `TestContainerConfigUserEnvAndWorkdir` | `prune-container-runner-tests` |
| R9 | Merge `TestContainerSessionPerTurnRequestSetup` into `TestContainerSessionThreeTurnsOneOpenOneClose` | `prune-container-session-tests` |
| R10 | Delete `TestContainmentForRecordsKaironBinary` (partner `TestContainmentMatchesEnforcement`) | `prune-containment-tests` |
| R11 | Merge `TestContainmentKaironBinaryJSON` into `TestContainmentToolTrustSerialisesEmptyArray` | `prune-containment-tests` |
| R12 | Delete `TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray` (partner `TestContainmentToolTrustSerialisesEmptyArray`) | `prune-containment-tests` |
| R13 | Merge `TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched` into `TestUpdateIncrementalSummaryRecordsModeAndContainment` | `prune-containment-tests` |
| R14 | Delete `TestLegacyJSONStillUnmarshals` (no partner) | `prune-containment-tests` |
| R15 | Delete `TestRunFixturesLoadAndScore` (partner `TestRunFixturesShapes`) | `prune-containment-tests` |
| R16 | Delete `TestCoverageRunCaseEditChangesSuiteNotPrompt` (partner `TestSuiteSHA256HashesRawCaseBytes`) | `prune-coverage-run-tests` |
| R17 | Merge `TestCoverageResumeUnchangedSuiteContinues` into `TestCoverageResumeChangedSuiteRefused` | `prune-coverage-run-tests` |
| R18 | Merge `TestCoverageStampIncompleteRun` into `TestCoverageStampFullRun` | `merge-coverage-stamp-cases` |
| R19 | Merge `TestCoverageStampCaseFilterIsNeverComplete` into `TestCoverageStampFullRun` | `merge-coverage-stamp-cases` |
| R20 | Merge `TestCoverageStampNoOpWithoutSuiteEntry` into `TestCoverageStampFullRun` | `merge-coverage-stamp-cases` |
| R21 | Merge `TestCoverageConfigureResetsRunScopedFields` into `TestConfigureKeepWorkspaces` | `merge-coverage-configure-case` |
| R22 | Delete `TestCoverageSummaryJSON` (partner `TestCoverageUpdateIncrementalSummaryCopiesFields`) | `prune-coverage-summary-tests` |
| R23 | Merge `TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded` into `TestCoverageUpdateIncrementalSummaryCopiesFields` | `prune-coverage-summary-tests` |
| R24 | Delete `TestCoverageEvaluateProgressiveCaseFilterIncomplete` (partner `TestCoverageStampCaseFilterIsNeverComplete`, now a case of `TestCoverageStampFullRun`) | `delete-coverage-evaluate-tests` |
| R25 | Delete `TestCoverageEvaluateUnpinnedLeavesFieldsUnset` (partner `TestCoverageStampNoOpWithoutSuiteEntry`, now cases of `TestCoverageStampFullRun`) | `delete-coverage-evaluate-tests` |
| R26 | Merge `TestAgentVerdictFloorNamesSeveralCriteriaInOrder` into `TestAgentVerdictFloorBoundary` | `prune-criterion-floor-tests` |
| R27 | Merge `TestDiffFilterQualityPerDollarUsesFilteredCost` into `TestDiffFilterReportsOnlyMatchingCases` | `prune-diff-filter-tests` |
| R28 | Merge `TestDiffFilterNoMatchOnEitherSideOmitsAgent` into `TestDiffFilterBuiltInAgentAndNameKeys` | `prune-diff-filter-tests` |
| R29 | Merge `TestDiffCasesBlockOnlyListsMatchingCasesWithFilter` into `TestDiffFilterReportsOnlyMatchingCases` | `prune-diff-filter-tests` |
| R30 | Delete `TestContainmentDiffKaironBinary` (partner `TestDiffListsKaironBinaryDifference`) | `prune-diff-provenance-tests` |
| R31 | Delete `TestDescribeContainmentIncludesKaironBinary` (partner `TestDiffContainmentNoneVsPresent`) | `prune-diff-provenance-tests` |
| R32 | Delete `TestDiffLegacyVsContainerDoesNotFail` (partner `TestDiffLegacyFixtureVsContainer`) | `prune-diff-provenance-tests` |
| R33 | Merge `TestDiffLegacyFixtureVsRecordedNative` into `TestDiffLegacyFixtureVsContainer` | `prune-diff-provenance-tests` |
| R34 | Delete `TestRunSelftestPassesAndPrintsOneVerdictLine` (partner `TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent`) | `prune-exit-status-tests` |
| R35 | Merge `TestCaseAndFilterRunRecordsCombinedLabel` into `TestRunCaseAndFilterCombineWithAND` | `prune-filter-run-tests` |
| R36 | Merge `TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing` into `TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites` | `prune-filter-run-tests` |

## Deferred work

none

## Solution Approach

This is a test-only refactor of `internal/eval`. No production file changes. The issue's table lists 36 tests: 14 are deleted and 22 are folded into a named partner as a table row, subtest or extra assertion. The 14 deletes are R4, R7, R10, R12, R14, R15, R16, R22, R24, R25, R30, R31, R32, R34. The other 22 rows are merges.

**Characterization.** AGENTS.md requires pinned behavior before a refactor. The existing suite is the characterization. Measured on this worktree before design:

- `go test -count=1 -cover ./internal/eval` passes at **87.6%** of statements (453s on a loaded machine). There is no rounding margin to rely on, so `capture-baseline` also records the unrounded statement ratio from a coverprofile and the sorted list of top-level test names.
- All 36 tests and all partners exist today, at the file:line positions the issue cites (no drift). The AC1 grep matches 14 functions and the AC2 grep matches 22 today.
- Two of the less obvious folding instructions were tried against the real code in a throwaway test (since deleted) and pass: R17's refuse, revert, resume sequence on one interrupted run (the refused resume leaves `.progress` in place, and the reverted suite resumes with the original `suite_sha256` and `Complete` true), and R28's `agent==zz` output containing `Filter: agent==zz`, `Cost Delta:` and `  Tokens: +0` with no `a1:` or `acc`.
- Nothing outside `internal/eval/*_test.go` and the research docs references any of the 36 names. The only other mentions are the doc comments directly above three of the functions (R16, R17, R36); they leave with their function, or move onto the subtest that now carries the regression story (R36).

**Per-task routine (every `prune-*`, `merge-*` and `delete-*` task).**

1. Run the tests being removed and their partners, and see them pass. This is the "before" state.
2. For a Merge, fold the removed test's distinct input or assertion into the partner as described in the task. Drop anything the partner already asserts. Keep every assertion the partner had. Do not weaken or loosen any partner assertion. Converting a flat partner into a table or into subtests is fine only if each original assertion survives, row by row, with its failure message.
3. Delete the original function together with its doc comment, and any helper, type or import that nothing else uses afterwards. Carry doc comments that explain a regression onto the subtest or table row that now holds the check.
4. Run the partner with `-v` and confirm the prescribed subtest names appear as `--- PASS` (the validation commands count them). Run `gofmt -l internal/eval` and `go vet ./internal/eval`.
5. Commit one commit per task as `test(eval): ...` (Conventional Commits). The body names the tests removed and the partners that now carry them. AGENTS.md "Say what you observed" for a refactor means naming the tests relied on. Do not push.
   **Stage only the files the task names, by explicit path** (for example `git add internal/eval/container_agent_test.go internal/eval/container_runner_test.go`). Never use `git add -A`, `git add .` or `git commit -a`. The worktree carries unrelated uncommitted modifications to `.kiro/agents/*.json` (and the untracked spec under `.kairon/specs/`) that must not enter any commit. Before committing, `git diff --cached --name-only` must list only `internal/eval/*_test.go` files.

**Validation commands do not depend on commit state.** A builder commits at the end of every task, so a command must give the same answer before and after that commit. Commands in this plan therefore either read the working tree (`go test`, `go vet`, `gofmt -l`, `grep` and `awk` over `internal/eval/*_test.go`), or compare the working tree or `HEAD` against the merge base, `$(git merge-base HEAD origin/main)`. No command uses a bare `git diff`, `git diff --cached` or `git status`, which compare against the index and are empty after a commit. `origin/main` resolves in this worktree (`git rev-parse --verify origin/main` prints `3828534`, which is also the current `HEAD`), so the merge base is the commit the branch started from and stays fixed while the builders commit.

**Subtest naming.** `t.Run` turns spaces into underscores. New subtests in this spec are named with hyphens only, so the validation commands can count them mechanically. Existing subtest names are left as they are.

**Spec deviations.** None are planned. If a builder finds that a folding instruction below does not work against the real code, it records a deviation per AGENTS.md "No silent deviation": a code comment at the point of departure citing this spec section and the reason, plus a `Spec deviations` entry in its sentinel. If no alternative still delivers the row, that is a blocker (`**Status**: BLOCKED`), not a deviation.

**Points the issue leaves implicit, decided here.**

- The partner of R21, `TestConfigureKeepWorkspaces`, is in `internal/eval/execute_case_test.go`, which is not one of the 11 files in the In Scope list. The issue's table names it as the partner, and AC2 requires the merged case to be among its subtests, so `merge-coverage-configure-case` edits that one function. The Out of Scope line "Tests not listed in the table" does not exclude it, because the table lists it in the Partner column. No other file outside the 11 is edited. Partners that stay untouched in other files (`TestSuiteSHA256HashesRawCaseBytes` in `metadata_test.go`) are read-only.
- R11 follows the issue's reason text: the "legacy decodes false" half of `TestContainmentKaironBinaryJSON` is Go zero-value behavior and is not folded. The true/false marshal key check and round trip are folded.
- R22 deletes `TestCoverageSummaryJSON` whole, as the issue says. Its two non-key subtests (omit-when-unrecorded, legacy decode) are `omitempty` and zero-value behavior and are not carried over.
- R4, R7, R10 to R12, R14, R15, R24, R25, R30 to R32 and R34 are deletions the issue has already justified by a named partner. The tasks require the builder to confirm the partner passes and still contains the cited assertion before deleting, as a safety check. If the partner lacks it, add the missing row to the partner (the issue says the partner already covers it, so this is a check, not planned work).

## Relevant Files

All under `internal/eval/`, all test files. Nothing else is modified.

| File | Change |
|------|--------|
| `container_agent_test.go` | delete `TestContainerKiroCLIPromptOnStdinNotArgv`, `TestContainerHelperPromptOnStdinNotArgv`, `TestContainerTimeoutMapping`, `TestContainerHelperReportedTimeout` |
| `container_runner_test.go` | partner file for R1 to R3 (`TestContainerExecsAreWrappedAndPromptStaysOnStdin`, `TestContainerTimeoutNamesEffectiveTimeout`); delete `TestBuildContainerMountsHelper`, `TestBuildContainerMountsKaironBinaryOnPathForEveryBackend`, `TestMapContainerErrorNamesGivenTimeout`, `TestContainerEnvPathAndEvalDir` and the `fakeDoneCtx` helper; `TestBuildContainerMountsKiroCLI` becomes a table; `TestContainerConfigUserEnvAndWorkdir` gains env entries and a custom-workspace subtest |
| `container_session_test.go` | delete `TestContainerSessionPerTurnRequestSetup`; extend `TestContainerSessionThreeTurnsOneOpenOneClose` |
| `containment_test.go` | delete 5 tests (`TestContainmentForRecordsKaironBinary`, `TestContainmentKaironBinaryJSON`, `TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray`, `TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched`, `TestLegacyJSONStillUnmarshals`) plus `TestRunFixturesLoadAndScore`; extend `TestContainmentToolTrustSerialisesEmptyArray` and `TestUpdateIncrementalSummaryRecordsModeAndContainment` |
| `coverage_run_test.go` | delete `TestCoverageRunCaseEditChangesSuiteNotPrompt`, `TestCoverageResumeUnchangedSuiteContinues`; extend `TestCoverageResumeChangedSuiteRefused` |
| `coverage_test.go` | delete 8 tests; `TestCoverageStampFullRun` becomes a table; `TestCoverageUpdateIncrementalSummaryCopiesFields` becomes a two-row table |
| `execute_case_test.go` | `TestConfigureKeepWorkspaces` becomes a table (partner of R21 only) |
| `criterion_floor_test.go` | delete `TestAgentVerdictFloorNamesSeveralCriteriaInOrder`; `TestAgentVerdictFloorBoundary` gains a multi-criterion row |
| `diff_filter_test.go` | delete 3 tests; extend `TestDiffFilterReportsOnlyMatchingCases` and `TestDiffFilterBuiltInAgentAndNameKeys` |
| `diff_provenance_test.go` | delete 4 tests; `TestDiffLegacyFixtureVsContainer` becomes a two-row table |
| `exit_status_test.go` | delete `TestRunSelftestPassesAndPrintsOneVerdictLine` |
| `filter_run_test.go` | delete 2 tests; extend `TestRunCaseAndFilterCombineWithAND`; `TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites` becomes a two-row table |

Read-only references: `metadata_test.go` (`TestSuiteSHA256HashesRawCaseBytes`), `containment.go`, `runner.go`, `config.go` (production code the tests call), `.kairon/research/test-audit-verdicts.md`, `.kairon/research/test-performance-analysis.md` section 8, `Taskfile.yml` (`task test` is `go test -v -race -coverprofile=coverage.out ./...`; `task lint` is `go vet ./...`).

## Team Orchestration

- All implementation tasks are `builder` tasks that run **strictly in sequence**, one builder at a time in a shared worktree. Several tasks edit the same file (`coverage_test.go` has four) or edit a partner in another file (`container_agent_test.go` merges into `container_runner_test.go`). A half-edited test file breaks the package build for any parallel builder, so the dependency chain is linear.
- Sequencing rules that matter for correctness, not only for file conflicts:
  - `prune-container-agent-tests` runs before `prune-container-runner-tests`. R3 adds the `sandbox-limit-timeout` subtest that asserts the `--resource-limit timeout=` hint. R7 (`TestMapContainerErrorNamesGivenTimeout`) is deleted on the grounds that this subtest asserts that hint, so the subtest must exist first.
  - `merge-coverage-stamp-cases` runs before `delete-coverage-evaluate-tests`. R24 and R25 name partners (`TestCoverageStampCaseFilterIsNeverComplete`, `TestCoverageStampNoOpWithoutSuiteEntry`) that are themselves merged into `TestCoverageStampFullRun`. The deletes are only safe once those cases exist as rows there.
  - `prune-containment-tests` does R11 before R12 inside one task, so `TestContainmentToolTrustSerialisesEmptyArray` has its final shape before the summary duplicate goes.
  - `prune-diff-provenance-tests` does R33 before R32, so `TestDiffLegacyFixtureVsContainer` has its final shape before its duplicate goes.
- `capture-baseline` runs first, before any edit, so coverage and the test-name list are measured on the unmodified package.
- `gate-coverage-and-task-test` (builder, because it may need to restore a lost branch-covering input as a partner row) runs after all pruning tasks. `validate-prune` (validator, read-only) then re-verifies everything independently.
- krew-lead copies accepted deviations (if any) into the PR's `Spec deviations` section and writes `Deferred work: none` in the PR description.
- No documenter task: no user-facing behavior or doc changes, and no live doc mentions the removed tests.

## Step-by-Step Task Breakdown

### 0. `capture-baseline`

Before editing anything, record on the unmodified package:

- The sorted list of top-level test names: `go test -list '.*' ./internal/eval | grep '^Test' | sort > .kairon/artifacts/issue-424-tests-before.txt`.
- The unrounded statement coverage: run `go test -count=1 -coverprofile=.kairon/artifacts/issue-424-baseline.out ./internal/eval`, then compute `awk 'NR>1 {t+=$2; if ($3>0) c+=$2} END {printf "%.4f\n", 100*c/t}'` over the profile and write the number to `.kairon/artifacts/issue-424-baseline-coverage.txt`.
- The package must pass, and `go test -cover` must print 87.6%.

`.kairon/artifacts/` is git-ignored; nothing is committed and no commit is made for this task.

### 1. `prune-container-agent-tests` (container_agent_test.go, partners in container_runner_test.go)

Folds R1 to R3 and deletes R4.

**R1: `TestContainerKiroCLIPromptOnStdinNotArgv` into `TestContainerExecsAreWrappedAndPromptStaysOnStdin`.** Split the partner into two subtests, `kiro-cli-exec` (its current first half) and `helper-exec` (its current second half). Each subtest calls `chdirTemp(t)` itself. In `kiro-cli-exec`:

- Set `kiroStdout` to `"\x1b[1mhello\x1b[0m"` instead of `"ok"`, and use `prompt := nastyPrompt()`.
- Capture all return values: `out, cost, rec, ec, err := invokeAgent(...)`.
- Keep every existing assertion: exec count 1, the open-umask wrapper check, `raw[4] == "kiro-cli"`, `assertNoPromptInArgv(t, x.rawCmds)`, and the `strings.Contains(x.stdins[0], "PROMPT-MARKER")` check.
- Add the distinct assertions: `out == "hello"` (ANSI stripped); `x.stdins[0] == prompt` (exact stdin); `assertNoPromptInArgv(t, x.cmds)` (the unwrapped commands); `ec == nil`; `cost.UsageSource == "estimated"`, `cost.Model == "m1"`, `cost.TokensIn == len(prompt)/4`; `rec.Model == "m1"`, `rec.Estimated`, `rec.Role == "agent"`.
- `pinAgentModel("builder", "m1")` is already in the partner.

**R2: `TestContainerHelperPromptOnStdinNotArgv` into the same partner.** In `helper-exec`:

- Use `prompt := nastyPrompt()` and a stub turn with `Response: "scripted"` (the partner used `"p"` and `"r"`). The partner's helper half never put the nasty prompt through the helper path, so these inputs are distinct, not duplicated.
- Keep the existing `unwrapOpenUmask(h.rawCmds[0])` wrapped check.
- Add: `out == "scripted"`; `assertNoPromptInArgv` over `h.cmds` and `h.rawCmds`; `h.stdins[0]` contains `PROMPT-MARKER` and `len(h.stdins) == 1`; `strings.Join(h.cmds[0], " ") == "/usr/local/bin/kairon inference-exec --backend stub"`.

**R3: `TestContainerTimeoutMapping` into `TestContainerTimeoutNamesEffectiveTimeout`** (in `container_runner_test.go`). Add a third subtest `sandbox-limit-timeout` after the existing two (`kiro-cli host deadline`, `helper-reported timeout`, which stay unchanged). It holds the whole body of the deleted test: `chdirTemp`, `useAgentConfigs`, `cc.ResourceLimits.Timeout = 30 * time.Millisecond`, `&fakeExecer{block: true}`, `invokeAgent("builder", "p", cc, callOpts{})`, and all assertions: error present; `errors.Is(err, inference.ErrTimeout)`; the message contains `Container execution timeout after 30ms` and `--resource-limit timeout=`; `ec.Stderr` has prefix `timeout after 30ms`; `ec.Platform == "linux/arm64"` and `ec.WorkingDir == "/workspace"`; `rec.Error != ""`. This subtest keeps the `--resource-limit` hint assertion that R7's deletion depends on.

**R4: delete `TestContainerHelperReportedTimeout`.** The partner's `helper-reported timeout` subtest has an identical setup (stub backend, fake Linux binary, helper envelope with `timeout:true`) and asserts `ErrTimeout` plus the `ec.Stderr` prefix, which is strictly more. Confirm that before deleting.

After the task: remove imports in `container_agent_test.go` that nothing else uses. Check `go vet`.

### 2. `prune-container-runner-tests` (container_runner_test.go)

Folds R5, R6 and R8, deletes R7. Depends on task 1.

**R5 and R6: into `TestBuildContainerMountsKiroCLI`.** `buildContainerMounts` ignores its backend argument and returns the same ordered list for every backend, so one table over backend and workspace covers both deleted tests. Convert the partner to a table `{name, backend, workspaceDir}` with three rows:

- `kiro-cli-default-workspace`: `inference.NameKiroCLI`, `/workspace`.
- `stub-default-workspace`: `inference.NameStub`, `/workspace`.
- `stub-custom-workspace`: `inference.NameStub`, `/work/space`.

Every row asserts all of the following, with the expected list derived from the row's `workspaceDir` using `path.Join`:

- `len(ms) == 5`.
- `reflect.DeepEqual(ms, want)` with the partner's current five-element list (`.kiro` read-only, workspace, `.eval`, `/opt/kairon/bin` read-only, `/usr/local/bin/kairon` read-only with the fake binary as `HostPath`). This subsumes R5's per-path `mountSummary` checks. Keep the explicit `/opt/kairon/kairon` absence check from R5 as well.
- The `sandbox.NewHostConfigWithMounts` block from the partner unchanged: read-only rootfs, tmpfs for `/tmp`, `/var/tmp`, `/home/sandbox` and exactly 3 entries, no tmpfs at the row's workspace, every mount of type bind.
- R6's kairon-binary checks, guarded by `if kaironBinaryMounted {...}` instead of `t.Skip` (a skip would also skip the partner's own assertions): exactly one mount at `containerHelperPath`, read-only, with `HostPath` equal to the fake binary; and `path.Dir(containerHelperPath)` is one of the `:`-separated directories of `containerPath`. Assert `containerHelperPath == "/usr/local/bin/kairon"` once, before the rows.
- Carry R6's doc comment onto the kairon-binary assertions.

`mountSummary` is still used by `containment_test.go`; keep it.

**R7: delete `TestMapContainerErrorNamesGivenTimeout` and the `fakeDoneCtx` type.** The partner `TestContainerTimeoutNamesEffectiveTimeout` asserts through `invokeAgent` that the message names the given timeout (`timeout after 1s`, not `5m0s`), and the new `sandbox-limit-timeout` subtest (task 1) asserts the `--resource-limit timeout=` hint. `fakeDoneCtx` and its four methods are used by this one test only, so they go too; remove the `context` import if nothing else in the file needs it (`go vet` will say).

**R8: `TestContainerEnvPathAndEvalDir` into `TestContainerConfigUserEnvAndWorkdir`.** Split the partner into two subtests:

- `default-workspace`: its current body. Add to the `want` list: `KAIRON_EVAL_DIR=/workspace/.eval`, `GH_PROMPT_DISABLED=1`, `GH_NO_UPDATE_NOTIFIER=1`. Add the PATH check using `envValue(cfgC.Env, "PATH")`: present and has prefix `/opt/kairon/bin:`.
- `custom-workspace`: a fresh `testContainerConfig()` with `WorkspaceDir = "/work/space"`; `envValue(containerEnv(cc), "KAIRON_EVAL_DIR") == "/work/space/.eval"`.

Every assertion the partner had (image, user, working dir, the seven env entries, sorted order of `A=1` before `B=2`) is kept. `envValue` is still used by `TestContainerEnvDropsGitHubCredentialsAndHarnessVars`; keep it.

### 3. `prune-container-session-tests` (container_session_test.go)

**R9: `TestContainerSessionPerTurnRequestSetup` into `TestContainerSessionThreeTurnsOneOpenOneClose`.** Both use `sessionEnv`, `&fakeSession{}`, `useFakeSession` and `containerTurnsCase(..., "one", "two", "three")` (case timeout `45s`). In the partner:

- Replace the inline `testContainerConfig()` argument with `cConfig := testContainerConfig()`, and pass `cConfig` to `executeCase`.
- Inside the existing per-turn loop (which already checks `req.Turn != i`), add: `req.Timeout == 45*time.Second`; `req.WorkDir == cConfig.WorkspaceDir`; `req.AgentConfigDir == ""`; `req.ToolTrust != nil`; `req.Prompt == []string{"one","two","three"}[i]`. Add the `time` import (it is already imported in this file).
- Keep every assertion the partner had: one open, three `Run` calls, `Turn == i`, one `Close` with `failed=false`, `TurnOutputs == "R1|R2|R3"`, `ActualOutput == "R3"`.

### 4. `prune-containment-tests` (containment_test.go)

Folds R11 and R13; deletes R10, R12, R14, R15. Order inside the task: R11, R13, then the deletes.

**R11: `TestContainmentKaironBinaryJSON` into `TestContainmentToolTrustSerialisesEmptyArray`.** Keep the partner's current assertions unchanged (non-nil empty `ToolTrust`, `"tool_trust":[]` in the JSON, zero-value `Containment` still emits the `tool_trust` key). Add two subtests `kairon_binary=true` and `kairon_binary=false` (a loop over `[]bool{true, false}` with `t.Run(fmt.Sprintf("kairon_binary=%t", want), ...)`): marshal `Containment{ToolTrust: []string{}, KaironBinary: want}`, assert the JSON contains `"kairon_binary":<want>` (always written, never omitted, including `false`), unmarshal it, assert `KaironBinary == want`. Carry the doc comment about "always written, never omitted". The old-format decode half is not folded (see the decisions above).

**R13: `TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched` into `TestUpdateIncrementalSummaryRecordsModeAndContainment`.** Wrap the partner's two blocks in subtests `container` and `native` with their assertions unchanged. Add `legacy-result-unknown-mode`: a file `l/summary.json` under the shared temp `dir` (create the directory like the other two), `updateIncrementalSummary(file, AgentResult{Agent: "a"}, "g")`, then assert the raw JSON contains neither `"sandbox"` nor `"containment"` (message `result with unknown mode must not invent one`).

**R10: delete `TestContainmentForRecordsKaironBinary`** (and its doc comment). `TestContainmentMatchesEnforcement` asserts `c.KaironBinary == hasKairon` against the real mount list built by `buildContainerMounts`, which is stricter than comparing to the `kaironBinaryMounted` constant.

**R12: delete `TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray`.** The summary marshals the same `Containment` type, whose empty `tool_trust` array is asserted by the partner (after R11 is in place).

**R14: delete `TestLegacyJSONStillUnmarshals`.** No partner; it asserts `json.Unmarshal` leaves absent fields zero.

**R15: delete `TestRunFixturesLoadAndScore`** (and its comment). The partner `TestRunFixturesShapes` in `diff_provenance_test.go` loads the same two fixtures (`legacy-native`, `native-pinned`) and asserts exact totals 6/12 and 7/8. That file is not edited. Confirm `runFixturesDir` is still used (it is, by `copyRunFixture`).

### 5. `prune-coverage-run-tests` (coverage_run_test.go)

**R16: delete `TestCoverageRunCaseEditChangesSuiteNotPrompt`** (with its doc comment). `TestSuiteSHA256HashesRawCaseBytes` (`metadata_test.go`, not edited) proves directly that a raw-byte edit to a case file changes the suite hash and that restoring the bytes restores it. Confirm it passes first.

**R17: `TestCoverageResumeUnchangedSuiteContinues` into `TestCoverageResumeChangedSuiteRefused`.** One shared expensive run (`newCandidateE2EEnv`, `runE2E`, `markInterrupted`), two sequential subtests:

- `edited-case-refused`: the partner's current body from reading `before` onwards, unchanged. Keep `data` (the original case bytes) in scope for the next subtest. All current assertions stay: refusal with `cannot resume`, `eval suite changed`, the agent name, both hashes; the saved result is byte-identical afterwards.
- `reverted-case-resumes`: write `data` back to the case file (`os.WriteFile(caseFile, data, 0o644)`), call `resumeE2E(env.evalsDir)` and require no error (`resume with an unchanged suite was refused`). Re-read the agent file with `readSelfTestResult`, assert `after.SuiteSHA256 == run.agent.SuiteSHA256` and `Complete` is recorded and true. A refused resume returns before `.progress` is removed, so the interrupted marker is still there for this resume (see `runProgressiveEvaluation`, the progress file is removed only on completion).

Remove the deleted test's helper usage only if it becomes unused (`markInterrupted`, `resumeE2E` stay in use).

### 6. `merge-coverage-stamp-cases` (coverage_test.go)

**R18, R19, R20: into `TestCoverageStampFullRun`.** Convert it to a table of `stampCoverage` calls, one subtest per row, with `t.Cleanup(resetConfig)` kept. Each row sets `cfg = runConfig{suites: ..., caseFilter: ...}`, builds `AgentResult{Agent: "selftest", Cases: covResults(...)}`, calls `stampCoverage(&r, cases)`, and checks the stamped fields. Rows and their required assertions:

| Subtest | cfg | Result cases | Cases passed in | Assertions |
|---------|-----|--------------|-----------------|------------|
| `full-run` | `suites{selftest: testSuiteHash}` | `a:[1]` | `covCases("a")` | `SuiteSHA256 == testSuiteHash`; `Complete` recorded and true; `CaseFilter` recorded and `""` (the partner's three current assertions, same messages) |
| `uncovered-case-is-incomplete` (R18) | same | `a:[1]` | `covCases("a","b")` | `Complete` recorded and false (`Complete = true with case b uncovered, want false`) |
| `case-filter-is-never-complete` (R19) | `suites{selftest: testSuiteHash}`, `caseFilter: "only"` | `only:[1]` | `covCases("only")` | `Complete` recorded and false; `CaseFilter` recorded and `"only"`. Keep the comment that a filtered run is not a full run even when its one case is covered |
| `no-suite-entry-nil-map` (R20) | `suites: nil`, `caseFilter: "x"` | `a:[1]` | `covCases("a")` | `SuiteSHA256 == ""`, `Complete == nil`, `CaseFilter == nil` |
| `no-suite-entry-other-agent` (R20) | `suites{someone-else: testSuiteHash}`, `caseFilter: "x"` | same | same | same as above |
| `no-suite-entry-empty-map` (R20) | `suites: {}`, `caseFilter: "x"` | same | same | same as above |

The three `no-suite-entry-*` rows keep the three inputs the old `NoOp` test iterated (`nil map`, `other agent`, `empty entry?`). Rows may assert more than the originals only where it holds when run. Delete the three original functions (`TestCoverageStampIncompleteRun`, `TestCoverageStampCaseFilterIsNeverComplete`, `TestCoverageStampNoOpWithoutSuiteEntry`).

### 7. `merge-coverage-configure-case` (coverage_test.go, execute_case_test.go)

**R21: `TestCoverageConfigureResetsRunScopedFields` into `TestConfigureKeepWorkspaces`** (in `execute_case_test.go`). Convert the partner to a table over the run-scoped `cfg` fields. Each row has `name`, an optional `dirty func()` that sets the field to a non-zero value before the call, `opts RunOptions`, an optional `applied func() bool`, and `cleared func() bool`. Per row (inside `t.Run`, with `chdirTemp(t)` and `t.Cleanup(resetConfig)`): call `dirty` if set; `configure(opts)` must return nil; if `applied` is set it must be true (`... not applied`); then `configure(RunOptions{})` must return nil and `cleared` must be true (`... leaked into the next configure`). Rows:

- `keepWorkspaces`: `opts.KeepWorkspaces = true`; `applied`: `cfg.keepWorkspaces`; `cleared`: `!cfg.keepWorkspaces`. These are the partner's two current assertions, same messages.
- `debug`: `opts.Debug = true`; `applied`: `cfg.debug`; `cleared`: `!cfg.debug`.
- `suites`: `dirty` sets `cfg.suites = map[string]string{"a": testSuiteHash}`; `cleared`: `cfg.suites == nil`.
- `caseFilter`: `dirty` sets `cfg.caseFilter = "x"`; `cleared`: `cfg.caseFilter == ""`.

The deleted test called `configure(RunOptions{Backend: inference.NameStub})`; `configure(RunOptions{})` resets the same fields, so the stub backend is not needed. If a builder finds otherwise, use the stub backend for the `suites` and `caseFilter` rows and record why. Delete `TestCoverageConfigureResetsRunScopedFields`.

The "only that function is edited" criterion is checked against the merge base, so it holds whether or not the task is committed. The check removes `TestConfigureKeepWorkspaces` (and the comment lines directly above it) from both `git show <merge-base>:internal/eval/execute_case_test.go` and the working file and requires the two remainders to be identical, so an edit to imports or any other function fails. It also requires the working file to still define `TestConfigureKeepWorkspaces`. No import change is expected: the table needs only `testSuiteHash`, `cfg` and the helpers already in the package. If the builder finds an import change is unavoidable, that is a deviation to record, and it must say why in the sentinel.

### 8. `prune-coverage-summary-tests` (coverage_test.go)

**R22: delete `TestCoverageSummaryJSON`.** `TestCoverageUpdateIncrementalSummaryCopiesFields` asserts the same explicit `"complete": false`, `"case_filter": ""` and `"suite_sha256"` keys through the real summary writer.

**R23: `TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded` into `TestCoverageUpdateIncrementalSummaryCopiesFields`.** Make it a two-row table, one `t.TempDir()` summary file per row:

- `recorded-coverage-copied`: the partner's current body unchanged (the `AgentResult` with `SuiteSHA256`, `Complete: false`, `CaseFilter: ""` and no `PromptSHA256`; the three raw-text key checks; the `Summary` readback of `agents.a` with all its field checks). Keep the comment that `SuiteSHA256` alone must write the entry.
- `unrecorded-result-writes-no-entry`: `AgentResult{Agent: "a"}`, `updateIncrementalSummary` returns nil, `readJSON` into a `Summary`, `len(s.Agents) == 0` (`want none for an unpinned, unrecorded result`).

### 9. `delete-coverage-evaluate-tests` (coverage_test.go)

Depends on task 6 (and tasks 7 and 8 for file order).

**R24: delete `TestCoverageEvaluateProgressiveCaseFilterIncomplete`.** The `case-filter-is-never-complete` row of `TestCoverageStampFullRun` pins the logic. Wiring through `evaluateProgressive` is pinned by `TestCoverageEvaluateProgressiveStampsEverySave`, and `TestCoverageEvaluateStampsSingleCaseRun` also covers `evaluate` with a case filter. Run all three before deleting.

**R25: delete `TestCoverageEvaluateUnpinnedLeavesFieldsUnset`.** The three `no-suite-entry-*` rows of `TestCoverageStampFullRun` pin the no-op rule.

After the task, confirm `pinRealSuite`, `evalProgressiveOK`, `covCase`, `useStubBackend` and `installFakeKiroCLI` are still used by the remaining tests (they are), and `go vet` is clean.

### 10. `prune-criterion-floor-tests` (criterion_floor_test.go)

**R26: `TestAgentVerdictFloorNamesSeveralCriteriaInOrder` into `TestAgentVerdictFloorBoundary`.** Change the table's `score, max int` pair into `scores []CriterionScore` and have each existing row build `[]CriterionScore{cs("x", score, max)}` with the same values:

- `exactly at the floor passes`: 1/2, floor 50, passes, nil below.
- `49 of 100 fails a floor of 50`: 49/100, fails, `[x]`.
- `50 of 100 passes`: 50/100, passes, nil below.
- `fractional floor, exact`: 19/20, floor 95, passes, nil below.

Add a row `several-criteria-named-in-criteria-order`: floor 50, `scores = cs("a", 1, 10), cs("ok", 5, 5), cs("b", 0, 1)`, `wantPassed = false`, `wantBelow = []string{"a", "b"}`. The subtest body (`floorResult(floatPtr(tt.floor), 1, floorCase("c", 0, tt.scores...))`, `Passed` check, `CriteriaBelowFloor` check with `reflect.DeepEqual`) is unchanged. Keep the `(criteria order)` hint in the error message. Delete the original function.

### 11. `prune-diff-filter-tests` (diff_filter_test.go)

**R27: `TestDiffFilterQualityPerDollarUsesFilteredCost` into `TestDiffFilterReportsOnlyMatchingCases`.** Both call `runDiffFiltered(t, "r1", "r2", "speed==fast")` on `writeFilterRuns(t, metaFast, metaSlow)`. Add `"  r1: 50.00 quality/$\n"` and `"  r2: 50.00 quality/$\n"` to the partner's existing `requireContains` list, with the comment explaining `0.500 / 0.01 = 50.00` and `1.000 / 0.02 = 50.00`.

**R29: `TestDiffCasesBlockOnlyListsMatchingCasesWithFilter` into `TestDiffFilterReportsOnlyMatchingCases`.** Same invocation. Add `"  Cases:\n"` and `"    fast-1 [skill=cli, speed=fast]  0.500 → 1.000\n"` to the same `requireContains`. The `slow-1` absence is already in the partner's `requireAbsent`; do not repeat it.

**R28: `TestDiffFilterNoMatchOnEitherSideOmitsAgent` into `TestDiffFilterBuiltInAgentAndNameKeys`.** Keep the partner's `byName` and `byAgent` blocks unchanged. Replace the `otherAgent` block by two subtests over the no-match filters:

- `no-match-agent-key`: `agent==zz`. Keep the existing `requireAbsent(otherAgent, "a1:")`, and add `requireContains(out, "Filter: agent==zz\n", "Cost Delta:", "  Tokens: +0\n")` and `requireAbsent(out, "acc")`.
- `no-match-metadata-key`: `speed==nothing`, which is the deleted test's own input (a metadata key, not a built-in one). It asserts `requireContains(out, "Filter: speed==nothing\n", "Cost Delta:", "  Tokens: +0\n")` and `requireAbsent(out, "a1:", "acc")`.

The issue says to add the Filter/Cost Delta lines to the `agent==zz` check. The second subtest additionally keeps the deleted test's metadata-key input so no input is lost. If `Filter:`, `Cost Delta:` or `Tokens: +0` is not printed for `agent==zz`, the builder records a deviation and keeps those lines on the `speed==nothing` subtest only.

### 12. `prune-diff-provenance-tests` (diff_provenance_test.go)

Order inside the task: R33 first, then the deletes.

**R33: `TestDiffLegacyFixtureVsRecordedNative` into `TestDiffLegacyFixtureVsContainer`.** Convert the partner to a table of two subtests, `container` and `native`, both starting with `chdirTemp(t)` and `copyRunFixture(t, "legacy-native", "legacy")`:

- `container`: the partner's current body unchanged (container summary with `trustContainment("read")` for `architect`; the four `want` strings including `architect containment.record: none →`; no `not directly comparable` and no `⚠`).
- `native`: `writeProvRun(t, "native", provRun{summary: provSummary("native", map[string]float64{"architect": 0.8})})`; `runDiff(t, "legacy", "native")`; output contains `legacy: mode unknown` (`modeUnknown`) and `native: mode native`; output does not contain `not directly comparable`. These are the deleted test's assertions; do not add the container row's extra assertions to it unless they hold.

Carry both doc comments (legacy vs container; legacy vs a run that recorded its mode).

**R30: delete `TestContainmentDiffKaironBinary`.** `TestDiffListsKaironBinaryDifference` asserts the same `a1 containment.kairon_binary: false → true` line end to end. The identical-containment case is covered by `TestDiffSameModeNoWarning`.

**R31: delete `TestDescribeContainmentIncludesKaironBinary`.** `TestDiffContainmentNoneVsPresent` asserts the full `describeContainment` string (`tool_trust=[] fake_gh=true read_only_fs=true kairon_binary=true network=unrestricted`) through diff output.

**R32: delete `TestDiffLegacyVsContainerDoesNotFail`.** `TestDiffLegacyFixtureVsContainer` makes the same legacy-vs-container assertions using the real legacy fixture. Run it first, including the `container` subtest from R33.

If `reflect` becomes unused in the file, remove the import (`go vet` will say).

### 13. `prune-exit-status-tests` (exit_status_test.go)

**R34: delete `TestRunSelftestPassesAndPrintsOneVerdictLine`.** The line format is pinned by `TestPrintAgentVerdictFormat`. One line per agent is pinned by `TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent`. Every caller of `runE2E` already fails if selftest does not pass. Run those partners before deleting. `verdictLines`, `runStubCapturing` and `copySelfTestEvals` stay in use.

### 14. `prune-filter-run-tests` (filter_run_test.go)

**R35: `TestCaseAndFilterRunRecordsCombinedLabel` into `TestRunCaseAndFilterCombineWithAND`.** In the existing `both match` subtest (it already runs `RunWithOptions("alpha", "a1", env.opts(RunOptions{Filters: []string{"speed==fast"}}))` and reads `res`), keep the `caseNames == ["a1"]` assertion and add: `res.CaseFilter` is recorded and equals `case=a1 AND speed==fast`, and `Complete` is recorded and false (`complete = true for a filtered --case run`). The `filter excludes the case` subtest is unchanged.

**R36: `TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing` into `TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites`.** Convert the partner to a table over the two agent orderings, two subtests:

| Subtest | Agents (`newFilterEnv`) | Saved by `speed==fast` | Newly matching on `speed==slow` |
|---------|-------------------------|------------------------|---------------------------------|
| `saved-agent-sorts-last` (the partner today) | `aaa`: `c1.yaml` slow; `zzz`: `c2.yaml` fast | `zzz` | `aaa` |
| `saved-agent-sorts-first` (the deleted test) | `alpha`: `a1.yaml` fast; `beta`: `b1.yaml` slow | `alpha` | `beta` |

Each row runs the initial `speed==fast` run, `markInterrupted`, reads the saved agent's file before, confirms the newly matching agent's file does not exist, resumes with `speed==slow` and `Resume: true`, and asserts: the resume is refused; the error contains `cannot resume`, `--filter changed`, the saved agent's name, `recorded case_filter="speed==fast"` and `now "speed==slow"`; the newly matching agent's file was not written; the saved agent's file is byte-identical. Parametrise the file names and agent names; keep every message. Carry both doc comments, one on each row (they explain the two regressions: the per-agent `continue` before `checkResumeIntegrity`, and the sort-order variant where an earlier agent would write before the changed filter is rejected).

### 15. `gate-coverage-and-task-test`

Runs after all pruning tasks. Checks, in this order:

1. The AC1 grep (14 names) and the AC2 grep (22 names) both print nothing.
2. The sorted top-level test names after the change versus `.kairon/artifacts/issue-424-tests-before.txt`: nothing is new, and exactly the 36 removed names are gone (this is the mechanical form of "every test not listed passes unchanged").
3. `go test -count=1 -cover ./internal/eval` reports at least 87.6%, and the unrounded statement ratio from a fresh coverprofile is not lower than the value in `.kairon/artifacts/issue-424-baseline-coverage.txt`.
4. `task test` exits 0.
5. `gofmt -l internal/eval` prints nothing, `go vet ./internal/eval` passes, and nothing outside `*_test.go`, `.kairon/` and `.kiro/` differs from the merge base (tracked changes via `git diff --name-only <merge-base>`, untracked files via `git ls-files --others --exclude-standard`). No commit on the branch touches `.kiro/`.

If coverage fell, find the removed or merged test that carried a statement block nothing else covers, and restore that input as a table row in the relevant partner (test-only; never restore a deleted function, never remove an assertion). If a block cannot be covered by any partner row, stop with `**Status**: BLOCKED` and name the block. Make no other change when everything passes.

### 16. `validate-prune`

Read-only verification against the issue, independent of the builders. It checks all of the gate's items, then the per-row evidence: each of the 14 deletes is gone, each of the 22 merges is gone as a function and appears as the prescribed subtest or assertion in its partner, and every partner passes with `-v`. It compares each merged partner with `git show <merge-base>:internal/eval/<file>` and confirms that no original assertion was removed or weakened. It also reports any deferral wording or unrecorded deviation.

## Validation Commands

```bash
# AC1: the 14 deleted tests are gone (prints nothing)
grep -nE 'func (TestContainerHelperReportedTimeout|TestContainmentDiffKaironBinary|TestContainmentForRecordsKaironBinary|TestCoverageEvaluateProgressiveCaseFilterIncomplete|TestCoverageEvaluateUnpinnedLeavesFieldsUnset|TestCoverageRunCaseEditChangesSuiteNotPrompt|TestCoverageSummaryJSON|TestDescribeContainmentIncludesKaironBinary|TestDiffLegacyVsContainerDoesNotFail|TestLegacyJSONStillUnmarshals|TestMapContainerErrorNamesGivenTimeout|TestRunFixturesLoadAndScore|TestRunSelftestPassesAndPrintsOneVerdictLine|TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray)\(' internal/eval/*_test.go

# AC2: the 22 merged tests are gone as separate functions (prints nothing)
grep -nE 'func (TestAgentVerdictFloorNamesSeveralCriteriaInOrder|TestBuildContainerMountsHelper|TestBuildContainerMountsKaironBinaryOnPathForEveryBackend|TestCaseAndFilterRunRecordsCombinedLabel|TestContainerEnvPathAndEvalDir|TestContainerHelperPromptOnStdinNotArgv|TestContainerKiroCLIPromptOnStdinNotArgv|TestContainerSessionPerTurnRequestSetup|TestContainerTimeoutMapping|TestContainmentKaironBinaryJSON|TestCoverageConfigureResetsRunScopedFields|TestCoverageResumeUnchangedSuiteContinues|TestCoverageStampCaseFilterIsNeverComplete|TestCoverageStampIncompleteRun|TestCoverageStampNoOpWithoutSuiteEntry|TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded|TestDiffCasesBlockOnlyListsMatchingCasesWithFilter|TestDiffFilterNoMatchOnEitherSideOmitsAgent|TestDiffFilterQualityPerDollarUsesFilteredCost|TestDiffLegacyFixtureVsRecordedNative|TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing|TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched)\(' internal/eval/*_test.go

# AC3 and AC4
go test -count=1 -cover ./internal/eval
task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: capture-baseline
    agent: builder
    description: >-
      Before any edit, record the unmodified package's state for the later gate. Write the sorted top-level test names to
      .kairon/artifacts/issue-424-tests-before.txt, run go test -count=1 -coverprofile=.kairon/artifacts/issue-424-baseline.out ./internal/eval,
      and write the unrounded statement ratio (sum of statements with a count above zero, divided by all statements, four decimals) to
      .kairon/artifacts/issue-424-baseline-coverage.txt. Confirm the package passes and go test -cover prints 87.6%. No source change and no commit
      (.kairon/artifacts is git-ignored).
    dependencies: []
    acceptance_criteria:
      - "issue-424-tests-before.txt lists every top-level Test function of internal/eval, sorted, and contains all 36 names from the issue table"
      - "issue-424-baseline-coverage.txt holds one number with four decimals that rounds to 87.6"
      - "go test -count=1 -cover ./internal/eval passes and reports 87.6% of statements"
      - "No tracked file differs from the merge base (outside .kiro/ and .kairon/specs/) and no untracked file exists outside them"
    validation_commands:
      - |-
        test "$(wc -l < .kairon/artifacts/issue-424-tests-before.txt)" -gt 100
      - |-
        test "$(grep -cE '^(TestAgentVerdictFloorNamesSeveralCriteriaInOrder|TestBuildContainerMountsHelper|TestBuildContainerMountsKaironBinaryOnPathForEveryBackend|TestCaseAndFilterRunRecordsCombinedLabel|TestContainerEnvPathAndEvalDir|TestContainerHelperPromptOnStdinNotArgv|TestContainerKiroCLIPromptOnStdinNotArgv|TestContainerSessionPerTurnRequestSetup|TestContainerTimeoutMapping|TestContainmentKaironBinaryJSON|TestCoverageConfigureResetsRunScopedFields|TestCoverageResumeUnchangedSuiteContinues|TestCoverageStampCaseFilterIsNeverComplete|TestCoverageStampIncompleteRun|TestCoverageStampNoOpWithoutSuiteEntry|TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded|TestDiffCasesBlockOnlyListsMatchingCasesWithFilter|TestDiffFilterNoMatchOnEitherSideOmitsAgent|TestDiffFilterQualityPerDollarUsesFilteredCost|TestDiffLegacyFixtureVsRecordedNative|TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing|TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched|TestContainerHelperReportedTimeout|TestContainmentDiffKaironBinary|TestContainmentForRecordsKaironBinary|TestCoverageEvaluateProgressiveCaseFilterIncomplete|TestCoverageEvaluateUnpinnedLeavesFieldsUnset|TestCoverageRunCaseEditChangesSuiteNotPrompt|TestCoverageSummaryJSON|TestDescribeContainmentIncludesKaironBinary|TestDiffLegacyVsContainerDoesNotFail|TestLegacyJSONStillUnmarshals|TestMapContainerErrorNamesGivenTimeout|TestRunFixturesLoadAndScore|TestRunSelftestPassesAndPrintsOneVerdictLine|TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray)$' .kairon/artifacts/issue-424-tests-before.txt)" -eq 36
      - |-
        awk '{v=$1+0; if (v >= 87.55 && v < 87.65) ok=1} END {exit ok ? 0 : 1}' .kairon/artifacts/issue-424-baseline-coverage.txt
      - |-
        base="$(git merge-base HEAD origin/main)" && test -z "$(git diff --name-only "$base" -- . ':!.kiro' ':!.kairon/specs' ':!.kairon/artifacts'; git ls-files --others --exclude-standard -- . ':!.kiro' ':!.kairon/specs' ':!.kairon/artifacts')"

  - id: prune-container-agent-tests
    agent: builder
    description: >-
      container_agent_test.go and its partners in container_runner_test.go. R1 and R2: split TestContainerExecsAreWrappedAndPromptStaysOnStdin
      into subtests kiro-cli-exec and helper-exec and fold the exact-stdin, ANSI, cost/record, helper nasty-prompt and exact helper command-line
      checks of TestContainerKiroCLIPromptOnStdinNotArgv and TestContainerHelperPromptOnStdinNotArgv into them, keeping every assertion the partner
      had. R3: fold TestContainerTimeoutMapping into TestContainerTimeoutNamesEffectiveTimeout as a third subtest sandbox-limit-timeout with all of
      its assertions, including the --resource-limit hint. R4: delete TestContainerHelperReportedTimeout after confirming the partner's
      helper-reported timeout subtest asserts the same and more. Delete the three merged functions. Remove imports left unused. Commit as
      test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["capture-baseline"]
    acceptance_criteria:
      - "TestContainerKiroCLIPromptOnStdinNotArgv, TestContainerHelperPromptOnStdinNotArgv, TestContainerTimeoutMapping and TestContainerHelperReportedTimeout no longer exist"
      - "TestContainerExecsAreWrappedAndPromptStaysOnStdin passes with subtests kiro-cli-exec and helper-exec"
      - "kiro-cli-exec keeps the open-umask wrapper, kiro-cli program and Contains(PROMPT-MARKER) assertions and adds out == hello, exact stdin equals the prompt, assertNoPromptInArgv over x.cmds, ec == nil, cost UsageSource/Model/TokensIn and rec Model/Estimated/Role checks"
      - "helper-exec uses nastyPrompt with response scripted, keeps the unwrapOpenUmask check, and adds out == scripted, no prompt in argv, prompt on stdin, and the exact command line /usr/local/bin/kairon inference-exec --backend stub"
      - "TestContainerTimeoutNamesEffectiveTimeout passes with its two existing subtests plus sandbox-limit-timeout, which asserts ErrTimeout, Container execution timeout after 30ms, --resource-limit timeout=, ec.Stderr prefix, ec.Platform linux/arm64, ec.WorkingDir /workspace and rec.Error non-empty"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestContainerKiroCLIPromptOnStdinNotArgv|TestContainerHelperPromptOnStdinNotArgv|TestContainerTimeoutMapping|TestContainerHelperReportedTimeout)\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestContainerExecsAreWrappedAndPromptStaysOnStdin$' -v ./internal/eval | grep -cE -- '--- PASS: TestContainerExecsAreWrappedAndPromptStaysOnStdin/(kiro-cli-exec|helper-exec) \(')" -eq 2
      - |-
        test "$(go test -count=1 -run '^TestContainerTimeoutNamesEffectiveTimeout$' -v ./internal/eval | grep -cE -- '--- PASS: TestContainerTimeoutNamesEffectiveTimeout/(kiro-cli_host_deadline|helper-reported_timeout|sandbox-limit-timeout) \(')" -eq 3
      - |-
        awk '/^func TestContainerExecsAreWrappedAndPromptStaysOnStdin\(/,/^}/' internal/eval/container_runner_test.go | grep -qF 'inference-exec --backend stub'
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-container-runner-tests
    agent: builder
    description: >-
      container_runner_test.go. R5 and R6: turn TestBuildContainerMountsKiroCLI into a table with rows kiro-cli-default-workspace,
      stub-default-workspace and stub-custom-workspace; each row asserts len 5, DeepEqual on the full ordered mount list derived from the row's
      workspace, the /opt/kairon/kairon absence, the existing NewHostConfigWithMounts block, and (guarded by kaironBinaryMounted rather than
      t.Skip) exactly one read-only kairon mount at containerHelperPath with the fake binary as HostPath and its directory on containerPath;
      assert containerHelperPath once. R7: delete TestMapContainerErrorNamesGivenTimeout and the fakeDoneCtx type it alone uses, after
      confirming the sandbox-limit-timeout subtest from the previous task asserts the --resource-limit hint. R8: split
      TestContainerConfigUserEnvAndWorkdir into subtests default-workspace (existing assertions plus KAIRON_EVAL_DIR=/workspace/.eval,
      GH_PROMPT_DISABLED=1, GH_NO_UPDATE_NOTIFIER=1 and the PATH prefix /opt/kairon/bin:) and custom-workspace (KAIRON_EVAL_DIR under
      /work/space). Delete TestBuildContainerMountsHelper, TestBuildContainerMountsKaironBinaryOnPathForEveryBackend and
      TestContainerEnvPathAndEvalDir. Remove unused imports. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-container-agent-tests"]
    acceptance_criteria:
      - "TestBuildContainerMountsHelper, TestBuildContainerMountsKaironBinaryOnPathForEveryBackend, TestMapContainerErrorNamesGivenTimeout and TestContainerEnvPathAndEvalDir no longer exist, and fakeDoneCtx is gone"
      - "TestBuildContainerMountsKiroCLI passes with subtests kiro-cli-default-workspace, stub-default-workspace and stub-custom-workspace, each keeping the partner's original DeepEqual and host-config assertions"
      - "The kairon-binary assertions (one mount, read-only, HostPath equals the fake binary, directory on containerPath, containerHelperPath equals /usr/local/bin/kairon) run for every row when kaironBinaryMounted is true"
      - "TestContainerConfigUserEnvAndWorkdir passes with subtests default-workspace and custom-workspace and keeps image, user, working dir, the original seven env entries and the A before B order checks"
      - "TestContainerTimeoutNamesEffectiveTimeout still passes with its three subtests"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestBuildContainerMountsHelper|TestBuildContainerMountsKaironBinaryOnPathForEveryBackend|TestMapContainerErrorNamesGivenTimeout|TestContainerEnvPathAndEvalDir)\(' internal/eval/*_test.go)"
      - |-
        test -z "$(grep -n 'fakeDoneCtx' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestBuildContainerMountsKiroCLI$' -v ./internal/eval | grep -cE -- '--- PASS: TestBuildContainerMountsKiroCLI/(kiro-cli-default-workspace|stub-default-workspace|stub-custom-workspace) \(')" -eq 3
      - |-
        test "$(go test -count=1 -run '^TestContainerConfigUserEnvAndWorkdir$' -v ./internal/eval | grep -cE -- '--- PASS: TestContainerConfigUserEnvAndWorkdir/(default-workspace|custom-workspace) \(')" -eq 2
      - go test -count=1 -run '^(TestContainerTimeoutNamesEffectiveTimeout|TestContainerEnvDropsGitHubCredentialsAndHarnessVars|TestContainmentMatchesEnforcement)$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-container-session-tests
    agent: builder
    description: >-
      container_session_test.go. R9: fold TestContainerSessionPerTurnRequestSetup into TestContainerSessionThreeTurnsOneOpenOneClose. Keep
      cConfig := testContainerConfig() in a variable and pass it to executeCase. In the existing per-turn loop add the checks Timeout == 45s,
      WorkDir == cConfig.WorkspaceDir, AgentConfigDir == empty, ToolTrust != nil and Prompt == one/two/three by index, next to the existing Turn
      check. Keep every other partner assertion. Delete the merged function. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-container-runner-tests"]
    acceptance_criteria:
      - "TestContainerSessionPerTurnRequestSetup no longer exists"
      - "TestContainerSessionThreeTurnsOneOpenOneClose passes and asserts, for each of the three Run requests, Turn == i, Timeout == 45s, WorkDir == the container config workspace, AgentConfigDir empty, ToolTrust non-nil and the prompt for that turn"
      - "The partner still asserts one open, three Run calls, one Close with failed=false, TurnOutputs R1|R2|R3 and ActualOutput R3"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestContainerSessionPerTurnRequestSetup\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^TestContainerSessionThreeTurnsOneOpenOneClose$' -v ./internal/eval
      - |-
        body="$(awk '/^func TestContainerSessionThreeTurnsOneOpenOneClose\(/,/^}/' internal/eval/container_session_test.go)"; for s in '45*time.Second' 'cConfig.WorkspaceDir' 'AgentConfigDir' 'ToolTrust' 'req.Prompt' 'req.Turn'; do printf '%s' "$body" | grep -qF -- "$s" || { echo "missing $s"; exit 1; }; done
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-containment-tests
    agent: builder
    description: >-
      containment_test.go. Do R11 first: fold TestContainmentKaironBinaryJSON into TestContainmentToolTrustSerialisesEmptyArray as subtests
      kairon_binary=true and kairon_binary=false (marshal Containment with that KaironBinary, key kairon_binary always written, round trip), keeping
      every existing partner assertion; the old-format decode half is not folded (the issue calls it Go zero-value behavior). Then R13: wrap
      TestUpdateIncrementalSummaryRecordsModeAndContainment's two blocks in subtests container and native unchanged and add
      legacy-result-unknown-mode (AgentResult with only Agent set writes neither sandbox nor containment). Then delete R10
      TestContainmentForRecordsKaironBinary, R12 TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray, R14 TestLegacyJSONStillUnmarshals and
      R15 TestRunFixturesLoadAndScore (partner TestRunFixturesShapes in diff_provenance_test.go, not edited; run it first), and the two merged
      functions. Remove unused imports. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-container-session-tests"]
    acceptance_criteria:
      - "TestContainmentForRecordsKaironBinary, TestContainmentKaironBinaryJSON, TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray, TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched, TestLegacyJSONStillUnmarshals and TestRunFixturesLoadAndScore no longer exist"
      - "TestContainmentToolTrustSerialisesEmptyArray passes with subtests kairon_binary=true and kairon_binary=false, and still asserts non-nil empty ToolTrust, tool_trust:[] in the JSON and the tool_trust key on a zero Containment"
      - "TestUpdateIncrementalSummaryRecordsModeAndContainment passes with subtests container, native and legacy-result-unknown-mode, each keeping its original assertions"
      - "TestContainmentMatchesEnforcement and TestRunFixturesShapes pass unchanged"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestContainmentForRecordsKaironBinary|TestContainmentKaironBinaryJSON|TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray|TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched|TestLegacyJSONStillUnmarshals|TestRunFixturesLoadAndScore)\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestContainmentToolTrustSerialisesEmptyArray$' -v ./internal/eval | grep -cE -- '--- PASS: TestContainmentToolTrustSerialisesEmptyArray/kairon_binary=(true|false) \(')" -eq 2
      - |-
        test "$(go test -count=1 -run '^TestUpdateIncrementalSummaryRecordsModeAndContainment$' -v ./internal/eval | grep -cE -- '--- PASS: TestUpdateIncrementalSummaryRecordsModeAndContainment/(container|native|legacy-result-unknown-mode) \(')" -eq 3
      - go test -count=1 -run '^(TestContainmentMatchesEnforcement|TestRunFixturesShapes|TestUpdateIncrementalSummaryRefusesModeChange|TestUpdateIncrementalSummaryNativeRemovesStaleContainment)$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-coverage-run-tests
    agent: builder
    description: >-
      coverage_run_test.go. R16: delete TestCoverageRunCaseEditChangesSuiteNotPrompt (with its doc comment) after confirming
      TestSuiteSHA256HashesRawCaseBytes in metadata_test.go passes; that file is not edited. R17: fold TestCoverageResumeUnchangedSuiteContinues
      into TestCoverageResumeChangedSuiteRefused using the one shared run, as two sequential subtests. edited-case-refused holds the partner's
      current assertions unchanged (refusal naming the agent and both hashes, saved result byte-identical) and keeps the original case bytes in
      scope. reverted-case-resumes writes the original bytes back, resumes with resumeE2E and requires success, then asserts the recorded
      suite_sha256 equals the first run's and Complete is recorded and true. Delete the merged function and its doc comment. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-containment-tests"]
    acceptance_criteria:
      - "TestCoverageRunCaseEditChangesSuiteNotPrompt and TestCoverageResumeUnchangedSuiteContinues no longer exist"
      - "TestCoverageResumeChangedSuiteRefused passes with subtests edited-case-refused and reverted-case-resumes, and runs the e2e suite once (one runE2E call in the function)"
      - "edited-case-refused keeps every original assertion: refusal text contains cannot resume, eval suite changed, the agent name, the recorded hash and the current hash, and the saved result file is unchanged"
      - "reverted-case-resumes asserts no error from the resume, suite_sha256 after the resume equals the first run's, and Complete is true"
      - "TestSuiteSHA256HashesRawCaseBytes passes unchanged"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCoverageRunCaseEditChangesSuiteNotPrompt|TestCoverageResumeUnchangedSuiteContinues)\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestCoverageResumeChangedSuiteRefused$' -v ./internal/eval | grep -cE -- '--- PASS: TestCoverageResumeChangedSuiteRefused/(edited-case-refused|reverted-case-resumes) \(')" -eq 2
      - |-
        test "$(awk '/^func TestCoverageResumeChangedSuiteRefused\(/,/^}/' internal/eval/coverage_run_test.go | grep -c 'runE2E(')" -eq 1
      - go test -count=1 -run '^(TestSuiteSHA256HashesRawCaseBytes|TestCoverageResumeIntegritySuite)$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: merge-coverage-stamp-cases
    agent: builder
    description: >-
      coverage_test.go. R18, R19, R20: convert TestCoverageStampFullRun to a table of stampCoverage calls with subtests full-run (the partner's
      three current assertions), uncovered-case-is-incomplete (cases a covered, a and b expected, Complete recorded and false),
      case-filter-is-never-complete (caseFilter only, one covered case only, Complete recorded and false, CaseFilter recorded as only) and
      no-suite-entry-nil-map, no-suite-entry-other-agent, no-suite-entry-empty-map (caseFilter x; SuiteSHA256 empty, Complete nil, CaseFilter
      nil). Keep t.Cleanup(resetConfig). Delete TestCoverageStampIncompleteRun, TestCoverageStampCaseFilterIsNeverComplete and
      TestCoverageStampNoOpWithoutSuiteEntry. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-coverage-run-tests"]
    acceptance_criteria:
      - "TestCoverageStampIncompleteRun, TestCoverageStampCaseFilterIsNeverComplete and TestCoverageStampNoOpWithoutSuiteEntry no longer exist"
      - "TestCoverageStampFullRun passes with exactly the six subtests full-run, uncovered-case-is-incomplete, case-filter-is-never-complete, no-suite-entry-nil-map, no-suite-entry-other-agent and no-suite-entry-empty-map"
      - "full-run asserts SuiteSHA256 equals testSuiteHash, Complete recorded true and CaseFilter recorded as the empty string"
      - "case-filter-is-never-complete asserts Complete recorded false and CaseFilter recorded as only, with a filtered run whose one case is covered"
      - "each no-suite-entry row asserts SuiteSHA256 empty, Complete nil and CaseFilter nil with caseFilter x set"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCoverageStampIncompleteRun|TestCoverageStampCaseFilterIsNeverComplete|TestCoverageStampNoOpWithoutSuiteEntry)\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestCoverageStampFullRun$' -v ./internal/eval | grep -cE -- '--- PASS: TestCoverageStampFullRun/(full-run|uncovered-case-is-incomplete|case-filter-is-never-complete|no-suite-entry-nil-map|no-suite-entry-other-agent|no-suite-entry-empty-map) \(')" -eq 6
      - go test -count=1 -run '^(TestCoverageCasesCovered|TestCoverageEvaluateStampsSingleCaseRun)$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: merge-coverage-configure-case
    agent: builder
    description: >-
      coverage_test.go and execute_case_test.go. R21: turn TestConfigureKeepWorkspaces (execute_case_test.go; named as the partner by the issue's
      table, so the one function is edited even though that file is not in the In Scope list) into a table of subtests keepWorkspaces, debug,
      suites and caseFilter. Each row optionally dirties a cfg field, calls configure(opts), checks that an option is applied when the row has one,
      then calls configure(RunOptions{}) and checks the field is cleared, with chdirTemp and t.Cleanup(resetConfig). keepWorkspaces keeps the
      partner's two assertions with their messages. Delete TestCoverageConfigureResetsRunScopedFields from coverage_test.go. Remove imports left
      unused. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["merge-coverage-stamp-cases"]
    acceptance_criteria:
      - "TestCoverageConfigureResetsRunScopedFields no longer exists"
      - "TestConfigureKeepWorkspaces passes with subtests keepWorkspaces, debug, suites and caseFilter"
      - "The keepWorkspaces row asserts the option is applied after configure with KeepWorkspaces true and is cleared by the next configure, with the partner's original messages"
      - "The suites and caseFilter rows pre-set a non-zero value and assert configure leaves suites nil and caseFilter empty"
      - "Only the TestConfigureKeepWorkspaces function (with its doc comment) is edited in execute_case_test.go: with that function removed from both the merge-base version and the working file, the rest of the file is byte-identical (imports unchanged), and the working file still defines TestConfigureKeepWorkspaces"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestCoverageConfigureResetsRunScopedFields\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestConfigureKeepWorkspaces$' -v ./internal/eval | grep -cE -- '--- PASS: TestConfigureKeepWorkspaces/(keepWorkspaces|debug|suites|caseFilter) \(')" -eq 4
      - |-
        base="$(git merge-base HEAD origin/main)" && f=internal/eval/execute_case_test.go && grep -q '^func TestConfigureKeepWorkspaces(' "$f" && strip() { awk 'skip {if (/^}/) skip=0; next} /^\/\// {buf = buf $0 "\n"; next} /^func TestConfigureKeepWorkspaces\(/ {buf=""; skip=1; next} {printf "%s", buf; buf=""; print} END {printf "%s", buf}'; } && test "$(git show "$base:$f" | strip | cksum)" = "$(strip < "$f" | cksum)"
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-coverage-summary-tests
    agent: builder
    description: >-
      coverage_test.go. R22: delete TestCoverageSummaryJSON after confirming TestCoverageUpdateIncrementalSummaryCopiesFields asserts the same
      explicit false and empty keys through the real summary writer. R23: fold TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded into
      TestCoverageUpdateIncrementalSummaryCopiesFields as a two-row table, one temp file per row: recorded-coverage-copied (the partner's current
      body unchanged, including the SuiteSHA256-alone comment) and unrecorded-result-writes-no-entry (AgentResult with only Agent set, then
      len(Summary.Agents) == 0). Delete the merged function. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["merge-coverage-configure-case"]
    acceptance_criteria:
      - "TestCoverageSummaryJSON and TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded no longer exist"
      - "TestCoverageUpdateIncrementalSummaryCopiesFields passes with subtests recorded-coverage-copied and unrecorded-result-writes-no-entry"
      - "recorded-coverage-copied keeps all of the partner's assertions: the three raw-text keys, the agents.a entry and its SuiteSHA256, Complete and CaseFilter fields"
      - "unrecorded-result-writes-no-entry asserts no agents entry for an unpinned, unrecorded result"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCoverageSummaryJSON|TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded)\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestCoverageUpdateIncrementalSummaryCopiesFields$' -v ./internal/eval | grep -cE -- '--- PASS: TestCoverageUpdateIncrementalSummaryCopiesFields/(recorded-coverage-copied|unrecorded-result-writes-no-entry) \(')" -eq 2
      - go test -count=1 -run '^TestCoverageAgentResultJSON$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: delete-coverage-evaluate-tests
    agent: builder
    description: >-
      coverage_test.go. Only after TestCoverageStampFullRun carries the case-filter and no-suite-entry rows. Run the partners first:
      TestCoverageStampFullRun, TestCoverageEvaluateProgressiveStampsEverySave and TestCoverageEvaluateStampsSingleCaseRun. R24: delete
      TestCoverageEvaluateProgressiveCaseFilterIncomplete (partner: the case-filter-is-never-complete row). R25: delete
      TestCoverageEvaluateUnpinnedLeavesFieldsUnset (partner: the three no-suite-entry rows). Confirm the helpers pinRealSuite, evalProgressiveOK,
      covCase and installFakeKiroCLI are still used. Remove imports left unused. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["merge-coverage-stamp-cases", "prune-coverage-summary-tests"]
    acceptance_criteria:
      - "TestCoverageEvaluateProgressiveCaseFilterIncomplete and TestCoverageEvaluateUnpinnedLeavesFieldsUnset no longer exist"
      - "TestCoverageStampFullRun, TestCoverageEvaluateProgressiveStampsEverySave, TestCoverageEvaluateProgressiveRepeat and TestCoverageEvaluateStampsSingleCaseRun pass unchanged"
      - "No helper left without a caller, and gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCoverageEvaluateProgressiveCaseFilterIncomplete|TestCoverageEvaluateUnpinnedLeavesFieldsUnset)\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^(TestCoverageStampFullRun|TestCoverageEvaluateProgressiveStampsEverySave|TestCoverageEvaluateProgressiveRepeat|TestCoverageEvaluateStampsSingleCaseRun)$' ./internal/eval
      - |-
        for h in pinRealSuite evalProgressiveOK covCase installFakeKiroCLI; do test "$(grep -c "$h(" internal/eval/*_test.go | awk -F: '{s+=$2} END {print s}')" -ge 2 || { echo "helper $h has no caller"; exit 1; }; done
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-criterion-floor-tests
    agent: builder
    description: >-
      criterion_floor_test.go. R26: fold TestAgentVerdictFloorNamesSeveralCriteriaInOrder into TestAgentVerdictFloorBoundary. Replace the table's
      score and max pair by a scores slice of CriterionScore (existing rows become cs("x", score, max) with identical values and expectations) and
      add the row several-criteria-named-in-criteria-order: floor 50, scores cs(a,1,10), cs(ok,5,5), cs(b,0,1), wantPassed false, wantBelow
      [a b]. The subtest body keeps both existing checks. Delete the merged function. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["delete-coverage-evaluate-tests"]
    acceptance_criteria:
      - "TestAgentVerdictFloorNamesSeveralCriteriaInOrder no longer exists"
      - "TestAgentVerdictFloorBoundary passes with its four original rows (same names, scores, floors and expectations) plus several-criteria-named-in-criteria-order"
      - "The new row expects Passed false and CriteriaBelowFloor equal to [a b] for scores a 1/10, ok 5/5, b 0/1 under floor 50"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestAgentVerdictFloorNamesSeveralCriteriaInOrder\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestAgentVerdictFloorBoundary$' -v ./internal/eval | grep -cE -- '--- PASS: TestAgentVerdictFloorBoundary/(exactly_at_the_floor_passes|49_of_100_fails_a_floor_of_50|50_of_100_passes|fractional_floor,_exact|several-criteria-named-in-criteria-order) \(')" -eq 5
      - go test -count=1 -run '^TestAgentVerdictFloor' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-diff-filter-tests
    agent: builder
    description: >-
      diff_filter_test.go. R27 and R29: add to the requireContains list of TestDiffFilterReportsOnlyMatchingCases the lines "  r1: 50.00 quality/$\n",
      "  r2: 50.00 quality/$\n", "  Cases:\n" and "    fast-1 [skill=cli, speed=fast]  0.500 → 1.000\n" (the slow-1 absence is already asserted there). R28: in
      TestDiffFilterBuiltInAgentAndNameKeys keep the byName and byAgent blocks and replace the otherAgent block by subtests no-match-agent-key
      (agent==zz: keep the a1: absence, add Filter: agent==zz, Cost Delta:, Tokens: +0 lines present and acc absent) and no-match-metadata-key
      (speed==nothing, the deleted test's own input: Filter: speed==nothing, Cost Delta:, Tokens: +0 present; a1: and acc absent). Delete
      TestDiffFilterQualityPerDollarUsesFilteredCost, TestDiffFilterNoMatchOnEitherSideOmitsAgent and
      TestDiffCasesBlockOnlyListsMatchingCasesWithFilter. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-criterion-floor-tests"]
    acceptance_criteria:
      - "TestDiffFilterQualityPerDollarUsesFilteredCost, TestDiffFilterNoMatchOnEitherSideOmitsAgent and TestDiffCasesBlockOnlyListsMatchingCasesWithFilter no longer exist"
      - "TestDiffFilterReportsOnlyMatchingCases passes and its requireContains list still holds every original line and now also the two quality/$ lines and the two Cases block lines"
      - "TestDiffFilterBuiltInAgentAndNameKeys passes with its byName and byAgent checks unchanged and subtests no-match-agent-key and no-match-metadata-key"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestDiffFilterQualityPerDollarUsesFilteredCost|TestDiffFilterNoMatchOnEitherSideOmitsAgent|TestDiffCasesBlockOnlyListsMatchingCasesWithFilter)\(' internal/eval/*_test.go)"
      - |-
        body="$(awk '/^func TestDiffFilterReportsOnlyMatchingCases\(/,/^}/' internal/eval/diff_filter_test.go)"; for s in 'r1: 50.00 quality/$' 'r2: 50.00 quality/$' '  Cases:' 'fast-1 [skill=cli, speed=fast]  0.500 → 1.000'; do printf '%s' "$body" | grep -qF -- "$s" || { echo "missing $s"; exit 1; }; done
      - |-
        test "$(go test -count=1 -run '^TestDiffFilterBuiltInAgentAndNameKeys$' -v ./internal/eval | grep -cE -- '--- PASS: TestDiffFilterBuiltInAgentAndNameKeys/(no-match-agent-key|no-match-metadata-key) \(')" -eq 2
      - go test -count=1 -run '^TestDiffFilterReportsOnlyMatchingCases$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-diff-provenance-tests
    agent: builder
    description: >-
      diff_provenance_test.go. Do R33 first: convert TestDiffLegacyFixtureVsContainer into two subtests. container is the partner's current body
      unchanged. native is the deleted TestDiffLegacyFixtureVsRecordedNative body (legacy fixture copy, native run from provSummary with
      architect 0.8; output has legacy: mode unknown and native: mode native; no "not directly comparable"). Keep both doc comments. Then delete
      R30 TestContainmentDiffKaironBinary (partner TestDiffListsKaironBinaryDifference), R31 TestDescribeContainmentIncludesKaironBinary (partner
      TestDiffContainmentNoneVsPresent), R32 TestDiffLegacyVsContainerDoesNotFail (partner TestDiffLegacyFixtureVsContainer), and the merged
      TestDiffLegacyFixtureVsRecordedNative. Run each partner before deleting. Remove unused imports. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-diff-filter-tests"]
    acceptance_criteria:
      - "TestContainmentDiffKaironBinary, TestDescribeContainmentIncludesKaironBinary, TestDiffLegacyVsContainerDoesNotFail and TestDiffLegacyFixtureVsRecordedNative no longer exist"
      - "TestDiffLegacyFixtureVsContainer passes with subtests container and native; container keeps its four wanted strings and its no-incomparability and no-warning checks; native asserts both mode lines and the absence of not directly comparable"
      - "TestDiffListsKaironBinaryDifference, TestDiffContainmentNoneVsPresent and TestDiffSameModeNoWarning pass unchanged"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestContainmentDiffKaironBinary|TestDescribeContainmentIncludesKaironBinary|TestDiffLegacyVsContainerDoesNotFail|TestDiffLegacyFixtureVsRecordedNative)\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestDiffLegacyFixtureVsContainer$' -v ./internal/eval | grep -cE -- '--- PASS: TestDiffLegacyFixtureVsContainer/(container|native) \(')" -eq 2
      - go test -count=1 -run '^(TestDiffListsKaironBinaryDifference|TestDiffContainmentNoneVsPresent|TestDiffSameModeNoWarning)$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-exit-status-tests
    agent: builder
    description: >-
      exit_status_test.go. R34: delete TestRunSelftestPassesAndPrintsOneVerdictLine after running its partners TestPrintAgentVerdictFormat and
      TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent, and confirming verdictLines, runStubCapturing and copySelfTestEvals still have callers.
      Remove imports left unused. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-diff-provenance-tests"]
    acceptance_criteria:
      - "TestRunSelftestPassesAndPrintsOneVerdictLine no longer exists"
      - "TestPrintAgentVerdictFormat and TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent pass unchanged"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestRunSelftestPassesAndPrintsOneVerdictLine\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^(TestPrintAgentVerdictFormat|TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent|TestRunSelftestFailReturnsThresholdErrorAfterWritingResults)$' ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: prune-filter-run-tests
    agent: builder
    description: >-
      filter_run_test.go. R35: in the both match subtest of TestRunCaseAndFilterCombineWithAND keep the cases-run check and add that CaseFilter is
      recorded as "case=a1 AND speed==fast" and Complete is recorded and false; delete TestCaseAndFilterRunRecordsCombinedLabel. R36: turn
      TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites into a table of subtests saved-agent-sorts-last (aaa slow c1, zzz fast c2;
      saved zzz, newly matching aaa; the partner today) and saved-agent-sorts-first (alpha fast a1, beta slow b1; saved alpha, newly matching
      beta; the deleted test). Each row runs the initial speed==fast run, markInterrupted, reads the saved file, resumes with speed==slow and
      Resume true, and asserts the refusal text (cannot resume, --filter changed, saved agent name, recorded case_filter="speed==fast", now
      "speed==slow"), that the newly matching agent's file was not written and that the saved file is byte-identical. Carry both doc comments, one
      per row. Delete TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing. Commit as test(eval), staging only the named test files by path (never git add -A).
    dependencies: ["prune-exit-status-tests"]
    acceptance_criteria:
      - "TestCaseAndFilterRunRecordsCombinedLabel and TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing no longer exist"
      - "TestRunCaseAndFilterCombineWithAND passes with its two original subtests and the both match subtest asserts the case=a1 AND speed==fast label and Complete false"
      - "TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites passes with subtests saved-agent-sorts-last and saved-agent-sorts-first, each asserting the refusal text, no write by the newly matching agent and an unchanged saved file"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCaseAndFilterRunRecordsCombinedLabel|TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing)\(' internal/eval/*_test.go)"
      - |-
        test "$(go test -count=1 -run '^TestRunCaseAndFilterCombineWithAND$' -v ./internal/eval | grep -cE -- '--- PASS: TestRunCaseAndFilterCombineWithAND/(both_match|filter_excludes_the_case) \(')" -eq 2
      - |-
        awk '/^func TestRunCaseAndFilterCombineWithAND\(/,/^}/' internal/eval/filter_run_test.go | grep -qF 'case=a1 AND speed==fast'
      - |-
        test "$(go test -count=1 -run '^TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites$' -v ./internal/eval | grep -cE -- '--- PASS: TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites/(saved-agent-sorts-last|saved-agent-sorts-first) \(')" -eq 2
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval

  - id: gate-coverage-and-task-test
    agent: builder
    description: >-
      Run the full gate after all pruning tasks. Confirm the AC1 and AC2 greps print nothing, that the sorted top-level test names versus
      .kairon/artifacts/issue-424-tests-before.txt show no new name and exactly the 36 removed names, that go test -cover ./internal/eval reports at
      least 87.6% and that the unrounded statement ratio from a fresh coverprofile is not lower than .kairon/artifacts/issue-424-baseline-coverage.txt,
      and that task test exits 0. If coverage fell, find the removed or merged test that carried a statement block nothing else covers and restore
      that input as a row in the relevant partner (test-only; never restore a deleted function, never remove an assertion). If a block cannot be
      covered by any partner row, stop and report Status BLOCKED naming the block. Make no other change when everything passes.
    dependencies: ["prune-filter-run-tests"]
    acceptance_criteria:
      - "The AC1 grep over internal/eval/*_test.go prints nothing"
      - "The AC2 grep over internal/eval/*_test.go prints nothing"
      - "Compared with the before list, no top-level test name was added and exactly 36 names (the 14 Delete and 22 Merge tests) were removed"
      - "go test -count=1 -cover ./internal/eval reports at least 87.6% and the unrounded statement ratio is not lower than the baseline value"
      - "task test exits 0 (go test -v -race -coverprofile=coverage.out ./... passes for the whole module)"
      - "gofmt -l internal/eval prints nothing and go vet ./internal/eval passes"
      - "No file other than _test.go files under internal/eval differs from the merge base outside .kairon/ and .kiro/, tracked or untracked, committed or not"
      - "No commit on the branch touches .kiro/ (the uncommitted .kiro/agents/*.json modifications were not staged)"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestContainerHelperReportedTimeout|TestContainmentDiffKaironBinary|TestContainmentForRecordsKaironBinary|TestCoverageEvaluateProgressiveCaseFilterIncomplete|TestCoverageEvaluateUnpinnedLeavesFieldsUnset|TestCoverageRunCaseEditChangesSuiteNotPrompt|TestCoverageSummaryJSON|TestDescribeContainmentIncludesKaironBinary|TestDiffLegacyVsContainerDoesNotFail|TestLegacyJSONStillUnmarshals|TestMapContainerErrorNamesGivenTimeout|TestRunFixturesLoadAndScore|TestRunSelftestPassesAndPrintsOneVerdictLine|TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray)\(' internal/eval/*_test.go)"
      - |-
        test -z "$(grep -nE 'func (TestAgentVerdictFloorNamesSeveralCriteriaInOrder|TestBuildContainerMountsHelper|TestBuildContainerMountsKaironBinaryOnPathForEveryBackend|TestCaseAndFilterRunRecordsCombinedLabel|TestContainerEnvPathAndEvalDir|TestContainerHelperPromptOnStdinNotArgv|TestContainerKiroCLIPromptOnStdinNotArgv|TestContainerSessionPerTurnRequestSetup|TestContainerTimeoutMapping|TestContainmentKaironBinaryJSON|TestCoverageConfigureResetsRunScopedFields|TestCoverageResumeUnchangedSuiteContinues|TestCoverageStampCaseFilterIsNeverComplete|TestCoverageStampIncompleteRun|TestCoverageStampNoOpWithoutSuiteEntry|TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded|TestDiffCasesBlockOnlyListsMatchingCasesWithFilter|TestDiffFilterNoMatchOnEitherSideOmitsAgent|TestDiffFilterQualityPerDollarUsesFilteredCost|TestDiffLegacyFixtureVsRecordedNative|TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing|TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched)\(' internal/eval/*_test.go)"
      - |-
        after="$(mktemp)"; go test -list '.*' ./internal/eval | grep '^Test' | sort > "$after"; test -z "$(comm -13 .kairon/artifacts/issue-424-tests-before.txt "$after")" && test "$(comm -23 .kairon/artifacts/issue-424-tests-before.txt "$after" | wc -l)" -eq 36
      - |-
        go test -count=1 -cover ./internal/eval | awk '/coverage:/ {v=$5; gsub("%","",v); print "coverage", v; if (v+0 < 87.6) exit 1; found=1} END {if (!found) exit 1}'
      - |-
        go test -count=1 -coverprofile=.kairon/artifacts/issue-424-after.out ./internal/eval && awk 'NR>1 {t+=$2; if ($3>0) c+=$2} END {printf "%.4f\n", 100*c/t}' .kairon/artifacts/issue-424-after.out > .kairon/artifacts/issue-424-after-coverage.txt && awk 'NR==FNR {b=$1+0; next} {a=$1+0} END {exit (a >= b) ? 0 : 1}' .kairon/artifacts/issue-424-baseline-coverage.txt .kairon/artifacts/issue-424-after-coverage.txt
      - task test
      - |-
        test -z "$(gofmt -l internal/eval)"
      - go vet ./internal/eval
      - |-
        base="$(git merge-base HEAD origin/main)" && test -z "$(git diff --name-only "$base" -- . ':!*_test.go' ':!.kairon' ':!.kiro'; git ls-files --others --exclude-standard -- . ':!*_test.go' ':!.kairon' ':!.kiro')"
      - |-
        base="$(git merge-base HEAD origin/main)" && test -z "$(git diff --name-only "$base" HEAD -- .kiro)"

  - id: validate-prune
    agent: validator
    description: >-
      Read-only end-to-end verification against the issue. Check AC1 (Delete grep prints nothing), AC2 (Merge grep prints nothing; each partner
      passes with -v and shows its prescribed subtest names or inline assertions), contract C1 (only _test.go files changed; for each merged
      partner compare with git show of the merge-base version and confirm no original assertion was removed or weakened), AC3 (go test -cover
      reports at least 87.6% and the unrounded ratio is not below the baseline) and AC4 (task test exits 0). Walk the 36 rows of the Scope
      Coverage table one by one: each Delete is gone and its partner passes; each Merge is gone as a function and its distinct input or
      assertion is present in the partner. Check that R7's partner subtest sandbox-limit-timeout asserts the --resource-limit hint, that the
      R24 and R25 partners are cases of TestCoverageStampFullRun, and that execute_case_test.go changed only inside TestConfigureKeepWorkspaces.
      Report any deferral wording or unrecorded deviation from the spec.
    dependencies: ["gate-coverage-and-task-test"]
    acceptance_criteria:
      - "The AC1 grep prints nothing (all 14 Delete tests are gone)"
      - "The AC2 grep prints nothing (all 22 Merge tests are gone as separate functions)"
      - "Every partner passes with -v and shows its prescribed subtests: TestContainerExecsAreWrappedAndPromptStaysOnStdin (kiro-cli-exec, helper-exec), TestContainerTimeoutNamesEffectiveTimeout (kiro-cli_host_deadline, helper-reported_timeout, sandbox-limit-timeout), TestBuildContainerMountsKiroCLI (kiro-cli-default-workspace, stub-default-workspace, stub-custom-workspace), TestContainerConfigUserEnvAndWorkdir (default-workspace, custom-workspace), TestContainmentToolTrustSerialisesEmptyArray (kairon_binary=true, kairon_binary=false), TestUpdateIncrementalSummaryRecordsModeAndContainment (container, native, legacy-result-unknown-mode), TestCoverageResumeChangedSuiteRefused (edited-case-refused, reverted-case-resumes), TestCoverageStampFullRun (six rows), TestConfigureKeepWorkspaces (keepWorkspaces, debug, suites, caseFilter), TestCoverageUpdateIncrementalSummaryCopiesFields (recorded-coverage-copied, unrecorded-result-writes-no-entry), TestAgentVerdictFloorBoundary (several-criteria-named-in-criteria-order), TestDiffFilterBuiltInAgentAndNameKeys (no-match-agent-key, no-match-metadata-key), TestDiffLegacyFixtureVsContainer (container, native), TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites (saved-agent-sorts-last, saved-agent-sorts-first)"
      - "The inline merges are present: the per-turn Timeout/WorkDir/AgentConfigDir/ToolTrust/Prompt checks in TestContainerSessionThreeTurnsOneOpenOneClose, the quality/$ and Cases lines in TestDiffFilterReportsOnlyMatchingCases, and the case=a1 AND speed==fast label in TestRunCaseAndFilterCombineWithAND"
      - "git diff against the merge base shows no non-test file changed outside .kairon/ and .kiro/, and no original partner assertion was removed or weakened"
      - "No commit on the branch touches .kiro/"
      - "go test -count=1 -cover ./internal/eval reports at least 87.6%"
      - "task test exits 0"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestContainerHelperReportedTimeout|TestContainmentDiffKaironBinary|TestContainmentForRecordsKaironBinary|TestCoverageEvaluateProgressiveCaseFilterIncomplete|TestCoverageEvaluateUnpinnedLeavesFieldsUnset|TestCoverageRunCaseEditChangesSuiteNotPrompt|TestCoverageSummaryJSON|TestDescribeContainmentIncludesKaironBinary|TestDiffLegacyVsContainerDoesNotFail|TestLegacyJSONStillUnmarshals|TestMapContainerErrorNamesGivenTimeout|TestRunFixturesLoadAndScore|TestRunSelftestPassesAndPrintsOneVerdictLine|TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray)\(' internal/eval/*_test.go)"
      - |-
        test -z "$(grep -nE 'func (TestAgentVerdictFloorNamesSeveralCriteriaInOrder|TestBuildContainerMountsHelper|TestBuildContainerMountsKaironBinaryOnPathForEveryBackend|TestCaseAndFilterRunRecordsCombinedLabel|TestContainerEnvPathAndEvalDir|TestContainerHelperPromptOnStdinNotArgv|TestContainerKiroCLIPromptOnStdinNotArgv|TestContainerSessionPerTurnRequestSetup|TestContainerTimeoutMapping|TestContainmentKaironBinaryJSON|TestCoverageConfigureResetsRunScopedFields|TestCoverageResumeUnchangedSuiteContinues|TestCoverageStampCaseFilterIsNeverComplete|TestCoverageStampIncompleteRun|TestCoverageStampNoOpWithoutSuiteEntry|TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded|TestDiffCasesBlockOnlyListsMatchingCasesWithFilter|TestDiffFilterNoMatchOnEitherSideOmitsAgent|TestDiffFilterQualityPerDollarUsesFilteredCost|TestDiffLegacyFixtureVsRecordedNative|TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing|TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched)\(' internal/eval/*_test.go)"
      - go test -count=1 -v -run '^(TestContainerExecsAreWrappedAndPromptStaysOnStdin|TestContainerTimeoutNamesEffectiveTimeout|TestBuildContainerMountsKiroCLI|TestContainerConfigUserEnvAndWorkdir|TestContainerSessionThreeTurnsOneOpenOneClose|TestContainmentToolTrustSerialisesEmptyArray|TestUpdateIncrementalSummaryRecordsModeAndContainment|TestCoverageResumeChangedSuiteRefused|TestCoverageStampFullRun|TestConfigureKeepWorkspaces|TestCoverageUpdateIncrementalSummaryCopiesFields|TestAgentVerdictFloorBoundary|TestDiffFilterReportsOnlyMatchingCases|TestDiffFilterBuiltInAgentAndNameKeys|TestDiffLegacyFixtureVsContainer|TestRunCaseAndFilterCombineWithAND|TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites)$' ./internal/eval
      - |-
        base="$(git merge-base HEAD origin/main)" && test -z "$(git diff --name-only "$base" -- . ':!*_test.go' ':!.kairon' ':!.kiro'; git ls-files --others --exclude-standard -- . ':!*_test.go' ':!.kairon' ':!.kiro')"
      - |-
        base="$(git merge-base HEAD origin/main)" && test -z "$(git diff --name-only "$base" HEAD -- .kiro)"
      - |-
        go test -count=1 -cover ./internal/eval | awk '/coverage:/ {v=$5; gsub("%","",v); print "coverage", v; if (v+0 < 87.6) exit 1; found=1} END {if (!found) exit 1}'
      - task test
```
