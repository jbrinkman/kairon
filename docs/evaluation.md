# Evaluation Framework

Kairon's evaluation framework measures agent quality and cost, enabling data-driven prompt improvements.

## Directory Structure

```
.kairon/evals/
  rubrics/           # Scoring criteria per agent
    architect.yaml
    builder.yaml
    documenter.yaml
    krew-lead.yaml
    planner.yaml
    validator.yaml
  cases/             # Test cases per agent
    architect/
      case-1.yaml
    builder/
      case-1.yaml
    documenter/
      case-1.yaml
    krew-lead/
      case-1.yaml
    ...
  results/           # One directory per run: <timestamp>-<git-short-hash>
    <timestamp>-<git-short-hash>/
      architect.json
      builder.json
      documenter.json
      krew-lead.json
      summary.json
```

This is the default evals directory. Use `--evals-dir` to point the harness at a different directory with the same layout (see [Evals Directory](#evals-directory---evals-dir)).

## Rubric Format

Each agent has a rubric YAML file defining scoring criteria:

```yaml
agent: architect
criteria:
  - name: task_decomposition
    description: "Spec breaks work into discrete, independently implementable tasks"
    scoring: 1-5
  - name: file_reference_accuracy
    description: "Referenced files exist and are relevant"
    scoring: 1-5
    deterministic: true    # Scored by code, not LLM
  - name: cost_efficiency
    description: "Token usage relative to output quality"
    type: cost             # Tracked as cost metric
```

Fields:
- `agent` — which agent this rubric evaluates
- `criteria[].name` — unique identifier for the criterion
- `criteria[].description` — what is being measured
- `criteria[].scoring` — score range (e.g. "1-5")
- `criteria[].deterministic` — if true, scored by code checks rather than LLM
- `criteria[].type` — set to "cost" for cost-tracking criteria

## Test Case Format

```yaml
name: simple-feature-issue
description: "Evaluate architect output for a simple feature request"
input: |
  The issue body or spec that the agent receives as input.
output: |
  Optional: pre-captured agent output for offline evaluation.
stub:                      # Optional: scripted response for `--backend stub`
  turns:
    - response: |
        ## Summary
        ### Details
```

Fields:
- `name` — unique identifier
- `description` — what this case tests
- `input` — the input the agent would receive
- `output` — (optional) pre-captured output for offline scoring
- `setup` — (optional) extra prompt context; `type: file` entries read `path` from disk
- `stub.turns[]` — (optional) scripted model responses, used only by the `stub` backend (see [Stub Case Fields](#stub-case-fields))

## Running Evaluations

```bash
# Evaluate all agents
kairon eval

# Evaluate a specific agent
kairon eval architect

# Compare two runs (names of directories under results/)
kairon eval diff <runA> <runB>

# Choose the inference backend (default: kiro-cli)
kairon eval --backend kiro-cli architect
kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest

# Use a different evals directory
kairon eval --evals-dir path/to/evals architect
```

| Flag | Default | Description |
|------|---------|-------------|
| `--backend` | `kiro-cli` | Inference backend for agent and judge calls. Valid values: `kiro-cli`, `stub`. An unknown value fails immediately and lists the valid backends. |
| `--evals-dir` | `.kairon/evals` | Directory holding `rubrics/`, `cases/`, `fixtures/`, optional `agents/`, and `results/`. Persistent flag, so `kairon eval diff` honours it too. |

## Adding Test Cases

1. Create a YAML file in `.kairon/evals/cases/<agent>/`
2. Provide an `input` field with representative agent input
3. Optionally capture real agent output in the `output` field for offline evaluation

## How Scoring Works

- **Deterministic criteria** — scored by code checks (file existence, structural completeness)
- **LLM-judged criteria** — scored by an LLM evaluator using the rubric description (requires output and a configured judge)
- **Cost criteria** — tracked automatically from token usage

### Skipped Criteria

Non-deterministic criteria require an LLM judge to score. When no LLM judge is configured, these criteria are marked as `skipped` in the results and excluded from aggregate score calculations. This prevents false signal — scores only reflect what was actually measured.

Skipped criteria appear in results as:
```json
{
  "name": "task_decomposition",
  "score": 0,
  "max_score": 5,
  "skipped": true,
  "reasoning": "LLM judge not configured — criterion skipped"
}
```

To get full scoring coverage, configure an LLM judge (future feature). Until then, aggregate scores reflect only deterministic criteria.

Results are written to `<evals-dir>/results/<timestamp>-<git-hash>/` (default `.kairon/evals/results/...`) enabling before/after comparison when prompts change.

## Inference Backends

Every "send a prompt to a model, get text back" call made by the harness (the agent under test and the LLM judge) goes through an `inference.Backend` defined in `internal/inference`. The package is stdlib-only and does not import `internal/eval`, so other packages can reuse it without import cycles.

| Backend | Behaviour |
|---------|-----------|
| `kiro-cli` (default) | Shells out to `kiro-cli`. Agent: `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools [--model <model>]`; judge: `kiro-cli chat --no-interactive [--model <model>]`; prompt on stdin. `--model` is appended when the request carries a model, which is always the case in a normal `kairon eval` run (see [Model Pinning and Run Provenance](#model-pinning-and-run-provenance)). Usage is always **estimated**. Requires `kiro-cli` on `PATH`. |
| `stub` | Deterministic and in-process. Never starts a process or touches the network, and does not require `kiro-cli`. The agent's output comes from the case's `stub.turns`; every judge call returns score 5 (the maximum of the judge scale) with `pass: true`. |

```bash
kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
```

Notes:
- The `kiro-cli` startup probe (`kiro-cli --version`) and the PATH availability check are performed by the selected backend; the stub reports zero startup overhead and is always available.
- `--backend stub` cannot be combined with `--sandbox` (the sandbox runs `kiro-cli` inside a container); the run fails with an error.
- The Docker sandbox path still runs `kiro-cli` directly in the container, always reports estimated usage, and does **not** honor `evals.agent_model` (see [`--sandbox` and `evals.agent_model`](#--sandbox-and-evalsagent_model)).

### Stub Case Fields

The stub backend reads its script from the test case:

```yaml
stub:
  turns:
    - response: |          # required: text returned as the agent's output
        ## Self-test
        ### Usage
      model: stub-model    # optional: recorded as the model in results
      usage:               # optional: scripted, "reported" token usage
        input_tokens: 123
        output_tokens: 45
```

| Field | Required | Description |
|-------|----------|-------------|
| `stub.turns[].response` | yes | Text returned as the agent output. |
| `stub.turns[].model` | no | Model name recorded in `agent_cost.model`. |
| `stub.turns[].usage.input_tokens` / `output_tokens` | no | If present, these counts are used verbatim and marked `reported`. If absent, usage is estimated from text length. |

Only `turns[0]` is used today. A case with no `stub`, empty `turns`, or an empty `response` fails with `case has no stub.turns[0].response` rather than silently producing empty output. The `kiro-cli` backend ignores `stub`.

### Reported vs Estimated Usage

Each `agent_cost` and `judge_cost` in a result file carries `model` (when known) and `usage_source`:

| `usage_source` | Meaning |
|----------------|---------|
| `reported` | Token counts were supplied by the backend (for the stub: `stub.turns[].usage`). |
| `estimated` | Token counts were estimated as roughly 4 characters per token (`len/4`) of the prompt and the output. |

- `kiro-cli` exposes neither the served model nor token counts, so its usage is always `estimated`. The `model` it records is the model the process was *launched with* (the value passed as `--model`, i.e. the pinned model), not a model confirmed by the service. It is empty (omitted from JSON) only for unpinned calls, such as code paths that do not go through `kairon eval`.
- `estimated_usd` is always computed from the token counts at a fixed $3 / $15 per million input / output tokens, whether the counts were reported or estimated.
- When several judge calls are accumulated into `judge_cost`, the merged `usage_source` is `reported` only if every contributing call was `reported`; otherwise it is `estimated`. The stub judge always produces estimated usage with model `stub`.
- `summary.json` totals only `tokens_in`, `tokens_out` and `estimated_usd` (plus the provenance fields described below).
- The individual calls behind `agent_cost` and `judge_cost` are listed in each case's `calls` array (see [Per-call records](#per-call-records-calls)).

Example from the self-test `stub-usage` case:

```json
"agent_cost": {
  "tokens_in": 123,
  "tokens_out": 45,
  "estimated_usd": 0.001044,
  "model": "stub-model",
  "usage_source": "reported"
}
```

## Model Pinning and Run Provenance

Every agent call and judge call in a `kairon eval` run is pinned to an explicit model, and every result records which models and which prompt produced it. A run cannot silently fall back to the account default (`auto`), which may be a frontier model.

### The `evals` block

Configure pinning in the `evals` block of `.kairon/config.yaml`. The block is optional: when it is absent, or only partly present, the defaults below apply. Only the `evals` key is read, so eval runs do not need `repo:` to be set, and a missing `.kairon/config.yaml` simply means "all defaults". A config file that cannot be read or parsed refuses the run instead of guessing.

```yaml
evals:
  agent_model: ""                  # optional override for the agent under test
  judge_model: "claude-sonnet-5.5"
  allowed_models:
    - "claude-sonnet-5.5"
    - "claude-sonnet-5"
    - "claude-sonnet-4.6"
    - "claude-sonnet-4.5"
    - "claude-sonnet-4"
    - "claude-haiku-4.5"
```

| Key | Default | Description |
|-----|---------|-------------|
| `evals.agent_model` | empty (not set) | Model for the agent under test. When set, it is used for every agent in the run and **overrides** the `model` in the agent's config. When empty, each agent runs on the `model` from its own agent config (`<evals-dir>/agents/<agent>.json` if present, else `.kiro/agents/<agent>.json`). |
| `evals.judge_model` | `claude-sonnet-5.5` | Model for every LLM-judge call. An explicit empty value stays empty and is refused. |
| `evals.allowed_models` | the six models listed above, in that order | Allowlist. The effective agent model and the judge model must each be exactly one of these entries. |

Notes:
- A user-supplied `allowed_models` list **replaces** the default list; it is not appended to it. Omit the key to keep the defaults.
- All values are whitespace-trimmed, and empty `allowed_models` entries are dropped.
- Use `agent_model` to run on a model that is available on your machine without editing agent configs (for example `agent_model: claude-sonnet-4.5` when `claude-sonnet-5.5` is not available). The override must itself be in `allowed_models`.
- Agent configs are never modified; the model is passed to `kiro-cli` with `--model`.

### Allowlist refusal

Before the first case starts, `kairon eval` resolves the judge model and the effective model of every agent in scope (the named agent, or every agent that has a rubric) and checks each against `allowed_models`. A model is **rejected** when it is:

- empty (for example `judge_model: ""`, or an agent config with no `model` and no `evals.agent_model` override),
- `auto` (case-insensitive), or
- not exactly one of the `allowed_models` entries (including the case where `allowed_models` is empty, which permits nothing).

The check runs in the pre-flight step of the run, so a refused run:

- starts no case and makes no `kiro-cli` call (not even the `--version` startup probe),
- creates no results directory, and
- exits with an error.

It applies to both backends (the stub backend is refused the same way) and to single-test-case and `--resume` runs. `--list` and `--cleanup` are not blocked.

**All violations are reported together**, judge first, then agents, so one run shows everything that needs fixing. Each line names the setting or agent, the rejected value (or says it is empty), where an agent's model came from (`evals.agent_model` or the agent config path), and the full allowlist, followed by a hint to edit the `evals` block:

```
❌ eval model pinning refused the run before any case started:
  - evals.judge_model: model "auto" is not permitted; allowed_models: [claude-sonnet-5.5, claude-sonnet-5, claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4, claude-haiku-4.5]
  - agent "architect" (model from .kiro/agents/architect.json "model"): model is empty; allowed_models: [claude-sonnet-5.5, claude-sonnet-5, claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4, claude-haiku-4.5]
Edit the evals block in .kairon/config.yaml (agent_model, judge_model, allowed_models)
```

An agent whose config cannot be found or parsed, or whose `prompt: file://...` file is missing, is reported in the same list (the error names both config paths that were tried). If `allowed_models` is empty the message says `allowed_models is empty: no model can be used`.

When the pre-flight passes, one line summarises what the run is pinned to:

```
🔒 Models: judge=claude-sonnet-5.5, architect=claude-sonnet-5.5 (prompt sha256 1a2b3c4d…)
```

### `--sandbox` and `evals.agent_model`

**`--sandbox` runs do not honor `evals.agent_model`.** In a sandboxed run the agent is executed inside the container with its own `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools` command, which does not pass `--model`, so the agent runs on the `model` in its agent config. The run prints a warning that `evals.agent_model` is ignored, validates the agent config's model against `allowed_models`, and records that config model as `agent_model`. The container also ignores the `<evals-dir>/agents/` overlay, so provenance is computed from `.kiro/agents/<agent>.json`.

`evals.judge_model` **is still honored** in a sandboxed run: judge calls execute on the host through the inference backend, pinned with `--model <judge_model>`, and are subject to the same allowlist check.

### Recorded provenance

#### Per-call records (`calls`)

Each case in `<agent>.json` carries a `calls` array with one record per agent call and one per judge call, in execution order. The record shape is the shared `inference.CallRecord`:

| Field | Description |
|-------|-------------|
| `role` | `agent` or `judge`. |
| `model` | The model that served the call when the backend reports one, otherwise the pinned (requested) model. `kiro-cli` cannot report the served model, so its records carry the pinned model; the stub records `stub` / `stub-model`. |
| `agent` | Agent name (agent calls only). |
| `criterion` | Rubric criterion being judged (judge calls only). |
| `input_tokens` / `output_tokens` | Token counts for the call. |
| `cost_usd` | Cost of the call, using the same fixed $3 / $15 per million tokens estimate as `agent_cost` / `judge_cost`. |
| `estimated` | `true` unless the usage was `reported` by the backend. |
| `duration_ms` | Call duration in milliseconds: the backend's measurement, or wall-clock time when the backend reports none (the stub). |
| `prompt_sha256` | Hash of the agent's prompt inputs (agent calls only; same value as the file-level `prompt_sha256`, see below). |
| `error` | Set when the call failed. A failed call is still recorded. |

Details:
- `agent_cost` and `judge_cost` are unchanged; `calls` is the per-call breakdown behind them.
- No agent record is written when prompt assembly failed, because no call was made.
- A judge call is recorded even when its output could not be parsed (the tokens were spent).
- In a `--sandbox` run the agent record is built from wall-clock time and the estimated cost, with `estimated: true` and the agent config's model.

The per-call records (field values below are illustrative):

```json
{
  "case_name": "stub-usage",
  "calls": [
    {
      "role": "agent",
      "model": "stub-model",
      "agent": "selftest",
      "input_tokens": 123,
      "output_tokens": 45,
      "cost_usd": 0.001044,
      "estimated": false,
      "duration_ms": 0,
      "prompt_sha256": "9f2c…"
    },
    {
      "role": "judge",
      "model": "stub",
      "criterion": "clarity",
      "input_tokens": 210,
      "output_tokens": 18,
      "cost_usd": 0.0009,
      "estimated": true,
      "duration_ms": 0
    }
  ]
}
```

#### Run-level fields in `<agent>.json`

The per-agent result file always carries:

| Field | Description |
|-------|-------------|
| `agent_model` | The effective model the agent ran on (`evals.agent_model`, else the agent config's `model`; in a sandbox run always the config's `model`). |
| `judge_model` | The model used for judge calls. |
| `prompt_sha256` | Hash of everything that shapes the agent's prompt (see below). |
| `resources_present` | The agent-config `resources` entries that existed when the hash was computed. Always written; an empty list serialises as `[]`. |

```json
{
  "agent": "selftest",
  "git_hash": "a1b2c3d",
  "agent_model": "claude-sonnet-5.5",
  "judge_model": "claude-sonnet-5.5",
  "prompt_sha256": "9f2c…",
  "resources_present": [],
  "cases": [ … ]
}
```

#### Run summary (`summary.json`)

`summary.json` always carries `judge_model` and an `agents` map with `agent_model`, `prompt_sha256` and `resources_present` per agent:

```json
{
  "git_hash": "a1b2c3d",
  "total_cost": { … },
  "agent_scores": { "architect": 0.85, "builder": 0.8 },
  "judge_model": "claude-sonnet-5.5",
  "agents": {
    "architect": { "agent_model": "claude-sonnet-5.5", "prompt_sha256": "…", "resources_present": [] },
    "builder":   { "agent_model": "claude-sonnet-5.5", "prompt_sha256": "…", "resources_present": [] }
  }
}
```

**Single-agent rule.** A run can cover several agents (`kairon eval` with no agent), each with its own model and prompt hash, so one top-level value would be ambiguous. The top-level `agent_model`, `prompt_sha256` and `resources_present` are therefore written **only when the run covers exactly one agent** (`kairon eval <agent>`, a single-case run, or the self-test) and are omitted otherwise. Use the `agents` map, or the per-agent `<agent>.json`, for multi-agent runs. Single-case runs also write a `summary.json`, with the same provenance fields.

#### How `prompt_sha256` is computed

`prompt_sha256` is a lowercase hex SHA-256 (64 characters). It is a hash only; the prompt text itself is not stored. It is computed over an ordered sequence of parts:

1. **config** — the raw bytes of the agent config file. The config is `<evals-dir>/agents/<agent>.json` when it exists, else `.kiro/agents/<agent>.json` relative to the working directory (in a sandbox run only the latter).
2. **prompt** — if the config's `prompt` starts with `file://`, the bytes of that file. A relative path resolves against the config file's directory; absolute paths are allowed. A missing or unreadable prompt file is an error. An inline prompt is already covered by the config bytes.
3. **resource** — for each entry of the config's `resources` array, in config order, the bytes of every existing matching file.

Each part is framed as `<kind>\x00<decimal length>\x00<bytes>` (`kind` is `config`, `prompt` or `resource`), so parts cannot run together ambiguously. File paths are not hashed, so the same contents give the same hash on any machine.

Consequently, editing the prompt file, the config, or any present resource changes the hash, and so does reordering resources. Running the same inputs twice gives an identical hash, which makes a before/after comparison of two runs a check that only the intended thing changed.

Resource handling:
- Only string entries of `resources` are considered; object entries are ignored.
- A `file://` or `skill://` prefix is stripped. Any other scheme is not a local file and counts as not present.
- Relative paths resolve against the **process working directory** (the repository root for a normal run), not against the config file.
- An entry containing `*`, `?` or `[` is expanded as a glob (matches sorted). It counts as present if at least one regular file matches, and every match is hashed.
- **A missing resource is normal, not an error.** Resources such as `skill://.kiro/skills/<agent>-conventions/SKILL.md` are optional per-project overrides and often do not exist. A missing entry is skipped: it does not contribute to the hash and is omitted from `resources_present`. If the file is created later, it appears in `resources_present` and the hash changes.
- `resources_present` lists entries exactly as written in the config (for example `skill://.kiro/skills/sentinel-protocol/SKILL.md`), in config order.

When an `<evals-dir>/agents/` overlay is used, `kiro-cli` runs in a temporary working directory (see the [overlay caveat](#agent-configs-agents-precedence)) and does not load relative resources from the repository. The hash reflects the files the harness can see from the repository root.

### Resume refusal

`kairon eval --resume` refuses to continue into a result file that was written under a different prompt or different models, so one `<agent>.json` never mixes two versions. If the existing file has a `prompt_sha256` and that value, its `agent_model` or its `judge_model` differs from what the resumed run is now pinned to, the run stops with an error:

```
❌ cannot resume: prompt or models changed since the interrupted run of architect (recorded agent_model=… judge_model=… prompt_sha256=…; now agent_model=… judge_model=… prompt_sha256=…)
```

Start a fresh run (without `--resume`) after changing a prompt, a resource or `evals`. A result file written before provenance existed (no `prompt_sha256`) is **refused when it already holds saved cases** — resuming would attribute those scores to the current prompt and models, which were unknown when they were produced; an empty legacy file is allowed. An unchanged resume continues as before.

## Evals Directory (`--evals-dir`)

`--evals-dir` selects the directory the harness reads rubrics, cases and fixtures from and writes results to. It defaults to `.kairon/evals`. The layout is the same as the default one, plus an optional `agents/` directory:

```
<evals-dir>/
  rubrics/<agent>.yaml
  cases/<agent>/*.yaml
  fixtures/...                 # files referenced from setup[].path
  agents/                      # optional: agent configs for this eval set
    <agent>.json
    <prompt>.md                # anything the config references via file://./
  results/<timestamp>-<git-short-hash>/
```

- **Setup file rebasing** — a `setup[].path` beginning with `.kairon/evals/` is rebased onto the chosen evals dir when `--evals-dir` is not the default. For example `.kairon/evals/fixtures/selftest-input.md` resolves to `<evals-dir>/fixtures/selftest-input.md`. Other paths are left untouched.
- **Results** — written under `<evals-dir>/results/`; `--resume` and `eval diff` look there too. The sandbox's `.kairon/evals/tmp/...` debug artefacts stay at their fixed location.

### Agent configs: `agents/` precedence

If `<evals-dir>/agents/<agent>.json` exists, the agent under test is run with that config instead of the repository's `.kiro/agents/<agent>.json`. If it does not exist, nothing changes and the repo's `.kiro/agents` is used. The precedence is chosen per agent, so an evals dir may override only some agents.

**kiro-cli temp-cwd overlay caveat.** `kiro-cli` only discovers local agents under `<cwd>/.kiro/agents/`. To give `<evals-dir>/agents/` precedence without touching the repo, the `kiro-cli` backend copies the whole `agents/` directory (so `file://./x.md` prompts keep resolving) into a temporary `<tmp>/.kiro/agents/`, runs the agent with its working directory set to `<tmp>`, and deletes the temp directory afterwards. Consequences:

- The agent's working directory is a temp directory, **not** the repository, whenever an `agents/` override applies. Tools that read files or run shell commands relative to the cwd will not see the repo.
- Only the agent-under-test call is affected; judge calls always run in the normal working directory.
- Without an override no working directory is set and behaviour is identical to earlier versions.
- The `stub` backend ignores `agents/` because it does not run an agent.

## Self-Test

The harness has a self-test that exercises the full pipeline (rubric and case loading, prompt assembly, agent call, deterministic and judged scoring, cost accounting, result files) with no model calls and no `kiro-cli`:

```bash
task eval:selftest
# equivalent to:
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
```

Fixtures live in `internal/eval/testdata/evals/`:

```
internal/eval/testdata/evals/
  agents/selftest.json              # minimal agent config (model: claude-sonnet-5.5; prompt: file://./selftest-prompt.md;
                                    #   resources: a non-existent selftest-conventions skill, so resources_present is [])
  agents/selftest-prompt.md
  rubrics/selftest.yaml             # structural_completeness (deterministic), clarity (LLM-judged), cost_efficiency (cost)
  cases/selftest/stub-basic.yaml    # no stub usage -> estimated; setup file exercises path rebasing
  cases/selftest/stub-usage.yaml    # stub model + usage 123/45 -> reported
  fixtures/selftest-input.md        # referenced as .kairon/evals/fixtures/selftest-input.md
```

`selftest.json` declares a `model` so the self-test passes the model-pinning pre-flight, and lists a `skill://.kiro/skills/selftest-conventions/SKILL.md` resource that intentionally does not exist. It exercises the "missing resources are normal" rule: the run succeeds and the recorded `resources_present` is `[]`.

The self-test runs with the stub backend, so its per-call `model` values are `stub` / `stub-model` (what the stub reports) rather than the pinned names. The pinned models still appear in the run-level `agent_model` and `judge_model` fields.

To list the self-test cases: `kairon eval --evals-dir internal/eval/testdata/evals --list selftest`.

Results go to `internal/eval/testdata/evals/results/`, which is git-ignored (`.gitignore` entry `internal/eval/testdata/evals/results/`), so running the self-test leaves the working tree clean. These fixtures live under `testdata`, outside the template-synced `.kairon/evals/`, so they do not affect `task sync:check`.

## Adding a New Backend

1. Add a file in `internal/inference` (for example `mybackend.go`) with a type implementing `inference.Backend`:
   - `Name() string` — the registry name, also the `--backend` value.
   - `Available() error` — nil when the backend can be used in this environment.
   - `StartupProbe() time.Duration` — start-up overhead, or `0` if there is none.
   - `Invoke(ctx, Request) (Response, error)` — handle `RoleAgent` and `RoleJudge`.
2. Add a name constant and register a constructor in the `registry` map in `internal/inference/inference.go`. `inference.Names()` feeds both the `--backend` help text and the unknown-backend error, so no CLI change is needed.
3. Follow the contract:
   - Fill `Response.Usage` with `Source` set to `inference.UsageReported` when the backend supplies real token counts (for the stub, from the case's `stub.turns[].usage`); otherwise use `inference.EstimateUsage`.
   - Set `Response.Model` when known.
   - Populate `Command`, `Stderr`, `ExitCode` and `Duration` even when returning an error, since the harness builds `error_context` from them.
   - Wrap `inference.ErrTimeout` on timeouts so `errors.Is(err, inference.ErrTimeout)` holds.
   - A judge response must contain `===JSON_START===` ... `===JSON_END===` with `{"score": <1-5>, "reasoning": "...", "pass": <bool>}`.
4. Keep the package stdlib-only and do not import `internal/eval`.
5. Add unit tests next to it (see `stub_test.go` and `kirocli_test.go`), and verify with `go test ./internal/inference/...`.

## Agent Coverage

All six shipped agents have rubrics and test cases:

| Agent | Rubric | Key Criteria |
|-------|--------|--------------|
| `architect` | `rubrics/architect.yaml` | task_decomposition, acceptance_criteria_testability, file_reference_accuracy, completeness |
| `builder` | `rubrics/builder.yaml` | code_correctness, spec_adherence, code_quality, test_coverage |
| `documenter` | `rubrics/documenter.yaml` | documentation_completeness, accuracy, file_naming_convention, practical_usage_guidance |
| `krew-lead` | `rubrics/krew-lead.yaml` | workflow_adherence, delegation_quality, retry_policy_compliance, error_handling |
| `planner` | `rubrics/planner.yaml` | requirement_clarity, scope_appropriateness, acceptance_criteria_quality, constraint_identification |
| `validator` | `rubrics/validator.yaml` | issue_coverage, test_execution, defect_detection, actionable_feedback |

## Evaluation Workflow

The evaluation framework serves as unit testing for prompt engineering. Follow this workflow when modifying agent prompts or configurations:

### Before Making Changes (Baseline)

**Required**: Run `kairon eval` before making any prompt changes to establish a baseline:

```bash
# Capture current performance
kairon eval
```

This creates a results snapshot at `.kairon/evals/results/<timestamp>-<git-hash>/` for comparison.

### After Making Changes (Verification)

**Required**: Run `kairon eval` after prompt changes to verify improvements:

```bash
# Test modified behavior
kairon eval

# Compare with baseline
kairon eval diff <baseline-hash> <current-hash>
```

### Creating Test Cases for Behavioral Changes

When making specific behavioral changes, create targeted test cases:

1. **Identify the behavior** — What specific agent behavior are you changing?
2. **Create test case** — Add a case in `.kairon/evals/cases/<agent>/` that exercises this behavior
3. **Verify coverage** — Ensure existing rubric criteria measure the desired change
4. **Test iteratively** — Run evaluations as you refine the prompt

Example workflow for improving architect task decomposition:
```bash
# 1. Baseline
kairon eval architect

# 2. Add test case for complex decomposition scenario
# Edit .kairon/evals/cases/architect/complex-decomposition.yaml

# 3. Modify architect prompt
# Edit .kairon/agents/architect-prompt.md

# 4. Verify improvement
kairon eval architect
kairon eval diff <baseline> <current>
```

### Evaluation as Unit Testing

Treat evaluations like unit tests:
- **Red-Green-Refactor**: Baseline (red) → Change (green) → Optimize (refactor)
- **Regression prevention**: Catch unintended behavior changes
- **Performance tracking**: Monitor cost and quality over time
- **Documentation**: Results serve as behavioral specifications

## Container Sandboxing

The evaluation framework includes container sandboxing for secure, isolated agent testing using Docker.

### Using the --sandbox Flag

Run agent evaluations in Docker containers for complete isolation:

```bash
# Run all agents in sandbox containers
kairon eval --sandbox

# Run specific agent in sandbox
kairon eval --sandbox architect

# List available agents (detects project type)
kairon eval --sandbox --list architect
```

The `--sandbox` flag automatically:
- Detects project type (Go, Node.js, Python, Rust, Java)
- Generates appropriate Dockerfile with required toolchains
- Creates isolated container with resource limits
- Mocks GitHub CLI operations
- Copies project files and runs evaluations safely

> **Model pinning in sandbox runs:** `--sandbox` does **not** honor `evals.agent_model`; sandboxed agent calls run on the `model` in the agent config. `evals.judge_model` is still honored for judge calls. See [`--sandbox` and `evals.agent_model`](#--sandbox-and-evalsagent_model).

### Project Detection

The sandbox automatically detects project types and installs required toolchains:

| Project Type | Detection Files | Toolchain Installed |
|--------------|-----------------|-------------------|
| Go | `go.mod`, `go.sum` | Go compiler and tools |
| Node.js | `package.json` | Node.js and npm |
| Python | `requirements.txt`, `pyproject.toml` | Python and pip |
| Rust | `Cargo.toml` | Rust and Cargo |
| Java | `pom.xml`, `build.gradle` | OpenJDK and Maven/Gradle |
| Task | `Taskfile.yml` | Task runner |

Multi-language projects are supported - all detected toolchains will be installed.

### Resource Limits

Containers run with strict resource limits to prevent runaway processes:

| Resource | Default Limit | Environment Variable |
|----------|---------------|----------------------|
| CPU | 1.0 core (1,000,000 μs) | `KAIRON_EVAL_CPU_QUOTA` |
| Memory | 512MB | `KAIRON_EVAL_MEMORY_LIMIT` |
| Timeout | 5 minutes | `KAIRON_EVAL_TIMEOUT` |
| Network | Disabled | N/A |

Configure resource limits via environment variables:

```bash
# Restrict to 0.5 CPU cores and 256MB memory
KAIRON_EVAL_CPU_QUOTA=500000 \
KAIRON_EVAL_MEMORY_LIMIT=268435456 \
kairon eval --sandbox architect

# Set 30-second timeout for quick tests
KAIRON_EVAL_TIMEOUT=30s \
kairon eval --sandbox builder
```

### GitHub CLI Mocking

The sandbox includes a mocked GitHub CLI (`gh`) that returns realistic responses without making real API calls:

```bash
# Mocked commands return test data:
gh auth status          # ✓ Logged in as sandbox-user (mocked)
gh issue create         # Returns mock issue URL
gh pr create           # Returns mock PR URL
gh issue list          # Returns mock issue JSON
```

This enables testing GitHub-dependent workflows safely without:
- Making real API requests
- Requiring authentication
- Creating test repositories
- Rate limiting issues

### Dynamic Dockerfile Generation

Containers use dynamically generated Dockerfiles based on detected project types:

```dockerfile
FROM alpine:3.19

# Install essential tools
RUN apk add --no-cache \
    git \
    curl \
    bash \
    ca-certificates

# Install detected toolchains (example: Go + Node.js project)
# Install Go
RUN apk add --no-cache go
ENV GOPATH=/home/sandbox/go
ENV PATH=$PATH:/usr/local/go/bin:$GOPATH/bin

# Install Node.js
RUN apk add --no-cache nodejs npm
ENV NODE_PATH=/usr/lib/node_modules

# Setup sandbox user and workspace
RUN adduser -D -s /bin/bash sandbox
WORKDIR /workspace
USER sandbox
CMD ["/bin/bash"]
```

### Container Lifecycle

Each evaluation follows this container lifecycle:

1. **Detection** - Analyze project files to determine required toolchains
2. **Generation** - Create Dockerfile with appropriate base image and tools
3. **Build** - Build Docker image with generated Dockerfile
4. **Create** - Create container with resource limits and security settings
5. **Copy** - Copy project files and mock GitHub CLI into container
6. **Execute** - Run agent evaluation inside container
7. **Cleanup** - Stop and remove container, clean up temporary files

### Troubleshooting Container Issues

**Docker not running:**
```bash
# Ensure Docker daemon is running
sudo systemctl start docker   # Linux
open -a Docker               # macOS
```

**Permission denied:**
```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
newgrp docker
```

**Out of memory:**
```bash
# Check container resource usage
docker stats

# Increase memory limit
KAIRON_EVAL_MEMORY_LIMIT=1073741824 kairon eval --sandbox
```

**Timeout errors:**
```bash
# Increase timeout for complex evaluations
KAIRON_EVAL_TIMEOUT=10m kairon eval --sandbox
```

**Build failures:**
```bash
# Check Docker logs for build issues
docker logs <container-id>

# Verify project detection
kairon eval --sandbox --list
```

**Network connectivity (for debugging only):**
The sandbox disables network access by default. To enable for debugging:
```bash
# ⚠️ Only for debugging - reduces security
KAIRON_EVAL_NETWORK_MODE=bridge kairon eval --sandbox
```

### Security Considerations

Container sandboxing provides multiple security layers:

- **Process isolation** - Containers run in separate namespaces
- **Resource limits** - CPU and memory usage restricted
- **Network isolation** - No external network access by default
- **User isolation** - Runs as non-root `sandbox` user
- **GitHub mocking** - No real API calls or authentication required
- **Temporary containers** - Automatically cleaned up after evaluation

## Comparing Runs

The `eval diff` command shows:
- Per-criterion score deltas per agent
- Token and cost deltas
- Quality-per-dollar assessment
