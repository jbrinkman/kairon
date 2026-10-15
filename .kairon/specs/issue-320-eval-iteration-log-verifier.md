# Design Spec: Evals — iteration log format and a verifier for logged scores

Closes #320

Depends on #316 (per-agent pass threshold and `agent_verdicts` in `summary.json`, already merged: `internal/eval/verdict.go`).

## Problem Summary

Stage 3 of the maturity model requires at least two documented prompt iterations (baseline, hypothesis, change, measured results, reasoning) in chronological cause-and-effect order. Agents will write these logs, so every score a log claims must be checkable against eval results committed under `.kairon/evals/results/`. This issue delivers:

1. A log format, defined in `.kairon/iterations/README.md`.
2. `kairon eval verify-log <agent>`, which checks a log set against the recorded results.
3. Fixtures and tests for the verifier, plus documentation in `docs/evaluation.md`.

Out of scope: writing any real iteration log; trend/report commands.

## Findings From the Codebase

- Results live at `<evals-dir>/results/<run>/` (default evals dir `.kairon/evals`, constant `defaultEvalsDir` in `internal/eval/config.go`). A run directory holds `summary.json` and `<agent>.json`. Run names are `<yymmdd-hhmmss>-<git-hash>` (older runs may be hash only).
- The recorded agent score is, in order of preference:
  1. `summary.json` → `agent_verdicts.<agent>.score` (**percent**, 0–100; written since #316).
  2. `summary.json` → `agent_scores.<agent>` (**0–1 fraction**; multiply by 100). Older runs.
  3. Recompute from `<agent>.json` with the existing `agentVerdict(AgentResult).Score` (`internal/eval/verdict.go`). Runs with neither summary field.
  If none is available the run has no recorded score for that agent (violation, see `run-data`).
- `prompt_sha256` per agent is in `<agent>.json` (`AgentResult.PromptSHA256`) and in `summary.json` → `agents.<agent>.prompt_sha256`. Prefer `<agent>.json`, fall back to the summary. Both are produced by `resolveAgentProvenance` (`internal/eval/provenance.go`); a `--prompt-file` run hashes the candidate's bytes, so a changed candidate yields a different hash.
- `loadSummary` / `loadAgentResult` (`internal/eval/diff.go`) already decode these files and tolerate missing/old fields. `resolveRunDirectory` depends on the package-level `cfg`; the verifier must NOT use `cfg` (it takes explicit directories), so it uses `loadSummary`/`loadAgentResult` with explicit paths, not `resolveRunDirectory`.
- `cmd/kairon/cmd/eval.go` registers `evalCmd` with `--evals-dir` as a **persistent** flag (`evalEvalsDir`) and one subcommand, `diff`. `verify-log` is added the same way and inherits `--evals-dir` (default `""` meaning `.kairon/evals`).
- `gopkg.in/yaml.v3` is already a dependency (`go.mod`).
- `docs/evaluation.md` already says candidate prompts "by convention live under `.kairon/iterations/<agent>/`" (e.g. `v2.md`) and that Kairon does not scan that directory. `verify-log` only reads files matching `iteration-NN.md`; every other file in the agent directory (candidates, notes) is ignored. Keep that sentence true and extend it.
- `.gitignore` ignores `internal/eval/testdata/evals/results/` but NOT `internal/eval/testdata/verifylog/evals/results/` (checked with `git check-ignore`), so fixture results are committed normally.
- No template synchronization is needed: `.kairon/iterations/` and `internal/eval/testdata/` are not in the builder-conventions sync mappings, and no `.kiro/` file changes. `task sync:check` must still pass (run it).
- `AGENTS.md` TDD rules apply: failing test first, tests and implementation in the same commit, name the first-failing test in the commit message/PR.

## Solution Approach

### 1. Log format (defined in `.kairon/iterations/README.md`)

Path: `.kairon/iterations/<agent>/iteration-NN.md`, `NN` zero-padded to at least two digits, starting at `00`.

```markdown
---
iteration: 1
date: 2026-07-02
change_type: prompt
baseline_run: 260701-101500-a1b2c3d
result_run: 260702-093000-e4f5a6b
baseline_score: 72.5
result_score: 88.0
---

## Baseline
...
## Hypothesis
...
## Change
...
## Results
...
## Reasoning
...
```

Front-matter (YAML between the first two `---` lines, file must start with `---`). All seven keys are required; unknown extra keys are ignored.

| Key | Rule |
|-----|------|
| `iteration` | integer, equal to `NN` in the file name |
| `date` | `YYYY-MM-DD`, a real calendar date |
| `change_type` | `prompt` or `eval` |
| `baseline_run` | name of a directory under `<evals-dir>/results/` (single path element: no `/`, `\`, or `..`) |
| `result_run` | same |
| `baseline_score` | percent with exactly one decimal, `0.0`–`100.0` (regex `^(100\.0\|[0-9]{1,2}\.[0-9])$`) |
| `result_score` | same |

Scores are decoded as strings (not floats) so the one-decimal format can be enforced; yaml.v3 decodes an unquoted `85.0` into a `string` field using the scalar text. Builder: confirm with a test; if it does not, decode the field as `yaml.Node` and use `.Value`.

Body: level-2 headings `## Baseline`, `## Hypothesis`, `## Change`, `## Results`, `## Reasoning`. Each must appear exactly once, in that relative order, with non-blank content before the next `##` heading (headings inside fenced code blocks are ignored). Extra `##` headings are allowed.

`change_type: prompt` means the iteration changed the agent prompt (the two runs must record different `prompt_sha256`); `eval` means the rubric/cases/judge changed and the prompt may be identical.

The README must also say: results referenced by logs must be committed under `.kairon/evals/results/`; how to find `baseline_run`/`result_run` (directory names under results) and the score (`agent_verdicts.<agent>.score` in `summary.json`, the `PASS|FAIL <agent>: NN.N%` line); that candidate prompt files (`v2.md`, etc.) may live next to the logs and are ignored by the verifier; the verification command; and the rules table below.

### 2. Verifier: `internal/eval/verifylog.go`

Pure function, no use of package state `cfg`:

```go
const DefaultIterationsDir = ".kairon/iterations"
const DefaultMinIterations = 2
const scoreTolerance = 0.1 // percent points

type VerifyLogOptions struct {
    IterationsDir string // "" => DefaultIterationsDir
    EvalsDir      string // "" => defaultEvalsDir
    MinIterations int    // used as given (0 disables the minimum); negative is an error
}

type LogViolation struct {
    File    string // path as joined from IterationsDir (or the directory for set-level rules)
    Rule    string // one of the rule IDs below
    Message string
}

type VerifyLogError struct{ Violations []LogViolation }
func (e *VerifyLogError) Error() string // one line per violation: "<file>: <rule>: <message>"

// VerifyLog checks <iterations-dir>/<agent>/iteration-NN.md against
// <evals-dir>/results. It collects ALL violations (not fail-fast).
// On success prints "OK <agent>: N iterations verified" to w and returns nil.
func VerifyLog(w io.Writer, agent string, opts VerifyLogOptions) error
```

The default `MinIterations` of 2 lives in the CLI flag; the library uses the value it is given.

Agent name validation: non-empty, single path element (no `/`, `\`, `..`); else a plain usage-style error (not a log violation).

Algorithm:

1. List `<iterations-dir>/<agent>/`. If the directory is missing or contains no `iteration-*.md`, return violation rule `no-logs` (always, regardless of `--min-iterations`).
2. Select files matching `^iteration-(\d{2,})\.md$`. Files starting with `iteration-` and ending `.md` that do not match (e.g. `iteration-1.md`, `iteration-ab.md`) are violations of rule `sequence`. All other files are ignored.
3. Parse each file (front-matter, headings) and report `front-matter` / `headings` violations per file. A file that fails front-matter parsing is excluded from the cross-file checks that need its fields, but is still counted as an iteration for contiguity.
4. Cross-file checks, in iteration order:
   - `sequence`: the sorted NN values must be exactly `0..N-1` (gap, duplicate such as `00` and `000`, not starting at 00), and front-matter `iteration` must equal the file's NN.
   - `chain`: for iteration `k>0`, `baseline_run` must equal iteration `k-1`'s `result_run`.
   - `chronology`: `date` of iteration `k` must not be earlier than iteration `k-1`'s.
   - `run-exists`: both `baseline_run` and `result_run` must be directories under `<evals-dir>/results/`.
   - `run-data`: each referenced run must record a score for `<agent>` (precedence above). For `prompt` iterations it must also record a `prompt_sha256` for `<agent>` (fail closed: an unverifiable change is a violation).
   - `score-match`: `|claimed - recorded| > 0.1` (+1e-9 float slack so that exactly 0.1 passes) for both `baseline_score` vs `baseline_run` and `result_score` vs `result_run`. The message names the field, claimed value, recorded value and run.
   - `prompt-change`: for `change_type: prompt`, `baseline_run` and `result_run` must record different `prompt_sha256` for `<agent>`.
   - `min-iterations`: number of iteration files `< MinIterations` → violation on the agent directory.
5. If any violations, sort by (file, rule order) and return `*VerifyLogError`; otherwise print the OK line.

Rule IDs (stable, documented; relied on by P3, B3 and iteration issues): `no-logs`, `front-matter`, `headings`, `sequence`, `chain`, `chronology`, `run-exists`, `run-data`, `score-match`, `prompt-change`, `min-iterations`.

Each message names the file (path as given, i.e. `<iterations-dir>/<agent>/iteration-NN.md`) and the rule, satisfying "naming the file and the rule".

Design decisions to preserve:
- Runs are matched by exact directory name only. No hash-only resolution (unlike `eval diff`), because a log must pin one specific run.
- Run names containing a path separator or `..` are `front-matter` violations, so a log can never make the verifier read outside `<evals-dir>/results/`.
- A run's score is never rounded before comparison; the tolerance covers the one-decimal rounding of claimed scores.
- `chronology` and non-blank section content go beyond the six acceptance criteria but are required by the problem statement ("chronological cause-and-effect order", "recording … reasoning"); they are documented rules.

### 3. CLI: `cmd/kairon/cmd/eval_verify_log.go` (new)

```
kairon eval verify-log <agent> [--iterations-dir DIR] [--min-iterations N] [--evals-dir DIR]
```

- `verifyLogCmd`: `Use: "verify-log <agent>"`, `Args: cobra.ExactArgs(1)`, `cmd.SilenceUsage = true` once args are parsed (like `evalCmd`), calls `eval.VerifyLog(cmd.OutOrStdout(), args[0], eval.VerifyLogOptions{...})`.
- Flags: `--iterations-dir` (string, default `.kairon/iterations`), `--min-iterations` (int, default `2`, negative rejected). `--evals-dir` is the inherited persistent flag on `evalCmd` (`evalEvalsDir`).
- Registered with `evalCmd.AddCommand(verifyLogCmd)` in `init()` of the new file (Go runs `init()` in file-name order within a package, and `evalCmd` is a package-level var, so ordering is safe).
- Exit status: 0 on success; non-zero when `VerifyLog` returns an error (main prints `Error: <violations>` to stderr and exits 1).

### 4. Fixtures: `internal/eval/testdata/verifylog/`

Shared results (read-only for tests) in `internal/eval/testdata/verifylog/evals/results/<run>/{summary.json,selftest.json}`. Minimal files: `summary.json` with `agent_verdicts.selftest.score` and `agents.selftest.prompt_sha256`; `selftest.json` with `agent`, `prompt_sha256`, `cases: []`. Run names are realistic (`260701-100000-aaaaaaa`, ...). Suggested runs:

| Run | score | prompt_sha256 |
|-----|-------|---------------|
| `r0` baseline | 60.0 | hash A (64 hex chars) |
| `r1` | 72.5 | hash B |
| `r2` | 90.0 | hash C |
| `r3` | 91.0 | hash C (same prompt, eval-only change) |
| `r1-dup` | 73.0 | hash B (same prompt as r1) |
| `r-legacy` (optional) | `agent_scores` only 0.8 (=80.0), no verdicts | hash D |

Log sets, each `internal/eval/testdata/verifylog/<set>/selftest/iteration-NN.md`, each failing only its own rule:

| Set | Content | Expected |
|-----|---------|----------|
| `valid` | 3 iterations: `00` prompt r0→r1, `01` prompt r1→r2, `02` eval r2→r3 | exit 0 |
| `missing-run` | `result_run` names a non-existent run | `run-exists` |
| `score-mismatch` | claimed `result_score` differs from the run by > 0.1 | `score-match` |
| `gap` | `00` and `02` | `sequence` |
| `broken-chain` | `01.baseline_run` is an existing run other than `00.result_run` | `chain` |
| `no-prompt-change` | `prompt` iteration r1→r1-dup (same sha) | `prompt-change` |

The `valid` set with `--min-iterations 5` → `min-iterations`.

### 5. Tests

`internal/eval/verifylog_test.go` (written first, package `eval`):
- Table test over every fixture set above: error is `*VerifyLogError`, contains exactly the expected rule, and the violation `File` ends in the expected `iteration-NN.md`; `valid` returns nil and prints `OK selftest: 3 iterations verified`.
- `valid` with `MinIterations: 5` → `min-iterations`.
- Tolerance boundary: claimed differs by exactly 0.1 passes, by 0.2 fails (use `t.TempDir()` copies built by a small helper that writes logs and runs).
- Score sources: `agent_verdicts` percent, `agent_scores` fraction fallback (`r-legacy`), recompute from `<agent>.json` cases when summary has neither.
- Unit cases (TempDir): missing front-matter delimiter; missing key; bad `change_type`; `97.25` / `85` / `101.0` scores; non-calendar date; `iteration` ≠ file NN; run name with `../`; missing heading; headings out of order; empty section; heading inside a code fence ignored; `chronology` violation; `no-logs` (missing dir); non-`iteration-NN.md` files ignored; `iteration-1.md` → `sequence`; prompt iteration whose run lacks `prompt_sha256` → `run-data`; `eval` iteration with identical sha passes; multiple violations are all reported; agent name `../x` rejected.
- A test that `VerifyLog` never reads outside `<evals-dir>/results/` is covered by the `../` case.

`cmd/kairon/cmd/eval_verify_log_test.go`:
- Flag registration and defaults (`iterations-dir` = `.kairon/iterations`, `min-iterations` = `2`), `--evals-dir` inherited.
- Run `verifyLogCmd.RunE` (set package vars, as `eval_test.go` does; `go test` cwd is `cmd/kairon/cmd`, so fixture paths are `../../../internal/eval/testdata/verifylog/...`) against `valid` → nil, and against each failing set → error containing the file name and the rule ID. `--min-iterations 5` on `valid` → error.
- Missing agent argument → `cobra.ExactArgs(1)` error.

## Relevant Files

Create:
- `.kairon/iterations/README.md`
- `internal/eval/verifylog.go`, `internal/eval/verifylog_test.go`
- `internal/eval/testdata/verifylog/evals/results/*/{summary.json,selftest.json}`
- `internal/eval/testdata/verifylog/{valid,missing-run,score-mismatch,gap,broken-chain,no-prompt-change}/selftest/iteration-NN.md`
- `cmd/kairon/cmd/eval_verify_log.go`, `cmd/kairon/cmd/eval_verify_log_test.go`

Modify:
- `docs/evaluation.md` — new section "Iteration Logs and `verify-log`" (after "Evaluating a candidate prompt", before "Case Workspaces"): format summary linking to `.kairon/iterations/README.md`, command and flags, rules table with IDs, where the score comes from (precedence), exit status, fixtures and the `selftest` verification command. Add `.kairon/iterations/` to the directory-structure discussion by a one-line pointer, and amend "Where to put candidates" to say `verify-log` reads only `iteration-NN.md` files.

Read-only references: `internal/eval/diff.go` (`loadSummary`, `loadAgentResult`), `internal/eval/verdict.go` (`agentVerdict`), `internal/eval/types.go` (`Summary`, `AgentResult`), `internal/eval/provenance.go`, `cmd/kairon/cmd/eval.go`, `AGENTS.md`.

Not changed: `AGENTS.md`, `.kairon/specs/**` (historical), `CHANGELOG.md`, `Taskfile.yml`, `cmd/kairon/templates/**` (no synced file changes).

## Team Orchestration

- `write-iterations-readme-and-docs` and `create-verifylog-fixtures` have no dependencies and run in parallel (docs/README are written from this spec's rule table, which is the contract).
- `implement-verifylog-core` needs the fixtures (its tests read them) and follows TDD: tests first, observe the expected failure (`undefined: VerifyLog` is a compile error and does NOT count; first create a stub returning `errors.New("not implemented")` so tests fail for the right reason), then implement.
- `implement-verify-log-command` needs the core.
- `validate-verify-log` (read-only) runs last and re-checks every acceptance criterion.
- One PR; the builder commits tests and implementation together per AGENTS.md and names the first-failing test in the commit message.

## Step-by-Step Task Breakdown

1. **create-verifylog-fixtures** — Create the fixture result runs and the six log sets exactly as in the tables above. Every log is complete (full front-matter and all five headings with content), so each failing set fails only its intended rule. Scores in `valid` match the runs exactly.
2. **write-iterations-readme-and-docs** — Write `.kairon/iterations/README.md` (layout, front-matter table, body headings, complete example, rules table, where scores/runs come from, command, note that candidate prompts coexist) and the `docs/evaluation.md` section. The README example should be consistent with the `valid` fixture so docs and tests agree.
3. **implement-verifylog-core** — `internal/eval/verifylog.go` and `verifylog_test.go` as designed. Reuse `loadSummary`, `loadAgentResult`, `agentVerdict`. No `cfg` access. Run `gofmt`.
4. **implement-verify-log-command** — `cmd/kairon/cmd/eval_verify_log.go` plus tests; confirm `--evals-dir` inheritance and `SilenceUsage`.
5. **validate-verify-log** — Run build, vet/lint, full tests, sync check and the acceptance commands below.

## Validation Commands

```bash
# AC2: valid set exits 0
go run ./cmd/kairon eval verify-log selftest \
  --iterations-dir internal/eval/testdata/verifylog/valid \
  --evals-dir internal/eval/testdata/verifylog/evals

# AC3/AC4/AC5: each exits non-zero and names file + rule
for s in missing-run score-mismatch gap broken-chain no-prompt-change; do
  go run ./cmd/kairon eval verify-log selftest \
    --iterations-dir internal/eval/testdata/verifylog/$s \
    --evals-dir internal/eval/testdata/verifylog/evals && echo "UNEXPECTED PASS $s"
done

# AC6
go run ./cmd/kairon eval verify-log selftest --min-iterations 5 \
  --iterations-dir internal/eval/testdata/verifylog/valid \
  --evals-dir internal/eval/testdata/verifylog/evals

go test ./internal/eval/... ./cmd/kairon/cmd/...
task test
task lint
task sync:check
go run ./cmd/kairon plan parse .kairon/specs/issue-320-eval-iteration-log-verifier.md
```

## Acceptance Criteria Traceability

| AC | Delivered by |
|----|--------------|
| 1 | `.kairon/iterations/README.md` with complete example (task `write-iterations-readme-and-docs`) |
| 2 | `valid` fixture, `VerifyLog`, `verify-log` command with `--iterations-dir`/`--evals-dir` |
| 3 | rules `run-exists`, `run-data`, `score-match`; fixtures `missing-run`, `score-mismatch` |
| 4 | rules `sequence`, `chain`; fixtures `gap`, `broken-chain` |
| 5 | rule `prompt-change`; fixture `no-prompt-change` |
| 6 | rule `min-iterations`; `--min-iterations` default 2 |

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "create-verifylog-fixtures"
    agent: "builder"
    description: "Create fixtures under internal/eval/testdata/verifylog/: shared result runs in evals/results/<run>/{summary.json,selftest.json} (scores 60.0, 72.5, 90.0, 91.0, a duplicate-prompt run, and a legacy agent_scores-only run) and the log sets valid, missing-run, score-mismatch, gap, broken-chain and no-prompt-change, each as <set>/selftest/iteration-NN.md with complete front-matter and all five body headings, each failing only its own rule."
    dependencies: []
    acceptance_criteria:
      - "internal/eval/testdata/verifylog/evals/results/ contains run directories each with summary.json (agent_verdicts.selftest.score and agents.selftest.prompt_sha256) and selftest.json (agent, prompt_sha256, cases)"
      - "The valid set has iteration-00.md (prompt), iteration-01.md (prompt) and iteration-02.md (eval, same prompt_sha256 in both runs) forming an unbroken baseline_run/result_run chain with claimed scores equal to the recorded scores"
      - "The sets missing-run, score-mismatch, gap, broken-chain and no-prompt-change exist and each violates exactly one rule (run-exists, score-match, sequence, chain, prompt-change respectively)"
      - "Fixture results are not gitignored (git check-ignore reports nothing for them)"
    validation_commands:
      - "test -f internal/eval/testdata/verifylog/valid/selftest/iteration-00.md"
      - "test -d internal/eval/testdata/verifylog/evals/results"
      - "for s in valid missing-run score-mismatch gap broken-chain no-prompt-change; do test -d internal/eval/testdata/verifylog/$s/selftest || exit 1; done"
      - "! git check-ignore -q internal/eval/testdata/verifylog/evals/results/$(ls internal/eval/testdata/verifylog/evals/results | head -1)/summary.json"

  - id: "write-iterations-readme-and-docs"
    agent: "builder"
    description: "Write .kairon/iterations/README.md defining the log layout, YAML front-matter, body headings, a complete example, the verification rules table (rule IDs no-logs, front-matter, headings, sequence, chain, chronology, run-exists, run-data, score-match, prompt-change, min-iterations), where scores and runs come from, and that candidate prompt files may coexist. Add a 'Iteration Logs and verify-log' section to docs/evaluation.md (command, flags --iterations-dir/--min-iterations/--evals-dir, score source precedence, exit status, selftest verification command) and amend 'Where to put candidates' to say verify-log reads only iteration-NN.md files."
    dependencies: []
    acceptance_criteria:
      - "README defines .kairon/iterations/<agent>/iteration-NN.md with NN starting at 00"
      - "README lists front-matter keys iteration, date, change_type (prompt or eval), baseline_run, result_run, baseline_score, result_score (percent, one decimal) and body headings Baseline, Hypothesis, Change, Results, Reasoning"
      - "README contains one complete example log (front-matter plus all five headings with content)"
      - "README and docs/evaluation.md document every rule ID and the default values .kairon/iterations and --min-iterations 2"
      - "docs/evaluation.md contains the selftest verify-log command from the issue and documents that scores come from agent_verdicts.<agent>.score with agent_scores and recompute fallbacks"
      - "No file under .kairon/specs/ other than this spec is modified"
    validation_commands:
      - "test -f .kairon/iterations/README.md"
      - "grep -q 'iteration-NN.md' .kairon/iterations/README.md"
      - "grep -q 'verify-log' docs/evaluation.md"
      - "for r in no-logs front-matter headings sequence chain chronology run-exists run-data score-match prompt-change min-iterations; do grep -q \"$r\" .kairon/iterations/README.md || exit 1; done"

  - id: "implement-verifylog-core"
    agent: "builder"
    description: "TDD: add internal/eval/verifylog_test.go first (table test over every fixture set plus TempDir unit cases for tolerance boundary, score-source precedence, front-matter and heading validation, chronology, no-logs, path-traversal rejection, multiple violations), confirm it fails for the right reason against a not-implemented stub, then implement internal/eval/verifylog.go (VerifyLogOptions, LogViolation, VerifyLogError, VerifyLog, DefaultIterationsDir, DefaultMinIterations) per the design. Reuse loadSummary, loadAgentResult and agentVerdict; do not read package state cfg. Commit test and implementation together."
    dependencies: ["create-verifylog-fixtures"]
    acceptance_criteria:
      - "VerifyLog returns nil and prints 'OK selftest: 3 iterations verified' for the valid fixture set"
      - "The missing-run, score-mismatch, gap, broken-chain and no-prompt-change sets return a *VerifyLogError whose violations carry the rules run-exists, score-match, sequence, chain and prompt-change and a File ending in the offending iteration-NN.md"
      - "The valid set with MinIterations 5 returns a min-iterations violation"
      - "A claimed score exactly 0.1 away from the recorded score passes; 0.2 away fails with score-match"
      - "Recorded score is read from agent_verdicts (percent), then agent_scores (fraction x100), then recomputed from <agent>.json"
      - "A prompt iteration whose run records no prompt_sha256 fails with run-data; an eval iteration with identical prompt_sha256 passes"
      - "Run names or agent names containing path separators or '..' are rejected and nothing outside <evals-dir>/results is read"
      - "All violations in a set are reported, not only the first; Error() renders one '<file>: <rule>: <message>' line each"
      - "The first-failing test name is recorded in the commit message or sentinel"
    validation_commands:
      - "gofmt -l internal/eval | (! grep .)"
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'VerifyLog' -count=1 -v"
      - "go test ./internal/eval/... -count=1"

  - id: "implement-verify-log-command"
    agent: "builder"
    description: "Add cmd/kairon/cmd/eval_verify_log.go registering 'eval verify-log <agent>' on evalCmd with --iterations-dir (default .kairon/iterations) and --min-iterations (default 2, negative rejected), inheriting the persistent --evals-dir, using cobra.ExactArgs(1), SilenceUsage after argument parsing, and calling eval.VerifyLog with cmd.OutOrStdout(). Add cmd/kairon/cmd/eval_verify_log_test.go (written first) covering flag registration/defaults, the valid fixture set, each failing fixture set naming file and rule, --min-iterations 5, and the missing-argument error."
    dependencies: ["implement-verifylog-core"]
    acceptance_criteria:
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals exits 0"
      - "The missing-run, score-mismatch, gap, broken-chain and no-prompt-change fixture sets each exit non-zero with output naming the iteration file and the rule ID"
      - "The valid set with --min-iterations 5 exits non-zero"
      - "--iterations-dir defaults to .kairon/iterations and --min-iterations defaults to 2; --evals-dir is inherited from eval"
      - "Usage text is not printed for a verification failure; a missing agent argument is a usage error"
    validation_commands:
      - "go build ./..."
      - "go test ./cmd/kairon/cmd/ -run 'VerifyLog' -count=1 -v"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals --min-iterations 5"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/gap --evals-dir internal/eval/testdata/verifylog/evals"

  - id: "validate-verify-log"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion in issue #320: README completeness, verify-log exit codes for all fixture sets with file and rule named in the output, --min-iterations behaviour, docs section present, full test/lint/sync checks green, and no historical artifacts (.kairon/specs other than this spec, CHANGELOG.md) modified."
    dependencies: ["write-iterations-readme-and-docs", "implement-verify-log-command"]
    acceptance_criteria:
      - "AC1: .kairon/iterations/README.md defines the path layout, front-matter keys, body headings and includes a complete example"
      - "AC2: verify-log on the valid fixture set exits 0"
      - "AC3: missing-run and score-mismatch sets exit non-zero naming the file and rule"
      - "AC4: gap and broken-chain sets exit non-zero naming the file and rule"
      - "AC5: no-prompt-change set exits non-zero"
      - "AC6: the valid set with --min-iterations 5 exits non-zero"
      - "task test, task lint and task sync:check all pass"
      - "AGENTS.md TDD rule satisfied: tests and implementation are in the same commit"
    validation_commands:
      - "go build ./..."
      - "task test"
      - "task lint"
      - "task sync:check"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/missing-run --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/score-mismatch --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/broken-chain --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/no-prompt-change --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --min-iterations 5 --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
```
