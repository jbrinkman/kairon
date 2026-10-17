# Issue #342: Evals: report per-criterion scores and an optional floor

**Closes #342**

## Solution Approach

An agent's verdict is decided on one pooled number (`agentScoreTotals`, `internal/eval/scoring.go`), so a criterion
with few checks can score 0% while the agent still passes. The verdict also carries no per-criterion numbers. The
change has two parts. Neither touches how the agent `score` is computed (out of scope).

1. **Report.** Pool each non-cost criterion's score and maximum across every case and every repeat of an agent, and
   put the result on the agent's verdict as a new `criteria` array (and print it).
2. **Floor.** A new optional rubric field `min_criterion_score` (percent, unset by default). When set, the agent
   fails if any criterion's pooled percent is below it, even if the overall score meets `pass_threshold`. The FAIL
   line and the error name the offending criterion.

### Design decisions

**Where the aggregation lives.** Add `agentCriterionTotals(ar AgentResult) []CriterionTotal` to `scoring.go`, next to
`agentScoreTotals` and built on the same rule as `caseTotals`: a `Skipped` criterion contributes 0 to the numerator
and its `MaxScore` to the denominator. Extract the skipped rule into one small helper used by both `caseTotals` and
the new function, so the arithmetic stays in one place. The invariant is: the sum of the criteria's `Score`s and the
sum of their `MaxScore`s equal `agentScoreTotals`. A unit test must pin this invariant.

**Pooling.** The function iterates `ar.Cases`. That slice already holds every run of every case, so repeats are
pooled by construction. Score and maximum are integer sums. With `--repeat N` the maximum is N times the single-run
maximum, and the percent equals the mean over repeats. This matches how the agent `score` is pooled.

**Which criteria appear, and why AC4 already holds.** A criterion is listed if it has at least one `CriterionScore`
in some case. Under `checks_only`, `scoreCase` already emits no `CriterionScore` for a criterion without checks in a
case, so that case adds nothing to the criterion's maximum. AC4 therefore needs no scoring change. It needs a test,
because the per-criterion maximum is now observable. A non-cost criterion that no case ever scored has nothing
measured (0/0), so it is absent from the list and is never judged against the floor. Entries with `MaxScore` 0 are
dropped, so a percent is never computed over a zero denominator. Cost criteria are never listed because `scoreCase`
never produces a score for them.

**Order.** Criteria appear in first-seen order across `ar.Cases`. For a normal rubric this is rubric order.

**Percent and the boundary.** `percent = score*100/max`. The floor comparison reuses `percentAtLeast(score, max,
floor)`, which multiplies before dividing. A criterion passes the floor when `percentAtLeast` is true. A percent
exactly equal to the floor passes. This gives the same exact boundary behaviour as `pass_threshold`.

**Floor is carried on the result, not looked up.** `agentVerdict(ar)` is a pure function of the `AgentResult` (the
verifylog recompute calls it with no rubric). So the floor must be stamped on the result, exactly as `Threshold` is:
add `AgentResult.MinCriterionScore *float64` (`json:"min_criterion_score,omitempty"`). Set it from the rubric in both
`evaluate` and `evaluateProgressive`, and re-stamp it in the `--resume` branch next to `result.Threshold =
rubric.Threshold()`. A result without the field (nil) means no floor, so legacy result files and every rubric
without the field behave exactly as before (AC3).

**Rubric field.** `Rubric.MinCriterionScore *float64` (`yaml:"min_criterion_score,omitempty"`), a pointer so unset
is distinguishable. Validate it in `loadRubrics` like `pass_threshold`: a percent in `(0, 100]`, written as a negated
range check so NaN is rejected. An error is `invalid rubric <file>: min_criterion_score must be a percent in (0,
100], got <v>`. The value `0` is rejected rather than treated as "off", which matches `pass_threshold`'s range.
`audit.go` unmarshals a `Rubric` too, but only reads criteria, so it needs no change.

**Verdict shape (relied on by the iteration and promotion issues, so the names are fixed here).**

```go
// CriterionTotal is one non-cost criterion pooled over every case and repeat of an agent.
type CriterionTotal struct {
    Name     string  `json:"name"`
    Score    int     `json:"score"`     // points, summed over cases and repeats (skipped counts as 0)
    MaxScore int     `json:"max_score"` // summed maxima
    Percent  float64 `json:"percent"`   // score*100/max_score, 0-100
}
```

`AgentVerdict` gains:

| Field | JSON | Notes |
|-------|------|-------|
| `Criteria []CriterionTotal` | `criteria` | Always written, `[]` when nothing is scored (like `repeat_scores`). |
| `MinCriterionScore *float64` | `min_criterion_score,omitempty` | Echo of the floor; absent when unset. |
| `CriteriaBelowFloor []string` | `criteria_below_floor,omitempty` | Names, in `criteria` order; absent when none. |

`Passed` becomes `scorePasses && len(CriteriaBelowFloor) == 0`. `Score`, `Threshold`, `ScoreMin`, `ScoreMax`,
`RepeatScores`, `CasesTotal` and `CasesFailed` are computed exactly as today. Adding `criteria` to every verdict is
additive. `agent_scores` is untouched.

**Fail-closed.** With nothing scored, `Passed` is already false, and there are no criteria to name.

**Output.** `printAgentVerdict` keeps its first line byte-for-byte when there is no floor failure. `TestPrintAgentVerdictFormat`
and the repeat-format tests build verdicts without `Criteria`, so they must keep passing unchanged.
- After the verdict line, when `v.Criteria` is non-empty, print one indented line per criterion, for example
  `  file_written: 2/2 (100.0%)`. These lines do not start with `PASS `/`FAIL `, so `grep '^PASS \|^FAIL '` still works.
- When criteria are below the floor, append to the FAIL line, after the existing text and before any repeat suffix:
  `; criterion below floor 50.0%: report_marker 0.0% (0/1)`. For several criteria, join them with `, `
  (`criteria below floor 50.0%: a 10.0% (1/10), b 0.0% (0/1)`).

**Error and exit status.** `AgentFailure` gains `Floor float64` and `BelowFloor []CriterionTotal`. `ThresholdError.Error()`
keeps today's exact text when `BelowFloor` is empty. When criteria are below the floor it names them, and it must
not print a misleading `97.0% < 95.0%` when the overall score met the threshold. For example:
`selftest-floor (criterion report_marker 0.0% < floor 50.0%)`. Introduce one helper, `failureOf(agent string, v
AgentVerdict) AgentFailure`, and use it at the three sites that build an `AgentFailure` today (`RunSingleTestCase`
~line 329, and the all-agents loop ~line 1922; builders should grep for `AgentFailure{`). The exit status stays 1 via
`thresholdFailure`.

**Single-case runs.** `kairon eval <agent> <case>` computes the verdict from the one case that ran, so the floor
applies to that case's criteria. This is the same function, so the docs just state it.

**Template sync.** None. The change touches `internal/eval/**` and `docs/evaluation.md`. Neither is
template-synchronized, and the `.kairon/evals/` tree (which is) is out of scope. Run `task sync:check` once at
the end to confirm it is still clean.

## Relevant Files

Modify:
- `internal/eval/types.go`: `Rubric.MinCriterionScore`, `AgentResult.MinCriterionScore`, new `CriterionTotal`.
- `internal/eval/scoring.go`: `agentCriterionTotals`, a shared skipped-score helper used by `caseTotals`.
- `internal/eval/verdict.go`: new `AgentVerdict` fields, floor evaluation in `agentVerdict`, `printAgentVerdict`,
  `AgentFailure`, `ThresholdError.Error`, `failureOf`.
- `internal/eval/runner.go`: `loadRubrics` validation; stamp the floor in `evaluate`, `evaluateProgressive` and its
  resume branch; use `failureOf` at the `AgentFailure{` sites.
- `docs/evaluation.md`: see Task 3.
- Tests: `internal/eval/verdict_test.go`, `internal/eval/scoring_test.go`, `internal/eval/threshold_test.go`
  (rubric load validation, next to `TestLoadRubricsPassThreshold`), `internal/eval/exit_status_test.go`
  (`TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent` removes the extra self-test rubrics. Add
  `selftest-floor.yaml` to that list), `internal/eval/sandbox_workspace_test.go` (`wantThresholdFailure` and
  `TestWantThresholdFailure`).

Create:
- `internal/eval/selftest_criteria_test.go`: end-to-end tests for AC1, AC3 and AC4 (run via `runSelfTestAgent` /
  `runEvalIn` helpers).
- `internal/eval/selftest_floor_test.go`: end-to-end tests for AC2.
- Fixtures under `internal/eval/testdata/evals/` (all new, modelled on `selftest-checks*`):
  - `agents/selftest-floor.json`, `agents/selftest-floor-prompt.md`. The pin pre-flight needs the agent config.
  - `rubrics/selftest-floor.yaml`:
    ```yaml
    agent: selftest-floor
    checks_only: true
    pass_threshold: 60
    min_criterion_score: 50
    criteria:
      - name: file_written
        description: "The agent created the marker file"
        scoring: 1-5
      - name: report_marker
        description: "The report mentions the output marker"
        scoring: 1-5
      - name: cost_efficiency
        description: "Token usage relative to output quality"
        type: cost
    ```
  - `cases/selftest-floor/floor-one-criterion-zero.yaml`: the stub turn runs `echo hi > marker.txt` and responds
    with text that does NOT contain `output marker alpha`. Checks: `file_written` gets `file_exists marker.txt` and
    `file_contains marker.txt (?m)^hi$` (both pass). `report_marker` gets `output_contains "output marker alpha"`
    (fails). Result: file_written 2/2 (100%), report_marker 0/1 (0%). The overall score is 2/3 = 66.7%, above
    `pass_threshold: 60`, but `report_marker` is below the 50% floor, so the agent FAILs. `pass_threshold: 60` is
    deliberate: it keeps the fixture to three checks and makes "overall above threshold, one criterion at 0%" true.
    The case's own threshold is also 60, so the case passes and `cases_failed` is 0; only the floor fails the agent.

Read for context (do not edit): `internal/eval/checks_only_test.go`, `internal/eval/selftest_checks_test.go`,
`internal/eval/repeat_test.go`, `internal/eval/verifylog.go` (it calls `agentVerdict(...).Score`, unaffected).

Not to touch (AGENTS.md / issue scope): `.kairon/evals/` (rubrics and cases), `.kairon/specs/` other than this
file, `CHANGELOG.md`, `cmd/kairon/templates/`.

## Team Orchestration

- `criterion-report` and `docs-criterion-scores` have no dependency and run in parallel. The docs task uses the
  field names fixed in this spec, so it does not need the code to exist.
- `criterion-floor` edits the same files (`verdict.go`, `types.go`, `runner.go`) and builds on the new verdict
  fields, so it depends on `criterion-report`.
- `validate-all` (read-only) runs last and depends on all three.
- **TDD (AGENTS.md).** In each code task: write the failing test first, run it, and confirm it fails for the
  expected reason, not for a compile error. Add the new struct fields and empty function stubs first so the tests
  compile and fail on assertions. Then implement, re-run, and commit tests and implementation together. Name the
  test seen failing first in the commit message. Docs need no test first.
- **Baseline for AC3.** Before the first code edit, the `criterion-report` builder captures the base-commit
  self-test scores outside the repo:
  `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest` and then
  `jq -S '[.cases[] | {c: .case_name, s: [.scores[] | [.name, .score, .max_score]]}]'
  internal/eval/testdata/evals/results/<newest run dir>/selftest.json > /tmp/issue-342-selftest-baseline.json`.
  Later tasks and the validator compare against it with the same `jq`. (`internal/eval/testdata/evals/results/` is
  git-ignored. Do not commit anything from it.)
- Any test that enumerates every rubric or every self-test agent must learn about `selftest-floor`. The known
  ones are `exit_status_test.go` and `sandbox_workspace_test.go`. Run the full `go test ./internal/eval/...` to find
  others (for example anything counting rubrics or agents in the evals dir).

## Step-by-Step Task Breakdown

### Task 1: `criterion-report`: per-criterion totals on the verdict (TDD)
1. Capture the AC3 baseline (see Team Orchestration).
2. Add `CriterionTotal`, `AgentVerdict.Criteria`, and a stub `agentCriterionTotals`.
3. Write failing tests:
   - Unit (`scoring_test.go` / `verdict_test.go`): pooling over several cases; pooling over repeats (Run 1..N);
     a `Skipped` criterion counts 0 with its max kept; first-seen order; cost criteria absent; a criterion present
     in only some cases (the `checks_only` shape) has a maximum equal to the number of cases that scored it;
     `MaxScore` 0 entries dropped; percent exactness (19/20 = 95.0); the sum-equals-`agentScoreTotals` invariant;
     `criteria` is `[]` (not null) in the JSON of an empty verdict; verdict `Score`/`Passed`/`CasesFailed` are
     identical to before for a table of pre-existing inputs.
   - Print (`exit_status_test.go` style): `printAgentVerdict` with criteria prints the unchanged first line, then
     one indented line per criterion. With no `Criteria`, output is byte-identical to today.
   - End to end (`selftest_criteria_test.go`):
     - AC1: run `selftest` on the stub backend. `summary.json`'s `agent_verdicts.selftest.criteria` lists
       `structural_completeness` and `clarity` (not `cost_efficiency`), and each entry's `score`, `max_score` and
       `percent` equal the sum over `selftest.json`'s cases.
     - AC4: run `selftest-checks`. `file_written` has `max_score` 2 and `report_marker` has `max_score` 1, i.e. each
       equals its number of checks across the two cases (`check-one-criterion` has no `report_marker` check).
       Derive the expectation by counting checks in the loaded cases, not by hard-coding 2 and 1.
     - AC3: `selftest` still exits 0 with unchanged per-case scores, and the same `AgentResult` yields the same
       `Score`/`Passed`/exit outcome as before.
4. Implement `agentCriterionTotals`, the shared skipped helper (refactor `caseTotals` to use it; its existing
   tests are the characterization tests), the `agentVerdict` wiring, and the `printAgentVerdict` criterion lines.
   `setAgentVerdict` and `buildSummary` need no change because they call `agentVerdict`.
5. Run the whole `internal/eval` suite, then compare the baseline with `jq`.

### Task 2: `criterion-floor`: `min_criterion_score` (TDD)
1. Add `Rubric.MinCriterionScore`, `AgentResult.MinCriterionScore`, and the three `AgentVerdict`/`AgentFailure`
   fields as stubs. Create the `selftest-floor` fixtures (agent json + prompt, rubric, case) as listed above.
2. Write failing tests:
   - Rubric load (`threshold_test.go`): the field loads from YAML; absent is nil; `0`, `-1`, `100.5` and NaN are
     rejected with an error naming the rubric file and `min_criterion_score`; `100` and `0.5` are accepted.
   - Verdict unit tests (`verdict_test.go`): floor unset gives an identical verdict to today, including with a 0%
     criterion at overall 95%+ (the issue's scenario: one 1-check criterion at 0/1 plus 19 passing checks is 95% and
     passes). With the floor set to 50 the same result fails, `Passed` is false, `CriteriaBelowFloor` is the
     criterion name, and `Score` is unchanged. Boundary: exactly 50% passes and 49/100 fails. Several criteria below
     the floor are all named in `criteria` order. With repeats, the floor is judged on the pooled percent. Floor set
     and overall below threshold: fails, and the criteria below floor are still listed. Nothing scored: fails
     closed.
   - Print/error: the FAIL line ends with `; criterion below floor 50.0%: report_marker 0.0% (0/1)` and, with
     repeats, still carries the `[repeat ...]` suffix last. `ThresholdError.Error()` for a floor-only failure
     names the criterion and does not print `97.0% < 95.0%`. It is unchanged for a threshold-only failure (the
     existing `TestThresholdErrorWrapsSentinelAndNamesAgents` still passes).
   - End to end (`selftest_floor_test.go`), the test seen failing first: run `selftest-floor` through
     `RunWithOptions` with the stub backend, natively. It returns an error that `errors.Is(err, ErrThresholdFailed)`
     and whose text names `report_marker`. Stdout has a `FAIL selftest-floor:` line naming `report_marker`. The
     saved `summary.json` verdict has `passed: false`, `score` ≈ 66.7, `min_criterion_score: 50` and
     `criteria_below_floor: ["report_marker"]`. `selftest-floor.json` carries `min_criterion_score: 50`. The case
     itself passed (`cases_failed` 0). Before the implementation this fails because the key is ignored and the
     run passes (`err == nil`), which is the expected reason.
   - Control (same file): copy the evals dir, delete the `min_criterion_score:` line from the copy's rubric, and
     run again. It passes with exit 0 and the same `score`, proving the floor is the only cause (AC3).
   - Resume: `--resume` of a `selftest-floor` run keeps the floor (it is re-stamped from the rubric).
3. Implement: rubric field and `loadRubrics` validation; stamp the floor in `evaluate`, `evaluateProgressive` and
   the resume branch; floor evaluation and `Passed` in `agentVerdict`; `printAgentVerdict` FAIL suffix;
   `AgentFailure`/`ThresholdError`/`failureOf` and the three call sites.
4. Update enumerations: add `selftest-floor.yaml` to the removal list in
   `TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent`. Add `selftest-floor` to `wantThresholdFailure`
   (expected to fail, native and sandbox) and add rows to `TestWantThresholdFailure`. Check `parity_sandbox_test.go`
   (it iterates only `selftest` and `selftest-fail`) and run the full suite for others.
5. Confirm that `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-floor`
   exits non-zero, that `task eval:selftest` exits 0, and that its per-case scores match the baseline.

### Task 3: `docs-criterion-scores`: `docs/evaluation.md`
- **Rubric Format** (line ~43): add `min_criterion_score: 50   # Optional ...` to an example and a `Fields` bullet
  next to `pass_threshold`. Say: percent in `(0, 100]`, unset by default, `0`, above 100 and NaN fail loading with an
  error naming the file, compared with `>=` (exactly at the floor passes), and that without it nothing changes.
- **Agent verdict** (line ~926): explain that the agent score pools points across all checks, so a criterion with
  fewer checks counts for less. Define the floor rule: the agent fails if any listed criterion's pooled percent is
  below `min_criterion_score`, even when `score >= threshold`. It is judged on the pooled (mean over repeats)
  percent. Criteria no case scored (all unchecked in a `checks_only` rubric) have nothing measured, are not listed,
  and are not judged. A single-case run applies the floor to that case's criteria. `cases_failed` is unaffected.
- **Output and exit status** (line ~940): show the per-criterion lines and the FAIL-line suffix with an example, and
  the new error text for a floor failure.
- **Percent versus fraction** table and **Run summary** (`summary.json`) verdict field table (line ~1293+): add
  `criteria[]` (`name`, `score`, `max_score` (points, **not** percent), `percent` (percent)), `min_criterion_score`
  and `criteria_below_floor`. Update the example JSON with a `criteria` array. State that `score`/`max_score` are
  pooled over all cases and repeats, so with `--repeat N` the maximum is N times the single-run maximum. Note that
  verdict `score` is percent while a criterion's `score` is points.
- **How Scoring Works** (line ~859): one paragraph that, under `checks_only`, a criterion without checks in a
  case adds nothing to that criterion's maximum, so its maximum is its number of checks across the cases.
- **Result JSON** for `<agent>.json`: mention `min_criterion_score` appears at the top level when the rubric sets it.
- **Self-Test** (line ~1956 and ~2101): list `agents/selftest-floor.json`, its prompt, `rubrics/selftest-floor.yaml`,
  `cases/selftest-floor/floor-one-criterion-zero.yaml`, and a paragraph that `selftest-floor` is expected to FAIL (exit
  1) because `report_marker` is at 0% under a 50% floor although the overall 66.7% is above its 60% threshold, with
  the run command. Add `selftest-floor` to the "expected-to-fail" note beside `selftest-fail` and `selftest-checks-fail`.
- Do not edit `.kairon/specs/` or `CHANGELOG.md`.

### Task 4: `validate-all`: read-only verification
Check every acceptance criterion below, plus AGENTS.md compliance (tests committed with implementation; no edits
under `.kairon/evals/`, `.kairon/specs/` except this file, `CHANGELOG.md`, or `cmd/kairon/templates/`).

## Validation Commands

```bash
go build ./...
go vet ./internal/eval/...
go test ./internal/eval/... -count=1
go test ./cmd/... -count=1

# AC1: criteria listed in the selftest verdict (newest run dir)
task eval:selftest
jq '.agent_verdicts.selftest.criteria' "$(ls -dt internal/eval/testdata/evals/results/*/ | head -1)summary.json"

# AC2: floor failure
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-floor; echo $?   # non-zero, FAIL line names report_marker

# AC3: unchanged scores and exit code
task eval:selftest; echo $?    # 0, per-case scores equal /tmp/issue-342-selftest-baseline.json

# AC4: per-criterion maximum equals its number of checks
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks
jq '.agent_verdicts["selftest-checks"].criteria' "$(ls -dt internal/eval/testdata/evals/results/*/ | head -1)summary.json"   # file_written max_score 2, report_marker max_score 1

# AC5: docs (manual inspection)
grep -n 'min_criterion_score\|criteria_below_floor' docs/evaluation.md

task lint
task sync:check
```

## Acceptance Criteria Mapping

| Issue AC | Where addressed | Verified by |
|----------|-----------------|-------------|
| 1. Per-criterion score, max, percent in the verdict | `agentCriterionTotals`, `AgentVerdict.Criteria`, printed lines | Task 1 unit and `selftest_criteria_test.go`; `jq` on `summary.json` |
| 2. `min_criterion_score` floor, FAIL line names the criterion | `Rubric.MinCriterionScore`, `AgentResult` stamp, `agentVerdict`, `printAgentVerdict`, `ThresholdError` | `selftest_floor_test.go`; `selftest-floor` run exits non-zero |
| 3. No floor, nothing changes | nil floor is a no-op; `caseTotals` refactor covered by existing tests; baseline compare | Control test; `task eval:selftest` vs baseline |
| 4. Unscored `checks_only` criterion adds no maximum | existing `scoreCase` behaviour, now observable | `selftest-checks` maxima test |
| 5. Docs | Task 3 | Manual inspection and `grep` |

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "criterion-report"
    agent: "builder"
    description: "TDD: capture the base-commit selftest per-case score baseline to /tmp/issue-342-selftest-baseline.json, then add CriterionTotal and AgentVerdict.Criteria, agentCriterionTotals in scoring.go (sharing one skipped-score helper with caseTotals), wire it into agentVerdict, and print one indented per-criterion line after the verdict line in printAgentVerdict. Write failing unit and end-to-end tests first (selftest_criteria_test.go covers AC1, AC3, AC4), then implement; commit tests and implementation together."
    dependencies: []
    acceptance_criteria:
      - "Baseline of selftest per-case (name, score, max_score) saved outside the repo before any code edit"
      - "AgentVerdict has criteria (always written; [] when empty) listing every non-cost criterion scored in any case, with name, score, max_score and percent, in first-seen order"
      - "Criteria are pooled over every case and every repeat in AgentResult.Cases; a Skipped criterion counts 0 with its max kept; entries with max 0 are dropped; cost criteria never appear"
      - "Sum of criteria score and max_score equals agentScoreTotals (invariant test); agentVerdict Score, Passed, CasesTotal, CasesFailed and agent_scores are unchanged"
      - "summary.json agent_verdicts.selftest.criteria lists structural_completeness and clarity with score, max_score, percent equal to the sums over selftest.json cases"
      - "In a selftest-checks run each criterion's max_score equals its number of checks across the cases (file_written 2, report_marker 1), computed by counting checks in the loaded cases"
      - "printAgentVerdict output is byte-identical to today when Criteria is empty and its first line is unchanged otherwise; existing TestPrintAgentVerdictFormat and repeat format tests pass unchanged"
      - "task eval:selftest exits 0 and per-case scores equal the saved baseline"
      - "The test seen failing first is named in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "bash -c 'd=$(ls -dt internal/eval/testdata/evals/results/*/ | head -1); jq -S \"[.cases[] | {c: .case_name, s: [.scores[] | [.name, .score, .max_score]]}]\" \"${d}selftest.json\" | diff - /tmp/issue-342-selftest-baseline.json'"

  - id: "criterion-floor"
    agent: "builder"
    description: "TDD: add the optional rubric field min_criterion_score (percent in (0,100], validated in loadRubrics like pass_threshold), stamp it on AgentResult in evaluate, evaluateProgressive and the resume branch, make agentVerdict fail the agent when any criterion's pooled percent is below it (percentAtLeast boundary), report min_criterion_score and criteria_below_floor on the verdict, name the criteria in the FAIL line and in ThresholdError via AgentFailure/failureOf, and create the selftest-floor agent, rubric and case fixtures under internal/eval/testdata/evals/. Write the failing end-to-end test (selftest_floor_test.go) and unit tests first, then implement; update wantThresholdFailure, TestWantThresholdFailure and the rubric removal list in exit_status_test.go; commit tests and implementation together."
    dependencies: ["criterion-report"]
    acceptance_criteria:
      - "Rubric.MinCriterionScore loads from YAML key min_criterion_score; unset is nil; 0, negative, above 100 and NaN fail loading with an error naming the rubric file and min_criterion_score; 100 and 0.5 are accepted"
      - "With the floor set, agentVerdict has Passed false when any criterion's pooled percent is below the floor even if score >= threshold; exactly at the floor passes; CriteriaBelowFloor lists the names in criteria order; Score is unchanged"
      - "Without the floor, verdicts are identical to before, including a 95% agent with a 0% one-check criterion (still passes)"
      - "The floor is judged on pooled percent across repeats and is re-stamped on --resume"
      - "The FAIL line names the criterion and its percent (and score/max) and keeps the [repeat ...] suffix last; ThresholdError text for a floor-only failure names the criterion and never prints a misleading score < threshold comparison; threshold-only failures print exactly as before"
      - "internal/eval/testdata/evals has agents/selftest-floor.json and prompt, rubrics/selftest-floor.yaml (checks_only, pass_threshold 60, min_criterion_score 50) and cases/selftest-floor/floor-one-criterion-zero.yaml giving file_written 2/2 and report_marker 0/1"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-floor exits non-zero, its FAIL line names report_marker, and the overall score (66.7%) is above its threshold"
      - "A control run with min_criterion_score removed from a copy of the rubric exits 0 with the same score"
      - "TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent, TestWantThresholdFailure and every other existing test still pass; task eval:selftest exits 0 with per-case scores equal to the baseline"
      - "The test seen failing first (the selftest-floor end-to-end test) is named in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"
      - "bash -c 'go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-floor; test $? -ne 0'"
      - "bash -c 'go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-floor 2>&1 | grep -E \"^FAIL selftest-floor:.*report_marker\"'"
      - "task eval:selftest"

  - id: "docs-criterion-scores"
    agent: "builder"
    description: "Document min_criterion_score and the per-criterion verdict fields in docs/evaluation.md: Rubric Format (example and field bullet), Agent verdict (floor rule, pooled percent, repeats, unscored criteria, single-case runs), Output and exit status (criterion lines, FAIL suffix, error text), the summary.json verdict field table and example (criteria[], min_criterion_score, criteria_below_floor), How Scoring Works (checks_only criterion adds nothing to the maximum), and the Self-Test fixture listing for selftest-floor. Docs only; no test first."
    dependencies: []
    acceptance_criteria:
      - "Rubric Format shows min_criterion_score in an example and in the field list: percent in (0, 100], unset by default, invalid values fail loading naming the file, exactly at the floor passes, no change when unset"
      - "Agent verdict section states the floor rule (fails when any listed criterion's pooled percent is below it even if the overall score meets the threshold), that it is judged on the pooled mean over repeats, that unscored criteria are not listed or judged, and that a single-case run applies it to that case"
      - "summary.json verdict field table documents criteria[] with name, score, max_score and percent (score/max_score are points pooled over all cases and repeats; percent is percent), plus min_criterion_score and criteria_below_floor; the example JSON includes criteria"
      - "Output section shows the per-criterion lines, the FAIL-line suffix naming the criterion, and the floor error text"
      - "How Scoring Works states that a criterion a checks_only rubric does not score in a case adds nothing to that criterion's maximum"
      - "Self-Test section lists the selftest-floor agent, rubric and case and documents it as expected to FAIL with the run command"
      - "No edits under .kairon/specs/ other than this spec, and none to CHANGELOG.md"
    validation_commands:
      - "grep -n 'min_criterion_score' docs/evaluation.md"
      - "grep -n 'criteria_below_floor' docs/evaluation.md"
      - "grep -n 'selftest-floor' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion of issue #342 against the code, fixtures, tests and docs; confirm AGENTS.md TDD and scope rules (tests committed with implementation, no changes under .kairon/evals/, other specs, CHANGELOG.md or cmd/kairon/templates/) and that task sync:check is clean."
    dependencies: ["criterion-report", "criterion-floor", "docs-criterion-scores"]
    acceptance_criteria:
      - "AC1: task eval:selftest summary.json agent_verdicts.selftest.criteria lists every non-cost criterion with score, max_score and percent pooled across cases"
      - "AC2: the selftest-floor run exits non-zero, the FAIL line names report_marker, and the overall score is above its pass threshold"
      - "AC3: with no min_criterion_score, task eval:selftest exits 0 and per-case scores match the baseline; verdict fields other than the additive criteria are unchanged"
      - "AC4: in a selftest-checks run each criterion's max_score equals its number of checks across the cases"
      - "AC5: docs/evaluation.md documents min_criterion_score and the per-criterion fields"
      - "go test ./internal/eval/... and go test ./cmd/... pass; task lint and task sync:check pass"
      - "git diff against the base shows no edits to .kairon/evals/, .kairon/specs/ (other than this spec), CHANGELOG.md or cmd/kairon/templates/"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/... -count=1"
      - "go test ./cmd/... -count=1"
      - "task eval:selftest"
      - "bash -c 'go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-floor; test $? -ne 0'"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks"
      - "task lint"
      - "task sync:check"
```
