# Design Spec: Require TDD for AI agents in AGENTS.md

Closes #307

## Problem

AI agents working in this repository have no instruction to write tests first. `AGENTS.md` is the one file every
tool reads (Kairon pipeline, KiroCrew, Copilot, the `pr-comment-resolver` skill), so the rule belongs there.

## Solution Approach

Add one new top-level (`##`) section, **"Test-driven development"**, to `AGENTS.md`. It is a pure documentation
change to a single file.

Decisions:

- **Placement:** append the section at the end of `AGENTS.md`, after "Live documentation and code (edit freely)".
  It is a sibling `##` section, not nested inside "Historical artifacts (do not edit)" or "Live documentation and
  code (edit freely)". Appending avoids reflowing the existing sections or the intro that says "described below".
- **Tone/format:** match the file — short prose, `##` heading, a short numbered list for the ordered steps, and a
  short bullet list for the rules. No tables are needed. Target roughly 25-35 lines.
- **Tool-agnostic wording:** address "agents" generally. It may name the Kairon pipeline, KiroCrew, Copilot and the
  `pr-comment-resolver` skill as examples of tools that follow it. It must NOT mention the `builder` agent (or any
  specific pipeline agent role).
- **Do not make test-first a gate for non-code changes:** docs, prompts, config and skills are exempt.
- **No template sync:** `AGENTS.md` is not in `cmd/kairon/templates/`, so `task sync:check` is unaffected.
  Do not add it to the templates.

### Proposed section content

The builder should use this text (light wording edits are fine if every acceptance criterion below still holds):

```markdown
## Test-driven development

These rules apply to every agent and tool that changes code in this repository (the Kairon pipeline, KiroCrew,
Copilot, the `pr-comment-resolver` skill, and any other). Follow them for new features, bug fixes and refactors.

### New features and bug fixes

1. Write a failing test first that captures the desired behavior (for a bug, the correct behavior the bug violates).
2. Run it and confirm it fails **for the expected reason** — not because of a compile error, typo or unrelated setup problem.
3. Write the minimal code that makes the test pass.
4. Re-run the tests and confirm they pass.

### Refactors

1. Before changing anything, confirm existing tests cover the behavior you are about to change.
2. If coverage is missing, add characterization tests first that pin down the current behavior.
3. Those tests must pass both before and after the refactor.

### Rules

- **Commit the test and the implementation (or refactor) together** in the same commit, so no commit leaves CI red.
  Do not commit a failing test on its own.
- **Keep the change minimal.** Implement the smallest change that makes the new test pass. Put unrelated changes in
  separate issues or PRs.
- **Say what you observed.** In the commit message or PR description, name the test you saw fail first (features and
  bug fixes), or the tests you added or relied on (refactors).
- **Exemption:** non-code changes (docs, prompts, config, skills) do not require a test first.
```

## Relevant Files

| File | Action |
|------|--------|
| `AGENTS.md` | **Modify** — append the new `## Test-driven development` section |
| `.kairon/specs/issue-307-require-tdd-in-agents-md.md` | This spec (architect output) |

Explicitly NOT touched:

- `.kiro/agents/*.json` — the worktree already has uncommitted modifications to these that are **not** part of this
  issue. Do not edit, stage or commit them.
- `.kiro/agents/*-prompt.md` (including builder config/prompts), `.kiro/skills/`
- `cmd/kairon/templates/` and anything under it
- Historical artifacts listed in `AGENTS.md` (`.kairon/specs/` existing files, `.kairon/evals/results/`,
  `.kairon/validation-results.md`, `VALIDATION_SUMMARY.md`, `CHANGELOG.md`)
- `README.md`, `CONTRIBUTING.md`, `docs/`

## Team Orchestration

Two sequential tasks, delivered in one PR:

1. `add-tdd-section` (builder) edits `AGENTS.md`. No code, so no tests are written first (the change is itself
   within the exemption it documents).
2. `validate-tdd-section` (validator) read-only checks each acceptance criterion and that only `AGENTS.md` (plus
   the spec and sentinel artifacts) changed, and that the pre-existing `.kiro/agents/*.json` modifications were not
   staged or altered.

Commit note for krew-lead/builder: stage `AGENTS.md` (and the spec) by explicit path. Do NOT use `git add -A` or
`git add .`, because `.kiro/agents/*.json` carry unrelated uncommitted changes.

## Step-by-Step Task Breakdown

### Task 1: add-tdd-section (builder)

Append the "Test-driven development" section to `AGENTS.md` using the proposed content above.

Acceptance criteria:

- New `## Test-driven development` heading at top level, not nested inside, and positioned outside, the
  "Historical artifacts (do not edit)" and "Live documentation and code (edit freely)" sections (issue AC 1).
- Feature/bug-fix steps: failing test first; run and confirm it fails for the expected reason; minimal code to pass;
  re-run to confirm passing (AC 2).
- Refactor rules: confirm existing coverage; add characterization tests first if missing; tests pass before and
  after (AC 3).
- States test and implementation/refactor are committed together so no commit leaves CI red, and does not tell
  agents to commit a failing test alone (AC 4).
- Asks agents to name in the commit message or PR description the test seen failing first (features/bug fixes) or
  the tests added/relied on (refactors) (AC 5).
- States the smallest change that makes the new test pass, and unrelated changes go to separate issues/PRs (AC 6).
- States non-code changes (docs, prompts, config, skills) are exempt (AC 7).
- Tool-agnostic wording; does not contain the word "builder" (AC 8).
- Only `AGENTS.md` is modified by this task (AC 9).

Dependencies: none.

### Task 2: validate-tdd-section (validator)

Verify Task 1 against every criterion, read-only.

Dependencies: `add-tdd-section`.

## Validation Commands

Run from the project root:

```bash
# Section exists at top level
grep -n '^## Test-driven development' AGENTS.md

# Section is outside the two named sections (list all top-level headings and eyeball order)
grep -n '^## ' AGENTS.md

# AC 8: no mention of builder inside AGENTS.md
! grep -i 'builder' AGENTS.md || true   # inspect output; the new section must not contain "builder"
sed -n '/^## Test-driven development/,$p' AGENTS.md | ! grep -qi 'builder'

# Only AGENTS.md is changed among tracked non-pre-existing files
git diff --name-only -- . ':(exclude).kiro/agents' ':(exclude).kairon'

# Template sync unaffected
task sync:check
```

Note: `AGENTS.md` already contains the word "builder" only if existing text uses it; the check that matters is
restricted to the new section (the `sed` command above). Existing text must not be altered.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "add-tdd-section"
    agent: "builder"
    description: "Append a new top-level '## Test-driven development' section to the end of AGENTS.md, using the proposed content in .kairon/specs/issue-307-require-tdd-in-agents-md.md. Cover: for new features and bug fixes, write a failing test first, run it and confirm it fails for the expected reason, write minimal code to pass, re-run to confirm passing; for refactors, confirm existing tests cover the behavior, add characterization tests first if coverage is missing, and require they pass before and after; commit test and implementation (or refactor) together in the same commit so no commit leaves CI red and never commit a failing test on its own; ask the agent to say in the commit message or PR description which test it observed failing first (features/bug fixes) or which tests it added or relied on (refactors); keep the implementation the smallest change that makes the new test pass and put unrelated changes in separate issues or PRs; exempt non-code changes (docs, prompts, config, skills). Wording must be tool-agnostic (may name the Kairon pipeline, KiroCrew, Copilot and the pr-comment-resolver skill) and must not mention the builder agent. Keep it short and in the same tone/format as the rest of AGENTS.md. Edit ONLY AGENTS.md: do not touch .kiro/agents/*.json (pre-existing unrelated uncommitted changes), builder config/prompts, cmd/kairon/templates/, historical artifacts, or any existing file under .kairon/specs/. If committing, stage by explicit path and never use git add -A or git add ."
    dependencies: []
    acceptance_criteria:
      - "AGENTS.md contains a '## Test-driven development' heading that is a top-level section, outside the 'Historical artifacts (do not edit)' and 'Live documentation and code (edit freely)' sections"
      - "The section requires, for new features and bug fixes: a failing test first, run and confirm it fails for the expected reason, minimal code to pass, and re-run to confirm passing"
      - "The section covers refactors: confirm existing tests cover the behavior, add characterization tests first if coverage is missing, and tests must pass before and after the refactor"
      - "The section says the test and implementation (or refactor) are committed together in the same commit so no commit leaves CI red, and does not instruct committing a failing test on its own"
      - "The section asks the agent to state in the commit message or PR description which test it observed failing first (features/bug fixes) or which tests it added or relied on (refactors)"
      - "The section says to make the smallest change that makes the new test pass and to put unrelated changes in separate issues or PRs"
      - "The section exempts non-code changes (docs, prompts, config, skills) from test-first"
      - "The new section does not contain the word 'builder' and its wording is tool-agnostic"
      - "The only file modified by this task is AGENTS.md; no existing line of AGENTS.md outside the appended section is changed"
    validation_commands:
      - "grep -n '^## Test-driven development' AGENTS.md"
      - "sh -c \"! sed -n '/^## Test-driven development/,$p' AGENTS.md | grep -qi 'builder'\""
      - "sh -c \"git diff --name-only -- . ':(exclude).kiro/agents' ':(exclude).kairon' | grep -vx 'AGENTS.md' | wc -l | grep -qx 0\""
      - "task sync:check"

  - id: "validate-tdd-section"
    agent: "validator"
    description: "Read-only verification that AGENTS.md satisfies all nine issue #307 acceptance criteria: new top-level TDD section outside the two named sections; feature/bug-fix steps; refactor rules; same-commit rule with no standalone failing-test commit; commit message/PR description disclosure; smallest-change rule; non-code exemption; tool-agnostic wording without mentioning the builder agent; and no other files changed (no edits to cmd/kairon/templates/, builder config/prompts, historical artifacts, or the pre-existing .kiro/agents/*.json modifications)."
    dependencies: ["add-tdd-section"]
    acceptance_criteria:
      - "Each of the nine issue acceptance criteria is confirmed against the text of AGENTS.md"
      - "git diff shows only AGENTS.md changed (excluding the architect spec/sentinel artifacts and the pre-existing, untouched .kiro/agents/*.json modifications)"
      - "No file under cmd/kairon/templates/ or .kairon/specs/ (other than the issue-307 spec) is modified"
      - "task sync:check passes"
    validation_commands:
      - "grep -n '^## ' AGENTS.md"
      - "git diff --stat -- AGENTS.md cmd/kairon/templates"
      - "task sync:check"
```
