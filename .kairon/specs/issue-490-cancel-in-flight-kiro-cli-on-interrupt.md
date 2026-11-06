# Design Spec: Cancel in-flight kiro-cli calls on SIGINT and SIGTERM

Closes #490

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`cmd/kairon/cmd/eval.go`: create a signal-aware context (`signal.NotifyContext` for `os.Interrupt` and `SIGTERM`) for `kairon eval`" | `signal-context-helper`, `eval-command-wiring` |
| S2 | In Scope: "`cmd/kairon/cmd/check.go`: create the same signal-aware context for `kairon check`" | `signal-context-helper`, `checks-judge-context` |
| S3 | In Scope: "`internal/eval` (`runner.go`, `simulated_user.go`): the agent, judge and simulated-user `Backend.Invoke` calls receive that context instead of `context.Background()`" | `eval-run-context` |
| S4 | In Scope: "`internal/checks` (`judge_infer.go`): the judge `Backend.Invoke` call receives that context instead of `context.Background()`" | `checks-judge-context` |
| S5 | In Scope: "`internal/inference/kirocli.go`: update the `newKiroCLICommand` doc comment to drop the reference to #490 as an open gap" | `inference-doc-comment` |
| S6 | In Scope: "On cancellation, the existing `setProcessGroup` cancel hook kills the `kiro-cli` process group" | `inference-cancel-test`, `eval-command-wiring`, `checks-judge-context` |
| AC1 | "Sending SIGINT to `kairon eval` while a `kiro-cli` call is in flight leaves no `kiro-cli` process or descendant running." | `eval-command-wiring` (`TestEvalCommand_SIGINTKillsKiroCLIGroup`), `validate-all` |
| AC2 | "Sending SIGTERM to `kairon eval` in the same situation leaves no `kiro-cli` process or descendant running." | `eval-command-wiring` (`TestEvalCommand_SIGTERMKillsKiroCLIGroup`), `validate-all` |
| AC3 | "Sending SIGINT to `kairon check` while a judge `kiro-cli` call is in flight leaves no `kiro-cli` process or descendant running." | `checks-judge-context` (`TestCheckCommand_SIGINTKillsKiroCLIGroup`), `validate-all` |
| AC4 | "A test cancels the parent context of an in-flight `kiro-cli` call (a fake binary with a child that holds stdout) and asserts that the call returns within 1.5 seconds and that the child PID no longer exists." | `inference-cancel-test` (`TestKiroCLI_AgentParentCancelKillsChild`, `TestKiroCLI_JudgeParentCancelKillsChild`) |
| AC5 | "No `Invoke(context.Background(), …)` call remains in `internal/eval` or `internal/checks` production code." | `eval-run-context`, `checks-judge-context`, `validate-all` |
| AC6 | "The `newKiroCLICommand` doc comment no longer refers to #490 as an open gap." | `inference-doc-comment`, `validate-all` |
| C1 | Constraint: "Cancellation kills the process group through the existing `setProcessGroup` cancel hook, and `internal/inference/stubcmd_unix.go` does not change, because the issue states the existing hook is the mechanism." | `inference-cancel-test`, `validate-all` |

## Deferred work

none

## Solution Approach

### What is wrong today

- `newKiroCLICommand` (`internal/inference/kirocli.go`) starts every `kiro-cli` call with `exec.CommandContext` and `setProcessGroup` (`internal/inference/stubcmd_unix.go`). `setProcessGroup` sets `Setpgid` and a `Cancel` hook that sends SIGKILL to the whole group (`syscall.Kill(-pid, SIGKILL)`). When the command's context is cancelled, the whole group dies. This is already correct and tested for timeouts (`TestKiroCLI_AgentTimeoutKillsChild`).
- Because of `Setpgid`, a terminal Ctrl-C reaches `kairon` but not `kiro-cli` or its descendants.
- Nothing cancels the context. Four production call sites pass `context.Background()` to `Backend.Invoke`: `internal/eval/runner.go` (agent call in `invokeAgent`, line 725; judge call in `scoreLLMJudge`, line 1650), `internal/eval/simulated_user.go` (routing call in `newRouter`, line 246) and `internal/checks/judge_infer.go` (line 55). The `kairon eval` and `kairon check` commands install no signal handler, so SIGINT used to terminate `kairon` by default and now leaves `kiro-cli` and its descendants running until they next write to a closed pipe.

The fix is a cancellable context at the callers plus one signal handler per command. The kill mechanism is unchanged: `internal/inference/stubcmd_unix.go` (and `stubcmd_other.go`) are not edited.

### Chosen design

1. **One helper creates the signal-aware context.** New `cmd/kairon/cmd/signal.go` with `signalContext(parent context.Context, notice io.Writer) (context.Context, context.CancelFunc)`.
   - `parent == nil` is treated as `context.Background()`. Cobra's `cmd.Context()` is nil when a test calls `RunE` directly.
   - It calls `signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)` (`syscall.SIGTERM` exists on every platform the module builds for).
   - It restores default signal handling after the first signal. A goroutine waits for the context to be done, writes one line to `notice` (stderr) when the cause was a signal and not the caller's normal `stop()`, and calls the `stop` function that `signal.NotifyContext` returned. A second SIGINT or SIGTERM then terminates `kairon` immediately. This matters because two paths do not observe the context: a `command` check (`sh -c`, up to 5 minutes) and a `--sandbox` container agent call (`runAgentInContainer` keeps its own context). Without it the first Ctrl-C would be ignored until those finish.
   - The returned cancel function releases the handler and is idempotent. Callers `defer` it.
   - Notice text: `kairon: signal received, cancelling in-flight calls (send it again to exit immediately)`.
2. **`kairon eval`.** `evalCmd.RunE` calls `signalContext(cmd.Context(), cmd.ErrOrStderr())` after `cmd.SilenceUsage = true` and passes the result to `eval.RunWithOptions` in a new `RunOptions.Context` field. `--list`, `--cleanup` and `diff` do not use it.
3. **`internal/eval` keeps its package-level configuration model.** `internal/eval/config.go` documents that configuration is "kept here rather than threaded through" because `loadCases`, `GetTestCase` and similar have fixed signatures. This design follows that rule instead of adding a `ctx` parameter to dozens of call chains.
   - `RunOptions.Context context.Context` (nil means `context.Background()`, for programmatic callers and tests).
   - `runConfig` gets `ctx context.Context`. `configure` sets it from `opts.Context`. The existing invariant holds: `configure` runs once before a run and `cfg` is only read during it, so parallel readers (`InvestigateParallelExecution`) stay race-free.
   - New `func (c runConfig) callContext() context.Context` returns `c.ctx`, or `context.Background()` when it is nil. It is the one place a nil context becomes `Background`, so no production `Invoke` call is written with `context.Background()`.
   - New `func runInterrupted() error` returns nil while the run context is live and `fmt.Errorf("❌ interrupted: %w", ctx.Err())` once it is done.
   - The three `Backend.Invoke` calls in `internal/eval` (agent in `invokeAgent`, judge in `scoreLLMJudge`, routing in `newRouter`) change to `cfg.callContext()`. The fourth eval-side judge call, the check judge built by `newCheckJudge`, receives the context through `InferenceJudge.Func(ctx)` (item 4). `RunPerformanceInvestigation` reaches `invokeAgent` and so gets the context too.
4. **`internal/checks` takes the context as a parameter, not a struct field.** `InferenceJudge.Func()` becomes `Func(ctx context.Context) JudgeFunc`, and the returned closure calls `j.Backend.Invoke(ctx, req)`. The `JudgeFunc` type (`func(JudgeQuery) (JudgeVerdict, error)`) does not change, so `checks.Evaluate`, `checks.Input` and the roughly 20 test call sites that build a `JudgeFunc` are untouched. A compile error forces every `Func` caller to supply a context, so no caller can silently keep `Background`. Callers: `newCheckJudge` in `internal/eval/judge_check.go` passes `cfg.callContext()`; `runCheckCommand` passes the signal context.
5. **`kairon check`.** `runCheckCommand` calls `signalContext(cmd.Context(), cmd.ErrOrStderr())` and `defer`s the cancel, then passes that context to `judge.Func(ctx)`.
6. **An interrupted `kairon eval` must stop, not keep going.** Installing a handler means SIGINT no longer kills `kairon`. After the first signal every later `Invoke` fails at once (`exec.CommandContext` refuses to start with a done context). Without a stop, the run would finish every remaining case as a fast failure, and `evaluateProgressive` would save each as a completed case. `isTestCaseCompleted` would then make `--resume` skip them, so an interrupt would corrupt the resumable state. Therefore:
   - `executeCaseRepeats` stops starting new repeats once `runInterrupted() != nil`.
   - `evaluateProgressive` checks `runInterrupted()` at the top of each case and again right after `executeCaseRepeats`, before `checkSuiteUnchanged` and before the case is stored or saved. On interrupt it returns the error and does not save the cut-off case. Cases saved earlier stay saved, and `Run` already leaves `.progress` in place on error, so `kairon eval --resume` re-runs the interrupted case from its first repeat (the documented behaviour for any interrupted case).
   - `evaluate` (single-case path) stops its loop on interrupt, and `runSingleTestCase` returns the interrupt error before writing `<agent>.json` and `summary.json`.
   - The process exits non-zero with `❌ interrupted: context canceled`.
7. **`kairon check` needs no stop logic.** It writes nothing to disk. After the first signal, the in-flight judge call is killed and reports `FAIL … judge call failed: …context canceled`. Remaining judge checks fail at once, results print, and the exit status is non-zero. A `command` check in progress runs to its own end unless the user sends the signal a second time (item 1).
8. **Container path.** `runAgentInContainer` and `openAgentContainerSession` keep their own `context.Background()`-derived contexts. They do not call `Backend.Invoke` and are not part of this change. With `--sandbox`, the first signal stops the run at the next case boundary, after the current container call returns. A second signal exits at once.
9. **Doc comment.** `newKiroCLICommand`'s "Behavior change" paragraph is rewritten to say that Ctrl-C reaches `kairon` but not `kiro-cli`, that the `eval` and `check` commands pass a context cancelled by SIGINT/SIGTERM, and that cancelling it kills the group through `setProcessGroup`. The text must contain no `490` (acceptance criterion 6 greps for it).

### Test-driven development

Per AGENTS.md every code change starts with a test that fails for the expected reason, in the same commit as the code.

- A test that needs a new field or signature (`RunOptions.Context`, `Func(ctx)`, `signalContext`) would otherwise fail to compile, which is not the expected reason. The builder first adds the minimal compiling stub with the old behaviour (`callContext()` returns `Background`, `Func(ctx)` ignores `ctx`, `signalContext` returns `parent` and a no-op cancel), runs the new tests, confirms each fails on its assertion, then writes the real code.
- `inference-cancel-test` is a characterization test: `internal/inference` already cancels correctly, so the test passes on first run. The builder proves it can fail by temporarily deleting the `setProcessGroup(cmd)` line in `newKiroCLICommand`, confirming the child-gone assertion fails, and restoring the line with `git checkout -- internal/inference/kirocli.go` before any other edit to that file.
- A test that sends a real signal to the test process registers its own `signal.Notify` guard first. A red run then fails an assertion and cannot kill the `go test` process.
- Tests that send signals or inspect PIDs are in `//go:build unix` files.

## Relevant Files

Modified:
- `internal/inference/kirocli.go` (doc comment only)
- `internal/eval/config.go` (`runConfig.ctx`, `callContext`, `runInterrupted`, `configure`)
- `internal/eval/runner.go` (`RunOptions.Context` if declared here, three-site `Invoke` changes at lines 725 and 1650, interrupt checks in `executeCaseRepeats`, `evaluate`, `evaluateProgressive`, `runSingleTestCase`)
- `internal/eval/simulated_user.go` (line 246; drop the `context` import if it becomes unused)
- `internal/eval/judge_check.go` (`Func(cfg.callContext())`)
- `internal/checks/judge_infer.go` (`Func(ctx)`; `Invoke(ctx, req)`)
- `internal/checks/judge_infer_test.go` (existing `.Func()` calls become `.Func(context.Background())`)
- `cmd/kairon/cmd/eval.go`, `cmd/kairon/cmd/check.go`
- `docs/evaluation.md`, `README.md`

Created:
- `cmd/kairon/cmd/signal.go`, `cmd/kairon/cmd/signal_unix_test.go`, `cmd/kairon/cmd/fakekiro_unix_test.go` (shared fake `kiro-cli` helper)
- `internal/inference/kirocli_cancel_unix_test.go`
- New test files in `internal/eval` and `internal/checks`, and test additions in `cmd/kairon/cmd/check_test.go` or `eval_test.go`, named in the tasks below

Must not change: `internal/inference/stubcmd_unix.go`, `internal/inference/stubcmd_other.go`.

Template sync: no file under a template-synchronized path (see `.kiro/skills/builder-conventions/SKILL.md`) changes. `task sync:check` must still pass and is part of `validate-all`.

## Team Orchestration

```
inference-cancel-test -> inference-doc-comment
signal-context-helper --\
                         +--> checks-judge-context --\
eval-run-context -------+--> eval-command-wiring -----+--> document-interrupt --> validate-all
```

- `inference-cancel-test`, `signal-context-helper` and `eval-run-context` have no dependencies and run in parallel. They touch different packages.
- `inference-doc-comment` waits for `inference-cancel-test` because that task temporarily edits `kirocli.go` for its mutation check and restores it with `git checkout`, which would discard a doc-comment edit made in parallel.
- `checks-judge-context` and `eval-command-wiring` wait for `signal-context-helper` (the helper and the shared fake `kiro-cli` test helper) and `eval-run-context` (`RunOptions.Context`, `callContext`). `checks-judge-context` changes the `Func` signature, so it also edits `internal/eval/judge_check.go`. That is why it follows `eval-run-context`, which must not touch that file.
- `document-interrupt` waits for everything that changes behaviour. `validate-all` runs last and is read-only.

## Step-by-Step Task Breakdown

### Task 1: `inference-cancel-test`
New `internal/inference/kirocli_cancel_unix_test.go`. It reuses `installFakeKiro`, `newKiro`, `readChildPID`, `waitGone` and `timeoutReturnBound` (1.5 s). The fake mode `orphan` starts `sleep 5 &`, which inherits stdout and so holds the pipe, and records its PID in `child.pid`.
- `TestKiroCLI_AgentParentCancelKillsChild` and `TestKiroCLI_JudgeParentCancelKillsChild`: call `Invoke` with a cancellable parent and `Request.Timeout` of one minute, so the request timeout cannot be what ends the call. From a goroutine, wait until `child.pid` exists, then cancel. Measure from the cancel. Assert: the call returns in under `timeoutReturnBound`; the error is non-nil and does not satisfy `errors.Is(err, ErrTimeout)`; `waitGone` confirms the child PID is gone.
- Run the mutation check described under TDD and record the failing assertion in the sentinel.

### Task 2: `inference-doc-comment`
Rewrite the "Behavior change" paragraph of the `newKiroCLICommand` doc comment as described in design item 9. Nothing else in `kirocli.go` changes. `grep -n 490 internal/inference/kirocli.go` must exit 1.

### Task 3: `signal-context-helper`
Add `signalContext` (design item 1), `signal_unix_test.go` and the shared fake-`kiro-cli` test helper. Tests (written first, against the compiling stub):
- `TestSignalContext_SIGINTCancels` and `TestSignalContext_SIGTERMCancels`: install the guard, send the signal to `os.Getpid()`, assert the context is done within 2 s and the notice line was written to the buffer.
- `TestSignalContext_ParentCancelDoesNotNotify`: cancelling the parent cancels the child context and writes no notice.
- `TestSignalContext_CancelIsIdempotentAndQuiet`: calling the returned cancel twice is safe, and a normal release writes no notice.
- `TestSignalContext_NilParent`.
- Shared helper in `fakekiro_unix_test.go`: `installBlockingFakeKiro(t) (readyFile, pidFile string)` puts a `kiro-cli` script first on `PATH`. The script answers `--version` with exit 0, drains stdin, writes its own PID and the PID of a background `sleep 300` child to `pidFile`, touches `readyFile`, and then sleeps. A `t.Cleanup` SIGKILLs both recorded PIDs. A second helper, `waitForFile`, polls up to 5 s. `processAlive` mirrors the one in the inference tests (signal 0).

### Task 4: `eval-run-context`
Design item 3 and design item 6. Tests (in `internal/eval`, new file `run_context_test.go`; a context-recording test backend replaces `cfg.backend`):
- `TestInvokeCallersUseRunContext`: table over the agent call (`invokeAgent` with a nil `ContainerConfig`), `scoreLLMJudge` and the simulated-user router from `newRouter`. Each case runs with `RunOptions{Context: ctx}` where `ctx` carries a value key, and asserts the backend saw a context carrying the value, and that cancelling `ctx` cancels the one the backend saw.
- `TestRunProgressive_InterruptStopsAndKeepsResumable`: a two-case suite where the first agent call cancels the run context and returns an error. Assert: the returned error satisfies `errors.Is(err, context.Canceled)`; the backend was called once; `<agent>.json` in the results directory holds no result for the interrupted case; `.progress` still exists.
- `TestExecuteCaseRepeats_StopsOnInterrupt` (`--repeat 3`, cancel during run 1: one `Invoke`).
- `TestRunSingleTestCase_InterruptWritesNoResult`.
- `TestCallContext_NilIsBackground` (documents the one fallback).
- Existing tests that call `RunWithOptions` without a context keep passing unchanged.

### Task 5: `checks-judge-context`
Design items 4 and 5, plus the `internal/eval/judge_check.go` change. Tests:
- `TestInferenceJudge_PassesContextToBackend` (`internal/checks`, in `judge_infer_test.go` or a new file): a backend that records the `context.Context` it receives; assert the value key set on the caller's context is visible and that cancelling it cancels the backend's context. Update the existing `.Func()` calls to `.Func(context.Background())`.
- `TestCheckJudge_UsesRunContext` (`internal/eval`): `newCheckJudge` hands the backend a context derived from `RunOptions.Context`.
- `TestCheckCommand_SIGINTKillsKiroCLIGroup` (`cmd/kairon/cmd`, unix): a temp directory and a checks file with one judge check, the shared blocking fake `kiro-cli` on `PATH`, `checkBackend` set to `kiro-cli`, `config.LoadEvals` satisfied by a `.kairon/config.yaml` in the temp working directory. Run `runCheckCommand` in a goroutine; after `readyFile` appears, send SIGINT to the test process (with the guard installed). Assert: the command returns within 5 s with a non-nil error, and neither recorded PID is alive. Also assert that the printed result line for the judge check is `FAIL`.
- The file must have zero `Invoke(context.Background()` / `Invoke(context.TODO()` in production code (acceptance criterion 5 grep).

### Task 6: `eval-command-wiring`
Design items 2 and 6 (command side). Tests in `cmd/kairon/cmd/eval_signal_unix_test.go`, reusing the temp-project setup of `evalSelfTestProject` in `eval_test.go` but with `evalBackend` set to `kiro-cli` and the blocking fake `kiro-cli` on `PATH`:
- `TestEvalCommand_SIGINTKillsKiroCLIGroup` and `TestEvalCommand_SIGTERMKillsKiroCLIGroup`: run `evalCmd.RunE(evalCmd, []string{"selftest"})` in a goroutine; once `readyFile` appears, send the signal to the test process (guard installed first). Assert: `RunE` returns within 5 s with an error that satisfies `errors.Is(err, context.Canceled)`; neither the fake `kiro-cli` PID nor the `sleep 300` PID is alive; no agent JSON in the results directory records the interrupted case as completed.
- If the selftest fixtures cannot run with the real `kiro-cli` backend as they stand (model pinning, agent staging), the builder adds the minimum setup in the test file (config.yaml `evals` block, a copied agent config). It must not change the fixtures that ship in templates.

### Task 7: `document-interrupt`
- `docs/evaluation.md`: add a short "Interrupting a run" subsection near the existing `kiro-cli` timeout paragraph (search for "When a native `kiro-cli` call times out"), covering `kairon eval` and `kairon check`. Content: SIGINT or SIGTERM cancels every in-flight `kiro-cli` call and kills its process group; `kairon eval` then stops at once, does not save the interrupted case, leaves `.progress` so `--resume` re-runs that case from its first repeat, and exits non-zero; a second signal exits immediately; `kairon check` prints a `FAIL` line for the cancelled judge check and exits non-zero; a `command` check or a `--sandbox` container call that is already running is not interrupted by the first signal. Platforms without process groups keep the existing caveat.
- `README.md`: one sentence under CLI Usage or the eval section pointing at that subsection.
- No "known gap" or "future work" wording. State what happens.

### Task 8: `validate-all`
Read-only. Runs the gates, the six acceptance-criterion verifications, the constraint check, and a manual real-binary run of acceptance criteria 1 to 3 (see Validation Commands).

## Validation Commands

Run from the repository root:

```bash
go build ./...
go vet ./...
test -z "$(gofmt -l cmd internal)"
go test -race -count=1 ./internal/inference/... ./internal/checks/... ./internal/eval/... ./cmd/...
task sync:check

# Acceptance criterion 4
go test -count=1 ./internal/inference/... -run 'TestKiroCLI_(Agent|Judge)ParentCancelKillsChild' -v
# Acceptance criterion 5 (must exit 1)
grep -rnE 'Invoke\(context\.(Background|TODO)\(\)' internal/eval internal/checks --include='*.go' --exclude='*_test.go'
# Acceptance criterion 6 (must exit 1)
grep -n '490' internal/inference/kirocli.go
# Constraint (must print nothing)
git diff --name-only origin/main | grep -E 'internal/inference/stubcmd_(unix|other)\.go'
```

Acceptance criteria 1 to 3, real binary (validator): build `kairon`; make a temp git repository with a `.kairon/config.yaml` that pins the judge model as `evalSelfTestProject` does; put a fake `kiro-cli` first on `PATH` that answers `--version`, drains stdin, starts `sleep 300 &`, then runs `sleep 300`. Then:
1. `kairon eval selftest --backend kiro-cli --evals-dir <copy of internal/eval/testdata/evals>`, send SIGINT to the `kairon` PID once the fake has started, wait 5 s, run `pgrep -f 'sleep 300'` and `pgrep -x kiro-cli`. Both must exit 1.
2. Repeat with SIGTERM.
3. `kairon check --backend kiro-cli --checks <file with one judge check> --dir <temp dir>`, send SIGINT, wait 5 s, run both `pgrep` commands. Both must exit 1.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "inference-cancel-test"
    agent: "builder"
    description: "Add internal/inference/kirocli_cancel_unix_test.go (build tag unix) with TestKiroCLI_AgentParentCancelKillsChild and TestKiroCLI_JudgeParentCancelKillsChild. Reuse installFakeKiro with mode orphan (sleep 5 child holds stdout), newKiro, readChildPID, waitGone and timeoutReturnBound. Invoke with a cancellable parent context and Request.Timeout of one minute; wait until child.pid exists, cancel, and measure from the cancel. Assert the call returns in under 1.5s, the error is non-nil and is not ErrTimeout, and the child PID is gone. These are characterization tests that pass on first run, so run a mutation check: temporarily delete the setProcessGroup(cmd) line in newKiroCLICommand in internal/inference/kirocli.go, confirm the child-gone assertion fails, and restore with git checkout -- internal/inference/kirocli.go. Do not edit stubcmd_unix.go. Record the failing assertion in the sentinel."
    dependencies: []
    acceptance_criteria:
      - "internal/inference/kirocli_cancel_unix_test.go exists with TestKiroCLI_AgentParentCancelKillsChild and TestKiroCLI_JudgeParentCancelKillsChild"
      - "Each test cancels the parent context of an in-flight kiro-cli call whose fake binary has a child holding stdout, asserts the call returns within 1.5 seconds of the cancel, and asserts the child PID no longer exists"
      - "Each test uses a Request.Timeout of at least one minute and asserts errors.Is(err, ErrTimeout) is false, so only the parent cancellation can end the call"
      - "go test ./internal/inference/... exits 0"
      - "The sentinel records the mutation check: with the setProcessGroup line removed the child-gone assertion failed, and the line was restored"
      - "git diff --name-only shows no change to internal/inference/stubcmd_unix.go or internal/inference/kirocli.go from this task"
    validation_commands:
      - "go vet ./internal/inference/..."
      - "go test -race -count=1 ./internal/inference/... -run 'TestKiroCLI_(Agent|Judge)ParentCancelKillsChild' -v"
      - "go test -race -count=1 ./internal/inference/..."
      - "git diff --quiet HEAD -- internal/inference/stubcmd_unix.go internal/inference/kirocli.go"

  - id: "inference-doc-comment"
    agent: "builder"
    description: "Rewrite the Behavior change paragraph of the newKiroCLICommand doc comment in internal/inference/kirocli.go. State that Setpgid moves kiro-cli out of the terminal foreground process group so Ctrl-C reaches kairon but not kiro-cli, that the kairon eval and kairon check commands pass a context cancelled by SIGINT and SIGTERM to Backend.Invoke, and that cancelling it kills the whole group through the setProcessGroup Cancel hook. Remove the sentences saying SIGINT has no cancellation path today and that the wiring is tracked in an issue, and remove the until-it-lands sentence. The file must contain no occurrence of the digits 490. Change nothing but comment text."
    dependencies: ["inference-cancel-test"]
    acceptance_criteria:
      - "The newKiroCLICommand doc comment no longer refers to #490 or to any issue as an open gap, and describes the current behaviour: SIGINT and SIGTERM cancel the context and setProcessGroup kills the group"
      - "grep -n 490 internal/inference/kirocli.go exits 1"
      - "Only comment lines in internal/inference/kirocli.go change; go build and go vet pass"
      - "internal/inference/stubcmd_unix.go is unchanged"
    validation_commands:
      - "! grep -n 490 internal/inference/kirocli.go"
      - "go build ./... && go vet ./internal/inference/..."
      - "gofmt -l internal/inference | wc -l | grep -qx 0"
      - "git diff --quiet HEAD -- internal/inference/stubcmd_unix.go"

  - id: "signal-context-helper"
    agent: "builder"
    description: "Add cmd/kairon/cmd/signal.go with signalContext(parent context.Context, notice io.Writer) (context.Context, context.CancelFunc). Treat a nil parent as context.Background(). Use signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM). A goroutine waits for the context to be done, writes one notice line to notice only when the cause was a signal and not the caller's release or the parent's cancellation, and then calls the stop function so a second signal gets default handling and terminates the process. The returned cancel releases the handler and is idempotent. Also add the unix test files signal_unix_test.go and fakekiro_unix_test.go. Tests: SIGINT cancels, SIGTERM cancels (each installs its own signal.Notify guard first, then signals os.Getpid, asserts done within 2s and the notice was written), parent cancellation cancels without a notice, double cancel is safe and quiet, nil parent works. fakekiro_unix_test.go provides installBlockingFakeKiro(t) returning readyFile and pidFile (script answers --version with exit 0, drains stdin, writes its own PID and the PID of a background sleep 300 child to pidFile, touches readyFile, then sleeps; Cleanup SIGKILLs both PIDs), waitForFile (polls up to 5s) and processAlive. Write the tests first against a compiling stub (signalContext returns parent and a no-op cancel) and confirm they fail on their assertions."
    dependencies: []
    acceptance_criteria:
      - "signalContext uses signal.NotifyContext with os.Interrupt and syscall.SIGTERM"
      - "After the first signal the handler is released so a second SIGINT or SIGTERM terminates the process with default handling"
      - "A notice line is written to the supplied writer once per signal-caused cancellation and never for a normal release or a parent cancellation"
      - "A nil parent does not panic"
      - "The signal tests install their own signal.Notify guard before sending a signal, so a red run fails an assertion instead of killing the test process"
      - "installBlockingFakeKiro, waitForFile and processAlive exist in a unix-tagged test file for the tasks that depend on this one"
      - "The builder saw each new test fail on an assertion against the stub before writing the implementation, and names the failing tests in the sentinel"
    validation_commands:
      - "go vet ./cmd/..."
      - "go test -race -count=1 ./cmd/kairon/cmd/ -run 'TestSignalContext' -v"
      - "gofmt -l cmd | wc -l | grep -qx 0"

  - id: "eval-run-context"
    agent: "builder"
    description: "Thread a run context through internal/eval using its package-level cfg model. Add RunOptions.Context (nil means Background) and runConfig.ctx set by configure; add runConfig.callContext() (the only place a nil context becomes context.Background) and runInterrupted() returning nil while the context is live and an error wrapping ctx.Err() with the text interrupted once it is done. Change the three Backend.Invoke calls in internal/eval to cfg.callContext(): invokeAgent (runner.go line 725), scoreLLMJudge (runner.go line 1650) and newRouter (simulated_user.go line 246); drop the context import in simulated_user.go if it becomes unused. Stop an interrupted run without corrupting resumable state: executeCaseRepeats stops starting repeats once interrupted; evaluateProgressive checks runInterrupted at the top of each case and right after executeCaseRepeats, before checkSuiteUnchanged and before the case is stored or saved, and returns the error; evaluate stops its loop; runSingleTestCase returns the interrupt error before writing the result files. Do not edit internal/eval/judge_check.go in this task. Tests first in internal/eval/run_context_test.go with a context-recording backend: TestInvokeCallersUseRunContext (agent, scoreLLMJudge and router calls see the run context value and its cancellation), TestRunProgressive_InterruptStopsAndKeepsResumable (two cases, first call cancels; error Is context.Canceled, one backend call, interrupted case not in the saved agent JSON, .progress still present), TestExecuteCaseRepeats_StopsOnInterrupt, TestRunSingleTestCase_InterruptWritesNoResult, TestCallContext_NilIsBackground. Add RunOptions.Context and callContext as a compiling stub returning Background first, and confirm the tests fail on assertions before the real code."
    dependencies: []
    acceptance_criteria:
      - "No Invoke(context.Background(), ...) or Invoke(context.TODO(), ...) remains in internal/eval production code other than through cfg.callContext()"
      - "Agent, judge and simulated-user router Backend.Invoke calls receive a context derived from RunOptions.Context and are cancelled when it is cancelled"
      - "After the run context is cancelled, kairon eval starts no further repeat or case, does not store or save the cut-off case, keeps .progress, and returns an error wrapping context.Canceled"
      - "A single-case run that is interrupted writes neither the agent JSON nor summary.json"
      - "Existing eval tests that call RunWithOptions without a Context pass unchanged"
      - "The builder saw each new test fail on an assertion against the stub before writing the implementation, and names the failing tests in the sentinel"
    validation_commands:
      - "go vet ./internal/eval/..."
      - "go test -race -count=1 ./internal/eval/... -run 'TestInvokeCallersUseRunContext|TestRunProgressive_InterruptStopsAndKeepsResumable|TestExecuteCaseRepeats_StopsOnInterrupt|TestRunSingleTestCase_InterruptWritesNoResult|TestCallContext_NilIsBackground' -v"
      - "go test -race -count=1 ./internal/eval/..."
      - '! grep -rnE "Invoke\(context\.(Background|TODO)\(\)" internal/eval --include="*.go" --exclude="*_test.go"'

  - id: "checks-judge-context"
    agent: "builder"
    description: "Make the judge Backend.Invoke call cancellable and wire kairon check. In internal/checks/judge_infer.go change InferenceJudge.Func() to Func(ctx context.Context) JudgeFunc and call j.Backend.Invoke(ctx, req); leave the JudgeFunc type, checks.Input and checks.Evaluate unchanged. Update the existing .Func() calls in internal/checks/judge_infer_test.go and any other compile errors to pass context.Background(). In internal/eval/judge_check.go call .Func(cfg.callContext()). In cmd/kairon/cmd/check.go call signalContext(cmd.Context(), cmd.ErrOrStderr()) in runCheckCommand, defer the cancel, and pass the context to judge.Func(ctx). Tests first: TestInferenceJudge_PassesContextToBackend in internal/checks (a backend recording the context it gets; the caller's value key is visible and cancelling the caller's context cancels it), TestCheckJudge_UsesRunContext in internal/eval (newCheckJudge passes a context derived from RunOptions.Context), and TestCheckCommand_SIGINTKillsKiroCLIGroup in cmd/kairon/cmd (unix; checks file with one judge check, installBlockingFakeKiro, checkBackend kiro-cli, a .kairon/config.yaml in the temp working directory so config.LoadEvals passes, own signal.Notify guard, run runCheckCommand in a goroutine, after readyFile appears send SIGINT to the test process; assert it returns within 5s with a non-nil error, the judge result line is FAIL, and neither recorded PID is alive). Make Func(ctx) a compiling stub that ignores ctx first and confirm the new tests fail on assertions before the real code."
    dependencies: ["signal-context-helper", "eval-run-context"]
    acceptance_criteria:
      - "InferenceJudge.Func takes a context.Context and the judge Backend.Invoke call receives it; no Invoke(context.Background(), ...) or Invoke(context.TODO(), ...) remains in internal/checks production code"
      - "kairon check creates the signal-aware context with signalContext and cancels the in-flight judge kiro-cli call on SIGINT or SIGTERM, which kills the process group so neither kiro-cli nor its sleep child is left running"
      - "internal/eval/judge_check.go passes cfg.callContext() so eval judge checks are cancelled with the run"
      - "The JudgeFunc type, checks.Input and checks.Evaluate signatures are unchanged, and all existing checks tests pass with only the .Func() call updates"
      - "The builder saw each new test fail on an assertion against the stub before writing the implementation, and names the failing tests in the sentinel"
    validation_commands:
      - "go build ./... && go vet ./internal/checks/... ./internal/eval/... ./cmd/..."
      - "go test -race -count=1 ./internal/checks/... -run 'TestInferenceJudge_PassesContextToBackend' -v"
      - "go test -race -count=1 ./internal/eval/... -run 'TestCheckJudge_UsesRunContext' -v"
      - "go test -race -count=1 ./cmd/kairon/cmd/ -run 'TestCheckCommand_SIGINTKillsKiroCLIGroup' -v"
      - "go test -race -count=1 ./internal/checks/... ./internal/eval/... ./cmd/..."
      - '! grep -rnE "Invoke\(context\.(Background|TODO)\(\)" internal/eval internal/checks --include="*.go" --exclude="*_test.go"'

  - id: "eval-command-wiring"
    agent: "builder"
    description: "Wire kairon eval to the signal-aware context. In cmd/kairon/cmd/eval.go, after cmd.SilenceUsage = true, call signalContext(cmd.Context(), cmd.ErrOrStderr()), defer the cancel, and pass the context in the new eval.RunOptions.Context field to eval.RunWithOptions. Do not use it for diff. Add cmd/kairon/cmd/eval_signal_unix_test.go with TestEvalCommand_SIGINTKillsKiroCLIGroup and TestEvalCommand_SIGTERMKillsKiroCLIGroup. Reuse the temp-project setup of evalSelfTestProject in eval_test.go but set evalBackend to kiro-cli, put the blocking fake kiro-cli (installBlockingFakeKiro) on PATH, and install a signal.Notify guard before signalling. Run evalCmd.RunE(evalCmd, []string{selftest}) in a goroutine, wait for readyFile, send the signal to the test process, and assert RunE returns within 5s with an error satisfying errors.Is(err, context.Canceled), neither the recorded fake kiro-cli PID nor the sleep 300 child is alive, and no saved agent JSON records the interrupted case. If the selftest fixtures need extra setup to run with the real kiro-cli backend (judge_model config block, staged agent config), add the minimum in the test file and do not change fixtures that ship in templates. Write the tests first; with eval.go not yet passing the context they must fail on the assertion that the child PID is still alive or RunE did not return in time."
    dependencies: ["signal-context-helper", "eval-run-context"]
    acceptance_criteria:
      - "evalCmd passes a context cancelled by SIGINT and SIGTERM to eval.RunWithOptions through RunOptions.Context"
      - "Sending SIGINT to a running kairon eval with a kiro-cli call in flight leaves no kiro-cli process or sleep descendant running within 5 seconds (TestEvalCommand_SIGINTKillsKiroCLIGroup)"
      - "Sending SIGTERM in the same situation leaves no kiro-cli process or sleep descendant running within 5 seconds (TestEvalCommand_SIGTERMKillsKiroCLIGroup)"
      - "The interrupted run returns an error wrapping context.Canceled and records no saved result for the interrupted case"
      - "Existing cmd tests that call evalCmd.RunE directly with a nil command context still pass"
      - "The builder saw both new tests fail on assertions before the wiring was added, and names them in the sentinel"
    validation_commands:
      - "go build ./... && go vet ./cmd/..."
      - "go test -race -count=1 ./cmd/kairon/cmd/ -run 'TestEvalCommand_(SIGINT|SIGTERM)KillsKiroCLIGroup' -v"
      - "go test -race -count=1 ./cmd/..."

  - id: "document-interrupt"
    agent: "documenter"
    description: "Document the interrupt behaviour. In docs/evaluation.md add an Interrupting a run subsection near the paragraph that begins When a native kiro-cli call times out. Cover kairon eval and kairon check: SIGINT or SIGTERM cancels every in-flight kiro-cli call and kills its process group; kairon eval then stops at once, does not save the interrupted case, leaves .progress so --resume re-runs that case from its first repeat, and exits non-zero; a second signal exits immediately; kairon check prints a FAIL line for the cancelled judge check and exits non-zero; a command check or a --sandbox container call that is already running is not interrupted by the first signal, so send the signal again to exit at once; platforms without process groups keep the existing caveat. Add one sentence with a link to that subsection in README.md near the kairon eval and kairon check usage. State behaviour only: no known-gap, follow-up or future-work wording."
    dependencies: ["checks-judge-context", "eval-command-wiring", "inference-doc-comment"]
    acceptance_criteria:
      - "docs/evaluation.md has an Interrupting a run subsection that states the behaviours listed in the task description and matches the implemented behaviour"
      - "README.md links to that subsection"
      - "No new text uses the words follow-up, deferred, future work, not yet or known gap"
      - "No template-synchronized file changes, so task sync:check still passes"
    validation_commands:
      - "grep -n 'Interrupting a run' docs/evaluation.md"
      - "grep -n 'evaluation.md#interrupting-a-run' README.md"
      - "task sync:check"

  - id: "validate-all"
    agent: "validator"
    description: "Verify the whole change read-only. Run build, vet, gofmt, the race test run over internal/inference, internal/checks, internal/eval and cmd, and task sync:check. Verify the six acceptance criteria and the constraint exactly as the issue words them: criterion 4 by running the two TestKiroCLI_*ParentCancelKillsChild tests and reading them; criterion 5 and criterion 6 by their grep commands (both must exit 1); the constraint by git diff --name-only origin/main not listing internal/inference/stubcmd_unix.go. Verify criteria 1 to 3 against the real binary: build kairon, create a temp git repository with a config.yaml that pins the judge model as evalSelfTestProject does, put a fake kiro-cli first on PATH that answers --version, drains stdin, starts sleep 300 in the background and then runs sleep 300; run kairon eval selftest --backend kiro-cli --evals-dir with a copy of internal/eval/testdata/evals, send SIGINT to the kairon process once the fake has started, wait 5 seconds, and confirm pgrep -f 'sleep 300' and pgrep -x kiro-cli both exit 1; repeat with SIGTERM; then run kairon check --backend kiro-cli with a checks file containing one judge check, send SIGINT, wait 5 seconds and confirm both pgrep commands exit 1. Confirm in the code that each In Scope item of the Scope Coverage table is delivered and that the PR description can state Deferred work: none."
    dependencies: ["inference-cancel-test", "inference-doc-comment", "signal-context-helper", "eval-run-context", "checks-judge-context", "eval-command-wiring", "document-interrupt"]
    acceptance_criteria:
      - "go build ./..., go vet ./..., gofmt -l cmd internal and task sync:check all pass"
      - "go test -race ./internal/inference/... ./internal/checks/... ./internal/eval/... ./cmd/... exits 0"
      - "Acceptance criterion 1: after SIGINT to kairon eval with a kiro-cli call in flight, pgrep -f 'sleep 300' and pgrep -x kiro-cli both exit 1 five seconds later"
      - "Acceptance criterion 2: the same with SIGTERM"
      - "Acceptance criterion 3: the same with SIGINT sent to kairon check running a judge check"
      - "Acceptance criterion 4: TestKiroCLI_AgentParentCancelKillsChild and TestKiroCLI_JudgeParentCancelKillsChild exist in internal/inference and pass"
      - "Acceptance criterion 5: grep -rnE 'Invoke\\(context\\.(Background|TODO)\\(\\)' internal/eval internal/checks --include='*.go' --exclude='*_test.go' exits 1"
      - "Acceptance criterion 6: grep -n 490 internal/inference/kirocli.go exits 1"
      - "Constraint: git diff --name-only origin/main does not list internal/inference/stubcmd_unix.go"
      - "Every In Scope item in the Scope Coverage table is delivered by the code, and no in-scope work is described as deferred"
    validation_commands:
      - "go build ./... && go vet ./..."
      - "gofmt -l cmd internal | wc -l | grep -qx 0"
      - "go test -race -count=1 ./internal/inference/... ./internal/checks/... ./internal/eval/... ./cmd/..."
      - "task sync:check"
      - "go test -count=1 ./internal/inference/... -run 'TestKiroCLI_(Agent|Judge)ParentCancelKillsChild' -v"
      - '! grep -rnE "Invoke\(context\.(Background|TODO)\(\)" internal/eval internal/checks --include="*.go" --exclude="*_test.go"'
      - "! grep -n 490 internal/inference/kirocli.go"
      - "test -z \"$(git diff --name-only origin/main | grep 'internal/inference/stubcmd_unix.go')\""
```
