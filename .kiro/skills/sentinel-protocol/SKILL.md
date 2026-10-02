---
name: sentinel-protocol
description: Core Kiro Krew completion-signalling protocol. Defines the sentinel file path format and write rules that every pipeline agent uses to signal task completion to krew-lead, and that krew-lead uses to detect it. This is engine behavior, not a project convention — do not override it per project.
---

# Sentinel Protocol

Kiro Krew detects agent/task completion via **sentinel files** written to
`.kiro-krew/artifacts/`, using a naming convention (NOT via agent JSON config
fields). This skill is the single source of truth for that path format and the
write rules. Every pipeline agent (architect, builder, validator, documenter)
writes a sentinel; krew-lead reads it. All of them reference this skill so the
convention is defined once.

## Path Format

Plan-based tasks (a task has an `id`):

```
.kiro-krew/artifacts/<agent>-<issue-number>-<task-id>.md
```

- `<agent>` — the writing agent's name (e.g. `builder`, `validator`).
- `<issue-number>` — the GitHub issue number being processed.
- `<task-id>` — the plan task's `id` field (unique per plan).

Including `<task-id>` is required: a single plan can assign several tasks to
the **same** agent (e.g. two `builder` tasks). Without the task id those tasks
would write and read the **same** file, so a later task could overwrite an
earlier task's sentinel, or a completion check could misread a stale sentinel
from a previous task as the current task's success.

### Legacy / non-plan fallback

When there is no plan and therefore no task id (the legacy sequential
workflow), and for whole-issue signals that are not tied to a specific plan
task, use the task-less form:

```
.kiro-krew/artifacts/<agent>-<issue-number>.md
```

A reader that expects a plan task MUST use the `<task-id>` form and MUST NOT
fall back to the task-less form to satisfy a task-scoped check (that is exactly
the stale-read this protocol prevents).

## Write Rules (writing agents)

1. After completing your work, write your sentinel at the path above for your
   agent name, the current issue number, and — for a plan task — the task id.
2. Write to that path only. Do not write sentinels for other agents or tasks.
3. Include a brief summary of what you produced so krew-lead can recover your
   result from the file if your response is otherwise empty.
4. A sentinel's presence with a success summary signals successful completion.

## Read Rules (krew-lead)

1. A plan task is complete only when its task-scoped sentinel
   (`<agent>-<issue-number>-<task-id>.md`) exists with a success status.
2. On an empty subagent response, check for the sentinel before retrying:
   `test -f .kiro-krew/artifacts/<agent>-<issue-number>-<task-id>.md`; if it
   exists, read it to recover the summary and continue; if missing, proceed
   with normal retry escalation.
3. Never accept a task-less sentinel as proof a specific plan task completed.

## Examples (issue 42, plan task `implement-api` handled by builder)

- Plan task:  `.kiro-krew/artifacts/builder-42-implement-api.md`
- Legacy/no-plan builder: `.kiro-krew/artifacts/builder-42.md`
