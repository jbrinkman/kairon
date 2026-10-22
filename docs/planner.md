# Planner output

The planner agent no longer creates GitHub issues. Its final reply is a [stage-output document](stage-output.md).
A deterministic Go boundary, [`internal/planner`](../internal/planner), checks that document, then either writes the
issue through the [provenance step](provenance.md) or routes a punch-out into the [failure contract](failure-contract.md).
The agent never touches GitHub and never writes the provenance marker or hash.

> **Certification status.** The rewritten prompt has **not** been certified by the live planner eval suite.
> Acceptance criterion 8 of #381 ("the new prompt passes the planner eval suite, iterations recorded") is **NOT MET**.
> See [What is certified and what is not](#what-is-certified-and-what-is-not).

## The output contract

The whole final reply is one stage-output document, with no text before or after it. The planner shows the unfenced
form. The boundary also accepts exactly one `yaml` fence around the document.

```yaml
schema_version: 1
stage: planner
result: canonical        # or punch_out
source:
  kind: document         # interactive; automated uses kind: github_issue
  ref: "interactive-request"
generated: 2026-10-22T14:36:43Z
body: |
  # Short issue title

  ## Problem Statement
  ...
```

| Field | Rule |
|-------|------|
| `result: canonical` | Carries `body`. This is the issue text. |
| `result: punch_out` | Carries `unresolved`, a list of `{slot, reason}`, and no `body`. Automated mode only. |
| `source` | Interactive: `kind: document`, `ref: "interactive-request"`. Automated: `kind: github_issue`, `ref` is the issue number as a string. |
| `generated` | A real RFC 3339 UTC time from `date -u +%Y-%m-%dT%H:%M:%SZ`. |
| `inferences` | Optional, automated mode. One `{slot, inferred, evidence}` per value filled by inference. `evidence` is a quote or a `path:line`. |

The body never contains a `<!-- planner-provenance:` line, a hash, `gh issue create` or a label.

The body standard, in order: an H1 title, `## Problem Statement`, `## Context`, `## Scope` with `### In Scope` and
`### Out of Scope`, `## Acceptance Criteria` (numbered, each with its own `Verify:` line), optional `## Constraints`,
and `## Dependencies` holding one `Dependencies:` line. There are at most 6 In Scope entries and 8 criteria. Both
modes produce this same document type; the mode changes how the planner gets there, not what it returns.

## The two modes

| | Interactive | Automated |
|---|---|---|
| Who is present | A human | Nobody |
| Selected by | The first line is anything other than the intake line | The first line is `AutomatedIntakeLine` (see [D4](#d4-the-mode-signal)) |
| Questions | One decision per turn; options `a)`/`b)`/`c)` plus `Other`, each option one commitment, one recommendation or none | Never asks, never offers options, never drafts for approval |
| Result | `canonical` only, and only after the human approves the draft | `canonical` when every required slot is filled, otherwise `punch_out` |
| Documents | One per reply; a split yields one document per turn | Exactly one, never a split |
| Gaps | Asked | Filled from evidence actually present and recorded in `inferences`; unresolvable slots are punched out |

## The output boundary

`planner.Boundary.Handle` takes the final reply of each planner turn and does the following, in order. Nothing is
written to GitHub until every reply has passed steps 1 to 3.

1. **Extract.** The reply must be exactly one YAML document, bare or inside one `yaml` fence. Any other text is a
   rejection.
2. **Mode contract.** Interactive mode rejects a `punch_out`, since no issue exists to comment on. Automated mode
   rejects a reply count other than one, a `source` that is not `github_issue`, a `ref` that is not the issue being
   planned, and a stage other than `planner`. Both modes reject a body that contains the provenance marker prefix.
3. **Guard chain.** `guard.Run` with `guard.DefaultChain` runs on every document ([guards](guards.md)).
4. **Punch-out** (automated). `failure.PunchOut` posts the comment first and the label second. The failure context
   has `Stage` planner, `Guard` `planner-intake` and `Attempt` 1 of 1. `Actual` lists one `slot: reason` line per
   `unresolved` entry. The attempt is fixed at 1 of 1 because a punch-out is final; `failure.Runner` would overwrite
   it with its own counter, so the boundary calls `PunchOut` directly.
5. **Commit** (canonical). The H1 is taken out as the title ([D6](#d6-the-title-is-the-first-h1)), placeholders are
   substituted ([D7](#d7-split-chains-use-issue-n-placeholders)), and
   [`provenance.CommitChainWith`](provenance.md#committing-a-chain) hashes the final body and writes it once.

| Outcome | Meaning | Written? |
|---------|---------|----------|
| `OutcomeCommitted` | Every document was written. `Result.Committed` lists them. | Yes |
| `OutcomePunchedOut` | Automated punch-out: comment and label posted. | Comment and label |
| `OutcomePunchOutPending` | A punch-out was decided but a GitHub write failed. The error is returned. | Partly, possibly |
| `OutcomeRoutedToIntake` | The issue changed under the planner (revision conflict). | No |
| `OutcomeRejected` | The reply broke the contract. `Failure` holds `Expected` and `Actual`; the disposition is `Recoverable`. The caller retries or asks the planner to try again. | No |
| `OutcomeChecked` | `Check` (the dry-run path) found every reply acceptable. | No |
| `OutcomeInvalid` | Zero value, returned together with every error. | No |

The package imports no `os/exec`, `net/http` or `crypto/hmac` (`deps_test.go`). GitHub access goes through
`provenance.IssueStore` (implemented with `gh` by `internal/github.IssueStore`) and `failure.Issues`.

The boundary needs the planner's **final assistant message**, not raw `kiro-cli` output with tool traces. Whoever
calls it in automated mode must pass that message. No such caller exists yet; the watcher's call into the planner is
out of scope for #381 (#382 is the consuming side).

## `kairon planner submit`

`kairon planner submit` is how an interactive reply becomes an issue.

```bash
# One document, from a file, creating a new issue (no label)
kairon planner submit --repo owner/name --file reply.yaml

# Check only: extract, mode contract, guard chain. Writes nothing and needs no --repo.
kairon planner submit --dry-run --file reply.yaml

# A split: documents in dependency order, separated by a line containing only ---
kairon planner submit --repo owner/name --file split.yaml

# Resume: issue 1 already exists as #412
kairon planner submit --repo owner/name --known issue-1=412 --file second.yaml
```

| Flag | Meaning |
|------|---------|
| `--file` | File holding the replies. Defaults to stdin. |
| `--repo` | Target repository, `owner/name`. Required unless `--dry-run`. |
| `--label` | Label to apply to each created issue. Repeatable. **Off unless given** ([D5](#d5-the-planner-does-not-ask-the-label-question)). |
| `--known` | An issue already created in this split, as `issue-N=<number>`. Repeatable. Later documents may refer to it as `#{issue-N}`. |
| `--dry-run` | Run the boundary checks only. |

On success it prints one line per document, so a user can resume with `--known`:

```
committed issue-1 -> #412
```

On rejection it prints the guard's `Expected` and `Actual` text and exits 1. Every reply is checked before anything
is written, so a bad later reply leaves no earlier issue behind. `submit` is interactive only: it rejects a
`punch_out`. Automated callers use the package directly.

## Decisions that shape the behavior

### D4: the mode signal

The first line of an automated prompt is `planner.AutomatedIntakeLine`:

```
AUTOMATED INTAKE: non-interactive, no human is available to answer questions. Do not ask questions and do not offer options.
```

Its absence means interactive. The Go constant is the single source of truth. `planner.AutomatedPrompt(repo, issue,
title, body)` builds a prompt that starts with it, and `TestPlannerBoundary*` in `internal/eval` reads the
`automated-*.yaml` eval cases and fails if their first line differs from the constant.

### D5: the planner does not ask the label question

The label is a parameter of the commit step (`kairon planner submit --label`, default off). The planner does not ask
about it, and the old "Gate 2" label turn is gone.

The reason is that the output document is a closed schema with no `labels` key (adding one is a `schema_version`
bump), and the planner must not apply labels itself. An answer given in chat could never reach the boundary, so
asking would collect an answer that is then discarded. No eval check requires the label turn.

This deviates from the literal text of step 9 in `planner-requirements.md` §10.1. That document is historical and is
not edited. **A maintainer should look at this deviation when reviewing the PR.**

### D6: the title is the first H1

`provenance.Request.Title` is required to create an issue and the envelope has no `title` key. So the title is the
first H1 line of `body`. The boundary removes that line, and one blank line after it, before the guard chain and the
hash, so the hash covers exactly what is committed. On the create path a missing H1 is a rejection. On the update path
(automated) the existing GitHub title is kept and a missing H1 is tolerated.

### D7: split chains use `#{issue-N}` placeholders

For a split, the planner proposes a numbered list in dependency order and then returns one document per turn. In the
body of issue k, each earlier issue j of the list that it depends on is written `#{issue-j}`, where j is the 1-based
position in the list:

```
Dependencies: #{issue-1}, #{issue-2}
```

An issue that already exists on GitHub is written `#N`, and an issue with no dependency is `Dependencies: none`. The
boundary gives the n-th document of a chain the key `issue-n` (continuing after any `--known` keys) and
`provenance.SubstituteDependencies` replaces each placeholder before the guard chain and the hash. The committed
line is the single-line `Dependencies: #12, #15` form that the watcher's dependency parser reads. A placeholder with
no committed target is an error and nothing is written.

### D8: automated intake emits one document and never splits

An oversized automated request yields a `punch_out` with the slot `scope.size`, and the reason lists the capabilities
that should become separate issues. Choosing a split is a decision among materially different options, so automated
mode may not make it. **No current eval case covers this rule, so it is uncertified.**

## The manual-submit consequence

Because the planner no longer creates issues, an interactive user has to copy the final document into
`kairon planner submit`. Interactive use is therefore less convenient than before until the TUI planning tab submits
automatically. That auto-submit is a **follow-up**; the TUI is not changed by #381. Nothing in the watcher calls the
planner either.

## What is certified and what is not

**The live eval did not run, so acceptance criterion 8 is NOT MET.** The `live-preflight-baseline` task found that the
`kiro-cli` inside the eval sandbox container has no credentials (`Failed to open browser for authentication`), so
every agent call exits 1 with no output. This is recorded as `LIVE-EVAL-BLOCKED` in
`.kairon/artifacts/builder-381-live-preflight-baseline.md`. As a result:

- No run of the legacy prompt (R0) or of the new prompt exists, and no score is recorded.
- No `iteration-NN.md` log exists. `verify-log` has nothing to verify.
- The promoted prompt (`.kiro/agents/planner-prompt.md`, identical to `.kairon/iterations/planner/v1.md`) was
  chosen on the strength of the offline gates only.
- The by-hand check of `inferences[].evidence` against the fixtures was not done. The evidence judge for
  `best_effort_then_punch_out` cannot yet catch a fabricated quote (it needs the workspace fixture of #389).

**A maintainer must run the commands in
[`.kairon/iterations/planner/ITERATIONS.md`](../.kairon/iterations/planner/ITERATIONS.md)**: get `kiro-cli`
credentials into the eval container, run R0 on the legacy prompt and R1 on `v1.md` with `--sandbox ... --repeat 3`,
iterate on a new candidate if it fails, write the logs and run
`kairon eval verify-log planner --require-cycle=false`. Always pass `--sandbox`.

### Verified offline

| Evidence | Command |
|----------|---------|
| Boundary behavior, including create, update, conflict, punch-out and chain resolution | `go test ./internal/planner -count=1` |
| Chain seeding for resumed splits | `go test ./internal/provenance -run TestCommitChainWith -count=1` |
| `gh`-backed store | `go test ./internal/github -run TestIssueStore -count=1` |
| `submit` command | `go test ./cmd/kairon/... -run Planner -count=1` |
| Eval suite stub documents pass the boundary; the preamble matches the constant; both modes give one document type | `go test ./internal/eval -run 'TestPlannerSuite\|TestPlannerBoundary' -count=1` |
| The suite's reference transcripts pass | `go run ./cmd/kairon eval --backend stub planner` |
| The prompt has no GitHub write and carries the mode line, contract keys and placeholder form | `go test ./internal/planner -run TestPlannerPrompt -count=1` |

Two limits on these:

- `--backend stub` replays reference transcripts. It measures the suite, not the prompt.
- `TestPlannerPrompt` is a regression guard for the original defect (the prompt instructing `gh issue create`). It is
  not a certification of the planner's behavior.

### Needs a live model (not done)

Whether the new prompt makes the model follow the two option rules, ask one decision per turn, withhold the document
until approval, fill automated gaps from evidence and punch out when it cannot, split in dependency order, and stay
within the requirements boundary. These are what `kairon eval --sandbox planner` certifies, through the rubric
described in [Planner suite](evaluation.md#planner-suite). The `multi-issue-split` case only observes the first
document of a split, and the automated-oversize rule (D8) has no case at all.

### Boundary of the regex checks

The eval checks use regular expressions, which are weaker than the real parser (#388). After a live run, the final
reply of every document-returning case should be run through `kairon planner submit --dry-run` to find any reply the
regexes pass and the parser rejects. This has not been done, because there are no live replies.

## Not changed by #381

- The planner agent configuration, `.kiro/agents/planner.json`, is not modified. The "never touches GitHub"
  guarantee does not depend on tool configuration: a body the planner wrote to GitHub itself would have no valid
  marker, so [`Classify`](provenance.md#reading-a-body-the-three-states) returns `none` or `mismatch`.
- The eval suite (rubric, cases, fixtures) is frozen. Certification must be by the #380 suite.
- The TUI planning tab and the watcher.
