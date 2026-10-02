---
name: discover-qa-tools
description: Discover project-specific QA tools from CI configs and build files. Use when starting a QA workflow to identify formatting, linting, and testing commands available in the project.
---

# Discover QA Tools

Discover all quality assurance commands available in the current project by examining CI/CD configuration and build tool files.

## Process

1. **Check CI/CD configuration files** for quality-related steps:
   - `.github/workflows/*.yml`, `.github/workflows/*.yaml`
   - `.gitlab-ci.yml`
   - `Jenkinsfile`
   - `.circleci/config.yml`

2. **Check build tool configuration** for quality-related tasks/scripts:
   - `Taskfile.yml` (task names and commands)
   - `Makefile` (target names)
   - `package.json` (scripts section)
   - `pyproject.toml` (tool sections)
   - `tox.ini` (environments)

3. **Extract commands** that perform:
   - Formatting checks (check-only/verify mode)
   - Linting / static analysis
   - Testing (unit, integration)
   - Type checking

4. **Map to local equivalents**: CI commands should be mapped to locally runnable commands. Prefer build tool wrappers (e.g., `task lint`) over raw commands when available.

## Output Format

Write the discovery results to `.kairon/artifacts/qa-tools.md`:

```markdown
# QA Tools Discovery

**Project**: [repo name]
**Discovered**: [timestamp]

## Formatting Checks
- `[command]` — [source file where found]

## Linting Checks
- `[command]` — [source file where found]

## Tests
- `[command]` — [source file where found]

## All QA Commands (execution order)
1. `[formatting command]`
2. `[linting command]`
3. `[test command]`
```

## Caching

The output file at `.kairon/artifacts/qa-tools.md` serves as a cache. It should be regenerated if:
- The file does not exist
- The file is older than 24 hours
- CI configuration files have been modified since the last discovery

## Usage

Krew-lead invokes this skill once before the QA loop and passes the resulting command list to both builder and validator agents as context.
