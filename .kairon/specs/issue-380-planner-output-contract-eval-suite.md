# Design Spec: Rebuild the planner eval spec and suite to certify the output-document contract

Closes #380

## 1. Problem and approach

The planner rewrite (#381) must be driven by Kairon's own eval framework: the failing suite exists first (RED),
then the prompt is rewritten to pass it (GREEN). Today there is no `.kairon/evals/cases/planner/` directory, and
`.kairon/evals/rubrics/planner.yaml` scores the **old** planner (`single_question_adherence`, `requirement_clarity`,
...). Nothing asserts the new contract in `.kairon/specs/maturity-model/planner-requirements.md`: a returned
stage-output document, two modes, and guard-committed provenance (§12).

**Approach.** Add planner content to the existing harness. No harness code changes.

1. Rewrite `rubrics/planner.yaml` as a `checks_only: true` rubric. Every non-cost criterion is scored from the
   case `checks`. Mechanical rules use deterministic checks (`output_contains`, `output_not_contains`,
   `gh_log_not_contains`, `fact_asked`). Only judgment-only properties use `judge` checks.
2. Add seven cases under `.kairon/evals/cases/planner/`, each with `agent: planner`. Each case also carries
   `stub.turns` holding a **reference (GREEN) transcript**, so the suite can be run offline with `--backend stub`.
3. Add synced fixture files (issue bodies, a large request) under `.kairon/evals/fixtures/`.
4. Add one Go test, `internal/eval/planner_suite_test.go`, that makes the RED/GREEN claim reproducible without a
   model: reference transcripts must score a pass and be valid per `internal/stageoutput`; **legacy transcripts**
   (what the current `planner-prompt.md` produces) must fail the output-document and two-mode criteria.
5. Mirror `.kairon/evals/` into `cmd/kairon/templates/kairon/evals/` and pass `task sync:check` for the evals tree.

**Registration.** `kairon eval planner` discovers an agent's suite by name: `rubrics/planner.yaml` plus every
`cases/planner/*.yaml`. Creating those files is the registration. The Go test asserts that
`ListAvailableTestCases("planner")` returns all seven case names, which is the guard against a case silently
dropping out.

**Why not assert on the real prompt in CI.** A real-model run is slow, costs money and is nondeterministic. The
RED proof against the unmodified prompt has two parts. The deterministic part is the legacy-transcript test above.
The real part is a manual `kairon eval --sandbox planner` run against the unmodified prompt, recorded in the PR
description (see §6).

### Decisions and trade-offs

| Decision | Choice | Reason |
|----------|--------|--------|
| Rubric scoring | `checks_only: true` | Issue AC 3 prefers deterministic assertions. `judge` checks are still checks, so judgment-only criteria stay allowed. |
| Thresholds | `pass_threshold: 90`, `min_criterion_score: 60` | Real runs vary. The floor stops a high total from hiding a dead criterion (for example output validity at 0%). Builder may tune after the real run, but must keep RED. |
| Case grounding | `setup` entries (`type: file` fixture files plus inline `type: text`) | `fixtures/workspaces/` is excluded from `sync:check`, so a case that names a `workspace:` would fail to load in projects created by `kairon init`. No shipped case uses `workspace:`. |
| `requires_sandbox` | **Not set** | The harness records a native/stub run of such a case as failed, which would break the offline stub proof. The manual real-model RED run must use `--sandbox` (fake `gh`), because the old prompt can call `gh issue create` (see risks). |
| `gh` log checks | Only `gh_log_not_contains 'issue create'` | This asserts the planner did **not** write to GitHub. No case asserts on the result of a `gh issue create`, which satisfies the issue constraint. It is vacuous natively and effective under `--sandbox`. |
| Document location in output | The document is the reply text. It may be wrapped in one ```` ```yaml ```` fence. | #376 defines the document but not its transport. Regexes tolerate an optional fence. |
| Mode signal | Automated cases open with an explicit preamble ("AUTOMATED INTAKE: non-interactive, no human is available ..."). Interactive cases use a plain user request. | Requirements do not say how the planner is told its mode. #381 owns that. **Assumption to confirm in #381:** if it picks a different signal, only the `input` preambles in the three automated cases change. |

## 2. Relevant files

Create:

- `.kairon/evals/cases/planner/interactive-single-decision.yaml`
- `.kairon/evals/cases/planner/interactive-option-rules.yaml`
- `.kairon/evals/cases/planner/interactive-return-document.yaml`
- `.kairon/evals/cases/planner/automated-best-effort-fill.yaml`
- `.kairon/evals/cases/planner/automated-punch-out.yaml`
- `.kairon/evals/cases/planner/automated-bug-classification.yaml`
- `.kairon/evals/cases/planner/multi-issue-split.yaml`
- `.kairon/evals/fixtures/planner-intake-feature-issue.md` (automated best-effort input)
- `.kairon/evals/fixtures/planner-intake-punchout-issue.md` (automated punch-out input)
- `.kairon/evals/fixtures/planner-intake-bug-issue.md` (automated bug input)
- `.kairon/evals/fixtures/planner-large-request.md` (split input)
- `.kairon/evals/fixtures/planner-repo-context.md` (repository excerpts the planner may "find": exporter conventions, config, a failing code path)
- `internal/eval/planner_suite_test.go`
- `internal/eval/testdata/planner-legacy/*.txt` (one legacy-behavior transcript per case; see `suite-proof-test`)
- `cmd/kairon/templates/kairon/evals/cases/planner/*.yaml` (copies)
- `cmd/kairon/templates/kairon/evals/fixtures/planner-*.md` (copies)

Modify:

- `.kairon/evals/rubrics/planner.yaml` and `cmd/kairon/templates/kairon/evals/rubrics/planner.yaml` (copy)
- `docs/evaluation.md` (short "Planner suite" section: run commands, RED/GREEN workflow, mode-preamble assumption)

Read-only inputs (do not edit):

- `.kairon/specs/maturity-model/planner-requirements.md` (§3.1.1, §3.5, §5-§7, §10, §10.3, §12)
- `.kiro/agents/planner-prompt.md` (**must not be rewritten**; #381 owns it)
- `docs/evaluation.md` (case/check/simulated-user schema), `docs/stage-output.md`, `internal/stageoutput/`
- `internal/watcher/dependencies.go` (what `Dependencies:` forms parse)
- `Taskfile.yml` (`sync:check`)

**Do not touch** the uncommitted modifications to `.kiro/agents/*.json` already in this worktree. They are not part
of this issue, and they make `task sync:check` fail on the agents tree before any change here. Verify the evals tree
with the scoped command in §6, and report the pre-existing agents-tree failure rather than "fixing" it.

## 3. Rubric (`.kairon/evals/rubrics/planner.yaml`)

```yaml
agent: planner
checks_only: true
pass_threshold: 90
min_criterion_score: 60
criteria: [...]   # names below; scoring: 1-5 (ignored for checked criteria); cost_efficiency has type: cost
```

Removed (they no longer apply or are folded in): `requirement_clarity`, `scope_appropriateness`,
`acceptance_criteria_quality`, `constraint_identification`, `question_clarity`, `single_question_adherence`.
No criterion may require the agent to write a provenance marker or run `gh issue create`.

| Criterion | What it certifies | Requirement |
|-----------|-------------------|-------------|
| `output_document_validity` | Reply is a stage-output document: `schema_version: 1`, `stage: planner`, `result`, `source.kind/ref`, `generated` (RFC 3339 UTC); `body` exactly when canonical, `unresolved` exactly when punch_out; nothing else in the reply | §3.1.1, §3.5 |
| `two_mode_behavior` | Interactive: asks, never returns a punch-out, no document before approval. Automated: never asks, never offers options, returns a document immediately | §10 |
| `conversation_protocol` | One decision per turn; investigate before asking (repository facts are not asked); a)/b)/c)/Other format | §10.1 steps 1, 4 |
| `option_rules` | Every option is a single commitment (*and* allowed, *or* forbidden); the recommendation names exactly one option, never conditional; no recommendation if none can be chosen | §10.1 step 4, §10.3 |
| `elicitation_completeness` | Coverage slots are filled or asked; defaults confirmed in one turn; escape hatch honored | §10.1 steps 3, 6, 7 |
| `best_effort_then_punch_out` | Automated: groundable slots are filled with recorded `inferences` (`slot`/`inferred`/`evidence`); an unresolvable required slot yields `result: punch_out` with a non-empty `unresolved` list of `{slot, reason}`, no `body`, no partial canonical | §10.2, §3.5 |
| `issue_quality` | Body follows the shared spine in order; classification fits; optional sections attach by type; each acceptance criterion has a `Verify:` line; single-line `Dependencies:`; no User Story; no Verification/Test Plan section; no vague terms | §5, §6, §6.1, §6.2, §7, §10.3 |
| `requirements_boundary` | No ungrounded assumption, no invented Constraint, no design outside Constraints, no re-deriving what the repo states | §10.3 |
| `gate_compliance` | **Reframed:** the planner returned a well-formed output document the guard could validate, and did not write to GitHub or write a provenance hash | §12, §8 |
| `dependency_split` | Oversized request is split; remaining issues are one-line summaries in dependency order; only the first is drafted | §10.1 step 10 |
| `cost_efficiency` | `type: cost` | harness |

## 4. Cases

All cases: `agent: planner`, a `description` naming the behavior and requirement section, `checks` with a `criterion`
on every check, and `stub.turns` (plus `stub.routes` for simulated-user cases) holding a **reference transcript** that
passes every check. A `stub.turns` document must also pass `stageoutput.Parse` and `Validate` (enforced by `suite-proof-test`).

### Starting-point patterns (builder refines; each must be proven on a passing and a failing sample in `suite-proof-test`)

Document body lines sit indented inside the YAML `body: |` block, so spine patterns use `(?mi)^\s*`.

| Rule | Check | Pattern |
|------|-------|---------|
| Reply starts with the document, nothing before it | `output_contains` | `\A\s*(\x60{3}ya?ml\s*\n)?schema_version: 1\s*\n` |
| Stage | `output_contains` | `(?m)^stage: planner\s*$` |
| Result variant | `output_contains` | `(?m)^result: (canonical\|punch_out)\s*$` |
| Source | `output_contains` | `(?m)^source:\s*\n\s+kind: (github_issue\|document)\s*\n\s+ref: \S+` |
| Timestamp | `output_contains` | `(?m)^generated: "?\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z"?\s*$` |
| Canonical has body, no `unresolved` | `output_contains` / `output_not_contains` | `(?m)^body: \|` / `(?m)^unresolved:` |
| Punch-out has `unresolved` entries, no body | `output_contains` / `output_not_contains` | `(?m)^unresolved:\s*\n\s+- slot: \S+\s*\n\s+reason: ` / `(?m)^body:` |
| Inferences recorded | `output_contains` | `(?m)^inferences:\s*\n\s+- slot: .+\n\s+inferred: .+\n\s+evidence: .+` |
| Spine order | `output_contains` | `(?is)Problem( Statement)?.*Context.*Scope.*Acceptance Criteria.*Dependencies` (headings only, anchored with `^\s*#{1,6}\s*`) |
| `Verify:` lines present | `output_contains` | `(?s)Verify:.*Verify:` (per-criterion coverage goes to a `judge` check) |
| No test plan section | `output_not_contains` | `(?mi)^\s*#{1,6}\s*(Verification\|Test Plan)\b` |
| No user story | `output_not_contains` | `(?mi)^\s*#{1,6}\s*User Story` |
| Dependencies is one line | `output_contains` | `(?m)^\s*Dependencies:\s*(#\d+(,\s*#\d+)*\|<no-dependency form the scheduler accepts>)\s*$` |
| No bullet list under a Dependencies header | `output_not_contains` | `(?mi)^\s*#{1,6}\s*Dependencies\s*\n(\s*\n)*\s*[-*]\s` |
| Banned vague terms | `output_not_contains` | `(?i)\b(properly\|appropriate(ly)?\|gracefully\|robust(ly)?\|as needed\|should work\|etc\.\|fast)\b` |
| One question per turn (`turn: N`) | `output_not_contains` | `(?s)\?[^?]*\?` |
| Options offered | `output_contains` | `(?m)^\s*a\)` and `(?m)^\s*b\)` |
| No disjunctive option | `output_not_contains` | `(?mi)^\s*[a-e]\)[^\n]*\bor\b` |
| Conditional recommendation | `output_not_contains` | `(?i)recommend[^\n]*\b[a-e]\)[^\n]*\b(or\|if\|else\|unless\|depending)\b` |
| A recommendation exists | `output_contains` | `(?i)recommend` |
| Automated mode never asks | `output_not_contains` | `(?m)\?\s*$` and `(?m)^\s*a\)` |
| No GitHub write | `output_not_contains` / `gh_log_not_contains` | `gh issue create` / `issue create` |
| No document before approval (`turn: 1`) | `output_not_contains` | `(?m)^schema_version:` |
| Interactive never punches out | `output_not_contains` | `(?m)^result: punch_out` |

The Dependencies "none" form: read `internal/watcher/dependencies.go` and the §7 text, then pick the form the
scheduler parses as zero dependencies. Do not invent a form the parser does not accept.

`judge` checks (one yes/no `question`, self-contained, since the judge sees neither `expected_output` nor `context`):
every acceptance criterion has a `Verify:` line; options in the turn are single commitments; exactly one
recommendation; each inference's evidence is a real quote or file reference from the provided materials; the split
list is in dependency order; no Constraint appears that the user did not state.

### Case list

| Case | Mode | Shape | Key assertions (criterion) |
|------|------|-------|----------------------------|
| `interactive-single-decision` | interactive | `simulated_user` (facts for format, scope, edge cases; `max_turns: 3`); input is a one-line feature request; `setup` text with repo context | turns 1 and 2: one `?`, a)/b) options, no document, no `gh issue create` (`conversation_protocol`, `two_mode_behavior`); `fact_asked` for an unstated requirement and `fact_not_asked` for a repo-answerable one (`elicitation_completeness`, `requirements_boundary`) |
| `interactive-option-rules` | interactive | single turn; a bug report whose root cause may differ from the symptom (§10.1 step 4 symptom / root cause / both) | options a)-c) plus Other; no disjunctive option; no conditional recommendation; a recommendation exists; judge: exactly one recommended option (`option_rules`) |
| `interactive-return-document` | interactive | `simulated_user`, `default_reply: "Use the recommended option for everything. Approved. Do not add the label."`, `stop_when: '(?m)^schema_version:'` | final turn: `result: canonical`, full envelope, spine, `Verify:`, single-line `Dependencies:`, no User Story / Test Plan, no vague terms, no `inferences` required, no `gh issue create` in output or gh log (`output_document_validity`, `issue_quality`, `gate_compliance`) |
| `automated-best-effort-fill` | automated | `input` has the automated preamble; `setup` files: the feature issue and the repo context; gaps are groundable from repo patterns | reply starts with the document; `result: canonical`; non-empty `inferences` with `slot`/`inferred`/`evidence`; no `?` line ends, no options; judge: evidence is real (`best_effort_then_punch_out`, `two_mode_behavior`) |
| `automated-punch-out` | automated | same preamble; issue states a goal with no measurable target and an external-contract decision the repo cannot settle | reply starts with the document; `result: punch_out`; non-empty `unresolved` with `slot` and `reason`; **no** `body:`; no `result: canonical`; no questions (`best_effort_then_punch_out`, `output_document_validity`) |
| `automated-bug-classification` | automated | bug issue with a reproduction in the issue text | `result: canonical`; body has a Reproduction section and current-vs-expected; no Test Plan section; `Verify:` lines; judge: classified as a bug, not a feature (`issue_quality`) |
| `multi-issue-split` | interactive | `simulated_user` with `default_reply: "Use the recommended option for everything. Approved. Do not add the label."`; `setup` file `planner-large-request.md` (more than 6 blocking questions, more than 8 criteria) | split proposed; remaining issues one-line; judge: list is in dependency order and only the first issue is drafted; first body has at most 6 In Scope entries; `Dependencies:` is one line (`dependency_split`, `issue_quality`) |

Notes for the builder:

- Stub transcripts must be realistic, not minimal. Each reference document must satisfy every check in its case and
  parse with `stageoutput.Parse` followed by `Validate`.
- Simulated-user `stub.routes` length must be at most `max_turns - 1`. `stub.turns` length must be between
  `1 + len(turns)` and `max_turns + len(turns)`. Read `docs/evaluation.md` "Load errors" before writing the YAML.
- `stop_when` on the document cases is the regex `(?m)^schema_version:`. Checks without `turn` then read the
  document turn, because the last simulated turn is the one that matched.
- Every criterion named in a check must exist in the rubric. A criterion with no checks in a case is simply not
  scored for that case under `checks_only`.

## 5. Team orchestration

```
planner-fixtures ─┬─> cases-interactive ─┐
                  ├─> cases-automated  ──┼─> suite-proof-test ─> sync-templates ─> docs-planner-suite
planner-rubric  ──┴─> case-split       ──┘
```

- `planner-fixtures` and `planner-rubric` start together (no shared files).
- `cases-interactive`, `cases-automated` and `case-split` run in parallel after both. They write disjoint files.
- `suite-proof-test` needs every case, because it loads the whole suite.
- `sync-templates` runs after it so the copy covers every file in its final state.
- `docs-planner-suite` is last because it documents the final commands.

Per `AGENTS.md`: the Go test in `suite-proof-test` is the test-first artifact for the harness-facing claims. Eval YAML and docs are
non-code and are exempt from test-first.

## 6. Validation

```bash
# Plan parses
go run ./cmd/kairon plan parse .kairon/specs/issue-380-planner-output-contract-eval-suite.md

# Suite loads and lists all seven cases
go run ./cmd/kairon eval --list planner

# GREEN (offline): reference transcripts pass
go run ./cmd/kairon eval --backend stub planner

# Deterministic RED/GREEN proof and document validity
go test ./internal/eval -run 'TestPlannerSuite' -count=1

# Template sync, evals tree only (the agents tree already differs for unrelated reasons)
diff -rq --exclude=results --exclude=.DS_Store --exclude=tmp --exclude=workspaces --exclude=hidden .kairon/evals/ cmd/kairon/templates/kairon/evals/

# Whole-repo gates
task test
task lint
```

**Manual real-model RED proof (AC 5), recorded in the PR description, not committed.** Run against the unmodified
prompt, in a sandbox so the old prompt's `gh issue create` hits the fake `gh`:

```bash
go run ./cmd/kairon eval --sandbox planner
```

Expected: the agent verdict is FAIL, with `output_document_validity` and `two_mode_behavior` at or near 0%. Do not
run this without `--sandbox`: the old prompt creates real GitHub issues.

## 7. Risks and open points

- **Mode signal is an assumption** (see Decisions). #381 may choose a different mechanism. Only the automated
  `input` preambles change.
- **Regex strictness vs. model variance.** The one-question regex `(?s)\?[^?]*\?` also fails a turn that quotes a
  `?` in a code sample. The `\bor\b` option regex rejects any option containing "or". Both are intentional
  (conservative); the judge checks catch what regexes cannot. Tune thresholds only after the real run, and keep RED.
- **Document transport.** #376 does not say how the document is wrapped in agent text. Regexes allow one optional
  `yaml` fence. If #381 chooses a different wrapper, update the start-anchor pattern only.
- **Existing `.kiro/agents/*.json` drift** makes `task sync:check` fail before this change. It is out of scope.
- **Results directories** (`.kairon/evals/results/`) are generated and excluded from sync. Do not commit run output.

## 8. Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "planner-fixtures"
    agent: "builder"
    description: "Create the synced planner eval fixture files under .kairon/evals/fixtures/: planner-intake-feature-issue.md, planner-intake-punchout-issue.md, planner-intake-bug-issue.md, planner-large-request.md and planner-repo-context.md. The feature issue has gaps that the repo context can ground. The punch-out issue states a goal with no measurable target plus an external-contract decision the repo cannot settle. The bug issue has a reproduction. The large request needs more than 6 blocking questions. Do not edit .kiro/agents/planner-prompt.md."
    dependencies: []
    acceptance_criteria:
      - "Five files exist at .kairon/evals/fixtures/planner-intake-feature-issue.md, planner-intake-punchout-issue.md, planner-intake-bug-issue.md, planner-large-request.md and planner-repo-context.md"
      - "planner-repo-context.md states at least one repository convention that answers a gap in the feature issue (for example empty input is a no-op), written as a quotable file-path-plus-line excerpt"
      - "planner-intake-punchout-issue.md contains no text that lets a competent engineer infer the measurable target or the external-contract answer from planner-repo-context.md"
      - "No fixture is placed under fixtures/workspaces/ or fixtures/hidden/"
      - ".kiro/agents/planner-prompt.md is unmodified (git diff is empty for that path)"
    validation_commands:
      - "test -f .kairon/evals/fixtures/planner-intake-feature-issue.md && test -f .kairon/evals/fixtures/planner-intake-punchout-issue.md && test -f .kairon/evals/fixtures/planner-intake-bug-issue.md && test -f .kairon/evals/fixtures/planner-large-request.md && test -f .kairon/evals/fixtures/planner-repo-context.md"
      - "git diff --quiet -- .kiro/agents/planner-prompt.md"

  - id: "planner-rubric"
    agent: "builder"
    description: "Rewrite .kairon/evals/rubrics/planner.yaml for the output-document contract as a checks_only rubric with pass_threshold 90 and min_criterion_score 60. Criteria: output_document_validity, two_mode_behavior, conversation_protocol, option_rules, elicitation_completeness, best_effort_then_punch_out, issue_quality, requirements_boundary, gate_compliance, dependency_split, and cost_efficiency (type: cost). Remove requirement_clarity, scope_appropriateness, acceptance_criteria_quality, constraint_identification, question_clarity and single_question_adherence. gate_compliance must read 'returned a well-formed output document the guard could validate', with no mention of writing a marker."
    dependencies: []
    acceptance_criteria:
      - "The rubric has agent: planner and checks_only: true and loads without error"
      - "The rubric names exactly these eleven criteria: output_document_validity, two_mode_behavior, conversation_protocol, option_rules, elicitation_completeness, best_effort_then_punch_out, issue_quality, requirements_boundary, gate_compliance, dependency_split, cost_efficiency"
      - "No criterion description requires the agent to write a provenance marker, a hash, or run gh issue create"
      - "None of the six removed criterion names remains"
    validation_commands:
      - "grep -c '^  - name:' .kairon/evals/rubrics/planner.yaml | grep -qx 11"
      - "grep -q 'checks_only: true' .kairon/evals/rubrics/planner.yaml"
      - "! grep -Eq 'single_question_adherence|requirement_clarity|scope_appropriateness|acceptance_criteria_quality|constraint_identification|question_clarity' .kairon/evals/rubrics/planner.yaml"
      - "! grep -Eqi 'write a (valid )?(provenance )?marker|writes? the hash' .kairon/evals/rubrics/planner.yaml"
      - "go build ./..."

  - id: "cases-interactive"
    agent: "builder"
    description: "Create the three interactive planner cases in .kairon/evals/cases/planner/: interactive-single-decision.yaml (simulated_user, one decision per turn, fact_asked/fact_not_asked), interactive-option-rules.yaml (single turn, symptom/root-cause/both options, no disjunctive option, no conditional recommendation, exactly one recommendation) and interactive-return-document.yaml (simulated_user with the use-the-recommendations escape hatch, stop_when on schema_version, returns a canonical document with the full spine). Follow the check patterns in section 4 of the spec. Each case has agent: planner, deterministic checks for every mechanical rule, judge checks only for judgment, and stub.turns (and stub.routes where simulated_user) holding a reference transcript that passes every check."
    dependencies: ["planner-fixtures", "planner-rubric"]
    acceptance_criteria:
      - "Three YAML files exist and each has agent: planner"
      - "Every check uses a criterion that exists in rubrics/planner.yaml"
      - "interactive-single-decision asserts, with a turn field, one question mark, a) and b) options, no schema_version and no gh issue create on turns 1 and 2"
      - "interactive-option-rules has output_not_contains checks for a disjunctive option (or inside an option line) and a conditional recommendation, an output_contains check for a recommendation, and one judge check for exactly one recommended option"
      - "interactive-return-document asserts result: canonical, schema_version: 1, stage: planner, source, generated, body, the spine order, Verify: lines, a single-line Dependencies: form, absence of User Story and Test Plan headings, absence of vague terms, and absence of gh issue create in output and gh log"
      - "No case asserts on the result of a gh issue create call; only the absence of the write is checked"
      - "Mechanically checkable rules use deterministic check types, not judge"
      - "The cases load under the harness and pass with --backend stub"
    validation_commands:
      - "test -f .kairon/evals/cases/planner/interactive-single-decision.yaml && test -f .kairon/evals/cases/planner/interactive-option-rules.yaml && test -f .kairon/evals/cases/planner/interactive-return-document.yaml"
      - "grep -L '^agent: planner' .kairon/evals/cases/planner/interactive-*.yaml | wc -l | grep -qx 0"
      - "go run ./cmd/kairon eval --backend stub planner --case interactive-single-decision"
      - "go run ./cmd/kairon eval --backend stub planner --case interactive-option-rules"
      - "go run ./cmd/kairon eval --backend stub planner --case interactive-return-document"

  - id: "cases-automated"
    agent: "builder"
    description: "Create the three automated-intake planner cases in .kairon/evals/cases/planner/: automated-best-effort-fill.yaml (expects result: canonical with a non-empty inferences list of slot/inferred/evidence, no questions), automated-punch-out.yaml (expects result: punch_out with a non-empty unresolved list of slot/reason and no body, no questions) and automated-bug-classification.yaml (expects a canonical bug body with a Reproduction section and current-vs-expected, no Test Plan). Each input starts with the explicit automated-intake preamble, and each uses setup file entries pointing at the planner-intake-* and planner-repo-context.md fixtures. Reply must begin with the document (start-anchor pattern). Each case has agent: planner, deterministic checks for mechanical rules, judge only for evidence-grounding and classification, and stub.turns holding a reference transcript."
    dependencies: ["planner-fixtures", "planner-rubric"]
    acceptance_criteria:
      - "Three YAML files exist and each has agent: planner"
      - "automated-best-effort-fill asserts result: canonical, an inferences block with slot, inferred and evidence, no line ending in a question mark, no a) option, and has one judge check that evidence is real"
      - "automated-punch-out asserts result: punch_out, an unresolved block with slot and reason, absence of a body: key, absence of result: canonical, and absence of questions"
      - "automated-bug-classification asserts a Reproduction heading, a current-versus-expected statement, and absence of a Test Plan or Verification heading"
      - "Every automated case asserts the reply starts with schema_version: 1 and contains stage: planner, source and generated"
      - "No case asserts on a GitHub side effect result"
      - "The cases load under the harness and pass with --backend stub"
    validation_commands:
      - "test -f .kairon/evals/cases/planner/automated-best-effort-fill.yaml && test -f .kairon/evals/cases/planner/automated-punch-out.yaml && test -f .kairon/evals/cases/planner/automated-bug-classification.yaml"
      - "grep -L '^agent: planner' .kairon/evals/cases/planner/automated-*.yaml | wc -l | grep -qx 0"
      - "go run ./cmd/kairon eval --backend stub planner --case automated-best-effort-fill"
      - "go run ./cmd/kairon eval --backend stub planner --case automated-punch-out"
      - "go run ./cmd/kairon eval --backend stub planner --case automated-bug-classification"

  - id: "case-split"
    agent: "builder"
    description: "Create .kairon/evals/cases/planner/multi-issue-split.yaml: a simulated_user interactive case whose setup file is planner-large-request.md and whose default_reply accepts recommendations and approves. Asserts a split is proposed with the remaining issues as one-line summaries, a judge check that the list is in dependency order and only the first issue is drafted, at most 6 In Scope entries in the first body, and a single-line Dependencies: form. Stub transcript holds a reference that passes."
    dependencies: ["planner-fixtures", "planner-rubric"]
    acceptance_criteria:
      - "The file exists with agent: planner and uses the dependency_split and issue_quality criteria"
      - "One judge check asks whether the remaining issues are listed in dependency order, and one asks whether only the first issue is drafted"
      - "A deterministic check asserts the Dependencies: line is single-line and no bullet list follows a Dependencies header"
      - "The case loads and passes with --backend stub"
    validation_commands:
      - "test -f .kairon/evals/cases/planner/multi-issue-split.yaml && grep -q '^agent: planner' .kairon/evals/cases/planner/multi-issue-split.yaml"
      - "go run ./cmd/kairon eval --backend stub planner --case multi-issue-split"

  - id: "suite-proof-test"
    agent: "builder"
    description: "Write internal/eval/planner_suite_test.go with tests named TestPlannerSuite*. (1) Registration: ListAvailableTestCases(planner) returns exactly the seven case names. (2) Every case file has agent: planner. (3) GREEN: copy the shipped rubric and cases into a temp evals dir, run with the stub backend and assert the agent verdict passes. (4) Document validity: every YAML document found in a stub.turns reply (strip one optional yaml fence) passes stageoutput.Parse and Validate; punch_out documents have non-empty unresolved and no body. (5) RED: add one legacy-behavior transcript per case under internal/eval/testdata/planner-legacy/<case>.txt reproducing what the current planner-prompt.md produces (a User Story draft, several questions in one reply, a gh issue create command, no output document, questions asked in automated mode). Run the same suite with those transcripts as stub.turns and assert the agent verdict FAILS and that output_document_validity and two_mode_behavior are below the 60 percent floor. (6) Regex proof: for every output_contains/output_not_contains pattern in the suite, the test compiles it with Go regexp. Follow existing helpers (writeProjectFile, the pinning_test.go and checks tests) for the temp project setup. Test-first: write the RED assertions and see them fail for the right reason before they pass."
    dependencies: ["cases-interactive", "cases-automated", "case-split"]
    acceptance_criteria:
      - "TestPlannerSuite tests exist in internal/eval/planner_suite_test.go and pass"
      - "The registration test fails if any of the seven cases is removed or renamed"
      - "The GREEN test passes using the shipped stub transcripts"
      - "The RED test passes only because the legacy transcripts fail: it asserts a failing agent verdict and output_document_validity and two_mode_behavior below 60 percent"
      - "Every document in the shipped stub transcripts parses and validates with internal/stageoutput"
      - "Legacy transcripts exist for all seven cases and none contains a stage-output document"
      - "The test does not read or write .kairon/evals/results/ of the repository and calls no model"
      - "The commit message of the work names the test seen failing first"
    validation_commands:
      - "go test ./internal/eval -run 'TestPlannerSuite' -count=1 -v"
      - "ls internal/eval/testdata/planner-legacy | wc -l | grep -qx 7"
      - "go vet ./internal/eval"
      - "go run ./cmd/kairon eval --backend stub planner"

  - id: "sync-templates"
    agent: "builder"
    description: "Mirror the evals tree into the templates: copy .kairon/evals/rubrics/planner.yaml, .kairon/evals/cases/planner/ and the five new fixtures planner-*.md into cmd/kairon/templates/kairon/evals/ using the sync commands in .kiro/skills/builder-conventions/SKILL.md. Do not copy fixtures/workspaces/, fixtures/hidden/ or results/. Do not modify .kiro/agents/*.json or their template copies; the agents tree already differs for unrelated, pre-existing reasons and must be reported, not fixed."
    dependencies: ["suite-proof-test"]
    acceptance_criteria:
      - "diff -rq with the sync:check exclusions between .kairon/evals/ and cmd/kairon/templates/kairon/evals/ reports no differences"
      - "cmd/kairon/templates/kairon/evals/cases/planner/ contains seven case files"
      - "No file under cmd/kairon/templates/kiro/agents/ or .kiro/agents/ is changed by this task"
      - "Any task sync:check failure remaining is limited to the pre-existing .kiro/agents tree and is reported in the sentinel"
    validation_commands:
      - "diff -rq --exclude=results --exclude=.DS_Store --exclude=tmp --exclude=workspaces --exclude=hidden .kairon/evals/ cmd/kairon/templates/kairon/evals/"
      - "ls cmd/kairon/templates/kairon/evals/cases/planner | wc -l | grep -qx 7"
      - "go build ./..."
      - "go test ./cmd/kairon/... -count=1"

  - id: "docs-planner-suite"
    agent: "builder"
    description: "Add a short 'Planner suite' section to docs/evaluation.md covering: what the suite certifies (output-document contract, two modes, hard bars), how to run it offline (kairon eval --backend stub planner) and for real (kairon eval --sandbox planner, and why --sandbox is required against an old prompt), the RED/GREEN workflow for #381, the automated-mode preamble assumption, and the TestPlannerSuite offline proof. Do not edit CHANGELOG.md, .kairon/specs/ other than this issue's spec, or any file named as historical in AGENTS.md."
    dependencies: ["sync-templates"]
    acceptance_criteria:
      - "docs/evaluation.md has a 'Planner suite' heading with the run commands and the --sandbox warning"
      - "The section names the seven case files and the criteria they feed"
      - "The section states that the suite is expected to FAIL against the pre-#381 planner prompt"
      - "No historical artifact listed in AGENTS.md is modified"
    validation_commands:
      - "grep -q 'Planner suite' docs/evaluation.md"
      - "grep -q 'eval --sandbox planner' docs/evaluation.md"
      - "git diff --quiet -- CHANGELOG.md .kiro/agents/planner-prompt.md"
      - "go test ./internal/eval -run 'TestPlannerSuite' -count=1"
      - "task lint"
```
