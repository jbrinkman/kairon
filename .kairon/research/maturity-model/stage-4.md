# Stage 4 — Workflow Certification Criteria

**Intelligence Operating System (IOS) — Stage 4: Workflow Certification**
_Improving's AI Maturity Framework_

> This document defines the **capabilities Kairon must demonstrate** to satisfy
> Stage 4 of the AI Maturity Framework. It is a definition of required features and
> disciplines, not a packaging or submission guide — certification packaging (ZIP
> submissions, examiner-facing READMEs, etc.) is intentionally out of scope and must
> not appear in this or any later stage definition. Stage 4 builds on the Stage 3
> definition: it certifies multiple Stage-3-qualified agents wired into a reliable,
> guarded, auditable end-to-end workflow.

## Overview

Stage 4 evaluates five areas: how validated agents are **connected** into a workflow,
how the workflow is **guarded**, where it **punches out to humans**, how reliably it
runs **end-to-end**, and how traceable it is through an **audit trail**.

## 1. Workflow Definition

Multiple validated Stage 3 agents connected into a multi-step workflow.

- [ ] Multiple Stage 3 agents wired into an end-to-end workflow
- [ ] Handoffs and branching logic documented
- [ ] Every agent passes its Stage 3 quality bar (95%+ on real work)
- [ ] No agents that still require regular manual correction

## 2. Guardrails

Deterministic validation checks positioned between workflow steps.

- [ ] **Adversarial review agents** that challenge prior step outputs
- [ ] **Hooks and sentinel files** for deterministic runtime checks
- [ ] Guardrails sit **between** steps, not inside them
- [ ] Must be automated — human review between steps is Stage 2, not Stage 4

## 3. Punch-Out Evidence

Documentation of human evacuation points with active bypass testing.

- [ ] Explicit human decision points where the workflow must stop for sign-off
- [ ] **Actively tested** — someone attempted to bypass and was blocked
- [ ] Clear separation of "fail workflow" (automated) vs "punch to human" (manual)
- [ ] Punch-out points that exist on paper but were never tested do NOT qualify

## 4. End-to-End Success Rate

A measured success rate across the full workflow.

- [ ] The **end-to-end number** — not just per-step accuracy
- [ ] Measured across the full workflow, start to finish
- [ ] Trend showing stability or improvement over time
- [ ] If you don't know your end-to-end number, you haven't reached Stage 4

## 5. Audit Trail

Any failure can be traced to its origin step.

- [ ] Structured logs identifying which step produced which output
- [ ] Trace a specific failure to its **exact origin step**
- [ ] **Per-step model and token/cost data** — which model, input tokens, output
  tokens, and cost per step
- [ ] Coverage of all workflow steps, not just some
