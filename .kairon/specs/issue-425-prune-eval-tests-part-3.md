# Design spec: Prune redundant internal/eval tests, part 3 of 5

Closes #425

## Scope Coverage

In Scope has one item (a single list entry naming the files). Acceptance Criteria are numbered 1 to 4. The 29 rows of the issue's Current / Target State table (12 Delete, 17 Merge) are listed as T1 to T29 so each is traceable to a task.

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/eval` test files: `filter_test.go`, `judge_check_test.go`, `metadata_test.go`, `mockcli_test.go`, `multi_turn_test.go`, `perf_test.go`, `planner_suite_test.go`, `pricing_cost_test.go`, `provenance_test.go`, `provenance_workflow_test.go`, `repeat_exec_test.go`, plus `execute_case_test.go` (partner `TestConfigureKeepWorkspaces`) and `checks_scoring_test.go` (partner `TestChecksScoringNoOutputOrWorkspaceScoresZero`), which are edited only to receive merged cases" | `prune-filter-tests`, `prune-judge-check-tests`, `prune-metadata-tests`, `prune-provenance-tests`, `prune-remaining-tests` |
| AC1 | "The 12 tests marked Delete no longer exist." | `prune-metadata-tests`, `prune-provenance-tests`, `prune-remaining-tests`, `verify-prune` |
| AC2 | "Each of the 17 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-filter-tests`, `prune-judge-check-tests`, `prune-metadata-tests`, `prune-provenance-tests`, `prune-remaining-tests`, `verify-prune` |
| AC3 | "Package coverage does not drop." | `verify-prune` |
| AC4 | "Every remaining test passes unchanged, and the gate passes." | `verify-prune` |
| T1 | Current / Target State: "TestFilterParseTrimsWhitespace" (Merge into `TestFilterParseOperators`) | `prune-filter-tests` |
| T2 | Current / Target State: "TestFilterParseOperatorChoosesLongest" (Merge into `TestFilterParseOperators`) | `prune-filter-tests` |
| T3 | Current / Target State: "TestFilterMatchMissingKeyNeverMatches" (Merge into `TestFilterMatchEquality`) | `prune-filter-tests` |
| T4 | Current / Target State: "TestFilterEqualityNeverParsesNumbers" (Merge into `TestFilterMatchEquality`) | `prune-filter-tests` |
| T5 | Current / Target State: "TestFilterBuiltInKeysAndMetadataKeys" (Merge into `TestFilterResultLookup`) | `prune-filter-tests` |
| T6 | Current / Target State: "TestJudgeCheckDebugOffWritesNothing" (Merge into `TestJudgeCheckDebugPrintsPrompt`) | `prune-judge-check-tests` |
| T7 | Current / Target State: "TestJudgeCheckConfigureCarriesDebug" (Merge into `TestConfigureKeepWorkspaces`) | `prune-judge-check-tests` |
| T8 | Current / Target State: "TestJudgeCheckEmptyOutputMakesNoJudgeCall" (Merge into `TestChecksScoringNoOutputOrWorkspaceScoresZero`) | `prune-judge-check-tests` |
| T9 | Current / Target State: "TestJudgeCheckPromptOmitsCaseExpectations" (Merge into `TestJudgeCheckRequestAndPrompt`) | `prune-judge-check-tests` |
| T10 | Current / Target State: "TestMetadataAbsentLoadsAsBefore" (Delete) | `prune-metadata-tests` |
| T11 | Current / Target State: "TestMetadataReservedKeyErrorExplains" (Merge into `TestMetadataInvalidFailsLoadCases`) | `prune-metadata-tests` |
| T12 | Current / Target State: "TestMetadataValueErrorSaysWhatIsAllowed" (Merge into `TestMetadataInvalidFailsLoadCases`) | `prune-metadata-tests` |
| T13 | Current / Target State: "TestMetadataUnmarshalYAMLDirect" (Delete) | `prune-metadata-tests` |
| T14 | Current / Target State: "TestMetadataCaseResultJSON" (Delete) | `prune-metadata-tests` |
| T15 | Current / Target State: "TestSuiteSHA256ChangesOnMetadataOnlyEdit" (Delete) | `prune-metadata-tests` |
| T16 | Current / Target State: "TestMockCLIDefaultDataDirIsWorkspaceMocks" (Delete) | `prune-remaining-tests` |
| T17 | Current / Target State: "TestMultiTurnClassicCaseSendsTurnZero" (Merge into `TestMultiTurnClassicCaseHasNoTurnFields`) | `prune-remaining-tests` |
| T18 | Current / Target State: "TestStartupOverheadMeasurement" (Delete) | `prune-remaining-tests` |
| T19 | Current / Target State: "TestPlannerSuiteCaseFilesDeclarePlannerAgent" (Delete) | `prune-remaining-tests` |
| T20 | Current / Target State: "TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments" (Delete) | `prune-remaining-tests` |
| T21 | Current / Target State: "TestPlannerSuiteRegexesCompile" (Delete) | `prune-remaining-tests` |
| T22 | Current / Target State: "TestNilRunPinsPrices" (Delete) | `prune-remaining-tests` |
| T23 | Current / Target State: "TestProvenanceHashResourceOrderMatters" (Merge into `TestProvenanceHashChangesWithPromptResourceAndConfig`) | `prune-provenance-tests` |
| T24 | Current / Target State: "TestProvenanceHashFramingIsUnambiguous" (Merge into `TestProvenanceHashChangesWithPromptResourceAndConfig`) | `prune-provenance-tests` |
| T25 | Current / Target State: "TestProvenanceTypesSerialisation" (Delete) | `prune-provenance-tests` |
| T26 | Current / Target State: "TestProvenanceDispatcherEditChangesBothHashes" (Merge into `TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash`) | `prune-provenance-tests` |
| T27 | Current / Target State: "TestWorkflowHashSwappingStepPromptsChangesHash" (Merge into `TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash`) | `prune-provenance-tests` |
| T28 | Current / Target State: "TestAgentConfigFileTrustedAgents" (Delete) | `prune-provenance-tests` |
| T29 | Current / Target State: "TestRepeatExecutionRejectsRepeatWithCleanupAndPerf" (Merge into `TestRepeatExecutionRunOptionsValidation`) | `prune-remaining-tests` |

## Deferred work

none

## Solution approach

This is a test-only change in package `internal/eval`. No production code, docs or templates change. The work is 12 deletions and 17 merges in 13 test files. A merge folds the old test's distinct input or assertion into the named partner as a table row or `t.Run` subtest, and drops only what the partner already asserts. A delete removes the function with no replacement.

Findings from reading the code (all checked against the current worktree, which is at `3828534`):

- All 29 named tests exist at the lines the issue gives. All 21 distinct partners exist, so nothing in the issue is impossible or contradictory.
- No live file outside the test files and the frozen audit records references the 29 names. The one exception is a doc comment above `TestRepeatExecutionRejectsRepeatWithCleanupAndPerf` in `repeat_exec_test.go`, which goes away with that function.
- Coverage baseline: `go test -coverprofile ./internal/eval` at this commit reports **87.6%** (415 s). The same run with the 12 Delete tests skipped (`go test -skip '^(...)$'`) reports 87.6% and produces a byte-identical coverage profile. Deleting the 12 tests therefore leaves every covered statement covered. The 17 merges keep every assertion, so they cannot lower coverage as long as each merged input still executes. No item is blocked by the 87.6% floor.
- `AGENTS.md` TDD rule for refactors: each partner is run green before it is edited (characterization), and the same tests pass after. The change is mostly removal, so no new failing test is written first. The PR description must name the partners relied on.

### Merge mechanics (applies to every Merge)

1. Keep every assertion the partner already had. Fold in only the old test's distinct input or assertion.
2. Make the merged case a named subtest so it shows up in `go test -run '^<partner>$' -v`. When the partner is a table, add rows (adding an optional `name` field to the row struct is fine, and when it is empty the row keeps its current `t.Run` name). When the partner is a single straight-line body, move the existing body into one `t.Run` and add the merged case as another `t.Run`. Do not nest `chdirTemp`/`provProject`/`execEnv` setup at two levels (the existing body goes into its own subtest so setup runs once per subtest).
3. Subtest names contain the **token** in the table below, written with spaces in the source (Go prints spaces as underscores in `-v` output, so the token is written with underscores here). The validator greps `--- PASS: <Partner>/...<token>` in the `-v` output.
4. If a merged case changes global config (`cfg`, `debugWriter`), restore it with `t.Cleanup(resetConfig)` or the helper the old test used. A leaked `cfg.debug` or `cfg.backend` would change later tests.
5. After removing functions, remove imports and test-only helpers that became unused only because of this change. `go vet ./internal/eval` and `gofmt -l internal/eval` must be clean.

### Per-merge instructions

| # | Old test | Partner | What to fold in | `-v` token |
|---|----------|---------|-----------------|------------|
| T1 | `TestFilterParseTrimsWhitespace` | `TestFilterParseOperators` | Two rows: `"  speed  ==  fast ,  slow  "` gives `Filter{speed, ==, [fast slow]}` and `" n >= 3 "` gives `Filter{n, >=, [3]}` | `trims_whitespace` |
| T2 | `TestFilterParseOperatorChoosesLongest` | `TestFilterParseOperators` | One row: `"a<=>b"` gives `Filter{a, <=, [>b]}` | `longest_operator` |
| T3 | `TestFilterMatchMissingKeyNeverMatches` | `TestFilterMatchEquality` | Nine rows with want=false for `absent==x`, `absent<>x`, `absent!=x`, `absent<1`, `absent<=1`, `absent>1`, `absent>=1`, `absent==x,y`, `absent<>x,y`. The shared `look` map already has no `absent` key | `missing_key` |
| T4 | `TestFilterEqualityNeverParsesNumbers` | `TestFilterMatchEquality` | Add `"retries": "3"` to the `look` map. Rows: `retries==3.0` false, `retries==03` false, `retries<>3.0` true, `retries==3` true. No existing row uses `retries` | `textual_equality` |
| T5 | `TestFilterBuiltInKeysAndMetadataKeys` | `TestFilterResultLookup` | One table over both lookups: the existing `CaseResult` via `resultLookup` (agent, name, speed, skill present; `absent` missing; the legacy-result filter assertions) plus `TestCase.filterValue` (tc `n1`/`alpha` with skill+speed: all four keys present, `absent` missing; bare tc `n2`/`beta`: agent and name present, skill missing). `tc.filterValue` has the same `func(string) (string, bool)` shape as the result lookup | `TestCase_` (subtest names for the TestCase rows start with `TestCase`) |
| T6 | `TestJudgeCheckDebugOffWritesNothing` | `TestJudgeCheckDebugPrintsPrompt` | Turn the partner into two rows, `debug on` (existing body) and `debug off` (same `captureDebug` setup, no `cfg.debug`, one judge check, assert `buf.Len() == 0`). Call `useRecording` inside each row so config is reset per row | `debug_off` |
| T7 | `TestJudgeCheckConfigureCarriesDebug` (file `judge_check_test.go`) | `TestConfigureKeepWorkspaces` (file `execute_case_test.go`) | A `debug flag` row in the configure table: `configure(RunOptions{Backend: "stub", Debug: true})` sets `cfg.debug`, then `configure(RunOptions{Backend: "stub"})` clears it. Keep `t.Cleanup(resetConfig)` from the old test. The existing keep-workspaces check becomes a `keep workspaces` row with its current assertions | `debug_flag` |
| T8 | `TestJudgeCheckEmptyOutputMakesNoJudgeCall` (file `judge_check_test.go`) | `TestChecksScoringNoOutputOrWorkspaceScoresZero` (file `checks_scoring_test.go`) | A row `empty output with judge checks makes no judge call`: two `CheckJudge` checks on `clarity`, `ActualOutput: ""`, a `yesBackend()` installed via `useRecording`, asserting `len(b.reqs) == 0`, `len(cr.Calls) == 0`, score 0 of max 2, reasoning contains `checks not run`. Do not force the partner's existing `Deterministic` assertion onto this row unless production sets it for judge criteria. The partner's two existing rows keep their assertions. Add the judge assertion to the `no workspace` row only if production behaves that way; otherwise leave that row alone | `no_judge_call` |
| T9 | `TestJudgeCheckPromptOmitsCaseExpectations` | `TestJudgeCheckRequestAndPrompt` | Set `ExpectedOutput: "SECRET-EXPECTED"` on the partner's case. Add subtest `prompt omits expected output` asserting the recorded prompt does not contain it. All existing assertions stay | `omits_expected_output` |
| T11 | `TestMetadataReservedKeyErrorExplains` | `TestMetadataInvalidFailsLoadCases` | Add an optional `explain` column to the row struct. Row `reserved agent` gets `explain: "reserved"` and the loop asserts the message contains it when non-empty | `reserved_agent` (existing row) |
| T12 | `TestMetadataValueErrorSaysWhatIsAllowed` | `TestMetadataInvalidFailsLoadCases` | Row `list value` gets `explain: "string, number or boolean"` | `list_value` (existing row) |
| T17 | `TestMultiTurnClassicCaseSendsTurnZero` | `TestMultiTurnClassicCaseHasNoTurnFields` | Install `useRecordingBackend(t)` before `executeCase` (it wraps the stub, so the output stays `done`). Keep the existing assertions in a `no turn fields` subtest, and add subtest `turn zero request` asserting exactly one request, `Turn == 0`, `Prompt == "do it"` | `turn_zero_request` |
| T23 | `TestProvenanceHashResourceOrderMatters` | `TestProvenanceHashChangesWithPromptResourceAndConfig` | Subtest `resource order`: project with resources `one.md` ("1") and `two.md` ("2"), hash, rewrite `a.json` with the resources swapped, assert the hash changed. The partner's existing body (prompt edit, revert, resource edit, config edit) moves into its own subtest | `resource_order` |
| T24 | `TestProvenanceHashFramingIsUnambiguous` | `TestProvenanceHashChangesWithPromptResourceAndConfig` | Subtest `byte shift between adjacent resources`: `one.md`="ab", `two.md`="c" then `one.md`="a", `two.md`="bc"; assert the hash changed | `byte_shift` |
| T26 | `TestProvenanceDispatcherEditChangesBothHashes` | `TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash` | Subtest `dispatcher edit` with its own `wfRoot(t)`: edit `lead-prompt.md`, assert both `PromptSHA256` and `WorkflowSHA256` changed. The partner's existing body (sub-agent prompt edit, step-b unchanged, resource edit) moves into a `sub-agent edit` subtest unchanged | `dispatcher_edit` |
| T27 | `TestWorkflowHashSwappingStepPromptsChangesHash` | `TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash` | Subtest `swap step prompts` with its own `wfRoot(t)`: swap `step-a-prompt.md` and `step-b-prompt.md` contents, assert `WorkflowSHA256` changed | `swap_step_prompts` |
| T29 | `TestRepeatExecutionRejectsRepeatWithCleanupAndPerf` | `TestRepeatExecutionRunOptionsValidation` | Two rows, `repeat with cleanup` = `RunOptions{Repeat: 2, Cleanup: true}` and `repeat with perf` = `RunOptions{Repeat: 2, Perf: true}`, want `"repeat"`. The loop already asserts the error text and that no `results` dir is created | `repeat_with_cleanup`, `repeat_with_perf` |

### Delete notes

Each delete removes the function with no replacement. For nine of them the partner named in the issue already covers the behavior, and that partner must still pass. Three have no partner (`TestStartupOverheadMeasurement`, `TestPlannerSuiteCaseFilesDeclarePlannerAgent`, `TestAgentConfigFileTrustedAgents`). The issue's reason for each is that the test checks a backend-independent range, an inert YAML field, or Go's `encoding/json`, so nothing needs to replace it.

| # | Deleted test | Partner that must still pass |
|---|--------------|------------------------------|
| T10 | `TestMetadataAbsentLoadsAsBefore` | `TestMetadataDoesNotChangeExecutionOrScoring` |
| T13 | `TestMetadataUnmarshalYAMLDirect` | `TestMetadataLoadsScalarTextForms` |
| T14 | `TestMetadataCaseResultJSON` | `TestResultsRecordMetadataForEveryRepeat` |
| T15 | `TestSuiteSHA256ChangesOnMetadataOnlyEdit` | `TestSuiteSHA256HashesRawCaseBytes` |
| T16 | `TestMockCLIDefaultDataDirIsWorkspaceMocks` | `TestMockCLIReplyLookupOrder` |
| T18 | `TestStartupOverheadMeasurement` | none |
| T19 | `TestPlannerSuiteCaseFilesDeclarePlannerAgent` | none |
| T20 | `TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments` | `TestPlannerSuiteRedLegacyTranscriptsFail` |
| T21 | `TestPlannerSuiteRegexesCompile` | `TestPlannerSuiteGreenReferenceTranscriptsPass` |
| T22 | `TestNilRunPinsPrices` | `TestCostFromUsageUnpinnedPricesNothing` |
| T25 | `TestProvenanceTypesSerialisation` | `TestCallRecordsSelfTestProvenance` |
| T28 | `TestAgentConfigFileTrustedAgents` | none |

Likely import fallout: `metadata_test.go` (`yaml`, `json`, `reflect` if no longer used), `perf_test.go` (`time`), `planner_suite_test.go` (`yaml`, `checks`, `regexp`, `os`, `filepath`, and helpers such as `readLegacyTurns` only if nothing else calls them), `provenance_test.go` (`json`, `strings`), `provenance_workflow_test.go` (`json`, `strings`). The builder confirms each with the compiler, and removes an unused helper only when this change is what made it unused.

## Relevant files

All under `internal/eval/`, all `_test.go`, all modified (nothing created):

- `filter_test.go`
- `judge_check_test.go`
- `execute_case_test.go` (receives T7)
- `checks_scoring_test.go` (receives T8)
- `metadata_test.go`
- `mockcli_test.go`
- `multi_turn_test.go`
- `perf_test.go`
- `planner_suite_test.go`
- `pricing_cost_test.go`
- `provenance_test.go`
- `provenance_workflow_test.go`
- `repeat_exec_test.go`

Read-only references: `.kairon/research/test-audit-verdicts.md` (per-test verdicts), `.kairon/research/test-performance-analysis.md` (§8, baseline 87.6%). Neither is edited. `.kiro/agents/*.json` show as modified in the worktree before this work started. They are not part of this change and must not be staged by the builders.

## Team orchestration

- All six tasks edit one Go package. A half-edited file (for example an unused import after a delete) stops the whole package from compiling, which would make a sibling builder's `go vet` and `go test -run` fail for reasons unrelated to its work. The five builder tasks therefore run in a chain (`prune-filter-tests` then `prune-judge-check-tests` then `prune-metadata-tests` then `prune-provenance-tests` then `prune-remaining-tests`). Each one ends with the package compiling and its own partners green.
- Each builder runs its partners green before editing (characterization, per `AGENTS.md`) and again after.
- `verify-prune` (validator) runs last. It is read-only and runs the issue's grep checks, every partner with `-v`, the coverage run, and `task test`. Expect `go test -cover ./internal/eval` to take about 7 to 8 minutes and `task test` (which adds `-race` and every package) to take longer, so krew-lead should allow for that timeout.
- No documenter task: no doc, template or live prompt text changes.
- The PR description must state `Deferred work: none` and `Spec deviations: none` (unless a builder records a deviation in its sentinel), and name the partner tests relied on as the characterization tests.

## Step-by-step task breakdown

### Task 1: `prune-filter-tests` (builder)
Edit `filter_test.go` for T1 to T5. Acceptance: the five old functions are gone, the three partners pass and list the tokens `trims_whitespace`, `longest_operator`, `missing_key`, `textual_equality`, `TestCase_`. Depends on nothing.

### Task 2: `prune-judge-check-tests` (builder)
Edit `judge_check_test.go` (remove T6, T7, T8, T9 and extend `TestJudgeCheckDebugPrintsPrompt`, `TestJudgeCheckRequestAndPrompt`), `execute_case_test.go` (T7 row in `TestConfigureKeepWorkspaces`) and `checks_scoring_test.go` (T8 row in `TestChecksScoringNoOutputOrWorkspaceScoresZero`). Depends on Task 1.

### Task 3: `prune-metadata-tests` (builder)
Edit `metadata_test.go`: delete T10, T13, T14, T15; merge T11, T12 via an `explain` column. Fix imports. Depends on Task 2.

### Task 4: `prune-provenance-tests` (builder)
Edit `provenance_test.go` (T23, T24 merges, T25 delete) and `provenance_workflow_test.go` (T26, T27 merges, T28 delete). Depends on Task 3.

### Task 5: `prune-remaining-tests` (builder)
Delete T16 (`mockcli_test.go`), T18 (`perf_test.go`), T19, T20, T21 (`planner_suite_test.go`), T22 (`pricing_cost_test.go`); merge T17 (`multi_turn_test.go`) and T29 (`repeat_exec_test.go`). Depends on Task 4.

### Task 6: `verify-prune` (validator)
Run the issue's two grep checks verbatim, each partner with `-v`, the coverage run, `task fmt:check`, `task lint` and `task test`. Depends on Task 5.

## Validation commands

```bash
# AC1: prints nothing
grep -nE 'func (TestAgentConfigFileTrustedAgents|TestMetadataAbsentLoadsAsBefore|TestMetadataCaseResultJSON|TestMetadataUnmarshalYAMLDirect|TestMockCLIDefaultDataDirIsWorkspaceMocks|TestNilRunPinsPrices|TestPlannerSuiteCaseFilesDeclarePlannerAgent|TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments|TestPlannerSuiteRegexesCompile|TestProvenanceTypesSerialisation|TestStartupOverheadMeasurement|TestSuiteSHA256ChangesOnMetadataOnlyEdit)\(' internal/eval/*_test.go

# AC2: prints nothing
grep -nE 'func (TestFilterBuiltInKeysAndMetadataKeys|TestFilterEqualityNeverParsesNumbers|TestFilterMatchMissingKeyNeverMatches|TestFilterParseOperatorChoosesLongest|TestFilterParseTrimsWhitespace|TestJudgeCheckConfigureCarriesDebug|TestJudgeCheckDebugOffWritesNothing|TestJudgeCheckEmptyOutputMakesNoJudgeCall|TestJudgeCheckPromptOmitsCaseExpectations|TestMetadataReservedKeyErrorExplains|TestMetadataValueErrorSaysWhatIsAllowed|TestMultiTurnClassicCaseSendsTurnZero|TestProvenanceDispatcherEditChangesBothHashes|TestProvenanceHashFramingIsUnambiguous|TestProvenanceHashResourceOrderMatters|TestRepeatExecutionRejectsRepeatWithCleanupAndPerf|TestWorkflowHashSwappingStepPromptsChangesHash)\(' internal/eval/*_test.go

# AC2: each partner passes and lists the merged case (token from the per-merge table)
go test ./internal/eval -count=1 -v -run '^TestFilterParseOperators$'

# AC3: reports at least 87.6%
go test -cover ./internal/eval

# AC4
task test
```

## Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "prune-filter-tests"
    agent: "builder"
    description: "In internal/eval/filter_test.go, merge TestFilterParseTrimsWhitespace and TestFilterParseOperatorChoosesLongest into TestFilterParseOperators as rows, merge TestFilterMatchMissingKeyNeverMatches and TestFilterEqualityNeverParsesNumbers into TestFilterMatchEquality as rows (add retries=3 to its look map), and merge TestFilterBuiltInKeysAndMetadataKeys into TestFilterResultLookup as one table over the CaseResult and TestCase lookups. Remove the five old functions. Run each partner green before editing and again after. Test files only, no production code."
    dependencies: []
    acceptance_criteria:
      - "TestFilterParseTrimsWhitespace, TestFilterParseOperatorChoosesLongest, TestFilterMatchMissingKeyNeverMatches, TestFilterEqualityNeverParsesNumbers and TestFilterBuiltInKeysAndMetadataKeys no longer exist as functions"
      - "TestFilterParseOperators lists subtests containing trims_whitespace (two rows) and longest_operator in -v output and passes"
      - "TestFilterMatchEquality lists subtests containing missing_key (nine rows) and textual_equality (four rows) and passes"
      - "TestFilterResultLookup lists subtests whose names start with TestCase_ for the TestCase lookups and keeps the CaseResult and legacy-result assertions"
      - "Every assertion the three partners had before is still present"
      - "No unused imports or test-only helpers left behind; gofmt and go vet clean for internal/eval"
    validation_commands:
      - "! grep -nE 'func (TestFilterParseTrimsWhitespace|TestFilterParseOperatorChoosesLongest|TestFilterMatchMissingKeyNeverMatches|TestFilterEqualityNeverParsesNumbers|TestFilterBuiltInKeysAndMetadataKeys)[(]' internal/eval/*_test.go"
      - "go test ./internal/eval -count=1 -v -run '^(TestFilterParseOperators|TestFilterMatchEquality|TestFilterResultLookup)$' > /tmp/issue-425-filter.log 2>&1"
      - "grep -E -- '--- PASS: TestFilterParseOperators/.*trims_whitespace' /tmp/issue-425-filter.log"
      - "grep -E -- '--- PASS: TestFilterParseOperators/.*longest_operator' /tmp/issue-425-filter.log"
      - "grep -E -- '--- PASS: TestFilterMatchEquality/.*missing_key' /tmp/issue-425-filter.log"
      - "grep -E -- '--- PASS: TestFilterMatchEquality/.*textual_equality' /tmp/issue-425-filter.log"
      - "grep -E -- '--- PASS: TestFilterResultLookup/TestCase_' /tmp/issue-425-filter.log"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "prune-judge-check-tests"
    agent: "builder"
    description: "Merge TestJudgeCheckDebugOffWritesNothing into TestJudgeCheckDebugPrintsPrompt (debug on and debug off rows), TestJudgeCheckPromptOmitsCaseExpectations into TestJudgeCheckRequestAndPrompt (set ExpectedOutput, assert absent from the prompt), TestJudgeCheckConfigureCarriesDebug into TestConfigureKeepWorkspaces in execute_case_test.go (debug flag set then reset, keep t.Cleanup(resetConfig)), and TestJudgeCheckEmptyOutputMakesNoJudgeCall into TestChecksScoringNoOutputOrWorkspaceScoresZero in checks_scoring_test.go (judge checks plus a recording backend, assert no requests and no calls). Remove the four old functions from judge_check_test.go. Run each partner green before editing and again after. Test files only."
    dependencies: ["prune-filter-tests"]
    acceptance_criteria:
      - "TestJudgeCheckDebugOffWritesNothing, TestJudgeCheckConfigureCarriesDebug, TestJudgeCheckEmptyOutputMakesNoJudgeCall and TestJudgeCheckPromptOmitsCaseExpectations no longer exist as functions"
      - "TestJudgeCheckDebugPrintsPrompt lists a subtest containing debug_off and keeps its debug-on assertions"
      - "TestConfigureKeepWorkspaces lists a subtest containing debug_flag, restores config with t.Cleanup(resetConfig), and keeps the keepWorkspaces assertions"
      - "TestChecksScoringNoOutputOrWorkspaceScoresZero lists a subtest containing no_judge_call that asserts zero backend requests, zero cr.Calls, score 0 of 2 and a 'checks not run' reasoning, and its two existing rows keep their assertions"
      - "TestJudgeCheckRequestAndPrompt sets ExpectedOutput, lists a subtest containing omits_expected_output, and keeps all prior assertions"
      - "No config leaks between tests (cfg.debug, cfg.backend restored); gofmt and go vet clean for internal/eval"
    validation_commands:
      - "! grep -nE 'func (TestJudgeCheckDebugOffWritesNothing|TestJudgeCheckConfigureCarriesDebug|TestJudgeCheckEmptyOutputMakesNoJudgeCall|TestJudgeCheckPromptOmitsCaseExpectations)[(]' internal/eval/*_test.go"
      - "go test ./internal/eval -count=1 -v -run '^(TestJudgeCheckDebugPrintsPrompt|TestConfigureKeepWorkspaces|TestChecksScoringNoOutputOrWorkspaceScoresZero|TestJudgeCheckRequestAndPrompt)$' > /tmp/issue-425-judge.log 2>&1"
      - "grep -E -- '--- PASS: TestJudgeCheckDebugPrintsPrompt/.*debug_off' /tmp/issue-425-judge.log"
      - "grep -E -- '--- PASS: TestConfigureKeepWorkspaces/.*debug_flag' /tmp/issue-425-judge.log"
      - "grep -E -- '--- PASS: TestChecksScoringNoOutputOrWorkspaceScoresZero/.*no_judge_call' /tmp/issue-425-judge.log"
      - "grep -E -- '--- PASS: TestJudgeCheckRequestAndPrompt/.*omits_expected_output' /tmp/issue-425-judge.log"
      - "go test ./internal/eval -count=1 -run 'TestJudgeCheck|TestConfigure|TestChecksScoring'"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "prune-metadata-tests"
    agent: "builder"
    description: "In internal/eval/metadata_test.go delete TestMetadataAbsentLoadsAsBefore, TestMetadataUnmarshalYAMLDirect, TestMetadataCaseResultJSON and TestSuiteSHA256ChangesOnMetadataOnlyEdit. Merge TestMetadataReservedKeyErrorExplains and TestMetadataValueErrorSaysWhatIsAllowed into TestMetadataInvalidFailsLoadCases by adding an optional explain column (reserved agent row gets 'reserved', list value row gets 'string, number or boolean') that the loop asserts when non-empty. Remove imports left unused. Run the partners (TestMetadataInvalidFailsLoadCases, TestMetadataDoesNotChangeExecutionOrScoring, TestMetadataLoadsScalarTextForms, TestResultsRecordMetadataForEveryRepeat, TestSuiteSHA256HashesRawCaseBytes) green before editing and again after. Test files only."
    dependencies: ["prune-judge-check-tests"]
    acceptance_criteria:
      - "TestMetadataAbsentLoadsAsBefore, TestMetadataUnmarshalYAMLDirect, TestMetadataCaseResultJSON and TestSuiteSHA256ChangesOnMetadataOnlyEdit no longer exist"
      - "TestMetadataReservedKeyErrorExplains and TestMetadataValueErrorSaysWhatIsAllowed no longer exist as functions"
      - "TestMetadataInvalidFailsLoadCases passes, still lists the reserved_agent and list_value subtests, and asserts the 'reserved' and 'string, number or boolean' explanations for those two rows"
      - "TestMetadataDoesNotChangeExecutionOrScoring, TestMetadataLoadsScalarTextForms, TestResultsRecordMetadataForEveryRepeat and TestSuiteSHA256HashesRawCaseBytes pass unchanged"
      - "gofmt and go vet clean for internal/eval"
    validation_commands:
      - "! grep -nE 'func (TestMetadataAbsentLoadsAsBefore|TestMetadataUnmarshalYAMLDirect|TestMetadataCaseResultJSON|TestSuiteSHA256ChangesOnMetadataOnlyEdit|TestMetadataReservedKeyErrorExplains|TestMetadataValueErrorSaysWhatIsAllowed)[(]' internal/eval/*_test.go"
      - "go test ./internal/eval -count=1 -v -run '^TestMetadataInvalidFailsLoadCases$' > /tmp/issue-425-metadata.log 2>&1"
      - "grep -E -- '--- PASS: TestMetadataInvalidFailsLoadCases/reserved_agent' /tmp/issue-425-metadata.log"
      - "grep -E -- '--- PASS: TestMetadataInvalidFailsLoadCases/list_value' /tmp/issue-425-metadata.log"
      - "go test ./internal/eval -count=1 -run '^(TestMetadataDoesNotChangeExecutionOrScoring|TestMetadataLoadsScalarTextForms|TestResultsRecordMetadataForEveryRepeat|TestSuiteSHA256HashesRawCaseBytes)$'"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "prune-provenance-tests"
    agent: "builder"
    description: "In internal/eval/provenance_test.go merge TestProvenanceHashResourceOrderMatters and TestProvenanceHashFramingIsUnambiguous into TestProvenanceHashChangesWithPromptResourceAndConfig as subtests (the existing body moves into its own subtest), and delete TestProvenanceTypesSerialisation. In internal/eval/provenance_workflow_test.go merge TestProvenanceDispatcherEditChangesBothHashes and TestWorkflowHashSwappingStepPromptsChangesHash into TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash as subtests (dispatcher edit, swap step prompts, each with its own wfRoot; the existing body becomes a sub-agent edit subtest), and delete TestAgentConfigFileTrustedAgents. Remove imports left unused. Run the partners and TestCallRecordsSelfTestProvenance green before editing and again after. Test files only."
    dependencies: ["prune-metadata-tests"]
    acceptance_criteria:
      - "TestProvenanceTypesSerialisation and TestAgentConfigFileTrustedAgents no longer exist"
      - "TestProvenanceHashResourceOrderMatters, TestProvenanceHashFramingIsUnambiguous, TestProvenanceDispatcherEditChangesBothHashes and TestWorkflowHashSwappingStepPromptsChangesHash no longer exist as functions"
      - "TestProvenanceHashChangesWithPromptResourceAndConfig lists subtests containing resource_order and byte_shift, and still asserts prompt edit, revert, resource edit and config edit"
      - "TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash lists subtests containing dispatcher_edit and swap_step_prompts, and still asserts the sub-agent prompt edit, step-b unchanged and sub-agent resource edit"
      - "TestCallRecordsSelfTestProvenance passes unchanged"
      - "gofmt and go vet clean for internal/eval"
    validation_commands:
      - "! grep -nE 'func (TestProvenanceTypesSerialisation|TestAgentConfigFileTrustedAgents|TestProvenanceHashResourceOrderMatters|TestProvenanceHashFramingIsUnambiguous|TestProvenanceDispatcherEditChangesBothHashes|TestWorkflowHashSwappingStepPromptsChangesHash)[(]' internal/eval/*_test.go"
      - "go test ./internal/eval -count=1 -v -run '^(TestProvenanceHashChangesWithPromptResourceAndConfig|TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash)$' > /tmp/issue-425-prov.log 2>&1"
      - "grep -E -- '--- PASS: TestProvenanceHashChangesWithPromptResourceAndConfig/.*resource_order' /tmp/issue-425-prov.log"
      - "grep -E -- '--- PASS: TestProvenanceHashChangesWithPromptResourceAndConfig/.*byte_shift' /tmp/issue-425-prov.log"
      - "grep -E -- '--- PASS: TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash/.*dispatcher_edit' /tmp/issue-425-prov.log"
      - "grep -E -- '--- PASS: TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash/.*swap_step_prompts' /tmp/issue-425-prov.log"
      - "go test ./internal/eval -count=1 -run '^(TestCallRecordsSelfTestProvenance|TestProvenance|TestWorkflowHash)'"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "prune-remaining-tests"
    agent: "builder"
    description: "Delete TestMockCLIDefaultDataDirIsWorkspaceMocks (mockcli_test.go), TestStartupOverheadMeasurement (perf_test.go), TestPlannerSuiteCaseFilesDeclarePlannerAgent, TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments and TestPlannerSuiteRegexesCompile (planner_suite_test.go) and TestNilRunPinsPrices (pricing_cost_test.go). Merge TestMultiTurnClassicCaseSendsTurnZero into TestMultiTurnClassicCaseHasNoTurnFields (multi_turn_test.go) using useRecordingBackend with no turn fields and turn zero request subtests, and merge TestRepeatExecutionRejectsRepeatWithCleanupAndPerf into TestRepeatExecutionRunOptionsValidation (repeat_exec_test.go) as repeat with cleanup and repeat with perf rows. Remove imports and test-only helpers that become unused only because of these removals. Run the partners (TestMockCLIReplyLookupOrder, TestPlannerSuiteRedLegacyTranscriptsFail, TestPlannerSuiteGreenReferenceTranscriptsPass, TestCostFromUsageUnpinnedPricesNothing and the two merge partners) green before editing and again after. Test files only."
    dependencies: ["prune-provenance-tests"]
    acceptance_criteria:
      - "TestMockCLIDefaultDataDirIsWorkspaceMocks, TestStartupOverheadMeasurement, TestPlannerSuiteCaseFilesDeclarePlannerAgent, TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments, TestPlannerSuiteRegexesCompile and TestNilRunPinsPrices no longer exist"
      - "TestMultiTurnClassicCaseSendsTurnZero and TestRepeatExecutionRejectsRepeatWithCleanupAndPerf no longer exist as functions"
      - "TestMultiTurnClassicCaseHasNoTurnFields lists subtests containing turn_zero_request and still asserts the output, TurnOutputs nil, no turn JSON fields, no turn progress and Turn 0 on calls"
      - "TestRepeatExecutionRunOptionsValidation lists subtests containing repeat_with_cleanup and repeat_with_perf and keeps its two original rows"
      - "TestMockCLIReplyLookupOrder, TestPlannerSuiteRedLegacyTranscriptsFail, TestPlannerSuiteGreenReferenceTranscriptsPass and TestCostFromUsageUnpinnedPricesNothing pass unchanged"
      - "gofmt and go vet clean for internal/eval"
    validation_commands:
      - "! grep -nE 'func (TestMockCLIDefaultDataDirIsWorkspaceMocks|TestStartupOverheadMeasurement|TestPlannerSuiteCaseFilesDeclarePlannerAgent|TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments|TestPlannerSuiteRegexesCompile|TestNilRunPinsPrices|TestMultiTurnClassicCaseSendsTurnZero|TestRepeatExecutionRejectsRepeatWithCleanupAndPerf)[(]' internal/eval/*_test.go"
      - "go test ./internal/eval -count=1 -v -run '^(TestMultiTurnClassicCaseHasNoTurnFields|TestRepeatExecutionRunOptionsValidation)$' > /tmp/issue-425-rest.log 2>&1"
      - "grep -E -- '--- PASS: TestMultiTurnClassicCaseHasNoTurnFields/.*turn_zero_request' /tmp/issue-425-rest.log"
      - "grep -E -- '--- PASS: TestRepeatExecutionRunOptionsValidation/.*repeat_with_cleanup' /tmp/issue-425-rest.log"
      - "grep -E -- '--- PASS: TestRepeatExecutionRunOptionsValidation/.*repeat_with_perf' /tmp/issue-425-rest.log"
      - "go test ./internal/eval -count=1 -run '^(TestMockCLIReplyLookupOrder|TestPlannerSuiteRedLegacyTranscriptsFail|TestPlannerSuiteGreenReferenceTranscriptsPass|TestCostFromUsageUnpinnedPricesNothing)$'"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "verify-prune"
    agent: "validator"
    description: "Read-only verification of the whole change. Run the issue's two grep checks verbatim, run every one of the 12 merge partners and the 9 delete partners with -v and confirm each merged case appears, inspect git diff to confirm it touches only the 13 named internal/eval test files (ignore the pre-existing .kiro/agents modifications and files under .kairon/), run task fmt:check and task lint, run go test -cover ./internal/eval and confirm at least 87.6%, then run task test and confirm exit 0. Allow roughly 8 minutes for the coverage run and longer for task test."
    dependencies: ["prune-remaining-tests"]
    acceptance_criteria:
      - "The AC1 grep prints nothing: none of the 12 Delete tests exists"
      - "The AC2 grep prints nothing: none of the 17 Merge tests exists as a function"
      - "go test ./internal/eval -run '^<partner>$' -v exits 0 for each of the 12 merge partners and lists the merged subtest token from the per-merge table in the spec"
      - "Inspecting git diff (ignoring the pre-existing .kiro/agents modifications and files under .kairon/) shows only the 13 named internal/eval _test.go files changed, with no production .go file, doc or template edited"
      - "go test -cover ./internal/eval reports coverage of at least 87.6% of statements"
      - "task test exits 0"
      - "task fmt:check and task lint exit 0"
    validation_commands:
      - "! grep -nE 'func (TestAgentConfigFileTrustedAgents|TestMetadataAbsentLoadsAsBefore|TestMetadataCaseResultJSON|TestMetadataUnmarshalYAMLDirect|TestMockCLIDefaultDataDirIsWorkspaceMocks|TestNilRunPinsPrices|TestPlannerSuiteCaseFilesDeclarePlannerAgent|TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments|TestPlannerSuiteRegexesCompile|TestProvenanceTypesSerialisation|TestStartupOverheadMeasurement|TestSuiteSHA256ChangesOnMetadataOnlyEdit)[(]' internal/eval/*_test.go"
      - "! grep -nE 'func (TestFilterBuiltInKeysAndMetadataKeys|TestFilterEqualityNeverParsesNumbers|TestFilterMatchMissingKeyNeverMatches|TestFilterParseOperatorChoosesLongest|TestFilterParseTrimsWhitespace|TestJudgeCheckConfigureCarriesDebug|TestJudgeCheckDebugOffWritesNothing|TestJudgeCheckEmptyOutputMakesNoJudgeCall|TestJudgeCheckPromptOmitsCaseExpectations|TestMetadataReservedKeyErrorExplains|TestMetadataValueErrorSaysWhatIsAllowed|TestMultiTurnClassicCaseSendsTurnZero|TestProvenanceDispatcherEditChangesBothHashes|TestProvenanceHashFramingIsUnambiguous|TestProvenanceHashResourceOrderMatters|TestRepeatExecutionRejectsRepeatWithCleanupAndPerf|TestWorkflowHashSwappingStepPromptsChangesHash)[(]' internal/eval/*_test.go"
      - "go test ./internal/eval -count=1 -v -run '^(TestFilterParseOperators|TestFilterMatchEquality|TestFilterResultLookup|TestJudgeCheckDebugPrintsPrompt|TestConfigureKeepWorkspaces|TestChecksScoringNoOutputOrWorkspaceScoresZero|TestJudgeCheckRequestAndPrompt|TestMetadataInvalidFailsLoadCases|TestMultiTurnClassicCaseHasNoTurnFields|TestProvenanceHashChangesWithPromptResourceAndConfig|TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash|TestRepeatExecutionRunOptionsValidation)$' > /tmp/issue-425-partners.log 2>&1"
      - "grep -E -- '--- PASS: TestFilterParseOperators/.*trims_whitespace' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestFilterParseOperators/.*longest_operator' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestFilterMatchEquality/.*missing_key' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestFilterMatchEquality/.*textual_equality' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestFilterResultLookup/TestCase_' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestJudgeCheckDebugPrintsPrompt/.*debug_off' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestConfigureKeepWorkspaces/.*debug_flag' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestChecksScoringNoOutputOrWorkspaceScoresZero/.*no_judge_call' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestJudgeCheckRequestAndPrompt/.*omits_expected_output' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestMetadataInvalidFailsLoadCases/reserved_agent' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestMetadataInvalidFailsLoadCases/list_value' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestMultiTurnClassicCaseHasNoTurnFields/.*turn_zero_request' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestProvenanceHashChangesWithPromptResourceAndConfig/.*resource_order' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestProvenanceHashChangesWithPromptResourceAndConfig/.*byte_shift' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash/.*dispatcher_edit' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash/.*swap_step_prompts' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestRepeatExecutionRunOptionsValidation/.*repeat_with_cleanup' /tmp/issue-425-partners.log"
      - "grep -E -- '--- PASS: TestRepeatExecutionRunOptionsValidation/.*repeat_with_perf' /tmp/issue-425-partners.log"
      - "go test ./internal/eval -count=1 -run '^(TestMetadataDoesNotChangeExecutionOrScoring|TestMetadataLoadsScalarTextForms|TestResultsRecordMetadataForEveryRepeat|TestSuiteSHA256HashesRawCaseBytes|TestMockCLIReplyLookupOrder|TestPlannerSuiteRedLegacyTranscriptsFail|TestPlannerSuiteGreenReferenceTranscriptsPass|TestCostFromUsageUnpinnedPricesNothing|TestCallRecordsSelfTestProvenance)$'"
      - "task fmt:check"
      - "task lint"
      - "go test -count=1 -cover ./internal/eval | tee /tmp/issue-425-cover.log"
      - "grep -E 'coverage: (87\\.[6-9]|8[89]\\.[0-9]|9[0-9]\\.[0-9]|100\\.0)% of statements' /tmp/issue-425-cover.log"
      - "task test"
```
