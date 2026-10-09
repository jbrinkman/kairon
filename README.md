# Kairon

A GitHub issue-driven AI orchestration system that transforms labeled issues into working code through coordinated AI agent collaboration.

## How It Works

Kairon watches a GitHub repository for issues with a configured label, then spawns AI agents to implement solutions automatically:

```
GitHub Issue (labeled) → Watcher detects → Krew-Lead orchestrates
    → Architect designs → Builder implements → Validator verifies → PR created
```

The system uses `kiro-cli` agents working in isolated git worktrees. Each issue gets its own branch, and on success a pull request is created automatically.

## Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- [GitHub CLI (`gh`)](https://cli.github.com/) — authenticated via `gh auth login`
- [Kiro CLI (`kiro-cli`)](https://kiro.dev) — for running AI agents

## Installation

### Using Go Install

```bash
go install github.com/jbrinkman/kairon@latest
```

### Download Prebuilt Binaries

#### Linux
```bash
# For AMD64 (x86_64)
curl -L https://github.com/jbrinkman/kairon/releases/latest/download/kairon-linux-amd64 -o kairon
chmod +x kairon
sudo mv kairon /usr/local/bin/

# For ARM64 (aarch64)
curl -L https://github.com/jbrinkman/kairon/releases/latest/download/kairon-linux-arm64 -o kairon
chmod +x kairon
sudo mv kairon /usr/local/bin/

# Check your architecture
uname -m  # x86_64 = AMD64, aarch64/arm64 = ARM64
```

### Build from Source

```bash
git clone https://github.com/jbrinkman/kairon.git
cd kairon

# Using Task (recommended)
task build

# Or using Go directly
go build ./cmd/kairon
```

### Development Tasks

This project uses [Task](https://taskfile.dev) for build automation:

```bash
task build    # Build optimized binary with version metadata
task dev      # Development build (faster compilation)
task test     # Run tests with coverage
task clean    # Clean build artifacts
task lint     # Run linters and formatters
```

## Quick Start

### 1. Initialize a project

```bash
cd your-project
kairon init
```

This creates:
- `.kairon/config.yaml` — watcher configuration
- `.kairon/scripts/` — worktree management scripts
- `.kiro/agents/` — agent configurations (krew-lead, architect, builder, validator, documenter)

### 2. Configure

Edit `.kairon/config.yaml`:

```yaml
repo: owner/repo-name
label: kairon
poll_interval: 5m
max_retries: 3
```

| Field | Description | Default |
|-------|-------------|---------|
| `repo` | GitHub repository (owner/name) | *required* |
| `label` | Issue label to watch for | `kairon` |
| `poll_interval` | How often to poll GitHub | `5m` |
| `max_retries` | Max retry attempts per issue | `3` |
| `base_branch` | Integration branch that new issue worktrees are created from (optional) | auto-detect from `origin/HEAD`, else `main` |

### 3. Run

```bash
kairon
```

This starts the interactive REPL. From there, start the watcher:

```
kairon> watch start
kairon> status
```

## CLI Usage

```bash
# Display version and exit
kairon --version

# Initialize project with agent configs and templates (skips existing files)
kairon init

# Force-update templates (overwrites all files except config.yaml)
kairon update

# Start interactive REPL (default when no arguments)
kairon
```

### REPL Commands

| Command | Description |
|---------|-------------|
| `watch start` | Start polling GitHub for labeled issues |
| `watch stop` | Stop polling |
| `status` | Show all agents with issue, status, and elapsed time |
| `stop <issue>` | Stop the agent working on a specific issue number |
| `plan [desc]` | Start interactive planning session |
| `theme` | Show current theme |
| `theme <name>` | Switch to theme |
| `about` | Show version information and check for updates |
| `exit` | Exit (confirms if agents are still running) |
| `help` | Show available commands |

### Hotkey Toggle

Press **Ctrl+Alt+P** (or **Ctrl+Option+P** on macOS) to toggle between console and planning modes:

- **Console Mode**: Main Kairon interface for managing watchers and agents
- **Planning Mode**: Interactive AI-assisted issue creation and planning

Both modes preserve their state when you switch, allowing seamless workflow transitions. See [docs/hotkey-toggle.md](docs/hotkey-toggle.md) for detailed usage information.

## Architecture

### Agent Pipeline

When the watcher detects a labeled issue:

1. **Krew-Lead** — Orchestrates the workflow. Creates a git worktree, delegates to other agents, manages the lifecycle from issue to PR.
2. **Architect** — Reads the issue, explores the codebase, and produces a design specification at `.kairon/specs/issue-<number>-<slug>.md`.
3. **Builder** — Implements code changes according to the architect's specification. Focused on a single task at a time.
4. **Validator** — Read-only agent that verifies the implementation meets acceptance criteria. Runs tests and checks.
5. **Documenter** — Generates documentation in `app_docs/` for completed features.

### Agent Spawning

The manager spawns agents as `kiro-cli` processes:

```
kiro-cli chat --agent krew-lead --no-interactive --trust-all-tools "Process issue #N from repo owner/name. Worktree name: issue-N-<pid>"
```

Each agent runs with environment variables: `ISSUE_NUMBER`, `REPO`, and `KAIRON_WATCHER_PID`.

### Git Worktree Isolation

Each issue is processed in an isolated git worktree named `issue-<number>-<pid>` (where `<pid>` is the watcher process ID):
- `.kairon/scripts/worktree-create.sh <name>` — creates `.worktrees/<name>/` on branch `spec/<name>`
- `.kairon/scripts/worktree-merge.sh <name>` — merges back, removes worktree, deletes branch
- Orphaned worktrees (from crashed processes) are cleaned up automatically by checking if the PID is still running

**Branching from a fresh `origin/<branch>`:** the long-lived main checkout is never fast-forwarded, so `worktree-create.sh` does not branch from its local `HEAD`. Instead it fetches the integration branch from `origin` and creates `spec/<name>` from `origin/<branch>` (with `--no-track`, so the spec branch has no upstream). Issues picked up after other PRs have merged therefore start from the latest remote tip.

The integration branch is resolved in this order (first non-empty wins):

1. `KAIRON_BASE_BRANCH` environment variable
2. `base_branch` in `.kairon/config.yaml`
3. The default branch of `origin` (`origin/HEAD`)
4. `main`

Fallbacks:
- **Offline / fetch fails:** if a cached `origin/<branch>` already exists, the script warns and branches from that last-known ref. If none exists, it exits with an error.
- **No `origin` remote:** the script warns and branches from local `HEAD` (useful for local-only repositories).

`base_branch` only controls which branch the worktree starts from. Pull requests are still created against the repository's default branch.

### Issue Lifecycle

| State | Label | Description |
|-------|-------|-------------|
| Ready | `kairon` | Watcher will pick up this issue |
| Processing | — | Agent spawned and working |
| Done | `kairon-done` | PR created successfully |
| Failed | `kairon-failed` | Exhausted retries |

Issues with `kairon-done` or `kairon-failed` labels are excluded from polling. The done/failed labels are derived from the configured label (e.g., if label is `my-label`, done becomes `my-label-done`).

### Issue Dependencies

The watcher can hold an issue back until the issues it depends on are **closed**, enforcing a serial order across a chain of related work. Dependencies are declared in the issue body and parsed by [`internal/watcher/dependencies.go`](internal/watcher/dependencies.go).

Parsing is **format-sensitive** — only the following patterns are recognized (all case-insensitive, and every reference must use `#<number>`):

| Format | Example |
|--------|---------|
| `Depends on Issue #N` | `Depends on Issue #314` |
| `Blocked by: #N` | `Blocked by: #314` |
| `Depends on [Issue #N]` | `Depends on [Issue #314]` |
| `Dependencies: #N, #M, ...` | `Dependencies: #315, #317` |

The `Dependencies:` form must be a **single line** — the numbers have to follow the `Dependencies:` keyword on the same line, comma-separated, each prefixed with `#`. A markdown bullet list under a `## Dependencies` header is **not** parsed:

```markdown
<!-- NOT parsed — the watcher sees zero dependencies and schedules immediately -->
## Dependencies
- #314 must merge first
- #317 must merge first

<!-- Parsed correctly -->
## Dependencies
Dependencies: #314, #317
```

A dependency is satisfied only once the referenced issue is **closed**. Any dependency that is still open — or that cannot be fetched — marks the dependent issue as blocked, so it stays out of the processing queue until every prerequisite closes.

### Retry Logic

The system has two layers of retry:

1. **Global retries** (watcher level) — Persisted in `.kairon/retries/issue-<number>.count`. The watcher skips issues that have reached `max_retries` attempts and survives process restarts.
2. **Per-agent retries** (manager level) — When an agent exits with a non-zero code, the manager retries with exponential backoff (delay = retry count × 1 second) up to `max_retries`.

After exhausting retries, the issue is labeled `<label>-failed`.

## Agent Configuration

Agent configs live in `.kiro/agents/`. Each agent has a JSON config and a prompt markdown file.

**krew-lead.json** (orchestrator):
```json
{
  "name": "krew-lead",
  "tools": ["read", "shell", "subagent", "todo_list"],
  "toolsSettings": {
    "subagent": {
      "trustedAgents": ["architect", "builder", "validator", "documenter"]
    }
  },
  "model": "claude-sonnet-4"
}
```

**builder.json** (worker):
```json
{
  "name": "builder",
  "description": "Focused engineering agent that executes ONE task at a time.",
  "prompt": "file://./builder-prompt.md",
  "tools": ["read", "write", "shell"],
  "allowedTools": ["read", "write", "shell"],
  "model": "claude-sonnet-4"
}
```

**validator.json** (read-only verifier):
```json
{
  "name": "validator",
  "description": "Read-only validation agent that verifies task completion.",
  "prompt": "file://./validator-prompt.md",
  "tools": ["read", "shell"],
  "allowedTools": ["read", "shell"],
  "toolsSettings": {
    "shell": { "autoAllowReadonly": true }
  },
  "model": "claude-sonnet-4"
}
```

## GitHub Integration

Kairon uses the `gh` CLI for all GitHub operations — no API tokens to configure. Ensure you're authenticated:

```bash
gh auth login
gh auth status
```

The system calls:
- `gh issue list` — poll for labeled issues
- `gh issue view` — read issue details
- `gh issue edit` — add labels (`kairon-done`, `kairon-failed`)
- `gh pr create` — create pull requests

## License

See [LICENSE](LICENSE).
