# Design Spec: Provenance commit step (canonicalize, hash, classify, write once)

Closes #378

Design reference: `.kairon/specs/maturity-model/planner-requirements.md` §4.3, §7, §8, §8.3, §8.4, §8.5.
Depends on #376 (closed, `internal/stageoutput`) and #377 (closed, `internal/guard`, `docs/guards.md`).
Supersedes the reverted `internal/issueprov` package (PR #372). The hash idea is the same. The difference is that the
deterministic layer computes and writes it, never the agent.

## 1. Problem and Goal

After a guard chain returns `guard.OutcomeReadyToCommit`, the boundary must perform exactly one authoritative GitHub
write: the validated body plus a `v1` provenance marker. The pipeline must also classify any issue body on read as
valid, none or mismatch, using the same canonicalization code as the writer.

Delivered in one PR:

- New package `internal/provenance` (canonicalize, hash, marker, classify, commit step, chain commit with dependency
  substitution).
- `docs/provenance.md` plus a doc-drift test, and links from `README.md` and `docs/guards.md`.

Out of scope (do not implement): the guard interface and runner (#377, already merged); the planner emitting the output
document (#381); the watcher reading or verifying the hash (#382); retry and punch-out behaviour (#379); any HMAC,
signing key or secret; a `gh`-backed implementation of the store (see 2.7).

## 2. Solution Approach

### 2.1 Package and dependency rule

New package **`internal/provenance`** (`github.com/jbrinkman/kairon/internal/provenance`).

- Non-test code imports only the standard library, `internal/guard` and `internal/stageoutput`.
- It must not import `os`, `os/exec`, `net`, `net/http`, `crypto/hmac`, `internal/github`, `internal/watcher`,
  `internal/agent` or any other internal package. All GitHub I/O goes through the `IssueStore` interface (2.5), so the
  package is pure and every behaviour is testable with a fake.
- `TestNoForbiddenImports` enforces this with the same technique as `internal/guard/deps_test.go`
  (`go list -f '{{join .Imports "\n"}}' .` for direct imports, `go list -deps .` for internal packages; allowed internal
  packages are `provenance`, `guard`, `stageoutput`). Forbidding `os` and `crypto/hmac` is also what proves the
  "no key material" constraint: the hash code cannot read an environment variable, a file or a key.
- Tests that need `internal/watcher` (AC 7) live in the **external** test package `provenance_test`. #382 will make
  `internal/watcher` import `internal/provenance`, and an in-package test importing `watcher` would then be an import
  cycle. `go list .Imports` ignores test files, so the dependency rule stays enforceable.

### 2.2 Canonicalization (one function, `canonical.go`)

```go
func Canonicalize(body string) string
```

Steps, in order:

1. Normalize line endings: `\r\n` and lone `\r` become `\n`. (GitHub's web editor submits CRLF.)
2. Split on `\n`. Drop every line whose `strings.TrimSpace` starts with `<!-- planner-provenance:` (the marker prefix,
   any version, well-formed or not). Dropping all such lines, not just the last one, makes re-marking idempotent and
   removes injected extra markers from the hash input.
3. Strip trailing whitespace from each remaining line (`strings.TrimRight(line, " \t\f\v")`).
4. Drop trailing empty lines at the end of the text, then join with `\n`. The result has no trailing newline.

Step 4 is one rule beyond the three listed in the issue and §8.3. It is required for the round trip: the writer emits
`canonical + "\n" + marker`, so removing the marker line leaves `canonical + "\n"`. Without step 4 the writer's own output
would not canonicalize back to what was hashed. It also makes the hash immune to a trailing newline that GitHub or an
editor adds or removes. Document it in `docs/provenance.md`.

Consequence to document: markdown hard line breaks written as two trailing spaces are removed by step 3. The committed
body is the canonical form (see 2.3), so the stripping happens at write time, not just at hash time.

`Canonicalize` is idempotent: `Canonicalize(Canonicalize(x)) == Canonicalize(x)`.

### 2.3 Hash, marker and writer (`marker.go`)

```go
const (
    MarkerPrefix = "<!-- planner-provenance:"
    VersionV1    = "v1"
)

func Hash(body string) string                              // lower-case hex SHA-256 of Canonicalize(body)
func Marker(hash string, generated time.Time) string       // "<!-- planner-provenance: v1 sha256=<hex> generated=<RFC3339 UTC> -->"
func Mark(body string, generated time.Time) string         // Canonicalize(body) + "\n" + Marker(Hash(body), generated)
```

- `generated` is RFC 3339 in UTC (`2006-01-02T15:04:05Z`), which is an ISO-8601 profile. It is the commit time, taken
  from the committer's injected clock. It is not part of the hash input, since the marker line is excluded.
- `Mark` of an already-marked body yields the same hash (AC 4), because step 2 of canonicalization removes the old
  marker before hashing. If the canonical body is empty, `Mark` returns the marker alone.
- The committed body is `Canonicalize(body) + "\n" + marker`, with no trailing newline: the marker is the last line.
  Invariant: `Canonicalize(Mark(b, t)) == Canonicalize(b)`.

### 2.4 Classifier (`classify.go`)

```go
type State int // StateInvalid (zero, never returned), StateValid, StateNone, StateMismatch; String() = "valid"|"none"|"mismatch"
func Classify(body string) State
```

Algorithm (uses `Canonicalize` and `Hash`; there is no second normalization path):

1. Normalize line endings, split into lines.
2. Candidate lines = lines whose `TrimSpace` has the prefix `MarkerPrefix`. No candidates: `StateNone`.
3. Otherwise `StateValid` only if **all** hold; anything else is `StateMismatch`:
   - exactly one candidate;
   - it is the last non-empty line of the body;
   - its trimmed text matches `^<!-- planner-provenance: v1 sha256=([0-9a-f]{64}) generated=(\S+) -->$` and
     `generated` parses as RFC 3339;
   - the captured hex equals `Hash(body)`.

A marker that is present but malformed, has an unknown version (for example `v2`), is duplicated, is not last, or
carries the wrong hash is `StateMismatch`, never `StateValid` or `StateNone` (AC 5). A mismatch routes to the planner
exactly like `none` (§8.4), so being strict here costs a re-plan, never a privilege.

### 2.5 Commit step (`commit.go`)

The commit step is **not a guard** (§4.3). It is a method the boundary calls after `guard.Run` reports
`OutcomeReadyToCommit`.

```go
type IssueStore interface {
    // Revision returns the issue's current revision token (GitHub updatedAt or an equivalent opaque token).
    Revision(ctx context.Context, number int) (string, error)
    // Create makes a new issue and returns its number. This is a GitHub write.
    Create(ctx context.Context, title, body string) (int, error)
    // Update replaces an existing issue's body. This is a GitHub write.
    Update(ctx context.Context, number int, body string) error
}

type Conflict struct{ Number int; Observed, Current string }

// Router receives an issue whose revision changed between planner read and commit.
type Router interface {
    RouteToIntake(ctx context.Context, c Conflict) error // preferred: re-plan the human's edit (§9)
    PunchOut(ctx context.Context, c Conflict) error      // used only if routing to intake fails (§14)
}

// Target says where the body goes. Number 0 means create a new issue; otherwise update Number, and
// ObservedRevision (recorded when the planner read the issue) is required.
type Target struct{ Number int; ObservedRevision string }

type Request struct {
    Doc       *stageoutput.Document // must validate, Result == canonical, non-empty Body
    Title     string                // required when Target.Number == 0
    Target    Target
    Admission guard.Result          // the result from guard.Run
}

type Outcome int // OutcomeInvalid (zero), OutcomeCommitted, OutcomeRoutedToIntake, OutcomePunchedOut
type Result struct{ Outcome Outcome; Number int; Hash string; Body string }

type Committer struct{ Store IssueStore; Router Router; Now func() time.Time } // nil Now => time.Now
func (c *Committer) Commit(ctx context.Context, req Request) (Result, error)
```

`Commit` order of operations:

1. **Gate.** If `req.Admission.Outcome != guard.OutcomeReadyToCommit` (including the zero `OutcomeInvalid` and
   `OutcomeRejected`) or `req.Admission.Failure != nil`: return `ErrNotReady`. No store method is called, not even
   `Revision` (AC 6).
2. **Validate request.** `Doc` non-nil, `Doc.Validate()` passes, `Result == canonical`, `Body` non-empty (a
   `punch_out` document has nothing to commit); `Title` non-empty for a create; `ObservedRevision` non-empty for an
   update. Violations return an error matching `ErrInvalid`, with no store call.
3. **Compute provenance.** `body := Mark(req.Doc.Body, now())`. This is the only place the marker is produced.
4. **Create path** (`Target.Number == 0`): one `Store.Create(title, body)`. No concurrency check, since the issue does not
   exist yet. Return `OutcomeCommitted` with the new number and hash.
5. **Update path.** `cur := Store.Revision(number)`. If the call fails, return an error matching
   `ErrRevisionUnavailable` and write nothing (the commit cannot prove the issue is unchanged). If
   `cur != ObservedRevision`: **do not write**. Call `Router.RouteToIntake`; if that returns an error, call
   `Router.PunchOut`; if that also fails, return an error matching `ErrUnroutable` (joined errors). Return
   `OutcomeRoutedToIntake` or `OutcomePunchedOut` with a nil error. If equal: one `Store.Update(number, body)`; return
   `OutcomeCommitted`.
6. A failing `Create`/`Update` returns the wrapped error, `OutcomeInvalid`, after exactly one write attempt. `Commit`
   never retries; bounded retry belongs to #379.

"Advance" (spawning the next stage) is the caller's reaction to `OutcomeCommitted`. `Result` is the signal; this package
spawns nothing.

Residual risk to state in the docs: `gh` and the issues REST API have no conditional write (no `If-Match`), so the
re-check narrows the window between check and write to a request round trip but cannot remove it, and `updatedAt` has
one-second granularity. A human edit that slips into that remaining window is still caught on read, because the stored
hash then fails to match and `Classify` returns `mismatch`, which re-routes the issue (§8.4).

### 2.6 Dependency substitution and chain commit (`substitute.go`, `chain.go`)

Section 7 says the number of an earlier issue is substituted into a later issue's `Dependencies:` line **before** the later
issue enters its guard chain and is hashed. Placeholder convention, defined here because #381 has not yet:

- In a proposed chain, a body refers to an earlier proposed issue as `#{key}` inside its dependencies line, for example
  `Dependencies: #{schema}, #314`. `key` matches `[A-Za-z0-9][A-Za-z0-9_-]*`.
- `#{...}` anywhere else in the body (for example Ruby interpolation in a code sample) is never touched.

```go
func SubstituteDependencies(body string, numbers map[string]int) (string, error)
```

For each line containing (case-insensitive) `dependencies:`: take the text after the first occurrence, split on `,`, trim
each part, and require each to be `#<digits>` or `#{key}`. Replace each `#{key}` with `#<numbers[key]>`, remove duplicate
numbers (keep first), and rewrite the line as `<original prefix and keyword> #N, #M`. Errors:

- `ErrUnknownPlaceholder`: a `#{key}` with no entry in `numbers` (an unknown key or a forward reference).
- `ErrBadDependencyLine`: a part that is neither `#digits` nor `#{key}` on a line that contains a placeholder. A line
  with no placeholder is returned unchanged, so existing real numbers and unrelated text are preserved.

The output is the single-line form that `DependencyParser.ParseDependencies` parses (`dependenciesRe =
(?i)dependencies:\s*(.+?)(?:\n|$)`). A `## Dependencies` header with bullets is never produced.

```go
type ChainItem struct{ Key string; Title string; Doc stageoutput.Document; Target Target }
type Admit func(raw string) (guard.Result, error) // the boundary's wrapper around guard.Run
type ChainResult struct {
    Committed []Committed       // in order; Committed{Key string; Result Result}
    StoppedAt int               // index of the item that did not commit, or -1
    Stopped   *StopReason       // nil when every item committed; carries guard.Result and/or Result for the stopping item
}
func (c *Committer) CommitChain(ctx context.Context, items []ChainItem, admit Admit) (ChainResult, error)
```

For each item, in order:

1. `body' := SubstituteDependencies(item.Doc.Body, numbersSoFar)`; set it on a copy of the document.
2. `raw := stageoutput.Marshal(copy)`; `res := admit(raw)`. This is the guard chain, run on the **already substituted**
   document.
3. If `res.Outcome != OutcomeReadyToCommit`, stop (the guard's `OnFailure` already acted). Later items are not attempted
   because they may depend on this one.
4. `Commit(...)` with the substituted document. This hashes the substituted body. On `OutcomeCommitted`, record
   `numbersSoFar[item.Key] = result.Number`. On a conflict outcome, stop.
5. Return the issues committed so far in `ChainResult` so the caller can resume. GitHub has no multi-write transaction,
   so already-created issues are not rolled back. They are complete, valid standalone issues. Resuming a partial chain is
   #381's concern; state this in the docs.

Substitution never happens after hashing. The test for AC 7 proves it by checking the stored hash validates against the
committed (substituted) body.

### 2.7 No `gh` adapter in this PR (assumption)

The issue lists the commit step, canonicalization, classifier, substitution and docs as in scope, and lists the planner
output (#381) and the watcher read side (#382) as out of scope. A concrete `IssueStore` built on `gh` has no consumer
until #381, so it is left to that issue. This keeps `internal/provenance` free of `os/exec` and fully unit-testable.
Call this out in the PR description. If the maintainer wants the adapter in this PR, it is a small addition to
`internal/github` (`gh issue view --json updatedAt`, `gh issue create`, `gh issue edit --body-file`) and should not
change the interface above.

### 2.8 Template sync

No template-synchronized path changes (`internal/`, `docs/` and `README.md` are not mirrored into
`cmd/kairon/templates/`), so there is no sync work. Note for builders: the worktree already has uncommitted edits to
`.kiro/agents/*.json` injected by the environment. They are not part of this task. Do not stage, commit or revert them.
`task sync:check` already fails at baseline because of those edits, so it is deliberately not a validation command here.

## 3. Relevant Files

| File | Action |
|------|--------|
| `internal/provenance/doc.go` | create: package comment, states the import rule and "routing signal, not content trust" |
| `internal/provenance/canonical.go`, `canonical_test.go` | create: `Canonicalize` |
| `internal/provenance/marker.go`, `marker_test.go` | create: `Hash`, `Marker`, `Mark`, constants |
| `internal/provenance/classify.go`, `classify_test.go` | create: `State`, `Classify` |
| `internal/provenance/commit.go`, `commit_test.go`, `fakes_test.go` | create: store/router interfaces, `Committer`, `Commit`, in-memory fake store |
| `internal/provenance/substitute.go`, `substitute_test.go` | create: `SubstituteDependencies` |
| `internal/provenance/chain.go`, `chain_test.go` | create: `CommitChain` (external package `provenance_test` for the watcher parser check) |
| `internal/provenance/deps_test.go` | create: `TestNoForbiddenImports` |
| `internal/provenance/docs_test.go` | create: doc-drift test over `docs/provenance.md` |
| `docs/provenance.md` | create: reference |
| `README.md` | modify: add a link to `docs/provenance.md` next to the existing `docs/guards.md` link |
| `docs/guards.md` | modify: where it says #378 adds the commit step, link to `provenance.md` (keep the literal `#378`, `docs_test.go` requires it) |
| `internal/watcher/dependencies.go` | read only: the parser the substituted line must satisfy |
| `internal/guard/*`, `internal/stageoutput/*` | read only: types reused (`guard.Result`, `guard.OutcomeReadyToCommit`, `stageoutput.Document`, `Marshal`) |

Do not edit `.kairon/specs/maturity-model/` or any other historical artifact (AGENTS.md).

## 4. Acceptance Criteria to Test Mapping

| AC | Test (all in `internal/provenance`) | What it asserts |
|----|--------------------------------------|-----------------|
| 1 | `TestCanonicalize_RemovesMarkerNormalizesEndingsStripsTrailingWhitespace`, `TestCanonicalize_EquivalentBodiesMatch` | A body and a copy with CRLF, lone CR, per-line trailing spaces/tabs, a trailing newline and an existing marker line canonicalize to the identical string. Also pins idempotence. |
| 2 | `TestCommit_WritesV1MarkerAsLastLine` | With a fake store, the committed body's final line equals `<!-- planner-provenance: v1 sha256=<hex> generated=<RFC3339Z> -->` for the injected clock, `<hex>` equals an independently computed `sha256` of the canonical body, and the marker matches the documented regex. |
| 3 | `TestClassify_ThreeStates` | Fixtures: unmarked gives `none`, correctly marked gives `valid`, marked-then-edited gives `mismatch`. |
| 4 | `TestMark_MarkingTwiceYieldsSameHash`, `TestHash_ExcludesMarker` | `Hash(Mark(b)) == Hash(b)`, and re-marking a marked body leaves the sha256 field unchanged. |
| 5 | `TestRoundTrip_CommittedBodyClassifiesValid`, `TestClassify_MalformedMarkerIsMismatch` | A body written by `Commit` classifies `valid`. Table of malformed markers (bad hex length, uppercase hex, unknown version `v2`, missing `generated`, bad timestamp, duplicate marker, marker not last, wrong hash) all classify `mismatch`, never `valid` or `none`. |
| 6 | `TestCommit_RefusesWithoutAllPass`, `TestCommit_ExactlyOneWrite` | A zero `guard.Result` and an `OutcomeRejected` result return `ErrNotReady` and make zero store calls. An all-pass create makes exactly one `Create` and no `Update`; an all-pass update makes exactly one `Update`. A failing write is attempted once. |
| 7 | `TestCommitChain_SubstitutesBeforeHashing` (external package) | Two proposed issues, the second `Dependencies: #{first}`. The second's committed body contains `Dependencies: #<first number>`, `watcher.DependencyParser{}.ParseDependencies` returns `[]int{<first number>}` from it, and `Classify(committedSecond) == valid`. Also: the admit callback receives the substituted document (not the placeholder), and an unknown placeholder returns `ErrUnknownPlaceholder` with no write. |
| 8 | `TestCommit_ConcurrentEditAbortsWrite`, `TestCommit_ConflictFallsBackToPunchOut` | Fake store whose `Revision` differs from `Target.ObservedRevision`: zero `Update` calls, the fake body unchanged, `Router.RouteToIntake` called once and result `OutcomeRoutedToIntake`. If `RouteToIntake` errors, `PunchOut` is called once and the result is `OutcomePunchedOut`. If both error, `ErrUnroutable`. A `Revision` error returns `ErrRevisionUnavailable` with no write. |

Constraint tests: `TestNoForbiddenImports` (no `os`, `os/exec`, `net`, `net/http`, `crypto/hmac`, no internal package
other than `guard` and `stageoutput`) covers "no HMAC, no secret, no key material". `TestSubstituteDependencies_*`
cover the single-line form (including a negative test that a `## Dependencies` bullet list is not produced and that
`ParseDependencies` of a header-plus-bullets body returns nothing, documenting why).

## 5. Team Orchestration

```
canonicalize-hash-classify ──> commit-step ──────────┐
                                                     ├──> commit-chain ──> provenance-docs ──> validate-complete
dependency-substitution ─────────────────────────────┘
```

- `canonicalize-hash-classify` and `dependency-substitution` have no dependencies and run in parallel (different files,
  no shared symbols).
- `commit-step` needs `Mark`/`Classify` from the first task (its round-trip test calls `Classify`).
- `commit-chain` needs `Commit` and `SubstituteDependencies`.
- `provenance-docs` needs the final API and behaviour so that doc examples are real.
- `validate-complete` is read-only.

Every builder follows AGENTS.md TDD: write the test, add the minimum stub so it compiles, run it and confirm it fails on
the expected assertion, then implement. Commit tests and implementation together. Name the failing test seen first in the
commit message. Run `gofmt`, `go vet` and `go test -race` before finishing.

## 6. Step-by-Step Task Breakdown

### Task 1: canonicalize-hash-classify
Create `internal/provenance` with `doc.go`, `canonical.go`, `marker.go`, `classify.go` and their tests per 2.2 to 2.4.
Also create `deps_test.go` (`TestNoForbiddenImports`). Tests first.
**Acceptance:** AC 1, 3, 4, and the classifier half of AC 5 pass; `Canonicalize` is idempotent; `Classify` returns
`mismatch` for each malformed-marker case in the table.
**Dependencies:** none.

### Task 2: dependency-substitution
Create `substitute.go` and `substitute_test.go` per 2.6 (function only, not the chain).
**Acceptance:** placeholders in dependencies lines are replaced with the real numbers in single-line comma form;
placeholders outside a dependencies line are untouched; unknown keys give `ErrUnknownPlaceholder`; a malformed part on
a placeholder line gives `ErrBadDependencyLine`; lines without placeholders are unchanged.
**Dependencies:** none (parallel with Task 1).

### Task 3: commit-step
Create `commit.go`, `commit_test.go`, `fakes_test.go` per 2.5.
**Acceptance:** AC 2, 6, 8 and the round-trip half of AC 5 pass; `Commit` calls no store method unless the admission result
is all-pass; one write per issue; conflict path performs no write and routes.
**Dependencies:** Task 1.

### Task 4: commit-chain
Create `chain.go` and `chain_test.go` (external test package `provenance_test`) per 2.6.
**Acceptance:** AC 7 passes, including the `ParseDependencies` assertion, the validates-after-substitution assertion and
the guard-sees-substituted-document assertion; the chain stops at the first guard rejection or conflict and reports
what was committed.
**Dependencies:** Task 2, Task 3.

### Task 5: provenance-docs
Create `docs/provenance.md` and `internal/provenance/docs_test.go`; add links in `README.md` and `docs/guards.md`.
The document covers: the marker format with a worked example; canonicalization rules, including the trailing-line rule
and the hard-break caveat; the three read states and the pipeline action for each (§8.4); the commit step order and the
optimistic-concurrency behaviour with its residual race; dependency substitution and the `#{key}` convention with the
single-line requirement; and an explicit section that the hash is a routing and classification signal, never
trust-of-content, that there is no HMAC or secret and why (§8.3, §8.5). Use fenced `markdown-valid`,
`markdown-none` and `markdown-mismatch` blocks as worked bodies; `docs_test.go` reads them (same approach as
`internal/guard/docs_test.go`) and asserts `Classify` returns `valid`, `none` and `mismatch` respectively, and asserts the
doc names required terms (`v1`, `sha256`, `valid`, `none`, `mismatch`, `routing`, `HMAC`, `Dependencies:`, `#377`,
`#381`, `#382`).
**Acceptance:** docs test passes; links present; the document says the hash is not a content-trust control.
**Dependencies:** Task 4.

### Task 6: validate-complete
Read-only verification of every acceptance criterion against the code, the tests and the whole repo checks.
**Dependencies:** Task 5.

## 7. Validation Commands

```
go build ./...
go vet ./...
test -z "$(gofmt -l .)"
go test -race -count=1 ./internal/provenance/...
go test -race -count=1 ./internal/guard/... ./internal/stageoutput/... ./internal/watcher/...
go test -race -count=1 ./...
git status --porcelain -- cmd/kairon/templates   # must print nothing
```

## 8. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "canonicalize-hash-classify"
    agent: "builder"
    description: "Create package internal/provenance with Canonicalize (normalize CRLF/CR to LF, drop every line starting with the '<!-- planner-provenance:' prefix, strip per-line trailing whitespace, drop trailing empty lines, no trailing newline), Hash (lower-case hex SHA-256 of the canonical body), Marker/Mark (writes Canonicalize(body)+newline+'<!-- planner-provenance: v1 sha256=<hex> generated=<RFC3339 UTC> -->' as the last line), the State type (StateInvalid zero value, StateValid, StateNone, StateMismatch with String) and Classify (none when no marker candidate line; valid only when exactly one candidate that is the last non-empty line, matches the strict v1 regex with a 64-char lower-case hex and an RFC 3339 generated value, and whose hex equals Hash(body); otherwise mismatch). Also create doc.go and deps_test.go (TestNoForbiddenImports: non-test code may import only the standard library, internal/guard and internal/stageoutput, and must not directly import os, os/exec, net, net/http or crypto/hmac). Follow AGENTS.md TDD: write the tests first, add minimal stubs so they compile, confirm each fails on its assertion, then implement. Do not touch the .kiro/agents files already modified in the worktree."
    dependencies: []
    acceptance_criteria:
      - "TestCanonicalize_EquivalentBodiesMatch shows a body and a copy differing only by CRLF/lone CR, per-line trailing spaces and tabs, trailing newlines and an existing marker line canonicalize to the identical string, and Canonicalize is idempotent (AC 1)"
      - "TestClassify_ThreeStates shows an unmarked body is none, a body produced by Mark is valid, and a marked body edited afterwards is mismatch (AC 3)"
      - "TestMark_MarkingTwiceYieldsSameHash shows marking a body, then marking the result, yields the same sha256 and the marker is excluded from its own hash input (AC 4)"
      - "TestClassify_MalformedMarkerIsMismatch shows bad hex length, uppercase hex, unknown version v2, missing generated, unparsable timestamp, duplicate marker, marker not on the last non-empty line, and wrong hash all classify as mismatch and never valid or none (AC 5, classifier half)"
      - "TestNoForbiddenImports passes: no direct os, os/exec, net, net/http or crypto/hmac import, and the only internal packages in go list -deps are provenance, guard and stageoutput (no key material, no secret)"
      - "The marker line matches the regex ^<!-- planner-provenance: v1 sha256=[0-9a-f]{64} generated=\\S+ -->$ and is the final line of Mark output with no trailing newline"
    validation_commands:
      - "go build ./internal/provenance/..."
      - "go vet ./internal/provenance/..."
      - "go test -race -count=1 ./internal/provenance/..."
      - "test -z \"$(gofmt -l internal/provenance)\""

  - id: "dependency-substitution"
    agent: "builder"
    description: "Add SubstituteDependencies(body string, numbers map[string]int) (string, error) in internal/provenance/substitute.go with tests. For each line that contains (case-insensitive) 'dependencies:', split the text after the first occurrence on commas, require every part to be '#<digits>' or the placeholder '#{key}' (key matches [A-Za-z0-9][A-Za-z0-9_-]*), replace placeholders with real issue numbers, drop duplicate numbers, and rewrite the line in the single-line form 'Dependencies: #N, #M'. Lines with no placeholder are returned unchanged; '#{...}' outside a dependencies line is never touched. Return ErrUnknownPlaceholder for a key missing from the map (unknown key or forward reference) and ErrBadDependencyLine for a malformed part on a line that has a placeholder. Write tests first per AGENTS.md TDD (confirm each fails on its assertion before implementing). Keep the file independent of the other provenance files so it can be built in parallel."
    dependencies: []
    acceptance_criteria:
      - "TestSubstituteDependencies_ReplacesPlaceholders: 'Dependencies: #{a}, #314' with a=7 becomes 'Dependencies: #7, #314' and duplicates are removed"
      - "TestSubstituteDependencies_OnlyDependenciesLine: '#{x}' in a code sample or prose line is left byte-for-byte unchanged"
      - "TestSubstituteDependencies_UnknownPlaceholder returns an error matching ErrUnknownPlaceholder for an unknown key and for a forward reference"
      - "TestSubstituteDependencies_BadLine returns an error matching ErrBadDependencyLine for a placeholder line containing a non-reference part such as 'none'"
      - "TestSubstituteDependencies_NoPlaceholderUnchanged returns a body with no placeholder exactly as given"
      - "Output never uses a '## Dependencies' bullet-list form; the rewritten line is a single line"
    validation_commands:
      - "go build ./internal/provenance/..."
      - "go vet ./internal/provenance/..."
      - "go test -race -count=1 -run 'SubstituteDependencies' ./internal/provenance/..."
      - "test -z \"$(gofmt -l internal/provenance)\""

  - id: "commit-step"
    agent: "builder"
    description: "Add internal/provenance/commit.go with the IssueStore interface (Revision, Create, Update), Router interface (RouteToIntake, PunchOut), Conflict, Target, Request (Doc *stageoutput.Document, Title, Target, Admission guard.Result), Outcome (OutcomeInvalid zero, OutcomeCommitted, OutcomeRoutedToIntake, OutcomePunchedOut), Result, Committer{Store, Router, Now} and Commit. Commit order: refuse with ErrNotReady unless Admission.Outcome is guard.OutcomeReadyToCommit and Failure is nil, with no store call at all; validate the request (canonical doc with non-empty body, title for create, ObservedRevision for update) with errors matching ErrInvalid; body := Mark(doc.Body, now); create path makes exactly one Store.Create; update path reads Store.Revision (error => ErrRevisionUnavailable and no write), and if it differs from Target.ObservedRevision performs no write and calls Router.RouteToIntake, falling back to Router.PunchOut if that errors (both error => ErrUnroutable), otherwise makes exactly one Store.Update. Never retry. Write an in-memory fake store and router in fakes_test.go that count calls and record bodies. Tests first per AGENTS.md TDD (confirm each fails on its assertion before implementing)."
    dependencies: ["canonicalize-hash-classify"]
    acceptance_criteria:
      - "TestCommit_WritesV1MarkerAsLastLine: the body passed to the fake store ends with the line '<!-- planner-provenance: v1 sha256=<hex> generated=<RFC3339Z> -->' for the injected clock, the hex equals an independently computed sha256 of the canonical body, and it matches the documented marker form (AC 2)"
      - "TestRoundTrip_CommittedBodyClassifiesValid: a body written by Commit classifies as valid (AC 5, round-trip half)"
      - "TestCommit_RefusesWithoutAllPass: a zero guard.Result and an OutcomeRejected result both return ErrNotReady and the fake store records zero calls of any kind (AC 6)"
      - "TestCommit_ExactlyOneWrite: an all-pass create makes exactly one Create and zero Update; an all-pass update makes exactly one Update; a failing write is attempted exactly once (AC 6)"
      - "TestCommit_ConcurrentEditAbortsWrite: when Revision differs from Target.ObservedRevision there are zero Update calls, the stored body is unchanged, RouteToIntake is called once and the result is OutcomeRoutedToIntake (AC 8)"
      - "TestCommit_ConflictFallsBackToPunchOut: RouteToIntake error leads to exactly one PunchOut call and OutcomePunchedOut; both failing returns ErrUnroutable; a Revision error returns ErrRevisionUnavailable with no write (AC 8)"
      - "A punch_out document, a nil document, an empty body, a create with an empty title and an update with an empty ObservedRevision each return an error matching ErrInvalid and make no store call"
    validation_commands:
      - "go build ./internal/provenance/..."
      - "go vet ./internal/provenance/..."
      - "go test -race -count=1 ./internal/provenance/..."
      - "test -z \"$(gofmt -l internal/provenance)\""

  - id: "commit-chain"
    agent: "builder"
    description: "Add internal/provenance/chain.go with ChainItem{Key, Title, Doc stageoutput.Document, Target}, Admit func(raw string) (guard.Result, error), ChainResult and (*Committer).CommitChain. For each item in order: call SubstituteDependencies on the item's body using the numbers committed so far, put the substituted body on a copy of the document, stageoutput.Marshal it, call admit on that raw text (the guard chain sees the already substituted document), stop at the first item whose result is not OutcomeReadyToCommit (the guard already ran its OnFailure) or whose Commit does not return OutcomeCommitted, otherwise Commit the substituted document (so Mark hashes the substituted body) and record numbers[item.Key] = result.Number. Return the issues committed so far so a caller can resume; do not roll back. Put the tests in chain_test.go in the external package provenance_test so importing internal/watcher does not create a future import cycle. Tests first per AGENTS.md TDD (confirm each fails on its assertion before implementing)."
    dependencies: ["dependency-substitution", "commit-step"]
    acceptance_criteria:
      - "TestCommitChain_SubstitutesBeforeHashing: committing two proposed issues where the second has 'Dependencies: #{first}' stores a second body containing 'Dependencies: #<first number>' (AC 7)"
      - "In the same test, watcher.DependencyParser{}.ParseDependencies on the committed second body returns exactly []int{<first number>} (single-line form satisfies internal/watcher/dependencies.go) (AC 7)"
      - "In the same test, Classify of the committed second body is valid, so the stored hash validates against the committed substituted bytes (AC 7)"
      - "TestCommitChain_AdmitSeesSubstitutedDocument: the raw text passed to admit for the second issue contains the real number and no '#{' placeholder"
      - "TestCommitChain_UnknownPlaceholderWritesNothing: a forward reference returns ErrUnknownPlaceholder, the guard admit callback is not called for that item and the store records no write for it"
      - "TestCommitChain_StopsOnRejection: when admit returns an OutcomeRejected result for item 2 of 3, item 3 is never processed, item 1 stays committed and ChainResult reports StoppedAt == 1"
      - "TestCommitChain_StopsOnConflict: a revision conflict on an update item stops the chain with no write for that item and routes via the Router"
    validation_commands:
      - "go build ./internal/provenance/..."
      - "go vet ./internal/provenance/..."
      - "go test -race -count=1 ./internal/provenance/..."
      - "test -z \"$(gofmt -l internal/provenance)\""

  - id: "provenance-docs"
    agent: "builder"
    description: "Write docs/provenance.md and internal/provenance/docs_test.go, and add links to README.md (next to the docs/guards.md link) and docs/guards.md (where it says #378 adds the commit step; keep the literal '#378'). The document covers the v1 marker format with a worked example, the canonicalization rules (including dropping trailing empty lines and the loss of two-space hard line breaks), the three read states valid/none/mismatch with the pipeline action for each (planner-requirements §8.4: if the hash is not valid, planner-process the issue, otherwise proceed), the commit step order and optimistic concurrency with its residual race (no conditional write in gh/REST, updatedAt one-second granularity, caught on read as mismatch), dependency substitution with the '#{key}' placeholder and the single-line 'Dependencies:' requirement, that no gh-backed IssueStore ships here (#381 supplies it), and a section stating the hash is a routing and classification signal and never a trust-of-content control, with no HMAC, key or secret and why (§8.3, §8.5). Include fenced blocks tagged markdown-valid, markdown-none and markdown-mismatch holding real worked bodies (generate the valid one with provenance.Mark). docs_test.go reads them like internal/guard/docs_test.go and asserts Classify returns valid, none and mismatch respectively, and asserts the document mentions v1, sha256, valid, none, mismatch, routing, HMAC, 'Dependencies:', #377, #381 and #382. Use plain, direct prose and match the tone of docs/guards.md."
    dependencies: ["commit-chain"]
    acceptance_criteria:
      - "docs/provenance.md exists and documents the marker format, the three read-states, the commit step with optimistic concurrency, dependency substitution, and the no-HMAC decision"
      - "The document states explicitly that the hash is a routing/classification signal and not a content-trust control"
      - "TestDocs_ExamplesClassifyAsLabelled passes: the markdown-valid block classifies valid, markdown-none classifies none, markdown-mismatch classifies mismatch"
      - "TestDocs_NamesRequiredTerms passes"
      - "README.md and docs/guards.md link to docs/provenance.md and docs/guards.md still contains '#378' and '#379'"
    validation_commands:
      - "test -f docs/provenance.md"
      - "go test -race -count=1 ./internal/provenance/... ./internal/guard/..."
      - "grep -n 'provenance.md' README.md docs/guards.md"
      - "grep -n 'HMAC' docs/provenance.md"
      - "test -z \"$(gofmt -l internal/provenance)\""

  - id: "validate-complete"
    agent: "validator"
    description: "Verify the whole change against the eight issue #378 acceptance criteria, criterion by criterion, by reading the tests and running them: confirm each AC maps to a passing, meaningful test (not a vacuous one), that the writer and the classifier share the single Canonicalize path, that Commit makes no store call unless the guard result is all-pass and makes exactly one write, that the update path rechecks the revision and performs no write on change, that substitution happens before hashing and the committed second body satisfies watcher.DependencyParser.ParseDependencies, that internal/provenance imports only the standard library, internal/guard and internal/stageoutput and has no HMAC or key material, and that docs/provenance.md exists with the required content. Confirm no file under cmd/kairon/templates changed and that the pre-existing .kiro/agents edits were not staged or reverted. Run the full repo checks."
    dependencies: ["provenance-docs"]
    acceptance_criteria:
      - "All 8 acceptance criteria of issue #378 are each verified against a named passing test"
      - "go build, go vet, gofmt and the full go test -race suite pass"
      - "internal/provenance has no forbidden imports and no key material"
      - "docs/provenance.md states the hash is a routing signal and not content trust"
      - "No template-synchronized file was modified"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
      - "go test -race -count=1 ./internal/provenance/..."
      - "go test -race -count=1 ./..."
      - "test -z \"$(git status --porcelain -- cmd/kairon/templates)\""
      - "test -f docs/provenance.md"
```
