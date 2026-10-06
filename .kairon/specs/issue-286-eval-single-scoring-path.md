# Design Spec: Eval runner — one scoring path for full, resumed and single-case runs

Closes #286

## 1. Solution Approach

`internal/eval/runner.go` holds two copies of the per-case scoring and reporting logic:

- `evaluate` (used by `runSingleTestCase`, i.e. `--case`): lines ~397-508.
- `evaluateProgressive` (used by `Run` and `runWithResume` via `runProgressiveEvaluation`): lines ~1336-1480.

Each copy contains (a) the per-criterion scoring loop and (b) the per-case pass/warn/fail printout including the below-threshold criterion breakdown. The two blocks are textually identical apart from comments. The helpers they call (`scoreDeterministic`, `scoreLLMJudge`, `parseMaxScore`, `getThreshold`) are already single-copy.

Pure refactor. Extract exactly two package-level functions and make both evaluators call them:

```go
// scoreCase runs every non-cost rubric criterion against cr.ActualOutput and
// appends the CriterionScores to cr.Scores, adding LLM-judge cost to cr.JudgeCost.
func scoreCase(rubric Rubric, tc TestCase, cr *CaseResult)

// printCaseResult writes the final status line for a scored case:
// "no output" / "no scored criteria" / pass (✅) / warn (⚠️, pct>=60) / fail (❌),
// plus the breakdown of criteria scoring below 3/4 of max when pct < threshold.
func printCaseResult(out io.Writer, tc TestCase, cr CaseResult)
```

Both go in a new file `internal/eval/scoring.go` (same package, no new exports). The bodies are moved verbatim from the existing loop/printout, so scoring rules, thresholds (`getThreshold`, the 60% warn cut, the `MaxScore*3/4` breakdown cut), output text, and result JSON stay byte-for-byte identical.

### Behavior that MUST be preserved (call-site ordering differs today)

- `evaluate`: prompt/agent block -> score -> print -> `TrackTestCase` -> append to `result.Cases`. No persistence.
- `evaluateProgressive`: resume skip -> prompt/agent block -> score -> upsert into `result.Cases` -> `saveProgressiveResult` (may print `⚠️ (save failed: ...)` BEFORE the final status line) -> `TrackTestCase` -> print.

Do not reorder these. The helpers replace only the scoring loop and the printout; everything else in each function stays where it is. `scoreCase` takes `*CaseResult` so `cr.Scores` stays nil when a rubric has no non-cost criteria (preserves JSON `"scores": null`). Keep the `criterion.Type == "cost"` skip inside `scoreCase`.

### Out of scope (note only)

The duplicated prompt-assembly and agent-invocation block (`assemblePrompt` + `invokeAgent` + the ` → running agent...` / ` ❌ (prompt error)` / ` ❌ (agent failed)` printing) also exists in both `evaluate` and `evaluateProgressive`. Per the issue it is NOT touched here; a follow-up can unify it. Also not touched: scoring rules, thresholds, flags, result JSON, Docker sandbox, and the `result.Cases` append-vs-upsert difference.

## 2. Relevant Files

| File | Action |
|------|--------|
| `internal/eval/scoring.go` | Create — `scoreCase`, `printCaseResult` |
| `internal/eval/runner.go` | Modify — replace scoring loop + printout in `evaluate` and `evaluateProgressive` with calls to the helpers |
| `internal/eval/scoring_test.go` | Create — unit tests for the helpers and single-vs-progressive parity |
| `internal/eval/backend_routing_test.go` | Read-only — existing test calls `evaluate(...)` (line ~303); signature must not change |
| `Taskfile.yml` | Read-only — `eval:selftest` target |
| `internal/eval/testdata/evals/cases/selftest/{stub-basic,stub-usage}.yaml` | Read-only — self-test cases used for parity checks |

No changes to `cmd/`, `types.go`, `sandbox/`, templates, or docs (no user-visible change, so no template sync needed).

## 3. Team Orchestration

- `scoring-extract` (builder) does the refactor in `runner.go` + `scoring.go`.
- `scoring-tests` (builder) adds unit tests; depends on `scoring-extract` because it targets the new function signatures.
- `validate-parity` (validator) runs after both; it verifies the duplicated blocks are gone, runs the test suite, and compares self-test scores between base commit and PR branch and between `--case` and full runs.

Sequence: `scoring-extract` -> `scoring-tests` -> `validate-parity`. No parallelism is useful: the change is confined to one function pair.

## 4. Step-by-Step Task Breakdown

### Task 1: scoring-extract — Extract shared scoring and printing
**Acceptance Criteria**:
- `scoreCase` and `printCaseResult` exist once, in `internal/eval/scoring.go`, with bodies moved verbatim from the existing loop/printout.
- `evaluate` and `evaluateProgressive` call them; neither contains a per-criterion `for _, criterion := range rubric.Criteria` loop or the `pct >= threshold` / `pct >= 60` printout any more.
- Statement order inside both evaluators is unchanged (see "Behavior that MUST be preserved").
- `evaluate` signature unchanged; `go build ./...` and existing tests pass.
**Dependencies**: None

### Task 2: scoring-tests — Unit tests
**Acceptance Criteria**:
- Tests for `printCaseResult` cover: no output, no scored criteria (all skipped), pass, warn (60% <= pct < threshold), fail (<60%), and breakdown lines shown only when below threshold and only for non-skipped criteria scoring < 3/4 of max.
- Test for `scoreCase` covers: cost criterion skipped, deterministic path, LLM-judge path with empty output (score 0, skipped, reasoning "no output available for LLM judging"), and nil `Scores` when there are no non-cost criteria.
- Parity test: using the stub backend (helpers in `backend_routing_test.go`, e.g. `useStubBackend`, `chdirTemp`), the same case run through `evaluate` and through `evaluateProgressive` (temp results dir, `isResume=false`) yields identical `Scores` and identical status-line output.
**Dependencies**: scoring-extract

### Task 3: validate-parity — Verify no behavior change
**Acceptance Criteria**:
- (AC1) Grep/manual check: scoring loop and status printout each occur once in `internal/eval` (in `scoring.go`).
- (AC2) `task eval:selftest` on base commit (`db32c8f`) and on the PR branch produce identical per-case `case_name` + `scores` in the results JSON.
- (AC3) Running one self-test case with `--case stub-basic` yields the same `scores` as that case in the full-suite run.
- `task test`, `task lint`, `task fmt:check` pass.
**Dependencies**: scoring-extract, scoring-tests

## 5. Validation Commands

```bash
go build ./...
go vet ./internal/eval/...
go test ./internal/eval/...
task fmt:check
task lint

# AC1: each appears exactly once
test "$(grep -rn 'range rubric.Criteria' internal/eval --include=*.go | grep -v _test.go | wc -l | tr -d ' ')" = "1"
test "$(grep -rn 'no scored criteria' internal/eval --include=*.go | grep -v _test.go | wc -l | tr -d ' ')" = "1"

# AC2: base vs branch (baseline = db32c8f); compare case_name + scores only
git worktree add /tmp/kairon-base-286 db32c8f
(cd /tmp/kairon-base-286 && task eval:selftest)
task eval:selftest
# results land under internal/eval/testdata/evals/results/<timestamp>-<hash>/selftest.json in each tree
jq -S '[.cases[] | {case_name, scores}] | sort_by(.case_name)' <base selftest.json>   > /tmp/base-scores.json
jq -S '[.cases[] | {case_name, scores}] | sort_by(.case_name)' <branch selftest.json> > /tmp/branch-scores.json
diff /tmp/base-scores.json /tmp/branch-scores.json

# AC3: single-case vs full run
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals --case stub-basic selftest
jq -S '.cases[] | select(.case_name=="stub-basic") | .scores' <single-case selftest.json> > /tmp/single.json
jq -S '.cases[] | select(.case_name=="stub-basic") | .scores' <full-run selftest.json>   > /tmp/full.json
diff /tmp/single.json /tmp/full.json

# cleanup
git worktree remove --force /tmp/kairon-base-286
```

Validator must remove generated `results/` directories it created and not leave the temp worktree behind.

## 6. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "scoring-extract"
    agent: "builder"
    description: "Create internal/eval/scoring.go with scoreCase(rubric Rubric, tc TestCase, cr *CaseResult) and printCaseResult(out io.Writer, tc TestCase, cr CaseResult), moving the per-criterion scoring loop and the pass/warn/fail + below-threshold breakdown printout verbatim out of evaluate and evaluateProgressive in runner.go, and replace both inline copies with calls to the helpers. Keep statement order at each call site unchanged (progressive: score, upsert, save, TrackTestCase, print; single: score, print, TrackTestCase, append). Do not touch prompt assembly/agent invocation, scoring rules, thresholds, flags, result JSON, or sandbox."
    dependencies: []
    acceptance_criteria:
      - "internal/eval/scoring.go defines scoreCase and printCaseResult; each scoring loop and status printout exists exactly once in non-test code in internal/eval"
      - "evaluate and evaluateProgressive no longer contain 'range rubric.Criteria' or the pct>=threshold / pct>=60 printout; both call scoreCase and printCaseResult"
      - "The evaluate function signature is unchanged and existing tests (including backend_routing_test.go) still pass"
      - "Order of save/TrackTestCase/print relative to scoring is unchanged in both evaluators; cr.Scores remains nil when a rubric has no non-cost criteria"
      - "No changes outside internal/eval"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/..."
      - "test \"$(grep -rn 'range rubric.Criteria' internal/eval --include=*.go | grep -v _test.go | wc -l | tr -d ' ')\" = \"1\""
      - "test \"$(grep -rn 'no scored criteria' internal/eval --include=*.go | grep -v _test.go | wc -l | tr -d ' ')\" = \"1\""
      - "task fmt:check"

  - id: "scoring-tests"
    agent: "builder"
    description: "Add internal/eval/scoring_test.go with unit tests for printCaseResult (no output, no scored criteria, pass, warn, fail, breakdown only when below threshold) and scoreCase (cost criterion skipped, deterministic path, LLM-judge with empty output, nil Scores for no criteria), plus a parity test that runs the same stub-backed case through evaluate and evaluateProgressive and asserts identical Scores and identical status-line output."
    dependencies: ["scoring-extract"]
    acceptance_criteria:
      - "printCaseResult tests cover all five status outcomes and the below-threshold breakdown filtering (skipped criteria and criteria at or above 3/4 of max are omitted)"
      - "scoreCase tests cover cost-criterion skip, deterministic scoring, empty-output LLM-judge skip with reasoning 'no output available for LLM judging', and nil Scores when no non-cost criteria"
      - "Parity test proves evaluate and evaluateProgressive produce identical Scores and identical status line for the same case"
      - "Tests use the stub backend and a temp dir; no kiro-cli or network is invoked"
    validation_commands:
      - "go test ./internal/eval/... -run 'Scor|PrintCase|Parity' -v"
      - "go test ./internal/eval/..."
      - "task fmt:check"

  - id: "validate-parity"
    agent: "validator"
    description: "Verify the refactor is behavior-preserving: duplicated scoring/printing blocks are gone (AC1); task eval:selftest on base commit db32c8f and on the branch produce identical per-case case_name+scores in results JSON (AC2); stub-basic run with --case scores identically to the same case in the full self-test run (AC3); tests, lint and fmt pass. Remove generated results dirs and the temp base worktree afterwards."
    dependencies: ["scoring-extract", "scoring-tests"]
    acceptance_criteria:
      - "AC1: scoreCase/printCaseResult are the only scoring loop and status printout in internal/eval non-test code; evaluate and evaluateProgressive both call them"
      - "AC2: jq-extracted [case_name, scores] from selftest.json are identical between base commit db32c8f and the PR branch"
      - "AC3: scores for stub-basic from a --case run equal scores for stub-basic from a full task eval:selftest run"
      - "git diff shows no changes outside internal/eval (plus the spec/artifact files) and no changes to scoring rules, thresholds, flags, or result JSON schema"
      - "task test, task lint and task fmt:check pass"
    validation_commands:
      - "go test ./internal/eval/..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals --case stub-basic selftest"
      - "git diff --stat HEAD -- . ':!internal/eval' ':!.kairon'"
```
