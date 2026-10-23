# Design Spec: Split the planner into two single-mode agents sharing a Kairon-managed skill

Closes #397

Design of record: `.kairon/specs/maturity-model/planner-requirements.md` (§1, §3, §3.1.1, §3.5, §4, §5-§7, §9,
§10, §10.1, §10.2, §10.3, §14). Consumed as-is, not re-implemented: `internal/stageoutput`, `internal/guard`,
`internal/provenance`, `internal/failure`. Consumed, not edited: the planner eval suite from #380
(`.kairon/evals/rubrics/planner.yaml`, `.kairon/evals/cases/planner/*.yaml`, `internal/eval/planner_suite_test.go`).

## 1. Solution approach

Today `.kiro/agents/planner-prompt.md` has one agent that runs `gh issue create` itself and assumes a human is always
present. The design of record removes the agent from the write path: the planner returns a stage-output document and a
deterministic boundary validates, hashes and commits it. This issue builds three things.

1. **Two single-mode agents.** `planner` (interactive elicitation, §10.1) and `planner-automated` (unattended intake,
   §10.2). Mode is a fact the caller knows, so the caller picks the agent name and neither prompt branches on mode.
2. **One shared Kairon-managed skill, `planner-shared`.** It holds everything that is the same in both modes: the
   output contract, restrictions, investigate/classify/cover, the issue-body standard and the mode-neutral hard bars.
   Both manifests load it through `resources` (`skill://`). It is a runtime pull; there is no build or compose step.
3. **The planner output boundary** (`internal/planner`, new). It extracts the agent's document from its reply, routes
   on `result`, sends `canonical` through the guard chain and the provenance commit step, and sends `punch_out` to the
   Tier 2 failure contract. It is glue over the merged packages and adds no new contract.

### Decisions

| # | Decision | Reason |
|---|----------|--------|
| D1 | Interactive-only rules (one decision per turn, a)/b)/c)/Other, single-commitment option rule, single-option recommendation rule, confirm-defaults gate, draft-review gate, label turn, oversize split) live in `planner-prompt.md` only. Automated-only rules (never interview, best-effort inference with recorded evidence, punch-out naming each unresolvable slot) live in `planner-automated-prompt.md` only. | AC2 forbids interactive-only instructions in the automated agent. A shared skill that carried the option rules would load them into the automated agent anyway, so the skill holds only mode-neutral content. |
| D2 | The skill describes both result variants of the contract. Which variant an agent may emit is stated in that agent's prompt: `planner` never emits `punch_out`, `planner-automated` owns it. | Same contract in both modes (constraint), but different permission (AC6). |
| D3 | The boundary is mode-agnostic. It routes on `result` only. The only mode-dependent code is the agent name chosen at the invocation site (`planner.AgentInteractive` / `planner.AgentAutomated`). | AC5: no separate mode-routing component. |
| D4 | Add one planner-side guard, `DependenciesForm`, to the chain after `guard.DefaultChain`. It rejects a body with no single `Dependencies:` line, with an unresolved `#{key}` placeholder, or with a `## Dependencies` header followed by bullets. | The merged structural guard only checks that a `Dependencies` section exists. The constraint "never a bullet list" is otherwise enforced only by the prompt. Appending a guard is the extension seam §4.1 describes. |
| D5 | The automated eval half runs the three `automated-*` cases against `planner-automated` through an `--evals-dir` overlay (`agents/planner.json` is the `planner-automated` manifest renamed to `planner`, plus its prompt). The interactive half runs the four interactive cases against `planner` unchanged. A Taskfile target and script drive both. | The harness keys a suite by the directory name: `loadCases` overwrites `tc.Agent` with `planner`, and the rubric is `planner.yaml`. #380's cases cannot be edited here, and a suite for `planner-automated` would duplicate them. The overlay certifies the real `planner-automated` manifest, prompt and shared skill without touching #380. The alternative (a `run_as` case field) is a harness change and is deferred. |
| D6 | The label question (§10.1 step 9) stays in `planner-prompt.md`, as the #380 reference transcripts show. Its answer is **not** carried: schema v1 is closed (adding a key is a `schema_version` bump, §3.4) and the boundary applies no label. | Honest gap, listed under Risks. A follow-up must decide how the answer reaches the boundary. |
| D7 | `.kairon/iterations/planner/ITERATIONS.md` is the human-readable RED to GREEN log the issue asks for. No `iteration-NN.md` is written unless committed real-model result directories exist for `kairon eval verify-log` to check. | `verify-log` checks every score against committed results; a fabricated entry would fail it and mislead. |

## 2. Relevant files

Create (live files, each with a byte-identical template copy unless noted):

| Live path | Template path | Notes |
|-----------|---------------|-------|
| `.kiro/skills/planner-shared/SKILL.md` | `cmd/kairon/templates/kiro/skills/planner-shared/SKILL.md` | Kairon-managed; in the sync gate |
| `.kiro/agents/planner-automated.json` | `cmd/kairon/templates/kiro/agents/planner-automated.json` | |
| `.kiro/agents/planner-automated-prompt.md` | `cmd/kairon/templates/kiro/agents/planner-automated-prompt.md` | |
| `internal/planner/doc.go`, `agents.go` | none | constants and package doc |
| `internal/planner/extract.go` | none | reply to document text |
| `internal/planner/boundary.go`, `deps_guard.go`, `chain.go` | none | the output boundary |
| `internal/planner/run.go` | none | the automated invocation site |
| `internal/planner/*_test.go` | none | see tasks |
| `internal/github/issuestore.go` (+ test) | none | `gh`-backed `provenance.IssueStore` |
| `scripts/eval-planner.sh` | none | two-half eval driver (root `scripts/`, deliberately not `.kairon/scripts/`, which is template-synced) |
| `.kairon/iterations/planner/ITERATIONS.md` | none | RED to GREEN log |
| `docs/planner.md` | none | new reference doc |

Modify:

- `.kiro/agents/planner.json`, `.kiro/agents/planner-prompt.md` and their template copies: interactive-only rewrite.
- `Taskfile.yml`: add the `planner-shared` diff line to `sync:check`; add `eval:planner`.
- `.kiro/skills/builder-conventions/SKILL.md`: add `.kiro/skills/planner-shared/*` to the sync mapping table and a
  sync command. (Project-owned, not templated.)
- `internal/tui/planning_tab.go` (line ~224) and `internal/tui/commands.go` (line ~361): replace the `"planner"`
  literal with `planner.AgentInteractive`.
- `docs/evaluation.md` (Planner suite section): two agents, which cases run against which, the overlay, and that the
  mode signal is now the agent name (the `AUTOMATED INTAKE` preamble in the three automated cases stays as input text).
- `README.md`: one line linking `docs/planner.md`.

Do not touch:

- `.kairon/specs/` (everything except this file), `.kairon/evals/results/` (the eval harness writes new directories;
  do not edit existing ones), `CHANGELOG.md`.
- `.kairon/evals/rubrics/planner.yaml`, `.kairon/evals/cases/planner/*`, `internal/eval/planner_suite_test.go`,
  `internal/eval/testdata/planner-legacy/*` (#380).
- `internal/stageoutput`, `internal/guard`, `internal/provenance`, `internal/failure` (consume only).
- `.kiro/skills/planner-conventions/SKILL.md`: unchanged in role, never templated, never in the sync gate.
- **The uncommitted edits to `.kiro/agents/{architect,builder,documenter,krew-lead,validator}.json`.** They are
  pipeline setup (a `@creds-agent` tool and an `mcpServers` block). Do not stage them. See the planner.json note below.

### Worktree hazards the builder must respect

- `.kiro/agents/planner.json` is one of the dirty files **and** one of ours. Write the new `planner.json` from the
  committed (`git show HEAD:.kiro/agents/planner.json`) formatting, without `@creds-agent` or `mcpServers`. The
  committed diff for it must show only the intended change. Stage by explicit path; never `git add -A` or `git add .`.
- Because of those dirty files, `task sync:check` already fails on the agents tree in this worktree before any change
  here. Do not "fix" it by copying dirty files into the templates. Verify our files with the scoped per-file `diff`
  commands in the tasks. A clean checkout (CI) runs the real `task sync:check`.
- The eval harness stages only **git-tracked** files of `.kiro/` into a case workspace (`projectKiroFilter`,
  `git ls-files --cached`) and never stages `*-conventions` skills. Run `git add` on the new skill and agent files
  before any `kairon eval` run, or the agent is evaluated without the shared skill. This is also what makes the
  evaluation load the real skill and not a copy.

## 3. Component design

### 3.1 The shared skill (`planner-shared`)

`SKILL.md` with front matter `name: planner-shared`, a one-sentence `description`. Sections, in order:

1. **Restrictions.** No GitHub write of any kind (`gh issue create`, `gh issue edit`, `gh issue comment`,
   `gh api` with a write method, any label change). No provenance marker, hash or `sha256`/`provenance` key in the
   output. No writes to project files. Shell is read-only: `cat`, `ls`, `grep`/`rg`, `git log|show|diff|status`,
   `gh issue view|list`, `gh pr view|list`, reading `.kairon/config.yaml`. The boundary, not the agent, writes
   (§3, §10.3). If asked to implement or fix, decline: the planner documents work, it does not do it.
2. **Output contract** (§3.1.1, §3.5). The reply is exactly one stage-output document and nothing else. It may be
   wrapped in one `yaml` fence. Fields: `schema_version: 1`, `stage: planner`, `result`, `source.kind`
   (`github_issue` | `document`), `source.ref`, `generated` (RFC 3339 UTC), then `body: |` for `canonical` or
   `unresolved:` (non-empty list of `slot` and `reason`) for `punch_out`; optional `inferences` (`slot`, `inferred`,
   `evidence`). The two variants are mutually exclusive. Include one complete example of each variant. State that the
   prompt of the agent says which variant that agent may emit.
3. **Investigate, classify, cover** (§4 here means design steps 1-3 of §10.1/§10.2): read the repository before
   anything else, resolve what it answers with a file reference; classify Feature / Bug / Refactor and pick optional
   sections; the coverage map slots (problem and evidence; desired behavior; scope boundary and out of scope;
   acceptance criteria and their verification; edge and error behavior; constraints; dependencies).
4. **Issue-body standard** (§5-§7): the three types and the folding table; the shared spine in order (title <=70
   characters, Problem Statement, Context, Scope with In and Out of Scope, Acceptance Criteria each with a
   black-box `Verify:` line, optional Constraints, Dependencies); the optional-section catalog and when each attaches;
   no User Story, no Verification/Test Plan section; size limits (<=6 In Scope entries, <=8 acceptance criteria);
   **the Dependencies rule**: a single line `Dependencies: #12, #15`, or `Dependencies: none`; never a `## Dependencies`
   header with bullets (the watcher parses zero dependencies from that). A dependency on a not-yet-created issue of the
   same split is written `#{key}` on that same single line, where `key` matches `[A-Za-z0-9][A-Za-z0-9_-]*`; the
   boundary resolves it (§7). A dependency on an existing issue is `#<number>`.
5. **Mode-neutral hard bars** (§10.3): no ungrounded assumption (every statement traces to the repository, the
   request, or an inference whose evidence is recorded where the agent's prompt permits inference); no vague terms in
   acceptance criteria (`properly`, `appropriate`, `gracefully`, `fast`, `robust`, `as needed`, `etc.`, `should work`)
   unless quantified; no design outside Constraints and no Constraint the requester did not state; do not re-derive
   what the repository already states; return a well-formed document and nothing else.

The skill must contain **no** interactive-only text (`one question`, `a)` option lists, `Recommended:`, `draft`
review, `approve`) and **no** mode-selection text (`if you are running interactively`, `in automated mode, ...`).

### 3.2 `planner` (interactive) and `planner-automated`

Both manifests follow `builder.json`/`validator.json`: `resources` is a list of two `skill://` entries,
`skill://.kiro/skills/planner-conventions/SKILL.md` and `skill://.kiro/skills/planner-shared/SKILL.md`. Model
`claude-sonnet-5.5` as in the existing manifests. Each prompt uses the RTCC headings (`## Role`, `## Task`,
`## Context`, `## Constraints`) so `kairon eval audit` can map instructions. Neither prompt restates the output
contract or the body standard; each points at the shared skill.

| | `planner` | `planner-automated` |
|---|-----------|---------------------|
| Manifest `description` | Interactive planning agent that interviews a human and returns a stage-output document. | Unattended intake agent that canonicalizes an existing issue into a stage-output document or a punch-out. |
| `tools` / `allowedTools` | `read`, `write` (limited to `.worktrees/planning-*` as today), `shell`, `web_search`, `web_fetch` | `read`, `shell` only. No `write`, no web tools. |
| `toolsSettings.shell` | `autoAllowReadonly: true`; deny GitHub write commands and `git push` | `autoAllowReadonly: true`; allow only the read-only commands listed in 3.1; deny GitHub write commands and `git push` |
| Prompt: only | the §10.1 flow: investigate before asking; coverage map; one decision per turn; a)/b)/c)/Other; single-commitment option rule (`and` allowed, `or` forbidden); single-option recommendation, none when no option can be chosen; constraints asked, never invented; escape hatch; one confirm-defaults turn; draft review (body, coverage summary outside the issue, one approval question); label question as a separate turn; return the document; oversize split. Open required slot is a question. **Never returns `punch_out`.** Bug root-cause vs symptom choice. | the §10.2 flow: never interview, never offer options, never write a draft for approval; read the issue (given in the input, as an attached file, or as `owner/repo#N` to fetch with `gh issue view`); best-effort fill with grounded inference, recording `slot`/`inferred`/`evidence` for every inference ("reasonable" means a competent engineer reading the same issue and repository reaches the same conclusion; a coin-flip among materially different options is not reasonable); then either a `canonical` document, or a `punch_out` whose `unresolved` names **every** unresolvable required slot with its reason; never a partial canonical document; the reply is the document immediately. |
| Prompt: must not contain | any `punch_out` emission instruction; the contract field list; the body spine | any of: one-question-per-turn, option lists, `Recommended:`, draft review or approval, label question; the contract field list; the body spine |

Oversize split in `planner`: more than ~6 blocking questions, or a body over the size limits, triggers a proposed
split: remaining issues as one-line summaries in dependency order, each with a short kebab-case `key` in the summary;
continue with only the first; the first must not depend on a later one. Each later issue is planned in its own run and
returns its own document, referring to earlier ones as `#{key}` (see 3.1). The boundary commits them in order (3.3).

### 3.3 The planner output boundary (`internal/planner`)

```go
package planner

const (
    AgentInteractive = "planner"           // interactive launch
    AgentAutomated   = "planner-automated" // watcher / unattended intake
    SharedSkill      = "planner-shared"
)

// Extract returns the document text from an agent reply: the whole reply, or the content of
// one yaml/yml fence that is the whole reply. Surrounding whitespace is ignored. Anything
// else before or after (prose, a second fence, a second document) is an error.
func Extract(reply string) (string, error) // errors.Is ErrNoDocument | ErrExtraneousText

type Boundary struct {
    Committer *provenance.Committer
    Issues    failure.Issues         // punch-out port (internal/github.CLI satisfies it)
    Target    failure.Target         // Repo, Issue, FailLabel for punch-out; Issue 0 on the create path
    Guards    func(guard.FailureHandler) []guard.Guard // nil means DefaultChain + DependenciesForm
    NewRunID  func() string          // nil means failure.NewRunID
}

type Request struct {
    Reply   string
    Attempt guard.Attempt            // supplied by the caller; this package never counts attempts
    Target  provenance.Target        // Number 0 creates; else Number + ObservedRevision
    Title   string                   // optional; default is the first "# " line of the body
}

func (b *Boundary) Process(ctx context.Context, req Request) (Result, error)
func (b *Boundary) ProcessChain(ctx context.Context, items []ChainRequest) (provenance.ChainResult, error)
```

`Process` order of operations:

1. `Extract`. On error return `OutcomeRejected` with a `guard.FailureContext` (`Guard: "extract"`, `Expected`: the
   reply is exactly one stage-output document, `Actual`: the error), disposition `failure.Recoverable`.
2. `stageoutput.Parse`. On error run the reply through the guard chain so the structural guard produces the
   `FailureContext`; return `OutcomeRejected`.
3. Route on `doc.Result` (§3.5):
   - `canonical`: `guard.Run` with a `failure.Route` whose `Recoverable()` handler is the chain's failure handler. On
     `OutcomeRejected` return `OutcomeRejected` with the failure and `Disposition` from `route.Result()` so a
     `failure.Runner` can retry. On `OutcomeReadyToCommit` call `Committer.Commit`; map `OutcomeCommitted` (with
     `Number` and `Hash`), `OutcomeRoutedToIntake` and `OutcomePunchedOut`.
   - `punch_out`: require `Target.Issue > 0` (else a contract error: there is no issue to comment on). Build the
     `guard.FailureContext` per §3.5: `Stage: planner`, `Guard: "intake"`, `Attempt: {1, 1}`, `Expected`: the required
     slots were fillable, `Actual`: the `unresolved` list, one `slot: reason` per entry. Call
     `failure.PunchOut(Issues, Target, runID, fc, remedy)`; comment first, then label. Return `OutcomePunchedOut`.
4. The agent's reply is never written anywhere by this package except through `Committer` and `PunchOut`.

`ProcessChain` takes `[]ChainRequest{Key, Reply, Title, Target}` in dependency order, extracts and parses each,
builds `provenance.ChainItem`s and calls `Committer.CommitChain` with an `Admit` that wraps `guard.Run`. Substitution
of `#{key}` happens before each guard chain and before hashing (§7).

`DependenciesForm` is a `guard.Guard` (`Admit` is pure; `OnFailure` calls the injected handler). It is checked after
substitution, so a placeholder still present in `Admit` means an unresolved reference.

`RunAutomated` (the automated invocation site, `run.go`) takes an injected `Invoker`
(`func(ctx, agent, prompt, dir string) (reply string, err error)`; production runs `kiro-cli chat --agent <agent>
--no-interactive` and strips ANSI, as `internal/agent/output_capture.go` does), the repo, the issue number and the
observed revision. It always passes `AgentAutomated`. It wraps `Process` in a `failure.Runner` so a recoverable
rejection is retried (new `Attempt` each time) up to `max_retries` and a punch-out is terminal. It does not pass
`--trust-all-tools`: the manifest's `allowedTools` bound what the unattended run may do.

The watcher is not wired to call `RunAutomated` (that is #382). Interactive sessions are TTY sessions
(`kiro-cli chat --classic`) whose reply the boundary cannot capture today; wiring the interactive reply into
`Process` is a follow-up. This issue changes the interactive launch sites only to use `planner.AgentInteractive`.

`internal/github` gains `IssueStore{Repo string; run ghFunc}` implementing `provenance.IssueStore` with `gh`
(`Revision` = `updatedAt` from `gh issue view --json updatedAt`; `Create` = `gh issue create --body-file -` and parse
the number from the printed URL; `Update` = `gh issue edit --body-file -`). Bodies travel on stdin, never in argv.
`RouteToIntake` is intentionally trivial: nothing was written, so the issue is still unmarked or mismatched and the
watcher's own routing (#382) picks it up; a `provenance.Router` adapter in `internal/planner` returns nil there and
calls `failure.PunchOut` for its `PunchOut` method.

### 3.4 Sync gate and init

- `kairon init` and `kairon update` copy the whole embedded `templates/` tree (`internal/templates/extract.go`), so
  adding `cmd/kairon/templates/kiro/skills/planner-shared/` and the two agent files is enough for installation. No Go
  change is needed there. `planner-conventions` is not in `templates/`, so it is never written or overwritten.
- `Taskfile.yml` `sync:check` already diffs `.kiro/agents/` as a directory, so the new agent files are covered once
  they exist. Add the skill line:
  `diff -rq .kiro/skills/planner-shared/ cmd/kairon/templates/kiro/skills/planner-shared/ || error=1`
- `.kiro/skills/builder-conventions/SKILL.md`: add the mapping row and the sync command.

### 3.5 Eval driver and the RED to GREEN loop

`scripts/eval-planner.sh [--dry-run]` and `task eval:planner`:

1. Interactive half: `kairon eval --sandbox planner --filter 'name==interactive-single-decision,interactive-option-rules,interactive-return-document,multi-issue-split'`.
2. Automated half: build a temp evals dir (copy `.kairon/evals` without `results/`), write `agents/planner.json` as the
   `planner-automated.json` manifest with only `name` rewritten to `planner` (use `sed`, no `jq`), copy
   `planner-automated-prompt.md` next to it, then run
   `kairon eval --evals-dir <tmp> --sandbox planner --filter 'name==automated-best-effort-fill,automated-bug-classification,automated-punch-out'`.
   Copy the new result directory back under `.kairon/evals/results/`.
3. `--dry-run` prints both commands and the overlay contents without running anything.
4. Exit non-zero if either half fails.

The stub backend replays the reference transcripts regardless of the prompt, so it cannot certify the prompts. The
offline proof is: the #380 tests still pass (`go test ./internal/eval -run TestPlanner`), the static prompt tests in
`internal/planner` pass, and the boundary tests accept both agents' reference documents. The GREEN claim needs a
real-model run (`task eval:planner`). If that cannot run in the builder's environment (no `kiro-cli`, credentials or
container runtime), `ITERATIONS.md` must say so plainly and leave the real-model rows empty. Do not invent scores.

RED comes first. The real-model RED run of the **unmodified** planner must be recorded before the prompt is rewritten,
so the baseline task has no dependencies and every task that rewrites `planner-prompt.md` depends on it.

## 4. Team orchestration

| Layer | Tasks | Can run in parallel |
|-------|-------|---------------------|
| 0 | `red-baseline`, `github-issue-store` | both (disjoint files) |
| 1 | `shared-skill` | after `red-baseline`; it also creates the `internal/planner` package |
| 2 | `interactive-agent`, `automated-agent`, `sync-gate`, `output-extract` | all four (disjoint files; `sync-gate` owns `Taskfile.yml`) |
| 3 | `boundary-process`, `eval-driver` | in parallel; `eval-driver` needs both agents and `sync-gate` and edits `Taskfile.yml` after `sync-gate` |
| 4 | `boundary-chain`, `automated-invoker` | in parallel |
| 5 | `green-run` | after `eval-driver`, `boundary-chain`, `automated-invoker` |
| 6 | `document-planner` | after `green-run` |
| 7 | `validate-all` | last |

Every task that edits a template-synchronized file ends with the scoped `diff` for the files it touched. Commit tests
and implementation together (repo `AGENTS.md`); stage explicit paths only.

## 5. Step-by-step task breakdown

### Task `red-baseline` (builder)
Record RED before anything is rewritten. Run `go test ./internal/eval -run 'TestPlannerSuite' -count=1` and save the
result (the #380 legacy-transcript RED test is `TestPlannerSuiteRedLegacyTranscriptsFail`). Then try a real-model run
of the **unmodified** planner: `kairon eval --sandbox planner` (the old prompt can run `gh issue create`, so
`--sandbox` is mandatory). Create `.kairon/iterations/planner/ITERATIONS.md` with a title, a short method paragraph
(the two halves, the overlay, why the stub cannot certify prompts) and an iteration table whose first row is the
baseline: date, change `none (baseline)`, suite result, run directory names. If the real run cannot execute, record the
reason and the deterministic RED only.

### Task `github-issue-store` (builder)
TDD. In `internal/github/issuestore.go` add `IssueStore` per 3.3 with a fake `ghFunc` in
`issuestore_test.go`. Tests: `Revision` parses `updatedAt`; `Create` sends the body on stdin and returns the number
parsed from the URL; `Update` sends the body on stdin; a gh error is wrapped; an invalid repo or number is rejected
before any call (reuse `validateTarget`); a compile-time `var _ provenance.IssueStore = IssueStore{}`. Check
`internal/provenance` does not import `internal/github` (it must not).

### Task `shared-skill` (builder)
TDD. Create `internal/planner` with `doc.go`, `agents.go` (the three constants) and `helpers_test.go` (find the repo
root, read a manifest, read a file). Write `skill_test.go` first and watch it fail because the skill is missing, then
write `.kiro/skills/planner-shared/SKILL.md` per 3.1 and copy it to
`cmd/kairon/templates/kiro/skills/planner-shared/SKILL.md`. Tests assert: front matter name; it contains the contract
fields (`schema_version`, `result: canonical`, `result: punch_out`, `unresolved`, `inferences`), the spine headings,
the single-line `Dependencies:` rule, the `#{key}` rule, the prohibition on `gh issue create`, and the read-only shell
list; it contains none of the interactive-only or mode-selection strings in 3.1; live and template are byte-identical;
`planner-conventions` is absent from `cmd/kairon/templates/`.

### Task `interactive-agent` (builder)
TDD. Write `interactive_agent_test.go` first (RED), then rewrite `.kiro/agents/planner.json` and
`.kiro/agents/planner-prompt.md` per 3.2 and copy both to the templates. Tests assert: the manifest name, prompt path
and that `resources` lists both skills as `skill://` URIs; the prompt has the four RTCC headings, the coverage map,
the single-commitment and single-recommendation rules, the confirm-defaults and draft-review gates, the oversize
split, and a sentence forbidding `punch_out`; the prompt does not contain `gh issue create` as an instruction, the
contract field list (`schema_version`), the body spine, or any mode-selection phrase; live and template are
byte-identical. Follow the worktree hazards in section 2 for `planner.json`.

### Task `automated-agent` (builder)
TDD. Write `automated_agent_test.go` first (RED), then create `.kiro/agents/planner-automated.json` and
`.kiro/agents/planner-automated-prompt.md` per 3.2 and copy both to the templates. Tests assert: manifest name,
`resources` lists both skills, tools are exactly `read` and `shell`; the prompt has the RTCC headings, states that it
never asks, never offers options and never drafts for approval, requires `inferences` with evidence, requires a
`punch_out` naming every unresolvable slot with a reason and forbids a partial canonical document; the prompt matches
none of the interactive-only patterns (`one question`, option lists `a)`, `Recommended:`, draft review, approval,
label question) and none of the mode-selection patterns, and does not restate the contract or the spine; live and
template are byte-identical. Add a cross-agent test: both manifests list the same shared skill URI.

### Task `sync-gate` (builder)
Add the skill diff line to `sync:check` in `Taskfile.yml` and the mapping row and command to
`.kiro/skills/builder-conventions/SKILL.md`. TDD: `syncgate_test.go` in `internal/planner` asserts `Taskfile.yml`
contains the `planner-shared` diff, does not mention `planner-conventions`, and that `builder-conventions` lists the
mapping. Demonstrate drift: append a byte to the live skill, confirm the scoped diff fails, restore the file, confirm
it passes.

### Task `output-extract` (builder)
TDD. `extract_test.go` first: a bare document; a document in one `yaml` fence; a `yml` fence; leading and trailing
blank lines; prose before; prose after; two fences; two documents; empty reply; a fence not closed. Then
`extract.go` with `ErrNoDocument` and `ErrExtraneousText`. Table-test that every `stub.turns` reply in
`.kairon/evals/cases/planner/*.yaml` that starts a document extracts and then `stageoutput.Parse`s (load the cases with
`gopkg.in/yaml.v3`; do not import `internal/eval`).

### Task `boundary-process` (builder)
TDD. `boundary_test.go` first, with fakes for `provenance.IssueStore`, `provenance.Router` and `failure.Issues`.
Cases: a valid canonical document (use the `interactive-return-document` reference reply) reaches `Committer.Commit`
and the store receives a body ending in a `v1` marker (AC7); a punch-out document (use the `automated-punch-out`
reply) posts a comment, then adds the fail label, in that order, and writes no issue body (AC7); a canonical document
that fails the structural guard is `OutcomeRejected`, recoverable, with nothing written; a `## Dependencies` bullet
body is rejected by `DependenciesForm`; an unresolved `#{key}` is rejected; a revision change routes to intake and
writes nothing; a punch-out with `Target.Issue == 0` is a contract error; no fake records a `gh issue create` or a
hash write other than the committer's. Same-contract test (constraint): the reference reply of each of the seven
cases that carries a document parses to a `stageoutput.Document` with `Stage == planner`, and the interactive and
automated replies are the same Go type. Then `boundary.go`, `deps_guard.go`.

### Task `boundary-chain` (builder)
TDD. `chain_test.go` first: a three-document split in dependency order with `Dependencies: none`,
`Dependencies: #{a}` and `Dependencies: #{a}, #{b}`; after `ProcessChain` the committed bodies contain
`Dependencies: #101` and `Dependencies: #101, #102` (AC8), no `## Dependencies` bullet list, and each stored hash
validates against the committed body (`provenance.Classify` reports valid); a second document that fails the guard
stops the chain and leaves the first committed; a forward reference stops the chain with `ErrUnknownPlaceholder`.
Then `chain.go`.

### Task `automated-invoker` (builder)
TDD. `run_test.go` first with a fake `Invoker`: it is always called with agent `planner-automated` (`AgentAutomated`);
the prompt names the repo and issue; the argv built for production has `--agent planner-automated`, `--no-interactive`
and no `--trust-all-tools`; a recoverable rejection is retried until `max_retries` then punches out with a comment and
a label; a punch-out reply is not retried; ANSI sequences are stripped from the reply before `Extract`. Then `run.go`.
Replace the `"planner"` literals in `internal/tui/planning_tab.go` and `internal/tui/commands.go` with
`planner.AgentInteractive` and keep their tests green. A test asserts there is no function in `internal/planner` that
maps a mode to an agent.

### Task `eval-driver` (builder)
Write `scripts/eval-planner.sh` and the `eval:planner` Taskfile target per 3.5. TDD: `evaldriver_test.go` runs
`bash scripts/eval-planner.sh --dry-run` (skip if `bash` is missing) and asserts both `kairon eval` commands,
the exact filter lists, `--sandbox`, the overlay manifest named `planner`, and that nothing under
`.kairon/evals/` is modified by a dry run. Update `docs/evaluation.md` (Planner suite section) as listed in section 2.

### Task `green-run` (builder)
Stage the new files with `git add` (explicit paths), then run the offline checks and `task eval:planner`. Iterate on
the two prompts and the skill until both halves pass, and after every change re-copy to the templates and re-run the
scoped diffs. Append one row per iteration to `ITERATIONS.md`: change made, why, the score of each half, run
directories. The last row is the GREEN verdict. If the real-model run is not possible, say so in `ITERATIONS.md`
and report it in the sentinel; do not mark GREEN.

### Task `document-planner` (documenter)
Write `docs/planner.md`: the two agents and when each is used, the shared skill and the ownership tiers, the output
boundary and its outcomes, how the agent is chosen at the call site, the eval driver, the known gaps (label answer,
interactive capture, watcher wiring). Add one link in `README.md`. Do not edit `.kairon/specs/` or `CHANGELOG.md`.

### Task `validate-all` (validator)
Read-only verification of every acceptance criterion in section 7, including a fresh read of both prompts for mode
selection and interactive-only text, and the scoped sync checks.

## 6. Validation commands (whole change)

```bash
go build ./... && go vet ./... && test -z "$(gofmt -l .)"
go test ./internal/planner/... ./internal/github/... ./internal/eval/... ./internal/tui/... -count=1
go test ./... -count=1
for f in planner.json planner-prompt.md planner-automated.json planner-automated-prompt.md; do diff -q ".kiro/agents/$f" "cmd/kairon/templates/kiro/agents/$f" || exit 1; done
diff -rq .kiro/skills/planner-shared/ cmd/kairon/templates/kiro/skills/planner-shared/
test ! -e cmd/kairon/templates/kiro/skills/planner-conventions
go run ./cmd/kairon plan parse .kairon/specs/issue-397-planner-two-single-mode-agents-shared-skill.md
bash scripts/eval-planner.sh --dry-run
task eval:planner        # real model; see ITERATIONS.md
```

`task sync:check` is the CI gate. In this worktree it fails on the agents tree because of the pipeline's uncommitted
edits to five other manifests; that failure is pre-existing and not caused by this change.

## 7. Acceptance criteria mapping

| # | Criterion | Where it is satisfied and verified |
|---|-----------|------------------------------------|
| 1 | Result is a stage-output document; no `gh issue create`, no hash | Skill section 2; `boundary-process` tests parse every reference reply and record store/gh calls; `#380` checks `gate_compliance` in `green-run` |
| 2 | Two single-mode agents, no mode branch | `interactive-agent` and `automated-agent` prompt tests; `validate-all` reads both prompts |
| 3 | Contract lives once, in the skill; both load it | `shared-skill`, `interactive-agent`, `automated-agent` tests (resources, absence of contract text in prompts) |
| 4 | Kairon-managed, init-installed, gated; `planner-conventions` not | Templates exist; `sync-gate` test and drift demonstration; `test ! -e` on the templates |
| 5 | Agent chosen at the call site; no routing component | `automated-invoker` tests; `AgentInteractive` / `AgentAutomated` constants; no mode-to-agent function exists |
| 6 | Interactive never punches out; automated owns it | Prompt tests; `#380` cases `interactive-*` and `automated-punch-out` in `green-run` |
| 7 | Boundary routes `canonical` to commit and `punch_out` to Tier 2 | `boundary-process` integration tests with fakes |
| 8 | Split emits ordered documents with resolved single-line dependencies | `boundary-chain` tests. Note: the shipped `#380` `multi-issue-split` case returns **one** document (the first issue, `Dependencies: none`) and keeps later issues as summaries, so it cannot assert resolved placeholders. The resolved form is proven by `boundary-chain`. The `#380` case still certifies the planner's split behavior in `green-run`. |
| 9 | Both agents pass the suite with the real skill; `ITERATIONS.md` RED to GREEN | `green-run`: `task eval:planner` (interactive cases on `planner`, automated cases on `planner-automated` through the overlay); staged `.kiro` is the real tracked skill |

## 8. Risks and open points

1. **Suite shape.** The issue says both agents pass the full suite. A single agent cannot pass cases written for the
   other mode, so "full suite" is realized as every case run against the agent of its mode (D5). If the maintainers
   want a per-case agent in the harness, that is a separate harness change.
2. **No stub certification.** The stub replays reference transcripts, so GREEN needs a real-model run. It may not be
   possible in an automated builder session. The spec requires honest reporting in that case.
3. **Label answer (D6).** `planner` asks the label question but cannot carry the answer. Needs a follow-up decision.
4. **Shell restriction is defense in depth, not a guarantee.** `toolsSettings.shell` keys (`autoAllowReadonly`,
   deny and allow command lists) must be confirmed against Kiro's agent-config documentation; use only keys that
   exist. The real guarantees are the boundary being the only writer in the pipeline and the `gh_log_not_contains`
   check in the eval (effective under `--sandbox`).
5. **Interactive capture and watcher wiring** are out of scope (#382 and a follow-up); the boundary and
   `RunAutomated` are ready for them and covered by tests.
6. **`kairon update` overwrites.** It force-writes template files, so a project's local edits to `planner.json`,
   `planner-automated.json`, their prompts and `planner-shared` are replaced. Project-specific planner guidance
   belongs in `planner-conventions`, which `kairon init` and `kairon update` never touch.

## 9. Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "red-baseline"
    agent: "builder"
    description: "Record RED before any prompt is rewritten: run the offline planner suite tests, attempt a real-model sandbox run of the unmodified planner, and create .kairon/iterations/planner/ITERATIONS.md with the baseline row. Record honestly if the real run cannot execute."
    dependencies: []
    acceptance_criteria:
      - ".kairon/iterations/planner/ITERATIONS.md exists with a method paragraph and a first table row marked baseline"
      - "The baseline row records the result of go test ./internal/eval -run TestPlannerSuite and the run directory of the real-model sandbox run, or states why the real run could not execute"
      - "No score in ITERATIONS.md is invented; every recorded score matches a run output or a directory under .kairon/evals/results/"
      - "No file under .kiro/, cmd/kairon/templates/ or .kairon/evals/cases/ is modified by this task"
    validation_commands:
      - "test -f .kairon/iterations/planner/ITERATIONS.md"
      - "grep -qi baseline .kairon/iterations/planner/ITERATIONS.md"
      - "go test ./internal/eval -run TestPlannerSuite -count=1"
      - "git diff --quiet HEAD -- .kairon/evals/cases .kairon/evals/rubrics cmd/kairon/templates"

  - id: "github-issue-store"
    agent: "builder"
    description: "Add a gh-backed provenance.IssueStore (Revision, Create, Update) in internal/github with an injected ghFunc, tested with a fake. Bodies travel on stdin."
    dependencies: []
    acceptance_criteria:
      - "IssueStore implements provenance.IssueStore, checked by a compile-time assertion"
      - "Revision returns the issue updatedAt from gh issue view --json updatedAt"
      - "Create sends the body on stdin via --body-file - and returns the issue number parsed from the printed URL"
      - "Update sends the body on stdin via --body-file -"
      - "An invalid repo or issue number is rejected before any gh call, and a gh error is wrapped"
      - "internal/provenance does not import internal/github"
    validation_commands:
      - "go build ./internal/github/..."
      - "go test ./internal/github/... -count=1"
      - "go test ./internal/provenance/... -count=1"
      - "go vet ./internal/github/..."

  - id: "shared-skill"
    agent: "builder"
    description: "Create the internal/planner package skeleton (doc.go, agents.go constants, test helpers) and the Kairon-managed planner-shared skill with a byte-identical template copy, driven by failing tests written first."
    dependencies: ["red-baseline"]
    acceptance_criteria:
      - ".kiro/skills/planner-shared/SKILL.md exists with front matter name planner-shared and covers restrictions, the output contract with one example of each result variant, investigate/classify/cover, the issue-body standard and the mode-neutral hard bars"
      - "The skill states the single-line Dependencies: #N, #M rule, the Dependencies: none form, the #{key} placeholder rule, and forbids a ## Dependencies bullet list"
      - "The skill forbids gh issue create and any provenance or hash key, and lists the allowed read-only shell commands"
      - "The skill contains no interactive-only text (one question, option lists, Recommended:, draft review, approval) and no mode-selection text"
      - "cmd/kairon/templates/kiro/skills/planner-shared/SKILL.md is byte-identical to the live skill"
      - "cmd/kairon/templates/ contains no planner-conventions path"
      - "internal/planner declares AgentInteractive planner, AgentAutomated planner-automated and SharedSkill planner-shared"
    validation_commands:
      - "go build ./internal/planner/..."
      - "go test ./internal/planner/... -count=1 -run 'Skill|Template'"
      - "diff -rq .kiro/skills/planner-shared/ cmd/kairon/templates/kiro/skills/planner-shared/"
      - "test ! -e cmd/kairon/templates/kiro/skills/planner-conventions"
      - "go vet ./internal/planner/..."

  - id: "interactive-agent"
    agent: "builder"
    description: "Rewrite the interactive planner agent (planner.json and planner-prompt.md) as interactive-only, loading planner-conventions and planner-shared via resources, with template copies. Build planner.json from the committed formatting and do not carry the pipeline-injected creds-agent edits."
    dependencies: ["shared-skill", "red-baseline"]
    acceptance_criteria:
      - "planner.json resources lists skill://.kiro/skills/planner-conventions/SKILL.md and skill://.kiro/skills/planner-shared/SKILL.md; it has no @creds-agent tool and no mcpServers block"
      - "planner-prompt.md has the RTCC headings and encodes the full interactive flow: investigate first, coverage map, one decision per turn, a/b/c/Other options, single-commitment option rule, single-option recommendation rule with none when no option can be chosen, constraints asked not invented, escape hatch, one confirm-defaults turn, draft review, label turn, return the document, oversize split with one-line summaries in dependency order"
      - "The prompt states that the interactive planner never returns punch_out and asks a question for any open required slot"
      - "The prompt tells the agent not to run gh issue create or write a hash, and contains no instruction that does"
      - "The prompt contains no output-contract field list and no issue-body spine (those live in the skill) and no mode-selection phrase"
      - "Live and template planner.json and planner-prompt.md are byte-identical; no other dirty .kiro/agents/*.json file is staged or modified by this task"
    validation_commands:
      - "go test ./internal/planner/... -count=1 -run 'Interactive'"
      - "diff -q .kiro/agents/planner.json cmd/kairon/templates/kiro/agents/planner.json"
      - "diff -q .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md"
      - "grep -q 'planner-shared' .kiro/agents/planner.json"
      - "! grep -q 'creds-agent' .kiro/agents/planner.json"
      - "! grep -q 'schema_version' .kiro/agents/planner-prompt.md"

  - id: "automated-agent"
    agent: "builder"
    description: "Create the planner-automated agent (manifest and prompt) as automated-only with template copies: never interview, best-effort grounded inference with recorded evidence, then a canonical document or a punch_out naming every unresolvable slot."
    dependencies: ["shared-skill", "red-baseline"]
    acceptance_criteria:
      - "planner-automated.json names the agent planner-automated, uses tools read and shell only, and lists both skills in resources as skill:// URIs"
      - "planner-automated-prompt.md has the RTCC headings and states that the agent never asks a question, never offers options and never writes a draft for approval"
      - "The prompt requires an inferences entry (slot, inferred, evidence) for every inference and defines reasonable inference as one a competent engineer reading the same issue and repository would reach"
      - "The prompt requires result punch_out with an unresolved list naming every unresolvable required slot and its reason, and forbids a partial canonical document"
      - "The prompt contains none of: one-question-per-turn, option lists, Recommended:, draft review or approval, a label question, and no mode-selection phrase"
      - "The prompt does not restate the output contract field list or the issue-body spine"
      - "Both manifests list the same planner-shared skill URI"
      - "Live and template copies of both files are byte-identical"
    validation_commands:
      - "go test ./internal/planner/... -count=1 -run 'Automated'"
      - "diff -q .kiro/agents/planner-automated.json cmd/kairon/templates/kiro/agents/planner-automated.json"
      - "diff -q .kiro/agents/planner-automated-prompt.md cmd/kairon/templates/kiro/agents/planner-automated-prompt.md"
      - "grep -q 'planner-shared' .kiro/agents/planner-automated.json"
      - "! grep -qiE 'one question|Recommended:|draft review' .kiro/agents/planner-automated-prompt.md"

  - id: "sync-gate"
    agent: "builder"
    description: "Register the planner-shared skill in the template-sync gate (Taskfile.yml sync:check) and in the builder-conventions sync mapping, keeping planner-conventions out. Demonstrate that drift fails the gate."
    dependencies: ["shared-skill"]
    acceptance_criteria:
      - "Taskfile.yml sync:check contains a diff -rq for .kiro/skills/planner-shared/ against cmd/kairon/templates/kiro/skills/planner-shared/"
      - "Neither Taskfile.yml nor the templates mention planner-conventions"
      - ".kiro/skills/builder-conventions/SKILL.md lists the planner-shared mapping row and sync command"
      - "Appending a byte to the live skill makes the scoped diff fail, and restoring the file makes it pass"
    validation_commands:
      - "go test ./internal/planner/... -count=1 -run 'SyncGate'"
      - "grep -q 'planner-shared' Taskfile.yml"
      - "! grep -q 'planner-conventions' Taskfile.yml"
      - "grep -q 'planner-shared' .kiro/skills/builder-conventions/SKILL.md"
      - "diff -rq .kiro/skills/planner-shared/ cmd/kairon/templates/kiro/skills/planner-shared/"

  - id: "output-extract"
    agent: "builder"
    description: "Add Extract in internal/planner: return the stage-output document text from an agent reply (bare, or in exactly one yaml fence) and reject any other surrounding text, driven by table tests that include every reference document in the #380 planner cases."
    dependencies: ["shared-skill"]
    acceptance_criteria:
      - "Extract accepts a bare document, a document in one yaml or yml fence, and surrounding blank lines"
      - "Extract returns ErrExtraneousText for prose before or after, a second fence or a second document, and ErrNoDocument for an empty reply or an unclosed fence"
      - "Every stub.turns reply that begins a document in .kairon/evals/cases/planner/*.yaml extracts and then passes stageoutput.Parse"
      - "internal/planner does not import internal/eval"
    validation_commands:
      - "go test ./internal/planner/... -count=1 -run 'Extract'"
      - "go vet ./internal/planner/..."
      - "test -z \"$(gofmt -l internal/planner)\""

  - id: "boundary-process"
    agent: "builder"
    description: "Implement the planner output boundary Process: extract, parse, route on result, send canonical through the guard chain (DefaultChain plus a DependenciesForm guard) and the provenance Commit, send punch_out to failure.PunchOut. Prove it with fake-backed integration tests."
    dependencies: ["output-extract"]
    acceptance_criteria:
      - "A valid canonical document reaches provenance Committer.Commit and the store receives a body that ends with a v1 provenance marker"
      - "A punch_out document posts the explanatory comment and then adds the fail-state label, in that order, and writes no issue body; a punch_out with no target issue is a contract error"
      - "A canonical document that fails a guard returns a rejected, recoverable outcome with the FailureContext and nothing written"
      - "DependenciesForm rejects a ## Dependencies bullet list, a missing Dependencies: line and an unresolved #{key} placeholder"
      - "A revision change routes to intake and writes nothing"
      - "The reference reply of every #380 case that carries a document parses to a stageoutput.Document with stage planner, and the interactive and automated replies are the same Go type"
      - "Process contains no mode parameter and never calls gh or writes a hash itself"
    validation_commands:
      - "go test ./internal/planner/... -count=1 -run 'Boundary|DependenciesForm|SameContract'"
      - "go test ./internal/guard/... ./internal/provenance/... ./internal/failure/... -count=1"
      - "go vet ./internal/planner/..."
      - "! grep -rn 'gh issue create' internal/planner --include=*.go --exclude=*_test.go"

  - id: "boundary-chain"
    agent: "builder"
    description: "Add ProcessChain: commit an ordered multi-issue split through provenance.CommitChain with guard.Run as the Admit function, so #{key} dependencies resolve into the single-line Dependencies: #N form before hashing."
    dependencies: ["boundary-process"]
    acceptance_criteria:
      - "A three-document split with Dependencies: none, #{a} and #{a}, #{b} commits in order and the stored bodies contain Dependencies: #101 and Dependencies: #101, #102"
      - "No committed body contains a ## Dependencies bullet list, and each stored hash classifies as valid over the committed body"
      - "A document that fails the guard chain stops the chain and the earlier documents stay committed"
      - "A forward reference stops the chain with provenance.ErrUnknownPlaceholder"
    validation_commands:
      - "go test ./internal/planner/... -count=1 -run 'Chain'"
      - "go test ./internal/provenance/... -count=1"
      - "go vet ./internal/planner/..."

  - id: "automated-invoker"
    agent: "builder"
    description: "Add RunAutomated, the unattended invocation site that always selects planner-automated, runs it through an injected Invoker with failure.Runner retry, and passes the reply to Process; replace the planner literals at the interactive launch sites with planner.AgentInteractive."
    dependencies: ["boundary-process", "github-issue-store"]
    acceptance_criteria:
      - "RunAutomated always invokes agent planner-automated; the production argv is kiro-cli chat --agent planner-automated --no-interactive with no --trust-all-tools"
      - "The reply has ANSI sequences stripped before Extract"
      - "A recoverable rejection is retried up to max_retries with a new attempt each time, then punches out with a comment and a label; a punch_out reply is terminal and is not retried"
      - "internal/tui/planning_tab.go and internal/tui/commands.go use planner.AgentInteractive instead of a string literal"
      - "No function in internal/planner maps a mode to an agent name, and no separate mode-routing component exists"
    validation_commands:
      - "go test ./internal/planner/... -count=1 -run 'Run|Invoker|NoModeRouting'"
      - "go build ./... "
      - "go test ./internal/tui/... -count=1"
      - "! grep -n '\"planner\"' internal/tui/planning_tab.go internal/tui/commands.go | grep -v '//'"

  - id: "eval-driver"
    agent: "builder"
    description: "Add scripts/eval-planner.sh and the eval:planner Taskfile target that run the interactive cases against planner and the automated cases against planner-automated through an evals-dir overlay, with a --dry-run mode, and update the Planner suite section of docs/evaluation.md."
    dependencies: ["interactive-agent", "automated-agent", "sync-gate"]
    acceptance_criteria:
      - "scripts/eval-planner.sh --dry-run prints both kairon eval commands with --sandbox and the exact name filters for the four interactive cases and the three automated cases, and writes nothing under .kairon/evals/"
      - "The automated half uses an overlay whose agents/planner.json is the planner-automated manifest with only name changed to planner, plus the planner-automated prompt; the shared skill is the tracked .kiro/skills/planner-shared, not a copy"
      - "The script exits non-zero if either half fails and copies new result directories back under .kairon/evals/results/"
      - "Taskfile.yml has an eval:planner target that runs the script"
      - "docs/evaluation.md Planner suite section describes the two agents, which cases run against which, and that the mode signal is the agent name"
      - ".kairon/evals/cases/planner, .kairon/evals/rubrics/planner.yaml and internal/eval/planner_suite_test.go are unchanged"
    validation_commands:
      - "bash -n scripts/eval-planner.sh"
      - "bash scripts/eval-planner.sh --dry-run"
      - "go test ./internal/planner/... -count=1 -run 'EvalDriver'"
      - "go test ./internal/eval -run TestPlannerSuite -count=1"
      - "git diff --quiet HEAD -- .kairon/evals/cases .kairon/evals/rubrics internal/eval/planner_suite_test.go"

  - id: "green-run"
    agent: "builder"
    description: "Stage the new files, run the offline checks and the real-model task eval:planner, iterate on the two prompts and the shared skill until both halves pass, re-syncing templates after each change, and log every iteration in ITERATIONS.md ending with the GREEN verdict. If the real run cannot execute, say so and do not mark GREEN."
    dependencies: ["eval-driver", "boundary-chain", "automated-invoker"]
    acceptance_criteria:
      - "ITERATIONS.md has one row per iteration: change, reason, interactive-half result, automated-half result, run directories"
      - "The final row is GREEN only if both halves of task eval:planner passed with the real model; otherwise it states that the real-model run was not possible and why"
      - "The agents were evaluated with the tracked .kiro/skills/planner-shared staged from the repository, not a copy inlined in a fixture"
      - "After the last prompt or skill change, the scoped template diffs all pass"
      - "No score in ITERATIONS.md is invented"
    validation_commands:
      - "go test ./internal/eval -run TestPlannerSuite -count=1"
      - "go test ./internal/planner/... -count=1"
      - "diff -rq .kiro/skills/planner-shared/ cmd/kairon/templates/kiro/skills/planner-shared/"
      - "for f in planner.json planner-prompt.md planner-automated.json planner-automated-prompt.md; do diff -q .kiro/agents/$f cmd/kairon/templates/kiro/agents/$f || exit 1; done"
      - "grep -qi 'GREEN\\|not possible' .kairon/iterations/planner/ITERATIONS.md"

  - id: "document-planner"
    agent: "documenter"
    description: "Write docs/planner.md describing the two single-mode agents, the shared skill and its ownership tier, the output boundary and its outcomes, how the agent is chosen at the call site, the eval driver, and the known gaps; add a link from README.md."
    dependencies: ["green-run"]
    acceptance_criteria:
      - "docs/planner.md explains planner versus planner-automated, the planner-shared skill (Kairon-managed, in the sync gate) versus planner-conventions (project-owned, never templated), and the boundary routing on result"
      - "docs/planner.md lists the known gaps: label answer not carried, interactive reply not captured, watcher not wired"
      - "README.md links to docs/planner.md"
      - "No file under .kairon/specs/ other than this issue spec, and not CHANGELOG.md, is modified"
    validation_commands:
      - "test -f docs/planner.md"
      - "grep -q 'docs/planner.md' README.md"
      - "grep -q 'planner-shared' docs/planner.md"
      - "git diff --quiet HEAD -- CHANGELOG.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue 397 read-only: build, vet, format, all tests, scoped template diffs, plan parse, dry-run eval driver, a fresh read of both prompts for mode selection and interactive-only text, and the ITERATIONS.md honesty check."
    dependencies: ["document-planner"]
    acceptance_criteria:
      - "go build, go vet, gofmt and go test ./... all pass"
      - "The scoped template diffs for the shared skill and the four planner agent files pass; templates contain no planner-conventions"
      - "planner-prompt.md and planner-automated-prompt.md contain no mode-selection logic, and planner-automated-prompt.md contains no interactive-only instruction"
      - "Both manifests list planner-shared as a skill:// resource and neither prompt contains the output contract or the body spine"
      - "The boundary tests show a valid canonical document reaching the provenance commit and a punch_out reaching the failure contract (comment, then label)"
      - "ITERATIONS.md records RED first and does not claim a real-model GREEN that no run directory supports"
      - "Only intended files changed: no dirty pipeline manifest other than planner.json is in the diff, and .kairon/specs/ other than this spec is untouched"
    validation_commands:
      - "go build ./... && go vet ./... && test -z \"$(gofmt -l .)\""
      - "go test ./... -count=1"
      - "diff -rq .kiro/skills/planner-shared/ cmd/kairon/templates/kiro/skills/planner-shared/"
      - "test ! -e cmd/kairon/templates/kiro/skills/planner-conventions"
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-397-planner-two-single-mode-agents-shared-skill.md"
      - "bash scripts/eval-planner.sh --dry-run"
      - "git diff --name-only HEAD -- .kairon/specs | grep -v issue-397 | wc -l | grep -qx 0"
```
