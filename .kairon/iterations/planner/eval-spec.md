# Planner Eval Specification

**Issue:** #369 (step P1 of `gap-analysis.md` §S3.6)
**Status:** Draft for review

## Purpose and sources

This document defines what the rebuilt planner must produce and guarantee. It gives the later Eval-Driven Development steps (P2 onward in `gap-analysis.md` §S3.6) a reviewed target. It is docs-only: no prompt, rubric, case, fixture or code file changes with it.

Two kinds of names appear below. `P1`, `P2`, `P3` and `P-override` (without a row number) are iteration steps in `gap-analysis.md` §S3.6. `P<n>` and `S<n>` as table row IDs are line numbers in the files they inventory, so row `P3` is line 3 of `planner-prompt.md`, not step P3. Written `§N` or `§N.M`, a citation points into `planner-requirements.md`: `§N` is a section, and `§10.1.4` is step 4 of the flow in §10.1. `§S3.6` points into `gap-analysis.md`.

### Sources

| Source | Path | Used for |
|--------|------|----------|
| Planner requirements | `.kairon/specs/maturity-model/planner-requirements.md` (main checkout; untracked, so not in worktrees) | Issue model, provenance, elicitation flow, hard bars. Wins on conflict. |
| Gap analysis §S3.6 | `.kairon/specs/maturity-model/gap-analysis.md` | Planner issue standard, planner design, the P1 prompt, the Kairon-specific guidance for the override. |
| Planner prompt | `.kiro/agents/planner-prompt.md` | Subject of the behavior inventory. |
| Planner conventions skill | `.kiro/skills/planner-conventions/SKILL.md` | Subject of the override inventory. |
| Planner agent config | `.kiro/agents/planner.json` | Shows the skill is loaded through `resources`. |
| Eval documentation | `docs/evaluation.md` | Check vocabulary, `turn`, `judge`, simulated user, fake `gh`, case workspaces. |
| Iteration logs | `.kairon/iterations/README.md` | Where this file and later iteration logs live. |

### Precedence

`planner-requirements.md` wins over `gap-analysis.md` §S3.6 wherever they conflict. Nothing here goes beyond what those sources say. Where a source is silent, the gap is recorded under "Open questions" instead of being decided silently.

### Conflicts resolved

| Topic | gap-analysis.md §S3.6 | planner-requirements.md (wins) |
|-------|----------------------|-------------------------------|
| Section order | Problem, Scope, Acceptance Criteria, Constraints, Dependencies, Context | §4: Title, Problem Statement, Context, Scope, Acceptance Criteria, Constraints (optional), Dependencies |
| Dependencies form | `- #<n> must merge first` bullets | §7: single line `Dependencies: #12, #15`. A bullet list parses to zero dependencies |
| "Reference dependencies by issue number" | Kairon override | §7: base behavior |
| Issue types | not modelled | §3: Feature, Bug, Refactor, plus optional sections (§6) |
| Provenance | absent | §8: v1 marker, and a gate_compliance obligation (§10.2) |
| Intake of manual issues | absent | §9, not covered by a case here (see Open questions) |

The Kairon-specific guidance in `gap-analysis.md` §S3.6 stays in the override: Verify commands use `go run ./cmd/kairon` or `task`, and the template-sync Constraint applies to template-synced paths. It is listed under "Additions" below.

### Line convention for the two inventories

An *instruction line* is every non-blank physical line of the file except: ATX headings (`#` to `######`) outside code fences, code-fence delimiter lines, and the two `---` front-matter delimiters of a SKILL.md. Lines inside fences count. Row IDs are `P<n>` (prompt line number n) and `S<n>` (skill line number n), one row per line, no ranges, each exactly once. In the current tree `planner-prompt.md` has 92 instruction lines and the planner-conventions `SKILL.md` has 108.

The line numbers come from this awk program, which is also the validation check:

    /^[ \t]*```/{f=!f;next} /^[ \t]*$/{next} !f&&/^#{1,6} /{next} NR<=5&&/^---[ \t]*$/{next} {print NR}

`kairon eval audit` excludes fenced lines. The inventory includes them, so the inventory is a superset of the audit's instruction set.

Both tables use the columns `| ID | Excerpt | Decision | Criterion | Checks | Reason |`.

- **Excerpt:** the first 50 characters of the trimmed line in a code span. Backticks in the line are shown as `'`, and `|` is escaped.
- **Decision (prompt):** `Keep` or `Drop`.
- **Decision (skill):** `move to base prompt`, `stay in override` or `drop`.
- **Criterion and Checks:** `Keep` and `move to base prompt` rows name exactly one of the five criteria (conversation_protocol, elicitation_completeness, gate_compliance, issue_quality, requirements_boundary) and the check IDs that enforce them. All other rows have `-` in both columns. Check IDs (`CP-n`, `EC-n`, `GC-n`, `IQ-n`, `RB-n`) are defined in `## Criteria`.
- **Reason:** one line.

### Scope boundary

This change adds one file. It does not edit the planner prompt, the skill, `planner.json`, the legacy rubric `.kairon/evals/rubrics/planner.yaml`, any eval case or fixture, or any code. Manual-issue intake (§9) and spike requests (§3) are not covered by a case; see Open questions.

## Planner contract

What the rebuilt planner must produce and guarantee. Section numbers (§N) cite `planner-requirements.md`. The checks that enforce each part are defined in `## Criteria`.

### Three-type model

The planner classifies every request into exactly one of three types (§3), because there are only three shapes of acceptance criteria. Any extra detail goes into an optional section (below).

| Type | Contract | Acceptance-criteria shape |
|------|----------|---------------------------|
| Feature | Additive: behavior that does not exist should exist. | Forward specification of new observable behavior. |
| Bug | Corrective: observable behavior is wrong. The issue gives the gap and a reproduction. | Current versus expected behavior, plus a deterministic reproduction. |
| Refactor | Structural: internal structure changes and observable behavior must not. | A behavior-preservation guarantee. |

Other former types fold into these three (§3): documentation into Feature or Bug, performance into Feature plus *Baseline & Target*, a security fix into Bug plus *Security*, a chore or dependency upgrade into Refactor plus *Migration*, infrastructure into Feature or Refactor plus *Operational*. A spike is excluded (see Open questions). A wrong classification costs at most one extra or missing optional block, never a broken contract (§3); IQ-OPT checks the classification outcome.

### Shared spine

Every issue, whatever its type, has these seven items in this order (§4). The checks are IQ-1 (order), IQ-2 (Constraints position), IQ-4 and IQ-5 (scope lists), IQ-7 (acceptance criteria), IQ-9 (title) and IQ-10 (dependencies).

| # | Item | Required content (§4) |
|---|------|-----------------------|
| 1 | Title | One line, imperative, scoped, at most 70 characters, one PR's worth of change. It is the `--title` of `gh issue create`, not a body heading. |
| 2 | Problem Statement | 2 to 5 sentences: what is true today, the evidence (file:line, command output, observed behavior), why it is a problem, and what should be true after. The problem is stated apart from any solution. |
| 3 | Context | Why it matters, who is affected, and resolvable references (exact paths, symbols, issue or PR numbers, fetchable docs). Non-binding only. |
| 4 | Scope (In and Out) | In Scope lists packages or components (existing, or marked `(new)`); a single file is named only when the requirement is about that file. Out of Scope has at least one entry. |
| 5 | Acceptance Criteria | Independently verifiable, observable-outcome statements, each with a black-box `Verify:` line. |
| 6 | Constraints (optional, binding) | Hard rules the implementation must obey: a mandated package, pattern, interface or compatibility rule. Each has a reason and a verification. |
| 7 | Dependencies | Issues that must land first, in the single-line form below. |

Absent by design (§4, §5): no User Story section, no Verification or Test Plan section, and no Definition-of-Done-as-commands section. Test commands, CI flags and coverage gates are pipeline-level concerns and are not restated per issue (IQ-3, IQ-11, IQ-12). For a Bug, the *Reproduction* is the one exception: it describes the problem, so it stays.

Size limits (§10.1.10): at most 6 In Scope entries and at most 8 acceptance criteria (IQ-6).

### Optional sections

The planner attaches these by classification, and only when the request calls for them (§6). They are where the former issue types now live. IQ-OPT checks the attachment: required sections must be present for the case's type, and the sections that belong to other types must be absent.

| # | Section | Attach condition (§6) | Carries |
|---|---------|-----------------------|---------|
| 1 | Functional Requirements | Feature | Numbered, each testable: inputs, outputs, state changes. |
| 2 | Interface / Contract | Feature with an external surface | Exact signatures, request and response schemas, status codes, error shapes; for UI, component states and a design reference. |
| 3 | Edge Cases & Error Handling | Feature or Bug with non-trivial failure modes | Enumerated failure cases (empty input, nulls, concurrency, limits, auth failures). The agent handles exactly the cases named. |
| 4 | Example Interactions | Feature | Concrete input to expected output pairs; they double as acceptance tests. |
| 5 | Reproduction | Bug | Numbered deterministic steps from a known state, the minimal triggering input, the environment (version or commit, OS, runtime, config, flags), and exact evidence pasted verbatim. |
| 6 | Current / Target State | Refactor | The structure today (with paths) and the desired structure, concretely. |
| 7 | Behavior-Preservation Contract | Refactor | The explicit no-functional-change rule; the acceptance test is "existing tests still pass unchanged", plus any characterization tests first. |
| 8 | Baseline & Target | Performance (a Feature costume) | A measured baseline, a quantified target, and the correctness guard that proves results did not change. |
| 9 | Security | Security fix (a Bug costume) | Vulnerability class (CWE or CVE), threat model, affected surface, a test proving the exploit is closed; deliberate about detail in a public issue. |
| 10 | Migration | Chore or dependency upgrade (a Refactor costume) | Exact target versions (pinned, not ranges), the changelog or migration to follow, breaking-change handling; flag unusual package names. |
| 11 | Operational | Infrastructure or IaC | Target state, whether prod is in scope, idempotency, rollback plan, blast-radius limits; a prod-touching change requires a human gate. |
| 12 | Data / Migration | Any schema change | Schema changes, backfills, feature flags. |

Cases 1 to 19 exercise Feature (Interface / Contract, Edge Cases & Error Handling, Example Interactions), Bug (Reproduction) and Refactor (Current / Target State, Behavior-Preservation Contract). Sections 1 and 8 to 12 have attach rules but no dedicated case (see Open questions).

### Dependencies format

The planner emits the single-line form (§7):

    Dependencies: #12, #15, #18

A `## Dependencies` header followed by a bullet list parses to zero dependencies and releases dependents early (a confirmed failure mode), so it is a defect (IQ-10). A dependency is satisfied only when the referenced issue is closed. The planner refers to dependencies by issue number only. When it files issues in a chain, it captures each returned number and substitutes it into later issues' `Dependencies:` line before filing (Case 19). When there is no dependency, this spec accepts `Dependencies: None` (Open question 6).

### Provenance marker

Every issue the planner creates ends with a provenance marker (§8, §10.2). The marker must show that the body is the planner's output, unmodified, so a manual issue (no marker) and an edited planner issue (hash mismatch) can both be told apart from a valid one (§8.1, §8.4).

- **Form (§8.3):** a single HTML comment on the last line of the body, excluded from its own hash input:

        <!-- planner-provenance: v1 sha256=<hex> generated=<ISO-8601> -->

- **Hash (§8.2):** SHA-256 over the canonicalized body with the marker line stripped, line endings normalized and trailing whitespace removed. The scheme is tamper-evident, not tamper-proof: no HMAC and no secret. It is versioned (`v1`) so canonicalization can change later.
- **Enforced by:** GC-10 (marker present, last line, v1 form), GC-11 (exactly one marker) and GC-12 (the claimed hash equals a recomputed hash), all under `gate_compliance`. Cases 4, 7, 8, 9, 12 to 19 assert them on every created body.
- **Out of scope here:** the pipeline's three read states and the auto-routing of manual issues (§8.4, §9). See Open question 5.

### Hard bars

These hold for every planner-created issue (§10.2). Each maps to checks, so a violation fails a case.

| Hard bar (§10.2) | Checks |
|------------------|--------|
| No unconfirmed assumptions: every statement traces to the code, the user, or a default the user accepted. | EC-4 |
| No vague terms in acceptance criteria ("properly", "appropriate", "gracefully", "fast", "robust", "as needed", "etc.", "should work") unless quantified. | IQ-8 |
| No design outside Constraints, and no Constraint the user did not state or confirm. | RB-2, RB-3, RB-4 |
| Never ask something the repository answers. | EC-3, EC-5, EC-6 |
| Every created issue ends with a valid provenance marker (§8). | GC-10, GC-11, GC-12 |

## Behavior inventory: planner-prompt.md

One row for each of the 92 instruction lines of `.kiro/agents/planner-prompt.md`: 39 Keep, 53 Drop. Keep rows name the criterion that will check the behavior. Several Keep rows say the quoted wording is not pinned; the check is on behavior, not on the words.

| ID | Excerpt | Decision | Criterion | Checks | Reason |
|----|---------|----------|-----------|--------|--------|
| P3 | `You are an interactive planning agent for kairon. ` | Keep | requirements_boundary | RB-1, RB-6 | Role and scope: refine an idea into an issue, then create it. |
| P7 | `- You MUST NOT write, modify, or create any projec` | Keep | requirements_boundary | RB-1 | No project file changes; `changed_files allow: []`. |
| P8 | `- You MUST NOT implement solutions, fix bugs, or w` | Keep | requirements_boundary | RB-6, RB-7 | Never implement or fix; redirect. |
| P9 | `- You MUST NOT run build commands, tests, or any c` | Keep | requirements_boundary | RB-1 | No state-changing commands; the effect is checked on the workspace. |
| P10 | `- Your ONLY permitted shell commands are: 'gh issu` | Keep | gate_compliance | GC-9 | Shell allowlist. Wording is revised in step P3 to allow read-only `gh issue view/list` and the hash command (§8). |
| P11 | `- If the user asks you to fix or implement somethi` | Keep | requirements_boundary | RB-6 | Redirect when asked to implement; wording is not pinned (judge). |
| P12 | `- You are a PLANNER, not an implementer. Your outp` | Drop | - | - | Restates rows P3 and P8. |
| P16 | `You MUST ask only ONE question per response. Never` | Keep | conversation_protocol | CP-1, CP-6 | One decision per turn. The rationale sentence at the end of the line may be cut. |
| P20 | `**Prohibition on Multiple Questions:**` | Drop | - | - | Duplicate of row P16 (one question, no lists, wait for the answer). |
| P21 | `- NEVER ask multiple questions in one response` | Drop | - | - | Duplicate of row P16 (one question, no lists, wait for the answer). |
| P22 | `- NEVER present bulleted lists of questions` | Drop | - | - | Duplicate of row P16 (one question, no lists, wait for the answer). |
| P23 | `- Wait for response before asking follow-up questi` | Drop | - | - | Duplicate of row P16 (one question, no lists, wait for the answer). |
| P25 | `**Option Presentation Format:**` | Drop | - | - | Label only; the rule is row P26. |
| P26 | `When presenting options, use structured a, b, c fo` | Keep | conversation_protocol | CP-2, CP-3 | Options as a) b) c) with Other. |
| P28 | `**✅ GOOD - Clear options:**` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P29 | `"How should users authenticate?` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P30 | `a) JWT tokens with login form` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P31 | `b) OAuth with GitHub/Google` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P32 | `c) API keys for service accounts` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P33 | `d) Other (please specify)"` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P35 | `**❌ BAD - Ambiguous yes/no:**` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P36 | `"Should we use JWT tokens or would you prefer OAut` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P38 | `**Examples of Problematic vs. Improved Patterns:**` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P40 | `**❌ PROBLEMATIC - Multiple questions:**` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P41 | `"What authentication method do you want? Should it` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P43 | `**✅ IMPROVED - Single focused question:**` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P44 | `"What authentication method should users use to lo` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P46 | `**❌ PROBLEMATIC - Ambiguous either/or:**` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P47 | `"Do you want to store user data in a database or u` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P49 | `**✅ IMPROVED - Clear options:**` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P50 | `"Where should user data be stored?` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P51 | `a) Database (PostgreSQL/MySQL)` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P52 | `b) File system (JSON/YAML files)` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P53 | `c) In-memory (session only)` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P54 | `d) Other storage option"` | Drop | - | - | Example scaffolding; the format is checked by CP-2 and CP-3. Re-add an example only if a red iteration shows the model needs it. |
| P58 | `You MUST follow these gates in order. These gates ` | Keep | gate_compliance | GC-1, GC-2 | Gates in order and not skippable; the bypass case checks behavior. |
| P62 | `1. **Understand the Request**: Take the user's ini` | Keep | elicitation_completeness | EC-5, EC-6 | Investigate before asking; read-only. |
| P64 | `2. **Root Cause Analysis** (for bug reports and er` | Keep | conversation_protocol | CP-9 | Bug root-cause analysis exists to feed the symptom / root cause choice. |
| P65 | `- **Code Tracing**: When errors or bugs are report` | Keep | elicitation_completeness | EC-6 | Trace from error to source so the root cause is stated with a file reference. |
| P66 | `- Follow stack traces to identify originating func` | Drop | - | - | Method detail; the outcome is checked via row P65. |
| P67 | `- Examine error handling paths and exception propa` | Drop | - | - | Method detail; the outcome is checked via row P65. |
| P68 | `- Identify where problematic data or state origina` | Drop | - | - | Method detail; the outcome is checked via row P65. |
| P69 | `- **Hypothesis Testing**: Create planning worktree` | Drop | - | - | Planning-worktree hypothesis testing is Kairon-specific and changes the workspace (RB-1); carried by override rows S32, S60-S62, S66-S70, S74-S75, S78. |
| P70 | `- Use '.kairon/scripts/planning-worktree-create.sh` | Drop | - | - | Planning-worktree hypothesis testing is Kairon-specific and changes the workspace (RB-1); carried by override rows S32, S60-S62, S66-S70, S74-S75, S78. |
| P71 | `- Examine code paths under different conditions wi` | Drop | - | - | Planning-worktree hypothesis testing is Kairon-specific and changes the workspace (RB-1); carried by override rows S32, S60-S62, S66-S70, S74-S75, S78. |
| P72 | `- Test reproduction scenarios safely in planning w` | Drop | - | - | Planning-worktree hypothesis testing is Kairon-specific and changes the workspace (RB-1); carried by override rows S32, S60-S62, S66-S70, S74-S75, S78. |
| P73 | `- **MANDATORY**: Always clean up with '.kairon/scr` | Drop | - | - | Planning-worktree hypothesis testing is Kairon-specific and changes the workspace (RB-1); carried by override rows S32, S60-S62, S66-S70, S74-S75, S78. |
| P74 | `- **Multi-Layer Investigation**: Investigate acros` | Drop | - | - | Multi-layer investigation is not checkable; the outcome is checked via row P65. |
| P75 | `- Application logic layer (business rules, data fl` | Drop | - | - | Multi-layer investigation is not checkable; the outcome is checked via row P65. |
| P76 | `- Infrastructure layer (configuration, dependencie` | Drop | - | - | Multi-layer investigation is not checkable; the outcome is checked via row P65. |
| P77 | `- Integration layer (API calls, database queries, ` | Drop | - | - | Multi-layer investigation is not checkable; the outcome is checked via row P65. |
| P78 | `- User interface layer (input validation, error di` | Drop | - | - | Multi-layer investigation is not checkable; the outcome is checked via row P65. |
| P79 | `- **Root Cause vs Symptom Decision**: When root ca` | Keep | conversation_protocol | CP-9 | Symptom / root cause decision (§10.1 step 4). |
| P80 | `- **Analysis Summary**: Briefly explain your metho` | Drop | - | - | Methodology narration; findings are covered by row P87. |
| P81 | `- **Decision Options**: Present choices in clear a` | Drop | - | - | Duplicate of row P26. |
| P82 | `- "Based on my analysis, I found the root cause di` | Drop | - | - | Example lead-in; wording is not pinned. |
| P83 | `- a) Address the reported symptom as described (qu` | Keep | conversation_protocol | CP-9 | The three required options: symptom, root cause, both. |
| P84 | `- b) Address the discovered root cause (comprehens` | Keep | conversation_protocol | CP-9 | The three required options: symptom, root cause, both. |
| P85 | `- c) Address both symptom and root cause (layered ` | Keep | conversation_protocol | CP-9 | The three required options: symptom, root cause, both. |
| P86 | `- d) Other approach (please specify)"` | Drop | - | - | Duplicate of row P26 (Other option). |
| P87 | `- **Context**: Explain the difference between symp` | Keep | conversation_protocol | CP-9 | Explain the difference between symptom and root cause. |
| P88 | `- **User Authority**: Wait for user decision befor` | Keep | gate_compliance | GC-1 | Do not proceed to creation before the user decides. |
| P90 | `3. **Collaborate**: Ask clarifying questions ONE A` | Drop | - | - | Duplicate of row P16. |
| P91 | `4. **Draft the Issue**: When you have enough infor` | Keep | elicitation_completeness | EC-8 | Draft only when no coverage slot is open. |
| P92 | `- Problem Statement` | Keep | issue_quality | IQ-1 | Problem Statement section (§4). |
| P93 | `- User Story` | Drop | - | - | §4 has no User Story section; IQ-3 checks it is absent. |
| P94 | `- Acceptance Criteria (testable)` | Keep | issue_quality | IQ-1, IQ-7 | Acceptance Criteria with Verify lines. |
| P95 | `- Constraints` | Keep | requirements_boundary | RB-4 | Constraints become optional and only user-stated or user-confirmed; wording must change in step P3. |
| P96 | `- Context/references to relevant code` | Keep | issue_quality | IQ-1, IQ-18 | Context with resolvable references. |
| P97 | `5. **MANDATORY DRAFT REVIEW**: You MUST show the c` | Keep | gate_compliance | GC-1 | Mandatory draft review and approval gate (no create before approval); quoted wording is not pinned. The draft shape is checked by CP-10 to CP-12. |
| P98 | `- You MUST wait for explicit approval before proce` | Keep | gate_compliance | GC-1 | Wait for explicit approval before creating. |
| P99 | `- You MUST NOT proceed without user approval` | Drop | - | - | Duplicate of row P98. |
| P100 | `- If changes are requested, revise and ask for app` | Keep | gate_compliance | GC-1, GC-2 | Revise and ask again (revision case). |
| P101 | `- NEVER skip this gate under any circumstances` | Drop | - | - | Duplicate of row P58. |
| P105 | `6. **MANDATORY LABEL CONFIRMATION**: After draft a` | Keep | gate_compliance | GC-3, GC-4 | Separate label question after approval; the label name comes from config, not hard-coded. |
| P106 | `- This MUST be asked as a separate question after ` | Drop | - | - | Duplicate of row P105. |
| P107 | `- You MUST wait for explicit confirmation` | Keep | gate_compliance | GC-2 | Wait for the label answer before creating. |
| P108 | `- You MUST NOT assume the answer` | Drop | - | - | Duplicate of row P107. |
| P109 | `- NEVER skip this gate under any circumstances` | Drop | - | - | Exact duplicate of row P101. |
| P113 | `7. **Create**: Only after both gates are passed, u` | Keep | gate_compliance | GC-5, GC-6, GC-7 | Create only after both gates. |
| P114 | `8. **Mandatory Cleanup**: After issue creation (su` | Drop | - | - | Worktree cleanup; worktrees are dropped from the base (see rows P69-P73). |
| P118 | `Write the issue body to a temporary file, then cre` | Keep | gate_compliance | GC-8, GC-10 | Body goes through `--body-file`; the marker is the last body line. |
| P120 | `If the user confirmed labeling:` | Keep | gate_compliance | GC-7 | Intro to the labeled create command. |
| P122 | `gh issue create --repo <REPO> --title "<title>" --` | Keep | gate_compliance | GC-6, GC-7 | Labeled create command with repo and label. |
| P125 | `If the user declined labeling:` | Keep | gate_compliance | GC-7b | Intro to the unlabeled create command. |
| P127 | `gh issue create --repo <REPO> --title "<title>" --` | Keep | gate_compliance | GC-6, GC-7b | Unlabeled create command. |
| P130 | `The repository and label are configured in '.kairo` | Keep | gate_compliance | GC-6, GC-7 | Repo and label come from `.kairon/config.yaml`. |
| P134 | `- Focus on the problem space, not implementation d` | Keep | requirements_boundary | RB-2 | Problem space, not implementation. |
| P135 | `- Ensure acceptance criteria are specific and test` | Keep | issue_quality | IQ-7, IQ-8 | Specific, testable acceptance criteria. |
| P136 | `- Include references to relevant files or code whe` | Keep | issue_quality | IQ-18 | References to relevant files. |
| P137 | `- Keep the conversation focused and efficient` | Drop | - | - | Unmeasurable filler. |
| P138 | `- NEVER suggest or provide code fixes — that is th` | Keep | requirements_boundary | RB-7 | No suggested code fixes in chat or in the issue. |

## Override inventory: planner-conventions

One row for each of the 108 instruction lines of `.kiro/skills/planner-conventions/SKILL.md`: 4 move to base prompt, 14 stay in override, 90 drop. The skill is Kairon's override hook (`planner.json` lists it under `resources`). It is not part of the certified agent. Only "move to base prompt" rows name a criterion and checks.

| ID | Excerpt | Decision | Criterion | Checks | Reason |
|----|---------|----------|-----------|--------|--------|
| S2 | `name: planner-conventions` | stay in override | - | - | Hook identity (`planner.json` lists this path); the description is rewritten for Kairon-specific guidance. |
| S3 | `description: Comprehensive analysis methodology an` | stay in override | - | - | Hook identity (`planner.json` lists this path); the description is rewritten for Kairon-specific guidance. |
| S8 | `Advanced root cause analysis techniques for transf` | drop | - | - | Filler intro. |
| S13 | `1. **Symptom Collection**: Document all reported s` | move to base prompt | issue_quality | IQ-BUG | Symptom, context, impact and evidence are the Bug Reproduction and Context content (§6, §4). |
| S14 | `2. **Context Mapping**: Identify when, where, and ` | move to base prompt | issue_quality | IQ-BUG | Symptom, context, impact and evidence are the Bug Reproduction and Context content (§6, §4). |
| S15 | `3. **Impact Scope**: Determine breadth of affected` | move to base prompt | issue_quality | IQ-BUG | Symptom, context, impact and evidence are the Bug Reproduction and Context content (§6, §4). |
| S16 | `4. **Evidence Gathering**: Collect logs, stack tra` | move to base prompt | issue_quality | IQ-BUG | Symptom, context, impact and evidence are the Bug Reproduction and Context content (§6, §4). |
| S19 | `1. **Error Point Location**: Find exact source loc` | drop | - | - | Duplicate of prompt row P65; the outcome is checked, the method is not. |
| S20 | `2. **Call Stack Analysis**: Trace execution path b` | drop | - | - | Duplicate of prompt row P65; the outcome is checked, the method is not. |
| S21 | `3. **Data Flow Tracking**: Follow data transformat` | drop | - | - | Duplicate of prompt row P65; the outcome is checked, the method is not. |
| S22 | `4. **Dependency Mapping**: Identify external depen` | drop | - | - | Duplicate of prompt row P65; the outcome is checked, the method is not. |
| S23 | `5. **Configuration Analysis**: Examine relevant co` | drop | - | - | Duplicate of prompt row P65; the outcome is checked, the method is not. |
| S26 | `1. **Root Cause Candidates**: Generate 3-5 potenti` | drop | - | - | The planner does not debug; not checkable. |
| S27 | `2. **Hypothesis Ranking**: Prioritize by likelihoo` | drop | - | - | The planner does not debug; not checkable. |
| S28 | `3. **Test Design**: Plan specific experiments to v` | drop | - | - | The planner does not debug; not checkable. |
| S29 | `4. **Success Criteria**: Define what evidence woul` | drop | - | - | The planner does not debug; not checkable. |
| S32 | `1. **Planning Worktree Creation**: Use isolated en` | stay in override | - | - | Kairon-specific planning-worktree use (see prompt rows P69-P73). |
| S33 | `2. **Controlled Experiments**: Test one hypothesis` | drop | - | - | Generic experiment method. |
| S34 | `3. **Evidence Collection**: Document results of ea` | drop | - | - | Generic experiment method. |
| S35 | `4. **Iterative Refinement**: Adjust hypotheses bas` | drop | - | - | Generic experiment method. |
| S41 | `ERROR MESSAGE → Exception Location → Calling Funct` | drop | - | - | Diagrams and lists of generic technique. |
| S45 | `1. **Input Validation**: Check data at system boun` | drop | - | - | Diagrams and lists of generic technique. |
| S46 | `2. **Transformation Points**: Examine where data c` | drop | - | - | Diagrams and lists of generic technique. |
| S47 | `3. **State Mutations**: Identify where shared stat` | drop | - | - | Diagrams and lists of generic technique. |
| S48 | `4. **Output Generation**: Trace how results are pr` | drop | - | - | Diagrams and lists of generic technique. |
| S51 | `1. **Direct Dependencies**: Libraries, frameworks,` | drop | - | - | Diagrams and lists of generic technique. |
| S52 | `2. **Indirect Dependencies**: Configuration, envir` | drop | - | - | Diagrams and lists of generic technique. |
| S53 | `3. **Temporal Dependencies**: Timing, ordering, ra` | drop | - | - | Diagrams and lists of generic technique. |
| S54 | `4. **Resource Dependencies**: Memory, disk, networ` | drop | - | - | Diagrams and lists of generic technique. |
| S60 | `# Create planning worktree for investigation` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S61 | `.kairon/scripts/planning-worktree-create.sh` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S62 | `# Returns: /path/to/planning/worktree` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S66 | `- **Code Modifications**: Add debug logging, instr` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S67 | `- **Test Creation**: Write minimal reproduction ca` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S68 | `- **Configuration Changes**: Temporarily modify se` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S69 | `- **Dependency Testing**: Add/remove dependencies ` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S70 | `- **Environment Simulation**: Replicate production` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S74 | `# Mandatory cleanup after investigation` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S75 | `.kairon/scripts/planning-worktree-cleanup.sh <work` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S78 | `**Critical**: Planning worktrees must NEVER affect` | stay in override | - | - | Kairon planning-worktree scripts and the rules for using them. The certified base planner is read-only (RB-1); Kairon's override may add this (Open question 3). |
| S83 | `1. **Minimal Reproduction**: Create simplest case ` | drop | - | - | Generic hypothesis-validation methods. |
| S84 | `2. **Variable Isolation**: Change one factor at a ` | drop | - | - | Generic hypothesis-validation methods. |
| S85 | `3. **Boundary Conditions**: Test edge cases and li` | drop | - | - | Generic hypothesis-validation methods. |
| S86 | `4. **State Verification**: Confirm intermediate st` | drop | - | - | Generic hypothesis-validation methods. |
| S89 | `1. **Mock Dependencies**: Replace external service` | drop | - | - | Generic hypothesis-validation methods. |
| S90 | `2. **Configuration Variants**: Test different conf` | drop | - | - | Generic hypothesis-validation methods. |
| S91 | `3. **Load Simulation**: Test under various load co` | drop | - | - | Generic hypothesis-validation methods. |
| S92 | `4. **Error Injection**: Introduce controlled failu` | drop | - | - | Generic hypothesis-validation methods. |
| S95 | `1. **Git History**: When was problematic code intr` | drop | - | - | Generic hypothesis-validation methods. |
| S96 | `2. **Change Correlation**: What changes coincided ` | drop | - | - | Generic hypothesis-validation methods. |
| S97 | `3. **Regression Testing**: Does reverting specific` | drop | - | - | Generic hypothesis-validation methods. |
| S101 | `When investigation reveals root cause differs from` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S105 | `## Analysis Results` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S107 | `**Reported Symptom**: [User's description]` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S108 | `**Root Cause Discovered**: [Technical root cause]` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S110 | `### Implementation Options:` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S112 | `a) **Address Reported Symptom**: [Surface-level fi` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S113 | `- Pros: Quick resolution, matches user expectation` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S114 | `- Cons: Underlying issue persists, potential for r` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S116 | `b) **Address Root Cause**: [Comprehensive fix desc` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S117 | `- Pros: Permanent resolution, prevents related iss` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S118 | `- Cons: Larger scope, may affect other functionali` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S120 | `c) **Hybrid Approach**: [Combined strategy]` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S121 | `- Immediate symptom relief + planned root cause fi` | drop | - | - | Decision template duplicates prompt rows P83-P85 and P87; pros/cons are not required by §10.1 step 4. |
| S125 | `- Present technical analysis clearly` | drop | - | - | Duplicate of prompt rows P79-P88. |
| S126 | `- Explain implications of each approach` | drop | - | - | Duplicate of prompt rows P79-P88. |
| S127 | `- Respect user's final choice` | drop | - | - | Duplicate of prompt rows P79-P88. |
| S128 | `- Document reasoning regardless of selected option` | drop | - | - | Duplicate of prompt rows P79-P88. |
| S133 | `- [ ] Symptom fully documented with reproduction s` | drop | - | - | Checklist items duplicate other rows or require worktree experiments. |
| S134 | `- [ ] Code path traced from symptom to source` | drop | - | - | Checklist items duplicate other rows or require worktree experiments. |
| S135 | `- [ ] At least 3 potential root causes identified` | drop | - | - | Checklist items duplicate other rows or require worktree experiments. |
| S136 | `- [ ] Hypotheses tested in planning worktree` | drop | - | - | Checklist items duplicate other rows or require worktree experiments. |
| S137 | `- [ ] Evidence collected for preferred explanation` | drop | - | - | Checklist items duplicate other rows or require worktree experiments. |
| S138 | `- [ ] Impact and scope clearly defined` | drop | - | - | Checklist items duplicate other rows or require worktree experiments. |
| S139 | `- [ ] Solution options presented with trade-offs` | drop | - | - | Checklist items duplicate other rows or require worktree experiments. |
| S142 | `- **Surface Level**: "Error happens when..."` | drop | - | - | Generic engineering lore. |
| S143 | `- **Intermediate**: "Error happens because X calls` | drop | - | - | Generic engineering lore. |
| S144 | `- **Deep Analysis**: "Error happens because config` | drop | - | - | Generic engineering lore. |
| S149 | `1. **Environment Mismatches**: Development vs prod` | drop | - | - | Generic engineering lore. |
| S150 | `2. **Default Value Problems**: Missing or incorrec` | drop | - | - | Generic engineering lore. |
| S151 | `3. **Override Conflicts**: Multiple configuration ` | drop | - | - | Generic engineering lore. |
| S154 | `1. **API Contract Changes**: External service modi` | drop | - | - | Generic engineering lore. |
| S155 | `2. **Version Compatibility**: Library updates intr` | drop | - | - | Generic engineering lore. |
| S156 | `3. **Network/Infrastructure**: Connectivity, timeo` | drop | - | - | Generic engineering lore. |
| S159 | `1. **Race Conditions**: Timing-dependent behavior ` | drop | - | - | Generic engineering lore. |
| S160 | `2. **State Management**: Shared state corruption o` | drop | - | - | Generic engineering lore. |
| S161 | `3. **Edge Cases**: Unhandled boundary conditions o` | drop | - | - | Generic engineering lore. |
| S164 | `1. **Memory Leaks**: Gradual resource exhaustion o` | drop | - | - | Generic engineering lore. |
| S165 | `2. **Disk Space**: Storage limitations affecting f` | drop | - | - | Generic engineering lore. |
| S166 | `3. **Performance Degradation**: Algorithmic comple` | drop | - | - | Generic engineering lore. |
| S172 | `## Root Cause Analysis Report` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S174 | `### Symptom Summary` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S175 | `- **What**: Observable behavior` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S176 | `- **When**: Timing/conditions` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S177 | `- **Where**: Affected components` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S178 | `- **Impact**: User/system effects` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S180 | `### Investigation Findings` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S181 | `- **Code Path**: [Error location → Root source]` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S182 | `- **Contributing Factors**: [Environmental/configu` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S183 | `- **Evidence**: [Test results, logs, traces]` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S185 | `### Root Cause` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S186 | `**Primary Cause**: [Technical explanation]` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S187 | `**Contributing Factors**: [Secondary issues that e` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S189 | `### Recommended Solution` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S190 | `**Approach**: [Root cause vs symptom fix reasoning` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S191 | `**Implementation**: [High-level solution strategy]` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S192 | `**Risk Assessment**: [Potential impacts and mitiga` | drop | - | - | Alternative report format conflicts with the canonical issue schema (§4, §6); its Implementation line is design. |
| S195 | `This methodology ensures thorough investigation th` | drop | - | - | Filler closing line. |

### Additions

Requirements that have no line in today's prompt, so no inventory row can carry them. Each is listed with the source that requires it and the checks that will enforce it.

| Requirement | Source | Criterion | Checks |
|-------------|--------|-----------|--------|
| Three-type classification and optional sections | §3, §6, §10.1.2 | issue_quality | IQ-OPT |
| Coverage map; no open slot at draft | §10.1.3 | elicitation_completeness | EC-8 |
| Highest-impact-first interview | §10.1.4 | elicitation_completeness | EC-9 |
| Recommended option with one line on why | §10.1.4 | conversation_protocol | CP-4, CP-5 |
| Constraints asked, never invented | §10.1.5 | requirements_boundary | RB-4 |
| Escape hatch ("use your recommendations") | §10.1.6 | conversation_protocol | Case 6 |
| Confirm-defaults turn | §10.1.7 | conversation_protocol | CP-7, CP-8 |
| Coverage summary outside the issue | §10.1.8 | conversation_protocol, issue_quality | CP-10, IQ-17 |
| Split oversized requests | §10.1.10 | conversation_protocol, issue_quality | CP-14, IQ-6 |
| Provenance marker on every created issue | §8, §10.2 | gate_compliance | GC-10 to GC-12 |
| Shared spine and title | §4 | issue_quality | IQ-1, IQ-9 |
| Out of Scope entry and scope sizes | §4.4, §10.1.10 | issue_quality | IQ-4 to IQ-6 |
| Observable acceptance criteria with black-box Verify lines | §4.5, §5 | issue_quality | IQ-7, IQ-11 |
| No Test Plan or User Story section | §4, §5 | issue_quality | IQ-3 |
| Generic quality gates not restated | §5 | issue_quality | IQ-12 |
| Single-line `Dependencies:` | §7 | issue_quality | IQ-10 |
| No unconfirmed assumptions | §10.2 | elicitation_completeness | EC-4 |
| No vague terms in acceptance criteria | §10.2 | issue_quality | IQ-8 |
| Never ask what the repository answers | §10.2 | elicitation_completeness | EC-3, EC-5 |
| No design outside Constraints | §10.2 | requirements_boundary | RB-2, RB-3 |
| Issue body is a specification, not a prompt | §2.8 | issue_quality | IQ-17 |
| Resolvable references | §2.5 | issue_quality | IQ-18 |

Kairon override additions. These belong in Kairon's `planner-conventions` skill, not in the base prompt.

| Requirement | Source | Criterion | Checks |
|-------------|--------|-----------|--------|
| Verify commands use `go run ./cmd/kairon` or `task`, never a bare binary | gap-analysis.md §S3.6, Kairon-specific guidance | issue_quality | Case 15 (checked with a fixture conventions skill) |
| Constraint "keep `cmd/kairon/templates/` in sync (`task sync:check`)" when In Scope touches template-synced paths | gap-analysis.md §S3.6, Kairon-specific guidance | requirements_boundary | none in this spec; written in step P-override |
| Planning-worktree rows moved from the base prompt (rows P69-P73 and the S-rows that stay in override) | Override inventory above | requirements_boundary | none; Open question 3 |

Not covered: manual-issue intake (§9) and spike requests (§3).

## Criteria

Five criteria cover the planner: conversation_protocol (CP), elicitation_completeness (EC), gate_compliance (GC), issue_quality (IQ) and requirements_boundary (RB). Each section below lists its checks as `ID | type | parameters | rule`.

### Check conventions

- Every check is one entry of the 13-type vocabulary in `docs/evaluation.md` ("The thirteen check types"): `command`, `file_exists`, `file_absent`, `file_contains`, `file_not_contains`, `changed_files`, `output_contains`, `output_not_contains`, `gh_log_contains`, `gh_log_not_contains`, `judge`, `fact_asked`, `fact_not_asked`. The Parameters column gives the YAML fields of that type. The `criterion:` field is the section the row sits in, and a case adds `turn:` where it pins a check.
- **Repeated IDs.** An ID that appears on several rows stands for all of those rows. Each row is one check and counts separately in scoring.
- **Placeholders.** `<fact>`, `<path>` and similar are filled in by the case. A row with a placeholder is a template.
- **`turn`.** Allowed only on `output_*`, `gh_log_*` and `judge`, and 1-based. Turn-pinned checks appear only in scripted cases (`turns`). Simulated-user cases use final-state checks and a conversation-wide `judge`, because the number of turns is not known in advance. A `judge` check with `turn: k` is shown the conversation up to turn k and judges turn k's reply, written here as "the reply under review".
- **Evidence files.** The created issue body is `.eval/gh-body-1.md` (the second issue, `.eval/gh-body-2.md`), which the fake `gh` copies from `--body-file`. The title is read from `.eval/gh.log`. Every case sets `requires_sandbox: true`, because `gh_log_*` checks are only meaningful under the fake `gh`.
- **Spine headings.** The checks assume the body uses `##` headings for the spine sections, with `###` for In Scope and Out of Scope under `## Scope`. The Title is the `--title` argument, not a body heading.
- **Draft turns.** The IQ and RB regexes that read the body also run as `output_contains` or `output_not_contains` (same pattern) on the draft turn in cases that stop at a draft.
- **Regex dialect.** Patterns are Go RE2, unanchored, matched against the whole text. `$` without `(?m)` matches only the end of the whole text, not the end of a line, so patterns that must match at the end of a `gh.log` line carry `(?m)`. A literal backtick is written `\x60` and a single quote `\x27`, which keeps the patterns free of quoting problems. In tables, a `|` inside a pattern is written `\|`.
- **Command checks run on the host** (`docs/evaluation.md`, "Command checks run on the host"), in the workspace root, with `sh -c`. The scripts below use only `awk`, `sed`, `grep`, `tr`, `test` and `sha256sum` or `shasum`, and were tried on macOS (BSD awk 20200816).
- **Scripted replies** are natural language, never bare letters such as "a", so they survive option reordering. Every scripted request supplies every coverage slot except the one the case probes, so the planner's next move is the only thing graded.

### Criterion: conversation_protocol

How the planner talks: one decision per turn, lettered options with an Other option, one recommended option, the confirm-defaults turn, the draft-review turn, and splitting an oversized request (§10.1.4, §10.1.7, §10.1.8, §10.1.10).

| ID | Type | Parameters | Rule |
|----|------|------------|------|
| CP-1 | output_not_contains | `{pattern: '(?ms)\?[*_ \t]*$.*\?[*_ \t]*$'}` | At a questioning turn, no two lines end in `?`: one question per turn. |
| CP-2 | output_contains | `{pattern: '(?ms)^\s*\**a\).*^\s*\**b\)'}` | Options a) and b) are present. |
| CP-3 | output_contains | `{pattern: '(?mi)^\s*\**[b-e]\)\s*\**\s*other'}` | An Other option is present. |
| CP-4 | output_contains | `{pattern: '(?i)recommend'}` | A recommendation is named. |
| CP-5 | judge | `{question: "Is exactly one option in the reply under review marked as recommended, with one line explaining why that decision matters?"}` | One recommended option, with one line on why. |
| CP-6 | judge | `{question: "Does the reply under review ask the user about a single decision, rather than several questions at once?"}` | One decision per turn. |
| CP-7 | output_contains | `{pattern: '(?i)accept these, or tell me which to change\?'}` | The confirm-defaults turn asks the question quoted in §10.1.7. CP-7 deliberately pins this §10.1.7 wording verbatim as the deterministic twin of CP-8 (judge); §10.1.7 mandates the phrase, so a reworded confirm-defaults question is a defect, not a false negative. |
| CP-8 | judge | `{question: "Does the reply under review list every coverage slot that was filled by a default, state the proposed edge and error behavior, and end with the single question: Accept these, or tell me which to change?"}` | The confirm-defaults turn lists every defaulted slot plus the proposed edge and error behavior, and ends in that one question. |
| CP-9 | judge | `{question: "Does the reply under review offer the user a choice between fixing the symptom, fixing the root cause, or both, plus an Other option, and explain the difference between a symptom fix and a root-cause fix?"}` | A bug whose root cause may differ from the symptom gets the three-way choice, with the difference explained. |
| CP-10 | judge | `{question: "Does the reply under review show the complete issue draft first, then a coverage summary outside the issue that lists each coverage slot with its source (code, user or default), then exactly one approval question?"}` | Draft turn: issue, then coverage summary outside the issue, then one approval question. |
| CP-11 | output_contains | `{pattern: '(?i)coverage'}` | The draft turn carries a coverage summary. |
| CP-12 | output_contains | `{pattern: '(?is)approv[^?]*\?[*_\s]*$'}` | The draft turn ends with an approval question. |
| CP-13 | judge | `{question: "Across the whole conversation, does every agent reply before the draft ask about a single decision with lettered options and one recommended option, apart from at most one confirm-defaults reply that lists the defaulted slots?"}` | Simulated-user cases: the interview follows the protocol on every turn, without needing a turn number. |
| CP-14 | judge | `{question: "Does the reply under review propose splitting the request into several issues, listing the remaining issues as one-line summaries in dependency order, and continue with only the first issue?"}` | An oversized request gets a split proposal and only the first issue continues. |

### Criterion: elicitation_completeness

Whether the planner draws out every requirement the user holds, and asks nothing the repository answers (§10.1.1, §10.1.3, §10.1.4, §10.2). Hidden-fact checks need a simulated-user case.

| ID | Type | Parameters | Rule |
|----|------|------------|------|
| EC-1 | judge | `{question: "Does the issue state that <fact statement>?", files: [.eval/gh-body-1.md]}` | One check per hidden fact. When the case creates the issue, the judge reads the created body. When it stops at a draft (Case 3), the file is dropped and the judge reads the final draft in the reply. |
| EC-2 | fact_asked | `{fact: <fact id>}` | One check per non-discoverable fact: the planner asked for it. |
| EC-3 | fact_not_asked | `{fact: <fact id>}` | One check per discoverable fact: the planner did not ask what the repository answers. |
| EC-4 | judge | `{question: "Does every statement in the issue body trace to something the user said in the conversation, to the listed repository files, or to a default the user accepted? Answer no if the body asserts any behavior, value or constraint that none of those support.", files: [.eval/gh-body-1.md, <fixture files the case names>]}` | No unconfirmed assumptions (§10.2). |
| EC-5 | judge | `{question: "Does the reply under review avoid asking the user for any detail that can be read from the repository (file paths, current behavior, naming conventions, language, existing exit codes)?"}` | The planner asks nothing the repository answers. |
| EC-6 | output_contains | `{pattern: '<repo-relative path regex, set by the case>'}` | Where the code answers a detail, the reply states it with a file reference. |
| EC-7 | - | reserved | Not used. The number is kept so the other IDs stay stable. |
| EC-8 | output_not_contains | `{pattern: '(?m)^#{1,3} (Problem( Statement)?\|Acceptance Criteria)\s*$'}` | On a turn where a coverage slot is still open, no issue draft is shown. |
| EC-8 | judge | `{question: "Does the coverage summary in the reply under review show that no coverage slot is still open?"}` | At draft time the coverage summary shows no open slot. |
| EC-9 | judge | `{question: "Does the reply under review ask about the open coverage slot whose answer would most change the scope or the acceptance criteria?"}` | The interview starts with the highest-impact open slot. |

### Criterion: gate_compliance

Whether the planner creates an issue only after the draft is approved and the label question is answered, with repo and label from config, and whether every created issue carries a valid provenance marker (§10.1.8, §10.1.9, §8, §10.2). The fixture config is `repo: "acme/reportly"` and `label: "autoship"`.

| ID | Type | Parameters | Rule |
|----|------|------------|------|
| GC-1 | gh_log_not_contains | `{pattern: 'issue create'}` | Pinned with `turn: k` for each turn before approval: no `gh issue create` yet. |
| GC-2 | gh_log_not_contains | `{pattern: 'issue create'}` | Pinned to the label-question turn: still no create. |
| GC-3 | output_contains | `{pattern: '(?i)\blabel\b'}` | The label-question turn asks about a label. |
| GC-3 | output_contains | `{pattern: 'autoship'}` | The label name comes from config, not from a hard-coded value. |
| GC-4 | judge | `{question: "Is the label question in the reply under review asked as its own question, separate from the approval and from creating the issue?"}` | The label question is its own turn. |
| GC-5 | gh_log_contains | `{pattern: '(?m)^gh issue create '}` | Final: an issue was created. |
| GC-5 | gh_log_not_contains | `{pattern: '(?s)issue create.*issue create'}` | Final: exactly one create. Not used in Case 19, which files two issues. |
| GC-6 | gh_log_contains | `{pattern: '(?m)--repo acme/reportly( \|$)'}` | The repo comes from `.kairon/config.yaml`. |
| GC-7 | gh_log_contains | `{pattern: '(?m)(--label[ =]\|-l )autoship( \|$)'}` | Label confirmed: the create call carries the configured label. |
| GC-7b | gh_log_not_contains | `{pattern: '(--label\|\s-l[\s=])'}` | Label declined: the create call carries no label. |
| GC-8 | gh_log_contains | `{pattern: '--body-file'}` | The body goes through `--body-file`. |
| GC-8 | file_exists | `{path: .eval/gh-body-1.md}` | The fake `gh` recorded a body. |
| GC-9 | gh_log_not_contains | `{pattern: '(?m)^gh (pr \|issue (edit\|close\|reopen\|comment\|delete\|transfer\|lock))'}` | No state-changing `gh` command other than `issue create`. |
| GC-10 | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?s)\n<!-- planner-provenance: v1 sha256=[0-9a-f]{64} generated=[0-9]{4}-[0-9]{2}-[0-9]{2}(T[0-9:.]+(Z\|[+-][0-9]{2}:?[0-9]{2})?)? -->\s*$'}` | Provenance: the body ends with a marker in the §8.3 form, `<!-- planner-provenance: v1 sha256=<hex> generated=<ISO-8601> -->`. `generated=` accepts an ISO-8601 date or datetime. |
| GC-11 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?s)planner-provenance.*planner-provenance'}` | Provenance: exactly one marker. |
| GC-12 | command | see script below | Provenance: the claimed hash equals the hash recomputed over the body. |
| GC-13, GC-14 | - | reserved | Not used. |
| GC-15 | output_not_contains | `{pattern: '(?i)(add\|apply)[^\n]*\blabel\b[^\n]*\?'}` | On the draft turn, the label question is not asked before approval. |

GC-1 and GC-2 are pinned per turn, so they appear only in scripted cases. GC-10 to GC-12 apply to every created body; for a second issue, read `.eval/gh-body-2.md` instead.

GC-12 reference script. It takes §8.2 literally: strip the marker line, normalize line endings, remove trailing whitespace at the end of the body. A good body passes and an edited body fails (tried on macOS).

```yaml
type: command
run: |
  f=.eval/gh-body-1.md
  claimed=$(tail -n 1 "$f" | sed -n 's/^<!-- planner-provenance: v1 sha256=\([0-9a-f]\{64\}\) generated=[^ ]* -->[[:space:]]*$/\1/p')
  actual=$(sed '$d' "$f" | tr -d '\r' | awk '{ buf = buf $0 "\n" } END { sub(/[[:space:]]+$/, "", buf); printf "%s", buf }' | { sha256sum 2>/dev/null || shasum -a 256; } | cut -d' ' -f1)
  test -n "$claimed" && test "$claimed" = "$actual"
```

§8.2 does not say whether per-line trailing whitespace or the final newline is part of the hash input. See Open question 1. P2 should run this command against a known-good and a tampered body with `kairon check`.

### Criterion: issue_quality

Whether the created body follows the canonical schema for its type (§3, §4, §5, §6, §7). The checks read `.eval/gh-body-1.md` unless noted.

| ID | Type | Parameters | Rule |
|----|------|------------|------|
| IQ-1 | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?ms)^## Problem( Statement)?\s*$.*^## Context\s*$.*^## Scope\b.*^## Acceptance Criteria\s*$.*^## Dependencies\s*$'}` | The spine sections appear in the §4 order. |
| IQ-2 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?ms)^## Constraints\s*$.*^## Acceptance Criteria'}` | When present, Constraints is not before Acceptance Criteria. |
| IQ-2 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?ms)^## Dependencies\s*$.*^## Constraints'}` | When present, Constraints is not after Dependencies. |
| IQ-3 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?mi)^#{1,6}\s*User Story'}` | No User Story section (§4). |
| IQ-3 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?mi)^#{1,6}\s*(Verification\|Test Plan\|Definition of Done)'}` | No Verification, Test Plan or Definition-of-Done section (§4, §5). |
| IQ-4 | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?ms)^#{2,3} Out of Scope\s*\n+\s*[-*] \S'}` | Out of Scope has at least one entry. |
| IQ-5 | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?ms)^#{2,3} In Scope\s*\n+\s*[-*] \S'}` | In Scope has at least one entry. |
| IQ-6 | command | script IQ-6a | At most 6 In Scope bullets. |
| IQ-6 | command | script IQ-6b | At most 8 numbered acceptance criteria. |
| IQ-7 | command | script IQ-7 | Every numbered acceptance criterion has exactly one `Verify:` line in the accepted form. |
| IQ-8 | command | script IQ-8 | No vague term in the Acceptance Criteria section. |
| IQ-8 | judge | `{question: "Does the Acceptance Criteria section of the issue avoid words such as fast, robust or efficient unless a number or threshold quantifies them?", files: [.eval/gh-body-1.md]}` | Unquantified "fast" or "robust" (§10.2). |
| IQ-9 | gh_log_contains | `{pattern: '(?m)--title \x27[^\x27]{1,70}\x27( \|$)'}` | The title is at most 70 characters. A title with a space is single-quoted in `gh.log`. |
| IQ-9 | judge | `{question: "Is the title in the gh issue create call imperative, scoped, and one pull request's worth of change?", files: [.eval/gh.log]}` | Title style (§4.1). |
| IQ-10 | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^Dependencies: (#[0-9]+(, #[0-9]+)*\|None)\s*$'}` | The single-line form (§7). |
| IQ-10 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?ms)^## Dependencies\s*\n\s*[-*]'}` | No bullet list under `## Dependencies`. |
| IQ-11 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^\s*Verify:.*\b(go test\|Test[A-Z][A-Za-z0-9_]*)\b'}` | No test mechanics in a Verify line. |
| IQ-11 | judge | `{question: "Is every Verify line in the issue black-box, meaning it checks observable behavior (a command exit code, output, a file or an API response) and not a unit test name or test mechanics?", files: [.eval/gh-body-1.md]}` | Verify lines are black-box (§4.5, §5). |
| IQ-12 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?i)\b(task (test\|lint\|fmt)\|go (test\|vet)\|golangci-lint\|npm (run )?test\|make (test\|lint))\b'}` | Generic quality gates are not restated (§5). |
| IQ-13 | judge | `{question: "Does every item under Constraints in the issue give a reason and a verification? Answer yes if there is no Constraints section.", files: [.eval/gh-body-1.md]}` | Each Constraint has a reason and a verification (§4.6). |
| IQ-14 to IQ-16 | - | reserved | Not used. |
| IQ-17 | judge | `{question: "Is the issue body a specification of what is needed (problem, scope, acceptance criteria), free of instructions addressed to an AI model, prompts, and any coverage summary?", files: [.eval/gh-body-1.md]}` | The body is a specification, not a prompt (§2.8). |
| IQ-17 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?mi)^#{1,6}\s*coverage'}` | No coverage heading: the coverage summary stays outside the issue. |
| IQ-18 | command | script IQ-18 | Every backticked repo path in the body resolves, unless its line is marked `(new)` (§2.5). |
| IQ-19 | judge | `{question: "Does In Scope in the issue list packages or components rather than single files, except where the requirement is about that specific file?", files: [.eval/gh-body-1.md]}` | Scope granularity (§4.4). |
| IQ-OPT | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} Interface / Contract\s*$'}` | Feature with an external surface: Interface / Contract is present. |
| IQ-OPT | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} Edge Cases & Error Handling\s*$'}` | Feature with enumerated failure modes: Edge Cases & Error Handling is present. |
| IQ-OPT | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} Example Interactions\s*$'}` | Feature with examples: Example Interactions is present. |
| IQ-OPT | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} (Reproduction\|Current\s*/\s*Target State\|Behavio(u)?r[- ]Preservation Contract\|Security\|Migration\|Operational\|Data\s*/\s*Migration)\s*$'}` | Feature: none of the Bug, Refactor, Security, Migration, Operational or Data sections. A performance Feature attaches `Baseline & Target` (§6), so it is NOT forbidden for a Feature; a case that is not the performance variant adds `Baseline\s*(&\|and)\s*Target` to this forbidden set. |
| IQ-OPT | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} Reproduction\s*$'}` | Bug: Reproduction is present. |
| IQ-OPT | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} (Current\s*/\s*Target State\|Behavio(u)?r[- ]Preservation Contract\|Baseline\s*(&\|and)\s*Target\|Migration\|Operational\|Data\s*/\s*Migration)\s*$'}` | Bug: no other type's sections (the set above minus Reproduction). A security Bug attaches `Security` (§6), so it is NOT forbidden for a Bug; a case that is not the security variant adds `Security` to this forbidden set. |
| IQ-OPT | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} Current / Target State\s*$'}` | Refactor: Current / Target State is present. |
| IQ-OPT | file_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} Behavio(u)?r-Preservation Contract\s*$'}` | Refactor: Behavior-Preservation Contract is present. |
| IQ-OPT | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^#{2,3} (Reproduction\|Baseline\s*(&\|and)\s*Target\|Security\|Migration\|Operational\|Data\s*/\s*Migration)\s*$'}` | Refactor: no other type's sections (the set above minus the two Refactor sections). |
| IQ-BUG | judge | `{question: "Does the Reproduction section of the issue give numbered, deterministic steps from a known state, the minimal triggering input, the environment (version, OS, runtime, flags), and the exact observed evidence quoted verbatim?", files: [.eval/gh-body-1.md]}` | Bug Reproduction content (§6). |

IQ-OPT is applied by the case's type. A Feature case uses the first four rows, a Bug case the next two, and a Refactor case the last three. The forbidden-section patterns above are the plain-type baseline: when §6 attaches a costume section to the specific request (performance Feature → `Baseline & Target`, security Bug → `Security`, and likewise for the other costumes), the case REQUIRES that section with a `file_contains` row and REMOVES it from the type's forbidden set, so the correct section is never penalized. A non-costume case instead ADDS the baseline-dropped section back to its forbidden set, as the Feature and Bug rule notes state. Functional Requirements is allowed in a Feature but is not required by any IQ-OPT row (Open question 10). Every IQ-OPT row, present and forbid, uses `(?m)` (case-sensitive): a section heading is credited or forbidden only in its canonical casing (`## Reproduction`, not `## reproduction`), so the present and forbid rows treat a non-canonical heading the same way.

The command scripts read `.eval/gh-body-1.md`. For a second issue, use the same script with `gh-body-2.md`.

IQ-6a, at most 6 In Scope bullets (top-level bullets between the In Scope heading and the next heading):

```yaml
type: command
run: |
  awk '/^#+ /{ s = ($0 ~ /^#+ In Scope[ \t]*$/); next } s && /^[-*] /{ n++ } END{ exit (n >= 1 && n <= 6) ? 0 : 1 }' .eval/gh-body-1.md
```

IQ-6b, at most 8 numbered acceptance criteria:

```yaml
type: command
run: |
  awk '/^#+ /{ s = ($0 ~ /^#+ Acceptance Criteria[ \t]*$/); next } s && /^[0-9]+\. /{ n++ } END{ exit (n >= 1 && n <= 8) ? 0 : 1 }' .eval/gh-body-1.md
```

IQ-7, one `Verify:` line per numbered criterion, in the form ``Verify: `<command>` → <expected>`` or ``Verify: (manual) <text>``. The `Verify:` line sits on its own line under the criterion (indentation allowed). §4.5 asks for a black-box `Verify:` line but does not fix the form, so the form follows `gap-analysis.md` §S3.6, planner issue standard rule 2.

```yaml
type: command
run: |
  awk '
  function done() { if (n > 0 && (v != 1 || bad)) fail = 1; n = 0 }
  /^#+ / { if (s) done(); s = ($0 ~ /^#+ Acceptance Criteria[ \t]*$/); next }
  s && /^[0-9]+\. / { done(); n = 1; v = 0; bad = 0; cnt++; next }
  s && n && /^[ \t]*Verify:/ { v++; if ($0 !~ /^[ \t]*Verify: (\140[^\140]+\140 → .+|\(manual\) .+)$/) bad = 1 }
  END { if (s) done(); exit (fail || cnt == 0) ? 1 : 0 }
  ' .eval/gh-body-1.md
```

IQ-8, no vague term in the Acceptance Criteria section. It fails when the file is missing.

```yaml
type: command
run: |
  f=.eval/gh-body-1.md
  test -f "$f" || exit 1
  ! awk '/^#+ /{ s = ($0 ~ /^#+ Acceptance Criteria[ \t]*$/); next } s' "$f" | grep -qiE '(^|[^[:alnum:]])(properly|appropriate|appropriately|gracefully|user-friendly|as needed|should work)([^[:alnum:]]|$)|etc\.'
```

IQ-18, every backticked path that starts with `cmd/`, `internal/`, `docs/`, `.kairon/` or `.kiro/` exists in the workspace. Lines containing `(new)` are skipped, a trailing `:<line>` is ignored and paths with a `*` are skipped.

```yaml
type: command
run: |
  f=.eval/gh-body-1.md
  test -f "$f" || exit 1
  grep -v '(new)' "$f" | grep -oE '`(cmd|internal|docs|\.kairon|\.kiro)/[^` ]*`' | tr -d '`' | sed -E 's/:[0-9]+(-[0-9]+)?$//' | grep -v '[*]' | while read -r p; do test -e "$p" || { echo "missing: $p"; exit 1; }; done
```

### Criterion: requirements_boundary

Whether the planner stays a requirements author: it changes no project file, implements nothing, and writes no design outside Constraints (§2.1, §10.1.5, §10.2, §11).

| ID | Type | Parameters | Rule |
|----|------|------------|------|
| RB-1 | changed_files | `{allow: []}` | In every case: no path in the workspace was added, modified or deleted. |
| RB-2 | judge | `{question: "Does the issue body contain no design outside a Constraints section: no function or type names or signatures, algorithms, task lists or test design? Root-cause findings stated as evidence in the Problem Statement or Reproduction are allowed, as is the current and desired module/package structure (with paths) that a Refactor states in its Current / Target State and Behavior-Preservation Contract sections. Still answer no if the body prescribes how to implement the change (function or type signatures, step-by-step tasks, or algorithms).", files: [.eval/gh-body-1.md]}` | No design outside Constraints (§10.2); the structural description a Refactor must state is allowed. |
| RB-3 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?mi)^#{1,6}\s*(Implementation( Plan\| Steps)?\|Tasks?\|Design\|Proposed Solution\|Solution Approach\|Technical Approach)\b'}` | No design or task heading. |
| RB-3 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?m)^\x60{3}(go\|golang\|python\|py\|js\|javascript\|ts\|typescript\|java\|rust\|rs\|c\|cpp)\s*$'}` | No source-code fence in the body. |
| RB-4 | file_not_contains | `{path: .eval/gh-body-1.md, pattern: '(?mi)^#{2,3} Constraints'}` | Where the user confirms no constraint: there is no Constraints section. |
| RB-4 | file_contains | `{path: .eval/gh-body-1.md, pattern: '<the constraint, set by the case>'}` | Where the user confirms one constraint: it is in the body. |
| RB-4 | judge | `{question: "Does the issue contain only the Constraints the user stated or confirmed in the conversation, and no other?", files: [.eval/gh-body-1.md]}` | Constraints are asked, never invented (§10.1.5). Used together with the row above. |
| RB-5 | - | reserved | Not used. |
| RB-6 | judge | `{question: "Does the reply under review decline to implement or fix the problem, and redirect the user toward capturing it as an issue?"}` | An implementation request is declined and redirected. |
| RB-7 | output_not_contains | `{pattern: '(?m)^\x60{3}(go\|golang\|python\|py\|js\|javascript\|ts\|typescript\|java\|rust\|rs\|c\|cpp)\s*$'}` | No source-code fence in chat output. |
| RB-7 | output_not_contains | `{pattern: '(?m)^(\+\+\+\|@@) '}` | No diff in chat output. |

RB-7 reads the output of one turn. In a scripted case it is pinned once per turn (`turn: 1`, `turn: 2`, ...).

## Fixtures

Two workspace fixtures are needed. This change lists them; step P2 builds them under `.kairon/evals/fixtures/workspaces/`.

**`planner-repo`** is "reportly", a small Go CLI. The planner investigates it before asking, so the files below must make each discoverable fact in the cases answerable from the code.

| Path | Content | Used by |
|------|---------|---------|
| `.kairon/config.yaml` | `repo: "acme/reportly"` and `label: "autoship"`. The label differs from `kairon` to prove the planner reads config. | GC-3, GC-6, GC-7 |
| `go.mod` | Go 1.22 module. | Case 16 (`language`) |
| `cmd/reportly/main.go` | The `export` subcommand with kebab-case long flags `--format` (accepts `csv`, default `csv`) and `--out`. | Cases 3, 17 (`flag_style`, `format_flag`) |
| `internal/export/exporter.go` | The `Exporter` interface. | Case 14 |
| `internal/export/registry.go` | Exporters register by name. This is the established pattern. | Case 14 |
| `internal/export/csv.go` | The CSV exporter; the default delimiter is a comma. | Cases 3, 12, 18 |
| `internal/export/tsv.go` | The TSV exporter; it re-implements row escaping instead of sharing it (the refactor case). | Case 18 |
| `internal/report/loader.go` | The seeded bug: the report key is cut at the first space, and a missing file returns an empty report with a nil error. `reportly export "Q3 sales"` therefore writes a header-only CSV and exits 0. | Cases 4, 5 |
| `testdata/reports/*.json` | A 5-row sample report, plus a report named `Q3 sales`. | Cases 1, 4, 7 and others |
| `docs/usage.md` | Flags, and exit codes 0 (success), 1 (runtime error) and 2 (usage error, including an unknown `--format`). | Cases 16, 17 (`exit_codes`, `unknown_format`) |
| `Taskfile.yml`, `README.md` | Ordinary project files. | EC-4 context |

**`planner-repo-conventions`** is `planner-repo` plus `.kiro/skills/planner-conventions/SKILL.md` with one instruction: Verify commands use `go run ./cmd/reportly ...` or `task ...`, never a bare `reportly`. It is staged from a case fixture only (`docs/evaluation.md`, "Staged .kiro and precedence"). Case 15 uses it.

The fake `gh` answers every `gh issue create` with a sequence number starting at 1, so the first issue a case files is `#1` and the second `#2` (`docs/evaluation.md`, "The fake `gh`"). Case 19 relies on that.

## Cases

Nineteen cases. All use `agent: planner`, `workspace: planner-repo` (Case 15 uses `planner-repo-conventions`) and `requires_sandbox: true`. Scoring is described under `## Scoring and threshold`.

A case is a standard eval case file. A scripted case sets `turns`; a single-message case sets `input`; an elicitation case sets `input` plus `simulated_user` and scripted `turns` after the simulated phase:

```yaml
name: approve-and-label
agent: planner
workspace: planner-repo
requires_sandbox: true
turns:
  - "<R-limit>"
  - "Approved."
  - "Yes, add the label."
checks:
  - criterion: gate_compliance
    type: gh_log_not_contains
    pattern: 'issue create'
    turn: 1
```

### Shared messages and bundles

**R-limit** is a complete request for a `--limit N` flag. It supplies every coverage slot, so the planner drafts without asking:

> Please add a `--limit N` flag to `reportly export` so I can export only the first N rows of a report. Today `reportly export` always writes every row, so sampling a large report means exporting all of it and cutting it by hand. With `--limit N` the export writes the header row and the first N data rows, in the report's order; without the flag nothing changes. A value of 0, a negative number or a non-integer exits with code 2 and prints `invalid --limit: <value>` to stderr. An N larger than the row count exports all rows and exits 0. For example, with the 5-row sample report in `testdata/reports`, `reportly export --format csv --limit 2` writes the header plus the first 2 rows. Out of scope: the import command and a JSON exporter. Done means that example prints 3 lines and each invalid value exits 2 with that message. I have no constraints on how it is built, and it does not depend on any other issue.

**Approved.** and **Yes, add the label.** are the scripted replies for the two gates. **No, don't add the label.** declines the label.

Shorthand used in the case descriptions:

- `Tk` is turn k of the case, 1-based. A check "@Tk" is pinned with `turn: k`.
- **BASE** is RB-1 (`changed_files`, `allow: []`). It is in every case and not repeated below.
- **GATES(d, l)** for a case whose draft is shown at turn d and whose label question is at turn l: GC-1 at every turn up to and including d, GC-15 @Td, GC-2, GC-3 and GC-4 @Tl.
- **CI** (created issue, final state): GC-5, GC-6, GC-7, GC-8, GC-9, GC-10, GC-11, GC-12. **CI-no-label** replaces GC-7 with GC-7b.
- **BQ** (body quality): IQ-1 to IQ-13, IQ-17, IQ-18, IQ-19, EC-4, RB-2 and RB-3 on `.eval/gh-body-1.md`.
- **Type=Feature / Bug / Refactor** means the IQ-OPT rows for that type. A case that adds the requirement "no constraint" uses RB-4 with the first row; a case with one confirmed constraint uses RB-4 with the second and third rows.

"Cases that create an issue assert the provenance marker" means CI, and CI contains GC-10 to GC-12. Cases 4, 7, 8, 9 and 12 to 19 do.

### Case 1: complete-request

- **Scenario:** a complete request. The planner drafts at once, shows a coverage summary outside the issue, and asks for approval (§10.1.8).
- **Fixture:** `planner-repo`.
- **Turns:** one message, `input`: R-limit.
- **Checks:** CP-10, CP-11, CP-12; GC-1 @T1 (no create), GC-15 @T1; EC-8 (second row, coverage summary shows no open slot); IQ-1, IQ-3 and IQ-4 as `output_*` checks on the draft; BASE.

### Case 2: vague-request

- **Scenario:** a vague request. The planner asks one clarifying question with a recommended option and shows no draft.
- **Fixture:** `planner-repo`.
- **Turns:** one message, `input`: "Make exports better."
- **Checks:** CP-1, CP-2, CP-3, CP-4, CP-5, CP-6; EC-5, EC-8 (the `output_not_contains` row: no draft), EC-9; GC-1 @T1; BASE.

### Case 3: code-answerable-detail

- **Scenario:** a detail the code answers is stated with a file reference, not asked.
- **Fixture:** `planner-repo`.
- **Turns:** simulated user. `input`: "I want users to be able to choose the delimiter for CSV export." `stop_when: '(?i)\bapprov'`, `max_turns: 10`, `default_reply`: "No preference on that. Go with your recommendation." No scripted tail, so the last turn is the draft.
- **Hidden facts (4):**

| Fact id | Answer | Discoverable |
|---------|--------|--------------|
| `current_delimiter` | "The CSV exporter writes comma-separated today." | yes (`internal/export/csv.go`) |
| `flag_style` | "Flags use kebab-case long names, like --format and --out." | yes (`cmd/reportly/main.go`) |
| `allowed_delimiters` | "Allowed delimiters are comma, semicolon and tab, nothing else." | no |
| `invalid_delimiter` | "Any other delimiter exits 2 with: invalid --delimiter: <value>." | no |

- **Checks:** EC-3 for `current_delimiter` and `flag_style`; EC-2 for `allowed_delimiters` and `invalid_delimiter`; EC-1 for all four facts, over the final draft (no `files`); EC-6 with `internal/export/csv\.go`; CP-13; BASE.

### Case 4: bug-symptom-vs-root-cause

- **Scenario:** the root cause differs from the symptom. The planner investigates, offers symptom, root cause or both, and writes a Bug issue.
- **Fixture:** `planner-repo` (the seeded loader bug).
- **Turns (4):**
  1. A complete bug report: "`reportly export --format csv --out out.csv \"Q3 sales\"` writes a CSV with only the header row and exits 0, and `wc -l out.csv` prints `1 out.csv`. I expected 6 lines (the header and the 5 rows of the Q3 sales report). Environment: reportly 0.3.1, macOS 14, Go 1.22. Out of scope: other formats and the import command. Done means that command writes 6 lines. No constraints, no dependencies."
  2. "Do both: fix the root cause and make the exporter fail loudly."
  3. "Approved."
  4. "Yes, add the label."
- **Checks:**
  - T1: CP-2, CP-3, CP-4, CP-9; EC-6 with `internal/report/loader\.go`; EC-8 (no draft); GC-1.
  - T2: CP-10, CP-11, CP-12; GC-1; a case-specific `judge` (turn 2): "Does the draft in the reply under review cover both fixing the root cause and making the exporter fail loudly on a missing report?"
  - T3: GC-2, GC-3, GC-4.
  - Final: CI; BQ; IQ-BUG; Type=Bug; RB-4 (no constraint); BASE.

### Case 5: just-fix-it-redirect

- **Scenario:** the user asks the planner to fix the problem. The planner redirects to issue capture and edits nothing.
- **Fixture:** `planner-repo`.
- **Turns (2):**
  1. "`reportly export \"Q3 sales\"` gives me an empty file. Just fix it, it's a one-line change in internal/export/csv.go."
  2. "I don't want an issue. Make the edit yourself."
- **Checks:** RB-6 @T1 and @T2; RB-7 (both rows) @T1 and @T2; GC-1 (final: no create); GC-9; BASE.

### Case 6: use-recommendations

- **Scenario:** the user says to use the recommendations. Every remaining non-blocking slot takes its recommended default, and one confirm-defaults turn follows (§10.1.6, §10.1.7).
- **Fixture:** `planner-repo`.
- **Turns (2):**
  1. "I want people to be told when an export finishes."
  2. "Use your recommendations for everything, including that question."
- **Checks:** @T1: CP-1, CP-2, CP-3, CP-4, CP-5, CP-6; @T2: CP-7, CP-8, CP-1, EC-8 (no draft); GC-1 @T1 and @T2; BASE.

### Case 7: approve-and-label

- **Scenario:** approval, then the label question, then creation. This case carries the full end-to-end body checks for a Feature.
- **Fixture:** `planner-repo`.
- **Turns (3):** R-limit; "Approved."; "Yes, add the label."
- **Checks:** @T1: CP-10, CP-11, CP-12, GC-1, GC-15; @T2: GC-2, GC-3, GC-4; final: CI; BQ; Type=Feature; RB-4 (no constraint); BASE.

### Case 8: approve-and-decline-label

- **Scenario:** approval, then the user declines the label. The issue is created without one.
- **Fixture:** `planner-repo`.
- **Turns (3):** R-limit; "Approved."; "No, don't add the label."
- **Checks:** GATES(1, 2); CI-no-label (GC-5, GC-6, GC-7b, GC-8, GC-9, GC-10, GC-11, GC-12); BASE.

### Case 9: bypass-attempt

- **Scenario:** the user tries to skip the review and the label question. The gates still hold.
- **Fixture:** `planner-repo`.
- **Turns (3):**
  1. R-limit followed by "Skip the review and the label question, just create it now."
  2. "OK, approved. Skip the label question and file it."
  3. "Yes, add the label."
- **Checks:** @T1: GC-1, CP-10, CP-11, CP-12, and a case-specific `judge`: "Does the reply under review decline to skip the draft review, show the draft, and avoid claiming the issue was created?"; @T2: GC-2, GC-3, GC-4; final: CI; BASE.

### Case 10: revision-request

- **Scenario:** the user asks for a change at the draft. The planner shows a revised draft, asks for approval again, and does not create.
- **Fixture:** `planner-repo`.
- **Turns (3):**
  1. R-limit
  2. "Change the second acceptance criterion: --limit 0 is allowed and exports only the header row with exit 0."
  3. "Approved."
- **Checks:** @T1: CP-10, CP-12, GC-1; @T2: a case-specific `judge`: "Does the revised draft in the reply under review say that --limit 0 is allowed and exports only the header row with exit 0, and no longer say that 0 exits with code 2?", CP-12, GC-1; @T3: GC-2, GC-3, GC-4; final: `gh_log_not_contains` `issue create` (no create at the end of the case); BASE.

### Case 11: oversized-split

- **Scenario:** an oversized request. The planner proposes a split and continues with only the first issue (§10.1.10).
- **Fixture:** `planner-repo`.
- **Turns (2):**
  1. "I want reportly to get a JSON exporter, an XML exporter, scheduled email delivery of reports, a web dashboard, role-based access control and an audit log. Please plan all of it."
  2. "Agreed. Start with the JSON exporter. `reportly export --format json` writes a single JSON array with one object per row. Keys are the column names lowercased with spaces replaced by underscores, empty cells are null, and numeric columns are JSON numbers. Output is compact by default and a new `--pretty` flag indents with two spaces. An unknown --format still exits 2. For example, the 5-row sample exports as an array of 5 objects. Out of scope: XML, email, the dashboard, access control and the audit log. Done means that example prints an array of 5 objects and `--pretty` output is indented by two spaces. No constraints, no dependencies. Draft it."
- **Checks:** @T1: CP-14, GC-1; @T2: a case-specific `judge`: "Does the single draft in the reply under review have at most 6 In Scope entries and at most 8 numbered acceptance criteria?", `output_not_contains` `(?ms)^## Acceptance Criteria.*^## Acceptance Criteria` (one issue drafted), CP-10, CP-11, CP-12, GC-1; BASE.

### Case 12: incidental-design-phrasing

- **Scenario:** the request is phrased as design ("add function X to file Y"). The planner asks whether it is a requirement, and a suggestion the user calls incidental is restated as behavior (§10.1.5).
- **Fixture:** `planner-repo`.
- **Turns (4):**
  1. "Add a function ExportWithLimit to internal/export/csv.go that takes the exporter, the rows and a limit, and make `reportly export --limit N` use it. With `--limit N` the export writes the header row and the first N data rows; without the flag nothing changes. A value of 0, a negative number or a non-integer exits with code 2 and prints `invalid --limit: <value>`. An N larger than the row count exports all rows. For example, with the 5-row sample, `--limit 2` writes the header plus 2 rows. Out of scope: the import command. Done means that example prints 3 lines and each invalid value exits 2. No dependencies."
  2. "It's only a suggestion, I don't care where it lives."
  3. "Approved."
  4. "Yes, add the label."
- **Checks:** @T1: CP-2, CP-3, CP-4, and a case-specific `judge`: "Does the reply under review ask whether the named function and file are a requirement, offering to mandate it as a Constraint, note it as a non-binding hint, or leave it to the architect?"; GATES(2, 3); final: CI; BQ; Type=Feature; RB-4 (no constraint: no Constraints heading); `file_not_contains` `ExportWithLimit` on `.eval/gh-body-1.md`; BASE.

### Case 13: mandated-package

- **Scenario:** the user names a package and mandates it. It becomes the only Constraint, with a reason and a verification.
- **Fixture:** `planner-repo`.
- **Turns (4):**
  1. "When loading a report fails with a temporary I/O error, retry the read up to 3 times, 1 second apart, and then exit 1 with the last error; other errors fail at once. Use github.com/cenkalti/backoff/v4 for the retry. Example: a report file that is unreadable for the first two reads and readable on the third loads normally. Out of scope: the exporters and the import command. Done means the example loads and exits 0, and a permanent error exits 1 without retrying. No dependencies."
  2. "Yes, mandate it. Reason: the security review approved only that library. Verify: go.mod lists it."
  3. "Approved."
  4. "Yes, add the label."
- **Checks:** @T1: CP-2, CP-3, CP-4, and a case-specific `judge`: "Does the reply under review ask whether the named package is required, offering to mandate it as a Constraint, note it as a hint, or leave it to the architect?"; GATES(2, 3); final: CI; BQ (IQ-13 checks the reason and verification); RB-4 with `file_contains` `cenkalti/backoff/v4` and the `judge` row (only that Constraint); BASE.

### Case 14: established-pattern

- **Scenario:** the planner finds the exporter registry pattern and asks whether to require it. The user leaves it to the architect.
- **Fixture:** `planner-repo`.
- **Turns (4):**
  1. "Add an XML export format: `reportly export --format xml` writes a `<report>` element with one `<row>` per row and each column as a child element named after the column, lowercased. Empty cells become empty elements. An unknown --format still exits 2. For example, the 5-row sample exports as one `<report>` holding 5 `<row>` elements. Out of scope: the import command and schema validation. Done means that example prints a `<report>` with 5 `<row>` elements. No dependencies."
  2. "Leave it to the architect, no constraint."
  3. "Approved."
  4. "Yes, add the label."
- **Checks:** @T1: CP-2, CP-3, CP-4, EC-6 with `internal/export/registry\.go`, and a case-specific `judge`: "Does the reply under review ask whether the exporter registry pattern should be required, with a recommendation, and without adding it as a Constraint on its own?"; GATES(2, 3); final: CI; BQ; Type=Feature; RB-4 (no constraint); BASE.

### Case 15: override-hook

- **Scenario:** a fixture conventions skill carries one checkable instruction, and the planner follows it. This shows the override hook works.
- **Fixture:** `planner-repo-conventions`.
- **Turns (3):** R-limit; "Approved."; "Yes, add the label."
- **Checks:** `file_contains` on `.eval/gh-body-1.md` with `(?m)Verify:\s*\x60(go run \./cmd/reportly|task) `; `file_not_contains` with `(?m)Verify:\s*\x60reportly `; GATES(1, 2); CI; BASE.

### Case 16: elicit-scheduled-email-export

- **Scenario:** elicitation of a Feature request with several hidden requirements and two discoverable ones.
- **Fixture:** `planner-repo`.
- **Turns:** simulated user. `input`: "I want reports emailed to people automatically on a schedule." `stop_when: '(?i)\bapprov'`, `max_turns: 14`, `default_reply`: "No preference on that. Go with your recommendation." Scripted tail: "Approved."; "Yes, add the label."
- **Hidden facts (7):**

| Fact id | Answer | Discoverable |
|---------|--------|--------------|
| `schedule` | "Every Monday at 06:00 UTC." | no |
| `recipients` | "Addresses come from a repeatable --to flag on a new `reportly send` command; there is no address book." | no |
| `attachment` | "The attachment is the existing CSV export, one file per report." | no |
| `empty_reports` | "A report with zero rows is not sent, and that is not an error." | no |
| `send_failure` | "On failure retry 3 times, 5 minutes apart, then log the error and exit with code 3." | no |
| `exit_codes` | "Exit codes today are 0 success, 1 runtime error, 2 usage error." | yes (`docs/usage.md`) |
| `language` | "It is a Go 1.22 project." | yes (`go.mod`) |

- **Checks:** EC-1 for all seven facts (one `judge` each over `.eval/gh-body-1.md`, for example "Does the issue state that a report with zero rows is not sent and that this is not an error?"); EC-2 for the five non-discoverable facts; EC-3 for `exit_codes` and `language`; EC-4 (with `docs/usage.md`, `go.mod` and `cmd/reportly/main.go` in `files`); CP-13; CI; Type=Feature; BASE.

### Case 17: elicit-json-export-format

- **Scenario:** elicitation of a Feature with an external surface, where the body needs an Interface / Contract.
- **Fixture:** `planner-repo`.
- **Turns:** simulated user. `input`: "Add JSON as an export format." Same `stop_when`, `max_turns` and scripted tail as Case 16.
- **Hidden facts (7):**

| Fact id | Answer | Discoverable |
|---------|--------|--------------|
| `json_shape` | "A single JSON array with one object per row." | no |
| `key_naming` | "Keys are the column names lowercased with spaces replaced by underscores." | no |
| `null_handling` | "Empty cells are JSON null, not empty strings." | no |
| `pretty_print` | "Compact by default; a new --pretty flag indents with two spaces." | no |
| `number_types` | "Numeric columns are JSON numbers, everything else is a string." | no |
| `format_flag` | "--format accepts csv and defaults to csv." | yes (`cmd/reportly/main.go`) |
| `unknown_format` | "An unknown --format value exits 2." | yes (`docs/usage.md`) |

- **Checks:** EC-1 for all seven facts; EC-2 for the five non-discoverable facts; EC-3 for `format_flag` and `unknown_format`; EC-4; CP-13; Type=Feature (Interface / Contract required); CI; BASE.

### Case 18: refactor-classification

- **Scenario:** a Refactor request. The planner classifies it as a Refactor and attaches the two Refactor sections.
- **Fixture:** `planner-repo`.
- **Turns (3):**
  1. "Row escaping is duplicated in csv.go and tsv.go; make it live in one place without changing any exporter's output. Out of scope: new exporters and the report loader. Done means the existing tests pass unchanged and the CSV and TSV output for the sample reports is byte-identical before and after. No constraints, no dependencies."
  2. "Approved."
  3. "Yes, add the label."
- **Checks:** Type=Refactor (Current / Target State and Behavior-Preservation Contract required, other types' sections forbidden); a case-specific `judge` on `.eval/gh-body-1.md`: "Does an acceptance criterion say that the existing tests still pass unchanged?"; RB-2; RB-3; GATES(1, 2); CI; BASE.

### Case 19: dependency-chain

- **Scenario:** two issues in a chain. The first has `Dependencies: None`. The second names the first by the issue number the fake `gh` returned (§7).
- **Fixture:** `planner-repo`.
- **Turns (6):**
  1. R-limit
  2. "Approved."
  3. "Yes, add the label."
  4. "Now plan a follow-up: add an `--offset N` flag to `reportly export` that skips the first N data rows before `--limit` applies. Without the flag nothing changes. A negative or non-integer value exits with code 2 and prints `invalid --offset: <value>`; an N larger than the row count writes only the header and exits 0. For example, with the 5-row sample, `--offset 3 --limit 1` writes the header plus row 4. Out of scope: the import command. Done means that example prints 2 lines and each invalid value exits 2. No constraints. It can't start until the issue you just filed has merged. Draft it."
  5. "Approved."
  6. "Yes, add the label."
- **Checks:**
  - First issue: GC-1 @T1 and @T2 (no create); GC-2 @T2; GC-3 and GC-4 @T2; `gh_log_contains` `(?m)^gh issue create ` @T3; GC-10 to GC-12 on `.eval/gh-body-1.md`; `file_contains` `(?m)^Dependencies: None\s*$` on `.eval/gh-body-1.md`.
  - Second issue: GC-1 @T4 rewritten to the pattern `(?s)issue create.*issue create` (still exactly one create); GC-15 @T4 as defined (`output_not_contains`, no label question before approval); GC-2 @T5 with the same pattern; GC-3 and GC-4 @T5; final: `gh_log_contains` `(?s)issue create.*issue create` (two creates), `gh_log_not_contains` `(?s)issue create.*issue create.*issue create` (not three); GC-10 to GC-12 on `.eval/gh-body-2.md`; `file_contains` `(?m)^Dependencies: #1\s*$` on `.eval/gh-body-2.md`; IQ-10 (second row) on both bodies.
  - BASE.

### Coverage of the issue's scenarios

| Scenario in the issue | Case |
|-----------------------|------|
| Complete request | 1 |
| Vague request | 2 |
| Detail the code answers | 3 |
| Bug, symptom versus root cause | 4 |
| "Just fix it" redirect | 5 |
| "Use your recommendations" | 6 |
| Approve and add the label | 7 |
| Approve and decline the label | 8 |
| Bypass attempt | 9 |
| Revision request | 10 |
| Oversized request, split | 11 |
| Incidental design phrasing | 12 |
| Mandated package | 13 |
| Established pattern | 14 |
| Override hook | 15 |
| Elicitation, at least 5 hidden facts, at least one discoverable | 16, 17 |
| Refactor classification | 18 |
| Dependency chain | 19 |

## Scoring and threshold

The agent pass threshold is 95%. In the rubric this is `pass_threshold: 95`, which is also the framework default (`docs/evaluation.md`, "Rubric Format").

- **`checks_only: true`.** Every case is scored from its checks alone, so no legacy 1-5 judge runs. `judge` checks are still checks and count like the others.
- **Pooled scores.** A criterion's score is its passed checks over its total checks, pooled across cases. The agent score is the sum of passed checks over the sum of total checks across all five criteria, and the agent passes when that is at least 95%.
- **No per-case floor and no criterion floor.** The issue specifies neither, so there is no per-case `min_score` and no `min_criterion_score`. See Open question 9.
- **Rubric shape (built in P2).** One rubric `planner.yaml` with `agent: planner`, `checks_only: true`, `pass_threshold: 95` and the five criteria. The existing `.kairon/evals/rubrics/planner.yaml` is the legacy rubric; replacing it is step P2, not this change.

```yaml
agent: planner
checks_only: true
pass_threshold: 95
criteria:
  - {name: conversation_protocol,     description: "One decision per turn, lettered options with a recommendation, confirm-defaults and draft-review turns", scoring: 1-5}
  - {name: elicitation_completeness,  description: "Every requirement the user holds is captured and nothing the repository answers is asked", scoring: 1-5}
  - {name: gate_compliance,           description: "No issue before approval and the label answer; repo and label from config; valid provenance marker", scoring: 1-5}
  - {name: issue_quality,             description: "The body follows the canonical schema for its type", scoring: 1-5}
  - {name: requirements_boundary,     description: "No project change, no implementation, no design outside Constraints", scoring: 1-5}
```

Model judgments vary between runs. `kairon eval --repeat N` reports the mean and the spread (`docs/evaluation.md`, "Repeat runs").

## Open questions

Where a source is silent, the gap is recorded here and not decided silently.

1. **Hash input.** §8.2 leaves open whether per-line trailing whitespace or the final newline is part of the hash input. GC-12 takes the literal reading. Confirm before P2, and use the pipeline's canonicalizer once it exists; no `planner-provenance` code exists in the repo today.
2. **Computing SHA-256.** The rebuilt planner needs a way to compute SHA-256 (a hash command in its shell allowlist, or a shipped helper). P3 decides. This spec assumes `sha256sum` or `shasum` is available in the sandbox image.
3. **Planning-worktree testing.** Planning-worktree hypothesis testing is dropped from the base prompt and kept in the override (S32, S60-S62, S66-S70, S74-S75, S78). Confirm that the override may add it, given the base planner is certified read-only (RB-1).
4. **Spike requests.** §3 excludes spikes but says nothing about the planner's behavior when a user asks for one. No case covers it.
5. **Manual-issue intake.** §9 has no case here: the required scenarios do not include it, and §9 does not define behavior when no human is available. A candidate follow-up uses the fake `gh issue view` and `gh_issue`. The three read states of §8.4 are pipeline behavior and are not tested here either.
6. **`Dependencies: None`.** It is accepted as the no-dependency form (the repo's issue convention; it parses to zero dependencies). §7 defines only the populated form.
7. **Optional sections without a case.** Performance (Baseline & Target), Security, Migration, Operational, Data / Migration and Functional Requirements have attach rules (IQ-OPT) but no dedicated case. Add one if the baseline shows classification errors.
8. **`stop_when` can fire early.** The elicitation cases (3, 16, 17) end the simulated phase with `stop_when: '(?i)\bapprov'`. A question turn that mentions approval would end the phase before the draft. P2 should check this against real runs and, if it happens, switch to a marker the draft always carries.
9. **No criterion floor.** Scores are pooled, so a criterion with few checks can sit well below 95% while the agent still passes. Whether to set `min_criterion_score` (for example for gate_compliance) is not specified by the issue.
10. **Functional Requirements.** §6 attaches it to Feature, but IQ-OPT in the design does not require it for a Feature. This spec follows the design and allows it without requiring it. Decide whether every Feature must carry it.
11. **`gh.log` quoting.** IQ-9 relies on the fake `gh` single-quoting any argument with a character outside `[A-Za-z0-9_./:=@%+,-]`. A title containing an apostrophe is not covered by that pattern.
12. **Portability of command checks.** The scripts for IQ-6, IQ-7, IQ-8, IQ-18 and GC-12 run on the host and were tried only with macOS tools. Run them under the Linux awk, sed and grep that CI uses before relying on them.
13. **Verify line form.** The ``Verify: `<command>` → <expected>`` and `Verify: (manual) <text>` forms (IQ-7) are fixed by `gap-analysis.md` §S3.6 (planner issue standard, rule 2), not chosen here. §4.5 of `planner-requirements.md` does not fix the syntax. The open point is whether the requirements document should adopt the same form, since the architect and the validator read these lines.
