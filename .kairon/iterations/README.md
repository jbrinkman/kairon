# Iteration Logs

An iteration log records one measured change to an agent: what the score was, what you expected, what you changed, what the score became and why you think so. A set of logs for one agent is the evidence of a documented, cause-and-effect improvement history (maturity-model Stage 3).

Every score a log claims must be checkable against eval results committed under `.kairon/evals/results/`. `kairon eval verify-log <agent>` does that check.

## Layout

```
.kairon/iterations/
  README.md                       # this file
  <agent>/
    iteration-00.md               # first iteration (NN starts at 00)
    iteration-01.md
    iteration-02.md
    load-bearing.md               # optional: instruction-to-criterion table read by `kairon eval audit`
    v2.md                         # optional candidate prompt files may live here too
```

- One directory per agent, named exactly like the agent (`architect`, `builder`, ...).
- Log files are named `iteration-NN.md`, where `NN` is the iteration number zero-padded to at least two digits. Numbering starts at `00` and must be contiguous: `00`, `01`, `02`, ... No gaps, no duplicates (`00` and `000` collide).
- Only files named `iteration-NN.md` are read by `verify-log`. Everything else in the agent directory (candidate prompts such as `v2.md`, notes, scratch files) may coexist and is ignored. A file that starts with `iteration-` and ends in `.md` but does not match `iteration-NN.md` (for example `iteration-1.md`) is reported as a `sequence` violation, so a misnamed log is never silently skipped.

## File format

A log is a Markdown file with YAML front-matter followed by five fixed headings.

### Front-matter

The file must start with `---`; the front-matter ends at the next `---` line. All eight keys are required. Unknown extra keys are ignored.

| Key | Format | Meaning |
|-----|--------|---------|
| `iteration` | integer, equal to `NN` in the file name | Position of this log in the sequence. |
| `date` | `YYYY-MM-DD`, a real calendar date | When the iteration was measured. Dates must not go backwards from one iteration to the next. |
| `change_type` | `prompt` or `eval` | `prompt`: the agent prompt changed and nothing else did. The two runs must record different `prompt_sha256` and the same `suite_sha256`, `judge_model`, `agent_model` and sandbox mode. `eval`: the rubric, cases or judge changed and the prompt did not. The two runs must record the same `prompt_sha256`, and a different `suite_sha256` or a different `judge_model` (changing neither fails); `agent_model` and the sandbox mode must still be the same. For either type both runs must be complete full runs. |
| `baseline_run` | name of a directory under `<evals-dir>/results/` | The run measured before the change. A single path element: no `/`, `\` or `..`. |
| `result_run` | same | The run measured after the change. |
| `baseline_score` | percent, exactly one decimal, `0.0`-`100.0` (`72.5`, `100.0`, `5.0`) | The agent score of `baseline_run`. Not `85`, not `97.25`, not `101.0`. |
| `result_score` | same | The agent score of `result_run`. |
| `phase` | `baseline`, `red`, `green` or `refactor` | Which Eval-Driven Development step this iteration is. `baseline` is allowed only in `iteration-00.md`. The committed results must back the claim; see [Phases](#phases). |

Scores are **percent**, not fractions: write `72.5`, not `0.725`.

A missing or invalid `phase` is reported as its own `phase` violation, not as `front-matter`, and the chain, run and score checks of the same file still run.

## Phases

Each iteration declares one phase. `verify-log` compares the per-check results that every run stores in `<agent>.json` (`cases[].scores[].checks[]`) for `baseline_run` and `result_run`, so a phase claim is checked against committed evidence.

| `phase` | Meaning | What the verifier requires |
|---------|---------|----------------------------|
| `baseline` | Starting point. Allowed only in `iteration-00.md`. | Nothing beyond its position. `change_type` may be either value and no check evidence is required, because iteration 00 compares two starting points (for example the legacy prompt against a minimal skeleton). |
| `red` | Add or change the eval so it exposes a gap. | `change_type` is `eval`, and `result_run` has at least one failing check that `baseline_run` does not also have. |
| `green` | Change the prompt so the failing check passes. | `change_type` is `prompt`; at least one check that failed in `baseline_run` passes in every repeat of `result_run`; and the recorded score of `result_run` is not lower than that of `baseline_run`. Each failed condition is its own violation. |
| `refactor` | Tidy the prompt without losing ground. | `change_type` is `prompt`, and the recorded score of `result_run` is not lower than that of `baseline_run`. |

A set that is meant to show Eval-Driven Development also needs a complete cycle: a `red` iteration followed later by a `green` iteration, and at least one `refactor` iteration. `--require-cycle` (on by default) enforces this; see [Verifying a log set](#verifying-a-log-set).

### Check identity and "failing"

The verifier needs to know whether two runs mean the same check. A check's identity is the triple **(case name, criterion name, check label without its leading `#<n> `)**. The label is what the harness records, for example `#2 output_contains pattern=/ready/`, so the identity of that check is `basic / alpha / output_contains pattern=/ready/`. Dropping the `#<n>` index means inserting a check earlier in a case does not make every later check look new.

Limitation: only what the label prints is part of the identity. Fields it omits (for example `expect_exit`, `inject`, or a `command` string past 80 characters) are not. Changing only those leaves the identity unchanged.

Per run:

- Each entry in `cases` for one case name is one repeat (`--repeat`). The entries are counted; the `run` number is not relied on.
- A check is **failing** when it fails in **any** repeat (the any-repeat rule). A check that passes in repeat 1 and fails in repeat 2 is failing.
- A check **passes everywhere** when its case has at least one repeat, the check is recorded in **every** repeat of that case, and every occurrence passes. A check that is absent from a repeat (the case errored, the check was deleted) does not count as passing. If two checks in a case share one identity, all of them must pass.
- Skipped criteria, and criteria with no recorded checks, contribute nothing.

### Rules per phase

- **`red`** is satisfied when the set of failing checks of `result_run` is not a subset of the failing checks of `baseline_run`. That is the literal reading of "at least one failing check that its baseline run doesn't have". A baseline that records no checks has no failing checks, so any failing check in the result run is new. A flaky check that happens to fail only in the result run also satisfies it; the `eval` change-type rules (the suite or the judge must change, the prompt must not) bound that.
- **`green`** is satisfied when some check that failed in `baseline_run` passes everywhere in `result_run`.
- **Score comparison (`green` and `refactor`).** The verifier compares the **recorded** scores of the two runs (the same score `score-match` reads), not the scores written in the front-matter. The result run's score must be greater than or equal to the baseline run's. There is no 0.1 tolerance here: a lower score by any amount fails, and an equal score passes. The message prints both recorded scores with two decimals. If either run has no recorded score, the comparison is skipped (`run-data` already reports that). If either run does not exist, the whole phase check is skipped (`run-exists` already reports that).

### Body headings

Five level-2 headings, each exactly once and in this order. Each needs non-blank content before the next `##` heading. A heading inside a fenced code block is ignored. Extra `##` headings are allowed.

| Heading | Write here |
|---------|------------|
| `## Baseline` | The starting score and the run it came from. |
| `## Hypothesis` | What you expect the change to do and why. |
| `## Change` | Exactly what you changed (prompt text, rubric, cases). |
| `## Results` | The measured score after the change and the run it came from. |
| `## Reasoning` | What the result means, whether the hypothesis held, and what the next iteration builds on. |

## Complete example

This is `.kairon/iterations/selftest/iteration-00.md`. Its front-matter is that of the `baseline` iteration of the `valid` fixture used by the verifier tests; the fixture's body text is shorter.

````markdown
---
iteration: 0
date: 2026-07-01
change_type: prompt
baseline_run: 260701-100000-aaaaaaa
result_run: 260702-100000-bbbbbbb
baseline_score: 60.0
result_score: 72.5
phase: baseline
---

## Baseline

The selftest agent scored 60.0% on run 260701-100000-aaaaaaa.

## Hypothesis

A clearer prompt should raise the score on the selftest cases, because most
lost points are on the structural_completeness criterion.

## Change

Rewrote the output-format section of the selftest prompt so every required
heading is listed explicitly. Evaluated as a candidate with
`kairon eval selftest --prompt-file .kairon/iterations/selftest/v2.md`.

## Results

The selftest agent scored 72.5% on run 260702-100000-bbbbbbb.

## Reasoning

The score rose by 12.5 points and the prompt hash changed, so the gain is
attributable to the new prompt. The next iteration builds on run
260702-100000-bbbbbbb.
````

Iteration `01` then sets `baseline_run` to `260702-100000-bbbbbbb` (the previous `result_run`) and a `date` on or after `2026-07-01`.

## Red / Green / Refactor example

The `valid` fixture (`internal/eval/testdata/verifylog/valid/`) is a four-iteration set: the `baseline` above followed by one Red, one Green and one Refactor iteration. `verify-log selftest` prints `OK selftest: 4 iterations verified` for it.

Case `basic` has criterion `alpha` and two repeats. Its two checks are `#1 file_exists path=out.txt` (always passes) and `#2 output_contains pattern=/ready/` (the check under test). The runs record these facts:

| Run | Prompt | Suite | Score | `#2 output_contains pattern=/ready/` |
|-----|--------|-------|-------|--------------------------------------|
| `260702-100000-bbbbbbb` | B | S1 | 72.5 | not recorded (no checks) |
| `260706-100000-aaaaa11` | B | S2 | 70.0 | passes in repeat 1, **fails in repeat 2** |
| `260707-100000-bbbbb11` | C | S2 | 85.0 | passes in both repeats |
| `260708-100000-ccccc11` | D | S2 | 85.0 | passes in both repeats |

The front-matter of iterations `01` to `03` (only the front-matter is shown; each log also needs the five body headings with content):

```yaml
# iteration-01.md  (red: add a check that fails on the current prompt)
iteration: 1
date: 2026-07-02
change_type: eval
baseline_run: 260702-100000-bbbbbbb
result_run: 260706-100000-aaaaa11
baseline_score: 72.5
result_score: 70.0
phase: red
```

```yaml
# iteration-02.md  (green: change the prompt so that check passes)
iteration: 2
date: 2026-07-03
change_type: prompt
baseline_run: 260706-100000-aaaaa11
result_run: 260707-100000-bbbbb11
baseline_score: 70.0
result_score: 85.0
phase: green
```

```yaml
# iteration-03.md  (refactor: tidy the prompt, score does not drop)
iteration: 3
date: 2026-07-04
change_type: prompt
baseline_run: 260707-100000-bbbbb11
result_run: 260708-100000-ccccc11
baseline_score: 85.0
result_score: 85.0
phase: refactor
```

What proves each phase:

- **Red (`01`).** `change_type: eval`: the prompt hash is the same (B) and the suite hash differs (S1 to S2). `#2` fails in repeat 2 of the result run, and the baseline run records no failing check, so there is a new failing check. The any-repeat rule makes it failing even though it passed in repeat 1. The score may drop (72.5 to 70.0); `red` has no score rule.
- **Green (`02`).** `change_type: prompt`: the prompt hash changed (B to C) and the suite hash is the same (S2). `#2` failed in the baseline run and passes in both repeats of the result run, and the score rose (70.0 to 85.0).
- **Refactor (`03`).** `change_type: prompt`: the prompt changed (C to D), the suite is the same, and the recorded score stayed at 85.0. An equal score passes.

Each of these checks is also exercised by a failing fixture set; see "Verifying the verifier" in [docs/evaluation.md](../../docs/evaluation.md#verifying-the-verifier).

## Where runs and scores come from

Runs are produced by `kairon eval` and stored, one directory per run, under `.kairon/evals/results/<yymmdd-hhmmss>-<git-hash>/` (older runs may be named by hash only). Each holds `summary.json` and one `<agent>.json` per agent.

- `baseline_run` / `result_run`: the directory name, matched **exactly**. Hash-only shortcuts that `kairon eval diff` accepts are not accepted here, because a log must pin one specific run.
- Scores: take them from the `PASS|FAIL <agent>: NN.N%` line that `kairon eval` prints, or from `summary.json`. The verifier reads the recorded agent score in this order and uses the first one available:
  1. `agent_verdicts.<agent>.score` in `summary.json` (percent, 0-100).
  2. `agent_scores.<agent>` in `summary.json` (a 0-1 fraction; multiplied by 100). Older runs.
  3. Recomputed from the per-case scores in `<agent>.json`. Runs with neither summary field.
- Prompt identity: `prompt_sha256` for the agent, from `<agent>.json`, falling back to `agents.<agent>.prompt_sha256` in `summary.json`. A `--prompt-file` run hashes the candidate file's bytes, so a changed candidate yields a different hash.
- Suite identity and coverage: `suite_sha256`, `complete` and `case_filter` for the agent, from `<agent>.json`, falling back to `agents.<agent>.<name>` in `summary.json`.
  - `suite_sha256` is a hash of the agent's rubric file(s), its case files (`cases/<agent>/*.yaml`) and every workspace fixture file, hidden (`inject`) file, mock script and `setup` file those cases reference. It does **not** cover the agent config, prompt or resources (that is `prompt_sha256`), the models, other agents' files, or fixtures no case references. Editing a case changes it and leaves `prompt_sha256` alone; editing the prompt does the reverse.
  - `complete` is `true` only when every case ran for every `--repeat` and `case_filter` is empty. A run made with `--case` (`kairon eval <agent> <case>`) is **never** complete, even if the agent has a single case, and records the case name in `case_filter`. An interrupted run is saved with `complete: false`.
- Models and mode: `agent_model`, `judge_model` and the sandbox mode (`native` or `container`), from `<agent>.json`, falling back to `summary.json` (`agents.<agent>.agent_model`, `judge_model`, `sandbox`).
- **Commit the runs your logs reference.** Result directories must be checked in under `.kairon/evals/results/`, otherwise the log cannot be verified by anyone else.
- **Runs recorded before run coverage existed fail `run-data`.** They record no `suite_sha256` and no `complete`, so they cannot be verified. Re-record them with `kairon eval <agent>`, commit the new runs, and update `baseline_run`, `result_run` and the scores of every log that referenced the old ones (an iteration's `baseline_run` must still equal the previous iteration's `result_run`). A `change_type: eval` iteration now also needs `prompt_sha256` on both runs, because it has to prove the prompt did not change. The full definitions are in [docs/evaluation.md](../../docs/evaluation.md) under "Run coverage" and "Migration note: run coverage and `verify-log`".

## Candidate prompt files

Candidate prompts for `kairon eval <agent> --prompt-file` conventionally live next to the logs (`.kairon/iterations/<agent>/v2.md`). They may coexist with the `iteration-NN.md` logs; `verify-log` ignores them.

## Verifying a log set

```bash
kairon eval verify-log <agent> [--iterations-dir DIR] [--min-iterations N] [--require-cycle[=false]] [--evals-dir DIR]
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--iterations-dir` | `.kairon/iterations` | Directory containing `<agent>/iteration-NN.md`. |
| `--min-iterations` | `2` | Minimum number of iteration logs required. `0` disables the minimum. |
| `--require-cycle` | `true` | Require a complete Red, Green, Refactor cycle in the set: a `red` iteration followed later by a `green` iteration, and at least one `refactor` iteration. Pass `--require-cycle=false` to turn the requirement off (the per-iteration `phase`, `red`, `green` and `refactor` rules still apply). |
| `--evals-dir` | `.kairon/evals` | Evals directory whose `results/` holds the referenced runs. |

On success it prints `OK <agent>: N iterations verified` and exits 0. Otherwise it reports **all** violations, one per line as `<file>: <rule>: <message>`, and exits non-zero.

### Verification rules

| Rule ID | Fails when |
|---------|------------|
| `no-logs` | The agent directory is missing or holds no `iteration-NN.md` file. Always reported, whatever `--min-iterations` is. |
| `front-matter` | The file does not start with `---`, the front-matter is not valid YAML, a required key is missing, `change_type` is not `prompt` or `eval`, `date` is not a real `YYYY-MM-DD` date, a score is not percent with one decimal in `0.0`-`100.0`, or a run name contains `/`, `\` or `..`. |
| `headings` | One of `Baseline`, `Hypothesis`, `Change`, `Results`, `Reasoning` is missing, duplicated, out of order, or has no content. |
| `sequence` | Numbering is not exactly `00..N-1` (gap, duplicate, not starting at `00`), the front-matter `iteration` differs from the file's `NN`, or a file named like `iteration-1.md` does not match `iteration-NN.md`. |
| `chain` | For iteration `k > 0`, `baseline_run` is not iteration `k-1`'s `result_run`. |
| `chronology` | The `date` of iteration `k` is earlier than that of iteration `k-1`. |
| `run-exists` | `baseline_run` or `result_run` is not a directory under `<evals-dir>/results/`. |
| `run-data` | A referenced run records no score for the agent, or does not record `prompt_sha256`, `suite_sha256` or `complete` for the agent; one violation per run lists every missing field, for both change types. An unverifiable change fails closed. Runs recorded before run coverage existed fail here and must be re-recorded. |
| `run-complete` | A referenced run records `complete: false` for the agent (a `--case` run or an interrupted run). Only full runs can be compared. A run that records no `complete` at all is `run-data` instead. |
| `run-config` | The two runs differ in `agent_model` or `sandbox` (`native` or `container`), or, for `change_type: prompt` only, in `judge_model`. A fact recorded by one run and not the other counts as a difference; recorded by neither is equal. |
| `score-match` | A claimed score differs from the run's recorded score by more than 0.1 percentage points. A difference of exactly 0.1 passes; the tolerance only covers the one-decimal rounding of claimed scores. |
| `prompt-change` | The prompt relation does not match `change_type`, when both runs record `prompt_sha256`. `prompt`: the two runs record the same `prompt_sha256`. `eval`: the two runs record different `prompt_sha256`. |
| `suite-change` | The suite relation does not match `change_type`, when both runs record `suite_sha256`. `prompt`: the two runs record different `suite_sha256`. `eval`: the two runs record the same `suite_sha256` and the same `judge_model` (neither the suite nor the judge changed). |
| `phase` | `phase` is missing, is not one of `baseline`, `red`, `green`, `refactor`, or is `baseline` in a file other than `iteration-00.md`. The message names the key, the bad value and the allowed values. Does not stop the other checks for the same file. |
| `red` | A `red` iteration whose `change_type` is not `eval`, or whose result run has no failing check that the baseline run does not also have (check identity and the any-repeat rule are in [Phases](#phases)). |
| `green` | A `green` iteration whose `change_type` is not `prompt`; or where no check that failed in the baseline run passes in every repeat of the result run; or where the result run's recorded score is lower than the baseline run's. Each failed condition is a separate violation. |
| `refactor` | A `refactor` iteration whose `change_type` is not `prompt`, or whose result run's recorded score is lower than the baseline run's. |
| `cycle` | `--require-cycle` is on and the set has no `red` iteration followed later (higher iteration number) by a `green` iteration, or has no `refactor` iteration. Reported against the agent directory, one violation per missing part. Label-based: it reads the `phase` values of the logs, whether or not those logs pass their own rules. Not reported when there are no logs (`no-logs` covers that). |
| `min-iterations` | Fewer iteration logs exist than `--min-iterations` (default `2`). |

The `red`, `green` and `refactor` rules run only for a log with a valid `phase` and valid other front-matter, and only when both of its runs exist. `refactor` has no check-evidence requirement beyond the score. `baseline` has no rule beyond its position.

Because `verify-log` reports every violation, one run shows everything to fix.

## Load-bearing table

`kairon eval audit <agent>` checks that at least 90% of an agent's instructions are **load-bearing**: each one serves at least one evaluation criterion, and the table names the eval case that proves it. It also rejects instructions that are repeated. The mapping from instruction to criterion and case is a hand-written table at:

```
.kairon/iterations/<agent>/load-bearing.md
```

`load-bearing.md` coexists with the `iteration-NN.md` logs and `verify-log` ignores it, because only files named `iteration-NN.md` are read as logs. The audit command, its flags and its rules are documented in [docs/evaluation.md](../../docs/evaluation.md) under "Prompt Audit (`kairon eval audit`)".

### Table format

The file holds a Markdown table whose header row is exactly:

```markdown
| Instruction | Criterion | Evidence |
```

| Column | Content |
|--------|---------|
| `Instruction` | The instruction text, copied **verbatim** from the prompt or skill (see "Verbatim match" below). |
| `Criterion` | The `name` of a criterion in `<evals-dir>/rubrics/<agent>.yaml`. |
| `Evidence` | Names the eval case that proves the instruction matters, as `case:<case-name>`. Other text is allowed in the cell (for example a short justification). The audit verifies the case; see "Evidence format" below. |

Text outside the table (a title, notes) is allowed and ignored. Rows before the header row are ignored. Every pipe-delimited row after the header is a data row, apart from the `|---|---|---|` separator row. A data row must have exactly three cells, and its `Instruction` and `Criterion` cells must not be empty.

### Evidence format

Every Evidence cell names at least one case with the token `case:<case-name>`, where `<case-name>` is the `name:` of a case of the same agent. Other text in the cell is allowed, so `Covered by case:basic (alpha checks)` and `` `case:basic` `` both name the case `basic`.

The audit verifies each named case:

1. The case exists: some `<evals-dir>/cases/<agent>/*.yaml` file (default `.kairon/evals/cases/<agent>/`; non-recursive, `.yaml` only) has `name: <case-name>`. The case is identified by its `name:` field, not by its file name.
2. The case has at least one `checks:` entry whose `criterion` equals the row's `Criterion`. Any check type counts. A criterion that is scored only by the LLM judge, with no entry under `checks:`, does not prove the instruction.

Token grammar:

- Names are matched exactly and case-sensitively. A name is made of letters, digits, `_`, `.` and `-`; spaces, commas, semicolons, parentheses and backticks end it.
- `case:` must start the cell or follow a character that is not a letter, digit or `_`, so `showcase:basic` does not name a case.
- A trailing `.` or `-` is dropped (sentence punctuation), so `case:basic.` names `basic`. A case whose own name ends in `.` or `-` cannot be referenced.
- A cell may name several cases, for example `case:basic, case:strict`. Each one is verified, and a repeated name is verified once.
- Like every cell, Evidence must not contain an unescaped `|`.

A problem is reported as a `table` violation at the table line (`<table>:<line>: table: ...`), and every row with a non-empty `Instruction` and `Criterion` is checked, including a second row for an instruction that was already mapped. These violations are always reported, whatever the coverage:

| Problem | Message |
|---------|---------|
| No `case:<case-name>` token (including an empty cell) | `row <N>: Evidence must name at least one case of agent "<agent>" as case:<case-name>` |
| The named case does not exist | `row <N>: evidence names case "<name>" which is not a case of agent "<agent>" (looked in <cases-dir>)` |
| The case has no check on the row's `Criterion` | `row <N>: case "<name>" has no check on criterion "<criterion>"` |

If the `Criterion` is not in the rubric, the row is already an `unknown-criterion` problem, so the "no check on criterion" test is skipped for it. A missing `cases/<agent>/` directory means the agent has no cases, so every Evidence reference fails as "not a case". A case file that is not valid YAML, or has no `name:`, stops the audit with an error instead of a violation.

### Example

For an agent `demo` whose rubric `rubrics/demo.yaml` defines the criteria `alpha` and `beta`, and which has one case `basic` with a check on each criterion:

```yaml
# <evals-dir>/cases/demo/basic.yaml
name: basic
agent: demo
input: Do the task.
checks:
  - criterion: alpha
    type: changed_files
    allow: [README.md]
  - criterion: beta
    type: changed_files
    allow: [README.md]
```

```markdown
| Instruction | Criterion | Evidence |
|---|---|---|
| Be careful. | alpha | case:basic |
| \| a \| b \| | alpha | case:basic |
| Use `a \| b` carefully. | beta | case:basic |
| Read the code. | beta | case:basic |
| Do not guess. | beta | case:basic |
```

This table and case are the `fence-and-table` audit fixture (see `internal/eval/testdata/audit/fence-and-table/`): the table is `iterations/demo/load-bearing.md` and the case is `evals/cases/demo/basic.yaml`. Its prompt contains the lines `Be careful.`, `| a | b |`, ``Use `a | b` carefully.``, `Read the code.` and `Do not guess.`. The pipes in the 2nd and 3rd instructions are written `\|` in the table. An `Evidence` cell must not contain an unescaped `|` either, because it would add a fourth cell.

### Rules

- **What counts as an instruction.** Every physical line of the agent prompt, and of each audited engine skill, that is non-blank after trimming, outside a fenced code block, not an ATX heading (`#` to `######`) and not a table separator row (`|---|---|`). Each occurrence counts separately: a line that appears three times is three instructions, and every occurrence after the first is also a `duplicate` violation (see "Duplicates").
- **Front-matter is skipped.** A leading YAML front-matter block (the file starts with `---` and ends at the next `---` line) is metadata, not instructions, and is not counted. This keeps every `SKILL.md` from needing rows for `---` and `name:`. Any other `---` line (a horizontal rule) is an ordinary instruction line.
- **Verbatim match.** An instruction is mapped when its trimmed text equals the trimmed `Instruction` cell exactly. There is no case folding, no whitespace collapsing and no Markdown rendering, so copy the line as it is in the file, bullet markers and backticks included.
- **Escaped pipes.** A `|` inside an `Instruction` cell must be written `\|`, otherwise it ends the cell. The audit turns `\|` back into `|` before it compares.
- **Criterion rule.** The `Criterion` cell must be the `name` of a criterion in `<evals-dir>/rubrics/<agent>.yaml` (default `.kairon/evals/rubrics/<agent>.yaml`). A row that names a criterion that is not in the rubric does not count as mapped.
- **Duplicate rows.** The first row for an instruction wins. A second row for the same instruction with a **different** criterion is a `table` violation. The Evidence cell of every row, including a second row, is verified.
- **Duplicates.** A prompt or skill must not repeat an instruction (Stage 3: no dead weight). An instruction whose trimmed text equals an earlier one is a `duplicate` violation, reported at the repeat as `<file>:<line>: duplicate: duplicate of <first-file>:<first-line>: "<text>"`. "Earlier" means earlier in the prompt, then in the audited engine skills in the order they are audited, so a prompt line repeated in a skill is flagged in the skill. Every repeat names the **first** occurrence, so three copies give two violations. The check is on the exact trimmed text, with the same matching as the table, and it covers any short repeated line, a `---` horizontal rule included. Reword or drop the repeat. Duplicates are always reported, whatever the coverage, and a mapped repeat does not help. The report counts them in `duplicates=<D>`; `total` and `mapped` still count every occurrence.
- **Hooks are excluded.** Skills whose name ends in `-conventions` (for example `builder-conventions`) are per-project override hooks. They are never read, never scored and need no rows. Engine skills that the agent lists in `resources` (for example `sentinel-protocol`) are audited together with the prompt, so their lines need rows too.
- **Threshold.** The audit passes when **at least 90%** of all counted instructions are mapped (`mapped * 100 >= 90 * total`, integer arithmetic). Exactly 90% passes and 89% fails. The percentage is printed truncated to one decimal, so 899 of 1000 shows as `89.9%` and fails. A prompt with no instructions fails.

Put the table next to the iteration logs and update it in the same change as the prompt: a new or reworded instruction needs a new or changed row.

## See also

- [docs/evaluation.md](../../docs/evaluation.md): "Iteration Logs and `verify-log`", "Prompt Audit (`kairon eval audit`)" and "Evaluating a candidate prompt (`--prompt-file`)".
