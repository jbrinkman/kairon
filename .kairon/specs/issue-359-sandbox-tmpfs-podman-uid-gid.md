# Design Spec: Sandbox /home/sandbox tmpfs must not use uid=/gid= options (Podman compatibility)

Closes #359

## Problem

`tmpfsMounts()` in `internal/eval/sandbox/mounts.go` builds the `/home/sandbox` tmpfs entry as
`rw,nosuid,nodev,uid=1000,gid=1000,mode=0755,size=256m`. Docker accepts `uid=`/`gid=`; Podman's
Docker-compatible API rejects them at container create:
`unknown mount option "uid=1000": invalid mount option`. Every container created through
`NewHostConfigWithMounts` therefore fails on Podman hosts (TestArchitectureDetectionInContainer,
TestPlatformSpecificImagePulling, TestKiroCLIInstallationMocking, TestResourceLimitsEnforcement,
TestKiroCLINotInstalledInBaseImage, ...).

## Solution Approach

Remove `uid=`/`gid=` from the `/home/sandbox` tmpfs and make the directory writable for uid 1000
through its mode alone: `rw,nosuid,nodev,mode=0777,size=256m`.

Why this approach:
- Runtime-neutral: `mode=`, `size=`, `nosuid`, `nodev` are accepted by both Docker and Podman. Mode 0777 is the
  variant the issue reports as probed-working on Podman.
- Safe in context: the tmpfs is ephemeral, size-capped, private to one container, and the container has a single
  unprivileged user (`sandbox`, uid 1000). World-writable here grants nothing beyond what that user already has.
  (The directory will be owned by root:root on the runtime, so `sandbox` writes via the "other" bits.)
- Smallest change: keeps the `HostConfig.Tmpfs` map shape, so `len(hc.Tmpfs) == 3` assertions in
  `internal/eval/container_runner_test.go` and `mounts_test.go` keep holding.

Rejected alternative: structured `mount.TypeTmpfs` + `TmpfsOptions`. The Docker API's `TmpfsOptions` exposes only
`SizeBytes`, `Mode` (and a raw `Options` list in newer clients) — there is still no portable owner field, so it
yields the same `mode=0777` outcome while forcing changes to `hc.Tmpfs` consumers and tests. No benefit.

Not chosen: `1777` (sticky). Not probed on Podman as the issue's candidate, and it adds nothing in a
single-user container. Builder should stay with `0777` unless a test shows a problem.

Consequences to handle:
- Unused-constant hygiene: `SandboxUID`/`SandboxGID` are no longer used to build the tmpfs spec. Keep them
  (exported; the new runtime test uses them to run as `1000:1000`) and fix their doc comment, which currently says
  they "own /home/sandbox". Don't leave a misleading comment, and drop the `fmt` import from `mounts.go` only if
  it becomes unused (it is still used by the validators, so it stays).
- Docs: `docs/evaluation.md` (Read-only root filesystem table, ~line 2513) documents the old option string and must
  be updated. `docs/unified-container-flow.md` mentions the mount generically; no change needed.
- `.kairon/specs/**` other than this file is frozen; do not touch.

## Relevant Files

Modify:
- `internal/eval/sandbox/mounts.go` — `tmpfsMounts()` home entry; `SandboxUID`/`SandboxGID` comment; tmpfs doc
  comment explaining why no uid/gid (Podman rejects them; containers/podman#10831).
- `internal/eval/sandbox/mounts_test.go` — `TestNewHostConfigWithMounts_Tmpfs` currently *requires*
  `uid=1000`, `gid=1000`, `mode=0755` for `/home/sandbox` and only forbids uid/gid on `/tmp` and `/var/tmp`.
  Update it to the new contract (see tasks).
- `internal/eval/sandbox/container_test.go` — add the runtime test that writes under `/home/sandbox` as uid 1000.
- `docs/evaluation.md` — update the `/home/sandbox` row.

Read-only context:
- `internal/eval/sandbox/container.go` (`Create`, `Exec`, `ExecWithOutput`; exec runs as the container's configured
  `User`), `internal/eval/container_runner_test.go` (asserts 3 tmpfs entries; unchanged), `internal/eval/runner.go`
  (`containerUser = "sandbox"`, `HOME=/home/sandbox`), `workspace_reserved_test.go`, `internal/config/sandbox_test.go`
  (workspace_dir collision with `/home/sandbox`; unchanged).

## Team Orchestration

Single builder task owns code + tests together (AGENTS.md: the failing test and the fix are committed together and
the builder must record which test it saw fail first). Docs update is independent of the code and can run in
parallel with it. A validator task runs after both.

```
implement-fix-with-tests ──┐
                           ├──> validate-all
update-docs ───────────────┘
```

## Step-by-Step Task Breakdown

### Task 1: implement-fix-with-tests (builder)
TDD order — do not reorder:
1. In `mounts_test.go`, rewrite the `/home/sandbox` expectation in `TestNewHostConfigWithMounts_Tmpfs`:
   wants `rw`, `nosuid`, `nodev`, `mode=0777`, `size=<TmpfsHomeSize>`; and extend the "must not set uid/gid" loop to
   cover `/home/sandbox` as well as `/tmp` and `/var/tmp` (i.e. iterate over all `hc.Tmpfs` values). Also add a
   direct `tmpfsMounts()` unit test asserting no `uid=`/`gid=` substring in any entry (AC3). Keep the `noexec`
   check and the `/tmp`/`/var/tmp` expectations (`mode=1777`, sizes) exactly as-is (AC4).
2. In `container_test.go`, add `TestSandboxHomeWritableAsSandboxUser` (name at builder's discretion): uses
   `skipIfNoContainerDaemon(t)` (the repo's existing daemon-availability guard; it must not be used to hide this
   failure — on a host with a daemon it runs), `NewContainer("alpine:3.19")`, `container.Config{User: "1000:1000",
   Cmd: sleep 30, Env HOME=/home/sandbox}`, `HostConfig` from `NewHostConfigWithMounts(DefaultLimits(), nil)`,
   Create/Start, then `ExecWithOutput` of `sh -c 'id -u && echo ok > /home/sandbox/probe && cat /home/sandbox/probe'`
   asserting uid `1000` and `ok`. Use `SandboxUID`/`SandboxGID` to build the user string. Also assert `/tmp` is still
   writable and the root FS is still read-only (e.g. `touch /etc/x` fails) to cover AC4 at runtime.
3. Run the new/updated tests and confirm they fail for the expected reason: the unit test fails on the `uid=`
   substring / `mode=0755`; on a Podman host the runtime test fails with `unknown mount option "uid=1000"`. A compile
   error is not an acceptable "failure".
4. Change `tmpfsMounts()`: home entry becomes `"rw,nosuid,nodev,mode=0777,size=" + TmpfsHomeSize`; update the
   comments as noted above.
5. Re-run `go test ./internal/eval/sandbox/... ./internal/eval/...` and confirm green. No `t.Skip`/build tags added
   to dodge failures.
6. Commit test + implementation together (one commit); the message names the test observed failing first.

Acceptance criteria: AC1–AC5 of the issue (see plan block).

### Task 2: update-docs (builder or documenter; assigned to documenter)
Update the `/home/sandbox` row in `docs/evaluation.md` Read-only root filesystem table to
`rw,nosuid,nodev,mode=0777,size=256m`, and add one sentence to the notes: ownership options (`uid=`/`gid=`) are
deliberately omitted because Podman rejects them; world-writable mode is acceptable because the tmpfs is private,
ephemeral and single-user. Change only `docs/`; no template-synchronized files are affected.

### Task 3: validate-all (validator)
Read-only verification of every acceptance criterion; see plan block.

## Validation Commands

```bash
go build ./...
go vet ./internal/eval/...
go test ./internal/eval/sandbox/... -run 'Tmpfs|HostConfig|Mounts|HomeWritable' -count=1
go test ./internal/eval/... -count=1
! grep -n 'uid=' internal/eval/sandbox/mounts.go          # no uid= option in tmpfs spec
grep -n 'mode=0777' internal/eval/sandbox/mounts.go
task sync:check
```

Environment note for the validator: the container-backed tests need a reachable Docker or Podman daemon. If none is
reachable the runtime tests are skipped by the existing guard; say so explicitly rather than reporting AC1/AC2 as
verified. The unit tests (AC3, AC4) run regardless.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-fix-with-tests"
    agent: "builder"
    description: "TDD fix in internal/eval/sandbox: update mounts_test.go and add a container runtime test first (confirm they fail for the right reason), then change tmpfsMounts() so the /home/sandbox tmpfs is 'rw,nosuid,nodev,mode=0777,size=<TmpfsHomeSize>' with no uid=/gid=; fix SandboxUID/SandboxGID and tmpfs comments; commit tests and fix together."
    dependencies: []
    acceptance_criteria:
      - "tmpfsMounts() /home/sandbox entry contains no uid= or gid= option and contains mode=0777, nosuid, nodev, rw and size=TmpfsHomeSize"
      - "Unit test asserts no uid=/gid= substring in any tmpfs entry returned by tmpfsMounts() / NewHostConfigWithMounts (including /home/sandbox)"
      - "Runtime test runs as uid/gid 1000 from SandboxUID/SandboxGID and successfully writes and reads a file under /home/sandbox, also checks /tmp is writable and rootfs is read-only"
      - "/tmp and /var/tmp tmpfs specs (mode=1777, sizes) and ReadonlyRootfs behavior are unchanged; existing mounts_test.go and workspace_reserved_test.go tests pass"
      - "No t.Skip, build tags, or test removals added to avoid failures; changes confined to internal/eval/sandbox/"
      - "Commit contains the tests and the fix together and names the test observed failing first"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/sandbox/... -count=1"
      - "go test ./internal/eval/... -count=1"
      - "! grep -n 'uid=' internal/eval/sandbox/mounts.go"

  - id: "update-docs"
    agent: "documenter"
    description: "Update docs/evaluation.md Read-only root filesystem table so the /home/sandbox tmpfs row shows rw,nosuid,nodev,mode=0777,size=256m, and add a note that uid=/gid= are intentionally omitted because Podman rejects them."
    dependencies: []
    acceptance_criteria:
      - "docs/evaluation.md no longer mentions uid=1000,gid=1000 for the /home/sandbox tmpfs"
      - "docs/evaluation.md states mode=0777 for /home/sandbox and explains why ownership options are omitted"
      - "No files under .kairon/specs/ other than issue-359 spec, and no CHANGELOG.md, are modified"
    validation_commands:
      - "! grep -n 'uid=1000,gid=1000' docs/evaluation.md"
      - "grep -n 'mode=0777' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #359: no uid=/gid= in tmpfs specs, runtime writability test for uid 1000, /tmp and /var/tmp and read-only rootfs unchanged, existing tests green, change scope confined, docs updated."
    dependencies: ["implement-fix-with-tests", "update-docs"]
    acceptance_criteria:
      - "go test ./internal/eval/sandbox/... passes with no 'unknown mount option' failures (or, if no container daemon is reachable, the report states container-backed tests were skipped and could not be verified)"
      - "Unit test for tmpfsMounts/NewHostConfigWithMounts asserting no uid=/gid= is present and passing"
      - "Runtime /home/sandbox write test as uid 1000 is present and passing (or explicitly reported as unverified due to no daemon)"
      - "git diff against the base branch touches only internal/eval/sandbox/, docs/evaluation.md and the issue-359 spec"
      - "task sync:check and go vet pass"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"
      - "task sync:check"
      - "! grep -n 'uid=' internal/eval/sandbox/mounts.go"
```
