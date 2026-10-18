# Design Spec: Watcher provenance evaluation and planner routing (issue #368)

Closes #368

## 1. Problem

`Watcher.checkIssues` (`internal/watcher/watcher.go`) runs each polled issue through: tracked, `hasActiveWorktree`, `hasExceededGlobalRetries`, backoff, `DependencyValidator.ValidateIssue`, then `manager.Spawn`. Nothing looks at provenance. Per planner-requirements.md §9 an issue with no planner marker, or whose body changed after the planner generated it (hash mismatch), must be canonicalized by the planner before the implementation pipeline sees it.

Assignee and label filtering is done by `github.ListIssues` (the `gh` arguments and the `-done`/`-failed` exclusion) before the watcher loop. It is not touched.

## 2. Findings that shape the design

1. **No classifier exists yet.** #367 is open. The watcher defines the consumer-side interface and the three-state type. #367 later satisfies the interface directly or through a one-line adapter.
2. **`checkIssues` is not unit-testable today.** The per-issue body is inline in the loop, calls `github.ListIssues`, and calls the concrete `*agent.Manager.Spawn`, which shells out to `worktree-create.sh` and `kiro-cli`. The acceptance criteria require driving the per-issue evaluation with fakes. The loop body therefore moves into `processIssue(issue)` and the two side effects (spawn, planner run) become replaceable unexported fields. This follows the existing convention in `dependencies.go` (`getIssueDetails issueLookupFunc // unexported; tests may replace it`).
3. **The manager cannot track planner runs.** `Manager.Spawn` creates a `spec/` worktree and launches `krew-lead`. `HandleExit` then verifies a PR exists, retries, adds `-done`/`-failed` labels and removes worktrees. A planner run produces no PR, so reusing `Spawn`/`agents` would make every successful planner run look like a failure and be retried, and could label the issue `kairon-failed`. **Decision: no change to `internal/agent`.** The watcher owns planner in-flight tracking and the production runner lives in `internal/watcher`. Planner runs do not appear in `status` (known limitation, §9).
4. **Zero-value `Watcher` is used in `internal/tui` tests** (`&watcher.Watcher{}`). New fields must be nil/zero safe. Lazily initialise maps, and never dereference the classifier or runner outside the poll path.
5. **`tracked` is never cleared.** Once an issue is spawned it is skipped until restart. Routing to the planner must not set `tracked`, otherwise the issue would never reach implementation after canonicalization. Routing must also not call `incrementGlobalRetryCount`, because planner runs are not implementation attempts and must not consume `max_retries`.
6. **The planner agent is interactive today** (`.kiro/agents/planner.json`, prompt restricts it to `gh issue create`). How the planner canonicalizes is explicitly out of scope. The watcher only starts it non-interactively the same way the manager starts krew-lead: `kiro-cli chat --agent planner --no-interactive --trust-all-tools "<prompt>"`.

## 3. Solution Approach

### 3.1 Types (new file `internal/watcher/provenance.go`)

```go
// Provenance is the watcher's view of an issue body's planner provenance.
type Provenance int

const (
    ProvenanceUnknown  Provenance = iota // zero value, invalid: fail closed
    ProvenanceValid                      // body carries a planner marker whose hash matches
    ProvenanceNone                       // no planner marker (manually created issue)
    ProvenanceMismatch                   // marker present, hash does not match (edited after generation)
)

func (p Provenance) String() string // "valid" | "none" | "mismatch" | "unknown"

// ProvenanceClassifier is satisfied by the #367 classify logic (directly or via an adapter).
type ProvenanceClassifier interface {
    Classify(issueBody string) Provenance
}

// ProvenanceClassifierFunc adapts a plain function (e.g. a #367 function) to ProvenanceClassifier.
type ProvenanceClassifierFunc func(issueBody string) Provenance
func (f ProvenanceClassifierFunc) Classify(b string) Provenance { return f(b) }

// PlannerRunner starts one planner run for an issue and blocks until it ends.
type PlannerRunner interface {
    Run(issue github.Issue, provenance Provenance) error
}
```

The zero value is `ProvenanceUnknown` (not `Valid`) so an uninitialised or buggy classifier cannot silently send an issue to implementation.

### 3.2 Watcher changes (`internal/watcher/watcher.go`)

New fields (all zero-safe):

| Field | Purpose |
|-------|---------|
| `classifier ProvenanceClassifier` | injected; nil means "no classifier wired" |
| `planner PlannerRunner` | production default set in `New`; tests replace it with a fake |
| `spawnAgent func(issueNumber int, repo string) (*agent.Agent, error)` | set in `New` to `mgr.Spawn`; tests replace it |
| `plannerInFlight map[int]struct{}` | issues with a planner run in progress, guarded by `w.mu` |
| `plannerAttempts map[int]int` | planner runs started per issue in this process, guarded by `w.mu` |

New exported method `SetProvenanceClassifier(c ProvenanceClassifier)` (takes `w.mu`). This is the single wiring point #367 or a follow-up uses from `cmd/kairon/cmd/root.go`. This issue does **not** edit `root.go` (outside `internal/watcher`).

**Production default when no classifier is wired: nil classifier means every issue is treated as `ProvenanceValid`.** Behaviour is identical to today until #367 supplies a classifier, so nothing regresses and nothing is routed to the planner by accident. No log line is needed for this default.

`checkIssues` becomes: poll, then `for _, issue := range issues { w.processIssue(issue) }`. `processIssue` is the existing loop body with each `continue` turned into `return`, plus the new step:

```
processIssue(issue):
  1. tracked                      -> return           (unchanged)
  2. hasActiveWorktree            -> return           (unchanged)
  3. hasExceededGlobalRetries     -> return           (unchanged)
  4. backoff gate                 -> return           (unchanged)
  5. dependency validation        -> return on error/unresolved (unchanged, incl. RecordFailure)
  6. NEW provenance:
       p := w.classify(issue.Body)       // nil classifier => Valid
       Valid    -> clear plannerAttempts[issue]; continue to step 7
       None, Mismatch -> w.routeToPlanner(issue, p); return
       default  -> log error "unrecognised provenance", return (fail closed: no spawn, no planner)
  7. tracked=true, incrementGlobalRetryCount, spawnAgent(...)   (unchanged)
```

Because step 6 runs after step 5, an unmarked issue with unresolved dependencies is skipped on dependencies and the classifier is never called (AC5). Steps 1 to 5 are not modified.

`routeToPlanner(issue, p)`:

1. Lock `w.mu`. If `w.planner == nil`, log and return.
2. If `plannerInFlight[issue.Number]` is set, log "planner run already in flight" and return (AC4).
3. If `plannerAttempts[issue.Number] >= w.config.MaxRetries`, log a warning and return. This bounds token spend if the planner exits without producing a valid body (for example while planner-side canonicalization is not implemented). It is in-memory, resets on restart, and resets when the issue is observed `Valid` (step 6) so a legitimate later edit can be re-canonicalized.
4. Lazily create the maps, set in-flight, increment attempts, unlock.
5. `go func()`: call `planner.Run(issue, p)`. On exit, lock and delete the in-flight entry. On error, log it and call `w.backoffTracker.RecordFailure(issue.Number)` so repeated failures back off using the existing mechanism.

Because the issue is neither `tracked` nor retry-counted, the next poll after a successful canonicalization (body now `Valid`) goes through step 7 normally.

### 3.3 Production planner runner (new file `internal/watcher/planner_runner.go`)

`kiroPlannerRunner{repo string, run func(*exec.Cmd) error}`; `New` sets `planner: newKiroPlannerRunner(cfg.Repo)` with `run` defaulting to `(*exec.Cmd).Run`.

- `command(issue, p) *exec.Cmd` builds (without starting) `kiro-cli chat --agent planner --no-interactive --trust-all-tools "<prompt>"`. The prompt names the issue number, repo and provenance (`none`: no planner marker; `mismatch`: edited after generation) and asks the planner to canonicalize the issue so its body carries a valid planner hash. Env mirrors the manager: `ISSUE_NUMBER`, `REPO`, `KAIRON_WATCHER_PID`. Working directory is the project root (the planner creates its own `.worktrees/planning-*` worktrees).
- `Run` opens `.kairon/logs/planner-issue-<n>.log` (create/append, `MkdirAll` the directory), points stdout and stderr at it, calls `r.run(cmd)`, and returns the error. No console writer and no output-capture hook (those live in `agent.Manager`).
- Planner-side canonicalization behaviour and the planner prompt are out of scope. Until they exist, a wired classifier will start planner runs that may not fix the body. The attempt cap in §3.2 limits the cost. `.kiro/agents/*` and `cmd/kairon/templates/` are **not** touched, so no template re-sync is needed. `task sync:check` must still pass unchanged.

## 4. Relevant Files

| File | Change |
|------|--------|
| `internal/watcher/provenance.go` | **new**: `Provenance`, `ProvenanceClassifier`, `ProvenanceClassifierFunc`, `PlannerRunner` |
| `internal/watcher/provenance_test.go` | **new**: `String()` and `ProvenanceClassifierFunc` tests |
| `internal/watcher/watcher.go` | modify: new fields, `SetProvenanceClassifier`, extract `processIssue`, provenance step, `routeToPlanner` |
| `internal/watcher/watcher_test.go` | **new**: characterization tests for existing criteria, then provenance and routing tests (AC1 to AC5) |
| `internal/watcher/planner_runner.go` | **new**: production `kiroPlannerRunner` |
| `internal/watcher/planner_runner_test.go` | **new**: command construction and `Run` tests with injected `run` |
| `README.md` | modify: document provenance routing under Issue Lifecycle |
| `internal/agent/*`, `cmd/`, `internal/tui/`, `.kiro/`, `cmd/kairon/templates/` | **not modified** |

Do not edit `.kairon/specs/` other than this file. Do not modify or stage `.kiro/agents/*.json` (modified by the sandbox, unrelated).

## 5. Test Design (TDD per AGENTS.md)

Test helper conventions for `watcher_test.go`:

- `t.Chdir(t.TempDir())` so `.worktrees/` and `.kairon/retries/` checks and writes are isolated (module is Go 1.26).
- Build the watcher with `New(&config.Config{Repo: "o/r", MaxRetries: 3, PollInterval: time.Minute}, nil, "")`, then override `w.spawnAgent` (records issue numbers) and `w.planner` (fake) and set `w.dependencyValidator.getIssueDetails` to a fake. No `gh`, no `kiro-cli`, no network.
- Fakes: `fakeClassifier{result Provenance; calls int}` (mutex-guarded), `fakePlanner` that records calls under a mutex and can block on a channel; use channels (not sleeps) to synchronise. Tests run under `-race` (`task test`).

**Characterization tests (written first, must pass before and after extracting `processIssue`):**

- tracked issue is skipped; issue with active worktree (`.worktrees/issue-<n>-<os.Getpid()>` dir) is skipped; issue at the retry limit (`.kairon/retries/issue-<n>.count` = MaxRetries) is skipped; issue with an open dependency is skipped and records a backoff; clean issue spawns once, becomes tracked and increments the retry count file.

**Feature tests (each must be seen failing for the expected reason first, i.e. an assertion failure, not a compile error; stub the new symbols so tests compile):**

| Test | Acceptance criterion |
|------|----------------------|
| `TestProcessIssue_ClassifiesEachState` table over Valid/None/Mismatch via injected fake classifier; asserts the classifier received the issue body | AC1 |
| `TestProcessIssue_ValidSpawns` valid body: `spawnAgent` called once, planner not called, tracked set, retry count incremented | AC2 |
| `TestProcessIssue_NilClassifierTreatedAsValid` default `New` watcher with no classifier spawns | default behaviour |
| `TestProcessIssue_NoneRoutesToPlanner`, `TestProcessIssue_MismatchRoutesToPlanner`: planner called once with the right issue and provenance, `spawnAgent` not called, issue not tracked, retry count file not written | AC3 |
| `TestProcessIssue_PlannerNotRepeatedWhileInFlight`: fake planner blocks; two `processIssue` calls for the same issue start the planner once; after release and completion, a third call starts it again | AC4 |
| `TestProcessIssue_DependenciesGateBeforeProvenance`: unmarked body plus open dependency: no planner call, no spawn, classifier call count 0, backoff recorded | AC5 |
| `TestProcessIssue_UnknownProvenanceFailsClosed`: classifier returns `ProvenanceUnknown`: no spawn, no planner | fail-closed |
| `TestRouteToPlanner_AttemptCap`: after `MaxRetries` completed runs for an unmarked issue the next poll does not start another; observing the issue `Valid` resets the counter | cap |
| `TestRouteToPlanner_FailureRecordsBackoff`: planner returns an error: in-flight entry cleared, `backoffTracker` has a failure for the issue | failure path |

**Runner tests (`planner_runner_test.go`):** `command` returns args `chat --agent planner --no-interactive --trust-all-tools <prompt>` where the prompt contains the issue number, repo and provenance word; env contains `ISSUE_NUMBER=<n>`, `REPO=<repo>`, `KAIRON_WATCHER_PID=`; `Run` with an injected `run` creates `.kairon/logs/planner-issue-<n>.log` in the temp dir, invokes `run` once and returns its error.

## 6. Team Orchestration

Single PR, all tasks inside `internal/watcher` plus a README note.

1. `implement-provenance-routing` (builder) does the types, the `processIssue` extraction, routing, in-flight tracking, the attempt cap, and all of §5 except the runner tests. It defines `PlannerRunner` so the runner task can implement it.
2. `implement-planner-runner` (builder) depends on task 1 (same package, `New` wiring, interface). It adds the production runner and its tests.
3. `document-provenance-routing` (documenter) depends on task 2.
4. `validate-provenance-routing` (validator) runs last, read-only.

No parallel builder tasks: both builder tasks edit `watcher.go`/`New`, so sequencing avoids conflicts.

## 7. Step-by-Step Task Breakdown

### Task 1: implement-provenance-routing (builder)
Add characterization tests, then feature tests (failing), then the types, fields, `SetProvenanceClassifier`, `processIssue` extraction, provenance step and `routeToPlanner`. In this task `New` leaves `planner` nil and `routeToPlanner` returns early when it is nil (the runner is wired in task 2). Commit tests and implementation together.

### Task 2: implement-planner-runner (builder)
Add `kiroPlannerRunner`, its tests, and wire it as the default `planner` in `New`. Check that no test in the repo (including `internal/tui` with `&watcher.Watcher{}`) breaks.

### Task 3: document-provenance-routing (documenter)
In `README.md` under "Issue Lifecycle" (or a new sibling subsection "Issue Provenance"), describe: provenance is evaluated after the other criteria; `valid` proceeds; `none`/`mismatch` are routed to the planner and not implemented; one in-flight planner run per issue and the `max_retries` cap per process; planner runs log to `.kairon/logs/planner-issue-<n>.log`; with no classifier wired every issue is treated as valid (current behaviour). Do not edit `CHANGELOG.md` or `.kairon/specs/` files other than this spec.

### Task 4: validate-provenance-routing (validator)
Read-only check of every acceptance criterion and scope constraints.

## 8. Validation Commands

QA commands from `Taskfile.yml`: `task lint` (`go vet ./...`), `task fmt:check` (`test -z "$(gofmt -l .)"`), `task test` (`go test -v -race -coverprofile=coverage.out ./...`), `task sync:check`. Targeted: `go test -race ./internal/watcher/... -count=1`.

## 9. Risks, Limitations and Open Questions

- **Planner behaviour is not implemented here.** If a classifier is wired before the planner can canonicalize, planner runs may not change the body. The per-process attempt cap (`max_retries`) stops the loop. After the cap the issue stays unprocessed until restart or until it becomes valid. This is logged as a warning.
- **Planner runs are not visible in `status`** and are not stopped by `manager.StopAll`. They are separate `kiro-cli` children owned by the watcher. A watcher restart clears the in-flight set, so an orphaned run could overlap a new one. Accepted to keep `internal/agent` unchanged. Planner log files give visibility.
- **Wiring the real classifier** (`watcher.SetProvenanceClassifier(...)` in `root.go`) belongs to #367 or a follow-up. If #367 defines its own result type, adapt it with `ProvenanceClassifierFunc`.
- **Each poll re-runs dependency validation** for unmarked issues that are waiting on an in-flight planner run. This is existing behaviour for any non-tracked issue and costs nothing extra when there are no dependencies.

## 10. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-provenance-routing"
    agent: "builder"
    description: "TDD in internal/watcher. First add characterization tests (watcher_test.go) for the existing per-issue criteria (tracked, active worktree, retry limit, unresolved dependency with backoff, clean issue spawns once) using t.Chdir(t.TempDir()), a watcher built with New(&config.Config{Repo:\"o/r\",MaxRetries:3,PollInterval:time.Minute}, nil, \"\"), and fake spawnAgent / dependencyValidator.getIssueDetails; extract the checkIssues loop body into processIssue(issue github.Issue) (continue -> return) and confirm they pass before and after. Then write the feature tests from spec section 5 and see them fail on assertions (stub new symbols so they compile). Then implement: provenance.go (Provenance with ProvenanceUnknown zero value + Valid/None/Mismatch and String(), ProvenanceClassifier interface, ProvenanceClassifierFunc, PlannerRunner interface Run(issue github.Issue, p Provenance) error); Watcher fields classifier, planner, spawnAgent (set to mgr.Spawn in New), plannerInFlight, plannerAttempts (lazily initialised, guarded by w.mu, zero-value Watcher safe); SetProvenanceClassifier; the provenance step after dependency validation (nil classifier => Valid; Valid clears plannerAttempts and proceeds to the unchanged tracked/increment/spawn; None/Mismatch => routeToPlanner and return without setting tracked or incrementing the retry count; any other value => log and return); routeToPlanner (nil planner => log and return; skip if in flight; skip if plannerAttempts >= config.MaxRetries; run planner.Run in a goroutine, clear in-flight on exit, RecordFailure on error). Do not change steps 1-5 of the existing criteria, ListIssues, internal/agent, cmd/, internal/tui or .kiro/. Commit tests and implementation together and name the first-failing tests in the commit message."
    dependencies: []
    acceptance_criteria:
      - "Characterization tests for tracked, active worktree, retry limit, unresolved dependency and clean spawn exist and pass both before and after the processIssue extraction"
      - "A table test drives processIssue with Valid, None and Mismatch through an injected fake ProvenanceClassifier and asserts the classifier received the issue body (AC1)"
      - "A Valid body spawns the implementation agent exactly once, does not call the planner, sets tracked and increments the retry count file (AC2)"
      - "A watcher with no classifier wired treats the issue as Valid and spawns it (documented production default)"
      - "A None body and a Mismatch body each call the fake planner once with the correct issue and provenance, never call spawnAgent, leave the issue untracked and do not write .kairon/retries/issue-<n>.count (AC3)"
      - "With a blocking fake planner, two processIssue calls for the same unmarked issue start exactly one planner run; after it finishes a later call starts a new run (AC4)"
      - "An unmarked body with an open dependency is skipped on dependencies: planner not called, spawnAgent not called, classifier call count is 0, backoff failure recorded (AC5)"
      - "A classifier returning ProvenanceUnknown results in neither a spawn nor a planner run"
      - "After config.MaxRetries planner runs for one issue no further run starts; observing the issue Valid resets the counter; a planner error clears the in-flight entry and records a backoff failure"
      - "Steps 1-5 of the existing criteria are behaviourally unchanged; zero-value &watcher.Watcher{} still compiles and internal/tui tests pass"
      - "Tests were observed failing on assertions before the implementation; tests and implementation are in the same commit; tests run under -race with no real gh or kiro-cli"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/watcher/..."
      - "test -z \"$(gofmt -l internal/watcher)\""
      - "go test -race ./internal/watcher/... -count=1"
      - "go test ./internal/tui/... -count=1"
      - "test -z \"$(git -P diff --name-only origin/main -- internal/agent cmd internal/tui cmd/kairon/templates)\""

  - id: "implement-planner-runner"
    agent: "builder"
    description: "TDD in internal/watcher. Add planner_runner.go with kiroPlannerRunner{repo string, run func(*exec.Cmd) error} implementing PlannerRunner, newKiroPlannerRunner(repo) defaulting run to (*exec.Cmd).Run, command(issue, p) building (not starting) `kiro-cli chat --agent planner --no-interactive --trust-all-tools <prompt>` with a prompt naming the issue number, repo and provenance (none: no planner marker; mismatch: edited after generation) and asking for canonicalization, and env ISSUE_NUMBER, REPO, KAIRON_WATCHER_PID. Run opens .kairon/logs/planner-issue-<n>.log (MkdirAll, create/append), sets stdout and stderr to it, calls r.run and returns the error. Wire New to set planner: newKiroPlannerRunner(cfg.Repo). Write planner_runner_test.go first and see it fail on assertions: args and prompt content, env entries, log file created under a t.Chdir temp dir, injected run invoked once, its error returned. Do not touch internal/agent, .kiro/ or cmd/kairon/templates; no template re-sync is needed. Commit tests and implementation together."
    dependencies: ["implement-provenance-routing"]
    acceptance_criteria:
      - "command(issue, ProvenanceNone) and command(issue, ProvenanceMismatch) produce args chat --agent planner --no-interactive --trust-all-tools <prompt>, with a prompt containing the issue number, the repo and the provenance word"
      - "The command env contains ISSUE_NUMBER=<n>, REPO=<repo> and KAIRON_WATCHER_PID=<pid>"
      - "Run creates .kairon/logs/planner-issue-<n>.log, invokes the injected run function exactly once and returns its error unchanged"
      - "New wires the production runner as the default planner; New-built watchers still have a nil classifier so behaviour is unchanged until a classifier is set"
      - "No test starts a real kiro-cli or gh"
      - "No diff in internal/agent, cmd/, internal/tui, .kiro/ (other than the sandbox's pre-existing agent json changes, which must not be staged) or cmd/kairon/templates; task sync:check passes"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/watcher/..."
      - "test -z \"$(gofmt -l internal/watcher)\""
      - "go test -race ./internal/watcher/... -count=1"
      - "go test ./... -count=1"
      - "task sync:check"

  - id: "document-provenance-routing"
    agent: "documenter"
    description: "Update README.md (live doc, edit freely) with a subsection 'Issue Provenance' next to 'Issue Lifecycle' / 'Issue Dependencies': provenance (valid, none, mismatch) is evaluated after assignee/label, worktree, retry and dependency criteria; valid issues proceed to the implementation workflow unchanged; none (no planner marker) and mismatch (edited after generation) issues are routed to the planner for canonicalization and are not implemented; at most one in-flight planner run per issue and at most max_retries planner runs per issue per watcher process; planner runs log to .kairon/logs/planner-issue-<n>.log and are not shown in status; with no classifier wired every issue is treated as valid, which is the current behaviour. Do not edit CHANGELOG.md or any file under .kairon/specs/ other than issue-368."
    dependencies: ["implement-planner-runner"]
    acceptance_criteria:
      - "README.md has a provenance subsection covering the three states, their routing, the in-flight and attempt-cap rules, the log path and the no-classifier default"
      - "The text states that provenance is evaluated after the existing criteria and does not change them"
      - "No changes to CHANGELOG.md or to files under .kairon/specs/ besides this spec"
    validation_commands:
      - "grep -n 'Issue Provenance' README.md"
      - "grep -n 'planner-issue-' README.md"
      - "test -z \"$(git -P diff --name-only origin/main -- CHANGELOG.md)\""

  - id: "validate-provenance-routing"
    agent: "validator"
    description: "Read-only verification of issue #368 acceptance criteria: per-issue evaluation classifies via an injected classifier (AC1); a valid body spawns the implementation agent (AC2); none and mismatch bodies route to the planner and never spawn (AC3); a second poll does not start a second in-flight planner run (AC4); an unmarked issue with an unresolved dependency is skipped on dependencies and the classifier/planner are not called (AC5). Confirm steps 1-5 of the existing criteria are unchanged, the classifier is injected (no package-level call), nil classifier defaults to valid, tests run without gh or kiro-cli, tests and implementation share a commit, and scope is limited to internal/watcher plus README.md."
    dependencies: ["implement-provenance-routing", "implement-planner-runner", "document-provenance-routing"]
    acceptance_criteria:
      - "go test -race ./internal/watcher/... passes and includes tests named for each of AC1 to AC5 plus the nil-classifier default and unknown fail-closed cases"
      - "task test, task lint, task fmt:check and task sync:check all pass"
      - "git diff against origin/main touches only internal/watcher/* and README.md plus the architect spec and sentinel artifacts; internal/agent, cmd/, internal/tui, cmd/kairon/templates and CHANGELOG.md are unchanged; .kiro/agents/*.json are not staged or committed"
      - "Planner routing does not set tracked and does not write .kairon/retries/issue-<n>.count"
      - "README.md documents provenance routing"
    validation_commands:
      - "go test -race ./internal/watcher/... -count=1 -v"
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "test -z \"$(git -P diff --name-only origin/main -- internal/agent cmd internal/tui cmd/kairon/templates CHANGELOG.md)\""
      - "grep -n 'Issue Provenance' README.md"
```
