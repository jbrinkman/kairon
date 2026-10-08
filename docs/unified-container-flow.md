# Unified Container Creation Flow Documentation

## Overview

The eval system uses one **ensure image → build workspace → create → verify → execute** flow for both test and production environments. A single tools-only base image is built once and reused; everything that varies per case is supplied from the host through bind mounts. Nothing is generated, copied or installed inside a running container.

See [evaluation.md](evaluation.md#container-sandboxing) for the user-facing description (mounts, ownership rules, platform notes, cache rules). This page describes the code flow.

## Complete Flow Analysis

### Phase 1: Ensure the base image
**Location:** `internal/eval/sandbox/baseimage.go` - `ImageManager.EnsureBaseImage()`
**Used by:** Tests and Production (called once per run from `eval.Run()`)
**Purpose:** Return the tag of the tools-only base image, building it only when it does not exist.

**Steps:**
1. Resolve the pinned tool set (`DefaultToolSet`: kiro-cli and gh versions) into Docker build args with `ToolSet.BuildArgs(platform)`.
2. Compute the tag `kairon-eval-base:<platform>-<12 hex>` with `BaseImageTag`, a pure function of the embedded `base.Dockerfile` bytes, the platform and the build args. Nothing else (evals directory, agents, skills, cases, cwd, environment) participates.
3. `ImageInspect(tag)`: if the image exists, reuse it (`built=false`).
4. Otherwise build it once (mutex-guarded) and return `built=true`.

`Run()` prints `✅ Base image reused: <tag>` or `🔨 Base image built: <tag>`. The image is persistent: it is never removed at the end of a run. Building needs network access (kiro-cli and gh are downloaded as root at build time).

### Phase 2: Build the case workspace (host side)
**Location:** `internal/eval/workspace.go` - `newCaseWorkspace()`
**Used by:** Native and container runs (via `executeCase`)
**Purpose:** Create the per-case git workspace, staged `.kiro/` and `.eval/` on the host.

The workspace is a temp directory under `KAIRON_EVAL_WORKSPACE_ROOT` (or the OS temp directory), symlink-resolved, containing the case fixture as the single git commit, a staged `.kiro/` (fixture > `<evals-dir>/agents` > project `.kiro`) and an empty `.eval/`. Next to it, in the same private parent, the host writes the fake `gh` to `bin/gh` and, when the case defines `gh_issue`, renders `.eval/gh-issue.json` and `.eval/gh-issue.txt` for it. See [Case Workspaces](evaluation.md#case-workspaces) and [The fake `gh`](evaluation.md#the-fake-gh).

### Phase 3: Create container
**Location:** `internal/eval/runner.go` - `invokeAgentInContainer()`, `internal/eval/sandbox/mounts.go` - `NewHostConfigWithMounts()`
**Used by:** Tests and Production
**Purpose:** Create a container from the cached base image with explicit bind mounts.

**Steps:**
1. Build the mount list (`buildContainerMounts`): `<ws>/.kiro` read-only, `<ws>` read-write and `<ws>/.eval` read-write at the configured workspace path, the per-case bin directory holding the fake `gh` read-only at `/opt/kairon/bin`, plus the linux `kairon` helper read-only at `/opt/kairon/kairon` for non-`kiro-cli` backends only.
2. Validate every host path (absolute, exists, symlink-resolved) before the container is created. Structured `HostConfig.Mounts` are used, not `Binds` strings, so a missing path fails instead of being created as a root-owned directory.
3. Apply resource limits (CPU, memory) and `NetworkMode: none`, set `ReadonlyRootfs: true` and add the `/tmp`, `/var/tmp` and `/home/sandbox` tmpfs mounts; no tmpfs is mounted at the workspace path.
4. Set `User: sandbox`, `WorkingDir`, `HOME=/home/sandbox`, `PATH` with `/opt/kairon/bin` first, `KAIRON_EVAL_DIR`, `GH_PROMPT_DISABLED=1`, `GH_NO_UPDATE_NOTIFIER=1` and the git `safe.directory=*` environment, plus the configured environment (for example `KIRO_CLI_DISABLE_TELEMETRY=1`). GitHub credential variables (`GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_HOST`) are never passed on.
5. On SELinux-enforcing hosts, add `label=disable`.
6. Create and start the container.

### Phase 4: Verify
**Location:** `internal/eval/sandbox/container.go` - `ValidateKiroCLI()`
**Used by:** Tests and Production (`kiro-cli` backend)
**Purpose:** Check, read-only, that the baked `kiro-cli` is present and executable. Nothing is installed.

### Phase 5: Execute
**Location:** `internal/eval/runner.go` - `runAgentInContainer()`
Before this, `invokeAgent` resolves the agent's whole-tool trust set (`resolveTrustSet` in `internal/eval/trust.go`: `evals.trust_tools` override, else the agent config's `allowedTools`, else empty) and sets it on the request, so the `kiro-cli` argv uses `--trust-tools=<csv>` instead of `--trust-all-tools`. A resolution failure fails the call before anything runs. See [Tool trust](evaluation.md#tool-trust).
Every exec is wrapped by `sandbox.WithOpenUmask` (`sh -c 'umask 000; exec "$@"' kairon-exec …`) so files created by the agent are world-accessible and the host can score and delete them. The prompt is delivered on stdin only. The host exec deadline equals the effective timeout for `kiro-cli` and the timeout plus 10 seconds for helper backends. The `agentExecer` interface has only `ExecWithStdin`; there is no copy-into-container operation.

## Flow Consistency Verification

Test and production environments share the same code:

- `EnsureBaseImage()` obtains the image.
- `newCaseWorkspace()` builds the workspace.
- `NewHostConfigWithMounts()` and `CreateWithPlatform()` create the container from the base image name.
- `ValidateKiroCLI()` verifies the pre-installed binary.

A source-level guard test fails if `runner.go` reintroduces `SetupGitHubMocking`, `ConfigureMockGitHubPath` or a `.CopyTo(` call.

## Performance Notes

- **Cold run:** one base-image build (dominated by downloading kiro-cli and gh), then container creation per case.
- **Warm run:** the image is reused (`ImageInspect` only). Editing agents, skills, rubrics or cases never triggers a rebuild; only a change to `base.Dockerfile`, a tool pin or the platform does.
- Timings were not re-measured for this change; the numbers previously listed on this page described the removed per-run generate-and-build flow.

## Debug Mode

With `--debug`, failed containers are preserved for inspection and `kairon eval --cleanup` removes tracked debug containers. Debug mode does not preserve workspaces: use `--keep-workspaces` together with `--debug` to inspect both.

```bash
# Run with debug mode and keep workspaces
kairon eval --debug --keep-workspaces --sandbox architect

# Inspect the cached base image
docker images | grep kairon-eval-base

# Inspect debug containers
docker ps -a | grep kairon-eval
docker exec -it <container-id> sh
```

## Validation Commands

### Unit Tests
```bash
go test ./internal/eval/sandbox/... -count=1   # base image tag, mounts, umask wrapper
go test ./internal/eval/... -count=1           # workspace, executeCase, container runner (fake executor)
```

### Daemon-gated tests (need Podman or Docker, network for the first build)
```bash
task eval:selftest:sandbox
```

### Manual Verification
```bash
# Tools-only base image: no mounts, nothing project-specific inside
docker run --rm kairon-eval-base:<platform>-<hash> sh -c 'id -un; command -v kiro-cli gh git sh; ls -A /workspace'
```

## Error Handling and Diagnostics

### Common Issues and Solutions

1. **Build failures:** check network connectivity for the kiro-cli and gh downloads.
2. **Platform mismatches:** verify platform detection matches the target architecture (`linux/amd64` or `linux/arm64`).
3. **Mount failures on macOS:** set `KAIRON_EVAL_WORKSPACE_ROOT` to a directory shared with the Docker Desktop / Podman machine VM.
4. **Helper binary not usable:** the helper mounted at `/opt/kairon/kairon` must be readable and executable by uid 1000 (`0755`); set `KAIRON_SANDBOX_BINARY` to a prebuilt static linux binary.
5. **Resource limits:** adjust CPU/memory/timeout limits for heavy workloads; a case can set its own `timeout`.

### Enhanced Error Messages
- Container timeout: names the effective timeout and suggests the case `timeout` or `--resource-limit timeout=`
- Out of memory: suggests increasing the memory limit
- Image pull failures: provides network connectivity guidance
- Binary not found: detailed troubleshooting for installation failures

## Future Enhancements

1. **Image Registry:** Push built images to a registry for sharing across instances
2. **Network policy and mock guidance:** the container still has `NetworkMode: none` as a plain setting, not an enforced policy. Network policy, and guidance for writing mocks of network services, are separate follow-up work. Read-only root filesystem, the fake `gh` and whole-tool trust are done (see [Sandbox Containment](evaluation.md#sandbox-containment))
3. **Health Checks:** Add container health checks for better reliability monitoring
