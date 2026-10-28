# Design Spec: Apply the trigger label the user chose during interactive planning

Closes #405

## Scope Coverage

Every In Scope item, acceptance criterion and constraint of issue #405 is listed. Task IDs refer to the `kiro-plan` block at the end of this spec.

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "Carrying the user's label answer from the interactive planning session to #404's planning handoff." | `label-path-prompt` (path and prompt line), `planner-agent-files` (agent records the answer), `label-apply` (handoff reads it) |
| S2 | In Scope: "Applying the configured trigger label to the newly created issue after an `OutcomeCommitted` result, and only when the answer was yes." | `label-apply` |
| S3 | In Scope: "The interactive planner prompt and manifest (live and template copies), if the agent has to record the answer." | `planner-agent-files` (the agent does have to record it: prompt step 10, manifest `allowedPaths`) |
| S4 | In Scope: "`docs/planner.md`: remove the \"label answer is not carried\" known gap." | `planner-docs` |
| AC1 | "When the user answers yes, the issue created by the handoff carries the configured trigger label. Verify: a `go test ./internal/tui/...` test with a fake boundary and fake issue port asserts that a label call for the configured label was made on the committed issue number." | `label-apply` (`TestHandoffLabelYesAppliesConfiguredLabel`, `TestHandoffLabelUsesConfiguredLabelName`, `TestHandoffLabelYesWithRealBoundary`) |
| AC2 | "When the user answers no, no label call is made. Verify: the same test with a no answer asserts zero label calls." | `label-apply` (`TestHandoffLabelNoOrMissingAnswerAppliesNothing`) |
| AC3 | "A rejected or failed commit applies no label. Verify: a `go test ./internal/tui/...` case with `OutcomeRejected` asserts zero label calls." | `label-apply` (`TestHandoffLabelNotAppliedWithoutCommit`: rejected, `Process` error, routed to intake, punched out) |
| AC4 | "If the label call fails after a successful commit, the console says so and names the issue. The issue is not created again. Verify: a `go test ./internal/tui/...` case with a failing label port asserts one `Process` call, and that a console line contains the issue number and the error." | `label-apply` (`TestHandoffLabelFailureNamesIssueAndDoesNotRecreate`, plus the invalid-answer, missing-port and empty-label-name cases) |
| AC5 | "Live and template copies of every changed agent file match. Verify: `task sync:check` exits 0." | `planner-agent-files`, `validate-all` (checked in a `git archive HEAD` copy, see section 5) |
| AC6 | "`docs/planner.md` no longer lists the label gap. Verify: `grep -c \"label answer is not carried\" docs/planner.md` prints `0`." | `planner-docs`, `validate-all` |
| C1 | Constraint: "No work in this issue is deferred. ... the PR description lists every follow-up issue number, or states \"none\"." | This spec (`## Deferred work` is `none`); `validate-all`; the PR description says `Deferred work: none` and `Follow-up issues: none` |
| O1 | Out of Scope (kept unchanged): `planner-automated`, `RunAutomated`, any label other than the configured trigger label, the rejection dialog and resume flow from #404 | `validate-all` checks that these are untouched |

## Deferred work

none

## Problem

The interactive `planner` asks whether to add the project's trigger label (`.kiro/agents/planner-prompt.md`, step 9) and then discards the answer. Since #404, `finishPlanning` (`internal/tui/planning_handoff.go`) sends the approved document to `Boundary.Process` on the create path, so every interactively planned issue is created without the label, whatever the user said.

## Decision: where the answer travels

The issue leaves one decision to the architect: bump the stage-output `schema_version`, or keep the answer outside the document.

**Decision: keep it outside the document, in a per-session sidecar file next to #404's document file. `schema_version` stays 1.**

| Dimension | Schema v2 key (e.g. `apply_label: true`) | Sidecar file `.kairon/sessions/<id>.label` |
|-----------|------------------------------------------|---------------------------------------------|
| Blast radius | Touches `internal/stageoutput` (parser, strict-schema, version constant), the guards, the canonicalizer and hash in `internal/provenance`, `planner-shared`, `planner-automated`, `docs/stage-output.md`, and every fixture and eval transcript | Touches the TUI handoff, the interactive prompt and manifest, and docs |
| Fit | The label is an instruction to the console, not part of the requirements. It would be a field the boundary must carry or strip, and the automated path would have to accept and ignore it | The label is only meaningful to the handoff, which is the only code that acts on it |
| Out of Scope | The issue puts `planner-automated` and `RunAutomated` out of scope. A schema bump changes the contract both agents share | Does not touch them |
| Reversibility | A schema version cannot be walked back | A file and one prompt step |
| Cost | Existing documents and the closed-schema guarantee change | One more path in the prompt and one more `allowedPaths` entry |

Consequences of the sidecar:

- The agent has to record the answer, so the prompt and manifest change (S3).
- The label write stays out of `Boundary`. The boundary keeps writing only the issue body; the handoff applies the label after `OutcomeCommitted`, through its own narrow port. The agent still never writes to GitHub.

## Solution Approach

### 1. The sidecar file and the prompt line

- Path: `.kairon/sessions/<session-id>.label`. A new `planningLabelPath(sessionID)` uses the same session ID validation as `planningDocumentPath` (empty ID, a path separator or `..` is refused). Factor the shared validation into one helper; do not duplicate it.
- Content: exactly `yes` or `no`, one word, optional trailing newline, case-insensitive. Nothing else.
- `planningPrompt` gains a second line directly under the first:

  ```
  Document path: .kairon/sessions/<id>.document.yaml
  Label answer path: .kairon/sessions/<id>.label
  ```

  The resumed sentence (if any) and the person's request follow as before. A session ID with no valid path still adds nothing. `planningResumePrompt` goes through `planningPrompt`, so it carries both lines. The classic launch site, the ACP tab and both resume paths all call `planningPrompt`, so no launch site changes.
- The agent writes the label file first, then the document file. The document file is the handoff trigger, so the answer is always on disk when the document is.
- The agent rewrites both files whenever it rewrites the document (a revision after a rejection), so a changed answer is picked up.

### 2. Reading the answer

`finishPlanning` reads the label file in the same step that reads the document, and stores the result, with the label file's path, in `planningHandoffDoneMsg` (`label`, `labelPath`) next to `sent`. The answer that belongs to the sent document therefore travels with it, even if the agent rewrites the files while the send is in flight.

```go
// labelAnswer is what the label file said when the document was read.
type labelAnswer struct {
    apply   bool   // the person answered yes
    problem string // non-empty: the file could not be used; apply is false
}

func readLabelAnswer(path string) labelAnswer
```

| File state | Result |
|------------|--------|
| Does not exist | `apply=false`, no problem. No answer means no label. This also covers a prompt without the label line and a session started before this change. |
| `yes` (any case, surrounding whitespace) | `apply=true` |
| `no` (any case, surrounding whitespace) | `apply=false` |
| Anything else, longer than 16 bytes, not a regular file, or unreadable | `apply=false`, `problem` says what was found. The text is quoted with `%q`, and file content is shown only when it is 16 bytes or fewer. |

### 3. The label port and where the call happens

```go
// planningLabeler is the part of the GitHub port the handoff uses to apply
// the trigger label. github.CLI satisfies it.
type planningLabeler interface {
    AddLabel(repo string, issue int, label string) error
}
var _ planningLabeler = github.CLI{}
```

`failure.Issues` already has `AddLabel(repo, issue, label)` and `github.CLI.AddLabel` implements it via `gh issue edit --add-label` (it rejects an empty label and a non-positive issue). No new GitHub code is needed. A narrow one-method interface keeps the test fake small; `githubtest.Fake` also satisfies it and supports `Fail["AddLabel"]`, but the tests use a local `fakeLabeler`.

- New model field `planLabeler planningLabeler`, set in `newModel` (`tui.go`) by `newPlanningLabeler(cfg)`: `github.CLI{}` when `cfg.Repo` is set, otherwise nil (the same condition as `planBoundary`).
- `finishPlanning` captures `repo := m.config.Repo`, `labelName := m.config.Label` and `m.planLabeler` before it returns the command, as it already does for the boundary.
- The label call is made **inside the command**, after `Process` returns, so `gh` never runs on the UI goroutine. It runs only when `err == nil && res.Outcome == planner.OutcomeCommitted && answer.apply`:

  ```go
  res, err := boundary.Process(ctx, req)
  done := planningHandoffDoneMsg{..., result: res, err: err, label: answer, labelName: labelName}
  if err == nil && res.Outcome == planner.OutcomeCommitted && answer.apply {
      done.labelTried = true
      done.labelErr = applyTriggerLabel(labeler, repo, labelName, res.Number)
  }
  return done
  ```

  `applyTriggerLabel` returns an error for a nil labeler or an empty label name, otherwise the result of `AddLabel(repo, number, labelName)`.
- `Process` is called once per send. The label step does not call it again, and the document file is deleted after a commit, so a failed label cannot create the issue a second time. `Process` still runs exactly once for the session unless the agent writes a different, newer document.

### 4. Console output (`handlePlanningHandoffDone`, committed branch)

After the existing success line (`Created issue #N ...`):

| Situation | Console line |
|-----------|--------------|
| Label applied | Success style: ``Added the label `kairon` to issue #N.`` |
| `labelErr != nil` | Error style: ``Issue #N was created, but adding the label `kairon` failed: <err>. Add the label by hand if you want the watcher to pick it up.`` Contains the issue number and the error text. |
| `label.problem != ""` | Error style: `Issue #N was created without the trigger label: <problem>. Add it by hand if you want the watcher to pick it up.` |
| Answer no, or no answer | No extra line |

Then the existing flow continues unchanged: if a newer document exists, send it (the label file is kept, it may belong to that document); otherwise delete the document file and then the label file. A failure to remove the label file (other than "does not exist") gets an error line that names the path, because a stale `yes` would label the next issue of the same session. Rejected, `Process` error and unexpected-outcome branches never touch the label.

### 5. Working tree caveat (environment-injected changes)

The worktree has uncommitted changes to every `.kiro/agents/*.json` that are not part of this issue (reformatting, an `@creds-agent` tool, an `mcpServers` block). They make `task sync:check`, `TestInteractiveManifest` (tools list), `TestInteractiveManifestHasNoInjectedCapabilities` and `TestInteractiveFilesMatchTemplates` fail in this worktree regardless of this change. They must not be reverted, included in a commit or copied into the templates. Rules for the builder, identical to #404's:

- Edit live `.kiro/agents/planner.json` by adding only the new `allowedPaths` entry, in the file's existing (reformatted) style. Do not touch anything else in it.
- Edit `cmd/kairon/templates/kiro/agents/planner.json` (it equals HEAD's live file) by adding the same entry in its existing style. Never run `cp .kiro/agents/*.json cmd/kairon/templates/kiro/agents/`.
- Stage the live manifest from the clean template blob, not from the working tree file:
  `git update-index --cacheinfo 100644,$(git hash-object -w cmd/kairon/templates/kiro/agents/planner.json),.kiro/agents/planner.json`.
  That commits HEAD's formatting plus the one new entry and leaves the injected edits unstaged.
- Copy the prompt with `cp .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/` (the prompt has no injected edits).
- Stage by explicit path. Never use `git add -A`, `git add .` or `git add .kiro/agents`. No other `.kiro/agents/*.json` may be staged.
- Verify sync for only the files this issue changes with `diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md` (no output) and `diff <(git show :.kiro/agents/planner.json) cmd/kairon/templates/kiro/agents/planner.json` (no output; the staged blob equals the template).
- Verify the full `task sync:check` (AC5) in a CI-equivalent copy of the committed tree, where it exits 0:
  `d=$(mktemp -d) && git archive HEAD | tar -x -C "$d" && (cd "$d" && task sync:check && go test ./internal/planner/... && go test ./internal/eval -run TestPlannerSuite)`.
  In the worktree itself, `task sync:check` reports only the pre-existing differences in the other `.kiro/agents/*.json` files and nothing for `planner-prompt.md` or `planner.json`.

## Relevant Files

Modify:

- `internal/tui/planning_handoff.go`: `planningLabelPath`, `planningPrompt` label line, `planningLabeler`, `newPlanningLabeler`, `labelAnswer`, `readLabelAnswer`, `applyTriggerLabel`, new fields on `planningHandoffDoneMsg`, label capture in `finishPlanning`, label call in the command, console lines and label-file removal in `handlePlanningHandoffDone`.
- `internal/tui/tui.go`: `planLabeler` field and its initialisation in `newModel`.
- `internal/tui/planning_handoff_test.go`: update the prompt tests that assert the exact prompt (`TestPlanningPromptWithoutRequestIsJustThePath` changes to two lines; the prefix checks stay valid).
- `internal/tui/planning_label_test.go` (new): the label tests below.
- `internal/planner/interactive_agent_test.go`: manifest and prompt tests.
- `.kiro/agents/planner-prompt.md`, `cmd/kairon/templates/kiro/agents/planner-prompt.md` (identical).
- `.kiro/agents/planner.json`, `cmd/kairon/templates/kiro/agents/planner.json`: one new `allowedPaths` entry, `.kairon/sessions/*.label`.
- `docs/planner.md`.

Read-only references: `internal/planner/boundary.go`, `internal/failure/state.go` (`Issues`), `internal/github/adapter.go` (`CLI.AddLabel`), `internal/github/githubtest/fake.go`, `internal/config/config.go` (`Label`, default `kairon`).

Must stay unchanged (Out of Scope): `internal/planner/boundary.go`, `internal/planner/run.go` and the automated invoker, `.kiro/agents/planner-automated*`, `.kiro/skills/planner-shared/`, `internal/stageoutput/`, `internal/tui/planning_rejection.go`, `docs/stage-output.md`.

## Team Orchestration

- `label-path-prompt` and `planner-agent-files` have no dependencies and run in parallel. They share one contract, fixed in this spec: the prompt line is exactly `Label answer path: .kairon/sessions/<id>.label`, and the file holds `yes` or `no`.
- `label-apply` depends on `label-path-prompt` (same Go files).
- `planner-docs` waits for both code tasks so it describes what shipped.
- `validate-all` is read-only and runs last.

## Step-by-Step Task Breakdown

### Task 1: `label-path-prompt` (builder)

Failing tests first, in `internal/tui/planning_handoff_test.go`:

- `TestPlanningLabelPath`: `s1` gives `.kairon/sessions/s1.label`; an empty ID, `a/b`, `a\b` and `../x` give an error.
- `TestPlanningPromptNewSession` and `TestPlanningPromptResumedSessionAddsSentence` expect line 2 to be `Label answer path: .kairon/sessions/s1.label`; `TestPlanningPromptWithoutRequestIsJustThePath` expects exactly the two lines; `TestPlanningResumePromptCarriesFailureAndPath` expects both lines; `TestPlanningPromptInvalidSessionAddsNothing` is unchanged.
- The classic and ACP prompt tests (`classic_launch_test.go`, `planning_acp_handoff_test.go`) check `Document path:` by prefix or `Contains` and must keep passing; add one assertion in each that the prompt also contains the `Label answer path:` line.

Run them and confirm they fail because the line is missing, not because of a compile error (add the function stub only after seeing the missing-line failure for the prompt tests; `TestPlanningLabelPath` fails to compile until `planningLabelPath` exists, so write a stub that returns an error and confirm it fails on the assertion). Then implement `planningLabelPath` and the prompt change, sharing the ID validation with `planningDocumentPath`. Commit tests and implementation together.

### Task 2: `label-apply` (builder)

Failing tests first, in the new `internal/tui/planning_label_test.go`. Helpers: `fakeLabeler` (mutex-guarded `calls []labelCall{repo string, issue int, label string}`, optional `err`), `writeLabelAnswer(t, sessionID, content)`, and a `labelFixture` that wraps `handoffFixture`, sets `m.planLabeler`, and writes a document plus a label file for session `s1`. Reuse `fakeBoundary`, `runHandoff`, `activityText`, `fileExists` from `planning_handoff_test.go`.

| Test | Asserts | AC |
|------|---------|----|
| `TestHandoffLabelYesAppliesConfiguredLabel` | Fake boundary returns committed #77; label file `yes\n`. Exactly one `AddLabel` call equal to `{o/r, 77, kairon}`; one `Process` call; success line mentions the label and `#77`; document file and label file are deleted | AC1 |
| `TestHandoffLabelUsesConfiguredLabelName` | `m.config.Label = "my-label"`; the call uses `my-label`, not `kairon` | AC1 |
| `TestHandoffLabelYesWithRealBoundary` | The real `planner.Boundary` over `handoffStore` (creates issue 321) with a valid canonical document: the label call is on issue 321 and `store.createCount() == 1` | AC1 |
| `TestHandoffLabelNoOrMissingAnswerAppliesNothing` | Table: `no`, `NO\n`, and no label file. Zero `AddLabel` calls; no label console line; issue still created | AC2 |
| `TestHandoffLabelYesIsCaseAndWhitespaceTolerant` | `YES`, ` yes \n` apply the label | AC1 |
| `TestHandoffLabelNotAppliedWithoutCommit` | Label file `yes`. Table: `OutcomeRejected` (with a failure), `OutcomeRejected` without one, a `Process` error, `OutcomeRoutedToIntake`, `OutcomePunchedOut`. Zero `AddLabel` calls in every case; document and label files are kept | AC3 |
| `TestHandoffLabelFailureNamesIssueAndDoesNotRecreate` | `fakeLabeler{err: errors.New("gh issue edit failed: boom")}`; committed #77. One `Process` call; one console line contains both `#77` and `boom`; the document file is gone; a second `finishPlanning(s1End())` returns no command and `Process` stays at 1 call | AC4 |
| `TestHandoffLabelInvalidAnswerIsReported` | Label file `maybe`, and a 100-byte file: zero label calls; a console line contains `#77` and says the label was not applied; the issue is still created and reported | AC4 |
| `TestHandoffLabelMissingPortOrNameIsReported` | `m.planLabeler = nil` with `yes`, and `m.config.Label = ""` with `yes`: zero `AddLabel` calls; a console line contains `#77` and the reason | AC4 |
| `TestHandoffLabelKeptWhenNewerDocumentWritten` | A newer document is written during the send (use `rewriteDuringSend`): the earlier document's label is applied, the label file is kept, the newer document is sent | edge case |
| `TestHandoffLabelFileRemovalFailureIsReported` | Apply a `planningHandoffDoneMsg` whose `labelPath` is a non-empty directory (the technique `TestHandoffRemovalFailureTellsUserToDeleteFile` uses): a console line names that path and the issue number | edge case |

Run them and confirm each fails for the expected reason (no `planLabeler` field yet means a compile error, so first add the empty `planningLabeler` type and the field, see the failing assertions such as "label calls = 0, want 1", then implement). Then implement sections 2 to 4 of the approach. Add `var _ planningLabeler = github.CLI{}` and `newPlanningLabeler`; set `planLabeler` in `newModel`. Confirm the existing handoff tests still pass unchanged (no label file means no label behaviour). Commit tests and implementation together; the message names the first failing test.

### Task 3: `planner-agent-files` (builder)

Failing tests first, in `internal/planner/interactive_agent_test.go`:

- Add `interactiveSessionLabelPath = ".kairon/sessions/*.label"`. `TestInteractiveManifest` expects `[".worktrees/planning-*", ".kairon/sessions/*.document.yaml", ".kairon/sessions/*.label"]`. `TestInteractiveManifestAllowsSessionDocument` also requires the label entry. The template-versus-live `allowedPaths` test already compares both.
- New `TestInteractivePromptRecordsLabelAnswer`: in the Task section, the write step (between `**Write the document file.**` and `**Return the document.**`) names the `Label answer path:` line, says to write `yes` or `no`, says the label file is written before the document file, and handles a prompt with no such line. The label step (`**Label question.**`) still says the answer is not part of the document and the agent never applies a label itself, and now says Kairon applies the label from the recorded answer.
- `TestInteractivePromptStatesSingleWriteException` is extended: the constraint line names both `Document path:` and `Label answer path:` as the exception, and still says the reply is still the document.
- All other existing prompt tests (RTCC headings, elicitation flow, never `punch_out`, no `gh issue create` outside a prohibition, no restated shared content, no mode-selection words, `only names a path`) keep passing.

Then edit `.kiro/agents/planner-prompt.md`:

- Step 9: add that Kairon applies the label from the answer the agent records in step 10.
- Step 10: "The first message may carry a line `Document path: <path>` and a line `Label answer path: <path>`. After the label turn, write the person's answer to the `Label answer path:` with the `write` tool: the single word `yes` or `no` and nothing else. Then write the same document you are about to return to the `Document path:`. Write the label file first. Do this again whenever you rewrite the document. The files are Kairon's hand-off. If the first message has no `Document path:` line, skip this step."
- Context bullet "A message that only names a path ..." keeps its first words and also names the `Label answer path:` line.
- Constraint: the one exception now covers the files at the given `Document path:` and `Label answer path:`.

Then update both manifests per section 5, copy the prompt to the template, and run the checks from section 5. The eval cases are unchanged because their input carries no path lines.

### Task 4: `planner-docs` (builder, non-code, no test first)

In `docs/planner.md`:

- Delete the "The label answer is not carried" bullet under "Known gaps". Add no new gap entry.
- Tools row: `write` is limited to `.worktrees/planning-*`, the per-session document files `.kairon/sessions/*.document.yaml` and the per-session label files `.kairon/sessions/*.label`.
- "The per-session file and the prompt line": add the second prompt line, the label file (`.kairon/sessions/<session-id>.label`, `yes` or `no`, written before the document), and the statement that the stage-output schema stays at `schema_version: 1` because the answer travels outside the document.
- Add a subsection "The trigger label" after "Results": when the label is applied (only after `OutcomeCommitted`, only for `yes`, the `label` value of `.kairon/config.yaml`, one `AddLabel` call on the committed issue number), when it is not (`no`, no file, rejected, `Process` error, other outcomes), what the console shows when the call fails or the answer is unusable (a line with the issue number; the issue is not created again), and that the agent still never writes to GitHub: Kairon's handoff makes the one label call, `Boundary` is unchanged and the automated path never asks.
- Add a row or sentence to the Results table: a committed issue also reports the label outcome.

### Task 5: `validate-all` (validator)

Read-only. Verify every Scope Coverage row against the final diff and run the validation commands below.

## Validation Commands

```bash
go test ./internal/tui/... -run 'TestHandoffLabel|TestPlanningLabelPath|TestPlanningPrompt|TestPlanningResumePrompt'
go test ./internal/tui/...
go vet ./...
test -z "$(gofmt -l .)"
diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md
test "$(grep -c 'label answer is not carried' docs/planner.md)" = "0"
d=$(mktemp -d) && git archive HEAD | tar -x -C "$d" && (cd "$d" && task sync:check && go test ./internal/planner/... && go test ./internal/eval -run TestPlannerSuite)
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "label-path-prompt"
    agent: "builder"
    description: "TDD in internal/tui. Add planningLabelPath(sessionID) returning .kairon/sessions/<id>.label with the same ID validation as planningDocumentPath (share one helper), and add a second prompt line 'Label answer path: <path>' directly under 'Document path:' in planningPrompt (and so in planningResumePrompt). Write the failing tests first (TestPlanningLabelPath, the updated TestPlanningPrompt* and TestPlanningResumePromptCarriesFailureAndPath, one added assertion each in the classic and ACP prompt tests), see them fail for the missing path or line and not for a compile error, then implement. Commit tests and implementation together. Do not change the launch sites."
    dependencies: []
    acceptance_criteria:
      - "planningLabelPath('s1') returns '.kairon/sessions/s1.label' and returns an error for an empty ID, an ID with '/' or a backslash, and an ID containing '..'"
      - "planningPrompt('s1', 'Plan it', false) starts with 'Document path: .kairon/sessions/s1.document.yaml\\nLabel answer path: .kairon/sessions/s1.label\\n' and ends with '\\n\\nPlan it'"
      - "planningPrompt with no request is exactly the two path lines; with an invalid session ID it returns the request unchanged; the resumed sentence still appears only when resumed"
      - "planningResumePrompt, the classic argv prompt and the ACP first message each contain the Label answer path line"
      - "The new tests were seen failing for the expected reason before the implementation, and pass after"
      - "No launch site (commands.go, planning_tab.go) is modified"
    validation_commands:
      - "go test ./internal/tui/... -run 'TestPlanningLabelPath|TestPlanningPrompt|TestPlanningResumePrompt|Classic|ACP'"
      - "go vet ./internal/tui/..."
      - "test -z \"$(gofmt -l internal/tui)\""

  - id: "label-apply"
    agent: "builder"
    description: "TDD in internal/tui. Add the planningLabeler port (AddLabel(repo, issue, label) error; github.CLI satisfies it), the planLabeler model field set in newModel, labelAnswer/readLabelAnswer, applyTriggerLabel, and new fields on planningHandoffDoneMsg. finishPlanning reads the label file with the document and captures repo, m.config.Label and the labeler; the returned command calls AddLabel(repo, res.Number, label) only when Process returned no error, OutcomeCommitted, and the answer is yes. handlePlanningHandoffDone reports a success line, or an error line that names the issue number and the error (failed call, unusable answer, nil port, empty label name), and removes the label file together with the document file after a commit when no newer document exists. Write internal/tui/planning_label_test.go first with a local fakeLabeler, see each test fail for the expected reason (for example 'label calls = 0, want 1'), then implement. Commit tests and implementation together. Process is still called once per send; boundary.go, run.go and the schema are not modified."
    dependencies: ["label-path-prompt"]
    acceptance_criteria:
      - "Answer yes + fake boundary committing #77: exactly one AddLabel call {o/r, 77, kairon}, one Process call, document and label files deleted, success line shown (AC1)"
      - "The label name comes from config.Label (a test with 'my-label' proves it) and the call uses the real boundary's issue number in the real-boundary test (AC1)"
      - "Answer no, no label file, or an uppercase NO: zero AddLabel calls (AC2)"
      - "OutcomeRejected, a Process error, OutcomeRoutedToIntake and OutcomePunchedOut: zero AddLabel calls and both files kept (AC3)"
      - "A failing labeler after a commit: Process called once, a console line contains the issue number and the error text, the document file is gone, and a second finishPlanning returns no command (AC4)"
      - "An unusable answer file, a nil labeler and an empty configured label each produce zero AddLabel calls and a console line naming the issue number"
      - "A newer document written during the send keeps the label file and is sent"
      - "Exactly one '.AddLabel(' call site exists in non-test files of internal/tui, and no 'issue create' text; the existing handoff tests pass unchanged"
    validation_commands:
      - "go test ./internal/tui/... -run 'TestHandoffLabel'"
      - "go test ./internal/tui/..."
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
      - "test \"$(grep -rn '\\.AddLabel(' internal/tui --include='*.go' | grep -v _test | wc -l)\" -eq 1"
      - "test -z \"$(grep -rn 'issue create' internal/tui --include='*.go' | grep -v _test)\""

  - id: "planner-agent-files"
    agent: "builder"
    description: "Update the interactive planner agent files so the agent records the label answer. Tests first in internal/planner/interactive_agent_test.go: manifest allowedPaths must be ['.worktrees/planning-*', '.kairon/sessions/*.document.yaml', '.kairon/sessions/*.label']; the write step of the prompt names the 'Label answer path:' line, the words yes and no, and writing the label file before the document file; step 9 still says the answer is not part of the document and the agent never applies a label; the write-exception constraint names both path lines and still says the reply is still the document. See them fail for the expected reason, then edit .kiro/agents/planner-prompt.md and copy it to cmd/kairon/templates/kiro/agents/, add only the one allowedPaths entry to live planner.json (existing reformatted style, injected content untouched) and to the template planner.json (HEAD style), and stage the live manifest from the clean template blob with git update-index --cacheinfo. Never copy live .kiro/agents/*.json into templates, never git add -A, do not revert the injected edits. planner-automated files, planner-shared skill and the eval cases are unchanged."
    dependencies: []
    acceptance_criteria:
      - "New and updated tests were seen failing for the expected reason before the edits, and pass after (run them with -run so the injected-manifest tests that already fail in this worktree do not hide the result)"
      - "The prompt step that writes files names 'Label answer path:', says to write exactly yes or no, orders the label file before the document file, and covers a prompt with no path lines"
      - "All pre-existing prompt tests still pass (RTCC headings, elicitation flow, never punch_out, no gh issue create outside a prohibition, no restated shared content, no mode-selection words, 'only names a path')"
      - "diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md prints nothing"
      - "git show :.kiro/agents/planner.json and cmd/kairon/templates/kiro/agents/planner.json are identical, differ from git show HEAD:.kiro/agents/planner.json only by the new allowedPaths entry, and no other .kiro/agents/*.json is staged"
      - "In a git archive HEAD copy after the commit, task sync:check, go test ./internal/planner/... and go test ./internal/eval -run TestPlannerSuite exit 0"
    validation_commands:
      - "go test ./internal/planner/... -run 'TestInteractivePrompt|TestInteractiveManifestAllowsSessionDocument|TestInteractiveTemplate'"
      - "diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md"
      - "grep -q '.kairon/sessions/\\*.label' cmd/kairon/templates/kiro/agents/planner.json"
      - "grep -q 'Label answer path:' cmd/kairon/templates/kiro/agents/planner-prompt.md"
      - "git diff --cached --name-only -- .kiro/agents | grep -v -x -e .kiro/agents/planner.json -e .kiro/agents/planner-prompt.md ; test $? -eq 1"

  - id: "planner-docs"
    agent: "builder"
    description: "Update docs/planner.md (non-code, no test first). Remove the 'The label answer is not carried' known-gap bullet and add no new gap. Update the Tools row to include .kairon/sessions/*.label. Describe the second prompt line, the label file (yes or no, written before the document), and that schema_version stays 1. Add a 'The trigger label' subsection: applied only after OutcomeCommitted and only for yes, the label is config.Label, one AddLabel call on the committed issue number, never applied for no, no file, rejection, Process error or other outcomes, what the console shows when the call fails or the answer is unusable (issue number and error; the issue is not created again), the agent still never writes to GitHub, the automated path never asks. Match the shipped code. No sentence may defer work to an unnamed follow-up."
    dependencies: ["label-apply", "planner-agent-files"]
    acceptance_criteria:
      - "grep -c 'label answer is not carried' docs/planner.md prints 0"
      - "docs/planner.md describes the label file, the Label answer path prompt line, when the label is and is not applied, and the failure console behavior, matching the code"
      - "The Known gaps list gains no entry, and no new text in docs/planner.md refers to deferred work without an issue number"
    validation_commands:
      - "test \"$(grep -c 'label answer is not carried' docs/planner.md)\" = \"0\""
      - "grep -q 'Label answer path' docs/planner.md"
      - "grep -q '.kairon/sessions/\\*.label' docs/planner.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every Scope Coverage row of issue 405 against the final diff and run all validation commands. Read-only. Check criterion by criterion with command output, and that Out of Scope files are untouched."
    dependencies: ["planner-docs"]
    acceptance_criteria:
      - "AC1 to AC6 of issue 405 are each verified with command output"
      - "internal/planner/boundary.go, internal/planner/run.go, the planner-automated agent files, .kiro/skills/planner-shared/, internal/stageoutput/, internal/tui/planning_rejection.go and docs/stage-output.md are unchanged in the diff against the base"
      - "Only planner.json from the injected .kiro/agents/*.json set is committed, and only with the one new allowedPaths entry"
      - "Exactly one '.AddLabel(' call site in non-test files of internal/tui, and Boundary.Process still has exactly one call site there"
      - "The spec's Deferred work is none and nothing added by this change refers to deferred work without an issue number; the PR description must say 'Deferred work: none' and 'Follow-up issues: none'"
      - "go test ./internal/tui/..., go vet ./... and gofmt pass; task sync:check, go test ./internal/planner/... and go test ./internal/eval -run TestPlannerSuite pass in a git archive HEAD copy"
    validation_commands:
      - "go test ./internal/tui/..."
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
      - "test \"$(grep -rn '\\.AddLabel(' internal/tui --include='*.go' | grep -v _test | wc -l)\" -eq 1"
      - "test \"$(grep -rn '\\.Process(ctx' internal/tui --include='*.go' | grep -v _test | wc -l)\" -eq 1"
      - "test \"$(grep -c 'label answer is not carried' docs/planner.md)\" = \"0\""
      - "git diff --stat origin/HEAD -- internal/planner/boundary.go internal/planner/run.go .kiro/agents/planner-automated.json .kiro/agents/planner-automated-prompt.md .kiro/skills/planner-shared internal/stageoutput internal/tui/planning_rejection.go docs/stage-output.md"
      - "d=$(mktemp -d) && git archive HEAD | tar -x -C \"$d\" && (cd \"$d\" && task sync:check && go test ./internal/planner/... && go test ./internal/eval -run TestPlannerSuite)"
```
