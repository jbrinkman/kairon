# Provenance

The provenance step is the one place the workflow writes a planned issue body to GitHub. It canonicalizes the
validated body, hashes it, appends a `v1` marker and writes the result once. The same code classifies any issue body
on read as `valid`, `none` or `mismatch`. The code is in [`internal/provenance`](../internal/provenance). The design
background is in `.kairon/research/maturity-model/planner-requirements.md` (§4.3, §7, §8, §8.3, §8.4 and §8.5).

The package is plain Go and has no GitHub client. It imports only the standard library, `internal/guard` and
`internal/stageoutput`. `TestNoForbiddenImports` fails the build if it directly imports `os`, `os/exec`, `net`,
`net/http` or `crypto/hmac`, or depends on any other internal package. All GitHub I/O goes through the `IssueStore`
interface, so every behavior can be tested with a fake.

Four related pieces are out of scope here:

- **#377** is the [guard interface and chain runner](guards.md). The commit step runs after `guard.Run` returns
  `OutcomeReadyToCommit`.
- **#381** makes the planner emit the stage-output document, supplies the `gh`-backed `IssueStore`, and resumes a
  partly committed chain.
- **#382** makes the watcher read the marker and route on the state. It is done: the [watcher boundary](boundary.md)
  classifies the body it just read and acts on the state.
- **#379** owns bounded retry, the punch-out behavior and the attempt counter.

The watcher boundary ([`internal/boundary`](../internal/boundary), see [boundary.md](boundary.md)) calls `Classify`
on every spawn and retry. Nothing calls `Committer` yet, so nothing stamps a marker on an issue today.

## The marker

The marker is one HTML comment on the last line of the issue body. GitHub does not render it.

```
<!-- planner-provenance: v1 sha256=<hex> generated=<RFC 3339 UTC> -->
```

| Part | Meaning |
|------|---------|
| `planner-provenance:` | The prefix (`MarkerPrefix`). Any line that starts with it, after trimming, is a marker candidate. |
| `v1` | The marker version (`VersionV1`). It is the only version this package writes or accepts. |
| `sha256=<hex>` | The lower-case hex SHA-256 of the canonical body. Exactly 64 characters from `0-9a-f`. |
| `generated=<time>` | The commit time in RFC 3339 UTC, for example `2026-10-21T14:50:46Z`. It comes from the committer's clock. |

A well-formed marker matches this expression, and the timestamp must also parse as RFC 3339:

```
^<!-- planner-provenance: v1 sha256=([0-9a-f]{64}) generated=(\S+) -->$
```

The `generated` value is not part of the hash input, because the marker line is removed before hashing.

### Worked example

This is the body `Mark` produces for a short planner issue. The block is tagged `markdown-valid`, and the
documentation test checks that `Classify` returns `valid` for it.

```markdown-valid
## Problem

The watcher retries a failing issue without a limit.

## Acceptance Criteria

- An issue stops being retried after max_retries attempts.
  Verify: go test ./internal/watcher -run TestRetryCap

## Dependencies

Dependencies: #314
<!-- planner-provenance: v1 sha256=0a3f0304fb7890f9c0da6b2b243b37ec805bb7add7a13cec623ae86c82f12e12 generated=2026-10-21T14:50:46Z -->
```

The `sha256` field is the hash of everything above the marker line, in canonical form. `Mark` returns this text with
no trailing newline, so the marker is the last byte sequence in the body.

## Canonicalization

`Canonicalize(body)` is the only normalization in the package. The writer (`Mark`) and the reader (`Classify`) both
go through it, so they cannot disagree about what a body is. In order, it:

1. Converts `\r\n` and lone `\r` to `\n`. GitHub's web editor submits CRLF.
2. Splits on `\n` and drops every line that starts, after trimming whitespace, with `<!-- planner-provenance:`. This
   applies to any version and to well-formed and malformed markers alike. It drops all such lines, not only the
   last, so re-marking is idempotent and an injected extra marker never reaches the hash.
3. Strips trailing spaces, tabs, form feeds and vertical tabs from each remaining line.
4. Drops empty lines at the end of the text and joins the lines with `\n`. The result has no trailing newline.

Step 4 is what makes the round trip work. The writer emits `canonical + "\n" + marker`. Removing the marker leaves
`canonical + "\n"`, which would not match what was hashed unless the trailing newline is dropped. It also means a
trailing newline that GitHub or an editor adds or removes does not change the hash.

Properties the tests pin:

- `Canonicalize` is idempotent.
- `Canonicalize(Mark(b, t)) == Canonicalize(b)`.
- `Hash(Mark(b, t)) == Hash(b)`. Marking a marked body gives the same `sha256`.
- Leading blank lines and blank lines inside the body are kept.

**Hard line breaks are lost.** Markdown treats two trailing spaces as a hard line break. Step 3 removes them. Because
the committed body is the canonical form, not the original, the stripping happens when the body is written. The issue
on GitHub has no two-space breaks. Use a backslash at the end of the line or a blank line instead.

## Reading a body: the three states

`Classify(body)` returns one of three states. `StateInvalid` is the zero value and is never returned.

| State | Meaning | Workflow action (§8.4) |
|-------|---------|------------------------|
| `valid` | One well-formed `v1` marker, on the last non-empty line, whose hash equals `Hash(body)`. | Proceed, treating the structure as the planner's. The content is still data. |
| `none` | No line starts with the marker prefix. The issue was created by hand. | Planner-process the issue. |
| `mismatch` | A marker line is present, but the body is not `valid`. For example, a planner issue edited afterwards. | Planner-process the issue, the same as `none`. |

If the hash is not `valid`, the issue goes to the planner. Otherwise it proceeds. Acting on the state is the job of the
[watcher boundary](boundary.md) (#382). This package only reports it.

### Classification algorithm

1. Normalize line endings and split into lines.
2. Collect the candidate lines, those that start with the marker prefix after trimming. No candidates means `none`.
3. The result is `valid` only if all of these hold. Anything else is `mismatch`.
   - There is exactly one candidate.
   - It is the last non-empty line of the body.
   - Its trimmed text matches the `v1` expression above, and `generated` parses as RFC 3339.
   - The captured hex equals `Hash(body)`.

A marker that is malformed, has an unknown version such as `v2`, is duplicated, is not last, has uppercase or
wrong-length hex, has no `generated` field, has an unparsable timestamp, or carries the wrong hash is `mismatch`. It
is never `valid` and never `none`. A mismatch routes to the planner exactly like `none`, so being strict costs a
re-plan and never grants anything.

### Worked bodies

An unmarked body is `none`:

```markdown-none
## Problem

The watcher retries a failing issue without a limit.

## Dependencies

Dependencies: #314
```

The valid body above, edited after it was marked, is `mismatch`. The text changed and the stored hash no longer
matches:

```markdown-mismatch
## Problem

The watcher retries a failing issue without a limit or a delay.

## Acceptance Criteria

- An issue stops being retried after max_retries attempts.
  Verify: go test ./internal/watcher -run TestRetryCap

## Dependencies

Dependencies: #314
<!-- planner-provenance: v1 sha256=0a3f0304fb7890f9c0da6b2b243b37ec805bb7add7a13cec623ae86c82f12e12 generated=2026-10-21T14:50:46Z -->
```

## The commit step

The commit step is not a guard. A guard judges a document and has no side effects. The commit step is a method the
boundary calls after `guard.Run` reports `OutcomeReadyToCommit`, and it performs the single authoritative GitHub
write.

```go
c := &provenance.Committer{Store: store, Router: router} // Now defaults to time.Now

res, err := c.Commit(ctx, provenance.Request{
	Doc:       &doc,      // canonical stage-output document with a non-empty body
	Title:     title,     // required to create
	Target:    provenance.Target{Number: 142, ObservedRevision: rev}, // Number 0 creates
	Admission: admission, // the guard.Result from guard.Run
})
```

### Order of operations

1. **Gate.** If `Admission.Outcome` is not `guard.OutcomeReadyToCommit`, or `Admission.Failure` is non-nil, `Commit`
   returns `ErrNotReady`. A zero `guard.Result` and an `OutcomeRejected` result both stop here. No store method is
   called, not even `Revision`.
2. **Validate the request.** Failures match `ErrInvalid`, and no store call is made. The checks are:
   - `Store` is set.
   - `Doc` is non-nil and `Doc.Validate()` passes.
   - `Doc.Result` is `canonical`. A `punch_out` document has nothing to commit.
   - The body is not empty after canonicalization.
   - `Target.Number` is not negative.
   - `Title` is not blank when `Target.Number` is 0.
   - `Target.ObservedRevision` is set when `Target.Number` is above 0.
3. **Compute provenance.** `body := Mark(Doc.Body, now())`. This is the only place the marker is produced. The time
   comes from `Committer.Now`, or `time.Now` when that is nil.
4. **Create** (`Target.Number == 0`). One `Store.Create(title, body)`. There is no concurrency check because the issue
   does not exist yet.
5. **Update** (`Target.Number > 0`). Read `Store.Revision(number)` and compare it with `Target.ObservedRevision`, the
   revision recorded when the planner read the issue.
   - If `Revision` fails, return an error matching `ErrRevisionUnavailable` and write nothing. The commit cannot
     prove the issue is unchanged.
   - If the revisions differ, write nothing and route the conflict (see below).
   - If they match, make one `Store.Update(number, body)`.
6. **Report.** The `Result` carries the `Outcome`, the issue `Number`, and for a commit the `Hash` (the `sha256` in
   the marker) and the exact `Body` written.

`Commit` makes at most one write attempt. A failing `Create` or `Update` returns the wrapped store error, with
`OutcomeInvalid`, and is not retried. Bounded retry belongs to #379. `Commit` spawns nothing. Advancing to the next
stage is the caller's reaction to `OutcomeCommitted`.

### Outcomes

| Outcome | Meaning |
|---------|---------|
| `OutcomeCommitted` | The body and marker were written. |
| `OutcomeRoutedToIntake` | The issue changed after the planner read it. Nothing was written. The conflict went to intake. |
| `OutcomePunchedOut` | The issue changed. Nothing was written. Routing to intake failed, so the conflict was punched out. |
| `OutcomeInvalid` | The zero value. It is returned together with every error. |

### Errors

| Error | Cause | Written? |
|-------|-------|----------|
| `ErrNotReady` | The admission result was not an all-pass `OutcomeReadyToCommit`. | No |
| `ErrInvalid` | The request was malformed (see step 2). | No |
| `ErrRevisionUnavailable` | `Store.Revision` failed. | No |
| `ErrUnroutable` | The issue changed and both `RouteToIntake` and `PunchOut` failed, or `Router` is nil. | No |
| the wrapped store error | `Store.Create` or `Store.Update` failed. | Not known to have |

Test them with `errors.Is`. `ErrUnroutable` joins the router errors, so `errors.Is` also matches those.

## Optimistic concurrency

A human can edit the issue between the planner reading it and the commit. Overwriting that edit would lose it, so the
update path rechecks the revision first.

- The revision token is whatever `IssueStore.Revision` returns. For GitHub it is `updatedAt` or an equivalent opaque
  value. The package only compares it for equality.
- If the token changed, `Commit` does not write. It calls `Router.RouteToIntake`, which re-plans the human's edit
  (§9). If that returns an error it calls `Router.PunchOut` (§14). If both fail it returns `ErrUnroutable`. A nil
  `Router` counts as both failing, so a conflict is never ignored and never overwritten.
- The `Conflict` passed to the router holds the issue number and the observed and current tokens.

### The residual race

`gh` and the issues REST API have no conditional write (no `If-Match`). The recheck narrows the window between the
check and the write to one request round trip. It cannot close it. `updatedAt` also has one-second granularity, so an
edit in the same second as the observed revision does not change the token.

An edit made after the revision check but before the write can be overwritten and lost without detection. The
replacement body carries its own matching hash, so `Classify` returns `valid`, not `mismatch`. Only an edit made
after the write, which changes the canonical body without updating the marker, leaves a stale hash: `Classify` then
returns `mismatch` and the issue is re-routed to the planner (§8.4). The window can cost a lost edit, but it can never
let an unvalidated body pass as `valid`, because any post-write edit is caught on the next read.

## Dependency substitution

Section 7 requires that the number of an earlier issue is written into a later issue's `Dependencies:` line before
the later issue enters its guard chain and is hashed. The hash must cover the text that is actually committed.

#381 has not defined a placeholder convention, so this package does:

- In a proposed chain, a body refers to an earlier proposed issue as `#{key}` on its dependencies line, for example
  `Dependencies: #{schema}, #314`. A key matches `[A-Za-z0-9][A-Za-z0-9_-]*`.
- `#{...}` anywhere else in the body, for example Ruby string interpolation in a code sample, is never touched.

```go
func SubstituteDependencies(body string, numbers map[string]int) (string, error)
```

For each line that contains `dependencies:` in any case, the function takes the text after the first occurrence and
splits it on commas. Every part must be `#<digits>` or `#{key}`. It replaces each placeholder with `#<number>`, drops
duplicate numbers (the first stays) and rewrites the line in the single-line form:

| Before | `numbers` | After |
|--------|-----------|-------|
| `Dependencies: #{schema}, #314` | `schema=7` | `Dependencies: #7, #314` |
| `Dependencies: #{a}, #{b}, #{a}` | `a=7, b=8` | `Dependencies: #7, #8` |
| `Dependencies: #314` | none | `Dependencies: #314` (unchanged, no placeholder) |
| `puts "#{name}"` | none | unchanged, not a dependencies line |

| Error | Cause |
|-------|-------|
| `ErrUnknownPlaceholder` | A `#{key}` has no entry in `numbers`. That is an unknown key, or a forward reference to an item not yet committed. |
| `ErrBadDependencyLine` | A line with a placeholder has a part that is neither `#digits` nor `#{key}`, such as `none`. |

### Why the line must be a single line

The output is the form the watcher's `DependencyParser` reads. Its `Dependencies:` pattern is
`(?i)dependencies:\s*(.+?)(?:\n|$)`, which captures the rest of one line. A `## Dependencies` header followed by a
bullet list has no numbers on the `Dependencies:` line, so the parser finds nothing and the watcher schedules the
issue immediately. Substitution never produces that form. A line with no placeholder is returned as written, so a
body that already uses the wrong form is not repaired.

### Committing a chain

```go
func (c *Committer) CommitChain(ctx context.Context, items []ChainItem, admit Admit) (ChainResult, error)
```

`ChainItem` holds a `Key`, a `Title`, a `stageoutput.Document` and a `Target`. `Admit` is the boundary's wrapper around
`guard.Run`. It receives the exact stage-output text for one item.

For each item, in order, `CommitChain`:

1. Substitutes placeholders in the body using the numbers committed so far, on a copy of the document. The caller's
   item is not changed.
2. Marshals the copy and passes the text to `admit`. The guard chain sees the substituted document.
3. Stops if the guard result is not `OutcomeReadyToCommit`. The guard's `OnFailure` has already acted.
4. Calls `Commit` with the substituted document, so `Mark` hashes the substituted body.
5. Stops if `Commit` did not return `OutcomeCommitted`. Otherwise it records `numbers[item.Key]`.

Substitution never happens after hashing. The test for this checks that the stored hash validates against the
committed body and that `DependencyParser.ParseDependencies` returns the real number from it.

`ChainResult` has three fields:

| Field | Meaning |
|-------|---------|
| `Committed` | The issues written so far, in order, each with its `Key` and `Result`. |
| `StoppedAt` | The index of the item that did not commit, or `-1` if every item committed. |
| `Stopped` | Why a guard rejection or a conflict stopped the chain: the `Admission` result and, for a conflict, the routed `Commit` result. Nil when every item committed. |

A guard rejection or a conflict that is routed successfully is a normal stop. The error is nil and `Stopped` is set.
Anything else stops the chain at that item with a non-nil error and the `ChainResult` so far: a malformed chain
(`ErrInvalid`), a substitution error, an `admit` error, a marshal error, or an error from `Commit`. `StoppedAt` is set
and `Stopped` is nil in that case. Later items are never attempted after a stop, because they may depend on the one
that stopped.

Two checks run before any item is touched. `admit` must not be nil, and two items must not share a non-empty `Key`,
because the later number would silently replace the earlier one. Both return an error matching `ErrInvalid` with
nothing written.

GitHub has no multi-write transaction, so issues already committed are not rolled back. Each is a complete, valid
standalone issue. Resuming a partly committed chain is #381's concern.

### No `gh` adapter in this package

This package defines `IssueStore` and `Router` but does not implement them. A `gh`-backed store would need
`os/exec`, which the import rule forbids, and it has no consumer until the planner emits documents (#381). #381
supplies the adapter. The interface does not change.

## The hash is a routing signal, not a trust control

The hash is a routing and classification signal. It is not a content-trust control. It lets the workflow sort an
issue into `valid`, `none` or `mismatch` and route it. It authenticates the structure of the body for routing and
says nothing about whether the content is correct, safe or approved.

Every issue body is untrusted data, whatever its provenance (§8.5). A `valid` state means "this is
planner-structured, route it as canonical". It does not mean the text is safe to follow as instructions. A planner
issue, a hand-written issue and a maliciously crafted issue are all data.

The package has **no HMAC, no signing key and no secret** (§8.3, §8.5). The reasons are:

- **The hash is tamper-evident, not tamper-proof.** A GitHub issue body is public and editable. Anyone who can edit
  it can recompute the SHA-256 and write a matching marker. The marker's only job is to let the workflow tell a
  committed planner body from a manual one and from one edited afterwards.
- **A forged marker gains nothing.** A valid marker grants trust in the structure for routing. It grants no
  privilege, no auto-merge and no other privileged action. A body crafted to validate is no more powerful than a
  well-formed issue written by hand, which anyone can already do. There is no privilege boundary for a key to
  protect.
- **A stronger mechanism would defend the wrong thing.** Even a keyed MAC or a signature that proved a planner wrote
  the text would not make it safe, because the body is still natural-language content that can carry a
  prompt-injection payload. The defense is that no stage treats issue content as instructions, and the security
  guard screens the body as data on the way through.
- **A wrong answer is cheap.** `none` and `mismatch` both send the issue to the planner. A forged or broken marker
  can at most produce `valid` for a body the planner did not write, and that grants nothing beyond routing it as
  canonical. A stale or edited marker costs a re-plan.

Do not use a `valid` state to grant permissions, skip a guard or treat the body as reviewed. If a future stage makes
a privileged decision because an issue was planner-produced, that decision needs its own protection, and the first
step is still to treat the body as untrusted data.

The import rule backs this up. `TestNoForbiddenImports` forbids `os` and `crypto/hmac`, so the hash code cannot read
an environment variable, a file or a key.

## Go usage

```go
res, err := guard.Run(in, guard.DefaultChain(handler))
if err != nil {
	return err
}

c := &provenance.Committer{Store: store, Router: router}
out, err := c.Commit(ctx, provenance.Request{
	Doc:       doc,
	Title:     title,
	Target:    provenance.Target{}, // create a new issue
	Admission: res,
})
switch {
case errors.Is(err, provenance.ErrNotReady):
	// The guard chain did not return OutcomeReadyToCommit. Nothing was written.
case err != nil:
	// Invalid request, unavailable revision, unroutable conflict or a failed write.
default:
	switch out.Outcome {
	case provenance.OutcomeCommitted:
		// out.Number, out.Hash and out.Body describe what was written. Advance here.
	case provenance.OutcomeRoutedToIntake, provenance.OutcomePunchedOut:
		// The issue changed under the planner. Nothing was written.
	}
}

// Reading: classify any issue body with the same canonicalization.
switch provenance.Classify(body) {
case provenance.StateValid:
	// proceed
case provenance.StateNone, provenance.StateMismatch:
	// planner-process the issue
}
```
