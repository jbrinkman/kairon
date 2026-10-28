# Design Specification: Prune redundant internal/eval/sandbox tests

Closes #434

## Scope Coverage

Task IDs refer to the `kiro-plan` block at the end of this spec. `verify-pruning` re-checks every row.

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/eval/sandbox`: `architecture_test.go`, `container_daemon_test.go`, `container_test.go`, `installation_test.go`, `integration_installation_test.go`, `mounts_test.go`" | `prune-architecture`, `prune-daemon`, `prune-container`, `prune-installation`, `prune-integration-installation`, `prune-mounts` |
| T1 | Table: "`TestDetectHostArchitecture` (architecture_test.go:14) Delete" | `prune-architecture`, `verify-pruning` |
| T2 | Table: "`TestArchitectureDetectionInContainer` (architecture_test.go:148) Delete" | `prune-architecture`, `verify-pruning` |
| T3 | Table: "`TestPlatformSpecificImagePulling` (architecture_test.go:198) Delete" | `prune-architecture`, `verify-pruning` |
| T4 | Table: "`TestKiroCLIInstallationMocking` (architecture_test.go:234) Delete" | `prune-architecture`, `verify-pruning` |
| T5 | Table: "`TestPodmanSocketCandidates_LinuxRoot` (container_daemon_test.go:87) Merge into partner `TestPodmanSocketCandidates_Linux`" | `prune-daemon`, `verify-pruning` |
| T6 | Table: "`TestPodmanSocketCandidates_Darwin` (container_daemon_test.go:92) Merge into partner `TestPodmanSocketCandidates_Linux`" | `prune-daemon`, `verify-pruning` |
| T7 | Table: "`TestPodmanSocketCandidates_UnknownOS` (container_daemon_test.go:102) Merge into partner `TestPodmanSocketCandidates_Linux`" | `prune-daemon`, `verify-pruning` |
| T8 | Table: "`TestPodmanDiscovery_DefaultEndpointWins` (container_daemon_test.go:177) Delete" | `prune-daemon`, `verify-pruning` |
| T9 | Table: "`TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden` (container_daemon_test.go:189) Delete" | `prune-daemon`, `verify-pruning` |
| T10 | Table: "`TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker` (container_daemon_test.go:198) Delete" | `prune-daemon`, `verify-pruning` |
| T11 | Table: "`TestContainerTimeout` (container_test.go:204) Delete" | `prune-container`, `verify-pruning` |
| T12 | Table: "`TestExecWithOutput_ErrorHandling` (container_test.go:364) Merge into partner `TestContainer_ExecWithStdin`" | `prune-container`, `verify-pruning` |
| T13 | Table: "`TestKiroCLIDownloadURL_SupportedPlatforms` (installation_test.go:18) Delete" | `prune-installation`, `verify-pruning` |
| T14 | Table: "`TestInstallationVerification_PermissionChecks` (installation_test.go:63) Delete" | `prune-installation`, `verify-pruning` |
| T15 | Table: "`TestPlatformSpecificBinaries` (installation_test.go:151) Delete" | `prune-installation`, `verify-pruning` |
| T16 | Table: "`TestDetectHostArchitecture_Unsupported` (installation_test.go:208) Delete" | `prune-installation`, `verify-pruning` |
| T17 | Table: "`TestValidateKiroCLI_Verification` (installation_test.go:215) Delete" | `prune-installation`, `verify-pruning` |
| T18 | Table: "`TestResourceLimits_Coverage` (installation_test.go:229) Delete" | `prune-installation`, `verify-pruning` |
| T19 | Table: "`TestKiroCLIVerification_Detailed` (installation_test.go:245) Merge into partner `TestRuntimeVerification_DoesNotInstall`" | `prune-installation`, `verify-pruning` |
| T20 | Table: "`TestKiroCLINotInstalledInBaseImage` (integration_installation_test.go:21) Delete" | `prune-integration-installation`, `verify-pruning` |
| T21 | Table: "`TestNewHostConfigWithMounts_NoWorkspaceTmpfs` (mounts_test.go:71) Merge into partner `TestNewHostConfigWithMounts_Tmpfs`" | `prune-mounts`, `verify-pruning` |
| T22 | Table: "`TestTmpfsMounts_NoOwnershipOptions` (mounts_test.go:162) Delete" | `prune-mounts`, `verify-pruning` |
| T23 | Table: "`TestWithOpenUmask_Shape` (mounts_test.go:321) Delete" | `prune-mounts`, `verify-pruning` |
| T24 | Table: "`TestWithOpenUmask_DoesNotMutateInput` (mounts_test.go:334) Delete" | `prune-mounts`, `verify-pruning` |
| B1 | Behavior-Preservation Contract: "No production code changes. Every test not listed passes unchanged. Merged partners keep every assertion they had." | `prune-architecture`, `prune-daemon`, `prune-container`, `prune-installation`, `prune-integration-installation`, `prune-mounts`, `verify-pruning` |
| AC1 | "The 18 tests marked Delete no longer exist." | `prune-architecture`, `prune-daemon`, `prune-container`, `prune-installation`, `prune-integration-installation`, `prune-mounts`, `verify-pruning` |
| AC2 | "Each of the 6 tests marked Merge is a case or subtest of its partner, and no longer a separate function." | `prune-daemon`, `prune-container`, `prune-installation`, `prune-mounts`, `verify-pruning` |
| AC3 | "Package coverage does not drop." | `record-baseline`, `record-coverage-after`, `verify-pruning` |
| AC4 | "Every remaining test passes unchanged, and the gate passes." | `record-coverage-after`, `verify-pruning` |

## Deferred work

none

## Solution Approach

This is a test-only refactor of `internal/eval/sandbox`. It deletes 18 tests, folds 6 tests into named partner tests, and changes no production code. The issue's Current / Target State table is the contract: every row is carried out exactly as written, and no test outside the table is touched.

Under AGENTS.md the refactor rules apply: the behavior must be pinned before it changes, and the pinning tests must pass before and after.

1. **Record the baseline** (`record-baseline`). Run the package with a coverage profile and save it. It is the "before" half of AC3.
2. **For each of the 6 merges, add the folded case to the partner first and run it green. Then delete the old function.** The partner is the characterization test. Its folded case must pass before the old function disappears.
3. **Delete the 18 pure deletions.** The partner named in the table already asserts the behavior. The issue records this verdict and it is not re-litigated here.
4. **Clean up what the deletions strand.** Unused imports break compilation. Helpers whose only caller was a deleted test are dead code. Every such case is listed under Hazards.
5. **Record the "after" coverage and run the gate** (`record-coverage-after`, then `verify-pruning`).

### Measured baseline (architect, this worktree at `a1aecb0`, darwin/arm64, Podman daemon running)

| Measurement | Result |
|-------------|--------|
| `go test -count=1 -cover ./internal/eval/sandbox` | `ok ... 143.973s coverage: 66.1% of statements` |
| Exact statements | 399 covered of 604 (66.060%) |
| Same run with all 24 tests skipped (`go test -skip '^(...24 names...)$'`) | 399 of 604, 81.3s |

The second row matters for the gate. The 66.1% displayed is rounded from 66.060%, so there is no headroom: losing one covered statement shows 65.9% and fails AC3. Skipping all 24 tests (merged tests included, which is the worst case) left 399 of 604 covered. Pruning therefore should not lose a statement, and the merges can only add coverage. Two caveats:

- One covered block flips between runs inside `registry.go` `load()`. The `ReadFile` error branch is covered on a first run when no `.kairon/evals/tmp/containers.json` exists. The `Unmarshal` return is covered on later runs. Each is one statement, so the count stays 399. Compare covered-statement counts, not block lists.
- The numbers depend on the host. Without a container daemon every `skipIfNoContainerDaemon` test skips and coverage is lower. Record the baseline and the "after" run on the same machine with a daemon running.

## Relevant Files

All changes are in `internal/eval/sandbox/`:

| File | Change |
|------|--------|
| `architecture_test.go` | Delete 4 tests. Remove the `runtime` and `strings` imports. |
| `container_daemon_test.go` | Delete 3 tests. Fold 3 into `TestPodmanSocketCandidates_Linux`. Remove the `strings` import. |
| `container_test.go` | Delete 1 test. Fold 1 into `TestContainer_ExecWithStdin`. No import change. |
| `installation_test.go` | Delete 6 tests. Fold 1 into `TestRuntimeVerification_DoesNotInstall`. Remove the `os` and `path/filepath` imports. |
| `integration_installation_test.go` | Delete the one test and its helper `kiroCLIMissing`. The file is then empty, so remove it with `git rm`. |
| `mounts_test.go` | Delete 3 tests. Fold 1 into `TestNewHostConfigWithMounts_Tmpfs`. No import change. |

Reference only (read, do not edit):

- `daemon.go` (`podmanSocketCandidates`, `discoverDaemonHost`) and `container.go` (`DetectHostArchitecture`, `ValidateKiroCLI`, `verifyKiroCLIInstallation`, `ExecWithOutput`): production code, unchanged.
- `baseimage_test.go` (`TestToolSetBuildArgs`), `integration_architecture_test.go` (`TestArchitectureIntegration_*`): partners that stay as they are.
- `.kairon/research/test-audit-verdicts.md` and `.kairon/research/test-performance-analysis.md`: the source of the verdicts.

## Hazards

### Imports that become unused (compile errors)

| File | Remove | Why |
|------|--------|-----|
| `architecture_test.go` | `"runtime"` | Used only by `TestDetectHostArchitecture`. |
| `architecture_test.go` | `"strings"` | Used only by `TestArchitectureDetectionInContainer` and `TestKiroCLIInstallationMocking`. |
| `container_daemon_test.go` | `"strings"` | Used only by `TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker`. |
| `installation_test.go` | `"os"` and `"path/filepath"` | Used only by `TestInstallationVerification_PermissionChecks`. |

After the edits `architecture_test.go` imports `context`, `testing`, `container`, `assert` and `require`. Imports that stay and must not be removed: `fmt`, `runtime`, `os`, `time`, `net` and `net/http` in `container_daemon_test.go`; `context`, `time`, `container`, `assert` and `require` in `installation_test.go`; every import in `container_test.go` and `mounts_test.go`. A new import is needed only if a snippet below needs one. The folded cases need none.

### Dead helpers (not flagged by `go vet`, but they would be dead code)

- `kiroCLIMissing` in `integration_installation_test.go` is called only by `TestKiroCLINotInstalledInBaseImage` (checked with `grep -rn kiroCLIMissing`). Delete it with the test. Nothing else is left in the file, so `git rm` it.
- In `container_daemon_test.go`, `ensureContainerDaemon`, `noDefaultDaemon`, `startFakePodmanSocket` and `noContainerDaemonMessage` all keep a caller after the deletions (`TestPodmanDiscovery_SelectsFakePodmanSocket`, `TestPodmanDiscovery_DeadCandidateIsNotSelected`, `skipIfNoContainerDaemon`). Keep them all.
- `mustHostConfig`, `realTempDir`, `skipIfNoContainerDaemon` and `errAfterReader` keep callers. Keep them.
- The builder must re-run `grep -rn '<helper>' internal/eval/sandbox/` for any helper it suspects after its edits.

### Build tag on five partners

The partners `TestArchitectureIntegration_CurrentArchitectureConsistency`, `_ContainerLifecycle` and `_KiroCLIInstallation` live in `integration_architecture_test.go`, which has `//go:build integration`. `task test` (`go test -v -race -coverprofile=coverage.out ./...`) does not set that tag, so these partners run only with `-tags integration`. The table names them as partners for T1 to T4 and T16, and this spec follows the table. Two consequences for the work:

- Nothing in this refactor edits or retags `integration_architecture_test.go`.
- `verify-pruning` runs `go vet -tags integration ./internal/eval/sandbox` and the cheap, daemon-free partner (`TestArchitectureIntegration_CurrentArchitectureConsistency`) under the tag. This confirms that the tagged file still compiles and passes. The measured coverage (399 of 604 with all 24 tests skipped) shows `task test` itself loses no statement.

### Other

- `TestPodmanSocketCandidates_LinuxNoXDGFallsBackToUID`, `TestExistingSockets_FiltersMissingAndNonSockets`, `TestInstallationFailures_ErrorHandling`, `TestKiroCLIInstallation_VerificationLogic`, `TestResourceLimitsEnforcement` and every other test not in the table stay exactly as they are. `TestPodmanSocketCandidates_LinuxNoXDGFallsBackToUID` looks like another table row but is not in the issue's table, so it is not folded in.
- Subtests run sequentially and share the parent's container and context. Do not add `t.Parallel()`.
- `TestTmpfsMounts_NoOwnershipOptions` (T22) is deleted as the issue directs. Its one exact-string check of the `/home/sandbox` options is covered by the partner's per-option check on the same entry, together with the partner's no-`uid=`/`gid=` loop and the exactly-3-entries check.
- Stage only the six files named under Relevant Files. The worktree already has unrelated modified files under `.kiro/agents/`. They are not part of this change.

## The 6 merges, precisely

In every merge the partner keeps all of its existing assertions, unchanged. The folded case is added as a subtest (a table row run through `t.Run`, or a `t.Run` block), so it is visible in `go test -run '^<partner>$' -v` output as required by AC2. Names below are the `t.Run` names. Go prints spaces as `_` in `-v` output. Code is illustrative: the builder adapts it to the surrounding style.

### M1 to M3: `TestPodmanSocketCandidates_LinuxRoot`, `_Darwin`, `_UnknownOS` into `TestPodmanSocketCandidates_Linux` (container_daemon_test.go)

The partner is one fixed call today. Turn it into a table over `(goos, getenv, uid) -> []string`. Keep the name `TestPodmanSocketCandidates_Linux`.

| Subtest name | goos | getenv | uid | Want |
|--------------|------|--------|-----|------|
| `xdg_runtime_dir` (the partner's existing case) | `linux` | map `{"XDG_RUNTIME_DIR": "/run/user/1000"}` | 1000 | `/run/user/1000/podman/podman.sock`, `/run/podman/podman.sock` |
| `root_uid_0` (from `_LinuxRoot`) | `linux` | always `""` | 0 | `/run/podman/podman.sock` |
| `darwin` (from `_Darwin`) | `darwin` | map `{"TMPDIR": "/var/folders/ab/xyz/T/", "HOME": "/Users/alice"}` | 501 | `/var/folders/ab/xyz/T/podman/podman-machine-default-api.sock`, `/Users/alice/.local/share/containers/podman/machine/podman.sock`, `/Users/alice/.local/share/containers/podman/machine/podman-machine-default/podman.sock` |
| `unknown_os` (from `_UnknownOS`) | `plan9` | always `"x"` (every key) | 1 | empty |

Hazards:

- Use a `getenv func(string) string` field in the row, not a map. The `unknown_os` row returns `"x"` for every key, which a map cannot express faithfully.
- The unknown-OS original uses `assert.Empty`, which accepts `nil` and `[]string{}`. `assert.Equal([]string(nil), []string{})` fails. In the loop, use `assert.Empty` when `len(tt.want) == 0` and `assert.Equal` otherwise.
- Keep the exact expected paths above. They are copied from the three original tests.

Expected `-v` PASS lines: `TestPodmanSocketCandidates_Linux/xdg_runtime_dir`, `/root_uid_0`, `/darwin`, `/unknown_os`.

### M4: `TestExecWithOutput_ErrorHandling` into `TestContainer_ExecWithStdin` (container_test.go)

Add one `t.Run("ExecWithOutput reports a nonexistent binary", ...)` after the existing "ExecWithOutput semantics unchanged" subtest and before the cancellation subtest. It runs in the partner's existing container, which is created with `&container.HostConfig{}` exactly as the deleted test's container was. Content:

1. `out, err := c.ExecWithOutput(ctx, []string{"echo", "success"})`: `assert.NoError`, `assert.Contains(out, "success")`.
2. `_, err = c.ExecWithOutput(ctx, []string{"nonexistent-command"})`: `assert.Error(t, err, "non-existent command should return error")`.

The deleted test's `false` -> `exit code 1` assertion is dropped. The partner's "ExecWithOutput semantics unchanged" subtest already asserts the exact string `command failed with exit code 2: boom`, which covers non-zero exit-code reporting (this is the issue's rule: "drop anything the partner already asserts"). The success check is kept because no partner subtest asserts a successful `ExecWithOutput`. This removes one 10-second container.

Expected `-v` PASS line: `TestContainer_ExecWithStdin/ExecWithOutput_reports_a_nonexistent_binary`.

### M5: `TestKiroCLIVerification_Detailed` into `TestRuntimeVerification_DoesNotInstall` (installation_test.go)

The partner calls `c.ValidateKiroCLI(...)` and asserts `Error`. Keep that assertion at the top level of the test. Hoist `ctx := context.Background()` to a variable. Then add:

```go
t.Run("verifyKiroCLIInstallation error names kiro-cli", func(t *testing.T) {
    err := c.verifyKiroCLIInstallation(ctx)
    require.Error(t, err, "Should fail when no container is running")
    assert.Contains(t, err.Error(), "kiro-cli")
})
```

Why a direct call and not a message check on the `ValidateKiroCLI` error: `ValidateKiroCLI` wraps with the prefix `kiro-cli verification failed:`, so `Contains "kiro-cli"` on the wrapped error would pass whatever the inner message says. The deleted test checked the inner error, and the subtest keeps that strength. `require.Error` is used so a nil error fails the subtest instead of panicking on `err.Error()`. The original `assert.Error` followed by `err.Error()` had that panic.

Expected `-v` PASS line: `TestRuntimeVerification_DoesNotInstall/verifyKiroCLIInstallation_error_names_kiro-cli`.

### M6: `TestNewHostConfigWithMounts_NoWorkspaceTmpfs` into `TestNewHostConfigWithMounts_Tmpfs` (mounts_test.go)

Both tests build `hc` from `newHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: ws, ContainerPath: "/workspace"}}, false)`, so the partner's `hc` is already the input the deleted test used. Keep all existing assertions at the top level. Append at the end of the function:

```go
t.Run("no tmpfs at or under the workspace", func(t *testing.T) {
    if _, ok := hc.Tmpfs["/workspace"]; ok {
        t.Error("host config must not have a tmpfs at /workspace")
    }
    for p := range hc.Tmpfs {
        if p == "/workspace" || strings.HasPrefix(p, "/workspace/") {
            t.Errorf("tmpfs entry %q must not be at or under the workspace", p)
        }
    }
})
```

`strings` is already imported. The subtest name has no `/` so `-v` output stays unambiguous.

Expected `-v` PASS line: `TestNewHostConfigWithMounts_Tmpfs/no_tmpfs_at_or_under_the_workspace`.

## Team Orchestration

| Layer | Task | Agent | Why it sits here |
|-------|------|-------|------------------|
| 0 | `record-baseline` | builder | Must run on the untouched tree. |
| 1 | `prune-architecture` | builder | |
| 2 | `prune-daemon` | builder | |
| 3 | `prune-container` | builder | |
| 4 | `prune-installation` | builder | |
| 5 | `prune-integration-installation` | builder | |
| 6 | `prune-mounts` | builder | |
| 7 | `record-coverage-after` | builder | Needs every edit in place. |
| 8 | `verify-pruning` | validator | Independent read-only check of every row of Scope Coverage. |

The six edit tasks touch different files but one Go package, and each task's `go vet` compiles every file in the package. Running them in parallel would let one builder's half-finished edit break another's compile check, so they are chained. Each is small. No task needs the documenter: the only references to these tests outside the package are in `.kairon/research/`, which describes the plan, not the current test list.

Each edit task follows the same protocol:

1. For a merge: add the folded case to the partner and run it green.
2. Delete the old function(s). A function's doc comment goes with it.
3. Fix imports; remove dead helpers.
4. Run `gofmt -l`, `go vet`, `go vet -tags integration` and the focused `go test` from the task's `validation_commands`.
5. Commit only when the package compiles and the focused tests pass. The commit message names the tests relied on (refactor rule: "say what you observed"), for example "relied on `TestPodmanSocketCandidates_Linux` (with the folded rows) and `TestDiscoverDaemonHost_*`".

## Step-by-Step Task Breakdown

### Task 1: `record-baseline`
Save the "before" coverage profile and the pass list for the partners, on the untouched tree.
- Run `go test -count=1 -cover -coverprofile=.kairon/artifacts/issue-434-coverage-before.out ./internal/eval/sandbox`, and write the covered/total statements and displayed percentage to `.kairon/artifacts/issue-434-coverage-before.txt`. These files live in the gitignored artifacts directory and carry the numbers to later tasks and into the PR description.
- Expect about 66.1% (399 of 604) on a host with a daemon. If the host has no container daemon, report that in the sentinel: AC3 cannot be evidenced there, and `verify-pruning` must run on a host with one.
- Confirm that every partner named in the issue's table exists and passes before any change (the commands in the task).
- No source change.

### Task 2: `prune-architecture` (T1 to T4)
In `architecture_test.go`, delete `TestDetectHostArchitecture`, `TestArchitectureDetectionInContainer`, `TestPlatformSpecificImagePulling` and `TestKiroCLIInstallationMocking`. Remove the `runtime` and `strings` imports. `TestKiroCLIVersionedURL_PlatformMapping` and `TestCreateWithPlatform` stay.

### Task 3: `prune-daemon` (T5 to T10)
In `container_daemon_test.go`, rewrite `TestPodmanSocketCandidates_Linux` as the table in M1 to M3, then delete `_LinuxRoot`, `_Darwin` and `_UnknownOS`. Delete `TestPodmanDiscovery_DefaultEndpointWins`, `TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden` and `TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker`. Remove the `strings` import.

### Task 4: `prune-container` (T11, T12)
In `container_test.go`, add the M4 subtest to `TestContainer_ExecWithStdin`, then delete `TestExecWithOutput_ErrorHandling` and `TestContainerTimeout`.

### Task 5: `prune-installation` (T13 to T19)
In `installation_test.go`, add the M5 subtest to `TestRuntimeVerification_DoesNotInstall`, then delete `TestKiroCLIVerification_Detailed`. Delete `TestKiroCLIDownloadURL_SupportedPlatforms`, `TestInstallationVerification_PermissionChecks`, `TestPlatformSpecificBinaries`, `TestDetectHostArchitecture_Unsupported`, `TestValidateKiroCLI_Verification` and `TestResourceLimits_Coverage`. Remove the `os` and `path/filepath` imports. The comment above `TestKiroCLIDownloadURL_SupportedPlatforms` ("The platform -> kiro-cli download URL mapping lives in kiroCLIVersionedURL...") describes that test and goes with it.

### Task 6: `prune-integration-installation` (T20)
Delete `TestKiroCLINotInstalledInBaseImage` and its helper `kiroCLIMissing`. The file is then empty, so `git rm internal/eval/sandbox/integration_installation_test.go`.

### Task 7: `prune-mounts` (T21 to T24)
In `mounts_test.go`, add the M6 subtest to `TestNewHostConfigWithMounts_Tmpfs`, then delete `TestNewHostConfigWithMounts_NoWorkspaceTmpfs`, `TestTmpfsMounts_NoOwnershipOptions` (with its doc comment), `TestWithOpenUmask_Shape` and `TestWithOpenUmask_DoesNotMutateInput`. `TestWithOpenUmask_PreservesArguments` and `_AppliesUmask` stay.

### Task 8: `record-coverage-after` (AC3, AC4)
Run the package with a profile to `.kairon/artifacts/issue-434-coverage-after.out`, compare it with the "before" profile, and write the result to `.kairon/artifacts/issue-434-coverage-after.txt`. The covered-statement count must not drop and the total must be identical (identical total also shows no production code change). If the covered count is one lower, re-run once and compare per block first: the `registry.go` `load()` flip described above does not change the count.

### Task 9: `verify-pruning` (all rows)
The validator runs, read-only, every Verify command from the issue (in the corrected form below), the per-partner `-v` checks, the coverage comparison, `go vet -tags integration`, the diff-scope check, and `task test`. It reports PASS or FAIL per Scope Coverage row.

## Notes on the issue's Verify commands

The Verify lines of AC1 and AC2 as written contain `\\(` before the closing quote (probably a markdown escape). Run as written, `grep -E` rejects it with `parentheses not balanced`, so it can never print matches. The plan uses `\(`, which matches `func TestName(` and prints the expected matches on the untouched tree. This changes how the criterion is written, not what it requires: the same 18 and 6 names, the same files. The commands are wrapped in `sh -c '...; [ $? -eq 1 ]'` so that "no match" (exit 1) passes, a match (exit 0) fails, and a grep error (exit 2) also fails.

## Validation Commands

Run from the repository root. The `-run` commands for the partner tests need a container daemon.

```bash
# AC1: the 18 deleted tests are gone (prints nothing, passes on exit 1 from grep)
sh -c 'grep -nE "func (TestArchitectureDetectionInContainer|TestContainerTimeout|TestDetectHostArchitecture|TestDetectHostArchitecture_Unsupported|TestInstallationVerification_PermissionChecks|TestKiroCLIDownloadURL_SupportedPlatforms|TestKiroCLIInstallationMocking|TestKiroCLINotInstalledInBaseImage|TestPlatformSpecificBinaries|TestPlatformSpecificImagePulling|TestPodmanDiscovery_DefaultEndpointWins|TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden|TestResourceLimits_Coverage|TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker|TestTmpfsMounts_NoOwnershipOptions|TestValidateKiroCLI_Verification|TestWithOpenUmask_DoesNotMutateInput|TestWithOpenUmask_Shape)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'

# AC2: the 6 merged tests are no longer separate functions
sh -c 'grep -nE "func (TestExecWithOutput_ErrorHandling|TestKiroCLIVerification_Detailed|TestNewHostConfigWithMounts_NoWorkspaceTmpfs|TestPodmanSocketCandidates_Darwin|TestPodmanSocketCandidates_LinuxRoot|TestPodmanSocketCandidates_UnknownOS)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'

# AC2: each partner passes with the merged case among its subtests
go test -count=1 -run '^TestPodmanSocketCandidates_Linux$' -v ./internal/eval/sandbox
go test -count=1 -run '^TestContainer_ExecWithStdin$' -v ./internal/eval/sandbox
go test -count=1 -run '^TestRuntimeVerification_DoesNotInstall$' -v ./internal/eval/sandbox
go test -count=1 -run '^TestNewHostConfigWithMounts_Tmpfs$' -v ./internal/eval/sandbox

# AC3: coverage does not drop (displayed value at least 66.1%)
go test -count=1 -cover ./internal/eval/sandbox

# AC4: the gate
task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "record-baseline"
    agent: "builder"
    description: "On the untouched tree, record the 'before' coverage profile for internal/eval/sandbox and confirm every partner test named in the issue's table exists and passes. No source change."
    dependencies: []
    acceptance_criteria:
      - "Files .kairon/artifacts/issue-434-coverage-before.out and issue-434-coverage-before.txt exist; the .txt states covered/total statements (expected 399/604) and the displayed percentage (expected 66.1%) and the host (OS/arch, daemon present)"
      - "All partner tests named in the issue's table exist and pass before any change: TestArchitectureIntegration_CurrentArchitectureConsistency (with -tags integration), TestPodmanSocketCandidates_Linux, TestDiscoverDaemonHost_DefaultEndpointAnswers, TestDiscoverDaemonHost_ExplicitDockerHostSkipsProbing, TestContainer_ExecWithStdin, TestKiroCLIVersionedURL_PlatformMapping, TestToolSetBuildArgs, TestRuntimeVerification_DoesNotInstall, TestResourceLimits, TestNewHostConfigWithMounts_Tmpfs, TestWithOpenUmask_PreservesArguments, TestWithOpenUmask_AppliesUmask"
      - "If no container daemon is reachable, the sentinel says so, because AC3 cannot then be evidenced on this host"
      - "No tracked file is modified by this task"
    validation_commands:
      - "go test -count=1 -cover -coverprofile=.kairon/artifacts/issue-434-coverage-before.out ./internal/eval/sandbox"
      - "test -s .kairon/artifacts/issue-434-coverage-before.out"
      - |-
        awk 'NR>1{t+=$2; if($3>0)c+=$2} END{printf "%d/%d = %.3f%%\n", c, t, 100*c/t}' .kairon/artifacts/issue-434-coverage-before.out | tee .kairon/artifacts/issue-434-coverage-before.txt
      - "go test -count=1 -tags integration -run '^TestArchitectureIntegration_CurrentArchitectureConsistency$' ./internal/eval/sandbox"

  - id: "prune-architecture"
    agent: "builder"
    description: "In architecture_test.go delete TestDetectHostArchitecture, TestArchitectureDetectionInContainer, TestPlatformSpecificImagePulling and TestKiroCLIInstallationMocking, and remove the now-unused runtime and strings imports. TestKiroCLIVersionedURL_PlatformMapping and TestCreateWithPlatform stay unchanged."
    dependencies: ["record-baseline"]
    acceptance_criteria:
      - "The 4 named functions no longer exist in internal/eval/sandbox/*_test.go"
      - "architecture_test.go imports only context, testing, container, assert and require, and the package compiles with and without -tags integration"
      - "TestKiroCLIVersionedURL_PlatformMapping and TestCreateWithPlatform are unchanged and pass"
      - "No file other than architecture_test.go is modified by this task; gofmt -l reports nothing"
    validation_commands:
      - |-
        sh -c 'grep -nE "func (TestDetectHostArchitecture|TestArchitectureDetectionInContainer|TestPlatformSpecificImagePulling|TestKiroCLIInstallationMocking)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'
      - |-
        sh -c 'test -z "$(gofmt -l internal/eval/sandbox)"'
      - "go vet ./internal/eval/sandbox"
      - "go vet -tags integration ./internal/eval/sandbox"
      - "go test -count=1 -run '^(TestKiroCLIVersionedURL_PlatformMapping|TestCreateWithPlatform)$' ./internal/eval/sandbox"

  - id: "prune-daemon"
    agent: "builder"
    description: "In container_daemon_test.go, rewrite TestPodmanSocketCandidates_Linux as a table with subtests xdg_runtime_dir (existing case), root_uid_0, darwin and unknown_os (folding TestPodmanSocketCandidates_LinuxRoot, _Darwin and _UnknownOS with their exact inputs and expected paths), then delete those three functions. Delete TestPodmanDiscovery_DefaultEndpointWins, TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden and TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker. Remove the now-unused strings import. Keep ensureContainerDaemon, noDefaultDaemon, startFakePodmanSocket and noContainerDaemonMessage, which still have callers. Do not touch TestPodmanSocketCandidates_LinuxNoXDGFallsBackToUID."
    dependencies: ["prune-architecture"]
    acceptance_criteria:
      - "TestPodmanSocketCandidates_Linux is a table whose rows run through t.Run with the names xdg_runtime_dir, root_uid_0, darwin and unknown_os, each with the exact inputs and expected paths of the original tests (getenv is a func field; unknown_os returns \"x\" for every key and expects an empty result checked with assert.Empty; non-empty wants use assert.Equal)"
      - "The 6 named functions (LinuxRoot, Darwin, UnknownOS, DefaultEndpointWins, ExplicitDockerHostIsNotOverridden, MessageNamesPodmanAndDocker) no longer exist"
      - "The strings import is removed and no other import is removed; TestPodmanSocketCandidates_LinuxNoXDGFallsBackToUID and all other tests in the file are unchanged"
      - "go test -v on TestPodmanSocketCandidates_Linux prints PASS for all four subtests"
      - "gofmt -l reports nothing; go vet passes with and without -tags integration"
    validation_commands:
      - |-
        sh -c 'grep -nE "func (TestPodmanSocketCandidates_LinuxRoot|TestPodmanSocketCandidates_Darwin|TestPodmanSocketCandidates_UnknownOS|TestPodmanDiscovery_DefaultEndpointWins|TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden|TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'
      - |-
        sh -c 'go test -count=1 -run "^TestPodmanSocketCandidates_Linux\$" -v ./internal/eval/sandbox | grep -cE -- "--- PASS: TestPodmanSocketCandidates_Linux/(xdg_runtime_dir|root_uid_0|darwin|unknown_os) " | grep -qx 4'
      - "go test -count=1 -run '^(TestPodmanSocketCandidates_Linux|TestPodmanSocketCandidates_LinuxNoXDGFallsBackToUID|TestExistingSockets_FiltersMissingAndNonSockets|TestPodmanDiscovery_SelectsFakePodmanSocket|TestPodmanDiscovery_DeadCandidateIsNotSelected|TestDiscoverDaemonHost_DefaultEndpointAnswers|TestDiscoverDaemonHost_SelectsFirstLivePodmanSocket|TestDiscoverDaemonHost_ExplicitDockerHostSkipsProbing|TestDiscoverDaemonHost_NothingReachable|TestDaemonNotRunningError_NamesBothRuntimesAndWraps)$' ./internal/eval/sandbox"
      - |-
        sh -c 'test -z "$(gofmt -l internal/eval/sandbox)"'
      - "go vet ./internal/eval/sandbox"
      - "go vet -tags integration ./internal/eval/sandbox"

  - id: "prune-container"
    agent: "builder"
    description: "In container_test.go, add a subtest 'ExecWithOutput reports a nonexistent binary' to TestContainer_ExecWithStdin (after 'ExecWithOutput semantics unchanged'): echo success returns no error and output containing success, and ExecWithOutput of nonexistent-command returns an error. Then delete TestExecWithOutput_ErrorHandling and TestContainerTimeout. The false -> exit code 1 assertion is dropped because the partner's exact-string exit code 2 assertion already covers it. No import changes are needed."
    dependencies: ["prune-daemon"]
    acceptance_criteria:
      - "TestContainer_ExecWithStdin has a subtest named 'ExecWithOutput reports a nonexistent binary' that runs in the partner's existing container and asserts the echo success case and the nonexistent-command error case"
      - "All existing subtests of TestContainer_ExecWithStdin are unchanged"
      - "TestExecWithOutput_ErrorHandling and TestContainerTimeout no longer exist (TestContainerTimeoutMapping and TestContainerTimeoutNamesEffectiveTimeout in internal/eval are different tests and stay)"
      - "go test -v on TestContainer_ExecWithStdin prints PASS for the new subtest; gofmt -l reports nothing; go vet passes with and without -tags integration"
    validation_commands:
      - |-
        sh -c 'grep -nE "func (TestExecWithOutput_ErrorHandling|TestContainerTimeout)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'
      - |-
        sh -c 'go test -count=1 -run "^TestContainer_ExecWithStdin\$" -v ./internal/eval/sandbox | grep -qE -- "--- PASS: TestContainer_ExecWithStdin/ExecWithOutput_reports_a_nonexistent_binary "'
      - "go test -count=1 -run '^TestContainer_ExecWithStdin$' ./internal/eval/sandbox"
      - |-
        sh -c 'test -z "$(gofmt -l internal/eval/sandbox)"'
      - "go vet ./internal/eval/sandbox"
      - "go vet -tags integration ./internal/eval/sandbox"

  - id: "prune-installation"
    agent: "builder"
    description: "In installation_test.go, hoist ctx in TestRuntimeVerification_DoesNotInstall and add a subtest 'verifyKiroCLIInstallation error names kiro-cli' (direct c.verifyKiroCLIInstallation(ctx) call, require.Error then assert.Contains err.Error() 'kiro-cli'), keeping the existing ValidateKiroCLI assert.Error unchanged; then delete TestKiroCLIVerification_Detailed. Delete TestKiroCLIDownloadURL_SupportedPlatforms (with its leading comment), TestInstallationVerification_PermissionChecks, TestPlatformSpecificBinaries, TestDetectHostArchitecture_Unsupported, TestValidateKiroCLI_Verification and TestResourceLimits_Coverage. Remove the now-unused os and path/filepath imports; keep context, testing, time, container, assert and require."
    dependencies: ["prune-container"]
    acceptance_criteria:
      - "TestRuntimeVerification_DoesNotInstall keeps its original ValidateKiroCLI assertion and gains the subtest 'verifyKiroCLIInstallation error names kiro-cli' with a Contains 'kiro-cli' check on the inner error"
      - "The 7 named functions (KiroCLIVerification_Detailed, KiroCLIDownloadURL_SupportedPlatforms, InstallationVerification_PermissionChecks, PlatformSpecificBinaries, DetectHostArchitecture_Unsupported, ValidateKiroCLI_Verification, ResourceLimits_Coverage) no longer exist"
      - "The os and path/filepath imports are removed and no other import is removed; TestInstallationFailures_ErrorHandling, TestContainer_LogStartup, TestContainer_GetContainerInfo and TestContainer_ArchitectureErrors are unchanged"
      - "Partners TestKiroCLIVersionedURL_PlatformMapping, TestToolSetBuildArgs, TestResourceLimits and TestRuntimeVerification_DoesNotInstall pass; gofmt -l reports nothing; go vet passes with and without -tags integration"
    validation_commands:
      - |-
        sh -c 'grep -nE "func (TestKiroCLIVerification_Detailed|TestKiroCLIDownloadURL_SupportedPlatforms|TestInstallationVerification_PermissionChecks|TestPlatformSpecificBinaries|TestDetectHostArchitecture_Unsupported|TestValidateKiroCLI_Verification|TestResourceLimits_Coverage)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'
      - |-
        sh -c 'go test -count=1 -run "^TestRuntimeVerification_DoesNotInstall\$" -v ./internal/eval/sandbox | grep -qE -- "--- PASS: TestRuntimeVerification_DoesNotInstall/verifyKiroCLIInstallation_error_names_kiro-cli "'
      - "go test -count=1 -run '^(TestKiroCLIVersionedURL_PlatformMapping|TestToolSetBuildArgs|TestResourceLimits|TestRuntimeVerification_DoesNotInstall|TestInstallationFailures_ErrorHandling|TestContainer_LogStartup|TestContainer_GetContainerInfo|TestContainer_ArchitectureErrors)$' ./internal/eval/sandbox"
      - |-
        sh -c 'test -z "$(gofmt -l internal/eval/sandbox)"'
      - "go vet ./internal/eval/sandbox"
      - "go vet -tags integration ./internal/eval/sandbox"

  - id: "prune-integration-installation"
    agent: "builder"
    description: "Delete TestKiroCLINotInstalledInBaseImage and its helper kiroCLIMissing from integration_installation_test.go. The helper's only callers are in the deleted test (checked with grep), so after the deletion the file holds nothing; remove it with git rm."
    dependencies: ["prune-installation"]
    acceptance_criteria:
      - "TestKiroCLINotInstalledInBaseImage no longer exists"
      - "kiroCLIMissing is no longer defined or referenced anywhere in the repository"
      - "internal/eval/sandbox/integration_installation_test.go no longer exists"
      - "The package compiles with and without -tags integration"
    validation_commands:
      - "test ! -e internal/eval/sandbox/integration_installation_test.go"
      - |-
        sh -c 'grep -rnE "kiroCLIMissing|TestKiroCLINotInstalledInBaseImage" --include=*.go . ; [ $? -eq 1 ]'
      - "go vet ./internal/eval/sandbox"
      - "go vet -tags integration ./internal/eval/sandbox"

  - id: "prune-mounts"
    agent: "builder"
    description: "In mounts_test.go, append the subtest 'no tmpfs at or under the workspace' to TestNewHostConfigWithMounts_Tmpfs (assert hc.Tmpfs has no /workspace key and no entry at or under /workspace/), keeping all its existing assertions at the top level; then delete TestNewHostConfigWithMounts_NoWorkspaceTmpfs, TestTmpfsMounts_NoOwnershipOptions (with its doc comment), TestWithOpenUmask_Shape and TestWithOpenUmask_DoesNotMutateInput. No import changes are needed."
    dependencies: ["prune-integration-installation"]
    acceptance_criteria:
      - "TestNewHostConfigWithMounts_Tmpfs has the subtest 'no tmpfs at or under the workspace' with the explicit /workspace key check and the at-or-under-/workspace prefix loop, and its existing assertions are unchanged"
      - "The 4 named functions no longer exist"
      - "TestWithOpenUmask_PreservesArguments, TestWithOpenUmask_AppliesUmask and all other tests in the file are unchanged and pass"
      - "gofmt -l reports nothing; go vet passes with and without -tags integration"
    validation_commands:
      - |-
        sh -c 'grep -nE "func (TestNewHostConfigWithMounts_NoWorkspaceTmpfs|TestTmpfsMounts_NoOwnershipOptions|TestWithOpenUmask_Shape|TestWithOpenUmask_DoesNotMutateInput)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'
      - |-
        sh -c 'go test -count=1 -run "^TestNewHostConfigWithMounts_Tmpfs\$" -v ./internal/eval/sandbox | grep -qE -- "--- PASS: TestNewHostConfigWithMounts_Tmpfs/no_tmpfs_at_or_under_the_workspace "'
      - "go test -count=1 -run '^(TestNewHostConfigWithMounts_|TestWithOpenUmask_|TestSELinuxEnforcingFromContent)' ./internal/eval/sandbox"
      - |-
        sh -c 'test -z "$(gofmt -l internal/eval/sandbox)"'
      - "go vet ./internal/eval/sandbox"
      - "go vet -tags integration ./internal/eval/sandbox"

  - id: "record-coverage-after"
    agent: "builder"
    description: "Record the 'after' coverage for internal/eval/sandbox and compare it with the baseline. The total statement count must be identical (no production change) and the covered count must be at least the baseline's. Write the comparison to .kairon/artifacts/issue-434-coverage-after.txt so krew-lead can put before/after in the PR description."
    dependencies: ["prune-mounts"]
    acceptance_criteria:
      - "Files .kairon/artifacts/issue-434-coverage-after.out and issue-434-coverage-after.txt exist and state before and after covered/total statements"
      - "Total statements equal the baseline; covered statements are greater than or equal to the baseline; go test -cover displays at least 66.1%"
      - "If the covered count is lower, the sentinel names the missing blocks (go tool cover -func diff) instead of reporting success"
    validation_commands:
      - "go test -count=1 -cover -coverprofile=.kairon/artifacts/issue-434-coverage-after.out ./internal/eval/sandbox"
      - |-
        awk 'FNR==1{f++; next} {t[f]+=$2; if($3>0)c[f]+=$2} END{printf "before %d/%d after %d/%d\n", c[1],t[1],c[2],t[2]; exit !(t[1]==t[2] && c[2]>=c[1])}' .kairon/artifacts/issue-434-coverage-before.out .kairon/artifacts/issue-434-coverage-after.out | tee .kairon/artifacts/issue-434-coverage-after.txt

  - id: "verify-pruning"
    agent: "validator"
    description: "Read-only verification of every Scope Coverage row: the 18 deletions are gone, the 6 merges are subtests of their partners and pass, only test files in internal/eval/sandbox changed, coverage did not drop, and the gate passes."
    dependencies: ["record-coverage-after"]
    acceptance_criteria:
      - "AC1: none of the 18 Delete-marked functions exists in internal/eval/sandbox/*_test.go"
      - "AC2: none of the 6 Merge-marked functions exists as a separate function, and each partner's go test -v run prints PASS for the folded subtest (TestPodmanSocketCandidates_Linux: xdg_runtime_dir, root_uid_0, darwin, unknown_os; TestContainer_ExecWithStdin: ExecWithOutput_reports_a_nonexistent_binary; TestRuntimeVerification_DoesNotInstall: verifyKiroCLIInstallation_error_names_kiro-cli; TestNewHostConfigWithMounts_Tmpfs: no_tmpfs_at_or_under_the_workspace)"
      - "AC3: go test -cover ./internal/eval/sandbox reports at least 66.1% and the before/after comparison in issue-434-coverage-after.txt shows covered statements not lower and total statements equal"
      - "AC4: task test exits 0"
      - "B1: git diff against a1aecb0, excluding .kiro/agents and .kairon, lists only internal/eval/sandbox/*_test.go files (no production code); partner tests outside the table, including TestPodmanSocketCandidates_LinuxNoXDGFallsBackToUID, are unchanged"
      - "Every partner keeps all assertions it had before: the diff for each of the 4 partners only adds code (existing lines unchanged), apart from hoisting ctx in TestRuntimeVerification_DoesNotInstall and turning the single call in TestPodmanSocketCandidates_Linux into a table row"
      - "go vet passes with and without -tags integration, and gofmt -l reports nothing"
    validation_commands:
      - |-
        sh -c 'grep -nE "func (TestArchitectureDetectionInContainer|TestContainerTimeout|TestDetectHostArchitecture|TestDetectHostArchitecture_Unsupported|TestInstallationVerification_PermissionChecks|TestKiroCLIDownloadURL_SupportedPlatforms|TestKiroCLIInstallationMocking|TestKiroCLINotInstalledInBaseImage|TestPlatformSpecificBinaries|TestPlatformSpecificImagePulling|TestPodmanDiscovery_DefaultEndpointWins|TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden|TestResourceLimits_Coverage|TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker|TestTmpfsMounts_NoOwnershipOptions|TestValidateKiroCLI_Verification|TestWithOpenUmask_DoesNotMutateInput|TestWithOpenUmask_Shape)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'
      - |-
        sh -c 'grep -nE "func (TestExecWithOutput_ErrorHandling|TestKiroCLIVerification_Detailed|TestNewHostConfigWithMounts_NoWorkspaceTmpfs|TestPodmanSocketCandidates_Darwin|TestPodmanSocketCandidates_LinuxRoot|TestPodmanSocketCandidates_UnknownOS)\(" internal/eval/sandbox/*_test.go; [ $? -eq 1 ]'
      - |-
        sh -c 'go test -count=1 -run "^TestPodmanSocketCandidates_Linux\$" -v ./internal/eval/sandbox | grep -cE -- "--- PASS: TestPodmanSocketCandidates_Linux/(xdg_runtime_dir|root_uid_0|darwin|unknown_os) " | grep -qx 4'
      - |-
        sh -c 'go test -count=1 -run "^TestContainer_ExecWithStdin\$" -v ./internal/eval/sandbox | grep -qE -- "--- PASS: TestContainer_ExecWithStdin/ExecWithOutput_reports_a_nonexistent_binary "'
      - |-
        sh -c 'go test -count=1 -run "^TestRuntimeVerification_DoesNotInstall\$" -v ./internal/eval/sandbox | grep -qE -- "--- PASS: TestRuntimeVerification_DoesNotInstall/verifyKiroCLIInstallation_error_names_kiro-cli "'
      - |-
        sh -c 'go test -count=1 -run "^TestNewHostConfigWithMounts_Tmpfs\$" -v ./internal/eval/sandbox | grep -qE -- "--- PASS: TestNewHostConfigWithMounts_Tmpfs/no_tmpfs_at_or_under_the_workspace "'
      - |-
        sh -c 'go test -count=1 -cover ./internal/eval/sandbox | grep -E "^ok .*coverage: (66\.[1-9]|6[7-9]\.|[7-9][0-9]\.|100)"'
      - |-
        awk 'FNR==1{f++; next} {t[f]+=$2; if($3>0)c[f]+=$2} END{printf "before %d/%d after %d/%d\n", c[1],t[1],c[2],t[2]; exit !(t[1]==t[2] && c[2]>=c[1])}' .kairon/artifacts/issue-434-coverage-before.out .kairon/artifacts/issue-434-coverage-after.out
      - |-
        sh -c 'git -P diff --name-only a1aecb0 -- . ":!.kiro/agents" ":!.kairon" | grep -v "^internal/eval/sandbox/[a-z_]*_test\.go$"; [ $? -eq 1 ]'
      - |-
        sh -c 'test -z "$(gofmt -l internal/eval/sandbox)"'
      - "go vet ./..."
      - "go vet -tags integration ./internal/eval/sandbox"
      - "go test -count=1 -tags integration -run '^TestArchitectureIntegration_CurrentArchitectureConsistency$' ./internal/eval/sandbox"
      - "task test"
```
