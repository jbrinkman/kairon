# Design Spec: Evals sandbox containment — fake gh, read-only FS, tool trust

Closes #298

Depends on #297 (merged: tools-only base image, mounted `.kiro` / workspace / `.eval/`).

## 1. Problem and Scope

A `--sandbox` eval run must contain an arbitrary agent at the layers Kairon controls, without assuming a fixed tool set. Three enforceable, mechanism-cheap layers are added:

| Layer | Mechanism | Enforced by |
|-------|-----------|-------------|
| `gh` | A fake `gh` script is bind-mounted read-only and put first on `PATH`. It logs to `.eval/gh.log`, copies `--body-file` bodies, and answers `gh issue view` from the case's `gh_issue`. The baked real `gh` stays reachable by absolute path but is unauthenticated. | Kernel mount + env |
| Filesystem | `ReadonlyRootfs: true` plus small tmpfs mounts for temp and `$HOME`. Only the workspace and `.eval/` (existing mounts) are writable. | Container runtime |
| Tool trust | `--trust-all-tools` is replaced in the container by `--trust-tools=<per-agent set>` (whole tool, not per argument). | kiro-cli |

Out of scope (do not touch): network egress control (`NetworkMode: "none"` from #297 is left exactly as it is), mock guidance, provenance/scoring parity in `summary.json`, extension-seam prose and E4 supersession, new check types (E5-E7). Per-argument interception is a documented limit, not built here.

## 2. Findings From the Codebase

- `internal/eval/sandbox/mounts.go` — `newHostConfigWithMounts` builds the `HostConfig` (limits, `NetworkMode: "none"`, structured bind mounts). No `ReadonlyRootfs`, no tmpfs.
- `internal/eval/runner.go` — `buildContainerMounts` (3 mounts; +helper for non-kiro-cli), `containerEnv`, `newContainerConfig`, `invokeAgent` (container branch builds the `inference.Request`), `runAgentInContainer` (uses `inference.KiroCLIAgentCommand(req)`), `newCallRecord`. `TestRunnerHasNoRuntimeInstalls` bans `.CopyTo(` in `runner.go`: the fake `gh` must arrive by **mount**, not copy.
- `internal/inference/kirocli.go` — `KiroCLIAgentCommand` hard-codes `--trust-all-tools`; it is shared by the native backend and the container transport. Many tests pin the native string, so the native path must stay `--trust-all-tools`.
- `internal/inference/stub.go` — `StubTurn.Commands` run via `sh -c` in `Request.WorkDir`; under `--sandbox` the stub runs *inside* the container through `kairon inference-exec --backend stub`, so `Request` and `Response` cross a JSON boundary (new fields need JSON tags).
- `internal/eval/sandbox/mock_github.go` — leftover `MockGitHubSkill` (embeds `testdata/github-cli-mock/`, a `/tmp/gh-mock.log` script that fakes a logged-in state) and `SimulateGitHubResponse`, both marked "kept for #298". `ContainerConfig.MockGitHub` is unused ("fake gh behavior is #298").
- `internal/eval/dockerfile/base.Dockerfile` — real `gh` at `/usr/local/bin/gh`; no `jq`. The image build context is a Dockerfile only (no `COPY`). We do not change the image.
- `internal/eval/workspace.go` — `caseWorkspace{Dir, EvalDir, KiroDir, root}`; `root` is a private 0700 parent. `.eval/` is created empty; the workspace is `a+rwX`.
- `internal/config/evals.go` — `EvalsConfig` (`agent_model`, `judge_model`, `allowed_models`), read once by `pinRun` into `runPins`.
- Agent JSON in this repo already carries `allowedTools` (e.g. builder `["read","write","shell"]`, documenter `["read","write"]`). Test agents `selftest*.json` have `allowedTools: ["read"]`.
- Local `kiro-cli chat --help` (v2.28) confirms `--trust-tools <TOOL_NAMES>`: "`--trust-tools=fs_read,fs_write`", "trust no tools: `--trust-tools=`". Built-in tool names seen in the binary: `fs_read`, `fs_write`, `execute_bash`; MCP tools are `@server/tool`.
- Template-synchronized paths (`.kiro/agents`, `.kairon/{scripts,themes,evals}`, sentinel skill) are **not touched** by this change; all eval fixtures live under `internal/eval/testdata/evals/`. `task sync:check` should stay green with no template edits.

## 3. Solution Approach

### 3.1 Fake `gh` (configured, not installed)

**Delivery.** A POSIX-`sh` script, embedded in the Go binary, is written by the host into a per-case directory `<root>/bin/gh` (mode 0755, dir 0755; `root` is the existing private parent, removed with the workspace). That directory is bind-mounted **read-only** at `/opt/kairon/bin`. The container `PATH` becomes `/opt/kairon/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`. Nothing is copied into a running container, the base image is unchanged (so its cache key is unchanged), and `/usr/local/bin/gh` (real) is left in place so absolute-path calls reach the unauthenticated real binary.

**Backing data.** Written by the host into the mounted `.eval/` before the container starts (so the existing `a+rwX` pass covers it):
- `.eval/gh-issue.json` — the case's `gh_issue` rendered as valid JSON with **one top-level field per line** (so the script can select `--json` fields with `grep`, no `jq`).
- `.eval/gh-issue.txt` — the default human `gh issue view` rendering.
- Only written when the case defines `gh_issue`.

The script finds `.eval` via env `KAIRON_EVAL_DIR` (set to `<workspace_dir>/.eval`), so a non-default `sandbox.workspace_dir` works.

**Behavior** (every invocation, including failures, first appends one line to `$KAIRON_EVAL_DIR/gh.log`):
- Log line: `gh <args>` — args space-joined, no timestamp (deterministic); an arg is single-quoted only if it contains characters outside `[A-Za-z0-9_./:=@%+,-]` or is empty; newlines inside an arg are folded to spaces so a call is always one line.
- `--body-file <path>` / `-F <path>` / `--body-file=<path>`: claim the next free `n` atomically (noclobber create of `gh-body-<n>.md`), then copy the file over it. `-` copies stdin. A missing file → real-gh-style `open <path>: no such file or directory`, exit 1 (still logged).
- `gh issue create` → prints `https://github.com/fake-owner/fake-repo/issues/<seq>`; `gh pr create` → `.../pull/<seq>` (`seq` = line number in `gh.log`).
- `gh issue view [N|url]` → from `gh_issue`: no `--json` → `gh-issue.txt`; `--json a,b` → `{...}` with just those fields; unknown field → `Unknown JSON field: "x"`, exit 1. If `N` differs from the configured number, or no `gh_issue` exists, exit 1 with a gh-style "could not resolve" / "no issue configured for this case (gh_issue)". `--jq`, `-q`, `--template`, `-t` are **not supported** (no `jq` in the image): exit 1 with a clear message, logged. Documented limit.
- Simulated (success, empty or plausible output): `issue list|edit|comment|close|reopen`, `pr list|view|comment|edit`, `auth status` (reports a fake logged-in state so agents' pre-checks pass — this is the *fake*, never the real binary), `--version`.
- Everything else: logged, stderr `kairon fake gh: "<args>" is not simulated (call logged only)`, exit 1. Honest failure rather than silent success; case authors see the call in `gh.log` and can extend the script.

**Real `gh` unauthenticated.** The container env is built from scratch (no host token leaks today). Make it a guarantee: `containerEnv` additionally (a) drops `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_HOST` if a caller ever puts them in `ContainerConfig.Environment`, and (b) sets `GH_PROMPT_DISABLED=1`, `GH_NO_UPDATE_NOTIFIER=1`. `$HOME` is a fresh tmpfs, so there is no `~/.config/gh/hosts.yml`. `gh auth status` by absolute path therefore reports "not logged in" and exits non-zero. (The existing `NetworkMode: "none"` is defence in depth only; this layer does not rely on it.)

**Replace legacy code.** Delete `MockGitHubSkill`, `GitHubMockResponse`, `SimulateGitHubResponse` and `sandbox/testdata/github-cli-mock/` and the test that references them (`TestMockGitHubSkill_Embedded` in `installation_test.go`). Remove `ContainerConfig.MockGitHub` (and its use in `createContainerConfig`, `runner_config_test.go`). `mock_github.go` keeps the new embedded script (`//go:embed fakegh/gh`), the `GHIssue` type, `RenderGHIssue`, and a `WriteFakeGH(dir)` helper. Behavior only; no install code.

**New case field** (`types.go`):

```yaml
gh_issue:            # optional; data for `gh issue view` under --sandbox
  number: 42         # default 1
  title: "Add widget"   # required when gh_issue is set
  body: "..."
  state: OPEN        # default OPEN
  author: octocat    # default "fake-user"
  labels: [bug]
requires_sandbox: true   # optional; refuse to run this case natively
```

`gh_issue` set without `requires_sandbox: true` is a load error (a native run would call the **real** `gh` on the developer's machine). `executeCase` with `cConfig == nil` and `RequiresSandbox` records a failed case ("case requires --sandbox") **before** creating a workspace or invoking anything.

### 3.2 Read-only root filesystem

In `newHostConfigWithMounts`: set `ReadonlyRootfs: true` and `Tmpfs` for `/tmp` (`rw,nosuid,nodev,mode=1777,size=256m`), `/var/tmp` (same, `size=64m`) and `/home/sandbox` (`rw,nosuid,nodev,uid=1000,gid=1000,mode=0755,size=256m`). `$HOME` must be writable because `kiro-cli` keeps state there; it is ephemeral per container. No `noexec` (kiro-cli and tools may exec from temp). Always on — no opt-out. Tmpfs memory counts against the container memory limit; sizes are named constants. Existing assertions (`NetworkMode == "none"`, no `/workspace` tmpfs) stay true. Writes outside mounts then fail with `Read-only file system` (EROFS) — distinguishable from the `Permission denied` the unprivileged user already got for `/etc`, which is what the test must assert.

### 3.3 Tool trust (whole-tool)

**Config names (relied on by later issues — keep stable):**
- YAML: `evals.trust_tools` — `map[string][]string`, agent name → tool names.
- Go: `config.EvalsConfig.TrustTools`; `inference.ToolTrust`; `inference.Request.ToolTrust`; `eval.resolveTrustSet(agent)`.

**Resolution** (`internal/eval/trust.go`, only for container runs), first match wins:
1. `evals.trust_tools[<agent>]` from `.kairon/config.yaml` (explicit override; may be `[]` = trust nothing).
2. The agent config's `allowedTools` (same file `resolveAgentProvenance` reads: `<evals-dir>/agents/<agent>.json`, else `.kiro/agents/<agent>.json`).
3. Neither → empty set (`--trust-tools=`), **fail closed**. A missing agent config is an error naming the paths tried.

Entries `"*"` or empty are rejected (config load error) so trust-all cannot be restored by accident. `toolsSettings` stay agent-author-owned and pass through in the staged `.kiro` unchanged; Kairon generates none (that would be per-argument policy).

**Name normalization** (`inference/trust.go`, one table): `read→fs_read`, `write→fs_write`, `shell→execute_bash`, `aws→use_aws`; `fs_*`/`execute_bash`/`use_aws`, `@server`, `@server/tool` and any other name pass through; duplicates dropped, order kept.

**Plumbing.** `inference.Request.ToolTrust *ToolTrust` (`nil` = unrestricted/legacy; non-nil, even with zero tools = restricted). `invokeAgent`'s container branch sets it; the native path never does.
- `KiroCLIAgentCommand`: `ToolTrust == nil` → `--trust-all-tools` (byte-identical to today); else a single argv element `--trust-tools=<csv>` (`--trust-tools=` when empty), and the same text in the human `command` string.
- Stub backend: new `StubTurn.ToolCalls []StubToolCall{Tool, Command}`. After `Commands`, each tool call runs `sh -c Command` in `WorkDir` **only if** `ToolTrust == nil` or the normalized tool is in the set; otherwise the command is skipped and a `ToolDenial{Tool, Command, Reason: "tool not trusted"}` is added to `Response.ToolDenials`. `Commands` stay ungated (scripted environment actions, not tool calls) so existing self-tests are unaffected. This is a **model of the harness plumbing**, deterministic and network-free; real enforcement is kiro-cli's.
- Recording: `inference.CallRecord` gains `TrustedTools *[]string json:"trusted_tools,omitempty"` (pointer so a restricted-to-nothing `[]` is distinct from native `null`/absent) and `ToolDenials []ToolDenial json:"tool_denials,omitempty"`. `newCallRecord` copies them from `req`/`resp`. Additive: older `*.json` results still load.
- Limit (documented): kiro-cli's own denials are not reliably detectable from its output, so `tool_denials` is populated by the stub only; for `kiro-cli` the record shows **which tools were trusted**, and the raw output/stderr is kept as today.

## 4. Relevant Files

Create:
- `internal/eval/sandbox/fakegh/gh` — embedded fake `gh` script (POSIX sh).
- `internal/eval/sandbox/mock_github_test.go` — hermetic script tests (run `sh` on the host).
- `internal/inference/trust.go`, `internal/inference/trust_test.go` — `ToolTrust`, normalization, `ToolDenial`.
- `internal/eval/trust.go`, `internal/eval/trust_test.go` — `resolveTrustSet`.
- `internal/eval/containment_sandbox_test.go` — daemon-gated AC1-AC3 tests.
- `internal/eval/testdata/evals/agents/selftest-sandbox.json` (+ `-prompt.md`), `selftest-sandbox-ro.json` (+ `-prompt.md`)
- `internal/eval/testdata/evals/rubrics/selftest-sandbox.yaml`, `selftest-sandbox-ro.yaml`
- `internal/eval/testdata/evals/cases/selftest-sandbox/*.yaml`, `cases/selftest-sandbox-ro/*.yaml` (listed in Task 6)

Modify:
- `internal/eval/sandbox/mock_github.go` — replace legacy content (see 3.1).
- `internal/eval/sandbox/mounts.go`, `mounts_test.go` — read-only root + tmpfs.
- `internal/eval/sandbox/installation_test.go` — drop `TestMockGitHubSkill_Embedded`.
- `internal/inference/{inference.go,kirocli.go,stub.go,record.go}` and their tests (`kirocli_test.go`, `stub_test.go`, `stub_commands_test.go`, `exec_test.go` as needed).
- `internal/config/evals.go`, `evals_test.go` — `TrustTools`, normalization/validation.
- `internal/eval/{runner.go,types.go,workspace.go,provenance.go,execute_case.go}` — `BinDir` + mount, env, `gh_issue` render, case fields + validation, `RequiresSandbox` guard, pins carry trust overrides, `invokeAgent` sets `ToolTrust`, `newCallRecord`, remove `MockGitHub`.
- Tests that pin the container argv or mount count: `backend_routing_test.go`, `container_agent_test.go`, `container_runner_test.go`, `runner_config_test.go`, `workspace_test.go`, `sandbox_workspace_test.go`.
- `Taskfile.yml` — add the new gated test to `eval:selftest:sandbox`.
- `docs/evaluation.md`, `docs/unified-container-flow.md` (the "no mocked tools" / "gh unconfigured" statements).

Delete: `internal/eval/sandbox/testdata/github-cli-mock/`.

## 5. Team Orchestration

```
trust-core ──► trust-resolution ─┐
fake-gh ─────────────────────────┼─► wire-containment ─► selftest-fixtures ─► docs ─► validate-all
readonly-fs ─────────────────────┘
```

`trust-core`, `fake-gh` and `readonly-fs` touch disjoint packages (`inference`, `sandbox/mock_github*`, `sandbox/mounts*`) and run in parallel. `trust-resolution` and `wire-containment` both edit `runner.go`, so `wire-containment` is sequenced after all three. Fixtures/tests need the wiring; docs describe the finished behavior; the validator closes. One PR.

## 6. Step-by-Step Task Breakdown

### Task 1 — `trust-core` (builder)
`internal/inference`: add `ToolTrust` (+ `NewToolTrust(names)` with the alias table, dedupe), `ToolDenial`, `Request.ToolTrust`, `Response.ToolDenials`, `CallRecord.TrustedTools/ToolDenials`; `KiroCLIAgentCommand` emits `--trust-tools=<csv>` when `ToolTrust != nil` (single argv element; empty → `--trust-tools=`); `StubTurn.ToolCalls` with the trust gate.
Acceptance: nil `ToolTrust` output identical to today (existing tests unchanged); `{fs_read}` → `--trust-tools=fs_read`; empty → `--trust-tools=`; `read`/`write`/`shell` normalize; stub denies an untrusted tool call (command not run, denial recorded) and runs a trusted one; `Request`/`Response` round-trip through `ServeExec`/`DecodeExecResult` with the new fields.
Dependencies: none.

### Task 2 — `trust-resolution` (builder)
`config.EvalsConfig.TrustTools` (`trust_tools`; trim, drop empty agent keys, reject `*`/empty entries with an error naming the agent; default nil). `agentConfigFile` gains `AllowedTools`. `runPins` carries the overrides (set in `pinRun` from the already-loaded `EvalsConfig`). `eval.resolveTrustSet(agent)` per 3.3. `invokeAgent` container branch sets `req.ToolTrust`; resolution errors fail the call with `tool trust: …`. `newCallRecord` copies trust/denials. Update container-argv tests (`backend_routing_test`, `container_agent_test`) to point `cfg.evalsDir` at a temp dir with an agent config and expect `--trust-tools=…`.
Acceptance: precedence override > `allowedTools` > empty; missing config is an error; `*` rejected; native `invokeAgent` still emits `--trust-all-tools`; container call for an agent with `allowedTools: ["read","write"]` gets `--trust-tools=fs_read,fs_write`; record shows `trusted_tools`.
Dependencies: `trust-core`.

### Task 3 — `fake-gh` (builder)
Write the script, `GHIssue` + `RenderGHIssue` (JSON one-field-per-line + text), `WriteFakeGH`, delete legacy code and its test/testdata (3.1). Test by executing the script with `sh` against a temp `KAIRON_EVAL_DIR` (skip if `sh` is missing); also run it under `busybox sh`/`dash` if present, since the image uses busybox ash.
Acceptance: `issue create --title t --body-file b.md` → log line `gh issue create --title t --body-file b.md`, `gh-body-1.md` == `b.md`, second call → `gh-body-2.md`; `--body-file -` copies stdin; missing body file → exit 1 and still logged; `issue view --json title,body` returns only those fields as valid JSON; wrong number / no `gh_issue` / `--jq` → exit 1 with message; args with spaces are single-quoted in the log; unsimulated command → exit 1, logged; nothing is ever executed from `PATH` (the script never invokes another `gh`).
Dependencies: none.

### Task 4 — `readonly-fs` (builder)
`ReadonlyRootfs: true` + tmpfs map in `newHostConfigWithMounts`; constants for sizes; update `mounts_test.go` (assert `ReadonlyRootfs`, the three tmpfs entries and options, `NetworkMode` still `none`, still no `/workspace` tmpfs).
Acceptance: unit assertions above pass; no change to bind-mount behavior.
Dependencies: none.

### Task 5 — `wire-containment` (builder)
- `workspace.go`: `caseWorkspace.BinDir` (`<root>/bin`), created 0755 with the fake `gh` written 0755 in `newCaseWorkspace`; render `gh-issue.{json,txt}` into `.eval/` when `tc.GHIssue != nil` (before `setPermissions`).
- `types.go`: `TestCase.GHIssue *sandbox.GHIssue`, `RequiresSandbox bool`; remove `ContainerConfig.MockGitHub`.
- `runner.go`: `buildContainerMounts` adds `{BinDir → /opt/kairon/bin, ReadOnly}`; `containerEnv` adds `PATH`, `KAIRON_EVAL_DIR`, `GH_PROMPT_DISABLED`, `GH_NO_UPDATE_NOTIFIER` and filters GitHub token vars; `loadCases`/`validateCaseFields` validation (3.1: title required, number default 1, `gh_issue` ⇒ `requires_sandbox`); `executeCase` guard.
- Update tests that count mounts (3 → 4, helper case 4 → 5), env, and `runner_config_test.go`; keep `TestRunnerHasNoRuntimeInstalls` green (no `.CopyTo(`).
Acceptance: mounts include the read-only `/opt/kairon/bin`; env `PATH` begins with `/opt/kairon/bin`; a token in `ContainerConfig.Environment` never reaches the container; native run of a `requires_sandbox` case fails fast without a workspace; `gh_issue` without `requires_sandbox` is rejected at load; rendered `gh-issue.json` is valid JSON one-field-per-line.
Dependencies: `fake-gh`, `readonly-fs`, `trust-resolution`.

### Task 6 — `selftest-fixtures` (builder)
Add under `internal/eval/testdata/evals/`:
- Agent `selftest-sandbox` (`allowedTools: ["read","write"]`) with cases (all `requires_sandbox: true`, stub backend):
  - `stub-gh-fake` — commands: write `b.md`, `gh issue create --title t --body-file b.md`, `gh issue view --json title,body > view.json`; `gh_issue` set.
  - `stub-workspace-write` — `echo x > ./marker.txt` succeeds.
  - `stub-tool-allowed` — `tool_calls: [{tool: fs_write, command: "echo x > tool-marker.txt"}]`.
- Agent `selftest-sandbox-ro` (`allowedTools: ["read"]`) with cases:
  - `stub-write-outside-mounts` (expected to **fail**, like `selftest-fail`) — command `echo x > /etc/kairon-probe`.
  - `stub-tool-denied` — same `fs_write` tool call; case completes, `tool-marker.txt` absent, denial recorded.
- Rubrics modeled on `selftest.yaml` (the response text must contain a `##` section and a `###` subsection).
`containment_sandbox_test.go` (gated by `KAIRON_EVAL_SANDBOX_SELFTEST=1` and `sandbox.EnsureContainerDaemon()`, skip otherwise — same pattern as `TestSelftestSandbox`): runs the agents under `--sandbox --backend stub --keep-workspaces` and asserts:
  1. `.eval/gh.log` contains the create call; `.eval/gh-body-1.md` byte-equals `b.md`; `view.json` has the configured title/body.
  2. `stub-write-outside-mounts` fails and its `ErrorContext.Stderr` contains `Read-only file system`; `marker.txt` exists on the host in `stub-workspace-write`.
  3. `stub-tool-denied`: `tool-marker.txt` absent, `Calls[0].ToolDenials` has `fs_write`, `TrustedTools == ["fs_read"]`; `stub-tool-allowed`: file present, no denials, `TrustedTools` contains `fs_write`.
  4. Direct container check (via `sandbox.Container` with the same `HostConfig`): `command -v gh` → `/opt/kairon/bin/gh`; `/usr/local/bin/gh auth status` exits non-zero with output matching `(?i)not logged in`.
Non-gated tests: a native run of `selftest-sandbox` records every case as failed with "requires --sandbox" and creates no workspace; fixtures load and validate. Add the new test name to the regex in `Taskfile.yml` `eval:selftest:sandbox`.
Dependencies: `wire-containment`.

### Task 7 — `docs` (documenter)
`docs/evaluation.md`: new **Sandbox Containment** section (layers table; fake `gh` behavior, log/body files, `gh_issue`, unsupported `--jq`, real `gh` unauthenticated; read-only root + tmpfs list; tool trust: resolution order, `evals.trust_tools`, alias table, **default per-agent trust set table** for the template agents — architect/builder/validator `read,write,shell`, documenter `read,write`, planner `read,write,shell,web_search,web_fetch`, krew-lead `read,shell,subagent,todo_list`; "whole tool, not per argument"; kiro-cli denial limit; stub `tool_calls` gate is a model). **Limits** subsection: network is not severed or gatewayed *by this layer* — the current container still has `NetworkMode: "none"` from #297 and network policy is the network/mock-guidance work; production side effects over the network are the eval author's responsibility via mocks, with a forward reference to the mock-guidance issue (do not invent an issue number); per-argument interception is an extension seam covered by the final issue. Update stale statements: line ~157 (native keeps `--trust-all-tools`), ~296 and ~825 (container uses `--trust-tools=…`, differs from native), the base-image paragraph ("`gh` is present but unconfigured… #298"), the Mounts table and lifecycle step 3 (new `/opt/kairon/bin` mount, read-only root), "GitHub CLI in the Sandbox", Stub Case Fields (`tool_calls`, `gh_issue`, `requires_sandbox`), per-call record fields (`trusted_tools`, `tool_denials`), the `evals` block (`trust_tools`), and Self-Test (new cases/test). Update the `docs/unified-container-flow.md` sentence about mocked tools. Add a commented `trust_tools` example to `.kairon/config.yaml`'s `evals:` block comment only if the file is not template-synchronized (it is not listed in sync mappings; confirm with `task sync:check`).
Dependencies: `selftest-fixtures`.

### Task 8 — `validate-all` (validator)
Read-only verification of every acceptance criterion below and of the full test/lint suite; run the gated sandbox tests if a daemon is reachable, else report them as not run.
Dependencies: `docs`.

## 7. Acceptance Criteria Traceability

| Issue AC | Where satisfied | Verified by |
|----------|-----------------|-------------|
| 1 fake gh | 3.1; Tasks 3, 5, 6 | `mock_github_test.go` (script), `TestContainment…` (`gh.log`, `gh-body-1.md`, `view.json`, absolute-path `gh auth status` not logged in) |
| 2 read-only FS | 3.2; Tasks 4, 6 | `mounts_test.go` (`ReadonlyRootfs`, tmpfs); `stub-write-outside-mounts` fails with `Read-only file system`; `stub-workspace-write` succeeds |
| 3 tool trust | 3.3; Tasks 1, 2, 6 | argv tests (`--trust-tools=fs_read`), `stub-tool-denied` vs `stub-tool-allowed`, `trusted_tools`/`tool_denials` in the call record; default sets in docs |
| 4 docs | Task 7 | review of `docs/evaluation.md` |

## 8. Risks, Assumptions and Open Items

1. **Tool names unverified end to end.** `fs_read`/`fs_write` come from `kiro-cli --help`; `execute_bash` from the binary's tool table; `shell`/`read`/`write` → legacy names is an assumption, as are `web_search`, `web_fetch`, `subagent`, `todo_list` passing through. The error string `custom tool '' should be prefixed with @{MCPSERVERNAME}/` suggests `--trust-tools` may reject a bare `@server`; if so, `trust_tools` overrides must use `@server/tool`. The alias table is a single place to fix. The builder should try `kiro-cli chat --no-interactive --trust-tools=@x` locally; there is no way to run a real model in CI, so argv is verified by unit tests only.
2. **How kiro-cli combines `--trust-tools` with the agent's `allowedTools`** is not documented in `--help`. The design makes both agree by default (trust set = `allowedTools`); an override that is wider than `allowedTools` may be additionally limited by kiro-cli itself.
3. **AC3 "records the denial" is exact only for the stub.** kiro-cli denials are not machine-readable, so the record states what was trusted. The stub gate is a deterministic model of the harness plumbing, not of kiro-cli.
4. **Network.** Issue says network is neither severed nor gatewayed *here*; the pre-existing `NetworkMode: "none"` is left untouched (changing it is the network work). This also means a real `kiro-cli` run in the container still cannot reach the model endpoint today (already documented in #297); this PR does not fix that.
5. **`gh` fake limits:** no `--jq`/`--template`; unknown commands exit 1. `auth status` on the *fake* claims logged-in. The fake is bypassed if an agent calls `/usr/local/bin/gh` — by design, that binary is unauthenticated.
6. **`$HOME` tmpfs** is a deliberate hole in "read-only" (needed by kiro-cli); it is ephemeral and size-capped, and is called out in the docs along with `/tmp` and `/var/tmp`.
7. **Native runs are not contained.** `requires_sandbox` is the guard for cases that would be dangerous natively.
8. Not added (out of scope, mention in docs as possible hardening): dropping capabilities, `no-new-privileges`, pids limits.

## 9. Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/inference/... ./internal/config/... ./internal/eval/... -count=1
task test
task lint
task fmt:check
task sync:check
# hermetic run of the stub self-test (native; must still pass unchanged)
task eval:selftest
# daemon-gated: needs Podman or Docker (skips with a message otherwise)
task eval:selftest:sandbox
# containment cases only, under the sandbox
KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestContainment' -count=1 -v
# native refusal of sandbox-only cases (must fail every case, create no workspace, call no gh)
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-sandbox || true
# repo-wide checks of removed names
! grep -rn "SimulateGitHubResponse\|MockGitHubSkill\|github-cli-mock" --include=*.go --include=*.md --exclude-dir=specs --exclude-dir=results --exclude=CHANGELOG.md .
```

## 10. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "trust-core"
    agent: "builder"
    description: "In internal/inference add ToolTrust (with alias-normalizing NewToolTrust), ToolDenial, Request.ToolTrust, Response.ToolDenials, CallRecord.TrustedTools/ToolDenials; make KiroCLIAgentCommand emit a single --trust-tools=<csv> argument when ToolTrust is non-nil (nil keeps --trust-all-tools byte-identical); add StubTurn.ToolCalls with a trust gate in the stub backend (denied calls are skipped and recorded). Add unit tests."
    dependencies: []
    acceptance_criteria:
      - "KiroCLIAgentCommand with nil ToolTrust returns exactly the same args and command string as before"
      - "ToolTrust {fs_read} yields the single argv element --trust-tools=fs_read; an empty restricted set yields --trust-tools="
      - "read/write/shell normalize to fs_read/fs_write/execute_bash, duplicates are dropped, @server/tool names pass through unchanged"
      - "Stub backend skips a tool_call whose normalized tool is not trusted, appends a ToolDenial and still returns the turn response; a trusted tool_call runs its command in WorkDir; Commands remain ungated"
      - "Request and Response with the new fields round-trip through ServeExec and DecodeExecResult"
      - "CallRecord JSON omits trusted_tools when nil and renders [] for a restricted-to-nothing set"
    validation_commands:
      - "go build ./internal/inference/..."
      - "go vet ./internal/inference/..."
      - "go test ./internal/inference/... -count=1"

  - id: "fake-gh"
    agent: "builder"
    description: "Replace the legacy mock in internal/eval/sandbox with the fake gh: embedded POSIX sh script (fakegh/gh) that logs every call to $KAIRON_EVAL_DIR/gh.log, copies --body-file/-F bodies to gh-body-<n>.md, answers gh issue view from .eval/gh-issue.json|txt, simulates a fixed command set and exits 1 for the rest; add GHIssue, RenderGHIssue, WriteFakeGH; delete MockGitHubSkill, GitHubMockResponse, SimulateGitHubResponse, testdata/github-cli-mock and TestMockGitHubSkill_Embedded. Add hermetic tests that execute the script with sh."
    dependencies: []
    acceptance_criteria:
      - "gh issue create --title t --body-file b.md appends exactly one log line 'gh issue create --title t --body-file b.md' and creates gh-body-1.md byte-equal to b.md; a second call creates gh-body-2.md"
      - "--body-file - copies stdin; a missing body file exits 1 with an open-style error and is still logged"
      - "gh issue view --json title,body prints valid JSON containing only those fields; a different issue number, no gh_issue, an unknown JSON field, and --jq each exit 1 with a clear message"
      - "Arguments with spaces or quotes are single-quoted in the log and every call is a single line"
      - "An unsimulated command exits 1, prints 'kairon fake gh: ... is not simulated' to stderr and is logged"
      - "RenderGHIssue emits valid JSON with exactly one top-level field per line and a human text rendering"
      - "The script passes under busybox sh or dash when available and never invokes another gh"
      - "Legacy symbols and testdata are gone and the package builds"
    validation_commands:
      - "go build ./internal/eval/sandbox/..."
      - "go vet ./internal/eval/sandbox/..."
      - "go test ./internal/eval/sandbox/... -count=1"

  - id: "readonly-fs"
    agent: "builder"
    description: "In internal/eval/sandbox/mounts.go set ReadonlyRootfs true and add tmpfs mounts for /tmp, /var/tmp and /home/sandbox (uid/gid 1000) with named size constants; update mounts_test.go. NetworkMode stays none."
    dependencies: []
    acceptance_criteria:
      - "HostConfig.ReadonlyRootfs is true for every config built by NewHostConfigWithMounts"
      - "Tmpfs contains /tmp, /var/tmp and /home/sandbox with rw,nosuid,nodev options, and /home/sandbox carries uid=1000,gid=1000"
      - "No tmpfs entry exists at the workspace path and NetworkMode is still none"
      - "Existing bind-mount validation tests still pass"
    validation_commands:
      - "go build ./internal/eval/sandbox/..."
      - "go test ./internal/eval/sandbox/... -run 'NewHostConfigWithMounts' -count=1"

  - id: "trust-resolution"
    agent: "builder"
    description: "Add evals.trust_tools to config.EvalsConfig (normalize, reject '*' and empty entries), read allowedTools from the agent config, carry overrides in runPins, implement eval.resolveTrustSet (override > allowedTools > empty, missing config is an error), set Request.ToolTrust in invokeAgent's container branch only, copy trust and denials in newCallRecord, and update the container argv tests."
    dependencies: ["trust-core"]
    acceptance_criteria:
      - "evals.trust_tools parses from .kairon/config.yaml, is trimmed, defaults to nil, and a '*' or empty entry is a load error naming the agent"
      - "resolveTrustSet precedence is override, then allowedTools, then an empty set; a missing agent config returns an error listing the paths tried"
      - "A container invokeAgent for an agent whose allowedTools are read and write runs kiro-cli with --trust-tools=fs_read,fs_write and without --trust-all-tools"
      - "A native invokeAgent still passes --trust-all-tools"
      - "The agent CallRecord carries trusted_tools for container calls and omits it for native calls"
      - "Updated backend_routing_test and container_agent_test pass using a temp evals dir with an agent config"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/config/... ./internal/eval/ -run 'Trust|BackendRouting|ContainerAgent|Evals' -count=1"

  - id: "wire-containment"
    agent: "builder"
    description: "Wire the layers into the runner: caseWorkspace.BinDir with the fake gh written by newCaseWorkspace, gh-issue.json/txt rendered into .eval/ when the case has gh_issue, a read-only /opt/kairon/bin mount in buildContainerMounts, container env (PATH with /opt/kairon/bin first, KAIRON_EVAL_DIR, GH_PROMPT_DISABLED, GH_NO_UPDATE_NOTIFIER, GitHub token variables filtered out), TestCase.GHIssue and RequiresSandbox with load-time validation, the executeCase guard for native runs, and removal of ContainerConfig.MockGitHub. Update affected tests."
    dependencies: ["fake-gh", "readonly-fs", "trust-resolution"]
    acceptance_criteria:
      - "buildContainerMounts returns the previous mounts plus a read-only BinDir mounted at /opt/kairon/bin; no runtime copy or install is introduced and TestRunnerHasNoRuntimeInstalls passes"
      - "containerEnv PATH starts with /opt/kairon/bin and sets KAIRON_EVAL_DIR to <workspace_dir>/.eval"
      - "GH_TOKEN, GITHUB_TOKEN, GH_ENTERPRISE_TOKEN, GITHUB_ENTERPRISE_TOKEN and GH_HOST in ContainerConfig.Environment never reach the container env"
      - "A case with gh_issue but without requires_sandbox: true is rejected at load; gh_issue requires a title and defaults number to 1"
      - "A requires_sandbox case run without a container config fails with 'requires --sandbox' before any workspace is created or agent is invoked"
      - "newCaseWorkspace writes .eval/gh-issue.json and .eval/gh-issue.txt only when gh_issue is set, and BinDir/gh is executable"
      - "ContainerConfig.MockGitHub no longer exists and the whole module builds and tests green"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./internal/eval/... -count=1"

  - id: "selftest-fixtures"
    agent: "builder"
    description: "Add the selftest-sandbox and selftest-sandbox-ro agents, rubrics and cases under internal/eval/testdata/evals plus internal/eval/containment_sandbox_test.go (daemon-gated like TestSelftestSandbox) covering fake gh, read-only filesystem and tool trust, a non-gated native-refusal test, and add the new test to the eval:selftest:sandbox task."
    dependencies: ["wire-containment"]
    acceptance_criteria:
      - "Cases stub-gh-fake, stub-workspace-write, stub-tool-allowed (agent selftest-sandbox) and stub-write-outside-mounts, stub-tool-denied (agent selftest-sandbox-ro) exist and every case is requires_sandbox: true"
      - "Gated test: .eval/gh.log contains the gh issue create call and .eval/gh-body-1.md equals b.md; view.json contains the configured gh_issue title and body"
      - "Gated test: stub-write-outside-mounts fails and its ErrorContext stderr contains 'Read-only file system'; stub-workspace-write leaves marker.txt on the host"
      - "Gated test: stub-tool-denied leaves no tool-marker.txt and records an fs_write denial with trusted_tools [fs_read]; stub-tool-allowed creates tool-marker.txt with no denials"
      - "Gated test: inside the container 'command -v gh' is /opt/kairon/bin/gh and '/usr/local/bin/gh auth status' exits non-zero with output matching (?i)not logged in"
      - "Without a daemon or without KAIRON_EVAL_SANDBOX_SELFTEST=1 the gated tests skip with a message; the native-refusal test always runs and passes"
      - "task eval:selftest (native, selftest agent) still passes unchanged"
    validation_commands:
      - "go test ./internal/eval/ -count=1"
      - "task eval:selftest"
      - "KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestContainment' -count=1 -v"

  - id: "docs"
    agent: "documenter"
    description: "Update docs/evaluation.md (new Sandbox Containment section with layers, fake gh, read-only filesystem, tool trust resolution, evals.trust_tools, alias table and default per-agent trust set table, and an explicit Limits subsection: whole-tool trust not per-argument, network not enforced and a forward reference to the mock-guidance issue, kiro-cli denials not detectable) and fix every statement made stale by this change; update the matching sentence in docs/unified-container-flow.md."
    dependencies: ["selftest-fixtures"]
    acceptance_criteria:
      - "docs/evaluation.md documents fake gh behavior, .eval/gh.log, gh-body-<n>.md, gh_issue and the unsupported --jq limit"
      - "docs/evaluation.md documents the read-only root filesystem with the writable mounts and tmpfs paths"
      - "docs/evaluation.md documents the trust resolution order, evals.trust_tools, and a default per-agent trust set table"
      - "docs/evaluation.md states trust is whole-tool and not per-argument, and that network side effects are the eval author's responsibility via mocks with a forward reference to the mock-guidance issue, not an enforced boundary"
      - "Statements that sandbox runs use --trust-all-tools, that gh is unconfigured pending #298, that SimulateGitHubResponse is kept, and the mounts table and lifecycle step are corrected"
      - "No issue number is invented for the mock-guidance issue"
    validation_commands:
      - "grep -n 'trust_tools' docs/evaluation.md"
      - "grep -n 'gh.log' docs/evaluation.md"
      - "! grep -n 'SimulateGitHubResponse' docs/evaluation.md"
      - "task sync:check"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #298 against the implementation and run the full build, test, lint, format and sync checks; run the daemon-gated containment tests if a container daemon is reachable and report them as not run otherwise."
    dependencies: ["docs"]
    acceptance_criteria:
      - "AC1: fake gh logs calls, copies body files, answers issue view from gh_issue, and the real gh by absolute path reports not logged in (or the gated test is reported as not run with the reason)"
      - "AC2: writes outside the mounts fail with 'Read-only file system' while ./marker.txt in the workspace succeeds (or the gated test is reported as not run)"
      - "AC3: the container kiro-cli argv uses --trust-tools=<per-agent set> and never --trust-all-tools; the read-only agent's tool call is denied and recorded, the fs_write agent's succeeds"
      - "AC4: docs/evaluation.md documents the layers and limits as specified"
      - "Native paths are unchanged: native kiro-cli argv still uses --trust-all-tools and task eval:selftest passes"
      - "go build, go vet, task test, task lint, task fmt:check and task sync:check all pass"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "task eval:selftest:sandbox"
```
