# Step 0 Validation — Eval Harness Readiness for Stage 3 and Stage 4

**Validated:** 2026-10-16, against `main` @ `694d0c3` (all Step 0 issues E1–E12, E14 and E8 merged)
**Assessed against:** `stage-3.md`, `stage-4.md` (this folder)
**Method:** read the eval harness code (`internal/eval`, `internal/inference`, `cmd/kairon/cmd/eval*.go`) and asked,
for each certification criterion, whether the harness can produce evidence that would survive an examiner's check.
Previous PRs, the Step 0 task list and the existing rubrics and cases were deliberately not used: the prompts and
evals will be rewritten. Spot checks: one stub self-test run, and a probe of `kiro-cli` 2.31.1 output.

## Verdict

Not ready. Four gaps (1–4) would make Stage 3 evidence wrong or unverifiable and must close before the planner
rebuild (P2) starts scoring. Four more (5–8) weaken the evidence. Stage 4 needs five more harness capabilities
(9–13). Each finding has a GitHub issue (see the table).

## Findings

| # | Stage | Finding | Issue | Depends on |
|---|-------|---------|-------|------------|
| 1 | 3 | Criteria without checks in a case fall back to the legacy 1–5 judge or keyword heuristics | #336 | — |
| 2 | 3 | Project override hooks and untracked `.kiro/` files reach eval workspaces; provenance hashes the repo, not the workspace | #337 | — |
| 3 | 3 | Runs don't record suite identity or completeness; verify-log doesn't compare models or suite | #343 | #337 |
| 4 | 3 | Red/Green/Refactor isn't machine-checkable | #348 | #343 |
| 5 | 3 | Audit ignores the Evidence column and counts duplicate lines | #338 | — |
| 6 | 3 | Nothing enforces ≥3 distinct criteria covered by checks | #344 | #338 |
| 7 | 3 | Judge checks can't see the conversation or target a turn | #339 | — |
| 8 | 3 | Pooled-point scoring lets a criterion fail completely at 95%; no per-criterion scores | #342 | #336 |
| 9 | 4 | Every call is priced at Sonnet rates; cost code lives in `internal/eval` | #340 | — |
| 10 | 4 | Workflow cases can't name sub-agent versions or attribute calls to steps | #345 | #337 |
| 11 | 4 | Sandboxed kiro-cli agents can't run a `kairon` MCP server | #341 | — |
| 12 | 4 | No end-to-end attempt success rate and no trend across runs | #347 | #342, #343 |
| 13 | 4 | The check engine can't be reused as a guardrail or run on real work | #346 | #336, #339 |

Dependencies are declared in each issue body as `Dependencies: #N, #M`, the single-line form the watcher parses
(`README.md`, "Issue Dependencies"). Pick-up order: wave 1 = #336, #337, #338, #339, #340, #341; wave 2 = #342,
#343, #344, #345, #346; wave 3 = #347, #348. Some dependencies exist only to serialize edits to the same code
(#338→#344 audit, #336 and #339→#346 checks).

## Stage 3 detail

### 1. Criteria without checks fall back to legacy scorers

*Criterion:* substantive quality checks; clear pass/fail thresholds (stage-3 §2).
`scoreCase` (`internal/eval/scoring.go:98`, `:106`) scores every rubric criterion on every case. A criterion with no
checks in that case goes to the 1–5 LLM judge (5 points) or, when `deterministic: true`, to the stdout keyword
heuristics (`internal/eval/runner.go:1323`). Confirmed on a stub run: `selftest/check-file-exists` has one check (1
point) and also scored 5/5 from a judge call on `clarity`, which it doesn't check.
*Needed:* an opt-in check-only rubric mode in which an unchecked criterion is absent from that case's score.

### 2. Override hooks and local files in workspaces; provenance from the wrong files

*Criterion:* certification with override hooks empty (gap-analysis §S3.5 item 10, §S3.7); results tie a score to
the exact prompt (stage-3 §3).
`stageKiro` (`internal/eval/workspace.go:592`) copies the project's whole `.kiro/` into every workspace, including
Kairon's `builder-conventions`, `planner-conventions`, `validator-conventions` (29 KB) and untracked
`.kiro/settings/`. Provenance (`internal/eval/provenance.go`) resolves `resources` against the repository working
directory, so a fixture-supplied hook isn't what `prompt_sha256` records, and editing Kairon's own override changes
the agent hash, which verify-log accepts as a prompt iteration. Found by reading the code; keeping a workspace to
confirm it was blocked by a host safety policy.

### 3. Run identity and completeness

*Criterion:* chronological progression with clear cause and effect (stage-3 §3).
Runs record no hash of the rubric, cases, fixtures, hidden files or mocks, so a `prompt` iteration can't prove the
suite stayed the same, and an `eval` iteration can't prove it changed. `loadRunRecord`
(`internal/eval/verifylog.go:580`) reads only the score and `prompt_sha256`, never the models or sandbox mode. An
interrupted run keeps a verdict in `summary.json` (only `.progress` marks it), and a `--case` run
(`runner.go:301`) writes a full summary with a verdict; verify-log accepts both.

### 4. Red/Green/Refactor

*Criterion:* evidence of EDD discipline (stage-3 §3).
Iteration logs carry only `change_type: prompt|eval`. Nothing checks that a Red step added a failing check, a Green
step fixed one, or a Refactor kept the score.

### 5. Audit evidence and duplicates

*Criterion:* ≥90% load-bearing; no dead weight (stage-3 §1).
`loadAuditTable` (`internal/eval/audit.go:483`) never reads the Evidence column (documented as free text), so a row
can map to a criterion that no case checks. `extractInstructions` counts each occurrence (`audit.go:446`), so a
repeated rule counts as several load-bearing lines.

### 6. Three distinct criteria

*Criterion:* ≥3 distinct evaluation criteria (stage-3 §2).
`loadRubrics` (`runner.go:1687`) validates only `pass_threshold`; two real criteria plus a `cost` entry look like
three, and an unchecked criterion still counts. The audit is the natural gate.

### 7. Judge checks and the conversation

*Criterion:* substantive checks of conversational behavior (planner criteria in gap-analysis §S3.6).
A judge check sees the user messages (`renderJudgeTurns`, `runner.go:1524`) and the final output only
(`buildYesNoPrompt`, `internal/eval/judge_check.go:24`), and has no `turn` field.

### 8. Score weighting

*Criterion:* clear pass/fail thresholds (stage-3 §2).
`agentScoreTotals` (`scoring.go:219`) pools points across all checks. A one-check criterion can score 0 while the
agent passes at 95% if the other criteria have 19 or more checks. Results show no per-criterion percentage.

## Stage 4 detail

### 9. Per-step cost

*Criterion:* per-step model, input tokens, output tokens and cost (stage-4 §5).
`costFromUsage` (`internal/eval/types.go:188`) prices every model at $3/$15 per million tokens. kiro-cli 2.31.1
reports no usage or model: `--output-format stream-json` emits `sessionId`, `contextUsagePercentage`,
`turnDurationMs`, `stopReason` and `finalText` only (probed 2026-10-16), so every kiro-cli record is an estimate.
The issue covers honest per-model pricing in a shared package. **Real usage needs a direct-API backend**, which is
not filed: it needs a provider decision (gap-analysis E13, §S3.8).

### 10. Workflow step attribution

*Criterion:* trace a failure to its exact origin step; coverage of all steps (stage-4 §5).
A case evaluates one agent. Sub-agents dispatched inside kiro-cli are invisible: one call record for the whole
workflow, and `prompt_sha256` covers only the dispatching agent.

### 11. `kairon` binary in the sandbox

*Criterion:* workflow cases run contained (they create PRs); krew-lead's only tool is a Kairon MCP dispatch tool
(gap-analysis Step 3+ note).
`buildContainerMounts` (`runner.go:852`) mounts the linux `kairon` binary only for non-kiro-cli backends, at
`/opt/kairon/kairon`, which isn't on the container PATH (`runner.go:810`).

### 12. End-to-end number and trend

*Criterion:* the end-to-end number; trend over time (stage-4 §4).
The verdict (`internal/eval/verdict.go`) pools repeats, so a run can pass at 95% with a third of its attempts
failing. `kairon eval diff` compares two runs; nothing lists a series. Production metrics from the watcher are
separate Stage 4 work (gap-analysis Stage 4 §4).

### 13. Shared check engine

*Criterion:* deterministic guardrails between steps; every agent at 95%+ on real work (stage-4 §1–2).
`EvaluateChecks` (`internal/eval/checks_eval.go:33`) takes only a directory, output and gh log, but lives in
`internal/eval` with the runner and the sandbox, and there's no command to run checks against a real directory.

## Already sufficient

- Model pinning: allowlist pre-flight, `auto` refused, per-call records with model, tokens, cost, duration.
- Fail-closed scoring: skipped or errored criteria count 0; 95% default threshold; non-zero exit on failure.
- Check vocabulary, hidden `inject` tests, `changed_files`, fail-closed yes/no judge checks.
- Multi-turn and answer-bank simulated-user cases (punch-out bypass cases for the planner are expressible).
- `--prompt-file` candidate isolation, `--repeat` with mean, min and max.
- RTCC structure audit; verify-log chain, contiguity, score-match and prompt-change rules.
- Results are not gitignored, so runs can be committed. Output checks read the assistant message; kiro-cli writes
  tool traces to stderr.

## Defaults taken (non-interactive planning)

The issues were drafted with the planner issue standard (gap-analysis §S3.6, Step 1) without an interview; these
choices took the recommended default and can be changed on the issue before work starts.

| Issue | Default |
|-------|---------|
| #336 | Check-only scoring is opt-in per rubric (`checks_only: true`); existing rubrics keep today's scoring |
| #337 | Only `*-conventions` hooks and untracked `.kiro/` files are excluded; the rest of the project `.kiro/` stays |
| #348 | Phases `baseline`, `red`, `green`, `refactor`; a set needs a red→green pair and a refactor |
| #338 | Evidence cells name cases as `case:<name>`; duplicates are always violations |
| #342 | Optional `min_criterion_score` floor; agent `score` unchanged |
| #340 | Prices come from config; models without a price record unknown cost (no default for Haiku) |
| #345 | Sub-agents get their own provenance plus a `workflow_sha256`; step records arrive in `.eval/calls.jsonl` |
| #347 | `kairon eval trend` over committed results; production metrics out of scope |
| #346 | New `kairon check` command over the case `checks` format |
