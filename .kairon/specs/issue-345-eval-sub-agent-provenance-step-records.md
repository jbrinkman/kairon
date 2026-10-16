# Design: Evals — record dispatched sub-agents and import step records

Closes #345

## Context and findings

- **#337 is already merged** in this worktree (`b274d0d Evals: keep project override hooks out of eval workspaces (#354)`). Provenance is therefore already computed from a staged workspace: `resolveAgentProvenanceWith` stages a throwaway baseline `.kiro/` and calls `resolveProvenanceInRoot(root, agent, cand)`; `executeCase` calls `resolveProvenanceInRoot(ws.Dir, …)` for each case's own workspace. Sub-agent provenance must use **the same function and the same staged root**, so a sub-agent is hashed from the bytes the case's workspace holds (including fixture overrides and the evals-dir `agents/` overlay), never from the repository working directory.
- `agentConfigFile` (internal/eval/provenance.go) does not read `toolsSettings.subagent.trustedAgents`. The real `krew-lead.json` has `trustedAgents: [architect, builder, validator, documenter]`.
- `inference.CallRecord` (internal/inference/record.go) has no `step` and no marker for who wrote the record.
- `executeCase` (internal/eval/execute_case.go) appends one agent `CallRecord` per turn, `AgentCost.Add(cost)` per turn, then calls the `score()` closure on every terminal path (agent failure, simulated-user failure, normal end). A failed case sets `cr.ActualOutput = ""` and `cr.ErrorContext`, which makes every criterion score 0 (existing fail-closed pattern, see `stub-empty-response`).
- `.eval/` is harness-owned but agent-writable; `readSimLog` (checks_fact.go) is the precedent for reading a JSONL file from it hardened (`resolveInDir`, no symlink, regular file, `maxCheckFileBytes` cap).
- Tests that run **all** rubrics of `testdata/evals` (`TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent` in exit_status_test.go removes four named rubrics and expects exactly two agents) will see a new `selftest-workflow` rubric; that test's removal list must gain `selftest-workflow.yaml`.
- Nothing under `.kiro/` or `cmd/kairon/templates/` changes, so `task sync:check` is expected to stay green untouched (still run it).

## Solution approach

Two independent additions to the harness, wired through the existing result/summary plumbing.

### A. Sub-agent provenance and `workflow_sha256`

1. `agentConfigFile` gains `ToolsSettings.Subagent.TrustedAgents []string`. Decode errors (a non-list `trustedAgents`) are errors, not silently ignored — otherwise a workflow would silently lose its hash.
2. `resolveProvenanceInRoot` is split: the current body becomes `resolveSingleProvenanceInRoot` (unchanged hashing; `prompt_sha256` byte-identical to today). The new `resolveProvenanceInRoot` calls it for the dispatching agent, then collects sub-agents and, if there are any, fills `SubAgents` and `WorkflowSHA256`.
3. Sub-agent collection: transitive closure over `trustedAgents` (a trusted sub-agent that itself trusts agents runs in the same process too), with a visited set (cycle-safe; the dispatching agent itself is skipped). Each name must match `^[A-Za-z0-9][A-Za-z0-9._-]*$` (no path traversal) — otherwise an error. Each sub-agent is resolved with `resolveSingleProvenanceInRoot(root, name, nil)`; a `--prompt-file` candidate only ever applies to the dispatching agent. A trusted agent whose config is not in the staged `.kiro/agents/` is an error naming both the sub-agent and the dispatching agent (fail closed: a workflow hash over a missing step would be misleading); `pinRun` reports it as a violation like any other provenance error.
4. Recorded per sub-agent: `agent_model` (config `model`, trimmed — sub-agents run with their own config model; this is *recorded*, not checked against `evals.allowed_models`, which stays a rule for the agent under test), `prompt_sha256` (same algorithm as the dispatching agent), `resources_present` (non-nil, config order).
5. `workflow_sha256` = lowercase hex SHA-256 over length-framed parts (`writeHashPart`), for the dispatching agent first and then every sub-agent sorted by name: part `workflow-agent` = the agent name, part `prompt-sha256` = that agent's `prompt_sha256` hex. Names are hashed so swapping prompts between steps changes the hash; paths are never hashed. It is computed over the sub-agents' *hashes*, never the other way round: the dispatching agent's own `prompt_sha256` is computed first from its own files only, so it is unchanged by a sub-agent edit (AC 2). An agent with no trusted agents has no `sub_agents` and no `workflow_sha256` (field omitted), so existing results and summaries are unchanged.
6. Shapes (all `omitempty`, so non-workflow results keep their exact shape):
   - new `SubAgentProvenance{AgentModel, PromptSHA256 string; ResourcesPresent []string}` (JSON `agent_model`, `prompt_sha256`, `resources_present` — the last never omitted, `[]` when empty).
   - `agentProvenance`: `SubAgents map[string]SubAgentProvenance`, `WorkflowSHA256 string`.
   - `CaseResult`: `SubAgents`, `WorkflowSHA256` (`sub_agents`, `workflow_sha256`) — from the case's own staged workspace.
   - `AgentResult`: same two fields — from the baseline workspace via `runPins.applyTo`.
   - `AgentProvenance` (summary.json `agents.<agent>`): same two fields, so the summary lists the dispatching agent's provenance and, nested under `sub_agents`, each trusted agent's. Sub-agents are deliberately **not** added as keys of `agents` (that would break the "exactly one agent ⇒ top-level provenance fields" rule and make every consumer treat steps as agents under test).
7. Surfaces: `pinRun`'s `🔒 Models:` line is followed by one line per workflow agent listing `sub-agents: name=model (prompt sha256 xxxxxxxx…), … ; workflow sha256 xxxxxxxx…`. The resume guard (`runner.go` ~2076) also compares `WorkflowSHA256` and mentions it in the refusal message, so a resume after a sub-agent edit cannot mix versions. `verify-log` and `diff_provenance` are unchanged (they track `prompt_sha256` iterations; a sub-agent-only edit is not a prompt iteration of the agent under test) — noted as a Stage 4 follow-up, not part of this issue.

### B. Importing step records from `.eval/calls.jsonl`

1. `inference.CallRecord` gains `Step string \`json:"step,omitempty"\`` and `Source string \`json:"source,omitempty"\``. `Source` is `"workspace"` (exported const `inference.SourceWorkspace`) on imported records and empty on records the harness itself made, so existing call-record JSON is unchanged.
2. New file `internal/eval/steprecords.go`:
   - `const stepRecordsRel = ".eval/calls.jsonl"`.
   - `readStepRecords(dir string) ([]inference.CallRecord, error)`: absent file ⇒ `nil, nil` (normal). Any other problem — symlink, non-regular, larger than `maxCheckFileBytes`, unreadable, changed while read — is an error. Same hardening as `readSimLog`.
   - One record per non-blank line. Per line: must be a JSON object decoding into `CallRecord` (unknown fields tolerated so the Stage 4 tool can add fields without breaking the harness; they are not preserved); `step` and `agent` must be non-empty; `role` empty ⇒ `"agent"`, any other value than `"agent"` is rejected (imported cost is *agent* cost; a judge-role record would be misfiled); `input_tokens`, `output_tokens`, `cost_usd`, `duration_ms` must not be negative (a workspace must not be able to lower the case cost). A bad line yields an error `.eval/calls.jsonl line N: <reason>` (1-based physical line number, blank lines counted).
   - **All-or-nothing**: if any line is bad, no record is imported (the case is zero-scored anyway; a half-imported step list would look authoritative).
   - Every accepted record gets `Source = "workspace"` forced by the harness (whatever the file said).
   - `importStepRecords(cr *CaseResult, dir string) error`: on success appends the records to `cr.Calls` and, for each, `cr.AgentCost.Add(CostInfo{TokensIn, TokensOut, EstimatedUSD: cost_usd, Model: model, UsageSource: "reported" if !estimated else "estimated"})` — imported tokens/cost count toward `agent_cost` (AC 4). `cost_usd` is taken as given (the dispatch tool computes it).
3. Hook point: the first statement of the `score()` closure in `executeCase`. That is after the turn loop and after the container session is closed (normal path), and also covers the agent-failure and simulated-user-failure paths, where step records of a failed workflow are exactly what is needed to attribute the failure. Consequences: records land in `cr.Calls` right after the agent call(s) and before any judge call appended during scoring (AC 3 ordering). The file is cumulative across turns, so it is imported **once per case**, not per turn.
4. Malformed ⇒ fail closed with the existing pattern: print `      Error: …` to the case output; set `cr.ActualOutput = ""`; set `cr.ErrorContext = &ErrorContext{Stderr: msg}` (if an error context already exists from an agent failure, append `\n` + msg to its `Stderr` instead of replacing it). Scoring then runs as for any no-output case, so every criterion scores 0 and the message names the line (AC 5).
5. Known caveat to document: with a real `kiro-cli` run the dispatching agent's own call record is estimated from the outer prompt/response text, and imported step records are **additional** to it. A dispatch tool that also reports the outer call would double count; it should report steps only.

### C. Documentation

`docs/evaluation.md` (live doc, edit freely): add `step` and `source` rows to the per-call table; add a section "Workflows: sub-agent provenance and step records" under *Model Pinning and Run Provenance* covering: the `.eval/calls.jsonl` format (field table, example, validation rules, absent-is-normal, malformed ⇒ score 0 with `line N`, all-or-nothing, `source: "workspace"`, cost accounting and double-count caveat), `sub_agents` (where it appears: per-case, `<agent>.json`, `summary.json`), `workflow_sha256` (exact framing above, omitted when there are no trusted agents, own `prompt_sha256` unaffected, resume refusal), and a note in *How `prompt_sha256` is computed* that sub-agents are not part of an agent's own hash. Docs need no test-first, but a small docs test pins the key terms (same pattern as `checks_docs_test.go`).

## Relevant files

Modify:
- `internal/inference/record.go` — `Step`, `Source`, `SourceWorkspace`
- `internal/inference/record_test.go` — JSON shape tests
- `internal/eval/provenance.go` — trustedAgents parsing, split, sub-agent collection, `workflow_sha256`, `applyTo`, `pinRun` output
- `internal/eval/types.go` — `SubAgentProvenance`; new fields on `CaseResult`, `AgentResult`, `AgentProvenance`
- `internal/eval/execute_case.go` — per-case sub-agent provenance; import hook in `score()`
- `internal/eval/runner.go` — `updateSummary` agent provenance (≈line 2347), resume guard (≈2076)
- `internal/eval/exit_status_test.go` — add `selftest-workflow.yaml` to the removed-rubrics list
- `internal/eval/provenance_test.go`, `internal/eval/execute_case_test.go`, `internal/eval/pinning_test.go` — new tests
- `docs/evaluation.md`

Create:
- `internal/eval/steprecords.go`, `internal/eval/steprecords_test.go`
- `internal/eval/workflow_docs_test.go`
- `internal/eval/testdata/evals/agents/selftest-workflow.json` (+ `selftest-workflow-prompt.md`): `model` `claude-sonnet-5.5`, `tools: [read]`, `allowedTools: [read]`, `toolsSettings.subagent.trustedAgents: ["selftest-step-a", "selftest-step-b"]`. Its `resources` must NOT include the sub-agent files.
- `internal/eval/testdata/evals/agents/selftest-step-a.json`, `selftest-step-b.json` (+ `-prompt.md` each) — sub-agent configs, distinct prompts, **no rubric** (they are not agents under test). `selftest-step-a` carries a resource entry that is absent (e.g. `skill://.kiro/skills/selftest-step-a-conventions/SKILL.md`) to prove `resources_present: []`.
- `internal/eval/testdata/evals/rubrics/selftest-workflow.yaml` (copy of `selftest.yaml` with `agent: selftest-workflow`)
- `internal/eval/testdata/evals/cases/selftest-workflow/stub-workflow.yaml` — AC 1/2: plain stub response with `##`/`###` headings.
- `internal/eval/testdata/evals/cases/selftest-workflow/stub-step-records.yaml` — AC 3/4: stub turn with `usage` (e.g. 100/40) and `commands` that append two valid records to `.eval/calls.jsonl` (`printf '%s\n' '{"role":"agent","agent":"selftest-step-a","step":"design","model":"stub-model","input_tokens":10,"output_tokens":5,"cost_usd":0.000105,"estimated":false,"duration_ms":1}' >> .eval/calls.jsonl`, and a second for `selftest-step-b` / `build`).
- `internal/eval/testdata/evals/cases/selftest-fail/calls-malformed-line.yaml` — AC 5: agent `selftest-fail`; commands write one valid line, then a blank line, then `not json` (so the error must say line 3); expected to fail (score 0).

Not touched (out of scope): `.kiro/agents/*`, `cmd/kairon/templates/*`, the dispatch tool/MCP server.

## Team orchestration

- `record-fields` (inference package) and `subagent-provenance` (eval provenance + types + testdata agents) are independent and run in parallel.
- `wire-provenance` needs `subagent-provenance` (shared types, `execute_case.go`, `runner.go`).
- `step-records` needs `record-fields` (new CallRecord fields) and `subagent-provenance` (the `selftest-workflow` agent/rubric it adds cases to); it also edits `execute_case.go`, so it runs **after** `wire-provenance` to avoid a conflicting edit of the same file.
- `docs` needs `wire-provenance` and `step-records` (documents their final behaviour); `validate-all` is last.
- TDD per AGENTS.md: each builder task writes its failing test first, confirms it fails for the expected reason, then implements; test and implementation are committed together; the commit message names the test seen failing.

## Step-by-step task breakdown

### Task 1 — `record-fields`: CallRecord `step` and `source`
Tests first (`record_test.go`): a record with `Step`/`Source` marshals to JSON containing `"step"`/`"source"`; a record without them marshals with neither key (existing shape unchanged); an agent record round-trips. Then add the fields and `SourceWorkspace = "workspace"`.
Dependencies: none.

### Task 2 — `subagent-provenance`: resolve sub-agents and `workflow_sha256`
Tests first (provenance_test.go, using `provProject`-style temp projects):
- agent with `trustedAgents` ⇒ `SubAgents` has each trusted agent's trimmed model, 64-hex `PromptSHA256`, non-nil `ResourcesPresent`; `WorkflowSHA256` is 64-hex and stable across runs.
- editing a sub-agent's prompt changes `WorkflowSHA256` and that sub-agent's hash but **not** the dispatching agent's `PromptSHA256` (AC 2).
- editing the dispatching agent's prompt changes both; swapping two sub-agents' prompts changes `WorkflowSHA256`.
- an agent without `trustedAgents` (or with an empty list) has `SubAgents == nil`, `WorkflowSHA256 == ""`, and its `PromptSHA256` equals the value computed before this change (pin the golden hash used by existing tests).
- transitive trust and cycle (a↔b, a trusts itself) terminate and list each agent once.
- errors: trusted agent without a config (message names sub-agent and dispatcher), name with `/` or `..`, non-list `trustedAgents`.
- a sub-agent override supplied by the evals-dir overlay / a fixture-staged skill resource is reflected (staged-workspace rule); `--prompt-file` candidate for the dispatcher does not touch sub-agent hashes.
Then implement per section A.1–A.6 (`provenance.go`, `types.go`) and add the three `selftest-*` agent configs/prompts, the `selftest-workflow` rubric and `stub-workflow.yaml`; add `selftest-workflow.yaml` to the removed-rubrics list in `exit_status_test.go`.
Dependencies: none.

### Task 3 — `wire-provenance`: surface sub-agent provenance in results, summary, pin line and resume
Tests first: (a) `executeCase` on a pinned `selftest-workflow` run sets `cr.SubAgents` and `cr.WorkflowSHA256`; a case whose fixture supplies a sub-agent skill resource has a different `workflow_sha256` from a case without; (b) a stub run `RunWithOptions("selftest-workflow", …)` writes `summary.json` with `agents.selftest-workflow.sub_agents.{selftest-step-a,selftest-step-b}` (model, prompt_sha256, resources_present) and `workflow_sha256`, and `<agent>.json` carries the same; a plain `selftest` run's JSON contains neither key; (c) resume refusal when the recorded `workflow_sha256` differs from the current one; (d) captured stdout of the run contains the `sub-agents:` pin line. Then implement `applyTo`, `executeCase` per-case fill, `updateSummary`, resume guard and the `pinRun` line.
Dependencies: `subagent-provenance`.

### Task 4 — `step-records`: import `.eval/calls.jsonl`
Tests first (`steprecords_test.go`, plus an `executeCase` test with the stub backend): absent file ⇒ no records, no error; two valid lines ⇒ two records with `Source == "workspace"` and `Step`/`Agent` intact, appended after the agent call and before judge calls, and `AgentCost` equals outer-call usage plus imported tokens/cost (AC 3/4); the harness overwrites a `source` claimed in the file; malformed JSON, missing `step`, missing `agent`, `role: "judge"`, negative tokens/cost, symlinked file, directory at the path, oversize file ⇒ error naming the line (or the problem) and no records imported; blank lines are skipped but counted in line numbers; a malformed file zero-scores the case (all criteria 0, `ErrorContext.Stderr` contains `.eval/calls.jsonl line 3`) (AC 5); when the agent call itself failed, valid step records are still imported and the original `ErrorContext` is kept (malformed ⇒ message appended); multi-turn case imports once. Then implement section B, add the `stub-step-records.yaml` and `calls-malformed-line.yaml` cases, and confirm `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-workflow` passes while `selftest-fail calls-malformed-line` fails with the line-naming error.
Dependencies: `record-fields`, `subagent-provenance`, `wire-provenance`.

### Task 5 — `docs`: document file format, `workflow_sha256`, sub-agent provenance
Edit `docs/evaluation.md` per section C; add `workflow_docs_test.go` asserting the doc mentions `.eval/calls.jsonl`, `workflow_sha256`, `sub_agents`, `"source"` and `step`. Check no historical artifact (`.kairon/specs/`, `CHANGELOG.md`) is edited.
Dependencies: `wire-provenance`, `step-records`.

### Task 6 — `validate-all`: independent verification
Read-only run of the full checks and every acceptance-criterion verification below.
Dependencies: `docs`.

## Acceptance-criteria mapping

| AC | Where satisfied | Verified by |
|----|-----------------|-------------|
| 1 | A.1–A.6, `selftest-workflow` + `selftest-step-a/b` testdata | `go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-workflow` → `summary.json` `agents.selftest-workflow` and its `sub_agents`; pin line; tests in tasks 2–3 |
| 2 | A.5 (hash order of computation) | provenance test "sub-agent edit"; manual: edit `selftest-step-a-prompt.md`, re-run → `workflow_sha256` changes, `prompt_sha256` does not (revert the edit afterwards) |
| 3 | B.2–B.3 | `stub-step-records` case; `executeCase` test |
| 4 | B.2 cost accounting | `agent_cost` assertion in the same test |
| 5 | B.4 | `selftest-fail/calls-malformed-line` case; unit tests |
| 6 | C | docs test + manual inspection |

## Validation commands

```
go build ./...
go vet ./internal/eval/... ./internal/inference/...
go test ./internal/inference/... ./internal/eval/... -count=1
task fmt:check
task sync:check
task lint
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-workflow
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-fail calls-malformed-line   # expected to fail (exit non-zero) with the line-naming error
```

Running evals writes `internal/eval/testdata/evals/results/` (git-ignored); remove the run directories created during validation.

## Risks and decisions

- **Double counting** with a real backend (outer estimated call + step records) — documented, not solved here (dispatch tool is out of scope).
- **Unknown fields** in a step record are tolerated and dropped rather than rejected, to keep the Stage 4 format extensible; the documented fields are the contract.
- **Sub-agent model not enforced** against `evals.allowed_models` — the issue asks to record; enforcement would refuse existing krew-lead evals and is a separate decision.
- **All-or-nothing import** on a malformed file — chosen for determinism; the error names the first bad line.
- Existing tests that run every rubric of `testdata/evals` need the one-line update in Task 2; the builder must run the whole `internal/eval` suite to catch any other enumeration.

## Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "record-fields"
    agent: "builder"
    description: "Add Step and Source fields (and SourceWorkspace constant) to inference.CallRecord, test first, keeping existing JSON unchanged when the fields are empty"
    dependencies: []
    acceptance_criteria:
      - "CallRecord has Step (json step, omitempty) and Source (json source, omitempty); const SourceWorkspace = \"workspace\""
      - "A failing test was written first and observed failing for the expected reason; record_test.go covers marshal with and without the fields and a round trip"
      - "A record without step/source marshals byte-identically to before"
    validation_commands:
      - "go build ./internal/inference/..."
      - "go test ./internal/inference/... -count=1"
      - "test -z \"$(gofmt -l internal/inference)\""

  - id: "subagent-provenance"
    agent: "builder"
    description: "Resolve trusted sub-agents' provenance from the staged workspace and compute workflow_sha256; add the new result types and the selftest-workflow / selftest-step-a / selftest-step-b testdata (agents, rubric, stub-workflow case); update exit_status_test.go rubric-removal list. Test first."
    dependencies: []
    acceptance_criteria:
      - "agentConfigFile reads toolsSettings.subagent.trustedAgents; a non-list value is an error"
      - "resolveProvenanceInRoot returns SubAgents (agent_model, prompt_sha256, non-nil resources_present per trusted agent, transitive, cycle-safe, dispatcher excluded) and WorkflowSHA256 framed over agent name + prompt_sha256, dispatcher first then sub-agents sorted by name"
      - "The dispatching agent's PromptSHA256 is unchanged by a sub-agent edit and equal to the pre-change value for agents without trustedAgents; agents without trusted agents get nil SubAgents and empty WorkflowSHA256"
      - "Missing sub-agent config, unsafe sub-agent name and non-list trustedAgents produce errors naming the problem; --prompt-file candidate affects only the dispatching agent"
      - "types.go has SubAgentProvenance and omitempty sub_agents/workflow_sha256 fields on CaseResult, AgentResult and AgentProvenance"
      - "testdata/evals contains selftest-workflow (trusting selftest-step-a and selftest-step-b), the two sub-agent configs and prompts (no rubrics), the selftest-workflow rubric and cases/selftest-workflow/stub-workflow.yaml; exit_status_test.go removes selftest-workflow.yaml in its all-agents test"
      - "Failing tests were observed failing for the expected reason before implementation"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Provenance|Workflow|SubAgent|RunBothAgents' -count=1"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "wire-provenance"
    agent: "builder"
    description: "Surface sub-agent provenance in per-case results, <agent>.json (runPins.applyTo), summary.json agents map, the pinRun output line and the resume guard. Test first."
    dependencies: ["subagent-provenance"]
    acceptance_criteria:
      - "executeCase fills CaseResult.SubAgents/WorkflowSHA256 from the case's own staged workspace"
      - "A stub run of selftest-workflow writes summary.json agents.selftest-workflow.sub_agents for selftest-step-a and selftest-step-b plus workflow_sha256, and <agent>.json carries the same; a plain selftest run contains neither key"
      - "pinRun prints a sub-agents line with models, short prompt hashes and the workflow hash for workflow agents"
      - "Resume is refused when the recorded workflow_sha256 differs from the current one (including a recorded file without it for a workflow agent), and the message mentions workflow_sha256"
      - "Failing tests were observed failing for the expected reason before implementation"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -count=1"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-workflow"
      - "test -z \"$(gofmt -l internal/eval)\""

  - id: "step-records"
    agent: "builder"
    description: "Implement readStepRecords/importStepRecords for .eval/calls.jsonl, hook into executeCase score() so records are appended to Calls (source workspace) and counted in agent_cost, fail closed with a line-naming error on malformed input; add stub-step-records and calls-malformed-line testdata cases. Test first."
    dependencies: ["record-fields", "subagent-provenance", "wire-provenance"]
    acceptance_criteria:
      - "Absent .eval/calls.jsonl is normal; each valid line becomes a CallRecord with Source=workspace appended to cr.Calls after the agent call and before judge calls, with step and agent preserved"
      - "Imported input/output tokens and cost_usd are added to cr.AgentCost; the stub-step-records case agent_cost equals the outer stub usage plus the two imported records"
      - "Malformed JSON, missing step or agent, non-agent role, negative numbers, symlink, non-regular or oversized file produce an error naming .eval/calls.jsonl and the 1-based line; nothing is imported, ActualOutput is empty, ErrorContext.Stderr holds the message and every criterion scores 0"
      - "Records are imported on agent-failure paths too without replacing the original ErrorContext; multi-turn cases import once"
      - "selftest-fail/calls-malformed-line fails with a message naming line 3; selftest-workflow passes"
      - "Failing tests were observed failing for the expected reason before implementation"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ ./internal/inference/ -count=1"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-workflow"
      - "test -z \"$(gofmt -l internal/eval internal/inference)\""

  - id: "docs"
    agent: "documenter"
    description: "Update docs/evaluation.md: add step/source to the call-record table and a Workflows section documenting the .eval/calls.jsonl format and validation, cost accounting and double-count caveat, sub_agents provenance (case, agent file, summary), workflow_sha256 definition and resume refusal; add workflow_docs_test.go pinning the key terms"
    dependencies: ["wire-provenance", "step-records"]
    acceptance_criteria:
      - "docs/evaluation.md documents the file format (fields, one record per line, step and agent required, absent file normal, malformed line scores 0 and names the line, source workspace)"
      - "docs/evaluation.md documents workflow_sha256 (framing, omitted without trusted agents, own prompt_sha256 unaffected) and sub-agent provenance (agent_model, prompt_sha256, resources_present; staged-workspace rule)"
      - "workflow_docs_test.go passes and fails if those terms are removed"
      - "No file under .kairon/specs/ or CHANGELOG.md is edited"
    validation_commands:
      - "go test ./internal/eval/ -run 'WorkflowDocs' -count=1"
      - "grep -q 'workflow_sha256' docs/evaluation.md"
      - "grep -q '.eval/calls.jsonl' docs/evaluation.md"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of every acceptance criterion and the full build, test, format, lint and template-sync gates"
    dependencies: ["docs"]
    acceptance_criteria:
      - "go build ./... and go test ./... pass"
      - "task fmt:check, task lint and task sync:check pass"
      - "selftest-workflow stub run lists provenance for selftest-workflow and each trusted agent and passes; sub-agent edit changes workflow_sha256 but not the dispatcher prompt_sha256 (covered by a test or manual check)"
      - "stub-step-records case lists both imported records after the agent call with step and agent, and agent_cost includes their tokens"
      - "selftest-fail calls-malformed-line records an error naming the line and scores 0"
      - "docs/evaluation.md documents file format, workflow_sha256 and sub-agent provenance"
      - "No change under .kiro/agents/ or cmd/kairon/templates/ was introduced by this work"
    validation_commands:
      - "go build ./..."
      - "go test ./... -count=1"
      - "task fmt:check"
      - "task lint"
      - "task sync:check"
      - "go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-workflow"
```
