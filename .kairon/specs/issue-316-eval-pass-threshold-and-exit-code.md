# Design Spec: Evals: enforce per-agent pass threshold; count failed criteria

Closes #316

## Summary

Today `kairon eval` can report a higher score for an agent that failed more. Skipped and errored criteria are removed from the score's denominator. Nothing gives a per-agent verdict, the 80% per-case threshold is only printed, and the process always exits 0. This change:

1. adds `pass_threshold` to rubrics (percent, default 95) and makes it the default case threshold (`min_score` still overrides it);
2. counts every errored, skipped or output-less criterion as 0 while keeping it in the denominator;
3. records a per-agent verdict (`score`, `threshold`, `passed`, `cases_total`, `cases_failed`) in `summary.json` and prints one `PASS`/`FAIL` line per agent;
4. makes `kairon eval` exit non-zero when any evaluated agent fails;
5. fixes the summary's total cost so it accumulates across agents (it is currently overwritten by the last agent saved);
6. keeps result directories written before this change loadable by `kairon eval diff`.

## Findings from the current code

All in `internal/eval`:

- `scoring.go: caseTotals` skips `Skipped` criteria in both numerator and denominator. `agentScoreTotals` sums `caseTotals`. This one function is the root cause of "a failure can raise the score".
- `runner.go: scoreCase` / `scoreDeterministic` set `Skipped = true` for: no output (`"no output to evaluate"`), an unknown deterministic criterion, an LLM criterion with empty output, and LLM judge errors/unparsable results (`scoreLLMJudge`). `executeCase` calls `scoreCase` even when the agent call failed, workspace setup failed, or the case `requires_sandbox` in a native run, so those cases carry `Skipped` zero scores today.
- `runner.go: getThreshold(tc)` returns `MinScore` or a hardcoded 80. It is used only by `printCaseResult`.
- `runner.go: updateIncrementalSummary` is the writer of `summary.json` for every path (progressive, single-case, resume). It does `summary.TotalCost = agentCost`, so the total holds only the agent saved last. It is also called after every case, with the whole agent result so far, so the verdict must be derived from the `AgentResult` alone.
- `runner.go: runProgressiveEvaluation` is the only multi-agent loop. It prints `✅ <agent>: N cases completed` and returns nil. `runSingleTestCase` writes its own result and summary. `buildSummary` is only referenced from tests (verify with grep before touching it).
- `Summary.Agents` already exists and means provenance (`map[string]AgentProvenance`), so the verdicts need a new key.
- `diff.go` only reads `agent_scores`, `total_cost` and the per-agent result files, all with tolerant `json.Unmarshal`. New optional fields therefore cannot break loading of old directories. `criterionAverages` also skips `Skipped` criteria and must follow the new rule.
- `selftest-fail` currently has one case (`stub-timeout`). An empty stub response is an agent-call error in the stub backend (`case has no stub.turns[0].response`), so an `stub: {turns: [{response: ""}]}` case produces empty `actual_output` and exercises the "no agent output" path.
- `docs/evaluation.md` does not document `min_score` at all, documents the old "skipped criteria are excluded" rule (Skipped Criteria section, `Run summary` table, the parity paragraph) and says "There is no per-agent pass threshold, PASS/FAIL verdict or non-zero exit code in `kairon eval` yet" (that E7 paragraph is now false).

## Dependency assessment: #314 (check types)

#314 does **not** block this issue. It is still OPEN with no PR, and its own body says "Dependencies: None". What it adds is a `checks` list on cases and per-criterion scoring from check counts. This design reads only `CriterionScore.Score`/`MaxScore`/`Skipped` after `scoreCase` has run, so it is independent of how a criterion got its score. #314 AC3 even assumes this work: `selftest-fail` carries its failing check cases "so `task eval:selftest` stays green once thresholds are enforced".

Assumptions, to be re-checked by whoever merges second:

- Once #314 lands, check-scored criteria still produce `CriterionScore{Score, MaxScore}`. Nothing here needs changing then.
- Textual merge conflicts are expected in `internal/eval/types.go` (`TestCase`, `Rubric` area), `scoring.go`/`runner.go` (`scoreCase`) and the Self-Test, Scoring and summary sections of `docs/evaluation.md`. They are mechanical.
- #314 will add its own `selftest-fail` cases. The new case here is named `stub-empty-response` to avoid colliding with those. Adding failing cases to `selftest-fail` only makes that agent's verdict FAIL, which is intended. `selftest` must remain all-pass.

## Design decisions

1. **Agent threshold.** `Rubric.PassThreshold *float64` (`yaml:"pass_threshold,omitempty"`, `json:"pass_threshold,omitempty"`), default `DefaultPassThreshold = 95.0`, via `func (r Rubric) Threshold() float64`. Validated in `loadRubrics`: must be within 0–100 and not NaN, otherwise the load fails naming the file. This is the agent's pass bar.
2. **Case threshold.** `getThreshold(rubric Rubric, tc TestCase)` returns `*tc.MinScore` if set, else `rubric.Threshold()`. The old hardcoded 80 is removed. `executeCase` records the effective value on the result as `CaseResult.Threshold *float64` (`json:"threshold,omitempty"`). `AgentResult.Threshold *float64` records the rubric's value, re-stamped from the current rubric on resume. Nil (legacy files) falls back to the agent threshold, then 95.
3. **Denominator rule.** `caseTotals` sums `Score` and `MaxScore` for every criterion, `Skipped` or not. `CriterionScore.Skipped` stays in the JSON as a reason flag so existing result files and tooling still parse, but it no longer changes arithmetic. A criterion whose `MaxScore` was never set stays at 0 and adds nothing (cost criteria are not scored at all, as today).
4. **Percent arithmetic.** One helper computes `float64(score) * 100 / float64(max)` (multiply first, so 19/20 is exactly 95.0 and `>=` comparisons at the boundary are exact). A case or agent with `max == 0` (nothing scorable, for example a rubric with only cost criteria) is treated as score 0 and **fails**: nothing measured cannot meet a bar. `agent_scores` (the 0–1 fraction used by `eval diff`) is still omitted in that case, as today.
5. **Verdict.** In `scoring.go`, one function `agentVerdict(ar AgentResult) AgentVerdict`:
   - `Score` = agent percent (0–100) over all cases;
   - `Threshold` = `ar.Threshold` or 95;
   - `Passed` = `Score >= Threshold`;
   - `CasesTotal` = `len(ar.Cases)`;
   - `CasesFailed` = cases whose own percent is below their own threshold (`CaseResult.Threshold`, else the agent threshold). A case with `max == 0` counts as failed;
   - `Cost` = that agent's agent+judge cost (tokens_in, tokens_out, estimated_usd only, matching the current total-cost shape).
   `Passed` depends on the aggregate score only. `cases_failed` is reported and printed but does not by itself fail the agent. This reads the issue literally ("fails its threshold" is the agent's score against the threshold) and gives a case `min_score` a single meaning: the bar for that case's own line. If a stricter "no failed case" rule is wanted later it is a one-line change in `agentVerdict`.
6. **Summary.** `Summary.AgentVerdicts map[string]AgentVerdict` with JSON key `agent_verdicts` (omitempty). `AgentVerdict` JSON: `score`, `threshold`, `passed`, `cases_total`, `cases_failed`, `cost`. `updateIncrementalSummary` sets `AgentVerdicts[agent]` from `agentVerdict(agentResult)` and recomputes `TotalCost` as the sum of all verdict costs, so it accumulates across agents and stays correct when an agent is saved repeatedly (once per case) or resumed. `TotalCost.Model`/`UsageSource` stay unset, as today. `agent_scores` is kept unchanged in meaning (fraction, now with failures counted).
7. **CLI.** After each agent, `runProgressiveEvaluation` prints one line starting with `PASS ` or `FAIL `, for example `PASS selftest: 100.0% (threshold 95.0%, 0/5 cases failed)`. `runSingleTestCase` prints the same line for its agent. After the "Evaluation complete" output, a run with any failing agent returns `*ThresholdError` which satisfies `errors.Is(err, ErrThresholdFailed)` and names the failing agents. `cmd/kairon/cmd/eval.go` sets `cmd.SilenceUsage = true` for that error (it is not a usage mistake). `main.go` already prints `Error: …` and exits 1. Agents with no cases directory are skipped with a warning as today, and so are not "evaluated". `--list`, `--perf`, `--cleanup` are unaffected.
8. **Legacy compatibility.** All new JSON fields are optional on read. `eval diff` is unchanged apart from `criterionAverages` counting skipped criteria as 0 so per-criterion lines follow the same rule. For old directories the agent line still shows their stored `agent_scores`; only the per-criterion lines are recomputed. Add a test that diffs the legacy fixtures against a new-format summary.
9. **Impact on the live evals (out of scope, must be called out in the PR).** The rubrics under `.kairon/evals/rubrics/` do not set `pass_threshold`, so they inherit 95, and their agents (architect baseline ≈ 0.81) will report FAIL and exit non-zero until improved or given an explicit `pass_threshold` in a follow-up. Those files and the template copies are deliberately not touched here, so no template sync is needed. `task sync:check` must still pass.
10. **Existing tests.** Many tests call `RunWithOptions`/`Run` on agents that now legitimately fail (mock `kiro-cli` architect cases, `selftest-fail`, the sandbox agents) and assert a nil error. Update them to tolerate `ErrThresholdFailed` (a small test helper) or, where the test is about something else, give the test rubric `pass_threshold: 0`. Update arithmetic expectations that encoded "skipped excluded" (`containment_test.go` 7/12 → 7/16, `scoring_test.go`, `diff_provenance_test.go`).

## Relevant files

Modify:
- `internal/eval/types.go`: `Rubric.PassThreshold`, `Rubric.Threshold()`, `DefaultPassThreshold`, `CaseResult.Threshold`, `AgentResult.Threshold`, `AgentVerdict`, `Summary.AgentVerdicts`, doc comment on `TestCase.MinScore`.
- `internal/eval/scoring.go`: `caseTotals` (new rule), percent helper, `agentVerdict`, `printCaseResult`, verdict line formatter.
- `internal/eval/runner.go`: `getThreshold`, `loadRubrics` validation, `executeCase` call sites that set `Threshold`, `evaluate`/`evaluateProgressive` stamping, `runProgressiveEvaluation` and `runSingleTestCase` (print line, collect failures, return `ThresholdError`), `updateIncrementalSummary` (verdict + cost sum), `buildSummary` if still used.
- `internal/eval/execute_case.go`: set `cr.Threshold`.
- `internal/eval/diff.go`: `criterionAverages`.
- `cmd/kairon/cmd/eval.go`: silence usage on `ErrThresholdFailed`.
- Tests: `threshold_test.go`, `scoring_test.go`, `containment_test.go`, `diff_provenance_test.go`, `selftest_test.go`, `pinning_test.go`, `backend_routing_test.go`, `execute_case_test.go`, daemon-gated `selftest_sandbox_test.go`, `sandbox_workspace_test.go`, `parity_sandbox_test.go` (only where they assert a nil error from a run whose agent now fails).
- `docs/evaluation.md`.

Create:
- `internal/eval/verdict.go` (or in `scoring.go`): `ErrThresholdFailed`, `ThresholdError`.
- `internal/eval/testdata/evals/cases/selftest-fail/stub-empty-response.yaml`.
- New tests for the verdict, cost accumulation, the exit error and legacy diff (names below).

Do not touch: `.kairon/evals/` (rubrics, cases, results), `.kairon/specs/` (other than this file), `CHANGELOG.md`, template-synced files.

## Team orchestration

Single PR, one builder at a time on the shared files, so the tasks are mostly sequential:

`core-scoring-threshold` → `summary-verdict-cost` → `cli-verdict-exit` → `selftest-fixtures-e2e`, with `document-thresholds` starting once `cli-verdict-exit` is done (parallel with `selftest-fixtures-e2e`, disjoint files), and `validate-all` last.

Every code task follows AGENTS.md TDD: write the failing test first, run it, confirm it fails for the expected reason (not a compile error; stub the new symbol first if needed), implement the minimum, rerun. Name the first-failing test in the commit message or PR description. Docs are exempt from test-first.

## Step-by-step task breakdown

### Task 1: core-scoring-threshold
Rubric `pass_threshold` (with validation), `getThreshold(rubric, tc)`, `CaseResult.Threshold`/`AgentResult.Threshold` stamping, `caseTotals` counting skipped criteria, exact percent helper, `agentVerdict`, `printCaseResult` using the shared helper, `criterionAverages` in `diff.go`. Tests first: rubric default 95 and override, bad values rejected, `min_score` override, a skipped criterion lowers the case and agent score, no-output case scores 0/max, 19/20 passes a 95 bar exactly, `max == 0` fails. Update `threshold_test.go`, `scoring_test.go`, `containment_test.go` (7/16), `diff_provenance_test.go` expectations.

### Task 2: summary-verdict-cost
`AgentVerdict`, `Summary.AgentVerdicts`, `updateIncrementalSummary` writes the verdict and sums per-agent costs into `TotalCost`. Tests first: two agents saved in sequence give a total equal to the sum; saving the same agent repeatedly (per-case progressive saves) does not double count; verdict fields present and correct; a summary JSON without `agent_verdicts` still loads. Fix `buildSummary` if referenced.

### Task 3: cli-verdict-exit
`ErrThresholdFailed`/`ThresholdError`, PASS/FAIL line per agent in `runProgressiveEvaluation` and `runSingleTestCase`, error returned after all output and results are written, `SilenceUsage` in `cmd/eval.go`. Tests first: failing agent run returns an error satisfying `errors.Is`, passing run returns nil, one line per agent starts with `PASS `/`FAIL `, summary still written when failing. Update the existing tests that assume a nil error from a failing run (decision 10).

### Task 4: selftest-fixtures-e2e
Add `selftest-fail/stub-empty-response.yaml` (stub turn with an empty response). End-to-end tests with the stub backend: `selftest` exits clean with `passed: true`, 100 and the five fields in its summary; `selftest-fail` returns `ErrThresholdFailed`, every non-cost criterion is `score 0` with `max_score > 0`, `score` 0 and `passed: false`; running both together records `total_cost` equal to the sum of both agents' costs (AC5); `eval diff` loads the legacy result directories (a fixture-based test, plus the real directories in the validator step, AC6). Update `selftest_test.go` helpers and counts.

### Task 5: document-thresholds
Update `docs/evaluation.md`: `pass_threshold` in Rubric Format and its field list; `min_score` in Test Case Format; rewrite "Skipped Criteria" (counted as 0, still in the denominator); add the exit-code and PASS/FAIL contract; add `agent_verdicts` and the cumulative `total_cost` to the Run summary example and table; replace the "E7 is separate work" paragraph and the "skipped … excluded" parity wording; update the Self-Test section (`selftest-fail` now has two cases; the sample command exits non-zero; `task eval:selftest` exits 0); note that rubrics without `pass_threshold` inherit 95; and that `cases_failed` is informational. Check `README.md` and other `docs/` pages with `grep -rn "agent_scores\|exits 0\|min_score" docs README.md`.

### Task 6: validate-all
Read-only verification of every acceptance criterion with the commands below.

## Validation commands

```bash
go build ./...
go test ./internal/eval/... ./cmd/...
task test
task lint
task sync:check
task eval:selftest                                   # must exit 0 and print a PASS line
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail   # must exit non-zero and print a FAIL line
go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2   # must succeed (AC6)
```

## Acceptance criteria mapping

| # | Criterion | Where satisfied | Verified by |
|---|-----------|-----------------|-------------|
| 1 | `pass_threshold`, default 95, case default, `min_score` override | Task 1, docs in Task 5 | threshold tests; `grep -n "pass_threshold\|min_score" docs/evaluation.md` |
| 2 | Failed/skipped/no-output criteria are 0 and stay in the denominator | Task 1, fixture in Task 4 | scoring tests; `selftest-fail` `stub-empty-response` result |
| 3 | `score`, `threshold`, `passed`, `cases_total`, `cases_failed` in the summary; PASS/FAIL line | Tasks 2, 3 | summary JSON test, stdout test |
| 4 | Non-zero exit on failure, 0 when all pass | Task 3 | the two `go run` commands above |
| 5 | Total cost accumulates across agents | Task 2, test in Task 4 | two-agent run test |
| 6 | Pre-change results still load in `eval diff` | Task 1 (diff), test in Task 4 | the real `eval diff` command above |

## Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "core-scoring-threshold"
    agent: "builder"
    description: "Add rubric pass_threshold (default 95, validated), make case thresholds default to it with min_score overriding, record thresholds on CaseResult/AgentResult, change caseTotals so skipped/errored/no-output criteria count as 0 in the denominator, add an exact percent helper and agentVerdict, update printCaseResult and diff criterionAverages. Test first (TDD)."
    dependencies: []
    acceptance_criteria:
      - "Rubric accepts pass_threshold (percent); Rubric.Threshold() defaults to 95; values outside 0-100 or NaN are rejected by loadRubrics with an error naming the file"
      - "getThreshold(rubric, tc) returns tc.MinScore when set, otherwise the rubric threshold; the hardcoded 80 is gone"
      - "caseTotals counts Skipped criteria with their MaxScore in the denominator and their Score (0) in the numerator; a case with no output scores 0 over a non-zero maximum"
      - "Percent is computed as score*100/max so 19 of 20 equals exactly 95 and passes a 95 threshold; a case or agent with max 0 is a failure"
      - "agentVerdict returns score (0-100), threshold, passed (score >= threshold), cases_total, cases_failed (cases below their own threshold) and the agent's cost"
      - "A failing test was observed first for the new behaviour and its name is recorded in the commit message"
      - "Existing tests that encoded the skipped-excluded rule (containment_test 7/12, scoring_test, diff_provenance_test) are updated to the new rule"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Threshold|Totals|Verdict|Scor|Rubric|Criterion|Diff' -count=1"
      - "go test ./internal/eval/ -count=1"

  - id: "summary-verdict-cost"
    agent: "builder"
    description: "Record the per-agent verdict in summary.json under agent_verdicts and make total_cost the sum of all agents' costs, derived from the AgentResult in updateIncrementalSummary (and buildSummary if still referenced). Test first (TDD)."
    dependencies: ["core-scoring-threshold"]
    acceptance_criteria:
      - "Summary has AgentVerdicts (json agent_verdicts, omitempty) with score, threshold, passed, cases_total, cases_failed and cost per agent"
      - "updateIncrementalSummary sets the verdict from agentVerdict and recomputes TotalCost as the sum of verdict costs; saving agent A then agent B gives A+B, and repeatedly saving A (per-case progressive saves) does not double count"
      - "total_cost keeps only tokens_in, tokens_out and estimated_usd (no model or usage_source), as before"
      - "agent_scores keeps its meaning (fraction) and is still omitted for an agent whose denominator is 0"
      - "A summary.json without agent_verdicts (written before this change) still unmarshals and updates cleanly"
      - "Parity tests that compare whole Summary values across native and container runs still pass"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Summary|Cost|Verdict|Containment|Parity' -count=1"
      - "go test ./internal/eval/ -count=1"

  - id: "cli-verdict-exit"
    agent: "builder"
    description: "Print one PASS/FAIL line per agent, return a ThresholdError (errors.Is ErrThresholdFailed) after all results are written when any evaluated agent fails, and keep cobra from printing usage for it. Update existing tests that assumed a nil error from a run whose agent now fails. Test first (TDD)."
    dependencies: ["summary-verdict-cost"]
    acceptance_criteria:
      - "runProgressiveEvaluation and runSingleTestCase print exactly one line per evaluated agent beginning with 'PASS ' or 'FAIL ' showing score, threshold and failed/total cases"
      - "RunWithOptions/Run return an error satisfying errors.Is(err, ErrThresholdFailed) naming the failing agents when any agent fails, and nil when all pass; the error is returned only after results, summary and the progress-file cleanup are done"
      - "cmd/kairon/cmd/eval.go sets SilenceUsage for ErrThresholdFailed so the process prints a one-line error and exits 1 (main.go already exits 1 on any error)"
      - "Agents without a cases directory are still skipped with a warning and do not affect the verdict; --list, --perf and --cleanup are unaffected"
      - "Existing tests in pinning_test, backend_routing_test, execute_case_test and the daemon-gated sandbox tests pass, tolerating ErrThresholdFailed (or using pass_threshold 0 in their test rubric) only where the failure is not what they test"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ ./cmd/... -count=1"
      - "go vet ./internal/eval/... ./cmd/..."

  - id: "selftest-fixtures-e2e"
    agent: "builder"
    description: "Add the selftest-fail stub-empty-response case and end-to-end stub-backend tests for the verdict fields, exit behaviour, cumulative cost across selftest and selftest-fail, and legacy result loading in eval diff. Test first (TDD)."
    dependencies: ["cli-verdict-exit"]
    acceptance_criteria:
      - "internal/eval/testdata/evals/cases/selftest-fail/stub-empty-response.yaml exists with an empty stub response; its result has actual_output empty and every non-cost criterion with score 0 and max_score greater than 0"
      - "A selftest run returns nil and its summary has agent_verdicts.selftest with passed true, score 100, threshold 95, cases_total 5, cases_failed 0; stdout has a PASS line"
      - "A selftest-fail run returns ErrThresholdFailed, its verdict has passed false, a score below the threshold and cases_failed equal to cases_total; stdout has a FAIL line"
      - "A run of selftest and selftest-fail together records summary total_cost equal to the sum of the two agents' costs (tokens and estimated_usd)"
      - "A test loads the legacy run fixtures (fixtures/runs/legacy-native, native-pinned) and a new-format summary through diffTo without error"
      - "selftest_test.go assertions and case counts still hold and selftest stays all-pass"
    validation_commands:
      - "go test ./internal/eval/ -run 'SelfTest|Selftest|Verdict|Cost|Diff' -count=1"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail; test $? -ne 0"
      - "go test ./internal/eval/... -count=1"

  - id: "document-thresholds"
    agent: "documenter"
    description: "Update docs/evaluation.md for pass_threshold, min_score, the new scoring rule, the PASS/FAIL line and exit code, agent_verdicts and cumulative total_cost, the self-test changes, and remove the stale 'E7 is separate work' and 'skipped excluded' wording. Also check README.md and other docs pages for stale statements."
    dependencies: ["cli-verdict-exit"]
    acceptance_criteria:
      - "Rubric Format documents pass_threshold (percent, default 95) and Test Case Format documents min_score and that it overrides the rubric's pass_threshold"
      - "Skipped Criteria section states errored, skipped and no-output criteria count as 0 and stay in the denominator"
      - "The exit-code contract (non-zero when any evaluated agent fails, 0 when all pass) and the PASS/FAIL line format are documented, including that cases_failed is informational and passed depends on the aggregate score"
      - "The Run summary example and field table include agent_verdicts (score, threshold, passed, cases_total, cases_failed, cost) and describe total_cost as the sum across agents"
      - "The Self-Test section describes the second selftest-fail case, says the selftest-fail command now exits non-zero, and says task eval:selftest exits 0"
      - "The paragraph saying there is no per-agent threshold, verdict or exit code yet is removed or rewritten; the parity text no longer says skipped criteria are excluded"
      - "A note states rubrics without pass_threshold inherit 95"
    validation_commands:
      - "grep -n 'pass_threshold' docs/evaluation.md"
      - "grep -n 'min_score' docs/evaluation.md"
      - "grep -n 'agent_verdicts' docs/evaluation.md"
      - "! grep -n 'E7) is separate work\\|excluded from numerator and denominator' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion in issue #316, TDD evidence, protected-path integrity and the full test, lint and sync checks."
    dependencies: ["selftest-fixtures-e2e", "document-thresholds"]
    acceptance_criteria:
      - "Criterion 1: pass_threshold and min_score behaviour is covered by passing tests and documented in docs/evaluation.md"
      - "Criterion 2: the selftest-fail stub-empty-response result shows score 0 on every non-cost criterion with a non-zero max_score, and the agent's score is lower than it would be with those criteria excluded"
      - "Criterion 3: the selftest summary.json contains score, threshold, passed, cases_total and cases_failed for the agent and stdout contains a PASS line"
      - "Criterion 4: task eval:selftest exits 0 and the selftest-fail command exits non-zero with a FAIL line"
      - "Criterion 5: a selftest plus selftest-fail run records total_cost equal to the sum of both agents' costs"
      - "Criterion 6: kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2 succeeds"
      - "git diff shows no changes to .kairon/evals/, CHANGELOG.md or .kairon/specs/ other than this spec, and task sync:check, task lint and task test pass"
    validation_commands:
      - "task test"
      - "task lint"
      - "task sync:check"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail; test $? -ne 0"
      - "go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2"
      - "git diff --stat -- .kairon/evals CHANGELOG.md"
```
