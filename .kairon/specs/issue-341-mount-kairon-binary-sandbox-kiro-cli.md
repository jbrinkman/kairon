# Design Spec: Evals — mount the kairon binary for sandboxed kiro-cli agents

Closes #341

## Problem

In a `--sandbox` eval run, `buildContainerMounts` (`internal/eval/runner.go`) mounts the linux `kairon`
binary only when the backend is not `kiro-cli`, and mounts it at `/opt/kairon/kairon` (`containerHelperPath`),
which is not on the container `PATH` (`containerPath` = `/opt/kairon/bin:/usr/local/sbin:/usr/local/bin:...`).
A sandboxed `kiro-cli` agent therefore cannot start a `kairon` MCP server, so the Stage 4 dispatch-only
krew-lead cannot be evaluated under containment (`.kairon/specs/maturity-model/step-0-validation.md`, finding 11).

Today the binary is a private *helper* for the stub backend (`kairon inference-exec`, invoked by absolute path in
`runAgentInContainer`). This issue turns it into a first-class, always-present, read-only part of the sandbox that
agents can call as `kairon`.

## Current behaviour (verified in code)

- `internal/eval/runner.go`
  - `containerHelperPath = "/opt/kairon/kairon"`; `containerBinDir = "/opt/kairon/bin"`; `containerPath` puts
    `/opt/kairon/bin` first, then `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`.
  - `buildContainerMounts(ws, cConfig, backendName)` adds `.kiro` (ro), workspace (rw), `.eval` (rw), fake-gh bin
    dir (ro, gated by `fakeGHInstalled`), and — only `if backendName != inference.NameKiroCLI` — the binary from
    `resolveLinuxBinary(cConfig.Platform)` at `containerHelperPath` (ro), after checking mode has `o+rx`
    (uid 1000 `sandbox` is not the owner).
  - `runAgentInContainer` runs non-kiro backends as `containerHelperPath inference-exec --backend <name>`.
  - `prepareBaseImage(cConfig)` is the shared per-run pre-flight for every container entry point
    (full run `:375`, single case `:278`, resume `:1835`); it runs before any case starts.
- `internal/eval/sandbox/linuxbin.go`: `ResolveLinuxBinary(platform)` — `KAIRON_SANDBOX_BINARY` → running
  executable (if linux + same arch and not a `go test` binary) → cross-compile from module root → error naming
  `KAIRON_SANDBOX_BINARY`. Results memoised.
- Today a missing binary surfaces only when the first case opens its container session (inside the case), not
  before cases start.
- `internal/eval/containment.go`: `containmentFor(agent)` builds `Containment{ToolTrust, FakeGH, ReadOnlyFS, Network}`;
  `FakeGH` comes from the const `fakeGHInstalled`, which `buildContainerMounts` also honours (record cannot drift
  from enforcement). `Containment` is defined in `types.go`; `diff_provenance.go` renders/diffs it.
- `reservedMockCommands` (`workspace.go`) lists command names a case may not mock.
- Self-test fixtures: `internal/eval/testdata/evals/{agents,cases}/selftest-sandbox*`, exercised by the
  daemon-gated `TestContainmentSandbox` etc. (`task eval:selftest:sandbox`, opt-in via
  `KAIRON_EVAL_SANDBOX_SELFTEST=1`, skips without a container daemon).

## Solution Approach

1. **One mount, on `PATH`, for every backend.** Change `containerHelperPath` to `/usr/local/bin/kairon` and mount
   the resolved linux binary there, read-only, unconditionally (drop the `backendName != kiro-cli` condition).
   - `/usr/local/bin` is already on `containerPath`, after the fake-gh dir, so a bare `kairon` resolves with no
     PATH change and the fake-gh/mocks precedence is unchanged.
   - Same mechanism as the existing file/dir mounts onto a read-only rootfs (mountpoint is created by the runtime
     before the rootfs is made read-only), so it is not a new risk. Builder MUST confirm under the daemon via the new
     self-test; if the runtime refuses a file mountpoint in `/usr/local/bin`, the fallback is a dedicated
     directory (e.g. mount at `/opt/kairon/sbin/kairon` and append `/opt/kairon/sbin` to `containerPath`) — keep
     `containerHelperPath` as the single constant so only one place changes.
   - `runAgentInContainer` keeps using `containerHelperPath` (now `/usr/local/bin/kairon`) for `inference-exec`; the
     stub path continues to work unchanged.
   - Keep the `o+rx` mode check and make its error message backend-neutral ("kairon binary …"), still naming the
     file and `chmod 755`.
2. **Read-only by construction.** The mount is `ReadOnly: true`; the agent (uid 1000, not the file's owner) can't
   write it. AC 3 is proven by a self-test case, not just by the flag.
3. **Fail before any case starts (AC 5).** Add a binary pre-flight to `prepareBaseImage` (the shared pre-flight):
   when `cConfig != nil`, call `resolveLinuxBinary(cConfig.Platform)` *first* (cheap, before creating the image
   manager / building the base image) and return an error such as
   `preparing the kairon binary for the sandbox (linux/arm64): <resolver error>`. The resolver error already names
   `KAIRON_SANDBOX_BINARY` and the alternatives (static linux binary, or run from a source checkout with Go), so
   "naming the binary and how to provide it" is satisfied. Because `prepareBaseImage` runs for the full run, the
   single-case and the resume paths, no case starts. `ResolveLinuxBinary` is memoised, so the later per-case
   resolution in `buildContainerMounts` is free and consistent.
4. **Record it (AC 4).** Add `KaironBinary bool` (`json:"kairon_binary"`, not omitempty — same convention as
   `fake_gh`/`read_only_fs`) to `Containment`. Source it from a new const `kaironBinaryMounted = true` in
   `containment.go` that `buildContainerMounts` also honours (mount only `if kaironBinaryMounted`), mirroring
   `fakeGHInstalled`, so record and enforcement cannot drift. Update `describeContainment`/`containmentDiff` in
   `diff_provenance.go` (`kairon_binary: false → true`; old results lacking the field read as `false`, which is
   accurate, and show as a mode difference in `eval diff`).
5. **Prevent shadowing.** Add `kairon` to `reservedMockCommands` ("reserved for the harness's kairon binary"): the
   mock bin dir is first on `PATH`, so a case mock named `kairon` would silently replace the mounted binary.
6. **Self-test coverage.** Add two `requires_sandbox: true` cases to the `selftest-sandbox` agent (stub backend,
   so they run wherever the daemon is available):
   - `stub-kairon-binary` (AC 1): stub turn `kairon --version > .eval/kairon-version.txt` plus
     `command -v kairon > kairon-path.txt`; checks `file_contains` on `.eval/kairon-version.txt` (pattern
     matching the version output — builder confirms what `kairon --version` prints for a `go build`/cross-compiled
     binary, e.g. `kairon`) and on `kairon-path.txt` (`/usr/local/bin/kairon`).
   - `stub-kairon-binary-readonly` (AC 3): stub turn attempts to write the binary's path, capturing the refusal and
     not failing the turn, e.g. `(echo x >> /usr/local/bin/kairon) 2> write-refused.txt || true`, then
     `kairon --version > after.txt` to prove it still runs; checks `file_contains` on `write-refused.txt` with a
     pattern that accepts the kernel's refusal (`Read-only file system|Permission denied` — the file is owned by the
     host user so EACCES may precede EROFS; builder verifies which is produced and may tighten the pattern, noting
     that docs already distinguish EROFS from EACCES). The case must **pass** (unlike `stub-write-outside-mounts`,
     which is expected to fail).
   Extend `containmentCases`, `TestContainmentFixtures` and `TestContainmentSandbox` (new subtests, plus a
   `containment.kairon_binary == true` assertion on the AC 1 run's `selftest-sandbox.json`/`summary.json`).
   The `selftest-sandbox` run-level case count assertions (e.g. `len(cases) != len(want)`) must be updated.
7. **Docs** (`docs/evaluation.md` only; `docs/unified-container-flow.md` also states the old path and is stale if left
   — it is live documentation per AGENTS.md, update its two sentences too):
   - ASCII diagram (~2164) and mounts table (~2194): the binary is mounted for **all** backends at
     `/usr/local/bin/kairon`, read-only.
   - Backend table (~2236) and "The helper binary" section (~2240–2246): rename to "The `kairon` binary"; it is
     needed for **every** `--sandbox` run, including `kiro-cli`; failure now happens before any case starts, with
     the error naming `KAIRON_SANDBOX_BINARY`.
   - Read-only paragraph (~2353): `/usr/local/bin/kairon` replaces `/opt/kairon/kairon`.
   - Containment record: "four fields" → five; add `kairon_binary` row; update JSON examples (~1260) and the
     Containment-model table (~2582); mention the new reserved mock name (~121).
   - Self-test section (~1880, ~1970, ~2008): list the two new cases and their assertions; add a short note on the
     manual `kiro-cli` verification for AC 2 (below).

## Acceptance-criteria mapping

| AC | How it is met | How it is verified |
|----|---------------|--------------------|
| 1 | Unconditional ro mount at `/usr/local/bin/kairon` (on PATH) | `task eval:selftest:sandbox` → `stub-kairon-binary` passes `file_contains` on `.eval/kairon-version.txt`; hermetic unit test asserts mount for every backend |
| 2 | Condition on backend removed | Unit test `buildContainerMounts(..., NameKiroCLI)` includes the mount (5 mounts). Manual: `kairon eval <agent> --sandbox --backend kiro-cli --debug` with a case that fails (debug keeps the container only on failure — `Close(failed)`), then `docker inspect <container>` lists `/usr/local/bin/kairon` (RO) among Mounts |
| 3 | `ReadOnly: true` bind mount, non-owner uid | `task eval:selftest:sandbox` → `stub-kairon-binary-readonly` records refusal in `write-refused.txt`; unit test asserts `ReadOnly` |
| 4 | `Containment.KaironBinary` from `kaironBinaryMounted` | Unit tests (record, enforcement parity, diff); gated test asserts `kairon_binary: true` in AC 1 run's results |
| 5 | Pre-flight in `prepareBaseImage` | Unit test with stubbed `resolveLinuxBinary` error: `prepareBaseImage(&ContainerConfig{...})` (and `RunWithOptions` with `Sandbox`) returns the error naming `KAIRON_SANDBOX_BINARY` before the image manager is created and before any case runs. Manual: `KAIRON_SANDBOX_BINARY=/nonexistent` / no Go → same message, for stub and kiro-cli |

## Relevant Files

Modify:
- `internal/eval/runner.go` — `containerHelperPath` value + comment; `buildContainerMounts` unconditional mount
  honouring `kaironBinaryMounted`, neutral error text; `prepareBaseImage` pre-flight; doc comments mentioning
  "non-kiro-cli" helper.
- `internal/eval/containment.go` — `kaironBinaryMounted` const; set `KaironBinary` in `containmentFor`.
- `internal/eval/types.go` — `Containment.KaironBinary`.
- `internal/eval/diff_provenance.go` — `containmentDiff`, `describeContainment`.
- `internal/eval/workspace.go` — add `kairon` to `reservedMockCommands`.
- Tests to update: `container_runner_test.go` (`TestBuildContainerMountsKiroCLI` now expects 5 mounts incl. the
  binary; `TestBuildContainerMountsHelper` path; errors test), `container_agent_test.go:243`
  (`/opt/kairon/kairon inference-exec` → `/usr/local/bin/kairon inference-exec`), `containment_test.go`
  (`TestContainmentMatchesEnforcement` must call `useFakeLinuxBinary`; `containerResult` helper and JSON fixture at
  ~228), `diff_provenance_test.go:192` (exact `describeContainment` string), `containment_sandbox_test.go:353`
  (the kiro-cli mount call now needs the fake binary), any other `buildContainerMounts` callers (grep), mock
  validation tests (`mocks_test.go`) for the new reserved name.
- `internal/eval/testdata/evals/cases/selftest-sandbox/` — two new case YAMLs.
- `internal/eval/containment_sandbox_test.go` — `containmentCases`, fixtures test, gated subtests.
- `docs/evaluation.md`, `docs/unified-container-flow.md`.

Not touched / out of scope: any dispatch tool or MCP server; native (non-sandbox) runs; `sandbox/linuxbin.go`
resolution logic (reused as is); the base image (the binary is bind-mounted, never baked in, so the image cache
key is unchanged). The testdata eval cases are not in the template-sync mapping (only `.kairon/evals/**` is), so
`task sync:check` is unaffected — the builder still runs it to confirm. Do not edit `.kairon/specs/` other than
this file, `CHANGELOG.md`, etc. (AGENTS.md).

## Team Orchestration

All work lands in one PR. TDD per AGENTS.md: each code task writes failing tests first, confirms they fail for
the expected reason, then implements; test and implementation are committed together.

- `containment-record` (builder) — no dependencies; establishes the `kaironBinaryMounted` const and field.
- `mount-and-preflight` (builder) — depends on `containment-record` (uses the const); also owns the
  reserved-mock-name change and updating existing mount tests.
- `selftest-cases` (builder) — depends on `mount-and-preflight`; fixtures + gated tests (run under the daemon if
  available, otherwise they skip — state this in the sentinel).
- `docs` (builder or documenter) — depends on `containment-record` and `mount-and-preflight`; can run in parallel
  with `selftest-cases`.
- `validate` (validator) — last.

## Step-by-Step Task Breakdown

### Task 1: containment-record
Add `KaironBinary` to `Containment`, `kaironBinaryMounted` const, `containmentFor` population, diff/describe
updates. Tests first: record has `KaironBinary == true`; `containmentDiff` reports `kairon_binary: false → true`;
`describeContainment` string; JSON round-trip (field always written; absent in an old file decodes to false).

### Task 2: mount-and-preflight
Tests first (expected to fail for the right reason, i.e. assertion failures, not compile errors — add the new const
via Task 1 first):
- `buildContainerMounts` includes the binary read-only at `/usr/local/bin/kairon` for `kiro-cli` and `stub` (5 mounts)
  and its container path is a directory on `containerPath`; mount omitted only if `kaironBinaryMounted` were false.
- The mount is the only entry at that path and `ReadOnly`.
- `prepareBaseImage` returns an error containing `KAIRON_SANDBOX_BINARY` (via the `resolveLinuxBinary` seam) and
  does not create an image manager; `nil` cConfig still returns `nil, nil`.
- `validateMockCommand("kairon")` is rejected.
Then implement per Solution Approach 1, 3, 5, and update the existing tests listed above.

### Task 3: selftest-cases
Add the two cases and extend the gated tests (Solution Approach 6). Verify with
`KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestContainmentFixtures|TestContainmentSandbox' -count=1 -v`
(or `task eval:selftest:sandbox`) when a daemon is reachable; `TestContainmentFixtures` and
`TestSelfTest*Native` run without one. If no daemon is available, say so explicitly in the sentinel.

### Task 4: docs
Update `docs/evaluation.md` and `docs/unified-container-flow.md` as listed in Solution Approach 7. Documentation is
exempt from tests-first.

### Task 5: validate
Run the full gate, check each AC against the mapping table, and confirm the manual items are documented.

## Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/eval/... -count=1
task fmt:check
task lint
task test
task sync:check
# daemon-gated (skips with a message when no Podman/Docker daemon):
task eval:selftest:sandbox
# AC 5 manual check (no binary can be found/built):
#   KAIRON_SANDBOX_BINARY=/nonexistent kairon eval --sandbox --backend stub ... → fails before any case starts
```

## Risks / Notes

- File bind-mount at `/usr/local/bin/kairon` on a read-only rootfs: same runtime mechanism as the existing
  `/opt/kairon/kairon` mount, but a different parent directory; verified only by the gated self-test (cannot be
  verified without a daemon). Fallback documented in Solution Approach 1.
- Podman vs Docker may differ in the refusal text for the write attempt (EROFS vs EACCES) — the AC 3 check accepts both.
- `go test` binaries are excluded by the resolver, so tests cross-compile a real `kairon`; the first gated run is
  slower.
- `--debug` keeps the container only for a failed case (`Close(failed)`); AC 2's manual check should use a failing
  case.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "containment-record"
    agent: "builder"
    description: "Add the kairon_binary fact to the containment record: Containment.KaironBinary field, kaironBinaryMounted const in containment.go populated by containmentFor, and updates to describeContainment/containmentDiff. Write failing tests first (record value, diff line, describe string, JSON round-trip incl. old files without the field)."
    dependencies: []
    acceptance_criteria:
      - "Containment has KaironBinary bool with json tag kairon_binary, always written (not omitempty)"
      - "containmentFor sets KaironBinary from the kaironBinaryMounted const (true)"
      - "containmentDiff reports 'kairon_binary: false → true' and describeContainment includes kairon_binary=%t"
      - "Tests were observed failing before the implementation and pass after; existing containment/diff tests updated"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval -run 'Containment|Diff|Provenance' -count=1"
      - "task fmt:check"

  - id: "mount-and-preflight"
    agent: "builder"
    description: "Mount the linux kairon binary read-only at /usr/local/bin/kairon for every backend (set containerHelperPath to /usr/local/bin/kairon, drop the non-kiro-cli condition, honour kaironBinaryMounted, backend-neutral error text); add a resolveLinuxBinary pre-flight at the top of prepareBaseImage so a missing binary fails before any case starts; reserve 'kairon' in reservedMockCommands; update existing mount/agent/containment tests that assumed the old path or the kiro-cli exemption. Tests first."
    dependencies: ["containment-record"]
    acceptance_criteria:
      - "buildContainerMounts for inference.NameKiroCLI and inference.NameStub both include a ReadOnly mount of the resolved binary at /usr/local/bin/kairon (5 mounts); the old /opt/kairon/kairon mount no longer exists"
      - "The container path of the mount is in a directory listed on containerPath, and the stub backend still runs 'kairon inference-exec' via containerHelperPath"
      - "prepareBaseImage returns, before creating the image manager, an error containing KAIRON_SANDBOX_BINARY and the platform when resolveLinuxBinary fails; prepareBaseImage(nil) still returns nil, nil"
      - "A case mock named 'kairon' is rejected at load with a reserved-command error"
      - "The o+rx mode check remains and its error names the file; wording is backend-neutral"
      - "Tests were observed failing before the implementation and pass after; no existing test is deleted to make the build pass"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"
      - "task fmt:check"
      - "task lint"

  - id: "selftest-cases"
    agent: "builder"
    description: "Add two requires_sandbox sandbox self-test cases to the selftest-sandbox agent: stub-kairon-binary (runs 'kairon --version > .eval/kairon-version.txt' and 'command -v kairon > kairon-path.txt', file_contains checks on both) and stub-kairon-binary-readonly (attempts to write /usr/local/bin/kairon capturing the refusal in write-refused.txt without failing the turn, then runs kairon --version; file_contains check for the refusal). Extend containmentCases, TestContainmentFixtures and TestContainmentSandbox, including an assertion that the run's containment record has kairon_binary true."
    dependencies: ["mount-and-preflight"]
    acceptance_criteria:
      - "Both case YAMLs load, are requires_sandbox: true, belong to agent selftest-sandbox, and TestContainmentFixtures expects them"
      - "Under a container daemon, stub-kairon-binary passes its file_contains checks and stub-kairon-binary-readonly passes (refusal recorded as 'Read-only file system' or 'Permission denied')"
      - "TestContainmentSandbox asserts containment.kairon_binary == true in the results of the run that executed stub-kairon-binary"
      - "A native run still refuses both cases with 'requires --sandbox' (TestContainmentNativeRefusal passes)"
      - "Without a daemon the gated tests skip and the always-run tests pass"
    validation_commands:
      - "go test ./internal/eval -run 'TestContainmentFixtures|TestContainmentNativeRefusal|TestContainmentMockCLIFixtureParity' -count=1"
      - "task eval:selftest:sandbox"
      - "task sync:check"

  - id: "docs"
    agent: "builder"
    description: "Update docs/evaluation.md (and the stale path statements in docs/unified-container-flow.md): binary mounted for all backends read-only at /usr/local/bin/kairon on PATH; 'helper binary' section renamed and now applies to kiro-cli; pre-flight failure before any case; containment record has five fields incl. kairon_binary (table, JSON examples, containment-model table); 'kairon' is a reserved mock name; new self-test cases and their assertions; manual AC 2 docker-inspect verification (debug keeps container only on failure)."
    dependencies: ["containment-record", "mount-and-preflight"]
    acceptance_criteria:
      - "No remaining reference to /opt/kairon/kairon or 'non-kiro-cli backends only' for the binary in docs/evaluation.md or docs/unified-container-flow.md"
      - "The containment record is documented with five fields and the JSON examples include kairon_binary"
      - "The binary-resolution section states that every --sandbox run needs the binary and the failure occurs before any case starts, naming KAIRON_SANDBOX_BINARY"
      - "The new self-test cases and the reserved mock name 'kairon' are documented"
    validation_commands:
      - "! grep -rn '/opt/kairon/kairon' docs/evaluation.md docs/unified-container-flow.md"
      - "grep -n 'kairon_binary' docs/evaluation.md"
      - "go test ./internal/eval -run 'Docs|docs' -count=1"

  - id: "validate"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #341 against the implementation: mounts for all backends, read-only, on PATH, containment record, pre-flight failure, self-test cases, docs; run the full project gate."
    dependencies: ["selftest-cases", "docs"]
    acceptance_criteria:
      - "go build, go vet, task lint, task fmt:check, task test and task sync:check all pass"
      - "buildContainerMounts includes the read-only kairon mount for kiro-cli and stub backends (unit tests green)"
      - "Containment record carries kairon_binary: true for container runs and is absent for native runs"
      - "prepareBaseImage fails before any case starts when the linux binary cannot be resolved, with a message naming KAIRON_SANDBOX_BINARY"
      - "task eval:selftest:sandbox passes if a daemon is reachable; otherwise the report states it skipped and why"
      - "AGENTS.md TDD rules were followed (commit message or sentinel names the test seen failing first) and no historical artifact was edited"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "task fmt:check"
      - "task lint"
      - "task test"
      - "task sync:check"
      - "task eval:selftest:sandbox"
```
