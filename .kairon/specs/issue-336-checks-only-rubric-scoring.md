# Issue #336: Evals: score check-based rubrics from checks only

**Closes #336**

## Solution Approach

Add an opt-in rubric field `checks_only` (bool, default `false`). When a rubric sets it, the harness scores every
case from its pass/fail checks only:

- **Scoring (`scoreCase`)**: a non-cost criterion that has no checks in the case is *skipped entirely*: it gets no
  `CriterionScore`, so it is absent from `cr.Scores` and contributes nothing to `caseTotals` / `agentScoreTotals`
  (numerator or denominator). The legacy LLM judge (`scoreLLMJudge`) and the keyword heuristic
  (`scoreDeterministic`) are never reached for such a rubric. This is a single early `continue` in the existing
  criteria loop, placed right after the "criterion has checks" branch. Everything else (checked criteria,
  no-output handling, the "check names a criterion not in the rubric" fallback) is unchanged.
- **Load-time guard (`loadCases`)**: for a `checks_only` rubric every case must have at least one check, otherwise
  `loadCases` returns an error wrapping `errInvalidChecks` that names the case file (`e.Name()`). `Run` already
  turns `errInvalidChecks` / `errMalformedCase` into a run failure (`runProgressiveEvaluation`), and
  `RunSingleTestCase` / `--list` surface it through `GetTestCase` / `loadCases`, so the run fails non-zero.
- **"Before any case runs"**: `runProgressiveEvaluation` loads each rubric's cases inside its per-agent loop, so
  with several agents an earlier agent's cases would already have run when a later agent is rejected. Add a small
  preflight at the top of that function: for every rubric with `ChecksOnly`, call `loadCases(rubric.Agent)` and
  return the error if it is `errInvalidChecks` / `errMalformedCase`, before the first `evaluateProgressive`.
  (Non-`checks_only` rubrics are untouched, so their behavior is identical to today.)
- **No behavior change without the flag (AC 6)**: the new branch is guarded by `rubric.ChecksOnly`. The default is
  `false`, so all existing rubrics (including every one under `.kairon/evals/`, which is out of scope) score as
  before.

### Decisions and clarifications

- "Legacy judge" means `scoreLLMJudge` (role `judge`, one call per unchecked non-deterministic criterion).
  `judge`-type **checks** (`checks_judge.go`) and the simulated-user router also use role `judge`; they are *not*
  removed and remain allowed in a `checks_only` rubric. The new self-test cases must therefore use no `judge`
  checks and no `simulated_user`, so that AC 3 ("no `calls` record with role `judge`") is verifiable.
- A `checks_only` criterion with checks and no agent output keeps today's behavior: `checkedScore` returns
  `0/<check count>` with reasoning `"agent produced no output; checks not run"` (AC 5 needs no code change, only a
  test and fixture). The failing agent run exits non-zero through the existing `thresholdFailure` path.
- The `scoring` and `deterministic` fields of a criterion are ignored in a `checks_only` rubric (a checked
  criterion's max is its check count, as today). Do not add validation for them.
- No result-JSON schema change; `checks_only` is not stamped into results.
- The field name `checks_only` and its YAML key are fixed by the issue's constraints.

## Relevant Files

Modify:
- `internal/eval/types.go`: add `ChecksOnly bool \`yaml:"checks_only,omitempty" json:"checks_only,omitempty"\`` to `Rubric`, with a doc comment.
- `internal/eval/scoring.go`: skip unchecked non-cost criteria when `rubric.ChecksOnly`; update the `scoreCase` doc comment.
- `internal/eval/runner.go`:
  - `loadCases`: load the agent's rubric (lazily, once) not only when a case has checks but also to enforce `checks_only`; reject a check-less case with a `errInvalidChecks`-wrapped error naming `e.Name()`.
  - `runProgressiveEvaluation`: preflight loop over `checks_only` rubrics before any agent is evaluated.
- `internal/eval/exit_status_test.go`: `TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent` runs all rubrics and expects exactly two agents; also remove the two new `selftest-checks*.yaml` rubrics there (alongside the sandbox rubrics it already removes).
- `internal/eval/scoring_test.go` (or `checks_scoring_test.go`): new unit tests (see tasks).
- `internal/eval/selftest_checks_test.go`: new self-test assertions for the new agents.
- `docs/evaluation.md`: document `checks_only` (Rubric Format fields list + example; "How Scoring Works"; Self-Test fixture listing).

Create (test fixtures, all under `internal/eval/testdata/`):
- `evals/agents/selftest-checks.json`, `evals/agents/selftest-checks-prompt.md`: copy the shape of `selftest-fail.json` / `selftest-fail-prompt.md`.
- `evals/agents/selftest-checks-fail.json`, `evals/agents/selftest-checks-fail-prompt.md`: same, for the failing agent.
- `evals/rubrics/selftest-checks.yaml`: `checks_only: true`; two scored criteria (e.g. `file_written`, `report_marker`, both `scoring: 1-5`; neither `deterministic`) plus `cost_efficiency` (`type: cost`).
- `evals/rubrics/selftest-checks-fail.yaml`: `checks_only: true`, same two criteria (+ cost).
- `evals/cases/selftest-checks/check-one-criterion.yaml`: stub turn writes `marker.txt`; one `file_exists` check on `file_written` only (so `report_marker` has no checks).
- `evals/cases/selftest-checks/check-both-criteria.yaml`: checks on both criteria (e.g. `file_exists` on `file_written`, `output_contains` on `report_marker`), all passing.
- `evals/cases/selftest-checks-fail/stub-empty-response.yaml`: `stub.turns: [{response: ""}]`, with checks on both criteria (at least one check on each; e.g. two on `file_written`, one on `report_marker`) so the 0-of-N-per-criterion assertion is non-trivial.
- `checks-only-invalid/rubrics/selftest-checks.yaml` (`agent: selftest-checks`, `checks_only: true`), `checks-only-invalid/agents/selftest-checks.json` + prompt (needed by the pin pre-flight), and `checks-only-invalid/cases/selftest-checks/no-checks.yaml` (a valid case with *no* `checks:` block). Name the case file distinctively so the test can assert it appears in the error.

Read for context (do not edit): `internal/eval/checks.go` (`ValidateChecks`, `errInvalidChecks`), `internal/eval/execute_case.go`, `internal/eval/verdict.go`, `internal/eval/sandbox_workspace_test.go` (`wantThresholdFailure`, see below).

Not to touch (out of scope / frozen per AGENTS.md): `.kairon/evals/` rubrics and cases, `.kairon/specs/` other than this file, `CHANGELOG.md`. The `.kairon/evals/` tree is template-synced, but this change does not modify it, so `task sync:check` is unaffected.

## Team Orchestration

- `core-scoring` (code + unit tests, TDD) and `docs-checks-only` (docs only) have no dependency and run in parallel.
- `selftest-fixtures` needs the working implementation (its tests assert end-to-end behavior and the stub fixtures only load meaningfully once `ChecksOnly` is parsed), so it depends on `core-scoring`.
- `validate-all` runs last (read-only) and depends on all three.
- AGENTS.md mandates TDD: in each code task write the failing test first, run it and confirm it fails for the expected reason (not a compile error or typo; to avoid a compile-error "failure", add the struct field first or assert via YAML-loaded rubrics), then implement, re-run, and commit test + implementation together. Name the test seen failing first in the commit message / PR description. Docs-only changes need no test first.
- `selftest-checks-fail` is a *new* expected-to-fail agent, so `wantThresholdFailure` in `sandbox_workspace_test.go` must list it (`case "selftest-fail", "selftest-sandbox-ro", "selftest-checks-fail"`), and `TestWantThresholdFailure` should get rows for `selftest-checks` (false) and `selftest-checks-fail` (true). Check that every other test enumerating the self-test agents (e.g. `parity_sandbox_test.go`, which only iterates `selftest` and `selftest-fail`, and the all-rubrics run in `exit_status_test.go`) still holds; run the full `go test ./internal/eval/...` to find any other enumeration.

## Step-by-Step Task Breakdown

### Task 1: `core-scoring`: `checks_only` field, scoring branch, load-time guard (TDD)
1. Tests first (unit level, temp dirs / hand-built `Rubric`, no stub run needed):
   - `scoreCase` with `Rubric{ChecksOnly: true}`, two criteria, a case that checks only one: expect exactly one `CriterionScore` (the checked one), `caseTotals` equals the check counts, and no `cr.Calls` record (use the existing mock/inference seam used by `scoring_test.go` for the judge to assert zero judge calls; also a `deterministic: true` unchecked criterion yields no score).
   - Same rubric with `ChecksOnly: false` still yields the unchecked criterion scored by judge/heuristic (characterization of AC 6; reuse existing coverage if already present, and add if not).
   - `checks_only` with empty `ActualOutput`: checked criterion scores `0/N`, `Skipped` false, reasoning `noOutputChecksReasoning`, unchecked criterion absent.
   - Rubric YAML with `checks_only: true` loads with `ChecksOnly == true`; absent key gives `false`.
   - `loadCases` on a temp evals dir whose rubric is `checks_only` and which holds a case without `checks:` returns an error satisfying `errors.Is(err, errInvalidChecks)` and containing the case file name; the same case dir under a non-`checks_only` rubric loads fine.
   - Preflight: a two-agent temp evals dir where agent A is valid and agent B is `checks_only` with a check-less case; running all agents fails with the file name in the error and A's results file is never written (no case ran).
2. Confirm the tests fail for the expected reason, then implement `types.go`, `scoring.go`, and the `runner.go` changes described above. Keep the change minimal.
3. Run `go test ./internal/eval/...`; fix only enumerations affected by this task.

### Task 2: `docs-checks-only`: document the field (no test required)
In `docs/evaluation.md`:
- Rubric Format: add a `checks_only: true` line with comment to the YAML example (or a second short example rubric) and a `checks_only` bullet in the Fields list: boolean, default `false`; unchecked criteria are not scored for that case and add nothing to the case or agent score or maximum; no legacy judge call and no keyword heuristic; every case of the agent must have at least one check or loading fails naming the case file; `judge` checks are still allowed; a checked criterion still scores `0/N` when the agent produced no output.
- How Scoring Works: add a short "`checks_only` rubrics" paragraph/bullet stating the above and that rubrics without the field are scored exactly as before.
- Self-Test: add the new fixtures to the fixtures listing (`agents/selftest-checks*.json`, `rubrics/selftest-checks*.yaml`, `cases/selftest-checks/*`, `cases/selftest-checks-fail/stub-empty-response.yaml`, `checks-only-invalid/`), noting `selftest-checks-fail` is expected to fail and `checks-only-invalid` is a deliberately rejected fixture.

### Task 3: `selftest-fixtures`: fixtures and end-to-end self-tests (TDD)
Depends on Task 1. Create the fixtures listed above and extend `selftest_checks_test.go` (using `runSelfTestAgent` / `runEvalIn` / `copyFixturesTo`):
- `selftest-checks`: `check-one-criterion` records a score for `file_written` only (`report_marker` absent), `check-both-criteria` records both; both fully pass; no `Calls` entry with `Role == "judge"` in either result; agent run passes (no `ErrThresholdFailed`); case/agent totals equal the check counts.
- `selftest-checks-fail`: `stub-empty-response` records `0/N` for each checked criterion, run returns `ErrThresholdFailed`.
- `checks-only-invalid`: running with `--evals-dir internal/eval/testdata/checks-only-invalid` returns a non-nil non-threshold error that names the case file, with no case executed (no result file for the agent).
- Update `wantThresholdFailure` / `TestWantThresholdFailure` and the all-rubrics test as described in Team Orchestration.
- Confirm the existing `selftest` / `selftest-fail` expectations are unchanged (AC 6), including `TestSelfTestChecksPassingCases` (`clarity` still an unchecked maximum score).

### Task 4: `validate-all`: independent verification (read-only)
Run the validation commands below and check each acceptance criterion; report per-criterion pass/fail.

## Acceptance Criteria Traceability

| AC | Delivered by | Verified by |
|----|--------------|-------------|
| 1 | `types.go`, Task 2 docs | `grep -n checks_only docs/evaluation.md` shows the field list entry and an example (manual read) |
| 2 | `scoring.go` early `continue` | `selftest-checks` run, `check-one-criterion` has one `CriterionScore` (`file_written`) |
| 3 | same (judge/heuristic unreachable) | results JSON of the AC 2 run has no `calls[].role == "judge"` |
| 4 | `loadCases` guard + preflight | `checks-only-invalid` run: non-zero exit, error names the case file |
| 5 | existing `checkedScore` no-output path + new fixture | `selftest-checks-fail` run: `0/N` per checked criterion, non-zero exit |
| 6 | all changes guarded by `rubric.ChecksOnly` | `task eval:selftest` per-case scores identical to base; full `go test ./internal/eval/...` |

## Validation Commands

```bash
go build ./...
go vet ./internal/eval/...
go test ./internal/eval/... -count=1
task lint
task fmt:check

# AC 2 / 3: passes; inspect the newest results for selftest-checks.json
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks
# AC 4: expect non-zero exit and an error naming the case file
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/checks-only-invalid selftest-checks; echo $?
# AC 5: expect non-zero exit
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks-fail; echo $?
# AC 6
task eval:selftest
task sync:check
```

Note: runs write into `<evals-dir>/results/`; delete any `internal/eval/testdata/**/results` directory created by manual runs before committing (existing `.gitignore` rules may already cover it; verify with `git status`).

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "core-scoring"
    agent: "builder"
    description: "TDD: add Rubric.ChecksOnly (checks_only), skip unchecked non-cost criteria in scoreCase when set (no legacy judge call, no keyword heuristic), reject check-less cases of a checks_only rubric in loadCases with an errInvalidChecks error naming the case file, and add a preflight in runProgressiveEvaluation so rejection happens before any case runs. Write failing unit tests first, then implement; commit tests and implementation together."
    dependencies: []
    acceptance_criteria:
      - "Rubric has a ChecksOnly field loaded from YAML key checks_only (default false)"
      - "scoreCase with a checks_only rubric produces CriterionScores only for criteria that have checks in the case; unchecked criteria are absent and add nothing to caseTotals or agentScoreTotals"
      - "A checks_only rubric makes no scoreLLMJudge call and runs no scoreDeterministic heuristic"
      - "loadCases returns an error wrapping errInvalidChecks that names the case file when a checks_only rubric's agent has a case with no checks; the same case under a non-checks_only rubric still loads"
      - "runProgressiveEvaluation rejects an invalid checks_only agent before any agent's first case runs"
      - "A checked criterion with empty agent output still scores 0 of its check count"
      - "Rubrics without checks_only score exactly as before (characterization test passes before and after)"
      - "The test seen failing first is named in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"

  - id: "docs-checks-only"
    agent: "builder"
    description: "Document the checks_only rubric field in docs/evaluation.md: Rubric Format field list plus example, How Scoring Works paragraph, and the new self-test fixtures in the Self-Test section."
    dependencies: []
    acceptance_criteria:
      - "docs/evaluation.md Rubric Format section shows checks_only in an example and lists it in Fields (boolean, default false)"
      - "Docs state unchecked criteria are not scored and add nothing to case/agent score or maximum, no legacy judge call or keyword heuristic is made, and judge checks remain allowed"
      - "Docs state a check-less case in a checks_only rubric's agent is rejected before any case runs, naming the case file"
      - "Docs state a checked criterion still scores 0/N when the agent produced no output, and that rubrics without the field score as before"
      - "Self-Test fixture listing mentions selftest-checks, selftest-checks-fail and checks-only-invalid"
    validation_commands:
      - "grep -n 'checks_only' docs/evaluation.md"
      - "grep -n 'selftest-checks' docs/evaluation.md"

  - id: "selftest-fixtures"
    agent: "builder"
    description: "TDD: add the stub-backend fixtures (selftest-checks and selftest-checks-fail agents, agent configs/prompts, rubrics, cases; checks-only-invalid evals dir) and end-to-end tests in selftest_checks_test.go; update wantThresholdFailure/TestWantThresholdFailure and the all-rubrics run in exit_status_test.go so the new agents do not break existing enumerations."
    dependencies: ["core-scoring"]
    acceptance_criteria:
      - "Running selftest-checks via the stub backend: check-one-criterion records a score for the checked criterion only, check-both-criteria records both, all pass, run exits zero"
      - "The selftest-checks results contain no calls record with role judge (fixtures use no judge checks and no simulated_user)"
      - "Running with --evals-dir internal/eval/testdata/checks-only-invalid selftest-checks exits non-zero with an error naming the check-less case file and runs no case"
      - "selftest-checks-fail/stub-empty-response records 0 for each checked criterion and the agent run returns ErrThresholdFailed (non-zero exit)"
      - "wantThresholdFailure treats selftest-checks-fail as expected-to-fail and selftest-checks as expected-to-pass; TestWantThresholdFailure covers both"
      - "TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent and every other existing test still pass; selftest and selftest-fail per-case scores are unchanged"
    validation_commands:
      - "go test ./internal/eval/... -count=1"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks"
      - "bash -c 'go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/checks-only-invalid selftest-checks; test $? -ne 0'"
      - "bash -c 'go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks-fail; test $? -ne 0'"
      - "task eval:selftest"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion of issue #336 against the implementation, docs and fixtures; also confirm AGENTS.md TDD and scope rules were followed (no changes under .kairon/evals/, CHANGELOG.md or other specs)."
    dependencies: ["core-scoring", "docs-checks-only", "selftest-fixtures"]
    acceptance_criteria:
      - "AC 1: docs/evaluation.md documents checks_only with an example"
      - "AC 2 and 3: the selftest-checks run records a score only for the checked criterion in check-one-criterion and its results have no calls record with role judge"
      - "AC 4: the checks-only-invalid run exits non-zero and the error names the case file"
      - "AC 5: selftest-checks-fail records 0 for each checked criterion and exits non-zero"
      - "AC 6: task eval:selftest passes and per-case scores for selftest and selftest-fail are unchanged"
      - "go build, go vet, full go test ./internal/eval/..., task lint and task fmt:check all pass"
      - "git diff touches nothing under .kairon/evals/, CHANGELOG.md or other .kairon/specs files"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/... -count=1"
      - "task lint"
      - "task fmt:check"
      - "task eval:selftest"
      - "task sync:check"
      - "git diff --stat origin/main -- .kairon/evals CHANGELOG.md"
```
