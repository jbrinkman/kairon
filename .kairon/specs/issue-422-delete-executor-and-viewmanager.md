# Design Spec: Delete plan.Executor and tui.ViewManager and their tests

Closes #422

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/plan`: delete the `Executor` code in `executor.go`, plus `GetConfigPath`, `Count`, `GetTaskByID`, and the tests of all of them (including executor tests not listed in the table below as dead-code verdicts, such as `TestExecutorWithPartialFailure` and `TestIntegration_EndToEnd`)" | `remove-plan-executor`, `remove-plan-accessors` |
| S2 | In Scope: "`internal/tui`: delete `ViewManager` and the code only it uses, with `view_state_test.go` and `TestViewManagerTransitions`" | `remove-view-manager` |
| S3 | In Scope: "`internal/plan/registry_test.go`: in `TestNewAgentRegistry` and `TestDiscoverAgents_FromTrustedList`, which stay, replace `registry.Count()` with `len(registry.GetAgentNames())` and `registry.GetConfigPath(name)` with `registry.agents[name]`; no other change to those tests. They call the deleted methods, and `TestDiscoverAgents_FromTrustedList` is the only success-path test of `DiscoverAgents`, which has production callers (`cmd/kairon/cmd/plan.go:58`, `internal/plan/validator.go:212`)" | `remove-plan-accessors` |
| C1 | Contract: "Behaviour reachable from `cmd/kairon` does not change. `go build ./...` and every remaining test pass. The only edits to remaining tests are the two in `internal/plan/registry_test.go` named in In Scope." | `remove-plan-executor`, `remove-plan-accessors`, `remove-view-manager`, `validate-removal` |
| AC1 | "The Executor code and the types, constants and test helpers used only by it are gone from `internal/plan`." | `remove-plan-executor`, `validate-removal` |
| AC2 | "`GetConfigPath`, `Count` and `GetTaskByID` are gone." | `remove-plan-accessors`, `validate-removal` |
| AC3 | "`ViewManager` and the code only it uses are gone, including both `view_state` files." | `remove-view-manager`, `validate-removal` |
| AC4 | "The tests of the deleted code are gone." | `remove-plan-executor`, `remove-plan-accessors`, `remove-view-manager`, `validate-removal` |
| AC5 | "`Plan.GetTaskIDs` and its remaining test stay." | `remove-plan-accessors`, `validate-removal` |
| AC6 | "`TopologicalSort` and its tests remain and pass." | `remove-plan-executor`, `validate-removal` |
| AC7 | "The module builds and the gate passes, with no remaining test edited except the two registry tests named in In Scope." | `remove-plan-executor`, `remove-plan-accessors`, `remove-view-manager`, `validate-removal` |

## Deferred work

none

## Solution Approach

This is a deletion-only change, so AGENTS.md's refactor rule applies: the remaining tests must pass before and after, and no new behaviour is written. Nothing reachable from `cmd/kairon` changes. The only production symbol that stays out of `executor.go` is `TopologicalSort`, which `kairon plan parse` calls (`cmd/kairon/cmd/plan.go:77`).

Decisions:

1. **`executor.go` becomes `toposort.go`.** After the `Executor` code is gone, `executor.go` would hold only `TopologicalSort`. The issue's Out of Scope says to move it to its own file if `executor.go` is removed. `toposort.go` holds `TopologicalSort` byte-for-byte (same doc comment and body), with imports `fmt` and `sort`. The `sync` import goes away with the `Executor`.
2. **`executor_test.go` becomes `toposort_test.go`.** `executor_test.go` lines 12-190 are the five `TestTopologicalSort*` tests. They move unchanged into `toposort_test.go`, whose import block is `import (\n\t"testing"\n)` (they use no other package; verified by a scratch build). The old `executor_test.go` is deleted. `TestCompatibility_TopologicalSortStability` stays in `compatibility_test.go` untouched.
3. **Delete tests before the code they cover, and edit the two registry tests first.** Inside each task, the order is: (a) run the package tests green, (b) delete or edit tests, (c) delete production code, (d) run again. For `registry_test.go` the two edited tests compile and pass against the current code (`GetAgentNames` and `registry.agents` already exist), so the package is green after every step.
4. **`(*OutputView).SetStyles` is deleted with `ViewManager`.** Its only caller was `ViewManager.UpdateOutputViewStyles` (`grep -rn SetStyles` finds no other caller of it; the remaining `SetStyles` hits are `textinput` methods). The issue says to delete "the code only it uses".
5. **`NewOutputView` is kept.** `ViewManager.InitOutputView` was its only production caller, but the remaining test `TestOutputViewRendersCapturedManagerOutput` (`internal/tui/integration_test.go:219`) still calls it. AC7 forbids editing that test, so deleting `NewOutputView` would break the contract. `OutputView.Init`, `Update`, `Resize` and `View` are kept: `AgentTab` uses them.

Spec deviations: none.

## Inventory: what goes and what stays

Everything below was checked with `grep -rnE` over the whole repository (`--include=*.go`). Outside the files listed, there are no references to any deleted symbol.

### `internal/plan` production code

| Symbol | File | Action | Evidence |
|--------|------|--------|----------|
| `TaskStatus` type and `TaskStatusPending`, `TaskStatusRunning`, `TaskStatusSuccess`, `TaskStatusFailed`, `TaskStatusSkipped` | `executor.go` | Delete | Used only by `ExecutePlan`, `shouldSkipTask`, `executeLayer` and the executor/integration/compatibility tests |
| `TaskResult`, `ExecutionSummary`, `TaskSpawner` | `executor.go` | Delete | Same |
| `DefaultMaxParallelTasks`, `Executor`, `NewExecutor`, `NewExecutorWithLimit`, `(*Executor).ExecutePlan`, `shouldSkipTask`, `executeLayer` | `executor.go` | Delete | No caller outside `internal/plan` tests; krew-lead runs plan tasks itself |
| `TopologicalSort` | `executor.go` | **Keep, move to `toposort.go`** | `cmd/kairon/cmd/plan.go:77`, `TestTopologicalSort*`, `TestCompatibility_TopologicalSortStability` |
| `(*AgentRegistry).GetConfigPath`, `(*AgentRegistry).Count` | `registry.go` | Delete | Only callers are `registry_test.go` |
| `(*Plan).GetTaskByID` | `types.go` | Delete | Only caller is `TestPlanGetTaskByID` |
| `(*Plan).GetTaskIDs` | `types.go` | **Keep** | PR #416 adds a caller in `cmd/kairon/cmd/scope.go`; `TestPlanGetTaskIDs` covers it |
| `AgentRegistry.Contains`, `GetAgentNames`, `DiscoverAgents`, `NewAgentRegistry` | `registry.go` | **Keep** | Production callers |

### `internal/plan` tests

| Test or helper | File | Action |
|----------------|------|--------|
| `TestTopologicalSortWithSimpleChain`, `...WithParallelTasks`, `...WithDiamondPattern`, `...WithEmptyPlan`, `...WithNilPlan` | `executor_test.go` | **Move unchanged** to `toposort_test.go` |
| `TestExecutorWithSuccessfulTasks`, `TestExecutorWithFailedTask`, `TestExecutorWithPartialFailure`, `TestExecutorParallelExecution`, `TestExecutorWithEmptyPlan`, `TestExecutorWithNilPlan`, `TestExecutorLayerSequencing`, `TestExecutorTaskResultMapping`, `TestExecutorRespectsConcurrencyLimit`, `TestNewExecutorWithLimit_NonPositiveFallsBackToDefault` | `executor_test.go` | Delete (file is deleted) |
| `TestIntegration_ParallelExecution`, `TestIntegration_SequentialExecution`, `TestIntegration_MixedDependencies`, `TestIntegration_TaskFailureHandling`, `TestIntegration_EndToEnd` | `integration_test.go` | Delete |
| Helpers `taskEvent`, `taskExecutionLog` and its methods `logStart`, `logComplete`, `getEventsByType`, `getEventTime`, `createMockSpawnerWithLog` | `integration_test.go` | Delete. Used only by the five deleted integration tests; `TestIntegration_InvalidPlanParsing` does not use them |
| `TestIntegration_InvalidPlanParsing` | `integration_test.go` | **Keep unchanged** |
| `TestCompatibility_ExecutorEmptyPlan` | `compatibility_test.go` | Delete. All other compatibility tests, including `TestCompatibility_TopologicalSortStability`, stay |
| `TestAgentRegistry_GetConfigPath`, `TestAgentRegistry_Count` | `registry_test.go` | Delete |
| `TestNewAgentRegistry`, `TestDiscoverAgents_FromTrustedList` | `registry_test.go` | **Edit, only the call substitutions in S3** |
| `TestPlanGetTaskByID`, `TestPlanGetTaskIDs_EmptyPlan` | `types_test.go` | Delete |
| `TestPlanGetTaskIDs` | `types_test.go` | **Keep unchanged** |

No remaining test depends on a helper in the deleted files: after the dry run described below, `go vet ./internal/plan ./internal/tui` and `go test -race ./internal/plan ./internal/tui` pass.

### `internal/tui`

| Symbol or test | File | Action |
|----------------|------|--------|
| `ViewType`, `ViewConsole`, `ViewAgentOutput`, `ViewState`, `ViewManager`, `NewViewManager`, and the methods `CurrentView`, `ToggleView`, `SetView`, `Update`, `InitOutputView`, `UpdateOutputViewStyles`, `RenderCurrentView`, `PreserveConsoleScroll`, `GetConsoleScroll` | `view_state.go` | Delete the file |
| `TestViewManager`, `TestViewStatePreservation` | `view_state_test.go` | Delete the file |
| `TestViewManagerTransitions`, `TestTerminalResizeHandling` | `integration_test.go` | Delete both functions, and the `tea "charm.land/bubbletea/v2"` import line (its only use was in `TestTerminalResizeHandling`) |
| `(*OutputView).SetStyles` | `output_view.go` | Delete (decision 4) |
| `TabManager.ToggleView`, `TabManager.RenderCurrentView` | `tab_manager.go` | **Keep.** Different type with the same method names; used by `tui.go` and its tests. The AC3 grep does not match them |
| `NewOutputView`, `OutputView.Init/Update/Resize/View` | `output_view.go` | **Keep** (decision 5) |

## How AC7's `git diff --numstat` check treats each file

`git diff --numstat origin/main -- '*_test.go' | awk '$1 > 0 {print $3}'` prints a path only when that file has added lines. A deletion-only edit has `0` added lines, so it prints nothing.

| File | Added lines | Printed? |
|------|-------------|----------|
| `internal/plan/compatibility_test.go` | 0 | no |
| `internal/plan/executor_test.go` (deleted) | 0 | no |
| `internal/plan/integration_test.go` | 0 | no |
| `internal/plan/types_test.go` | 0 | no |
| `internal/tui/integration_test.go` | 0 | no |
| `internal/tui/view_state_test.go` (deleted) | 0 | no |
| `internal/plan/registry_test.go` | 5 (the two edited tests) | **yes, allowed** |
| `internal/plan/toposort_test.go` (new) | 185 | **yes, allowed** (the moved TopologicalSort tests) |

A scratch clone confirmed this: the command printed exactly `internal/plan/registry_test.go` and `internal/plan/toposort_test.go`. Git does not report `executor_test.go` -> `toposort_test.go` as a rename (the moved part is under half of the old file), so the new file shows as `185 0`. Even if a rename were detected, the added count would be 0.

Rules the builders must follow so no other test file gets an added line:

1. In `internal/plan/integration_test.go`, shrink the import block to the parenthesized form `import (\n\t"testing"\n)`. Rewriting it as `import "testing"` adds a line and makes the file appear in the AC7 output. `gofmt` accepts the parenthesized form.
2. In `internal/tui/integration_test.go`, only remove the `tea` import line. Do not regroup or reformat the remaining imports.
3. Remove each deleted function together with its doc comment and the blank line after it, so `gofmt` has nothing to change. Run `gofmt -l` afterwards; if it reports a file, fix it by deleting lines, not by adding any.
4. Do not touch any other `*_test.go` file.
5. `toposort_test.go` and `toposort.go` are new files. Stage them (`git add`) or commit them; untracked files are invisible to `git diff origin/main`.

If `origin/main` moves because of a fetch, compare against `$(git merge-base origin/main HEAD)` instead; the expected output is the same two paths.

## One addition beyond the issue's list

`test_integration.sh` (tracked, live) invokes deleted tests by name: `-run TestExecutor`, `-run TestExecutorParallelExecution`, five `-run TestIntegration_*` entries and `-run TestCompatibility_ExecutorEmptyPlan`. `go test -run <pattern>` with no matching test exits 0, so after this change those eleven lines would report PASS for tests that no longer exist. Task `prune-integration-script` removes those `run_test` entries (and renames the "Test 5: Plan executor" comment to "Test 5: Topological sort"). `TestTopologicalSort` and `TestIntegration_InvalidPlanParsing` entries stay. This is a script, not a `*_test.go` file, so AC7 is unaffected. It is separate from the other tasks so a reviewer can drop it without touching the rest.

## Relevant Files

Modify:
- `internal/plan/integration_test.go`, `internal/plan/compatibility_test.go`, `internal/plan/registry_test.go`, `internal/plan/types_test.go`
- `internal/plan/registry.go`, `internal/plan/types.go`
- `internal/tui/integration_test.go`, `internal/tui/output_view.go`
- `test_integration.sh`

Create:
- `internal/plan/toposort.go` (`TopologicalSort`, moved from `executor.go`)
- `internal/plan/toposort_test.go` (five `TestTopologicalSort*` tests, moved from `executor_test.go`)

Delete:
- `internal/plan/executor.go`, `internal/plan/executor_test.go`
- `internal/tui/view_state.go`, `internal/tui/view_state_test.go`

Do not edit: anything under `.kairon/specs/` other than this file, `VALIDATION_SUMMARY.md` (historical, mentions `TestViewManagerTransitions`), `CHANGELOG.md`, and the research documents under `.kairon/research/` that describe the Executor as dead code (they record the finding that this issue acts on). No template-synchronized file is touched, so `task sync:check` needs no re-sync.

## Team Orchestration

- `remove-plan-executor`, `remove-view-manager` and `prune-integration-script` have no dependencies and can run in parallel. They touch different packages or files.
- `remove-plan-accessors` also edits `internal/plan`. It waits for `remove-plan-executor` so two builders never edit the same package while the other's validation commands compile it.
- `validate-removal` (validator, read-only) runs after all four builder tasks and checks all seven acceptance criteria with the issue's exact commands.

## Step-by-Step Task Breakdown

### Task 1: `remove-plan-executor`
Dependencies: none.
1. Run `go test ./internal/plan` and confirm it passes (baseline for the refactor rule).
2. Create `internal/plan/toposort.go`: `package plan`, imports `fmt` and `sort`, then `TopologicalSort` copied byte-for-byte from `executor.go` with its doc comment. Delete `executor.go`.
3. Create `internal/plan/toposort_test.go`: `package plan`, `import (\n\t"testing"\n)`, then the five `TestTopologicalSort*` tests copied unchanged from `executor_test.go` lines 12-190. Delete `executor_test.go`.
4. `integration_test.go`: delete the five `TestIntegration_*` execution tests and the helpers listed in the inventory; keep `TestIntegration_InvalidPlanParsing`; set imports to `import (\n\t"testing"\n)`.
5. `compatibility_test.go`: delete `TestCompatibility_ExecutorEmptyPlan` (with its doc comment).
6. `go build ./...`, `gofmt -l internal/plan`, `go vet ./internal/plan`, `go test -race ./internal/plan`.

### Task 2: `remove-plan-accessors`
Dependencies: `remove-plan-executor`.
1. In `registry_test.go`, edit `TestNewAgentRegistry` (`registry.Count()` twice -> `len(registry.GetAgentNames())`) and `TestDiscoverAgents_FromTrustedList` (`registry.Count()` twice -> `len(registry.GetAgentNames())`; `registry.GetConfigPath(name)` -> `registry.agents[name]`). Change nothing else in those tests. Run `go test ./internal/plan` (still green; the methods still exist).
2. Delete `TestAgentRegistry_GetConfigPath` and `TestAgentRegistry_Count` from `registry_test.go`, and `TestPlanGetTaskByID` and `TestPlanGetTaskIDs_EmptyPlan` from `types_test.go`. Keep `TestPlanGetTaskIDs`.
3. Delete `GetConfigPath` and `Count` from `registry.go`, and `GetTaskByID` from `types.go`. Keep `GetTaskIDs`.
4. `go build ./...`, `gofmt -l internal/plan`, `go test -race ./internal/plan`.

### Task 3: `remove-view-manager`
Dependencies: none.
1. Run `go test ./internal/tui` and confirm it passes.
2. Delete `internal/tui/view_state.go` and `internal/tui/view_state_test.go`.
3. In `internal/tui/integration_test.go`, delete `TestViewManagerTransitions` and `TestTerminalResizeHandling` and the `tea "charm.land/bubbletea/v2"` import line only.
4. In `internal/tui/output_view.go`, delete `SetStyles` with its doc comment. Keep `NewOutputView`.
5. `go build ./...`, `gofmt -l internal/tui`, `go vet ./internal/tui`, `go test -race ./internal/tui`.

### Task 4: `prune-integration-script`
Dependencies: none. Edit `test_integration.sh` as described above. `bash -n test_integration.sh` must pass.

### Task 5: `validate-removal`
Dependencies: all of the above. Run every acceptance-criterion command from the issue, `task test`, `task lint`, `task fmt:check`, `task sync:check`, and the AC7 numstat check. Report PASS or FAIL per criterion.

## Validation Commands

From the project root:

```bash
go build ./...
task test
task lint
task fmt:check
task sync:check
# AC1
grep -rnE '\b(Executor|NewExecutor|NewExecutorWithLimit|TaskSpawner|ExecutionSummary|TaskResult|TaskStatus[A-Za-z]*|DefaultMaxParallelTasks|createMockSpawnerWithLog|taskExecutionLog)\b' internal/plan --include=*.go    # prints nothing
# AC2
grep -rnE 'func \((r \*AgentRegistry\) (GetConfigPath|Count)|p \*Plan\) GetTaskByID)\(' internal/plan    # prints nothing
# AC3
grep -rnE '\b(ViewManager|NewViewManager|ViewType|ViewState|ViewConsole|ViewAgentOutput)\b' internal/tui --include=*.go    # prints nothing
ls internal/tui/view_state.go internal/tui/view_state_test.go    # exits non-zero
# AC4
grep -rnE 'func (TestAgentRegistry_Count|TestAgentRegistry_GetConfigPath|TestExecutor[A-Za-z]*|TestNewExecutorWithLimit_NonPositiveFallsBackToDefault|TestIntegration_(MixedDependencies|ParallelExecution|SequentialExecution|TaskFailureHandling|EndToEnd)|TestCompatibility_ExecutorEmptyPlan|TestPlanGetTaskByID|TestPlanGetTaskIDs_EmptyPlan|TestViewManager|TestViewManagerTransitions|TestViewStatePreservation|TestTerminalResizeHandling)\(' internal/plan internal/tui --include=*_test.go    # prints nothing
# AC5
grep -n 'func (p \*Plan) GetTaskIDs' internal/plan/types.go    # one line
grep -n 'func TestPlanGetTaskIDs(' internal/plan/types_test.go    # one line
# AC6
grep -n '^func TopologicalSort' internal/plan/*.go    # one line
go test -run 'TopologicalSort' -v ./internal/plan    # exits 0, runs TestTopologicalSortWithSimpleChain
# AC7
git diff --numstat origin/main -- '*_test.go' | awk '$1 > 0 {print $3}'    # only internal/plan/registry_test.go and internal/plan/toposort_test.go
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "remove-plan-executor"
    agent: "builder"
    description: "Delete the Executor code and its tests from internal/plan. Move TopologicalSort unchanged from executor.go to toposort.go and the five TestTopologicalSort* tests unchanged from executor_test.go to toposort_test.go (import block 'import (\\n\\t\"testing\"\\n)'). Delete executor.go and executor_test.go. In integration_test.go delete TestIntegration_ParallelExecution, _SequentialExecution, _MixedDependencies, _TaskFailureHandling, _EndToEnd and the helpers taskEvent, taskExecutionLog (with logStart, logComplete, getEventsByType, getEventTime) and createMockSpawnerWithLog, keep TestIntegration_InvalidPlanParsing, and shrink the imports to the parenthesized single-line-removal form. In compatibility_test.go delete only TestCompatibility_ExecutorEmptyPlan. Edit no other test file. Run go test ./internal/plan before and after."
    dependencies: []
    acceptance_criteria:
      - "internal/plan/executor.go and internal/plan/executor_test.go do not exist"
      - "internal/plan/toposort.go contains TopologicalSort with a body identical to the old one and imports only fmt and sort"
      - "internal/plan/toposort_test.go contains TestTopologicalSortWithSimpleChain, TestTopologicalSortWithParallelTasks, TestTopologicalSortWithDiamondPattern, TestTopologicalSortWithEmptyPlan and TestTopologicalSortWithNilPlan with unchanged bodies"
      - "No Executor, NewExecutor, NewExecutorWithLimit, TaskSpawner, ExecutionSummary, TaskResult, TaskStatus*, DefaultMaxParallelTasks, createMockSpawnerWithLog or taskExecutionLog identifier remains under internal/plan"
      - "TestIntegration_InvalidPlanParsing and every other compatibility test remain and pass"
      - "integration_test.go, compatibility_test.go have zero added lines against origin/main (import block stays parenthesized)"
      - "go build ./... and go test -race ./internal/plan pass; gofmt -l internal/plan prints nothing"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/plan"
      - "go test -race ./internal/plan"
      - "test ! -e internal/plan/executor.go && test ! -e internal/plan/executor_test.go"
      - "test -z \"$(gofmt -l internal/plan)\""
      - |-
        test -z "$(grep -rnE '\b(Executor|NewExecutor|NewExecutorWithLimit|TaskSpawner|ExecutionSummary|TaskResult|TaskStatus[A-Za-z]*|DefaultMaxParallelTasks|createMockSpawnerWithLog|taskExecutionLog)\b' internal/plan --include=*.go)"
      - |-
        test -z "$(grep -rnE 'func (TestExecutor[A-Za-z]*|TestNewExecutorWithLimit_NonPositiveFallsBackToDefault|TestIntegration_(MixedDependencies|ParallelExecution|SequentialExecution|TaskFailureHandling|EndToEnd)|TestCompatibility_ExecutorEmptyPlan)\(' internal/plan --include=*_test.go)"
      - |-
        test "$(grep -n '^func TopologicalSort' internal/plan/*.go | wc -l | tr -d ' ')" = 1
      - |-
        go test -run 'TopologicalSort' -v ./internal/plan 2>&1 | grep -q -- '--- PASS: TestTopologicalSortWithSimpleChain'
      - |-
        test -z "$(grep -n '^import "testing"' internal/plan/integration_test.go)"

  - id: "remove-plan-accessors"
    agent: "builder"
    description: "In internal/plan, first edit TestNewAgentRegistry and TestDiscoverAgents_FromTrustedList in registry_test.go (registry.Count() -> len(registry.GetAgentNames()), registry.GetConfigPath(name) -> registry.agents[name], nothing else) and confirm the package is still green. Then delete TestAgentRegistry_GetConfigPath and TestAgentRegistry_Count from registry_test.go and TestPlanGetTaskByID and TestPlanGetTaskIDs_EmptyPlan from types_test.go, then delete AgentRegistry.GetConfigPath, AgentRegistry.Count and Plan.GetTaskByID. Keep Plan.GetTaskIDs and TestPlanGetTaskIDs. Edit no other test file."
    dependencies: ["remove-plan-executor"]
    acceptance_criteria:
      - "TestNewAgentRegistry and TestDiscoverAgents_FromTrustedList use len(registry.GetAgentNames()) and registry.agents[name] and are otherwise unchanged (exactly 5 added lines in registry_test.go against origin/main)"
      - "AgentRegistry.GetConfigPath, AgentRegistry.Count and Plan.GetTaskByID no longer exist"
      - "TestAgentRegistry_GetConfigPath, TestAgentRegistry_Count, TestPlanGetTaskByID and TestPlanGetTaskIDs_EmptyPlan no longer exist"
      - "Plan.GetTaskIDs and TestPlanGetTaskIDs remain"
      - "types_test.go has zero added lines against origin/main"
      - "go build ./... and go test -race ./internal/plan pass; gofmt -l internal/plan prints nothing"
    validation_commands:
      - "go build ./..."
      - "go test -race ./internal/plan"
      - "test -z \"$(gofmt -l internal/plan)\""
      - |-
        test -z "$(grep -rnE 'func \((r \*AgentRegistry\) (GetConfigPath|Count)|p \*Plan\) GetTaskByID)\(' internal/plan)"
      - |-
        test -z "$(grep -rnE 'func (TestAgentRegistry_Count|TestAgentRegistry_GetConfigPath|TestPlanGetTaskByID|TestPlanGetTaskIDs_EmptyPlan)\(' internal/plan --include=*_test.go)"
      - |-
        test -z "$(grep -nE '\.Count\(\)|GetConfigPath|GetTaskByID' internal/plan/*.go)"
      - |-
        test "$(grep -n 'func (p \*Plan) GetTaskIDs' internal/plan/types.go | wc -l | tr -d ' ')" = 1
      - |-
        test "$(grep -n 'func TestPlanGetTaskIDs(' internal/plan/types_test.go | wc -l | tr -d ' ')" = 1
      - |-
        test "$(git diff origin/main -U0 -- internal/plan/registry_test.go | grep -c '^+[^+]')" = 5

  - id: "remove-view-manager"
    agent: "builder"
    description: "In internal/tui, delete view_state.go and view_state_test.go; delete TestViewManagerTransitions and TestTerminalResizeHandling from integration_test.go together with the now-unused tea \"charm.land/bubbletea/v2\" import line (remove only that line); delete the OutputView.SetStyles method from output_view.go (its only caller was ViewManager). Keep NewOutputView (still called by TestOutputViewRendersCapturedManagerOutput), TabManager.ToggleView and TabManager.RenderCurrentView. Edit no other test file. Run go test ./internal/tui before and after."
    dependencies: []
    acceptance_criteria:
      - "internal/tui/view_state.go and internal/tui/view_state_test.go do not exist"
      - "No ViewManager, NewViewManager, ViewType, ViewState, ViewConsole or ViewAgentOutput identifier remains under internal/tui"
      - "TestViewManager, TestViewStatePreservation, TestViewManagerTransitions and TestTerminalResizeHandling no longer exist"
      - "OutputView.SetStyles is gone; NewOutputView, TabManager.ToggleView and TabManager.RenderCurrentView remain"
      - "internal/tui/integration_test.go has zero added lines against origin/main"
      - "go build ./... and go test -race ./internal/tui pass; gofmt -l internal/tui prints nothing"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/tui"
      - "go test -race ./internal/tui"
      - "test -z \"$(gofmt -l internal/tui)\""
      - "test ! -e internal/tui/view_state.go && test ! -e internal/tui/view_state_test.go"
      - |-
        test -z "$(grep -rnE '\b(ViewManager|NewViewManager|ViewType|ViewState|ViewConsole|ViewAgentOutput)\b' internal/tui --include=*.go)"
      - |-
        test -z "$(grep -rnE 'func (TestViewManager|TestViewManagerTransitions|TestViewStatePreservation|TestTerminalResizeHandling)\(' internal/tui --include=*_test.go)"
      - |-
        test -z "$(grep -n 'func (ov \*OutputView) SetStyles' internal/tui/output_view.go)"
      - |-
        test "$(grep -n 'func NewOutputView(' internal/tui/output_view.go | wc -l | tr -d ' ')" = 1
      - |-
        test -z "$(git diff --numstat origin/main -- internal/tui/integration_test.go | awk '$1 > 0')"

  - id: "prune-integration-script"
    agent: "builder"
    description: "In test_integration.sh remove the run_test entries that invoke deleted tests: 'Plan executor' (-run TestExecutor), 'Parallel execution' (-run TestExecutorParallelExecution), the integration entries for TestIntegration_ParallelExecution, _SequentialExecution, _MixedDependencies, _TaskFailureHandling and _EndToEnd, and 'Executor empty plan' (-run TestCompatibility_ExecutorEmptyPlan). Keep the 'Topological sort' entry and the TestIntegration_InvalidPlanParsing entry; rename the '# Test 5: Plan executor' comment to '# Test 5: Topological sort'. Change nothing else in the script."
    dependencies: []
    acceptance_criteria:
      - "test_integration.sh no longer names TestExecutor*, TestIntegration_ParallelExecution, _SequentialExecution, _MixedDependencies, _TaskFailureHandling, _EndToEnd or TestCompatibility_ExecutorEmptyPlan"
      - "test_integration.sh still runs TestTopologicalSort, TestIntegration_InvalidPlanParsing and TestCompatibility_TopologicalSortStability"
      - "bash -n test_integration.sh succeeds"
    validation_commands:
      - "bash -n test_integration.sh"
      - |-
        test -z "$(grep -nE 'TestExecutor|TestIntegration_(ParallelExecution|SequentialExecution|MixedDependencies|TaskFailureHandling|EndToEnd)|TestCompatibility_ExecutorEmptyPlan' test_integration.sh)"
      - "grep -q 'TestTopologicalSort ' test_integration.sh"
      - "grep -q 'TestIntegration_InvalidPlanParsing' test_integration.sh"
      - "grep -q 'TestCompatibility_TopologicalSortStability' test_integration.sh"

  - id: "validate-removal"
    agent: "validator"
    description: "Verify all seven acceptance criteria of issue #422 with the issue's exact commands, plus the full gate. Report PASS or FAIL for each criterion. Also check that no remaining test file other than registry_test.go and toposort_test.go has added lines against origin/main, that the moved TopologicalSort tests and function are byte-identical to the originals on origin/main, and that no spec file other than issue-422-delete-executor-and-viewmanager.md changed."
    dependencies: ["remove-plan-executor", "remove-plan-accessors", "remove-view-manager", "prune-integration-script"]
    acceptance_criteria:
      - "AC1: the Executor/types/constants/helpers grep over internal/plan prints nothing"
      - "AC2: the GetConfigPath/Count/GetTaskByID func grep prints nothing"
      - "AC3: the ViewManager/ViewType/ViewState/ViewConsole/ViewAgentOutput grep over internal/tui prints nothing and ls of both view_state files exits non-zero"
      - "AC4: the deleted-test-name grep over internal/plan and internal/tui prints nothing"
      - "AC5: GetTaskIDs and TestPlanGetTaskIDs each print one line"
      - "AC6: exactly one TopologicalSort func; go test -run TopologicalSort -v ./internal/plan exits 0 and runs TestTopologicalSortWithSimpleChain"
      - "AC7: go build ./... and task test exit 0; the numstat command prints only internal/plan/registry_test.go and internal/plan/toposort_test.go"
      - "task lint, task fmt:check and task sync:check pass"
      - "git diff --name-only origin/main -- .kairon/specs shows only issue-422-delete-executor-and-viewmanager.md"
    validation_commands:
      - "go build ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - |-
        test -z "$(grep -rnE '\b(Executor|NewExecutor|NewExecutorWithLimit|TaskSpawner|ExecutionSummary|TaskResult|TaskStatus[A-Za-z]*|DefaultMaxParallelTasks|createMockSpawnerWithLog|taskExecutionLog)\b' internal/plan --include=*.go)"
      - |-
        test -z "$(grep -rnE 'func \((r \*AgentRegistry\) (GetConfigPath|Count)|p \*Plan\) GetTaskByID)\(' internal/plan)"
      - |-
        test -z "$(grep -rnE '\b(ViewManager|NewViewManager|ViewType|ViewState|ViewConsole|ViewAgentOutput)\b' internal/tui --include=*.go)"
      - "! ls internal/tui/view_state.go internal/tui/view_state_test.go"
      - |-
        test -z "$(grep -rnE 'func (TestAgentRegistry_Count|TestAgentRegistry_GetConfigPath|TestExecutor[A-Za-z]*|TestNewExecutorWithLimit_NonPositiveFallsBackToDefault|TestIntegration_(MixedDependencies|ParallelExecution|SequentialExecution|TaskFailureHandling|EndToEnd)|TestCompatibility_ExecutorEmptyPlan|TestPlanGetTaskByID|TestPlanGetTaskIDs_EmptyPlan|TestViewManager|TestViewManagerTransitions|TestViewStatePreservation|TestTerminalResizeHandling)\(' internal/plan internal/tui --include=*_test.go)"
      - |-
        test "$(grep -n 'func (p \*Plan) GetTaskIDs' internal/plan/types.go | wc -l | tr -d ' ')" = 1
      - |-
        test "$(grep -n 'func TestPlanGetTaskIDs(' internal/plan/types_test.go | wc -l | tr -d ' ')" = 1
      - |-
        test "$(grep -n '^func TopologicalSort' internal/plan/*.go | wc -l | tr -d ' ')" = 1
      - |-
        go test -run 'TopologicalSort' -v ./internal/plan 2>&1 | grep -q -- '--- PASS: TestTopologicalSortWithSimpleChain'
      - |-
        test -z "$(git diff --numstat origin/main -- '*_test.go' | awk '$1 > 0 {print $3}' | grep -vxE 'internal/plan/(registry|toposort)_test.go')"
      - |-
        test -z "$(git diff --name-only origin/main -- .kairon/specs | grep -v 'issue-422-delete-executor-and-viewmanager.md')"
```
