# Design Spec: Evals — tools-only base image + mounted .kiro, workspace and outputs for the sandbox

Closes #297

## 1. Problem (verified in code)

`--sandbox` copies nothing in and reads nothing out. Verified against `main` @ `1a3a362` (#296 merged):

| Area | Today | Consequence |
|---|---|---|
| Image (`Container.GenerateDockerfileWithPlatform`) | Generated per run: Alpine + git/curl/bash/unzip + **project toolchains** picked by `DetectProject` (go/node/python/rust/java/task) + kiro-cli from `.../latest/...zip` + `sandbox` user. **No `gh`.** `internal/eval/dockerfile/base.Dockerfile` exists but is not read by any code. | Image content varies with the project, the version of kiro-cli is unpinned, so "rebuild only when the tool set changes" cannot hold. The image is built per run under an evaluation-scoped tag and removed at the end, so nothing is reused across runs. |
| `/workspace` | `NewHostConfigWithLimits` mounts a **tmpfs** at `/workspace`; the comment says project files "must be copied in after container start". | Nothing is ever copied in, and the tmpfs is gone when the container is removed. Outputs can never reach the host. |
| `.kiro/` | `Container.SetupGitHubMocking` runs `mkdir -p /workspace/.kiro/skills` as the unprivileged `sandbox` user, then `CopyToContainer`s a mock skill; `ConfigureMockGitHubPath` appends to `~/.bashrc`. | This is the `mkdir: can't create directory '/workspace/.kiro/': Permission denied` failure (#192/#194): project/eval content installed at runtime under a path the user cannot write. |
| Helper binary (#296) | `runAgentInContainer` `CopyTo`s the linux kairon helper into `/tmp/kairon` at runtime. | Also a runtime copy into the running container (harness tooling rather than eval content, but it depends on a writable `/tmp`, which the containment issue #298 will remove). |
| Native path | **There is no per-case workspace at all.** The native backend runs in the process cwd (the repo root). `TestCase` has no `workspace`/`timeout`, `StubTurn` has no `commands`, there is no `--keep-workspaces`, no `workspace_dir` in `CaseResult`, no `.eval/`, and no E5 check types (`grep -rn 'keep-workspaces\|workspace_dir\|file_exists\|changed_files\|gh_issue' internal cmd` finds only `internal/config` sandbox settings). | See §2.0: the issue text assumes these exist ("reuses the existing workspace fixture format", "behave as native"). They are specified in the Stage 3 gap analysis (E4/E5) but were never built. |

### 2.0 Premise gap and how this spec handles it

Acceptance criteria 3, 5, 6 and 7 are phrased relative to a native workspace, `--keep-workspaces`, `workspace_dir`, case `timeout`, stub `commands` and E5 checks. None of those exist in code, and no open issue builds them (E4 was superseded by this series, #301). The series' dependency list names only #296. Options considered:

1. Build the native side too, minimally, as one shared host-side workspace lifecycle used by **both** paths. Chosen. It is the only way "identical in layout to a native run" and "behave as native" are testable, and the container path needs the host-side builder anyway (AC 3). The names and semantics are exactly those relied on by the gap analysis (`workspace`, `timeout`, `stub.turns[].commands`, `.eval/`, `--keep-workspaces`, `workspace_dir`, fixture root `fixtures/workspaces/`), so later issues see no change.
2. Stop and ask for a new native-workspace issue. Rejected: it blocks the series, and the shared builder is ~300 lines.

Consequences the reviewer should know:

- **Native behavior change:** native runs now execute in a per-case temp workspace instead of the repo root (`inference.Request.WorkDir`). That is the stated Stage 3 goal (gap-analysis E4), but it changes what a real `kiro-cli` eval sees. The workspace contains the project `.kiro/` (so agents and skills resolve) and nothing else unless the case names a fixture.
- **E5 `file_exists` / `changed_files` are NOT built** (explicitly out of scope: "new check types (E5–E7)"). AC 5's "same PASS/FAIL as native" is verified here at the data both checks read: the host workspace's file tree, `git status --porcelain` and `.eval/` contents, compared between a native and a `--sandbox` run of the same case. When E5 lands it reads `CaseResult.WorkspaceDir` (set before scoring, §3.5) and needs no container-specific code.
- **`selftest-fail`** (AC 7) does not exist either. This spec adds an agent named `selftest-fail` under `internal/eval/testdata/evals/` (agent config, rubric, one case). It is a separate agent so the existing `selftest` agent keeps meaning "everything passes".

## 2. Solution Approach

Split strictly by stability. Three layers, each with one owner and one mechanism:

```
build time, as root, cached by content hash        runtime, host-side only, mounted
┌───────────────────────────────────────┐     ┌────────────────────────────────────────────┐
│ kairon-eval-base:<platform>-<hash>    │     │ <ws>/.kiro   ro  staged project .kiro      │
│  alpine, git, bash/sh, ca-certs,      │  +  │ <ws>         rw  git repo, fixture commit  │
│  gh (pinned), kiro-cli (pinned),      │     │ <ws>/.eval   rw  outputs (inside <ws>)     │
│  user sandbox(uid 1000), /workspace   │     │ /opt/kairon/kairon ro  helper (stub only)  │
└───────────────────────────────────────┘     └────────────────────────────────────────────┘
```

No code path creates, copies or installs project/eval content inside a running container. Everything under `<ws>` is created by the harness on the host and **bind-mounted**; results are therefore on the host the moment the process writes them (no "copy `.eval/` out" step is needed — the issue's scope line is satisfied by construction, and AC 5 holds by identity rather than by synchronization).

### 2.1 Base image (build time, as root)

- `internal/eval/dockerfile/base.Dockerfile` is rewritten and finally used. It is embedded with `//go:embed` from a new tiny package file `internal/eval/dockerfile/embed.go` (`package dockerfile; var Base string`), so the Dockerfile is the single definition and `loadTemplate`'s cwd-relative file reads disappear.
- Content (all as root, in this order): `FROM alpine:3.19`; `ARG`s for the tool pins; `apk add git bash ca-certificates` plus build-only `curl unzip` (removed in the same `RUN`); install kiro-cli to `/usr/local/bin/kiro-cli` from the pinned URL; install `gh` from the pinned release tarball for the platform arch to `/usr/local/bin/gh`; `adduser -D -u 1000 -s /bin/bash sandbox`; `mkdir -p /workspace && chown sandbox:sandbox /workspace`; a smoke `RUN kiro-cli --version && gh --version && git --version && sh -c true`; `WORKDIR /workspace`; `USER sandbox`; `CMD ["/bin/bash"]`.
- It contains **no** `COPY`/`ADD` and no project toolchains (go/node/python/…): the issue's stable set is kiro-cli, gh, git, a POSIX shell and the `sandbox` user. (`bash` stays because kiro-cli's shell tool and the existing image assume it; `sh` is the guaranteed POSIX shell.) `gh` is a tool only: unauthenticated, and with `NetworkMode: none` unable to reach GitHub. Fake behavior is #298.
- `sandbox.ToolSet{KiroCLIVersion, GHVersion string}` plus `DefaultToolSet` hold the pins. `ToolSet.BuildArgs(platform)` returns the Docker build args (`KIRO_CLI_URL`, `GH_VERSION`, `GH_ARCH`). `getKiroCLIDownloadURL` keeps the platform→zip mapping and takes the pinned version. **The builder must verify** the versioned path `https://desktop-release.q.us-east-1.amazonaws.com/<version>/kirocli-<arch>-linux-musl.zip` with `curl -fsI`; if only `latest/` exists, keep `latest` in the URL but still hash the `KiroCLIVersion` string and document that bumping the string is the cache-buster (an upstream change to `latest` is not detected — stated in docs).
- Tag: `kairon-eval-base:<platform with / → ->-<12 hex of sha256(Dockerfile bytes ‖ platform ‖ sorted build args)>`. The tag begins with the existing `ImageNamePrefix` (`kairon-eval`), so `CreateWithPlatform` never tries to pull it. The hash covers **only** the Dockerfile and the tool pins — not the evals dir, agents, skills, cases or cwd — which is what makes AC 8 true.
- `ImageManager.EnsureBaseImage(ctx, platform) (tag string, built bool, err error)`: `ImageInspect(tag)` → reuse; else build once (mutex-guarded, `buildImageDirect` with `BuildArgs`) and return `built=true`. The image is **persistent**: it is not removed at end of run (the old evaluation-scoped images and `ImageManager.Cleanup` removal are deleted). `Run()` calls it once before the cases and prints `✅ Base image reused: <tag>` or `🔨 Base image built: <tag>`. A tag is immutable, so a bumped pin yields a new tag and a rebuild; the old image remains until `docker image prune`.
- Build needs network (documented, unchanged from today). `NetworkMode: none` applies to *containers*, not to the build.

### 2.2 Host-side per-case workspace (shared by native and container)

New `internal/eval/workspace.go`:

```go
type caseWorkspace struct {
    Dir     string // host path of the workspace root (a git repo)
    EvalDir string // Dir/.eval  (outputs)
    KiroDir string // Dir/.kiro  (staged agent/skill config)
}
func newCaseWorkspace(tc TestCase) (*caseWorkspace, error)
func (w *caseWorkspace) Remove() error // best effort; never fails the run
```

`newCaseWorkspace` (all host operations, run as the harness user):

1. `root := os.MkdirTemp($KAIRON_EVAL_WORKSPACE_ROOT or os.TempDir(), "kairon-eval-ws-")`, then `filepath.EvalSymlinks` (macOS `/var` → `/private/var`; Docker Desktop and Podman machine only share real paths). The env var exists because a user's `TMPDIR` may not be a shared mount.
2. Copy `evalsPath("fixtures","workspaces",tc.Workspace)` into it (regular files and directories; symlinks skipped). `tc.Workspace == ""` → empty workspace. A named fixture that is missing is an error naming the path. `Workspace` must match `^[A-Za-z0-9._-]+$` and not be `.`/`..` (validated in `loadCases`).
3. `git init -b main` then one commit containing the fixture (`--allow-empty` for the default empty fixture), with identity and hooks forced via `-c user.name=kairon-eval -c user.email=eval@kairon.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null` and `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null`. **This is the only commit; its tree is the fixture** (AC 3).
4. Append `.eval/` and `.kiro/` to `.git/info/exclude` (not to a tracked file, so the fixture tree is untouched). `git status --porcelain` therefore never lists harness-owned paths. Files a fixture *tracks* under `.kiro/` stay tracked; exclude only hides untracked ones.
5. Stage `.kiro/` **on the host, inside the workspace**, with precedence *fixture > `<evals-dir>/agents/` > project `.kiro/`*: copy the project's `.kiro/` (cwd-relative, i.e. what `kairon init` produced, including agents, skills and MCP config) without overwriting fixture-provided files; copy `<evals-dir>/agents/*` into `.kiro/agents/` overriding project files but not fixture files. Staging into the workspace (rather than mounting the live `.kiro/`) is deliberate: the overlay agent configs must be merged in, a fixture may supply override hooks (`*-conventions`), `file://` / `skill://` resources keep resolving relative to the same layout, and the live repo's `.kiro/` is never exposed to the agent.
6. `mkdir .eval`.
7. Make the tree usable by the unprivileged container user whatever its host owner: `chmod a+rwX` on every directory and file under the workspace including `.git` (git does not track mode bits other than `x`, so this causes no status noise), except `.kiro/` which gets `a+rX` (it is mounted read-only).

`Remove()` runs `os.RemoveAll`; on failure it prints a warning with the path and returns nil (rootless Podman can leave subuid-owned files; the umask wrapper in §2.4 prevents it in practice).

### 2.3 Mounts (container path)

`sandbox.Mount{HostPath, ContainerPath string; ReadOnly bool}` and `sandbox.NewHostConfigWithMounts(limits, mounts)`:

| Host | Container | Mode |
|---|---|---|
| `<ws>/.kiro` | `<WorkspaceDir>/.kiro` | **ro** |
| `<ws>` | `<WorkspaceDir>` (`sandbox.workspace_dir`, default `/workspace`) | rw |
| `<ws>/.eval` | `<WorkspaceDir>/.eval` | rw |
| resolved linux kairon helper (stub and other non-kiro-cli backends only) | `/opt/kairon/kairon` | ro |

- Use the structured `container.HostConfig.Mounts` with `mount.TypeBind` and `BindOptions{CreateMountpoint: false}`, **not** `Binds` strings. `Binds` silently creates a missing host source as a root-owned directory; `Mounts` fails fast, which is what we want. Every `HostPath` must be absolute, exist, and be symlink-resolved before the container is created.
- The `/workspace` **tmpfs entry is removed** from the host config used by the runner (the new constructor never sets it). `.eval/` is a separate bind of a directory that physically lives under `<ws>` so the on-disk layout equals the native layout, while the container-side contract ("three explicit mounts") is what #298's read-only-root work builds on.
- The helper moves from a runtime `CopyTo` to a read-only bind. `ResolveLinuxBinary` is unchanged; its result must be `0755`-readable by uid 1000 or the harness returns an error naming the file.
- Container `Config`: `User: "sandbox"` (explicit guard), `WorkingDir: <WorkspaceDir>`, `Env` adds `HOME=/home/sandbox` and `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0=*`. The last one is required: the mounted repo is owned by a different uid than `sandbox`, and git refuses it ("dubious ownership") otherwise. It is environment, not a file operation.
- SELinux: when the host reports enforcing (`/sys/fs/selinux/enforce` == `1`), add `SecurityOpt: ["label=disable"]` for the run. This avoids relabeling the user's directories with `:z`/`:Z`. It is skipped everywhere else. Documented as a limit that #298 may revisit.

### 2.4 Ownership and cleanup contract

Mounted directories are owned by the harness user, the container runs as uid 1000. Rules that make both sides work on Linux Docker (rootful), rootless Podman and Docker Desktop/Podman machine (macOS):

- The host builder grants world read/write (`a+rwX`) before the container starts (§2.2.7).
- Every command the harness execs in the container is wrapped as `sh -c 'umask 000; exec "$@"' kairon-exec <cmd…>` (`sandbox.WithOpenUmask(cmd)`), so files and directories the agent creates are world-accessible and the host user can read them for scoring and delete them afterward even when the container uid maps to a foreign host uid. The wrapper is applied to the kiro-cli exec and to the helper exec; it is a process wrapper, not a content install.
- Host-side removal never fails the run (§2.2).

### 2.5 Runner wiring

One shared per-case function replaces the duplicated bodies of `evaluate` and `evaluateProgressive`:

```go
// executeCase: workspace → invoke → score (workspace still present) → keep or remove.
func executeCase(rubric Rubric, tc TestCase, cConfig *ContainerConfig, out io.Writer, keep bool) CaseResult
```

- Order is load-bearing: the workspace exists while `scoreCase` runs (E5/E6 read it), and is removed afterward unless `--keep-workspaces`. `CaseResult.WorkspaceDir` (`json:"workspace_dir,omitempty"`) is set **before** scoring and recorded whether or not the directory is later removed (AC 6).
- `invokeAgent(agent, prompt, cConfig, opts callOpts)` where `callOpts{Stub *inference.StubScript; Workspace *caseWorkspace; Timeout time.Duration}`. `perf.go` and tests are updated to the new signature (`callOpts{Stub: …}`).
- Native: `req.WorkDir = ws.Dir`, `req.AgentConfigDir = ""` (the staged `.kiro/agents` already holds the overlay). The kiro-cli backend sets `cmd.Dir = req.WorkDir` when non-empty; the existing `applyAgentConfigOverlay` temp-dir path is used only when `WorkDir` is empty (unchanged for direct callers).
- Container: `req.WorkDir = cConfig.WorkspaceDir` (the container-side path). `invokeAgentInContainer` takes the workspace, builds the §2.3 mounts, and keeps the existing read-only `ValidateKiroCLI` check for the kiro-cli backend (a check, not an install). **Removed from the container branch:** `SetupGitHubMocking`, `ConfigureMockGitHubPath`, `CopyTo` of the helper. `agentExecer` loses `CopyTo`, so `runAgentInContainer` cannot copy anything even by mistake (compile-time proof for AC 2). `ContainerConfig.MockGitHub` stays as an unconsulted field with a comment pointing to #298.
- Image: `cConfig.CachedImageName` is set once in `Run()` from `EnsureBaseImage`. The per-call fallback build is replaced by the same call (still cached). No image is removed after a call.
- `RunOptions.KeepWorkspaces` + `--keep-workspaces` flag in `cmd/kairon/cmd/eval.go`.
- The single-case and resume paths (`runSingleTestCase`, `runWithResume`) go through `executeCase` too, so `workspace_dir` and keep behave the same everywhere. They remain native (as today: `willContainerize` is false for them).

### 2.6 Case `timeout` (AC 7)

- `TestCase.Timeout string` (`yaml:"timeout,omitempty"`), parsed in `loadCases` as a `time.ParseDuration` > 0; an invalid value is a load error naming the case.
- Effective request timeout: `tc.Timeout` when set; otherwise unchanged (native: `KAIRON_EVAL_TIMEOUT` or 2m; container: `ResourceLimits.Timeout`). It is written to `req.Timeout`, so the in-container backend and the host exec agree.
- Container host exec deadline: `req.Timeout` for kiro-cli; `req.Timeout + 10s` for helper-based backends, so the in-container backend normally reports the timeout itself (an `ErrTimeout`-wrapped envelope from `DecodeExecResult`) and the host deadline is only a backstop. `mapContainerError` is given the effective timeout and its message names the case `timeout` as well as `--resource-limit timeout=` (today it prints `cConfig.ResourceLimits.Timeout`, which would say "5m0s" for a 1s case).
- Stub backend: runs `turn.Commands` (see §2.7) under `context.WithTimeout(ctx, timeoutOrDefault(req.Timeout))`, using `exec.CommandContext` with its own process group and `cmd.WaitDelay = time.Second`, and kills the group on cancel. Without that, a killed `sh` leaves `sleep 3` holding the output pipe and `Wait` blocks for the full 3 s. A deadline returns a wrapped `ErrTimeout` (`"stub timeout after <d>"`), so `completeAgentCall` renders `timeout after 1s` in `ErrorContext.Stderr` exactly as for kiro-cli. Process-group kill is Unix-only; a `_unix.go`/`_other.go` pair (build tags) keeps the package compiling elsewhere.
- Recorded as: `ActualOutput == ""`, `ErrorContext` non-nil with `timeout after 1s`, `errors.Is(err, inference.ErrTimeout)`, the case scored as a failure (existing "no output" path).

### 2.7 Stub `commands`

`inference.StubTurn.Commands []string` (`yaml:"commands,omitempty" json:"commands,omitempty"`). `inference.Request.WorkDir string` (`json:"WorkDir"`, same Go-name-as-key convention as the rest of `Request`). The stub's `RoleAgent` branch, before returning the turn's response, runs each command with `sh -c <cmd>` in `req.WorkDir` (required when commands exist; empty `WorkDir` + commands is an error so a test cannot accidentally write into the repo root). A non-zero exit returns an error carrying the command and its stderr (`Response.Stderr`, `ExitCode`); the response is not returned. In the container the same code runs through `inference-exec` (#296) as `sandbox`, in `/workspace`, with the open umask.

### 2.8 Removed legacy code

After the runner no longer calls it: `Container.GenerateDockerfileWithPlatform`, `loadTemplate`, `addKiroCLIToDockerfile` (its URL logic moves to the base-image file), `ProjectInfo`/`DetectProject` (`project_detector.go`), `internal/eval/dockerfile/templates/*.Dockerfile`, `SetupGitHubMocking`, `copyContentToContainer`, `ConfigureMockGitHubPath`, `ImageManager.BuildForEvaluation`/removal-on-cleanup, `GetCustomImageName`, and the tests that exercised them. Behaviors worth keeping are **ported, not dropped**: platform→kiro-cli URL (incl. unsupported platform error), `DetectHostArchitecture`, daemon-skip pattern. `SimulateGitHubResponse`, `GitHubMockResponse` and the embedded `testdata/github-cli-mock` are left in place for #298 to reuse or replace. Deleting project toolchain detection is a deliberate consequence of AC 1 (stable tools only): toolchains for a consuming project's evals are not baked. Documented as a limitation; extending the tool set is a ToolSet change with a rebuild.

### Out of scope (unchanged)
Fake `gh` behavior and `gh_issue`, network policy, read-only root FS, tool trust (#298); provenance, `sandbox` mode field in `summary.json`, `eval diff` (#300); E5–E7 check types; kiro-cli authentication inside the container (the repo has no mechanism today and this issue does not add one — a real `kiro-cli` sandbox run still needs credentials supplied through `ContainerConfig.Environment`).

## 3. Relevant Files

Create:
- `internal/eval/dockerfile/embed.go` — `Base` (embedded Dockerfile).
- `internal/eval/sandbox/baseimage.go` (+ `baseimage_test.go`, `baseimage_daemon_test.go`) — `ToolSet`, `DefaultToolSet`, `BuildArgs`, `BaseImageTag`, `EnsureBaseImage`.
- `internal/eval/sandbox/mounts.go` (+ test) — `Mount`, `NewHostConfigWithMounts`, `WithOpenUmask`, SELinux probe, path validation.
- `internal/eval/workspace.go` (+ `workspace_test.go`) — `caseWorkspace`.
- `internal/eval/execute_case.go` (+ test) — `executeCase`, `callOpts`.
- `internal/inference/stubcmd_unix.go` / `stubcmd_other.go` — process-group handling for stub commands.
- `internal/eval/testdata/evals/cases/selftest/stub-marker.yaml`, `stub-seeded-workspace.yaml`
- `internal/eval/testdata/evals/fixtures/workspaces/seeded/` (a README and one nested file)
- `internal/eval/testdata/evals/agents/selftest-fail.json` (+ `selftest-fail-prompt.md`), `rubrics/selftest-fail.yaml`, `cases/selftest-fail/stub-timeout.yaml`

Modify:
- `internal/eval/dockerfile/base.Dockerfile` — rewritten (§2.1).
- `internal/inference/inference.go`, `stub.go`, `kirocli.go` — `WorkDir`, `StubTurn.Commands`, stub command execution/timeout, `cmd.Dir`.
- `internal/eval/types.go` — `TestCase.Workspace`/`Timeout`, `CaseResult.WorkspaceDir`, `RunOptions.KeepWorkspaces`, comment on `ContainerConfig.MockGitHub`.
- `internal/eval/runner.go` — `invokeAgent`, `evaluate`/`evaluateProgressive` → `executeCase`, `Run()` image prep, `invokeAgentInContainer`, `runAgentInContainer`, `agentExecer`, `mapContainerError`, `loadCases` validation, single-case/resume.
- `internal/eval/perf.go` — new `invokeAgent` signature.
- `internal/eval/sandbox/container.go`, `image_manager.go`, `resource_limits.go`, `mock_github.go`, `project_detector.go` — additions then removals per §2.8.
- `cmd/kairon/cmd/eval.go` — `--keep-workspaces`.
- `Taskfile.yml` — `sync:check` excludes `workspaces` and `hidden` under `.kairon/evals/fixtures/`; `eval:selftest:sandbox` also runs the base-image test.
- `.kiro/skills/builder-conventions/SKILL.md` — the fixtures sync command copies files only (`cp .kairon/evals/fixtures/*` fails on a subdirectory). Live, not template-synced (`*-conventions`), edit freely.
- `internal/eval/selftest_test.go`, `selftest_sandbox_test.go`, `runner_config_test.go`, `performance_test.go`, `architecture_compatibility_test.go`, and the sandbox tests listed in §2.8 — updated to the new APIs.
- `docs/evaluation.md` (and `docs/unified-container-flow.md` if it describes the removed flow).

Delete: `internal/eval/dockerfile/templates/*`, `internal/eval/sandbox/project_detector.go` (and tests) once unreferenced.

Not touched: `cmd/kairon/templates/**` (template-synced content is unchanged; `internal/eval/testdata/**` and `.kairon/evals/fixtures/workspaces/` are not synced). `.kairon/specs/**` other than this file.

Read-only references: `internal/eval/sandbox/linuxbin.go`, `container_daemon_test.go` (`skipIfNoContainerDaemon`), `internal/inference/exec.go`, `.kairon/specs/maturity-model/gap-analysis.md` (E4 section for names), `.kairon/specs/issue-296-*.md`, `issue-290-*.md`.

## 4. Key Interfaces

```go
// internal/eval/sandbox
type ToolSet struct{ KiroCLIVersion, GHVersion string }
var DefaultToolSet = ToolSet{ /* pinned */ }
func (t ToolSet) BuildArgs(platform string) (map[string]*string, error)
func BaseImageTag(platform string, dockerfile string, args map[string]*string) string // pure
func (im *ImageManager) EnsureBaseImage(ctx context.Context, platform string) (tag string, built bool, err error)

type Mount struct{ HostPath, ContainerPath string; ReadOnly bool }
func NewHostConfigWithMounts(limits ResourceLimits, mounts []Mount) (*container.HostConfig, error) // validates, no /workspace tmpfs
func WithOpenUmask(cmd []string) []string

// internal/inference
type StubTurn struct { /* existing */ Commands []string }
type Request struct { /* existing */ WorkDir string }

// internal/eval
type callOpts struct { Stub *inference.StubScript; Workspace *caseWorkspace; Timeout time.Duration }
func invokeAgent(agent, prompt string, cConfig *ContainerConfig, opts callOpts) (string, CostInfo, inference.CallRecord, *ErrorContext, error)
type agentExecer interface { // CopyTo removed
    ExecWithStdin(ctx context.Context, cmd []string, stdin io.Reader) (sandbox.ExecResult, error)
}
```

## 5. Team Orchestration

```
stub-workdir-commands ──┐
case-workspace ─────────┼─> native-workspace-wiring ──┐
                        │                             ├─> container-runner ─> remove-legacy ─> selftest-and-container-tests ─> update-docs ─> validate-all
sandbox-base-image ─> sandbox-mounts ─────────────────┘
```

- `stub-workdir-commands` (`internal/inference`), `case-workspace` (`internal/eval` new files + `types.go` + `loadCases`) and `sandbox-base-image` (`internal/eval/sandbox` additions) touch disjoint files and run in parallel. All three are **additive**, so each package still builds and tests while the others are in flight.
- `sandbox-mounts` edits files in `internal/eval/sandbox` that `sandbox-base-image` also touches, so it runs after it.
- `native-workspace-wiring` and `container-runner` both edit `runner.go`, so they are sequential; `container-runner` needs the mounts and the native wiring.
- Deletions of legacy code happen only in `remove-legacy`, after the runner has stopped calling it, so no intermediate state fails to compile.
- Tests that need a container daemon skip via `skipIfNoContainerDaemon` (#290). The image build and container tests are additionally gated on `KAIRON_EVAL_SANDBOX_SELFTEST=1` (the existing gate) so `task test` never downloads kiro-cli.

All work lands in one PR.

## 6. Step-by-Step Task Breakdown

### Task 1 — `stub-workdir-commands` (builder)
`inference.Request.WorkDir`, `StubTurn.Commands`; stub runs commands (§2.7) with timeout/process-group kill (§2.6); kiro-cli backend sets `cmd.Dir = WorkDir`. Tests: command creates `marker.txt` in `WorkDir`; failing command → error with stderr and no response; empty `WorkDir` with commands → error; `sleep 3` with `Timeout: 1s` returns `ErrTimeout` in under 2 s; JSON round-trip keeps `WorkDir` and `commands`; `kirocli_test.go` unchanged and green (add a `WorkDir` case).

### Task 2 — `case-workspace` (builder)
`TestCase.Workspace`/`Timeout`, `loadCases` validation, `CaseResult.WorkspaceDir`, `RunOptions.KeepWorkspaces`; `workspace.go` per §2.2 with the permission step. Tests (real `git`, no daemon): default → one empty commit; named fixture → exactly one commit whose tree equals the fixture; `.eval/` and untracked `.kiro/` absent from `git status --porcelain`; staging precedence (fixture > evals-dir agents > project `.kiro`) with a fixture override of one agent file; project `.kiro/` is not modified; invalid/missing/escaping names rejected; modes include `a+rw` on `.git` and workspace files; `Remove` deletes; unreadable-dir removal does not error; `KAIRON_EVAL_WORKSPACE_ROOT` honored; returned path is symlink-resolved.

### Task 3 — `sandbox-base-image` (builder)
Rewrite `base.Dockerfile`, add `embed.go`, `ToolSet`/`BuildArgs`/`BaseImageTag`/`EnsureBaseImage`, platform URL logic with pinned versions (verify URLs, §2.1). Keep the old generator for now. Unit tests (no daemon): tag differs when `GHVersion`, `KiroCLIVersion`, platform or Dockerfile bytes change; tag is identical regardless of cwd, env, and evals directory contents (the AC 8 invariant); Dockerfile static assertions (no `COPY`/`ADD`, no `.kiro`, `USER sandbox`, `-u 1000`, installs kiro-cli and gh, no toolchain installs); unsupported platform errors; tag has the `kairon-eval` prefix. Gated test: `EnsureBaseImage` twice → the second call returns `built=false` with the same tag and image ID.

### Task 4 — `sandbox-mounts` (builder, after Task 3)
`mounts.go` per §2.3/§2.4: `Mount`, `NewHostConfigWithMounts` (structured `Mounts`, `CreateMountpoint:false`, no `/workspace` tmpfs, limits and `NetworkMode: none` preserved, `User`/env/`label=disable` helpers), `WithOpenUmask`, path validation (absolute, exists, symlink-resolved, not the filesystem root). Tests: ro/rw flags, no tmpfs at the workspace path, missing host path rejected, relative path rejected, umask wrapper argv shape and that `"$@"` preserves arguments containing spaces/quotes, SELinux probe pure function.

### Task 5 — `native-workspace-wiring` (builder, after Tasks 1 and 2)
`executeCase` (§2.5) used by `evaluate`, `evaluateProgressive`, single-case and resume; `callOpts`; `perf.go`; `--keep-workspaces`; native `WorkDir`; `Timeout` precedence; keep/remove semantics; update existing tests for the new signature. Tests: `commands: ["echo hi > marker.txt"]` stub case natively leaves `marker.txt` in the kept workspace and not in the repo root; without keep the recorded dir is gone, with keep it exists and holds `.eval/`; a seeded fixture shows `M README.md` after the stub appends to it; `selftest-fail`-style timeout case records a timeout failure in ~1 s natively; scoring sees the workspace (a test scorer hook reads `cr.WorkspaceDir` and asserts it exists during `scoreCase`).

### Task 6 — `container-runner` (builder, after Tasks 4 and 5)
Rewrite the container branch per §2.5: image from `EnsureBaseImage` in `Run()` (print reused/built), mounts from the case workspace, helper as a read-only mount, remove mocking/`CopyTo`, drop `CopyTo` from `agentExecer`, open-umask wrapper on execs, env additions, `Timeout` rules and `mapContainerError` message (§2.6), `req.WorkDir`. Update the #296 `container_agent_test.go` fake and any test referencing the removed calls. Unit tests with the fake executor: no `CopyTo` exists; argv is umask-wrapped and contains no prompt; kiro-cli argv still equals the native argv after unwrapping; host exec deadline = timeout (kiro-cli) / timeout+10 s (helper); timeout message names the effective timeout (`1s`, not the sandbox default); mount list for kiro-cli contains no helper mount and for stub contains it at `/opt/kairon/kairon` ro; a source-level guard test fails if `runner.go` contains `SetupGitHubMocking`, `ConfigureMockGitHubPath` or `.CopyTo(`.

### Task 7 — `remove-legacy` (builder, after Task 6)
Delete the code and tests in §2.8; port the kept behaviors' tests; `Taskfile.yml` `sync:check` excludes; `builder-conventions` skill fixture-sync command; confirm `grep -rn 'GenerateDockerfileWithPlatform\|DetectProject\|SetupGitHubMocking\|ConfigureMockGitHubPath\|BuildForEvaluation' --include=*.go .` is empty; `go vet ./...`.

### Task 8 — `selftest-and-container-tests` (builder, after Task 7)
Test data (§3 Create), updates to `assertSelfTestResults` (case count now 5: `stub-basic`, `stub-usage`, `stub-quoted-input`, `stub-marker`, `stub-seeded-workspace`) and `TestSelftestSandbox`, plus:
- Sandbox-package gated test (AC 1): container from the base image with **no mounts** → `id -un` is `sandbox`; `command -v kiro-cli gh git sh` all succeed; `ls -A /workspace` is empty; `/workspace/.kiro` absent; no `.kairon`/agent config anywhere under `/workspace`, `/home/sandbox`, `/`-level project paths; `gh --version` works.
- Eval-level gated test (`TestSandboxWorkspace`, AC 2–7): run `stub-marker`, `stub-seeded-workspace` and `selftest-fail` both native and `--sandbox` with keep, and assert: no `Permission denied` anywhere in results; marker file content `hi\n` on the host; native/sandbox host trees (excluding `.git`, `.kiro`) and `git status --porcelain` identical; `.eval/` exists in both; repo-root `git status --porcelain` identical before and after; without keep the recorded dir is gone, with keep it exists and holds `.eval/`; the `selftest-fail` timeout case records a timeout failure (`ErrorContext.Stderr` contains `timeout after 1s`) in under ~30 s total including container start; editing an agent config and a case in a copy of the evals dir and re-running reuses the same base image ID and does not build.
- `Taskfile.yml` `eval:selftest:sandbox` runs both gated tests.
- If a daemon is available run `task eval:selftest:sandbox` and record the result; otherwise record that the container run could not be executed here.

### Task 9 — `update-docs` (documenter, after Task 8)
`docs/evaluation.md`: new "Sandbox layering" section (base image contents and what is deliberately absent; the three mounts + helper mount; staging precedence; workspace fixture format `fixtures/workspaces/<name>/`; case fields `workspace`, `timeout`; stub `commands`; `.eval/`; `--keep-workspaces` and `workspace_dir`; ownership/umask contract; SELinux/Podman/macOS notes and `KAIRON_EVAL_WORKSPACE_ROOT`; base-image cache key and when it rebuilds, `ToolSet` bump procedure, `latest` caveat if applicable; build needs network); rewrite the stale "Container Sandboxing" subsections (automatic project toolchain install, mocked `gh` at runtime, the Dockerfile example, the `HOME`/`/workspace` description) and the "native runs execute in the repo root" assumptions; note that `gh` is present but unconfigured until #298; update the self-test section for the new cases and `selftest-fail`. Check `docs/unified-container-flow.md` and `README.md` for statements made false by this change. No `.kairon/specs/**` edits other than this file.

### Task 10 — `validate-all` (validator, after Task 9)
Read-only verification of AC 1–8 against the code and the test evidence, plus `task test`, `task lint`, `task fmt:check`, `task sync:check` (including with a throw-away fixture under `.kairon/evals/fixtures/workspaces/`, removed afterwards).

## 7. Acceptance-Criteria Traceability

| Issue AC | Satisfied by |
|---|---|
| 1 base image tools only | §2.1; Task 3 static test; Task 8 AC-1 container test |
| 2 no runtime install; `.kiro` ro, workspace/.eval rw; no Permission denied | §2.3–2.5: bind mounts only, `agentExecer` without `CopyTo`, mocking removed; Task 6 source guard; Task 8 `Permission denied` assertion |
| 3 host-built git workspace, first commit = fixture, marker.txt | §2.2; Tasks 2, 5, 8 |
| 4 live repo root untouched | cwd never used as workspace; Task 8 porcelain before/after |
| 5 workspace incl. `.eval/` on host, same layout as native | §2.5 identity by bind mount, shared `newCaseWorkspace`; Task 8 tree/status comparison (E5 checks not built — §2.0) |
| 6 `--keep-workspaces` / `workspace_dir` | §2.5; Tasks 5, 8 |
| 7 case `timeout` under `--sandbox` | §2.6, §2.7; Tasks 1, 6, 8 |
| 8 base image reuse / rebuild | §2.1 tag = f(Dockerfile, pins, platform); Task 3 unit test; Task 8 reuse test |
| Constraint: sync:check | workspaces excluded in Taskfile; no template changes; Task 7, Task 10 |
| Constraint: daemon tests skip | `skipIfNoContainerDaemon` + env gate (Tasks 3, 8) |

## 8. Risks and Notes

- **Ownership across runtimes is the hard part.** `a+rwX` plus the open umask is the portable answer; it makes workspace contents world-writable on a shared multi-user host while a run is in flight (temp dir created `0700` at the root by `MkdirTemp`, so other local users cannot traverse into it). Not verified in this session (no daemon was run); the gated tests cover Docker and rootless Podman when available.
- **Native behavior change** (§2.0). Reviewers should look at the kiro-cli `cmd.Dir` change and the `AgentConfigDir` hand-off together.
- **`.kiro/` is excluded from `git status`** (untracked, harness-owned, read-only to the agent). A future `changed_files` check therefore cannot see edits to untracked `.kiro` files natively; in the container the mount is read-only so it cannot happen.
- **Pinned tool URLs** must be verified by the builder (network). If a versioned kiro-cli URL does not exist the pin degrades to a cache-buster string, which is documented.
- **kiro-cli auth** inside the container is untouched (out of scope), as are fake `gh`, network, read-only root FS and tool trust (#298).
- **`/tmp` is no longer used** by the harness inside the container (helper is mounted), which unblocks a read-only root in #298.
- **Project toolchains are no longer baked** — deliberate (AC 1) and documented.
- Debug mode preserves failed containers but not their workspaces; use `--keep-workspaces` together with `--debug` to inspect both.

## 9. Validation Commands

```bash
go build ./... && go vet ./...
go test ./internal/inference/... ./internal/eval/... ./cmd/... -count=1
task test && task lint && task fmt:check && task sync:check
go run ./cmd/kairon plan parse .kairon/specs/issue-297-sandbox-base-image-and-mounts.md
# Native marker case with a kept workspace:
go run ./cmd/kairon eval --backend stub --no-sandbox --keep-workspaces --evals-dir internal/eval/testdata/evals selftest
# Needs Podman or Docker (+ network for the first base-image build):
task eval:selftest:sandbox
go run ./cmd/kairon eval --backend stub --sandbox --keep-workspaces --evals-dir internal/eval/testdata/evals selftest
git status --porcelain   # unchanged by the runs above
grep -rn 'GenerateDockerfileWithPlatform\|DetectProject\|SetupGitHubMocking\|ConfigureMockGitHubPath' --include=*.go .   # no matches
```

## 10. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "stub-workdir-commands"
    agent: "builder"
    description: "In internal/inference add Request.WorkDir and StubTurn.Commands. The stub backend runs each command with 'sh -c' in WorkDir before returning the turn response, under a context bounded by the request timeout, in its own process group so a timed-out command (for example 'sleep 3') is killed promptly, returning an ErrTimeout-wrapped error. A failing command returns an error with its stderr and exit code. The kiro-cli backend sets cmd.Dir to WorkDir when non-empty. Add unit tests."
    dependencies: []
    acceptance_criteria:
      - "StubTurn has Commands []string with yaml/json key 'commands'; Request has WorkDir with JSON key 'WorkDir'; both survive a JSON round trip (the inference-exec wire format)"
      - "A stub turn with commands ['echo hi > marker.txt'] and WorkDir set creates marker.txt containing 'hi' in WorkDir and returns the scripted response"
      - "Commands with an empty WorkDir return an error and run nothing"
      - "A command exiting non-zero returns an error that contains the command and its stderr, and no response text"
      - "A turn whose command is 'sleep 3' with Request.Timeout of 1s returns an error satisfying errors.Is(err, ErrTimeout) in under 2 seconds"
      - "The kiro-cli backend runs with cmd.Dir equal to WorkDir when WorkDir is non-empty; the applyAgentConfigOverlay behavior is unchanged when WorkDir is empty; all existing tests in internal/inference pass unmodified"
      - "The package builds on non-Unix GOOS through a build-tagged fallback for process-group handling"
    validation_commands:
      - "go build ./internal/inference/..."
      - "GOOS=windows go build ./internal/inference/..."
      - "go vet ./internal/inference/..."
      - "go test ./internal/inference/... -count=1"
      - "test -z \"$(gofmt -l internal/inference)\""

  - id: "case-workspace"
    agent: "builder"
    description: "Add TestCase.Workspace and TestCase.Timeout (validated in loadCases), CaseResult.WorkspaceDir, RunOptions.KeepWorkspaces, and internal/eval/workspace.go implementing the host-side per-case workspace: temp dir (KAIRON_EVAL_WORKSPACE_ROOT or os.TempDir, symlink-resolved), copy of fixtures/workspaces/<name>/ (default empty), git init with a single hermetic commit of the fixture, .eval/ and .kiro/ added to .git/info/exclude, host-side staging of .kiro with precedence fixture > evals-dir agents > project .kiro, mkdir .eval, a+rwX permissions (a+rX on .kiro), and a best-effort Remove. Add unit tests that use real git and need no container daemon."
    dependencies: []
    acceptance_criteria:
      - "A case without a workspace gets a git repo with exactly one (empty) commit; a case naming a fixture gets exactly one commit whose tree equals the fixture directory"
      - "Workspace names that are empty-invalid, contain path separators, are '.' or '..', or name a missing fixture are rejected with an error naming the case or path"
      - "A timeout value that is not a positive Go duration is a loadCases error naming the case"
      - ".eval/ exists and neither .eval/ nor harness-staged .kiro/ appears in 'git status --porcelain'; files the fixture tracks under .kiro stay tracked"
      - "Staged .kiro contains the project's .kiro content (agents, skills, MCP config) and <evals-dir>/agents overrides project agents, and a fixture-provided file wins over both; the project's own .kiro directory is not modified"
      - "Every directory and file in the workspace including .git is world read/write (a+rwX) and git commit still works as another uid would see it; .kiro is world read-only"
      - "The returned Dir is absolute and symlink-resolved; KAIRON_EVAL_WORKSPACE_ROOT relocates it; Remove deletes the tree and never returns an error that fails a run"
      - "CaseResult.WorkspaceDir has JSON key workspace_dir and is omitted when empty; existing result files still decode"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -count=1"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "sandbox-base-image"
    agent: "builder"
    description: "Rewrite internal/eval/dockerfile/base.Dockerfile as the tools-only base image (alpine, git, bash, ca-certificates, pinned gh, pinned kiro-cli, sandbox user uid 1000, empty /workspace owned by sandbox, smoke test of the tools, USER sandbox, no COPY/ADD, no toolchains) and embed it from internal/eval/dockerfile/embed.go. In internal/eval/sandbox add ToolSet/DefaultToolSet/BuildArgs, the pure BaseImageTag (hash of Dockerfile bytes, platform and build args only), and ImageManager.EnsureBaseImage which reuses an existing tag via ImageInspect or builds once and reports whether it built. Verify the pinned download URLs with curl. Keep the old generator in place (removed later). Add unit tests and a daemon-skipping gated test."
    dependencies: []
    acceptance_criteria:
      - "base.Dockerfile contains no COPY or ADD, no reference to .kiro, agents, skills or cases, creates user sandbox with uid 1000, ends with USER sandbox, and installs kiro-cli, gh and git; no go/node/python/rust/java toolchain is installed"
      - "BaseImageTag is a pure function; its result changes when GHVersion, KiroCLIVersion, platform or the Dockerfile bytes change and is identical across different working directories, environment variables and evals directory contents"
      - "The tag starts with the existing ImageNamePrefix 'kairon-eval' so CreateWithPlatform never attempts a pull"
      - "Unsupported platforms return an error; linux/amd64 and linux/arm64 produce valid kiro-cli and gh build args"
      - "EnsureBaseImage returns built=false when the tag already exists and never removes the image; a daemon-gated test (skipIfNoContainerDaemon plus KAIRON_EVAL_SANDBOX_SELFTEST=1) shows the second of two consecutive calls returns built=false with the same tag and image ID as the first"
      - "Pinned kiro-cli and gh URLs were verified (HTTP success) and the result, including whether the kiro-cli URL is versioned or 'latest', is recorded in a code comment next to ToolSet"
      - "Existing sandbox tests still compile and pass or skip"
    validation_commands:
      - "go build ./internal/eval/..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/sandbox/... -count=1"
      - "! grep -nE '^(COPY|ADD) ' internal/eval/dockerfile/base.Dockerfile"
      - "grep -n 'USER sandbox' internal/eval/dockerfile/base.Dockerfile"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "sandbox-mounts"
    agent: "builder"
    description: "Add internal/eval/sandbox/mounts.go: the Mount type, NewHostConfigWithMounts (structured HostConfig.Mounts of type bind with CreateMountpoint false, read-only flag support, resource limits and NetworkMode none preserved, no tmpfs at the workspace path), host path validation (absolute, exists, symlink-resolved), WithOpenUmask wrapper, and an SELinux-enforcing probe (pure decision function) that adds SecurityOpt label=disable only on enforcing hosts. Keep NewHostConfigWithLimits until the runner switches. Add unit tests."
    dependencies: ["sandbox-base-image"]
    acceptance_criteria:
      - "NewHostConfigWithMounts returns bind mounts with the requested ReadOnly flags and CreateMountpoint false, and its host config has no Tmpfs entry for the workspace path"
      - "A relative, missing or filesystem-root host path is rejected before any container is created, with an error naming the path"
      - "WithOpenUmask returns ['sh','-c','umask 000; exec \"$@\"','kairon-exec', ...cmd] and preserves arguments containing spaces, quotes and newlines when executed by sh"
      - "label=disable is added only when the pure SELinux decision says enforcing"
      - "CPU, memory and NetworkMode none from ResourceLimits are applied exactly as before"
    validation_commands:
      - "go build ./internal/eval/sandbox/..."
      - "go vet ./internal/eval/sandbox/..."
      - "go test ./internal/eval/sandbox/... -count=1"
      - "test -z \"$(gofmt -l internal/eval/sandbox)\""

  - id: "native-workspace-wiring"
    agent: "builder"
    description: "Add executeCase and callOpts and use them from evaluate, evaluateProgressive, runSingleTestCase and the resume path: create the case workspace, invoke the agent (native: WorkDir set to the workspace and AgentConfigDir cleared), set CaseResult.WorkspaceDir before scoring, score while the workspace still exists, then remove the workspace unless --keep-workspaces. Apply the case timeout precedence to the request. Add the --keep-workspaces flag and RunOptions.KeepWorkspaces. Update perf.go and existing tests to the new invokeAgent signature."
    dependencies: ["stub-workdir-commands", "case-workspace"]
    acceptance_criteria:
      - "invokeAgent takes callOpts{Stub, Workspace, Timeout}; there is one per-case code path shared by evaluate, evaluateProgressive, single-case and resume"
      - "The workspace exists while scoreCase runs and CaseResult.WorkspaceDir is set before scoring and recorded even when the directory is later removed"
      - "Native run of a stub case with turn commands ['echo hi > marker.txt'] succeeds, leaves marker.txt in the kept workspace, and leaves the repository root untouched ('git status --porcelain' unchanged)"
      - "Without --keep-workspaces the recorded workspace_dir no longer exists after the run; with it the directory exists, is a git repo, and holds .eval/"
      - "A case with timeout 1s whose stub turn runs 'sleep 3' is recorded natively as a timeout failure (ErrTimeout, ErrorContext.Stderr contains 'timeout after 1s', empty actual_output) in under 2.5 seconds"
      - "A seeded fixture case shows its stub-modified file as ' M' in the kept workspace's git status and the git log has exactly one commit"
      - "kairon eval --keep-workspaces is accepted by the CLI and appears in 'kairon eval --help'"
      - "Existing eval tests pass after being updated for the signature change; perf.go compiles with callOpts"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./internal/eval/... ./internal/inference/... ./cmd/... -count=1"
      - "go run ./cmd/kairon eval --help | grep -q keep-workspaces"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "container-runner"
    agent: "builder"
    description: "Rewrite the container branch of internal/eval/runner.go: Run() obtains the base image once via EnsureBaseImage and prints reused or built; invokeAgentInContainer takes the case workspace, builds the read-only .kiro mount, the read-write workspace and .eval mounts and (for non-kiro-cli backends) a read-only helper mount at /opt/kairon/kairon via NewHostConfigWithMounts, sets User sandbox, HOME and the git safe.directory environment, wraps every exec with WithOpenUmask, sets req.WorkDir to the container workspace path, applies the case-timeout rules (host deadline equals the timeout for kiro-cli and timeout plus 10s for helper backends) and fixes mapContainerError to name the effective timeout. Remove SetupGitHubMocking, ConfigureMockGitHubPath and the helper CopyTo from the runner and drop CopyTo from agentExecer. Update the #296 tests and add the new unit tests with the fake executor."
    dependencies: ["sandbox-mounts", "native-workspace-wiring"]
    acceptance_criteria:
      - "runner.go contains no call to SetupGitHubMocking, ConfigureMockGitHubPath or CopyTo, and the agentExecer interface has only ExecWithStdin"
      - "The container is created from the cached base image with exactly these mounts: <ws>/.kiro read-only at <WorkspaceDir>/.kiro, <ws> read-write at <WorkspaceDir>, <ws>/.eval read-write at <WorkspaceDir>/.eval, plus the helper read-only at /opt/kairon/kairon only for non-kiro-cli backends; there is no tmpfs at the workspace path"
      - "No image is built or removed per call; Run() prints whether the base image was reused or built"
      - "Every in-container exec is wrapped with the open-umask wrapper; unwrapped, the kiro-cli argv equals the native argv and the prompt is still only on stdin"
      - "The host exec deadline is the effective timeout for kiro-cli and the effective timeout plus 10 seconds for helper backends; the timeout error message names the effective timeout (for example 1s) and still satisfies errors.Is(err, inference.ErrTimeout)"
      - "req.WorkDir is the container workspace path and the container Config sets User sandbox, WorkingDir, HOME and GIT_CONFIG_* safe.directory environment"
      - "A guard test fails if runner.go reintroduces SetupGitHubMocking, ConfigureMockGitHubPath or '.CopyTo('"
      - "All existing internal/eval tests pass after updates, and tests that need a daemon skip when none is reachable"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"
      - "! grep -nE 'SetupGitHubMocking|ConfigureMockGitHubPath|\\.CopyTo\\(' internal/eval/runner.go"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "remove-legacy"
    agent: "builder"
    description: "Delete the legacy sandbox code superseded by the base image and mounts: GenerateDockerfileWithPlatform, loadTemplate, addKiroCLIToDockerfile, project_detector.go, internal/eval/dockerfile/templates, SetupGitHubMocking, copyContentToContainer, ConfigureMockGitHubPath, ImageManager.BuildForEvaluation and removal-on-cleanup, GetCustomImageName and NewHostConfigWithLimits (if unreferenced), together with the tests that exercised them; port the tests for behavior that is kept (platform to kiro-cli URL mapping, unsupported platform error, host architecture detection). Update Taskfile.yml sync:check to exclude the workspaces and hidden fixture directories and fix the builder-conventions skill fixtures copy command so it copies files only."
    dependencies: ["container-runner"]
    acceptance_criteria:
      - "No Go file references GenerateDockerfileWithPlatform, DetectProject, SetupGitHubMocking, ConfigureMockGitHubPath or BuildForEvaluation, and internal/eval/dockerfile/templates no longer exists"
      - "SimulateGitHubResponse, GitHubMockResponse and testdata/github-cli-mock are left in place for the containment issue"
      - "Tests for the kept behaviors (platform URL mapping, unsupported platform error, DetectHostArchitecture) still exist and pass"
      - "task sync:check passes both without any workspaces fixture and with a throw-away directory .kairon/evals/fixtures/workspaces/tmp-check/README.md present (removed afterwards); nothing under cmd/kairon/templates is modified"
      - "The builder-conventions skill's fixtures sync command no longer fails when .kairon/evals/fixtures contains a subdirectory"
    validation_commands:
      - "go build ./... && go vet ./..."
      - "go test ./internal/eval/... -count=1"
      - "! grep -rn 'GenerateDockerfileWithPlatform\\|DetectProject\\|SetupGitHubMocking\\|ConfigureMockGitHubPath\\|BuildForEvaluation' --include=*.go ."
      - "test ! -d internal/eval/dockerfile/templates"
      - "task sync:check"
      - "git diff --quiet -- cmd/kairon/templates"

  - id: "selftest-and-container-tests"
    agent: "builder"
    description: "Add self-test data under internal/eval/testdata/evals: cases stub-marker (turn commands ['echo hi > marker.txt']) and stub-seeded-workspace (workspace seeded, fixture under fixtures/workspaces/seeded/, turn appends to a seeded file), and a separate agent selftest-fail (agent config, prompt, rubric) with case stub-timeout (timeout 1s, turn command 'sleep 3'). Update assertSelfTestResults and TestSelftestSandbox for the five selftest cases. Add the daemon-gated base-image test (container from the base image with no mounts) and a daemon-gated TestSandboxWorkspace that runs the new cases natively and under --sandbox with kept workspaces and compares trees, git status, .eval/ presence, repo-root cleanliness, keep/no-keep behavior, timeout recording and base-image reuse after editing an agent config and a case. Update the eval:selftest:sandbox Taskfile target to run the gated tests. Run them if a daemon is available."
    dependencies: ["remove-legacy"]
    acceptance_criteria:
      - "task eval:selftest still passes natively with 5 selftest cases and the stub-marker and stub-seeded-workspace cases pass their rubric; the selftest-fail agent can be run with 'kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail' and records the stub-timeout case as a timeout failure"
      - "Gated AC-1 test: a container from the base image with no mounts runs as 'sandbox', has kiro-cli, gh, git and sh on PATH, /workspace empty and /workspace/.kiro absent"
      - "Gated TestSandboxWorkspace: no 'Permission denied' appears in any result or error context; the host-side workspace of the sandboxed marker case contains marker.txt with 'hi'; native and sandbox host trees (excluding .git and .kiro) and 'git status --porcelain' are identical for all three cases"
      - "Gated test: repo-root 'git status --porcelain' is identical before and after the sandbox runs"
      - "Gated test: without --keep-workspaces the recorded workspace_dir is gone after a sandbox run; with it the directory exists and holds .eval/"
      - "Gated test: the sandboxed stub-timeout case (timeout 1s, 'sleep 3') is recorded as a timeout failure containing 'timeout after 1s'"
      - "Gated test: after editing an agent config and a case in the evals directory a second --sandbox run reuses the same base image ID and reports no build"
      - "Without a container daemon, or without KAIRON_EVAL_SANDBOX_SELFTEST=1, the gated tests are skipped with a message naming Podman and Docker (or the env var) and 'task eval:selftest:sandbox' exits 0; no container is started by a plain 'go test ./...'"
      - "The result of running the gated tests with a daemon, or the fact that no daemon was available, is recorded in the task's sentinel"
    validation_commands:
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail"
      - "task eval:selftest:sandbox"
      - "task sync:check"
      - "git status --porcelain"

  - id: "update-docs"
    agent: "documenter"
    description: "Update docs/evaluation.md for the new sandbox layering: base image contents and absences, the mounts table and helper mount, .kiro staging precedence, the workspace fixture format and case fields (workspace, timeout), stub turn commands, .eval/, --keep-workspaces and workspace_dir, the ownership and umask contract, SELinux/Podman/macOS notes and KAIRON_EVAL_WORKSPACE_ROOT, the base image cache key and rebuild rules (including the ToolSet bump procedure and the kiro-cli latest caveat if it applies), that image builds need network, and that gh is present but unconfigured until the containment issue. Rewrite the stale Container Sandboxing subsections (automatic toolchain installation, runtime-mocked gh, the Dockerfile example) and statements that native runs execute in the repo root. Update the self-test section for the new cases and the selftest-fail agent. Check docs/unified-container-flow.md and README.md for statements made false by this change."
    dependencies: ["selftest-and-container-tests"]
    acceptance_criteria:
      - "docs/evaluation.md no longer says the sandbox detects project types and installs toolchains, or that a mocked gh is installed into the container at runtime, and no longer shows the per-project generated Dockerfile example"
      - "docs/evaluation.md documents the three mounts and the helper mount with their read-only or read-write modes, the .kiro staging precedence, and states that nothing is copied into a running container"
      - "docs/evaluation.md documents the fixtures/workspaces/<name>/ format, the case fields workspace and timeout, stub turn commands, --keep-workspaces and the workspace_dir result field"
      - "docs/evaluation.md explains when the base image is rebuilt (Dockerfile or tool pin changes only) and when it is reused (agent, skill or case edits), and how to bump a baked tool"
      - "docs/evaluation.md states that file_exists and changed_files checks are not part of this change and that workspace scoring reads the host workspace"
      - "No file under .kairon/specs other than this issue's spec is modified"
    validation_commands:
      - "grep -n 'keep-workspaces' docs/evaluation.md"
      - "grep -n 'fixtures/workspaces' docs/evaluation.md"
      - "! grep -n 'automatically detects project types' docs/evaluation.md"
      - "git diff --stat -- docs README.md"
      - "git diff --quiet -- .kairon/specs ':!.kairon/specs/issue-297-sandbox-base-image-and-mounts.md'"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion of issue #297 against the code, tests and docs, and that build, tests, lint, formatting and template sync pass."
    dependencies: ["update-docs"]
    acceptance_criteria:
      - "AC1: base.Dockerfile has only kiro-cli, gh, git, shell, the sandbox user and an empty /workspace, with no COPY/ADD or project content; test evidence (or an explicit statement that no daemon was available) for the container check"
      - "AC2: no code path copies or mkdirs project or eval content in a running container (no SetupGitHubMocking, ConfigureMockGitHubPath or CopyTo in the runner; agentExecer has only ExecWithStdin); .kiro is mounted read-only and workspace and .eval read-write"
      - "AC3: the per-case workspace is built on the host as a git repo whose only commit is the fixture, and is mounted at the workspace path; the marker.txt stub case passes natively and, when a daemon is available, under --sandbox"
      - "AC4: repository root 'git status --porcelain' is unchanged by native and sandbox marker runs"
      - "AC5: the workspace including .eval/ is on the host after the run with the same layout as native"
      - "AC6: --keep-workspaces and workspace_dir behave as specified for native and sandbox"
      - "AC7: the case timeout is applied to the containerized run and a 1s case with 'sleep 3' is a timeout failure"
      - "AC8: the base image tag depends only on the Dockerfile, tool pins and platform, with unit test evidence, and reuse test evidence when a daemon is available"
      - "task test, task lint, task fmt:check and task sync:check pass; no file under cmd/kairon/templates and no historical spec was modified"
    validation_commands:
      - "go build ./... && go vet ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-297-sandbox-base-image-and-mounts.md"
```
