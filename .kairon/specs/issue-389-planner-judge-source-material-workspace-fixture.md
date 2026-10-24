# Design: Give the planner evidence-verification judge its source material via a workspace fixture

Closes #389

## Problem

The planner eval suite (#387) asks the judge whether the planner's evidence is real. Two judge checks do this:

| Case | Question (abridged) |
|------|---------------------|
| `automated-best-effort-fill` (criterion `best_effort_then_punch_out`) | "Does every entry in the inferences list cite evidence that is a real quote or a real file reference taken from the issue or the repository context that was provided…?" |
| `interactive-return-document` (criterion `requirements_boundary`) | "Is every requirement … grounded in something the requester chose or in repository evidence quoted earlier in the conversation, with no invented constraint?" |

Neither check lists `files`, so the judge sees only the case input and the agent reply. It cannot verify any citation, so a fabricated `internal/export/ndjson.go:12` is judged on plausibility alone.

Root cause, verified in the code:

- `setup` file entries are read by `assemblePrompt` (`internal/eval/runner.go`, via `rebaseEvalsPath`) and concatenated into the agent prompt. They are never written into the case workspace.
- `loadJudgeFiles` -> `resolveInDir` (`internal/checks/checks_judge.go`) resolves `files` against `cr.WorkspaceDir`. A listed file that is not in the workspace fails the whole check with `file does not exist`, before any judge call.
- `newCaseWorkspace` (`internal/eval/workspace.go`) stages only: the `tc.Workspace` fixture (via `copyTree`), mocks, the fake gh, `.kiro`, and the gh issue. No planner case sets `workspace:`.
- `.kairon/evals/fixtures/workspaces/` does not exist today.

## Solution Approach

Use the existing workspace-fixture mechanism and add no staging path.

1. **One workspace fixture, `planner-evidence`.** Create `.kairon/evals/fixtures/workspaces/planner-evidence/` holding the two files the evidence judge checks against, at the workspace root:
   - `planner-intake-feature-issue.md`
   - `planner-repo-context.md`

   `newCaseWorkspace` copies the tree with `copyTree`, so the judge's `files: [planner-intake-feature-issue.md, planner-repo-context.md]` resolve.

2. **Single source of truth = the same file, not a copy.** `git mv` the two files out of `.kairon/evals/fixtures/` into the workspace fixture, and repoint every `setup` path that referenced them to the workspace-fixture path (`.kairon/evals/fixtures/workspaces/planner-evidence/<file>`). The agent prompt (`assemblePrompt`) and the judge (`loadJudgeFiles`, from the staged copy of that same directory) therefore read the same bytes from one file in the repository. There is no second copy to drift. Symlinks are not an option: `copyTree` skips them.

   Six cases reference `planner-repo-context.md` and one also references the feature issue. All are repointed, so no stale duplicate remains under `fixtures/`. Other planner fixtures (`planner-intake-bug-issue.md`, `planner-intake-punchout-issue.md`, `planner-large-request.md`) stay where they are: no evidence-verification judge check reads them.

3. **Apply it to every evidence-verification judge check.** Both checks above get `files:` and their case gets `workspace: planner-evidence`:
   - `automated-best-effort-fill`: `files: [planner-intake-feature-issue.md, planner-repo-context.md]`
   - `interactive-return-document`: `files: [planner-repo-context.md]` (the request itself is the case `input`; repository evidence is the repo context).

   The other judge checks (no-question-asked, both-decisions-inferred, bug-report shape, option recommendation, Verify lines, dependency order) do not verify evidence against sources and are left unchanged. A test (below) pins this rule so a future evidence check cannot be added without sources.

4. **Ship the fixture with the cases (important consequence of `workspace:`).** `kairon init`/`kairon update` extract the whole `cmd/kairon/templates` tree (`internal/templates/extract.go`). A case that sets `workspace:` fails `validateCaseFields` ("workspace fixture … not found") when the fixture directory is absent, and that error fails loading the whole agent's cases. So the fixture must be mirrored into `cmd/kairon/templates/kairon/evals/fixtures/workspaces/planner-evidence/`. Today `task sync:check` excludes `workspaces` by name, which would leave the mirror unverified. Therefore:
   - `Taskfile.yml` `sync:check` keeps its `--exclude=workspaces --exclude=hidden` (workspaces remain live-only by default) and adds a loop that diffs every workspace directory present under the template `fixtures/workspaces/` against its live counterpart.
   - A Go test asserts the invariant from the other side: every `workspace:` named by a shipped case exists in the templates and is byte-identical to the live one.
   - The `builder-conventions` skill gets the sync rule and copy command for mirrored workspaces.

5. **Prove the judge gets the sources, and that fabricated evidence fails.** A scripted `stub.judge` answer cannot prove either (it answers yes/no regardless of input). So the test uses a deterministic *citation-grounding judge* implementing `checks.JudgeFunc`: it extracts every `path:line` token from the reply's `evidence:` lines and answers yes only if every token appears in the content of `JudgeQuery.Files`. With no files it answers no with a distinct reason ("no source files were provided"); with a missing citation it answers no naming the citation ("not found in the provided sources: …"). The test runs the case's real judge check through `EvaluateChecks` against a real `newCaseWorkspace` of the real case, so file resolution, caps and fencing are the production code path.

   Negative sample: a recorded fabricated reply `internal/eval/testdata/planner-fabricated/automated-best-effort-fill.txt`, identical to the reference reply except that it cites a file absent from the fixtures (`internal/export/ndjson.go:12`), a real file with a nonexistent line (`internal/export/csv.go:99`), and an invented quote. It must score FAIL with the "not found" reason. Asserting the *specific* reason is what makes the test fail for the judge's reason rather than vacuously (a judge with no sources would also say no, but with the other reason).

### Why not the alternatives

- *Make `loadJudgeFiles` read `setup` files, or stage `setup` into the workspace:* a new staging path, explicitly forbidden, and it would change semantics for every judge check.
- *Copy the fixtures into the workspace fixture and keep the originals:* two copies that can drift; needs a guard test and still risks a divergent prompt vs judge view.
- *Make the workspace fixture hold all planner sources:* the real-backend agent runs in that workspace and can read every file in it. Keeping the fixture to only what the evidence checks need avoids exposing other cases' issues to the agent.

### Known limitations (state in docs, no change)

- The workspace is also the agent's working directory under `kiro-cli`, so a real agent could read the two files there. They are exactly the content already in its prompt, so this adds no information.
- `interactive-return-document` shares the fixture with the best-effort case and therefore sees an unused feature-issue file in its workspace. Accepted to avoid a duplicate `planner-repo-context.md`.
- `automated-bug-classification` records inferences with evidence but has no evidence-verification judge check today. Adding one is a new check, not in scope for this issue.
- The deterministic grounding judge proves plumbing and discrimination, not the real LLM judge's behavior; the latter cannot be tested offline.

## Relevant Files

Create:
- `.kairon/evals/fixtures/workspaces/planner-evidence/planner-intake-feature-issue.md` (moved, content unchanged)
- `.kairon/evals/fixtures/workspaces/planner-evidence/planner-repo-context.md` (moved, content unchanged)
- `cmd/kairon/templates/kairon/evals/fixtures/workspaces/planner-evidence/` (byte-identical mirror of the above two files)
- `internal/eval/planner_evidence_test.go` (grounding judge, source-identity, audit, negative-sample and mirror tests)
- `internal/eval/testdata/planner-fabricated/automated-best-effort-fill.txt` (recorded fabricated reply)

Modify:
- `.kairon/evals/cases/planner/automated-best-effort-fill.yaml` (`workspace:`, setup paths, `files:` on the evidence check)
- `.kairon/evals/cases/planner/interactive-return-document.yaml` (same)
- `.kairon/evals/cases/planner/automated-bug-classification.yaml`, `automated-punch-out.yaml`, `interactive-option-rules.yaml`, `interactive-single-decision.yaml` (setup path for the moved repo context only)
- `cmd/kairon/templates/kairon/evals/cases/planner/*.yaml` (byte-identical mirrors of the six edited cases)
- `internal/eval/planner_suite_test.go` (`copyPlannerSuite` must also copy `fixtures/workspaces/**`; the `planner-*.md` glob no longer finds the moved files)
- `Taskfile.yml` (`sync:check` loop for mirrored workspaces)
- `.kiro/skills/builder-conventions/SKILL.md` (sync table row, copy command)
- `docs/evaluation.md` (judge `files` section, Case Workspaces sync sentence at ~line 2919 and ~407, planner suite section "Fixtures are `fixtures/planner-*.md`")

Delete (git mv): `.kairon/evals/fixtures/planner-intake-feature-issue.md`, `.kairon/evals/fixtures/planner-repo-context.md`, and their copies in `cmd/kairon/templates/kairon/evals/fixtures/`.

Read-only references: `internal/eval/workspace.go` (`newCaseWorkspace`, `copyTree`, `validateCaseFields`), `internal/eval/runner.go` (`assemblePrompt`, `judgeCaseInput`), `internal/checks/checks_judge.go` (`loadJudgeFiles`, `evalJudge`), `internal/checks/prompt.go` (`BuildYesNoPrompt`), `internal/eval/suite.go` (`workspace()` already folds workspace files into `suite_sha256`, so provenance covers the new fixture with no change).

Do not edit: anything under `.kairon/specs/` other than this file, `.kairon/evals/results/`, `CHANGELOG.md` (AGENTS.md).

## Team Orchestration

Single PR. Dependency graph:

```
red-evidence-tests -> wire-workspace-fixture -> mirror-and-sync-guard -+-> validate-all
                                             \-> document-judge-sources -+
```

- `red-evidence-tests` first (AGENTS.md test-first). Its tests must be run and observed failing for the stated reasons before wiring. They are **not committed alone**.
- `wire-workspace-fixture` makes them pass. Test and wiring are committed together in one commit, whose message names the tests seen failing first.
- `mirror-and-sync-guard` and `document-judge-sources` touch disjoint files and can run in parallel after wiring.
- `validate-all` is read-only.

Commit policy (AGENTS.md): a commit never leaves CI red. Per the Brazil/git steering: build before committing, never push.

## Step-by-Step Task Breakdown

### Task 1: red-evidence-tests

Write `internal/eval/planner_evidence_test.go` and the recorded fabricated reply. Contents:

1. `groundingJudge() checks.JudgeFunc`: described above. Citation regex: `[A-Za-z0-9_./-]+\.(go|md|yaml|yml|json|sh):\d+`. Scan only `evidence:` lines of the reply (`(?m)^\s*evidence:.*$`). Source text = concatenation of `q.Files[i].Content`. Return `JudgeVerdict{Yes: false, Reasoning: "no source files were provided to verify the evidence"}` when `len(q.Files)==0`, `"citation <c> not found in the provided sources"` for the first missing token, else `Yes: true`. Also return no when the reply has zero citations (an `evidence:`-less reply cannot pass).
2. `TestPlannerEvidenceChecksHaveSources` (audit, AC3): load the shipped planner cases (`configure(RunOptions{Backend:"stub", EvalsDir: p.evals})`, `loadCases("planner")`). For every judge check whose question matches `(?i)\bevidence\b|\bgrounded\b|\breal (quote|file)` assert `len(c.Files) > 0`; assert at least two such checks exist (best-effort-fill and interactive-return-document) so the audit cannot pass vacuously.
3. `TestPlannerEvidenceSourcesAreTheSetupBytes` (AC1, single source): for each such check, the case has `Workspace != ""`; every `files` entry exists in `fixtures/workspaces/<ws>/`; and for each `files` entry there is a `setup` entry whose rebased path is exactly `fixtures/workspaces/<ws>/<entry>`. Assert `assemblePrompt(tc.Setup, …)` contains the file's exact bytes. This pins "judge reads the same bytes the agent was given".
4. `TestPlannerEvidenceJudgeLoadsSources` (AC1, runtime): `newCaseWorkspace(tc)` for `automated-best-effort-fill`; run the case's evidence judge check through `EvaluateChecks` with `CheckInput{Dir: ws.Dir, Output: <stub reference reply>, Input: judgeCaseInput(tc), Judge: <recording wrapper of groundingJudge>}`. Assert: the check passes; its detail does not contain `file does not exist`; the recorded `JudgeQuery.Files` paths are exactly `[planner-intake-feature-issue.md, planner-repo-context.md]`, in that order, and each `Content` equals `os.ReadFile` of the corresponding setup path.
5. `TestPlannerEvidenceFabricatedReplyFails` (AC2): same, but `Output` is the recorded fabricated reply. Assert the check fails, detail contains `judge answered no` and `not found in the provided sources` and `internal/export/ndjson.go:12`, and does not contain `no source files`. Also assert the unmodified reference reply passes, so the failure is attributable to the fabricated citations only.
6. `testdata/planner-fabricated/automated-best-effort-fill.txt`: the reference reply from `automated-best-effort-fill.yaml` `stub.turns[0].response`, with three `evidence:` values replaced: a nonexistent file (`internal/export/ndjson.go:12`), a real file at a nonexistent line (`internal/export/csv.go:99`), and an invented quoted claim attached to a nonexistent path. It must still be a valid stage-output document (`stageoutput.Parse` + `Validate`), so the sample is realistic and only the evidence is wrong. Add that parse check to the test.
7. Update `copyPlannerSuite` in `planner_suite_test.go` to also copy every regular file under `<evals>/fixtures/workspaces/` (preserving relative paths). Without this the existing suite tests fail with `workspace fixture … not found` after wiring.

Run `go test ./internal/eval -run 'TestPlannerEvidence' -count=1` and record the **expected failures before wiring**:
- audit: the two evidence checks have no `files`;
- source-identity: `Workspace` is empty;
- loads-sources: case has no `files`, so the recording judge sees zero files and the grounding judge answers `no source files were provided`;
- fabricated: fails on the `not found in the provided sources` assertion (the judge's reason is "no source files", not the missing citation). If any of these fail for a compile error or unrelated setup problem, fix the test first.

Do not commit.

Acceptance criteria:
- The new tests compile and fail pre-wiring for the reasons listed above (the failing output is saved to the task's sentinel summary).
- The fabricated sample parses as a valid planner `canonical` document.
- `copyPlannerSuite` copies `fixtures/workspaces/**`.

Dependencies: none.

### Task 2: wire-workspace-fixture

1. `git mv .kairon/evals/fixtures/planner-intake-feature-issue.md .kairon/evals/fixtures/workspaces/planner-evidence/` and the same for `planner-repo-context.md`. Content unchanged.
2. Edit the six planner case files:
   - Add `workspace: planner-evidence` to `automated-best-effort-fill.yaml` and `interactive-return-document.yaml` only.
   - In all six, change setup `path:` for the moved files to `.kairon/evals/fixtures/workspaces/planner-evidence/<file>`.
   - `automated-best-effort-fill.yaml` check "real quote or real file reference": add `files: [planner-intake-feature-issue.md, planner-repo-context.md]`.
   - `interactive-return-document.yaml` check "grounded in … repository evidence": add `files: [planner-repo-context.md]`.
   - Leave the stub transcripts, `stub.judge` lists and every other check byte-for-byte as they are.
3. Run the Task 1 tests: all must pass now.
4. Run `go run ./cmd/kairon eval --backend stub planner` (or the built binary): must pass, and the best-effort-fill evidence check detail must be a `judge answered yes` and not contain `file does not exist`.
5. Commit tests + wiring together (also include the template mirror from Task 3 only if `task sync:check` is required green per commit; otherwise do Task 3 before committing; see Task 3). Commit message names `TestPlannerEvidenceChecksHaveSources` etc. as seen failing first.

Acceptance criteria:
- Both files live only under `fixtures/workspaces/planner-evidence/`; `ls .kairon/evals/fixtures/planner-repo-context.md` fails.
- `grep -rn "fixtures/planner-repo-context\|fixtures/planner-intake-feature" .kairon/evals/cases` returns nothing.
- `go test ./internal/eval -run 'TestPlannerEvidence|TestPlannerSuite' -count=1` passes.
- `kairon eval --backend stub planner` passes.

Dependencies: `red-evidence-tests`.

### Task 3: mirror-and-sync-guard

1. Mirror into templates, byte for byte:
   ```bash
   mkdir -p cmd/kairon/templates/kairon/evals/fixtures/workspaces/planner-evidence
   cp .kairon/evals/fixtures/workspaces/planner-evidence/* cmd/kairon/templates/kairon/evals/fixtures/workspaces/planner-evidence/
   git rm -f cmd/kairon/templates/kairon/evals/fixtures/planner-intake-feature-issue.md cmd/kairon/templates/kairon/evals/fixtures/planner-repo-context.md
   cp -r .kairon/evals/cases/* cmd/kairon/templates/kairon/evals/cases/
   ```
2. `Taskfile.yml` `sync:check`: keep the existing `--exclude=workspaces --exclude=hidden` diff and add, before the success/failure branch:
   ```bash
   # Workspace fixtures that ship with the templates must match the live ones.
   # Shipped cases that set `workspace:` fail to load without them. A workspace
   # absent from the templates stays live-only.
   for d in cmd/kairon/templates/kairon/evals/fixtures/workspaces/*/; do
     [ -d "$d" ] || continue
     diff -rq ".kairon/evals/fixtures/workspaces/$(basename "$d")" "$d" || error=1
   done
   ```
   Keep the YAML block-scalar indentation of the surrounding script.
3. Add to `planner_evidence_test.go` `TestShippedCaseWorkspacesAreMirroredInTemplates`: for every case under `.kairon/evals/cases/**` with `workspace: X` (parse YAML heads; do not depend on `loadCases`), the directory `cmd/kairon/templates/kairon/evals/fixtures/workspaces/X/` exists and every regular file under the live `X/` is byte-identical in the template tree and vice versa (no extra files).
4. Update `.kiro/skills/builder-conventions/SKILL.md`: add a sync-table row `.kairon/evals/fixtures/workspaces/<name>/` (only workspaces named by a shipped case; others stay live-only) -> `cmd/kairon/templates/kairon/evals/fixtures/workspaces/<name>/`, and a `cp -r` command in the Evals block. Do not template-sync this skill (`*-conventions`).
5. Verify: `task sync:check` passes; `diff -rq .kairon/evals/cases cmd/kairon/templates/kairon/evals/cases`, `diff -rq .kairon/evals/fixtures/workspaces/planner-evidence cmd/kairon/templates/kairon/evals/fixtures/workspaces/planner-evidence` and `diff -rq .kairon/evals/rubrics cmd/kairon/templates/kairon/evals/rubrics` print nothing. Negative check (then revert): append a byte to a template workspace file and confirm `task sync:check` fails.

Acceptance criteria:
- `task sync:check` passes.
- Scoped `diff -rq` over cases, rubrics, fixtures (excluding `workspaces`/`results`) and the workspace dir reports no differences.
- `TestShippedCaseWorkspacesAreMirroredInTemplates` passes and fails when a template file is altered.
- Template fixtures dir no longer contains the two moved files.

Dependencies: `wire-workspace-fixture`.

### Task 4: document-judge-sources

Edit `docs/evaluation.md`:
- In "The `judge` check" `files` bullet, add a short note and example: `setup` files go only into the agent prompt; to let the judge read a source file, put it in a workspace fixture, set `workspace:`, list it in `files`, and point the `setup` entry at that same fixture file so the agent and the judge read one file. Reference the planner case as the worked example.
- Line ~407 and ~2919: replace "Fixtures under `fixtures/workspaces/` … are excluded from `task sync:check`" with the precise rule: workspaces are excluded from the bulk comparison; a workspace named by a shipped case must be mirrored in the templates and is diffed individually by `sync:check`; `hidden/` stays excluded.
- Planner suite section (~line 3233): replace "Fixtures are `fixtures/planner-*.md` and are synced…" to say the evidence sources live in `fixtures/workspaces/planner-evidence/` (shared by `automated-best-effort-fill` and `interactive-return-document`), the remaining fixtures are `fixtures/planner-*.md`, and describe the `TestPlannerEvidence*` tests including the recorded fabricated sample and the limitation that the offline test uses a deterministic grounding judge.
- Do not touch `.kairon/specs/`, `CHANGELOG.md` or results.

Acceptance criteria:
- The three places above are updated and consistent with the implementation (names, paths, test names).
- No remaining statement in `docs/` says `fixtures/workspaces/` is wholly excluded from sync.

Dependencies: `wire-workspace-fixture`.

### Task 5: validate-all

Read-only verification of every issue acceptance criterion with command output quoted. See validation commands below.

Dependencies: `mirror-and-sync-guard`, `document-judge-sources`.

## Validation Commands

Run from the repository root.

```bash
# AC1: judge check loads the sources, no 'file does not exist'
go build -o /tmp/kairon-389 ./cmd/kairon
/tmp/kairon-389 eval --backend stub planner
go test ./internal/eval -run 'TestPlannerEvidenceJudgeLoadsSources|TestPlannerEvidenceSourcesAreTheSetupBytes' -count=1 -v

# AC2: fabricated evidence fails the criterion
go test ./internal/eval -run 'TestPlannerEvidenceFabricatedReplyFails' -count=1 -v

# AC3: every evidence-verification judge check has sources
go test ./internal/eval -run 'TestPlannerEvidenceChecksHaveSources' -count=1 -v
grep -n "type: judge" -A3 .kairon/evals/cases/planner/*.yaml | grep -i "evidence\|grounded"
grep -c "files:" .kairon/evals/cases/planner/automated-best-effort-fill.yaml .kairon/evals/cases/planner/interactive-return-document.yaml

# AC4: template mirror
task sync:check
diff -rq .kairon/evals/cases cmd/kairon/templates/kairon/evals/cases
diff -rq .kairon/evals/rubrics cmd/kairon/templates/kairon/evals/rubrics
diff -rq .kairon/evals/fixtures/workspaces/planner-evidence cmd/kairon/templates/kairon/evals/fixtures/workspaces/planner-evidence
go test ./internal/eval -run 'TestShippedCaseWorkspacesAreMirroredInTemplates' -count=1

# AC5: full gate
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "red-evidence-tests"
    agent: "builder"
    description: "Test-first: add internal/eval/planner_evidence_test.go (deterministic grounding judge, evidence-check audit, source-identity test, runtime source-loading test, fabricated-evidence negative test) and the recorded fabricated reply internal/eval/testdata/planner-fabricated/automated-best-effort-fill.txt; extend copyPlannerSuite in planner_suite_test.go to copy fixtures/workspaces/**. Run the new tests before any wiring and record that they fail for the expected reasons (no files on the evidence checks, no workspace, judge reports 'no source files were provided'). Do not commit yet."
    dependencies: []
    acceptance_criteria:
      - "internal/eval/planner_evidence_test.go defines TestPlannerEvidenceChecksHaveSources, TestPlannerEvidenceSourcesAreTheSetupBytes, TestPlannerEvidenceJudgeLoadsSources and TestPlannerEvidenceFabricatedReplyFails, and they compile"
      - "Before wiring, each of those four tests fails, and the failure output shows the intended reason (missing files on evidence checks, empty workspace, 'no source files were provided'), not a compile or setup error"
      - "The fabricated sample parses and validates as a stageoutput planner canonical document and cites internal/export/ndjson.go:12, internal/export/csv.go:99 and an invented quote that are absent from the fixtures"
      - "The audit test asserts at least two evidence-verification judge checks exist so it cannot pass vacuously"
      - "copyPlannerSuite copies every regular file under fixtures/workspaces/ into the temp evals dir"
      - "No commit is made in this task"
    validation_commands:
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/eval)\""
      - "go test ./internal/eval -run 'TestPlannerEvidence' -count=1 2>&1 | grep -c 'FAIL'"

  - id: "wire-workspace-fixture"
    agent: "builder"
    description: "git mv planner-intake-feature-issue.md and planner-repo-context.md into .kairon/evals/fixtures/workspaces/planner-evidence/ (content unchanged); repoint the setup path of all six planner cases that reference them; set workspace: planner-evidence on automated-best-effort-fill and interactive-return-document; add files: to their evidence-verification judge checks ([planner-intake-feature-issue.md, planner-repo-context.md] and [planner-repo-context.md]). Then make the task-1 tests pass and commit tests and wiring together, naming the tests seen failing first in the commit message."
    dependencies: ["red-evidence-tests"]
    acceptance_criteria:
      - "The two fixture files exist only under .kairon/evals/fixtures/workspaces/planner-evidence/ and are byte-identical to the previous files"
      - "No case under .kairon/evals/cases references .kairon/evals/fixtures/planner-repo-context.md or planner-intake-feature-issue.md"
      - "automated-best-effort-fill and interactive-return-document set workspace: planner-evidence, and each setup entry for a judged source points at the workspace fixture file (single source of truth)"
      - "The evidence judge check in each of those two cases lists files; no other judge check or stub transcript changed"
      - "All TestPlannerEvidence* and TestPlannerSuite* tests pass"
      - "kairon eval --backend stub planner passes and the best-effort-fill evidence check detail has no 'file does not exist'"
      - "The commit contains the tests and the wiring together and its message names the tests observed failing first"
    validation_commands:
      - "go test ./internal/eval -run 'TestPlannerEvidence|TestPlannerSuite' -count=1"
      - "go run ./cmd/kairon eval --backend stub planner"
      - "test ! -e .kairon/evals/fixtures/planner-repo-context.md"
      - "test -z \"$(grep -rn 'fixtures/planner-repo-context\\|fixtures/planner-intake-feature' .kairon/evals/cases)\""

  - id: "mirror-and-sync-guard"
    agent: "builder"
    description: "Mirror every edited .kairon/evals file into cmd/kairon/templates/kairon/evals/ byte for byte (cases, the new planner-evidence workspace fixture; remove the two moved fixture files from the template fixtures dir). Add a Taskfile sync:check loop that diffs each workspace dir present in the templates against its live counterpart, add TestShippedCaseWorkspacesAreMirroredInTemplates, and update the builder-conventions skill sync table and copy commands for mirrored workspaces."
    dependencies: ["wire-workspace-fixture"]
    acceptance_criteria:
      - "task sync:check passes"
      - "diff -rq over .kairon/evals/cases, .kairon/evals/rubrics and the planner-evidence workspace dir against the template tree reports no differences"
      - "cmd/kairon/templates/kairon/evals/fixtures/ no longer contains planner-intake-feature-issue.md or planner-repo-context.md"
      - "Altering one byte of a template workspace file makes both task sync:check and TestShippedCaseWorkspacesAreMirroredInTemplates fail (verified once, then reverted)"
      - ".kiro/skills/builder-conventions/SKILL.md documents the mirrored-workspace mapping and copy command and is not itself copied into templates"
    validation_commands:
      - "task sync:check"
      - "diff -rq .kairon/evals/cases cmd/kairon/templates/kairon/evals/cases"
      - "diff -rq .kairon/evals/rubrics cmd/kairon/templates/kairon/evals/rubrics"
      - "diff -rq .kairon/evals/fixtures/workspaces/planner-evidence cmd/kairon/templates/kairon/evals/fixtures/workspaces/planner-evidence"
      - "go test ./internal/eval -run 'TestShippedCaseWorkspacesAreMirroredInTemplates' -count=1"

  - id: "document-judge-sources"
    agent: "documenter"
    description: "Update docs/evaluation.md: explain in the judge files section that setup files reach only the agent prompt and how to give a judge sources via a workspace fixture (one file for setup and files); correct the two statements that fixtures/workspaces/ is wholly excluded from sync:check; update the planner suite section for the planner-evidence workspace fixture and the TestPlannerEvidence tests, including the recorded fabricated sample and the deterministic-grounding-judge limitation."
    dependencies: ["wire-workspace-fixture"]
    acceptance_criteria:
      - "The judge files bullet explains setup vs workspace and gives the single-source pattern"
      - "No statement in docs/ says fixtures/workspaces/ is wholly excluded from task sync:check"
      - "The planner suite section names fixtures/workspaces/planner-evidence/ and the TestPlannerEvidence tests with names that exist in the code"
      - "No file under .kairon/specs/ other than the issue-389 spec, and no CHANGELOG.md, is modified"
    validation_commands:
      - "grep -n 'planner-evidence' docs/evaluation.md"
      - "! grep -n 'excluded from the `task sync:check` comparison of `.kairon/evals`' docs/evaluation.md"
      - "git diff --name-only | grep -v '^\\.kairon/specs/issue-389-' | grep '^\\.kairon/specs/' ; test $? -ne 0"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion in issue #389: stub planner run, fabricated-evidence failure, evidence-check audit, template mirror and sync:check, and the full gate (go test -race ./..., go vet ./..., gofmt -l). Quote the command output for each criterion."
    dependencies: ["mirror-and-sync-guard", "document-judge-sources"]
    acceptance_criteria:
      - "kairon eval --backend stub planner passes and shows no 'file does not exist' for automated-best-effort-fill"
      - "TestPlannerEvidenceJudgeLoadsSources proves the loaded files are the issue and repo-context fixtures with the exact setup bytes"
      - "TestPlannerEvidenceFabricatedReplyFails passes, i.e. the fabricated reply scores FAIL with the 'not found in the provided sources' reason"
      - "TestPlannerEvidenceChecksHaveSources passes and a grep of .kairon/evals/cases/planner/*.yaml shows no evidence-verification judge check without files"
      - "task sync:check passes and scoped diff -rq reports no differences"
      - "go test -race ./..., go vet ./... and gofmt -l report no failures"
      - "git diff shows no change under .kairon/specs/ (other than the issue-389 spec), .kairon/evals/results/ or CHANGELOG.md"
    validation_commands:
      - "go build -o /tmp/kairon-389 ./cmd/kairon && /tmp/kairon-389 eval --backend stub planner"
      - "go test ./internal/eval -run 'TestPlannerEvidence|TestShippedCaseWorkspacesAreMirroredInTemplates|TestPlannerSuite' -count=1 -v"
      - "task sync:check"
      - "go test -race ./..."
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
```
