# Stage-output document

The stage-output document is the value a workflow stage returns. It is a YAML mapping with a small metadata
envelope and a body, parsed under a strict schema. The Go type, parser and serializer live in
[`internal/stageoutput`](../internal/stageoutput). The package imports only the standard library and
`gopkg.in/yaml.v3`, so stages and guards can import it without creating an import cycle.

A stage produces the document, and a guard validates it before the next stage consumes it. This package only
defines the contract: the guard chain, provenance hashing and the conversion of individual stages are separate work.
The design background is in `.kairon/research/maturity-model/planner-requirements.md` (§3, §3.1.1, §3.3, §3.4, §3.5).

## Document shape

Every document has the same top-level keys, in this order:

```text
schema_version
stage
result
source:
  kind
  ref
generated
body            (canonical only)
unresolved      (punch_out only)
inferences      (optional)
```

### Canonical example

A `canonical` document carries the stage's substantive content in `body`.

```yaml
schema_version: 1
stage: planner
result: canonical
source:
  kind: github_issue
  ref: "142"
generated: "2026-10-20T20:00:00Z"
body: |
  # Add retry budget to the watcher

  Goal: cap the number of attempts per issue.

  - max_retries: 3 (from config.yaml)
  - Out of scope: changing the backoff curve
inferences:
  - slot: retry_scope
    inferred: per issue, not per agent
    evidence: "Issue text: 'stop retrying the same issue forever'"
```

### Punch-out example

A `punch_out` document says the stage could not produce a canonical result. It lists what is missing in
`unresolved` and has no `body`.

```yaml
schema_version: 1
stage: planner
result: punch_out
source:
  kind: github_issue
  ref: "143"
generated: "2026-10-20T20:05:00Z"
unresolved:
  - slot: target_branch
    reason: >-
      The issue names two integration branches and does not say which
      one the change should start from.
  - slot: acceptance_criteria
    reason: The issue describes a symptom but no observable pass condition.
```

## Envelope fields

| Field | Type | Required | Values and rules |
|-------|------|----------|------------------|
| `schema_version` | integer | yes | `>= 1` and a version the parser supports (currently `1`). Must be a YAML integer: `"1"`, `1.5`, `true` and `null` are rejected. |
| `stage` | string | yes | `planner`, `architect`, `krew-lead`, `builder`, `validator` or `documenter`. |
| `result` | string | yes | `canonical` or `punch_out`. Selects the variant. |
| `source` | mapping | yes | The input the document was derived from. |
| `source.kind` | string | yes | `github_issue` or `document`. |
| `source.ref` | string | yes | Non-empty after trimming, for example an issue number or a document path. The serializer quotes it when the bare value would resolve to another YAML type (for example `"142"`). |
| `generated` | string | yes | RFC 3339 timestamp in UTC (offset `Z` or `+00:00`). Kept as a string so it round-trips verbatim. |
| `body` | string | `canonical` only | Not blank. Written as a block scalar (`\|`) by `Marshal`. |
| `unresolved` | list | `punch_out` only | At least one entry. Each entry needs a non-empty `slot` and `reason`. |
| `inferences` | list | no | Allowed on either variant. Each entry needs a non-empty `slot`, `inferred` and `evidence`. Absence is valid. |

## Result variants

`result` is a discriminated union. Exactly one of `body` and `unresolved` is present, chosen by `result`. Presence
decides, not emptiness: an empty value counts as present and is rejected where the field is forbidden.

| Shape | Outcome |
|-------|---------|
| `canonical` with a non-blank `body` and no `unresolved` | valid |
| `punch_out` with at least one `unresolved` entry and no `body` | valid |
| `canonical` with no `body`, or a blank `body` | rejected: `body` is required |
| `canonical` with `unresolved` present, including `unresolved: []` | rejected: `unresolved` not allowed |
| `punch_out` with `body` present, including `body: ""` | rejected: `body` not allowed |
| `punch_out` with no `unresolved`, or `unresolved: []` | rejected: at least one entry required |
| `unresolved` entry missing `slot` or `reason` | rejected: names the entry, for example `unresolved[1].reason` |
| `inferences` entry missing `slot`, `inferred` or `evidence` | rejected: names the entry |

A rejected shape looks like this. It uses a distinct fence so the documentation test does not feed it to `Parse`
as a valid example:

```yaml-invalid
schema_version: 1
stage: planner
result: punch_out
source:
  kind: github_issue
  ref: "143"
generated: "2026-10-20T20:05:00Z"
body: |
  A punch_out must not carry a body.
unresolved:
  - slot: target_branch
    reason: Not stated.
```

### Null is absent

An explicit YAML `null` for `body` or `unresolved` is treated the same as leaving the key out. `body: null` on a
`canonical` document fails because `body` is required, and `unresolved: null` on a `canonical` document is valid
because `unresolved` is absent. This differs from `body: ""` and `unresolved: []`, which are present and empty.

## Why YAML is the single required format

A stage-output document is written and parsed as YAML. There is no second required format.

- **Precedent.** The architect already returns a fenced `kiro-plan` block that is YAML, parsed in
  `internal/plan/parser.go`. The stage-output document uses the same encoding and the same strict-decode pattern.
- **No new dependency.** `gopkg.in/yaml.v3` is already in `go.mod`.
- **Readable body.** A multi-line `body` is a block scalar (`|`), so Markdown, colons, quotes and `#` stay readable
  in the file and survive a round trip unchanged. The same text in JSON would be a single escaped line.
- **Strict parsing.** The decoder is `yaml.NewDecoder` with `KnownFields(true)`. An unknown or misspelled key at any
  depth (top level, `source`, `inferences[]`, `unresolved[]`) is an error instead of being silently dropped.

A JSON-shaped payload is valid YAML, because JSON is a subset of YAML. It flows through the same parser and the same
strict decoder, so a producer that emits JSON is accepted. That is a property of YAML, not a second format: there is
no JSON code path, no JSON schema and no JSON-specific validation. A JSON-shaped payload with an unknown
key is rejected like any other.

Input must be exactly one YAML document. Empty input, whitespace-only input, a top-level list and a multi-document
stream (`---` followed by a second document) are rejected.

## Version-scoped strictness

Strictness is scoped to `schema_version`. Each supported version has a closed key set, and the parser for that
version rejects any key outside it. There is no catch-all extensions field. Adding a key is therefore a version bump:
add a new wire struct, register it as the next version, and raise `CurrentSchemaVersion`. Older documents keep
parsing under their own version.

Parsing runs in two phases:

1. **Version probe.** Only `schema_version` is read, and this phase is deliberately lenient about other keys. A newer
   document legitimately has keys this parser does not know. The value must be a YAML integer `>= 1`.
2. **Strict decode.** If the version is supported, the document is decoded with `KnownFields(true)` into that
   version's wire struct, converted to `Document`, and validated.

The probe runs first, so a document from a newer schema is reported as an unknown version and never as an unknown
field.

### Telling the errors apart

| Error | Meaning | Check |
|-------|---------|-------|
| `ErrUnknownVersion` | `schema_version` is a valid integer but not a version this build supports. Returned as `*UnknownVersionError` with `Version` and `MaxSupported`. | `errors.Is(err, ErrUnknownVersion)` |
| `ErrInvalidDocument` | The document is malformed: YAML syntax error, empty input, missing or non-integer `schema_version`, unknown or misspelled key, bad enum value, or a result-union violation. | `errors.Is(err, ErrInvalidDocument)` |

The two never overlap. An unknown-version error does not match `ErrInvalidDocument`, and a malformed document does
not match `ErrUnknownVersion`. A missing, non-integer, zero or negative `schema_version` is `ErrInvalidDocument`,
because there is no version to be unknown. Validation errors list every problem found, each naming its field path.

The same document with an extra key shows the difference. At `schema_version: 2` it is `ErrUnknownVersion` and the
extra key is not mentioned. At `schema_version: 1` it is `ErrInvalidDocument` because of the unknown key.

## Not in the envelope

- **Provenance hash.** The envelope has no hash field. A guard stamps provenance (#378), so a stage cannot supply or
  forge it.
- **Inferences across the planning boundary.** `inferences` is an audit record for the stage that produced it. It is
  not serialized across the planning-to-implementation boundary (planner-requirements §11.1).
- **Extension keys.** There is no free-form field for extra metadata. New fields arrive through a new
  `schema_version`.

## Go usage

```go
import (
	"errors"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

doc, err := stageoutput.Parse(data)
switch {
case errors.Is(err, stageoutput.ErrUnknownVersion):
	var uv *stageoutput.UnknownVersionError
	if errors.As(err, &uv) {
		// uv.Version is what the document declared; uv.MaxSupported is what this build understands.
	}
	return err
case errors.Is(err, stageoutput.ErrInvalidDocument):
	// malformed or structurally invalid; err lists every problem
	return err
case err != nil:
	// unexpected
	return err
}

if doc.Result == stageoutput.ResultPunchOut {
	// doc.Unresolved lists what could not be resolved
}

out, err := stageoutput.Marshal(doc) // validates first, then emits YAML in canonical key order
```
