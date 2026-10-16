# Design Spec: `kairon eval audit` — verify load-bearing evidence and reject duplicate instructions

Closes #338

## Problem

`kairon eval audit` (added in #322/#335) counts an instruction as load-bearing when the agent's
`load-bearing.md` table maps it to a rubric criterion. It takes the table on trust:

1. `loadAuditTable` (`internal/eval/audit.go`) never reads the Evidence column. A row may have an empty
   Evidence cell, or map to a criterion that no case of the agent checks, and still count as mapped.
2. `extractInstructions` counts every occurrence of a line. A rule repeated four times is four
   "load-bearing" instructions, although Stage 3 (`stage-3.md` §1) also requires no dead weight.

The agreed design (gap-analysis.md §S3.5 item 8) maps each instruction to a criterion **and to the case
that proves it**. This issue makes the audit check that proof and reject repeated instructions.

Out of scope (per issue): editing any agent prompt; writing any real `.kairon/iterations/<agent>/load-bearing.md`;
rubric-level rules. Consequence: real agents still fail the audit (no tables yet); the only visible change for
them is the extra `duplicates=` field in the report and possible `duplicate` violations.

## Investigation Summary

- `Audit` (`internal/eval/audit.go`) is **stateless**: directories are explicit options and no package state
  (`cfg`) is read. The existing case loader `loadCases` (`runner.go`) uses `evalsPath()` → package-global
  `cfg.evalsDir`, so it **must not be reused**. A small stateless case reader is needed (same reason
  `loadAuditConfig` re-implements `locateAgentConfig`).
- Cases live at `<evals-dir>/cases/<agent>/*.yaml` (non-recursive, `.yaml` only). A case is identified by its
  `name:` field (that is what `--test-case`/`ValidateTestCase` match on), not by its file name. Each check
  carries `criterion:` (`Check.Criterion` in `checks.go`). Judge checks carry a criterion too; **any** check
  type counts as "a check on the criterion".
- `loadAuditTable` currently returns `map[string]auditTableRow` and reports `table` violations through the
  `add` callback with the table line number (`Line` field → rendered `file:LINE:`). Evidence validation fits the
  same callback, so violations print as `<table>:<line>: table: ...` and always appear (they are not
  "details of a coverage failure").
- The rubric's criterion set is already loaded (`loadAuditCriteria`) before the table is parsed.
- `Audit` builds `instructions` in the order prompt → engine skills (first-occurrence order of `resources`);
  "earlier instruction" for the duplicate rule means earlier in that slice.
- Existing audit tests/fixtures use Evidence text like `e`, `x`, `whatever`, and the fixtures have **no**
  `cases/` directory. They all become invalid under the new rule and must be migrated in the same change:
  - `internal/eval/audit_test.go` helper `auditTable` writes `| ... | e |`, and `auditCase.write` creates no
    cases; subtest `"evidence is not verified"` asserts the old behavior and must be replaced.
  - Fixtures `coverage-89`, `coverage-90`, `fence-and-table`, `no-rtcc`, `ok-hooks`, `partial`, `unmapped`,
    `wrong-order`, `candidate` under `internal/eval/testdata/audit/` (layout `evals/` + `iterations/`, agent
    `demo`, criteria `alpha`/`beta`).
  - Report assertions of the form `total=N mapped=M coverage=...` in `audit_test.go` (the new field sits
    between `mapped` and `coverage`); `total=N mapped=M` prefixes in `cmd/kairon/cmd/eval_audit_test.go` keep
    working.
- Template-sync: none of the touched files (`internal/eval/**`, `cmd/kairon/cmd/eval_audit.go`, `docs/`,
  `.kairon/iterations/README.md`) are in `task sync:check`'s list (agents, scripts, themes, evals dir,
  sentinel-protocol skill). No re-sync is needed. `.kairon/evals/` is **not** touched.
- The issue's AC-1 verify command passes the **same** directory as `--iterations-dir` and `--evals-dir` with
  agent `selftest`: `go run ./cmd/kairon eval audit selftest --iterations-dir X --evals-dir X`. So the four new
  fixture sets use a **flat** layout (not the `evals/`+`iterations/` split of the older fixtures):

  ```
  internal/eval/testdata/audit/<fixture>/
    agents/selftest.json            # {"name":"selftest","prompt":"file://./selftest-prompt.md","resources":[]}
    agents/selftest-prompt.md       # RTCC headings + a few instruction lines
    rubrics/selftest.yaml           # criteria alpha, beta
    cases/selftest/<case>.yaml      # minimal cases: name, agent, checks[].criterion
    selftest/load-bearing.md        # the table (iterations dir == evals dir)
  ```

  The agent overlay `<evals-dir>/agents/selftest.json` wins over `.kiro/agents/`, so these run from the repo
  root without touching real agents.

## Solution Approach

### 1. Evidence grammar (AC 1, 2)

An Evidence cell **names a case** with the token `case:<case-name>`; other text in the cell is allowed.

- Token regexp (RE2, no lookbehind): `(?:^|[^A-Za-z0-9_])case:([A-Za-z0-9_.-]+)`; for each match, strip trailing
  `.` and `-` from the captured name (sentence punctuation: `case:foo.` names `foo`), and skip the match if the
  stripped name is empty. The leading-boundary group stops `showcase:x` from matching. Backticks, spaces,
  commas, semicolons, parentheses terminate the name. Case names are matched exactly and case-sensitively.
  A case name that itself ends in `.` or `-` is unsupported (documented).
- A cell may name several cases; each named case is validated; repeats of the same name in one cell are
  validated once.
- Validation applies to **every data row that has a non-empty Instruction and Criterion cell** (3 cells), including
  second rows for an already-seen instruction and rows rejected as conflicting duplicates — "every Evidence cell".
  It is independent of coverage and always reported (like other `table` violations).

Per row, in this order (a row yields at most one violation per problem):

| Problem | Rule | Message (Line = table line) |
|---------|------|-----------------------------|
| no `case:<name>` token | `table` | `row <N>: Evidence must name at least one case of agent "<agent>" as case:<case-name>` |
| named case not among the agent's cases | `table` | `row <N>: evidence names case "<name>" which is not a case of agent "<agent>" (looked in <casesDir>)` |
| case exists but has no check whose `criterion` equals the row's Criterion | `table` | `row <N>: case "<name>" has no check on criterion "<criterion>"` |

Decisions:
- All three use rule `table` (no new rule ID for evidence): they are defects in the table, they name the table
  line, and AC 1 mandates `table` for the first. Messages include the case name, satisfying AC 2.
- The "no check on criterion" test is **skipped when the row's Criterion is not in the rubric**; that row is already
  an `unknown-criterion` problem and a double report is noise. Missing-token and unknown-case are still reported.
- Only checks count, not rubric-level judge scoring: a case whose criterion is scored purely by the LLM judge
  without a `checks:` entry does not prove the criterion (matches the issue text "at least one check").

### 2. Stateless case reader

New in `internal/eval/audit.go` (or `audit_evidence.go`, same package):

```go
// auditCases maps case name -> set of criteria it has at least one check on.
type auditCases struct {
    dir   string                     // <evals-dir>/cases/<agent>, for messages
    names map[string]map[string]bool
}
func loadAuditCases(evalsDir, agent string) (auditCases, error)
```

- Read `<evalsDir>/cases/<agent>/` non-recursively, `*.yaml` only (mirrors `loadCases`). A missing directory
  (`fs.ErrNotExist`) is **not** an error: the agent has no cases, so every Evidence reference fails as
  "not a case".
- Decode each file with plain `yaml.Unmarshal` into `struct{Name string; Checks []struct{Criterion string}}`
  (lenient: unknown fields ignored; the audit must not depend on every runner-only field).
- Malformed YAML or a file with an empty `name:` is a **plain error** (`parsing case <file>: ...`): "anything that
  prevents auditing at all" — same class as a bad rubric. Same name in two files merges criteria.
- `Audit` loads cases right after the rubric criteria and passes `{agent, criteria, cases}` into
  `loadAuditTable`, which gains the validation described above. A missing/unreadable table keeps returning
  `nil` (no evidence validation possible).

### 3. Duplicate rule (AC 3, 4)

After `instructions` is assembled (prompt, then skills), walk it once keeping `first[text] = instruction`
(verbatim trimmed text, the same key used for table matching). For each later instruction whose text is already
in `first`, emit:

`<repeat file>:<repeat line>: duplicate: duplicate of <first file>:<first line>: "<text>"`

- New rule ID `duplicate` (`auditRuleDuplicate`), added to `auditRuleOrder` after `table` and before `unmapped`.
- Every occurrence after the first is its own violation and refers to the **first** occurrence (not the previous
  one). Works across files (prompt vs. skill, skill vs. skill). Files are already ranked for sorting (prompt,
  skills), so the existing sort keeps output stable.
- Always reported (like `structure`, `resource`, `table`) — not gated by the 90% threshold, since "no dead
  weight" is a hard Stage-3 requirement.
- Counting is **unchanged**: `total` and `mapped` still count each occurrence (README: "each occurrence counts").
  The duplicate violation fails the audit on its own, so inflated counts can never produce a pass. The new
  `duplicates` number is the count of duplicate violations (occurrences after the first).
- Thematic breaks (`---` outside front-matter) and other short repeated lines are ordinary instructions and
  are flagged like any other repeat; authors reword or drop them. This is documented, not special-cased.

### 4. Report (AC 4)

Change the `Instructions:` line to
`Instructions: total=%d mapped=%d duplicates=%d coverage=%s (threshold %d%%)`.
`writeAuditReport` gains a `duplicates int` parameter. The `OK ...` success line is unchanged.

### 5. Documentation (AC 5)

- `.kairon/iterations/README.md` "Load-bearing table": replace the "free text, not checked" Evidence description
  with the `case:<case-name>` format, what is verified (exists in `<evals-dir>/cases/<agent>/`, has a check on the
  row's Criterion), the lookup path, the grammar details (boundary, trailing punctuation, multiple cases, other
  text allowed, `|` must be escaped), the **Duplicates** rule, and the corrected "What counts as an instruction"
  bullet ("each occurrence counts separately" stays, but note that a repeat is a `duplicate` violation). The
  example table's Evidence cells must follow the format, and the paired `fence-and-table` fixture is updated so the
  README example still "models" it (fixture gets `cases/demo/*.yaml`).
- `docs/evaluation.md` "Prompt Audit": add the Evidence/`duplicate` rules to the load-bearing bullet, the
  rule-ID table (`table` extended with the three evidence problems; new `duplicate` row), the report format
  (`duplicates=<D>`), update the sample outputs (`total=63 mapped=0 duplicates=<n> ...` — **run the command to get
  real numbers**; `coverage-90`, `coverage-89` samples), the "What is audited" list (cases are now read), the
  determinism sentence ("reads only the agent config, the prompt, the rubric, the **cases** and the table"), and
  the "Verifying the audit" section with the four new fixtures and their exact commands.
- `cmd/kairon/cmd/eval_audit.go` `Long` help: mention that Evidence must name `case:<name>` and that repeated
  instructions fail.

## Relevant Files

| File | Change |
|------|--------|
| `internal/eval/audit.go` | `auditRuleDuplicate`; `auditRuleOrder`; `auditCases` + `loadAuditCases`; evidence validation in `loadAuditTable`; duplicate scan in `Audit`; `writeAuditReport` signature + `duplicates=` |
| `internal/eval/audit_test.go` | Migrate helpers (`auditTable`, `auditCase.write` create a case file with checks on alpha and beta; Evidence `case:basic`); replace `"evidence is not verified"`; add evidence, duplicate, report tests |
| `internal/eval/testdata/audit/{coverage-89,coverage-90,fence-and-table,no-rtcc,ok-hooks,partial,unmapped,wrong-order,candidate}/` | Add `evals/cases/demo/basic.yaml` (name `basic`, checks on `alpha` and `beta`); set every Evidence cell in `iterations/demo/load-bearing.md` to `case:basic` |
| `internal/eval/testdata/audit/evidence-missing/` | **New** flat fixture (agent `selftest`): a row with empty Evidence and a row with Evidence text lacking `case:` |
| `internal/eval/testdata/audit/evidence-unknown-case/` | **New**: Evidence names `case:ghost` which does not exist |
| `internal/eval/testdata/audit/evidence-unchecked-criterion/` | **New**: case exists but only has checks on `alpha`; row criterion is `beta` |
| `internal/eval/testdata/audit/duplicate-line/` | **New**: prompt repeats a rule line (plus a cross-file repeat if feasible with `SkillsFS`; otherwise prompt-only) |
| `cmd/kairon/cmd/eval_audit.go` | `Long` help text |
| `cmd/kairon/cmd/eval_audit_test.go` | CLI tests for the four new fixtures (non-zero exit, message content, `duplicates=`) |
| `.kairon/iterations/README.md` | Evidence format, duplicate rule, example table |
| `docs/evaluation.md` | Audit section updates listed above |

Not touched: `.kiro/**`, `cmd/kairon/templates/**`, `.kairon/evals/**`, any real `load-bearing.md`, any agent
prompt, `CHANGELOG.md`, other specs.

## Team Orchestration

- `audit-evidence` (builder) goes first: it owns the `audit.go` signature changes and the migration of existing
  tests/fixtures; nothing else can build on the old fixtures.
- `audit-duplicates` (builder) depends on it (same two Go files; sequencing avoids merge conflicts and lets it reuse
  the migrated helpers and the flat-fixture helper).
- `cli-tests` (builder) and `docs` (documenter) both depend only on the Go behavior and run in parallel after
  `audit-duplicates`. `docs` needs real command output for samples.
- `validate` (validator) runs last, read-only.

## Step-by-Step Task Breakdown (TDD, per AGENTS.md)

### Task 1: `audit-evidence` — Evidence verification
1. **Write failing tests first** in `audit_test.go` and confirm each fails for the *expected reason* (the audit
   returns nil / passes today, not a compile error — so stub nothing; the tests only call existing `Audit`):
   - `TestAuditEvidence` subtests using `auditCase` (extend `auditCase` with `cases map[string]string` or a
     `caseChecks map[string][]string` field that writes `cases/demo/<name>.yaml`): missing/empty Evidence; text
     without token; `showcase:basic` (boundary) fails; `case:basic.` (trailing dot) passes; `` `case:basic` ``
     passes; several tokens, one bad → one violation naming it; unknown case; case without a check on the row's
     criterion; ghost criterion row (unknown-criterion) does not also report "no check"; repeated identical row
     with bad Evidence is reported on its own line; violations carry `Rule == "table"` and the right `Line`;
     missing `cases/` dir → unknown-case; malformed case YAML → plain (non-`*AuditError`) error; table violations
     appear even when coverage ≥ 90%.
   - `TestAuditEvidenceFixtures` for `evidence-missing`, `evidence-unknown-case`, `evidence-unchecked-criterion`
     (flat-layout helper `auditFlatFixtureOpts(name)` → `EvalsDir == IterationsDir == testdata/audit/<name>`, agent
     `selftest`): error contains `<table path>:<line>: table:` and the case name where applicable. Create the
     fixture directories first so the failure is "audit passes" rather than "file not found".
2. Migrate the helpers/fixtures listed above (so previously-green tests stay green: they will go red only after the
   implementation lands, which is the intended order within one commit — do the migration in the same working
   session, run the full `-run Audit` set at the end).
3. Implement `loadAuditCases`, the token extraction, and validation in `loadAuditTable`; thread `agent`,
   `criteria`, `cases` through. Keep the existing messages unchanged.
4. All of `go test ./internal/eval -count=1` green; `go vet ./internal/eval`.

### Task 2: `audit-duplicates` — Duplicate rule and report field
1. **Failing tests first**: `TestAuditDuplicates` — repeat within prompt (message names repeat `file:line` and
   `duplicate of file:line`), three occurrences (both repeats refer to the first), cross-file prompt↔skill repeat
   via `SkillsFS`, no duplicate across fenced code or headings, duplicate reported even with 100% coverage, repeat
   still counted in `total`/`mapped`; `TestAuditReportDuplicates` — `Instructions: total=N mapped=M duplicates=D
   coverage=...` exact line, `duplicates=0` for clean prompts; fixture test for `duplicate-line` (flat layout,
   exits with `duplicate` violations naming both lines and report containing `duplicates=1`). Confirm they fail
   because `duplicate` violations / the field do not exist yet.
2. Implement `auditRuleDuplicate`, ordering entry, duplicate scan, `writeAuditReport(..., duplicates, ...)`.
3. Update all existing assertions of `total=… mapped=… coverage=…` in `audit_test.go` to the new line.
4. `go test ./internal/eval -count=1` green.

### Task 3: `cli-tests` — CLI behavior and help text
1. Failing tests first in `cmd/kairon/cmd/eval_audit_test.go` driving `eval audit selftest` with
   `--iterations-dir X --evals-dir X` for the four new fixtures (reuse the file's existing runner helper; read it
   first): non-nil error, error text contains the table line / case name / both duplicate locations, stdout
   contains `duplicates=1` for `duplicate-line`. Confirm they fail only if the library change is absent (they
   should be green once tasks 1–2 are in; if the helper cannot take a flat fixture, add a variant).
2. Update `Long` help text in `eval_audit.go`.
3. Manually confirm exit codes with `go run ./cmd/kairon eval audit selftest ...; echo $?` for each fixture.

### Task 4: `docs` — README and evaluation.md
Update `.kairon/iterations/README.md` and `docs/evaluation.md` as specified in "Documentation". Regenerate sample
outputs by running the commands (do not hand-write counts). Non-code change: no test-first requirement, but add
nothing that is not true of the implemented behavior. Verify that every table row of the README example matches
the migrated `fence-and-table` fixture and passes: `go run ./cmd/kairon eval audit demo --evals-dir
internal/eval/testdata/audit/fence-and-table/evals --iterations-dir
internal/eval/testdata/audit/fence-and-table/iterations` exits 0.

### Task 5: `validate` — read-only verification
Run every acceptance command in the issue, the full test suite, lint, and `task sync:check`; check no forbidden
paths changed.

## Acceptance Criteria Traceability

| AC | Covered by |
|----|-----------|
| 1 | Task 1 (`evidence-missing`, `TestAuditEvidence`), CLI test in Task 3 |
| 2 | Task 1 (`evidence-unknown-case`, `evidence-unchecked-criterion`) |
| 3 | Task 2 (`duplicate-line`) |
| 4 | Task 2 (`duplicates=` field), Task 3 |
| 5 | Task 4 |

## Validation Commands

```bash
go build ./...
go vet ./internal/eval ./cmd/...
go test ./internal/eval -run Audit -count=1
go test ./cmd/kairon/cmd -run Audit -count=1
go test ./... -count=1
task lint
task sync:check

# AC 1–4 (each must exit non-zero and name the line/case)
for f in evidence-missing evidence-unknown-case evidence-unchecked-criterion duplicate-line; do
  go run ./cmd/kairon eval audit selftest \
    --iterations-dir internal/eval/testdata/audit/$f --evals-dir internal/eval/testdata/audit/$f; echo "$f exit=$?"
done
```

## Risks / Notes

- Migrating existing fixtures is part of this change; leaving any fixture with old Evidence text will fail the
  older tests. The builder should grep `testdata/audit/**/load-bearing.md` and `audit_test.go` for Evidence cells.
- Existing fixtures may contain repeated lines that would now trip `duplicate` (e.g. `coverage-*` generated
  prompts, `fence-and-table`). Check by running the audit on each; if a fixture's intent is not duplicates, edit
  the fixture text (distinct wording), not the rule.
- Real-agent audits now also print `duplicate` violations if prompts/skills repeat lines (e.g. `builder`'s
  prompt vs. `sentinel-protocol`). That is expected and out of scope to fix (prompts are not edited here).
- `internal/eval` has `dockerfile`/`sandbox` tests that need a daemon; run only the packages named above plus
  `go test ./... ` where the sandbox tests self-skip (per #290).

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "audit-evidence"
    agent: "builder"
    description: "TDD: make `kairon eval audit` verify the load-bearing Evidence column. Write failing tests first (audit_test.go TestAuditEvidence + TestAuditEvidenceFixtures; create the flat-layout fixtures evidence-missing, evidence-unknown-case, evidence-unchecked-criterion under internal/eval/testdata/audit/ with agent `selftest`, agents/, rubrics/, cases/selftest/, selftest/load-bearing.md so --iterations-dir == --evals-dir works) and confirm they fail because the audit passes today. Then implement a stateless loadAuditCases (reads <evals-dir>/cases/<agent>/*.yaml, case id = `name:`, criteria = checks[].criterion; missing dir = no cases; malformed YAML = plain error), the `case:<name>` token extraction (regexp `(?:^|[^A-Za-z0-9_])case:([A-Za-z0-9_.-]+)`, strip trailing `.`/`-`), and per-row validation in loadAuditTable emitting `table` violations at the table line for: no token, unknown case, case with no check on the row's criterion (skipped when criterion is not in the rubric). Migrate the helpers (auditTable, auditCase.write create a case `basic` with checks on alpha and beta, Evidence `case:basic`), replace the 'evidence is not verified' subtest, and migrate the existing fixtures (coverage-89, coverage-90, fence-and-table, no-rtcc, ok-hooks, partial, unmapped, wrong-order, candidate) by adding evals/cases/demo/basic.yaml and setting every Evidence cell to `case:basic`."
    dependencies: []
    acceptance_criteria:
      - "A table row whose Evidence cell has no `case:<name>` token (empty or other text only) yields a `table` violation at the table line: `<table>:<N>: table: row <N>: Evidence must name at least one case of agent \"selftest\" as case:<case-name>`"
      - "A named case that is not among <evals-dir>/cases/<agent>/*.yaml `name:` values yields a `table` violation naming the table line and the case; a named case with no check whose criterion equals the row's Criterion yields a `table` violation naming the line, the case and the criterion"
      - "Other text in the Evidence cell is allowed; `showcase:x` does not count as a token; `case:basic.` and backticked `case:basic` resolve to `basic`; several tokens are each validated once"
      - "Evidence violations are always reported (also when coverage >= 90%) and the audit still returns *AuditError for them; malformed case YAML is a plain error, a missing cases directory means no cases"
      - "Audit stays stateless: no use of package-level cfg / evalsPath / loadCases"
      - "Failing-first evidence: the new tests were observed failing because the audit passed (not a compile error) before the implementation"
      - "All pre-existing audit tests and the migrated fixtures pass; `go test ./internal/eval -count=1` is green"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval"
      - "go test ./internal/eval -run Audit -count=1"
      - "go test ./internal/eval -count=1"
      - "go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/evidence-missing --evals-dir internal/eval/testdata/audit/evidence-missing; test $? -ne 0"

  - id: "audit-duplicates"
    agent: "builder"
    description: "TDD: add the `duplicate` rule and the duplicate count to the report. Write failing tests first (audit_test.go TestAuditDuplicates, TestAuditReportDuplicates, and a fixture test for a new flat-layout fixture internal/eval/testdata/audit/duplicate-line/ with agent `selftest`, a prompt that repeats a rule line, a valid table with `case:` Evidence and a matching case) and confirm they fail because no duplicate violation / `duplicates=` field exists. Then implement auditRuleDuplicate (in auditRuleOrder after `table`, before `unmapped`), a single pass over the assembled instructions (prompt, then engine skills) that records the first occurrence of each trimmed text and emits `<repeat file>:<repeat line>: duplicate: duplicate of <first file>:<first line>: \"<text>\"` for every later occurrence, always reported regardless of coverage; keep total/mapped counting unchanged; change the report line to `Instructions: total=%d mapped=%d duplicates=%d coverage=%s (threshold %d%%)` by adding a duplicates parameter to writeAuditReport; update every existing assertion of the old Instructions line in audit_test.go. If an existing fixture or helper prompt repeats a line unintentionally, reword the fixture, not the rule."
    dependencies: ["audit-evidence"]
    acceptance_criteria:
      - "An instruction whose trimmed text equals an earlier instruction in the prompt or its audited engine skills is a `duplicate` violation positioned at the repeat's file:line whose message names the first occurrence file:line; a third occurrence also names the first occurrence"
      - "Duplicates are reported even when coverage is >= 90% and even when every occurrence is mapped; fenced code and headings never count as duplicates"
      - "total and mapped still count every occurrence"
      - "The report line is `Instructions: total=T mapped=M duplicates=D coverage=P% (threshold 90%)`, and the duplicate-line fixture's report prints duplicates=1 (or the fixture's actual repeat count)"
      - "The duplicate-line fixture set exits non-zero via `go run ./cmd/kairon eval audit selftest --iterations-dir ... --evals-dir ...` and the output names both lines"
      - "Failing-first evidence: new tests were observed failing for the expected reason before implementation"
      - "`go test ./internal/eval -count=1` is green"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval"
      - "go test ./internal/eval -run Audit -count=1"
      - "go test ./internal/eval -count=1"
      - "go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/duplicate-line --evals-dir internal/eval/testdata/audit/duplicate-line; test $? -ne 0"

  - id: "cli-tests"
    agent: "builder"
    description: "Add CLI-level tests in cmd/kairon/cmd/eval_audit_test.go for the four new flat-layout fixtures (evidence-missing, evidence-unknown-case, evidence-unchecked-criterion, duplicate-line) invoked as `eval audit selftest --iterations-dir X --evals-dir X`: assert a non-nil error whose text contains the table line / case name / both duplicate locations and, for duplicate-line, that stdout contains `duplicates=1`. Read the file's existing runAuditFixture helper first and add a flat-layout variant if needed. Update the Long help text of auditCmd in cmd/kairon/cmd/eval_audit.go to state that Evidence must name `case:<case-name>` and that repeated instructions fail. Confirm the exit status of each fixture with `go run ... ; echo $?`."
    dependencies: ["audit-duplicates"]
    acceptance_criteria:
      - "CLI tests exist for all four new fixtures and pass"
      - "Existing CLI audit tests (coverage-90 passes, coverage-89 fails, prompt-file flag, usage silencing) still pass against the migrated fixtures"
      - "Help text of `kairon eval audit --help` mentions case:<case-name> Evidence and the duplicate rule"
    validation_commands:
      - "go build ./..."
      - "go vet ./cmd/..."
      - "go test ./cmd/kairon/cmd -run Audit -count=1"
      - "go run ./cmd/kairon eval audit --help"

  - id: "docs"
    agent: "documenter"
    description: "Update .kairon/iterations/README.md (Load-bearing table section: Evidence column format `case:<case-name>` and what is verified, lookup path <evals-dir>/cases/<agent>/, token grammar notes, Duplicates rule, updated 'What counts as an instruction' wording, example table whose Evidence cells follow the format and match the migrated fence-and-table fixture) and docs/evaluation.md (Prompt Audit: load-bearing bullet, 'reads' sentence now includes cases, extended `table` rule and new `duplicate` rule in the rule-ID table, report format with `duplicates=<D>`, regenerated sample outputs for builder / coverage-90 / coverage-89 by actually running the commands, and a 'Verifying the audit' addition listing the four new fixtures with their exact commands). Do not edit any agent prompt, CHANGELOG.md, or files under .kairon/specs/ other than this spec."
    dependencies: ["audit-duplicates"]
    acceptance_criteria:
      - "README documents the Evidence format (`case:<case-name>`, other text allowed), the verification (case exists, has a check on the row's Criterion) and the duplicate rule, and its example table follows the format"
      - "docs/evaluation.md rule table lists `duplicate` and the evidence problems under `table`, the report format shows duplicates=, and sample outputs match real command output"
      - "The README example rows equal the fence-and-table fixture table and `go run ./cmd/kairon eval audit demo --evals-dir internal/eval/testdata/audit/fence-and-table/evals --iterations-dir internal/eval/testdata/audit/fence-and-table/iterations` exits 0"
      - "No statement in either file still says the audit does not read/check the Evidence column"
    validation_commands:
      - "grep -n 'case:<case-name>' .kairon/iterations/README.md docs/evaluation.md"
      - "grep -n 'duplicate' .kairon/iterations/README.md docs/evaluation.md"
      - "go run ./cmd/kairon eval audit demo --evals-dir internal/eval/testdata/audit/fence-and-table/evals --iterations-dir internal/eval/testdata/audit/fence-and-table/iterations"

  - id: "validate"
    agent: "validator"
    description: "Read-only verification of issue #338: run every acceptance command from the issue, the build, vet, tests, lint and sync check, and confirm that no forbidden path (agent prompts, .kiro/**, cmd/kairon/templates/**, real load-bearing.md, CHANGELOG.md, other specs) was modified."
    dependencies: ["cli-tests", "docs"]
    acceptance_criteria:
      - "AC1: evidence-missing fixture exits non-zero and names the table line"
      - "AC2: evidence-unknown-case and evidence-unchecked-criterion fixtures each exit non-zero and name the line and the case"
      - "AC3: duplicate-line fixture exits non-zero naming the repeat's file:line and the first occurrence"
      - "AC4: duplicate-line report prints duplicates=<n> next to total and mapped"
      - "AC5: README documents the Evidence format and duplicate rule and its example table follows them"
      - "go build, go vet, go test ./internal/eval ./cmd/..., task lint and task sync:check pass"
      - "git diff --name-only shows no changes under .kiro/, cmd/kairon/templates/, .kairon/evals/, .kairon/iterations/*/ (only README.md), CHANGELOG.md"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./internal/eval ./cmd/... -count=1"
      - "task lint"
      - "task sync:check"
      - "for f in evidence-missing evidence-unknown-case evidence-unchecked-criterion duplicate-line; do ! go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/$f --evals-dir internal/eval/testdata/audit/$f || exit 1; done"
      - "git diff --name-only origin/main...HEAD"
```
