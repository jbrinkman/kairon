# Design Spec: Evals — iteration log format and a verifier for logged scores

Closes #320

## 1. Solution Approach

Stage 3 of the maturity model needs at least two documented prompt iterations (baseline, hypothesis, change, measured results, reasoning) in chronological cause-and-effect order. Agents will write these logs, so every number in a log must be checkable against committed eval results. This issue delivers three things, all in one PR:

1. A log **format**, defined in `.kairon/iterations/README.md` with a complete example: `.kairon/iterations/<agent>/iteration-NN.md` (NN from `00`), YAML front-matter plus five fixed body headings.
2. A **verifier**, `kairon eval verify-log <agent>`, implemented in `internal/eval` as a pure, read-only function (no use of the package-global `cfg`, so it never calls `configure`), wired into `cmd/kairon/cmd/eval.go` as a subcommand next to `diff`.
3. **Fixtures and tests** under `internal/eval/testdata/verifylog/`, written test-first per AGENTS.md.

### How the existing results are stored (investigated)

- A run is the directory `<evals-dir>/results/<YYMMDD-HHMMSS>-<git-hash>/` holding `summary.json`, `<agent>.json` and sometimes `performance.json` (`internal/eval/runner.go`, `types.go`).
- The **recorded agent score** is `summary.json` → `agent_scores[<agent>]`, a fraction (`0.875`) equal to `score/max` over all cases with skipped criteria excluded (`agentScoreTotals`, `runner.go`). It is absent when the agent had no scored criteria. The log's percent claim is `agent_scores × 100`.
- **`prompt_sha256`** (from #316, `internal/eval/provenance.go`) lives in `summary.json` → `agents[<agent>].prompt_sha256`, and also in `<agent>.json` → `prompt_sha256`. Older or single-case runs may carry only one of them. `loadRunInfo(summary, dir)` in `diff_provenance.go` already merges both sources; the verifier reuses it, together with `loadSummary` / `loadAgentResult` from `diff.go`, instead of re-parsing.
- `resolveRunDirectory` (`diff.go`) accepts a hash-only name and picks the latest match, and it reads the global `cfg`. The verifier does **not** reuse it: a log must pin an exact run, so `baseline_run` / `result_run` are the **exact directory names** under `<evals-dir>/results/`. A name must be a single path element (no `/`, `\`, or `..`), which also blocks path traversal from agent-written input.
- `.kairon/evals/results/` is committed and not git-ignored (checked with `git check-ignore`); only `internal/eval/testdata/evals/results/` is ignored. The new fixture results live under `internal/eval/testdata/verifylog/evals/results/`, which is not ignored.
- `.kairon/iterations/` is not in the template-sync mappings (`builder-conventions`, `task sync:check`), so nothing is copied into `cmd/kairon/templates/`.

### Log format (contract relied on by later issues)

Path: `.kairon/iterations/<agent>/iteration-NN.md`. `NN` is a zero-padded number with at least two digits, starting at `00`.

```markdown
---
iteration: 0
date: 2026-07-01
change_type: prompt
baseline_run: 260701-100000-aaaaaaa
result_run: 260702-100000-bbbbbbb
baseline_score: 60.0
result_score: 72.5
---

## Baseline
## Hypothesis
## Change
## Results
## Reasoning
```

Field rules:

| Field | Rule |
|-------|------|
| `iteration` | integer; must equal `NN` in the file name |
| `date` | `YYYY-MM-DD` |
| `change_type` | `prompt` or `eval` (`eval` = rubric/case/fixture change; the agent prompt is not expected to change) |
| `baseline_run`, `result_run` | exact run directory names under `<evals-dir>/results/` |
| `baseline_score`, `result_score` | percent, **written with exactly one decimal** (`60.0`, `87.5`, `100.0`); range 0.0–100.0 |

Body: the five level-2 headings `## Baseline`, `## Hypothesis`, `## Change`, `## Results`, `## Reasoning`, each present exactly once, in that order, each with non-empty content. Unknown extra front-matter keys are ignored (a typo in a required key is already caught as a missing required field), so later issues can add fields.

### Verifier rules

Each violation is reported as `<file>: [<rule>] <message>`; all violations are collected and reported together (sorted by file then rule). Rule ids are a stable contract (later issues and docs refer to them):

| Rule id | Fails when | Fixture set |
|---------|-----------|-------------|
| `front-matter` | missing/unterminated/invalid YAML; a required key missing or wrong type; `date` not `YYYY-MM-DD`; `change_type` not `prompt`/`eval`; a score not in `N.N` form or outside 0–100; a run name that is empty or not a single path element | unit tests (temp dir) |
| `headings` | a required heading missing, duplicated, out of order, or with empty content | unit tests (temp dir) |
| `numbering` | iteration numbers are not contiguous from `00` (a gap, or the first is not `00`); `iteration:` differs from the file name; a file named `iteration-*` does not match `iteration-NN.md` | `gap` |
| `chain` | iteration N's `baseline_run` is not iteration N−1's `result_run` (N ≥ 1) | `broken-chain` |
| `date-order` | an iteration's `date` is earlier than the previous iteration's | unit tests (temp dir) |
| `run-missing` | `baseline_run` or `result_run` has no `<evals-dir>/results/<run>/summary.json` | `missing-run` |
| `run-no-score` | the run exists but `summary.json` has no `agent_scores[<agent>]` | unit tests (temp dir) |
| `score-mismatch` | a claimed score differs from `agent_scores[<agent>] × 100` by more than 0.1 (tolerance check `|diff| ≤ 0.1 + 1e-9` so that a 0.1 difference passes despite float error) | `score-mismatch` |
| `prompt-unchanged` | `change_type: prompt` and both runs record the same `prompt_sha256` for the agent | `no-prompt-change` |
| `prompt-unrecorded` | `change_type: prompt` and either run records no `prompt_sha256` for the agent (the claim cannot be checked, so it fails) | unit tests (temp dir) |
| `min-iterations` | fewer iterations than `--min-iterations` (default 2); also fires, with the directory as the file, when the agent has no iterations directory | `valid` with `--min-iterations 5` |

`change_type: eval` iterations are exempt from `prompt-unchanged` / `prompt-unrecorded` (changing a rubric need not change the prompt hash).

Rules that need a run (`run-missing`, `run-no-score`, `score-mismatch`, `prompt-*`) are evaluated per iteration after that iteration's front-matter parsed; an iteration whose front-matter is invalid skips them and is reported once under `front-matter`. A usage/IO problem (unreadable directory, invalid agent name, negative `--min-iterations`) is a returned `error`, not a violation.

### Go API (`internal/eval/verifylog.go`)

```go
const DefaultIterationsDir = ".kairon/iterations"
const DefaultMinIterations = 2

type VerifyLogOptions struct {
    Agent         string // required; a single path element
    IterationsDir string // "" => DefaultIterationsDir; logs at <dir>/<agent>/iteration-NN.md
    EvalsDir      string // "" => defaultEvalsDir; runs at <dir>/results/<run>/
    MinIterations int    // checked as given (callers pass DefaultMinIterations); must be >= 0
}

type Violation struct {
    File    string // path as built from IterationsDir (or the agent directory)
    Rule    string // one of the rule ids above
    Message string
}

func (v Violation) String() string // "<file>: [<rule>] <message>"

// VerifyLog reads the agent's log set and checks it against the committed
// results. It returns every violation found (nil when valid) and an error only
// for usage/IO problems. It does not touch package-global cfg.
func VerifyLog(opts VerifyLogOptions) ([]Violation, error)
```

CLI (`cmd/kairon/cmd/eval.go`): `kairon eval verify-log <agent>` with `Args: cobra.ExactArgs(1)`, local flags `--iterations-dir` (default `.kairon/iterations`) and `--min-iterations` (default `2`); `--evals-dir` is the existing persistent flag of `evalCmd` and is inherited. On success it prints `✅ <agent>: N iteration(s) verified`. On violations it prints each `Violation.String()` line to stderr and returns an error `verify-log failed: N violation(s)`, so `main` exits 1. Do not add `SilenceUsage`/`SilenceErrors` changes to other commands.

### Fixtures (`internal/eval/testdata/verifylog/`)

Shared results (`evals/results/<run>/summary.json` plus `selftest.json`), agent `selftest`:

| Run dir | `agent_scores.selftest` | `prompt_sha256` | Notes |
|---------|------------------------|-----------------|-------|
| `260701-100000-aaaaaaa` | 0.600 | `aaaa…` (64 × `a`) | |
| `260702-100000-bbbbbbb` | 0.725 | `bbbb…` | sha only in `selftest.json` (summary has no `agents` block) — exercises the `loadRunInfo` fallback |
| `260703-100000-ccccccc` | 0.850 | `cccc…` | |
| `260704-100000-ddddddd` | 0.700 | `aaaa…` (same as run a) | used by `no-prompt-change` |
| `260705-100000-eeeeeee` | 0.800 | `cccc…` (same as run c) | used by the `eval`-type iteration in `valid` |

Log sets (each `<set>/selftest/iteration-NN.md`, with all five headings filled in):

| Set | Files | Expected result |
|-----|-------|-----------------|
| `valid` | 00: a→b (60.0→72.5, prompt); 01: b→c (72.5→85.0, prompt); 02: c→e (85.0→80.0, eval) | exit 0; with `--min-iterations 5` fails `min-iterations` |
| `missing-run` | like `valid` 00, 01, but 01's `result_run` names a non-existent run | only `run-missing` on `iteration-01.md` |
| `score-mismatch` | like `valid` 00, 01, but 00's `result_score` is `90.0` (recorded 72.5) | only `score-mismatch` on `iteration-00.md` |
| `gap` | 00: a→b; 02: b→c (no 01) | only `numbering` on `iteration-02.md` |
| `broken-chain` | 00: a→b; 01: baseline a (60.0)→c (85.0) | only `chain` on `iteration-01.md` |
| `no-prompt-change` | 00: a→d (60.0→70.0, prompt, same sha); 01: d→c (70.0→85.0, prompt) | only `prompt-unchanged` on `iteration-00.md` |

Each failing set trips exactly one rule, and the tests assert the exact `(file, rule)` set so a fixture can't pass by accident.

## 2. Relevant Files

Create:
- `.kairon/iterations/README.md` — format definition, field table, rule table, complete example, how to verify
- `internal/eval/verifylog.go` — `VerifyLog`, `Violation`, parsing/validation
- `internal/eval/verifylog_test.go` — fixture-set tests, temp-dir tests for the remaining rules, README example test
- `internal/eval/testdata/verifylog/evals/results/<5 run dirs>/{summary.json,selftest.json}`
- `internal/eval/testdata/verifylog/{valid,missing-run,score-mismatch,gap,broken-chain,no-prompt-change}/selftest/iteration-NN.md`

Modify:
- `cmd/kairon/cmd/eval.go` — `verifyLogCmd`, flags, registration
- `cmd/kairon/cmd/eval_test.go` — flag registration and fixture-set command tests
- `docs/evaluation.md` — new section "Iteration Logs and `verify-log`", plus a pointer in "Evaluation Workflow" and the Directory Structure/`eval` usage list

Read-only references: `internal/eval/diff.go` (`loadSummary`, `loadAgentResult`), `internal/eval/diff_provenance.go` (`loadRunInfo`), `internal/eval/provenance.go`, `internal/eval/types.go`, `AGENTS.md`, `.kairon/specs/maturity-model/stage-3.md`. Do not edit any other `.kairon/specs/` file.

## 3. Team Orchestration

- `verifylog-fixtures` has no dependencies and goes first (fixtures are plain data).
- `verifylog-core` needs the fixtures and follows TDD: write `verifylog_test.go` first, run it and confirm it fails because `VerifyLog` is undefined or returns nothing (not a typo), then implement. Test and implementation are one change.
- `verify-log-command` needs the API; `iterations-readme` needs the parser (its test parses the README example with the verifier's own front-matter/heading parser). These two are independent and can run in parallel.
- `docs-evaluation` needs the final flags and rule ids (from `verify-log-command` and `iterations-readme`).
- `validate-all` (validator, read-only) runs last.

All of it ships in one PR. Per AGENTS.md, the commit message must name the test seen failing first (e.g. `TestVerifyLogFixtureSets`). The README and docs are non-code and need no test first, but the README example is covered by a test anyway.

## 4. Step-by-Step Task Breakdown

### Task 1 — `verifylog-fixtures` (builder)
Create the results tree and the six log sets exactly as in §1 "Fixtures". `summary.json` follows the real shape (`git_hash`, `total_cost`, `agent_scores`, and for most runs an `agents.selftest` block with `agent_model`, `prompt_sha256`, `resources_present`). `selftest.json` can be minimal (`agent`, `git_hash`, `prompt_sha256`, `resources_present`, `cases: []`). Each iteration file has valid front-matter and all five headings with one or two sentences of text. Check that the files are not git-ignored (`git check-ignore` must print nothing).
Acceptance: all files present; `valid` and every failing set differ only in the intended way; JSON parses.

### Task 2 — `verifylog-core` (builder)
TDD. In `verifylog_test.go` write:
- `TestVerifyLogFixtureSets`: table over the six sets, asserting the exact `(file basename, rule)` pairs (empty for `valid`);
- `TestVerifyLogMinIterations`: `valid` with 5 → `min-iterations`; `valid` with 3 and with 2 → OK; missing agent dir → `min-iterations`;
- `TestVerifyLogScoreTolerance`: a 0.1 difference passes, 0.2 fails (temp dir copy);
- temp-dir tests for `front-matter` (each variant in the rule table, incl. `87.55`, `87`, `101.0`, bad date, bad `change_type`, run name `../x`), `headings` (missing, duplicate, out of order, empty), `date-order`, `run-no-score`, `prompt-unrecorded`, `eval`-type exemption, and `numbering` for an `iteration-1.md` file name and for a log set starting at `01`;
- usage errors: agent name containing a path separator, negative `MinIterations`.
Run them and confirm they fail for the right reason, then implement `verifylog.go` per §1 (use `gopkg.in/yaml.v3`, already a dependency; decode `date` as a string and score fields through a node-level check of the scalar text `^(100\.0|[0-9]{1,2}\.[0-9])$`; split front-matter on a first line `---` and the next line that is exactly `---`; headings are lines matching `^## (Baseline|Hypothesis|Change|Results|Reasoning)\s*$` outside fenced code blocks). Reuse `loadSummary`, `loadAgentResult` and `loadRunInfo`; do not call `configure`/`evalsPath` and do not modify `cfg`.
Acceptance: `go test ./internal/eval -run VerifyLog -count=1` passes; `go vet` clean; no changes to existing eval behavior.

### Task 3 — `verify-log-command` (builder)
Add `verifyLogCmd` and the `--iterations-dir` / `--min-iterations` flags in `cmd/kairon/cmd/eval.go`, register under `evalCmd`. Test first in `eval_test.go`: flags exist with the right defaults (`.kairon/iterations`, `2`), `--evals-dir` is inherited, and running the command against the repo fixtures (paths relative to the test file as in `evalSelfTestProject`) returns nil for `valid` and an error for each failing set whose combined stderr output contains the file name and the rule id (capture via `cmd.SetErr`/a buffer, not `os.Stderr`). Restore flag globals in cleanup.
Acceptance: the issue's AC2–AC6 commands behave as specified when run through `go run`.

### Task 4 — `iterations-readme` (builder)
Write `.kairon/iterations/README.md`: purpose; layout; front-matter field table; heading table with one line on what each section must contain (Baseline = the numbers being improved, taken from `baseline_run`; Hypothesis = falsifiable prediction; Change = the exact diff/commit; Results = measured numbers from `result_run`, per criterion if useful; Reasoning = why the result confirms or refutes the hypothesis and what comes next); rule table (ids from §1); how to run `kairon eval verify-log <agent>` with all flags; a note that run names must be exact directory names of committed results under `.kairon/evals/results/`; and one **complete example** iteration file in a four-backtick ````markdown fence. Add `TestIterationsReadmeExampleParses` in `verifylog_test.go` that extracts that example (first `markdown` fence starting with `---`), writes it as `iteration-00.md` into a temp dir and asserts that it passes `front-matter` and `headings` checks (the run names in the example are illustrative, so run-dependent rules are not asserted).
Acceptance: README contains the example; the test passes.

### Task 5 — `docs-evaluation` (builder)
In `docs/evaluation.md` add a section "Iteration Logs and `verify-log`" (after "Evaluation Workflow"): format summary linking to `.kairon/iterations/README.md`, the command and flags, the rule table, how scores map (`agent_scores × 100`, tolerance 0.1), and the `prompt_sha256` check tied to the "Model Pinning and Run Provenance" section. In "Evaluation Workflow", add a short step "record the iteration" pointing to it. Keep the existing text accurate (do not describe trend/report commands; they are out of scope).
Acceptance: section present; command and flag names match the implementation.

### Task 6 — `validate-all` (validator)
Verify each issue acceptance criterion against the working tree: AC1 by reading the README; AC2–AC6 by running the commands below and checking the exit codes and the file/rule in the output; run `task lint`, `task fmt:check`, `task sync:check`, and the Go tests for the touched packages. Confirm only the intended paths changed and no other spec was edited.

## 5. Validation Commands

```bash
# AC2: valid set exits 0
go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals

# AC3-AC5: each failing set exits non-zero and names the file and the rule
! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/missing-run --evals-dir internal/eval/testdata/verifylog/evals      # iteration-01.md [run-missing]
! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/score-mismatch --evals-dir internal/eval/testdata/verifylog/evals   # iteration-00.md [score-mismatch]
! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/gap --evals-dir internal/eval/testdata/verifylog/evals              # iteration-02.md [numbering]
! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/broken-chain --evals-dir internal/eval/testdata/verifylog/evals     # iteration-01.md [chain]
! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/no-prompt-change --evals-dir internal/eval/testdata/verifylog/evals # iteration-00.md [prompt-unchanged]

# AC6
! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals --min-iterations 5

go test ./internal/eval ./cmd/kairon/cmd -run 'VerifyLog|IterationsReadme|EvalFlags' -count=1
go test ./internal/eval/... ./cmd/... -count=1
task lint && task fmt:check && task sync:check
```

The Go tests assert the file and rule names for the failing sets (the shell commands above only assert the exit code), so the test run is the authoritative check for "naming the file and the rule".

## 6. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "verifylog-fixtures"
    agent: "builder"
    description: "Create the verify-log fixtures under internal/eval/testdata/verifylog/: the shared evals/results tree (5 runs for agent selftest, each with summary.json and selftest.json) and the six log sets valid, missing-run, score-mismatch, gap, broken-chain, no-prompt-change as laid out in the spec's Fixtures section."
    dependencies: []
    acceptance_criteria:
      - "internal/eval/testdata/verifylog/evals/results/ contains run dirs 260701-100000-aaaaaaa, 260702-100000-bbbbbbb, 260703-100000-ccccccc, 260704-100000-ddddddd and 260705-100000-eeeeeee, each with summary.json (agent_scores.selftest of 0.600, 0.725, 0.850, 0.700, 0.800) and selftest.json"
      - "Run d records the same prompt_sha256 as run a and run e the same as run c; run b records its prompt_sha256 only in selftest.json"
      - "Each of the six sets has selftest/iteration-NN.md files with valid front-matter (iteration, date, change_type, baseline_run, result_run, baseline_score, result_score) and the five headings Baseline, Hypothesis, Change, Results, Reasoning with non-empty content"
      - "valid has iterations 00 (a to b, prompt), 01 (b to c, prompt), 02 (c to e, eval); each failing set deviates from valid in exactly the one way described in the spec"
      - "git check-ignore prints nothing for any new fixture path"
    validation_commands:
      - "for f in internal/eval/testdata/verifylog/evals/results/*/*.json; do python3 -m json.tool \"$f\" > /dev/null || exit 1; done"
      - "test -f internal/eval/testdata/verifylog/valid/selftest/iteration-02.md"
      - "test ! -f internal/eval/testdata/verifylog/gap/selftest/iteration-01.md"
      - "test -z \"$(git check-ignore internal/eval/testdata/verifylog/evals/results/260701-100000-aaaaaaa/summary.json)\""

  - id: "verifylog-core"
    agent: "builder"
    description: "TDD: write internal/eval/verifylog_test.go first (fixture-set table, min-iterations, score tolerance, temp-dir tests for every remaining rule, usage errors), confirm it fails for the expected reason, then implement internal/eval/verifylog.go (VerifyLog, VerifyLogOptions, Violation, DefaultIterationsDir, DefaultMinIterations) implementing every rule in the spec's rule table without using package-global cfg."
    dependencies: ["verifylog-fixtures"]
    acceptance_criteria:
      - "Tests were written before the implementation and observed failing because VerifyLog was missing or returned no violations, not because of a compile typo or setup problem"
      - "TestVerifyLogFixtureSets asserts the exact (file, rule) pairs: valid has none; missing-run gives run-missing on iteration-01.md; score-mismatch gives score-mismatch on iteration-00.md; gap gives numbering on iteration-02.md; broken-chain gives chain on iteration-01.md; no-prompt-change gives prompt-unchanged on iteration-00.md"
      - "valid with MinIterations 5 yields a min-iterations violation, with 2 and 3 yields none; a missing agent directory yields min-iterations"
      - "A score difference of exactly 0.1 passes and 0.2 fails; scores not written as N.N (for example 87, 87.55) or outside 0-100 yield front-matter violations"
      - "front-matter, headings, date-order, run-no-score and prompt-unrecorded rules are each covered by a passing test; change_type eval is exempt from the prompt-sha rules"
      - "VerifyLog collects all violations sorted by file then rule, returns an error (not a violation) for an agent name with a path separator or a negative MinIterations, and rejects run names that are not a single path element"
      - "Implementation reuses loadSummary, loadAgentResult and loadRunInfo and does not call configure or modify cfg; existing eval tests still pass"
    validation_commands:
      - "go build ./internal/eval"
      - "go vet ./internal/eval"
      - "go test ./internal/eval -run 'VerifyLog' -count=1"
      - "go test ./internal/eval -count=1"

  - id: "verify-log-command"
    agent: "builder"
    description: "Add the 'kairon eval verify-log <agent>' subcommand in cmd/kairon/cmd/eval.go with local flags --iterations-dir (default .kairon/iterations) and --min-iterations (default 2), inheriting the persistent --evals-dir; write the cmd tests first in eval_test.go. Print '<file>: [<rule>] <message>' lines to stderr and return an error on violations so the process exits non-zero."
    dependencies: ["verifylog-core"]
    acceptance_criteria:
      - "Tests were written first and observed failing because the verify-log subcommand did not exist"
      - "verify-log requires exactly one argument; --iterations-dir defaults to .kairon/iterations and --min-iterations to 2; --evals-dir is inherited from evalCmd"
      - "Running against the valid fixture set returns nil; running against each of missing-run, score-mismatch, gap, broken-chain and no-prompt-change returns an error and the captured stderr output names the file (iteration-NN.md) and the rule id"
      - "valid with --min-iterations 5 returns an error mentioning min-iterations"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals exits 0 and each failing set exits non-zero"
      - "Existing eval and diff command behavior and tests are unchanged"
    validation_commands:
      - "go build ./cmd/kairon/..."
      - "go test ./cmd/kairon/cmd -run 'VerifyLog|EvalFlags' -count=1"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/missing-run --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/score-mismatch --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/gap --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/broken-chain --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/no-prompt-change --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals --min-iterations 5"

  - id: "iterations-readme"
    agent: "builder"
    description: "Write .kairon/iterations/README.md defining the iteration log format (path, front-matter fields, five body headings, verifier rule table, how to run kairon eval verify-log, exact-run-name requirement) with one complete example iteration file in a four-backtick markdown fence, and add TestIterationsReadmeExampleParses to internal/eval/verifylog_test.go which extracts the example and checks it with the verifier's own front-matter and heading parsing."
    dependencies: ["verifylog-core"]
    acceptance_criteria:
      - "README defines .kairon/iterations/<agent>/iteration-NN.md with NN starting at 00"
      - "README documents front-matter keys iteration, date, change_type (prompt or eval), baseline_run, result_run, baseline_score, result_score (percent, one decimal) and the headings Baseline, Hypothesis, Change, Results, Reasoning"
      - "README lists the verifier rule ids front-matter, headings, numbering, chain, date-order, run-missing, run-no-score, score-mismatch, prompt-unchanged, prompt-unrecorded, min-iterations and the flags --iterations-dir, --evals-dir, --min-iterations"
      - "README contains a complete, valid example iteration file, and TestIterationsReadmeExampleParses passes against it"
      - "README states that no real iteration log is shipped by this change"
    validation_commands:
      - "test -f .kairon/iterations/README.md"
      - "grep -q 'iteration-NN.md' .kairon/iterations/README.md"
      - "grep -q 'min-iterations' .kairon/iterations/README.md"
      - "go test ./internal/eval -run 'IterationsReadme' -count=1"

  - id: "docs-evaluation"
    agent: "builder"
    description: "Update docs/evaluation.md with an 'Iteration Logs and verify-log' section (format summary linking to .kairon/iterations/README.md, command and flags, rule table, score mapping agent_scores x 100 with 0.1 tolerance, prompt_sha256 check) and a short 'record the iteration' step in the Evaluation Workflow section."
    dependencies: ["verify-log-command", "iterations-readme"]
    acceptance_criteria:
      - "docs/evaluation.md has a section documenting kairon eval verify-log <agent>, --iterations-dir (default .kairon/iterations), --evals-dir and --min-iterations (default 2)"
      - "The section lists every rule id from the spec and explains that claimed scores are compared with summary.json agent_scores multiplied by 100 within 0.1, and that prompt iterations must change prompt_sha256"
      - "Evaluation Workflow mentions recording an iteration log and links to the new section"
      - "No trend or report command is described"
    validation_commands:
      - "grep -q 'verify-log' docs/evaluation.md"
      - "grep -q 'iterations-dir' docs/evaluation.md"
      - "grep -q 'prompt-unchanged' docs/evaluation.md"
      - "grep -q 'min-iterations' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #320 against the working tree: README content and example (AC1), verify-log exit codes and file/rule naming for the valid, missing-run, score-mismatch, gap, broken-chain and no-prompt-change sets and --min-iterations 5 (AC2-AC6), tests, lint, formatting and template sync; confirm no other spec or historical artifact was edited and no real iteration log was added."
    dependencies: ["docs-evaluation"]
    acceptance_criteria:
      - "AC1: .kairon/iterations/README.md defines the path, front-matter fields, headings and contains a complete example"
      - "AC2: verify-log on the valid set exits 0"
      - "AC3-AC5: missing-run, score-mismatch, gap, broken-chain and no-prompt-change each exit non-zero and the output names the file and the rule"
      - "AC6: valid with --min-iterations 5 exits non-zero"
      - "All Go tests in internal/eval and cmd/kairon/cmd pass; task lint, task fmt:check and task sync:check pass"
      - "git status shows changes only in .kairon/iterations/README.md, internal/eval, cmd/kairon/cmd, docs/evaluation.md and this spec; .kairon/iterations contains no iteration-NN.md files"
    validation_commands:
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/gap --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals --min-iterations 5"
      - "go test ./internal/eval/... ./cmd/... -count=1"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-320-iteration-log-verifier.md"
```
