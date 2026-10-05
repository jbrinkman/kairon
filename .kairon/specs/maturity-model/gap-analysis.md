# Kairon AI Maturity Model — Gap Analysis

**Analysis Date:** 2026-10-04
**Assessed Against:** Improving's AI Maturity Framework (Stages 3, 4, 5)

> This document assesses Kairon's current implementation against each Stage 3, 4, and
> 5 criterion. Each criterion is marked as: ✅ **Implemented**, 🟡 **Partial**, or
> ❌ **Not Implemented**. For partial/not-implemented items, a ready-to-use planner
> prompt is included to create the necessary GitHub issue(s).

---

## Stage 3 — Task Certification

Stage 3 evaluates prompt design, evaluation infrastructure, and iteration discipline.

### Approach: TDD Rebuild Per Agent

The existing agent prompts are **reference material only**. For Stage 3 compliance, we
tear down each agent to nothing and rebuild using Eval-Driven Development (EDD):

1. **Define evals first** — Write ≥3 distinct evaluation criteria that define what the
   agent MUST do
2. **Build prompt to pass evals** — Start with a minimal prompt, run evals, iterate
   until 95%+ scores
3. **Document iterations** — Each change is a logged iteration with baseline → hypothesis
   → change → results → reasoning
4. **Ablation audit** — Verify ≥90% of final prompt is load-bearing (removing it drops
   scores)

This produces: a prompt that demonstrably works, evals that prove it, and an iteration
log showing the journey.

**Agent rebuild order:** planner → builder → validator → architect → documenter → krew-lead

(planner first because we'll use it to create all subsequent issues; builder next as
the core execution unit; krew-lead last because it orchestrates the workflow agents
and its evals depend on them working)

> **Note:** The planner agent is not part of the orchestrated workflow (it creates
> issues, not executes them), but it IS part of the Kairon system and must meet the
> same Stage 3 bar. Our goal is maturity model compliance across all agents, not just
> the minimum for certification. By rebuilding planner first, we bootstrap the process:
> a Stage 3 compliant planner will create better issues for the remaining agents.

---

### Current State Assessment

| Agent | Evals Exist | Prompt Quality | 95%+ Evidence | Iteration Log |
|-------|-------------|----------------|---------------|---------------|
| builder | ✅ 5 cases | 🟡 Good structure | ❌ No | ❌ No |
| validator | ✅ 4 cases | 🟡 Good structure | ❌ No | ❌ No |
| architect | ✅ 4 cases | 🟡 Loose structure | ❌ No | ❌ No |
| documenter | ✅ 3 cases | 🟡 Ad-hoc | ❌ No | ❌ No |
| planner | ❓ TBD | 🟡 Needs review | ❌ No | ❌ No |
| krew-lead | ✅ 6 cases | 🟡 Strong but large | ❌ No | ❌ No |

**Files:**
- Evals: `/Users/jbrinkman/projects/kairon/.kairon/evals/cases/<agent>/`
- Prompts: `/Users/jbrinkman/projects/kairon/.kiro/agents/<agent>-prompt.md`
- Rubrics: `/Users/jbrinkman/projects/kairon/.kairon/evals/rubrics/<agent>.yaml`
- Planner: `/Users/jbrinkman/projects/kairon/.kiro/agents/planner-prompt.md`

---

### Agent 1: Planner — TDD Rebuild (Bootstrap)

The planner agent transforms user intent into well-structured GitHub issues. We rebuild
it first because a Stage 3 compliant planner will create the issues for all other agents.

#### Phase 1: Define Planner Evals

```
Create a GitHub issue to define Stage 3 evaluation criteria for the planner agent.

**Problem:**
The planner agent needs ≥3 distinct evaluation criteria. The planner's job is to take user intent (a problem description or feature request) and produce a well-structured GitHub issue with clear acceptance criteria. Evals should test issue quality.

**Acceptance Criteria:**
1. Define ≥3 distinct evaluation criteria for planner. Suggested criteria:
   - Issue clarity: Is the issue title clear and the body well-structured?
   - Acceptance criteria quality: Are ACs specific, measurable, and testable?
   - Scope appropriateness: Is the issue appropriately scoped (not too broad)?
2. Each criterion must have clear pass/fail threshold
3. Write eval cases in .kairon/evals/cases/planner/ with:
   - Fixture: sample user intent/description
   - Expected issue structure
4. Verify evals run on mid-tier model
5. Document criteria rationale in .kairon/evals/cases/planner/README.md

**Scope:**
- Create or rebuild planner eval cases
- One agent only (planner)
- Do NOT modify the planner prompt yet
```

#### Phase 2: Build Planner Prompt via EDD

```
Create a GitHub issue to rebuild the planner prompt using Eval-Driven Development.

**Problem:**
The planner prompt needs EDD rebuild. Start minimal, run evals, iterate to 95%+. The existing planner-prompt.md is reference material only.

**Acceptance Criteria:**
1. Create new planner prompt starting minimal
2. Run planner evals, record baseline scores
3. Iterate using Red/Green/Refactor until 95%+ scores
4. Document each iteration in .kairon/iterations/planner/:
   - Minimum 2 documented iterations
   - Each with: baseline, hypothesis, change, results, reasoning
5. Final prompt passes Load-Bearing audit
6. Sync to cmd/kairon/templates/kiro/agents/planner-prompt.md

**Scope:**
- Rebuild planner-prompt.md from scratch
- One agent only (planner)
```

---

### Agent 2: Builder — TDD Rebuild

The builder agent executes implementation tasks. Rebuild it from scratch using EDD.

#### Phase 1: Define Builder Evals

```
Create a GitHub issue to define Stage 3 evaluation criteria for the builder agent.

**Problem:**
The builder agent needs ≥3 distinct, substantive evaluation criteria that define what a Stage 3 compliant builder prompt must achieve. Existing evals in .kairon/evals/cases/builder/ can be reviewed as reference but should be rebuilt to ensure they are substantive quality checks, not window dressing.

**Acceptance Criteria:**
1. Define ≥3 distinct evaluation criteria for builder. Suggested criteria:
   - Task completion: Given a spec, does the builder produce working code?
   - Spec compliance: Does the output match the spec's acceptance criteria?
   - Code quality: Does the code follow project conventions (formatting, testing)?
2. Each criterion must be testable with a clear pass/fail threshold (default 80%)
3. Write eval cases in .kairon/evals/cases/builder/ with:
   - Fixture: sample spec input
   - Expected behavior description
   - Rubric reference
4. Verify evals run on mid-tier model (claude-sonnet-4) not frontier
5. Document criteria rationale in .kairon/evals/cases/builder/README.md

**Scope:**
- Review and rebuild builder eval cases
- One agent only (builder)
- Do NOT modify the builder prompt yet
```

#### Phase 2: Build Builder Prompt via EDD

```
Create a GitHub issue to rebuild the builder prompt using Eval-Driven Development.

**Problem:**
The builder prompt needs to be rebuilt from scratch using EDD. Start with a minimal prompt, run the builder evals defined in the previous issue, and iterate until achieving 95%+ scores. The existing builder-prompt.md is reference material only — we are not patching it, we are replacing it.

**Acceptance Criteria:**
1. Create a new builder prompt starting minimal (just role + core task)
2. Run builder evals, record baseline scores
3. Iterate using Red/Green/Refactor:
   - Red: Identify failing/low-scoring criterion
   - Green: Add minimal prompt content to pass it
   - Refactor: Remove any content that doesn't improve scores
4. Achieve 95%+ overall score on builder evals
5. Document each iteration in .kairon/iterations/builder/:
   - iteration-builder-01.md: Baseline scores, hypothesis, change, results, reasoning
   - iteration-builder-02.md: Next iteration
   - (minimum 2 iterations required)
6. Final prompt must pass Load-Bearing audit: removing any section drops scores

**Scope:**
- Rebuild builder-prompt.md from scratch
- One agent only (builder)
- Produces: new prompt + iteration log + 95%+ evidence
```

---

### Agent 3: Validator — TDD Rebuild

The validator agent verifies builder output meets acceptance criteria.

#### Phase 1: Define Validator Evals

```
Create a GitHub issue to define Stage 3 evaluation criteria for the validator agent.

**Problem:**
The validator agent needs ≥3 distinct evaluation criteria. The validator's job is to verify that builder output meets the spec's acceptance criteria and block bad work. Evals should test this verification capability.

**Acceptance Criteria:**
1. Define ≥3 distinct evaluation criteria for validator. Suggested criteria:
   - Correct acceptance: Does validator PASS work that meets all acceptance criteria?
   - Correct rejection: Does validator FAIL work that misses acceptance criteria?
   - Specific feedback: Does validator provide actionable feedback on failures?
2. Each criterion must have clear pass/fail threshold
3. Write eval cases in .kairon/evals/cases/validator/ with:
   - Fixture: sample builder output + original spec
   - Cases for both good and bad builder output
   - Expected pass/fail judgment
4. Verify evals run on mid-tier model
5. Document criteria rationale in .kairon/evals/cases/validator/README.md

**Scope:**
- Review and rebuild validator eval cases
- One agent only (validator)
- Do NOT modify the validator prompt yet
```

#### Phase 2: Build Validator Prompt via EDD

```
Create a GitHub issue to rebuild the validator prompt using Eval-Driven Development.

**Problem:**
The validator prompt needs EDD rebuild. Start minimal, run evals, iterate to 95%+. The existing validator-prompt.md is reference material only.

**Acceptance Criteria:**
1. Create new validator prompt starting minimal
2. Run validator evals, record baseline scores
3. Iterate using Red/Green/Refactor until 95%+ scores
4. Document each iteration in .kairon/iterations/validator/:
   - Minimum 2 documented iterations
   - Each with: baseline, hypothesis, change, results, reasoning
5. Final prompt passes Load-Bearing audit
6. Sync to cmd/kairon/templates/kiro/agents/validator-prompt.md

**Scope:**
- Rebuild validator-prompt.md from scratch
- One agent only (validator)
```

---

### Agent 4: Architect — TDD Rebuild

The architect agent analyzes issues and produces implementation specs.

#### Phase 1: Define Architect Evals

```
Create a GitHub issue to define Stage 3 evaluation criteria for the architect agent.

**Problem:**
The architect agent needs ≥3 distinct evaluation criteria. The architect's job is to read an issue, explore the codebase, and produce a spec that the builder can execute. Evals should test spec quality.

**Acceptance Criteria:**
1. Define ≥3 distinct evaluation criteria for architect. Suggested criteria:
   - Spec completeness: Does the spec contain all info builder needs?
   - Acceptance criteria clarity: Are ACs specific and testable?
   - Codebase awareness: Does spec reference actual files/patterns from the repo?
2. Each criterion must have clear pass/fail threshold
3. Write eval cases in .kairon/evals/cases/architect/ with:
   - Fixture: sample GitHub issue
   - Expected spec structure and content
4. Verify evals run on mid-tier model
5. Document criteria rationale

**Scope:**
- Review and rebuild architect eval cases
- One agent only (architect)
- Do NOT modify the architect prompt yet
```

#### Phase 2: Build Architect Prompt via EDD

```
Create a GitHub issue to rebuild the architect prompt using Eval-Driven Development.

**Problem:**
The architect prompt needs EDD rebuild. Current prompt is loosely structured. Start minimal, run evals, iterate to 95%+.

**Acceptance Criteria:**
1. Create new architect prompt starting minimal
2. Run architect evals, record baseline scores
3. Iterate using Red/Green/Refactor until 95%+ scores
4. Document each iteration in .kairon/iterations/architect/:
   - Minimum 2 documented iterations
5. Final prompt passes Load-Bearing audit
6. Sync to templates

**Scope:**
- Rebuild architect-prompt.md from scratch
- One agent only (architect)
```

---

### Agent 5: Documenter — TDD Rebuild

The documenter agent generates documentation for completed features.

#### Phase 1: Define Documenter Evals

```
Create a GitHub issue to define Stage 3 evaluation criteria for the documenter agent.

**Problem:**
The documenter agent needs ≥3 distinct evaluation criteria. The documenter's job is to create user-facing documentation for completed features. Evals should test documentation quality.

**Acceptance Criteria:**
1. Define ≥3 distinct evaluation criteria for documenter. Suggested criteria:
   - Accuracy: Does documentation correctly describe the implemented feature?
   - Completeness: Does it cover usage, examples, and edge cases?
   - Clarity: Is it readable by the target audience?
2. Each criterion must have clear pass/fail threshold
3. Write eval cases in .kairon/evals/cases/documenter/
4. Verify evals run on mid-tier model
5. Document criteria rationale

**Scope:**
- Review and rebuild documenter eval cases
- One agent only (documenter)
- Do NOT modify the documenter prompt yet
```

#### Phase 2: Build Documenter Prompt via EDD

```
Create a GitHub issue to rebuild the documenter prompt using Eval-Driven Development.

**Problem:**
The documenter prompt needs EDD rebuild. Current prompt is the most ad-hoc of all agents. Start minimal, run evals, iterate to 95%+.

**Acceptance Criteria:**
1. Create new documenter prompt starting minimal
2. Run documenter evals, record baseline scores
3. Iterate using Red/Green/Refactor until 95%+ scores
4. Document each iteration in .kairon/iterations/documenter/:
   - Minimum 2 documented iterations
5. Final prompt passes Load-Bearing audit
6. Sync to templates

**Scope:**
- Rebuild documenter-prompt.md from scratch
- One agent only (documenter)
```

---

### Agent 6: Krew-Lead — TDD Rebuild

The krew-lead orchestrator coordinates the other agents. Rebuilt last because its
evals depend on the worker agents functioning correctly.

#### Phase 1: Define Krew-Lead Evals

```
Create a GitHub issue to define Stage 3 evaluation criteria for the krew-lead agent.

**Problem:**
The krew-lead orchestrator needs ≥3 distinct evaluation criteria. Krew-lead's job is to orchestrate the workflow: parse issues, delegate to agents, handle errors, and produce PRs. Evals should test orchestration quality.

**Acceptance Criteria:**
1. Define ≥3 distinct evaluation criteria for krew-lead. Suggested criteria:
   - Correct delegation: Does krew-lead dispatch to the right agent for each phase?
   - Error handling: Does krew-lead retry on failure and escalate appropriately?
   - Workflow completion: Does a successful run produce a PR?
2. Each criterion must have clear pass/fail threshold
3. Write eval cases in .kairon/evals/cases/krew-lead/
4. Verify evals run on mid-tier model
5. Document criteria rationale

**Scope:**
- Review and rebuild krew-lead eval cases
- One agent only (krew-lead)
- Do NOT modify the krew-lead prompt yet
- Depends on: builder, validator, architect, documenter being Stage 3 compliant
```

#### Phase 2: Build Krew-Lead Prompt via EDD

```
Create a GitHub issue to rebuild the krew-lead prompt using Eval-Driven Development.

**Problem:**
The krew-lead prompt is the largest and most complex. It needs EDD rebuild. Start minimal with just orchestration logic, run evals, iterate to 95%+.

**Acceptance Criteria:**
1. Create new krew-lead prompt starting minimal (just workflow phases)
2. Run krew-lead evals, record baseline scores
3. Iterate using Red/Green/Refactor until 95%+ scores
4. Document each iteration in .kairon/iterations/krew-lead/:
   - Minimum 2 documented iterations
5. Final prompt passes Load-Bearing audit
6. Sync to templates

**Scope:**
- Rebuild krew-lead-prompt.md from scratch
- One agent only (krew-lead)
- This is the final Stage 3 agent
```

---

### Eval Infrastructure Prerequisites

Before starting agent rebuilds, ensure the eval framework meets Stage 3 requirements.

#### Pin Mid-Tier Model

```
Create a GitHub issue to configure the eval runner for mid-tier model enforcement.

**Problem:**
Stage 3 requires evals run on mid-tier models (Sonnet, GPT-4), not frontier models. The current eval runner uses whatever model kiro-cli defaults to, which may be frontier.

**Acceptance Criteria:**
1. Add model configuration to eval runner (internal/eval/runner.go or config file)
2. Default to claude-sonnet-4 for all evals
3. Add validation that rejects frontier models (opus, gpt-4-turbo) for Stage 3 compliance
4. Document the model requirement in .kairon/evals/README.md
5. CI should enforce mid-tier model for eval runs

**Scope:**
- Eval runner configuration only
- Does not modify any agent prompts
```

---

### Stage 3 Completion Checklist

After all agent rebuilds, Stage 3 compliance requires:

- [ ] All 5 agents rebuilt via EDD with 95%+ scores
- [ ] Each agent has ≥3 distinct eval criteria
- [ ] Each agent has ≥2 documented iterations in .kairon/iterations/
- [ ] All evals run on mid-tier model (claude-sonnet-4)
- [ ] All prompts pass Load-Bearing audit (≥90% load-bearing)
- [ ] Templates synced (cmd/kairon/templates/ matches .kiro/agents/)

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
- `/Users/jbrinkman/projects/kairon/.kiro/agents/krew-lead-prompt.md` — Detailed workflow definition
- `/Users/jbrinkman/projects/kairon/internal/plan/executor.go` — Unwired dead code
- `/Users/jbrinkman/projects/kairon/internal/plan/registry.go` — trustedAgents enforcement works

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
- `/Users/jbrinkman/projects/kairon/.kiro/agents/validator-prompt.md` — Conformance checker, not adversarial
- `/Users/jbrinkman/projects/kairon/.kiro/skills/sentinel-protocol/SKILL.md` — Sentinel file protocol
- `/Users/jbrinkman/projects/kairon/internal/plan/validator.go` — Plan validation (deterministic)

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
- `/Users/jbrinkman/projects/kairon/.kiro/agents/krew-lead-prompt.md` — Gates defined in prompt
- `/Users/jbrinkman/projects/kairon/.kairon/evals/cases/krew-lead/error-recovery.yaml` — Tests retry reasoning, not bypass

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
- `/Users/jbrinkman/projects/kairon/.kairon/retries/` — Per-issue retry counts (not success rates)
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

**Files assessed:**
- `/Users/jbrinkman/projects/kairon/internal/logging/` — JSON logger, debug-mode
- `/Users/jbrinkman/projects/kairon/internal/eval/types.go` — CostInfo (eval only)
- `/Users/jbrinkman/projects/kairon/.kairon/specs/issue-239-add-structured-logging-with-live-viewer.md` — Spec exists
- `/Users/jbrinkman/projects/kairon/.kairon/specs/issue-252-logging-json-formatter.md` — Spec exists

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
- `/Users/jbrinkman/projects/kairon/.kiro/agents/krew-lead.json`:
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

**Files assessed:**
- All `/Users/jbrinkman/projects/kairon/.kiro/agents/*.json`

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
- `/Users/jbrinkman/projects/kairon/internal/agent/manager.go` — Separate process spawning
- `/Users/jbrinkman/projects/kairon/.kiro/skills/sentinel-protocol/SKILL.md` — File-based communication

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

---

## Summary

### Stage 3 — Task Certification (TDD Rebuild)

Stage 3 uses a per-agent TDD rebuild approach. All 6 agents must pass, not just the
workflow agents — the planner is part of the Kairon system and meets the same bar.

| Agent | Phase 1: Evals | Phase 2: Prompt | Status |
|-------|---------------|-----------------|--------|
| planner | Define ≥3 criteria | Rebuild via EDD to 95%+ | 🔴 Not started |
| builder | Define ≥3 criteria | Rebuild via EDD to 95%+ | 🔴 Not started |
| validator | Define ≥3 criteria | Rebuild via EDD to 95%+ | 🔴 Not started |
| architect | Define ≥3 criteria | Rebuild via EDD to 95%+ | 🔴 Not started |
| documenter | Define ≥3 criteria | Rebuild via EDD to 95%+ | 🔴 Not started |
| krew-lead | Define ≥3 criteria | Rebuild via EDD to 95%+ | 🔴 Not started |

**Prerequisite:** Pin eval runner to mid-tier model (claude-sonnet-4)

**Stage 3 Readiness: ~20%** — Eval infrastructure exists but no agent has been TDD-rebuilt.

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

### Phase 1 — Stage 3: TDD Agent Rebuilds

Work through agents sequentially. Each agent = 2 tightly-scoped issues.

**Step 0: Prerequisite**
- [ ] Pin mid-tier model in eval runner

**Step 1: Planner Agent (Bootstrap)**
- [ ] Issue: Define planner evals (≥3 criteria)
- [ ] Issue: Rebuild planner prompt via EDD (95%+, 2 iterations)
- *Once complete, use the planner to create all subsequent issues*

**Step 2: Builder Agent**
- [ ] Issue: Define builder evals (≥3 criteria)
- [ ] Issue: Rebuild builder prompt via EDD (95%+, 2 iterations)

**Step 3: Validator Agent**
- [ ] Issue: Define validator evals
- [ ] Issue: Rebuild validator prompt via EDD

**Step 4: Architect Agent**
- [ ] Issue: Define architect evals
- [ ] Issue: Rebuild architect prompt via EDD

**Step 5: Documenter Agent**
- [ ] Issue: Define documenter evals
- [ ] Issue: Rebuild documenter prompt via EDD

**Step 6: Krew-Lead Agent** (depends on Steps 2-5)
- [ ] Issue: Define krew-lead evals
- [ ] Issue: Rebuild krew-lead prompt via EDD

**Stage 3 Complete when:** All 6 agents have 95%+ evals, ≥2 iteration logs each, load-bearing audit passed.

### Phase 2 — Stage 4: Workflow Enforcement

- [ ] Wire Executor into runtime
- [ ] Success rate metrics (`kairon metrics` command)
- [ ] Punch-out bypass testing
- [ ] Audit trail completion

### Phase 3 — Stage 5: Orchestration Isolation

- [ ] Refactor krew-lead to dispatch-only
- [ ] Tighten sub-agent scoping
- [ ] Adversarial context verification test
