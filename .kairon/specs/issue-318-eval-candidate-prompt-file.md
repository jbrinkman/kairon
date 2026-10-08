# Design Spec: Evals — evaluate a candidate prompt without modifying the live agent

Closes #318

## Problem

A prompt iteration today means editing `.kiro/agents/<agent>-prompt.md`. Krew-lead spawns builder and validator from the worktree's `.kiro/agents/`, so a mid-iteration edit changes the pipeline that is building the PR, and an edited planner prompt degrades the planner used day to day. Iterations need to score a candidate prompt while the live agent stays untouched.

## Solution Approach

Add `kairon eval <agent> --prompt-file <path>`. The candidate is injected **only into the per-case workspaces** the harness already builds. Nothing is written under the repository's `.kiro/agents/`.

How the existing harness makes this cheap:

- `newCaseWorkspace` (`internal/eval/workspace.go`) already stages a private `ws/.kiro/` per case (`stageKiro`: fixture > `<evals-dir>/agents` > project `.kiro`) *before* `setPermissions` makes it read-only. Native runs use `ws/` as the agent cwd (kiro-cli discovers `ws/.kiro/agents`); `--sandbox` runs bind-mount `ws/.kiro` read-only. So replacing the prompt file inside `ws/.kiro/agents/` is seen by the agent on both paths, and by nothing else.
- Provenance (`internal/eval/provenance.go`) hashes `config bytes + prompt-file bytes + resources`. Hashing the candidate bytes in place of the live prompt bytes makes `prompt_sha256` "reflect the candidate content" with no new hash scheme; a candidate byte-identical to the live prompt hashes identically (a useful property, documented).

### Key decisions

1. **Where the candidate lands.** The agent config's `prompt` must be a **relative `file://` reference** (e.g. `file://./planner-prompt.md`). The target is `Join("agents", ref)` relative to the staged `.kiro` dir. Inline prompts, absolute `file://` paths, and refs that escape the `.kiro` tree are rejected in pre-flight with a clear error, because the workspace copy would not be what the agent reads (or would write outside the workspace). Resolved once in pre-flight from `locateAgentConfig(agent, false)` (overlay first, then `.kiro/agents`), so every workspace uses the same target.
2. **Staging is remove-then-create.** The target is removed before the candidate is written so a pre-existing entry can never be written through (defence in depth for AC2; `copyTree` already skips symlinks). The candidate always wins over a fixture-provided file at the same path — the user asked for it explicitly. Staging happens after `stageKiro` and before `setPermissions`.
3. **Pre-flight, before any case.** In `RunWithOptions`: (a) option validation at the very top: `--prompt-file` without an agent is rejected (AC4), and `--prompt-file` combined with `--list`, `--perf` or `--cleanup` is rejected (they run no cases through workspaces); (b) after `configure`, the candidate is loaded: must exist, be a regular file, be non-empty (AC5), and the agent config must have a stageable prompt ref. All of it happens before `pinRun`, any result directory, or any model call. No results directory is created on refusal.
4. **State plumbing.** A new `candidate *candidatePrompt` on `runConfig` (same package-state pattern as `keepWorkspaces`/`pins`; set after `configure`, read-only during the run — preserves the documented invariant). `newCaseWorkspace(tc)` signature is unchanged; it reads `cfg.candidate`.
5. **Provenance.** `resolveAgentProvenance(agent, ignoreOverlay)` keeps its signature (many tests call it) and delegates to a new `resolveAgentProvenanceWith(agent, ignoreOverlay, override *candidatePrompt)`. With an override, the live prompt file is not read (it need not even exist) and the candidate bytes are hashed as the `prompt` part. `agentProvenance` gains `PromptFile`.
6. **Recorded field.** `prompt_file` (omitted when empty, so existing result shapes and parity tests are unchanged) is added to `AgentResult`, to `AgentProvenance` (per-agent entry in `summary.json`), and to the single-agent top-level `Summary` fields — exactly alongside `prompt_sha256`. The value is the path **as given on the command line**, slash-normalised (not made absolute, so results stay machine-independent). `runPins.applyTo` and `updateIncrementalSummary` carry it, so full, single-case and `--resume` runs all record it.
7. **Resume.** The candidate content is part of `prompt_sha256`, so the existing `checkResumeIntegrity` already refuses to resume a candidate run without the same `--prompt-file` (and vice versa). A test pins this; no new resume logic.
8. **Candidate location convention.** Candidates live under `.kairon/iterations/<agent>/` (issue constraint). The flag does **not** enforce a location (tests use temp paths); the convention is documented. This change creates no candidate prompt and does not create `.kairon/iterations/`.
9. **Not changed.** `.kiro/agents/**`, `cmd/kairon/templates/**`, `.kairon/evals/{cases,fixtures,rubrics}` — so no template re-sync is needed; `task sync:check` is still run as the builder-conventions gate. The judge call is unaffected (only the agent under test sees the candidate). `kairon eval diff` is not changed (it already reports a `prompt_sha256` change).

## Relevant Files

Create:
- `internal/eval/candidate.go` — `candidatePrompt`, `loadCandidatePrompt`, `validateCandidateOptions`, `(*candidatePrompt).stage`.
- `internal/eval/candidate_test.go` — unit tests for the above, provenance override, workspace staging, results shape.

Modify:
- `internal/eval/types.go` — `RunOptions.PromptFile`; `AgentResult.PromptFile`; `AgentProvenance.PromptFile`; `Summary.PromptFile`.
- `internal/eval/config.go` — `runConfig.candidate`.
- `internal/eval/provenance.go` — `agentProvenance.PromptFile`; `resolveAgentProvenanceWith`; `pinRun` passes the override and prints the prompt file in the `🔒 Models:` line; `runPins.applyTo` copies `PromptFile`.
- `internal/eval/workspace.go` — call `cfg.candidate.stage(w.KiroDir)` between `stageKiro` and `setPermissions` in `newCaseWorkspace`.
- `internal/eval/runner.go` — `RunWithOptions` pre-flight; `updateIncrementalSummary` carries `prompt_file`.
- `cmd/kairon/cmd/eval.go` — `--prompt-file` flag, wired into `RunOptions`.
- `cmd/kairon/cmd/eval_test.go` — flag + end-to-end tests (AC1–AC5).
- `docs/evaluation.md` — flag table, Running Evaluations, staged `.kiro` precedence, `prompt_sha256` computation, recorded fields, Evaluation Workflow (iterate on a candidate instead of editing the live prompt).

Relevant, read-only: `internal/eval/execute_case.go`, `internal/eval/containment.go` (`applyRunContext`), `internal/eval/testdata/evals/agents/selftest*.json` (use `file://./selftest-prompt.md`), `Taskfile.yml` (`eval:selftest`, `sync:check`).

## Team Orchestration

Single PR, strictly TDD (AGENTS.md): in each builder task the tests are written first, run, and seen to fail *for the expected reason* (missing symbol/behaviour, not a typo), then the minimal code makes them pass; test + implementation are committed together. Name the failing test observed in the commit/PR text.

- `eval-candidate-core` (internal/eval types, loader, provenance override) → `eval-candidate-wiring` (workspace staging + RunWithOptions + summary) → `cli-prompt-file-flag` (cmd + end-to-end AC tests).
- `docs-evaluation` depends only on `eval-candidate-core` and can run in parallel with `eval-candidate-wiring` and `cli-prompt-file-flag` (it touches only `docs/evaluation.md`).
- `validate-all` (validator, read-only) runs last and checks every acceptance criterion plus the sync/QA gates.

## Step-by-Step Task Breakdown

### Task 1: eval-candidate-core
Tests first (`internal/eval/candidate_test.go`), then code.
- `loadCandidatePrompt(agent, path)`: error for missing file, directory, empty/whitespace-only file; error when the agent config is missing; error when `prompt` is inline, absolute `file://`, or escapes `.kiro`; success returns `Path` (as given, slash-normalised), `Content`, `Target` (e.g. `agents/a-prompt.md`).
- `validateCandidateOptions(agent, opts)`: `PromptFile` with empty agent → error containing "an agent is required" and the flag name; with `List`/`Perf`/`Cleanup` → error; no `PromptFile` → nil.
- `resolveAgentProvenanceWith` with override: sha differs from the live sha, equals the live sha when contents are identical, works when the live prompt file is deleted, sets `PromptFile`; without override behaves exactly as today (existing provenance tests unchanged).
- `types.go` fields and `config.go` `runConfig.candidate`.

### Task 2: eval-candidate-wiring
Depends on Task 1. Tests first, then code.
- `newCaseWorkspace` with `cfg.candidate` set: `ws/.kiro/agents/<prompt>` bytes equal the candidate; the project's `.kiro/agents/<prompt>` bytes and mtime-independent content unchanged; works when the config comes from the `<evals-dir>/agents` overlay; overrides a fixture-provided file; the staged file is read-only for others (set by `setPermissions`); with no candidate the workspace is identical to today.
- `RunWithOptions` pre-flight order: option validation → `configure` → candidate load → `pinRun`; on refusal no `results/` directory exists.
- `pinRun` uses the override; `applyTo` and `updateIncrementalSummary` record `prompt_file` in `<agent>.json`, `summary.agents.<agent>` and single-agent top-level `summary` fields; omitted when no candidate (existing golden/parity tests pass untouched).
- Resume: a run recorded with a candidate refuses `--resume` without the same candidate (existing "prompt or models changed" error) — add the test, no new logic expected.

### Task 3: cli-prompt-file-flag
Depends on Task 2. Tests first (`cmd/kairon/cmd/eval_test.go`), then `eval.go`.
- `--prompt-file` registered (string, default empty) and passed as `RunOptions.PromptFile`; tests reset the package var in cleanup.
- AC4: `evalCmd.RunE(evalCmd, nil)` with `--prompt-file` → error mentioning an agent is required.
- AC5: stub backend self-test with a missing candidate → error, no `results/` dir.
- AC1–AC3 end to end with the stub backend on a temp git repo (reuse `evalSelfTestProject`; add a committed `.kiro/agents/` copy of the selftest agent so AC2 is observable): run once without and once with `--keep-workspaces --prompt-file`; assert each kept workspace's `.kiro/agents/selftest-prompt.md` equals the candidate, `git status --porcelain .kiro/agents` is empty, and `summary.json` has `prompt_file` plus a `prompt_sha256` different from the baseline run. Point `KAIRON_EVAL_WORKSPACE_ROOT` at a `t.TempDir()` so kept workspaces are cleaned.

### Task 4: docs-evaluation
Depends on Task 1 (behaviour is fixed there). Edit `docs/evaluation.md` only:
- Flag table row for `--prompt-file` and a usage example in *Running Evaluations*, including the `.kairon/iterations/<agent>/` convention and "agent argument required".
- *Staged `.kiro` and precedence*: the candidate overrides the staged prompt file (after fixture/overlay/project staging), only inside case workspaces.
- *How `prompt_sha256` is computed*: with `--prompt-file` the `prompt` part is the candidate bytes; identical content ⇒ identical hash.
- *Recorded provenance*: `prompt_file` in `<agent>.json`, `summary.json` (`agents.<agent>` and single-agent top level), as given, omitted without a candidate.
- Refusals: no agent, missing/empty/non-regular file, inline or non-relative prompt ref, combination with `--list`/`--perf`/`--cleanup`; `--resume` needs the same `--prompt-file`.
- *Evaluation Workflow*: replace "Edit `.kairon/agents/architect-prompt.md`" guidance with iterating on a candidate (`kairon eval architect --prompt-file .kairon/iterations/architect/v2.md`), and adopting it afterwards by copying into `.kiro/agents/` (+ template sync) as a separate step.

### Task 5: validate-all
Depends on Tasks 2, 3, 4. Read-only verification of AC1–AC5 and QA gates (see Validation Commands).

## Acceptance Criteria Mapping

| Issue AC | Where satisfied | Verified by |
|---|---|---|
| 1 Candidate used for every case, only workspaces see it | `candidatePrompt.stage` in `newCaseWorkspace` | workspace unit test; cmd e2e kept-workspace comparison |
| 2 `.kiro/agents/` unchanged | nothing writes outside `ws/`; remove-then-create | e2e `git status --porcelain .kiro/agents` empty; unit test comparing project file bytes |
| 3 `prompt_file` + candidate `prompt_sha256` | `resolveAgentProvenanceWith`, `applyTo`, `updateIncrementalSummary` | e2e baseline vs candidate summary |
| 4 No agent rejected | `validateCandidateOptions` at top of `RunWithOptions` | cmd test + `go run … ; echo $?` |
| 5 Missing file rejected before any case | `loadCandidatePrompt` before `pinRun` | cmd test (no `results/`) + `go run … ; echo $?` |

## Validation Commands

```bash
go test ./internal/eval -run 'Candidate|Provenance|Workspace' -count=1
go test ./cmd/kairon/cmd -run 'Eval' -count=1
task test
task lint
task fmt:check
task sync:check
task eval:selftest

# AC4 (non-zero, error says an agent is required)
go run ./cmd/kairon eval --prompt-file x.md; echo $?
# AC5 (non-zero, nothing run)
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --prompt-file missing.md; echo $?
# AC2
git status --porcelain .kiro/agents   # empty
```

## Risks / Notes

- Result-shape changes are additive and `omitempty`; existing tests/golden files and the native-vs-container parity test must pass unchanged.
- An agent whose config uses an inline prompt cannot be iterated with `--prompt-file` (clear error); none of the repo's agents do this.
- Adopting a winning candidate into `.kiro/agents/` (and the template sync that implies) is deliberately out of scope for this issue.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "eval-candidate-core"
    agent: "builder"
    description: "TDD first (write failing tests in internal/eval/candidate_test.go, confirm they fail for the expected reason), then add internal/eval/candidate.go (candidatePrompt, loadCandidatePrompt, validateCandidateOptions, stage), RunOptions.PromptFile, PromptFile fields on AgentResult/AgentProvenance/Summary/agentProvenance, runConfig.candidate, and resolveAgentProvenanceWith (candidate bytes hashed as the prompt part; resolveAgentProvenance keeps its signature)."
    dependencies: []
    acceptance_criteria:
      - "Failing tests were written and observed failing before implementation; test and implementation are in the same commit"
      - "loadCandidatePrompt rejects missing file, directory, empty file, missing agent config, inline prompt, absolute file:// prompt and refs escaping the .kiro tree"
      - "validateCandidateOptions returns an error containing 'an agent is required' when PromptFile is set without an agent, and errors when combined with List, Perf or Cleanup"
      - "resolveAgentProvenanceWith with a candidate yields a different prompt_sha256 than the live prompt, the same sha when contents are identical, and does not read the live prompt file"
      - "resolveAgentProvenance without a candidate behaves exactly as before (existing provenance tests pass unchanged)"
      - "New PromptFile JSON fields are omitempty (prompt_file) so existing result shapes are unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval -run 'Candidate|Provenance' -count=1"
      - "go vet ./internal/eval"

  - id: "eval-candidate-wiring"
    agent: "builder"
    description: "TDD first, then wire the candidate into the run: stage it into each case workspace's .kiro between stageKiro and setPermissions (remove-then-create), add RunWithOptions pre-flight (option validation first, candidate load after configure and before pinRun, no results dir on refusal), make pinRun use the override and print the prompt file, and carry prompt_file through applyTo and updateIncrementalSummary (full, single-case and resume paths). Add a test that --resume without the original --prompt-file is refused."
    dependencies: ["eval-candidate-core"]
    acceptance_criteria:
      - "Failing tests were written and observed failing before implementation; test and implementation are in the same commit"
      - "Every case workspace's staged agent prompt file is byte-equal to the candidate file; the project's .kiro/agents files are byte-unchanged"
      - "The candidate overrides fixture-provided and overlay-provided prompt files only inside the workspace, and works for overlay-sourced agent configs"
      - "A refused candidate (no agent, missing file) creates no results directory and invokes no case"
      - "<agent>.json and summary.json (agents.<agent> and single-agent top level) record prompt_file and a prompt_sha256 reflecting the candidate; both are absent/unchanged without a candidate"
      - "Resume of a candidate run without the same --prompt-file is refused with the existing prompt-changed error"
      - "Existing eval package tests, including native-vs-container parity tests, pass unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval -count=1"
      - "go vet ./internal/eval"

  - id: "cli-prompt-file-flag"
    agent: "builder"
    description: "TDD first in cmd/kairon/cmd/eval_test.go, then add the --prompt-file string flag to the eval command in cmd/kairon/cmd/eval.go and pass it as RunOptions.PromptFile. Add end-to-end stub-backend tests covering AC1-AC5 on a temp git repo with a committed .kiro/agents copy of the selftest agent, resetting the package flag var and pointing KAIRON_EVAL_WORKSPACE_ROOT at a temp dir."
    dependencies: ["eval-candidate-wiring"]
    acceptance_criteria:
      - "Failing tests were written and observed failing before implementation; test and implementation are in the same commit"
      - "--prompt-file is registered on eval with an empty default and a usage string that names the agent requirement"
      - "kairon eval --prompt-file x.md (no agent) returns a non-zero error whose message says an agent is required"
      - "A missing candidate with the stub backend returns an error and creates no results directory"
      - "With --keep-workspaces and a candidate, each kept workspace's .kiro/agents/selftest-prompt.md equals the candidate and git status --porcelain .kiro/agents is empty"
      - "summary.json of a candidate run records prompt_file and a prompt_sha256 different from the baseline run without a candidate"
    validation_commands:
      - "go build ./..."
      - "go test ./cmd/kairon/cmd -count=1"
      - "go run ./cmd/kairon eval --prompt-file x.md; test $? -ne 0"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --prompt-file missing.md; test $? -ne 0"

  - id: "docs-evaluation"
    agent: "builder"
    description: "Update docs/evaluation.md only: --prompt-file flag row and example (with the .kairon/iterations/<agent>/ convention and the agent-required rule), staged .kiro precedence note, prompt_sha256 computation with a candidate, recorded prompt_file fields, refusal cases (including --resume needing the same flag), and rewrite the Evaluation Workflow to iterate on a candidate instead of editing the live prompt. No other files; no candidate prompts created; nothing under .kiro/agents touched."
    dependencies: ["eval-candidate-core"]
    acceptance_criteria:
      - "docs/evaluation.md documents --prompt-file in the flag table and Running Evaluations with a working example"
      - "The prompt_sha256 section states that the candidate bytes replace the live prompt bytes and identical content gives an identical hash"
      - "The recorded-provenance section lists prompt_file in <agent>.json and summary.json and says it is omitted without a candidate"
      - "All refusal cases and the resume rule are documented"
      - "The Evaluation Workflow no longer tells readers to edit the live prompt while iterating"
      - "No files other than docs/evaluation.md are changed by this task"
    validation_commands:
      - "grep -n -e '--prompt-file' docs/evaluation.md"
      - "grep -n 'prompt_file' docs/evaluation.md"
      - "test -z \"$(git status --porcelain .kiro/agents)\""

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of issue #318: confirm AC1-AC5 against the implementation and run the QA and template-sync gates (builder-conventions). Confirm no change under .kiro/agents or cmd/kairon/templates and that no candidate prompt or .kairon/iterations directory was added."
    dependencies: ["eval-candidate-wiring", "cli-prompt-file-flag", "docs-evaluation"]
    acceptance_criteria:
      - "AC1: with --keep-workspaces every kept workspace's agent prompt file equals the candidate file"
      - "AC2: git status --porcelain .kiro/agents is empty after a candidate run"
      - "AC3: a candidate self-test run records prompt_file and a prompt_sha256 different from a run without a candidate"
      - "AC4: kairon eval --prompt-file x.md exits non-zero and the error says an agent is required"
      - "AC5: a missing candidate file exits non-zero before any case runs and creates no results directory"
      - "task test, task lint, task fmt:check, task sync:check and task eval:selftest all pass"
      - "git diff against the base shows no changes under .kiro/agents/ or cmd/kairon/templates/"
    validation_commands:
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "git status --porcelain .kiro/agents"
      - "go run ./cmd/kairon eval --prompt-file x.md; test $? -ne 0"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --prompt-file missing.md; test $? -ne 0"
```
