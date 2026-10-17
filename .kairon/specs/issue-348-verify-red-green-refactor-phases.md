# Design: verify Red/Green/Refactor phases in iteration logs

Closes #348

## Problem

`kairon eval verify-log` (issue #343 and earlier) checks that an iteration log's scores, run chain and `change_type` agree with the committed eval results. It cannot tell whether an iteration is a Red, Green or Refactor step, so nothing proves Eval-Driven Development discipline (maturity-model Stage 3, stage-3.md §3, gap-analysis.md §S3.5 item 5). Agents will write these logs, so each phase claim must be checkable against committed results.

## Solution Approach

Add one front-matter key, `phase`, and one verifier rule per phase. The verifier reads the per-check results that every run already stores in `<agent>.json` (`cases[].scores[].checks[]`) and compares the baseline run with the result run of each iteration. A set-level rule requires a complete Red → Green → Refactor cycle. No change to the runner or to the result format is needed.

Everything lives in the existing `VerifyLog` pipeline (`internal/eval/verifylog.go`). New rules follow the existing pattern: collect all violations, report `<file>: <rule>: <message>`, exit non-zero.

### Front-matter

New required key `phase` (a string). Allowed values: `baseline`, `red`, `green`, `refactor`. `baseline` is allowed only in `iteration-00.md` (by file number). `phase` is the 8th required key; the existing seven are unchanged.

A bad or missing `phase` is its own rule (`phase`), not `front-matter`, and does **not** set `parsedLog.valid` to false. The other checks (chain, runs, scores) still run, so one run reports every problem. The phase-specific rules below run only for a log whose front-matter is `valid` **and** whose phase is one of the four values.

### New rule IDs

Add to the `rule*` constants and to `ruleOrder` (after `suite-change`, before `min-iterations`): `phase`, `red`, `green`, `refactor`, `cycle`.

| Rule | Fails when |
|------|-----------|
| `phase` | `phase` is missing, is not one of `baseline`/`red`/`green`/`refactor`, or is `baseline` in a file other than `iteration-00.md`. Message names the key, the bad value and the allowed values. |
| `red` | `change_type` is not `eval`, or the result run has no failing check that the baseline run does not also have (see "Check identity"). |
| `green` | `change_type` is not `prompt`; or no check that failed in the baseline run passes in every repeat of the result run; or the result run's score is lower than the baseline run's. Each failing sub-condition is its own violation. |
| `refactor` | `change_type` is not `prompt`, or the result run's score is lower than the baseline run's. |
| `cycle` | `--require-cycle` is on and the set has no `red` iteration followed later by a `green` iteration, or has no `refactor` iteration. Reported against the agent directory (set-level), one violation per missing part. Skipped when there are no logs (`no-logs` already covers that). |

`baseline` has no rule beyond its position. Its `change_type` is either value and no evidence is required, because iteration 00 compares two starting points (for example the legacy prompt against a minimal skeleton).

### Check identity and "failing"

Per-check results are `checks.Result{Index, Type, Label, Passed, Detail}` (see `internal/checks/checks.go`). `Label` is `#<index> <type> <key fields>`, for example `#2 output_contains pattern=/ready/`. The index is the check's position in the case, so adding a check in the middle shifts the later indexes.

Identity of a check = **(case name, criterion name, label with the leading `#<n> ` removed)**. This satisfies "same case, criterion and check definition" without being fooled by index shifts. The label carries the type and the key fields the harness already considers part of the definition. Limitation to document: fields the label does not print (for example `expect_exit`, `inject`, a `command` string past 80 characters) are not part of the identity.

Per run, built once in `loadRunRecord` from the already-loaded `AgentResult`:

- Group `Cases` by `case_name`. Each `CaseResult` is one repeat (`Run` defaults to 1; do not rely on its value, count entries).
- `failing(run)` = set of identities whose result is `Passed == false` in **any** `CaseResult` (any repeat). This is the "fails in any repeat" rule.
- `passesEverywhere(run, id)` = the case has at least one `CaseResult`, **every** `CaseResult` of that case contains the identity, and every occurrence has `Passed == true`. A check that is absent from a repeat (case errored, check deleted) does not count as passing. If two checks in a case share an identity, all occurrences must pass.
- Skipped criteria and criteria with no `checks` contribute nothing.

Rules:

- `red`: `failing(result) \ failing(baseline)` is non-empty. This is the literal reading of the issue ("at least one failing check that its baseline run doesn't have"). A baseline that records no checks therefore has an empty failing set. Document in the README that a flaky pre-existing check that fails only in the result run also satisfies it; the `eval` change_type rules (suite or judge must change, prompt must not) bound that.
- `green`: some identity `k ∈ failing(baseline)` has `passesEverywhere(result, k)`.

### Score comparison

`green` and `refactor` compare the **recorded** scores of the two runs (`runRecord.score`, already read for `score-match`), not the claimed front-matter scores. Rule: `result.score >= baseline.score - scoreEpsilon`. There is no 0.1 tolerance here; a lower score by any amount fails. Skip the comparison (no cascade) when either run has no recorded score; `run-data` already reports that. Skip the whole phase evidence block when either run does not exist (`run-exists` already reports that).

Messages for the score violation name both runs and both recorded scores.

### Cycle rule and `--require-cycle`

`cycle` is label-based: it looks at the `phase` values of all logs with a valid phase, in file-number order, without requiring those logs to pass their own rules (a failing log is already a violation; this avoids duplicate noise). "Followed later" means a higher iteration number. A `refactor` iteration may appear anywhere; the issue states only "at least one". (Open point for the reviewer: the maturity-model text implies a refactor after the green; the issue does not require it and the per-agent issues depend on this exact rule, so it is implemented as written.)

`VerifyLogOptions` gets `RequireCycle bool`, used as given like `MinIterations`. The zero value is `false`, so existing library callers and unit tests keep their current behaviour. The CLI flag defaults to `true`.

### CLI

`cmd/kairon/cmd/eval_verify_log.go`: package var `verifyLogRequireCycle bool`, flag `--require-cycle` (`BoolVar`, default `true`, help text mentions that `--require-cycle=false` disables the Red→Green→Refactor requirement), passed as `RequireCycle`. Update the `Long` text. The success line stays `OK <agent>: N iterations verified`.

## Relevant Files

Modify:
- `internal/eval/verifylog.go`: `phase` key and constants, new rule IDs and `ruleOrder`, `RequireCycle`, check-evidence loading in `loadRunRecord`, `checkPhase` (per log, called from the `checkRuns` loop), `checkCycle` (set level).
- `internal/eval/verifylog_test.go`: helpers (`vlFM`, `vlGoodFM`, `vlStdRuns`, `vlFixtureOpts`) and the 50 `vlFM`/`vlGoodFM` call sites need a phase; new tests.
- `cmd/kairon/cmd/eval_verify_log.go`, `cmd/kairon/cmd/eval_verify_log_test.go`.
- `.kairon/iterations/README.md`, `docs/evaluation.md` (section "Iteration Logs and `verify-log`").
- Every existing fixture log under `internal/eval/testdata/verifylog/*/selftest/iteration-*.md` (add `phase`).

Create (fixtures under `internal/eval/testdata/verifylog/`):
- Sets: `missing-phase`, `baseline-not-first`, `red-no-new-failure`, `green-no-fix`, `green-score-drop`, `refactor-score-drop`, `no-refactor`. Rewrite `valid`.
- New runs in `evals/results/` whose `selftest.json` carries `cases[].scores[].checks[]` (see "Fixture design").

Not touched: `.kiro/agents/*.json` (unrelated uncommitted changes in this worktree), `.kairon/specs/` other than this file, `CHANGELOG.md`, `.kairon/evals/results/`. `.kairon/iterations/README.md` and `internal/eval/testdata/**` are not in the template-sync mappings, so `task sync:check` should be unaffected; run it anyway.

## Fixture design

All fixtures use agent `selftest`, and `--evals-dir internal/eval/testdata/verifylog/evals`. New runs are named `yymmdd-hhmmss-<7 hex>` like the existing ones and record the standard facts (`agent-m`, `judge-m`, native, `complete: true`, `case_filter: ""`, prompt and suite hashes). Existing runs `a` (60.0, prompt A), `b` (72.5, prompt B) have no checks and stay unchanged.

Case `basic`, criterion `alpha`, two repeats (`run: 1`, `run: 2`) as separate `cases[]` entries. Checks: `#1 file_exists path=out.txt` (always passes) and `#2 output_contains pattern=/ready/` (the check under test).

| Run | Prompt | Suite | Score | `#2` result |
|-----|--------|-------|-------|-------------|
| `R1` (red result) | B | S2 | 70.0 | passes in repeat 1, **fails in repeat 2** (shows "any repeat") |
| `R0` (red, no new failure) | B | S2 | 70.0 | passes both |
| `G1` (green result) | C' | S2 | 85.0 | passes both |
| `Gx` (green, no fix) | C' | S2 | 85.0 | passes repeat 1, fails repeat 2 |
| `Gd` (green, score drop) | C' | S2 | 65.0 | passes both |
| `F1` (refactor result) | D' | S2 | 85.0 | passes both (equal score proves "not lower") |
| `Fd` (refactor, score drop) | D' | S2 | 80.0 | passes both |

Prompt hashes C', D' must differ from each other and from B; S2 differs from S1.

Sets (each is a copy of the valid prefix plus one defect):

| Set | Logs | Expected result (CLI, defaults) |
|-----|------|-------------------------------|
| `valid` | 00 baseline prompt `a→b`; 01 red eval `b→R1`; 02 green prompt `R1→G1`; 03 refactor prompt `G1→F1` | exit 0, `OK selftest: 4 iterations verified` |
| `missing-phase` | `valid` with `phase` removed from `iteration-00.md` | `phase` on `iteration-00.md` only (cycle is still satisfied by 01–03) |
| `baseline-not-first` | `a→b` baseline, then a second log with `phase: baseline` | `phase` on that log |
| `red-no-new-failure` | 00 baseline, 01 red eval `b→R0` | `red`, plus `cycle` unless disabled |
| `green-no-fix` | 00 baseline, 01 red `b→R1`, 02 green `R1→Gx` | `green` (no check passes in every repeat), plus `cycle` unless disabled |
| `green-score-drop` | 00, 01 red `b→R1`, 02 green `R1→Gd` | `green` (score), plus `cycle` unless disabled |
| `refactor-score-drop` | `valid` with 03 `G1→Fd` | `refactor` only (cycle satisfied) |
| `no-refactor` | `valid` without 03 | `cycle`; exit 0 with `--require-cycle=false` |

The unit-test fixture table runs the sets that also trip `cycle` with `RequireCycle: false`, so each asserts exactly one violation of the intended rule. CLI tests assert that the error text contains the file name and rule.

Existing sets get a legal phase so they still report only their own rule: `baseline` for `iteration-00`; `refactor` for later `prompt` iterations (all existing prompt pairs rise in score; check each); `red` for the two `eval` iterations in `eval-prompt-changed` and `eval-nothing-changed`, whose result runs (`6666666`, `7777777`) get a failing check in `selftest.json` so `red` is satisfied. In the unit tests, runs used as `eval` results (`r0s`, `r2s`, `r2b`) likewise get a failing check so the default `eval → red` helper phase holds. `TestVerifyLogFixtureValidEvalIteration` is updated to pin the new red pair (same prompt, different suite); delete runs `c`/`d` only if nothing references them (`c` is used by `gap`, `broken-chain`, `score-mismatch`). `TestVerifyLogFixtureRunsRecordCoverage` must still pass (new runs record coverage; `checked >= 12`).

## Team Orchestration

Single PR, one builder at a time (the tasks share `verifylog.go` and its test file, so they are sequential). Docs and CLI work run in parallel after the verifier core is done. The validator runs last.

```
phase-key ──► evidence-rules ──► cycle-rule ──┬──► cli-flag ──┐
                                              └──► docs ──────┴──► validate-all
```

TDD (AGENTS.md): in each task write the failing test first, run it and confirm it fails for the expected reason (a missing rule, not a compile error), implement the minimum, rerun. Commit test and implementation together per task, with the observed first failure named in the commit message. Do not commit a failing test alone. Build before each commit.

## Step-by-Step Task Breakdown

### Task 1: `phase` key, `phase` rule, fixture migration (`phase-key`)
- Add `Phase *string` to `logFrontMatter`, `phase` to `parsedLog`, constants for the four values, rule constants/`ruleOrder` for all five new rules.
- Report `phase` violations as specified; do not clear `valid`.
- Update test helpers so every generated log carries a phase (derive a default from iteration number and change type, with an explicit override for tests that need a specific phase), and add `phase` to every existing fixture log.
- Add the `missing-phase` and `baseline-not-first` sets and tests.
- Acceptance: AC 1.

### Task 2: Red/Green/Refactor evidence rules and fixtures (`evidence-rules`)
- Build per-run check records in `loadRunRecord` (identity, failing set, passes-everywhere).
- Implement `red`, `green`, `refactor` per the rules above, using recorded scores.
- Create the new runs and sets (`red-no-new-failure`, `green-no-fix`, `green-score-drop`, `refactor-score-drop`), rewrite `valid` to the 4-iteration cycle, give the eval-result runs of legacy sets a failing check, and update fixture-pinning tests.
- Unit tests for identity (index shift does not change identity; different criterion or case is a different check), any-repeat failure, absent check is not a pass, equal score passes, tiny drop fails, missing score or run does not cascade.
- Acceptance: AC 2, 3, 4.

### Task 3: Cycle rule and `RequireCycle` option (`cycle-rule`)
- Add `RequireCycle` to `VerifyLogOptions` and `checkCycle`. Add the `no-refactor` set.
- Unit tests: red-after-green fails, missing green fails, missing refactor fails, both missing gives two violations, `RequireCycle: false` passes, and no logs gives only `no-logs`.
- Acceptance: AC 5 (library level).

### Task 4: CLI flag (`cli-flag`)
- Add `--require-cycle` (default true), update `Long`, update `cmd/kairon/cmd/eval_verify_log_test.go`: flag registered with default `true`, `runVerifyLog` sets and restores `verifyLogRequireCycle`, `valid` prints `OK selftest: 4 iterations verified`, new failing sets are in the table (error names file and rule), `no-refactor` fails by default and passes with the flag false.
- Acceptance: AC 1, 5 via the CLI.

### Task 5: Documentation (`docs`)
- `.kairon/iterations/README.md`: add `phase` to the front-matter table (now eight keys), a "Phases" section with the four values, per-phase rules, check identity and the any-repeat rule, `--require-cycle` in the flag table, the new rule IDs in the rule table, and an example: update the complete example with `phase: baseline` and add a Red/Green/Refactor example set (front-matter of the four logs plus a short note on which run facts prove each). Fix text that says "seven keys" and the claim that the example matches the `valid` fixture.
- `docs/evaluation.md` "Iteration Logs and `verify-log`": flag table and command synopsis, rule table, new subsection on phases, fixture table, `valid` now prints 4 iterations, the verification commands from the issue.
- No template sync needed (these paths are not mapped); do not edit `.kairon/specs/` or `CHANGELOG.md`.
- Acceptance: AC 6.

### Task 6: Validation (`validate-all`)
- Read-only: run the full checks, the CLI scenarios from the issue, and inspect the README for AC 6.

## Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/eval -run VerifyLog -count=1
go test ./cmd/kairon/cmd -run VerifyLog -count=1
go test ./... 
task fmt:check
task sync:check

F=internal/eval/testdata/verifylog
E=$F/evals
go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/valid --evals-dir $E                      # exit 0
! go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/missing-phase --evals-dir $E            # names iteration-00.md and "phase"
! go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/baseline-not-first --evals-dir $E
! go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/red-no-new-failure --evals-dir $E
! go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/green-no-fix --evals-dir $E
! go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/green-score-drop --evals-dir $E
! go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/refactor-score-drop --evals-dir $E
! go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/no-refactor --evals-dir $E
go run ./cmd/kairon eval verify-log selftest --iterations-dir $F/no-refactor --evals-dir $E --require-cycle=false   # exit 0
```

## Risks and Notes

- Making `phase` required breaks every existing fixture and test log at once. Task 1 migrates them in the same commit as the rule.
- Check identity depends on `Label`. The label omits some definition fields; this is documented rather than changing the result format (out of scope: run identity rules).
- The `red` rule is the literal "failing in result, not failing in baseline". A baseline with no recorded check data makes every failing result check new. Documented.
- Recorded scores are compared exactly, so a log whose claimed scores round equal can still fail `refactor` when the recorded scores differ by a hundredth. The message prints both recorded values.
- `phase`, its four values and `--require-cycle` are a contract for the per-agent iteration issues; do not rename them.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "phase-key"
    agent: "builder"
    description: "Add the required `phase` front-matter key and `phase` rule (values baseline/red/green/refactor, baseline only for iteration 00) to internal/eval/verifylog.go, register all five new rule IDs, migrate the test helpers and every existing fixture log to carry a phase, and add the missing-phase and baseline-not-first fixture sets. Write the failing tests first (TDD per AGENTS.md)."
    dependencies: []
    acceptance_criteria:
      - "A log without `phase` produces a `phase` violation naming the file and the key; an unknown value and `baseline` in a file other than iteration-00.md also produce `phase` violations"
      - "A phase violation does not suppress chain, run or score checks for the same file"
      - "Rule constants phase, red, green, refactor, cycle exist and are ordered in ruleOrder before min-iterations"
      - "All pre-existing verify-log unit tests and fixture sets still report exactly their original single rule"
      - "Fixture sets missing-phase and baseline-not-first exist and fail with rule `phase`"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval -run VerifyLog -count=1"
      - "go vet ./internal/eval/..."
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/missing-phase --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/baseline-not-first --evals-dir internal/eval/testdata/verifylog/evals"

  - id: "evidence-rules"
    agent: "builder"
    description: "Load per-check results from <agent>.json into each run record (identity = case + criterion + label without the `#n ` prefix; failing = fails in any repeat; passes-everywhere = present and passing in every repeat) and implement the red, green and refactor rules, comparing recorded scores with no tolerance. Create the fixture runs R0, R1, Gx, Gd, G1, F1, Fd with checks, the sets red-no-new-failure, green-no-fix, green-score-drop, refactor-score-drop, rewrite the valid set as baseline/red/green/refactor, give the eval-result runs of legacy sets and unit-test std runs a failing check, and update fixture-pinning tests. Failing tests first."
    dependencies: ["phase-key"]
    acceptance_criteria:
      - "red: change_type other than eval fails; a result run with no failing check absent from the baseline's failing set fails; a check that fails in only one repeat counts as failing"
      - "green: change_type other than prompt fails; no baseline-failing check passing in every repeat of the result run fails; a result score below the baseline score fails; each condition is a separate `green` violation"
      - "refactor: change_type other than prompt fails; a result score below the baseline score fails; an equal score passes"
      - "Check identity ignores the `#n` index (inserting a check earlier does not make an existing check new) and distinguishes case and criterion"
      - "A check missing from a repeat does not count as passing; a missing run or missing score does not cascade extra violations"
      - "Sets red-no-new-failure, green-no-fix, green-score-drop and refactor-score-drop each report exactly one violation of the intended rule when RequireCycle is false"
      - "The valid set has four iterations (baseline, red, green, refactor) and verifies with 'OK selftest: 4 iterations verified'"
      - "Legacy eval sets still report only their original rule; TestVerifyLogFixtureRunsRecordCoverage still passes"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval -run VerifyLog -count=1"
      - "go vet ./internal/eval/..."

  - id: "cycle-rule"
    agent: "builder"
    description: "Add RequireCycle to VerifyLogOptions (used as given, zero value false) and a set-level `cycle` rule: a red iteration followed later by a green iteration, and at least one refactor iteration, reported against the agent directory; skipped when there are no logs. Add the no-refactor fixture set and unit tests. Failing tests first."
    dependencies: ["evidence-rules"]
    acceptance_criteria:
      - "With RequireCycle true, a set lacking red-then-green fails with a `cycle` violation; a set lacking refactor fails with a `cycle` violation; a set lacking both yields two violations"
      - "green before red does not satisfy the cycle"
      - "With RequireCycle false the same sets pass the cycle check"
      - "An empty or missing agent directory yields only `no-logs`"
      - "The no-refactor fixture set fails on `cycle` and passes with RequireCycle false; the valid set passes with RequireCycle true"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval -run VerifyLog -count=1"
      - "go vet ./internal/eval/..."

  - id: "cli-flag"
    agent: "builder"
    description: "Add the --require-cycle bool flag (default true) to `kairon eval verify-log`, pass it as RequireCycle, update the command Long text, and update cmd/kairon/cmd/eval_verify_log_test.go (flag default, runVerifyLog restores the new package var, valid prints 4 iterations, new failing sets named by file and rule, no-refactor fails by default and passes with the flag false). Failing tests first."
    dependencies: ["cycle-rule"]
    acceptance_criteria:
      - "`kairon eval verify-log --help` lists --require-cycle with default true"
      - "`--require-cycle=false` makes the no-refactor set exit 0; the default makes it exit non-zero"
      - "The issue's verify command for missing-phase exits non-zero and its output names iteration-00.md and the `phase` rule"
      - "Every fixture set named in the issue exits non-zero via the CLI; valid exits 0"
    validation_commands:
      - "go build ./..."
      - "go test ./cmd/kairon/cmd -run VerifyLog -count=1"
      - "go run ./cmd/kairon eval verify-log --help | grep -- --require-cycle"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/no-refactor --evals-dir internal/eval/testdata/verifylog/evals --require-cycle=false"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/no-refactor --evals-dir internal/eval/testdata/verifylog/evals"

  - id: "docs"
    agent: "builder"
    description: "Update .kairon/iterations/README.md and docs/evaluation.md: document `phase` (now eight required front-matter keys), the rules for baseline/red/green/refactor, check identity and the any-repeat rule, the recorded-score comparison, --require-cycle, the new rule IDs, the new fixture sets and the 4-iteration valid set, and include an example log (the complete example with `phase: baseline` plus a Red/Green/Refactor example). Do not edit .kairon/specs/ (other than this spec), CHANGELOG.md or .kiro/agents/*.json."
    dependencies: ["cycle-rule"]
    acceptance_criteria:
      - ".kairon/iterations/README.md documents phase, its four values, the per-phase rules, --require-cycle and contains an example log with a phase"
      - "No text in README.md or docs/evaluation.md still says seven required keys or 'OK selftest: 3 iterations verified'"
      - "The rule tables in both documents list phase, red, green, refactor and cycle with the same conditions as the code"
      - "docs/evaluation.md flag table and synopsis include --require-cycle"
      - "The fixture table in docs/evaluation.md lists every new set and the rule it reports"
    validation_commands:
      - "grep -n 'phase' .kairon/iterations/README.md"
      - "grep -n 'require-cycle' .kairon/iterations/README.md docs/evaluation.md"
      - "! grep -n '3 iterations verified' docs/evaluation.md"
      - "task sync:check"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #348 end to end: run the build, vet, full tests and format check, run each CLI scenario from the issue against the fixture sets, check that TDD evidence is in the commit messages and that no file under .kiro/agents/ or .kairon/specs/ (other than the issue-348 spec) was modified, and inspect the README for AC 6."
    dependencies: ["cli-flag", "docs"]
    acceptance_criteria:
      - "AC1: missing-phase and baseline-not-first exit non-zero; the missing-phase output names iteration-00.md and the phase rule"
      - "AC2: red-no-new-failure exits non-zero with a `red` violation"
      - "AC3: green-no-fix and green-score-drop each exit non-zero with `green` violations"
      - "AC4: refactor-score-drop exits non-zero with a `refactor` violation"
      - "AC5: valid exits 0; no-refactor exits non-zero; --require-cycle=false turns the cycle rule off"
      - "AC6: README documents phase, its rules, --require-cycle and has an example log"
      - "go build, go vet, go test ./..., task fmt:check and task sync:check all pass"
      - "git diff shows no change to .kiro/agents/*.json made by this work"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./... -count=1"
      - "task fmt:check"
      - "task sync:check"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/missing-phase --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/baseline-not-first --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/red-no-new-failure --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/green-no-fix --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/green-score-drop --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/refactor-score-drop --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/no-refactor --evals-dir internal/eval/testdata/verifylog/evals"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/no-refactor --evals-dir internal/eval/testdata/verifylog/evals --require-cycle=false"
```
