# Planner Provenance Marker

The planner provenance marker is a single HTML comment line at the end of an issue body. It lets a reader classify a body as planner-generated and unchanged, manually created, or edited after generation.

The hash is weak and has no secret. It detects edits made after generation. It does not prevent forgery, so treat it as a routing hint and never as an authorization check.

The code lives in `internal/issueprov`, which uses only the standard library. It is unrelated to `internal/eval/provenance.go`, which records eval-run provenance.

## Marker format (v1)

```
<!-- planner-provenance: v1 sha256=<hex> generated=<ISO-8601> -->
```

- `v1` names the canonicalization rules the hash was computed under.
- `<hex>` is 64 lowercase hex characters: the SHA-256 of the canonicalized body.
- `<ISO-8601>` is written as RFC 3339 in UTC, for example `2026-10-18T19:15:34Z`. The reader accepts only RFC 3339.
- `generated` is **not hashed**. Re-marking the same body at a later time gives the same `sha256`.

Example for the body `"## Summary\n\nAdd a widget.\n\n## Acceptance Criteria\n1. It works."`, as asserted in the package tests:

```
## Summary

Add a widget.

## Acceptance Criteria
1. It works.

<!-- planner-provenance: v1 sha256=ca7fb2fab6ad52682ff7d2ea9e61817eb9b024304693f88df5665de6b10e3c13 generated=2026-10-18T19:15:34Z -->
```

## Canonicalization

`Canonicalize(body)` applies these steps in order. `Hash(body)` is the lowercase hex SHA-256 of the result.

1. Normalize line endings: `\r\n` becomes `\n`, then any remaining lone `\r` becomes `\n`.
2. Split on `\n`.
3. Drop every marker line. A marker line is any line that starts with `<!-- planner-provenance:` after trimming surrounding whitespace. The test is broad on purpose, so a corrupted marker is still recognized as a marker.
4. Strip trailing whitespace from each remaining line.
5. Join with `\n`, then remove all trailing blank lines. The result has no trailing newline.

Leading blank lines and interior whitespace are kept. Changing either changes the hash.

## Read states

`Classify(body)` returns exactly one of three states.

| State | Value | Condition |
|-------|-------|-----------|
| valid | `StateValid` | Exactly one marker line, strictly well-formed, version `v1`, a `generated` value that parses as RFC 3339, and a `sha256` equal to `Hash(body)`. |
| none | `StateNone` | No line starts with the marker prefix. The issue was created manually. |
| mismatch | `StateMismatch` | A marker line exists but the body is not valid. Causes: the hash differs from the recomputed hash (the body was edited), a malformed marker, an unknown version such as `v2`, an invalid `generated` value, or more than one marker line. |

A well-formed marker matches `^<!-- planner-provenance: (v[0-9]+) sha256=([0-9a-f]{64}) generated=(\S+) -->$` after trimming. Truncated markers, uppercase or wrong-length digests, a missing `generated`, a missing closing `-->`, extra fields and trailing text are all `mismatch`.

Details that follow from the rules:

- Trailing whitespace, CRLF line endings and a final newline do not change the state.
- The marker may sit anywhere in the body. `Append` always puts it last, but a marker moved elsewhere still validates if the rest of the body is unchanged.
- Detection is line-based and not markdown-aware. A line inside a fenced code block that starts with the marker prefix counts as a marker line.
- A marker followed by additional text, for example a note appended after the marker line, is `mismatch`.

## Routing

An issue classified `none` or `mismatch` is re-routed through the planner. It is never rejected. Only `valid` issues skip planner review.

## Package usage

```go
import "github.com/jbrinkman/kairon/internal/issueprov"

// Writer (planner): add or replace the marker. The marker is the last line.
body = issueprov.Append(body, time.Now())

// Reader (watcher): classify an issue body.
switch issueprov.Classify(issue.Body) {
case issueprov.StateValid:
	// planner-canonical and unedited
case issueprov.StateNone, issueprov.StateMismatch:
	// route through the planner
}
```

| Function | Purpose |
|----------|---------|
| `Canonicalize(body string) string` | Returns the v1 canonical form. |
| `Hash(body string) string` | Returns the hex SHA-256 of the canonical form. |
| `MarkerLine(body string, generated time.Time) string` | Returns the marker line without a trailing newline. `generated` is rendered in UTC. |
| `Append(body string, generated time.Time) string` | Removes existing marker lines, trims trailing blank lines, and adds `"\n\n"` plus a fresh marker line. The result has no trailing newline. An empty body yields only the marker line. |
| `Classify(body string) State` | Returns `StateNone`, `StateValid` or `StateMismatch`. |

`Append` is idempotent on the hash. Marking an already marked body replaces the marker and keeps `sha256` unchanged.

## Versioning

The scheme version `v1` travels inside the marker. A future `v2` can change canonicalization without misclassifying old issues. A reader that does not know a version classifies it as `mismatch`, which sends the issue through the planner.

## Status

Only the `internal/issueprov` package exists. Watcher integration and planner emission are not yet implemented: the watcher does not call `Classify`, and the planner does not call `Append`.
