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
| `kiro-cli` (default) | Shells out to `kiro-cli`. Agent: `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools`; judge: `kiro-cli chat --no-interactive`; prompt on stdin. Usage is always **estimated**. Requires `kiro-cli` on `PATH`. |
| `stub` | Deterministic and in-process. Never starts a process or touches the network, and does not require `kiro-cli`. The agent's output comes from the case's `stub.turns`; every judge call returns score 5 (the maximum of the judge scale) with `pass: true`. |

```bash
kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
```

Notes:
- The `kiro-cli` startup probe (`kiro-cli --version`) and the PATH availability check are performed by the selected backend; the stub reports zero startup overhead and is always available.
- `--backend stub` cannot be combined with `--sandbox` (the sandbox runs `kiro-cli` inside a container); the run fails with an error.
- The Docker sandbox path still runs `kiro-cli` directly in the container and always reports estimated usage.

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

- `kiro-cli` exposes neither model nor token counts, so its usage is always `estimated` and `model` is empty (omitted from JSON).
- `estimated_usd` is always computed from the token counts at a fixed $3 / $15 per million input / output tokens, whether the counts were reported or estimated.
- When several judge calls are accumulated into `judge_cost`, the merged `usage_source` is `reported` only if every contributing call was `reported`; otherwise it is `estimated`. The stub judge always produces estimated usage with model `stub`.
- `summary.json` totals only `tokens_in`, `tokens_out` and `estimated_usd`.

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
  agents/selftest.json              # minimal agent config (prompt: file://./selftest-prompt.md)
  agents/selftest-prompt.md
  rubrics/selftest.yaml             # structural_completeness (deterministic), clarity (LLM-judged), cost_efficiency (cost)
  cases/selftest/stub-basic.yaml    # no stub usage -> estimated; setup file exercises path rebasing
  cases/selftest/stub-usage.yaml    # stub model + usage 123/45 -> reported
  fixtures/selftest-input.md        # referenced as .kairon/evals/fixtures/selftest-input.md
```

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
