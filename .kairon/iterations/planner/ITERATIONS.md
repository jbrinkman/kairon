# Planner iterations (issue #397)

Human-readable RED to GREEN log for splitting the planner into two single-mode agents (`planner`,
`planner-automated`) that share the Kairon-managed `planner-shared` skill. This is not an `iteration-NN.md` log:
`kairon eval verify-log` reads only files named `iteration-NN.md`, and it needs committed real-model result
directories to check scores against. None exist yet, so none is written (spec decision D7).

## Method

The planner suite from #380 (`.kairon/evals/rubrics/planner.yaml`, `.kairon/evals/cases/planner/*.yaml`) has seven
cases: four interactive (`interactive-single-decision`, `interactive-option-rules`, `interactive-return-document`,
`multi-issue-split`) and three automated (`automated-best-effort-fill`, `automated-bug-classification`,
`automated-punch-out`). The harness keys a suite by directory name, so every case runs against an agent called
`planner`. The interactive half therefore runs against the real `planner` agent. The automated half runs the same
harness through an `--evals-dir` overlay in which `agents/planner.json` is the `planner-automated` manifest renamed to
`planner`, plus its prompt, so the real `planner-automated` prompt and shared skill are what get evaluated. Both halves
use `kairon eval --sandbox` (the pre-split prompt can run `gh issue create`, so the sandbox's fake `gh` is mandatory).

Two kinds of evidence are recorded, and they are not interchangeable:

- **Offline (stub) tests.** `go test ./internal/eval -run TestPlannerSuite` replays reference transcripts through the
  stub backend. The stub replays the same transcripts whatever the prompt says, so it can check that the suite is
  well-formed and that legacy transcripts fail it (RED). It cannot certify any prompt, so a stub score is never
  evidence of GREEN.
- **Real-model runs.** `kairon eval --sandbox planner` (and the overlay run for the automated half) call a real model.
  Only these can show a prompt passing or failing. A score is recorded here only if it came from a run output or a
  result directory under `.kairon/evals/results/`.

## Iterations

| # | Date | Change | Offline suite (`go test ./internal/eval -run TestPlannerSuite -count=1`) | Real-model run (`kairon eval --sandbox planner`) | Result directories |
|---|------|--------|--------------------------------------------------------------------------|---------------------------------------------------|--------------------|
| 0 (baseline) | 2026-10-22 | none (baseline): unmodified `planner` prompt and manifest at `b319bb4` | PASS (`ok`, all 7 tests). `TestPlannerSuiteRedLegacyTranscriptsFail` printed `FAIL planner: 40.1%`, 7/7 cases failed, criteria below the 60.0% floor: output_document_validity 16.2%, two_mode_behavior 47.6%, best_effort_then_punch_out 41.7%, issue_quality 34.1%, conversation_protocol 23.1%, elicitation_completeness 0.0%, requirements_boundary 50.0%. `TestPlannerSuiteGreenReferenceTranscriptsPass` printed `PASS planner: 100.0%` (stub replay of the reference transcripts; validates the suite, not a prompt). | **Could not execute: no model score.** See "Baseline real run" below. | none kept |

The offline RED is the legacy-transcript test: it shows the suite rejects planner output that has no stage-output
document. The "40.1%" is a stub replay of pre-#380 transcripts, not a measurement of the current prompt.

### Baseline real run

Command: `kairon eval --sandbox planner` (a binary built from this worktree at `b319bb4`, before any prompt change),
run on 2026-10-22. The sandbox container started for each of the seven cases and `kiro-cli 2.28.0` was verified inside
it, but every `kiro-cli chat --agent planner --no-interactive ...` call in the container exited with status 1 after about
10 seconds. The recorded `stderr` was `Failed to open browser for authentication.` The container has no kiro login
and the run cannot open a browser to create one. All seven cases ended as `agent failed` / `no output`, the harness
reported 0.0% on every criterion, and the run ended with `eval failed: 1 of 1 agents below their pass threshold`.

That 0.0% is an infrastructure failure, not a measurement of the planner prompt, so it is **not** recorded as a baseline
score. The result directory the run wrote (`.kairon/evals/results/261022-223555-b319bb4`) holds no model output;
it was discarded so it cannot be mistaken for a baseline in `eval diff` or `eval trend`. Per the task rules, no
credentials were hunted for or injected.

Caveats for whoever repeats the run:

- The working tree has pipeline-injected edits to `.kiro/agents/planner.json` (a `@creds-agent` tool and an `mcpServers`
  block, uncommitted). The harness stages tracked `.kiro/` files from the working tree, so a real baseline run from this
  worktree would evaluate that manifest, not the committed one. The prompt (`planner-prompt.md`) is the committed,
  unmodified one (its hash in the discarded run was `c574f8ef4f5c30a31d3342274f895fef486b6dbbee9f6b49d911a5d68d7193e2`).
- Only the interactive-half command (`kairon eval --sandbox planner`) was attempted. The automated half needs the
  overlay, which does not exist until the `eval-driver` task, and the `planner-automated` agent does not exist yet.
- A real baseline needs an environment where `kiro-cli` is authenticated inside the sandbox container. Until one is
  available, no real-model row can be filled in, and later rows must not be marked GREEN.

## Iteration 1: green-run (real-model run not possible)

| # | Date | Change | Reason | Interactive half (`planner`) | Automated half (`planner-automated` overlay) | Result directories |
|---|------|--------|--------|------------------------------|----------------------------------------------|--------------------|
| 1 | 2026-10-22 | none to the prompts or the skill. The new files were staged by explicit path (`.kiro/skills/planner-shared/`, `.kiro/agents/planner-automated.json`, `.kiro/agents/planner-automated-prompt.md`) so the harness stages the tracked skill from the repository. | Run the real-model eval of the rewritten `planner` and the new `planner-automated`. | **Not possible: no model score.** One probe, `kairon eval --sandbox planner --filter 'name==interactive-single-decision'`, ended `agent failed` / `no output` with `kiro-cli invocation failed: exit status 1`. The recorded stderr says to authenticate with `kiro-cli login --use-device-flow`. This is the same container-authentication failure as the baseline. | **Not run.** The overlay half would hit the same unauthenticated `kiro-cli` in the sandbox container. | none kept. The probe directory `261022-230947-b319bb4` held only the 0.0% infra-failure result and was deleted. |

### Verdict: NOT GREEN. The real-model run was not possible, so real-model certification (AC 9) remains outstanding.

`kiro-cli` is not authenticated inside the sandbox container, so no real-model case could produce output. The 0.0%
shown by the harness for that probe is an infrastructure failure, not a measurement of either prompt, and is not
recorded as a score. No prompt or skill was changed in this task, because there was no failing real-model case to
iterate against. No credentials were searched for or injected.

What was verified instead (offline, none of it certifies a prompt):

- `go test ./internal/eval -run TestPlannerSuite -count=1`: the suite is well-formed, legacy transcripts fail it, and
  the reference transcripts pass it (stub replay).
- `go test ./internal/planner/... -count=1`: static prompt and skill tests, the output boundary, the chain and the
  automated invoker, all against fakes.
- Scoped template diffs for `planner-shared` and the four planner agent files: byte-identical.
- `bash scripts/eval-planner.sh --dry-run` prints both half commands. The shared skill is the tracked
  `.kiro/skills/planner-shared`, not a copy.

To finish AC 9: run `task eval:planner` where `kiro-cli` is logged in inside the sandbox container, record one row per
iteration here, and mark GREEN only when both halves pass.
