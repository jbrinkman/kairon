# Stage 5 — Multi-Workflow Certification Criteria

**Intelligence Operating System (IOS) — Stage 5: Multi-Workflow Orchestration**
_Improving's AI Maturity Framework_

> This document defines the **capabilities Kairon must demonstrate** to satisfy
> Stage 5 of the AI Maturity Framework. It is a definition of required features and
> disciplines, not a packaging or submission guide — certification packaging (ZIP
> submissions, examiner-facing READMEs, etc.) is intentionally out of scope and must
> not appear in this or any model definition file. Stage 5 builds on Stage 4: it
> certifies a dispatch-only coordinator orchestrating multiple scoped sub-agents
> across workflows, with the whole solution living under version control.

## Overview

Stage 5 evaluates how a coordinator orchestrates multiple workflows without becoming
a mega-agent. The coordinator may only **dispatch**; each sub-agent is **scoped to one
workflow**; adversarial review runs **in its own context**; the full solution lives in
a **version-controlled repository**; and every **Stage 4 guarantee survives** the
multi-workflow structure. The central failure mode — "The Counterfeit" — is a
coordinator that keeps real-work tools and collapses into a single mega-agent.

## 1. Dispatch-Only Coordinator

A coordinator agent that can call sub-agents and nothing else.

- [ ] No direct tool access, file access, or workflow steps of its own
- [ ] Dispatch surface only (launch / dispatch / invoke verbs)
- [ ] ✗ Anti-pattern: the mega-agent counterfeit — a coordinator holding real-work tools

## 2. Sub-Agents Scoped to One Workflow

Each sub-agent limited to the specific tools, skills, and steps its single workflow
needs.

- [ ] Per-workflow tool manifest, no broader access
- [ ] Stage 3 guarantees maintained (prompt, criteria, measured results)
- [ ] Agents that still need regular manual correction disqualify the workflow

## 3. Adversarial Review in Its Own Context

A dedicated adversarial agent reviews each output in its own context window.

- [ ] Distinct context — never the producing agent's message history
- [ ] Distinct adversarial lens (challenge / catch / detect / block)
- [ ] Context sharing = second opinion, not adversarial review

## 4. Version-Controlled Repository

Full solution source under version control with meaningful history.

- [ ] Coordinator, sub-agents, and workflow definitions all committed
- [ ] Meaningful commit history (a git log export is helpful evidence)

## 5. Stage 4 Continuity

Every Stage 4 guarantee survives the multi-workflow structure.

- [ ] Fully custom harness — no CLI shells for runtime inference
- [ ] Punch-outs honored at the coordinator boundary (no auto-approve)
- [ ] Audit trail with per-step model / token / cost data

## Anti-Pattern — The Counterfeit

A coordinator that keeps its own tools becomes one mega-agent instead of a dispatcher —
a single bad call can now reach every workflow it was supposed to keep separate.

- [ ] ✗ Coordinator with real-work tools = mega-agent
- [ ] ✓ Only dispatch / launch verbs belong on a coordinator
