# Design Spec: Delete tests that pin documentation

Closes #417

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/boundary`, `internal/checks`, `internal/failure`, `internal/guard`, `internal/provenance`, `internal/stageoutput`: their `docs_test.go` / docs tests listed below" | `delete-docs-tests-small-packages`, `stageoutput-docs-and-testdata` |
| S2 | In Scope: "`internal/eval`: `workflow_docs_test.go`, `simulated_user_docs_test.go`" | `delete-eval-docs-tests` |
| S3 | In Scope: "`internal/stageoutput`: `TestParse_CanonicalExampleFromSpec` and `TestParse_PunchOutExampleFromSpec` read their documents from `internal/stageoutput/testdata/` (new files)" | `stageoutput-docs-and-testdata` |
| AC1 | "None of the 24 deleted tests exists." | `delete-docs-tests-small-packages`, `delete-eval-docs-tests`, `stageoutput-docs-and-testdata`, `validate-all` |
| AC2 | "No test opens a file under `docs/` or `.kairon/research/` by a relative path." | `delete-docs-tests-small-packages`, `delete-eval-docs-tests`, `stageoutput-docs-and-testdata`, `validate-all` |
| AC3 | "The two stageoutput parser tests pass and read their input from `testdata/`." | `stageoutput-docs-and-testdata` (`TestParse_CanonicalExampleFromSpec`, `TestParse_PunchOutExampleFromSpec`), `validate-all` |
| AC4 | "Package coverage does not drop." | `delete-docs-tests-small-packages`, `delete-eval-docs-tests`, `stageoutput-docs-and-testdata`, `validate-all` |
| AC5 | "The gate passes." | `validate-all` |

## Deferred work

none

## Problem

26 tests open files under `docs/` or `.kairon/research/` and assert on their contents (required words, required sections, YAML examples that must parse). The maintainer's rule (2026-10-28) is that no test pins documentation. 24 are deleted. The other 2 (`TestParse_CanonicalExampleFromSpec`, `TestParse_PunchOutExampleFromSpec`) test real parser behaviour, so they stay and read their input from new `testdata/` files.

No production code changes. Nothing under `docs/` or `.kairon/research/` is edited (both are out of scope; the research files that mention these tests are historical analysis and stay as written).

## Solution Approach

Pure test-code deletion plus one input relocation. The architecture investigation found the following in the current tree, all verified by reading the files:

### Whole files that go away

Every test in these files is on the delete list, and every helper in them is used only by those tests (grep for `docPath`, `readDoc`, `inputFence`, `yamlFence`, `yamlInvalidFence`, `docInput`, `allCheckTypes`, `documentedChecks`, `simulatedUserDocCase` outside these files finds no other user in the same package):

| File | Deleted tests | Helpers that go with it |
|------|---------------|-------------------------|
| `internal/boundary/docs_test.go` | `TestDocs_NamesRequiredTerms`, `TestDocs_InputExamplesParse` | `docPath`, `inputFence`, `readDoc` |
| `internal/failure/docs_test.go` | `TestDocs_NamesRequiredTerms`, `TestDocs_HasNoYAMLFence` | `docPath`, `readDoc` |
| `internal/guard/docs_test.go` | `TestDocs_NamesRequiredTerms`, `TestDocs_ValidYAMLExamplesAreAdmitted`, `TestDocs_InvalidYAMLExamplesAreRejected` | `docPath`, `yamlFence`, `yamlInvalidFence`, `readDoc`, `docInput` |
| `internal/provenance/docs_test.go` | `TestDocs_NamesRequiredTerms`, `TestDocs_ExamplesClassifyAsLabelled`, `TestDocs_StatesHashIsNotContentTrust` | `docPath`, `validFence`, `noneFence`, `mismatchFence`, `readDoc` |
| `internal/stageoutput/docs_test.go` | `TestDocs_NamesRequiredTerms`, `TestDocs_ValidYAMLExamplesParse`, `TestDocs_InvalidExamplesAreRejected` | `docPath`, `yamlFence`, `readDoc` |
| `internal/checks/checks_docs_test.go` | `TestChecksDocsCoversEveryCheckType`, `TestChecksDocsExamplesAreValid`, `TestChecksDocsStatesKeyRules` | `allCheckTypes`, `yamlFence`, `docYAMLBlocks`, `documentedChecks`, `documentedCheck`, `readEvaluationDoc` |
| `internal/eval/simulated_user_docs_test.go` | `TestSimulatedUserDocExampleLoads`, `TestSimulatedUserDocStatesKeyRules` | `simulatedUserDocCase` |

Delete each file with `git rm`. A builder must not leave an empty `package x` file behind.

### `internal/eval`: partial file edit plus orphaned shared helpers

This is the trap the task description asks about. `go build` and `go vet` do not flag unused functions, variables or package-level declarations in test files, so the leftovers compile silently. They must still be removed, because they either read documentation (`readEvaluationDoc`) or exist only to serve deleted tests.

1. `internal/eval/workflow_docs_test.go`: delete `TestWorkflowDocsSectionExists`, `TestWorkflowDocsCallRecordTable`, `TestWorkflowDocsStepRecordsFormat`, `TestWorkflowDocsSubAgentProvenance`, `TestWorkflowDocsWorkflowSHA256`, `TestWorkflowDocsResumeAndPromptHash`. Also delete what only they used: the `wfDocHeading` / `wfDocEndMarker` constants, `wfDocSlice`, `wfDocRequire`, and the file-top comment that says "These tests pin the terms an eval author depends on". Keep `TestWorkflowDocsAlgorithmMatchesCode` unchanged. It uses `crypto/sha256`, `encoding/hex`, `strconv`, `strings` and `testing`, so the import block stays as is.
2. `internal/eval/checks_helpers_test.go`: `docYAMLBlocks`, `readEvaluationDoc` and the `yamlFence` variable are used only by the deleted eval tests. Remove all three. `readEvaluationDoc` opens `../../docs/evaluation.md` through `filepath.Join("..", "..", "docs", "evaluation.md")`, which acceptance criterion 2's regex does not match but which the issue's goal ("no test reads documentation") covers. After removal, `regexp` is unused in that file: drop that import. `os`, `os/exec`, `path/filepath` and `strings` are still used by `writeEvalFile`, `chkGit` and `wantFail`.
3. `repoRoot` in `internal/eval/selftest_sandbox_test.go` stays: other sandbox tests use it.
4. `loadTurnsCase` (in `turns_schema_test.go`) stays: many tests use it.

### `internal/stageoutput`: move the two parser inputs to `testdata/`

`internal/stageoutput/parse_test.go` has a helper `specFence(t, marker)` that reads `../../.kairon/research/maturity-model/planner-requirements.md` and returns the first ```` ```yaml ```` fence after a marker. Replace it with `readTestdata(t, name)` that reads `testdata/<name>`, and create two files:

- `internal/stageoutput/testdata/canonical-example.yaml` is the fence after `#### 3.1.1 Canonical envelope shape`, copied verbatim (including its comments), with a trailing newline.
- `internal/stageoutput/testdata/punch-out-example.yaml` is the fence after ``A complete `result: punch_out` document:``, copied verbatim, with a trailing newline.

Content of `canonical-example.yaml`:

```yaml
# A stage-output document: a YAML mapping carrying a metadata envelope plus a body.
schema_version: 1                 # integer, REQUIRED. The output-contract version; ties to the §3.4 rule.
stage: planner                    # string enum, REQUIRED. One of:
                                  #   planner | architect | krew-lead | builder | validator | documenter
result: canonical                 # string enum, REQUIRED. canonical | punch_out (see §3.5).
source:                           # mapping, REQUIRED. Identifies the input this output was derived from.
  kind: github_issue              #   string enum, REQUIRED: github_issue | document
  ref: "142"                      #   string, REQUIRED: the issue number, or the upstream document's identity
generated: 2026-10-20T20:00:00Z   # string, REQUIRED. ISO-8601 / RFC 3339 timestamp, UTC.
body: |                           # string, REQUIRED when result=canonical. The substantive content, block scalar.
  ## Problem
  ...the canonical issue text per §5-§8...
inferences:                       # list, OPTIONAL. Present when automated intake filled slots by inference (§10.2).
  - slot: edge_cases              #   string, REQUIRED: the slot that was filled by inference.
    inferred: handle empty input as a no-op          # string, REQUIRED: what was inferred.
    evidence: existing exporters in internal/export/*.go all treat empty input as a no-op  # string, REQUIRED.
```

Content of `punch-out-example.yaml`:

```yaml
schema_version: 1
stage: planner
result: punch_out
source:
  kind: github_issue
  ref: "142"
generated: 2026-10-20T20:00:00Z
unresolved:
  - slot: acceptance_criteria
    reason: >-
      The issue states the goal ("make export faster") but gives no measurable target;
      a materially different acceptance criterion follows from each plausible target.
  - slot: scope.external_contract
    reason: >-
      Whether the v2 API must stay backward compatible is a product decision the
      repository cannot settle.
```

The two files must match the research document byte for byte so the characterization is exact. Extract them from the research document with the script in the task below instead of retyping, and compare against the blocks above. If the research document has changed since this spec was written, the blocks above are authoritative.

The test names stay `TestParse_CanonicalExampleFromSpec` and `TestParse_PunchOutExampleFromSpec` (the issue and acceptance criterion 3 name them), and every `assert`/`require` line in both is unchanged. Only the line that obtains `in` changes. In `parse_test.go`, `regexp` was used only by `specFence`; replace it with `path/filepath` in the import block (`os` is still used by `readTestdata`). Update the doc comment above the tests (it should say the inputs are copies of the design document's examples, kept in `testdata/`; do not cite a research path as something the test reads).

`internal/stageoutput/docs_test.go` is deleted in the same task, so the package has one owner for all its edits.

### Measured effect (architect dry run)

The deletions and the stageoutput change were applied in a scratch copy of the tree (`/tmp/k417`, not in the worktree) and measured. Baseline in the worktree and after the change are identical to the floors:

| Package | Floor | Baseline | After change |
|---------|-------|----------|--------------|
| `internal/boundary` | 96.2 | 96.2 | 96.2 |
| `internal/checks` | 83.8 | 83.8 | 83.8 |
| `internal/failure` | 98.1 | 98.1 | 98.1 |
| `internal/guard` | 95.0 | 98.1 | 98.1 |
| `internal/provenance` | 95.0 | 95.0 | 95.0 |
| `internal/stageoutput` | 94.9 | 94.9 | 94.9 |
| `internal/eval` | 87.6 | 87.6 | 87.6 |

Note on `internal/eval`: a full run of that package takes about 330 s (the scratch run needed a git repository, because some eval tests call `git rev-parse`). `TestSimulatedUserDocStatesKeyRules` also asserted `(TestCase{SimulatedUser: &SimulatedUser{}}).maxTurns() == 10`; coverage stays at 87.6% without it. If a builder's own run of `go test -cover ./internal/eval` ever reports below 87.6, that is a blocker to report (do not add tests: "Any other test change" is out of scope), not something to hide.

The two parser tests passed from `testdata/` in the scratch copy, `gofmt -l` and `go vet ./internal/...` were clean, and the AC2 grep printed nothing.

## Relevant Files

Delete:
- `internal/boundary/docs_test.go`
- `internal/failure/docs_test.go`
- `internal/guard/docs_test.go`
- `internal/provenance/docs_test.go`
- `internal/stageoutput/docs_test.go`
- `internal/checks/checks_docs_test.go`
- `internal/eval/simulated_user_docs_test.go`

Modify:
- `internal/eval/workflow_docs_test.go` (keep only `TestWorkflowDocsAlgorithmMatchesCode`)
- `internal/eval/checks_helpers_test.go` (remove `yamlFence`, `docYAMLBlocks`, `readEvaluationDoc`, the `regexp` import)
- `internal/stageoutput/parse_test.go` (`specFence` becomes `readTestdata`; import `regexp` becomes `path/filepath`)

Create:
- `internal/stageoutput/testdata/canonical-example.yaml`
- `internal/stageoutput/testdata/punch-out-example.yaml`

Do not touch: `docs/`, `.kairon/research/`, `.kairon/specs/` (other specs), any non-test file, and the unrelated uncommitted `.kiro/agents/*.json` changes in the worktree (stage files by path; never `git add -A`).

`cmd/kairon/templates/` has no copy of any of these files, so `task sync:check` is unaffected.

## Team Orchestration

Three builder tasks touch disjoint files and run in parallel. One validator task runs after all three. Commit all three builder results together on the issue branch as one commit (a test deletion plus its helpers must not leave a commit with a red build); commit message: `test: Delete tests that pin documentation`, body names the 24 deleted tests, says the two parser tests were relied on as characterization (they passed before from the research doc and pass after from `testdata/`), and says `Deferred work: none` and `Spec deviations: none`. This is a deletion, so the TDD "failing test first" rule does not apply; the refactor rule applies to the parser tests only and is met by the before/after pass.

The PR description needs `Deferred work: none` and `Spec deviations: none`.

## Step-by-Step Task Breakdown

### Task 1: `delete-docs-tests-small-packages`
Delete the five-package set `boundary`, `failure`, `guard`, `provenance` docs files and `checks/checks_docs_test.go` with `git rm`. No other edit in these packages is needed. Run each package's tests, `go vet`, and `-cover`. Dependencies: none.

### Task 2: `delete-eval-docs-tests`
`git rm internal/eval/simulated_user_docs_test.go`; reduce `workflow_docs_test.go` to `TestWorkflowDocsAlgorithmMatchesCode` and remove its orphaned constants and helpers; remove `yamlFence`, `docYAMLBlocks`, `readEvaluationDoc` and the `regexp` import from `checks_helpers_test.go`. Verify no remaining reference with grep. Run `go vet ./internal/eval` and `gofmt -l`. Dependencies: none.

### Task 3: `stageoutput-docs-and-testdata`
`git rm internal/stageoutput/docs_test.go`; create the two `testdata` files; change `parse_test.go` as described. Dependencies: none.

### Task 4: `validate-all`
Read-only verification of every acceptance criterion, including coverage of all seven packages and the full `task test`. Dependencies: tasks 1 to 3.

## Validation Commands

Run from the repository root.

```bash
# AC1: all print nothing
grep -nE 'func (TestDocs_InputExamplesParse|TestDocs_NamesRequiredTerms)\(' internal/boundary/*_test.go
grep -nE 'func (TestChecksDocsCoversEveryCheckType|TestChecksDocsExamplesAreValid|TestChecksDocsStatesKeyRules)\(' internal/checks/*_test.go
grep -nE 'func (TestSimulatedUserDocExampleLoads|TestSimulatedUserDocStatesKeyRules|TestWorkflowDocsCallRecordTable|TestWorkflowDocsResumeAndPromptHash|TestWorkflowDocsSectionExists|TestWorkflowDocsStepRecordsFormat|TestWorkflowDocsSubAgentProvenance|TestWorkflowDocsWorkflowSHA256)\(' internal/eval/*_test.go
grep -nE 'func (TestDocs_HasNoYAMLFence|TestDocs_NamesRequiredTerms)\(' internal/failure/*_test.go
grep -nE 'func (TestDocs_InvalidYAMLExamplesAreRejected|TestDocs_NamesRequiredTerms|TestDocs_ValidYAMLExamplesAreAdmitted)\(' internal/guard/*_test.go
grep -nE 'func (TestDocs_ExamplesClassifyAsLabelled|TestDocs_NamesRequiredTerms|TestDocs_StatesHashIsNotContentTrust)\(' internal/provenance/*_test.go
grep -nE 'func (TestDocs_InvalidExamplesAreRejected|TestDocs_NamesRequiredTerms|TestDocs_ValidYAMLExamplesParse)\(' internal/stageoutput/*_test.go
# kept test still present
grep -n 'func TestWorkflowDocsAlgorithmMatchesCode(' internal/eval/workflow_docs_test.go

# AC2: prints nothing (both greps)
grep -rnE '"(\.\./)+(docs|\.kairon/research)/' --include='*_test.go' internal cmd
grep -rnE 'readEvaluationDoc|docYAMLBlocks|wfDocSlice|wfDocRequire|specFence|"docs", "evaluation' internal --include='*_test.go'

# AC3
go test -run '^TestParse_(CanonicalExampleFromSpec|PunchOutExampleFromSpec)$' -v ./internal/stageoutput
ls internal/stageoutput/testdata/

# AC4: compare each to its floor
go test -cover ./internal/boundary ./internal/checks ./internal/failure ./internal/guard ./internal/provenance ./internal/stageoutput
go test -cover ./internal/eval    # about 330 s

# AC5 and hygiene
gofmt -l internal cmd             # prints nothing
go vet ./...
task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "delete-docs-tests-small-packages"
    agent: "builder"
    description: "git rm the docs test files in internal/boundary, internal/failure, internal/guard, internal/provenance (docs_test.go in each) and internal/checks/checks_docs_test.go. Every test and helper in these files is on the issue's delete list or used only by them. Do not edit anything else in these packages and do not touch docs/ or .kairon/research/."
    dependencies: []
    acceptance_criteria:
      - "internal/boundary/docs_test.go, internal/failure/docs_test.go, internal/guard/docs_test.go, internal/provenance/docs_test.go and internal/checks/checks_docs_test.go no longer exist"
      - "The 13 tests TestDocs_InputExamplesParse and TestDocs_NamesRequiredTerms (boundary), TestDocs_HasNoYAMLFence and TestDocs_NamesRequiredTerms (failure), TestDocs_InvalidYAMLExamplesAreRejected, TestDocs_NamesRequiredTerms and TestDocs_ValidYAMLExamplesAreAdmitted (guard), TestDocs_ExamplesClassifyAsLabelled, TestDocs_NamesRequiredTerms and TestDocs_StatesHashIsNotContentTrust (provenance), TestChecksDocsCoversEveryCheckType, TestChecksDocsExamplesAreValid and TestChecksDocsStatesKeyRules (checks) are gone, and the per-package grep from the issue prints nothing"
      - "go vet and go test pass for internal/boundary, internal/checks, internal/failure, internal/guard and internal/provenance"
      - "Coverage is at least 96.2% for boundary, 83.8% for checks, 98.1% for failure, 95.0% for guard and 95.0% for provenance"
      - "No test file in these five packages references docs/ or .kairon/research/ by a relative path"
      - "docs/ and .kairon/research/ are unchanged"
    validation_commands:
      - "test -z \"$(ls internal/boundary/docs_test.go internal/failure/docs_test.go internal/guard/docs_test.go internal/provenance/docs_test.go internal/checks/checks_docs_test.go 2>/dev/null)\""
      - "test -z \"$(grep -rnE 'func (TestDocs_|TestChecksDocs)' internal/boundary internal/checks internal/failure internal/guard internal/provenance --include='*_test.go')\""
      - "go vet ./internal/boundary ./internal/checks ./internal/failure ./internal/guard ./internal/provenance"
      - "go test -cover ./internal/boundary ./internal/checks ./internal/failure ./internal/guard ./internal/provenance"
      - "test -z \"$(gofmt -l internal/boundary internal/checks internal/failure internal/guard internal/provenance)\""

  - id: "delete-eval-docs-tests"
    agent: "builder"
    description: "In internal/eval: git rm simulated_user_docs_test.go; reduce workflow_docs_test.go to TestWorkflowDocsAlgorithmMatchesCode by deleting the six phrase-pinning TestWorkflowDocs* tests plus the now-unused wfDocHeading and wfDocEndMarker constants, wfDocSlice, wfDocRequire and the file-top comment about pinning terms; in checks_helpers_test.go remove the yamlFence variable, docYAMLBlocks, readEvaluationDoc and the regexp import. Keep repoRoot and loadTurnsCase. go vet does not flag unused test helpers, so verify with grep."
    dependencies: []
    acceptance_criteria:
      - "internal/eval/simulated_user_docs_test.go no longer exists"
      - "The eight deleted eval tests (TestSimulatedUserDocExampleLoads, TestSimulatedUserDocStatesKeyRules, TestWorkflowDocsCallRecordTable, TestWorkflowDocsResumeAndPromptHash, TestWorkflowDocsSectionExists, TestWorkflowDocsStepRecordsFormat, TestWorkflowDocsSubAgentProvenance, TestWorkflowDocsWorkflowSHA256) are gone"
      - "TestWorkflowDocsAlgorithmMatchesCode is still in internal/eval/workflow_docs_test.go with its body unchanged and passes"
      - "No identifier readEvaluationDoc, docYAMLBlocks, yamlFence, wfDocSlice, wfDocRequire, wfDocHeading, wfDocEndMarker or simulatedUserDocCase remains in internal/eval"
      - "No eval test file opens docs/ or .kairon/research/ (including via filepath.Join with separate path elements)"
      - "go vet ./internal/eval and gofmt -l internal/eval are clean"
      - "go test -cover ./internal/eval reports at least 87.6%; if it does not, stop and report it as a blocker instead of editing other tests"
    validation_commands:
      - "test ! -e internal/eval/simulated_user_docs_test.go"
      - "grep -n 'func TestWorkflowDocsAlgorithmMatchesCode(' internal/eval/workflow_docs_test.go"
      - "test -z \"$(grep -rnE 'readEvaluationDoc|docYAMLBlocks|yamlFence|wfDocSlice|wfDocRequire|wfDocHeading|wfDocEndMarker|simulatedUserDocCase' internal/eval)\""
      - "test -z \"$(grep -nE 'func (TestSimulatedUserDocExampleLoads|TestSimulatedUserDocStatesKeyRules|TestWorkflowDocsCallRecordTable|TestWorkflowDocsResumeAndPromptHash|TestWorkflowDocsSectionExists|TestWorkflowDocsStepRecordsFormat|TestWorkflowDocsSubAgentProvenance|TestWorkflowDocsWorkflowSHA256)\\(' internal/eval/*_test.go)\""
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go test -run 'TestWorkflowDocsAlgorithmMatchesCode|TestSimulatedUser|TestLoad' ./internal/eval"
      - "go test -cover ./internal/eval"

  - id: "stageoutput-docs-and-testdata"
    agent: "builder"
    description: "In internal/stageoutput: git rm docs_test.go (TestDocs_NamesRequiredTerms, TestDocs_ValidYAMLExamplesParse, TestDocs_InvalidExamplesAreRejected and their helpers). Create testdata/canonical-example.yaml and testdata/punch-out-example.yaml as verbatim copies (plus a trailing newline) of the yaml fences after '#### 3.1.1 Canonical envelope shape' and 'A complete `result: punch_out` document:' in .kairon/research/maturity-model/planner-requirements.md (contents are given in the spec; extract with a script rather than retyping). In parse_test.go replace specFence with readTestdata(t, name) reading testdata/<name>, point TestParse_CanonicalExampleFromSpec and TestParse_PunchOutExampleFromSpec at the new files, keep every assertion unchanged, replace the regexp import with path/filepath, and update the comment above the helper. Before editing, run the two parser tests to confirm they pass from the research doc (characterization baseline)."
    dependencies: []
    acceptance_criteria:
      - "internal/stageoutput/docs_test.go no longer exists and TestDocs_NamesRequiredTerms, TestDocs_ValidYAMLExamplesParse and TestDocs_InvalidExamplesAreRejected are gone"
      - "internal/stageoutput/testdata/ contains canonical-example.yaml and punch-out-example.yaml, equal to the fenced blocks in the research document"
      - "TestParse_CanonicalExampleFromSpec and TestParse_PunchOutExampleFromSpec keep their names and every assertion, read only from testdata/, and pass"
      - "parse_test.go contains no path under .kairon/research or docs/ and no unused import"
      - "go vet ./internal/stageoutput and gofmt -l internal/stageoutput are clean"
      - "go test -cover ./internal/stageoutput reports at least 94.9%"
      - ".kairon/research/ is unchanged"
    validation_commands:
      - "test ! -e internal/stageoutput/docs_test.go"
      - "ls internal/stageoutput/testdata/canonical-example.yaml internal/stageoutput/testdata/punch-out-example.yaml"
      - "test -z \"$(grep -nE 'research|docs/' internal/stageoutput/parse_test.go)\""
      - "go test -run '^TestParse_(CanonicalExampleFromSpec|PunchOutExampleFromSpec)$' -v ./internal/stageoutput"
      - "go vet ./internal/stageoutput"
      - "test -z \"$(gofmt -l internal/stageoutput)\""
      - "go test -cover ./internal/stageoutput"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of all five acceptance criteria and the behavior-preservation contract. Run the issue's grep checks, the relative-path grep and the orphaned-helper grep, run the two parser tests, run go test -cover on all seven packages and compare to the floors, check that git diff touches only the listed test files and testdata (no production code, no docs/, no .kairon/research/, no .kiro/agents/ changes attributable to the builders), and run task test. Report PASS or FAIL per criterion with the observed values."
    dependencies: ["delete-docs-tests-small-packages", "delete-eval-docs-tests", "stageoutput-docs-and-testdata"]
    acceptance_criteria:
      - "AC1: every grep from the issue's Verify line prints nothing, and TestWorkflowDocsAlgorithmMatchesCode still exists"
      - "AC2: the relative-path grep prints nothing, and no readEvaluationDoc, docYAMLBlocks, wfDocSlice, wfDocRequire or specFence identifier remains in any test file"
      - "AC3: the two parser tests exit 0 and ls internal/stageoutput/testdata/ lists canonical-example.yaml and punch-out-example.yaml"
      - "AC4: reported coverage is at least boundary 96.2, checks 83.8, eval 87.6, failure 98.1, guard 95.0, provenance 95.0, stageoutput 94.9"
      - "AC5: task test exits 0"
      - "The change set contains only deletions and edits of the listed *_test.go files plus the two new testdata files; no non-test Go file, docs/ file or .kairon/research/ file is modified; the plan, spec and PR description state Deferred work: none"
    validation_commands:
      - "test -z \"$(grep -rnE '\"(\\.\\./)+(docs|\\.kairon/research)/' --include='*_test.go' internal cmd)\""
      - "test -z \"$(grep -rnE 'readEvaluationDoc|docYAMLBlocks|wfDocSlice|wfDocRequire|specFence' internal --include='*_test.go')\""
      - "go test -run '^TestParse_(CanonicalExampleFromSpec|PunchOutExampleFromSpec)$' -v ./internal/stageoutput"
      - "go test -cover ./internal/boundary ./internal/checks ./internal/failure ./internal/guard ./internal/provenance ./internal/stageoutput"
      - "go test -cover ./internal/eval"
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
      - "task test"
```
