## Codebase Context

Kairon is a GitHub issue-driven AI orchestration system that transforms labeled issues into working code through coordinated AI agent collaboration.

### Project Structure
```
kairon/
├── cmd/kairon/            # Main CLI application
├── internal/
│   ├── tui/               # Terminal UI components
│   ├── manager/           # Agent management
│   ├── watcher/           # GitHub issue monitoring
│   └── eval/              # Evaluation framework
├── .kiro/
│   └── agents/            # Agent configurations
└── .kairon/
    ├── config.yaml        # Project configuration
    └── specs/             # Generated specifications
```

### Key Technologies
- Go 1.21+ with BubbleTea for TUI
- GitHub CLI for API integration
- YAML configuration files
- Git worktrees for isolation

### Agent Pipeline
1. Watcher detects labeled GitHub issues
2. Krew-Lead spawns and orchestrates other agents
3. Architect reads issue and creates specification
4. Builder implements the solution
5. Validator verifies the implementation
6. PR is automatically created

### Common Patterns
- All agents use kiro-cli for execution
- Git worktrees provide isolated environments
- Configuration lives in .kairon/config.yaml
- Specs are generated in .kairon/specs/
