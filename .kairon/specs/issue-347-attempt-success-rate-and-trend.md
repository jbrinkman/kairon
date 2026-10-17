# Design Spec: Evals: report attempt success rate and a score trend

Closes #347

## 1. Problem Summary

Stage 4 of the maturity model needs two things the eval harness does not produce today: the end-to-end number (how often a full attempt succeeds) and a trend over time.

1. **Pooled score hides failed attempts.** `agentVerdict` (`internal/eval/verdict.go`) pools every point of every case into one percent. `caseAccounting` then pools the repeats of each case before judging it, so repeats never show up as separate attempts. Measured on the committed self-test with `--backend stub --repeat 2 selftest-fail`: 42 case results, 1 of them `passed: true`, yet the verdict reports `cases_total: 21`, `cases_failed: 21`, and nothing counts attempts.
2. **No view across runs.** `kairon eval diff` compares exactly two runs. Nothing lists an agent's results over `<evals-dir>/results/`.

For a workflow agent such as krew-lead, one `CaseResult` is one end-to-end attempt, so counting case results gives the Stage 4 number.

Dependencies #342 (per-criterion scores, commit `4413995`) and #343 (run coverage: `complete`, `suite_sha256`, `case_filter`, commit `23e1370`) are merged and closed. This change builds on both: the verdict code already has the criteria and floor logic, and `loadRunRecord` already reads `complete`.

Out of scope (per the issue): production workflow metrics from the watcher, charts, HTML output. Also not changed: `kairon eval diff` output, how scores are computed, the `passed` rule of the verdict.

## 2. Solution Approach

Two independent additions that share one schema.

### 2.1 Verdict fields (AC 1, AC 2)

Add three fields to `AgentVerdict`, written to `summary.json` under `agent_verdicts.<agent>`. The names are fixed: Stage 4 workflow certification reads them.

| Field | Type | Definition |
|-------|------|------------|
| `attempts_total` | integer | `len(ar.Cases)`: every `CaseResult` of the agent, across all cases and all repeats. |
| `attempts_passed` | integer | The number of those results for which `casePassed(c, agentThreshold(ar))` is true. |
| `success_rate` | number, percent (0-100) | `attempts_passed * 100 / attempts_total`; `0` when `attempts_total` is `0`. |

Decisions:

- **An attempt is a case result, not a pooled case.** This is the difference from `cases_total` / `cases_failed`, which count distinct cases and pool repeats. `casePassed` already honours a stamped `Passed` and recomputes a legacy result, so no new pass logic is added. A result with no output is stamped `passed: false` by `stampCase`, so it counts as a failed attempt.
- **Always written.** No `omitempty`: `0` is a real value (every attempt failed) and must be distinguishable from "field missing" by Stage 4 consumers. A test asserts the three keys are present in the raw JSON, including when the rate is `0`.
- **Independent of the floor and the threshold.** A `min_criterion_score` failure or a score below `pass_threshold` does not change the attempt counts. An agent can pass at 95% with a success rate of 67%. That gap is the point of the field.
- **Computed from the result alone,** inside `agentVerdict`, like every other verdict field. `setAgentVerdict` is called on every progressive save and on `--resume`, so the counts stay correct and idempotent with no extra bookkeeping.
- **Unrounded in JSON,** like `score`. Rounding to one decimal happens only when printing.
- **Single-case runs** (`kairon eval <agent> <case>`) compute the verdict from the cases that ran, so the counts cover only those. Same rule as the floor.

New helper in `verdict.go`:

```go
// attemptAccounting returns the number of case results (attempts) of the agent
// and how many of them passed their own threshold.
func attemptAccounting(ar AgentResult, agentThreshold float64) (total, passed int)
```

### 2.2 PASS/FAIL line (AC 2)

`printAgentVerdict` inserts the rate directly after `cases failed`, before the floor suffix, the `[repeat ...]` suffix and the cost-incomplete marker:

```
PASS selftest: 100.0% (threshold 95.0%), 0/25 cases failed, success rate 100.0% (25/25 attempts)
FAIL selftest-fail: 8.1% (threshold 95.0%), 21/21 cases failed, success rate 2.4% (1/42 attempts) [repeat 2: min 6.5%, max 9.7%]
```

Format: `..., <cases_failed>/<cases_total> cases failed, success rate <rate>% (<passed>/<total> attempts)` with one decimal for the rate. The line still starts with `PASS ` / `FAIL `. A verdict with `attempts_total` 0 prints `success rate 0.0% (0/0 attempts)`.

Decision: inline placement, not a trailing suffix. The rate is part of the verdict, not an optional note, and a fixed position lets a reader or script match `success rate (\d+\.\d)%` on every line. The cost is that the exact-line assertions in existing tests and the line examples in `docs/evaluation.md` change. Section 4 lists them.

The same function prints the line for full runs, `--resume` and single-case runs (`runner.go`: two call sites), so no call site changes.

### 2.3 `kairon eval trend <agent>` (AC 3, AC 4, AC 5)

New file `internal/eval/trend.go`. Like `VerifyLog`, it takes explicit directories and never reads `cfg`, so it is unit-testable without package state:

```go
type TrendOptions struct {
    EvalsDir string // "" => defaultEvalsDir
    JSON     bool
}

type TrendRow struct {
    Run          string   `json:"run"`
    Date         string   `json:"date"`          // RFC 3339 UTC; "" when the run name has no timestamp
    Score        float64  `json:"score"`         // percent, same value as the verdict score
    SuccessRate  *float64 `json:"success_rate"`  // percent; nil (null) when not recorded
    AgentModel   string   `json:"agent_model"`   // "" when not recorded
    JudgeModel   string   `json:"judge_model"`   // "" when not recorded
    PromptSHA256 string   `json:"prompt_sha256"` // first 8 characters only; "" when not recorded
    Complete     *bool    `json:"complete"`      // nil (null) when the run predates coverage tracking
}

func LoadTrend(agent, evalsDir string) ([]TrendRow, error)
func Trend(w io.Writer, agent string, opts TrendOptions) error
```

**Which runs are listed.** Every directory directly under `<evals-dir>/results/` for which the existing `loadRunRecord(resultsDir, run, agent)` reports `exists && hasScore`. That is the same rule `verify-log` uses: a score from `agent_verdicts`, else `agent_scores`, else the scoreable cases of `<agent>.json`. Non-directories, directories with no data for the agent and runs of other agents are skipped silently. Reusing `loadRunRecord` gives the score, `agentModel`, `judgeModel`, `promptSHA` and `complete` with the same precedence `verify-log` applies (agent file first, then summary), and avoids a second reader that could drift.

**Order.** Oldest first. The date comes from the run directory name via the existing `splitRunDirName` (`YYMMDD-HHMMSS`, UTC, so `260701-100000-aaaaaaa` is `2026-07-01T10:00:00Z`). Sort key: parsed time, then directory name. A run whose name has no timestamp (the old hash-only format) has a zero time, so it sorts first and shows no date. Decision: do not fall back to file modification times. They change on checkout and would make the output machine-dependent.

**Success rate source.** In order: (1) the summary verdict's `success_rate` when its `attempts_total` is greater than 0; (2) otherwise recomputed with `agentVerdict(result)` from `<agent>.json`, when that file loads and has at least one case result; (3) otherwise `nil`. Step 2 gives runs written before this change a rate, since their case results already carry `passed`. A run with no case results has no attempts, so no rate. This differs from the verdict itself, which reports `0` for zero attempts.

**Completeness.** `Complete` is `&rec.complete` when `rec.hasComplete`, else `nil`. `complete: false` covers a `--case` run and an interrupted run. It is not an error; the row is listed and marked.

**Text output.** One line per run, no header, so `wc -l` equals the number of runs. Fields are separated by two spaces. An unknown value is `-`:

```
260701-100000-aaaaaaa  2026-07-01T10:00:00Z  score 80.0%  success 75.0%  agent_model agent-m  judge_model judge-m  prompt aaaaaaaa  complete
260703-100000-ccccccc  2026-07-03T10:00:00Z  score 100.0%  success 100.0%  agent_model agent-m  judge_model judge-m  prompt cccccccc  INCOMPLETE
```

The last field is `complete`, `INCOMPLETE` (uppercase, so it stands out and is greppable) or `completeness-unknown`.

**JSON output.** `--json` prints `json.MarshalIndent(rows, "", "  ")` followed by a newline: an array with one object per run, same rows and same order. Optional numbers and booleans are `null` when unknown, strings are `""`. The key `prompt_sha256` holds the 8-character prefix, as the issue asks for the first 8 characters in both forms; the docs say so.

**Errors (AC 5).** `Trend` returns an error and prints nothing when:

- the agent name is not a single path element (`validatePathElement`, as in `VerifyLog`);
- no run has a result for the agent, including a missing or empty results directory. Message: `no eval results for agent "nope" under <evals-dir>/results`.

An unreadable results directory other than "does not exist" is returned as an error. The cobra command sets `cmd.SilenceUsage = true` before calling it so the failure is a result, not a usage mistake. The process exit status is non-zero through the usual `RunE` error path.

### 2.4 CLI wiring

New file `cmd/kairon/cmd/eval_trend.go`, registered with `evalCmd.AddCommand(trendCmd)` in `init()`:

- `Use: "trend <agent>"`, `Args: cobra.ExactArgs(1)`.
- `--json` bool flag (local to `trend`).
- `--evals-dir` is inherited from `evalCmd` (persistent flag `evalEvalsDir`); it is not redefined.
- Output goes to `cmd.OutOrStdout()`.

### 2.5 Fixture for the trend (AC 3, AC 4, AC 5)

New directory `internal/eval/testdata/trend/results/` (not covered by `.gitignore`, which ignores only `testdata/evals/results/`). The AC commands use `--evals-dir internal/eval/testdata/trend`, so runs live under `results/`. Each run has `summary.json` and `selftest.json` written in the style of `testdata/verifylog/evals/results/`:

| Run | `selftest` content | Purpose |
|-----|--------------------|---------|
| `260701-100000-aaaaaaa` | legacy verdict (no `attempts_*`); `selftest.json` has 4 case results, 3 with `passed: true`; `complete: true` | rate recomputed from the agent file (75.0) |
| `260702-100000-bbbbbbb` | verdict with `attempts_total: 4`, `attempts_passed: 4`, `success_rate: 100`; `complete: true`; different `prompt_sha256` | rate read from the summary |
| `260703-100000-ccccccc` | verdict with `attempts_total: 1`, `attempts_passed: 1`; `complete: false`, `case_filter: "stub-basic"` | the incomplete run, marked `INCOMPLETE` |
| `260704-100000-ddddddd` | verdict with `attempts_total: 4`, `attempts_passed: 3`, `success_rate: 75`; `complete: true` | later run, different models |
| `260705-100000-eeeeeee` | only another agent (`other`) | proves runs without the agent are skipped |

The fixture has four `selftest` runs; the AC 3 command prints four lines, the third marked `INCOMPLETE`. `trend nope` finds none and fails (AC 5). Edge cases that need an odd directory (hash-only run name, missing results directory, non-directory entry, run with no cases, a path-like agent name) are tested with `t.TempDir()` fixtures in `trend_test.go`, so the committed fixture stays small and readable.

## 3. Relevant Files

Modify:

- `internal/eval/verdict.go`: three `AgentVerdict` fields, `attemptAccounting`, fill in `agentVerdict`, new line in `printAgentVerdict`.
- `docs/evaluation.md`: verdict docs, line examples, field tables, new trend section after "Comparing Runs" (Task 4).

Create:

- `internal/eval/trend.go`, `internal/eval/trend_test.go`
- `internal/eval/attempts_test.go` (verdict fields, line format, end-to-end stub run)
- `internal/eval/testdata/trend/results/**` (section 2.5)
- `cmd/kairon/cmd/eval_trend.go`, `cmd/kairon/cmd/eval_trend_test.go`

Update test expectations that assert the exact PASS/FAIL line (they change because of section 2.2):

- `internal/eval/exit_status_test.go` (lines ~55, 56, 188, 209)
- `internal/eval/repeat_test.go` (lines ~274-280)
- `internal/eval/criterion_floor_test.go` (lines ~162, 165)
- `internal/eval/criterion_totals_test.go` (lines ~151, 162)
- `internal/eval/selftest_floor_test.go` (line ~97)
- `internal/eval/verdict_test.go` (the required-key list near line 282: add the three keys)

Reference only (reused, not changed): `internal/eval/verifylog.go` (`loadRunRecord`, `validatePathElement`, `firstNonEmpty`), `internal/eval/runner.go` (`splitRunDirName`, `timestampLayout` in `util.go`), `cmd/kairon/cmd/eval_verify_log.go` (command pattern).

Not touched: `.kairon/specs/**` (historical, including the maturity-model files cited by the issue), `CHANGELOG.md`, `.kairon/evals/results/`. Template sync: no file in the sync table is modified, so `cmd/kairon/templates/` is unchanged; `task sync:check` is still run as a guard.

## 4. Compatibility

- **Old summaries and agent files.** `AgentVerdict` gains fields with zero values on read, so every legacy `summary.json` still decodes. `eval diff`, `--resume` and `verify-log` read verdicts the same way as before and ignore the new keys.
- **Old runs in `trend`.** Handled by the success-rate fallback and by `null` / `""` for unrecorded facts. A run with no `complete` field is listed with `completeness-unknown`.
- **PASS/FAIL line.** Anything that matches the full line text will see the new `, success rate ...` text. `grep '^PASS \|^FAIL '` is unaffected. Repo tests and docs listed in section 3 are updated in the same PR.
- **Stage 4 contract.** `attempts_total`, `attempts_passed` and `success_rate` keep these names, types and units. A test pins the keys and the percent unit.

## 5. Step-by-Step Task Breakdown

Order: Task 1 first (the trend reads the new verdict field). Task 2 and Task 3 follow in sequence. Task 4 (docs) needs the final output formats. Task 5 validates. TDD per `AGENTS.md`: each builder task writes its failing tests first, runs them and confirms they fail for the expected reason, then implements, and commits tests and implementation together.

### Task 1: `verdict-attempts` - verdict fields and PASS/FAIL line

**Acceptance Criteria**:
- `AgentVerdict` has `AttemptsTotal int` (`attempts_total`), `AttemptsPassed int` (`attempts_passed`) and `SuccessRate float64` (`success_rate`), none `omitempty`.
- `attemptAccounting` counts every `CaseResult` (repeats included) and counts passed ones via `casePassed`. Unit tests cover: one result per case; repeats (2 cases x 3 runs, with mixed pass/fail gives `attempts_total` 6 and the right `attempts_passed`, while `cases_total` stays 2); a legacy result with `Passed == nil`; a result with no output counted as failed; zero results gives `0, 0, 0`.
- The case where the pooled score passes but attempts fail is pinned: a verdict with `passed: true` and `success_rate` < 100.
- A raw-JSON test confirms the three keys are written, including `"success_rate":0`.
- `printAgentVerdict` prints `, success rate X.X% (P/T attempts)` right after `cases failed`, before the floor suffix, the repeat suffix and the cost-incomplete marker. Floor, repeat, cost-incomplete and per-criterion lines are otherwise unchanged.
- An end-to-end test (modelled on `exit_status_test.go`, stub backend, a temp copy of `testdata/evals`) runs `selftest-fail` with `Repeat: 2` and asserts in `summary.json` that `attempts_total` equals the number of case results in `selftest-fail.json`, `attempts_passed` equals the number with `passed: true`, that the printed FAIL line shows the matching `success rate`, and that `attempts_total` is greater than `cases_total`.
- Existing exact-line assertions listed in section 3 are updated to the new format; no other behaviour changes.

**Dependencies**: None.

### Task 2: `trend-core` - `internal/eval/trend.go`, fixture and tests

**Acceptance Criteria**:
- `LoadTrend`, `Trend`, `TrendOptions` and `TrendRow` exist as in section 2.3 and use `loadRunRecord` for score, models, prompt hash and `complete`.
- Fixture `internal/eval/testdata/trend/results/` matches section 2.5.
- Tests (written first) cover: the fixture lists four rows in date order with the third `complete == false`; the other agent's run is excluded; the recomputed rate (75) for the legacy run and the recorded rates for the others; `prompt_sha256` is exactly 8 characters; text output is one line per run and the incomplete run's line contains `INCOMPLETE`; JSON output decodes to an array of four objects with the keys in section 2.3 and `null` for an unrecorded `success_rate` / `complete`; ordering with a hash-only run name (listed first, empty date) using a temp dir; non-directory entries and runs with no data for the agent are skipped; a missing results directory, an unknown agent and an agent name containing `/` or `..` return errors, and the unknown-agent message contains the agent name; nothing is written to `w` on error.
- `Trend` does not read or modify the package `cfg`.

**Dependencies**: `verdict-attempts`.

### Task 3: `trend-cmd` - `kairon eval trend`

**Acceptance Criteria**:
- `cmd/kairon/cmd/eval_trend.go` registers `trend <agent>` on `evalCmd`, with a local `--json` flag, inheriting `--evals-dir`, exactly one argument, `SilenceUsage = true` before the call, output on `cmd.OutOrStdout()`.
- Tests (written first, in the style of `eval_verify_log_test.go`) check: the command is registered and `--evals-dir` is inherited, not redefined; `trend selftest` against `../../../internal/eval/testdata/trend` prints four lines in order with one `INCOMPLETE`; `--json` output is a JSON array of four objects; `trend nope` returns an error naming `nope`; flag variables are restored after each test.
- `go run ./cmd/kairon eval trend nope --evals-dir internal/eval/testdata/trend; echo $?` prints a non-zero status.

**Dependencies**: `trend-core`.

### Task 4: `docs-evaluation` - `docs/evaluation.md`

**Acceptance Criteria**:
- "Agent verdict" gets a bullet defining `attempts_total`, `attempts_passed` and `success_rate`, and states how they differ from `cases_total` / `cases_failed` (repeats are separate attempts, not pooled) and that the floor and threshold do not change them.
- All PASS/FAIL line examples now show the success-rate text (the examples in the Output section, the `--repeat` and floor examples, the cost-incomplete example, the self-test lines near "Self-Test", and the prose that quotes the format string). The format string paragraph describes the new position.
- The `agent_verdicts.<agent>` field table and the percent-versus-fraction table list the three fields (`success_rate` is percent 0-100; the counts are integers). The `summary.json` example includes them.
- A new section "Attempt trend (`kairon eval trend`)" after "Comparing Runs" documents: synopsis and flags, which runs are listed, ordering and where the date comes from, the columns and the exact text format, the JSON array and its keys (including that `prompt_sha256` is the 8-character prefix and that unknown values are `null` / `""`), the success-rate fallback for older runs, what `INCOMPLETE` and `completeness-unknown` mean, the error for an agent with no results, and the exit status. It states that `trend` does not call a model or a container, and that production workflow metrics are out of scope.
- No file under `.kairon/specs/` or `CHANGELOG.md` is edited.

**Dependencies**: `verdict-attempts`, `trend-core`, `trend-cmd`.

### Task 5: `validate-all` - verification

**Acceptance Criteria**:
- Every command in the issue's acceptance criteria 1-5 behaves as described (run against a temp copy of `testdata/evals` for AC 1 so no files are written under the repo).
- Full unit tests, vet, gofmt and template-sync checks pass.
- `git status` shows no changes under `.kairon/specs/` other than this spec, no `CHANGELOG.md` change and no `cmd/kairon/templates/` change.

**Dependencies**: `docs-evaluation`.

## 6. Team Orchestration

- Sequential chain: `verdict-attempts` -> `trend-core` -> `trend-cmd` -> `docs-evaluation` -> `validate-all`.
- The chain is deliberate. `trend-core` consumes `AgentVerdict.SuccessRate`, `trend-cmd` consumes `eval.Trend`, and the docs need final output formats. The tasks are not large, so there is little to gain from parallelism, and a single chain avoids merge conflicts in `verdict.go` and the shared test helpers.
- One exception: `docs-evaluation` may start its verdict-related edits as soon as `verdict-attempts` is done, but it is listed after the command task so the trend section matches the final output.
- Commits: each builder task commits its tests with its implementation (AGENTS.md). Commit messages name the test seen failing first (e.g. `TestAttemptAccountingRepeats`, `TestTrendFixtureOrder`).
- Validator is read-only; it reports and does not fix.

## 7. Validation Commands

```bash
go build ./...
go vet ./internal/eval/ ./cmd/...
test -z "$(gofmt -l .)"
go test ./internal/eval/ ./cmd/kairon/cmd/ -count=1
task sync:check

# AC 1 and AC 2 (use a temp copy so results are not written into the repo)
rm -rf /tmp/ev347 && cp -r internal/eval/testdata/evals /tmp/ev347
go run ./cmd/kairon eval --backend stub --evals-dir /tmp/ev347 selftest-fail --repeat 2 | grep '^FAIL '
python3 - <<'PY'
import glob, json
d = glob.glob('/tmp/ev347/results/*')[0]
v = json.load(open(d + '/summary.json'))['agent_verdicts']['selftest-fail']
cases = json.load(open(d + '/selftest-fail.json'))['cases']
assert v['attempts_total'] == len(cases), (v['attempts_total'], len(cases))
assert v['attempts_passed'] == sum(1 for c in cases if c.get('passed')), v['attempts_passed']
assert abs(v['success_rate'] - v['attempts_passed'] * 100 / v['attempts_total']) < 1e-9
print('ok', v['attempts_passed'], '/', v['attempts_total'])
PY

# AC 3, AC 4, AC 5
go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend
go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend --json | python3 -c 'import json,sys; r=json.load(sys.stdin); assert isinstance(r,list) and len(r)==4; print("ok", len(r))'
go run ./cmd/kairon eval trend nope --evals-dir internal/eval/testdata/trend; echo $?
```

## 8. Risks

- **Existing line assertions.** Inline placement changes about ten exact-line assertions and the docs. Mitigation: they are enumerated in section 3 and updated in Task 1/4; the full `go test` run catches any missed one.
- **Stage 4 consumers.** The field names are contractual. They are pinned by a raw-JSON test.
- **Date source.** The date is the run directory name, not the time the run finished. For `kairon eval` runs the name is created at run start, so an interrupted or resumed run shows its start time. This is documented rather than worked around.
- **Hash-only run names.** Old-format runs have no date and list first. A temp-dir test covers this. A dated name without a hash (e.g. the real run `260624-215305`) parses through `splitRunDirName` and needs no special case.

## 9. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "verdict-attempts"
    agent: "builder"
    description: "Add attempts_total, attempts_passed and success_rate to AgentVerdict (internal/eval/verdict.go) via a new attemptAccounting helper that counts every CaseResult (repeats included) and passed ones via casePassed; always written, no omitempty. Print ', success rate X.X% (P/T attempts)' right after 'cases failed' in printAgentVerdict, before the floor, repeat and cost-incomplete suffixes. Write failing tests first (attempts_test.go), including an end-to-end stub run of selftest-fail --repeat 2, and update the existing exact-line assertions in exit_status_test.go, repeat_test.go, criterion_floor_test.go, criterion_totals_test.go, selftest_floor_test.go and the key list in verdict_test.go."
    dependencies: []
    acceptance_criteria:
      - "AgentVerdict serialises attempts_total, attempts_passed and success_rate (percent 0-100) with no omitempty; a raw-JSON test shows all three keys present, including success_rate 0"
      - "attemptAccounting returns the number of case results and the number passing casePassed; repeats count as separate attempts (2 cases x 3 runs gives attempts_total 6 while cases_total stays 2); a legacy result with Passed nil is recomputed; a result with no output is a failed attempt; zero results gives 0, 0 and success_rate 0"
      - "A verdict that passes (score >= threshold) can have success_rate below 100, and the floor and threshold do not change the attempt counts"
      - "printAgentVerdict output for PASS and FAIL lines contains 'success rate X.X% (P/T attempts)' immediately after 'cases failed' and before any floor, repeat or cost-incomplete suffix; the line still starts with PASS or FAIL"
      - "End-to-end test: eval of selftest-fail with the stub backend and Repeat 2 writes a summary whose attempts_total equals the number of case results in selftest-fail.json, attempts_passed equals the number with passed true, attempts_total is greater than cases_total, and the printed FAIL line shows the matching success rate"
      - "All previously existing tests pass after their exact-line expectations are updated; the failing tests were observed failing for the expected reason before the implementation was written"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -count=1"
      - "go vet ./internal/eval/"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "trend-core"
    agent: "builder"
    description: "Implement internal/eval/trend.go: TrendOptions, TrendRow, LoadTrend and Trend. List every directory under <evals-dir>/results with a score for the agent (reuse loadRunRecord), oldest first by the date parsed from the run name (splitRunDirName; undated names first, then by name), with run, date (RFC 3339 UTC), score, success_rate (summary verdict when attempts_total > 0, else recomputed from <agent>.json, else null), agent_model, judge_model, 8-character prompt_sha256 and complete (null when not recorded). Text output is one line per run, no header, ending in complete, INCOMPLETE or completeness-unknown; --json output is an indented JSON array of the same rows. Return an error naming the agent when no run has a result, and for a non-path-element agent name. Create the fixture internal/eval/testdata/trend/results (4 selftest runs with the third incomplete, plus one run of another agent) as specified. Write failing tests first (trend_test.go)."
    dependencies: ["verdict-attempts"]
    acceptance_criteria:
      - "Fixture internal/eval/testdata/trend/results holds four selftest runs (260701..260704) and one run of another agent; the third selftest run records complete false and case_filter stub-basic"
      - "LoadTrend on the fixture returns four rows in date order; the other agent's run is excluded; the incomplete run has Complete pointing to false"
      - "The legacy run's success rate (75) is recomputed from its agent file; the other runs' rates come from the summary verdict; a run with no usable attempts has a nil success rate"
      - "prompt_sha256 is exactly the first 8 characters; unrecorded strings are empty and unrecorded numbers/booleans are null in JSON"
      - "Text output has exactly one line per run and the incomplete run's line contains INCOMPLETE; JSON output decodes to an array of four objects with keys run, date, score, success_rate, agent_model, judge_model, prompt_sha256, complete"
      - "Temp-dir tests cover: hash-only run name sorts first with an empty date, non-directory entries and runs without the agent are skipped, a missing results directory returns an error"
      - "An unknown agent returns an error whose message contains the agent name and nothing is written to the writer; an agent name containing '/' or '..' is rejected"
      - "Trend does not read or modify the package-level cfg; the failing tests were observed failing for the expected reason before the implementation was written"
    validation_commands:
      - "go build ./internal/eval/"
      - "go test ./internal/eval/ -run Trend -count=1"
      - "go vet ./internal/eval/"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "trend-cmd"
    agent: "builder"
    description: "Add cmd/kairon/cmd/eval_trend.go: the 'trend <agent>' subcommand of evalCmd (cobra.ExactArgs(1), local --json flag, inherited --evals-dir, SilenceUsage set before the call, output to cmd.OutOrStdout()) calling eval.Trend. Write failing tests first (eval_trend_test.go) in the style of eval_verify_log_test.go, using ../../../internal/eval/testdata/trend."
    dependencies: ["trend-core"]
    acceptance_criteria:
      - "trend is registered on evalCmd; --json is a local flag; --evals-dir is inherited and not redefined"
      - "Running trend selftest against the fixture prints four lines in date order with exactly one INCOMPLETE"
      - "Running with --json prints a JSON array of four objects"
      - "Running trend nope returns an error naming nope; flag variables are restored after each test"
      - "go run ./cmd/kairon eval trend nope --evals-dir internal/eval/testdata/trend exits non-zero"
    validation_commands:
      - "go build ./..."
      - "go test ./cmd/kairon/cmd/ -run Trend -count=1"
      - "go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend"
      - "go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend --json"
      - "! go run ./cmd/kairon eval trend nope --evals-dir internal/eval/testdata/trend"
      - "test -z \"$(gofmt -l cmd)\""

  - id: "docs-evaluation"
    agent: "builder"
    description: "Update docs/evaluation.md: define attempts_total, attempts_passed and success_rate in the Agent verdict section (and how they differ from cases_total/cases_failed); update every PASS/FAIL line example and the format-string prose to the new 'success rate X.X% (P/T attempts)' text; add the three fields to the agent_verdicts field table, the percent-versus-fraction table and the summary.json example; add an 'Attempt trend (kairon eval trend)' section after 'Comparing Runs' covering flags, listed runs, ordering and date source, text and JSON formats, success-rate fallback for older runs, INCOMPLETE and completeness-unknown, the no-results error and exit status. Do not edit .kairon/specs/ or CHANGELOG.md."
    dependencies: ["verdict-attempts", "trend-core", "trend-cmd"]
    acceptance_criteria:
      - "No PASS/FAIL example line in docs/evaluation.md lacks the success rate text: grep for 'cases failed' shows each line with ', success rate'"
      - "The three verdict fields appear in the Agent verdict prose, the agent_verdicts field table, the percent-versus-fraction table and the summary.json example, with success_rate documented as percent 0-100"
      - "The new trend section documents the exact text format, the JSON keys (prompt_sha256 is the 8-character prefix; unknown values are null or empty), the ordering rule, the fallback for older runs, INCOMPLETE, and the error and non-zero exit for an agent with no results"
      - "The documented example output matches what go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend prints"
      - "git diff shows no change under .kairon/specs/ and no change to CHANGELOG.md"
    validation_commands:
      - "grep -n 'cases failed' docs/evaluation.md"
      - "grep -n 'attempts_total' docs/evaluation.md"
      - "grep -n 'eval trend' docs/evaluation.md"
      - "go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend"
      - "test -z \"$(git diff --name-only -- CHANGELOG.md)\""

  - id: "validate-all"
    agent: "validator"
    description: "Verify the complete change against the issue's acceptance criteria 1-5: run the AC 1 command against a temp copy of internal/eval/testdata/evals and check attempts_total, attempts_passed and success_rate against selftest-fail.json and the printed FAIL line; run the three trend commands; run the full test suite, vet, gofmt and task sync:check; confirm no historical artifact, CHANGELOG.md or template file changed."
    dependencies: ["docs-evaluation"]
    acceptance_criteria:
      - "AC 1: summary attempts_total equals the number of case results and attempts_passed equals the number with passed true, and success_rate equals attempts_passed*100/attempts_total"
      - "AC 2: the FAIL line of the AC 1 run shows the success rate"
      - "AC 3: eval trend selftest on the fixture prints one line per fixture run of the agent, oldest first, with run name, date, score, success rate, agent model, judge model, 8-character prompt hash and completeness, and the incomplete run marked"
      - "AC 4: --json prints a JSON array with one object per run"
      - "AC 5: eval trend nope exits non-zero with a message naming nope"
      - "go test ./internal/eval/ ./cmd/kairon/cmd/ passes; go vet, gofmt and task sync:check pass"
      - "No changes under .kairon/specs/ other than issue-347-attempt-success-rate-and-trend.md, none to CHANGELOG.md and none under cmd/kairon/templates/"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ ./cmd/kairon/cmd/ -count=1"
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
      - "task sync:check"
      - "go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend"
      - "go run ./cmd/kairon eval trend selftest --evals-dir internal/eval/testdata/trend --json"
      - "! go run ./cmd/kairon eval trend nope --evals-dir internal/eval/testdata/trend"
```
