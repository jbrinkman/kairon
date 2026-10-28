# Design Spec: Commit the interactive planner's document through the boundary when planning ends

Closes #404

## Scope Coverage

Every In Scope item, functional requirement, acceptance criterion and constraint of issue #404 is listed. Task IDs refer to the `kiro-plan` block at the end of this spec.

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/tui`: a front-end-agnostic planning-completion handoff (new) that reads the session's document file, calls `Boundary.Process`, and reports the outcome. Both the `execDoneMsg` handler (classic) and the ACP planning tab call it." | `handoff-core` (handoff), `classic-launch-site` (classic caller), `acp-launch-site` (ACP caller) |
| S2 | In Scope: "`internal/tui`: a planning-rejection dialog (new overlay) that shows the boundary's failure reason and offers **Resume planning** or **Dismiss**." | `rejection-dialog` |
| S3 | In Scope: "`internal/tui`: both launch sites tell the agent the per-session document path in the initial prompt, for new and for resumed sessions." | `handoff-core` (prompt builders), `classic-launch-site`, `acp-launch-site` |
| S4 | In Scope: "`.kiro/agents/planner-prompt.md` and `.kiro/agents/planner.json`, plus their template copies in `cmd/kairon/templates/kiro/agents/`: the interactive planner writes the approved document to the given path and still returns the document as its reply. Add the path to `toolsSettings.write.allowedPaths`." | `planner-agent-files` |
| S5 | In Scope: "`docs/planner.md`: remove the \"interactive reply is not captured\" known gap and describe the handoff." | `planner-docs` |
| FR1 | "When a planning session starts or resumes, Kairon picks one document path for it under `.kairon/sessions/`, keyed by the planning session ID, and puts that path in the agent's initial prompt." | `handoff-core`, `classic-launch-site`, `acp-launch-site` |
| FR2 | "The interactive planner writes the approved stage-output document to that path, after the label turn and before returning the document as its reply. It writes the same document it returns." | `planner-agent-files` |
| FR3 | "The handoff runs when a planning session finishes. For classic, that is process exit (`execDoneMsg`). For ACP, it is the end of an agent turn after which the document file exists." | `classic-launch-site`, `acp-launch-site` |
| FR4 | "If the document file doesn't exist when the handoff runs, nothing happens: no dialog, no GitHub call." | `handoff-core` |
| FR5 | "If the file exists, its contents go to `Boundary.Process` with `Target.Number = 0`." | `handoff-core` |
| FR6 | "`OutcomeCommitted`: the console shows a success line with the new issue number and URL, and the document file is deleted so a later session end can't create a duplicate issue." | `handoff-core` |
| FR7 | "`OutcomeRejected` ...: the rejection dialog opens. It shows the failing guard and the failure's expected and actual text, and offers **Resume planning** and **Dismiss**. Nothing is written to GitHub, and the file is kept either way." | `rejection-dialog`, `handoff-core` |
| FR8 | "**Resume planning** reopens the planning session the document came from, in the same front end, and gives the agent the failure context so it can revise and rewrite the document. **Dismiss** closes the dialog and leaves the file in place for a later resume." | `rejection-dialog` (dialog actions), `classic-launch-site` (classic resume), `acp-launch-site` (ACP resume) |
| FR9 | "A `Process` error (gh failure, cancelled context): the console shows an error line, and the file is kept." | `handoff-core` |
| EC1 | Edge case: "The document file is empty or not valid YAML: treated as `OutcomeRejected` through `Extract`, and the dialog opens." | `handoff-core` (test with the real `planner.Boundary`) |
| EC2 | Edge case: "The planner returns `result: punch_out` on the create path: `Process` returns `ErrNoPunchOutTarget`, which is handled as requirement 9 (error line, file kept)." | `handoff-core` (test with the real `planner.Boundary`) |
| EC3 | Edge case: "The session ends twice for the same approved document ...: the file was deleted on commit, so the second end does nothing (requirement 4)." | `handoff-core` |
| AC1 | "Ending a classic planning session whose document file holds a valid canonical document calls `Boundary.Process` once with that document and `Target.Number == 0`, then deletes the file." | `handoff-core`, `classic-launch-site` |
| AC2 | "The ACP planning tab uses the same handoff: an agent turn that ends with the document file present calls the same function as classic." | `acp-launch-site` |
| AC3 | "With no document file, ending a session in either front end makes no `Process` call and opens no dialog." | `handoff-core`, `classic-launch-site`, `acp-launch-site` |
| AC4 | "A rejected document opens the rejection dialog with the guard name and the expected and actual text, and keeps the file." | `rejection-dialog`, `handoff-core` |
| AC5 | "**Resume planning** reopens the originating session and gives the agent the failure context. **Dismiss** returns to the console and keeps the file." | `rejection-dialog`, `classic-launch-site`, `acp-launch-site` |
| AC6 | "Each launch site puts the per-session document path in the agent's initial prompt, and the path is under `.kairon/sessions/`." | `handoff-core`, `classic-launch-site`, `acp-launch-site` |
| AC7 | "The interactive planner prompt tells the agent to write the approved document to the given path, `planner.json` allows writes there, live and template copies match, and the planner eval suite's interactive cases still pass with the reply still being the document." | `planner-agent-files`, `validate-all` |
| AC8 | "`docs/planner.md` no longer lists \"The interactive reply is not captured\" as a known gap and describes the handoff." | `planner-docs` |
| C1 | Constraint: "One handoff function serves both front ends. It doesn't branch on which one ran the session, so classic can be removed without touching it." (exactly one `Process` call site in `internal/tui`) | `handoff-core`, `validate-all` |
| C2 | Constraint: "The TUI never runs `gh issue create`. Issue creation goes only through `Boundary.Process`." | `handoff-core`, `validate-all` |
| C3 | Constraint: "No work in this issue is deferred." | This spec (`## Deferred work` is `none`), `validate-all` |
| O1 | Out of Scope (kept unchanged): `planner-automated`, its prompt, `RunAutomated`, `planner-shared/SKILL.md` and the stage-output schema | `planner-agent-files` and `validate-all` check they are untouched |

## Deferred work

none

## Problem

An interactive planning session ends with the planner returning its stage-output document, and nothing in Kairon picks that document up. `planner.Boundary.Process` (`internal/planner/boundary.go:162`) has one caller, `RunAutomated`. The `execDoneMsg` handler (`internal/tui/tui.go`) only restores the console. The ACP planning tab (`internal/tui/planning_tab.go`) never sends its reply anywhere. No issue is created.

A terminal session's output cannot be captured, so the document travels through a file. Kairon chooses the path per planning session and tells the agent. The agent writes the approved document there. When the session ends, Kairon reads the file and sends its contents to `Boundary.Process` on the create path.

## Solution Approach

### 1. One handoff, owned by the model, called by both front ends

A new file `internal/tui/planning_handoff.go` holds everything front-end-agnostic. Nothing in it names classic or ACP.

```go
// planningBoundary is the part of planner.Boundary the handoff uses.
type planningBoundary interface {
    Process(ctx context.Context, req planner.Request) (planner.Result, error)
}

// planningEnd is what a front end hands over when a planning session ends.
type planningEnd struct {
    SessionID string
    // Resume reopens the session in the front end that ran it and gives the
    // agent the failure context. Each front end supplies its own.
    Resume func(m model, fc guard.FailureContext) (model, tea.Cmd)
}

func planningDocumentPath(sessionID string) (string, error)           // ".kairon/sessions/<id>.document.yaml"
func planningPrompt(sessionID, request string, resumed bool) string     // initial prompt: "Document path: <path>" + request
func planningResumePrompt(sessionID string, fc guard.FailureContext) string
func (m model) finishPlanning(end planningEnd) (model, tea.Cmd)        // THE handoff
```

`finishPlanning(end)`:

1. Resolve the path. Stat it. If the file does not exist, return `(m, nil)` with no console line, no overlay and no GitHub call (FR4).
2. If a handoff for `end.SessionID` is already in flight (`m.planningHandoffs[id]`), return `(m, nil)`. This stops a second turn end from sending the same document twice while `gh` is still running.
3. Read the file. A read error becomes a console error line and the file is kept.
4. Return a `tea.Cmd` that runs off the UI goroutine. The command calls `m.planBoundary.Process(ctx, planner.Request{Reply: contents, Attempt: guard.Attempt{Current: 1, Max: 1}, Target: provenance.Target{Number: 0}})` with a 2-minute timeout, and returns a `planningHandoffDoneMsg{end, path, result, err}`. This is the only `Process` call in `internal/tui`.

`model.Update` handles `planningHandoffDoneMsg` (`handlePlanningHandoffDone`):

| Result | Action |
|--------|--------|
| `err != nil` (gh failure, cancelled context, `ErrNoPunchOutTarget`, missing title) | Console error line naming the cause. File kept (FR9, EC2). |
| `OutcomeCommitted` | `os.Remove(path)`, then a success line with `Result.Number` and `https://github.com/<repo>/issues/<n>`. If the remove fails, add an error line that says the issue exists and the file must be deleted by hand to avoid a duplicate (FR6). |
| `OutcomeRejected` | `failure := planner.RejectionFailure(result, attempt)`. Open the rejection dialog (section 2). Also add a console line naming the guard and the kept file path, so a second rejection that replaces an open dialog still leaves a record. File kept (FR7). |
| any other outcome | Console error line `unexpected outcome`. File kept. (Not reachable on the create path; handled so it is never read as success.) |

In all cases the in-flight marker for the session is cleared.

The boundary is built once in `newModel` from the config, with the same collaborators `internal/agent/intake.go` uses: `github.NewIssueStore(cfg.Repo)` as the committer's store, `github.CLI{}` as `Issues`, and a `planner.ConflictRouter` for `failure.Target{Repo: cfg.Repo, FailLabel: failure.FailedLabel(cfg.Label)}`. Tests replace `m.planBoundary` with a fake. If `cfg.Repo` is empty the boundary is nil and a present document produces an error line (file kept).

`Request.Title` stays empty so the boundary takes the first `# ` heading of the body. `IssueStore.Create` returns only the number, so the URL is built from the configured repo.

Comments and strings in the non-test files of `internal/tui` must not contain the text `Boundary.Process`, `.Process(ctx` (except the single call) or `issue create`, because the constraint checks use `grep` on those strings. Say "the planning boundary" in comments.

### 2. Rejection dialog overlay

New `overlayPlanningRejection` in the `overlayType` list. State on the model: `planningRejection *planningRejection{ end planningEnd; failure guard.FailureContext; path string }`.

- `openPlanningRejection` builds the content lines: a heading, `Guard: <name>`, `Expected: <text>`, `Actual: <text>` (multi-line text split on newlines, no truncation), the kept file path, and the footer `R  Resume planning    D / Esc  Dismiss`. It sets `activeOverlay`, `overlayTitle` ("Planning document rejected") and the content directly, so the generic "Press ESC to close" footer is not added. `renderOverlay` already wraps long lines.
- Key handling is added in `Update` before the generic overlay-Esc branch: `r` or `enter` calls `planningRejection.end.Resume(m, failure)` after clearing the overlay; `d` or `esc` clears the overlay and the state. Other keys are swallowed by the existing "block input while an overlay is active" rule.
- Resume with a nil `Resume` function, or one that cannot find its front end, shows a console error line and keeps the file.

### 3. Launch sites tell the agent the path

Both launch sites call `planningPrompt`. The first line of the prompt is `Document path: .kairon/sessions/<session-id>.document.yaml`. For a resumed session the prompt adds one sentence: the file may already hold the last document, read it before continuing. The user's own request follows after a blank line.

`planningDocumentPath` rejects a session ID that is empty or contains a path separator or `..` (the ID comes from a file name in `.kairon/sessions/`, so the check is defence in depth).

**Classic** (`commands.go`, `tui.go`):

- `handlePlanSubprocess` is split. The new `launchClassicPlanning(sessionID, prompt, statusLine string)` holds the console-state preservation, banner and `tea.ExecProcess` part. The command is built by a pure function `classicPlanningCmd(sessionID, prompt string) *exec.Cmd` so a test can read its `Args`. A model field `execPlanning func(cmd *exec.Cmd, sessionID string) tea.Cmd` (default wraps `tea.ExecProcess` and returns `execDoneMsg{err, sessionID}`) lets tests record the command without starting a process.
- The session lookup keeps its current shape, with one change: only sessions created for classic (`PlanningData == nil`) are reused, because an ACP tab's session ID names a tab, not a terminal session. When none exists, `sessionManager.Create(session.Planning)` makes one. If that fails, the launch stops with an error line, because a session without an ID would silently lose its document.
- `execDoneMsg` gains `sessionID string`. Its handler keeps the existing cleanup and adds `m.finishPlanning(planningEnd{SessionID: msg.sessionID, Resume: classicResume(msg.sessionID)})` to the returned batch. It runs whether or not the process exited with an error, because a user may quit after approving.
- `classicResume` calls `launchClassicPlanning(sessionID, planningResumePrompt(sessionID, fc), ...)`. Retiring classic means deleting `handlePlanSubprocess`, `launchClassicPlanning`, `classicPlanningCmd`, `classicResume` and the `execDoneMsg` case. `finishPlanning` is untouched.

**ACP** (`planning_tab.go`, `tui.go`):

- `sendMessage` prefixes the message with `planningPrompt(pt.sessionID, message, resumed)` the first time a message is sent over an ACP connection. The decision is made in `Update` before the command closure runs (field `pathAnnounced`, reset when the client is not connected, so a restored tab on a new connection announces again). `resumed` is true when the tab already holds earlier messages. The text shown in the tab stays the user's own message. If the tab has no session ID (no session manager), nothing is added and the handoff is a no-op.
- The `done` stream event, and the non-error `isComplete` response, return a `planningTurnEndedMsg{tabID, sessionID}` command. `model.Update` handles it with `m.finishPlanning(planningEnd{SessionID: msg.sessionID, Resume: acpResume(msg.tabID)})`. Every turn end runs the file check; a turn with no file is a no-op by FR4.
- `acpResume(tabID)` finds the tab with `tabManager.GetPlanningTabByID`, makes it the active tab, and calls a new `PlanningTab.ResumeAfterRejection(fc) tea.Cmd`. That method adds a system message to the tab, then sends `planningResumePrompt` (which always includes the path) through the shared message-start code extracted from the `enter` handler. If the tab is closed, the console shows an error line and the file stays.
- **Message routing (needed for AC2 to work in the program, not only in a unit test).** `model.Update` has no case for `planningStreamStartMsg`, `planningStreamMsg` or `planningResponseMsg`; they fall through to the footer input, so the real model never delivers a stream `done` to the tab. The builder confirms this first with a failing test that drives `model.Update`. The fix adds a `tabID` to the three messages and a case in `model.Update` that forwards each to `tabManager.GetPlanningTabByID(msg.tabID)`. Messages with no matching tab are dropped. This is routing only. Rendering and focus behavior (#271) are not touched.

### 4. Agent files

`.kiro/agents/planner-prompt.md` (and the template copy) gain these changes, worded to pass the existing prompt tests (no "mode", "automated", "unattended", "watcher", and none of the shared-contract terms listed in `TestInteractivePromptDoesNotRestateSharedContractOrSelectMode`):

- Task step 10 becomes: after the label turn, write the same document to the path given on the `Document path:` line of the first message, with the `write` tool, then return the document as the whole reply. If no `Document path:` line was given, only return it.
- A message that only names a path is not a request: ask what the person wants to plan.
- A message that says the previous document was rejected names the check, what was expected and what was found. Revise the document with the person if needed, rewrite the file, and return it.
- The constraint "Never write, modify or create project files" gets one stated exception: the file at the given `Document path:`. The prompt says that file is Kairon's hand-off and that the reply is still the document. This resolves the `planner-shared` line "No writes to project files ... Your result is your reply", which stays unchanged, because the prompt decides where the skill leaves a point open.
- The "Once you return the document, add nothing before or after it" constraint stays.

`planner.json` gets a second `allowedPaths` entry, `.kairon/sessions/*.document.yaml`.

### 5. Working tree caveat (environment-injected changes)

The working tree has uncommitted changes to every `.kiro/agents/*.json` that are not part of this issue (an `@creds-agent` tool, `mcpServers`, reformatted JSON). They already make `task sync:check`, `TestInteractiveManifest` (tools list), `TestInteractiveManifestHasNoInjectedCapabilities` and `TestInteractiveFilesMatchTemplates` fail in this worktree, independent of this change. Rules for the builder:

- Edit live `planner.json` by adding only the `allowedPaths` entry. Do not revert or reformat anything else.
- Edit `cmd/kairon/templates/kiro/agents/planner.json` (it matches HEAD's live file) by adding the same entry. Never copy the live `.kiro/agents/*.json` into the templates (`cp .kiro/agents/*.json` from the sync recipe would copy the injected content).
- Commit the live manifest from a clean blob, not from the working tree file:
  `git update-index --cacheinfo 100644,$(git hash-object -w cmd/kairon/templates/kiro/agents/planner.json),.kiro/agents/planner.json`.
  This stages HEAD's formatting plus the one new entry. The injected edits stay unstaged.
- Stage by explicit path. Never use `git add -A`, `git add .` or `git add .kiro/agents`.
- Copy `planner-prompt.md` to the template with `cp .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/` (the prompt has no injected edits).
- A change is verified in a CI-equivalent copy of the committed tree: `d=$(mktemp -d) && git archive HEAD | tar -x -C "$d" && cd "$d" && task sync:check && go test ./internal/planner/... && go test ./internal/eval -run TestPlannerSuite`. In a clean tree `task sync:check` exits 0, which is what CI runs.

## Relevant Files

Create:
- `internal/tui/planning_handoff.go`: `planningBoundary`, `planningEnd`, path and prompt builders, `finishPlanning`, `planningHandoffDoneMsg`, `handlePlanningHandoffDone`, `newPlanningBoundary`.
- `internal/tui/planning_handoff_test.go`: handoff tests (AC1, AC3, FR4 to FR6, FR9, EC1 to EC3, AC6 path rules).
- `internal/tui/planning_rejection.go`: `planningRejection`, `openPlanningRejection`, key handling.
- `internal/tui/planning_rejection_test.go`: AC4, AC5 dialog tests.
- `internal/tui/planning_acp_handoff_test.go`: ACP turn end, ACP launch prompt, ACP resume, routing.

Modify:
- `internal/tui/tui.go`: `overlayPlanningRejection`, model fields (`planBoundary`, `planningHandoffs`, `planningRejection`, `execPlanning`), `execDoneMsg` handler, new `Update` cases (`planningHandoffDoneMsg`, `planningTurnEndedMsg`, stream-message routing), rejection key handling, `newModel` wiring.
- `internal/tui/commands.go`: split `handlePlanSubprocess`, add `launchClassicPlanning`, `classicPlanningCmd`, `classicResume`; `execDoneMsg` gains `sessionID`.
- `internal/tui/planning_tab.go`: `tabID` on stream messages, `planningTurnEndedMsg`, path announcement in `sendMessage`, `ResumeAfterRejection`, shared message-start helper.
- `internal/tui/planner_agent_test.go`: keep passing (both launch files still use `planner.AgentInteractive`).
- `.kiro/agents/planner-prompt.md`, `cmd/kairon/templates/kiro/agents/planner-prompt.md`.
- `.kiro/agents/planner.json` (one entry only), `cmd/kairon/templates/kiro/agents/planner.json`.
- `internal/planner/interactive_agent_test.go`: new assertions for the write instruction and the `allowedPaths` entry; `TestInteractiveManifest` expects both `allowedPaths` entries.
- `docs/planner.md`: remove the known gap bullet, update the tools row (`write` limited to the planning worktrees and the session document files), describe the handoff.

Reference, not modified: `internal/planner/boundary.go`, `internal/planner/run.go`, `internal/agent/intake.go` (collaborator wiring), `internal/github/issuestore.go`, `internal/session/manager.go`, `.kiro/skills/planner-shared/SKILL.md`, `internal/stageoutput`, `.kiro/agents/planner-automated*.`

## Team Orchestration

```
rejection-dialog ──> handoff-core ──> classic-launch-site ──> acp-launch-site ──┐
planner-agent-files ────────────────────────────────────────────────────────────┼──> planner-docs ──> validate-all
```

- `planner-agent-files` touches no Go code under `internal/tui` and runs in parallel with the TUI chain.
- The TUI chain is sequential. `classic-launch-site` and `acp-launch-site` both edit `tui.go`, so they do not run in parallel.
- Each task commits its tests and implementation together, in a commit that builds and passes. The commit body names the test that was seen failing first.
- `planner-docs` starts once the behavior is final, so the doc describes what shipped.
- `validate-all` is read-only.

## Step-by-Step Task Breakdown

### Task 1: `rejection-dialog`
Failing tests first: `TestRejectionDialogShowsGuardExpectedActual` (activates the overlay; content contains the guard name and both strings; the file still exists), `TestRejectionDialogDismissKeepsFile` (`d` and `esc` give `overlayNone`, file present), `TestRejectionDialogResumeCallsFrontEnd` (`r` clears the overlay and calls `Resume` once with the failure), `TestRejectionDialogNilResumeShowsError`. Implement the overlay type, `planningEnd`, `planningRejection`, `openPlanningRejection` and the key handling. Existing overlay tests still pass.

### Task 2: `handoff-core`
Failing tests first, with a fake `planningBoundary` and a temp directory as the working directory:
- committed document: one `Process` call, `Reply` equals the file contents, `Target.Number == 0`, file deleted, success line has the number and URL;
- no file: zero calls, no console line, `overlayNone`;
- `Process` error: error line, file kept;
- rejected result: the dialog opens through task 1, file kept;
- second end after commit: zero calls;
- two ends while the first is in flight: one call;
- file removal failure after commit: warning line that tells the user to delete the file;
- real `planner.Boundary` with a fake store: empty file and non-YAML file give a rejection (guard `extract`); a `punch_out` document gives an error line with the file kept;
- `planningDocumentPath` returns a path with prefix `.kairon/sessions/` and rejects IDs with separators or `..`; `planningPrompt` starts with `Document path: <path>`, adds the resumed sentence only when resumed, and appends the request.

Then implement `planning_handoff.go` and wire `newModel`. Run the two constraint greps.

### Task 3: `classic-launch-site`
Failing tests first: `execDoneMsg{sessionID}` with the file present gives one `Process` call, the file removed after the done message is applied (AC1); with no file, zero calls and `overlayNone` (AC3); `classicPlanningCmd` argv contains `Document path: .kairon/sessions/<id>.document.yaml` for a new and for a resumed session, and the path has the right prefix (AC6); classic Resume (through the dialog) calls `execPlanning` with the same session ID and a prompt containing the guard, expected and actual text (AC5); a session lookup that sees only an ACP-tab session creates a new classic session. Implement the split, the session lookup change and the `execDoneMsg` change. A test helper flattens `tea.BatchMsg` so tests can run the batched commands.

### Task 4: `acp-launch-site`
Failing tests first, with a fake `acp.Client` that records `MessageRequest.Message`:
- a stream `done` through `model.Update` (not `tab.Update`) reaches the tab and, with the file present, produces one `Process` call with the file contents (AC2). This is the test that first shows the routing gap;
- the same with no file gives zero calls and `overlayNone` (AC3);
- the first message sent over a connection contains `Document path: .kairon/sessions/<id>.document.yaml`, a second message on the same connection does not, a message after a reconnect does again, and a restored tab (earlier messages) adds the resumed sentence (AC6);
- ACP Resume sends a message that contains the session ID path and the failure text, activates the tab, and returns an error line when the tab is gone (AC5).

Implement `tabID` on the stream messages, routing, `planningTurnEndedMsg`, the announcement and `ResumeAfterRejection`.

### Task 5: `planner-agent-files`
Failing tests first, in `internal/planner/interactive_agent_test.go`: the prompt contains `Document path:` and a write instruction ordered after the label turn and before the return step, says the reply is still the document, and states the one exception to the no-project-files rule; `TestInteractiveManifestAllowsSessionDocument` reads `toolsSettings.write.allowedPaths` and requires both `.worktrees/planning-*` and `.kairon/sessions/*.document.yaml` (a contains check, so the injected fields do not affect it); `TestInteractiveManifest` expects both entries; the template prompt and the template manifest's `allowedPaths` equal the live ones. Then edit the prompt and manifests under the working tree caveat above. The eval cases are unchanged: their input has no `Document path:` line, so the agent only returns the document.

### Task 6: `planner-docs`
Edit `docs/planner.md`: delete the known-gap bullet (`grep -c "interactive reply is not captured" docs/planner.md` prints `0`), change the Tools row, and add a section "Committing an interactive session" that describes the per-session file, the prompt line, when the handoff runs for each front end, each outcome (committed, rejected with the dialog and its two choices, `Process` error, no file), the duplicate guard, and that the handoff is front-end-agnostic so removing `plan classic` only deletes its launch site. No new known-gap entries are added. Non-code, so no test first.

### Task 7: `validate-all`
Read-only. Run every validation command below, check each Scope Coverage row against the diff, confirm that the planner-automated files, `RunAutomated`, `planner-shared` and the schema are untouched, and confirm the injected `.kiro/agents/*.json` changes were not committed except as the one-entry `planner.json` change.

## Validation Commands

```bash
# TUI behavior (AC1-AC6)
go test ./internal/tui/...
go vet ./...
test -z "$(gofmt -l .)"

# Constraints
test "$(grep -rn "Boundary.Process\|\.Process(ctx" internal/tui --include='*.go' | grep -v _test | wc -l)" -eq 1
test -z "$(grep -rn "issue create" internal/tui --include='*.go' | grep -v _test)"

# Docs (AC8)
test "$(grep -c "interactive reply is not captured" docs/planner.md)" = "0"

# Agent files, sync and eval suite (AC7) in a CI-equivalent copy of the committed tree.
# Needed because of the injected .kiro/agents/*.json edits in this worktree.
d=$(mktemp -d) && git archive HEAD | tar -x -C "$d" && (cd "$d" && task sync:check && go test ./internal/planner/... && go test ./internal/eval -run TestPlannerSuite)

# Out-of-scope files unchanged (BASE = the commit the worktree branched from, e.g. $(git merge-base HEAD origin/main))
git diff --quiet "$BASE" HEAD -- .kiro/agents/planner-automated.json .kiro/agents/planner-automated-prompt.md .kiro/skills/planner-shared internal/planner/run.go internal/stageoutput
```

In a clean checkout, `task sync:check` and `go test ./internal/planner/...` also pass directly. In this worktree they fail on the injected manifest edits, as described in section 5.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "rejection-dialog"
    agent: "builder"
    description: "Add the planning-rejection overlay: overlayPlanningRejection, the planningEnd and planningRejection types, openPlanningRejection (guard, expected, actual, kept file path, Resume planning / Dismiss) and its key handling (r or enter resumes, d or esc dismisses). TDD: write the failing dialog tests first and commit tests and implementation together."
    dependencies: []
    acceptance_criteria:
      - "A test that opens the dialog with a FailureContext asserts activeOverlay is overlayPlanningRejection and the overlay content contains the guard name, the expected text and the actual text; it was seen failing before the implementation for the expected reason (missing behavior, not a compile error)"
      - "Dismiss (d and esc) leaves activeOverlay == overlayNone and the document file still exists"
      - "Resume (r) clears the overlay and calls planningEnd.Resume exactly once with the failure; a nil Resume shows a console error line and keeps the file"
      - "Existing overlay tests in internal/tui still pass"
      - "Tests and implementation are in one commit, staged by explicit path, no push"
    validation_commands:
      - "go test ./internal/tui/... -run 'RejectionDialog'"
      - "go test ./internal/tui/..."
      - "go vet ./internal/tui/..."
      - "test -z \"$(gofmt -l internal/tui)\""

  - id: "handoff-core"
    agent: "builder"
    description: "Add internal/tui/planning_handoff.go: planningBoundary port, newPlanningBoundary (github.NewIssueStore, github.CLI, ConflictRouter), planningDocumentPath, planningPrompt, planningResumePrompt, finishPlanning (stat, in-flight guard, read, one off-thread Process call with Target.Number 0) and handlePlanningHandoffDone (committed: delete file and success line with number and URL; rejected: open dialog and keep file; error: error line and keep file). Wire the model fields in newModel and the Update case. TDD with a fake boundary and the real planner.Boundary for empty, non-YAML and punch_out files."
    dependencies: ["rejection-dialog"]
    acceptance_criteria:
      - "Committed document: exactly one Process call, Request.Reply equals the file contents, Target.Number == 0, file deleted, console success line contains the issue number and https://github.com/<repo>/issues/<n>"
      - "No file: zero Process calls, no console line, activeOverlay == overlayNone"
      - "Process error: console error line, file kept. Rejected result: dialog opens and file kept. Removal failure after commit: console error line that tells the user to delete the file by hand"
      - "A second finishPlanning after a commit makes zero calls; two finishPlanning calls while the first is in flight make one call"
      - "Real planner.Boundary with a fake store: empty file and non-YAML file open the dialog with guard 'extract'; a punch_out document gives an error line with the file kept (ErrNoPunchOutTarget)"
      - "planningDocumentPath returns a path with prefix .kairon/sessions/ and an error for an empty ID, an ID with a path separator, or an ID containing '..'; planningPrompt begins with 'Document path: <path>', adds the resumed sentence only when resumed, then the request"
      - "grep -rn 'Boundary.Process\\|\\.Process(ctx' internal/tui --include='*.go' | grep -v _test shows exactly one line; grep -rn 'issue create' internal/tui --include='*.go' | grep -v _test prints nothing"
      - "Failing tests were seen failing for the expected reason before implementation; tests and implementation in one commit"
    validation_commands:
      - "go test ./internal/tui/... -run 'Handoff|PlanningDocument|PlanningPrompt'"
      - "go test ./internal/tui/..."
      - "test \"$(grep -rn 'Boundary.Process\\|\\.Process(ctx' internal/tui --include='*.go' | grep -v _test | wc -l)\" -eq 1"
      - "test -z \"$(grep -rn 'issue create' internal/tui --include='*.go' | grep -v _test)\""
      - "go vet ./internal/tui/..."
      - "test -z \"$(gofmt -l internal/tui)\""

  - id: "classic-launch-site"
    agent: "builder"
    description: "Wire the classic launch site: split handlePlanSubprocess into launchClassicPlanning plus the pure classicPlanningCmd, reuse only classic-created sessions (PlanningData == nil) and create one when none exists, put planningPrompt in the argv for new and resumed sessions, add sessionID to execDoneMsg, call finishPlanning from the execDoneMsg handler (also when the process exited with an error), add classicResume, and add the execPlanning seam for tests. TDD."
    dependencies: ["handoff-core"]
    acceptance_criteria:
      - "execDoneMsg{sessionID} with the document file present (valid canonical document, fake boundary): after running the returned commands and applying the done message there is one Process call with the file contents and Target.Number 0, and the file is gone"
      - "execDoneMsg with no file: zero Process calls and activeOverlay == overlayNone"
      - "classicPlanningCmd args contain 'Document path: .kairon/sessions/<id>.document.yaml' for a new session and a resumed session, and the path has prefix .kairon/sessions/"
      - "Classic Resume from the dialog calls execPlanning with the same session ID and a prompt containing the guard, expected and actual text; Dismiss leaves overlayNone and the file present"
      - "A planning session that belongs to an ACP tab (PlanningData set) is not reused by plan classic; a new session is created; a Create failure shows an error line and does not launch"
      - "TestInteractiveLaunchSitesUsePlannerAgentInteractive still passes; tests and implementation in one commit"
    validation_commands:
      - "go test ./internal/tui/... -run 'Classic|ExecDone|LaunchSites'"
      - "go test ./internal/tui/..."
      - "go vet ./internal/tui/..."
      - "test -z \"$(gofmt -l internal/tui)\""

  - id: "acp-launch-site"
    agent: "builder"
    description: "Wire the ACP planning tab: first confirm with a failing model.Update test that stream messages never reach the tab, then add tabID to planningStreamStartMsg, planningStreamMsg and planningResponseMsg and route them to the owning planning tab; emit planningTurnEndedMsg on stream done and non-error completion; handle it in model.Update by calling finishPlanning with acpResume; announce the Document path on the first message of each ACP connection (resumed sentence when the tab has earlier messages); add PlanningTab.ResumeAfterRejection and acpResume (activate tab, send the resume prompt, error line if the tab is closed). Do not change rendering or focus behavior. TDD with a fake acp.Client."
    dependencies: ["classic-launch-site"]
    acceptance_criteria:
      - "A stream 'done' delivered through model.Update (not tab.Update) with the document file present and a fake boundary makes exactly one Process call with the file contents, the same finishPlanning used by classic"
      - "The same turn end with no file makes zero Process calls and leaves activeOverlay == overlayNone"
      - "The first message a fake acp.Client receives contains 'Document path: .kairon/sessions/<id>.document.yaml'; a second message on the same connection does not; a message after a reconnect does; a tab restored with earlier messages adds the resumed sentence; the text shown in the tab stays the user's own message"
      - "ACP Resume sends a message containing the session ID path and the failure text to the originating tab and makes it the active tab; when the tab is gone it shows a console error line and keeps the file"
      - "Existing planning tab tests still pass without edits other than adding the tabID field where messages are constructed"
      - "Tests and implementation in one commit"
    validation_commands:
      - "go test ./internal/tui/... -run 'ACP|PlanningTab|TurnEnd|Routing'"
      - "go test ./internal/tui/..."
      - "test \"$(grep -rn 'Boundary.Process\\|\\.Process(ctx' internal/tui --include='*.go' | grep -v _test | wc -l)\" -eq 1"
      - "go vet ./internal/tui/..."
      - "test -z \"$(gofmt -l internal/tui)\""

  - id: "planner-agent-files"
    agent: "builder"
    description: "Update the interactive planner agent files. Tests first in internal/planner/interactive_agent_test.go (prompt has the Document path write instruction after the label turn and before the return step, says the reply is still the document, states the single exception to the no-project-files rule; manifest allowedPaths holds .worktrees/planning-* and .kairon/sessions/*.document.yaml; template prompt equals live prompt; template allowedPaths equals live). Then edit .kiro/agents/planner-prompt.md and cmd/kairon/templates/kiro/agents/planner-prompt.md (identical), add only the allowedPaths entry to live .kiro/agents/planner.json without touching the injected content, add the same entry to cmd/kairon/templates/kiro/agents/planner.json, and stage the live manifest from the clean template blob via git update-index --cacheinfo. Never copy live .kiro/agents/*.json into templates and never git add -A."
    dependencies: []
    acceptance_criteria:
      - "New tests were seen failing for the expected reason before the edits, and pass after"
      - "The prompt still passes the existing prompt tests (RTCC headings, elicitation flow, never punch_out, no gh issue create outside a prohibition, no restated shared content, no mode-selection words)"
      - "planner-automated.json, planner-automated-prompt.md, planner-shared skill, run.go and the schema are unchanged"
      - "diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md prints nothing"
      - "git show HEAD:.kiro/agents/planner.json differs from git show :.kiro/agents/planner.json (the staged blob) only by the new allowedPaths entry, and no other .kiro/agents/*.json is staged"
      - "In a git archive HEAD copy, task sync:check exits 0 and go test ./internal/planner/... and go test ./internal/eval -run TestPlannerSuite exit 0"
    validation_commands:
      - "go test ./internal/planner/... -run 'TestInteractivePrompt|TestInteractiveManifestAllowsSessionDocument'"
      - "diff .kiro/agents/planner-prompt.md cmd/kairon/templates/kiro/agents/planner-prompt.md"
      - "grep -q '.kairon/sessions/\\*.document.yaml' cmd/kairon/templates/kiro/agents/planner.json"
      - "git diff --cached --name-only -- .kiro/agents | grep -v -x -e .kiro/agents/planner.json -e .kiro/agents/planner-prompt.md ; test $? -eq 1"
      - "d=$(mktemp -d) && git archive HEAD | tar -x -C \"$d\" && (cd \"$d\" && task sync:check && go test ./internal/planner/... && go test ./internal/eval -run TestPlannerSuite)"

  - id: "planner-docs"
    agent: "builder"
    description: "Update docs/planner.md: remove the 'The interactive reply is not captured' known-gap bullet, update the planner Tools row to say write is limited to the planning worktrees and the per-session document files, and add a section 'Committing an interactive session' describing the per-session file under .kairon/sessions/, the Document path prompt line, when the handoff runs in each front end, the four results (committed, rejected with the dialog and Resume planning / Dismiss, Process error, no file), the duplicate-issue guard, and that the handoff is front-end-agnostic so removing plan classic only deletes its launch site. Add no new known-gap entries. Non-code change, no test first."
    dependencies: ["handoff-core", "classic-launch-site", "acp-launch-site", "planner-agent-files"]
    acceptance_criteria:
      - "grep -c 'interactive reply is not captured' docs/planner.md prints 0"
      - "docs/planner.md describes the handoff, the rejection dialog and its two choices, and the kept-file behavior, matching the shipped code"
      - "No sentence in docs/planner.md defers work to an unnamed follow-up"
    validation_commands:
      - "test \"$(grep -c 'interactive reply is not captured' docs/planner.md)\" = \"0\""
      - "grep -q 'Document path' docs/planner.md"
      - "grep -q 'Resume planning' docs/planner.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every Scope Coverage row of issue 404 against the final diff and run all validation commands. Read-only."
    dependencies: ["planner-docs"]
    acceptance_criteria:
      - "All eight acceptance criteria of issue 404 are verified criterion by criterion with command output"
      - "Exactly one Process call site in non-test internal/tui files, and no 'issue create' text there"
      - "planner-automated agent files, RunAutomated, planner-shared skill and the stage-output schema are unchanged"
      - "Only planner.json from the injected .kiro/agents/*.json set is committed, and only with the one new allowedPaths entry"
      - "The spec's Deferred work is none and no code, doc or comment added by this change refers to deferred work without an issue number"
      - "go test ./internal/tui/..., go vet ./... and gofmt pass; task sync:check, go test ./internal/planner/... and go test ./internal/eval -run TestPlannerSuite pass in a git archive HEAD copy"
    validation_commands:
      - "go test ./internal/tui/..."
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
      - "test \"$(grep -rn 'Boundary.Process\\|\\.Process(ctx' internal/tui --include='*.go' | grep -v _test | wc -l)\" -eq 1"
      - "test -z \"$(grep -rn 'issue create' internal/tui --include='*.go' | grep -v _test)\""
      - "test \"$(grep -c 'interactive reply is not captured' docs/planner.md)\" = \"0\""
      - "d=$(mktemp -d) && git archive HEAD | tar -x -C \"$d\" && (cd \"$d\" && task sync:check && go test ./internal/planner/... && go test ./internal/eval -run TestPlannerSuite)"
```
