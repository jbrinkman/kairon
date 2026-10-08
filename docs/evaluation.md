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
  fixtures/          # Files referenced by cases
    workspaces/      # Optional per-case workspace fixtures: workspaces/<name>/ (see Case Workspaces)
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
workspace: seeded         # Optional: workspace fixture name (see Case Workspaces)
timeout: 30s               # Optional: per-case timeout (Go duration)
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
- `workspace` — (optional) name of a fixture under `<evals-dir>/fixtures/workspaces/` that the case's workspace starts from (see [Case Workspaces](#case-workspaces)). Must match `^[A-Za-z0-9._-]+$` (and not be `.` or `..`) and the fixture directory must exist, otherwise loading the cases fails with an error naming the case.
- `timeout` — (optional) a positive Go duration such as `30s` or `2m`. It overrides the default timeout for this case, natively and under `--sandbox` (see [Case Timeout](#case-timeout)). An invalid or non-positive value is a load error naming the case.
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
| `--keep-workspaces` | off | Keep each case's workspace after the run instead of deleting it. The path is printed on the case line and recorded as `workspace_dir` in the results (see [Case Workspaces](#case-workspaces)). Combine with `--debug` to inspect both the preserved container and its workspace. |

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
- Every backend can run under `--sandbox`, including `--backend stub`. The container is only a transport: the backend chosen with `--backend` runs inside it, the prompt is delivered on stdin, and the request, cost accounting and call record are the same as a native run. See [Backends in the Container](#backends-in-the-container).
- Under `--sandbox` the agent model is pinned and passed exactly as in a native run: `evals.agent_model` is honoured and reaches `kiro-cli` as `--model` (see [`--sandbox` and `evals.agent_model`](#--sandbox-and-evalsagent_model)).

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
      commands:            # optional: shell commands run in the case workspace before responding
        - "echo hi > marker.txt"
```

| Field | Required | Description |
|-------|----------|-------------|
| `stub.turns[].response` | yes | Text returned as the agent output. |
| `stub.turns[].model` | no | Model name recorded in `agent_cost.model`. |
| `stub.turns[].usage.input_tokens` / `output_tokens` | no | If present, these counts are used verbatim and marked `reported`. If absent, usage is estimated from text length. |
| `stub.turns[].commands` | no | Shell commands, each run with `sh -c` in the case workspace (the request's `WorkDir`), in order, before the response is returned. This lets a stub case simulate an agent that edits files. |

Only `turns[0]` is used today. A case with no `stub`, empty `turns`, or an empty `response` fails with `case has no stub.turns[0].response` rather than silently producing empty output. The `kiro-cli` backend ignores `stub`.

How `commands` behave:
- They run in the case workspace: natively the host workspace directory, under `--sandbox` the container's workspace path, as the `sandbox` user. Commands with no workspace directory are an error and run nothing, so a test cannot write into the repository root by accident.
- A command that exits non-zero fails the call with an error carrying the command and its stderr; the scripted response is not returned and later commands do not run.
- All commands of a turn share one deadline: the request timeout (the case [`timeout`](#case-timeout), else the default). On expiry the command's whole process group is killed and the call fails with a timeout error (`stub timeout after <duration>`) that wraps `inference.ErrTimeout`, so it is recorded exactly like a `kiro-cli` timeout.

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
- Agent configs are never modified; the model is passed to `kiro-cli` with `--model`, both natively and under `--sandbox`.

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

**`--sandbox` honours `evals.agent_model`.** A sandboxed run builds the same agent request as a native run, so the pinned model is sent with the call. With the `kiro-cli` backend the container runs `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools --model <model>`, with the same arguments as the native backend. The effective model is the same as without `--sandbox`: `evals.agent_model` when set, otherwise the `model` in the agent config. It is validated against `allowed_models` in the pre-flight, recorded as `agent_model`, and recorded on the agent call. No warning is printed.

`evals.judge_model` is honoured in the same way: judge calls execute on the host through the inference backend, pinned with `--model <judge_model>`, and are subject to the same allowlist check.

The `<evals-dir>/agents/` overlay is staged into the case workspace's `.kiro/agents/` on the host and reaches the container through the read-only `.kiro` mount (see [Staged `.kiro`](#staged-kiro-and-precedence)), so the agent itself can see it. The model-pinning provenance for `--backend kiro-cli --sandbox` is nevertheless computed from `.kiro/agents/<agent>.json` only (`prompt_sha256`, `resources_present`, and the config model used when `evals.agent_model` is unset); that part of the pre-flight is unchanged. With `--backend stub --sandbox` no agent config is read inside the container, so the overlay applies exactly as in a native run; this is what lets the self-test, whose agent config lives only in `internal/eval/testdata/evals/agents/`, pass the pre-flight under `--sandbox`.

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
- A judge call is recorded even when its output could not be parsed (the tokens were spent). Its cost appears in that call's `calls[]` record but **not** in the case's `judge_cost`, which keeps a zero cost for a failed or unparseable judge call — so for such a case the sum of `calls[].cost_usd` can exceed `judge_cost`.
- A `--sandbox` run builds its agent record exactly like a native one, from the same request and the same completion logic: the cost comes from the backend's reported or estimated usage, `model` is the served model when the backend reports one (the stub) or the pinned model (`kiro-cli`), and `estimated` is `true` unless the usage was `reported`. For the same case the sandboxed and native `output`, `agent_cost` and call record match.

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
| `agent_model` | The effective model the agent ran on (`evals.agent_model`, else the agent config's `model`), the same with and without `--sandbox`. |
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

1. **config** — the raw bytes of the agent config file. The config is `<evals-dir>/agents/<agent>.json` when it exists, else `.kiro/agents/<agent>.json` relative to the working directory (in a `kiro-cli` sandbox run only the latter).
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

When an `<evals-dir>/agents/` overlay is used, the agent runs in its case workspace, whose `.kiro/` is staged by the harness (see [Case Workspaces](#case-workspaces)), and does not load relative resources from the repository root. The hash reflects the files the harness can see from the repository root.

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

**How the overlay reaches the agent.** `kiro-cli` only discovers local agents under `<cwd>/.kiro/agents/`. Every case runs in its own workspace (see [Case Workspaces](#case-workspaces)), and the harness stages that workspace's `.kiro/` on the host with the precedence *workspace fixture > `<evals-dir>/agents/` > project `.kiro/`*. The whole `agents/` directory is copied, so `file://./x.md` prompts keep resolving. Consequences:

- The agent's working directory is the case workspace, **not** the repository root. Tools that read files or run shell commands relative to the cwd see the workspace (the project's `.kiro/` plus whatever the case's fixture provides), not the repo. The repository's own `.kiro/` is only read, never modified.
- Only the agent-under-test call is affected; judge calls always run in the normal working directory.
- The `stub` backend does not run an agent, so it ignores `agents/` (it still runs `stub.turns[].commands` in the workspace).
- The temporary-directory overlay in the `kiro-cli` backend (copy `agents/` to `<tmp>/.kiro/agents/` and run there) is now only used by callers that provide no workspace directory; `kairon eval` always provides one.

## Case Workspaces

Every case runs in its own **workspace**, built on the host by the harness. The native and `--sandbox` paths share the same builder, so a case sees the same layout either way. Nothing runs in the repository root, and the live repository is never modified by a case.

A workspace is a temporary directory laid out like this:

```
<workspace>/            # a git repo; its single commit is the fixture
  <fixture files>       # contents of fixtures/workspaces/<name>/ (empty by default)
  .kiro/                # staged agent and skill configuration (harness-owned)
  .eval/                # outputs directory (harness-owned, created empty)
```

How it is built, in order:

1. A private (`0700`) parent directory is created under `$KAIRON_EVAL_WORKSPACE_ROOT` or, when unset, the OS temp directory, and symlinks in the path are resolved (macOS `/var` is really `/private/var`, and Docker Desktop and Podman machine only share real paths). Set `KAIRON_EVAL_WORKSPACE_ROOT` when your `TMPDIR` is not on a path your container runtime can mount.
2. The case's fixture, `<evals-dir>/fixtures/workspaces/<name>/`, is copied in (regular files and directories only; symlinks and `.git` are skipped). A case without `workspace:` gets an empty workspace.
3. `git init -b main` and a single commit of the fixture (`--allow-empty` for the default workspace). The commit is hermetic: fixed identity and date, no hooks, no signing, and no user or system git config. **That commit is the only commit and its tree is the fixture**, so `git status --porcelain` afterwards shows exactly what the agent (or the stub's `commands`) changed.
4. `.eval/` and `.kiro/` are added to `.git/info/exclude` (not to a tracked file), so harness-owned paths never appear in `git status`. Files the fixture itself tracks under `.kiro/` stay tracked.
5. `.kiro/` is staged (below) and an empty `.eval/` is created.
6. Permissions are opened so the unprivileged container user can use the tree whatever its host owner: `a+rwX` on everything including `.git`, and `a+rX` (read-only) on `.kiro/`.

### Staged `.kiro` and precedence

The workspace's `.kiro/` is assembled on the host, never copied into a running container. Precedence, highest first:

1. a file provided by the workspace fixture itself,
2. `<evals-dir>/agents/*` (copied to `.kiro/agents/`),
3. the project's own `.kiro/` (agents, skills, MCP config, as set up by `kairon init`).

The project's `.kiro/` is only read. The live repository's `.kiro/` is never exposed to the agent, and a fixture can override any file (for example a `*-conventions` skill).

### Workspace fixtures

Fixtures live in `<evals-dir>/fixtures/workspaces/<name>/` and are referenced by a case's `workspace:` field:

```yaml
name: stub-seeded-workspace
agent: selftest
workspace: seeded            # => fixtures/workspaces/seeded/
input: |
  Append a line to the seeded README and report that you did.
stub:
  turns:
    - commands:
        - "echo 'appended by the stub' >> README.md"
      response: |
        ## Seeded workspace
        ### Details
```

The name must match `^[A-Za-z0-9._-]+$` and not be `.` or `..`, and the directory must exist. Both are checked when cases are loaded. After this case runs, `git status --porcelain` in the kept workspace is ` M README.md`.

Fixtures under `fixtures/workspaces/` and `fixtures/hidden/` are excluded from the `task sync:check` comparison of `.kairon/evals` against the shipped templates.

### Keeping workspaces and `workspace_dir`

By default a workspace is deleted once its case has been scored. With `--keep-workspaces` it is kept so you can inspect the result (`git -C <workspace> status`, `ls <workspace>/.eval`). Either way, each case in `<agent>.json` records the path:

```json
{
  "case_name": "stub-marker",
  "workspace_dir": "/private/var/folders/…/kairon-eval-ws-123456789/ws",
  …
}
```

`workspace_dir` is omitted when empty (for example when workspace creation failed, which is recorded as a failed case) and older result files without it still load. The path is set before the case is scored and the workspace is removed only after scoring, so scoring reads the host workspace: its file tree, `git status` and `.eval/` contents. Without `--keep-workspaces` the recorded directory no longer exists once the run is over. Removal is best effort: a failure prints a warning with the path and never fails the run.

The single-case (`kairon eval <agent> <case>`) and `--resume` paths go through the same code, so `workspace_dir` and `--keep-workspaces` behave identically there.

> The `file_exists` and `changed_files` check types are **not** part of this change. Workspace scoring today means the data those checks will read; when they land they read `workspace_dir` and need no container-specific code.

### Outputs: `.eval/`

`.eval/` is the outputs directory. Under `--sandbox` it is bind-mounted read-write into the container, but it physically lives inside the host workspace, so whatever the process writes there is on the host the moment it is written. No "copy outputs out" step exists, and the layout is identical to a native run.

### Case Timeout

A case's `timeout` takes precedence over the default for that case:

| Where | Without `timeout` | With `timeout` |
|-------|-------------------|----------------|
| Native | `KAIRON_EVAL_TIMEOUT`, else 2 minutes | the case `timeout` |
| `--sandbox` | the sandbox limit (`--resource-limit timeout=`, `sandbox.timeout`; default 5 minutes) | the case `timeout` |

The same value is sent to the backend as the request timeout, so a host-side and an in-container deadline agree. For the `kiro-cli` backend the container exec deadline equals the timeout. For helper-based backends such as `stub` the host deadline is the timeout plus 10 seconds, so the in-container backend normally reports the timeout itself and the host deadline is only a backstop. A timeout is recorded as an empty `actual_output`, an `error_context.stderr` beginning `timeout after <duration>` (the effective duration, for example `1s`, not the sandbox default), a call record with an `error`, and a failing score. The container error message names both the case `timeout` and `--resource-limit timeout=`.

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
  agents/selftest-fail.json         # separate agent whose case is expected to FAIL (see below)
  agents/selftest-fail-prompt.md
  rubrics/selftest.yaml             # structural_completeness (deterministic), clarity (LLM-judged), cost_efficiency (cost)
  rubrics/selftest-fail.yaml
  cases/selftest/stub-basic.yaml    # no stub usage -> estimated; setup file exercises path rebasing
  cases/selftest/stub-usage.yaml    # stub model + usage 123/45 -> reported
  cases/selftest/stub-quoted-input.yaml  # input with quotes, newlines, $(...) and backticks; must reach the backend verbatim
  cases/selftest/stub-marker.yaml   # stub turn command 'echo hi > marker.txt' leaves marker.txt in the case workspace
  cases/selftest/stub-seeded-workspace.yaml  # workspace: seeded; the stub appends to README.md -> ' M README.md'
  cases/selftest-fail/stub-timeout.yaml      # timeout: 1s, stub turn command 'sleep 3' -> timeout failure
  fixtures/selftest-input.md        # referenced as .kairon/evals/fixtures/selftest-input.md
  fixtures/workspaces/seeded/       # README.md plus docs/notes.txt: the seeded workspace fixture
```

`selftest.json` declares a `model` so the self-test passes the model-pinning pre-flight, and lists a `skill://.kiro/skills/selftest-conventions/SKILL.md` resource that intentionally does not exist. It exercises the "missing resources are normal" rule: the run succeeds and the recorded `resources_present` is `[]`.

The `selftest` agent has five cases and all of them pass (`task eval:selftest`). The `selftest-fail` agent is deliberately separate, so `selftest` keeps meaning "everything passes". Its one case, `stub-timeout`, is **expected to fail**: it records a timeout failure (empty `actual_output`, `error_context.stderr` containing `timeout after 1s`) in about a second. Run it with:

```bash
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail
```

The run itself exits 0; the failure is the recorded result for that case.

The self-test runs with the stub backend, so its per-call `model` values are `stub` / `stub-model` (what the stub reports) rather than the pinned names. The pinned models still appear in the run-level `agent_model` and `judge_model` fields.

To list the self-test cases: `kairon eval --evals-dir internal/eval/testdata/evals --list selftest`.

Results go to `internal/eval/testdata/evals/results/`, which is git-ignored (`.gitignore` entry `internal/eval/testdata/evals/results/`), so running the self-test leaves the working tree clean. These fixtures live under `testdata`, outside the template-synced `.kairon/evals/`, so they do not affect `task sync:check`.

### Self-Test in the Container Sandbox

The same self-test can run hermetically inside a container sandbox. It needs a Podman or Docker daemon and network access for the image build (see [Backends in the Container](#backends-in-the-container)):

```bash
task eval:selftest:sandbox
# runs the daemon-gated tests with KAIRON_EVAL_SANDBOX_SELFTEST=1:
#   internal/eval/sandbox: TestBaseImage_ToolsOnlyNoMounts, TestEnsureBaseImage_ReusesExistingImage
#   internal/eval:         TestSelftestSandbox, TestSandboxWorkspace

# or run the sandboxed self-test directly:
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest
```

`TestSelftestSandbox` runs `selftest` natively and then again with `--sandbox --backend stub`, and requires the sandboxed run to match the native one for every case: non-empty and identical `actual_output`, identical `agent_cost`, and the same model on the recorded agent call. It also applies the same self-test expectations to the sandboxed results. The `stub-quoted-input` case checks that shell metacharacters in the input arrive intact.

The other gated tests cover the sandbox layering:
- `TestBaseImage_ToolsOnlyNoMounts` starts a container from the base image with no mounts and checks it runs as `sandbox` (uid 1000), has `kiro-cli`, `gh`, `git` and `sh` on `PATH`, and has an empty `/workspace` with no `/workspace/.kiro` and no project or agent content anywhere.
- `TestEnsureBaseImage_ReusesExistingImage` calls `EnsureBaseImage` twice and requires the second call to report no build with the same tag and image ID.
- `TestSandboxWorkspace` runs `stub-marker`, `stub-seeded-workspace` and the `selftest-fail` timeout case both natively and under `--sandbox` with kept workspaces and compares them: no `Permission denied` anywhere, `marker.txt` containing `hi` on the host, identical host trees (excluding `.git` and `.kiro`) and `git status --porcelain`, `.eval/` present in both, an unchanged repository-root `git status`, keep/no-keep behaviour of `workspace_dir`, the sandboxed timeout recorded as `timeout after 1s`, and base-image reuse after editing an agent config and a case.

A plain `go test ./...` also runs `TestSelfTestWorkspaceCasesNative`, which needs no daemon: it runs the same new cases natively and checks the workspace behaviour above.

Skip behaviour (the test never builds an image by accident, so `task test` and `go test ./...` are unaffected):

- **No container daemon.** When neither Podman nor Docker is reachable the tests are skipped, not failed, with a message such as `no container daemon reachable (tried Podman and Docker); start Podman or Docker to run the sandbox self-test`. `task eval:selftest:sandbox` then exits 0.
- **Gate not set.** Without `KAIRON_EVAL_SANDBOX_SELFTEST=1` (for example a plain `go test ./internal/eval`) the tests are skipped with `sandbox self-test is opt-in: set KAIRON_EVAL_SANDBOX_SELFTEST=1 or run task eval:selftest:sandbox (needs Podman or Docker)` and no container is started.

The direct `go run ... --sandbox` command has no skip: without a reachable daemon it fails up front with the daemon-unavailable error.

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

The evaluation framework includes container sandboxing for isolated agent testing, using Podman or Docker.

### Using the --sandbox Flag

Run agent evaluations in containers for isolation:

```bash
# Run all agents in sandbox containers
kairon eval --sandbox

# Run specific agent in sandbox
kairon eval --sandbox architect

# List available test cases for an agent
kairon eval --sandbox --list architect

# Keep each case's host-side workspace for inspection
kairon eval --sandbox --keep-workspaces architect
```

The `--sandbox` flag:
- Obtains one cached, **tools-only base image** (built on first use, reused afterwards; see [Sandbox Layering](#sandbox-layering)).
- Builds each case's workspace on the host (see [Case Workspaces](#case-workspaces)) and bind-mounts it into a fresh container with resource limits and no network.
- Runs the selected backend inside the container as the unprivileged `sandbox` user.

Nothing is copied into a running container: no project files, no `.kiro`, no helper binary, no mocked tools.

> **Model pinning in sandbox runs:** `--sandbox` honours `evals.agent_model` (it is passed to `kiro-cli` as `--model`, exactly as in a native run) and `evals.judge_model` for judge calls. See [`--sandbox` and `evals.agent_model`](#--sandbox-and-evalsagent_model).

### Sandbox Layering

The sandbox is split strictly by stability. Stable tools are baked once into an image at build time. Everything that varies per project or per run is supplied from the host through bind mounts.

```
build time, as root, cached by content hash        run time, host-side only, mounted
┌───────────────────────────────────────┐     ┌──────────────────────────────────────────────┐
│ kairon-eval-base:<platform>-<hash>    │     │ <ws>/.kiro   ro  staged project .kiro        │
│  alpine, git, bash/sh, ca-certs,      │  +  │ <ws>         rw  git repo, fixture commit    │
│  gh (pinned), kiro-cli (pinned),      │     │ <ws>/.eval   rw  outputs (inside <ws>)       │
│  user sandbox (uid 1000), /workspace  │     │ /opt/kairon/kairon ro  helper (non-kiro-cli) │
└───────────────────────────────────────┘     └──────────────────────────────────────────────┘
```

#### Base image contents

The image is defined by `internal/eval/dockerfile/base.Dockerfile` (embedded in the binary). It contains:

- Alpine 3.19 with `git`, `bash` and CA certificates (a POSIX `sh` is always present),
- `kiro-cli` at a pinned version, in `/usr/local/bin`,
- `gh` (GitHub CLI) at a pinned version, in `/usr/local/bin`,
- the unprivileged user `sandbox` (uid 1000) and an empty `/workspace` it owns,
- a build-time smoke test (`kiro-cli --version && gh --version && git --version`).

Deliberately **absent**: project toolchains (Go, Node.js, Python, Rust, Java, Task), any `COPY`/`ADD` of project content, `.kiro`, agents, skills, cases and the `kairon` binary. The sandbox no longer detects the project type or installs toolchains; if your evals need a toolchain, add it to the image (see [bumping a baked tool](#when-the-base-image-is-rebuilt)).

`gh` is present but **unconfigured**: it is unauthenticated, and with the container network disabled it cannot reach GitHub. Faking `gh` behaviour and a network policy are separate work (the containment issue, #298), as are a read-only root filesystem and tool trust. `ContainerConfig.MockGitHub` is no longer consulted. `kiro-cli` authentication inside the container is also not provided: a real `kiro-cli` sandbox run still needs credentials supplied through the container environment.

#### Mounts

Every container gets exactly these mounts (all are `bind` mounts of host paths that must exist; a missing path is an error, not an auto-created root-owned directory):

| Host | Container | Mode |
|------|-----------|------|
| `<workspace>/.kiro` | `<workspace_dir>/.kiro` | read-only |
| `<workspace>` | `<workspace_dir>` | read-write |
| `<workspace>/.eval` | `<workspace_dir>/.eval` | read-write |
| the linux `kairon` helper (non-`kiro-cli` backends only) | `/opt/kairon/kairon` | read-only |

`<workspace_dir>` is `sandbox.workspace_dir` from `.kairon/config.yaml` (default `/workspace`). There is no `tmpfs` at the workspace path. The container runs as `sandbox` with `WorkingDir` set to the workspace, `HOME=/home/sandbox`, and a git `safe.directory=*` setting passed as environment (`GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_0`/`GIT_CONFIG_VALUE_0`), because the mounted repo is owned by a different uid than `sandbox` and git would otherwise refuse it as "dubious ownership".

#### Ownership and cleanup

Mounted directories are owned by the host user while the container runs as uid 1000. To make that work on rootful Docker, rootless Podman and the Docker Desktop / Podman machine VMs on macOS:

- The host opens the workspace to everyone (`a+rwX`) before the container starts. The temporary parent directory is `0700`, so other local users cannot traverse into it.
- Every command the harness executes in the container is wrapped as `sh -c 'umask 000; exec "$@"' kairon-exec <command…>`, so files the agent creates are world-accessible and the host user can read them for scoring and delete them afterwards, even when the container uid maps to a foreign host uid. The wrapper does not change the command's arguments, and the prompt is still delivered on stdin only.
- Removing the workspace never fails a run (a warning with the path is printed instead).

While a run is in flight the workspace contents are world-writable.

#### Platform notes

- **macOS (Docker Desktop, Podman machine):** the workspace path must be shared with the VM. The harness resolves symlinks (`/var` → `/private/var`), but if your `TMPDIR` is not a shared mount, set `KAIRON_EVAL_WORKSPACE_ROOT` to a directory under your home directory.
- **Rootless Podman:** supported through the ownership rules above; the open umask wrapper is what normally prevents files that the host cannot delete.
- **SELinux:** when the host reports SELinux as enforcing (`/sys/fs/selinux/enforce` is `1`), the container is created with `label=disable` so the user's directories are not relabeled with `:z`/`:Z`. This is only done on enforcing hosts and may be revisited by the containment work.

#### When the base image is rebuilt

The image tag is `kairon-eval-base:<platform>-<12 hex>` (platform `/` becomes `-`), where the hash covers **only** the Dockerfile bytes, the platform and the tool pins (`sandbox.ToolSet`). It does not depend on the evals directory, agents, skills, cases, the working directory or the environment.

| Change | Result |
|--------|--------|
| Edit an agent config, a prompt, a skill, a rubric or a case; add a fixture; change `.kiro` | Image **reused** (`✅ Base image reused: <tag>`) |
| Change `base.Dockerfile`, a tool pin, or the platform | New tag, **rebuilt once** (`🔨 Base image built: <tag>`) |

The image is persistent: it is not removed at the end of a run, and the old per-run evaluation images are gone. A bumped pin yields a new tag, and the previous image stays until you remove it yourself (for example with `docker image prune` or `podman image prune`).

To bump a baked tool, change the version in `DefaultToolSet` in `internal/eval/sandbox/baseimage.go` (`KiroCLIVersion`, `GHVersion`). Both pins are versioned download URLs (the kiro-cli zip at `…/<version>/kirocli-<arch>-linux-musl.zip`, `gh` from the GitHub release `v<version>`), so the pin names a real, immutable artifact and nothing resolves to `latest`. To add another tool or toolchain, edit `base.Dockerfile`; either way the next run builds once.

Building the image needs network access and a container daemon, even for `--backend stub`. Container *execution* has no network; `NetworkMode: none` applies to containers, not to the build.

### Backends in the Container

The container is a transport: whichever backend `--backend` selects runs inside it, using the same request and the same cost and call-record logic as a native run. The prompt is always delivered on **stdin**; it is never placed on the command line, so argument-length limits and shell quoting cannot affect it (quotes, newlines, `$(...)` and backticks in a case input arrive verbatim, and large prompts do not deadlock against output).

| Backend | What runs in the container | Needs |
|---------|----------------------------|-------|
| `kiro-cli` | `kiro-cli` directly: `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools [--model <model>]`, prompt on stdin. The argument list and the output handling (ANSI stripped, usage estimated, model = the pinned `--model`) are the same code the native backend uses. | `kiro-cli` in the image (baked at image build). The harness only verifies it is present (`ValidateKiroCLI`); it installs nothing. |
| `stub` (and any other non-`kiro-cli` backend) | The backend runs in-process in the container through a hidden helper command, `kairon inference-exec --backend <name>`. The host mounts a linux `kairon` binary read-only at `/opt/kairon/kairon`, sends the request as one JSON document on stdin, and reads one JSON result on stdout. The stub reads its script (`stub.turns`, including `commands`) from that request, and `WorkDir` is the container workspace path. | A static linux `kairon` binary for the container's platform (see below). `kiro-cli` is not validated. |

`kairon inference-exec` is an internal protocol between the harness and its own binary; it is hidden from `kairon --help` and is not meant to be run by hand.

**The helper binary.** The container image has no `kairon`, so for the stub backend the harness needs a static linux binary matching the container's platform (`linux/amd64` or `linux/arm64`, the host's architecture), which it bind-mounts read-only at `/opt/kairon/kairon`. The file must be readable and executable by the `sandbox` user (mode `0755`); otherwise the run fails with an error naming the file. It is resolved in this order:

1. `KAIRON_SANDBOX_BINARY` — path to a prebuilt static linux `kairon` binary. Use this when Go or the source tree is not available, for example `KAIRON_SANDBOX_BINARY=dist/release/kairon-linux-arm64` (see `task build:linux:arm64` / `task build:linux:amd64`).
2. The running executable, when it is itself a linux binary of the container's architecture.
3. Cross-compilation: `CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build -trimpath ./cmd/kairon`, run from the kairon module root (found by walking up from the working directory to a `go.mod` for `github.com/jbrinkman/kairon`). The result is cached under the user cache directory in `kairon/sandbox/`.

Case 3 requires `go` on `PATH` and a kairon source checkout, which is the case for `task eval:selftest:sandbox`. If none of these is available the run fails with an error that names `KAIRON_SANDBOX_BINARY`. The `kiro-cli` backend never needs the helper binary.

**Network.** Container *execution* has no network by default, but building the base image does: it downloads `kiro-cli` and `gh`, so the first build needs network access (and a container daemon) even for `--backend stub`. Later runs reuse the cached image (see [When the base image is rebuilt](#when-the-base-image-is-rebuilt)).

**Debugging.** With `--debug` the harness prints `🔧 Debug: container invoke backend=<backend> agent=<agent> model=<model>` before each in-container call, showing which backend and model were sent. The in-container call is bounded by the effective timeout (the case's [`timeout`](#case-timeout), else `--resource-limit timeout=` or the sandbox config), which is also passed to the backend as the request timeout.

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

### GitHub CLI in the Sandbox

The base image includes `gh` at a pinned version, but the harness does **not** install a mocked `gh` into the container at run time (that mechanism was removed), and nothing configures or authenticates it. With the container network disabled, `gh` commands cannot reach GitHub, so a sandboxed run cannot make real API calls or create issues or pull requests. Fake `gh` responses and a `gh_issue` check belong to the containment work (#298). The helper `SimulateGitHubResponse` and its embedded skill are kept in the code for that work to reuse or replace.

### Container Lifecycle

Each evaluation follows this lifecycle:

1. **Base image** - once per run, `EnsureBaseImage` reuses the cached tools-only image or builds it (see [When the base image is rebuilt](#when-the-base-image-is-rebuilt)). Nothing is built or removed per case.
2. **Workspace** - on the host, build the case's git workspace, staged `.kiro/` and `.eval/` (see [Case Workspaces](#case-workspaces)).
3. **Create** - create the container from the base image with resource limits, no network, the `sandbox` user and the [mounts](#mounts).
4. **Execute** - run the selected backend inside the container, wrapped for the open umask, with the prompt on stdin (see [Backends in the Container](#backends-in-the-container)). For `kiro-cli` the harness first checks (read-only) that `kiro-cli` is present.
5. **Score** - score the case while the host workspace still exists.
6. **Cleanup** - stop and remove the container, then delete the workspace unless `--keep-workspaces` is set.

### Troubleshooting Container Issues

**Container daemon not running:**
```bash
# Ensure the Docker daemon is running
sudo systemctl start docker   # Linux
open -a Docker               # macOS

# Or start the Podman machine
podman machine start
```

**Permission denied:**
```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
newgrp docker
```

Permission errors *inside* the workspace (`mkdir: can't create directory … Permission denied`) should not occur: the workspace and `.eval/` are mounted read-write and opened to all users, and `.kiro/` is intentionally read-only. If you see one, check that you are not writing to `.kiro/` and that `KAIRON_EVAL_WORKSPACE_ROOT` points at a path your runtime shares with its VM.

**Workspace mount fails (macOS):**
```bash
# Put workspaces under a directory the VM shares
KAIRON_EVAL_WORKSPACE_ROOT="$HOME/.cache/kairon-eval" kairon eval --sandbox
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

# Or set it for one case in its YAML:  timeout: 10m
```

**Build failures:**
```bash
# The first run builds the base image and needs network access (kiro-cli and gh are downloaded)
docker images | grep kairon-eval-base

# Inspect a kept workspace and a preserved debug container
kairon eval --sandbox --debug --keep-workspaces
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
- **User isolation** - Runs as the non-root `sandbox` user (uid 1000)
- **Read-only agent configuration** - the staged `.kiro/` is mounted read-only, and the live repository is never mounted
- **No real GitHub access** - `gh` is unauthenticated and the network is off
- **Temporary containers** - Automatically cleaned up after evaluation

Limits of this layer: the container's root filesystem is still writable, tool trust is unchanged, and the workspace is world-writable on the host while a run is in flight. Tightening these is the containment work (#298).

## Comparing Runs

The `eval diff` command shows:
- Per-criterion score deltas per agent
- Token and cost deltas
- Quality-per-dollar assessment
