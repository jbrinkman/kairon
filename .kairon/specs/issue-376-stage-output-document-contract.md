# Design Spec: Stage-output document contract (strict-schema YAML envelope and body)

Closes #376

Design reference: `.kairon/specs/maturity-model/planner-requirements.md` §3, §3.1.1, §3.3, §3.4, §3.5, principle 12.

## 1. Problem and Goal

No type in the codebase describes what a pipeline stage returns. Today only the architect has a typed, strictly parsed
artifact (the fenced `kiro-plan` block in `internal/plan/parser.go`). This issue adds the pipeline-wide, foundational
stage-output document: a YAML mapping with a metadata envelope and a body, parsed under a strict schema, with a
`result` discriminated union (`canonical` | `punch_out`) and version-scoped strictness.

Delivered in one PR: a new package, its tests, and a `docs/` reference. Nothing consumes the type yet.

Out of scope (do not implement): the guard chain (#377), provenance hashing (#378), converting any stage
(#381, #382), serializing envelope metadata across the inter-workflow boundary (§11.1). The envelope carries no hash
field.

## 2. Solution Approach

### 2.1 Package

New package **`internal/stageoutput`** (import path `github.com/jbrinkman/kairon/internal/stageoutput`).

- Sibling of `internal/plan`, outside `internal/watcher` and `internal/agent`.
- Imports only the standard library and `gopkg.in/yaml.v3` (already in `go.mod`). It must not import any other
  `internal/*` package, so stages and guards can import it without a cycle. A `go list -deps` test enforces this
  (same technique as `internal/checks/deps_test.go`).

### 2.2 Public API

```go
const CurrentSchemaVersion = 1

type Stage string   // planner | architect | krew-lead | builder | validator | documenter
type Result string  // canonical | punch_out
type SourceKind string // github_issue | document

type Source struct { Kind SourceKind; Ref string }
type Inference struct { Slot, Inferred, Evidence string }
type Unresolved struct { Slot, Reason string }

type Document struct {
    SchemaVersion int
    Stage         Stage
    Result        Result
    Source        Source
    Generated     string       // RFC 3339, UTC; kept as a string so it round-trips verbatim
    Body          string       // canonical only
    Unresolved    []Unresolved // punch_out only
    Inferences    []Inference  // optional
}

func Parse(data []byte) (*Document, error)   // version-scoped, strict, validates
func Marshal(d *Document) ([]byte, error)    // validates first, then emits YAML
func (d *Document) Validate() error          // structural checks (shared by Parse and Marshal)

var ErrUnknownVersion = errors.New("unknown schema_version")
var ErrInvalidDocument = errors.New("invalid stage-output document")
type UnknownVersionError struct { Version, MaxSupported int } // Is(ErrUnknownVersion) == true
```

Exported string-typed enums plus `Valid()` helpers (or exported slices of allowed values) so #377 can reuse them.
`Document` carries no struct tags; YAML tags live on the unexported per-version wire structs (2.3).

### 2.3 Version-scoped strict parsing (§3.4)

Parsing is two-phase and dispatches on the declared `schema_version`.

1. **Version probe (non-strict, on purpose).** Decode the data into
   `struct{ SchemaVersion yaml.Node \`yaml:"schema_version"\` }` using a plain decoder (no `KnownFields`). This phase
   must be lenient because a newer document legitimately has keys this parser does not know. Then:
   - key absent, or not a scalar with tag `!!int` (so `"1"`, `1.5`, `true`, `null` are all rejected; this avoids
     yaml.v3 silently truncating a float into an int) → `ErrInvalidDocument`;
   - value `< 1` → `ErrInvalidDocument`;
   - value not in the parser registry → `*UnknownVersionError` (`errors.Is(err, ErrUnknownVersion)`), returned
     **before** any strict decode, so an unexpected key in a newer document surfaces as an unknown-version error,
     never as an "unknown field" error. The message states the declared version and the highest supported one.
2. **Strict decode.** Look the version up in `var versionParsers = map[int]func([]byte) (*Document, error)`. The
   `v1` entry decodes into an unexported wire struct `wireV1` with `yaml.NewDecoder(...)` +
   `decoder.KnownFields(true)`, exactly the pattern at `internal/plan/parser.go` lines 66-71 (keep the same style of
   explanatory comment). `KnownFields(true)` applies to nested mappings (`source`, `inferences[]`,
   `unresolved[]`), so a typo anywhere is an error. After the single `Decode`, decode once more and require `io.EOF`
   to reject multi-document streams (`---` followed by a second document). Empty input → `ErrInvalidDocument`.
   Then convert `wireV1` → `Document` and run `Validate()`.

Adding a key later means: add `wireV2`, register it as version 2, bump `CurrentSchemaVersion`. A documented
forward-compat allowance (§3.4 last bullet) would be an explicit table entry; none exists for v1, so none is added.
Do not add a catch-all `map[string]any` "extensions" field: that would relax strictness, which §3.4 forbids.

### 2.4 Presence versus value (the discriminated union)

The union rules need to distinguish "key absent" from "key present but empty" (`punch_out` with `body: ""` must be
rejected as "forbids body"; `canonical` with `unresolved: []` must be rejected as "forbids unresolved"). So `wireV1`
uses pointers for the variant-specific fields: `Body *string`, `Unresolved *[]wireUnresolved`. An explicit YAML
`null` decodes to a nil pointer and is treated as absent (document this in the doc comment). `Document` itself uses
plain `string`/slices; at the Go level `Validate()` uses `Body != ""` and `len(Unresolved) > 0` as "present".

### 2.5 Validation rules (`Validate`)

All failures wrap `ErrInvalidDocument`; collect every problem with `errors.Join` (or a `ValidationError` holding a
`[]string`) so a guard can report all of them at once. Each message names the field path (e.g.
`unresolved[1].reason`).

| Field | Rule |
|-------|------|
| `schema_version` | integer `>= 1` (a value above `CurrentSchemaVersion` never reaches `Validate`; the probe rejects it) |
| `stage` | one of `planner`, `architect`, `krew-lead`, `builder`, `validator`, `documenter` |
| `result` | `canonical` or `punch_out` |
| `source.kind` | `github_issue` or `document` |
| `source.ref` | non-empty after trimming. yaml.v3 accepts an unquoted `ref: 142` into a string field, which is acceptable; the serializer always emits it quoted because Go marshals the string as `"142"` |
| `generated` | parses with `time.RFC3339` and has a zero UTC offset (§3.1.1: "UTC") |
| `canonical` | `body` present and not whitespace-only; `unresolved` absent (present-but-empty counts as present, so rejected) |
| `punch_out` | `unresolved` present with at least one entry, each with non-empty `slot` and `reason`; `body` absent (even `""` is rejected) |
| `inferences` | optional; each entry needs non-empty `slot`, `inferred`, `evidence`. Allowed on either variant (the issue only restricts it to "when present"); absence is valid |

A missing `inferences` entry field is rejected by `Validate` (empty string) and, because the wire fields are plain
strings under `KnownFields`, a misspelled field is rejected by the decoder.

### 2.6 Serialization

`Marshal` calls `Validate()` first (a producer cannot emit an invalid document), then builds a `yaml.Node` tree (or
uses a `MarshalYAML` on the wire struct) so that:

- keys are emitted in the canonical order from §3.1.1: `schema_version`, `stage`, `result`, `source`, `generated`,
  `body` | `unresolved`, `inferences`;
- `body` uses `yaml.LiteralStyle` (block scalar `|`). yaml.v3's emitter falls back to a quoted scalar when a literal
  block cannot represent the text (e.g. trailing spaces on a line). That is acceptable: the invariant is that
  `Parse(Marshal(d))` returns the identical body, not that the style is always `|`. A test must cover a body with
  colons, quotes, `#`, a leading-space line and a trailing newline, and assert byte-for-byte round-trip;
- `generated` and `source.ref` are strings and are quoted by the encoder when they look like timestamps or numbers,
  which preserves them verbatim;
- `SchemaVersion` is written as the document's own value (so Marshal of a v1 doc emits `schema_version: 1`);
  `Marshal` rejects a document whose `SchemaVersion` is not a registered version.

Round-trip normalization note for the builder: a body like `"a\n"` must come back as `"a\n"`. Verify against yaml.v3
behaviour with chomping (`|`, `|-`, `|+`) via the round-trip test and let the test decide; if yaml.v3 does not
preserve some edge (e.g. a body that is only newlines), reject it in `Validate` as whitespace-only rather than
special-casing the emitter.

### 2.7 JSON-shaped payloads (AC 4)

No special code. A flow-style mapping such as `{"schema_version": 1, "stage": "planner", ...}` is valid YAML and flows
through the same probe and strict decoder. Tests prove both halves: a valid JSON-shaped payload parses, and a
JSON-shaped payload with an unknown key is rejected.

### 2.8 Errors consumers can branch on

- `errors.Is(err, ErrUnknownVersion)` / `errors.As(err, &*UnknownVersionError)` → route as unprocessable input (the
  punch-out path in §14, built by #377/#382; not implemented here).
- `errors.Is(err, ErrInvalidDocument)` → structural failure (missing key, bad enum, union violation).
- Anything else wrapped from yaml.v3 (`failed to parse stage-output YAML: %w`) → syntax errors and unknown or
  misspelled keys. Unknown keys are reported by yaml.v3 as `field X not found in type ...`. Wrap them so they also
  satisfy `ErrInvalidDocument` (`fmt.Errorf("%w: ...: %w", ErrInvalidDocument, err)`; Go 1.20+ supports multiple
  `%w`), giving callers one class for "this document is malformed" and a distinct class for "unknown version".

## 3. Relevant Files

Create:

| Path | Purpose |
|------|---------|
| `internal/stageoutput/doc.go` | Package comment: purpose, "no other internal imports" rule, pointer to `docs/stage-output.md` |
| `internal/stageoutput/types.go` | `Document`, enums, `Source`, `Inference`, `Unresolved`, `CurrentSchemaVersion`, error vars/types |
| `internal/stageoutput/validate.go` | `Validate` and enum helpers |
| `internal/stageoutput/marshal.go` | `Marshal` and key ordering / literal style |
| `internal/stageoutput/parse.go` | `Parse`, version probe, `versionParsers` registry |
| `internal/stageoutput/v1.go` | `wireV1` struct and its conversion to `Document` |
| `internal/stageoutput/validate_test.go` | Union table test, inferences, enums, source, generated |
| `internal/stageoutput/marshal_test.go` | Round trip, key order, block-scalar body |
| `internal/stageoutput/parse_test.go` | Strictness, version scoping, JSON-shaped, multi-doc, empty input |
| `internal/stageoutput/deps_test.go` | `go list -deps` asserts no `internal/*` imports other than itself |
| `internal/stageoutput/docs_test.go` | The doc exists, names the required terms, and its fenced YAML examples parse |
| `docs/stage-output.md` | The reference (section 5) |

Modify:

| Path | Change |
|------|--------|
| `README.md` | One line in a suitable place linking `docs/stage-output.md` (optional but cheap; keep it to a single sentence) |

Relevant, read-only: `internal/plan/parser.go` (pattern to copy), `internal/plan/types.go`,
`internal/checks/deps_test.go` (dependency-test pattern), `internal/checks/checks_docs_test.go` (docs-test pattern),
`.kairon/specs/maturity-model/planner-requirements.md`.

No template-synchronized files (agents, scripts, themes, evals, sentinel-protocol skill) change, so
`task sync:check` is unaffected. Do not edit anything under `.kairon/specs/` other than this spec (see `AGENTS.md`).

## 4. Team Orchestration

- `core-types-validate-marshal` and `parse-and-version` touch the same package and the second needs the types, so
  they run in sequence: task 1 then task 2.
- `reference-doc` depends on task 2 (its test parses the doc's YAML examples with the real parser). It can run in
  parallel with nothing else; sequence after task 2.
- `validate-complete` (validator, read-only) runs last.
- TDD per `AGENTS.md` for every Go task: write the failing test first, confirm it fails for the expected reason (an
  assertion, not a compile error; for new-API tests, first add the minimal type/function stubs so the test compiles
  and fails on behaviour), then implement, then re-run. Commit tests and implementation together. The commit
  message or PR description names the test seen failing first.
- The doc task is non-code (no test-first requirement), but its small docs test is written alongside it.

## 5. Step-by-Step Task Breakdown

### Task 1: `core-types-validate-marshal`
Create `doc.go`, `types.go`, `validate.go`, `marshal.go`, `v1.go` (wire structs only, with yaml tags) and tests
`validate_test.go`, `marshal_test.go`, `deps_test.go`.

Test-first order:
1. `TestValidate_ResultUnion` (table-driven; covers AC 5): valid canonical, valid punch_out pass; canonical with
   `Unresolved`, punch_out with `Body`, punch_out with empty `Unresolved`, canonical with empty `Body` are rejected,
   each asserting `errors.Is(err, ErrInvalidDocument)` and that the message names the offending field.
2. `TestValidate_Envelope`: each required field missing or invalid (stage not in enum, result not in enum,
   source kind not in enum, empty ref, `generated` not RFC 3339, `generated` with a non-UTC offset, version `0`).
3. `TestValidate_Inferences` (AC 6): canonical with and without `Inferences`; entry missing `slot`, `inferred` or
   `evidence` is rejected.
4. `TestMarshalParse...` is added in task 2 (it needs `Parse`). In this task, `TestMarshal_*` asserts the emitted
   text: key order, `body: |` block scalar for a multi-line body, `unresolved` list shape, `Marshal` of an invalid
   document returns an error and no bytes.
5. `TestNoInternalDeps` (`go list -deps .` contains no `github.com/jbrinkman/kairon/internal/...` other than the
   package itself, and in particular not `internal/watcher` or `internal/agent`).

Acceptance: see the plan block. Dependencies: none.

### Task 2: `parse-and-version`
Create `parse.go` and `parse_test.go`; wire `wireV1` → `Document`.

Test-first order:
1. `TestParse_RoundTrip` (AC 1): build a canonical document (with inferences) and a punch_out document, `Marshal`
   then `Parse`, `assert.Equal` on the whole struct.
2. `TestParse_RejectsUnknownKey` (AC 2): top-level `stagee:` extra key and a misspelled required key (`stagee:`
   instead of `stage:`), plus unknown keys nested in `source`, `inferences[0]` and `unresolved[0]`.
3. `TestParse_BodyVerbatim` (AC 3): a literal block body with colons, quotes, `#`, `key: value` lines, a line that
   looks like YAML (`- item`), blank lines, and a trailing newline; assert equality with the expected string.
4. `TestParse_JSONShaped` (AC 4): valid JSON payload parses and equals the YAML form; JSON payload with an extra key
   is rejected.
5. `TestParse_UnknownVersion` (AC 7): `schema_version: 2` (and `99`) with an extra key `model: x` returns an error
   with `errors.Is(err, ErrUnknownVersion)` true, `errors.As` yields `UnknownVersionError{Version: 2,
   MaxSupported: 1}`, and the error does **not** satisfy `ErrInvalidDocument` and does not mention the unknown key.
   Also the same document at `schema_version: 1` is rejected as an invalid document (unknown key), proving the
   version is what changes the outcome.
6. `TestParse_VersionField`: missing `schema_version`, `"1"` (string), `1.5`, `true`, `null`, `0`, `-1` → all
   `ErrInvalidDocument`; none are `ErrUnknownVersion`. A bare `schema:` key is rejected as unknown under v1.
7. `TestParse_StreamShape`: empty input, whitespace-only, a YAML list at top level, and two `---` documents are all
   rejected.
8. `TestParse_PunchOutExample`: the exact punch_out document from planner-requirements §3.5 (with folded `>-`
   reasons) parses and yields two `Unresolved` entries; and the §3.1.1 canonical example parses.
9. `TestParse_UnquotedScalars`: unquoted `ref: 142` and unquoted `generated: 2026-10-20T20:00:00Z` parse into the
   string fields (yaml.v3 resolves them as int/timestamp and then sets the string); confirm the behaviour and, if
   yaml.v3 does not do this, make the wire fields `yaml.Node`-based or use a small custom unmarshal so the §3.1.1
   example shape (unquoted timestamp) parses. The documented example must parse as written.

Acceptance: see the plan block. Dependencies: Task 1.

### Task 3: `reference-doc`
Create `docs/stage-output.md` and `internal/stageoutput/docs_test.go`; add the README link.

The doc must contain (AC 8):
- Purpose and where it sits (a stage returns a value; the guard validates it; link to planner-requirements §3).
- The document shape and a complete canonical example and a complete punch_out example (fenced ```` ```yaml ````
  blocks that the docs test feeds to `Parse`).
- An envelope field table: `schema_version`, `stage`, `result`, `source` (`kind`, `ref`), `generated`, `body`,
  `unresolved`, `inferences`, with type, required/optional and the allowed enum values.
- The two result variants and the mutual-exclusion rules, including the table of rejected shapes.
- Why YAML is the single required format: architect `kiro-plan` precedent, `gopkg.in/yaml.v3` already a dependency,
  `KnownFields(true)` strict parse, readable multi-line body as a block scalar, a JSON-shaped payload is also valid
  YAML but is not a second format.
- Version-scoped strictness: closed key set per `schema_version`, adding a key means a new version, how a consumer
  sees `ErrUnknownVersion` versus `ErrInvalidDocument`, and that `null` is treated as absent for `body` and
  `unresolved`.
- What is deliberately not in the envelope: the provenance hash (stamped by the guard, #378), and the fact that
  `inferences` is not serialized across the planning-to-implementation boundary (§11.1).
- A short Go usage snippet for `Parse`/`Marshal`/`errors.Is`.

`docs_test.go`: reads `../../docs/stage-output.md`, asserts it contains the required terms (`schema_version`,
`stage`, `result`, `source`, `generated`, `canonical`, `punch_out`, `unresolved`, `inferences`, `KnownFields`,
`yaml.v3`), and that every ```` ```yaml ```` block parses with `Parse` (valid examples). Mark any intentionally
invalid example with a distinct fence info string (for example ```` ```yaml invalid ````) so the test can skip it, or
omit invalid examples as fenced YAML and describe them in the table instead. Dependencies: Task 2.

### Task 4: `validate-complete`
Read-only verification of acceptance criteria 1-8, the constraints, and the project gates.

## 6. Acceptance Criteria Traceability

| AC | Satisfied by |
|----|--------------|
| 1 | Task 1 (type, `Marshal`), Task 2 `TestParse_RoundTrip` |
| 2 | Task 2 `TestParse_RejectsUnknownKey` |
| 3 | Task 2 `TestParse_BodyVerbatim`, Task 1 `TestMarshal_*` (literal block) |
| 4 | Task 2 `TestParse_JSONShaped`; `parse.go`/`v1.go` import `yaml.v3` and call `KnownFields(true)` |
| 5 | Task 1 `TestValidate_ResultUnion`; Task 2 parse-level cases for presence vs empty (`body: ""`, `unresolved: []`) |
| 6 | Task 1 `TestValidate_Inferences`; Task 2 round trip with and without inferences |
| 7 | Task 2 `TestParse_UnknownVersion` |
| 8 | Task 3 `docs/stage-output.md` + `docs_test.go` |
| Constraint: own package, no cycle | Task 1 `TestNoInternalDeps` |
| Constraint: `schema_version`, no `schema` | Task 2 `TestParse_VersionField` |

## 7. Risks and Decisions

- **yaml.v3 scalar coercion (checked against v3.0.1 while writing this spec).** Unquoted `ref: 142` and
  `generated: 2026-10-20T20:00:00Z` decode into `string` fields with the original text. `v: 1.5` and `v: 1.0` decode
  into an `int` field as `1` with no error, which is why the version probe checks the node tag. The §3.1.1 example uses an unquoted timestamp, so Task 2 step 9
  verifies this; the fallback is specified there. For `schema_version` the probe checks the node tag to avoid
  yaml.v3's float-to-int truncation.
- **Block scalar fidelity.** The emitter may pick a quoted style for awkward bodies. Round-trip equality, not the
  literal style, is the invariant (2.6).
- **`inferences` on `punch_out`.** The issue and §3.1.1 describe `inferences` as an optional audit record and do not
  forbid it on punch-out. It is allowed on both variants. This is the one reading of the design doc that the issue
  does not pin down; the doc states it explicitly.
- **`generated` as string, UTC required.** §3.1.1 types it as a string in RFC 3339 UTC. Keeping a string avoids
  `time.Time` re-formatting on round trip and keeps the later hash/diff story (#378) free of timestamp
  normalization.
- **Stage enum is closed.** A new stage requires a code change and, per §3.4, a version bump.
- **No `Extensions` map.** Extensibility is by version, per §3.4.

## 8. Validation Commands

```bash
go build ./...
go vet ./...
go test -race ./internal/stageoutput/...
go test ./...
task lint
task fmt:check
task test
task sync:check
grep -n 'KnownFields(true)' internal/stageoutput/*.go
grep -n 'gopkg.in/yaml.v3' internal/stageoutput/*.go
test "$(go list -deps ./internal/stageoutput | grep -c 'jbrinkman/kairon/internal')" -eq 1
test -f docs/stage-output.md
```

## 9. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "core-types-validate-marshal"
    agent: "builder"
    description: "Create package internal/stageoutput with the Document type, enums, error types, Validate (result discriminated union, envelope, inferences rules), Marshal (canonical key order, literal block-scalar body, validates first), wireV1 structs with yaml tags, doc.go, and tests written first (validate_test.go, marshal_test.go, deps_test.go). Follow TDD per AGENTS.md: confirm each new test fails for the expected reason before implementing."
    dependencies: []
    acceptance_criteria:
      - "Package internal/stageoutput exists, imports only the standard library and gopkg.in/yaml.v3, and a go list -deps test proves no other internal/* package (in particular internal/watcher and internal/agent) is linked"
      - "Document has SchemaVersion, Stage, Result, Source{Kind,Ref}, Generated, Body, Unresolved[]{Slot,Reason}, Inferences[]{Slot,Inferred,Evidence}; ErrUnknownVersion, ErrInvalidDocument and UnknownVersionError (errors.Is(ErrUnknownVersion)) are exported"
      - "Validate table test passes: valid canonical and valid punch_out accepted; canonical with unresolved, punch_out with body, punch_out with empty unresolved, and canonical with no body each rejected with an error satisfying errors.Is(err, ErrInvalidDocument) whose message names the offending field"
      - "Validate rejects stage/result/source.kind outside their enums, empty source.ref, generated that is not RFC 3339 UTC, schema_version below 1, and an inference entry missing slot, inferred or evidence; a document with no inferences is valid"
      - "Marshal validates first (invalid document returns an error and no bytes), emits keys in order schema_version, stage, result, source, generated, body or unresolved, inferences, and writes a multi-line body as a block scalar"
      - "Commit message names the test observed failing first"
    validation_commands:
      - "go build ./internal/stageoutput/..."
      - "go vet ./internal/stageoutput/..."
      - "go test -race ./internal/stageoutput/..."
      - "test -z \"$(gofmt -l internal/stageoutput)\""

  - id: "parse-and-version"
    agent: "builder"
    description: "Implement Parse in internal/stageoutput: non-strict version probe (schema_version must be an !!int node, >= 1), ErrUnknownVersion/UnknownVersionError for versions not in the registry before any strict decode, a versionParsers registry with a v1 entry that decodes into wireV1 using yaml.NewDecoder + KnownFields(true) (copying the pattern in internal/plan/parser.go), pointer fields for body/unresolved so presence is distinguishable from empty, rejection of empty input, non-mapping input and multi-document streams, then conversion to Document and Validate. Tests first: round trip, unknown/misspelled keys (top level and nested), verbatim body, JSON-shaped payload, unknown version with an extra key, bad schema_version values, stream shape, the planner-requirements section 3.5 and 3.1.1 examples, and unquoted ref/timestamp scalars."
    dependencies: ["core-types-validate-marshal"]
    acceptance_criteria:
      - "Marshal then Parse of a canonical document (with inferences) and of a punch_out document returns a Document equal to the original (schema_version, stage, result, source.kind, source.ref, generated, body, unresolved, inferences)"
      - "A top-level extra key (e.g. stagee:) or a misspelled required key, and an unknown key nested in source, inferences[] or unresolved[], each return a non-nil error that satisfies errors.Is(err, ErrInvalidDocument)"
      - "A literal block body containing colons, quotes, #, key: value lines, a '- item' line, blank lines and a trailing newline is preserved byte-for-byte"
      - "A JSON-shaped payload parses to the same Document as its YAML form, and a JSON-shaped payload with an unknown key is rejected"
      - "schema_version 2 or 99 with an extra unknown key returns an error satisfying errors.Is(err, ErrUnknownVersion) and errors.As to UnknownVersionError{Version, MaxSupported: 1}, is not an ErrInvalidDocument, and does not report the unknown key; the same document at schema_version 1 is rejected as an invalid document"
      - "Missing, string, float, bool, null, zero and negative schema_version are ErrInvalidDocument (not ErrUnknownVersion); a bare 'schema:' key is rejected under v1"
      - "Empty input, whitespace-only input, a top-level YAML list, and a multi-document stream are rejected"
      - "canonical with 'unresolved: []' and punch_out with 'body: \"\"' are rejected (presence, not emptiness, decides)"
      - "The section 3.1.1 canonical example and the section 3.5 punch_out example from planner-requirements.md parse as written, including an unquoted timestamp and folded (>-) reasons"
      - "parse.go and v1.go import gopkg.in/yaml.v3 and call KnownFields(true)"
    validation_commands:
      - "go test -race ./internal/stageoutput/..."
      - "go vet ./internal/stageoutput/..."
      - "grep -n 'KnownFields(true)' internal/stageoutput/v1.go internal/stageoutput/parse.go"
      - "grep -n 'gopkg.in/yaml.v3' internal/stageoutput/parse.go"
      - "test -z \"$(gofmt -l internal/stageoutput)\""

  - id: "reference-doc"
    agent: "builder"
    description: "Write docs/stage-output.md (purpose, document shape with a complete canonical and a complete punch_out YAML example, envelope field table, the two result variants and rejected shapes, why YAML is the single required format with the architect kiro-plan precedent, yaml.v3 and KnownFields(true), version-scoped strictness and the ErrUnknownVersion vs ErrInvalidDocument distinction, null-as-absent rule, what is deliberately not in the envelope, a short Go usage snippet) plus internal/stageoutput/docs_test.go that asserts the required terms are present and every fenced yaml example parses with Parse. Add one sentence linking the doc from README.md."
    dependencies: ["parse-and-version"]
    acceptance_criteria:
      - "docs/stage-output.md exists and names schema_version, stage, result, source (kind, ref), generated, body, unresolved, inferences, the canonical and punch_out variants, and the YAML / yaml.v3 / KnownFields(true) decision"
      - "The doc explains why YAML is the single required format (architect kiro-plan precedent, existing go.mod dependency, readable block-scalar body) and that a JSON-shaped payload is valid YAML but not a second format"
      - "The doc states version-scoped strictness (closed key set per schema_version, adding a key is a version bump) and how callers distinguish ErrUnknownVersion from ErrInvalidDocument"
      - "docs_test.go passes: required terms present and every valid fenced yaml example in the doc parses with Parse"
      - "README.md contains a link to docs/stage-output.md"
    validation_commands:
      - "test -f docs/stage-output.md"
      - "go test -race ./internal/stageoutput/..."
      - "grep -n 'stage-output.md' README.md"
      - "grep -n 'KnownFields' docs/stage-output.md"

  - id: "validate-complete"
    agent: "validator"
    description: "Verify issue #376 acceptance criteria 1-8 and constraints (own package outside internal/watcher and internal/agent, schema_version integer with no bare schema key, YAML as the single encoding, version-scoped strictness), and that the repository CI gates pass. Confirm the guard chain, hashing and stage conversions were not implemented, no file under .kairon/specs other than this spec was changed, and no template-synchronized file changed."
    dependencies: ["reference-doc"]
    acceptance_criteria:
      - "Each of issue #376 acceptance criteria 1-8 is demonstrated by a named passing test or by the existing doc file"
      - "go build ./..., task lint, task fmt:check, task test and task sync:check all pass"
      - "go list -deps ./internal/stageoutput shows no internal package other than itself"
      - "git diff against the base shows changes only under internal/stageoutput/, docs/stage-output.md, README.md and this spec"
    validation_commands:
      - "go build ./..."
      - "task lint"
      - "task fmt:check"
      - "task test"
      - "task sync:check"
      - "go test -race -count=1 ./internal/stageoutput/..."
```
