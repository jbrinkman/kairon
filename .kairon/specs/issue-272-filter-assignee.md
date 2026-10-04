# Design Specification: Filter Watcher Issues by Assignee

**Issue**: #272  
**Title**: Filter watcher issues by assignee  
**Closes**: #272

## Problem Statement

The watcher currently picks up any open issue with the configured label (e.g., `kairon`), regardless of who (if anyone) it's assigned to. This creates several challenges:

1. **Multi-user collision**: Multiple team members running watchers simultaneously process the same issues
2. **Workspace conflicts**: Parallel processing attempts create git worktree conflicts
3. **Lack of control**: Users cannot specify which issues their local watcher should handle
4. **Team coordination overhead**: Manual coordination required to prevent duplicate work

**Current Behavior:**
```
GitHub Issue with label "kairon" → Watcher detects → Always processes
```

**User Story:**  
As a kiro-krew user working in a shared repository, I want the watcher to only process issues assigned to a specific GitHub user, so that multiple team members can run watchers simultaneously without colliding on the same issues.

## Solution Approach

Add assignee-based filtering to the watcher with intelligent user detection:

1. **Optional Config Field**: Add `user` field to `.kairon/config.yaml`
2. **Auto-detection**: If `user` not configured, detect authenticated user via `gh api user`
3. **Filter Integration**: Pass assignee to GitHub client for server-side filtering
4. **Logging Enhancement**: Display which user the watcher is filtering by on startup
5. **Error Handling**: Gracefully handle authentication failures and API errors
6. **Backward Compatibility**: Existing configs without `user` field work seamlessly (auto-detect)

**Proposed Flow:**
```
Config Load → User Detection (explicit or auto) → Filter by Assignee → Process Assigned Issues
```

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│ .kairon/config.yaml                                          │
│   user: "username" (optional)                                │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│ Watcher Initialization                                       │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ User Detection:                                          │ │
│ │ if cfg.User != "" → use cfg.User                        │ │
│ │ else → GetAuthenticatedUser() via gh api user          │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ Log: "polling owner/repo for label 'kairon' assigned to    │
│      <username>"                                             │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│ Poll Loop: checkIssues()                                     │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ Call: ListIssues(repo, label, assignee)                 │ │
│ │ Returns: Only issues assigned to specified user         │ │
│ └──────────────────────────────────────────────────────────┘ │
└────────────────┬─────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────────────────┐
│ GitHub Client: internal/github/client.go                     │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ GetAuthenticatedUser() → gh api user                    │ │
│ │   Returns: {login: "username"}                          │ │
│ └──────────────────────────────────────────────────────────┘ │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ ListIssues(repo, label, assignee)                       │ │
│ │   If assignee != "":                                    │ │
│ │     gh issue list --assignee <user> ...                │ │
│ │   Else:                                                 │ │
│ │     gh issue list ... (all issues)                     │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

## Relevant Files

### Files to Modify

1. **internal/config/config.go**
   - Add `User string` field to `Config` struct
   - Purpose: Store optional assignee username for filtering

2. **internal/github/client.go**
   - Add `GetAuthenticatedUser()` function
   - Modify `ListIssues()` signature to accept optional `assignee` parameter
   - Purpose: Extend GitHub API integration with user detection and assignee filtering

3. **internal/watcher/watcher.go**
   - Add `user string` field to `Watcher` struct
   - Modify `New()` constructor to detect/validate user
   - Update `Start()` to log the assignee filter in startup message
   - Modify `checkIssues()` to pass assignee to `ListIssues()`
   - Purpose: Integrate assignee filtering into watcher lifecycle

### Files Referenced (No Changes)

4. **.kairon/config.yaml** (user's config file)
   - Will optionally include `user: "username"` field
   - Backward compatible: absence of field triggers auto-detection

## Team Orchestration

This is a single-PR feature implementation with sequential dependencies:

### Task Execution Strategy

Tasks must execute sequentially due to dependencies:

```
Task 1: Extend GitHub Client
    ↓
Task 2: Update Config Schema
    ↓
Task 3: Integrate Watcher Filtering
    ↓
Task 4: Validation
```

## Step-by-Step Task Breakdown

### Task 1: Extend GitHub Client with User Detection and Assignee Filtering

**Agent**: builder  
**Files**: `internal/github/client.go`

**Acceptance Criteria**:
- Add `GetAuthenticatedUser()` function that:
  - Executes `gh api user --jq '.login'`
  - Returns the authenticated user's login (username) as a string
  - Returns error if `gh` command fails or output is empty
  - Includes descriptive error messages (e.g., "failed to get authenticated user: <reason>")
- Modify `ListIssues()` function signature from:
  ```go
  func ListIssues(repo, label string) ([]Issue, error)
  ```
  to:
  ```go
  func ListIssues(repo, label, assignee string) ([]Issue, error)
  ```
- Update `ListIssues()` implementation to:
  - If `assignee != ""`, add `--assignee <assignee>` flag to `gh issue list` command
  - If `assignee == ""`, omit the assignee flag (all issues, backward compatible)
  - Preserve existing filtering logic (exclude `-done` and `-failed` labels)
- All existing error handling (rate limiting, JSON parsing) remains unchanged
- Function maintains backward compatibility when `assignee` is empty string

**Implementation Notes**:
- `gh api user` returns JSON: `{"login":"username", ...}` — use `--jq '.login'` to extract just the username
- `gh issue list --assignee <user>` performs server-side filtering, reducing unnecessary data transfer
- Empty `assignee` parameter allows gradual rollout without breaking existing callers

**Validation Commands**:
```bash
go build ./internal/github
go test ./internal/github -run TestGetAuthenticatedUser -v
go test ./internal/github -run TestListIssues -v
```

**Dependencies**: None (foundational task)

---

### Task 2: Update Config Schema to Support User Field

**Agent**: builder  
**Files**: `internal/config/config.go`, `.kairon/config.yaml` (template comment)

**Acceptance Criteria**:
- Add `User string` field to `Config` struct with YAML tag:
  ```go
  User string `yaml:"user"`
  ```
- Position the field logically with other watcher-related fields (near `Repo`, `Label`)
- No validation required (empty string is valid and triggers auto-detection)
- `Load()` function automatically populates field from YAML if present
- `Save()` function preserves the field if it exists
- Config struct remains backward compatible (unmarshaling YAML without `user` field leaves it as empty string)

**Implementation Notes**:
- Optional field — absence means auto-detect, presence means explicit filter
- No default value needed (empty string has semantic meaning: auto-detect)
- Consider adding inline comment in struct definition: `// Optional GitHub username for issue filtering (auto-detects if empty)`

**Validation Commands**:
```bash
go build ./internal/config
go test ./internal/config -v
```

**Dependencies**: None (can run in parallel with Task 1, but Task 3 requires both)

---

### Task 3: Integrate Assignee Filtering into Watcher

**Agent**: builder  
**Files**: `internal/watcher/watcher.go`

**Acceptance Criteria**:
- Add `user string` field to `Watcher` struct (stores the resolved username)
- Modify `New()` constructor to:
  - Accept the resolved username (either from config or auto-detected)
  - Store it in the `user` field
- Update `Start()` method to:
  - Log startup message with assignee filter: `[watcher] started — polling <repo> every <interval> for label "<label>" assigned to <user>`
  - Example: `[watcher] started — polling jbrinkman/kairon every 1m for label "kairon" assigned to octocat`
- Modify `checkIssues()` method to:
  - Call `github.ListIssues(w.config.Repo, w.config.Label, w.user)` (pass assignee)
  - All other logic (dependency validation, retry checks, worktree checks) remains unchanged
- **Critical**: Implement user detection in the initialization path (before watcher creation):
  - If `cfg.User != ""`, use `cfg.User` directly
  - If `cfg.User == ""`, call `github.GetAuthenticatedUser()` and handle errors:
    - If error occurs, log error: `[watcher] failed to auto-detect user: <error>`
    - Do not start the watcher (return error or exit gracefully)
    - Suggest explicit `user` configuration if auto-detection fails
- User detection happens **once** during watcher initialization, not on every poll

**Implementation Notes**:
- User detection belongs in the initialization logic (e.g., in `cmd/kairon/cmd/root.go` or `internal/tui/commands.go` where watcher is created), not in `watcher.New()` itself
- The watcher should receive a validated, non-empty username — it doesn't handle detection logic
- Error handling for auto-detection failure should prevent watcher startup (fail-fast)
- Logging format should clearly indicate the assignee filter is active

**Validation Commands**:
```bash
go build ./internal/watcher
go test ./internal/watcher -v
go build ./cmd/kairon
```

**Dependencies**: Task 1 (requires `github.GetAuthenticatedUser()` and updated `ListIssues()`), Task 2 (requires `Config.User` field)

---

### Task 4: End-to-End Validation

**Agent**: validator  
**Files**: All modified files

**Acceptance Criteria**:
- **Build Validation**:
  - `task build` succeeds without errors
  - No compilation warnings introduced
  - Binary runs without panics
- **Unit Test Validation**:
  - `go test ./internal/config -v` passes
  - `go test ./internal/github -v` passes
  - `go test ./internal/watcher -v` passes
- **Integration Validation** (manual or scripted):
  - **Scenario 1: Explicit User Configuration**
    - Set `user: "valid-github-username"` in `.kairon/config.yaml`
    - Run `kairon`, execute `watch start`
    - Verify log message: `[watcher] started — polling <repo> every <interval> for label "<label>" assigned to valid-github-username`
    - Verify only issues assigned to `valid-github-username` are processed
  - **Scenario 2: Auto-detection (Empty User Field)**
    - Remove or comment out `user` field in `.kairon/config.yaml`
    - Ensure `gh auth status` shows authenticated user
    - Run `kairon`, execute `watch start`
    - Verify log message: `[watcher] started — polling <repo> every <interval> for label "<label>" assigned to <detected-username>`
    - Verify only issues assigned to detected user are processed
  - **Scenario 3: Auto-detection Failure**
    - Remove or comment out `user` field in `.kairon/config.yaml`
    - Simulate `gh api user` failure (e.g., `gh auth logout`)
    - Run `kairon`, execute `watch start`
    - Verify error logged: `[watcher] failed to auto-detect user: <error>`
    - Verify watcher does not start
  - **Scenario 4: Backward Compatibility**
    - Use existing `.kairon/config.yaml` without `user` field
    - Ensure authenticated `gh` session exists
    - Verify watcher starts successfully with auto-detected user
- **GitHub API Behavior Validation**:
  - Confirm `gh issue list --assignee <user>` returns correct filtered results
  - Verify issues without assignees are excluded
  - Verify issues assigned to other users are excluded
- **No Regressions**:
  - Existing watcher features (retry logic, dependency validation, worktree cleanup) continue to work
  - Existing logging, polling interval, label filtering remain functional

**Validation Commands**:
```bash
# Build validation
task build

# Unit test validation
go test ./internal/config -v
go test ./internal/github -v
go test ./internal/watcher -v

# Full test suite
task test

# Integration validation (manual)
# 1. Test with explicit user:
echo 'user: "octocat"' >> .kairon/config.yaml
./kairon
# In REPL: watch start
# Verify log output and issue filtering

# 2. Test with auto-detection:
sed -i '' '/^user:/d' .kairon/config.yaml  # Remove user field
./kairon
# In REPL: watch start
# Verify log output and issue filtering

# 3. Test auto-detection failure:
gh auth logout
./kairon
# In REPL: watch start
# Verify error handling
gh auth login

# 4. GitHub API validation:
gh issue list --repo jbrinkman/kairon --label kairon --assignee octocat
```

**Dependencies**: Task 1, Task 2, Task 3 (validates complete integration)

---

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "extend-github-client"
    agent: "builder"
    description: "Extend GitHub client with GetAuthenticatedUser() and update ListIssues() to support assignee filtering"
    dependencies: []
    acceptance_criteria:
      - "GetAuthenticatedUser() function added that calls 'gh api user --jq .login'"
      - "GetAuthenticatedUser() returns username string and error"
      - "ListIssues() signature updated to accept assignee parameter"
      - "ListIssues() adds --assignee flag when assignee is non-empty"
      - "ListIssues() maintains backward compatibility when assignee is empty"
      - "Existing error handling and filtering logic preserved"
    validation_commands:
      - "go build ./internal/github"
      - "go test ./internal/github -v"

  - id: "update-config-schema"
    agent: "builder"
    description: "Add optional User field to Config struct for assignee filtering"
    dependencies: []
    acceptance_criteria:
      - "User string field added to Config struct with yaml tag"
      - "Field positioned near other watcher config fields (Repo, Label)"
      - "Load() and Save() functions handle field correctly"
      - "Backward compatible with existing configs (empty string when absent)"
    validation_commands:
      - "go build ./internal/config"
      - "go test ./internal/config -v"

  - id: "integrate-watcher-filtering"
    agent: "builder"
    description: "Integrate assignee filtering into watcher with user detection and logging"
    dependencies: ["extend-github-client", "update-config-schema"]
    acceptance_criteria:
      - "Watcher struct includes user field to store resolved username"
      - "Watcher initialization includes user detection logic (explicit or auto-detect)"
      - "Start() logs assignee filter in startup message"
      - "checkIssues() passes user to ListIssues() for filtering"
      - "Auto-detection failure prevents watcher startup with clear error message"
      - "User detection occurs once during initialization, not on every poll"
    validation_commands:
      - "go build ./internal/watcher"
      - "go test ./internal/watcher -v"
      - "go build ./cmd/kairon"

  - id: "validate-complete"
    agent: "validator"
    description: "Verify complete assignee filtering implementation meets all acceptance criteria"
    dependencies: ["integrate-watcher-filtering"]
    acceptance_criteria:
      - "All unit tests pass"
      - "Build succeeds without errors or warnings"
      - "Explicit user configuration works correctly"
      - "Auto-detection works when user field absent"
      - "Auto-detection failure handled gracefully"
      - "Backward compatibility preserved"
      - "GitHub API filtering behavior validated"
      - "No regressions in existing watcher functionality"
    validation_commands:
      - "task build"
      - "task test"
      - "go test ./internal/config -v"
      - "go test ./internal/github -v"
      - "go test ./internal/watcher -v"
```

## Error Handling Strategy

### Auto-Detection Failures

**Scenario**: `gh api user` fails (not authenticated, network error, API unavailable)

**Handling**:
1. Log clear error message: `[watcher] failed to auto-detect user: <error details>`
2. Do not start the watcher (fail-fast approach)
3. Suggest explicit configuration: "Set 'user' field in .kairon/config.yaml or ensure 'gh auth login' is configured"
4. Return error from initialization function (prevents watcher startup)

**Rationale**: Silent failures lead to unexpected behavior (processing all issues). Explicit failure is safer.

### API Filtering Failures

**Scenario**: `gh issue list --assignee <user>` fails during polling

**Handling**:
1. Treat like existing GitHub API errors (log and continue)
2. Existing error handling in `checkIssues()`: `log.Printf("[watcher] error fetching issues: %v", err)`
3. Retry on next polling interval
4. Rate limiting detection remains functional

**Rationale**: Transient API errors shouldn't crash the watcher. Existing retry logic handles this.

### Invalid User Configuration

**Scenario**: User configures `user: "nonexistent-user"` in config

**Handling**:
1. GitHub API returns empty result set (no issues assigned to nonexistent user)
2. Watcher logs: `[watcher] polling for issues...` (finds zero issues)
3. No special handling needed — behaves like no assigned issues exist

**Rationale**: GitHub API gracefully handles invalid usernames (returns empty). Over-validation adds complexity.

## Backward Compatibility

### Existing Configs Without `user` Field

**Behavior**: Auto-detection activates automatically
- Config loads successfully (Go zero value for string is `""`)
- Watcher initialization detects empty `user` field
- Calls `github.GetAuthenticatedUser()` to populate
- Proceeds with detected user

**Migration**: None required — seamless upgrade

### Existing `github.ListIssues()` Callers

**Behavior**: Function signature changes, requires update
- Current signature: `ListIssues(repo, label string)`
- New signature: `ListIssues(repo, label, assignee string)`
- **Action Required**: Update all callers to pass empty string `""` for backward compatible behavior

**Known Callers**:
1. `internal/watcher/watcher.go` — will be updated in Task 3
2. Any test files — will be updated in respective tasks

**Migration Strategy**: Pass `""` as third parameter to maintain current behavior (no assignee filtering)

## Constraints and Assumptions

### Constraints

1. **GitHub CLI Dependency**: All GitHub interactions use `gh` CLI (no direct API calls)
2. **Username Only**: Config stores GitHub login/username, not email or display name
3. **No Label Logic Changes**: Assignee filtering is additive, doesn't modify existing label/retry/dependency logic
4. **Single Assignee**: GitHub issues support multiple assignees, but `gh issue list --assignee` filters by any assigned user (inclusive)

### Assumptions

1. **Authenticated Session**: Users running Kairon have active `gh auth login` session
2. **Assignee Permissions**: Users can see issues assigned to them in the repository
3. **Consistent Username**: `gh api user` returns the same username across invocations (no identity switching mid-session)
4. **Server-Side Filtering**: GitHub API `--assignee` flag performs efficient server-side filtering (doesn't fetch all issues then filter locally)

## Testing Strategy

### Unit Tests

**internal/github/client.go**:
- Test `GetAuthenticatedUser()` success (mock `gh api user` output)
- Test `GetAuthenticatedUser()` failure (simulate command error)
- Test `ListIssues()` with assignee parameter (verify `--assignee` flag added)
- Test `ListIssues()` without assignee (verify backward compatibility)

**internal/config/config.go**:
- Test config loading with `user` field present
- Test config loading without `user` field (backward compatibility)
- Test config saving preserves `user` field

**internal/watcher/watcher.go**:
- Test watcher initialization with explicit user
- Test watcher initialization with auto-detection (mock `GetAuthenticatedUser()`)
- Test watcher startup logging includes assignee
- Test `checkIssues()` passes user to `ListIssues()`

### Integration Tests

**End-to-End Scenarios**:
1. **Explicit User**: Set `user` in config, verify correct filtering
2. **Auto-detection**: Remove `user` from config, verify detection and filtering
3. **Detection Failure**: Simulate auth failure, verify error handling
4. **Multi-user Scenario**: Two watchers with different users, verify no collision

### Manual Verification

**GitHub Repository Setup**:
1. Create test issues assigned to different users
2. Run watcher with various config combinations
3. Verify only assigned issues are processed
4. Check git worktrees created match assigned issues only

## Security Considerations

### Token Exposure

**Risk**: `gh api user` uses stored GitHub token (from `gh auth login`)

**Mitigation**:
- No token logging or display in code
- Use `gh` CLI's built-in token management (secure storage)
- Error messages don't expose token values

### Username Validation

**Risk**: Malicious username in config could cause command injection

**Mitigation**:
- `gh` CLI handles username sanitization internally
- No shell interpolation — `gh` command uses `exec.Command()` with separate arguments
- Username passed as separate argument, not concatenated into command string

### API Rate Limiting

**Risk**: Additional `gh api user` call consumes rate limit quota

**Mitigation**:
- Auto-detection happens once during startup, not on every poll
- Minimal impact: 1 API call per watcher startup
- Explicit `user` configuration avoids API call entirely

## Future Enhancements

### Out of Scope (Not Included in This PR)

1. **Multi-Assignee Support**: Filter issues assigned to multiple specific users
   - Requires config schema change to accept list of users
   - Complexity: OR logic in filtering
2. **Assignee Wildcards**: Support patterns like `@team-name` or `@org/*`
   - Requires GitHub team/org API integration
   - Complexity: resolving team members
3. **Dynamic Assignee Switching**: Change assignee filter without restarting watcher
   - Requires REPL command: `watch assignee <user>`
   - Complexity: thread-safe config updates
4. **Assignee Validation**: Verify username exists before starting watcher
   - Requires additional GitHub API call
   - Complexity: error handling for invalid usernames

### Potential Follow-Up Issues

- **Issue #TBD**: Support filtering by multiple assignees
- **Issue #TBD**: Add `watch assignee` REPL command for runtime changes
- **Issue #TBD**: Team-based filtering with `@team-name` syntax

## References

- **GitHub CLI Documentation**: https://cli.github.com/manual/gh_issue_list
- **GitHub API - List Issues**: https://docs.github.com/en/rest/issues/issues#list-repository-issues
- **Acceptance Criteria Source**: Issue #272 body (embedded in this spec's Problem Statement)
- **Existing Architecture**: `internal/watcher/watcher.go` (current polling implementation)
- **Related Specs**: None (new feature, no dependencies on other specs)
