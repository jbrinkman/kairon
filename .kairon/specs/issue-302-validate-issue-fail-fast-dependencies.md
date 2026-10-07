# Design Spec: Fail fast on first open dependency in ValidateIssue and decouple circular check

Closes #302

## 1. Problem

`DependencyValidator.ValidateIssue` (`internal/watcher/dependencies.go`) spends more GitHub API calls (`gh issue view`, one process spawn each) than the result needs. Verified in code:

1. After parsing, it calls `checkCircularDependencies(repo, issueNumber, issueBody, visited)`. That function calls `github.GetIssueDetails` for every direct dependency and recurses into each dependency's own dependencies: a full chain walk.
2. It then calls `github.GetIssueDetails` once for **every** direct dependency, collecting all non-closed ones (and every lookup error) into `unresolved`, with `continue` after each.

The only consumer is `Watcher.poll` (`internal/watcher/watcher.go`, ~lines 115-139). It:

- logs `CircularDependencies` as a warning only (it does not block);
- skips the issue as soon as `IsValid == false` and calls `backoffTracker.RecordFailure(issue.Number)`.

It never uses the number or identity of unresolved dependencies for control flow (only prints them). So once one dependency is known open, further lookups cannot change the outcome, and the circular walk cannot change it at all.

`ValidateIssue` and `NewDependencyValidator` have exactly one non-test caller (`watcher.go:38` and `watcher.go:122`). `BackoffTracker` is independent of this change.

## 2. Solution Approach

Change only `ValidateIssue`'s body, plus one minimal seam so tests can count lookups.

### 2.1 `ValidateIssue` new behavior

```go
func (dv *DependencyValidator) ValidateIssue(repo string, issueNumber int, issueBody string) (*ValidationResult, error) {
    dependencies := dv.parser.ParseDependencies(issueBody)
    if len(dependencies) > 0 {
        log.Printf("[watcher] parsed %d dependencies for issue #%d: %v", len(dependencies), issueNumber, dependencies)
    }
    if len(dependencies) == 0 {
        return &ValidationResult{IsValid: true}, nil // AC6: no API calls
    }

    // Fail fast: the watcher skips an issue on the first unresolved dependency,
    // so stop at the first one. Order is the order ParseDependencies returns.
    for _, dep := range dependencies {
        depDetails, err := dv.getIssueDetails(repo, dep)
        if err != nil {
            log.Printf("[watcher] error checking dependency #%d for issue #%d: %v", dep, issueNumber, err)
            return &ValidationResult{UnresolvedDependencies: []int{dep}}, nil // AC4
        }
        if !strings.EqualFold(depDetails.State, "closed") {
            log.Printf("[watcher] dependency #%d for issue #%d is in state '%s' (not closed)", dep, issueNumber, depDetails.State)
            return &ValidationResult{UnresolvedDependencies: []int{dep}}, nil // AC2, AC3
        }
    }
    return &ValidationResult{IsValid: true}, nil // AC5
}
```

Decisions:

- **Return shape on early stop:** `IsValid: false`, `UnresolvedDependencies: []int{dep}` (exactly one element), `CircularDependencies: nil`. `ValidateIssue` still returns a nil error in these cases (same as today: lookup errors are folded into "unresolved", never returned). The error return value stays for API compatibility.
- **Ordering:** iterate `dependencies` exactly as `ParseDependencies` returned it. Do not sort. Note that this order is by pattern group (`depends on issue`, `blocked by`, `depends on [issue]`, then the comma list), not by position in the body. That is the "order ParseDependencies returns" the AC asks for. Do not change `ParseDependencies` or the regexes.
- **Circular check removed from the hot path:** delete the `visited`/`checkCircularDependencies` call and the `circular` variable, and drop `CircularDependencies:` from the returned literals. Because no result is ever built with it, the field stays nil. `ValidationResult.CircularDependencies` (the struct field) and the watcher's `len(validationResult.CircularDependencies) > 0` warning block are left untouched; the warning becomes inert (AC8). `checkCircularDependencies` stays byte-for-byte unchanged and uncalled (AC7). It is a package-private method on `*DependencyValidator`, and the Go compiler does not flag unused methods. Lint (`unused` in golangci-lint) is the one risk: check `.golangci.yml`/`task lint` output. If `unused` reports it, do not delete the function. Instead add a short comment (e.g. `//nolint:unused // retained for future circular-dependency detection (#302)`) **above** the function without altering its body or signature, and mention this in the PR. If lint is clean, add nothing.
- **Skip log:** the watcher's skip log prints `UnresolvedDependencies`, which now holds at most one dependency. This is expected (constraint). No change to `watcher.go`.
- **Do not** add a new circular-dependency trigger, change `BackoffTracker`, or change the parsing formats.

### 2.2 Test seam

`ValidateIssue` uses the package-level `github.GetIssueDetails`, which shells out to `gh`; it cannot be counted in unit tests. Add a minimal injectable lookup on the validator:

```go
// issueLookupFunc fetches issue details; defaults to github.GetIssueDetails.
type issueLookupFunc func(repo string, number int) (*github.IssueDetails, error)

type DependencyValidator struct {
    parser          *DependencyParser
    getIssueDetails issueLookupFunc // unexported; tests may replace it
}

func NewDependencyValidator() *DependencyValidator {
    return &DependencyValidator{
        parser:          &DependencyParser{},
        getIssueDetails: github.GetIssueDetails,
    }
}
```

- The field and type are unexported: the public API (`NewDependencyValidator()`, `ValidateIssue` signature, `ValidationResult`) is unchanged.
- Tests (same package `watcher`) build `dv := NewDependencyValidator()` and then set `dv.getIssueDetails = fake`.
- `ValidateIssue` calls `dv.getIssueDetails(...)`. `checkCircularDependencies` is deliberately left calling `github.GetIssueDetails` directly, since the AC requires it to stay unchanged.
- Fields on `github.IssueDetails` used: `State` and `Body`. Confirm the exact struct in `internal/github` (JSON `body,state`) when writing fakes.

### 2.3 Docs

Searched `README.md`, `CONTRIBUTING.md`, `docs/`, `.kiro/` and the templates for "dependency validation", "circular", "ValidateIssue", "depends on", "blocked by". There is **no user-facing mention** of dependency validation, its circular check, or the list of unresolved dependencies in the live docs (the only hit is the architect prompt's unrelated "avoid circular dependencies" plan rule). No README/docs change is needed, and nothing in a template-synced path changes, so `task sync:check` is unaffected. Do not edit anything under `.kairon/specs/` other than this file (AGENTS.md).

## 3. Relevant Files

Modify:
- `internal/watcher/dependencies.go`: new `ValidateIssue` loop, `getIssueDetails` field and `NewDependencyValidator` init. `checkCircularDependencies`, `ValidationResult`, parser and `BackoffTracker` are unchanged.
- `internal/watcher/dependencies_test.go`: add `ValidateIssue` tests (new imports `errors`, `github`; `reflect` already imported).

Read-only / must not change:
- `internal/watcher/watcher.go`: the `CircularDependencies` warning and skip logic stay as is.
- `internal/github/*.go`: `GetIssueDetails`, `IssueDetails`.

## 4. Team Orchestration

Two builder tasks plus a validator pass, all in one PR:

1. `implement-fail-fast`: the production change and seam. No dependencies.
2. `add-validate-issue-tests`: unit tests. They need the seam from task 1 to compile, so they depend on it. (They are small enough that krew-lead could also run both in one builder session; the dependency keeps ordering explicit.)
3. `validate-complete`: read-only verification of every AC and the existing suites.

## 5. Step-by-Step Task Breakdown

### Task 1: implement-fail-fast
- In `internal/watcher/dependencies.go` add `issueLookupFunc`, the `getIssueDetails` field on `DependencyValidator`, and its initialization in `NewDependencyValidator` to `github.GetIssueDetails`.
- Rewrite `ValidateIssue` per §2.1: remove the `checkCircularDependencies` call and `visited`/`circular`; loop through `dependencies` in order via `dv.getIssueDetails`; return on the first error or non-closed state with `UnresolvedDependencies: []int{dep}`; return `IsValid: true` after all are closed; keep the zero-dependency early return before any lookup.
- Keep the existing log lines. Leave `checkCircularDependencies` unchanged; handle an `unused` lint finding only as described in §2.1.
- Dependencies: none.

### Task 2: add-validate-issue-tests
Add table-style or sub-tests in `internal/watcher/dependencies_test.go` using a fake lookup that records the requested issue numbers (a `calls []int` slice) and returns scripted `*github.IssueDetails` / errors. Bodies use the `Dependencies: #A, #B, #C` format so the parse order is deterministic (`[A, B, C]`). Required cases:
1. **First dep open:** deps `#10, #11, #12`, #10 `open`. Expect lookups `== [10]`, `IsValid == false`, `UnresolvedDependencies == [10]`, `CircularDependencies` empty.
2. **Later dep open:** deps `#10, #11, #12`, #10 `closed`, #11 `open`. Expect lookups `== [10, 11]` (stops at 11, #12 never looked up), `IsValid == false`, `UnresolvedDependencies == [11]`.
3. **All closed:** all three `closed`. Expect lookups `== [10, 11, 12]`, `IsValid == true`, `UnresolvedDependencies` empty. Also cover a mixed-case state (`"CLOSED"`) to keep the `EqualFold` behavior.
4. **Lookup error:** #10 closed, #11 returns an error. Expect lookups `== [10, 11]`, `IsValid == false`, `UnresolvedDependencies == [11]`, and `err == nil`. Add a variant where the first dep errors (lookups `== [10]`).
5. **No dependencies:** body without deps. Expect zero lookups, `IsValid == true`.
6. **No circular fetches:** a body whose dependency's fake body points back to the issue (a cycle). Expect only the direct lookup(s) (no recursive chain fetch) and `CircularDependencies` empty. This demonstrates AC1 via the call count.

Assert the exact slice of requested numbers (not just a count) so the order and stopping point are verified. Do not call `gh`; every test must replace `getIssueDetails`.
- Dependencies: `implement-fail-fast`.

### Task 3: validate-complete
- Verify each AC 1-10 against the code and tests; confirm `checkCircularDependencies` body unchanged (`git diff` shows no hunk inside it), `watcher.go` unmodified, `BackoffTracker` and parser unmodified, public API unchanged.
- Run the validation commands below.
- Dependencies: `add-validate-issue-tests`.

## 6. Validation Commands

```bash
go build ./...
go vet ./internal/watcher/...
go test ./internal/watcher/... -run 'ValidateIssue|ParseDependencies|BackoffTracker' -v
go test ./internal/watcher/... ./internal/github/...
task test
task lint
task fmt:check
git diff --stat -- internal/watcher/watcher.go   # expect no output
```

## 7. Acceptance Criteria Traceability

| AC | Where addressed |
|----|-----------------|
| 1 | Task 1 (call removed); test case 6 |
| 2, 3 | Task 1 loop; test cases 1, 2 |
| 4 | Task 1 error branch; test case 4 |
| 5 | Task 1; test case 3 |
| 6 | Task 1 early return; test case 5 |
| 7 | Task 1 leaves function untouched; Task 3 diff check |
| 8 | `ValidationResult` and `watcher.go` untouched; Task 3 |
| 9 | Task 2 cases 1-4 |
| 10 | Task 3 runs the full watcher/dependency suites |

## 8. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-fail-fast"
    agent: "builder"
    description: "In internal/watcher/dependencies.go, add an unexported injectable issue-lookup field (defaulting to github.GetIssueDetails) on DependencyValidator, and rewrite ValidateIssue to drop the checkCircularDependencies call and fail fast on the first non-closed or errored dependency, in ParseDependencies order. Leave checkCircularDependencies, ValidationResult, the parser, BackoffTracker and watcher.go unchanged."
    dependencies: []
    acceptance_criteria:
      - "ValidateIssue no longer calls checkCircularDependencies and no longer creates a visited map"
      - "ValidateIssue iterates dependencies in ParseDependencies order and returns at the first non-closed dependency without further lookups"
      - "On early stop IsValid is false and UnresolvedDependencies contains only that first dependency; CircularDependencies is nil"
      - "A GetIssueDetails error for a dependency stops the loop and is reported as the sole unresolved dependency, with a nil error returned"
      - "When every dependency is closed all are checked and IsValid is true with empty UnresolvedDependencies"
      - "An issue with no dependencies returns IsValid true with zero lookups"
      - "checkCircularDependencies body and signature are unchanged and ValidationResult.CircularDependencies still exists"
      - "NewDependencyValidator() and ValidateIssue signatures are unchanged (seam is unexported)"
      - "internal/watcher/watcher.go has no diff"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/watcher/..."
      - "git diff --exit-code -- internal/watcher/watcher.go"

  - id: "add-validate-issue-tests"
    agent: "builder"
    description: "Add unit tests for ValidateIssue in internal/watcher/dependencies_test.go using a fake lookup via the getIssueDetails seam that records requested issue numbers: first dep open, later dep open, all closed (including mixed-case state), lookup error (first and later dep), no dependencies, and a self-referencing cycle producing no recursive fetches."
    dependencies: ["implement-fail-fast"]
    acceptance_criteria:
      - "Test asserts lookups == [first dep] and UnresolvedDependencies == [first dep] when the first dependency is open"
      - "Test asserts lookups stop at the later open dependency (later deps never looked up) and UnresolvedDependencies holds only that dependency"
      - "Test asserts all dependencies are looked up and IsValid is true with empty UnresolvedDependencies when all are closed"
      - "Test asserts a lookup error stops the loop, is reported as the sole unresolved dependency, and ValidateIssue returns a nil error"
      - "Test asserts zero lookups and IsValid true for a body without dependencies"
      - "Test asserts no recursive chain fetches occur for a cyclic dependency body and CircularDependencies is empty"
      - "No test invokes the real gh CLI"
    validation_commands:
      - "go test ./internal/watcher/... -run 'ValidateIssue' -v"
      - "go test ./internal/watcher/..."

  - id: "validate-complete"
    agent: "validator"
    description: "Verify all acceptance criteria of issue #302: fail-fast semantics, decoupled circular check, unchanged checkCircularDependencies/ValidationResult/watcher.go/BackoffTracker/parser, new tests present and passing, existing tests, lint and formatting green."
    dependencies: ["add-validate-issue-tests"]
    acceptance_criteria:
      - "All ten issue acceptance criteria are verified against the code and tests"
      - "git diff shows no change inside checkCircularDependencies, BackoffTracker, ParseDependencies, or internal/watcher/watcher.go"
      - "All watcher and dependency tests pass"
      - "task lint and task fmt:check pass"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/watcher/... ./internal/github/..."
      - "task test"
      - "task lint"
      - "task fmt:check"
```
