# Design Spec: Evals: multi-turn test cases

Closes #317

Source issue: https://github.com/jbrinkman/kairon/issues/317 (depends on #314, closed: the eval checks work in `internal/eval`).

## 1. Problem

The planner is a gated conversation (draft, approval, label confirmation, creation). Today an eval case is one
`input` and one agent call, so no case can test a gate or any behaviour that depends on the user's answers.
The harness needs scripted multi-turn cases, with checks that can say *when* something happened ("no
`gh issue create` after turn 1, one after turn 3").

Scope: `internal/eval` (including `internal/eval/testdata/evals/`), `internal/inference` where the turn contract
has to cross the backend boundary, and `docs/evaluation.md`. Out of scope: new check types, judge changes,
simulated users (turns are scripted), agent cases under `.kairon/evals/`.

## 2. What exists today (verified in the tree)

- `TestCase.Input` (string) + `Setup []SetupEntry` become one prompt via `assemblePrompt` (`runner.go`).
  `executeCase` (`execute_case.go`) does: sandbox gate -> `assemblePrompt` -> `newCaseWorkspace` -> **one**
  `invokeAgent` -> `scoreCaseFn` -> workspace removal.
- `inference.Request.Turn` already exists (`inference.go`, "0 for now") and the **stub backend already selects
  `req.Stub.Turns[req.Turn]`** (`stub.go`). `newAgentRequest` leaves `Turn` at 0 with a comment that multi-turn
  selection "lands with E9". The stub half of AC 4 is therefore wiring, not new backend logic.
- `KiroCLIAgentCommand` (`inference/exec.go`) builds `chat --agent A --no-interactive (--trust-all-tools |
  --trust-tools=…) [--model M]` and is shared by the native backend and the container transport.
- `CaseResult.ActualOutput` is the single output; `scoreCase` (`scoring.go`) builds one `CheckInput{Dir, Output,
  GHLog, GHLogOversized, Base}` after the agent finished. `.eval/gh.log` is **one append-only file**; the fake
  `gh` (sandbox) appends to it, native stub cases write it by hand via `commands`.
- A `--sandbox` case creates a **fresh container per agent call** (`invokeAgentInContainer`), whose `$HOME` is a
  tmpfs. `kiro-cli` keeps conversation state under `$HOME`, so `--resume` cannot work across today's per-call
  containers. This is the one structural obstacle to AC 2 under `--sandbox` (see 4.6).
- `loadCases` decodes with `KnownFields(true)`, so unknown keys are load errors; `validateCaseFields(tc, file)`
  holds per-case validation and names the case as `case %q (%s): …`. `ValidateChecks` / `validateCheck` +
  `checkSpecs` reject fields that do not belong to a check type.
- The self-test counts are pinned in tests: `selftest` = 16 cases (`selftest_test.go`:
  `5+11`; `sandbox_workspace_test.go`: 16), `selftest-fail` = 12 (`selftest_checks_test.go`).

## 3. Solution approach (decisions)

### 3.1 Case schema

```yaml
name: planner-gates
agent: planner
setup: [ … ]               # prepended to turn 1 ONLY
turns:                     # replaces `input`
  - "I want a feature that exports reports as CSV"
  - "Looks good, go on"
  - "Yes, those labels are right"
stub:
  turns:                   # exactly one entry per turn (response + commands + …), same index
    - response: "…draft…"
    - response: "…label confirmation…"
    - commands: ["echo 'gh issue create --title t' >> .eval/gh.log"]
      response: "…created…"
checks:
  - {criterion: gate_adherence, type: gh_log_not_contains, pattern: 'issue create', turn: 1}
  - {criterion: gate_adherence, type: gh_log_contains,     pattern: 'issue create', turn: 3}
```

- `TestCase.Turns []string \`yaml:"turns,omitempty"\``. `turns` and `input` are mutually exclusive.
  A non-blank `input` together with a non-nil `turns` is a **load error naming the case**:
  `case "x" (file.yaml): turns and input are mutually exclusive; use turns for a multi-turn case`.
- Also rejected, each naming the case: `turns: []` (present but empty), a blank entry (`turns[2] is empty`).
- Add one helper, `func (tc TestCase) userTurns() []string`: returns `Turns`, or `[]string{Input}` for a classic
  case. Every consumer iterates this; a classic case is simply "a case with one turn", so there is a single code
  path. (Do **not** add a normalising pass that rewrites `Input` into `Turns` at load: result files and the judge
  prompt still need to tell the two forms apart.)
- `stub.turns`: for a `turns` case with a `stub`, `len(stub.turns)` must equal `len(turns)` (load error
  otherwise, naming the case and both counts). Classic `input` cases keep today's behaviour (only entry 0 is
  used; no new validation, so nothing existing breaks).
- Setup entries: `assemblePrompt(tc.Setup, turns[0])` for turn 1; turns 2..n are sent verbatim
  (`assemblePrompt(nil, turns[i])` returns the input unchanged).

### 3.2 Turn numbering (one conversion point)

- YAML (`turn`) is **1-based**, as the issue specifies. `inference.Request.Turn` and `stub.turns[i]` are
  **0-based indexes**. The only conversion lives where the harness builds the request
  (`req.Turn = turnNumber-1`). Docs state both conventions side by side.

### 3.3 Backend contract (what makes it suit a future direct-API backend)

The harness is the only thing that knows the script; the backend owns conversation continuity. The contract
added to `inference.Request`:

- `Turn` (0-based) is the position of this user message in the conversation. `Prompt` carries **only that turn's
  user message** (turn 1 additionally carries the setup context). It never carries earlier turns or earlier
  answers, and the harness never passes a "resume" flag or a transcript.
- A backend must continue the conversation itself when `Turn > 0`; it decides *how*:
  - `kiro-cli`: `Turn > 0` => add `--resume` (resumes the previous conversation in the same working
    directory; the workspace is unique per case, so "the most recent conversation in this dir" is this case's).
  - `stub`: index `Turns[Turn]`.
  - a future direct-API backend: keep a message list keyed by the conversation (the per-case `WorkDir`) and
    append; `Turn == 0` starts a new list. Nothing in the harness or the YAML changes for it.
- This is why `Request` gets **no** `Resume bool`: "resume" is a kiro-cli mechanism, "turn" is the harness
  concept. `P2` only relies on `turns` / `turn`, which are backend-neutral.
- Known limit to document (not solved here): under `--sandbox`, non-`kiro-cli` backends run as a fresh
  `kairon inference-exec` process per exec, so a future in-memory-history backend cannot hold history inside the
  container; it would have to run host-side like the judge. The stub is stateless (index based) so it is fine.

### 3.4 Check semantics

- `Check.Turn *int \`yaml:"turn,omitempty"\`` (pointer so an explicit `turn: 0` is detectable and rejected;
  `turn` is 1-based and must be >= 1).
- Allowed **only** on `output_contains`, `output_not_contains`, `gh_log_contains`, `gh_log_not_contains`
  (`checkFields` / `checkSpecs` get a `turn` flag; any other type with `turn` gets the existing
  `turn is not valid for type <t>` error). Reason, to be stated in the docs: the workspace is only inspected
  once, at the end of the case, so a `file_*`, `changed_files` or `command` check cannot honestly be pinned to
  an earlier turn. Failing loudly beats a `turn: 1` that silently checks the final workspace.
- Default (no `turn`) = the **last** turn: identical to today's behaviour for classic cases.
- Range: `turn <= number of turns of the case` (a classic case has 1, so `turn: 1` is legal there). Out of range
  is a load error naming the case, the 1-based check number and the type. Because `ValidateChecks` has no case
  context, this range check is a case-level step (`validateCheckTurns(tc, file)`) called from `loadCases`.
- Evaluation: `CheckInput` gains per-turn evidence; `EvaluateChecks` stays a plain function with no harness
  knowledge:

  ```go
  type TurnEvidence struct {
      Output         string // the agent's output for the turn
      GHLog          string // .eval/gh.log as it stood at the END of the turn (cumulative)
      GHLogOversized bool
  }
  type CheckInput struct { …existing…; Turns []TurnEvidence } // Turns[i] is turn i+1
  ```

  `output_*` / `gh_log_*` with `Turn` set read `Turns[Turn-1]`; without `Turn`, or when `Turns` is empty, they read
  `Output` / `GHLog` exactly as now. A hand-built check with an out-of-range turn yields a failed result with a
  clear detail (never a panic, never a silent pass), consistent with the "invalid check" rule.
- `checkLabel` appends ` turn=N` when set, so reasoning and `checks[].label` show which turn failed.

### 3.5 gh-log "up to the end of that turn"

`gh.log` is a single append-only file, so per-turn visibility is a **snapshot taken after each turn returns,
before the next turn starts**: `readGHLog(workspaceDir)` (existing, cap + oversize flag) after every turn,
stored in `TurnEvidence.GHLog`. Snapshotting content (rather than recording a byte offset into the final file)
means a later turn that rewrites or truncates the log cannot change what an earlier turn "saw". The last
turn's snapshot equals the final log, which keeps the default path identical to today.

### 3.6 Results

- `CaseResult.TurnOutputs []string \`json:"turn_outputs,omitempty"\`` lists every turn's output in order
  (AC 5). Present only for cases declared with `turns` (a classic case's JSON shape does not change, so old
  files load and the native/container parity tests keep comparing like with like).
- `CaseResult.ActualOutput` = the **last** turn's output (unchanged meaning for `diff`, "no output" handling,
  heuristics and the judge).
- `inference.CallRecord` gains `Turn int \`json:"turn,omitempty"\`` (1-based, set only for `turns` cases) so the
  N agent records in `calls[]` are distinguishable. Judge records are unchanged.
- `AgentCost` = sum of the per-turn costs (`CostInfo.Add`; `reported` only if every turn is `reported`).
- Judge input: the judge prompt template is **not** changed. `scoreLLMJudge` substitutes a rendered "INPUT" for a
  multi-turn case: `Turn 1 (user): …\n\nTurn 2 (user): …` (a classic case still passes `Input` verbatim), and
  `ACTUAL OUTPUT` stays the last turn's output. Documented as a known limitation; a transcript-aware judge is a
  separate issue (judge changes are out of scope).

### 3.7 Failure semantics

If turn *k* fails (backend error, timeout, trust-resolution failure, non-zero exit):

- turns *k+1..n* are **not** run;
- `ActualOutput = ""` (so the existing "agent produced no output; checks not run" path applies: every checked
  criterion records `0/<total>`, no check runs);
- `TurnOutputs` holds the outputs of turns 1..k-1 (useful evidence), `Calls` holds records for turns 1..k (the
  failed one with its `error`), `ErrorContext` is the failing turn's, and the printed error is prefixed
  `turn k/n:`.

Each turn gets the full case/sandbox timeout (`timeout:` is **per turn**, not for the case); documented.

### 3.8 Same conversation, same workspace (AC 2)

- Workspace: already one per case; every turn is invoked against the same `caseWorkspace` (native `WorkDir`;
  container bind mounts). Stub `commands` of turn 2 therefore see turn 1's files.
- Native `kiro-cli`: turns 2..n run `kiro-cli chat --agent A --no-interactive --resume …` in the same workspace
  directory (implemented in `KiroCLIAgentCommand` when `req.Turn > 0`; position: directly after
  `--no-interactive`). Turn 1 and every classic case produce byte-identical arguments to today.
- **Container**: add a per-case *session*. `invokeAgentInContainer` is split into open / run / close: open
  = mounts + host config + create + start + `ValidateKiroCLI` (once); run = `runAgentInContainer` (once per
  turn, same container, so the `$HOME` tmpfs holding `kiro-cli`'s conversation state survives between turns);
  close = `CleanupWithDebugInfo`. The existing one-shot function is re-expressed as open+run+close, so a
  classic/one-turn case is unchanged. `executeCase` opens a session only when a container config is present
  **and** the case has more than one turn. Seam for tests: `var openContainerSession = openAgentContainerSession`
  returning an interface `{ Run(req) (Response, *ErrorContext, error); Close(failed bool) }`; the existing
  `invokeInContainer` seam keeps working for one-shot calls.

## 4. Relevant files

Modify:
- `internal/inference/inference.go` — redefine/document `Request.Turn`; no new fields.
- `internal/inference/exec.go` — `--resume` when `req.Turn > 0` in `KiroCLIAgentCommand`.
- `internal/inference/record.go` — `CallRecord.Turn`.
- `internal/eval/types.go` — `TestCase.Turns`, `userTurns()`, `CaseResult.TurnOutputs`, unexported per-turn gh
  snapshots on `CaseResult`.
- `internal/eval/checks.go` — `Check.Turn`, `checkFields.turn`, `checkSpecs`, `validateCheck` (turn >= 1, allowed
  types), `CheckInput.Turns`, `TurnEvidence`.
- `internal/eval/checks_eval.go` — per-turn evidence lookup in `runCheck`, `checkLabel` turn suffix.
- `internal/eval/workspace.go` — `validateCaseFields`: turns/input rules, stub-turn count, check-turn range.
- `internal/eval/execute_case.go` — turn loop, per-turn prompts, snapshots, output/cost/call aggregation,
  failure semantics, progress output, container session lifecycle.
- `internal/eval/runner.go` — `callOpts.Turn` -> `req.Turn` in `newAgentRequest`/`invokeAgent` (remove the
  "E9" TODO comment), container session split (4.8), judge input rendering in `scoreLLMJudge`.
- `internal/eval/scoring.go` — build `CheckInput.Turns` from `TurnOutputs` + snapshots.
- `internal/eval/perf.go` — use the first user turn (`userTurns()[0]`) instead of `tc.Input` so the performance
  investigation does not send an empty prompt for a `turns` case (no other perf change).
- `internal/eval/testdata/evals/cases/selftest/` and `…/selftest-fail/` — new cases (section 6).
- Tests that pin counts: `internal/eval/selftest_test.go` (16 -> 18), `internal/eval/sandbox_workspace_test.go`
  (16 -> 18), `internal/eval/selftest_checks_test.go` (selftest-fail 12 -> 13 and the passing/failing case lists).
- `docs/evaluation.md`.

Create (tests; names are suggestions): `internal/inference/exec_turn_test.go`,
`internal/eval/turns_schema_test.go`, `internal/eval/checks_turn_test.go`, `internal/eval/multi_turn_test.go`,
`internal/eval/container_session_test.go`.

Not modified: `.kairon/evals/**` (out of scope; therefore **no template sync** is needed, `task sync:check` is
unaffected), judge prompt template, check types.

## 5. TDD plan (per AGENTS.md: failing test first, for the right reason, committed with the implementation)

Each task below lists the tests to write first and the failure to expect before implementing.

1. **inference-turn-contract** — tests: `TestKiroCLIAgentCommand_Resume` (Turn 0 args unchanged byte for byte;
   Turn 1 contains `--resume` right after `--no-interactive`, with and without `--model` / `--trust-tools`);
   `TestStubTurnSelection` for `Turn` 0/1/2 incl. commands per turn (stub already supports it: this is a
   characterisation test). Expected pre-implementation failure: the `--resume` assertions fail because the args
   do not contain `--resume` (not a compile error: the test only uses existing symbols). `CallRecord.Turn` JSON
   test (`turn` omitted when 0).
2. **check-turn-field** — tests in `checks_turn_test.go`: validation (`turn` on `file_exists` rejected with
   `turn is not valid for type file_exists`; `turn: 0` rejected; allowed types accepted), `EvaluateChecks` with
   `Turns` evidence (turn 1 `gh_log_not_contains` passes while turn 3 `gh_log_contains` passes on the same
   input; default = last; out-of-range -> failed result with a detail), label contains `turn=2`. Expected
   failure: unknown field / assertion on missing behaviour. (Use composite literals for `Check{Turn: intPtr(2)}`
   only after the field exists; write the YAML-decoding tests first so the first red is behavioural.)
3. **case-turns-schema** — tests in `turns_schema_test.go` using `t.TempDir()` evals dirs (pattern:
   `configure`/`cfg.evalsDir`, as in `config_test.go`): both `turns` and `input` -> error contains the case name
   and `mutually exclusive` (AC 1); `turns: []`; blank entry; stub count mismatch; `turn: 4` on a 3-turn case;
   `turn: 2` on a classic case; classic case unchanged; `userTurns()`; judge INPUT rendering for turns.
4. **multi-turn-execution** — tests in `multi_turn_test.go` against the stub backend and the existing
   `executeCase` helpers (`execRubric`, `stubCase` style): 3-turn case yields `TurnOutputs` of len 3 with each
   scripted response, `ActualOutput` = turn 3, per-turn `Calls[i].Turn == i+1`, cost summed; setup text appears
   in turn 1's prompt only (recording backend registered via the test seam used by `backend_routing_test.go`;
   assert `Prompt` and `Turn` per request); turn-1 `gh_log_not_contains` passes and turn-3 `gh_log_contains`
   passes; **same workspace** (turn 2's command `cat turn1.txt` succeeds); failure at turn 2 stops turn 3,
   `ActualOutput == ""`, `TurnOutputs` len 1, error prefixed `turn 2/3`; end-to-end native `kiro-cli` wiring with
   the existing `installFakeKiroCLI` helper: call 1 has no `--resume`, call 2 has `--resume`, stdin of each call is
   that turn's message only. Expected pre-implementation failure: `TurnOutputs` empty / only one agent call.
5. **container-session** — tests in `container_session_test.go` with a fake `openContainerSession`: one open and
   one close for a 3-turn case, `Run` called 3 times with `Turn` 0,1,2, close(failed=true) when a turn fails,
   one-turn and classic cases still use the one-shot `invokeInContainer` seam (no session). Daemon-gated
   coverage comes from the self-test cases (task 6) through the existing `TestSelftestSandbox`.
6. **selftest-fixtures** — new cases (section 6) and the count updates; the red state is the count/assertion
   failure before the cases exist.
7. **docs** — no test first (docs exemption).

## 6. Self-test cases (AC 1, 3, 4, 5)

`internal/eval/testdata/evals/cases/selftest/`:
- `multi-turn-gh-log.yaml` (AC 3/4/5): 3 `turns`; `stub.turns` with distinct scripted responses (`TURN-ONE-MARKER`,
  `TURN-TWO-MARKER`, `TURN-THREE-MARKER`); turn 3's stub turn has `commands:
  ['echo "gh issue create --title t" >> .eval/gh.log']` (same technique as `check-gh-log-contains`, since the
  native self-test has no fake gh). Checks on `structural_completeness`: `gh_log_not_contains 'issue create'
  turn: 1`; `gh_log_not_contains turn: 2`; `gh_log_contains 'issue create' turn: 3`; `gh_log_contains` with no
  `turn` (defaults to last); `output_contains TURN-ONE-MARKER turn: 1`; `output_contains TURN-TWO-MARKER
  turn: 2`; `output_contains TURN-THREE-MARKER` (default last); `output_not_contains TURN-ONE-MARKER`
  (default last = turn 3). Asserted by tests: `turn_outputs` has 3 entries equal to the scripted responses
  (AC 4/5), all checks pass.
- `multi-turn-workspace.yaml`: 2 turns; turn 1 writes `turn1.txt`; turn 2's command is
  `test -f turn1.txt && echo second > turn2.txt` (fails the stub, hence the case, if the workspace were not
  shared); `file_exists` for both files (workspace checks, no `turn`). Also uses `setup` with a `text` entry so
  the "turn 1 only" rule is exercised end to end (asserted precisely in the unit test with a recording backend).

`internal/eval/testdata/evals/cases/selftest-fail/`:
- `multi-turn-wrong-turn.yaml`: same shape, but asserts `gh_log_contains 'issue create' turn: 1` for a call made in
  turn 3 -> expected to **fail**, proving turn scoping is real (the check would pass if it saw the final log).

AC 1's invalid case **cannot** live in `cases/selftest/` (a load error is fatal for the whole agent directory).
It is written as a YAML string into a `t.TempDir()` evals dir inside `turns_schema_test.go`; the test asserts the
`loadCases` error names the case and contains `turns and input are mutually exclusive`.

Counts to update in tests: `selftest` 16 -> 18, `selftest-fail` 12 -> 13. Native and `--sandbox` runs of the new
cases must match (`TestSelftestSandbox`, daemon-gated), which is what exercises the container session.

## 7. AC traceability

| AC | Delivered by | Verified by |
|----|--------------|-------------|
| 1 `turns` replaces `input`; setup on turn 1 only; both -> named error | 3.1, `validateCaseFields`, executeCase prompts | `turns_schema_test.go` (error names case); recording-backend prompt test |
| 2 same conversation + workspace; kiro-cli `--resume` | 3.3, 3.8, `KiroCLIAgentCommand`, container session | `TestKiroCLIAgentCommand_Resume`; fake-`kiro-cli` end-to-end test; **manual** PINEAPPLE run (below) |
| 3 `turn` on checks; default last; gh-log up to end of turn | 3.4, 3.5 | `multi-turn-gh-log` case + `checks_turn_test.go` |
| 4 one `stub.turns` entry per turn | existing stub + `req.Turn` wiring + count validation | `multi-turn-gh-log` results show each scripted output |
| 5 results record every turn's output | 3.6 `turn_outputs` | test asserts JSON `turn_outputs` has 3 entries |

### Manual verification for AC 2 (cannot run in CI; needs authenticated `kiro-cli`)

1. Copy `.kairon/evals` to a temp dir (`cp -r .kairon/evals /tmp/evals317`), add
   `cases/planner/remember-word.yaml` there with `turns: ["Remember the word PINEAPPLE. Reply only OK.", "What
   was the word? Reply with the word only."]` and `checks: [{criterion: <a non-cost planner criterion>,
   type: output_contains, pattern: PINEAPPLE}]`.
2. `kairon eval --evals-dir /tmp/evals317 planner remember-word` (native) and the same with `--sandbox`.
3. Expect `turn_outputs[1]` to contain `PINEAPPLE` and the check to pass in both modes.
   Record the result in the PR description. **Open point to confirm during this run**: the issue verified
   `--no-interactive --resume`; combining it with `--agent <name>` (which the harness always passes) is not
   covered by that verification. If `--resume` rejects or ignores `--agent`, adjust `KiroCLIAgentCommand` for
   `Turn > 0` only (the resumed session is already bound to its agent); turn 1 stays unchanged.

## 8. Documentation (`docs/evaluation.md`)

- Test Case Format: `turns` field (and mutual exclusion), `stub.turns` "one entry per turn", `timeout` is per turn.
- Checks: `turn` field, which types accept it, default last, why workspace checks cannot, how gh-log snapshots
  work; add `turn` to the check-fields wording ("a field that does not belong to the type is a load error").
- New section "Multi-turn cases" (semantics, numbering 1-based YAML vs 0-based `Request.Turn`, failure
  semantics, judge input rendering, per-turn timeout, native `--resume`, one container per case across turns,
  the backend contract of 3.3 for future backends).
- Result JSON: `turn_outputs`, `calls[].turn`.
- Stub Case Fields: replace "Only `turns[0]` is used today" with the per-turn rule (classic cases still use entry 0).
- Inference Backends / Adding a New Backend: the `Request.Turn` contract.
- Self-Test: new case files and the updated counts (`selftest` eighteen, `selftest-fail` thirteen; the "sixteen"
  and "twelve" sentences and the directory listing).
- Native caveat for gh-log checks applies per turn too (no real log natively).

## 9. Team orchestration

```
inference-turn-contract ─┐
check-turn-field ────────┼─► multi-turn-execution ─► container-session ─┐
case-turns-schema ───────┘            │                                ├─► selftest-fixtures ─► docs ─► validate
   (needs check-turn-field)           └────────────────────────────────┘
```

- Independent, can run in parallel: `inference-turn-contract` and `check-turn-field` (disjoint files).
- `case-turns-schema` needs `Check.Turn` (it validates check turns against the case): after `check-turn-field`.
- `multi-turn-execution` needs all three. `container-session` follows it (same files: `execute_case.go`,
  `runner.go`). Fixtures need the executor; docs describe the final behaviour; the validator runs last.
- One PR, all five ACs; no phases. The `Closes #317` footer goes in the PR/commit description. Commit tests with
  their implementation (no standalone red commits) and name the first-failing test in the commit message.

## 10. Risks and notes

- `kiro-cli --resume` + `--agent` combination unverified (see 7); the `--resume` position in argv is the only
  thing to adjust if it matters.
- Native runs write conversations into the user's real `kiro-cli` store (one per case workspace dir). Acceptable
  and unavoidable for `--resume`; mention in docs.
- Memory: gh-log snapshots are capped at 10 MiB each (existing cap); only stored for `turns` cases.
- Existing result files and classic cases must be unchanged in shape: guard with the existing parity tests
  (`TestProvenanceParitySandbox`, `TestSelfTestChecksLegacyScoresUnchanged`).

## 11. Validation commands

```
go build ./...
go vet ./...
test -z "$(gofmt -l .)"
go test ./internal/inference/... -count=1
go test ./internal/eval/... -count=1
go test -race ./internal/eval/... ./internal/inference/... -count=1
task eval:selftest
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail
task eval:selftest:sandbox          # skips cleanly without a container daemon
task sync:check
task lint && task fmt:check && task test
```

## 12. Machine-readable plan

```kiro-plan
version: "1.0"
tasks:
  - id: "inference-turn-contract"
    agent: "builder"
    description: "Define the turn contract at the backend boundary (TDD): write failing tests first, then make kiro-cli add --resume when Request.Turn > 0 (directly after --no-interactive; Turn 0 and classic cases byte-identical), document Request.Turn as the 0-based position of this user message with Prompt carrying only that turn's message, and add CallRecord.Turn (json 'turn', omitempty, 1-based). No Resume field is added to Request."
    dependencies: []
    acceptance_criteria:
      - "A test asserting KiroCLIAgentCommand with Turn 1 contains --resume after --no-interactive was seen failing for that reason (missing flag) before implementation"
      - "KiroCLIAgentCommand output for Turn 0 is byte-identical to the previous output for all existing combinations (trust-all, --trust-tools, with and without --model)"
      - "Turn > 0 adds --resume for both the native backend and the container transport (both use KiroCLIAgentCommand)"
      - "CallRecord marshals 'turn' only when non-zero; records without it still unmarshal"
      - "A stub characterisation test shows Turns[req.Turn] selection and per-turn commands for Turn 0, 1, 2"
      - "Request.Turn comment states the backend contract (message only, backend owns continuity, suits a history-keeping direct-API backend)"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/inference/... -count=1"
      - "go vet ./internal/inference/..."

  - id: "check-turn-field"
    agent: "builder"
    description: "Add the per-check 'turn' field (TDD): Check.Turn *int (yaml 'turn'), allowed only on output_contains, output_not_contains, gh_log_contains and gh_log_not_contains (checkFields/checkSpecs), validated >= 1 (explicit 0 rejected), CheckInput.Turns []TurnEvidence{Output, GHLog, GHLogOversized}, per-turn lookup in runCheck (no turn or empty Turns => existing Output/GHLog; out-of-range => failed result with clear detail), and ' turn=N' in checkLabel."
    dependencies: []
    acceptance_criteria:
      - "Tests written first fail for the behavioural reason (turn accepted/ignored, evidence not used) rather than a compile error where feasible"
      - "turn on file_exists/file_absent/file_contains/file_not_contains/changed_files/command is a load error 'turn is not valid for type <t>'; turn: 0 and negative values are rejected"
      - "With Turns evidence of 3 turns, a gh_log_not_contains 'issue create' turn:1 passes while gh_log_contains 'issue create' turn:3 passes on the same input; a check without turn uses the last turn"
      - "A hand-built check with turn beyond len(Turns) returns a failed CheckResult with a detail naming the turn, never a panic or pass"
      - "checkLabel includes turn=N only when turn is set; existing labels unchanged"
      - "All pre-existing checks tests pass unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Check|Turn' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "case-turns-schema"
    agent: "builder"
    description: "Add TestCase.Turns ([]string, yaml 'turns') and userTurns() (Turns, else []string{Input}); extend validateCaseFields/loadCases so: turns+non-blank input is a load error naming the case ('turns and input are mutually exclusive'); empty turns list and blank entries are errors naming the case; for a turns case with a stub, len(stub.turns) must equal len(turns); every check 'turn' must be <= number of turns (classic case = 1). Render a multi-turn judge INPUT ('Turn N (user): ...') in scoreLLMJudge without changing the judge template; make perf.go use userTurns()[0]. Tests use t.TempDir() evals dirs (AC 1 invalid case cannot live in testdata cases/selftest because a load error is fatal for the directory)."
    dependencies: ["check-turn-field"]
    acceptance_criteria:
      - "A loadCases test with a case that has both turns and input fails and the error contains the case name and 'turns and input are mutually exclusive' (AC 1)"
      - "turns: [], a blank turn entry, stub.turns count mismatch for a turns case, a check turn above the turn count, and turn: 2 on a classic case are all load errors naming the case; turn: 1 on a classic case loads"
      - "Classic input cases (including those with a stub of more than one entry) load exactly as before"
      - "userTurns() returns Turns or the single Input"
      - "Judge prompt for a turns case contains the labelled user turns and the last turn's output; the judge prompt for a classic case is byte-identical to before"
      - "perf.go no longer sends an empty prompt for a turns case"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Turns|Schema|Judge|LoadCases' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "multi-turn-execution"
    agent: "builder"
    description: "Run the turns in executeCase (TDD): add callOpts.Turn and set req.Turn in newAgentRequest/invokeAgent (convert 1-based YAML to 0-based index in one place, drop the 'E9' TODO); build prompts with setup on turn 1 only; invoke each turn against the same caseWorkspace; after each turn append its output to CaseResult.TurnOutputs (json 'turn_outputs', only for turns cases) and snapshot .eval/gh.log via readGHLog into unexported per-turn evidence; set CallRecord.Turn (1-based) for turns cases; sum AgentCost across turns with CostInfo.Add; ActualOutput = last turn output; on a failing turn stop, set ActualOutput empty, keep earlier TurnOutputs and Calls, prefix the error with 'turn k/n:'; print 'turn k/n' progress; have scoreCase build CheckInput.Turns from TurnOutputs and the snapshots. Classic cases take the same path as a one-turn case with unchanged output."
    dependencies: ["inference-turn-contract", "check-turn-field", "case-turns-schema"]
    acceptance_criteria:
      - "A 3-turn stub case produces turn_outputs with exactly the 3 scripted responses in order and ActualOutput equal to the third (AC 4, AC 5)"
      - "A recording backend sees Turn 0,1,2 and a prompt per request containing only that turn's message; setup context appears in turn 1's prompt only (AC 1)"
      - "In a 3-turn case whose third stub turn appends 'gh issue create' to .eval/gh.log, a turn:1 gh_log_not_contains 'issue create' check passes and a turn:3 gh_log_contains 'issue create' check passes (AC 3)"
      - "Turn 2's stub command can read a file written by turn 1 (same workspace)"
      - "A failure in turn 2 of 3 does not run turn 3, leaves ActualOutput empty, keeps TurnOutputs of length 1, records 2 agent calls with the failed one carrying an error, and the printed error starts with 'turn 2/3'"
      - "With a fake kiro-cli first on PATH (installFakeKiroCLI), the first call has no --resume, the second has --resume, and each call's stdin is only that turn's message (AC 2 wiring)"
      - "Results for classic cases (no turns) have no turn_outputs and no calls[].turn; the existing parity and legacy-score tests pass unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'MultiTurn|ExecuteCase|Scoring|Checks' -count=1"
      - "go test ./internal/eval/... ./internal/inference/... -count=1"

  - id: "container-session"
    agent: "builder"
    description: "Keep one container per multi-turn case under --sandbox so kiro-cli conversation state in the $HOME tmpfs survives between turns (TDD with a fake session): split invokeAgentInContainer into open (mounts, host config, create, start, ValidateKiroCLI once), Run (runAgentInContainer per turn) and Close(failed) behind an interface and a seam 'openContainerSession'; re-express the one-shot invokeAgentInContainer as open+Run+Close; executeCase/invokeAgent use a session only when cConfig != nil and the case has more than one turn (callOpts.Session), otherwise the existing invokeInContainer path is used unchanged."
    dependencies: ["multi-turn-execution"]
    acceptance_criteria:
      - "A 3-turn container-mode test with a fake session shows exactly one open, three Run calls with Turn 0,1,2 and one Close"
      - "When a turn fails, Close is called with failed=true (debug container preserved as today) and later turns do not run"
      - "One-turn and classic cases under cConfig still go through the invokeInContainer seam and no session is opened (existing container_agent_test.go/container_runner_test.go tests pass unchanged)"
      - "Container timeout, trust-set resolution (fail closed) and WorkDir handling are applied per turn exactly as for a single call"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Container|Session|Trust' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "selftest-fixtures"
    agent: "builder"
    description: "Add the self-test fixtures and update pinned counts: cases/selftest/multi-turn-gh-log.yaml (3 turns, distinct scripted responses, turn-scoped gh_log and output checks, default-last checks), cases/selftest/multi-turn-workspace.yaml (2 turns plus a setup text entry, shared workspace via files), cases/selftest-fail/multi-turn-wrong-turn.yaml (turn-1 gh_log_contains for a turn-3 call, expected to fail). Update selftest 16->18, sandbox_workspace_test 16->18, selftest-fail 12->13 and the passing/failing case lists, add assertions that multi-turn-gh-log results JSON lists 3 outputs equal to the scripted responses. The AC 1 invalid case stays in the TempDir test, not in testdata."
    dependencies: ["container-session"]
    acceptance_criteria:
      - "task eval:selftest passes all 18 selftest cases with every score at maximum and no ErrorContext"
      - "selftest-fail records multi-turn-wrong-turn as a failed case whose reasoning names the gh_log_contains check with turn=1"
      - "The results JSON of multi-turn-gh-log has turn_outputs with 3 entries matching the stub responses, and calls[] has 3 agent records with turn 1..3 (AC 4, AC 5)"
      - "TestSelftestSandbox (daemon-gated) is updated for the new counts and compares native and sandbox results for the multi-turn cases; it skips cleanly without a daemon"
      - "No file under .kairon/evals or cmd/kairon/templates is changed (task sync:check still passes)"
    validation_commands:
      - "go test ./internal/eval/ -run 'SelfTest|Selftest' -count=1"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail"
      - "task eval:selftest:sandbox"
      - "task sync:check"

  - id: "docs"
    agent: "builder"
    description: "Update docs/evaluation.md (live doc, no test required): turns field and mutual exclusion, stub.turns one-entry-per-turn rule (replace 'Only turns[0] is used today'), per-turn timeout, check 'turn' field with the four allowed types and why workspace checks cannot take it, gh-log per-turn snapshots, a 'Multi-turn cases' section (1-based YAML vs 0-based Request.Turn, failure semantics, judge input rendering, native --resume, one container per case, backend contract for a future direct-API backend and its sandbox limit), result JSON turn_outputs and calls[].turn, the Request.Turn contract in Adding a New Backend, the self-test listing and counts (eighteen / thirteen) and the manual PINEAPPLE verification procedure."
    dependencies: ["selftest-fixtures"]
    acceptance_criteria:
      - "Every behaviour described in section 3 of the spec appears in docs/evaluation.md and matches the implementation"
      - "No stale statements remain ('Only turns[0] is used today', 'sixteen cases', 'twelve cases', single-output wording for multi-turn)"
      - "The case-file field list includes turns and the Checks table/wording includes turn"
      - "No edits to .kairon/specs, CHANGELOG.md or other historical artifacts"
    validation_commands:
      - "grep -n 'turns' docs/evaluation.md"
      - "grep -n 'turn_outputs' docs/evaluation.md"
      - "! grep -n 'Only `turns\\[0\\]` is used today' docs/evaluation.md"

  - id: "validate-complete"
    agent: "validator"
    description: "Read-only verification that issue #317 acceptance criteria 1, 3, 4 and 5 are met by automated tests, that AC 2 is covered by the argv/stdin wiring tests plus the documented manual procedure, that classic single-input cases are unchanged, and that the TDD and template-sync rules of AGENTS.md hold."
    dependencies: ["docs"]
    acceptance_criteria:
      - "AC 1: loadCases on a case with both turns and input fails and the error names the case; setup text is only in turn 1's prompt"
      - "AC 2: turns 2..n run with --resume in the same workspace directory (native) and in the same container (sandbox) per the tests; manual PINEAPPLE procedure documented"
      - "AC 3: the 3-turn self-test case passes a turn-1 gh_log_not_contains 'issue create' and a turn-3 gh_log_contains 'issue create'; selftest-fail wrong-turn case fails as designed"
      - "AC 4: results show each turn's scripted output"
      - "AC 5: results JSON turn_outputs lists 3 outputs"
      - "Full test suite, vet, gofmt and sync:check are green; out-of-scope items (new check types, judge template, simulated users, .kairon/evals cases) are untouched"
    validation_commands:
      - "go build ./..."
      - "task lint"
      - "task fmt:check"
      - "task test"
      - "task eval:selftest"
      - "task eval:selftest:sandbox"
      - "task sync:check"
      - "git diff --stat origin/main -- .kairon/evals cmd/kairon/templates"
```
