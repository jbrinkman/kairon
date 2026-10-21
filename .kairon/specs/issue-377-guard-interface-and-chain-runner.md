# Design Spec: Guard interface and ordered guard-chain runner

Closes #377

Design reference: `.kairon/specs/maturity-model/planner-requirements.md` §4, §4.1, §4.2, §4.4, §11, §14, principle 11.
Depends on #376 (closed): `internal/stageoutput`, `docs/stage-output.md`.

## 1. Problem and Goal

The pipeline needs deterministic checks between stages. A boundary is an ordered chain of small guards that all share
one two-method interface (§4.1): an **admission** method (pure pass/fail) and a **failure** method (the guard's own
retry-or-punch-out action). No such interface or runner exists.

Delivered in one PR: a new package `internal/guard` holding the `Guard` interface, `AdmissionResult`,
`FailureContext`, the chain runner, one structural reference guard, tests, and a `docs/guards.md` reference. Nothing
in the watcher or any stage calls the runner yet.

Out of scope (do not implement): the provenance/commit terminal step (#378); what a failure method actually does,
meaning the two-tier retry-then-punch-out behavior and the attempt counter (#379); security and correctness guards;
configurable chains; wiring the runner into the watcher or any stage (#381, #382).

## 2. Solution Approach

### 2.1 Package and dependency rule

New package **`internal/guard`** (import path `github.com/jbrinkman/kairon/internal/guard`).

- Imports the standard library and `internal/stageoutput` only. It must not import `internal/agent`,
  `internal/watcher`, `internal/github`, `internal/acp`, or any other internal package, and must not directly import
  `os`, `os/exec`, `net` or `net/http`. This is how the constraints "deterministic Go, never an agent" and "no
  agent-spawn dependency" and AC 4 "no GitHub or shared-state write by the runner" become testable.
- `TestNoForbiddenImports` enforces it with `go list -f '{{join .Imports "\n"}}' .` (direct imports of the non-test
  package) and `go list -deps .` (the only `jbrinkman/kairon/internal/...` packages allowed are `guard` and
  `stageoutput`). Direct imports are used for the `os`/`net` check because `os` is reachable transitively through
  `fmt` and `yaml.v3`. Same technique as `internal/checks/deps_test.go` and `internal/stageoutput/deps_test.go`.

### 2.2 Types (file `guard.go`)

```go
// Attempt is the retry position for a boundary (§14.1). The boundary supplies it; this package never counts.
type Attempt struct{ Current, Max int }

// Input is what a guard's admission method receives. It is a value type with no pointer, slice or map fields,
// so a guard cannot mutate the document or anything the runner holds.
type Input struct {
    Stage   stageoutput.Stage // the stage whose output is being admitted (boundary-supplied, so it is known even when Raw does not parse)
    Raw     string            // the exact text the stage returned: a stage-output document (YAML)
    Attempt Attempt
}

type FailureContext struct {
    Stage    stageoutput.Stage
    Guard    string  // name of the guard whose admission failed
    Attempt  Attempt // current + max
    Expected string  // what admission required
    Actual   string  // what it observed
}

type AdmissionResult struct {
    Pass    bool
    Failure *FailureContext // non-nil iff !Pass
}

type Guard interface {
    // Admit judges the stage output. Pure: no document mutation, no GitHub or shared-state write, no failure action.
    Admit(in Input) AdmissionResult
    // OnFailure receives the FailureContext Admit produced and enacts this guard's retry-vs-punch-out decision.
    // The runner calls it at most once per run, and only on the guard whose Admit failed.
    OnFailure(fc FailureContext) error
}
```

Helpers: `func Admitted() AdmissionResult` (Pass true, nil Failure) and
`func (in Input) Reject(guard, expected, actual string) AdmissionResult` (builds a `FailureContext` from `in.Stage`
and `in.Attempt`, so a guard cannot forget those two fields). `func (a Attempt) Valid() bool` (`Current >= 1 &&
Max >= Current`). `func (in Input) Document() (*stageoutput.Document, error)` calls `stageoutput.Parse([]byte(in.Raw))`
on every call; it is deliberately not memoized, so there is no hidden shared state and each caller gets its own
copy. `Validate()` methods on `Input`, `FailureContext` and `AdmissionResult` return an error wrapping the
package sentinel `ErrInvalid` (see 2.4).

Why the interface has exactly two methods and no `Name()`: the issue and §4.1 fix the interface at two. The guard
name travels in `FailureContext.Guard`, which the guard sets itself.

Why `Input` carries the raw text and not a parsed `*stageoutput.Document` (decision for the reviewer): the
structural guard must report "does not parse under the strict schema" as an ordinary admission failure with a
`FailureContext`. If the runner parsed first, a malformed document would fail before any guard existed to own the
failure method, which contradicts §4.1 ("the guard owns the failure decision"). Guards that need the parsed form call
`in.Document()`. `Raw` is a `string` rather than `[]byte` so the type is immutable by construction.

### 2.3 Chain runner (file `chain.go`)

```go
type Outcome int
const (
    OutcomeInvalid       Outcome = iota // zero value; never returned with a nil error
    OutcomeReadyToCommit                // every guard passed (§4.2 step 2)
    OutcomeRejected                     // a guard failed and its OnFailure was invoked
)

type Result struct {
    Outcome    Outcome
    Failure    *FailureContext // set iff Outcome == OutcomeRejected: the context handed to the failing guard's OnFailure
    FailureErr error           // what that OnFailure returned (nil = handled); only meaningful when Rejected
}

func Run(in Input, guards []Guard) (Result, error)
```

`Run` algorithm:

1. Validate `in` and `guards`. An empty or nil slice returns `ErrEmptyChain`; a nil element returns `ErrInvalid`
   naming the index. (An empty chain is rejected because a boundary with no guards would silently admit everything;
   §4.2's "every guard passed" is not meant to be vacuous.) These errors are returned before any guard runs.
2. For each guard in slice order: call `Admit(in)` once. Validate the result: `Pass` true requires a nil `Failure`;
   `Pass` false requires a non-nil `Failure` that passes `FailureContext.Validate()` (valid stage, non-empty guard,
   valid attempt, non-empty expected and actual). A violation returns a `*ContractError` (matches `ErrInvalid`)
   naming the guard index and the problem; the runner does not call `OnFailure` (there is no valid context to give)
   and runs no later guard.
3. On pass, continue. On fail, call that same guard's `OnFailure(*result.Failure)` exactly once, stop (no later
   `Admit`, no later `OnFailure`), and return `Result{OutcomeRejected, Failure: &copy, FailureErr: err}` with a nil
   `error`.
4. When every guard passes, return `Result{Outcome: OutcomeReadyToCommit}`. The runner performs no side effect; the
   caller (the boundary, #378) commits.

The runner is a stateless function. It does not recover panics from guards (a panicking guard is a programming
error that should be loud), does not retry, does not count attempts, and does not interpret `FailureErr`. Handing
`OnFailure` a copy of the `FailureContext` (a plain value type) means a handler cannot alter what `Result` reports.

The two return channels differ on purpose: `error` means the chain could not be run or a guard broke the contract;
`Result.FailureErr` means the chain ran, a guard rejected the output, and the guard's own failure action returned an
error. #379 decides what a non-nil `FailureErr` triggers.

### 2.4 Errors

`ErrInvalid` (bad input, nil guard, broken admission contract), `ErrEmptyChain` (wraps/matches `ErrInvalid`), and
`ErrNoFailureHandler` (see 2.5). `ContractError{GuardIndex int; Problems []string}` with `Is(ErrInvalid)`.

### 2.5 Structural reference guard (file `structural.go`)

```go
const StructuralName = "structural"

type FailureHandler func(FailureContext) error

var DefaultRequiredSections = []string{"Problem", "Context", "Scope", "Acceptance Criteria", "Dependencies"}

type Structural struct {
    RequiredSections []string       // nil means DefaultRequiredSections
    handler          FailureHandler // supplied by the boundary; #379 provides the real retry/punch-out handler
}

func NewStructural(handler FailureHandler) *Structural
```

`Admit` checks, in order, stopping at the first group that fails so `Expected`/`Actual` stay about one thing:

1. **Schema.** `in.Document()` must succeed. On error: `Expected` = "document parses under the strict stage-output
   schema (stageoutput.Parse)", `Actual` = the parse error text (`ErrUnknownVersion`, `ErrInvalidDocument` and yaml
   errors all surface here).
2. **Punch-out passes.** `Result == punch_out` carries no body to inspect, and the schema check already proved it is
   well-formed, so the guard admits it. What a punch-out document triggers is routed by #379/#382, not here.
3. **Required sections (canonical only).** Scan `Body` for markdown headings (`^#{1,6}\s+text`), skipping lines inside
   fenced code blocks (a fenced ` ## Problem ` is not a heading). A required name `N` is present when some heading,
   lower-cased with trailing `:` and punctuation trimmed, equals `N` (lower-cased) or starts with `N` followed by a
   non-alphanumeric character or end of text. So `## Problem Statement` satisfies `Problem` and
   `## Scope — In and Out` satisfies `Scope`. On any missing section: `Expected` = "sections present: Problem, Context,
   ..." (the full required list) and `Actual` = "missing sections: Context, Dependencies" (the missing ones, in required
   order).
4. **`Verify:` lines.** If an `Acceptance Criteria` section exists (heading match as above; content runs to the next
   heading of the same or higher level), split it into items. An item starts at a list marker (`-`, `*`, `+`, `1.` or
   `1)`) whose indent is at most that of the first item; deeper-indented marker lines and plain lines are continuation
   text of the current item. Each item's text must match `(?i)\bverify(\*\*|__)?:`. Zero items is a failure. On failure:
   `Expected` = "every acceptance criterion carries a Verify: line", `Actual` = "criteria without Verify: 2, 4 (of 5)"
   or "Acceptance Criteria section has no list items". If `Acceptance Criteria` is not a section of the body, step 3
   already reported it (when required), and this step is skipped.

   Limits of the reference guard (state them in the doc, do not build them): it does not require non-blank text after
   `Verify:`, does not check that Out of Scope has an entry, title length, or the single-line `Dependencies:` form
   (§7). Those are further guards.

`OnFailure(fc)` calls the injected handler. With a nil handler it returns `ErrNoFailureHandler`, so a failure is never
swallowed silently (§14 intro) before #379 supplies real handling.

`Reject` is always called with `StructuralName` as the guard name.

**Chain order, hard-coded (§4.4):** `func DefaultChain(handler FailureHandler) []Guard` returns
`[]Guard{NewStructural(handler)}`. It is the single place the order is written; `Run` still takes the slice.

## 3. Relevant Files

Create:

| Path | Purpose |
|------|---------|
| `internal/guard/doc.go` | Package comment: purpose, the two-method rule, the import restriction, pointer to `docs/guards.md` |
| `internal/guard/guard.go` | `Guard`, `Input`, `Attempt`, `AdmissionResult`, `FailureContext`, helpers, `Validate` methods, errors |
| `internal/guard/chain.go` | `Outcome`, `Result`, `Run`, `ContractError`, `DefaultChain` |
| `internal/guard/structural.go` | `Structural` guard, `DefaultRequiredSections`, heading/list scanning |
| `internal/guard/guard_test.go` | AC 1 and AC 2: trivial guard, no `FailureContext` on pass, interface shape via `reflect`, immutable `Input`, `FailureContext` completeness |
| `internal/guard/chain_test.go` | AC 3, 4, 5, empty chain, nil guard, contract violations, `FailureErr` pass-through, `DefaultChain` order |
| `internal/guard/structural_test.go` | AC 6 table test plus scanner edge cases |
| `internal/guard/deps_test.go` | `TestNoForbiddenImports` (2.1) |
| `internal/guard/docs_test.go` | The doc exists, names the required terms, and its YAML example parses with the structural guard |
| `docs/guards.md` | The reference (section 5, task 3) |

Modify:

| Path | Change |
|------|--------|
| `README.md` | One sentence linking `docs/guards.md`, next to the existing `docs/stage-output.md` link |

Read-only: `internal/stageoutput/*` (`Parse`, `Document`, `Stage.Valid`), `internal/checks/deps_test.go` and
`internal/stageoutput/deps_test.go` (dependency-test pattern), `internal/stageoutput/docs_test.go` (docs-test pattern),
`.kairon/specs/maturity-model/planner-requirements.md`.

No template-synchronized file changes (agents, scripts, themes, evals, sentinel-protocol skill), so
`task sync:check` is unaffected. Nothing under `.kairon/specs/` other than this spec changes (see `AGENTS.md`).

## 4. Team Orchestration

- Task 1 (`guard-types-and-runner`) creates the types and runner. Task 2 (`structural-guard`) needs those types and
  runs after it. Task 3 (`reference-doc`) needs the structural guard because its docs test runs the doc's example
  through it. Task 4 (`validate-complete`, validator, read-only) runs last. The chain is serial because all three
  builder tasks share one package.
- TDD per `AGENTS.md` for tasks 1 and 2: write the failing test first. Because a test for a new API cannot compile
  without the API, first add the minimal stubs (types, a `Run` that returns the zero `Result`, a `Structural` whose
  `Admit` returns `Admitted()`), confirm each new test fails on an assertion (not a compile error), then implement.
  Commit tests and implementation together. The commit message names the test seen failing first.
- Task 3 is documentation; its small docs test is written alongside it (no test-first requirement for docs).

## 5. Step-by-Step Task Breakdown

### Task 1: `guard-types-and-runner`
Create `doc.go`, `guard.go`, `chain.go`, `guard_test.go`, `chain_test.go`, `deps_test.go`.

Test-first order:
1. `TestGuard_InterfaceShape` (AC 1): `reflect.TypeOf((*Guard)(nil)).Elem()` has exactly 2 methods, named `Admit` and
   `OnFailure`; `Admit` takes one `Input` and returns one `AdmissionResult`; `OnFailure` takes one `FailureContext`
   and returns `error`.
2. `TestInput_HasNoMutableFields` (AC 1): walk `Input` with `reflect`; every field kind is `String`, `Int`, or a
   struct recursively made of those. Fails if anyone adds a slice, map, pointer, interface or func.
3. `TestAdmission_TrivialGuard` (AC 1): a `fakeGuard` that passes or rejects on demand. Pass returns `Pass == true`
   and `Failure == nil`; reject returns `Pass == false` and `Failure != nil`. `AdmissionResult.Validate()` rejects
   `{Pass:true, Failure:&fc}` and `{Pass:false, Failure:nil}`.
4. `TestFailureContext_AllFieldsPopulated` (AC 2): `in.Reject("g", "exp", "act")` on an `Input` with stage
   `planner` and `Attempt{2,3}` yields a context with all five fields (`Stage`, `Guard`, `Attempt.Current`,
   `Attempt.Max`, `Expected`, `Actual`) set to the expected values; `Validate()` rejects each field being empty or
   invalid in turn (table).
5. `TestRun_StopsAtFirstFailure` (AC 3): three recording guards, second rejects; the third's `Admit` call count is 0,
   the first and second are 1.
6. `TestRun_AllPassReadyToCommit` (AC 4): three passing guards; `Outcome == OutcomeReadyToCommit`, `Failure == nil`,
   `err == nil`, no `OnFailure` call on any guard. The recording guards share one `events []string` log: it contains
   exactly the three `admit` entries in order, so the runner added no entries of its own (the runner has no write
   path to a GitHub client or shared store by construction; the import test backs that).
7. `TestRun_FailureRoutedToOwningGuardOnce` (AC 5): first passes, second rejects, third never runs. The second's
   `OnFailure` is called exactly once with a `FailureContext` equal to the one its `Admit` returned (all fields
   populated); the first guard's `OnFailure` count is 0 and the third's is 0 and `Result.Failure` equals the context.
8. `TestRun_FailureErrPassedThrough`: `OnFailure` returns an error; `Run` returns `err == nil` and
   `Result.FailureErr` is that error.
9. `TestRun_RejectsBadChain`: nil slice and empty slice give `ErrEmptyChain` (and `errors.Is(err, ErrInvalid)`); nil
   element gives `ErrInvalid`; invalid `Input` (unknown stage, `Attempt{0,3}`, `Attempt{4,3}`) gives `ErrInvalid`.
   No guard is invoked in any of these.
10. `TestRun_ContractViolation`: a guard returning `{Pass:false}` with nil `Failure`, one returning `Pass:true` with a
    `Failure`, and one returning a `Failure` with an empty `Expected` each yield `*ContractError`
    (`errors.Is(err, ErrInvalid)`), zero `OnFailure` calls, and no later guard run.
11. `TestNoForbiddenImports` (`deps_test.go`, section 2.1).

Dependencies: none.

### Task 2: `structural-guard`
Create `structural.go` and `structural_test.go`; add `DefaultChain` to `chain.go` with a test in `chain_test.go`.

Test-first order:
1. `TestStructural_Admit` (AC 6, table-driven). Build documents with `stageoutput.Marshal` (so the fixtures are valid
   envelopes) around a `body` string. Cases and expected outcome:
   - well-formed canonical planner doc with all five sections and every criterion carrying `Verify:` → pass, nil
     `Failure`;
   - missing `## Acceptance Criteria` section → fail, `Expected` mentions the section list, `Actual` contains
     `Acceptance Criteria`;
   - missing `## Dependencies` → fail, `Actual` contains `Dependencies` and not `Problem`;
   - two criteria, the second without `Verify:` → fail, `Actual` names criterion 2;
   - `## Acceptance Criteria` heading present but no list items → fail;
   - `Verify:` only on a continuation line of a multi-line criterion → pass;
   - heading-like text inside a fenced code block does not satisfy a required section → fail;
   - `## Problem Statement` and `## Scope — In and Out` satisfy `Problem` and `Scope` → pass;
   - bold `**Verify:**` accepted, `verify:` lower-case accepted;
   - `punch_out` document → pass.
   Every failing case asserts `Failure.Stage`, `Failure.Guard == "structural"`, `Failure.Attempt`,
   non-empty `Expected` and non-empty `Actual`.
2. `TestStructural_RejectsUnparseable`: empty string, non-YAML text, a document with an unknown key
   (`stagee:`), and `schema_version: 2`. Each fails with `Expected` mentioning the strict stage-output schema and
   `Actual` carrying the parse error text; the `schema_version: 2` case's `Actual` mentions the highest supported
   version.
3. `TestStructural_DoesNotMutate`: `Admit` called twice on the same `Input` returns equal results (pure and
   repeatable); the `Input` passed in is unchanged.
4. `TestStructural_OnFailure`: handler receives the exact `FailureContext`; its returned error is returned; nil
   handler returns `ErrNoFailureHandler`.
5. `TestStructural_CustomRequiredSections`: `RequiredSections: []string{"Problem"}` passes a body with only a
   Problem section (and no Verify check because no `Acceptance Criteria` section exists).
6. `TestStructural_InChain`: `Run(in, DefaultChain(handler))` with a valid document returns `ReadyToCommit`; with a
   document missing a section returns `Rejected` and the handler was invoked once.
7. `TestDefaultChain_Order`: returns one guard of type `*Structural`.

Dependencies: Task 1.

### Task 3: `reference-doc`
Create `docs/guards.md` and `internal/guard/docs_test.go`; add the README sentence.

The doc contains:
- Purpose and position: guards sit between stages, are deterministic Go, never an agent (§4, principle 11). Link to
  planner-requirements §4 and to `docs/stage-output.md`.
- The two-method interface with the Go signatures, what each method may and may not do, and who calls them.
- `AdmissionResult` and `FailureContext`: field table (`stage`, `guard`, `attempt` current+max, `expected`, `actual`),
  the "present iff `Pass` is false" rule, and that the context feeds the punch-out comment (§14.2).
- `Input`: why it carries raw text and the stage, and why that makes the document immutable to a guard.
- Chain semantics: the ordered algorithm from 2.3, a numbered list, plus the two result channels (`error` versus
  `Result.FailureErr`), the empty-chain rule, the contract-violation rule, and that the runner performs no side
  effect and does not commit (#378) or count attempts (#379).
- The structural reference guard: the checks, the default required sections, how headings and `Verify:` items are
  detected, what it deliberately does not check, a complete valid example document (fenced `yaml`) and an invalid one
  (fenced `yaml-invalid`) with the `Actual` text it produces.
- How to add a guard (write two methods, append to the slice; no runner change) and the hard-coded order in
  `DefaultChain` (§4.4).
- A short Go usage snippet building `Input`, calling `Run`, and switching on `Result.Outcome`.

`docs_test.go`: reads `../../docs/guards.md`; asserts it contains `Admit`, `OnFailure`, `AdmissionResult`,
`FailureContext`, `Stage`, `Guard`, `Attempt`, `Expected`, `Actual`, `ready to commit`, `ErrEmptyChain`, `Verify:`,
`structural`, `#378`, `#379`; for every fenced ```` ```yaml ```` block, runs it through `NewStructural(nil).Admit`
with a valid `Input` and requires `Pass`; for every ```` ```yaml-invalid ```` block requires `!Pass`.

Dependencies: Task 2.

### Task 4: `validate-complete`
Read-only verification of acceptance criteria 1-6, the constraints, the import rule, and the repository gates.

## 6. Acceptance Criteria Traceability

| AC | Satisfied by |
|----|--------------|
| 1 | `TestGuard_InterfaceShape`, `TestInput_HasNoMutableFields`, `TestAdmission_TrivialGuard` |
| 2 | `TestFailureContext_AllFieldsPopulated`; every failing structural case in `TestStructural_Admit` |
| 3 | `TestRun_StopsAtFirstFailure` |
| 4 | `TestRun_AllPassReadyToCommit`, `TestNoForbiddenImports` |
| 5 | `TestRun_FailureRoutedToOwningGuardOnce` |
| 6 | `TestStructural_Admit` (valid, missing section, missing `Verify:`), `TestStructural_RejectsUnparseable` |
| Constraint: admission returns only `AdmissionResult` | `TestGuard_InterfaceShape` |
| Constraint: deterministic Go, no agent-spawn dependency | `TestNoForbiddenImports` |
| Constraint: hard-coded order, runner takes a slice | `Run(in, []Guard)` signature, `DefaultChain`, `TestDefaultChain_Order` |
| Docs deliverable | Task 3, `docs_test.go` |

## 7. Risks and Decisions

- **`Input` carries raw text, not a parsed document.** This reads the issue's "admission method taking the
  stage-output document" slightly loosely. The reason is in 2.2: the schema check must be a guard, so it needs the
  unparsed text. `in.Document()` gives guards the typed form. If a reviewer prefers a parsed-document parameter, the
  structural guard would need a separate parse step ahead of the chain, and parse failures would have no guard to own
  them.
- **Method name `OnFailure`, return type `error`.** Not pinned by the issue. `error` means "enacting the decision
  failed" and leaves the retry-versus-punch-out disposition inside the guard (§4.1). #379 can capture a GitHub client
  or context in the guard struct without changing the interface. No `context.Context` parameter, per the issue's
  signature.
- **Empty chain is an error.** §4.2 does not say. A boundary that silently admits with zero guards is the worse
  failure mode, so the runner refuses.
- **Structural guard scope.** The default section list is the planner issue standard (§6): Problem, Context, Scope,
  Acceptance Criteria, Dependencies. The architect and other stages have different bodies, which is why
  `RequiredSections` is a field. `Constraints` is optional per §6, so it is not required. `punch_out` documents pass
  the structural guard.
- **`FailureContext.Attempt` is supplied, not computed.** The boundary passes `Attempt` in `Input`; counting and the
  budget belong to #379. The runner only validates `1 <= Current <= Max`.
- **No `recover()`.** A guard that panics crashes loudly rather than being mistaken for a failed admission.
- **`Verify:` detection is textual.** A regex over each list item, not a markdown parser. Edge cases (nested lists,
  fenced code) are covered by the table test; full CommonMark fidelity is not a goal.
- **Not using an interface for the runner.** `Run` is a function over a slice; there is only one runner, and the
  seam §4.4 wants is the `Guard` interface plus the slice.

## 8. Validation Commands

```bash
go build ./...
go vet ./...
go test -race -count=1 ./internal/guard/...
go test ./...
task lint
task fmt:check
task test
task sync:check
test -f docs/guards.md
grep -n 'guards.md' README.md
test -z "$(go list -f '{{join .Imports "\n"}}' ./internal/guard | grep -E '^(os|os/exec|net|net/http)$')"
test "$(go list -deps ./internal/guard | grep -c 'jbrinkman/kairon/internal')" -eq 2
go run ./cmd/kairon plan parse .kairon/specs/issue-377-guard-interface-and-chain-runner.md
```

## 9. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "guard-types-and-runner"
    agent: "builder"
    description: "Create package internal/guard with the two-method Guard interface (Admit(Input) AdmissionResult, OnFailure(FailureContext) error), Input (stage, raw text, attempt; no pointer/slice/map fields), Attempt, AdmissionResult (Pass plus Failure present iff Pass is false), FailureContext (stage, guard, attempt current+max, expected, actual), the Admitted and Input.Reject helpers, Validate methods, ErrInvalid/ErrEmptyChain/ErrNoFailureHandler/ContractError, and the stateless chain runner Run(in, []Guard) (Result, error) with Outcome ReadyToCommit/Rejected. Write the tests first (guard_test.go, chain_test.go, deps_test.go) following TDD per AGENTS.md: add minimal stubs so tests compile, confirm each new test fails on an assertion, then implement. Runner semantics: ordered, one Admit per guard, stop at first failure without calling later guards, call only the failing guard's OnFailure exactly once with the FailureContext from its AdmissionResult, validate admission results (contract violations return ContractError and call no OnFailure), reject empty chain and nil guards, perform no side effect on an all-pass chain."
    dependencies: []
    acceptance_criteria:
      - "internal/guard imports only the standard library and internal/stageoutput: TestNoForbiddenImports proves no direct import of os, os/exec, net or net/http and that the only jbrinkman/kairon/internal packages in go list -deps are guard and stageoutput (so no internal/agent, internal/watcher or internal/github)"
      - "TestGuard_InterfaceShape proves the Guard interface has exactly two methods (Admit taking one Input and returning one AdmissionResult; OnFailure taking one FailureContext and returning error), and TestInput_HasNoMutableFields proves Input contains no pointer, slice, map, interface or func fields"
      - "TestAdmission_TrivialGuard shows a trivial guard returning AdmissionResult{Pass:true} with nil Failure on pass and Pass:false with a non-nil FailureContext on fail, and AdmissionResult.Validate rejects Pass:true with a Failure and Pass:false without one"
      - "TestFailureContext_AllFieldsPopulated asserts a failing admission built with Input.Reject has stage, guard, attempt current, attempt max, expected and actual all populated, and FailureContext.Validate rejects each being empty or invalid"
      - "TestRun_StopsAtFirstFailure: with three guards where the second rejects, the third's Admit is never invoked"
      - "TestRun_AllPassReadyToCommit: with all-passing guards Run returns OutcomeReadyToCommit, a nil error, a nil Failure, no OnFailure call on any guard, and the shared event log contains only the guards' own admit entries"
      - "TestRun_FailureRoutedToOwningGuardOnce: the failing guard's OnFailure is called exactly once with a populated FailureContext equal to the one its Admit returned, no passing guard's OnFailure is called, and Result.Failure equals that context"
      - "Run returns ErrEmptyChain for a nil or empty slice, ErrInvalid for a nil element or an invalid Input (unknown stage, Attempt{0,3}, Attempt{4,3}), and a *ContractError for a broken AdmissionResult, in every case without calling OnFailure; an OnFailure error is reported in Result.FailureErr with a nil returned error"
      - "Commit message names the test observed failing first"
    validation_commands:
      - "go build ./internal/guard/..."
      - "go vet ./internal/guard/..."
      - "go test -race -count=1 ./internal/guard/..."
      - "test -z \"$(gofmt -l internal/guard)\""

  - id: "structural-guard"
    agent: "builder"
    description: "Add the structural reference guard to internal/guard (structural.go, structural_test.go) and DefaultChain in chain.go. Structural.Admit parses Input.Raw with stageoutput.Parse (parse failure is an admission failure with Expected 'document parses under the strict stage-output schema (stageoutput.Parse)' and Actual the parse error), admits punch_out documents, and for canonical documents checks that required markdown sections are present (default Problem, Context, Scope, Acceptance Criteria, Dependencies; headings match by case-insensitive prefix at a word boundary; headings inside fenced code blocks are ignored) and that every list item under the Acceptance Criteria section contains a Verify: marker (regex (?i)\\bverify(\\*\\*|__)?:), populating FailureContext Expected and Actual with the missing sections or the criterion numbers lacking Verify:. OnFailure calls an injected FailureHandler and returns ErrNoFailureHandler when none is set. DefaultChain(handler) returns the hard-coded ordered slice. Tests first per AGENTS.md TDD (stubs, confirm assertion failure, implement): table-driven TestStructural_Admit, unparseable inputs, purity, OnFailure, custom RequiredSections, running the guard through Run and DefaultChain."
    dependencies: ["guard-types-and-runner"]
    acceptance_criteria:
      - "TestStructural_Admit (table-driven) passes: a well-formed canonical document passes with a nil Failure; a document missing a required section and a document with a criterion lacking a Verify: line each fail"
      - "Every failing structural case has Failure.Stage, Failure.Guard equal to 'structural', Failure.Attempt, a non-empty Expected and a non-empty Actual; the missing-section Actual names the missing section(s) and the missing-Verify Actual names the criterion number"
      - "Unparseable input (empty string, non-YAML text, unknown key, schema_version 2) fails admission with Expected mentioning the strict stage-output schema and Actual carrying the parse error, rather than returning a Go error or panicking"
      - "Headings inside fenced code blocks do not count, '## Problem Statement' satisfies Problem, '## Scope' variants satisfy Scope, a Verify: on a continuation line of a multi-line criterion counts, a bold or lower-case Verify marker counts, an Acceptance Criteria section with no list items fails, and a punch_out document passes"
      - "Admit is pure and repeatable (same Input gives equal results) and the Input is unchanged afterwards"
      - "OnFailure passes the exact FailureContext to the injected handler and returns its error, and returns ErrNoFailureHandler for a nil handler"
      - "Run(in, DefaultChain(handler)) returns OutcomeReadyToCommit for a valid document and OutcomeRejected with the handler invoked once for an invalid one; DefaultChain returns one *Structural guard"
      - "Commit message names the test observed failing first"
    validation_commands:
      - "go build ./internal/guard/..."
      - "go vet ./internal/guard/..."
      - "go test -race -count=1 ./internal/guard/..."
      - "test -z \"$(gofmt -l internal/guard)\""

  - id: "reference-doc"
    agent: "builder"
    description: "Write docs/guards.md: purpose and position (guards between stages, deterministic Go, never an agent, link to planner-requirements section 4 and docs/stage-output.md), the two-method interface with signatures and what each method may and may not do, AdmissionResult and FailureContext field table and the present-iff-fail rule, Input and why it carries raw text, chain semantics as a numbered algorithm including the error versus Result.FailureErr channels, empty-chain and contract-violation rules, that the runner performs no side effect and leaves commit to #378 and retry-then-punch-out behavior to #379, the structural reference guard (checks, default required sections, detection rules, what it does not check) with a complete valid ```yaml example and an invalid ```yaml-invalid example, how to add a guard, the hard-coded DefaultChain order, and a short Go usage snippet. Add internal/guard/docs_test.go asserting the required terms are present, every ```yaml block is admitted by NewStructural(nil).Admit and every ```yaml-invalid block is rejected. Add one sentence linking docs/guards.md from README.md next to the stage-output link."
    dependencies: ["structural-guard"]
    acceptance_criteria:
      - "docs/guards.md exists and names Admit, OnFailure, AdmissionResult, FailureContext (stage, guard, attempt current and max, expected, actual), the present-iff-pass-is-false rule, the ordered chain algorithm, 'ready to commit', ErrEmptyChain, the structural guard and its Verify: check, and references #378 and #379 as out of scope"
      - "The doc states that admission neither mutates the document nor writes to GitHub nor takes failure action, and that the runner performs no side effect"
      - "docs_test.go passes: required terms present, every ```yaml example is admitted by the structural guard and every ```yaml-invalid example is rejected"
      - "README.md contains a link to docs/guards.md"
    validation_commands:
      - "test -f docs/guards.md"
      - "go test -race -count=1 ./internal/guard/..."
      - "grep -n 'guards.md' README.md"
      - "grep -n 'OnFailure' docs/guards.md"

  - id: "validate-complete"
    agent: "validator"
    description: "Verify issue #377 acceptance criteria 1-6 and constraints against the implementation: two-method Guard interface and AdmissionResult/FailureContext semantics, ordered runner behavior (stop at first failure, ready-to-commit signal with no side effect, failure routed once to the failing guard only), structural reference guard behavior, docs/guards.md, and that internal/guard imports only the standard library and internal/stageoutput. Confirm provenance/commit (#378), retry-then-punch-out behavior (#379), other guards, configurable chains and watcher wiring were not implemented, no file under .kairon/specs other than this spec changed, and no template-synchronized file changed. Run the repository gates."
    dependencies: ["reference-doc"]
    acceptance_criteria:
      - "Each of issue #377 acceptance criteria 1-6 is demonstrated by a named passing test (see the traceability table in the spec)"
      - "go build ./..., task lint, task fmt:check, task test and task sync:check all pass"
      - "go list -deps ./internal/guard shows exactly two jbrinkman/kairon/internal packages (guard and stageoutput) and go list -f '{{join .Imports \"\\n\"}}' ./internal/guard shows no os, os/exec, net or net/http import"
      - "git diff against the base shows changes only under internal/guard/, docs/guards.md, README.md and this spec"
    validation_commands:
      - "go build ./..."
      - "task lint"
      - "task fmt:check"
      - "task test"
      - "task sync:check"
      - "go test -race -count=1 ./internal/guard/..."
      - "test \"$(go list -deps ./internal/guard | grep -c 'jbrinkman/kairon/internal')\" -eq 2"
```
