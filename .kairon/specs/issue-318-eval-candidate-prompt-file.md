# Design Spec: Evaluate a candidate prompt without modifying the live agent

Closes #318

## Problem

A prompt iteration today means editing `.kiro/agents/<agent>-prompt.md`. Krew-lead spawns builder and validator from the worktree's `.kiro/agents/`, so a mid-iteration edit changes the pipeline that is building the PR. An edit to the planner prompt also degrades the planner used day to day. Iterations need to score a candidate prompt while the live agent stays untouched.

## Solution Approach

Add `kairon eval <agent> --prompt-file <path>`. The candidate is injected **only into the per-case workspaces** that the harness already builds for every case (`internal/eval/workspace.go`). Nothing in the repository is written.

How the pieces already fit together (verified by reading the code):

- `newCaseWorkspace` -> `stageKiro` copies the project's `.kiro/` into `<ws>/.kiro/`, then overlays `<evals-dir>/agents/`. The agent runs with `<ws>` as its cwd (native) or with `<ws>` bind-mounted (`--sandbox`), and `kiro-cli` discovers the agent from `<ws>/.kiro/agents/`. `copyTree` copies file contents (no symlinks), so overwriting a staged file cannot touch the live file.
- The agent config's `"prompt": "file://./x-prompt.md"` is resolved relative to the config file's directory, which in the workspace is `<ws>/.kiro/agents/`.
- `prompt_sha256` is computed once in `pinRun` -> `resolveAgentProvenance` over `config` + `prompt` file + `resource` parts, and copied into results by `runPins.applyTo`, `updateIncrementalSummary` and each call record.

Design decisions:

1. **Substitute the file the config's `prompt` points at, in the staged copy only.** After `stageKiro` copies project `.kiro/` and the evals-dir overlay, write the candidate bytes over `<ws>/.kiro/agents/<prompt-ref>`. The staged config is not rewritten, so everything else about the agent (model, tools, resources, MCP) is the live agent. Candidate wins over a workspace-fixture-provided file at the same path (it is the thing under test); the write happens before `setPermissions`, so the staged file ends up read-only like the rest of `.kiro/`. Remove any existing destination entry before writing so a pre-existing symlink/hardlink can never be written through.
2. **Which config / which prompt path.** Use the same lookup as provenance (`locateAgentConfig(agent, false)`: `<evals-dir>/agents/<agent>.json` else `.kiro/agents/<agent>.json`). A single helper resolves the staged-relative destination of the prompt and is used by both provenance (pre-flight) and staging, so the two cannot disagree. The helper rejects: an inline prompt (no `file://`), an absolute `file://` path (it would not resolve inside the workspace/container), and a ref that escapes `.kiro/` after cleaning. Pre-flight rejection means a bad combination fails before any case runs.
3. **Provenance.** With a candidate, the `prompt` hash part uses the candidate bytes instead of the live prompt file bytes; the `config` and `resource` parts are unchanged. So `prompt_sha256` differs from the baseline iff the candidate content differs from the live prompt, and equals the baseline hash for byte-identical content (a useful no-op check). The live prompt file need not even exist when a candidate is supplied.
4. **Recorded `prompt_file`.** The path as given on the command line, normalised with `filepath.Clean` + `filepath.ToSlash` (relative stays relative, absolute stays absolute). `omitempty` everywhere so runs without a candidate are byte-for-byte unchanged in shape. Recorded in: `<agent>.json` (`AgentResult.PromptFile`), `summary.json` `agents.<agent>.prompt_file` (`AgentProvenance.PromptFile`), and top-level `summary.prompt_file` under the existing single-agent rule (always single-agent here, since an agent is required). `inference.CallRecord` is a shared type and is intentionally not changed; its `prompt_sha256` already reflects the candidate via the pin.
5. **Validation, all before any case and before any state change** (`RunWithOptions` top, before `configure`):
   - `--prompt-file` without an agent -> error containing `agent is required` (AC4).
   - `--prompt-file` with `--list`, `--perf` or `--cleanup` -> error (nothing is evaluated through workspaces in those modes, so the flag would be silently ignored).
   - File missing / unreadable / a directory / empty -> error naming the path (AC5). An empty prompt is almost certainly a mistake.
   - Path is resolved against the process working directory (the repo root for a normal run).
6. **Resume safety.** `--resume` already refuses a changed `prompt_sha256`. Add `prompt_file` to that comparison so resuming a candidate run without the flag (or with a different file whose content happens to hash identically) is refused rather than mixing provenance. Files without `prompt_file` compare equal to "no candidate".
7. **`eval diff`.** Show `prompt_file` differences in the Run Provenance block next to `prompt_sha256` (reporting only; never fails the diff).
8. **Single global.** Follow the established `cfg` pattern (`runConfig`): add `candidate *candidatePrompt` set by `RunWithOptions` after `configure` (which resets `cfg`, so a candidate never leaks between runs, matching the "pins reset to nil" behaviour).

Backends: native `kiro-cli`, `--sandbox` and `stub` all go through the same workspace staging, so one injection point covers them. The `stub` backend does not run an agent, so with `stub` the observable effects are the staged file, `prompt_file` and `prompt_sha256` (exactly what the acceptance criteria verify).

Non-goals / out of scope (per the issue): writing any candidate prompt, creating `.kairon/iterations/`, any change under `.kiro/agents/`. Candidate prompts conventionally live at `.kairon/iterations/<agent>/`; this is documented, not enforced or created. Only the file named by the config's `prompt` is substituted; the config, resources/skills and MCP settings stay live.

### Environment note for builders / validator (important for AC2)

In this worktree `git status --porcelain .kiro/agents` is **already non-empty** before any work (six `.kiro/agents/*.json` files carry uncommitted environment-injected changes). Do **not** touch, revert or "fix" them. AC2 must be verified as "unchanged by the run": snapshot before/after (for example `shasum -a 256 .kiro/agents/* | sort` plus `git status --porcelain .kiro/agents`, and compare), or in an isolated temp git repo where it starts clean (as the Go tests do). Never `git add`/commit `.kiro/agents`.

### Template sync

No template-synchronized path is touched (`.kiro/agents/`, `.kairon/scripts`, `.kairon/themes`, `.kairon/evals/{fixtures,rubrics,cases}`, `.kiro/skills/sentinel-protocol`). Test fixtures are created in temp dirs by the tests (or live under the un-synced `internal/eval/testdata`). `task sync:check` is still run as a regression check but no sync copy is expected. Per the builder-conventions sentinel format, record `Template Sync: N/A`.

## Relevant Files

Modify:
- `internal/eval/types.go` - `RunOptions.PromptFile`; `AgentResult.PromptFile`, `AgentProvenance.PromptFile`, `Summary.PromptFile` (all `json:"prompt_file,omitempty"`).
- `internal/eval/config.go` - `runConfig.candidate *candidatePrompt` (reset by `configure`).
- `internal/eval/provenance.go` - `agentProvenance.PromptFile`; candidate-aware hashing in `resolveAgentProvenance`; `runPins.applyTo` copies `PromptFile`.
- `internal/eval/runner.go` - `RunWithOptions` validation/wiring; `updateIncrementalSummary` copies `prompt_file` into `Agents` and the single-agent top level; `checkResumeIntegrity` compares `prompt_file`.
- `internal/eval/workspace.go` - `stageKiro` (or a helper it calls) writes the candidate into the staged prompt path.
- `internal/eval/diff_provenance.go` - carry and print `prompt_file`.
- `cmd/kairon/cmd/eval.go` - `--prompt-file` flag, `evalPromptFile` var, pass `PromptFile` in `RunOptions`.
- `docs/evaluation.md` - document the flag, semantics, provenance fields, conventions, caveats.

Create:
- `internal/eval/candidate.go` - `candidatePrompt{Agent, Path, Content}`, `loadCandidatePrompt(agent, opts)`, `candidatePromptDest(configPath)` helper, `stageCandidatePrompt`.
- `internal/eval/candidate_test.go` - unit tests (see tasks).
- `internal/eval/candidate_e2e_test.go` - end-to-end stub-backend tests.

Read-only references: `internal/eval/execute_case.go`, `internal/eval/testdata/evals/**` (selftest agent `selftest.json` -> `selftest-prompt.md`), `internal/eval/selftest_test.go` helpers (`copyFixturesTo`, `newRunDirs`, `readSelfTestResult`), `internal/eval/provenance_test.go` (`provProject`, `chdirTemp`), `cmd/kairon/cmd/eval_test.go` (`evalSelfTestProject`).

Do not touch: `.kiro/agents/**`, `.kairon/specs/**` (other than this file), `CHANGELOG.md`, `.kairon/evals/results/`.

## Team Orchestration

- `candidate-core` (builder) lands first: it defines `candidatePrompt`, `RunOptions.PromptFile`, `cfg.candidate`, the result-struct fields, validation and provenance hashing that everything else uses.
- After it, three builders can run in parallel because they touch disjoint files: `stage-candidate-workspace` (`workspace.go`, `candidate.go` staging func), `cli-flag` (`cmd/kairon/cmd/*`), `resume-and-diff` (`runner.go` resume check, `diff_provenance.go`). Note `candidate-core` and `stage-candidate-workspace` both add to `candidate.go`; core creates the file, stage appends a function in a new section, so the dependency edge is required.
- `e2e-acceptance-tests` (builder) runs after staging and resume/diff; it exercises the whole path with the stub backend.
- `document-candidate-prompt` (documenter) after implementation.
- `validate-complete` (validator) last: runs full QA and the five issue acceptance checks.

Every builder task follows AGENTS.md TDD: write the failing test first, run it and confirm it fails for the expected reason (not a compile error - add the minimal type/stub signature needed to compile first if necessary), then implement, then re-run. State the observed failing test name in the sentinel summary.

## Step-by-Step Task Breakdown

### Task 1: candidate-core (validation, options, provenance)
Tests first in `candidate_test.go`:
- `TestLoadCandidatePrompt`: no `PromptFile` -> `nil, nil`; `PromptFile` with empty agent -> error containing `agent is required`; missing file, directory, empty file -> errors naming the path; valid file -> `Content` equals bytes, `Path` equals `filepath.ToSlash(filepath.Clean(input))`.
- `TestRunWithOptionsPromptFileRejected`: agent empty; with `List`, `Perf`, `Cleanup` -> non-nil errors, and no results dir created / no config state changed; missing file with agent `selftest` + stub backend + copied fixtures -> error and no `results/` dir (AC4, AC5).
- Provenance tests (use `provProject`): with `cfg.candidate` set, `resolveAgentProvenance` returns `PromptFile`, and a hash that (a) differs from baseline when content differs, (b) equals the baseline when candidate bytes equal the live prompt bytes, (c) equals the hash of a project whose live prompt file was replaced by the candidate bytes, (d) works when the live prompt file is absent; errors for inline prompt, absolute `file://` path, and a ref escaping `.kiro/`.
- `TestApplyToRecordsPromptFile` and `TestUpdateIncrementalSummaryPromptFile`: `prompt_file` appears in `AgentResult`, `Summary.Agents[agent]` and top-level (single agent); absent from JSON when no candidate (key not present).
Then implement `candidate.go`, struct fields/JSON tags, `runConfig.candidate`, `pinRun` unchanged except through `resolveAgentProvenance`, `applyTo`, `updateIncrementalSummary`, and the `RunWithOptions` pre-flight (before `configure`; assign `cfg.candidate` right after `configure` succeeds). Keep `resolveAgentProvenance(agent, ignoreOverlay)` signature (existing tests call it); read `cfg.candidate` inside, with an explicit-candidate inner function for testability.

### Task 2: stage-candidate-workspace
Tests first (`candidate_test.go`, using `newCaseWorkspace` with a temp project like `workspace_test.go`):
- With `cfg.candidate` set, `<ws>/.kiro/agents/<prompt-ref>` equals the candidate bytes; the live `.kiro/agents/<prompt-ref>` bytes and mtime are unchanged; other staged files are unchanged; staged file mode is not group/other-writable.
- Overlay case: config in `<evals-dir>/agents/` with `file://./x.md` -> staged `x.md` replaced, evals-dir copy unchanged.
- A workspace fixture that provides the same prompt path is overridden by the candidate.
- A pre-existing symlink at the destination is replaced, not written through.
- Without a candidate, staging is byte-identical to today (existing `workspace_test.go` still green).
Implement `stageCandidatePrompt` called from `stageKiro` after the evals-dir overlay copy and before `setPermissions`.

### Task 3: cli-flag
Tests first in `cmd/kairon/cmd/eval_test.go`:
- `TestEvalFlagsRegistered` extended: `--prompt-file` exists on `eval`, default empty, usage mentions candidate/agent.
- `TestEvalPromptFileWithoutAgentRejected`: `evalCmd.RunE(evalCmd, nil)` with `evalPromptFile="x.md"` -> error containing `agent is required` (restore the var in cleanup).
- `TestEvalPromptFileMissingRejected` using `evalSelfTestProject`: `selftest` + missing file -> error, no `results/` dir.
- `TestEvalPromptFileRecorded`: `selftest` with a temp candidate -> `summary.json` has `prompt_file` and `agents.selftest.prompt_file`, and `prompt_sha256` differs from a no-candidate run in a second fixture copy.
Implement flag + `RunOptions.PromptFile` pass-through. The flag is local to `eval` (not inherited by `diff`).

### Task 4: resume-and-diff
Tests first:
- `checkResumeIntegrity`: an existing `<agent>.json` recorded with `prompt_file: a.md` and a resume with no candidate (or `b.md`) is refused with a message naming both values; identical `prompt_file` and hash resumes; legacy file without `prompt_file` + no candidate resumes as today.
- `eval diff` provenance: two runs differing in `prompt_file` print a `prompt_file` difference line; a pre-existing run without the field prints `(not recorded)` consistent with other fields and does not error.
Implement in `runner.go` (`checkResumeIntegrity`) and `diff_provenance.go` (`agentProv.promptFile`, `loadRunInfo`, `printProvenance`).

### Task 5: e2e-acceptance-tests
In `candidate_e2e_test.go` using `RunWithOptions` with `Backend: "stub"` and a copied fixtures dir (`copyFixturesTo`) plus a `t.TempDir()` candidate:
- AC1: run `selftest` with `KeepWorkspaces: true` and the candidate; for every case in `selftest.json`, `<workspace_dir>/.kiro/agents/selftest-prompt.md` equals the candidate bytes. (Clean up kept workspaces in the test.)
- AC2: snapshot (path + sha256 of every file) of the repo-style `.kiro/agents` tree used by the test project and of the evals-dir `agents/` before and after - identical. Include a run in a temp git repo where `git status --porcelain .kiro/agents` is empty before and after.
- AC3: run without candidate and with candidate against separate fixture copies; `selftest.json` `prompt_file` empty (key absent) vs set; `prompt_sha256` differs; same for `summary.json`.
- A run with `Resume: true` after a candidate run without the candidate is refused.
- Same-content-as-live candidate: `prompt_sha256` equal to baseline while `prompt_file` is set.
- Sandbox/daemon-gated variants are not required (the sandbox shares `stageKiro`); do not add daemon-dependent tests.

### Task 6: document-candidate-prompt
Update `docs/evaluation.md`: a new "Evaluating a candidate prompt (`--prompt-file`)" section (placed near "Agent configs: `agents/` precedence" and linked from the Evaluation Workflow section, replacing the guidance that tells readers to edit the live prompt for an iteration) covering: purpose; usage example `kairon eval architect --prompt-file .kairon/iterations/architect/v2.md`; the `.kairon/iterations/<agent>/` convention (documented, directory not created by the tool); exactly what is substituted (only the file the config's `prompt` references, in each case workspace) and what is not (config, resources, MCP); the live `.kiro/agents/` is never modified; `prompt_file` and `prompt_sha256` semantics in `<agent>.json` and `summary.json` (add to the field tables and the "How `prompt_sha256` is computed" section, noting the prompt part uses candidate bytes); rejection rules (agent required, missing/empty/directory file, inline or absolute prompt refs, incompatible with `--list`/`--perf`/`--cleanup`); resume and diff behaviour; verifying with `--keep-workspaces`. Do not edit `.kairon/specs/**` or `CHANGELOG.md`.

### Task 7: validate-complete
Run QA and the acceptance checks below. Read-only.

## Validation Commands

```bash
go build ./...
go test ./internal/eval/... ./cmd/kairon/... -count=1
task lint
task fmt:check
task test
task sync:check            # regression only; no template change expected

# AC4
go run ./cmd/kairon eval --prompt-file x.md; echo $?          # non-zero, error says an agent is required
# AC5
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --prompt-file missing.md; echo $?   # non-zero

# AC1/AC2/AC3 (manual end-to-end; use a copy of the fixtures so results/ is not polluted)
E=$(mktemp -d)/evals && cp -R internal/eval/testdata/evals "$E" && rm -rf "$E/results"
printf '# Candidate\n\nAnswer concisely with ## and ### headings.\n' > /tmp/cand-318.md
git status --porcelain .kiro/agents > /tmp/before-318.txt; shasum -a 256 .kiro/agents/* > /tmp/before-318.sha
go run ./cmd/kairon eval --backend stub --evals-dir "$E" selftest
go run ./cmd/kairon eval --backend stub --evals-dir "$E" --keep-workspaces selftest --prompt-file /tmp/cand-318.md
git status --porcelain .kiro/agents | diff - /tmp/before-318.txt && shasum -a 256 .kiro/agents/* | diff - /tmp/before-318.sha   # AC2 (worktree is pre-dirtied; compare, do not require empty)
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "candidate-core"
    agent: "builder"
    description: "TDD: add candidate prompt core in internal/eval - RunOptions.PromptFile, candidatePrompt + loadCandidatePrompt validation (agent required, missing/dir/empty file, conflicts with List/Perf/Cleanup) wired at the top of RunWithOptions before configure, cfg.candidate, candidate-aware prompt_sha256 in resolveAgentProvenance (candidate bytes replace the live prompt part; inline/absolute/escaping prompt refs rejected), and prompt_file recorded in AgentResult, AgentProvenance and Summary (omitempty) via applyTo and updateIncrementalSummary. Do not touch .kiro/agents."
    dependencies: []
    acceptance_criteria:
      - "Failing tests written first and observed failing for the expected reason (name them in the sentinel), then passing"
      - "RunWithOptions with PromptFile and empty agent returns an error containing 'agent is required' before any state change or case run"
      - "A missing, directory or empty PromptFile returns an error naming the path before any case runs and creates no results directory"
      - "PromptFile combined with List, Perf or Cleanup is rejected"
      - "With a candidate, prompt_sha256 differs from baseline when content differs and equals baseline when bytes are identical; the live prompt file is not required to exist"
      - "Inline prompt, absolute file:// prompt and a prompt ref escaping .kiro/ are rejected in pre-flight with a clear error"
      - "prompt_file is present in AgentResult, summary agents.<agent> and single-agent top-level summary when a candidate is used, and the key is absent otherwise"
      - "Existing resolveAgentProvenance(agent, ignoreOverlay) callers and all existing internal/eval tests still compile and pass"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval -run 'Candidate|PromptFile|Provenance|ApplyTo|IncrementalSummary' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "stage-candidate-workspace"
    agent: "builder"
    description: "TDD: in internal/eval/workspace.go stageKiro (and candidate.go stageCandidatePrompt) write the candidate bytes over <ws>/.kiro/agents/<prompt-ref> after the project .kiro copy and evals-dir overlay and before setPermissions, overriding any fixture-provided file, removing any existing symlink at the destination first, so only case workspaces see the candidate."
    dependencies: ["candidate-core"]
    acceptance_criteria:
      - "Failing test written first and observed failing for the expected reason, then passing"
      - "Staged prompt file in the workspace equals the candidate bytes for both repo-config and evals-dir overlay configs"
      - "The live .kiro/agents prompt file (bytes and mtime) and the evals-dir agents/ copy are unchanged after staging"
      - "A fixture-provided file at the prompt path and a pre-existing symlink at the destination are overridden without writing through to the target"
      - "Staged file is not group/other-writable after setPermissions; without a candidate staging output is unchanged and existing workspace tests pass"
    validation_commands:
      - "go test ./internal/eval -run 'Workspace|Candidate|StageKiro' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "cli-flag"
    agent: "builder"
    description: "TDD: add the --prompt-file flag to the eval command in cmd/kairon/cmd/eval.go (evalPromptFile var, local flag not inherited by diff), pass it as RunOptions.PromptFile, and cover it with tests in eval_test.go for registration, missing agent, missing file, and recorded prompt_file/prompt_sha256."
    dependencies: ["candidate-core"]
    acceptance_criteria:
      - "Failing tests written first and observed failing for the expected reason, then passing"
      - "--prompt-file is registered on eval with an empty default and a usage string describing the candidate prompt"
      - "evalCmd.RunE with --prompt-file and no agent returns an error containing 'agent is required'"
      - "A missing candidate file is rejected with an error and no results directory is created"
      - "A selftest run with a candidate records prompt_file and a prompt_sha256 different from a run without one"
    validation_commands:
      - "go test ./cmd/kairon/... -count=1"
      - "go run ./cmd/kairon eval --prompt-file x.md; test $? -ne 0"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --prompt-file missing.md; test $? -ne 0"

  - id: "resume-and-diff"
    agent: "builder"
    description: "TDD: make --resume refuse a change of prompt_file (in addition to prompt_sha256) in checkResumeIntegrity, and make eval diff carry and print prompt_file differences in the Run Provenance block (agentProv.promptFile, loadRunInfo, printProvenance); legacy files without prompt_file behave as before."
    dependencies: ["candidate-core"]
    acceptance_criteria:
      - "Failing tests written first and observed failing for the expected reason, then passing"
      - "Resuming a candidate run without the candidate (or with a different prompt_file) is refused with a message naming recorded and current values"
      - "Resuming with identical prompt_file and hash, and resuming a legacy file without prompt_file and no candidate, still work"
      - "eval diff reports a prompt_file difference between two runs and does not fail on runs lacking the field"
    validation_commands:
      - "go test ./internal/eval -run 'Resume|Diff|Provenance' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "e2e-acceptance-tests"
    agent: "builder"
    description: "Add end-to-end stub-backend tests in internal/eval/candidate_e2e_test.go covering AC1 (kept workspaces' prompt file equals candidate for every case), AC2 (live .kiro/agents and evals-dir agents tree unchanged, git status clean in a temp repo), AC3 (prompt_file recorded and prompt_sha256 differs from baseline in <agent>.json and summary.json), same-content candidate hash equality, and resume refusal. No daemon-dependent tests."
    dependencies: ["stage-candidate-workspace", "resume-and-diff"]
    acceptance_criteria:
      - "For every case in selftest.json from a --keep-workspaces candidate run, <workspace_dir>/.kiro/agents/selftest-prompt.md equals the candidate bytes; kept workspaces are removed by test cleanup"
      - "Before/after snapshot (path and sha256) of the project .kiro/agents tree and the evals-dir agents/ directory is identical after a candidate run; git status --porcelain .kiro/agents is empty before and after in the temp repo"
      - "Baseline run has no prompt_file key and candidate run has prompt_file with a different prompt_sha256, in both selftest.json and summary.json"
      - "Tests pass without kiro-cli on PATH and without a container daemon"
    validation_commands:
      - "go test ./internal/eval -run 'CandidateE2E|PromptFile' -count=1 -v"
      - "go test ./internal/eval/... ./cmd/kairon/... -count=1"

  - id: "document-candidate-prompt"
    agent: "documenter"
    description: "Update docs/evaluation.md with a --prompt-file section (usage, .kairon/iterations/<agent>/ convention, what is and is not substituted, live .kiro/agents never modified, prompt_file/prompt_sha256 semantics added to the field tables and the prompt_sha256 computation section, rejection rules, resume and diff behaviour, verifying with --keep-workspaces) and fix the Evaluation Workflow guidance that tells readers to edit the live prompt. Do not edit .kairon/specs or CHANGELOG.md."
    dependencies: ["e2e-acceptance-tests", "cli-flag"]
    acceptance_criteria:
      - "docs/evaluation.md documents the flag, its validation errors, and the exact behaviour implemented (including prompt hash part using candidate bytes and prompt_file normalisation)"
      - "Field tables for <agent>.json and summary.json list prompt_file"
      - "Evaluation Workflow section recommends --prompt-file for iterations instead of editing the live prompt"
      - "No files under .kiro/agents, .kairon/specs (other than this spec) or CHANGELOG.md are modified"
    validation_commands:
      - "grep -n -- '--prompt-file' docs/evaluation.md"
      - "grep -n 'prompt_file' docs/evaluation.md"
      - "task sync:check"

  - id: "validate-complete"
    agent: "validator"
    description: "Verify every issue acceptance criterion and project QA: build, tests, lint, format, sync check; AC1-AC5 via tests and the manual commands in the spec; confirm .kiro/agents is unchanged by comparing before/after (the worktree is pre-dirtied by environment changes that must not be reverted or committed), and that no candidate prompt or .kairon/iterations directory was created."
    dependencies: ["document-candidate-prompt"]
    acceptance_criteria:
      - "go build, task test, task lint, task fmt:check and task sync:check all pass"
      - "AC1: with --keep-workspaces every kept workspace's agent prompt file equals the candidate"
      - "AC2: .kiro/agents file hashes and git status --porcelain .kiro/agents are identical before and after a candidate run"
      - "AC3: candidate run records prompt_file and a prompt_sha256 different from a run without one"
      - "AC4: 'go run ./cmd/kairon eval --prompt-file x.md' exits non-zero with an 'agent is required' error"
      - "AC5: missing candidate file with the stub backend exits non-zero before any case runs"
      - "Diff is limited to internal/eval, cmd/kairon/cmd and docs/evaluation.md (plus this spec and sentinels)"
    validation_commands:
      - "go build ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "go run ./cmd/kairon eval --prompt-file x.md; test $? -ne 0"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --prompt-file missing.md; test $? -ne 0"
      - "git diff --name-only HEAD -- . ':!.kiro/agents' | grep -Ev '^(internal/eval/|cmd/kairon/cmd/|docs/evaluation.md|.kairon/specs/issue-318-)' ; test $? -eq 1"
```

## Risks and Notes

- `kiro-cli` agent discovery reads `<cwd>/.kiro/agents/`; the staged workspace copy is exactly what it reads, so substituting there is sufficient. A real-model run was not exercised during design; coverage uses the stub backend, which is what the acceptance criteria verify.
- Substituting only the `prompt` file means a candidate cannot change tools/model/resources; that is intentional for this issue (the downstream P3/B3/iteration issues rely only on `--prompt-file` and `prompt_file`).
- `prompt_file` stores the user-supplied path text, not a content identity; identity is `prompt_sha256`. Consumers should key on the hash.
- The `docs/evaluation.md` Evaluation Workflow section currently says to edit `.kairon/agents/architect-prompt.md` (a stale path); the documenter task corrects it while rewriting that guidance.
