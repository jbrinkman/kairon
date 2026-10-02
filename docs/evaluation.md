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
  results/           # Results tagged by git hash
    <git-short-hash>/
      architect.json
      builder.json
      documenter.json
      krew-lead.json
      summary.json
```

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
```

Fields:
- `name` — unique identifier
- `description` — what this case tests
- `input` — the input the agent would receive
- `output` — (optional) pre-captured output for offline scoring

## Running Evaluations

```bash
# Evaluate all agents
kairon eval

# Evaluate a specific agent
kairon eval architect

# Compare two runs
kairon eval diff abc1234 def5678
```

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

Results are written to `.kairon/evals/results/<git-hash>/` enabling before/after comparison when prompts change.

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

This creates a results snapshot at `.kairon/evals/results/<git-hash>/` for comparison.

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
