# Planner prompt iterations (issue #381)

## Status: pending: live runs not performed

No live `kairon eval --sandbox planner` run has been made against the legacy prompt or against any candidate.
No `iteration-NN.md` log exists in this directory, and no score is recorded anywhere for this work. Acceptance
criterion 8 of issue #381 ("the new prompt passes the planner eval suite, iterations recorded") is **NOT MET**.

## Why: LIVE-EVAL-BLOCKED

The `live-preflight-baseline` task ran the preflight on the unmodified tree (see
`.kairon/artifacts/builder-381-live-preflight-baseline.md`):

| Check | Result |
|-------|--------|
| `kiro-cli --version` (host) | OK, 2.33.1 |
| `kiro-cli whoami` (host) | OK, logged in |
| `podman info`, `docker info` | OK |
| `go run ./cmd/kairon eval --sandbox planner interactive-option-rules` | FAILED, exit 1 |

The failure is in the sandbox. The `kiro-cli` inside the eval container (2.28.0) has no credentials. Its
stderr was `Failed to open browser for authentication. Please try again with: kiro-cli login --use-device-flow`,
the agent exited 1 with no output, and the case reported `agent produced no output; checks not run`. The 0.0%
that run printed comes from the missing output, not from the legacy prompt, so it was not recorded as R0. The
failure is deterministic, so it was not retried.

## What was done instead

The best candidate, `v1.md`, was promoted on the strength of the offline gates only. This is **not** a
certification by the eval suite.

| Offline gate | Result |
|--------------|--------|
| `TestPlannerPrompt` fails against the legacy prompt (forbidden `gh issue create`, missing anchors) | observed to fail |
| `go test ./internal/planner -count=1` with the promoted prompt | pass |
| `KAIRON_PLANNER_PROMPT=.kairon/iterations/planner/v1.md go test ./internal/planner -run TestPlannerPrompt` | pass |
| `go test ./internal/eval -run 'TestPlannerSuite\|TestPlannerBoundary' -count=1` | pass |
| `go run ./cmd/kairon eval --backend stub planner` | PASS 100.0% (7/7). The stub backend replays reference transcripts; it measures the suite, not the prompt |
| Canonical example from the prompt through `kairon planner submit --dry-run` | accepted by the boundary (extract, mode contract, guard chain) |
| Punch-out example through `kairon planner submit --dry-run` | rejected, by design: `submit` is interactive only and the boundary rejects `punch_out` there. Punch-out acceptance is covered by `TestBoundaryAutomatedPunchOutRoutesToTier2` |
| `go test ./... -count=1` | pass |
| No diff under `.kairon/evals/cases`, `rubrics`, `fixtures` | confirmed |

Not done: the by-hand spot-check of each `inferences[].evidence` against the fixtures (decision D2). It needs a
real model reply from `automated-best-effort-fill`, and none could be produced.

## Why `--require-cycle` is off, and what RED evidence is missing

`kairon eval verify-log` requires a `red` iteration by default: the suite changed while the prompt was held
constant. That change was #387 and happened in another PR. No planner suite existed before it, so no run of the
legacy prompt against the planner suite exists, and a `red` log cannot be written honestly here. The RED evidence
this work would have recorded is R0: the legacy prompt (sha256 `a17b25397f97442459ff709baf9f92182d88cf585bbdaaf351e6ae18de5422b3`)
run with `--sandbox`, expected to give an agent FAIL with `output_document_validity` and `two_mode_behavior` near
zero. R0 is **missing**. Until it exists, the baseline log cannot be written, so `verify-log` runs with
`--require-cycle=false`.

## Prompt hashes

| File | sha256 |
|------|--------|
| legacy `.kiro/agents/planner-prompt.md` at `f3d604f` | `a17b25397f97442459ff709baf9f92182d88cf585bbdaaf351e6ae18de5422b3` |
| `v1.md` = promoted `.kiro/agents/planner-prompt.md` = template twin | `7bb83237b8764e96bea3e6a80cc0a77c5dae72a5c2c048705d40d8c6e09fbf12` |

## Maintainer steps to finish

First make `kiro-cli` credentials available inside the eval container (for example `kiro-cli login --use-device-flow`
in the sandbox image, or an API key passed to it). Confirm with a one-case run that produces output:

```bash
go run ./cmd/kairon eval --sandbox planner interactive-option-rules
```

Always pass `--sandbox`: without it the legacy prompt runs a real `gh issue create`. All runs must use the same
sandbox mode.

1. **R0, baseline on the legacy prompt from git history.** The live prompt is already v1, so pass the legacy
   text as a candidate. Use the skill as it was before this PR too if you want a pure baseline
   (`git stash` the skill change, or check out `f3d604f` in a scratch worktree):

   ```bash
   git show f3d604f:.kiro/agents/planner-prompt.md > /tmp/planner-legacy.md
   go run ./cmd/kairon eval --sandbox planner --prompt-file /tmp/planner-legacy.md --repeat 3
   ```

   Expect a FAIL verdict. Note the run directory name and the criteria below the 60% floor.

2. **R1, score the candidate.**

   ```bash
   go run ./cmd/kairon eval --sandbox planner --prompt-file .kairon/iterations/planner/v1.md --repeat 3
   ```

   If it does not pass, edit a new candidate `v2.md` (never the suite) and repeat. Commit every run directory that a
   log will cite.

3. **Section 4.3 check.** For each document-returning case, take the final reply from the result JSON and run
   `go run ./cmd/kairon planner submit --dry-run --repo owner/repo --file <reply>`. Record any reply the eval regexes
   pass and the parser rejects. Read each `inferences[].evidence` in `automated-best-effort-fill` against the
   fixtures by hand and record the result.

4. **Write the logs.** `iteration-00.md` (phase `baseline`, `baseline_run` R0, `result_run` R1, change type `prompt`),
   then `iteration-01.md` and later as `green` (and optionally one `refactor`). Each log's `baseline_run` must equal
   the previous log's `result_run`. Scores are percentages with one decimal and must be copied from the run, not
   typed from memory. Link the logs from this file and change the status line above.

5. **Verify.**

   ```bash
   go run ./cmd/kairon eval verify-log planner --require-cycle=false
   ```

If the promoted prompt differs from the winning candidate, run it once more (Rg) and add it to the chain.
