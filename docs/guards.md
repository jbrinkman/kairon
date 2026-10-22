# Guards

A guard is a deterministic check that runs between two workflow stages. It looks at the
[stage-output document](stage-output.md) one stage returned and decides whether the next step may proceed. The
guard interface, the ordered chain runner and one reference guard live in
[`internal/guard`](../internal/guard).

Guards are plain Go. A guard never spawns an agent, never calls a model and never reads from or writes to GitHub.
The package imports only the standard library and `internal/stageoutput`, and `TestNoForbiddenImports` fails the
build if it directly imports `os`, `os/exec`, `net` or `net/http`, or depends on any other internal package. The
design background is in `.kairon/specs/maturity-model/planner-requirements.md` (§4, §4.1, §4.2, §4.4 and §14).

This package defines the guard contract and the runner. Two related pieces are out of scope here:

- **#378** adds the commit step that runs after every guard has passed (the single GitHub write and the provenance
  hash). It is described in [provenance.md](provenance.md).
- **#379** defines what a guard's failure method actually does: the bounded retry, the punch-out, and the attempt
  counter. It is described in [failure-contract.md](failure-contract.md).

Nothing in the watcher or any stage calls the runner yet.

## The Guard interface

Every guard has the same two methods. A boundary can compose guards without knowing what each one checks.

```go
type Guard interface {
	// Admit judges the stage output.
	Admit(in Input) AdmissionResult
	// OnFailure receives the FailureContext that Admit produced and enacts
	// this guard's retry-versus-punch-out decision.
	OnFailure(fc FailureContext) error
}
```

| Method | May | May not |
|--------|-----|---------|
| `Admit` | Read `Input`. Parse `Input.Raw` through `in.Document()`. Return `Admitted()` or `in.Reject(...)`. | Mutate the document. Write to GitHub or any shared state. Take failure action. Return anything other than an `AdmissionResult`. |
| `OnFailure` | Act on the `FailureContext`: retry the upstream stage or punch out. Return an error if that action failed. | Rely on being called when `Admit` passed. The runner calls it only on the guard that rejected, and at most once per run. |

`Admit` is a pure pass/fail judgement, so it is cheap to test and safe to call repeatedly. `OnFailure` carries the one
guard-specific policy decision, because only the guard knows whether its own check is recoverable by a retry. The
runner supplies the context. It does not decide how the failure is handled.

The interface has no `Name()` method. A guard puts its own name in `FailureContext.Guard`.

## AdmissionResult and FailureContext

```go
type AdmissionResult struct {
	Pass    bool
	Failure *FailureContext // non-nil if and only if Pass is false
}

type FailureContext struct {
	Stage    stageoutput.Stage
	Guard    string
	Attempt  Attempt // Current and Max
	Expected string
	Actual   string
}
```

| Field | Meaning |
|-------|---------|
| `Stage` | The stage that produced the rejected output. |
| `Guard` | The name of the guard whose `Admit` failed, for example `structural`. |
| `Attempt` | The current attempt number and the maximum for this boundary (`Attempt.Current`, `Attempt.Max`). |
| `Expected` | What `Admit` required. |
| `Actual` | What `Admit` observed: the failing field, the missing sections, or the parse error. |

All five are required. `FailureContext.Validate()` rejects an unknown stage, an empty `Guard`, an empty `Expected` or
`Actual`, and an attempt that is not `1 <= Current <= Max`.

**The present-iff-fail rule.** `Failure` is non-nil if and only if `Pass` is false. A pass carries no failure
context, and a fail always carries one. `AdmissionResult.Validate()` enforces the rule, and the runner calls it on
every result.

The `FailureContext` goes only to the failing guard's `OnFailure`. It holds the same information a punch-out comment
needs (planner-requirements §14.2), so that comment can be built from the typed value instead of a bare boolean.

Two helpers keep guards short:

- `Admitted()` returns `AdmissionResult{Pass: true}`.
- `in.Reject(guard, expected, actual)` returns a failing result and fills `Stage` and `Attempt` from `in`, so a guard
  cannot forget them.

## Input

```go
type Input struct {
	Stage   stageoutput.Stage // the stage whose output is being admitted
	Raw     string            // the exact text the stage returned (a stage-output document, YAML)
	Attempt Attempt           // supplied by the boundary; this package never counts
}
```

`Input` carries the raw text, not a parsed `*stageoutput.Document`. The reason is that "the document does not parse
under the strict schema" must be an ordinary admission failure with a `FailureContext`, owned by a guard and routed to
that guard's `OnFailure`. If the runner parsed first, a malformed document would fail before any guard existed to own
the failure. A guard that needs the typed form calls `in.Document()`, which runs `stageoutput.Parse` on every call. It
is not memoized, so no state is shared between guards.

`Stage` is supplied by the boundary rather than read from the document, so it is known even when `Raw` does not
parse.

`Input` is a value type with no pointer, slice or map fields. `Raw` is a `string`, not a `[]byte`. A guard therefore
cannot change the document or anything the runner holds. `TestInput_HasNoMutableFields` fails if someone adds such a
field.

`Input.Validate()` requires a known `Stage` and a valid `Attempt`.

## The chain runner

```go
func Run(in Input, guards []Guard) (Result, error)

type Result struct {
	Outcome    Outcome
	Failure    *FailureContext // set when Outcome is OutcomeRejected
	FailureErr error           // what the failing guard's OnFailure returned
}
```

`Outcome` is one of:

| Outcome | Meaning |
|---------|---------|
| `OutcomeReadyToCommit` | Every guard passed. The output is ready to commit. |
| `OutcomeRejected` | A guard failed and its `OnFailure` was called. |
| `OutcomeInvalid` | The zero value. `Run` never returns it with a nil error. |

### Algorithm

1. Check the arguments. An empty or nil `guards` slice returns `ErrEmptyChain`. An invalid `Input` returns an error
   matching `ErrInvalid`. A nil element (including an interface holding a nil pointer) returns an error matching
   `ErrInvalid` that names its index. No guard has run at this point.
2. Take the guards in slice order. For each one, call `Admit(in)` once.
3. Validate the `AdmissionResult` with `Validate()`. If `Pass` and `Failure` disagree, or the `FailureContext` is
   incomplete, return a `*ContractError` naming the guard index and the problems. The runner does not call `OnFailure`
   (there is no valid context to give it) and runs no later guard.
4. If the guard passed, go to the next guard.
5. If the guard failed, call that same guard's `OnFailure` exactly once with a copy of its `FailureContext`. Do not
   call `Admit` or `OnFailure` on any other guard. Return `Result{Outcome: OutcomeRejected, Failure: &fc,
   FailureErr: <what OnFailure returned>}` and a nil error.
6. If every guard passed, return `Result{Outcome: OutcomeReadyToCommit}` and a nil error.

### Two error channels

| Channel | Means | `OnFailure` called? |
|---------|-------|---------------------|
| Returned `error` | The chain could not run (`ErrEmptyChain`, a nil guard, an invalid `Input`) or a guard broke the contract (`*ContractError`). | No |
| `Result.FailureErr` | The chain ran, a guard rejected the output, and that guard's `OnFailure` returned an error. | Yes, once |

Both `ErrEmptyChain` and `*ContractError` match `ErrInvalid` under `errors.Is`. A non-nil `FailureErr` is reported as
is. The runner does not interpret it. What a caller does with it is part of #379.

An empty chain is an error because a boundary with no guards would admit everything without anyone noticing.

### What the runner does not do

The runner performs no side effect. It reads `in`, calls the guards and returns a value. It does not commit, write to
GitHub, retry, count attempts or recover from a guard that panics. When the result is `OutcomeReadyToCommit`, the
caller commits (#378, see [provenance.md](provenance.md)). What a failing guard's `OnFailure` does, including the retry-then-punch-out behavior and the
attempt counter, belongs to #379.

## The structural reference guard

`Structural` is the one guard in this package. It is a worked example of the interface and a useful check on its own.
Its name in `FailureContext.Guard` is `structural` (the constant `StructuralName`).

```go
func NewStructural(handler FailureHandler) *Structural

type FailureHandler func(FailureContext) error
```

`OnFailure` passes the `FailureContext` to `handler` and returns the handler's error. With a nil handler it returns
`ErrNoFailureHandler`, so a failure is never silently dropped.

### Checks

`Admit` runs these checks in order and stops at the first one that fails, so `Expected` and `Actual` describe a single
problem.

1. **Schema.** `Input.Raw` must parse under the strict stage-output schema (`stageoutput.Parse`). A parse failure is an
   admission failure, not a Go error. `Expected` is `document parses under the strict stage-output schema
   (stageoutput.Parse)` and `Actual` is the parse error text. This covers empty input, non-YAML text, an unknown key
   and an unsupported `schema_version`.
2. **Punch-out.** A `punch_out` document has no body to inspect, and the schema check already showed it is
   well-formed, so it is admitted. What a punch-out triggers is decided elsewhere.
3. **Required sections** (`canonical` documents only). Every required section must appear as a markdown heading in
   `body`. On failure, `Expected` lists all required sections and `Actual` is `missing sections: ` followed by the
   missing ones, in required order.
4. **`Verify:` lines.** If the body has an `Acceptance Criteria` section, it must contain at least one list item and
   every list item must contain a `Verify:` marker. On failure, `Expected` is `every acceptance criterion carries a
   Verify: line` and `Actual` is either `criteria without Verify: 2, 4 (of 5)` or `Acceptance Criteria section has
   no list items`.

### Default required sections

`DefaultRequiredSections` is `Problem`, `Context`, `Scope`, `Acceptance Criteria` and `Dependencies`. That is the
planner issue standard. Other stages have different bodies, so the list is the `RequiredSections` field on
`Structural`. A nil slice means the defaults. If `Acceptance Criteria` is not in the list and not in the body, the
`Verify:` check is skipped.

### Detection rules

- **Headings.** A heading is a line of one to six `#` characters, then whitespace, then text, with up to three spaces
  of indent. Lines inside a fenced code block (backticks or tildes) are never headings.
- **Matching a section name.** The heading text is lower-cased and stripped of leading and trailing punctuation
  (such as `:` or `*`). It matches a required name when it equals the name or starts with the name followed by a
  character that is not a letter or digit. `## Problem Statement` satisfies `Problem`, and `## Scope — In and Out`
  satisfies `Scope`. `## Problems` does not.
- **Section extent.** The `Acceptance Criteria` section runs from its heading to the next heading of the same or a
  higher level, or to the end of the body.
- **List items.** An item starts at a list marker (`-`, `*`, `+`, `1.` or `1)`) whose indent is no deeper than the
  first item's. Deeper-indented markers and plain lines are continuation text of the current item. Lines inside a
  fenced code block never start an item.
- **`Verify:` marker.** The item's text, including continuation lines, matches `(?i)\bverify(\*\*|__)?:`. So
  `Verify:`, `verify:` and `**Verify:**` all count.

### What it does not check

The structural guard is a reference, not a complete issue-standard check. It does not:

- require any text after `Verify:`,
- check that `Scope` or an out-of-scope list has an entry,
- check title length or document size,
- check the single-line `Dependencies:` form,
- look for secrets, injected instructions or whether the content is correct.

Each of those is a separate guard to write when needed.

### Example: admitted

This canonical planner document has all five sections, and every criterion has a `Verify:` line.

```yaml
schema_version: 1
stage: planner
result: canonical
source:
  kind: github_issue
  ref: "142"
generated: "2026-10-20T20:00:00Z"
body: |
  ## Problem

  The watcher retries a failing issue without a limit.

  ## Context

  The retry count is stored in .kairon/retries/issue-<number>.count.

  ## Scope

  Cap the attempts per issue. Out of scope: changing the backoff delay.

  ## Acceptance Criteria

  - An issue stops being retried after max_retries attempts.
    Verify: go test ./internal/watcher -run TestRetryCap
  - The cap is read from config.yaml.
    Verify: grep -n max_retries .kairon/config.yaml

  ## Dependencies

  Dependencies: #314
```

### Example: rejected

This document parses, but the second criterion has no `Verify:` line. The structural guard rejects it with
`Expected` set to `every acceptance criterion carries a Verify: line` and `Actual` set to `criteria without
Verify: 2 (of 2)`. It uses the `yaml-invalid` fence so the documentation test treats it as a document that must be
rejected.

```yaml-invalid
schema_version: 1
stage: planner
result: canonical
source:
  kind: github_issue
  ref: "142"
generated: "2026-10-20T20:00:00Z"
body: |
  ## Problem

  The watcher retries a failing issue without a limit.

  ## Context

  The retry count is stored in .kairon/retries/issue-<number>.count.

  ## Scope

  Cap the attempts per issue.

  ## Acceptance Criteria

  - An issue stops being retried after max_retries attempts.
    Verify: go test ./internal/watcher -run TestRetryCap
  - The cap is read from config.yaml.

  ## Dependencies

  Dependencies: #314
```

## Adding a guard

1. Write a type with the two methods. `Admit` returns `Admitted()` or `in.Reject(name, expected, actual)`. It does no
   I/O and changes nothing. `OnFailure` carries out the guard's retry-or-punch-out decision.
2. Add it to the slice that `DefaultChain` returns.

The runner and the other guards do not change.

Keep the package's import rule. A guard that needs a file, a process or the network does not belong in
`internal/guard`, because the import test would fail.

## Chain order

The order is hard-coded in `DefaultChain`, the single place it is written:

```go
func DefaultChain(handler FailureHandler) []Guard {
	return []Guard{NewStructural(handler)}
}
```

`Run` takes a plain slice, so a configurable chain can replace `DefaultChain` later without touching the runner. The
chain is not configurable today.

## Go usage

```go
in := guard.Input{
	Stage:   stageoutput.StagePlanner,
	Raw:     raw, // the text the planner returned
	Attempt: guard.Attempt{Current: 1, Max: 3},
}

res, err := guard.Run(in, guard.DefaultChain(handler))
if err != nil {
	// The chain could not run, or a guard broke the contract.
	// errors.Is(err, guard.ErrEmptyChain) and errors.Is(err, guard.ErrInvalid) tell the cases apart.
	return err
}

switch res.Outcome {
case guard.OutcomeReadyToCommit:
	// Every guard passed. The caller commits (#378).
case guard.OutcomeRejected:
	// res.Failure is what the failing guard received in OnFailure.
	// res.FailureErr is the error its OnFailure returned, if any (#379).
}
```
