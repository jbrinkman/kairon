# Planner

The planner turns a request into a requirements document. It is two single-mode agents that share one
Kairon-managed skill, plus a deterministic output boundary that is the only thing allowed to write the result to
GitHub. This page describes the agents, the skill and its ownership tier, the boundary and its outcomes, how a call
site picks an agent, the eval driver, and what is not finished yet.

The design background is `.kairon/research/maturity-model/planner-requirements.md` (§1, §3, §3.5, §4, §10 and §14). The
boundary is glue over [`internal/stageoutput`](stage-output.md), [`internal/guard`](guards.md),
[`internal/provenance`](provenance.md) and [`internal/failure`](failure-contract.md), and adds no contract of its own.
The code is in [`internal/planner`](../internal/planner).

## The two agents

Mode is a fact the caller already knows: either a person is present or nobody is. So the caller picks the agent by
name, and neither prompt branches on mode. There is no "if you are running interactively" text in either prompt.

| | `planner` | `planner-automated` |
|---|-----------|---------------------|
| Used when | A person is present and plans a new issue. | The run is unattended and canonicalizes an existing issue. |
| Manifest | `.kiro/agents/planner.json` | `.kiro/agents/planner-automated.json` |
| Prompt | `.kiro/agents/planner-prompt.md` | `.kiro/agents/planner-automated-prompt.md` |
| Tools | `read`, `write` (limited to `.worktrees/planning-*` and the per-session files `.kairon/sessions/*.document.yaml` and `.kairon/sessions/*.label`), `shell`, `web_search`, `web_fetch` | `read`, `shell` only |
| Asks questions | Yes: one decision per turn, lettered options with one recommendation. | Never. |
| Fills a gap by | Asking the person, or a default the person accepted. | Grounded inference with the evidence recorded in `inferences`. |
| May return | `result: canonical` only. | `result: canonical` or `result: punch_out`. |
| Review gate | Confirm-defaults turn, then a draft review the person approves. | None. The reply is the document from its first line. |

What each prompt holds is what is specific to its mode:

- `planner-prompt.md` holds the interactive-only rules: investigate before asking, the coverage map, one decision
  per turn, `a)`/`b)`/`c)`/Other options, the single-commitment option rule (items may be joined by *and*, never by
  *or*), the single-option recommendation rule, constraints asked and never invented, the escape hatch, the
  confirm-defaults turn, the draft review, the label question, and the oversize split. It states that the agent never
  returns `punch_out`: every open required slot becomes a question.
- `planner-automated-prompt.md` holds the automated-only rules: never ask, never offer options, never draft for
  approval, best-effort inference where "reasonable" means a competent engineer reading the same issue and repository
  reaches the same conclusion, and a `punch_out` whose `unresolved` list names every unresolvable required slot with a
  reason. It forbids a partial `canonical` document. A coin-flip among materially different options is not a
  reasonable inference, so that slot is punched out.

Neither prompt restates the output contract or the issue-body standard. Both point at the shared skill.

## The shared skill

`planner-shared` (`.kiro/skills/planner-shared/SKILL.md`) holds everything that is the same in both modes:

- the restrictions: no GitHub write of any kind, no provenance marker or hash in the output, no writes to project
  files, a read-only shell allow-list, and declining to implement
- the output contract: one stage-output document and nothing else (optionally inside one `yaml` fence), with one
  complete example of each result variant
- how to investigate, classify (Feature, Bug or Refactor) and cover the request
- the issue-body standard, including the Dependencies rule
- the mode-neutral hard bars: no ungrounded assumption, no unquantified vague terms in acceptance criteria, no design
  outside Constraints, and no re-deriving what the repository already states

The skill describes both result variants. Which variant an agent may emit is stated in that agent's own prompt, and
where the two differ, the prompt decides. The skill carries no interactive-only text and no mode-selection text,
because loading it into the automated agent would otherwise load those rules into it too.

Both manifests load the skill through `resources` as a `skill://` URI. It is a runtime pull. There is no build or
compose step.

### The Dependencies rule

The watcher parses the single-line form only (see [Issue Dependencies](../README.md#issue-dependencies)), so the skill
requires `Dependencies: #12, #15` or `Dependencies: none`. A `## Dependencies` header followed by bullets is never
correct. In a multi-issue split, a dependency on an issue not yet created is written `#{key}` on that same single
line; the boundary resolves it to a number before hashing.

## Ownership tiers

Two planner skills exist, and they have different owners.

| Skill | Owner | In `templates/` | In `task sync:check` | Overwritten by `kairon update` |
|-------|-------|-----------------|----------------------|--------------------------------|
| `planner-shared` | Kairon | Yes (`cmd/kairon/templates/kiro/skills/planner-shared/`) | Yes | Yes |
| `planner-conventions` | The project | No | No | Never touched |

`planner-shared` is Kairon-managed. `kairon init` installs it and `kairon update` force-writes it, along with the four
planner agent files. `task sync:check` diffs the live skill against its template, so drift fails the gate. A project's
local edits to `planner.json`, `planner-automated.json`, their prompts or `planner-shared` are replaced on update.

`planner-conventions` is project-owned. It is never templated, never written or overwritten by `init` or `update`, and
never in the sync gate. Put project-specific planner guidance there. Both manifests load it alongside the shared
skill.

## Choosing the agent at the call site

The agent name is the only mode-dependent value in the package, and it is chosen where the mode is known:

```go
const (
	AgentInteractive = "planner"           // a person is present
	AgentAutomated   = "planner-automated" // unattended intake
	SharedSkill      = "planner-shared"
)
```

- The interactive launch sites in `internal/tui` (the planning tab's ACP client and the `kiro-cli chat --classic`
  planning command) pass `planner.AgentInteractive`.
- `planner.AutomatedCommand` is the unattended invocation site. It always selects `planner.AgentAutomated`.

There is no function that maps a mode to an agent, and no separate mode-routing component. The boundary itself is
mode-agnostic.

## The output boundary

The agent never writes to GitHub. It returns a stage-output document, and `planner.Boundary` validates it and hands it
to the commit step or to the failure contract. The only writers in the pipeline are `provenance.Committer` and
`failure.PunchOut`.

### Process

`Boundary.Process(ctx, Request)` handles one agent reply. It has no mode parameter.

1. **Extract.** `planner.Extract` accepts the whole reply, or the content of one `yaml`/`yml` fence that is the whole
   reply. Prose before or after, a second fence or a second document returns `ErrExtraneousText`; an empty reply or an
   unclosed fence returns `ErrNoDocument`. Either way the reply is rejected as recoverable (guard name `extract`) and
   nothing is written.
2. **Parse.** `stageoutput.Parse` applies the strict schema. A document that does not parse goes through the guard
   chain so the structural guard produces the `FailureContext`, and is rejected.
3. **Route on `result`.**

| `result` | Path | Outcome |
|----------|------|---------|
| `canonical` | The guard chain runs: `guard.DefaultChain`, then `DependenciesForm`. On a pass, `Committer.Commit` writes the body with its `v1` provenance marker, once. | `OutcomeCommitted`, `OutcomeRoutedToIntake` or `OutcomePunchedOut` |
| `canonical` (a guard fails) | Nothing is written. The `FailureContext` is returned with a `Recoverable` disposition so the caller can retry. | `OutcomeRejected` |
| `punch_out` | The `unresolved` list becomes the `FailureContext` (guard `intake`, attempt 1 of 1), and `failure.PunchOut` posts the explanatory comment, then adds the fail-state label. The issue body is never written. | `OutcomePunchedOut`, `Unrecoverable` |

A `punch_out` document needs `Target.Issue > 0`. On the create path there is no issue to comment on, so it returns
`ErrNoPunchOutTarget` (which matches `guard.ErrInvalid`) instead of being swallowed.

### Outcomes

| Outcome | Meaning | Written to GitHub |
|---------|---------|-------------------|
| `OutcomeCommitted` | The body and its provenance marker were written once. `Result.Number` and `Result.Hash` are set. | The issue body |
| `OutcomeRejected` | The reply failed extraction or a guard. `Result.Failure` says why. | Nothing |
| `OutcomeRoutedToIntake` | The issue changed under the planner (its revision no longer matches `ObservedRevision`). | Nothing |
| `OutcomePunchedOut` | A human was told why the pipeline stopped: a comment, then the label. Also reported for a revision conflict the committer had to punch out. | A comment, then a label |

A non-nil error means `Process` could not run at all (an invalid attempt, a missing collaborator, or a GitHub call that
failed). The `Result` is then the zero value.

### DependenciesForm

The merged structural guard only checks that a Dependencies section exists. `DependenciesForm` is a planner-side guard
appended to the chain. It rejects a canonical body that has:

- no `Dependencies:` line, or more than one
- a `## Dependencies` heading followed by a bullet list, or a bullet that carries `Dependencies:`
- an unresolved `#{key}` placeholder on the line
- a value that is neither `none` nor a comma-separated list of `#<number>`

Fenced code is ignored, so a body may show an example of a wrong form. A `punch_out` document passes, because it has no
body.

### Splitting into several issues

`Boundary.ProcessChain` commits an ordered multi-issue split. Each `ChainRequest` carries a `Key`, the reply and a
target, in dependency order. Every reply is extracted and parsed before anything is written. The documents then go to
`provenance.CommitChain`, which resolves each `#{key}` into `Dependencies: #N` from the numbers committed so far, runs
the guard chain on the substituted document, and only then hashes and writes it.

A guard rejection or a revision conflict stops the chain without an error and reports where (`StoppedAt`). Issues
already committed stay committed. A forward reference or an unknown key stops it with `provenance.ErrUnknownPlaceholder`.
A reply that is not exactly one `canonical` document fails the call with `guard.ErrInvalid` before anything is written.

### Unattended runs

Unattended runs are intake: the watcher boundary starts one for a `kairon` issue whose body has no valid provenance
marker, and the TUI command `validate <issue>` starts one by hand (#409). Both go through the agent manager
(`internal/agent/intake.go`), which runs each attempt like a krew-lead run: one process, started without waiting, with
its own log at `.kairon/logs/issue-<n>-intake.log`, and a monitor goroutine that acts on the exit. The planner package
supplies the pieces of an attempt:

- `planner.AutomatedCommand(prompt, dir)` builds `kiro-cli chat --agent planner-automated --no-interactive <prompt>`.
  It never passes `--trust-all-tools`: the manifest's `allowedTools` bound what an unattended run can do.
- `planner.AutomatedPrompt(repo, issue, attempt, previous)` names the issue and the attempt and, on a retry, the
  previous guard, what was expected and what was found.
- The reply goes through `planner.StripANSI` and then `Boundary.Process`, with the issue revision the run read before its
  first attempt, so the commit step detects a concurrent edit.
- `planner.RejectionFailure` and `planner.InvokeFailure` turn a rejected reply or a failed process into the
  `FailureContext` to retry on.

The manager retries a recoverable rejection or a failed process up to `max_retries` more times, then punches out once
with a comment and the label. A `punch_out` reply is terminal: `Process` has already commented and labeled. A commit,
or a revision conflict (nothing written), ends the run. See [Watcher boundary: intake](boundary.md#intake) for how the
watcher avoids duplicate runs and loops.

The `gh`-backed `provenance.IssueStore` is `github.IssueStore` in `internal/github`. Bodies travel on stdin, never in
argv.

## Committing an interactive session

A terminal session's output cannot be captured, so an interactive session hands its document to the boundary through a
file. The handoff is `finishPlanning` in `internal/tui/planning_handoff.go`.

### The per-session files and the prompt lines

When a planning session starts or resumes, Kairon picks two paths for it, both keyed by the planning session ID:

- `.kairon/sessions/<session-id>.document.yaml`, the approved document.
- `.kairon/sessions/<session-id>.label`, the person's answer to the trigger-label question.

A session ID that is empty, contains a path separator or contains `..` has no paths.

The first two lines of the agent's initial prompt are `Document path: <path>` and
`Label answer path: .kairon/sessions/<session-id>.label`. A resumed session adds one sentence saying the document file
may already hold the last document and should be read first. The person's own request follows after a blank line.

After the label turn, the interactive `planner` writes the label file first, then the document file, both with its
`write` tool, and still returns the same document as its reply. The label file holds the single word `yes` or `no` and
nothing else. The `planner.json` manifest allows these two writes; it allows no other write outside
`.worktrees/planning-*`. If a prompt has no `Document path:` line, the agent writes nothing and only returns the
document.

The answer is not a field of the document. It travels in the label file, outside the document, so the stage-output
schema is unchanged and `schema_version` stays 1.

### When the handoff runs

Both front ends call the same function. It does not branch on which front end ran the session.

| Front end | The handoff runs when | Document path announced |
|-----------|-----------------------|-------------------------|
| Classic (`plan classic`) | The `kiro-cli chat --classic` process exits, including a non-zero exit, because the person may quit after approving. | In the argv prompt of every launch, new or resumed. |
| ACP planning tab (`plan`) | An agent turn ends: the stream `done` event, or a completed response that is not an error. A turn that leaves no file does nothing. | With the first message sent over each ACP connection. A tab restored with earlier messages, or a new connection, announces it again. The tab shows the person's own text. |

Classic reuses only sessions created for classic. A session that belongs to an ACP tab names a tab, not a terminal
session, so `plan classic` creates a new one. If it cannot create a session, it stops with an error line, because a
session without an ID would lose its document.

### Results

If the file does not exist, nothing happens: no console line, no dialog and no GitHub call. If it exists, its contents
go to `Boundary.Process` on the create path (`Target.Number` is 0), off the UI goroutine, with a 2-minute timeout.
`Request.Title` stays empty, so the boundary takes the first `# ` heading of the body. Four results are possible:

| Result | What the console shows | The file |
|--------|------------------------|----------|
| `OutcomeCommitted` | A success line with the new issue number and `https://github.com/<repo>/issues/<n>`. | Deleted. |
| `OutcomeRejected` (including an empty file or text that is not one stage-output document, which fails the `extract` guard, and a document that opens with `schema_version:` but is not valid YAML, which fails the `structural` guard) | A line naming the failing guard and the kept file path, and the rejection dialog. Nothing is written to GitHub. | Kept. |
| A `Process` error (a failed `gh` call, a cancelled or timed-out context, or a `punch_out` document, which returns `ErrNoPunchOutTarget` on the create path) | An error line naming the cause. | Kept. |
| No file | Nothing. | Not applicable. |

A committed issue also reports the label outcome, described in [The trigger label](#the-trigger-label). The label
file is deleted together with the document file.

An unreadable file, or a session with no configured repository, also gives an error line and keeps the file.

The rejection dialog, titled "Planning document rejected", shows the guard name, the expected and actual text from the
failure, and the kept file path. It offers two choices:

- **Resume planning** (`R` or Enter) reopens the session the document came from, in the same front end, and gives the
  agent the failure: the guard, what was expected and what was found, and the path. The agent revises the document
  with the person if needed, rewrites the file and returns it. Classic launches `kiro-cli chat --classic` again for the
  same session. ACP makes the originating tab active and sends the failure as a message. If that tab is closed, busy or
  read-only, the console shows an error line and the file stays.
- **Dismiss** (`D` or Esc) closes the dialog and leaves the file for a later resume.

The dialog never touches the file. Neither choice writes anything to GitHub.

### The trigger label

The watcher only picks up issues that carry the trigger label, so the planner asks the person whether to add it. The
agent never applies it. Kairon's handoff makes the one GitHub label call, after the boundary has committed the issue.
The boundary is unchanged and the automated `planner-automated` path never asks the question.

The handoff reads the label file at the same moment it reads the document, so a rewrite while a send is in flight
cannot change what that send does. The label is applied when all of these hold:

- the outcome is `OutcomeCommitted`
- the label file says `yes` (case and surrounding whitespace do not matter)

Then the handoff makes one `AddLabel` call on the committed issue number, using the `label` value from
`.kairon/config.yaml`. It is never applied, and no label call is made, when the answer is `no`, when there is no label
file, when the boundary rejects the document, when `Process` returns an error, or for any other outcome.

What the console shows for a committed issue:

| Case | Console |
|------|---------|
| `yes` and the call succeeds | A success line: ``Added the label `<label>` to issue #<n>.`` |
| `yes` and the call fails, does not finish within 30 seconds, or no GitHub connection or no label name is configured | An error line with the issue number, the label name and the error. The issue is not created again. Add the label by hand for the watcher to pick it up. A call that timed out is abandoned, not stopped, so it may still add the label. |
| The file is not a regular file, cannot be read, is longer than 16 bytes, or says anything other than `yes` or `no` | An error line with the issue number and the problem. No label is applied. |
| `no`, or no file | Nothing. |

After a commit the handoff deletes the label file with the document file, so a stale `yes` cannot label the next issue
of the same session. If the label file cannot be removed, the console says so. If the agent wrote a newer document while
the send was in flight, the label file is kept for that document's send. The label file is also kept when it was
created or rewritten after the document was read, because the agent writes the answer before the document it belongs to.

### The duplicate-issue guard

Only a commit deletes the file, so a later session end cannot create a second issue from the same approved document.
While a send for a session is in flight, a second session end for it does nothing. If the file cannot be removed after
a commit, the console says the issue exists and that the file must be deleted by hand to avoid a duplicate.

Because rejected and errored documents are kept, the next session end for the same session sends the file again. A
document the person revised and rewrote is therefore picked up without further steps.

### Removing `plan classic`

The handoff is front-end-agnostic. Each front end passes a session ID and its own Resume function, and `internal/tui`
has one call to `Boundary.Process`. Removing `plan classic` deletes only its launch site: `handlePlanSubprocess`,
`launchClassicPlanning`, `classicPlanningCmd`, `classicResume` and the `execDoneMsg` case. `finishPlanning` is not
touched.

## Eval driver

The planner eval suite from #380 (`.kairon/evals/rubrics/planner.yaml` and `.kairon/evals/cases/planner/*.yaml`) has
eleven cases. The harness keys a suite by its directory name, so every case runs as an agent called `planner`. The
suite is therefore run in two halves, each against the agent of its mode:

| Half | Cases | Agent |
|------|-------|-------|
| Interactive | `interactive-single-decision`, `interactive-option-rules`, `interactive-return-document`, `multi-issue-split`, `interactive-document-handoff`, `interactive-path-only-message`, `interactive-decline-implementation` | `planner`, unchanged |
| Automated | `automated-best-effort-fill`, `automated-bug-classification`, `automated-punch-out`, `automated-dependencies-line` | `planner-automated`, run as `planner` through an `--evals-dir` overlay |

The overlay is a copy of `.kairon/evals` without `results/`, in which `agents/planner.json` is the
`planner-automated` manifest with only `name` changed to `planner`, plus the `planner-automated` prompt. The shared
skill is not copied. The harness stages the git-tracked `.kiro/skills/planner-shared` from the repository, so the real
skill is what is evaluated. Run `git add` on new skill and agent files before an eval, or the agent is evaluated
without them.

```bash
task eval:planner                  # run both halves (needs kairon on PATH, a model backend and a container runtime)
task eval:planner -- --dry-run     # print both commands and the overlay; run nothing, write nothing
bash scripts/eval-planner.sh --dry-run
```

`scripts/eval-planner.sh` always passes `--sandbox`, because an agent that is not yet correct can run
`gh issue create`. Both halves always run. New result directories from the automated half are copied back under
`.kairon/evals/results/`. The script exits non-zero if either half fails. Override the binary with `KAIRON_BIN` and the
evals directory with `KAIRON_EVALS_DIR`. See [evaluation.md](evaluation.md) for the harness.

The stub backend replays the reference transcripts whatever the prompt says, so it cannot certify a prompt. The offline
checks are `go test ./internal/eval -run TestPlannerSuite` (the suite is well-formed, legacy transcripts fail it) and
`go test ./internal/planner/...` (static prompt and skill tests, the boundary, the chain and the automated invoker,
against fakes). The RED to GREEN log is `.kairon/iterations/planner/ITERATIONS.md`.

## Known gaps

- **Real-model eval certification is outstanding.** Neither a RED baseline nor a GREEN run exists from a real model.
  `kiro-cli` is not authenticated inside the sandbox container, so every case ends `agent failed` with no output. The
  0.0% the harness reports for those runs is an infrastructure failure, not a measurement of any prompt, and is not
  recorded as a score. `ITERATIONS.md` says NOT GREEN. To finish it, run `task eval:planner` where `kiro-cli` is logged
  in inside the sandbox container, and record one row per iteration.
- **The split case is one document.** The shipped `multi-issue-split` eval case returns one document (the first issue)
  and keeps later issues as one-line summaries, so it cannot assert resolved `#{key}` placeholders. `ProcessChain`
  proves the resolved form in unit tests.
- **The shell restriction is defense in depth.** `toolsSettings.shell` and the shared skill's allow-list limit what an
  agent runs, but the real guarantees are that the boundary is the only writer and that the eval's
  `gh_log_not_contains` check holds under `--sandbox`.
