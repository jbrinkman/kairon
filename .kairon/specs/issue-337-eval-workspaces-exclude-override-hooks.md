# Design Spec: Evals: keep project override hooks out of eval workspaces

Closes #337

## 1. Problem Summary

`stageKiro` (`internal/eval/workspace.go`) copies the project's whole `.kiro/` into every case workspace. Two problems follow.

1. **Leakage.** Kairon's own `*-conventions` override hooks (`builder-conventions`, `planner-conventions`, `validator-conventions`) and untracked local files (`.kiro/settings/`, which `.gitignore` excludes) reach every case. An agent cannot be certified with its override hooks empty, and results depend on the machine that ran them.
2. **Provenance mismatch.** `resolveAgentProvenance` (`internal/eval/provenance.go`) resolves `resources` against the repository working directory, not the staged workspace.
   - A fixture that supplies its own `*-conventions` skill runs one file, but `prompt_sha256` and `resources_present` describe another.
   - Editing Kairon's own override changes the agent's `prompt_sha256`, which `kairon eval verify-log` accepts as a prompt iteration.

Constraint (gap-analysis.md S3.5 item 10): `*-conventions` override hooks are empty in eval workspaces unless a case fixture supplies one.

## 2. Solution Approach

Make the staged workspace the single source of truth. Provenance is computed from the same bytes the agent will see, never from the repository working directory.

### 2.1 Stage a clean `.kiro`

`stageKiro` copies the project `.kiro/` with two filters. Both apply only to the **project layer**. The evals-dir `agents/` overlay and the fixture layer are unchanged, and the precedence stays fixture > evals-dir agents > project.

- **Tracked-only.** Stage only files git tracks. Run `git ls-files -z --cached -- .kiro` in the working directory and build a set of tracked paths. Skip any file not in the set.
  - Tracked files are staged with their current working-tree content, so uncommitted edits to tracked files (such as the pre-existing `.kiro/agents/*.json` edits) are still staged.
  - A file added with `git add` or `git add -N` counts as tracked. A tracked file deleted from the working tree is skipped because it is not on disk.
  - Fallback: if the working directory is not inside a git repository (`git` exits 128 with "not a git repository"), there are no untracked files by definition. Stage the whole tree. Any other `git` failure is returned as an error, so the filter never silently degrades.
  - Run git with `hermeticGitEnv()`, and set `cmd.Dir` to the working directory.
- **No override hooks.** Skip every project path under `.kiro/skills/<name>-conventions/` (the whole subtree). Reuse the existing `auditConventionsSuffix` constant (`-conventions`, in `audit.go`) so the definition has one home.
  - A fixture that supplies `skills/<name>-conventions/...` still gets it, because fixture files are copied first and the existing `fixtureKiro(rel)` skip applies.
  - A fixture that provides only part of a conventions directory gets only its own files. The project's files in that directory are never merged in.

### 2.2 Compute provenance from a staged root

Refactor provenance to be rooted at a directory that has a staged `.kiro/`.

- Add `resolveProvenanceInRoot(root, agent string, cand *candidatePrompt)`. The agent config is `<root>/.kiro/agents/<agent>.json`. The `file://` prompt is relative to the config's directory. `resources` entries (`skill://` / `file://` / bare, glob supported) resolve against `root`, not the process cwd. Absolute paths stay absolute.
  - Add a root-aware `resolveResourceFilesIn(root, entry)`. Keep `resolveResourceFiles(entry)` as a thin wrapper (root `""`) if tests or other callers need it.
  - The hash framing (`config`, `prompt`, `resource` parts, length-framed, no paths hashed) is **unchanged**. For a tree with no conventions or untracked files the hash is byte-identical to today's.
- **Candidate prompt.** Because `stageKiroInto` already writes the `--prompt-file` candidate over the staged prompt, the staged prompt file and any resource that aliases it already hold the candidate bytes. The `promptAlias` special-casing in `resolveAgentProvenanceWith` becomes unnecessary and should be removed rather than kept. Candidate validation errors (`candidatePromptDest`) still surface. Keep ignoring a candidate whose `Agent` differs from `agent`.
- **Agent-level provenance (run pin).** `resolveAgentProvenance(agent, ignoreOverlay)` / `resolveAgentProvenanceWith` keep their signatures. They now:
  1. create a throwaway directory,
  2. stage a baseline `.kiro` into it using the same staging function with no fixture files and `ignoreOverlay` honoured,
  3. call `resolveProvenanceInRoot` on it,
  4. remove the directory.
  
  This is "the workspace a case gets without fixture overrides", so a project override file never changes it (AC4). The `Model` and `ConfigPath` fields still describe the config: `ConfigPath` stays the source path, `.kiro/agents/<agent>.json` or `<evals-dir>/agents/<agent>.json`. Keep `locateAgentConfig` unchanged because `trust.go` and `candidate.go` use it.
- **Per-case provenance.** After `newCaseWorkspace(tc)` stages the workspace, compute `resolveProvenanceInRoot(ws.Dir, agent, cfg.candidate)`. This describes exactly what the case's agent sees, including fixture-supplied `*-conventions`, fixture prompts and fixture resources.

### 2.3 Record it per case

- Add to `CaseResult` (`types.go`):
  - `PromptSHA256 string \`json:"prompt_sha256,omitempty"\``
  - `ResourcesPresent []string \`json:"resources_present,omitempty"\``
  - A `CaseResult.MarshalJSON` following the existing `Summary.MarshalJSON` pattern: when `PromptSHA256 != ""`, emit `resources_present` even when empty (`[]`), and omit both fields otherwise. Results from unpinned runs and results written before this change keep their current shape.
- In `executeCase`, after `newCaseWorkspace` succeeds and only when the run is pinned for the agent (`cfg.pins.pinOf(rubric.Agent)` reports ok), resolve the per-case provenance and set `cr.PromptSHA256` / `cr.ResourcesPresent`. A resolution error is reported like a workspace failure (`❌ (workspace failed)`, `ErrorContext`, `scoreCaseFn`, return).
- Each agent `calls[]` record for that case should carry the case's `prompt_sha256` rather than the agent-level one, so the call record cannot disagree with the case. After `invokeAgent` returns in `executeCase`, overwrite `rec.PromptSHA256` with the workspace value when it is set. This keeps native and container paths identical, since both go through `executeCase`.
- Do **not** change `AgentResult` / `Summary` / `AgentProvenance` shapes. They keep the agent-level baseline values (AC4). `--resume` integrity and `verify-log` keep comparing agent-level values.

### 2.4 Self-test fixture for AC3

`internal/eval/testdata/evals/agents/selftest.json` already lists `skill://.kiro/skills/selftest-conventions/SKILL.md` in `resources`. Add:

- `internal/eval/testdata/evals/fixtures/workspaces/conventions/.kiro/skills/selftest-conventions/SKILL.md` with a short body. `.gitignore` does not ignore `.kiro/skills`, so this is committable.
- `internal/eval/testdata/evals/cases/selftest/stub-conventions-fixture.yaml`: `agent: selftest`, `workspace: conventions`, a minimal stub turn whose response satisfies the `structural_completeness` criterion (copy the shape of `stub-basic.yaml` or `stub-seeded-workspace.yaml`).

Expected result of `task eval:selftest`:
- the `stub-conventions-fixture` case lists `skill://.kiro/skills/selftest-conventions/SKILL.md` in its `resources_present`;
- its `prompt_sha256` differs from, for example, `stub-basic`'s;
- the summary-level `resources_present` is `[]`.

Update any test that hard-counts or enumerates self-test cases. For example, `selftest_test.go` compares the result to the fixtures on disk. `selftest_checks_test.go` has hard-coded counts, so check whether its counts cover only subsets. `sandbox_workspace_test.go` and `parity_sandbox_test.go` iterate cases.

### 2.5 Behavioural consequences to document

- The first run after this change gives a different agent-level `prompt_sha256` for any agent whose project override file existed (the file is no longer hashed). `--resume` of an older interrupted run is refused with the existing "prompt or models changed" message.
- A new agent config or skill that is not yet tracked by git is not staged. The author must `git add` it (`git add -N` is enough).
- Per-case `prompt_sha256` / `resources_present` can differ from the agent-level value only when the case's fixture overrides `.kiro` content.

## 3. Relevant Files

Modify:
- `internal/eval/workspace.go`: `stageKiro` refactor (tracked-only, drop conventions), shared staging function usable on any directory; per-case provenance hook.
- `internal/eval/provenance.go`: root-based resolution, baseline staging for the agent-level pin, root-aware resource resolution, drop the `promptAlias` special case.
- `internal/eval/types.go`: `CaseResult` fields and `MarshalJSON`.
- `internal/eval/execute_case.go`: stamp per-case provenance, override call-record hash.
- `internal/eval/candidate.go`: only if `stageCandidatePrompt` needs to take a kiro-dir or root instead of `*caseWorkspace` so it can be shared with baseline staging. Behaviour is unchanged.
- `docs/evaluation.md`: see Task 4.
- Existing tests that assume project override files are staged or hashed: `provenance_test.go`, `workspace_test.go`, `candidate_test.go`, `pinning_test.go`, `diff_provenance_test.go`, `containment_test.go`, `parity_sandbox_test.go`, `selftest_*_test.go`.

Create:
- `internal/eval/testdata/evals/fixtures/workspaces/conventions/.kiro/skills/selftest-conventions/SKILL.md`
- `internal/eval/testdata/evals/cases/selftest/stub-conventions-fixture.yaml`
- New tests in `internal/eval/workspace_test.go`, `provenance_test.go` and `execute_case_test.go` (or a new `workspace_staging_test.go`).

Read-only or out of scope (do **not** touch):
- Anything under `.kiro/` (including the pre-existing uncommitted edits to `.kiro/agents/*.json`) and `cmd/kairon/templates/`. No template sync is needed.
- `kairon eval audit` (`audit.go`). It already lists override hooks without reading them.
- Historical artifacts listed in `AGENTS.md`.

## 4. Team Orchestration

All work lands in one PR. Tasks 1 to 3 are sequential because they share `workspace.go` and `provenance.go`. Task 4 (docs) depends on the final behaviour. Task 5 is the independent validator pass.

```
stage-clean-kiro -> root-based-provenance -> per-case-provenance -> update-evaluation-docs -> validate-issue-337
```

AGENTS.md TDD rules apply to every code task:
1. Write the failing test first and run it to confirm it fails for the expected reason (not a compile error).
2. Implement.
3. Commit the test and the implementation together.
4. Name the observed-failing test in the commit message.

Tasks that change behaviour covered by existing tests (project `*-conventions` previously staged or hashed) must update those tests in the same commit. Do not commit a red tree.

Commit with Conventional Commits. Do **not** push.

## 5. Step-by-Step Task Breakdown

### Task 1: `stage-clean-kiro`

Failing tests first (`workspace_test.go`, using `chdirTemp` plus `git init` and `git add` helpers):
- A project with tracked `.kiro/skills/foo-conventions/SKILL.md` stages no `skills/foo-conventions`.
- A tracked `.kiro/skills/engine/SKILL.md` is staged.
- A fixture providing `.kiro/skills/foo-conventions/SKILL.md` gets it, and only its files.
- An untracked `.kiro/settings/probe.json` is not staged, and a tracked file with uncommitted edits is staged with its edited content.
- A non-git working directory stages the whole tree except conventions.

Implementation: add `projectKiroFilter` (tracked set via `git ls-files`) and a conventions predicate. Refactor `stageKiro` so the core takes a destination kiro dir, the fixture predicate and `ignoreOverlay`, and the project, overlay and candidate layers are written in the existing order. Task 2 reuses this for baseline staging.

Existing `TestWorkspaceKiroStaging*` tests probably run in a non-git temp dir and keep passing through the fallback. Check them.

### Task 2: `root-based-provenance`

Failing tests first (`provenance_test.go`):
- With a project `*-conventions` resource file present, `resolveAgentProvenance` reports it **absent** from `ResourcesPresent`, and the hash is unchanged by creating, editing or deleting that file (AC4).
- `resolveProvenanceInRoot` on a staged dir containing a conventions file lists it and hashes it.
- An untracked project resource does not affect the hash.
- Candidate-prompt hashing still matches the staged prompt, including when a resource aliases the prompt.
- Hash stability: a tree with no conventions or untracked files hashes identically to the pre-change algorithm. Compare against a hash computed in the test from the same framing.

Implementation:
- Add `resolveProvenanceInRoot` and `resolveResourceFilesIn`.
- Rework `resolveAgentProvenanceWith` to stage a baseline into a temp dir, compute, and clean up (always remove, including on error).
- Remove `promptAlias`.
- Update older provenance tests that expected a project conventions file to be present or hashed. The behaviour change is intended: the asserted behaviour moves to the fixture-supplied case.

### Task 3: `per-case-provenance`

Failing tests first (`execute_case_test.go` / `workspace_test.go` / `types` JSON test):
- With pins set and a fixture supplying a conventions resource, `executeCase` returns a `CaseResult` whose `ResourcesPresent` lists it and whose `PromptSHA256` differs from a no-fixture case. The call record carries the same hash.
- A case without the fixture records `resources_present: []` (serialised) and the baseline hash.
- With no pins, both fields are absent from the JSON, as before.
- `CaseResult` JSON round-trip: `prompt_sha256` present means `resources_present` is always emitted.

Implementation: the `CaseResult` fields and `MarshalJSON`, the `executeCase` wiring from section 2.3, and the self-test fixture and case from section 2.4. Fix the self-test counting tests the new case affects.

Manual verification (record the output in the commit or PR description):
- AC1: `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --keep-workspaces`. Then `ls -d <workspace_dir>/.kiro/skills/*-conventions 2>/dev/null` shows only the `selftest-conventions` case's workspace. Remove the kept workspaces afterwards.
- AC2: create an untracked `.kiro/settings/probe.json` (it is gitignored and untracked). Confirm no kept workspace has it, then delete the probe.
- AC4: run `task eval:selftest`. Create `.kiro/skills/selftest-conventions/SKILL.md`, run again, confirm the summary `prompt_sha256` is unchanged and `resources_present` is `[]`. Delete the probe file afterwards.

### Task 4: `update-evaluation-docs`

Update `docs/evaluation.md`:
- The "Staged `.kiro` and precedence" and Case Workspaces text: the project layer is tracked files only, with `*-conventions` skill directories excluded unless the fixture supplies them.
- "How `prompt_sha256` is computed" and the paragraph that currently says the hash reflects "the files the harness can see from the repository root": hashes now describe the staged workspace, and the agent-level value is the baseline workspace without fixture overrides.
- "A missing resource is normal" bullet: update the example. A project `*-conventions` file is never staged, so it never appears. Only a fixture can supply it.
- The result-file field tables (`<agent>.json` case entries): document per-case `prompt_sha256` and `resources_present`, and when they are present. Explain how they relate to the agent-level fields.
- The resume-refusal and `calls[].prompt_sha256` descriptions where they say the value equals the file-level one.
- A short migration note covering the section 2.5 consequences.

Check `docs/` tests (for example `simulated_user_docs_test.go`) so docs assertions still hold.

## 6. Validation Commands

```
go build ./...
go vet ./...
go test ./internal/eval/... -count=1
go test ./... -count=1
task eval:selftest
task sync:check
git diff --stat -- .kiro cmd/kairon/templates
```

The `git diff` command must show no change by this PR under `cmd/kairon/templates`. The only `.kiro` diff is the pre-existing `.kiro/agents/*.json` edits.

## 7. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "stage-clean-kiro"
    agent: "builder"
    description: "TDD: change stageKiro in internal/eval/workspace.go so the project .kiro layer stages only git-tracked files and skips skills/*-conventions directories unless the case fixture supplies them. Refactor staging into a reusable function that can stage into any directory (needed for baseline provenance). Fall back to staging the whole tree (minus conventions) when the working directory is not a git repository. Write failing tests first and commit tests together with the implementation."
    dependencies: []
    acceptance_criteria:
      - "A failing test was observed first, for the expected reason (not a compile error), and is named in the commit message"
      - "A case workspace contains no .kiro/skills/*-conventions directory when the project has one and the fixture supplies none"
      - "A fixture that supplies .kiro/skills/<name>-conventions/ gets exactly its own files, with no project files merged into that directory"
      - "Untracked files under the project .kiro (for example .kiro/settings/probe.json) are not staged, while tracked files with uncommitted edits are staged with their edited content"
      - "A non-git working directory stages the whole project .kiro except conventions; any other git failure returns an error"
      - "Precedence fixture > evals-dir agents > project and the --prompt-file candidate behaviour are unchanged"
      - "Nothing under .kiro/ or cmd/kairon/templates/ is modified"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'Workspace|Stage|Kiro' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "root-based-provenance"
    agent: "builder"
    description: "TDD: make provenance compute from a staged root. Add resolveProvenanceInRoot and a root-aware resource resolver; rework resolveAgentProvenance/resolveAgentProvenanceWith to stage a throwaway baseline .kiro (no fixture) and hash that, removing the temp dir afterwards; remove the promptAlias special case since the staged candidate already holds the candidate bytes. Update existing provenance tests whose expectations depended on project conventions files being hashed. Commit tests with the implementation."
    dependencies: ["stage-clean-kiro"]
    acceptance_criteria:
      - "A failing test was observed first, for the expected reason, and is named in the commit message"
      - "Creating, editing or deleting a project .kiro/skills/<agent>-conventions/SKILL.md does not change the agent-level prompt_sha256 and leaves resources_present unchanged (AC4)"
      - "resolveProvenanceInRoot resolves config, prompt and resources against the given staged root, so a staged conventions file is listed and hashed"
      - "Untracked project files do not affect the agent-level hash"
      - "For trees with no conventions or untracked files the hash equals the previous algorithm's output (same part framing)"
      - "--prompt-file candidate hashing matches the staged prompt bytes, including when a resource aliases the prompt file; candidate validation errors still surface"
      - "The temp baseline directory is always removed, including on error"
      - "locateAgentConfig is unchanged so trust.go and candidate.go behave as before"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'Provenance|Pin|Candidate|Resume|Trust' -count=1"
      - "go test ./internal/eval/... -count=1"

  - id: "per-case-provenance"
    agent: "builder"
    description: "TDD: add prompt_sha256 and resources_present to CaseResult (with MarshalJSON that always emits resources_present when prompt_sha256 is set), compute them in executeCase from the case's staged workspace when the run is pinned, and use the same hash on the case's agent call records. Add the selftest fixtures (workspaces/conventions with .kiro/skills/selftest-conventions/SKILL.md and cases/selftest/stub-conventions-fixture.yaml) and fix any self-test tests that count or enumerate cases. Commit tests with the implementation."
    dependencies: ["root-based-provenance"]
    acceptance_criteria:
      - "A failing test was observed first, for the expected reason, and is named in the commit message"
      - "Each pinned case result records prompt_sha256 and resources_present computed from that case's staged workspace; unpinned runs and old results keep their JSON shape"
      - "The stub-conventions-fixture self-test case lists skill://.kiro/skills/selftest-conventions/SKILL.md in its resources_present and its prompt_sha256 differs from a case without the fixture (AC3)"
      - "Agent-level (AgentResult/Summary) prompt_sha256 and resources_present stay the baseline values: [] and unchanged when a project selftest-conventions file is created (AC4)"
      - "The case's agent call records carry the same prompt_sha256 as the case"
      - "No kept self-test workspace contains a .kiro/skills/*-conventions directory except the fixture-supplied selftest-conventions one (AC1)"
      - "An untracked .kiro/settings/probe.json never appears in a kept workspace (AC2)"
      - "A per-case provenance resolution error is reported as a workspace failure for that case"
      - "Native and --sandbox paths behave identically (both go through executeCase)"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest --keep-workspaces"

  - id: "update-evaluation-docs"
    agent: "builder"
    description: "Update docs/evaluation.md for the new staging and provenance behaviour: tracked-only project .kiro, conventions excluded unless a fixture supplies them, how prompt_sha256/resources_present are computed from the staged workspace, agent-level baseline versus per-case fields in the result tables, corrected 'missing resource' and 'repository root' text, resume/calls[] wording, and a short migration note (first-run hash change, untracked configs need git add). Make no changes under .kiro/ or cmd/kairon/templates/."
    dependencies: ["per-case-provenance"]
    acceptance_criteria:
      - "docs/evaluation.md states that only git-tracked project .kiro files are staged and that *-conventions skills are staged only from a case workspace fixture"
      - "The prompt_sha256 section describes hashing over the staged workspace and no longer says the hash reflects files visible from the repository root"
      - "Per-case prompt_sha256/resources_present are documented in the case result fields, with their relation to the agent-level fields"
      - "The migration note covers the one-time hash change and the git add requirement for new configs"
      - "Existing docs-asserting tests still pass"
    validation_commands:
      - "go test ./internal/eval/... -count=1"
      - "grep -n 'conventions' docs/evaluation.md"
      - "git diff --stat -- .kiro cmd/kairon/templates"

  - id: "validate-issue-337"
    agent: "validator"
    description: "Read-only verification of all four acceptance criteria of issue #337, the TDD commit discipline from AGENTS.md, and that nothing under .kiro/ or cmd/kairon/templates/ was changed by this work."
    dependencies: ["update-evaluation-docs"]
    acceptance_criteria:
      - "AC1: no kept self-test workspace has a .kiro/skills/*-conventions directory other than the fixture-supplied selftest-conventions, while the repository's .kiro/skills has three *-conventions directories"
      - "AC2: with an untracked .kiro/settings/probe.json present, no kept workspace contains it"
      - "AC3: task eval:selftest output shows the conventions-fixture case listing selftest-conventions in resources_present with a prompt_sha256 different from a case without the fixture"
      - "AC4: summary prompt_sha256 and resources_present ([]) are unchanged after creating .kiro/skills/selftest-conventions/SKILL.md in the repository"
      - "go build, go vet, go test ./... and task sync:check pass"
      - "No diff under .kiro/ or cmd/kairon/templates/ beyond the pre-existing uncommitted .kiro/agents/*.json edits; kept workspaces and probe files are cleaned up"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./... -count=1"
      - "task eval:selftest"
      - "task sync:check"
      - "git diff --stat -- .kiro cmd/kairon/templates"
```
