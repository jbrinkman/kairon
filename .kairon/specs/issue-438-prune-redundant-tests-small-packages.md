# Design Spec: Prune redundant tests in small packages

Closes #438

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/github`: `issuestore_test.go`" | `prune-github-incidents-version`, `validate-all` |
| S2 | In Scope: "`internal/incidents`: `logger_test.go`" | `prune-github-incidents-version`, `validate-all` |
| S3 | In Scope: "`internal/logging`: `file_handler_test.go`, `ring_buffer_test.go`" | `prune-logging`, `validate-all` |
| S4 | In Scope: "`internal/session`: `types_test.go`, `validation_focused_test.go`" | `prune-session`, `validate-all` |
| S5 | In Scope: "`internal/version`: `version_test.go`" | `prune-github-incidents-version`, `validate-all` |
| S6 | In Scope: "`internal/watcher`: `dependencies_test.go`, `watcher_boundary_test.go`" | `prune-watcher`, `validate-all` |
| AC1 | "The 9 tests marked Delete no longer exist." | `prune-github-incidents-version`, `prune-logging`, `prune-session`, `validate-all` (`TestGenerateFilename`, `TestNewFileHandler`, `TestFileHandler_Write`, `TestFileHandler_FilenameFormat`, `TestMetadataHandling`, `TestSessionTypes`, `TestModeSwitchLogging`, `TestPrereleaseConcatenation`, `TestEmptyPrerelease`) |
| AC2 | "Each of the 5 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-logging`, `prune-session`, `prune-watcher`, `validate-all` (`TestFileHandler_Rotation`, `TestSessionManagerCRUD`, `TestValidateIssue`, `TestCheckIssues_AdmissionUnavailableUntracksAndRefunds`, `TestCheckIssues_OtherSpawnErrorStaysTracked`) |
| AC3 | "The 1 group marked Combine is one table-driven test with a case for each former test, and the former functions are gone." | `prune-logging`, `validate-all` (`TestMalformedKeyVals`) |
| AC4 | "`IssueStore`'s interface check is a compile-time assertion." | `prune-github-incidents-version`, `validate-all` |
| AC5 | "Package coverage does not drop." | `prune-github-incidents-version`, `prune-logging`, `prune-session`, `prune-watcher`, `validate-all` |
| AC6 | "Every remaining test passes unchanged, and the gate passes." | `prune-github-incidents-version`, `prune-logging`, `prune-session`, `prune-watcher`, `validate-all` |

## Deferred work

none

## Solution Approach

Test-only change in one PR. 17 test functions across six packages are removed, folded into a partner, or replaced.
No production file is edited, and no new test file is added.

Pre-design measurements (go1.26.5, this worktree, before any edit):

| Package | `go test -cover` today | Floor (AC5) |
|---------|-----------------------|-------------|
| `internal/github` | 71.3% | 71.3% |
| `internal/incidents` | 60.3% | 60.3% |
| `internal/logging` | 53.5% | 53.5% |
| `internal/session` | 28.3% | 28.3% |
| `internal/version` | 30.8% | 30.8% |
| `internal/watcher` | 69.2% | 69.2% |

The floors equal today's values, so there is no slack. I simulated every deletion with `go test -cover -skip '^(…)$'`. I also skipped the
to-be-merged functions entirely as a worst case. Every package still reports exactly its floor. None of the 17 tests is the sole
cover of any statement, so the floors hold even if a merge were done imperfectly. The merges are still specified in full because AC2
and the Behavior-Preservation Contract require every assertion to survive.

### Decision: where the `provenance.IssueStore` assertion lives (AC4)

The issue says "No production code changes" and "ideally in issuestore.go", and AC4 requires
`grep -rn 'var _ provenance.IssueStore' internal/github/*.go` to print **one** line.

Checked against the code: `internal/github/issuestore.go:24` **already contains** `var _ provenance.IssueStore = IssueStore{}`
(package level, so `go build` already fails if the interface drifts). The only other match is `issuestore_test.go:32`, which is the
body of `TestIssueStoreImplementsProvenanceStore`. Therefore:

- Deleting the test function removes the second match and leaves exactly one line, in `issuestore.go`. AC4 is satisfied.
- Adding a new `var _` to a `_test.go` file (the `read_test.go` precedent) would make the grep print **two** lines and fail AC4.
  It would also duplicate `issuestore.go:24`. Rejected.
- Adding or moving a line in `issuestore.go` is unnecessary and would touch production code. Rejected.

So the work for this row is a deletion plus an import cleanup. The "ideally in issuestore.go" wish is already met, and the
"no production change" and "one line" requirements do not conflict. No BLOCKED condition.

The `read_test.go:13` `var _ boundary.Reader = CLI{}` precedent is a `_test.go` placement, but `adapter.go:210` carries the same
assertion in production, so the project's practice is to keep the canonical assertion in production code too. This confirms the
choice.

After removing the function, `issuestore_test.go` no longer uses `provenance` anywhere (line 9 import, line 32 use only).
The import at line 9 **must be removed** or the package's tests fail to compile.

## Relevant Files

All paths are modified; none created. `types_test.go` is deleted because it ends up with no tests.

| File | Change |
|------|--------|
| `internal/github/issuestore_test.go` | Delete `TestIssueStoreImplementsProvenanceStore` (line 31); drop the `internal/provenance` import (line 9) |
| `internal/incidents/logger_test.go` | Delete `TestGenerateFilename` (line 66). `strings` stays used by `TestIncidentLogger` |
| `internal/version/version_test.go` | Delete `TestPrereleaseConcatenation` (19) and `TestEmptyPrerelease` (38); drop the `encoding/json` import (only those two use it). `testing` and `semver` stay |
| `internal/logging/file_handler_test.go` | Delete `TestNewFileHandler` (12), `TestFileHandler_Write` (53), `TestFileHandler_FilenameFormat` (279); fold `TestFileHandler_RotationSequenceNumbers` (374) into `TestFileHandler_Rotation` (100); drop the `time` import (only `FilenameFormat` uses `time.Parse`) |
| `internal/logging/ring_buffer_test.go` | Delete `TestMetadataHandling` (385); replace `TestOddKeyVals` (417) and `TestNonStringKeys` (438) with one table test `TestMalformedKeyVals` |
| `internal/session/types_test.go` | Delete the file (both tests removed; only `package` and a `testing` import would remain) |
| `internal/session/manager_test.go` | Add the Type and Role/Content checks to `TestSessionManagerCRUD` |
| `internal/session/validation_focused_test.go` | Delete `TestModeSwitchLogging` (331, last function in the file). `os`, `fmt`, `filepath`, `time` stay used by earlier tests |
| `internal/watcher/dependencies_test.go` | Add an `issue` field and two cycle rows to `TestValidateIssue` (457); delete `TestValidateIssueCyclicDependencyDoesNotFetchChain` (586). Imports unchanged |
| `internal/watcher/watcher_boundary_test.go` | Turn `TestCheckIssues_AdmissionUnavailableUntracksAndRefunds` (63) and `TestCheckIssues_OtherSpawnErrorStaysTracked` (79) into tables; delete `TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount` (30) and `TestCheckIssues_SuccessfulSpawnStaysTracked` (92). Imports unchanged |

Line numbers in the issue's table were checked against the files and are all correct. Partners all exist under the names given.
Re-grep by function name rather than trusting line numbers once edits start (they shift).

Not touched: `.kiro/agents/*.json` (pre-existing uncommitted modifications unrelated to this issue; do not stage, revert or edit
them, and stage files by explicit path, never `git add -A`), production code, `internal/hotkey` (#421), the
`.kairon/research/` audit documents, and `.kairon/specs/` other than this file.

None of these files is template-synchronized, so `task sync:check` is unaffected.

## Merge Mechanics (exact)

The rule for every merge: the partner keeps every assertion it had; the folded-in test contributes its distinct input and any
assertion the partner lacks; assertions the partner already makes are not duplicated.

### M1. `TestFileHandler_RotationSequenceNumbers` into `TestFileHandler_Rotation` (logging)

`TestFileHandler_Rotation` becomes a parent with two `t.Run` subtests. Each subtest creates its own `MkdirTemp` directory,
because the second case counts files in the directory.

1. Subtest `size threshold rotates to a new file`: the existing body of `TestFileHandler_Rotation`, unchanged (same assertions: path
   changes, both files exist, old file reopenable).
2. Subtest `same-minute handlers get distinct files`: the body of `TestFileHandler_RotationSequenceNumbers` (three `NewFileHandler`
   calls on one config and directory, a write each, close all), with two changes:
   - `len(files) < 2` / "expected at least 2" becomes `len(files) != 3` / "expected exactly 3 log files". This is deterministic:
     each `NewFileHandler` calls `rotate()`, which picks the plain minute name or a `-N` suffixed name, so three handlers always
     yield three distinct files even across a minute boundary.
   - Keep the loop that asserts every file name has the `debug-` prefix.

The other deleted logging tests lose nothing the partner needs: `TestFileHandler_MultipleWrites` already asserts byte count, size and
content (covers `Write`); `TestFileHandler_DirectoryCreation` covers directory creation and the file living under it.

### M2. `TestOddKeyVals` + `TestNonStringKeys` into `TestMalformedKeyVals` (logging, AC3)

One table test in `ring_buffer_test.go`, placed where the two functions were. Fields: `name string`, `keyvals []interface{}`,
`wantMeta map[string]interface{}`. Per case: `rb := NewRingBuffer(10)`, `rb.Add(log.InfoLevel, "test", tt.keyvals...)`, then the
original assertions: exactly 1 entry (`Fatalf`), `len(entries[0].Metadata) == len(tt.wantMeta)`, and each wanted key equals its value.

| Subtest name | keyvals | wantMeta |
|--------------|---------|----------|
| `odd number of keyvals drops the trailing key` | `"key1", "value1", "key2"` | `{key1: value1}` |
| `non-string key is skipped` | `123, "value", "key2", "value2"` | `{key2: value2}` |

`go test -run '^TestMalformedKeyVals$' -v ./internal/logging` must list exactly these two subtests. **The PR description and the final
report must name `TestMalformedKeyVals` as the combined table test** (AC3 says "the PR names the table test").

### M3. `TestSessionStateJSONMarshaling` into `TestSessionManagerCRUD` (session)

CRUD already round-trips through `Save`/`Load` (which use `ToJSON`/`FromJSON`), and its first `Load` already checks
`state.Type == Console`. Add, directly after the existing `len(state2.History) != 1` check on the reloaded `state2`:

- `state2.Type != Console` -> `t.Errorf` (the type survives Save/Load, the original test's type check).
- `state2.History[0].Role != "user" || state2.History[0].Content != "test message"` -> `t.Errorf` (the original role/content check).
  Guard it so it cannot index an empty slice (use `else` of the length check, or make the length mismatch `t.Fatalf`-safe without
  changing the existing assertion's meaning).

Do not change the existing assertions. `types_test.go` is then deleted in full (it holds only the merged test and
`TestSessionTypes`).

### M4. `TestValidateIssueCyclicDependencyDoesNotFetchChain` into `TestValidateIssue` (watcher)

The partner's loop hardcodes issue number 1 (`dv.ValidateIssue("owner/repo", 1, tt.body)`), but the cycle cases depend on issue 10.
Add an `issue int` field to the row struct and, in the loop, `issue := tt.issue; if issue == 0 { issue = 1 }`, then pass `issue`.
Existing rows stay as they are. Add two rows (keep the explanatory comments from the original):

| Row name | issue | body | details | wantLookups | wantValid | wantUnresolved |
|----------|-------|------|---------|-------------|-----------|----------------|
| `two issue cycle only looks up the direct dependency` | 10 | `Dependencies: #11` | `{11: {State: "closed", Body: "Dependencies: #10"}}` | `[11]` | true | none |
| `self referencing issue is looked up once` | 10 | `Dependencies: #10` | `{10: {State: "open", Body: "Dependencies: #10"}}` | `[10]` | false | `[10]` |

The shared loop asserts lookups, `IsValid`, `UnresolvedDependencies` and an empty `CircularDependencies`. That is a superset of what
the removed function asserted (it asserted unresolved only for the self-reference case; for the two-issue row the table also
asserts it is empty, which holds because the dependency is closed).

### M5. `TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount` into `TestCheckIssues_AdmissionUnavailableUntracksAndRefunds` (watcher)

Keep the partner's name (AC2 runs it by name). Make it a table over the spawn error:

| Subtest name | spawn error |
|--------------|-------------|
| `ErrNotAdmitted` | `fmt.Errorf("%w: skipped (done)", agent.ErrNotAdmitted)` |
| `ErrAdmissionUnavailable` | `fmt.Errorf("%w: gh down", agent.ErrAdmissionUnavailable)` |

Per case, call `newSpawnWatcher(t, tt.err)` inside the subtest (each call does `t.Chdir(t.TempDir())`, so state is isolated), run
`w.checkIssues()`, and assert the union of both originals: `*spawned == []int{7}` (this assertion existed only in the NotAdmitted
test; apply it to both rows), `trackedKeys(w)` empty, `getCurrentRetryCount(7) == 0`, and
`.kairon/retries/issue-7.count` removed. Put the error name in the failure messages.
`TestCheckIssues_NotAdmittedRefundKeepsPriorAttempts` is not in scope and stays unchanged.

### M6. `TestCheckIssues_SuccessfulSpawnStaysTracked` into `TestCheckIssues_OtherSpawnErrorStaysTracked` (watcher)

Keep the partner's name. Table over the spawn error:

| Subtest name | spawn error |
|--------------|-------------|
| `other spawn error` | `errors.New("worktree create failed")` |
| `successful spawn` | `nil` |

Both assert `trackedKeys(w) == []int{7}` and `getCurrentRetryCount(7) == 1`. Both `errors` and `reflect` stay imported.

## Team Orchestration

One PR, built by sequential builder tasks (one package group each, no shared files between tasks) and closed by a read-only
validator task. Tasks run one at a time in the order below. Each builder task runs the package's tests with coverage before
finishing and commits only the files it changed, by explicit path.

1. `prune-github-incidents-version`: the three simplest packages (pure deletes plus one import cleanup each).
2. `prune-logging`: deletes, one two-subtest merge (M1) and the `TestMalformedKeyVals` table (M2).
3. `prune-session`: merge M3, two deletes, delete `types_test.go`.
4. `prune-watcher`: merges M4, M5, M6.
5. `validate-all`: run every AC `Verify:` command and `task test`.

TDD note (AGENTS.md): this is a refactor of tests, not a behavior change. The "characterization" requirement is met by the
pre-design coverage measurements above and by each merged partner still passing with the folded-in case present. Say in the commit
messages and the PR which tests were relied on (the partners).

## Step-by-Step Task Breakdown

### Task 1: `prune-github-incidents-version`
- `internal/github/issuestore_test.go`: delete `TestIssueStoreImplementsProvenanceStore`, remove the `provenance` import. Do **not** add any new `var _ provenance.IssueStore` line; `issuestore.go:24` is the single assertion AC4 expects.
- `internal/incidents/logger_test.go`: delete `TestGenerateFilename`.
- `internal/version/version_test.go`: delete `TestPrereleaseConcatenation` and `TestEmptyPrerelease`; remove the `encoding/json` import.
- Confirm no production file changed (`git diff --stat -- internal/github/issuestore.go` is empty).

### Task 2: `prune-logging`
- Apply M1 and M2 and the deletions in `file_handler_test.go` and `ring_buffer_test.go`; remove the unused `time` import.

### Task 3: `prune-session`
- Apply M3, delete `TestModeSwitchLogging` and `internal/session/types_test.go`.

### Task 4: `prune-watcher`
- Apply M4, M5, M6.

### Task 5: `validate-all`
- Read-only. Run the six AC verify groups and `task test` (which is `go test -v -race ./...` with a coverage profile).

## Validation Commands

```bash
# AC1 (each prints nothing)
grep -nE 'func (TestGenerateFilename)\(' internal/incidents/*_test.go
grep -nE 'func (TestFileHandler_FilenameFormat|TestFileHandler_Write|TestMetadataHandling|TestNewFileHandler)\(' internal/logging/*_test.go
grep -nE 'func (TestModeSwitchLogging|TestSessionTypes)\(' internal/session/*_test.go
grep -nE 'func (TestEmptyPrerelease|TestPrereleaseConcatenation)\(' internal/version/*_test.go
# AC2 (each prints nothing), then partners pass with the merged cases as subtests
grep -nE 'func (TestFileHandler_RotationSequenceNumbers)\(' internal/logging/*_test.go
grep -nE 'func (TestSessionStateJSONMarshaling)\(' internal/session/*_test.go
grep -nE 'func (TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount|TestCheckIssues_SuccessfulSpawnStaysTracked|TestValidateIssueCyclicDependencyDoesNotFetchChain)\(' internal/watcher/*_test.go
go test -run '^TestFileHandler_Rotation$' -v ./internal/logging
go test -run '^TestSessionManagerCRUD$' -v ./internal/session
go test -run '^TestValidateIssue$' -v ./internal/watcher
go test -run '^TestCheckIssues_AdmissionUnavailableUntracksAndRefunds$' -v ./internal/watcher
go test -run '^TestCheckIssues_OtherSpawnErrorStaysTracked$' -v ./internal/watcher
# AC3
grep -nE 'func (TestNonStringKeys|TestOddKeyVals)\(' internal/logging/*_test.go   # prints nothing
go test -run '^TestMalformedKeyVals$' -v ./internal/logging                         # two subtests
# AC4
grep -nE 'func (TestIssueStoreImplementsProvenanceStore)\(' internal/github/*_test.go   # prints nothing
grep -rn 'var _ provenance.IssueStore' internal/github/*.go                              # one line: issuestore.go:24
# AC5
go test -cover ./internal/github ./internal/incidents ./internal/logging ./internal/session ./internal/version ./internal/watcher
# AC6
task test
```

Floors for AC5: github 71.3%, incidents 60.3%, logging 53.5%, session 28.3%, version 30.8%, watcher 69.2%.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "prune-github-incidents-version"
    agent: "builder"
    description: "Test-only pruning in internal/github, internal/incidents and internal/version. Delete TestIssueStoreImplementsProvenanceStore (github/issuestore_test.go) and remove its now-unused provenance import without adding any new var _ line, because issuestore.go:24 already holds the single compile-time assertion AC4 expects. Delete TestGenerateFilename (incidents/logger_test.go). Delete TestPrereleaseConcatenation and TestEmptyPrerelease (version/version_test.go) and remove the now-unused encoding/json import. No production code changes. Commit only these three files by explicit path; do not touch .kiro/agents/*.json."
    dependencies: []
    acceptance_criteria:
      - "TestIssueStoreImplementsProvenanceStore, TestGenerateFilename, TestPrereleaseConcatenation and TestEmptyPrerelease no longer exist"
      - "issuestore_test.go no longer imports internal/provenance and the github package tests compile"
      - "grep -rn 'var _ provenance.IssueStore' internal/github/*.go prints exactly one line (issuestore.go:24)"
      - "version_test.go no longer imports encoding/json and still contains TestVersionParsedFromJSON"
      - "go test -cover reports at least 71.3% for internal/github, 60.3% for internal/incidents and 30.8% for internal/version"
      - "No file other than the three named test files is modified by this task"
    validation_commands:
      - "! grep -nE 'func (TestGenerateFilename)[(]' internal/incidents/*_test.go"
      - "! grep -nE 'func (TestEmptyPrerelease|TestPrereleaseConcatenation)[(]' internal/version/*_test.go"
      - "! grep -nE 'func (TestIssueStoreImplementsProvenanceStore)[(]' internal/github/*_test.go"
      - "[ $(grep -rn 'var _ provenance.IssueStore' internal/github/*.go | wc -l) -eq 1 ]"
      - "git diff --quiet -- internal/github/issuestore.go"
      - "go vet ./internal/github ./internal/incidents ./internal/version"
      - "go test -cover ./internal/github | awk '/^ok/ {found=1; ok=($5+0>=71.3)} END {exit !(found && ok)}'"
      - "go test -cover ./internal/incidents | awk '/^ok/ {found=1; ok=($5+0>=60.3)} END {exit !(found && ok)}'"
      - "go test -cover ./internal/version | awk '/^ok/ {found=1; ok=($5+0>=30.8)} END {exit !(found && ok)}'"

  - id: "prune-logging"
    agent: "builder"
    description: "Test-only pruning in internal/logging per spec sections M1 and M2. In file_handler_test.go delete TestNewFileHandler, TestFileHandler_Write and TestFileHandler_FilenameFormat, remove the now-unused time import, and fold TestFileHandler_RotationSequenceNumbers into TestFileHandler_Rotation as two t.Run subtests ('size threshold rotates to a new file' keeps the existing body unchanged; 'same-minute handlers get distinct files' is the old sequence-number body with the file count asserted as exactly 3 instead of at least 2, keeping the debug- prefix check), each subtest using its own temp directory. In ring_buffer_test.go delete TestMetadataHandling and replace TestOddKeyVals and TestNonStringKeys with one table test named TestMalformedKeyVals with the two subtests 'odd number of keyvals drops the trailing key' and 'non-string key is skipped', keeping the original assertions (1 entry, metadata length, wanted key value). Commit only these two files by explicit path."
    dependencies: ["prune-github-incidents-version"]
    acceptance_criteria:
      - "TestNewFileHandler, TestFileHandler_Write, TestFileHandler_FilenameFormat, TestMetadataHandling, TestFileHandler_RotationSequenceNumbers, TestOddKeyVals and TestNonStringKeys no longer exist as functions"
      - "TestFileHandler_Rotation has exactly two passing subtests and the same-minute subtest asserts exactly 3 files"
      - "TestMalformedKeyVals exists, is table-driven, and go test -run '^TestMalformedKeyVals$' -v lists exactly two passing subtests, one per former test"
      - "file_handler_test.go no longer imports time and the package compiles"
      - "go test -cover ./internal/logging reports at least 53.5%"
    validation_commands:
      - "! grep -nE 'func (TestFileHandler_FilenameFormat|TestFileHandler_Write|TestMetadataHandling|TestNewFileHandler)[(]' internal/logging/*_test.go"
      - "! grep -nE 'func (TestFileHandler_RotationSequenceNumbers)[(]' internal/logging/*_test.go"
      - "! grep -nE 'func (TestNonStringKeys|TestOddKeyVals)[(]' internal/logging/*_test.go"
      - "[ $(go test -run '^TestFileHandler_Rotation$' -v ./internal/logging | grep -c -- '--- PASS: TestFileHandler_Rotation/') -eq 2 ]"
      - "[ $(go test -run '^TestMalformedKeyVals$' -v ./internal/logging | grep -c -- '--- PASS: TestMalformedKeyVals/') -eq 2 ]"
      - "grep -n 'len(files) != 3' internal/logging/file_handler_test.go"
      - "go vet ./internal/logging"
      - "go test -cover ./internal/logging | awk '/^ok/ {found=1; ok=($5+0>=53.5)} END {exit !(found && ok)}'"

  - id: "prune-session"
    agent: "builder"
    description: "Test-only pruning in internal/session per spec section M3. In manager_test.go add to TestSessionManagerCRUD, on the reloaded state2 after the existing history length check, a check that state2.Type equals Console and that state2.History[0] has Role user and Content 'test message' (guard the index against an empty history; do not change any existing assertion). Delete internal/session/types_test.go entirely (it only holds TestSessionStateJSONMarshaling and TestSessionTypes). Delete TestModeSwitchLogging from validation_focused_test.go (the last function in the file; its imports remain in use by earlier tests). Do not delete or change LogModeSwitch or ValidateSessionFlow (production code, and still called by TestSessionDataIntegrityDuringModeSwitch). Commit only these files by explicit path."
    dependencies: ["prune-logging"]
    acceptance_criteria:
      - "TestSessionStateJSONMarshaling, TestSessionTypes and TestModeSwitchLogging no longer exist and types_test.go is removed"
      - "TestSessionManagerCRUD passes and now asserts the reloaded state's Type and the first message's Role and Content, with all of its earlier assertions intact"
      - "validation_focused_test.go compiles with no unused imports"
      - "internal/session production files are unmodified"
      - "go test -cover ./internal/session reports at least 28.3%"
    validation_commands:
      - "! grep -nE 'func (TestModeSwitchLogging|TestSessionTypes)[(]' internal/session/*_test.go"
      - "! grep -nE 'func (TestSessionStateJSONMarshaling)[(]' internal/session/*_test.go"
      - "test ! -e internal/session/types_test.go"
      - "git diff --quiet -- internal/session/manager.go internal/session/types.go internal/session/planner.go"
      - "go test -run '^TestSessionManagerCRUD$' -v ./internal/session"
      - "go vet ./internal/session"
      - "go test -cover ./internal/session | awk '/^ok/ {found=1; ok=($5+0>=28.3)} END {exit !(found && ok)}'"

  - id: "prune-watcher"
    agent: "builder"
    description: "Test-only pruning in internal/watcher per spec sections M4, M5 and M6. dependencies_test.go: add an 'issue int' field to the TestValidateIssue row struct (loop uses 1 when it is 0), add the rows 'two issue cycle only looks up the direct dependency' (issue 10, body 'Dependencies: #11', details {11: closed with Body 'Dependencies: #10'}, lookups [11], valid) and 'self referencing issue is looked up once' (issue 10, body 'Dependencies: #10', details {10: open with Body 'Dependencies: #10'}, lookups [10], invalid, unresolved [10]), then delete TestValidateIssueCyclicDependencyDoesNotFetchChain. watcher_boundary_test.go: make TestCheckIssues_AdmissionUnavailableUntracksAndRefunds a table over {ErrNotAdmitted ('skipped (done)'), ErrAdmissionUnavailable ('gh down')} asserting spawned == [7], untracked, retry count 0 and retry file removed for both rows; make TestCheckIssues_OtherSpawnErrorStaysTracked a table over {errors.New('worktree create failed'), nil} asserting tracked == [7] and retry count 1 for both rows; call newSpawnWatcher inside each subtest; delete TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount and TestCheckIssues_SuccessfulSpawnStaysTracked. Leave TestCheckIssues_NotAdmittedRefundKeepsPriorAttempts and all other tests unchanged. Commit only these two files by explicit path."
    dependencies: ["prune-session"]
    acceptance_criteria:
      - "TestValidateIssueCyclicDependencyDoesNotFetchChain, TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount and TestCheckIssues_SuccessfulSpawnStaysTracked no longer exist as functions"
      - "TestValidateIssue includes both cycle rows as subtests and all pre-existing rows still pass"
      - "TestCheckIssues_AdmissionUnavailableUntracksAndRefunds has subtests ErrNotAdmitted and ErrAdmissionUnavailable"
      - "TestCheckIssues_OtherSpawnErrorStaysTracked has subtests for the spawn error and the successful spawn"
      - "go test -cover ./internal/watcher reports at least 69.2%"
    validation_commands:
      - "! grep -nE 'func (TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount|TestCheckIssues_SuccessfulSpawnStaysTracked|TestValidateIssueCyclicDependencyDoesNotFetchChain)[(]' internal/watcher/*_test.go"
      - "go test -run '^TestValidateIssue$' -v ./internal/watcher | grep -- '--- PASS: TestValidateIssue/two_issue_cycle'"
      - "go test -run '^TestValidateIssue$' -v ./internal/watcher | grep -- '--- PASS: TestValidateIssue/self_referencing'"
      - "[ $(go test -run '^TestCheckIssues_AdmissionUnavailableUntracksAndRefunds$' -v ./internal/watcher | grep -c -- '--- PASS: TestCheckIssues_AdmissionUnavailableUntracksAndRefunds/') -eq 2 ]"
      - "[ $(go test -run '^TestCheckIssues_OtherSpawnErrorStaysTracked$' -v ./internal/watcher | grep -c -- '--- PASS: TestCheckIssues_OtherSpawnErrorStaysTracked/') -eq 2 ]"
      - "go vet ./internal/watcher"
      - "go test -cover ./internal/watcher | awk '/^ok/ {found=1; ok=($5+0>=69.2)} END {exit !(found && ok)}'"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of all six acceptance criteria of issue #438. Check each AC Verify command from the spec's Validation Commands section: the nine Delete tests, the five Merge tests and the Combine group are gone as functions; each partner and TestMalformedKeyVals pass with the folded-in cases as subtests; exactly one 'var _ provenance.IssueStore' line exists in internal/github/*.go (issuestore.go); package coverage meets all six floors; task test exits 0. Also verify that no production (non-_test.go) file changed, that .kiro/agents/*.json were not modified by the builders beyond the pre-existing uncommitted changes, that the merged partners kept every assertion described in spec sections M1 to M6 (including exactly 3 files in the Rotation same-minute subtest and spawned == [7] on both admission rows), that the PR text can name TestMalformedKeyVals, and that no spec deviation is left undocumented."
    dependencies: ["prune-watcher"]
    acceptance_criteria:
      - "AC1 to AC4 grep and go test -run checks give the stated results"
      - "go test -cover meets 71.3 (github), 60.3 (incidents), 53.5 (logging), 28.3 (session), 30.8 (version) and 69.2 (watcher)"
      - "task test exits 0"
      - "git diff --name-only against the base branch lists only _test.go files under the six in-scope packages (plus the deleted types_test.go and this spec and sentinel artifacts), and no production .go file"
      - "Any departure from this spec is recorded in a code comment and in the builder sentinel; otherwise Spec deviations: none"
    validation_commands:
      - "! grep -nE 'func (TestGenerateFilename)[(]' internal/incidents/*_test.go"
      - "! grep -nE 'func (TestFileHandler_FilenameFormat|TestFileHandler_Write|TestMetadataHandling|TestNewFileHandler|TestFileHandler_RotationSequenceNumbers|TestNonStringKeys|TestOddKeyVals)[(]' internal/logging/*_test.go"
      - "! grep -nE 'func (TestModeSwitchLogging|TestSessionTypes|TestSessionStateJSONMarshaling)[(]' internal/session/*_test.go"
      - "! grep -nE 'func (TestEmptyPrerelease|TestPrereleaseConcatenation)[(]' internal/version/*_test.go"
      - "! grep -nE 'func (TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount|TestCheckIssues_SuccessfulSpawnStaysTracked|TestValidateIssueCyclicDependencyDoesNotFetchChain)[(]' internal/watcher/*_test.go"
      - "! grep -nE 'func (TestIssueStoreImplementsProvenanceStore)[(]' internal/github/*_test.go"
      - "[ $(grep -rn 'var _ provenance.IssueStore' internal/github/*.go | wc -l) -eq 1 ]"
      - "go test -run '^TestMalformedKeyVals$' -v ./internal/logging"
      - "go test -cover ./internal/github | awk '/^ok/ {found=1; ok=($5+0>=71.3)} END {exit !(found && ok)}'"
      - "go test -cover ./internal/incidents | awk '/^ok/ {found=1; ok=($5+0>=60.3)} END {exit !(found && ok)}'"
      - "go test -cover ./internal/logging | awk '/^ok/ {found=1; ok=($5+0>=53.5)} END {exit !(found && ok)}'"
      - "go test -cover ./internal/session | awk '/^ok/ {found=1; ok=($5+0>=28.3)} END {exit !(found && ok)}'"
      - "go test -cover ./internal/version | awk '/^ok/ {found=1; ok=($5+0>=30.8)} END {exit !(found && ok)}'"
      - "go test -cover ./internal/watcher | awk '/^ok/ {found=1; ok=($5+0>=69.2)} END {exit !(found && ok)}'"
      - "task test"
```
