# Design Spec: Evals — supersede E4 with the container sandbox; document containment and extension seams

Closes #301

## 1. Problem and Scope

The Stage 3 gap analysis (`.kairon/specs/maturity-model/gap-analysis.md`) still describes **E4** ("Isolated per-case
workspaces and a fake gh") as the Stage 3 isolation design: a temp git workspace, a `gh` shim on `PATH`, an isolated
`GH_CONFIG_DIR`, "Docker isn't required". That is convention-based isolation. It cannot enforce a boundary against an
arbitrary, extensible third-party agent (MCP servers, network access, absolute-path writes/`/usr/local/bin/gh`, tools
that ignore `PATH`). The container sandbox series (#296 → #300, merged as `#303`, `#305`, `#306`, `#309`, `#310`)
replaced it with a real filesystem boundary and layered containment. The gap analysis was never updated and still
contradicts the shipped design, and `docs/evaluation.md` documents the layers in several places but nowhere as one
model, and has no "extension seams" section.

**DOCS-ONLY. No code, test, config or template behaviour changes.**

In scope (both are LIVE files; editing them is allowed by `AGENTS.md`):
- `.kairon/specs/maturity-model/gap-analysis.md` — E4 section, dependency map, §S3.4 PR #194/#192 row, stale
  isolation claims.
- `docs/evaluation.md` — consolidated containment model + extension seams.

Out of scope: any Go/script change; reopening, relabelling or otherwise touching issue #192 / PR #194; editing any other
`.kairon/specs/**` file (`issue-*.md` records, `stage-3.md`, `stage-4.md`, `stage-5.md` are frozen — the only exception
is this spec); new check types (E5–E7); changing `NetworkMode`; adding a network gateway.

## 2. Findings From the Codebase (verified; the builder must not contradict these)

Verified against `main` @ `3d518f8`:

| Claim to document | Source of truth |
|-------------------|-----------------|
| Root FS is read-only; always on, no opt-out. Writable: workspace, `<ws>/.eval`, tmpfs `/tmp`, `/var/tmp`, `/home/sandbox`. | `internal/eval/sandbox/mounts.go` (`ReadonlyRootfs = true`); `docs/evaluation.md` "Read-only root filesystem" |
| Fake `gh` is a POSIX `sh` script written to `<ws-parent>/bin/gh`, bind-mounted **read-only** at `/opt/kairon/bin`, first on `PATH`. Real `gh` stays at `/usr/local/bin/gh`, unauthenticated (no token env, fresh `$HOME` tmpfs). | `docs/evaluation.md` "The fake `gh`"; spec #298 |
| Tool trust is **whole-tool**: native = `--trust-all-tools`; `--sandbox` = `--trust-tools=<csv>`. Resolution order: `evals.trust_tools[agent]` → agent config `allowedTools` → empty set (fail closed). `*`/empty entries rejected. Names normalised (`read→fs_read`, `write→fs_write`, `shell→execute_bash`, `aws→use_aws`). | `internal/eval/trust.go` `resolveTrustSet`; `internal/inference/trust.go` |
| Network: containers are created with `NetworkMode: "none"`, but this is **explicitly not a network policy** and not part of the guarantee; the recorded `containment.network` is the constant `"unrestricted"` (meaning "no guarantee"). Base-image *build* needs network. | `internal/eval/sandbox/mounts.go:106`; `docs/evaluation.md` "Execution mode and containment", "Limits" |
| Network side effects are the **eval author's responsibility via mocks**. Mock = script staged to `<ws-parent>/bin/<command>` (same RO mount), declared with case field `mocks: [{command, script}]`, requires `requires_sandbox: true`, `command` bare/unique/not `gh`, `script` an existing regular file inside the evals dir. Shim: `fixtures/mock-cli.sh`; canned replies in `workspaces/<name>/.mocks/<command>/`; log `.eval/mock-<command>.log`. | `internal/eval/types.go` (`Mocks`), `internal/eval/workspace.go` (`validateMocks`); `docs/evaluation.md` "The mock pattern" |
| A `PATH` shim does **not** intercept built-in/MCP tools (`use_aws`, `web_fetch`, `web_search`, `@server/tool`), absolute paths, or in-process SDKs. | `docs/evaluation.md` "Mocks and tool trust", "Limits of the mock approach" |
| No per-argument trust: `execute_bash` trusted = every shell command. Kairon neither generates nor verifies `toolsSettings`. `kiro-cli` refusals are not detectable (`tool_denials` is stub-only). How `kiro-cli` combines `--trust-tools` with `allowedTools` is undocumented. | `docs/evaluation.md` "Limits" |
| Native runs (no `--sandbox`) are **not contained** (`--trust-all-tools`, real `gh`). `requires_sandbox: true` makes a case refuse to run natively. | `docs/evaluation.md` "Sandbox Containment" |
| A `--sandbox` run records `sandbox: "container"` and a per-agent `containment` record `{tool_trust, fake_gh, read_only_fs, network}`; native records none. | `docs/evaluation.md` "Execution mode and containment" |
| **Retained from E4** and shared by native and sandbox runs: per-case git workspace built on the host (`workspace:` fixture, hermetic single commit), `.kiro/` staging, `.eval/`, `timeout`, `--keep-workspaces` / `workspace_dir`, stub `commands`, fixtures under `fixtures/workspaces/` and `fixtures/hidden/` excluded from `task sync:check`. | `docs/evaluation.md` "Case Workspaces" |
| Dropped from E4's design: the `GH_CONFIG_DIR` + `PATH`-shim approach as the *containment* mechanism. The fake `gh` now lives in the container mount; the real `gh` is unauthenticated by construction. | `docs/evaluation.md` "The fake `gh`" |
| "No per-tool-call hook" has **no code seam today**: nothing in `internal/` references a hook. The honest statement is that this kiro-cli build exposes only whole-tool `--trust-tools`; a per-call hook is future/hypothetical and its API is undefined. | grep of `internal/`, `docs/` |
| Issue/PR state (checked 2026-10-08 via `gh`): **PR #194 is CLOSED; issue #192 is still OPEN** (labels `kiro-krew`, `kiro-krew-done`). The issue text says "they stay closed", which is not literally true of #192. The doc must state facts, and this PR must not change either. | `gh issue view 192/194` |

`docs/evaluation.md` is **not** template-synced (only `.kairon/evals/**` is), so no `cmd/kairon/templates/` change and
`task sync:check` is unaffected. `gap-analysis.md` is likewise not synced.

## 3. Solution Approach

Two independent doc edits plus a read-only verification pass. Nothing is duplicated: the new `docs/evaluation.md`
section is a **map** (one place for layers, owners, limits, seams) that links into the existing detailed sections
rather than restating them, so it cannot drift from them. Every claim in it must trace to a row in §2.

### 3.1 `gap-analysis.md`

1. **E4 section (`##### E4 …`, ~L282–323).** Retitle to `E4 — ~~Isolated per-case workspaces and a fake gh~~ SUPERSEDED by the container sandbox`
   (plain "SUPERSEDED" text; no strikethrough needed). Replace the paste-able "Create a GitHub issue" prompt block with a
   short decision record (keep the heading anchor-able; keep it short):
   - **Status / decision (2026-10-08):** E4 is not to be filed. Superseded by the container sandbox series #296–#300.
   - **Why E4 was rejected:** convention-based isolation (CWD redirect, `PATH` `gh` shim, fresh `.kiro/`) is unsound
     for arbitrary, extensible third-party agents. It cannot enforce a boundary against (a) MCP servers and built-in
     tools that do their own I/O, (b) network access, (c) absolute-path writes and absolute-path binaries
     (`/usr/local/bin/gh`, `/etc`, `$HOME`, the live repo). It protects only agents that cooperate.
   - **What replaced it** — a table `Concern | E4 (convention) | Now (container series)` covering: filesystem
     (CWD only → read-only root FS, mounts), `gh` (PATH shim + `GH_CONFIG_DIR` → fake `gh` in a read-only mount, real
     `gh` unauthenticated), tools (`--trust-all-tools` → per-agent whole-tool `--trust-tools`), network/other CLIs
     (nothing → author-supplied mocks via `mocks:`; no gateway), prompt delivery/backends (#296). Name the issues:
     #296 prompt + backend routing, #297 tools-only base image + mounts, #298 fake `gh` / read-only FS / tool trust,
     #299 mock guidance, #300 provenance and scoring parity.
   - **What is retained from E4** (still real, shared by native and sandbox runs): workspace fixtures, staged `.kiro/`,
     `.eval/`, `timeout`, `--keep-workspaces`/`workspace_dir`, stub `commands`, and the rule that
     `fixtures/workspaces/` and `fixtures/hidden/` are not copied into `cmd/kairon/templates/` (other sections link to
     "see E4" for this, so state it here).
   - **Honest limits** in two sentences, then a pointer to `docs/evaluation.md#containment-model` (use the final anchor
     of the new section). Native runs remain uncontained; `requires_sandbox: true` is the guard.
   - Downstream issues that said "depends on E4" are satisfied by the series.
2. **§S3.4 row "PR #194 / issue #192"** → rewrite to: #194/#192 were an *earlier attempt* at a container sandbox
   ("move file setup to build time"); PR closed 2026-10-05 (pre-rename, did not fix H2 — prompt never sent); **superseded by the container
   sandbox series #296–#300** (tools-only base image, mounted `.kiro`/workspace/outputs, fake `gh`, read-only FS,
   tool trust, mocks); the branch is kept; **neither is reopened or relabelled by this work**. State #192's actual
   current state as of the edit (OPEN today — re-check with `gh issue view 192 --json state`; do not claim it is closed).
   Remove "Stage 3 uses native isolated workspaces (E4)".
3. **Dependency map (`#### Dependency order`, L144–152)** → replace `E3 → E4 → E5` with
   `E3 → [container sandbox series #296–#300, supersedes E4] → E5` and update the serial-order line
   (`E1, E2, E3, <sandbox series>, E5, …`). Add one line saying the series is merged, so E5 is unblocked.
4. **Stale E4 / isolation claims** (each is a consistency fix, minimal wording changes):
   - §S3.5 change 7 "Isolation" (L120): replace the "temp copy + `gh` shim + isolated `GH_CONFIG_DIR`… Docker isn't
     required" text with the container-sandbox statement (cases that can cause side effects run under `--sandbox`,
     read-only FS, fake `gh`, whole-tool trust, author-supplied mocks; native runs are not contained).
   - §S3.5 L131 "gh-shim log" → "fake-`gh` log".
   - Dependency bullets `- E4` / `- E3, E4` / `- E3, E4, E5, …` (~L368, 557, 862, 1074): change `E4` to
     `the container sandbox series (#296–#300; supersedes E4)` (keep the rest of each list intact).
   - L859/L1071 "(see E4)" workspace-fixture sync rule: keep valid by retaining the sentence in the E4 record above
     (item 1); no edit needed beyond confirming the anchor still reads sensibly.
   - L1087 "an extension of E4" (workspace that is a git worktree of Kairon at a pinned commit): reword to "an extension of
     the workspace fixtures".
   - L1102 checklist item "Cases run in isolated workspaces; no case touches the repo root or the real GitHub API" →
     "Cases with side-effect risk run under `--sandbox` (container; `requires_sandbox: true`); no case touches the repo
     root or the real GitHub API".
   - L1582 `- [ ] E4 Isolated per-case workspaces + gh shim` → `- [x] E4 superseded by the container sandbox series (#296–#300)`.
   - L1135 "Isolated workspaces and the fake gh: …" — leave unless it contradicts; do not churn.
   - H1/H2 findings (L65–66) are point-in-time *findings*; leave as written (they describe the problem, not the plan).
   - The E1–E4 "Out of Scope: the Docker sandbox" lines are historical scoping of those prompts; leave.
5. Do **not** touch `stage-3.md`, `stage-4.md`, `stage-5.md`, or any `issue-*.md` (frozen). `grep` showed no E4 references there.

### 3.2 `docs/evaluation.md`

Add one new `###` section **"Containment Model and Extension Seams"**, placed immediately after
"Preventing Production Side Effects (Containment and Mocking)" (i.e. before `### Container Lifecycle`), with these `####` subsections and a stable anchor `#containment-model`:

1. **Why a container, not a convention** (3–5 sentences): records the decision to supersede E4 (link nothing outside
   the repo; mention the gap analysis by path). Convention-based isolation (CWD, `PATH` shim) cannot enforce a boundary
   against arbitrary extensible agents (MCP, network, absolute paths); the sandbox gives a real mount-level boundary
   and puts everything else in named layers with named owners.
2. **Containment model** — ONE table, columns: *Layer · Owner · Mechanism · Enforced? · Honest limit*. Rows exactly:
   - Filesystem — Kairon — read-only root FS + bind mounts (`.kiro` ro, workspace rw, `.eval` rw, `/opt/kairon/bin` ro), live repo never mounted — Enforced (container runtime) — Writable holes: workspace, `.eval/`, tmpfs `/tmp` `/var/tmp` `/home/sandbox`; workspace world-writable on host during a run; capabilities not dropped, no pids limit, no `no-new-privileges`.
   - `gh` — Kairon — fake `gh` first on `PATH` in a ro mount; real `gh` unauthenticated — Enforced (mount + env) — fixed command set, no `--jq`/`--template`; an agent can still run `/usr/local/bin/gh` (fails: not logged in).
   - Tool trust — Kairon sets the set, `kiro-cli` enforces — `--trust-tools=<per-agent set>` — Enforced **whole-tool only** — no per-argument/per-call control in this `kiro-cli` build (trusting `execute_bash` trusts every command; `fs_write` every writable path); denials not detectable for real `kiro-cli` (`tool_denials` is stub-only); how `--trust-tools` combines with `allowedTools` is undocumented; tool-name mapping unverified end to end.
   - Network side effects (AWS, `npm publish`, HTTP, any other CLI) — **Eval author** — case-level mocks (`mocks:`, endpoint config in the workspace fixture) — **Not enforced** (convention) — no network gateway: `NetworkMode: none` is a setting, not a policy; the agent is its own model client so an allowlist could not tell model traffic from exfiltration; `PATH` mocks miss built-in/MCP tools, absolute paths and in-process SDKs.
   - Native (non-sandbox) runs — Eval author / operator — none — **Not contained** — `--trust-all-tools`, real `gh`, writes anywhere; `requires_sandbox: true` is the guard.
   - Recording — Kairon — `sandbox` + per-agent `containment` in results; `eval diff` warns on mode mismatch — Enforced by code — records what was applied; `network: "unrestricted"` records absence of a guarantee, not the container's network mode.
   Follow with a 3–4 line "reading the table" note: enforced = a mechanism stops it regardless of what the agent does; convention = works only if the author wrote the mock; and a sentence that the model is deliberately not airtight.
3. **What the model does not defend against** — short bullet list (in-process SDK calls; MCP/built-in tools when trusted;
   credentials passed into the container environment; a trusted `execute_bash` doing anything the container can do
   offline in the writable mounts; model-endpoint access needed by the agent itself).
4. **Extension seams** — each seam: *what you edit · what it does · where it stops*:
   - **Add a mock for a CLI** (`PATH` shim): `mocks:` in the case YAML + `fixtures/mock-cli.sh` + canned replies under
     `fixtures/workspaces/<name>/.mocks/<command>/`; for logic beyond the shim, copy it under a new name in `fixtures/`.
     Needs `requires_sandbox: true`; mock lands in `/opt/kairon/bin` (ro). Link "The mock pattern".
   - **Point a tool at a stand-in endpoint** (endpoint config): project config in the workspace fixture (`.npmrc`
     `registry=…`, tool endpoint override files). Works because the agent's CWD is the workspace; does not cover
     in-process SDKs that ignore config. Link "HTTP endpoints and in-process SDKs".
   - **Set per-agent tool trust**: `evals.trust_tools.<agent>` in `.kairon/config.yaml` (override) or the agent's
     `allowedTools` (default); `[]` trusts nothing; `*`/empty rejected; recorded as `trusted_tools` / `containment.tool_trust`.
     Guidance: leave `use_aws`, `web_fetch`, `web_search`, `@server/tool` out of a case that relies on a `PATH` mock.
     Include the yaml example already used in the doc (consistent, not new behaviour).
   - **Extend the fake `gh`**: the script is embedded in the `kairon` binary; unsupported commands log and exit 1,
     which is the signal to extend it (code change — out of scope here; state it as a seam, not a how-to).
   - **Future: per-tool-call hook (preferred fine-grained layer).** State plainly that **this `kiro-cli` build has no
     per-tool-call/per-argument hook and Kairon has no code for one**; this is a statement of where it would go, not a
     feature. If `kiro-cli` gains one, it would plug in next to whole-tool trust: the trust resolution
     (`resolveTrustSet` in `internal/eval/trust.go`, `inference.ToolTrust`) and the argv built for the container
     `kiro-cli` call are the single place trust is applied, and the `containment` record (a new field alongside
     `tool_trust`) is where it would be reported. It would be the **preferred** way to constrain
     `execute_bash`/`fs_write`/`use_aws` by argument, and would reduce (not remove) reliance on mocks; it would not
     replace the filesystem layer. No schema, config key or timeline is promised.
   - **Network gateway**: also not built; same honesty. One sentence on why it is not preferred (see "Why Kairon does
     not network-gateway these") and that it would be the second seam if a deployment needs it.
5. **Where each concern is documented in detail** — a short link list into the existing sections (Sandbox Containment,
   The fake `gh`, Read-only root filesystem, Tool trust, Limits, Preventing Production Side Effects, Execution mode and
   containment). No duplicated prose.

Small consistency edits elsewhere in `docs/evaluation.md` (one line each, no restatement):
- In `### Sandbox Containment` (intro paragraph) and in `### Security Considerations`, add a pointer to the new section.
- Do not alter the existing "Limits" bullets; the new table must agree with them (re-read after writing).

## 4. Relevant Files

| File | Action |
|------|--------|
| `.kairon/specs/maturity-model/gap-analysis.md` | Modify (E4 section, dependency map, §S3.4 row, stale claims) |
| `docs/evaluation.md` | Modify (new section + two pointer lines) |
| `.kairon/specs/issue-301-supersede-e4-container-sandbox-containment-docs.md` | This spec (created) |
| `.kairon/specs/issue-29{6,7,8,9}-*.md`, `issue-300-*.md` | Read-only references; **do not edit** |
| `internal/eval/trust.go`, `internal/inference/trust.go`, `internal/eval/sandbox/mounts.go`, `internal/eval/workspace.go`, `internal/eval/types.go`, `.kairon/evals/fixtures/mock-cli.sh` | Read-only references to verify claims |
| `README.md`, `CONTRIBUTING.md`, other `docs/*.md`, templates | Checked: no E4 / "native isolated workspaces" references; **no change** |

## 5. Team Orchestration

- `edit-gap-analysis` and `edit-evaluation-doc` touch disjoint files and have no dependencies → run **in parallel**.
- `validate-docs` (validator, read-only) runs after both and checks each acceptance criterion and the "no code changed" constraint.
- Single PR; all four acceptance criteria are covered; nothing is deferred.

## 6. Step-by-Step Task Breakdown

### Task 1: edit-gap-analysis (builder)
Apply §3.1. **Acceptance criteria:** E4 section rewritten as superseded with the rationale (CWD + `PATH` shim unsound for
extensible third-party agents: MCP, network, absolute-path writes), the mapping table, retained-from-E4 list and
honest limits; §S3.4 row rewritten and no longer says "Stage 3 uses native isolated workspaces (E4)", names #194/#192
as earlier container attempts superseded by #296–#300, states neither is reopened/relabelled and reports #192's real
state; dependency map and serial order updated; remaining E4 dependency bullets, §S3.5 Isolation, checklist lines updated;
no strikethrough/markdown breakage; frozen files untouched. **Dependencies:** none.

### Task 2: edit-evaluation-doc (builder)
Apply §3.2. **Acceptance criteria:** new section with anchor `#containment-model`; one table listing every layer with
owner, mechanism, enforced/convention, and honest limit, including "no per-argument tool hook in this kiro-cli build"
and "no network gateway"; extension seams for mocks (PATH shim / endpoint config), per-agent trust, and the future
per-tool-call hook as the preferred fine-grained layer (explicitly marked as not existing today); pointer lines in
"Sandbox Containment" and "Security Considerations"; all intra-doc links resolve to existing headings; no claim
outside §2; existing sections unchanged otherwise. **Dependencies:** none.

### Task 3: validate-docs (validator)
Read-only verification of both files against §2 and the four acceptance criteria; confirm `git diff --stat` touches only
the two target files (plus this spec); confirm no `.go`, template, script or other `.kairon/specs/**` change; confirm
links/anchors; confirm the plan parses. **Dependencies:** tasks 1 and 2.

## 7. Acceptance Criteria Traceability

| Issue AC | Where satisfied | Check |
|----------|-----------------|-------|
| 1. E4 superseded with rationale | Task 1 / §3.1 item 1 | grep `SUPERSEDED` in E4 heading; rationale mentions MCP, network, absolute-path |
| 2. Dependency map + #194/#192 row; stale claim removed | Task 1 / §3.1 items 2–4 | `! grep "native isolated workspaces"`; row names #296–#300; map text updated |
| 3. Containment model as a whole | Task 2 / §3.2 items 1–3 | table has all layers/owners/limits; both honest limits present |
| 4. Extension seams | Task 2 / §3.2 item 4 | mocks, endpoint config, trust, future hook subsections |

## 8. Risks, Assumptions and Open Items

- **#192 state mismatch.** Issue text says #192/#194 "stay closed"; `gh` shows #192 OPEN. The docs state actual
  state and say nothing is changed. Builder re-checks with `gh issue view 192 --json state` at edit time and writes what it sees; it must not run any `gh issue edit/reopen/close`.
- **Series ↔ PR mapping** (#296→#303, #297→#305, #298→#306, #299→#309, #300→#310) is inferred from `git log` subjects.
  The builder verifies with `git log --oneline` and, if any pairing is uncertain, cites only issue numbers.
- **`AGENTS.md` freezes `.kairon/specs/` including `maturity-model/`**, but the issue is an explicit maintainer request
  for a `gap-analysis.md` edit and states it is a permitted live edit. Edit only `gap-analysis.md`; leave
  `stage-*.md` alone.
- **Future hook is speculative.** Phrase it conditionally everywhere; no invented flag, config key, field name committed as real.
- **The new table duplicates facts** that live in detailed sections. Mitigation: keep cells terse and link out; builder re-reads
  "Limits" and "Preventing Production Side Effects" after writing to ensure no contradiction.
- Docs-only, so per `AGENTS.md` no test-first requirement applies; verification is by grep/link checks and the repo's existing tests being untouched.

## 9. Validation Commands

```bash
# Spec itself
go run ./cmd/kairon plan parse .kairon/specs/issue-301-supersede-e4-container-sandbox-containment-docs.md

# AC 2: stale claim gone, new references present
! grep -n "native isolated workspaces" .kairon/specs/maturity-model/gap-analysis.md
grep -n "SUPERSEDED" .kairon/specs/maturity-model/gap-analysis.md
grep -n "#296" .kairon/specs/maturity-model/gap-analysis.md

# AC 3/4: doc sections present
grep -n "^### Containment Model and Extension Seams" docs/evaluation.md
grep -n -i "per-tool-call" docs/evaluation.md
grep -n -i "network gateway" docs/evaluation.md

# Docs-only: only the intended files changed
git diff --name-only origin/main...HEAD
git status --porcelain

# Repo health unchanged
task sync:check
task test
```

## 10. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "edit-gap-analysis"
    agent: "builder"
    description: "Edit .kairon/specs/maturity-model/gap-analysis.md only: rewrite the E4 section as superseded by the container sandbox series (#296-#300) with rationale, E4-vs-container mapping table, retained-from-E4 list and honest limits; rewrite the S3.4 PR #194 / issue #192 row (earlier container attempts, superseded, not reopened or relabelled, #192 real state reported, stale 'native isolated workspaces (E4)' claim removed); update the dependency map and serial order; fix remaining stale E4/isolation references (S3.5 Isolation, gh-shim log, E4 dependency bullets, checklist lines). Do not touch any other .kairon/specs file."
    dependencies: []
    acceptance_criteria:
      - "The E4 section heading says SUPERSEDED and the body states the rationale: convention-based isolation (CWD + PATH gh shim) is unsound for arbitrary extensible third-party agents because it cannot enforce a boundary against MCP, network or absolute-path writes"
      - "The E4 section maps E4's mechanisms to the container series and names issues #296, #297, #298, #299 and #300, lists what is retained from E4 (workspace fixtures, .eval, timeout, --keep-workspaces, sync exclusion of fixtures/workspaces and fixtures/hidden), and states the honest limits (native runs uncontained, no per-argument hook, no network gateway)"
      - "The S3.4 row for PR #194 / issue #192 describes them as earlier container attempts superseded by the #296-#300 series, says neither is reopened or relabelled, reports #192's actual current state, and no longer contains the text 'Stage 3 uses native isolated workspaces (E4)'"
      - "The dependency order block no longer routes E3 -> E4 -> E5 and instead references the container sandbox series; the serial-order line is updated"
      - "No remaining text in the file presents E4 as the live Stage 3 isolation plan; other '- E4' dependency bullets, the S3.5 Isolation item, the gh-shim log wording and the E4 checklist lines are updated"
      - "No file other than gap-analysis.md is modified by this task"
    validation_commands:
      - "! grep -n 'native isolated workspaces' .kairon/specs/maturity-model/gap-analysis.md"
      - "grep -n 'SUPERSEDED' .kairon/specs/maturity-model/gap-analysis.md"
      - "grep -n '#296' .kairon/specs/maturity-model/gap-analysis.md"
      - "! grep -n 'E3 → E4 → E5' .kairon/specs/maturity-model/gap-analysis.md"
      - "! grep -n 'gh shim on PATH and an isolated' .kairon/specs/maturity-model/gap-analysis.md"
      - "git diff --name-only | grep -v -E '^(.kairon/specs/maturity-model/gap-analysis.md|docs/evaluation.md|.kairon/specs/issue-301-.*\\.md)$' | wc -l | grep -q '^ *0$'"

  - id: "edit-evaluation-doc"
    agent: "builder"
    description: "Edit docs/evaluation.md only: add a new '### Containment Model and Extension Seams' section (anchor #containment-model) after 'Preventing Production Side Effects (Containment and Mocking)' and before 'Container Lifecycle'. It must contain: why a container rather than convention-based isolation (E4 superseded); one table of every containment layer with owner, mechanism, enforced-vs-convention and honest limit (filesystem, gh, tool trust, network side effects via mocks, native runs, recording); what the model does not defend against; extension seams (add a mock via PATH shim, point a tool at a stand-in endpoint via workspace config, set per-agent tool trust via evals.trust_tools / allowedTools, extend the fake gh, and the future kiro-cli per-tool-call hook as the preferred fine-grained layer, clearly marked as not existing in this build); and a link list to the detailed sections. Add one-line pointers to it from 'Sandbox Containment' and 'Security Considerations'. Every claim must match the existing detailed sections and the code; do not invent flags, config keys or behaviour."
    dependencies: []
    acceptance_criteria:
      - "docs/evaluation.md has a '### Containment Model and Extension Seams' section located between 'Preventing Production Side Effects (Containment and Mocking)' and 'Container Lifecycle'"
      - "A single table lists every layer (filesystem, gh, tool trust, network side effects, native runs, recording) with owner, mechanism, whether it is enforced or a convention, and its honest limit"
      - "The honest limits explicitly include: no per-argument/per-call tool hook in this kiro-cli build (trust is whole-tool), and no network gateway (NetworkMode none is not a policy; network side effects are the eval author's responsibility via mocks)"
      - "Extension seams document how a consuming project adds mocks (PATH shim via mocks: / mock-cli.sh and endpoint config in the workspace fixture), how it sets per-agent tool trust (evals.trust_tools and allowedTools, fail-closed, recorded in trusted_tools / containment.tool_trust), and where a future kiro-cli per-tool-call hook would plug in (trust resolution and the container kiro-cli argv, reported in the containment record) as the preferred fine-grained layer, explicitly stated as not existing today with no committed schema"
      - "The section states the decision that the container sandbox supersedes convention-based isolation (E4) and why"
      - "Pointer lines to the new section exist in 'Sandbox Containment' and 'Security Considerations'; all internal links point at existing headings; existing Limits bullets are unchanged and not contradicted"
      - "No file other than docs/evaluation.md is modified by this task"
    validation_commands:
      - "grep -n '^### Containment Model and Extension Seams' docs/evaluation.md"
      - "grep -n -i 'per-tool-call' docs/evaluation.md"
      - "grep -n -i 'network gateway' docs/evaluation.md"
      - "grep -n -i 'E4' docs/evaluation.md"
      - "grep -c '#containment-model' docs/evaluation.md | grep -v '^0$'"
      - "git diff --name-only | grep -v -E '^(.kairon/specs/maturity-model/gap-analysis.md|docs/evaluation.md|.kairon/specs/issue-301-.*\\.md)$' | wc -l | grep -q '^ *0$'"

  - id: "validate-docs"
    agent: "validator"
    description: "Read-only verification that both documents meet every acceptance criterion of issue #301, that claims match the code and existing docs, that links resolve, that only the intended files changed, and that no code behaviour changed."
    dependencies: ["edit-gap-analysis", "edit-evaluation-doc"]
    acceptance_criteria:
      - "gap-analysis.md E4 section is superseded with the required rationale; the S3.4 row and dependency map reflect the container sandbox series; the stale 'native isolated workspaces (E4)' claim is gone; #192/#194 are not described as reopened or relabelled and #192's state is reported accurately"
      - "docs/evaluation.md documents the whole containment model (enforced layers, author's-responsibility layer, honest limit of each) in one place with owners, and the extension seams including the future per-tool-call hook"
      - "Every factual claim in the new sections is consistent with internal/eval/sandbox/mounts.go, internal/eval/trust.go, internal/inference/trust.go, internal/eval/workspace.go and the existing docs/evaluation.md sections (no invented behaviour; the future hook is described as not existing)"
      - "git diff touches only gap-analysis.md, docs/evaluation.md and this spec; no .go, template, script, config, other .kairon/specs file, README.md or CONTRIBUTING.md changes"
      - "task sync:check and task test still pass"
    validation_commands:
      - "go run ./cmd/kairon plan parse .kairon/specs/issue-301-supersede-e4-container-sandbox-containment-docs.md"
      - "! grep -n 'native isolated workspaces' .kairon/specs/maturity-model/gap-analysis.md"
      - "grep -n '^### Containment Model and Extension Seams' docs/evaluation.md"
      - "git diff --name-only"
      - "task sync:check"
      - "task test"
```
