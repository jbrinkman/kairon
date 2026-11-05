# Design Spec: Stop kiro-cli calls from hanging past their timeout

Closes #420

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/inference`: the `kiro-cli` invocation paths in `kirocli.go` (the agent path at line 63 and the judge path at line 125)" | `bound-kiro-cli-calls` |
| S2 | In Scope: "`internal/eval`: `TestInvokeAgentDefaultBackendTimeout`" | `write-timeout-tests` |
| S3 | In Scope: "`internal/inference`: a timing assertion in the matching tests in `kirocli_test.go`" | `write-timeout-tests` |
| AC1 | "A timed-out `kiro-cli` call returns within 1.5s when the fake `kiro-cli` leaves a child holding stdout." | `bound-kiro-cli-calls` (`TestInvokeAgentDefaultBackendTimeout`) |
| AC2 | "The test fails if the call blocks past the bound." | `write-timeout-tests` (`TestInvokeAgentDefaultBackendTimeout`) |
| AC3 | "No process from a timed-out call is left running." | `write-timeout-tests` (`TestKiroCLI_AgentTimeoutKillsChild`), `bound-kiro-cli-calls` |
| AC4 | "A timed-out judge-role call (`kiro-cli chat --no-interactive`) also returns within 1.5s when the fake `kiro-cli` leaves a child holding stdout, and leaves no process running." | `write-timeout-tests` (`TestKiroCLI_JudgeTimeoutKillsChild`), `bound-kiro-cli-calls` |
| AC5 | "The gate passes." | `validate-all` |

## Deferred work

none

## Solution Approach

### What is wrong today

`kiroCLIBackend.invokeAgent` (`internal/inference/kirocli.go:63`) and `invokeJudge` (`:125`) build the command with `exec.CommandContext(ctx, "kiro-cli", ...)` and nothing else. When the deadline passes, Go sends SIGKILL to the `kiro-cli` process only. Anything `kiro-cli` started (an MCP server, a shell, a tool process) keeps the stdout and stderr pipe write ends open. `cmd.Wait` waits for the output-copying goroutines to see EOF, so it blocks until that grandchild exits on its own. Because `WaitDelay` is zero, nothing bounds that wait. The grandchild is also never killed.

The eval test shows it. `TestInvokeAgentDefaultBackendTimeout` installs a fake `kiro-cli` whose body is `sleep 5`. `sh` runs `sleep` as a child, `KAIRON_EVAL_TIMEOUT=200ms` kills `sh`, and the call returns after 5.1s. The test passes because it never measures elapsed time.

### Chosen design: reuse the stub backend's two mechanisms

The stub backend (`runOneStubCommand`, `internal/inference/stub.go:230-236`) already solves this with two pieces, and the same package holds both:

1. `setProcessGroup(cmd)` (`stubcmd_unix.go`, build tag `unix`) sets `Setpgid` and replaces `cmd.Cancel` with `syscall.Kill(-pid, SIGKILL)`. The whole tree dies at the deadline, so the pipes close at once and the call returns at about the timeout.
2. `cmd.WaitDelay` bounds `Wait` for anything that escapes the group (a grandchild that calls `setsid`).

Both `kirocli.go` call sites need the same three lines, so add one small constructor in `kirocli.go` and call it from both paths:

```go
// kiroCLIWaitDelay bounds how long Wait lingers for output pipes after the
// call's context is done or the process exits. It is the backstop for a
// descendant that left the process group; setProcessGroup is the primary fix.
const kiroCLIWaitDelay = time.Second

func newKiroCLICommand(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "kiro-cli", args...)
	cmd.WaitDelay = kiroCLIWaitDelay
	setProcessGroup(cmd)
	return cmd
}
```

The worst case is the timeout plus `kiroCLIWaitDelay`. For the issue's 200ms reproduction that is 1.2s, under the 1.5s bound. In the normal case the group kill closes the pipes immediately and the call returns within a few milliseconds of the timeout.

A separate constant is used instead of `stubCommandWaitDelay` because the stub backend is out of scope and its constant is documented as the stub's.

### Non-unix platforms: no new build-tagged file

`setProcessGroup` already has both halves: `stubcmd_unix.go` (`//go:build unix`) and `stubcmd_other.go` (`//go:build !unix`, a no-op). They are in package `inference`, so `kirocli.go` can call `setProcessGroup` on every platform with no change to either file. I confirmed that `GOOS=windows GOARCH=amd64 go build ./internal/inference ./internal/eval` compiles today. On a platform without process groups the call is still bounded by `WaitDelay`, but a descendant of a timed-out call can survive. This matches the stub backend's current behavior on those platforms. The builder must not add a Windows counterpart. The unit tests that prove the kill use `syscall.Kill`, so they live in a `//go:build unix` test file (the existing `kirocli_test.go` is `!windows` and runs `sh` scripts).

### Decision: a normal exit with a lingering pipe holder is not a failure

Adding `WaitDelay` changes one case beyond a timeout. If `kiro-cli` exits with status 0 but a leftover descendant still holds stdout, `cmd.Wait` used to block until the descendant exited. With `WaitDelay` it returns `exec.ErrWaitDelay` after one second. Without handling, `invokeAgent` would turn that into `kiro-cli invocation failed: exec: WaitDelay expired before I/O complete` and the judge path into `kiro-cli chat failed`, which fails a call that finished its work and would trigger a retry. `internal/checks/checks_command.go:98` already treats this case as success.

Both paths therefore treat `errors.Is(runErr, exec.ErrWaitDelay)` as success when `ctx.Err() == nil`. The output captured so far is the output the process wrote before exiting. Neither path kills the group after a normal exit; that is not what the issue asks for and could kill processes a successful run intentionally left behind. When the deadline has expired the existing `ctx.Err() == context.DeadlineExceeded` branch still classifies the failure as `ErrTimeout`, with the same messages as today (`kiro-cli timeout after <d>` for the agent path, `kiro-cli chat failed: <err>` for the judge path).

### Known behavior change to record in a code comment

`Setpgid` moves `kiro-cli` out of the terminal's foreground process group. A Ctrl-C at the terminal reaches `kairon` but no longer reaches `kiro-cli` directly. The eval and checks callers pass `context.Background()`, so there is no cancellation path from SIGINT. `kiro-cli` runs with stdin from a string, `--no-interactive`, and stdout/stderr on pipes, so it does not need the terminal; if `kairon` exits, the next write to a closed pipe ends it. The comment on `newKiroCLICommand` states this. Wiring SIGINT into the callers' contexts is not part of this issue and is not planned here.

### Not changed

- `StartupProbe` (`kiro-cli --version`, `kirocli.go:37`) has no context or timeout and is not an "invocation path" named by the issue.
- Timeout values and how they are configured (`timeoutOrDefault`, `KAIRON_EVAL_TIMEOUT`, `DefaultTimeout`) are untouched. The stub backend is untouched.
- `internal/session/planner.go` and `internal/agent/manager.go` start `kiro-cli` through their own code and are not the paths in the issue.

## Relevant Files

| File | Change |
|------|--------|
| `internal/inference/kirocli.go` | Modify. Add `kiroCLIWaitDelay` and `newKiroCLICommand`; use it in `invokeAgent` and `invokeJudge`; treat `exec.ErrWaitDelay` with a live context as success in both. |
| `internal/inference/kirocli_test.go` | Modify. Add an `orphan` and a `leftover` mode to `fakeScript`; add a timing assertion to `TestKiroCLI_AgentTimeout` and `TestKiroCLI_JudgeTimeout`. |
| `internal/inference/kirocli_timeout_unix_test.go` | Create (`//go:build unix`). PID helpers and the tests that need `syscall.Kill`. |
| `internal/eval/backend_routing_test.go` | Modify `TestInvokeAgentDefaultBackendTimeout`: make the fake leave a child holding stdout and assert elapsed time. |
| `docs/evaluation.md` | Modify. One sentence in "Case Timeout" saying a timed-out `kiro-cli` call kills the call's whole process group and returns within the timeout plus one second. |
| `internal/inference/stubcmd_unix.go`, `stubcmd_other.go`, `stub.go` | Read only. Reused as is. |

No file under `cmd/kairon/templates/` changes, so `task sync:check` is unaffected.

## Test Design (TDD per AGENTS.md)

Constants in the tests: bound `1500 * time.Millisecond`; inference tests use a `300 * time.Millisecond` timeout; the eval test keeps `KAIRON_EVAL_TIMEOUT=200ms`.

### Fake `kiro-cli` modes (add to `fakeScript` in `kirocli_test.go`)

```sh
  orphan)
    sleep 5 &
    echo $! > "$FAKE_KIRO_DIR/child.pid"
    wait
    ;;
  leftover)
    sleep 5 &
    echo $! > "$FAKE_KIRO_DIR/child.pid"
    printf 'ok'
    ;;
```

`sleep 5 &` inherits the script's stdout and stderr, so it holds the pipes. `orphan` blocks in `wait` until the deadline kills it. `leftover` prints and exits 0 while the child keeps running. The existing `sleep` mode uses `exec sleep 5` (no child) and stays as is.

### Tests

| Test | File | What it asserts | Red on `main` because |
|------|------|-----------------|-----------------------|
| `TestInvokeAgentDefaultBackendTimeout` (changed) | `internal/eval/backend_routing_test.go` | Fake body `sleep 5 &` then `wait` (explicit child holding stdout; same observable as the issue's `sleep 5`). Measure `time.Since(start)` around `invokeAgent`; `t.Errorf` if it is 1.5s or more. Keep the existing error text, `ErrorContext.Stderr` prefix and `Environment` assertions. | The call returns after about 5.1s. |
| `TestKiroCLI_AgentTimeout`, `TestKiroCLI_JudgeTimeout` (changed) | `kirocli_test.go` | Add elapsed time below 1.5s around `Invoke`. | These two use `exec sleep` with no child, so they already pass on `main`; the assertion pins the bound and guards a regression. |
| `TestKiroCLI_AgentTimeoutKillsChild` | `kirocli_timeout_unix_test.go` | `orphan` mode, `RoleAgent`, 300ms timeout. Assert `errors.Is(err, ErrTimeout)`, error text `kiro-cli timeout after 300ms`, elapsed below 1.5s, then read `child.pid` and assert the PID is gone. | Elapsed is about 5s. |
| `TestKiroCLI_JudgeTimeoutKillsChild` | `kirocli_timeout_unix_test.go` | Same with `RoleJudge`: `errors.Is(err, ErrTimeout)`, error starts with `kiro-cli chat failed: `, elapsed below 1.5s, child PID gone. | Elapsed is about 5s. |
| `TestKiroCLI_AgentLeftoverChildNotAFailure` | `kirocli_timeout_unix_test.go` | `leftover` mode, `RoleAgent`, default timeout. Assert no error, `resp.Text == "ok"`, `resp.ExitCode == 0`, elapsed below 1.5s. | `Wait` blocks about 5s. |
| `TestKiroCLI_JudgeLeftoverChildNotAFailure` | `kirocli_timeout_unix_test.go` | Same with `RoleJudge`; `resp.Text == "ok"`. | `Wait` blocks about 5s. |

Test rules:

- Use `t.Errorf` for the elapsed check and run the PID check after it, so one run reports both.
- PID helpers, in the `unix` file: `readChildPID(t, recDir)` (fail clearly if `child.pid` is missing or not an integer), `processAlive(pid)` (`syscall.Kill(pid, 0) == nil`) and `waitGone(t, pid)` that polls every 25ms for at most 1s after the call returns. Do not poll longer than 1s: a WaitDelay-only fix returns at about 1.3s with the child alive and the 5s sleep must still be running at the end of the poll, so the PID assertion fails for that wrong fix. This is what makes AC3 independent of the timing assertion.
- Register `t.Cleanup` to `syscall.Kill(pid, SIGKILL)` for every child PID read, so a red run does not leave `sleep` processes. For the `leftover` tests the cleanup is the only thing that stops the child early.
- The checks package has the same pattern (`internal/checks/checks_eval_cmd_test.go`: `processAlive`, `readPID`, `waitDead`); copy the shape, do not import it (it is a test file in another package).
- The `leftover` tests exist because adding `WaitDelay` would otherwise turn a slow success into a failure (see the decision above). They are new behavior, so they are written first and seen failing.
- The escape-the-group path (a descendant that calls `setsid`) is covered by `WaitDelay` alone. `setsid` is not available on macOS, so it has no portable shell test. State this in the sentinel; do not add a non-portable test.
- Do not add `-race`-hostile shared state. `invokeAgent` reads `errOutput.String()` after `Run` returns; Go closes the pipes and waits for the copy goroutines before `Wait` returns, so this is safe under `go test -race`.

### TDD order and commits

1. Write all tests and the fake modes. Run them against unmodified `kirocli.go`. Confirm each "Red on `main`" row fails for the stated reason (elapsed over 1.5s), not for a compile or setup error. Record the failing test names and elapsed times for the commit message.
2. Make the `kirocli.go` change. Re-run until green.
3. Commit tests and implementation together (AGENTS.md: never commit a failing test alone). The commit message names the tests seen failing first.

## Team Orchestration

Tasks run in sequence. Nothing runs in parallel, because each task depends on the previous one's final state:

1. `write-timeout-tests` (builder) writes tests only and records the red run. It leaves the working tree uncommitted.
2. `bound-kiro-cli-calls` (builder) makes the production change, turns the tests green and makes the single commit that holds tests and implementation.
3. `document-timeout-kill` (documenter) edits `docs/evaluation.md`. It waits for the behavior to be final.
4. `validate-all` (validator) runs the whole gate and the cross-compile check, and judges the spec deviation and deferral rules.

The tasks share files (`kirocli_test.go` is touched only by task 1, `kirocli.go` only by task 2), so there is no merge conflict between them.

## Step-by-Step Task Breakdown

### Task 1: `write-timeout-tests`
Add the `orphan` and `leftover` fake modes, the timing assertions, the new unix-only test file, and the eval test change. Run the six tests against the unmodified code and confirm the five that should be red are red for the stated reason.

### Task 2: `bound-kiro-cli-calls`
Add `kiroCLIWaitDelay` and `newKiroCLICommand` to `kirocli.go`, replace both `exec.CommandContext` calls, and treat `exec.ErrWaitDelay` with `ctx.Err() == nil` as success in both paths. Put a comment on `newKiroCLICommand` that cites this spec (`.kairon/specs/issue-420-kiro-cli-timeout-hang.md`, "Chosen design"), explains the Ctrl-C behavior change, and notes that `setProcessGroup` is a no-op on non-unix platforms. Do not edit `stub.go`, `stubcmd_unix.go` or `stubcmd_other.go`. Commit tests and implementation together.

### Task 3: `document-timeout-kill`
Add one sentence to the "Case Timeout" section of `docs/evaluation.md` (after the paragraph that ends "...names both the case `timeout` and `--resource-limit timeout=`.") stating that on a native `kiro-cli` timeout the whole process group is killed and the call returns within one second of the timeout even if a descendant still holds the output pipes. Do not edit `README.md`, `CHANGELOG.md` or anything under `.kairon/specs/`.

### Task 4: `validate-all`
Run `task test`, `task lint`, `task fmt:check`, `task sync:check` and the cross-compile check. Re-run the issue's reproduction and read the reported time. Confirm there are no leftover `sleep 5` processes after the test run, and that the PR description can say `Deferred work: none` and `Spec deviations: none` (or lists real deviations).

## Validation Commands

```bash
# AC1/AC2: the reproduction; expect PASS in well under 1.5s (about 0.2s with the group kill)
go test -run '^TestInvokeAgentDefaultBackendTimeout$' -count=1 -v ./internal/eval

# AC3/AC4 and the timing assertions
go test -race -run 'TestKiroCLI_(Agent|Judge)(Timeout|TimeoutKillsChild|LeftoverChildNotAFailure)$' -count=1 -v ./internal/inference

# Whole packages under the race detector
go test -race -count=1 ./internal/inference ./internal/eval ./internal/checks

# Every platform the package builds for
GOOS=windows GOARCH=amd64 go build ./...
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./...

# AC5, the gate
task test
task lint
task fmt:check
task sync:check
```

## Risks and Notes

- **Ctrl-C:** covered above. `kiro-cli` no longer receives the terminal's SIGINT directly. Impact is limited to a manual Ctrl-C of `kairon eval` while a call is running.
- **Timing flake:** the 1.5s bound is the issue's. The expected time with the group kill is the timeout plus a few milliseconds; the WaitDelay-only worst case is timeout plus 1s, which is 1.3s for the 300ms tests and 1.2s for the eval test, leaving 0.2s to 0.3s of slack. Do not shorten `kiroCLIWaitDelay` below 1s and do not raise the test timeouts above 300ms.
- **Zombie processes in containers:** `syscall.Kill(pid, 0)` reports a zombie as alive. After the group kill the orphaned `sleep` is reparented to init and reaped. A container whose PID 1 never reaps could make `waitGone` fail; the checks package tests carry the same assumption.
- **Spec deviations:** none planned. If the builder departs from this spec, AGENTS.md requires a code comment citing the section and a `Spec deviations` entry in the PR description.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "write-timeout-tests"
    agent: "builder"
    description: "Write the failing tests first. In internal/inference/kirocli_test.go add `orphan` and `leftover` modes to fakeScript (a `sleep 5 &` child that holds stdout/stderr, pid written to $FAKE_KIRO_DIR/child.pid) and add an elapsed-below-1.5s assertion to TestKiroCLI_AgentTimeout and TestKiroCLI_JudgeTimeout. Create internal/inference/kirocli_timeout_unix_test.go (//go:build unix) with readChildPID, processAlive, waitGone (poll 25ms, at most 1s) and t.Cleanup SIGKILL of the child, and the tests TestKiroCLI_AgentTimeoutKillsChild, TestKiroCLI_JudgeTimeoutKillsChild, TestKiroCLI_AgentLeftoverChildNotAFailure and TestKiroCLI_JudgeLeftoverChildNotAFailure as specified in the Test Design section. In internal/eval/backend_routing_test.go change TestInvokeAgentDefaultBackendTimeout so the fake kiro-cli body is `sleep 5 &` then `wait`, and assert elapsed time around invokeAgent is below 1.5s while keeping the existing error, ErrorContext.Stderr and Environment assertions. Do not touch kirocli.go. Run the new and changed tests against the unmodified production code and record which fail and why. Leave the working tree uncommitted for the next task."
    dependencies: []
    acceptance_criteria:
      - "TestInvokeAgentDefaultBackendTimeout asserts elapsed time below 1.5s and fails on the unmodified kirocli.go because the call takes about 5s"
      - "TestKiroCLI_AgentTimeoutKillsChild and TestKiroCLI_JudgeTimeoutKillsChild each start a fake kiro-cli that spawns a child holding stdout, time out after 300ms, assert elapsed below 1.5s and assert the child's PID no longer exists within 1s of the call returning; both fail on the unmodified kirocli.go because elapsed is about 5s"
      - "TestKiroCLI_AgentLeftoverChildNotAFailure and TestKiroCLI_JudgeLeftoverChildNotAFailure assert no error, Text equal to ok, exit code 0 and elapsed below 1.5s, and fail on the unmodified kirocli.go because the call takes about 5s"
      - "TestKiroCLI_AgentTimeout and TestKiroCLI_JudgeTimeout assert elapsed time below 1.5s"
      - "The new PID tests live in a file with the build tag `unix`, and every child PID read is killed in t.Cleanup so a red run leaves no sleep process"
      - "The red run is recorded: each failing test fails on the elapsed-time assertion, not on a compile error or missing fixture"
    validation_commands:
      - "go vet ./internal/inference ./internal/eval"
      - "go test -count=1 -run 'TestKiroCLI_(Agent|Judge)Timeout$' ./internal/inference"
      - "gofmt -l internal/inference internal/eval"

  - id: "bound-kiro-cli-calls"
    agent: "builder"
    description: "In internal/inference/kirocli.go add `const kiroCLIWaitDelay = time.Second` and `newKiroCLICommand(ctx, args)` which builds exec.CommandContext(ctx, \"kiro-cli\", args...), sets cmd.WaitDelay = kiroCLIWaitDelay and calls setProcessGroup(cmd). Use it in invokeAgent and invokeJudge in place of the two exec.CommandContext calls. In both paths treat errors.Is(runErr, exec.ErrWaitDelay) with ctx.Err() == nil as success (no error, exit code 0, output captured so far); keep the existing timeout classification and error messages. Comment newKiroCLICommand with a reference to .kairon/specs/issue-420-kiro-cli-timeout-hang.md, the Ctrl-C behavior change and the no-op setProcessGroup on non-unix platforms. Do not edit stub.go, stubcmd_unix.go or stubcmd_other.go, StartupProbe, timeout values or any configuration. Commit the tests from write-timeout-tests and this change together in one commit whose message names the tests seen failing first."
    dependencies: ["write-timeout-tests"]
    acceptance_criteria:
      - "invokeAgent and invokeJudge both build their command through newKiroCLICommand, which sets WaitDelay to 1s and calls setProcessGroup"
      - "go test -run '^TestInvokeAgentDefaultBackendTimeout$' -count=1 -v ./internal/eval exits 0 and reports well under 1.5s"
      - "TestKiroCLI_AgentTimeoutKillsChild and TestKiroCLI_JudgeTimeoutKillsChild pass: elapsed below 1.5s, ErrTimeout preserved, child PID gone"
      - "TestKiroCLI_AgentLeftoverChildNotAFailure and TestKiroCLI_JudgeLeftoverChildNotAFailure pass"
      - "All other tests in ./internal/inference, ./internal/eval and ./internal/checks still pass under -race, including the unchanged error wording for agent and judge failures"
      - "GOOS=windows GOARCH=amd64 go build ./... succeeds and no new build-tagged production file was added"
      - "The stub backend files and the timeout configuration are unmodified (git diff shows no change to stub.go, stubcmd_unix.go, stubcmd_other.go, inference.go)"
      - "Tests and implementation are in the same commit"
    validation_commands:
      - "go test -race -count=1 ./internal/inference ./internal/eval ./internal/checks"
      - "go test -run '^TestInvokeAgentDefaultBackendTimeout$' -count=1 -v ./internal/eval"
      - "GOOS=windows GOARCH=amd64 go build ./..."
      - "go vet ./..."
      - "gofmt -l internal/inference internal/eval"

  - id: "document-timeout-kill"
    agent: "documenter"
    description: "In docs/evaluation.md, section 'Case Timeout', add one sentence after the paragraph that ends with 'names both the case `timeout` and `--resource-limit timeout=`.' saying that on a native kiro-cli timeout Kairon kills the call's whole process group (on platforms with process groups) and the call returns within one second of the timeout even if a descendant still holds the output pipes. Edit no other file."
    dependencies: ["bound-kiro-cli-calls"]
    acceptance_criteria:
      - "docs/evaluation.md Case Timeout section states that a timed-out native kiro-cli call kills the whole process group and returns within the timeout plus one second"
      - "The sentence does not mention a deferred or future change and cites no issue number it does not own"
      - "No other file is modified by this task"
    validation_commands:
      - "git diff --stat -- docs/evaluation.md"
      - "grep -n 'process group' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of the whole change against every acceptance criterion and the AGENTS.md rules. Run the issue's reproduction and read the reported time, run the inference timeout tests, the cross-compile checks and the full gate. Confirm no sleep process is left running by the tests, the stub backend and timeout configuration are untouched, tests and implementation share a commit, and the PR can state Deferred work: none."
    dependencies: ["bound-kiro-cli-calls", "document-timeout-kill"]
    acceptance_criteria:
      - "AC1: the reproduction test exits 0 and reports under 1.5s"
      - "AC2: TestInvokeAgentDefaultBackendTimeout asserts elapsed time below 1.5s and the assertion is present in the test source"
      - "AC3: an internal/inference test spawns a child through the fake kiro-cli, times out the agent role and asserts the child PID no longer exists"
      - "AC4: an internal/inference test does the same for the judge role (kiro-cli chat --no-interactive) with elapsed below 1.5s and the child PID gone"
      - "AC5: task test exits 0, and task lint, task fmt:check and task sync:check also exit 0"
      - "GOOS=windows GOARCH=amd64 go build ./... and GOOS=linux GOARCH=arm64 go build ./... succeed"
      - "No spec deviation is undocumented and no deferred work is mentioned"
    validation_commands:
      - "go test -run '^TestInvokeAgentDefaultBackendTimeout$' -count=1 -v ./internal/eval"
      - "go test -race -run 'TestKiroCLI_(Agent|Judge)(Timeout|TimeoutKillsChild|LeftoverChildNotAFailure)$' -count=1 -v ./internal/inference"
      - "GOOS=windows GOARCH=amd64 go build ./..."
      - "GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
```
