# Design Spec: Prune redundant internal/eval tests, part 1 of 5

Closes #423

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/eval`: `architecture_compatibility_test.go`, `attempts_test.go`, `audit_test.go`, `backend_routing_test.go`, `candidate_e2e_test.go`, `candidate_test.go`, `checks_judge_test.go`, `checks_only_test.go`, `checks_scoring_test.go`, `checks_test.go`, `config_test.go`" | `arch-move-and-prune`, `attempts-and-repeat`, `audit-and-cmd-audit`, `backend-routing-prune`, `candidate-e2e-merge`, `candidate-merge`, `checks-judge-and-only`, `checks-scoring-and-checks`, `config-prune` |
| S2 | In Scope: "`internal/eval`: `repeat_exec_test.go` and `pinning_test.go`, only to receive the merges whose partners are `TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals` and `TestSummaryMultiAgentOmitsTopLevelAgentFields`" | `attempts-and-repeat`, `candidate-merge` |
| S3 | In Scope: "`internal/eval/sandbox`: a test file (no build tag) that receives `TestDetectHostArchitecture`, moved from `internal/eval`'s `TestSandboxArchitectureDetection`" | `arch-move-and-prune` |
| S4 | In Scope: "`cmd/kairon/cmd`: `eval_audit_test.go`, only to receive the two audit merges whose partner is `TestAuditFlatFixtureEvidenceAndDuplicates`" | `audit-and-cmd-audit` |
| AC1 | "The 15 tests marked Delete no longer exist." | `arch-move-and-prune`, `attempts-and-repeat`, `audit-and-cmd-audit`, `backend-routing-prune`, `checks-judge-and-only`, `checks-scoring-and-checks`, `config-prune`, `validate-complete` |
| AC2 | "Each of the 13 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `attempts-and-repeat`, `audit-and-cmd-audit`, `candidate-e2e-merge`, `candidate-merge`, `checks-judge-and-only`, `checks-scoring-and-checks`, `validate-complete` |
| AC3 | "`TestSandboxArchitectureDetection` is moved to the sandbox package as `TestDetectHostArchitecture` and runs in the default build." | `arch-move-and-prune`, `validate-complete` |
| AC4 | "Package coverage does not drop." | `baseline-coverage`, `coverage-and-gate`, `validate-complete` |
| AC5 | "Every remaining test passes unchanged, and the gate passes." | `coverage-and-gate`, `validate-complete` |

## Deferred work

none

## Solution Approach

This is a test-only refactor of `internal/eval` (plus one receiving test file in `internal/eval/sandbox` and one in
`cmd/kairon/cmd`). No production code changes. Per AGENTS.md the existing tests are the characterization tests for a
refactor: no new failing test is written first. Instead each task keeps the touched package green and must keep every
assertion of every partner. The one add-before-delete ordering that matters is the sandbox move: the Delete rows for
the architecture tests name `sandbox.TestDetectHostArchitecture` as their partner, so that test is added to the sandbox
package before the eval tests that point at it are deleted, all in one task and one commit.

Three kinds of change:

1. **Delete (15).** Remove the function. If an import or a helper becomes unused, remove it (the compiler and
   `go vet` report it). Do not touch a helper that is still used elsewhere.
2. **Merge (13).** Fold the test's distinct input or assertion into the named partner as a table row, subtest or extra
   assertion. Anything the partner already asserts is dropped (issue Context). The partner keeps its function name,
   because acceptance criteria run `go test -run '^<partner>$' -v`. The partner keeps every assertion it had.
3. **Move (1).** `TestSandboxArchitectureDetection` becomes `sandbox.TestDetectHostArchitecture`, unchanged apart from
   the package qualifier (the call is `DetectHostArchitecture()` inside package `sandbox`, not
   `sandbox.DetectHostArchitecture()`). Every assertion is kept.

### Facts verified against the worktree (HEAD `ec7bf83`)

- All 29 listed tests and all 17 partners exist, in the cited files. Line numbers in `backend_routing_test.go` have
  drifted by 8 (`TestScoreLLMJudgeFailureWording` is at 158, `TestNoKiroCLIProcessOutsideDockerPath` at 361,
  `TestMeasureStartupOverheadUsesBackendProbe` at 224); names match, so this is not a blocker.
- Partner locations outside the file of the merged test: `TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals`
  is `repeat_exec_test.go:145`; `TestSummaryMultiAgentOmitsTopLevelAgentFields` is `pinning_test.go:490`;
  `TestCaseTotalsCountsLLMJudgeErrorAndNoUsableJudge` and `TestPrintCaseResultBreakdownOnlyBelowThreshold` are in
  `scoring_test.go`; `TestLoadCasesClassicTurnOneLoads` is in `turns_schema_test.go`;
  `TestAuditFlatFixtureEvidenceAndDuplicates` is `cmd/kairon/cmd/eval_audit_test.go:47`. Those partners of Delete rows
  are not edited.
- `internal/eval/sandbox/architecture_test.go` has no build tag and no `TestDetectHostArchitecture`. The only other
  architecture tests in the package are `TestArchitectureIntegration_*` in `integration_architecture_test.go`
  (`//go:build integration`). `sandbox.DetectHostArchitecture` is a pure `runtime.GOARCH` switch in `container.go`.
- No live file outside `internal/eval/*_test.go` mentions any of the 29 names (checked `README.md`, `docs/`, `.kiro/`,
  `.kairon/` outside `specs/`, `research/`, `results/`). No documentation change is needed.
- `scoredUserTurns` (deleted test `TestScoredUserTurns`) stays in `scoring.go` and is still exercised through
  `caseCheckInput`; it is production code and is not touched.
- **Coverage has no headroom.** `go test -cover ./internal/eval` currently reports `87.6%` (about 6 minutes, 349 s of
  test time), and the acceptance bar is `87.6%`. A change that moves the unrounded figure below 87.55% fails AC4.
  `coverage-and-gate` therefore diffs per-function coverage against the baseline profile and repairs any loss by adding
  the missing case to a partner (never by keeping a test the issue says to delete; if that is the only way, the issue
  is BLOCKED and krew-lead halts).
- The worktree already has unrelated uncommitted modifications to `.kiro/agents/*.json`. Builders must not stage or
  commit them.

## Relevant Files

Modified:

- `internal/eval/architecture_compatibility_test.go` (delete 4 tests, remove the moved test)
- `internal/eval/sandbox/architecture_test.go` (receive `TestDetectHostArchitecture`; add the `runtime` import)
- `internal/eval/attempts_test.go` (merge 2, delete 1)
- `internal/eval/repeat_exec_test.go` (receives `TestSelftestFailRepeatAttempts`)
- `internal/eval/audit_test.go` (merge 4, delete 1)
- `cmd/kairon/cmd/eval_audit_test.go` (receives the evidence and duplicate-line fixtures' stricter assertions)
- `internal/eval/backend_routing_test.go` (delete 2)
- `internal/eval/candidate_e2e_test.go` (merge 2)
- `internal/eval/candidate_test.go` (merge 1 into the hardlink partner; delete the symlink function)
- `internal/eval/pinning_test.go` (receives `TestUpdateIncrementalSummaryPromptFile`)
- `internal/eval/checks_judge_test.go` (delete 1)
- `internal/eval/checks_only_test.go` (delete 1, merge 2)
- `internal/eval/checks_scoring_test.go` (delete 2)
- `internal/eval/checks_test.go` (merge 1, delete 1)
- `internal/eval/config_test.go` (delete 2)

Not modified: any non-test `.go` file, `docs/`, `README.md`, `.kairon/specs/` other than this file.

## Team Orchestration

All tasks run sequentially in the one shared worktree (each edits different files, but the Go package is shared and
the final gates are package-wide, so no parallelism). Order: baseline, then the sandbox move with the architecture
deletes, then each remaining file group, then the coverage and `task test` gate, then independent validation.

Rules for every builder task:

- Edit only the files named in the task. If a Delete or Merge cannot be done as described (for example a named
  assertion cannot be expressed in the partner), stop and write the sentinel with `**Status**: BLOCKED`; do not drop
  the assertion.
- Keep `gofmt` clean and remove imports and helpers that the edit orphans.
- Per AGENTS.md "No silent deviation": any departure from this spec gets a code comment citing the section and the
  reason, and is listed in the sentinel so krew-lead can put it in the PR's `Spec deviations` section. Expected: none.
- Commit test changes for a task in a single commit that compiles and passes (never leave CI red). In the commit
  message, name the characterization tests relied on (the partners). Do not push.

## Step-by-Step Task Breakdown

### Task 1: `baseline-coverage`
Run `go test -coverprofile=.kairon/artifacts/issue-423-baseline.cover ./internal/eval` before any edit, record the
printed percentage and `go tool cover -func` total in the sentinel. Also save `go tool cover -func` output to
`.kairon/artifacts/issue-423-baseline.func.txt` for later diffing. `.kairon/artifacts/` is gitignored; commit nothing.
Expected: `coverage: 87.6% of statements` (measured by the architect at HEAD). No source edits.

### Task 2: `arch-move-and-prune` (architecture_compatibility + sandbox move)
In this order:
1. Add `TestDetectHostArchitecture` to `internal/eval/sandbox/architecture_test.go` (no build tag; add `"runtime"` to
   the imports). Body is `TestSandboxArchitectureDetection`, unchanged apart from the package qualifier (call
   `DetectHostArchitecture()`, not `sandbox.DetectHostArchitecture()`): `DetectHostArchitecture()` with
   `require.NoError`, `assert.Contains(platform, "linux/")`, and the `runtime.GOARCH` switch asserting
   `linux/amd64` or `linux/arm64`, `t.Fatalf` on any other arch. Run `go test -run '^TestDetectHostArchitecture$' -v
   ./internal/eval/sandbox` and confirm it runs (not skipped) and passes.
2. In `internal/eval/architecture_compatibility_test.go` delete `TestSandboxArchitectureDetection` (moved) and delete
   `TestArchitectureCompatibility`, `TestRunnerArchitectureSupport`, `TestEvalFrameworkArchitectureIndependence`,
   `TestArchitecturePlatformConsistency`. `TestKiroCLIURLGeneration` stays and still uses `runtime`, `sandbox`,
   `assert` and `require`; drop any import that is no longer used.

### Task 3: `attempts-and-repeat` (attempts_test.go, repeat_exec_test.go)
- `TestAttemptAccountingOnePerCase` into `TestAttemptAccountingRepeats`: restructure the partner into two subtests,
  `one attempt per case` (the merged input `c1 pass, c2 fail, c3 pass` with `Repeat` unset, expecting `attemptAccounting`
  = 3 total, 2 passed) and `repeats` (the partner's current body unchanged: 6/3 totals, `AttemptsTotal` 6,
  `AttemptsPassed` 3, `CasesTotal` 2, `SuccessRate == 50`). A shared table is acceptable if each case keeps its own
  expected totals; the `repeats` case keeps the verdict assertions.
- Delete `TestSuccessRateIsPercentNotFraction` (`TestAttemptAccountingRepeats` asserts `success_rate == 50` and
  `TestVerdictPassesWithSuccessRateBelow100` asserts 66.67).
- `TestSelftestFailRepeatAttempts` into `TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals`: the partner
  already runs `runStubRepeat(t, "selftest-fail", "", RunOptions{Repeat: 2})`. Capture `out` and `runDir` (currently
  `_`) and add a subtest `attempt accounting` that asserts everything the merged test asserted, using the partner's
  `sum`/`res`/`out`: `attempts_total == len(res.Cases)`; `attempts_passed` equals the count of cases with `Passed`
  true; `attempts_total > cases_total`; `success_rate` matches `passed*100/len` within 1e-9; exactly one verdict line
  that starts `FAIL selftest-fail: ` and contains `cases failed, success rate %.1f%% (%d/%d attempts)`, and matches
  `cases failed, success rate \d+\.\d%`; `summary.json` on disk (read from `runDir`) contains `"attempts_total"`,
  `"attempts_passed"` and `"success_rate"`; and a guard that `len(res.Cases) != 0`. The partner's own assertions for
  `ErrThresholdFailed`, spread and recomputed totals stay unchanged. Delete the old function in `attempts_test.go`
  and fix imports in both files (`fmt`, `regexp` move to `repeat_exec_test.go` if needed). This removes a second
  22-case `--repeat 2` run.

### Task 4: `audit-and-cmd-audit` (audit_test.go, cmd/kairon/cmd/eval_audit_test.go)
Do the `cmd` additions first, then delete the internal functions, so no assertion is ever absent.
- `TestAuditOrderingIsStable` into `TestAuditUnmappedFixture`: after the existing assertions, assert
  `auditRules(ae)` equals `[]string{"unmapped", "unknown-criterion", "coverage"}` (`reflect.DeepEqual`; `reflect` is
  already imported). Delete the old function.
- `TestAuditDuplicateRuleOrder` into `TestAuditDuplicates`, subtest `repeat of an unmapped line is a duplicate and
  counted unmapped` (input `auditRTCC + "Mapped.\nOrphan.\nOrphan.\n"`, so the rules are on lines 5, 6, 7 and the
  repeat is on line 7): add one assertion that the violations on line 7, in report order, have rules
  `["duplicate", "unmapped"]`. The first half of the old test (comparing `auditRuleOrder` map values) is the constant
  check the issue says to drop. The issue text says "line-6"; that was the line in the old test's own prompt, and in
  the partner's prompt the repeat is line 7, so the assertion is on line 7. This is the issue's intent, not a
  deviation. At the assertion, the builder writes a code comment that quotes the issue's "line-6", says that 6 was the
  line in the old `TestAuditDuplicateRuleOrder` prompt, and says that in this subtest's prompt the same repeat is on
  line 7. Krew-lead repeats that note in the PR description (see "PR description requirements"). Delete the old
  function.
- Delete `TestAuditReportDuplicates` (`TestAuditDuplicates` asserts `duplicates=0` in 'distinct lines' and
  `duplicates=2` in 'third occurrence').
- `TestAuditEvidenceFixtures` and `TestAuditDuplicateLineFixture` into
  `cmd/kairon/cmd/eval_audit_test.go` `TestAuditFlatFixtureEvidenceAndDuplicates`. Add table fields and assertions
  to the existing rows `evidence-missing`, `evidence-unknown-case`, `evidence-unchecked-criterion` and
  `duplicate-line` (do not add new rows; the fixtures are already there):
  - the stricter full message in `wantErr` (as a substring of the error text; paths are relative to the fixture
    directory, as the existing rows already write them):
    - evidence-missing: `selftest/load-bearing.md:3: table: row 3: Evidence must name at least one case of agent "selftest" as case:<case-name>` and the same with `:4:` and `row 4`.
    - evidence-unknown-case: `selftest/load-bearing.md:4: table: row 4: evidence names case "ghost" which is not a case of agent "selftest"`
    - evidence-unchecked-criterion: `selftest/load-bearing.md:5: table: row 5: case "alpha-only" has no check on criterion "beta"`
    - duplicate-line: one contiguous string, `P:9: duplicate: duplicate of P:8: "Never skip validation."`, where `P` is
      the fixture directory plus `/agents/selftest-prompt.md`. Build `P` in the test as
      `filepath.Join(auditFixtures, "duplicate-line", "agents", "selftest-prompt.md")` (add the `path/filepath` import).
      This is the same construction as the old test's `filepath.Join(opts.EvalsDir, "agents", "selftest-prompt.md")`
      and matches what `kairon eval audit` prints (checked at HEAD: `Error: <evalsDir>/agents/selftest-prompt.md:9:
      duplicate: duplicate of <evalsDir>/agents/selftest-prompt.md:8: "Never skip validation."`, with `<evalsDir>` the
      cleaned path the command receives). The string replaces the row's four separate `wantErr` fragments (`prompt +
      ":9: duplicate:"`, `"duplicate of"`, `prompt + ":8:"`, `"Never skip validation."`), which it contains, so the old
      assertion is kept in full.
  - a new `wantViolations int` field, asserted via `errors.As(err, &ae)` with `var ae *eval.AuditError`
    (import `github.com/jbrinkman/kairon/internal/eval`): for the three evidence rows the number of `Rule == "table"`
    violations equals `wantViolations` and equals `len(ae.Violations)` (2, 1, 1); for `duplicate-line`,
    `len(ae.Violations) == 1`. Rows other than these four leave the field zero and skip the check (add an explicit
    `checkViolations bool` or use `-1` for "not checked" so a zero is not ambiguous).
  - `coverage=100.0%` in `wantOut` for the three evidence rows, and
    `Instructions: total=5 mapped=5 duplicates=1 coverage=100.0% (threshold 90%)` for `duplicate-line`.
  Then delete the two functions from `internal/eval/audit_test.go`. `auditFlatFixtureOpts`, `runAuditAgent` and
  `tableViolations` are still used by other tests; leave them.

### Task 5: `backend-routing-prune` (backend_routing_test.go)
Delete `TestScoreLLMJudgeFailureWording` and `TestNoKiroCLIProcessOutsideDockerPath`. Their partners
(`TestCaseTotalsCountsLLMJudgeErrorAndNoUsableJudge`, `TestMeasureStartupOverheadUsesBackendProbe`) are not edited.
Remove imports (`os`, `strings`, others) only if now unused.

### Task 6: `candidate-e2e-merge` (candidate_e2e_test.go)
- `TestCandidateE2ELiveAgentsUnchanged` into `TestCandidateE2EKeptWorkspacesHoldCandidate`: in the partner, before
  its `runE2E`, take the `git status --porcelain .kiro/agents` clean check and the `treeSnapshot` of the project
  `.kiro/agents` and the evals-dir `agents/` (fail if either is empty). After the partner's single run, add subtests:
  `kept workspaces hold the candidate` (the partner's existing per-case loop and completeness guard, unchanged) and
  `live agents unchanged` (`run.agent.PromptFile != ""` guard; `assertSnapshotsEqual` for both trees; git status
  clean for `.kiro/agents`; `git status --porcelain` clean for the whole repo). The partner keeps its `PATH` without
  kiro-cli. Delete the old function.
- `TestCandidateE2ESameContentCandidateHashEqualsBaseline` into `TestCandidateE2EProvenanceRecorded`: the partner
  already has the baseline `base`. Add subtest `same-content candidate hashes like the baseline` that writes a
  candidate with the live prompt bytes (`writeE2ECandidate(t, string(liveEvalsPrompt(t, env.evalsDir)))`), runs it
  once against a fresh evals dir, and asserts the same four things as the old test: `agent.PromptSHA256` equals
  `base.agent.PromptSHA256`; `summary.Agents[e2eAgent].PromptSHA256` equals the baseline's; `agent.PromptFile` and
  `summary.Agents[e2eAgent].PromptFile` are non-empty. Per the issue, make this run a one-case run (for example the
  `stub-marker` case, `markerCase`) by giving `runE2E` a case-filter variant (`runE2ECase(t, evalsDir, testcase,
  opts)`, with `runE2E` calling it with `""`); the hash is of the prompt bytes, not of the case set. If the one-case
  run cannot produce the same assertions (for example the result file layout differs), run the full suite instead
  and record that as a documented deviation. Delete the old function. Net effect: one run instead of two.

### Task 7: `candidate-merge` (candidate_test.go, pinning_test.go)
- `TestUpdateIncrementalSummaryPromptFile` into `TestSummaryMultiAgentOmitsTopLevelAgentFields` (`pinning_test.go`).
  Add a `promptFile` argument to `mk()`. Keep the partner's existing assertions. Add these merged assertions: after
  the single-agent update with `PromptFile: "iter/v2.md"`, `s.Agents["a"].PromptFile == "iter/v2.md"` and top-level
  `s.PromptFile == "iter/v2.md"`; after the second agent (no prompt file) is added, top-level `PromptFile == ""`,
  `Agents["a"].PromptFile == "iter/v2.md"` (or the value used) and `Agents["b"].PromptFile == ""`. Add subtest
  `prompt_file key is absent without a candidate` that updates a fresh single-agent summary with no `PromptFile` and
  asserts the raw JSON does not contain `prompt_file`. Delete the old function from `candidate_test.go`.
- `TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough` into
  `TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough`: one table with rows `symlink` and `hardlink`.
  Per-row setup creates the link (`os.Symlink` row skips on `runtime.GOOS == "windows"`; `os.Link` row calls
  `t.Skipf("hard links unsupported: %v", err)` on error), then both rows run `stageCandidatePrompt` and assert: the
  link target/peer content is unchanged (`symlink target` / `hardlink peer`), the destination holds the candidate
  bytes, and (symlink row; fine to apply to both) `os.Lstat(dest).Mode().IsRegular()`. Delete the old function.

### Task 8: `checks-judge-and-only` (checks_judge_test.go, checks_only_test.go)
- Delete `TestScoredUserTurns` (partner `TestCaseCheckInputCarriesUserTurns` asserts the same nil and turns
  outcomes).
- Delete `TestRubricChecksOnlyYAML`. Remove the `gopkg.in/yaml.v3` import if nothing else in the file uses it.
- `TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase` and `TestLoadCasesChecksOnlyAllowsFullyCheckedCases` into
  `TestLoadCasesChecksOnlyRejectsCheckLessCase`: one table over (`checks_only`, case set) with subtests
  `checks_only rejects a check-less case`, `checks_only false allows a check-less case` and
  `checks_only allows fully checked cases`. Row 1 keeps the current assertions: error non-nil, wraps
  `errInvalidChecks`, names `zz-missing-checks-it.yaml`. Row 2 (checks_only false, `good.yaml` and the check-less
  case) asserts no error and `len(cases) == 2`. Row 3 (checks_only true, `good.yaml` only) asserts no error and
  `len(cases) == 1`. Each row calls `configure(...)` and `t.Cleanup(resetConfig)` as the originals did. Delete the two
  old functions.

### Task 9: `checks-scoring-and-checks` (checks_scoring_test.go, checks_test.go)
- Delete `TestChecksScoringNoCommandRunWithoutOutput` (partner `TestChecksScoringNoOutputOrWorkspaceScoresZero`
  'empty output' asserts the `touch` command did not run).
- Delete `TestPrintCaseResultNoChecksUnchanged` (partner `TestPrintCaseResultBreakdownOnlyBelowThreshold`).
- `TestChecksValidateNilRubricSkipsCrossCheck` into `TestChecksValidateRubricCrossCheck`: add a `nilRubric bool`
  field to the table and a row `nil rubric skips the cross-check` with `Check{Criterion: "anything", Type:
  CheckFileExists, Path: "a"}` and `wantErr: ""`. When `nilRubric` is set, pass a nil `*Rubric` to `ValidateChecks`.
  The row must assert that no error is returned (the valid-case branch already does). Delete the old function.
- Delete `TestLoadCasesWithoutChecksUnchanged` (partner `TestLoadCasesClassicTurnOneLoads`).
- Clean up imports and unused helpers in both files.

### Task 10: `config-prune` (config_test.go)
Delete `TestCostInfoJSONOmitsEmptyNewFields` and `TestTestCaseParsesStubFromYAML`. Remove now-unused imports
(`encoding/json`, `gopkg.in/yaml.v3`, `strings`) only if nothing else in the file uses them.

### Task 11: `coverage-and-gate`
1. `gofmt -l` clean for `internal/eval`, `internal/eval/sandbox`, `cmd/kairon/cmd`; `go vet ./...`.
2. `go test -coverprofile=.kairon/artifacts/issue-423-after.cover -cover ./internal/eval` and confirm the reported
   figure is at least `87.6%`.
3. If it is below, run `go tool cover -func` on both profiles, diff against `issue-423-baseline.func.txt`, find the
   functions that lost coverage and the deleted test that covered them, and restore the coverage by adding the
   missing input as a case or assertion in that test's named partner (or, for a Delete row, in the partner named in
   the issue). Do not re-add a Delete or Merge function. If the only way to restore coverage is to keep a function
   the issue says to remove, write the sentinel as BLOCKED, quoting AC4 and the function.
4. `task test` exits 0 (this is `go test -v -race -coverprofile=coverage.out ./...`, a long run; `coverage.out` and
   `coverage.html` are gitignored).

### Task 12: `validate-complete`
Read-only validator re-runs every acceptance-criterion check from the issue: the Delete grep (prints nothing), the
Merge grep (prints nothing), each of the 13 merged cases is visible in `go test -run '^<partner>$' -v` output (or, for
the `cmd` partner, `go test -run '^TestAuditFlatFixtureEvidenceAndDuplicates$' -v ./cmd/kairon/cmd`), the moved test
(one file, no `//go:build`, runs and passes in `./internal/eval/sandbox`), `go test -cover ./internal/eval` at least
`87.6%`, `task test` exit 0. It also checks the diff touches no non-test `.go` file, that no deleted test's partner
lost an assertion, and that any recorded deviation has a code comment.

## Validation Commands

```bash
# AC1: prints nothing
grep -nE 'func (TestArchitectureCompatibility|TestArchitecturePlatformConsistency|TestAuditReportDuplicates|TestChecksScoringNoCommandRunWithoutOutput|TestCostInfoJSONOmitsEmptyNewFields|TestEvalFrameworkArchitectureIndependence|TestLoadCasesWithoutChecksUnchanged|TestNoKiroCLIProcessOutsideDockerPath|TestPrintCaseResultNoChecksUnchanged|TestRubricChecksOnlyYAML|TestRunnerArchitectureSupport|TestScoreLLMJudgeFailureWording|TestScoredUserTurns|TestSuccessRateIsPercentNotFraction|TestTestCaseParsesStubFromYAML)[(]' internal/eval/*_test.go

# AC2: prints nothing
grep -nE 'func (TestAttemptAccountingOnePerCase|TestAuditDuplicateLineFixture|TestAuditDuplicateRuleOrder|TestAuditEvidenceFixtures|TestAuditOrderingIsStable|TestCandidateE2ELiveAgentsUnchanged|TestCandidateE2ESameContentCandidateHashEqualsBaseline|TestChecksValidateNilRubricSkipsCrossCheck|TestLoadCasesChecksOnlyAllowsFullyCheckedCases|TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase|TestSelftestFailRepeatAttempts|TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough|TestUpdateIncrementalSummaryPromptFile)[(]' internal/eval/*_test.go

# AC3
grep -n 'func TestSandboxArchitectureDetection(' internal/eval/*_test.go          # prints nothing
test "$(grep -ln 'func TestDetectHostArchitecture(' internal/eval/sandbox/*_test.go | wc -l)" -eq 1   # exactly one file (numeric compare: BSD wc pads)
f=$(grep -ln 'func TestDetectHostArchitecture(' internal/eval/sandbox/*_test.go) && test -n "$f" && ! grep -n '^//go:build' $f   # no build tag
go test -run '^TestDetectHostArchitecture$' -v ./internal/eval/sandbox | grep -- '--- PASS: TestDetectHostArchitecture'   # runs, not skipped

# AC4 / AC5
go test -cover ./internal/eval        # coverage: 87.6% or higher
task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "baseline-coverage"
    agent: "builder"
    description: "Before any edit, record the baseline of go test -cover ./internal/eval: write a coverage profile and a go tool cover -func listing to .kairon/artifacts/ (gitignored) and put the printed percentage and the func total in the sentinel. No source edits."
    dependencies: []
    acceptance_criteria:
      - "go test -coverprofile=.kairon/artifacts/issue-423-baseline.cover ./internal/eval passes and prints a coverage percentage"
      - ".kairon/artifacts/issue-423-baseline.func.txt exists and holds the go tool cover -func output for the baseline profile"
      - "The sentinel states the baseline percentage (expected 87.6%) and the func total"
      - "git status shows no changed source files from this task"
    validation_commands:
      - "test -s .kairon/artifacts/issue-423-baseline.cover"
      - "test -s .kairon/artifacts/issue-423-baseline.func.txt"
      - "tail -n 1 .kairon/artifacts/issue-423-baseline.func.txt"

  - id: "arch-move-and-prune"
    agent: "builder"
    description: "Add TestDetectHostArchitecture (body of TestSandboxArchitectureDetection, unchanged apart from the package qualifier, no build tag) to internal/eval/sandbox/architecture_test.go first, then in internal/eval/architecture_compatibility_test.go delete TestSandboxArchitectureDetection (moved) and delete TestArchitectureCompatibility, TestRunnerArchitectureSupport, TestEvalFrameworkArchitectureIndependence and TestArchitecturePlatformConsistency. Remove imports orphaned by the edit."
    dependencies: ["baseline-coverage"]
    acceptance_criteria:
      - "internal/eval/sandbox/architecture_test.go defines func TestDetectHostArchitecture, unchanged apart from the package qualifier from the old TestSandboxArchitectureDetection (NoError, Contains linux/, GOARCH switch with Fatalf on other arches), and the file has no //go:build line"
      - "go test -run '^TestDetectHostArchitecture$' -v ./internal/eval/sandbox exits 0 and prints '--- PASS: TestDetectHostArchitecture' (a skip does not count as a run)"
      - "TestSandboxArchitectureDetection, TestArchitectureCompatibility, TestRunnerArchitectureSupport, TestEvalFrameworkArchitectureIndependence and TestArchitecturePlatformConsistency no longer exist in internal/eval"
      - "TestKiroCLIURLGeneration is unchanged and passes"
      - "gofmt and go vet are clean for both packages"
    validation_commands:
      - "! grep -nE 'func (TestSandboxArchitectureDetection|TestArchitectureCompatibility|TestRunnerArchitectureSupport|TestEvalFrameworkArchitectureIndependence|TestArchitecturePlatformConsistency)[(]' internal/eval/*_test.go"
      - "test \"$(grep -ln 'func TestDetectHostArchitecture(' internal/eval/sandbox/*_test.go | wc -l)\" -eq 1"
      - "f=$(grep -ln 'func TestDetectHostArchitecture(' internal/eval/sandbox/*_test.go) && test -n \"$f\" && ! grep -n '^//go:build' $f"
      - "go test -run '^TestDetectHostArchitecture$' -v ./internal/eval/sandbox"
      - "go test -run '^TestDetectHostArchitecture$' -v ./internal/eval/sandbox | grep -- '--- PASS: TestDetectHostArchitecture'"
      - "go test -run '^TestKiroCLIURLGeneration$' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval internal/eval/sandbox)\""
      - "go vet ./internal/eval/..."

  - id: "attempts-and-repeat"
    agent: "builder"
    description: "attempts_test.go: merge TestAttemptAccountingOnePerCase into TestAttemptAccountingRepeats as subtests 'one attempt per case' and 'repeats' (repeats keeps the verdict and success_rate==50 assertions); delete TestSuccessRateIsPercentNotFraction. Merge TestSelftestFailRepeatAttempts into TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals (repeat_exec_test.go) as an 'attempt accounting' subtest that uses the partner's single --repeat 2 selftest-fail run and keeps every assertion (attempts_total, attempts_passed, attempts_total > cases_total, success_rate, the FAIL verdict line text and regexp, the three summary.json keys). Delete the three old functions and fix imports."
    dependencies: ["arch-move-and-prune"]
    acceptance_criteria:
      - "TestAttemptAccountingOnePerCase, TestSuccessRateIsPercentNotFraction and TestSelftestFailRepeatAttempts no longer exist"
      - "TestAttemptAccountingRepeats passes with subtests 'one_attempt_per_case' and 'repeats' in -v output, and still asserts attempts 6/3, cases_total 2 and success_rate 50"
      - "TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals passes with an 'attempt_accounting' subtest and keeps its original assertions"
      - "Only one 22-case selftest-fail --repeat 2 run remains for these assertions"
    validation_commands:
      - "! grep -nE 'func (TestAttemptAccountingOnePerCase|TestSuccessRateIsPercentNotFraction|TestSelftestFailRepeatAttempts)[(]' internal/eval/*_test.go"
      - "go test -run '^TestAttemptAccountingRepeats$' -v ./internal/eval | grep -E -- '--- PASS: TestAttemptAccountingRepeats/(one_attempt_per_case|repeats)'"
      - "go test -run '^TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals$' -v ./internal/eval | grep -E -- '--- PASS: TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals/attempt_accounting'"
      - "go test -run 'TestAttempt|TestVerdictPasses|TestPrintAgentVerdictSuccessRate' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go vet ./internal/eval/..."

  - id: "audit-and-cmd-audit"
    agent: "builder"
    description: "First extend TestAuditFlatFixtureEvidenceAndDuplicates in cmd/kairon/cmd/eval_audit_test.go with the stricter assertions of TestAuditEvidenceFixtures and TestAuditDuplicateLineFixture (full violation messages, with the duplicate-line message asserted as one contiguous string P:9: duplicate: duplicate of P:8: \"Never skip validation.\" where P is filepath.Join(auditFixtures, \"duplicate-line\", \"agents\", \"selftest-prompt.md\"); exact violation counts via errors.As to *eval.AuditError; coverage=100.0% / Instructions totals line in wantOut) on the existing evidence-missing, evidence-unknown-case, evidence-unchecked-criterion and duplicate-line rows. Then in internal/eval/audit_test.go: add the auditRules order assertion to TestAuditUnmappedFixture; add the duplicate-before-unmapped rule order assertion (on line 7, the repeat's line in that subtest's prompt, with a code comment that quotes the issue's line-6 and says 6 was the old TestAuditDuplicateRuleOrder prompt's line) to the 'repeat of an unmapped line is a duplicate and counted unmapped' subtest of TestAuditDuplicates; delete TestAuditReportDuplicates; delete TestAuditOrderingIsStable, TestAuditDuplicateRuleOrder, TestAuditEvidenceFixtures and TestAuditDuplicateLineFixture."
    dependencies: ["attempts-and-repeat"]
    acceptance_criteria:
      - "The five audit functions TestAuditOrderingIsStable, TestAuditEvidenceFixtures, TestAuditDuplicateRuleOrder, TestAuditReportDuplicates and TestAuditDuplicateLineFixture no longer exist in internal/eval"
      - "TestAuditFlatFixtureEvidenceAndDuplicates passes and its evidence-missing, evidence-unknown-case, evidence-unchecked-criterion and duplicate-line subtests assert the exact messages, the exact violation counts (2, 1, 1, 1) and the coverage/totals lines the removed tests asserted"
      - "The duplicate-line row asserts the contiguous string P:9: duplicate: duplicate of P:8: \"Never skip validation.\" (P = fixture dir + /agents/selftest-prompt.md), so the old TestAuditDuplicateLineFixture message assertion is kept without loss"
      - "TestAuditUnmappedFixture asserts the rule order [unmapped unknown-criterion coverage]"
      - "TestAuditDuplicates passes and its unmapped-repeat subtest asserts rules [duplicate unmapped] on the repeat's line (line 7), with a code comment that cites the issue's line-6 and says it was the old prompt's line"
      - "No edits to non-test files; helpers auditFlatFixtureOpts, runAuditAgent and tableViolations remain because other tests use them"
    validation_commands:
      - "! grep -nE 'func (TestAuditOrderingIsStable|TestAuditEvidenceFixtures|TestAuditDuplicateRuleOrder|TestAuditReportDuplicates|TestAuditDuplicateLineFixture)[(]' internal/eval/*_test.go"
      - "go test -run '^TestAuditFlatFixtureEvidenceAndDuplicates$' -v ./cmd/kairon/cmd"
      - "go test -run '^(TestAuditUnmappedFixture|TestAuditDuplicates)$' -v ./internal/eval"
      - "go test -run 'TestAudit' ./internal/eval ./cmd/kairon/cmd"
      - "test -z \"$(gofmt -l internal/eval cmd/kairon/cmd)\""
      - "go vet ./internal/eval/... ./cmd/kairon/cmd/..."

  - id: "backend-routing-prune"
    agent: "builder"
    description: "Delete TestScoreLLMJudgeFailureWording and TestNoKiroCLIProcessOutsideDockerPath from internal/eval/backend_routing_test.go and remove imports orphaned by the deletion. Their partners TestCaseTotalsCountsLLMJudgeErrorAndNoUsableJudge (scoring_test.go) and TestMeasureStartupOverheadUsesBackendProbe are not edited."
    dependencies: ["audit-and-cmd-audit"]
    acceptance_criteria:
      - "TestScoreLLMJudgeFailureWording and TestNoKiroCLIProcessOutsideDockerPath no longer exist"
      - "The two partner tests still pass unchanged"
      - "Other backend_routing tests (including TestScoreLLMJudgeDefaultBackend) are unchanged and pass"
    validation_commands:
      - "! grep -nE 'func (TestScoreLLMJudgeFailureWording|TestNoKiroCLIProcessOutsideDockerPath)[(]' internal/eval/*_test.go"
      - "go test -run '^(TestCaseTotalsCountsLLMJudgeErrorAndNoUsableJudge|TestMeasureStartupOverheadUsesBackendProbe|TestScoreLLMJudgeDefaultBackend)$' ./internal/eval"
      - "go test -run 'TestStub|TestInvokeAgent|TestMeasureStartup|TestAgentConfigDir' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go vet ./internal/eval/..."

  - id: "candidate-e2e-merge"
    agent: "builder"
    description: "In candidate_e2e_test.go merge TestCandidateE2ELiveAgentsUnchanged into TestCandidateE2EKeptWorkspacesHoldCandidate (pre-run git-status and tree snapshots of project .kiro/agents and evals agents/, the partner's single run, then subtests 'kept workspaces hold the candidate' and 'live agents unchanged' with the PromptFile guard, both assertSnapshotsEqual calls and both git status checks). Merge TestCandidateE2ESameContentCandidateHashEqualsBaseline into TestCandidateE2EProvenanceRecorded as a 'same-content candidate hashes like the baseline' subtest that adds one same-content candidate run (a one-case run via a runE2ECase helper) against the partner's baseline and keeps the four hash and prompt_file assertions. Delete both old functions."
    dependencies: ["backend-routing-prune"]
    acceptance_criteria:
      - "TestCandidateE2ELiveAgentsUnchanged and TestCandidateE2ESameContentCandidateHashEqualsBaseline no longer exist"
      - "TestCandidateE2EKeptWorkspacesHoldCandidate passes with subtests 'kept_workspaces_hold_the_candidate' and 'live_agents_unchanged' and performs exactly one RunWithOptions"
      - "TestCandidateE2EProvenanceRecorded passes with subtest 'same-content_candidate_hashes_like_the_baseline' asserting agent and summary prompt_sha256 equal the baseline and prompt_file recorded in selftest.json and summary agents.selftest"
      - "If the one-case run is not usable, the full-suite run is used and the departure is recorded in a code comment citing this task and in the sentinel"
    validation_commands:
      - "! grep -nE 'func (TestCandidateE2ELiveAgentsUnchanged|TestCandidateE2ESameContentCandidateHashEqualsBaseline)[(]' internal/eval/*_test.go"
      - "go test -run '^TestCandidateE2EKeptWorkspacesHoldCandidate$' -v ./internal/eval | grep -E -- '--- PASS: TestCandidateE2EKeptWorkspacesHoldCandidate/live_agents_unchanged'"
      - "go test -run '^TestCandidateE2EProvenanceRecorded$' -v ./internal/eval | grep -E -- '--- PASS: TestCandidateE2EProvenanceRecorded/same-content'"
      - "go test -run 'TestCandidateE2E' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go vet ./internal/eval/..."

  - id: "candidate-merge"
    agent: "builder"
    description: "Merge TestUpdateIncrementalSummaryPromptFile (candidate_test.go) into TestSummaryMultiAgentOmitsTopLevelAgentFields (pinning_test.go) by adding a promptFile argument to mk() and the single-agent, multi-agent and key-absent prompt_file assertions; delete the old function. Merge TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough into TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough as one table with 'symlink' and 'hardlink' rows (symlink row skips on windows, hardlink row skips when hard links are unsupported); delete the old function."
    dependencies: ["candidate-e2e-merge"]
    acceptance_criteria:
      - "TestUpdateIncrementalSummaryPromptFile and TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough no longer exist"
      - "TestSummaryMultiAgentOmitsTopLevelAgentFields passes and keeps all its original assertions plus: agents.a.prompt_file and top-level prompt_file set for one agent, top-level empty with agent a kept and agent b empty for two agents, and the raw JSON lacks prompt_file without a candidate"
      - "TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough passes with subtests 'symlink' and 'hardlink', each asserting no write-through, the candidate bytes at the destination, and a regular file at the destination"
    validation_commands:
      - "! grep -nE 'func (TestUpdateIncrementalSummaryPromptFile|TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough)[(]' internal/eval/*_test.go"
      - "go test -run '^TestSummaryMultiAgentOmitsTopLevelAgentFields$' -v ./internal/eval"
      - "go test -run '^TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough$' -v ./internal/eval | grep -E -- '--- (PASS|SKIP): TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough/(symlink|hardlink)'"
      - "go test -run 'TestStage|TestUpdateIncremental|TestApplyTo|TestSummary' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go vet ./internal/eval/..."

  - id: "checks-judge-and-only"
    agent: "builder"
    description: "checks_judge_test.go: delete TestScoredUserTurns. checks_only_test.go: delete TestRubricChecksOnlyYAML; merge TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase and TestLoadCasesChecksOnlyAllowsFullyCheckedCases into TestLoadCasesChecksOnlyRejectsCheckLessCase as a table over (checks_only, case set) with three subtests, keeping the reject row's errInvalidChecks and file-name assertions and the allow rows' no-error and case-count (2 and 1) assertions; delete the two old functions; remove orphaned imports."
    dependencies: ["candidate-merge"]
    acceptance_criteria:
      - "TestScoredUserTurns, TestRubricChecksOnlyYAML, TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase and TestLoadCasesChecksOnlyAllowsFullyCheckedCases no longer exist"
      - "TestLoadCasesChecksOnlyRejectsCheckLessCase passes with three subtests (reject check-less, allow check-less when checks_only is false, allow fully checked cases)"
      - "TestCaseCheckInputCarriesUserTurns is unchanged and passes"
    validation_commands:
      - "! grep -nE 'func (TestScoredUserTurns|TestRubricChecksOnlyYAML|TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase|TestLoadCasesChecksOnlyAllowsFullyCheckedCases)[(]' internal/eval/*_test.go"
      - "go test -run '^TestLoadCasesChecksOnlyRejectsCheckLessCase$' -v ./internal/eval"
      - "go test -run '^TestCaseCheckInputCarriesUserTurns$' ./internal/eval"
      - "go test -run 'ChecksOnly|CheckInput|ScoreCaseJudge' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go vet ./internal/eval/..."

  - id: "checks-scoring-and-checks"
    agent: "builder"
    description: "checks_scoring_test.go: delete TestChecksScoringNoCommandRunWithoutOutput and TestPrintCaseResultNoChecksUnchanged. checks_test.go: merge TestChecksValidateNilRubricSkipsCrossCheck into TestChecksValidateRubricCrossCheck as a 'nil rubric' table row (new nilRubric field, Check{Criterion: anything, Type: file_exists, Path: a}, no error expected); delete the old function; delete TestLoadCasesWithoutChecksUnchanged; remove orphaned imports and helpers."
    dependencies: ["checks-judge-and-only"]
    acceptance_criteria:
      - "TestChecksScoringNoCommandRunWithoutOutput, TestPrintCaseResultNoChecksUnchanged, TestChecksValidateNilRubricSkipsCrossCheck and TestLoadCasesWithoutChecksUnchanged no longer exist"
      - "TestChecksValidateRubricCrossCheck passes with a nil-rubric subtest that expects no error from ValidateChecks"
      - "TestChecksScoringNoOutputOrWorkspaceScoresZero, TestPrintCaseResultBreakdownOnlyBelowThreshold and TestLoadCasesClassicTurnOneLoads are unchanged and pass"
    validation_commands:
      - "! grep -nE 'func (TestChecksScoringNoCommandRunWithoutOutput|TestPrintCaseResultNoChecksUnchanged|TestChecksValidateNilRubricSkipsCrossCheck|TestLoadCasesWithoutChecksUnchanged)[(]' internal/eval/*_test.go"
      - "go test -run '^TestChecksValidateRubricCrossCheck$' -v ./internal/eval | grep -E -- '--- PASS: TestChecksValidateRubricCrossCheck/nil'"
      - "go test -run '^(TestChecksScoringNoOutputOrWorkspaceScoresZero|TestPrintCaseResultBreakdownOnlyBelowThreshold|TestLoadCasesClassicTurnOneLoads)$' ./internal/eval"
      - "go test -run 'TestChecks|TestPrintCaseResult|TestLoadCases' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go vet ./internal/eval/..."

  - id: "config-prune"
    agent: "builder"
    description: "Delete TestCostInfoJSONOmitsEmptyNewFields and TestTestCaseParsesStubFromYAML from internal/eval/config_test.go and remove imports orphaned by the deletion."
    dependencies: ["checks-scoring-and-checks"]
    acceptance_criteria:
      - "TestCostInfoJSONOmitsEmptyNewFields and TestTestCaseParsesStubFromYAML no longer exist"
      - "Other config_test.go tests (TestCostFromUsage, TestDefaultConfig and the rest) are unchanged and pass"
    validation_commands:
      - "! grep -nE 'func (TestCostInfoJSONOmitsEmptyNewFields|TestTestCaseParsesStubFromYAML)[(]' internal/eval/*_test.go"
      - "go test -run 'TestCostFromUsage|TestDefaultConfig|TestConfigure|TestRebaseEvalsPath' ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go vet ./internal/eval/..."

  - id: "coverage-and-gate"
    agent: "builder"
    description: "Run the package-wide gates. Confirm gofmt and go vet are clean, run go test -cover ./internal/eval and confirm at least 87.6%. If coverage fell, diff go tool cover -func against the baseline listing, find the lost functions, and restore coverage by adding the missing case or assertion to the relevant named partner (never by re-adding a Delete or Merge function; if impossible, write a BLOCKED sentinel quoting the acceptance criterion). Then run task test and confirm exit 0."
    dependencies: ["config-prune"]
    acceptance_criteria:
      - "go test -cover ./internal/eval reports coverage of at least 87.6% of statements"
      - "task test exits 0"
      - "gofmt -l and go vet ./... are clean"
      - "No non-test .go file and no file under docs/ or .kairon/specs/ (other than this spec) is modified by the PR"
    validation_commands:
      - "test -z \"$(gofmt -l internal cmd)\""
      - "go vet ./..."
      - "go test -cover ./internal/eval 2>&1 | tee .kairon/artifacts/issue-423-after-cover.txt | grep -E 'coverage: (87\\.[6-9]|8[89]\\.[0-9]|9[0-9]\\.[0-9]|100\\.0)%'"
      - "task test"

  - id: "validate-complete"
    agent: "validator"
    description: "Independently verify every acceptance criterion of issue 423: Delete and Merge greps print nothing, each of the 13 merged cases appears among its partner's subtests or assertions with no assertion lost, the moved test exists once without a build tag and passes in the default build, coverage is at least 87.6%, task test exits 0, and the diff touches only test files plus this spec."
    dependencies: ["coverage-and-gate"]
    acceptance_criteria:
      - "The Delete grep from AC1 prints nothing"
      - "The Merge grep from AC2 prints nothing and each partner passes under go test -run '^<partner>$' -v with the merged case present"
      - "grep -ln 'func TestDetectHostArchitecture(' internal/eval/sandbox/*_test.go prints exactly one file with no //go:build line (each of the three parts is a command in this task's validation_commands), and go test -run '^TestDetectHostArchitecture$' -v ./internal/eval/sandbox exits 0 and prints '--- PASS: TestDetectHostArchitecture'"
      - "go test -cover ./internal/eval reports at least 87.6%"
      - "task test exits 0"
      - "git diff against the base shows no changed non-test .go files and no changes to the 15 Delete rows' partners beyond what the issue lists"
      - "Any spec deviation reported by a builder has a code comment and a stated reason, and the work does not narrow an In Scope item"
    validation_commands:
      - "! grep -nE 'func (TestArchitectureCompatibility|TestArchitecturePlatformConsistency|TestAuditReportDuplicates|TestChecksScoringNoCommandRunWithoutOutput|TestCostInfoJSONOmitsEmptyNewFields|TestEvalFrameworkArchitectureIndependence|TestLoadCasesWithoutChecksUnchanged|TestNoKiroCLIProcessOutsideDockerPath|TestPrintCaseResultNoChecksUnchanged|TestRubricChecksOnlyYAML|TestRunnerArchitectureSupport|TestScoreLLMJudgeFailureWording|TestScoredUserTurns|TestSuccessRateIsPercentNotFraction|TestTestCaseParsesStubFromYAML)[(]' internal/eval/*_test.go"
      - "! grep -nE 'func (TestAttemptAccountingOnePerCase|TestAuditDuplicateLineFixture|TestAuditDuplicateRuleOrder|TestAuditEvidenceFixtures|TestAuditOrderingIsStable|TestCandidateE2ELiveAgentsUnchanged|TestCandidateE2ESameContentCandidateHashEqualsBaseline|TestChecksValidateNilRubricSkipsCrossCheck|TestLoadCasesChecksOnlyAllowsFullyCheckedCases|TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase|TestSelftestFailRepeatAttempts|TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough|TestUpdateIncrementalSummaryPromptFile)[(]' internal/eval/*_test.go"
      - "! grep -n 'func TestSandboxArchitectureDetection(' internal/eval/*_test.go"
      - "test \"$(grep -ln 'func TestDetectHostArchitecture(' internal/eval/sandbox/*_test.go | wc -l)\" -eq 1"
      - "f=$(grep -ln 'func TestDetectHostArchitecture(' internal/eval/sandbox/*_test.go) && test -n \"$f\" && ! grep -n '^//go:build' $f"
      - "go test -run '^TestDetectHostArchitecture$' -v ./internal/eval/sandbox"
      - "go test -run '^TestDetectHostArchitecture$' -v ./internal/eval/sandbox | grep -- '--- PASS: TestDetectHostArchitecture'"
      - "go test -run '^(TestAttemptAccountingRepeats|TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals|TestAuditUnmappedFixture|TestAuditDuplicates|TestCandidateE2EKeptWorkspacesHoldCandidate|TestCandidateE2EProvenanceRecorded|TestSummaryMultiAgentOmitsTopLevelAgentFields|TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough|TestLoadCasesChecksOnlyRejectsCheckLessCase|TestChecksValidateRubricCrossCheck)$' -v ./internal/eval"
      - "go test -run '^TestAuditFlatFixtureEvidenceAndDuplicates$' -v ./cmd/kairon/cmd"
      - "go test -cover ./internal/eval 2>&1 | grep -E 'coverage: (87\\.[6-9]|8[89]\\.[0-9]|9[0-9]\\.[0-9]|100\\.0)%'"
      - "task test"
```

## PR description requirements (for krew-lead)

- `Closes #423`
- `Deferred work: none`
- `Spec deviations:` list any builder-recorded deviations with spec section, what was done, and why; otherwise
  `Spec deviations: none`.
- Note that the duplicate-before-unmapped assertion from `TestAuditDuplicateRuleOrder` is on line 7 of
  `TestAuditDuplicates`' prompt, not on "line-6" as the issue's table says: 6 was the line in the old test's own
  prompt, and the builder's code comment at the assertion says the same. This is the issue's intent, so it is not
  listed under `Spec deviations`.
- Name the characterization tests relied on (the 17 partners) and state the before and after
  `go test -cover ./internal/eval` figures.
