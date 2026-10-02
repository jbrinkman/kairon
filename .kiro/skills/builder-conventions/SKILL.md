---
name: builder-conventions
description: Project-specific conventions and patterns for the builder agent. Maintains synchronization between template and live project files.
---

# Builder Conventions

Project-specific conventions, patterns, and best practices for the builder agent to follow during implementation tasks.

## Mandatory Template Synchronization

**Critical for Self-Hosting**: Kiro Krew uses itself to build itself. Live project files and embedded templates must stay in sync so that `kiro-krew init` and `kiro-krew update` always deploy current configurations.

Sync is **one-way** (live → template). CI enforces this via `task sync:check` in the Validate PR workflow.

### Sync Mappings

| Live Path | Template Path |
|-----------|---------------|
| `.kiro/agents/*.json` | `cmd/kiro-krew/templates/kiro/agents/` |
| `.kiro/agents/*.md` | `cmd/kiro-krew/templates/kiro/agents/` |
| `.kiro-krew/scripts/*.sh` | `cmd/kiro-krew/templates/kiro-krew/scripts/` |
| `.kiro-krew/themes/*.yaml` | `cmd/kiro-krew/templates/kiro-krew/themes/` |
| `.kiro-krew/evals/fixtures/*` | `cmd/kiro-krew/templates/kiro-krew/evals/fixtures/` |
| `.kiro-krew/evals/rubrics/*` | `cmd/kiro-krew/templates/kiro-krew/evals/rubrics/` |
| `.kiro-krew/evals/cases/**/*` | `cmd/kiro-krew/templates/kiro-krew/evals/cases/` |
| `.kiro/skills/sentinel-protocol/*` | `cmd/kiro-krew/templates/kiro/skills/sentinel-protocol/` |

### Exclusion Patterns

**Never sync `*-conventions` skills** — they are project-specific and must NOT be distributed in templates.

### Sync Commands

Run the appropriate commands after modifying any template-synchronized files:

```bash
# Agent files (JSON configs and prompt files)
cp .kiro/agents/*.json cmd/kiro-krew/templates/kiro/agents/
cp .kiro/agents/*.md cmd/kiro-krew/templates/kiro/agents/

# Scripts
cp .kiro-krew/scripts/*.sh cmd/kiro-krew/templates/kiro-krew/scripts/

# Themes
cp .kiro-krew/themes/*.yaml cmd/kiro-krew/templates/kiro-krew/themes/

# Evals (excluding results directory)
cp .kiro-krew/evals/fixtures/* cmd/kiro-krew/templates/kiro-krew/evals/fixtures/
cp .kiro-krew/evals/rubrics/* cmd/kiro-krew/templates/kiro-krew/evals/rubrics/
mkdir -p cmd/kiro-krew/templates/kiro-krew/evals/cases/
cp -r .kiro-krew/evals/cases/* cmd/kiro-krew/templates/kiro-krew/evals/cases/
```

### Verification

Run `task sync:check` to verify all template-synchronized files match. This is the same check CI runs — if it passes locally, CI will pass.

```bash
task sync:check
```

If verification fails, re-run the sync commands above for the affected file category, then re-run `task sync:check`.

## Workflow Integration

When completing tasks that modify template-synchronized files, follow this sequence:

1. **Implement** — complete the assigned task
2. **Sync** — run the appropriate sync commands from above
3. **Verify** — run `task sync:check` (task cannot be marked complete if this fails)
4. **QA** — run `task lint` and `task test`
5. **Complete** — create sentinel file documenting results

### Sentinel File Requirements

Include sync verification status in sentinel files:

```markdown
## Task Complete

**Template Sync**: ✅ VERIFIED (or "N/A - no template files modified")
**QA Results**:
- Linting: ✅ PASS
- Tests: ✅ PASS
- Sync Verification: ✅ PASS

**Sync Commands Used**:
- `cp .kiro/agents/builder.json cmd/kiro-krew/templates/kiro/agents/`
```

## Implementation Patterns

### Quality Assurance
- Run ALL discovered QA commands before completion
- Use QA discovery results from `.kiro-krew/artifacts/qa-tools.md`
- Document specific QA command sources (CI vs build tool)

### File Modifications
- Preserve existing formatting and structure
- Maintain JSON validity for configuration files
- Verify template sync before task completion
- Document changes in sentinel files including sync status

### Error Recovery
- Address validator feedback from `.kiro-krew/artifacts/validator-<issue>.md`
- Focus on specific failing commands identified by validator
- Include sync verification in error recovery process
- Document how feedback was incorporated

## Project Standards

### Code Quality
- Follow existing code style and conventions
- Use project's configured linting and formatting tools
- Ensure all tests pass (100% pass rate required)

### Documentation
- Update relevant docs when adding features
- Follow project's documentation format
- Include usage examples for new functionality
