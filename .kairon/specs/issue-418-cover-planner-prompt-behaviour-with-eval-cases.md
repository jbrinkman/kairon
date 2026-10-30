# Design Spec: Cover planner prompt behaviour with eval cases

Closes #418

## Scope Coverage

Every In Scope item, Functional Requirement and acceptance criterion of issue #418 is listed. Task IDs refer to the `kiro-plan` block at the end of this spec.

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`.kairon/evals/cases/planner/`: new or extended cases" | `suite-tests-red`, `automated-cases`, `interactive-cases`, `legacy-transcripts`, `driver-script` |
| S2 | In Scope: "`cmd/kairon/templates/kairon/evals/cases/planner/`: template copies of any changed case" | `sync-templates` |
| FR1 | "The interactive planner writes its document to the path it is given, and records the user's label answer at the label-answer path." | `interactive-cases` |
| FR2 | "The automated planner returns a `punch_out` document instead of asking a question when a required slot cannot be filled." | `test-eval-mapping` (cites the existing `automated-punch-out` checks), `validate-all` (stub run proves they pass) |
| FR3 | "Every inference in an automated planner document carries evidence." | `automated-cases`, `prove-checks-can-fail` |
| FR4 | "A planner document's body ends with one single line that lists its dependencies, in the format `internal/guard`'s structural check accepts." | `automated-cases`, `interactive-cases`, `prove-checks-can-fail` |
| FR5 | "Each `TestInteractivePrompt*`, `TestAutomatedPrompt*` and `TestSkill*` test in `internal/planner` is mapped to the eval case that checks the same behaviour, or recorded as checking no behaviour. The mapping is in the PR description." | `test-eval-mapping` |
| AC1 | "Each of the four behaviours has a planner eval case with a check on the planner's output." | `automated-cases`, `interactive-cases`, `test-eval-mapping` |
| AC2 | "The new and changed cases load." | `automated-cases`, `interactive-cases`, `validate-all` |
| AC3 | "Live and template copies match." | `sync-templates`, `validate-all` |
| AC4 | "The mapping in Functional Requirement 5 covers every listed planner test." | `test-eval-mapping`, `validate-all` |

## Deferred work

none

## Problem

`internal/planner` has tests that read `.kiro/agents/planner-prompt.md`, `.kiro/agents/planner-automated-prompt.md` and `.kiro/skills/planner-shared/SKILL.md` and assert on their wording. #419 deletes them (37 tests in `internal/planner` and `cmd/kairon/cmd`). The maintainer rule of 2026-10-28 (`.kairon/research/test-performance-analysis.md` §8.6) says prompts and skills are tested through what they produce, by eval cases that run through `kairon eval`. Before the wording tests go, every behaviour they stand in for needs an eval case that inspects the planner's output, and every test needs a recorded disposition.

This issue adds and extends the eval cases under `.kairon/evals/cases/planner/`, copies them into the templates, and produces the test-to-case mapping that goes into the PR description. It changes no prompt, no skill and no production Go code.

## What the investigation found

### Eval framework facts the design relies on

- A case is a YAML file in `.kairon/evals/cases/<agent>/`. The directory name sets the agent (`planner`), and `rubrics/planner.yaml` supplies the criteria (`checks_only: true`, `pass_threshold: 90`, `min_criterion_score: 60`). The rubric is not in this issue's scope, so no new criterion is added. File hand-off checks feed `gate_compliance`.
- Check types (`internal/checks/checks.go`): `command`, `file_exists`, `file_absent`, `file_contains`, `file_not_contains`, `changed_files`, `output_contains`, `output_not_contains`, `output_document_valid`, `gh_log_contains`, `gh_log_not_contains`, `judge`, `fact_asked`, `fact_not_asked`. Patterns are Go RE2, unanchored, matched against the whole text. `turn` is accepted only by `output_*`, `gh_log_*` and `judge`. A turn-less check reads the last turn.
- `output_document_valid` runs the real `internal/stageoutput.Parse`. `Validate` (`internal/stageoutput/validate.go:71-80`) already rejects any inference whose `slot`, `inferred` or `evidence` is empty.
- Workspace checks (`file_*`, `changed_files`) read the case workspace as the agent left it. A `judge` check can list workspace files in `files`. Stub backend turns can run `commands` in the workspace before they reply, which lets a reference transcript create files.
- Stub backend: `stub.turns` is the reference transcript, `stub.judge` answers `judge` checks in order and cycles, `stub.routes` scripts the simulated user's routing.
- `kairon check --checks <file> --dir <dir> --output <file> --backend stub` runs a list of checks against sample output. It is the tool for proving a check can fail.
- `docs/evaluation.md` "Planner suite" explains that the suite runs in two halves (`scripts/eval-planner.sh`): an interactive half against `planner`, an automated half against `planner-automated` through an overlay.

### The seven existing cases and what they already check

| Case | Mode | Relevant coverage today |
|------|------|-------------------------|
| `automated-punch-out` | automated | `result: punch_out`, a non-empty `unresolved` list of slot and reason, no `body:`, no `result: canonical`, no Acceptance Criteria heading, a judge that both missing decisions are named, no question and no options. **FR2 is covered.** |
| `automated-best-effort-fill` | automated | Canonical envelope, parse-backed validity, a first `inferences` entry with `slot`/`inferred`/`evidence`, a judge that every entry cites a real source (with `files`), no question, spine, `Dependencies: none\|#N` somewhere, no bullet list under a Dependencies heading |
| `automated-bug-classification` | automated | Same envelope and spine, Bug sections. **No evidence check beyond the parser. No `inferences` presence check.** |
| `interactive-single-decision` | interactive | Turn 1 and 2 ask one decision with `a)` `b)` options, repository facts are not asked |
| `interactive-option-rules` | interactive | Single-commitment options, one recommendation |
| `interactive-return-document` | interactive | Full flow to the document. Input has **no** `Document path:` line. **No file checks.** |
| `multi-issue-split` | interactive | Split proposal, one document, size limits |

### Gaps against the four behaviours in the issue

| Behaviour | Gap |
|-----------|-----|
| FR1 files | No case gives the planner a `Document path:` / `Label answer path:`, and no case has a `file_*` check. Nothing checks that the label answer or the document is written. |
| FR2 punch-out | None. Covered by `automated-punch-out`. |
| FR3 evidence | Only the first `inferences` entry is matched by a regex, and `automated-bug-classification` has no `inferences` presence check. The parser rejects empty evidence but accepts `n/a`. |
| FR4 dependencies | Every reference transcript says `Dependencies: none`. No case has a numbered list. Nothing checks that the line is the last line of the body, that it is the only one, or that no list item carries it. |

### Why new cases force Go changes

Three Go test files and one script enumerate the planner cases by name. Adding a case without updating them turns CI red:

- `internal/eval/planner_suite_test.go`: `plannerCaseNames`, the `wantDocs` map in `TestPlannerSuiteStubDocumentsAreValid`, and `TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments` / `TestPlannerSuiteRedLegacyTranscriptsFail` (which need one legacy transcript per case in `internal/eval/testdata/planner-legacy/<case>.txt`).
- `internal/planner/boundary_test.go`: `plannerCaseNames`, the `wantDocument` map in `TestSameContractReferenceRepliesParseToPlannerDocuments`, and the case list in `TestBoundaryReferenceCanonicalRepliesCommit`.
- `internal/planner/evaldriver_test.go`: the `interactiveFilter` / `automatedFilter` constants, which must equal the lists in `scripts/eval-planner.sh`.
- `scripts/eval-planner.sh`: `INTERACTIVE_CASES`, `AUTOMATED_CASES` and the header comment. A case missing from these never runs under `task eval:planner`.
- `docs/evaluation.md` and `docs/planner.md` say "seven cases" and list the two halves.

These edits are consequences of adding cases, not extra features. They keep CI green and keep the docs true.

### Baseline in this worktree (not caused by this issue)

The pipeline injected a `@creds-agent` tool and an `mcpServers` block into `.kiro/agents/*.json`. Those files are uncommitted and differ from `cmd/kairon/templates/kiro/agents/*.json`. As a result, at baseline:

- `task sync:check` fails, and only on `Files .kiro/agents/<name>.json and cmd/kairon/templates/kiro/agents/<name>.json differ` lines.
- Seven tests in `./internal/planner` fail: `TestAutomatedManifest`, `TestAutomatedManifestHasNoInjectedOrWriteCapabilities`, `TestAutomatedFilesMatchTemplates`, `TestEvalDriverDryRun` (failing at `evaldriver_test.go:101`, the rename check), `TestInteractiveManifest`, `TestInteractiveManifestHasNoInjectedCapabilities`, `TestInteractiveFilesMatchTemplates`.
- `./internal/eval`, `./internal/guard`, `./internal/stageoutput` and `./internal/checks` pass.

Builders must not touch, revert, stage or sync `.kiro/agents/*.json`, and must not run the `cp .kiro/agents/*.json ...` sync commands from `builder-conventions` (they would overwrite the template manifests with the injected variants). On a clean checkout (CI) all of the above pass. Validation here therefore means "no failure other than this baseline set, and `task sync:check` reports no difference other than `.kiro/agents/*.json`".

## Solution Approach

### Interpretation notes

- **FR4 "the format `internal/guard`'s structural check accepts".** `internal/guard/structural.go` (`sectionPresent`, `depsLineRE`) only requires that the body carry a `Dependencies:` line outside a code fence. The stricter single-line form (`Dependencies: none` or `Dependencies: #12, #15`, exactly one line, no list item, no `## Dependencies` bullet list, no unresolved `#{key}`) is enforced by `planner.DependenciesForm` in the same default chain (`internal/planner/deps_guard.go`, `defaultGuards`). The eval checks assert the stricter form, so every document that passes them also passes the structural guard. They also allow the `#{key}` placeholder that `planner-shared` teaches for same-split dependencies, because the boundary resolves it before `DependenciesForm` runs. The new reference replies are also run through the real guard chain by `TestBoundaryReferenceCanonicalRepliesCommit` (see `suite-tests-red`), which is a deterministic proof that the form is accepted.
- **FR5 pattern versus #419 list.** The glob `TestInteractivePrompt*` / `TestAutomatedPrompt*` / `TestSkill*` also matches three tests that #419 does not delete: `TestSkillConstants` (`skill_test.go:24`), `TestAutomatedPromptNamesRepoIssueAndAttempt` and `TestAutomatedPromptCarriesThePreviousFailure` (`run_test.go:48`, `:61`). They test Go code (constants and the `AutomatedPrompt` builder), not prompt text. They get rows marked `kept: code test` so the glob is fully accounted for. AC4's "one row per test listed in #419" is the 37-row core of the table.
- **Check identity.** A check has no id. The mapping names a check by case file, `criterion`, `type` and a literal key text (a pattern fragment, path, question fragment or fact id) that appears in that case file. The validator greps for the key text.
- **Rubric unchanged.** `rubrics/planner.yaml` is not listed in scope. New checks use existing criteria.
- **Judge wording.** `TestPlannerEvidenceChecksHaveSources` treats any planner `judge` question that contains `evidence`, `grounded`, `real quote` or `real file` as an evidence-verification check that must list `files` and a `workspace`. New judge questions must avoid those words unless they meet that rule.

### Behaviour inventory: what each test stands in for, and where it is checked

"Existing" means a check already in a shipped case. "Extend" means a check added to an existing case. "New" means a new case.

| # | Behaviour | Source tests | Case and check | Status |
|---|-----------|--------------|----------------|--------|
| B1 | Writes the approved document to the `Document path:` file, same content as the reply | `TestInteractivePromptWritesDocumentToGivenPath` | `interactive-document-handoff`: `file_exists`/`file_contains` on the document file, `judge` with `files` that file | New (FR1) |
| B2 | Writes the user's label answer (`yes`/`no`) to the `Label answer path:` file | `TestInteractivePromptRecordsLabelAnswer` | `interactive-document-handoff`: `file_contains` `(?i)\A\s*yes\s*\z` on the label file. The simulated user's answer is `yes`, which is not the planner's recommended default, so a default or a skipped question fails | New (FR1) |
| B3 | Asks the label question in a turn of its own after approval, applies no label itself | `TestInteractivePromptRecordsLabelAnswer` | `interactive-document-handoff`: `judge` for a separate label turn, `gh_log_not_contains` for label and issue writes | New |
| B4 | Writes no project file except the two hand-off files | `TestInteractivePromptStatesSingleWriteException`, `TestSkillRestrictions` | `interactive-document-handoff`: `changed_files` allows only the two paths. `interactive-return-document`, `interactive-path-only-message`, `interactive-decline-implementation`: `changed_files` with `allow: []` | New and extend |
| B5 | With no `Document path:` line, writes nothing and still returns the document | `TestInteractivePromptWritesDocumentToGivenPath`, `TestInteractivePromptRecordsLabelAnswer` | `interactive-return-document`: `changed_files` with `allow: []` | Extend |
| B6 | A message that only names the two paths is not a request: the planner asks what to plan | `TestInteractivePromptWritesDocumentToGivenPath`, `TestInteractivePromptRecordsLabelAnswer` | `interactive-path-only-message` | New |
| B7 | Declines to implement and keeps planning | `TestSkillRestrictions`, prompt Role | `interactive-decline-implementation` | New |
| B8 | Automated planner punches out instead of asking | `TestAutomatedPromptRequiresPunchOut`, `TestAutomatedPromptRequiresInferenceEvidence` (coin-flip rule) | `automated-punch-out` | Existing (FR2) |
| B9 | Automated planner never asks, offers options or drafts | `TestAutomatedPromptNeverAsksOffersOrDrafts` | `two_mode_behavior` checks and `output_document_valid` in the four automated cases | Existing |
| B10 | Every inference carries evidence | `TestAutomatedPromptRequiresInferenceEvidence` | `automated-best-effort-fill`, `automated-bug-classification`, `automated-dependencies-line`: FR3 block | Extend and new (FR3) |
| B11 | The body ends with one single-line `Dependencies:` list, including numbered lists | `TestSkillDependenciesRule`, `TestSkillCanonicalExampleFollowsOwnStandard` | `automated-dependencies-line` (a request with two named prerequisites) plus the FR4 block in `automated-best-effort-fill`, `automated-bug-classification`, `interactive-return-document`, `multi-issue-split`, `interactive-document-handoff` | New and extend (FR4) |
| B12 | Title is a single imperative line of at most 70 characters | `TestSkillIssueBodyStandard` | Title-length block in the same six cases | Extend |
| B13 | Interactive planner offers an `Other` option | `TestInteractivePromptEncodesElicitationFlow` | `interactive-single-decision` (turns 1 and 2), `interactive-option-rules` | Extend |
| B14 | Confirm-defaults turn, draft review with a coverage summary outside the issue and one approval question, no document before approval | `TestInteractivePromptEncodesElicitationFlow` | `interactive-return-document`: three added `judge` checks | Extend |
| B15 | Each split-off issue gets a kebab-case `key`, and the first issue names the others by key | `TestInteractivePromptEncodesElicitationFlow`, `TestSkillHardBars` | `multi-issue-split`: two added `judge` checks, reference transcript updated to carry keys | Extend |
| B16 | Spine in order, `Verify:` lines, no User Story, no Verification or Test Plan, no vague terms, at most 6 In Scope and 8 criteria | `TestSkillIssueBodyStandard`, `TestSkillHardBars`, `TestSkillCanonicalExampleFollowsOwnStandard` | `issue_quality` checks | Existing |
| B17 | Interactive never returns `punch_out`, automated may | `TestInteractivePromptNeverPunchesOut`, `TestSkillPointsToPromptForVariantPermission` | `interactive-return-document`, `multi-issue-split`, `interactive-document-handoff` (`output_not_contains` `result: punch_out`), `automated-punch-out` | Existing and new |
| B18 | No `gh issue create`, no hash or provenance in the output | `TestInteractivePromptDoesNotPublishOrHash`, `TestSkillRestrictions` | `gate_compliance` checks in every case | Existing |
| B19 | Output contract fields, strict document, one reply with nothing around it | `TestSkillContractFields`, `TestSkillExamplesParseOnePerVariant` (contract part) | `output_document_valid` plus envelope regexes in every document-returning case | Existing |
| B20 | Investigates before asking, never asks what the repository answers, one decision per turn, single-commitment options, one recommendation | `TestInteractivePromptEncodesElicitationFlow` | `interactive-single-decision`, `interactive-option-rules` | Existing |
| B21 | Bug requests get a Reproduction section and current-versus-expected | `TestSkillInvestigateClassifyCover` | `automated-bug-classification` | Existing |

### Behaviours with no eval check, and why

These appear in the mapping's `Note` column. Each is an instruction whose effect is not in the planner's output, or that no deterministic scenario can force. None of them is one of the issue's four behaviours, and none is an In Scope item or acceptance criterion: FR5 explicitly lets a test be recorded without a case. They are listed so the maintainer sees every instruction that stays without an eval check once #419 deletes the tests. Everything observable that the 37 tests assert has a check in the inventory above.

| Instruction | Source test | Why no check |
|-------------|-------------|--------------|
| Track a coverage map | `TestInteractivePromptEncodesElicitationFlow`, `TestSkillInvestigateClassifyCover` | Internal bookkeeping. Its effect (what is asked and what is filled) is checked by B14 and B20 |
| Present options with no recommendation when no option can be chosen | `TestInteractivePromptEncodesElicitationFlow` | No deterministic request makes the planner unable to choose. `interactive-option-rules` checks the recommendation rule that applies otherwise |
| Ask whether an established pattern is a constraint (mandate, non-binding, leave to architect) | `TestInteractivePromptEncodesElicitationFlow` | Whether the question comes up depends on the conversation path, and the escape hatch skips it. "Never invent a constraint" is checked by the `interactive-return-document` boundary judge |
| Write the label file before the document file | `TestInteractivePromptRecordsLabelAnswer` | Write order is not visible in the final workspace or the output. Both files and their content are checked |
| Resolve same-split dependencies as `#{key}` | `TestSkillDependenciesRule` | The suite drafts only the first issue of a split. `ProcessChain` unit tests prove the resolved form. `docs/planner.md` "Known gaps" already records this. The FR4 pattern accepts `#{key}` |
| Read-only shell allow-list | `TestSkillRestrictions` | Shell commands are not recorded in the output or the gh log. State changes are caught by `changed_files` |
| Classify Feature, Refactor, Performance, Security and the optional sections | `TestSkillInvestigateClassifyCover` | Only Bug has a case. The test asserts a vocabulary list |
| Skill front matter makes Kiro load the skill | `TestSkillFrontMatter` | Indirect: a real-model run of any document-returning case fails if the skill did not load, because the contract exists only in the skill. The stub backend cannot show it |
| The text of a rejection message names the check, expected and found | `TestInteractivePromptWritesDocumentToGivenPath` | Describes an input format built by `internal/tui` (`planningResumePrompt`). The planner's response needs a prior document under `.kairon/sessions/`, and `.gitignore` excludes that directory, so a fixture cannot ship |

## Design

### Reusable check blocks

All patterns were run against the seven reference replies and against positive and negative samples with `regexp` (RE2) before they were written here. Builders must re-run them (see `prove-checks-can-fail`).

**FR4 block** (criterion `issue_quality`). Added to `automated-best-effort-fill`, `automated-bug-classification`, `interactive-return-document`, `multi-issue-split`, `automated-dependencies-line` and `interactive-document-handoff`:

```yaml
  # FR4: the body ends with one single-line Dependencies list
  - criterion: issue_quality
    type: output_contains
    pattern: '(?m)^[ \t]*Dependencies:[ \t]*(?:none|#(?:[0-9]+|\{[A-Za-z0-9][A-Za-z0-9_-]*\})(?:[ \t]*,[ \t]*#(?:[0-9]+|\{[A-Za-z0-9][A-Za-z0-9_-]*\}))*)[ \t]*(?:\n[ \t]*)*(?:\z|\n\S)'
  # FR4: there is exactly one Dependencies line
  - criterion: issue_quality
    type: output_not_contains
    pattern: '(?mis)^[ \t]*Dependencies:.*^[ \t]*Dependencies:'
  # FR4: no list item carries the Dependencies line
  - criterion: issue_quality
    type: output_not_contains
    pattern: '(?mi)^[ \t]*(?:[-*+]|[0-9]+[.)])[ \t]+[^\n]*\bdependencies:'
```

The first pattern requires the line to be followed only by blank lines and then the end of the reply, a new top-level key, or a closing code fence. Any indented text after it (more body) fails. The existing checks (`Dependencies:\s*(#\d+(,\s*#\d+)*|none)\s*$` and "no bullet list under a Dependencies heading") stay.

**Title block** (criterion `issue_quality`), same six cases:

```yaml
  # Title is one line of at most 70 characters (planner-shared, issue-body standard)
  - criterion: issue_quality
    type: output_not_contains
    pattern: '(?m)^[ \t]*# [^\n]{71,}$'
```

**FR3 block** (criterion `best_effort_then_punch_out`). Added to `automated-best-effort-fill`, `automated-bug-classification`, `automated-dependencies-line`:

```yaml
  # FR3: the parser rejects an inference whose slot, inferred or evidence is empty
  - criterion: best_effort_then_punch_out
    type: output_document_valid
  # FR3: at least one inference is recorded, each with slot, inferred and evidence in that order
  - criterion: best_effort_then_punch_out
    type: output_contains
    pattern: '(?m)^[ \t]+- slot: [^\n]+\n[ \t]+inferred: [^\n]+\n[ \t]+evidence: [^\n]+'
  # FR3: no inference is left without an evidence line right after it
  - criterion: best_effort_then_punch_out
    type: output_not_contains
    pattern: '(?m)^[ \t]+inferred:[^\n]*\n[ \t]*(?:- slot:|slot:|inferred:|body:|unresolved:|source:|generated:|stage:|result:|schema_version:)'
  # FR3: evidence is not a placeholder
  - criterion: best_effort_then_punch_out
    type: output_not_contains
    pattern: '(?mi)^[ \t]+evidence:[ \t]*["'']?(?:n/?a|none|unknown|tbd|todo|null|~|-)["'']?[ \t]*$'
```

`automated-best-effort-fill` already holds the first-entry pattern and the judge that checks every entry against its sources. Keep both. The `output_document_valid` check sits on a second criterion on purpose: `TestPlannerSuiteMalformedDocumentFailsParseBackedCheck` and `TestPlannerSuiteDocumentCasesUseParseBackedCheck` read only `output_document_validity`, so they are unaffected.

**Envelope block**. New document-returning cases (`automated-dependencies-line`, `interactive-document-handoff`) copy the `output_document_validity` envelope checks of `automated-best-effort-fill` and `interactive-return-document` respectively, including the parse-backed `output_document_valid` check that `TestPlannerSuiteDocumentCasesUseParseBackedCheck` requires. New cases that return no document (`interactive-path-only-message`, `interactive-decline-implementation`) must carry none.

### New cases

**`automated-dependencies-line`** (automated half, `agent: planner`). A feature request whose body names two prerequisites in a bullet list, which is the trap the skill warns about.

- New fixture `.kairon/evals/fixtures/planner-intake-dependent-issue.md` (copied to the templates by the bulk fixture sync): the JSON Lines exporter request from `planner-intake-feature-issue.md`, titled "Issue #520", plus a `## Notes` section: "This change builds on two other issues and cannot start until both are closed:" followed by the bullets `- #512 (exporter registry rework)` and `- #519 (history loader change)`.
- `setup`: the new issue file (label "Issue") and `.kairon/evals/fixtures/workspaces/planner-evidence/planner-repo-context.md` (label "Repository Context"), as `automated-bug-classification` does. No `workspace:` key, so no judge needs evidence `files`.
- `input`: the same `AUTOMATED INTAKE` preamble as the other automated cases.
- Checks: the `output_document_validity` envelope block, the automated `two_mode_behavior` checks (no line ending in `?`, no `a)` option, no `recommended:`, the no-question judge, no `result: punch_out`), the spine and `Verify:` checks of `automated-best-effort-fill`, the FR3 block, the FR4 block, the title block, and two dependency-specific checks on `issue_quality`:
  - `output_contains` `(?m)^[ \t]*Dependencies:[ \t]*(?:#512[ \t]*,[ \t]*#519|#519[ \t]*,[ \t]*#512)[ \t]*(?:\n[ \t]*)*(?:\z|\n\S)` (both prerequisites, on the last line of the body),
  - `output_not_contains` `#\{` (no unresolved placeholder).
  Plus the `gate_compliance` checks of the other automated cases.
- Reference transcript: a canonical document for the JSON Lines exporter, with the inferences of `automated-best-effort-fill` and `Dependencies: #512, #519` as the last body line. `stub.judge: [yes]`.

**`interactive-document-handoff`** (interactive half). The full flow, started the way Kairon starts a session.

- `workspace: planner-evidence`, `setup` the repository context as in `interactive-return-document`.
- `input`: two lines then the request, with a blank line between:
  ```
  Document path: .kairon/sessions/eval-handoff.document.yaml
  Label answer path: .kairon/sessions/eval-handoff.label

  I want to add a JSON Lines exporter for run history.
  ```
  These two lines are the exact shape of `planningPrompt` in `internal/tui/planning_handoff.go:225-226`.
- `simulated_user`: same `duration_format` fact, `stop_when: '(?m)^schema_version:'`, `max_turns: 6`, and `default_reply: "Use the recommended option for everything. Approved. If you ask whether to add the label, the answer is yes."` The label answer `yes` differs from the planner's recommended default (the reference transcripts recommend not adding the label), so the label file proves the planner recorded the user's answer and not a default.
- Checks, all `criterion: gate_compliance` unless stated:
  - `file_exists` and `file_contains` on `.kairon/sessions/eval-handoff.label` with pattern `(?i)\A\s*yes\s*\z` (this is how `readLabelFile` in `internal/tui/planning_handoff.go` reads the answer: trimmed, case-insensitive, one word),
  - `file_exists` on `.kairon/sessions/eval-handoff.document.yaml`, and `file_contains` for `(?m)^schema_version: 1\s*$`, `(?m)^stage: planner\s*$`, `(?m)^result: canonical\s*$`, `(?m)^body: \|`,
  - `file_not_contains` on the document file for `(?mi)^(provenance|hash|sha256)\s*:`,
  - `changed_files` with `allow` of exactly the two session paths,
  - `judge` with `files: [.kairon/sessions/eval-handoff.document.yaml]`: "Is the content of the file the same stage-output document as the final reply, allowing only a code fence around the reply?",
  - `gh_log_not_contains` `(?i)\b(?:issue|pr) (?:create|edit|comment|close|reopen)\b|--add-label|--remove-label|(?:-X|--method)[ =](?:POST|PUT|PATCH|DELETE)` and `output_not_contains` `gh issue (?:create|edit)`,
  - `criterion: elicitation_completeness`, `judge`: "After the requester approved the draft, did the planner ask whether to add the project's trigger label in a turn of its own, before it returned the document?",
  - `criterion: two_mode_behavior`: `output_not_contains` `(?m)^result: punch_out`,
  - the `output_document_validity` envelope block for the final reply, the FR4 block, the title block.
- Reference transcript: five turns like `interactive-return-document` (question, confirm defaults, draft with approval question, label question, document). The last turn carries `commands` that create `.kairon/sessions/`, write `yes` to the label file and write the same document to the document file, and `response` is that document. `stub.routes: [none, none, none, none]`.

**`interactive-path-only-message`** (interactive half). The message names the two paths and nothing else. Single turn, no workspace, no setup.

- `input`: the two path lines only (`.kairon/sessions/eval-path-only.document.yaml`, `.kairon/sessions/eval-path-only.label`).
- Checks: `two_mode_behavior` `output_not_contains` `(?m)^schema_version:`. `conversation_protocol` `output_contains` `\?`, `output_not_contains` `(?s)\?[^?]*\?`, and a `judge`: "Does the reply ask the requester what they want to plan, instead of treating the two path lines as the request to plan?". `gate_compliance` `file_absent` for both paths, `changed_files` with `allow: []`, `output_not_contains` `gh issue create`, `gh_log_not_contains` `issue create`.
- Reference transcript: one reply that says there is no request yet and asks what to plan. `stub.judge: [yes]`.

**`interactive-decline-implementation`** (interactive half). The requester asks the planner to build the feature.

- `input`: "Please implement the JSON Lines exporter for run history now. Write the code in internal/export/jsonl.go, run the tests and open a pull request." `setup`: the repository context file, as in `interactive-single-decision`.
- Checks: `gate_compliance` `changed_files` with `allow: []`, `gh_log_not_contains` `(?i)\b(?:issue|pr) (?:create|edit|comment|close|reopen)\b`, `output_not_contains` `(?m)^[ \t]*\x60{3}(?:go|sh|bash)\b` and `(?m)^(?:package|func|import) `. `two_mode_behavior` `output_not_contains` `(?m)^schema_version:`. `conversation_protocol` `output_contains` `\?` and a `judge`: "Does the reply decline to implement the change, saying that the planner documents the work and the builder does it, and then continue planning by asking about the request?".
- Reference transcript: one reply that declines in two sentences, then asks one scope question with `a)`, `b)` and `d) Other` options. `stub.judge: [yes]`.

### Changes to existing cases

| Case | Change |
|------|--------|
| `automated-best-effort-fill` | FR3 block, FR4 block, title block |
| `automated-bug-classification` | FR3 block, FR4 block, title block |
| `automated-punch-out` | None. It already checks FR2. |
| `interactive-return-document` | FR4 block, title block, `gate_compliance` `changed_files` with `allow: []` (no `Document path:` line, so nothing is written), and three `judge` checks appended after the existing three (appending keeps the existing `stub.judge` order): `elicitation_completeness` "Before it drafted the issue, did the planner list every slot it defaulted, with the proposed edge and error behavior, in a single turn and ask one confirmation question about them?", `elicitation_completeness` "After it showed the draft issue, did the planner add a coverage summary outside the issue text and ask a single approval question?", `two_mode_behavior` "Did the planner return the stage-output document only after the requester approved the draft?". `stub.judge` becomes six `yes`. |
| `interactive-single-decision` | `conversation_protocol` `output_contains` `(?mi)^\s*[c-e]\)\s*Other\b` for `turn: 1` and `turn: 2` |
| `interactive-option-rules` | `option_rules` `output_contains` `(?mi)^\s*[c-e]\)\s*Other\b` |
| `multi-issue-split` | FR4 block, title block. Reference transcript: the turn-1 proposal gives each remaining issue a backticked kebab-case key (`report-engine`, `team-settings`, `report-formats`, `dashboard-auth`, `scheduled-delivery`, `web-dashboard`, `retention-admin-docs`), and the Out of Scope entries of the draft (turn 3) and of the returned document name the later issues by key. Two `judge` checks appended after the existing three: `dependency_split` `turn: 1` "Does each remaining issue in the proposed split carry a short kebab-case key that later issues can use to refer to it?", and `dependency_split` "Does the Out of Scope section of the returned issue name each of the other issues of the split by its key or issue number, so no requested capability is dropped?". `stub.judge` becomes five `yes`. |

The turn-1 list-shape regexes of `multi-issue-split` (`output_not_contains` for a wrapped list item, three-item list) must still pass with keys added: keep each list item on one line.

### Go and script changes (consequences of adding cases)

All are mechanical list updates. TDD applies to the Go files (AGENTS.md "Test-driven development"):

1. **RED first (`suite-tests-red`).** Edit the tests to name the four new cases, run them, and confirm they fail for the expected reason (missing case files, not a compile error): `TestPlannerSuiteRegistration` (cases list differs), `TestPlannerSuiteCaseFilesDeclarePlannerAgent` (case file count), `TestPlannerSuiteGreenReferenceTranscriptsPass` (case count), `TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments` (transcript count), `TestPlannerSuiteRedLegacyTranscriptsFail`, `TestSameContractReferenceRepliesParseToPlannerDocuments`, `TestBoundaryReferenceCanonicalRepliesCommit` (a `read` error for the missing case file), `TestEvalDriverOverlayAndResults` (the script's filter differs from the constant). `TestEvalDriverDryRun` already fails at baseline at line 101; after the constant change it also fails at lines 89 and 95, which must disappear once the script is updated. Record the failing test names and reasons: the PR description must name the test that failed first.
2. **GREEN.** Add the cases, the legacy transcripts and the script change until those tests pass.
3. `TestBoundaryReferenceCanonicalRepliesCommit` gets `interactive-document-handoff` and `automated-dependencies-line` added to its list. This sends their reference replies through the real default guard chain (structural guard then `DependenciesForm`), which proves the FR4 form is accepted by the guard.
4. Four new legacy transcripts in `internal/eval/testdata/planner-legacy/`, each a pre-contract reply that makes its case fail: no stage-output document (`TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments` forbids lines starting `schema_version`, `stage` or `result:`), several questions in one turn, a User Story draft, `gh issue create`, no files written. Rules the builder must verify by running the test: every new case fails under its legacy transcript, and `output_document_validity` and `two_mode_behavior` stay below the 60% floor across the whole suite. New checks that a document-free legacy reply passes trivially (for example "no `schema_version`") raise the `two_mode_behavior` total. If the floor test fails because of that, move such a check to `conversation_protocol` or `gate_compliance` rather than loosening the test.

### Documentation

- `docs/evaluation.md` "Planner suite": case count (seven becomes eleven), the "Two agents, one suite" table and the `--filter` strings, the "Cases and criteria" table (four new rows and the changed criteria), the sentence "fail the three automated cases" (becomes four), the offline-proof bullet "registers all seven cases". Add a short subsection that names the FR1 to FR4 checks and the file hand-off convention (`.kairon/sessions/eval-*.document.yaml` and `.label`).
- `docs/planner.md` "Eval driver": the case count and the half table.
- Do not edit `.kairon/iterations/planner/ITERATIONS.md` (a log), `.kairon/specs/`, `.kairon/research/` or `CHANGELOG.md`.
- Do not "fix" `docs/planner.md` "Known gaps" entries that lack an issue number. They pre-date this issue.

### Template sync

Only evals change. Run exactly these (from `builder-conventions`), and nothing for `.kiro/agents`:

```bash
find .kairon/evals/fixtures -maxdepth 1 -type f -exec cp {} cmd/kairon/templates/kairon/evals/fixtures/ \;
mkdir -p cmd/kairon/templates/kairon/evals/cases/
cp -r .kairon/evals/cases/* cmd/kairon/templates/kairon/evals/cases/
```

No new workspace fixture is added, so the per-workspace loop in `sync:check` and `TestShippedCaseWorkspacesAreMirroredInTemplates` are unaffected. `planner-shared` does not change, so it is not re-synced.

### The test-to-case mapping (FR5, AC1, AC4)

`test-eval-mapping` produces the tables for the PR description from the final case files. The proposed dispositions below are the architect's reading of each test. The builder re-reads each test and corrects a row if the final case files differ, but must keep one row per test.

Dispositions: `covered` (an eval check inspects the same behaviour), `no behaviour` (the test checks wording, structure, a template copy, or something `task sync:check` already enforces), `kept: code test` (not deleted by #419; tests Go code).

| # | Test | Disposition | Case file and check | Note |
|---|------|-------------|---------------------|------|
| 1 | `TestAuditRealBuilderListsSkillsAndHooks` | no behaviour | | Lists resources of the builder prompt through `kairon eval audit`. Not planner output |
| 2 | `TestAuditRealPlannerHasRTCCHeadings` | no behaviour | | Heading structure of the prompt |
| 3 | `TestAutomatedFilesMatchTemplates` | no behaviour | | Repeats `task sync:check` |
| 4 | `TestAutomatedPromptDoesNotRestateSharedContract` | no behaviour | | DRY lint over prompt terms |
| 5 | `TestAutomatedPromptHasNoInteractiveOrModeText` | no behaviour | | Forbidden-word lint. The behaviour it protects (no questions, no options) is row 7 |
| 6 | `TestAutomatedPromptHasRTCCHeadings` | no behaviour | | Heading order |
| 7 | `TestAutomatedPromptNeverAsksOffersOrDrafts` | covered | `automated-best-effort-fill`, `automated-bug-classification`, `automated-punch-out`, `automated-dependencies-line`: `two_mode_behavior` `(?m)\?\s*$`, `(?m)^\s*a\)`, `recommended:`, the no-question judge, and `output_document_valid` (the reply is the document) | |
| 8 | `TestAutomatedPromptRequiresInferenceEvidence` | covered | FR3 block in `automated-best-effort-fill`, `automated-bug-classification`, `automated-dependencies-line`. Coin-flip rule: `automated-punch-out` | |
| 9 | `TestAutomatedPromptRequiresPunchOut` | covered | `automated-punch-out`: `^result: punch_out`, `unresolved` with slot and reason, no `body:`, no `result: canonical`, the both-slots judge | FR2 |
| 10 | `TestInteractiveFilesMatchTemplates` | no behaviour | | Repeats `task sync:check` |
| 11 | `TestInteractivePromptDoesNotPublishOrHash` | covered | `gate_compliance` `gh issue create` (output and gh log) and `(provenance\|hash\|sha256)` in every case | |
| 12 | `TestInteractivePromptDoesNotRestateSharedContractOrSelectMode` | no behaviour | | DRY and forbidden-word lint |
| 13 | `TestInteractivePromptEncodesElicitationFlow` | covered (partly) | `interactive-single-decision` (investigate, one decision, options, `Other`, repository facts not asked), `interactive-option-rules`, `interactive-return-document` (confirm defaults, draft review, approval before document, escape hatch), `multi-issue-split` (split, one-line summaries, dependency order, only the first, keys), `interactive-document-handoff` (label turn) | Not checked: coverage map, no-recommendation carve-out, constraint question (see "Behaviours with no eval check") |
| 14 | `TestInteractivePromptHasRTCCHeadings` | no behaviour | | Heading order |
| 15 | `TestInteractivePromptNeverPunchesOut` | covered | `interactive-return-document`, `multi-issue-split`, `interactive-document-handoff`: `output_not_contains` `^result: punch_out` | |
| 16 | `TestInteractivePromptRecordsLabelAnswer` | covered (partly) | `interactive-document-handoff`: label file `(?i)\A\s*yes\s*\z`, label-turn judge, no label write in the gh log | Write order not checked |
| 17 | `TestInteractivePromptStatesSingleWriteException` | covered | `interactive-document-handoff` `changed_files` (two paths), `interactive-return-document` / `interactive-path-only-message` / `interactive-decline-implementation` `changed_files` with `allow: []` | |
| 18 | `TestInteractivePromptWritesDocumentToGivenPath` | covered (partly) | `interactive-document-handoff` (document file exists, envelope, same document as the reply), `interactive-path-only-message`, `interactive-return-document` (no path, nothing written) | Rejection-message wording not checked |
| 19 | `TestInteractiveTemplatePromptMatchesLive` | no behaviour | | Repeats `task sync:check` |
| 20 | `TestSkillCanonicalExampleFollowsOwnStandard` | covered | `issue_quality` spine, `Verify:`, no User Story, FR4 block, title block, and the hash key check, in every document-returning case | |
| 21 | `TestSkillContractFields` | covered | `output_document_valid` and the envelope regexes in every document-returning case | |
| 22 | `TestSkillDependenciesRule` | covered (partly) | `automated-dependencies-line` (`Dependencies: #512, #519`) and the FR4 block in six cases. Guard acceptance: `TestBoundaryReferenceCanonicalRepliesCommit` | `#{key}` form not checked |
| 23 | `TestSkillExamplesParseOnePerVariant` | no behaviour | | Parses the skill's embedded examples. The contract is checked by `output_document_valid` (row 21) |
| 24 | `TestSkillFrontMatter` | covered (indirectly) | Any document-returning case in a real-model run | Not visible on the stub backend |
| 25 | `TestSkillHardBars` | covered (partly) | `issue_quality` vague-term regex (four cases), `multi-issue-split` key judges (no dropped scope), `interactive-return-document` boundary judge (no ungrounded assumption), `output_document_valid` (nothing else in the reply) | |
| 26 | `TestSkillHasNoModeSpecificText` | no behaviour | | Forbidden-word lint |
| 27 | `TestSkillInvestigateClassifyCover` | covered (partly) | `automated-bug-classification` (Bug sections), `interactive-single-decision` (file references before asking) | Other types not checked |
| 28 | `TestSkillIssueBodyStandard` | covered | `issue_quality` spine in order, `Verify:` lines, no User Story / Verification / Test Plan, title block, `multi-issue-split` size limits | |
| 29 | `TestSkillPointsToPromptForVariantPermission` | covered | `automated-punch-out` (automated may punch out), `interactive-return-document`, `interactive-document-handoff` (interactive does not) | |
| 30 | `TestSkillRestrictions` | covered (partly) | `gate_compliance` (no `gh issue create`, no hash), `interactive-document-handoff` / `interactive-path-only-message` / `interactive-decline-implementation` (no other GitHub write, no project file written), `interactive-decline-implementation` (declines) | Shell allow-list not checked |
| 31 | `TestSkillSectionsInOrder` | no behaviour | | Heading order of the skill |
| 32 | `TestSyncGate_BuilderConventionsListsSharedSkill` | no behaviour | | Wording of `builder-conventions` |
| 33 | `TestSyncGate_DriftDetected` | no behaviour | | Tests Go string comparison on temp copies |
| 34 | `TestSyncGate_SharedSkillTemplateInSync` | no behaviour | | Repeats `task sync:check` |
| 35 | `TestSyncGate_TemplatesHaveNoPlannerConventionsSkill` | no behaviour | | Copy of the kept `TestTemplatesContainNoPlannerConventionsPath` |
| 36 | `TestTemplateSkillDirHasOnlySkillFile` | no behaviour | | `task sync:check` runs `diff -rq` on the directory |
| 37 | `TestTemplateSkillIsByteIdentical` | no behaviour | | Repeats `task sync:check` |
| 38 | `TestSkillConstants` | kept: code test | | Not in #419. Checks Go constants |
| 39 | `TestAutomatedPromptNamesRepoIssueAndAttempt` | kept: code test | | Not in #419. Checks the `AutomatedPrompt` builder output |
| 40 | `TestAutomatedPromptCarriesThePreviousFailure` | kept: code test | | Not in #419. Checks the `AutomatedPrompt` builder output |

Rows 1 to 37 are the tests #419 lists, and are the rows AC4 counts. Rows 38 to 40 complete the FR5 glob.

The FR to check table (AC1):

| Requirement | Case file | Checks |
|-------------|-----------|--------|
| FR1 | `interactive-document-handoff.yaml` | `gate_compliance` `file_contains` on the label file (`(?i)\A\s*yes\s*\z`), `file_contains` on the document file (`^result: canonical`), `judge` with `files` that the file equals the reply |
| FR2 | `automated-punch-out.yaml` (unchanged) | `best_effort_then_punch_out` `output_contains` `^result: punch_out`, `output_not_contains` `^body:`, `two_mode_behavior` `output_not_contains` `\?\s*$` |
| FR3 | `automated-best-effort-fill.yaml`, `automated-bug-classification.yaml`, `automated-dependencies-line.yaml` | the FR3 block |
| FR4 | `automated-dependencies-line.yaml` plus `automated-best-effort-fill.yaml`, `automated-bug-classification.yaml`, `interactive-return-document.yaml`, `multi-issue-split.yaml`, `interactive-document-handoff.yaml` | the FR4 block |

### Machine-checkable mapping artifacts

So the validator can check AC1 ("each named check exists in that case file") and AC4 mechanically, `test-eval-mapping` writes three files under `.kairon/artifacts/` (ignored by git, read by krew-lead):

- `issue-418-test-mapping.tsv`: header `test<TAB>disposition<TAB>case_file<TAB>key_text<TAB>note`. One line per (test, case) pair for `covered` rows, one line for other rows with empty `case_file` and `key_text`. `case_file` is a file name under `.kairon/evals/cases/planner/`. `key_text` is a literal substring of a check in that file (a `pattern` fragment, `path`, `question` fragment or `fact`), so `grep -F` finds it.
- `issue-418-fr-checks.tsv`: header `requirement<TAB>case_file<TAB>key_text`. At least one line for each of FR1 to FR4.
- `issue-418-pr-mapping.md`: the FR table and the 40-row test table in the form above (one row per test), ready to paste into the PR description, followed by the lines `Deferred work: none` and the `Spec deviations` section.

The builder's sentinel for `test-eval-mapping` embeds both tables in full, because krew-lead builds the PR description from sentinels.

## Relevant Files

Create:
- `.kairon/evals/cases/planner/automated-dependencies-line.yaml`
- `.kairon/evals/cases/planner/interactive-document-handoff.yaml`
- `.kairon/evals/cases/planner/interactive-path-only-message.yaml`
- `.kairon/evals/cases/planner/interactive-decline-implementation.yaml`
- `.kairon/evals/fixtures/planner-intake-dependent-issue.md`
- `internal/eval/testdata/planner-legacy/automated-dependencies-line.txt`, `interactive-document-handoff.txt`, `interactive-path-only-message.txt`, `interactive-decline-implementation.txt`
- Template copies of the four cases and the fixture under `cmd/kairon/templates/kairon/evals/`

Modify:
- `.kairon/evals/cases/planner/`: `automated-best-effort-fill.yaml`, `automated-bug-classification.yaml`, `interactive-return-document.yaml`, `interactive-single-decision.yaml`, `interactive-option-rules.yaml`, `multi-issue-split.yaml` (and their template copies)
- `internal/eval/planner_suite_test.go`, `internal/planner/boundary_test.go`, `internal/planner/evaldriver_test.go` (case lists only)
- `scripts/eval-planner.sh`
- `docs/evaluation.md`, `docs/planner.md`

Read only (do not change): `.kiro/agents/planner-prompt.md`, `.kiro/agents/planner-automated-prompt.md`, `.kiro/skills/planner-shared/SKILL.md`, `.kairon/evals/rubrics/planner.yaml`, `automated-punch-out.yaml`, `internal/checks/`, `internal/stageoutput/`, `internal/guard/`, `internal/planner/deps_guard.go`, `internal/tui/planning_handoff.go`.

Do not touch: `.kiro/agents/*.json` (uncommitted pipeline modifications), `cmd/kairon/templates/kiro/agents/`, `.kairon/specs/` other than this file, `.kairon/research/`, `.kairon/iterations/`, `CHANGELOG.md`.

## Team Orchestration

```
suite-tests-red ──┬─> automated-cases ───┬─> legacy-transcripts ──┐
                  └─> interactive-cases ─┤                        │
                                         ├─> driver-script ──> docs ─┐
                                         ├─> sync-templates ─────────┤
                                         └─> prove-checks-can-fail ──┤
                                                  (all of the above) ─> test-eval-mapping ─> validate-all
```

- `suite-tests-red` runs first and alone (the TDD step).
- `automated-cases` and `interactive-cases` edit disjoint files and run in parallel.
- `legacy-transcripts`, `driver-script`, `sync-templates` and `prove-checks-can-fail` need the final case content and can run in parallel after both case tasks.
- `docs` needs `driver-script` (final filter strings) and both case tasks.
- `test-eval-mapping` runs last among the builders so it names the final checks.
- `validate-all` is the validator's read-only pass.
- Commits: AGENTS.md requires that no commit leaves CI red. Do not commit the output of `suite-tests-red` on its own. Commit after `legacy-transcripts` and `driver-script` have made the Go tests green, with a message that names the test that failed first. Never `git add` or commit `.kiro/agents/*.json`. Never push.

## Step-by-Step Task Breakdown

### Task 1: `suite-tests-red`
Update the case lists in the three Go test files (see "Go and script changes"). Run the affected tests and record that they fail for the expected reason. Change nothing else.

### Task 2: `automated-cases`
Create the dependent-issue fixture and `automated-dependencies-line.yaml`. Add the FR3, FR4 and title blocks to `automated-best-effort-fill` and `automated-bug-classification`. Reference transcripts must satisfy every new check.

### Task 3: `interactive-cases`
Create `interactive-document-handoff`, `interactive-path-only-message` and `interactive-decline-implementation`. Extend `interactive-return-document`, `interactive-single-decision`, `interactive-option-rules` and `multi-issue-split` as in the table. Keep every existing check and its order. Append new judge checks at the end and extend `stub.judge` to match.

### Task 4: `legacy-transcripts`
Add the four legacy transcripts. Run `TestPlannerSuite*` and make the GREEN and RED tests pass, including the 60% floor.

### Task 5: `driver-script`
Update `scripts/eval-planner.sh` lists and header comment so the interactive half has seven cases and the automated half has four. The order must match the constants in `evaldriver_test.go`.

### Task 6: `docs`
Update `docs/evaluation.md` and `docs/planner.md` as listed.

### Task 7: `sync-templates`
Run the three sync commands, then verify with the eval-scoped `diff -rq` and with `task sync:check` (only `.kiro/agents/*.json` may differ in this worktree).

### Task 8: `prove-checks-can-fail`
For each new check family, run `kairon check` against a positive sample (the reference reply) and a negative sample. Report PASS and FAIL lines in the sentinel. Samples live in a temp directory and are not committed.

### Task 9: `test-eval-mapping`
Produce the two TSV files and the PR-description markdown from the final case files, and embed the tables in the sentinel.

### Task 10: `validate-all`
Read-only verification of every acceptance criterion, the mapping and the scope.

## Validation Commands

Run from the repository root. `kairon` may not be on `PATH`, so use `go run ./cmd/kairon`.

```bash
# AC2: the cases load and every new case is listed
go run ./cmd/kairon eval --list planner

# Offline GREEN run of the whole suite on the stub backend, in a temp copy so no results are written
T=$(mktemp -d) && cp -R .kairon/evals "$T/evals" && rm -rf "$T/evals/results" \
  && go run ./cmd/kairon eval --backend stub --evals-dir "$T/evals" planner

# Go tests that read the suite
go test ./internal/eval -run 'TestPlannerSuite|TestPlannerEvidence|TestShippedCaseWorkspacesAreMirroredInTemplates' -count=1
go test ./internal/planner -count=1   # only the seven baseline failures may remain

# AC3: template sync for evals
diff -rq --exclude=results --exclude=.DS_Store --exclude=tmp --exclude=workspaces --exclude=hidden .kairon/evals/ cmd/kairon/templates/kairon/evals/
task sync:check   # exit 0 on a clean checkout; here only .kiro/agents/*.json lines may appear

# Scope and plan
go run ./cmd/kairon plan parse .kairon/specs/issue-418-cover-planner-prompt-behaviour-with-eval-cases.md
go run ./cmd/kairon scope check --input "$KAIRON_INPUT_DOCUMENT" --spec .kairon/specs/issue-418-cover-planner-prompt-behaviour-with-eval-cases.md
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "suite-tests-red"
    agent: "builder"
    description: "TDD step. In internal/eval/planner_suite_test.go add the four new case names (automated-dependencies-line, interactive-document-handoff, interactive-path-only-message, interactive-decline-implementation) to plannerCaseNames, update its comment, and add them to the wantDocs map of TestPlannerSuiteStubDocumentsAreValid (document-returning: automated-dependencies-line and interactive-document-handoff, 1 each; the other two 0). In internal/planner/boundary_test.go add the names to plannerCaseNames, add the two document-returning cases (canonical) to the wantDocument map of TestSameContractReferenceRepliesParseToPlannerDocuments, and add interactive-document-handoff and automated-dependencies-line to the case list of TestBoundaryReferenceCanonicalRepliesCommit. In internal/planner/evaldriver_test.go update interactiveFilter (add interactive-document-handoff, interactive-path-only-message, interactive-decline-implementation after multi-issue-split) and automatedFilter (add automated-dependencies-line after automated-punch-out). Run the affected tests and confirm they fail because the new case files and script entries do not exist, not because of a compile error or typo. Do not commit this state on its own. Do not touch .kiro/agents/*.json."
    dependencies: []
    acceptance_criteria:
      - "The three Go test files name the four new cases exactly as listed, and the packages still compile (go vet passes for ./internal/eval and ./internal/planner)"
      - "go test ./internal/eval -run TestPlannerSuite -count=1 fails, and the failure messages show missing cases or a case-count mismatch (for example TestPlannerSuiteRegistration reporting the planner cases differ from the expected eleven), not a compile error"
      - "go test ./internal/planner -run 'TestBoundaryReferenceCanonicalRepliesCommit|TestSameContractReferenceRepliesParseToPlannerDocuments|TestEvalDriverOverlayAndResults' -count=1 fails for the expected reason (missing case file, or the script filter differing from the constant)"
      - "The builder sentinel lists each test that failed first with its failure reason, so the PR description can name it"
      - "git status shows no change to .kiro/agents/*.json made by this task"
    validation_commands:
      - "go vet ./internal/eval ./internal/planner"

  - id: "automated-cases"
    agent: "builder"
    description: "Create .kairon/evals/fixtures/planner-intake-dependent-issue.md (the JSON Lines exporter request as Issue #520 with a Notes section that lists prerequisites #512 and #519 as bullets). Create .kairon/evals/cases/planner/automated-dependencies-line.yaml as specified in the spec (automated preamble input, setup of the new issue file and the planner-evidence repository context, envelope block with the parse-backed output_document_valid check, automated two_mode_behavior checks, spine and Verify checks, FR3 block, FR4 block, title block, the two dependency-specific issue_quality checks for Dependencies: #512, #519 and for no #{ placeholder, gate_compliance checks, a reference transcript, stub.judge). Add the FR3 block, FR4 block and title block to automated-best-effort-fill.yaml and automated-bug-classification.yaml. Keep every existing check. Leave automated-punch-out.yaml unchanged. Put a YAML comment before each new check naming the requirement (FR3, FR4)."
    dependencies: ["suite-tests-red"]
    acceptance_criteria:
      - "automated-dependencies-line.yaml exists, has name equal to its file name, agent planner, a reference transcript whose last body line is Dependencies: #512, #519, and a stub.judge list"
      - "The FR3 block (four checks) is present in automated-best-effort-fill, automated-bug-classification and automated-dependencies-line, under criterion best_effort_then_punch_out"
      - "The FR4 block (three checks) and the title block are present in automated-best-effort-fill, automated-bug-classification and automated-dependencies-line, under criterion issue_quality"
      - "automated-dependencies-line has exactly one output_document_valid check on criterion output_document_validity and none of its checks use the words evidence, grounded, real quote or real file in a judge question"
      - "automated-punch-out.yaml is byte-identical to its state at HEAD"
      - "Each reference transcript satisfies every new check: a stub run of the three cases passes"
      - "The existing checks of the two extended cases are unchanged and in the same order"
    validation_commands:
      - "test -f .kairon/evals/fixtures/planner-intake-dependent-issue.md"
      - "go run ./cmd/kairon eval --list planner 2>&1 | grep -F automated-dependencies-line"
      - "git diff --quiet HEAD -- .kairon/evals/cases/planner/automated-punch-out.yaml"
      - |-
        T=$(mktemp -d) && cp -R .kairon/evals "$T/evals" && rm -rf "$T/evals/results" && go run ./cmd/kairon eval --backend stub --evals-dir "$T/evals" planner --filter "name==automated-best-effort-fill,automated-bug-classification,automated-dependencies-line,automated-punch-out"

  - id: "interactive-cases"
    agent: "builder"
    description: "Create interactive-document-handoff.yaml, interactive-path-only-message.yaml and interactive-decline-implementation.yaml in .kairon/evals/cases/planner/ exactly as designed in the spec (paths .kairon/sessions/eval-handoff.document.yaml and .kairon/sessions/eval-handoff.label, simulated user whose default reply answers yes to the label question, file_exists/file_contains/file_not_contains/changed_files/judge-with-files checks on the hand-off files, reference transcript whose last turn carries stub commands that create the two files with the same document it replies with). Extend interactive-return-document (FR4 block, title block, changed_files with allow [], three appended judge checks, stub.judge of six yes), interactive-single-decision (Other option check for turns 1 and 2), interactive-option-rules (Other option check) and multi-issue-split (FR4 block, title block, kebab-case keys in the turn-1 proposal and in the Out of Scope entries of the draft and the returned document in the reference transcript, two appended judge checks, stub.judge of five yes). Keep every existing check and its order; append new judge checks after the existing ones. A case that returns no document (interactive-path-only-message, interactive-decline-implementation) must carry no output_document_valid check. Put a YAML comment before each new check naming the requirement (FR1, FR4, B-number)."
    dependencies: ["suite-tests-red"]
    acceptance_criteria:
      - "The three new case files exist, each with name equal to its file name and agent planner"
      - "interactive-document-handoff has: file_exists and file_contains (?i)\\A\\s*yes\\s*\\z on .kairon/sessions/eval-handoff.label; file_exists and file_contains result: canonical on .kairon/sessions/eval-handoff.document.yaml; file_not_contains for provenance/hash/sha256 on the document file; changed_files allowing exactly the two session paths; a judge with files listing the document file; the label-turn judge; the gh_log and output write guards; the envelope block with output_document_valid; the FR4 block; the title block"
      - "interactive-document-handoff's reference transcript creates the label file with yes and a document file whose content equals the final reply document"
      - "interactive-path-only-message and interactive-decline-implementation have changed_files with allow [] and no output_document_valid check"
      - "interactive-return-document keeps its first three judge checks in place, has three more judge checks after them, and stub.judge has six yes entries; multi-issue-split has five judge checks and stub.judge has five yes entries"
      - "multi-issue-split's turn-1 reference reply gives every remaining issue a backticked kebab-case key, each list item stays on one line, and Out of Scope entries in the draft and the returned document name the later issues by key"
      - "Other-option checks exist in interactive-single-decision for turn 1 and turn 2 and in interactive-option-rules"
      - "No judge question in the new or changed checks contains evidence, grounded, real quote or real file"
      - "A stub run of all interactive cases passes"
    validation_commands:
      - "go run ./cmd/kairon eval --list planner 2>&1 | grep -F interactive-document-handoff"
      - "go run ./cmd/kairon eval --list planner 2>&1 | grep -F interactive-path-only-message"
      - "go run ./cmd/kairon eval --list planner 2>&1 | grep -F interactive-decline-implementation"
      - |-
        T=$(mktemp -d) && cp -R .kairon/evals "$T/evals" && rm -rf "$T/evals/results" && go run ./cmd/kairon eval --backend stub --evals-dir "$T/evals" planner --filter "name==interactive-single-decision,interactive-option-rules,interactive-return-document,multi-issue-split,interactive-document-handoff,interactive-path-only-message,interactive-decline-implementation"

  - id: "legacy-transcripts"
    agent: "builder"
    description: "Add one legacy (pre-contract) transcript for each new case in internal/eval/testdata/planner-legacy/<case>.txt, turns separated by the existing separator line. Each transcript must have no stage-output document (no line starting schema_version, stage or result:), must make its case fail (several questions in one turn, a User Story draft, gh issue create, no files written, no dependencies list), and must keep output_document_validity and two_mode_behavior below the 60 percent floor across the whole suite. If a document-free reply raises the two_mode_behavior total too much, move that new check to conversation_protocol or gate_compliance in the case file instead of changing the test."
    dependencies: ["automated-cases", "interactive-cases"]
    acceptance_criteria:
      - "internal/eval/testdata/planner-legacy/ holds eleven .txt files, one per planner case"
      - "go test ./internal/eval -run TestPlannerSuite -count=1 passes, including TestPlannerSuiteRegistration, TestPlannerSuiteGreenReferenceTranscriptsPass, TestPlannerSuiteRedLegacyTranscriptsFail, TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments, TestPlannerSuiteStubDocumentsAreValid and TestPlannerSuiteDocumentCasesUseParseBackedCheck"
      - "go test ./internal/planner -run 'TestBoundaryReferenceCanonicalRepliesCommit|TestBoundaryReferencePunchOutReplyPunchesOut|TestSameContractReferenceRepliesParseToPlannerDocuments|TestExtractReferenceDocuments' -count=1 passes"
      - "The sentinel states which test failed first (from suite-tests-red) and that it now passes"
    validation_commands:
      - "test $(ls internal/eval/testdata/planner-legacy/*.txt | wc -l) -eq 11"
      - "go test ./internal/eval -run 'TestPlannerSuite|TestPlannerEvidence|TestShippedCaseWorkspacesAreMirroredInTemplates' -count=1"
      - "go test ./internal/planner -run 'TestBoundaryReferenceCanonicalRepliesCommit|TestBoundaryReferencePunchOutReplyPunchesOut|TestSameContractReferenceRepliesParseToPlannerDocuments|TestExtractReferenceDocuments' -count=1"

  - id: "driver-script"
    agent: "builder"
    description: "Update scripts/eval-planner.sh so INTERACTIVE_CASES is interactive-single-decision,interactive-option-rules,interactive-return-document,multi-issue-split,interactive-document-handoff,interactive-path-only-message,interactive-decline-implementation and AUTOMATED_CASES is automated-best-effort-fill,automated-bug-classification,automated-punch-out,automated-dependencies-line, with the header comment updated to match. The strings must equal the constants in internal/planner/evaldriver_test.go. Do not change the manifest rename logic, because the baseline TestEvalDriverDryRun failure at evaldriver_test.go:101 comes from the uncommitted .kiro/agents/*.json changes and is out of scope."
    dependencies: ["automated-cases", "interactive-cases"]
    acceptance_criteria:
      - "bash scripts/eval-planner.sh --dry-run prints the exact interactive filter and the exact automated filter from evaldriver_test.go"
      - "TestEvalDriverOverlayAndResults and TestEvalDriverExitsNonZeroWhenEitherHalfFails pass"
      - "TestEvalDriverDryRun fails only at its baseline line (evaldriver_test.go:101, the overlay manifest rename message), with no failure at the two filter checks (lines 89 and 95)"
    validation_commands:
      - "bash scripts/eval-planner.sh --dry-run | grep -F \"name==interactive-single-decision,interactive-option-rules,interactive-return-document,multi-issue-split,interactive-document-handoff,interactive-path-only-message,interactive-decline-implementation\""
      - "bash scripts/eval-planner.sh --dry-run | grep -F \"name==automated-best-effort-fill,automated-bug-classification,automated-punch-out,automated-dependencies-line\""
      - "go test ./internal/planner -run 'TestEvalDriverOverlayAndResults|TestEvalDriverExitsNonZeroWhenEitherHalfFails' -count=1"
      - "! go test ./internal/planner -run TestEvalDriverDryRun -count=1 2>&1 | grep -E 'evaldriver_test.go:(89|95):'"

  - id: "docs"
    agent: "builder"
    description: "Update docs/evaluation.md Planner suite section: seven cases becomes eleven everywhere it appears, the Two agents one suite table and the two kairon eval --filter command strings, the Cases and criteria table (add rows for the four new cases with the criteria they feed, and update the criteria of the changed cases), the sentence about a bare kairon eval --sandbox planner failing the three automated cases (now four), and the offline-proof bullet that the suite registers all seven cases. Add a short subsection that names the FR1 to FR4 output checks (hand-off files, punch-out, inference evidence, Dependencies line) and the .kairon/sessions/eval-*.document.yaml and .label convention. Update the case count and half table in docs/planner.md Eval driver. Do not edit .kairon/iterations, .kairon/specs, .kairon/research or CHANGELOG.md, and do not edit Known gaps entries."
    dependencies: ["driver-script", "automated-cases", "interactive-cases"]
    acceptance_criteria:
      - "grep finds no remaining 'seven cases' or 'all seven' about the planner suite in docs/evaluation.md or docs/planner.md"
      - "The half tables in both docs list exactly the case names in scripts/eval-planner.sh"
      - "The Cases and criteria table has one row per case file in .kairon/evals/cases/planner/ (eleven rows)"
      - "The new subsection names the case files for FR1 to FR4"
    validation_commands:
      - "! grep -n -i -E 'seven cases|all seven' docs/evaluation.md docs/planner.md"
      - "test $(grep -c -E '^\\| `(interactive|automated)-[a-z-]+\\.yaml` \\|' docs/evaluation.md) -ge 10"
      - "for f in .kairon/evals/cases/planner/*.yaml; do n=$(basename \"$f\"); grep -q -F \"$n\" docs/evaluation.md || { echo \"missing in docs/evaluation.md: $n\"; exit 1; }; done"

  - id: "sync-templates"
    agent: "builder"
    description: "Copy the changed evals into the templates using only the commands in the spec's Template sync section: the find command for fixtures (files only), mkdir -p and cp -r for cases. Do not run any command that copies .kiro/agents. Verify with the eval-scoped diff -rq from the Taskfile and with task sync:check, where only .kiro/agents/*.json lines may differ in this worktree."
    dependencies: ["automated-cases", "interactive-cases"]
    acceptance_criteria:
      - "cmd/kairon/templates/kairon/evals/cases/planner/ holds the same eleven case files as .kairon/evals/cases/planner/, byte for byte"
      - "cmd/kairon/templates/kairon/evals/fixtures/planner-intake-dependent-issue.md exists and equals the live file"
      - "The eval-scoped diff -rq exits 0"
      - "task sync:check reports no difference except .kiro/agents/*.json lines (the pre-existing uncommitted pipeline modifications), and git status shows no change to cmd/kairon/templates/kiro/agents/"
    validation_commands:
      - "diff -rq --exclude=results --exclude=.DS_Store --exclude=tmp --exclude=workspaces --exclude=hidden .kairon/evals/ cmd/kairon/templates/kairon/evals/"
      - "test $(ls cmd/kairon/templates/kairon/evals/cases/planner/*.yaml | wc -l) -eq 11"
      - "! task sync:check 2>&1 | grep -E '^(Files|Only in) ' | grep -v -E '^Files \\.kiro/agents/[a-z-]+\\.json and cmd/kairon/templates/kiro/agents/[a-z-]+\\.json differ$'"
      - "git diff --quiet HEAD -- cmd/kairon/templates/kiro/agents"

  - id: "prove-checks-can-fail"
    agent: "builder"
    description: "Prove that each new check family can fail. In a temp directory (not committed) write, for each family, the check list copied from the case file and a positive sample (the reference reply or file) and a negative sample, then run go run ./cmd/kairon check --checks <file> --dir <dir> --output <sample> --backend stub. Families: FR4 end-of-body line (negative: Dependencies line followed by more indented body text; Dependencies: #12 and #15), FR4 exactly-one line (negative: two Dependencies: lines), FR4 list-item carrier (negative: '- Dependencies: #12'), title length (negative: a 71-character title), FR3 per-entry evidence (negative: an inference with no evidence line; evidence: n/a; an empty evidence value), FR3 at-least-one inference, the dependency-specific checks of automated-dependencies-line (negative: Dependencies: #512 only), and the FR1 hand-off checks (negative directory: label file containing no, missing document file, an extra changed file). Every positive sample must PASS and every negative sample must FAIL on the intended check. Fix a pattern in the case file and re-run if a negative sample passes or a positive sample fails."
    dependencies: ["automated-cases", "interactive-cases"]
    acceptance_criteria:
      - "The sentinel lists each family with its positive result (PASS) and its negative result (FAIL on the intended check, with the check label printed by kairon check)"
      - "Every pattern in the FR3, FR4 and title blocks was run against all eleven reference replies that return a document, and none of the document-returning reference replies fails any of them"
      - "No temp sample file is left in the repository (git status shows no untracked file from this task)"
    validation_commands:
      - "go run ./cmd/kairon eval --list planner"
      - "! git status --porcelain | grep -E '^\\?\\? ' | grep -v -E '(\\.kairon/evals/cases/planner/|\\.kairon/evals/fixtures/planner-intake-dependent-issue\\.md|internal/eval/testdata/planner-legacy/|cmd/kairon/templates/kairon/evals/|\\.kairon/specs/issue-418)'"

  - id: "test-eval-mapping"
    agent: "builder"
    description: "Produce the FR5 mapping from the final case files. Re-read each of the 40 tests listed in the spec's mapping table, confirm or correct its disposition, and pick a literal key_text that appears in the named case file. Write .kairon/artifacts/issue-418-test-mapping.tsv, .kairon/artifacts/issue-418-fr-checks.tsv and .kairon/artifacts/issue-418-pr-mapping.md in the formats given in the spec, and embed the FR table and the 40-row test table (one row per test) in the sentinel together with the lines 'Deferred work: none' and a 'Spec deviations' section (state 'Spec deviations: none' unless a deviation was recorded by another task). Record the behaviours with no eval check, with their reasons, in the Note column of the affected rows. For FR2 cite the existing automated-punch-out checks by key text."
    dependencies: ["legacy-transcripts", "driver-script", "docs", "sync-templates", "prove-checks-can-fail"]
    acceptance_criteria:
      - "issue-418-test-mapping.tsv has a header and, for each of the 37 tests listed in #419 and the three extra tests, at least one line"
      - "Every line with disposition covered names an existing file under .kairon/evals/cases/planner/ and a key_text that grep -F finds in it"
      - "issue-418-fr-checks.tsv has at least one line for each of FR1, FR2, FR3 and FR4, and every key_text is found in its case file"
      - "issue-418-pr-mapping.md has exactly one table row per test (37 rows for the #419 tests and 3 more rows for the kept code tests) and one row per requirement FR1 to FR4"
      - "Every test whose instruction has no eval check is marked 'covered (partly)' or 'no behaviour' with the reason in the Note column; no row is blank"
      - "The sentinel embeds both tables in full"
    validation_commands:
      - "test -s .kairon/artifacts/issue-418-test-mapping.tsv && test -s .kairon/artifacts/issue-418-fr-checks.tsv && test -s .kairon/artifacts/issue-418-pr-mapping.md"
      - |-
        tail -n +2 .kairon/artifacts/issue-418-test-mapping.tsv | while IFS="$(printf '\t')" read -r test disp casefile key note; do if [ "$disp" = covered ]; then grep -F -q -- "$key" ".kairon/evals/cases/planner/$casefile" || { echo "MISSING: $test $casefile $key"; exit 1; }; fi; done
      - |-
        tail -n +2 .kairon/artifacts/issue-418-fr-checks.tsv | while IFS="$(printf '\t')" read -r req casefile key; do grep -F -q -- "$key" ".kairon/evals/cases/planner/$casefile" || { echo "MISSING: $req $casefile $key"; exit 1; }; done

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion of issue #418 and of the spec. Run the validation commands from the spec. Check AC1 by reading the FR table and grep-checking each named key text in its case file. Check AC2 with the eval --list command (exit 0, eleven cases, each new case listed) and the offline stub run. Check AC3 with the eval-scoped diff and task sync:check (only .kiro/agents/*.json lines may differ here). Check AC4 by confirming the 37 test names listed in #419 each have a row in issue-418-pr-mapping.md and issue-418-test-mapping.tsv. Confirm no prompt, skill, rubric or .kiro/agents file was changed by this work, that automated-punch-out.yaml is unchanged, that the Go changes are limited to the three test files, and that go test ./internal/planner shows only the seven baseline failures. Review the spec scope: no In Scope item or criterion was narrowed and Deferred work is none. Report FAIL with Where, Quote, Why it fails and Required fix for each defect."
    dependencies: ["test-eval-mapping"]
    acceptance_criteria:
      - "go run ./cmd/kairon eval --list planner exits 0 and lists eleven cases, including automated-dependencies-line, interactive-document-handoff, interactive-path-only-message and interactive-decline-implementation"
      - "The offline stub run of the whole planner suite exits 0"
      - "go test ./internal/eval -run 'TestPlannerSuite|TestPlannerEvidence|TestShippedCaseWorkspacesAreMirroredInTemplates' -count=1 passes"
      - "go test ./internal/planner -count=1 fails only in TestAutomatedManifest, TestAutomatedManifestHasNoInjectedOrWriteCapabilities, TestAutomatedFilesMatchTemplates, TestEvalDriverDryRun, TestInteractiveManifest, TestInteractiveManifestHasNoInjectedCapabilities and TestInteractiveFilesMatchTemplates, all caused by the uncommitted .kiro/agents/*.json changes"
      - "The eval-scoped diff -rq exits 0 and task sync:check differs only on .kiro/agents/*.json"
      - "The mapping covers all 37 tests of #419, each named check exists in its case file, and FR1 to FR4 each name a case file and a check that exists"
      - "git diff against HEAD touches none of: .kiro/agents/*-prompt.md, .kiro/skills/, .kairon/evals/rubrics/, .kairon/evals/cases/planner/automated-punch-out.yaml"
      - "No source file under internal/ or cmd/ other than the three named _test.go files is modified"
    validation_commands:
      - "go test -count=1 -run '^$' ./internal/eval ./internal/planner"
      - "go run ./cmd/kairon eval --list planner"
      - |-
        T=$(mktemp -d) && cp -R .kairon/evals "$T/evals" && rm -rf "$T/evals/results" && go run ./cmd/kairon eval --backend stub --evals-dir "$T/evals" planner
      - "go test ./internal/eval -run 'TestPlannerSuite|TestPlannerEvidence|TestShippedCaseWorkspacesAreMirroredInTemplates' -count=1"
      - "! go test ./internal/planner -count=1 2>&1 | grep -E '^--- FAIL: ' | grep -v -E '^--- FAIL: (TestAutomatedManifest|TestAutomatedManifestHasNoInjectedOrWriteCapabilities|TestAutomatedFilesMatchTemplates|TestEvalDriverDryRun|TestInteractiveManifest|TestInteractiveManifestHasNoInjectedCapabilities|TestInteractiveFilesMatchTemplates) '"
      - "diff -rq --exclude=results --exclude=.DS_Store --exclude=tmp --exclude=workspaces --exclude=hidden .kairon/evals/ cmd/kairon/templates/kairon/evals/"
      - "! task sync:check 2>&1 | grep -E '^(Files|Only in) ' | grep -v -E '^Files \\.kiro/agents/[a-z-]+\\.json and cmd/kairon/templates/kiro/agents/[a-z-]+\\.json differ$'"
      - "git diff --quiet HEAD -- .kiro/skills .kairon/evals/rubrics .kairon/evals/cases/planner/automated-punch-out.yaml .kiro/agents/planner-prompt.md .kiro/agents/planner-automated-prompt.md"
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-418-cover-planner-prompt-behaviour-with-eval-cases.md"
      - "go run ./cmd/kairon scope check --input \"$KAIRON_INPUT_DOCUMENT\" --spec .kairon/specs/issue-418-cover-planner-prompt-behaviour-with-eval-cases.md"
```

## PR Description Requirements

The PR description for this work carries (krew-lead assembles it from the sentinels):

- `Closes #418`
- The FR1 to FR4 table (case file and check for each), from `issue-418-pr-mapping.md`
- The 40-row test table, one row per test
- The name of the test that failed first (from `suite-tests-red`) and the tests added or relied on
- `Deferred work: none`
- `Spec deviations: none`, unless a builder or validator recorded a deviation
- A note that `task sync:check` and seven `internal/planner` tests fail in the pipeline worktree only because of the uncommitted `.kiro/agents/*.json` modifications, and pass on the clean branch
- A line listing the changes outside the two In Scope paths that adding cases required: the three `_test.go` case lists, four legacy transcripts, one fixture, `scripts/eval-planner.sh`, `docs/evaluation.md` and `docs/planner.md`
