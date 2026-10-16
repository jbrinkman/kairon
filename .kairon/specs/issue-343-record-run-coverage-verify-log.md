# Design Spec: Evals: record run coverage and verify it in verify-log

Closes #343

## 1. Problem Summary

`kairon eval verify-log` is meant to show cause and effect between the two runs of an iteration. Today a run does not record enough for that.

1. **No suite identity.** Nothing in a run identifies the eval suite (rubric, case files, workspace fixtures, hidden files, mock scripts). A `prompt` iteration cannot prove the suite stayed the same. An `eval` iteration cannot prove the suite changed while the prompt did not.
2. **No configuration comparison.** `loadRunRecord` (`internal/eval/verifylog.go`) reads only the score and `prompt_sha256`. It never compares agent model, judge model or sandbox mode between the two runs.
3. **Partial runs are accepted.**
   - An interrupted run keeps a verdict in `summary.json`. Only a `.progress` file marks it, and the directory can be committed without it being noticed.
   - A `--case` run (`runSingleTestCase`, `internal/eval/runner.go`) writes a full-looking summary with a verdict.

Dependency #337 (merged, commit `b274d0d`) already made `prompt_sha256` a hash of the staged workspace. The new rules compare that value, so no change to how it is computed is needed.

Out of scope (per the issue): iteration phases (Red/Green/Refactor) and how scores are computed.

## 2. Solution Approach

Record three new per-agent facts in every run, then make `verify-log` compare them. The recording side and the verification side meet at one schema (section 2.1). Later issues (iteration phases, trend) rely on that schema, so the field names are fixed.

### 2.1 Schema: three new fields per agent

Written in `<agent>.json` (`AgentResult`) and in `summary.json` under `agents.<agent>` (`AgentProvenance`):

| Field | Type | Meaning |
|-------|------|---------|
| `suite_sha256` | string, 64 lowercase hex | Identity of the agent's eval suite (section 2.2). |
| `complete` | bool | `true` only when every case of the agent has a result for every repeat **and** `case_filter` is empty. |
| `case_filter` | string | The `--case` value; `""` for a full run. |

Representation (so a legacy file is distinguishable from a recorded `false` / `""`):

- `SuiteSHA256 string \`json:"suite_sha256,omitempty"\``
- `Complete *bool \`json:"complete,omitempty"\`` — a non-nil pointer to `false` is still written.
- `CaseFilter *string \`json:"case_filter,omitempty"\`` — a non-nil pointer to `""` is still written.

The three are present together or absent together. A run that predates this change has none of them, which `verify-log` reads as "not recorded". Existing consumers (`eval diff`, resume) decode legacy files unchanged. `eval diff` is not changed by this issue.

Decision: a `--case` run is never `complete`, even if the agent has exactly one case. "Full run" means "no filter and every case covered".

Decision: the top-level single-agent mirror fields in `summary.json` (`prompt_sha256`, `agent_model`, ...) are **not** extended. The new fields live only in `agents.<agent>` and `<agent>.json`.

### 2.2 `suite_sha256`

New file `internal/eval/suite.go`:

```go
// suiteSHA256 returns the lowercase hex SHA-256 of agent's eval suite.
func suiteSHA256(agent string) (string, error)
```

It reads the active evals directory (`evalsPath`), so it honours `--evals-dir`.

Inputs, in this order of discovery:

1. **Rubric.** Every `rubrics/*.yaml` whose parsed `agent` equals `agent` (the same selection `loadRubrics` makes), as raw bytes.
2. **Case files.** Every `cases/<agent>/*.yaml` (non-recursive, `.yaml` only, as `loadCases` does), as raw bytes. This covers the whole agent suite whatever `--case` selects, so a filtered run and a full run of the same suite hash alike.
3. **References made by those cases**, found through `loadCases(agent)`:
   - `workspace: <name>` → every regular file under `fixtures/workspaces/<name>/` (symlinks skipped, like `copyTree`). The execute bit is part of the identity because `copyTree` preserves it.
   - `checks[].inject` entries → `fixtures/hidden/<entry>`.
   - `mocks[].script` → `<evals-dir>/<script>`.
   - `setup[]` entries of `type: file` → `rebaseEvalsPath(path)`. The default form `.kairon/evals/fixtures/selftest-input.md` is rebased onto `--evals-dir`.

Hashing rules:

- Each input is one length-framed part, using the existing `writeHashPart(h, kind, data)` (`provenance.go`). A leading domain part (`suite` / `v1`) separates this hash from `prompt_sha256`.
- The part's kind carries the logical name: `rubric <rel>`, `case <rel>`, `file <rel>`, `file+x <rel>`, `missing <rel>`. `<rel>` is slash-separated and relative to the evals directory, so the hash does not depend on where the evals directory is. A `setup` file outside the evals directory uses its cleaned path as written.
- Inputs are de-duplicated and sorted by (kind, name), so discovery order and a fixture shared by several cases do not matter.
- A referenced file that does not exist is hashed as `missing <rel>` (no error): the case itself reports the problem at run time, and the suite identity must still be stable. Any other read error is returned.
- File contents are hashed as raw bytes (no line-ending normalisation), like `prompt_sha256`.

Deliberately **not** part of the suite: the agent config, prompt and `resources` (that is `prompt_sha256`), models, files of other agents, workspace fixtures no case references.

### 2.3 Recording coverage in a run

`runConfig` (`config.go`) gains two run-scoped fields, reset by `configure` like `pins`:

- `suites map[string]string` — agent → `suite_sha256`; `nil` means "coverage not tracked" (direct calls that bypass `RunWithOptions`, as with `pins`).
- `caseFilter string` — the `--case` value.

`RunWithOptions` (`runner.go`):

- After `pinRun` succeeds, and when the run executes cases (not `List`, not `Perf`), call a new `pinSuites(agent, options)`. For each agent in `agentsInScope`, compute `suiteSHA256`. An agent whose rubric or cases cannot be loaded is skipped here, so the existing later error or warning is unchanged. A hashing error is returned before any case starts.
- Set `cfg.caseFilter = testcase` next to `cfg.candidate = candidate`.

A new helper (`stampCoverage(r *AgentResult, cases []TestCase)`) sets `SuiteSHA256`, `Complete` and `CaseFilter` on a result. It is a no-op when `cfg.suites` has no entry for the agent.

- `complete` is `cfg.caseFilter == "" && len(cases) > 0 && casesCovered(r.Cases, cases, runRepeat())`.
- `casesCovered` is true when every case name in `cases` has results for every run number `1..repeat` (a result with `Run == 0` counts as run 1, like legacy files).
- `evaluateProgressive` calls `stampCoverage` after loading a resumed result and after every `replaceCaseResults`, so each progressive save is accurate: an interrupted run is saved with `complete: false`, and the final save of a finished run has `complete: true`.
- `evaluate` (the `--case` path) calls it once after its loop. Its `cases` is the single target, and `cfg.caseFilter` is set, so `complete` is `false` and `case_filter` is the case name.

`updateIncrementalSummary` copies the three fields into `AgentProvenance`. The `agents.<agent>` entry is now written when `PromptSHA256 != "" || SuiteSHA256 != ""`.

Resume integrity (`checkResumeIntegrity`): when `cfg.suites` has the agent and the saved `<agent>.json` holds cases, refuse the resume if `suite_sha256` differs, or is absent (a file written before this change). The message follows the existing wording: `❌ cannot resume: eval suite changed since the interrupted run of <agent> (recorded suite_sha256=…; now …)`. Without this, a resume would stamp the new suite hash onto cases that ran under the old suite. When `cfg.suites` has no entry for the agent nothing is checked, so unpinned and direct test calls are unaffected.

### 2.4 Verifying it in `verify-log`

`runRecord` gains: `agentModel`, `judgeModel`, `sandbox` (`"native"`, `"container"` or `""`), `suiteSHA`, `complete`/`hasComplete`, `caseFilter`. `loadRunRecord` fills them using the precedence it already uses for `prompt_sha256`, field by field: `<agent>.json` first, then `summary.json`.

| Fact | `<agent>.json` | `summary.json` |
|------|----------------|----------------|
| `agent_model` | `agent_model` | `agents.<agent>.agent_model` |
| `judge_model` | `judge_model` | `judge_model` |
| `sandbox` | `sandbox` (bool → `container` / `native`) | `sandbox` (string) |
| `suite_sha256`, `complete`, `case_filter` | same names | `agents.<agent>.<name>` |

A fact recorded by neither file is "not recorded". For comparison it is a distinct value: not-recorded vs recorded differs, not-recorded vs not-recorded is equal.

`checkRuns` (both runs of one iteration, `baseline_run` and `result_run`):

| # | Condition | Rule | Message names |
|---|-----------|------|---------------|
| 1 | A run records no `suite_sha256` or no `complete` (one violation per run, listing the missing fields). For either change type a run with no `prompt_sha256` is also `run-data`. | `run-data` | run, agent, missing field(s) |
| 2 | A run records `complete: false` | `run-complete` (new) | run, agent, `complete=false`, `case_filter` |
| 3 | The two runs differ in `agent_model` or `sandbox`; for `change_type: prompt` also `judge_model` | `run-config` (new) | the field and both values |
| 4 | `prompt` iteration: `suite_sha256` differs | `suite-change` (new) | both hashes |
| 5 | `eval` iteration: `prompt_sha256` differs | `prompt-change` (existing rule, extended) | both hashes |
| 6 | `eval` iteration: same `suite_sha256` **and** same `judge_model` | `suite-change` | the shared hash and judge |
| 7 | `prompt` iteration: same `prompt_sha256` (existing) | `prompt-change` | unchanged |

Rule semantics: `prompt-change` now means "the prompt hash relation matches the change type" (differ for `prompt`, equal for `eval`). `suite-change` means "the suite relation matches the change type" (equal for `prompt`; changed suite or judge for `eval`).

Collect-all behaviour is kept. Each pairwise check runs whenever both runs record the facts it needs; missing facts are reported once by rule 1 rather than cascading. Rules 2 and 3 apply to both change types. `ruleOrder` gets: `run-complete` after `run-data`, `run-config` after `run-complete`, then `score-match`, `prompt-change`, `suite-change`, `min-iterations`.

Deliberate non-goal: `.progress` is not consulted. `complete` is per agent and is the single signal, so an agent that finished in a multi-agent run whose other agent was interrupted is still usable.

Behavioural change to document: any run recorded before this change now fails `run-data` for the missing fields. That is the intended fail-closed behaviour; the iteration must reference re-recorded runs. An `eval` iteration now needs `prompt_sha256` on both runs (it must prove the prompt is unchanged).

### 2.5 Fixtures under `internal/eval/testdata/verifylog/`

The shared `evals/results/` runs get the new fields. Every committed set shares them.

Existing runs (`aaaaaaa`, `bbbbbbb`, `ccccccc`, `ddddddd`, `eeeeeee`): add to `summary.json` top-level `judge_model` and `sandbox: "native"`, and to `agents.selftest`: `agent_model`, `suite_sha256`, `complete: true`, `case_filter: ""`. Suite `S1` (`1`×64) for `a`, `b`, `c`, `e`; suite `S2` (`2`×64) for `d`, so valid iteration 02 (`eval`, `c`→`d`) shows a changed suite with the same prompt hash. `fffffff` stays a legacy run with none of the new fields (kept so the legacy case has a committed example).

New runs (each differs from `bbbbbbb` in exactly one respect; other facts equal to `b`):

| Run | Differs in |
|-----|------------|
| `260705-100000-1111111` | `complete: false`, `case_filter: stub-basic` |
| `260705-110000-2222222` | `agent_model` |
| `260705-120000-3333333` | `sandbox: container` |
| `260705-130000-4444444` | `judge_model` |
| `260705-140000-5555555` | prompt hash and `suite_sha256` (prompt iteration whose suite also changed) |
| `260705-150000-6666666` | prompt hash and `suite_sha256` (for the eval iteration) |
| `260705-160000-7777777` | nothing: same prompt, suite, judge, models |
| `260705-170000-8888888` | no `suite_sha256` / `complete` (legacy shape) |

New log sets, each `selftest/iteration-00.md` (copy of valid's `a`→`b` prompt iteration) plus `iteration-01.md` (`b` → new run), so `--min-iterations 2` holds and the intended rule is the **only** violation:

| Set | `iteration-01` `change_type` → result run | Expected rule |
|-----|------------------------------------------|---------------|
| `partial-run` | prompt → `…1111111` | `run-complete` |
| `model-mismatch` | prompt → `…2222222` | `run-config` (names `agent_model`) |
| `sandbox-mismatch` | prompt → `…3333333` | `run-config` (names `sandbox`) |
| `judge-mismatch` | prompt → `…4444444` | `run-config` (names `judge_model`) |
| `suite-changed` | prompt → `…5555555` | `suite-change` |
| `eval-prompt-changed` | eval → `…6666666` | `prompt-change` |
| `eval-nothing-changed` | eval → `…7777777` | `suite-change` |
| `legacy-run` | prompt → `…8888888` | `run-data` (names `suite_sha256`, `complete`) |

Claimed scores in each log must equal the run's recorded score so `score-match` stays silent. Dates are non-decreasing (`2026-07-01`, `2026-07-02`).

## 3. Relevant Files

Create:
- `internal/eval/suite.go`, `internal/eval/suite_test.go`
- `internal/eval/testdata/verifylog/{partial-run,model-mismatch,sandbox-mismatch,judge-mismatch,suite-changed,eval-prompt-changed,eval-nothing-changed,legacy-run}/selftest/iteration-0{0,1}.md`
- `internal/eval/testdata/verifylog/evals/results/260705-*/` (the eight runs above: `summary.json` and `selftest.json`)
- A coverage test file (for example `internal/eval/coverage_test.go`) for recording and resume.

Modify:
- `internal/eval/types.go` — `AgentResult` and `AgentProvenance` fields.
- `internal/eval/config.go` — `suites`, `caseFilter`.
- `internal/eval/runner.go` — `RunWithOptions` (`pinSuites`, `caseFilter`), `evaluate`, `evaluateProgressive`, `updateIncrementalSummary`, `checkResumeIntegrity`, `stampCoverage`/`casesCovered`.
- `internal/eval/verifylog.go` — `runRecord`, `loadRunRecord`, `checkRuns`, rule constants and `ruleOrder`.
- `internal/eval/verifylog_test.go` — helpers (`vlVerdictSummary`, `vlAgentJSON`, `vlStdRuns`) must write complete runs; rewrite the tests that encoded the old behaviour (`TestVerifyLogLegacyFixtureRun`, the "eval iteration needs no prompt_sha256" case in `TestVerifyLogPromptHashRules`); extend `TestVerifyLogFixtureSets`.
- `cmd/kairon/cmd/eval_verify_log_test.go` — failing-set table; `cmd/kairon/cmd/eval_verify_log.go` — only if the help text lists rules (it currently does not).
- `internal/eval/testdata/verifylog/evals/results/{260701-100000-aaaaaaa,260702-100000-bbbbbbb,260702-120000-eeeeeee,260703-100000-ccccccc,260704-100000-ddddddd}/summary.json` (and `selftest.json` where it holds the hash).
- Existing tests that assert the exact shape of `summary.json`, `agents.<agent>` or resume behaviour: `selftest_test.go`, `pinning_test.go`, `provenance_test.go`, `runner_test.go`, `repeat_exec_test.go`, `parity_sandbox_test.go`, `candidate_e2e_test.go`. Run the whole package and fix the ones that fail for the intended reason.
- `docs/evaluation.md` — run summary schema, resume refusal, verify-log rules table, fixtures table, behavioural-change note.
- `.kairon/iterations/README.md` — `change_type` definitions, "Where runs and scores come from", rules table.

Do **not** touch:
- `.kairon/specs/` other than this file, `.kairon/evals/results/`, `CHANGELOG.md`, `.kairon/validation-results.md`, `VALIDATION_SUMMARY.md` (AGENTS.md historical artifacts).
- `.kiro/agents/*.json` (pre-existing uncommitted edits in this worktree) and `cmd/kairon/templates/`.
- Score computation (`scoring.go`, `verdict.go`) and prompt-hash computation (`provenance.go`).

Template sync: none of the changed paths are template-synchronized (`.kairon/iterations/` and `internal/eval/testdata/` are not in the sync map). Run `task sync:check` at the end to confirm.

## 4. Team Orchestration

All work lands in one PR.

```
suite-hash ──────────┐
                     ├─> wire-run ──────────────┐
coverage-model ──────┤                          ├─> verify-log-fixtures -> update-docs -> validate-issue-343
                     └─> verify-log-rules ──────┘
```

- `suite-hash` and `coverage-model` are independent (different files) and run in parallel. `coverage-model` takes `cfg.suites` as a plain map, so it does not need the hash function.
- `wire-run` (`runner.go`, `pinSuites`) and `verify-log-rules` (`verifylog.go`) touch different files and run in parallel once their prerequisites finish.
- `verify-log-fixtures` is last among the code tasks so the hand-written fixtures mirror what a real run now writes.
- `update-docs` follows the final behaviour. `validate-issue-343` is the read-only pass.

TDD (AGENTS.md) applies to every code task: write the failing test first; run it and confirm it fails for the expected reason, not a compile error; implement the minimum; re-run. Commit the test and implementation together, and name the test seen failing in the commit message. Where a task changes behaviour that existing tests encode, update those tests in the same commit so no commit is red. Use Conventional Commits and do not push. Do not edit the pre-existing uncommitted `.kiro/agents/*.json` changes.

## 5. Step-by-Step Task Breakdown

### Task 1: `suite-hash`
Implement `suiteSHA256(agent)` in `internal/eval/suite.go` as in section 2.2. Tests (`suite_test.go`, written first) use a temp evals directory built with `t.TempDir()` and `configure`/`resetConfig`:
- Deterministic across calls and across a copy of the evals directory at another path.
- Changes when the rubric, a case file, a referenced workspace fixture file, a referenced hidden file, a mock script or a `setup` file changes.
- Changes when a file's execute bit changes.
- Does **not** change when an unreferenced workspace fixture, another agent's case or rubric, or the agent config changes.
- A shared fixture is counted once; a missing referenced file hashes differently from an empty file and returns no error.
- The committed self-test suite hashes without error and is 64 lowercase hex characters.

### Task 2: `coverage-model`
Add the schema (section 2.1), `cfg.suites`/`cfg.caseFilter` (`configure` resets both), `casesCovered`, `stampCoverage`, the stamping calls in `evaluate` and `evaluateProgressive`, and the `updateIncrementalSummary` copy. Tests (seed `cfg.suites` directly):
- `casesCovered`: all cases and repeats present; one case missing; one repeat missing; `Run == 0` counts as 1; zero cases is not covered.
- `stampCoverage`: full run → `complete=true`, `case_filter=""`; filter set → `complete=false`, filter recorded; no `cfg.suites` entry → fields untouched.
- A progressive save after the first of several cases records `complete: false`; the final save records `true`.
- `summary.json` `agents.<agent>` carries the three fields, including `"complete": false` and `"case_filter": ""` explicitly. A result without them marshals without the keys.
- A legacy `AgentResult` / `Summary` JSON decodes with `Complete == nil`.

### Task 3: `wire-run`
Add `pinSuites`, call it and set `cfg.caseFilter` in `RunWithOptions`, and add the `checkResumeIntegrity` suite refusal (section 2.3). Tests with the stub backend and a small temp evals directory:
- A full run records `suite_sha256` and `complete: true` (AC1/AC2).
- Editing a case file between two runs changes `suite_sha256` and leaves `prompt_sha256` unchanged (AC1).
- A `--case` run records `case_filter` and `complete: false` (AC2), and does so for an agent whose only case is the one named.
- `--repeat 2` is complete only when both runs of every case exist.
- Resume: unchanged suite resumes; changed suite is refused with the message above; a saved result with cases but no `suite_sha256` is refused; an empty or legacy file with no cases is not refused; with no `cfg.suites` entry nothing is checked.
- A hashing error stops the run before any case runs.

### Task 4: `verify-log-rules`
Implement section 2.4 in `verifylog.go`: the new record fields, rules 1-7, rule constants, `ruleOrder`. Tests build runs in temp dirs through the `vl*` helpers (extended with the new facts):
- One test per rule 1-7, asserting rule, file suffix and that the message names the run, the field and, where applicable, both values.
- Both runs failing the same rule are each reported; missing facts are reported once and do not trigger the pairwise rules.
- `judge_model` mismatch fails a `prompt` iteration but not an `eval` iteration; `agent_model` and `sandbox` fail both.
- Not-recorded vs recorded is a mismatch; not-recorded vs not-recorded is equal.
- Field-wise precedence: `<agent>.json` over `summary.json`, summary as fallback; `sandbox` bool and string forms normalise to the same value.
- An `eval` iteration with equal prompt, same suite and same judge fails; changing only the judge or only the suite passes.
- Update `vlVerdictSummary`, `vlAgentJSON`, `vlStdRuns` so the standard runs are complete and consistent, and rewrite the tests that encoded the removed behaviour.

### Task 5: `verify-log-fixtures`
Create the committed fixtures of section 2.5 and extend `TestVerifyLogFixtureSets` and `cmd/kairon/cmd/eval_verify_log_test.go` `TestVerifyLogFailingSets` with the eight new sets (exactly one violation each, expected rule, file `iteration-01.md`, `--min-iterations 2`). `valid` must still print `OK selftest: 3 iterations verified`. Replace `TestVerifyLogLegacyFixtureRun` with a test that the committed legacy run `fffffff` now fails `run-data`. Add a test that every committed run under `verifylog/evals/results` other than the two legacy ones (`fffffff`, `…8888888`) records all three new fields, so a fixture cannot silently drift to legacy shape.

### Task 6: `update-docs`
Update `docs/evaluation.md` and `.kairon/iterations/README.md`:
- Run summary schema: `suite_sha256` (what it covers and does not, how it is built), `complete`, `case_filter`, and the decision that a `--case` run is never complete. Add them to the `summary.json` `agents` example and the `<agent>.json` description.
- Resume refusal: the new suite message and the legacy-file refusal.
- `verify-log` rules table in both files: add `run-complete`, `run-config`, `suite-change`; extend `run-data` and `prompt-change`. Update the `change_type` meaning (`prompt`: prompt hash differs, suite and judge unchanged; `eval`: prompt unchanged, and suite or judge changed) and "Where runs and scores come from".
- Fixtures table: the eight new sets and the rule each reports. Keep the `valid` command and its expected output.
- A migration note: runs recorded before this change fail `run-data`; re-record them.
Docs tests (`checks_docs_test.go`, `simulated_user_docs_test.go`) parse `docs/evaluation.md`; keep them passing.

### Task 7: `validate-issue-343`
Read-only verification of every acceptance criterion with the commands in the plan, plus `task test`, `task lint`, `task fmt:check`, `task sync:check`, and a check that no historical artifact was edited.

## 6. Acceptance Criteria Mapping

| AC | Covered by |
|----|-----------|
| 1 `suite_sha256` | Tasks 1, 2, 3 |
| 2 `complete`, `case_filter` | Tasks 2, 3 |
| 3 incomplete run fails | Tasks 4, 5 (`partial-run`) |
| 4 model/sandbox/judge mismatch | Tasks 4, 5 (`model-mismatch`, `sandbox-mismatch`, `judge-mismatch`) |
| 5 prompt iteration, suite changed | Tasks 4, 5 (`suite-changed`) |
| 6 eval iteration, prompt changed / nothing changed | Tasks 4, 5 (`eval-prompt-changed`, `eval-nothing-changed`) |
| 7 legacy run → `run-data` | Tasks 4, 5 (`legacy-run`) |
| 8 consistent set passes | Task 5 (`valid`) |

## 7. Validation Commands

```bash
go build ./...
go test ./internal/eval/ -run 'Suite|Coverage|VerifyLog' -count=1
go test ./cmd/kairon/cmd/ -run VerifyLog -count=1
task test
task lint
task fmt:check
task sync:check

# AC1 / AC2
task eval:selftest
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --case stub-basic

# AC3-AC7: each exits non-zero and names the rule
for s in partial-run model-mismatch sandbox-mismatch judge-mismatch suite-changed eval-prompt-changed eval-nothing-changed legacy-run; do
  ! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/$s --evals-dir internal/eval/testdata/verifylog/evals
done

# AC8
go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals
```

## 8. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "suite-hash"
    agent: "builder"
    description: "Implement suiteSHA256(agent) in internal/eval/suite.go: a deterministic, evals-dir-relative hash over the agent's rubric file(s), its case files and every workspace fixture file, hidden (inject) file, mock script and setup file the cases reference. Write the failing tests first (suite_test.go)."
    dependencies: []
    acceptance_criteria:
      - "suiteSHA256 returns 64 lowercase hex characters and is identical across calls and across a copy of the evals directory at a different path"
      - "The hash changes when the rubric, a case file, a referenced workspace fixture file, a referenced hidden file, a mock script, a setup file, or a fixture file's execute bit changes"
      - "The hash does not change when an unreferenced workspace fixture, another agent's case or rubric, or the agent config changes"
      - "A fixture shared by several cases is counted once, and a missing referenced file hashes differently from an empty file without returning an error"
      - "The committed self-test suite under internal/eval/testdata/evals hashes without error"
      - "The failing test was observed failing for the expected reason before the implementation was written"
    validation_commands:
      - "go build ./internal/eval/"
      - "go test ./internal/eval/ -run Suite -count=1"
      - "go vet ./internal/eval/"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "coverage-model"
    agent: "builder"
    description: "Add suite_sha256, complete and case_filter to AgentResult and AgentProvenance (SuiteSHA256 string omitempty; Complete *bool and CaseFilter *string omitempty so false and empty are still written), cfg.suites and cfg.caseFilter (reset by configure), casesCovered, stampCoverage, stamping in evaluate and evaluateProgressive after every replaceCaseResults, and copying into updateIncrementalSummary (entry written when PromptSHA256 or SuiteSHA256 is set). Write the failing tests first."
    dependencies: []
    acceptance_criteria:
      - "AgentResult and AgentProvenance serialise suite_sha256, complete and case_filter; complete:false and case_filter:\"\" are written explicitly when recorded and omitted when not recorded"
      - "Legacy AgentResult and Summary JSON without the fields decode with Complete == nil"
      - "casesCovered is true only when every case has results for every run 1..repeat (Run 0 counts as 1) and false for zero cases"
      - "A progressive save part-way through a run records complete:false and the final save of a finished run records complete:true"
      - "A run with a case filter records complete:false and the filter in case_filter, even when the agent has only that one case"
      - "stampCoverage is a no-op when cfg.suites has no entry for the agent, so unpinned and direct test calls keep their previous shape"
      - "Existing tests that encoded the old summary shape are updated in the same commit and the package is green"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Coverage|Summary|Repeat|Pinning' -count=1"
      - "go test ./internal/eval/ -count=1"
      - "go vet ./internal/eval/"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "wire-run"
    agent: "builder"
    description: "Wire coverage into RunWithOptions: add pinSuites (compute suiteSHA256 for each in-scope agent after pinRun when cases will run; skip agents whose rubric or cases fail to load; return hashing errors before any case starts), set cfg.caseFilter from the testcase argument, and extend checkResumeIntegrity to refuse a resume when the saved suite_sha256 differs or is absent from a result that holds cases. Write the failing end-to-end tests first using the stub backend and a temp evals directory."
    dependencies: ["suite-hash", "coverage-model"]
    acceptance_criteria:
      - "A full stub run records suite_sha256 and complete:true under agents.<agent> in summary.json and in <agent>.json"
      - "Editing one case file between two runs changes suite_sha256 and leaves prompt_sha256 unchanged"
      - "A --case run records case_filter equal to the case name and complete:false"
      - "With --repeat 2 a run is complete only when both runs of every case exist"
      - "Resume with an unchanged suite continues; with a changed suite or a saved result that has cases but no suite_sha256 it is refused with a message naming the agent and both hashes; an empty file is not refused; nothing is checked when cfg.suites has no entry for the agent"
      - "A suite hashing error stops the run before any case executes"
      - "task eval:selftest passes and the written summary has the three fields for selftest"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Coverage|Resume|Suite|SelfTest' -count=1"
      - "go test ./internal/eval/ -count=1"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --case stub-basic"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "verify-log-rules"
    agent: "builder"
    description: "Implement the new verify-log checks in internal/eval/verifylog.go: read agent_model, judge_model, sandbox, suite_sha256, complete and case_filter in loadRunRecord (field-wise precedence <agent>.json then summary.json), and in checkRuns add the run-data extension, the run-complete and run-config rules, the suite-change rule and the eval-iteration side of prompt-change, plus rule constants and ruleOrder. Update the vl* test helpers and rewrite tests that encoded the removed behaviour. Write the failing tests first."
    dependencies: ["coverage-model"]
    acceptance_criteria:
      - "A referenced run with no suite_sha256 or no complete fails with run-data naming the run, agent and missing field(s); a run with no prompt_sha256 is run-data for both change types"
      - "A run with complete:false fails with run-complete naming the file, the run and the rule"
      - "Different agent_model or sandbox between an iteration's runs fails run-config naming the field for both change types; different judge_model fails only a prompt iteration"
      - "A prompt iteration whose runs record different suite_sha256 fails suite-change"
      - "An eval iteration whose runs record different prompt_sha256 fails prompt-change, and one that records the same suite_sha256 and the same judge_model fails suite-change; changing only the judge or only the suite passes"
      - "Pairwise rules do not fire (and do not cascade) when a needed fact is missing; not-recorded versus recorded is a mismatch and not-recorded versus not-recorded is equal"
      - "Existing rules and messages for score-match, run-exists, chain, sequence and the prompt-iteration prompt-change are unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run VerifyLog -count=1"
      - "go vet ./internal/eval/"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "verify-log-fixtures"
    agent: "builder"
    description: "Create the committed verify-log fixtures: add the new fields to the shared runs under internal/eval/testdata/verifylog/evals/results, add the eight new runs (260705-*), and add the eight failing log sets (partial-run, model-mismatch, sandbox-mismatch, judge-mismatch, suite-changed, eval-prompt-changed, eval-nothing-changed, legacy-run), each with iteration-00 (a to b) and iteration-01 (b to the new run). Extend TestVerifyLogFixtureSets and the cmd TestVerifyLogFailingSets, replace TestVerifyLogLegacyFixtureRun, and add a guard test that non-legacy committed runs record all three fields."
    dependencies: ["verify-log-rules", "wire-run"]
    acceptance_criteria:
      - "The valid set still exits 0 and prints OK selftest: 3 iterations verified; valid iteration 02 is an eval iteration with the same prompt_sha256 and a different suite_sha256"
      - "Each new set yields exactly one violation with --min-iterations 2: partial-run run-complete; model-mismatch and sandbox-mismatch and judge-mismatch run-config naming agent_model, sandbox and judge_model; suite-changed suite-change; eval-prompt-changed prompt-change; eval-nothing-changed suite-change; legacy-run run-data naming suite_sha256 and complete"
      - "The existing missing-run, score-mismatch, gap, broken-chain and no-prompt-change sets still report their original single rule"
      - "The CLI command exits non-zero for each failing set and zero for valid, with and without the default --min-iterations"
      - "A guard test fails if a committed run other than the two legacy runs lacks suite_sha256, complete or case_filter"
    validation_commands:
      - "go test ./internal/eval/ -run VerifyLog -count=1"
      - "go test ./cmd/kairon/cmd/ -run VerifyLog -count=1"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/partial-run --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/model-mismatch --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/sandbox-mismatch --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/suite-changed --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/eval-prompt-changed --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/eval-nothing-changed --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/legacy-run --evals-dir internal/eval/testdata/verifylog/evals"

  - id: "update-docs"
    agent: "documenter"
    description: "Update docs/evaluation.md and .kairon/iterations/README.md: document suite_sha256 (inputs and exclusions), complete and case_filter (a --case run is never complete), add them to the summary.json and <agent>.json examples, document the new resume refusal, add run-complete, run-config and suite-change and extend run-data and prompt-change in both rules tables, update the change_type meaning and 'Where runs and scores come from', list the eight new fixture sets with their rules, and add a migration note that runs recorded before this change fail run-data. Do not edit historical artifacts."
    dependencies: ["wire-run", "verify-log-fixtures"]
    acceptance_criteria:
      - "Both rules tables list run-data, run-complete, run-config, prompt-change and suite-change with the conditions implemented in verifylog.go"
      - "docs/evaluation.md describes suite_sha256 inputs and exclusions exactly as implemented, plus complete, case_filter and the resume refusal message"
      - "The fixtures table in docs/evaluation.md lists all eight new sets and the rule each reports, and keeps the valid-set command with its expected output"
      - "change_type: prompt and change_type: eval are described with the suite, prompt and judge conditions the verifier enforces"
      - "A migration note states that runs recorded before this change fail run-data and must be re-recorded"
      - "No file under .kairon/specs/ (other than the issue 343 spec), .kairon/evals/results/ or CHANGELOG.md is modified"
    validation_commands:
      - "go test ./internal/eval/ -run 'Docs|Checks' -count=1"
      - "grep -n 'suite_sha256' docs/evaluation.md .kairon/iterations/README.md"
      - "grep -n 'run-complete' docs/evaluation.md .kairon/iterations/README.md"
      - "grep -n 'suite-change' docs/evaluation.md .kairon/iterations/README.md"
      - "git diff --quiet -- CHANGELOG.md .kairon/evals/results"

  - id: "validate-issue-343"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion of issue 343: run the full test, lint, format and template-sync checks, the self-test and single-case runs, and every verify-log fixture set; confirm the TDD evidence in commit messages and that no historical artifact or pre-existing .kiro/agents edit was changed."
    dependencies: ["update-docs"]
    acceptance_criteria:
      - "AC1: two task eval:selftest runs on a temp copy with one case file edited record different suite_sha256 and the same prompt_sha256 for selftest"
      - "AC2: task eval:selftest records complete:true and the --case stub-basic run records case_filter stub-basic and complete:false"
      - "AC3-AC7: partial-run, model-mismatch, sandbox-mismatch, suite-changed, eval-prompt-changed, eval-nothing-changed and legacy-run each exit non-zero naming the file and the expected rule (legacy-run names run-data)"
      - "AC8: the valid set exits 0"
      - "task test, task lint, task fmt:check and task sync:check all pass"
      - "Commit messages name the test observed failing first, and tests and implementation are committed together"
      - "Nothing under .kairon/evals/results, CHANGELOG.md, other .kairon/specs files or cmd/kairon/templates was modified"
    validation_commands:
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --case stub-basic"
      - "go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/valid --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/partial-run --evals-dir internal/eval/testdata/verifylog/evals"
      - "! go run ./cmd/kairon eval verify-log selftest --iterations-dir internal/eval/testdata/verifylog/legacy-run --evals-dir internal/eval/testdata/verifylog/evals"
      - "git diff --quiet origin/main -- cmd/kairon/templates .kairon/evals/results CHANGELOG.md"
```
