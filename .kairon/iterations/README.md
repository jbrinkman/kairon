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

The file must start with `---`; the front-matter ends at the next `---` line. All seven keys are required. Unknown extra keys are ignored.

| Key | Format | Meaning |
|-----|--------|---------|
| `iteration` | integer, equal to `NN` in the file name | Position of this log in the sequence. |
| `date` | `YYYY-MM-DD`, a real calendar date | When the iteration was measured. Dates must not go backwards from one iteration to the next. |
| `change_type` | `prompt` or `eval` | `prompt`: the agent prompt changed, so the two runs must record different `prompt_sha256`. `eval`: the rubric, cases or judge changed; the prompt may be identical. |
| `baseline_run` | name of a directory under `<evals-dir>/results/` | The run measured before the change. A single path element: no `/`, `\` or `..`. |
| `result_run` | same | The run measured after the change. |
| `baseline_score` | percent, exactly one decimal, `0.0`-`100.0` (`72.5`, `100.0`, `5.0`) | The agent score of `baseline_run`. Not `85`, not `97.25`, not `101.0`. |
| `result_score` | same | The agent score of `result_run`. |

Scores are **percent**, not fractions: write `72.5`, not `0.725`.

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

This is `.kairon/iterations/selftest/iteration-00.md`. It is consistent with the `valid` fixture used by the verifier tests.

````markdown
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

## Where runs and scores come from

Runs are produced by `kairon eval` and stored, one directory per run, under `.kairon/evals/results/<yymmdd-hhmmss>-<git-hash>/` (older runs may be named by hash only). Each holds `summary.json` and one `<agent>.json` per agent.

- `baseline_run` / `result_run`: the directory name, matched **exactly**. Hash-only shortcuts that `kairon eval diff` accepts are not accepted here, because a log must pin one specific run.
- Scores: take them from the `PASS|FAIL <agent>: NN.N%` line that `kairon eval` prints, or from `summary.json`. The verifier reads the recorded agent score in this order and uses the first one available:
  1. `agent_verdicts.<agent>.score` in `summary.json` (percent, 0-100).
  2. `agent_scores.<agent>` in `summary.json` (a 0-1 fraction; multiplied by 100). Older runs.
  3. Recomputed from the per-case scores in `<agent>.json`. Runs with neither summary field.
- Prompt identity: `prompt_sha256` for the agent, from `<agent>.json`, falling back to `agents.<agent>.prompt_sha256` in `summary.json`. A `--prompt-file` run hashes the candidate file's bytes, so a changed candidate yields a different hash.
- **Commit the runs your logs reference.** Result directories must be checked in under `.kairon/evals/results/`, otherwise the log cannot be verified by anyone else.

## Candidate prompt files

Candidate prompts for `kairon eval <agent> --prompt-file` conventionally live next to the logs (`.kairon/iterations/<agent>/v2.md`). They may coexist with the `iteration-NN.md` logs; `verify-log` ignores them.

## Verifying a log set

```bash
kairon eval verify-log <agent> [--iterations-dir DIR] [--min-iterations N] [--evals-dir DIR]
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--iterations-dir` | `.kairon/iterations` | Directory containing `<agent>/iteration-NN.md`. |
| `--min-iterations` | `2` | Minimum number of iteration logs required. `0` disables the minimum. |
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
| `run-data` | A referenced run records no score for the agent, or (for `change_type: prompt`) records no `prompt_sha256` for the agent. An unverifiable change fails closed. |
| `score-match` | A claimed score differs from the run's recorded score by more than 0.1 percentage points. A difference of exactly 0.1 passes; the tolerance only covers the one-decimal rounding of claimed scores. |
| `prompt-change` | For `change_type: prompt`, `baseline_run` and `result_run` record the same `prompt_sha256` for the agent. |
| `min-iterations` | Fewer iteration logs exist than `--min-iterations` (default `2`). |

Because `verify-log` reports every violation, one run shows everything to fix.

## Load-bearing table

`kairon eval audit <agent>` checks that at least 90% of an agent's instructions are **load-bearing**: each one serves at least one evaluation criterion. The mapping from instruction to criterion is a hand-written table at:

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
| `Evidence` | Free text saying why removing the instruction would lower that criterion's score. The column must exist, but the audit does not check its content: it is the justification a human reviewer reads. |

Text outside the table (a title, notes) is allowed and ignored. Rows before the header row are ignored. Every pipe-delimited row after the header is a data row, apart from the `|---|---|---|` separator row. A data row must have exactly three cells, and its `Instruction` and `Criterion` cells must not be empty.

### Example

For an agent `demo` whose rubric `rubrics/demo.yaml` defines the criteria `alpha` and `beta`:

```markdown
| Instruction | Criterion | Evidence |
|---|---|---|
| Be careful. | alpha | Cases that skip verification lose points on alpha. |
| \| a \| b \| | alpha | Escaped pipes: the prompt line is a literal pipe-delimited `a`/`b` row. |
| Use `a \| b` carefully. | beta | Escaped pipe inside inline code in the prompt line. |
| Read the code. | beta | Cases that answer without reading the code lose points on beta. |
| Do not guess. | beta | Guessed answers fail the beta checks. |
```

This table is modelled on the `fence-and-table` audit fixture (see `internal/eval/testdata/audit/fence-and-table/`). Its prompt contains the lines `Be careful.`, `| a | b |`, ``Use `a | b` carefully.``, `Read the code.` and `Do not guess.`. The pipes in the 2nd and 3rd instructions are written `\|` in the table. An `Evidence` cell must not contain an unescaped `|` either, because it would add a fourth cell.

### Rules

- **What counts as an instruction.** Every physical line of the agent prompt, and of each audited engine skill, that is non-blank after trimming, outside a fenced code block, not an ATX heading (`#` to `######`) and not a table separator row (`|---|---|`). Each occurrence counts separately: a line that appears three times is three instructions.
- **Front-matter is skipped.** A leading YAML front-matter block (the file starts with `---` and ends at the next `---` line) is metadata, not instructions, and is not counted. This keeps every `SKILL.md` from needing rows for `---` and `name:`. Any other `---` line (a horizontal rule) is an ordinary instruction line.
- **Verbatim match.** An instruction is mapped when its trimmed text equals the trimmed `Instruction` cell exactly. There is no case folding, no whitespace collapsing and no Markdown rendering, so copy the line as it is in the file, bullet markers and backticks included.
- **Escaped pipes.** A `|` inside an `Instruction` cell must be written `\|`, otherwise it ends the cell. The audit turns `\|` back into `|` before it compares.
- **Criterion rule.** The `Criterion` cell must be the `name` of a criterion in `<evals-dir>/rubrics/<agent>.yaml` (default `.kairon/evals/rubrics/<agent>.yaml`). A row that names a criterion that is not in the rubric does not count as mapped.
- **Duplicate rows.** The first row for an instruction wins. A second row for the same instruction with a **different** criterion is a `table` violation.
- **Hooks are excluded.** Skills whose name ends in `-conventions` (for example `builder-conventions`) are per-project override hooks. They are never read, never scored and need no rows. Engine skills that the agent lists in `resources` (for example `sentinel-protocol`) are audited together with the prompt, so their lines need rows too.
- **Threshold.** The audit passes when **at least 90%** of all counted instructions are mapped (`mapped * 100 >= 90 * total`, integer arithmetic). Exactly 90% passes and 89% fails. The percentage is printed truncated to one decimal, so 899 of 1000 shows as `89.9%` and fails. A prompt with no instructions fails.

Put the table next to the iteration logs and update it in the same change as the prompt: a new or reworded instruction needs a new or changed row.

## See also

- [docs/evaluation.md](../../docs/evaluation.md): "Iteration Logs and `verify-log`", "Prompt Audit (`kairon eval audit`)" and "Evaluating a candidate prompt (`--prompt-file`)".
