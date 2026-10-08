# Design Spec: eval `setPermissions` WalkDir fails on vanished transient git files

Closes #311

## Problem

`caseWorkspace.setPermissions` (`internal/eval/workspace.go`, ~L453) runs two `filepath.WalkDir` passes:

1. over `w.Dir` (skipping `w.KiroDir`) calling `chmodOpen(p, d, 0o666)`;
2. over `w.KiroDir` calling `chmodOpen(p, d, 0o444)`.

`newCaseWorkspace` calls `setPermissions` right after `gitInit` commits. Git auto-maintenance (`git gc --auto` /
`git maintenance run --auto`) may spawn after that commit and create/remove transient files such as
`.git/objects/maintenance.lock`. `WalkDir` reads a directory's entries up front, so an entry can be listed and then
removed before the callback reaches it. Then `chmodOpen` → `d.Info()` (an `lstat`) — or the later `os.Chmod` — returns
`lstat …: no such file or directory`, the callback returns it, and `newCaseWorkspace` fails (flaky).

The same race can occur when a *directory* vanishes: `WalkDir` then calls the callback a second time with the
`ReadDir` error for that directory.

## Solution Approach

Treat "the entry disappeared while we were walking" as a skip, in both places the error can surface, and nothing else:

- **Walk-level**: in the `WalkDir` callback, if the incoming per-entry `err` satisfies `errors.Is(err, fs.ErrNotExist)`,
  return `nil` (skip) instead of propagating it. Any other `err` is still returned unchanged.
- **chmodOpen-level**: if `d.Info()` or `os.Chmod` fails with `fs.ErrNotExist`, return `nil`. Any other error is still
  returned unchanged.

Design decisions:

- Only `fs.ErrNotExist` is tolerated (via `errors.Is`, which matches `*fs.PathError` wrapping `ENOENT`). `EACCES`,
  `EPERM`, `EIO`, etc. continue to fail workspace creation.
- **The walk root is not skippable.** If the root itself (`w.Dir` or `w.KiroDir`) is missing, that is a real setup bug,
  not a vanished transient file, so the error propagates (`p == root` check). Note that for a root `lstat` failure
  `WalkDir` passes `d == nil`; the callback must return before touching `d`.
- To make the callback unit-testable without racing real git, extract the two duplicated closures into one small
  helper that returns an `fs.WalkDirFunc`:

  ```go
  // permWalkFunc returns a WalkDir callback that applies chmodOpen(add) to every
  // entry under root, skipping skipDir's subtree (pass "" for none). Entries that
  // vanish mid-walk (fs.ErrNotExist, other than root itself) are skipped; every
  // other error is returned.
  func permWalkFunc(root, skipDir string, add fs.FileMode) fs.WalkDirFunc
  ```

  `setPermissions` becomes two `filepath.WalkDir(root, permWalkFunc(...))` calls with the same wrapping error messages
  (`"setting workspace permissions: %w"`, `"setting %s permissions: %w"`) as today. Permission targets (0o666 / 0o444
  with `&^ 0o022`, +x on dirs and already-executable files, symlinks untouched) and the `p == kiro → SkipDir` behaviour
  are unchanged.
- `chmodOpen` keeps its signature; it gains the two `errors.Is(err, fs.ErrNotExist)` → `return nil` branches. Add the
  `errors` import to `workspace.go`.

### Explicitly out of scope (per issue constraints)

- No change to hermetic git setup (`hermeticGitEnv`, `gitInit`), mount model, or permission targets.
- Do not disable git auto-maintenance (`gc.auto`, `maintenance.auto`) or alter git config.
- Do not skip any error other than `fs.ErrNotExist`.
- Do not edit `.kairon/specs/` historical files or `CHANGELOG.md`.

## Relevant Files

| File | Change |
|------|--------|
| `internal/eval/workspace.go` | Add `errors` import; add `permWalkFunc`; rewrite `setPermissions` to use it; make `chmodOpen` tolerate `fs.ErrNotExist` from `d.Info()` / `os.Chmod`. |
| `internal/eval/workspace_test.go` | New unit tests (see below). Existing `TestWorkspacePermissions` must keep passing untouched. |

No template-synchronized files (`cmd/kairon/templates/`) are involved, so `task sync:check` is not affected. No docs
change is needed (internal fix; the exempt rule in AGENTS.md for non-code changes does not apply — code + tests only).

## Tests (TDD — AGENTS.md rules)

Write these first, confirm they fail **for the expected reason**, then implement. Commit tests and implementation
together in one commit; name the test(s) seen failing first in the commit message/PR description.

To avoid failing on a compile error (not an acceptable "expected failure"), first add `permWalkFunc` as a stub that
reproduces *current* behaviour (return `err` if non-nil, skip `skipDir`, else `chmodOpen`) and have `setPermissions`
use it; then add tests, run them, observe the failures below, then implement the fix. All tests are POSIX-only
(`runtime.GOOS == "windows"` → `t.Skip`) like `TestWorkspacePermissions`, and use `t.TempDir()` — no git needed.

1. `TestChmodOpenVanishedFileIsSkipped` (AC 2): create a file, obtain its `fs.DirEntry` via `os.ReadDir`, `os.Remove`
   the file, call `chmodOpen(path, d, 0o666)` → expect `nil`. Currently fails with `lstat …: no such file or
   directory`.
2. `TestChmodOpenOtherInfoErrorPropagates` (AC 3, chmodOpen level): call `chmodOpen` with a fake `fs.DirEntry` whose
   `Info()` returns `fs.ErrPermission` (small test-local struct implementing `fs.DirEntry`, `Type()` = 0) → expect
   `errors.Is(err, fs.ErrPermission)`. Passes before and after (regression guard).
3. `TestPermWalkFuncSkipsNotExist` (AC 1, callback level): call the returned func with a non-root path, a nil/dummy
   entry and `&fs.PathError{Op: "lstat", Path: p, Err: syscall.ENOENT}` (and a variant wrapped with `fmt.Errorf("%w")`)
   → expect `nil`. Fails currently (error returned).
4. `TestPermWalkFuncPropagatesOtherErrors` (AC 3): same call with `fs.ErrPermission` and `syscall.EIO` wrapped in
   `*fs.PathError` → expect the error returned (`errors.Is`). Passes before and after.
5. `TestPermWalkFuncRootNotExistPropagates`: call with `p == root`, `d == nil`, `fs.ErrNotExist` → expect non-nil.
6. `TestPermWalkMidWalkDeletion` (AC 1, real walk): `t.TempDir()` with `a.txt`, `b.txt`, and `sub/c.txt` (and
   optionally an entire `sub2/` dir with a file). Run `filepath.WalkDir(root, wrapper)` where `wrapper` delegates to
   `permWalkFunc(root, "", 0o666)` but, on first visiting `a.txt`, removes `b.txt` (and `os.RemoveAll("sub2")`) —
   `WalkDir` lists directory entries sorted and up front, so those entries are visited after removal. Expect `nil`
   error and that `a.txt` and `sub/c.txt` got the new modes. Fails currently with `lstat …: no such file or directory`.
7. Existing `TestWorkspacePermissions` (AC 4): `.kiro` subtree 0444/no group-other write, everything else a+rwX —
   unchanged and must still pass (run via `-run 'TestWorkspace'`).

## Team Orchestration

Small, single-file fix; a single builder task (tests + implementation must land together per AGENTS.md TDD rules and
are tightly coupled in one file pair, so splitting would only add conflicts) followed by an independent validator
pass. No parallelism is needed. Documenter is not required (no user-facing behavior or docs change).

## Step-by-Step Task Breakdown

### Task 1 — `fix-walk-race` (builder)
1. Add the `permWalkFunc` stub (current behaviour) and switch `setPermissions` to it. Run `go build ./internal/eval/`.
2. Add tests 1–6 above to `internal/eval/workspace_test.go`. Run
   `go test ./internal/eval/ -run 'TestChmodOpen|TestPermWalk' -count=1`; confirm tests 1, 3 and 6 fail with the
   `no such file or directory` / unexpected-error reason (tests 2, 4, 5 pass). Record the failing test names.
3. Implement: `errors` import; `fs.ErrNotExist` handling in `permWalkFunc` (non-root only) and in `chmodOpen` (both
   `d.Info()` and `os.Chmod`). Keep `setPermissions` error wrapping messages identical.
4. Run `go test ./internal/eval/ -run 'TestWorkspace' -count=1`, `go test ./internal/eval/... -count=1`, `go vet
   ./internal/eval/...`, `gofmt -l internal/eval`.
5. Stay within scope: do not touch git setup, config or permission targets. Do not commit unless instructed by the
   pipeline; when committing, put tests and implementation in the same commit and name the tests seen failing first.

Acceptance criteria: AC 1–3 covered by tests 1–6 passing; AC 4 and 5 by the existing suites.

### Task 2 — `validate-fix` (validator)
Read-only verification of every issue acceptance criterion and constraint: the diff only touches
`internal/eval/workspace.go` and `internal/eval/workspace_test.go`; only `fs.ErrNotExist` is tolerated; non-ENOENT
errors still propagate; permission targets unchanged; tests present for each AC; test suites and vet/format pass.

## Validation Commands

```bash
go build ./...
go test ./internal/eval/ -run 'TestChmodOpen|TestPermWalk' -count=1 -v
go test ./internal/eval/ -run 'TestWorkspace' -count=1
go test ./internal/eval/... -count=1
go vet ./internal/eval/...
test -z "$(gofmt -l internal/eval)"
git diff --name-only origin/main...HEAD   # expect only the two files above (plus spec/artifacts)
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "fix-walk-race"
    agent: "builder"
    description: "TDD fix in internal/eval: add permWalkFunc helper (used by setPermissions for both w.Dir and w.KiroDir walks) that skips non-root fs.ErrNotExist per-entry errors, make chmodOpen treat fs.ErrNotExist from d.Info()/os.Chmod as a skip, and add unit tests (vanished file via chmodOpen, callback fed ErrNotExist/ENOENT, callback fed non-ENOENT errors, root-missing propagates, real mid-walk deletion). Write tests first against a behaviour-preserving stub and confirm they fail for the expected reason, then implement; tests and implementation land together."
    dependencies: []
    acceptance_criteria:
      - "setPermissions' WalkDir callback returns nil for a non-root per-entry error satisfying errors.Is(err, fs.ErrNotExist), skipping the entry"
      - "chmodOpen returns nil when the entry vanished before d.Info() or os.Chmod (fs.ErrNotExist)"
      - "Non-ENOENT errors (e.g. fs.ErrPermission, EIO) are still returned from both the walk callback and chmodOpen"
      - "A missing walk root (w.Dir or w.KiroDir) still returns an error"
      - "A real WalkDir where a not-yet-visited sibling file/dir is removed mid-walk completes without error"
      - ".kiro subtree still gets 0444 with no group/other write; the rest still gets a+rwX (TestWorkspacePermissions passes unchanged)"
      - "No changes to hermetic git setup, git config, mount model or permission targets; only internal/eval/workspace.go and workspace_test.go are modified"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'TestChmodOpen|TestPermWalk' -count=1 -v"
      - "go test ./internal/eval/ -run 'TestWorkspace' -count=1"
      - "go test ./internal/eval/... -count=1"
      - "go vet ./internal/eval/..."
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "validate-fix"
    agent: "validator"
    description: "Read-only verification that the fix meets all issue #311 acceptance criteria and constraints (scope limited to the walk race, only fs.ErrNotExist skipped, tests present for each criterion, no regressions)."
    dependencies: ["fix-walk-race"]
    acceptance_criteria:
      - "Diff touches only internal/eval/workspace.go and internal/eval/workspace_test.go (excluding spec/artifact files)"
      - "Only fs.ErrNotExist is tolerated; non-ENOENT error tests exist and pass"
      - "Each of AC 1-3 has at least one dedicated passing unit test"
      - "go test ./internal/eval/ -run 'TestWorkspace' -count=1 passes"
      - "go test ./internal/eval/... passes with no regressions, go vet and gofmt are clean"
    validation_commands:
      - "go test ./internal/eval/ -run 'TestChmodOpen|TestPermWalk' -count=1 -v"
      - "go test ./internal/eval/ -run 'TestWorkspace' -count=1"
      - "go test ./internal/eval/... -count=1"
      - "go vet ./internal/eval/..."
      - "test -z \"$(gofmt -l internal/eval)\""
```
