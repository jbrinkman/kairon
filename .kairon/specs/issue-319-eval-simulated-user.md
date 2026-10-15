# Design Spec: Evals: answer-bank simulated user for elicitation cases

Closes #319

Source issue: https://github.com/jbrinkman/kairon/issues/319 (depends on #315 judge checks and #317 multi-turn
cases, both merged: `d78d5f3`, `45a8d37`).

## 1. Problem

The planner must draw out every requirement the user holds without filling gaps with assumptions. Scripted `turns`
(#317) cannot measure that: the planner's questions vary from run to run, so fixed replies stop matching them. An
elicitation case needs a **simulated user** that answers whatever the agent asks from a fixed **bank of facts**, and
the eval must then be able to say which facts the agent asked for.

Scope: `internal/eval` (including `internal/eval/testdata/evals/`), `internal/inference` where the routing request has
to cross the backend boundary (only for the stub), and `docs/evaluation.md`. Out of scope: free-form generated user
replies (answers come verbatim from the bank), planner cases under `.kairon/evals/`, any check that reads the final
issue body (P1/P2 build on the names fixed here).

## 2. What exists today (verified in the tree)

- **Multi-turn execution (#317).** `executeCase` (`execute_case.go`) loops over `tc.userTurns()`; every turn runs in the
  same workspace via `invokeAgent` with `callOpts.Turn` (1-based) -> `Request.Turn` (0-based, `requestTurn`). The
  backend owns continuity (`kiro-cli --resume` for `Turn > 0`; stub indexes `Stub.Turns[Turn]`). A multi-turn
  `--sandbox` case keeps one `lazyContainerSession` for all turns. Per turn it records `CaseResult.TurnOutputs`, a
  `.eval/gh.log` snapshot (`turnGHLogs`) and `CallRecord.Turn`.
- **Turn schema.** `validateTurnFields` (`workspace.go`): `turns` and a non-blank `input` are mutually exclusive;
  `turns: []` and blank entries are rejected; for a `turns` case with a stub `len(stub.turns) == len(turns)`; a check's
  `turn` must be `<=` the number of turns (`userTurns()`).
- **Judge calls.** `newCheckJudge` (`judge_check.go`) calls `cfg.backend.Invoke` with `RoleJudge`, `cfg.pins.judgeModel()`,
  `judgeCheckTimeout` and `Request.YesNo`; it appends a `CallRecord` to `cr.Calls` and adds cost to `cr.JudgeCost` only
  on success (a failed/unparseable call is carried by its record only). The stub answers yes/no judge requests from
  `Stub.Judge` (`stubYesNoJudge`), everything else with a fixed score-5 JSON. `parseYesNo` shows the delimited-JSON
  parsing convention (`===JSON_START===` / `===JSON_END===`, last START, ANSI stripped).
- **Stub is index-based and stateless for turns** (`Stub.Turns[req.Turn]`), but `Stub.Judge` has a mutable cursor
  (`judgeNext`, cycling). Routing must not copy that pattern (see 3.5).
- **Checks.** `Check`, `checkSpecs` (required/allowed fields per type; unknown field for a type = load error),
  `validateCheck`, `evaluateOne`, `runCheck`, `checkLabel`, `checkDir` + `resolveInDir` (symlink-refusing, size-capped
  file read) and `readGHLog` (`scoring.go`). Eleven types. `checks_docs_test.go` requires **every type in `checkSpecs`
  to be in `allCheckTypes` and to have a valid YAML example in `docs/evaluation.md`** (so adding a type forces a docs
  edit in the same commit).
- **Case validation.** `loadCases` decodes with `KnownFields(true)`; `validateCaseFields` -> `validateTurnFields` holds
  per-case rules and names the case as `case %q (%s): ...`. `ValidateChecks` has no case context, so case-aware check
  rules (turn range) live in `validateTurnFields`.
- **Judge input.** `judgeCaseInput(tc)` renders `Turn N (user): ...` from `tc.Turns` for a turns case.
- **Self-test pins.** `selftest` has 21 cases (`selftest_checks_test.go`: `5+11+2+3`), `selftest-fail` has 19
  (`10+1+1+1+1+5`); docs say "twenty-one" / "nineteen" (`docs/evaluation.md` Self-Test).
- **Workspace.** `.eval/` is harness-owned, bind-mounted read-write into the container, so the agent can write there
  (the same reason `readGHLog` refuses a non-regular `gh.log`).

## 3. Solution approach (decisions)

### 3.1 Case schema

```yaml
name: csv-export-elicitation
agent: planner
input: "I want a feature that exports reports as CSV"     # the opening user message (required with simulated_user)
simulated_user:
  facts:
    - id: format
      answer: "Comma separated, UTF-8, one header row."
      discoverable: false              # the fixture code does NOT answer this
    - id: schedule
      answer: "Reports are exported weekly, every Monday."
      discoverable: false
    - id: language
      answer: "The service is written in Go."
      discoverable: true               # the fixture code answers this; asking is unnecessary
  default_reply: "I don't know, use your best judgement."
  stop_when: 'ISSUE-DRAFT-READY'       # regex on the agent's output (optional)
  max_turns: 8                         # agent turns in the simulated phase (default 10)
turns:                                 # optional scripted turns, run AFTER the simulated phase
  - "Looks good, create it."
stub:                                  # stub backend only
  routes: [format, none]               # one routing decision per simulated reply
  turns: [ ... ]
checks:
  - {criterion: elicitation, type: fact_asked, fact: format}
  - {criterion: elicitation, type: fact_not_asked, fact: language}
```

New types (`internal/eval/types.go`):

```go
type SimulatedUser struct {
    Facts        []Fact `yaml:"facts" json:"facts"`
    DefaultReply string `yaml:"default_reply" json:"default_reply"`
    StopWhen     string `yaml:"stop_when,omitempty" json:"stop_when,omitempty"`
    MaxTurns     *int   `yaml:"max_turns,omitempty" json:"max_turns,omitempty"` // pointer: explicit 0 is rejected
    stopRE       *regexp.Regexp                                                // compiled at load
}
type Fact struct {
    ID           string `yaml:"id" json:"id"`
    Answer       string `yaml:"answer" json:"answer"`
    Discoverable *bool  `yaml:"discoverable" json:"discoverable"` // pointer: omitting it is a load error
}
// TestCase gains: SimulatedUser *SimulatedUser `yaml:"simulated_user,omitempty" json:"simulated_user,omitempty"`
```

Rules, each a load error naming the case (`case %q (%s): ...`):

- `simulated_user` requires a non-blank `input` (the opening message). With `simulated_user`, `input` and `turns` may
  **both** be present (the "mutually exclusive" rule of #317 applies only to cases without `simulated_user`); `turns`
  entries must still be non-blank and `turns: []` is still rejected.
- `facts`: at least one; `id` non-blank, no whitespace, unique, and not `none` (case-insensitive; `none` is the
  "no match" value of `stub.routes` and the sim log); `answer` non-blank; `discoverable` present (`true` or `false`).
  `discoverable` is recorded in the log for the fact's decisions so later metrics (P1/P2: which asked questions were
  unnecessary) can compute them; nothing in this issue acts on it beyond requiring the author to state it.
- `default_reply` non-blank. `stop_when`, when set, must compile as an RE2 regex. `max_turns`, when set, must be `>= 1`
  (default 10, helper `maxTurns()`).
- `stub.turns` (when a stub is present): `1+len(turns) <= len(stub.turns) <= max_turns+len(turns)`. A simulated phase
  runs between 1 and `max_turns` agent turns, then the scripted turns, so fewer entries could never complete and more
  could never be used. The stub's own "no stub.turns[k]" error covers a run that needs more than declared.
- `stub.routes` (new, `internal/inference.StubScript.Routes []string`): only valid with `simulated_user`; each entry a
  fact `id` or `none`; `len(routes) <= max_turns-1` (a route is consumed only when the simulated user replies, and the
  last allowed simulated turn is never answered).
- Checks `fact_asked` / `fact_not_asked` require `simulated_user` and a `fact` naming a defined fact id.
- A check's `turn` upper bound is `max_turns+len(turns)` for a simulated case (the real number is only known after the
  run); an out-of-range turn at run time already yields a failed check with a clear detail (#317).

### 3.2 Turn model (one loop for classic, scripted and simulated cases)

A classic case is a one-turn case (#317); a simulated case is a case whose *next user message* is decided after each
agent turn. `executeCase` therefore stops iterating a precomputed slice and asks a small **turn plan** for the next
message:

```go
// internal/eval/simulated_user.go
type turnPlan struct { ... }                       // built once by newTurnPlan(tc, router)
func (p *turnPlan) first() string                  // turns[0]/input for classic; input for simulated
func (p *turnPlan) next(k int, out string) (msg string, more bool, err error)
//   k = 1-based number of the agent turn that just finished, out = its output
```

- Classic / scripted: `next` returns `turns[k]` while `k < len(turns)`.
- Simulated: while in the simulated phase, after agent turn `k`:
  1. if `stop_when` matches `out` -> phase ends (no routing, no reply);
  2. else if `k == max_turns` -> phase ends;
  3. else route `out` (3.4), append the decision to the sim log, and return the fact's `answer` (verbatim, no trimming)
     or `default_reply` as the next user message.
  When the phase ends, `next` continues with `turns[0]`, `turns[1]`, ... (the scripted turns), one per agent turn; the
  case ends after the last one. A case with no scripted turns ends when the phase ends.
- Setup context (`tc.Setup`) is prepended to the **first** message only, as in #317. Replies and scripted turns are sent
  verbatim. Every message is passed as `Request.Prompt` alone with `Request.Turn = k-1`: **the backend contract of #317
  is unchanged**, a reply is just the next user turn, so a future direct-API backend needs nothing new for simulated
  cases. Under `--sandbox` the case keeps its one container session whenever it is simulated or has more than one turn.
- Counting: "max_turns agent turns" counts agent turns of the simulated phase only; scripted turns do not count against
  it. Turn numbers in `turn_outputs`, `calls[].turn`, check `turn` and the sim log are the **global** 1-based agent-turn
  numbers (simulated turns first, then scripted).
- `stop_when` is evaluated only on simulated-phase turns, case-sensitively (authors use `(?i)` when they want otherwise),
  on that turn's output.

### 3.3 Results

- `CaseResult.UserTurns []string` (`json:"user_turns,omitempty"`): every user message actually sent, in order (opening,
  each reply, each scripted turn). Set **only** for simulated cases, so classic and #317 `turns` results keep their JSON
  shape. This is how a test (and a reader of `<agent>.json`) sees "reply equals f1's answer, then `default_reply`".
- `CaseResult.TurnOutputs`, `Calls[].turn` and per-turn gh snapshots are recorded for simulated cases exactly as for
  `turns` cases (a simulated case counts as multi-turn; the `multi` flag becomes `tc.Turns != nil || tc.SimulatedUser != nil`).
- Routing calls are recorded in `Calls` as judge records (`role: judge`) with a new `CallRecord.Purpose string
  (json:"purpose,omitempty")` set to `simulated_user` (no rubric criterion applies, so `Criterion` stays empty). Their
  cost goes to `JudgeCost` on success only, like every other judge call. Calls stay in execution order (agent 1, route,
  agent 2, ...).
- Progress output prints `turn k` (no `/n`: the total is not known in advance) for simulated cases.
- Judge input: for a simulated case the "INPUT" shown to the legacy judge and to `judge` checks is rendered from the
  recorded `UserTurns` (`Turn N (user): ...`), so the judge sees the real conversation rather than only the opening
  message. Implemented by scoring with a judge-view copy of the case whose `Turns` is `cr.UserTurns`; the judge prompt
  template and `scoreLLMJudge`'s signature do not change.

### 3.4 Routing (the judge maps a question to at most one fact)

After each non-final simulated agent turn the harness makes one **routing call**:

- `cfg.backend.Invoke` with `Role: RoleJudge`, `Model: cfg.pins.judgeModel()`, `Timeout: judgeCheckTimeout`, `Stub:
  tc.Stub`, and two new request fields `Route bool` and `RouteSeq int` (3.5). The prompt (builder owns the wording)
  lists each fact as `id` + `answer`, then the agent's output in an `===BEGIN AGENT OUTPUT===` fence declared untrusted
  data (capped to the last 8 KiB, because questions come last), and asks which single fact the agent is asking about.
  If several questions are asked, the first one that a fact answers wins. Reply format, parsed like `parseYesNo`:
  `===JSON_START==={"reasoning": "...", "fact": "<id>" | "none"}===JSON_END===`.
- `parseRoute(text, ids)` accepts only an id from the case's facts or `none` (trimmed; ids compared exactly, `none`
  case-insensitively). Anything else (unknown id, missing key, bad JSON, list instead of string) is a parse error.
- **The reply text never comes from the model.** The model returns an id; the harness looks the answer up in the bank.
  So an injected instruction in the agent's output can at most choose which bank fact is returned.
- **Fail closed.** A failed or unparseable routing call (including an unknown id) ends the case like a failed agent turn:
  the remaining turns are not run, `ActualOutput = ""`, `TurnOutputs` keeps the outputs recorded so far, the error is
  printed as `turn k: simulated user: <reason>`, and `ErrorContext.Stderr` carries it. A silent fallback to
  `default_reply` would hide a broken judge as "the agent asked nothing relevant".
- The question excerpt in the log (3.6) is the last 200 runes of the agent output with whitespace runs collapsed.
- `--debug` prints the routing prompt like the other judge prompts (`debugWriter`).
- Routing is always host-side (like every judge call), also under `--sandbox`.

### 3.5 Stub backend: `stub.routes`, no model calls (AC 6)

- `inference.StubScript.Routes []string` (`yaml:"routes,omitempty" json:"routes,omitempty"`).
- `inference.Request` gains `Route bool` ("a `RoleJudge` request that asks which bank fact an agent output asks about")
  and `RouteSeq int` ("0-based number of this routing decision in the case"). The stub answers a `Route` request with
  `Routes[RouteSeq]` rendered as the delimited JSON above (`{"reasoning": "stub route", "fact": "<entry>"}`); a missing
  script, or `RouteSeq >= len(Routes)`, is an error (`case has no stub.routes[N]`) which fails the case closed (3.4).
- **Index-based, not cursor-based.** Unlike `Stub.Judge` (mutable cursor, cycles), the harness passes `RouteSeq` and the
  stub indexes, exactly like `Turn`/`Turns`. A rerun of the same loaded case, `--resume`, or parallel cases cannot
  desynchronise it, and running out of scripted routes is loud instead of silently cycling.
- Real backends ignore `Route`/`RouteSeq` (it is just a judge prompt); a future direct-API backend implements it for
  free by answering the prompt. `Route` is mutually exclusive with `YesNo` (stub returns an error if both are set).
- With the stub backend no process is spawned and no network is used, so the AC 2 case makes no model call; the test
  proves it by putting a recording fake `kiro-cli` first on `PATH` (`installFakeKiroCLI`) and asserting it recorded
  nothing.

### 3.6 `.eval/sim-log.jsonl` and the fact checks

Format: one JSON object per routing decision, in order, newline-terminated:

```json
{"turn":1,"question":"…last 200 chars of the agent output…","fact":"f1","discoverable":false}
{"turn":2,"question":"…","fact":"none"}
```

- `turn`: the 1-based agent turn whose output was routed. `fact`: the fact id or the string `none`. `discoverable`: the
  routed fact's flag (omitted for `none`). No reply text is logged (it is a pure function of `fact`; `user_turns` holds it).
  Go type `SimDecision{Turn int; Question string; Fact string; Discoverable *bool}`.
- **Written by the harness** into `ws.EvalDir` (host side; the file is created empty at the start of the case so "no
  decision yet" is distinguishable from "missing") and appended after each decision.
- **Tamper resistance.** The agent can write `.eval/`. The harness keeps the decisions in memory and, after the last turn
  and after the container session is closed (just before scoring), rewrites the file from memory. Checks therefore read
  what the harness decided, not what an agent left behind.
- Check types `fact_asked` and `fact_not_asked` (`CheckFactAsked`, `CheckFactNotAsked`; field `Fact string yaml:"fact"`,
  `checkFields.fact`; **required**, and allowed only on these two types):
  - `fact_asked` passes when at least one decision has `fact == <id>`; `fact_not_asked` passes when none has.
  - Evaluated from the file via `readSimLog(in.Dir)` (regular, non-symlink file, capped at 10 MiB like the gh log). A
    missing file, a non-regular file, an oversized file or an unparseable line **fails** the check with a detail
    (`sim log missing`, `sim log line 3: invalid JSON`) - never a silent pass, including for `fact_not_asked`.
  - `turn` is **not** allowed on them (not in this issue's scope; the log is per case).
  - Detail strings name the fact and the turns it was asked on (`fact f1 was asked on turn 1`) or the ids that were
    asked (`asked: none` / `asked: f1`) to make failures diagnosable.
  - `checkLabel` shows `fact=<id>` (`#2 fact_not_asked fact=f1`).
  - Case-level rules (3.1) reject a fact check on a case without `simulated_user` and an undefined fact id; hand-built
    checks evaluated outside a simulated case simply fail with `sim log missing`.

### 3.7 Docs

`docs/evaluation.md` documents every field with the example of 3.1 (AC 1). See section 7.

## 4. Relevant files

Modify:
- `internal/inference/inference.go` - `StubScript.Routes`, `Request.Route`, `Request.RouteSeq` (+ contract comments).
- `internal/inference/stub.go` - answer `Route` requests from `Routes[RouteSeq]`.
- `internal/inference/record.go` - `CallRecord.Purpose`.
- `internal/eval/types.go` - `SimulatedUser`, `Fact`, `TestCase.SimulatedUser`, `CaseResult.UserTurns` and the unexported
  in-memory decisions, `userTurns()` for a simulated case (`[Input] + Turns`, used by perf only), `maxTurns()`.
- `internal/eval/checks.go` - `CheckFactAsked`/`CheckFactNotAsked`, `Check.Fact`, `checkFields.fact`, `checkSpecs`,
  `present()`, validation.
- `internal/eval/checks_eval.go` - dispatch in `runCheck`, `checkLabel`.
- `internal/eval/workspace.go` - `validateTurnFields` / new `validateSimulatedUser` (3.1).
- `internal/eval/execute_case.go` - turn plan loop, session condition, sim-log lifecycle, failure semantics, progress.
- `internal/eval/scoring.go` - judge-view copy of the case for simulated cases.
- `internal/eval/perf.go` - nothing expected (uses `userTurns()[0]`); verify it still sends the opening message.
- `internal/eval/testdata/evals/cases/selftest/` and `.../selftest-fail/` - new cases (section 6).
- Pinned counts: `internal/eval/selftest_checks_test.go` (`5+11+2+3` -> +2; `10+1+1+1+1+5` -> +1),
  `internal/eval/sandbox_workspace_test.go` and `internal/eval/selftest_test.go` if they pin the case lists,
  `internal/eval/checks_docs_test.go` (`allCheckTypes`).
- `docs/evaluation.md`.

Create: `internal/eval/simulated_user.go` (turn plan, `buildRoutePrompt`, `parseRoute`, `SimDecision`, sim-log write/read,
`routeFunc`), `internal/eval/checks_fact.go` (`evalFact`, `readSimLog`), and tests (names are suggestions):
`internal/inference/stub_route_test.go`, `internal/eval/checks_fact_test.go`, `internal/eval/simulated_user_schema_test.go`,
`internal/eval/simulated_user_test.go`, `internal/eval/selftest_simulated_test.go`.

Not modified: `.kairon/evals/**` (out of scope), `cmd/kairon/templates/**` (so `task sync:check` is unaffected and is
still run), the judge prompt template, `.kairon/specs/` other than this file.

## 5. TDD plan (AGENTS.md: failing test first, for the right reason, committed together with the implementation)

Name the first-failing test of each task in the commit message.

1. **inference-route-contract** - tests: `TestStubRoute_ReturnsRouteBySeq` (RouteSeq 0 -> `f1`, 1 -> `none`, delimited JSON
   with a `fact` key), `TestStubRoute_ExhaustedIsError` (seq beyond the list names `stub.routes[N]`),
   `TestStubRoute_NoScript`, `TestStubRoute_WithYesNoIsError`, `TestStubRoute_Stateless` (calling seq 1 first, then 0,
   gives the same answers: no cursor), `TestCallRecordPurposeOmittedWhenEmpty`, YAML decode of `stub.routes`. Expected red:
   the stub still answers a `Route` judge request with the legacy score-5 JSON (assertion failure, not a compile
   error - write the tests against `Request{Role: RoleJudge, ...}` first and add the fields as the minimal change).
2. **fact-checks** - tests in `checks_fact_test.go` using a temp workspace: `fact_asked` passes/fails from a log with
   two decisions; `fact_not_asked` the inverse; missing file, symlinked file, directory, oversized file, malformed line ->
   failed result with a detail (also for `fact_not_asked`); `fact` required, `fact` not valid for `output_contains`,
   `turn` not valid on a fact check, blank `fact`; label contains `fact=f1`; docs-coverage test red until the docs
   example exists (the task adds the minimal table rows and one YAML example per new type so the existing docs tests stay
   green). Expected red: `unknown type "fact_asked"`.
3. **simulated-user-schema** - tests in `simulated_user_schema_test.go` using `t.TempDir()` evals dirs (pattern of
   `turns_schema_test.go`): every rule of 3.1 as a load error that names the case (missing `discoverable`, duplicate id,
   id `none`, blank id/answer/default_reply, bad `stop_when`, `max_turns: 0`/`-1`, missing `input`, `turns: []`,
   fact check without `simulated_user`, unknown fact in a check, `stub.routes` without `simulated_user`, route naming an
   unknown fact, too many routes, `stub.turns` below `1+len(turns)` and above `max_turns+len(turns)`); positive cases:
   `input` + `turns` + `simulated_user` loads, `max_turns` defaults to 10, `stop_when` compiled, a check `turn` up to
   `max_turns+len(turns)` loads; a case with `turns` and `input` but **no** `simulated_user` still fails with
   `turns and input are mutually exclusive`; classic and #317 cases unchanged. Expected red: `field simulated_user not
   found` (KnownFields decode error).
4. **simulated-user-execution** - tests in `simulated_user_test.go` with the stub backend and the recording-backend seam used by
   `multi_turn_test.go`/`backend_routing_test.go`: replies per turn equal f1's `answer` byte for byte then
   `default_reply` (assert the recorded `Request.Prompt` of turns 2 and 3 and `UserTurns`; setup text only in turn 1);
   `Request.Turn` 0,1,2; a `stop_when` match on turn 2 ends the phase and the scripted turns run as turns 3.. (turn count,
   `UserTurns`, `TurnOutputs`); `max_turns` ends the phase without a reply to the last turn; default `max_turns` 10
   (a never-stopping stub of 10 outputs ends after 10); stop on turn 1 makes no routing call; the sim log has one line per
   decision with the right `turn`/`fact`/`discoverable`/`question` excerpt (last 200 runes, whitespace collapsed); log
   rewritten from memory after the run (an agent turn that truncates `.eval/sim-log.jsonl` via a stub command does not
   change what `fact_asked` sees); routing failure (exhausted `stub.routes`, unknown id from a recording judge, judge
   error) fails the case closed with `turn k: simulated user:` and no `default_reply` fallback; judge `Calls` record has
   `purpose: simulated_user` and cost lands in `JudgeCost` only on success; `parseRoute` table test (valid id, `none`,
   `None`, unknown id, missing key, non-string, no delimiters, ANSI); `buildRoutePrompt` contains every fact id/answer and
   fences the output; judge-view input for a simulated case lists the real replies; **AC 6**:
   `TestSimulatedUserMakesNoModelCalls` (fake `kiro-cli` first on PATH records nothing for the stub run); container
   session test (fake `openContainerSession`): one open, N `Run`s with `Turn` 0..N-1, one close, for a simulated case.
   Expected red: simulated case runs only one agent turn / `TurnOutputs` has one entry.
5. **selftest-fixtures** - new fixtures and tests in `selftest_simulated_test.go` (section 6) plus updated pinned counts.
   Expected red: the count assertions (`want 23, got 21`) and the missing-case assertions.
6. **docs** - no test first (docs exemption), but `checks_docs_test.go` and a new docs test (`simulated_user` example
   parses with `loadCases`-equivalent strict decoding) keep it honest.

## 6. Self-test cases (AC 2-6)

All use the deterministic stub backend; rubric criterion `structural_completeness` (checks make it deterministic). Agent
turns end with a question so the cases read like elicitation.

`internal/eval/testdata/evals/cases/selftest/`:
- `sim-routes.yaml` (**AC 2, 4, 5, 6**): `input`; `simulated_user` with facts `f1` (`discoverable: false`, a distinctive
  answer) and `f2` (`discoverable: true`), `default_reply`, `max_turns: 3`, no `stop_when`; `stub.routes: [f1, none]`;
  3 `stub.turns` (each with a unique marker and a question). Checks: `fact_asked: f1`, `fact_not_asked: f2`,
  `output_contains` with `turn: 1/2/3`. Tests assert: `user_turns == [input, <f1 answer>, <default_reply>]`; with
  `--keep-workspaces` the kept workspace holds `.eval/sim-log.jsonl` with exactly two lines (turn 1 -> `f1`, turn 2 -> `none`);
  the case scores full marks; **no `kiro-cli` call is recorded** by the fake on PATH.
- `sim-stop-when.yaml` (**AC 3**): `stop_when: 'PLAN-READY'`, `max_turns: 5`, `stub.routes: [f1]`, scripted `turns: ["Please
  summarise.", "Thanks."]`, `stub.turns` of 4 entries where the **second** matches `PLAN-READY`. Tests assert 2 simulated
  turns then the 2 scripted ones: `len(turn_outputs) == 4`, `user_turns == [input, <f1 answer>, "Please summarise.", "Thanks."]`,
  exactly one sim-log line, and `calls[].turn` 1..4.

`internal/eval/testdata/evals/cases/selftest-fail/`:
- `sim-fact-not-asked.yaml` (**AC 5, failing side**): same shape as `sim-routes` but with `fact_not_asked: f1` while the
  stub routes `f1` -> the check fails; test asserts the case fails and its reasoning names `fact_not_asked fact=f1`.

AC 1's invalid cases cannot live in `cases/selftest/` (a load error is fatal for the directory); they are YAML strings in
`t.TempDir()` evals dirs in `simulated_user_schema_test.go`.

Counts to update: `selftest` 21 -> 23, `selftest-fail` 19 -> 20 (code and docs). `TestSelftestSandbox` (daemon-gated) must
pass for the new cases natively and under `--sandbox` (it exercises the container session with the host-side routing).

## 7. Documentation (`docs/evaluation.md`)

- **Test Case Format**: add `simulated_user` and the `input`+`turns` relationship to the field comment block.
- **New section "Simulated user (elicitation cases)"** after "Multi-turn cases": why (questions vary between runs), the
  full field table (`facts[].id/answer/discoverable`, `default_reply`, `stop_when`, `max_turns` default 10) with the
  example of 3.1 (**AC 1**), the loop (stop_when / max_turns, scripted `turns` after, global turn numbering), routing
  (judge maps the question to at most one fact; answers verbatim; `default_reply` when none; the reply is never
  generated), `discoverable` semantics, the injection property, fail-closed routing, host-side routing and its judge cost
  (`calls[]` `purpose: simulated_user`, `judge_cost`), `user_turns` in the result JSON, the sim-log format and tamper
  rule, `stub.routes` (index-based, count rule, no model calls), the load-error list, and the "reply is just the next user
  turn" note for future backends (`Request.Route`/`RouteSeq` only matter to the stub).
- **Checks**: table rows for `fact_asked` / `fact_not_asked` with the `fact` field; "eleven" -> "thirteen" in the three
  places; a short subsection with a YAML example; `fact` not valid on other types and `turn` not valid on fact checks.
- **Stub Case Fields**: `routes`; the per-turn count rule for simulated cases.
- **Result JSON**: `user_turns`, `calls[].purpose`.
- **Adding a New Backend**: `Request.Route`/`RouteSeq` (stub-only contract, real backends just answer the prompt).
- **Self-Test**: the three new cases and the counts (`selftest` twenty-three, `selftest-fail` twenty).
- Everything not named here (the `.kairon/specs/` tree, `CHANGELOG.md`) stays untouched.

## 8. AC traceability

| AC | Delivered by | Verified by |
|----|--------------|-------------|
| 1 `simulated_user` fields (`facts[id,answer,discoverable]`, `default_reply`, `stop_when`, `max_turns` default 10), scripted `turns` after | 3.1, 3.2 | schema tests; `docs/evaluation.md` section with example + docs test |
| 2 judge maps question to at most one fact; verbatim answer or `default_reply` | 3.2, 3.4 | `sim-routes` case + `user_turns`/recorded prompts; `parseRoute` tests |
| 3 stop on `stop_when` or `max_turns`; scripted turns continue | 3.2 | `sim-stop-when` case (2 simulated turns then scripted) and `max_turns` tests |
| 4 every decision logged to `.eval/sim-log.jsonl` | 3.6 | kept workspace of `sim-routes` contains both decisions; tamper test |
| 5 `fact_asked` / `fact_not_asked` from the log | 3.6 | `selftest` `fact_asked: f1` passes; `selftest-fail` `fact_not_asked: f1` fails; check unit tests |
| 6 stub routing from `stub.routes`, no model calls | 3.5 | `TestSimulatedUserMakesNoModelCalls` (fake `kiro-cli` records nothing) |

## 9. Team orchestration

```
inference-route-contract ─┐
fact-checks ──────────────┼─► simulated-user-schema ─► simulated-user-execution ─► selftest-fixtures ─► docs ─► validate-complete
                          └──────────────────────────────▲
```

- `inference-route-contract` (`internal/inference`) and `fact-checks` (`checks*.go`, small docs addition) touch disjoint
  files and can run in parallel.
- `simulated-user-schema` needs `StubScript.Routes` (route validation) and `Check.Fact` (case-level fact-check rules).
- `simulated-user-execution` needs the types, validation, stub routing and the log reader.
- Fixtures need the executor; docs describe final behaviour; the validator runs last.
- One PR, all six acceptance criteria, no phases. Put `Closes #319` in the PR description. Commit each task's tests with
  its implementation (never a standalone red commit) and name the first failing test in the message. Run `task
  sync:check` at the end even though nothing synchronised is edited.

## 10. Risks and notes

- **Routing quality is model-dependent.** Real runs are nondeterministic by nature; that is why `fact_asked` is a
  per-fact boolean over a log and why the log keeps the question excerpt (so a human can audit a misroute). The stub makes
  the harness itself deterministic.
- **Dynamic turn count.** Check `turn` for a simulated case can only be range-checked statically against
  `max_turns+len(turns)`; the run-time out-of-range failure from #317 covers the rest.
- **Judge cost.** Every simulated reply costs one judge call; `max_turns` bounds it. `judge_cost` and `calls[]` show it.
- **`--sandbox`.** Routing runs host-side; agent turns share one container (the #317 session), so nothing structural
  changes. `TestSelftestSandbox` covers it when a daemon is available and skips cleanly otherwise.
- **Unused `discoverable`.** It is validated and logged but nothing scores it here; P1/P2 consume it. A reviewer may ask
  for a `fact_unnecessary` check - out of scope, the log has what it needs.
- **Real `kiro-cli` is not exercised in CI.** The routing prompt/parse path is covered by unit tests with a recording
  backend; a manual run of a real elicitation case is left to P1/P2.

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
  - id: "inference-route-contract"
    agent: "builder"
    description: "Add the routing request to the inference layer (TDD: write the failing stub tests first). In internal/inference add StubScript.Routes ([]string, yaml/json 'routes'), Request.Route (bool) and Request.RouteSeq (int, 0-based number of the routing decision in the case) with contract comments, and make the stub backend answer a RoleJudge request with Route set from Routes[RouteSeq] as delimited JSON ({\"reasoning\": \"stub route\", \"fact\": \"<entry>\"} between ===JSON_START=== and ===JSON_END===). Index-based like Turn/Turns - no cursor. A missing script, RouteSeq beyond the list, or Route together with YesNo is an error. Add CallRecord.Purpose (json 'purpose', omitempty)."
    dependencies: []
    acceptance_criteria:
      - "A stub test asserting Route requests with RouteSeq 0 and 1 return facts f1 and none was seen failing (stub still returned the legacy score-5 JSON) before implementation"
      - "RouteSeq beyond len(Routes) returns an error naming stub.routes[N]; a request with no Stub script returns an error; Route together with YesNo returns an error"
      - "Answers are stateless: asking RouteSeq 1 before RouteSeq 0 yields the same answers as in order"
      - "Non-route judge requests (legacy and YesNo) and agent requests behave exactly as before; existing inference tests pass unchanged"
      - "CallRecord marshals 'purpose' only when non-empty"
      - "StubScript decodes 'routes' from YAML"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/inference/... -count=1"
      - "go vet ./internal/inference/..."

  - id: "fact-checks"
    agent: "builder"
    description: "Add the fact_asked and fact_not_asked check types (TDD). Add CheckFactAsked/CheckFactNotAsked, Check.Fact (yaml 'fact'), checkFields.fact (required and allowed only on these two types; 'turn' is not allowed on them), SimDecision {Turn int, Question string, Fact string, Discoverable *bool} (json turn/question/fact/discoverable), readSimLog(dir) for <dir>/.eval/sim-log.jsonl (regular non-symlink file only, 10 MiB cap, JSONL), evalFact in runCheck, and ' fact=<id>' in checkLabel. fact_asked passes when any decision has fact == id; fact_not_asked passes when none has. A missing, non-regular, symlinked, oversized or malformed log FAILS the check with a detail (including fact_not_asked), never a silent pass. Update allCheckTypes in checks_docs_test.go and add to docs/evaluation.md the two table rows and one valid YAML example per new type so TestChecksDocsCoversEveryCheckType and TestChecksDocsExamplesAreValid stay green (full docs follow in the docs task)."
    dependencies: []
    acceptance_criteria:
      - "Tests written first fail for the behavioural reason (unknown type fact_asked) before implementation"
      - "fact_asked f1 passes and fact_not_asked f2 passes for a log with decisions f1 and none; fact_asked f2 and fact_not_asked f1 fail with a detail naming the fact and the turns/ids asked"
      - "Missing log file, a directory, a symlink, an oversized log and a malformed line each give a failed check with a clear detail for both types"
      - "fact is required on both types, rejected on every other type ('fact is not valid for type output_contains'), blank fact rejected, turn rejected on both types"
      - "checkLabel for a fact check contains fact=<id>; existing labels unchanged"
      - "docs/evaluation.md has table rows and a valid YAML example for both types; checks_docs tests pass"
      - "All pre-existing checks tests pass unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Fact|Check|Docs' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "simulated-user-schema"
    agent: "builder"
    description: "Add the simulated_user case schema and its validation (TDD, tests use t.TempDir() evals dirs like turns_schema_test.go). Add types SimulatedUser {Facts []Fact, DefaultReply string, StopWhen string, MaxTurns *int, compiled stopRE} and Fact {ID, Answer, Discoverable *bool}, TestCase.SimulatedUser (yaml 'simulated_user'), maxTurns() (default 10), and make userTurns() return [Input]+Turns for a simulated case (used by perf only). Extend validateTurnFields with validateSimulatedUser: input required and may coexist with turns when simulated_user is present (the turns/input mutual exclusion stays for cases without it); facts >=1 with unique, non-blank, whitespace-free ids that are not 'none', non-blank answers and an explicit discoverable; non-blank default_reply; stop_when compiles; max_turns >=1 when set; stub.turns count within 1+len(turns) .. max_turns+len(turns); stub.routes only with simulated_user, each entry a fact id or 'none', len <= max_turns-1; fact_asked/fact_not_asked require simulated_user and a defined fact id; check turn upper bound is max_turns+len(turns) for simulated cases. Every error names the case via case %q (%s)."
    dependencies: ["inference-route-contract", "fact-checks"]
    acceptance_criteria:
      - "Tests written first fail for the expected reason (simulated_user rejected as an unknown field) before implementation"
      - "Each rule above has a test that asserts a load error containing the case name and a rule-specific phrase"
      - "A valid case with input + simulated_user + scripted turns loads; max_turns defaults to 10; stop_when is compiled; a check turn up to max_turns+len(turns) loads"
      - "A case with turns and input but no simulated_user still fails with 'turns and input are mutually exclusive'; classic and #317 turns cases load exactly as before"
      - "task-level: existing turns_schema_test.go and all other eval tests pass unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Simulated|Turns|Schema|LoadCases' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "simulated-user-execution"
    agent: "builder"
    description: "Implement the simulated phase (TDD) in internal/eval/simulated_user.go and execute_case.go. Replace the precomputed-slice loop with a turnPlan (first(), next(k, output)) so classic, scripted and simulated cases share one loop; classic/turns behaviour and output are unchanged. Simulated: after agent turn k, stop_when match or k==max_turns ends the phase (no routing, no reply); otherwise route the output with a RoleJudge/Route request (judge model pin, judgeCheckTimeout, RouteSeq, tc.Stub; prompt lists facts id+answer and fences the last 8 KiB of the agent output as untrusted; reply JSON {reasoning, fact}), parse with parseRoute (only a known id or 'none'), append a SimDecision to memory and to .eval/sim-log.jsonl (created empty at case start; question = last 200 runes, whitespace collapsed), and send the fact's answer verbatim or default_reply as the next user message; then the scripted turns continue. Record CaseResult.UserTurns (simulated cases only), routing CallRecords (role judge, Purpose 'simulated_user', JudgeCost only on success), global 1-based turn numbers, progress 'turn k'. Routing failure fails the case closed (no default_reply fallback, ActualOutput empty, 'turn k: simulated user: ...'). Rewrite the sim log from memory after the last turn and after the container session is closed, before scoring. Use the container session whenever the case is simulated or has >1 turn. Score simulated cases with a judge-view case whose Turns is UserTurns."
    dependencies: ["simulated-user-schema"]
    acceptance_criteria:
      - "A test with replies is red first: before implementation a simulated case runs a single agent turn"
      - "With stub.routes [f1, none] and max_turns 3, the recorded Request.Prompt of turn 2 equals f1's answer byte for byte and turn 3's equals default_reply; Request.Turn is 0,1,2; setup text is only in turn 1's prompt; UserTurns lists input, answer, default_reply (AC 2)"
      - "A stop_when match on the second agent turn ends the simulated phase after 2 turns and the scripted turns run as turns 3 and 4; max_turns ends the phase without answering the last turn; max_turns defaults to 10; a stop on turn 1 makes no routing call (AC 3)"
      - "The sim log has one JSON line per decision with turn, question excerpt, fact (id or none) and discoverable; an agent stub command that truncates .eval/sim-log.jsonl does not change what fact_asked sees (AC 4)"
      - "fact_asked/fact_not_asked evaluate correctly end to end through scoreCase on the log (AC 5)"
      - "An exhausted stub.routes, an unknown fact id from a recording judge and a judge error each fail the case closed with a 'turn k: simulated user:' error and never send default_reply"
      - "With a recording fake kiro-cli first on PATH and the stub backend, running the AC 2 case records no kiro-cli call (AC 6)"
      - "A fake container session shows one open, N Run calls with Turn 0..N-1 and one close for a simulated case; one-turn classic cases still use the one-shot path"
      - "Routing CallRecords carry purpose 'simulated_user' and their cost is in JudgeCost only on success; results of classic and #317 turns cases are unchanged (no user_turns, no purpose); existing parity and legacy-score tests pass"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Simulated|MultiTurn|ExecuteCase|Container|Session' -count=1"
      - "go test ./internal/eval/... ./internal/inference/... -count=1"
      - "go test -race ./internal/eval/ -run 'Simulated' -count=1"

  - id: "selftest-fixtures"
    agent: "builder"
    description: "Add the self-test fixtures and update pinned counts. selftest/sim-routes.yaml (facts f1 discoverable:false and f2 discoverable:true, default_reply, max_turns 3, stub.routes [f1, none], 3 stub.turns with markers and questions; checks fact_asked f1, fact_not_asked f2, output_contains per turn). selftest/sim-stop-when.yaml (stop_when matched by the second stub response, max_turns 5, stub.routes [f1], scripted turns ['Please summarise.', 'Thanks.'], 4 stub.turns). selftest-fail/sim-fact-not-asked.yaml (fact_not_asked f1 while the stub routes f1; expected to fail). Add selftest_simulated_test.go asserting AC 2-6 on these cases (user_turns, kept-workspace sim-log with both decisions, turn_outputs lengths, no kiro-cli calls, fail reasoning naming fact_not_asked fact=f1). Update selftest 21->23 and selftest-fail 19->20 in selftest_checks_test.go, sandbox_workspace_test.go/selftest_test.go and any other pinned lists. Do not touch .kairon/evals or cmd/kairon/templates."
    dependencies: ["simulated-user-execution"]
    acceptance_criteria:
      - "task eval:selftest passes all 23 selftest cases with every score at maximum and no ErrorContext"
      - "selftest-fail records sim-fact-not-asked as a failed case whose reasoning names the fact_not_asked fact=f1 check; the other 19 selftest-fail cases are unchanged"
      - "The kept workspace of sim-routes contains .eval/sim-log.jsonl with two lines: turn 1 fact f1 and turn 2 fact none (AC 4)"
      - "sim-routes results show user_turns [input, f1 answer, default_reply] (AC 2) and sim-stop-when shows 4 turn_outputs after 2 simulated turns (AC 3)"
      - "The fake-kiro-cli-on-PATH test records nothing for sim-routes (AC 6)"
      - "TestSelftestSandbox (daemon-gated) is updated for the new counts and compares native and sandbox results for the simulated cases; it skips cleanly without a daemon"
      - "No file under .kairon/evals or cmd/kairon/templates changes (task sync:check passes)"
    validation_commands:
      - "go test ./internal/eval/ -run 'SelfTest|Selftest|Simulated' -count=1"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail"
      - "task eval:selftest:sandbox"
      - "task sync:check"

  - id: "docs"
    agent: "builder"
    description: "Update docs/evaluation.md (live doc; no test required first): add simulated_user to the Test Case Format comment block; add a 'Simulated user (elicitation cases)' section with the full field table (facts[].id/answer/discoverable, default_reply, stop_when, max_turns default 10) and the example from the spec (AC 1), the loop semantics (stop_when / max_turns, scripted turns after, global turn numbering), routing (at most one fact, verbatim answers, default_reply when none, replies never generated, injection property, fail-closed, host-side, judge cost and calls[] purpose), discoverable semantics, user_turns in the result JSON, the sim-log format and tamper rule, stub.routes (index-based, count rule, no model calls), the load-error list and the future-backend note; update the Checks table/prose for fact_asked/fact_not_asked ('eleven' -> 'thirteen' everywhere) with an example; Stub Case Fields (routes, count rule); Result JSON; Adding a New Backend (Route/RouteSeq); Self-Test listing and counts (selftest twenty-three, selftest-fail twenty). Add a docs test that the simulated_user example strictly decodes and validates."
    dependencies: ["selftest-fixtures"]
    acceptance_criteria:
      - "Every behaviour in sections 3.1-3.6 of the spec appears in docs/evaluation.md and matches the implementation"
      - "The documented simulated_user example loads through the strict case decoder and validation (docs test) and lists every field including discoverable, default_reply, stop_when and max_turns"
      - "No stale statements remain ('eleven check types', 'twenty-one cases', 'nineteen cases', 'mutually exclusive' without the simulated_user exception)"
      - "No edits to .kairon/specs (other than this spec), CHANGELOG.md or other historical artifacts"
    validation_commands:
      - "grep -n 'simulated_user' docs/evaluation.md"
      - "grep -n 'sim-log.jsonl' docs/evaluation.md"
      - "! grep -n 'eleven' docs/evaluation.md"
      - "go test ./internal/eval/ -run 'Docs' -count=1"

  - id: "validate-complete"
    agent: "validator"
    description: "Read-only verification that issue #319 acceptance criteria 1-6 are met by automated tests and self-test cases, that classic and #317 multi-turn cases are unchanged, that routing fails closed, that the sim log is rewritten from memory before scoring, and that AGENTS.md rules hold (tests committed with implementation, no historical artifacts edited, template sync unaffected)."
    dependencies: ["docs"]
    acceptance_criteria:
      - "AC 1: docs/evaluation.md documents facts, discoverable, default_reply, stop_when and max_turns (default 10) with an example, and scripted turns may follow; schema tests cover each load error"
      - "AC 2: sim-routes records user_turns equal to f1's answer then default_reply, answers verbatim"
      - "AC 3: sim-stop-when runs 2 simulated turns then its scripted turns; max_turns ends the phase"
      - "AC 4: the kept workspace of sim-routes holds .eval/sim-log.jsonl with both decisions (turn, question excerpt, fact id or none)"
      - "AC 5: selftest passes fact_asked f1; selftest-fail fails fact_not_asked f1"
      - "AC 6: the stub run makes no model calls (fake kiro-cli on PATH records nothing)"
      - "Full test suite, vet, gofmt, lint and sync:check are green; .kairon/evals and cmd/kairon/templates are untouched; the judge prompt template, #317 turn semantics and backend turn contract are unchanged"
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
