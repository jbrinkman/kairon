# Design Spec: Evals — pass/fail checks on workspace files, output and gh calls

Closes #314

## 1. Problem and Current State

Deterministic scoring lives in `scoreDeterministic` (`internal/eval/runner.go`). It picks a keyword heuristic from the
*criterion name* (`completeness` → output contains `## `, `test_execution` → output contains `PASS`, …) and looks only at
the agent's stdout. Case authors cannot assert on what the agent actually did.

What already exists and is reused:

- `executeCase` (`execute_case.go`) builds a per-case git workspace (`workspace.go`: `caseWorkspace`, fixture committed as
  the single commit, `.eval/` and `.kiro/` in `.git/info/exclude`), calls the agent, then calls `scoreCaseFn` **while the
  workspace still exists**. `CaseResult.WorkspaceDir` is set before scoring.
- The fake `gh` (sandbox) appends every call to `<workspace>/.eval/gh.log`, one line per call (`gh issue create --title t …`).
- `fixtures/hidden/` is already a reserved, `sync:check`-excluded fixture location (`Taskfile.yml`, `docs/evaluation.md`).
- `scoreCase` (`scoring.go`) appends one `CriterionScore` per non-cost rubric criterion; `caseTotals` sums
  `Score`/`MaxScore` and ignores `Skipped`.
- `loadCases` (`runner.go`) parses and validates case YAML (`validateCaseFields` in `workspace.go`).
- Self-test fixtures: `internal/eval/testdata/evals/` (agents `selftest`, `selftest-fail`, rubrics, cases, fixtures).
  They are **not** under `.kairon/evals/`, so no template sync is needed (`task sync:check` is unaffected).

## 2. Solution Approach

Add a declarative, deterministic **`checks`** list to cases and a small, reusable evaluator.

```
case.yaml checks[] ──loadCases/validate──► []Check
                                              │
executeCase ─► agent runs ─► scoreCase ─► EvaluateChecks(checks, CheckInput{Dir, Output, GHLog, Base})
                                              │                         ▲ only these inputs (constraint)
                                              ▼
                          per-criterion CriterionScore{Score=passed, MaxScore=total, Reasoning, Checks[]}
```

Key decisions:

1. **Pure evaluator, three inputs.** `EvaluateChecks(checks []Check, in CheckInput) []CheckResult` takes
   `CheckInput{Dir, Output, GHLog string; Base string}`. `Dir`/`Output`/`GHLog` are the three required inputs from the
   issue's constraint; `Base` is an *optional* git revision for `changed_files` (default `HEAD`). The evaluator never
   touches `cfg`, `caseWorkspace`, `TestCase` or the rubric, so Stage 4 can call it on any directory. Everything the
   check needs from the case loader (compiled regex, resolved `inject` sources) is stored on the `Check` itself in
   unexported fields at load time. A `Check` built by hand (no loader) is compiled/validated lazily by
   `EvaluateChecks`, which reports an invalid check as a failed result (never a panic, never a silent pass).
2. **Check score replaces the heuristic for that criterion in that case.** A criterion with ≥1 check in a case scores
   `Score = checks passed`, `MaxScore = checks total`, `Deterministic = true` (regardless of the rubric's `scoring`,
   `deterministic` flag, or whether it would otherwise be LLM-judged — so no judge call is spent on it). Criteria with no
   checks in the case go through the existing code path unchanged (AC5).
3. **Exported vocabulary.** `Check`, `CheckType`, `CheckInput`, `CheckResult`, `EvaluateChecks`, `ValidateChecks` and the
   ten `CheckType` constants are exported and live in `internal/eval`; P2/B2/Stage 4 rely on the YAML field names, which
   are fixed by the issue (`criterion`, `type`, `run`, `expect_exit`, `inject`, `path`, `pattern`, `allow`).
4. **No new dependencies.** Globbing is a ~40-line in-package matcher (no `doublestar`); regexes are Go RE2.

### 2.1 Schema (YAML)

```yaml
checks:
  - criterion: structural_completeness   # required, must name a non-cost rubric criterion of the case's agent
    type: command                         # required, one of the ten types below
    run: "go test ./..."                  # command
    expect_exit: 0                        # command, optional, default 0
    inject: [hidden_test.go]              # command, optional
```

| `type` | Required fields | Optional | Pass when |
|---|---|---|---|
| `command` | `run` | `expect_exit` (default 0), `inject` | `sh -c run` in `Dir` exits with `expect_exit` |
| `file_exists` | `path` | | `path` exists in `Dir` (any entry type, symlink counts) |
| `file_absent` | `path` | | `path` does not exist |
| `file_contains` | `path`, `pattern` | | regular file whose content matches regex |
| `file_not_contains` | `path`, `pattern` | | regular file exists **and** content does not match |
| `changed_files` | `allow` (list of globs; `[]` allowed = no change allowed) | | no added/modified/deleted path outside `allow` |
| `output_contains` / `output_not_contains` | `pattern` | | final agent output matches / does not match |
| `gh_log_contains` / `gh_log_not_contains` | `pattern` | | gh log text matches / does not match |

Rules enforced at load (`ValidateChecks`, error names case file, check index and type):

- Unknown `type`, missing `criterion`, missing required field, **or a field that does not belong to the type** (catches
  typos like `expected_exit`) → error. `allow` missing is an error; `allow: []` is valid (builder: confirm `yaml.v3`
  yields nil vs non-nil empty slice, otherwise use `*[]string`).
- `path` must satisfy `filepath.IsLocal` (no absolute path, no `..`). `pattern` must compile as RE2 (unanchored,
  matched against the whole text; authors use `(?m)` / `(?s)` as needed).
- `expect_exit` must be 0–255. `inject` only on `command`; each entry is a path relative to
  `<evals-dir>/fixtures/hidden/`, must be local, a regular file (Lstat, **no symlinks**), must exist, and its
  destination (same relative path under `Dir`) must not start with `.git`, `.eval` or `.kiro`. The loader resolves the
  absolute source into the unexported `injectDir` so the evaluator needs no evals-dir.
- `criterion` must exist in the agent's rubric and must not be `type: cost`. Cross-validation runs inside `loadCases`
  (it loads the agent's rubric via `loadRubrics(agent)`; if the rubric cannot be loaded this step is skipped and the
  existing rubric errors surface elsewhere). Failures wrap a sentinel `errInvalidChecks`.
- `gh_log_*` checks do **not** require `requires_sandbox` (the native self-test, `task eval:selftest`, must be able to
  pass every type). Natively the real `gh` is not logged, so the gh log is empty unless the stub/agent wrote
  `.eval/gh.log`; documented caveat. An empty/missing log is `""`, so `gh_log_not_contains` passes vacuously.

### 2.2 Evaluation semantics

- **Order.** All non-`command` checks are evaluated first, against the workspace exactly as the agent left it. `command`
  checks run afterwards, in listed order. Results are returned in the original list order. This makes `file_*` and
  `changed_files` independent of command side effects and of `inject`.
- **`command`.** `sh -c <run>`, `cmd.Dir = Dir`, stdin `/dev/null`, environment = process environment minus the
  GitHub credential variables in `blockedContainerEnv` (`GH_TOKEN`, `GITHUB_TOKEN`, …). Own process group, killed on
  timeout; package var `checkCommandTimeout = 5 * time.Minute` (a seam for tests; not a schema field). Combined
  output captured (cap 64 KiB); the tail (≤512 bytes) goes into `Detail` on failure. Start failure, timeout and signal
  death fail the check with a descriptive `Detail`.
- **`inject`.** Before a `command` runs, each file is copied into `Dir` (creating parent dirs; if a file already exists
  there — e.g. the agent wrote its own `hidden_test.go` — the fixture overwrites it for the run). After the command,
  the workspace is restored: injected files that did not exist are removed (and empty dirs the injection created),
  overwritten files are put back from a temp backup. Hence the agent never sees injected files (they exist only after
  it finished), and later checks and `--keep-workspaces` see the workspace as the agent left it.
- **Path safety (all `path`/`inject` destinations).** Resolve `Dir`+rel by walking components with `Lstat`; if any
  existing *intermediate* component is a symlink the check fails ("path traverses a symlink"). `file_contains` /
  `file_not_contains` also refuse a symlink as the final component and non-regular files; `file_exists` / `file_absent`
  use `Lstat` on the final component. This prevents an agent from pointing `out.txt` at a host file or an `inject`
  destination outside the workspace. File reads are capped at 10 MiB (over the cap → failed check with a message).
- **`changed_files`.** In `Dir` with `hermeticGitEnv()` and `-c core.fsmonitor=false -c core.hooksPath=/dev/null`
  (repo config is agent-controlled): `git diff --name-status --no-renames -z <Base>` (staged + unstaged tracked changes
  vs the base, so an agent that ran `git commit` cannot hide changes) plus
  `git ls-files --others --exclude-standard -z` (untracked). Paths under `.eval/` are ignored. Each remaining path
  must match at least one `allow` glob; offenders are listed with their kind (`added`/`modified`/`deleted`). Not a git
  repo / git error → failed check with the error. Paths ignored by git (`.gitignore`, `info/exclude`, e.g. `.kiro/`)
  are invisible — documented. Glob dialect (`checks_glob.go`): `*` = any run of non-`/`, `?` = one non-`/`, `[...]`
  class, `**` = any run including `/` (`docs/**` matches everything below `docs/`; `**/*.go` any `.go` file at any
  depth), match against the whole slash-separated, repo-relative path, a pattern without `/` matches only a top-level
  path. Invalid glob → load error.
- **`output_*`**: `Output` as given. **`gh_log_*`**: `GHLog` as given.

### 2.3 Scoring integration

`scoreCase` evaluates all of the case's checks **once** (commands are expensive), groups results by `criterion`, then in
the existing criterion loop:

- criterion has checks → `CriterionScore{Name, Score: passed, MaxScore: total, Deterministic: true, Reasoning, Checks: results}`; no
  heuristic and no judge call.
- criterion has no checks → existing code, byte-for-byte (AC5).
- a check naming a criterion absent from the loop (hand-built case bypassing the loader) → an extra
  `CriterionScore{Score: 0, MaxScore: <its check count>, Reasoning: "criterion %q is not in the rubric"}`, never silently dropped.

`CriterionScore` gains `Checks []CheckResult \`json:"checks,omitempty"\`` (additive; older result files still load).
`CheckResult` JSON: `{ "index": 2, "type": "file_exists", "label": "#2 file_exists path=missing.txt", "passed": false, "detail": "file does not exist" }`.

Reasoning format (AC2):

- all pass: `3/3 checks passed`
- otherwise: `1/2 checks passed; failed: #2 file_exists path=missing.txt (file does not exist); #4 …` — each failed
  check is named with its 1-based position, type and key fields (`command "…"`, `path=…`, `pattern=/…/`, `allow=[…]`).
  Each `detail` is truncated to 200 chars in reasoning (full text, ≤2 KiB, stays in `Checks[].detail`).

Inputs supplied by `scoreCase`: `Dir = cr.WorkspaceDir`, `Output = cr.ActualOutput`, `GHLog` = contents of
`<Dir>/.eval/gh.log` (empty if absent), `Base` = new `caseWorkspace.BaseCommit` (the fixture commit sha, recorded by
`gitInit` via `git rev-parse HEAD`; plumbed through a new unexported `CaseResult.baseCommit`, like
`containerWorkspaceDir`, not serialized).

**Agent failed / no workspace.** If `cr.ActualOutput == ""` (agent call failed, timed out, sandbox refusal) or
`cr.WorkspaceDir == ""`, checks are **not evaluated**; each checked criterion records `0/<total>` (not skipped) with
reasoning `agent produced no output; checks not run`. Rationale: checks assert on what the agent *did*; a skipped
criterion would be excluded from the aggregate and make failed runs look better. (Legacy criteria keep their existing
"skipped: no output" behavior.)

Console: `printCaseResult` additionally prints one line per failed check (`      ✗ <label>: <detail>`) whenever a
case has any failed check, independent of the percentage threshold. Thresholds/exit codes are untouched (out of scope).

`runProgressiveEvaluation` currently downgrades *any* `loadCases` error to a warning and skips the agent; an invalid
`checks` block would therefore silently drop the agent's cases and exit 0. Make errors wrapping `errInvalidChecks` fatal
(missing cases directory remains a warning, other errors unchanged).

### 2.4 Security and trust (flag to maintainers)

- **`command` checks execute on the host, including in `--sandbox` runs.** The base image has no language toolchains
  (alpine + git/gh/kiro-cli), and the evaluator must work on any directory, so a container runner is not part of this
  change. Consequence: a `command` check such as `go test ./...` runs *agent-authored* code with the developer's
  privileges, which weakens the sandbox's containment for those cases. Mitigations in this design: credential env
  vars scrubbed, timeout + process-group kill, no stdin, `.git`-config hardening for the git calls, symlink guards.
  Docs must state this prominently; `command` is for trusted case authors reviewing agent changes. A future container
  runner can slot in behind the `runCheckCommand` function variable (package-level seam introduced here for tests).
- The gh log lives in `.eval/`, which the agent can write; it is evidence from the fake `gh`, not tamper-proof.
- No network listeners or new auth surfaces are added.

## 3. Relevant Files

Create (all `internal/eval/`):

- `checks.go` — `CheckType` consts, `Check`, `CheckResult`, `CheckInput`, `ValidateChecks`, label/reasoning formatting.
- `checks_eval.go` — `EvaluateChecks`, per-type evaluators, safe path resolution, inject/restore, `runCheckCommand`.
- `checks_git.go` — `changedPaths(dir, base)` (diff + untracked) and `.eval/` filtering.
- `checks_glob.go` — `globMatch` / `compileGlob`.
- `checks_proc_unix.go`, `checks_proc_other.go` — process-group helper (same pattern as `internal/inference/stubcmd_*.go`).
- Tests: `checks_test.go` (schema/validation/labels), `checks_glob_test.go`, `checks_eval_test.go`,
  `checks_scoring_test.go` (scoreCase integration, AC2/AC5), `selftest_checks_test.go` (end-to-end stub runs, AC3/AC4),
  `checks_docs_test.go` (AC1 doc coverage).
- Fixtures under `internal/eval/testdata/evals/`: `fixtures/hidden/hidden_test.go` (content `package hidden\n`, gofmt-clean
  because `gofmt -l .` walks `testdata`), and the case files listed in §5.

Modify:

- `internal/eval/types.go` — `TestCase.Checks []Check`, `CriterionScore.Checks`, unexported `CaseResult.baseCommit`.
- `internal/eval/scoring.go` — checks branch in `scoreCase`; `printCaseResult` failed-check lines.
- `internal/eval/runner.go` — `loadCases` (validate + rubric cross-check), `runProgressiveEvaluation` (fatal on `errInvalidChecks`).
- `internal/eval/workspace.go` — `caseWorkspace.BaseCommit` set in `gitInit`; `execute_case.go` copies it onto `cr.baseCommit`.
- `internal/eval/selftest_test.go` (case count `5` → new total at line ~141), `sandbox_workspace_test.go` (`len(edited.Cases) != 5` at ~388),
  and any other test that asserts the `selftest`/`selftest-fail` case set or count (`grep -n "selftest-fail\|!= 5" internal/eval/*_test.go`;
  `parity_sandbox_test.go`, `selftest_sandbox_test.go` iterate dynamically and should need no change).
- `internal/eval/testdata/evals/agents/selftest-fail.json` and `selftest-fail-prompt.md` — drop "its only case… timeout"
  wording; it now hosts the failing check cases plus the timeout case.
- `docs/evaluation.md` — see Task 6.

Not touched (out of scope): `.kairon/evals/**`, `cmd/kairon/templates/**`, legacy heuristics in `scoreDeterministic`, thresholds/exit codes.

## 4. Team Orchestration

```
checks-types-load ─┐
                   ├─► checks-evaluate ─► checks-scoring ─► selftest-fixtures ─► docs-evaluation ─► validate-all
checks-glob ───────┘
```

- `checks-types-load` and `checks-glob` have no dependency on each other and run in parallel (disjoint files).
- Everything after is sequential: evaluation needs the types and glob; scoring needs the evaluator; the fixtures need
  the scoring path to run end to end; docs describe the final behavior and case list.
- All work lands in **one PR**. Every builder task follows AGENTS.md TDD: write the failing test, run it and confirm it
  fails **for the expected reason** (not a compile error), implement minimally, re-run. Tests and implementation are
  committed together (one commit per PR is fine; never a red-only commit). The PR description names the tests seen failing first.
- Builders: run `go vet ./internal/eval` and `gofmt -l internal/` before finishing each task. No template sync is
  required (`internal/eval/testdata` is not a synced path); `task sync:check` is still run once by the validator.

## 5. Step-by-Step Task Breakdown

### Task 1 — `checks-types-load`: schema, validation, loader integration
Files: `checks.go`, `types.go`, `runner.go` (`loadCases`, `runProgressiveEvaluation`), `checks_test.go`.
Acceptance: YAML `checks` list decodes into `TestCase.Checks` with the exact field names of §2.1; every validation rule in
§2.1 has a table-driven test (unknown type, wrong-type field, missing `allow`, bad regex, bad glob, absolute/`..` path,
missing/symlink inject source, `inject` on non-command, unknown criterion, cost criterion, `expect_exit` range);
default `expect_exit` is 0; `loadCases` returns an error wrapping `errInvalidChecks` naming file/index/type;
`runProgressiveEvaluation` fails (instead of warning) for such an error; a case without `checks` loads exactly as before.
Failing-test-first: `TestLoadCases_RejectsUnknownCheckType` etc. fail with "no error returned" before implementation.

### Task 2 — `checks-glob`: glob matcher
Files: `checks_glob.go`, `checks_glob_test.go`. Acceptance: dialect of §2.2 (`*`, `?`, `[..]`, `**`, whole-path
match, top-level-only for slash-less patterns); invalid patterns return an error; table tests cover `README.md`,
`docs/**`, `**/*.go`, `*.md` (not matching `docs/a.md`), `a/**/b`, bad `[`.

### Task 3 — `checks-evaluate`: the evaluator
Files: `checks_eval.go`, `checks_git.go`, `checks_proc_*.go`, `checks_eval_test.go`. Depends on 1, 2.
Acceptance: each of the ten types has a passing and a failing unit test on a `t.TempDir()`; evaluation uses only
`CheckInput` (tests build checks by hand with no evals dir, no `cfg`); `changed_files` is tested for added / modified /
deleted / untracked / `.eval/` ignored / agent-committed change / `Base` / not-a-repo; commands: exit match, non-zero
expected, timeout (via `checkCommandTimeout`), credential env scrubbed, runs in `Dir`; inject: file present during the
command, absent afterwards, overwritten file restored, empty created dirs removed, destination symlink escape refused,
`file_contains` refuses a symlink to an outside file; non-command checks are order-independent of command side effects.

### Task 4 — `checks-scoring`: scoring integration
Files: `scoring.go`, `types.go`, `workspace.go` (`BaseCommit`), `execute_case.go`, `checks_scoring_test.go`. Depends on 1, 3.
**Step 0 (characterization, committed with the change):** before editing `scoreCase`, add a test that pins the current
output of the five existing `selftest` cases without checks: `structural_completeness` 5/5 reasoning
`found 2/2 expected structural elements`, `clarity` 5/5 — it must pass before and after (AC5).
Acceptance: checked criterion → `Score=passed`, `MaxScore=total`, `Deterministic=true`, no judge call recorded for it
(`cr.Calls` has no judge record for that criterion); mixed case (one criterion with checks, one without) scores the
latter exactly as before; reasoning names every failed check (AC2: one pass + one fail → `1/2 checks passed; failed: #2 …`);
check on a criterion missing from the rubric yields a visible 0/N score; empty output / empty workspace → `0/N` not
skipped with the "agent produced no output" reasoning and **no command is executed**; `GHLog` read from `.eval/gh.log`;
`Base` is the fixture commit (an agent-committed change still fails `changed_files`); `printCaseResult` prints failed-check
lines and is otherwise byte-identical for cases without checks (existing `TestPrintCaseResultStatuses` untouched and green).

### Task 5 — `selftest-fixtures`: self-test cases and end-to-end tests
Files: `testdata/evals/cases/selftest/check-*.yaml`, `testdata/evals/cases/selftest-fail/check-*.yaml`,
`testdata/evals/fixtures/hidden/hidden_test.go`, `selftest-fail` agent description/prompt, `selftest_checks_test.go`,
count updates in existing tests. Depends on 4. All new cases use `structural_completeness` (already in both rubrics, so
**no rubric change**, which keeps the five legacy cases' scores identical), stub commands only, and a response containing
`## ` / `### ` headings. Tests are written first (they fail because the cases do not exist yet), then the cases.

`selftest` (every case passes, 100%): `check-command` (`test -f built.txt`; plus `run: "exit 3"` with `expect_exit: 3`),
`check-command-inject` (AC4: stub runs `ls -R > .eval/seen.txt`; checks: `command` `test -f hidden_test.go && grep -q hidden hidden_test.go`
with `inject: [hidden_test.go]`; `file_not_contains` `.eval/seen.txt` pattern `hidden_test\.go`; `file_absent` `hidden_test.go`),
`check-file-exists`, `check-file-absent`, `check-file-contains`, `check-file-not-contains`, `check-changed-files`
(workspace `seeded`; stub appends to `README.md` and writes `.eval/note.txt`; `allow: [README.md]` → `.eval/` ignored),
`check-output-contains`, `check-output-not-contains`, `check-gh-log-contains` (stub: `echo "gh issue create --title t" >> .eval/gh.log`;
pattern `(?m)^gh issue create`), `check-gh-log-not-contains` (pattern `gh pr merge`).

`selftest-fail` (cases built to fail): one `check-<type>` per type, each with a check that must fail — `command`
(`exit 3`, default expect 0), `file-exists` (`missing.txt`), `file-absent` (`marker.txt` exists), `file-contains` /
`file-not-contains` (inverted pattern), `changed-files` (workspace `seeded`; stub deletes `docs/notes.txt` and creates
`extra.txt`, `allow: [README.md]` → reasoning names both), `output-contains` / `output-not-contains`,
`gh-log-contains` (empty log), `gh-log-not-contains` (stub appends `gh pr merge 1` to `.eval/gh.log`) — plus
`check-partial` (AC2: `file_exists marker.txt` passes, `file_exists missing.txt` fails → exactly `1/2`, reasoning names `#2 file_exists path=missing.txt`).
The existing `stub-timeout` case stays.

`selftest_checks_test.go` (stub backend, native, `copyFixturesTo` temp copy like other tests; no `kiro-cli`):
a table `type → want pass/fail` asserting for every one of the ten types that the `selftest` case scores `N/N` and the
`selftest-fail` case scores `< N/N` with each failed check named in `reasoning` and `checks[].passed` as expected (AC3);
`check-partial` is `1/2` (AC2); the AC4 test asserts `seen.txt` does not contain `hidden_test.go`, the `command` check
passed, and `hidden_test.go` is absent from the kept workspace afterwards; legacy cases keep their pinned scores (AC5);
`RunWithOptions` for `selftest` returns nil. Update the hard-coded case counts in the existing tests.

### Task 6 — `docs-evaluation`: documentation
Files: `docs/evaluation.md`, `checks_docs_test.go`. Depends on 5. Content: a new **Checks** section (after "Test Case
Format") documenting the `checks` field table, each of the ten types with a YAML example, the glob dialect, regex
dialect, evaluation order, `inject` + `fixtures/hidden/`, scoring/reasoning format and the `checks` JSON in results,
the agent-failed behavior, the empty-gh-log-on-native caveat, the **host-execution warning for `command`** (§2.4), and
the reusable-evaluator note (`EvaluateChecks(checks, CheckInput{Dir, Output, GHLog})`). Also update: directory-structure
tree (`fixtures/hidden/`), the Test Case Format field list (`checks`), "How Scoring Works" (checks override heuristics;
legacy heuristics remain for criteria without checks), the "Case Workspaces" note that says `file_exists`/`changed_files`
"are not part of this change" (now implemented — reword), and the Self-Test section (file tree, case counts: `selftest`
has 5 + 11 cases, all pass; `selftest-fail` is expected to fail and why). `checks_docs_test.go` asserts the doc mentions each
`CheckType` in a `type: <name>` YAML example (AC1). Documentation-only edits need no test first beyond that doc-coverage test.

### Task 7 — `validate-all`: independent verification (validator, read-only)
Runs the full gate and walks the five acceptance criteria and constraints (see plan).

## 6. Acceptance-Criteria Traceability

| Issue AC | Where satisfied | Verified by |
|---|---|---|
| 1 `checks` list, ten types, fields | §2.1, Tasks 1/3/6 | `checks_test.go`, `checks_eval_test.go`, `checks_docs_test.go` |
| 2 `passed/total`, reasoning names failures | §2.3, Task 4/5 | `check-partial` in `selftest-fail` → `1/2` |
| 3 pass case per type (`selftest`) + fail case per type (`selftest-fail`); `task eval:selftest` exits 0 | Task 5 | `selftest_checks_test.go`; `task eval:selftest` |
| 4 agent cannot see `inject` files | §2.2 inject, Task 3/5 | `check-command-inject` + `seen.txt` assertion |
| 5 unchecked criteria unchanged | §2.3, Task 4 step 0 | pinned characterization test on the five legacy cases |
| Constraint: evaluable on any dir/output/gh log | §2 decision 1 | `checks_eval_test.go` builds checks with only `CheckInput` (manual review by validator) |

## 7. Validation Commands

```bash
go test ./internal/eval -run 'Check' -count=1
go test ./... -count=1            # task test
task lint
task fmt:check
task sync:check
task eval:selftest                 # exit 0
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail   # exit 0; failures recorded in results JSON
```

## 8. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "checks-types-load"
    agent: "builder"
    description: "Add the checks schema (Check, CheckType, CheckResult, CheckInput), ValidateChecks, TestCase.Checks and CriterionScore.Checks, and integrate validation (including rubric cross-check) into loadCases; make errInvalidChecks fatal in runProgressiveEvaluation. TDD: write failing table tests first."
    dependencies: []
    acceptance_criteria:
      - "YAML 'checks' decodes into TestCase.Checks with fields criterion, type, run, expect_exit, inject, path, pattern, allow exactly as named in the issue"
      - "Unknown type, missing required field, field not valid for the type, missing allow, bad regex, bad glob, absolute or '..' path, bad/symlink inject source, inject on a non-command check, unknown rubric criterion and cost criterion are each rejected at load with an error naming the case file, check index and type"
      - "expect_exit defaults to 0; allow: [] is valid while a missing allow is an error"
      - "Errors wrap errInvalidChecks and runProgressiveEvaluation returns them instead of downgrading to a warning; a missing cases directory is still only a warning"
      - "A case without checks loads exactly as before"
    validation_commands:
      - "go test ./internal/eval -run 'TestChecks|TestLoadCases' -count=1"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/)\""

  - id: "checks-glob"
    agent: "builder"
    description: "Implement the in-package glob matcher (*, ?, [..], ** across slashes, whole-path match) used by changed_files allow patterns, with table-driven tests. No new module dependency."
    dependencies: []
    acceptance_criteria:
      - "'README.md' matches only the top-level README.md; '*.md' does not match 'docs/a.md'; 'docs/**' matches every path below docs/; '**/*.go' matches .go files at any depth; 'a/**/b' matches a/b and a/x/y/b"
      - "An invalid pattern (e.g. unterminated '[') returns an error that is surfaced as a load error by ValidateChecks"
      - "go.mod and go.sum are unchanged"
    validation_commands:
      - "go test ./internal/eval -run 'TestGlob' -count=1"
      - "go vet ./internal/eval"
      - "git diff --exit-code go.mod go.sum"

  - id: "checks-evaluate"
    agent: "builder"
    description: "Implement EvaluateChecks(checks, CheckInput{Dir, Output, GHLog, Base}) for all ten check types: safe symlink-refusing path resolution, regex file/output/gh-log checks, git-based changed_files (diff vs Base + untracked, .eval/ ignored, hardened git invocation), command execution with timeout/process group/credential-scrubbed env, and inject copy-in with restore. Non-command checks run before command checks; results keep list order. TDD with temp dirs and real git."
    dependencies: ["checks-types-load", "checks-glob"]
    acceptance_criteria:
      - "Each of the ten types has at least one passing and one failing unit test using only CheckInput (no cfg, no caseWorkspace, no evals dir)"
      - "changed_files reports added, modified, deleted and untracked paths outside allow, ignores .eval/, detects a change the agent committed (via Base), and fails cleanly outside a git repository"
      - "command honours expect_exit (including non-zero), runs in Dir, times out via checkCommandTimeout with the process group killed, and does not see GH_TOKEN/GITHUB_TOKEN"
      - "inject files exist while the command runs and are gone afterwards; a pre-existing file at the destination is restored; destinations reached through a symlink are refused"
      - "file_contains/file_not_contains refuse symlinks and files over 10 MiB with a failed result, never a panic or silent pass"
      - "An invalid hand-built Check yields a failed CheckResult with an explanatory detail"
    validation_commands:
      - "go test ./internal/eval -run 'TestEvaluateChecks|TestChecksEval' -count=1"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/)\""

  - id: "checks-scoring"
    agent: "builder"
    description: "Integrate checks into scoreCase: first add a characterization test pinning the scores of the five existing selftest cases, then score checked criteria as passed/total with reasoning naming each failed check, record per-check results in CriterionScore.Checks, read the gh log from .eval/gh.log, pass the fixture commit as Base (caseWorkspace.BaseCommit -> CaseResult.baseCommit), handle empty output/workspace as 0/N, and print failed-check lines in printCaseResult."
    dependencies: ["checks-types-load", "checks-evaluate"]
    acceptance_criteria:
      - "The characterization test for the unchecked selftest cases (structural_completeness 5/5 'found 2/2 expected structural elements', clarity 5/5) passes before and after the change"
      - "A criterion with checks scores Score=passed, MaxScore=total, Deterministic=true and triggers no judge call; criteria without checks in the same case are scored exactly as before"
      - "A case with one passing and one failing check on one criterion records 1/2 and its reasoning contains the failed check's label (e.g. '#2 file_exists path=missing.txt') and detail"
      - "A check naming a criterion not in the rubric produces a visible 0/N score instead of being dropped"
      - "With empty output or no workspace the checked criteria score 0/N, are not skipped, carry reasoning 'agent produced no output; checks not run', and no command is executed"
      - "changed_files uses the fixture commit as Base so an agent-committed change is still detected"
      - "printCaseResult output is byte-identical for cases without checks and lists failed checks for cases with them"
    validation_commands:
      - "go test ./internal/eval -run 'TestChecksScoring|TestPrintCaseResult|TestScore' -count=1"
      - "go test ./internal/eval -count=1"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal/)\""

  - id: "selftest-fixtures"
    agent: "builder"
    description: "Add the self-test check cases: 11 passing cases under agent selftest (one per type plus the AC4 inject case) and 10 per-type failing cases plus check-partial under selftest-fail, fixtures/hidden/hidden_test.go, updated selftest-fail agent description/prompt, and selftest_checks_test.go end-to-end assertions. Write the tests first (they fail because the cases do not exist), then add the cases. Update hard-coded case counts in existing tests. Do not edit rubrics (keeps the legacy cases' scores identical)."
    dependencies: ["checks-scoring"]
    acceptance_criteria:
      - "Every one of the ten check types has a passing case under selftest and a failing case under selftest-fail; selftest results show N/N for all check cases and selftest-fail shows < N/N with each failed check named in reasoning"
      - "selftest-fail check-partial records exactly 1/2 and its reasoning names the failing check"
      - "check-command-inject: .eval/seen.txt does not mention hidden_test.go, the command check passed with the file present, and hidden_test.go is absent from the kept workspace afterwards"
      - "The five pre-existing selftest cases keep their pinned scores (no 'checks', rubrics unchanged)"
      - "task eval:selftest exits 0 and the selftest-fail run exits 0 with failures recorded in the results JSON"
      - "Existing tests that hard-code the selftest case count (selftest_test.go, sandbox_workspace_test.go) are updated and green; gofmt -l reports nothing (hidden_test.go is gofmt-clean)"
      - "No file under .kairon/evals or cmd/kairon/templates is modified"
    validation_commands:
      - "go test ./internal/eval -run 'TestSelfTestChecks|TestSelfTest' -count=1"
      - "go test ./internal/eval -count=1"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail"
      - "test -z \"$(gofmt -l .)\""
      - "git diff --exit-code -- .kairon/evals cmd/kairon/templates"

  - id: "docs-evaluation"
    agent: "builder"
    description: "Update docs/evaluation.md: new Checks section documenting all ten types with a YAML example each, glob/regex dialects, evaluation order, inject and fixtures/hidden, scoring and reasoning format, result JSON, agent-failed behavior, native gh-log caveat, host-execution warning for command checks, and the reusable EvaluateChecks inputs; update directory tree, Test Case Format, How Scoring Works, the obsolete 'not part of this change' note in Case Workspaces, and the Self-Test section. Add checks_docs_test.go asserting each CheckType appears in a 'type: <name>' example."
    dependencies: ["selftest-fixtures"]
    acceptance_criteria:
      - "docs/evaluation.md documents every one of the ten check types with a YAML example containing 'type: <name>' and its fields"
      - "The doc states that command checks run on the host (also under --sandbox), the scoring rule (passed out of total, failed checks named), that criteria without checks use the legacy heuristics, and the evaluation order"
      - "Directory tree, Test Case Format, Case Workspaces note and Self-Test file list/case counts match the implemented behavior and fixtures"
      - "checks_docs_test.go passes and fails if a CheckType is missing from the doc"
    validation_commands:
      - "go test ./internal/eval -run 'TestChecksDocs' -count=1"
      - "grep -c 'type: ' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of the whole change against the five issue acceptance criteria and the constraints, including that EvaluateChecks needs only Dir, Output and GHLog (plus optional Base) and that AGENTS.md TDD and frozen-path rules were respected."
    dependencies: ["docs-evaluation"]
    acceptance_criteria:
      - "task test, task lint, task fmt:check and task sync:check all pass"
      - "task eval:selftest exits 0; the selftest-fail run records the expected failures; results JSON shows per-case pass/fail for every check type"
      - "AC2: selftest-fail check-partial records 1/2 and names the failing check; AC4: seen.txt omits hidden_test.go while the command check passed; AC5: legacy selftest cases unchanged"
      - "docs/evaluation.md documents every check type with an example"
      - "EvaluateChecks signature takes only a checks slice and CheckInput{Dir, Output, GHLog, Base optional}; it references no cfg/caseWorkspace/TestCase (manual code review)"
      - "No frozen paths edited (.kairon/specs other than this spec, .kairon/evals/results, CHANGELOG.md) and .kairon/evals/cases untouched"
    validation_commands:
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail"
      - "grep -n 'func EvaluateChecks' internal/eval/checks_eval.go"
      - "git diff --exit-code -- .kairon/evals/cases CHANGELOG.md"
```

## 9. Risks and Open Points

- **Host execution of `command` checks** (§2.4) is the one real containment trade-off; recommended follow-up (not in this
  PR): a container-backed `runCheckCommand` for `--sandbox` runs once the base image can carry toolchains.
- Adding cases to `selftest`/`selftest-fail` changes counts asserted by existing tests and by gated sandbox tests
  (`TestSelftestSandbox`, `TestProvenanceParitySandbox`, `TestSandboxWorkspace`); they compare native vs container
  results case by case, and the new cases use only POSIX stub commands and host-side checks, so parity should hold. They
  are daemon-gated and cannot be run in every environment; the validator will state if they were not run.
- `yaml.v3` nil-vs-empty handling for `allow` must be confirmed by the first test in Task 1.
- Native runs have no gh log (real `gh` is not logged): `gh_log_not_contains` passes vacuously there; documented.
