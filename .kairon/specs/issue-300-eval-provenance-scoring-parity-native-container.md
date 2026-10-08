# Design Spec: Evals — provenance and scoring parity across native and container runs

Closes #300

## 1. Problem (verified in code at `4213d56`)

A `--sandbox` run and a native run of the same case must be comparable. Reading `internal/eval` shows what already holds and what does not.

**Already shared (do not fork):**

| Concern | Where | Path-dependent? |
|---|---|---|
| Pinned models, `prompt_sha256`, `resources_present` | `pinRun` → `cfg.pins.applyTo(&result)` in `evaluate` / `evaluateProgressive` | No. `RunWithOptions` calls `pinRun(agent, options, false)` before the sandbox branch, so both paths pin identically. |
| Per-call `model` / `prompt_sha256` | `newAgentRequest` + `completeAgentCall` (`rec.PromptSHA256 = cfg.pins.agentPromptSHA`) | No. Native and container use the same two functions (#303). |
| Case scoring | `executeCase` → `scoreCase` | No. One path (#287). The container only adds `containerWorkspaceDir` path rewriting for deterministic checks. |
| Agent score | `updateIncrementalSummary` (`totalScore/totalMax`, skipped criteria excluded) | No, but the arithmetic is duplicated three times: `updateIncrementalSummary`, `buildSummary` (dead code) and `printCaseResult`. |

**Gaps this issue closes:**

1. `Summary` (`summary.json`) has **no execution-mode field** and no containment record. `AgentResult.Sandbox *bool` exists (added for resume safety) but is a bool pointer private to the per-agent file, and:
   - `evaluate()` (the single-case `--testcase` path) never sets it, so a `--testcase --sandbox` run is indistinguishable from a native one.
   - nothing records *what* contained the run (tool-trust set, fake `gh`, read-only FS, network).
2. `eval diff` loads only scores and costs. It never reads provenance, so comparing a native run with a container run, or two runs with different judge models or prompts, prints deltas with no warning that the runs are incomparable.
3. Parity is asserted nowhere: the existing `TestSelftestSandbox` compares `actual_output`, `agent_cost` and the call model, but not the run-level provenance, the summary, the score denominators or the diff.
4. The score and denominator arithmetic exists in three copies. The E7 per-agent threshold (see below) will be added to this code; if it lands in only one copy, native and container summaries could diverge.

### E7 is not in the tree

Issue #300 says "the E7 threshold verdict is computed the same way regardless of execution path" and "the threshold/denominator rule (E7) is shared code". E7 (`.kairon/specs/maturity-model/gap-analysis.md`, "Per-agent pass threshold and exit code") is **not implemented** at this commit: there is no `pass_threshold`, no `passed` / `threshold` / `cases_failed` in `Summary`, no per-agent PASS/FAIL line, and `kairon eval` exits 0 regardless. The issue lists the dependencies as #296, #298 and #294 only. This issue therefore does **not** implement E7 (out of scope: "New check types or threshold logic (E5–E7)"). It guarantees parity structurally and by test:

- one shared denominator helper that E7 will extend (task `shared-score-totals`);
- a parity test that compares the **whole** `Summary` structurally between a native and a container run after normalising only the fields that are meant to differ (`sandbox`, `containment`, per-run directory names). If E7's fields (`score`, `threshold`, `passed`, `cases_total`, `cases_failed`) exist by the time this lands, they are compared automatically, with no test change.

## 2. Solution Approach

Record the execution context once, through the same code that records the model pins, so no path can skip it.

```
RunWithOptions -> pinRun (shared)  -> cConfig = nil | createContainerConfig
evaluate / evaluateProgressive:
    result := AgentResult{...}
    applyRunContext(&result, cConfig)      // NEW: pins.applyTo + Sandbox + Containment, one call site per path
    ... executeCase (unchanged, shared) ...
updateIncrementalSummary(result)           // writes Summary.Sandbox + Summary.Containment[agent]
Diff -> loads summary + agent files -> "Run Provenance" block, differences flagged
```

Decisions:

1. **Field names.** Inherited provenance fields (`agent_model`, `judge_model`, `prompt_sha256`, `resources_present`, `agents`) are untouched. New, all optional (`omitempty`, back-compatible):
   - `Summary.Sandbox string` — `"native"` or `"container"` (`type RunMode string`). Satisfies "`summary.json` has `sandbox: container`".
   - `Summary.Containment map[string]Containment` — keyed by agent, like `Summary.Agents`, because the tool-trust set is per agent. Absent for native runs.
   - `AgentResult.Containment *Containment` — what applied to that agent's cases.
   - `AgentResult.Sandbox *bool` is **kept as is** (type, name, resume semantics). Changing it to a string would break resume of existing files; `Summary.Sandbox` is derived from it. The two files therefore spell the same fact differently (`true`/`false` in `<agent>.json`, `"container"`/`"native"` in `summary.json`); document this.
2. **`Containment` struct** (the containment summary of AC2):

   ```go
   type Containment struct {
       ToolTrust  []string `json:"tool_trust"`   // normalised --trust-tools names; [] = trust nothing (not omitempty)
       FakeGH     bool     `json:"fake_gh"`
       ReadOnlyFS bool     `json:"read_only_fs"`
       Network    string   `json:"network"`      // always "unrestricted"
   }
   ```

3. **Single source of truth for each containment fact**, so the record cannot drift from what was enforced:
   - `ToolTrust` = `resolveTrustSet(agent).Names()`, the same function and inputs `invokeAgent` uses for the call, so it equals `trusted_tools` on the agent call records.
   - `ReadOnlyFS` = a new exported const `sandbox.ReadonlyRootfs = true` that `NewHostConfigWithMounts` uses for `HostConfig.ReadonlyRootfs`.
   - `FakeGH` = a const next to `containerBinDir` that `buildContainerMounts` honours (the fake `gh` directory is always mounted). A unit test pins both to the real mount list and `HostConfig`.
   - `Network` = const `"unrestricted"`. **Note the wording carefully:** containers are currently created with `NetworkMode: "none"` (`mounts.go`), but docs/evaluation.md already states this is "not a network policy" and the issue's constraint is that network is *recorded as unrestricted, consistent with the mock-guidance decision*. The field records that Kairon makes **no network containment guarantee**, not the runtime's network mode. Docs must say so, so nobody reads `unrestricted` as a statement about `NetworkMode`. Do not edit `NetworkMode`.
4. **Native runs** record `sandbox: native` and no containment (nothing is contained). They do not invent a containment block with `fake_gh:false`; absence is the correct signal and keeps old and new native summaries shape-compatible.
5. **Resume safety extends to the summary.** `checkResumeIntegrity` already refuses a mode change per agent file. Add the same check against `summary.json`'s `sandbox` (mirroring the existing run-level `judge_model` check), because a multi-agent run interrupted before an agent's file exists has nothing per-agent to compare. `updateIncrementalSummary` also refuses to overwrite a different recorded mode (defence in depth: one summary never mixes modes).
6. **Shared denominator.** Extract `caseTotals(CaseResult) (score, max int)` and `agentScoreTotals(AgentResult) (score, max float64)` into `scoring.go`; `printCaseResult`, `updateIncrementalSummary` and `buildSummary` all call them. Behaviour is byte-identical today (skipped criteria excluded). E7 will change the rule in this one place.
7. **`eval diff`.** Add a "Run Provenance" block before the per-agent deltas. It always prints each run's execution mode and lists every difference in: `sandbox`, containment (per agent), `judge_model`, per-agent `agent_model`, per-agent `prompt_sha256`. A differing mode gets an explicit `⚠` line ("runs used different execution modes; scores are not directly comparable"). It never fails the diff. Runs that predate the fields show mode `unknown (predates sandbox-mode tracking)`; mode is read from `summary.sandbox`, else from the agent files' `sandbox` bool (all agents must agree), else unknown. Output goes through an `io.Writer` (`diffTo`) so it can be tested; `Diff` stays the public entry point writing to stdout.
8. **Provenance parity (AC1)** needs no new emission code: `applyTo` and the call-record path are already shared. The deliverable is the proof: tests assert identical `agent_model`, `judge_model`, `prompt_sha256`, `resources_present`, and per-call `model` / `prompt_sha256` between a native and a container `selftest` run. The one real provenance gap, the single-case path not stamping the run mode, is fixed by routing it through `applyRunContext`.

### What stays untouched

Pin/allowlist logic, `provenance.go` hashing, `CallRecord` shape, `ErrorContext.container_id` (still the per-case container marker), scoring rules, `NetworkMode`, the cases/rubrics under `.kairon/evals/`, `.kairon/evals/results/` (historical output, per AGENTS.md), `CHANGELOG.md`, `.kairon/specs/` other than this file. No template-synchronized file is modified, so `task sync:check` needs no re-sync (it must still pass).

## 3. Relevant Files

| File | Change |
|---|---|
| `internal/eval/types.go` | `RunMode`, `Containment`, `Summary.Sandbox`, `Summary.Containment`, `AgentResult.Containment` |
| `internal/eval/containment.go` (new) | `containmentFor(agent)`, `applyRunContext(&AgentResult, *ContainerConfig)`, consts `networkUnrestricted`, `fakeGHInstalled` |
| `internal/eval/scoring.go` | `caseTotals`, `agentScoreTotals`; `printCaseResult` uses them |
| `internal/eval/runner.go` | `evaluate`, `evaluateProgressive` (2 sites) call `applyRunContext`; `updateIncrementalSummary` writes mode/containment and uses `agentScoreTotals`; `buildSummary` uses it; `checkResumeIntegrity` adds the summary-level mode check |
| `internal/eval/sandbox/mounts.go` | exported const `ReadonlyRootfs` used by `NewHostConfigWithMounts` |
| `internal/eval/diff.go` | `diffTo(io.Writer, …)`, provenance loading and comparison, "Run Provenance" block |
| `internal/eval/testdata/evals/fixtures/runs/` (new) | committed result-directory fixtures: `legacy-native` (pre-#294 shape), `native-pinned` (post-#294, no sandbox fields). Not under `testdata/evals/results/`, which is git-ignored |
| `internal/eval/containment_test.go`, `diff_provenance_test.go`, `parity_sandbox_test.go` (new) | tests below |
| `Taskfile.yml` | add the gated parity test to `eval:selftest:sandbox` |
| `docs/evaluation.md` | document the new fields, the parity guarantee, the `unrestricted` wording, `eval diff` output, resume check |

## 4. Team Orchestration

Sequential core, then tests and docs:

```
shared-score-totals -> record-run-mode -> diff-provenance -> parity-tests -> docs -> validate-all
```

`record-run-mode` and `shared-score-totals` both edit `runner.go`, and `diff-provenance` needs the new types, so they are ordered rather than parallel. No task spawns work for another. The builder runs one task at a time; the validator runs last and is read-only.

## 5. Step-by-Step Task Breakdown

### Task 1: `shared-score-totals`
Extract the score/denominator arithmetic into `scoring.go` (`caseTotals`, `agentScoreTotals`) and use it from `printCaseResult`, `updateIncrementalSummary` and `buildSummary`. Pure refactor: skipped criteria stay out of both numerator and denominator, exactly as today. Add table tests (skipped, errored case with no scores, zero denominator). No file format change.

### Task 2: `record-run-mode`
Add the types; add `sandbox.ReadonlyRootfs`; write `containment.go`; replace the three `cfg.pins.applyTo(&result)` calls (one in `evaluate`, two in `evaluateProgressive`) with `applyRunContext(&result, cConfig)`; write `sandbox` and `containment` in `updateIncrementalSummary` (native: `sandbox: native`, no containment, and any stale containment entry for the agent removed); add the summary-level mode check to `checkResumeIntegrity` and the conflicting-mode refusal in `updateIncrementalSummary`. `applyRunContext` must not change any provenance field.

Details:
- `applyRunContext`: call `cfg.pins.applyTo(r)`, set `r.Sandbox = &isContainer`, set `r.Containment = &c` for a container run, `nil` for native (clear it when a resumed file carries one from a previous process).
- If `resolveTrustSet` fails (no override and the agent config is unreadable) record `tool_trust: []` (fail-closed, the call also fails) and print a one-line warning. Stamping never fails the run.
- Resume: a legacy file (`Sandbox == nil`) keeps its existing "refuse if it has cases" behaviour.

### Task 3: `diff-provenance`
Rework `Diff` into `diffTo(w io.Writer, runA, runB string)` (public `Diff`/`DiffWithOptions` unchanged in signature). Load each run's provenance (`runInfo{mode, judgeModel, agents{model, sha}, containment}`) from `summary.json` plus agent files, derive mode with the fallbacks in §2.7, and print the "Run Provenance" block before the existing output. Everything after it is unchanged. Missing optional fields never error; a malformed agent file keeps the existing skip behaviour.

### Task 4: `parity-tests`
- Fixtures `fixtures/runs/legacy-native` and `fixtures/runs/native-pinned` (copied by tests into a temp evals dir).
- Hermetic tests (no daemon): `applyRunContext` native vs container (provenance fields identical, `Sandbox`, `Containment` correct, containment equals `trusted_tools` of `resolveTrustSet`, `ReadOnlyFS` equals `sandbox.NewHostConfigWithMounts(...).ReadonlyRootfs`, `FakeGH` equals presence of the `containerBinDir` mount from `buildContainerMounts`); `updateIncrementalSummary` mode/containment, conflicting-mode refusal; resume refusal on summary mode mismatch; shared totals give the same score from native-built and container-built `AgentResult`s; `diffTo` output for native↔container (contains the mode difference and `⚠`), legacy↔native, legacy↔container (unknown mode, no error), same-mode runs (no `⚠`).
- Gated test `TestProvenanceParitySandbox` (needs `KAIRON_EVAL_SANDBOX_SELFTEST=1` and a daemon, same gating as `TestSelftestSandbox`): runs `selftest` and `selftest-fail` natively and with `--sandbox` (stub backend, `internal/eval/testdata/evals`) and asserts (a) identical provenance fields in `<agent>.json`, in `summary.json`, and on every agent/judge `calls[]` record (`model`, `prompt_sha256`), (b) native summary `sandbox: native` without containment, container summary `sandbox: container` with the containment block, (c) the whole `Summary` equal after zeroing `Sandbox`/`Containment`, which covers `agent_scores`, the denominators and any E7 fields, (d) per-case `caseTotals` and the `pct >= getThreshold(tc)` outcome equal for every case, (e) `diffTo` between the two runs succeeds and reports the mode difference.
- Add `TestProvenanceParitySandbox` to the `eval:selftest:sandbox` test regex in `Taskfile.yml`.

### Task 5: `docs`
Update `docs/evaluation.md` (§6) and fix any statement the change makes stale.

### Task 6: `validate-all`
Read-only verification of every acceptance criterion and the full check suite.

## 6. Documentation Changes (`docs/evaluation.md`)

- **Run summary** subsection: add `sandbox` and `containment` to the example and field table; note that `<agent>.json` keeps `sandbox` as a boolean (`true` = container) and that `summary.json` spells it `native`/`container`.
- New subsection **Execution mode and containment**: the `containment` fields; native runs record `sandbox: native` and no containment; why `network` is always `unrestricted` (no network guarantee is made; this is not the container's `NetworkMode`; reference the existing Limits text and the planned mock-guidance follow-up without inventing an issue number).
- **Parity guarantee**: provenance and scoring come from the same code on both paths; the denominator rule lives in one place; the sandboxed record differs from the native one only by `sandbox`, `containment` and `trusted_tools` (update the existing "match, except that the sandboxed agent record also carries `trusted_tools`" sentence).
- **Comparing Runs**: describe the Run Provenance block, the `⚠` mode-difference line, `unknown` for old runs, and that old result directories still load.
- **Resume refusal**: mention the summary-level sandbox-mode check.
- State plainly that the per-agent threshold verdict (E7) is separate work and that parity of its inputs is what this change guarantees.
- Do not touch `CHANGELOG.md` or other `.kairon/specs/` files.

## 7. Acceptance Criteria Traceability

| AC | Delivered by | Verified by |
|---|---|---|
| 1. Same provenance on container path | shared `pinRun`/`applyTo`/call-record code (unchanged) + `applyRunContext` on the single-case path | hermetic `applyRunContext` test; gated parity (a) |
| 2. Mode + containment recorded | `Summary.Sandbox`, `Summary.Containment`, `AgentResult.Containment`; network `unrestricted` | hermetic summary test; gated parity (b) |
| 3. Same verdict and denominator | `caseTotals` / `agentScoreTotals` shared; whole-`Summary` comparison | shared-totals tests; gated parity (c), (d) with `selftest-fail` |
| 4. `eval diff` native vs container | Run Provenance block | `diffTo` tests; gated parity (e) |
| 5. Old results load | optional fields; mode fallbacks | legacy fixtures test; `eval diff` on a committed run in `.kairon/evals/results` |

## 8. Risks and Notes

- **E7 absent.** Described in §1. If E7 merges first, `agentScoreTotals` is the integration point and the whole-`Summary` comparison picks up its fields.
- **`selftest-fail` under `--sandbox`.** Its only case (`stub-timeout`, `timeout: 1s`, stub `sleep 3`) must time out and score identically in both paths. If the in-container timeout produces a different `ErrorContext`, that is allowed to differ; only scores, denominators and the pass/fail outcome must match. If the container outcome genuinely differs, that is a real parity bug to report, not to mask in the test.
- **Mode vocabulary.** `RunMode` values are `native` / `container` exactly as the issue suggests. Do not add `sandbox: true` to `summary.json`.
- **Daemon-gated tests** skip with a message when no Podman/Docker daemon is reachable; the validator must report them as "not run" in that case, never as passed.

## 9. Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/eval/... -count=1
task test
task lint
task fmt:check
task sync:check
task eval:selftest
# AC4/AC5 (legacy dirs are read-only inputs)
go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2
# AC1-AC4 end to end (needs Podman or Docker; skips otherwise)
KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestProvenanceParitySandbox' -count=1 -v
```

## 10. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "shared-score-totals"
    agent: "builder"
    description: "In internal/eval/scoring.go add caseTotals(CaseResult) (score, max int) and agentScoreTotals(AgentResult) (score, max float64), both excluding skipped criteria exactly as today, and make printCaseResult, updateIncrementalSummary and buildSummary in runner.go use them so the score/denominator arithmetic exists once. Pure refactor: no change to any emitted JSON or printed output. Add table tests for skipped criteria, a case with no scores and a zero denominator."
    dependencies: []
    acceptance_criteria:
      - "caseTotals and agentScoreTotals exist in scoring.go and no other function in internal/eval sums CriterionScore.Score/MaxScore inline (grep for 'totalMax +=' finds none outside the helpers)"
      - "Skipped criteria contribute to neither numerator nor denominator, identical to the previous behaviour"
      - "printCaseResult output and summary.json agent_scores for the selftest agent are unchanged"
      - "All existing internal/eval tests still pass"
    validation_commands:
      - "go build ./internal/eval/..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/ -run 'Score|Summary|Threshold|SelfTest' -count=1"

  - id: "record-run-mode"
    agent: "builder"
    description: "Record execution mode and containment for every run. In types.go add type RunMode string (native, container), type Containment {ToolTrust []string tool_trust (not omitempty), FakeGH bool fake_gh, ReadOnlyFS bool read_only_fs, Network string network}, Summary.Sandbox string `json:\"sandbox,omitempty\"`, Summary.Containment map[string]Containment `json:\"containment,omitempty\"` and AgentResult.Containment *Containment `json:\"containment,omitempty\"` (keep AgentResult.Sandbox *bool unchanged). Export const ReadonlyRootfs in internal/eval/sandbox/mounts.go and use it in NewHostConfigWithMounts. Add internal/eval/containment.go with containmentFor(agent) (ToolTrust from resolveTrustSet(agent).Names(); ReadOnlyFS from sandbox.ReadonlyRootfs; FakeGH from a const honoured by buildContainerMounts; Network const \"unrestricted\") and applyRunContext(*AgentResult, *ContainerConfig) which calls cfg.pins.applyTo, sets Sandbox, and sets or clears Containment. Replace the three cfg.pins.applyTo(&result) call sites (evaluate, evaluateProgressive x2) with applyRunContext. Make updateIncrementalSummary write Summary.Sandbox and Summary.Containment[agent] (native: 'native', no containment, stale entry removed; refuse to overwrite a different recorded mode) and make checkResumeIntegrity refuse when summary.json's recorded sandbox differs from the current mode. If resolveTrustSet fails, record tool_trust [] and print a warning without failing the run."
    dependencies: ["shared-score-totals"]
    acceptance_criteria:
      - "A container-mode AgentResult produced via applyRunContext has Sandbox=true and Containment{ToolTrust equal to resolveTrustSet names, FakeGH true, ReadOnlyFS true, Network 'unrestricted'}; a native one has Sandbox=false and Containment nil"
      - "applyRunContext leaves agent_model, judge_model, prompt_sha256 and resources_present identical between native and container for the same pins"
      - "summary.json written for a container result contains sandbox 'container' and a containment entry for the agent; for a native result it contains sandbox 'native' and no containment key"
      - "evaluate() (single-case path) now records Sandbox in the result file and summary.json"
      - "tool_trust serialises as [] (not absent) for a trust-nothing agent"
      - "Resume with a different mode than summary.json recorded is refused with a message naming both modes; updateIncrementalSummary returns an error instead of overwriting a different recorded mode"
      - "Legacy summary.json and agent files (no new fields) still unmarshal and resume behaviour for legacy agent files is unchanged"
      - "NetworkMode in mounts.go is unchanged"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "go test ./internal/eval/... -count=1"

  - id: "diff-provenance"
    agent: "builder"
    description: "Make eval diff surface provenance and execution mode. Refactor Diff into diffTo(w io.Writer, runA, runB string) keeping Diff and DiffWithOptions signatures. Load per-run provenance (mode, judge_model, per-agent agent_model/prompt_sha256, per-agent containment) from summary.json plus agent files, deriving mode from summary.sandbox, else the agent files' sandbox bool (all must agree), else 'unknown (predates sandbox-mode tracking)'. Print a 'Run Provenance' block before the existing deltas: each run's mode, then every difference in sandbox, containment, judge_model, agent_model and prompt_sha256, with a warning line when the execution modes differ. The diff never fails because of missing or old fields; all existing output after the block is unchanged."
    dependencies: ["record-run-mode"]
    acceptance_criteria:
      - "Diff of a native run against a container run succeeds and its output states the mode difference (native vs container) and the warning that scores are not directly comparable"
      - "Diff of two runs with the same mode prints no mode-difference warning"
      - "A containment difference (e.g. different tool_trust for an agent) is listed"
      - "judge_model, agent_model and prompt_sha256 differences are listed per agent"
      - "A run directory written before this change (no sandbox, no provenance fields) loads without error and shows mode unknown"
      - "diffTo writes only to the supplied writer; Diff and DiffWithOptions still work and TestDiffResolvesUnderEvalsDir passes"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/eval/ -run 'Diff' -count=1"
      - "go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2"

  - id: "parity-tests"
    agent: "builder"
    description: "Add fixtures internal/eval/testdata/evals/fixtures/runs/legacy-native (pre-#294 summary and agent file shapes) and native-pinned (post-#294, no sandbox fields). Add hermetic tests (internal/eval/containment_test.go, diff_provenance_test.go): applyRunContext native vs container provenance equality and containment correctness (ToolTrust equals trusted_tools source, ReadOnlyFS equals NewHostConfigWithMounts().ReadonlyRootfs, FakeGH equals presence of the containerBinDir mount from buildContainerMounts), summary mode/containment emission and conflicting-mode refusal, resume refusal on summary mode mismatch, shared-totals score equality between native-built and container-built results, and diffTo outputs for native vs container, legacy vs native, legacy vs container and same-mode runs. Add the daemon-gated TestProvenanceParitySandbox in parity_sandbox_test.go (gated like TestSelftestSandbox) running selftest and selftest-fail natively and with --sandbox on the stub backend and asserting identical provenance in <agent>.json, summary.json and every calls[] record, sandbox native vs container with the containment block, whole-Summary equality after zeroing Sandbox and Containment, equal per-case caseTotals and threshold outcome, and a successful diffTo that reports the mode difference. Add TestProvenanceParitySandbox to the eval:selftest:sandbox regex in Taskfile.yml."
    dependencies: ["record-run-mode", "diff-provenance"]
    acceptance_criteria:
      - "Fixtures live under testdata/evals/fixtures/runs/ (not the git-ignored testdata/evals/results/) and the tests copy them into a temp evals dir"
      - "Hermetic tests need no container daemon and pass with go test ./internal/eval/ -count=1"
      - "TestProvenanceParitySandbox skips with a message unless KAIRON_EVAL_SANDBOX_SELFTEST=1 and a daemon is reachable, and never reports a skip as a pass"
      - "The gated test compares the whole Summary (not hand-picked fields) so E7 fields are covered if present, and compares selftest-fail as well as selftest"
      - "No test or fixture edits .kairon/evals/results/, CHANGELOG.md or another .kairon/specs file"
      - "Taskfile eval:selftest:sandbox includes TestProvenanceParitySandbox"
    validation_commands:
      - "go test ./internal/eval/... -count=1"
      - "go vet ./..."
      - "KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestProvenanceParitySandbox' -count=1 -v"
      - "grep -n 'TestProvenanceParitySandbox' Taskfile.yml"

  - id: "docs"
    agent: "documenter"
    description: "Update docs/evaluation.md: add sandbox and containment to the Run summary example and field table (noting <agent>.json keeps sandbox as a boolean while summary.json uses native/container); add an 'Execution mode and containment' subsection (containment fields, native records no containment, network is always 'unrestricted' meaning no network guarantee and not the container NetworkMode, referencing the existing Limits text and the planned mock-guidance follow-up without inventing an issue number); document the parity guarantee and update the sentence saying sandboxed and native records match except trusted_tools; document the Run Provenance block, the mode-difference warning and 'unknown' for old runs under Comparing Runs and that old result directories still load; mention the summary-level sandbox check under Resume refusal; state that the per-agent threshold verdict (E7) is separate work and only parity of its inputs is guaranteed here. Do not edit CHANGELOG.md or any other .kairon/specs file."
    dependencies: ["parity-tests"]
    acceptance_criteria:
      - "docs/evaluation.md documents summary.json sandbox and containment with an example for a container run"
      - "docs/evaluation.md explains that network 'unrestricted' records the absence of a network guarantee and is not the container NetworkMode, with no invented issue number"
      - "The Comparing Runs section describes the Run Provenance block and the mode-difference warning"
      - "No statement in the doc contradicts the new fields (the 'match, except trusted_tools' sentence is updated)"
      - "Only docs/evaluation.md is changed"
    validation_commands:
      - "grep -n 'containment' docs/evaluation.md"
      - "grep -n 'Run Provenance' docs/evaluation.md"
      - "git diff --name-only -- docs CHANGELOG.md .kairon/specs"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #300 against the implementation and run the full build, test, lint, format and sync checks. Run the daemon-gated parity test if a container daemon is reachable and report it as not run (with the reason) otherwise. Confirm E7 is not implemented in this tree and that parity is covered structurally (shared totals, whole-Summary comparison). Confirm no historical artifact was edited."
    dependencies: ["docs"]
    acceptance_criteria:
      - "AC1: native and container selftest runs have identical agent_model, judge_model, prompt_sha256 and resources_present in <agent>.json and summary.json and on call records (or the gated test is reported as not run with the reason, with the hermetic applyRunContext test passing)"
      - "AC2: container summary.json has sandbox 'container' and the containment block (tool_trust, fake_gh, read_only_fs, network 'unrestricted'); native summary.json has sandbox 'native' and no containment"
      - "AC3: selftest-fail native and --sandbox produce the same agent score, denominators and per-case threshold outcome, computed by the shared helpers (or reported as not run)"
      - "AC4: go run ./cmd/kairon eval diff on a native run and a container run succeeds and reports the sandbox-mode difference (hermetic diffTo test, plus the gated run if available)"
      - "AC5: go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2 succeeds and legacy fixtures load"
      - "go build, go vet, task test, task lint, task fmt:check, task sync:check and task eval:selftest all pass"
      - "git diff shows no change to CHANGELOG.md, .kairon/evals/results/, validation reports, or any .kairon/specs file other than issue-300-eval-provenance-scoring-parity-native-container.md; NetworkMode in mounts.go is unchanged"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2"
      - "KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestProvenanceParitySandbox' -count=1 -v"
```
