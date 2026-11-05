# Design Spec: Delete tests that pin prompt and skill text

Closes #419

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/planner`: `automated_agent_test.go`, `interactive_agent_test.go`, `skill_test.go`, `syncgate_test.go`" | `prune-automated-agent-tests`, `prune-interactive-agent-tests`, `prune-skill-tests`, `prune-syncgate-tests`, `validate-all` |
| S2 | In Scope: "`cmd/kairon/cmd`: `TestAuditRealBuilderListsSkillsAndHooks`, `TestAuditRealPlannerHasRTCCHeadings`" | `prune-cmd-audit-tests`, `validate-all` |
| S3 | In Scope: "Test helpers left unused by the deletions" | `prune-automated-agent-tests`, `prune-interactive-agent-tests`, `prune-skill-tests`, `prune-syncgate-tests`, `prune-cmd-audit-tests`, `validate-all` |
| AC1 | "None of the 37 tests exists." | `prune-automated-agent-tests`, `prune-interactive-agent-tests`, `prune-skill-tests`, `prune-syncgate-tests`, `prune-cmd-audit-tests`, `validate-all` |
| AC2 | "After the deletions, the only test in `internal/planner` or `cmd/kairon/cmd` that reads a live agent prompt or skill file is `TestEvalDriverOverlayAndResults`." | `prune-automated-agent-tests`, `prune-interactive-agent-tests`, `prune-skill-tests`, `prune-syncgate-tests`, `prune-cmd-audit-tests`, `validate-all` |
| AC3 | "Template sync is still enforced, and templates still cannot ship `planner-conventions`." | `prune-skill-tests`, `prune-syncgate-tests`, `validate-all` |
| AC4 | "The gate passes." | `validate-all` |

## Deferred work

none

## Problem

37 tests in `internal/planner` and `cmd/kairon/cmd` read agent prompts or skills and assert on their wording, headings or embedded examples, or byte-compare live and template copies. The maintainer rule (2026-10-28) is that prompts and skills are tested through the output they produce (eval cases, #418), never through their text. The template comparisons repeat `task sync:check`, which CI runs (`.github/workflows/ci.yml`, "Template sync check" step).

This is a deletion-only change. No production code changes, no prompt or skill edits, no edits to `.kairon/research/` or other docs. Every test that is not on the delete list passes unchanged.

## Verification of the issue against the code

Checked against base commit `3ed716e` (HEAD of this worktree):

- **Names and line numbers all match.** All 37 test names exist in the files the issue names, at the line numbers the issue gives (cmd 2, `automated_agent_test.go` 7, `interactive_agent_test.go` 10, `skill_test.go` 14, `syncgate_test.go` 4). No drift.
- **The In Scope list names four whole files, but the delete list is per test.** The delete list governs. Each of the four files keeps tests that are not on it (they are "other planner tests", #436, or are manifest/Taskfile tests that pin no prompt or skill text). They stay, unchanged:
  - `automated_agent_test.go`: `TestAutomatedManifest`, `TestAutomatedManifestHasNoInjectedOrWriteCapabilities`, `TestAutomatedManifestShellIsReadOnly`, `TestPlannerManifestsListSameSharedSkill`
  - `interactive_agent_test.go`: `TestInteractiveManifest`, `TestInteractiveManifestAllowsSessionDocument`, `TestInteractiveTemplateManifestAllowedPathsMatchLive`, `TestInteractiveManifestHasNoInjectedCapabilities`
  - `skill_test.go`: `TestSkillConstants`, `TestTemplatesContainNoPlannerConventionsPath` (keeper)
  - `syncgate_test.go`: `TestSyncGate_TaskfileDiffsSharedSkill`, `TestSyncGate_TaskfileExcludesPlannerConventions`
- `TestInteractiveTemplateManifestAllowedPathsMatchLive` is marked DELETE in `.kairon/research/test-audit-verdicts.md` but is not on the issue's list. The issue wins; it stays. It compares manifest `allowedPaths`, not prompt or skill text.
- `.kairon/research/test-audit-verdicts.md` marks several of the 37 as KEEP or MERGE (for example `TestSkillFrontMatter`, `TestAutomatedPromptRequiresPunchOut`). The issue's Context cites the later maintainer rule and the issue's Current/Target State says all 37 are deleted with no partner. The issue wins. `.kairon/research/` is not edited.
- **AC2's grep matches constants, not only test bodies.** It matches any quoted `".kiro/agents/<x>.md"` or `".kiro/skills/<x>/SKILL.md"` string literal, so the three path constants `automatedPromptPath` (`automated_agent_test.go:13`), `interactivePromptPath` (`interactive_agent_test.go:13`) and `liveSkillPath` (`skill_test.go:15`) must be removed even though a compiler would not complain. The only remaining match must be `internal/planner/evaldriver_test.go:231` (the keeper `TestEvalDriverOverlayAndResults`, which uses a string literal and none of the constants).
- **The worktree is not clean, and that breaks the baseline.** Uncommitted edits to `.kiro/agents/*.json` (the pipeline injects `@creds-agent` and an `mcpServers` block into the live manifests) make `task sync:check` fail and make five planner tests fail before any change: `TestAutomatedManifest`, `TestAutomatedManifestHasNoInjectedOrWriteCapabilities`, `TestInteractiveManifest`, `TestInteractiveManifestHasNoInjectedCapabilities`, `TestEvalDriverDryRun` (plus `TestAutomatedFilesMatchTemplates` and `TestInteractiveFilesMatchTemplates`, which this issue deletes). On a clean clone of `3ed716e`, `task sync:check` exits 0 and `go test ./internal/planner ./cmd/kairon/cmd` passes. AC3 and AC4 are therefore verified in a clean checkout (the "clean-room check" below), not in this worktree. Nobody may edit, revert or stage `.kiro/agents/*.json` to make the worktree pass.

## Helper and import analysis

`go build` and `go vet` do not flag unused functions, constants or package-level variables in test files, so leftovers compile silently. Every one below was checked with `grep -w` across `internal/planner/*.go` and `cmd/kairon/cmd/*.go`. A helper is removed only when all its callers are deleted tests.

### Keepers do not use anything that is removed

- `TestEvalDriverOverlayAndResults` (`evaldriver_test.go`) uses `runDriver`, `repoRoot`, `readRepoFile` and its own literals. `repoRoot` and `readRepoFile` (both in `helpers_test.go`) have other callers and stay. It uses none of the removed constants or helpers.
- `TestTemplatesContainNoPlannerConventionsPath` (`skill_test.go`) uses `repoRoot` plus `io/fs`, `path/filepath`, `strings`. Those three imports must stay in `skill_test.go`.
- `TestSelfTestWorkspaceCasesNative` is in `internal/eval/sandbox_workspace_test.go` and is not touched by anything here.

### `internal/planner`

| File | Remove (tests listed in the issue) | Remove (helpers, consts, imports left unused) | Keep |
|------|------------------------------------|-----------------------------------------------|------|
| `automated_agent_test.go` | 7 tests: `TestAutomatedPromptHasRTCCHeadings`, `TestAutomatedPromptNeverAsksOffersOrDrafts`, `TestAutomatedPromptRequiresInferenceEvidence`, `TestAutomatedPromptRequiresPunchOut`, `TestAutomatedPromptHasNoInteractiveOrModeText`, `TestAutomatedPromptDoesNotRestateSharedContract`, `TestAutomatedFilesMatchTemplates` | func `readAutomatedPrompt`; consts `automatedPromptPath`, `automatedTmplManifest`, `automatedTmplPrompt`; imports `bytes`, `regexp`; the comments that sit directly above removed tests | consts `automatedManifestPath`, `automatedPromptFileURI`, `interactiveManifest` (used by `interactive_agent_test.go`), `conventionsSkillURI`, `sharedSkillURI`; func `stringList` (also used by `interactive_agent_test.go`); imports `reflect`, `strings`, `testing` |
| `interactive_agent_test.go` | 10 tests: `TestInteractiveTemplatePromptMatchesLive`, `TestInteractivePromptWritesDocumentToGivenPath`, `TestInteractivePromptStatesSingleWriteException`, `TestInteractivePromptRecordsLabelAnswer`, `TestInteractivePromptHasRTCCHeadings`, `TestInteractivePromptEncodesElicitationFlow`, `TestInteractivePromptNeverPunchesOut`, `TestInteractivePromptDoesNotPublishOrHash`, `TestInteractivePromptDoesNotRestateSharedContractOrSelectMode`, `TestInteractiveFilesMatchTemplates` | funcs `readInteractivePrompt`, `promptSection`; consts `interactivePromptPath`, `interactiveTmplPrompt`; imports `bytes`, `regexp`; comments above removed tests | consts `interactiveTmplManifest` (used by `TestInteractiveTemplateManifestAllowedPathsMatchLive`), `interactivePromptFileURI`, `interactiveManifestModel`, `interactiveWriteScopePath`, `interactiveSessionDocumentPath`, `interactiveSessionLabelPath` (with their comments); func `writeAllowedPaths`; imports `reflect`, `slices`, `strings`, `testing` |
| `skill_test.go` | 14 tests: `TestSkillFrontMatter`, `TestSkillSectionsInOrder`, `TestSkillContractFields`, `TestSkillExamplesParseOnePerVariant`, `TestSkillCanonicalExampleFollowsOwnStandard`, `TestSkillInvestigateClassifyCover`, `TestSkillIssueBodyStandard`, `TestSkillDependenciesRule`, `TestSkillRestrictions`, `TestSkillHardBars`, `TestSkillHasNoModeSpecificText`, `TestSkillPointsToPromptForVariantPermission`, `TestTemplateSkillIsByteIdentical`, `TestTemplateSkillDirHasOnlySkillFile` | the whole `const (liveSkillPath, templateSkillPath)` block; func `readSkill`; var `yamlFence`; func `sectionBetween` (and its comment); imports `bytes`, `regexp`, `github.com/jbrinkman/kairon/internal/stageoutput`; comments above removed tests | `TestSkillConstants`; `TestTemplatesContainNoPlannerConventionsPath` and its comment; imports `io/fs`, `path/filepath`, `strings`, `testing` |
| `syncgate_test.go` | 4 tests: `TestSyncGate_TemplatesHaveNoPlannerConventionsSkill`, `TestSyncGate_BuilderConventionsListsSharedSkill`, `TestSyncGate_SharedSkillTemplateInSync`, `TestSyncGate_DriftDetected` (and the comment above it) | func `dirsMatch`; imports `io/fs`, `os`, `path/filepath` | const `sharedSkillDiff`; func `syncCheckBody`; `TestSyncGate_TaskfileDiffsSharedSkill`; `TestSyncGate_TaskfileExcludesPlannerConventions`; imports `strings`, `testing` |

Other `internal/planner` files (`helpers_test.go`, `evaldriver_test.go`, `run_test.go`, `boundary_test.go`, ...) need no edit. `repoRoot`, `readRepoFile` and `readManifest` keep callers after the deletions (`evaldriver_test.go`, `boundary_test.go`, `extract_test.go`, the kept manifest tests).

### `cmd/kairon/cmd/eval_audit_test.go`

Remove `TestAuditRealBuilderListsSkillsAndHooks`, `TestAuditRealPlannerHasRTCCHeadings`, the comment block above the first ("The real-repo tests run from the repository root ...") and the comment above the second. After that, `useShippedSkills` (and its comment) has no caller: remove it. That leaves the imports `io/fs`, `os` and `path/filepath` unused: remove them. Keep `bytes`, `strings`, `testing`, const `auditFixtures`, and `runAudit`, `runAuditFixture`, `runAuditFlatFixture` and all other tests in the file.

`auditSkillsFS` in `eval_audit.go` is production code. No test overrides it after this change, but it is still read by production code, so it stays. Do not touch it (no production code changes).

## Approach

Pure test-code deletion, one task per file so the work parallelizes. The edits are independent: no removed symbol is referenced by a kept test in another file, so each file compiles on its own after its task. Run `gofmt -w` on every touched file (the const blocks need realigning; `task fmt:check` is part of CI).

TDD rules in `AGENTS.md`: nothing here adds or changes behaviour, so there is no new failing test to write. The refactor rule (tests must pass before and after) is met by the before/after test-list comparison in `validate-all`: the set of remaining tests must equal the set at `3ed716e` minus exactly the 37 names, and every remaining test must pass in a clean checkout. The PR description must say which tests were deleted and `Deferred work: none` and `Spec deviations: none`.

Do not stage or commit `.kiro/agents/*.json`; they carry pipeline-injected changes unrelated to this issue.

## Team Orchestration

- `prune-automated-agent-tests`, `prune-interactive-agent-tests`, `prune-skill-tests`, `prune-syncgate-tests` and `prune-cmd-audit-tests` have no dependencies and run in parallel. They touch five different files. They do not commit.
- `validate-all` (validator) runs after all five.

## Clean-room check

The worktree's injected `.kiro/agents/*.json` edits make `task sync:check` and several manifest tests fail regardless of this change. To get a true result, verify in a clone of the base commit with only this change applied:

```bash
CLEAN=$(mktemp -d) && git clone -q --no-hardlinks . "$CLEAN" && git -C "$CLEAN" checkout -q 3ed716e \
  && git diff 3ed716e -- internal cmd | git -C "$CLEAN" apply \
  && (cd "$CLEAN" && task sync:check && task fmt:check && task lint && task test)
```

`git diff 3ed716e -- internal cmd` covers committed and uncommitted edits and excludes `.kiro/`, so the injection is left out.

## Validation Commands

```bash
# AC1: none of the 37 tests exists (each prints nothing; ! makes the exit code 0)
! grep -nE 'func (TestAuditRealBuilderListsSkillsAndHooks|TestAuditRealPlannerHasRTCCHeadings)\(' cmd/kairon/cmd/*_test.go
! grep -nE 'func (TestAutomatedFilesMatchTemplates|TestAutomatedPromptDoesNotRestateSharedContract|TestAutomatedPromptHasNoInteractiveOrModeText|TestAutomatedPromptHasRTCCHeadings|TestAutomatedPromptNeverAsksOffersOrDrafts|TestAutomatedPromptRequiresInferenceEvidence|TestAutomatedPromptRequiresPunchOut|TestInteractiveFilesMatchTemplates|TestInteractivePromptDoesNotPublishOrHash|TestInteractivePromptDoesNotRestateSharedContractOrSelectMode|TestInteractivePromptEncodesElicitationFlow|TestInteractivePromptHasRTCCHeadings|TestInteractivePromptNeverPunchesOut|TestInteractivePromptRecordsLabelAnswer|TestInteractivePromptStatesSingleWriteException|TestInteractivePromptWritesDocumentToGivenPath|TestInteractiveTemplatePromptMatchesLive|TestSkillCanonicalExampleFollowsOwnStandard|TestSkillContractFields|TestSkillDependenciesRule|TestSkillExamplesParseOnePerVariant|TestSkillFrontMatter|TestSkillHardBars|TestSkillHasNoModeSpecificText|TestSkillInvestigateClassifyCover|TestSkillIssueBodyStandard|TestSkillPointsToPromptForVariantPermission|TestSkillRestrictions|TestSkillSectionsInOrder|TestTemplateSkillDirHasOnlySkillFile|TestTemplateSkillIsByteIdentical|TestSyncGate_BuilderConventionsListsSharedSkill|TestSyncGate_DriftDetected|TestSyncGate_SharedSkillTemplateInSync|TestSyncGate_TemplatesHaveNoPlannerConventionsSkill)\(' internal/planner/*_test.go

# AC2: only evaldriver_test.go reads a live prompt or skill path
test -z "$(grep -nE '"\.kiro/(agents/[a-z-]+\.md|skills/[a-z-]+/SKILL\.md)"' internal/planner/*_test.go cmd/kairon/cmd/*_test.go | grep -v '^internal/planner/evaldriver_test.go:')"
grep -nE '"\.kiro/agents/planner-automated-prompt\.md"' internal/planner/evaldriver_test.go

# Orphans: no removed helper, const or import is left behind
! grep -nE '\b(readAutomatedPrompt|readInteractivePrompt|promptSection|readSkill|sectionBetween|yamlFence|dirsMatch|useShippedSkills|liveSkillPath|templateSkillPath|automatedPromptPath|automatedTmplManifest|automatedTmplPrompt|interactivePromptPath|interactiveTmplPrompt)\b' internal/planner/*_test.go cmd/kairon/cmd/*_test.go
gofmt -l internal cmd          # prints nothing
go vet ./internal/planner ./cmd/kairon/cmd

# Exactly 37 tests gone, none added (test-name sets, base commit vs now)
CLEAN=$(mktemp -d) && git clone -q --no-hardlinks . "$CLEAN" && git -C "$CLEAN" checkout -q 3ed716e \
  && (cd "$CLEAN" && go test -list '.*' ./internal/planner ./cmd/kairon/cmd | grep '^Test' | sort) > /tmp/419-before.txt \
  && go test -list '.*' ./internal/planner ./cmd/kairon/cmd | grep '^Test' | sort > /tmp/419-after.txt \
  && test "$(comm -23 /tmp/419-before.txt /tmp/419-after.txt | wc -l | tr -d ' ')" = 37 \
  && test "$(comm -13 /tmp/419-before.txt /tmp/419-after.txt | wc -l | tr -d ' ')" = 0

# AC3 and AC4: run the clean-room check above (sync:check, fmt:check, lint, test)
# The keeper that guards the packaging rule:
go test -run '^TestTemplatesContainNoPlannerConventionsPath$' ./internal/planner
```

## Relevant Files

Modified (test code only):

- `internal/planner/automated_agent_test.go`
- `internal/planner/interactive_agent_test.go`
- `internal/planner/skill_test.go`
- `internal/planner/syncgate_test.go`
- `cmd/kairon/cmd/eval_audit_test.go`

Read-only references (do not edit): `internal/planner/evaldriver_test.go`, `internal/planner/helpers_test.go`, `cmd/kairon/cmd/eval_audit.go`, `Taskfile.yml`, `.github/workflows/ci.yml`, `.kairon/research/test-audit-verdicts.md`, `.kairon/research/test-performance-analysis.md`.

## Step-by-Step Task Breakdown

1. **prune-automated-agent-tests**: edit `automated_agent_test.go` per the table. Run `gofmt -w`.
2. **prune-interactive-agent-tests**: edit `interactive_agent_test.go` per the table. Run `gofmt -w`.
3. **prune-skill-tests**: reduce `skill_test.go` to `TestSkillConstants` and `TestTemplatesContainNoPlannerConventionsPath`. Run `gofmt -w`.
4. **prune-syncgate-tests**: reduce `syncgate_test.go` to `sharedSkillDiff`, `syncCheckBody` and the two `TestSyncGate_Taskfile*` tests. Run `gofmt -w`.
5. **prune-cmd-audit-tests**: delete the two real-repo audit tests, `useShippedSkills` and the three imports in `eval_audit_test.go`. Run `gofmt -w`.
6. **validate-all**: read-only verification of AC1 to AC4 and the orphan and test-set checks.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "prune-automated-agent-tests"
    agent: "builder"
    description: "In internal/planner/automated_agent_test.go delete the 7 prompt-text and template-copy tests (TestAutomatedPromptHasRTCCHeadings, TestAutomatedPromptNeverAsksOffersOrDrafts, TestAutomatedPromptRequiresInferenceEvidence, TestAutomatedPromptRequiresPunchOut, TestAutomatedPromptHasNoInteractiveOrModeText, TestAutomatedPromptDoesNotRestateSharedContract, TestAutomatedFilesMatchTemplates) with the comments directly above them. Then remove what they leave unused: func readAutomatedPrompt, consts automatedPromptPath, automatedTmplManifest, automatedTmplPrompt, and imports bytes and regexp. Keep consts automatedManifestPath, automatedPromptFileURI, interactiveManifest, conventionsSkillURI, sharedSkillURI, func stringList, and the four kept tests (TestAutomatedManifest, TestAutomatedManifestHasNoInjectedOrWriteCapabilities, TestAutomatedManifestShellIsReadOnly, TestPlannerManifestsListSameSharedSkill). Run gofmt -w. Do not commit. Do not touch .kiro/agents/*.json."
    dependencies: []
    acceptance_criteria:
      - "None of the 7 listed automated tests is defined in internal/planner/automated_agent_test.go"
      - "readAutomatedPrompt, automatedPromptPath, automatedTmplManifest and automatedTmplPrompt no longer appear in internal/planner/*_test.go"
      - "The file imports exactly reflect, strings and testing and gofmt -l prints nothing for it"
      - "The four kept tests and stringList are unchanged"
    validation_commands:
      - "! grep -nE 'func (TestAutomatedFilesMatchTemplates|TestAutomatedPromptDoesNotRestateSharedContract|TestAutomatedPromptHasNoInteractiveOrModeText|TestAutomatedPromptHasRTCCHeadings|TestAutomatedPromptNeverAsksOffersOrDrafts|TestAutomatedPromptRequiresInferenceEvidence|TestAutomatedPromptRequiresPunchOut)\\(' internal/planner/automated_agent_test.go"
      - "! grep -nE '\\b(readAutomatedPrompt|automatedPromptPath|automatedTmplManifest|automatedTmplPrompt)\\b' internal/planner/*_test.go"
      - "test -z \"$(gofmt -l internal/planner/automated_agent_test.go)\""
      - "go vet ./internal/planner"

  - id: "prune-interactive-agent-tests"
    agent: "builder"
    description: "In internal/planner/interactive_agent_test.go delete the 10 prompt-text and template-copy tests (TestInteractiveTemplatePromptMatchesLive, TestInteractivePromptWritesDocumentToGivenPath, TestInteractivePromptStatesSingleWriteException, TestInteractivePromptRecordsLabelAnswer, TestInteractivePromptHasRTCCHeadings, TestInteractivePromptEncodesElicitationFlow, TestInteractivePromptNeverPunchesOut, TestInteractivePromptDoesNotPublishOrHash, TestInteractivePromptDoesNotRestateSharedContractOrSelectMode, TestInteractiveFilesMatchTemplates) with the comments directly above them. Then remove what they leave unused: funcs readInteractivePrompt and promptSection, consts interactivePromptPath and interactiveTmplPrompt, and imports bytes and regexp. Keep interactiveTmplManifest (used by TestInteractiveTemplateManifestAllowedPathsMatchLive), interactivePromptFileURI, interactiveManifestModel, interactiveWriteScopePath, interactiveSessionDocumentPath, interactiveSessionLabelPath, func writeAllowedPaths, and the four kept tests (TestInteractiveManifest, TestInteractiveManifestAllowsSessionDocument, TestInteractiveTemplateManifestAllowedPathsMatchLive, TestInteractiveManifestHasNoInjectedCapabilities). Run gofmt -w. Do not commit. Do not touch .kiro/agents/*.json."
    dependencies: []
    acceptance_criteria:
      - "None of the 10 listed interactive tests is defined in internal/planner/interactive_agent_test.go"
      - "readInteractivePrompt, promptSection, interactivePromptPath and interactiveTmplPrompt no longer appear in internal/planner/*_test.go"
      - "The file imports exactly reflect, slices, strings and testing and gofmt -l prints nothing for it"
      - "The four kept tests and writeAllowedPaths are unchanged"
    validation_commands:
      - "! grep -nE 'func (TestInteractiveFilesMatchTemplates|TestInteractivePromptDoesNotPublishOrHash|TestInteractivePromptDoesNotRestateSharedContractOrSelectMode|TestInteractivePromptEncodesElicitationFlow|TestInteractivePromptHasRTCCHeadings|TestInteractivePromptNeverPunchesOut|TestInteractivePromptRecordsLabelAnswer|TestInteractivePromptStatesSingleWriteException|TestInteractivePromptWritesDocumentToGivenPath|TestInteractiveTemplatePromptMatchesLive)\\(' internal/planner/interactive_agent_test.go"
      - "! grep -nE '\\b(readInteractivePrompt|promptSection|interactivePromptPath|interactiveTmplPrompt)\\b' internal/planner/*_test.go"
      - "test -z \"$(gofmt -l internal/planner/interactive_agent_test.go)\""
      - "go vet ./internal/planner"

  - id: "prune-skill-tests"
    agent: "builder"
    description: "Reduce internal/planner/skill_test.go to TestSkillConstants and TestTemplatesContainNoPlannerConventionsPath (with its comment). Delete the 14 skill-text and template-copy tests (TestSkillFrontMatter, TestSkillSectionsInOrder, TestSkillContractFields, TestSkillExamplesParseOnePerVariant, TestSkillCanonicalExampleFollowsOwnStandard, TestSkillInvestigateClassifyCover, TestSkillIssueBodyStandard, TestSkillDependenciesRule, TestSkillRestrictions, TestSkillHardBars, TestSkillHasNoModeSpecificText, TestSkillPointsToPromptForVariantPermission, TestTemplateSkillIsByteIdentical, TestTemplateSkillDirHasOnlySkillFile) with the comments directly above them. Then remove what they leave unused: the const block (liveSkillPath, templateSkillPath), func readSkill, var yamlFence, func sectionBetween with its comment, and imports bytes, regexp and github.com/jbrinkman/kairon/internal/stageoutput. Keep imports io/fs, path/filepath, strings and testing, which the keeper TestTemplatesContainNoPlannerConventionsPath needs. Run gofmt -w. Do not commit."
    dependencies: []
    acceptance_criteria:
      - "None of the 14 listed skill tests is defined in internal/planner/skill_test.go"
      - "TestSkillConstants and TestTemplatesContainNoPlannerConventionsPath remain, unchanged"
      - "liveSkillPath, templateSkillPath, readSkill, yamlFence and sectionBetween no longer appear in internal/planner/*_test.go"
      - "The file imports exactly io/fs, path/filepath, strings and testing and gofmt -l prints nothing for it"
      - "go test -run '^TestTemplatesContainNoPlannerConventionsPath$' ./internal/planner exits 0"
    validation_commands:
      - "! grep -nE 'func (TestSkillCanonicalExampleFollowsOwnStandard|TestSkillContractFields|TestSkillDependenciesRule|TestSkillExamplesParseOnePerVariant|TestSkillFrontMatter|TestSkillHardBars|TestSkillHasNoModeSpecificText|TestSkillInvestigateClassifyCover|TestSkillIssueBodyStandard|TestSkillPointsToPromptForVariantPermission|TestSkillRestrictions|TestSkillSectionsInOrder|TestTemplateSkillDirHasOnlySkillFile|TestTemplateSkillIsByteIdentical)\\(' internal/planner/skill_test.go"
      - "! grep -nE '\\b(liveSkillPath|templateSkillPath|readSkill|yamlFence|sectionBetween)\\b' internal/planner/*_test.go"
      - "test -z \"$(gofmt -l internal/planner/skill_test.go)\""
      - "go vet ./internal/planner"
      - "go test -run '^(TestSkillConstants|TestTemplatesContainNoPlannerConventionsPath)$' ./internal/planner"

  - id: "prune-syncgate-tests"
    agent: "builder"
    description: "In internal/planner/syncgate_test.go delete TestSyncGate_TemplatesHaveNoPlannerConventionsSkill, TestSyncGate_BuilderConventionsListsSharedSkill, TestSyncGate_SharedSkillTemplateInSync and TestSyncGate_DriftDetected (with the comment above it), then remove func dirsMatch and the imports io/fs, os and path/filepath. Keep const sharedSkillDiff, func syncCheckBody, TestSyncGate_TaskfileDiffsSharedSkill, TestSyncGate_TaskfileExcludesPlannerConventions, and imports strings and testing. Run gofmt -w. Do not commit."
    dependencies: []
    acceptance_criteria:
      - "None of the 4 listed TestSyncGate tests is defined in internal/planner/syncgate_test.go"
      - "TestSyncGate_TaskfileDiffsSharedSkill and TestSyncGate_TaskfileExcludesPlannerConventions remain, unchanged, and pass"
      - "dirsMatch no longer appears in internal/planner/*_test.go"
      - "The file imports exactly strings and testing and gofmt -l prints nothing for it"
    validation_commands:
      - "! grep -nE 'func (TestSyncGate_BuilderConventionsListsSharedSkill|TestSyncGate_DriftDetected|TestSyncGate_SharedSkillTemplateInSync|TestSyncGate_TemplatesHaveNoPlannerConventionsSkill)\\(' internal/planner/syncgate_test.go"
      - "! grep -nw 'dirsMatch' internal/planner/*_test.go"
      - "test -z \"$(gofmt -l internal/planner/syncgate_test.go)\""
      - "go vet ./internal/planner"
      - "go test -run '^TestSyncGate_' ./internal/planner"

  - id: "prune-cmd-audit-tests"
    agent: "builder"
    description: "In cmd/kairon/cmd/eval_audit_test.go delete TestAuditRealBuilderListsSkillsAndHooks and TestAuditRealPlannerHasRTCCHeadings together with the comment blocks above them. Then delete func useShippedSkills (and its comment), which only those two tests called, and the imports io/fs, os and path/filepath. Keep bytes, strings, testing, const auditFixtures, runAudit, runAuditFixture, runAuditFlatFixture and every other test. Do not edit cmd/kairon/cmd/eval_audit.go (auditSkillsFS stays: no production code changes). Run gofmt -w. Do not commit."
    dependencies: []
    acceptance_criteria:
      - "TestAuditRealBuilderListsSkillsAndHooks and TestAuditRealPlannerHasRTCCHeadings are not defined in cmd/kairon/cmd/*_test.go"
      - "useShippedSkills no longer appears in cmd/kairon/cmd/*_test.go"
      - "eval_audit_test.go imports exactly bytes, strings and testing and gofmt -l prints nothing for it"
      - "cmd/kairon/cmd/eval_audit.go is unchanged"
      - "go test ./cmd/kairon/cmd passes"
    validation_commands:
      - "! grep -nE 'func (TestAuditRealBuilderListsSkillsAndHooks|TestAuditRealPlannerHasRTCCHeadings)\\(' cmd/kairon/cmd/*_test.go"
      - "! grep -nw 'useShippedSkills' cmd/kairon/cmd/*_test.go"
      - "test -z \"$(gofmt -l cmd/kairon/cmd/eval_audit_test.go)\""
      - "git diff --quiet 3ed716e -- cmd/kairon/cmd/eval_audit.go"
      - "go vet ./cmd/kairon/cmd"
      - "go test ./cmd/kairon/cmd -run '^TestAudit'"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of all four acceptance criteria. Because the worktree carries pipeline-injected .kiro/agents/*.json edits that break task sync:check and five manifest tests regardless of this change, run AC3 and AC4 in the clean-room check from the spec (clone of 3ed716e plus git diff 3ed716e -- internal cmd applied). Also confirm only the five test files changed (git diff --name-only 3ed716e -- . ':!.kiro/agents' lists exactly internal/planner/{automated_agent,interactive_agent,skill,syncgate}_test.go and cmd/kairon/cmd/eval_audit_test.go, plus the spec and sentinel files), no production .go file changed, that the remaining test set equals the base test set minus exactly the 37 names, and that the PR description will carry 'Deferred work: none' and 'Spec deviations: none'."
    dependencies: ["prune-automated-agent-tests", "prune-interactive-agent-tests", "prune-skill-tests", "prune-syncgate-tests", "prune-cmd-audit-tests"]
    acceptance_criteria:
      - "AC1: both greps for the 37 test names print nothing"
      - "AC2: the grep for quoted .kiro/agents/*.md and .kiro/skills/*/SKILL.md paths prints only lines in internal/planner/evaldriver_test.go"
      - "AC3: in the clean-room check task sync:check exits 0, and go test -run '^TestTemplatesContainNoPlannerConventionsPath$' ./internal/planner exits 0"
      - "AC4: in the clean-room check task test exits 0 (task fmt:check and task lint also exit 0)"
      - "The test-name set diff against 3ed716e shows exactly 37 removed and 0 added"
      - "No orphaned helper, constant or import from the deletions remains"
      - "No non-test .go file, prompt, skill, template or doc was modified, and .kiro/agents/*.json was not staged or edited by this change"
    validation_commands:
      - "! grep -nE 'func (TestAuditRealBuilderListsSkillsAndHooks|TestAuditRealPlannerHasRTCCHeadings)\\(' cmd/kairon/cmd/*_test.go"
      - "! grep -nE 'func (TestAutomatedFilesMatchTemplates|TestAutomatedPromptDoesNotRestateSharedContract|TestAutomatedPromptHasNoInteractiveOrModeText|TestAutomatedPromptHasRTCCHeadings|TestAutomatedPromptNeverAsksOffersOrDrafts|TestAutomatedPromptRequiresInferenceEvidence|TestAutomatedPromptRequiresPunchOut|TestInteractiveFilesMatchTemplates|TestInteractivePromptDoesNotPublishOrHash|TestInteractivePromptDoesNotRestateSharedContractOrSelectMode|TestInteractivePromptEncodesElicitationFlow|TestInteractivePromptHasRTCCHeadings|TestInteractivePromptNeverPunchesOut|TestInteractivePromptRecordsLabelAnswer|TestInteractivePromptStatesSingleWriteException|TestInteractivePromptWritesDocumentToGivenPath|TestInteractiveTemplatePromptMatchesLive|TestSkillCanonicalExampleFollowsOwnStandard|TestSkillContractFields|TestSkillDependenciesRule|TestSkillExamplesParseOnePerVariant|TestSkillFrontMatter|TestSkillHardBars|TestSkillHasNoModeSpecificText|TestSkillInvestigateClassifyCover|TestSkillIssueBodyStandard|TestSkillPointsToPromptForVariantPermission|TestSkillRestrictions|TestSkillSectionsInOrder|TestTemplateSkillDirHasOnlySkillFile|TestTemplateSkillIsByteIdentical|TestSyncGate_BuilderConventionsListsSharedSkill|TestSyncGate_DriftDetected|TestSyncGate_SharedSkillTemplateInSync|TestSyncGate_TemplatesHaveNoPlannerConventionsSkill)\\(' internal/planner/*_test.go"
      - "test -z \"$(grep -nE '\"\\.kiro/(agents/[a-z-]+\\.md|skills/[a-z-]+/SKILL\\.md)\"' internal/planner/*_test.go cmd/kairon/cmd/*_test.go | grep -v '^internal/planner/evaldriver_test.go:')\""
      - "! grep -nE '\\b(readAutomatedPrompt|readInteractivePrompt|promptSection|readSkill|sectionBetween|yamlFence|dirsMatch|useShippedSkills|liveSkillPath|templateSkillPath|automatedPromptPath|automatedTmplManifest|automatedTmplPrompt|interactivePromptPath|interactiveTmplPrompt)\\b' internal/planner/*_test.go cmd/kairon/cmd/*_test.go"
      - "test -z \"$(gofmt -l internal cmd)\""
      - "CLEAN=$(mktemp -d) && git clone -q --no-hardlinks . \"$CLEAN\" && git -C \"$CLEAN\" checkout -q 3ed716e && (cd \"$CLEAN\" && go test -list '.*' ./internal/planner ./cmd/kairon/cmd | grep '^Test' | sort) > /tmp/419-before.txt && go test -list '.*' ./internal/planner ./cmd/kairon/cmd | grep '^Test' | sort > /tmp/419-after.txt && test \"$(comm -23 /tmp/419-before.txt /tmp/419-after.txt | wc -l | tr -d ' ')\" = 37 && test \"$(comm -13 /tmp/419-before.txt /tmp/419-after.txt | wc -l | tr -d ' ')\" = 0"
      - "CLEAN=$(mktemp -d) && git clone -q --no-hardlinks . \"$CLEAN\" && git -C \"$CLEAN\" checkout -q 3ed716e && git diff 3ed716e -- internal cmd | git -C \"$CLEAN\" apply && (cd \"$CLEAN\" && task sync:check && task fmt:check && task lint && go test -run '^TestTemplatesContainNoPlannerConventionsPath$' ./internal/planner && task test)"
```
