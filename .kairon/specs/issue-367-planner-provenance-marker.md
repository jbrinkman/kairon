# Design Spec: Planner provenance marker (canonicalize, hash, classify issue bodies)

Closes #367

## Problem

The pipeline cannot tell a planner-produced issue from a raw human-written one, and cannot detect a planner issue
that a human edited after generation. The maturity-model planner requirements (§8) call for a weak, tamper-evident
content hash embedded in the issue body. A reader can then classify any issue as planner-canonical (`valid`),
manually created (`none`) or edited-after-generation (`mismatch`) and route it. The hash has no secret: an unmarked
or mismatched issue is re-routed through the planner, never rejected.

`internal/eval/provenance.go` is eval-RUN provenance (models, prompt hashes) and is unrelated.

**Source-material note.** The issue cites `.kairon/specs/maturity-model/planner-requirements.md` §8, but that file
does not exist in this tree, on any branch, or in git history. The `maturity-model/` directory holds only
`gap-analysis.md`, `stage-3/4/5.md` and `step-0-validation.md`. This spec therefore takes the marker form and
read-states from the issue text alone. Every rule is stated explicitly below, so the builder does not need §8.
Where the issue is silent (trailing blank lines, multiple marker lines, unknown versions), this spec makes a
decision and says so under "Decisions beyond the issue text".

## Solution Approach

Add one small, standard-library-only Go package that is the single reader/writer of the marker. The future planner
calls it to write the marker and the future watcher calls it to read the marker. Neither is wired in by this issue.

- **Package:** `internal/issueprov`, import path `github.com/jbrinkman/kairon/internal/issueprov`. The name is
  deliberately not `provenance`, to avoid confusion with `internal/eval/provenance.go`.
- **Dependencies:** `crypto/sha256`, `encoding/hex`, `regexp`, `strings`, `time`, `unicode`. It must not import
  `internal/watcher`, `internal/eval` or any other Kairon package. A `go list -deps` test enforces this, following
  the existing `internal/checks/deps_test.go` pattern.
- **Hash scope:** the SHA-256 covers the canonicalized body only. The `generated` timestamp is not hashed, so
  re-marking the same body at a later time yields the same hash (AC 5).

### Marker format (scheme `v1`)

```
<!-- planner-provenance: v1 sha256=<64 lowercase hex> generated=<RFC 3339 UTC> -->
```

- `generated` is written as `t.UTC().Format(time.RFC3339)`, for example `2026-10-18T19:15:34Z`. It is an
  ISO-8601 profile, and RFC 3339 is the subset Go parses reliably.
- `v1` travels in the marker. A future `v2` can change canonicalization without misclassifying old issues, because
  each marker names the rules it was hashed under.

### Canonicalization (`v1`)

`Canonicalize(body)` applies these steps in order:

1. Normalize line endings: `\r\n` becomes `\n`, then any remaining lone `\r` becomes `\n`.
2. Split on `\n`.
3. Drop every marker line. A marker line is any line whose `strings.TrimSpace` starts with the literal prefix
   `<!-- planner-provenance:`. The test is deliberately broad so that a corrupted marker is still recognized as a
   marker (AC 6).
4. Strip trailing whitespace (`strings.TrimRightFunc(line, unicode.IsSpace)`) from each remaining line.
5. Join with `\n`, then remove all trailing empty lines, so the result has no trailing `\n`.

`Hash(body)` returns `hex(sha256(Canonicalize(body)))`.

### Classification

`Classify(body) State` returns exactly one of:

| State | Meaning |
|-------|---------|
| `StateNone` (`"none"`) | No marker line exists (the body contains no line matching the marker prefix). |
| `StateValid` (`"valid"`) | Exactly one marker line, strictly well-formed, version `v1`, and its `sha256` equals `Hash(body)`. |
| `StateMismatch` (`"mismatch"`) | A marker line exists but the body is not valid. The causes are a hash that differs from the recomputed hash, a malformed or unparseable marker, an unknown version, a bad `generated` value, or more than one marker line. |

A marker line is **well-formed** when its `strings.TrimSpace` matches this anchored regexp:

```
^<!-- planner-provenance: (v[0-9]+) sha256=([0-9a-f]{64}) generated=(\S+) -->$
```

and `generated` additionally parses with `time.Parse(time.RFC3339, ...)`. Anything else that carries the prefix is
`StateMismatch`, never `StateValid` or `StateNone` (AC 6).

### Writing

- `MarkerLine(body string, generated time.Time) string` returns the marker line for `body`, with no trailing newline.
- `Append(body string, generated time.Time) string` returns `body` with any existing marker lines removed and
  trailing blank lines trimmed, followed by `"\n\n"` and the marker line. There is no trailing newline, so the
  marker is the last line (AC 3). An empty body yields just the marker line. `Append` does not otherwise rewrite
  the user's content. It preserves the body's own line endings and trailing whitespace, because `Canonicalize`
  absorbs both.

`Append` is idempotent on the hash: re-marking a marked body replaces the marker and the hash is unchanged (AC 5).

### Public API (final)

```go
package issueprov

const Version = "v1"

type State string

const (
    StateNone     State = "none"
    StateValid    State = "valid"
    StateMismatch State = "mismatch"
)

func Canonicalize(body string) string
func Hash(body string) string
func MarkerLine(body string, generated time.Time) string
func Append(body string, generated time.Time) string
func Classify(body string) State
```

### Decisions beyond the issue text

1. **Trailing blank lines are dropped from the canonical form (step 5).** The issue lists marker removal, line
   endings and per-line trailing whitespace. Without this extra rule, `Append`'s separator newline would change the
   hash between "before marking" and "after marking", which breaks AC 3 and AC 5. It also keeps the hash stable if
   GitHub or an editor adds or strips a final newline.
2. **Lone `\r` is treated as a line ending.** Harmless and more tolerant than `\r\n` alone.
3. **Unknown version (for example `v2`) classifies as `mismatch`.** This reader cannot verify rules it does not
   know, and `mismatch` routes the issue through the planner, which is the safe direction.
4. **More than one marker line classifies as `mismatch`.** It is ambiguous, and a stray extra marker is a sign of
   hand-editing.
5. **The marker may sit anywhere in the body for classification.** Only `Append` places it last. A marker that
   moved still validates if the rest of the body is unchanged, because the hash excludes marker lines.
6. **Digest comparison is plain string equality.** There is no secret, so constant-time comparison buys nothing.
7. **Marker detection is line-based and not markdown-aware.** A line inside a fenced code block that begins with the
   marker prefix counts as a marker line. This is the safe direction (it forces `mismatch` or `valid`, never a
   silent `none`) and is documented.

## Relevant Files

Create:
- `internal/issueprov/issueprov.go` — package doc, constants, `Canonicalize`, `Hash`, `MarkerLine`, `Append`, `Classify`.
- `internal/issueprov/issueprov_test.go` — behaviour tests for AC 1–6 and the decisions above.
- `internal/issueprov/deps_test.go` — import-independence test (no `internal/watcher`, no `internal/eval`).
- `docs/planner-provenance.md` — short reference for the marker format, canonicalization and the three read-states.

Read-only references: `internal/checks/deps_test.go` (pattern for the dependency test),
`internal/eval/provenance.go` (unrelated, do not touch), `internal/watcher/` (future consumer, do not touch).

Not modified: the watcher, the planner, `README.md`, `cmd/kairon/templates/` and any agent or skill file. This work
touches no template-synchronized file, so `task sync:check` is unaffected. `.kairon/specs/maturity-model/` is
historical and must not be edited (AGENTS.md).

## Team Orchestration

- `implement-issueprov` (builder) is the only code task. It follows TDD per AGENTS.md. The builder writes the tests
  first, confirms they fail for the expected reason (the package does not exist or the function is missing, not a
  typo), then writes the implementation and commits tests and code together. The commit message names the first
  failing test observed.
- `document-issueprov` (documenter) depends on the implementation so the doc quotes the real, tested digest. It is
  a non-code change and needs no test first.
- `validate-issueprov` (validator) runs read-only checks after both complete.

```
implement-issueprov ──> document-issueprov ──> validate-issueprov
```

The tasks are a chain because the doc quotes the implementation's tested digest, and validation needs both.

## Step-by-Step Task Breakdown

### Task 1: implement-issueprov (builder)

TDD order. Write `issueprov_test.go` and `deps_test.go` first, run them and confirm they fail because the package
symbols are undefined, then implement `issueprov.go`.

Test fixtures, using a shared base body:

```
base = "## Summary\n\nAdd a widget.\n\n## Acceptance Criteria\n1. It works."
```

The expected digest was computed independently with `printf '%s' "$base" | shasum -a 256`:

```
Hash(base) == "ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13"
```

Tests, one per acceptance criterion plus the decisions:

- **AC 1:** `Hash(base)` equals the digest above. `Hash(base + "\n")`, `Hash` of the CRLF version, and `Hash` of a
  version with trailing spaces on each line all equal the same digest. `MarkerLine(base, t)` for
  `t = 2026-10-18T19:15:34Z` equals
  `<!-- planner-provenance: v1 sha256=ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13 generated=2026-10-18T19:15:34Z -->`.
  Also check that a non-UTC `time.Time` is rendered in UTC.
- **AC 2:** table test with three fixtures. The unmarked `base` gives `StateNone`, `Append(base, t)` gives
  `StateValid`, and `Append(base, t)` with `"extra"` added to the text gives `StateMismatch`.
- **AC 3:** `got := Append(base, t)` classifies as `StateValid`, and `strings.Split(got, "\n")[last]` equals
  `MarkerLine(base, t)`. Also: an empty body gives just the marker line, and `Append` of a body that ends in
  several blank lines trims them.
- **AC 4:** start from `Append(base, t)`. Add trailing spaces and tabs to every line, then separately convert
  `\n` to `\r\n`. Both still classify as `StateValid`.
- **AC 5:** `once := Append(base, t1)` and `twice := Append(once, t2)` with `t2 != t1`. The extracted `sha256`
  values are equal, `Classify(twice)` is `StateValid`, and `twice` contains exactly one marker line.
- **AC 6:** each of these classifies as `StateMismatch` (table test): a truncated marker
  (`<!-- planner-provenance: v1 sha256=abc -->`), a non-hex or uppercase digest, a missing `generated`, a
  non-RFC3339 `generated`, a missing closing `-->`, and a marker with a valid hash but a corrupted `v1` token
  (`vX`).
- **Decisions:** a well-formed `v2` marker is `StateMismatch`. Two marker lines (one valid, one duplicate) are
  `StateMismatch`. A valid marker moved to the top of the body is still `StateValid`. Editing the body after
  marking is `StateMismatch` (AC 2 variant).
- **Independence (`deps_test.go`):** `go list -deps .` must not list `internal/watcher` or `internal/eval` (or
  subpackages). Mirror `internal/checks/deps_test.go`.

Implementation notes:

- Compile the strict regexp once at package level (`regexp.MustCompile`).
- Write a private `isMarkerLine(line string) bool` that checks the prefix of `strings.TrimSpace(line)`, and a
  private `parseMarker(line string) (version, sum, generated string, ok bool)`.
- Keep `Canonicalize` pure and allocation-light. Issue bodies are small, so no streaming is needed.
- Add a package doc comment that states the weak-hash purpose (no secret, tamper-evident not tamper-proof), the
  three states, and that this package is the shared reader/writer for the planner and the watcher.

### Task 2: document-issueprov (documenter)

Write `docs/planner-provenance.md`. Keep it short. Content:

- Purpose in two sentences: the marker lets a reader classify a body as planner-canonical, manual or edited, and the
  hash is weak and has no secret.
- The marker line format, with one concrete example that uses the tested digest
  (`ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13`) for the body in the test fixture.
- The five canonicalization steps from this spec, as a numbered list.
- A read-states table (`valid`, `none`, `mismatch`) with the exact conditions, including the malformed, unknown
  version and multiple-marker cases.
- How to use the package: `issueprov.Append`, `issueprov.Classify`, with a short Go snippet.
- Versioning: `v1` is in the marker so rules can change later.
- A short note that watcher integration and planner emission are not yet wired up.
- State the intended routing: re-route `none` and `mismatch` issues through the planner, never reject them.

Do not edit `README.md` or any file under `.kairon/specs/` (the issue scopes docs to `docs/`).

### Task 3: validate-issueprov (validator)

Read-only. Run the validation commands below, then check each of the six acceptance criteria against the tests by
name, and confirm that the doc's digest matches the tested digest and that no file outside the Relevant Files list
(plus this spec and sentinels) changed.

## Validation Commands

```bash
go build ./...
go vet ./internal/issueprov/...
go test -race -count=1 ./internal/issueprov/...
test -z "$(gofmt -l internal/issueprov)"
! go list -deps ./internal/issueprov | grep -E 'kairon/internal/(watcher|eval)'
grep -q 'ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13' docs/planner-provenance.md
go run ./cmd/kairon plan parse .kairon/specs/issue-367-planner-provenance-marker.md | grep -q '"status": "valid"'
task sync:check
task lint
task test
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-issueprov"
    agent: "builder"
    description: "TDD: write tests then implement the internal/issueprov package (Canonicalize, Hash, MarkerLine, Append, Classify with StateNone/StateValid/StateMismatch) in the same commit. The package uses only the standard library and does not import internal/watcher or internal/eval."
    dependencies: []
    acceptance_criteria:
      - "internal/issueprov/issueprov_test.go asserts Hash of the base fixture equals ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13 and MarkerLine produces '<!-- planner-provenance: v1 sha256=<hex> generated=<RFC3339 UTC> -->' (AC 1)"
      - "Classify returns exactly one of none, valid, mismatch; unmarked, correctly marked and marked-then-edited fixtures classify as none, valid, mismatch (AC 2)"
      - "Append then Classify yields valid and the marker line is the last line of the returned body, with no trailing newline (AC 3)"
      - "Adding trailing whitespace to every line, or converting \\n to \\r\\n, on a validly marked body still classifies as valid (AC 4)"
      - "Marking a body, then re-marking the result with a different timestamp, yields an equal sha256 and exactly one marker line (AC 5)"
      - "Truncated, non-hex or uppercase digest, missing or non-RFC3339 generated, missing closing -->, and corrupted version token marker lines all classify as mismatch (AC 6)"
      - "A well-formed v2 marker, two marker lines, and a body edited after marking all classify as mismatch; a valid marker moved to the top of the body still classifies as valid"
      - "deps_test.go fails if go list -deps lists internal/watcher or internal/eval (or their subpackages); the test passes for the new package"
      - "Tests and implementation are committed together; the commit message names the first failing test observed"
      - "go vet and gofmt report no issues for internal/issueprov"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/issueprov/..."
      - "go test -race -count=1 ./internal/issueprov/..."
      - "test -z \"$(gofmt -l internal/issueprov)\""

  - id: "document-issueprov"
    agent: "documenter"
    description: "Write docs/planner-provenance.md: a short reference for the v1 marker format, the canonicalization steps, the three read-states, package usage and the current integration status."
    dependencies: ["implement-issueprov"]
    acceptance_criteria:
      - "docs/planner-provenance.md exists and shows the exact marker line format '<!-- planner-provenance: v1 sha256=<hex> generated=<ISO-8601> -->'"
      - "The doc lists the five canonicalization steps (normalize line endings, split, drop marker lines, strip trailing whitespace per line, join and remove trailing blank lines) and states that the generated timestamp is not hashed"
      - "The doc has a read-states table covering valid, none and mismatch, including malformed marker, unknown version and multiple marker lines as mismatch"
      - "The doc includes the tested example digest ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13"
      - "The doc states the hash is weak and has no secret, that unmarked or mismatched issues are re-routed through the planner and never rejected, and that watcher integration and planner emission are not yet implemented"
      - "No file other than docs/planner-provenance.md is modified by this task"
    validation_commands:
      - "test -f docs/planner-provenance.md"
      - "grep -q 'ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13' docs/planner-provenance.md"
      - "grep -q 'planner-provenance: v1 sha256=' docs/planner-provenance.md"
      - "grep -qi 'mismatch' docs/planner-provenance.md"

  - id: "validate-issueprov"
    agent: "validator"
    description: "Read-only verification that all six issue acceptance criteria and both constraints are met, the doc matches the tested digest, and only the expected files changed."
    dependencies: ["implement-issueprov", "document-issueprov"]
    acceptance_criteria:
      - "go build, go vet, gofmt and go test -race pass for ./internal/issueprov/..."
      - "Each of issue ACs 1-6 maps to a named passing test in internal/issueprov"
      - "go list -deps ./internal/issueprov lists neither internal/watcher nor internal/eval"
      - "docs/planner-provenance.md quotes the digest asserted in the tests"
      - "task sync:check, task lint and task test pass"
      - "git diff against the base branch touches only internal/issueprov/*, docs/planner-provenance.md, this spec and .kairon/artifacts sentinels"
    validation_commands:
      - "go build ./..."
      - "go test -race -count=1 ./internal/issueprov/..."
      - "test -z \"$(gofmt -l internal/issueprov)\""
      - "grep -q 'ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13' docs/planner-provenance.md"
      - "task sync:check"
      - "task lint"
      - "task test"
```
