# Design Spec: Evals — send the prompt and route the backend in the container sandbox

Closes #296

## 1. Problem (verified in code)

`internal/eval/runner.go` has two agent paths with nothing in common:

| | Native (`invokeAgentViaBackend`) | Container (`invokeAgentInContainer`) |
|---|---|---|
| Request | builds `inference.Request` (agent, prompt, timeout, `Model` = `cfg.pins.agentModel`, `Stub`) | none; takes `(agent, prompt)` and never uses `prompt` except for the cost guess |
| Backend | `cfg.backend.Invoke` (kiro-cli or stub) | hard-coded `kiro-cli chat --agent <a> --no-interactive --trust-all-tools` |
| Prompt delivery | stdin | **nowhere** — not on argv, no stdin attached. `Container.ExecWithOutput` only attaches stdout/stderr. Every `--sandbox` run records empty output. |
| `--model` | appended when `req.Model != ""` | never |
| Cost / record | `costFromUsage(resp.Model, resp.Usage)`, `newCallRecord` | `estimateCost(prompt, result)` (chars/4, `Model:""`), and `invokeAgent` hand-builds a `CallRecord` with `Estimated: true` |
| ErrorContext | command, cwd, stderr, exit code | container id/image/platform |

In addition, `RunWithOptions` rejects `--sandbox` for every backend except `kiro-cli`, so `task eval:selftest` (stub) can never run hermetically in a container, and `pinRun(container=true)` ignores `evals.agent_model` and looks for the agent config only under `.kiro/agents` (because the container used to build its own command).

## 2. Solution Approach

Make the container a **transport**, not a second implementation. One request/response pipeline serves both paths; the only thing that differs is *where the backend process runs*.

```
invokeAgent(agent, prompt, cConfig, stub)
  req  := newAgentRequest(agent, prompt, stub)          // shared by native + container (extracted)
  resp, err, baseEC := cConfig == nil ? cfg.backend.Invoke(req)            // native
                                       : runAgentInContainer(req, cConfig) // container
  return completeAgentCall(req, resp, err, wall, baseEC)  // shared: costFromUsage, newCallRecord, ErrorContext
```

Decisions:

1. **Shared request + shared completion.** Extract the body of `invokeAgentViaBackend` into `newAgentRequest` (timeout, `AgentConfigDir`, `Model`, `Stub`) and `completeAgentCall` (`costFromUsage(resp.Model, resp.Usage)`, `newCallRecord`, `PromptSHA256`, ErrorContext, timeout wording). Both paths call them, so output, `model`, `cost`, `usage_source` and the call record are identical **by construction** (AC 3, 4 and the parity constraint). `invokeAgent`'s hand-built `CallRecord{Estimated:true,...}` and `estimateCost` are deleted.
2. **Prompt on stdin, always.** `sandbox.Container` gets `ExecWithStdin(ctx, cmd, stdin io.Reader) (ExecResult, error)`: it sets `AttachStdin`, copies stdin to the hijacked connection in a goroutine (concurrent with `stdcopy`, so a large prompt cannot deadlock against output), then `CloseWrite()`. argv carries only flags. Arg-length and shell-quoting limits cannot apply. `ExecWithOutput` becomes a thin wrapper with unchanged behaviour (trimmed stdout, `command failed with exit code N: <stderr>`).
3. **Backend routing by name, inside the container.**
   - `kiro-cli`: exec `kiro-cli` directly in the container with the argv built by the *same* function the native backend uses (`inference.KiroCLIAgentCommand(req)` → `chat --agent <a> --no-interactive --trust-all-tools [--model <m>]`) and decode the result with the *same* function (`inference.KiroCLIAgentResponse`: ANSI strip, `Model = req.Model`, `EstimateUsage`). `kiro-cli`'s native `invokeAgent` is refactored to use these two helpers, so there is a single definition of the command and of response parsing. No new binary is needed in the container, so existing real-model users are unaffected.
   - every other backend (today: `stub`): run `inference.Backend.Invoke` **in the container** through a hidden helper subcommand `kairon inference-exec --backend <name>`: it reads one JSON `inference.Request` on stdin, runs the named backend in-process, and writes one JSON envelope (`Response` + `Error` + `Timeout`) on stdout. The host decodes it back into `(Response, error)`, re-wrapping `inference.ErrTimeout`. The stub therefore genuinely executes inside the container, reading its script from the request (the case's `stub.turns`), and the wire format is generic enough for the future direct-API backends (E-series) without further sandbox changes.
4. **The helper binary.** The container image is Alpine + kiro-cli and has no `kairon`. A new `sandbox.ResolveLinuxBinary(platform)` returns a static linux binary for the container's platform, memoised per run: (a) `KAIRON_SANDBOX_BINARY` if set; (b) the running executable when `GOOS=linux` and `GOARCH` matches the platform; (c) otherwise `CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build -trimpath ./cmd/kairon` from the kairon module root (found by walking up from the cwd for a `go.mod` declaring `module github.com/jbrinkman/kairon`) into `os.UserCacheDir()/kairon/sandbox/`. If none works the error says exactly that and names `KAIRON_SANDBOX_BINARY`. It is only needed for non-kiro-cli backends, i.e. the self-test, which is run from a source checkout. It is copied with the existing `Container.CopyTo` to `/tmp/kairon` (writable by the non-root `sandbox` user; `/usr/local/bin` is not).
5. **Model threading.** `req.Model = cfg.pins.agentModel(agent)` — the same value the native path sends. `pinRun` stops treating the container specially for *model* selection: because `--model` is now passed, `evals.agent_model` **is honoured** under `--sandbox`, the "ignored" warning is removed, and the pin is the effective model. When `evals.agent_model` is unset the pin is the agent config's `model` (never empty for a real run, which is what AC 3 means by "the backend's default"); an *unpinned* request (unit tests, `cfg.pins == nil`) records exactly what the native path records, because the record is built by the same `newCallRecord`.
6. **Agent-config overlay.** `<evals-dir>/agents/` is still not copied into a *kiro-cli* container (workspace copy-in is the next issue), so provenance for kiro-cli+sandbox keeps ignoring the overlay (`resolveAgentProvenance(name, ignoreOverlay=true)`). For the stub backend nothing reads an agent config in the container, so overlay-based provenance applies exactly as natively — this is what lets `selftest` (whose config lives only in `testdata/evals/agents/`) pass the pre-flight under `--sandbox`. Signature change: `pinRun(agent, opts, ignoreOverlay bool)`, called with `willContainerize(...) && cfg.backend.Name() == inference.NameKiroCLI`.
7. **Guard removed.** The "`--backend X` cannot be combined with `--sandbox`" check in `RunWithOptions` is deleted; any registered backend can now run in the container. The existing test that asserts the rejection is replaced by one asserting `--backend stub --sandbox` is accepted (it reaches the daemon check, not a backend error).
8. **Container setup is backend-aware.** `ValidateKiroCLI` and GitHub mocking are only executed for the kiro-cli backend (they exist solely for it). The stub path copies the helper and runs it, making it fast and independent of the `kiro-cli` install. (The image build itself still installs kiro-cli and therefore needs network at build time; this is documented, not changed.)
9. **Debug logging (AC 3).** With `cConfig.Debug`, the container invocation prints `🔧 Debug: container invoke backend=<b> agent=<a> model=<req.Model>` before exec. Timeout for the exec is `cConfig.ResourceLimits.Timeout`, also written into `req.Timeout` so the in-container backend honours it.
10. **Self-test target (AC 5).** `task eval:selftest:sandbox` runs a gated Go test `TestSelftestSandbox` (`internal/eval`) with `KAIRON_EVAL_SANDBOX_SELFTEST=1`. The test uses `sandbox.EnsureContainerDaemon()` and `t.Skip`s with the #290-style message (names Podman and Docker) when no daemon is reachable, and skips with a "set KAIRON_EVAL_SANDBOX_SELFTEST=1 / run `task eval:selftest:sandbox`" message when the gate is unset, so `task test` never builds images by accident. It runs `selftest` natively and under `--sandbox --backend stub` and compares `actual_output`, `agent_cost` and the recorded call model between the two result files. The direct documented invocation `go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest` also works.

### Out of scope (unchanged)
Per-case workspace copy-in / artifact copy-out, fake `gh`, network policy, read-only FS, tool trust, `summary.json` provenance parity, new check types/scoring.

## 3. Relevant Files

Modify:
- `internal/inference/inference.go` — `json` tags on `Request` / `Response` / `Usage` so they round-trip over the wire. Tags keep the exact Go field names as keys (`Role`, `Agent`, `Prompt`, `Model`, `Stub`, …; `StubScript` keeps its existing lowercase `turns`), and `Duration` stays integer nanoseconds.
- `internal/inference/kirocli.go` — use the extracted `KiroCLIAgentCommand` / `KiroCLIAgentResponse` helpers (behaviour byte-identical; existing `kirocli_test.go` must stay green).
- `internal/eval/runner.go` — `RunWithOptions` guard removal, `pinRun` call, `invokeAgent`, `invokeAgentViaBackend` (→ `newAgentRequest` + `completeAgentCall`), `invokeAgentInContainer` (→ `runAgentInContainer` returning `inference.Response`), delete `estimateCost`, debug log. `createContainerConfig` needs no behavioural change (`KIRO_CLI_DISABLE_TELEMETRY` env stays); only add a doc comment noting the backend is chosen by `cfg.backend`.
- `internal/eval/provenance.go` — `pinRun` / `resolveAgentProvenance` parameter semantics (`ignoreOverlay`), honour `evals.agent_model` in container, drop the warning.
- `internal/eval/sandbox/container.go` — `ExecResult`, `ExecWithStdin`, `ExecWithOutput` as wrapper, ctx-cancel closes the hijacked stream.
- `internal/eval/config_test.go`, `internal/eval/pinning_test.go`, `internal/eval/backend_routing_test.go` — update tests that reference `estimateCost`, the sandbox warning/`agent_model` ignore, and the stub+sandbox rejection.
- `internal/eval/testdata/evals/cases/selftest/` — add `stub-quoted-input.yaml` (input/setup with quotes, newlines, `$(...)`, backticks) and update any test that counts self-test cases.
- `Taskfile.yml` — `eval:selftest:sandbox`.
- `docs/evaluation.md` — see task `update-docs`.

Create:
- `internal/inference/exec.go` (+ `exec_test.go`) — `KiroCLIAgentCommand`, `KiroCLIAgentResponse`, `ExecEnvelope`, `ServeExec`, `DecodeExecResult`.
- `cmd/kairon/cmd/inference_exec.go` (+ test) — hidden `inference-exec` command (`Hidden: true`, nothing written to stdout except the envelope).
- `internal/eval/sandbox/linuxbin.go` (+ test) — `ResolveLinuxBinary`.
- `internal/eval/container_agent_test.go` — unit tests with a fake container executor.
- `internal/eval/selftest_sandbox_test.go` — gated `TestSelftestSandbox`.

Not touched: `cmd/kairon/templates/` and the template-synchronized live paths (agents, scripts, themes, `.kairon/evals/**`, sentinel skill). The self-test fixtures live under `internal/eval/testdata/`, which is not template-synced; `task sync:check` must still pass unchanged.

Read-only references: `internal/inference/stub.go`, `internal/eval/types.go` (`costFromUsage`, `ErrorContext`, `ContainerConfig`), `internal/eval/sandbox/daemon.go` (`EnsureContainerDaemon`), `internal/eval/sandbox/container_daemon_test.go` (#290 skip pattern).

## 4. Key Interfaces

```go
// internal/inference/exec.go
func KiroCLIAgentCommand(req Request) (args []string, command string)
func KiroCLIAgentResponse(req Request, stdout, stderr string, exitCode int, d time.Duration) Response

type ExecEnvelope struct {
    Response Response `json:"response"`
    Error    string   `json:"error,omitempty"`
    Timeout  bool     `json:"timeout,omitempty"` // errors.Is(err, ErrTimeout) on the far side
}
// ServeExec reads one JSON Request from r, runs backend `name`, writes one ExecEnvelope to w.
// A backend error is reported in the envelope (nil return); only I/O / decode / unknown-backend errors are returned.
func ServeExec(ctx context.Context, name string, r io.Reader, w io.Writer) error
// DecodeExecResult turns the helper's stdout back into (Response, error); Timeout wraps ErrTimeout.
func DecodeExecResult(stdout []byte) (Response, error)

// internal/eval/sandbox/container.go
type ExecResult struct { Stdout, Stderr string; ExitCode int } // untrimmed
func (c *Container) ExecWithStdin(ctx context.Context, cmd []string, stdin io.Reader) (ExecResult, error)
// error only for transport/ctx failures; a non-zero exit is reported via ExitCode.

// internal/eval/runner.go (seam so the container path is unit-testable without a daemon)
type agentExecer interface {
    CopyTo(ctx context.Context, destPath, srcPath string) error
    ExecWithStdin(ctx context.Context, cmd []string, stdin io.Reader) (sandbox.ExecResult, error)
}
func runAgentInContainer(ctx context.Context, x agentExecer, req inference.Request, cConfig *ContainerConfig) (inference.Response, *ErrorContext, error)
```

In-container commands:
- kiro-cli: `kiro-cli chat --agent <a> --no-interactive --trust-all-tools [--model <m>]`, stdin = `req.Prompt`.
- other backends: `/tmp/kairon inference-exec --backend <name>`, stdin = JSON(`req`).

Error mapping in `runAgentInContainer` keeps the existing user-facing messages (timeout hint `--resource-limit timeout=`, OOM hint, image-pull hint) and returns `inference.ErrTimeout`-wrapped errors on deadline so `completeAgentCall` prints the same timeout text as native. A non-zero kiro-cli exit becomes `kiro-cli invocation failed: exit status N` plus `resp.Stderr`/`ExitCode`, matching the native backend.

## 5. Team Orchestration

```
inference-wire ──┬─> inference-exec-cmd ─┐
                 │                       ├─> selftest-sandbox ─> update-docs ─> validate-all
sandbox-exec ────┴─> runner-container-path ┘
```

- `inference-wire` and `sandbox-exec` are independent (different packages) and run in parallel.
- `inference-exec-cmd` needs only `inference-wire`; `runner-container-path` needs both `inference-wire` and `sandbox-exec`. These two can run in parallel.
- `selftest-sandbox` needs the command, the runner and the sandbox primitives.
- Docs after behaviour is final; validator last (read-only).

All work lands in one PR; no deferred acceptance criteria.

## 6. Step-by-Step Task Breakdown

### Task 1 — `inference-wire` (builder)
Add `internal/inference/exec.go` with the helpers/envelope in §4; add `json` tags to `Request`, `Response`, `Usage` that keep the Go field names as the JSON keys (the `inference-exec-cmd` validation command depends on `Role`/`Prompt`/`Stub`); refactor `kiroCLIBackend.invokeAgent` to call `KiroCLIAgentCommand` / `KiroCLIAgentResponse`. Tests: command string/argv with and without model; response ANSI-strip, `Model` echo, estimated usage; `ServeExec` with the stub (scripted text, model, reported usage, `Stub` round-trips through JSON), with a failing backend call (error in envelope), unknown backend (returned error), judge role; `DecodeExecResult` timeout wraps `ErrTimeout`; existing `kirocli_test.go` unchanged and green.

### Task 2 — `sandbox-exec` (builder)
In `container.go` add `ExecResult` and `ExecWithStdin`; reimplement `ExecWithOutput` on it with identical semantics; close the hijacked connection when `ctx` is done so the timeout actually ends the exec. Add `linuxbin.go` with `ResolveLinuxBinary(platform string) (string, error)` (memoised, env override, same-OS executable, cross-compile fallback, actionable error); factor the decision logic into a pure function that is unit-tested without Go or a daemon (override wins; matching linux exe; mismatch → build path; no module root → error naming `KAIRON_SANDBOX_BINARY`). Add a daemon-gated test (`skipIfNoContainerDaemon`) that execs `cat` in `alpine:3.19` with stdin containing a >1 MiB payload with quotes/newlines and gets it back byte-identical, and that a non-zero exit is reported via `ExitCode`.

### Task 3 — `inference-exec-cmd` (builder, after Task 1)
Add hidden cobra command `inference-exec --backend <name>` in `cmd/kairon/cmd/inference_exec.go` calling `inference.ServeExec(ctx, name, os.Stdin, os.Stdout)`; no banner/log output on stdout (verify `logging.Initialize` and `PersistentPreRunE` write nothing to stdout; diagnostics to stderr only). It must not appear in `kairon --help`. Test by piping a stub `Request` JSON through the command and decoding the envelope.

### Task 4 — `runner-container-path` (builder, after Tasks 1 and 2)
Implement §2 items 1, 3, 5–9 in `runner.go` / `provenance.go`: extract `newAgentRequest` / `completeAgentCall`; rewrite `invokeAgent` container branch and `invokeAgentInContainer` around `runAgentInContainer` + `agentExecer`; kiro-cli direct vs. helper routing by `cfg.backend.Name()`; copy the helper from `ResolveLinuxBinary(cConfig.Platform)` to `/tmp/kairon`; skip `ValidateKiroCLI`/GitHub mocking for non-kiro-cli; debug line; delete the backend guard and `estimateCost`; `pinRun(…, ignoreOverlay)` with `evals.agent_model` honoured and no warning. Update the affected existing tests. New `container_agent_test.go` with a fake `agentExecer` (for the helper path the fake decodes the stdin JSON and calls `inference.ServeExec`, i.e. the real stub): assert (a) prompt appears on stdin and **not** in any argv element, including a 1 MiB prompt with quotes/newlines/`$()`; (b) kiro-cli argv ends with `--model <m>` when the request has a model and has no `--model` when it has none; (c) stub container run returns the scripted text, and `invokeAgent` yields the same `output`, `CostInfo` (incl. `Model`, `UsageSource`) and `CallRecord` as the native stub run for both self-test cases (reported-usage case and estimated case); (d) error and timeout mapping; (e) `go test -run . ./internal/eval -count=1` has no remaining reference to `estimateCost`.

### Task 5 — `selftest-sandbox` (builder, after Tasks 3 and 4)
Add `stub-quoted-input.yaml` self-test case (and fix any case-count assertions), `TestSelftestSandbox` (gated + daemon-skip + native/sandbox parity as in §2 item 10; `t.Chdir` to repo root; results dirs cleaned with the existing `registerResultsCleanup`), and the `eval:selftest:sandbox` Taskfile target (`KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run '^TestSelftestSandbox$' -count=1 -v`) whose description states it needs Podman or Docker and skips otherwise. If a daemon is available, run it and the direct `go run … --sandbox` command and record the result; if not, record that the container run could not be executed here.

### Task 6 — `update-docs` (documenter, after Task 5)
`docs/evaluation.md`: remove the "`--backend stub` cannot be combined with `--sandbox`" note and the "always estimated / does not honor `evals.agent_model`" note; rewrite the "`--sandbox` and `evals.agent_model`" section (model now passed as `--model`, `evals.agent_model` honoured, overlay still ignored for kiro-cli); update the sandbox model-pinning note and the sandbox provenance bullets (call record now built like native, cost via reported/estimated usage); document how each backend runs in the container (kiro-cli direct, stub via `kairon inference-exec` with a linux binary, `KAIRON_SANDBOX_BINARY`, cross-compile requirements, image build needs network); document stdin delivery; document `task eval:selftest:sandbox` and its skip behaviour next to the existing self-test section.

### Task 7 — `validate-all` (validator, after Task 6)
Read-only verification of every acceptance criterion, `task test`, `task lint`, `task fmt:check`, `task sync:check`.

## 7. Acceptance-Criteria Traceability

| Issue AC | Satisfied by |
|---|---|
| 1 prompt on stdin, non-empty, matches native | `ExecWithStdin` + `runAgentInContainer` (T2, T4); unit test (a); `TestSelftestSandbox` output equality (T5) |
| 2 same backend abstraction, stub in container, `--backend stub --sandbox` exits 0 | guard removal, helper routing (T3, T4); T5 run |
| 3 model threaded; debug log; default not empty | `req.Model` from pins, `--model` via `KiroCLIAgentCommand`, debug line, pin-time default = agent config model (T4); unit test (b) |
| 4 cost via `costFromUsage`, no `estimateCost` | `completeAgentCall` shared, `estimateCost` deleted (T4); parity test (c) |
| 5 `task eval:selftest:sandbox`, skip w/o daemon | Taskfile target + gated test (T5) |
| Keep templates in sync | no template-synced file changes; `task sync:check` in T7 |

## 8. Risks and Notes

- **No daemon check in CI.** Unit tests cover the container path through the `agentExecer` fake (real stub code, real JSON wire); only `ExecWithStdin` and the end-to-end self-test need a daemon and are skipped without one (#290 pattern). This session could not run a container (spec written without executing one).
- **Cross-compile availability.** The stub-in-container path needs a linux binary. On macOS this means `go` plus the kairon source tree (true for `task eval:selftest:sandbox`), or `KAIRON_SANDBOX_BINARY`. The kiro-cli backend needs neither. If `go build` with `CGO_ENABLED=0` fails because of a cgo dependency, the builder must report it rather than weaken the static-binary requirement.
- **Hidden command** is an internal protocol between the host harness and its own binary; it is intentionally undocumented in `--help` and documented only in `docs/evaluation.md`'s sandbox section.
- **Behaviour change:** `--sandbox` now honours `evals.agent_model` (previously warned and ignored). Called out in docs; it follows directly from AC 3.
- **Interpretation of AC 3 "no configured model records the backend's default":** the eval pre-flight pins every in-scope agent (to `evals.agent_model` or the agent config's `model`), so real runs never send an empty model; the container path records whatever the native path records for the same request.

## 9. Validation Commands

```bash
go build ./... && go vet ./...
go test ./internal/inference/... ./internal/eval/... ./cmd/... -count=1
task test && task lint && task fmt:check && task sync:check
go run ./cmd/kairon plan parse .kairon/specs/issue-296-eval-sandbox-prompt-and-backend-routing.md
# Needs Podman or Docker (+ network for the image build):
task eval:selftest:sandbox
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest
# Native baseline still passes:
task eval:selftest
```

## 10. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "inference-wire"
    agent: "builder"
    description: "Add internal/inference/exec.go (KiroCLIAgentCommand, KiroCLIAgentResponse, ExecEnvelope, ServeExec, DecodeExecResult), json tags on Request/Response/Usage, and refactor the kiro-cli backend's invokeAgent to use the shared command/response helpers with unchanged behaviour. Include unit tests."
    dependencies: []
    acceptance_criteria:
      - "KiroCLIAgentCommand returns args 'chat --agent <a> --no-interactive --trust-all-tools' and appends '--model <m>' only when req.Model is non-empty; the prompt never appears in args"
      - "KiroCLIAgentResponse strips ANSI, echoes req.Model as Response.Model, and uses EstimateUsage (estimated source)"
      - "kiroCLIBackend.invokeAgent uses both helpers and all existing tests in internal/inference/kirocli_test.go pass unmodified"
      - "ServeExec reads one JSON Request, runs the named backend, and writes one ExecEnvelope; a stub request with a Stub script returns the scripted text, model and usage; a backend error is carried in the envelope; an unknown backend returns an error"
      - "DecodeExecResult round-trips Response and returns an error satisfying errors.Is(err, ErrTimeout) when Timeout is set"
      - "internal/inference does not import internal/eval"
    validation_commands:
      - "go build ./internal/inference/..."
      - "go vet ./internal/inference/..."
      - "go test ./internal/inference/... -count=1"
      - "test -z \"$(gofmt -l internal/inference)\""

  - id: "sandbox-exec"
    agent: "builder"
    description: "In internal/eval/sandbox add ExecResult and Container.ExecWithStdin (stdin attached and half-closed, concurrent with output demux, ctx cancellation closes the stream), reimplement ExecWithOutput on top with unchanged semantics, and add ResolveLinuxBinary (env override, matching linux executable, cross-compile fallback, memoised, actionable error) with a pure, unit-tested decision function. Add a daemon-gated stdin round-trip test."
    dependencies: []
    acceptance_criteria:
      - "ExecWithStdin delivers stdin to the process, returns untrimmed Stdout and Stderr separately plus ExitCode, and reports a non-zero exit via ExitCode rather than an error"
      - "ExecWithOutput behaviour is unchanged (trimmed stdout; error text 'command failed with exit code N: <stderr>') and existing sandbox tests still compile and pass or skip"
      - "Cancelling the context makes ExecWithStdin return promptly with the context error"
      - "ResolveLinuxBinary honours KAIRON_SANDBOX_BINARY first, uses os.Executable only when GOOS=linux and GOARCH matches the platform, otherwise cross-compiles with CGO_ENABLED=0 from the kairon module root, and its error names KAIRON_SANDBOX_BINARY when nothing works"
      - "Daemon-gated test round-trips a payload larger than 1 MiB containing quotes and newlines through 'cat' byte-identically, and is skipped with the existing skipIfNoContainerDaemon message when no daemon is reachable"
    validation_commands:
      - "go build ./internal/eval/sandbox/..."
      - "go vet ./internal/eval/sandbox/..."
      - "go test ./internal/eval/sandbox/... -count=1"
      - "test -z \"$(gofmt -l internal/eval/sandbox)\""

  - id: "inference-exec-cmd"
    agent: "builder"
    description: "Add the hidden cobra command 'kairon inference-exec --backend <name>' in cmd/kairon/cmd/inference_exec.go that calls inference.ServeExec with os.Stdin/os.Stdout, writes nothing else to stdout, and is hidden from help. Add a test."
    dependencies: ["inference-wire"]
    acceptance_criteria:
      - "'kairon inference-exec --backend stub' with a JSON stub Request on stdin prints exactly one JSON ExecEnvelope on stdout containing the scripted response, and exits 0"
      - "An unknown backend exits non-zero with the error on stderr and nothing on stdout"
      - "The command is Hidden and does not appear in 'kairon --help'"
      - "No banner, log or version output is written to stdout when the command runs"
    validation_commands:
      - "go build ./cmd/..."
      - "go test ./cmd/... -count=1"
      - "printf '%s' '{\"Role\":\"agent\",\"Agent\":\"a\",\"Prompt\":\"p\",\"Stub\":{\"turns\":[{\"response\":\"hello\"}]}}' | go run ./cmd/kairon inference-exec --backend stub | grep -q hello"
      - "! go run ./cmd/kairon --help | grep -q inference-exec"

  - id: "runner-container-path"
    agent: "builder"
    description: "Rework the container branch of internal/eval/runner.go so it runs through the same inference.Request and completion logic as the native path: extract newAgentRequest and completeAgentCall; add agentExecer and runAgentInContainer (prompt on stdin, kiro-cli executed directly with the shared argv incl. --model, other backends executed via '/tmp/kairon inference-exec --backend <name>' using ResolveLinuxBinary and CopyTo); skip kiro-cli validation and GitHub mocking for non-kiro-cli backends; add the debug line recording backend/agent/model; remove the stub+sandbox guard and estimateCost; change pinRun/resolveAgentProvenance so evals.agent_model is honoured under --sandbox (no warning) and the evals-dir agent overlay is ignored only for the kiro-cli backend. Update affected tests and add container_agent_test.go with a fake agentExecer."
    dependencies: ["inference-wire", "sandbox-exec"]
    acceptance_criteria:
      - "invokeAgent has one shared request builder and one shared completion function used by both native and container paths; the hand-built CallRecord with Estimated:true and the estimateCost function no longer exist, and nothing on the container path calls estimateCost"
      - "Prompt is delivered on stdin and never appears in any argv element, verified with a 1 MiB prompt containing quotes, newlines and $(...)"
      - "kiro-cli container argv equals the native argv and ends with '--model <m>' when req.Model is set; there is no --model when it is empty"
      - "With the stub backend the container path returns the scripted text and the same output, CostInfo (tokens, usd, Model, UsageSource) and CallRecord as the native stub run for both stub-basic and stub-usage self-test cases"
      - "With cConfig.Debug the container invocation prints a line naming the backend, agent and request model"
      - "RunWithOptions no longer rejects --backend stub with --sandbox; a pinned selftest agent passes the pre-flight under --sandbox --backend stub using the evals-dir agent config"
      - "evals.agent_model is honoured for --sandbox runs and the 'ignored for --sandbox' warning is removed; for the kiro-cli backend the evals-dir overlay is still ignored for provenance"
      - "Container timeout, OOM and image-pull errors keep their existing user-facing messages and timeouts satisfy errors.Is(err, inference.ErrTimeout)"
      - "All existing tests in internal/eval still pass after being updated for the removed guard, estimateCost and warning"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... ./internal/inference/... -count=1"
      - "! grep -n 'estimateCost' internal/eval/runner.go"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "selftest-sandbox"
    agent: "builder"
    description: "Add the stub-quoted-input self-test case, the gated TestSelftestSandbox (skips with a clear Podman/Docker message when no daemon, skips unless KAIRON_EVAL_SANDBOX_SELFTEST=1; otherwise runs selftest natively and under --sandbox --backend stub and compares actual_output, agent_cost and call model), and the 'eval:selftest:sandbox' Taskfile target."
    dependencies: ["inference-exec-cmd", "runner-container-path"]
    acceptance_criteria:
      - "internal/eval/testdata/evals/cases/selftest/stub-quoted-input.yaml exists with shell metacharacters in its input and a stub turn, and native 'task eval:selftest' still passes with all self-test case-count assertions updated"
      - "Taskfile.yml defines eval:selftest:sandbox whose description says it needs Podman or Docker and which runs TestSelftestSandbox with KAIRON_EVAL_SANDBOX_SELFTEST=1"
      - "Without a container daemon the target exits 0 and prints a skip message that names Podman and Docker"
      - "Without KAIRON_EVAL_SANDBOX_SELFTEST=1 'go test ./internal/eval' does not start any container"
      - "With a daemon, the test passes and asserts non-empty sandbox output equal to the native output, and equal model and cost fields"
      - "'task sync:check' still passes and no file under cmd/kairon/templates is modified"
    validation_commands:
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "task eval:selftest:sandbox"
      - "task sync:check"
      - "git diff --quiet -- cmd/kairon/templates"

  - id: "update-docs"
    agent: "documenter"
    description: "Update docs/evaluation.md for the new container behaviour: remove the stub+sandbox restriction and the 'always estimated / ignores evals.agent_model' statements, rewrite the '--sandbox and evals.agent_model' section, update sandbox provenance bullets, document per-backend container execution (kiro-cli direct, stub via kairon inference-exec with KAIRON_SANDBOX_BINARY / cross-compile requirements, image build needs network), stdin prompt delivery, and the eval:selftest:sandbox target with its skip behaviour."
    dependencies: ["selftest-sandbox"]
    acceptance_criteria:
      - "docs/evaluation.md no longer says --backend stub cannot be combined with --sandbox"
      - "docs/evaluation.md states that --sandbox honours evals.agent_model and passes --model, and no longer says the sandbox ignores it or emits a warning"
      - "docs/evaluation.md states the prompt is delivered on stdin and describes how kiro-cli and stub run in the container, including KAIRON_SANDBOX_BINARY"
      - "docs/evaluation.md documents 'task eval:selftest:sandbox', the direct 'go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest' command, and the skip message when no Podman/Docker daemon is present"
      - "No file under .kairon/specs other than this issue's spec is modified"
    validation_commands:
      - "! grep -n 'cannot be combined with `--sandbox`' docs/evaluation.md"
      - "grep -n 'eval:selftest:sandbox' docs/evaluation.md"
      - "grep -n 'KAIRON_SANDBOX_BINARY' docs/evaluation.md"
      - "git diff --stat -- docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification that every acceptance criterion of issue #296 is met and that build, tests, lint, formatting and template sync pass."
    dependencies: ["update-docs"]
    acceptance_criteria:
      - "AC1: the container path writes the prompt to stdin only (argv contains no prompt) and test evidence shows non-empty output equal to the native run"
      - "AC2: the container branch calls the same Request/Backend pipeline selected by --backend, the stub runs in-container via inference-exec, and the stub+sandbox guard is gone"
      - "AC3: the request model reaches kiro-cli as --model, the debug line records it, and pinned runs never send an empty model"
      - "AC4: container cost comes from costFromUsage via the shared completion function; estimateCost is not defined or called on the container path"
      - "AC5: task eval:selftest:sandbox exists, runs selftest under --sandbox --backend stub, and skips with a clear message when no daemon is reachable"
      - "task test, task lint, task fmt:check and task sync:check pass"
      - "Only this issue's spec was added under .kairon/specs and no historical artifact was edited"
    validation_commands:
      - "go build ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "task eval:selftest:sandbox"
      - "! grep -rn 'estimateCost' internal/eval --include=*.go"
      - "git diff --name-only origin/main -- .kairon/specs"
```
