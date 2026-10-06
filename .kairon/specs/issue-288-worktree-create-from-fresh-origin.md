# Issue #288: worktree-create.sh must branch from freshly-fetched origin/<integration-branch>, not stale local HEAD

Closes #288

## Problem

`.kairon/scripts/worktree-create.sh` runs:

```bash
OUTPUT=$(git worktree add "$WORKTREE_PATH" -b "$BRANCH_NAME" 2>&1)
```

There is no `git fetch` and no explicit start-point, so the new `spec/issue-N-<pid>` branch is cut from
whatever the main checkout's local `HEAD` happens to be. The watcher runs in a long-lived main checkout
that is never fast-forwarded, so an issue picked up after other PRs merged yields a branch that is behind
`origin/main`. The resulting PR conflicts and/or re-does work that already landed.

The script is invoked from exactly two places, both in `internal/agent/manager.go` (initial spawn ~L191 and
retry ~L457), always with cwd = the main repo root and a single arg `issue-<N>-<pid>`. Agents run *inside* the
worktree afterwards and are told to skip worktree creation, so fixing the script fixes the whole pipeline.

## Solution Approach

Change only the start-point selection in `worktree-create.sh`; keep its CLI contract (arg, stdout = absolute
worktree path, logs on stderr, exit codes, idempotency, stale-branch removal) unchanged.

### 1. Resolve the integration branch (never hard-coded)

New shell function `resolve_base_branch`, first non-empty wins:

1. `$KAIRON_BASE_BRANCH` environment variable (override; also makes tests trivial).
2. `base_branch:` top-level key in `.kairon/config.yaml` (new, optional). Parsed with `sed`/`grep` (no YAML
   tool dependency; strip surrounding quotes, trailing `# comment`, whitespace). Only match a top-level
   (column 0) key so nested keys cannot collide.
3. Auto-detect from the remote: `git symbolic-ref --quiet --short refs/remotes/origin/HEAD` (strip the
   `origin/` prefix); if that is unset, `git ls-remote --symref origin HEAD` and parse `ref: refs/heads/<X>`.
4. Fall back to `main`.

Validate the result with `git check-ref-format --branch "$BASE"`; invalid value => error, exit 1 (also guards
against odd characters flowing into git arguments). Always pass the value quoted / after `--` where relevant.

### 2. Fetch and branch from the remote tip

- If remote `origin` exists (`git remote get-url origin`):
  - `git fetch origin "+refs/heads/$BASE:refs/remotes/origin/$BASE" --quiet`. The explicit refspec guarantees
    `refs/remotes/origin/$BASE` is updated regardless of the repo's configured fetch refspec.
  - Fetch fails (offline/auth): if `refs/remotes/origin/$BASE` already exists, warn on stderr
    ("fetch failed; using last-known origin/$BASE") and continue; if it does not exist, print an error and
    `exit 1`. (Do not silently fall back to local HEAD when a remote exists — that is the bug.)
  - Start-point = `origin/$BASE`, verified with `git rev-parse --verify --quiet "refs/remotes/origin/$BASE^{commit}"`.
- If there is no `origin` remote (local-only repos, sandbox/eval fixtures): warn and keep today's behaviour
  (start-point = `HEAD`). This preserves existing uses that have no remote.
- Create with: `git worktree add --no-track "$WORKTREE_PATH" -b "$BRANCH_NAME" "$START_POINT"`.
  `--no-track` is important: branching from a remote-tracking ref otherwise sets upstream to `origin/main`,
  which makes a later `git push` of `spec/...` ambiguous/refused under `push.default=simple`.
- Log the chosen base and short SHA on stderr (`Branching from origin/main (abc1234)`); stdout stays only the
  absolute worktree path (manager.go parses stdout with `TrimSpace`).

Order inside the script: arg check -> idempotency early-exit (unchanged; an existing worktree is left as-is) ->
stale-branch removal (unchanged) -> resolve base -> fetch -> `git worktree add`.

### 3. Config surface

Add an optional `base_branch` key so the Go side recognises it (documentation + `kairon` config consumers):
`Config.BaseBranch string \`yaml:"base_branch"\`` in `internal/config/config.go`, default empty (= auto-detect),
no validation beyond what the script performs. Add a commented example to `.kairon/config.yaml` and to the
template `cmd/kairon/templates/kairon/config.yaml` (that template is NOT covered by `task sync:check`; edit both).
The Go code does not need to pass the value to the script — the script reads config.yaml itself (same cwd).

### Decisions / out of scope

- `planning-worktree-create.sh` has the same local-HEAD pattern but the issue scopes only `worktree-create.sh`
  and planning worktrees are throw-away test environments; leave unchanged (mention in PR description).
- `worktree-merge.sh` (merges spec branch into current local branch) is not changed.
- Dependent-issue sequencing is explicitly out of scope.
- Krew-lead PR creation uses `gh pr create --head spec/...` without `--base`, so GitHub targets the repo
  default branch; a non-default `base_branch` therefore is a worktree-base setting only. Note this in docs; do
  not change krew-lead prompt in this PR.

## Relevant Files

| File | Change |
|------|--------|
| `.kairon/scripts/worktree-create.sh` | Modify (core fix) |
| `cmd/kairon/templates/kairon/scripts/worktree-create.sh` | Sync copy (live -> template, must be byte-identical; `task sync:check`) |
| `internal/config/config.go` | Add `BaseBranch` field |
| `internal/config/config_test.go` | Test `base_branch` parses, defaults to empty |
| `.kairon/config.yaml` | Add commented `# base_branch:` example (live) |
| `cmd/kairon/templates/kairon/config.yaml` | Add same commented example (not sync-checked) |
| `internal/agent/worktree_create_test.go` | New: Go tests exercising the script against temp git repos |
| `README.md` | Update Git Worktree Isolation section + config table (`base_branch`) |
| `docs/agent-conventions.md` | No functional change needed (only mentions running the script); leave |

Only the single template copy of the script exists (verified: `cmd/kairon/templates/kairon/scripts/worktree-create.sh`
is currently identical to the live file). `internal/agent/manager.go` needs no change.

## Team Orchestration

- `implement-script` and `add-config-key` are independent and run in parallel.
- `add-script-tests` needs the new script behaviour (depends on `implement-script`).
- `update-docs` needs both `implement-script` and `add-config-key` (documents behaviour + config key).
- `validate-all` (validator, read-only) runs last.

## Step-by-Step Task Breakdown

### Task 1 (`implement-script`): Fix worktree-create.sh and sync template
Implement sections 1 and 2 above in `.kairon/scripts/worktree-create.sh`, then
`cp .kairon/scripts/worktree-create.sh cmd/kairon/templates/kairon/scripts/` (only this file changed; run
`cp .kairon/scripts/*.sh ...` per builder-conventions is also fine). Keep executable bit. Keep bash (not sh).
Dependencies: none.

### Task 2 (`add-config-key`): Add `base_branch` config key
Add field to `Config`, a test in `config_test.go` (value parsed; absent => ""), commented examples in both
config.yaml files. Dependencies: none.

### Task 3 (`add-script-tests`): Go tests for the script
New `internal/agent/worktree_create_test.go`. Helper builds, in `t.TempDir()`: a bare `origin.git` with branch
`main` (HEAD -> main), a `local` clone, then (from a second clone "other") pushes new commit(s) to origin so
`local` is behind without having fetched. Script under test is referenced by absolute path
(`../../.kairon/scripts/worktree-create.sh` resolved from the test's package dir) and run with `bash <script>
<name>` and `cmd.Dir = local`. Set `GIT_CONFIG_GLOBAL=/dev/null`-style isolation plus user.name/email via env
so tests do not depend on host config; skip if `git` or `bash` missing. Cases:

1. Local behind origin/main: `git merge-base spec/<name> origin/main` == `git rev-parse origin/main` (origin's
   NEW tip), and `local`'s own `main` is untouched/still behind. (Primary acceptance test.)
2. Fetch is performed by the script (no manual fetch beforehand) — covered by case 1; additionally assert
   `origin/main` in `local` was updated.
3. stdout is exactly the absolute worktree path (single line); worktree dir exists.
4. Branch has no upstream (`git config --get branch.spec/<name>.remote` empty).
5. Non-main integration branch via `.kairon/config.yaml` `base_branch: develop` (origin has `develop` with a
   distinct commit) -> branch based on origin/develop. Include quoted value and trailing comment variants.
6. `KAIRON_BASE_BRANCH` env overrides config value.
7. Auto-detect: origin default branch `trunk` (HEAD -> trunk), no config/env -> based on origin/trunk (not
   hard-coded main).
8. No `origin` remote: exits 0, worktree created from local HEAD.
9. Fetch fails but cached `origin/main` exists (origin URL repointed to a nonexistent path): exits 0, based on
   cached origin/main, stderr contains a warning.
10. Configured branch does not exist on origin and not cached: exit non-zero, no worktree/branch created.
11. Idempotent re-run with the same name exits 0 and prints the same path; pre-existing stale `spec/<name>`
    branch without worktree is replaced and rebased onto fresh origin tip.

Dependencies: `implement-script`.

### Task 4 (`update-docs`): README documentation
README: config table gains `base_branch` row (optional; default auto-detect from `origin/HEAD`, else `main`);
"Git Worktree Isolation" section states `worktree-create.sh` fetches the integration branch and creates
`spec/<name>` from `origin/<branch>` (not local HEAD), lists the resolution order (env, config, origin/HEAD,
`main`), notes the offline fallback and no-remote fallback, and that PRs still target the repo default branch.
Dependencies: `implement-script`, `add-config-key`.

### Task 5 (`validate-all`): Verify
Read-only validation of all acceptance criteria; run full test/lint/sync checks.
Dependencies: `add-script-tests`, `update-docs`.

## Acceptance Criteria Traceability

| Issue criterion | Where satisfied / verified |
|-----------------|----------------------------|
| Script fetches remote integration branch before creating worktree | Task 1; test case 1/2 |
| Worktree/branch created from remote tip, not local HEAD | Task 1 (`git worktree add ... "origin/$BASE"`); test case 1 |
| Behind-origin checkout: `merge-base <branch> origin/main` == origin/main tip | Test case 1 |
| Integration branch not hard-coded (config or detected) | Task 1/2; test cases 5, 6, 7 |

## Validation Commands

```bash
bash -n .kairon/scripts/worktree-create.sh
task sync:check
go build ./...
go vet ./internal/agent/... ./internal/config/...
go test -race ./internal/agent/... ./internal/config/...
task test
task lint
```

Manual spot check (optional): in a scratch clone that is behind origin, run
`bash .kairon/scripts/worktree-create.sh issue-0-manual` and compare
`git merge-base spec/issue-0-manual origin/main` with `git rev-parse origin/main`; then
`git worktree remove --force .worktrees/issue-0-manual && git branch -D spec/issue-0-manual`.

## Risks

- Behaviour change for repos whose `origin` default branch is not the branch humans want (mitigated by
  `base_branch` / `KAIRON_BASE_BRANCH`).
- Network call added to every worktree creation (one small fetch of one branch; failure degrades to cached ref).
- Tests shell out to `git`/`bash`; guarded with `exec.LookPath` skips and isolated env.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "implement-script"
    agent: "builder"
    description: "Modify .kairon/scripts/worktree-create.sh to resolve the integration branch (KAIRON_BASE_BRANCH env, then top-level base_branch in .kairon/config.yaml, then origin/HEAD via git symbolic-ref / git ls-remote --symref, then main), validate it with git check-ref-format, fetch it from origin with an explicit refspec, and create the worktree with 'git worktree add --no-track <path> -b <branch> origin/<base>'. Fetch failure falls back to a cached origin/<base> with a stderr warning (error if none); no origin remote falls back to local HEAD with a warning. Keep CLI contract (stdout = absolute path only, logs on stderr, idempotency, stale-branch removal). Then copy the script to cmd/kairon/templates/kairon/scripts/worktree-create.sh."
    dependencies: []
    acceptance_criteria:
      - "Script fetches origin/<base> before 'git worktree add' and passes origin/<base> as the explicit start-point"
      - "Integration branch name is not hard-coded as the only option: env, config base_branch, origin/HEAD detection are honoured before falling back to main"
      - "Worktree branch has no upstream tracking (--no-track)"
      - "Invalid branch names are rejected via git check-ref-format with a non-zero exit"
      - "stdout contains only the absolute worktree path; all logging goes to stderr"
      - "Template copy cmd/kairon/templates/kairon/scripts/worktree-create.sh is byte-identical to the live script and both are executable"
    validation_commands:
      - "bash -n .kairon/scripts/worktree-create.sh"
      - "grep -q 'git fetch' .kairon/scripts/worktree-create.sh"
      - "grep -q -- '--no-track' .kairon/scripts/worktree-create.sh"
      - "diff .kairon/scripts/worktree-create.sh cmd/kairon/templates/kairon/scripts/worktree-create.sh"
      - "task sync:check"

  - id: "add-config-key"
    agent: "builder"
    description: "Add optional base_branch config key: Config.BaseBranch string with yaml tag base_branch in internal/config/config.go (default empty = auto-detect, no extra validation), a test in internal/config/config_test.go covering parsed value and empty default, and a commented '# base_branch: main' example (with explanatory comment) in both .kairon/config.yaml and cmd/kairon/templates/kairon/config.yaml. Do not uncomment it in the live config."
    dependencies: []
    acceptance_criteria:
      - "Config struct exposes BaseBranch with yaml tag base_branch"
      - "Config test proves base_branch: develop loads as 'develop' and absence yields empty string"
      - "Both config.yaml files contain a commented base_branch example and still load successfully"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/config/..."
      - "grep -q 'base_branch' .kairon/config.yaml"
      - "grep -q 'base_branch' cmd/kairon/templates/kairon/config.yaml"

  - id: "add-script-tests"
    agent: "builder"
    description: "Create internal/agent/worktree_create_test.go that runs .kairon/scripts/worktree-create.sh (resolved by path from the package dir) via bash with cmd.Dir set to a temp clone, using temp bare origin + local clone + second clone pushing new commits, with isolated git env (no host config; test user.name/email). Cover: (1) local behind origin/main => git merge-base <branch> origin/main equals origin/main's new tip and local main untouched; (2) origin/main updated by the script's own fetch; (3) stdout is exactly the absolute worktree path; (4) no upstream on branch; (5) base_branch in config (plain, quoted, trailing comment) selects origin/develop; (6) KAIRON_BASE_BRANCH env overrides config; (7) origin default branch 'trunk' auto-detected with no config; (8) no origin remote => exit 0 from local HEAD; (9) fetch failure with cached origin/main => exit 0 with warning on stderr; (10) missing configured branch => non-zero exit and no worktree/branch created; (11) idempotent re-run and stale branch replaced. Skip gracefully if git or bash are not on PATH."
    dependencies: ["implement-script"]
    acceptance_criteria:
      - "Test file exists and covers all 11 listed scenarios with clear subtests"
      - "The behind-origin test asserts merge-base equals origin/main tip (issue acceptance criterion)"
      - "Tests are hermetic: temp dirs only, no network, no dependence on host git config, no leftover files in the repo"
      - "All tests pass with -race"
    validation_commands:
      - "go vet ./internal/agent/..."
      - "go test -race -run WorktreeCreate -v ./internal/agent/..."
      - "go test -race ./internal/agent/..."

  - id: "update-docs"
    agent: "builder"
    description: "Update README.md: add a base_branch row to the configuration table (optional; default auto-detect from origin/HEAD else main) and extend the Git Worktree Isolation section to say worktree-create.sh fetches the integration branch and creates spec/<name> from origin/<branch> rather than local HEAD, listing resolution order (KAIRON_BASE_BRANCH, config base_branch, origin/HEAD, main), the offline cached-ref fallback, the no-remote fallback to local HEAD, and that PRs still target the repository default branch."
    dependencies: ["implement-script", "add-config-key"]
    acceptance_criteria:
      - "README config table documents base_branch"
      - "README Git Worktree Isolation section describes fetch + branch-from-origin behaviour and resolution order"
      - "No other files changed by this task"
    validation_commands:
      - "grep -q 'base_branch' README.md"
      - "grep -q 'origin/' README.md"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of issue #288 acceptance criteria: script fetches and branches from origin/<integration-branch>, integration branch not hard-coded, behind-origin merge-base test passes, template copy in sync, docs updated, full test/lint/sync suites green."
    dependencies: ["add-script-tests", "update-docs"]
    acceptance_criteria:
      - "worktree-create.sh fetches the remote integration branch before git worktree add"
      - "Worktree is created from origin/<base>, not local HEAD, and the behind-origin test proves merge-base equals origin tip"
      - "Integration branch is resolved from env/config/origin HEAD with main only as last-resort fallback"
      - "Live and template scripts are identical (task sync:check passes)"
      - "task test and task lint pass"
    validation_commands:
      - "bash -n .kairon/scripts/worktree-create.sh"
      - "task sync:check"
      - "go test -race -run WorktreeCreate -v ./internal/agent/..."
      - "task test"
      - "task lint"
```
