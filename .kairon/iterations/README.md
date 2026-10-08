# Iteration Logs

An iteration log records one measured change to an agent: what the scores were, what you expected to
happen, what you changed, what the scores became, and why. Logs are written by agents and humans, and
every number in them is checked against committed eval results by `kairon eval verify-log`.

> No real iteration log is shipped by this change. This directory contains only this README; the example
> below is illustrative and its run names do not exist under `.kairon/evals/results/`.

## Layout

```
.kairon/iterations/<agent>/iteration-NN.md
```

- `<agent>` is the agent name used in `kairon eval run` (a single directory name, for example `builder`).
- `NN` is a zero-padded iteration number with at least two digits. Numbering starts at `00` and is
  contiguous: `iteration-00.md`, `iteration-01.md`, `iteration-02.md`, ...
- Iterations form a chain: each iteration starts from where the previous one ended
  (`baseline_run` of iteration N is `result_run` of iteration N-1).

## Front-matter

Each file starts with a YAML block delimited by a first line of `---` and a closing line of `---`.

| Key | Rule |
|-----|------|
| `iteration` | Integer. Must equal `NN` in the file name. |
| `date` | `YYYY-MM-DD`. Must not be earlier than the previous iteration's date. |
| `change_type` | `prompt` or `eval`. Use `eval` for a rubric, case or fixture change where the agent prompt is not expected to change. |
| `baseline_run` | Exact run directory name under `<evals-dir>/results/` (see below). |
| `result_run` | Exact run directory name under `<evals-dir>/results/`. |
| `baseline_score` | Percent for the agent in `baseline_run`, written with exactly one decimal (`60.0`, `87.5`, `100.0`). Range 0.0-100.0. |
| `result_score` | Percent for the agent in `result_run`, same format. |

Unknown extra keys are ignored.

### Exact run names

`baseline_run` and `result_run` must be the **exact directory names** of runs committed under
`.kairon/evals/results/`, for example `260701-100000-aaaaaaa` (`<YYMMDD-HHMMSS>-<git-hash>`). A bare git
hash is not accepted, and a name may not contain `/`, `\` or `..`. This pins each claim to one specific,
reviewable run. Commit the runs you reference.

### How scores are checked

The claimed percent is compared with `agent_scores[<agent>]` in the run's `summary.json`, multiplied by 100.
A difference of up to 0.1 percentage points is allowed.

## Body

Five level-2 headings, each present exactly once, in this order, each with non-empty content:

| Heading | Content |
|---------|---------|
| `## Baseline` | The numbers being improved, taken from `baseline_run`. |
| `## Hypothesis` | A falsifiable prediction of what the change will do to the scores. |
| `## Change` | The exact change made (diff summary or commit). |
| `## Results` | The measured numbers from `result_run`, per criterion if useful. |
| `## Reasoning` | Why the results confirm or refute the hypothesis, and what to try next. |

Headings inside fenced code blocks are ignored.

## Verifier rules

Each violation is printed as `<file>: [<rule>] <message>`. All violations are collected and reported together.

| Rule id | Fails when |
|---------|-----------|
| `front-matter` | Front-matter is missing, unterminated or invalid YAML; a required key is missing or has the wrong type; `date` is not `YYYY-MM-DD`; `change_type` is not `prompt` or `eval`; a score is not written as `N.N` or is outside 0-100; a run name is empty or not a single path element. |
| `headings` | A required heading is missing, duplicated, out of order, or has empty content. |
| `numbering` | Iteration numbers are not contiguous from `00`; `iteration:` differs from the file name; a file named `iteration-*` does not match `iteration-NN.md`. |
| `chain` | Iteration N's `baseline_run` is not iteration N-1's `result_run`. |
| `date-order` | An iteration's `date` is earlier than the previous iteration's. |
| `run-missing` | `baseline_run` or `result_run` has no `<evals-dir>/results/<run>/summary.json`. |
| `run-no-score` | The run exists but its `summary.json` has no `agent_scores` entry for the agent. |
| `score-mismatch` | A claimed score differs from `agent_scores[<agent>]` x 100 by more than 0.1. |
| `prompt-unchanged` | `change_type: prompt`, but both runs record the same `prompt_sha256` for the agent. |
| `prompt-unrecorded` | `change_type: prompt`, but either run records no `prompt_sha256` for the agent, so the claim cannot be checked. |
| `min-iterations` | Fewer iterations than `--min-iterations` (default 2), or the agent has no iterations directory. |

`change_type: eval` iterations are exempt from `prompt-unchanged` and `prompt-unrecorded`.

## Running the verifier

```bash
kairon eval verify-log <agent> [--iterations-dir DIR] [--evals-dir DIR] [--min-iterations N]
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--iterations-dir` | `.kairon/iterations` | Directory containing `<agent>/iteration-NN.md`. |
| `--evals-dir` | the eval default (`.kairon/evals`) | Directory containing `results/<run>/`. |
| `--min-iterations` | `2` | Minimum number of iterations required. |

On success it prints `✅ <agent>: N iteration(s) verified` and exits 0. On violations it prints each one to
stderr and exits non-zero.

## Example

A complete iteration file (`.kairon/iterations/builder/iteration-00.md`). The run names are illustrative.

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

Run `260701-100000-aaaaaaa` scored 60.0% for the builder agent. The weakest criterion was
`tests-first`, which passed in only 2 of 8 cases.

## Hypothesis

The prompt never tells the builder to write a failing test before the implementation. Adding an explicit
test-first step will raise `tests-first` to at least 6 of 8 cases and the overall score by at least 10
percentage points.

## Change

Added a "Test first" step to the builder prompt: write a failing test, confirm it fails for the expected
reason, then implement. No other instructions were touched.

## Results

Run `260702-100000-bbbbbbb` scored 72.5% (+12.5 points). `tests-first` passed in 7 of 8 cases; the other
criteria were unchanged within noise.

## Reasoning

The gain is concentrated in `tests-first`, which supports the hypothesis that the missing instruction was the
cause. The overall improvement exceeded the predicted 10 points. Next iteration: look at the remaining
failing criterion, `minimal-diff`.
````
