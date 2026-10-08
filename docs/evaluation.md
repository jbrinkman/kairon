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
    mock-cli.sh      # Reusable stand-in for any CLI (aws, npm, curl, ...); see Preventing Production Side Effects
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
requires_sandbox: true     # Optional: refuse to run without --sandbox (see Sandbox Containment)
gh_issue:                  # Optional: data for the sandbox's fake `gh issue view` (needs requires_sandbox)
  title: "Add widget"
mocks:                     # Optional: author-supplied command mocks placed first on PATH (needs requires_sandbox)
  - command: aws
    script: fixtures/mock-cli.sh
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
- `requires_sandbox` — (optional, default `false`) when `true`, the case refuses to run without `--sandbox`: a native run records the case as failed with `case "<name>" requires --sandbox` before it creates a workspace or invokes anything (see [Sandbox Containment](#sandbox-containment)).
- `gh_issue` — (optional) the issue the sandbox's fake `gh issue view` answers with: `number` (default `1`), `title` (required), `body`, `state` (default `OPEN`), `author` (default `fake-user`), `labels`. It is only valid together with `requires_sandbox: true`; otherwise loading the cases fails with an error naming the case, because a native run would call the developer's **real** `gh` (see [The fake `gh`](#the-fake-gh)).
- `mocks` — (optional) a list of `{command, script}` entries that each place a mock of a command on the container `PATH`, so a bare `aws`, `npm` or `curl` resolves to the author's script instead of a real tool. `command` is the bare command name (it must match `^[A-Za-z0-9][A-Za-z0-9._+-]*$`, must be unique within the case, and cannot be `gh`, which is the harness's fake). `script` is a path relative to the evals directory (no absolute path, no `..`) naming an existing regular file; `fixtures/mock-cli.sh` is the reusable one. Like `gh_issue`, `mocks` is only valid together with `requires_sandbox: true`, because a native run has no such directory on `PATH` and would silently call the **real** tool; otherwise loading the cases fails with an error naming the case. See [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking).
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

# Evaluate a candidate prompt without touching the live agent
kairon eval architect --prompt-file .kairon/iterations/architect/v2.md
```

| Flag | Default | Description |
|------|---------|-------------|
| `--backend` | `kiro-cli` | Inference backend for agent and judge calls. Valid values: `kiro-cli`, `stub`. An unknown value fails immediately and lists the valid backends. |
| `--evals-dir` | `.kairon/evals` | Directory holding `rubrics/`, `cases/`, `fixtures/`, optional `agents/`, and `results/`. Persistent flag, so `kairon eval diff` honours it too. |
| `--keep-workspaces` | off | Keep each case's workspace after the run instead of deleting it. The path is printed on the case line and recorded as `workspace_dir` in the results (see [Case Workspaces](#case-workspaces)). Combine with `--debug` to inspect both the preserved container and its workspace. |
| `--prompt-file` | none | Evaluate this candidate prompt file instead of the agent's live prompt. Requires an agent argument (`kairon eval <agent> --prompt-file <path>`). The candidate is staged only into the per-case workspaces; the repository's `.kiro/agents/` is never modified. See [Evaluating a Candidate Prompt](#evaluating-a-candidate-prompt---prompt-file). |

### Evaluating a Candidate Prompt (`--prompt-file`)

Iterating on a prompt by editing `.kiro/agents/<agent>-prompt.md` changes the agent the pipeline itself is running on. `--prompt-file` scores a candidate instead, leaving the live agent untouched:

```bash
# Baseline: the live prompt
kairon eval architect

# Candidate: same agent, same cases, the candidate's prompt text
kairon eval architect --prompt-file .kairon/iterations/architect/v2.md

# Compare the two runs (directories under results/)
kairon eval diff <baseline-run> <candidate-run>
```

**Convention.** Keep candidates under `.kairon/iterations/<agent>/` (for example `.kairon/iterations/architect/v2.md`). The flag does not enforce a location; any readable path works. The convention keeps candidates out of `.kiro/agents/` and easy to find.

**An agent argument is required.** The candidate replaces *that agent's* prompt, so `kairon eval --prompt-file x.md` (no agent) is refused. Use `--prompt-file` with a single agent, and optionally a single case (`kairon eval architect <case> --prompt-file …` or `--case`).

**What is replaced.** The agent config's `prompt` must be a **relative `file://` reference** (for example `"prompt": "file://./architect-prompt.md"`). The candidate's bytes are written to that file inside each case workspace's staged `.kiro/agents/` directory, so the agent reads the candidate on native and `--sandbox` runs alike. Nothing outside the workspaces is written. Only the agent under test sees the candidate; the judge is unaffected. The agent config itself (model, tools, resources) is unchanged, so a candidate changes the prompt text only.

**Refusals.** The checks run before any result directory is created or any model is called, so a refused invocation runs nothing and leaves no `results/` directory behind. `kairon eval` exits non-zero in each of these cases:

| Condition | Message (abridged) |
|-----------|--------------------|
| No agent argument | `--prompt-file: an agent is required (usage: kairon eval <agent> --prompt-file <path>)` |
| Combined with `--list`, `--perf` or `--cleanup` | `--prompt-file cannot be combined with --list` (or `--perf`, `--cleanup`). Those modes run no cases through workspaces, so the candidate would be silently ignored. |
| Candidate path does not exist or cannot be read | `--prompt-file: …` with the underlying OS error |
| Candidate is not a regular file (for example a directory) | `--prompt-file: <path> is not a regular file` |
| Candidate is empty or whitespace-only | `--prompt-file: <path> is empty` |
| Agent config cannot be found or parsed | `--prompt-file: …` naming the config that was tried |
| The agent's `prompt` is inline or missing (not `file://`) | `prompt must be a relative file:// reference … to use --prompt-file` |
| The agent's `prompt` is an absolute `file://` path | `prompt reference … must be a relative file:// path to use --prompt-file` |
| The agent's `prompt` reference escapes the `.kiro` tree (for example `file://../../x.md`) | `prompt reference … escapes the .kiro tree` |
| `--resume` of a candidate run without the same `--prompt-file`, or `--resume --prompt-file` onto a run recorded without it | The standard [resume refusal](#resume-refusal): `cannot resume: prompt or models changed …` |

The model pre-flight ([Allowlist refusal](#allowlist-refusal)) still applies to a candidate run, since the candidate only replaces the prompt.

**Resume.** Because the candidate's bytes are part of `prompt_sha256`, `--resume` of a candidate run must repeat the same `--prompt-file` with the same content. Resuming without it, with a different candidate, or after editing the candidate file is refused, as is resuming a run recorded without a candidate while passing one. Start a fresh run instead.

**Recorded.** A candidate run records the path in `prompt_file` and a `prompt_sha256` computed from the candidate's bytes (see [Recorded provenance](#recorded-provenance) and [How `prompt_sha256` is computed](#how-prompt_sha256-is-computed)).

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
| `kiro-cli` (default) | Shells out to `kiro-cli`. Agent (native run): `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools [--model <model>]`; under `--sandbox` the agent call uses `--trust-tools=<per-agent set>` instead of `--trust-all-tools` (see [Tool trust](#tool-trust)); judge: `kiro-cli chat --no-interactive [--model <model>]`; prompt on stdin. `--model` is appended when the request carries a model, which is always the case in a normal `kairon eval` run (see [Model Pinning and Run Provenance](#model-pinning-and-run-provenance)). Usage is always **estimated**. Requires `kiro-cli` on `PATH`. |
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
      tool_calls:          # optional: scripted tool calls, gated by the tool trust set (see Tool trust)
        - tool: fs_write
          command: "echo x > tool-marker.txt"
```

| Field | Required | Description |
|-------|----------|-------------|
| `stub.turns[].response` | yes | Text returned as the agent output. |
| `stub.turns[].model` | no | Model name recorded in `agent_cost.model`. |
| `stub.turns[].usage.input_tokens` / `output_tokens` | no | If present, these counts are used verbatim and marked `reported`. If absent, usage is estimated from text length. |
| `stub.turns[].commands` | no | Shell commands, each run with `sh -c` in the case workspace (the request's `WorkDir`), in order, before the response is returned. This lets a stub case simulate an agent that edits files. They are scripted environment actions, **not** tool calls, so the trust gate never applies to them. |
| `stub.turns[].tool_calls[]` | no | Scripted tool calls, each `{tool, command}`. They run after `commands`, in order. Each runs `sh -c <command>` in the workspace only if the tool is in the request's trust set; otherwise the command is skipped and a denial is recorded (see [Tool trust](#tool-trust)). With no trust set (every native run) every tool call runs. |

Only `turns[0]` is used today. A case with no `stub`, empty `turns`, or an empty `response` fails with `case has no stub.turns[0].response` rather than silently producing empty output. The `kiro-cli` backend ignores `stub`.

How `commands` behave:
- They run in the case workspace: natively the host workspace directory, under `--sandbox` the container's workspace path, as the `sandbox` user. Commands with no workspace directory are an error and run nothing, so a test cannot write into the repository root by accident.
- A command that exits non-zero fails the call with an error carrying the command and its stderr; the scripted response is not returned and later commands do not run.
- All commands of a turn share one deadline: the request timeout (the case [`timeout`](#case-timeout), else the default). On expiry the command's whole process group is killed and the call fails with a timeout error (`stub timeout after <duration>`) that wraps `inference.ErrTimeout`, so it is recorded exactly like a `kiro-cli` timeout.
- `commands` and the trusted `tool_calls` of a turn run under that same deadline, `commands` first. A failing command stops the turn like any other command, and denials already recorded for the turn are kept on the failed call.

### Reported vs Estimated Usage

Each `agent_cost` and `judge_cost` in a result file carries `model` (when known) and `usage_source`:

| `usage_source` | Meaning |
|----------------|---------|
| `reported` | Token counts were supplied by the backend (for the stub: `stub.turns[].usage`). |
| `estimated` | Token counts were estimated as roughly 4 characters per token (`len/4`) of the prompt and the output. |

- `kiro-cli` exposes neither the served model nor token counts, so its usage is always `estimated`. The `model` it records is the model the process was *launched with* (the value passed as `--model`, i.e. the pinned model), not a model confirmed by the service. It is empty (omitted from JSON) only for unpinned calls, such as code paths that do not go through `kairon eval`.
- `estimated_usd` is always computed from the token counts at a fixed $3 / $15 per million input / output tokens, whether the counts were reported or estimated.
- When several judge calls are accumulated into `judge_cost`, the merged `usage_source` is `reported` only if every contributing call was `reported`; otherwise it is `estimated`. The stub judge always produces estimated usage with model `stub`.
- `summary.json` totals only `tokens_in`, `tokens_out` and `estimated_usd` (plus the provenance, execution-mode and containment fields described below).
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
  # trust_tools:                   # optional: agent name -> tools trusted in --sandbox runs (see Tool trust)
  #   builder: [read, write, shell]
```

| Key | Default | Description |
|-----|---------|-------------|
| `evals.agent_model` | empty (not set) | Model for the agent under test. When set, it is used for every agent in the run and **overrides** the `model` in the agent's config. When empty, each agent runs on the `model` from its own agent config (`<evals-dir>/agents/<agent>.json` if present, else `.kiro/agents/<agent>.json`). |
| `evals.judge_model` | `claude-sonnet-5.5` | Model for every LLM-judge call. An explicit empty value stays empty and is refused. |
| `evals.allowed_models` | the six models listed above, in that order | Allowlist. The effective agent model and the judge model must each be exactly one of these entries. |
| `evals.trust_tools` | none | Map of agent name to the tool names that agent is trusted to use in a `--sandbox` run. It **overrides** the agent config's `allowedTools` for that agent; an empty list (`builder: []`) trusts nothing. `*` and empty entries are rejected. Has no effect on native runs. See [Tool trust](#tool-trust). |

Notes:
- A user-supplied `allowed_models` list **replaces** the default list; it is not appended to it. Omit the key to keep the defaults.
- All values are whitespace-trimmed, and empty `allowed_models` entries are dropped. For `trust_tools`, agent and tool names are trimmed and entries with an empty agent name are dropped.
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

**`--sandbox` honours `evals.agent_model`.** A sandboxed run builds the same agent request as a native run, so the pinned model is sent with the call. With the `kiro-cli` backend the container runs `kiro-cli chat --agent <agent> --no-interactive --trust-tools=<per-agent set> --model <model>`: the same arguments as the native backend except that `--trust-all-tools` is replaced by `--trust-tools=…` (see [Tool trust](#tool-trust)). The effective model is the same as without `--sandbox`: `evals.agent_model` when set, otherwise the `model` in the agent config. It is validated against `allowed_models` in the pre-flight, recorded as `agent_model`, and recorded on the agent call. No warning is printed.

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
| `trusted_tools` | The whole-tool trust set the call ran with, in `kiro-cli --trust-tools` spelling (for example `["fs_read","fs_write"]`). Present only on `--sandbox` agent calls; omitted for native calls, which run with `--trust-all-tools`. An empty list `[]` means "restricted to nothing", which is different from absent. |
| `tool_denials` | Tool calls the trust gate refused: `{tool, command, reason}` with `reason` `tool not trusted`. Omitted when empty. Populated by the stub backend only (see [Limits](#limits)). |
| `error` | Set when the call failed. A failed call is still recorded. |

Details:
- `agent_cost` and `judge_cost` are unchanged; `calls` is the per-call breakdown behind them.
- No agent record is written when prompt assembly failed, because no call was made.
- A judge call is recorded even when its output could not be parsed (the tokens were spent). Its cost appears in that call's `calls[]` record but **not** in the case's `judge_cost`, which keeps a zero cost for a failed or unparseable judge call — so for such a case the sum of `calls[].cost_usd` can exceed `judge_cost`.
- A `--sandbox` run builds its agent record exactly like a native one, from the same request and the same completion logic: the cost comes from the backend's reported or estimated usage, `model` is the served model when the backend reports one (the stub) or the pinned model (`kiro-cli`), and `estimated` is `true` unless the usage was `reported`. For the same case the sandboxed and native `output`, `agent_cost`, call record `model` and `prompt_sha256` match. The sandboxed agent record additionally carries `trusted_tools`; the only other differences between a native and a container run are the run-level `sandbox` and `containment` fields (see [Execution mode and containment](#execution-mode-and-containment) and [Parity between native and container runs](#parity-between-native-and-container-runs)).

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
| `prompt_file` | The `--prompt-file` path, exactly as given on the command line (slash-normalised, not made absolute, so results stay machine-independent). **Omitted** when the run used no candidate. When present, `prompt_sha256` was computed from the candidate's bytes. |
| `resources_present` | The agent-config `resources` entries that existed when the hash was computed. Always written; an empty list serialises as `[]`. |
| `sandbox` | Execution mode as a **boolean**: `true` for a `--sandbox` (container) run, `false` for a native run. Written by every run, including single-case (`--testcase`) runs. Absent in files written before mode tracking. Note that `summary.json` spells the same fact as a string (`"native"` / `"container"`); see below. |
| `containment` | What contained this agent's cases (see [Execution mode and containment](#execution-mode-and-containment)). Present for container runs only; absent for native runs. |

```json
{
  "agent": "selftest",
  "git_hash": "a1b2c3d",
  "agent_model": "claude-sonnet-5.5",
  "judge_model": "claude-sonnet-5.5",
  "prompt_sha256": "9f2c…",
  "sandbox": false,
  "resources_present": [],
  "cases": [ … ]
}
```

A candidate run (`--prompt-file`) additionally carries `prompt_file` next to `prompt_sha256`:

```json
{
  "agent": "architect",
  "prompt_sha256": "4be1…",
  "prompt_file": ".kairon/iterations/architect/v2.md",
  …
}
```

#### Run summary (`summary.json`)

`summary.json` always carries `judge_model` and an `agents` map with `agent_model`, `prompt_sha256` and `resources_present` per agent (plus `prompt_file` for an agent run with a `--prompt-file` candidate). It also records the run's execution mode in `sandbox` and, for container runs, a per-agent `containment` map. The example below is a `--sandbox` (container) run (the `tool_trust` values are illustrative; they follow each agent's trust set, see [Tool trust](#tool-trust)):

```json
{
  "git_hash": "a1b2c3d",
  "total_cost": { … },
  "agent_scores": { "architect": 0.85, "builder": 0.8 },
  "judge_model": "claude-sonnet-5.5",
  "agents": {
    "architect": { "agent_model": "claude-sonnet-5.5", "prompt_sha256": "…", "resources_present": [] },
    "builder":   { "agent_model": "claude-sonnet-5.5", "prompt_sha256": "…", "resources_present": [] }
  },
  "sandbox": "container",
  "containment": {
    "architect": { "tool_trust": ["fs_read", "fs_write", "execute_bash"], "fake_gh": true, "read_only_fs": true, "network": "unrestricted" },
    "builder":   { "tool_trust": ["fs_read", "fs_write", "execute_bash"], "fake_gh": true, "read_only_fs": true, "network": "unrestricted" }
  }
}
```

| Field | Description |
|-------|-------------|
| `git_hash`, `total_cost`, `agent_scores` | Run totals. `agent_scores` is score / maximum per agent, skipped criteria excluded (see [Skipped Criteria](#skipped-criteria)). |
| `judge_model`, `agents` | Run-level provenance, as described above. |
| `sandbox` | Execution mode of the whole run: `"native"` or `"container"`. Absent in a `summary.json` written before mode tracking. |
| `containment` | Map of agent name to its containment record (see [Execution mode and containment](#execution-mode-and-containment)). Present for `container` runs; **absent** for `native` runs. |

**Two spellings of one fact.** `summary.json` records the mode as a string (`"native"` / `"container"`), while each `<agent>.json` keeps `sandbox` as a boolean (`true` = container, `false` = native). Both come from the same run, and `eval diff` and `--resume` read both. A summary never mixes modes: one run is entirely native or entirely container.

**Single-agent rule.** A run can cover several agents (`kairon eval` with no agent), each with its own model and prompt hash, so one top-level value would be ambiguous. The top-level `agent_model`, `prompt_sha256`, `prompt_file` and `resources_present` are therefore written **only when the run covers exactly one agent** (`kairon eval <agent>`, a single-case run, or the self-test) and are omitted otherwise. Use the `agents` map, or the per-agent `<agent>.json`, for multi-agent runs. Single-case runs also write a `summary.json`, with the same provenance fields.

**`prompt_file` is omitted without a candidate.** `prompt_file` appears in `<agent>.json`, in `summary.json` under `agents.<agent>`, and (single-agent runs) at the top level of `summary.json`, only when the run used `--prompt-file`. Without a candidate the field is absent everywhere, so result files from ordinary runs are unchanged. Full, single-case and `--resume` runs all record it. For example, a candidate run of one agent:

```json
{
  "agents": {
    "architect": {
      "agent_model": "claude-sonnet-5.5",
      "prompt_sha256": "4be1…",
      "prompt_file": ".kairon/iterations/architect/v2.md",
      "resources_present": []
    }
  },
  "agent_model": "claude-sonnet-5.5",
  "prompt_sha256": "4be1…",
  "prompt_file": ".kairon/iterations/architect/v2.md",
  …
}
```

The pre-flight line also names the candidate: `🔒 Models: judge=…, architect=… (prompt sha256 4be1…, prompt file .kairon/iterations/architect/v2.md)`.

#### Execution mode and containment

Every run records how it was executed, through the same code that records the model pins, so no path (full run, single case, resume) can skip it.

- A **native** run records `sandbox: "native"` in `summary.json` (`sandbox: false` in `<agent>.json`) and **no containment**: nothing is contained, so there is nothing to describe. Native summaries do not carry a `containment` key at all, and an old native summary has the same shape as a new one apart from `sandbox`.
- A **container** run (`--sandbox`) records `sandbox: "container"` (`sandbox: true` in `<agent>.json`) and a containment record per agent, in `summary.json` under `containment.<agent>` and in `<agent>.json` as `containment`. The record is per agent because the tool trust set is per agent.

The containment record has four fields:

| Field | Meaning |
|-------|---------|
| `tool_trust` | The normalised `--trust-tools` names the agent's cases ran with, in `kiro-cli` spelling (for example `["fs_read","fs_write"]`; see [Tool trust](#tool-trust)). It is the same resolved set the call used, so it equals `trusted_tools` on that agent's [call records](#per-call-records-calls). `[]` means "trust nothing" and is always written, never omitted. If the trust set cannot be resolved (no `evals.trust_tools` override and the agent config is unreadable), `[]` is recorded and a warning is printed; the agent call itself fails closed, as described under Tool trust. |
| `fake_gh` | `true`: the fake `gh` is first on `PATH` (see [The fake `gh`](#the-fake-gh)). |
| `read_only_fs` | `true`: the container root filesystem is read-only (see [Read-only root filesystem](#read-only-root-filesystem)). |
| `network` | Always `"unrestricted"`. See below. |

`tool_trust`, `fake_gh` and `read_only_fs` are taken from the same sources that enforce them (the trust resolution used for the call, the fake-`gh` mount and the container host config's read-only root filesystem), so the record cannot drift from what was applied.

**`network: "unrestricted"` is not the container's network mode.** The value records that Kairon makes **no network containment guarantee** for the run. It says nothing about the runtime's `NetworkMode`: containers are currently created with `NetworkMode: none`, but, as the [Sandbox Containment](#sandbox-containment) layer table and [Limits](#limits) state, that is a pre-existing setting that is "not a network policy" and is not part of the containment guarantee. Do not read `unrestricted` as "the container has network access", and do not read the absence of a guarantee as "the network is blocked". Side effects over the network remain the eval author's responsibility, handled with mocks; guidance for writing those mocks is planned as a separate follow-up (the mock-guidance issue, see [Limits](#limits)). `NetworkMode` itself is unchanged by this recording.

#### Parity between native and container runs

A native run and a `--sandbox` run of the same case are meant to be directly comparable, and the harness is built so that this holds structurally rather than by convention:

- **Provenance.** `agent_model`, `judge_model`, `prompt_sha256` and `resources_present`, in `<agent>.json` and `summary.json`, and `model` and `prompt_sha256` on every `calls[]` record, come from the same pinning and call-record code on both paths, so they are identical for the same inputs.
- **Scoring.** Both paths score a case through the same function. The score and denominator arithmetic (skipped criteria excluded from numerator and denominator) lives in one place, shared by the printed case result, the incremental `summary.json` writer and the summary builder, so a native and a container summary cannot compute `agent_scores` differently.
- **What may differ.** Apart from fields that are inherently per-run (timestamps in directory names, per-case container ids and workspace paths), a container run differs from a native run only in `sandbox`, `containment`, and `trusted_tools` on the agent call records.

The daemon-gated test `TestProvenanceParitySandbox` (run by `task eval:selftest:sandbox`) checks this by running `selftest` and `selftest-fail` natively and with `--sandbox` on the stub backend. It compares the provenance fields in `<agent>.json`, `summary.json` and every call record, checks `native` without containment against `container` with it, compares the **whole** `Summary` after normalising only `sandbox` and `containment`, compares per-case score totals and the threshold outcome, and requires `eval diff` between the two runs to succeed and report the mode difference. Like the other gated tests it skips, without passing, when the gate is unset or no daemon is reachable (see [Self-Test in the Container Sandbox](#self-test-in-the-container-sandbox)).

**The per-agent threshold verdict (E7) is separate work.** There is no per-agent pass threshold, PASS/FAIL verdict or non-zero exit code in `kairon eval` yet: scores are reported, and `kairon eval` does not fail the process on them. This change does not add one. What it guarantees is parity of the **inputs** such a verdict will use (the per-case and per-agent score and denominator, computed in one shared place). When the verdict is added it will build on that shared code, and because the parity test compares the whole `Summary`, any verdict fields added to it are covered on both paths.

#### How `prompt_sha256` is computed

`prompt_sha256` is a lowercase hex SHA-256 (64 characters). It is a hash only; the prompt text itself is not stored. It is computed over an ordered sequence of parts:

1. **config** — the raw bytes of the agent config file. The config is `<evals-dir>/agents/<agent>.json` when it exists, else `.kiro/agents/<agent>.json` relative to the working directory (in a `kiro-cli` sandbox run only the latter).
2. **prompt** — if the config's `prompt` starts with `file://`, the bytes of that file. A relative path resolves against the config file's directory; absolute paths are allowed. A missing or unreadable prompt file is an error. An inline prompt is already covered by the config bytes. **With `--prompt-file`, the candidate's bytes replace the live prompt file's bytes in this part**; the live prompt file is not read (it need not even exist). Everything else is hashed as usual.
3. **resource** — for each entry of the config's `resources` array, in config order, the bytes of every existing matching file.

Each part is framed as `<kind>\x00<decimal length>\x00<bytes>` (`kind` is `config`, `prompt` or `resource`), so parts cannot run together ambiguously. File paths are not hashed, so the same contents give the same hash on any machine.

Consequently, editing the prompt file, the config, or any present resource changes the hash, and so does reordering resources. Running the same inputs twice gives an identical hash, which makes a before/after comparison of two runs a check that only the intended thing changed.

With a `--prompt-file` candidate the hash reflects the candidate's content, not its path or name: a candidate whose content differs from the live prompt gives a different `prompt_sha256`, and a candidate whose bytes are identical to the live prompt gives the **identical** hash (so a candidate copy of the live prompt is a valid control run). The path is recorded separately as `prompt_file`.

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

Start a fresh run (without `--resume`) after changing a prompt, a resource or `evals`. **`--resume` needs the same `--prompt-file`:** a candidate's content is part of `prompt_sha256`, so resuming a candidate run without `--prompt-file`, with a different or edited candidate, or resuming a run recorded without a candidate while passing one, changes the hash and is refused with the error above. A result file written before provenance existed (no `prompt_sha256`) is **refused when it already holds saved cases** — resuming would attribute those scores to the current prompt and models, which were unknown when they were produced; an empty legacy file is allowed. An unchanged resume continues as before.

**Execution mode.** Resume also refuses to switch between native and `--sandbox`, because scoring `requires_sandbox` cases under the other mode would mark them completed under the wrong execution model. The check is made at two levels:

- per agent, against the `sandbox` boolean in the existing `<agent>.json` (a file that holds saved cases but predates mode tracking is refused, since its mode cannot be verified);
- for the run as a whole, against the `sandbox` string in `summary.json`. This catches a multi-agent run that was interrupted before a later agent's file existed, where there is nothing per-agent to compare. The error names both modes:

```
❌ cannot resume: sandbox mode changed since the interrupted run (summary recorded sandbox=native; now container) — resume with the same mode or start a fresh run
```

Resume with the same mode, or start a fresh run. As a last line of defence, the summary writer also refuses to overwrite a recorded mode with a different one, so one `summary.json` never mixes modes. A `summary.json` with no recorded `sandbox` (an old run) is not mode-checked at this level.

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
<workspace-parent>/     # private (0700) parent, removed with the workspace
  bin/gh                # the fake gh (harness-owned; mounted read-only at /opt/kairon/bin under --sandbox)
  bin/<command>         # one executable per case `mocks:` entry, staged next to the fake gh (same mount)
  ws/                   # the workspace proper (<workspace> below)
    <fixture files>     # contents of fixtures/workspaces/<name>/ (empty by default)
    .kiro/              # staged agent and skill configuration (harness-owned)
    .eval/              # outputs directory (harness-owned, created empty)
```

The tree below `ws/` is the git repo. `bin/` sits beside it, outside the repo and outside the agent's writable mounts, so an agent cannot modify the fake `gh` or a case's mocks. `bin/` holds the fake `gh` and any mocks the case declares with `mocks:` (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking)).

How it is built, in order:

1. A private (`0700`) parent directory is created under `$KAIRON_EVAL_WORKSPACE_ROOT` or, when unset, the OS temp directory, and symlinks in the path are resolved (macOS `/var` is really `/private/var`, and Docker Desktop and Podman machine only share real paths). Set `KAIRON_EVAL_WORKSPACE_ROOT` when your `TMPDIR` is not on a path your container runtime can mount.
2. The case's fixture, `<evals-dir>/fixtures/workspaces/<name>/`, is copied in (regular files and directories only; symlinks and `.git` are skipped). A case without `workspace:` gets an empty workspace.
3. `git init -b main` and a single commit of the fixture (`--allow-empty` for the default workspace). The commit is hermetic: fixed identity and date, no hooks, no signing, and no user or system git config. **That commit is the only commit and its tree is the fixture**, so `git status --porcelain` afterwards shows exactly what the agent (or the stub's `commands`) changed.
4. `.eval/` and `.kiro/` are added to `.git/info/exclude` (not to a tracked file), so harness-owned paths never appear in `git status`. Files the fixture itself tracks under `.kiro/` stay tracked.
5. `.kiro/` is staged (below) and an empty `.eval/` is created. If the case defines `gh_issue`, the host also renders `.eval/gh-issue.json` and `.eval/gh-issue.txt` for the fake `gh` (see [The fake `gh`](#the-fake-gh)); a case without `gh_issue` gets neither file. The fake `gh` script itself is written to `<workspace-parent>/bin/gh` (mode `0755`), next to the workspace rather than inside it; each of the case's `mocks:` is written beside it as `<workspace-parent>/bin/<command>` (mode `0755`), right after the fake `gh`.
6. Permissions are opened so the unprivileged container user can use the tree whatever its host owner: `a+rwX` on everything including `.git`, and `a+rX` (read-only) on `.kiro/`.

### Staged `.kiro` and precedence

The workspace's `.kiro/` is assembled on the host, never copied into a running container. Precedence, highest first:

1. a file provided by the workspace fixture itself,
2. `<evals-dir>/agents/*` (copied to `.kiro/agents/`),
3. the project's own `.kiro/` (agents, skills, MCP config, as set up by `kairon init`).

The project's `.kiro/` is only read. The live repository's `.kiro/` is never exposed to the agent, and a fixture can override any file (for example a `*-conventions` skill).

**`--prompt-file` is applied last.** After the three layers above are staged, and before the workspace is made read-only, a [`--prompt-file`](#evaluating-a-candidate-prompt---prompt-file) candidate replaces the agent's prompt file (the relative `file://` target of its config's `prompt`, for example `.kiro/agents/architect-prompt.md`) in that workspace. The existing entry is removed first, so the candidate wins over a fixture-provided or overlay-provided file at the same path. This happens in each case workspace only; the project's `.kiro/` and the `<evals-dir>/agents/` overlay are not modified.

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

`.eval/` is the outputs directory. Under `--sandbox` it is bind-mounted read-write into the container, but it physically lives inside the host workspace, so whatever the process writes there is on the host the moment it is written. No "copy outputs out" step exists, and the layout is identical to a native run. Under `--sandbox` the fake `gh` also uses it: it appends every call to `.eval/gh.log` and copies body files to `.eval/gh-body-<n>.md` (see [The fake `gh`](#the-fake-gh)). A case mock built from `fixtures/mock-cli.sh` logs to `.eval/mock-<command>.log` the same way.

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
  agents/selftest-sandbox.json      # containment agent, allowedTools [read, write]; its cases are all requires_sandbox
  agents/selftest-sandbox-prompt.md
  agents/selftest-sandbox-ro.json   # containment agent, allowedTools [read]
  agents/selftest-sandbox-ro-prompt.md
  rubrics/selftest-sandbox.yaml
  rubrics/selftest-sandbox-ro.yaml
  cases/selftest-sandbox/stub-gh-fake.yaml            # fake gh: logged call, copied --body-file, gh issue view from gh_issue
  cases/selftest-sandbox/stub-workspace-write.yaml    # './marker.txt' in the workspace is writable
  cases/selftest-sandbox/stub-tool-allowed.yaml       # fs_write tool call runs (fs_write is trusted)
  cases/selftest-sandbox/stub-mock-cli.yaml           # mocks: aws -> fixtures/mock-cli.sh; call logged in .eval/mock-aws.log, answered from the workspace
  cases/selftest-sandbox-ro/stub-write-outside-mounts.yaml  # write to /etc fails with 'Read-only file system' (expected to FAIL)
  cases/selftest-sandbox-ro/stub-tool-denied.yaml     # fs_write tool call is denied and recorded (only fs_read is trusted)
  fixtures/selftest-input.md        # referenced as .kairon/evals/fixtures/selftest-input.md
  fixtures/mock-cli.sh              # byte-identical copy of .kairon/evals/fixtures/mock-cli.sh (this evals dir resolves case scripts here)
  fixtures/workspaces/seeded/       # README.md plus docs/notes.txt: the seeded workspace fixture
  fixtures/workspaces/mock-aws/     # report.txt plus .mocks/aws/s3-cp.out: canned reply for the mocked aws
```

`selftest.json` declares a `model` so the self-test passes the model-pinning pre-flight, and lists a `skill://.kiro/skills/selftest-conventions/SKILL.md` resource that intentionally does not exist. It exercises the "missing resources are normal" rule: the run succeeds and the recorded `resources_present` is `[]`.

The `selftest` agent has five cases and all of them pass (`task eval:selftest`). The `selftest-fail` agent is deliberately separate, so `selftest` keeps meaning "everything passes". Its one case, `stub-timeout`, is **expected to fail**: it records a timeout failure (empty `actual_output`, `error_context.stderr` containing `timeout after 1s`) in about a second. Run it with:

```bash
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail
```

The run itself exits 0; the failure is the recorded result for that case.

The `selftest-sandbox` and `selftest-sandbox-ro` agents exercise [Sandbox Containment](#sandbox-containment) with the stub backend (`selftest-sandbox` includes `stub-mock-cli`, the working example of [the mock pattern](#preventing-production-side-effects-containment-and-mocking)). Every one of their cases is `requires_sandbox: true`, so a native run records each as failed with `requires --sandbox` and starts nothing. `stub-write-outside-mounts` is, like `selftest-fail`, **expected to fail** under `--sandbox`: that failure is the proof that the root filesystem is read-only. Run them under the sandbox with:

```bash
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest-sandbox
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest-sandbox-ro
```

The self-test runs with the stub backend, so its per-call `model` values are `stub` / `stub-model` (what the stub reports) rather than the pinned names. The pinned models still appear in the run-level `agent_model` and `judge_model` fields.

To list the self-test cases: `kairon eval --evals-dir internal/eval/testdata/evals --list selftest`.

Results go to `internal/eval/testdata/evals/results/`, which is git-ignored (`.gitignore` entry `internal/eval/testdata/evals/results/`), so running the self-test leaves the working tree clean. These fixtures live under `testdata`, outside the template-synced `.kairon/evals/`, so they do not affect `task sync:check`.

### Self-Test in the Container Sandbox

The same self-test can run hermetically inside a container sandbox. It needs a Podman or Docker daemon and network access for the image build (see [Backends in the Container](#backends-in-the-container)):

```bash
task eval:selftest:sandbox
# runs the daemon-gated tests with KAIRON_EVAL_SANDBOX_SELFTEST=1:
#   internal/eval/sandbox: TestBaseImage_ToolsOnlyNoMounts, TestEnsureBaseImage_ReusesExistingImage
#   internal/eval:         TestSelftestSandbox, TestSandboxWorkspace, TestContainmentSandbox, TestContainmentContainerGH, TestProvenanceParitySandbox

# or run the sandboxed self-test directly:
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest
```

`TestSelftestSandbox` runs `selftest` natively and then again with `--sandbox --backend stub`, and requires the sandboxed run to match the native one for every case: non-empty and identical `actual_output`, identical `agent_cost`, and the same model on the recorded agent call. It also applies the same self-test expectations to the sandboxed results. The `stub-quoted-input` case checks that shell metacharacters in the input arrive intact.

The other gated tests cover the sandbox layering:
- `TestProvenanceParitySandbox` runs `selftest` and `selftest-fail` natively and with `--sandbox` and checks provenance and scoring parity and the `eval diff` mode report (see [Parity between native and container runs](#parity-between-native-and-container-runs)).
- `TestBaseImage_ToolsOnlyNoMounts` starts a container from the base image with no mounts and checks it runs as `sandbox` (uid 1000), has `kiro-cli`, `gh`, `git` and `sh` on `PATH`, and has an empty `/workspace` with no `/workspace/.kiro` and no project or agent content anywhere.
- `TestEnsureBaseImage_ReusesExistingImage` calls `EnsureBaseImage` twice and requires the second call to report no build with the same tag and image ID.
- `TestSandboxWorkspace` runs `stub-marker`, `stub-seeded-workspace` and the `selftest-fail` timeout case both natively and under `--sandbox` with kept workspaces and compares them: no `Permission denied` anywhere, `marker.txt` containing `hi` on the host, identical host trees (excluding `.git` and `.kiro`) and `git status --porcelain`, `.eval/` present in both, an unchanged repository-root `git status`, keep/no-keep behaviour of `workspace_dir`, the sandboxed timeout recorded as `timeout after 1s`, and base-image reuse after editing an agent config and a case.

A plain `go test ./...` also runs `TestSelfTestWorkspaceCasesNative`, which needs no daemon: it runs the same new cases natively and checks the workspace behaviour above. It likewise runs `TestContainmentFixtures`, `TestContainmentMockCLIFixtureParity` and `TestContainmentNativeRefusal`, which need no daemon either: the first checks that the containment cases load and are all `requires_sandbox` (and that `stub-mock-cli` declares the `aws` mock), the second that the self-test copy of `mock-cli.sh` is byte-identical to the live `.kairon/evals/fixtures/mock-cli.sh`, the third that a native run fails every one of them with `requires --sandbox`, makes no agent call and creates no workspace. The mock script itself is covered by hermetic tests that run it with `sh` on the host (`mockcli_test.go`) and the `mocks:` validation and staging by `mocks_test.go`.

The containment tests are daemon-gated like the rest:
- `TestContainmentSandbox` runs the containment agents under `--sandbox --backend stub --keep-workspaces` and checks: `.eval/gh.log` holds the `gh issue create` call and `.eval/gh-body-1.md` equals `b.md`; `view.json` holds the configured `gh_issue` title and body; `stub-mock-cli` (subtest `mocked cli`) leaves `.eval/mock-aws.log` with exactly the two `aws` calls, `aws-path.txt` holding `/opt/kairon/bin/aws`, `aws-out.txt` equal to the canned `.mocks/aws/s3-cp.out`, `aws-unsimulated.txt` containing `is not simulated`, and only those three `aws-*.txt` files in `git status --porcelain`; `stub-write-outside-mounts` fails with `Read-only file system` in `error_context.stderr`; `marker.txt` exists on the host after `stub-workspace-write`; `stub-tool-denied` leaves no `tool-marker.txt` and records an `fs_write` denial with `trusted_tools` `["fs_read"]`; `stub-tool-allowed` creates `tool-marker.txt` with no denials.
- `TestContainmentContainerGH` starts a container with the case's mounts, environment and host config and checks that `command -v gh` is `/opt/kairon/bin/gh`, and that `/usr/local/bin/gh auth status` (the real `gh`, by absolute path) exits non-zero with a "not logged in" message.

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

The evaluation framework serves as unit testing for prompt engineering. Iterate on a **candidate** prompt with `--prompt-file`; do not edit the live prompt (`.kiro/agents/<agent>-prompt.md`) while you iterate. The live prompt is what the running pipeline and your day-to-day agents use, and a mid-iteration edit changes them. A candidate is staged only into the per-case evaluation workspaces (see [Evaluating a Candidate Prompt](#evaluating-a-candidate-prompt---prompt-file)).

### Before Making Changes (Baseline)

**Required**: Run `kairon eval` for the agent before you start, to establish a baseline of the live prompt:

```bash
# Capture current performance of the live prompt
kairon eval architect
```

This creates a results snapshot at `.kairon/evals/results/<timestamp>-<git-hash>/` for comparison.

### Iterating on a Candidate

Copy the live prompt to a candidate under `.kairon/iterations/<agent>/`, edit the copy, and evaluate it:

```bash
mkdir -p .kairon/iterations/architect
cp .kiro/agents/architect-prompt.md .kairon/iterations/architect/v2.md
# edit .kairon/iterations/architect/v2.md

kairon eval architect --prompt-file .kairon/iterations/architect/v2.md
```

Each iteration (`v3.md`, `v4.md`, …) is a new file and a new run. `git status --porcelain .kiro/agents` stays empty throughout. The run records `prompt_file` and a `prompt_sha256` of the candidate's content, so every result states which candidate produced it (see [Recorded provenance](#recorded-provenance)). A candidate is evaluated against the agent's existing config, so the model, tools and resources are those of the live agent. To resume an interrupted candidate run, repeat the same `--prompt-file` (see [Resume refusal](#resume-refusal)).

### After Making Changes (Verification)

**Required**: Compare the candidate's results with the baseline before adopting it:

```bash
# Compare with baseline (names of directories under results/)
kairon eval diff <baseline-run> <candidate-run>
```

`eval diff` reports a `prompt_sha256` change between the two runs, confirming the candidate's text was what was scored.

### Adopting a Candidate

Adopting a winning candidate is a separate, deliberate step, done only after the comparison is favourable:

1. Copy the candidate over the live prompt: `cp .kairon/iterations/architect/v2.md .kiro/agents/architect-prompt.md`.
2. Re-sync the embedded templates and verify (`cp .kiro/agents/*.md cmd/kairon/templates/kiro/agents/`, then `task sync:check`), because `.kiro/agents/` is template-synchronized.
3. Run `kairon eval architect` once more on the now-live prompt. Its `prompt_sha256` equals the candidate run's, since identical content gives an identical hash, and it records no `prompt_file`.

### Creating Test Cases for Behavioral Changes

When making specific behavioral changes, create targeted test cases:

1. **Identify the behavior** — What specific agent behavior are you changing?
2. **Create test case** — Add a case in `.kairon/evals/cases/<agent>/` that exercises this behavior
3. **Verify coverage** — Ensure existing rubric criteria measure the desired change
4. **Test iteratively** — Run evaluations against each candidate as you refine the prompt

Example workflow for improving architect task decomposition:
```bash
# 1. Baseline (live prompt)
kairon eval architect

# 2. Add test case for complex decomposition scenario
# Edit .kairon/evals/cases/architect/complex-decomposition.yaml

# 3. Create a candidate; the live prompt is not edited
mkdir -p .kairon/iterations/architect
cp .kiro/agents/architect-prompt.md .kairon/iterations/architect/v2.md
# Edit .kairon/iterations/architect/v2.md

# 4. Verify improvement
kairon eval architect --prompt-file .kairon/iterations/architect/v2.md
kairon eval diff <baseline> <candidate>
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
- Builds each case's workspace on the host (see [Case Workspaces](#case-workspaces)) and bind-mounts it into a fresh container with resource limits, no network and a read-only root filesystem.
- Runs the selected backend inside the container as the unprivileged `sandbox` user, with a fake `gh` first on `PATH` and a per-agent tool trust set instead of `--trust-all-tools` (see [Sandbox Containment](#sandbox-containment)).

Nothing is copied into a running container: no project files, no `.kiro`, no helper binary, no fake `gh`. Everything the agent sees arrives through bind mounts.

> **Model pinning in sandbox runs:** `--sandbox` honours `evals.agent_model` (it is passed to `kiro-cli` as `--model`, exactly as in a native run) and `evals.judge_model` for judge calls. See [`--sandbox` and `evals.agent_model`](#--sandbox-and-evalsagent_model).

### Sandbox Layering

The sandbox is split strictly by stability. Stable tools are baked once into an image at build time. Everything that varies per project or per run is supplied from the host through bind mounts.

```
build time, as root, cached by content hash        run time, host-side only, mounted
┌───────────────────────────────────────┐     ┌──────────────────────────────────────────────┐
│ kairon-eval-base:<platform>-<hash>    │     │ <ws>/.kiro   ro  staged project .kiro        │
│  alpine, git, bash/sh, ca-certs,      │  +  │ <ws>         rw  git repo, fixture commit    │
│  gh (pinned), kiro-cli (pinned),      │     │ <ws>/.eval   rw  outputs (inside <ws>)       │
│  user sandbox (uid 1000), /workspace  │     │ /opt/kairon/bin ro  fake gh + case mocks     │
└───────────────────────────────────────┘     │ /opt/kairon/kairon ro  helper (non-kiro-cli) │
                                              └──────────────────────────────────────────────┘
```

The root filesystem of the running container is read-only; only the mounts above (the rw ones) and a few small tmpfs directories are writable (see [Read-only root filesystem](#read-only-root-filesystem)).

#### Base image contents

The image is defined by `internal/eval/dockerfile/base.Dockerfile` (embedded in the binary). It contains:

- Alpine 3.19 with `git`, `bash` and CA certificates (a POSIX `sh` is always present),
- `kiro-cli` at a pinned version, in `/usr/local/bin`,
- `gh` (GitHub CLI) at a pinned version, in `/usr/local/bin`,
- the unprivileged user `sandbox` (uid 1000) and an empty `/workspace` it owns,
- a build-time smoke test (`kiro-cli --version && gh --version && git --version`).

Deliberately **absent**: project toolchains (Go, Node.js, Python, Rust, Java, Task), any `COPY`/`ADD` of project content, `.kiro`, agents, skills, cases and the `kairon` binary. The sandbox no longer detects the project type or installs toolchains; if your evals need a toolchain, add it to the image (see [bumping a baked tool](#when-the-base-image-is-rebuilt)).

The baked `gh` is the **real** GitHub CLI and is **unauthenticated**: the container environment carries no GitHub token, `$HOME` is a fresh tmpfs with no `~/.config/gh/hosts.yml`, and the network is disabled. It is not what an agent reaches by default: a fake `gh` is first on `PATH` and answers instead (see [The fake `gh`](#the-fake-gh)). The real binary is only reached by absolute path (`/usr/local/bin/gh`), where `gh auth status` reports "not logged in". `ContainerConfig.MockGitHub` no longer exists. `kiro-cli` authentication inside the container is also not provided: a real `kiro-cli` sandbox run still needs credentials supplied through the container environment.

#### Mounts

Every container gets exactly these mounts (all are `bind` mounts of host paths that must exist; a missing path is an error, not an auto-created root-owned directory):

| Host | Container | Mode |
|------|-----------|------|
| `<workspace>/.kiro` | `<workspace_dir>/.kiro` | read-only |
| `<workspace>` | `<workspace_dir>` | read-write |
| `<workspace>/.eval` | `<workspace_dir>/.eval` | read-write |
| `<workspace-parent>/bin` (holds the fake `gh` and any case mocks) | `/opt/kairon/bin` | read-only |
| the linux `kairon` helper (non-`kiro-cli` backends only) | `/opt/kairon/kairon` | read-only |

`<workspace_dir>` is `sandbox.workspace_dir` from `.kairon/config.yaml` (default `/workspace`). There is no `tmpfs` at the workspace path. On top of these bind mounts the container has three small `tmpfs` mounts (`/tmp`, `/var/tmp`, `/home/sandbox`) and a read-only root filesystem; see [Read-only root filesystem](#read-only-root-filesystem). The container runs as `sandbox` with `WorkingDir` set to the workspace, `HOME=/home/sandbox`, `PATH` starting with `/opt/kairon/bin`, and a git `safe.directory=*` setting passed as environment (`GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_0`/`GIT_CONFIG_VALUE_0`), because the mounted repo is owned by a different uid than `sandbox` and git would otherwise refuse it as "dubious ownership".

#### Ownership and cleanup

Mounted directories are owned by the host user while the container runs as uid 1000. To make that work on rootful Docker, rootless Podman and the Docker Desktop / Podman machine VMs on macOS:

- The host opens the workspace to everyone (`a+rwX`) before the container starts. The temporary parent directory is `0700`, so other local users cannot traverse into it.
- Every command the harness executes in the container is wrapped as `sh -c 'umask 000; exec "$@"' kairon-exec <command…>`, so files the agent creates are world-accessible and the host user can read them for scoring and delete them afterwards, even when the container uid maps to a foreign host uid. The wrapper does not change the command's arguments, and the prompt is still delivered on stdin only.
- Removing the workspace never fails a run (a warning with the path is printed instead).

While a run is in flight the workspace contents are world-writable.

#### Platform notes

- **macOS (Docker Desktop, Podman machine):** the workspace path must be shared with the VM. The harness resolves symlinks (`/var` → `/private/var`), but if your `TMPDIR` is not a shared mount, set `KAIRON_EVAL_WORKSPACE_ROOT` to a directory under your home directory.
- **Rootless Podman:** supported through the ownership rules above; the open umask wrapper is what normally prevents files that the host cannot delete.
- **SELinux:** when the host reports SELinux as enforcing (`/sys/fs/selinux/enforce` is `1`), the container is created with `label=disable` so the user's directories are not relabeled with `:z`/`:Z`. This is only done on enforcing hosts.

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
| `kiro-cli` | `kiro-cli` directly: `kiro-cli chat --agent <agent> --no-interactive --trust-tools=<per-agent set> [--model <model>]`, prompt on stdin. The argument list and the output handling (ANSI stripped, usage estimated, model = the pinned `--model`) are the same code the native backend uses; the only difference is that the native backend passes `--trust-all-tools` where the container passes `--trust-tools=…` (see [Tool trust](#tool-trust)). | `kiro-cli` in the image (baked at image build). The harness only verifies it is present (`ValidateKiroCLI`); it installs nothing. |
| `stub` (and any other non-`kiro-cli` backend) | The backend runs in-process in the container through a hidden helper command, `kairon inference-exec --backend <name>`. The host mounts a linux `kairon` binary read-only at `/opt/kairon/kairon`, sends the request as one JSON document on stdin, and reads one JSON result on stdout. The stub reads its script (`stub.turns`, including `commands` and `tool_calls`) from that request, and `WorkDir` is the container workspace path. The request carries the trust set and the result carries any tool denials, so the stub's trust gate runs inside the container. | A static linux `kairon` binary for the container's platform (see below). `kiro-cli` is not validated. |

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

### Sandbox Containment

A `--sandbox` run contains an arbitrary agent at the layers Kairon itself controls. It does not assume a fixed tool set. Three layers are enforced by mechanism, plus a pre-existing network setting that is **not** part of the containment guarantee (see [Limits](#limits)):

| Layer | Mechanism | Enforced by |
|-------|-----------|-------------|
| `gh` | A fake `gh` is bind-mounted read-only and is first on `PATH`. It logs every call, copies `--body-file` bodies and answers `gh issue view` from the case's `gh_issue`. The baked, real `gh` stays reachable by absolute path but is unauthenticated. | Kernel mount and environment |
| Filesystem | Read-only root filesystem. Only the workspace, `.eval/` and a few small tmpfs directories are writable. | Container runtime |
| Tool trust | `--trust-all-tools` is replaced in the container by `--trust-tools=<per-agent set>`. Trust is per tool, not per argument. | `kiro-cli` |
| Network (not a guarantee) | Containers are created with `NetworkMode: none`. This is left exactly as it was and is not a network policy. | Container runtime |

Native runs (without `--sandbox`) are **not** contained: they keep `--trust-all-tools`, use the real `gh` on your machine and write wherever the agent can. For a case that is only safe inside the sandbox, set `requires_sandbox: true` (see [Test Case Format](#test-case-format)).

What a run applied is recorded in its results: `sandbox` and a per-agent `containment` record in `summary.json`, and a native run records no containment. See [Execution mode and containment](#execution-mode-and-containment) (including why the recorded `network` value is `unrestricted`).

For the whole model on one page (layers, owners, limits and extension seams), see [Containment model](#containment-model).

What this does and does not cover, and how to keep a case away from real AWS, `npm publish` and arbitrary HTTP, is the subject of [Preventing Production Side Effects (Containment and Mocking)](#preventing-production-side-effects-containment-and-mocking) below.

#### The fake `gh`

**Delivery.** The fake is a POSIX `sh` script embedded in the `kairon` binary. For each case the host writes it to `<workspace-parent>/bin/gh` (mode `0755`) and bind-mounts that directory **read-only** at `/opt/kairon/bin`. The container `PATH` is `/opt/kairon/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, so a bare `gh` resolves to the fake. Nothing is copied into the running container and the base image is unchanged, so its cache key is too. The script needs no `jq`.

**Files.** The fake finds the outputs directory through `KAIRON_EVAL_DIR`, which is set to `<workspace_dir>/.eval` (so a non-default `sandbox.workspace_dir` works).

| File in `.eval/` | Written by | Content |
|------------------|------------|---------|
| `gh.log` | the fake, on every call | One line per call: `gh <args>`. Args are space-joined with no timestamp (deterministic). An argument is single-quoted only if it is empty or contains a character outside `[A-Za-z0-9_./:=@%+,-]` (for example `gh issue create --title 'Add widget'`); newlines inside an argument are folded to spaces, so a call is always one line. Every invocation is logged first, including ones that then fail. |
| `gh-body-<n>.md` | the fake | A copy of the body passed with `--body-file <path>`, `-F <path>` or `--body-file=<path>`. `n` starts at 1 and takes the next free number (claimed atomically), so a second call writes `gh-body-2.md`. `--body-file -` copies stdin. A missing file fails like real `gh` (`open <path>: no such file or directory`, exit 1) and is still logged. Bodies are copied for every call that carries the flag, including commands the fake does not simulate. `-F` is always read as `--body-file`, so `gh api -F key=value` fails with an open error. |
| `gh-issue.json`, `gh-issue.txt` | the host, before the container starts | The case's `gh_issue` rendered as JSON (one top-level field per line) and as the human `gh issue view` text. Written only when the case defines `gh_issue`. |

Because `.eval/` lives in the host workspace, `gh.log` and the `gh-body-<n>.md` files are on the host the moment they are written. With `--keep-workspaces` you can read them after the run (`cat <workspace_dir>/.eval/gh.log`).

**Behaviour.**

| Call | Result |
|------|--------|
| `gh issue create …` | Prints `https://github.com/fake-owner/fake-repo/issues/<seq>`, exit 0. `<seq>` is a sequence number counting only `create` calls, shared across `gh issue create` and `gh pr create` (so the first create in a case is `1`, the next `2`, …, regardless of other `gh` calls logged in between). It is claimed atomically, so concurrent creates never collide. |
| `gh pr create …` | Prints `https://github.com/fake-owner/fake-repo/pull/<seq>`, exit 0. `<seq>` shares the same create counter as `gh issue create`. |
| `gh issue view [N\|url]` | Answers from the case's `gh_issue`; see below. |
| `gh issue list\|edit\|comment\|close\|reopen`, `gh pr list\|view\|comment\|edit` | Simulated: exit 0 with no output. |
| `gh auth status` | Prints a fake logged-in state and exits 0, so an agent's pre-checks pass. This is the *fake*; it never consults the real binary. |
| `gh --version` | Prints `gh version 2.0.0 (kairon fake gh)`. |
| anything else | Logged, then stderr `kairon fake gh: "<args>" is not simulated (call logged only)` and exit 1. An honest failure rather than a silent success: the call is visible in `gh.log` and you can extend the script. |

**`gh issue view` and `gh_issue`.** A case that wants `gh issue view` to return something sets `gh_issue` (and `requires_sandbox: true`, which loading enforces):

```yaml
requires_sandbox: true
gh_issue:
  number: 42            # default 1
  title: "Add widget"   # required
  body: "..."
  state: OPEN           # default OPEN
  author: octocat       # default fake-user
  labels: [bug]
```

- Without `--json`, `gh issue view` prints `gh-issue.txt`.
- `--json a,b` prints a JSON object holding just those fields, in the order requested. The available fields are `author` (`{"login": …}`), `body`, `labels` (`[{"name": …}]`), `number`, `state`, `title` and `url`. An unknown field prints `Unknown JSON field: "x"` and the available fields to stderr and exits 1.
- A number or URL that differs from the configured `number` exits 1 with a `could not resolve to an issue with the number N` message. If the case has no `gh_issue`, the call exits 1 with `kairon fake gh: no issue configured for this case (gh_issue)`.
- **`--jq`, `-q`, `--template` and `-t` are not supported** (the image has no `jq`). `gh issue view` exits 1 with a message telling the agent to use `--json` and parse the output itself, and the call is still logged. This is a documented limit of the fake, not something to work around by editing the image.

**The real `gh` is unauthenticated.** The baked `gh` is still at `/usr/local/bin/gh`, so an agent that calls it by absolute path reaches the real binary. It cannot authenticate: the container environment is built from scratch and additionally drops `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN` and `GH_HOST` even if they appear in `ContainerConfig.Environment`; `GH_PROMPT_DISABLED=1` and `GH_NO_UPDATE_NOTIFIER=1` are set; and `$HOME` is a fresh tmpfs, so there is no `~/.config/gh/hosts.yml`. `/usr/local/bin/gh auth status` therefore reports "not logged in" and exits non-zero. This does not rely on the network being disabled.

#### Read-only root filesystem

Every container is created with `ReadonlyRootfs: true`. It is always on; there is no opt-out. Only these paths are writable:

| Path | Kind | Notes |
|------|------|-------|
| `<workspace_dir>` (default `/workspace`) | bind mount, read-write | The case's git workspace. |
| `<workspace_dir>/.eval` | bind mount, read-write | Outputs, `gh.log` and `gh-body-<n>.md`. |
| `/tmp` | tmpfs, `rw,nosuid,nodev,mode=1777,size=256m` | Scratch. |
| `/var/tmp` | tmpfs, `rw,nosuid,nodev,mode=1777,size=64m` | Scratch. |
| `/home/sandbox` | tmpfs, `rw,nosuid,nodev,uid=1000,gid=1000,mode=0755,size=256m` | `$HOME`. `kiro-cli` keeps state here, so it must be writable. Ephemeral: it is gone when the container is. |

Everything else is read-only, including `<workspace_dir>/.kiro`, `/opt/kairon/bin`, `/opt/kairon/kairon` and the rest of the image. A write outside the writable paths fails with `Read-only file system` (EROFS), which is distinguishable from the `Permission denied` the unprivileged `sandbox` user already got for places like `/etc`.

Notes:
- `/home/sandbox` is a deliberate hole in "read-only". It is ephemeral and size-capped, and is the reason a real `kiro-cli` run works at all.
- `noexec` is not set on the tmpfs mounts: `kiro-cli` and tools may execute from temporary locations.
- tmpfs memory counts against the container memory limit (see [Resource Limits](#resource-limits)).
- Not added, and possible further hardening: dropping Linux capabilities, `no-new-privileges` and a pids limit.

#### Tool trust

Tool trust applies to **container runs only**. A native run keeps `kiro-cli chat … --trust-all-tools`; under `--sandbox` the agent call gets a single argument `--trust-tools=<csv>` in its place (`--trust-tools=` when the set is empty, which trusts no tools). The trust set is per tool, not per argument.

**Resolution.** For each agent, the first match wins:

1. `evals.trust_tools[<agent>]` in `.kairon/config.yaml`: an explicit override. It may be an empty list, which trusts nothing.
2. The agent config's `allowedTools`, read from the same file the model pinning uses: `<evals-dir>/agents/<agent>.json` if it exists, else `.kiro/agents/<agent>.json`.
3. Neither: the empty set (`--trust-tools=`). The default **fails closed**. An agent config without an `allowedTools` key trusts nothing.

If there is no override and the agent config cannot be found or read, the call fails with a `tool trust: …` error that lists the paths tried; the agent is not run, and the failed call is still recorded.

`*` and empty entries in `evals.trust_tools` are rejected when the config is loaded (the error names the agent), so trusting all tools cannot be restored by accident. `toolsSettings` in an agent config (for example `shell.autoAllowReadonly` or `write.allowedPaths`) stay owned by the agent author and pass through in the staged `.kiro` unchanged; Kairon generates none.

```yaml
evals:
  trust_tools:
    builder: [read, write]   # narrower than the shipped builder allowedTools
    validator: []            # trust nothing
```

**Name normalization.** The short names used in agent configs are rewritten to the spelling `kiro-cli --trust-tools` expects. The table lives in one place (`internal/inference/trust.go`). Duplicates are dropped and the first-seen order is kept; surrounding whitespace is trimmed.

| In `allowedTools` / `trust_tools` | Passed to `--trust-tools` |
|-----------------------------------|---------------------------|
| `read` | `fs_read` |
| `write` | `fs_write` |
| `shell` | `execute_bash` |
| `aws` | `use_aws` |
| `fs_read`, `fs_write`, `execute_bash`, `use_aws` | unchanged |
| any other name (`web_search`, `web_fetch`, `subagent`, `todo_list`, `@server/tool`, …) | unchanged |

**Default per-agent trust sets.** With no `trust_tools` override, the trust set is the agent's own `allowedTools`. For the agents that `kairon init` ships:

| Agent | `allowedTools` | Passed to `--trust-tools` |
|-------|----------------|---------------------------|
| `architect` | `read`, `write`, `shell` | `fs_read,fs_write,execute_bash` |
| `builder` | `read`, `write`, `shell` | `fs_read,fs_write,execute_bash` |
| `validator` | `read`, `write`, `shell` | `fs_read,fs_write,execute_bash` |
| `documenter` | `read`, `write` | `fs_read,fs_write` |
| `planner` | `read`, `write`, `shell`, `web_search`, `web_fetch` | `fs_read,fs_write,execute_bash,web_search,web_fetch` |
| `krew-lead` | `read`, `shell`, `subagent`, `todo_list` | `fs_read,execute_bash,subagent,todo_list` |

If you edit an agent's `allowedTools` (or add MCP entries), the default follows it; entries are passed through as written. The self-test agents use `allowedTools` too: `selftest-sandbox` trusts `fs_read,fs_write` and `selftest-sandbox-ro` trusts `fs_read`.

**What is recorded.** Each container agent call carries `trusted_tools` in its [call record](#per-call-records-calls): the normalized set it ran with, `[]` when restricted to nothing, absent for native calls.

**The stub backend models the plumbing.** With `--backend stub`, a turn's `tool_calls` run only if the tool is in the trust set; a denied call is skipped (its command does not run) and added to the call's `tool_denials` with reason `tool not trusted`. For example, with `selftest-sandbox-ro` (trust set `fs_read`) an `fs_write` tool call leaves no file behind and records a denial, while the same call under `selftest-sandbox` (trust set `fs_read,fs_write`) runs. This is a deterministic, network-free model of the harness plumbing; the real enforcement is `kiro-cli`'s.

#### Limits

Containment here is deliberately narrow. These are the boundaries it does **not** provide:

- **Trust is whole-tool, not per-argument.** Trusting `execute_bash` trusts every shell command the agent runs; trusting `fs_write` trusts every path it can write. Kairon does not inspect or constrain arguments, and per-argument interception is not built. It is an extension seam for later work. `toolsSettings` an agent author wrote (for example `allowedPaths`) are passed through as written; Kairon neither generates nor verifies them.
- **The network is not an enforced boundary.** None of the layers above severs or gateways network access. Containers are currently created with `NetworkMode: none`, which this work leaves exactly as it was; it is not a policy to rely on, and it is the reason a real `kiro-cli` run in the container cannot reach a model endpoint today. **Side effects over the network are the eval author's responsibility, handled with mocks** (for example a fake of the service a case would otherwise call). The contract, the risk vectors, the working mock pattern and its limits are in [Preventing Production Side Effects (Containment and Mocking)](#preventing-production-side-effects-containment-and-mocking). Mocks are a per-case convention, not an enforced boundary: nothing in the harness stops a case that is given credentials and a network, and that calls a real service it did not mock, from reaching it.
- **`kiro-cli` denials are not detectable.** Refused tool calls cannot be reliably detected from `kiro-cli`'s output, so `tool_denials` is populated by the stub backend only. For a real `kiro-cli` run the record states which tools were **trusted** (`trusted_tools`), and the raw output and stderr are kept as before; it does not state which were denied.
- **Tool names are not verified end to end.** `fs_read` and `fs_write` come from `kiro-cli chat --help` and `execute_bash` from its tool table. That `read`/`write`/`shell` map to those names, and that `web_search`, `web_fetch`, `subagent` and `todo_list` pass through unchanged, is an assumption that CI cannot check (it cannot run a real model); the argument list is unit-tested only. If `kiro-cli` rejects a bare `@server`, list MCP tools as `@server/tool`. How `kiro-cli` combines `--trust-tools` with an agent's own `allowedTools` is not documented: an override wider than `allowedTools` may still be limited by `kiro-cli` itself.
- **The fake `gh` is a fake.** It supports a fixed set of commands, does not support `--jq`/`--template`, and is bypassed by an agent that calls `/usr/local/bin/gh` (which is unauthenticated, by design).
- **Native runs are not contained** (see above).

### Preventing Production Side Effects (Containment and Mocking)

An eval case runs an arbitrary agent with tools. Some of what that agent can do reaches beyond the container: delete cloud resources, publish a package, call a real API. This section states what Kairon prevents, what it leaves to you, and gives a working pattern for the part that is yours.

#### The containment contract

| Concern | Who is responsible | How |
|---------|--------------------|-----|
| Filesystem | **Kairon** (enforced) | Read-only root filesystem; only the workspace, `.eval/` and small tmpfs directories are writable; the live repository is never mounted (see [Read-only root filesystem](#read-only-root-filesystem)). |
| GitHub (`gh`) | **Kairon** (enforced) | A fake `gh` is first on `PATH`; the real `gh` is unauthenticated (see [The fake `gh`](#the-fake-gh)). |
| Which tools run | **Kairon** (whole-tool, via `kiro-cli`) | `--trust-tools=<per-agent set>` instead of `--trust-all-tools` (see [Tool trust](#tool-trust)). |
| Network and every other production side effect | **The eval author** | Write cases against mocks, as described below. |

Stated plainly: Kairon enforces the filesystem boundary and the fake `gh`. The agent is the LLM client and keeps network access for model access. Preventing any other production side effect is the eval author's responsibility, done by writing the case against mocks. (Containers are currently created with `NetworkMode: none`. That is a plain setting, not a network policy, and not something to rely on; see [Limits](#limits).)

#### Risk vectors

These are the ways a case with credentials and a network can leave a mark outside the sandbox:

- **Real AWS / cloud SDK calls.** An agent that runs `aws s3 rm`, `aws iam delete-user`, `gcloud …`, `az …` or `kubectl delete …` acts on whatever account the environment's credentials reach. Mock the CLI (`mocks:` with `command: aws`, `gcloud`, `kubectl`, …).
- **`npm publish`** (and the other publish and registry CLIs: `cargo publish`, `twine upload`, `docker push`, …). A publish is public and often irreversible. Mock the CLI, or point the tool at a registry that cannot be reached with project config in the workspace fixture (see [HTTP endpoints](#http-endpoints-and-in-process-sdks)).
- **Arbitrary HTTP to real services.** `curl`, `wget`, a script's HTTP client, a webhook post. Mock the client CLI, or point the tool's endpoint at a stand-in through workspace config.

**Why Kairon does not network-gateway these.** The obvious alternative is an egress allowlist or proxy. That is an application-level gateway: it has to classify every destination, and it still cannot separate a legitimate model call from an exfiltration to the same host, because the agent is itself the model client and must be allowed to talk to the model endpoint. The complexity would buy a filter that looks like a boundary and is not. Kairon enforces the layers it can enforce cheaply and leaves the rest to mocks, which keep a case away from a real service by construction instead of by filtering.

#### The mock pattern

A mock is a script placed first on the container `PATH`, so a bare `aws`, `npm` or `curl` runs the script instead of a real tool. It reuses the mechanism of the fake `gh`: the host stages the script into `<workspace-parent>/bin/<command>` (mode `0755`), that directory is bind-mounted read-only at `/opt/kairon/bin`, and `/opt/kairon/bin` comes first on `PATH`. The agent cannot modify the script. `PATH` applies to every process in the container, so it reaches the commands a real `kiro-cli` agent runs through its shell tool as well as the stub backend's `commands`.

Three pieces, all following the fake-`gh` precedent:

| Piece | Where | Role |
|-------|-------|------|
| The shim | [`fixtures/mock-cli.sh`](../.kairon/evals/fixtures/mock-cli.sh) in your evals directory (shipped by `kairon init`) | One POSIX `sh` script that can stand in for **any** CLI. It behaves according to the name it is installed as. |
| Canned replies | `fixtures/workspaces/<name>/.mocks/<command>/` | Backing data, committed with the workspace fixture so `git status` stays clean. |
| Case wiring | the case's `mocks:` field | Declares which commands are mocked and which script implements each. |

**Fixture layout** (copy this tree; `mock-aws` is the name used by the self-test):

```
.kairon/evals/
  fixtures/
    mock-cli.sh                     # the shim (copied by `kairon init`; do not edit it per case)
    workspaces/
      mock-aws/                     # referenced by `workspace: mock-aws`
        report.txt                  # whatever the case works on
        .mocks/
          aws/                      # data directory for the command "aws"
            s3-cp.out               # reply for `aws s3 cp …`
            default.out             # optional: reply for any other aws call
  cases/
    builder/
      upload-report.yaml
```

**Case YAML:**

```yaml
name: upload-report-uses-mocked-aws
description: "The agent uploads the report with the aws CLI; the call must hit the mock, not a real account"
agent: builder
requires_sandbox: true        # required whenever mocks is set
workspace: mock-aws           # fixtures/workspaces/mock-aws/ holds the canned replies
mocks:
  - command: aws                      # bare command name placed first on PATH
    script: fixtures/mock-cli.sh      # path relative to the evals directory
input: |
  Upload report.txt to s3://prod-bucket/ with the aws CLI and report the result.
```

`mocks[].command` and `mocks[].script` are validated when the cases are loaded (see [Test Case Format](#test-case-format)): `requires_sandbox: true` is mandatory (a native run has no mock directory on `PATH` and would call the real tool), `command` must be a bare name, unique in the case and not `gh`, and `script` must be an existing regular file inside the evals directory.

**What the shim does**, per call:

1. Appends one line, `<command> <args>`, to `.eval/mock-<command>.log` **before** anything else, so a failing or unsimulated call is logged too. Quoting matches the fake `gh`'s `gh.log`: arguments are space-joined with no timestamp, an argument is single-quoted only if it is empty or contains a character outside `[A-Za-z0-9_./:=@%+,-]`, and newlines are folded to spaces.
2. Looks for a canned reply in the data directory: `$KAIRON_MOCK_DATA` if set, else `.mocks/<command>/` in the workspace. It takes the first two arguments that do not start with `-` (`a1`, `a2`), keeps only `[A-Za-z0-9_]` in each, and tries `<a1>-<a2>.out`, then `<a1>.out`, then `default.out`. The first file that exists is printed to stdout verbatim.
3. Exits with the integer in the sibling `<name>.rc` if there is one (for example `255` to simulate `AccessDenied`), else `0`.
4. If nothing matches, prints `kairon mock <command>: "<args>" is not simulated (call logged only)` to stderr and exits `1`. An unscripted destructive call is recorded and never executed.

The shim never runs another binary (so a real `aws` elsewhere on `PATH` is never invoked) and never opens a connection. It needs `KAIRON_EVAL_DIR`, which the sandbox sets to `<workspace_dir>/.eval`.

For the case above, with `.mocks/aws/s3-cp.out` containing `upload: ./report.txt to s3://prod-bucket/report.txt (kairon mock aws)`:

| The agent runs | The shim prints | `.eval/mock-aws.log` gains |
|----------------|-----------------|----------------------------|
| `aws s3 cp report.txt s3://prod-bucket/report.txt` | `upload: ./report.txt to s3://prod-bucket/report.txt (kairon mock aws)`, exit 0 | `aws s3 cp report.txt s3://prod-bucket/report.txt` |
| `aws iam delete-user --user-name prod-admin` | stderr `kairon mock aws: "iam delete-user --user-name prod-admin" is not simulated (call logged only)`, exit 1 | `aws iam delete-user --user-name prod-admin` |

The working example is the self-test case [`stub-mock-cli`](../internal/eval/testdata/evals/cases/selftest-sandbox/stub-mock-cli.yaml) (agent `selftest-sandbox`, workspace fixture `mock-aws`); the self-test's own evals directory holds a byte-identical copy of the shim, because case scripts resolve against the evals directory in use. Run it with:

```bash
go run ./cmd/kairon eval --backend stub --sandbox --keep-workspaces --evals-dir internal/eval/testdata/evals selftest-sandbox
cat <workspace_dir>/.eval/mock-aws.log     # workspace_dir is printed on the case line and recorded in the results
```

**Adapting it to another tool.** The same script mocks any CLI; change `command:` and ship the replies under `.mocks/<command>/`:

```yaml
mocks:
  - command: npm
    script: fixtures/mock-cli.sh      # .mocks/npm/publish.out answers `npm publish`; no match -> logged, exit 1
  - command: curl
    script: fixtures/mock-cli.sh      # .mocks/curl/default.out answers every curl call
```

For `curl` and `wget` use `default.out`: the first non-flag argument is a URL, and removing every character outside `[A-Za-z0-9_]` from it gives a name nobody wants to type. A flag's *value* is not skipped when choosing `a1` and `a2` (write `aws s3 cp --profile x`, or provide `default.out`). If you need richer behaviour (different replies by argument, stdin handling), copy `mock-cli.sh` under a new name in `fixtures/` and edit the dispatch at the bottom; its header comment documents the contract. Keep it POSIX `sh`: the image uses busybox `ash` and has no `jq`.

##### HTTP endpoints and in-process SDKs

Kairon does not run a mock HTTP server: the base image has none, and a case that needs one ships it in its own image or workspace. Without one, there are two ways to keep HTTP traffic off a real service, both of which work today:

- **Shim the client CLI** (`curl`, `wget`, `aws`, `npm`, `gcloud`) with `mock-cli.sh`, as above. This is what `mocks:` is for.
- **Point the tool at a stand-in through project config that lives in the workspace fixture.** The agent's working directory is the workspace, so such files are honoured. For example a `.npmrc` containing `registry=http://127.0.0.1:4873` (an address nothing answers on, so a publish fails instead of reaching the public registry), or a tool's endpoint override in its own config file.

An SDK called in-process by code the agent writes or runs (a Python script using `boto3`, a Node script using `fetch`) does not look up a command on `PATH`, so a `PATH` mock never sees it. It is covered only by the config route (endpoint overrides the SDK honours), by not running such code in the case, or by trusting fewer tools (below).

**Inspecting and scoring.** Everything the mock recorded is on the host the moment it is written, because `.eval/` lives in the host workspace. Run with `--keep-workspaces` and read `<workspace_dir>/.eval/mock-<command>.log`. Scoring of the recorded interaction today is host-side: the daemon-gated `TestContainmentSandbox` (subtest `mocked cli`) reads the kept workspace and asserts the exact log, the resolution of `aws` to `/opt/kairon/bin/aws`, the canned reply, the `is not simulated` message and the `git status` of the workspace. There is no rubric check type that reads the log: `file_exists`, `changed_files` and similar checks do not exist yet (see [Case Workspaces](#keeping-workspaces-and-workspace_dir)). When file-based checks land, they read the same host files (`<workspace_dir>/.eval/mock-<command>.log`); no schema for them is defined here.

#### Mocks and tool trust

[Tool trust](#tool-trust) and mocks are complementary and solve different halves of the problem:

- **Trust limits which tools run.** Each tool removed from `evals.trust_tools[<agent>]` (or from the agent's `allowedTools`; see the per-agent default trust table under [Tool trust](#tool-trust)) is one less surface a case has to mock.
- **Mocks make the tools that do run safe.** A trusted `execute_bash` can run any command. The shim decides what a bare `aws`, `npm` or `curl` does when it does.

The caution: the `PATH` shim intercepts commands resolved through `PATH` by a trusted **shell** tool. Built-in and MCP tools do their own I/O and are **not** intercepted by it: `use_aws` (the `aws` alias), `web_fetch`, `web_search` and MCP tools (`@server/tool`). A case that relies on a mock for AWS or HTTP should leave those out of the trust set, because a mock cannot cover them. The shipped `planner` agent, for instance, trusts `web_search` and `web_fetch` by default. For a builder-style agent that you want to keep off real AWS and HTTP:

```yaml
# .kairon/config.yaml
evals:
  trust_tools:
    builder: [read, write, shell]     # no aws / use_aws, web_fetch, web_search or MCP entries
```

This is a statement about the `PATH` shim, not about `kiro-cli` internals. How `kiro-cli` combines `--trust-tools` with an agent's own `allowedTools` is not documented (see [Limits](#limits)), so check the recorded `trusted_tools` on the call if it matters.

#### Limits of the mock approach

- **A convention, not an enforced boundary.** Mocks protect a case only if its author writes them. A case that calls a real service without a mock is not prevented by Kairon: with credentials in reach and a network, nothing in the harness stops it.
- **Bypasses.** A mock covers calls resolved through `PATH`. An absolute path (`/usr/local/bin/aws`), an in-process SDK, or a built-in or MCP tool (`use_aws`, `web_fetch`, …) does not go through it.
- **Not tamper-proof on the workspace side.** The shim file is read-only (it lives in the read-only `/opt/kairon/bin`), but the canned replies (`.mocks/`) and the log (`.eval/`) are in the agent-writable workspace. The log is a record for the eval author, not an audit trail.
- **The self-test shows wiring, not avoidance.** The base image contains no `aws` or `npm`, so `stub-mock-cli` can show which binary a bare `aws` resolves to, what is recorded and that an unsimulated destructive call is not executed. It cannot show a real `aws` being avoided. In a project whose image does contain `aws`, the same wiring is what keeps the call off the real service.
- **Trade-off against an app gateway.** A gateway would be a mechanical boundary that does not depend on the author remembering to mock. The price is a destination classifier that cannot tell model traffic from exfiltration and that someone has to maintain per environment. Kairon accepts the weaker, simpler convention, and this section is how it is made usable.

### Containment Model and Extension Seams

This section is the one-page map of how a `--sandbox` run is contained: which layers exist, who owns each, whether it is enforced or a convention, where it stops, and where a consuming project can extend it. It links to the detailed sections rather than restating them; if the two ever disagree, the detailed section wins.

#### Why a container, not a convention

Kairon's earlier plan for Stage 3 isolation (item E4 in `.kairon/specs/maturity-model/gap-analysis.md`) was convention-based: run each case in a temporary workspace, put a `gh` shim first on `PATH` and point `GH_CONFIG_DIR` at an empty directory. **That design is superseded by the container sandbox.** A convention only holds for an agent that cooperates. Kairon runs arbitrary, extensible third-party agents (MCP servers, built-in tools that do their own I/O, scripts that call binaries by absolute path such as `/usr/local/bin/gh`, writes outside the current directory), and a working-directory change or a `PATH` shim cannot stop any of them. The container gives a real mount-level boundary (a read-only root filesystem and a short list of bind mounts), and everything the boundary cannot cover is placed in a named layer with a named owner, below. What E4 contributed beyond isolation (per-case git workspaces, staged `.kiro/`, `.eval/`, timeouts, `--keep-workspaces`) is kept and is shared by native and sandbox runs (see [Case Workspaces](#case-workspaces)).

#### Containment model

| Layer | Owner | Mechanism | Enforced? | Honest limit |
|-------|-------|-----------|-----------|--------------|
| Filesystem | Kairon | Read-only root filesystem plus bind mounts: staged `.kiro/` read-only, workspace and `.eval/` read-write, `/opt/kairon/bin` read-only. The live repository is never mounted. | **Enforced** (container runtime) | Writable holes are the workspace, `.eval/` and the tmpfs `/tmp`, `/var/tmp` and `/home/sandbox`. The workspace is world-writable on the host while a run is in flight. Linux capabilities are not dropped and there is no pids limit. |
| `gh` | Kairon | A fake `gh` is first on `PATH` in a read-only mount; the real `gh` at `/usr/local/bin/gh` is unauthenticated (no token variables, fresh `$HOME`). | **Enforced** (mount and environment) | The fake supports a fixed command set and no `--jq`/`--template`. An agent can still run `/usr/local/bin/gh`, which reports "not logged in". |
| Tool trust | Kairon chooses the set; `kiro-cli` enforces it | `--trust-tools=<per-agent set>` instead of `--trust-all-tools`. | **Enforced, whole-tool only** | There is no per-argument or per-call tool hook in this `kiro-cli` build: trusting `execute_bash` trusts every shell command, and trusting `fs_write` trusts every writable path. `kiro-cli` denials are not detectable (`tool_denials` is populated by the stub backend only). How `--trust-tools` combines with an agent's `allowedTools` is undocumented, and the tool-name mapping is not verified end to end. |
| Network side effects (AWS, `npm publish`, HTTP, any other CLI) | **Eval author** | Per-case mocks (`mocks:` with `fixtures/mock-cli.sh`) and endpoint config in the workspace fixture. | **Not enforced** (convention) | There is no network gateway. `NetworkMode: none` is a pre-existing setting, not a network policy, and is not part of the guarantee. Network side effects are the eval author's responsibility via mocks. A `PATH` mock misses built-in and MCP tools, absolute paths and in-process SDKs. |
| Native runs (no `--sandbox`) | Eval author / operator | None. `requires_sandbox: true` makes a case refuse to run natively. | **Not contained** | `--trust-all-tools`, the real `gh`, writes wherever the agent can. |
| Recording | Kairon | `sandbox` plus a per-agent `containment` record (`tool_trust`, `fake_gh`, `read_only_fs`, `network`) in the results; `eval diff` reports a mode difference. | **Enforced by code** | It records what was applied, not whether it was sufficient. `network: "unrestricted"` records the absence of a guarantee, not the container's network mode. |

Reading the table: **enforced** means a mechanism stops the action whatever the agent does; **convention** means the protection exists only if the eval author wrote it (for example the mock). The model is deliberately not airtight, and the rows marked "not enforced" or "not contained" are the ones to review when you add a case.

##### What the model does not defend against

- Code that runs in-process and talks to the network without going through `PATH` (an SDK called from a script the agent writes or runs).
- Built-in and MCP tools that are trusted: `use_aws`, `web_fetch`, `web_search` and `@server/tool` do their own I/O, so the `PATH` shim does not intercept them and their network side effects are uncontrolled. Their filesystem writes are still subject to the read-only root filesystem and bind mounts like any other process in the container.
- Credentials that a consuming project passes into the container environment. Kairon drops the GitHub credential variables; it does not drop others.
- A trusted `execute_bash` doing anything the container can do inside its writable mounts.
- Network access the agent itself needs. The agent is its own model client, so a destination filter could not tell model traffic from exfiltration (see [Why Kairon does not network-gateway these](#risk-vectors)).

#### Extension seams

Each seam is what you edit, what it does and where it stops.

- **Add a mock for a CLI (`PATH` shim).** Edit the case YAML (`mocks: [{command, script}]` with `requires_sandbox: true`), use `fixtures/mock-cli.sh`, and ship canned replies under `fixtures/workspaces/<name>/.mocks/<command>/`. For logic the shim cannot express, copy it under a new name in `fixtures/`. The script is staged into `/opt/kairon/bin` (read-only) and is first on `PATH`. It stops at what resolves through `PATH`. See [The mock pattern](#the-mock-pattern).
- **Point a tool at a stand-in endpoint (workspace config).** Put project config in the workspace fixture, for example a `.npmrc` with `registry=http://127.0.0.1:4873` or a tool's own endpoint override file. The agent's working directory is the workspace, so the file is honoured. It stops at in-process SDKs that ignore config. See [HTTP endpoints and in-process SDKs](#http-endpoints-and-in-process-sdks).
- **Set per-agent tool trust.** Set `evals.trust_tools.<agent>` in `.kairon/config.yaml` (an explicit override, `[]` trusts nothing) or edit the agent's `allowedTools` (the default). With neither, the set is empty, so the default fails closed; `*` and empty entries are rejected at load. The resolved set is recorded as `trusted_tools` on each call and as `containment.tool_trust` per agent. Leave `use_aws`, `web_fetch`, `web_search` and `@server/tool` out of a case that relies on a `PATH` mock. It stops at whole-tool granularity. See [Tool trust](#tool-trust) and [Mocks and tool trust](#mocks-and-tool-trust).

  ```yaml
  evals:
    trust_tools:
      builder: [read, write]   # narrower than the shipped builder allowedTools
      validator: []            # trust nothing
  ```

- **Extend the fake `gh`.** The fake is a script embedded in the `kairon` binary. A `gh` command it does not simulate is logged and exits 1; that is the signal that the fake needs extending. This is a code change to Kairon, not a case-level setting. See [The fake `gh`](#the-fake-gh).
- **Future: a per-tool-call hook (the preferred fine-grained layer).** **This does not exist in this build.** The `kiro-cli` used here exposes only whole-tool `--trust-tools`, and Kairon has no code for a per-tool-call or per-argument hook; its interface is undefined, and no schema, config key or timeline is committed. If `kiro-cli` gains one, it would plug in next to whole-tool trust: trust is resolved in one place (`resolveTrustSet` in `internal/eval/trust.go`, producing an `inference.ToolTrust`) and applied in the `kiro-cli` argument list built for the container call, and the `containment` record (a field alongside `tool_trust`) is where it would be reported. It would be the preferred way to constrain `execute_bash`, `fs_write` or `use_aws` by argument, and it would reduce, not remove, the reliance on mocks. It would not replace the filesystem layer.
- **Future: a network gateway.** Also not built. It is not preferred, for the reason given under [Risk vectors](#risk-vectors); it would be the second seam if a deployment needs a mechanical network boundary.

#### Where each concern is documented in detail

- [Sandbox Containment](#sandbox-containment): the layer summary and [Limits](#limits)
- [The fake `gh`](#the-fake-gh)
- [Read-only root filesystem](#read-only-root-filesystem)
- [Tool trust](#tool-trust)
- [Preventing Production Side Effects (Containment and Mocking)](#preventing-production-side-effects-containment-and-mocking), including [The mock pattern](#the-mock-pattern)
- [Execution mode and containment](#execution-mode-and-containment): what a run records
- [Security Considerations](#security-considerations)

### Container Lifecycle

Each evaluation follows this lifecycle:

1. **Base image** - once per run, `EnsureBaseImage` reuses the cached tools-only image or builds it (see [When the base image is rebuilt](#when-the-base-image-is-rebuilt)). Nothing is built or removed per case.
2. **Workspace** - on the host, build the case's git workspace, staged `.kiro/`, `.eval/` and the `bin/` directory with the fake `gh` and any case mocks (see [Case Workspaces](#case-workspaces)).
3. **Create** - create the container from the base image with resource limits, no network, a read-only root filesystem with small tmpfs mounts, the `sandbox` user, the [mounts](#mounts) (including the read-only directory at `/opt/kairon/bin` that holds the fake `gh` and any case mocks) and an environment whose `PATH` starts with it.
4. **Execute** - run the selected backend inside the container, wrapped for the open umask, with the prompt on stdin (see [Backends in the Container](#backends-in-the-container)). For `kiro-cli` the harness first checks (read-only) that `kiro-cli` is present, and the agent is started with `--trust-tools=<per-agent set>` (see [Tool trust](#tool-trust)).
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

**`Read-only file system`:**
The container's root filesystem is read-only. Only the workspace, `.eval/`, `/tmp`, `/var/tmp` and `/home/sandbox` are writable (see [Read-only root filesystem](#read-only-root-filesystem)). A tool that writes elsewhere (a cache under `/var/cache`, a file in `/usr/local`, …) fails with this error; point it at `/tmp` or the workspace instead.

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

**Network connectivity:**
Containers are created with `NetworkMode: none`. That is a plain setting, not a network policy, and not something to rely on for containment (see [Limits](#limits)). If a case needs a service, give it a mock rather than a network (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking)).

### Security Considerations

Container sandboxing provides multiple security layers:

- **Process isolation** - Containers run in separate namespaces
- **Resource limits** - CPU and memory usage restricted
- **Network isolation** - No external network access by default (not a containment guarantee; see [Limits](#limits))
- **User isolation** - Runs as the non-root `sandbox` user (uid 1000)
- **Read-only root filesystem** - only the workspace, `.eval/` and small tmpfs directories (`/tmp`, `/var/tmp`, `/home/sandbox`) are writable
- **Read-only agent configuration** - the staged `.kiro/` is mounted read-only, and the live repository is never mounted
- **Fake `gh`, no real GitHub access** - a fake `gh` is first on `PATH` and logs every call; the real `gh` is unauthenticated and GitHub credential variables never reach the container
- **Case mocks** - a case can place its own mock of any other command (`aws`, `npm`, `curl`, …) in the same read-only directory; this is opt-in per case, not enforced (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking))
- **Whole-tool trust** - `kiro-cli` runs with `--trust-tools=<per-agent set>` instead of `--trust-all-tools`
- **Temporary containers** - Automatically cleaned up after evaluation

For the layers, owners and extension seams on one page, see [Containment model](#containment-model).

Limits of this layer: tool trust is per tool, not per argument; the network is not an enforced boundary, so network side effects are the eval author's responsibility via mocks (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking); mocks are a per-case convention, not an enforcement); `kiro-cli`'s own tool denials cannot be detected; Linux capabilities are not dropped and there is no pids limit; and the workspace is world-writable on the host while a run is in flight. See [Limits](#limits) for details.

## Comparing Runs

```bash
kairon eval diff <runA> <runB>
```

The `eval diff` command shows, in this order:
- A **Run Provenance** block (below)
- Per-criterion score deltas per agent
- Token and cost deltas
- Quality-per-dollar assessment

### Run Provenance

Scores are only comparable when the runs were produced under comparable conditions, so the diff starts with a `Run Provenance` block, printed before any deltas. It always shows each run's execution mode, then lists every difference between the two runs in:

- `sandbox` (execution mode: `native` or `container`),
- `containment`, per agent (`tool_trust`, `fake_gh`, `read_only_fs`, `network`; a run with a containment record against one without is shown as `record: none → …`),
- `judge_model`,
- `agent_model`, per agent,
- `prompt_sha256`, per agent.

Trust sets are compared ignoring order. A value a run did not record is shown as `(not recorded)`. When nothing differs the block says `No provenance differences.`

Example, a native run against a `--sandbox` run:

```
Run Provenance:
  260620-200919-e369501: mode native
  260621-160207-8a19eb2: mode container
  sandbox: native → container
  selftest containment.record: none → tool_trust=[execute_bash,fs_read,fs_write] fake_gh=true read_only_fs=true network=unrestricted
  ⚠ Runs used different execution modes (native vs container); scores are not directly comparable.
```

(The run names and values are illustrative.)

**Mode-difference warning.** When both runs have a known execution mode and the modes differ, the block ends with a `⚠` line stating that the runs used different execution modes and that their scores are not directly comparable. A native run is not contained and a container run is, so a score difference may come from the mode rather than from the change under test. Compare like with like where you can.

**Old runs show mode `unknown`.** The mode is read from `summary.json`'s `sandbox`; when that is absent, from the `sandbox` boolean in the agent files, provided every agent file records it and they all agree; otherwise it is shown as `unknown (predates sandbox-mode tracking)`. If either run's mode is unknown the `⚠` line is replaced by a note that comparability cannot be verified, because nothing can be said about whether the modes differ.

**The provenance block only reports.** It never fails the diff and never changes the deltas that follow. All the provenance and mode fields are optional, so **result directories written before this tracking existed still load**: a missing field shows as `(not recorded)` and the mode as `unknown`, and a malformed agent file is skipped as before.
