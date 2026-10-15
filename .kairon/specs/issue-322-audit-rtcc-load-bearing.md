# Design Spec: `kairon eval audit` — RTCC structure and load-bearing coverage

Closes #322

## Problem

Maturity-model Stage 3 requires (a) structured prompting (RTCC: Role, Task, Context, Constraints) and
(b) that at least 90% of a prompt's instructions are *load-bearing*, i.e. each serves at least one
evaluation criterion. Nothing measures either today. This issue adds `kairon eval audit <agent>
[--prompt-file <path>]`, a deterministic, model-free check of both, plus the documented
`load-bearing.md` table format it reads.

Out of scope (per issue): editing any agent prompt; writing any real `.kairon/iterations/<agent>/load-bearing.md`.
Consequence: against today's repo every real agent **fails** the audit (planner on structure, all
agents on missing table). That is expected and is what the acceptance "Verify" commands rely on.

## Investigation Summary

- `kairon eval verify-log` (`cmd/kairon/cmd/eval_verify_log.go`, `internal/eval/verifylog.go`) is the
  template to follow: a cobra subcommand of `evalCmd`, `Args: cobra.ExactArgs(1)`, `cmd.SilenceUsage = true`,
  an `--iterations-dir` local flag (default `eval.DefaultIterationsDir`), the persistent `--evals-dir`
  flag inherited from `evalCmd` (`evalEvalsDir`), a library function taking explicit dirs and an
  `io.Writer`, and a multi-line error type (`VerifyLogError`) rendered as `<file>: <rule>: <message>`.
  Reuse `validatePathElement` (verifylog.go) for the agent name, `defaultEvalsDir` (config.go) and
  `DefaultIterationsDir`.
- `--prompt-file` already exists as a **local flag on `evalCmd`** (`evalPromptFile`), not a persistent
  one. `audit` therefore declares its own `--prompt-file` local flag (own variable `auditPromptFile`).
  Do not touch `evalCmd`'s flag.
- Agent configs: `.kiro/agents/<agent>.json` (overlay `<evals-dir>/agents/<agent>.json` wins, as in
  `locateAgentConfig`, which reads the package-global `cfg` and so must **not** be reused here —
  audit must be stateless). The prompt is `"prompt": "file://./<x>-prompt.md"`, resolved relative to the
  config's directory. `resources` is a list of `skill://.kiro/skills/<name>/SKILL.md` strings (and
  possibly other forms; see `agentConfigFile.Resources []json.RawMessage` in provenance.go).
- Today's agents' resources: every agent but `planner` lists `sentinel-protocol` (engine skill) plus
  `<agent>-conventions` (override hook). `planner` lists only `planner-conventions`.
  `cmd/kairon/templates/kiro/skills/` currently ships only `sentinel-protocol` (conventions skills are
  deliberately never shipped — see `builder-conventions`).
- Templates are embedded in `package main` (`cmd/kairon/main.go`: `//go:embed templates`) and handed to
  package `cmd` via `SetTemplates` → `cmd.Templates` (`embed.FS`). `internal/eval` cannot import
  `main`, so the audit takes an `fs.FS` of shipped skills as an option; the CLI passes
  `fs.Sub(Templates, "templates/kiro/skills")`. Tests pass `os.DirFS(<fixture>/skills)`.
- Rubrics: `<evals-dir>/rubrics/<agent>.yaml`, decoded into `Rubric{Agent, Criteria []Criterion}`
  (types.go). `loadRubrics` is global-config based and loads all agents; write a small
  `loadRubricFile(path)` that decodes one file with `yaml.v3` into `Rubric`.
- Existing planner prompt has no `## Role/Task/Context/Constraints` headings (confirmed: headings are
  `ABSOLUTE RESTRICTIONS`, `Critical Rule…`, …), so AC2 holds without any prompt edit.
- No file touched by this issue is template-synchronized (`task sync:check` covers agents, scripts,
  themes, evals, sentinel-protocol only). `.kairon/iterations/README.md` and `docs/evaluation.md` are
  live docs. Still run `task sync:check` at the end. Do **not** edit `.kairon/specs/` other than this
  file, `.kairon/evals/results/`, `CHANGELOG.md`.

## Solution Approach

A new file `internal/eval/audit.go` (pure, stateless library function) and `cmd/kairon/cmd/eval_audit.go`
(thin cobra wrapper), mirroring verify-log.

### Library API (package `eval`)

```go
// AuditOptions configures Audit.
type AuditOptions struct {
    IterationsDir string // "" => DefaultIterationsDir
    EvalsDir      string // "" => defaultEvalsDir
    PromptFile    string // "" => the agent's own prompt; else audit this candidate instead
    SkillsFS      fs.FS  // shipped engine skills, rooted so "<name>/SKILL.md" resolves; nil => no engine skills can be resolved
    Threshold     int    // not exposed; constant auditThresholdPercent = 90
}

// AuditViolation: one failed rule. Rendered "<file>[:<line>]: <rule>: <message>".
type AuditViolation struct { File string; Line int; Rule, Message string }
type AuditError struct{ Violations []AuditViolation }   // Error() = one line per violation

func Audit(w io.Writer, agent string, opts AuditOptions) error
```

`Audit` writes the human report to `w` (stdout) and returns `*AuditError` if any rule fails (cobra then
prints `Error: …` to stderr and `main` exits 1, exactly like verify-log). A non-violation failure
(bad agent name, missing agent config, unreadable/empty `--prompt-file`, missing/invalid rubric) is a
plain `error`.

### Pipeline

1. **Validate** agent name with `validatePathElement`. Resolve dirs (defaults as above).
2. **Locate config**: `<evals-dir>/agents/<agent>.json` if it exists, else `.kiro/agents/<agent>.json`;
   neither → error naming both paths (same wording style as `locateAgentConfig`). Decode with
   `agentConfigFile`.
3. **Prompt source**: if `PromptFile` is set it must be a non-empty regular file (same checks as
   `loadCandidatePrompt`: stat, `IsRegular`, non-empty) — the candidate is audited *instead of* the
   agent's prompt. Otherwise the config's `prompt` must be `file://<rel>`; resolve relative to the
   config's directory; an inline or absolute prompt is an error. The load-bearing table and the rubric
   always belong to the *agent* (not the candidate).
4. **Resources** (engine-skill resolution). For each entry of `resources` that decodes as a string
   starting `skill://`: the skill name is the parent directory element of the URI path
   (`.kiro/skills/<name>/SKILL.md` → `<name>`). Non-string resources and non-`skill://` strings are
   ignored (printed in the report as `Ignored resources:` only if any exist).
   - name ends with `-conventions` → **override hook**: listed under "Override hooks (excluded from
     score)", never read, never scored.
   - else → **engine skill**: read `<name>/SKILL.md` from `SkillsFS`. Found → audited (listed under
     "Audited"). Not found (or `SkillsFS == nil`) → violation `resource`:
     `skill "<name>" is not shipped under cmd/kairon/templates/kiro/skills/ and cannot be audited`.
   - Duplicates in `resources` are processed once (first occurrence order).
5. **RTCC structure** (agent prompt/candidate only, **not** skills): scan level-2 ATX headings outside
   code fences (reuse `h2HeadingRe` semantics from verifylog.go; extract the fence tracker into a
   shared helper rather than duplicating). Heading names are compared exactly and case-sensitively:
   `Role`, `Task`, `Context`, `Constraints`. Extra `##` headings (and other levels) are allowed
   between them. Violation rule `structure`:
   - for each absent heading: `missing required heading "## Context"`, one violation per heading
     (so the planner run names all four);
   - if all are present but not in order (checked as: positions of the first occurrence of each must
     be strictly increasing): `required headings out of order: want Role, Task, Context, Constraints; found <order>`.
   - If some are missing, ordering of the present ones is also checked and reported separately if wrong.
6. **Extract instructions** from the prompt text and from each audited skill. Rules (literal to the
   issue): an instruction is a physical line that, after stripping leading/trailing whitespace, is
   non-empty, is **outside a code fence**, is not an ATX heading (`^ {0,3}#{1,6}([ \t]|$)`), and is not a
   table separator row (a line made only of `|`, `-`, `:` and spaces containing at least one `-` and
   one `|`). Fence tracking follows CommonMark: opening run of ≥3 backticks or tildes (≤3 leading
   spaces), closed by the same character with a run ≥ the opener's length and nothing but spaces after;
   an unclosed fence runs to EOF; the fence lines themselves are not instructions.
   - **Deliberate interpretation (document in docs):** a leading YAML front-matter block (file starts
     with `---` line, ends at the next `---` line) is metadata, not instructions, and is skipped
     for *both* prompts and skills. Without this every `SKILL.md` would need rows for `---` and `name:`.
     Any other `---` line (horizontal rule) is an ordinary instruction line by the literal rule.
   - Each occurrence counts separately (a line repeated 3 times is 3 instructions). Each instruction
     records `(file, line, text)` where `text` is `strings.TrimSpace(line)`. Line endings: normalise
     `\r\n`.
7. **Load-bearing table**: `<iterations-dir>/<agent>/load-bearing.md`.
   - Missing file → violation `table`: `load-bearing table not found` (file path in `File`); every
     instruction then counts as unmapped *in the totals* but **per-instruction `unmapped` violations are
     still emitted** (so the user sees what needs mapping). Keep it simple: treat as empty table.
   - Parse: find the header row whose cells (trimmed, case-sensitive) are exactly
     `Instruction | Criterion | Evidence`. No header → violation `table`: `no "| Instruction | Criterion | Evidence |" header row`.
     Data rows are every subsequent pipe-delimited line (outside code fences) that starts with `|`,
     excluding separator rows. Split cells on unescaped `|`; `\|` inside a cell is unescaped to `|`
     (needed because instructions can themselves contain pipes). Trim each cell. A data row with a
     cell count ≠ 3 → violation `table` (`row N: expected 3 columns, got K`) and is skipped. Rows
     with an empty Instruction or Criterion cell → violation `table` and skipped. The Evidence column
     must exist but its content is **not** verified by the audit (free text; documented as the
     human-checkable justification).
   - Build `map[instructionText]criterionName` (first row wins on duplicate Instruction cells; a
     conflicting duplicate with a different criterion is a `table` violation to avoid silent ambiguity).
8. **Rubric**: `<evals-dir>/rubrics/<agent>.yaml` → set of `criteria[].name`. Missing/invalid → plain
   `error` (cannot audit without it). Empty criteria list → plain error.
9. **Map**: for each instruction (prompt first, then skills in resource order):
   - not in table → violation `unmapped` at `<file>:<line>`: `instruction not in load-bearing table: "<text>"`;
   - in table but criterion ∉ rubric → violation `unknown-criterion` at `<file>:<line>`:
     `maps to criterion "<c>" which is not in rubric <rubric path>: "<text>"`;
   - else mapped (counts toward score). `unknown-criterion` instructions are **not** mapped.
   Matching is exact string equality of `TrimSpace`d text (verbatim); no case folding, no
   whitespace collapsing.
10. **Score**: `total` = all extracted instructions (prompt + audited skills; hooks excluded, ignored
    resources excluded), `mapped` as above. Pass iff `total > 0 && mapped*100 >= 90*total` (integer
    arithmetic — no float rounding, so 89/100 can never pass as "90.0"). `total == 0` fails with rule
    `coverage` (fail closed). Below threshold → violation `coverage`:
    `NN.N% of instructions are load-bearing; need at least 90%`. Display percent as
    `float64(mapped*1000/total)/10` formatted `%.1f` (truncation, never rounds up across the
    threshold). The percent is always printed, pass or fail.
11. **Report** (stdout, always, before returning the error), stable format:

```
Audit: builder
Prompt: .kiro/agents/builder-prompt.md            (or "Prompt: <path> (candidate)")
Audited resources:
  skill sentinel-protocol (N instructions)
Override hooks (excluded from score):
  skill builder-conventions
Instructions: total=120 mapped=90 coverage=75.0% (threshold 90%)
```

   followed, on success, by `OK builder: 120 instructions, 100.0% load-bearing, RTCC structure present`;
   on failure the violations are returned via `AuditError` (stderr `Error:` lines, as verify-log does).
   AC1's verify command therefore prints `skill sentinel-protocol` under *Audited resources* and
   `skill builder-conventions` under *Override hooks* even though it exits non-zero today.
   Violation ordering: by file (prompt, then skills in order, then table), then line, then rule.

### CLI (`cmd/kairon/cmd/eval_audit.go`)

```go
var auditCmd = &cobra.Command{
    Use:   "audit <agent>",
    Short: "Audit an agent prompt for RTCC structure and load-bearing coverage",
    Args:  cobra.ExactArgs(1),
    RunE:  ... cmd.SilenceUsage = true; eval.Audit(cmd.OutOrStdout(), args[0], eval.AuditOptions{...})
}
```

Flags: local `--iterations-dir` (default `eval.DefaultIterationsDir`; own variable `auditIterationsDir`,
*not* shared with verify-log's), local `--prompt-file` (own var), and the inherited persistent
`--evals-dir` (`evalEvalsDir`). `SkillsFS` is built as `fs.Sub(Templates, "templates/kiro/skills")`;
if the sub fails return the error. Register with `evalCmd.AddCommand(auditCmd)` in `init()`.

Careful: `evalCmd` itself has `Use: "eval [agent] [testcase]"` with a `RunE`; cobra resolves the literal
word `audit` to the subcommand first (same as `verify-log`/`diff`), so no change to `evalCmd`. Add a test
asserting `kairon eval audit` routes to `auditCmd` (see existing `TestVerifyLogFlagsRegistered`).

## Relevant Files

Create:
- `internal/eval/audit.go` — library (extraction, RTCC, table parse, scoring, report).
- `internal/eval/audit_test.go` — unit + fixture tests.
- `internal/eval/testdata/audit/…` — fixtures (below).
- `cmd/kairon/cmd/eval_audit.go` — cobra wiring.
- `cmd/kairon/cmd/eval_audit_test.go` — flag registration, routing, end-to-end with fixtures, real-repo AC1/AC2 behaviour.

Modify:
- `.kairon/iterations/README.md` — add a "Load-bearing table" section (AC5).
- `docs/evaluation.md` — add "Prompt Audit (`kairon eval audit`)" section after "Iteration Logs and `verify-log`"; add to Directory Structure / intro cross-references as appropriate.
- `internal/eval/verifylog.go` — *only* if extracting a shared fence-tracking helper (preferred small refactor; verify-log tests must still pass unchanged — it is a refactor, so rely on the existing `verifylog_test.go` as characterization tests).

Read-only references: `internal/eval/provenance.go` (`agentConfigFile`), `candidate.go` (`loadCandidatePrompt` checks), `types.go`, `.kiro/agents/*.json`, `cmd/kairon/templates/kiro/skills/sentinel-protocol/SKILL.md`.

Not touched (explicitly): any `.kiro/agents/*-prompt.md`, any `.kairon/iterations/<agent>/load-bearing.md`, `.kairon/specs/` other than this file, template-synced files.

### Fixture layout `internal/eval/testdata/audit/`

Each case dir is self-contained and is passed as `--evals-dir <case>/evals` and `--iterations-dir <case>/iterations`
(agent config via the evals-dir overlay `evals/agents/`, rubric `evals/rubrics/`), with the skills FS =
`os.DirFS("<case>/skills")` in library tests. The CLI end-to-end tests use the CLI's embedded skills, so
CLI fixtures only use agents whose resources are conventions hooks or `sentinel-protocol`
(the one shipped skill) — use `sentinel-protocol` in the `skills` case so the shipped file's content is
audited; for the rest use agents with no engine-skill resource to keep the counts fixture-controlled.

```
testdata/audit/
  rubric-shared/ (or each case carries its own evals/rubrics/demo.yaml with criteria: alpha, beta)
  ok-hooks/        agent "demo": config has resources [skill://.kiro/skills/demo-conventions/SKILL.md,
                   skill://.kiro/skills/engine-skill/SKILL.md]; skills/engine-skill/SKILL.md present,
                   skills/demo-conventions/SKILL.md present with junk lines (proves hook is not scored)
  no-rtcc/         prompt with none of the four headings
  wrong-order/     Role, Context, Task, Constraints
  partial/         only Role + Task (reports Context and Constraints missing)
  unmapped/        one unmapped line and one line mapped to criterion "ghost" (not in rubric)  -> AC3
  coverage-90/     10 instructions, 9 mapped  -> exits 0   (exactly 90%)
  coverage-89/     100 instructions, 89 mapped -> non-zero (89%)
  fence-and-table/ fenced code, headings, a table separator, a front-matter block and a pipe-containing
                   instruction (`\|` escaped in the table) to pin extraction rules
  candidate/       live prompt unstructured + candidate file passing structure; --prompt-file wins
```

Fixture prompts must contain the four RTCC headings unless the case is about structure, so each case
isolates one rule. `coverage-89` can be generated at checked-in size (100 short lines); it is fine to
write it by script and commit the output.

## Team Orchestration

- Task `audit-core` (builder) is the only task with real design weight; the CLI task depends on its
  API. Docs depend on the CLI (they quote real output) and can start once `audit-core` is merged
  into the branch; to keep it simple they are sequenced after the CLI.
- `docs-audit` (documenter) and the final `validate` (validator) run last. No parallel builder tasks:
  the CLI is ~40 lines and depends on the library signature.
- Everything lands in **one PR**. Follow AGENTS.md TDD: for `audit-core` and `cli-audit`, write the
  failing tests (and fixtures) first, run them and confirm they fail for the expected reason (missing
  symbol → write a minimal stub so the failure is an assertion, not a compile error), then implement,
  and commit tests + implementation together. Name the first-failing test in the commit message /
  PR description. Docs-only changes need no test first.

## Step-by-Step Task Breakdown

### Task 1 — `audit-core`: library, fixtures, tests (builder)
Implement `internal/eval/audit.go` per the pipeline above, test-first.
Acceptance criteria:
- `Audit` exists with the API above; stateless (no reads of package `cfg`; works with `EvalsDir`/`IterationsDir` pointing at fixtures).
- Instruction extraction rules pinned by tests: blank lines, headings (levels 1–6), table separators, backtick and tilde fences (incl. longer closers, unclosed fence), front-matter skipped, `---` mid-file counted, CRLF.
- RTCC: all four present in order passes; missing headings each named; out-of-order reported; headings inside code fences don't count; `### Role` / `## role` don't count.
- Hooks (`*-conventions`) listed and excluded from totals; engine skills audited from `SkillsFS`; unshipped non-conventions skill → `resource` violation; `SkillsFS == nil` handled.
- Table parsing: header required; `\|` unescape; wrong column count; empty cells; duplicate conflicting rows.
- AC3: fixture `unmapped` reports both the unmapped line (`unmapped`) and the line mapped to a non-existent criterion (`unknown-criterion`), each with `file:line`.
- AC4: `coverage-90` returns nil and prints total/mapped/`90.0%`; `coverage-89` returns an `*AuditError` with a `coverage` violation and prints `89.0%`; a 0-instruction input fails closed; a unit test proves 899/1000 (89.9%) fails and displays `89.9%`.
- `--prompt-file` candidate audited instead of the live prompt; the table and rubric still come from the agent; empty/non-regular candidate file → plain error.
- `verify-log` tests still pass (if the fence helper was extracted).
Validation: `go test ./internal/eval -run 'Audit' -count=1`, `go test ./internal/eval -count=1`, `go vet ./internal/eval`.

### Task 2 — `cli-audit`: cobra command (builder; depends on Task 1)
Implement `cmd/kairon/cmd/eval_audit.go` and tests (test-first).
Acceptance criteria:
- `audit` is registered on `evalCmd`; `--iterations-dir` (default `.kairon/iterations`) and `--prompt-file` are local flags; `--evals-dir` is inherited, not redefined; exactly one arg required; `SilenceUsage` set once args parse.
- `SkillsFS` is wired from the embedded templates (`fs.Sub(Templates, "templates/kiro/skills")`). If `Templates` is the zero `embed.FS` in `go test` (package `cmd` tests don't call `SetTemplates`) the test helper must either call `SetTemplates` with a test FS or the code must tolerate a failed `fs.Sub`/missing dir — pick one and test it; do **not** import `main`.
- AC1 end-to-end: `go run ./cmd/kairon eval audit builder` output contains `sentinel-protocol` under audited resources and `builder-conventions` under override hooks (exit status is non-zero today because no table exists; that is correct).
- AC2 end-to-end: `go run ./cmd/kairon eval audit planner; echo $?` prints a non-zero status and the error names `## Role`, `## Task`, `## Context`, `## Constraints` as missing.
- AC4 via CLI with `--evals-dir/--iterations-dir` fixtures: 90% → exit 0 and `OK`; 89% → non-zero.
- A test that runs the real-repo audit (`builder`, `planner`) from the repo root must not depend on any load-bearing table existing and must not break when one is later added — assert only on the resource/hook listing and on the planner's structure violations, using `t.Chdir` or relative paths consistent with other tests in `cmd/kairon/cmd`.
Validation: `go test ./cmd/kairon/cmd -count=1`, `go build ./...`.

### Task 3 — `docs-audit`: README + evaluation docs (documenter; depends on Task 2)
Acceptance criteria:
- `.kairon/iterations/README.md` gains a "Load-bearing table" section (AC5, manual verify): location `.kairon/iterations/<agent>/load-bearing.md`, the header `| Instruction | Criterion | Evidence |`, an **example table** (with a separator row, a row whose instruction contains an escaped `\|`, and a row per rubric criterion), the definition of an instruction (non-blank, outside fences, not heading, not table separator; front-matter skipped), the verbatim-match rule (exact after trimming), that Criterion must be a `name` in `<evals-dir>/rubrics/<agent>.yaml`, that Evidence is free text not machine-checked, that hooks (`*-conventions`) are excluded, that engine-skill lines count too, and the 90% threshold. Add the new file to the Layout tree; note `load-bearing.md` coexists with `iteration-NN.md` (verify-log already ignores non-matching names — state it).
- `docs/evaluation.md` gains a "Prompt Audit (`kairon eval audit`)" section: synopsis, flags table (`--prompt-file`, `--iterations-dir`, `--evals-dir`), the RTCC rule, the resources rule (engine vs hooks), rule IDs table (`structure`, `resource`, `table`, `unmapped`, `unknown-criterion`, `coverage`), report format, exit status, how to run the fixtures (commands for `coverage-90` → 0 and `coverage-89` → 1), and the note that it makes no model calls. Link to the README for the table format. Existing anchors (e.g. `#iteration-logs-and-verify-log`) must not change.
- Examples in the docs use real command output captured from the built binary.
Validation: `grep -n 'Load-bearing' .kairon/iterations/README.md`, `grep -n 'eval audit' docs/evaluation.md`, `task lint` (if markdown lint is part of it) .

### Task 4 — `validate`: independent verification (validator; depends on Tasks 1–3)
Acceptance criteria: each of AC1–AC5 verified with the commands below; `task test`, `task lint`, `task sync:check` pass; `git diff --name-only origin/main` contains no `.kiro/agents/*-prompt.md`, no `load-bearing.md`, no `.kairon/specs/*` other than this spec, no `CHANGELOG.md`; TDD evidence (named first-failing test) is in the commit message.

## Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/eval ./cmd/kairon/cmd -count=1
task test
task lint
task sync:check

# AC1
go run ./cmd/kairon eval audit builder 2>&1 | grep -E 'sentinel-protocol|builder-conventions'
# AC2 (non-zero, names the four headings)
go run ./cmd/kairon eval audit planner; echo "exit=$?"
# AC3
go run ./cmd/kairon eval audit demo --evals-dir internal/eval/testdata/audit/unmapped/evals \
  --iterations-dir internal/eval/testdata/audit/unmapped/iterations; echo "exit=$?"
# AC4
go run ./cmd/kairon eval audit demo --evals-dir internal/eval/testdata/audit/coverage-90/evals \
  --iterations-dir internal/eval/testdata/audit/coverage-90/iterations; echo "exit=$?"   # 0
go run ./cmd/kairon eval audit demo --evals-dir internal/eval/testdata/audit/coverage-89/evals \
  --iterations-dir internal/eval/testdata/audit/coverage-89/iterations; echo "exit=$?"   # non-zero
# AC5 (manual): README has an example table
grep -n '| Instruction | Criterion | Evidence |' .kairon/iterations/README.md
```

## Risks / Decisions to Flag

- **Front-matter skipping** is a deliberate narrowing of the literal "any non-blank line" rule (otherwise
  `SKILL.md` metadata lines would each need a table row). Documented in both docs. A hidden test using a
  literal reading on a skill with front-matter would differ; this is the one place the spec interprets
  rather than transcribes the issue.
- **Engine skills come from the embedded templates**, not the repo's live `.kiro/skills/`, so the audit
  scores what users actually receive. The two are kept identical by `sync:check`.
- **Percent is computed with integer arithmetic** to keep "exactly 90% passes / 89% fails" exact.
- Real agents fail the audit until their tables are written (out of scope by the issue). The
  `OK` path is only exercised by fixtures.
- Library tests must not use the package-global `cfg`/`configure`, to stay order-independent.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "audit-core"
    agent: "builder"
    description: "TDD: implement internal/eval/audit.go (Audit, AuditOptions, AuditError, instruction extraction, RTCC check, load-bearing table parser, rubric mapping, 90% threshold with integer arithmetic, report output) with fixtures under internal/eval/testdata/audit/ and tests in internal/eval/audit_test.go. Write failing tests first and confirm they fail for the expected reason."
    dependencies: []
    acceptance_criteria:
      - "Audit(w, agent, AuditOptions{IterationsDir, EvalsDir, PromptFile, SkillsFS}) is stateless and returns *AuditError listing violations as '<file>[:<line>]: <rule>: <message>' with rules structure, resource, table, unmapped, unknown-criterion, coverage"
      - "Instruction extraction skips blank lines, ATX headings, table separator rows, fenced code (backtick and tilde) and leading YAML front-matter; each occurrence counts"
      - "Prompt without '## Role', '## Task', '## Context', '## Constraints' in that order fails with each missing heading named; out-of-order headings are reported; headings inside code fences do not count"
      - "skill:// resources ending in -conventions are listed as override hooks and excluded from the score; other skills are read from SkillsFS and audited; an unshipped non-conventions skill yields a resource violation"
      - "Fixture with one unmapped line and one line mapped to a criterion absent from the rubric reports both (unmapped and unknown-criterion) with file:line"
      - "Report prints total instructions, mapped instructions and percentage; fixture at exactly 90% (9/10) returns nil, fixture at 89% (89/100) returns a coverage violation; 899/1000 displays 89.9% and fails; zero instructions fails"
      - "PromptFile candidate is audited instead of the agent prompt while the table and rubric still come from the agent; empty or non-regular candidate returns a plain error"
      - "Existing verifylog tests still pass unchanged if a shared fence helper is extracted"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval"
      - "go test ./internal/eval -run Audit -count=1"
      - "go test ./internal/eval -count=1"

  - id: "cli-audit"
    agent: "builder"
    description: "TDD: add cmd/kairon/cmd/eval_audit.go registering 'kairon eval audit <agent>' with local flags --iterations-dir and --prompt-file, inherited --evals-dir, SkillsFS wired from the embedded templates via fs.Sub(Templates, \"templates/kiro/skills\"), and tests in cmd/kairon/cmd/eval_audit_test.go."
    dependencies: ["audit-core"]
    acceptance_criteria:
      - "auditCmd is registered on evalCmd with ExactArgs(1) and SilenceUsage set after argument parsing"
      - "--iterations-dir defaults to .kairon/iterations, --prompt-file is a local flag, --evals-dir is inherited and not redefined"
      - "go run ./cmd/kairon eval audit builder output lists sentinel-protocol as audited and builder-conventions as an override hook"
      - "go run ./cmd/kairon eval audit planner exits non-zero and names the missing ## Role, ## Task, ## Context and ## Constraints headings"
      - "CLI run against the 90% fixture exits 0 and against the 89% fixture exits non-zero"
      - "No import of package main; tests that depend on embedded templates set them explicitly or tolerate their absence"
    validation_commands:
      - "go build ./..."
      - "go test ./cmd/kairon/cmd -count=1"
      - "go run ./cmd/kairon eval audit builder 2>&1 | grep -E 'sentinel-protocol|builder-conventions'"
      - "sh -c 'go run ./cmd/kairon eval audit planner; test $? -ne 0'"

  - id: "docs-audit"
    agent: "documenter"
    description: "Document the load-bearing table in .kairon/iterations/README.md (with an example table) and add a 'Prompt Audit (kairon eval audit)' section to docs/evaluation.md using real command output."
    dependencies: ["cli-audit"]
    acceptance_criteria:
      - ".kairon/iterations/README.md has a 'Load-bearing table' section with the '| Instruction | Criterion | Evidence |' header, an example table including an escaped pipe row, the instruction definition, verbatim-match rule, rubric-criterion rule, hook exclusion, front-matter note and 90% threshold; Layout tree lists load-bearing.md"
      - "docs/evaluation.md documents synopsis, flags (--prompt-file, --iterations-dir, --evals-dir), RTCC rule, engine skills versus override hooks, rule IDs, report format, exit status and the fixture commands for the 90% and 89% cases"
      - "No existing documentation anchors are renamed; no historical artifact (.kairon/specs other than this spec, CHANGELOG.md, evals results) is edited"
    validation_commands:
      - "grep -n 'Instruction | Criterion | Evidence' .kairon/iterations/README.md"
      - "grep -n 'eval audit' docs/evaluation.md"
      - "task sync:check"

  - id: "validate"
    agent: "validator"
    description: "Independently verify issue #322 acceptance criteria 1-5, the full test/lint/sync suite, and that out-of-scope files were not modified."
    dependencies: ["audit-core", "cli-audit", "docs-audit"]
    acceptance_criteria:
      - "AC1: eval audit builder reports sentinel-protocol as audited and builder-conventions as an override hook"
      - "AC2: eval audit planner exits non-zero naming the missing headings"
      - "AC3: unmapped fixture reports both the unmapped line and the unknown-criterion line"
      - "AC4: output shows total, mapped and percentage; 90% fixture exits 0; 89% fixture exits non-zero"
      - "AC5: README contains an example load-bearing table"
      - "task test, task lint and task sync:check pass"
      - "git diff against origin/main touches no agent prompt, no load-bearing.md, no CHANGELOG.md and no .kairon/specs file other than the issue-322 spec"
    validation_commands:
      - "go build ./..."
      - "task test"
      - "task lint"
      - "task sync:check"
      - "go run ./cmd/kairon eval audit builder"
      - "sh -c 'go run ./cmd/kairon eval audit planner; test $? -ne 0'"
```
