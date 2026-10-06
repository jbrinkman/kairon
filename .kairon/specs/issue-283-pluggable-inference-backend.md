# Design Spec: Pluggable inference backend with a deterministic stub self-test

Closes #283

## 1. Solution Approach

Introduce a new shared package, `internal/inference`, that owns every "send a prompt to a model, get text back" call. `internal/eval` stops starting `kiro-cli` itself (outside the Docker sandbox path) and instead calls an `inference.Backend`. Two backends ship:

- `kiro-cli` (default): a byte-for-byte move of today's behaviour (`invokeAgentNative`, `runKiroCLI`, `MeasureStartupOverhead`, `exec.LookPath`).
- `stub`: deterministic, no process, no network. The agent's output comes from `stub.turns[0].response` on the case; every judge call returns a passing judgment with the maximum score.

Second, make the eval directory configurable (`--evals-dir`) so the harness can run against a self-test fixture set in `internal/eval/testdata/evals/` with results written next to it (and git-ignored).

### Key decisions

1. **Package name/location: `internal/inference`.** It must not import `internal/eval` (stdlib only), so `internal/agent` and a later direct-API harness can import it. `internal/agent/manager.go` is NOT migrated in this PR (out of scope); we only guarantee importability.
2. **Backend interface** (sketch; the builder may adjust names but must keep the semantics):

   ```go
   package inference

   type Role string
   const (RoleAgent Role = "agent"; RoleJudge Role = "judge")

   type UsageSource string
   const (UsageReported UsageSource = "reported"; UsageEstimated UsageSource = "estimated")

   type Usage struct {
       InputTokens, OutputTokens int
       Source UsageSource
   }

   // StubScript is test-double data carried on a Request; real backends ignore it.
   type StubScript struct { Turns []StubTurn `yaml:"turns" json:"turns"` }
   type StubTurn struct {
       Response string     `yaml:"response" json:"response"`
       Model    string     `yaml:"model,omitempty" json:"model,omitempty"`
       Usage    *StubUsage `yaml:"usage,omitempty" json:"usage,omitempty"`
   }
   type StubUsage struct {
       InputTokens  int `yaml:"input_tokens" json:"input_tokens"`
       OutputTokens int `yaml:"output_tokens" json:"output_tokens"`
   }

   type Request struct {
       Role           Role
       Agent          string        // agent name (RoleAgent only)
       Prompt         string
       Timeout        time.Duration
       AgentConfigDir string        // optional dir holding <agent>.json (+ prompt files); takes precedence over .kiro/agents
       Stub           *StubScript   // used only by the stub backend
       Turn           int           // stub turn index; 0 for now
   }

   type Response struct {
       Text     string
       Model    string        // "" when unknown
       Usage    Usage
       Command  string        // human-readable command line (for ErrorContext)
       Stderr   string
       ExitCode int
       Duration time.Duration
   }

   type Backend interface {
       Name() string
       Available() error                 // replaces exec.LookPath("kiro-cli"); stub: nil
       StartupProbe() time.Duration      // replaces MeasureStartupOverhead body; stub: 0, no process
       Invoke(ctx context.Context, req Request) (Response, error)
   }

   var ErrTimeout = errors.New("inference timeout")
   func New(name string) (Backend, error)   // unknown -> error "unknown backend %q; valid backends: kiro-cli, stub"
   func Names() []string                    // ["kiro-cli", "stub"] (sorted)
   func EstimateUsage(input, output string) Usage // len/4, Source=estimated (moved from eval.estimateCost)
   ```

   `Invoke` returns a populated `Response` (Command/Stderr/ExitCode) even when it also returns an error, so eval can build its `ErrorContext`. Timeouts return an error for which `errors.Is(err, ErrTimeout)` is true.
3. **kiro-cli backend preserves current behaviour exactly:**
   - `RoleAgent`: `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools`, prompt on stdin, stdout/stderr captured separately, ANSI stripped from returned `Text`, usage = `EstimateUsage(prompt, text)` (estimated). Error strings stay verbatim: `kiro-cli timeout after %v` and `kiro-cli invocation failed: %w`.
   - `RoleJudge`: `kiro-cli chat --no-interactive`, prompt on stdin, `cmd.Output()` semantics, raw (un-stripped) stdout as `Text`, usage estimated from prompt and raw text; failures keep the `kiro-cli chat failed: %v` wording.
   - `StartupProbe`: times `kiro-cli --version` (existing code). `Available`: `exec.LookPath("kiro-cli")`.
   - `Model` is empty (kiro-cli exposes none). Usage is always `estimated`.
   - `AgentConfigDir` handling: if set and `<dir>/<agent>.json` exists, stage a temp overlay `T/.kiro/agents/` (copy of the whole dir so `file://./x.md` prompts resolve) and run with `cmd.Dir = T`, cleaned up afterwards. kiro-cli only discovers local agents under `<cwd>/.kiro/agents/`, so this is how `<evals-dir>/agents/` gets precedence over the repo's `.kiro/agents/`. When unset, no `cmd.Dir` is set (behaviour identical to today). Document the cwd caveat in docs. (Builder: verify discovery with `kiro-cli agent list` from a temp overlay; if a cleaner mechanism exists in the installed CLI, prefer it, but never mutate the repo's `.kiro/agents/`.)
4. **stub backend:**
   - `RoleAgent`: returns `req.Stub.Turns[req.Turn].Response`. Missing stub / empty turns / out-of-range turn ⇒ clear error (`case has no stub.turns[0].response`), not a silent empty string.
   - Usage: if the turn has `usage`, return it with `Source=reported` and `Model` from the turn; otherwise `EstimateUsage(prompt, response)` (estimated).
   - `RoleJudge`: returns `===JSON_START===\n{"score": 5, "reasoning": "stub judge: always passes", "pass": true}\n===JSON_END===` (5 is the max of the judge scale in `scoreLLMJudge`), estimated usage, `Model: "stub"`.
   - `Available()` nil; `StartupProbe()` 0; never spawns a process or touches the network.
5. **Eval dir as package-level configuration.** `internal/eval` already uses package globals (`globalProfiler`), and `loadCases`/`loadRubrics`/`GetTestCase` are called from many places and tests with their current signatures. To avoid a signature cascade, add an unexported `runConfig` (package var `cfg`) holding `evalsDir string` (default `.kairon/evals`) and `backend inference.Backend` (default kiro-cli). `configure(opts RunOptions) error` sets it and is called first thing in `RunWithOptions` (before cleanup/list/perf/single/resume/run, so unknown backends are rejected before any work). Helpers `evalsPath(parts ...string)` build paths. Tests reset it via a `resetConfig()` helper.
6. **Setup-file path rebasing.** Existing cases reference fixtures as repo-relative `.kairon/evals/fixtures/...` (`setup[].path`). In `assemblePrompt`, a path with prefix `.kairon/evals/` is rebased onto `cfg.evalsDir` when a non-default evals-dir is in use; other paths are untouched. With no `--evals-dir` nothing changes.
7. **Cost/usage reporting.** Extend `eval.CostInfo` with `Model string json:"model,omitempty"` and `UsageSource string json:"usage_source,omitempty"` (`reported`/`estimated`). `estimateCost` becomes a thin converter `costFromUsage(model, usage)` using the same $3/$15 per M-token formula on the token counts, so numbers for kiro-cli are unchanged. Add `CostInfo.Add(other)` to replace the two duplicated inline judge-cost accumulations (in `evaluate` and `evaluateProgressive`); merged `UsageSource` is `reported` only if every contributor was `reported`, else `estimated`. New fields are additive JSON and `diff.go` ignores unknown fields.
8. **Errors / guards:**
   - `--backend nope` ⇒ non-zero exit; message lists valid backends (`unknown backend "nope"; valid backends: kiro-cli, stub`).
   - `--backend stub` together with `--sandbox` ⇒ error (the container path spawns kiro-cli and is explicitly out of scope). `invokeAgentInContainer` stays as is.
9. **`--evals-dir` surface.** Add as a persistent flag on `evalCmd` so `eval diff` also honours it. Thread it into: `loadRubrics`, `loadCases`, rubrics-dir existence check and "no rubrics found" message in `Run`, all four `resultsDir` constructions (`runSingleTestCase`, `Run`, `RunPerformanceInvestigation`, and `runWithResume`'s base dir), `Diff`/`diff.go` results dir. Leave `.kairon/evals/tmp/...` (sandbox debug/Dockerfile artefacts) alone.
10. **`perf.go`:** `MeasureStartupOverhead()` keeps its exported name and caching but calls `cfg.backend.StartupProbe()` instead of `exec.Command("kiro-cli", "--version")`. The "📊 Measuring kiro-cli startup overhead..." line uses the backend name. `perf.go` must contain no `exec.Command("kiro-cli"...)`.
11. **Agent under test wiring.** `invokeAgent` keeps its signature; with `cConfig == nil` it calls a new `invokeAgentViaBackend(agent, prompt, tc)` (needs the case's `Stub`, so pass `*TestCase`/stub script through; `invokeAgent` callers in `evaluate`, `evaluateProgressive` and `perf.go` are updated). Agent timeout keeps the `KAIRON_EVAL_TIMEOUT` env (default 2m) logic; judge timeout stays 2m. `ErrorContext` is built in eval from `Response.Command/Stderr/ExitCode` + working dir + env exactly when today's code builds it (`err != nil || stderr != ""`), including the `timeout after %v\n` stderr prefix. The `(>30s)` print moves to the eval wrapper using `Response.Duration`. `AgentConfigDir` is set to `<evals-dir>/agents` only when `<evals-dir>/agents/<agent>.json` exists.
12. **`scoreLLMJudge`** calls `cfg.backend.Invoke` with `RoleJudge` instead of `runKiroCLI` (delete `runKiroCLI`). Parsing is unchanged; cost comes from `Response.Usage`. It needs the case's stub only for the agent role, so the judge request carries none.

### Self-test fixtures (`internal/eval/testdata/evals/`)

Same layout as `.kairon/evals/`:

```
internal/eval/testdata/evals/
  agents/selftest.json              # minimal agent config, prompt: file://./selftest-prompt.md
  agents/selftest-prompt.md
  rubrics/selftest.yaml             # agent: selftest
  cases/selftest/stub-basic.yaml
  cases/selftest/stub-usage.yaml
  fixtures/selftest-input.md        # used by one case through setup[].path: .kairon/evals/fixtures/selftest-input.md (exercises rebasing)
```

- `rubrics/selftest.yaml`: one deterministic criterion named `structural_completeness` (scored by the existing `completeness` heuristic: needs `## ` and `### ` in output ⇒ full marks), one LLM-judged criterion (`clarity`, scoring `1-5`; stub judge returns 5), one `type: cost` criterion.
- `stub-basic.yaml`: `stub.turns[0].response` contains `## ` and `### ` headings; no `usage` (so usage is `estimated`). Uses a `setup` file entry pointing at the fixture.
- `stub-usage.yaml`: `stub.turns[0]` has `response`, `model: stub-model`, and `usage: {input_tokens: 123, output_tokens: 45}` ⇒ results show `tokens_in 123`, `tokens_out 45`, `usage_source: reported`.
- Results land in `internal/eval/testdata/evals/results/` and are ignored via `.gitignore`.

### Non-goals (do not do)
Direct-API backend, model selection, scoring-rule changes, Docker sandbox changes, CI changes, migrating `internal/agent` to the new package, `.kairon/evals` template changes (self-test lives under `testdata`, so `task sync:check` is unaffected).

## 2. Relevant Files

Create:
- `internal/inference/inference.go` (types, interface, `New`, `Names`, `EstimateUsage`, errors)
- `internal/inference/kirocli.go`, `internal/inference/stub.go`
- `internal/inference/inference_test.go`, `kirocli_test.go`, `stub_test.go`
- `internal/eval/config.go` (runConfig, `configure`, `evalsPath`, rebasing helper), `internal/eval/config_test.go`
- `internal/eval/selftest_test.go` (end-to-end stub run with a recording fake `kiro-cli`)
- `internal/eval/testdata/evals/**` (fixtures listed above)

Modify:
- `internal/eval/runner.go` (routing through backend, paths, cost merge, remove `runKiroCLI`/`invokeAgentNative`/`estimateCost` body)
- `internal/eval/perf.go` (startup probe via backend; results dir)
- `internal/eval/types.go` (`RunOptions.Backend`, `EvalsDir`, `Perf`; `TestCase.Stub`; `CostInfo.Model/UsageSource` + `Add`)
- `internal/eval/diff.go` (results dir from evals-dir)
- `cmd/kairon/cmd/eval.go` (`--backend` default `kiro-cli`, persistent `--evals-dir`, pass through `RunOptions`, route `--perf`/`--cleanup` via options so configuration applies first)
- `Taskfile.yml` (`eval:selftest`), `.gitignore`, `docs/evaluation.md`
- Existing tests that reference changed helpers (`runner_test.go`, `runner_config_test.go`, `performance_test.go`, `perf_test.go`, etc.) — adjust minimally; they must keep passing.

Reference (read-only): `internal/eval/selective.go`, `internal/eval/util.go`, `internal/agent/manager.go`, `.kairon/evals/**`, `.kiro/skills/builder-conventions/SKILL.md`.

## 3. Team Orchestration

Mostly a serial spine because `runner.go` is touched by several tasks:

1. `inference-package` (new, isolated) first.
2. `eval-config-and-types` (depends on 1): types, config, path threading, RunOptions.
3. `route-through-backend` (depends on 1, 2): swap all non-Docker kiro-cli call sites for the backend; perf probe; cost merge.
4. In parallel after 3: `cli-task-gitignore` (cmd flags, Taskfile, .gitignore) and `selftest-fixtures` (testdata).
5. After 4: `selftest-e2e-tests` (automated AC 2/3/6/7/8 checks) and `docs-evaluation` in parallel.
6. `validate-all` (validator) last.

## 4. Step-by-Step Task Breakdown

### Task 1: `inference-package`
Create `internal/inference` per decisions 2–4 with unit tests: registry (`New("nope")` error lists `kiro-cli, stub`), stub agent/judge/usage behaviour (reported vs estimated, missing-stub error, judge JSON parseable with the delimiters), kiro-cli backend tested against a fake `kiro-cli` shell script placed first on `PATH` (stdin passthrough, args, ANSI stripping for agent, timeout → `ErrTimeout`, stderr/exit code captured, `AgentConfigDir` overlay staged and cleaned up). Package imports stdlib only.
Dependencies: none.

### Task 2: `eval-config-and-types`
Add `config.go` (`cfg`, `configure`, `evalsPath`, fixture rebasing), extend `RunOptions`/`TestCase`/`CostInfo` (+`Add`, `costFromUsage`), and replace every hardcoded `.kairon/evals/{rubrics,cases,results}` path (except sandbox `tmp`) with `evalsPath`. Default behaviour (no flags) must be identical; existing tests pass.
Dependencies: Task 1.

### Task 3: `route-through-backend`
Make `invokeAgent` (native path) and `scoreLLMJudge` use `cfg.backend`; delete `invokeAgentNative`, `runKiroCLI`; make `MeasureStartupOverhead` and the `exec.LookPath` check use the backend; pass the case's stub script to the agent call; build `ErrorContext` from `Response`; populate `CostInfo.Model/UsageSource`; reject `stub + sandbox`; validate backend first in `RunWithOptions`. Afterwards `grep -n '"kiro-cli"' internal/eval/*.go` (non-test) should only hit the Docker path (`invokeAgentInContainer`) and display strings.
Dependencies: Tasks 1, 2.

### Task 4a: `cli-task-gitignore`
`cmd/kairon/cmd/eval.go`: `--backend` (string, default `kiro-cli`, help lists valid backends from `inference.Names()`), persistent `--evals-dir`; pass into `RunOptions`; `--perf`/`--cleanup` go through `RunWithOptions`. `Taskfile.yml`: `eval:selftest` (desc + `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest`). `.gitignore`: add `internal/eval/testdata/evals/results/`.
Dependencies: Task 3.

### Task 4b: `selftest-fixtures`
Create fixture set described above.
Dependencies: Task 3.

### Task 5a: `selftest-e2e-tests`
`internal/eval/selftest_test.go`: builds a fake `kiro-cli` that appends every invocation to a log file, prepends it to `PATH`, runs `RunWithOptions("selftest", "", RunOptions{Backend:"stub", EvalsDir: <testdata>})` from a temp copy or with results cleaned up, then asserts: no fake invocations recorded; a new run dir exists under `<evals-dir>/results/`; `selftest.json` has the `stub-usage` case with `tokens_in=123`, `tokens_out=45`, `usage_source=reported`, and judge scores at max; unknown backend returns an error containing `kiro-cli` and `stub`; `stub+sandbox` is rejected. Clean up created results dirs in the test.
Dependencies: Tasks 4a, 4b.

### Task 5b: `docs-evaluation`
`docs/evaluation.md`: backend abstraction, `--backend`/`--evals-dir`, stub case fields (`stub.turns[].response`, `.usage`, `.model`), `<evals-dir>/agents/` precedence and the kiro-cli cwd overlay caveat, usage `reported` vs `estimated`, `task eval:selftest`, results location/ignore rule, how to add a new backend (one file in `internal/inference` + registry entry).
Dependencies: Tasks 4a, 4b.

### Task 6: `validate-all`
Validator runs every command below and checks each issue acceptance criterion.
Dependencies: 5a, 5b.

## 5. Validation Commands

```bash
go build ./... && go vet ./... && test -z "$(gofmt -l .)"
go test ./internal/inference/... ./internal/eval/... ./cmd/...
task sync:check

# AC1 (manual review): only the Docker path / backend spawn kiro-cli
grep -rn 'exec.*"kiro-cli"' internal/eval --include=*.go | grep -v _test.go   # expect only sandbox path hits
go list -deps ./internal/inference | grep -c 'kairon/internal/eval'            # expect 0

# AC2 / AC3: fake kiro-cli first on PATH records nothing
FAKE=$(mktemp -d); printf '#!/bin/sh\necho "$@" >> %s/calls.log\n' "$FAKE" > "$FAKE/kiro-cli"; chmod +x "$FAKE/kiro-cli"
PATH="$FAKE:$PATH" go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
test ! -s "$FAKE/calls.log"
ls internal/eval/testdata/evals/results/
git status --porcelain .kairon/evals/results        # empty: nothing added

# AC4
go run ./cmd/kairon eval --evals-dir internal/eval/testdata/evals --list selftest

# AC5 / AC6
task eval:selftest
test -z "$(git status --porcelain internal/eval/testdata/evals/results)"

# AC7
go run ./cmd/kairon eval --backend nope selftest; echo $?   # non-zero, lists kiro-cli, stub

# AC8
grep -h '"usage_source"' internal/eval/testdata/evals/results/*/selftest.json | head
```

## 6. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "inference-package"
    agent: "builder"
    description: "Create internal/inference: Backend interface, Request/Response/Usage types, registry (New/Names with unknown-backend error listing valid backends), kiro-cli backend (behaviour-identical port of agent and judge invocations, startup probe, availability check, AgentConfigDir overlay), deterministic stub backend (stub.turns response/usage/model, passing judge JSON), EstimateUsage, plus unit tests. Stdlib-only; must not import internal/eval."
    dependencies: []
    acceptance_criteria:
      - "internal/inference exports Backend, Request, Response, Usage (reported/estimated), StubScript/StubTurn/StubUsage, ErrTimeout, New, Names, EstimateUsage"
      - "New(\"nope\") returns an error that names the bad backend and lists valid backends kiro-cli and stub"
      - "stub backend agent role returns stub.turns[0].response, reports usage as 'reported' when given and 'estimated' otherwise, and errors clearly when no stub turn exists"
      - "stub backend judge role returns ===JSON_START===/===JSON_END=== delimited JSON with score 5 and pass true"
      - "kiro-cli backend builds the same argv/stdin/ANSI/timeout/error-string behaviour as the old invokeAgentNative and runKiroCLI, verified against a fake kiro-cli on PATH"
      - "package has no dependency on internal/eval"
    validation_commands:
      - "go build ./internal/inference/..."
      - "go vet ./internal/inference/..."
      - "go test ./internal/inference/..."
      - "test \"$(go list -deps ./internal/inference | grep -c 'kairon/internal/eval')\" = \"0\""

  - id: "eval-config-and-types"
    agent: "builder"
    description: "Add internal/eval/config.go (package-level runConfig with evalsDir default .kairon/evals and backend default kiro-cli, configure(), evalsPath(), setup-file path rebasing in assemblePrompt). Extend RunOptions (Backend, EvalsDir, Perf), TestCase (Stub *inference.StubScript yaml:stub), CostInfo (Model, UsageSource, Add, costFromUsage). Replace hardcoded .kairon/evals rubrics/cases/results paths in runner.go, perf.go and diff.go with evalsPath (leave sandbox tmp paths). Default behaviour unchanged."
    dependencies: ["inference-package"]
    acceptance_criteria:
      - "loadRubrics, loadCases, Run rubrics-dir check, all results dirs (single case, Run, perf investigation, resume base dir) and Diff resolve under the configured evals dir"
      - "with no options set the resolved paths are exactly .kairon/evals/{rubrics,cases,results}"
      - "setup[].path values prefixed .kairon/evals/ are rebased onto a non-default evals dir; other paths are untouched"
      - "CostInfo gains omitempty model and usage_source JSON fields and an Add method where merged source is 'reported' only if all parts are reported"
      - "TestCase accepts stub.turns[].response/usage/model from YAML"
      - "all existing eval package tests still pass"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/..."

  - id: "route-through-backend"
    agent: "builder"
    description: "Route all non-Docker model calls through cfg.backend: invokeAgent native path and scoreLLMJudge use Backend.Invoke (delete invokeAgentNative and runKiroCLI), MeasureStartupOverhead uses Backend.StartupProbe, the exec.LookPath check becomes Backend.Available, startup message uses backend name. Pass case stub script to agent calls (evaluate, evaluateProgressive, perf.go). Build ErrorContext from Response exactly as before. Populate CostInfo.Model/UsageSource and use CostInfo.Add for judge cost. RunWithOptions calls configure() first (rejecting unknown backends) and handles Perf; reject stub combined with sandbox. invokeAgentInContainer is unchanged."
    dependencies: ["inference-package", "eval-config-and-types"]
    acceptance_criteria:
      - "no kiro-cli process is started from internal/eval outside invokeAgentInContainer (the Docker path); perf.go contains no exec.Command for kiro-cli"
      - "default (kiro-cli) backend produces the same command lines, timeouts, error strings and ErrorContext as before"
      - "with the stub backend, MeasureStartupOverhead returns without starting a process and the availability check does not consult PATH"
      - "unknown backend and stub+sandbox each return an error before any work is done"
      - "existing eval tests pass"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/..."
      - "test -z \"$(grep -n 'runKiroCLI\\|invokeAgentNative' internal/eval/*.go | grep -v _test.go)\""

  - id: "cli-task-gitignore"
    agent: "builder"
    description: "Update cmd/kairon/cmd/eval.go with --backend (default kiro-cli, help lists inference.Names()) and persistent --evals-dir, pass both via RunOptions (route --perf and --cleanup through RunWithOptions so config applies first). Add Taskfile.yml task eval:selftest running the stub backend against internal/eval/testdata/evals with agent selftest. Add internal/eval/testdata/evals/results/ to .gitignore."
    dependencies: ["route-through-backend"]
    acceptance_criteria:
      - "go run ./cmd/kairon eval --backend nope selftest exits non-zero and the error lists kiro-cli and stub"
      - "`kairon eval --help` documents --backend and --evals-dir; running with no new flags behaves as before"
      - "task eval:selftest is defined with a desc and invokes --backend stub --evals-dir internal/eval/testdata/evals selftest"
      - ".gitignore ignores internal/eval/testdata/evals/results/ but not .kairon/evals/results"
    validation_commands:
      - "go build ./cmd/kairon"
      - "go run ./cmd/kairon eval --help | grep -e '--backend' -e '--evals-dir'"
      - "sh -c 'go run ./cmd/kairon eval --backend nope selftest; test $? -ne 0'"
      - "task --list | grep eval:selftest"
      - "grep -n 'testdata/evals/results' .gitignore"

  - id: "selftest-fixtures"
    agent: "builder"
    description: "Create internal/eval/testdata/evals/ fixture set: rubrics/selftest.yaml (deterministic structural_completeness, LLM-judged clarity, cost criterion), cases/selftest/stub-basic.yaml (stub response with ## and ### headings, setup file entry via .kairon/evals/fixtures/selftest-input.md), cases/selftest/stub-usage.yaml (stub.turns[0] with response, model, usage input_tokens 123 output_tokens 45), fixtures/selftest-input.md, agents/selftest.json and agents/selftest-prompt.md (prompt via file://./selftest-prompt.md)."
    dependencies: ["route-through-backend"]
    acceptance_criteria:
      - "testdata/evals contains a selftest rubric, at least two cases under cases/selftest, and agents/selftest.json with its prompt file"
      - "go run ./cmd/kairon eval --evals-dir internal/eval/testdata/evals --list selftest lists both cases"
      - "fixtures are outside the .kairon/evals template-sync check (task sync:check still passes)"
    validation_commands:
      - "go run ./cmd/kairon eval --evals-dir internal/eval/testdata/evals --list selftest"
      - "test -f internal/eval/testdata/evals/agents/selftest.json && test -f internal/eval/testdata/evals/agents/selftest-prompt.md && test -f internal/eval/testdata/evals/rubrics/selftest.yaml"
      - "task sync:check"

  - id: "selftest-e2e-tests"
    agent: "builder"
    description: "Add internal/eval/selftest_test.go that runs the selftest agent end to end with the stub backend while a recording fake kiro-cli is first on PATH, asserting the fake recorded nothing, a new run dir appeared under <evals-dir>/results, the stub-usage case shows tokens_in 123 / tokens_out 45 / usage_source reported, judge scores are maximum, unknown backend is rejected with valid names listed, and stub+sandbox is rejected. Tests clean up the result dirs they create."
    dependencies: ["cli-task-gitignore", "selftest-fixtures"]
    acceptance_criteria:
      - "test fails if any kiro-cli invocation (including --version probe or LookPath-based check) occurs during a stub run"
      - "test verifies reported usage counts and max judge score in the written results JSON"
      - "test leaves git status of internal/eval/testdata/evals/results empty"
    validation_commands:
      - "go test ./internal/eval/ -run SelfTest -count=1"
      - "test -z \"$(git status --porcelain internal/eval/testdata/evals/results)\""

  - id: "docs-evaluation"
    agent: "documenter"
    description: "Update docs/evaluation.md: inference backend abstraction and where it lives (internal/inference), --backend kiro-cli|stub, --evals-dir layout incl. optional agents/ precedence over .kiro/agents and the kiro-cli temp-cwd overlay caveat, stub case fields (stub.turns[].response/usage/model), reported vs estimated usage in results, self-test fixtures and task eval:selftest, results location and git-ignore rule, how to add a new backend."
    dependencies: ["cli-task-gitignore", "selftest-fixtures"]
    acceptance_criteria:
      - "docs/evaluation.md documents --backend, --evals-dir, stub.turns fields, agents/ precedence, task eval:selftest and usage source semantics"
      - "documentation examples match the implemented flag names and fixture layout"
    validation_commands:
      - "grep -n -e '--backend' -e '--evals-dir' -e 'eval:selftest' -e 'stub.turns' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #283: build/vet/fmt/tests, AC1 (no kiro-cli spawn outside backend and Docker path; eval imports inference), AC2 (fake kiro-cli records nothing on stub run), AC3 (results under testdata/evals/results, none under .kairon/evals/results), AC4 (--list selftest), AC5 (task eval:selftest exits 0), AC6 (git status clean for results), AC7 (unknown backend rejected and lists valid), AC8 (reported usage shown), plus task sync:check and default-flag behaviour unchanged."
    dependencies: ["selftest-e2e-tests", "docs-evaluation"]
    acceptance_criteria:
      - "go build, go vet, gofmt check and go test ./... pass"
      - "all eight issue acceptance criteria verified with the commands in section 5"
      - "task sync:check passes"
    validation_commands:
      - "go build ./... && go vet ./... && test -z \"$(gofmt -l .)\""
      - "go test ./internal/inference/... ./internal/eval/... ./cmd/..."
      - "task eval:selftest"
      - "test -z \"$(git status --porcelain internal/eval/testdata/evals/results)\""
      - "task sync:check"
```
