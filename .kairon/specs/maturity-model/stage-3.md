# Stage 3 — Task Certification Criteria

**Intelligence Operating System (IOS) — Stage 3: Task Certification**
_Improving's AI Maturity Framework_

> This document defines the **capabilities Kairon must demonstrate** to satisfy
> Stage 3 of the AI Maturity Framework. It is a definition of required features and
> disciplines, not a packaging or submission guide — certification packaging (ZIP
> submissions, examiner-facing READMEs, etc.) is intentionally out of scope and must
> not appear in this or any later stage definition. Later stages build on the Stage 3
> definition.

## Overview

Stage 3 evaluates three areas of discipline: how prompts are **designed**, how they
are **evaluated**, and how they are **evolved**. Each area carries concrete criteria
Kairon must meet.

## 1. Prompt Design

The practitioner's prompts/commands are deliberately engineered.

- [ ] Follows structured prompting (RTCC or similar)
- [ ] Demonstrates deliberate instruction design
- [ ] **Load-Bearing Principle**: ≥90% of instructions must be load-bearing — each
  instruction serves at least one evaluation criterion
- [ ] No dead weight or filler text

## 2. Evaluation

Prompts are backed by substantive, repeatable evaluations (scripts, Promptfoo configs,
or documented test cases).

- [ ] **≥3 distinct evaluation criteria** — not variations of the same check
- [ ] Substantive quality checks, not window dressing
- [ ] Capable of producing **95%+ scores** when the prompt performs well
- [ ] Runs on **mid-tier models or lower** (e.g. Sonnet, GPT-4) — no frontier model
  requirement
- [ ] Clear pass/fail thresholds or scoring rubrics

## 3. Iteration & Evolution

Prompt evolution is documented with evidence of disciplined, measured iteration.

- [ ] **≥2 documented iterations** minimum
- [ ] Each iteration documents: baseline scores, hypothesis, change made, measured
  results, and reasoning
- [ ] Evidence of EDD discipline (Red/Green/Refactor cycle)
- [ ] Chronological progression with clear cause-and-effect
