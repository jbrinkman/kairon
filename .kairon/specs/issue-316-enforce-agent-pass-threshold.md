# Design Spec: Evals — enforce a per-agent pass threshold; count failed criteria

Closes #316

Dependencies: #314

## 1. Solution Approach

Today the eval harness can *report* scores but cannot *judge* them:

- `caseTotals` (`internal/eval/scoring.go`) drops `Skipped` criteria from both numerator and denominator. A criterion that errors, is skipped, or has no agent output therefore **removes** itself from the score, so a failure can raise an agent's score.
- The only threshold is the per-case `getThreshold(tc)` in `runner.go` (`min_score`, default 80), and it is only printed by `printCaseResult`.
- `buildSummary` / `updateIncrementalSummary` write `agent_scores` (a 0–1 fraction) and nothing else about pass/fail.
- `updateIncrementalSummary` sets `summary.TotalCost = agentCost`, i.e. it **overwrites** the total with the cost of whichever agent was saved last instead of accumulating across agents.
- `RunWithOptions`/`Run`/`runSingleTestCase`/`runWithResume` return `nil` after a completed run, so `kairon eval` always exits 0.

Design (all inside `internal/eval`, plus one line in `cmd/kairon/cmd/eval.go` and `docs/evaluation.md`):

1. **Thresholds live on the rubric.** `Rubric` gets an optional `pass_threshold` (percent, `(0,100]`, default `95`). A case's threshold is `min_score` when set, else its rubric's threshold. The agent's threshold is its rubric's threshold.
2. **Every non-cost criterion counts.** `caseTotals` no longer skips `Skipped` criteria: a skipped/errored/no-output criterion contributes `0` to the numerator and its `MaxScore` to the denominator. The `skipped` flag stays in the JSON as *information* ("why it scored 0"), but it no longer changes arithmetic. This is the single place the arithmetic lives, so `agent_scores`, the per-case line and the new verdict all inherit the fix (and native/container parity is preserved).
3. **A per-agent verdict is computed in one function** (`agentVerdict`) from an `AgentResult` alone, recorded in `summary.json` under a new `agent_verdicts` map, printed as one `PASS`/`FAIL` line per agent, and turned into a non-zero exit via a sentinel error.
4. **Total cost accumulates** by storing each agent's cost in its verdict entry and summing the entries (in sorted agent order, so floats are deterministic) — idempotent under the per-case progressive saves and under `--resume`.
5. **Back-compat:** all new JSON fields are additive; `agent_scores` keeps its meaning (0–1 fraction) so `eval diff` keeps working on old and new result directories.

### Decisions (and why)

| Decision | Rationale |
|---|---|
| Agent verdict = **aggregate** `score% >= threshold` where `score% = Σscore / Σmax × 100` over all non-cost criteria of all cases. `cases_failed` is informational and does **not** by itself fail the agent. | Matches the issue wording ("fails its threshold", "score falls accordingly") and reuses `agentScoreTotals`. Per-case failure is still visible on each case line and via `cases_failed`. |
| Compute percent as `float64(score)*100/float64(max)` (multiply first) and compare `>= threshold`. | Exactly-at-threshold must pass (e.g. 19/20 = 95%). Multiplying integers first avoids `0.95*100`-style float drift. Add a boundary test. |
| An agent whose denominator is 0 (rubric with only cost criteria, or no scored criteria) gets `score 0`, `passed false`, and **no** `agent_scores` entry. | Fails closed: nothing was measured, so nothing can clear a 95% bar. `agent_scores` keeps the existing "zero denominator ⇒ no entry" behaviour (pinned by `TestBuildSummaryZeroDenominatorOmitsAgentScore`). |
| New map `agent_verdicts` (not extra fields on `agents`). | `agents` is provenance, populated only for pinned runs, and its length drives the single-agent top-level fields. A separate map exists for every evaluated agent (pinned or not) and cannot disturb provenance. |
| `score`/`threshold` are **percent** (0–100). `agent_scores` stays a **0–1 fraction**. | `agent_scores` is read by `eval diff` on historical runs; changing units would break AC6. Docs must call out the two units. |
| Single-case (`--case`) runs and `--resume` runs enforce the verdict too. | "Any evaluated agent" — one code path (`printAgentVerdict` + `ThresholdError`) for all entry points. |
| `eval diff`'s per-criterion averages (`criterionAverages`) are left unchanged. | Historical runs must diff exactly as before (AC6). Only the agent-level number (from `summary.agent_scores`) changes meaning for new runs. Note it in docs; not in scope to change. |
| `total_cost` = agent cost **+ judge cost** per case (what `updateIncrementalSummary` already writes). `buildSummary` (currently unused outside tests, agent-cost only) delegates to the same helper. | One definition of "cost" for the summary; no behaviour change for the real run path except accumulation. |
| Not changed: repeat runs, check types, the judge prompt, anything under `.kairon/evals/`, `cmd/kairon/templates/`, `.kairon/specs/` (other than this file). | Out of scope / frozen per `AGENTS.md`. Shipped rubrics simply inherit the 95 default. |

### Data contract (relied on by iteration logs and promotion issues — keep stable)

`summary.json` (new, per evaluated agent):

```json
"agent_verdicts": {
  "selftest": {
    "score": 100,
    "threshold": 95,
    "passed": true,
    "cases_total": 16,
    "cases_failed": 0,
    "cost": { "tokens_in": 0, "tokens_out": 0, "estimated_usd": 0 }
  }
}
```

- `score`, `threshold`: percent (0–100), float. `passed`: bool. `cases_total`, `cases_failed`: ints. `cost`: `CostInfo` of that agent (agent + judge calls).
- `total_cost` = Σ over `agent_verdicts[*].cost` (tokens in/out and `estimated_usd`; `model`/`usage_source` are not aggregated, as today).
- `<agent>.json` additions: `threshold` (agent's pass threshold, percent) on `AgentResult`; `threshold` and `passed` on each `CaseResult` (`Passed` is `*bool` + `omitempty` so a legacy case without it is distinguishable from `false`; `Threshold` is `float64` + `omitempty`).

CLI (stdout, one line per evaluated agent, line starts with the verdict word):

```
PASS selftest: 100.0% (threshold 95.0%), 0/16 cases failed
FAIL selftest-fail: 4.0% (threshold 95.0%), 13/13 cases failed
```

Process exit: `0` when every evaluated agent passed; otherwise `kairon eval` prints the lines above, then `Error: eval failed: 1 of 2 agents below their pass threshold: selftest-fail (4.0% < 95.0%)` on stderr (via `main.go`) and exits `1`. Usage text must **not** be printed for this error.

## 2. Relevant Files

Modify:

- `internal/eval/types.go` — `Rubric.PassThreshold`, `DefaultPassThreshold`, `Rubric.Threshold()`; `CaseResult.Threshold/Passed`; `AgentResult.Threshold`; `AgentVerdict` type; `Summary.AgentVerdicts`.
- `internal/eval/runner.go` — rubric validation in `loadRubrics`; replace `getThreshold(tc)` with `caseThreshold(tc, rubric)`; stamp thresholds in `evaluate`/`evaluateProgressive`; `buildSummary` and `updateIncrementalSummary` (verdicts + accumulated cost); `runProgressiveEvaluation`, `runSingleTestCase`, `Run` return the threshold error and print verdict lines.
- `internal/eval/scoring.go` — `caseTotals` counts skipped criteria; `printCaseResult` takes the rubric/threshold and lists skipped criteria in the breakdown; new `agentVerdict`, `printAgentVerdict`, `ThresholdError`/`ErrThresholdFailed` (a new small file `internal/eval/verdict.go` is acceptable for the new symbols).
- `cmd/kairon/cmd/eval.go` — `cmd.SilenceUsage = true` at the start of `RunE` (so a verdict failure is not followed by a usage dump).
- `internal/eval/testdata/evals/cases/selftest-fail/stub-empty-response.yaml` — **new** case (empty stub response).
- Existing tests that pin old semantics (update, do not delete coverage): `threshold_test.go`, `scoring_test.go` (`caseTotals`/`agentScoreTotals`/`TestBuildSummaryZeroDenominatorOmitsAgentScore`), `containment_test.go` (7/12 agent score), `diff_provenance_test.go` (`agentScoreTotals` on loaded legacy data), `parity_sandbox_test.go` (`getThreshold`, `selftest-fail` run now returns the threshold error), `selftest_checks_test.go` (case counts, `TestSelfTestChecksRunExitsCleanly`), `sandbox_workspace_test.go` and any test that calls `RunWithOptions`/`Run`/`runWithResume` on a low-scoring agent (`selftest-fail`, `architect` fixtures in `pinning_test.go`, `config_test.go`, `checks_scoring_test.go`) — they must tolerate `errors.Is(err, ErrThresholdFailed)` where a failing verdict is expected.
- `docs/evaluation.md` — see task `update-docs`.

Read-only / do **not** edit: `.kairon/evals/**` (rubrics, cases, `results/`), `cmd/kairon/templates/**`, `.kairon/specs/**` (except this spec), `CHANGELOG.md`.

Template sync: none of the touched files are template-synchronised; `task sync:check` must still pass unchanged.

## 3. Team Orchestration

Strict TDD per `AGENTS.md`: in every builder task, write the failing test(s) first, run them and confirm they fail for the *expected* reason (assertion failure, not a compile error/typo — add the minimal type/function stub first if a test needs a new symbol), then implement, re-run, and keep test + implementation in the same commit. Name the first-failing test in the commit message.

Dependency graph:

```
rubric-threshold ──► count-failed-criteria ──► agent-verdict-summary ──┬─► eval-exit-status ──┐
                                                                       └─► update-docs ───────┴─► validate-complete
```

The three code tasks touch overlapping files (`scoring.go`, `runner.go`, `types.go`) and are therefore sequential. `update-docs` only needs the field names fixed in this spec's data contract, so it runs in parallel with `eval-exit-status` (disjoint files). `validate-complete` (read-only) runs last.

## 4. Step-by-Step Task Breakdown

### Task 1 — `rubric-threshold`: rubric `pass_threshold` and case-threshold resolution (AC1)

Tests first (`threshold_test.go`, plus a rubric-loading test in `runner_config_test.go` or a new `rubric_threshold_test.go`):
- `Rubric{}.Threshold()` is `95`; `pass_threshold: 90` loads and yields `90`.
- `caseThreshold(tc, rubric)`: no `min_score` ⇒ rubric threshold (95 default, or the rubric's value); `min_score` set ⇒ overrides (including a value *above* and *below* the rubric's).
- `loadRubrics` rejects `pass_threshold` ≤ 0, > 100 or NaN with an error that names the rubric file; absent key is fine.

Implementation:
- `const DefaultPassThreshold = 95.0`; `Rubric.PassThreshold *float64` (`yaml:"pass_threshold,omitempty" json:"pass_threshold,omitempty"`); `func (r Rubric) Threshold() float64`.
- `caseThreshold(tc TestCase, r Rubric) float64` replaces `getThreshold`; update `printCaseResult(out, tc, cr)` to take the resolved threshold (or the rubric) and its callers (`evaluate`, `evaluateProgressive`), and update `parity_sandbox_test.go`.
- Keep the printed `(threshold: N%)` format.

### Task 2 — `count-failed-criteria`: skipped / errored / no-output criteria score 0 and stay in the denominator (AC2)

Tests first:
- `caseTotals`: a case with `{Score:0, MaxScore:5, Skipped:true}` plus `{Score:5, MaxScore:5}` ⇒ `(5, 10)`; `agentScoreTotals` likewise; an LLM-judged criterion with no output / no judge / judge error ⇒ `Score 0`, `MaxScore` from the rubric, `Skipped true`, and counted.
- New fixture `internal/eval/testdata/evals/cases/selftest-fail/stub-empty-response.yaml` (agent `selftest-fail`, `stub.turns[0].response: ""`). A test that runs `selftest-fail` through the stub backend asserts this case's `structural_completeness` is `0/5` (`MaxScore` 5, counted), `ActualOutput == ""`, and that the agent's total score is lower than it would be without the case (compute both from the result).
- `printCaseResult`: a case with a skipped criterion lists it (as `0/N`) in the below-threshold breakdown.

Implementation: remove the `if s.Skipped { continue }` in `caseTotals`; update its doc comment; drop the `!s.Skipped` filter in `printCaseResult`'s breakdown; keep `Skipped` in the JSON and in `criterionAverages` (diff) untouched. Fix existing tests whose expectations encoded the old exclusion (list in §2). Confirm that the fixture's empty response really produces "no output" (the stub rejects an empty response at call time, not at load time); if the case loader rejects it, adjust the loader/test rather than the fixture's intent.

### Task 3 — `agent-verdict-summary`: per-agent verdict, summary fields, cost accumulation (AC3 JSON part, AC5, AC6)

Tests first:
- `agentVerdict(AgentResult)`: all passing ⇒ `passed true`; below threshold ⇒ `false`; **exactly at threshold** (19/20) ⇒ `true`; zero denominator ⇒ `score 0`, `passed false`; `cases_total` = `len(Cases)`; `cases_failed` counts cases with `Passed == &false` and, for legacy cases with `Passed == nil`, recomputes `pct >= AgentResult.Threshold` (fallback 95 when 0).
- `executeCase`/`evaluate*` stamp `CaseResult.Threshold`/`Passed` (using `caseThreshold`, `Passed = max>0 && pct >= threshold`; a case with no output is `Passed=false`) and `AgentResult.Threshold = rubric.Threshold()`, including on the `--resume` path where `result = existing` is replaced.
- `updateIncrementalSummary` / `buildSummary`: summary has `agent_verdicts.<agent>` with `score`, `threshold`, `passed`, `cases_total`, `cases_failed`; **saving the same agent repeatedly (progressive saves) does not change the total**; saving `a` then `b` gives `total_cost == cost(a)+cost(b)` (tokens exact, USD within 1e-9), regardless of save order; previously-saved agents' verdicts survive a later agent's save; `buildSummary` uses the same helper (agent + judge cost).
- Back-compat: a `summary.json` written before this change (no `agent_verdicts`) still unmarshals; `loadSummary`/`diffTo` work against the two checked-in legacy directories `260620-200919-e369501` and `260621-160207-8a19eb2` under `.kairon/evals/results` (read-only use) — add a test that runs `diffTo` on them (skip with a clear message if the directories are absent) in addition to the CLI check.
- Whole-`Summary` parity (`assertSummaryParity`) still holds: verdict maps and sorted-order cost sums are deterministic.

Implementation:
- Types per §1 data contract (`AgentVerdict`, `Summary.AgentVerdicts map[string]AgentVerdict` with `json:"agent_verdicts,omitempty"`).
- `agentVerdict(ar AgentResult) AgentVerdict` and `agentCostTotals(ar AgentResult) CostInfo` (agent + judge, tokens and USD).
- `updateIncrementalSummary`: replace `summary.TotalCost = agentCost` with: store `verdict.Cost`, then `TotalCost = Σ` over `AgentVerdicts` in **sorted agent-name order**. Keep every other behaviour (mode-mixing refusal, containment, provenance, `agent_scores` omission on zero denominator).
- `AgentScores[agent]` is still `score/max` (fraction) — unchanged meaning.

### Task 4 — `eval-exit-status`: PASS/FAIL lines and non-zero exit (AC3 stdout part, AC4)

Tests first:
- `printAgentVerdict` writes exactly `PASS <agent>: <score>% (threshold <t>%), <failed>/<total> cases failed` / `FAIL …` (one line, starts with the verdict word; one decimal).
- `ThresholdError` wraps `ErrThresholdFailed` (`errors.Is`), lists every failing agent with score and threshold, and its text contains the agent names.
- End to end via the stub backend and a copied evals dir: `RunWithOptions("selftest", …)` returns `nil` and the output has `PASS selftest`; `RunWithOptions("selftest-fail", …)` returns an error satisfying `errors.Is(err, ErrThresholdFailed)`, its `summary.json` has `passed:false`, and the output has `FAIL selftest-fail`; a run covering both (`RunWithOptions("", …)` against a fixtures copy restricted to those two agents, or `Run` with both rubrics) yields `total_cost` equal to the sum of the two `<agent>.json` costs (AC5), one verdict line per agent and an error naming only the failing agent.
- `--case` (`runSingleTestCase`) and `--resume` (`runWithResume`) paths return the same error / print the same line.
- The completed run still writes results, removes `.progress` and prints the performance report before returning the error (the error must not abort the run early).
- `cmd/kairon/cmd/eval_test.go`: the eval command's `RunE` sets `SilenceUsage` (assert via the cobra command that a returned verdict error does not trigger usage output).

Implementation: `var ErrThresholdFailed = errors.New(...)`; `type ThresholdError struct{ Failed []AgentFailure }` with `Error()` and `Unwrap()`; in `runProgressiveEvaluation` print the verdict line right after each agent's final save, collect failures, and after the "Evaluation complete" block return `&ThresholdError{…}` (`Run` already returns this error after printing the performance report; `runSingleTestCase` does the same after its summary write). Update every existing test that runs a failing agent through `RunWithOptions`/`Run` to accept `ErrThresholdFailed` (list in §2) — assert it where the failure is expected rather than ignoring all errors. Update `TestSelfTestChecksRunExitsCleanly` so `selftest` returns `nil` and `selftest-fail` returns `ErrThresholdFailed`.

### Task 5 — `update-docs`: `docs/evaluation.md` (AC1 documentation, contract for the constraint)

Non-code change; no test-first requirement. Update, keeping the existing structure:
- **Rubric Format**: document `pass_threshold` (percent, `(0,100]`, default 95) and its validation error.
- **Test Case Format**: document `min_score` (percent; overrides the rubric's `pass_threshold` for that case) and the resolution order `min_score` → rubric `pass_threshold` → 95.
- **How Scoring Works / Skipped Criteria**: rewrite — a skipped, errored or no-output criterion scores 0 and stays in the denominator; `skipped: true` is retained as the *reason*; remove "excluded from aggregate score calculations" and the "aggregate scores reflect only deterministic criteria" sentence; fix the "When the agent failed" paragraph and the "Skipped criteria stay excluded" sentence under *Scoring and reasoning*.
- New section **Pass/Fail Verdict and Exit Status**: formulas, per-case vs per-agent threshold, `cases_failed`, the `PASS`/`FAIL` line format, exit codes (`0` all pass, `1` any agent below threshold or any other error), single-case/resume behaviour, zero-denominator fail-closed rule, and the two units (`agent_scores` fraction vs `score` percent).
- **Run summary (`summary.json`)**: add `agent_verdicts` to the example and the field table; document `total_cost` as the sum across agents; add `threshold` to the `<agent>.json` and case JSON descriptions.
- **Parity section**: replace the paragraph "The per-agent threshold verdict (E7) is separate work…" with the now-implemented behaviour (the verdict is part of the whole-`Summary` parity comparison).
- **Self-Test section**: `selftest-fail` now has 13 cases (add `stub-empty-response`: empty stub response ⇒ every criterion 0 and counted); `task eval:selftest` exits 0; the `selftest-fail` command now exits **non-zero** (replace "The run itself exits 0…"); update the `Task`-command comments and the case listing.
- **Comparing Runs**: one sentence that result directories written before this change still load (old `agent_scores`, no `agent_verdicts`) and that per-criterion averages in `eval diff` still exclude skipped criteria.
Do not edit `.kairon/evals/**` or any frozen path.

### Task 6 — `validate-complete`: independent verification (all ACs)

Read-only. Re-run every validation command below, confirm each acceptance criterion from the issue against the checked-in code and docs, confirm no frozen path changed (`git diff --stat` shows nothing under `.kairon/evals/`, `.kairon/specs/` except this spec, `cmd/kairon/templates/`, `CHANGELOG.md`), and that each builder commit contains both tests and implementation.

## 5. Validation Commands

```bash
go build ./...
go vet ./...
task fmt:check
go test ./internal/eval/... ./cmd/... -count=1
task test
task sync:check

# AC1 — docs
grep -n "pass_threshold" docs/evaluation.md && grep -n "min_score" docs/evaluation.md

# AC2/AC3/AC4/AC5 — build once, then exercise the real CLI
go build -o /tmp/kairon-316 ./cmd/kairon
task eval:selftest                                    # exits 0
/tmp/kairon-316 eval --backend stub --evals-dir internal/eval/testdata/evals selftest | grep -E '^PASS selftest: '
! /tmp/kairon-316 eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail >/dev/null 2>&1   # non-zero
/tmp/kairon-316 eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail 2>&1 | grep -E '^FAIL selftest-fail: '

# AC6 — legacy result directories still diff
go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2
```

Expected: the self-test `summary.json` in the newest `internal/eval/testdata/evals/results/*/` contains `agent_verdicts.<agent>.{score,threshold,passed,cases_total,cases_failed}` and `total_cost` equal to the sum of the per-agent costs.

## 6. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "rubric-threshold"
    agent: "builder"
    description: "TDD: add rubric pass_threshold (percent, default 95) with validation, Rubric.Threshold(), and caseThreshold(tc, rubric) replacing getThreshold so a case defaults to its rubric's threshold and min_score still overrides it. Update printCaseResult callers and existing threshold tests. Commit tests and implementation together."
    dependencies: []
    acceptance_criteria:
      - "Rubric YAML accepts pass_threshold; absent means 95; values <= 0, > 100 or NaN fail loadRubrics with an error naming the rubric file"
      - "caseThreshold returns min_score when set, otherwise the rubric's pass_threshold (default 95), covered by unit tests for both override directions"
      - "getThreshold is replaced and all callers (printCaseResult, evaluate, evaluateProgressive, parity_sandbox_test) compile and pass"
      - "The first failing test was observed to fail for the expected reason (assertion, not compile error) and is named in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'Threshold|Rubric' -count=1"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "count-failed-criteria"
    agent: "builder"
    description: "TDD: make skipped, errored and no-output criteria score 0 and stay in the denominator by removing the Skipped exclusion in caseTotals (and the breakdown filter in printCaseResult). Add the selftest-fail case stub-empty-response (empty stub response) and tests proving every criterion scores 0/MaxScore and the agent score falls. Update existing tests that encoded the old exclusion. Commit tests and implementation together."
    dependencies: ["rubric-threshold"]
    acceptance_criteria:
      - "caseTotals and agentScoreTotals include Skipped criteria as 0 with their MaxScore in the denominator; unit tests cover deterministic no-output, LLM-judge no-output/no-judge/judge-error and checked criteria with no output"
      - "internal/eval/testdata/evals/cases/selftest-fail/stub-empty-response.yaml exists and a test shows it scores 0 on every criterion (MaxScore > 0, counted) with empty ActualOutput, lowering the selftest-fail agent score"
      - "The skipped flag is still written to result JSON; criterionAverages in diff.go is unchanged"
      - "printCaseResult lists skipped criteria in the below-threshold breakdown"
      - "Existing tests pinning the old behaviour (scoring_test, containment_test, diff_provenance_test, selftest_checks_test case counts) are updated, not deleted"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'CaseTotals|AgentScoreTotals|BuildSummary|SelfTest|Scoring|Skipped|EmptyResponse' -count=1"
      - "test -f internal/eval/testdata/evals/cases/selftest-fail/stub-empty-response.yaml"
      - "git diff --quiet HEAD -- .kairon/evals cmd/kairon/templates"

  - id: "agent-verdict-summary"
    agent: "builder"
    description: "TDD: add AgentVerdict (score percent, threshold, passed, cases_total, cases_failed, cost), agentVerdict() and agentCostTotals(); stamp threshold/passed on CaseResult and threshold on AgentResult in evaluate and evaluateProgressive (including the resume path); write summary.agent_verdicts in updateIncrementalSummary and buildSummary; make total_cost accumulate across agents (sum of verdict costs in sorted agent order, idempotent under progressive saves). Keep agent_scores as a 0-1 fraction and keep legacy result directories loadable. Commit tests and implementation together."
    dependencies: ["count-failed-criteria"]
    acceptance_criteria:
      - "agentVerdict passes at exactly the threshold (19/20 at 95), fails below it, and fails closed (score 0, passed false) on a zero denominator; cases_total and cases_failed are correct including legacy cases with Passed == nil"
      - "summary.json contains agent_verdicts.<agent> with score, threshold, passed, cases_total and cases_failed for every evaluated agent; score and threshold are percent"
      - "total_cost equals the sum of all agents' costs (agent + judge), is unchanged by repeated progressive saves of the same agent, and is independent of save order (tokens exact, USD within 1e-9)"
      - "agent_scores semantics and zero-denominator omission are unchanged; whole-Summary native/container parity test code still compiles and holds"
      - "A summary.json without agent_verdicts and the checked-in legacy result directories 260620-200919-e369501 and 260621-160207-8a19eb2 still load and diff (tested)"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Verdict|Summary|Cost|Diff|Parity' -count=1"
      - "go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "eval-exit-status"
    agent: "builder"
    description: "TDD: print one PASS/FAIL line per evaluated agent and make kairon eval exit non-zero when any evaluated agent fails its threshold. Add ErrThresholdFailed and ThresholdError, return it from runProgressiveEvaluation/Run/runSingleTestCase/runWithResume after results, summary and performance report are written, set cmd.SilenceUsage in cmd/kairon/cmd/eval.go, and update every existing test that runs a failing agent so it asserts errors.Is(err, ErrThresholdFailed) where failure is expected. Commit tests and implementation together."
    dependencies: ["agent-verdict-summary"]
    acceptance_criteria:
      - "stdout has exactly one line per evaluated agent in the form 'PASS <agent>: <score>% (threshold <t>%), <failed>/<total> cases failed' or the FAIL equivalent, starting with the verdict word"
      - "RunWithOptions('selftest') on the stub backend returns nil; RunWithOptions('selftest-fail') returns an error for which errors.Is(err, ErrThresholdFailed) is true; the error text names every failing agent with score and threshold"
      - "A run covering selftest and selftest-fail records total_cost equal to the sum of both agents' costs and exits non-zero naming only selftest-fail"
      - "Single-case (--case) and --resume paths print the same verdict line and return the same error; results, summary and .progress cleanup still happen before the error is returned"
      - "Running the CLI: 'task eval:selftest' exits 0 and the selftest-fail command exits non-zero without printing usage text"
      - "Existing tests that ran failing agents expecting a nil error are updated to assert the threshold error explicitly"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./internal/eval/... ./cmd/... -count=1"
      - "go build -o /tmp/kairon-316 ./cmd/kairon"
      - "task eval:selftest"
      - "/tmp/kairon-316 eval --backend stub --evals-dir internal/eval/testdata/evals selftest | grep -E '^PASS selftest: '"
      - "! /tmp/kairon-316 eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail > /dev/null 2>&1"
      - "/tmp/kairon-316 eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail 2>&1 | grep -E '^FAIL selftest-fail: '"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "update-docs"
    agent: "builder"
    description: "Update docs/evaluation.md: document rubric pass_threshold and case min_score with the resolution order; rewrite the Skipped Criteria / scoring sections for the new counting rule; add a Pass/Fail Verdict and Exit Status section; document agent_verdicts, total_cost accumulation and the new case/agent threshold fields in the summary and result JSON; replace the 'E7 is separate work' paragraph; update the self-test section (selftest-fail has 13 cases incl. stub-empty-response, selftest-fail run exits non-zero); note legacy results still load in eval diff. Non-code change, no frozen paths."
    dependencies: ["agent-verdict-summary"]
    acceptance_criteria:
      - "docs/evaluation.md documents pass_threshold (percent, (0,100], default 95) in Rubric Format and min_score in Test Case Format, plus the order min_score -> rubric pass_threshold -> 95"
      - "No sentence remains claiming skipped criteria are excluded from totals, that kairon eval exits 0 on failing cases, or that the per-agent verdict is separate/future work"
      - "agent_verdicts fields (score, threshold, passed, cases_total, cases_failed, cost) and the percent-vs-fraction distinction from agent_scores are documented with the exact PASS/FAIL line format and exit codes"
      - "Self-test section lists stub-empty-response, shows 13 selftest-fail cases and the non-zero exit of the selftest-fail command"
      - "Only docs/evaluation.md changed in this task; git diff shows no change under .kairon/evals, .kairon/specs or cmd/kairon/templates"
    validation_commands:
      - "grep -n 'pass_threshold' docs/evaluation.md"
      - "grep -n 'min_score' docs/evaluation.md"
      - "grep -n 'agent_verdicts' docs/evaluation.md"
      - "! grep -n 'The run itself exits 0' docs/evaluation.md"
      - "! grep -n 'E7) is separate work' docs/evaluation.md"
      - "task sync:check"

  - id: "validate-complete"
    agent: "validator"
    description: "Read-only end-to-end verification of issue #316: run the full test suite and the CLI acceptance checks, verify each of the six acceptance criteria against the code, docs and real command output, and verify that no frozen path (.kairon/evals, other .kairon/specs files, cmd/kairon/templates, CHANGELOG.md) was modified and that tests were committed together with implementation."
    dependencies: ["eval-exit-status", "update-docs"]
    acceptance_criteria:
      - "AC1: docs/evaluation.md documents pass_threshold (default 95) and min_score override; loadRubrics accepts pass_threshold; tests cover default, rubric value and min_score override"
      - "AC2: the selftest-fail stub-empty-response case scores 0 on every criterion and stays in the denominator; the selftest-fail agent score is lower than without it"
      - "AC3: the self-test summary.json contains score, threshold, passed, cases_total and cases_failed per agent under agent_verdicts, and stdout contains one PASS/FAIL line per agent"
      - "AC4: task eval:selftest exits 0 and the selftest-fail CLI run exits non-zero"
      - "AC5: a run of selftest and selftest-fail together records total_cost equal to the sum of both agents' costs"
      - "AC6: 'kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2' succeeds"
      - "go test, go vet, task fmt:check and task sync:check all pass; git diff against the base shows no change to frozen paths"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "task fmt:check"
      - "task test"
      - "task sync:check"
      - "task eval:selftest"
      - "go build -o /tmp/kairon-316 ./cmd/kairon"
      - "! /tmp/kairon-316 eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail > /dev/null 2>&1"
      - "go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2"
      - "git diff --quiet $(git merge-base HEAD origin/main) -- .kairon/evals cmd/kairon/templates CHANGELOG.md"
```
