# Issue 388: Parse-backed check type for planner output-document validity

Closes #388

## Problem

The planner eval suite (#387) scores the `output_document_validity` criterion with regex `output_contains` /
`output_not_contains` checks over the agent's reply. A regex envelope is weaker than the runtime guard:
`generated: "2026-99-21T12:00:00Z"` matches `\d{4}-\d{2}-\d{2}T…Z` but `stageoutput.Parse` rejects it
(`checkGenerated` uses `time.Parse(time.RFC3339, …)`). The only real validation today is
`TestPlannerSuiteStubDocumentsAreValid`, which parses the **stub** replies, never a model reply. None of the 13 check
types runs the parser over the reply.

## Findings from exploring the code

- **The parser.** `internal/stageoutput.Parse([]byte) (*Document, error)` is the single entry point. `parseV1` already
  calls `Document.Validate()` and the wire-level presence checks, so one `Parse` call is the whole contract. The runtime
  guard reaches it through `guard.Input.Document()` → `stageoutput.Parse([]byte(in.Raw))` (internal/guard/guard.go).
  Every rejection matches `stageoutput.ErrInvalidDocument` (or `ErrUnknownVersion`).
- **No import cycle.** `internal/stageoutput` is a leaf (`TestNoInternalDeps` forbids any internal import).
  `internal/checks` only forbids `internal/eval[/…]` (`TestNoHarnessDeps`). So `checks → stageoutput` is allowed and
  adds no cycle. Nothing needs to move.
- **Check plumbing.** A type is: a `CheckType` constant and a `checkSpecs` entry (required/allowed fields) in
  `internal/checks/checks.go`; a `case` in `runCheck` (`checks_eval.go`) that returns `(passed bool, detail string)`;
  a re-export in `internal/eval/checks_alias.go`; an entry in `allCheckTypes` (`checks_docs_test.go`); and a table row
  plus a YAML example (with `checks:`) in `docs/evaluation.md`. `TestChecksDocsCoversEveryCheckType` and
  `TestChecksDocsExamplesAreValid` fail if the docs lag.
- **Per-turn evidence.** `turnEvidence(c, in)` already resolves "this turn's output, or the last turn's when no `turn`".
  The multi-turn document cases (`interactive-return-document`, `multi-issue-split`) end on the turn that matches
  `stop_when: '(?m)^schema_version:'`, so the default last-turn read is the document.
- **Document-returning planner cases** (those with a `schema_version:` stub reply; `wantDocs` in
  `planner_suite_test.go`): `automated-best-effort-fill`, `automated-bug-classification`, `automated-punch-out`,
  `interactive-return-document`, `multi-issue-split`. `interactive-option-rules` and `interactive-single-decision`
  return no document and have no `output_document_validity` checks, so they are untouched.
- **Fenced replies.** The existing envelope regex (`\A\s*(\x60{3}ya?ml\s*\n)?schema_version: 1`), the `judge` question
  and `stripYAMLFence` in `planner_suite_test.go` all tolerate exactly one surrounding ```` ```yaml ```` fence. The
  guard itself parses the raw text with no fence handling.

## Solution approach

Add a deterministic check type **`output_document_valid`** to `internal/checks`.

- **Fields.** No required fields. Optional `turn` (same meaning as on `output_contains`). `pattern` and every other field
  are rejected by the existing `checkSpecs` machinery, so it can never degrade into a regex check.
- **Behaviour.** Read the turn's output via `turnEvidence`. Remove exactly one wrapping `yaml`/`yml` fence if the whole
  reply is that fence (same tolerance as the suite's regex envelope and `stripYAMLFence`; this is transport, not
  document grammar). Call `stageoutput.Parse` on the result. Pass when it returns nil. Fail with
  `Detail = "output is not a valid stage-output document: " + err.Error()`, so the failing reason is the parser's own
  message (for example `generated: "2026-99-21T12:00:00Z" is not an RFC 3339 timestamp`), not a pattern mismatch.
  Empty output, prose, a second YAML document, an unknown key, and a `schema_version` the parser does not know all fail
  because `Parse` rejects them. Nothing re-implements the grammar, the timestamp rule or the variant rules.
- **Scope of the check.** It certifies "the real parser accepts this". Stage name, `result` variant and body rules stay
  with the existing regex checks, which are kept: they give field-level diagnostics and `result`/`stage` expectations
  per case, and AC3 needs them to remain so a malformed document can pass the regex envelope and still be rejected.
- **Planner cases.** In each of the five document-returning cases, add one `output_document_valid` check under
  `output_document_validity`, placed first in that criterion's block (before the regex envelope). The regex envelope
  stays ("in addition to").
- **Negative sample (AC3).** A shipped negative case would make `kairon eval planner` fail, so the proof is a test in
  `internal/eval/planner_suite_test.go`: for each document-returning case, take its stub document, rewrite `generated`
  to `2026-99-21T12:00:00Z`, evaluate the case's `output_document_validity` checks that need no judge, and assert every
  regex check still passes while `output_document_valid` fails with the parser's reason. A companion test asserts every
  document-returning case carries the new check on that criterion (AC2 enforced in CI, not only by `rg`).
- **Docs.** Update `docs/evaluation.md` (type count, table row, section with an example, planner "Offline proof" bullets)
  and the `--output` flag help in `cmd/kairon/cmd/check.go`.

### Decisions and trade-offs

1. **Name `output_document_valid`.** It reads like the other `output_*` types, says what it asserts, and does not clash
   with the criterion name `output_document_validity`.
2. **Fence tolerance.** I keep the suite's existing tolerance of one wrapping `yaml` fence so this check does not become
   stricter than the contract the suite already certifies, and so a model that fences is not suddenly scored 0. The
   residual gap is that the guard, which parses raw text, would reject a fenced reply. No stage calls the guard runner
   yet (docs/guards.md), so there is no live boundary to disagree with. When the boundary is wired it should share this
   unwrap or the planner prompt should forbid fences. Recorded here so it is not lost.
3. **No `stage`/`result` fields on the check.** Not requested, and the regexes already pin them per case. Keeping the
   type field-free keeps the check unable to drift from the parser.
4. **No selftest case.** Self-test coverage is not enforced per type, and new self-test cases change counts asserted by
   many `internal/eval` tests. The unit tests in `internal/checks` and the planner suite tests cover the type.

## Relevant files

Create:
- `internal/checks/checks_document.go` — `evalDocument`, `unwrapYAMLFence`.
- `internal/checks/checks_document_test.go` — unit tests (written first).

Modify:
- `internal/checks/checks.go` — `CheckOutputDocumentValid` constant, `checkSpecs` entry (`allowed: turn`).
- `internal/checks/checks_eval.go` — `runCheck` case.
- `internal/checks/checks_docs_test.go` — add to `allCheckTypes`.
- `internal/eval/checks_alias.go` — re-export `CheckOutputDocumentValid`.
- `internal/eval/planner_suite_test.go` — AC2 and AC3 tests.
- `docs/evaluation.md` — type count (lines ~173, 197 and the two `#the-thirteen-check-types` links), table row,
  section/example, "Offline proof" bullets.
- `cmd/kairon/cmd/check.go` — `--output` flag help mentions the new type.
- `.kairon/evals/cases/planner/{automated-best-effort-fill,automated-bug-classification,automated-punch-out,interactive-return-document,multi-issue-split}.yaml`
- Mirrors: `cmd/kairon/templates/kairon/evals/cases/planner/` (same five files, byte for byte).

Read for reference, do not edit: `internal/stageoutput/*`, `internal/guard/*`, `docs/stage-output.md`, `docs/guards.md`.
Do not edit `.kairon/specs/` other than this file, `.kairon/evals/results/`, `CHANGELOG.md`.

## Team orchestration

```
implement-check-type  ──►  update-planner-cases  ──►  planner-suite-tests  ──►  validate-all
```

The tasks are sequential. `update-planner-cases` and `planner-suite-tests` need the type to exist, and
`planner-suite-tests` asserts what the cases contain. `docs/evaluation.md` is edited by two builders, so they must not
run in parallel. One builder per task, one PR.

AGENTS.md TDD applies. In `implement-check-type`, write `checks_document_test.go` first, run it and confirm it fails
for the expected reason (the type is unknown / not registered), then implement. Commit test and implementation
together. Name the test seen failing first in the commit message.

## Step-by-step task breakdown

### Task 1: implement-check-type (builder)

1. Write `internal/checks/checks_document_test.go` first. Table-driven over `Evaluate` / `evaluateOne` with
   `Input{Output: …}`:
   - well-formed canonical document passes (`Passed == true`, `Detail == ""`);
   - well-formed `punch_out` document passes;
   - the same canonical document wrapped in one ```` ```yaml ```` fence passes;
   - month-99 timestamp (`generated: "2026-99-21T12:00:00Z"`) fails, `Detail` contains `generated` and
     `RFC 3339`, and does **not** contain `pattern /` (parser reason, not a regex mismatch);
   - the month-99 document **matches** the planner envelope regex
     `(?m)^generated: "?\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z"?\s*$`, asserted in the test, to show the point of
     the type;
   - prose, empty output, unknown key, two documents, and `schema_version: 2` each fail with the parser's message;
   - `turn: 1` of a two-turn `Input.Turns` reads turn 1; with no `turn` it reads the last turn; `turn: 3` on two
     turns fails with the out-of-range detail;
   - `Validate` rejects `pattern` and `path` on the type and accepts `turn`; a check with no extra fields validates;
   - `Result` has `Index`, `Type == CheckOutputDocumentValid`, a label of the form `#1 output_document_valid`.
   Run `go test ./internal/checks -run Document` and confirm failure for the expected reason.
2. Implement: constant and `checkSpecs` entry in `checks.go`; `runCheck` case using `turnEvidence`; `evalDocument` in
   `checks_document.go` using `stageoutput.Parse` (import `github.com/jbrinkman/kairon/internal/stageoutput`);
   `unwrapYAMLFence` (single fence only, mirrors `plannerYAMLFence` in `planner_suite_test.go`); add to
   `allCheckTypes`; re-export in `internal/eval/checks_alias.go`.
3. Update `docs/evaluation.md`: "thirteen" → "fourteen" in the field comment, the `### The …check types` heading and the
   two `#the-thirteen-check-types` links (rename the anchor to `#the-fourteen-check-types`); add the table row
   (`output_document_valid` | none | `turn` | the output, with one wrapping `yaml` fence removed, parses with
   `internal/stageoutput.Parse`, the same parser the guard uses); add a short subsection with a valid `checks:` YAML
   example and one sentence on fence tolerance and on the failure detail being the parser's message. Update the
   `--output` flag help in `cmd/kairon/cmd/check.go` to mention the new type and `turn` doc line (`turn` accepted by …)
   in the docs.
4. Confirm `internal/checks` still has no `internal/eval` dependency (`TestNoHarnessDeps`) and `stageoutput` is not
   modified.

### Task 2: update-planner-cases (builder, after Task 1)

In each of the five document-returning cases add, as the first check of the `output_document_validity` block and under
the `# Envelope` comment, with a one-line comment saying it runs the real parser:

```yaml
  - criterion: output_document_validity
    type: output_document_valid
```

Keep all existing regex checks. Do not touch `interactive-option-rules` or `interactive-single-decision`. Then sync:
`mkdir -p cmd/kairon/templates/kairon/evals/cases/ && cp -r .kairon/evals/cases/* cmd/kairon/templates/kairon/evals/cases/`
and run `task sync:check`. Run `go test ./internal/eval -run TestPlannerSuite` and
`go run ./cmd/kairon eval --backend stub planner`.

### Task 3: planner-suite-tests (builder, after Task 2)

In `internal/eval/planner_suite_test.go`:

- `TestPlannerSuiteDocumentCasesUseParseBackedCheck` (AC2): load the cases (as `stubDocuments` does); for every case
  whose stub has a `schema_version:` reply (use `stubDocuments`, do not hard-code a second list), require at least one
  `output_document_valid` check with `criterion: output_document_validity`. For every case without a document,
  require none.
- `TestPlannerSuiteMalformedDocumentFailsParseBackedCheck` (AC3): for each document-returning case, take its stub
  document, replace the `generated:` value with `2026-99-21T12:00:00Z`, and run `checks.Evaluate` over that case's
  `output_document_validity` checks of type `output_contains`, `output_not_contains` and `output_document_valid`
  (skip `judge`, which needs a model) with `Input{Output: mutated}`. Assert that the regex checks all pass, that the
  `output_document_valid` result fails with `Detail` containing `RFC 3339`, and that the unmutated document passes all
  of them. Use a guard so the mutation really changed the text (fail if not).
- Extend the existing GREEN/RED tests only if they break. RED must still hold: legacy transcripts have no document, so
  the new check fails and `output_document_validity` stays below the 60% floor.

Update the "Offline proof" bullets in `docs/evaluation.md` (GREEN now also says each document is accepted by the parse
-backed check; add a bullet for the malformed-timestamp sample).

### Task 4: validate-all (validator, after Task 3)

Read-only. Run the full gate and confirm each acceptance criterion with the commands below. Report any failure with
the exact command and output.

## Acceptance criteria (issue → where verified)

1. New check type runs the parser; a unit test passes a well-formed and a malformed (month 99) document, with the
   failure coming from the parser. → Task 1, `checks_document_test.go`.
2. `output_document_validity` of every document-returning planner case uses it. → Task 2, `rg -n
   "output_document_valid" .kairon/evals/cases/planner` shows one hit in each of the five cases; Task 3 test enforces it.
3. A malformed document that passes the regex envelope is rejected by the new check. → Task 3 test.
4. Edited `.kairon/evals/` files are mirrored into `cmd/kairon/templates/kairon/evals/`. → Task 2, `task sync:check`.
5. Gate clean. → Task 4: `go test -race ./...`, `go vet ./...`, `gofmt -l .` empty,
   `go run ./cmd/kairon eval --backend stub planner` exits 0.

## Risks

- **Check identity in eval diffs/trends** is `(case, criterion, label without index)`. Adding a check to a criterion
  shifts indexes but not the identity of existing checks; the new one shows up as new. Expected.
- **Criterion percentages** change because the denominator grows by one check per case. GREEN stays 100%, RED stays below
  the floor, so the tests that assert them hold.
- **Fence gap** between this check and the raw-text guard (decision 2 above).
- **Windows/sandbox:** the check reads only in-memory output, so it needs no workspace and works natively and in a
  container.

## Validation commands

```bash
go test ./internal/checks -run 'Document|ChecksDocs' -count=1
go test ./internal/eval -run TestPlannerSuite -count=1
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
task sync:check
go run ./cmd/kairon eval --backend stub planner
rg -n "output_document_valid" .kairon/evals/cases/planner
```

## Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-check-type"
    agent: "builder"
    description: "Test-first: write internal/checks/checks_document_test.go (well-formed canonical and punch_out documents pass, a month-99 timestamp document fails with the parser's reason and still matches the planner generated-regex, fence/turn/field-validation cases), confirm it fails for the expected reason, then add the output_document_valid check type in internal/checks (constant, checkSpecs entry allowing only turn, runCheck case, checks_document.go calling stageoutput.Parse after removing one wrapping yaml fence), add it to allCheckTypes, re-export it in internal/eval/checks_alias.go, and update docs/evaluation.md (fourteen types, table row, example, anchors) and the --output flag help in cmd/kairon/cmd/check.go. Commit test and implementation together."
    dependencies: []
    acceptance_criteria:
      - "internal/checks defines CheckOutputDocumentValid = \"output_document_valid\", registered in checkSpecs with no required fields and only turn allowed, and in allCheckTypes"
      - "The check parses with stageoutput.Parse (the guard's parser); no regex re-spells the document grammar; stageoutput is not modified"
      - "A unit test shows a well-formed document passes and a month-99 timestamp document fails with a Detail containing the parser's reason (RFC 3339) and not 'pattern /', while the same malformed document matches the planner generated-regex"
      - "Prose, empty output, unknown key, two documents and schema_version 2 each fail; one wrapping yaml fence is accepted; turn selects the right turn and an out-of-range turn fails"
      - "docs/evaluation.md documents the type (count, table row, valid YAML example) and TestChecksDocsCoversEveryCheckType and TestChecksDocsExamplesAreValid pass"
      - "TestNoHarnessDeps still passes (no internal/eval dependency, no import cycle)"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/checks -count=1"
      - "go test ./internal/stageoutput -count=1"
      - "go vet ./internal/checks ./internal/eval"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "update-planner-cases"
    agent: "builder"
    description: "Add an output_document_valid check as the first check of the output_document_validity criterion in the five document-returning planner cases (automated-best-effort-fill, automated-bug-classification, automated-punch-out, interactive-return-document, multi-issue-split), keeping the regex envelope, then mirror .kairon/evals/cases into cmd/kairon/templates/kairon/evals/cases byte for byte."
    dependencies: ["implement-check-type"]
    acceptance_criteria:
      - "rg -n output_document_valid .kairon/evals/cases/planner shows the new type on the output_document_validity criterion in each of the five document-returning cases"
      - "interactive-option-rules and interactive-single-decision are unchanged"
      - "Existing regex checks in the five cases are retained"
      - "task sync:check passes (templates mirror the edited case files byte for byte)"
      - "go run ./cmd/kairon eval --backend stub planner exits 0"
    validation_commands:
      - "rg -n output_document_valid .kairon/evals/cases/planner"
      - "task sync:check"
      - "go test ./internal/eval -run TestPlannerSuite -count=1"
      - "go run ./cmd/kairon eval --backend stub planner"

  - id: "planner-suite-tests"
    agent: "builder"
    description: "Add planner suite tests in internal/eval/planner_suite_test.go: one asserting every document-returning case (derived from its stub replies) carries an output_document_valid check on output_document_validity and non-document cases do not, and one negative-sample test that rewrites each case's stub document with generated 2026-99-21T12:00:00Z and shows the regex checks still pass while the output_document_valid check fails with the parser's reason (and the unmodified document passes all of them). Update the Offline proof section of docs/evaluation.md."
    dependencies: ["update-planner-cases"]
    acceptance_criteria:
      - "TestPlannerSuiteDocumentCasesUseParseBackedCheck fails if any document-returning planner case lacks output_document_valid on output_document_validity"
      - "TestPlannerSuiteMalformedDocumentFailsParseBackedCheck shows, for every document-returning case, all regex checks of the criterion pass on the malformed document while output_document_valid fails with a detail containing RFC 3339, and the unmodified document passes"
      - "The test fails if the mutation does not change the stub document"
      - "TestPlannerSuiteRedLegacyTranscriptsFail and TestPlannerSuiteGreenReferenceTranscriptsPass still pass"
      - "docs/evaluation.md Offline proof section describes the parse-backed check and the malformed-timestamp sample"
    validation_commands:
      - "go test ./internal/eval -run TestPlannerSuite -count=1 -v"
      - "go test ./internal/checks -run ChecksDocs -count=1"
      - "go vet ./internal/eval"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion in issue 388 and the full gate: tests with -race, vet, gofmt, template sync, and the stub planner eval. Also confirm stageoutput and guard packages are unmodified and no historical artifact (other than the issue-388 spec) was edited."
    dependencies: ["planner-suite-tests"]
    acceptance_criteria:
      - "go test -race ./... passes"
      - "go vet ./... is clean and gofmt -l . prints nothing"
      - "task sync:check passes"
      - "go run ./cmd/kairon eval --backend stub planner exits 0"
      - "rg -n output_document_valid .kairon/evals/cases/planner shows the new type in the five document-returning cases"
      - "git diff shows no change under internal/stageoutput, internal/guard, CHANGELOG.md, .kairon/evals/results or .kairon/specs (other than the issue-388 spec)"
      - "Test and implementation for the new check type are in the same commit, and the commit message names the test seen failing first"
    validation_commands:
      - "go test -race ./..."
      - "go vet ./..."
      - "test -z \"$(gofmt -l .)\""
      - "task sync:check"
      - "go run ./cmd/kairon eval --backend stub planner"
      - "rg -n output_document_valid .kairon/evals/cases/planner"
      - "git diff --stat origin/main -- internal/stageoutput internal/guard CHANGELOG.md"
```
