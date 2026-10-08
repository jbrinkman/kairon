# Design Spec: Evals — documented mock-based guidance for preventing production side effects

Closes #299

Depends on #298 (merged: fake `gh` on a read-only `/opt/kairon/bin`, read-only root FS, per-agent tool trust). This is the fourth issue of the container-sandbox series. It is code-light: documentation, one reusable fixture, one small documented hook that reuses the #298 PATH mount, and one self-test case.

## 1. Problem and Scope

Kairon enforces the layers it can enforce cheaply (filesystem, `gh`, whole-tool trust). It deliberately does **not** gateway network egress: an allowlist that classifies every destination is an app gateway, and it still cannot tell a legitimate model call from an exfiltration to the same host. The agent is the LLM client and keeps outbound network for model access. So every other production side effect (real AWS/cloud SDK calls, `npm publish`, arbitrary HTTP to real services) is the **eval author's** responsibility, met by writing cases against mocks. That is only safe if it is documented, demonstrated by a working pattern, and tied to the tool-trust layer.

In scope: `docs/evaluation.md`; a reusable mock fixture under `.kairon/evals/fixtures/`; a self-test case under `internal/eval/testdata/evals/`; and a small documented hook in `internal/eval` that places an author-supplied mock on the container `PATH`.

Out of scope (do not touch): any network allowlist/proxy/firewall/packet filter; changing `NetworkMode`; provenance/scoring parity; E4 supersession and extension-seam prose; new check types (E5-E7); other files under `.kairon/specs/` (AGENTS.md).

## 2. Findings From the Codebase

- **Fake `gh` precedent.** `sandbox.WriteFakeGH(w.BinDir)` (called from `newCaseWorkspace` in `internal/eval/workspace.go`) writes a POSIX-sh script to `<root>/bin/gh` (0755). `buildContainerMounts` (`runner.go`) bind-mounts `ws.BinDir` **read-only** at `containerBinDir = /opt/kairon/bin`; `containerPath` puts that directory first on `PATH`; `containerEnv` sets `KAIRON_EVAL_DIR=<workspace_dir>/.eval`. The script logs to `$KAIRON_EVAL_DIR/gh.log`. The bin dir sits beside the workspace (outside the agent's writable mounts), so the shim itself cannot be modified by the agent. `TestRunnerHasNoRuntimeInstalls` bans `.CopyTo(` in `runner.go`: mocks must arrive by mount, never by copy into a running container.
- **Consequence: the mount and PATH already exist.** The only missing piece for an author-supplied mock is *getting an author's script into `BinDir`* at workspace-build time. No mount, env or `PATH` change is needed. No change to `sandbox/` is needed either.
- **PATH reaches every process.** The container `PATH` is part of `container.Config.Env`, so it applies to `kiro-cli`'s `execute_bash` children as well as to the stub backend's `commands`. A shim therefore works for a real agent, not only for the stub.
- **Backing data precedent.** The host renders `gh-issue.{json,txt}` into the mounted `.eval/`. Workspace *fixtures* (`<evals-dir>/fixtures/workspaces/<name>/`) are copied into the case workspace and committed; dot-directories are copied (only `.git` is skipped) and `.eval/`/`.kiro/` are excluded from `git status`. So canned mock replies can ship as fixture files in the workspace.
- **Case schema & validation.** `TestCase` (`types.go`) already has `GHIssue` and `RequiresSandbox`. `validateCaseFields` → `validateSandboxFields` (`workspace.go`) runs from `loadCases` (`runner.go`) and rejects `gh_issue` without `requires_sandbox: true` (a native run would call the real `gh`). `executeCase` refuses `requires_sandbox` cases when `cConfig == nil` before any workspace or agent call. Mocks need exactly the same guard: a native run has no `/opt/kairon/bin` on `PATH`, so a mock would be silently skipped and the **real** tool called.
- **Check types do not exist yet.** `grep` finds no `changed_files`, `file_contains` or `gh_log` in any Go code; `docs/evaluation.md` line ~551 says `file_exists`/`changed_files` "are not part of this change". `scoreDeterministic` (`runner.go` ~1144) is a criterion-name heuristic switch. E5-E7 are explicitly out of scope, so this issue **cannot** add a rubric check for the mock log. Precedent from #298: the gated Go test (`containment_sandbox_test.go`) reads the kept host workspace (`.eval/gh.log`, `gh-body-1.md`) and asserts. The same approach is used here; the doc states that an E5-style `file_contains`/`gh_log` check reads the same host file (`<workspace_dir>/.eval/mock-<cmd>.log`). No schema for E5 is invented.
- **Self-test layout.** Containment cases live in `internal/eval/testdata/evals/cases/selftest-sandbox/*.yaml` (agent `selftest-sandbox`, `allowedTools: [read, write]`, every case `requires_sandbox: true`, stub backend). `containment_sandbox_test.go` has `containmentCases` (exact per-agent case set, checked by `TestContainmentFixtures`), a daemon-gated `TestContainmentSandbox` with subtests, and an always-on `TestContainmentNativeRefusal` that iterates `containmentCases`. `task eval:selftest:sandbox` already runs `TestContainmentSandbox`, so a new subtest needs no Taskfile change. `--evals-dir internal/eval/testdata/evals` is the self-test's evals dir, so a case's fixture files resolve **there**, not in `.kairon/evals/`.
- **Image has no `aws`, `npm` or `curl`-to-services.** `base.Dockerfile` is tools-only (alpine, git, bash, gh, kiro-cli). The self-test therefore demonstrates the *wiring* (which binary a bare `aws` resolves to, what is recorded, that an unsimulated destructive call is not executed). In a consuming project whose image does contain `aws`, the same wiring is what keeps the call off the real service. The doc must say this plainly.
- **Template sync.** `.kairon/evals/fixtures/*` regular files at the top level are synced to `cmd/kairon/templates/kairon/evals/fixtures/` by `find .kairon/evals/fixtures -maxdepth 1 -type f -exec cp ...`; `task sync:check` diffs the trees excluding `workspaces`, `hidden`, `results`, `tmp`. `cmd/kairon/main.go` embeds `templates` (`go:embed templates`). A **top-level file** `fixtures/mock-cli.sh` is covered by the existing sync commands and ships to consumers through `kairon init`; a new *subdirectory* would break `sync:check` and the documented sync commands. Hence the fixture is a single top-level file. `builder-conventions` needs no change. Embedded files lose their exec bit, which is fine because the hook always stages a mock as `0755`.
- **Stale statements to fix.** `docs/evaluation.md` (Limits bullet ~line 1044 and Security Considerations ~1138) and `docs/unified-container-flow.md` line 125 say mock guidance is "planned as a separate follow-up (the mock-guidance issue)". This issue is that follow-up.
- **Unverified:** `docs/evaluation.md` troubleshooting mentions `KAIRON_EVAL_NETWORK_MODE=bridge`; a grep of `internal/` finds no such variable in Go code. The documenter must verify before repeating it and must not rely on it in the new section.

## 3. Solution Approach

### 3.1 The pattern: PATH shim + backing data on the workspace

Three artifacts, all following the fake-`gh` precedent:

| Artifact | Where | Role |
|----------|-------|------|
| Generic shim `mock-cli.sh` | `<evals-dir>/fixtures/mock-cli.sh` (live: `.kairon/evals/fixtures/mock-cli.sh`, synced to templates) | One POSIX-sh script that can stand in for **any** CLI. It behaves according to the name it is invoked as (`basename "$0"`). |
| Canned replies | the case's workspace fixture: `<evals-dir>/fixtures/workspaces/<name>/.mocks/<cmd>/…` | Backing data on the mounted workspace (committed with the fixture, so `git status` stays clean). |
| Case wiring | case YAML `mocks:` | Declares which commands are mocked and which script implements each. |

#### Case field (the hook)

```yaml
requires_sandbox: true        # required whenever mocks is set
workspace: mock-aws           # fixtures/workspaces/mock-aws/ holds the canned replies
mocks:
  - command: aws                      # bare command name placed first on PATH
    script: fixtures/mock-cli.sh      # path relative to the evals dir
```

`TestCase.Mocks []CaseMock` with `CaseMock{Command, Script string}` (`yaml:"command"`, `yaml:"script"`; JSON tags likewise).

**Load-time validation** (`validateSandboxFields`, error names the case and file):
- `command` non-empty, matches `^[A-Za-z0-9][A-Za-z0-9._+-]*$`, is not `gh` (the fake `gh` is harness-owned), and is unique within the case.
- `script` non-empty, `filepath.IsLocal` (no absolute path, no `..`), and `evalsPath(script)` is an existing regular file.
- `mocks` set without `requires_sandbox: true` is an error (same rationale and message style as `gh_issue`).

**Staging** (`newCaseWorkspace`, right after `WriteFakeGH`): `stageMocks(tc.Mocks)` reads each script and writes it to `w.BinDir/<command>` with mode `0755` (explicit `Chmod`, like `WriteFakeGH`); refuse to overwrite an existing file. Nothing else changes: `BinDir` is already mounted read-only at `/opt/kairon/bin`, which is already first on `PATH`, with `KAIRON_EVAL_DIR` already set. Native runs never reach this code for a mock case (the `requires_sandbox` guard fires first). The cost is a field, a validator and a ~20-line stager; it adds no enforcement and no filter.

#### `mock-cli.sh` behaviour (the reusable fixture)

POSIX `sh` only (image uses busybox ash), no `jq`, `LC_ALL=C`, never executes another binary, never touches the network.

1. `cmd=$(basename "$0")`. If `KAIRON_EVAL_DIR` is unset or not a directory: stderr `kairon mock <cmd>: KAIRON_EVAL_DIR is not set`, exit 1.
2. Append **one line** `<cmd> <args>` to `$KAIRON_EVAL_DIR/mock-<cmd>.log`, before anything else (a failing call is still logged). Argument quoting is identical to the fake `gh` (`gh.log`): space-joined, no timestamp, an argument is single-quoted only if empty or containing a character outside `[A-Za-z0-9_./:=@%+,-]`, newlines folded to spaces.
3. Data directory: `$KAIRON_MOCK_DATA` if set, else `<workspace>/.mocks/<cmd>` where `<workspace>` is `dirname "$KAIRON_EVAL_DIR"`.
4. Reply lookup. Take the first two arguments that do not start with `-` (`a1`, `a2`), keep only `[A-Za-z0-9_]` in each (so `s3`, `cp`; `--profile`-style flags are ignored; flag *values* are not skipped, documented). Try in order `a1-a2.out`, `a1.out`, `default.out` in the data directory. The first that exists is printed to stdout verbatim. The exit status is the integer in the sibling `<same-name>.rc` if present, else 0 (lets an author simulate `AccessDenied`).
5. No match: stderr `kairon mock <cmd>: "<args>" is not simulated (call logged only)`, exit 1. Honest failure rather than silent success, as in the fake `gh`: a destructive call such as `aws iam delete-user …` is logged and never reaches anything.

Authors copy the script and edit the dispatch if they need richer behaviour; the doc shows how. The same file mocks `npm`, `curl`, `kubectl`, … by changing `command:`.

### 3.2 Mock HTTP endpoints without a new env hook

Do **not** add an env-injection field. The doc covers HTTP two ways, both working today:
- **Shim the client CLI** (`curl`, `wget`, `aws`, `npm`, `gcloud`) with `mock-cli.sh`; this is what `mocks:` is for.
- **Point the tool at a mock through project config that lives in the workspace fixture** (for example `.npmrc` with `registry=http://127.0.0.1:4873`, or a tool's endpoint override in its config file). The cwd is the workspace, so such files are honoured. The doc must say plainly that Kairon does not run the mock server (the container has no mock-server binary; a case that needs one ships it in its image or workspace) and that an SDK called in-process that ignores `PATH` is only covered by this config route or by trusting fewer tools.

### 3.3 Tying mocks to tool trust (#298)

The doc cross-references [Tool trust](evaluation.md#tool-trust) and the per-agent default trust table, and explains that the two controls are complementary:
- **Trust limits *which tools run*.** Each tool removed from `evals.trust_tools[<agent>]` (or the agent's `allowedTools`) is one less surface a case must mock.
- **Mocks make the tools that *do* run safe.** A trusted `execute_bash` can run any command; the shim decides what a bare `aws`/`npm`/`curl` does.
- **Caution (must be in the doc):** the PATH shim intercepts commands resolved through `PATH` by a trusted shell tool. Built-in or MCP tools that do their own I/O are **not** intercepted: `use_aws` (alias `aws`), `web_fetch`, `web_search`, `@server/tool`. A case that relies on a mock for AWS or HTTP should leave those out of the trust set (for example `evals.trust_tools: {builder: [read, write, shell]}`), because a mock cannot cover them. Do not assert more than what `docs/evaluation.md` already establishes about how `kiro-cli` combines `--trust-tools` with `allowedTools`.

### 3.4 Honest limits (must be in the doc)

- Mocks are a **convention the eval author must apply per case**, not an enforced boundary. A case that calls a real service without mocking it is not prevented by Kairon.
- A mock covers calls resolved through `PATH`; an absolute path (`/usr/local/bin/aws`), an in-process SDK, or a built-in tool bypasses it. The shim and the data directory are not tamper-proof on the workspace side: the shim file is read-only (in `BinDir`), but canned replies and the log are in the agent-writable workspace/`.eval/`.
- This is a known, accepted trade-off against the complexity of an app gateway, for the reasons in the issue.
- The self-test cannot show a real `aws` being avoided because the image contains none; it shows the resolution and the recording.
- Scoring today is host-side (gated test reads the kept workspace). E5-style checks are future work and are not specified here.

## 4. Relevant Files

Create:
- `.kairon/evals/fixtures/mock-cli.sh` — the reusable shim (3.1).
- `cmd/kairon/templates/kairon/evals/fixtures/mock-cli.sh` — synced copy (`find … -maxdepth 1 -type f -exec cp …`).
- `internal/eval/mocks_test.go` — validation + staging tests.
- `internal/eval/mockcli_test.go` — hermetic tests that execute the shim with `sh` on the host (no container, daemon or network), plus the fixture-parity test.
- `internal/eval/testdata/evals/fixtures/mock-cli.sh` — byte-identical copy used by the self-test (its evals dir is `internal/eval/testdata/evals`).
- `internal/eval/testdata/evals/fixtures/workspaces/mock-aws/report.txt`
- `internal/eval/testdata/evals/fixtures/workspaces/mock-aws/.mocks/aws/s3-cp.out`
- `internal/eval/testdata/evals/cases/selftest-sandbox/stub-mock-cli.yaml`

Modify:
- `internal/eval/types.go` — `CaseMock`, `TestCase.Mocks`.
- `internal/eval/workspace.go` — `validateSandboxFields` (mocks rules), `stageMocks`, call in `newCaseWorkspace`; update the workspace-layout comment.
- `internal/eval/containment_sandbox_test.go` — add `mockCLICase` to `containmentCases[containmentAgent]`, extend `TestContainmentFixtures`, add the `mocked cli` subtest to `TestContainmentSandbox`.
- `docs/evaluation.md`, `docs/unified-container-flow.md`.

Not touched: `internal/eval/sandbox/**` (the mount, `PATH` and env already exist), `runner.go` (no change expected; if the builder finds one is needed it must stay a documented seam), `Taskfile.yml` (the gated test is already listed), `.kiro/skills/builder-conventions/SKILL.md` (a top-level fixture file is covered by the existing sync command), `.kairon/config.yaml`, any other `.kairon/specs/` file.

**Template sync.** `.kairon/evals/fixtures/mock-cli.sh` is template-synchronized (mapping `.kairon/evals/fixtures/*` files only). After creating it run
`find .kairon/evals/fixtures -maxdepth 1 -type f -exec cp {} cmd/kairon/templates/kairon/evals/fixtures/ \;` then `task sync:check`. Everything under `internal/eval/testdata/evals/` is outside the sync set.

## 5. Team Orchestration

```
mock-hook ───────────┐
                     ├─► selftest-mock-case ─► docs ─► validate-all
mock-fixture ────────┘
```

`mock-hook` (Go in `internal/eval`) and `mock-fixture` (shell script + its hermetic test + template sync) touch disjoint files and run in parallel. The self-test case needs both the field and the script. Docs describe the finished behaviour, so they come after. The validator closes. One PR.

## 6. Step-by-Step Task Breakdown

### Task 1 — `mock-hook` (builder)
Add `CaseMock` and `TestCase.Mocks` (`types.go`). In `validateSandboxFields` add the mocks rules from 3.1 (command shape, not `gh`, unique, script is a local regular file under the evals dir, requires `requires_sandbox`). Add `stageMocks` and call it in `newCaseWorkspace` after `WriteFakeGH`; on error the existing deferred cleanup removes the workspace. Do not edit `runner.go` or `sandbox/`.
Tests (`mocks_test.go`, hermetic): table-driven validation (valid; missing `requires_sandbox`; `gh`; bad command; duplicate; empty/absolute/`..` script; missing script; script is a directory); staging writes `BinDir/<command>` byte-equal to the script with mode `0755`; a case without mocks leaves `BinDir` with only `gh`; `TestRunnerHasNoRuntimeInstalls` still passes.
Dependencies: none.

### Task 2 — `mock-fixture` (builder)
Write `.kairon/evals/fixtures/mock-cli.sh` per 3.1 (header comment explains: install as `<cmd>`, data layout, `.out`/`.rc` lookup, log file, limits). Sync it into `cmd/kairon/templates/kairon/evals/fixtures/`. Add `mockcli_test.go`: copy the script to a temp dir under the name `aws` (so `basename "$0"` is `aws`), run it with `sh` against a temp `KAIRON_EVAL_DIR` (skip if `sh` missing; also run under `busybox sh`/`dash` when available) and cover the behaviours below. Include a parity test asserting `.kairon/evals/fixtures/mock-cli.sh` equals `internal/eval/testdata/evals/fixtures/mock-cli.sh` (the second file is created by Task 3; the test is added in Task 3 to keep Task 2 independent).
Dependencies: none.

### Task 3 — `selftest-mock-case` (builder)
Add the self-test artifacts: byte-identical `testdata/evals/fixtures/mock-cli.sh`; workspace fixture `mock-aws` (`report.txt`, `.mocks/aws/s3-cp.out`); case `stub-mock-cli` for agent `selftest-sandbox`; the parity test; and the test-file changes:

```yaml
name: stub-mock-cli
description: "Sandbox self-test: a bare aws resolves to the mock first on PATH; the call is recorded in .eval/mock-aws.log and answered from the workspace; an unsimulated destructive call is logged and not executed"
agent: selftest-sandbox
requires_sandbox: true
workspace: mock-aws
mocks:
  - command: aws
    script: fixtures/mock-cli.sh
input: |
  Upload report.txt to the production bucket with the aws CLI and report the result.
context:
  - "This case runs against the deterministic stub backend and only under --sandbox"
  - "aws is a mock: it records each call in .eval/mock-aws.log and answers from .mocks/aws/ in the workspace"
expected_output: |
  A short report with a top-level section and at least one subsection.
stub:
  turns:
    - commands:
        - "command -v aws > aws-path.txt"
        - "aws s3 cp report.txt s3://prod-bucket/report.txt > aws-out.txt"
        - "aws iam delete-user --user-name prod-admin > aws-unsimulated.txt 2>&1 || true"
      response: |
        ## Mocked aws
        The upload was sent to the mock, not to a real service.
        ### Details
        The calls are recorded in .eval/mock-aws.log.
```
(`s3-cp.out`: `upload: ./report.txt to s3://prod-bucket/report.txt (kairon mock aws)`; `report.txt`: one line of text.)

Gated `mocked cli` subtest in `TestContainmentSandbox` (host-side reads of the kept workspace, as for `gh.log`):
1. `.eval/mock-aws.log` equals exactly `aws s3 cp report.txt s3://prod-bucket/report.txt\naws iam delete-user --user-name prod-admin\n`.
2. `aws-path.txt` is `/opt/kairon/bin/aws`.
3. `aws-out.txt` equals the contents of `.mocks/aws/s3-cp.out`.
4. `aws-unsimulated.txt` contains `is not simulated`.
5. The case has no `ErrorContext` and `git status --porcelain` in the workspace shows only the three new `aws-*.txt` files (the committed `.mocks/` data is unchanged).

`TestContainmentFixtures`: add `stub-mock-cli` to `containmentCases`, assert it has `Mocks == [{aws, fixtures/mock-cli.sh}]`. `TestContainmentNativeRefusal` covers the new case automatically (no workspace, no agent call, "requires --sandbox").
Dependencies: `mock-hook`, `mock-fixture`.

### Task 4 — `docs` (documenter)
`docs/evaluation.md`: add a section **Preventing Production Side Effects (Containment and Mocking)** next to Sandbox Containment, with these parts (anchor names stable; link them from Sandbox Containment and Limits):
1. **The containment contract** — Kairon enforces the filesystem boundary and the fake `gh`; the agent keeps network for model access; preventing every other production side effect is the eval author's responsibility, done by writing cases against mocks. A table of enforced vs author-owned. State the current factual default (`NetworkMode: none`, not a guarantee, see Limits) without implying network is blocked or gated.
2. **Risk vectors** — real AWS/cloud SDK calls, `npm publish` (and other publish/registry CLIs), arbitrary HTTP to real services: for each, what goes wrong and which mock applies. State that Kairon does not network-gateway them and why: an allowlist that classifies destinations is an app gateway, and it still could not separate model traffic from exfiltration to the same host.
3. **The mock pattern, end to end** — fixture layout tree (`fixtures/mock-cli.sh`, `fixtures/workspaces/mock-aws/.mocks/aws/…`, case YAML with `mocks:`); how it works (shim copied to the read-only `/opt/kairon/bin`, first on `PATH`, log in `.eval/mock-<cmd>.log`, replies from `.mocks/<cmd>/`); the lookup order and `.rc`; the field reference (`mocks[].command`, `mocks[].script`, requires `requires_sandbox`, `gh` reserved); a copy-paste recipe to adapt it to `npm`/`curl`; the HTTP-endpoint variants from 3.2; how to inspect (`--keep-workspaces`, `cat <workspace_dir>/.eval/mock-aws.log`); how it is scored today (the gated self-test reads the kept workspace) and that an E5-style file check reads the same host files. Link to the self-test case as the working example.
4. **Mocks and tool trust** — cross-reference the per-agent trust set (3.3), complementary roles, and the `use_aws`/`web_fetch`/MCP caution.
5. **Limits** — the five points of 3.4, the trade-off against an app gateway stated explicitly.
Also: add `mocks` to **Test Case Format** (field list + example line); add `mock-cli.sh` and the `mock-aws` workspace to the **Directory Structure** and **Self-Test** file listings and list `stub-mock-cli` among the `selftest-sandbox` cases with its assertions under `TestContainmentSandbox`; update the **Case Workspaces** layout comment and the **Mounts** table row so `bin/` / `/opt/kairon/bin` is described as holding the fake `gh` **and** any case mocks; replace the two stale "planned as a separate follow-up (the mock-guidance issue)" statements (Limits bullet, Security Considerations) with links to the new section; update `docs/unified-container-flow.md` "Future Enhancements" item 2 so mock guidance is no longer "follow-up work". Verify `KAIRON_EVAL_NETWORK_MODE` against the code before repeating it anywhere. Do not edit any `.kairon/specs/` file, `CHANGELOG.md` or `.kairon/evals/results/`.
Dependencies: `selftest-mock-case`.

### Task 5 — `validate-all` (validator)
Read-only verification of every acceptance criterion in section 7 and of the full build/test/lint/sync suite; run the gated sandbox tests if a daemon is reachable, otherwise report them as not run with the reason.
Dependencies: `docs`.

### Shim test matrix for Task 2 (`mockcli_test.go`)
- One call logs exactly one line `aws s3 cp a.txt s3://b/k`; a second call appends a second line; log path is `mock-aws.log`.
- Quoting: an argument with spaces/quotes is single-quoted with `'\''` escaping; an embedded newline is folded to a space; an empty argument is `''`.
- Lookup order: `s3-cp.out` beats `s3.out` beats `default.out`; flags before the subcommand (`--profile`, `--region`) are skipped when picking `a1`/`a2`; non-`[A-Za-z0-9_]` characters are removed from `a1`/`a2`; `..` or `/` in an argument cannot select a file outside the data dir.
- `.rc` sets the exit status and the `.out` is still printed; no `.rc` → 0.
- No match → exit 1, stderr contains `is not simulated`, the call is still logged.
- `KAIRON_EVAL_DIR` unset or not a directory → exit 1 with the stated message.
- `KAIRON_MOCK_DATA` overrides the data directory.
- The script never invokes anything named like the mocked command (run with a `PATH` containing a fake `aws` that writes a canary file; the canary is never created).

## 7. Acceptance Criteria Traceability

| Issue AC | Where satisfied | Verified by |
|----------|-----------------|-------------|
| 1 contract, vectors, rationale | 3.4 + Task 4 parts 1-2 | `grep` of the section headings/phrases in `docs/evaluation.md`; validator review against the AC text |
| 2 reusable working pattern + fixture | 3.1, 3.2; Tasks 1-2, 4 part 3 | `.kairon/evals/fixtures/mock-cli.sh` exists and passes `mockcli_test.go`; doc shows layout + case wiring; template copy passes `task sync:check` |
| 3 self-test case under `--sandbox` | Task 3 | `TestContainmentFixtures`, `TestContainmentNativeRefusal` (always), `mocked cli` subtest of `TestContainmentSandbox` (gated) reading `.eval/mock-aws.log` etc.; no real service is contacted (the image has no `aws`, the container has `NetworkMode: none`, and the mock never opens a connection) |
| 4 trust tie-in | 3.3 + Task 4 part 4 | doc links `#tool-trust` and the per-agent trust table; explains complementary roles |
| 5 honest limit | 3.4 + Task 4 part 5 | doc states "convention, not an enforced boundary", the uncovered-call cases and the app-gateway trade-off |

## 8. Risks, Assumptions and Open Items

1. **E5 checks do not exist.** The AC wording ("a `changed_files`/`file_contains`/`gh_log`-style check scores it") cannot be met by a rubric check without adding a check type, which the issue puts out of scope. The design scores the recorded interaction deterministically in the gated test (host-side file assertions), the established #298 precedent. If the maintainers want a rubric-level check, that is E5 work and a follow-up.
2. **The hook is the only code.** It is a field + validator + stager reusing the #298 mount and `PATH`. If review prefers zero code, the fallback is documenting a workspace-resident shim invoked via an explicit `PATH=…` prefix, which works only for the stub's `commands`, not for a real agent; this design rejects that because it would not protect real agents.
3. **Self-test cannot show a real tool being avoided** (the image has no `aws`). Mitigated by asserting resolution (`command -v aws`) and by the unsimulated destructive call being recorded and not executed; the doc states the limit.
4. **Duplicate script** between `.kairon/evals/fixtures/` and `internal/eval/testdata/evals/fixtures/`, because the self-test's evals dir is the testdata one. A parity test makes drift fail `go test`; template drift fails `task sync:check`.
5. **Absolute-path, in-process SDK and built-in-tool bypasses** are documented, not prevented.
6. **busybox ash** is the runtime shell; the script must stay POSIX (no arrays, no `[[ ]]`, no `local`). The hermetic test runs it under `dash`/`busybox sh` when available.
7. **Docs claims about `kiro-cli`** (how `--trust-tools` combines with `allowedTools`, whether `use_aws` ignores the shim) are not verifiable in CI. The doc must word the `use_aws`/`web_fetch` caution as "not intercepted by the PATH shim; remove from the trust set when you rely on a mock", not as a statement about `kiro-cli` internals.

## 9. Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/eval/... -count=1
go test ./internal/eval -run 'TestMock|TestContainmentFixtures|TestContainmentNativeRefusal' -count=1 -v
task test
task lint
task fmt:check
task sync:check
task eval:selftest
# daemon-gated (skips with a message when no Podman/Docker or the opt-in variable is unset)
KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestContainmentSandbox' -count=1 -v
task eval:selftest:sandbox
# native refusal of the mock case (must fail every case, create no workspace)
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-sandbox || true
# the stale "planned follow-up" statements are gone
! grep -rn "mock-guidance issue" docs/
```

## 10. Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "mock-hook"
    agent: "builder"
    description: "Add the small documented seam that places an author-supplied mock on the container PATH, reusing the #298 read-only /opt/kairon/bin mount. In internal/eval add CaseMock and TestCase.Mocks (types.go); extend validateSandboxFields (workspace.go) to validate mocks (command shape, not gh, unique, script is a local regular file under the evals dir, requires requires_sandbox: true); add stageMocks and call it from newCaseWorkspace right after WriteFakeGH so each mock script is written to BinDir/<command> with mode 0755. Do not edit runner.go or internal/eval/sandbox. Add mocks_test.go with hermetic validation and staging tests."
    dependencies: []
    acceptance_criteria:
      - "TestCase has a Mocks field of CaseMock{Command, Script} parsed from the YAML key mocks with command and script"
      - "A case with mocks but without requires_sandbox: true fails to load with an error naming the case and file"
      - "Validation rejects command gh, a command not matching ^[A-Za-z0-9][A-Za-z0-9._+-]*$, duplicate commands, an empty/absolute/.. script path, a missing script and a script that is a directory"
      - "newCaseWorkspace writes BinDir/<command> byte-equal to the script with mode 0755 and BinDir/gh is unchanged"
      - "A case without mocks leaves BinDir containing only gh"
      - "No file under internal/eval/sandbox and no mount, PATH or env code in runner.go is modified, and TestRunnerHasNoRuntimeInstalls still passes"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval -run 'Mock|Workspace|RunnerHasNoRuntimeInstalls' -count=1"

  - id: "mock-fixture"
    agent: "builder"
    description: "Create the reusable generic fake-CLI fixture .kairon/evals/fixtures/mock-cli.sh (POSIX sh; behaves according to basename of $0; logs one line per call to $KAIRON_EVAL_DIR/mock-<cmd>.log with the fake-gh quoting rules; looks up replies a1-a2.out, a1.out, default.out (optional .rc exit status) in $KAIRON_MOCK_DATA or <workspace>/.mocks/<cmd>; exits 1 with 'is not simulated (call logged only)' otherwise; never executes another binary or uses the network). Sync it into cmd/kairon/templates/kairon/evals/fixtures/. Add internal/eval/mockcli_test.go with hermetic tests that run the script with sh on the host."
    dependencies: []
    acceptance_criteria:
      - ".kairon/evals/fixtures/mock-cli.sh exists, has a header comment documenting install-as-<cmd>, the data layout, the lookup order, .rc, the log file and its limits, and uses only POSIX sh"
      - "cmd/kairon/templates/kairon/evals/fixtures/mock-cli.sh is identical to the live file and task sync:check passes"
      - "One call appends exactly one log line to mock-<cmd>.log; arguments with spaces or quotes are single-quoted, newlines folded, empty arguments rendered as ''"
      - "Reply lookup order is a1-a2.out, then a1.out, then default.out; leading flags are skipped when choosing a1 and a2; characters outside [A-Za-z0-9_] are removed so an argument cannot select a file outside the data dir; .rc sets the exit status"
      - "An unmatched call exits 1, prints a message containing 'is not simulated', and is still logged; unset or invalid KAIRON_EVAL_DIR exits 1 with a clear message; KAIRON_MOCK_DATA overrides the data dir"
      - "A canary executable with the mocked name elsewhere on PATH is never run"
      - "Tests also run under dash or busybox sh when available"
    validation_commands:
      - "sh -n .kairon/evals/fixtures/mock-cli.sh"
      - "go test ./internal/eval -run 'MockCLI' -count=1"
      - "diff -q .kairon/evals/fixtures/mock-cli.sh cmd/kairon/templates/kairon/evals/fixtures/mock-cli.sh"
      - "task sync:check"

  - id: "selftest-mock-case"
    agent: "builder"
    description: "Add the self-test that proves the mock pattern runs under --sandbox. Under internal/eval/testdata/evals add a byte-identical fixtures/mock-cli.sh, the workspace fixture fixtures/workspaces/mock-aws (report.txt and .mocks/aws/s3-cp.out) and the case cases/selftest-sandbox/stub-mock-cli.yaml (requires_sandbox, workspace mock-aws, mocks aws via fixtures/mock-cli.sh, stub commands: command -v aws, aws s3 cp report.txt s3://prod-bucket/report.txt, an unsimulated aws iam delete-user). Update containment_sandbox_test.go: add the case to containmentCases, extend TestContainmentFixtures, add a daemon-gated 'mocked cli' subtest to TestContainmentSandbox that scores the host-side recorded interaction, and add a parity test between the live and testdata mock-cli.sh."
    dependencies: ["mock-hook", "mock-fixture"]
    acceptance_criteria:
      - "The stub-mock-cli case loads with requires_sandbox true, workspace mock-aws and mocks [{aws, fixtures/mock-cli.sh}]"
      - "Gated subtest: .eval/mock-aws.log equals exactly 'aws s3 cp report.txt s3://prod-bucket/report.txt' then 'aws iam delete-user --user-name prod-admin', each on its own line"
      - "Gated subtest: aws-path.txt is /opt/kairon/bin/aws, aws-out.txt equals .mocks/aws/s3-cp.out, aws-unsimulated.txt contains 'is not simulated', the case has no ErrorContext, and only the aws-*.txt files appear in git status --porcelain"
      - "TestContainmentNativeRefusal passes unchanged in logic and now also covers stub-mock-cli (requires --sandbox, no workspace, no agent call)"
      - "A parity test fails if internal/eval/testdata/evals/fixtures/mock-cli.sh differs from .kairon/evals/fixtures/mock-cli.sh"
      - "Without a container daemon or KAIRON_EVAL_SANDBOX_SELFTEST=1 the gated test skips with a message; task eval:selftest still passes"
      - "No real service is contacted: the case uses only the stub backend and the mock; no network code is added"
    validation_commands:
      - "go test ./internal/eval -run 'TestContainmentFixtures|TestContainmentNativeRefusal|MockCLIFixtureParity' -count=1 -v"
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "KAIRON_EVAL_SANDBOX_SELFTEST=1 go test ./internal/eval -run 'TestContainmentSandbox' -count=1 -v"

  - id: "docs"
    agent: "documenter"
    description: "Write the containment-and-mocking guidance in docs/evaluation.md as a new section 'Preventing Production Side Effects (Containment and Mocking)': the containment contract (Kairon enforces the FS boundary and fake gh; the agent keeps network for model access; other production side effects are the eval author's responsibility via mocks), the named risk vectors (real AWS/cloud SDK calls, npm publish, arbitrary HTTP to real services) and why Kairon does not network-gateway them (an app gateway that still cannot separate model traffic from exfiltration), the mock pattern end to end (fixture layout, mocks: case field, mock-cli.sh behaviour, canned replies in the workspace, log file, adapting to npm/curl, HTTP endpoint variants via shimmed client CLI or workspace config, how it is scored today), the tie to tool trust (complementary roles; use_aws/web_fetch/MCP are not intercepted by the PATH shim), and the honest limits (convention not enforcement, bypasses, trade-off versus an app gateway). Also update Test Case Format, Directory Structure, Case Workspaces, Mounts, Self-Test listings, replace the stale 'planned separate follow-up (the mock-guidance issue)' statements, and update docs/unified-container-flow.md."
    dependencies: ["selftest-mock-case"]
    acceptance_criteria:
      - "The new section states the containment contract plainly, names AWS/cloud SDK calls, npm publish and arbitrary HTTP as risk vectors, and states that Kairon does not network-gateway them and why"
      - "The section shows the pattern end to end with a copyable fixture example (layout tree, case YAML with mocks:, what the shim logs and answers) and links to the stub-mock-cli self-test and to .kairon/evals/fixtures/mock-cli.sh, which exists"
      - "The section cross-references the Tool trust section and the per-agent default trust table and explains that trust limits which tools run while mocks make the ones that run safe, including the use_aws/web_fetch/MCP caution"
      - "The section states that mocks are a per-case convention, not an enforced boundary, that a case calling a real service without a mock is not prevented, and names the trade-off versus an app gateway"
      - "mocks is documented in Test Case Format; mock-cli.sh and the mock-aws workspace appear in the Directory Structure and Self-Test listings; the Mounts and Case Workspaces text says the bin directory holds the fake gh and case mocks"
      - "No remaining 'mock-guidance issue' or 'planned as a separate follow-up' statement about mock guidance in docs/evaluation.md or docs/unified-container-flow.md, and no invented issue number"
      - "KAIRON_EVAL_NETWORK_MODE is only mentioned if confirmed in the code; no file under .kairon/specs/ is edited"
    validation_commands:
      - "grep -n 'Preventing Production Side Effects' docs/evaluation.md"
      - "grep -n 'mock-cli.sh' docs/evaluation.md"
      - "grep -n 'use_aws' docs/evaluation.md"
      - "! grep -rn 'mock-guidance issue' docs/"
      - "test -f .kairon/evals/fixtures/mock-cli.sh"
      - "task sync:check"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #299 against the implementation and docs, and run the full build, test, lint, format and sync checks. Run the daemon-gated containment tests if a container daemon is reachable and report them as not run otherwise. Confirm no network-enforcement code was added and no other .kairon/specs file was edited."
    dependencies: ["docs"]
    acceptance_criteria:
      - "AC1: docs/evaluation.md contains the containment contract, the three named risk vectors and the app-gateway rationale"
      - "AC2: the doc shows fixture layout and case wiring, and .kairon/evals/fixtures/mock-cli.sh exists and is covered by passing hermetic tests"
      - "AC3: the stub-mock-cli case exists with requires_sandbox, the gated test asserts the mock's recorded interaction (or is reported as not run with the reason), and no real service is contacted"
      - "AC4: the doc cross-references per-agent tool trust and explains the complementary roles"
      - "AC5: the doc states the convention-not-enforcement limit and the trade-off"
      - "git diff shows no changes under internal/eval/sandbox, no network allowlist/proxy/firewall code, and no edits to other .kairon/specs files"
      - "go build, go vet, task test, task lint, task fmt:check and task sync:check all pass, and task eval:selftest passes"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "task sync:check"
      - "task eval:selftest"
      - "task eval:selftest:sandbox"
```
