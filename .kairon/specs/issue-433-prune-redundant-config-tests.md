# Design Spec: Prune redundant internal/config tests

Closes #433

## Scope Coverage

Every In Scope item, every row of the issue's Current / Target State table, the Behavior-Preservation Contract and every acceptance criterion of issue #433 is listed. Task IDs refer to the `kiro-plan` block at the end of this spec.

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/config`: `config_test.go`, `evals_test.go`, `pricing_files_test.go`, `pricing_test.go`, `sandbox_test.go`, `themes_test.go`" | `prune-config-tests`, `prune-evals-tests`, `prune-pricing-tests`, `prune-sandbox-tests`, `prune-themes-tests` |
| R1 | Table row: "`TestLoad_EnableCopilotReviewDefaults` \| `internal/config/config_test.go:9` \| Combine into one table test with `TestLoad_BaseBranch`, `TestLoad_UserField`" | `prune-config-tests` |
| R2 | Table row: "`TestLoad_UserField` \| `internal/config/config_test.go:134` \| Combine into one table test with `TestLoad_BaseBranch`, `TestLoad_EnableCopilotReviewDefaults`" | `prune-config-tests` |
| R3 | Table row: "`TestLoad_BaseBranch` \| `internal/config/config_test.go:193` \| Combine into one table test with `TestLoad_EnableCopilotReviewDefaults`, `TestLoad_UserField`" | `prune-config-tests` |
| R4 | Table row: "`TestDefaultEvalsConfig` \| `internal/config/evals_test.go:39` \| Delete \| `TestLoadEvals_PartialBlockKeepsOtherDefaults`" | `prune-evals-tests` |
| R5 | Table row: "`TestLoadEvals_MissingFileReturnsDefaults` \| `internal/config/evals_test.go:61` \| Combine into one table test with `TestLoadPricing_MissingFileReturnsDefaults`" | `prune-evals-tests`, `prune-pricing-tests` |
| R6 | Table row: "`TestLoadEvals_NoEvalsKeyReturnsDefaults` \| `internal/config/evals_test.go:72` \| Combine into one table test with `TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo`, `TestLoadPricing_NullKeyReturnsDefaults`" | `prune-evals-tests`, `prune-pricing-tests` |
| R7 | Table row: "`TestLoadEvals_AllowedModelsReplacesDefault` \| `internal/config/evals_test.go:120` \| Merge into partner \| `TestLoadEvals_PartialBlockKeepsOtherDefaults`" | `prune-evals-tests` |
| R8 | Table row: "`TestLoadEvals_ExplicitEmptyJudgeModelStaysEmpty` \| `internal/config/evals_test.go:158` \| Merge into partner \| `TestLoadEvals_PartialBlockKeepsOtherDefaults`" | `prune-evals-tests` |
| R9 | Table row: "`TestLoadEvals_ExplicitEmptyAllowedModels` \| `internal/config/evals_test.go:172` \| Merge into partner \| `TestLoadEvals_PartialBlockKeepsOtherDefaults`" | `prune-evals-tests` |
| R10 | Table row: "`TestLoad_ExposesEvalsDefaults` \| `internal/config/evals_test.go:229` \| Merge into partner \| `TestLoad_AllDefaultValues`" | `prune-config-tests`, `prune-evals-tests` |
| R11 | Table row: "`TestLoad_EvalsAllowedModelsReplacesDefault` \| `internal/config/evals_test.go:259` \| Merge into partner \| `TestLoad_EvalsPartialBlockAndNoModelRejection`" | `prune-evals-tests` |
| R12 | Table row: "`TestLoadEvals_TrustToolsDefaultsNil` \| `internal/config/evals_test.go:343` \| Merge into partner \| `TestLoadEvals_PartialBlockKeepsOtherDefaults`" | `prune-evals-tests` |
| R13 | Table row: "`TestShippedConfigFiles_PricingBlockIsDocumented` \| `internal/config/pricing_files_test.go:86` \| Delete \| `TestShippedConfigFiles_PricingBlockPricesEverySonnetModel`" | `prune-pricing-tests` |
| R14 | Table row: "`TestLoadPricing_MissingFileReturnsDefaults` \| `internal/config/pricing_test.go:56` \| Combine into one table test with `TestLoadEvals_MissingFileReturnsDefaults`" | `prune-evals-tests`, `prune-pricing-tests` |
| R15 | Table row: "`TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo` \| `internal/config/pricing_test.go:67` \| Combine into one table test with `TestLoadEvals_NoEvalsKeyReturnsDefaults`, `TestLoadPricing_NullKeyReturnsDefaults`" | `prune-evals-tests`, `prune-pricing-tests` |
| R16 | Table row: "`TestLoadPricing_NullKeyReturnsDefaults` \| `internal/config/pricing_test.go:79` \| Combine into one table test with `TestLoadEvals_NoEvalsKeyReturnsDefaults`, `TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo`" | `prune-evals-tests`, `prune-pricing-tests` |
| R17 | Table row: "`TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable` \| `internal/config/pricing_test.go:106` \| Merge into partner \| `TestLoadPricing_PresentKeyReplacesDefaults`" | `prune-pricing-tests` |
| R18 | Table row: "`TestLoad_NoPricingKeyUsesDefaults` \| `internal/config/pricing_test.go:232` \| Merge into partner \| `TestLoad_AllDefaultValues`" | `prune-config-tests`, `prune-pricing-tests` |
| R19 | Table row: "`TestLoad_PresentPricingKeyReplacesDefaults` \| `internal/config/pricing_test.go:243` \| Delete \| `TestLoadPricing_PresentKeyReplacesDefaults`" | `prune-pricing-tests` |
| R20 | Table row: "`TestLoad_SandboxConfigDefaults` \| `internal/config/sandbox_test.go:10` \| Merge into partner \| `TestLoad_SandboxConfigParsing`" | `prune-sandbox-tests` |
| R21 | Table row: "`TestLoad_DurationParsing` \| `internal/config/sandbox_test.go:238` \| Delete \| `TestLoad_SandboxConfigParsing`" | `prune-sandbox-tests` |
| R22 | Table row: "`TestLoadThemeReturnsErrorForMissingTheme` \| `internal/config/themes_test.go:103` \| Merge into partner \| `TestLoadThemeInvalidName`" | `prune-themes-tests` |
| C1 | Behavior-Preservation Contract: "No production code changes. Every test not listed passes unchanged. Merged partners keep every assertion they had." | `baseline-record`, `prune-config-tests`, `prune-evals-tests`, `prune-pricing-tests`, `prune-sandbox-tests`, `prune-themes-tests`, `validate-prune` |
| AC1 | "The 4 tests marked Delete no longer exist." | `prune-evals-tests`, `prune-pricing-tests`, `prune-sandbox-tests`, `validate-prune` |
| AC2 | "Each of the 10 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-config-tests`, `prune-evals-tests`, `prune-pricing-tests`, `prune-sandbox-tests`, `prune-themes-tests`, `validate-prune` |
| AC3 | "Each of the 3 groups marked Combine is one table-driven test with a case for each former test, and the former functions are gone." | `prune-config-tests`, `prune-evals-tests`, `prune-pricing-tests`, `validate-prune` |
| AC4 | "Package coverage does not drop." | `baseline-record`, `prune-config-tests`, `prune-evals-tests`, `prune-pricing-tests`, `prune-sandbox-tests`, `prune-themes-tests`, `validate-prune` |
| AC5 | "Every remaining test passes unchanged, and the gate passes." | `baseline-record`, `validate-prune` |

## Deferred work

none

## Overview

The test audit (`.kairon/research/test-audit-verdicts.md`, §8 of `.kairon/research/test-performance-analysis.md`) found 22 tests in `internal/config` that do not earn their runtime. This change prunes them: 4 deleted, 10 merged into named partners, and 8 combined into 3 table-driven tests. It touches six `_test.go` files in `internal/config` and nothing else. No production code changes. This is a test refactor, so the TDD rules for refactors apply: confirm the existing tests pass before the change, keep them passing after it, and add no failing-test-first step (AGENTS.md "Refactors").

## Solution Approach

### Facts measured on the current worktree

- `go test -cover ./internal/config` reports **79.0%**. The exact figure is 177 of 224 statements (79.018%). The AC4 floor of 79.0% therefore has **no slack**: losing one covered statement gives 176/224 = 78.57% and prints `78.6%`, which fails AC4.
- Re-running with the four Delete targets skipped (`go test -cover -skip '^(TestDefaultEvalsConfig|TestLoad_DurationParsing|TestLoad_PresentPricingKeyReplacesDefaults|TestShippedConfigFiles_PricingBlockIsDocumented)$' ./internal/config`) still gives 177/224. Skipping `TestLoadThemeReturnsErrorForMissingTheme` also still gives 177/224. So the deletions themselves are coverage-neutral. The risk is a merge or combine that drops an input, so every row below must keep its original YAML input and the loader call it exercised.
- `writeProjectConfig(t, *string)` and `strPtr` are defined in `evals_test.go` and are visible to every test file in the package. `writeProjectConfig` calls `t.Chdir`, so tests that use it must not call `t.Parallel()`.
- Nothing outside `internal/config` references the removed test names, except the research docs and generated eval results, which are historical or descriptive and are left alone.

### Rules every edit follows

1. **Keep inputs, drop duplicates.** A merged test's YAML input and its distinct assertion move into the partner. An assertion the partner already makes is not copied.
2. **Partners keep every assertion they had.** Existing assertions in `TestLoadEvals_PartialBlockKeepsOtherDefaults`, `TestLoad_AllDefaultValues`, `TestLoad_EvalsPartialBlockAndNoModelRejection`, `TestLoadPricing_PresentKeyReplacesDefaults`, `TestLoad_SandboxConfigParsing` and `TestLoadThemeInvalidName` stay, with the same semantics.
3. **Combined tables name their test.** The three Combine groups become three named table tests (names fixed below, and the PR description names each).
4. **New names must not collide with removed names.** The AC1/AC2/AC3 greps are `grep -nE 'func (<removed names>)\(' internal/config/*_test.go` and must print nothing. None of the new table test names equals a removed function name. The new names deliberately do not reuse the removed `TestLoad_BaseBranch`, `TestLoad_UserField`, `TestLoad_EnableCopilotReviewDefaults`, `TestLoadEvals_*` or `TestLoadPricing_*` function names, even as prefixes. Subtest names (strings passed to `t.Run`) are not `func` declarations and are free to echo the former names.
5. **No silent assertion loss.** If keeping a case is impossible, stop and report BLOCKED. Do not drop it.
6. **Clean imports and helpers.** Removing tests can leave unused imports (`os`, `reflect`, `inference`, `time`, `strings`) or unused helpers in a file. Remove only what becomes unused. `go vet ./internal/config` and `gofmt` must be clean. Adding a test that uses `reflect` or `inference` to a file may require adding that import there.

### Final test layout

**Group 1: `TestLoad_ScalarFieldDecoding` in `config_test.go`** (replaces `TestLoad_EnableCopilotReviewDefaults`, `TestLoad_UserField`, `TestLoad_BaseBranch`).
One table of `(name, yaml, get-field, want)` rows built on `writeProjectConfig` and `Load()`, which replaces about 90 lines of chdir boilerplate. It has three top-level subtests, one per former test: `EnableCopilotReviewDefaults`, `UserField`, `BaseBranch`. Each contains the former test's cases as nested subtests, all 8 kept: 3 copilot rows (unset gives true, explicit true, explicit false), 2 user rows (omitted gives `""`, `octocat`), 3 base_branch rows (omitted gives `""`, `develop`, only-in-a-comment gives `""`). The "only in a comment" row stays, because the issue says combine, not delete. Each row asserts `Load()` returns no error and the field equals the expected value, as the originals did.

**Group 2: `TestLoadEvalsAndPricing_MissingFileReturnsDefaults` in `evals_test.go`** (replaces `TestLoadEvals_MissingFileReturnsDefaults`, `TestLoadPricing_MissingFileReturnsDefaults`).
Subtests `LoadEvals` and `LoadPricing`. Each calls `writeProjectConfig(t, nil)`, calls its loader, requires `err == nil` and requires `reflect.DeepEqual(got, <default>)`. Defaults are `DefaultEvalsConfig()` and `DefaultPricing()`. The two loaders return different types, so rows use a closure of the shape `func() (any, error)` plus a `want any`. `evals_test.go` needs the `inference` import only if the builder types a row with it. `any` closures avoid that.

**Group 3: `TestLoadEvalsAndPricing_AbsentKeyReturnsDefaults` in `evals_test.go`** (replaces `TestLoadEvals_NoEvalsKeyReturnsDefaults`, `TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo`, `TestLoadPricing_NullKeyReturnsDefaults`).
Subtests `LoadEvals_NoEvalsKey` (yaml `label: foo\n`), `LoadPricing_NoPricingKeyWithoutRepo` (yaml `label: foo\n`) and `LoadPricing_NullKey` (yaml `pricing:\n`). All three keep the "no `repo` key, the loader must not require it" property. Each requires `err == nil` and `reflect.DeepEqual` against the defaults.

**Merges into `TestLoadEvals_PartialBlockKeepsOtherDefaults`** (`evals_test.go`).
Convert it to a table-driven test with one `check` function per row (rows must not be forced into a single `DeepEqual`, because `allowed_models: []` must keep its `len == 0` assertion and a `DeepEqual` against `[]string{}` or `nil` would change its meaning). Rows, with fixed subtest names:

| Subtest | YAML | Assertions kept |
|---------|------|-----------------|
| `partial_block` | `evals:\n  agent_model: claude-sonnet-4.5\n` | `AgentModel == "claude-sonnet-4.5"`, `JudgeModel == DefaultJudgeModel`, `AllowedModels` deep-equals `wantDefaultAllowed` (the original test, unchanged) |
| `allowed_models_replaces_default` | `evals:\n  allowed_models:\n    - my-model\n    - other-model\n` | `AllowedModels` deep-equals `{"my-model","other-model"}` (replace, not append) |
| `explicit_empty_judge_model` | `evals:\n  judge_model: ""\n` | `JudgeModel == ""` |
| `explicit_empty_allowed_models` | `evals:\n  allowed_models: []\n` | `len(AllowedModels) == 0` |
| `trust_tools_defaults_nil` | `evals:\n  judge_model: claude-sonnet-5.5\n` | `TrustTools == nil`, and `DefaultEvalsConfig().TrustTools == nil` |

The original `ExplicitEmptyJudgeModelStaysEmpty` also called `got.CheckModel(got.JudgeModel)` and expected an error. The issue's Reason column says that rejection "is already a `TestCheckModel` case", and it is (`TestCheckModel` subtest `empty`, which calls `CheckModel("")` on the default config and checks for `model is empty`). Following the issue, the merged row drops that call. This is the issue's instruction and not a spec deviation. The builder confirms the `empty` subtest exists before dropping it.

**Merge into `TestLoad_EvalsPartialBlockAndNoModelRejection`** (`evals_test.go`).
Convert to a two-row Load-evals table: `partial_block_and_no_model_rejection` (the existing yaml and all three existing assertions, unchanged) and `allowed_models_replaces_default` (yaml `repo: owner/name\nevals:\n  allowed_models: [only-one]\n`, asserts `Load()` returns no error and `cfg.Evals.AllowedModels` deep-equals `{"only-one"}`).

**Merges into `TestLoad_AllDefaultValues`** (`config_test.go`).
After its existing assertions, add two subtests that share the already-loaded `cfg`: `evals_defaults` asserts `reflect.DeepEqual(cfg.Evals, DefaultEvalsConfig())`, and `pricing_defaults` asserts `reflect.DeepEqual(cfg.Pricing, DefaultPricing())`. The table in the issue calls these "one more assertion", and AC2 says the merged case must be "among its subtests" of the partner. Using `t.Run` for the two new assertions satisfies both. All existing assertions in the function stay as they are. `config_test.go` gains a `reflect` import.

**Merge into `TestLoadPricing_PresentKeyReplacesDefaults`** (`pricing_test.go`).
Convert to a two-row table: `one_entry` (the existing yaml and `reflect.DeepEqual` assertion against `{"my-model": {1.5, 7}}`) and `empty_mapping` (yaml `pricing: {}\n`, asserts `err == nil` and `len(got) == 0`).

**Merge into `TestLoad_SandboxConfigParsing`** (`sandbox_test.go`).
Add a row named `no sandbox key uses defaults` with `configContent: "repo: test/repo"` and the same expected `SandboxConfig` as the existing `empty sandbox section uses defaults` row (`/workspace`, 1.0, 1024, 5m). The merged test used the identical input and expectation.

**Merge into `TestLoadThemeInvalidName`** (`themes_test.go`).
Add `"nonexistent-theme"` to the `invalidNames` list. The existing loop asserts a non-nil error and a nil theme, which is exactly the merged test's assertion. `nonexistent-theme` is a well-formed name and fails on the "theme not found" read, not on the name pattern, so add a one-line comment saying so next to the entry.

**Deletes.**
`TestDefaultEvalsConfig`, `TestLoad_DurationParsing`, `TestLoad_PresentPricingKeyReplacesDefaults`, `TestShippedConfigFiles_PricingBlockIsDocumented`. Their reasons are in the issue's table, and each partner already carries the coverage.

## Relevant Files

All paths are in `internal/config/`. All are modified; none is created or deleted.

| File | Change |
|------|--------|
| `config_test.go` | Replace three scalar-field tests with `TestLoad_ScalarFieldDecoding`; add `evals_defaults` and `pricing_defaults` subtests to `TestLoad_AllDefaultValues` |
| `evals_test.go` | Delete `TestDefaultEvalsConfig`; add Group 2 and Group 3 tables; fold four tests into `TestLoadEvals_PartialBlockKeepsOtherDefaults`; fold one into `TestLoad_EvalsPartialBlockAndNoModelRejection`; remove `TestLoad_ExposesEvalsDefaults` and the two former Evals loader tests |
| `pricing_test.go` | Remove the three former pricing loader tests (their rows live in the Group 2 and 3 tables), `TestLoad_NoPricingKeyUsesDefaults` and `TestLoad_PresentPricingKeyReplacesDefaults`; fold `EmptyMapping` into `TestLoadPricing_PresentKeyReplacesDefaults` |
| `pricing_files_test.go` | Delete `TestShippedConfigFiles_PricingBlockIsDocumented` and its doc comment |
| `sandbox_test.go` | Add one row to `TestLoad_SandboxConfigParsing`; remove `TestLoad_SandboxConfigDefaults` and `TestLoad_DurationParsing` |
| `themes_test.go` | Add one entry to `TestLoadThemeInvalidName`; remove `TestLoadThemeReturnsErrorForMissingTheme` |

Not touched: all production code, `.kairon/specs/`, `.kairon/research/`, `CHANGELOG.md` and generated eval results (see AGENTS.md). No template-synchronized file changes, so `task sync:check` is unaffected.

## Team Orchestration

All edits are in one Go package and one worktree. A builder that runs `go test ./internal/config` while another builder has a half-edited file in the same package would see a compile error that is not its own. The builder tasks therefore run **in sequence**, and no two run in parallel. The order is also chosen so that a destination assertion exists before its source test is deleted:

1. `baseline-record` (validator, read-only): record the pre-change test results, package coverage and test names.
2. `prune-config-tests` (builder): Group 1, plus the destination assertions in `TestLoad_AllDefaultValues`.
3. `prune-evals-tests` (builder): Group 2 and 3 tables, the evals merges, the evals deletes, and removal of `TestLoad_ExposesEvalsDefaults` (its assertion now lives in step 2).
4. `prune-pricing-tests` (builder): remove the pricing tests whose rows live in step 3's tables or in step 2's assertions, fold `EmptyMapping`, and delete `TestShippedConfigFiles_PricingBlockIsDocumented` in `pricing_files_test.go`.
5. `prune-sandbox-tests` (builder).
6. `prune-themes-tests` (builder).
7. `validate-prune` (validator, read-only): check every acceptance criterion.

Each builder leaves the package compiling and `go test ./internal/config` green at the end of its task, and does not lower coverage below 79.0%. Each commits with its tests, per AGENTS.md, and the PR description follows the "Deferred work: none" and "Spec deviations: none" rules.

## Step-by-Step Task Breakdown

### Task 1: `baseline-record` (validator)
Before any edit, run `go test ./internal/config -count=1`, `go test -cover ./internal/config` and `go test -list '.*' ./internal/config`. Record that the package passes, that coverage is 79.0% (177/224 statements), and the full list of test function names. Confirm the 22 target tests exist at the file locations in the issue's table. Dependencies: none.

### Task 2: `prune-config-tests` (builder)
In `config_test.go`: implement `TestLoad_ScalarFieldDecoding` (Group 1, 8 cases, three top-level subtests) using `writeProjectConfig`; delete the three former functions; add `evals_defaults` and `pricing_defaults` subtests to `TestLoad_AllDefaultValues`; fix imports. Dependencies: `baseline-record`.

### Task 3: `prune-evals-tests` (builder)
In `evals_test.go`: implement Group 2 and Group 3; fold the four evals tests into `TestLoadEvals_PartialBlockKeepsOtherDefaults`; fold `TestLoad_EvalsAllowedModelsReplacesDefault` into `TestLoad_EvalsPartialBlockAndNoModelRejection`; delete `TestDefaultEvalsConfig`, `TestLoad_ExposesEvalsDefaults`, `TestLoadEvals_MissingFileReturnsDefaults`, `TestLoadEvals_NoEvalsKeyReturnsDefaults` and the four merged functions; fix imports. Dependencies: `prune-config-tests`.

### Task 4: `prune-pricing-tests` (builder)
In `pricing_test.go`: delete `TestLoadPricing_MissingFileReturnsDefaults`, `TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo`, `TestLoadPricing_NullKeyReturnsDefaults`, `TestLoad_NoPricingKeyUsesDefaults` and `TestLoad_PresentPricingKeyReplacesDefaults`; fold `TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable` into `TestLoadPricing_PresentKeyReplacesDefaults`. In `pricing_files_test.go`: delete `TestShippedConfigFiles_PricingBlockIsDocumented`. Fix imports in both. Dependencies: `prune-evals-tests`.

### Task 5: `prune-sandbox-tests` (builder)
In `sandbox_test.go`: add the `no sandbox key uses defaults` row to `TestLoad_SandboxConfigParsing`; delete `TestLoad_SandboxConfigDefaults` and `TestLoad_DurationParsing`; fix imports. Dependencies: `prune-pricing-tests`.

### Task 6: `prune-themes-tests` (builder)
In `themes_test.go`: add `"nonexistent-theme"` to `TestLoadThemeInvalidName` with the comment; delete `TestLoadThemeReturnsErrorForMissingTheme`. Dependencies: `prune-sandbox-tests`.

### Task 7: `validate-prune` (validator)
Check AC1 to AC5 and the contract against the baseline from Task 1, without modifying files. Dependencies: `prune-themes-tests`.

## Validation Commands

Run from the project root.

```bash
# AC1: the 4 deleted tests are gone (prints nothing)
grep -nE 'func (TestDefaultEvalsConfig|TestLoad_DurationParsing|TestLoad_PresentPricingKeyReplacesDefaults|TestShippedConfigFiles_PricingBlockIsDocumented)\(' internal/config/*_test.go

# AC2: the 10 merged tests are gone as functions (prints nothing)
grep -nE 'func (TestLoadEvals_AllowedModelsReplacesDefault|TestLoadEvals_ExplicitEmptyAllowedModels|TestLoadEvals_ExplicitEmptyJudgeModelStaysEmpty|TestLoadEvals_TrustToolsDefaultsNil|TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable|TestLoadThemeReturnsErrorForMissingTheme|TestLoad_EvalsAllowedModelsReplacesDefault|TestLoad_ExposesEvalsDefaults|TestLoad_NoPricingKeyUsesDefaults|TestLoad_SandboxConfigDefaults)\(' internal/config/*_test.go

# AC2: each partner passes and lists the merged case
go test -run '^TestLoadEvals_PartialBlockKeepsOtherDefaults$' -v ./internal/config
go test -run '^TestLoad_AllDefaultValues$' -v ./internal/config
go test -run '^TestLoad_EvalsPartialBlockAndNoModelRejection$' -v ./internal/config
go test -run '^TestLoadPricing_PresentKeyReplacesDefaults$' -v ./internal/config
go test -run '^TestLoad_SandboxConfigParsing$' -v ./internal/config
go test -run '^TestLoadThemeInvalidName$' -v ./internal/config

# AC3: the 8 combined functions are gone (prints nothing)
grep -nE 'func (TestLoadEvals_MissingFileReturnsDefaults|TestLoadEvals_NoEvalsKeyReturnsDefaults|TestLoadPricing_MissingFileReturnsDefaults|TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo|TestLoadPricing_NullKeyReturnsDefaults|TestLoad_BaseBranch|TestLoad_EnableCopilotReviewDefaults|TestLoad_UserField)\(' internal/config/*_test.go

# AC3: the three table tests, one subtest per former test
go test -run '^TestLoad_ScalarFieldDecoding$' -v ./internal/config
go test -run '^TestLoadEvalsAndPricing_MissingFileReturnsDefaults$' -v ./internal/config
go test -run '^TestLoadEvalsAndPricing_AbsentKeyReturnsDefaults$' -v ./internal/config

# AC4: coverage is at least 79.0%
go test -cover ./internal/config

# AC5 and static checks
go vet ./internal/config
test -z "$(gofmt -l internal/config)"
task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "baseline-record"
    agent: "validator"
    description: "Before any edit, record the baseline for internal/config: all tests pass, package coverage is 79.0% (177 of 224 statements), the list of test function names, and that the 22 target tests exist where the issue's table says. Read-only; report the numbers in the sentinel so later tasks can compare."
    dependencies: []
    acceptance_criteria:
      - "go test ./internal/config -count=1 passes before any change"
      - "go test -cover ./internal/config reports 79.0% and the report states the figure"
      - "The sentinel lists the test function names from go test -list and confirms each of the 22 target tests is present"
      - "git diff --stat -- internal/config shows no changes at the time of recording"
    validation_commands:
      - "go test ./internal/config -count=1"
      - "go test -cover ./internal/config"
      - "go test -list '.*' ./internal/config"
      - "git diff --quiet -- internal/config"

  - id: "prune-config-tests"
    agent: "builder"
    description: "In internal/config/config_test.go replace TestLoad_EnableCopilotReviewDefaults, TestLoad_UserField and TestLoad_BaseBranch with one table test named TestLoad_ScalarFieldDecoding built on writeProjectConfig, with three top-level subtests (EnableCopilotReviewDefaults, UserField, BaseBranch) holding all 8 former cases as nested subtests. Add subtests evals_defaults (reflect.DeepEqual(cfg.Evals, DefaultEvalsConfig())) and pricing_defaults (reflect.DeepEqual(cfg.Pricing, DefaultPricing())) to TestLoad_AllDefaultValues, keeping its existing assertions. Do not use t.Parallel (writeProjectConfig uses t.Chdir). Remove only imports that become unused and add reflect. The new test name must not equal any removed function name."
    dependencies: ["baseline-record"]
    acceptance_criteria:
      - "TestLoad_EnableCopilotReviewDefaults, TestLoad_UserField and TestLoad_BaseBranch no longer exist as functions"
      - "TestLoad_ScalarFieldDecoding exists and go test -run '^TestLoad_ScalarFieldDecoding$' -v lists top-level subtests EnableCopilotReviewDefaults, UserField and BaseBranch, with 3, 2 and 3 nested cases, all passing"
      - "Every original case input and expected value is preserved, including the base_branch only-in-a-comment case"
      - "TestLoad_AllDefaultValues keeps all its original assertions and passes with subtests evals_defaults and pricing_defaults"
      - "No production file is modified; go vet and gofmt are clean for internal/config"
      - "go test -cover ./internal/config reports at least 79.0%"
    validation_commands:
      - "! grep -nE 'func (TestLoad_BaseBranch|TestLoad_EnableCopilotReviewDefaults|TestLoad_UserField)\\(' internal/config/*_test.go"
      - "go test -run '^TestLoad_ScalarFieldDecoding$' -v ./internal/config"
      - "go test -run '^TestLoad_AllDefaultValues$' -v ./internal/config"
      - "go test -count=1 ./internal/config"
      - "go test -cover ./internal/config | grep -E 'coverage: (79\\.[0-9]|[89][0-9]\\.[0-9]|100\\.0)%'"
      - "go vet ./internal/config"
      - "test -z \"$(gofmt -l internal/config)\""
      - "git diff --quiet -- cmd internal ':(exclude)internal/config/*_test.go'"

  - id: "prune-evals-tests"
    agent: "builder"
    description: "In internal/config/evals_test.go: (1) add TestLoadEvalsAndPricing_MissingFileReturnsDefaults with subtests LoadEvals and LoadPricing; (2) add TestLoadEvalsAndPricing_AbsentKeyReturnsDefaults with subtests LoadEvals_NoEvalsKey, LoadPricing_NoPricingKeyWithoutRepo (both yaml 'label: foo') and LoadPricing_NullKey (yaml 'pricing:'), each asserting err==nil and reflect.DeepEqual against the defaults; (3) convert TestLoadEvals_PartialBlockKeepsOtherDefaults to a per-row check table with subtests partial_block, allowed_models_replaces_default, explicit_empty_judge_model (JudgeModel==\"\"; drop CheckModel call, covered by TestCheckModel 'empty', as the issue states), explicit_empty_allowed_models (len==0) and trust_tools_defaults_nil (TrustTools nil and DefaultEvalsConfig().TrustTools nil); (4) convert TestLoad_EvalsPartialBlockAndNoModelRejection to a two-row table adding allowed_models_replaces_default; (5) delete TestDefaultEvalsConfig, TestLoad_ExposesEvalsDefaults (its assertion now lives in TestLoad_AllDefaultValues), TestLoadEvals_MissingFileReturnsDefaults, TestLoadEvals_NoEvalsKeyReturnsDefaults and the five merged functions listed in the Scope Coverage rows R7-R9, R11, R12. Keep TestDefaultEvalsConfigReturnsFreshSlice and all other tests unchanged. Fix imports."
    dependencies: ["prune-config-tests"]
    acceptance_criteria:
      - "TestDefaultEvalsConfig, TestLoad_ExposesEvalsDefaults, TestLoadEvals_MissingFileReturnsDefaults, TestLoadEvals_NoEvalsKeyReturnsDefaults, TestLoadEvals_AllowedModelsReplacesDefault, TestLoadEvals_ExplicitEmptyJudgeModelStaysEmpty, TestLoadEvals_ExplicitEmptyAllowedModels, TestLoadEvals_TrustToolsDefaultsNil and TestLoad_EvalsAllowedModelsReplacesDefault no longer exist as functions"
      - "TestLoadEvals_PartialBlockKeepsOtherDefaults passes with subtests partial_block, allowed_models_replaces_default, explicit_empty_judge_model, explicit_empty_allowed_models and trust_tools_defaults_nil, and the original partial-block assertions are unchanged"
      - "TestLoad_EvalsPartialBlockAndNoModelRejection passes with subtests partial_block_and_no_model_rejection and allowed_models_replaces_default"
      - "TestLoadEvalsAndPricing_MissingFileReturnsDefaults lists exactly the subtests LoadEvals and LoadPricing, both passing"
      - "TestLoadEvalsAndPricing_AbsentKeyReturnsDefaults lists exactly the subtests LoadEvals_NoEvalsKey, LoadPricing_NoPricingKeyWithoutRepo and LoadPricing_NullKey, all passing"
      - "go test -cover ./internal/config reports at least 79.0%; go vet and gofmt are clean"
    validation_commands:
      - "! grep -nE 'func (TestDefaultEvalsConfig|TestLoad_ExposesEvalsDefaults|TestLoadEvals_MissingFileReturnsDefaults|TestLoadEvals_NoEvalsKeyReturnsDefaults|TestLoadEvals_AllowedModelsReplacesDefault|TestLoadEvals_ExplicitEmptyJudgeModelStaysEmpty|TestLoadEvals_ExplicitEmptyAllowedModels|TestLoadEvals_TrustToolsDefaultsNil|TestLoad_EvalsAllowedModelsReplacesDefault)\\(' internal/config/evals_test.go"
      - "go test -run '^TestLoadEvals_PartialBlockKeepsOtherDefaults$' -v ./internal/config"
      - "go test -run '^TestLoad_EvalsPartialBlockAndNoModelRejection$' -v ./internal/config"
      - "go test -run '^TestLoadEvalsAndPricing_MissingFileReturnsDefaults$' -v ./internal/config"
      - "go test -run '^TestLoadEvalsAndPricing_AbsentKeyReturnsDefaults$' -v ./internal/config"
      - "go test -count=1 ./internal/config"
      - "go test -cover ./internal/config | grep -E 'coverage: (79\\.[0-9]|[89][0-9]\\.[0-9]|100\\.0)%'"
      - "go vet ./internal/config"
      - "test -z \"$(gofmt -l internal/config)\""

  - id: "prune-pricing-tests"
    agent: "builder"
    description: "In internal/config/pricing_test.go delete TestLoadPricing_MissingFileReturnsDefaults, TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo, TestLoadPricing_NullKeyReturnsDefaults (their rows now live in the evals_test.go tables), TestLoad_NoPricingKeyUsesDefaults (now the pricing_defaults subtest of TestLoad_AllDefaultValues) and TestLoad_PresentPricingKeyReplacesDefaults. Convert TestLoadPricing_PresentKeyReplacesDefaults to a two-row table with subtests one_entry (existing assertion unchanged) and empty_mapping (yaml 'pricing: {}', err==nil, len(got)==0), and delete TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable. In internal/config/pricing_files_test.go delete TestShippedConfigFiles_PricingBlockIsDocumented and its doc comment. Fix imports in both files; leave every other test unchanged."
    dependencies: ["prune-evals-tests"]
    acceptance_criteria:
      - "TestLoadPricing_MissingFileReturnsDefaults, TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo, TestLoadPricing_NullKeyReturnsDefaults, TestLoad_NoPricingKeyUsesDefaults, TestLoad_PresentPricingKeyReplacesDefaults, TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable and TestShippedConfigFiles_PricingBlockIsDocumented no longer exist as functions"
      - "TestLoadPricing_PresentKeyReplacesDefaults passes with subtests one_entry and empty_mapping"
      - "TestShippedConfigFiles_PricingBlockPricesEverySonnetModel and TestShippedConfigFiles_Loadable are unchanged and pass"
      - "go test -cover ./internal/config reports at least 79.0%; go vet and gofmt are clean"
    validation_commands:
      - "! grep -nE 'func (TestLoadPricing_MissingFileReturnsDefaults|TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo|TestLoadPricing_NullKeyReturnsDefaults|TestLoad_NoPricingKeyUsesDefaults|TestLoad_PresentPricingKeyReplacesDefaults|TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable|TestShippedConfigFiles_PricingBlockIsDocumented)\\(' internal/config/*_test.go"
      - "go test -run '^TestLoadPricing_PresentKeyReplacesDefaults$' -v ./internal/config"
      - "go test -run '^TestShippedConfigFiles_' -v ./internal/config"
      - "go test -count=1 ./internal/config"
      - "go test -cover ./internal/config | grep -E 'coverage: (79\\.[0-9]|[89][0-9]\\.[0-9]|100\\.0)%'"
      - "go vet ./internal/config"
      - "test -z \"$(gofmt -l internal/config)\""

  - id: "prune-sandbox-tests"
    agent: "builder"
    description: "In internal/config/sandbox_test.go add a row named 'no sandbox key uses defaults' to TestLoad_SandboxConfigParsing (configContent 'repo: test/repo', expected SandboxConfig {/workspace, 1.0, 1024, 5m}, same as the existing empty-sandbox row). Delete TestLoad_SandboxConfigDefaults and TestLoad_DurationParsing. Keep TestLoad_SandboxConfigValidation, TestValidateSandboxWorkspaceDir and containsError. Fix imports."
    dependencies: ["prune-pricing-tests"]
    acceptance_criteria:
      - "TestLoad_SandboxConfigDefaults and TestLoad_DurationParsing no longer exist as functions"
      - "go test -run '^TestLoad_SandboxConfigParsing$' -v lists subtest no_sandbox_key_uses_defaults and it passes"
      - "The existing rows of TestLoad_SandboxConfigParsing (including timeout: 10m) are unchanged"
      - "go test -cover ./internal/config reports at least 79.0%; go vet and gofmt are clean"
    validation_commands:
      - "! grep -nE 'func (TestLoad_SandboxConfigDefaults|TestLoad_DurationParsing)\\(' internal/config/*_test.go"
      - "go test -run '^TestLoad_SandboxConfigParsing$' -v ./internal/config | grep -- 'no_sandbox_key_uses_defaults'"
      - "go test -count=1 ./internal/config"
      - "go test -cover ./internal/config | grep -E 'coverage: (79\\.[0-9]|[89][0-9]\\.[0-9]|100\\.0)%'"
      - "go vet ./internal/config"
      - "test -z \"$(gofmt -l internal/config)\""

  - id: "prune-themes-tests"
    agent: "builder"
    description: "In internal/config/themes_test.go add \"nonexistent-theme\" to the invalidNames list in TestLoadThemeInvalidName with a one-line comment that it is a well-formed name that fails on the theme-not-found read, not on the name pattern. Delete TestLoadThemeReturnsErrorForMissingTheme. The loop's assertions (non-nil error, nil theme) stay unchanged."
    dependencies: ["prune-sandbox-tests"]
    acceptance_criteria:
      - "TestLoadThemeReturnsErrorForMissingTheme no longer exists as a function"
      - "go test -run '^TestLoadThemeInvalidName$' -v lists subtest nonexistent-theme and it passes"
      - "go test -cover ./internal/config reports at least 79.0%; go vet and gofmt are clean"
    validation_commands:
      - "! grep -nE 'func TestLoadThemeReturnsErrorForMissingTheme\\(' internal/config/*_test.go"
      - "go test -run '^TestLoadThemeInvalidName$' -v ./internal/config | grep -- 'nonexistent-theme'"
      - "go test -count=1 ./internal/config"
      - "go test -cover ./internal/config | grep -E 'coverage: (79\\.[0-9]|[89][0-9]\\.[0-9]|100\\.0)%'"
      - "go vet ./internal/config"
      - "test -z \"$(gofmt -l internal/config)\""

  - id: "validate-prune"
    agent: "validator"
    description: "Read-only check of every acceptance criterion against the baseline from baseline-record: AC1 to AC3 greps print nothing; each partner passes with its merged case listed; each of the three table tests lists one top-level subtest per former test; go test -cover ./internal/config is at least 79.0%; task test exits 0; no production code changed; every test not listed in the issue still exists and passes (compare go test -list against the baseline: only the 22 target names may be missing, and only the three table test names may be new); every merged partner kept its original assertions; no spec deviation or deferred work is needed (PR text says 'Deferred work: none' and 'Spec deviations: none')."
    dependencies: ["prune-themes-tests"]
    acceptance_criteria:
      - "AC1: the grep for the 4 Delete tests prints nothing"
      - "AC2: the grep for the 10 Merge tests prints nothing, and all six partners pass with the merged case among their subtests"
      - "AC3: the grep for the 8 Combine tests prints nothing, and the three table tests list one subtest per former test (EnableCopilotReviewDefaults, UserField, BaseBranch; LoadEvals, LoadPricing; LoadEvals_NoEvalsKey, LoadPricing_NoPricingKeyWithoutRepo, LoadPricing_NullKey)"
      - "AC4: go test -cover ./internal/config reports at least 79.0%"
      - "AC5: task test exits 0"
      - "git diff against the base shows changes only in the six internal/config test files (plus pipeline artifacts), and no production file"
      - "The set difference of go test -list names versus the baseline is exactly the 22 removed names and the 3 new table test names"
    validation_commands:
      - "! grep -nE 'func (TestDefaultEvalsConfig|TestLoad_DurationParsing|TestLoad_PresentPricingKeyReplacesDefaults|TestShippedConfigFiles_PricingBlockIsDocumented)\\(' internal/config/*_test.go"
      - "! grep -nE 'func (TestLoadEvals_AllowedModelsReplacesDefault|TestLoadEvals_ExplicitEmptyAllowedModels|TestLoadEvals_ExplicitEmptyJudgeModelStaysEmpty|TestLoadEvals_TrustToolsDefaultsNil|TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable|TestLoadThemeReturnsErrorForMissingTheme|TestLoad_EvalsAllowedModelsReplacesDefault|TestLoad_ExposesEvalsDefaults|TestLoad_NoPricingKeyUsesDefaults|TestLoad_SandboxConfigDefaults)\\(' internal/config/*_test.go"
      - "! grep -nE 'func (TestLoadEvals_MissingFileReturnsDefaults|TestLoadEvals_NoEvalsKeyReturnsDefaults|TestLoadPricing_MissingFileReturnsDefaults|TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo|TestLoadPricing_NullKeyReturnsDefaults|TestLoad_BaseBranch|TestLoad_EnableCopilotReviewDefaults|TestLoad_UserField)\\(' internal/config/*_test.go"
      - "go test -run '^TestLoad_ScalarFieldDecoding$' -v ./internal/config"
      - "go test -run '^TestLoadEvalsAndPricing_MissingFileReturnsDefaults$' -v ./internal/config"
      - "go test -run '^TestLoadEvalsAndPricing_AbsentKeyReturnsDefaults$' -v ./internal/config"
      - "go test -cover ./internal/config | grep -E 'coverage: (79\\.[0-9]|[89][0-9]\\.[0-9]|100\\.0)%'"
      - "go vet ./internal/config"
      - "test -z \"$(gofmt -l internal/config)\""
      - "task test"
```
