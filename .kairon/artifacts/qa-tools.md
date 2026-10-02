# QA Tools Discovery

**Project**: jbrinkman/kairon
**Discovered**: $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Formatting Checks
- `task fmt:check` — Taskfile.yml (Check code formatting - fails if unformatted)

## Linting Checks
- `task lint` — Taskfile.yml (Run linters and static analysis with go vet)

## Tests
- `task test` — Taskfile.yml (Run all tests with coverage and race detection)

## Template Sync Checks
- `task sync:check` — Taskfile.yml (Verify template-synchronized files match their live counterparts)

## Build Verification
- `task build` — Taskfile.yml (Build the application with version metadata)

## All QA Commands (execution order)
1. `task fmt:check`
2. `task sync:check`
3. `task lint`
4. `task test`
5. `task build`
