# Design Spec: Skip sandbox tests when no Podman or Docker daemon is running

Closes #290

## Problem Analysis

`task test` (`go test -v -race -coverprofile=coverage.out ./...`) fails on machines with no container daemon. Seven tests in `internal/eval/sandbox/installation_test.go` call `NewContainer("alpine:3.19")` without first calling `skipIfNoDocker(t)`. `NewContainerWithDebug` (container.go) pings the daemon and returns `Docker is not running. Start Docker and try again: ...`, which the tests turn into a `require.NoError` failure.

Unguarded tests (all confirmed by reading the file; every other `NewContainer` caller in the package already calls `skipIfNoDocker`):

| Test | Line |
|------|------|
| TestDockerfileGeneration_IncludesKiroCLI | 16 (NewContainer inside `t.Run` at line 48) |
| TestDockerfileGeneration_ProjectDetection | 221 |
| TestContainer_LogStartup | 299 |
| TestContainer_GetContainerInfo | 315 |
| TestContainer_ArchitectureErrors | 400 |
| TestGenerateDockerfile_ErrorHandling | 415 |
| TestContainer_CompleteInstallationFlow | 464 |

Several of these only exercise Dockerfile generation, but the constructor they go through requires a live daemon. Decoupling the constructor is out of scope ("changing sandbox container implementation beyond daemon detection and the user-facing message"), so these tests are skipped rather than rewritten.

Secondary problems:

1. `skipIfNoDocker` only talks about "Docker" in its skip message and only honors `DOCKER_HOST`/default socket. A machine with only Podman running (socket not exported through `DOCKER_HOST`) is skipped although a usable daemon exists.
2. The runtime error text `Docker is not running. Start Docker and try again` appears in `container.go` and `image_manager.go` and names only Docker, although Podman works through its Docker-compatible socket.

## Solution Approach

Three small, mostly test-side changes:

1. **Shared runtime message** (non-test code). Add `internal/eval/sandbox/daemon.go` with one unexported helper `daemonNotRunningError(err error) error` that returns `fmt.Errorf("container daemon is not running. Start Podman or Docker and try again: %w", err)`. Use it in `NewContainerWithDebug` and `NewImageManager`. The wrapped `%w` error is preserved. The runtime still relies on the SDK (`client.FromEnv`), so it honors `DOCKER_HOST` exactly as before.

2. **Generalized test guard** (test-only code). Replace `skipIfNoDocker` in `container_test.go` with `skipIfNoContainerDaemon(t)` in a new file `daemon_test.go`-style helper file (`container_daemon_test.go`). Behavior:
   - If `DOCKER_HOST` is set, ping with `client.FromEnv` (unchanged semantics; a Podman socket exported via `DOCKER_HOST` counts as a daemon). Skip if the ping fails.
   - If `DOCKER_HOST` is unset, ping the default Docker endpoint via `client.FromEnv` (2s timeout, API negotiation, as today). If that works, proceed.
   - Otherwise probe well-known Podman socket paths (see below). On the first candidate whose ping succeeds (2s timeout each), call `t.Setenv("DOCKER_HOST", "unix://<path>")` so that the production constructors (`NewContainer`, `NewImageManager`), which use `client.FromEnv`, reach the same daemon the guard found. `t.Setenv` restores the environment automatically at test end. (No test in the package uses `t.Parallel`; verified with grep.)
   - If nothing is reachable, `t.Skip` with a message naming both runtimes, e.g. `no container daemon reachable (tried Podman and Docker); start Podman or Docker to run this test: <last error>`.
   - Podman is never required to be on `PATH`; only socket reachability matters.

   Podman socket candidate list, built by a pure function `podmanSocketCandidates(goos string, getenv func(string) string, uid int) []string` so it can be unit tested:
   - `$XDG_RUNTIME_DIR/podman/podman.sock` (Linux rootless; if `XDG_RUNTIME_DIR` is empty on Linux fall back to `/run/user/<uid>/podman/podman.sock`)
   - `/run/podman/podman.sock` (Linux rootful)
   - macOS (`darwin`): `$TMPDIR/podman/podman-machine-default-api.sock` and `$HOME/.local/share/containers/podman/machine/podman.sock` and `$HOME/.local/share/containers/podman/machine/podman-machine-default/podman.sock`
   Only candidates that exist on disk are pinged (`os.Stat` first) to avoid needless timeouts.

3. **Guard the seven failing tests.** Add `skipIfNoContainerDaemon(t)` as the first statement of each of the seven tests (for `TestDockerfileGeneration_IncludesKiroCLI` and `TestContainer_CompleteInstallationFlow` at the top-level function, not inside `t.Run`, so the whole test is reported SKIP). Update all existing call sites of `skipIfNoDocker` (container_test.go, installation_test.go, architecture_test.go, integration_*_test.go) to the new name. Also update the stale comment "without Docker dependency" in `TestContainer_CompleteInstallationFlow`.

### Why the guard does `t.Setenv` instead of changing production endpoint resolution

AC3 requires that an active Podman daemon makes the tests run. The tests reach the daemon through production constructors that use `client.FromEnv`. Changing production code to probe Podman sockets would alter runtime behavior beyond "daemon detection and the user-facing message" and would be hard to review as "affects only the no-daemon path" (AC4). Exporting `DOCKER_HOST` for the duration of a single test keeps production behavior identical while letting tests use a discovered Podman socket.

### Design decisions and trade-offs

- Rename vs keep `skipIfNoDocker`: renaming reflects the generalized behavior; cost is a mechanical rename across ~8 test files (same package). Chosen: rename.
- The error-message wording keeps `%w` wrapping and avoids the literal "Start Docker and try again" so AC5's grep returns nothing in `internal/eval/sandbox`.
- Out of scope but noted: `internal/eval/runner.go` (`checkDockerAvailability`, line 40) has the same "Start Docker and try again" message. It is outside `internal/eval/sandbox`, so it is left unchanged per the issue scope; it can be aligned in a follow-up.
- No starting/installing of Docker or Podman from the harness or CI.

## Relevant Files

Create:
- `internal/eval/sandbox/daemon.go` — `daemonNotRunningError` helper (runtime message).
- `internal/eval/sandbox/container_daemon_test.go` — `skipIfNoContainerDaemon`, `podmanSocketCandidates`, ping helper, and unit tests for them (including a fake Docker-compatible server on a temp unix socket for the Podman-discovery path).

Modify:
- `internal/eval/sandbox/container.go` — use `daemonNotRunningError` in `NewContainerWithDebug` (line ~110).
- `internal/eval/sandbox/image_manager.go` — use `daemonNotRunningError` in `NewImageManager` (line ~36).
- `internal/eval/sandbox/container_test.go` — remove old `skipIfNoDocker`, rename call sites; drop now-unused imports if any.
- `internal/eval/sandbox/installation_test.go` — add guard to the 7 tests; rename existing call sites.
- `internal/eval/sandbox/architecture_test.go`, `integration_architecture_test.go`, `integration_installation_test.go`, `integration_endtoend_test.go` — rename call sites only.

Reference only (not modified): `internal/eval/runner.go`, `Taskfile.yml` (`test` task), `go.mod` (docker SDK v28.5.2).

## Team Orchestration

- `daemon-error-message` (runtime message) and `container-daemon-guard` (test helper + rename) touch disjoint files and can run in parallel. Note the helper task also removes `skipIfNoDocker` from `container_test.go` and renames call sites in the other test files.
- `guard-failing-tests` edits `installation_test.go` and depends on the helper existing, so it runs after `container-daemon-guard` (they must not edit `installation_test.go` concurrently; the helper task owns renames in that file, so the guard task runs after it).
- `validate-all` runs after all builder tasks and verifies every acceptance criterion in the no-daemon environment plus unit tests of the detection logic.
- `document-change` is optional-small: a short note in the README is not needed; skip documenter unless validator finds a doc gap. (No documenter task is planned.)

## Step-by-Step Task Breakdown

### Task 1: daemon-error-message
Add `daemon.go` with `daemonNotRunningError`, use it in `container.go` and `image_manager.go`. Acceptance:
- No occurrence of `Start Docker and try again` remains under `internal/eval/sandbox`.
- New message names both Podman and Docker and wraps the original error with `%w`.
- `go build ./...` passes.
Dependencies: none (parallel with Task 2).

### Task 2: container-daemon-guard
Create `container_daemon_test.go` with `skipIfNoContainerDaemon`, `podmanSocketCandidates`, and unit tests; remove old `skipIfNoDocker`; rename all call sites package-wide (including `installation_test.go` existing calls). Acceptance:
- Skip message mentions both "Podman" and "Docker".
- With `DOCKER_HOST` unset and a reachable fake Podman socket among candidates, the helper sets `DOCKER_HOST` and does not skip (covered by a unit test using a fake unix-socket server answering `/_ping`; candidates are injectable so the test does not depend on the real host).
- `podmanSocketCandidates` returns the documented paths for `linux` and `darwin`.
- `go vet ./internal/eval/sandbox` passes.
Dependencies: none (parallel with Task 1).

### Task 3: guard-failing-tests
Add `skipIfNoContainerDaemon(t)` as the first statement in the seven named tests. Acceptance:
- With no daemon reachable, the seven tests report `--- SKIP` and the package reports `ok`.
- `go test ./internal/eval/sandbox -run TestContainer_LogStartup -v` shows a SKIP line mentioning both Podman and Docker (when no daemon is running).
Dependencies: Task 2.

### Task 4: validate-all
Read-only verification of every issue acceptance criterion.
Dependencies: Tasks 1, 3.

## Validation Commands

```
go build ./...
go vet ./internal/eval/sandbox
gofmt -l internal/eval/sandbox          # expect empty output
grep -rn "Start Docker and try again" internal/eval/sandbox   # expect no matches
go test ./internal/eval/sandbox -run 'TestDockerfileGeneration_IncludesKiroCLI|TestDockerfileGeneration_ProjectDetection|TestContainer_LogStartup|TestContainer_GetContainerInfo|TestContainer_ArchitectureErrors|TestGenerateDockerfile_ErrorHandling|TestContainer_CompleteInstallationFlow' -v
go test ./internal/eval/sandbox -run TestContainer_LogStartup -v
task test
```

With a daemon absent (`DOCKER_HOST=unix:///nonexistent.sock` forces this on a machine that has Docker): the package must say `ok` and the seven tests must say `--- SKIP`. Note that on a machine with an unexported Podman socket present, the guard will discover it and run the tests instead of skipping; to force the no-daemon path in that case, run with `DOCKER_HOST` unset and `XDG_RUNTIME_DIR`/`HOME`/`TMPDIR` pointed at an empty temp dir, or validate on a host without Podman.

Manual (ACs 3 and 4, per issue): with a Podman machine running and `DOCKER_HOST` pointing at its socket (or unset with the socket in a well-known location), the previously-failing tests execute and pass; with Docker running, they execute and pass as before.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "daemon-error-message"
    agent: "builder"
    description: "Add internal/eval/sandbox/daemon.go with an unexported daemonNotRunningError(err) helper whose message names both Podman and Docker and wraps err with %w; use it in NewContainerWithDebug (container.go) and NewImageManager (image_manager.go) in place of the 'Docker is not running. Start Docker and try again' errors."
    dependencies: []
    acceptance_criteria:
      - "grep -rn 'Start Docker and try again' internal/eval/sandbox returns no matches"
      - "The new error message mentions both Podman and Docker and wraps the underlying error with %w"
      - "container.go and image_manager.go both call daemonNotRunningError; no other behavior in these files changes"
      - "go build ./... succeeds"
    validation_commands:
      - "go build ./..."
      - "! grep -rn 'Start Docker and try again' internal/eval/sandbox"
      - "gofmt -l internal/eval/sandbox | (! grep .)"

  - id: "container-daemon-guard"
    agent: "builder"
    description: "Create internal/eval/sandbox/container_daemon_test.go containing skipIfNoContainerDaemon(t) (replaces skipIfNoDocker): ping via client.FromEnv with API negotiation and 2s timeout; if DOCKER_HOST is unset and the default endpoint fails, probe existing Podman socket candidates from podmanSocketCandidates(goos, getenv, uid) and on success t.Setenv DOCKER_HOST to unix://<path>; otherwise t.Skip with a message naming both Podman and Docker. Remove the old skipIfNoDocker from container_test.go and rename every call site in the package (container_test.go, installation_test.go, architecture_test.go, integration_architecture_test.go, integration_installation_test.go, integration_endtoend_test.go). Add unit tests for podmanSocketCandidates (linux and darwin) and for Podman-socket discovery using a fake Docker-compatible /_ping server on a temp unix socket (candidate list injectable so the test is independent of the host)."
    dependencies: []
    acceptance_criteria:
      - "skipIfNoDocker no longer exists anywhere in internal/eval/sandbox; all call sites use skipIfNoContainerDaemon"
      - "Skip message contains both 'Podman' and 'Docker'"
      - "podmanSocketCandidates returns XDG_RUNTIME_DIR/podman/podman.sock and /run/podman/podman.sock for linux, and the podman machine socket paths for darwin"
      - "Unit test proves that when DOCKER_HOST is unset and a fake Podman socket answers /_ping, the helper logic selects it and sets DOCKER_HOST instead of skipping"
      - "Podman binary is not required on PATH"
      - "go vet ./internal/eval/sandbox passes"
    validation_commands:
      - "! grep -rn 'skipIfNoDocker' internal/eval/sandbox"
      - "go vet ./internal/eval/sandbox"
      - "go test ./internal/eval/sandbox -run 'Podman|Candidates|SkipIfNoContainerDaemon' -v"
      - "gofmt -l internal/eval/sandbox | (! grep .)"

  - id: "guard-failing-tests"
    agent: "builder"
    description: "In installation_test.go add skipIfNoContainerDaemon(t) as the first statement of TestDockerfileGeneration_IncludesKiroCLI, TestDockerfileGeneration_ProjectDetection, TestContainer_LogStartup, TestContainer_GetContainerInfo, TestContainer_ArchitectureErrors, TestGenerateDockerfile_ErrorHandling and TestContainer_CompleteInstallationFlow (top-level function, not inside t.Run). Fix the stale 'without Docker dependency' comment in TestContainer_CompleteInstallationFlow."
    dependencies: ["container-daemon-guard"]
    acceptance_criteria:
      - "With no reachable container daemon, the seven named tests report --- SKIP and the sandbox package reports ok"
      - "go test ./internal/eval/sandbox -run TestContainer_LogStartup -v prints a SKIP line mentioning both Podman and Docker when no daemon is reachable"
      - "Tests that already had a guard are unchanged other than the rename"
    validation_commands:
      - "DOCKER_HOST=unix:///nonexistent-kairon.sock XDG_RUNTIME_DIR=$(mktemp -d) go test ./internal/eval/sandbox -run 'TestDockerfileGeneration_IncludesKiroCLI|TestDockerfileGeneration_ProjectDetection|TestContainer_LogStartup|TestContainer_GetContainerInfo|TestContainer_ArchitectureErrors|TestGenerateDockerfile_ErrorHandling|TestContainer_CompleteInstallationFlow' -v"
      - "go vet ./internal/eval/sandbox"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion from issue #290: no-daemon run skips (not fails) all seven named tests and package reports ok; skip message names Podman and Docker; detection unit tests pass; runtime message names both runtimes and old text is gone; existing guarded tests and production constructors are otherwise unchanged (git diff shows no behavioral change in container.go/image_manager.go beyond the error text); gofmt/vet clean. Manual ACs 3 and 4 (live Podman/Docker) are reported as not verifiable here unless a daemon is available."
    dependencies: ["daemon-error-message", "guard-failing-tests"]
    acceptance_criteria:
      - "go test ./internal/eval/sandbox exits 0 with no daemon reachable (DOCKER_HOST=unix:///nonexistent-kairon.sock) and the seven named tests appear as --- SKIP"
      - "TestContainer_LogStartup -v SKIP line mentions both Podman and Docker"
      - "grep -rn 'Start Docker and try again' internal/eval/sandbox returns no matches"
      - "task test exits 0 in an environment without a container daemon (or any non-sandbox failure is shown to be pre-existing and unrelated)"
      - "go vet ./... and gofmt -l . report nothing for touched files"
    validation_commands:
      - "DOCKER_HOST=unix:///nonexistent-kairon.sock XDG_RUNTIME_DIR=$(mktemp -d) go test ./internal/eval/sandbox -v 2>&1 | grep -E '^(--- SKIP|--- FAIL|ok|FAIL)'"
      - "DOCKER_HOST=unix:///nonexistent-kairon.sock go test ./internal/eval/sandbox -run TestContainer_LogStartup -v"
      - "! grep -rn 'Start Docker and try again' internal/eval/sandbox"
      - "go vet ./internal/eval/sandbox"
      - "test -z \"$(gofmt -l internal/eval/sandbox)\""
      - "task test"
```
