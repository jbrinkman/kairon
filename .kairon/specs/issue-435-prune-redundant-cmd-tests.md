# Design Spec: Prune redundant cmd/kairon/cmd tests

Closes #435

## Scope Coverage

Rows S1 and AC1-AC5 quote the issue's In Scope item and acceptance criteria. Rows T1-T21 map each of the 21 tests in the issue's Current / Target State table (action and partner as the issue gives them) to the plan tasks that change it.

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`cmd/kairon/cmd`: `check_test.go`, `eval_audit_test.go`, `eval_test.go`, `eval_trend_test.go`, `eval_verify_log_test.go`, `inference_exec_test.go`" | `baseline`, `flag-table-eval`, `flag-table-cli`, `backend-and-args`, `prune-check`, `prune-audit`, `prune-trend`, `prune-inference-exec`, `verify-log-tables`, `final-validate` |
| AC1 | "The 6 tests marked Delete no longer exist." | `backend-and-args`, `prune-check`, `prune-audit`, `prune-trend`, `final-validate` (`grep` check from the issue) |
| AC2 | "Each of the 10 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `flag-table-eval`, `flag-table-cli`, `backend-and-args`, `verify-log-tables`, `prune-check`, `prune-audit`, `prune-trend`, `prune-inference-exec`, `final-validate` (`go test -run '^<partner>$' -v`) |
| AC3 | "Each of the 2 groups marked Combine is one table-driven test with a case for each former test, and the former functions are gone." | `backend-and-args` (`TestEvalAgentSubcommandArgs`), `verify-log-tables` (`TestVerifyLogFlags`), `prune-audit`, `prune-trend`, `final-validate` |
| AC4 | "Package coverage does not drop." | `baseline`, `final-validate` (`go test -cover ./cmd/kairon/cmd`, floor 57.7%) |
| AC5 | "Every remaining test passes unchanged, and the gate passes." | `baseline`, `final-validate` (`task test`) |
| T1 | Delete: "`TestCheckCommandRegistered`" in `check_test.go`, partner `TestCheckFlagsRegistered` | `prune-check` |
| T2 | Merge: "`TestCheckFlagsRegistered`" in `check_test.go` into `TestEvalFlagsRegistered` | `flag-table-cli`, `prune-check` |
| T3 | Delete: "`TestAuditHelpDescribesEvidenceAndDuplicates`" in `eval_audit_test.go` | `prune-audit` |
| T4 | Delete: "`TestAuditHelpDescribesRubricRules`" in `eval_audit_test.go` | `prune-audit` |
| T5 | Merge: "`TestAuditFlagsRegistered`" in `eval_audit_test.go` into `TestEvalFlagsRegistered` | `flag-table-cli`, `prune-audit` |
| T6 | Combine: "`TestAuditArgs`" in `eval_audit_test.go` with `TestTrendArgs` and `TestVerifyLogMissingAgentIsUsageError` | `backend-and-args`, `prune-audit` |
| T7 | Delete: "`TestAuditSkillsFSWiredFromTemplates`" in `eval_audit_test.go` | `prune-audit` |
| T8 | Merge: "`TestEvalUnknownBackendRejectedForPerfAndCleanup`" in `eval_test.go` into `TestEvalUnknownBackendRejected` | `backend-and-args` |
| T9 | Delete: "`TestEvalSelfTestSucceedsWithDefaults`" in `eval_test.go`, partner `TestEvalPromptFileRecorded` | `backend-and-args` |
| T10 | Merge: "`TestEvalPromptFileFlagRegistered`" in `eval_test.go` into `TestEvalFlagsRegistered` | `flag-table-eval` |
| T11 | Merge: "`TestEvalRepeatFlagRegistered`" in `eval_test.go` into `TestEvalFlagsRegistered` | `flag-table-eval` |
| T12 | Merge: "`TestEvalFilterAndStrictFlagsRegistered`" in `eval_test.go` into `TestEvalFlagsRegistered` | `flag-table-eval` |
| T13 | Merge: "`TestTrendFlagsRegistered`" in `eval_trend_test.go` into `TestEvalFlagsRegistered` | `flag-table-cli`, `prune-trend` |
| T14 | Delete: "`TestTrendRestoresFlagVars`" in `eval_trend_test.go` | `prune-trend` |
| T15 | Combine: "`TestTrendArgs`" in `eval_trend_test.go` with `TestAuditArgs` and `TestVerifyLogMissingAgentIsUsageError` | `backend-and-args`, `prune-trend` |
| T16 | Combine: "`TestVerifyLogFlagsRegistered`" in `eval_verify_log_test.go` with `TestVerifyLogRequireCycleFlagParsed` | `verify-log-tables` |
| T17 | Merge: "`TestVerifyLogNoRefactorPassesWithoutRequireCycle`" in `eval_verify_log_test.go` into `TestVerifyLogNoRefactorFailsByDefault` | `verify-log-tables` |
| T18 | Merge: "`TestVerifyLogValidSetWithoutRequireCycle`" in `eval_verify_log_test.go` into `TestVerifyLogValidSet` | `verify-log-tables` |
| T19 | Combine: "`TestVerifyLogRequireCycleFlagParsed`" in `eval_verify_log_test.go` with `TestVerifyLogFlagsRegistered` | `verify-log-tables` |
| T20 | Combine: "`TestVerifyLogMissingAgentIsUsageError`" in `eval_verify_log_test.go` with `TestAuditArgs` and `TestTrendArgs` | `backend-and-args`, `verify-log-tables` |
| T21 | Merge: "`TestInferenceExecHidden`" in `inference_exec_test.go` into `TestEvalFlagsRegistered` | `flag-table-cli`, `prune-inference-exec` |

## Deferred work

none

## Solution Approach

This is a test-only refactor of six `_test.go` files in `cmd/kairon/cmd`. No production code, no template file and no unlisted test changes. 6 tests are deleted, 10 are folded into the table or subtest set of a named partner, and 5 become two new table tests.

### Baseline (recorded by the architect on this worktree before any change)

- `go test -cover ./cmd/kairon/cmd` reports `coverage: 63.2% of statements` (about 29 s). The issue's floor is 57.7%; the gate for AC4 is the floor, and the implementation must also explain any fall below 63.2% (none is expected: only tests are removed, and each removed test is subsumed by a surviving one).
- The package declares 78 `func Test...` functions. Target: 78 - 21 removed + 2 new = 59.
- The 21 functions to remove all exist at the line numbers the issue gives.

### Environment note: `task test` in this worktree

In this sandbox worktree `task test` exits non-zero before any change: seven tests in `internal/planner` (`TestAutomatedManifest`, `TestAutomatedManifestHasNoInjectedOrWriteCapabilities`, `TestAutomatedFilesMatchTemplates`, `TestInteractiveManifest`, `TestInteractiveManifestHasNoInjectedCapabilities`, `TestInteractiveFilesMatchTemplates`, `TestEvalDriverDryRun`) fail because the sandbox rewrites `.kiro/agents/*.json` (uncommitted; it adds an `@creds-agent` tool and MCP server). The architect confirmed that the same tests pass in a clean detached checkout of `HEAD`, and that `task test` there exits 0 (about 10.5 minutes). Those files are not part of this issue and must not be reverted or edited to make the local run pass. `final-validate` therefore proves AC5 on a clean checkout of `HEAD` with the working-tree test changes applied, and also runs `go test -race ./cmd/...` in the worktree. The package this issue changes, `cmd/kairon/cmd`, is unaffected by the injected files.

### Refactor discipline (AGENTS.md, Test-driven development, Refactors)

1. Coverage exists before the change: each former test is itself the characterization test of the behaviour it pins, and the `baseline` task records that the whole package passes and what it covers.
2. The old tests stay in place until the replacement rows exist and pass. The task order guarantees this: tables are written first (`flag-table-eval`, `flag-table-cli`, `backend-and-args`), then the old functions are deleted (`prune-*`, `verify-log-tables`). No intermediate state loses an assertion.
3. A new row must be able to fail. For each new table row that carries an assertion (not a bare existence check), the builder temporarily flips the expected value, runs the row, sees it fail for the expected reason (the assertion message, not a compile error), then restores the value. This replaces the "failing test first" step, which does not apply to a test-only refactor.
4. The surviving tests pass before and after. The commit message names the tests relied on (the partners and the two new table tests).

### Shared flag-table shape

`TestEvalFlagsRegistered` (in `eval_test.go`) is the "shared flag table". Its two existing assertions (`--backend` and `--evals-dir`) become the first two rows of a table, and a small helper (`flagCase` plus an assert function, defined once in `eval_test.go`) runs one row as a subtest. `TestVerifyLogFlags` reuses the same helper, so there is one flag-row shape in the package.

A row has these capabilities (field names are the builder's choice):

- the command that owns the flag, resolved at run time (a plain `*cobra.Command` variable, or `rootCmd.Find([]string{"check"})` for `check`, which fails the row if the command is not registered or its `Name()` is not `check`);
- the flag name, and whether it is looked up in `Flags()`, `PersistentFlags()` or `LocalFlags()`;
- an optional expected `DefValue`, an optional expected `Value.Type()`, an optional list of strings `Usage` must contain (including "every `inference.Names()` entry");
- an optional "must carry cobra's required annotation" check, using the existing `cobraRequiredAnnotation` constant in `check_test.go` (the constant stays where it is; it is used from the same package);
- lists of commands that must inherit the flag (`InheritedFlags().Lookup` non-nil) and commands that must not;
- a check that the flag is absent from `LocalFlags()` or `PersistentFlags()` where the old test asserted that.

Subtest names contain no spaces, so they are stable in `go test -run -v` output. The exact names are fixed below.

### Table: `TestEvalFlagsRegistered` (eval_test.go)

| Subtest name | Source | Assertions in the row |
|---|---|---|
| `eval:--backend` | existing | `evalCmd.Flags()` has it; `DefValue == inference.NameKiroCLI`; `Usage` contains every `inference.Names()` entry |
| `eval:--evals-dir` | existing | `evalCmd.PersistentFlags()` has it; `DefValue == ""`; `diffCmd.InheritedFlags()` has it |
| `eval:--prompt-file` | `TestEvalPromptFileFlagRegistered` | `evalCmd.Flags()` has it; `evalCmd.PersistentFlags()` does not; `diffCmd.InheritedFlags()` does not |
| `eval:--repeat` | `TestEvalRepeatFlagRegistered` | `evalCmd.Flags()` has it; `DefValue == "1"`; `diffCmd.InheritedFlags()` does not |
| `eval:--filter` | `TestEvalFilterAndStrictFlagsRegistered` | `evalCmd.PersistentFlags()` has it; `diffCmd` and `trendCmd` `InheritedFlags()` both have it |
| `eval:--strict` | `TestEvalFilterAndStrictFlagsRegistered` | `evalCmd.Flags()` has it; `Value.Type() == "bool"` and `DefValue == "false"`; `evalCmd.PersistentFlags()` does not; `diffCmd` and `trendCmd` `InheritedFlags()` do not |
| `check:--checks` | `TestCheckFlagsRegistered` | found via `rootCmd.Find`; `Flags()` has it; required annotation equals `["true"]` |
| `check:--dir` | `TestCheckFlagsRegistered` | same as `check:--checks` |
| `check:--backend` | `TestCheckFlagsRegistered` | `Flags()` has it; `DefValue == inference.NameKiroCLI`; `Usage` contains every `inference.Names()` entry |
| `audit:--iterations-dir` | `TestAuditFlagsRegistered` | `auditCmd.Flags()` has it; `DefValue == ".kairon/iterations"` |
| `audit:--prompt-file` | `TestAuditFlagsRegistered` | `auditCmd.LocalFlags()` has it |
| `audit:--evals-dir` | `TestAuditFlagsRegistered` | `auditCmd.LocalFlags()` does not have it; `auditCmd.InheritedFlags()` does |
| `trend:--json` | `TestTrendFlagsRegistered` | `trendCmd.LocalFlags()` has it; `DefValue == "false"` |
| `trend:--evals-dir` | `TestTrendFlagsRegistered` | `trendCmd.LocalFlags()` does not have it; `trendCmd.InheritedFlags()` does |
| `inference-exec:--backend` | `TestInferenceExecHidden` | `inferenceExecCmd.Flags()` has it |
| `inference-exec:hidden` | `TestInferenceExecHidden` | `inferenceExecCmd.Hidden` is true; the text `rootCmd.Help()` writes (captured with `rootCmd.SetOut`, restored in `t.Cleanup`) does not contain `inference-exec` |

### Per-test disposition

"Survives" lists every assertion that moves into the named place. "Dropped" lists the only assertions that are not carried over, each with the reason (the verdict's reason column, or the assertion is implied by a surviving one).

| Former test | Action | Survives (where) | Dropped, and why |
|---|---|---|---|
| `TestCheckCommandRegistered` | Delete | Nothing to move. `rootCmd.Find("check")` is still asserted by `runCheck` (used by every check run test) and by the `check:*` rows. | The whole test: it repeats `runCheck`'s own lookup (verdict). |
| `TestCheckFlagsRegistered` | Merge | Rows `check:--checks`, `check:--dir` (required annotation `["true"]`: the contract the verdict names), and `check:--backend` (default and usage-lists-every-backend; no other test asserts these for `check`). The `rootCmd.Find` lookup and `Name() == "check"` check move into these rows. | Existence-only checks for `--output`, `--gh-log`, `--hidden-dir`: `TestCheck...` tests pass each of them to `runCheck` (check_test.go lines 323-348, 400, 430, 456-479), so a missing flag fails there with "unknown flag". |
| `TestAuditHelpDescribesEvidenceAndDuplicates` | Delete | Nothing. | Asserts wording in `auditCmd.Long` (verdict). |
| `TestAuditHelpDescribesRubricRules` | Delete | Nothing. The rules stay tested by the audit fixture tests. | Asserts wording in `auditCmd.Long` (verdict). |
| `TestAuditFlagsRegistered` | Merge | Rows `audit:--iterations-dir` (default), `audit:--prompt-file` (local), `audit:--evals-dir` (not redefined locally, inherited). | The `evalCmd.Commands()` membership loop, `auditCmd.Name() == "audit"` and `evalCmd.Find({"audit","builder"})`: the inherited-`--evals-dir` row fails if `audit` is not a child of `eval`, and the name and routing are cobra's reading of `Use`. |
| `TestAuditArgs` | Combine | Subtest `audit` of `TestEvalAgentSubcommandArgs`. | Nothing. |
| `TestAuditSkillsFSWiredFromTemplates` | Delete | Nothing. | In package tests `Templates` is the zero `embed.FS`, so it only checks that `fs.Sub` on an empty FS does not fail (library behaviour, verdict). |
| `TestEvalUnknownBackendRejectedForPerfAndCleanup` | Merge | Subtests `perf` and `cleanup` of `TestEvalUnknownBackendRejected`; each asserts `err` is non-nil and contains `unknown backend`. | Nothing. |
| `TestEvalSelfTestSucceedsWithDefaults` | Delete | Nothing to move. `TestEvalPromptFileRecorded`'s baseline run is the same no-config selftest with the same exactly-one-`summary.json` check and `RunE` error check. | The whole test (verdict). |
| `TestEvalPromptFileFlagRegistered` | Merge | Row `eval:--prompt-file`: registered, local not persistent, not inherited by `diff` (the contract the verdict names). | Default `""` and the "candidate" usage wording (verdict: only the not-inherited row is contract). |
| `TestEvalRepeatFlagRegistered` | Merge | Row `eval:--repeat`: registered, default `"1"`, not inherited by `diff`. | `Value.Type() == "int"` (the variable is `IntVar`, and `TestEvalRepeatPassedToRun` proves the wiring), and "`evalCmd.UsageString()` contains `--repeat`" (cobra builds usage from the registered flags, so it follows from the row's lookup). |
| `TestEvalFilterAndStrictFlagsRegistered` | Merge | Rows `eval:--filter` (persistent, inherited by `diff` and `trend`) and `eval:--strict` (registered, bool, default false, not persistent, not inherited by `diff` or `trend`). | `--filter` type `stringArray` (verdict: proven by `TestEvalFilterFlagRepeatedAndCommas`) and the `UsageString()` contains `--filter`/`--strict` checks (follow from the lookups). |
| `TestTrendFlagsRegistered` | Merge | Rows `trend:--json` (local, default false) and `trend:--evals-dir` (not redefined locally, inherited). | The `evalCmd.Commands()` loop and `Name() == "trend"` (same reason as for audit). |
| `TestTrendRestoresFlagVars` | Delete | Nothing. | Tests the helper `runTrend`'s own cleanup (verdict). |
| `TestTrendArgs` | Combine | Subtest `trend` of `TestEvalAgentSubcommandArgs`. | Nothing. |
| `TestVerifyLogFlagsRegistered` | Combine | Subtest `registered` of `TestVerifyLogFlags`, with rows `iterations-dir` (default `.kairon/iterations`), `min-iterations` (default `2`), `require-cycle` (default `true`; usage contains `--require-cycle=false`), `evals-dir` (not local, inherited). | The `evalCmd.Commands()` loop and `Name() == "verify-log"` (same reason as for audit). |
| `TestVerifyLogNoRefactorPassesWithoutRequireCycle` | Merge | Subtest `require-cycle=false` of `TestVerifyLogNoRefactorFailsByDefault`: no error and output contains `OK selftest: 3 iterations verified`. | Nothing. |
| `TestVerifyLogValidSetWithoutRequireCycle` | Merge | Subtest `require-cycle=false` of `TestVerifyLogValidSet`: no error. | Nothing. |
| `TestVerifyLogRequireCycleFlagParsed` | Combine | Subtest `require-cycle-bound` of `TestVerifyLogFlags`, body unchanged: `Value.Set("false")` through the flag, `verifyLogRequireCycle` must become false, cleanup restores the variable, the default and `Changed`. | Nothing. |
| `TestVerifyLogMissingAgentIsUsageError` | Combine | Subtest `verify-log` of `TestEvalAgentSubcommandArgs`. | Nothing. |
| `TestInferenceExecHidden` | Merge | Rows `inference-exec:hidden` (Hidden flag and root help does not list the command) and `inference-exec:--backend` (exists). | Nothing. |

### The merged behavioural tests

- `TestEvalUnknownBackendRejected` becomes a table over mode with subtests `selftest`, `perf`, `cleanup`. Each subtest sets `evalBackend = "nope"`, sets the mode (`selftest`: args `{"selftest"}`; `perf`: `evalPerf = true`, args `nil`; `cleanup`: `evalCleanup = true`, args `nil`), calls `evalCmd.RunE`, and asserts a non-nil error. The existing assertion that the error contains `nope`, `inference.NameKiroCLI` and `inference.NameStub` stays for the `selftest` row. The `perf` and `cleanup` rows assert `unknown backend`. The builder may apply the union to every row if all rows pass it. `evalBackend`, `evalPerf` and `evalCleanup` are restored in `t.Cleanup`, and the modes never leak between rows.
- `TestVerifyLogNoRefactorFailsByDefault` becomes a two-row table, subtests `require-cycle=true` (error non-nil, contains `cycle`; the existing assertions) and `require-cycle=false` (no error, output contains `OK selftest: 3 iterations verified`). The function keeps its name because AC2 names it as the partner.
- `TestVerifyLogValidSet` loops over `requireCycle` in `{true, false}` with subtests `require-cycle=true` and `require-cycle=false`. Each row asserts no error and the `OK selftest: 4 iterations verified` line (the existing assertions). If the `false` row's output differs from the `true` row's, keep the old assertion set for that row (no error only) and record why in a code comment.

### The two Combine tables

- `TestEvalAgentSubcommandArgs` in `eval_test.go` (next to `TestEvalFlagsRegistered`): subtests `audit`, `trend`, `verify-log`, over `auditCmd`, `trendCmd` and `verifyLogCmd`. Each row asserts `cmd.Args(cmd, nil)` is an error, `cmd.Args(cmd, []string{"a", "b"})` is an error, and `cmd.Args(cmd, []string{"selftest"})` is nil. (`TestAuditArgs` used `builder` as its one valid argument; any single value tests the same `cobra.ExactArgs(1)`.)
- `TestVerifyLogFlags` in `eval_verify_log_test.go`: subtest `registered` (a table of the four rows above, run through the shared `flagCase` helper) and subtest `require-cycle-bound` (the body of `TestVerifyLogRequireCycleFlagParsed`). This is one table-driven test with a case for each former test, as AC3 requires.

### Design decisions

- **Where the verify-log flag rows live.** The issue's Reason column for `TestVerifyLogFlagsRegistered` says "fold into the shared flag table" while its Action says Combine with `TestVerifyLogRequireCycleFlagParsed` (partner "—") and AC3 requires one table-driven test with a case for each former test. These fit together as follows: the two former tests become one new table test, `TestVerifyLogFlags`, and the rows use the shared `flagCase` shape from `TestEvalFlagsRegistered`. The verify-log rows are not added to `TestEvalFlagsRegistered`, because AC3's check `go test -run '^<name>$' -v` must list one subtest per former test under one table test.
- **`TestEvalAgentSubcommandArgs` placement.** It lives in `eval_test.go` because all three commands are subcommands of `eval` and the file already holds the shared table. Any other file in the package would work equally well for AC3.
- **No `Templates`-wiring replacement.** `TestAuditSkillsFSWiredFromTemplates` is deleted outright. The real wiring is not testable from package `cmd` (the verdict's reason); the issue puts no replacement in scope.
- **Unrelated tests that the verdict file mentions stay as they are.** `TestEvalPromptFileRecorded`, `TestEvalSelfTestFailExitsWithThresholdError` and `TestEvalRepeatPassedToRun` are REWRITE-CHEAPER (#443) and out of scope. `internal/eval` tests that the verdict file mentions are not in `cmd/kairon/cmd` and stay.

## Relevant Files

Modified (all test-only):

- `cmd/kairon/cmd/eval_test.go`: flag-row helper; `TestEvalFlagsRegistered` as a table; `TestEvalUnknownBackendRejected` as a mode table; new `TestEvalAgentSubcommandArgs`; removes `TestEvalUnknownBackendRejectedForPerfAndCleanup`, `TestEvalSelfTestSucceedsWithDefaults`, `TestEvalPromptFileFlagRegistered`, `TestEvalRepeatFlagRegistered`, `TestEvalFilterAndStrictFlagsRegistered`. Needs new imports `bytes` and `github.com/spf13/cobra`.
- `cmd/kairon/cmd/check_test.go`: removes `TestCheckCommandRegistered`, `TestCheckFlagsRegistered`. Keeps `cobraRequiredAnnotation`, `runCheck`, `checkFixtures` and every other test. Drop any import this leaves unused (check `inference`).
- `cmd/kairon/cmd/eval_audit_test.go`: removes `TestAuditHelpDescribesEvidenceAndDuplicates`, `TestAuditHelpDescribesRubricRules`, `TestAuditFlagsRegistered`, `TestAuditArgs`, `TestAuditSkillsFSWiredFromTemplates`. Drops the `errors` import (it becomes unused; `io/fs` is still used by `useShippedSkills`).
- `cmd/kairon/cmd/eval_trend_test.go`: removes `TestTrendFlagsRegistered`, `TestTrendRestoresFlagVars`, `TestTrendArgs`.
- `cmd/kairon/cmd/eval_verify_log_test.go`: new `TestVerifyLogFlags`; `TestVerifyLogNoRefactorFailsByDefault` and `TestVerifyLogValidSet` become tables; removes `TestVerifyLogFlagsRegistered`, `TestVerifyLogNoRefactorPassesWithoutRequireCycle`, `TestVerifyLogValidSetWithoutRequireCycle`, `TestVerifyLogRequireCycleFlagParsed`, `TestVerifyLogMissingAgentIsUsageError`.
- `cmd/kairon/cmd/inference_exec_test.go`: removes `TestInferenceExecHidden`. `bytes` and `strings` stay in use through `runInferenceExec`.

Read only: `cmd/kairon/cmd/check.go`, `eval.go`, `eval_audit.go`, `eval_trend.go`, `eval_verify_log.go`, `inference_exec.go` (the flag declarations the rows mirror), `.kairon/research/test-audit-verdicts.md`, `.kairon/research/test-performance-analysis.md`.

Not touched: any non-test Go file, `cmd/kairon/templates/`, `docs/`, `README.md`. A search found no reference to any of the 21 removed names outside `.kairon/research/`, `.kairon/specs/` and the six test files, so no live doc needs an update.

## Team Orchestration

The six test files are edited by separate builder tasks. `eval_test.go` is edited by three tasks that run one after another, so two builders never edit the same file at once.

```
baseline (validator)
  -> flag-table-eval -> flag-table-cli -> backend-and-args
        -> prune-check, prune-audit, prune-trend, prune-inference-exec, verify-log-tables   (parallel: five different files)
              -> final-validate (validator)
```

- `baseline` records the starting state. No builder starts before it passes.
- `flag-table-eval`, `flag-table-cli` and `backend-and-args` only add or restructure tests in `eval_test.go` (plus the three eval-owned deletions and the `SelfTestSucceeds` deletion). Every old test they replace still exists in its own file, so the package is green and loses no assertion at every step.
- The five `prune`/`verify-log` tasks each own one file and depend on `backend-and-args`, so every replacement row and table already exists when an old function is deleted.
- `final-validate` is the only task that runs the whole gate (`task test`, about 10.5 minutes on a clean checkout; allow for it).

## Step-by-Step Task Breakdown

1. **baseline** (validator). Run `go test -cover ./cmd/kairon/cmd` and `go test -list '^Test' ./cmd/kairon/cmd`. Record in the sentinel: the coverage figure (architect measured 63.2%), the count of test functions (78 `func Test` declarations), and that the package passes. Confirm all 21 target functions and all named partners exist.
2. **flag-table-eval** (builder, `eval_test.go`). Add the flag-row helper. Convert `TestEvalFlagsRegistered` to the table with rows `eval:--backend`, `eval:--evals-dir`, `eval:--prompt-file`, `eval:--repeat`, `eval:--filter`, `eval:--strict`. Delete `TestEvalPromptFileFlagRegistered`, `TestEvalRepeatFlagRegistered`, `TestEvalFilterAndStrictFlagsRegistered`. Existing two assertions keep every check they had.
3. **flag-table-cli** (builder, `eval_test.go`). Add rows `check:--checks`, `check:--dir`, `check:--backend`, `audit:--iterations-dir`, `audit:--prompt-file`, `audit:--evals-dir`, `trend:--json`, `trend:--evals-dir`, `inference-exec:--backend`, `inference-exec:hidden`. The old tests in the other files are still present.
4. **backend-and-args** (builder, `eval_test.go`). Convert `TestEvalUnknownBackendRejected` to the mode table and delete `TestEvalUnknownBackendRejectedForPerfAndCleanup`. Delete `TestEvalSelfTestSucceedsWithDefaults`. Add `TestEvalAgentSubcommandArgs` with subtests `audit`, `trend`, `verify-log`.
5. **prune-check** (builder, `check_test.go`). Delete `TestCheckCommandRegistered` and `TestCheckFlagsRegistered`; fix imports.
6. **prune-audit** (builder, `eval_audit_test.go`). Delete the five functions; fix imports.
7. **prune-trend** (builder, `eval_trend_test.go`). Delete the three functions.
8. **prune-inference-exec** (builder, `inference_exec_test.go`). Delete `TestInferenceExecHidden`.
9. **verify-log-tables** (builder, `eval_verify_log_test.go`). Add `TestVerifyLogFlags`; convert `TestVerifyLogNoRefactorFailsByDefault` and `TestVerifyLogValidSet` to tables; delete the five former functions.
10. **final-validate** (validator). Run the whole gate (see the environment note for `task test`) and the mechanical checks of AC1-AC5. Check that only the six files changed under `cmd` and `internal`, that no unlisted test function's body changed, and that the removed set is exactly the 21 names and the added set exactly the two new table tests.

### Notes for the builders

- Do not change any helper (`runCheck`, `runAudit`, `runTrend`, `runVerifyLog`, `evalSelfTestProject`, ...) or any unlisted test. Only imports and the listed functions change.
- Do not edit this spec or other files under `.kairon/specs/`.
- Run `gofmt -l cmd/kairon/cmd` and `go vet ./cmd/kairon/cmd` before ending a task.
- Where a row replaces an old assertion, copy the old failure message text so a failure reads the same.
- Any difference from this spec is a deviation: put a code comment citing the spec section and the reason at the point of departure, and report it in the sentinel so krew-lead can copy it into the PR.

## Validation Commands

```bash
# AC1: nothing is printed
grep -nE 'func (TestAuditHelpDescribesEvidenceAndDuplicates|TestAuditHelpDescribesRubricRules|TestAuditSkillsFSWiredFromTemplates|TestCheckCommandRegistered|TestEvalSelfTestSucceedsWithDefaults|TestTrendRestoresFlagVars)\(' cmd/kairon/cmd/*_test.go

# AC2: nothing is printed, then the partners pass
grep -nE 'func (TestAuditFlagsRegistered|TestCheckFlagsRegistered|TestEvalFilterAndStrictFlagsRegistered|TestEvalPromptFileFlagRegistered|TestEvalRepeatFlagRegistered|TestEvalUnknownBackendRejectedForPerfAndCleanup|TestInferenceExecHidden|TestTrendFlagsRegistered|TestVerifyLogNoRefactorPassesWithoutRequireCycle|TestVerifyLogValidSetWithoutRequireCycle)\(' cmd/kairon/cmd/*_test.go
go test -run '^TestEvalFlagsRegistered$' -v ./cmd/kairon/cmd
go test -run '^TestEvalUnknownBackendRejected$' -v ./cmd/kairon/cmd
go test -run '^TestVerifyLogNoRefactorFailsByDefault$' -v ./cmd/kairon/cmd
go test -run '^TestVerifyLogValidSet$' -v ./cmd/kairon/cmd

# AC3: nothing is printed, then one subtest per former test
grep -nE 'func (TestAuditArgs|TestTrendArgs|TestVerifyLogFlagsRegistered|TestVerifyLogMissingAgentIsUsageError|TestVerifyLogRequireCycleFlagParsed)\(' cmd/kairon/cmd/*_test.go
go test -run '^TestEvalAgentSubcommandArgs$' -v ./cmd/kairon/cmd
go test -run '^TestVerifyLogFlags$' -v ./cmd/kairon/cmd

# AC4: at least 57.7% (architect baseline 63.2%)
go test -cover ./cmd/kairon/cmd

# AC5 (in the sandbox worktree the clean-checkout form in the plan's final-validate task is used; see the environment note)
task test
task lint
task fmt:check
task sync:check
```

## Pull request description (for krew-lead)

- Name the two new table tests: `TestEvalAgentSubcommandArgs` (arity over `audit`, `trend`, `verify-log`) and `TestVerifyLogFlags` (`registered` and `require-cycle-bound`). Name the partners that gained rows or subtests: `TestEvalFlagsRegistered`, `TestEvalUnknownBackendRejected`, `TestVerifyLogNoRefactorFailsByDefault`, `TestVerifyLogValidSet`.
- State the coverage before and after (`63.2%` before; the floor is `57.7%`).
- Say which tests were relied on (the partners above), as AGENTS.md requires for refactors.
- Include `Deferred work: none` and the `Spec deviations` section (`Spec deviations: none` unless a builder reported one).
- Close with `Closes #435`.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "baseline"
    agent: "validator"
    description: "Record the starting state of cmd/kairon/cmd before any change: package coverage, the list of test functions, and that all 21 target tests and their partners exist and the package passes."
    dependencies: []
    acceptance_criteria:
      - "go test -cover ./cmd/kairon/cmd passes and the sentinel records the coverage figure (architect measured 63.2%, issue floor 57.7%)"
      - "The sentinel records the number of test functions in the package (architect counted 78 func Test declarations)"
      - "All 21 target functions listed in the spec's Scope Coverage rows T1-T21 exist, at the files the issue names"
      - "All partners exist: TestEvalFlagsRegistered, TestEvalUnknownBackendRejected, TestEvalPromptFileRecorded, TestVerifyLogNoRefactorFailsByDefault, TestVerifyLogValidSet"
    validation_commands:
      - "go test -cover ./cmd/kairon/cmd"
      - "go test -list ^Test ./cmd/kairon/cmd"
      - "git grep -h -oE '^func Test[A-Za-z0-9_]+' -- 'cmd/kairon/cmd/*_test.go' | wc -l"

  - id: "flag-table-eval"
    agent: "builder"
    description: "In eval_test.go add the shared flag-row helper and convert TestEvalFlagsRegistered into a table. Keep both existing assertions as rows eval:--backend and eval:--evals-dir. Fold in the eval-owned flag tests as rows eval:--prompt-file, eval:--repeat, eval:--filter and eval:--strict, then delete TestEvalPromptFileFlagRegistered, TestEvalRepeatFlagRegistered and TestEvalFilterAndStrictFlagsRegistered. Row contents are in the spec section 'Table: TestEvalFlagsRegistered'; dropped assertions are listed in 'Per-test disposition'. Touch only eval_test.go (add imports bytes and cobra only if this task needs them)."
    dependencies: ["baseline"]
    acceptance_criteria:
      - "TestEvalFlagsRegistered runs subtests named eval:--backend, eval:--evals-dir, eval:--prompt-file, eval:--repeat, eval:--filter, eval:--strict and all pass"
      - "Rows eval:--backend and eval:--evals-dir keep every assertion the old function made (flag lookup, default value, usage lists every inference.Names() entry, persistent lookup, diff inherits)"
      - "Row eval:--prompt-file asserts local not persistent and not inherited by diff; eval:--repeat asserts default 1 and not inherited by diff; eval:--filter asserts persistent and inherited by diff and trend; eval:--strict asserts bool, default false, not persistent, not inherited by diff or trend"
      - "Each row with an assertion was seen to fail once against a deliberately wrong expected value before the value was restored"
      - "TestEvalPromptFileFlagRegistered, TestEvalRepeatFlagRegistered and TestEvalFilterAndStrictFlagsRegistered no longer exist; no other function in eval_test.go changed"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - 'out=$(go test -run "^TestEvalFlagsRegistered$" -v ./cmd/kairon/cmd) && for s in eval:--backend eval:--evals-dir eval:--prompt-file eval:--repeat eval:--filter eval:--strict; do echo "$out" | grep -qF -- "--- PASS: TestEvalFlagsRegistered/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - '! grep -nE "func (TestEvalPromptFileFlagRegistered|TestEvalRepeatFlagRegistered|TestEvalFilterAndStrictFlagsRegistered)\(" cmd/kairon/cmd/*_test.go'
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "flag-table-cli"
    agent: "builder"
    description: "In eval_test.go add the rows folded in from other files to TestEvalFlagsRegistered: check:--checks, check:--dir, check:--backend (command found with rootCmd.Find, required annotation via cobraRequiredAnnotation), audit:--iterations-dir, audit:--prompt-file, audit:--evals-dir, trend:--json, trend:--evals-dir, inference-exec:--backend and inference-exec:hidden (Hidden is true and rootCmd.Help() output does not contain inference-exec; restore rootCmd.SetOut in t.Cleanup). Do not delete anything in the other files yet. Row contents are in the spec section 'Table: TestEvalFlagsRegistered'."
    dependencies: ["flag-table-eval"]
    acceptance_criteria:
      - "TestEvalFlagsRegistered additionally runs subtests check:--checks, check:--dir, check:--backend, audit:--iterations-dir, audit:--prompt-file, audit:--evals-dir, trend:--json, trend:--evals-dir, inference-exec:--backend, inference-exec:hidden and all pass"
      - "check:--checks and check:--dir assert the required annotation equals [true]; check:--backend asserts default inference.NameKiroCLI and a usage that lists every inference.Names() entry; the check command is resolved with rootCmd.Find and its Name() must be check"
      - "audit and trend rows assert the defaults, local lookups and inherited --evals-dir listed in the spec's per-test disposition"
      - "inference-exec:hidden asserts both inferenceExecCmd.Hidden and that root help omits inference-exec, and leaves rootCmd output restored"
      - "Each new row with an assertion was seen to fail once against a deliberately wrong expected value before the value was restored"
      - "The old tests in the other files are untouched by this task and still pass"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - 'out=$(go test -run "^TestEvalFlagsRegistered$" -v ./cmd/kairon/cmd) && for s in check:--checks check:--dir check:--backend audit:--iterations-dir audit:--prompt-file audit:--evals-dir trend:--json trend:--evals-dir inference-exec:--backend inference-exec:hidden; do echo "$out" | grep -qF -- "--- PASS: TestEvalFlagsRegistered/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "backend-and-args"
    agent: "builder"
    description: "In eval_test.go: (1) turn TestEvalUnknownBackendRejected into a table over mode with subtests selftest, perf and cleanup, carrying the assertions of TestEvalUnknownBackendRejectedForPerfAndCleanup into the perf and cleanup rows, and delete that function; (2) delete TestEvalSelfTestSucceedsWithDefaults (its baseline run is covered by TestEvalPromptFileRecorded); (3) add TestEvalAgentSubcommandArgs with subtests audit, trend and verify-log, each asserting Args errors for no argument and for two arguments and accepts one argument."
    dependencies: ["flag-table-cli"]
    acceptance_criteria:
      - "TestEvalUnknownBackendRejected has subtests selftest, perf and cleanup, all pass; selftest keeps the check that the error contains nope, kiro-cli and stub; perf and cleanup assert an unknown backend error; evalBackend, evalPerf and evalCleanup are restored and do not leak between subtests"
      - "TestEvalAgentSubcommandArgs has subtests audit, trend and verify-log, all pass, each asserting error for nil args, error for two args, no error for one arg"
      - "TestEvalUnknownBackendRejectedForPerfAndCleanup and TestEvalSelfTestSucceedsWithDefaults no longer exist"
      - "TestEvalPromptFileRecorded, TestEvalSelfTestFailExitsWithThresholdError and TestEvalRepeatPassedToRun are unchanged"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - 'out=$(go test -run "^TestEvalUnknownBackendRejected$" -v ./cmd/kairon/cmd) && for s in selftest perf cleanup; do echo "$out" | grep -qF -- "--- PASS: TestEvalUnknownBackendRejected/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - 'out=$(go test -run "^TestEvalAgentSubcommandArgs$" -v ./cmd/kairon/cmd) && for s in audit trend verify-log; do echo "$out" | grep -qF -- "--- PASS: TestEvalAgentSubcommandArgs/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - '! grep -nE "func (TestEvalUnknownBackendRejectedForPerfAndCleanup|TestEvalSelfTestSucceedsWithDefaults)\(" cmd/kairon/cmd/*_test.go'
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "prune-check"
    agent: "builder"
    description: "In check_test.go delete TestCheckCommandRegistered and TestCheckFlagsRegistered (their surviving assertions are rows in TestEvalFlagsRegistered). Keep cobraRequiredAnnotation, runCheck, checkFixtures and every other test unchanged. Remove any import left unused."
    dependencies: ["backend-and-args"]
    acceptance_criteria:
      - "TestCheckCommandRegistered and TestCheckFlagsRegistered no longer exist"
      - "cobraRequiredAnnotation is still defined (the table uses it)"
      - "No other function in check_test.go changed; imports are used and the package builds"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - '! grep -nE "func (TestCheckCommandRegistered|TestCheckFlagsRegistered)\(" cmd/kairon/cmd/*_test.go'
      - "grep -n cobraRequiredAnnotation cmd/kairon/cmd/check_test.go"
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "prune-audit"
    agent: "builder"
    description: "In eval_audit_test.go delete TestAuditHelpDescribesEvidenceAndDuplicates, TestAuditHelpDescribesRubricRules, TestAuditFlagsRegistered, TestAuditArgs and TestAuditSkillsFSWiredFromTemplates. Remove the errors import that becomes unused (io/fs stays, useShippedSkills needs it). Leave every other test and helper unchanged."
    dependencies: ["backend-and-args"]
    acceptance_criteria:
      - "The five functions no longer exist"
      - "TestAuditArgs's checks are the audit subtest of TestEvalAgentSubcommandArgs and TestAuditFlagsRegistered's surviving checks are the audit:* rows of TestEvalFlagsRegistered (both already passing)"
      - "No other function or helper in eval_audit_test.go changed; imports are used and the package builds"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - '! grep -nE "func (TestAuditHelpDescribesEvidenceAndDuplicates|TestAuditHelpDescribesRubricRules|TestAuditFlagsRegistered|TestAuditArgs|TestAuditSkillsFSWiredFromTemplates)\(" cmd/kairon/cmd/*_test.go'
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "prune-trend"
    agent: "builder"
    description: "In eval_trend_test.go delete TestTrendFlagsRegistered, TestTrendRestoresFlagVars and TestTrendArgs. Leave runTrend and every other test unchanged."
    dependencies: ["backend-and-args"]
    acceptance_criteria:
      - "The three functions no longer exist"
      - "TestTrendArgs's checks are the trend subtest of TestEvalAgentSubcommandArgs and TestTrendFlagsRegistered's surviving checks are the trend:* rows of TestEvalFlagsRegistered (both already passing)"
      - "No other function or helper in eval_trend_test.go changed; imports are used and the package builds"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - '! grep -nE "func (TestTrendFlagsRegistered|TestTrendRestoresFlagVars|TestTrendArgs)\(" cmd/kairon/cmd/*_test.go'
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "prune-inference-exec"
    agent: "builder"
    description: "In inference_exec_test.go delete TestInferenceExecHidden (its assertions are the inference-exec:hidden and inference-exec:--backend rows of TestEvalFlagsRegistered). Leave runInferenceExec and every other test unchanged."
    dependencies: ["backend-and-args"]
    acceptance_criteria:
      - "TestInferenceExecHidden no longer exists"
      - "No other function in inference_exec_test.go changed; imports are used and the package builds"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - '! grep -nE "func TestInferenceExecHidden\(" cmd/kairon/cmd/*_test.go'
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "verify-log-tables"
    agent: "builder"
    description: "In eval_verify_log_test.go: add TestVerifyLogFlags (subtest registered running the four rows iterations-dir, min-iterations, require-cycle, evals-dir through the shared flag-row helper from eval_test.go, and subtest require-cycle-bound holding the body of TestVerifyLogRequireCycleFlagParsed); turn TestVerifyLogNoRefactorFailsByDefault into a two-row table (require-cycle=true, require-cycle=false) and TestVerifyLogValidSet into a loop over both require-cycle values; delete TestVerifyLogFlagsRegistered, TestVerifyLogNoRefactorPassesWithoutRequireCycle, TestVerifyLogValidSetWithoutRequireCycle, TestVerifyLogRequireCycleFlagParsed and TestVerifyLogMissingAgentIsUsageError. Row contents are in the spec sections 'Per-test disposition', 'The merged behavioural tests' and 'The two Combine tables'."
    dependencies: ["backend-and-args"]
    acceptance_criteria:
      - "TestVerifyLogFlags runs subtests registered (with rows iterations-dir default .kairon/iterations, min-iterations default 2, require-cycle default true with usage containing --require-cycle=false, evals-dir not local and inherited) and require-cycle-bound (flag Set to false updates verifyLogRequireCycle; cleanup restores variable, default and Changed), all pass"
      - "TestVerifyLogNoRefactorFailsByDefault has subtests require-cycle=true (error naming cycle) and require-cycle=false (no error, output contains OK selftest: 3 iterations verified), all pass"
      - "TestVerifyLogValidSet has subtests require-cycle=true and require-cycle=false, each with no error and the OK selftest: 4 iterations verified line (or the documented old assertion for the false row), all pass"
      - "The five former functions no longer exist; the arity checks of TestVerifyLogMissingAgentIsUsageError are the verify-log subtest of TestEvalAgentSubcommandArgs"
      - "Each new row with an assertion was seen to fail once against a deliberately wrong expected value before the value was restored"
      - "No other function or helper in eval_verify_log_test.go changed (TestVerifyLogFailingSets, TestVerifyLogMinIterations, TestVerifyLogNegativeMinIterationsRejected and TestVerifyLogSilencesUsageOnVerificationFailure are untouched)"
      - "gofmt and go vet are clean for cmd/kairon/cmd"
    validation_commands:
      - 'out=$(go test -run "^TestVerifyLogFlags$" -v ./cmd/kairon/cmd) && for s in registered require-cycle-bound; do echo "$out" | grep -qF -- "--- PASS: TestVerifyLogFlags/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - 'out=$(go test -run "^TestVerifyLogNoRefactorFailsByDefault$" -v ./cmd/kairon/cmd) && for s in require-cycle=true require-cycle=false; do echo "$out" | grep -qF -- "--- PASS: TestVerifyLogNoRefactorFailsByDefault/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - 'out=$(go test -run "^TestVerifyLogValidSet$" -v ./cmd/kairon/cmd) && for s in require-cycle=true require-cycle=false; do echo "$out" | grep -qF -- "--- PASS: TestVerifyLogValidSet/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - '! grep -nE "func (TestVerifyLogFlagsRegistered|TestVerifyLogNoRefactorPassesWithoutRequireCycle|TestVerifyLogValidSetWithoutRequireCycle|TestVerifyLogRequireCycleFlagParsed|TestVerifyLogMissingAgentIsUsageError)\(" cmd/kairon/cmd/*_test.go'
      - 'test -z "$(gofmt -l cmd/kairon/cmd)"'
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd"

  - id: "final-validate"
    agent: "validator"
    description: "Verify the whole change against AC1-AC5 and the behaviour-preservation contract: the 6 Delete functions are gone, the 10 Merge functions are gone and their partners pass with the merged cases, the 2 Combine tables exist with one subtest per former test, coverage is at least 57.7% (and is compared with the baseline), the removed function set is exactly the 21 names and the added set exactly TestEvalAgentSubcommandArgs and TestVerifyLogFlags, only the six listed test files changed under cmd and internal, no unlisted test function body changed, and the gate passes."
    dependencies: ["prune-check", "prune-audit", "prune-trend", "prune-inference-exec", "verify-log-tables"]
    acceptance_criteria:
      - "grep for the 6 Delete function names, the 10 Merge function names and the 5 Combine function names across cmd/kairon/cmd/*_test.go prints nothing"
      - "go test -run for TestEvalFlagsRegistered lists 16 passing subtests (eval:--backend, eval:--evals-dir, eval:--prompt-file, eval:--repeat, eval:--filter, eval:--strict, check:--checks, check:--dir, check:--backend, audit:--iterations-dir, audit:--prompt-file, audit:--evals-dir, trend:--json, trend:--evals-dir, inference-exec:--backend, inference-exec:hidden)"
      - "TestEvalUnknownBackendRejected (selftest, perf, cleanup), TestVerifyLogNoRefactorFailsByDefault and TestVerifyLogValidSet (require-cycle=true, require-cycle=false) pass with those subtests"
      - "TestEvalAgentSubcommandArgs lists subtests audit, trend, verify-log and TestVerifyLogFlags lists registered and require-cycle-bound, all passing"
      - "go test -cover ./cmd/kairon/cmd reports at least 57.7%; the sentinel states the figure next to the baseline"
      - "The set difference of test function names between the base commit and the working tree is exactly 21 removed (the names in the Scope Coverage rows T1-T21) and 2 added (TestEvalAgentSubcommandArgs, TestVerifyLogFlags)"
      - "git diff against the merge base shows no change to a non-test file under cmd or internal, and changes only the six listed test files"
      - "Reading the diff of the six files, no unlisted test function body changed (only imports, the listed functions and the new helper and tables)"
      - "task test exits 0 on a clean checkout of HEAD with the working-tree test changes applied (the sandbox worktree itself fails only the seven internal/planner tests named in the environment note, the same as at baseline); go test -race ./cmd/... passes in the worktree; task lint, task fmt:check and task sync:check exit 0"
    validation_commands:
      - '! grep -nE "func (TestAuditHelpDescribesEvidenceAndDuplicates|TestAuditHelpDescribesRubricRules|TestAuditSkillsFSWiredFromTemplates|TestCheckCommandRegistered|TestEvalSelfTestSucceedsWithDefaults|TestTrendRestoresFlagVars)\(" cmd/kairon/cmd/*_test.go'
      - '! grep -nE "func (TestAuditFlagsRegistered|TestCheckFlagsRegistered|TestEvalFilterAndStrictFlagsRegistered|TestEvalPromptFileFlagRegistered|TestEvalRepeatFlagRegistered|TestEvalUnknownBackendRejectedForPerfAndCleanup|TestInferenceExecHidden|TestTrendFlagsRegistered|TestVerifyLogNoRefactorPassesWithoutRequireCycle|TestVerifyLogValidSetWithoutRequireCycle)\(" cmd/kairon/cmd/*_test.go'
      - '! grep -nE "func (TestAuditArgs|TestTrendArgs|TestVerifyLogFlagsRegistered|TestVerifyLogMissingAgentIsUsageError|TestVerifyLogRequireCycleFlagParsed)\(" cmd/kairon/cmd/*_test.go'
      - 'out=$(go test -run "^TestEvalFlagsRegistered$" -v ./cmd/kairon/cmd) && for s in eval:--backend eval:--evals-dir eval:--prompt-file eval:--repeat eval:--filter eval:--strict check:--checks check:--dir check:--backend audit:--iterations-dir audit:--prompt-file audit:--evals-dir trend:--json trend:--evals-dir inference-exec:--backend inference-exec:hidden; do echo "$out" | grep -qF -- "--- PASS: TestEvalFlagsRegistered/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - 'out=$(go test -run "^TestEvalAgentSubcommandArgs$" -v ./cmd/kairon/cmd) && for s in audit trend verify-log; do echo "$out" | grep -qF -- "--- PASS: TestEvalAgentSubcommandArgs/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - 'out=$(go test -run "^TestVerifyLogFlags$" -v ./cmd/kairon/cmd) && for s in registered require-cycle-bound; do echo "$out" | grep -qF -- "--- PASS: TestVerifyLogFlags/$s " || { echo "missing subtest $s"; exit 1; }; done'
      - 'go test -run "^TestEvalUnknownBackendRejected$|^TestVerifyLogNoRefactorFailsByDefault$|^TestVerifyLogValidSet$" -v ./cmd/kairon/cmd'
      - 'bash -o pipefail -c "go test -cover ./cmd/kairon/cmd | grep -oE ''coverage: [0-9.]+'' | cut -d'' '' -f2 | xargs -I{} awk ''BEGIN{exit !({}>=57.7)}''"'
      - 'BASE=$(git merge-base HEAD origin/main); git grep -h -oE "^func Test[A-Za-z0-9_]+" $BASE -- "cmd/kairon/cmd/*_test.go" | sort > /tmp/k435-before.txt; grep -h -oE "^func Test[A-Za-z0-9_]+" cmd/kairon/cmd/*_test.go | sort > /tmp/k435-after.txt; [ "$(comm -23 /tmp/k435-before.txt /tmp/k435-after.txt | wc -l | tr -d " ")" = 21 ] && [ "$(comm -13 /tmp/k435-before.txt /tmp/k435-after.txt | wc -l | tr -d " ")" = 2 ]'
      - 'BASE=$(git merge-base HEAD origin/main); [ -z "$(git diff --name-only $BASE -- cmd internal | grep -v "_test.go$")" ]'
      - "go test -race ./cmd/..."
      - 'D=$(mktemp -d); git worktree add --detach $D/w HEAD >/dev/null && git diff HEAD -- cmd internal | git -C $D/w apply --allow-empty && (cd $D/w && task test); rc=$?; git worktree remove --force $D/w; exit $rc'
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
```
