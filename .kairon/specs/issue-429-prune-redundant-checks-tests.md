# Design Spec: Prune redundant internal/checks tests

Closes #429

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/checks`: `checks_document_test.go`, `checks_fact_test.go`, `checks_glob_test.go`, `checks_judge_test.go`, `checks_test.go`, `checks_turn_test.go`, `judge_infer_test.go`, `judge_prompt_test.go`, `load_test.go`" | `prune-document-tests`, `prune-fact-tests`, `prune-glob-tests`, `prune-judge-verdict-tests`, `prune-judge-fixture-tests`, `prune-checks-tests`, `prune-turn-validate-tests`, `prune-turn-eval-tests`, `prune-infer-tests`, `prune-prompt-tests`, `prune-load-tests` |
| C1 | "No production code changes. Every test not listed passes unchanged. Merged partners keep every assertion they had." | `prune-document-tests`, `prune-fact-tests`, `prune-glob-tests`, `prune-judge-verdict-tests`, `prune-judge-fixture-tests`, `prune-checks-tests`, `prune-turn-validate-tests`, `prune-turn-eval-tests`, `prune-infer-tests`, `prune-prompt-tests`, `prune-load-tests`, `gate-coverage-and-task-test`, `validate-prune` |
| AC1 | "The 9 tests marked Delete no longer exist." | `prune-document-tests`, `prune-glob-tests`, `prune-judge-verdict-tests`, `prune-checks-tests`, `prune-load-tests`, `validate-prune` |
| AC2 | "Each of the 16 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-document-tests`, `prune-fact-tests`, `prune-judge-verdict-tests`, `prune-judge-fixture-tests`, `prune-checks-tests`, `prune-turn-validate-tests`, `prune-turn-eval-tests`, `prune-infer-tests`, `prune-prompt-tests`, `validate-prune` |
| AC3 | "Package coverage does not drop." | `gate-coverage-and-task-test`, `validate-prune` |
| AC4 | "Every remaining test passes unchanged, and the gate passes." | `gate-coverage-and-task-test`, `validate-prune` |

## Deferred work

none

## Test-to-Task Map

Every row of the issue's Current / Target State table, so the 9 Deletes and 16 Merges can be checked one by one.

| Test (file) | Action | Partner | Delivered by |
|-------------|--------|---------|--------------|
| `TestDocumentCheckSpecAllowsOnlyTurn` (checks_document_test.go) | Delete | `TestDocumentCheckValidate` | `prune-document-tests` |
| `TestDocumentCheckResultShape` (checks_document_test.go) | Merge | `TestJudgeCheckLabel` | `prune-document-tests` |
| `TestFactChecks_Label` (checks_fact_test.go) | Merge | `TestJudgeCheckLabel` | `prune-fact-tests` |
| `TestGlobCompiledReuse` (checks_glob_test.go) | Delete | `TestGlobMatch` | `prune-glob-tests` |
| `TestJudgeCheckVocabulary` (checks_judge_test.go) | Delete | `TestJudgeCheckTurnFieldValidation` | `prune-judge-verdict-tests` |
| `TestJudgeCheckNoFails` (checks_judge_test.go) | Merge | `TestJudgeCheckYesPasses` | `prune-judge-verdict-tests` |
| `TestJudgeCheckManyLargeFilesNoPanic` (checks_judge_test.go) | Merge | `TestJudgeCheckFileCaps` | `prune-judge-fixture-tests` |
| `TestJudgeCheckNilJudgeFails` (checks_judge_test.go) | Delete | `TestJudgeCheckNilJudgeFailsEvenWithFiles` | `prune-judge-verdict-tests` |
| `TestJudgeCheckFilesReadBeforeCommandChecks` (checks_judge_test.go) | Merge | `TestJudgeCheckRunsInNonCommandPassInListOrder` | `prune-judge-fixture-tests` |
| `TestJudgeCheckEvaluatorStaysIndependent` (checks_judge_test.go) | Delete | none | `prune-judge-verdict-tests` |
| `TestJudgeCheckOneElementTurnsHasNoTranscript` (checks_judge_test.go) | Merge | `TestJudgeCheckClassicInputHasNoTranscript` | `prune-judge-fixture-tests` |
| `TestChecksCheckTypeSet` (checks_test.go) | Delete | `TestChecksDocsCoversEveryCheckType` | `prune-checks-tests` |
| `TestChecksValidateNilCriterionSkipsCrossCheck` (checks_test.go) | Merge | `TestChecksValidateOptionalCriterion` | `prune-checks-tests` |
| `TestChecksValidateEmptyList` (checks_test.go) | Merge | `TestChecksValidateTable` | `prune-checks-tests` |
| `TestCheckTurnYAMLDecodes` (checks_turn_test.go) | Merge | `TestChecksYAMLDecodesChecksList` | `prune-turn-validate-tests` |
| `TestCheckTurnValidateAllowedTypes` (checks_turn_test.go) | Merge | `TestCheckTurnValidateRejectedTypes` | `prune-turn-validate-tests` |
| `TestCheckTurnValidateRejectsBelowOne` (checks_turn_test.go) | Merge | `TestDocumentCheckValidate` | `prune-turn-validate-tests` |
| `TestCheckTurnEvalPerTurnOutput` (checks_turn_test.go) | Merge | `TestCheckTurnEvalPerTurnGHLog` | `prune-turn-eval-tests` |
| `TestCheckTurnLabel` (checks_turn_test.go) | Merge | `TestJudgeCheckLabel` | `prune-turn-eval-tests` |
| `TestInferenceJudgeUnpinnedNoStub` (judge_infer_test.go) | Merge | `TestInferenceJudgeRequest` | `prune-infer-tests` |
| `TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks` (judge_prompt_test.go) | Merge | `TestBuildYesNoPromptTranscriptOrderAndFences` | `prune-prompt-tests` |
| `TestBuildYesNoPromptTranscriptMarkerCount` (judge_prompt_test.go) | Merge | `TestBuildYesNoPromptTranscriptOrderAndFences` | `prune-prompt-tests` |
| `TestLoadFile_EvidenceNeedsHiddenDirForInject` (load_test.go) | Delete | `TestCheckEvidenceFlags` (cmd) | `prune-load-tests` |
| `TestFixtures_NoAbsolutePaths` (load_test.go) | Delete | none | `prune-load-tests` |
| `TestFixtures_Content` (load_test.go) | Delete | none | `prune-load-tests` |

## Solution Approach

This is a test-only refactor of `internal/checks`. No production file changes. The 25 tests in the issue's table are either deleted (9) or folded into a named partner as a table row, subtest or extra assertion (16).

**Characterization first.** AGENTS.md requires characterization tests before a refactor. The existing suite is the characterization. Measured on this worktree before design:

- `go test -cover ./internal/checks` passes at exactly **83.8%**, so there is no coverage margin.
- Running the package with all 9 Delete tests skipped (`go test -cover -skip '^(...)$' ./internal/checks`) still reports **83.8%**. Deleting them cannot by itself break AC3.
- Every partner named in the issue exists. `TestCheckEvidenceFlags` is in `cmd/kairon/cmd/check_test.go:316`, and its subtest "without hidden-dir the inject check is invalid" asserts the same rule as the deleted `TestLoadFile_EvidenceNeedsHiddenDirForInject`.
- No live doc or code references any of the 25 names, except the doc comment above `TestJudgeCheckManyLargeFilesNoPanic`, which travels with that test's merge (see `prune-judge-fixture-tests`).

**Per-task routine (every `prune-*` task).**

1. Run the tests being removed and their partner(s) and see them pass. This is the "before" baseline.
2. Fold the removed test's distinct input or assertion into the partner. Drop anything the partner already asserts (the issue says so explicitly for Merges). Keep every assertion the partner had. Do not weaken, loosen or delete any partner assertion. Converting a flat partner into a table is fine only if each original assertion survives, row by row.
3. Delete the original function, its now-unused imports and helpers. Keep doc comments that explain a regression (carry them onto the new subtest).
4. Run the partner with `-v` and confirm the prescribed subtest name appears as `--- PASS`. Run `go vet` and `gofmt -l`.
5. Confirm the package still passes and coverage is still >= 83.8%.
6. Commit the test change as one commit per task, per AGENTS.md ("commit the test and the implementation together"; there is no separate implementation here). The commit message names the tests removed and the partners that now carry them (AGENTS.md "Say what you observed": for a refactor, the tests relied on). Use the Conventional Commits `test(checks): ...` form.

**Subtest naming.** The validation commands grep for prescribed subtest names so that AC2 ("passes with the merged case among its subtests") is mechanically checkable. `t.Run` turns spaces into underscores, so the names below use hyphens. Where a merge is a plain extra assertion in a non-table partner, the task states the exact assertion text to grep for.

**No spec deviations are planned.** If a builder finds that a merge instruction below does not work against the real code, it must record the deviation per AGENTS.md "No silent deviation" (code comment citing this spec section, plus the `Spec deviations` entry in the sentinel).

## Relevant Files

All under `internal/checks/`, all test files. Nothing else is modified.

| File | Change |
|------|--------|
| `checks_document_test.go` | delete `TestDocumentCheckSpecAllowsOnlyTurn`, `TestDocumentCheckResultShape`; `TestDocumentCheckValidate` receives the below-one rows |
| `checks_fact_test.go` | delete `TestFactChecks_Label` |
| `checks_glob_test.go` | delete `TestGlobCompiledReuse` |
| `checks_judge_test.go` | delete 4 + merge 3 judge tests; `TestJudgeCheckLabel` becomes the single label table; verdict, file-cap, ordering and transcript partners extended |
| `checks_test.go` | delete `TestChecksCheckTypeSet`; extend `TestChecksValidateOptionalCriterion`, `TestChecksValidateTable`, `TestChecksYAMLDecodesChecksList` (the last one receives the turn sample) |
| `checks_turn_test.go` | delete 5 tests; `TestCheckTurnValidateRejectedTypes` and `TestCheckTurnEvalPerTurnGHLog` extended |
| `judge_infer_test.go` | delete `TestInferenceJudgeUnpinnedNoStub`; `TestInferenceJudgeRequest` becomes a two-row table |
| `judge_prompt_test.go` | delete 2 tests; `TestBuildYesNoPromptTranscriptOrderAndFences` extended |
| `load_test.go` | delete 3 tests |

Read-only references: `cmd/kairon/cmd/check_test.go` (`TestCheckEvidenceFlags`), `checks_eval_test.go` (`TestEvaluateChecks_ResultsKeepListOrderAndLabels`, which already asserts Index/Type generically), `checks_docs_test.go` (`TestChecksDocsCoversEveryCheckType`), `Taskfile.yml` (`task test` = `go test -v -race -coverprofile=coverage.out ./...`).

## Team Orchestration

- All implementation tasks are `builder` tasks that run **strictly in sequence**. They edit overlapping files (`checks_judge_test.go` receives merges from four source files, `checks_document_test.go` is both a source and a partner for `checks_turn_test.go`), and all builders share one worktree, so a half-edited test file would break the package build for a parallel builder.
- `prune-document-tests` runs first because it converts `TestJudgeCheckLabel` into the single label table. `prune-fact-tests` and `prune-turn-eval-tests` only add rows to that table.
- `prune-turn-validate-tests` follows `prune-document-tests`, since it adds rows to `TestDocumentCheckValidate`.
- `gate-coverage-and-task-test` (builder, because it may need to restore a lost branch-covering input as a table row) runs after all pruning tasks, then `validate-prune` (validator, read-only) re-verifies everything independently.
- No documenter task: no user-facing behavior or doc changes, and no live doc mentions the removed tests.

## Step-by-Step Task Breakdown

### 1. `prune-document-tests` (checks_document_test.go)

- **Delete `TestDocumentCheckSpecAllowsOnlyTurn`.** It asserts the internal `checkSpecs` entry. `TestDocumentCheckValidate` already proves the same rule (only `turn` is allowed, nothing required) through `Validate`. No change to the partner.
- **Merge `TestDocumentCheckResultShape` into `TestJudgeCheckLabel`** (in `checks_judge_test.go`). Convert `TestJudgeCheckLabel` from flat assertions into a table `{name, index, check, want}` and keep the two existing assertions as rows: the judge-question label (`#2 judge question="Does the body state a criterion?"`) and the long-question shortening check (`len(got) <= 120`, suffix `…"`; this row needs a custom assertion, so give the row an optional `check func(t *testing.T, got string)` or keep it as its own subtest). Add rows:
  - `document-result-label`: `checkLabel(2, docCheck())` equals `#2 output_document_valid`.
  - `document-label-with-turn`: `docCheck()` with `Turn = turnPtr(1)`, index 1, equals `#1 output_document_valid turn=1`.
- Drop the `Index == 2`, `Type == CheckOutputDocumentValid` and `len(rs) == 2` assertions. They are covered generically by `TestEvaluateChecks_ResultsKeepListOrderAndLabels` (confirm by reading it before deleting).

### 2. `prune-fact-tests` (checks_fact_test.go)

- **Merge `TestFactChecks_Label` into `TestJudgeCheckLabel`.** Add rows `fact-not-asked` (`checkLabel(2, factCheck(CheckFactNotAsked, "f1"))` = `#2 fact_not_asked fact=f1`), `fact-asked` (index 1, `#1 fact_asked fact=f1`) and `file-exists-unchanged` (`checkLabel(3, Check{Type: CheckFileExists, Path: "a.txt"})` = `#3 file_exists path=a.txt`, keeping the "existing labels are unchanged" comment). Delete the original and any import it alone used.

### 3. `prune-glob-tests` (checks_glob_test.go)

- **Delete `TestGlobCompiledReuse`.** `TestGlobMatch` covers the same `**/*.go` cases through `compileGlob`/`match`. Before deleting, confirm `TestGlobMatch` contains a `**/*.go`-style case that matches `main.go` and `a/b/main.go` and rejects `a/b/main.txt`. If a case is missing, add it as a row (the issue says the partner already covers it, so this is a safety check, not planned work).

### 4. `prune-judge-verdict-tests` (checks_judge_test.go)

- **Delete `TestJudgeCheckVocabulary`** (constant plus internal `checkSpecs` flags; `TestChecksValidateJudge` and `TestJudgeCheckTurnFieldValidation` exercise required/allowed behavior).
- **Merge `TestJudgeCheckNoFails` into `TestJudgeCheckYesPasses`** as one verdict table with rows `verdict-yes` and `verdict-no`. Row `verdict-yes` keeps all three YesPasses assertions (pass, detail contains `judge answered yes: clearly stated`, `r.Type == CheckJudge` and label contains `judge`). Row `verdict-no` uses `JudgeVerdict{Yes: false, Reasoning: "criterion missing"}` and `wantFail(t, r, "judge answered no: criterion missing")`.
- **Delete `TestJudgeCheckNilJudgeFails`** (identical to `TestJudgeCheckNilJudgeFailsEvenWithFiles` minus the file).
- **Delete `TestJudgeCheckEvaluatorStaysIndependent`** (type dispatch makes it trivially true).

### 5. `prune-judge-fixture-tests` (checks_judge_test.go)

- **Merge `TestJudgeCheckManyLargeFilesNoPanic` into `TestJudgeCheckFileCaps`** as a new subtest `many-large-files-total-budget`. It writes five 70000-byte files (`big0.md`..`big4.md`, each over `judgeMaxFileBytes`) into the shared `dir`, and keeps all assertions: the check passes (a panic would be recovered as a failure), `len(files) == len(names)`, every file `Truncated`, and total shown bytes <= `judgeMaxTotalBytes`. Move the doc comment about the `shownBytes` regression onto the subtest. Use names that do not collide with the existing `big.md`, `small.md`, `f0..f4.md` fixtures. The existing `per file` and `total` subtests must be unchanged.
- **Merge `TestJudgeCheckFilesReadBeforeCommandChecks` into `TestJudgeCheckRunsInNonCommandPassInListOrder`.** In the partner, create `a.md` containing `original` in `dir`, make the first judge check read it (`judgeCheck("first", "a.md")`), and make the existing command check also rewrite it (`echo ran >> order.txt && echo changed > a.md`). Inside the judge closure record `q.Files[0].Content` for the `first` question and assert it equals `original`. Do not add or remove checks from the partner's list, so its index and order assertions are unchanged.
- **Merge `TestJudgeCheckOneElementTurnsHasNoTranscript` into `TestJudgeCheckClassicInputHasNoTranscript`** as one table with rows `classic-input` and `one-element-turns`. Each row supplies the `Input` and the expected query `Input` and `Output`. Each row asserts: pass, `len(q.Transcript) == 0`, `q.Input`/`q.Output` unchanged, `turn: 2` fails with `the case has 1 turn(s)`, and the judge is not invoked for the out-of-range turn. The classic row keeps its extra `q.Turn == 0` assertion and its `turn: 1` accepted step (`turn: 1` on the one-element row should also pass; if it does not, keep that step classic-only and say why in a code comment).

### 6. `prune-checks-tests` (checks_test.go)

- **Delete `TestChecksCheckTypeSet`** (compares string constants to themselves and is stale; `TestChecksDocsCoversEveryCheckType` enumerates the real set).
- **Merge `TestChecksValidateNilCriterionSkipsCrossCheck` into `TestChecksValidateOptionalCriterion`** as a subtest `nil-callback-skips-cross-check`: `Validate("f.yaml", []Check{{Criterion: "anything", Type: CheckFileExists, Path: "a"}}, ValidateOptions{})` returns nil. The partner's existing body becomes a sibling subtest (or stays outside; either way every existing assertion remains).
- **Merge `TestChecksValidateEmptyList` into `TestChecksValidateTable`** as a row named `empty-list` asserting `Validate("f.yaml", nil, ValidateOptions{})` is nil. The table wraps each row's `Check` in a two-check list, so add an optional field to the row struct (for example `list []Check`, used instead of the wrapped pair when non-nil, or a `noChecks bool`). Every existing row must keep its current behavior and assertions.

### 7. `prune-turn-validate-tests` (checks_turn_test.go, checks_test.go, checks_document_test.go)

- **Merge `TestCheckTurnYAMLDecodes` into `TestChecksYAMLDecodesChecksList`.** Append a fourth check to the partner's `src` sample, `- criterion: clarity, type: gh_log_contains, pattern: x, turn: 3` (any valid form), update the length assertion to 4, assert `list[3].Turn != nil && *list[3].Turn == 3`, and assert `Turn == nil` on one of the existing entries (the "missing turn decodes as nil" half). Keep every existing assertion on entries 0 to 2.
- **Merge `TestCheckTurnValidateAllowedTypes` into `TestCheckTurnValidateRejectedTypes`.** Add one subtest per allowed type, named `allowed_output_contains`, `allowed_output_not_contains`, `allowed_gh_log_contains`, `allowed_gh_log_not_contains`, each decoding `{criterion: c, type: <t>, pattern: x, turn: 2}` and requiring `Validate` to return nil. The existing rejected-type subtests keep their names and assertions.
- **Merge `TestCheckTurnValidateRejectsBelowOne` into `TestDocumentCheckValidate`.** Turn the partner's `turn below one` subtest into a table over check types and values. Keep the existing row (`output_document_valid`, `turn: 0`, error contains `>= 1`) and add `output_contains` with `turn: 0` and `turn: -1`. Each added row asserts the error is non-nil and contains both `turn` and `>= 1`. Expected subtest paths: `TestDocumentCheckValidate/turn_below_one/output_document_valid_0`, `.../output_contains_0`, `.../output_contains_-1`.

### 8. `prune-turn-eval-tests` (checks_turn_test.go, checks_judge_test.go)

- **Merge `TestCheckTurnEvalPerTurnOutput` into `TestCheckTurnEvalPerTurnGHLog`** as one table over evidence kind, with subtests `gh-log` and `output`. The `gh-log` row holds the partner's existing assertions (pass for `notCreate` turn 1 and `create3` turn 3, fail `not found in gh log` for `create1`, fail `matched in gh log` for `notCreate3`). The `output` row holds the four output assertions (output_contains `out1` on turn 1 passes and on turn 2 fails with `not found in output`; output_not_contains `out3` on turn 2 passes and `out2` on turn 2 fails with `matched in output`). Both use `threeTurns()`.
- **Merge `TestCheckTurnLabel` into `TestJudgeCheckLabel`** with rows `gh-log-with-turn` (`#2 gh_log_contains pattern=/issue create/ turn=3`), `gh-log-without-turn` (`#2 gh_log_contains pattern=/issue create/`) and `file-exists-no-turn` (the label of a `file_exists` check without a turn must not contain `turn`; use the table's not-contains field, or a custom assertion on that row).

### 9. `prune-infer-tests` (judge_infer_test.go)

- **Merge `TestInferenceJudgeUnpinnedNoStub` into `TestInferenceJudgeRequest`** as a two-row table: `pinned-with-stub` (the existing body, unchanged assertions: verdict, one backend call, role, YesNo, timeout, model, stub, prompt) and `unpinned-no-stub` (`InferenceJudge{Backend: b}`, query `{Question: "q"}`, no error, `req.Model == ""` and `req.Stub == nil`). Share the verdict and call-count checks where they apply to both rows.

### 10. `prune-prompt-tests` (judge_prompt_test.go)

- **Merge `TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks` and `TestBuildYesNoPromptTranscriptMarkerCount` into `TestBuildYesNoPromptTranscriptOrderAndFences`.** Both use `twoTurnQuery()`, so reuse the partner's `got`. Add subtests `omits-single-turn-blocks` (the prompt contains none of `AGENT FINAL OUTPUT`, `BEGIN INPUT GIVEN TO THE AGENT`, `SHOULD-NOT-APPEAR-INPUT`) and `marker-count` (exactly 4 `===BEGIN TURN ` and 4 `===END TURN ` markers; keep the comment about bodies being inserted verbatim). Existing partner assertions stay outside the subtests, unchanged.

### 11. `prune-load-tests` (load_test.go)

- **Delete `TestLoadFile_EvidenceNeedsHiddenDirForInject`.** Before deleting, run `go test -count=1 -run '^TestCheckEvidenceFlags$' -v ./cmd/kairon/cmd` and confirm the subtest `without_hidden-dir_the_inject_check_is_invalid` passes. That cmd-level test loads the same `evidence.yaml` fixture and checks the rule more strictly.
- **Delete `TestFixtures_NoAbsolutePaths`** and **`TestFixtures_Content`** (fixture hygiene lints with no product behavior; consumers fail if fixtures change).
- Keep `fixtureDir` (still used by `TestLoadFile_Fixtures`, `TestLoadFile_InvalidFixtureLoadsButFailsValidate`). Remove only imports that become unused, if any.

### 12. `gate-coverage-and-task-test`

- Run `go test -count=1 -cover ./internal/checks` and confirm the reported coverage is at least **83.8%** (AC3).
- Run `task test` and confirm it exits 0 (AC4). This runs the whole module with `-race`, so it also confirms that nothing outside `internal/checks` depended on a removed test or helper.
- Run `gofmt -l internal/checks` (empty) and `go vet ./internal/checks`.
- If coverage fell below 83.8%: identify which removed or merged test carried a statement nothing else covers (`go test -coverprofile` before/after on the commit history), then restore that input as a table row in the relevant partner. Do not touch production code, and do not delete assertions. The pre-design measurement says this should not happen: deleting all 9 tests leaves 83.8%, and merges keep every input.
- Make no other change if all checks pass.

### 13. `validate-prune` (validator, read-only)

Independently re-verify against the issue, without trusting builder sentinels:

- AC1: the Delete grep prints nothing.
- AC2: the Merge grep prints nothing. Each partner passes with `-v`, and each prescribed subtest name appears as PASS. The inline merges (below) are present in the diff.
- Contract C1: only `_test.go` files changed under `internal/` and `cmd/`. Compare each merged partner with `git show <base>:internal/checks/<file>` and confirm that no original assertion was removed or weakened. The only assertions that may disappear are those the issue names as already covered: Index/Type/`len(rs)` in `TestDocumentCheckResultShape`.
- AC3 and AC4: coverage >= 83.8% and `task test` exits 0.
- AGENTS.md rules: no deferral wording, no unrecorded spec deviation.

## Validation Commands

```bash
# AC1: the 9 deleted tests are gone (prints nothing)
grep -nE 'func (TestChecksCheckTypeSet|TestDocumentCheckSpecAllowsOnlyTurn|TestFixtures_Content|TestFixtures_NoAbsolutePaths|TestGlobCompiledReuse|TestJudgeCheckEvaluatorStaysIndependent|TestJudgeCheckNilJudgeFails|TestJudgeCheckVocabulary|TestLoadFile_EvidenceNeedsHiddenDirForInject)\(' internal/checks/*_test.go

# AC2: the 16 merged tests are gone (prints nothing)
grep -nE 'func (TestBuildYesNoPromptTranscriptMarkerCount|TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks|TestCheckTurnEvalPerTurnOutput|TestCheckTurnLabel|TestCheckTurnValidateAllowedTypes|TestCheckTurnValidateRejectsBelowOne|TestCheckTurnYAMLDecodes|TestChecksValidateEmptyList|TestChecksValidateNilCriterionSkipsCrossCheck|TestDocumentCheckResultShape|TestFactChecks_Label|TestInferenceJudgeUnpinnedNoStub|TestJudgeCheckFilesReadBeforeCommandChecks|TestJudgeCheckManyLargeFilesNoPanic|TestJudgeCheckNoFails|TestJudgeCheckOneElementTurnsHasNoTranscript)\(' internal/checks/*_test.go

# AC3, AC4
go test -count=1 -cover ./internal/checks
task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: prune-document-tests
    agent: builder
    description: >-
      checks_document_test.go: delete TestDocumentCheckSpecAllowsOnlyTurn (TestDocumentCheckValidate already covers the rule),
      and merge TestDocumentCheckResultShape into TestJudgeCheckLabel in checks_judge_test.go. Convert TestJudgeCheckLabel into a label table
      keeping its two existing assertions (judge question label, long-question shortening) and add rows document-result-label
      (#2 output_document_valid) and document-label-with-turn (#1 output_document_valid turn=1). Drop only the Index/Type/len assertions
      that TestEvaluateChecks_ResultsKeepListOrderAndLabels already covers. Commit as test(checks).
    dependencies: []
    acceptance_criteria:
      - "Before editing, TestDocumentCheckSpecAllowsOnlyTurn, TestDocumentCheckResultShape, TestDocumentCheckValidate and TestJudgeCheckLabel were run and passed"
      - "func TestDocumentCheckSpecAllowsOnlyTurn no longer exists in internal/checks/*_test.go"
      - "func TestDocumentCheckResultShape no longer exists in internal/checks/*_test.go"
      - "TestJudgeCheckLabel keeps both of its original assertions (exact judge label and long-question shortening: len <= 120 and suffix ending in the ellipsis and quote)"
      - "TestJudgeCheckLabel has subtests document-result-label (want '#2 output_document_valid') and document-label-with-turn (want '#1 output_document_valid turn=1') that pass"
      - "TestDocumentCheckValidate is unchanged"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestDocumentCheckSpecAllowsOnlyTurn|TestDocumentCheckResultShape)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^(TestDocumentCheckValidate|TestJudgeCheckLabel)$' ./internal/checks
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/document-result-label'
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/document-label-with-turn'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-fact-tests
    agent: builder
    description: >-
      checks_fact_test.go: merge TestFactChecks_Label into the TestJudgeCheckLabel table (checks_judge_test.go) as rows fact-not-asked
      (#2 fact_not_asked fact=f1), fact-asked (#1 fact_asked fact=f1) and file-exists-unchanged (#3 file_exists path=a.txt), then delete
      the original function. Commit as test(checks).
    dependencies: ["prune-document-tests"]
    acceptance_criteria:
      - "Before editing, TestFactChecks_Label and TestJudgeCheckLabel were run and passed"
      - "func TestFactChecks_Label no longer exists in internal/checks/*_test.go"
      - "TestJudgeCheckLabel has passing subtests fact-not-asked, fact-asked and file-exists-unchanged with the exact want strings of the removed test"
      - "All earlier TestJudgeCheckLabel rows still pass unchanged"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestFactChecks_Label\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^TestJudgeCheckLabel$' ./internal/checks
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/fact-not-asked'
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/fact-asked'
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/file-exists-unchanged'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-glob-tests
    agent: builder
    description: >-
      checks_glob_test.go: delete TestGlobCompiledReuse. First confirm TestGlobMatch has a **/*.go case that matches main.go and a/b/main.go
      and rejects a/b/main.txt (add it as a row only if it is missing). Commit as test(checks).
    dependencies: ["prune-fact-tests"]
    acceptance_criteria:
      - "Before editing, TestGlobMatch and TestGlobCompiledReuse were run and passed"
      - "func TestGlobCompiledReuse no longer exists in internal/checks/*_test.go"
      - "TestGlobMatch still passes and covers the **/*.go match and non-match cases"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestGlobCompiledReuse\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^TestGlobMatch$' ./internal/checks
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-judge-verdict-tests
    agent: builder
    description: >-
      checks_judge_test.go: delete TestJudgeCheckVocabulary, TestJudgeCheckNilJudgeFails and TestJudgeCheckEvaluatorStaysIndependent;
      merge TestJudgeCheckNoFails into TestJudgeCheckYesPasses as a verdict table with rows verdict-yes (keeps all three YesPasses
      assertions) and verdict-no (judge answered no: criterion missing). Partners TestJudgeCheckTurnFieldValidation and
      TestJudgeCheckNilJudgeFailsEvenWithFiles stay unchanged. Commit as test(checks).
    dependencies: ["prune-glob-tests"]
    acceptance_criteria:
      - "Before editing, all four removed tests and the partners TestJudgeCheckYesPasses, TestJudgeCheckTurnFieldValidation and TestJudgeCheckNilJudgeFailsEvenWithFiles were run and passed"
      - "func TestJudgeCheckVocabulary, TestJudgeCheckNilJudgeFails, TestJudgeCheckEvaluatorStaysIndependent and TestJudgeCheckNoFails no longer exist in internal/checks/*_test.go"
      - "TestJudgeCheckYesPasses has subtests verdict-yes and verdict-no; verdict-yes still asserts pass, detail containing 'judge answered yes: clearly stated', and Type==CheckJudge with a label containing 'judge'; verdict-no asserts the failure detail 'judge answered no: criterion missing'"
      - "TestJudgeCheckTurnFieldValidation and TestJudgeCheckNilJudgeFailsEvenWithFiles are unchanged"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestJudgeCheckVocabulary|TestJudgeCheckNilJudgeFails|TestJudgeCheckEvaluatorStaysIndependent|TestJudgeCheckNoFails)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^(TestJudgeCheckYesPasses|TestJudgeCheckTurnFieldValidation|TestJudgeCheckNilJudgeFailsEvenWithFiles)$' ./internal/checks
      - |-
        go test -count=1 -run '^TestJudgeCheckYesPasses$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckYesPasses/verdict-yes'
      - |-
        go test -count=1 -run '^TestJudgeCheckYesPasses$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckYesPasses/verdict-no'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-judge-fixture-tests
    agent: builder
    description: >-
      checks_judge_test.go: merge TestJudgeCheckManyLargeFilesNoPanic into TestJudgeCheckFileCaps as subtest many-large-files-total-budget
      (five 70000-byte files with non-colliding names; keep all assertions and move the shownBytes regression comment onto it);
      merge TestJudgeCheckFilesReadBeforeCommandChecks into TestJudgeCheckRunsInNonCommandPassInListOrder by adding an a.md fixture with
      content 'original' that the first judge check reads, the existing command check rewrites, and the judge closure asserts it sees as 'original'
      (do not change the partner's check list or index assertions); merge TestJudgeCheckOneElementTurnsHasNoTranscript into
      TestJudgeCheckClassicInputHasNoTranscript as one table with rows classic-input and one-element-turns. Commit as test(checks).
    dependencies: ["prune-judge-verdict-tests"]
    acceptance_criteria:
      - "Before editing, the three removed tests and their three partners were run and passed"
      - "func TestJudgeCheckManyLargeFilesNoPanic, TestJudgeCheckFilesReadBeforeCommandChecks and TestJudgeCheckOneElementTurnsHasNoTranscript no longer exist in internal/checks/*_test.go"
      - "TestJudgeCheckFileCaps has a passing subtest many-large-files-total-budget asserting: check passes, all five files returned, every file Truncated, total shown bytes <= judgeMaxTotalBytes; the existing 'per file' and 'total' subtests are unchanged"
      - "TestJudgeCheckRunsInNonCommandPassInListOrder asserts the judge sees a.md content 'original' although a command check later rewrites it, and all its original assertions (call order first,second; indexes; all passed; result types) are intact"
      - "TestJudgeCheckClassicInputHasNoTranscript has subtests classic-input and one-element-turns; each asserts pass, empty Transcript, unchanged Input and Output, and that turn 2 fails with 'the case has 1 turn(s)' without invoking the judge; the classic row keeps its q.Turn==0 and turn:1 assertions"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestJudgeCheckManyLargeFilesNoPanic|TestJudgeCheckFilesReadBeforeCommandChecks|TestJudgeCheckOneElementTurnsHasNoTranscript)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^(TestJudgeCheckFileCaps|TestJudgeCheckRunsInNonCommandPassInListOrder|TestJudgeCheckClassicInputHasNoTranscript)$' ./internal/checks
      - |-
        go test -count=1 -run '^TestJudgeCheckFileCaps$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckFileCaps/many-large-files-total-budget'
      - |-
        go test -count=1 -run '^TestJudgeCheckClassicInputHasNoTranscript$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckClassicInputHasNoTranscript/one-element-turns'
      - |-
        go test -count=1 -run '^TestJudgeCheckClassicInputHasNoTranscript$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckClassicInputHasNoTranscript/classic-input'
      - |-
        grep -q '"original"' internal/checks/checks_judge_test.go
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-checks-tests
    agent: builder
    description: >-
      checks_test.go: delete TestChecksCheckTypeSet (TestChecksDocsCoversEveryCheckType is the partner and stays unchanged);
      merge TestChecksValidateNilCriterionSkipsCrossCheck into TestChecksValidateOptionalCriterion as subtest nil-callback-skips-cross-check;
      merge TestChecksValidateEmptyList into TestChecksValidateTable as a row named empty-list (add an optional whole-list field to the row
      struct so nil can be passed; every existing row keeps its behavior). Commit as test(checks).
    dependencies: ["prune-judge-fixture-tests"]
    acceptance_criteria:
      - "Before editing, the three removed tests and the partners TestChecksValidateOptionalCriterion, TestChecksValidateTable and TestChecksDocsCoversEveryCheckType were run and passed"
      - "func TestChecksCheckTypeSet, TestChecksValidateNilCriterionSkipsCrossCheck and TestChecksValidateEmptyList no longer exist in internal/checks/*_test.go"
      - "TestChecksValidateOptionalCriterion keeps all its original assertions (required-criterion error text, OptionalCriterion accepted, callback sees [\"\" \"clarity\"]) and gains a passing subtest nil-callback-skips-cross-check"
      - "TestChecksValidateTable has a passing row empty-list that validates a nil list with no error, and all existing rows still run through the same assertions"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestChecksCheckTypeSet|TestChecksValidateNilCriterionSkipsCrossCheck|TestChecksValidateEmptyList)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^(TestChecksValidateOptionalCriterion|TestChecksValidateTable|TestChecksDocsCoversEveryCheckType)$' ./internal/checks
      - |-
        go test -count=1 -run '^TestChecksValidateOptionalCriterion$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestChecksValidateOptionalCriterion/nil-callback-skips-cross-check'
      - |-
        go test -count=1 -run '^TestChecksValidateTable$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestChecksValidateTable/empty-list'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-turn-validate-tests
    agent: builder
    description: >-
      checks_turn_test.go: merge TestCheckTurnYAMLDecodes into TestChecksYAMLDecodesChecksList (checks_test.go) by appending a fourth
      gh_log_contains check with turn: 3 to the sample, raising the length assertion to 4, asserting Turn==3 on it and Turn==nil on an
      existing entry; merge TestCheckTurnValidateAllowedTypes into TestCheckTurnValidateRejectedTypes as subtests allowed_output_contains,
      allowed_output_not_contains, allowed_gh_log_contains, allowed_gh_log_not_contains; merge TestCheckTurnValidateRejectsBelowOne into
      TestDocumentCheckValidate (checks_document_test.go) by turning its 'turn below one' subtest into a table over check types with rows
      output_document_valid 0 (existing), output_contains 0 and output_contains -1, each asserting an error containing 'turn' and '>= 1'.
      Commit as test(checks).
    dependencies: ["prune-checks-tests"]
    acceptance_criteria:
      - "Before editing, the three removed tests and the partners TestChecksYAMLDecodesChecksList, TestCheckTurnValidateRejectedTypes and TestDocumentCheckValidate were run and passed"
      - "func TestCheckTurnYAMLDecodes, TestCheckTurnValidateAllowedTypes and TestCheckTurnValidateRejectsBelowOne no longer exist in internal/checks/*_test.go"
      - "TestChecksYAMLDecodesChecksList keeps every assertion on its first three entries, now expects 4 checks, asserts the turn: 3 entry decodes to 3 and an entry without turn decodes to nil"
      - "TestCheckTurnValidateRejectedTypes keeps its six rejected-type subtests (names and assertions unchanged) and gains four passing allowed_<type> subtests that require Validate to return nil"
      - "TestDocumentCheckValidate keeps all its other assertions (valid forms, seven rejects-<field> subtests); its below-one case is a table whose rows include the original output_document_valid turn 0 and new output_contains turn 0 and -1"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCheckTurnYAMLDecodes|TestCheckTurnValidateAllowedTypes|TestCheckTurnValidateRejectsBelowOne)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^(TestChecksYAMLDecodesChecksList|TestCheckTurnValidateRejectedTypes|TestDocumentCheckValidate)$' ./internal/checks
      - |-
        go test -count=1 -run '^TestCheckTurnValidateRejectedTypes$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestCheckTurnValidateRejectedTypes/allowed_output_contains'
      - |-
        go test -count=1 -run '^TestCheckTurnValidateRejectedTypes$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestCheckTurnValidateRejectedTypes/allowed_gh_log_not_contains'
      - |-
        go test -count=1 -run '^TestDocumentCheckValidate$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestDocumentCheckValidate/turn_below_one/output_contains_-1'
      - |-
        go test -count=1 -run '^TestDocumentCheckValidate$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestDocumentCheckValidate/turn_below_one/output_document_valid_0'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-turn-eval-tests
    agent: builder
    description: >-
      checks_turn_test.go: merge TestCheckTurnEvalPerTurnOutput into TestCheckTurnEvalPerTurnGHLog as one table over evidence kind with
      subtests gh-log (the partner's existing four assertions) and output (the four output assertions, using threeTurns());
      merge TestCheckTurnLabel into the TestJudgeCheckLabel table (checks_judge_test.go) as rows gh-log-with-turn
      (#2 gh_log_contains pattern=/issue create/ turn=3), gh-log-without-turn (#2 gh_log_contains pattern=/issue create/) and
      file-exists-no-turn (label of a file_exists check without a turn does not contain 'turn'). Commit as test(checks).
    dependencies: ["prune-turn-validate-tests"]
    acceptance_criteria:
      - "Before editing, TestCheckTurnEvalPerTurnOutput, TestCheckTurnLabel, TestCheckTurnEvalPerTurnGHLog and TestJudgeCheckLabel were run and passed"
      - "func TestCheckTurnEvalPerTurnOutput and TestCheckTurnLabel no longer exist in internal/checks/*_test.go"
      - "TestCheckTurnEvalPerTurnGHLog has passing subtests gh-log and output; gh-log keeps all four original assertions (including both fail detail strings 'not found in gh log' and 'matched in gh log'); output keeps all four removed assertions (including 'not found in output' and 'matched in output')"
      - "TestJudgeCheckLabel has passing rows gh-log-with-turn, gh-log-without-turn and file-exists-no-turn; every earlier row still passes"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestCheckTurnEvalPerTurnOutput|TestCheckTurnLabel)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^(TestCheckTurnEvalPerTurnGHLog|TestJudgeCheckLabel)$' ./internal/checks
      - |-
        go test -count=1 -run '^TestCheckTurnEvalPerTurnGHLog$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestCheckTurnEvalPerTurnGHLog/output'
      - |-
        go test -count=1 -run '^TestCheckTurnEvalPerTurnGHLog$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestCheckTurnEvalPerTurnGHLog/gh-log'
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/gh-log-with-turn'
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/gh-log-without-turn'
      - |-
        go test -count=1 -run '^TestJudgeCheckLabel$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestJudgeCheckLabel/file-exists-no-turn'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-infer-tests
    agent: builder
    description: >-
      judge_infer_test.go: merge TestInferenceJudgeUnpinnedNoStub into TestInferenceJudgeRequest as a two-row table: pinned-with-stub
      (the existing body, all assertions unchanged) and unpinned-no-stub (InferenceJudge{Backend: b}, query {Question: "q"}, no error,
      request Model empty and Stub nil). Commit as test(checks).
    dependencies: ["prune-turn-eval-tests"]
    acceptance_criteria:
      - "Before editing, TestInferenceJudgeUnpinnedNoStub and TestInferenceJudgeRequest were run and passed"
      - "func TestInferenceJudgeUnpinnedNoStub no longer exists in internal/checks/*_test.go"
      - "TestInferenceJudgeRequest has passing subtests pinned-with-stub and unpinned-no-stub; pinned-with-stub keeps the verdict, single-call, Role, YesNo, Timeout (2m), Model, Stub and Prompt assertions; unpinned-no-stub asserts Model == \"\" and Stub == nil"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func TestInferenceJudgeUnpinnedNoStub\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^TestInferenceJudgeRequest$' ./internal/checks
      - |-
        go test -count=1 -run '^TestInferenceJudgeRequest$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestInferenceJudgeRequest/pinned-with-stub'
      - |-
        go test -count=1 -run '^TestInferenceJudgeRequest$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestInferenceJudgeRequest/unpinned-no-stub'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-prompt-tests
    agent: builder
    description: >-
      judge_prompt_test.go: merge TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks and TestBuildYesNoPromptTranscriptMarkerCount into
      TestBuildYesNoPromptTranscriptOrderAndFences as subtests omits-single-turn-blocks (none of 'AGENT FINAL OUTPUT',
      'BEGIN INPUT GIVEN TO THE AGENT', 'SHOULD-NOT-APPEAR-INPUT' appear) and marker-count (4 '===BEGIN TURN ' and 4 '===END TURN '
      markers, keeping the verbatim-bodies comment). Reuse the partner's twoTurnQuery() prompt. Existing partner assertions stay unchanged.
      Commit as test(checks).
    dependencies: ["prune-infer-tests"]
    acceptance_criteria:
      - "Before editing, the two removed tests and TestBuildYesNoPromptTranscriptOrderAndFences were run and passed"
      - "func TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks and TestBuildYesNoPromptTranscriptMarkerCount no longer exist in internal/checks/*_test.go"
      - "TestBuildYesNoPromptTranscriptOrderAndFences keeps its original block-order, preamble, QUESTION, judged-turn and file-block assertions and has passing subtests omits-single-turn-blocks and marker-count"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks|TestBuildYesNoPromptTranscriptMarkerCount)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^TestBuildYesNoPromptTranscriptOrderAndFences$' ./internal/checks
      - |-
        go test -count=1 -run '^TestBuildYesNoPromptTranscriptOrderAndFences$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestBuildYesNoPromptTranscriptOrderAndFences/omits-single-turn-blocks'
      - |-
        go test -count=1 -run '^TestBuildYesNoPromptTranscriptOrderAndFences$' -v ./internal/checks 2>&1 | grep -q -- '--- PASS: TestBuildYesNoPromptTranscriptOrderAndFences/marker-count'
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: prune-load-tests
    agent: builder
    description: >-
      load_test.go: delete TestLoadFile_EvidenceNeedsHiddenDirForInject, TestFixtures_NoAbsolutePaths and TestFixtures_Content. Before
      deleting the first, confirm the cmd-level partner TestCheckEvidenceFlags (cmd/kairon/cmd/check_test.go) passes, including its subtest
      'without hidden-dir the inject check is invalid'. Keep fixtureDir and the other load tests; remove only imports that become unused.
      Commit as test(checks).
    dependencies: ["prune-prompt-tests"]
    acceptance_criteria:
      - "Before editing, go test -run '^TestCheckEvidenceFlags$' ./cmd/kairon/cmd passed, including the subtest without_hidden-dir_the_inject_check_is_invalid"
      - "func TestLoadFile_EvidenceNeedsHiddenDirForInject, TestFixtures_NoAbsolutePaths and TestFixtures_Content no longer exist in internal/checks/*_test.go"
      - "TestLoadFile_Fixtures, TestLoadFile_InvalidFixtureLoadsButFailsValidate, TestLoadFile_Rejections and every other remaining load test still pass unchanged"
      - "Only _test.go files changed; go vet and gofmt are clean; go test ./internal/checks passes"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestLoadFile_EvidenceNeedsHiddenDirForInject|TestFixtures_NoAbsolutePaths|TestFixtures_Content)\(' internal/checks/*_test.go)"
      - go test -count=1 -run '^TestCheckEvidenceFlags$' -v ./cmd/kairon/cmd
      - go test -count=1 -run '^TestLoadFile' ./internal/checks
      - go vet ./internal/checks
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go test -count=1 ./internal/checks

  - id: gate-coverage-and-task-test
    agent: builder
    description: >-
      Run the full gate after all pruning tasks. Confirm go test -cover ./internal/checks reports at least 83.8% and task test exits 0.
      If coverage fell below 83.8%, find the removed or merged test that carried a statement nothing else covers and restore that input as
      a table row in the relevant partner (test-only; no production changes, no removed assertions). Make no other change when everything passes.
    dependencies: ["prune-load-tests"]
    acceptance_criteria:
      - "go test -count=1 -cover ./internal/checks reports coverage of at least 83.8% of statements"
      - "task test exits 0 (go test -v -race -coverprofile=coverage.out ./... passes for the whole module)"
      - "gofmt -l internal/checks prints nothing and go vet ./internal/checks passes"
      - "No file other than _test.go files under internal/checks differs from the base branch outside .kairon/ and .kiro/"
    validation_commands:
      - |-
        go test -count=1 -cover ./internal/checks | awk '/coverage:/ {v=$5; gsub("%","",v); print "coverage", v; if (v+0 < 83.8) exit 1; found=1} END {if (!found) exit 1}'
      - task test
      - |-
        test -z "$(gofmt -l internal/checks)"
      - go vet ./internal/checks
      - |-
        test -z "$(git diff --name-only $(git merge-base HEAD origin/main) -- . ':!*_test.go' ':!.kairon' ':!.kiro')"

  - id: validate-prune
    agent: validator
    description: >-
      Read-only end-to-end verification against the issue. Check AC1 (Delete grep prints nothing), AC2 (Merge grep prints nothing; each
      partner passes with -v and shows its prescribed subtest as PASS; the inline merges are present), contract C1 (only _test.go files changed;
      each merged partner keeps every assertion it had at the base commit, except the Index/Type/len assertions the issue says are already
      covered generically), AC3 (coverage >= 83.8%) and AC4 (task test exits 0). Check every one of the 9 Deletes and 16 Merges in the
      spec's test-to-task map individually. Report any deferral wording or unrecorded deviation from the spec.
    dependencies: ["gate-coverage-and-task-test"]
    acceptance_criteria:
      - "The AC1 grep over internal/checks/*_test.go prints nothing (all 9 Delete tests are gone)"
      - "The AC2 grep over internal/checks/*_test.go prints nothing (all 16 Merge tests are gone as separate functions)"
      - "Each of the 12 partners that receive a merged case passes with -v and lists the prescribed subtest as PASS: TestJudgeCheckLabel (document-result-label, document-label-with-turn, fact-not-asked, fact-asked, file-exists-unchanged, gh-log-with-turn, gh-log-without-turn, file-exists-no-turn), TestJudgeCheckYesPasses (verdict-yes, verdict-no), TestJudgeCheckFileCaps (many-large-files-total-budget), TestJudgeCheckClassicInputHasNoTranscript (classic-input, one-element-turns), TestChecksValidateOptionalCriterion (nil-callback-skips-cross-check), TestChecksValidateTable (empty-list), TestCheckTurnValidateRejectedTypes (allowed_*), TestDocumentCheckValidate (turn_below_one/*), TestCheckTurnEvalPerTurnGHLog (gh-log, output), TestInferenceJudgeRequest (pinned-with-stub, unpinned-no-stub), TestBuildYesNoPromptTranscriptOrderAndFences (omits-single-turn-blocks, marker-count)"
      - "The inline merges are present: the 'original' file-content assertion in TestJudgeCheckRunsInNonCommandPassInListOrder, and the turn: 3 decode sample in TestChecksYAMLDecodesChecksList"
      - "git diff against the merge base shows no non-test file changed outside .kairon/ and .kiro/, and no original partner assertion was removed or weakened"
      - "go test -count=1 -cover ./internal/checks reports at least 83.8%"
      - "task test exits 0"
    validation_commands:
      - |-
        test -z "$(grep -nE 'func (TestChecksCheckTypeSet|TestDocumentCheckSpecAllowsOnlyTurn|TestFixtures_Content|TestFixtures_NoAbsolutePaths|TestGlobCompiledReuse|TestJudgeCheckEvaluatorStaysIndependent|TestJudgeCheckNilJudgeFails|TestJudgeCheckVocabulary|TestLoadFile_EvidenceNeedsHiddenDirForInject)\(' internal/checks/*_test.go)"
      - |-
        test -z "$(grep -nE 'func (TestBuildYesNoPromptTranscriptMarkerCount|TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks|TestCheckTurnEvalPerTurnOutput|TestCheckTurnLabel|TestCheckTurnValidateAllowedTypes|TestCheckTurnValidateRejectsBelowOne|TestCheckTurnYAMLDecodes|TestChecksValidateEmptyList|TestChecksValidateNilCriterionSkipsCrossCheck|TestDocumentCheckResultShape|TestFactChecks_Label|TestInferenceJudgeUnpinnedNoStub|TestJudgeCheckFilesReadBeforeCommandChecks|TestJudgeCheckManyLargeFilesNoPanic|TestJudgeCheckNoFails|TestJudgeCheckOneElementTurnsHasNoTranscript)\(' internal/checks/*_test.go)"
      - go test -count=1 -v -run '^(TestJudgeCheckLabel|TestJudgeCheckYesPasses|TestJudgeCheckFileCaps|TestJudgeCheckRunsInNonCommandPassInListOrder|TestJudgeCheckClassicInputHasNoTranscript|TestChecksValidateOptionalCriterion|TestChecksValidateTable|TestChecksYAMLDecodesChecksList|TestCheckTurnValidateRejectedTypes|TestDocumentCheckValidate|TestCheckTurnEvalPerTurnGHLog|TestInferenceJudgeRequest|TestBuildYesNoPromptTranscriptOrderAndFences)$' ./internal/checks
      - |-
        test -z "$(git diff --name-only $(git merge-base HEAD origin/main) -- . ':!*_test.go' ':!.kairon' ':!.kiro')"
      - |-
        go test -count=1 -cover ./internal/checks | awk '/coverage:/ {v=$5; gsub("%","",v); print "coverage", v; if (v+0 < 83.8) exit 1; found=1} END {if (!found) exit 1}'
      - task test
```
