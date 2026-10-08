# Kairon AI Maturity Model — Gap Analysis

**Analysis Date:** 2026-10-04 (Stage 3 re-assessed 2026-10-05)
**Assessed Against:** Improving's AI Maturity Framework (Stages 3, 4, 5)

> This document assesses Kairon's current implementation against each Stage 3, 4, and
> 5 criterion. Each criterion is marked as: ✅ **Implemented**, 🟡 **Partial**, or
> ❌ **Not Implemented**. For partial/not-implemented items, a ready-to-use planner
> prompt is included to create the necessary GitHub issue(s).
>
> All paths are repo-relative.

---

## Stage 3 — Task Certification

**Re-assessed:** 2026-10-05, against `main` @ `2fca610` plus open PRs and issues.
**Assessed against:** `.kairon/specs/maturity-model/stage-3.md`

Stage 3 evaluates prompt design, evaluation infrastructure, and iteration discipline.

### S3.0 Where this differs from the 2026-10-04 assessment

| Topic | 2026-10-04 said | Revised | Why |
|-------|-----------------|---------|-----|
| Biggest gap | The prompts need an EDD rebuild | **The eval harness** can't produce substantive, safe, repeatable scores, and that blocks every agent rebuild | §S3.2 |
| Existing evals | ✅ exist | They exist, but ❌ aren't substantive, and several are unsafe to run | §S3.2 H1–H4 |
| Case counts | builder 5, validator 4, architect 4, documenter 3, krew-lead 6, planner "TBD" | builder 5, validator 5, architect 7 (2 of them test the harness, not the architect), documenter 5, krew-lead 5, planner: rubric only, **0 cases**. The planner cases from #108 were deleted in #122 | `ls .kairon/evals/cases/*`, `git log` |
| Mid-tier prerequisite | Pin agents to claude-sonnet-4; CI enforces it | The agents already pinned mid-tier models (sonnet-4 / sonnet-4.5), and on 2026-10-05 all six moved to `claude-sonnet-5.5`, the highest model in the Sonnet (mid-tier) family. The **judge** is the unpinned call: it runs on the account default, `auto`. Models aren't recorded in results either. CI can't run evals (no LLM credentials), so enforce this with unit tests plus recorded provenance | §S3.2 H5, H7 |
| Load-bearing proof | Per-instruction ablation | A traceability table (instruction → criterion → check), verified by a tool, plus ablation at the section level. Per-instruction ablation costs one full eval run per line, and LLM-judge noise makes the results unreliable | §S3.5 |
| Where iteration happens | Rebuild the live prompt in one PR | Iterate on a **candidate file**, with one issue/PR per iteration and a final promotion PR. Editing the live prompt self-modifies the running pipeline: krew-lead spawns builder and validator from the worktree's `.kiro/agents/` | §S3.5 |
| What counts as "the prompt" | `<agent>-prompt.md` | The **certified agent** is the shipped prompt plus the shipped engine skills (`sentinel-protocol`). `*-conventions` resources are project override hooks: they may be absent, and `kairon init` never overwrites them. They aren't part of the certified agent. Evals run with the hooks empty, plus one case per agent that supplies a fixture override to prove the hook is honored. Kairon's own overrides (planner-conventions 7.9 KB, validator-conventions 29 KB) are project content and get reviewed separately | §S3.3, §S3.5 |
| Phase 2 size | One issue per agent | Too open-ended for lower-end models, so split it per iteration | §S3.6 |
| Threshold | "default 80%" (in the Builder Phase 1 prompt) | 95% per agent, enforced by exit code | stage-3.md §2 |
| Checklist | "All 5 agents" | 6 agents (planner included) | — |
| Readiness | ~20% | **~10%**. Prompts exist and show some deliberate design. Nothing in Evaluation or Iteration is met yet | §S3.1 |
| Stage 5 constraint | Not considered | Stage 5 needs direct LLM API calls instead of spawning kiro-cli, so the Stage 3 eval design must be harness-agnostic | §S3.5 |

---

### S3.1 Criterion-by-criterion assessment

| # | Criterion | Status | Evidence (verified 2026-10-05) |
|---|-----------|--------|--------------------------------|
| **1.1** | Structured prompting (RTCC or similar) | 🟡 Partial | All six prompts use ad-hoc headings (Purpose / Instructions / Workflow / Report; the planner uses Restrictions / Gates). None of them separates Role / Task / Context / Constraints, and context is either implicit or buried in skills. |
| **1.2** | Deliberate instruction design | 🟡 Partial | Some design is clearly deliberate: the planner gates, the validator's PR #238 anti-pattern, the sentinel task-id rationale. There are also contradictions. The planner prompt says its "ONLY permitted shell commands are `gh issue create`, `cat`…" but Gate 1 tells it to run the planning-worktree scripts. The base prompt also depends on content that exists only in Kairon's override skill (e.g. the builder's `.kairon/artifacts/qa-tools.md` path). The validator tells a Go project to run `npm test` / `npm run lint`. The validator's "exit with code 1" is something the agent can't do. The architect prompt still says "Kiro-krew". |
| **1.3** | ≥90% of instructions load-bearing | ❌ | This has never been measured, and no instruction → criterion map exists. Most distinctive rules have no eval that would notice if they were removed: builder sentinel and validator-feedback handling, planner label gate, single-question rule (no planner cases). |
| **1.4** | No dead weight or filler | ❌ | The architect repeats the single-PR rule in 4 sections. The builder says "cd into the working directory" twice. The validator carries npm commands that don't apply to this project. The planner has `web_search`/`web_fetch` tools that its prompt never mentions. (Missing `*-conventions` skills are **not** dead weight: they're the intended project-override hook.) |
| **2.1** | ≥3 distinct eval criteria | 🟡 Partial | Every rubric has 4–6 criteria (the planner's has 6 but 0 cases). They're distinct on paper, but several deterministic ones reduce to the same stdout keyword scan (H3). |
| **2.2** | Substantive checks, not window dressing | ❌ | No check inspects files, diffs, builds or test results. Cases target fictional or stale code. (H3, H4) |
| **2.3** | Capable of producing 95%+ | ❌ | Only the architect has ever been scored: best full run 81% (2026-06-21), best single case 19/20. The validator was scored on 1 case (65%). Builder, documenter, krew-lead and planner have never been run. No run since 2026-06-24 has produced agent output. |
| **2.4** | Runs on mid-tier or lower | 🟡 Partial | All six agent JSONs pin `claude-sonnet-5.5` (moved 2026-10-05 from sonnet-4 / sonnet-4.5; Sonnet is the top mid-tier family). The judge runs `kiro-cli chat --no-interactive` with no `--model`, so it uses the account default, `auto` (unknown, possibly frontier). Results don't record which model was used. |
| **2.5** | Clear pass/fail thresholds | 🟡 Partial | The per-case `min_score` (default 80%) is printed but never enforced. There's no agent-level threshold, `kairon eval` always exits 0, and the judge's `pass` field is ignored. |
| **3.1** | ≥2 documented iterations | ❌ | No iteration log exists anywhere. |
| **3.2** | Each iteration has baseline / hypothesis / change / results / reasoning | ❌ | — |
| **3.3** | EDD Red/Green/Refactor | ❌ | Prompt changes have shipped without eval runs. For example, #108 added planner rules and planner cases, and #122 later deleted the cases. |
| **3.4** | Chronological cause and effect | ❌ | 18 result dirs exist, but they're keyed only to git HEAD. Nothing ties a score to a prompt version or to a change. |

---

### S3.2 Eval harness findings (the main blocker)

| ID | Finding | Evidence |
|----|---------|----------|
| **H1** | **Unsafe to run.** Native mode runs `kiro-cli chat --agent <a> --no-interactive --trust-all-tools` in the **repo root** against the **real `gh`**. Builder cases would edit the live tree. `krew-lead/basic-orchestration` asks for the full workflow, including PR creation on `jbrinkman/kairon`. Planner cases would call the real `gh issue create`. | `internal/eval/runner.go` `invokeAgentNative` (~L781); no `cmd.Dir`; gh mock is only wired into the Docker path |
| **H2** | **The Docker sandbox never sends the case prompt.** The exec argv has no input and `ExecWithOutput` attaches no stdin. Every run since 2026-06-24 has empty output. The 2026-06-23 run shows `kiro-cli: executable file not found in $PATH` inside the container. | `runner.go` ~L719; `internal/eval/sandbox/container.go` `ExecWithOutput`; `.kairon/evals/results/2606{23..28}-*` |
| **H3** | **Deterministic scorers are keyword heuristics** on stdout, selected by criterion *name*. `completeness` checks for `## ` and `### `. `code_correctness` checks whether the output *says* "build passes". `test_execution` checks for `$ ` or `PASS`. `test_coverage` checks whether `_test.go` is mentioned. All of them can be satisfied by the agent describing work it never did. | `runner.go` `scoreDeterministic` |
| **H4** | **Cases are fictional or stale.** Builder cases target `internal/watcher/github.go` and `internal/manager/manager.go` (neither exists), a 60 fps TUI loop (`tui.go` ticks every 200 ms), and a status command that already exists (`handleStatus`, `internal/tui/commands.go:56`). Validator cases "verify" JWT auth and CPU measurements that aren't in the workspace, and none states the expected verdict. Krew-lead expected outputs contradict the current prompt (worktree creation, merge cleanup, phased plans). `fixtures/codebase-context.md` lists `internal/manager/` and Go 1.21. | `.kairon/evals/cases/**`, `.kairon/evals/fixtures/*` |
| **H5** | **The judge is unpinned and sampled once.** There's no `--model` flag and one call per criterion, with no repeats. The 1–5 scale is clamped (`max(1, …)`), so a total miss still scores 20%. | `runner.go` `runKiroCLI`, `scoreLLMJudge` |
| **H6** | **Scoring integrity.** Skipped criteria are dropped from the denominator, so a judge parse failure can *raise* the score. The judge's `pass` is ignored. Agent results never get a pass/fail verdict, and the exit code is always 0. `summary.TotalCost` is overwritten per agent instead of accumulated. | `runner.go` `buildSummary`, `updateIncrementalSummary` |
| **H7** | **No provenance.** Results record only git HEAD: no agent model, no judge model, nothing identifying the prompt text that was evaluated. Uncommitted prompt edits can't be told apart. | `types.go` `AgentResult`, `Summary` |
| **H8** | **No multi-turn support.** The planner is a gated conversation, so single-shot cases can't test the approval or label gates. *Verified 2026-10-05:* `kiro-cli chat --no-interactive --resume "<msg>"` continues the previous conversation in the same directory, so multi-turn is viable. | probe run |
| **H9** | **Duplicated scoring code.** `evaluate()` and `evaluateProgressive()` each implement case scoring, so every scoring change has to be made twice. That's a drift risk for lower-end builder models. | `runner.go` |
| **H10** | Cost is a chars÷4 estimate, not real usage. This isn't a Stage 3 criterion, but Stage 4/5 need real token data. | `runner.go` `estimateCost` |

---

### S3.3 Prompt findings

**Planner** (`planner-prompt.md` 7.0 KB; Kairon's override `planner-conventions` adds 7.9 KB)
- Strengths: explicit gates, a one-question rule backed by examples, a clear role restriction.
- Contradiction inside the base prompt: it allows "ONLY" `gh issue create`/`cat`/reading config, but tells the planner to run `.kairon/scripts/planning-worktree-*.sh`.
- Kairon's override (project content, not part of the certified agent): mostly generic root-cause-analysis methodology (3–5 hypotheses, load simulation, memory leaks), and it allows code changes in a planning worktree that the base prompt forbids. Because it loads on every Kairon planning session, it will affect the issues the planner writes for this program. Review it separately: keep only Kairon-specific guidance there, and move anything generic and load-bearing into the base prompt.
- **Missing output standard for tight issues.** The draft template (Problem, User Story, ACs, Constraints, Context) has no scope boundary, no Out of Scope list and no verification per AC, which is what keeps lower-end builder models from drifting. *Decided 2026-10-05:* adopt the "Planner issue standard" (§S3.6, Step 1). The generic standard goes in the base prompt and Kairon specifics in the override. The issue stays a requirements document, and design is left to the architect. The existing guideline "focus on the problem space, not implementation details" is consistent with this and stays.
- The rubric keeps `acceptance_criteria_quality` as a keyword heuristic (it counts `- [ ]`, `go test`, …).

**Builder** (`builder-prompt.md` 2.8 KB + engine skill `sentinel-protocol` 3.1 KB; Kairon's override `builder-conventions` adds 4.3 KB)
- Strengths: short, single-task focus, a defined report format.
- Gaps: no Context/Constraints separation. "cd into the working directory" appears twice. The report format and the sentinel content overlap. The base prompt must be complete without the override; today the `.kairon/artifacts/qa-tools.md` location appears only in Kairon's override.
- Template sync is Kairon-specific behavior and correctly lives in the override. Test it as an override case: the fixture supplies its own `builder-conventions`, which also proves the hook works.
- None of the builder's distinctive base rules has an eval: one task with no scope expansion, the task-scoped sentinel path, validator-feedback handling.

**Other agents (for later):** Kairon's `validator-conventions` override is 29 KB, nearly 3× the validator prompt, so review it before validator evals. The validator prompt tells a Go project to run npm, and its exit-code contract is unenforceable (engine enforcement belongs to Stage 4). The architect's single-PR rule is repeated 4×. The documenter writes to `app_docs/`, which is **gitignored**, so its output never lands in a PR; decide that before writing documenter evals. Krew-lead's workflow steps never dispatch the documenter.

---

### S3.4 Open PRs and issues that touch this work

| Item | What it is | Recommendation |
|------|-----------|----------------|
| PR #263 / issue #116 | Improvement tracking (baseline, trend, report; +2,741 lines) | **PR closed 2026-10-05** as superseded by E11. It targeted pre-rename paths and edited `runner.go`/`types.go`, which E1–E7 rewrite. Stage 3 needs *verifiable iteration logs*, not trend reports. The branch is kept. Issue #116 is still open. |
| PR #194 / issue #192 | Docker sandbox: move file setup to build time | An earlier attempt at a container sandbox. **PR closed 2026-10-05.** It was pre-rename and didn't fix H2 (prompt never sent). Both are **superseded by the container sandbox series #296–#300** (prompt and backend routing, tools-only base image and mounts, fake `gh` / read-only filesystem / tool trust, mocks, provenance and scoring parity). The branch is kept. Neither the PR nor the issue is reopened or relabelled by this work. Issue #192 is still **open** (checked 2026-10-08 with `gh issue view 192`; it carries the pre-rename `kiro-krew-done` label). |
| Issue #115 | "Add context support to eval test cases" | **Closed 2026-10-05 as completed.** It was already implemented (`TestCase.Context`, commit `d93682a`). Its "planner example" AC is covered by P2. |
| #271, #262/#264, #232, #270 | TUI and planning-tab UX, multi-project support | No file overlap with Stage 3 work. Note that #262 and #264 are duplicate PRs for #211. |
| Labels | #116, #192, #211, #231, #266 are open with the pre-rename `kiro-krew-done` label | Housekeeping only. |

---

### S3.5 Approach (revised)

**Keep:** an EDD rebuild per agent, in the order planner → builder → validator → architect → documenter → krew-lead, with the existing prompts treated as reference material.

**Changes:**
1. **Harness first (Step 0).** No agent work beyond the docs-only spec issues starts until cases can run safely and score real artifacts.
2. **Behavior inventory before evals.** Phase 1 lists every instruction in the legacy prompt *and its skills*, then marks each Keep (it needs a check) or Drop. That way hard-won lessons become checks instead of being lost in the teardown, and the inventory seeds the load-bearing table.
3. **Candidate file, not the live prompt.** Iterations edit `.kairon/iterations/<agent>/candidate-prompt.md` and are evaluated with `--prompt-file`. The live `.kiro/agents/<agent>-prompt.md` changes only in the promotion PR, which also keeps `legacy-prompt.md` for rollback.
4. **Iteration 0 records two scores:** legacy prompt and minimal RTCC skeleton. This proves the rebuilt prompt beats what the pipeline runs today.
5. **One issue per iteration,** created only after reading the previous iteration's results (the hypothesis depends on them). The minimum is 2 iterations, and at least one must be a Refactor (removing something without a score drop).
6. **Scoring model.** Each rubric criterion is a set of **pass/fail checks**, either deterministic or a yes/no judge question, and criterion score = passed ÷ total. An agent passes at **≥95%**, and the exit code reflects it. That gives you clear thresholds, and 95% stays meaningful.
7. **Isolation.** Cases that can cause side effects run under `--sandbox` in a container: read-only root filesystem, a fake `gh` in a read-only mount (the real `gh` is unauthenticated), whole-tool trust per agent, and author-supplied mocks for anything else. Every case still runs in a temp, git-initialized copy of a fixture workspace and never in the repo root. Native runs are not contained, so `requires_sandbox: true` makes a case refuse to run natively. This replaces the earlier convention-based design (E4: `gh` shim on PATH plus an isolated `GH_CONFIG_DIR`); see the superseded E4 record in §S3.6 and the container sandbox series #296–#300.
8. **Load-bearing proof.** Prompts put one instruction per line under `## Role` / `## Task` / `## Context` / `## Constraints`. `load-bearing.md` maps each line → criterion → case/check, and `kairon eval audit` computes the percentage over the prompt plus engine skills (override hooks excluded). One Refactor iteration does section-level ablation as supporting evidence.
9. **Anti-fabrication.** Iteration logs cite committed result runs, and `kairon eval verify-log` checks the claimed scores against the results JSON. This matters because lower-end models will be writing these logs.
10. **Override hooks.** Eval workspaces get `.kiro/` exactly as `kairon init` installs it (embedded templates), so `*-conventions` hooks are empty unless a case fixture supplies one. Each agent gets at least one override case that proves project guidance in the hook is followed.
11. **Issue standard (agreed 2026-10-05).** The issue is the requirements document (problem space). The architect's spec is the design and task list (solution space). This mirrors SDD frameworks such as Kiro specs (requirements → design → tasks). Binding technical decisions (a mandated package, pattern or interface) go in Constraints with a reason and verification; non-binding hints go in Context. The generic standard ships in the base planner prompt, and Kairon-specific guidance goes in Kairon's `planner-conventions` override. See "Planner issue standard" under Step 1 in §S3.6.
12. **Planner design (agreed 2026-10-05).** The planner investigates before asking, tracks a coverage map, asks one decision per turn with a recommended option, offers a use-the-recommendations escape hatch and one confirm-defaults turn, and leaves no unconfirmed assumptions. It is evaluated with state-snapshot cases plus answer-bank simulated-user cases (E14). See "Planner design" under Step 1 in §S3.6.
13. **Sub-agent inputs.** Rebuilt sub-agent prompts get their inputs (issue, spec, task, validator feedback) from the dispatch message or workspace files, and never fetch them from GitHub themselves. The planner is the exception, since it's the intake. Required by a dispatch-only coordinator with narrow tool manifests (Stage 5 §1–2), and it keeps evals hermetic.
14. **Agent definition format.** `.kiro/agents/<agent>.json` (prompt, resources including override hooks, tools, model) stays Kairon's agent definition format, and the Stage 4/5 custom harness reads it. Stage 3 provenance, evals and hooks then remain valid. See §S3.8.

**Stage 5 compatibility** (direct LLM API; kiro-cli kept as a user option):
- All agent and judge inference goes through one pluggable backend (E1: `--backend kiro-cli|stub`). kiro-cli is the only model backend in Stage 3, and a direct-API backend (`--backend api`) drops in for Stage 5. The stub backend makes harness behavior verifiable without model calls. The **judge** is plain text in/text out, so it's the cheapest first API backend: exact model pin, temperature 0, real token usage. That's optional as E13 (provider TBD).
- Checks score **artifacts** (workspace files, git diff, fake-`gh` log, the final assistant message), never kiro-cli's stdout formatting or tool traces. The same cases then certify both harnesses, which Stage 5's "Stage 3 guarantees maintained" requires.
- Multi-turn cases are data (`turns:`). kiro-cli implements them with `--resume`, and the API backend will use message history.
- Rebuilt prompts should be **self-contained**: put load-bearing generic guidance in the prompt, and describe capabilities ("run a shell command") rather than kiro-cli tool names (`subagent`, `todo_list`). The API harness must reproduce the `resources` semantics: load an engine skill, load an override skill if present, and silently skip a missing override. The audit covers the certified context, and results record a hash of it plus which override hooks were present.
- Stage 3 certification on the kiro-cli backend is acceptable. Stage 5 re-runs the same suites through the API backend.

---

### S3.6 Issue breakdown

How to use: paste one prompt at a time into the Kairon planner. **Replace `E#`/`P#`/`B#` references with real issue numbers** as issues get created.

Every prompt follows the "Planner issue standard" (Step 1 below): it states requirements only, scope is a package-level boundary, and `Verify:` lines are black-box. Harness issues verify through the deterministic stub self-test that E1 introduces (`--backend stub`, `task eval:selftest`), so their checks need no model calls. Design (internal names, new files, test approach) is left to the architect.

#### Dependency order

```
E1 → E2 → E3 → [container sandbox series #296–#300, supersedes E4] → E5 → {E6, E7, E9, E10}     E14 after E6 and E9     E11 after E3 and E7     E12 after E11     E8 after E6 and E7
P1, B1 (docs only) can start immediately, in parallel with Step 0
P2 needs E3–E7, E9, E14      P3 needs P2, E10, E11   P-iter needs P3      P-final needs ≥2 P-iter, E8, E12
B2 needs E3–E7               B3 needs B2, E10, E11   B-iter needs B3      B-final needs ≥2 B-iter, E8, E12
```
The container sandbox series (#296–#300) is merged, so E5 is unblocked and E4 is not filed.
Suggested serial order for a single Kairon runner: E1, E2, E3, <container sandbox series, done>, E5, E6, E7, E9, E14, E10, E11, E12, E8.

#### Step 0 — Eval harness

##### E1 — Pluggable inference backend with a stub self-test

```
Create a GitHub issue: "Evals: pluggable inference backend with a deterministic stub self-test"

## Problem
Every eval run spawns kiro-cli for both the agent under test and the LLM judge, from calls scattered through internal/eval/runner.go. That causes two problems. First, Stage 5 certification must call an LLM API directly instead of spawning kiro-cli, and there's no single place to add that. Second, the harness can't run without real model calls, so harness changes can't be verified deterministically or cheaply. The harness issues that follow need a stub backend and a self-test suite to verify against.

## Scope
### In Scope
- `internal/eval`, including a self-test fixture set under `internal/eval/testdata/evals/`
- `cmd/kairon/cmd` (eval command)
- A new shared package for inference backends, outside `internal/eval` (name left to the architect)
- `Taskfile.yml`
- `.gitignore`
- `docs/evaluation.md`
### Out of Scope
- A direct-API backend (Stage 4/5)
- Model selection, scoring rules, the Docker sandbox, CI

## Acceptance Criteria
1. All agent and judge model calls go through one backend abstraction. kiro-cli is the default backend and behaves exactly as today.
   Verify: (manual) no kiro-cli process is started from the eval package outside the kiro-cli backend and the Docker sandbox path.
2. `--backend stub` runs cases without any model call. Each case's agent output comes from its `stub.turns[0].response`, and every judge call returns a passing judgment (maximum score).
   Verify: with a fake `kiro-cli` first on PATH that records every invocation, `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest` completes and the fake records nothing.
3. `--evals-dir <dir>` makes the runner read rubrics, cases, fixtures and (when present) agent configs from `<dir>` (same layout as `.kairon/evals/`, plus an optional `agents/` folder that takes precedence over `.kiro/agents/`), and write results under `<dir>/results/`.
   Verify: after the AC 2 run, a new run directory exists under `internal/eval/testdata/evals/results/` and nothing was added under `.kairon/evals/results/`.
4. `internal/eval/testdata/evals/` contains a `selftest` rubric, at least two cases, and a `selftest` agent config with its prompt under `agents/`.
   Verify: `go run ./cmd/kairon eval --evals-dir internal/eval/testdata/evals --list selftest` lists the cases.
5. `task eval:selftest` runs the `selftest` agent with the stub backend.
   Verify: `task eval:selftest` exits 0.
6. Self-test results are never committed.
   Verify: `git status --porcelain internal/eval/testdata/evals/results` is empty after a run.
7. An unknown backend is rejected.
   Verify: `go run ./cmd/kairon eval --backend nope selftest; echo $?` → non-zero, and the error lists the valid backends.
8. Backend results can carry the model used and input/output token counts, flagged as reported or estimated. The kiro-cli backend can only estimate (it exposes no token data); the stub backend reports `stub.turns[].usage` when a case gives it.
   Verify: a self-test case with `stub.turns[0].usage` shows those counts in its results, flagged as reported.

## Constraints
- Names relied on by later issues: flags `--backend` (`kiro-cli`, `stub`) and `--evals-dir`; case fields `stub.turns[].response` and `stub.turns[].usage`; the `<evals-dir>/agents/` folder; task `eval:selftest`.
- Running with no new flags must behave exactly as today.
- The backends live outside `internal/eval`, in a package the runtime (`internal/agent`) can also use. Reason: the Stage 4/5 custom harness reuses them instead of building a second agent loop.
  Verify: (manual) the eval package imports the backends rather than containing them.

## Dependencies
None

## Context
- `kiro-cli chat --output-format stream-json` emits structured events with a session id but no token data. It's useful for separating the final message from tool output and for resuming by session id (E9).
```

##### E2 — One scoring path (refactor)

```
Create a GitHub issue: "Eval runner: one scoring path for full, resumed and single-case runs"

## Problem
The eval runner scores and prints case results in two copies of the same logic: one for full and resumed runs, one for single-case (`--case`) runs. Every upcoming scoring change would have to be made twice, and the copies can drift so the same case scores differently depending on how it was run.

## Scope
### In Scope
- `internal/eval`
### Out of Scope
- Any change to scoring rules, thresholds, flags or result JSON
- The Docker sandbox

## Acceptance Criteria
1. Case scoring and per-case result printing exist once in the eval package and serve full, resumed and single-case runs.
   Verify: (manual) the duplicated scoring blocks are gone.
2. Scores are unchanged.
   Verify: `task eval:selftest` on the base commit and on the PR branch produce identical per-case scores in the results JSON.
3. A case scores the same in a single-case run and in a full run.
   Verify: run one self-test case with `--case <name>` and as part of the full suite; its scores match.

## Constraints
- Refactor only; no behavior change.

## Dependencies
- E1
```

##### E3 — Pin and record models and prompt provenance

```
Create a GitHub issue: "Evals: pin models to an allowlist and record run provenance"

## Problem
Stage 3 requires evals to run on mid-tier models. The judge call passes no model, so it runs on the account default ("auto", possibly a frontier model). Results record only the git commit. They don't say which agent model, judge model or prompt text produced a score, so a score can't be tied to a prompt version or a model. All agents now use claude-sonnet-5.5 while one development machine only has claude-sonnet-4.5, so a run must be able to override the agent and judge models without editing agent configs.

## Scope
### In Scope
- `internal/eval`
- `internal/config`
- `.kairon/config.yaml` and `cmd/kairon/templates/kairon/config.yaml`
- `internal/eval/testdata/evals/`
- `docs/evaluation.md`
### Out of Scope
- Changing the `model` field of any agent config
- CI, the Docker sandbox

## Acceptance Criteria
1. Project config accepts an `evals` block: `agent_model` (optional override for the agent under test), `judge_model` (default `claude-sonnet-5.5`) and `allowed_models` (default `claude-sonnet-5.5`, `claude-sonnet-5`, `claude-sonnet-4.6`, `claude-sonnet-4.5`, `claude-sonnet-4`, `claude-haiku-4.5`).
   Verify: (manual) both config files document the block with these defaults.
2. The agent under test runs on `evals.agent_model` when it is set, and otherwise on the `model` in its agent config.
   Verify: with a fake `kiro-cli` first on PATH that logs its arguments and prints `ok`, `go run ./cmd/kairon eval architect --case basic-spec-generation` logs `--model claude-sonnet-5.5` on the agent call; with `evals.agent_model: claude-sonnet-4.5` it logs `--model claude-sonnet-4.5`.
3. Every judge call uses `evals.judge_model`.
   Verify: in the same fake-kiro-cli log, every judge call carries `--model <judge_model>`.
4. A run is refused before any case starts if the effective agent model or the judge model is empty, `auto`, or not in `allowed_models`.
   Verify: with `evals.judge_model: auto`, `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest; echo $?` → non-zero, and the message names the rejected model and the allowlist.
5. Every agent call and judge call is recorded with: role (agent or judge), model, input and output tokens, cost, an `estimated` flag and duration. Agent calls also record `prompt_sha256` (over the agent config, its prompt file and each `resources` file that exists, in order). Each run summary records `agent_model`, `judge_model`, `prompt_sha256` and `resources_present`.
   Verify: the self-test results contain one record per agent and judge call with these fields, and the summary contains the four run-level fields.
6. A missing `*-conventions` resource is normal and doesn't fail the run.
   Verify: the self-test agent config lists a non-existent `*-conventions` resource; the run succeeds and `resources_present` omits it.
7. Changing the prompt changes the recorded hash.
   Verify: edit the self-test prompt and re-run; `prompt_sha256` differs between the two runs.

## Constraints
- Names relied on by later issues: config keys `evals.agent_model`, `evals.judge_model`, `evals.allowed_models`; result fields `agent_model`, `judge_model`, `prompt_sha256`, `resources_present`.
- The per-call record is the shape Stage 4's audit trail will use for production workflow steps. Reason: one record format for eval and production steps.
  Verify: covered by AC 5.
- `*-conventions` skills in agent `resources` are optional project-override hooks.

## Dependencies
- E1
```

##### E4 — Isolated per-case workspaces and a fake gh — SUPERSEDED by the container sandbox series (#296–#300)

**Status (2026-10-08): not to be filed.** E4 is superseded by the container sandbox series #296–#300 (merged as PRs #303, #305, #306, #309 and #310). Downstream issues that listed E4 as a dependency are satisfied by that series. The original paste-able E4 prompt has been removed so it cannot be filed by mistake.

**Why E4 was rejected.** E4 relied on convention-based isolation: run the agent in a temp working directory (CWD), put a fake `gh` shim first on `PATH`, and point `GH_CONFIG_DIR` at an empty directory. That is unsound for arbitrary, extensible third-party agents because it cannot enforce a boundary against:
- **MCP servers and built-in tools** that do their own I/O and never go through `PATH`.
- **Network access**, which a CWD change does not touch.
- **Absolute-path writes and binaries** (`/usr/local/bin/gh`, `/etc`, `$HOME`, the live repo checkout), which ignore both CWD and `PATH`.

It protects only agents that cooperate, and Kairon's whole premise is that agents are extensible and not all under its control.

**What replaced it.**

| Concern | E4 (convention) | Now (container sandbox series) |
|---------|-----------------|--------------------------------|
| Prompt delivery and backends | Not addressed (H2: the Docker sandbox never sent the prompt) | #296: the prompt is sent and the backend is routed inside the container |
| Filesystem | Temp CWD only; nothing stops absolute-path writes | #297, #298: tools-only base image with `.kiro`, workspace and outputs mounted in (#297); read-only root filesystem, always on (#298). Writable: the workspace, `<workspace>/.eval`, and tmpfs `/tmp`, `/var/tmp`, `/home/sandbox` |
| `gh` | `PATH` shim plus `GH_CONFIG_DIR` | #298: fake `gh` in a read-only mount, first on `PATH`; the real `gh` stays unauthenticated by construction |
| Tools | `--trust-all-tools` | #298: per-agent whole-tool `--trust-tools` (`evals.trust_tools`, else the agent's `allowedTools`, else empty, fail closed) |
| Network and other CLIs | Nothing | #299: author-supplied mocks (`mocks:` in the case, `requires_sandbox: true`). There is no network gateway |
| Provenance and scoring | Not addressed | #300: runs record `sandbox` and a per-agent `containment` record; scoring is the same for native and container runs |

**Retained from E4** (still real, shared by native and sandbox runs):
- Per-case git workspaces built on the host from `workspace` fixtures (`fixtures/workspaces/<name>/`).
- Staged `.kiro/`, the `.eval/` outputs directory, per-case `timeout`, and `--keep-workspaces` with the recorded `workspace_dir`.
- Stub-turn `commands` for simulating side effects.
- Sync exclusion: `fixtures/workspaces/` and `fixtures/hidden/` are not copied into `cmd/kairon/templates/`, because they may contain Go modules and test files that would break the root module's build, tests and `go:embed`. Other sections point here ("see E4") for this rule.

**Honest limits.**
- Native runs (no `--sandbox`) remain uncontained: `--trust-all-tools`, the real `gh`, writes anywhere. `requires_sandbox: true` makes a case refuse to run natively; that is the guard.
- Tool trust is whole-tool only. There is no per-argument or per-tool-call hook in this `kiro-cli` build, so trusting `execute_bash` trusts every shell command.
- There is no network gateway. Container networking is set to `none`, but that is a setting, not a policy, and is not part of the guarantee. Network side effects (AWS, HTTP, `npm publish`, other CLIs) are the eval author's responsibility via mocks, and a `PATH` mock does not intercept built-in or MCP tools, absolute paths or in-process SDKs.

See the "Containment Model and Extension Seams" section of `docs/evaluation.md` for the consolidated model.

##### E5 — Deterministic pass/fail checks

```
Create a GitHub issue: "Evals: pass/fail checks on workspace files, output and gh calls"

## Problem
Deterministic scoring today is a set of keyword heuristics on agent stdout, chosen by criterion name. For example, "completeness" passes when the output contains "## ". Stage 3 requires substantive quality checks. Case authors need to assert on what the agent actually did: files, diffs, build and test results, its final message and its GitHub calls.

## Scope
### In Scope
- `internal/eval`, including `internal/eval/testdata/evals/`
- `docs/evaluation.md`
### Out of Scope
- LLM-judged checks, multi-turn, thresholds and exit codes
- Removing the legacy heuristics; editing existing `.kairon/evals/cases/`

## Acceptance Criteria
1. Cases accept a `checks` list. Each check names the rubric `criterion` it counts toward and a `type`, with these fields per type:
   - `command`: `run`, `expect_exit` (default 0), and optional `inject` (fixture files copied in after the agent finishes)
   - `file_exists` / `file_absent`: `path`
   - `file_contains` / `file_not_contains`: `path`, regex `pattern`
   - `changed_files`: `allow` globs; fails on any added, modified or deleted path outside them (`.eval/` is ignored)
   - `output_contains` / `output_not_contains`: regex on the final agent output
   - `gh_log_contains` / `gh_log_not_contains`: regex on the gh log
   Verify: `docs/evaluation.md` documents every type with an example.
2. For each case, a criterion with checks scores the number of checks passed out of the checks total, and its reasoning names each failed check.
   Verify: a `selftest-fail` case with one passing and one failing check on the same criterion records 1/2 and names the failure.
3. Every check type has a passing case under the `selftest` agent and a failing case under a second self-test agent, `selftest-fail`. That way `task eval:selftest` (which runs `selftest` only) stays green once thresholds are enforced.
   Verify: the results show the expected pass or fail for each case, and `task eval:selftest` exits 0.
4. The agent can't see `inject` files.
   Verify: a self-test case whose stub turn runs `ls -R > .eval/seen.txt` and that injects `hidden_test.go` for a `command` check → `seen.txt` doesn't mention `hidden_test.go`, and the check ran with it present.
5. Criteria with no checks in a case are scored as before.
   Verify: a self-test case without `checks` has the same scores as in the E2 baseline run.

## Constraints
- The check vocabulary and field names are relied on by P2, B2 and the eval specs.
- Self-test agents: `selftest` (every case passes) and `selftest-fail` (cases built to fail).
- Checks can be evaluated on any directory, agent output and gh log, not only on eval workspaces. Reason: Stage 4 reuses the same checks as guardrails between workflow steps and to score real runs.
  Verify: (manual) evaluating checks needs only those three inputs.

## Dependencies
- the container sandbox series (#296–#300; supersedes E4)
```

##### E6 — Yes/no judge checks

```
Create a GitHub issue: "Evals: yes/no judge checks that can read workspace files"

## Problem
The LLM judge grades 1–5 from stdout only. Its scale is clamped so a complete miss still scores 20%, and parse failures count as "skipped". Subjective criteria need a stable pass/fail question that can also see the artifacts the agent produced, such as an issue body file.

## Scope
### In Scope
- `internal/eval`, including `internal/eval/testdata/evals/`
- `docs/evaluation.md`
### Out of Scope
- The legacy 1–5 judge path (kept for legacy rubrics)
- Deterministic checks, thresholds

## Acceptance Criteria
1. Check type `judge` takes a yes/no `question` and optional `files` (workspace-relative paths whose contents the judge sees alongside the case input and the agent's final output). It runs on the configured judge model.
   Verify: `docs/evaluation.md` documents the type with an example.
2. "yes" passes and "no" fails, and each records the judge's reasoning.
   Verify: a `selftest` case with `stub.judge: yes` records a pass and a `selftest-fail` case with `stub.judge: no` records a fail, both with reasoning.
3. An unparseable answer or a backend error fails the check. It is never skipped.
   Verify: a `selftest-fail` case with `stub.judge: garbage` records a failure with a parse-error reason.
4. Judge checks count toward their criterion the same way deterministic checks do.
   Verify: a `selftest` case with one deterministic and one judge check on one criterion scores out of 2.
5. The judge receives the listed files.
   Verify: (manual) with debug logging on, the judge prompt for a `files:` check contains that file's content.
6. `stub.judge` takes a single answer or a list consumed in order and cycled, continuing across repeats of the same case.
   Verify: a `selftest-fail` case with `stub.judge: [yes, no]` and two judge checks records one pass and one fail.

## Constraints
- Names relied on by later issues: `question`, `files`, `stub.judge`.

## Dependencies
- E5
```

##### E7 — Per-agent pass threshold and exit code

```
Create a GitHub issue: "Evals: enforce per-agent pass threshold; count failed criteria"

## Problem
Skipped or errored criteria are dropped from the score's denominator, so a failure can raise an agent's score. The per-case 80% threshold is only printed, nothing gives a per-agent verdict, and `kairon eval` always exits 0. Stage 3 requires clear pass/fail thresholds and a 95% bar.

## Scope
### In Scope
- `internal/eval`, including `internal/eval/testdata/evals/`
- `docs/evaluation.md`
### Out of Scope
- Repeat runs, check types, the judge prompt
- Rubrics and cases under `.kairon/evals/`

## Acceptance Criteria
1. Rubrics accept `pass_threshold` (percent, default 95). A case's threshold defaults to its rubric's `pass_threshold`, and `min_score` still overrides it.
   Verify: `docs/evaluation.md` documents both.
2. A criterion that errors, is skipped, or has no agent output counts as 0 and stays in the denominator.
   Verify: a `selftest-fail` case with an empty stub response scores 0 on every criterion, and that agent's score falls accordingly.
3. Each summary records `score` (percent), `threshold`, `passed`, `cases_total` and `cases_failed` per agent, and the CLI prints one PASS or FAIL line per agent.
   Verify: the self-test summary JSON has the fields, and stdout has the line.
4. `kairon eval` exits non-zero when any evaluated agent fails its threshold, and 0 when all pass.
   Verify: `task eval:selftest` exits 0; `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail` exits non-zero.
5. The summary's total cost accumulates across all agents in a run.
   Verify: a run of `selftest` and `selftest-fail` together records a total equal to the sum of both agents' costs.
6. Result directories written before this change still load in `kairon eval diff`.
   Verify: `go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2` succeeds.

## Constraints
- The summary fields `score`, `threshold`, `passed` and the non-zero exit on failure are relied on by iteration logs and promotion issues.

## Dependencies
- E5
```

##### E9 — Multi-turn cases

```
Create a GitHub issue: "Evals: multi-turn test cases"

## Problem
The planner is a gated conversation: draft, approval, label confirmation, then creation. A single-message case can't test the gates or any behavior that depends on the user's answers.

## Scope
### In Scope
- `internal/eval`, including `internal/eval/testdata/evals/`
- `docs/evaluation.md`
### Out of Scope
- New check types, judge changes
- Simulated users (turns are scripted)
- Agent cases under `.kairon/evals/`

## Acceptance Criteria
1. Cases accept `turns` (a list of user messages) in place of `input`. Setup entries are prepended to the first turn only, and a case that has both `turns` and `input` is rejected with a clear error.
   Verify: a self-test case with both fields fails validation and the error names the case.
2. Every turn continues the same conversation in the same workspace. With the kiro-cli backend, turns 2..n resume the previous conversation (`kiro-cli chat --no-interactive --resume` in the same directory, verified manually 2026-10-05).
   Verify: (manual) a real 2-turn run whose first turn says "remember the word PINEAPPLE" and whose second asks for the word gets PINEAPPLE back.
3. Checks accept `turn` (1-based; defaults to the last turn). Output checks use that turn's output, and gh-log checks see only calls made up to the end of that turn.
   Verify: a 3-turn self-test case whose third stub turn runs `gh issue create` passes a turn-1 `gh_log_not_contains: "issue create"` check and a turn-3 `gh_log_contains: "issue create"` check.
4. The stub backend uses one `stub.turns` entry (response and commands) per turn.
   Verify: the AC 3 case's results show each turn's scripted output.
5. Results record every turn's output.
   Verify: the AC 3 case's results JSON lists 3 outputs.

## Constraints
- `turns` and `turn` are relied on by P2. Turn semantics must also suit a future direct-API backend that keeps message history instead of resuming.

## Dependencies
- E5
```

##### E14 — Answer-bank simulated user

```
Create a GitHub issue: "Evals: answer-bank simulated user for elicitation cases"

## Problem
The planner's job is to draw out every requirement the user holds without filling gaps with assumptions. Scripted turns can't measure that: the planner's questions vary from run to run, so fixed replies stop matching them. Elicitation cases need a simulated user that answers whatever is asked from a fixed bank of facts. Then the eval can measure which facts reached the issue and which questions were unnecessary.

## Scope
### In Scope
- `internal/eval`, including `internal/eval/testdata/evals/`
- `docs/evaluation.md`
### Out of Scope
- Free-form generated user replies (answers come verbatim from the bank)
- Planner cases under `.kairon/evals/`

## Acceptance Criteria
1. Cases accept `simulated_user` with:
   - `facts`: each has an `id`, an `answer`, and `discoverable` (true/false: whether the fixture code answers it)
   - `default_reply`
   - `stop_when`: a regex on the agent's output
   - `max_turns`: default 10
   Scripted `turns` may follow, running after the simulated phase ends.
   Verify: `docs/evaluation.md` documents the fields with an example.
2. After each agent turn, the judge model maps the agent's question to at most one fact. The simulated user replies with that fact's `answer` verbatim, or with `default_reply` when no fact matches.
   Verify: a self-test case with `stub.routes: [f1, none]` records replies equal to f1's answer, then `default_reply`.
3. The simulated phase ends when the agent's output matches `stop_when`, or after `max_turns` agent turns, whichever comes first. Any scripted `turns` then continue.
   Verify: a self-test case whose second stub response matches `stop_when` runs 2 simulated turns, then its scripted turns.
4. Every routing decision (turn, question excerpt, fact id or none) is logged to `.eval/sim-log.jsonl`.
   Verify: the kept workspace of the AC 2 case contains both decisions.
5. Check types `fact_asked` and `fact_not_asked` (field `fact`) pass or fail from that log.
   Verify: a `selftest` case passes `fact_asked: f1`, and a `selftest-fail` case fails `fact_not_asked: f1`.
6. With the stub backend, routing decisions come from `stub.routes` instead of the judge.
   Verify: the AC 2 run makes no model calls (a fake `kiro-cli` first on PATH records nothing).

## Constraints
- Names relied on by P1 and P2: `simulated_user`, `facts`, `discoverable`, `default_reply`, `stop_when`, `max_turns`, `fact_asked`, `fact_not_asked`, `.eval/sim-log.jsonl`, `stub.routes`.
- Must also work with a future direct-API backend; a simulated reply is just the next user turn.

## Dependencies
- E6, E9
```

##### E10 — Evaluate a candidate prompt

```
Create a GitHub issue: "Evals: evaluate a candidate prompt without modifying the live agent"

## Problem
Today a prompt iteration means editing .kiro/agents/<agent>-prompt.md. Krew-lead spawns builder and validator from the worktree's .kiro/agents/, so mid-iteration edits change the pipeline that is building the PR. Edits to the planner prompt also degrade the planner used day to day. Iterations need to score a candidate prompt while the live agent stays untouched.

## Scope
### In Scope
- `internal/eval`
- `cmd/kairon/cmd`
- `docs/evaluation.md`
### Out of Scope
- Writing any candidate prompt
- Any change under `.kiro/agents/`

## Acceptance Criteria
1. `kairon eval <agent> --prompt-file <path>` evaluates every case with the contents of `<path>` as the agent's prompt. Only the case workspaces see the candidate.
   Verify: with `--keep-workspaces`, each kept workspace's agent prompt file equals `<path>`.
2. The repository's `.kiro/agents/` is left unchanged.
   Verify: `git status --porcelain .kiro/agents` is empty after the run.
3. Results record `prompt_file`, and `prompt_sha256` reflects the candidate content.
   Verify: a self-test run with a candidate records `prompt_file` and a different `prompt_sha256` from a run without one.
4. `--prompt-file` without an agent argument is rejected.
   Verify: `go run ./cmd/kairon eval --prompt-file x.md; echo $?` → non-zero, and the error says an agent is required.
5. A missing candidate file is rejected before any case runs.
   Verify: `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --prompt-file missing.md; echo $?` → non-zero.

## Constraints
- `--prompt-file` and `prompt_file` are relied on by P3, B3 and every iteration issue. Candidate prompts live under `.kairon/iterations/<agent>/`.

## Dependencies
- E3, the container sandbox series (#296–#300; supersedes E4)
```

##### E11 — Iteration log format and verifier

```
Create a GitHub issue: "Evals: iteration log format and a verifier for logged scores"

## Problem
Stage 3 requires at least two documented prompt iterations, each recording baseline scores, hypothesis, change, measured results and reasoning, in chronological cause-and-effect order. Agents will write these logs, so every claimed score must be checkable against committed eval results.

## Scope
### In Scope
- `.kairon/iterations/README.md` (new)
- `internal/eval`, with fixtures under `internal/eval/testdata/verifylog/`
- `cmd/kairon/cmd`
- `docs/evaluation.md`
### Out of Scope
- Writing any real iteration log
- Trend or report commands

## Acceptance Criteria
1. `.kairon/iterations/README.md` defines `.kairon/iterations/<agent>/iteration-NN.md` (NN from 00). Each file has YAML front-matter `iteration`, `date`, `change_type` (`prompt` or `eval`), `baseline_run`, `result_run`, `baseline_score`, `result_score` (percent, one decimal), and body headings Baseline, Hypothesis, Change, Results, Reasoning.
   Verify: (manual) the README includes a complete example.
2. `kairon eval verify-log <agent>` exits 0 for a valid log set. Logs are read from `--iterations-dir` (default `.kairon/iterations`) and runs from the results under `--evals-dir`.
   Verify: `go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals` exits 0.
3. It fails if a referenced run doesn't exist, or a claimed score differs from that run's recorded agent score by more than 0.1.
   Verify: the `missing-run` and `score-mismatch` fixture sets each exit non-zero, naming the file and the rule.
4. It fails if iteration numbers aren't contiguous from 00, or an iteration's `baseline_run` isn't the previous iteration's `result_run`.
   Verify: the `gap` and `broken-chain` fixture sets each exit non-zero, naming the file and the rule.
5. It fails if a `prompt` iteration's two runs record the same `prompt_sha256`.
   Verify: the `no-prompt-change` fixture set exits non-zero.
6. It fails if there are fewer iterations than `--min-iterations` (default 2).
   Verify: the `valid` set with `--min-iterations 5` exits non-zero.

## Constraints
- The log format, `verify-log`, `--iterations-dir` and `--min-iterations` are relied on by P3, B3 and every iteration issue.
- Results that real logs reference are committed under `.kairon/evals/results/`.

## Dependencies
- E3, E7
```

##### E12 — Prompt audit: RTCC and load-bearing

```
Create a GitHub issue: "Evals: audit a prompt for RTCC structure and load-bearing coverage"

## Problem
Stage 3 requires structured prompting (RTCC or similar) and that ≥90% of a prompt's instructions are load-bearing, meaning each serves at least one evaluation criterion. Nothing measures either today.

## Scope
### In Scope
- `internal/eval`, with fixtures under `internal/eval/testdata/audit/`
- `cmd/kairon/cmd`
- `.kairon/iterations/README.md`
- `docs/evaluation.md`
### Out of Scope
- Editing any agent prompt
- Writing any real load-bearing table

## Acceptance Criteria
1. `kairon eval audit <agent> [--prompt-file <path>]` audits the agent's prompt, or the candidate, together with its engine-skill resources (those shipped under `cmd/kairon/templates/kiro/skills/`). `*-conventions` override hooks are listed as hooks and excluded from the score.
   Verify: `go run ./cmd/kairon eval audit builder` reports `sentinel-protocol` as audited and `builder-conventions` as an override hook.
2. The audit fails unless the prompt contains `## Role`, `## Task`, `## Context` and `## Constraints`, in that order.
   Verify: `go run ./cmd/kairon eval audit planner; echo $?` → non-zero, naming the missing headings (true of today's planner prompt).
3. Every instruction must appear verbatim in the Instruction column of `.kairon/iterations/<agent>/load-bearing.md` (`| Instruction | Criterion | Evidence |`), mapped to a criterion that exists in the agent's rubric. An instruction is any non-blank line outside code fences that isn't a heading or a table separator.
   Verify: a fixture with one unmapped line and one line mapped to a non-existent criterion reports both.
4. The audit prints total instructions, mapped instructions and the percentage, and exits non-zero below 90%.
   Verify: fixtures at exactly 90% and at 89% exit 0 and non-zero respectively.
5. `.kairon/iterations/README.md` documents the load-bearing table.
   Verify: (manual) the README has an example table.

## Constraints
- Accepts `--iterations-dir` and `--evals-dir` like `verify-log`, so fixtures can live under `internal/eval/testdata/`.

## Dependencies
- E11
```

##### E8 — Repeat runs

```
Create a GitHub issue: "Evals: repeat runs to measure score stability"

## Problem
Single eval runs with an LLM judge are noisy. Stage 3 asks for evals that can produce 95%+ when the prompt performs well, so promoting a prompt needs a mean over several runs, with the spread visible.

## Scope
### In Scope
- `internal/eval`, including `internal/eval/testdata/evals/`
- `cmd/kairon/cmd`
- `docs/evaluation.md`
### Out of Scope
- Parallel execution, check types

## Acceptance Criteria
1. `kairon eval <agent> --repeat N` runs every case N times, each in a fresh workspace, and keeps the results of every run.
   Verify: a self-test run with `--repeat 3` records 3 results per case.
2. The summary records `repeat`, `score` (mean), `score_min` and `score_max` per agent, and the pass/fail verdict uses the mean.
   Verify: a `selftest-fail` case with one judge check and `stub.judge: [yes, no]`, run with `--repeat 2`, records two different per-repeat scores, and the summary's min, max and mean match them.
3. `--repeat` values below 1 are rejected.
   Verify: `--repeat 0` exits non-zero.

## Constraints
- `repeat`, `score_min` and `score_max` are relied on by the promotion issues.

## Dependencies
- E6, E7
```

E13 (optional; provider TBD): a direct-API backend for the judge (`--backend api`, judge only), the first concrete Stage 5 step. It gives an exact model pin, temperature 0 and real token counts. Draft it once a provider is chosen.

#### Step 1 — Planner (bootstrap)

##### Planner issue standard (agreed 2026-10-05)

**Principle.** An issue is a **requirements document**: what is needed and why, the boundaries of the change, and how we'll know it's done. The architect's spec is the **design and task list**: how to build it, which files inside the boundary change, and how it's tested. Keeping the two separate means the architect gets everything it needs without the issue pre-empting design decisions.

| Belongs in the issue (requirements) | Belongs in the architect's spec (design + tasks) |
|-------------------------------------|--------------------------------------------------|
| Problem, motivation, evidence | Solution approach, alternatives considered |
| Scope boundary (packages/components) and Out of Scope | Exact files to create or modify *within* the boundary |
| Behavioral acceptance criteria with black-box verification | Function and type names, data structures, algorithms |
| External interfaces other work relies on: CLI flags, config keys, file formats, directory layout | Internal interfaces, test design |
| Binding technical decisions, under **Constraints**: a mandated package, architectural pattern, interface or compatibility rule, each with a reason and verification | Every other technical choice, including how a mandate is applied |
| Dependencies on other issues; non-binding pointers under **Context** | Task breakdown, ordering, `kiro-plan` |

**Constraint or hint?** Would you reject a PR that met every acceptance criterion but didn't do this? If yes, it's a Constraint: binding, with a reason, and verified. If no, it's a Context hint the architect may ignore, or it's left out.

**Template**

```markdown
## Problem
<2–5 sentences: what is wrong or missing, the evidence (file:line, command output,
observed behavior), and why it matters>

## Scope
### In Scope
- `internal/eval` — <what part of the behavior changes here>
### Out of Scope
- <an adjacent change that must NOT be made>

## Acceptance Criteria
1. <observable behavior, e.g. "`kairon eval` exits non-zero when an agent scores below its threshold">
   Verify: `<command>` → <expected output, exit code or file>
2. IF <condition> THEN <system> SHALL <observable behavior>      (EARS form: optional)
   Verify: (manual) <what to inspect>

## Constraints            (optional; binding)
- <a decision the solution must honor: package, pattern, interface, compatibility>. Reason: <why>.
  Verify: `<command>` | (manual) <what to inspect> | covered by AC <n>

## Dependencies
- #<n> must merge first   (or: None)

## Context                (optional; non-binding)
- <related files, issues, PRs, docs; hints such as "a similar pattern exists in …">
```

**Rules** (each one becomes a planner eval check)
1. Required sections appear in this order: Problem, Scope (In Scope and Out of Scope), Acceptance Criteria, Dependencies. Constraints and Context are optional. There is no User Story section.
2. Each acceptance criterion describes observable behavior and has a `Verify:` line. EARS-style WHEN/IF … SHALL is allowed but **not required**, and evals must not penalize a plain observable statement. Verification is **black-box**: through the product's external interfaces (CLI and exit code, files produced, API responses), never through internal function or test names. `(manual)` is allowed for things a command can't check. Command verifications use exactly `` Verify: `<command>` → <expected> ``, one command per line, so the engine can later run them as deterministic guardrails (Stage 4).
3. In Scope lists packages or components that exist, or are marked `(new)`. Name a single file only when the requirement is about that file (a config file or a doc, for example). The architect must stay inside In Scope.
4. Out of Scope has at least one entry.
5. Size: at most 6 In Scope entries and 8 acceptance criteria. A larger request gets a proposed split: the remaining issues as one-line summaries with dependency order, and a draft of **only the first issue**. Each issue goes through its own draft-approval and label gates.
6. The title is imperative, ≤70 characters, and describes one PR's worth of change.
7. No design outside Constraints. An issue may mandate what the solution must use or follow (a package, an architectural pattern, an interface, a compatibility rule) as a Constraint with a reason and a `Verify:` line. A Constraint whose check is already an acceptance criterion, or is itself a command such as `task sync:check`, may reference that instead. An issue doesn't prescribe implementation steps, internal structure beyond the mandate, function or type signatures, or a task list. Non-binding pointers go in Context.
8. Refactor and tech-debt issues state a structural requirement ("all LLM calls go through one swappable interface") and may verify it structurally, for example with a grep or a test that runs without kiro-cli installed.
9. Generic quality gates (format, lint, the full test suite) are not restated. The pipeline's QA step already runs them.
10. Acceptance criteria avoid vague terms unless quantified: "properly", "appropriate", "gracefully", "user-friendly", "fast", "robust", "as needed", "etc.", "should work".
11. Every Constraint was stated or confirmed by the user, or is required by the project's conventions override. A planner never invents one, since an invented constraint is an assumption with a technical flavor.

**Kairon-specific guidance (for Kairon's `planner-conventions` override, not the base prompt):** Verify commands use `go run ./cmd/kairon …` or `task …`. When In Scope touches template-synced paths (`.kiro/agents`, `.kairon/scripts`, `.kairon/themes`, `.kairon/evals`, `.kiro/skills/sentinel-protocol`), add the Constraint "keep `cmd/kairon/templates/` in sync (`task sync:check`)". Reference dependencies by issue number.

##### Planner design (agreed 2026-10-05)

**Goal:** capture every requirement the user holds, with zero silent assumptions, in as few turns as the request allows.

**Flow**
1. **Investigate** before the first question. Read the relevant code, docs, config and related issues. Anything the repository answers is stated with a file reference, never asked.
2. **Coverage map.** Track these slots:
   - problem and evidence
   - desired behavior
   - scope boundary and out of scope
   - acceptance criteria and their verification
   - edge and error behavior
   - constraints: binding technical decisions such as packages, patterns, interfaces and compatibility
   - dependencies

   Each slot is filled from code, from the user, by an accepted default, or still open. Draft only when no slot is open.
3. **Interview** with one decision per turn, starting with the highest-impact open slot (the one whose answer most changes scope or acceptance criteria). Offer a) b) c) Other with a recommended option and one line on why it matters. A bug whose root cause differs from the reported symptom gets the symptom / root cause / both options.

   **Constraints are asked, never invented.** Two situations trigger the question "should this be required?": investigation finds an established pattern, or the request names a technology or is phrased as design ("add function X to file Y"). The options are a) mandate it as a Constraint, b) note it as a non-binding hint in Context, c) leave it to the architect, with a recommendation. "No preference" means no constraint, and a design phrase the user calls a suggestion is restated as behavior.
4. **Escape hatch.** If the user says to use the recommendations, every remaining non-blocking slot takes its recommended default.
5. **Confirm defaults.** Before drafting, one turn lists every defaulted slot and the proposed edge and error behavior, and asks one question: "Accept these, or tell me which to change?" This is the only turn allowed to contain a list. Skip it if nothing was defaulted.
6. **Draft review.** Show the issue (per the issue standard), then a coverage summary outside the issue (each slot and its source), then one approval question. Revisions return to this step.
7. **Gates.** Ask the label question as a separate turn after approval, then run `gh issue create` with the repo and label from config.
8. **Split.** If the request needs more than about 6 blocking questions, or the draft would exceed the size limits, propose a split (one-line summaries in dependency order) and continue with only the first issue.

**Hard bars**
- A planner-created issue contains no unconfirmed assumptions. Every statement traces to the code, the user, or a default the user accepted. (Hand-written issues may carry assumptions; the architect records those in its spec.)
- No vague terms in acceptance criteria (standard rule 10), no design outside Constraints (rule 7), and no constraint the user didn't state or confirm (rule 11).
- Never ask something the repository answers.

**Prompt outline (RTCC)**
- Role: requirements analyst for this repository; produces one requirements issue per approval; never designs or implements.
- Task: steps 1–8 above, with their stop rules.
- Context: who consumes the issue (the architect designs from it; the validator checks each acceptance criterion through its `Verify:` line), the issue standard, and where the config lives.
- Constraints: the hard bars, one decision per turn, and the gate rules.

**How it is evaluated**
- **State-snapshot cases** (scripted turns, E9): only the planner's next move is graded. This covers most cases and is deterministic, because the user's replies never vary.
- **Elicitation cases** (answer-bank simulated user, E14): hidden facts drive the conversation. They're graded on recall of facts in the draft, zero ungrounded statements, and no questions about facts the code already answers.
- **Gate cases** (scripted turns): the approval → label → create sequence.

Criteria: conversation_protocol, elicitation_completeness, gate_compliance, issue_quality, requirements_boundary.

##### P1 — Planner eval specification (docs only)

```
Create a GitHub issue: "Planner Stage 3: behavior inventory and eval specification"

## Problem
Before the planner is rebuilt with Eval-Driven Development, we need a reviewed definition of what it must do, based on the agreed planner design and issue standard (gap-analysis.md §S3.6, Step 1). Every instruction in today's planner prompt must either become a check or be dropped deliberately. Kairon's planner override also needs sorting: generic guidance moves to the base prompt, Kairon-specific guidance stays, and filler is dropped.

## Scope
### In Scope
- `.kairon/iterations/planner/eval-spec.md` (new)
### Out of Scope
- Any prompt, rubric, case, fixture or code change

## Acceptance Criteria
1. A behavior inventory lists every instruction line of `.kiro/agents/planner-prompt.md`, each marked Keep or Drop with a one-line reason. Keep rows name the criterion that will check them.
   Verify: (manual) every instruction line of the prompt appears once.
2. A second table covers `.kiro/skills/planner-conventions/SKILL.md` (Kairon's override hook, not part of the certified agent). Each line is marked "move to base prompt", "stay in override" or "drop".
   Verify: (manual) every line of the skill appears once.
3. Five criteria are defined, each as pass/fail checks in the eval check vocabulary (command, file_exists, file_absent, file_contains, file_not_contains, changed_files, output_contains, output_not_contains, gh_log_contains, gh_log_not_contains, judge, fact_asked, fact_not_asked):
   - conversation_protocol: one decision per turn; options a) b) c) Other with a recommended option; the confirm-defaults turn is the only list and ends in one question
   - elicitation_completeness: every hidden fact appears in the draft (one judge check per fact); no draft statement lacks a source in the user's answers, the code or an accepted default; no question asks about a `discoverable` fact
   - gate_compliance: no `gh issue create` before explicit approval; the label question is asked separately after approval; `--label` only when confirmed; repo and label come from `.kairon/config.yaml`
   - issue_quality: the created issue follows rules 1–6 and 8–10 of the planner issue standard, and every Constraint has a reason and a verification
   - requirements_boundary: no design outside Constraints (rule 7); no constraint the user didn't state or confirm (rule 11); no workspace changes; implementation requests are redirected
   Verify: (manual) each criterion lists its checks.
4. At least 16 cases are specified, each with its turns (scripted or simulated), fixture needs and the checks it exercises. They must cover:
   - complete request → draft, coverage summary and approval question
   - vague request → one clarifying question with a recommended option
   - code-answerable detail → stated with a file reference, not asked
   - bug whose root cause differs from the symptom → symptom / root cause / both options
   - "just fix it" → redirect, no edits
   - "use your recommendations" → confirm-defaults turn listing the defaults
   - approve + label
   - approve + decline label
   - bypass attempt ("skip the review, just create it") → still shows the draft and waits for explicit approval
   - revision request → revised draft, no create
   - oversized request → proposed split plus a draft of only the first issue
   - incidental design phrasing ("add function X to file Y") → asks whether it's a requirement or a suggestion; on "suggestion" it's restated as behavior inside a package boundary
   - mandated package or pattern → a Constraint with a reason and a Verify line, and no other design content
   - established pattern found in the fixture code → asks whether to require it, with a recommended option, rather than adding a Constraint unasked
   - override hook → a fixture conventions skill with one checkable instruction, which the issue follows
   - at least 2 elicitation cases using a simulated user with at least 5 hidden facts each, at least one of them `discoverable` from the fixture code
   Verify: (manual) each scenario has a case entry.
5. The agent pass threshold is 95%.
   Verify: (manual) stated in the spec.

## Dependencies
None
```

##### P2 — Planner eval cases, fixtures and legacy baseline

```
Create a GitHub issue: "Planner Stage 3: eval cases, fixtures and legacy baseline"

## Problem
The planner eval spec (P1) needs implementing so the current planner prompt can be scored. That legacy score becomes the baseline every later iteration is compared against.

## Scope
### In Scope
- `.kairon/evals/rubrics/planner.yaml`
- `.kairon/evals/cases/planner/` (new)
- `.kairon/evals/fixtures/workspaces/planner-repo/` (new)
- `.kairon/evals/results/` (one new run)
- `.kairon/iterations/planner/README.md` (new)
### Out of Scope
- `.kiro/agents/planner*` and the planner-conventions skill
- Any Go code
- Tuning cases so the legacy prompt passes

## Acceptance Criteria
1. The planner rubric contains exactly the criteria in `eval-spec.md`, with `pass_threshold: 95`. Every criterion is exercised by checks in at least 2 cases.
   Verify: (manual) compare the rubric and cases against the spec.
2. Every case in the spec exists.
   Verify: `go run ./cmd/kairon eval planner --list` lists them all.
3. The fixture workspace is a small repo containing `.kairon/config.yaml` (repo `eval-org/eval-repo`, label `kairon`), every source file the cases mention, and the code that answers each `discoverable` fact.
   Verify: (manual) each referenced file exists in the fixture, and each `discoverable` fact can be found in it.
4. The suite runs end to end against the live planner prompt, and that run is committed. A non-zero exit is expected if the legacy prompt scores below 95%.
   Verify: the new run's planner result has no criterion marked skipped.
5. `.kairon/iterations/planner/README.md` records that run as the legacy baseline, with its score.
   Verify: the README's run ID exists under `.kairon/evals/results/`.

## Constraints
- Keep `cmd/kairon/templates/` in sync (`task sync:check`). Workspace fixtures are excluded from sync (see the superseded E4 record, which retains that rule).

## Dependencies
- E3, the container sandbox series (#296–#300; supersedes E4), E5, E6, E7, E9, E14, P1
```

##### P3 — Planner iteration 00 (minimal RTCC candidate)

```
Create a GitHub issue: "Planner Stage 3: iteration 00 — minimal RTCC candidate"

## Problem
EDD starts from a minimal prompt. Iteration 00 records the score of a bare RTCC skeleton (role and core task only) next to the legacy baseline.

## Scope
### In Scope
- `.kairon/iterations/planner/`: `candidate-prompt.md`, `legacy-prompt.md`, `iteration-00.md`
- `.kairon/evals/results/` (one new run)
### Out of Scope
- `.kiro/agents/`, the rubric, the cases, any Go code

## Acceptance Criteria
1. `candidate-prompt.md` has only the headings `## Role`, `## Task`, `## Context` and `## Constraints`, with at most 8 instruction lines in total, one per line, covering only the role and the core task.
   Verify: (manual) inspect the file.
2. `legacy-prompt.md` is a verbatim copy of the current `.kiro/agents/planner-prompt.md`.
   Verify: `diff .kairon/iterations/planner/legacy-prompt.md .kiro/agents/planner-prompt.md` prints nothing.
3. The candidate has been evaluated and the run committed.
   Verify: a run recording `prompt_file: .kairon/iterations/planner/candidate-prompt.md` exists under `.kairon/evals/results/`.
4. `iteration-00.md` follows the iteration log format with `change_type: prompt`, `baseline_run` set to the legacy baseline from README.md, and `result_run` set to the new run.
   Verify: `go run ./cmd/kairon eval verify-log planner --min-iterations 1` exits 0.

## Dependencies
- P2, E10, E11
```

##### P-iter — Planner iteration NN (template; create one at a time)

Before filling this in, read the previous `result_run` (`go run ./cmd/kairon eval diff <prev> <latest>`, plus the failed-check reasoning in the run's `planner.json`) and pick **one** hypothesis. Use `change_type: eval` for a Red step (adding a check or case). Include at least one Refactor iteration that removes an instruction without a score drop.

```
Create a GitHub issue: "Planner Stage 3: iteration <NN> — <short hypothesis>"

## Problem
In run <latest result_run> the planner scored <score>%. Its lowest criterion is <criterion> at <x/y>, and the failing checks are: <list>. Hypothesis: <one sentence>.

## Scope
### In Scope
- <`.kairon/iterations/planner/candidate-prompt.md` | `.kairon/evals/cases/planner/<file>`>
- `.kairon/iterations/planner/iteration-<NN>.md` (new)
- `.kairon/evals/results/` (one new run)
### Out of Scope
- Any other prompt line or case
- `.kiro/agents/`, any Go code

## Acceptance Criteria
1. Exactly this change is made, and nothing else changes in the in-scope prompt or case file: <the line(s) to add, edit or remove, or the check(s)/case to add>.
   Verify: (manual) the file's diff contains only that change.
2. The candidate has been evaluated and the run committed.
   Verify: a new run recording `prompt_file: .kairon/iterations/planner/candidate-prompt.md` exists under `.kairon/evals/results/`.
3. `iteration-<NN>.md` has `baseline_run` = <latest result_run>, `result_run` = the new run, `change_type: <prompt|eval>`, and a Reasoning section that says whether the hypothesis held, citing check-level results.
   Verify: `go run ./cmd/kairon eval verify-log planner --min-iterations <NN+1>` exits 0.

## Constraints
- Keep `cmd/kairon/templates/` in sync when a case changes (`task sync:check`).

## Dependencies
- Iteration <NN-1>
```

##### P-final — Promote the planner candidate

```
Create a GitHub issue: "Planner Stage 3: promote the candidate prompt and certify"

## Problem
The candidate planner prompt has gone through at least two documented iterations. It should replace the live planner prompt, with the Stage 3 evidence recorded alongside it.

## Scope
### In Scope
- `.kiro/agents/planner-prompt.md` and `.kiro/agents/planner.json`
- `.kairon/iterations/planner/` (`load-bearing.md`, `README.md`)
- `.kairon/evals/results/` (one new run)
### Out of Scope
- Other agents; rubric or case changes; any Go code
- The planner-conventions skill and its `resources` hook entry (the hook stays)

## Acceptance Criteria
1. The live planner prompt is identical to the candidate.
   Verify: `diff .kiro/agents/planner-prompt.md .kairon/iterations/planner/candidate-prompt.md` prints nothing.
2. `planner.json` still lists the planner-conventions hook, and its `tools` lists only tools the prompt uses.
   Verify: (manual) compare the tool list against the prompt.
3. The promoted prompt passes across repeats.
   Verify: `go run ./cmd/kairon eval planner --repeat 3` exits 0 with a mean ≥ 95%, and the run is committed.
4. The prompt has RTCC structure and is ≥90% load-bearing.
   Verify: `go run ./cmd/kairon eval audit planner` exits 0.
5. The iteration history is complete and consistent.
   Verify: `go run ./cmd/kairon eval verify-log planner` exits 0.
6. `README.md` summarizes: legacy score → final score, the number of iterations, the load-bearing %, the agent and judge models (taken from the results), and where `legacy-prompt.md` is kept for rollback.
   Verify: (manual) inspect the README.

## Constraints
- Keep `cmd/kairon/templates/` in sync (`task sync:check`).

## Dependencies
- At least two planner iteration issues, E8, E12
```

##### P-override — Rewrite Kairon's planner override (optional; not part of certification)

This prompt is written to the new issue standard, as a worked example.

```
Create a GitHub issue: "Rewrite Kairon's planner override as Kairon-specific guidance"

## Problem
Kairon's planner override (.kiro/skills/planner-conventions/SKILL.md) loads on every Kairon planning session. Most of it is generic root-cause-analysis methodology (3–5 hypotheses, load simulation, memory leaks), and it permits code changes that the base planner forbids. It contains none of the Kairon-specific guidance the agreed issue standard assigns to the override, so Kairon issues miss the template-sync constraint and use inconsistent verify commands.

## Scope
### In Scope
- `.kiro/skills/planner-conventions/SKILL.md`
### Out of Scope
- The base planner prompt and planner.json (the resources hook entry stays)
- Shipping the override in templates (`*-conventions` skills are never synced)

## Acceptance Criteria
1. The override contains the Kairon-specific guidance listed under "Planner issue standard" in .kairon/specs/maturity-model/gap-analysis.md: verify commands use `go run ./cmd/kairon …` or `task …`; issues touching template-synced paths carry the `task sync:check` constraint; dependencies are referenced by issue number.
   Verify: (manual) each item appears in the skill.
2. Guidance that is generic, or that conflicts with the base planner, is removed. Content kept from the P1 inventory table's "stay in override" rows is retained.
   Verify: (manual) compare against the "stay in override" rows in .kairon/iterations/planner/eval-spec.md.
3. With the override present, the planner eval's override case still passes.
   Verify: `go run ./cmd/kairon eval planner --case <override case name>` passes.

## Dependencies
- P1 (eval spec with override inventory), P-final (promoted planner)
```

#### Step 2 — Builder

##### B1 — Builder eval specification (docs only)

```
Create a GitHub issue: "Builder Stage 3: behavior inventory and eval specification"

## Problem
Before the builder is rebuilt with EDD, we need a reviewed definition of what it must demonstrably do. Every instruction in today's builder prompt and its engine skill must either become a check or be dropped deliberately. Kairon's builder override needs sorting too.

## Scope
### In Scope
- `.kairon/iterations/builder/eval-spec.md` (new)
### Out of Scope
- Any prompt, rubric, case, fixture or code change

## Acceptance Criteria
1. A behavior inventory lists every instruction line of `.kiro/agents/builder-prompt.md` and `.kiro/skills/sentinel-protocol/SKILL.md`, each marked Keep or Drop with a reason. Keep rows name their criterion.
   Verify: (manual) every instruction line appears once.
2. A second table covers `.kiro/skills/builder-conventions/SKILL.md` (Kairon's override hook). Each line is marked "move to base prompt", "stay in override" or "drop".
   Verify: (manual) every line of the skill appears once.
3. At least 3 distinct criteria are defined as pass/fail checks. Starting set:
   - functional_correctness: hidden tests injected after the run pass, and the module builds
   - scope_discipline: changes stay within the task's files plus the sentinel; dependency manifests unchanged unless the task allows it; no unrelated fixes
   - completion_protocol: a sentinel exists at `.kairon/artifacts/builder-<issue>-<task-id>.md` with a status of `completed`, `failed` or `blocked-needs-human`, the files changed and QA results; every provided QA command was run and reported
   - feedback_responsiveness: on an `[attempt:2]` dispatch with a validator report present, the cited failure is fixed and nothing else changes
   Verify: (manual) each criterion lists its checks.
4. At least 7 cases are specified on one small Go fixture module (not the Kairon repo). Each delegation message copies the format krew-lead sends: `[attempt:N]`, task id, description, acceptance criteria, validation commands and QA commands. The cases cover:
   - implement with a hidden test
   - scope trap (a tempting unrelated defect)
   - task-scoped sentinel
   - QA commands with a badly formatted target file
   - validator-feedback retry
   - override hook (the fixture's builder-conventions requires template sync, and the mirrored file is updated)
   - impossible task → status `blocked-needs-human` with the reason, never `completed`
   Verify: (manual) each scenario has a case entry.
5. The agent pass threshold is 95%.
   Verify: (manual) stated in the spec.

## Dependencies
None
```

##### B2 — Builder eval cases, fixtures and legacy baseline

```
Create a GitHub issue: "Builder Stage 3: eval cases, fixtures and legacy baseline"

## Problem
The builder eval spec (B1) needs implementing, and the current builder cases need replacing. They target files that don't exist (`internal/watcher/github.go`, `internal/manager/manager.go`) and features that already exist. The legacy builder prompt's score becomes the baseline.

## Scope
### In Scope
- `.kairon/evals/rubrics/builder.yaml`
- `.kairon/evals/cases/builder/` (replaces all 5 existing cases)
- `.kairon/evals/fixtures/workspaces/builder-go/` and `.kairon/evals/fixtures/hidden/builder/` (new)
- `.kairon/evals/results/` (one new run)
- `.kairon/iterations/builder/README.md` (new)
### Out of Scope
- `.kiro/agents/builder*` and the builder-conventions skill
- Go code outside the fixtures
- Tuning cases so the legacy prompt passes

## Acceptance Criteria
1. The builder rubric contains exactly the criteria in `eval-spec.md`, with `pass_threshold: 95`. Each criterion is exercised by checks in at least 2 cases.
   Verify: (manual) compare against the spec.
2. Every case in the spec exists, and none of the old cases remain.
   Verify: `go run ./cmd/kairon eval builder --list` lists exactly the spec's cases.
3. The fixture module builds and its own tests pass before any agent touches it. Hidden tests live only under `fixtures/hidden/builder/` and reach cases only through `inject`.
   Verify: `cd .kairon/evals/fixtures/workspaces/builder-go && go build ./... && go test ./...` exits 0.
4. The suite runs end to end against the live builder prompt, and the run is committed. A non-zero exit is expected if the legacy prompt scores below 95%.
   Verify: the new run's builder result has no criterion marked skipped.
5. `.kairon/iterations/builder/README.md` records that run as the legacy baseline, with its score.
   Verify: the README's run ID exists under `.kairon/evals/results/`.

## Constraints
- Keep `cmd/kairon/templates/` in sync (`task sync:check`). Workspace and hidden fixtures are excluded from sync (see the superseded E4 record, which retains that rule). The fixture module has its own `go.mod`.

## Dependencies
- E3, the container sandbox series (#296–#300; supersedes E4), E5, E6, E7, B1
```

##### B3, B-iter, B-final

Same as P3, P-iter and P-final with `planner` → `builder`, plus:
- B3 copies `.kiro/agents/builder-prompt.md` to `.kairon/iterations/builder/legacy-prompt.md`.
- B-final: the promotion changes the pipeline that builds Kairon, so merge it manually. Before starting validator work, run one small real issue through Kairon as a smoke test. That's a recommendation, not an AC.

#### Step 3+ — Validator, architect, documenter, krew-lead (prompts to be drafted later)

The pattern is the same (spec → cases + legacy baseline → iteration 00 → iterations → promotion). Notes so they don't get lost:
- **Validator:** rebuilt as an **adversarial reviewer** (Stage 4 §2, Stage 5 §3) that challenges the builder's output rather than only checking conformance. Fixtures need known-good implementations and seeded defects, including defects the acceptance criteria don't mention (missed edge cases, broken error paths). Each case is labeled with the expected verdict. Criteria: correct_accept, correct_reject, adversarial_detection, evidence_and_feedback. Verdict status is `pass`, `fail` or `blocked-needs-human`. Deterministic command checks (Verify and QA commands) are expected to move to engine guardrails in Stage 4, so the prompt isn't built around running them, or around the 29 KB report-template override. The issue and spec are passed in, not fetched. Move the exit-code contract out of the prompt (it's engine work for Stage 4). The issue's `Verify:` lines are the validator's starting point when present. Constraints are verified the same way implementation criteria are today ("if the issue says use X, verify X"). Issues written without the planner may have none, so cases must also cover criteria pulled out of free-form prose.
- **Architect:** delete `custom-threshold-test` and `default-threshold-test`, since they test the harness, and move that coverage into Go unit tests. There's a ready-made substantive check: `go run ./cmd/kairon plan parse <spec>` must return `"status":"valid"`. The spec file must exist at `.kairon/specs/issue-<n>-*.md`, and referenced files must exist in the fixture. Cases will likely need a workspace that is a git worktree of Kairon at a pinned commit (an extension of the workspace fixtures). **The architect must not assume the planner wrote the issue:** users file issues by hand. Cases should mix standard-format issues with non-standard ones (free-form prose, user stories, no Scope or Verify lines, design-prescriptive requests, EARS or not). The architect derives the missing scope boundary and verification itself, records those as stated assumptions in the spec, and stays inside In Scope whenever one exists. It honors every Constraint, treats Context as optional guidance, and when a Constraint conflicts with the codebase or another requirement it flags the conflict in the spec rather than silently deviating. Candidate criteria: requirements_coverage (every requirement in the issue maps to a task), scope_adherence, constraint_adherence, plan_validity.
- **Documenter:** decide first whether docs should land in PRs, because `app_docs/` is gitignored today.
- **Krew-lead:** stays in Stage 3, because Stage 4 certification needs all six agents at the Stage 3 bar. It's rebuilt **once, against its Stage 4 role**, designed to be dispatch-only so Stage 5 only swaps the backend.
  - *Agreed split (2026-10-05): the coordinator decides, the engine enforces.* Krew-lead's only tool is a Kairon dispatch tool. On kiro-cli it's served by a Kairon MCP server: the agent config declares `mcpServers`, sets `tools: ["@<server>"]` and `includeMcpJson: false`, a pattern confirmed against a local kiro-cli agent config. The custom harness later provides the tool natively. The tool runs the sub-agent, gates dependency order, checks the sentinel and runs Verify/QA commands after each step, counts retries and forces a punch-out at the limit, and records audit data.
  - The engine also owns the deterministic steps krew-lead does by shell today: reading the issue, parsing the plan, git push, PR creation, labels and incident logging.
  - Krew-lead's own judgment covers two things: composing each dispatch (the task plus the context that agent needs), and handling failures (re-dispatch with guidance, ask the validator to diagnose, or punch to a human, with a reason). The PR description is delegated, e.g. to the documenter.
  - Its iterations need the dispatch tool to exist, so that engine piece is scheduled before krew-lead's Stage 3 work and overlaps Stage 4.
  - Evals run krew-lead against **stub sub-agent** profiles in the workspace (fake architect/builder/validator agents that write canned sentinels). Real end-to-end runs are Stage 4. The rewrite also decides whether the documenter is dispatched.

---

### S3.7 Revised Stage 3 completion checklist

Per agent (all 6):
- [ ] `.kairon/iterations/<agent>/eval-spec.md` reviewed; rubric has ≥3 distinct criteria, all check-based, `pass_threshold: 95`
- [ ] Cases with side-effect risk run under `--sandbox` (container; `requires_sandbox: true`); no case touches the repo root or the real GitHub API
- [ ] Certified with override hooks empty, plus ≥1 override case proving `*-conventions` guidance is honored
- [ ] `kairon eval <agent> --repeat 3` mean ≥95% on allowlisted mid-tier agent and judge models (recorded in the committed results)
- [ ] `kairon eval verify-log <agent>` passes (≥2 iterations, chained, scores match results)
- [ ] `kairon eval audit <agent>` passes (RTCC headings, ≥90% load-bearing)
- [ ] Live prompt equals the certified candidate; `legacy-prompt.md` kept; `task sync:check` passes
- [ ] Krew-lead certified in its dispatch-only Stage 4 role; validator certified as an adversarial reviewer

Harness:
- [ ] E1–E12 and E14 merged (E13 optional); `task eval:selftest` passes
- [x] PR #263 and PR #194 closed; issue #115 closed (2026-10-05)

---

### S3.8 Forward compatibility with Stages 4–5 (reviewed 2026-10-05)

Every Stage 3 decision was checked against `stage-4.md` and `stage-5.md`, so Stage 3 work doesn't have to be redone later.

| Stage 3 decision | Stage 4/5 pressure | Resolution |
|------------------|--------------------|------------|
| Inference backend abstraction (E1) | Stage 5 §5: custom harness for runtime inference | Lives in a package the runtime can use; the direct-API backend serves runtime and evals alike |
| Provenance (E3) | Stage 4 §5: per-step model, tokens, cost | Recorded per agent/judge call with an `estimated` flag; same shape for production steps |
| Check engine (E5) | Stage 4 §1 "95%+ on real work"; §2 deterministic guardrails between steps | Runs on any directory/output/log, so the same checks can grade real runs and act as guardrails |
| `Verify:` lines (issue standard rule 2) | Stage 4 §2: deterministic checks between steps | Fixed machine-runnable syntax the engine can execute |
| Completion reports (builder, validator) | Stage 4 §3: fail-workflow vs punch-to-human | Status `completed` / `failed` / `blocked-needs-human` (validator: `pass` / `fail` / `blocked-needs-human`) |
| Sub-agent inputs | Stage 5 §1–2: dispatch-only coordinator, narrow tool manifests | Inputs arrive in the dispatch message or workspace files; sub-agents don't fetch from GitHub (planner excepted) |
| Validator rebuild | Stage 4 §2, Stage 5 §3: adversarial review in its own context | Rebuilt as an adversarial reviewer; deterministic command checks move to engine guardrails |
| Krew-lead rebuild | Stage 4: every agent at the Stage 3 bar; Stage 5 §1: dispatch-only | Rebuilt once against its Stage 4 role, dispatch-only through a Kairon MCP dispatch tool (Step 3+ note; agreed 2026-10-05) |
| Planner gates | Stage 4 §3: punch-outs actively bypass-tested | P1 bypass case gives evidence for the intake gate; engine-enforced gates remain Stage 4 work |
| Agent definitions | Stage 5: the harness loads agents | `.kiro/agents/*.json` stays the format the custom harness reads |

**Confirmed compatible, no change:**
- Sentinel files: Stage 4 §2 names "hooks and sentinel files".
- Isolated workspaces and the fake gh: the "no CLI shells" rule covers inference, and agents keep shell tools.
- Validator context isolation: its evals only see the issue, the spec and the workspace.
- Committed results and iteration logs: these satisfy Stage 5 §4.
- Candidate-prompt iterations.
- Multi-turn semantics: E9 already has a constraint for the API backend.

**Sequencing fact (verified 2026-10-05):** `kiro-cli chat --output-format stream-json` reports the session id, context-usage percentage and turn duration, but no token counts, cost or model. Stage 4 §5's per-step token/cost data therefore requires the direct-API backend for agent invocations. That makes the custom harness a Stage 4 prerequisite, not only a Stage 5 one.

---

## Stage 4 — Workflow Certification

Stage 4 evaluates workflow orchestration, guardrails, punch-outs, success rates, and audit trails.

### 1. Workflow Definition

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Multiple Stage 3 agents wired end-to-end | 🟡 Partial | 5 agents defined (krew-lead, architect, builder, validator, documenter). Wiring is prompt-instructed, not code-enforced. |
| Handoffs and branching logic documented | ✅ Implemented | krew-lead-prompt.md has detailed workflow phases, handoff rules, and branching (retry vs escalate). |
| Every agent passes Stage 3 quality bar (95%+) | 🟡 Partial | Quality bar defined but no recorded 95%+ runs. Agents theoretically pass but unproven. |
| No agents requiring regular manual correction | 🟡 Partial | No metrics tracked. Anecdotally, agents work but correction frequency unknown. |

**Critical Gap:** The `plan.Executor` in `internal/plan/executor.go` is **dead code** — it has zero non-test callers. The designed workflow architecture (concurrent execution, deterministic gating) exists only in tests and prompts, not in the running system.

**Files assessed:**
- `.kiro/agents/krew-lead-prompt.md` — Detailed workflow definition
- `internal/plan/executor.go` — Unwired dead code
- `internal/plan/registry.go` — trustedAgents enforcement works

#### Planner Prompt — Workflow Definition

```
Create a GitHub issue to wire the plan executor into runtime for Stage 4 compliance.

**Problem:**
Kairon has a well-designed workflow architecture in internal/plan/executor.go with concurrent task execution, deterministic gating, and proper handoffs — but it's dead code with zero non-test callers. The running system relies entirely on the krew-lead LLM following prompt instructions rather than engine-enforced workflow execution. This means workflow guarantees depend on LLM compliance, not deterministic code.

**Acceptance Criteria:**
1. Wire plan.Executor into the runtime execution path (internal/agent/ or internal/session/)
2. The watcher → krew-lead flow should use Executor for task dispatch rather than relying on krew-lead's prompt-driven subagent calls
3. Executor's deterministic gating (task dependencies, sentinel checking) becomes the enforced workflow
4. Add an integration test proving the full architect→builder→validator→documenter chain runs through the executor
5. krew-lead becomes a plan-parser and status-reporter, not the workflow engine itself

**Scope:**
- Connect Executor to runtime (significant refactor)
- Per-task filesystem/git isolation for concurrent execution
- Integration test for full workflow
- This is the critical Stage 4 blocker
```

---

### 2. Guardrails

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Adversarial review agents | 🟡 Partial | Validator exists but is **conformance-checking**, not adversarial. It verifies specs were followed, doesn't challenge/catch/detect producer errors independently. |
| Hooks and sentinel files | ✅ Implemented | Sentinel protocol in `.kiro/skills/sentinel-protocol/`. Agents write completion sentinels; krew-lead reads them. |
| Guardrails sit between steps, not inside | 🟡 Partial | Sentinel checks are between steps. But enforcement is prompt-instructed (krew-lead told to check), not engine-enforced. |
| Must be automated (human review = Stage 2) | 🟡 Partial | Guardrails are automated in design. But krew-lead compliance with guardrail instructions is LLM-dependent. |

**Files assessed:**
- `.kiro/agents/validator-prompt.md` — Conformance checker, not adversarial
- `.kiro/skills/sentinel-protocol/SKILL.md` — Sentinel file protocol
- `internal/plan/validator.go` — Plan validation (deterministic)

#### Planner Prompt — Guardrails Enforcement

```
Create a GitHub issue to upgrade guardrails for Stage 4 compliance.

**Problem:**
Kairon's guardrails exist but have two gaps: (1) the validator is a conformance checker that verifies specs were followed, not an adversarial reviewer that independently challenges outputs, and (2) guardrail enforcement (sentinel checking, gating) is prompt-instructed rather than engine-enforced. The krew-lead LLM is told to check sentinels; it's not forced to by code.

**Acceptance Criteria:**
1. Upgrade validator-prompt.md to include adversarial review patterns:
   - Challenge prior step outputs, don't just verify conformance
   - Look for edge cases the builder missed
   - Attempt to break/exploit the implementation
   - Distinct adversarial lens (catch/detect/block)
2. Move sentinel checking from krew-lead prompt instructions to engine code:
   - Executor should refuse to advance tasks until predecessor sentinels exist
   - Validator exit code should be checked by engine, not by krew-lead's judgment
3. Add eval criteria for validator adversarial effectiveness

**Scope:**
- Validator prompt rewrite for adversarial posture
- Engine-enforced gating in Executor
- New eval cases for adversarial review
```

---

### 3. Punch-Out Evidence

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Explicit human decision points | 🟡 Partial | Draft Review gate exists in krew-lead (user approves spec). Label Confirmation gate. But these are prompt-instructed, not enforced. |
| Actively tested (bypass attempted and blocked) | ❌ Not Implemented | No tests verify that bypassing human gates fails. The error-recovery eval tests retry reasoning, not bypass blocking. |
| Clear fail-workflow vs punch-to-human separation | 🟡 Partial | Conceptually separated (retry exhaustion → `-failed` label vs approval gates). No test proving the separation. |
| Paper-only punch-outs don't qualify | ❌ Not Implemented | Current punch-outs exist on paper (in prompts) but have never been bypass-tested. |

**Files assessed:**
- `.kiro/agents/krew-lead-prompt.md` — Gates defined in prompt
- `.kairon/evals/cases/krew-lead/error-recovery.yaml` — Tests retry reasoning, not bypass

#### Planner Prompt — Punch-Out Testing

```
Create a GitHub issue to implement and test punch-out points for Stage 4 compliance.

**Problem:**
Kairon has human decision points defined in prompts (Draft Review gate, Label Confirmation gate, retry exhaustion escalation) but they've never been actively tested. Stage 4 requires "someone attempted to bypass and was blocked." Current punch-outs exist only on paper.

**Acceptance Criteria:**
1. Identify all punch-out points in the workflow:
   - Draft Review gate (user must approve spec before build)
   - Label Confirmation (kairon-done applied only after validation passes)
   - Retry exhaustion (max_retries reached → human escalation, not silent continue)
2. Write integration tests that attempt to bypass each:
   - Test: Try to proceed past Draft Review without approval → assert blocked
   - Test: Try to apply kairon-done without passing validation → assert blocked
   - Test: Exhaust retries → assert workflow halts with -failed label and incident logged
3. Tests must be deterministic (not LLM-eval based)
4. Document each punch-out point in .kairon/docs/punch-outs.md

**Scope:**
- Integration tests for bypass blocking
- Punch-out documentation
- May require engine-level enforcement (ties to Executor wiring)
```

---

### 4. End-to-End Success Rate

| Criterion | Status | Evidence |
|-----------|--------|----------|
| End-to-end number tracked | ❌ Not Implemented | No success rate metrics exist. Per-issue pass/fail is tracked via labels but not aggregated. |
| Measured across full workflow | ❌ Not Implemented | No measurement infrastructure. Would need: issues attempted → PRs created → PRs merged. |
| Trend showing stability/improvement | ❌ Not Implemented | No historical tracking. |
| Know your number or haven't reached Stage 4 | ❌ Not Implemented | The number is unknown. |

**Files assessed:**
- `.kairon/retries/` — Per-issue retry counts (not success rates)
- No metrics, dashboards, or success tracking found

#### Planner Prompt — Success Rate Tracking

```
Create a GitHub issue to implement end-to-end success rate tracking for Stage 4 compliance.

**Problem:**
Kairon has no end-to-end success rate measurement. The framework tracks per-issue retries and applies pass/fail labels, but doesn't aggregate this into a success rate. Stage 4 requires knowing "the end-to-end number" — what percentage of issues that enter the workflow result in successfully merged PRs?

**Acceptance Criteria:**
1. Define the success metric: Issues with `kairon` label → PRs created → PRs merged (3-stage funnel)
2. Add a `kairon metrics` command that calculates:
   - Total issues processed
   - Success rate (kairon-done / total)
   - Failure rate (kairon-failed / total)
   - Average time to completion
3. Persist metrics history in .kairon/metrics/ for trend analysis
4. Add a weekly/monthly success rate report capability
5. Track the metric over at least 2 weeks to establish a baseline trend

**Scope:**
- New `metrics` command in cmd/kairon/
- Metrics persistence and history
- Trend visualization (text or simple chart)
```

---

### 5. Audit Trail

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Structured logs identifying step → output | 🟡 Partial | JSON logger exists (`internal/logging/`) but is debug/on-demand, not always-on. |
| Trace failure to exact origin step | 🟡 Partial | Exit codes tracked. Sentinel files identify which agent completed. But no correlation IDs linking failures to specific steps. |
| Per-step model and token/cost data | ❌ Not Implemented | Token/cost tracking exists only in eval subsystem (`internal/eval/types.go` CostInfo), not in live execution. |
| Coverage of all workflow steps | 🟡 Partial | Sentinel protocol covers completion. But intermediate outputs and failures aren't comprehensively logged. |

> **Note (2026-10-05):** kiro-cli exposes no token, cost or model data (see §S3.8), so per-step token/cost requires the direct-API backend for agent invocations.

**Files assessed:**
- `internal/logging/` — JSON logger, debug-mode
- `internal/eval/types.go` — CostInfo (eval only)
- `.kairon/specs/issue-239-add-structured-logging-with-live-viewer.md` — Spec exists
- `.kairon/specs/issue-252-logging-json-formatter.md` — Spec exists

#### Planner Prompt — Audit Trail

```
Create a GitHub issue to complete audit trail infrastructure for Stage 4 compliance.

**Problem:**
Kairon has partial audit infrastructure (JSON logger, sentinel files, exit codes) but doesn't meet Stage 4 requirements: structured logging is debug-only not always-on, failures can't be traced to exact origin steps via correlation IDs, and token/cost data is tracked only in the eval subsystem, not in live workflow execution.

**Acceptance Criteria:**
1. Make structured JSON logging always-on for workflow execution (not just debug mode)
2. Add correlation IDs:
   - run_id: unique per watcher invocation
   - issue_id: the GitHub issue number
   - task_id: the specific workflow task (architect, builder, validator, documenter)
3. Persist execution manifest per issue in .kairon/sessions/<issue>/:
   - Per-step: status, start/end time, exit code, sentinel path, captured output summary
   - Per-step: model used, input tokens, output tokens, estimated cost
4. Add `kairon audit <issue>` command to display the execution trace
5. Token/cost capture requires kiro-cli to expose usage data (may need upstream feature request)

**Scope:**
- Always-on structured logging
- Correlation IDs throughout execution path
- Execution manifest persistence
- Audit command
- Upstream dependency on kiro-cli for token/cost (document if blocked)
```

---

## Stage 5 — Multi-Workflow Orchestration

Stage 5 evaluates coordinator isolation, sub-agent scoping, adversarial context separation, version control, and Stage 4 continuity.

### 1. Dispatch-Only Coordinator

| Criterion | Status | Evidence |
|-----------|--------|----------|
| No direct tool access | ❌ Not Implemented | krew-lead.json grants: `read`, `shell`, `subagent`, `todo_list`. It has direct file and shell access. |
| Dispatch surface only | ❌ Not Implemented | krew-lead can read files, run shell commands, and maintain todo lists — not dispatch-only. |
| ✗ Mega-agent anti-pattern | ❌ **VIOLATED** | krew-lead IS the mega-agent anti-pattern: it holds real-work tools and orchestrates. |

**Files assessed:**
- `.kiro/agents/krew-lead.json`:
  ```json
  "tools": ["read", "shell", "subagent", "todo_list"]
  ```

#### Planner Prompt — Dispatch-Only Coordinator

```
Create a GitHub issue to refactor krew-lead into a dispatch-only coordinator for Stage 5 compliance.

**Problem:**
krew-lead violates the Stage 5 "Dispatch-Only Coordinator" requirement. It currently has tools: ["read", "shell", "subagent", "todo_list"]. A Stage 5 coordinator should ONLY have dispatch verbs (subagent/launch/invoke) — no direct file access, shell access, or workflow steps of its own. krew-lead is currently the "mega-agent counterfeit" anti-pattern.

**Acceptance Criteria:**
1. Reduce krew-lead.json tools to dispatch-only: `["subagent"]`
2. Remove `read`, `shell`, `todo_list` from krew-lead
3. Any file reading krew-lead needs (issue content, sentinel checking) should be:
   - Passed to it by the engine (preferred), or
   - Delegated to a utility sub-agent
4. Sentinel checking should be engine-enforced, not krew-lead doing shell `test -f`
5. Update krew-lead-prompt.md to reflect dispatch-only role
6. Ensure trustedAgents enforcement remains intact

**Scope:**
- krew-lead.json tool reduction
- krew-lead-prompt.md rewrite for dispatch-only
- Engine must provide inputs krew-lead currently reads itself
- Major architectural change — ties to Executor wiring
```

---

### 2. Sub-Agents Scoped to One Workflow

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Per-workflow tool manifest | 🟡 Partial | Each agent has `tools`/`allowedTools` in JSON config. But they're declared, not Kairon-enforced (relies on kiro-cli). |
| Stage 3 guarantees maintained | 🟡 Partial | See Stage 3 assessment — partial compliance. |
| Agents needing manual correction disqualify | 🟡 Partial | Not measured. Assumed compliant but unproven. |

**Current agent tool grants:**
| Agent | Tools | Assessment |
|-------|-------|------------|
| architect | read, write, shell | **Over-broad**: has write but only needs read |
| builder | read, write, shell | Appropriate for implementation work |
| validator | read, shell | **Over-broad**: "read-only" claim but has shell |
| documenter | read, write, shell | Appropriate |
| planner | read, write (allowedPaths restricted) | Well-scoped with path restrictions |

> **Correction pending (noted 2026-10-05; revisit in the Stage 5 pass):** `validator.json` grants
> `read, write, shell` and `documenter.json` grants `read, write`, so the validator and documenter rows above
> are stale. `*-conventions` entries in `resources` are intentional project-override hooks, not missing files.

**Files assessed:**
- All `.kiro/agents/*.json`

#### Planner Prompt — Sub-Agent Scoping

```
Create a GitHub issue to tighten sub-agent tool scoping for Stage 5 compliance.

**Problem:**
Kairon's sub-agents have tool declarations but two are over-scoped: (1) architect has write access but should be read-only (it produces specs, not code), (2) validator has shell access despite being described as "read-only verification." Tool limits are declared in JSON but enforced by kiro-cli, not Kairon — Kairon's orchestrator doesn't validate that dispatched agents have minimal tools for their step.

**Acceptance Criteria:**
1. Architect: Remove `write` tool, keep only `read` and `shell` (for code exploration)
2. Validator: Remove `shell` tool or restrict to read-only commands (no write side effects)
   - Consider adding `toolsSettings.shell.autoAllowReadonly: true` enforcement
3. Add `allowedPaths` restrictions:
   - Validator: can only write to `.kairon/sentinels/`
   - Architect: can only write to `.kairon/specs/`
4. Add orchestrator-side validation in registry.go or executor:
   - Before dispatching, verify agent's tool set is minimal for the step type
5. Document the tool scoping rationale in .kairon/docs/agent-security.md

**Scope:**
- JSON config changes for architect and validator
- Optional: orchestrator-side tool validation
- Documentation
```

---

### 3. Adversarial Review in Its Own Context

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Distinct context (own window) | ✅ Implemented | Validator runs as separate kiro-cli process with own context window. |
| Never sees producer's message history | ✅ Implemented | Communication is file/sentinel-based. Validator re-reads issue and spec, doesn't inherit builder's chat. |
| Context sharing = second opinion, not adversarial | ✅ Implemented | Architecture is correct — separate process, file-based communication, no context sharing. |

**However:** While architecturally correct, there's no test proving context isolation, and the shared worktree is a theoretical influence surface.

**Files assessed:**
- `internal/agent/manager.go` — Separate process spawning
- `.kiro/skills/sentinel-protocol/SKILL.md` — File-based communication

#### Planner Prompt — Adversarial Context Verification

```
Create a GitHub issue to verify and document adversarial review context isolation for Stage 5 compliance.

**Problem:**
Kairon's validator architecturally runs in its own context (separate kiro-cli process, file-based communication, no message history sharing) — this is correct. However, there's no test proving the isolation, and the shared worktree is a theoretical surface for producer influence. Stage 5 requires demonstrated isolation, not just designed isolation.

**Acceptance Criteria:**
1. Add an integration test proving context isolation:
   - Builder writes a "trap" message in its chat history
   - Validator runs and should NOT reference or be influenced by the trap
   - Assert validator's output is independent of builder's chat content
2. Document the isolation architecture in .kairon/docs/adversarial-review.md:
   - Separate process spawning
   - File-based communication (sentinel protocol)
   - No message history inheritance
   - Worktree as shared surface (acknowledged risk)
3. Consider: Should validator run in a separate worktree clone? (May be overkill)

**Scope:**
- Integration test for isolation
- Architecture documentation
- Risk acknowledgment for shared worktree
```

---

### 4. Version-Controlled Repository

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Coordinator, sub-agents, workflow definitions committed | ✅ Implemented | All in Git: `.kiro/agents/*.json`, `*-prompt.md`, skills, templates |
| Meaningful commit history | ✅ Implemented | Git log shows evolution. PRs document changes. |

**Files assessed:**
- All agent configs and prompts are tracked in Git
- `.kairon/specs/` contains issue specifications
- Meaningful PR history (274, 276, 277, 279, 280, etc.)

**Status: ✅ Fully Implemented** — No action needed.

---

### 5. Stage 4 Continuity

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Fully custom harness (no CLI shells for inference) | ❌ Not Implemented | krew-lead uses shell tool for runtime operations. Agents are spawned via kiro-cli (external tool). |
| Punch-outs honored at coordinator boundary | 🟡 Partial | Punch-outs exist in prompts but aren't engine-enforced at coordinator level. |
| Audit trail with per-step model/token/cost | ❌ Not Implemented | See Stage 4 §5 — not implemented in live execution. |

**Status:** Inherits Stage 4 gaps. Must complete Stage 4 before Stage 5 continuity can be claimed.

> **Harness note (2026-10-05):** Stage 5 certification will call the LLM API directly instead of spawning
> kiro-cli. kiro-cli remains available to users as an option. The Stage 3 eval design (§S3.5) routes all inference
> through one pluggable backend (`--backend`) and scores artifacts rather than kiro-cli output, so the same suites re-certify
> agents on the API harness.

---

## Summary

### Stage 3 — Task Certification

All 6 agents must pass, including the planner. See §S3.6 for the issue prompts and dependency order.

| Track | Issues | Status |
|-------|--------|--------|
| Step 0: Eval harness | E1–E12, E14 (E13 optional) | 🔴 Not started |
| planner | P1 spec → P2 cases + legacy baseline → P3 iteration 00 → ≥2 iterations → P-final | 🔴 Not started |
| builder | B1 → B2 → B3 → ≥2 iterations → B-final | 🔴 Not started |
| validator, architect, documenter, krew-lead | Same pattern; prompts to be drafted | 🔴 Not started |

**Stage 3 Readiness: ~10%.** The prompts exist and show some deliberate design, but nothing under Evaluation or
Iteration is met. The blocker is the eval harness: unsafe to run, keyword-based checks, an unpinned judge, and
no enforced thresholds.

### Stage 4 — Workflow Certification

| Area | Status | Blocking Items |
|------|--------|----------------|
| Workflow Definition | 🟡 Partial | **Critical**: Executor is dead code, unwired |
| Guardrails | 🟡 Partial | Engine enforcement, adversarial validator |
| Punch-Out Evidence | ❌ Not Implemented | Bypass tests don't exist |
| End-to-End Success Rate | ❌ Not Implemented | No metrics infrastructure |
| Audit Trail | 🟡 Partial | Always-on logging, token/cost in live path |

**Stage 4 Readiness: ~30%** — Architecture is designed; enforcement and measurement are missing.

### Stage 5 — Multi-Workflow Orchestration

| Area | Status | Blocking Items |
|------|--------|----------------|
| Dispatch-Only Coordinator | ❌ Not Implemented | **Critical**: krew-lead is mega-agent |
| Sub-Agents Scoped | 🟡 Partial | Over-broad tools on architect/validator |
| Adversarial Review Context | ✅ Implemented | Just needs verification test |
| Version-Controlled Repository | ✅ Implemented | Complete |
| Stage 4 Continuity | ❌ Not Implemented | Blocked by Stage 4 gaps |

**Stage 5 Readiness: ~40%** — Good foundations (version control, context isolation) but coordinator architecture violates core principle.

---

## Recommended Progression

### Phase 1 — Stage 3: Eval-Driven Agent Rebuilds

Issue prompts and dependencies are in §S3.6. P1 and B1 are docs-only and can start alongside Step 0.

**Step 0: Eval harness** (serial order for a single Kairon runner)
- [ ] E1 Pluggable inference backend + stub self-test (Stage 5 seam)
- [ ] E2 One scoring path (refactor)
- [ ] E3 Pin and record models and prompt provenance
- [x] E4 superseded by the container sandbox series (#296–#300)
- [ ] E5 Deterministic pass/fail checks
- [ ] E6 Yes/no judge checks
- [ ] E7 Agent-level pass threshold and exit code
- [ ] E9 Multi-turn cases
- [ ] E14 Answer-bank simulated user (planner elicitation)
- [ ] E10 `--prompt-file` candidate override
- [ ] E11 Iteration log format + `kairon eval verify-log`
- [ ] E12 `kairon eval audit` (RTCC + load-bearing)
- [ ] E8 `--repeat N`
- [ ] E13 (optional) Direct-API judge backend

**Step 1: Planner (bootstrap)**
- [ ] P1 Eval specification (docs only)
- [ ] P2 Eval cases + legacy baseline
- [ ] P3 Iteration 00 (minimal RTCC candidate)
- [ ] P-iter × ≥2 (created one at a time from the previous results)
- [ ] P-final Promote and certify. *Then use the certified planner for the remaining issues.*
- [ ] P-override (optional) Replace Kairon's planner-conventions override with Kairon-specific issue guidance

**Step 2: Builder**
- [ ] B1 Eval specification (docs only)
- [ ] B2 Eval cases + legacy baseline
- [ ] B3 Iteration 00
- [ ] B-iter × ≥2
- [ ] B-final Promote and certify (manual merge; smoke-test on one real issue)

**Step 3+: Validator (adversarial reviewer) → Architect → Documenter → Krew-lead** (same pattern; prompts to be drafted). Krew-lead is rebuilt against its dispatch-only Stage 4 role and needs the engine dispatch tool first, which overlaps Phase 2.

**Stage 3 complete when:** the §S3.7 checklist is satisfied for all 6 agents.

### Phase 2 — Stage 4: Workflow Enforcement

- [ ] Direct-API agent backend (required for per-step token/cost; see §S3.8)
- [ ] Engine dispatch tool and between-step guardrails (also a prerequisite for krew-lead's Stage 3 rebuild)
- [ ] Wire Executor into runtime
- [ ] Success rate metrics (`kairon metrics` command)
- [ ] Punch-out bypass testing
- [ ] Audit trail completion

### Phase 3 — Stage 5: Orchestration Isolation

- [ ] Refactor krew-lead to dispatch-only
- [ ] Tighten sub-agent scoping
- [ ] Adversarial context verification test
