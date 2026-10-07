# Design Spec: Evals — pin models to an allowlist and record run provenance

Closes #294

## 1. Problem

Stage 3 needs evals to run on mid-tier models and to be traceable. Today (verified in code):

- `internal/eval/runner.go` `scoreLLMJudge` builds `inference.Request{Role: RoleJudge, Prompt, Timeout}` with no model, so `kiro-cli chat --no-interactive` runs on the account default (`auto`, possibly a frontier model).
- `internal/inference/kirocli.go` never passes `--model` for either role, and `Response.Model` is always `""` for kiro-cli.
- Results record only `git_hash` (`AgentResult`, `Summary`). Nothing says which agent model, judge model or prompt text produced a score.
- `CostInfo` (`agent_cost` / `judge_cost`) is per-case and aggregated. Individual calls are not recorded.
- There is no `evals` block in `internal/config` or either `config.yaml`.

All six shipped agent configs pin `"model": "claude-sonnet-5.5"`, but one development machine only has `claude-sonnet-4.5`, so a run must be able to override the agent and judge models without editing agent configs.

## 2. Solution Approach

Four cooperating pieces, each in the package that already owns the concern:

1. **`internal/config`** owns the `evals` block, its defaults and the allowlist check (`EvalsConfig.CheckModel`). A lightweight `LoadEvals()` reads only the `evals:` key of `.kairon/config.yaml`, so eval runs don't need `repo:` or a theme (`config.Load()` requires `repo` and prints theme warnings; it is the wrong entry point for the eval harness).
2. **`internal/inference`** gains `Request.Model`. The kiro-cli backend turns it into `--model <m>` for both roles and echoes it in `Response.Model` (kiro-cli can't report the served model). `inference` also defines `CallRecord`, the per-call audit record. It lives here, not in `internal/eval`, because `inference` is stdlib-only and importable by the agent manager; Stage 4 can then reuse the exact shape for production workflow steps without pulling in `internal/eval` (Docker client etc.).
3. **`internal/eval`** adds a *pre-flight* (`pinRun`) that runs in `RunWithOptions` before any case starts. It resolves and validates the judge model and each in-scope agent's effective model, and computes each agent's `prompt_sha256` and `resources_present`. The result is kept in package run state `cfg.pins` (same "configure once, read-only during the run" invariant documented in `internal/eval/config.go`). The runner then passes the pinned models in every `inference.Request` and records one `CallRecord` per agent call and per judge call.
4. **Docs/config/fixtures** document the block in both config files and `docs/evaluation.md`, and the self-test fixture gains a `model` and a non-existent `*-conventions` resource.

### 2.1 Config: `evals` block (`internal/config`)

New file `internal/config/evals.go`:

```go
const DefaultJudgeModel = "claude-sonnet-5.5"

type EvalsConfig struct {
    AgentModel    string   `yaml:"agent_model"`    // optional override for the agent under test
    JudgeModel    string   `yaml:"judge_model"`    // default claude-sonnet-5.5
    AllowedModels []string `yaml:"allowed_models"` // default: see below
}

func DefaultEvalsConfig() EvalsConfig          // fresh slice each call (callers may mutate)
func LoadEvals() (EvalsConfig, error)          // reads ./.kairon/config.yaml, evals key only
func (e EvalsConfig) CheckModel(model string) error
```

Default `allowed_models`, in this order: `claude-sonnet-5.5`, `claude-sonnet-5`, `claude-sonnet-4.6`, `claude-sonnet-4.5`, `claude-sonnet-4`, `claude-haiku-4.5`.

- `Config` gets `Evals EvalsConfig \`yaml:"evals"\`` and `Load()` pre-populates it with `DefaultEvalsConfig()` before `yaml.Unmarshal` (same pattern as `Session`/`Sandbox`), so an absent or partial block keeps defaults. `Load()` does **not** reject model values; enforcement is a run-time concern of the eval harness (the effective agent model depends on the agent under test).
- `LoadEvals()`: file missing → defaults, nil error (lets `go test` in `internal/eval` and runs in repos without a config work); read error other than not-exist, or YAML parse error → error (the run is refused rather than guessing). It unmarshals into `struct{ Evals EvalsConfig \`yaml:"evals"\` }` pre-populated with defaults. yaml.v3 replaces a slice when the key is present, so a user-supplied `allowed_models` **replaces** the default list (it must not append; add a test). All string fields and list entries are `strings.TrimSpace`d; empty list entries are dropped. An explicit `judge_model: ""` stays empty (and is later refused). An explicit empty `agent_model` means "not set".
- `CheckModel(model)` returns nil only if the trimmed model is non-empty, is not `auto` (case-insensitive), and is exactly in `AllowedModels`. Error text always names the rejected model (or says it is empty) **and** prints the allowlist: e.g. `model "auto" is not permitted; allowed_models: [claude-sonnet-5.5, claude-sonnet-5, ...]`, `model is empty; allowed_models: [...]`, `allowed_models is empty: no model can be used`.

Both `.kairon/config.yaml` and `cmd/kairon/templates/kairon/config.yaml` get the same **commented** documentation block (defaults apply when it is absent; matches how the `sandbox` block is documented). The two files are already not byte-identical (the template is a minimal file), and `task sync:check` does not cover them, so "in sync" means: same block, same wording, same defaults.

```yaml
# Eval model pinning (optional). Applies to `kairon eval`.
# Every agent call and judge call is pinned to a model; a run is refused before
# any case starts if the effective agent model or the judge model is empty,
# "auto", or not in allowed_models.
# evals:
#   agent_model: ""                 # override the agent-under-test model; default: the "model" in its agent config
#   judge_model: "claude-sonnet-5.5"
#   allowed_models:
#     - "claude-sonnet-5.5"
#     - "claude-sonnet-5"
#     - "claude-sonnet-4.6"
#     - "claude-sonnet-4.5"
#     - "claude-sonnet-4"
#     - "claude-haiku-4.5"
```

### 2.2 Inference (`internal/inference`)

- `Request.Model string` — "" means "backend default / unpinned" (keeps every existing direct caller and test unchanged).
- `kiroCLIBackend.invokeAgent`: args become `chat --agent <a> --no-interactive --trust-all-tools [--model <m>]`; `invokeJudge`: `chat --no-interactive [--model <m>]`. `--model` is appended **after** the existing flags so the existing argv prefix (and the existing tests that assert it) is unchanged when no model is set. `Response.Command` includes the flag. On success **and** error, `Response.Model = req.Model` (the model the process was launched with; kiro-cli can't report the served model). `kiro-cli chat --model <MODEL>` exists (checked with `kiro-cli chat --help`).
- `stubBackend` is unchanged: it ignores `Request.Model` and keeps reporting its scripted model (`stub-model`) / `stub`. The stub is deterministic and never launches a model, so records from a stub run show `stub`/`stub-model` in `model`; the pinned models still appear in the run-level fields.
- New `internal/inference/record.go`:

```go
// CallRecord is one inference call, in the shape shared by eval runs and
// (Stage 4) production workflow audit trails.
type CallRecord struct {
    Role         string  `json:"role"`                       // "agent" | "judge"
    Model        string  `json:"model"`                      // served model when the backend reports one, else the pinned (requested) model
    Agent        string  `json:"agent,omitempty"`            // agent calls
    Criterion    string  `json:"criterion,omitempty"`        // judge calls
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    CostUSD      float64 `json:"cost_usd"`
    Estimated    bool    `json:"estimated"`                  // true unless usage source is "reported"
    DurationMS   int64   `json:"duration_ms"`
    PromptSHA256 string  `json:"prompt_sha256,omitempty"`    // agent calls only
    Error        string  `json:"error,omitempty"`            // call failed; record is still written
}
```

### 2.3 Eval provenance (`internal/eval`)

**Types (`types.go`)**

- `CaseResult.Calls []inference.CallRecord \`json:"calls,omitempty"\`` — one entry per agent call and per judge call, in execution order. `AgentCost`/`JudgeCost` stay (used by `diff.go`, `updateIncrementalSummary`, docs).
- `AgentResult` gains `AgentModel`, `JudgeModel`, `PromptSHA256` (`omitempty`) and `ResourcesPresent []string \`json:"resources_present"\`` (not omitempty; always a non-nil slice when provenance was resolved, so an all-missing list serialises as `[]`, which makes "omits the missing resource" observable).
- `Summary` gains `JudgeModel string \`json:"judge_model,omitempty"\``, `AgentModel`, `PromptSHA256` (`omitempty`), `ResourcesPresent []string \`json:"resources_present,omitempty"\``, plus `Agents map[string]AgentProvenance \`json:"agents,omitempty"\`` where `AgentProvenance{AgentModel, PromptSHA256 string; ResourcesPresent []string}`.
  **Multi-agent rule:** a run can cover several agents (`kairon eval` with no agent), each with its own model and hash, so a single top-level value would be ambiguous. `summary.json` always carries `judge_model` and the per-agent `agents` map; the top-level `agent_model`, `prompt_sha256`, `resources_present` are populated **only when the run covers exactly one agent** (`len(Agents) == 1`, which is the self-test case and `kairon eval <agent>`) and are omitted/cleared otherwise. The per-agent `<agent>.json` always carries all four fields. This satisfies "run summary records the four fields" for single-agent runs without inventing a lossy aggregate.

**New file `internal/eval/provenance.go`**

- `resolveAgentProvenance(agent string, container bool) (agentProvenance, error)`:
  1. Config path: `<evals-dir>/agents/<agent>.json` if it exists (reuse `agentConfigDir`), else `.kiro/agents/<agent>.json` relative to cwd. When `container` is true (Docker sandbox run) only `.kiro/agents/<agent>.json` is considered, because the container ignores the evals-dir overlay. Neither exists → error naming both paths.
  2. Parse JSON into `{Model string; Prompt string; Resources []json.RawMessage}`; only string entries of `resources` are considered (newer kiro configs may hold object entries; ignore them).
  3. Prompt: if `prompt` starts with `file://`, read that file (relative paths resolve against the config file's directory, as kiro does; absolute allowed). An unreadable/missing prompt file is an **error** (the prompt is required). An inline prompt is covered by the config bytes.
  4. Resources: for each string entry in config order, strip a `file://` or `skill://` prefix (other schemes are not local files → treated as not present). Relative paths resolve against the **process working directory** (the repo root for real runs). If the path contains glob metacharacters (`*?[`), expand with `filepath.Glob` (sorted); the entry counts as present if ≥1 regular file matches and every match is hashed. Otherwise `os.Stat`: a missing file is silently skipped (normal for `*-conventions` overrides, never an error); a non-not-exist stat/read error is an error. `ResourcesPresent` lists the entry strings exactly as written in the config (`skill://.kiro/skills/sentinel-protocol/SKILL.md`), in config order.
  5. `prompt_sha256`: lowercase hex SHA-256 over an ordered sequence of parts — config bytes, prompt-file bytes (if any), then each existing resource file's bytes in config order (glob matches sorted). Each part is framed as `<kind>\x00<decimal length>\x00<bytes>` with kind ∈ `config` | `prompt` | `resource`, so concatenation can't be ambiguous. File paths are **not** hashed (keeps the hash machine-independent). Consequences: editing the prompt, any present resource, or the config changes the hash; adding/removing a *missing* resource file changes the hash exactly when the file appears.
  6. `Model` = config's `model` (trimmed); the effective model is decided by the caller.
- `pinRun(agent string, opts RunOptions, container bool) error` (called from `RunWithOptions`):
  1. `config.LoadEvals()`; error → refuse.
  2. Judge: `CheckModel(JudgeModel)`.
  3. Agents in scope: `[agent]` if non-empty, else the agents from `loadRubrics("")`. If the rubrics can't be read, the agent part is skipped so that the existing, more specific downstream error (`rubrics directory not found`, etc.) is reported, and `agent == ""` (e.g. `--perf` without an agent) likewise skips it. For each agent: `resolveAgentProvenance`; effective model = `evals.agent_model` when non-empty, else the config model. **With a container run, `evals.agent_model` is ignored** (the sandbox builds its own `kiro-cli chat` command in `invokeAgentInContainer`, which is out of scope): the effective model is the config model and a warning line is printed. `CheckModel(effective)`.
  4. **All** violations are collected and returned together in one error (judge first, then agents), each naming the setting/agent, the rejected value (or "empty"), where an agent model came from (`evals.agent_model` or the config path), and the allowlist; followed by a hint to edit `evals` in `.kairon/config.yaml`. Nothing has been executed and no results directory exists at this point.
  5. On success set `cfg.pins = &runPins{Judge: ..., Agents: map[string]agentProvenance{...}}` and print one line, e.g. `🔒 Models: judge=claude-sonnet-5.5, selftest=claude-sonnet-5.5 (prompt sha256 1a2b3c4d…)`.
- `cfg.pins == nil` (any path that doesn't go through `RunWithOptions`, i.e. existing unit tests calling `Run`, `invokeAgent`, `scoreLLMJudge` directly) means **unpinned**: empty `Request.Model`, no `--model`, no provenance fields. `configure()` resets `pins` to nil so settings never leak between runs.

**Wiring (`runner.go`, `scoring.go`, `perf.go`, `config.go`)**

- `RunWithOptions` order: `configure` → existing stub+sandbox rejection → `Cleanup` early return → **`pinRun` unless `options.List`** → `Perf` → sandbox setup → rest. `container` for `pinRun` is `options.Sandbox && !options.NoSandbox && !options.Perf && !options.List && testcase == "" && !options.Resume`, i.e. exactly when a non-nil `ContainerConfig` reaches `Run`. (Single-case, resume and perf paths never containerise today.)
- `invokeAgentViaBackend` sets `Request.Model = cfg.pins.agentModel(agent)` ("" when unpinned). `scoreLLMJudge` sets `Request.Model = cfg.pins.judgeModel()`. Both measure wall time around `Invoke` and build a `CallRecord` with a shared helper `newCallRecord(req, resp, err, wall, cost)`: `Model` = `resp.Model` if non-empty else `req.Model`; `DurationMS` = `resp.Duration` if non-zero else `wall` (the stub reports 0); `Estimated` = usage source != `reported`; `CostUSD` from the same `costFromUsage` used for `CostInfo`; agent calls set `Agent` and `PromptSHA256` (from `cfg.pins`); judge calls set `Criterion`; a failed call sets `Error` and is still recorded. `scoreLLMJudge` records usage even when the judge output can't be parsed (tokens were spent), while its existing return of `CostInfo{}` on parse failure is left unchanged.
- Signature changes (append the record, keep the order of existing returns; update the listed call sites):
  - `invokeAgent(...) (string, CostInfo, inference.CallRecord, *ErrorContext, error)` — callers: `evaluate`, `evaluateProgressive`, `perf.go` (2 sites, discard), `backend_routing_test.go` (several).
  - `scoreLLMJudge(...) (CostInfo, int, string, bool, inference.CallRecord)` — callers: `scoreCase`, `backend_routing_test.go`.
  - The container path (`invokeAgentInContainer`) is not restructured; `invokeAgent` builds its record from wall-clock time, the returned `CostInfo` (estimated), and `Model` = the resolved config model.
- `evaluate` and `evaluateProgressive` append the agent record to `cr.Calls` (also on failure; not when prompt assembly failed because no call was made) and initialise `AgentResult{AgentModel, JudgeModel, PromptSHA256, ResourcesPresent}` from `cfg.pins`. `scoreCase` appends each judge record to `cr.Calls`.
- `updateIncrementalSummary` copies `JudgeModel` and `AgentProvenance` from the `AgentResult` into `Summary` and applies the single-agent rule above.
- `runSingleTestCase` currently writes only `<agent>.json`; it also calls `updateIncrementalSummary` so single-case runs (the AC2 verification command) have a `summary.json` with the same provenance fields.
- **Resume integrity:** in `runProgressiveEvaluation` with `isResume`, if an existing `<agent>.json` has a non-empty `prompt_sha256` and it, `agent_model` or `judge_model` differs from the current pins, return an error (`cannot resume: prompt or models changed since the interrupted run`), so one result file never mixes two prompt/model versions.

**Known limitation (documented, not fixed):** `updateIncrementalSummary` overwrites `TotalCost` with the last agent's cost instead of accumulating (pre-existing); not touched here.

### 2.4 Self-test fixture (`internal/eval/testdata/evals/agents/selftest.json`)

Add `"model": "claude-sonnet-5.5"` and `"resources": ["skill://.kiro/skills/selftest-conventions/SKILL.md"]` (the file doesn't exist from either cwd used — repo root for `go run`, `internal/eval` for `go test` — and is referenced only as an optional project-override hook). Resulting `resources_present` is `[]`. The fixtures sit under `testdata`, outside the template-synced `.kairon/evals/`, so `task sync:check` is unaffected.

### 2.5 Sandbox (out of scope, documented)

`invokeAgentInContainer` keeps building its own argv (`kiro-cli chat --agent X --no-interactive --trust-all-tools`) and does **not** honor `evals.agent_model`. Judge calls in a `--sandbox` run still execute on the host through the backend and do honor `judge_model`. `docs/evaluation.md` must say both.

## 3. Relevant Files

Modify:
- `internal/config/config.go` (add `Evals` field + defaults)
- `internal/inference/inference.go` (`Request.Model`), `internal/inference/kirocli.go` (`--model`, `Response.Model`)
- `internal/eval/types.go`, `internal/eval/config.go`, `internal/eval/runner.go`, `internal/eval/scoring.go`, `internal/eval/perf.go` (call-site only)
- `internal/eval/testdata/evals/agents/selftest.json`
- `.kairon/config.yaml`, `cmd/kairon/templates/kairon/config.yaml`
- `docs/evaluation.md`
- Existing tests that call changed signatures: `internal/eval/backend_routing_test.go`, `internal/eval/scoring_test.go` (uses `evaluate`/`evaluateProgressive`, signatures unchanged, re-check), `internal/eval/selftest_test.go` (extend)

Create:
- `internal/config/evals.go`, `internal/config/evals_test.go`
- `internal/inference/record.go`, tests in `internal/inference/kirocli_test.go` / new `record_test.go`
- `internal/eval/provenance.go`, `internal/eval/provenance_test.go`
- `cmd/kairon/cmd/eval_test.go` (extend; no production change in `cmd/kairon` is needed)

Do not touch: any agent config `model` field (out of scope); CI workflows; `invokeAgentInContainer` argv; `.kairon/specs/` historical specs other than this one.

**Working-tree note for builders/validators:** this worktree has uncommitted edits to `.kiro/agents/*.json` (injected `@creds-agent` MCP server config, reformatting) that are not part of this issue. Do not revert, stage or "sync" them. `task sync:check` therefore already reports `.kiro/agents` differences before any change; judge sync by confirming no *new* differences outside `.kiro/agents` (in particular none under `.kairon/evals`, `.kairon/scripts`, `.kairon/themes`, `.kiro/skills/sentinel-protocol`).

## 4. Team Orchestration

```
config-evals-block ──────────────┐
inference-model-flag ─┬──────────┤
                      └─► eval-provenance-core ─┤
                                                ▼
                                       eval-wire-run ─► docs-evaluation ─► validate-all
```

- `config-evals-block` and `inference-model-flag` touch disjoint packages and run in parallel.
- `eval-provenance-core` (types, hashing, fixture) needs `inference.CallRecord` from `inference-model-flag`, but not the config package.
- `eval-wire-run` needs all three: config (`LoadEvals`/`CheckModel`), inference (`Request.Model`, `CallRecord`), provenance (types, `resolveAgentProvenance`).
- `docs-evaluation` documents final behaviour, so it follows wiring. `validate-all` is read-only and runs last.
- Single PR; every acceptance criterion is covered (table in §7).

## 5. Step-by-Step Task Breakdown

### Task 1 — `config-evals-block` (builder)
Implement §2.1: `internal/config/evals.go` (`EvalsConfig`, `DefaultEvalsConfig`, `LoadEvals`, `CheckModel`), `Config.Evals` with defaults in `Load()`, and the commented `evals` block in both config files. Tests (`evals_test.go`): defaults (judge `claude-sonnet-5.5`, the 6-model list in order, empty agent_model); partial block keeps other defaults; user `allowed_models` replaces (not appends to) the default; missing file → defaults; malformed YAML → error; whitespace trimmed; `CheckModel` accepts an allowed model and rejects empty, `auto`/`AUTO`, unknown, and empty allowlist with messages that contain the model and the allowlist; `Load()` exposes `cfg.Evals` defaults; a test that reads both `.kairon/config.yaml` and `cmd/kairon/templates/kairon/config.yaml` (paths relative to the package: `../../.kairon/config.yaml`, `../../cmd/kairon/templates/kairon/config.yaml`) and asserts each documents `evals:`, `agent_model`, `judge_model`, `allowed_models` and every default model string.

### Task 2 — `inference-model-flag` (builder)
Implement §2.2. Tests in `kirocli_test.go` using the existing fake kiro-cli: agent argv is `chat --agent <a> --no-interactive --trust-all-tools --model <m>` and judge argv is `chat --no-interactive --model <m>` when `Model` is set; argv and `Command` are unchanged when it is empty (existing assertions keep passing); `Response.Model == req.Model` on success and on failure; stub ignores `Model` (`stub_test.go`). `record_test.go`: `CallRecord` JSON field names/round-trip.

### Task 3 — `eval-provenance-core` (builder)
Implement §2.3 types, `provenance.go` (`resolveAgentProvenance`, hashing) and §2.4 fixture. Tests (`provenance_test.go`, using `chdirTemp` + temp project): config lookup precedence (evals-dir over `.kiro/agents`; container ignores the overlay); hash is 64 lowercase hex and stable across runs; changes when the prompt file, a present resource, or the config changes; order matters (swapping two resources changes the hash); a missing `*-conventions` resource is skipped without error and absent from `ResourcesPresent`, and creating the file later changes the hash; glob resources; non-string `resources` entries ignored; missing prompt file errors; missing agent config errors naming both paths; `ResourcesPresent` is a non-nil empty slice when none exist. Also assert the committed `selftest.json` has a model in the default allowlist and a non-existent `*-conventions` resource.

### Task 4 — `eval-wire-run` (builder)
Implement the remaining §2.3 wiring: `runPins`/`cfg.pins` (reset in `configure`), `pinRun` and its call order in `RunWithOptions`, `Request.Model` in both call sites, `newCallRecord`, signature changes and call-site updates (incl. existing tests), `CaseResult.Calls`, `AgentResult`/`Summary` population, single-case `summary.json`, resume integrity check, sandbox warning. Tests:
- *Pinning with a fake kiro-cli* (`installFakeKiroCLI`; because `getGitShortHash` needs a repo, add a small `initTempGitRepo(t)` helper that `git init`s + commits in the temp project; temp project has `.kiro/agents/architect.json` with `model: claude-sonnet-5.5`, a rubric with one LLM criterion, a case): single-case run logs `--agent architect ... --model claude-sonnet-5.5` for the agent call; with `.kairon/config.yaml` `evals.agent_model: claude-sonnet-4.5` it logs that; **every** judge call line contains `--model <judge_model>` (default and overridden).
- *Refusal before any case*: `judge_model: auto`, `""`, and a model not in `allowed_models`; agent config without `model` and no override; `agent_model` not in allowlist. Assert error names the rejected model and the allowlist, kiro-cli log is empty (not even `--version`), and no results directory was created. Multiple violations are all reported. Stub backend is refused the same way. `--list` and `--cleanup` are not blocked.
- *Sandbox*: pure-function test that with `container=true` `evals.agent_model` is ignored and the config model is validated.
- *Self-test e2e* (stub, copy of the fixtures in a temp dir to avoid touching the tree): result file has `agent_model`, `judge_model`, `prompt_sha256` (64 hex), `resources_present == []`; every case has one `role=agent` and N `role=judge` records with model, `input_tokens`, `output_tokens`, `cost_usd`, `estimated` (false for the `stub-usage` agent call, true otherwise), `duration_ms` field present, and `prompt_sha256` on agent records only and equal to the file-level value; `summary.json` has `judge_model`, `agent_model`, `prompt_sha256`, `resources_present` and `agents.selftest`. Multi-agent summary omits top-level agent fields but keeps `agents`.
- *AC7*: run the self-test twice against two temp copies of the fixtures, the second with `selftest-prompt.md` edited; `prompt_sha256` differs; an unchanged copy gives an identical hash.
- *AC6*: a temp project whose agent config lists one existing and one missing `*-conventions` resource: run succeeds; `resources_present` contains only the existing one.
- *Resume*: resuming after the prompt file changed is refused; an unchanged resume continues.
- *cmd* (`cmd/kairon/cmd/eval_test.go`): chdir into a temp dir whose `.kairon/config.yaml` sets `evals.judge_model: auto`, set `evalBackend = "stub"` and `evalEvalsDir` to the absolute path of `internal/eval/testdata/evals`, call `evalCmd.RunE(evalCmd, []string{"selftest"})` → error containing `auto` and `allowed_models` (main turns any `RunE` error into exit status 1).
- Existing tests keep passing with no `.kairon/config.yaml` (defaults apply) — in particular `TestSelfTestStubRunEndToEnd` (cwd `internal/eval`) and `TestRunWithOptionsPerfRoutesToInvestigation`.

### Task 5 — `docs-evaluation` (documenter)
Update `docs/evaluation.md`: new section **Model Pinning and Run Provenance** covering the `evals` block (keys, defaults, how `agent_model` falls back to the agent config `model`), allowlist refusal (what is rejected, that it happens before any case starts, the error shape, all violations listed), provenance fields (per-call `calls[]` record with every field, `agent_model` / `judge_model` / `prompt_sha256` / `resources_present` in `<agent>.json` and `summary.json`, the single-agent rule and `agents` map, how `prompt_sha256` is computed and that missing resources are skipped, relative resource paths resolve from the working directory, `model` meaning for stub runs), resume refusal when prompt/models changed, and a clearly marked **`--sandbox` does not honor `evals.agent_model`** note (sandboxed agent calls run on the model in the agent config; judge calls still honor `judge_model`). Also correct now-stale statements: the Inference Backends table / "Reported vs Estimated Usage" claim that kiro-cli's `model` is empty (it is now the pinned model when a run is pinned), the self-test fixture listing (`selftest.json` has `model` and a non-existent `*-conventions` resource), and the Flags/Running sections if needed. Add an example result snippet.

### Task 6 — `validate-all` (validator)
Read-only verification of every acceptance criterion and the full QA suite (see §6/§7).

## 6. Validation Commands

```bash
go build ./...
go vet ./internal/config/... ./internal/inference/... ./internal/eval/... ./cmd/...
go test ./internal/config/... ./internal/inference/... ./internal/eval/... ./cmd/...
task test
task lint
task eval:selftest                       # stub run must still succeed with the updated fixture

# AC4: refused before any case, names model + allowlist (needs judge_model: auto in a *temp* project config;
# this is covered by TestEvalRejectsAutoJudgeModel in cmd/kairon/cmd — do not edit the repo's .kairon/config.yaml)
go test ./cmd/kairon/cmd -run 'TestEvalRejectsUnpinnedJudgeModel' -v

# AC2/AC3/AC5/AC6/AC7 (fake kiro-cli, stub e2e, hash change)
go test ./internal/eval -run 'TestPin|TestProvenance|TestCallRecords|TestSelfTest' -v

task sync:check    # pre-existing diffs in .kiro/agents are expected in this worktree (see §3); no others may appear
```

Manual spot checks (validator; use a scratch copy so the repo's results dir and config stay untouched):
`cp -r .kairon/evals "$TMP/evals"`; run the built binary with `--evals-dir "$TMP/evals"` inside a scratch git repo that has `.kiro/agents`, `.kairon/config.yaml`, and a fake `kiro-cli` first on `PATH` that appends `"$@"` to a log and prints `ok`; confirm the log shows `--agent architect ... --model claude-sonnet-5.5`, that `evals.agent_model: claude-sonnet-4.5` changes it, and that each judge line carries `--model <judge_model>`.

## 7. Acceptance-Criteria Traceability

| AC | Where satisfied | Verified by |
|----|-----------------|-------------|
| 1 config block + defaults in both files | §2.1, Task 1 | `internal/config` tests incl. template/live file test |
| 2 agent model = override else agent config | `pinRun`, `invokeAgentViaBackend` | Task 4 fake-kiro-cli tests |
| 3 judge uses `judge_model` | `scoreLLMJudge` + `--model` | Task 4 fake-kiro-cli test (every judge line) |
| 4 refusal before any case | `pinRun` in `RunWithOptions` | Task 4 refusal tests + cmd test |
| 5 per-call records + run summary fields | `CallRecord`, `CaseResult.Calls`, `AgentResult`, `Summary` | Task 4 self-test e2e |
| 6 missing `*-conventions` is normal | `resolveAgentProvenance`, fixture | Tasks 3 & 4 tests |
| 7 prompt change changes hash | hashing in §2.3 | Tasks 3 & 4 tests |
| 8 docs | Task 5 | validator reads `docs/evaluation.md` |

## 8. Risks and Notes

- **`--model` with `--agent`:** `kiro-cli chat --model` is a documented flag; that it overrides an agent config's `model` was not exercised against a live model here. The tests verify argv only.
- **Resource base directory:** relative `skill://`/`file://` resources resolve against the process cwd. When an evals-dir `agents/` overlay is used, kiro runs in a temp cwd that only contains the copied agents dir, so such resources are not loaded by kiro in that mode either; the hash reflects the files the harness can see from the repo root. Documented in `docs/evaluation.md`.
- **`model` in records under stub:** stub runs record `stub`/`stub-model`, not the pinned names; this is intentional (truthful) and documented.
- **Signature churn:** `invokeAgent` and `scoreLLMJudge` are package-private; all callers are in `internal/eval` (listed in §2.3).
- **Provenance is a hash, not stored prompt text.** The issue title says "prompt text" but AC5/AC7 define `prompt_sha256`; we store the hash only.
- **Pre-existing, left alone:** `TotalCost` overwrite in `updateIncrementalSummary`; `buildSummary` is not on the write path and is not extended.

## 9. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "config-evals-block"
    agent: "builder"
    description: "Add the evals config block in internal/config (EvalsConfig, DefaultEvalsConfig, LoadEvals, CheckModel, Config.Evals with defaults) and document it as a commented block with defaults in .kairon/config.yaml and cmd/kairon/templates/kairon/config.yaml, with unit tests."
    dependencies: []
    acceptance_criteria:
      - "EvalsConfig has agent_model, judge_model (default claude-sonnet-5.5) and allowed_models (default claude-sonnet-5.5, claude-sonnet-5, claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4, claude-haiku-4.5 in that order)"
      - "LoadEvals returns defaults when .kairon/config.yaml is missing, errors on malformed YAML, and a user allowed_models list replaces the default list"
      - "CheckModel rejects empty, auto (any case), models not in allowed_models and an empty allowlist, and every rejection message names the model and prints the allowlist"
      - "Config.Load exposes Config.Evals populated with defaults and does not reject model values"
      - "Both .kairon/config.yaml and cmd/kairon/templates/kairon/config.yaml contain the same commented evals block listing agent_model, judge_model and all default allowed_models, asserted by a test"
    validation_commands:
      - "go build ./internal/config/..."
      - "go test ./internal/config/..."
      - "go vet ./internal/config/..."

  - id: "inference-model-flag"
    agent: "builder"
    description: "Add Request.Model to internal/inference, make the kiro-cli backend pass --model for agent and judge calls and echo it in Response.Model, and add the shared CallRecord type in internal/inference/record.go."
    dependencies: []
    acceptance_criteria:
      - "Agent argv is 'chat --agent <a> --no-interactive --trust-all-tools --model <m>' and judge argv is 'chat --no-interactive --model <m>' when Request.Model is set"
      - "argv and Response.Command are unchanged when Request.Model is empty so existing tests still pass"
      - "Response.Model equals Request.Model for the kiro-cli backend on success and on error; the stub backend ignores Request.Model"
      - "inference.CallRecord has role, model, agent, criterion, input_tokens, output_tokens, cost_usd, estimated, duration_ms, prompt_sha256 and error JSON fields and round-trips through encoding/json"
      - "internal/inference stays stdlib-only and does not import internal/eval"
    validation_commands:
      - "go build ./internal/inference/..."
      - "go test ./internal/inference/..."
      - "go vet ./internal/inference/..."

  - id: "eval-provenance-core"
    agent: "builder"
    description: "In internal/eval add the provenance types (CaseResult.Calls, AgentResult and Summary provenance fields, AgentProvenance), implement resolveAgentProvenance with the prompt_sha256 and resources_present rules in internal/eval/provenance.go, and update the selftest.json fixture with a model and a non-existent *-conventions resource."
    dependencies: ["inference-model-flag"]
    acceptance_criteria:
      - "CaseResult has Calls []inference.CallRecord; AgentResult has agent_model, judge_model, prompt_sha256 and resources_present (resources_present serialises as [] when empty); Summary has judge_model, agent_model, prompt_sha256, resources_present and an agents map"
      - "resolveAgentProvenance prefers <evals-dir>/agents/<agent>.json over .kiro/agents/<agent>.json, ignores the overlay when container is true, and errors naming both paths when neither exists"
      - "prompt_sha256 is lowercase 64-char hex over config bytes, prompt file bytes and each existing resource in config order with length-framed parts; it changes when the prompt, a present resource or the config changes, and when two resources are reordered"
      - "A missing resource (such as a *-conventions skill) is skipped without error and omitted from resources_present; a missing prompt file is an error; non-string resources entries are ignored; glob resources are expanded"
      - "internal/eval/testdata/evals/agents/selftest.json has model claude-sonnet-5.5 and a resources entry for a non-existent selftest-conventions skill, and existing self-test tests still pass"
    validation_commands:
      - "go build ./internal/eval/..."
      - "go test ./internal/eval/... -run 'TestProvenance|TestSelfTest|TestDefaultConfig'"
      - "go vet ./internal/eval/..."

  - id: "eval-wire-run"
    agent: "builder"
    description: "Wire model pinning and provenance into the eval runner: pinRun pre-flight in RunWithOptions (judge and per-agent model validation against the allowlist, refusal before any case), cfg.pins, Request.Model on agent and judge calls, one CallRecord per call, AgentResult/Summary provenance population, single-case summary.json, resume integrity check, sandbox handling of evals.agent_model, plus updates to existing tests for changed signatures and new end-to-end tests."
    dependencies: ["config-evals-block", "inference-model-flag", "eval-provenance-core"]
    acceptance_criteria:
      - "With a fake kiro-cli on PATH, a single-case architect run logs '--model claude-sonnet-5.5' on the agent call, logs '--model claude-sonnet-4.5' when evals.agent_model is claude-sonnet-4.5, and every judge call line carries '--model <judge_model>'"
      - "A run with evals.judge_model auto or empty or not in allowed_models, or an effective agent model that is empty, auto or not allowed, is refused before any case starts: no kiro-cli invocation, no results directory, non-zero error naming the rejected model and the allowlist; all violations are reported together"
      - "'eval --backend stub --evals-dir internal/eval/testdata/evals selftest' with judge_model auto fails with an error naming auto and allowed_models (cmd test), and succeeds with defaults"
      - "Self-test results contain one role=agent and one role=judge record per call with model, input_tokens, output_tokens, cost_usd, estimated, duration_ms, and prompt_sha256 on agent records; <agent>.json and summary.json contain agent_model, judge_model, prompt_sha256 and resources_present; resources_present omits the missing selftest-conventions resource"
      - "Editing the self-test prompt changes the recorded prompt_sha256 between two runs; an unedited copy produces the same hash"
      - "With a container (sandbox) run evals.agent_model is ignored, a warning is printed and the config model is validated and recorded; judge calls still use judge_model"
      - "Resuming a run after the prompt or models changed is refused; single-case runs also write summary.json with the provenance fields"
      - "All pre-existing tests in internal/eval and cmd/kairon/cmd pass (call sites of invokeAgent and scoreLLMJudge updated) and agent config model fields are unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/... ./internal/inference/... ./internal/config/... ./cmd/..."
      - "go vet ./internal/eval/... ./cmd/..."
      - "task eval:selftest"

  - id: "docs-evaluation"
    agent: "documenter"
    description: "Update docs/evaluation.md to document the evals block and defaults, allowlist refusal, the recorded provenance fields (per-call records, prompt_sha256 computation, resources_present, run summary fields and the single-agent rule), resume refusal, and that --sandbox runs do not honor evals.agent_model; fix statements made stale by this change."
    dependencies: ["eval-wire-run"]
    acceptance_criteria:
      - "docs/evaluation.md has a section documenting evals.agent_model, evals.judge_model and evals.allowed_models with their defaults and the fallback to the agent config model"
      - "The allowlist refusal is documented: rejected values (empty, auto, not in allowed_models), that it happens before any case starts, and the error contents"
      - "All provenance fields are documented: per-call role, model, input_tokens, output_tokens, cost_usd, estimated, duration_ms, prompt_sha256; and agent_model, judge_model, prompt_sha256, resources_present in result and summary files, with how prompt_sha256 is computed and that missing resources are normal"
      - "docs/evaluation.md states explicitly that --sandbox runs do not honor evals.agent_model (judge_model is still honored)"
      - "Stale statements are corrected: kiro-cli model is no longer always empty, and the self-test fixture listing mentions the model and non-existent conventions resource"
    validation_commands:
      - "grep -q 'agent_model' docs/evaluation.md"
      - "grep -q 'allowed_models' docs/evaluation.md"
      - "grep -q 'prompt_sha256' docs/evaluation.md"
      - "grep -q 'resources_present' docs/evaluation.md"
      - "grep -qi 'sandbox.*agent_model\\|agent_model.*sandbox' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue 294 read-only: run the full build, tests, lint and self-test, run the targeted pinning and provenance tests, confirm both config files and docs/evaluation.md, and confirm template sync shows no new differences beyond the pre-existing .kiro/agents edits."
    dependencies: ["docs-evaluation"]
    acceptance_criteria:
      - "go build ./..., task test and task lint pass"
      - "task eval:selftest succeeds and its result files contain per-call records and the four run-level provenance fields with resources_present omitting the non-existent conventions resource"
      - "Targeted tests prove --model claude-sonnet-5.5 on the agent call, the agent_model override, --model <judge_model> on every judge call, refusal before any case for judge_model auto, and a changed prompt_sha256 after a prompt edit"
      - "Both .kairon/config.yaml and cmd/kairon/templates/kairon/config.yaml document the evals block with the defaults, and docs/evaluation.md covers the evals block, allowlist refusal, provenance fields and the --sandbox limitation"
      - "task sync:check reports no new differences outside .kiro/agents and no agent config model field was changed"
    validation_commands:
      - "go build ./..."
      - "task test"
      - "task lint"
      - "task eval:selftest"
      - "go test ./internal/config/... ./internal/inference/... ./internal/eval/... ./cmd/..."
      - "diff -rq --exclude=results --exclude=.DS_Store --exclude=tmp .kairon/evals/ cmd/kairon/templates/kairon/evals/"
      - "diff -rq .kairon/scripts/ cmd/kairon/templates/kairon/scripts/"
      - "diff -rq .kiro/skills/sentinel-protocol/ cmd/kairon/templates/kiro/skills/sentinel-protocol/"
```
