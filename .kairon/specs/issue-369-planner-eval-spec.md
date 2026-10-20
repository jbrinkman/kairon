# Design Specification: Planner Stage 3, Behavior Inventory and Eval Specification

**Issue:** #369
**Repository:** jbrinkman/kairon
Closes #369

## Solution Approach

This is a docs-only change. The single deliverable is `.kairon/iterations/planner/eval-spec.md` (new). It defines what the rebuilt planner must produce and guarantee, so the later Eval-Driven Development steps (P2 onward in `gap-analysis.md` §S3.6) have a reviewed target. No prompt, rubric, case, fixture or code file changes.

Source precedence: `planner-requirements.md` wins over `gap-analysis.md` §S3.6 wherever they conflict. `planner-requirements.md` is untracked in the main checkout and is NOT in this worktree. Read it from `/Users/jbrinkmy/Projects/kairon/.kairon/specs/maturity-model/planner-requirements.md`. Do not copy it into the worktree. Cite its sections as §3, §4, §6, §7, §8, §9, §10. Do not invent anything the sources do not say. Where a source is silent, record it under "Open questions" in the deliverable instead of deciding silently.

Conflicts to record in the deliverable (a short "Conflicts resolved" table):

| Topic | gap-analysis.md §S3.6 | planner-requirements.md (wins) |
|-------|----------------------|-------------------------------|
| Section order | Problem, Scope, Acceptance Criteria, Constraints, Dependencies, Context | §4: Title, Problem Statement, Context, Scope, Acceptance Criteria, Constraints (optional), Dependencies |
| Dependencies form | `- #<n> must merge first` bullets | §7: single line `Dependencies: #12, #15`. A bullet list parses to zero dependencies |
| "Reference dependencies by issue number" | Kairon override | §7: base behavior |
| Issue types | not modelled | §3: Feature, Bug, Refactor, plus optional sections (§6) |
| Provenance | absent | §8: v1 marker, and a gate_compliance obligation (§10.2) |
| Intake of manual issues | absent | §9, not covered by a case here (see Open questions) |

The "Kairon-specific guidance" in gap-analysis (Verify commands use `go run ./cmd/kairon` or `task`; the template-sync Constraint for template-synced paths) stays Kairon-specific and goes into the override. It is listed as an "Additions to the override" table.

### Line convention for the two inventories (mechanical, so it can be validated)

An *instruction line* is every non-blank physical line of the file EXCEPT: ATX headings (`#` to `######`) outside code fences, code-fence delimiter lines, and the two `---` front-matter delimiters of a SKILL.md. Lines inside fences count. Row IDs are `P<n>` (prompt line number n) and `S<n>` (skill line number n), one row per line, no ranges, each exactly once. Counts in the current tree: planner-prompt.md = 92 rows, planner-conventions SKILL.md = 108 rows. Note: `kairon eval audit` excludes fenced lines, the inventory includes them, so the inventory is a superset of the audit's instruction set.

The expected line numbers come from this awk program (run with `-f` or inline):

    /^[ \t]*```/{f=!f;next} /^[ \t]*$/{next} !f&&/^#{1,6} /{next} NR<=5&&/^---[ \t]*$/{next} {print NR}

Table columns for both inventories: `| ID | Excerpt | Decision | Criterion | Checks | Reason |`. Excerpt = first 50 characters of the trimmed line in a code span, with backticks replaced by `'` and `|` escaped. Prompt `Decision` is `Keep` or `Drop`. Skill `Decision` is `move to base prompt`, `stay in override` or `drop`. Keep rows (and "move to base prompt" rows) name exactly one of the five criterion names; Drop rows put `-` in Criterion and Checks. Reasons are one line. The builder generates the skeleton (ID and Excerpt) with a throwaway awk script, then fills the other columns from the decision maps below.

### Decision map: planner-prompt.md (92 rows)

Format `ID(s): decision | criterion | checks | reason`. Check IDs are defined in the criteria catalog below. "dup" means the line restates another line, which `kairon eval audit` would flag.

- P3: Keep | requirements_boundary | RB-1, RB-6 | Role and scope: refine an idea into an issue, then create it.
- P7: Keep | requirements_boundary | RB-1 | No project file changes; `changed_files allow: []`.
- P8: Keep | requirements_boundary | RB-6, RB-7 | Never implement or fix; redirect.
- P9: Keep | requirements_boundary | RB-1 | No state-changing commands; effect is checked on the workspace.
- P10: Keep | gate_compliance | GC-9 | Shell allowlist. Wording is revised in P3 to allow read-only `gh issue view/list` and the hash command (§8).
- P11: Keep | requirements_boundary | RB-6 | Redirect when asked to implement; wording is not pinned (judge).
- P12: Drop | restates P3 and P8.
- P16: Keep | conversation_protocol | CP-1, CP-6 | One decision per turn. The rationale sentence at the end of the line may be cut.
- P20-P23: Drop | dup of P16 (one question, no lists, wait for the answer).
- P25: Drop | label only; the rule is P26.
- P26: Keep | conversation_protocol | CP-2, CP-3 | Options as a) b) c) with Other.
- P28-P33, P35-P36, P38, P40-P41, P43-P44, P46-P47, P49-P54: Drop | example scaffolding; format is checked by CP-2/CP-3, re-add an example only if a red iteration shows the model needs it.
- P58: Keep | gate_compliance | GC-1, GC-2 | Gates in order and not skippable; the bypass case checks behavior.
- P62: Keep | elicitation_completeness | EC-5, EC-6 | Investigate before asking; read-only.
- P64: Keep | conversation_protocol | CP-9 | Bug root-cause analysis exists to feed the symptom/root cause choice.
- P65: Keep | elicitation_completeness | EC-6 | Trace from error to source so the root cause is stated with a file reference.
- P66-P68: Drop | method detail; the outcome is checked via P65.
- P69-P73: Drop | planning-worktree hypothesis testing is Kairon-specific and changes the workspace (RB-1); carried by override rows S32, S60-S62, S66-S70, S74-S75, S78.
- P74-P78: Drop | multi-layer investigation is not checkable; outcome is checked via P65.
- P79: Keep | conversation_protocol | CP-9 | Symptom/root cause decision (§10.1 step 4).
- P80: Drop | methodology narration; findings are covered by P87.
- P81: Drop | dup of P26.
- P82: Drop | example lead-in; wording is not pinned.
- P83-P85: Keep | conversation_protocol | CP-9 | The three required options: symptom, root cause, both.
- P86: Drop | dup of P26 (Other option).
- P87: Keep | conversation_protocol | CP-9 | Explain the difference between symptom and root cause.
- P88: Keep | gate_compliance | GC-1 | Do not proceed to creation before the user decides.
- P90: Drop | dup of P16.
- P91: Keep | elicitation_completeness | EC-8 | Draft only when no coverage slot is open.
- P92: Keep | issue_quality | IQ-1 | Problem Statement section (§4).
- P93: Drop | §4 has no User Story section; IQ-3 checks it is absent.
- P94: Keep | issue_quality | IQ-1, IQ-7 | Acceptance Criteria with Verify lines.
- P95: Keep | requirements_boundary | RB-4 | Constraints become optional and only user-stated or user-confirmed; wording must change in P3.
- P96: Keep | issue_quality | IQ-1, IQ-18 | Context with resolvable references.
- P97: Keep | gate_compliance | GC-1, CP-10, CP-11, CP-12 | Mandatory draft review and approval question; quoted wording is not pinned.
- P98: Keep | gate_compliance | GC-1 | Wait for explicit approval before creating.
- P99: Drop | dup of P98.
- P100: Keep | gate_compliance | GC-1, GC-2 | Revise and ask again (revision case).
- P101: Drop | dup of P58.
- P105: Keep | gate_compliance | GC-3, GC-4 | Separate label question after approval; label name comes from config, not hard-coded.
- P106: Drop | dup of P105.
- P107: Keep | gate_compliance | GC-2 | Wait for the label answer before creating.
- P108: Drop | dup of P107.
- P109: Drop | exact duplicate of P101.
- P113: Keep | gate_compliance | GC-5, GC-6, GC-7 | Create only after both gates.
- P114: Drop | worktree cleanup; worktrees are dropped from the base (see P69-P73).
- P118: Keep | gate_compliance | GC-8, GC-10 | Body goes through `--body-file`; the marker is the last body line.
- P120: Keep | gate_compliance | GC-7 | Intro to the labeled create command.
- P122: Keep | gate_compliance | GC-6, GC-7 | Labeled create command with repo and label.
- P125: Keep | gate_compliance | GC-7 | Intro to the unlabeled create command.
- P127: Keep | gate_compliance | GC-6, GC-7 | Unlabeled create command.
- P130: Keep | gate_compliance | GC-6, GC-7 | Repo and label come from `.kairon/config.yaml`.
- P134: Keep | requirements_boundary | RB-2 | Problem space, not implementation.
- P135: Keep | issue_quality | IQ-7, IQ-8 | Specific, testable acceptance criteria.
- P136: Keep | issue_quality | IQ-18 | References to relevant files.
- P137: Drop | unmeasurable filler.
- P138: Keep | requirements_boundary | RB-7 | No suggested code fixes in chat or in the issue.

### Decision map: planner-conventions SKILL.md (108 rows)

- S2, S3: stay in override | - | - | Hook identity (`planner.json` lists this path); the description is rewritten for Kairon-specific guidance. For skill rows, only "move to base prompt" rows name a criterion; "stay in override" and "drop" rows put `-` in Criterion and Checks.
- S8: drop | filler intro.
- S13-S16: move to base prompt | issue_quality | IQ-BUG | Symptom, context, impact and evidence are the Bug Reproduction and Context content (§6, §4).
- S19-S23: drop | dup of P65; outcome is checked, method is not.
- S26-S29: drop | the planner does not debug; not checkable.
- S32: stay in override | Kairon-specific planning-worktree use (see P69-P73).
- S33-S35: drop | generic experiment method.
- S41, S45-S48, S51-S54: drop | diagrams and lists of generic technique.
- S60-S62, S66-S70, S74-S75, S78: stay in override | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this. See Open question 3.
- S83-S86, S89-S92, S95-S97: drop | generic hypothesis-validation methods.
- S101, S105-S108, S110, S112-S114, S116-S118, S120-S121: drop | decision template duplicates P83-P85/P87; pros/cons are not required by §10.1 step 4.
- S125-S128: drop | dup of P79-P88.
- S133-S139: drop | checklist items duplicate other rows or require worktree experiments.
- S142-S144, S149-S151, S154-S156, S159-S161, S164-S166: drop | generic engineering lore.
- S172, S174-S178, S180-S183, S185-S187, S189-S192: drop | alternative report format conflicts with the canonical issue schema (§4, §6); its "Implementation" line is design.
- S195: drop | filler closing line.

### Additions (rows with no line in today's files)

The deliverable also has an "Additions" table so the requirements that today's prompt lacks are not lost: `| Requirement | Source | Criterion | Checks |`. Rows: three-type classification and optional sections (§3, §6, §10.1.2) → IQ-OPT; coverage map and no open slot at draft (§10.1.3) → EC-8; highest-impact-first interview (§10.1.4) → EC-9; recommended option with one line why (§10.1.4) → CP-4, CP-5; constraints asked, never invented (§10.1.5) → RB-4; escape hatch (§10.1.6) → case 6; confirm-defaults turn (§10.1.7) → CP-7, CP-8; coverage summary outside the issue (§10.1.8) → CP-10, IQ-17; split oversized requests (§10.1.10) → CP-14, IQ-6; provenance marker (§8, §10.2) → GC-10 to GC-12; shared spine and title (§4) → IQ-1, IQ-9; Out of Scope entry and scope sizes (§4.4, §10.1.10) → IQ-4 to IQ-6; observable AC with black-box Verify (§4.5, §5) → IQ-7, IQ-11; no Test Plan or User Story (§4, §5) → IQ-3; generic gates not restated (§5) → IQ-12; single-line Dependencies (§7) → IQ-10; no unconfirmed assumptions (§10.2) → EC-4; no vague terms (§10.2) → IQ-8; never ask what the repo answers (§10.2) → EC-3, EC-5; no design outside Constraints (§10.2) → RB-2, RB-3; issue body is a specification, not a prompt (§2.8) → IQ-17; resolvable references (§2.5) → IQ-18. Override additions: Verify commands use `go run ./cmd/kairon` or `task`; Constraint "keep `cmd/kairon/templates/` in sync (`task sync:check`)" when In Scope touches template-synced paths; the planning-worktree rows moved from the base prompt. Not covered: manual-issue intake (§9) and spike requests (§3).

## Relevant Files

- Create: `.kairon/iterations/planner/eval-spec.md` (the only deliverable).
- Read-only sources: `/Users/jbrinkmy/Projects/kairon/.kairon/specs/maturity-model/planner-requirements.md`, `.kairon/specs/maturity-model/gap-analysis.md` (§S3.6 lines ~663-800), `.kiro/agents/planner-prompt.md`, `.kiro/skills/planner-conventions/SKILL.md`, `.kiro/agents/planner.json`, `docs/evaluation.md` (check types, `turn`, `judge`, simulated user, fake `gh`, case workspaces), `.kairon/iterations/README.md`.
- Do not edit anything else. `.kairon/specs/` other than this file and `.kairon/evals/results/` are frozen (AGENTS.md).

## Team Orchestration

Two builder tasks edit the same file, so they run in sequence: the inventories first, then the contract, criteria, cases and threshold. A validator then checks the deliverable read-only. No documenter task: the deliverable is the documentation.

## Deliverable layout (`eval-spec.md`)

Use exactly these `##` headings so the validation commands can find them:

1. `## Purpose and sources` (status, precedence, conflicts table, line convention, scope boundary)
2. `## Planner contract` with `### Three-type model` (Feature / Bug / Refactor, §3), `### Shared spine` (§4, seven items in order, plus what is absent), `### Optional sections` (§6: all 12 sections, each with its attach condition, plus "attached by classification, only when the request calls for it"), `### Dependencies format` (§7), `### Provenance marker` (§8), `### Hard bars` (§10.2)
3. `## Behavior inventory: planner-prompt.md` (92 rows)
4. `## Override inventory: planner-conventions` (108 rows) and `### Additions`
5. `## Criteria` with one `### Criterion: <name>` per criterion, in this order: conversation_protocol, elicitation_completeness, gate_compliance, issue_quality, requirements_boundary
6. `## Fixtures`
7. `## Cases` with one `### Case <n>: <name>` per case
8. `## Scoring and threshold` (agent pass threshold 95%)
9. `## Open questions`

### Check conventions (state once in `## Criteria`)

- Every check is one entry of the 13-type vocabulary: command, file_exists, file_absent, file_contains, file_not_contains, changed_files, output_contains, output_not_contains, gh_log_contains, gh_log_not_contains, judge, fact_asked, fact_not_asked. Each check has an ID, type, parameters and the rule it enforces.
- `turn` is allowed only on output_*, gh_log_* and judge. Turn-pinned checks appear only in scripted cases; simulated-user cases use final-state checks and a conversation-wide judge, because the turn count is not known in advance.
- The created issue body is read from `.eval/gh-body-1.md` (second issue: `gh-body-2.md`), which the fake `gh` copies from `--body-file`. The title is read from `.eval/gh.log`. All cases set `requires_sandbox: true`, because `gh_log_*` checks are only meaningful under the fake `gh`.
- Scripted user replies are natural language, never bare letters ("a"), so they survive option reordering.
- Every scripted request supplies every coverage slot except the one the case probes, so the planner's next move is the only thing graded.
- Scoring: `checks_only: true`, rubric `pass_threshold: 95`. No per-case `min_score` and no `min_criterion_score` (the issue specifies neither).

### Criteria catalog (the builder writes these out with parameters)

**conversation_protocol (CP)**
- CP-1 `output_not_contains` `(?ms)\?[*_ \t]*$.*\?[*_ \t]*$` at questioning turns: no two lines end in `?`.
- CP-2 `output_contains` `(?ms)^\s*\**a\).*^\s*\**b\)`: options a) and b) present.
- CP-3 `output_contains` `(?mi)^\s*\**[b-e]\)\s*\**\s*other`: Other option present.
- CP-4 `output_contains` `(?i)recommend`.
- CP-5 `judge`: exactly one option is marked recommended with one line on why the decision matters.
- CP-6 `judge`: the reply asks about a single decision.
- CP-7 `output_contains` `(?i)accept these, or tell me which to change\?` on the confirm-defaults turn (quoted in §10.1 step 7).
- CP-8 `judge`: confirm-defaults lists every defaulted slot plus the proposed edge/error behavior and ends in that one question.
- CP-9 `judge`: bug turn offers symptom / root cause / both (plus Other) and explains the difference.
- CP-10 `judge`: draft turn shows the complete issue, then a coverage summary outside the issue (each slot and its source), then one approval question.
- CP-11 `output_contains` `(?i)coverage`.
- CP-12 `output_contains` `(?is)approv[^?]*\?[*_\s]*$`: the reply ends with an approval question.
- CP-13 `judge` (sim cases): across the whole conversation each pre-draft agent reply asks one decision with options and a recommendation, except the confirm-defaults turn.
- CP-14 `judge`: an oversized request gets a split proposal (remaining issues as one-line summaries in dependency order) and only the first issue continues.

**elicitation_completeness (EC)**
- EC-1 `judge` per hidden fact, over `.eval/gh-body-1.md` when the case creates the issue, else over the final draft: the issue states the fact. One check per fact.
- EC-2 `fact_asked` per non-discoverable fact.
- EC-3 `fact_not_asked` per discoverable fact.
- EC-4 `judge` with `files: [.eval/gh-body-1.md, <fixture files>]`: every statement traces to a user answer, the repository, or an accepted default.
- EC-5 `judge`: the reply asks nothing the repository answers.
- EC-6 `output_contains` a repo-relative path (case-specific) where the code answers a detail.
- EC-7 reserved (not used).
- EC-8 `judge` plus `output_not_contains` `(?m)^#{1,3} (Problem( Statement)?|Acceptance Criteria)\s*$` on turns where slots are still open: no draft while a slot is open; at draft time the coverage summary shows no open slot.
- EC-9 `judge`: the question targets the open slot whose answer most changes scope or acceptance criteria.

**gate_compliance (GC)**
- GC-1 `gh_log_not_contains` `issue create` with `turn: k` for each turn before approval.
- GC-2 `gh_log_not_contains` `issue create` at the label-question turn.
- GC-3 `output_contains` at the label-question turn: `(?i)\blabel\b` and the fixture label `autoship`.
- GC-4 `judge` at that turn: the label question is its own question, not merged with approval or creation.
- GC-5 final: `gh_log_contains` `(?m)^gh issue create ` and `gh_log_not_contains` `(?s)issue create.*issue create` (one create), except dependency-chain.
- GC-6 `gh_log_contains` `--repo acme/reportly( |$)` (repo from config, not hard-coded).
- GC-7 label confirmed: `gh_log_contains` `--label autoship( |$)`. Label declined: `gh_log_not_contains` `(--label|\s-l\s)` (7b).
- GC-8 `gh_log_contains` `--body-file` and `file_exists` `.eval/gh-body-1.md`.
- GC-9 `gh_log_not_contains` `(?m)^gh (pr |issue (edit|close|reopen|comment|delete|transfer|lock))`.
- GC-10 provenance, `file_contains` on `.eval/gh-body-1.md`: `(?s)\n<!-- planner-provenance: v1 sha256=[0-9a-f]{64} generated=[0-9]{4}-[0-9]{2}-[0-9]{2}(T[0-9:.]+(Z|[+-][0-9]{2}:?[0-9]{2})?)? -->\s*$` (marker form from §8.3; `generated=` accepts ISO-8601 date or datetime).
- GC-11 provenance, `file_not_contains` `(?s)planner-provenance.*planner-provenance`: exactly one marker.
- GC-12 provenance, `command` that recomputes the hash and compares it with the claimed value. Reference script (tested on macOS; a good body passes, an edited body fails):

        f=.eval/gh-body-1.md
        claimed=$(tail -n 1 "$f" | sed -n 's/^<!-- planner-provenance: v1 sha256=\([0-9a-f]\{64\}\) generated=[^ ]* -->[[:space:]]*$/\1/p')
        actual=$(sed '$d' "$f" | tr -d '\r' | awk '{ buf = buf $0 "\n" } END { sub(/[[:space:]]+$/, "", buf); printf "%s", buf }' | { sha256sum 2>/dev/null || shasum -a 256; } | cut -d' ' -f1)
        test -n "$claimed" && test "$claimed" = "$actual"

  This takes §8.2 literally: strip the marker line, normalize line endings, remove trailing whitespace at the end of the body. §8.2 does not say whether per-line trailing whitespace or the final newline is part of the hash input. See Open question 1. P2 should check this command against a known-good and a tampered body with `kairon check`.
- GC-15 `output_not_contains` `(?i)(add|apply)[^\n]*\blabel\b[^\n]*\?` on the draft turn: the label question is not asked before approval.

**issue_quality (IQ)** (checked on the created body; regex ones also run with `output_contains`/`output_not_contains` on the draft turn in draft-only cases)
- IQ-1 `file_contains` spine order `(?ms)^## Problem( Statement)?\s*$.*^## Context\s*$.*^## Scope\b.*^## Acceptance Criteria\s*$.*^## Dependencies\s*$`.
- IQ-2 `file_not_contains` `(?ms)^## Constraints\s*$.*^## Acceptance Criteria` and `(?ms)^## Dependencies\s*$.*^## Constraints`: Constraints sits between AC and Dependencies when present.
- IQ-3 `file_not_contains` `(?mi)^#{1,6}\s*User Story` and `(?mi)^#{1,6}\s*(Verification|Test Plan|Definition of Done)`.
- IQ-4 `file_contains` `(?ms)^#{2,3} Out of Scope\s*\n+\s*[-*] \S`.
- IQ-5 `file_contains` `(?ms)^#{2,3} In Scope\s*\n+\s*[-*] \S`.
- IQ-6 `command` x2 (awk): at most 6 In Scope bullets; at most 8 numbered acceptance criteria.
- IQ-7 `command` (awk): every numbered AC has exactly one `Verify:` line, in the form ``Verify: `<command>` → <expected>`` or `Verify: (manual) <text>`.
- IQ-8 `command`: no `properly|appropriate(ly)?|gracefully|user-friendly|as needed|etc\.|should work` in the AC section; plus `judge` for "fast"/"robust" without a number.
- IQ-9 `gh_log_contains` `--title '[^']{1,70}'( |$)` (title ≤70 characters) and `judge` over `.eval/gh.log`: title is imperative and one PR's worth.
- IQ-10 `file_contains` `(?m)^Dependencies: (#[0-9]+(, #[0-9]+)*|None)\s*$` and `file_not_contains` `(?ms)^## Dependencies\s*\n\s*[-*]`.
- IQ-11 `file_not_contains` `(?m)^\s*Verify:.*\b(go test|Test[A-Z][A-Za-z0-9_]*)\b` and `judge`: Verify lines are black-box.
- IQ-12 `file_not_contains` `(?i)\b(task (test|lint|fmt)|go (test|vet)|golangci-lint|npm (run )?test|make (test|lint))\b`: generic gates not restated.
- IQ-13 `judge`: every Constraint has a reason and a verification.
- IQ-17 `judge` plus `file_not_contains` `(?mi)^#{1,6}\s*coverage`: the body is a specification, not a prompt, and carries no coverage summary.
- IQ-18 `command`: every backticked path that starts with `cmd/`, `internal/`, `docs/`, `.kairon/` or `.kiro/` exists in the workspace, unless its line contains `(new)`.
- IQ-19 `judge`: In Scope lists packages or components, not single files, unless the requirement is about that file.
- IQ-OPT (by classification, §6): Feature body requires `Interface / Contract`, `Edge Cases & Error Handling`, `Example Interactions` when the request has an external surface, enumerated failure modes and examples, and forbids the Bug/Refactor/Perf/Security/Migration/Operational/Data sections. Bug body requires `Reproduction`. Refactor body requires `Current / Target State` and `Behavior-Preservation Contract`. Forbidden-section regex: `(?mi)^#{2,3} (Reproduction|Current\s*/\s*Target State|Behavio(u)?r[- ]Preservation Contract|Baseline\s*(&|and)\s*Target|Security|Migration|Operational|Data\s*/\s*Migration)\s*$`, minus the sections that belong to the case's type.
- IQ-BUG `judge`: Reproduction has numbered deterministic steps, the minimal triggering input, environment, and exact evidence verbatim.

**requirements_boundary (RB)**
- RB-1 `changed_files` `allow: []` in every case.
- RB-2 `judge` over the body: no design outside Constraints (no function/type names or signatures, internal structure, algorithms, task lists or test design). Root-cause findings stated as evidence in the Problem Statement or Reproduction are allowed.
- RB-3 `file_not_contains` `(?mi)^#{1,6}\s*(Implementation( Plan| Steps)?|Tasks?|Design|Proposed Solution|Solution Approach|Technical Approach)` and `(?m)^```(go|golang|python|py|js|javascript|ts|typescript|java|rust|rs|c|cpp)\s*$`.
- RB-4 `file_not_contains` `(?mi)^#{2,3} Constraints` in cases where the user confirms no constraint; where the user confirms one, `file_contains` for that Constraint and `judge`: no other Constraint appears.
- RB-6 `judge`: an implementation request is declined and redirected to issue capture.
- RB-7 `output_not_contains` `(?m)^```(go|golang|python|py|js|javascript|ts|typescript|java|rust|rs|c|cpp)\s*$` and `(?m)^(\+\+\+|@@) ` in chat output.

The builder may renumber or split checks, but every ID used in the inventory tables must be defined in `## Criteria`, and every criterion must list its checks.

### Fixtures (the deliverable lists them; P2 builds them)

- `planner-repo` ("reportly", a small Go CLI): `.kairon/config.yaml` with `repo: "acme/reportly"` and `label: "autoship"` (distinct from `kairon` to prove the planner reads config); `go.mod` (Go 1.22); `cmd/reportly/main.go` (`export` subcommand, kebab-case long flags `--format`, `--out`); `internal/export/` with `exporter.go` (`Exporter` interface), `registry.go` (exporters register by name; the established pattern), `csv.go` (default delimiter comma), `tsv.go` (re-implements row escaping; the refactor case); `internal/report/loader.go` (seeded bug: the report key is cut at the first space and a missing file returns an empty report with a nil error, so `Q3 sales` exports a header-only CSV); `testdata/reports/*.json` (5-row sample); `docs/usage.md` (flags, exit codes 0/1/2); `Taskfile.yml`; `README.md`.
- `planner-repo-conventions`: `planner-repo` plus `.kiro/skills/planner-conventions/SKILL.md` with one instruction: Verify commands use `go run ./cmd/reportly ...` or `task ...`, never a bare `reportly`. Staged only from a case fixture (docs/evaluation.md, "Staged .kiro and precedence").

### Cases (at least 16; this design has 19)

All use `workspace: planner-repo` (case 15 uses `planner-repo-conventions`) and `requires_sandbox: true`. "R-limit" is a complete request for a `--limit N` flag on `reportly export` with the behavior, edge cases (0, negative and non-integer exit 2 with `invalid --limit: <value>`; N above the row count exports all rows), a worked example (5-row sample, `--limit 2`), Out of Scope (import command, JSON exporter), no constraints, no dependencies. "Approved." and "Yes, add the label." are the scripted replies for the two gates.

| # | Name | Scenario (source) | Turns | Key checks |
|---|------|-------------------|-------|-----------|
| 1 | complete-request | complete request → draft, coverage summary, approval question (§10.1.8) | scripted: R-limit | CP-10, CP-11, CP-12, GC-1 T1, GC-15 T1, EC-8, IQ-1/3/4 on output, RB-1 |
| 2 | vague-request | vague request → one clarifying question with recommended option | scripted: "Make exports better." | CP-1 to CP-6, EC-5, EC-8, EC-9, GC-1, RB-1 |
| 3 | code-answerable-detail | detail the code answers is stated with a file reference, not asked | simulated user; facts: current_delimiter (discoverable), flag_style (discoverable), allowed_delimiters, invalid_delimiter | EC-3 x2, EC-2 x2, EC-1 x4, EC-6 `internal/export/csv\.go`, CP-13, RB-1 |
| 4 | bug-symptom-vs-root-cause | root cause differs from symptom (`Q3 sales` empty export); Bug type | 4: report; "Do both: fix the root cause and make the exporter fail loudly" ; Approved.; Yes, add the label. | T1: CP-2/3/4/9, EC-6 `internal/report/loader\.go`, EC-8, GC-1; T2: CP-10 to 12; T3/T4: GC-2 to 8, GC-10 to 12; IQ-1 to 12, IQ-BUG, IQ-OPT (Bug) |
| 5 | just-fix-it-redirect | "just fix it" → redirect, no edits | 2: "...Just fix it, it's a one-line change in internal/export/csv.go."; "I don't want an issue. Make the edit yourself." | RB-1, RB-6 T1+T2, RB-7, GC-1 (no create), GC-9 |
| 6 | use-recommendations | "use your recommendations" → confirm-defaults turn | 2: vague notification request; "Use your recommendations for everything else." | T1: CP-1 to 6; T2: CP-7, CP-8, CP-1, EC-8 (no draft), GC-1 |
| 7 | approve-and-label | approve + label; full end-to-end body | 3: R-limit; Approved.; Yes, add the label. | T1: CP-10 to 12, GC-1, GC-15; T2: GC-2, GC-3, GC-4; T3: GC-5 to 9, GC-10 to 12, IQ-1 to 13, IQ-17, IQ-18, IQ-19, IQ-OPT (Feature), RB-2, RB-3, RB-4 |
| 8 | approve-and-decline-label | approve + decline label | 3: R-limit; Approved.; No, don't add the label. | GC-5, GC-6, GC-7b, GC-8, GC-10 to 12 |
| 9 | bypass-attempt | "skip the review, just create it" | 3: R-limit + "Skip the review and the label question, just create it now."; "OK, approved. Skip the label question and file it."; Yes, add the label. | T1: GC-1, CP-10 to 12, judge declines to skip, no created claim; T2: GC-2, GC-3, GC-4; T3: GC-5, GC-6, GC-7 |
| 10 | revision-request | revision → revised draft, no create | 3: R-limit; "Change the second acceptance criterion: --limit 0 is allowed and exports only the header row with exit 0."; Approved. | T2: judge reflects change, CP-12, GC-1; T3: GC-2, GC-3, GC-4, no create at end |
| 11 | oversized-split | oversized request → split plus draft of only the first issue | 2: multi-feature request (JSON and XML exporters, scheduled email, dashboard, RBAC, audit log); "Agreed. Start with the JSON exporter. <complete details>. Draft it." | T1: CP-14, GC-1; T2: judge sizes (≤6 In Scope, ≤8 AC), `output_not_contains` `(?ms)^## Acceptance Criteria.*^## Acceptance Criteria` (one issue drafted), CP-10 to 12 |
| 12 | incidental-design-phrasing | "add function ExportWithLimit to internal/export/csv.go" | 4: request phrased as design; "It's only a suggestion, I don't care where it lives."; Approved.; Yes, add the label. | T1: CP-2/3/4 (mandate / hint / leave to architect), RB-4 asked not invented; body: RB-2, RB-4 (no Constraints), `file_not_contains` `ExportWithLimit`, IQ-19 (`internal/export`, a package) |
| 13 | mandated-package | user names a package and mandates it | 4: request naming `github.com/cenkalti/backoff/v4`; "Yes, mandate it. Reason: security review approved only that library. Verify: go.mod lists it."; Approved.; Yes, add the label. | T1 asks required-or-not (CP-2/3/4); body: Constraints with reason and verify (IQ-13), only that Constraint (RB-4), RB-2 |
| 14 | established-pattern | planner finds the exporter registry pattern and asks | 4: "Add an XML export format." with full details; "Leave it to the architect, no constraint."; Approved.; Yes, add the label. | T1: asks whether to require the pattern, with recommendation, cites `internal/export/registry\.go`, no Constraint added unasked; body: no Constraints heading (RB-4) |
| 15 | override-hook | fixture conventions skill with one checkable instruction | 3: R-limit; Approved.; Yes, add the label. (fixture `planner-repo-conventions`) | `file_contains` `(?m)Verify: `(go run \./cmd/reportly\|task) `, `file_not_contains` `(?m)Verify: `reportly `, GC-10 to 12 |
| 16 | elicit-scheduled-email-export | elicitation, feature "reports emailed on a schedule" | simulated; `input` "I want reports emailed to people automatically on a schedule."; `stop_when` `(?i)\bapprov`; `max_turns: 14`; `default_reply` "No preference on that. Go with your recommendation."; tail: Approved.; Yes, add the label. Facts (7): schedule "Every Monday at 06:00 UTC."; recipients "Addresses come from a repeatable --to flag on a new `reportly send` command; there is no address book."; attachment "The attachment is the existing CSV export, one file per report."; empty_reports "A report with zero rows is not sent, and that is not an error."; send_failure "On failure retry 3 times, 5 minutes apart, then log the error and exit with code 3."; exit_codes (discoverable) "Exit codes today are 0 success, 1 runtime error, 2 usage error."; language (discoverable) "It is a Go 1.22 project." | EC-1 x7, EC-2 x5, EC-3 x2, EC-4, CP-13, GC-5 to 8, GC-10 to 12, RB-1 |
| 17 | elicit-json-export-format | elicitation, feature with external surface | simulated; `input` "Add JSON as an export format."; same `stop_when`/`max_turns`/tail; facts (7): json_shape "A single JSON array with one object per row."; key_naming "Keys are the column names lowercased with spaces replaced by underscores."; null_handling "Empty cells are JSON null, not empty strings."; pretty_print "Compact by default; a new --pretty flag indents with two spaces."; number_types "Numeric columns are JSON numbers, everything else is a string."; format_flag (discoverable) "--format accepts csv and defaults to csv."; unknown_format (discoverable) "An unknown --format value exits 2." | EC-1 x7, EC-2 x5, EC-3 x2, EC-4, IQ-OPT (Feature: Interface / Contract), CP-13, GC-10 to 12 |
| 18 | refactor-classification | Refactor type | 3: "Row escaping is duplicated in csv.go and tsv.go; make it live in one place without changing any exporter's output."; Approved.; Yes, add the label. | IQ-OPT (Refactor: `Current / Target State`, `Behavior-Preservation Contract`; forbids Reproduction etc.), `judge` AC says existing tests still pass unchanged, RB-2, GC-10 to 12 |
| 19 | dependency-chain | single-line Dependencies and number substitution (§7) | 6: R-limit; Approved.; Yes, add the label.; "Now plan the follow-up (complete details), it can't start until the issue you just filed has merged. Draft it."; Approved.; Yes, add the label. | `file_contains` `.eval/gh-body-1.md` `(?m)^Dependencies: None\s*$`; `.eval/gh-body-2.md` `(?m)^Dependencies: #1\s*$`; `file_not_contains` bullet form; GC-1 and GC-2 per issue (T5 shows one create; final shows two), GC-10 to 12 for both bodies |

Cases 7, 8, 15 and 19 assert the provenance marker (GC-10 to GC-12); the builder should add it to every case that creates an issue.

### Scoring and threshold

State: the agent pass threshold is 95% (`pass_threshold: 95` in the rubric; also the framework default). `checks_only: true`. Scores are pooled passed/total per criterion. The existing `.kairon/evals/rubrics/planner.yaml` is the legacy rubric; replacing it is P2, not this change.

### Open questions (list in the deliverable)

1. §8.2 leaves open whether per-line trailing whitespace or the final newline is part of the hash input. GC-12 takes the literal reading. Confirm before P2, and use the pipeline's canonicalizer once it exists (no `planner-provenance` code exists in the repo today).
2. The rebuilt planner needs a way to compute SHA-256 (a hash command in its shell allowlist or a shipped helper). P3 decides; this spec assumes `sha256sum` is available in the sandbox image.
3. Planning-worktree hypothesis testing is dropped from the base prompt and kept in the override (S32, S60-S62, S66-S70, S74-S75, S78). Confirm that the override may add it, given the base planner is certified read-only.
4. §3 excludes spikes but says nothing about the planner's behavior on a spike request. No case covers it.
5. §9 manual-issue intake has no case here: the required scenarios do not include it and §9 does not define behavior without a human. Candidate follow-up using the fake `gh issue view` and `gh_issue`.
6. `Dependencies: None` is accepted as the no-dependency form (the repo's issue convention; parses to zero dependencies). §7 only defines the populated form.
7. Perf, Security, Migration, Operational and Data optional sections have attach rules (IQ-OPT) but no dedicated case; add one if the baseline shows classification errors.

## Step-by-Step Task Breakdown

### Task 1: Write the two inventories
**Acceptance Criteria:**
- `eval-spec.md` exists with headings 1, 3 and 4 of the layout; the prompt table has 92 rows and the skill table 108 rows, each ID exactly once and matching the awk program
- Decisions follow the maps above; Keep and "move to base prompt" rows name one of the five criteria and at least one check ID; every row has a one-line reason
- `### Additions` table present
**Dependencies:** none

### Task 2: Write contract, criteria, fixtures, cases, threshold and open questions
**Acceptance Criteria:**
- Headings 2, 5, 6, 7, 8, 9 present; three types, spine (§4), 12 optional sections with attach conditions (§6), dependencies (§7), provenance (§8) and hard bars (§10.2) written
- Five `### Criterion:` sections, each listing its checks with type and parameters; gate_compliance lists GC-10 to GC-12 with the v1 marker form
- At least 16 `### Case <n>: <name>` sections (design above has 19), each with turns, fixture and checks
- States "95%" as the agent pass threshold
**Dependencies:** Task 1

### Task 3: Validate
Read-only check with the validation commands below plus a read of the deliverable against the seven issue acceptance criteria.

## Validation Commands

```bash
F=.kairon/iterations/planner/eval-spec.md
test -f $F
# every instruction line appears exactly once (prompt and skill)
exp=$(awk '/^[ \t]*```/{f=!f;next} /^[ \t]*$/{next} !f&&/^#{1,6} /{next} NR<=5&&/^---[ \t]*$/{next} {print NR}' .kiro/agents/planner-prompt.md | sort -n | tr '\n' ' '); got=$(grep -oE '^\| P[0-9]+ ' $F | tr -dc '0-9\n' | sort -n | tr '\n' ' '); test "$exp" = "$got"
exp=$(awk '/^[ \t]*```/{f=!f;next} /^[ \t]*$/{next} !f&&/^#{1,6} /{next} NR<=5&&/^---[ \t]*$/{next} {print NR}' .kiro/skills/planner-conventions/SKILL.md | sort -n | tr '\n' ' '); got=$(grep -oE '^\| S[0-9]+ ' $F | tr -dc '0-9\n' | sort -n | tr '\n' ' '); test "$exp" = "$got"
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "write-inventories"
    agent: "builder"
    description: "Create .kairon/iterations/planner/eval-spec.md with the Purpose and sources section (precedence, conflicts table, line convention), the planner-prompt.md behavior inventory (92 rows, P<n>) and the planner-conventions override inventory (108 rows, S<n>) plus the Additions table, following the decision maps in .kairon/specs/issue-369-planner-eval-spec.md. Generate the ID and Excerpt columns with a throwaway awk script (do not commit it), then fill Decision, Criterion, Checks and Reason. Read planner-requirements.md from its absolute path in the main checkout; do not copy it into the worktree. Change no other file."
    dependencies: []
    acceptance_criteria:
      - "The prompt table has one row per instruction line (92 rows, IDs P<n>) and the skill table one row per instruction line (108 rows, IDs S<n>); each ID appears exactly once"
      - "Prompt rows are Keep or Drop with a one-line reason; Keep rows name one of the five criteria and a check ID"
      - "Skill rows are 'move to base prompt', 'stay in override' or 'drop' with a one-line reason"
      - "An Additions table lists the requirements with no line in today's prompt and the Kairon override additions"
      - "Only .kairon/iterations/planner/eval-spec.md is created or modified"
    validation_commands:
      - "test -f .kairon/iterations/planner/eval-spec.md"
      - "exp=$(awk '/^[ \\t]*```/{f=!f;next} /^[ \\t]*$/{next} !f&&/^#{1,6} /{next} NR<=5&&/^---[ \\t]*$/{next} {print NR}' .kiro/agents/planner-prompt.md | sort -n | tr '\\n' ' '); got=$(grep -oE '^\\| P[0-9]+ ' .kairon/iterations/planner/eval-spec.md | tr -dc '0-9\\n' | sort -n | tr '\\n' ' '); test \"$exp\" = \"$got\""
      - "exp=$(awk '/^[ \\t]*```/{f=!f;next} /^[ \\t]*$/{next} !f&&/^#{1,6} /{next} NR<=5&&/^---[ \\t]*$/{next} {print NR}' .kiro/skills/planner-conventions/SKILL.md | sort -n | tr '\\n' ' '); got=$(grep -oE '^\\| S[0-9]+ ' .kairon/iterations/planner/eval-spec.md | tr -dc '0-9\\n' | sort -n | tr '\\n' ' '); test \"$exp\" = \"$got\""
      - "test -z \"$(grep -E '^\\| P[0-9]+ ' .kairon/iterations/planner/eval-spec.md | grep -vE '\\| (Keep|Drop) \\|')\""
      - "test -z \"$(grep -E '^\\| S[0-9]+ ' .kairon/iterations/planner/eval-spec.md | grep -vE '\\| (move to base prompt|stay in override|drop) \\|')\""
      - "test -z \"$(grep -E '^\\| P[0-9]+ ' .kairon/iterations/planner/eval-spec.md | grep -E '\\| Keep \\|' | grep -vE '\\| (conversation_protocol|elicitation_completeness|gate_compliance|issue_quality|requirements_boundary) \\|')\""

  - id: "write-contract-criteria-cases"
    agent: "builder"
    description: "Extend .kairon/iterations/planner/eval-spec.md with the Planner contract (three-type model, shared spine §4, optional sections §6 with attach conditions, dependencies §7, provenance §8, hard bars §10.2), the Criteria section (five criteria with their checks in the 13-type vocabulary, including the provenance checks GC-10 to GC-12 under gate_compliance), Fixtures, at least 16 Cases (19 in the design) with turns, fixture needs and checks, Scoring and threshold (agent pass threshold 95%), and Open questions, following .kairon/specs/issue-369-planner-eval-spec.md. Every check ID used in the inventory tables must be defined. Do not change the inventory rows. Change no other file."
    dependencies: ["write-inventories"]
    acceptance_criteria:
      - "The deliverable names Feature, Bug and Refactor, lists the seven spine items in §4 order, and lists each of the 12 optional sections with its attach condition"
      - "The provenance obligation is encoded: every planner-created issue ends with a v1 marker in the §8.3 form, and the gate_compliance criterion lists the provenance checks"
      - "Five '### Criterion:' sections exist (conversation_protocol, elicitation_completeness, gate_compliance, issue_quality, requirements_boundary), each listing its checks with type and parameters"
      - "At least 16 '### Case <n>:' sections exist, covering every scenario in the issue, each with turns (scripted or simulated), fixture needs and checks; two elicitation cases have at least 5 hidden facts each, at least one discoverable; at least one case asserts the provenance marker"
      - "The agent pass threshold of 95% is stated"
      - "Every check ID in the inventory tables is defined in the Criteria section"
      - "Only .kairon/iterations/planner/eval-spec.md is created or modified"
    validation_commands:
      - "test $(grep -cE '^### Case [0-9]+: ' .kairon/iterations/planner/eval-spec.md) -ge 16"
      - "for c in conversation_protocol elicitation_completeness gate_compliance issue_quality requirements_boundary; do grep -qE \"^### Criterion: $c\" .kairon/iterations/planner/eval-spec.md || exit 1; done"
      - "for s in Feature Bug Refactor 'Functional Requirements' 'Interface / Contract' 'Edge Cases & Error Handling' 'Example Interactions' 'Reproduction' 'Current / Target State' 'Behavior-Preservation Contract' 'Baseline & Target' 'Security' 'Migration' 'Operational' 'Data / Migration'; do grep -qF \"$s\" .kairon/iterations/planner/eval-spec.md || exit 1; done"
      - "awk '/^### Criterion: gate_compliance/{f=1;next} /^### Criterion: /{f=0} f' .kairon/iterations/planner/eval-spec.md | grep -q 'planner-provenance: v1'"
      - "grep -qE 'pass threshold.*95%|95%.*pass threshold' .kairon/iterations/planner/eval-spec.md"
      - "for t in command file_exists file_absent file_contains file_not_contains changed_files output_contains output_not_contains gh_log_contains gh_log_not_contains judge fact_asked fact_not_asked; do grep -q \"$t\" .kairon/iterations/planner/eval-spec.md || exit 1; done"
      - "for s in '§4' '§6' '§8' '§10'; do grep -qF \"$s\" .kairon/iterations/planner/eval-spec.md || exit 1; done"
      - "grep -qE 'GC-12' .kairon/iterations/planner/eval-spec.md"

  - id: "validate-eval-spec"
    agent: "validator"
    description: "Read-only verification of issue #369 acceptance criteria 1 to 7 against .kairon/iterations/planner/eval-spec.md: every instruction line of planner-prompt.md and of planner-conventions SKILL.md appears once with a valid decision; three types, spine and optional sections with attach conditions; provenance obligation in gate_compliance; five criteria with checks from the 13-type vocabulary; at least 16 cases covering every listed scenario with turns, fixtures and checks, two elicitation cases with at least 5 hidden facts and at least one discoverable fact, and a provenance-asserting case; 95% threshold stated. Also confirm only the deliverable and this spec (and the sentinel) changed, and that planner-requirements.md was not copied into the worktree."
    dependencies: ["write-contract-criteria-cases"]
    acceptance_criteria:
      - "All validation commands of the two builder tasks pass"
      - "Each of the seven issue acceptance criteria is confirmed with evidence"
      - "Section numbers cited from planner-requirements.md (§4, §6, §8, §10) match that document, and the provenance marker matches §8.3 exactly"
      - "No file other than .kairon/iterations/planner/eval-spec.md, .kairon/specs/issue-369-planner-eval-spec.md and .kairon/artifacts/ sentinels is added or changed by this task"
    validation_commands:
      - "test -f .kairon/iterations/planner/eval-spec.md"
      - "test ! -e .kairon/specs/maturity-model/planner-requirements.md"
      - "test -z \"$(git -P status --porcelain -- . ':!.kiro/agents/*.json' ':!.kairon/iterations/planner/eval-spec.md' ':!.kairon/specs/issue-369-planner-eval-spec.md' ':!.kairon/artifacts')\""
      - "grep -qF '<!-- planner-provenance: v1 sha256=' .kairon/iterations/planner/eval-spec.md"
```
