# Design Spec: Audit requires three distinct criteria covered by checks

Closes #344

## Problem Summary

`kairon eval audit <agent>` is the command a Stage 3 promotion runs to prove prompt structure. Stage 3 §2 also
requires at least 3 distinct evaluation criteria, each backed by a substantive check. Nothing enforces that today:

- `loadAuditCriteria` (`internal/eval/audit.go`) only returns the *set of criterion names*. It ignores `type: cost`
  and does not count anything, so a rubric with two real criteria plus a `type: cost` entry passes.
- `loadAuditCases` records, per case, only the *set of criteria* it checks. It does not count checks and discards the
  check bodies, so the audit cannot tell that a criterion has no check in any case, or that two criteria carry
  identical checks.

#338 (closed, merged as `312a83f`) already added the case loader and the Evidence checks. This issue builds on that
code and adds three rubric-level rules plus a per-criterion section in the report.

## Solution Approach

All changes are in the existing `Audit` function path. No new files in `internal/eval` production code, no new CLI flag,
no model calls. `loadRubrics` in `runner.go` is **not** touched (explicit out of scope: self-test rubrics may keep fewer
criteria).

### New rules

Three new stable rule IDs (documented in `docs/evaluation.md`). Violations are attributed to the **rubric file**
(`<evals-dir>/rubrics/<agent>.yaml`, no line number), which satisfies "naming the rubric".

| Rule | Fires when | Message (must contain) |
|------|-----------|------------------------|
| `rubric` | The rubric has fewer than 3 distinct non-cost criteria | rubric path (as the violation `File`), the count found, the minimum 3, and the names of the non-cost criteria found |
| `unchecked` | A non-cost criterion has no check in any of the agent's cases | the criterion name in quotes, and the cases directory searched |
| `identical` | Two non-cost criteria have exactly the same checks across all of the agent's cases | both criterion names in quotes |

Definitions:

- **Non-cost criterion**: a rubric criterion whose `type` is not `cost`. (Compare after `strings.TrimSpace`; compare the
  type exactly to `cost`, matching how the runner treats it.) Criteria are de-duplicated by name; first occurrence wins.
  Criteria with an empty name are ignored.
- **Check on a criterion**: an entry in a case's `checks:` list whose `criterion:` equals the criterion name.
  Checks on a name not in the rubric, and checks on a `type: cost` criterion, are ignored by the three new rules.
- **Identical**: for criteria A and B, for **every** case of the agent (cases keyed by `name:`, merged if two files share
  a name, as today), the multiset of A's checks equals the multiset of B's checks, where a check is compared by its full
  contents **excluding the `criterion` field** (same `type` and same fields and values). Both criteria must have at
  least one check; two criteria with no checks are not "identical" (they are `unchecked`). Because a criterion with
  identical checks across all cases is what the issue asks for, a criterion that is checked in a *different set of cases*
  than another is not identical.
- Identical groups of more than two criteria: iterate criteria in rubric order; every criterion after the first member
  of a group gets one `identical` violation naming itself and the first member (mirrors the `duplicate` convention:
  every repeat names the first occurrence). A pair therefore yields exactly one violation naming both.

### Canonical check encoding

`loadAuditCases` today decodes only `criterion`. Extend its per-check decode to also keep the rest of the check:

- Decode each `checks:` entry into `map[string]any` (yaml.v3 yields `map[string]any` for nested maps, so the value is
  JSON-marshalable). Read `criterion` from it (must be a string; otherwise treat as empty, which matches no rubric
  criterion), `delete` the `criterion` key, then `json.Marshal` the remainder. `encoding/json` sorts map keys, so the
  string is canonical regardless of field order in YAML. Do **not** decode into the `Check` struct: that would depend on
  runner-specific fields and silently drop unknown ones, whereas the audit rule says "same type and fields".
- Store per case: `criterion -> []string` (canonical encodings in file order). Sort the slice before comparing so check
  order inside a case does not matter.
- Signature of a criterion = for each case name in sorted order, `name + "\x00" + join(sorted(encodings), "\x01")`,
  including only cases where the criterion has >= 1 check. Two criteria are identical iff their signatures are equal and
  non-empty. Use a `map[string][]string` signature -> criteria to group.

Edge: YAML `allow: [README.md]` vs. `allow: ["README.md"]` produce the same canonical JSON. `expect_exit: 0` written
explicitly vs. omitted are different checks (documented; no defaulting is applied).

### Data structure changes in `audit.go`

- `loadAuditCriteria(rubricPath)` returns `([]auditCriterion, error)` instead of `map[string]bool`, where
  `auditCriterion{name string; cost bool}` is in rubric order and de-duplicated. Keep the "has no criteria" plain error.
  Callers that need the name set (`criteria[row.criterion]`, `checkAuditEvidence`) build a `map[string]bool` from it once
  (a small helper `auditCriterionNames`), so Evidence/table behaviour is unchanged.
- `auditCases.names` changes from `map[string]map[string]bool` to `map[string]map[string][]string`
  (case -> criterion -> canonical encodings). `checkAuditEvidence` replaces `checked[criterion]` with
  `len(checked[criterion]) > 0`. A case with `checks: []` or no `checks:` is still registered (empty inner map), as today.
- New function `checkAuditRubric(add, rubricPath string, criteria []auditCriterion, cases auditCases) []auditCriterionStat`
  (name is a suggestion) that:
  1. builds the non-cost list; emits `rubric` violation if `< auditMinCriteria` (new const `= 3`);
  2. computes `checks` (total number of checks) and `cases` (number of distinct cases with >= 1 check) per non-cost
     criterion, emitting `unchecked` for each with `checks == 0`;
  3. groups the criteria that have checks by signature and emits `identical` violations;
  4. returns the stats in rubric order for the report.
  All violations use `File = rubricPath`, `Line = 0`.
- Register `rubricPath` in the `rank` map (after the table, before the config) so output ordering stays stable; add the
  three rule IDs to `auditRuleOrder` (`rubric`, `unchecked`, `identical` after `coverage`).
- `writeAuditReport` gains the criterion stats parameter and prints (after the `Instructions:` line):

  ```
  Criteria: non-cost=<N> (need at least 3)
    <name>: checks=<C> cases=<K>
  ```

  one indented line per non-cost criterion in rubric order. Printed whether the audit passes or fails (the report always
  goes to stdout). Cost criteria are not listed.

### Rubric-level rules are independent of coverage

Like `duplicate`, the three rules are always reported regardless of coverage percentage, and do not change `total`,
`mapped` or coverage counting.

### Existing fixtures and helpers must change (important)

Every existing audit fixture and the `auditCase` test helper use a rubric with exactly **two** criteria (`alpha`, `beta`)
whose case checks are textually identical (`type: changed_files`, `allow: [README.md]`). Under the new rules all of them
would start failing with `rubric` and `identical` violations, including the passing ones (`coverage-90`, `ok-hooks`,
`candidate`, `fence-and-table`) and the exact-message tests (`TestAuditEvidence*`, `TestAuditDuplicate*`,
`TestAuditPlainErrors`, cmd tests).

Plan: first make the existing fixtures/helper *compliant* (add a third criterion `gamma`; give each criterion a distinct
check, e.g. `allow: [alpha.md]`, `allow: [beta.md]`, `allow: [gamma.md]`). That change passes against the current audit
(characterization step) and does not alter any existing assertion, because tables only map to `alpha`/`beta` and the
Evidence cells name `case:basic`, which will check all three. Then add the new rules. This keeps every commit green.

Details for the compliant baseline:

- `auditDefaultRubric` in `audit_test.go`: add `gamma`.
- `auditCase.write`: default `caseChecks` becomes `{"basic": {"alpha","beta","gamma"}}`; each generated check uses
  `allow: [<criterion>.md]` instead of `[README.md]` so criteria are distinct. Tests that pass a custom `caseChecks`
  (e.g. `{"only-alpha": {"alpha"}}`, `{"b-one": {"beta"}, ...}`) and tests that pass a custom `rubric` keep working for
  their own assertions, but the new rules will now also fire on them. For those tests change the assertions from "no
  violations / exact violation list" to the specific rule they are exercising, using the existing `tableViolations` /
  `auditHas` helpers, **or** extend those tests' `caseChecks` with an extra case checking `gamma` (preferred, least
  invasive): e.g. add `"gamma-case": {"gamma"}`. The builder decides per test; the invariant is that no existing
  assertion is weakened.
- Fixture directories (all 17 under `internal/eval/testdata/audit/`): for each `.../rubrics/<agent>.yaml` append the
  `gamma` criterion; for each `.../cases/<agent>/basic.yaml` change the checks to
  `alpha -> allow [alpha.md]`, `beta -> allow [beta.md]`, add `gamma -> allow [gamma.md]`. For
  `evidence-unchecked-criterion/cases/selftest/alpha-only.yaml` keep it checking only `alpha`; `basic.yaml` still checks
  all three, so `gamma` is covered and `beta` is not covered by `alpha-only` (the existing assertion). Do not change any
  table or prompt file.
- The live-repo `.kairon/evals/rubrics/*.yaml` are not touched.

### New fixtures (flat layout, agent `selftest`, like the `evidence-*` and `duplicate-line` fixtures)

Each new directory is self-contained: `agents/selftest.json`, `agents/selftest-prompt.md` (RTCC headings plus a few
instructions), `rubrics/selftest.yaml`, `cases/selftest/*.yaml`, `selftest/load-bearing.md` mapping **every**
instruction (so coverage passes and the only failure is the rule under test; copy the structure of `duplicate-line` minus
the duplicated line). Evidence cells use `case:basic`.

| Fixture | Rubric | Cases | Expected |
|---------|--------|-------|----------|
| `two-criteria` | `alpha`, `beta`, and `cost` (`type: cost`) | `basic` checks alpha and beta (distinct) | exit non-zero; `rubric` violation on `…/rubrics/selftest.yaml` naming rubric, count 2 |
| `unchecked-criterion` | `alpha`, `beta`, `gamma` | `basic` checks alpha and beta only | exit non-zero; `unchecked` violation naming `gamma`; no other new-rule violation |
| `identical-criteria` | `alpha`, `beta`, `gamma` | `basic` has alpha, beta and gamma where `beta` and `gamma` have the same check body (`type: changed_files`, `allow: [shared.md]`), alpha differs | exit non-zero; one `identical` violation naming `"beta"` and `"gamma"` |
| `three-criteria-ok` (passing fixture for AC4) | `alpha`, `beta`, `gamma`, and a `type: cost` criterion `cost` | `basic` with 2 checks on alpha, 1 on beta, 1 on gamma; second case `second` with 1 more check on alpha | exit 0; report lists alpha `checks=3 cases=2`, beta `checks=1 cases=1`, gamma `checks=1 cases=1`; no cost line |

In `two-criteria`, the table must only map to `alpha`/`beta`. The cost criterion must have no checks (so `unchecked`
is not triggered by it — and the rule skips cost anyway).

A passing fixture may reuse `coverage-90` instead of `three-criteria-ok` for AC4, but a dedicated fixture with
non-trivial counts (`checks=3 cases=2`) proves the two counts differ, so it is included.

### CLI

`cmd/kairon/cmd/eval_audit.go`: extend the `Long` help text with one paragraph: the rubric must have at least three
non-cost criteria, each checked by at least one case, and no two criteria may carry identical checks. No flag changes.
`cmd/kairon/cmd/eval_audit_test.go`: add table-test entries for the three failing fixtures and the passing one.
(These cmd-test fixtures are shared with `internal/eval`, via `auditFixtures`.)

### Documentation

`docs/evaluation.md` "Prompt Audit (`kairon eval audit`)":

- Update the lead paragraph ("checks three things") to four, adding **Distinct criteria**.
- "What is audited": note the rubric criteria's `type` is now read and case check bodies are read (the sentence "reads
  only these two fields" in the cases bullet must be corrected: it now also reads each check's remaining fields to
  compare them, still runs nothing).
- New subsection "Distinct criteria" defining non-cost, the minimum of 3, `unchecked`, `identical`, the equality
  definition, and the note that the audit does not judge semantic difference beyond checks.
- Rules table: add `rubric`, `unchecked`, `identical`.
- "Report format": add the `Criteria:` block and per-criterion `checks=`/`cases=` lines.
- "Verifying the audit": the baseline description now says criteria `alpha`, `beta`, `gamma` with distinct checks (update
  the sentence at the start of the section and the "Evidence and duplicate fixtures" paragraph, which says criteria
  `alpha` and `beta`); the expected outputs of the `coverage-90` run now include the `Criteria:` lines; add the four
  new fixtures to the fixture table with their commands and expected violation text, including the AC1 command
  (`--iterations-dir` and `--evals-dir` both `internal/eval/testdata/audit/two-criteria`).
- Update the "Running it on the real agents" example outputs so they match real output after the change: real agents
  still fail; the `Instructions:` line is followed by a `Criteria:` block. Regenerate by actually running
  `go run ./cmd/kairon eval audit builder` and pasting the real output (keep the `Instructions:` numbers current).
- Note in "Plain errors" that a rubric with no criteria still stops the audit with an error; a rubric with 1 or 2
  non-cost criteria is a `rubric` violation, not a plain error.

`.kairon/iterations/README.md`: no change required (it does not describe rubric rules). Optionally add one sentence
pointing to the new rules; not required.

## Relevant Files

Modify:
- `internal/eval/audit.go` — rule IDs/order, `auditCriterion`, `loadAuditCriteria`, `loadAuditCases` (canonical checks),
  `checkAuditEvidence`, new `checkAuditRubric`, `Audit`, `writeAuditReport`.
- `internal/eval/audit_test.go` — helper baseline (3 criteria, distinct checks) and new tests.
- `internal/eval/testdata/audit/*/` — update the 17 existing fixture rubrics/cases (see above).
- `cmd/kairon/cmd/eval_audit.go` — help text.
- `cmd/kairon/cmd/eval_audit_test.go` — new fixture cases.
- `docs/evaluation.md` — docs as above.

Create:
- `internal/eval/testdata/audit/two-criteria/`
- `internal/eval/testdata/audit/unchecked-criterion/`
- `internal/eval/testdata/audit/identical-criteria/`
- `internal/eval/testdata/audit/three-criteria-ok/`

Read-only reference:
- `internal/eval/types.go` (`Rubric`, `Criterion.Type`), `internal/eval/checks.go` (`Check` fields; not used for decode),
  `internal/eval/runner.go` (`loadRubrics` — must NOT change).

Template sync: none of these files are template-synchronized (`cmd/kairon/templates/` is not touched), but run
`task sync:check` anyway as part of validation.

## Test-Driven Development (AGENTS.md)

The builder must, for the implementation task, write the failing tests first and run them to see them fail *for the
right reason* (a missing `rubric`/`unchecked`/`identical` violation, not a compile error: add the rule-ID constants and a
stub signature first if needed so the failure is behavioural), then implement, then re-run. The test and implementation
are committed together (no red commit). Name the first-failing tests in the commit message. The fixture-baseline task is
a refactor of test data: it must pass before and after (characterization), and is committed with the implementation or
alone only if green.

Unit tests to add in `internal/eval/audit_test.go` (using the `auditCase` helper, extended with `caseRaw` content or the
existing `rawCases` for full control over check bodies):

1. `TestAuditRubricMinimumCriteria`: 2 non-cost -> `rubric` violation, `File` = rubric path, message has count and
   names; 2 non-cost + 1 `type: cost` -> still violation (cost not counted); 3 non-cost -> no `rubric` violation;
   3 non-cost + cost -> none; duplicate criterion names counted once; coverage-independent (reported even at 100%).
2. `TestAuditUncheckedCriterion`: criterion with no check in any case -> `unchecked` naming it; criterion checked only in
   the second of two cases -> OK; no cases dir -> every non-cost criterion `unchecked`; checks on a criterion not in the
   rubric do not satisfy anything; cost criterion with no checks -> no `unchecked`.
3. `TestAuditIdenticalCriteria`: identical bodies in the single case -> one `identical` violation naming both;
   field order differs in YAML -> still identical; one field differs (`allow` value, `type`, extra field) -> not
   identical; same checks in case 1 but different in case 2 -> not identical; criterion checked in a subset of the cases
   the other is -> not identical; identical but check order within case differs -> identical; three identical ->
   two violations, both naming the first; two criteria with no checks -> `unchecked` only, not `identical`;
   duplicated check count (A has the check once, B twice) -> not identical (multiset).
4. `TestAuditReportCriteria`: report contains `Criteria: non-cost=3` and one `name: checks=C cases=K` line per non-cost
   criterion in rubric order (counts differ for checks vs cases), cost criterion absent; the block is present on failure
   too.
5. Fixture tests (`TestAuditCriteriaFixtures`, flat layout via `auditFlatFixtureOpts`): the four fixtures above.
6. Regression: all pre-existing audit tests still pass with the updated baseline.

## Team Orchestration

- `refresh-fixtures` and `new-fixtures` touch disjoint directories and can run in parallel.
- `implement-rules` depends on both (the new rules would fail unrefreshed fixtures; the new fixtures drive its fixture
  tests). It owns `audit.go` and `audit_test.go` so there are no concurrent edits to those files.
- `update-cli` and `update-docs` both depend on `implement-rules` (docs show real output) and touch disjoint files, so
  they run in parallel.
- `validate-all` runs last, read-only.

## Step-by-Step Task Breakdown

1. **refresh-fixtures** — Bring the existing test helper and 17 fixtures to the 3-distinct-criteria baseline without
   changing any assertion (passes against the current audit).
2. **new-fixtures** — Create `two-criteria`, `unchecked-criterion`, `identical-criteria`, `three-criteria-ok`.
3. **implement-rules** — TDD: add the failing tests, then implement the `rubric`/`unchecked`/`identical` rules, the
   canonical check decode, and the report block in `audit.go`.
4. **update-cli** — Help text and cmd test entries.
5. **update-docs** — `docs/evaluation.md` changes with real captured output.
6. **validate-all** — Run the acceptance commands and the project gates.

```kiro-plan
version: "1.0"
tasks:
  - id: "refresh-fixtures"
    agent: "builder"
    description: "Bring the audit test helper (auditDefaultRubric, auditCase.write) and the 17 existing fixtures under internal/eval/testdata/audit/ to a compliant baseline: add a third criterion gamma to every rubric, add a gamma check to every basic case, and make each criterion's check body distinct (allow: [<criterion>.md]). Do not edit any table or prompt file and do not change any existing assertion; the existing audit must still pass all current tests."
    dependencies: []
    acceptance_criteria:
      - "Every rubrics/*.yaml under internal/eval/testdata/audit/ lists alpha, beta and gamma"
      - "Every basic.yaml has distinct checks for alpha, beta and gamma (different allow lists)"
      - "evidence-unchecked-criterion/cases/selftest/alpha-only.yaml still checks only alpha"
      - "auditDefaultRubric and the default caseChecks in auditCase.write cover alpha, beta and gamma with distinct check bodies"
      - "go test ./internal/eval -run Audit and go test ./cmd/kairon/... pass unchanged against the current audit implementation"
    validation_commands:
      - "go test ./internal/eval -run Audit"
      - "go test ./cmd/kairon/..."

  - id: "new-fixtures"
    agent: "builder"
    description: "Create four flat-layout fixture directories (agent selftest) under internal/eval/testdata/audit/: two-criteria (alpha, beta, plus a type: cost criterion), unchecked-criterion (alpha, beta, gamma; no case checks gamma), identical-criteria (alpha, beta, gamma; beta and gamma have identical check bodies), and three-criteria-ok (alpha, beta, gamma plus a cost criterion; two cases so alpha has 3 checks in 2 cases, beta 1 check in 1 case, gamma 1 check in 1 case). Each has agents/selftest.json, agents/selftest-prompt.md with RTCC headings, rubrics/selftest.yaml, cases/selftest/*.yaml and selftest/load-bearing.md mapping every instruction with Evidence case:basic, modelled on duplicate-line without the duplicated line."
    dependencies: []
    acceptance_criteria:
      - "Four new directories exist with the full flat layout and every prompt instruction mapped in load-bearing.md"
      - "two-criteria rubric has exactly two non-cost criteria and one type: cost criterion"
      - "unchecked-criterion has a rubric criterion (gamma) that no case checks"
      - "identical-criteria has two criteria whose check bodies are identical apart from the criterion field"
      - "three-criteria-ok has check counts alpha=3/2 cases, beta=1/1, gamma=1/1 and a cost criterion with no checks"
    validation_commands:
      - "test -f internal/eval/testdata/audit/two-criteria/rubrics/selftest.yaml"
      - "test -f internal/eval/testdata/audit/unchecked-criterion/rubrics/selftest.yaml"
      - "test -f internal/eval/testdata/audit/identical-criteria/rubrics/selftest.yaml"
      - "test -f internal/eval/testdata/audit/three-criteria-ok/selftest/load-bearing.md"

  - id: "implement-rules"
    agent: "builder"
    description: "TDD per AGENTS.md. First add failing tests in internal/eval/audit_test.go (TestAuditRubricMinimumCriteria, TestAuditUncheckedCriterion, TestAuditIdenticalCriteria, TestAuditReportCriteria, TestAuditCriteriaFixtures) and confirm they fail for the right reason. Then in internal/eval/audit.go: add rule IDs rubric/unchecked/identical and order entries; add auditMinCriteria=3; change loadAuditCriteria to return ordered de-duplicated []auditCriterion with a cost flag; extend loadAuditCases to keep per-check canonical JSON (check body minus criterion, via map[string]any + json.Marshal) per case and criterion; adapt checkAuditEvidence; add checkAuditRubric producing rubric, unchecked and identical violations (File = rubric path, Line 0) and per-criterion stats (checks, cases); register the rubric path in the rank map; print a 'Criteria: non-cost=N (need at least 3)' block with one 'name: checks=C cases=K' line per non-cost criterion in writeAuditReport. Do not touch runner.go loadRubrics. Commit tests and implementation together."
    dependencies: ["refresh-fixtures", "new-fixtures"]
    acceptance_criteria:
      - "Audit fails with rule rubric, File equal to the rubric path, when fewer than 3 non-cost criteria exist; a type: cost criterion does not count"
      - "Audit fails with rule unchecked naming the criterion when a non-cost criterion has no check in any case; cost criteria are never reported"
      - "Audit fails with rule identical naming both criteria when their checks (type and all fields except criterion) are equal across all cases; field order and check order do not matter; criteria with no checks are not reported as identical"
      - "Report prints each non-cost criterion with checks and cases counts, on pass and on fail"
      - "All pre-existing audit tests still pass; loadRubrics in runner.go is unchanged"
      - "The new tests were observed failing for the expected reason before the implementation"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval"
      - "go vet ./internal/eval"
      - "git diff --quiet HEAD -- internal/eval/runner.go || test -z \"$(git diff HEAD -- internal/eval/runner.go | grep loadRubrics)\""

  - id: "update-cli"
    agent: "builder"
    description: "Extend the Long help text of the audit command in cmd/kairon/cmd/eval_audit.go with the three rubric rules (at least three non-cost criteria, each checked by at least one case, no two criteria with identical checks), and add table-test entries in cmd/kairon/cmd/eval_audit_test.go for the two-criteria, unchecked-criterion and identical-criteria fixtures (expect error text naming the rubric / criterion / both criteria) and the three-criteria-ok fixture (expect success and the per-criterion counts in stdout)."
    dependencies: ["implement-rules"]
    acceptance_criteria:
      - "kairon eval audit --help mentions the minimum of three non-cost criteria, the unchecked rule and the identical rule"
      - "cmd tests assert non-nil error naming the rubric for two-criteria, naming gamma for unchecked-criterion and naming beta and gamma for identical-criteria"
      - "cmd test for three-criteria-ok asserts exit success and 'alpha: checks=3 cases=2' in the output"
    validation_commands:
      - "go test ./cmd/kairon/..."
      - "go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/two-criteria --evals-dir internal/eval/testdata/audit/two-criteria; test $? -ne 0"

  - id: "update-docs"
    agent: "documenter"
    description: "Update docs/evaluation.md Prompt Audit section: four checks instead of three; document 'Distinct criteria' (non-cost definition, minimum 3, unchecked and identical rules, equality definition, no semantic judging); correct the statement that the audit reads only two case fields; add rubric, unchecked, identical to the rules table; add the Criteria block and checks/cases counts to the report format; update the verifying-the-audit baseline description (alpha, beta, gamma with distinct checks) and expected outputs by running the real commands; add the four new fixtures with their commands and expected violations including the issue's AC1 command; refresh the 'Running it on the real agents' example output by running kairon eval audit builder; note that 1-2 non-cost criteria is a violation while an empty rubric remains a plain error."
    dependencies: ["implement-rules"]
    acceptance_criteria:
      - "docs/evaluation.md documents the rubric, unchecked and identical rules and the minimum of 3 non-cost criteria"
      - "Report format section shows the Criteria block with checks= and cases= per criterion"
      - "All sample command outputs in the Prompt Audit section were produced by running the commands after the implementation"
      - "The four new fixtures appear in the verification section with their commands"
    validation_commands:
      - "grep -q 'identical' docs/evaluation.md"
      - "grep -q 'unchecked' docs/evaluation.md"
      - "grep -q 'three-criteria-ok' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #344 and the project gates, read-only: run the AC1 command and the unchecked-criterion, identical-criteria fixtures (non-zero exit, correct names in stderr), run the three-criteria-ok fixture (exit 0, per-criterion counts in the report), confirm loadRubrics is unchanged and cost criteria are never counted, run the full test, lint, fmt and template-sync gates."
    dependencies: ["update-cli", "update-docs"]
    acceptance_criteria:
      - "AC1: two-criteria fixture exits non-zero and the error names the rubric file"
      - "AC2: unchecked-criterion fixture exits non-zero and names criterion gamma"
      - "AC3: identical-criteria fixture exits non-zero and names beta and gamma"
      - "AC4: three-criteria-ok report lists every non-cost criterion with its checks and cases counts"
      - "go test ./..., task lint, task fmt:check and task sync:check all succeed"
      - "git diff against the base shows no change to loadRubrics in internal/eval/runner.go"
    validation_commands:
      - "go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/two-criteria --evals-dir internal/eval/testdata/audit/two-criteria; test $? -ne 0"
      - "go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/unchecked-criterion --evals-dir internal/eval/testdata/audit/unchecked-criterion; test $? -ne 0"
      - "go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/identical-criteria --evals-dir internal/eval/testdata/audit/identical-criteria; test $? -ne 0"
      - "go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/three-criteria-ok --evals-dir internal/eval/testdata/audit/three-criteria-ok"
      - "go test ./..."
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
```

## Validation Commands

```bash
# AC1: fewer than 3 non-cost criteria -> non-zero, rubric named
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/two-criteria --evals-dir internal/eval/testdata/audit/two-criteria; echo $?

# AC2: criterion without a check -> non-zero, names gamma
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/unchecked-criterion --evals-dir internal/eval/testdata/audit/unchecked-criterion; echo $?

# AC3: identical criteria -> non-zero, names beta and gamma
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/identical-criteria --evals-dir internal/eval/testdata/audit/identical-criteria; echo $?

# AC4: passing fixture lists every non-cost criterion with checks= and cases=
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/three-criteria-ok --evals-dir internal/eval/testdata/audit/three-criteria-ok; echo $?

# Gates
go test ./... && task lint && task fmt:check && task sync:check
```

## Risks and Notes

- **Blast radius on fixtures.** The new rules would fail every existing 2-criterion fixture and the identical
  `changed_files` bodies. This is why `refresh-fixtures` is a separate, first task and why the spec fixes exact fixture
  edits. Do not weaken existing assertions to make them pass.
- **Real agents.** `kairon eval audit <agent>` on the real agents already fails (no `load-bearing.md` ships). Some real
  rubrics may also trip the new rules; that is intended and is not fixed here (editing real rubrics is out of scope).
  Docs must not claim real agents pass.
- **Self-test rubrics / `kairon eval` runs** keep their current criteria count because `loadRubrics` is untouched.
- **"Distinct" is structural only.** Two criteria with different check bodies pass even if semantically equal; this is
  the stated out-of-scope boundary.
- **Equality granularity.** Defaults are not applied (`expect_exit: 0` vs omitted differ); documented in the docs.
- **Rule names** (`rubric`, `unchecked`, `identical`) are new public identifiers appearing in output; docs list them.
