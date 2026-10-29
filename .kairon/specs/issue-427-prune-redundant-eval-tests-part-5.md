# Design Spec: Prune redundant internal/eval tests, part 5 of 5

Closes #427

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/eval`: `simulated_user_test.go`, `suite_test.go`, `threshold_test.go`, `trend_test.go`, `turns_schema_test.go`, `verifylog_test.go`, `workspace_test.go`" | `prune-simulated-user`, `prune-suite`, `prune-threshold`, `prune-trend`, `prune-turns-schema`, `prune-verifylog-deletes`, `prune-verifylog-merges`, `prune-verifylog-combine`, `prune-workspace-deletes`, `prune-workspace-merges`, `prune-workspace-combine`, `validate-all` |
| AC1 | "The 17 tests marked Delete no longer exist." | `prune-simulated-user`, `prune-suite`, `prune-threshold`, `prune-trend`, `prune-turns-schema`, `prune-verifylog-deletes`, `prune-workspace-deletes`, `validate-all` |
| AC2 | "Each of the 10 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-trend`, `prune-turns-schema`, `prune-verifylog-merges`, `prune-workspace-merges`, `validate-all` |
| AC3 | "Each of the 2 groups marked Combine is one table-driven test with a case for each former test, and the former functions are gone." | `prune-verifylog-combine`, `prune-workspace-combine`, `validate-all` |
| AC4 | "Package coverage does not drop." | `baseline-coverage`, `validate-all` |
| AC5 | "Every remaining test passes unchanged, and the gate passes." | `prune-simulated-user`, `prune-suite`, `prune-threshold`, `prune-trend`, `prune-turns-schema`, `prune-verifylog-deletes`, `prune-verifylog-merges`, `prune-verifylog-combine`, `prune-workspace-deletes`, `prune-workspace-merges`, `prune-workspace-combine`, `validate-all` |
| T01 | "`TestOneTurnClassicCaseStillUsesOneShotContainerPath` ... Delete" | `prune-simulated-user` |
| T02 | "`TestSuiteSHA256Format` ... Delete" | `prune-suite` |
| T03 | "`TestRubricThreshold` ... Delete" | `prune-threshold` |
| T04 | "`TestTrendFixtureSuccessRate` ... Delete" | `prune-trend` |
| T05 | "`TestTrendFixtureProvenance` ... Delete" | `prune-trend` |
| T06 | "`TestTrendEmptyResultsDir` ... Merge into partner `TestTrendMissingResultsDir`" | `prune-trend` |
| T07 | "`TestTrendUnknownAgent` ... Merge into partner `TestTrendMissingResultsDir`" | `prune-trend` |
| T08 | "`TestLoadCasesClassicMultiEntryStubUnchanged` ... Merge into partner `TestLoadCasesClassicTurnOneLoads`" | `prune-turns-schema` |
| T09 | "`TestUserTurns` ... Delete" | `prune-turns-schema` |
| T10 | "`TestVerifyLogFixtureValidEvalIteration` ... Delete" | `prune-verifylog-deletes` |
| T11 | "`TestVerifyLogFixtureValidPhases` ... Delete" | `prune-verifylog-deletes` |
| T12 | "`TestVerifyLogErrorRendersOneLinePerViolation` ... Delete" | `prune-verifylog-deletes` |
| T13 | "`TestVerifyLogConstants` ... Delete" | `prune-verifylog-deletes` |
| T14 | "`TestVerifyLogScoreTolerance` ... Merge into partner `TestVerifyLogBaselineScoreToleranceBoundary`" | `prune-verifylog-merges` |
| T15 | "`TestVerifyLogNoLogs` ... Merge into partner `TestVerifyLogCycleNoLogsReportsOnlyNoLogs`" | `prune-verifylog-merges` |
| T16 | "`TestVerifyLogRejectsRunNamesOutsideResults` ... Combine into one table test with `TestVerifyLogDoesNotFollowRunNameOutOfResults`" | `prune-verifylog-combine` |
| T17 | "`TestVerifyLogDoesNotFollowRunNameOutOfResults` ... Combine into one table test with `TestVerifyLogRejectsRunNamesOutsideResults`" | `prune-verifylog-combine` |
| T18 | "`TestVerifyLogTestHelpersProduceValidJSON` ... Delete" | `prune-verifylog-deletes` |
| T19 | "`TestVerifyLogPhaseMissing` ... Merge into partner `TestVerifyLogPhaseUnknownValue`" | `prune-verifylog-merges` |
| T20 | "`TestVerifyLogPhaseRuleIDsAreRegisteredInOrder` ... Delete" | `prune-verifylog-deletes` |
| T21 | "`TestVerifyLogFixtureEvidenceRuns` ... Delete" | `prune-verifylog-deletes` |
| T22 | "`TestWorkspaceDefaultIsOneEmptyCommit` ... Merge into partner `TestWorkspaceFixtureIsSingleCommitEqualToFixture`" | `prune-workspace-merges` |
| T23 | "`TestWorkspaceRelativeRootBecomesAbsolute` ... Merge into partner `TestWorkspaceDirAbsoluteAndSymlinkResolved`" | `prune-workspace-merges` |
| T24 | "`TestWorkspacesAreIndependent` ... Delete" | `prune-workspace-deletes` |
| T25 | "`TestCaseResultWorkspaceDirJSON` ... Delete" | `prune-workspace-deletes` |
| T26 | "`TestRunOptionsKeepWorkspacesField` ... Delete" | `prune-workspace-deletes` |
| T27 | "`TestRemoveDeletesBinDir` ... Delete" | `prune-workspace-deletes` |
| T28 | "`TestChmodOpenVanishedFileIsSkipped` ... Combine into one table test with `TestChmodOpenVanishedBeforeChmodIsSkipped`" | `prune-workspace-combine` |
| T29 | "`TestChmodOpenVanishedBeforeChmodIsSkipped` ... Combine into one table test with `TestChmodOpenVanishedFileIsSkipped`" | `prune-workspace-combine` |
| T30 | "`TestPermWalkFuncSkipsNotExist` ... Merge into partner `TestPermWalkFuncPropagatesOtherErrors`" | `prune-workspace-merges` |
| T31 | "`TestPermWalkFuncRootNotExistPropagates` ... Merge into partner `TestPermWalkFuncPropagatesOtherErrors`" | `prune-workspace-merges` |

## Deferred work

none

## Solution Approach

Test-only change in one PR, confined to seven files in `internal/eval`. 31 test functions are removed: 17 deleted outright, 10 folded
into a named partner as table rows or subtests, and 4 (two groups of two) replaced by one table-driven test per group. No production
file is edited, no new test file is added, and no template-synchronized file is touched (`task sync:check` is unaffected).

### Measurements taken before any edit (go1.26.5, this worktree)

| Measurement | Value |
|-------------|-------|
| `go test -count=1 -cover ./internal/eval` | `coverage: 87.6% of statements`, about 340 s wall time |
| Exact statements (`-coverprofile`) | 4080 covered of 4659 = 87.5724% |
| Same run with all 31 tests skipped (`-skip`) | 4080 of 4659, identical |

The floor in AC4 (87.6%) equals today's printed value, so there is no slack. Skipping all 31 tests changes no covered statement, so
none of them is the sole cover of any statement. A coverage drop after the change therefore means a merge or combine lost an
assertion that reaches a statement, and the builder must find and restore it. Because the package takes about 340 s to run, builders
use `-run` filters and `go vet` for per-task checks. The full package run happens twice: once in `baseline-coverage` before any edit,
and once in `validate-all`.

Compare coverage using the exact statement count, not the rounded figure. The awk one-liner in the plan sums `$2` (statements) over
the profile rows and the rows with a non-zero count `$3`.

### Verification of the issue's table against the real code

Every function name and line number in the Current / Target State table matches the worktree (checked with
`grep -nE '^func ...' internal/eval/*_test.go`). Findings that matter for the builders:

1. **Partner `TestContainerSessionOneTurnAndClassicUseOneShot` is not in an in-scope file.** It lives in `container_session_test.go:204`.
   It is left untouched. It asserts `opened == 0` and two one-shot calls for a classic case plus a one-turn case, which covers
   everything the deleted `TestOneTurnClassicCaseStillUsesOneShotContainerPath` asserted (0 sessions, one-shot used).
2. **`TestVerifyLogPhaseMissing` versus the existing `""` row.** `TestVerifyLogPhaseUnknownValue` already has a row `` `""` `` (a raw string of
   two double-quote characters, so the YAML is `phase: ""`, a present-but-empty key). `TestVerifyLogPhaseMissing` passes the Go empty
   string `""` to `vlFMPhase`, which **omits the key entirely** (`if phase != ""`). These are different inputs. The merge must add a new
   row for the absent key and must not treat the existing `` `""` `` row as covering it. A loop `for _, bad := range []string{..., ""}` with
   `t.Run(bad, ...)` would also produce an unreadable subtest name, so the table needs an explicit `name` field.
3. **`TestPermWalkFuncPropagatesOtherErrors` will hold the skip rows too.** The issue fixes the partner name, so keep it. The table
   gains rows whose expected outcome is nil (the three not-exist inputs) next to the rows that must propagate.
4. **The `fmt` import in `workspace_test.go` is a canary.** After the 10 workspace removals, the only remaining use of `fmt` is the
   `"wrapped"` case (`fmt.Errorf("walk: %w", pe)`) from `TestPermWalkFuncSkipsNotExist`. I simulated the full deletion on a copy of the
   repo: `go vet` fails with `"fmt" imported and not used` at `workspace_test.go:6`. If that error appears after the builder's
   merge, a case was dropped. Re-add it. Do not delete the import.
5. **Helpers that become unused.** Simulated on a copy with every one of the 31 functions removed and comments ignored, exactly two
   helpers lose their last caller. Go does not flag unused functions, so `go vet` stays green, but they are dead code and must be removed in the
   same task as the tests that used them:
   - `trendFixtureRows` (`trend_test.go:28`), used only by `TestTrendFixtureSuccessRate` and `TestTrendFixtureProvenance`.
   - `vlFixtureCheckStates` (`verifylog_test.go:2300`), used only by `TestVerifyLogFixtureEvidenceRuns`.

   Every other helper keeps at least one caller: `trendNames`, `loadTurnsCase`, `suiteHashAt`, `suiteEvals`, `vlVerdictSummary`,
   `vlAgentJSON`, `vlFixtureRoot`, `cachedInfoEntry`, `errInfoEntry`, `skipOnWindows`. `evalOne` in `checks_helpers_test.go` is
   already unreferenced today; it is out of scope and stays. The imports `regexp` (suite_test.go:290), `reflect` (trend_test.go:72 and
   turns_schema_test.go:60), `encoding/json` (verifylog_test.go:180 and workspace_test.go:758) are still used. After each task run
   `go vet ./internal/eval`, which catches any unused import.
6. **`TestRemoveDeletesBinDir`'s premise holds.** `workspace.go:460` sets `BinDir = filepath.Join(root, binDirName)`, and
   `TestWorkspaceRemove` asserts `ws.root` is gone, so `BinDir` is gone with it.
7. **No documentation names any of the 31 tests.** A repo-wide grep finds them only in the test files themselves,
   `.kairon/research/test-audit-verdicts.md` (the verdict record this issue implements) and the historical
   `.kairon/specs/issue-311-...md`. No doc edit is needed, and the frozen spec is left alone.

### Test-first approach for a test-only refactor

AGENTS.md treats this as a refactor: characterization first, then change, and the same tests pass before and after. Concretely, in
every task:

1. Before editing, run the task's partner tests and the tests in the file with `-run` and confirm they pass (they are the
   characterization).
2. For each Merge or Combine, **add the table row or subtest first**, run the partner and confirm it passes with the new row, and only then
   delete the old function. Re-run the partner after the deletion.
3. A merged partner keeps every assertion it had, and each merged row keeps every assertion of the former test. A row may gain an
   assertion the former test did not have only if it passes without production changes. If it does not pass, keep the row to the
   former test's own assertions.
4. Commit the test edits together, one commit per task, conventional style (`test(eval): ...`). Each commit must leave
   `go vet ./internal/eval` green. Do not push. Do not stage `.kiro/agents/*.json` (sandbox-injected modifications).
5. Say which tests were relied on in the commit message (refactor rule in AGENTS.md).

### Names the PR must state (AC3)

Because the former test names must disappear, the two combined table tests get new names:

- `TestVerifyLogRunNameEscapeRejected` in `verifylog_test.go` (combines `TestVerifyLogRejectsRunNamesOutsideResults` and
  `TestVerifyLogDoesNotFollowRunNameOutOfResults`). Top-level subtests `RejectsRunNamesOutsideResults` (the baseline_run field,
  with its five run names as nested subtests) and `DoesNotFollowRunNameOutOfResults` (the result_run field).
- `TestChmodOpenVanishedIsSkipped` in `workspace_test.go` (combines `TestChmodOpenVanishedFileIsSkipped` and
  `TestChmodOpenVanishedBeforeChmodIsSkipped`). Subtests `VanishedFileIsSkipped` (Info step) and `VanishedBeforeChmodIsSkipped`
  (Chmod step).

Both names are free today (grep finds no existing definition). The PR description must name both, state `Deferred work: none` and
`Spec deviations: none`, and list the 8 partners below.

Partners that must show the merged cases as subtests in `go test -run '^<partner>$' -v`:

| Partner | Merged in |
|---------|-----------|
| `TestTrendMissingResultsDir` | `TestTrendEmptyResultsDir`, `TestTrendUnknownAgent` |
| `TestLoadCasesClassicTurnOneLoads` | `TestLoadCasesClassicMultiEntryStubUnchanged` |
| `TestVerifyLogBaselineScoreToleranceBoundary` | `TestVerifyLogScoreTolerance` |
| `TestVerifyLogCycleNoLogsReportsOnlyNoLogs` | `TestVerifyLogNoLogs` |
| `TestVerifyLogPhaseUnknownValue` | `TestVerifyLogPhaseMissing` |
| `TestWorkspaceFixtureIsSingleCommitEqualToFixture` | `TestWorkspaceDefaultIsOneEmptyCommit` |
| `TestWorkspaceDirAbsoluteAndSymlinkResolved` | `TestWorkspaceRelativeRootBecomesAbsolute` |
| `TestPermWalkFuncPropagatesOtherErrors` | `TestPermWalkFuncSkipsNotExist`, `TestPermWalkFuncRootNotExistPropagates` |

## Relevant Files

All paths are under `internal/eval/`. All are modified; none is created.

| File | Change |
|------|--------|
| `simulated_user_test.go` | delete 1 test |
| `suite_test.go` | delete 1 test |
| `threshold_test.go` | delete 1 test |
| `trend_test.go` | delete 2 tests and helper `trendFixtureRows`; fold 2 tests into `TestTrendMissingResultsDir` |
| `turns_schema_test.go` | delete 1 test; fold 1 test into `TestLoadCasesClassicTurnOneLoads` |
| `verifylog_test.go` | delete 7 tests and helper `vlFixtureCheckStates`; fold 3 tests into 3 partners; combine 2 tests into `TestVerifyLogRunNameEscapeRejected` |
| `workspace_test.go` | delete 4 tests; fold 4 tests into 3 partners; combine 2 tests into `TestChmodOpenVanishedIsSkipped` |

Read-only references: `container_session_test.go` (partner of T01), `verifylog.go` (`ruleOrder`), `workspace.go` (`BinDir`, `root`,
`permWalkFunc`, `chmodOpen`), `.kairon/research/test-audit-verdicts.md`.

## Team Orchestration

Only builders edit files and one validator verifies. There are no documenter or architect tasks (no doc names these tests).

- `baseline-coverage` runs first and alone. It must finish before any test file is edited, because it measures the unmodified package.
- The seven file groups touch the same Go package. A compile error in one file breaks `go vet` and `go test` for every other file, so
  the builder tasks run **in sequence**, not in parallel. Order: simulated_user, suite, threshold, trend, turns_schema, verifylog (three
  tasks), workspace (three tasks).
- `validate-all` runs last, read-only. It runs the full package once for coverage and `task test` once for the gate.

### Step-by-Step Task Breakdown

1. **`baseline-coverage`.** No edits. Run `go test -count=1 -coverprofile=/tmp/kairon-427-baseline.out ./internal/eval` (about 6
   minutes) and record the printed percentage and the exact covered and total statement counts. Expected: 87.6%, 4080 of 4659. If
   the number differs, the validator uses the recorded figure as the floor and still requires the printed value of at least 87.6%.
2. **`prune-simulated-user`.** Delete `TestOneTurnClassicCaseStillUsesOneShotContainerPath`. Partner `TestContainerSessionOneTurnAndClassicUseOneShot` is
   in `container_session_test.go` and stays unchanged.
3. **`prune-suite`.** Delete `TestSuiteSHA256Format`. The remaining hash-format assertion at `suite_test.go:290` keeps `regexp` in use.
4. **`prune-threshold`.** Delete `TestRubricThreshold`. `TestLoadRubricsPassThreshold` and `TestCaseThreshold` stay unchanged.
5. **`prune-trend`.** Delete `TestTrendFixtureSuccessRate` and `TestTrendFixtureProvenance`, then delete the now-unused
   `trendFixtureRows`. `TestTrendTextOutput` already pins both facts line by line and stays unchanged. Convert
   `TestTrendMissingResultsDir` to a table with four rows:
   - missing results dir: `t.TempDir()` with no `results/`, `Trend` text mode, needle `trendAgent`. Keeps every current assertion:
     error returned, error names the agent, nothing written, `LoadTrend` also errors.
   - empty results dir (from `TestTrendEmptyResultsDir`): `results/` created, empty. Assertion: `LoadTrend` errors and names `trendAgent`.
   - unknown agent, text mode, and unknown agent, `JSON: true` (from `TestTrendUnknownAgent`, which loops over `js` in `{false, true}`):
     evals dir is `trendFixture`, agent `"nope"`. Assertions: `Trend` errors, the error contains the quoted `"nope"`, the buffer is empty.
   Each row asserts at least its former test's checks; running both the `Trend` and `LoadTrend` checks on every row is allowed only if
   they pass.
6. **`prune-turns-schema`.** Delete `TestUserTurns`. Convert `TestLoadCasesClassicTurnOneLoads` into a table with two rows: the existing
   `turn: 1` YAML (asserts no error, one case), and the multi-entry `stub.turns` YAML from `TestLoadCasesClassicMultiEntryStubUnchanged`
   (asserts no error, one case, and `cases[0].Turns == nil`, applied to the stub row via a field in the table). `reflect` stays in use.
7. **`prune-verifylog-deletes`.** Delete `TestVerifyLogFixtureValidEvalIteration`, `TestVerifyLogFixtureValidPhases`,
   `TestVerifyLogErrorRendersOneLinePerViolation`, `TestVerifyLogConstants`, `TestVerifyLogTestHelpersProduceValidJSON`,
   `TestVerifyLogPhaseRuleIDsAreRegisteredInOrder`, `TestVerifyLogFixtureEvidenceRuns`, and the helper `vlFixtureCheckStates`. Remove the
   leading comment blocks that belong to the deleted functions (for example the "ensure the helper JSON builders ..." comment). Keep
   the `// ---------- ... ----------` section banners where other tests still follow them.
8. **`prune-verifylog-merges`.** Three folds, each adding the row before deleting the old function.
   - `TestVerifyLogScoreTolerance` into `TestVerifyLogBaselineScoreToleranceBoundary`: one table with a field column (`baseline_score` or
     `result_score`). Keep the partner's two baseline cases (60.1 passes as "exactly 0.1" in float64, 60.2 fails and names
     `baseline_score`). Add the five `result_score` cases against the recorded 72.5 (72.5, 72.6, 72.4 pass; 72.7 and 72.3 fail), where a
     failing row also asserts the message mentions `result_score`, the claimed value, `72.5` and `r1`.
   - `TestVerifyLogNoLogs` into `TestVerifyLogCycleNoLogsReportsOnlyNoLogs`: one table over `RequireCycle` in `{false, true}`, with the
     four subtests (missing directory, empty directory, only non-log files, reported even when min-iterations is 0). Use
     `{"v2.md": "candidate", "notes.txt": "n"}` for the only-non-log-files input, which contains the partner's `notes.txt`. The
     `MinIterations = 0` subtest runs for `RequireCycle == false` as it does today. Running it with `true` is allowed if it passes.
   - `TestVerifyLogPhaseMissing` into `TestVerifyLogPhaseUnknownValue`: give the table an explicit `name` and `phase` per row, keep the four
     existing rows, and add a row `key absent` with the Go empty string phase (see finding 2). Same assertions: a single `phase` violation on
     `iteration-00.md` whose message mentions `phase`, `baseline`, `red`, `green`, `refactor`.
9. **`prune-verifylog-combine`.** Replace `TestVerifyLogRejectsRunNamesOutsideResults` and `TestVerifyLogDoesNotFollowRunNameOutOfResults`
   with `TestVerifyLogRunNameEscapeRejected`. Subtest `RejectsRunNamesOutsideResults`: nested subtests for `../outside`, `..`, `a/b`, `a\b`,
   `r0/../r1` as `baseline_run`, with the decoy `outside/summary.json` (`vlVerdictSummary(60.0, vlShaA)`) and `outside/<agent>.json`
   (`vlAgentJSON(vlShaA)`) written, asserting exactly one `front-matter` violation whose message names `baseline_run`. Subtest
   `DoesNotFollowRunNameOutOfResults`: `result_run` is `../outside` with baseline `r0`, scores `60.0` and `50.0`, decoy summary
   `vlVerdictSummary(50.0, vlShaB)`, asserting exactly one `front-matter` violation whose message names `result_run`. A table row may carry a
   setup function so the two different decoys and front-matter shapes stay exact.
10. **`prune-workspace-deletes`.** Delete `TestWorkspacesAreIndependent`, `TestCaseResultWorkspaceDirJSON`,
    `TestRunOptionsKeepWorkspacesField`, `TestRemoveDeletesBinDir`. Leave `snapshotTree` and the gh_issue test that follows in place.
11. **`prune-workspace-merges`.** Three folds, row first. Watch the `fmt` canary (finding 4).
    - `TestWorkspaceDefaultIsOneEmptyCommit` into `TestWorkspaceFixtureIsSingleCommitEqualToFixture`: table with rows `no fixture`
      (`TestCase{Name: "c"}`, expected tree empty, `.eval` directory exists) and `seeded fixture` (the current fixture setup, tree
      `README.md\npkg/deep/file.txt\nrun.sh`, content and exec-bit checks). All rows assert commit count 1 and clean `git status --porcelain`.
      The fixture files are written only for the seeded row.
    - `TestWorkspaceRelativeRootBecomesAbsolute` into `TestWorkspaceDirAbsoluteAndSymlinkResolved`: rows `symlinked root` and `relative root`
      (`workspaceRootEnv = "rel-root"`). Move the Windows skip into the symlink row only, so the relative row runs on every OS as it does today. The
      relative row asserts `filepath.IsAbs(ws.Dir)`. The symlink-only assertions (resolved path, honored root, sub paths) stay on the
      symlink row.
    - `TestPermWalkFuncSkipsNotExist` and `TestPermWalkFuncRootNotExistPropagates` into `TestPermWalkFuncPropagatesOtherErrors`: table
      of `name, root, path, in, wantErr`. Rows: `permission`, `eio`, `eacces` (propagate, `errors.Is(err, in)`), `path error`, `wrapped`
      (uses `fmt.Errorf`), `sentinel` (all must return nil), and `root not exist` (path equals root; must return an error that
      `errors.Is(err, fs.ErrNotExist)`, plus the extra check that `filepath.WalkDir` over the missing root returns an error).
12. **`prune-workspace-combine`.** Replace the two chmod tests with `TestChmodOpenVanishedIsSkipped`, two subtests built from a row
    that carries a mode, a file mode and an entry builder. `VanishedFileIsSkipped` (file `maintenance.lock`, mode `0o644`): `os.ReadDir`,
    remove the file, then call `chmodOpen(p, entries[0], 0o666)`. `VanishedBeforeChmodIsSkipped` (file `f`, mode `0o600`): cache
    `entries[0].Info()`, remove the file, call `chmodOpen(p, cachedInfoEntry{fs.FileInfoToDirEntry(info), info}, 0o666)`. Both assert a nil
    result. Keep `skipOnWindows(t)` at the top of the parent test and keep the explanatory comments on each row. `cachedInfoEntry` and
    `errInfoEntry` stay (the latter serves `TestChmodOpenOtherInfoErrorPropagates`).
13. **`validate-all`.** Read-only checks of AC1 to AC5 and a diff scope check. See the plan.

## Validation Commands

Run from the repository root.

```bash
# AC1: must print nothing
grep -nE 'func (TestCaseResultWorkspaceDirJSON|TestOneTurnClassicCaseStillUsesOneShotContainerPath|TestRemoveDeletesBinDir|TestRubricThreshold|TestRunOptionsKeepWorkspacesField|TestSuiteSHA256Format|TestTrendFixtureProvenance|TestTrendFixtureSuccessRate|TestUserTurns|TestVerifyLogConstants|TestVerifyLogErrorRendersOneLinePerViolation|TestVerifyLogFixtureEvidenceRuns|TestVerifyLogFixtureValidEvalIteration|TestVerifyLogFixtureValidPhases|TestVerifyLogPhaseRuleIDsAreRegisteredInOrder|TestVerifyLogTestHelpersProduceValidJSON|TestWorkspacesAreIndependent)\(' internal/eval/*_test.go

# AC2: must print nothing
grep -nE 'func (TestLoadCasesClassicMultiEntryStubUnchanged|TestPermWalkFuncRootNotExistPropagates|TestPermWalkFuncSkipsNotExist|TestTrendEmptyResultsDir|TestTrendUnknownAgent|TestVerifyLogNoLogs|TestVerifyLogPhaseMissing|TestVerifyLogScoreTolerance|TestWorkspaceDefaultIsOneEmptyCommit|TestWorkspaceRelativeRootBecomesAbsolute)\(' internal/eval/*_test.go

# AC3: must print nothing
grep -nE 'func (TestChmodOpenVanishedBeforeChmodIsSkipped|TestChmodOpenVanishedFileIsSkipped|TestVerifyLogDoesNotFollowRunNameOutOfResults|TestVerifyLogRejectsRunNamesOutsideResults)\(' internal/eval/*_test.go

# AC4: exact comparison with the baseline profile (covered statements must be >= baseline covered)
go test -count=1 -coverprofile=/tmp/kairon-427-after.out ./internal/eval
awk 'NR>1{n+=$2; if($3>0)c+=$2} END{printf "stmts=%d covered=%d pct=%.4f\n", n, c, 100*c/n}' /tmp/kairon-427-after.out
go test -cover ./internal/eval            # must print coverage: 87.6% or higher

# AC5
task test                                  # exit 0
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "baseline-coverage"
    agent: "builder"
    description: "Measure and record the unmodified package's coverage before any test file is edited, so AC4 can be checked exactly. Make no file edits. Write the profile to /tmp/kairon-427-baseline.out (outside the repo)."
    dependencies: []
    acceptance_criteria:
      - "go test -count=1 -coverprofile=/tmp/kairon-427-baseline.out ./internal/eval passes on the unmodified tree"
      - "The printed coverage and the exact covered/total statement counts from the profile are recorded in the sentinel (architect measured 87.6%, 4080 of 4659)"
      - "No file under the repository is modified by this task"
    validation_commands:
      - "test -s /tmp/kairon-427-baseline.out"
      - "awk 'NR>1{n+=$2; if($3>0)c+=$2} END{printf \"stmts=%d covered=%d pct=%.4f\\n\", n, c, 100*c/n}' /tmp/kairon-427-baseline.out"
      - "git diff --quiet -- internal/eval"

  - id: "prune-simulated-user"
    agent: "builder"
    description: "In internal/eval/simulated_user_test.go delete TestOneTurnClassicCaseStillUsesOneShotContainerPath. Its partner TestContainerSessionOneTurnAndClassicUseOneShot is in container_session_test.go and must stay unchanged. First confirm the partner passes."
    dependencies: ["baseline-coverage"]
    acceptance_criteria:
      - "func TestOneTurnClassicCaseStillUsesOneShotContainerPath no longer exists in internal/eval"
      - "The partner TestContainerSessionOneTurnAndClassicUseOneShot passed before the edit and passes after it, and container_session_test.go is unchanged"
      - "go vet ./internal/eval is clean and no helper in simulated_user_test.go is left unreferenced"
      - "Changes are committed once with a conventional test(eval) message that names the tests relied on"
    validation_commands:
      - "! grep -nE 'func TestOneTurnClassicCaseStillUsesOneShotContainerPath\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^(TestContainerSessionOneTurnAndClassicUseOneShot|TestSimulatedUserFieldsAbsentFromOtherCases)$' ./internal/eval"
      - "git diff --quiet $(git merge-base HEAD origin/main) -- internal/eval/container_session_test.go"

  - id: "prune-suite"
    agent: "builder"
    description: "In internal/eval/suite_test.go delete TestSuiteSHA256Format. The hex-format assertion at suite_test.go:290 keeps the regexp import in use."
    dependencies: ["prune-simulated-user"]
    acceptance_criteria:
      - "func TestSuiteSHA256Format no longer exists"
      - "The remaining TestSuiteSHA256* tests pass unchanged and the regexp import is still used"
      - "go vet ./internal/eval is clean"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func TestSuiteSHA256Format\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^TestSuiteSHA256' ./internal/eval"

  - id: "prune-threshold"
    agent: "builder"
    description: "In internal/eval/threshold_test.go delete TestRubricThreshold. TestLoadRubricsPassThreshold (partner) and TestCaseThreshold stay unchanged."
    dependencies: ["prune-suite"]
    acceptance_criteria:
      - "func TestRubricThreshold no longer exists"
      - "TestLoadRubricsPassThreshold and TestCaseThreshold pass unchanged"
      - "go vet ./internal/eval is clean"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func TestRubricThreshold\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^(TestLoadRubricsPassThreshold|TestCaseThreshold)$' ./internal/eval"

  - id: "prune-trend"
    agent: "builder"
    description: "In internal/eval/trend_test.go delete TestTrendFixtureSuccessRate and TestTrendFixtureProvenance and the helper trendFixtureRows that only they used (TestTrendTextOutput already pins their facts and stays unchanged). Fold TestTrendEmptyResultsDir and TestTrendUnknownAgent into TestTrendMissingResultsDir as table rows (missing dir, empty results dir, unknown agent text mode, unknown agent JSON mode), keeping every former assertion, adding each row before deleting the old function."
    dependencies: ["prune-threshold"]
    acceptance_criteria:
      - "TestTrendFixtureSuccessRate, TestTrendFixtureProvenance, TestTrendEmptyResultsDir and TestTrendUnknownAgent no longer exist, and trendFixtureRows is removed"
      - "TestTrendMissingResultsDir is table-driven and go test -run '^TestTrendMissingResultsDir$' -v lists subtests for the missing dir, empty results dir and unknown agent in both text and JSON mode"
      - "Each merged row keeps the former test's assertions: empty dir -> LoadTrend error naming trendAgent; unknown agent -> Trend error containing the quoted \"nope\" and nothing written, in both JSON modes; missing dir -> all of the partner's current checks"
      - "TestTrendTextOutput is unchanged and passes; go vet ./internal/eval is clean"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func (TestTrendFixtureSuccessRate|TestTrendFixtureProvenance|TestTrendEmptyResultsDir|TestTrendUnknownAgent|trendFixtureRows)\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^TestTrendMissingResultsDir$' -v ./internal/eval | grep -cE '^[[:space:]]+--- PASS: TestTrendMissingResultsDir/' | awk '$1>=4{ok=1} END{exit !ok}'"
      - "go test -count=1 -run '^TestTrend' ./internal/eval"

  - id: "prune-turns-schema"
    agent: "builder"
    description: "In internal/eval/turns_schema_test.go delete TestUserTurns. Fold TestLoadCasesClassicMultiEntryStubUnchanged into TestLoadCasesClassicTurnOneLoads as a second table row, keeping the cases[0].Turns == nil assertion on the stub row."
    dependencies: ["prune-trend"]
    acceptance_criteria:
      - "TestUserTurns and TestLoadCasesClassicMultiEntryStubUnchanged no longer exist"
      - "TestLoadCasesClassicTurnOneLoads is table-driven with the turn-1 row and the multi-entry-stub row, and both rows pass as subtests"
      - "The stub row still asserts no load error, exactly one case, and Turns == nil"
      - "go vet ./internal/eval is clean (reflect import still used)"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func (TestUserTurns|TestLoadCasesClassicMultiEntryStubUnchanged)\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^TestLoadCasesClassicTurnOneLoads$' -v ./internal/eval | grep -cE '^[[:space:]]+--- PASS: TestLoadCasesClassicTurnOneLoads/' | awk '$1>=2{ok=1} END{exit !ok}'"
      - "go test -count=1 -run 'TestLoadCases|TestTurns' ./internal/eval"

  - id: "prune-verifylog-deletes"
    agent: "builder"
    description: "In internal/eval/verifylog_test.go delete TestVerifyLogFixtureValidEvalIteration, TestVerifyLogFixtureValidPhases, TestVerifyLogErrorRendersOneLinePerViolation, TestVerifyLogConstants, TestVerifyLogTestHelpersProduceValidJSON, TestVerifyLogPhaseRuleIDsAreRegisteredInOrder and TestVerifyLogFixtureEvidenceRuns, plus the helper vlFixtureCheckStates that only the last one used and any comment block that belonged to a deleted function. Confirm first that TestVerifyLogFixtureSets, TestVerifyLogCycleFixtureSets, TestVerifyLogReportsAllViolations and TestVerifyLogPhaseViolationDoesNotSuppressOtherChecks pass."
    dependencies: ["prune-turns-schema"]
    acceptance_criteria:
      - "The seven listed test functions and vlFixtureCheckStates no longer exist"
      - "TestVerifyLogFixtureSets, TestVerifyLogCycleFixtureSets, TestVerifyLogReportsAllViolations and TestVerifyLogPhaseViolationDoesNotSuppressOtherChecks pass unchanged"
      - "go vet ./internal/eval is clean (encoding/json, os, fmt imports still used) and no orphaned comment is left behind"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func (TestVerifyLogFixtureValidEvalIteration|TestVerifyLogFixtureValidPhases|TestVerifyLogErrorRendersOneLinePerViolation|TestVerifyLogConstants|TestVerifyLogTestHelpersProduceValidJSON|TestVerifyLogPhaseRuleIDsAreRegisteredInOrder|TestVerifyLogFixtureEvidenceRuns|vlFixtureCheckStates)\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^(TestVerifyLogFixtureSets|TestVerifyLogCycleFixtureSets|TestVerifyLogReportsAllViolations|TestVerifyLogPhaseViolationDoesNotSuppressOtherChecks)$' ./internal/eval"

  - id: "prune-verifylog-merges"
    agent: "builder"
    description: "In internal/eval/verifylog_test.go fold three tests into partners as table rows or subtests, adding each row before deleting the old function. (1) TestVerifyLogScoreTolerance into TestVerifyLogBaselineScoreToleranceBoundary (field column baseline_score/result_score; keep 60.1 pass and 60.2 fail naming baseline_score; add the five result_score cases against recorded 72.5 with the message checks). (2) TestVerifyLogNoLogs into TestVerifyLogCycleNoLogsReportsOnlyNoLogs (table over RequireCycle false/true; four subtests; only-non-log-files input is v2.md plus notes.txt). (3) TestVerifyLogPhaseMissing into TestVerifyLogPhaseUnknownValue (explicit name+phase per row; add a 'key absent' row using the Go empty string, which vlFMPhase omits; do not rely on the existing raw-string row for two double quotes)."
    dependencies: ["prune-verifylog-deletes"]
    acceptance_criteria:
      - "TestVerifyLogScoreTolerance, TestVerifyLogNoLogs and TestVerifyLogPhaseMissing no longer exist"
      - "go test -run '^TestVerifyLogBaselineScoreToleranceBoundary$' -v shows baseline_score and result_score subtests including the 72.7 and 72.3 failing rows"
      - "go test -run '^TestVerifyLogCycleNoLogsReportsOnlyNoLogs$' -v shows the no-logs subtests for both RequireCycle=false and RequireCycle=true"
      - "go test -run '^TestVerifyLogPhaseUnknownValue$' -v shows a subtest for the absent phase key in addition to the four existing values, with the same single phase violation and message checks"
      - "Every merged row keeps all assertions of its former test"
      - "go vet ./internal/eval is clean"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func (TestVerifyLogScoreTolerance|TestVerifyLogNoLogs|TestVerifyLogPhaseMissing)\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^(TestVerifyLogBaselineScoreToleranceBoundary|TestVerifyLogCycleNoLogsReportsOnlyNoLogs|TestVerifyLogPhaseUnknownValue)$' -v ./internal/eval"
      - "go test -count=1 -run '^TestVerifyLog' ./internal/eval"

  - id: "prune-verifylog-combine"
    agent: "builder"
    description: "In internal/eval/verifylog_test.go replace TestVerifyLogRejectsRunNamesOutsideResults and TestVerifyLogDoesNotFollowRunNameOutOfResults with one table-driven test named TestVerifyLogRunNameEscapeRejected. Subtest RejectsRunNamesOutsideResults covers baseline_run with nested subtests for ../outside, .., a/b, a\\b and r0/../r1 and the decoy outside/ summary and agent JSON; subtest DoesNotFollowRunNameOutOfResults covers result_run ../outside with its own decoy summary. Each keeps its exact violation assertions (one front-matter violation naming baseline_run or result_run)."
    dependencies: ["prune-verifylog-merges"]
    acceptance_criteria:
      - "TestVerifyLogRejectsRunNamesOutsideResults and TestVerifyLogDoesNotFollowRunNameOutOfResults no longer exist"
      - "TestVerifyLogRunNameEscapeRejected exists and go test -run '^TestVerifyLogRunNameEscapeRejected$' -v lists exactly one top-level subtest per former test: RejectsRunNamesOutsideResults and DoesNotFollowRunNameOutOfResults"
      - "The baseline_run subtest still covers all five run names and the result_run subtest still asserts a single front-matter violation naming result_run"
      - "go vet ./internal/eval is clean"
      - "Changes are committed with a conventional test(eval) message that names the new table test"
    validation_commands:
      - "! grep -nE 'func (TestVerifyLogRejectsRunNamesOutsideResults|TestVerifyLogDoesNotFollowRunNameOutOfResults)\\(' internal/eval/*_test.go"
      - "grep -nE 'func TestVerifyLogRunNameEscapeRejected\\(' internal/eval/verifylog_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^TestVerifyLogRunNameEscapeRejected$' -v ./internal/eval | grep -E '^[[:space:]]+--- PASS: TestVerifyLogRunNameEscapeRejected/(RejectsRunNamesOutsideResults|DoesNotFollowRunNameOutOfResults) '"

  - id: "prune-workspace-deletes"
    agent: "builder"
    description: "In internal/eval/workspace_test.go delete TestWorkspacesAreIndependent, TestCaseResultWorkspaceDirJSON, TestRunOptionsKeepWorkspacesField and TestRemoveDeletesBinDir. Confirm first that TestWorkspaceRemove (asserts ws.root is gone, and BinDir is root/bin per workspace.go:460) passes. The fmt import stays in use at this point because TestPermWalkFuncSkipsNotExist still exists; the merge task keeps it in use through the wrapped case."
    dependencies: ["prune-verifylog-combine"]
    acceptance_criteria:
      - "The four listed test functions no longer exist"
      - "TestWorkspaceRemove passes unchanged"
      - "go vet ./internal/eval is clean at the commit (fmt is still used at this point because TestPermWalkFuncSkipsNotExist has not been merged yet)"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func (TestWorkspacesAreIndependent|TestCaseResultWorkspaceDirJSON|TestRunOptionsKeepWorkspacesField|TestRemoveDeletesBinDir)\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^(TestWorkspaceRemove|TestWorkspaceRemoveToleratesUnreadableDirs|TestWorkspaceRemoveSwallowsFailure|TestWorkspaceCreationFailureLeavesNothingBehind)$' ./internal/eval"

  - id: "prune-workspace-merges"
    agent: "builder"
    description: "In internal/eval/workspace_test.go fold four tests into three partners, row first. (1) TestWorkspaceDefaultIsOneEmptyCommit into TestWorkspaceFixtureIsSingleCommitEqualToFixture: rows 'no fixture' (empty tree, .eval dir exists) and 'seeded fixture' (existing content and exec-bit checks); both assert commit count 1 and clean status; fixture files written only for the seeded row. (2) TestWorkspaceRelativeRootBecomesAbsolute into TestWorkspaceDirAbsoluteAndSymlinkResolved: rows 'symlinked root' and 'relative root'; the Windows skip moves to the symlink row only; symlink-only assertions stay on that row. (3) TestPermWalkFuncSkipsNotExist and TestPermWalkFuncRootNotExistPropagates into TestPermWalkFuncPropagatesOtherErrors: table of name/root/path/in/wantErr with rows permission, eio, eacces, path error, wrapped (fmt.Errorf), sentinel, and root not exist (also asserts filepath.WalkDir over the missing root errors). Keep the partner name even though it now has nil-result rows."
    dependencies: ["prune-workspace-deletes"]
    acceptance_criteria:
      - "TestWorkspaceDefaultIsOneEmptyCommit, TestWorkspaceRelativeRootBecomesAbsolute, TestPermWalkFuncSkipsNotExist and TestPermWalkFuncRootNotExistPropagates no longer exist"
      - "go test -run '^TestWorkspaceFixtureIsSingleCommitEqualToFixture$' -v lists a no-fixture and a seeded-fixture subtest"
      - "go test -run '^TestWorkspaceDirAbsoluteAndSymlinkResolved$' -v lists a symlinked-root and a relative-root subtest"
      - "go test -run '^TestPermWalkFuncPropagatesOtherErrors$' -v lists subtests for all seven inputs including 'wrapped' and 'root not exist'"
      - "The fmt import is still used (by the 'wrapped' case) and go vet ./internal/eval is clean"
      - "Every merged row keeps all assertions of its former test"
      - "Changes are committed with a conventional test(eval) message"
    validation_commands:
      - "! grep -nE 'func (TestWorkspaceDefaultIsOneEmptyCommit|TestWorkspaceRelativeRootBecomesAbsolute|TestPermWalkFuncSkipsNotExist|TestPermWalkFuncRootNotExistPropagates)\\(' internal/eval/*_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^(TestWorkspaceFixtureIsSingleCommitEqualToFixture|TestWorkspaceDirAbsoluteAndSymlinkResolved|TestPermWalkFuncPropagatesOtherErrors)$' -v ./internal/eval"
      - "go test -count=1 -run '^(TestWorkspace|TestPermWalk)' ./internal/eval"

  - id: "prune-workspace-combine"
    agent: "builder"
    description: "In internal/eval/workspace_test.go replace TestChmodOpenVanishedFileIsSkipped and TestChmodOpenVanishedBeforeChmodIsSkipped with one table-driven test named TestChmodOpenVanishedIsSkipped, with subtests VanishedFileIsSkipped (file maintenance.lock, mode 0644, entry from os.ReadDir, file removed, chmodOpen(p, entries[0], 0o666) returns nil) and VanishedBeforeChmodIsSkipped (file f, mode 0600, cached Info, file removed, chmodOpen with cachedInfoEntry returns nil). Keep skipOnWindows, the explanatory comments on each row, and cachedInfoEntry/errInfoEntry (the latter is used by TestChmodOpenOtherInfoErrorPropagates)."
    dependencies: ["prune-workspace-merges"]
    acceptance_criteria:
      - "TestChmodOpenVanishedFileIsSkipped and TestChmodOpenVanishedBeforeChmodIsSkipped no longer exist"
      - "go test -run '^TestChmodOpenVanishedIsSkipped$' -v lists exactly one subtest per former test: VanishedFileIsSkipped and VanishedBeforeChmodIsSkipped"
      - "Both subtests assert that chmodOpen returns nil for a vanished target, as the former tests did"
      - "TestChmodOpenOtherInfoErrorPropagates passes unchanged and go vet ./internal/eval is clean"
      - "Changes are committed with a conventional test(eval) message that names the new table test"
    validation_commands:
      - "! grep -nE 'func (TestChmodOpenVanishedFileIsSkipped|TestChmodOpenVanishedBeforeChmodIsSkipped)\\(' internal/eval/*_test.go"
      - "grep -nE 'func TestChmodOpenVanishedIsSkipped\\(' internal/eval/workspace_test.go"
      - "go vet ./internal/eval"
      - "go test -count=1 -run '^TestChmodOpenVanishedIsSkipped$' -v ./internal/eval | grep -E '^[[:space:]]+--- PASS: TestChmodOpenVanishedIsSkipped/(VanishedFileIsSkipped|VanishedBeforeChmodIsSkipped) '"
      - "go test -count=1 -run '^TestChmodOpenOtherInfoErrorPropagates$' ./internal/eval"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of AC1 to AC5 and the scope of the diff. Check that the 17 Delete, 10 Merge and 4 Combine former functions are gone (the three greps in the issue print nothing), that each of the 8 partners and both new table tests pass with the expected subtests, that only the seven in-scope test files changed (ignore .kiro/agents/*.json, which are sandbox-injected), that no production file changed, that no assertion was dropped in any merged or combined test (compare against git show of the pre-change file), that covered statements are at least the baseline recorded by baseline-coverage and the printed coverage is at least 87.6%, that go vet and gofmt are clean, and that task test exits 0. Also confirm the PR text will say Deferred work: none, Spec deviations: none, and name TestVerifyLogRunNameEscapeRejected and TestChmodOpenVanishedIsSkipped."
    dependencies: ["prune-workspace-combine"]
    acceptance_criteria:
      - "The AC1, AC2 and AC3 grep commands from the issue print nothing"
      - "Each partner (TestTrendMissingResultsDir, TestLoadCasesClassicTurnOneLoads, TestVerifyLogBaselineScoreToleranceBoundary, TestVerifyLogCycleNoLogsReportsOnlyNoLogs, TestVerifyLogPhaseUnknownValue, TestWorkspaceFixtureIsSingleCommitEqualToFixture, TestWorkspaceDirAbsoluteAndSymlinkResolved, TestPermWalkFuncPropagatesOtherErrors) passes with the merged case among its subtests"
      - "TestVerifyLogRunNameEscapeRejected and TestChmodOpenVanishedIsSkipped each list one subtest per former test"
      - "go test -cover ./internal/eval reports at least 87.6%, and covered statements in the final profile are at least the baseline's covered count (4080 of 4659 unless baseline-coverage recorded otherwise)"
      - "Every test not listed in the issue passes unchanged and task test exits 0"
      - "git diff against the branch point touches only the seven in-scope test files, the spec, and nothing under non-test Go files"
      - "go vet ./... and gofmt -l internal/eval are clean"
    validation_commands:
      - "! grep -nE 'func (TestCaseResultWorkspaceDirJSON|TestOneTurnClassicCaseStillUsesOneShotContainerPath|TestRemoveDeletesBinDir|TestRubricThreshold|TestRunOptionsKeepWorkspacesField|TestSuiteSHA256Format|TestTrendFixtureProvenance|TestTrendFixtureSuccessRate|TestUserTurns|TestVerifyLogConstants|TestVerifyLogErrorRendersOneLinePerViolation|TestVerifyLogFixtureEvidenceRuns|TestVerifyLogFixtureValidEvalIteration|TestVerifyLogFixtureValidPhases|TestVerifyLogPhaseRuleIDsAreRegisteredInOrder|TestVerifyLogTestHelpersProduceValidJSON|TestWorkspacesAreIndependent)\\(' internal/eval/*_test.go"
      - "! grep -nE 'func (TestLoadCasesClassicMultiEntryStubUnchanged|TestPermWalkFuncRootNotExistPropagates|TestPermWalkFuncSkipsNotExist|TestTrendEmptyResultsDir|TestTrendUnknownAgent|TestVerifyLogNoLogs|TestVerifyLogPhaseMissing|TestVerifyLogScoreTolerance|TestWorkspaceDefaultIsOneEmptyCommit|TestWorkspaceRelativeRootBecomesAbsolute)\\(' internal/eval/*_test.go"
      - "! grep -nE 'func (TestChmodOpenVanishedBeforeChmodIsSkipped|TestChmodOpenVanishedFileIsSkipped|TestVerifyLogDoesNotFollowRunNameOutOfResults|TestVerifyLogRejectsRunNamesOutsideResults)\\(' internal/eval/*_test.go"
      - "go test -count=1 -run '^(TestTrendMissingResultsDir|TestLoadCasesClassicTurnOneLoads|TestVerifyLogBaselineScoreToleranceBoundary|TestVerifyLogCycleNoLogsReportsOnlyNoLogs|TestVerifyLogPhaseUnknownValue|TestWorkspaceFixtureIsSingleCommitEqualToFixture|TestWorkspaceDirAbsoluteAndSymlinkResolved|TestPermWalkFuncPropagatesOtherErrors|TestVerifyLogRunNameEscapeRejected|TestChmodOpenVanishedIsSkipped)$' -v ./internal/eval"
      - "go vet ./..."
      - "test -z \"$(gofmt -l internal/eval)\""
      - "! git diff --name-only $(git merge-base HEAD origin/main) | grep -vE '^(internal/eval/(simulated_user|suite|threshold|trend|turns_schema|verifylog|workspace)_test\\.go|\\.kairon/specs/issue-427-.*\\.md|\\.kiro/agents/.*\\.json)$'"
      - "go test -count=1 -coverprofile=/tmp/kairon-427-after.out ./internal/eval && awk 'NR>1{n+=$2; if($3>0)c+=$2} END{printf \"stmts=%d covered=%d pct=%.4f\\n\", n, c, 100*c/n}' /tmp/kairon-427-after.out"
      - "go test -cover ./internal/eval | grep -E 'coverage: (87\\.[6-9]|8[89]\\.|9[0-9]\\.|100)'"
      - "task test"
```

## Risks

- **No coverage slack.** The floor equals the current figure. The 31-test `-skip` run shows no statement depends on any of them, so a
  loss can only come from a mistake in a merged table (an input never run, for example a decoy file not written or the absent-phase row
  built with the wrong empty value). The exact-count comparison catches it where the rounded 87.6% might not.
- **Wall time.** One package run takes about 340 s. `task test` adds `-race` and the other packages. The builder tasks avoid the full
  run on purpose.
- **Strengthening a row can break it.** The relative-root row, the empty-results-dir row and the `MinIterations = 0` row may behave
  differently from the original single-purpose test once extra assertions are applied. The fallback in the approach (point 3) is to keep the
  former test's own assertions. No production code may change to make an extra assertion pass.
