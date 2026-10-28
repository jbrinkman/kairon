# Issue #900: Cap retries

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/agent`: cap the attempts per issue" | `retry-cap` (`TestRetryCap`) |
| S2 | In Scope: "`docs/retries.md`: document the cap" | `retry-docs` |
| S3 | In Scope: "`.kairon/config.yaml`: a `max_attempts` setting" | `retry-config`, `retry-cap` |
| AC1 | "The third failed attempt stops the loop. Verify: `go test ./internal/agent/... -run TestRetryCap` exits 0." | `retry-cap` |
| AC2 | "The docs name the setting. Verify: `grep -c max_attempts docs/retries.md` prints at least `1`." | `retry-docs`; checked by `validate-all` |

## Deferred work

none

## Solution Approach

The loop counts attempts and stops at the cap. The cleanup runs in a Go `defer`
statement. The docs explain that a `TODO` or `follow-up` comment is not a cap.

```kiro-plan
version: "1.0"
tasks:
  - id: retry-config
    agent: builder
    description: Add the max_attempts setting
    dependencies: []
    acceptance_criteria: ["setting parses"]
    validation_commands: ["go test ./internal/config/..."]
  - id: retry-cap
    agent: builder
    description: Stop the loop at the cap
    dependencies: [retry-config]
    acceptance_criteria: ["TestRetryCap passes"]
    validation_commands: ["go test ./internal/agent/..."]
  - id: retry-docs
    agent: documenter
    description: Document the cap
    dependencies: [retry-cap]
    acceptance_criteria: ["docs name max_attempts"]
    validation_commands: ["grep -c max_attempts docs/retries.md"]
  - id: validate-all
    agent: validator
    description: Run every check
    dependencies: [retry-docs]
    acceptance_criteria: ["all checks pass"]
    validation_commands: ["go test ./..."]
```
