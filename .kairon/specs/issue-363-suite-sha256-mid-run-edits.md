# Design Spec: Keep recorded `suite_sha256` honest against mid-run suite edits

Closes #363

## 1. Problem

`kairon eval` computes each in-scope agent's `suite_sha256` once, in `pinSuites` (`internal/eval/runner.go`, called from `RunWithOptions` before any case starts), and keeps it in `cfg.suites`. `stampCoverage` then copies that pinned value onto every `AgentResult` it stamps, on every progressive save. Nothing re-reads the suite between the pin and a save. Cases, rubrics and fixtures are read from disk when each case runs (`loadCases`, `executeCase`, workspace seeding, `inject`/`mocks`/`setup`), so an edit made while the run is in progress is executed but recorded under the old hash.

Effect: the result's `suite_sha256` does not identify the suite that produced its score. `verify-log` compares `suite_sha256` across runs (`suite-change`, `run-config`), so two genuinely different suites can look identical. #343 closed this across runs (`checkResumeSuite`) but not inside one run.

## 2. Solution Approach

### 2.1 Options considered

| Option | Correctness | Cost | Verdict |
|--------|-------------|------|---------|
| A. Snapshot the suite (copy cases/rubrics/fixtures to a temp dir at pin time, run from the copy) | Strongest: executed bytes equal hashed bytes by construction | High. Every read path (`loadCases`, `loadRubrics`, `evalsPath` users, fixture seeding, `--evals-dir` rebasing in `rebaseEvalsPath`) must be redirected to the snapshot, and the copy costs time and disk per run. Large blast radius in a 2.6k-line runner | Rejected |
| B. Re-hash once, at the very end of an agent, before the final save | Detects drift, but every earlier progressive save has already written the stale pinned hash, and `complete: true` is already on disk from the last case's save | Lowest | Rejected. It leaves a persisted dishonest result |
| **C. Re-hash before every save that stamps the hash, refuse on mismatch, and do not write that save** | A result file is never written with a hash that disagrees with the current suite. Fails fast at the first case that finished after the edit | One `suiteSHA256` per saved case. It reads a few small YAML/fixture files, which is negligible next to a model call per case (tens of seconds) | **Chosen** |

Trade-off of C versus A: C detects and refuses; it does not make the run succeed in the presence of an edit. That matches acceptance criterion 1 ("either hashes-and-executes a stable snapshot, or re-hashes before saving and refuses") and avoids rewriting every suite read path. C is also cheaper than the issue's "once at save time" wording suggests: the issue's cost concern (snapshot copying) is avoided, and the per-case re-hash is what guarantees that no save is ever stamped stale.

### 2.2 What C guarantees and what it does not

Guaranteed:
- Every `<agent>.json` and `summary.json` write that carries `suite_sha256` happens only after the suite on disk re-hashed equal to the pinned value. The on-disk hash therefore matches the suite as it stood at that save.
- An edit made at any time after the pin and before a case's save is caught at that case's save (and the remaining cases never run). This includes an edit made during case N or between case N-1's save and case N's execution.
- The case whose save detected the drift is not saved, so its score is never attributed to the wrong hash. Earlier saved cases were each verified at their own save.
- After a refusal, `.progress` is left in place (same as the existing early-return error paths), so a later `--resume` runs `checkResumeSuite`: the saved hash (the pinned one) differs from the edited suite, and resume is refused. If the user reverts the edit, resume works as before.

Not guaranteed (documented limits, not goals):
- An edit made and reverted entirely between two checks (the content is identical at both hash points). The hash identifies the content at the check, and the content is again the pinned content, so the recorded hash is still correct for what is on disk; this cannot be detected by any hash-based scheme and only a snapshot (option A) would close it.
- Concurrency between separate `kairon eval` processes (out of scope per the issue).

### 2.3 Design

New helper in `internal/eval/runner.go`, next to `checkResumeSuite`:

```go
// checkSuiteUnchanged re-hashes agent's eval suite and refuses when it no
// longer matches the hash pinned at the start of the run. nil when the run
// pinned no suite for the agent (direct calls that bypass RunWithOptions).
func checkSuiteUnchanged(agent string) error
```

Behaviour:
- `pinned, ok := cfg.suites[agent]`; `!ok` returns nil (same no-op rule as `stampCoverage` and `checkResumeSuite`).
- `now, err := suiteSHA256(agent)`. A hashing error (for example a case file deleted or made malformed mid-run) returns `❌ cannot re-hash the eval suite of <agent> after a case ran: <err>`. The suite can no longer be identified, so the run is refused rather than stamped.
- `now != pinned` returns the error below. Wording follows `checkResumeSuite` (`❌` prefix, `suite_sha256=` labels, em-dash remedy):

```
❌ eval suite of <agent> changed while the run was in progress (suite_sha256 at run start=<pinned>; now <now>) — nothing was saved for the case that finished after the change; restore the suite and start a fresh run
```

Call sites (every place that stamps the hash and then persists it):

1. `evaluateProgressive` (`runner.go`, the loop around `stampCoverage` + `saveProgressiveResult`): call `checkSuiteUnchanged(rubric.Agent)` after `executeCaseRepeats` and **before** `replaceCaseResults`/`stampCoverage`/`saveProgressiveResult`. On error, stop the loop and return the error without saving. This requires `evaluateProgressive` to return `(AgentResult, error)`.
2. `runProgressiveEvaluation`: if `evaluateProgressive` returns an error, return it immediately. Do not run the final save, do not remove `.progress`, do not append to `failed`. The existing `defer releaseResultLock` releases the lock. `RunWithOptions` returns the error and the CLI exits non-zero by the same path used for `checkResumeSuite` errors.
3. `runSingleTestCase`: `evaluate` is reused by tests and keeps its signature. Call `checkSuiteUnchanged(agent)` right after `evaluate(...)` returns and before the result file and `updateIncrementalSummary` are written. On error return it, so nothing is written.

`stampCoverage` is unchanged. It stays pure (no I/O), so its existing tests and the `cfg.suites`-with-fake-hash tests of the stamp itself keep passing. The check is a precondition at the callers, not part of the stamp.

`pinSuites` is unchanged. `suiteSHA256` and `internal/eval/suite.go` are not touched. `verify-log` is not touched.

### 2.4 Existing tests that need a real pin

The `evaluateProgressive` tests in `coverage_test.go` (`TestCoverageEvaluateProgressiveStampsEverySave` and the neighbouring ones that set `cfg.suites = {"selftest": testSuiteHash}` with a fake hash and no suite on disk) would now correctly be refused, because the fake pin does not match any suite on disk. Update them to build a real evals directory for agent `selftest` (reusing the `suiteEvals`/`configure(RunOptions{EvalsDir: dir})` pattern from `suite_test.go`) and pin it with `pinSuites`. Their assertions about `Complete`, `CaseFilter` and the hash value then compare against the real pinned hash instead of the constant. Tests that leave `cfg.suites` nil are unaffected. Call sites of `evaluateProgressive` in other tests change mechanically to `res, err := ...` and fail the test on a non-nil `err`.

The `evaluate` signature does not change, so its test call sites do not change.

## 3. Relevant Files

Modify:
- `internal/eval/runner.go`: add `checkSuiteUnchanged`; change `evaluateProgressive` to return an error and call the check before each save; propagate in `runProgressiveEvaluation`; call the check in `runSingleTestCase` before writing.
- `internal/eval/coverage_test.go`: update fake-pin `evaluateProgressive` tests to a real pinned suite; add the new tests (below).
- `internal/eval/scoring_test.go`, `internal/eval/execute_case_test.go`, `internal/eval/criterion_floor_test.go`: mechanical `evaluateProgressive` return-value change.
- `docs/evaluation.md`: behavioural note under "Run coverage: `suite_sha256`, `complete` and `case_filter`" and a cross-reference from the resume "Eval suite" paragraph.

Read only (do not edit):
- `internal/eval/suite.go` (hash algorithm, out of scope), `internal/eval/verifylog.go` (rules, out of scope), `.kairon/specs/issue-343-*.md` (historical).

`internal/eval/testdata/evals/` needs no change: `task eval:selftest` is the no-regression check. The new tests build their own temp evals directories.

Template sync is not involved (`.kiro/agents`, `.kairon/scripts` untouched).

## 4. Tests (written first; see AGENTS.md TDD)

Place in `internal/eval/coverage_test.go` (or a new `internal/eval/suite_drift_test.go`). Reuse `suiteEvals`, `chdirTemp`, `installFakeKiroCLI`, `useStubBackend`, `covCase`, `verdictRubric` and `configure` helpers.

1. `TestCheckSuiteUnchangedNoDrift`: configure the temp evals dir, `pinSuites("alpha", opts)`, call `checkSuiteUnchanged("alpha")`, expect nil.
2. `TestCheckSuiteUnchangedRefusesEditedFixture`: pin, then rewrite `fixtures/workspaces/shared/README.md` (a referenced fixture); expect an error that starts with `❌`, contains `alpha`, the pinned hash and the new hash (`suiteHashAt`-style re-hash), and contains `suite_sha256`. Repeat as a table for an edited case file and an edited rubric.
3. `TestCheckSuiteUnchangedNotPinnedIsNoOp`: `cfg.suites` nil or without the agent returns nil with no hashing.
4. `TestCheckSuiteUnchangedHashErrorRefuses`: pin, delete or corrupt a case file so `suiteSHA256` errors; expect a `❌ cannot re-hash` error naming the agent.
5. `TestEvaluateProgressiveRefusesMidRunEdit` (the criterion 1 test, end to end through the save path): temp evals dir plus real stub `TestCase` list of two cases; case one's stub `Commands` rewrite a referenced suite file by absolute path. Assert that `evaluateProgressive` returns the `❌` error naming the agent and both hashes, that no `<agent>.json` was written containing the case that ran after the edit, and that case two never ran. Also assert that no saved file on disk has a `suite_sha256` that differs from `suiteSHA256(agent)` of the suite at the time.
6. `TestRunProgressiveEvaluationMidRunEditFails` (criterion 3): the same scenario through `runProgressiveEvaluation`; expect a non-nil error whose text matches the `❌ eval suite of <agent> changed while the run was in progress (suite_sha256 at run start=<h1>; now <h2>)` form, that `.progress` still exists, and that the final result file does not record a stale-stamped `complete: true`.
7. `TestRunSingleTestCaseMidRunEditFails`: same through `runSingleTestCase`; error returned and no result file written.
8. Existing no-drift behaviour (criterion 2): the updated `TestCoverageEvaluateProgressiveStampsEverySave` and `TestCoverageEvaluateStamps*` tests still assert the pinned hash is recorded on every save and the final result has `complete: true`.

TDD order for the builder (all in one commit):
- Add a stub `checkSuiteUnchanged` that returns nil and the new `evaluateProgressive` error return (no behaviour change; existing tests, updated as in 2.4, pass). This lets the new tests compile.
- Add tests 1 to 7. Run them. Confirm they fail on their assertions (the stub returns nil, so tests expect an error and get none), not on compile errors.
- Implement `checkSuiteUnchanged` and the three call sites. Confirm all tests pass.
- In the commit message, name the test seen failing first (for example `TestEvaluateProgressiveRefusesMidRunEdit`).

## 5. Team Orchestration

Single-PR change, three sequential tasks. There is nothing to parallelise before the code lands because the docs describe the final behaviour.

1. `implement-suite-recheck` (builder): tests, helper, signature change, call sites, test fixups. One commit (test + fix together).
2. `document-suite-recheck` (documenter), after 1: `docs/evaluation.md` only.
3. `validate-suite-recheck` (validator), after 1 and 2: read-only verification of every acceptance criterion.

## 6. Step-by-Step Task Breakdown

### Task 1: `implement-suite-recheck`
Acceptance criteria:
- `checkSuiteUnchanged(agent)` exists and behaves per 2.3.
- `evaluateProgressive` returns `(AgentResult, error)`, checks before each stamp+save, and returns without saving on drift.
- `runProgressiveEvaluation` returns the error immediately, leaves `.progress`, skips the final save.
- `runSingleTestCase` checks after `evaluate` and writes nothing on drift.
- The error text is `❌ eval suite of <agent> changed while the run was in progress (suite_sha256 at run start=<pinned>; now <now>) — ...` and names the agent and both hashes.
- `suite.go`, `verifylog.go` and `stampCoverage` are unchanged.
- Fake-pin tests updated to a real pinned suite; all prior tests pass; new tests 1 to 7 pass.
- `task eval:selftest` exits 0 and still records a `suite_sha256` in the generated `summary.json` and `<agent>.json`. The builder checks that wall time is not meaningfully different (one extra `suiteSHA256` per case).

### Task 2: `document-suite-recheck`
Acceptance criteria:
- `docs/evaluation.md` "Run coverage" section states that the suite is re-hashed before each save, that a mismatch fails the run with the quoted `❌` message (agent, hash at start, current hash), that the case that finished after the edit is not saved, that earlier saves remain valid, and that a later `--resume` is refused by the existing suite check until the suite is restored.
- The note states the limit: an edit that is reverted before the next check is not detectable.
- The existing resume "Eval suite" paragraph links to the new note. No change to the `suite_sha256` computation description or the `verify-log` rule tables.

### Task 3: `validate-suite-recheck`
Acceptance criteria: criteria 1 to 3 of the issue verified by running tests and reading code; `git -P diff --stat` shows no change to `internal/eval/suite.go`, `internal/eval/verifylog.go`, or `stampCoverage`; `.kairon/specs/` other than this file untouched; commit contains the test and the fix together.

## 7. Validation Commands

```bash
go build ./...
go vet ./internal/eval/...
go test ./internal/eval/ -run 'SuiteUnchanged|MidRunEdit|Coverage' -count=1
go test ./internal/eval/... -count=1
task eval:selftest
task test
task lint
git -P diff --stat origin/main -- internal/eval/suite.go internal/eval/verifylog.go   # expect no output
```

## 8. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-suite-recheck"
    agent: "builder"
    description: "TDD: add checkSuiteUnchanged (re-hash the agent's suite and refuse with a ❌ error naming the agent and both hashes when it differs from the pinned cfg.suites value); make evaluateProgressive return (AgentResult, error) and call the check before each stampCoverage+save; propagate the error in runProgressiveEvaluation (no final save, keep .progress); call the check after evaluate in runSingleTestCase before writing results. Update fake-pin evaluateProgressive tests to a real pinned suite and the mechanical call-site changes. Write the failing tests first (stub the helper so they compile and fail on assertions), then implement; commit tests and fix together. Do not touch internal/eval/suite.go, internal/eval/verifylog.go or stampCoverage."
    dependencies: []
    acceptance_criteria:
      - "checkSuiteUnchanged returns nil when the suite is unchanged or the agent was not pinned, and a ❌-prefixed error containing the agent name, the pinned hash and the current hash when a case file, rubric or referenced fixture was edited after pinSuites"
      - "A hashing error during the re-check (for example a deleted case file) is refused with a ❌ error naming the agent"
      - "evaluateProgressive checks before each stamp and save; on drift it returns the error and writes nothing for the case that finished after the edit, and later cases do not run"
      - "runProgressiveEvaluation returns the error (non-nil, so the CLI exits non-zero), skips the final save and leaves the .progress marker"
      - "runSingleTestCase returns the error and writes no result file on drift"
      - "The error text matches the checkResumeSuite style: starts with ❌ and uses suite_sha256= labels"
      - "A run with no mid-run change records the pinned suite_sha256 on every save exactly as before; existing coverage and stamp tests pass (fake-pin tests updated to a real pinned suite)"
      - "internal/eval/suite.go, internal/eval/verifylog.go and stampCoverage have no diff"
      - "New tests were observed failing on their assertions before the fix, and the commit contains tests and fix together"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'SuiteUnchanged|MidRunEdit|Coverage' -count=1"
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "test -z \"$(git -P diff origin/main --name-only -- internal/eval/suite.go internal/eval/verifylog.go)\""

  - id: "document-suite-recheck"
    agent: "documenter"
    description: "Update docs/evaluation.md: in the 'Run coverage: suite_sha256, complete and case_filter' section add a behavioural note that the suite is re-hashed before every save and a mid-run edit fails the run with the quoted ❌ message (agent, hash at start, current hash), that the case finished after the edit is not saved while earlier saves stay valid, that --resume is then refused by the existing suite check until the suite is restored, and the limit that an edit reverted before the next check cannot be detected. Link the note from the resume 'Eval suite' paragraph. Do not change the suite_sha256 computation description or the verify-log rule tables."
    dependencies: ["implement-suite-recheck"]
    acceptance_criteria:
      - "docs/evaluation.md documents the save-time re-hash, the exact error wording, and what is and is not saved"
      - "The documented limit (edit reverted before the next check) is stated"
      - "The resume 'Eval suite' paragraph cross-links to the new note"
      - "The suite_sha256 computation list and verify-log rule tables are unchanged"
    validation_commands:
      - "grep -n 'changed while the run was in progress' docs/evaluation.md"
      - "git -P diff --stat -- docs/evaluation.md"

  - id: "validate-suite-recheck"
    agent: "validator"
    description: "Read-only verification of issue #363 acceptance criteria: mid-run edit is refused with agent and both hashes (criterion 1 and 3), no-drift runs are unchanged and task eval:selftest records suite_sha256 and exits 0 (criterion 2), out-of-scope files (suite.go, verifylog.go, stampCoverage) are untouched, tests and fix are in the same commit, docs updated."
    dependencies: ["implement-suite-recheck", "document-suite-recheck"]
    acceptance_criteria:
      - "go test ./internal/eval/... passes and includes tests covering a mutated case file, rubric and referenced fixture, the error text, and the non-zero-error path through runProgressiveEvaluation"
      - "task eval:selftest exits 0 and the generated result files carry a 64-hex suite_sha256"
      - "task test and task lint pass"
      - "No diff in internal/eval/suite.go, internal/eval/verifylog.go or the body of stampCoverage; no edits under .kairon/specs/ except issue-363"
      - "docs/evaluation.md describes the new behaviour"
    validation_commands:
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "task test"
      - "task lint"
      - "test -z \"$(git -P diff origin/main --name-only -- internal/eval/suite.go internal/eval/verifylog.go)\""
      - "grep -n 'changed while the run was in progress' docs/evaluation.md"
```
