# Design Spec: Case metadata and `--filter` selection for `kairon eval`

Closes #392

## 1. Problem and scope

Eval cases cannot be categorized. `kairon eval` selects only by agent and case name. This change adds:

- an optional `metadata` map on a case file (string keys, scalar values);
- a repeatable `--filter <key><op><value>` flag that selects cases by `agent`, `name` or any metadata key;
- a `--strict` flag that turns "filters matched nothing" into an error;
- metadata display in `--list`, the per-case results JSON, `eval diff` and `eval trend`, and `--filter` on those two subcommands.

Out of scope (from the issue): tagging the shipped cases in `.kairon/evals/cases/`, and boolean grammars (`&&`, `||`, parentheses). No file under `.kairon/evals/cases/`, `.kairon/evals/rubrics/`, `.kairon/evals/fixtures/` or `.kiro/` changes, so no template sync is needed (`task sync:check` must still pass). Do not touch the pre-existing uncommitted `.kiro/agents/*.json` changes. Do not edit anything listed as historical in `AGENTS.md`.

## 2. Findings from the code

1. **`--strict` does not exist.** `grep` finds no `strict` flag in `cmd/kairon/cmd/` or `internal/eval/`. The issue refers to it as if it were there. This change adds it, scoped to the no-match rule only.
2. **`--case` and a full run take different paths.** `RunWithOptions` (`internal/eval/runner.go`) sends `--case`/`<agent> <testcase>` to `runSingleTestCase` (needs an agent) and everything else to `Run` -> `runProgressiveEvaluation` (all rubrics, or one). `--resume` goes through `runWithResume` -> `runProgressiveEvaluation`. `--list` goes to `listTestCases` -> `PrintTestCaseList`. Filters must apply on all of these.
3. **Partial runs are already modeled.** `stampCoverage` sets `complete = (cfg.caseFilter == "" && casesCovered(...))` and records `case_filter`. `verify-log` refuses incomplete runs for comparison. A filtered run is a partial run and must use this mechanism (see section 3.8).
4. **`StringSliceVar` would break the flag.** Cobra's `StringSlice` splits on commas, so `--filter speed==fast,medium` would become two filters. Use `StringArrayVar`.
5. **Case loading** is `loadCases` (`runner.go`): `yaml.Decoder` with `KnownFields(true)`, so `metadata` must be a declared field or loading fails. Errors there are wrapped as `errMalformedCase: <file>: <err>`, which already names the file.
6. **`CaseResult` is built in `executeCaseRepeats` and stamped by `stampCase`** (`verdict.go`). That is the single place to copy metadata into results.
7. **`suite_sha256`** (`suite.go`) hashes the raw bytes of every `cases/<agent>/*.yaml`.
8. **`eval trend`** (`trend.go`) is per run and per agent, built from `loadRunRecord`. It has no per-case rows today. `eval diff` (`diff.go`) reads `summary.json` and `<agent>.json`; it has no per-case display apart from `showOutputChanges`.

## 3. Design

### 3.1 Metadata on cases (AC 1-3)

`internal/eval/types.go`:

```go
// CaseMetadata is a case's key -> scalar text map. Selection and reporting only.
type CaseMetadata map[string]string

func (m *CaseMetadata) UnmarshalYAML(n *yaml.Node) error // validates, see below

type TestCase struct { ...; Metadata CaseMetadata `yaml:"metadata,omitempty" json:"metadata,omitempty"` }
type CaseResult struct { ...; Metadata CaseMetadata `json:"metadata,omitempty"` }
```

`UnmarshalYAML` receives the `metadata` node and:

- requires a mapping node (a scalar or list under `metadata:` is an error);
- for each key: reject an empty key, a key containing any of `= < > ! ,` (it could never be addressed by a filter), the reserved keys `agent` and `name`, and a duplicate key;
- for each value: resolve aliases, then accept only a scalar node. A sequence or mapping node is an error. A `!!null` scalar is an error (not a string, number or boolean).
- text form of the value (AC 6): `!!bool` -> `"true"`/`"false"` (decode, then `strconv.FormatBool`, so `True` and `TRUE` normalize); every other scalar (string, int, float, timestamp) -> `node.Value` exactly as written. A quoted `"3"` is the string `3`.

Error messages name the key; `loadCases` already prefixes the file name (`errMalformedCase: <file>.yaml: ...`). Required wording to test for: file name and key both appear, for example
`malformed case: slow.yaml: metadata key "tags": value must be a string, number or boolean (got a list)` and
`... metadata key "agent" is reserved: agent and name are built-in filter keys`.
yaml.v3 propagates a non-`TypeError` error from `UnmarshalYAML` unchanged, so the message survives. Verify this in the test.

Cases without `metadata` have a nil map and behave exactly as before. Metadata is never read by `executeCase`, `assemblePrompt`, scoring or the judge (constraint).

### 3.2 Filter model (AC 4-10, 12)

New file `internal/eval/filter.go`:

```go
type Filter struct {
    Key    string
    Op     string   // as typed: "==", "<>", "!=", "<", "<=", ">", ">="
    Values []string // exactly one for ordering operators
}
func ParseFilter(expr string) (Filter, error)
func ParseFilters(exprs []string) ([]Filter, error)   // first error wins; nil/empty -> nil, nil
func (f Filter) String() string                       // key + op + strings.Join(values, ",")
func FormatFilters(fs []Filter) string                // String() of each joined with " AND "
func (f Filter) matches(lookup func(key string) (string, bool)) bool
func matchAll(fs []Filter, lookup func(string) (string, bool)) bool
func filterCases(cases []TestCase, fs []Filter) []TestCase
func (tc TestCase) filterValue(key string) (string, bool)
func resultLookup(agent string, cr CaseResult) func(string) (string, bool)
```

**Parsing.**

1. Find the first byte of `s` that is one of `= < > !`. None -> error.
2. At that index take the longest operator from `==  <>  !=  <=  >=  <  >`. If none matches (`a=b`, `a!b`, `a=>b`, `a<=>b` handled by longest match: `<=` then value `>b`) -> unknown-operator error.
3. `key` = `strings.TrimSpace(s[:i])`; empty -> error. The key is case-sensitive.
4. Value text = everything after the operator. For `==`, `<>`, `!=`: split on `,`, `TrimSpace` each item; any empty item (including an empty value, `a==,b`, `a==b,`) -> error. For ordering operators: a `,` in the value -> error ("takes exactly one value"); the trimmed value must be non-empty.
5. A value cannot contain a comma (there is no escape syntax); say so in the docs.

**Error text** always shows the expected syntax, for example:
`invalid --filter "speed=fast": unknown operator; expected <key><operator><value> with operator one of ==, <>, !=, <, <=, >, >= (for example speed==fast, color<>red,blue, retries>=2)`.

**Matching.** `lookup(key)` returns `(text, present)`:

- `agent` -> `tc.Agent` (set by `loadCases`), `name` -> `tc.Name`; always present;
- any other key -> `tc.Metadata[key]`, present only if the key exists. `skill` has no special treatment (AC 3).

Rules, in order:

1. Key not present -> no match, for every operator including `<>` and `!=` (AC 10).
2. `==`: match if the text equals any listed value (byte-exact, case-sensitive, AC 6/8).
3. `<>`/`!=`: match if the text equals none of the listed values.
4. `<  <=  >  >=`: match only if both the case text and the filter value are numeric, then compare as `float64`. Otherwise no match (AC 7).
5. Several `--filter` flags: all must match (AND, AC 9).

**Numeric** means the whole string matches `^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$` and `strconv.ParseFloat` succeeds with a finite result. Do not rely on `ParseFloat` alone: it also accepts `NaN`, `Inf` and hex floats, which must be non-numeric. Equality never parses numbers: `retries==3` does not match `3.0`. State this in the docs.

### 3.3 Selection in a run (AC 11, 13, 14)

New options and config:

- `RunOptions.Filters []string` and `RunOptions.Strict bool`.
- `runConfig` gains `filters []Filter` and `scopeAgents []string` (nil means "no narrowing"). `configure` resets both, like the other fields.
- `cfg.caseFilter` becomes the *label* of the selection: `caseFilterLabel(testcase, filters)`. With no filters it is exactly `testcase` (today's value, so existing results and tests are unchanged). With filters it is the parts joined with `" AND "`: `case=<testcase>` (only when `--case` is given) followed by each `Filter.String()`. Example: `case=add-flag AND speed==fast`.

**Validation order in `RunWithOptions`**, before anything else touches state (next to `validateRepeat`):

1. `ParseFilters(options.Filters)`; a parse error is returned as is (CLI prints usage for it, see 3.6). It does not depend on `Strict` (AC 12).
2. `--filter` or `--strict` together with `--perf` or `--cleanup` is rejected with a message naming both flags (they run no cases; the flags would otherwise be silently ignored, same approach as `validateRepeat`). `--strict` without `--filter` is accepted and has no effect; document it.

**Pre-flight selection**, after `configure` and the `Cleanup` early return, before `pinRun`/`pinSuites`/sandbox setup:

```go
// selectForRun returns whether the run should proceed.
func selectForRun(w io.Writer, agent, testcase string, filters []Filter, strict bool, opts RunOptions) (proceed bool, err error)
```

- No filters -> `true, nil` immediately (all existing behavior untouched).
- Agents considered: `agentsInScope(agent, opts)` (the named agent, or every rubric agent).
- For each, `loadCases`. A load error is not handled here: the agent is left alone so the existing warning/fatal path reports it later (`errMalformedCase`/`errInvalidChecks` still fail the run in `runProgressiveEvaluation`; for a named agent, `listTestCases`/`runSingleTestCase` still report it).
- If `testcase != ""` and the named agent has no case with that name, return `true, nil` and let the existing `ValidateTestCase` error ("test case ... not found") surface. The no-match rule applies only when the agent/`--case` selection is itself non-empty.
- Narrow by `testcase` (when set), then by `filterCases`. Collect the agents that still have at least one case.
- If the total is zero:
  - non-strict: write `⚠️  No eval cases match --filter <FormatFilters>` plus the agent/case scope (`(agent: x, case: y)`) to `w` (stderr in the CLI), return `false, nil` (exit 0, nothing run, no results directory, no model call);
  - `--strict`: return `false, error` with `❌ no eval cases match --filter <FormatFilters> (--strict)` (non-zero exit; usage is not shown because it is a run result, `SilenceUsage` is already set).
- Otherwise set `cfg.scopeAgents` to the agents with matches and return `true, nil`.

`agentsInScope` (`provenance.go`) returns `cfg.scopeAgents` first when it is non-nil. Result: `pinRun` and `pinSuites` pin only agents that will run, so a disallowed model on a skipped agent does not block a filtered run, and no skipped agent gets a `suite_sha256`.

**Execution.**

- `runProgressiveEvaluation`: after `loadCases(rubric.Agent)`, `cases = filterCases(cases, cfg.filters)`. If filters are active and the result is empty, print `⏭  Skipping <agent>: no cases match --filter ...` and `continue` before `checkResumeIntegrity`, `evaluateProgressive`, the agent verdict and the `evaluated` counter (AC 14). Otherwise print one line `   filter <label>: N of M cases` before running. The earlier "reject any agent whose cases cannot load" loop is unchanged.
- `runSingleTestCase`: after `GetTestCase`, if filters are active and the case does not match, behave like the pre-flight no-match (this only happens for direct callers; the pre-flight already handled the CLI path). Keep one helper so both produce the same message.
- `stampCoverage` is unchanged. Because `cfg.caseFilter` is now non-empty for a filtered run, the existing `complete = cfg.caseFilter == "" && ...` rule makes every filtered run `complete: false` with `case_filter` set to the label, exactly like a `--case` run. This is intentional: a filtered run is a partial run, so `verify-log` will not accept it as a full run and trend marks it `INCOMPLETE`. Document it.

**Resume.** `--resume --filter ...` applies the filter to the cases of the resumed run (same `runProgressiveEvaluation` path). Add one more refusal to `checkResumeIntegrity`, inside the existing `len(existing.Cases) > 0` switch: if `derefString(existing.CaseFilter) != cfg.caseFilter`, return
`❌ cannot resume: --filter changed since the interrupted run of <agent> (recorded case_filter="..."; now "...") — resume with the same --filter or start a fresh run`.
A nil recorded `CaseFilter` compares as `""`, so an interrupted run from before this change resumes exactly as it does now without filters. This keeps one result file from mixing two selections.

### 3.4 Visibility: list and results (AC 15, 16)

- `PrintTestCaseList(agent)` keeps its signature and output for cases without metadata. Extract `printTestCaseList(w io.Writer, agent string, filters []Filter) error`; `PrintTestCaseList` calls it with `os.Stdout` and `cfg.filters`. When a case has metadata, print an extra line under it, keys sorted:
  ```
     • add-flag - Adds a CLI flag
         metadata: skill=cli, speed=fast
  ```
  With filters, only matching cases are listed and the header gains `(filter: speed==fast)`. `--list` still requires an agent (unchanged). `selectForRun` runs for `--list` too, so an empty filtered list follows the same warn/strict rule (AC 13).
- `stampCase` sets `cr.Metadata = maps.Clone(tc.Metadata)` (nil stays nil, so `omitempty` keeps old output byte-identical for cases without metadata). Every repeat of a case records it. `stampResumedCases` only stamps results with `Passed == nil`; it does not back-fill metadata onto results saved earlier (they keep what they ran with).

### 3.5 `eval diff` and `eval trend` (AC 17)

Both subcommands read saved results, never the case YAML, so they filter on what the results recorded: `agent` = the agent being reported, `name` = `CaseResult.CaseName`, every other key = `CaseResult.Metadata`. A result saved before this change has no metadata, so it matches only filters on `agent` and `name`. Both accept `--filter` (the same persistent flag, section 3.6) and use the same parser and matcher. `--strict` is not an option of either.

**`diff`.**

- `DiffWithOptions` parses `opts.Filters` and calls a new `diffFiltered(w, runA, runB, filters)`. `diffTo(w, runA, runB)` stays and calls `diffFiltered(..., nil)`, so existing callers and tests are unaffected. With no filters and no metadata in either run, output is byte-identical to today.
- With filters: print `Filter: <FormatFilters>` under the title. For each agent, reduce both `AgentResult.Cases` to the matching results, then compute the agent line from the reduced results (`agentScoreTotals` -> `score/max`, as a 0-1 fraction like today), the criterion averages, and `showOutputChanges` from the reduced results. If a side has no matching result for an agent, print `-` for that side and no delta; if neither side has one, omit the agent. If an agent file cannot be loaded while filters are active, print a one-line note and skip the agent (nothing to filter).
- The cost sections come from `summary.json` totals, which cannot be filtered. With filters, compute `Cost Delta`, `Cost Trends` tokens and `Quality per Dollar` from the sum of `AgentCost` + `JudgeCost` of the reduced results (`CostInfo.addTotals`), including `CostIncomplete`.
- Metadata display: when any case result in either run has metadata, print a `Cases:` block per agent (after the criterion lines): one line per case name seen in either run, `  <name> [k=v, k=v]  <scoreA> -> <scoreB>`, where a case's score is its pooled `score/max` over its runs. `showOutputChanges` appends the same `[k=v, ...]` to its `Case '<name>' output changed:` line (it uses run B's metadata, falling back to run A's). Nothing extra is printed when no result carries metadata.

**`trend`.**

- `TrendOptions` gains `Filters []Filter`.
- `TrendRow` gains `CaseMetadata map[string][]string` (`json:"case_metadata,omitempty"`): for each metadata key, the sorted, de-duplicated values across the run's (filtered) case results. Text rows append `  metadata speed=fast,medium skill=cli` only when non-empty, so rows for old runs are unchanged.
- With filters, `LoadTrend` recomputes `Score` and `SuccessRate` for each run from the matching case results (build an `AgentResult` copy with the reduced `Cases` and use `agentVerdict(...).Score` and its attempts counts). A run with no matching case result is dropped. If no run remains, return `no eval results for agent "<a>" matching --filter <label> under <dir>` (non-zero, like the existing no-results error). `Complete` still reports the run's own flag; the text and the docs say that a filtered row is a subset of that run.
- The existing unfiltered code path must keep its exact output (existing fixture `internal/eval/testdata/trend`).

### 3.6 CLI wiring (AC 4, 12, 13)

`cmd/kairon/cmd/eval.go`:

- `var evalFilter []string` and `var evalStrict bool`.
- `evalCmd.PersistentFlags().StringArrayVar(&evalFilter, "filter", nil, "...")`: persistent, so `eval diff` and `eval trend` inherit it with one declaration. It must be `StringArray`, not `StringSlice` (finding 4).
- `evalCmd.Flags().BoolVar(&evalStrict, "strict", false, "...")`: local to `eval` (diff and trend must not inherit it).
- In the `eval` `RunE`, call `eval.ParseFilters(evalFilter)` before `cmd.SilenceUsage = true`, next to the `--repeat` check, so a malformed filter prints usage and exits non-zero before any work. Pass `Filters: evalFilter, Strict: evalStrict` in `RunOptions`.
- `diffCmd` passes `Filters: evalFilter` in the `RunOptions` it builds; `eval_trend.go` parses `evalFilter` and passes it in `TrendOptions`. Both parse before `SilenceUsage`.
- The no-match warning goes to stderr (`os.Stderr`); `--list` output goes to stdout.

### 3.7 `suite_sha256` decision (constraint)

**Decision: keep hashing the raw case-file bytes. A metadata-only edit changes `suite_sha256`, so it invalidates `--resume` and `verify-log` run comparisons, like any other edit to a case file.**

Reasons:

1. No existing hash changes. Excluding `metadata` from the hash would mean parsing and re-serializing every case file (which changes every existing `suite_sha256` and breaks comparison with every recorded baseline) or textually stripping a YAML block (fragile with flow style, comments and anchors).
2. Metadata is recorded per case in the results, so it is part of what a result attests. A comparison between runs that tagged the same case differently is not like-for-like in what it selected.
3. The cost is bounded: tagging cases is a one-time edit, after which new baselines are recorded. The issue explicitly puts tagging the shipped cases out of scope.

Add a test that pins this (a metadata-only edit changes `suiteSHA256`; a case file without `metadata` hashes the same as before) and document it in `docs/evaluation.md` under "Run coverage" and "Resume refusal".

### 3.8 Documentation (AC 18)

`docs/evaluation.md`:

- "Test Case Format": add `metadata:` to the example block (commented as optional) and a `metadata` bullet. Keep new YAML examples free of top-level `checks:` and `simulated_user:` keys: `internal/checks/checks_docs_test.go` and `internal/eval/simulated_user_docs_test.go` scan the doc's YAML blocks.
- New section "Selecting cases: metadata and `--filter`" placed after "Running Evaluations": field rules (scalar types, reserved `agent`/`name`, text form of booleans and numbers, load errors), filter syntax, operator table, comma lists, AND, numeric rules (ordering needs two numbers; equality is textual), missing-key behavior (never matches, including `<>`), `--strict` and the exit codes, usage errors, `--list`, the results JSON field, `diff`/`trend` behavior (filter on recorded metadata; legacy results have none), how `--resume` and `complete`/`case_filter` treat filtered runs, the suite-hash decision, a comma limitation, and suggested conventional keys: `speed` (`fast`, `slow`), `skill` (the skill or capability exercised), `category` (free-form grouping). State that conventional keys are only a convention.
- Running Evaluations flag table: add `--filter` and `--strict` rows; add examples (`kairon eval architect --filter speed==fast`, `--filter 'skill==cli' --filter 'speed<>slow'`, `--list --filter ...`).
- "Run coverage" table: extend the `case_filter` row and bullets (a filtered run is never complete; label format).
- "Per-case fields" (results): add `metadata`.
- "Resume refusal": add the changed-filter refusal.
- trend and diff sections: document `--filter` and the metadata output.

`README.md`: add a short "Selecting Eval Cases" subsection under CLI Usage (next to "Running Checks Without an Eval") with a two-line example and a link to `docs/evaluation.md#selecting-cases-metadata-and---filter`. Add `kairon eval --filter ...` to the CLI Usage code block. Use the real anchor that the new heading generates.

### 3.9 Acceptance-criteria traceability

| AC | Where |
|----|-------|
| 1-3 | 3.1 (`CaseMetadata`, `loadCases`), 3.2 (`filterValue`) |
| 4-5 | 3.2 `ParseFilter`, 3.6 flag |
| 6-8, 10 | 3.2 matching rules |
| 9 | 3.2 `matchAll` |
| 11 | 3.3 pre-flight and execution |
| 12 | 3.2 errors, 3.3 validation order, 3.6 (before `SilenceUsage`) |
| 13-14 | 3.3 |
| 15-16 | 3.4 |
| 17 | 3.5, 3.6 |
| 18 | 3.8 |
| 19 | task tests below |

## 4. Relevant files

Create:
- `internal/eval/filter.go`, `internal/eval/filter_test.go`
- `internal/eval/metadata_test.go` (metadata loading, results JSON, suite hash, execution unaffected)
- `internal/eval/filter_run_test.go` (pre-flight, no-match, skip, resume, list)
- `internal/eval/diff_filter_test.go`, `internal/eval/trend_filter_test.go` (or extend `trend_test.go`)
- test fixtures: build temp evals dirs in tests (`t.TempDir()`, `--backend stub` style used by `checks_only_test.go` and `runner_config_test.go`); a small result-fixture directory under `internal/eval/testdata/` only if it keeps the diff/trend tests simpler

Modify:
- `internal/eval/types.go` (`CaseMetadata`, `TestCase.Metadata`, `CaseResult.Metadata`, `RunOptions.Filters/Strict`)
- `internal/eval/runner.go` (validation, pre-flight, `runProgressiveEvaluation`, `runSingleTestCase`, `checkResumeIntegrity`, `listTestCases`)
- `internal/eval/config.go` (`runConfig.filters`, `scopeAgents`; `configure`)
- `internal/eval/provenance.go` (`agentsInScope`)
- `internal/eval/selective.go` (`printTestCaseList`)
- `internal/eval/verdict.go` (`stampCase`)
- `internal/eval/diff.go`, `internal/eval/trend.go`
- `cmd/kairon/cmd/eval.go`, `cmd/kairon/cmd/eval_trend.go`, `cmd/kairon/cmd/eval_test.go`, `cmd/kairon/cmd/eval_trend_test.go`
- `docs/evaluation.md`, `README.md`

Unchanged on purpose: `internal/eval/suite.go` (decision 3.7), everything under `.kairon/evals/`, `.kiro/`, `cmd/kairon/templates/`.

## 5. Team orchestration

```
filter-core (T1) ──┬──> run-selection (T2) ──┐
                   │                          ├──> cli-wiring (T3) ──> docs (T5) ──> validate (T6)
                   └──> diff-trend (T4) ──────┘
```

- T1 first. It defines `CaseMetadata`, `Filter` and the matcher that T2 and T4 use.
- T2 (run path) and T4 (diff/trend) edit different files and run in parallel after T1. T2 owns `types.go` `RunOptions`, `runner.go`, `config.go`, `provenance.go`, `selective.go`, `verdict.go`. T4 owns `diff.go`, `trend.go` and their tests. Neither edits the other's files.
- T3 needs `RunOptions.Filters/Strict` (T2) and `TrendOptions.Filters` / `DiffWithOptions` filter handling (T4).
- T5 documents behavior as built, so it follows T2-T4.
- T6 validates everything, read-only.

TDD (AGENTS.md): in each task the builder writes the failing tests first, runs them and confirms they fail for the expected reason (missing field/function behavior, not a compile typo), then implements. The tests and implementation of a task go into one commit; no commit leaves CI red. The commit message names the test seen failing first. Docs-only task T5 needs no test first.

## 6. Step-by-step task breakdown

### T1. `case-metadata-and-filter-core` (builder)

Implement 3.1, 3.2 and the suite-hash test of 3.7. Tests (`filter_test.go`, `metadata_test.go`):

- Metadata: absent -> nil; string/int/float/bool/quoted-number text forms (`True` -> `"true"`, `3.0` stays `3.0`); list value, map value, null value, `metadata` that is not a mapping, key `agent`, key `name`, duplicate key, key with `=`: each fails `loadCases` with an error containing the file name and the key; a case file without `metadata` loads identically to before; unknown sibling fields still rejected (`KnownFields`).
- Parse: all seven operators (`!=` and `<>` both); whitespace around key/values trimmed; errors for no operator, `=`, `a=>b`... (unknown operator), empty key, empty value, `a==,b`, `a==b,`, comma list with each ordering operator; every error message contains the expected-syntax text and the bad input.
- Match: `==`/`<>`/`!=` case-sensitive (`Fast` vs `fast`), comma lists (any / none), missing key never matches for each of the seven operators, `agent` and `name` built-ins (including `name==a,b`), `skill` read from metadata, AND of several filters, ordering with both numeric (`2 < 10` numerically, not textually), ordering with a non-numeric case value, with a non-numeric filter value, `NaN`/`Inf`/`0x10` treated as non-numeric, equality not numeric (`3` vs `3.0`).
- `suiteSHA256`: a case file edited only in `metadata` changes the hash; a case file without `metadata` hashes as before (compute against the pre-change hashing of the same bytes, or assert it equals the hash of the raw-bytes construction).
- A stub-backend run of two cases that differ only in `metadata` gives identical agent prompt and scores (metadata does not change execution or scoring).

### T2. `run-selection` (builder, depends on T1)

Implement 3.3 and 3.4: `RunOptions.Filters/Strict`, config fields, validation order, `selectForRun`, `agentsInScope` narrowing, `runProgressiveEvaluation` skip and counts line, `runSingleTestCase` guard, `caseFilterLabel`, resume refusal, `printTestCaseList`, `stampCase` metadata. Tests (`filter_run_test.go`, plus extend existing runner tests only where needed):

- Pre-flight with a temp evals dir and the stub backend: no filters -> unchanged; filters that match -> only matching cases run; `--case` + filter combined with AND; `--case` naming a missing case -> the existing "not found" error, not the filter message.
- No match, non-strict: warning names every filter, nothing runs, returns nil, no results directory created. No match, strict: error names every filter. Both hold for `--list`.
- Malformed filter with `Strict` false and true: the same error, before any case ran (assert no results directory, no backend call).
- Several agents: an agent with zero matches is skipped (no `<agent>.json`, not in `summary.json`, no `suite_sha256` for it), the others run; all-zero follows the no-match rule.
- Filtered run: `<agent>.json` and `summary.json` record `complete:false` and `case_filter` equal to the label; unfiltered and `--case` runs record exactly what they did before.
- Results JSON `metadata` present for tagged cases (every repeat) and absent for untagged.
- Resume: same filter resumes; different or missing filter -> the new refusal; a legacy result (no `case_filter`) resumes without filters as before.
- `--filter` with `--perf` or `--cleanup` is rejected; `--strict` alone is accepted and changes nothing.
- `--list` output shows `metadata:` lines (sorted keys), honors filters, and is unchanged for untagged cases.

### T3. `cli-wiring` (builder, depends on T2 and T4)

Implement 3.6. Tests in `cmd/kairon/cmd/eval_test.go` / `eval_trend_test.go`:

- `--filter` is a persistent `StringArray` flag inherited by `diff` and `trend`; `--strict` is local to `eval` (not inherited by `diff`/`trend`); both appear in `--help`.
- Repeating the flag collects every value, and `--filter speed==fast,medium` stays one value (not split).
- A malformed filter returns an error before `SilenceUsage` is set (usage is shown), with an unknown backend configured to prove no later work ran, in the style of `TestEvalRepeatBelowOneRejected`.
- `Filters` and `Strict` reach `eval.RunOptions`; `diff` and `trend` receive the filters.
- Strict no-match returns a non-nil error and non-strict returns nil (stub backend, temp evals dir).

### T4. `diff-trend-metadata-filter` (builder, depends on T1)

Implement 3.5 in `diff.go` and `trend.go`, with tests (`diff_filter_test.go`, `trend_filter_test.go`) built from small result directories created in `t.TempDir()`:

- Unfiltered diff and trend output are byte-identical to today for results without metadata (existing tests pass untouched, including the `testdata/trend` fixture).
- Diff with filters: agent scores, criterion deltas, output changes and cost sections reflect only matching cases; one-sided and empty matches; `Filter:` line; `agent`/`name` built-in keys; results without metadata match only built-ins.
- Diff shows the `Cases:` block and `[k=v]` markers when metadata is present.
- Trend: `case_metadata` in JSON and the text suffix when present, absent otherwise; filtered score and success rate recomputed from matching cases; runs without matches dropped; the no-matching-runs error names the filter and agent.
- Malformed filters are rejected by the shared parser (covered in T1); here assert the options plumbing returns the parse error.

### T5. `docs` (builder, depends on T2, T3, T4)

Implement 3.8. Verify the docs-scanning tests still pass (`go test ./internal/checks ./internal/eval -run 'Docs'`), the new README link anchor matches the heading, and no template-synchronized file changed (`task sync:check`).

### T6. `validate-complete` (validator, depends on T5)

Read-only check of every acceptance criterion 1-19 against the code, tests and docs, plus the full gates: `task test`, `task lint`, `task fmt:check`, `task sync:check`, `task eval:selftest`. Confirm: `.kiro/agents/*.json`, `.kairon/specs/` (other than this spec), `CHANGELOG.md` and `.kairon/evals/cases/` are unmodified by the change; no new commit contains a failing test; each task's commit message names the test seen failing first.

## 7. Validation commands (whole change)

```bash
go build ./...
go vet ./...
go test ./internal/eval/... ./cmd/... ./internal/checks/... -count=1
task test
task lint
task fmt:check
task sync:check
task eval:selftest
# Manual smoke (stub backend, selftest fixtures):
go run ./cmd/kairon eval --list --evals-dir internal/eval/testdata/evals selftest --filter 'name==nope'    # warning, exit 0
go run ./cmd/kairon eval --list --evals-dir internal/eval/testdata/evals selftest --filter 'name==nope' --strict   # error, exit 1
go run ./cmd/kairon eval --list --evals-dir internal/eval/testdata/evals selftest --filter 'speed=fast'   # usage error, exit 1
```

## 8. Risks and notes

- **Metadata text form for numbers** is the literal as written; ints in non-decimal notation (`0x10`) are non-numeric for ordering. Documented, not special-cased.
- **Package-level `cfg`**: filters live in `cfg` like the other run settings; `configure` resets them, so filters cannot leak between runs or tests. Tests that call run functions directly must set `cfg.filters` through `configure` or `RunWithOptions`.
- **A filtered run overwrites nothing**: each run still creates its own results directory.
- **`--list` without an agent** still errors (unchanged). Listing across all agents is a natural follow-up but not required by the issue.

## 9. Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "case-metadata-and-filter-core"
    agent: "builder"
    description: "Add the optional case metadata map (CaseMetadata with validating UnmarshalYAML, TestCase.Metadata, CaseResult.Metadata) and the filter core in internal/eval/filter.go (ParseFilter/ParseFilters, Filter.String, FormatFilters, matching rules, numeric check, filterCases, TestCase.filterValue, resultLookup). Write the failing tests first (filter_test.go, metadata_test.go), confirm they fail for the expected reason, then implement; commit tests and implementation together. Includes the test that pins the suite_sha256 decision (raw bytes hashed; a metadata-only edit changes the hash; files without metadata hash as before) and a test that metadata does not change execution or scoring."
    dependencies: []
    acceptance_criteria:
      - "A case YAML with a metadata map of string/number/boolean values loads; the text form of a bool is true/false and other scalars keep their written text; a case without metadata loads exactly as before"
      - "A list value, map value, null value, non-mapping metadata, a key named agent or name, a duplicate key, or a key containing = < > ! or , fails loadCases with an error that contains the case file name and the key"
      - "ParseFilter accepts ==, <>, !=, <, <=, >, >= and trims whitespace around the key and each value; it rejects a missing or unknown operator, an empty key, an empty value, an empty list item, and a comma list with an ordering operator, each with a message that shows the expected syntax and the bad input"
      - "Matching: ==/<>/!= compare text case-sensitively; == matches any listed value and <>/!= matches none; a missing key never matches for any operator including <> and !=; ordering operators match only when both sides are plain decimal numbers (NaN, Inf and hex are not numeric); equality never parses numbers; several filters are ANDed"
      - "agent and name are built-in keys taken from the case; skill and every other key come from metadata"
      - "suiteSHA256 changes when only the metadata of a case file is edited and is unchanged for case files without metadata"
      - "Two cases that differ only in metadata produce identical agent prompts and scores on the stub backend"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'Filter|Metadata|SuiteSHA256' -count=1"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "run-selection"
    agent: "builder"
    description: "Wire filters into the run path (design sections 3.3 and 3.4): RunOptions.Filters/Strict, runConfig filters and scopeAgents, early filter parsing and rejection of --filter/--strict with --perf/--cleanup, selectForRun pre-flight with the warn vs --strict no-match rule, agentsInScope narrowing, per-agent skip in runProgressiveEvaluation, guard in runSingleTestCase, caseFilterLabel feeding cfg.caseFilter so filtered runs record complete=false and case_filter, the changed-filter resume refusal, printTestCaseList with metadata lines and filter support, and stampCase copying metadata into CaseResult. Write the failing tests first (filter_run_test.go), confirm the expected failure, then implement; commit tests and implementation together."
    dependencies: ["case-metadata-and-filter-core"]
    acceptance_criteria:
      - "With filters, only matching cases run; agent, --case and filters combine with AND; a --case naming a missing case still gives the existing not-found error"
      - "No match without --strict prints a warning naming every filter, runs nothing, creates no results directory and returns nil; with --strict it returns an error naming every filter"
      - "A malformed filter returns the usage-style error before any case runs, with and without Strict"
      - "With several agents in scope, an agent with zero matching cases is skipped (no result file, no summary entry, no suite hash) and the no-match rule applies only when the whole selection is empty"
      - "A filtered run records complete=false and case_filter equal to the label in <agent>.json and summary.json; unfiltered and --case runs record what they did before"
      - "--resume applies the filter and refuses when the recorded case_filter differs from the current label; a legacy result resumes as before"
      - "--filter or --strict together with --perf or --cleanup is rejected; --strict without --filter changes nothing"
      - "--list prints a sorted metadata line under cases that have metadata, honors filters, and is unchanged for cases without metadata"
      - "CaseResult.Metadata is set for every repeat of a tagged case and omitted from the JSON for untagged cases"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -count=1"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "diff-trend-metadata-filter"
    agent: "builder"
    description: "Implement design section 3.5 in internal/eval/diff.go and trend.go: diffFiltered (diffTo keeps its signature and calls it with no filters), DiffWithOptions filter handling, Filter line, filtered agent scores/criteria/output changes/cost sections computed from matching recorded case results, Cases block and [k=v] markers when metadata exists; TrendOptions.Filters, TrendRow.CaseMetadata (json case_metadata, omitted when empty), filtered score/success recomputation, dropping runs without matches, and the no-results error naming the filter. Write the failing tests first with small result directories built in t.TempDir(), confirm the expected failure, then implement; commit tests and implementation together."
    dependencies: ["case-metadata-and-filter-core"]
    acceptance_criteria:
      - "Unfiltered diff and trend output is byte-identical to today for results without metadata; existing diff and trend tests pass unmodified"
      - "diff with filters reports agent scores, criterion deltas, output changes and cost sections only from matching case results, handles a side with no matches, and prints a Filter line"
      - "diff prints a Cases block and [k=v] markers on output-changed cases when any result carries metadata, and nothing extra otherwise"
      - "agent and name filters work on recorded results; results saved without metadata match only agent and name filters"
      - "trend rows include case_metadata (JSON) and a metadata suffix (text) only when the run's cases carry metadata"
      - "trend with filters recomputes score and success rate from matching cases, drops runs with no match, and returns an error naming the agent and filter when no run remains"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'Diff|Trend' -count=1"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "cli-wiring"
    agent: "builder"
    description: "Implement design section 3.6 in cmd/kairon/cmd: persistent repeatable --filter (StringArrayVar, so commas are not split) on eval, local --strict on eval, filter parsing before SilenceUsage so a malformed filter shows usage, Filters/Strict passed in RunOptions, filters passed to eval diff (DiffWithOptions) and eval trend (TrendOptions). Write the failing cmd tests first, confirm the expected failure, then implement; commit tests and implementation together."
    dependencies: ["run-selection", "diff-trend-metadata-filter"]
    acceptance_criteria:
      - "--filter is a persistent string-array flag inherited by eval diff and eval trend; --strict is local to eval and not inherited; both are listed in eval --help"
      - "Repeating --filter collects every value and a value with commas (speed==fast,medium) stays a single value"
      - "A malformed filter returns an error before SilenceUsage is set and before any other work (an unknown backend is not reached), with or without --strict"
      - "Filters and Strict reach eval.RunOptions; diff and trend receive the filters"
      - "Strict no-match returns a non-nil error; non-strict no-match returns nil"
    validation_commands:
      - "go build ./..."
      - "go test ./cmd/... -count=1"
      - "go run ./cmd/kairon eval --list --evals-dir internal/eval/testdata/evals selftest --filter 'name==nope'"
      - "test -z \"$(gofmt -l internal cmd)\""

  - id: "docs"
    agent: "builder"
    description: "Document the feature (design section 3.8): docs/evaluation.md (Test Case Format metadata field and bullet, new 'Selecting cases: metadata and --filter' section with syntax, operators, comma lists, numeric rules, missing-key behavior, --strict, usage errors, --list, results field, diff/trend, resume and complete/case_filter behavior, suite_sha256 decision, comma limitation, suggested conventional keys speed/skill/category; flag table rows and examples; Run coverage and Resume refusal updates; per-case result field) and README.md (short Selecting Eval Cases subsection with a link to the docs anchor and a --filter line in the CLI Usage block). Keep new YAML examples free of top-level checks and simulated_user keys. No template-synchronized files change."
    dependencies: ["run-selection", "cli-wiring", "diff-trend-metadata-filter"]
    acceptance_criteria:
      - "docs/evaluation.md documents the metadata field, filter syntax and every operator, missing-key behavior, --strict, and the suggested conventional keys speed, skill and category"
      - "docs/evaluation.md states the suite_sha256 decision, how filtered runs record complete=false and case_filter, the changed-filter resume refusal, and how diff and trend use --filter"
      - "README.md links to the docs section and the link anchor matches the new heading"
      - "Examples in the docs match real behavior (operators, comma lists, quoting of < and > in shells)"
      - "The docs-scanning tests still pass and task sync:check passes"
    validation_commands:
      - "grep -n 'Selecting cases: metadata and' docs/evaluation.md"
      - "grep -n 'evaluation.md#selecting-cases-metadata-and' README.md"
      - "go test ./internal/checks ./internal/eval -run 'Docs' -count=1"
      - "task sync:check"

  - id: "validate-complete"
    agent: "validator"
    description: "Read-only verification of acceptance criteria 1-19 of issue #392 against the code, tests and docs, plus the repository gates, and confirmation that frozen or unrelated paths were not modified."
    dependencies: ["docs"]
    acceptance_criteria:
      - "Each issue acceptance criterion 1-19 maps to passing tests or documented behavior (criterion-by-criterion report)"
      - "task test, task lint, task fmt:check, task sync:check and task eval:selftest all pass"
      - "No change to .kiro/agents/*.json, other files under .kairon/specs/, CHANGELOG.md, .kairon/evals/cases/ or cmd/kairon/templates/"
      - "Existing case files, rubrics and result files remain valid without edits; unfiltered eval, diff and trend output is unchanged"
    validation_commands:
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "git -P diff --stat origin/HEAD -- .kairon/evals/cases cmd/kairon/templates CHANGELOG.md"
```
