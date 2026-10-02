# Kiro Krew

A GitHub issue-driven AI orchestration system that transforms labeled issues into working code through coordinated AI agent collaboration.

## How It Works

Kiro Krew watches a GitHub repository for issues with a configured label, then spawns AI agents to implement solutions automatically:

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
go install github.com/jbrinkman/kiro-krew@latest
```

### Download Prebuilt Binaries

#### Linux
```bash
# For AMD64 (x86_64)
curl -L https://github.com/jbrinkman/kiro-krew/releases/latest/download/kiro-krew-linux-amd64 -o kiro-krew
chmod +x kiro-krew
sudo mv kiro-krew /usr/local/bin/

# For ARM64 (aarch64)
curl -L https://github.com/jbrinkman/kiro-krew/releases/latest/download/kiro-krew-linux-arm64 -o kiro-krew
chmod +x kiro-krew
sudo mv kiro-krew /usr/local/bin/

# Check your architecture
uname -m  # x86_64 = AMD64, aarch64/arm64 = ARM64
```

### Build from Source

```bash
git clone https://github.com/jbrinkman/kiro-krew.git
cd kiro-krew

# Using Task (recommended)
task build

# Or using Go directly
go build ./cmd/kiro-krew
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
kiro-krew init
```

This creates:
- `.kiro-krew/config.yaml` — watcher configuration
- `.kiro-krew/scripts/` — worktree management scripts
- `.kiro/agents/` — agent configurations (krew-lead, architect, builder, validator, documenter)

### 2. Configure

Edit `.kiro-krew/config.yaml`:

```yaml
repo: owner/repo-name
label: kiro-krew
poll_interval: 5m
max_retries: 3
```

| Field | Description | Default |
|-------|-------------|---------|
| `repo` | GitHub repository (owner/name) | *required* |
| `label` | Issue label to watch for | `kiro-krew` |
| `poll_interval` | How often to poll GitHub | `5m` |
| `max_retries` | Max retry attempts per issue | `3` |

### 3. Run

```bash
kiro-krew
```

This starts the interactive REPL. From there, start the watcher:

```
kiro-krew> watch start
kiro-krew> status
```

## CLI Usage

```bash
# Display version and exit
kiro-krew --version

# Initialize project with agent configs and templates (skips existing files)
kiro-krew init

# Force-update templates (overwrites all files except config.yaml)
kiro-krew update

# Start interactive REPL (default when no arguments)
kiro-krew
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

- **Console Mode**: Main Kiro Krew interface for managing watchers and agents
- **Planning Mode**: Interactive AI-assisted issue creation and planning

Both modes preserve their state when you switch, allowing seamless workflow transitions. See [docs/hotkey-toggle.md](docs/hotkey-toggle.md) for detailed usage information.

## Architecture

### Agent Pipeline

When the watcher detects a labeled issue:

1. **Krew-Lead** — Orchestrates the workflow. Creates a git worktree, delegates to other agents, manages the lifecycle from issue to PR.
2. **Architect** — Reads the issue, explores the codebase, and produces a design specification at `.kiro-krew/specs/issue-<number>-<slug>.md`.
3. **Builder** — Implements code changes according to the architect's specification. Focused on a single task at a time.
4. **Validator** — Read-only agent that verifies the implementation meets acceptance criteria. Runs tests and checks.
5. **Documenter** — Generates documentation in `app_docs/` for completed features.

### Agent Spawning

The manager spawns agents as `kiro-cli` processes:

```
kiro-cli chat --agent krew-lead --no-interactive --trust-all-tools "Process issue #N from repo owner/name. Worktree name: issue-N-<pid>"
```

Each agent runs with environment variables: `ISSUE_NUMBER`, `REPO`, and `KIRO_KREW_WATCHER_PID`.

### Git Worktree Isolation

Each issue is processed in an isolated git worktree named `issue-<number>-<pid>` (where `<pid>` is the watcher process ID):
- `.kiro-krew/scripts/worktree-create.sh <name>` — creates `.worktrees/<name>/` on branch `spec/<name>`
- `.kiro-krew/scripts/worktree-merge.sh <name>` — merges back, removes worktree, deletes branch
- Orphaned worktrees (from crashed processes) are cleaned up automatically by checking if the PID is still running

### Issue Lifecycle

| State | Label | Description |
|-------|-------|-------------|
| Ready | `kiro-krew` | Watcher will pick up this issue |
| Processing | — | Agent spawned and working |
| Done | `kiro-krew-done` | PR created successfully |
| Failed | `kiro-krew-failed` | Exhausted retries |

Issues with `kiro-krew-done` or `kiro-krew-failed` labels are excluded from polling. The done/failed labels are derived from the configured label (e.g., if label is `my-label`, done becomes `my-label-done`).

### Retry Logic

The system has two layers of retry:

1. **Global retries** (watcher level) — Persisted in `.kiro-krew/retries/issue-<number>.count`. The watcher skips issues that have reached `max_retries` attempts and survives process restarts.
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

Kiro Krew uses the `gh` CLI for all GitHub operations — no API tokens to configure. Ensure you're authenticated:

```bash
gh auth login
gh auth status
```

The system calls:
- `gh issue list` — poll for labeled issues
- `gh issue view` — read issue details
- `gh issue edit` — add labels (`kiro-krew-done`, `kiro-krew-failed`)
- `gh pr create` — create pull requests

## License

See [LICENSE](LICENSE).
