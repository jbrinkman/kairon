# Issue #346: Move the check engine out of internal/eval and add `kairon check`

Closes #346

## Problem

`EvaluateChecks` (`internal/eval/checks_eval.go`) looks only at a `CheckInput` (directory, output, gh log, ...), but
it lives in `internal/eval`, which also holds the eval runner and imports `internal/eval/sandbox`. The workflow
engine cannot reuse the checks without linking the harness, and there is no way to run a checks file against a real
directory outside an eval case.

Verified in the code:

- The engine files (`checks.go`, `checks_eval.go`, `checks_command.go`, `checks_fact.go`, `checks_git.go`,
  `checks_glob.go`, `checks_inject.go`, `checks_judge.go`, `checks_proc_unix.go`, `checks_proc_other.go`) reference
  only a handful of harness symbols:
  - `Rubric` (only `checkCriterion` / `ValidateChecks`' `rubric` argument),
  - `blockedContainerEnv` (`runner.go`; used by `checkCommandEnv`),
  - `hermeticGitEnv` (`workspace.go`; used by `checks_git.go`),
  - `errInvalidChecks` is *defined* in `checks.go` but wrapped by `loadCases` in `runner.go`,
  - `errJudgeParse`, `SimDecision`, `simLogRel`, `shorten`, `stripANSISequences` are shared with
    `simulated_user.go` / `scoring.go` / `runner.go`.
- The judge check is split: the types, file loading and `parseYesNo` are in `checks_judge.go`, the prompt builder and
  the backend call (`buildYesNoPrompt`, `newCheckJudge`) are in `internal/eval/judge_check.go`. `newCheckJudge` reads
  the package global `cfg` (`cfg.backend`, `cfg.pins.judgeModel()`, `cfg.debug`) and eval's `CaseResult`/`CallRecord`
  types, so it cannot move as-is.
- `internal/inference` depends on nothing else in the repo (`go list -deps ./internal/inference` lists only itself),
  so the new package may use it. `internal/config` depends only on `internal/logging`.
- With `--backend stub` a yes/no judge request needs `Request.Stub.Judge`; with `Stub == nil` the stub backend
  returns the error `case has no stub.judge`. `kairon check` has no case, so it must supply a script itself (see
  "Judge in `kairon check`").

## Solution approach

1. **New package `internal/checks`** (import `github.com/jbrinkman/kairon/internal/checks`, package `checks`) holds
   check types, validation, evaluation, the judge types/prompt/parse and a backend-based judge builder. It imports
   only the standard library, `gopkg.in/yaml.v3` and `internal/inference`. It must NOT import `internal/eval`,
   `internal/eval/sandbox` or anything that does. A test enforces this with `go list -deps`.
2. **`internal/eval` becomes a client of `internal/checks`.** The engine files move with `git mv` (history kept).
   `internal/eval` keeps thin aliases/wrappers so existing eval code and tests keep their names, and so check
   behavior and eval scores do not change. No check type, schema rule, message or limit changes (issue "Out").
3. **`kairon check`** (`cmd/kairon/cmd/check.go`) loads a YAML list of checks, validates all of them before running any,
   evaluates them against `--dir`, prints one line per check and exits 0 only if all pass.

### Exported API of `internal/checks`

Keep the existing identifier names except where the `checks.` prefix makes them redundant (this keeps the move
mechanical and the diff reviewable):

| Was (`internal/eval`) | Now (`internal/checks`) |
|---|---|
| `Check`, `CheckType`, `CheckCommand` … `CheckFactNotAsked` | same names |
| `CheckResult` | `Result` |
| `CheckInput` | `Input` |
| `TurnEvidence` | same |
| `EvaluateChecks(checks, in)` | `Evaluate(checks, in)` |
| `ValidateChecks(file, checks, hiddenDir, rubric)` | `Validate(file, checks, ValidateOptions)` |
| `errInvalidChecks` | `ErrInvalid` |
| `JudgeFunc`, `JudgeQuery`, `JudgeVerdict`, `JudgeFile`, `JudgeTurn` | same |
| `errJudgeParse` | `ErrJudgeParse` |
| `parseYesNo`, `buildYesNoPrompt` | `ParseYesNo`, `BuildYesNoPrompt` |
| `SimDecision`, `simLogRel` | `SimDecision`, `SimLogRel` |
| `shorten`, `stripANSISequences` | `Shorten`, `StripANSI` |
| `blockedContainerEnv` | `GitHubCredentialEnv` (map[string]bool) |
| `hermeticGitEnv` | `HermeticGitEnv` (see below) |
| (new) | `LoadFile(path)`, `InferenceJudge`, `JudgeCall` |

Design decisions:

- **Decoupling from `Rubric`.**

  ```go
  // ValidateOptions configures Validate.
  type ValidateOptions struct {
      // HiddenDir is where `inject` sources are resolved ("" = no inject allowed, as today).
      HiddenDir string
      // Criterion, when non-nil, is called with every check's criterion and returns an error to reject it
      // (eval passes the rubric cross-check). nil skips the cross-check.
      Criterion func(name string) error
      // OptionalCriterion lets a check omit `criterion` (kairon check has no rubric). When false, a missing
      // criterion is the existing error "criterion is required".
      OptionalCriterion bool
  }
  ```

  `Validate` returns errors wrapping `ErrInvalid` with the exact existing text
  `invalid checks: <file>: check #<n> (<type>): <reason>` (type `?` when empty). `checkCriterion(name, rubric)` stays
  in `internal/eval` and is wrapped into the `Criterion` callback.
- **`internal/eval/checks_alias.go` (new)** re-exports for eval's own use and tests:
  `type Check = checks.Check`, `type CheckType = checks.CheckType`, the `Check*` constants, `type CheckResult =
  checks.Result`, `type CheckInput = checks.Input`, `type TurnEvidence = checks.TurnEvidence`, `type JudgeQuery`,
  `JudgeVerdict`, `JudgeFunc`, `JudgeFile`, `JudgeTurn`, `SimDecision`; `var errInvalidChecks = checks.ErrInvalid`,
  `var errJudgeParse = checks.ErrJudgeParse`, `const simLogRel = checks.SimLogRel`;
  `var blockedContainerEnv = checks.GitHubCredentialEnv`; wrappers
  `func EvaluateChecks(c []Check, in CheckInput) []CheckResult { return checks.Evaluate(c, in) }`,
  `func ValidateChecks(file string, c []Check, hiddenDir string, rubric *Rubric) error` (builds `ValidateOptions`
  with `Criterion` from `checkCriterion`), `shorten`, `stripANSISequences` (delegating; delete `ansiRegex` from
  `runner.go`). `TestCase.Checks` stays `[]Check` (alias), so YAML decoding of cases is unchanged.
- **`HermeticGitEnv`** in `checks` returns the environment minus `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
  `GIT_OBJECT_DIRECTORY`, `GIT_COMMON_DIR`, `GIT_CONFIG*`, `GIT_AUTHOR_*`, `GIT_COMMITTER_*`, plus
  `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`. Eval's `hermeticGitEnv()` becomes
  `append(checks.HermeticGitEnv(), "GIT_AUTHOR_DATE="+fixedGitDate, "GIT_COMMITTER_DATE="+fixedGitDate)` so workspace
  git commits stay byte-identical. The check engine's read-only `git diff` / `ls-files` never used the fixed dates, so
  check behavior is unchanged.
- **Inference-backed judge, shared by eval and the CLI.**

  ```go
  // JudgeCall describes one judge model call, for the caller's accounting.
  type JudgeCall struct {
      Criterion string
      Request   inference.Request
      Response  inference.Response
      Err       error          // the Invoke error, if any
      ParseErr  error          // set when the reply was not a yes/no verdict
      Wall      time.Duration
  }

  // InferenceJudge builds a JudgeFunc that asks a yes/no question of Backend.
  type InferenceJudge struct {
      Backend inference.Backend
      Model   string                // pinned judge model ("" = unpinned)
      Stub    *inference.StubScript // only the stub backend reads it
      Debug   io.Writer             // when non-nil, each prompt is written to it
      OnCall  func(JudgeCall)       // optional; called after every Invoke
  }
  func (j InferenceJudge) Func() JudgeFunc
  ```

  `Func()` is a line-for-line move of `newCheckJudge`'s body: `inference.Request{Role: RoleJudge, Prompt:
  BuildYesNoPrompt(q), Timeout: judgeCheckTimeout (2m), Model: j.Model, Stub: j.Stub, YesNo: true}`, the debug line
  `🔧 Debug: judge prompt (criterion=%s, judge check #%d)\n%s\n`, `OnCall` after the call, `ParseYesNo(resp.Text)` after
  that. Eval's `newCheckJudge(tc, cr)` becomes a ~15-line wrapper: `InferenceJudge{Backend: cfg.backend, Model:
  cfg.pins.judgeModel(), Stub: tc.Stub, Debug: <debugWriter if cfg.debug>, OnCall: func(c JudgeCall){ rec :=
  newCallRecord(c.Request, c.Response, c.Err, c.Wall, costFromUsage(c.Response.Model, c.Response.Usage));
  rec.Criterion = c.Criterion; cr.Calls = append(cr.Calls, rec); if c.Err == nil && c.ParseErr == nil {
  cr.JudgeCost.Add(costFromUsage(...)) } }}.Func()`. This keeps the existing rule: a failed/unparseable call records
  the `CallRecord` but does not add to `judge_cost`. (`debugWriter` stays an eval variable so existing tests can
  capture it.)
- **`LoadFile(path string) ([]Check, error)`** reads the file (regular file, at most 10 MiB, same limits as other
  check inputs), decodes with `yaml.NewDecoder` + `KnownFields(true)` into `[]Check` (the same strictness as case
  files, so a misspelled key is rejected), and rejects an empty list ("no checks in <file>") so a guardrail that checks
  nothing can never pass. Errors name the file. The file is the value of a case's `checks:` key, i.e. a top-level
  YAML sequence of the documented check mappings; `criterion` is accepted and optional.

### `kairon check`

```
kairon check --checks <file> --dir <dir> [--output <file>] [--gh-log <file>] [--hidden-dir <dir>] [--backend <name>]
```

| Flag | Meaning |
|---|---|
| `--checks` (required) | YAML list of checks (the case `checks` format; `criterion` optional) |
| `--dir` (required) | directory the `file_*`, `changed_files`, `command`, `judge` (`files`) and `fact_*` checks look at; must be an existing directory |
| `--output` | file whose text `output_contains` / `output_not_contains` (and the judge's output) read; omitted = empty text |
| `--gh-log` | file whose text `gh_log_contains` / `gh_log_not_contains` read; capped at 10 MiB (over the cap sets `GHLogOversized`, so gh_log checks fail explicitly, as in eval); omitted = empty |
| `--hidden-dir` | directory `inject` entries are resolved against and copied from; without it a check that uses `inject` is invalid |
| `--backend` (default `kiro-cli`, usage lists `inference.Names()`) | inference backend answering `judge` checks |

Flow (`RunE`; flag errors before `SilenceUsage = true`, everything after is a result, not a usage mistake):

1. Reject unknown `--backend` (`inference.New`) and a non-directory `--dir`; `--output` / `--gh-log` unreadable → error.
2. `checks.LoadFile` → `checks.Validate(file, list, ValidateOptions{HiddenDir: hidden, OptionalCriterion: true})`.
   **Any** error returns before any check runs (AC 5): the message is the `ErrInvalid` text that names the check
   index and type, e.g. `invalid checks: invalid.yaml: check #2 (file_contains): pattern is required`.
3. Only if the list contains a `judge` check: pin the judge model exactly as `kairon eval` does. `config.LoadEvals()`
   (reads `./.kairon/config.yaml`, defaults when missing) → `ev.CheckModel(ev.JudgeModel)`; on refusal return
   `❌ eval model pinning refused the run before any check started:\n  - evals.judge_model: <err>\nEdit the evals
   block in .kairon/config.yaml …` (same wording style as `pinRun`). The request's `Model` is `ev.JudgeModel`. A
   run without judge checks never reads the config and never needs the backend binary (so `pass.yaml` works with the
   default `kiro-cli` backend installed or not).
4. `checks.Evaluate(list, Input{Dir: abs(dir), Output: …, GHLog: …, GHLogOversized: …, Judge: …})`. `Base` is left
   empty (`changed_files` diffs against `HEAD` of the repository at `--dir`, which must be the repository root, as today).
5. Print one line per result to `cmd.OutOrStdout()`, in list order, whitespace collapsed to a single line:
   `PASS <label>` or `FAIL <label>: <detail>` (a passing judge check appends its detail, the judge's reasoning).
   `Label` is the existing `#<n> <type> <key fields>` string, so a failing check is named by index, type and key field.
6. If any check failed return `fmt.Errorf("%d of %d checks failed", failed, total)` → `main` prints `Error: …` to stderr
   and exits 1. All pass → exit 0 and nothing else on stdout (exactly one line per check).

#### Judge in `kairon check` (AC 4)

`InferenceJudge{Backend: backend, Model: ev.JudgeModel, Stub: stubScript}`. `stubScript` is `nil` for every backend
except `stub`, where it is `&inference.StubScript{Judge: inference.StubJudge{"yes"}}`: every judge check is answered
`yes` by the deterministic test double, with no model call. This is documented as a test double (it exists so the
command can be exercised without a model, like `task eval:selftest`); it does not change `internal/inference`. A
`judge` check is never skipped: with a real backend a failed call or unparseable answer fails the check
(`judge call failed: …` / `judge parse error: …`), unchanged from eval.

Security notes (flag in docs, no code change): `command` checks run `sh -c` on the host in `--dir` with the process
environment minus GitHub credential variables (existing behavior). The checks file is trusted input like a case file.
`kairon check` adds no network listener or auth surface.

## Relevant files

Create:
- `internal/checks/` — the moved engine plus `load.go` (`LoadFile`), `judge_infer.go` (`InferenceJudge`, `JudgeCall`),
  `prompt.go` (`BuildYesNoPrompt`, `writeTranscript`), `validate.go` (`ValidateOptions`, `Validate`),
  `env.go` (`GitHubCredentialEnv`, `HermeticGitEnv`) and `doc.go`.
- `internal/checks/deps_test.go` — `go list -deps ./internal/checks` contains neither `internal/eval` nor `internal/eval/sandbox`.
- `internal/eval/checks_alias.go` — aliases and wrappers described above.
- `cmd/kairon/cmd/check.go`, `cmd/kairon/cmd/check_test.go`.
- `internal/eval/testdata/check-cli/` fixtures (below).

Move (`git mv`, then change `package eval` → `package checks` and export what the table lists):
- `internal/eval/checks.go`, `checks_eval.go`, `checks_command.go`, `checks_fact.go`, `checks_git.go`,
  `checks_glob.go`, `checks_inject.go`, `checks_judge.go`, `checks_proc_unix.go`, `checks_proc_other.go` →
  `internal/checks/`.
- Their tests that touch unexported engine internals (`checks_glob_test.go`, `checks_eval_test.go`,
  `checks_eval_cmd_test.go`, `checks_eval_git_test.go`, `checks_eval_inject_test.go`, `checks_fact_test.go`,
  `checks_turn_test.go`, `checks_docs_test.go`, and the engine-only parts of `checks_test.go` and
  `checks_judge_test.go`) → `internal/checks/`. A test file mixing engine and harness code (`loadCases`, `evalsPath`,
  `configure`, `scoreCase`, `CaseResult`) is split: the engine tests move, the harness tests stay in `internal/eval`.
- `buildYesNoPrompt` / `writeTranscript` from `internal/eval/judge_check.go` and `judge_prompt_test.go` → `internal/checks`.
- `blockedContainerEnv` (`runner.go`) and the base of `hermeticGitEnv` (`workspace.go`) → `internal/checks/env.go`.

Modify:
- `internal/eval/judge_check.go` (`newCheckJudge` → wrapper; `judgeCheckTimeout` moves), `runner.go` (drop
  `blockedContainerEnv` literal and `ansiRegex`; `loadCases` keeps calling `ValidateChecks`), `workspace.go`
  (`hermeticGitEnv`), `scoring.go` (may call `checks.Evaluate` directly; `shorten` via wrapper), `simulated_user.go`
  (unchanged if the aliases exist).
- `docs/evaluation.md` — see Task 5.
- Do NOT modify `.kiro/agents/*.json` (they show as modified in the worktree before this task; leave them alone and do not stage them).
- No template-synchronized path changes (`.kiro/agents`, `.kairon/scripts`, `.kairon/themes`, `.kairon/evals/{fixtures,rubrics,cases}`,
  `.kiro/skills/sentinel-protocol` are untouched), so `cmd/kairon/templates/` needs no change; still run `task sync:check`.

### Fixtures: `internal/eval/testdata/check-cli/`

| Path | Content |
|---|---|
| `dir/hello.txt` | `hello world` + newline |
| `dir/sub/data.txt` | `alpha=1` + newline |
| `pass.yaml` | `file_exists path=hello.txt`; `file_contains path=hello.txt pattern='hello'`; `file_absent path=missing.txt`; `command run="grep -q hello hello.txt"` (all `criterion: demo`) |
| `fail.yaml` | `file_exists path=hello.txt` (passes) then `file_exists path=missing.txt` (fails: `#2 file_exists path=missing.txt`) |
| `judge.yaml` | one `judge` check, `question: "Does hello.txt greet the world?"`, `files: [hello.txt]` |
| `invalid.yaml` | check #1 valid (`file_exists`), check #2 `type: file_contains` with `path` but no `pattern` |
| `evidence.yaml` | `output_contains pattern='done'`, `gh_log_contains pattern='issue edit'`, `command run="test -f injected.txt" inject: [injected.txt]` |
| `output.txt` | `all work done` |
| `gh.log` | `gh issue edit 1 --add-label x` |
| `hidden/injected.txt` | `injected` |

Fixture files must not contain absolute paths. `dir/` must stay free of `.eval/` and must not be modified by any
check (the `command` check only reads).

## Team orchestration

Sequential core, one PR:

1. `extract-checks-package` (builder) — the mechanical move plus eval rewiring. Everything else depends on it.
2. `checks-loader-and-fixtures` (builder) — `checks.LoadFile` + fixtures. Needs the package from 1.
3. `check-command` (builder) — the cobra command. Needs 2.
4. `document-check-command` (documenter) — needs the final output format from 3.
5. `validate-all` (validator) — read-only verification of every acceptance criterion, including the
   baseline-vs-after score comparison.

Tasks 2 and 3 are not parallelized: both write `internal/checks`/fixtures that 3's tests consume. TDD (AGENTS.md):
Task 1 is a **refactor** — existing tests are the characterization suite and must pass before and after; Tasks 2 and 3
are **new features** — write the failing test first, confirm it fails for the expected reason (missing function / wrong
exit), then implement. Each task's commit contains its tests and implementation together; the commit message names the
failing test observed first (features) or the tests relied on (refactor).

## Step-by-step task breakdown

### Task 1: `extract-checks-package` (refactor)

1. **Baseline first (before touching code).** On the unmodified tree:
   ```
   go test ./... 2>&1 | tail -n 30                       # record pass/fail baseline
   rm -rf /tmp/kairon-346-base && mkdir -p /tmp/kairon-346-base
   go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
   cp -R internal/eval/testdata/evals/results /tmp/kairon-346-base/selftest
   rm -rf internal/eval/testdata/evals/results
   go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail   # exits 1 by design (9.7% < 95%)
   cp -R internal/eval/testdata/evals/results /tmp/kairon-346-base/selftest-fail
   rm -rf internal/eval/testdata/evals/results
   ```
   (`results/` is git-ignored; delete it afterwards so no run output is left in the tree.) These JSONs hold the
   per-case `scores` (`name`, `score`, `max_score`, `reasoning`, `checks`) to compare against.
2. Add `internal/checks/deps_test.go` first (it fails: the package does not exist / lists eval deps), then create the
   package with `git mv` and the edits in "Exported API". Keep unexported helpers unexported unless the table says
   otherwise. Move tests with their code; split mixed test files.
3. Rewire `internal/eval` with `checks_alias.go`, the `newCheckJudge` wrapper, `hermeticGitEnv`, `blockedContainerEnv`.
4. Make `go build ./... && go vet ./...` and `go test ./internal/checks ./internal/eval/... ./cmd/...` pass. No test is
   deleted or weakened; moved tests keep their assertions.
5. Re-run the two baseline commands into `/tmp/kairon-346-after/…`, compare per-case `scores` with the baseline
   (e.g. `jq -S '[.cases[] | {case_name, scores}]'` on both `selftest.json` / `selftest-fail.json`, ignoring timings, costs and
   run directory names) and require an empty diff. Delete the generated `results/` again.

Acceptance: AC 1, AC 2.

### Task 2: `checks-loader-and-fixtures` (feature)

Failing tests first in `internal/checks/load_test.go`: `LoadFile` on each fixture (pass/fail/judge/evidence parse and
validate; `invalid.yaml` decodes but `Validate` rejects it with `check #2 (file_contains)`), unknown key rejected, empty
list rejected, over-size / non-regular file rejected, `OptionalCriterion` accepts a check with no `criterion` while
`OptionalCriterion: false` still returns `criterion is required`. Then create the fixtures (table above) and
`load.go`.

Acceptance: fixtures exist with the listed content; `LoadFile` + `Validate` behave as specified; engine package
tests pass.

### Task 3: `check-command` (feature)

Failing tests first in `cmd/kairon/cmd/check_test.go` (run the command in-process with `SetOut`/`SetErr`; fixture
path `../../../internal/eval/testdata/check-cli`; reset flag variables in `t.Cleanup`):
- flags registered with the documented defaults (`--backend` default `kiro-cli`, usage lists every `inference.Names()`);
  `--checks` and `--dir` required;
- `pass.yaml` → nil error, stdout has exactly one `PASS` line per check (4);
- `fail.yaml` → non-nil error `1 of 2 checks failed`, stdout has `FAIL #2 file_exists path=missing.txt`;
- `invalid.yaml` → error naming `check #2` and `file_contains`; a command check placed before the invalid check does
  NOT run (use a temp dir and a checks file whose command #1 would `touch ran` — assert the file was not created);
- `judge.yaml` with `--backend stub` → nil error, one `PASS` line, no model call (the stub is the only backend);
  `judge.yaml` with `--backend kiro-cli` is not run in tests (would call a model); instead a unit test of the
  command's judge wiring uses a recording `inference.Backend` to assert `Request.Model == ev.JudgeModel`,
  `Role == RoleJudge`, `YesNo == true`, `Stub == nil`;
- pinning refusal: temp cwd with a `.kairon/config.yaml` whose `evals.judge_model` is not in `allowed_models` → error
  containing `evals.judge_model` and no check runs; a checks file with **no** judge check ignores a broken config;
- `evidence.yaml` with `--output`, `--gh-log`, `--hidden-dir` → all pass; without `--hidden-dir` → invalid
  (inject), without `--output` → `FAIL … output_contains`;
- unknown `--backend` and non-directory `--dir` → error before any check.

Then implement `check.go`, registered on `rootCmd` in `init()`.

Acceptance: AC 3, AC 4, AC 5.

### Task 4: `document-check-command` (docs, no test required per AGENTS.md exemption)

Edit `docs/evaluation.md`:
- Replace the "Using the evaluator elsewhere" Go sample/prose to use `checks.Evaluate`, `checks.Input`,
  `checks.JudgeFunc`, `checks.Validate`, `checks.ErrJudgeParse`, and say the package is `internal/checks` (no
  dependency on the eval harness or sandbox). Keep every `yaml` fence parseable (the docs test unmarshals them).
- Add a section `### Running checks on any directory (\`kairon check\`)` right after it: synopsis, flag table, the
  checks-file format (a top-level list; `criterion` optional) with a `yaml` example, output format
  (`PASS`/`FAIL` lines), exit status (0 only if every check passes; invalid file → non-zero before any check runs;
  zero checks → error), the judge/`--backend`/model-pinning behavior (pinned via `evals.judge_model` /
  `allowed_models`; `--backend stub` answers `yes` with no model call), what `--output`/`--gh-log`/`--hidden-dir`
  supply, the `changed_files` base (`HEAD`), and the safety note that `command` checks run on the host. Use the
  `internal/eval/testdata/check-cli` commands as examples.
- Also update any other sentence in the file that still points at `eval.EvaluateChecks` / `eval.ValidateChecks` /
  `eval.JudgeFunc` / `errJudgeParse` as the home of the engine (`grep -n 'eval\.\(Evaluate\|Validate\|Judge\|Check\)' docs/evaluation.md`).
Do not edit `.kairon/specs/**` or `CHANGELOG.md` (AGENTS.md).

Acceptance: AC 6; `go test ./internal/checks -run Docs` passes.

### Task 5: `validate-all` (read-only)

Run the validation commands below and report each AC as PASS/FAIL with evidence.

## Validation commands

```bash
# AC 1 – neither dependency is linked (both commands must print nothing and exit 0)
go list -deps ./internal/checks | grep -E 'kairon/internal/eval($|/)' ; test $? -eq 1
go test ./internal/checks -run TestNoHarnessDeps -count=1

# build + unit tests
go build ./... && go vet ./...
go test ./internal/checks ./internal/eval/... ./cmd/... -count=1

# AC 2 – eval scores unchanged vs the baseline captured in Task 1 (diff must be empty)
task eval:selftest
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail || true
# compare per-case scores with /tmp/kairon-346-base/{selftest,selftest-fail}; then: rm -rf internal/eval/testdata/evals/results

# AC 3
go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/pass.yaml --dir internal/eval/testdata/check-cli/dir; echo $?   # 0
go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/fail.yaml --dir internal/eval/testdata/check-cli/dir; echo $?   # FAIL #2 file_exists path=missing.txt ... then 1
go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/evidence.yaml --dir internal/eval/testdata/check-cli/dir \
  --output internal/eval/testdata/check-cli/output.txt --gh-log internal/eval/testdata/check-cli/gh.log \
  --hidden-dir internal/eval/testdata/check-cli/hidden; echo $?   # 0

# AC 4 – no model call
go run ./cmd/kairon check --backend stub --checks internal/eval/testdata/check-cli/judge.yaml --dir internal/eval/testdata/check-cli/dir; echo $?   # PASS line, 0

# AC 5
go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/invalid.yaml --dir internal/eval/testdata/check-cli/dir; echo $?   # names check #2 (file_contains), non-zero

# AC 6 + repo gates
grep -n 'kairon check' docs/evaluation.md
task sync:check
task lint
task test
git status --short   # only intended paths; .kiro/agents/*.json untouched by this work; no internal/eval/testdata/evals/results
```

## Risks and notes

- **Mixed test files.** `checks_test.go` (case loading + validation table), `checks_judge_test.go` and
  `selftest_judge*_test.go` mix engine and harness code. Split by what they call; never relax an assertion to make a
  move compile. Tests that swap the package variables `runCheckCommand`, `checkCommandTimeout` or `debugWriter` must live
  in the package that owns the variable.
- **Score stability.** The only behavioral-risk pieces are `newCheckJudge` (call recording and the "no judge_cost on
  failed/unparseable call" rule) and `hermeticGitEnv`. Task 1's baseline comparison and `judge_check_test.go` /
  `pinning_test.go` cover them.
- **Stub judge in the CLI** always answers `yes`; it is test-double behavior selected only by `--backend stub` and is
  documented as such. `internal/inference` is not modified.
- **Out of scope** (issue): new check types, changed check semantics, wiring checks into the watcher / agent runtime.

## Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "extract-checks-package"
    agent: "builder"
    description: "Refactor: capture the pre-change eval baseline (go test, selftest and selftest-fail results), then move the check engine (types, validation, evaluation, command/fact/git/glob/inject/judge support, judge prompt and yes/no parsing) from internal/eval into the new package internal/checks, export the symbols listed in the spec, decouple Validate from Rubric via ValidateOptions, add checks.InferenceJudge, and rewire internal/eval through thin aliases/wrappers so eval behavior and scores are unchanged. Move tests with their code (split mixed test files) and add TestNoHarnessDeps."
    dependencies: []
    acceptance_criteria:
      - "internal/checks exists and contains the moved engine files (checks.go, checks_eval.go, checks_command.go, checks_fact.go, checks_git.go, checks_glob.go, checks_inject.go, checks_judge.go, checks_proc_*.go) moved with git mv"
      - "go list -deps ./internal/checks lists neither github.com/jbrinkman/kairon/internal/eval nor github.com/jbrinkman/kairon/internal/eval/sandbox, enforced by TestNoHarnessDeps"
      - "checks.Validate takes ValidateOptions{HiddenDir, Criterion, OptionalCriterion} and returns errors wrapping checks.ErrInvalid with the unchanged text 'invalid checks: <file>: check #<n> (<type>): <reason>'"
      - "checks.InferenceJudge{Backend, Model, Stub, Debug, OnCall}.Func() reproduces the old newCheckJudge request (RoleJudge, YesNo, 2m timeout, pinned model) and eval's newCheckJudge is a wrapper that still appends one CallRecord per call with the criterion and adds judge_cost only for a successful parse"
      - "internal/eval keeps ValidateChecks/EvaluateChecks wrappers and type aliases, so loadCases and scoreCase are unchanged in behavior; blockedContainerEnv, hermeticGitEnv and stripANSISequences delegate to internal/checks"
      - "No existing test is deleted or has an assertion weakened; go build ./... go vet ./... and go test ./internal/checks ./internal/eval/... ./cmd/... pass"
      - "Per-case scores in selftest.json and selftest-fail.json after the change are identical to the baseline captured before the change; generated results/ directories are removed; .kiro/agents/*.json are not modified"
    validation_commands:
      - "go build ./... && go vet ./..."
      - "go test ./internal/checks ./internal/eval/... ./cmd/... -count=1"
      - "go test ./internal/checks -run TestNoHarnessDeps -count=1"
      - "task eval:selftest"
      - "task sync:check"

  - id: "checks-loader-and-fixtures"
    agent: "builder"
    description: "Feature (test first): add checks.LoadFile (strict YAML list of checks in the case checks format, size-capped, empty list rejected) and Validate's OptionalCriterion option, and create the fixtures under internal/eval/testdata/check-cli/ (pass.yaml, fail.yaml, judge.yaml, invalid.yaml, evidence.yaml, output.txt, gh.log, hidden/injected.txt, dir/hello.txt, dir/sub/data.txt)."
    dependencies: ["extract-checks-package"]
    acceptance_criteria:
      - "internal/checks/load_test.go is written first and observed failing for the expected reason (LoadFile undefined / criterion required) before the implementation"
      - "LoadFile decodes a top-level YAML sequence with KnownFields(true); an unknown key, an empty list, a non-regular file and a file over 10 MiB each return an error naming the file"
      - "Validate with OptionalCriterion accepts a check without criterion; without it the error is still 'criterion is required'"
      - "pass.yaml, judge.yaml, evidence.yaml load and validate; invalid.yaml loads but Validate fails naming 'check #2 (file_contains)'; fail.yaml validates"
      - "All fixture files exist with the content listed in the spec and contain no absolute paths"
    validation_commands:
      - "go test ./internal/checks -count=1"
      - "go vet ./internal/checks"
      - "test -f internal/eval/testdata/check-cli/pass.yaml -a -f internal/eval/testdata/check-cli/fail.yaml -a -f internal/eval/testdata/check-cli/judge.yaml -a -f internal/eval/testdata/check-cli/invalid.yaml -a -f internal/eval/testdata/check-cli/dir/hello.txt"

  - id: "check-command"
    agent: "builder"
    description: "Feature (test first): add the 'kairon check' cobra command (cmd/kairon/cmd/check.go) with --checks, --dir, --output, --gh-log, --hidden-dir and --backend (default kiro-cli). Validate everything before running any check, pin the judge model like kairon eval (only when a judge check exists), run judge checks through checks.InferenceJudge (stub backend gets a built-in yes script, no model call), print one PASS/FAIL line per check and return an error (exit 1) unless every check passes."
    dependencies: ["checks-loader-and-fixtures"]
    acceptance_criteria:
      - "cmd/kairon/cmd/check_test.go is written first and observed failing for the expected reason (check command not registered) before the implementation"
      - "go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/pass.yaml --dir internal/eval/testdata/check-cli/dir exits 0 and prints exactly one PASS line per check"
      - "The same command with fail.yaml exits non-zero and prints a FAIL line naming '#2 file_exists path=missing.txt'"
      - "With invalid.yaml the command exits non-zero before any check runs, with an error naming check #2 and type file_contains (a test proves an earlier command check did not run)"
      - "With --backend stub, judge.yaml passes with no model call; with a recording backend the judge Request has Role judge, YesNo true, Model equal to the configured evals.judge_model and no Stub"
      - "A judge_model outside allowed_models is refused with an 'evals.judge_model' message before any check runs; a checks file without judge checks never reads the evals config"
      - "--output, --gh-log and --hidden-dir feed output_*, gh_log_* and inject checks (evidence.yaml passes with them, fails/invalid without)"
      - "Unknown --backend and a non-directory --dir fail before any check runs; --backend usage lists inference.Names() and defaults to kiro-cli"
    validation_commands:
      - "go test ./cmd/... -run Check -count=1"
      - "go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/pass.yaml --dir internal/eval/testdata/check-cli/dir"
      - "go run ./cmd/kairon check --backend stub --checks internal/eval/testdata/check-cli/judge.yaml --dir internal/eval/testdata/check-cli/dir"
      - "! go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/fail.yaml --dir internal/eval/testdata/check-cli/dir"
      - "! go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/invalid.yaml --dir internal/eval/testdata/check-cli/dir"

  - id: "document-check-command"
    agent: "documenter"
    description: "Update docs/evaluation.md: document 'kairon check' (synopsis, flags, checks-file format, PASS/FAIL output, exit status, judge/--backend/model pinning and the stub test-double, changed_files base, host-execution safety note) and repoint the 'Using the evaluator elsewhere' section and any other eval.EvaluateChecks/ValidateChecks/JudgeFunc/errJudgeParse references to the internal/checks package."
    dependencies: ["check-command"]
    acceptance_criteria:
      - "docs/evaluation.md has a section documenting kairon check with all six flags (--checks, --dir, --output, --gh-log, --hidden-dir, --backend) and the exit-status rules"
      - "'Using the evaluator elsewhere' refers to checks.Evaluate, checks.Input, checks.Validate, checks.JudgeFunc and checks.ErrJudgeParse in internal/checks, and no remaining text says the evaluator lives in internal/eval"
      - "Every yaml fenced block containing a checks list still parses (go test ./internal/checks -run Docs passes)"
      - ".kairon/specs/** and CHANGELOG.md are not edited"
    validation_commands:
      - "grep -n 'kairon check' docs/evaluation.md"
      - "! grep -n 'eval\\.EvaluateChecks\\|eval\\.ValidateChecks\\|eval\\.JudgeFunc' docs/evaluation.md"
      - "go test ./internal/checks -run Docs -count=1"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion of issue #346: dependency hygiene of internal/checks, unchanged eval scores versus the baseline, kairon check behavior on the fixtures (pass, fail, judge with stub, invalid), documentation, and the repo gates."
    dependencies: ["document-check-command"]
    acceptance_criteria:
      - "go list -deps ./internal/checks lists neither internal/eval nor internal/eval/sandbox"
      - "task eval:selftest and the selftest-fail stub run produce per-case scores identical to the baseline captured before the refactor"
      - "kairon check exits 0 for pass.yaml, non-zero naming the failing check for fail.yaml, passes judge.yaml with --backend stub without a model call, and exits non-zero for invalid.yaml before any check runs"
      - "docs/evaluation.md documents kairon check"
      - "task test, task lint and task sync:check pass; .kiro/agents/*.json and .kairon/specs of other issues are unchanged by this work; no results/ directories are left in the tree"
    validation_commands:
      - "go list -deps ./internal/checks"
      - "task eval:selftest"
      - "go run ./cmd/kairon check --checks internal/eval/testdata/check-cli/pass.yaml --dir internal/eval/testdata/check-cli/dir"
      - "go run ./cmd/kairon check --backend stub --checks internal/eval/testdata/check-cli/judge.yaml --dir internal/eval/testdata/check-cli/dir"
      - "task test"
      - "task lint"
      - "task sync:check"
```
