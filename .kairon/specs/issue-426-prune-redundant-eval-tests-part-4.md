# Design Spec: Prune redundant internal/eval tests, part 4 of 5

Closes #426

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/eval`: `repeat_test.go`, `runner_config_test.go`, `runner_test.go`, `sandbox_workspace_test.go`, `scoring_test.go`, `selftest_checks_test.go`, `selftest_judge_test.go`, `selftest_judge_transcript_test.go`, `selftest_pricing_test.go`, `selftest_test.go`, `simulated_user_schema_test.go`" | `prune-repeat-tests`, `prune-container-config-tests`, `delete-runner-and-workspace-tests`, `prune-scoring-tests`, `delete-selftest-duplicates`, `prune-loader-schema-tests`, `prune-unknown-backend-test` |
| S2 | In Scope: "`internal/eval`: `turns_schema_test.go` and `config_test.go`, which hold the merge partners `TestLoadCasesTurnsSchemaErrors`, `TestLoadCasesTurnsAndInputMutuallyExclusive` and `TestRunWithOptionsRejectsUnknownBackend`" | `prune-loader-schema-tests`, `prune-unknown-backend-test` |
| C1 | Behavior-Preservation Contract: "No production code changes. Every test not listed passes unchanged. Merged partners keep every assertion they had." | `prune-repeat-tests`, `prune-container-config-tests`, `delete-runner-and-workspace-tests`, `prune-scoring-tests`, `delete-selftest-duplicates`, `prune-loader-schema-tests`, `prune-unknown-backend-test`, `gate-coverage-and-task-test`, `validate-prune` |
| AC1 | "The 8 tests marked Delete no longer exist." | `prune-repeat-tests`, `delete-runner-and-workspace-tests`, `delete-selftest-duplicates`, `validate-prune` |
| AC2 | "Each of the 11 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-repeat-tests`, `prune-container-config-tests`, `prune-scoring-tests`, `prune-loader-schema-tests`, `prune-unknown-backend-test`, `validate-prune` |
| AC3 | "No production code changes." | `prune-repeat-tests`, `prune-container-config-tests`, `delete-runner-and-workspace-tests`, `prune-scoring-tests`, `delete-selftest-duplicates`, `prune-loader-schema-tests`, `prune-unknown-backend-test`, `gate-coverage-and-task-test`, `validate-prune` |
| AC4 | "Package coverage does not drop." | `gate-coverage-and-task-test`, `validate-prune` |
| AC5 | "Every remaining test passes unchanged, and the gate passes." | `gate-coverage-and-task-test`, `validate-prune` |

## Deferred work

none

## Baseline (recorded before any change)

Measured on this worktree at `HEAD` = `origin/main` = `3828534` (`git diff origin/main...HEAD` is empty), Go 1.26.5 darwin/arm64, with no edits made. The builder and validator compare against these numbers.

| Measurement | Command | Result |
|-------------|---------|--------|
| Package result | `go test -count=1 -cover -v ./internal/eval` | exit 0, `ok ... 341.901s`, `coverage: 87.6% of statements` |
| Test counts | same run, counting `-v` lines | 807 top-level `--- PASS`, 998 indented subtest `--- PASS`, 0 `--- FAIL`, 7 `--- SKIP` |
| Exact coverage | `go test -count=1 -coverprofile=... ./internal/eval`, then summing the profile | 4080 of 4659 statements = **87.5724%** (prints as 87.6%) |
| Worst case for the deletions | same profile run with all 19 tests skipped (`-skip '^(...19 names...)$'`) | the covered-statement set is **identical**: 4080 of 4659, `diff` of the covered blocks is empty |
| gofmt / vet | `gofmt -l internal/eval`; `go vet ./internal/eval` | no output; exit 0 |

What this means for AC4:

- The coverage margin is one statement. `go test -cover` prints 87.6% only while at least 4079 of 4659 statements are covered (87.55% rounds up). The baseline is 4080.
- Skipping all 19 tests loses no covered statement, so removing them cannot lower coverage. The merges only add inputs to partners, so they cannot lower it either. A drop can only come from a merge that accidentally loses an input the removed test exercised, for example by writing a row that does not run. The per-row negative check in the routine below guards against that.
- The 19 tests' wall time in the baseline run: `TestSelfTestChecksRunExitStatus` 7.08s, `TestSelfTestJudgeFilesReachTheJudge` 3.42s, `TestSelfTestChecksLegacyScoresUnchanged` 3.18s. The other 16 take under 0.01s each.

`task test` runs `go test -v -race -coverprofile=coverage.out ./...` for the whole module and then builds `coverage.html`. Expect it to take several minutes (CI: about 235s; `internal/eval` alone took 342s here without `-race`). Run it in the background with output redirected to a file. `coverage.out`, `coverage.html` and `internal/eval/testdata/evals/results/` are git-ignored.

## Issue vs. code: discrepancies found

All 19 test names exist, and every `file:line` in the issue's table matches the code at `3828534` exactly (60, 128, 241, 114, 229, 245, 267, 260, 103, 139, 155, 200, 318, 338, 69, 120, 210, 338, 407). The following differences between the issue's wording and the code do not change what must be delivered. None needs a human decision, so none is a BLOCKED condition.

1. **A named partner no longer exists.** The Reason column for `TestSelfTestChecksRunExitStatus` names `TestEvalSelfTestSucceedsWithDefaults` (in `cmd/kairon/cmd`). Issue #435 (PR #468, already on `main`) deleted that test. The Delete action is still correct. The exit-status behavior it checked remains asserted by tests that exist now:
   - Success path (`selftest` returns nil): `TestChecksScoringCharacterizationLegacySelftestCases` fails the test if `RunWithOptions("selftest", ...)` errors, and the many `runSelfTestAgent`/`runEvalIn` tests do the same.
   - Failure path (`selftest-fail` returns `ErrThresholdFailed`): `TestResumeRunPrintsVerdictAndReturnsThresholdError` (`exit_status_test.go:217`) asserts it on a first run, and `cmd/kairon/cmd` `TestEvalSelfTestFailExitsWithThresholdError` (`eval_test.go:427`) asserts it through the CLI.
   - The builder confirms these pass before deleting (see task `delete-selftest-duplicates`). Nothing is folded in: the deleted test had no assertion beyond `errors.Is(err, ErrThresholdFailed)` and `err == nil`.
2. **Several partners are flat tests, not tables.** The issue says "add as a row" / "one table". The merge therefore includes converting the partner to a table, or giving it subtests, so that every original assertion survives. Affected partners: `TestRepeatVerdictMinMaxMean`, `TestPrintCaseResultBreakdownOnlyBelowThreshold`, `TestScoreCaseLLMJudgeEmptyOutputSkipped`, `TestLoadCasesTurnsAndInputMutuallyExclusive`, and `TestRunWithOptionsRejectsUnknownBackend` (the last needs its assertions extended, not a table). `TestScoreCaseNilScoresWhenNoNonCostCriteria` is a table, but every row expects `Scores == nil`, so it needs an expected-score-names field. `TestCreateContainerConfig_WithSandboxConfig` is a table with no `debug` field, so it needs one. `TestLoadCasesTurnsSchemaErrors` is a table with a single `want` substring per row, so it needs an `also` field to carry the three substrings of the merged test.
3. **Two Delete partners live in files that the In Scope list does not name.** `TestAgentVerdictCasesTotalAndFailed` is in `verdict_test.go`, and `TestChecksScoringCharacterizationLegacySelftestCases` is in `checks_scoring_test.go`. Both receive no edits, because the Delete tests add nothing to them. The other Delete partners (`TestSelfTestJudgePassingCases`, `TestSelftestPriceSonnetIsPricedPerModel`) are in in-scope files and are also unedited.
4. **Deleting `TestScoreLLMJudge_ANSISequences` leaves two unused imports.** In `runner_test.go`, `encoding/json` and `strings` are used only by that test. They must be removed or the package will not compile. No other deletion or merge leaves an unused import or helper (checked below).
5. **`wantThresholdFailure` stays.** It is called by `runEvalIn` (`sandbox_workspace_test.go:63`). Only `TestWantThresholdFailure` and its doc comment go.
6. **The issue's coverage threshold is a rounded figure.** See Baseline: the real margin is one statement.

## Test-to-Task Map

Every row of the issue's Current / Target State table, so the 8 Deletes and 11 Merges can be checked one by one. Lines are at `3828534`.

| Test (file:line) | Action | Partner | Delivered by |
|------------------|--------|---------|--------------|
| `TestRepeatVerdictLegacyCaseAccountingUnchanged` (repeat_test.go:60) | Delete | `TestAgentVerdictCasesTotalAndFailed` (verdict_test.go:65) | `prune-repeat-tests` |
| `TestRepeatVerdictMeanEqualsMeanOfRepeatScores` (repeat_test.go:128) | Merge | `TestRepeatVerdictMinMaxMean` (repeat_test.go:72) | `prune-repeat-tests` |
| `TestRepeatVerdictResultFieldsOmitEmpty` (repeat_test.go:241) | Delete | none | `prune-repeat-tests` |
| `TestCreateContainerConfig_NilSandboxConfig` (runner_config_test.go:114) | Merge | `TestCreateContainerConfig_WithSandboxConfig` (:11) | `prune-container-config-tests` |
| `TestCreateContainerConfig_EmptyStringValues` (runner_config_test.go:229) | Merge | `TestCreateContainerConfig_WithSandboxConfig` | `prune-container-config-tests` |
| `TestCreateContainerConfig_ZeroValues` (runner_config_test.go:245) | Merge | `TestCreateContainerConfig_WithSandboxConfig` | `prune-container-config-tests` |
| `TestCreateContainerConfig_DebugFlag` (runner_config_test.go:267) | Merge | `TestCreateContainerConfig_WithSandboxConfig` | `prune-container-config-tests` |
| `TestScoreLLMJudge_ANSISequences` (runner_test.go:260) | Delete | none | `delete-runner-and-workspace-tests` |
| `TestWantThresholdFailure` (sandbox_workspace_test.go:103) | Delete | none | `delete-runner-and-workspace-tests` |
| `TestPrintCaseResultNoBreakdownWhenPassing` (scoring_test.go:139) | Merge | `TestPrintCaseResultBreakdownOnlyBelowThreshold` (:113) | `prune-scoring-tests` |
| `TestScoreCaseSkipsCostCriterion` (scoring_test.go:155) | Merge | `TestScoreCaseNilScoresWhenNoNonCostCriteria` (:272) | `prune-scoring-tests` |
| `TestScoreCaseDeterministicEmptyOutputSkipped` (scoring_test.go:200) | Merge | `TestScoreCaseLLMJudgeEmptyOutputSkipped` (:215) | `prune-scoring-tests` |
| `TestSelfTestChecksLegacyScoresUnchanged` (selftest_checks_test.go:318) | Delete | `TestChecksScoringCharacterizationLegacySelftestCases` (checks_scoring_test.go:15) | `delete-selftest-duplicates` |
| `TestSelfTestChecksRunExitStatus` (selftest_checks_test.go:338) | Delete | none that exists (see discrepancy 1) | `delete-selftest-duplicates` |
| `TestSelfTestJudgeFilesReachTheJudge` (selftest_judge_test.go:69) | Delete | `TestSelfTestJudgePassingCases` (:36) | `delete-selftest-duplicates` |
| `TestLoadCasesJudgeTurnBeyondCaseTurns` (selftest_judge_transcript_test.go:120) | Merge | `TestLoadCasesTurnsSchemaErrors` (turns_schema_test.go:65) | `prune-loader-schema-tests` |
| `TestSelftestPriceFixturesDefineStubUsage` (selftest_pricing_test.go:210) | Delete | `TestSelftestPriceSonnetIsPricedPerModel` (:113) | `delete-selftest-duplicates` |
| `TestSelfTestRejectsUnknownBackend` (selftest_test.go:338) | Merge | `TestRunWithOptionsRejectsUnknownBackend` (config_test.go:116) | `prune-unknown-backend-test` |
| `TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser` (simulated_user_schema_test.go:407) | Merge | `TestLoadCasesTurnsAndInputMutuallyExclusive` (turns_schema_test.go:24) | `prune-loader-schema-tests` |

## Solution Approach

This is a test-only refactor of `internal/eval`. No production file changes. The 19 tests in the issue table are either deleted (8) or folded into the named partner as a table row, subtest or extra assertion (11).

**Characterization first.** AGENTS.md exempts only non-code changes from "test first", and this is a refactor of test code, so the refactor rules apply: existing tests are the characterization, and they must pass before and after. The Baseline section records the "before". Each task re-runs the tests it touches before editing (the removed tests and their partners) and after editing (the partners, with the new subtest names visible as `--- PASS`). The full package run in `gate-coverage-and-task-test` is the final "after".

**Per-task routine (every builder task).**

1. **Before editing**, run the tests being removed and their partners with `-count=1 -v` and see them pass. Paste the `--- PASS` lines into the sentinel as the "before" evidence.
2. **Fold** the removed test's distinct input or assertion into the partner. Drop only what the issue says the partner already asserts (listed per test below). Keep every assertion the partner had. Do not weaken, loosen, rename or delete any partner assertion, and do not rename any partner function (the AC2 check runs `-run '^<partner>$'`). Keep the removed test's explanatory comment by carrying it onto the new row or subtest.
3. **Delete** the removed function together with its doc comment. Remove any import or helper that becomes unused (the build must pass). The per-task notes name the ones known to be affected.
4. **Run the partners** with `-count=1 -v` and confirm each prescribed subtest name appears as `--- PASS`. `t.Run` turns spaces into underscores, so a row named `nil sandbox config` appears as `nil_sandbox_config`.
5. **Negative check for each new or converted row** (merges only). Temporarily change one expected value in the new row, run the partner, and confirm that this row fails with its assertion message (not a compile error and not a different row). Then revert the change. This proves the folded input actually runs and still asserts. `git diff` afterwards must show only the intended edit.
6. Run `go vet ./internal/eval` and `gofmt -l internal/eval` (no output).
7. **Commit** with an explicit `git add` of only the test files the task touched. The worktree has uncommitted modifications to `.kiro/agents/*.json` that are not part of this change. Staging them would break AC3 (`git diff --name-only origin/main...HEAD` lists only `_test.go` files). Use the Conventional Commits form `test(eval): <subject>`, and put the tests removed, the partner that now carries them, and the observed before/after results in the body (AGENTS.md "Say what you observed"). Do not push.

**Assertion-preservation rule for merges.** A converted partner must still make each of its original assertions on its original input. When a flat test becomes a table, its original body becomes one row (or one subtest) with identical inputs and expected values. Shared code moves into the loop only if every row can use it unchanged.

**No spec deviations are planned.** If a builder finds that an instruction below does not work against the real code, it records the deviation per AGENTS.md "No silent deviation": a code comment at the point of departure that cites the section of this spec, and a `Spec deviations` entry in the sentinel. Following the spec's instructions exactly is not a deviation.

## Relevant Files

All under `internal/eval/`, all `_test.go`. Nothing else is modified.

| File | Change |
|------|--------|
| `repeat_test.go` | delete `TestRepeatVerdictLegacyCaseAccountingUnchanged`, `TestRepeatVerdictResultFieldsOmitEmpty`; fold `TestRepeatVerdictMeanEqualsMeanOfRepeatScores` into `TestRepeatVerdictMinMaxMean`. Imports unchanged (`math` stays in use by the partner; `bytes`, `encoding/json`, `reflect`, `strings` are used by other tests) |
| `runner_config_test.go` | `TestCreateContainerConfig_WithSandboxConfig` gains a `debug` field and 4 rows; 4 functions deleted |
| `runner_test.go` | delete `TestScoreLLMJudge_ANSISequences`; remove imports `encoding/json` and `strings` |
| `sandbox_workspace_test.go` | delete `TestWantThresholdFailure` and its doc comment; keep `wantThresholdFailure` |
| `scoring_test.go` | 3 partners converted to tables or subtests; 3 functions deleted. Imports unchanged |
| `selftest_checks_test.go` | delete `TestSelfTestChecksLegacyScoresUnchanged`, `TestSelfTestChecksRunExitStatus` and their doc comments. Imports unchanged (`errors`, `fmt`, `os`, `path/filepath`, `strings` are used elsewhere in the file) |
| `selftest_judge_test.go` | delete `TestSelfTestJudgeFilesReachTheJudge` |
| `selftest_judge_transcript_test.go` | delete `TestLoadCasesJudgeTurnBeyondCaseTurns` and its doc comment. Imports unchanged (`strings` is used by `judgePrompts`) |
| `selftest_pricing_test.go` | delete `TestSelftestPriceFixturesDefineStubUsage` and its doc comment. `loadSelftestStub` (in `container_agent_test.go`) and the `priceSonnetCase`/`priceUnknownCase` constants stay in use elsewhere |
| `selftest_test.go` | delete `TestSelfTestRejectsUnknownBackend`. Imports unchanged |
| `simulated_user_schema_test.go` | delete `TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser`. Imports unchanged (`strings` is used elsewhere) |
| `turns_schema_test.go` | `TestLoadCasesTurnsSchemaErrors` gains an `also` field and a row; `TestLoadCasesTurnsAndInputMutuallyExclusive` becomes a two-row table |
| `config_test.go` | `TestRunWithOptionsRejectsUnknownBackend` takes the stronger assertions. Needed helpers and imports are already present (`strings`, `path/filepath`, and package helpers `installFakeKiroCLI`, `registerResultsCleanup`, `snapshotDirs`, `newRunDirs`, `readCalls`, `selftestEvalsDir`) |

Read-only references (no edit): `verdict_test.go` (`TestAgentVerdictCasesTotalAndFailed`), `checks_scoring_test.go` (`TestChecksScoringCharacterizationLegacySelftestCases`), `exit_status_test.go`, `container_agent_test.go` (`loadSelftestStub`), `cmd/kairon/cmd/eval_test.go`, `Taskfile.yml` (`task test` = `go test -v -race -coverprofile=coverage.out ./...`). No live doc, README or code comment outside the deleted functions' own doc comments mentions any of the 19 names (checked with grep over `internal`, `cmd`, `docs`, `README.md`, `CONTRIBUTING.md`, `.kiro`, `Taskfile.yml`), so no documentation change is needed. Historical files under `.kairon/specs/`, `.kairon/research/` and `.kairon/evals/results/` mention some names and must not be edited (AGENTS.md).

## Team Orchestration

- All implementation tasks are `builder` tasks that run **strictly in sequence**. They edit different files, but all builders share one worktree and one Go package. A half-edited test file breaks the package build for every other task, so no two builders run at once.
- Within a task, every edit to one file is done by one builder. The partner edits that share a file are in one task: all four `TestCreateContainerConfig_*` merges are in `prune-container-config-tests`, and both loader-table partners (`TestLoadCasesTurnsSchemaErrors`, `TestLoadCasesTurnsAndInputMutuallyExclusive`, both in `turns_schema_test.go`) are in `prune-loader-schema-tests`.
- Task order puts the cheap, independent edits first and the tasks that run slow selftest suites late: `prune-repeat-tests`, `prune-container-config-tests`, `delete-runner-and-workspace-tests`, `prune-scoring-tests`, `prune-loader-schema-tests`, `prune-unknown-backend-test`, `delete-selftest-duplicates`.
- `gate-coverage-and-task-test` (builder, because it may need to restore a lost input as a table row) runs after all pruning tasks. `validate-prune` (validator, read-only) then re-verifies everything without trusting builder sentinels.
- No documenter task: no user-facing behavior or live doc changes.

## Step-by-Step Task Breakdown

### 1. `prune-repeat-tests` (repeat_test.go)

- **Delete `TestRepeatVerdictLegacyCaseAccountingUnchanged`** (line 60). It builds three cases (`ok` 5/5 legacy, `stamped-fail` 5/5 with `Passed=false`, `legacy-fail` 4/5 legacy) with agent threshold 0 and asserts `CasesTotal == 3` and `CasesFailed == 2`. `TestAgentVerdictCasesTotalAndFailed` (`verdict_test.go:65`) builds the same classes (stamped fail, legacy fail 4/5 under the default 95% fallback, legacy pass) and asserts `CasesTotal` and `CasesFailed`, plus a threshold-80 recompute. Nothing here is distinct. Do not edit `verdict_test.go`. Before deleting, run both and confirm the partner passes.
- **Delete `TestRepeatVerdictResultFieldsOmitEmpty`** (line 241). It asserts JSON `omitempty` on `AgentResult.Repeat` and `CaseResult.Run` (absent at 0) and the key spelling `"repeat":3` and `"run":2` when set. The issue's verdict is Delete with no partner: every reader decodes an absent field as 0. `TestRepeatVerdictJSONFieldsAlwaysWritten` (just above it) keeps the `AgentVerdict` key assertions. Nothing is folded in.
- **Merge `TestRepeatVerdictMeanEqualsMeanOfRepeatScores`** (line 128) into `TestRepeatVerdictMinMaxMean` (line 72). The partner is a flat test, so give it two subtests (a table with a per-row `check func(t *testing.T, v AgentVerdict)` is equally fine):
  - `min-max-mean`: the partner's current body, unchanged (threshold 85, two repeats of one case scoring 20/20 and 16/20; asserts `Score == 90`, `ScoreMin == 80`, `ScoreMax == 100`, `RepeatScores` deep-equals `[100 80]`, `Repeat == 2`, `Passed`).
  - `mean-of-repeat-scores`: the removed test's body unchanged: threshold 50, `Repeat: 3`, cases `a` (7/9, 3/9, 9/9) and `b` (1/3, 2/3, 0/3) across runs 1 to 3. It fails with `t.Fatalf` unless `len(v.RepeatScores) == 3`, then asserts `math.Abs(v.Score - sum/3) <= 1e-9`, where `sum` is the sum of `v.RepeatScores`.
  - Keep `math` in the import block (the new subtest is its only user). The removed test's distinct input is the 3-repeat, 2-case fixture. The partner already asserts mean = 90 for two repeats, so only the three-repeat property is new.

### 2. `prune-container-config-tests` (runner_config_test.go)

All four tests call `createContainerConfig(sandboxCfg, resourceLimits, debug)` (`runner.go:808`). Fold them into the `tests` table of `TestCreateContainerConfig_WithSandboxConfig` (line 11):

- Add a `debug bool` field to the row struct. In the loop, call `createContainerConfig(tt.sandboxCfg, tt.resourceLimits, tt.debug)` and add `if result.Debug != tt.debug { t.Errorf("Debug = %v, expected %v", result.Debug, tt.debug) }`. The three existing rows leave `debug` false, so they now also assert `Debug == false`, which is the second half of `TestCreateContainerConfig_DebugFlag`.
- Add four rows. Each row's `expectConfig` has `Environment: {"KIRO_CLI_DISABLE_TELEMETRY": "1"}` (the loop already asserts it), `Platform` left empty (the loop only asserts it is non-empty), and:

| Row name (subtest path) | `sandboxCfg` | `debug` | Expected |
|-------------------------|--------------|---------|----------|
| `nil sandbox config` (`nil_sandbox_config`) | `nil` | false | `WorkspaceDir "/workspace"`, `CPUQuota 1000000`, `Memory 1024*1024*1024`, `Timeout 5*time.Minute` |
| `empty workspace dir` (`empty_workspace_dir`) | `{WorkspaceDir: "", CPUCores: 2.0, MemoryMB: 1024, Timeout: 5*time.Minute}` | false | `WorkspaceDir "/workspace"` (the default), `CPUQuota 2000000`, `Memory 1024*1024*1024`, `Timeout 5*time.Minute` |
| `zero cpu memory timeout` (`zero_cpu_memory_timeout`) | `{WorkspaceDir: "/custom", CPUCores: 0, MemoryMB: 0, Timeout: 0}` | false | `WorkspaceDir "/custom"`, `CPUQuota 1000000`, `Memory 1024*1024*1024`, `Timeout 5*time.Minute` (zero values fall back to defaults) |
| `debug enabled` (`debug_enabled`) | `nil` | true | same defaults as `nil sandbox config`, plus `Debug` true via the new field |

- The removed tests asserted only a subset of those fields (`EmptyStringValues` only `WorkspaceDir`; `ZeroValues` not `WorkspaceDir`). The table asserts all fields, which the production code satisfies, so each row is at least as strong as the test it replaces. Carry the "falls back to defaults" comments from the removed tests onto the rows.
- Delete `TestCreateContainerConfig_NilSandboxConfig` (line 114), `TestCreateContainerConfig_EmptyStringValues` (229), `TestCreateContainerConfig_ZeroValues` (245) and `TestCreateContainerConfig_DebugFlag` (267). Leave `TestCreateContainerConfig_ResourceLimitOverrides` untouched.

### 3. `delete-runner-and-workspace-tests` (runner_test.go, sandbox_workspace_test.go)

- **Delete `TestScoreLLMJudge_ANSISequences`** (runner_test.go:260 to 325). The test never calls `scoreLLMJudge`. It extracts the JSON itself with `strings.Index`, then calls `stripANSISequences` and `json.Unmarshal` on the result, so it checks a library and a one-line alias. Delete the whole function. **Remove the two imports `encoding/json` and `strings` from `runner_test.go`**: both are used only by this test (verified: the remaining file uses `os`, `path/filepath` and `testing`). `go vet` fails with "imported and not used" if they stay. `stripANSISequences` remains exercised by production paths (`runner.go:1671`, `simulated_user.go:198`), and the Baseline shows the covered-statement set does not change when this test is skipped.
- **Delete `TestWantThresholdFailure`** (sandbox_workspace_test.go:103 to 131, with its doc comment starting at line 95). It restates the `switch` of the helper `wantThresholdFailure` (line 85). **Keep the helper and its own doc comment (lines 77 to 84)**: `runEvalIn` calls it at line 63, and many tests depend on `runEvalIn`. Imports in this file are all used elsewhere, so none changes (confirmed by counting uses outside the deleted range).

### 4. `prune-scoring-tests` (scoring_test.go)

- **Merge `TestPrintCaseResultNoBreakdownWhenPassing`** (line 139) into `TestPrintCaseResultBreakdownOnlyBelowThreshold` (line 113). Convert the partner to a table with fields `name`, `rubric Rubric`, `scores []CriterionScore`, `want string`, `absent []string`. Both rows build `cr := CaseResult{ActualOutput: "out", Scores: scores}` and call `printCaseResult(&sb, TestCase{Name: "c"}, tt.rubric, cr)`.
  - Row `below-threshold-lists-weak-criteria`: `Rubric{}`, the partner's five scores and their per-line comments, `want` = `" ❌ 44% (threshold: 95%)\n      low: 1/5\n      skipped-low: 0/5\n      just-under: 2/5\n"`, `absent` = `{"edge", "high"}`. Keep the exact-match `t.Fatalf` first, then the loop over `absent` that fails if the output contains one of them.
  - Row `passing-omits-breakdown`: `Rubric{PassThreshold: floatPtr(80)}`, scores `weak 2/5`, `strong 5/5`, `strong2 5/5`, `strong3 5/5`, `want` = `" ✅ 85% (threshold: 80%)\n"`, `absent` empty. Keep the comment that 17/20 = 85% passes although `weak` is below 3/4 of its max.
- **Merge `TestScoreCaseSkipsCostCriterion`** (line 155) into `TestScoreCaseNilScoresWhenNoNonCostCriteria` (line 272). Keep `chdirTemp(t)` and `useStubBackend(t)` at the top. Replace the `map[string]Rubric` with a table of `name`, `rubric`, `output string`, `wantNames []string`; `wantNames == nil` means `cr.Scores` must be nil.
  - Rows `empty rubric` and `cost only`: unchanged rubrics, `output: "out"`, `wantNames` nil. They keep the assertion `cr.Scores != nil` fails with `Scores = %#v, want nil`.
  - New row `cost and deterministic` (subtest `cost_and_deterministic`): rubric `{Agent: "a", Criteria: [{cost_budget, "1-5", Type "cost"}, {structural_completeness, "1-5", Deterministic true}]}`, `output: "## H\n### S\ntext"` (the removed test's own output, not `"out"`), `wantNames: {"structural_completeness"}`. It asserts `len(cr.Scores) == 1` (`t.Fatalf`, message mentioning the skipped cost criterion) and `cr.Scores[0].Name == "structural_completeness"`.
  - Keep the function name, because the AC2 check runs `-run '^TestScoreCaseNilScoresWhenNoNonCostCriteria$'`.
- **Merge `TestScoreCaseDeterministicEmptyOutputSkipped`** (line 200) into `TestScoreCaseLLMJudgeEmptyOutputSkipped` (line 215). Keep `chdirTemp`, `installFakeKiroCLI(t, "")` and `useStubBackend` at the top. Two subtests (or table rows), each building `cr := CaseResult{}` and calling `scoreCase(rubric, TestCase{Name: "c"}, &cr)`:
  - `llm-judge`: the partner's current body (rubric `clarity` `"1-7"`, deep-equal against the single skipped score with reasoning `no output available for LLM judging`, `JudgeCost` zero, `readCalls` empty). Unchanged assertions.
  - `deterministic`: rubric `{Name: "structural_completeness", Scoring: "1-5", Deterministic: true}`. Assert exactly the removed test's checks: `len(cr.Scores) == 1`, `cr.Scores[0].Skipped`, and `cr.Scores[0].Score == 0` (message: want one skipped zero score). Add nothing else. The deterministic row asserts less than the LLM row on purpose, because the removed test did.
  - Run `llm-judge` first. The fake `kiro-cli` call log is shared, so its `readCalls` check must run in that subtest.
- Delete the three merged functions. `reflect`, `strings`, `testing` and `inference` imports stay in use.

### 5. `prune-loader-schema-tests` (turns_schema_test.go, selftest_judge_transcript_test.go, simulated_user_schema_test.go)

- **Merge `TestLoadCasesJudgeTurnBeyondCaseTurns`** (selftest_judge_transcript_test.go:120) into `TestLoadCasesTurnsSchemaErrors` (turns_schema_test.go:65). The partner's row struct has one `want` substring, and its loop already asserts the error names the case (`tt.name`) and contains `tt.want`. The removed test asserts three substrings: `judge-turn-9`, `check 1 (judge)` and `turn 9 is beyond the case's 2 turn(s)`.
  - Add an optional field `also []string` to the row struct, and a loop after the existing assertions: for each `s` in `tt.also`, `t.Errorf("error %q does not contain %q", err, s)` if the error lacks it. Existing rows leave it empty.
  - Add row `name: "judge-turn-9"`, `want: "turn 9 is beyond the case's 2 turn(s)"`, `also: {"check 1 (judge)"}`, with the removed test's YAML verbatim (`name: judge-turn-9`, `agent: a1`, turns `one`, `two`, and the check `{criterion: c, type: judge, question: q, turn: 9}`). The case name `judge-turn-9` is covered by the existing `tt.name` assertion. Carry the removed test's comment ("A judge check whose turn is beyond the case's turns never loads, so a case file cannot exercise the run-time guard; the loader names the turn count") onto the row.
- **Delete `TestLoadCasesJudgeTurnBeyondCaseTurns`** and its doc comment. `strings` stays in use in that file (`judgePrompts`).
- **Merge `TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser`** (simulated_user_schema_test.go:407) into `TestLoadCasesTurnsAndInputMutuallyExclusive` (turns_schema_test.go:24). Convert the partner to a table of `{name, yaml}` rows whose names equal the YAML case names. The loop keeps all original assertions per row: `loadTurnsCase` returns an error (`t.Fatal` otherwise), and the error contains the case name and `turns and input are mutually exclusive`.
  - Row `both-forms`: the partner's current YAML (`input: hello`, turns `one`, `two`).
  - Row `no-sim-both`: the removed test's YAML, `"name: no-sim-both\nagent: a1\ninput: hello\nturns:\n  - one\n"` (a single-entry `turns` list). That input is the removed test's distinct contribution. The case-name assertion is slightly stricter than the removed test's (it asserted only the message), which the loader satisfies because it prefixes the case name. If this somehow fails, record it as a deviation and keep the message-only check for that row.
- **Delete `TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser`**. `strings` stays in use in that file.

### 6. `prune-unknown-backend-test` (config_test.go, selftest_test.go)

- **Merge `TestSelfTestRejectsUnknownBackend`** (selftest_test.go:338 to 361) into `TestRunWithOptionsRejectsUnknownBackend` (config_test.go:116). The partner currently calls `RunWithOptions("a", "", RunOptions{Backend: "nope"})` and asserts only that the error contains `valid backends`. The removed test is stronger. Rewrite the partner body to:
  1. `t.Cleanup(resetConfig)`; `_, calls := installFakeKiroCLI(t, "")`.
  2. `resultsDir := filepath.Join(selftestEvalsDir, "results")`; `registerResultsCleanup(t, resultsDir)`; `before := snapshotDirs(t, resultsDir)`.
  3. `err := RunWithOptions("selftest", "", RunOptions{Backend: "nope", EvalsDir: selftestEvalsDir})`; `t.Fatal` if `err == nil`.
  4. Require the error to contain each of `valid backends` (the partner's original assertion, kept), `"nope"` with its quotes, `kiro-cli` and `stub` (the production message is `unknown backend "nope"; valid backends: kiro-cli, stub`, from `inference.go:251`).
  5. `newRunDirs(t, resultsDir, before)` must be empty (a rejected run creates no result directory).
  6. `readCalls(t, calls)` must be empty (the fake `kiro-cli` was never invoked).
- **Reasoning for the input change** (agent `"a"` and default evals dir become `"selftest"` and `selftestEvalsDir`): the backend is rejected inside `configure()` (`config.go:80` to `87`), which `RunWithOptions` calls (`runner.go:67`) before any agent or evals directory is read, so the original input offers no distinct path. The result-directory assertion needs a known evals directory, which is why the removed test used `selftestEvalsDir`. This follows the issue's instruction to fold the stronger assertions "into the config_test version" and is not a deviation.
- **Delete `TestSelfTestRejectsUnknownBackend`**. All its helpers are used elsewhere in `selftest_test.go`, and its imports stay in use.
- The partner remains one flat test (no subtests), because the issue defines this merge as moving assertions into the partner's body. The validator checks the merge by reading the function and by the grep in the task's validation commands.

### 7. `delete-selftest-duplicates` (selftest_checks_test.go, selftest_judge_test.go, selftest_pricing_test.go)

All four are plain deletes: nothing needs folding. For each, the builder reads the partner and confirms the claim before deleting.

- **Delete `TestSelfTestChecksLegacyScoresUnchanged`** (selftest_checks_test.go:318) and its doc comment (lines 316 to 317). It runs `selftest` and, for the five cases `stub-basic`, `stub-usage`, `stub-quoted-input`, `markerCase`, `seededCase`, asserts two scores: `structural_completeness` (5/5, reasoning `found 2/2 expected structural elements`, `Checks == nil`) and `clarity` (5/5, `Checks == nil`). `TestChecksScoringCharacterizationLegacySelftestCases` (`checks_scoring_test.go:15`) runs the same agent, loops over the same five cases and asserts the same fields plus `Deterministic` and `Skipped`, so it is a strict superset.
- **Delete `TestSelfTestChecksRunExitStatus`** (selftest_checks_test.go:338) and its doc comment (lines 336 to 337). Its named partner no longer exists (discrepancy 1). Before deleting, run `TestResumeRunPrintsVerdictAndReturnsThresholdError` and `TestChecksScoringCharacterizationLegacySelftestCases` in `internal/eval`, and `go test -count=1 -run '^TestEvalSelfTestFailExitsWithThresholdError$' ./cmd/kairon/cmd`, and confirm all pass. Record that in the sentinel as the replacement evidence for both the `selftest` (nil error) and `selftest-fail` (`ErrThresholdFailed`) outcomes. If any of them fails or is missing, stop and report `**Status**: BLOCKED` instead of deleting. Imports `errors`, `fmt`, `os`, `path/filepath`, `strings` stay in use elsewhere in the file (checked), as do `copyFixturesTo` and `resetConfig`.
- **Delete `TestSelfTestJudgeFilesReachTheJudge`** (selftest_judge_test.go:69). It asserts that `check-judge-files` has exactly one judge check that passed and exactly one judge call. `TestSelfTestJudgePassingCases` loops over `check-judge-files` (with `check-judge-yes` and `check-judge-mixed`) and asserts `len(judgeChecks) == 1`, the check passed with the stub's reasoning, and `len(judgeCalls) == 1` with no error. Nothing is distinct.
- **Delete `TestSelftestPriceFixturesDefineStubUsage`** (selftest_pricing_test.go:210) and its doc comment (lines 208 to 209). It asserts the fixtures' stub model and usage (`claude-sonnet-5.5` 1000/500 and `claude-haiku-4.5` 1000/500). `TestSelftestPriceSonnetIsPricedPerModel` asserts `call.Model == "claude-sonnet-5.5"` with 1000/500 tokens from the run output, and `TestSelftestPriceUnknownMarksCostUnknown` asserts the haiku model with 1000/500, so the run tests already assert the fixture content. `loadSelftestStub` stays in use in `container_agent_test.go`, and `priceSonnetCase` and `priceUnknownCase` stay in use in the pricing tests.

### 8. `gate-coverage-and-task-test` (builder)

- Run `go test -count=1 -coverprofile=/tmp/eval.cov ./internal/eval` and confirm it passes with `go tool cover -func=/tmp/eval.cov` total **at least 87.6%** (AC4). For a precise figure, sum the profile (covered statements over total). The baseline is 4080 of 4659 (87.5724%), and 4079 is the minimum that still prints 87.6%.
- Compare the test counts with the Baseline. Top-level `--- PASS` falls from 807 by 19 (8 deleted functions plus 11 merged functions) to **788**. `--- FAIL` is 0 and `--- SKIP` stays at 7. The indented subtest count rises because the partners gain subtests, and the exact figure is not asserted. Record the actual numbers in the sentinel and explain any difference from 788.
- Run `task fmt:check`, `task lint` (`go vet ./...`) and `task test` and confirm each exits 0 (AC5). `task test` runs the whole module with `-race`, so it also shows that nothing outside `internal/eval` depended on a removed test or helper.
- Confirm AC3: `git diff --name-only origin/main...HEAD` is non-empty and every line ends in `_test.go`, and none of them is under `.kiro/`.
- If coverage fell below 87.6%: find which merged test carried a statement nothing else covers (compare `go test -coverprofile` before and after, against the baseline profile method above) and restore that input as a table row in the relevant partner. Do not touch production code and do not delete assertions. The pre-design measurement says this should not happen: skipping all 19 tests leaves the covered set unchanged.
- Make no other change if all checks pass.

### 9. `validate-prune` (validator, read-only)

Independently re-verify against the issue, without trusting builder sentinels:

- AC1: the Delete grep prints nothing.
- AC2: the Merge grep prints nothing. Each partner passes with `-count=1 -v`, and each prescribed subtest name below appears as `--- PASS`. For `TestRunWithOptionsRejectsUnknownBackend`, read the function and confirm it holds all of the extra assertions (the three names, the result-directory check and the `kiro-cli` call check) as well as `valid backends`.
- Contract C1 and AC3: only `_test.go` files changed. For every converted partner, compare with `git show origin/main:internal/eval/<file>` and confirm that no original assertion was removed or weakened. The only assertions that may disappear are those this spec lists as covered elsewhere (the Delete tests' assertions).
- AC4 and AC5: coverage is at least 87.6% and `task test` exits 0.
- AGENTS.md rules: no deferral wording in the diff or the PR text, no unrecorded spec deviation, and any deviation recorded has a code comment citing this spec.

Prescribed subtests (`go test -run '^<partner>$' -v ./internal/eval`):

| Partner | Subtests that must appear as PASS |
|---------|-----------------------------------|
| `TestRepeatVerdictMinMaxMean` | `min-max-mean`, `mean-of-repeat-scores` |
| `TestCreateContainerConfig_WithSandboxConfig` | `complete_sandbox_config`, `partial_sandbox_config`, `minimal_sandbox_config`, `nil_sandbox_config`, `empty_workspace_dir`, `zero_cpu_memory_timeout`, `debug_enabled` |
| `TestPrintCaseResultBreakdownOnlyBelowThreshold` | `below-threshold-lists-weak-criteria`, `passing-omits-breakdown` |
| `TestScoreCaseNilScoresWhenNoNonCostCriteria` | `empty_rubric`, `cost_only`, `cost_and_deterministic` |
| `TestScoreCaseLLMJudgeEmptyOutputSkipped` | `llm-judge`, `deterministic` |
| `TestLoadCasesTurnsSchemaErrors` | `empty-turns`, `blank-turn`, `stub-count-mismatch`, `check-turn-too-high`, `classic-turn-2`, `judge-turn-9` |
| `TestLoadCasesTurnsAndInputMutuallyExclusive` | `both-forms`, `no-sim-both` |

## Validation Commands

```bash
# AC1: the 8 deleted tests are gone (prints nothing)
grep -nE 'func (TestRepeatVerdictLegacyCaseAccountingUnchanged|TestRepeatVerdictResultFieldsOmitEmpty|TestScoreLLMJudge_ANSISequences|TestSelfTestChecksLegacyScoresUnchanged|TestSelfTestChecksRunExitStatus|TestSelfTestJudgeFilesReachTheJudge|TestSelftestPriceFixturesDefineStubUsage|TestWantThresholdFailure)\(' internal/eval/*_test.go

# AC2: the 11 merged tests are gone (prints nothing)
grep -nE 'func (TestCreateContainerConfig_DebugFlag|TestCreateContainerConfig_EmptyStringValues|TestCreateContainerConfig_NilSandboxConfig|TestCreateContainerConfig_ZeroValues|TestLoadCasesJudgeTurnBeyondCaseTurns|TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser|TestPrintCaseResultNoBreakdownWhenPassing|TestRepeatVerdictMeanEqualsMeanOfRepeatScores|TestScoreCaseDeterministicEmptyOutputSkipped|TestScoreCaseSkipsCostCriterion|TestSelfTestRejectsUnknownBackend)\(' internal/eval/*_test.go

# AC2: every partner passes with the merged case among its subtests
go test -count=1 -v -run '^(TestRepeatVerdictMinMaxMean|TestCreateContainerConfig_WithSandboxConfig|TestPrintCaseResultBreakdownOnlyBelowThreshold|TestScoreCaseNilScoresWhenNoNonCostCriteria|TestScoreCaseLLMJudgeEmptyOutputSkipped|TestLoadCasesTurnsSchemaErrors|TestLoadCasesTurnsAndInputMutuallyExclusive|TestRunWithOptionsRejectsUnknownBackend)$' ./internal/eval

# helper that must survive the TestWantThresholdFailure delete
grep -n 'func wantThresholdFailure' internal/eval/sandbox_workspace_test.go

# AC3: only test files changed
test -n "$(git diff --name-only origin/main...HEAD)" && test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

# AC4: coverage is at least 87.6%
go test -count=1 -coverprofile=/tmp/eval.cov ./internal/eval && go tool cover -func=/tmp/eval.cov | awk '/^total:/ { v=$NF; sub("%","",v); if (v+0 < 87.6) { print "coverage " v " < 87.6"; exit 1 } }'

# AC5: the gate
task fmt:check && task lint && task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: prune-repeat-tests
    agent: builder
    description: >-
      repeat_test.go: delete TestRepeatVerdictLegacyCaseAccountingUnchanged (subset of TestAgentVerdictCasesTotalAndFailed in verdict_test.go, which
      is not edited) and TestRepeatVerdictResultFieldsOmitEmpty (no partner). Fold TestRepeatVerdictMeanEqualsMeanOfRepeatScores into
      TestRepeatVerdictMinMaxMean by giving the partner subtests min-max-mean (its current body unchanged) and mean-of-repeat-scores (the removed
      test's body unchanged: threshold 50, Repeat 3, cases a and b over three runs, Fatalf unless 3 RepeatScores, then
      abs(Score - sum/3) <= 1e-9). Keep the math import. Follow the per-task routine in the spec (before/after runs, negative check on the new
      subtest, vet, gofmt, explicit git add of repeat_test.go only, test(eval) commit, no push).
    dependencies: []
    acceptance_criteria:
      - "Before editing, TestRepeatVerdictLegacyCaseAccountingUnchanged, TestRepeatVerdictMeanEqualsMeanOfRepeatScores, TestRepeatVerdictResultFieldsOmitEmpty, TestRepeatVerdictMinMaxMean and TestAgentVerdictCasesTotalAndFailed were run with -v and passed (evidence in the sentinel)"
      - "func TestRepeatVerdictLegacyCaseAccountingUnchanged, TestRepeatVerdictResultFieldsOmitEmpty and TestRepeatVerdictMeanEqualsMeanOfRepeatScores no longer exist in internal/eval/*_test.go"
      - "TestRepeatVerdictMinMaxMean has passing subtests min-max-mean and mean-of-repeat-scores"
      - "min-max-mean keeps every original assertion: Score 90, ScoreMin 80, ScoreMax 100, RepeatScores [100 80], Repeat 2, Passed true"
      - "mean-of-repeat-scores keeps the 3-repeat fixture, the 3-entry RepeatScores check and the 1e-9 mean-of-repeat-scores assertion"
      - "Negative check done: changing an expected value in mean-of-repeat-scores makes that subtest fail for the assertion message, then it was reverted"
      - "verdict_test.go is unchanged; only repeat_test.go is modified and committed; go vet ./internal/eval and gofmt -l internal/eval are clean"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestRepeatVerdictLegacyCaseAccountingUnchanged|TestRepeatVerdictResultFieldsOmitEmpty|TestRepeatVerdictMeanEqualsMeanOfRepeatScores)\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^(TestRepeatVerdictMinMaxMean|TestAgentVerdictCasesTotalAndFailed)$' -v ./internal/eval
      - |-
        go test -count=1 -run '^TestRepeatVerdictMinMaxMean$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestRepeatVerdictMinMaxMean/min-max-mean'
      - |-
        go test -count=1 -run '^TestRepeatVerdictMinMaxMean$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestRepeatVerdictMinMaxMean/mean-of-repeat-scores'
      - go vet ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - |-
        test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

  - id: prune-container-config-tests
    agent: builder
    description: >-
      runner_config_test.go: fold TestCreateContainerConfig_NilSandboxConfig, TestCreateContainerConfig_EmptyStringValues,
      TestCreateContainerConfig_ZeroValues and TestCreateContainerConfig_DebugFlag into the table of TestCreateContainerConfig_WithSandboxConfig,
      then delete the four functions. Add a debug bool field to the row struct, pass tt.debug to createContainerConfig, and assert
      result.Debug == tt.debug for every row. Add rows nil sandbox config (nil cfg, defaults /workspace, 1000000, 1024MiB, 5m), empty workspace dir
      (WorkspaceDir empty, CPUCores 2.0, MemoryMB 1024, Timeout 5m; expects /workspace, 2000000, 1024MiB, 5m), zero cpu memory timeout
      (WorkspaceDir /custom, CPU/memory/timeout 0; expects /custom, 1000000, 1024MiB, 5m) and debug enabled (nil cfg, debug true, default values).
      Each row expects Environment KIRO_CLI_DISABLE_TELEMETRY=1. Leave TestCreateContainerConfig_ResourceLimitOverrides untouched. Follow the
      per-task routine in the spec.
    dependencies: ["prune-repeat-tests"]
    acceptance_criteria:
      - "Before editing, the four removed tests and TestCreateContainerConfig_WithSandboxConfig were run with -v and passed (evidence in the sentinel)"
      - "func TestCreateContainerConfig_NilSandboxConfig, TestCreateContainerConfig_EmptyStringValues, TestCreateContainerConfig_ZeroValues and TestCreateContainerConfig_DebugFlag no longer exist in internal/eval/*_test.go"
      - "TestCreateContainerConfig_WithSandboxConfig keeps its three original rows and all original assertions (WorkspaceDir, CPUQuota, Memory, Timeout, Platform non-empty, Environment length and values) and adds a Debug assertion for every row"
      - "Passing subtests nil_sandbox_config, empty_workspace_dir, zero_cpu_memory_timeout and debug_enabled exist with the expected values listed in the spec"
      - "debug_enabled asserts Debug is true; all other rows assert Debug is false"
      - "Negative check done on each new row (change one expected value, see that row alone fail, revert)"
      - "TestCreateContainerConfig_ResourceLimitOverrides is unchanged; only runner_config_test.go is modified and committed; go vet and gofmt are clean"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCreateContainerConfig_NilSandboxConfig|TestCreateContainerConfig_EmptyStringValues|TestCreateContainerConfig_ZeroValues|TestCreateContainerConfig_DebugFlag)\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^(TestCreateContainerConfig_WithSandboxConfig|TestCreateContainerConfig_ResourceLimitOverrides)$' -v ./internal/eval
      - |-
        for s in complete_sandbox_config partial_sandbox_config minimal_sandbox_config nil_sandbox_config empty_workspace_dir zero_cpu_memory_timeout debug_enabled; do go test -count=1 -run '^TestCreateContainerConfig_WithSandboxConfig$' -v ./internal/eval 2>&1 | grep -q -- "--- PASS: TestCreateContainerConfig_WithSandboxConfig/$s" || { echo "missing PASS for $s"; exit 1; }; done
      - go vet ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - |-
        test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

  - id: delete-runner-and-workspace-tests
    agent: builder
    description: >-
      Plain deletes. runner_test.go: delete TestScoreLLMJudge_ANSISequences (it never calls scoreLLMJudge) and remove the two imports
      encoding/json and strings, which only it used; keep os, path/filepath and testing. sandbox_workspace_test.go: delete
      TestWantThresholdFailure together with its doc comment, and keep the helper wantThresholdFailure and its doc comment because runEvalIn calls
      it. No other import or helper may become unused. Follow the per-task routine in the spec (no negative check: nothing is folded in).
    dependencies: ["prune-container-config-tests"]
    acceptance_criteria:
      - "Before editing, TestScoreLLMJudge_ANSISequences and TestWantThresholdFailure were run with -v and passed (evidence in the sentinel)"
      - "func TestScoreLLMJudge_ANSISequences and TestWantThresholdFailure no longer exist in internal/eval/*_test.go"
      - "func wantThresholdFailure still exists in internal/eval/sandbox_workspace_test.go and runEvalIn still calls it"
      - "runner_test.go no longer imports encoding/json or strings, and the package compiles (go vet ./internal/eval passes)"
      - "No other import or helper was removed or left unused; only runner_test.go and sandbox_workspace_test.go are modified and committed; gofmt is clean"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestScoreLLMJudge_ANSISequences|TestWantThresholdFailure)\(' internal/eval/*_test.go)"
      - |-
        grep -q 'func wantThresholdFailure(' internal/eval/sandbox_workspace_test.go
      - |-
        grep -q 'wantThresholdFailure(agent, opts)' internal/eval/sandbox_workspace_test.go
      - |-
        test -z "$(sed -n '1,10p' internal/eval/runner_test.go | grep -E '\"(encoding/json|strings)\"')"
      - go vet ./internal/eval
      - go test -count=1 -run '^(TestScoreDeterministic_FileReferenceWorkspace|TestScoreDeterministic_AcceptanceCriteriaQuality)$' -v ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - |-
        test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

  - id: prune-scoring-tests
    agent: builder
    description: >-
      scoring_test.go: (1) fold TestPrintCaseResultNoBreakdownWhenPassing into TestPrintCaseResultBreakdownOnlyBelowThreshold as a table with rows
      below-threshold-lists-weak-criteria (Rubric{}, the five scores, exact want string, absent edge/high) and passing-omits-breakdown
      (PassThreshold 80, weak 2/5 plus three strong 5/5, want ' ✅ 85% (threshold: 80%)'). (2) Fold TestScoreCaseSkipsCostCriterion into
      TestScoreCaseNilScoresWhenNoNonCostCriteria: turn the map into a table with name, rubric, output and wantNames fields (nil wantNames means
      Scores must be nil); keep empty rubric and cost only unchanged with output 'out'; add row cost and deterministic (cost_budget cost criterion
      plus structural_completeness deterministic, output '## H\n### S\ntext', expects exactly one score named structural_completeness). Keep the
      function name. (3) Fold TestScoreCaseDeterministicEmptyOutputSkipped into TestScoreCaseLLMJudgeEmptyOutputSkipped as subtests llm-judge
      (current body unchanged, including the readCalls check, run first) and deterministic (len(Scores)==1, Skipped, Score==0, nothing more).
      Delete the three merged functions. Follow the per-task routine in the spec.
    dependencies: ["delete-runner-and-workspace-tests"]
    acceptance_criteria:
      - "Before editing, the three removed tests and their three partners were run with -v and passed (evidence in the sentinel)"
      - "func TestPrintCaseResultNoBreakdownWhenPassing, TestScoreCaseSkipsCostCriterion and TestScoreCaseDeterministicEmptyOutputSkipped no longer exist in internal/eval/*_test.go"
      - "TestPrintCaseResultBreakdownOnlyBelowThreshold has passing subtests below-threshold-lists-weak-criteria (exact output plus edge/high absent) and passing-omits-breakdown (exact output ' ✅ 85% (threshold: 80%)')"
      - "TestScoreCaseNilScoresWhenNoNonCostCriteria keeps empty_rubric and cost_only asserting Scores is nil, and adds passing cost_and_deterministic asserting exactly one score named structural_completeness"
      - "TestScoreCaseLLMJudgeEmptyOutputSkipped has passing subtests llm-judge (all original assertions) and deterministic (one score, Skipped, Score 0)"
      - "Negative check done on each new row or subtest, then reverted"
      - "Partner function names are unchanged; only scoring_test.go is modified and committed; go vet and gofmt are clean"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestPrintCaseResultNoBreakdownWhenPassing|TestScoreCaseSkipsCostCriterion|TestScoreCaseDeterministicEmptyOutputSkipped)\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^(TestPrintCaseResultBreakdownOnlyBelowThreshold|TestScoreCaseNilScoresWhenNoNonCostCriteria|TestScoreCaseLLMJudgeEmptyOutputSkipped)$' -v ./internal/eval
      - |-
        go test -count=1 -run '^TestPrintCaseResultBreakdownOnlyBelowThreshold$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestPrintCaseResultBreakdownOnlyBelowThreshold/passing-omits-breakdown'
      - |-
        go test -count=1 -run '^TestPrintCaseResultBreakdownOnlyBelowThreshold$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestPrintCaseResultBreakdownOnlyBelowThreshold/below-threshold-lists-weak-criteria'
      - |-
        go test -count=1 -run '^TestScoreCaseNilScoresWhenNoNonCostCriteria$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestScoreCaseNilScoresWhenNoNonCostCriteria/cost_and_deterministic'
      - |-
        go test -count=1 -run '^TestScoreCaseLLMJudgeEmptyOutputSkipped$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestScoreCaseLLMJudgeEmptyOutputSkipped/llm-judge'
      - |-
        go test -count=1 -run '^TestScoreCaseLLMJudgeEmptyOutputSkipped$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestScoreCaseLLMJudgeEmptyOutputSkipped/deterministic'
      - go vet ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - |-
        test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

  - id: prune-loader-schema-tests
    agent: builder
    description: >-
      Loader schema merges. In turns_schema_test.go: (1) add an optional also []string field to the TestLoadCasesTurnsSchemaErrors row struct, a loop
      asserting the error contains each also entry, and a row named judge-turn-9 with the removed test's YAML verbatim (2-turn case, judge check with
      turn 9), want 'turn 9 is beyond the case's 2 turn(s)' and also {'check 1 (judge)'}; (2) turn TestLoadCasesTurnsAndInputMutuallyExclusive into a
      {name, yaml} table with rows both-forms (current YAML, unchanged assertions: error is non-nil, names the case, contains 'turns and input are
      mutually exclusive') and no-sim-both (name no-sim-both, input hello, single-entry turns [one]). Then delete
      TestLoadCasesJudgeTurnBeyondCaseTurns (selftest_judge_transcript_test.go, with its doc comment) and
      TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser (simulated_user_schema_test.go). Carry the removed test's comment onto the
      judge-turn-9 row. Follow the per-task routine in the spec.
    dependencies: ["prune-scoring-tests"]
    acceptance_criteria:
      - "Before editing, the two removed tests, TestLoadCasesTurnsSchemaErrors and TestLoadCasesTurnsAndInputMutuallyExclusive were run with -v and passed (evidence in the sentinel)"
      - "func TestLoadCasesJudgeTurnBeyondCaseTurns and TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser no longer exist in internal/eval/*_test.go"
      - "TestLoadCasesTurnsSchemaErrors keeps its five original rows and assertions and has a passing subtest judge-turn-9 that requires the error to contain judge-turn-9, 'check 1 (judge)' and 'turn 9 is beyond the case's 2 turn(s)'"
      - "TestLoadCasesTurnsAndInputMutuallyExclusive has passing subtests both-forms (original YAML and assertions) and no-sim-both (single-entry turns plus input)"
      - "Negative check done on the judge-turn-9, both-forms and no-sim-both rows, then reverted"
      - "Only turns_schema_test.go, selftest_judge_transcript_test.go and simulated_user_schema_test.go are modified and committed; go vet and gofmt are clean"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestLoadCasesJudgeTurnBeyondCaseTurns|TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser)\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^(TestLoadCasesTurnsSchemaErrors|TestLoadCasesTurnsAndInputMutuallyExclusive)$' -v ./internal/eval
      - |-
        go test -count=1 -run '^TestLoadCasesTurnsSchemaErrors$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestLoadCasesTurnsSchemaErrors/judge-turn-9'
      - |-
        go test -count=1 -run '^TestLoadCasesTurnsAndInputMutuallyExclusive$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestLoadCasesTurnsAndInputMutuallyExclusive/both-forms'
      - |-
        go test -count=1 -run '^TestLoadCasesTurnsAndInputMutuallyExclusive$' -v ./internal/eval 2>&1 | grep -q -- '--- PASS: TestLoadCasesTurnsAndInputMutuallyExclusive/no-sim-both'
      - go vet ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - |-
        test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

  - id: prune-unknown-backend-test
    agent: builder
    description: >-
      Fold TestSelfTestRejectsUnknownBackend (selftest_test.go:338) into TestRunWithOptionsRejectsUnknownBackend (config_test.go:116) and delete the
      original. New partner body: t.Cleanup(resetConfig); installFakeKiroCLI; registerResultsCleanup and snapshotDirs on
      selftestEvalsDir/results; RunWithOptions('selftest', '', RunOptions{Backend: 'nope', EvalsDir: selftestEvalsDir}) must return an error; the
      error must contain 'valid backends' (the partner's original assertion), the quoted name nope, kiro-cli and stub; newRunDirs must report no new
      result directory; readCalls must show no kiro-cli invocation. The input moves from agent 'a' with the default evals dir to the selftest agent
      and fixture dir because backend rejection happens in configure() before any agent or evals dir is read; the result-dir assertion needs a known
      evals dir. Follow the per-task routine in the spec.
    dependencies: ["prune-loader-schema-tests"]
    acceptance_criteria:
      - "Before editing, TestSelfTestRejectsUnknownBackend and TestRunWithOptionsRejectsUnknownBackend were run with -v and passed (evidence in the sentinel)"
      - "func TestSelfTestRejectsUnknownBackend no longer exists in internal/eval/*_test.go"
      - "TestRunWithOptionsRejectsUnknownBackend passes and asserts: error non-nil; error contains 'valid backends', the quoted nope, kiro-cli and stub; no new run directory under the results dir; no kiro-cli invocation recorded"
      - "Negative check done on each new assertion (for example change 'stub' to 'zzz' and see the test fail on that message), then reverted"
      - "Only config_test.go and selftest_test.go are modified and committed; go vet and gofmt are clean"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestSelfTestRejectsUnknownBackend\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^TestRunWithOptionsRejectsUnknownBackend$' -v ./internal/eval
      - |-
        body="$(sed -n '/^func TestRunWithOptionsRejectsUnknownBackend/,/^}/p' internal/eval/config_test.go)"; for m in 'valid backends' 'nope' 'kiro-cli' 'stub' 'newRunDirs' 'readCalls' 'installFakeKiroCLI' 'registerResultsCleanup'; do printf '%s' "$body" | grep -q -- "$m" || { echo "partner body lacks $m"; exit 1; }; done
      - go vet ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - |-
        test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

  - id: delete-selftest-duplicates
    agent: builder
    description: >-
      Plain deletes of four selftest duplicates, each after confirming its claim. selftest_checks_test.go: delete
      TestSelfTestChecksLegacyScoresUnchanged (strict subset of TestChecksScoringCharacterizationLegacySelftestCases in checks_scoring_test.go) and
      TestSelfTestChecksRunExitStatus, each with its doc comment. Its named partner TestEvalSelfTestSucceedsWithDefaults was already deleted by
      issue 435, so before deleting confirm these pass and record them as the replacement evidence: TestResumeRunPrintsVerdictAndReturnsThresholdError
      and TestChecksScoringCharacterizationLegacySelftestCases in internal/eval, and TestEvalSelfTestFailExitsWithThresholdError in
      cmd/kairon/cmd; if any fails or is missing, stop with Status BLOCKED instead of deleting. selftest_judge_test.go: delete
      TestSelfTestJudgeFilesReachTheJudge (already asserted in TestSelfTestJudgePassingCases' loop). selftest_pricing_test.go: delete
      TestSelftestPriceFixturesDefineStubUsage with its doc comment (the run tests assert the same model and tokens). Keep all imports and helpers
      in use; do not edit checks_scoring_test.go or any partner.
    dependencies: ["prune-unknown-backend-test"]
    acceptance_criteria:
      - "Before editing, the four removed tests and their partners (TestChecksScoringCharacterizationLegacySelftestCases, TestSelfTestJudgePassingCases, TestSelftestPriceSonnetIsPricedPerModel, TestSelftestPriceUnknownMarksCostUnknown) were run with -v and passed (evidence in the sentinel)"
      - "Replacement evidence for TestSelfTestChecksRunExitStatus was run and passed: TestResumeRunPrintsVerdictAndReturnsThresholdError, TestChecksScoringCharacterizationLegacySelftestCases and cmd TestEvalSelfTestFailExitsWithThresholdError"
      - "func TestSelfTestChecksLegacyScoresUnchanged, TestSelfTestChecksRunExitStatus, TestSelfTestJudgeFilesReachTheJudge and TestSelftestPriceFixturesDefineStubUsage no longer exist in internal/eval/*_test.go"
      - "The partners still pass unchanged; checks_scoring_test.go is not modified"
      - "No import or helper is left unused (loadSelftestStub, copyFixturesTo, resetConfig, runSelfTestAgent, priceSonnetCase and priceUnknownCase are still used); only selftest_checks_test.go, selftest_judge_test.go and selftest_pricing_test.go are modified and committed; go vet and gofmt are clean"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestSelfTestChecksLegacyScoresUnchanged|TestSelfTestChecksRunExitStatus|TestSelfTestJudgeFilesReachTheJudge|TestSelftestPriceFixturesDefineStubUsage)\(' internal/eval/*_test.go)"
      - go test -count=1 -run '^(TestChecksScoringCharacterizationLegacySelftestCases|TestSelfTestJudgePassingCases|TestSelftestPriceSonnetIsPricedPerModel|TestSelftestPriceUnknownMarksCostUnknown|TestResumeRunPrintsVerdictAndReturnsThresholdError)$' -v ./internal/eval
      - go test -count=1 -run '^TestEvalSelfTestFailExitsWithThresholdError$' -v ./cmd/kairon/cmd
      - go vet ./internal/eval
      - |-
        test -z "$(gofmt -l internal/eval)"
      - |-
        test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"

  - id: gate-coverage-and-task-test
    agent: builder
    description: >-
      Final gate. Run the full internal/eval package with a coverage profile and confirm it passes with total coverage at least 87.6% (baseline 4080
      of 4659 statements = 87.5724%; 4079 is the minimum that still prints 87.6%). Confirm no --- FAIL, top-level --- PASS fell from 807 to 788, and
      the 7 --- SKIP are unchanged; record the actual numbers. Run task fmt:check, task lint and task test and confirm each exits 0 (task test runs
      the whole module with -race and takes several minutes; run it in the background with output redirected to a file). Confirm git diff
      --name-only origin/main...HEAD is non-empty and lists only _test.go files (nothing under .kiro). If coverage fell below 87.6%, find the merged
      test whose input is no longer exercised and restore it as a table row in the relevant partner; never touch production code or delete
      assertions. Make no other change when everything passes.
    dependencies: ["delete-selftest-duplicates"]
    acceptance_criteria:
      - "go test -count=1 -coverprofile ./internal/eval passes and go tool cover -func reports total of at least 87.6%"
      - "The package run has 0 --- FAIL, 788 top-level --- PASS and 7 --- SKIP, or any difference is explained in the sentinel"
      - "task fmt:check, task lint and task test each exit 0"
      - "git diff --name-only origin/main...HEAD is non-empty and every listed file ends in _test.go"
      - "The Delete grep and the Merge grep from the spec's Validation Commands both print nothing"
      - "If a statement had to be restored, it was restored as a table row and no assertion was removed"
    validation_commands:
      - |-
        go test -count=1 -coverprofile=/tmp/eval.cov ./internal/eval && go tool cover -func=/tmp/eval.cov | awk '/^total:/ { v=$NF; sub("%","",v); if (v+0 < 87.6) { print "coverage " v " < 87.6"; exit 1 } }'
      - |-
        test -z "$(grep -nE 'func (TestRepeatVerdictLegacyCaseAccountingUnchanged|TestRepeatVerdictResultFieldsOmitEmpty|TestScoreLLMJudge_ANSISequences|TestSelfTestChecksLegacyScoresUnchanged|TestSelfTestChecksRunExitStatus|TestSelfTestJudgeFilesReachTheJudge|TestSelftestPriceFixturesDefineStubUsage|TestWantThresholdFailure)\(' internal/eval/*_test.go)"
      - |-
        test -z "$(grep -nE 'func (TestCreateContainerConfig_DebugFlag|TestCreateContainerConfig_EmptyStringValues|TestCreateContainerConfig_NilSandboxConfig|TestCreateContainerConfig_ZeroValues|TestLoadCasesJudgeTurnBeyondCaseTurns|TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser|TestPrintCaseResultNoBreakdownWhenPassing|TestRepeatVerdictMeanEqualsMeanOfRepeatScores|TestScoreCaseDeterministicEmptyOutputSkipped|TestScoreCaseSkipsCostCriterion|TestSelfTestRejectsUnknownBackend)\(' internal/eval/*_test.go)"
      - |-
        test -n "$(git diff --name-only origin/main...HEAD)" && test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"
      - task fmt:check
      - task lint
      - task test

  - id: validate-prune
    agent: validator
    description: >-
      Read-only, independent verification of the whole change against issue 426, without relying on builder sentinels. Check AC1 (Delete grep prints
      nothing), AC2 (Merge grep prints nothing, every partner passes with -v and every prescribed subtest in the spec's table appears as PASS, and
      TestRunWithOptionsRejectsUnknownBackend contains the three names, the no-result-dir check and the no-kiro-cli-call check besides 'valid
      backends'), the Behavior-Preservation Contract (compare each converted partner with git show origin/main:internal/eval/<file> and confirm no
      original assertion was removed or weakened; wantThresholdFailure survives; no unused imports), AC3 (only _test.go files in git diff --name-only
      origin/main...HEAD), AC4 (go test -cover ./internal/eval reports at least 87.6%) and AC5 (task test exits 0). Also confirm the AGENTS.md rules:
      no deferral wording, and every deviation from the spec has a code comment and a sentinel entry.
    dependencies: ["gate-coverage-and-task-test"]
    acceptance_criteria:
      - "AC1: the Delete grep over internal/eval/*_test.go prints nothing"
      - "AC2: the Merge grep prints nothing and each partner passes with its prescribed subtests as PASS"
      - "Every converted partner still makes every original assertion on its original input; the only assertions gone are those the spec lists as covered elsewhere"
      - "AC3: git diff --name-only origin/main...HEAD lists only files ending in _test.go"
      - "AC4: go test -cover ./internal/eval reports at least 87.6%"
      - "AC5: task test exits 0"
      - "No deferral wording and no unrecorded deviation from the spec"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestRepeatVerdictLegacyCaseAccountingUnchanged|TestRepeatVerdictResultFieldsOmitEmpty|TestScoreLLMJudge_ANSISequences|TestSelfTestChecksLegacyScoresUnchanged|TestSelfTestChecksRunExitStatus|TestSelfTestJudgeFilesReachTheJudge|TestSelftestPriceFixturesDefineStubUsage|TestWantThresholdFailure)\(' internal/eval/*_test.go)"
      - |-
        test -z "$(grep -nE 'func (TestCreateContainerConfig_DebugFlag|TestCreateContainerConfig_EmptyStringValues|TestCreateContainerConfig_NilSandboxConfig|TestCreateContainerConfig_ZeroValues|TestLoadCasesJudgeTurnBeyondCaseTurns|TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser|TestPrintCaseResultNoBreakdownWhenPassing|TestRepeatVerdictMeanEqualsMeanOfRepeatScores|TestScoreCaseDeterministicEmptyOutputSkipped|TestScoreCaseSkipsCostCriterion|TestSelfTestRejectsUnknownBackend)\(' internal/eval/*_test.go)"
      - go test -count=1 -v -run '^(TestRepeatVerdictMinMaxMean|TestCreateContainerConfig_WithSandboxConfig|TestPrintCaseResultBreakdownOnlyBelowThreshold|TestScoreCaseNilScoresWhenNoNonCostCriteria|TestScoreCaseLLMJudgeEmptyOutputSkipped|TestLoadCasesTurnsSchemaErrors|TestLoadCasesTurnsAndInputMutuallyExclusive|TestRunWithOptionsRejectsUnknownBackend)$' ./internal/eval
      - |-
        test -n "$(git diff --name-only origin/main...HEAD)" && test -z "$(git diff --name-only origin/main...HEAD | grep -v '_test\.go$')"
      - |-
        go test -count=1 -coverprofile=/tmp/eval.cov ./internal/eval && go tool cover -func=/tmp/eval.cov | awk '/^total:/ { v=$NF; sub("%","",v); if (v+0 < 87.6) { print "coverage " v " < 87.6"; exit 1 } }'
      - task test
```
