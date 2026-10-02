# Agent Conventions Skills

Agent conventions skills allow you to create project-specific customizations for Kairon agents without modifying core agent configurations. This system enables teams to define coding standards, patterns, and workflows that agents will follow automatically.

## How It Works

Kairon agents can reference optional "conventions skills" through their manifest files. These skills contain project-specific instructions that supplement the agent's base behavior:

```json
{
  "name": "builder",
  "description": "Focused engineering agent that executes ONE task at a time.",
  "prompt": "file://./builder-prompt.md",
  "resources": ["skill://.kairon/skills/builder-conventions/SKILL.md"],
  "tools": ["read", "write", "shell"]
}
```

The `resources` array contains skill URI references that agents load at runtime. If the skill files don't exist, agents continue to function normally - the system gracefully degrades without errors.

## Creating Project Conventions

### 1. Create the Skill Directory

Conventions skills live in `.kairon/skills/` and follow the naming pattern `{agent-name}-conventions`:

```bash
mkdir -p .kairon/skills/builder-conventions
```

### 2. Write the Skill File

Create `SKILL.md` with YAML frontmatter and markdown content:

```yaml
---
name: builder-conventions
description: Project-specific conventions and patterns for the builder agent.
---

# Builder Conventions

Project-specific instructions for the builder agent...

## Code Standards
- Use tabs for indentation (Go project standard)
- Run `gofmt` before committing
- Follow project naming conventions for variables

## Testing Requirements
- All new functions must have unit tests
- Integration tests required for API changes
- Run `go test ./...` before completion

## File Organization
- Place new utilities in `internal/utils/`
- API handlers go in `internal/handlers/`
- Tests alongside source files with `_test.go` suffix
```

### 3. Available Agent Types

You can create conventions for any agent:

- `architect-conventions` - Design and planning standards
- `builder-conventions` - Implementation and coding standards  
- `validator-conventions` - Testing and quality requirements
- `documenter-conventions` - Documentation format and style
- `krew-lead-conventions` - Workflow coordination patterns
- `planner-conventions` - Issue planning and structuring

## Template Synchronization

The `builder-conventions` skill includes special instructions for maintaining synchronization between template files and live project files. This ensures that changes to configuration files are reflected in both the active project and the templates used for `kairon init`.

### Synchronized File Types

When the builder agent modifies these file types, it automatically updates both versions:

| File Type | Live Location | Template Location |
|-----------|---------------|-------------------|
| Agent Manifests | `.kiro/agents/*.json` | `cmd/kairon/templates/kiro/agents/*.json` |
| Evaluation Cases | `.kairon/evals/cases/*, .kairon/evals/rubrics/*` | `cmd/kairon/templates/kairon/evals/cases/*, cmd/kairon/templates/kairon/evals/rubrics/*` |
| Scripts | `.kairon/scripts/**/*` | `cmd/kairon/templates/kairon/scripts/**/*` |
| Themes | `.kairon/themes/**/*` | `cmd/kairon/templates/kairon/themes/**/*` |

### Sync Verification

The builder-conventions skill provides commands to verify synchronization:

```bash
# Check agent manifest sync
for agent in architect builder documenter krew-lead planner validator; do
  diff .kairon/agents/$agent.json cmd/kairon/templates/kairon/agents/$agent.json
done

# Verify script functionality
.kairon/scripts/worktree-create.sh test-worktree
.kairon/scripts/worktree-merge.sh test-worktree
```

## Best Practices

### Keep It Focused
Each conventions skill should contain only instructions relevant to that specific agent:

```yaml
# ✅ Good - builder-specific
## Implementation Patterns
- Use builder pattern for complex object creation
- Implement error wrapping with fmt.Errorf
- Add logging at INFO level for major operations

# ❌ Avoid - planning concerns in builder skill  
## Issue Planning
- Break large features into smaller issues
- Use acceptance criteria format...
```

### Use Actionable Instructions
Write specific, actionable guidance rather than general principles:

```yaml
# ✅ Good - specific actions
## Error Handling
Run `go vet ./...` before marking tasks complete.
Wrap errors with context: `fmt.Errorf("failed to parse config: %w", err)`

# ❌ Avoid - vague guidance
## Quality
Write good code that follows best practices.
```

### Include Verification Commands
Provide commands agents can run to verify compliance:

```yaml
## Quality Assurance
Before completion, run these verification commands:
- `golangci-lint run ./...` - Code linting
- `go test -race ./...` - Tests with race detection  
- `go mod tidy` - Clean up dependencies
```

### Project-Specific Patterns
Document patterns specific to your codebase:

```yaml
## Database Queries
Use the project's query builder pattern:
```go
result, err := db.Query().
    Select("users").
    Where("active = ?", true).
    OrderBy("created_at DESC").
    Execute()
```

## Graceful Degradation

The conventions skills system is designed to be optional and non-breaking:

- **Missing Skills**: Agents function normally when conventions skills don't exist
- **Invalid Skills**: Malformed skill files are ignored without stopping agents
- **Partial Implementation**: You can implement conventions for only some agents
- **Gradual Adoption**: Add conventions incrementally as your project evolves

This ensures that existing Kiro Krew projects continue working without modification, while new projects can benefit from project-specific customizations.

## Examples

### Python Project Conventions

```yaml
---
name: builder-conventions
description: Python project standards and patterns.
---

# Python Builder Conventions

## Code Style
- Use Black formatter: `black .`
- Sort imports with isort: `isort .`
- Type hints required for all functions
- Docstrings required for public methods (Google style)

## Testing
- pytest for all tests
- Minimum 90% coverage: `pytest --cov=src --cov-report=term-missing`
- Tests in `tests/` directory with `test_*.py` naming

## Dependencies
- Use Poetry for dependency management
- Pin dependencies in pyproject.toml
- Run `poetry check` before completion

## Quality Gates
All commands must pass before task completion:
- `black --check .`
- `isort --check-only .`
- `flake8 .`
- `mypy src/`
- `pytest --cov=src --cov-fail-under=90`
```

### React Project Conventions

```yaml
---
name: builder-conventions
description: React project standards and component patterns.
---

# React Builder Conventions

## Component Standards
- Use functional components with hooks
- TypeScript required for all components
- Props interfaces defined with proper JSDoc
- Export components as default from index files

## File Structure
- Components in `src/components/{ComponentName}/`
- Tests co-located: `ComponentName.test.tsx`
- Styles in same directory: `ComponentName.module.css`

## Quality Checks
- `npm run lint` - ESLint validation
- `npm run type-check` - TypeScript compilation
- `npm test` - Jest test suite
- `npm run build` - Production build verification

## Testing Requirements
- React Testing Library for component tests
- Mock external dependencies
- Test user interactions, not implementation details
```

## Migration Guide

To add conventions to an existing project:

1. **Create skill directories**: `mkdir -p .kairon/skills/{agent-name}-conventions`
2. **Write skill files**: Start with your most critical standards
3. **Test gracefully**: Existing workflows continue unchanged
4. **Iterate**: Add more conventions as agents use them
5. **Share**: Commit conventions to version control for team consistency

The conventions skills system grows with your project, providing increasingly sophisticated customization as your codebase and team practices mature.