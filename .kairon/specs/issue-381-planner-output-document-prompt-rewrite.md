# Design Spec: Rewrite the planner prompt to return an output document, certified by its eval suite

Closes #381

## 1. Summary

The planner stops creating GitHub issues. Its result becomes a stage-output document (#376) that a deterministic
boundary validates (#377), hashes and commits (#378), or, for a punch-out, routes into the Tier 2 failure contract
(#379). The rewrite is the GREEN half of Eval-Driven Development: the suite from #380/#387 already exists and fails
against today's prompt, and the new prompt must pass it.

Three things have to land together, because the prompt alone cannot meet the acceptance criteria:

1. **The prompt and its skill** (`.kiro/agents/planner-prompt.md`, `.kiro/skills/planner-conventions/SKILL.md`).
2. **An output boundary in Go** that nothing calls today. #376-#379 built the parts (`stageoutput.Parse`, `guard.Run`,
   `provenance.CommitChain`, `failure.PunchOut`) but no code takes a planner reply and drives them. #378's docs
   say "#381 supplies the adapter", and `docs/provenance.md` says #381 defines the split-chain conventions. This spec
   adds `internal/planner` (the boundary), a `gh`-backed `IssueStore`, and a thin `kairon planner submit` command.
3. **A live eval run** that certifies the prompt. This cannot be done deterministically; see section 4.

### Decisions the builder must follow

| # | Decision | Reason |
|---|----------|--------|
| D1 | `#388` (parse-backed check type) is **out of scope**. | It declares `Dependencies: #381`, so the watcher will not start it until this issue closes. It edits every planner case, which the issue lists as out of scope (#380). Changing the suite during GREEN also breaks the `verify-log` rule that a `prompt` iteration keeps `suite_sha256` constant. The parser it needs already exists (`stageoutput.Parse`, #376). This issue closes the gap for its own evidence a different way: section 4.3. |
| D2 | `#389` (workspace fixture for the evidence judge) is **out of scope**. | It has `Dependencies: none`, so it can run in parallel. It changes case YAML and fixture staging (suite files). No acceptance criterion here needs it. Until it lands, the `best_effort_then_punch_out` evidence judge cannot catch a fabricated quote. Mitigation: the builder reads each `inferences[].evidence` in the live run against the fixtures by hand and records the result (task `live-eval-iterate`). |
| D3 | The suite (rubric, cases, fixtures) is **frozen** in this PR. | Certification must be by the #380 suite. If a check looks wrong, the builder does not edit it. See section 4.4. |
| D4 | The mode signal is the first line of the automated-intake preamble that the eval cases already use: `AUTOMATED INTAKE: non-interactive, no human is available to answer questions. Do not ask questions and do not offer options.` Absence of that line means interactive. | #380 left this open ("Assumption to confirm in #381") and changing it would mean editing three cases. The Go constant is the single source of truth and a test pins the eval inputs to it. |
| D5 | **The planner does not ask the label question.** The label is a parameter of the commit step (`kairon planner submit --label`, default off). | `planner-requirements.md` section 10.1 step 9 has the planner ask it. But the envelope is a closed schema (no `labels` key; adding one is a `schema_version` bump) and the planner must not apply labels, so an answer given in chat could never reach the boundary. Asking a question whose answer is discarded is worse than not asking. No eval check requires the label turn (verified with `grep`), so this does not affect certification. **Flag this in the PR for the maintainer**: it deviates from the literal text of step 9 and drops the old "Gate 2" turn. `.kairon/specs/maturity-model/` is historical (AGENTS.md) and is not edited. |
| D6 | The commit title is the first H1 line of `body`. The boundary removes that line (and one following blank line) before the guard chain and the hash. | `provenance.Request.Title` is required to create an issue and the envelope has no `title` key. The body is changed before admission, so the hash still covers exactly what is committed (requirements section 7). Missing H1 on the create path is a rejection. On the update path (automated) the existing GitHub title is kept and a missing H1 is tolerated. |
| D7 | Split chains use `#{issue-N}` placeholders. `N` is the 1-based position in the split list the planner proposed. The boundary gives the n-th document of a chain the key `issue-n`. | `provenance.SubstituteDependencies` already defines `#{key}`. The envelope has no key field, so the key has to come from position. The planner writes `Dependencies: #{issue-1}, #{issue-2}` for a later issue, and `#N` for an existing GitHub issue. After substitution the line is the single-line `Dependencies: #N, #M` form. |
| D8 | Automated intake emits **exactly one** document and never splits. An oversized request yields a `punch_out` with slot `scope.size`. | Requirements section 10.1 step 10 (split) is part of the interactive flow, and section 10.2 never mentions splitting. Choosing a split is a decision among materially different options, so automated mode may not make it. **No current eval case covers this rule**, so it is uncertified. Keep the prompt text for it to one or two sentences and say so in the PR. |
| D9 | `.kairon/iterations/planner/` is the "ITERATIONS.md" the issue asks for. The logs are `iteration-NN.md`, the format `kairon eval verify-log` checks. An `ITERATIONS.md` index file in that directory links them. | A free-form file would be unverifiable. The repo already has a verifier that rejects logs claiming scores nobody measured. |
| D10 | `planner.json` is **not modified**. | `git status` shows uncommitted, unrelated changes to every `.kiro/agents/*.json` in this worktree (an AIM/MCP credentials block, including an injection GUID). Editing `planner.json` risks committing them, and `cp .kiro/agents/*.json` into the templates would leak them. The "never touches GitHub" guarantee does not depend on tool config: provenance (section 8 of the requirements) already catches a body written by the planner, because it has no valid marker and classifies as `none` or `mismatch`. Optional hardening (a shell `deniedCommands` entry) is left as a follow-up. |

## 2. Findings from the codebase

- `.kiro/agents/planner-prompt.md` (and its identical template copy) is the legacy prompt: interactive only, a
  "User Story" in the draft, a "Gate 2" label question, and `gh issue create --body-file /tmp/issue-body.md` as
  Gate 3. Its only permitted shell commands are `gh issue create`, `cat` and reading the config. Root-cause and
  planning-worktree guidance in it is worth keeping (read-only investigation).
- `.kiro/skills/planner-conventions/SKILL.md` is loaded as a resource. It teaches a "root cause vs symptom" option
  block with Pros/Cons lines, "at least 3 root causes" and "tested in a planning worktree" checklists. Those conflict
  with the two option rules and with automated mode. It is a `*-conventions` skill, so it is **not** synced to the
  templates.
- The suite: `rubrics/planner.yaml` (`checks_only`, `pass_threshold: 90`, `min_criterion_score: 60`, eleven
  criteria) and seven cases. `internal/eval/planner_suite_test.go` already proves, offline, that the reference
  transcripts pass and legacy transcripts fail, and that every stub document parses and validates. Probed during this
  design: all five stub documents also pass `guard.Run(guard.DefaultChain(...))` with `ReadyToCommit`.
- `docs/evaluation.md` already has a "Planner suite" section that says the preamble is an "assumption to confirm in
  #381" and that the RED/GREEN flow uses `--prompt-file` candidates. Update, do not duplicate.
- `internal/provenance`: `CommitChain(ctx, items, admit)` keeps its `numbers` map local. Resuming a partly committed
  chain is explicitly "#381's concern", so this spec adds a seed (task `provenance-known-keys`).
  `Commit` needs a `Title` to create. `IssueStore` has no implementation; the package forbids `os/exec`.
- `internal/failure`: `Route.Terminal(remedy)` and `failure.PunchOut(iss, target, runID, fc, remedy)` exist.
  `failure.Runner` overwrites `Failure.Attempt` with its own counter, so a punch-out that must read `Attempt{1,1}`
  (requirements section 3.5) is sent with `PunchOut` directly, as the agent Manager does.
  `internal/github.CLI` implements `failure.Issues` (`State`, `PostComment`, `AddLabel`).
- `Dependencies: none` parses to zero dependencies (`internal/watcher/dependencies.go`), and
  `SubstituteDependencies` leaves a line without a placeholder unchanged. So `none` is the no-dependency form.
- Results under `.kairon/evals/results/` **are committed** (git tracks them). `verify-log` needs the referenced run
  directories, so the live runs must be committed too. `AGENTS.md` says results are generated and never hand-edited;
  committing a freshly generated run is not editing it.
- No planner results exist in `.kairon/evals/results/` yet (only architect and validator), and the suite did not
  exist before #387, so no run of the old prompt against any planner suite exists. R0 (section 4.2) has to be made.
- Local tooling in this environment: `kiro-cli`, `podman`, `docker`, `task` and Go 1.26 are installed. That does not
  prove `kiro-cli` is authenticated or that a sandbox image can be built; the preflight task checks.

## 3. Solution approach

```
 interactive:  human <-> kiro-cli chat --agent planner ---final reply (one document)---+
 automated:    caller builds AutomatedIntake prompt ---> planner ---final reply---------+
                                                                                       v
                                  internal/planner.Boundary.Handle  (deterministic Go, no agent)
                                    1 Extract: reply must be exactly one YAML document (optional ```yaml fence)
                                    2 mode contract: interactive never punch_out; automated: one doc, source matches issue
                                    3 guard.Run(DefaultChain) via failure.Route  --reject--> Outcome Rejected (Recoverable)
                                    4 result: punch_out -> failure.PunchOut(Attempt{1,1}, FailureContext from unresolved)
                                    5 result: canonical -> split H1 title, no marker in body,
                                                          provenance.CommitChainWith(known keys) -> hash -> Create/Update
```

### 3.1 The output contract (what the prompt must make the model produce)

The model's whole final reply is one stage-output document (`docs/stage-output.md`):

- `schema_version: 1`, `stage: planner`, `result: canonical | punch_out`, `source`, `generated` (RFC 3339 UTC,
  produced with `date -u +%Y-%m-%dT%H:%M:%SZ` so it is a real time), then `body` (canonical) or `unresolved`
  (punch-out). `inferences` is optional and only meaningful in automated mode.
- No text before or after it. No fence is required; one `yaml` fence is tolerated by the boundary and by the eval
  patterns. The prompt shows the unfenced form.
- `source`: interactive uses `kind: document`, `ref: "interactive-request"`; automated uses `kind: github_issue` and
  `ref` is the issue number as a string.
- The body never contains a `<!-- planner-provenance:` line, a hash, `gh issue create`, or a label.

### 3.2 Prompt content requirements

The builder writes the prompt freely and lets the suite drive it, but the finished prompt must contain each item
below. The right column is the rubric criterion that measures it.

| # | Requirement | Criterion |
|---|-------------|-----------|
| P1 | Role: produce a stage-output document; the final reply is that document only. Show one canonical and one punch-out example with the real field names. | `output_document_validity` |
| P2 | Restrictions: no project-file writes; no GitHub writes (`gh issue create/edit/comment/close`, any `gh pr`, `gh label`, `gh api` mutation); no provenance marker or hash; GitHub reads only with `gh issue view/list` and `gh pr view/list`; `write` only inside planning worktrees. Allowed shell: read-only inspection, `date -u`, `.kairon/scripts/planning-worktree-*.sh`, reading `.kairon/config.yaml`. Keep the "I am the planning agent" redirect when asked to implement. | `gate_compliance` |
| P3 | Mode selection by the first line (D4). | `two_mode_behavior` |
| P4 | Investigate first; anything the repository answers is stated with a `path:line` reference and never asked. Classify Feature / Bug / Refactor. Track the seven coverage slots. | `conversation_protocol`, `requirements_boundary` |
| P5 | Body standard in order: H1 title, `## Problem Statement`, `## Context`, `## Scope` with `### In Scope` and `### Out of Scope`, `## Acceptance Criteria` (numbered, each with its own `Verify:` line), optional `## Constraints`, `## Dependencies` holding one `Dependencies:` line. Optional sections by type. At most 6 In Scope entries and 8 criteria. No User Story, no Verification/Test Plan section, no vague terms (`properly`, `appropriate`, `gracefully`, `fast`, `robust`, `as needed`, `etc.`, `should work`). Bugs carry a Reproduction section and current-versus-expected. | `issue_quality` |
| P6 | Interactive flow, requirements section 10.1 steps 1-8: one decision per turn, `a)`/`b)`/`c)` plus `Other`, each option a single commitment (an `and`-conjunction is allowed, `or` is not), exactly one recommendation or none, never conditional. Constraints are asked, never invented. The "use the recommended option for everything" escape hatch. One turn to confirm defaults. Draft review shows the body, a coverage summary outside it, then one approval question. **No document before approval.** On approval the next reply is the document, nothing else (D5: no label question). | `conversation_protocol`, `option_rules`, `elicitation_completeness`, `two_mode_behavior` |
| P7 | Symptom / root cause / both choice for a bug whose root cause differs from the symptom (interactive). In automated mode, keep to what the issue asks, record the finding in Context as non-binding, and punch out only if the issue's ask and the root cause make the acceptance criteria materially different. | `option_rules`, `best_effort_then_punch_out` |
| P8 | Automated flow, requirements section 10.2: never ask, never offer options, never produce a draft for approval. Fill each open slot only from evidence actually present (issue text, repository conventions, related issues) and record `slot` / `inferred` / `evidence` with a quote or `path:line`. Return `canonical` when every required slot is filled, otherwise `punch_out` with one `{slot, reason}` per unresolvable slot, no `body`, no partial content. Carry over every dependency the original issue declares into the single `Dependencies:` line. One document only (D8). | `best_effort_then_punch_out`, `two_mode_behavior` |
| P9 | Split (interactive): propose a numbered list in dependency order with one-line summaries, continue with issue 1 only, one document per reply. For issue k the body uses `#{issue-j}` for each earlier issue j of the list it depends on (D7). The next user message starts issue k+1. | `dependency_split` |
| P10 | Hard bars from requirements section 10.3: no ungrounded assumption, no invented Constraint, no design outside Constraints. | `requirements_boundary` |

The skill is updated in the same pass: add a "Mode applicability" section (the decision framework and checklists are
interactive-only; in automated mode findings go to Context and `inferences`), restate the two option rules in the
option example, remove Pros/Cons from the options, and make the recommended option explicit.

### 3.3 The output boundary (`internal/planner`)

Pure Go behind interfaces, like `provenance`. A `deps_test.go` bans `os/exec`, `net/http` and `crypto/hmac` in this
package (same pattern as `internal/provenance/deps_test.go`). Proposed API (names may change; behavior may not):

```go
type Mode int // ModeInteractive, ModeAutomated

const AutomatedIntakeLine = "AUTOMATED INTAKE: non-interactive, no human is available to answer questions. Do not ask questions and do not offer options."
func AutomatedPrompt(repo string, issue int, title, body string) string // AutomatedIntakeLine + the rest of the preamble + the issue text

type Request struct {
    Mode             Mode
    Replies          []string       // final reply of each planner turn, dependency order; automated: exactly 1
    Attempt          guard.Attempt  // zero value means {1,1}
    Known            map[string]int // keys already committed in this split, e.g. "issue-1": 412
    Issue            int            // automated: the existing issue; interactive: 0 (create)
    ObservedRevision string         // automated: revision recorded when the planner read the issue
    Repo, FailLabel, RunID string   // automated punch-out target
}

type Outcome int // OutcomeCommitted, OutcomePunchedOut, OutcomePunchOutPending, OutcomeRoutedToIntake, OutcomeRejected
type Result struct {
    Outcome     Outcome
    Committed   []provenance.Committed
    Failure     *guard.FailureContext
    Disposition failure.Disposition // Recoverable for OutcomeRejected
}

type Boundary struct { Store provenance.IssueStore; Router provenance.Router; Issues failure.Issues; Now func() time.Time }
func (b *Boundary) Handle(ctx context.Context, req Request) (Result, error)
```

`Handle` order of work:

1. `Extract` each reply: trim; accept either the bare document or exactly one `yaml` fence around it; any other text
   before or after is a rejection (`Expected`: "the whole reply is one stage-output document").
2. Mode contract, each a rejection with `Guard: "planner-boundary"` and a filled `FailureContext`:
   interactive + `punch_out` (a human is present, so there is no issue to comment on); automated with a count of
   replies other than one; automated whose `source.kind` is not `github_issue` or whose `ref` is not `req.Issue`;
   a `body` containing `provenance.MarkerPrefix`.
3. `guard.Run(Input{Stage: planner, Raw, Attempt}, guard.DefaultChain(route.Recoverable()))` for every document.
   A rejection returns `OutcomeRejected` with the `FailureContext` and `Recoverable` (the caller's `failure.Runner`
   decides retry or punch-out; interactive callers print `Expected`/`Actual` and ask the planner to try again).
   Nothing is written.
4. `punch_out` (automated only): build the `FailureContext` required by requirements section 3.5 (`Stage` planner,
   `Guard` `planner-intake`, `Attempt{1,1}`, `Expected` "the required slots were fillable from the issue and the
   repository, directly or by grounded inference", `Actual` one `slot: reason` line per `unresolved` entry), then call
   `failure.PunchOut(b.Issues, Target{Repo, Issue, FailLabel}, RunID, fc, "")`. It posts the comment first and the
   label second. A write failure returns `OutcomePunchOutPending` plus the error.
5. `canonical`: assign keys `issue-(len(Known)+i+1)`, split the H1 title (D6), build `provenance.ChainItem`s with
   `Target{Number: Issue, ObservedRevision}` (zero `Issue` creates), and call `CommitChainWith(ctx, items, admit, Known)`
   where `admit` is `guard.Run` over the substituted text. A conflict maps to `OutcomeRoutedToIntake` or
   `OutcomePunchedOut` as `provenance` reports it.

The default `Router`: `RouteToIntake` logs and returns nil, because an issue edited under the planner has no valid
marker, so the next poll classifies it `none`/`mismatch` and plans it again. `PunchOut` calls `failure.PunchOut` with
an "edited while being planned" context.

### 3.4 Supporting pieces

- **`provenance.CommitChainWith(ctx, items, admit, known)`** seeds the number map. `CommitChain` calls it with nil.
  Keys in `known` may not be reused by an item (`ErrInvalid`, nothing written).
- **`internal/github.IssueStore`** implements `provenance.IssueStore` with `gh` (argument lists, never a shell; the
  body goes over stdin with `--body-file -`): `Revision` reads `updatedAt`, `Create` honors a `Labels` field set at
  construction, `Update` edits the body. Tests use the existing `ghFunc` seam.
- **`kairon planner submit`** (`cmd/kairon/cmd/planner.go`): reads one or more replies from `--file` or stdin,
  separated by a line containing only `---` (the boundary splits them; each is still parsed as one document),
  `--repo`, `--label` (default off), `--known issue-1=412` (repeatable), `--dry-run` (extract, mode contract and
  guard chain only, no writes). On success it prints `committed issue-1 -> #412` per document so a user can resume
  with `--known`. Exit 1 on rejection, printing `Expected`/`Actual`. Interactive mode only. Automated callers use the
  package directly. The watcher's call into the planner is **not** built here (out of scope; #382 is the consuming
  side).
- **TUI**: not changed. Interactive users now copy the document into `kairon planner submit`. Auto-submit from the
  planning tab is a follow-up; say so in `docs/planner.md`.

### 3.5 Multi-issue split and dependencies (AC 7)

The planner returns one document per reply, in list order, so a session produces N documents over N turns. The user
saves them (separated by `---`) and runs `submit` once, or runs it per document with `--known`. Either way the
documents go through `CommitChainWith`, which substitutes numbers before the guard and the hash, so every committed
`Dependencies:` line is `Dependencies: #12, #15`. The eval case `multi-issue-split` only observes the first document
(it stops on the first `schema_version:`), and that document uses `Dependencies: none`, which the existing patterns
accept. The `#{issue-N}` form appears only in later documents and is resolved by the boundary before anything is
committed.

## 4. What can and cannot be verified, and how

### 4.1 Deterministic (no model, runs in CI and for any agent)

| Evidence | Command |
|----------|---------|
| Boundary behavior incl. integration (AC 1, 5, 6, 7, same document type) | `go test ./internal/planner -count=1` |
| Chain seed | `go test ./internal/provenance -run 'TestCommitChainWith' -count=1` |
| `gh` store | `go test ./internal/github -run 'TestIssueStore' -count=1` |
| Suite itself still green/red as designed; stub documents pass the boundary; preamble constant matches the eval inputs; both modes produce one document type | `go test ./internal/eval -run 'TestPlannerSuite|TestPlannerBoundary' -count=1` |
| Offline reference pass | `go run ./cmd/kairon eval --backend stub planner` |
| The prompt has no GitHub write, carries the mode line, the contract keys and the placeholder form | `go test ./internal/planner -run 'TestPlannerPrompt' -count=1` |
| Log set is internally consistent with committed results | `go run ./cmd/kairon eval verify-log planner --require-cycle=false` |
| Template copy matches | `diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md` |
| Plan parses | `go run ./cmd/kairon plan parse .kairon/specs/issue-381-planner-output-document-prompt-rewrite.md` |

`TestPlannerPrompt*` is a regression guard for the original defect (the prompt instructing `gh issue create`). It is
**not** a certification of behavior and must stay small: forbidden strings (`gh issue create`, `--body-file`,
`/tmp/issue-body.md`) and required anchors (`AutomatedIntakeLine`, `schema_version: 1`, `result: punch_out`,
`unresolved`, `inferences`, `#{issue-`). It reads the live prompt, or the file named by `KAIRON_PLANNER_PROMPT` so a
candidate can be checked before it is promoted. It must fail against the legacy prompt.

### 4.2 Live (needs a model; this is what certifies AC 8)

`kairon eval --sandbox planner` makes real model calls: at least seven agent sessions (four are multi-turn), a
simulated-user call per turn, judge calls, all with `kiro-cli`. It is slow, costs money and is nondeterministic. It
needs `kiro-cli` authentication and a working Podman or Docker for `--sandbox`. A builder agent may be able to run
it on this host (the tools are installed) but the design must not assume so.

**Always `--sandbox`**, for every run. Without it the legacy prompt runs the real `gh issue create`. Also,
`verify-log` rejects a pair of runs whose sandbox mode differs, so all runs must use the same mode.

Runs and their order (the order matters because the live prompt and the skill are part of what is measured):

| Run | Prompt | When |
|-----|--------|------|
| R0 | legacy, on the **unmodified** tree | first, before any edit to the prompt or the skill |
| R1..Rn | candidates `.kairon/iterations/planner/vN.md` via `--prompt-file` | after the skill edit; the live prompt is untouched while iterating |
| Rg | the promoted live prompt | after copying the winner over `planner-prompt.md` |
| Rr | a tidied prompt (REFACTOR) | optional, score must not drop |

Use `--repeat 3` for the runs a `green`/`refactor` iteration cites, so "passes in every repeat" is meaningful (cost
is multiplied by three). `kairon eval` writes `.kairon/evals/results/<run>/`; commit the runs the logs reference.

**Fallback if the live run cannot happen.** Preflight (`kiro-cli --version`; a one-case run with
`kairon eval --sandbox planner interactive-option-rules`; `podman info` or `docker info`) decides. If it fails for
infrastructure reasons, the builder does not retry in a loop and does not write any iteration log, because
`verify-log` would reject a log without a committed run and a number nobody measured is fabrication. Instead it:
writes `LIVE-EVAL-BLOCKED: <reason>` into its sentinel, still delivers every deterministic item, promotes the best
candidate on the strength of the offline gates, and writes `.kairon/iterations/planner/ITERATIONS.md` with status
"pending: live runs not performed" and the exact commands for a maintainer. The PR description must say AC 8 is
**not** met. The validator accepts this path only if the sentinel shows the failing preflight output.

### 4.3 How D1 is handled without changing the suite

The regex envelope in the eval is weaker than the real parser (#388's point). This PR does not change that, but it
does not rely on it either:

- `TestPlannerBoundary...` parses and guards every stub document with the real code.
- After each live run the builder takes the final reply of every document-returning case from the result JSON and
  runs it through `kairon planner submit --dry-run`. A reply that the regexes pass and the parser rejects is a
  finding: record it in the iteration log and fix the prompt. This is evidence for this PR only; #388 still owns the
  permanent check.

### 4.4 If the suite seems wrong

If a failing check looks like a defect in a pattern rather than in the prompt, do **not** edit the rubric, cases or
fixtures (D3). Record the case, the check, and why in the iteration log and the sentinel, keep iterating on what the
prompt can fix, and list it for a follow-up. An edit to the suite would be a separate `change_type: eval` iteration
and needs a maintainer's instruction.

### 4.5 Shape of the iteration logs

`verify-log` runs with the default `--require-cycle` unless disabled, and that needs a `red` iteration: a change of
the suite with the prompt held constant. The suite change that made the planner suite fail was #387, a different PR,
and no planner suite existed before it, so a `red` log cannot be replayed honestly here. Therefore:

- `iteration-00.md`, phase `baseline`, `change_type: prompt`: `baseline_run` R0 (legacy prompt; this is the RED
  evidence, recorded in its Baseline section: verdict FAIL and the failing criteria) and `result_run` R1 (first
  candidate).
- `iteration-01.md` ... phase `green` once a candidate makes some check that failed in the baseline pass in every
  repeat with a score that does not drop; `change_type: prompt`. Add more `green` logs as needed.
- one final `refactor` log (`change_type: prompt`, score not lower) if a tidy-up is done.
- Verify with `kairon eval verify-log planner --require-cycle=false`, and state in `ITERATIONS.md` why the cycle
  rule is off (RED belongs to #387).

`chain`: each log's `baseline_run` must equal the previous log's `result_run`. Scores are percent with one decimal.
If the final promoted prompt is byte-identical to the last candidate, Rg can be that candidate's run only if the
recorded `prompt_sha256` allows; otherwise run once more and add it to the chain.

## 5. Acceptance criteria traceability

| AC | Deterministic evidence | Live evidence |
|----|------------------------|---------------|
| 1 document, no `gh issue create`, no hash | `TestBoundary` (marker in body rejected; `gh issue create` absent), `TestPlannerPrompt*`, `TestPlannerSuiteStubDocumentsAreValid` | `output_document_validity`, `gate_compliance`; section 4.3 |
| 2 single-commitment options | offline stub suite proves the checks can pass; prompt anchor | `option_rules`, `conversation_protocol` |
| 3 one concrete recommendation | same | `option_rules` |
| 4 automated never interviews, records evidence | stub suite | `best_effort_then_punch_out`, `two_mode_behavior` |
| 5 unresolvable slot punches out | `TestBoundaryAutomatedPunchOutRoutesToTier2` | `automated-punch-out` |
| 6 boundary routes document / punch-out | `TestBoundaryIntegration*` with fakes, plus one `failure.Runner` retry test | none needed |
| 7 split in dependency order, resolvable deps | `TestBoundaryChainResolvesDependencies` (the committed body's `Dependencies:` line parses with `watcher.DependencyParser` to the real numbers) | `multi-issue-split` (first document only) |
| 8 passes the suite, iterations recorded | `verify-log` on committed logs | `kairon eval --sandbox planner` passes; **cannot be asserted offline** |
| constraint: both modes, same document type | `TestPlannerBoundarySameDocumentType` | none |

## 6. Relevant files

Create:

- `internal/planner/{mode.go,extract.go,title.go,boundary.go}` and `*_test.go`, `deps_test.go`, `prompt_test.go`
- `internal/github/issuestore.go`, `internal/github/issuestore_test.go`
- `internal/eval/planner_boundary_test.go` (stub documents through the boundary, preamble drift, same document type)
- `cmd/kairon/cmd/planner.go`, `cmd/kairon/cmd/planner_test.go`
- `docs/planner.md`
- `.kairon/iterations/planner/{v1.md,...,iteration-00.md,...,ITERATIONS.md}`
- `.kairon/evals/results/<runs>/` (only if the live runs happen)

Modify:

- `.kiro/agents/planner-prompt.md` and `cmd/kairon/templates/kiro/agents/planner-prompt.md` (identical copy)
- `.kiro/skills/planner-conventions/SKILL.md` (not synced to templates)
- `internal/provenance/chain.go`, `chain_test.go`, `docs/provenance.md` (remove "#381 supplies the adapter" / "#381 has not defined" wording; link `docs/planner.md`)
- `docs/evaluation.md` (Planner suite section: preamble is now confirmed and pinned by a test; GREEN workflow; remove "expected to FAIL against the pre-#381 prompt" wording that becomes false)
- `README.md` (short pointer in the REPL/Architecture area: planner returns a document, `kairon planner submit`)

Do not modify:

- `.kairon/evals/{cases,rubrics,fixtures}/` (D3) and their template twins
- `.kiro/agents/planner.json` and any other `.kiro/agents/*.json` (D10)
- `.kairon/specs/` other than this file; `CHANGELOG.md` (AGENTS.md)

**Template sync trap.** Sync only the prompt: `cp .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/`.
Never `cp .kiro/agents/*.json`. `task sync:check` already fails on the agents tree before any change here (the
uncommitted JSON drift), so verify with the scoped diff above and report the pre-existing failure in the sentinel.

## 7. Team orchestration

```
live-preflight-baseline ─────────────────────────────┐ (R0 must run before any prompt/skill edit)
provenance-known-keys ─> planner-boundary ─┬─> prompt-static-tests ─┐
github-issue-store ───────────────────────┐│                         │
                                          ├┴─> planner-submit-cli    │
planner-skill-update (after R0) ──────────┴───────────────────────┐ │
                                                prompt-candidate-v1 ┘ (needs R0, static tests, skill)
                                                        │
planner-submit-cli ────────────────────────────────> live-eval-iterate
                                                        │
                                              sync-and-docs ─> validate-complete
```

- `live-preflight-baseline`, `provenance-known-keys` and `github-issue-store` start together. They touch different
  files (results dir, `internal/provenance`, `internal/github`).
- `planner-skill-update` must wait for R0, because the skill is staged into every eval workspace.
- Go work and prompt work meet at `live-eval-iterate`, which needs the CLI for the section 4.3 check.
- Per `AGENTS.md`, each Go task writes its failing test first and commits test and implementation together; the
  commit message names the test seen failing. Prompt, skill, docs and iteration logs are non-code.

## 8. Task breakdown

1. **live-preflight-baseline.** Run the preflight (section 4.2). If it passes, run `kairon eval --sandbox planner --repeat 3`
   on the untouched tree and commit the run directory (R0). Expect a FAIL verdict with `output_document_validity` and
   `two_mode_behavior` near zero. If it fails, write `LIVE-EVAL-BLOCKED` into the sentinel with the output.
2. **provenance-known-keys.** Test first: `TestCommitChainWithKnown` (an item depends on `#{a}` where `a` is only in
   `known`; the committed body has the real number and the hash covers it; a duplicate key between `known` and an item
   fails with `ErrInvalid` and writes nothing). Then add `CommitChainWith`; `CommitChain` delegates with nil.
3. **github-issue-store.** `internal/github.IssueStore{Repo, Labels}` with `Revision`, `Create`, `Update`.
   Tests assert the exact `gh` argument lists, that the body goes over stdin, that labels are passed on create only,
   and that errors are wrapped.
4. **planner-boundary.** Section 3.3. Test names must start with `TestBoundary` (integration: `TestBoundaryIntegration`).
   Cover: bare and fenced documents, prose before/after rejected, interactive punch-out rejected, automated count and
   source checks, marker in body rejected, canonical create path (title split, hash over the stripped body), canonical
   update path with a revision conflict routed to intake, punch-out posting the comment before the label with a
   comment that lists every `slot` and `reason`, a `failure.Runner` run that retries a malformed first reply and then
   commits, and a two-document chain whose second body uses `#{issue-1}`.
5. **planner-submit-cli.** Section 3.4. Tests use the fake store and a temp file; `--dry-run` writes nothing.
6. **prompt-static-tests.** Section 4.1. Must fail against the legacy prompt first.
7. **planner-skill-update.** Section 3.2, last paragraph. Run only after R0 exists (or after `LIVE-EVAL-BLOCKED` is recorded).
8. **prompt-candidate-v1.** Write the full new prompt to `.kairon/iterations/planner/v1.md` against P1-P10. Run
   `KAIRON_PLANNER_PROMPT=... go test ./internal/planner -run TestPlannerPrompt`. Do not edit the live prompt.
9. **live-eval-iterate.** Section 4.2-4.5: iterate candidates with `--prompt-file`, commit referenced runs, write the
   logs and `ITERATIONS.md`, run the section 4.3 dry-run check, spot-check `inferences` evidence by hand (D2), promote
   the winner to `.kiro/agents/planner-prompt.md`, run `verify-log`. Or follow the fallback.
10. **sync-and-docs.** Copy the prompt to the template; write `docs/planner.md` (contract, modes, boundary, `submit`,
    D4-D8, the manual-submit consequence, what is and is not certified); update `docs/provenance.md`,
    `docs/evaluation.md`, `README.md`.
11. **validate-complete.** Read-only: run every command in section 9, check the sentinels of tasks 1 and 9, and confirm
    D3/D10 (no suite or agent JSON change).

## 9. Validation commands

```bash
go run ./cmd/kairon plan parse .kairon/specs/issue-381-planner-output-document-prompt-rewrite.md
go build ./...
go test ./internal/planner ./internal/provenance ./internal/github ./cmd/kairon/... -count=1
go test ./internal/eval -run 'TestPlannerSuite|TestPlannerBoundary' -count=1
go run ./cmd/kairon eval --backend stub planner
go vet ./... && test -z "$(gofmt -l .)"
diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md
git diff --quiet -- .kairon/evals/cases .kairon/evals/rubrics .kairon/evals/fixtures .kiro/agents/planner.json
go run ./cmd/kairon eval verify-log planner --require-cycle=false     # only if the live runs happened
task test
```

## 10. Risks

- **The live GREEN may not be reachable in a few iterations.** Thresholds are 90 and 60 per criterion and the model
  is nondeterministic. Mitigation: the offline stub suite shows every check is satisfiable; section 4.4 forbids
  weakening the suite; the fallback in 4.2 keeps the PR honest.
- **Interactive UX gets worse until the TUI auto-submits** (D5, section 3.4). Accepted by the issue; documented.
- **Label deviation (D5)** and **uncertified automated-oversize rule (D8)** need a maintainer's eye in the PR.
- **Skill and prompt are measured together.** Run R0 before editing either, and do not edit the skill between
  candidate runs that a log compares.
- **Title stripping (D6)** is a convention this spec adds. It is covered by tests and `docs/planner.md`.
- **Tool chatter.** `Extract` needs the final assistant message, not raw `kiro-cli` stdout with tool traces. Whoever
  calls the boundary in automated mode must pass the final message; this PR does not build that caller.

## 11. Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "live-preflight-baseline"
    agent: "builder"
    description: "Check whether a live kairon eval run is possible here (kiro-cli --version, a one-case sandbox run of interactive-option-rules, podman info or docker info). If possible, run 'go run ./cmd/kairon eval --sandbox planner --repeat 3' on the UNMODIFIED tree (before any edit to .kiro/agents/planner-prompt.md or the planner-conventions skill) and commit the generated results directory as run R0; it must show a FAIL verdict. Always pass --sandbox: the legacy prompt runs a real gh issue create without it. If the preflight fails for infrastructure reasons, do not loop and do not fabricate anything: write the line 'LIVE-EVAL-BLOCKED: <reason>' and the failing command output into the sentinel."
    dependencies: []
    acceptance_criteria:
      - "Either a results directory containing planner.json and summary.json for a complete --sandbox run of the legacy prompt is committed under .kairon/evals/results/, or the sentinel contains a line starting with LIVE-EVAL-BLOCKED: followed by the failing preflight output"
      - "No file under .kiro/ and no file under .kairon/evals/cases, rubrics or fixtures is modified by this task"
      - "If the run happened, the sentinel records the run directory name, the agent verdict and the criteria below the 60 percent floor"
    validation_commands:
      - "ls .kairon/evals/results/*/planner.json >/dev/null 2>&1 || grep -q 'LIVE-EVAL-BLOCKED' .kairon/artifacts/builder-381-live-preflight-baseline.md"
      - "git diff --quiet -- .kairon/evals/cases .kairon/evals/rubrics .kairon/evals/fixtures"

  - id: "provenance-known-keys"
    agent: "builder"
    description: "Add provenance.Committer.CommitChainWith(ctx, items, admit, known map[string]int) that seeds the placeholder number map so a split chain can be resumed or committed one document at a time; CommitChain delegates with nil. A key in known that an item also uses is rejected with ErrInvalid before anything is written. Test first (TestCommitChainWithKnown*): see that it fails because the method does not exist, then implement. Commit test and code together."
    dependencies: []
    acceptance_criteria:
      - "CommitChainWith exists with the stated signature and CommitChain behaves exactly as before (existing chain tests unchanged and passing)"
      - "An item whose Dependencies line is '#{a}' with a only in known commits a body whose Dependencies line is the real number, and the stored hash validates against the committed body"
      - "A key present in both known and an item returns an error matching ErrInvalid and makes no store call"
      - "internal/provenance still imports nothing forbidden by TestNoForbiddenImports"
    validation_commands:
      - "go test ./internal/provenance -run 'TestCommitChainWith' -count=1"
      - "go test ./internal/provenance -count=1"
      - "go vet ./internal/provenance"

  - id: "github-issue-store"
    agent: "builder"
    description: "Implement internal/github.IssueStore (fields Repo string and Labels []string) satisfying provenance.IssueStore with the gh CLI through the existing ghFunc seam: Revision reads updatedAt of the issue, Create runs 'gh issue create' with the title, '--body-file -' with the body on stdin and one --label per configured label, and parses the new issue number from the output, Update runs 'gh issue edit' with the body on stdin. Argument lists only, never a shell. Test first with a fake ghFunc asserting exact arguments, stdin, label handling on create only, and wrapped errors."
    dependencies: []
    acceptance_criteria:
      - "IssueStore satisfies provenance.IssueStore (compile-time assertion in the package)"
      - "Tests assert the exact gh arguments for Revision, Create and Update, and that the body is passed on stdin and never on the command line"
      - "Labels are passed on Create only; an empty Labels slice adds no --label argument"
      - "A gh failure is returned wrapped with the operation name and issue number"
      - "internal/provenance does not import internal/github and no import cycle is created"
    validation_commands:
      - "go test ./internal/github -run 'TestIssueStore' -count=1"
      - "go build ./..."
      - "go vet ./internal/github"

  - id: "planner-boundary"
    agent: "builder"
    description: "Create internal/planner per section 3.3 of the design spec: mode.go (Mode, AutomatedIntakeLine, AutomatedPrompt), extract.go (one document, optional single yaml fence, no other text), title.go (split the first H1 from the body), boundary.go (Handle: extract, mode contract, guard.Run with failure.Route, punch_out to failure.PunchOut with Attempt{1,1} and a FailureContext built from the unresolved list, canonical to provenance.CommitChainWith with keys issue-N and an admit that runs the guard chain on the substituted text). Add deps_test.go banning os/exec, net/http and crypto/hmac. Tests are named TestBoundary* (integration: TestBoundaryIntegration*, chain: TestBoundaryChainResolvesDependencies, punch-out: TestBoundaryAutomatedPunchOutRoutesToTier2) and use fakes for provenance.IssueStore, provenance.Router and failure.Issues. Test first and commit tests with code."
    dependencies: ["provenance-known-keys"]
    acceptance_criteria:
      - "A reply with prose before or after the document, or two documents in one reply, is rejected with a filled FailureContext and no store or issue call"
      - "Interactive mode rejects a punch_out document; automated mode rejects a reply count other than one and a source that is not github_issue with ref equal to the issue number"
      - "A body containing the provenance marker prefix is rejected before any write"
      - "A valid canonical document commits through CommitChainWith: the H1 is the created title and is not in the committed body, and the hash covers the committed body"
      - "A punch_out document (automated) calls failure.PunchOut once; the fake records the comment before the label; the comment text names every unresolved slot and reason; no IssueStore call is made; the FailureContext has Attempt{1,1}"
      - "A malformed first reply followed by a valid one, run under failure.Runner, retries once and then commits; a malformed reply on its own makes no GitHub write"
      - "Two documents where the second says 'Dependencies: #{issue-1}' commit in order and the second committed body parses with watcher.DependencyParser to the number the fake store assigned to the first"
      - "An edited-under-the-planner revision conflict returns the routed outcome and writes nothing"
      - "The package has no forbidden imports"
    validation_commands:
      - "go test ./internal/planner -run 'TestBoundary|TestNoForbiddenImports' -count=1"
      - "go vet ./internal/planner"
      - "go build ./..."

  - id: "planner-submit-cli"
    agent: "builder"
    description: "Add 'kairon planner submit' in cmd/kairon/cmd/planner.go: reads replies from --file or stdin (documents separated by a line containing only ---), flags --repo, --label (default off), --known key=number (repeatable) and --dry-run (extract, mode contract and guard chain only, no writes). Interactive mode only. On success print one 'committed issue-N -> #M' line per document; on rejection print the FailureContext Expected and Actual and exit 1. Wire it to the boundary with github.IssueStore. Tests inject a fake store and use a temp file."
    dependencies: ["planner-boundary", "github-issue-store"]
    acceptance_criteria:
      - "'kairon planner submit --dry-run' on a valid document exits 0 and calls no store method"
      - "A rejected document exits non-zero and prints the guard's expected and actual text"
      - "--label is off unless given, and --known seeds the chain keys"
      - "The command is registered under the root command and listed in 'kairon planner --help'"
    validation_commands:
      - "go test ./cmd/kairon/... -run 'Planner' -count=1"
      - "go run ./cmd/kairon planner submit --help"
      - "go build ./..."

  - id: "prompt-static-tests"
    agent: "builder"
    description: "Write internal/planner/prompt_test.go (TestPlannerPrompt*) as a small regression guard against the original defect. It reads the live .kiro/agents/planner-prompt.md, or the file named by KAIRON_PLANNER_PROMPT. Forbidden strings: 'gh issue create', '--body-file', '/tmp/issue-body.md'. Required anchors: planner.AutomatedIntakeLine, 'schema_version: 1', 'result: punch_out', 'unresolved', 'inferences', '#{issue-'. Add internal/eval/planner_boundary_test.go (TestPlannerBoundary*): every stub document in the planner suite passes planner Extract, the mode contract and guard.DefaultChain; every automated-*.yaml case input starts with planner.AutomatedIntakeLine; an interactive and an automated canonical stub document parse to the same Go type, schema_version, stage and result with no mode-specific key. Run the prompt test first against the legacy prompt and see it fail for the forbidden-string reason."
    dependencies: ["planner-boundary"]
    acceptance_criteria:
      - "TestPlannerPrompt fails against the legacy prompt because it contains 'gh issue create' (record the failing output in the sentinel)"
      - "TestPlannerBoundary tests pass against the shipped eval suite without editing it"
      - "Changing the first line of an automated case input would make the preamble test fail (the test reads the YAML, not a copy of the string)"
      - "git diff shows no change under .kairon/evals/cases, rubrics or fixtures"
    validation_commands:
      - "go test ./internal/eval -run 'TestPlannerBoundary|TestPlannerSuite' -count=1"
      - "go vet ./internal/planner ./internal/eval"
      - "git diff --quiet -- .kairon/evals/cases .kairon/evals/rubrics .kairon/evals/fixtures"

  - id: "planner-skill-update"
    agent: "builder"
    description: "Update .kiro/skills/planner-conventions/SKILL.md (not a template-synced file): add a 'Mode applicability' section stating the root-cause decision framework, hypothesis testing and checklists apply to interactive mode and that in automated mode findings go into Context and the inferences list and nothing is asked; in the option example remove the Pros/Cons lines, state that every option is a single commitment (and allowed, or forbidden) and mark exactly one option as recommended. Do this only after the R0 baseline exists or LIVE-EVAL-BLOCKED has been recorded, because the skill is staged into every eval workspace."
    dependencies: ["live-preflight-baseline"]
    acceptance_criteria:
      - "The skill has a section naming interactive and automated applicability"
      - "The option example has no Pros/Cons lines and shows one recommended option"
      - "No template copy of the skill is created and cmd/kairon/templates/kiro/skills is unchanged"
    validation_commands:
      - "grep -qi 'automated' .kiro/skills/planner-conventions/SKILL.md"
      - "! grep -qE '^\\s*- (Pros|Cons):' .kiro/skills/planner-conventions/SKILL.md"
      - "git diff --quiet -- cmd/kairon/templates/kiro/skills"

  - id: "prompt-candidate-v1"
    agent: "builder"
    description: "Write the complete new planner prompt to .kairon/iterations/planner/v1.md covering requirements P1-P10 of section 3.2 of the design spec: stage-output document as the only final reply with canonical and punch-out examples; no GitHub writes and no hash; first-line mode selection using AutomatedIntakeLine; investigate-classify-coverage; the issue body standard; interactive flow with the two option rules, draft review gate and no label question; automated flow with best-effort inference, recorded evidence, punch-out and one document; split with #{issue-N} placeholders; hard bars. Keep the useful root-cause and planning-worktree guidance from the legacy prompt as read-only investigation. Do NOT edit the live .kiro/agents/planner-prompt.md in this task."
    dependencies: ["live-preflight-baseline", "prompt-static-tests", "planner-skill-update"]
    acceptance_criteria:
      - ".kairon/iterations/planner/v1.md exists and passes TestPlannerPrompt when KAIRON_PLANNER_PROMPT points at it"
      - "The candidate contains the examples for both result variants using the real envelope keys"
      - "The live .kiro/agents/planner-prompt.md is byte-identical to its template twin and unchanged by this task"
      - "The candidate states the automated-oversize punch-out rule in no more than two sentences"
    validation_commands:
      - "KAIRON_PLANNER_PROMPT=$PWD/.kairon/iterations/planner/v1.md go test ./internal/planner -run 'TestPlannerPrompt' -count=1"
      - "git diff --quiet -- .kiro/agents/planner-prompt.md"

  - id: "live-eval-iterate"
    agent: "builder"
    description: "Drive the candidate with the frozen suite (section 4 of the design spec). Score each candidate with 'go run ./cmd/kairon eval --sandbox planner --prompt-file .kairon/iterations/planner/vN.md' (use --repeat 3 for runs a green or refactor log cites), commit every run a log references, and write .kairon/iterations/planner/iteration-NN.md logs (00 baseline from R0 to the first candidate run, then green, optionally refactor) plus an ITERATIONS.md index explaining that the RED evidence is R0 and why --require-cycle is off. After each run extract the final reply of every document-returning case from the result JSON and run it through 'kairon planner submit --dry-run'; record any reply the regexes pass and the parser rejects. Read each inferences evidence field of the automated-best-effort-fill reply against the fixtures by hand and record the result. Promote the winning candidate by copying it over .kiro/agents/planner-prompt.md and run verify-log. Do not edit .kairon/evals cases, rubrics or fixtures; if a check looks wrong, record it in the log and the sentinel instead. If the sentinel of live-preflight-baseline has LIVE-EVAL-BLOCKED, write no iteration logs, promote v1 on the strength of the offline gates, write ITERATIONS.md with status 'pending: live runs not performed' and the exact maintainer commands, and say so in the sentinel."
    dependencies: ["prompt-candidate-v1", "planner-submit-cli"]
    acceptance_criteria:
      - "Either every iteration log passes 'kairon eval verify-log planner --require-cycle=false' and the last recorded run of the promoted prompt has an agent PASS verdict, or the sentinel contains LIVE-EVAL-BLOCKED with the preflight evidence and no iteration log exists"
      - "No iteration log cites a run directory that is not committed, and no score is typed that the run did not record"
      - ".kairon/iterations/planner/ITERATIONS.md exists and states the status truthfully"
      - ".kiro/agents/planner-prompt.md contains no 'gh issue create' and passes TestPlannerPrompt"
      - "git diff shows no change under .kairon/evals/cases, rubrics or fixtures"
      - "The dry-run check of section 4.3 and the manual evidence spot-check are recorded in the last iteration log or in the sentinel"
    validation_commands:
      - "test -f .kairon/iterations/planner/ITERATIONS.md"
      - "go test ./internal/planner -run 'TestPlannerPrompt' -count=1"
      - "ls .kairon/iterations/planner/iteration-*.md >/dev/null 2>&1 && go run ./cmd/kairon eval verify-log planner --require-cycle=false || grep -q 'LIVE-EVAL-BLOCKED' .kairon/artifacts/builder-381-live-preflight-baseline.md"
      - "git diff --quiet -- .kairon/evals/cases .kairon/evals/rubrics .kairon/evals/fixtures"

  - id: "sync-and-docs"
    agent: "builder"
    description: "Copy only the prompt into the template with 'cp .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/' (never cp the .json files: the working tree has unrelated uncommitted changes to them). Write docs/planner.md (output contract, the two modes, the boundary and its outcomes, kairon planner submit, decisions D4-D8, the manual-submit consequence and TUI auto-submit as a follow-up, what the eval certifies and what is verified only offline). Update docs/provenance.md to remove the statements that #381 will supply the adapter and has not defined the placeholder convention and link to docs/planner.md, update the Planner suite section of docs/evaluation.md (the preamble is confirmed and pinned by a test; the RED/GREEN workflow; remove wording that says the suite is expected to fail against the pre-#381 prompt), and add a short pointer in README.md. Do not touch CHANGELOG.md or any file named as historical in AGENTS.md."
    dependencies: ["live-eval-iterate"]
    acceptance_criteria:
      - "diff of .kiro/agents/planner-prompt.md and its template copy is empty"
      - "No .json file under cmd/kairon/templates/kiro/agents or .kiro/agents is changed by this task"
      - "docs/planner.md exists and names kairon planner submit, the AutomatedIntakeLine, the #{issue-N} convention and the label decision"
      - "docs/provenance.md no longer says #381 supplies the adapter"
      - "CHANGELOG.md and .kairon/specs/maturity-model are unchanged"
    validation_commands:
      - "diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md"
      - "grep -q 'planner submit' docs/planner.md"
      - "! grep -q '#381 supplies the adapter' docs/provenance.md"
      - "git diff --quiet -- CHANGELOG.md .kairon/specs/maturity-model"
      - "go test ./internal/... -count=1"

  - id: "validate-complete"
    agent: "validator"
    description: "Read-only verification of the whole issue. Run every command in section 9 of the design spec; read the sentinels of live-preflight-baseline and live-eval-iterate; confirm decisions D3 (suite unchanged) and D10 (no agent JSON change); confirm that if LIVE-EVAL-BLOCKED is present the preflight evidence is genuine and the PR text says acceptance criterion 8 is not met; confirm no iteration log cites a missing run."
    dependencies: ["sync-and-docs"]
    acceptance_criteria:
      - "All deterministic commands in section 9 pass, with the one pre-existing exception that task sync:check fails on the .kiro/agents JSON drift that is not part of this change"
      - "Acceptance criteria 1 to 7 are each backed by a named passing test or by the stub suite as listed in section 5 of the design spec"
      - "Acceptance criterion 8 is either backed by committed runs that verify-log accepts, or reported as NOT MET with the LIVE-EVAL-BLOCKED evidence"
      - ".kairon/evals/cases, rubrics, fixtures and .kiro/agents/planner.json have no diff"
      - "The legacy prompt text 'gh issue create' appears nowhere in .kiro/agents/planner-prompt.md or its template copy"
    validation_commands:
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-381-planner-output-document-prompt-rewrite.md"
      - "go test ./internal/planner ./internal/provenance ./internal/github ./cmd/kairon/... -count=1"
      - "go test ./internal/eval -run 'TestPlannerSuite|TestPlannerBoundary' -count=1"
      - "go run ./cmd/kairon eval --backend stub planner"
      - "go vet ./... && test -z \"$(gofmt -l .)\""
      - "! grep -q 'gh issue create' .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md"
      - "git diff --quiet -- .kairon/evals/cases .kairon/evals/rubrics .kairon/evals/fixtures .kiro/agents/planner.json"
```
