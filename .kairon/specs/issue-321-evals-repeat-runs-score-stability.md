# Design Spec: Evals — repeat runs to measure score stability

Closes #321

Dependencies: #315, #316 (both closed; the stub `judge` list and its persistent cursor, and the per-case workspace,
are already in the tree).

## Problem

A single eval run with an LLM judge is noisy. Promoting a prompt needs a **mean over several runs with the spread
visible**. Today `kairon eval <agent>` runs every case once and records one result per case.

## Current State (what the code does today)

- `cmd/kairon/cmd/eval.go` — cobra flags are copied into `eval.RunOptions` and handed to `eval.RunWithOptions`.
- `internal/eval/runner.go`
  - `RunWithOptions` → `configure(options)` (sets the package-level `cfg`, see `config.go`) → one of
    `runSingleTestCase` (`--case` / positional testcase), `runWithResume`, or `Run`.
  - `Run` → `runProgressiveEvaluation` → per rubric `evaluateProgressive`: loops cases, calls `executeCase`, `stampCase`,
    replaces-or-appends the result **by `CaseName`**, saves `<agent>.json` + `summary.json` after every case.
  - `runSingleTestCase` → `evaluate` (same loop without progressive save).
  - `isTestCaseCompleted` / `stampResumedCases` / `checkResumeIntegrity` identify a case by name.
- `internal/eval/execute_case.go` — `executeCase` is the one per-case path. It already creates a **fresh
  `newCaseWorkspace(tc)` on every call** and removes it after scoring (unless `--keep-workspaces`). So calling it N times
  already satisfies "each in a fresh workspace".
- `internal/eval/verdict.go` — `agentVerdict(AgentResult)` computes `Score` (percent) =
  `Σ case score * 100 / Σ case max` over `ar.Cases`, `Passed = score*100/max >= threshold`, `CasesTotal = len(ar.Cases)`,
  `CasesFailed` from `casePassed`. `setAgentVerdict` stores it in `Summary.AgentVerdicts[agent]`;
  `printAgentVerdict` prints the single `PASS|FAIL …` line.
- `internal/eval/scoring.go` — `caseTotals`, `agentScoreTotals` (pooled over all `ar.Cases`; feeds `agent_scores`).
- `internal/inference/stub*.go` — `stub.judge: [yes, no]` is a list **consumed through a cursor stored on the
  `*StubScript`** (`NextJudgeAnswer`). `TestSelfTestJudgeRepeatContinuesCursor` already pins that re-running the *same
  loaded* `TestCase` continues the cursor, and `TestSelfTestJudgeFreshLoadRestartsCursor` pins that a fresh load
  restarts it. **Consequence for this design: the cases must be loaded once and the same `TestCase` values reused for
  every repeat** — reloading per repeat would make every repeat see `yes` and AC2 could never show two different scores.

## Solution Approach

Add a `--repeat N` run option. Each case is executed N times through the existing `executeCase` (fresh workspace each
time). Every execution is kept as its own `CaseResult` in `AgentResult.Cases`, tagged with its 1-based `run` number. The
agent verdict is computed per repeat, then summarised as mean / min / max.

### Design decisions

1. **Flat results with a `run` tag (not a nested structure).** `AgentResult.Cases` stays a flat list; each entry gains
   `run` (1-based). With `--repeat 3` a case name appears three times (`run` 1, 2, 3). This keeps every existing
   consumer (`eval diff`, cost totals, `agent_scores`, resume) working on the same `[]CaseResult`. `run` is always
   written for new results (including `1` for a plain run); a result file without it (legacy) is read as `run` 1.
2. **Case-major execution order**: for each case, run it N times back to back, then move to the next case.
   - Partial progress stays consistent: after k of M cases every repeat covers the *same* k cases, so a mid-run
     `summary.json` has meaningful mean/min/max.
   - The progressive save (and resume) granularity stays "one case". The N results of a case are written together, so
     `isTestCaseCompleted` (by name) stays correct and unchanged: a case is completed only when all N repeats are saved.
     Cost of a crash is at most N-1 repeats of one case.
3. **Per-repeat score.** For repeat k, `score_k = Σ over cases of (run k) score * 100 / Σ over cases of (run k) max`
   (percent, same arithmetic as today, over the non-cost criteria; skipped/errored/no-output criteria count 0 with
   their max in the denominator — unchanged `caseTotals`).
4. **Summary statistics.**
   - `score` = the **mean**. Computed as the pooled `Σ score * 100 / Σ max` across all repeats. Every repeat has the
     same denominator (same cases, same rubric, `caseTotals` always includes each criterion's max), so this equals the
     arithmetic mean of the per-repeat scores while keeping the existing multiply-before-divide boundary behaviour (19/20
     passes 95). For `repeat == 1` it is bit-for-bit today's value.
   - `score_min` / `score_max` = min / max of `score_k`.
   - `repeat_scores` = the list of `score_k` in run order (makes the spread and the AC2 check directly visible; one
     entry per completed repeat).
   - `passed = score (mean) >= threshold`. A repeat reaching the threshold while the mean does not is a FAIL.
   - **Fail closed** unchanged: nothing scored → `score` 0, `passed` false (and `score_min`/`score_max` 0).
5. **`cases_total` / `cases_failed` with repeats.** `cases_total` = number of *distinct* case names. A case counts as
   failed when its group of results is a single result and `casePassed` says so (today's logic, unchanged for
   `repeat == 1` and legacy files), or — for several results of one case — when the case's pooled percent
   (`Σ score * 100 / Σ max` over its runs, via `percentAtLeast`) is below that case's threshold (`CaseResult.Threshold`
   when > 0, else the agent threshold). Still informational only.
6. **Cost** (`agentCostTotals`) already sums every `CaseResult`, so it correctly becomes the total cost of all repeats.
   `agent_scores` (0–1 fraction, `agentScoreTotals`) already pools every result, which *is* the mean — no code change.
7. **Where the numbers live.**
   - `summary.json` → `agent_verdicts.<agent>`: new fields `repeat`, `score_min`, `score_max`, `repeat_scores`;
     existing `score` is now the mean. These are the fields the promotion issues read. They are **not** `omitempty`
     (a legitimate `score_min` of `0` must be written). A `repeat == 1` run writes `repeat: 1`, `score_min == score_max
     == score`, `repeat_scores: [score]`.
   - `<agent>.json` → `AgentResult.repeat` (omitempty, absent in legacy files = 1) and `cases[].run`.
8. **Configuration reaches the runner through `cfg`** (the package already says "configuration is kept here rather than
   threaded through"), as `cfg.repeat`, set by `configure(opts)`. That avoids changing the signatures of `evaluate`,
   `evaluateProgressive`, `runSingleTestCase` etc., which many existing tests call directly. An unset/zero value means 1,
   so every existing test and direct caller is unchanged.
9. **Validation.**
   - `--repeat` below 1 is rejected by the CLI (`RunE`, before `cmd.SilenceUsage = true` so usage is shown, as for other
     argument mistakes) with an error like `--repeat must be at least 1 (got 0)`; the process exits non-zero. This check
     is in the cmd layer because `RunOptions.Repeat == 0` must keep meaning "default 1" for programmatic callers.
   - `RunWithOptions` additionally rejects `Repeat < 0`, and rejects `Repeat > 1` combined with `--list`, `--cleanup` or
     `--perf` (they never run cases, so a silent no-op would mislead). These checks happen first, before any state change.
10. **Resume.** `--resume` keeps working. `checkResumeIntegrity` gains a check next to the sandbox-mode check: if the
    saved `<agent>.json` has cases and its effective repeat (`Repeat`, 0 → 1) differs from the current `--repeat`, refuse
    with `cannot resume: repeat changed since the interrupted run of <agent> (recorded repeat=N; now M) — resume with
    --repeat N or start a fresh run`. This prevents one result file from mixing N values. `evaluateProgressive`
    stamps `result.Repeat` from the current run after loading a resumed file, like it does for threshold/provenance.
    Replace-by-name in `evaluateProgressive` must replace **all** existing results of that case name with the new N.
11. **Output.** For `repeat == 1` the console output is byte-identical to today. For `repeat > 1`:
    - Under the `[i/M] <case>` line print one line per run, prefixed `      run k/N`, followed by the usual
      progress text and the existing `printCaseResult` status for that run.
    - `printAgentVerdict`: for `Repeat <= 1` unchanged; for `Repeat > 1` append a suffix **after** the existing text so the
      line still starts with `PASS `/`FAIL ` and the old prefix still matches:
      `PASS|FAIL <agent>: <mean>% (threshold <t>%), <failed>/<total> cases failed [repeat <N>: min <min>%, max <max>%]`.
    - The `ThresholdError` text is unchanged; its percentage is now the mean.
12. **`eval diff`.** Unchanged except `showOutputChanges`, which maps case name → output: it must keep the **first**
    (run 1) output per case instead of the last, so it is deterministic with repeats. `criterionAverages` and cost
    summing already work over all results (they become means / totals).
13. **Fresh workspace guarantee.** Provided by reusing `executeCase` per run. A test must prove it (distinct
    `workspace_dir` per run with `--keep-workspaces`, and a stub case that writes a file ends each run with that file
    exactly once, not accumulated).
14. **Template sync.** Nothing under the template-synchronized live paths (`.kiro/agents`, `.kairon/scripts`,
    `.kairon/themes`, `.kairon/evals/{fixtures,rubrics,cases}`, `sentinel-protocol`) changes. The new self-test fixture lives
    in `internal/eval/testdata/evals/`, which `task sync:check` does not cover. Builder still runs `task sync:check`
    and records "Template Sync: N/A" in its sentinel.

### Out of scope

Parallel execution of repeats; new check types; showing the spread in `eval diff`; auto-promotion logic (that is the
"promotion issues"). No change to the existing `agent_scores` meaning.

## Relevant Files

Modify:
- `internal/eval/types.go` — `CaseResult.Run int \`json:"run,omitempty"\``; `AgentResult.Repeat int \`json:"repeat,omitempty"\``;
  `RunOptions.Repeat int` (doc: 0 means 1; negative is rejected).
- `internal/eval/verdict.go` — `AgentVerdict` new fields (`Repeat`, `ScoreMin`, `ScoreMax`, `RepeatScores`, JSON names
  `repeat`, `score_min`, `score_max`, `repeat_scores`, no `omitempty`); `agentVerdict` per-repeat/mean/min/max and distinct-case
  accounting; `printAgentVerdict` suffix for `Repeat > 1`.
- `internal/eval/scoring.go` — a small helper that groups `ar.Cases` by run (e.g. `repeatTotals(ar) []runTotals`), next to
  `caseTotals`/`agentScoreTotals` so score arithmetic stays in one place. `run == 0` is treated as 1.
- `internal/eval/config.go` — `runConfig.repeat`, set in `configure`; helper `runRepeat()` returning `max(1, cfg.repeat)`;
  validation helper for `RunOptions` (negative, and `>1` with list/cleanup/perf).
- `internal/eval/runner.go` — `RunWithOptions` validation; `evaluate` and `evaluateProgressive` run each case
  `runRepeat()` times (a shared helper, e.g. `executeCaseRepeats`, calling `executeCase` + `stampCase` and setting
  `cr.Run = k`); replace-all-by-name; `result.Repeat` stamping in both; `checkResumeIntegrity` repeat check;
  `runSingleTestCase` already goes through `evaluate`.
- `internal/eval/diff.go` — `showOutputChanges` keeps the first output per case.
- `cmd/kairon/cmd/eval.go` — `--repeat` int flag (default 1), validation, pass `Repeat` to `RunOptions`.
- `docs/evaluation.md` — see Task `repeat-docs`.
- `internal/eval/selftest_checks_test.go` — selftest-fail case count `10+1+1+1+1+5` → `+1` (new case) and the comment.

Create:
- `internal/eval/testdata/evals/cases/selftest-fail/check-judge-repeat.yaml` — one criterion
  (`structural_completeness`), **one** `judge` check, `stub.judge: [yes, no]`, same stub turn/response shape as
  `check-judge-no.yaml`. With `--repeat 2` run 1 scores `1/1` (yes) and run 2 `0/1` (no).
- `internal/eval/repeat_test.go` (verdict/arithmetic unit tests, execution/integration tests, validation tests).
- cmd-layer tests appended to `cmd/kairon/cmd/eval_test.go`.

Relevant, read-only context: `internal/eval/execute_case.go`, `internal/eval/selftest_judge_test.go`
(`loadSelfTestCase`, cursor tests), `internal/eval/exit_status_test.go` (`runStubCapturing`, `onlyRunDir`),
`internal/eval/selftest_checks_test.go` (`runSelfTestAgent`, `runEvalIn`), `internal/inference/stub.go`, `AGENTS.md`.

Adding a case to `selftest-fail` can change other `selftest-fail` assertions (case counts, aggregate score numbers in
`docs/evaluation.md` self-test text such as "nineteen cases"). The builder must run the whole `internal/eval` and
`cmd/...` suites and fix every assertion/doc sentence the new case invalidates (`selftest-fail` has nineteen cases today;
it will have twenty, and the new `check-judge-*` case makes the docs' "five `check-judge-*`" six).

## Team Orchestration

Sequential core with a docs task that can start once the interfaces exist:

1. `repeat-verdict-model` (builder) — result types + verdict arithmetic + verdict line. Pure functions, unit-testable alone.
2. `repeat-execution` (builder, after 1) — options/config/validation, repeat loops, resume integrity, self-test fixture,
   integration tests.
3. `repeat-cli` (builder, after 2) — flag and CLI-level rejection of `--repeat 0`.
4. `repeat-docs` (builder, after 2 and 3) — `docs/evaluation.md`.
5. `validate-repeat` (validator, after 4) — read-only verification of every acceptance criterion and the full suite.

Tasks 3 and 4 touch disjoint files (`cmd/kairon/cmd` vs `docs/`) but task 4 documents the flag, so it waits for 3.
Follow `AGENTS.md` TDD: for each code task write the failing test first, confirm it fails for the expected reason (not a
compile error — add the new struct fields/stubs first if needed so the test compiles and fails on the assertion), then
implement, commit test and implementation together, and name the observed-failing test in the commit message/sentinel.
Docs/prompt/config changes need no test-first.

## Step-by-Step Task Breakdown

### Task 1: `repeat-verdict-model`
Types and arithmetic.
- Add `CaseResult.Run`, `AgentResult.Repeat`, `RunOptions.Repeat`, `AgentVerdict.{Repeat,ScoreMin,ScoreMax,RepeatScores}`.
- Group results by run (legacy `Run == 0` → run 1) and compute per-repeat `(score, max)`.
- `agentVerdict`: `Repeat = max(ar.Repeat, 1)`; `Score` = pooled mean; `ScoreMin/Max` over per-repeat scores;
  `RepeatScores` in run order; `Passed = Score >= threshold` (exact boundary preserved); `CasesTotal` = distinct names;
  `CasesFailed` per Design decision 5; fail closed when nothing scored.
- `printAgentVerdict` suffix for `Repeat > 1`; unchanged otherwise.
- Tests (first, failing): table-driven `agentVerdict` tests on synthetic `AgentResult`s — repeat 1 equals the previous
  output exactly (including a legacy result with `Repeat 0`, `Run 0`, and `Passed == nil` cases); two repeats at 100% and
  80% → mean 90, min 80, max 100, `repeat_scores [100 80]`; **mean decides**: repeats 100 and 90 with threshold 95 pass
  (mean 95 exactly), repeats 100 and 80 fail although the best repeat is above the threshold; nothing scored fails closed
  with min = max = 0; distinct-case counting with 3 runs of 2 cases; JSON field names (`repeat`, `score_min`,
  `score_max`, `repeat_scores`) are present even when `0`; `printAgentVerdict` output for repeat 1 is byte-identical to the
  current format (existing `TestPrintAgentVerdictFormat` must still pass) and has the `[repeat N: min …, max …]` suffix
  for N > 1 while still starting with `PASS `/`FAIL `.

### Task 2: `repeat-execution`
Execution, configuration, resume, fixture.
- `configure` sets `cfg.repeat`; `runRepeat()`; `RunWithOptions` rejects `Repeat < 0` and `Repeat > 1` with
  `List`/`Cleanup`/`Perf` before any state change.
- `evaluate` and `evaluateProgressive`: per case, run `runRepeat()` times through `executeCase` (workspace fresh each
  time), `stampCase`, set `cr.Run = k`; print `      run k/N` lines only when N > 1; save progressively after all N
  repeats of a case; replace all prior results of that case name; set `result.Repeat = runRepeat()` (also after loading
  a resumed file). Cases must be loaded **once** and the same `TestCase` values reused (judge cursor).
- `checkResumeIntegrity`: repeat-change refusal (legacy 0 → 1; only when the saved file has cases).
- `showOutputChanges`: keep the first output per case.
- Add `selftest-fail/check-judge-repeat.yaml`; update the `selftest-fail` case-count assertion and any other assertion the
  new case invalidates.
- Tests (first, failing), in `internal/eval/repeat_test.go`, using the existing helpers (`runStubCapturing`,
  `runEvalIn`/`copyFixturesTo`, `onlyRunDir`, `loadSummary`/`loadAgentResult`):
  - **AC1**: `selftest` with `Repeat: 3` → `len(res.Cases) == 3 × (cases in selftest)`; each case name has exactly runs
    `{1,2,3}`; `res.Repeat == 3`; with `KeepWorkspaces: true` all `workspace_dir`s are distinct and exist, and the
    `stub-marker` case's workspace holds `marker.txt` exactly once per run (fresh, non-accumulating). Clean up kept
    workspaces (see `cleanupWorkspace`).
  - **AC2**: `selftest-fail` case `check-judge-repeat` (`testcase` arg) with `Repeat: 2` → exactly two results with
    per-repeat scores 1/1 and 0/1 (judge details "judge answered yes" then "judge answered no"); `summary.json`
    `agent_verdicts.selftest-fail` has `repeat 2`, `score_min 0`, `score_max 100`, `score 50`, `repeat_scores [100 0]`,
    `passed false`; the run returns an error `errors.Is(err, ErrThresholdFailed)`; the printed verdict line starts with
    `FAIL selftest-fail:` and carries `min 0.0%, max 100.0%`. Also run the whole `selftest-fail` agent with `Repeat: 2`
    and assert `score_min < score_max`, `score` between them, and that `min`/`max`/`mean` equal the values recomputed
    independently from `<agent>.json` per-run totals.
  - Repeat 1 / unset is unchanged: `Repeat: 0` and `Repeat: 1` produce `Repeat == 1`, `run == 1`, `score_min == score ==
    score_max`, and the old console output.
  - **Verdict uses the mean** end to end: a scripted agent whose repeats score 100 and 80 against threshold 95 fails
    although `score_max` ≥ threshold (can reuse `check-judge-repeat` with a rubric `pass_threshold` override in the temp
    evals copy, or the synthetic unit test plus this integration run).
  - Validation: `RunWithOptions` with `Repeat: -1` errors; `Repeat: 2` with `List`, `Cleanup`, `Perf` errors; none of
    these create a results directory.
  - Resume: interrupted `Repeat: 2` result (write a partial `<agent>.json` with both runs of the first case plus `.progress`) is
    resumed with `Repeat: 2` and does not re-run or duplicate the completed case; resuming with `Repeat: 3` (or 1) is
    refused with the `repeat changed` message; a legacy file without `repeat`/`run` resumes with repeat 1.
  - Cost: `summary.total_cost`/`agent_verdicts.<agent>.cost` for `Repeat: 3` is 3× the cost of `Repeat: 1` with the stub
    backend (deterministic estimated usage).
  - `eval diff` of a repeat-3 run against a repeat-1 run does not error and `showOutputChanges` uses run 1.

### Task 3: `repeat-cli`
- `cmd/kairon/cmd/eval.go`: `var evalRepeat int`; `evalCmd.Flags().IntVar(&evalRepeat, "repeat", 1, "Run every case N times ...")`;
  at the top of `RunE` (before `cmd.SilenceUsage = true`) `if evalRepeat < 1 { return fmt.Errorf("--repeat must be at least 1 (got %d)", evalRepeat) }`;
  pass `Repeat: evalRepeat` in `RunOptions`.
- Tests (first, failing), appended to `cmd/kairon/cmd/eval_test.go`: the flag is registered with default `1`;
  `RunE` with `evalRepeat` 0 and -1 returns an error naming `--repeat`, creates no results dir, and does not touch the
  backend; **AC3** additionally via a real process: `go run ./cmd/kairon eval --repeat 0 selftest` exits non-zero (a Go test
  using `exec.Command` on a built binary, or the validation command below); with `evalSelfTestProject` (stub backend)
  `evalRepeat = 3` + agent `selftest` runs and the run directory's `selftest.json` has 3 results per case and
  `summary.json` has `repeat: 3`. Restore `evalRepeat` to 1 in `t.Cleanup` so other tests are unaffected.

### Task 4: `repeat-docs`
Update `docs/evaluation.md` (live doc, edit freely; no template sync):
- **Running Evaluations**: example `kairon eval architect --repeat 5` and a `--repeat` row in the flag table (default
  `1`; `< 1` rejected; cannot be combined with `--list`/`--cleanup`/`--perf`; combined with `--case` repeats that case;
  `--resume` requires the same value).
- New section **Repeat runs (`--repeat`)** (after "Pass/Fail Verdict and Exit Status"): why (judge noise, 95%+ stage 3
  promotion), how it runs (case-major, fresh workspace each run, same loaded case so a `stub.judge` list continues across
  runs), what is kept (`cases[].run`, `repeat`), how the mean/min/max/`repeat_scores` are computed and that the verdict
  uses the mean (a best repeat above the threshold does not rescue a mean below it), cost = all repeats, the output line
  format with the `[repeat N: min …, max …]` suffix, resume rules, and that these fields are the stable contract for promotion
  tooling.
- Update **Agent verdict**, **Output and exit status**, **Percent versus fraction** and the **Run summary**
  `agent_verdicts.<agent>` field table/JSON example (`repeat`, `score_min`, `score_max`, `repeat_scores`; `score` is the mean;
  `cases_total` distinct cases) and the **Result JSON** notes for `<agent>.json` (`repeat`, `cases[].run`).
- **Self-Test** section: add `selftest-fail/check-judge-repeat.yaml` to the fixture tree, update counts ("nineteen" →
  "twenty" cases, "five `check-judge-*`" → six) and add the verification recipe:
  `kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest --repeat 3` (3 results per case) and
  `… selftest-fail check-judge-repeat --repeat 2` (two different scores, exits 1).
- Any YAML fence containing `checks:` must still parse (`checks_docs_test.go`); run it.

### Task 5: `validate-repeat`
Read-only: run the full test/lint/sync suite and manually exercise each acceptance criterion with the stub backend,
inspecting the generated JSON (see Validation Commands). Report per-criterion pass/fail.

## Acceptance Criteria Mapping (issue → verification)

| # | Criterion | Verified by |
|---|-----------|-------------|
| 1 | `--repeat N` runs every case N times in a fresh workspace and keeps every result | `TestRepeat…` AC1 test (3 results per `selftest` case, distinct kept workspaces) + CLI run below |
| 2 | Summary has `repeat`, `score` (mean), `score_min`, `score_max` per agent; verdict uses the mean | AC2 test on `check-judge-repeat` (`[yes, no]`, `--repeat 2`: scores 100/0 → min 0, max 100, mean 50, FAIL) + unit tests for the mean-decides rule |
| 3 | `--repeat` < 1 rejected, non-zero exit | cmd test and `go run … --repeat 0` exit status |

## Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/eval/... ./cmd/... -count=1
task test
task lint
task sync:check

# AC1: 3 results per case
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest --repeat 3
#   then inspect the newest run dir under internal/eval/testdata/evals/results/ (git-ignored):
#   jq '[.cases[] | .case_name] | group_by(.) | map(length) | unique' <run>/selftest.json   # -> [3]
#   jq '.agent_verdicts.selftest | {repeat, score, score_min, score_max, repeat_scores}' <run>/summary.json

# AC2: [yes, no] -> two different scores; exits non-zero (mean 50 < 95)
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail check-judge-repeat --repeat 2 ; test $? -ne 0
#   jq '.agent_verdicts."selftest-fail" | {repeat, score, score_min, score_max, repeat_scores}' <run>/summary.json
#   -> repeat 2, score 50, score_min 0, score_max 100, repeat_scores [100, 0]

# AC3: rejected
! go run ./cmd/kairon eval --repeat 0 selftest
! go run ./cmd/kairon eval --repeat -1 selftest
```

## Risks and Notes

- **Judge cursor**: the only reason AC2 works is that the same loaded `TestCase` (same `*StubScript`) is reused across repeats.
  The implementation must not call `loadCases` per repeat; a test pins it.
- **Flat results**: anything iterating `AgentResult.Cases` and keying by `CaseName` sees N entries now. The in-tree users
  were audited: `evaluateProgressive` (replace-all), `isTestCaseCompleted` (unchanged, correct), `stampResumedCases`
  (stamps every match), `diff.go` (`criterionAverages` fine, `showOutputChanges` fixed), `calculateAgentJudgeCosts` (sums all,
  fine), `agentVerdict` (reworked), `agentScoreTotals` / `agentCostTotals` (pool all, correct). The builder must re-grep
  `\.Cases` / `CaseName` for any consumer added since.
- **Output stability**: single-run output and the `PASS|FAIL` prefix are unchanged so existing tests and scripts that
  `grep '^PASS \|^FAIL '` keep working.
- **Denominator invariant** (every repeat has the same max) is what makes pooled == mean; a unit test asserts the verdict
  mean equals `Σ repeat_scores / len(repeat_scores)` within 1e-9 for equal-denominator inputs.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "repeat-verdict-model"
    agent: "builder"
    description: "Add run/repeat fields to the result types (CaseResult.Run, AgentResult.Repeat, RunOptions.Repeat) and extend AgentVerdict with repeat, score_min, score_max and repeat_scores; rework agentVerdict to compute per-repeat scores, the pooled mean, min/max and distinct-case accounting (legacy results read as one repeat); extend printAgentVerdict with a suffix for repeat > 1. Write the failing unit tests first (AGENTS.md TDD)."
    dependencies: []
    acceptance_criteria:
      - "CaseResult has Run (json run, omitempty), AgentResult has Repeat (json repeat, omitempty), RunOptions has Repeat (0 means 1)"
      - "AgentVerdict serialises repeat, score_min, score_max and repeat_scores without omitempty (a score_min of 0 is written)"
      - "agentVerdict for repeat 1 and for legacy results (Repeat 0, Run 0, Passed nil) returns exactly the values it returned before, with score_min == score_max == score and repeat 1"
      - "For per-repeat scores 100 and 80 the verdict has score 90, score_min 80, score_max 100, repeat_scores [100 80]; for 100 and 90 at threshold 95 it passes (mean 95) and for 100 and 80 it fails even though the best repeat is above the threshold"
      - "cases_total counts distinct case names and cases_failed follows the pooled per-case rule for multiple runs and the unchanged casePassed rule for a single run"
      - "Nothing scored fails closed with score, score_min and score_max all 0 and passed false"
      - "printAgentVerdict output is byte-identical for repeat <= 1 and for repeat > 1 still starts with PASS or FAIL and ends with a [repeat N: min X%, max Y%] suffix"
      - "A failing test was observed to fail on its assertion (not a compile error) before the implementation; its name is recorded in the sentinel"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'TestRepeatVerdict|TestPrintAgentVerdict|TestAgentVerdict' -count=1"
      - "go test ./internal/eval/ -count=1"

  - id: "repeat-execution"
    agent: "builder"
    description: "Run every case Repeat times through executeCase (fresh workspace each run, same loaded TestCase so the stub.judge cursor continues) in evaluate and evaluateProgressive, tag results with run, set AgentResult.Repeat, replace-all-by-name and save after all repeats of a case; plumb Repeat through configure/cfg; validate options in RunWithOptions; add the resume repeat-mismatch refusal; make showOutputChanges keep the run-1 output; add the selftest-fail/check-judge-repeat fixture and update assertions it invalidates. Write the failing integration tests first."
    dependencies: ["repeat-verdict-model"]
    acceptance_criteria:
      - "selftest run with Repeat 3 records exactly 3 CaseResults per case with run 1,2,3 and AgentResult.Repeat 3; with KeepWorkspaces every run has a distinct existing workspace_dir and a file written by a stub case appears exactly once per workspace"
      - "selftest-fail/check-judge-repeat.yaml exists with one judge check and stub.judge [yes, no]; run with Repeat 2 it records per-repeat scores 1/1 and 0/1, summary agent_verdicts has repeat 2, score_min 0, score_max 100, score 50, repeat_scores [100 0], passed false, and the run returns an error satisfying errors.Is(err, ErrThresholdFailed)"
      - "Running the whole selftest-fail agent with Repeat 2 gives score_min < score_max and score equal to the mean of the per-repeat totals recomputed from <agent>.json"
      - "The verdict uses the mean end to end: repeats of 100 and 80 against threshold 95 produce passed false"
      - "Repeat 0/1 runs are unchanged (run 1, repeat 1, identical console output); cases are loaded once per agent per run, not per repeat"
      - "RunWithOptions rejects Repeat < 0 and Repeat > 1 with List, Cleanup or Perf before creating any results directory"
      - "Resume with the same Repeat skips a fully completed case without duplicating results; resume with a different Repeat is refused with a 'repeat changed' error; a legacy result without repeat/run resumes as repeat 1"
      - "Total cost with the stub backend at Repeat 3 is three times the Repeat 1 cost"
      - "eval diff between a repeat-3 and a repeat-1 run does not error and compares run-1 outputs"
      - "The selftest-fail case-count assertion (selftest_checks_test.go) and every other assertion invalidated by the new case are updated; go test ./internal/eval/... passes"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./internal/eval/ -run 'TestRepeat' -count=1"
      - "go test ./internal/eval/... -count=1"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest"
      - "task sync:check"

  - id: "repeat-cli"
    agent: "builder"
    description: "Add the --repeat flag (int, default 1) to kairon eval, reject values below 1 in RunE before any work with a non-zero exit, and pass Repeat in RunOptions. Write the failing cmd tests first."
    dependencies: ["repeat-execution"]
    acceptance_criteria:
      - "kairon eval --help lists --repeat with default 1"
      - "kairon eval --repeat 0 selftest and --repeat -1 return an error that names --repeat and exit non-zero, create no results directory and never reach the backend"
      - "With the stub backend, kairon eval selftest --repeat 3 writes selftest.json with 3 results per case and summary.json with repeat 3"
      - "Tests restore evalRepeat to 1 so other cmd tests are unaffected; go test ./cmd/... passes"
    validation_commands:
      - "go build ./..."
      - "go test ./cmd/... -run 'TestEval' -count=1"
      - "go test ./cmd/... -count=1"
      - "go run ./cmd/kairon eval --help | grep -e '--repeat'"
      - "! go run ./cmd/kairon eval --repeat 0 selftest"
      - "! go run ./cmd/kairon eval --repeat -1 selftest"

  - id: "repeat-docs"
    agent: "builder"
    description: "Update docs/evaluation.md: --repeat example and flag row, a new 'Repeat runs (--repeat)' section, updated agent verdict / output / summary field tables and JSON examples (repeat, score_min, score_max, repeat_scores, cases[].run, mean semantics), and the Self-Test section (new check-judge-repeat case, updated case counts, verification recipes). Docs only; no test-first required."
    dependencies: ["repeat-execution", "repeat-cli"]
    acceptance_criteria:
      - "docs/evaluation.md documents --repeat (default 1, <1 rejected, rejected with --list/--cleanup/--perf, repeats --case, --resume needs the same value)"
      - "The verdict sections state that score is the mean over repeats, define score_min/score_max/repeat_scores/repeat, and show the [repeat N: min, max] output suffix"
      - "The summary.json and <agent>.json field tables and JSON examples include repeat, score_min, score_max, repeat_scores and cases[].run"
      - "The Self-Test section lists check-judge-repeat.yaml, counts (twenty selftest-fail cases, six check-judge-* cases) match the fixtures, and the AC1/AC2 verification commands are given"
      - "Every ```yaml fence containing checks: still parses and go test ./internal/eval -run Docs passes"
    validation_commands:
      - "grep -n -e '--repeat' docs/evaluation.md"
      - "grep -n 'score_min' docs/evaluation.md"
      - "go test ./internal/eval/ -run 'Docs' -count=1"
      - "task sync:check"

  - id: "validate-repeat"
    agent: "validator"
    description: "Read-only verification of issue #321: run the full build, tests, lint and template sync check, exercise each acceptance criterion with the stub backend and inspect the generated JSON, and confirm the existing single-run output and PASS/FAIL line prefix are unchanged."
    dependencies: ["repeat-docs"]
    acceptance_criteria:
      - "AC1: selftest --repeat 3 records 3 results per case, each in its own workspace"
      - "AC2: selftest-fail check-judge-repeat --repeat 2 records two different per-repeat scores; summary agent_verdicts has repeat, score (mean), score_min and score_max that match them; verdict is FAIL and the exit status is non-zero"
      - "AC3: --repeat 0 (and -1) exits non-zero"
      - "A plain kairon eval selftest run still prints PASS selftest: 100.0% (threshold 95.0%), 0/<n> cases failed with no repeat suffix and writes repeat 1, run 1"
      - "go test ./..., task lint and task sync:check all pass; no files outside the declared scope (internal/eval, internal/eval/testdata/evals, cmd/kairon/cmd, docs/evaluation.md) were modified"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./... -count=1"
      - "task lint"
      - "task sync:check"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest --repeat 3"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail check-judge-repeat --repeat 2 ; test $? -ne 0"
      - "! go run ./cmd/kairon eval --repeat 0 selftest"
```
