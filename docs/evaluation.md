# Evaluation Framework

Kairon's evaluation framework measures agent quality and cost, enabling data-driven prompt improvements.

## Directory Structure

```
.kairon/evals/
  rubrics/           # Scoring criteria per agent
    architect.yaml
    builder.yaml
    documenter.yaml
    krew-lead.yaml
    planner.yaml
    validator.yaml
  cases/             # Test cases per agent
    architect/
      case-1.yaml
    builder/
      case-1.yaml
    documenter/
      case-1.yaml
    krew-lead/
      case-1.yaml
    ...
  fixtures/          # Files referenced by cases
    mock-cli.sh      # Reusable stand-in for any CLI (aws, npm, curl, ...); see Preventing Production Side Effects
    workspaces/      # Optional per-case workspace fixtures: workspaces/<name>/ (see Case Workspaces)
    hidden/          # Optional files a `command` check injects while it runs; the agent never sees them (see Checks)
  results/           # One directory per run: <timestamp>-<git-short-hash>
    <timestamp>-<git-short-hash>/
      architect.json
      builder.json
      documenter.json
      krew-lead.json
      summary.json
```

This is the default evals directory. Use `--evals-dir` to point the harness at a different directory with the same layout (see [Evals Directory](#evals-directory---evals-dir)).

Iteration logs live outside the evals directory, in `.kairon/iterations/<agent>/iteration-NN.md`, and reference the runs under `results/` (see [Iteration Logs and `verify-log`](#iteration-logs-and-verify-log)). The same directory holds an agent's optional `load-bearing.md` table, which `kairon eval audit` reads together with the agent's cases under `<evals-dir>/cases/<agent>/` (see [Prompt Audit](#prompt-audit-kairon-eval-audit)).

## Rubric Format

Each agent has a rubric YAML file defining scoring criteria:

```yaml
agent: architect
pass_threshold: 95         # Optional: percent a case/agent must reach to pass (default 95)
criteria:
  - name: task_decomposition
    description: "Spec breaks work into discrete, independently implementable tasks"
    scoring: 1-5
  - name: file_reference_accuracy
    description: "Referenced files exist and are relevant"
    scoring: 1-5
    deterministic: true    # Scored by code, not LLM
  - name: cost_efficiency
    description: "Token usage relative to output quality"
    type: cost             # Tracked as cost metric
```

A rubric whose cases are scored purely by [checks](#checks) sets `checks_only: true`:

```yaml
agent: builder
checks_only: true          # Optional: score every case from its checks alone (default false)
criteria:
  - name: file_written
    description: "The expected file exists in the workspace"
    scoring: 1-5           # Ignored for a checked criterion: its maximum is its check count
  - name: report_marker
    description: "The agent's report carries the expected marker"
    scoring: 1-5
  - name: cost_efficiency
    description: "Token usage relative to output quality"
    type: cost
```

A rubric can also require that no single criterion is neglected, even when the agent's overall score is high, by setting `min_criterion_score`:

```yaml
agent: builder
checks_only: true
pass_threshold: 60
min_criterion_score: 50    # Optional: every scored criterion must reach 50% on its own (default: unset)
criteria:
  - name: file_written
    description: "The expected file exists in the workspace"
    scoring: 1-5
  - name: report_marker
    description: "The agent's report carries the expected marker"
    scoring: 1-5
```

Fields:
- `agent` — which agent this rubric evaluates
- `pass_threshold` — (optional) the percent a case, and the agent as a whole, must reach to pass. It must be greater than 0 and at most 100 (`(0, 100]`); the default is `95`. A value of `0` or below, above `100`, or NaN makes loading the rubric fail with an error that names the rubric file. A case can override it with `min_score` (see [Test Case Format](#test-case-format) and [Pass/Fail Verdict and Exit Status](#passfail-verdict-and-exit-status)).
- `min_criterion_score` — (optional) a per-criterion floor, in percent. When set, the agent fails if any non-cost criterion's pooled percent is below it, even when the agent's overall `score` meets `pass_threshold` (see [Agent verdict](#agent-verdict)). It is unset by default and must be greater than 0 and at most 100 (`(0, 100]`). A value of `0` or below, above `100`, or NaN makes loading the rubric fail with an error that names the rubric file and the field (`invalid rubric <file>: min_criterion_score must be a percent in (0, 100], got <value>`); `0` is rejected rather than read as "off", so leave the field out to turn the floor off. The comparison is `>=`: a criterion exactly at the floor passes. A rubric without the field is judged exactly as before.
- `checks_only` — (optional, boolean, default `false`) when `true`, every case of the agent is scored from its [checks](#checks) alone. A non-cost criterion that a case has no checks for is **not scored** in that case: it gets no entry in the case's scores and adds nothing to the case or agent score or maximum. Neither the legacy LLM judge nor the keyword heuristic runs for it, so no legacy judge call is made. [`judge` checks](#the-judge-check) are still checks and remain allowed. Every case of the agent must have at least one check; a case without any is rejected when the cases are loaded, before any case runs, with an error that names the case file (see [`checks_only` rubrics](#checks_only-rubrics)). Omitting the field leaves scoring unchanged.
- `criteria[].name` — unique identifier for the criterion
- `criteria[].description` — what is being measured
- `criteria[].scoring` — score range (e.g. "1-5")
- `criteria[].deterministic` — if true, scored by code checks rather than LLM
- `criteria[].type` — set to "cost" for cost-tracking criteria

## Test Case Format

```yaml
name: simple-feature-issue
description: "Evaluate architect output for a simple feature request"
input: |
  The issue body or spec that the agent receives as input.
# turns:                   # Optional, instead of `input`: a scripted multi-turn conversation (see Multi-turn cases)
#   - "First user message"
#   - "Second user message"
# simulated_user:          # Optional, together with `input`: answer the agent's questions from a bank of facts (see Simulated user)
#   facts:
#     - {id: format, answer: "Comma separated, UTF-8.", discoverable: false}
#   default_reply: "I don't know, use your best judgement."
output: |
  Optional: pre-captured agent output for offline evaluation.
workspace: seeded         # Optional: workspace fixture name (see Case Workspaces)
min_score: 90              # Optional: pass threshold in percent for this case (overrides the rubric's pass_threshold)
timeout: 30s               # Optional: timeout per agent call, i.e. per turn (Go duration)
requires_sandbox: true     # Optional: refuse to run without --sandbox (see Sandbox Containment)
gh_issue:                  # Optional: data for the sandbox's fake `gh issue view` (needs requires_sandbox)
  title: "Add widget"
mocks:                     # Optional: author-supplied command mocks placed first on PATH (needs requires_sandbox)
  - command: aws
    script: fixtures/mock-cli.sh
checks:                    # Optional: deterministic pass/fail checks (see Checks)
  - criterion: completeness
    type: file_exists
    path: docs/summary.md
stub:                      # Optional: scripted response for `--backend stub`
  turns:
    - response: |
        ## Summary
        ### Details
```

Fields:
- `name` — unique identifier
- `description` — what this case tests
- `input` — the input the agent would receive (a single-turn case). Mutually exclusive with `turns`, unless the case has `simulated_user`, where `input` is the required opening message
- `turns` — (optional) a list of user messages that replaces `input` for a scripted multi-turn conversation: one agent call per entry, all in the same conversation and the same workspace (see [Multi-turn cases](#multi-turn-cases)). A case without `simulated_user` sets `turns` **or** `input`, never both: a non-blank `input` together with `turns` is a load error naming the case (`turns and input are mutually exclusive; use turns for a multi-turn case`). An empty list (`turns: []`) and a blank entry (`turns[2] is empty`) are load errors too. With `simulated_user`, `input` and `turns` may both be present: the `turns` are sent after the simulated phase (see [Simulated user](#simulated-user-elicitation-cases)).
- `simulated_user` — (optional) turns the case into an elicitation case: the harness answers whatever the agent asks from a fixed bank of `facts` (see [Simulated user (elicitation cases)](#simulated-user-elicitation-cases)). Requires a non-blank `input`.
- `output` — (optional) pre-captured output for offline scoring
- `setup` — (optional) extra prompt context; `type: file` entries read `path` from disk. In a `turns` case the setup context is delivered with **turn 1 only**; later turns are sent verbatim.
- `workspace` — (optional) name of a fixture under `<evals-dir>/fixtures/workspaces/` that the case's workspace starts from (see [Case Workspaces](#case-workspaces)). Must match `^[A-Za-z0-9._-]+$` (and not be `.` or `..`) and the fixture directory must exist, otherwise loading the cases fails with an error naming the case.
- `min_score` — (optional) the pass threshold for this case, in percent. It overrides the rubric's `pass_threshold` for this case only. A case's threshold is resolved in this order: the case `min_score`, then the rubric `pass_threshold`, then the default `95`. The threshold decides whether the case counts toward `cases_failed`; the agent's own verdict always uses the rubric threshold (see [Pass/Fail Verdict and Exit Status](#passfail-verdict-and-exit-status)).
- `timeout` — (optional) a positive Go duration such as `30s` or `2m`. It overrides the default timeout for this case, natively and under `--sandbox` (see [Case Timeout](#case-timeout)). In a `turns` case it applies to **each turn**, not to the case as a whole. An invalid or non-positive value is a load error naming the case.
- `requires_sandbox` — (optional, default `false`) when `true`, the case refuses to run without `--sandbox`: a native run records the case as failed with `case "<name>" requires --sandbox` before it creates a workspace or invokes anything (see [Sandbox Containment](#sandbox-containment)).
- `gh_issue` — (optional) the issue the sandbox's fake `gh issue view` answers with: `number` (default `1`), `title` (required), `body`, `state` (default `OPEN`), `author` (default `fake-user`), `labels`. It is only valid together with `requires_sandbox: true`; otherwise loading the cases fails with an error naming the case, because a native run would call the developer's **real** `gh` (see [The fake `gh`](#the-fake-gh)).
- `mocks` — (optional) a list of `{command, script}` entries that each place a mock of a command on the container `PATH`, so a bare `aws`, `npm` or `curl` resolves to the author's script instead of a real tool. `command` is the bare command name (it must match `^[A-Za-z0-9][A-Za-z0-9._+-]*$`, must be unique within the case, and cannot be `gh`, which is the harness's fake, or `kairon`, which is the harness's own binary mounted at `/usr/local/bin/kairon`; a mock of either would silently shadow it because the mock directory is first on `PATH`). `script` is a path relative to the evals directory (no absolute path, no `..`) naming an existing regular file; `fixtures/mock-cli.sh` is the reusable one. Like `gh_issue`, `mocks` is only valid together with `requires_sandbox: true`, because a native run has no such directory on `PATH` and would silently call the **real** tool; otherwise loading the cases fails with an error naming the case. See [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking).
- `checks` — (optional) a list of deterministic pass/fail checks on the workspace the agent left behind, its output and the fake `gh` log. A criterion that has checks is scored `passed` out of `total` instead of by a heuristic or the LLM judge. Each entry has a `criterion` (a non-cost criterion of the agent's rubric) and a `type`, plus the fields that type takes, including an optional `turn` on the `output_*`, `gh_log_*` and `judge` types for multi-turn cases. An invalid `checks` block is a **fatal** load error that names the case file, the 1-based check number and the type; it is not downgraded to a warning (see [Checks](#checks)).
- `stub.turns[]` — (optional) scripted model responses, used only by the `stub` backend. In a `turns` case there is exactly one entry per turn; in a `simulated_user` case the count is bounded by `max_turns` (see [Stub Case Fields](#stub-case-fields))
- `stub.routes` — (optional, `simulated_user` cases only) scripted routing decisions, one per simulated reply, used only by the `stub` backend (see [Stub Case Fields](#stub-case-fields))
- `stub.judge` — (optional) scripted yes/no answers for `judge` checks, a single value or a list, used only by the `stub` backend (see [Stub Case Fields](#stub-case-fields))

## Checks

A case can declare `checks`: small, deterministic assertions about what the agent actually *did*, not only what it said. They are the way to score file edits, scope of change, hidden tests and `gh` calls without an LLM judge.

```yaml
checks:
  - criterion: structural_completeness   # required: a non-cost criterion of the agent's rubric
    type: command                         # required: one of the thirteen types below
    run: "go test ./..."
```

Every check has `criterion` and `type`. The other fields depend on the type, and a field that does not belong to the type (for example `path` on a `command`, `expect_exit` on a `file_exists`, or `turn` on a `file_exists`) is a load error rather than being ignored. This catches typos such as `expected_exit`.

| `type` | Required fields | Optional fields | Passes when |
|--------|-----------------|-----------------|-------------|
| `command` | `run` | `expect_exit` (default `0`), `inject` | `sh -c <run>` in the workspace exits with `expect_exit` |
| `file_exists` | `path` | | `path` exists in the workspace (file, directory or symlink) |
| `file_absent` | `path` | | `path` does not exist |
| `file_contains` | `path`, `pattern` | | `path` is a regular file whose content matches the regex |
| `file_not_contains` | `path`, `pattern` | | `path` is a regular file that exists **and** whose content does not match |
| `changed_files` | `allow` (list of globs; `[]` is valid) | | no path was added, modified or deleted outside `allow` |
| `output_contains` | `pattern` | `turn` | the agent's output (default: the last turn's) matches the regex |
| `output_not_contains` | `pattern` | `turn` | the agent's output (default: the last turn's) does not match the regex |
| `gh_log_contains` | `pattern` | `turn` | the fake `gh` log (default: the final log) matches the regex |
| `gh_log_not_contains` | `pattern` | `turn` | the fake `gh` log (default: the final log) does not match the regex |
| `judge` | `question` | `files`, `turn` | the judge model answers the yes/no `question` with `yes`; with `turn` it is shown the conversation up to that turn and judges that turn's reply |
| `fact_asked` | `fact` | | the simulated-user log (`.eval/sim-log.jsonl`) has at least one decision routed to fact id `fact` |
| `fact_not_asked` | `fact` | | the simulated-user log has no decision routed to fact id `fact` |

`turn` is only accepted by the four `output_*` / `gh_log_*` types and by `judge`; see [The `turn` field](#the-turn-field) and, for what a judge check does with it, [The `judge` check](#the-judge-check).

### The thirteen check types

`command` runs a shell command in the workspace root and compares its exit status. `expect_exit` (0–255, default `0`) lets a case assert a specific non-zero status. `inject` copies files from `fixtures/hidden/` into the workspace for the duration of the command (see [Hidden files and `inject`](#hidden-files-and-inject)).

```yaml
checks:
  - criterion: structural_completeness
    type: command
    run: "go test ./..."
    expect_exit: 0
    inject: [hidden_test.go]
```

`file_exists` passes when the path exists in the workspace. Any entry type counts, including a directory or a symlink (the final component is not followed).

```yaml
checks:
  - criterion: structural_completeness
    type: file_exists
    path: docs/summary.md
```

`file_absent` passes when the path does not exist.

```yaml
checks:
  - criterion: structural_completeness
    type: file_absent
    path: scratch/debug.log
```

`file_contains` passes when the file exists, is a regular file and its content matches `pattern`.

```yaml
checks:
  - criterion: structural_completeness
    type: file_contains
    path: README.md
    pattern: '(?m)^## Usage$'
```

`file_not_contains` passes when the file exists, is a regular file and its content does **not** match `pattern`. A missing file fails: "the file is absent" is not the same as "the file does not contain the text", so use `file_absent` for that.

```yaml
checks:
  - criterion: structural_completeness
    type: file_not_contains
    path: main.go
    pattern: 'TODO|FIXME'
```

`changed_files` passes when every path that differs from the starting state matches at least one `allow` glob. Added, modified, deleted and untracked paths all count. `allow: []` is valid and means "nothing may change"; a missing `allow` is an error.

```yaml
checks:
  - criterion: structural_completeness
    type: changed_files
    allow:
      - README.md
      - "docs/**"
      - "**/*_test.go"
```

`output_contains` and `output_not_contains` look at the agent's output (the text recorded as `actual_output`, which for a multi-turn case is the last turn's output). With `turn: N` they look at turn N's output instead (see [The `turn` field](#the-turn-field)).

```yaml
checks:
  - criterion: structural_completeness
    type: output_contains
    pattern: '(?m)^## Summary'
  - criterion: structural_completeness
    type: output_not_contains
    pattern: '(?i)as an ai'
```

`gh_log_contains` and `gh_log_not_contains` look at the fake `gh` log, `.eval/gh.log`, one line per call such as `gh issue create --title t` (see [The fake `gh`](#the-fake-gh)). By default they read the final log; with `turn: N` they read the log as it stood at the end of turn N.

```yaml
checks:
  - criterion: structural_completeness
    type: gh_log_contains
    pattern: '(?m)^gh issue create'
  - criterion: structural_completeness
    type: gh_log_not_contains
    pattern: 'gh pr merge'
```

> **Native runs have no `gh` log.** The fake `gh` exists only under `--sandbox`; a native run calls the real `gh`, which Kairon does not log. Natively the log is empty unless the stub or the agent wrote `.eval/gh.log` itself, so `gh_log_contains` fails and `gh_log_not_contains` passes vacuously. `gh_log_*` checks do not require `requires_sandbox`, so the native self-test can exercise every type, but a case that asserts on real `gh` calls should set `requires_sandbox: true`. The log lives in `.eval/`, which the agent can write: it is evidence from the fake, not tamper-proof. The same holds for each per-turn snapshot of a `turns` case.

`fact_asked` and `fact_not_asked` read the simulated-user decision log, `.eval/sim-log.jsonl` (one JSON object per line with `turn`, `question`, `fact` and `discoverable`; see [The sim log](#the-sim-log)). `fact_asked` passes when at least one decision has `fact` equal to the given id; `fact_not_asked` passes when none has. `fact` is required on these two types and rejected on every other; `turn` is not accepted on them. A missing, symlinked, non-regular, oversized (over 10 MiB) or malformed log **fails** the check with a detail, including for `fact_not_asked`; it is never a silent pass.

```yaml
checks:
  - criterion: structural_completeness
    type: fact_asked
    fact: format
  - criterion: structural_completeness
    type: fact_not_asked
    fact: language
```

### The `turn` field

In a [multi-turn case](#multi-turn-cases) a check can say *when* something happened: "no `gh issue create` after turn 1, one after turn 3".

```yaml
checks:
  - criterion: structural_completeness
    type: gh_log_not_contains
    pattern: 'issue create'
    turn: 1
  - criterion: structural_completeness
    type: gh_log_contains
    pattern: 'issue create'
    turn: 3
  - criterion: structural_completeness
    type: output_contains
    pattern: 'labels are right'
    turn: 2
```

- **Allowed types.** `turn` is accepted only by `output_contains`, `output_not_contains`, `gh_log_contains`, `gh_log_not_contains` and `judge`. On any other type it is a load error (`turn is not valid for type file_exists`).
- **Why workspace checks cannot take it.** The workspace is inspected once, at the end of the case, so a `file_*`, `changed_files` or `command` check cannot honestly be pinned to an earlier turn. A `turn: 1` on such a check would silently test the final workspace, so it is rejected loudly instead.
- **1-based.** `turn` counts from 1, the same numbering as the position in `turns`. `turn: 0` (or a negative value) is a load error.
- **Default: the last turn.** A check without `turn` reads the last turn's output and the final gh log (a `judge` check judges the last turn's reply and sees the whole conversation, see [The `judge` check](#the-judge-check)), which is exactly what a single-turn case has always done.
- **Range.** `turn` must not exceed the number of turns of the case. A single-turn case has one turn, so `turn: 1` is legal there. An out-of-range `turn` is a load error naming the case, the 1-based check number and the type. A hand-built check passed to `checks.Evaluate` with an out-of-range turn yields a failed result with a detail (`turn 4 is out of range: the case has 3 turn(s)`), never a panic or a silent pass.
- **gh log per turn.** `.eval/gh.log` is one append-only file, so per-turn visibility is a **snapshot taken after each turn returns and before the next turn starts**. A `gh_log_*` check with `turn: N` reads the snapshot of turn N: the log up to the end of that turn, **cumulative** (calls from earlier turns are in it too). Snapshotting the content, rather than remembering an offset into the final file, means a later turn that rewrites or truncates the log cannot change what an earlier turn saw. The last turn's snapshot equals the final log. Each snapshot is capped at 10 MiB like the final log; an oversized snapshot fails the `gh_log_*` check that reads it.
- **Labels.** A check with `turn` set shows it in its label (`#3 gh_log_contains pattern=/issue create/ turn=3`), so `reasoning`, `checks[].label` and the printed `✗` lines show which turn failed.

### The `judge` check

`judge` asks the judge model one yes/no `question`. It is the check to use when the property cannot be expressed as a regex or a command, for example "does the issue body state a testable acceptance criterion for each requirement?". It counts toward its criterion exactly like the deterministic checks (`passed` out of `total`).

```yaml
checks:
  - criterion: structural_completeness
    type: judge
    question: "Does the issue body file state a testable acceptance criterion for each requirement?"
    files: [.eval/issue-body.md]   # optional: workspace files the judge may read
    turn: 2                        # optional (multi-turn cases): judge the reply of turn 2; default: the last turn
```

- **Verdict.** The judge replies with `{"reasoning": "...", "answer": "yes" | "no"}`. `yes` passes and `no` fails. The judge's reasoning is recorded in `checks[].detail` for both outcomes: `judge answered yes: <reasoning>` or `judge answered no: <reasoning>`. The answer is matched case-insensitively with surrounding whitespace ignored; anything else is not a verdict.
- **What the judge sees.** In a single-turn case: the `question`, the case `input`, the agent's final output and the content of each listed file, every one fenced with `BEGIN`/`END` markers. In a [multi-turn case](#multi-turn-cases) (a `turns` list of two or more entries, or a [simulated-user case](#simulated-user-elicitation-cases) that ran two or more turns) the `INPUT` and `AGENT FINAL OUTPUT` blocks are replaced by the **whole conversation**: every user message and every agent reply, in order, labelled by turn. The case's `expected_output` and `context` are not included, so make the question self-contained.
- **The conversation.** The prompt has a `CONVERSATION (turns 1 to N, in order)` header that names the agent reply of turn N (the last `AGENT REPLY` block) as the output under review, so a question about "the output" or "the final output" means that reply and earlier turns are context. Each message sits between its own markers, user message first, then the agent's reply, for each turn: `===BEGIN TURN 1 USER MESSAGE===` … `===END TURN 1 USER MESSAGE===`, `===BEGIN TURN 1 AGENT REPLY===` … `===END TURN 1 AGENT REPLY===`, `===BEGIN TURN 2 USER MESSAGE===`, and so on. The user messages are the ones **actually sent**: for a simulated-user case that includes the opening message, the fact answers and the `default_reply` the simulated user wrote, not only the scripted ones. Agent replies are not truncated. A one-element `turns` list and a classic `input` case are not multi-turn: their prompt is the single-turn one above, unchanged.
- **`turn`.** Optional, 1-based, and only meaningful in a multi-turn case (it is accepted on a single-turn case, where `turn: 1` changes nothing). Without `turn` the check judges the **last** turn, the same default as `output_*` checks, and sees the full conversation. With `turn: k` the judged output is turn k's reply and the transcript **stops there**: turns 1 to k are shown and nothing after turn k, so a `turn: 1` check cannot be swayed by what the agent said in turn 2. Use it for per-turn rules ("does each reply ask exactly one question?") by writing one check per turn. `turn` scopes only the transcript: `files` are still read from the workspace as the agent left it at the end of the case, not as of turn k.
- **Turn out of range.** A `turn` greater than the case's number of turns is a load error naming the case file, the 1-based check number and the type (`turn 9 is beyond the case's 2 turn(s)`), like on the other types that take `turn` (see [The `turn` field](#the-turn-field)). For a simulated-user case the load-time bound is `max_turns` plus the scripted turns, which may be more than the run actually performed (`stop_when` ends the simulated phase early). A turn beyond the turns that actually ran, and a hand-built case that bypasses the loader, **fails** the check at run time with `turn 9 is out of range: the case has 2 turn(s)`. The turn is checked before any listed file is read and before any model call, so an out-of-range `turn` costs no tokens and is never a silent pass. `turn: 0` or a negative value is a load error.

- **`files`.** Optional. Entries are workspace-relative literal paths (no globs): no absolute path, no `..`, nothing inside `.git`, no duplicates. `.eval/` is allowed, which is where an agent typically leaves an issue body. At run time only a regular, non-symlink file is read, and a symlink in any directory component is refused. A missing file, a symlink or a file below a symlinked directory fails the check with `file does not exist: <path>` (or a similar message) **before** any model call, so an agent that did not produce the artifact is not rescued by the judge and no tokens are spent. Files are read as the agent left the workspace, before any `command` check runs or injects.
- **Caps.** Each file is capped at 64 KiB and the files together at 256 KiB. Excess is cut, the prompt carries an explicit `[truncated: showing first N of M bytes]` marker and the file's header says `[truncated]`. Non-UTF-8 bytes are replaced.
- **Judge model.** The call runs on the host through the inference backend, pinned to `evals.judge_model` (see [Model Pinning and Run Provenance](#model-pinning-and-run-provenance)), with a 2-minute timeout. This holds under `--sandbox` too: judge calls never run in the container.
- **Fail closed.** A `judge` check never passes by default and is never skipped. An unparseable answer (`judge parse error: …`), a backend error (`judge call failed: …`), a missing or unreadable file, and a check evaluated with no judge configured (`no judge configured`) all **fail** the check. This differs from the legacy 1–5 judge, whose parse failures mark the criterion *skipped* (see [Skipped Criteria](#skipped-criteria)). The reply is parsed from the **last** `===JSON_START===` to the next `===JSON_END===`, so a judge that quotes the delimiters while explaining cannot confuse the parse.
- **Order.** `judge` checks are evaluated in the first pass with the other non-`command` checks, in the order they are written (see [Evaluation order](#evaluation-order)). One call is made per check; there is no batching. With the stub backend, `stub.judge` answers are consumed in that order.
- **Cost accounting.** Each call appends a `calls[]` record with `role: judge` and the `criterion` of the check, and a successful call's cost is added to the case's `judge_cost`. A call whose reply could not be parsed, or that failed, still has its `calls[]` record but adds nothing to `judge_cost` (see [Per-call records](#per-call-records-calls)).
- **No call when the agent produced nothing.** If the agent produced no output the checks are not evaluated (see [When the agent failed](#when-the-agent-failed)), so no judge call is made.
- **`--debug`.** With `kairon eval --debug`, each judge check prints `🔧 Debug: judge prompt (criterion=<name>, judge check #<n>)` followed by the full prompt to **stderr**, before the call. That is how to confirm that the judge was shown a listed file, or which turns of the conversation it was shown. A `turn: 1` check and an unscoped check on the same case print two different transcripts. The output contains the case input or the conversation, the agent's output and file contents, so do not share it where they are sensitive. The legacy judge's output is not affected.
- **Untrusted input.** The agent's output and the files are written by the agent under test, and may contain text that tries to steer the judge. The prompt tells the judge to treat them as data and not to follow instructions in them, but a hostile output can still sway a model. A `judge` verdict is evidence, not proof; prefer a deterministic check where one can express the property.

### Regex dialect

`pattern` is a Go (RE2) regular expression. It is **unanchored** and matched against the **whole** text (the file, the output or the log), not line by line. `^` and `$` therefore match the start and end of the text unless you add `(?m)`, and `.` does not match a newline unless you add `(?s)`. Flags go inline: `(?m)`, `(?s)`, `(?i)`. Use single-quoted YAML strings so backslashes survive (`'hidden_test\.go'`). A pattern that does not compile is a load error. There are no lookaheads or backreferences.

### Glob dialect

`changed_files.allow` entries are matched against the whole, slash-separated path relative to the workspace root (for example `docs/guide/intro.md`):

| Syntax | Matches |
|--------|---------|
| `*` | any run of characters except `/` |
| `?` | exactly one character except `/` |
| `[abc]`, `[a-z]`, `[!x]` | one character from the class; never `/` |
| `**` | any run of characters including `/`; as a whole segment (`a/**/b`, `**/x`) it also matches zero directories |
| `\c` | the character `c` literally |

A pattern without `/` therefore matches only a top-level path: `README.md` matches `README.md` but not `docs/README.md`, and `*.md` does not match `docs/a.md`. `docs/**` matches everything below `docs/`, and `**/*.go` matches a `.go` file at any depth. An empty pattern, a leading `/` or an unterminated `[` is a load error.

### How `changed_files` sees changes

The workspace is a git repository whose only commit is the fixture (see [Case Workspaces](#case-workspaces)). `changed_files` compares the working tree to that commit, so it sees staged, unstaged and untracked changes, and an agent that ran `git commit` cannot hide a change. Details:

- Paths under `.eval/` (the harness outputs directory) are ignored.
- Paths that git ignores are invisible. That includes anything in `.gitignore` and `.git/info/exclude`, which is how `.kiro/` and `.eval/` are kept out of `git status`.
- Offending paths are listed in the failure detail with their kind (`added`, `modified` or `deleted`).
- If the directory is not a git repository, or git fails, the check fails with the error.

### Evaluation order

All checks of a case are evaluated **once**, after the last turn finishes and while the workspace still exists. (The per-turn outputs and gh log snapshots that `turn` checks read were recorded during the run; the checks themselves still run only at the end.)

1. Every non-`command` check (`file_*`, `changed_files`, `output_*`, `gh_log_*`, `judge`) is evaluated first, against the workspace exactly as the agent left it.
2. `command` checks then run, in the order they are listed.
3. Results are reported in the order the checks are written in the case.

This makes `file_*` and `changed_files` independent of what a command does to the workspace and of `inject`.

### Hidden files and `inject`

`fixtures/hidden/` holds files the agent must not see, such as hidden tests. A `command` check can list them in `inject`:

```yaml
checks:
  - criterion: structural_completeness
    type: command
    run: "go test ./..."
    inject: [hidden_test.go]
```

- Each entry is a path relative to `<evals-dir>/fixtures/hidden/`. It must be a relative, non-escaping path to an existing regular file; symlinks are rejected. The file is copied to the same relative path in the workspace.
- Injection happens only **after** the agent has finished, immediately before the command runs, and the workspace is restored right after it. Files that did not exist are removed again (and directories the injection created); a file the agent had already written at that path is overwritten for the run and put back afterwards. The agent therefore never sees an injected file, and later checks and `--keep-workspaces` see the workspace as the agent left it.
- The destination must not be under `.git`, `.eval` or `.kiro`.
- `fixtures/hidden/` is excluded from the `task sync:check` comparison with the shipped templates, like `fixtures/workspaces/`.

### Command checks run on the host

> **Warning: `command` checks execute on the host, including in `--sandbox` runs.**
> The sandbox base image carries no language toolchains, and the evaluator must work on any directory, so `command` checks do not run in the container. A command such as `go test ./...` therefore runs code **the agent wrote** with **your** privileges, which weakens the containment of `--sandbox` for those cases. Only use `command` checks in cases whose agent output you are prepared to run, and review what an agent can change before you add one.

What the evaluator does to limit the damage: the command runs with `sh -c` in the workspace directory with stdin closed; GitHub credential variables (`GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_HOST`) are removed from its environment; it runs in its own process group that is killed on timeout (5 minutes per command); and file paths and `inject` destinations are not followed through symlinks. None of this is a sandbox. File-based checks (`file_*`, `changed_files`) and `output_*` / `gh_log_*` checks only read, and never execute anything.

### Path rules

`path` must be relative and stay inside the workspace: an absolute path or one with `..` is a load error. At run time the evaluator also refuses to follow a symlink in any directory component, and `file_contains` / `file_not_contains` refuse a symlink as the file itself, so an agent cannot point `out.txt` at a host file. Reads are capped at 10 MiB; a larger file fails the check with a message.

### Scoring and reasoning

A criterion with at least one check in a case is scored by those checks:

- `score` is the number of checks that passed and `max_score` is the number of checks, so the criterion is **passed out of total** (`2/3`), not a 1–5 value. `deterministic` is `true`.
- The rubric's `scoring` range, its `deterministic` flag and the legacy 1–5 LLM judge are not used for that criterion in that case, so no legacy judge call is made for it. A [`judge` check](#the-judge-check) is still a check, so it is scored `passed`/`total` like the others, but it does make one yes/no judge call per check.
- Every failed check is named in `reasoning` with its 1-based position, type and key fields, then its detail in parentheses (each detail is shortened to 200 characters there; the full text, up to 2 KiB, stays in `checks[].detail`).

```
3/3 checks passed
1/2 checks passed; failed: #2 file_exists path=missing.txt (file does not exist)
```

The key fields in a label are `command "<run>"`, `path=<path>`, `pattern=/<regex>/` or `allow=[<globs>]`.

**Criteria without checks keep the legacy heuristics.** A criterion that no check names is scored exactly as before: deterministic criteria use the keyword heuristics on the agent's output, other criteria go to the LLM judge. A case can mix both, for example checks on `structural_completeness` and the judge for `clarity`. A rubric that sets [`checks_only`](#checks_only-rubrics) is the exception: its unchecked criteria are not scored at all. Cost criteria cannot have checks. Skipped criteria still count toward the totals, as `0` out of their maximum (see [Skipped Criteria](#skipped-criteria)).

When the case prints, every failed check is listed on its own line whatever the percentage:

```
      ✗ #2 file_exists path=missing.txt: file does not exist
```

### Result JSON

A checked criterion carries a `checks` array in `<agent>.json`, one entry per check in the order they were written. Older result files without it still load.

```json
{
  "name": "structural_completeness",
  "score": 1,
  "max_score": 2,
  "deterministic": true,
  "reasoning": "1/2 checks passed; failed: #2 file_exists path=missing.txt (file does not exist)",
  "checks": [
    { "index": 1, "type": "file_exists", "label": "#1 file_exists path=marker.txt", "passed": true },
    { "index": 2, "type": "file_exists", "label": "#2 file_exists path=missing.txt", "passed": false, "detail": "file does not exist" }
  ]
}
```

### When the agent failed

Checks assert on what the agent did. If the agent produced no output (the call failed, timed out or the sandbox refused the case) or there is no workspace, the checks are **not evaluated** and no command runs. In a multi-turn case a failure in any turn counts: the case has no output (see [Failure semantics](#failure-semantics)). Each criterion that has checks records `0/<total>`, is **not** skipped, and carries the reasoning `agent produced no output; checks not run`. Criteria without checks are marked skipped in this situation; a skipped criterion scores `0` and still counts toward the denominator, so a failed run can never score better than a run that was measured (see [Skipped Criteria](#skipped-criteria)).

A check that names a criterion missing from the scored rubric (only possible for a case that bypassed the loader) is never dropped: it records `0/<count>` with the reasoning `criterion "<name>" is not in the rubric`.

### Using the evaluator elsewhere

The check engine lives in its own package, `internal/checks` (import `github.com/jbrinkman/kairon/internal/checks`). It has no knowledge of the eval harness: it imports only the standard library, `gopkg.in/yaml.v3` and `internal/inference`, and never `internal/eval` or `internal/eval/sandbox` (a test enforces this). Other packages, such as the workflow engine, can therefore use it without linking the harness, and it can score any directory. `internal/eval` is a client of it and keeps thin aliases and wrappers for its own code and tests; new code should use `internal/checks` directly. To run a checks file from the command line, see [Running checks on any directory](#running-checks-on-any-directory-kairon-check).

```go
results := checks.Evaluate(list, checks.Input{
    Dir:       workspaceDir, // directory the file_*, changed_files and command checks look at
    Output:    agentOutput,  // text for output_contains / output_not_contains
    GHLog:     ghLogText,    // text for gh_log_contains / gh_log_not_contains
    Base:      "",           // optional git revision for changed_files; default HEAD
    Turns:     nil,          // optional per-turn evidence: Turns[i] is turn i+1 (see below)
    UserTurns: nil,          // optional: UserTurns[i] is the user message that produced Turns[i].Output
    Input:     caseInput,    // optional: the case input shown to judge checks
    Judge:     judgeFn,      // optional: checks.JudgeFunc that answers judge checks
})
```

`Input`, `UserTurns` and `Judge` are only used by `judge` checks. `Judge` is a `checks.JudgeFunc`, `func(JudgeQuery) (JudgeVerdict, error)`: it receives the criterion, question, input, output, the loaded `Files` (`Path`, `Content`, `Truncated`, `Size`) and, for a multi-turn case, the `Turn` being judged and the `Transcript`, and returns `JudgeVerdict{Yes, Reasoning}`. Return an error for a failed call; it fails the check as `judge call failed: <error>`. (An error wrapping `checks.ErrJudgeParse` is reported as `judge parse error` instead.) A `judge` check evaluated with `Judge == nil` **fails** with `no judge configured`; it is never skipped and never passes silently, so callers that use no `judge` checks are unaffected. In a normal eval run, `scoreCase` builds the `JudgeFunc`: it calls the configured backend on the pinned judge model and records the call and its cost. Callers outside the harness can build one with `checks.InferenceJudge{Backend, Model, Stub, Debug, OnCall}.Func()`, which sends the yes/no prompt (`checks.BuildYesNoPrompt`) to an `inference.Backend` with a 2-minute timeout and parses the reply (`checks.ParseYesNo`); `kairon check` does exactly that.

`checks.Evaluate` never reads the config, the case or the rubric, and returns one `checks.Result` per check in list order. A hand-built check is validated lazily: an invalid one yields a failed result with an explanation, never a panic and never a silent pass. `checks.Validate(file, list, checks.ValidateOptions{HiddenDir, Criterion, OptionalCriterion})` validates and compiles checks loaded from a file: `HiddenDir` is where `inject` sources are resolved (empty means `inject` is not allowed), `Criterion` is an optional callback that rejects a criterion (the eval harness passes its rubric cross-check) and `OptionalCriterion` lets a check omit `criterion`. Its errors wrap `checks.ErrInvalid` and read `invalid checks: <file>: check #<n> (<type>): <reason>`. `checks.LoadFile(path)` reads a standalone checks file (see [Checks file format](#checks-file-format)). In a normal eval run `Base` is the fixture commit of the case workspace, so a change the agent committed is still detected; when `Base` is empty `changed_files` diffs against `HEAD`.

`Turns` is a `[]checks.TurnEvidence{Output, GHLog, GHLogOversized}`, one entry per turn, and is what `turn` checks read. When `Turns` is empty (a single-turn case) every `output_*` / `gh_log_*` check reads `Output` / `GHLog` exactly as above. When it is set, a check that names a `turn` reads `Turns[turn-1]` and a check without `turn` reads the last entry; the harness fills `Turns` for `turns` cases only.

`UserTurns` pairs the user messages with `Turns`: `UserTurns[i]` is the message that produced `Turns[i].Output`. For a simulated-user case the harness passes the messages actually sent. A `judge` check reads both to build its conversation. A message missing from `UserTurns` is rendered empty rather than failing the check.

A `JudgeQuery` is `{Criterion, Question, Input, Output, Files, Turn, Transcript}`:

```go
type JudgeTurn struct {
    User  string // the user message actually sent in this turn
    Agent string // the agent's reply to it
}
```

- `Turn` is the 1-based turn being judged and `Transcript` holds turns 1 to `Turn` in order (`Transcript[i]` is turn i+1). In that form `Output` is the reply of `Turn`, not the case's final output.
- When `len(Turns) >= 2`, `Turn` is the check's `turn` (default: `len(Turns)`). A `turn` outside `1..len(Turns)` fails the check with `turn N is out of range: the case has M turn(s)` before `Judge` is called and before any file is read.
- When `Turns` has fewer than two entries (a classic case, a one-element `turns` list, or a hand-built input with no per-turn evidence) `Turn` is 0 and `Transcript` is empty; `Input` and `Output` are filled as before, and the single-turn prompt is unchanged. The case then counts as one turn: `turn: 1` is accepted and `turn: 2` fails with `the case has 1 turn(s)`.

### Running checks on any directory (`kairon check`)

`kairon check` runs a checks file against a real directory with no eval case, rubric, workspace or agent involved. Use it as a guardrail in a script or a workflow step: "did this directory end up the way I expect?".

```bash
kairon check --checks <file> --dir <dir> [--output <file>] [--gh-log <file>] [--hidden-dir <dir>] [--backend <name>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--checks` | *required* | YAML file holding the list of checks to run (see [Checks file format](#checks-file-format)). |
| `--dir` | *required* | Directory the `file_*`, `changed_files`, `command`, `judge` (`files`) and `fact_*` checks look at. It must be an existing directory. |
| `--output` | none | File whose text `output_contains` / `output_not_contains` (and the text a `judge` check is shown as the output) read. Omitted means empty text. |
| `--gh-log` | none | File whose text `gh_log_contains` / `gh_log_not_contains` read. Capped at 10 MiB: a larger file sets the same oversized flag as in eval, so `gh_log_*` checks fail explicitly. Omitted means empty text. |
| `--hidden-dir` | none | Directory that `inject` entries are resolved against and copied from. Without it, a check that uses `inject` is invalid. |
| `--backend` | `kiro-cli` | Inference backend that answers `judge` checks. Valid values are the same as for `kairon eval --backend`: `kiro-cli`, `stub`. An unknown value fails before any check runs. |

`--checks` and `--dir` are required (omitting one is a usage error), and the command takes no positional arguments.

#### Checks file format

A checks file is a **top-level YAML list** of the check mappings documented in [Checks](#checks): the same entries as the value of a case's `checks:` key, without the `checks:` key itself. The fields and the per-type rules are the same as in a case, with two differences: `criterion` is optional (there is no rubric to name), and `turn` has nothing to refer to (a checks file is evaluated as a single-turn case, so `turn: 1` is accepted and a higher value fails the check with `the case has 1 turn(s)`; leave it out). A misspelled key is rejected, as in case files, and a file with no checks is an error (`no checks in <file>`), so a guardrail that checks nothing can never pass. The file must be a regular file of at most 10 MiB.

```yaml
- type: file_exists
  path: hello.txt
- type: file_contains
  path: hello.txt
  pattern: 'hello'
- type: file_absent
  path: missing.txt
- type: command
  run: "grep -q hello hello.txt"
```

The command ships fixtures in `internal/eval/testdata/check-cli/` (`dir/` is the directory under test) that the examples below use.

#### Output and exit status

The whole file is validated before any check runs. Then the checks are evaluated against `--dir` as described in [Evaluation order](#evaluation-order) (non-`command` checks first, then `command` checks), and one line per check is printed to stdout **in the order they are written**, with whitespace collapsed to a single line:

```
PASS #1 file_exists path=hello.txt
FAIL #2 file_exists path=missing.txt: file does not exist
```

- `PASS <label>` or `FAIL <label>: <detail>`. The label is the same `#<n> <type> <key fields>` string used in eval results, so a failing check is named by its 1-based position, type and key field. A passing `judge` check appends the judge's reasoning as its detail.
- When every check passes, exactly one `PASS` line per check is printed and nothing else.
- When a check fails, the `FAIL` lines are followed by `Error: <n> of <m> checks failed` on stderr. Usage text is not printed.

| Exit status | Meaning |
|-------------|---------|
| `0` | every check passed |
| `1` | at least one check failed, **or** any other error: a missing required flag, an invalid checks file (before any check runs), an unreadable or empty checks file, an unknown `--backend`, a `--dir` that is not an existing directory, an unreadable `--output`, `--gh-log` or `--hidden-dir`, or a refused judge model |

An invalid file is reported with the same text as an invalid eval case, naming the file, the 1-based check number and the type, and **no check runs**. This matters for `command` checks: an earlier, valid `command` check is not executed when a later check in the file is invalid.

```bash
# All four checks pass: four PASS lines, exit 0
kairon check --checks internal/eval/testdata/check-cli/pass.yaml --dir internal/eval/testdata/check-cli/dir

# Check #2 fails: prints "FAIL #2 file_exists path=missing.txt: …", then "Error: 1 of 2 checks failed", exit 1
kairon check --checks internal/eval/testdata/check-cli/fail.yaml --dir internal/eval/testdata/check-cli/dir

# Check #2 is invalid: "invalid checks: …/invalid.yaml: check #2 (file_contains): pattern is required", exit 1, no check ran
kairon check --checks internal/eval/testdata/check-cli/invalid.yaml --dir internal/eval/testdata/check-cli/dir
```

#### Evidence flags: `--output`, `--gh-log`, `--hidden-dir`

`--output`, `--gh-log` and `--hidden-dir` supply the evidence that, in an eval run, the harness would collect itself. The text of the files is used verbatim.

```bash
# output_contains reads output.txt, gh_log_contains reads gh.log, and the command check injects hidden/injected.txt
kairon check --checks internal/eval/testdata/check-cli/evidence.yaml \
  --dir internal/eval/testdata/check-cli/dir \
  --output internal/eval/testdata/check-cli/output.txt \
  --gh-log internal/eval/testdata/check-cli/gh.log \
  --hidden-dir internal/eval/testdata/check-cli/hidden
```

- Without `--hidden-dir`, `evidence.yaml` is invalid because its `command` check uses `inject`.
- Without `--output`, an `output_contains` check fails (the output is empty text), and likewise `gh_log_contains` without `--gh-log`. `output_not_contains` and `gh_log_not_contains` pass vacuously on empty text, exactly as in a native eval run.
- `inject` works as in eval (see [Hidden files and `inject`](#hidden-files-and-inject)): each entry is copied from `--hidden-dir` into `--dir` immediately before the `command` runs and the directory is restored right after.

#### `changed_files` base

`changed_files` diffs `--dir` against the `HEAD` of the git repository at `--dir`, because `kairon check` leaves `Base` empty. `--dir` must therefore be the repository root, and a change that is already committed to `HEAD` is not seen. There is no eval fixture commit here. Added, modified, deleted and untracked paths all count, and paths that git ignores are invisible, as described in [How `changed_files` sees changes](#how-changed_files-sees-changes). If `--dir` is not a git repository the check fails with the git error.

#### Judge checks, `--backend` and model pinning

A `judge` check asks the judge model a yes/no question (see [The `judge` check](#the-judge-check)). In `kairon check` it behaves like the check in an eval run, with these specifics:

- **Model pinning.** The judge model is pinned exactly as in `kairon eval`: `evals.judge_model` from `./.kairon/config.yaml` (the built-in default when the file or the key is missing), validated against `evals.allowed_models` (see [Model Pinning and Run Provenance](#model-pinning-and-run-provenance)). It is passed to the backend as the request's model, and the call runs on the host with the 2-minute judge timeout. A model that is empty, `auto` or not in the allowlist **refuses the run before any check starts**, with a message such as:

  ```
  ❌ eval model pinning refused the run before any check started:
    - evals.judge_model: <reason>
  Edit the evals block in .kairon/config.yaml (judge_model, allowed_models)
  ```

- **Only when needed.** The config is read, the model is checked and the backend is used only if the file contains a `judge` check. A file without one never reads `.kairon/config.yaml`, so a broken config does not matter, and it does not need the `kiro-cli` binary even though `--backend` defaults to `kiro-cli`.
- **Fail closed.** A `judge` check is never skipped and never passes by default. With a real backend, a failed call (`judge call failed: …`) or an unparseable answer (`judge parse error: …`) fails the check, as in eval.
- **`--backend stub` is a test double.** The stub backend makes **no model call**: `kairon check` supplies it with a built-in script that answers `yes` to every `judge` check, so each one passes with the reasoning `stub judge: yes`. It exists to exercise the command (and scripts around it) without a model, like `task eval:selftest`; it says nothing about the directory, so do not use it as a guardrail. It is selected only by `--backend stub`; `kiro-cli` always asks the real model. Unlike an eval case, you cannot script other answers: there is no case to carry a `stub.judge` list.

```bash
# No model call: the stub answers "yes", so this prints a PASS line and exits 0
kairon check --backend stub --checks internal/eval/testdata/check-cli/judge.yaml --dir internal/eval/testdata/check-cli/dir

# Real judge: asks the model pinned by evals.judge_model (requires an authenticated kiro-cli)
kairon check --checks internal/eval/testdata/check-cli/judge.yaml --dir internal/eval/testdata/check-cli/dir
```

#### Safety

> **Warning: `command` checks run on the host, in `--dir`.**
> `kairon check` runs each `command` check with `sh -c` in `--dir` and your privileges, as described in [Command checks run on the host](#command-checks-run-on-the-host): stdin closed, GitHub credential variables (`GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_HOST`) removed from the environment, its own process group killed after 5 minutes. None of this is a sandbox, and there is no `--sandbox` for this command. Treat a checks file like a script: only run files you trust, against directories you are prepared to run code from. File-based checks (`file_*`, `changed_files`) and the `output_*` / `gh_log_*` checks only read.

`kairon check` opens no network listener and has no authentication surface of its own. Its only network use is the judge call, made by the selected `--backend` and only when the file has a `judge` check. The `judge` prompt includes the contents of the files listed in `files` and the `--output` text, so do not point a real backend at secrets you would not send to the model.

## Multi-turn cases

Some agent behaviour depends on the user's answers: the planner, for example, is a gated conversation (draft, approval, label confirmation, creation). A case with `turns` scripts that conversation: the harness sends one user message per entry, in order, and the agent's replies are never fed back by the harness. Turns are scripted, not simulated. To answer the agent's questions from a bank of facts instead, see [Simulated user (elicitation cases)](#simulated-user-elicitation-cases).

```yaml
name: planner-gates
agent: planner
setup:                     # delivered with turn 1 ONLY
  - type: text
    label: "Context"
    content: "The project exports reports."
turns:                     # replaces `input`
  - "I want a feature that exports reports as CSV"
  - "Looks good, go on"
  - "Yes, those labels are right"
stub:
  turns:                   # exactly one entry per turn, same order (stub backend only)
    - response: "…draft…"
    - response: "…label confirmation…"
    - commands: ["echo 'gh issue create --title t' >> .eval/gh.log"]
      response: "…created…"
checks:
  - criterion: structural_completeness
    type: gh_log_not_contains
    pattern: 'issue create'
    turn: 1
  - criterion: structural_completeness
    type: gh_log_contains
    pattern: 'issue create'
    turn: 3
```

What a `turns` case does:

- **One agent call per turn**, all against the **same workspace** (natively the same host directory, under `--sandbox` the same bind mounts). A file written in turn 1 is there in turn 2, and a stub turn's `commands` see the files of earlier turns.
- **The prompt of a turn is only that turn's message.** Turn 1 carries the `setup` context plus `turns[0]`; turns 2..n are sent verbatim. The harness never re-sends earlier turns or earlier answers and never passes a transcript or a "resume" flag: the backend owns conversation continuity (see [Backend contract](#backend-contract-for-turns)).
- **`stub.turns` has exactly one entry per turn**, in the same order. For a `turns` case with a `stub`, a different number of entries is a load error naming the case and both counts. A `turns` case with no `stub` is fine for the `kiro-cli` backend.
- **`timeout` is per turn**, not for the case: every turn gets the full case (or sandbox) timeout.
- **Checks** run once at the end, and `output_*` / `gh_log_*` checks may name the `turn` they apply to (see [The `turn` field](#the-turn-field)). A check without `turn` applies to the last turn.
- **A single-turn case is unchanged.** `input` cases behave as before, and their result files have no `turn_outputs` and no `calls[].turn`. A `turns` case with one entry is legal and is recorded like any other `turns` case.

### Turn numbering

There are two conventions, and the harness converts between them in exactly one place (where the agent request is built):

| Where | Numbering | Example |
|-------|-----------|---------|
| Case YAML (`checks[].turn`), results (`calls[].turn`, position in `turn_outputs`), printed `turn k/n`, error messages | **1-based** | the first user message is turn 1 |
| `inference.Request.Turn` and the index into `stub.turns[]` | **0-based** | the first user message is `Turn == 0` |

So `turn: 3` in a check, the third entry of `turns`, the third entry of `stub.turns` and `Request.Turn == 2` all describe the same turn.

### Failure semantics

If turn *k* of *n* fails (a backend error, a timeout, a tool-trust resolution failure or a non-zero exit):

- turns *k+1* … *n* are **not run**;
- `actual_output` is `""`, so the usual "agent produced no output; checks not run" rule applies: every criterion that has checks records `0/<total>` and no check runs (see [When the agent failed](#when-the-agent-failed));
- `turn_outputs` holds the outputs of turns 1 … *k−1*, which is useful evidence of how far the conversation got;
- `calls[]` holds an agent record for turns 1 … *k*, the failed one carrying its `error`, and `error_context` is the failing turn's;
- the printed error is prefixed with the turn: `Error: turn 2/3: …`.

`agent_cost` is the sum of the cost of every agent call that was made, including the failed one (see [Result JSON](#result-json-for-multi-turn-cases)).

### Judge input

The judge prompt template is unchanged. For a `turns` case the `INPUT` section is rendered from the scripted user messages, one labelled block per turn, and `ACTUAL OUTPUT` is the **last** turn's output:

```
Turn 1 (user): I want a feature that exports reports as CSV

Turn 2 (user): Looks good, go on

Turn 3 (user): Yes, those labels are right
```

A single-turn case still passes its `input` verbatim.

This section describes the **legacy 1-5 judge**, which scores criteria that have no `checks`. It is unchanged: it sees the user messages and the final output only, not the agent's earlier replies. A [`judge` check](#the-judge-check) is different: in a multi-turn case it is shown the full transcript, every user message and every agent reply up to the judged turn, each fenced and labelled by turn, and it can judge any turn with `turn`. To assert on an earlier turn's output without a model, use `output_*` checks with `turn`.

### Result JSON for multi-turn cases

A `turns` case adds two things to the result file (a single-turn case's JSON shape does not change):

- `turn_outputs` on the case: the agent's output for every turn, in order. `actual_output` is the last entry.
- `turn` on each agent record in `calls[]`: the 1-based turn of that call. Judge records carry no `turn`.

```json
{
  "case_name": "multi-turn-gh-log",
  "actual_output": "## Created\n\nTURN-THREE-MARKER: …",
  "turn_outputs": [
    "## Draft\n\nTURN-ONE-MARKER: …",
    "## Labels\n\nTURN-TWO-MARKER: …",
    "## Created\n\nTURN-THREE-MARKER: …"
  ],
  "calls": [
    { "role": "agent", "agent": "selftest", "turn": 1, "model": "stub-model", "…": "…" },
    { "role": "agent", "agent": "selftest", "turn": 2, "model": "stub-model", "…": "…" },
    { "role": "agent", "agent": "selftest", "turn": 3, "model": "stub-model", "…": "…" }
  ]
}
```

`agent_cost` is the sum of the per-turn costs; its `usage_source` is `reported` only if every turn's usage was `reported`, otherwise `estimated`. Older result files without these fields still load.

### Conversation continuity

**Native `kiro-cli`.** Turn 1 runs `kiro-cli chat --agent <agent> --no-interactive …` exactly as a single-turn case does. Turns 2..n add `--resume`, directly after `--no-interactive`, in the same workspace directory: `kiro-cli chat --agent <agent> --no-interactive --resume --trust-all-tools [--model <model>]`. `--resume` continues the most recent conversation in the working directory, and the workspace is unique per case, so that conversation is this case's. Each call receives only that turn's message on stdin. Two consequences: native runs write their conversations into your real `kiro-cli` conversation store (one per case workspace directory), and turns 2..n pass the same `--agent`, trust and `--model` arguments as turn 1.

**`--sandbox`: one container per case.** A single-turn case gets a fresh container for its one agent call. A `turns` case gets **one container for all its turns**. `kiro-cli` keeps its conversation state under `$HOME`, which in the container is a tmpfs (see [Read-only root filesystem](#read-only-root-filesystem)); a fresh container per turn would lose it and `--resume` could not work. The container is created and started when the first turn runs (so a turn that fails before reaching it, such as a tool-trust resolution failure, creates none), `kiro-cli` is validated once, every turn then runs in it, and it is removed after the last turn (or, in `--debug` mode, preserved if the case failed). The timeout, tool-trust set and `WorkDir` are applied to each turn exactly as for a single call. A case with a single turn, and every `input` case, keeps the one-shot container path.

**Stub.** The stub backend is stateless: it answers with `stub.turns[Request.Turn]`, so it needs nothing from the container beyond the transport.

### Backend contract for turns

The harness is the only thing that knows the script; the backend owns the conversation. `inference.Request.Turn` is the **0-based position of this user message in the conversation**, and:

- `Prompt` carries **only that turn's user message** (turn 1 additionally carries the setup context). It never carries earlier turns or earlier answers.
- The harness passes no `Resume` flag and no transcript. A backend must continue the conversation itself when `Turn > 0`, and decides how.
- `Turn == 0` (the zero value, and what every request that does not set it carries) starts a new conversation.

How each backend honours it:

| Backend | `Turn > 0` |
|---------|------------|
| `kiro-cli` | adds `--resume` (continues the previous conversation in the same working directory) |
| `stub` | answers with `Stub.Turns[Turn]` |
| a future direct-API backend | keeps a message list keyed by the conversation (the per-case `WorkDir`) and appends each prompt and reply; `Turn == 0` starts a new list |

This is why `Request` has no `Resume bool`: "resume" is a `kiro-cli` mechanism, "turn" is the harness concept. A direct-API backend needs no change to the harness or to case YAML, because `turns` and `turn` are backend-neutral.

**Known limit under `--sandbox`.** Non-`kiro-cli` backends run in the container as a fresh `kairon inference-exec` process per call (see [Backends in the Container](#backends-in-the-container)), so a future backend that holds the conversation **in memory** cannot keep that history inside the container across turns. It would have to run on the host, as the judge does. The stub is not affected because it is index-based and stateless. Keeping one container per case is what lets `kiro-cli`'s on-disk state survive; it does not give an in-process backend a long-lived process.

### Verifying `--resume` against a real `kiro-cli`

Conversation continuity with a real model cannot run in CI (it needs an authenticated `kiro-cli`). Automated tests cover the arguments (`--resume` only for `Turn > 0`, directly after `--no-interactive`) and that each call receives only its own message, using a fake `kiro-cli`. Verify the real behaviour by hand:

Step 1: copy the evals directory.

```bash
cp -r .kairon/evals /tmp/evals317
```

Step 2: add a case that asks the agent to remember a word in turn 1 and repeat it in turn 2.

```yaml
# /tmp/evals317/cases/planner/remember-word.yaml
name: remember-word
description: "Manual check that turn 2 continues the conversation of turn 1"
agent: planner
turns:
  - "Remember the word PINEAPPLE. Reply only OK."
  - "What was the word? Reply with the word only."
checks:
  - criterion: requirement_clarity   # any non-cost criterion of the agent's rubric
    type: output_contains
    pattern: 'PINEAPPLE'
    turn: 2
```

Step 3: run it natively and under the sandbox.

```bash
kairon eval --evals-dir /tmp/evals317 planner remember-word
kairon eval --evals-dir /tmp/evals317 --sandbox planner remember-word
```

Step 4: in both runs, `turn_outputs[1]` in the result file must contain `PINEAPPLE` and the check must pass.

If it does not, the `--resume` call is not reaching the same conversation. The `--resume` position and its combination with `--agent <name>` (which the harness always passes) is the thing to adjust in `KiroCLIAgentCommand`; turn 1 and every single-turn case must stay unchanged.

## Simulated user (elicitation cases)

Some agents, the planner above all, must draw out every requirement the user holds without filling the gaps with assumptions. Scripted [`turns`](#multi-turn-cases) cannot measure that: the agent's questions vary from run to run, so fixed replies stop matching them. A case with `simulated_user` answers **whatever the agent asks** from a fixed **bank of facts**, and the [`fact_asked` / `fact_not_asked`](#the-thirteen-check-types) checks then say which facts the agent asked for.

```yaml
name: csv-export-elicitation
agent: planner
input: "I want a feature that exports reports as CSV"   # the opening user message (required)
simulated_user:
  facts:
    - id: format
      answer: "Comma separated, UTF-8, one header row."
      discoverable: false              # the fixture code does NOT answer this
    - id: schedule
      answer: "Reports are exported weekly, every Monday."
      discoverable: false
    - id: language
      answer: "The service is written in Go."
      discoverable: true               # the fixture code answers this; asking is unnecessary
  default_reply: "I don't know, use your best judgement."
  stop_when: 'ISSUE-DRAFT-READY'       # regex on the agent's output (optional)
  max_turns: 8                         # agent turns in the simulated phase (default 10)
turns:                                 # optional scripted turns, sent AFTER the simulated phase
  - "Looks good, create it."
stub:                                  # stub backend only
  routes: [format, none]               # one routing decision per simulated reply
  turns:
    - response: "Which format should the export use?"
    - response: "How large are the reports?"
    - response: "ISSUE-DRAFT-READY: here is the draft."
    - response: "Created."
checks:
  - criterion: requirement_clarity
    type: fact_asked
    fact: format
  - criterion: requirement_clarity
    type: fact_not_asked
    fact: language
  - criterion: requirement_clarity
    type: output_contains
    pattern: 'ISSUE-DRAFT-READY'
    turn: 3
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `input` | yes | The opening user message. A `simulated_user` case needs a non-blank `input`, and unlike other cases it may be combined with `turns`. |
| `simulated_user.facts[]` | yes (at least one) | The answer bank. |
| `facts[].id` | yes | Identifier the checks and `stub.routes` refer to. Non-blank, no whitespace, unique within the case, and not `none` (any case): `none` is the "no fact matches" value. |
| `facts[].answer` | yes | The text sent to the agent, **verbatim** (no trimming, no rewriting), when the agent asks about this fact. Must not be blank. |
| `facts[].discoverable` | yes | `true` or `false`; omitting it is a load error. `true` means the fixture code could answer the question itself, so the agent should not have needed to ask. It is recorded in the [sim log](#the-sim-log) next to each decision for later metrics; nothing scores it yet. |
| `simulated_user.default_reply` | yes | Sent when the agent's output asks about no fact. Must not be blank. |
| `simulated_user.stop_when` | no | An RE2 regular expression. A simulated-phase turn whose output matches ends the phase. Case-sensitive (use `(?i)` otherwise). An invalid expression is a load error. |
| `simulated_user.max_turns` | no (default `10`) | The most agent turns the simulated phase runs. Must be `>= 1`; an explicit `0` is a load error. |
| `turns` | no | Scripted user messages sent after the simulated phase, one per agent turn. Entries must be non-blank and `turns: []` is still rejected. |

### How the loop runs

After agent turn *k* of the simulated phase:

1. If `stop_when` matches that turn's output, the phase ends. Nothing is routed and no reply is sent.
2. Otherwise, if *k* equals `max_turns`, the phase ends the same way. The last allowed turn is never answered.
3. Otherwise the output is [routed](#routing) and the chosen fact's `answer`, or `default_reply`, is sent as the next user message.

When the phase ends, the scripted `turns` follow, one per agent turn, and the case ends after the last one. A case without `turns` ends when the phase ends.

- `max_turns` counts the agent turns of the simulated phase only; scripted turns do not count against it.
- Turn numbers are **global and 1-based**: `turn_outputs`, `calls[].turn`, a check's `turn` and the sim log all number the agent turns from 1 across the simulated phase and then the scripted turns. In the example, turn 3 is the turn whose output matched `stop_when` and turn 4 is the scripted one.
- `stop_when` is only evaluated on simulated-phase turns, never on scripted ones.
- `setup` is delivered with the first message only; replies and scripted turns are sent verbatim. Each message is passed to the backend as `Request.Prompt` alone, with `Request.Turn = k-1`, exactly as for a `turns` case (see [Backend contract for turns](#backend-contract-for-turns)). A reply is just the next user turn, so a future direct-API backend needs nothing new to support simulated cases.
- Under `--sandbox` a simulated case keeps **one container** for all its turns, like a `turns` case (see [Conversation continuity](#conversation-continuity)).
- Progress output prints `turn k` (no `/n`, because the total is not known in advance).

### Routing

After each non-final simulated turn the harness makes **one routing call** to the judge: the model pinned for judging, a 2-minute timeout, always on the host (also under `--sandbox`, like every judge call). The prompt lists every fact as `id` plus `answer`, then the agent's output between `===BEGIN AGENT OUTPUT===` and `===END AGENT OUTPUT===`, declared untrusted data and limited to its last 8 KiB because questions come last. It asks which single fact the agent is asking about (the first answerable question wins when there are several) and expects:

```
===JSON_START===
{"reasoning": "…", "fact": "<id>" | "none"}
===JSON_END===
```

- **The reply text never comes from the model.** The model returns an id and the harness looks the answer up in the bank, so an instruction injected into the agent's output can at most choose *which* bank fact is returned.
- Only an id from the case's `facts` or `none` is accepted (ids exactly, `none` in any case). An unknown id, a missing `fact` key, a non-string value or bad JSON is a parse error.
- **Routing fails closed.** A routing call that fails or cannot be parsed ends the case like a failed agent turn: the remaining turns do not run, `actual_output` is `""` (so criteria with checks record `0/<total>`, see [When the agent failed](#when-the-agent-failed)), `turn_outputs` keeps the outputs recorded so far, and the error is printed and recorded in `error_context.stderr` as `turn k: simulated user: <reason>`. `default_reply` is never used as a fallback, because that would hide a broken judge as "the agent asked nothing relevant".
- `--debug` prints the routing prompt like the other judge prompts.
- Every routing call costs one judge call; `max_turns` bounds it. It is recorded in `calls[]` as a judge record with `purpose: simulated_user` (and no `criterion`). Its cost is added to `judge_cost` only when the call succeeded and parsed; a failed call is in `calls[]` only, like any other judge call (see [Per-call records](#per-call-records-calls)).

### Results

A simulated case records the same per-turn evidence as a `turns` case (`turn_outputs`, `calls[].turn`) and adds `user_turns`: every user message actually sent, in order (the opening message, each reply, each scripted turn). `user_turns` is set only for simulated cases, so the result JSON of classic and `turns` cases does not change.

```json
{
  "case_name": "sim-routes",
  "user_turns": [
    "I want a feature that exports reports as CSV",
    "SIM-F1-ANSWER: comma separated, UTF-8, one header row.",
    "SIM-DEFAULT-REPLY: I don't know, use your best judgement."
  ],
  "calls": [
    { "role": "agent", "agent": "selftest", "turn": 1, "…": "…" },
    { "role": "judge", "purpose": "simulated_user", "…": "…" },
    { "role": "agent", "agent": "selftest", "turn": 2, "…": "…" }
  ]
}
```

`calls[]` stays in execution order: agent turn 1, its routing call, agent turn 2, and so on.

For the legacy 1-5 judge, the `INPUT` of a simulated case is rendered from `user_turns` as `Turn N (user): …` blocks (see [Judge input](#judge-input)), so it sees the user side of the conversation that actually happened rather than only the opening message. A [`judge` check](#the-judge-check) goes further: it is shown the messages actually sent (`user_turns`) together with the agent's reply to each, in order, and can pick a turn with `turn`. If the simulated phase ends early (`stop_when`), a `turn` beyond the turns that ran fails at run time naming the number that did.

### The sim log

Each routing decision is appended to `.eval/sim-log.jsonl` in the case workspace, one JSON object per line:

```json
{"turn":1,"question":"…last 200 characters of the agent output…","fact":"format","discoverable":false}
{"turn":2,"question":"…","fact":"none"}
```

| Field | Description |
|-------|-------------|
| `turn` | The 1-based agent turn whose output was routed. |
| `question` | The last 200 characters of that output with whitespace runs collapsed to one space, so a human can audit a misroute. |
| `fact` | The routed fact id, or `none`. |
| `discoverable` | The routed fact's flag. Omitted for `none`. |

The reply text is not logged: it is a function of `fact` (see `user_turns`). The file is created empty when the case starts, so "no decision yet" differs from "no log".

`.eval/` is writable by the agent, so **the log is rewritten by the harness from its in-memory record of the decisions** after every decision and again once more after the last turn (after any container is closed, just before scoring). Anything an agent wrote or truncated there is replaced, and `fact_asked` / `fact_not_asked` read what the harness decided. Run with `--keep-workspaces` to read the file (see [Keeping workspaces](#keeping-workspaces-and-workspace_dir)).

### Fact checks

`fact_asked: <id>` passes when at least one decision has that fact; `fact_not_asked: <id>` passes when none has. They require a `simulated_user` case and a `fact` that names one of its facts (see [Checks](#the-thirteen-check-types)). A missing, unreadable or malformed log fails the check with a detail, for `fact_not_asked` too; it is never a silent pass.

### Stub backend: `stub.routes`

The stub backend makes the harness deterministic with no model calls (no `kiro-cli`, no network). `stub.routes` scripts the routing decisions: entry *N* answers the *N*th routing decision (0-based) of the case, and each entry is a fact `id` or `none`. It is **index-based, not a cursor**: the harness passes the decision number and the stub looks it up, like `Request.Turn` and `stub.turns`, so a rerun, `--resume` or parallel cases cannot desynchronise it. A decision with no entry fails the case closed with `case has no stub.routes[N]` rather than cycling. `stub.routes` is only valid with `simulated_user` (see [Stub Case Fields](#stub-case-fields)).

### Load errors

Each of these is a load error that names the case (`case "<name>" (<file>): …`):

- `simulated_user` without a non-blank `input`; `turns: []` or a blank `turns` entry.
- `facts` empty; a fact `id` that is blank, contains whitespace, is `none`, or is duplicated; a blank `answer`; a missing `discoverable`.
- A blank `default_reply`; a `stop_when` that is not a valid regular expression; a `max_turns` below `1`.
- `stub.turns` with fewer than `1 + len(turns)` or more than `max_turns + len(turns)` entries: the simulated phase runs between 1 and `max_turns` agent turns, then the scripted turns, so fewer could never complete and more could never be used. (A run that needs more entries than declared still fails with the stub's own `no stub.turns[k]` error.)
- `stub.routes` without `simulated_user`; an entry that is neither a fact id nor `none`; more than `max_turns - 1` entries (a route is consumed only when the simulated user replies).
- `fact_asked` / `fact_not_asked` on a case without `simulated_user`, or naming a fact that is not defined.
- A check `turn` beyond `max_turns + len(turns)`. The real number of turns is only known after the run, so this is the static bound; a `turn` beyond the turns that actually ran fails at run time with a clear detail, as in a `turns` case.
- A case with `turns` **and** `input` but no `simulated_user` is still rejected (`turns and input are mutually exclusive`).

### Notes

- **Routing quality depends on the judge model.** Real runs are nondeterministic by nature, which is why `fact_asked` is a per-fact boolean over a log that keeps the question excerpt. The stub makes the harness itself deterministic.
- **`discoverable` is recorded, not scored.** A later check can compute "asked but discoverable" from the log.
- **For backend authors.** `Request.Route` and `Request.RouteSeq` exist only so the stub can answer from `stub.routes`; a real backend treats the routing request as the ordinary judge prompt it is (see [Adding a New Backend](#adding-a-new-backend)).
- Not covered: free-form generated user replies (answers always come verbatim from the bank).

## Running Evaluations

```bash
# Evaluate all agents
kairon eval

# Evaluate a specific agent
kairon eval architect

# Compare two runs (names of directories under results/)
kairon eval diff <runA> <runB>

# Choose the inference backend (default: kiro-cli)
kairon eval --backend kiro-cli architect
kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest

# Use a different evals directory
kairon eval --evals-dir path/to/evals architect

# Evaluate a candidate prompt without touching the live agent
kairon eval architect --prompt-file .kairon/iterations/architect/v2.md

# Run every case 5 times and report the mean score with its spread
kairon eval architect --repeat 5
```

| Flag | Default | Description |
|------|---------|-------------|
| `--backend` | `kiro-cli` | Inference backend for agent and judge calls. Valid values: `kiro-cli`, `stub`. An unknown value fails immediately and lists the valid backends. |
| `--evals-dir` | `.kairon/evals` | Directory holding `rubrics/`, `cases/`, `fixtures/`, optional `agents/`, and `results/`. Persistent flag, so `kairon eval diff` honours it too. |
| `--keep-workspaces` | off | Keep each case's workspace after the run instead of deleting it. The path is printed on the case line and recorded as `workspace_dir` in the results (see [Case Workspaces](#case-workspaces)). Combine with `--debug` to inspect both the preserved container and its workspace. |
| `--prompt-file` | none | Evaluate this file as the agent's prompt instead of the live one, in the per-case workspaces only. Requires an agent (`kairon eval <agent> --prompt-file <path>`); nothing under `.kiro/agents/` is modified. Local to `kairon eval` (not available on `eval diff`). See [Evaluating a candidate prompt](#evaluating-a-candidate-prompt---prompt-file). |
| `--repeat` | `1` | Run every case N times, each run in a fresh workspace, and keep every result. The agent `score` becomes the **mean** over the N runs, with `score_min`, `score_max` and `repeat_scores` showing the spread. A value below `1` is rejected (non-zero exit). Cannot be combined with `--list`, `--cleanup` or `--perf` (they run no cases). Combined with `--case` (or `<agent> <testcase>`) it repeats that one case. `--resume` requires the same value the interrupted run used. See [Repeat runs](#repeat-runs---repeat). |

## Adding Test Cases

1. Create a YAML file in `.kairon/evals/cases/<agent>/`
2. Provide an `input` field with representative agent input, or a `turns` list for a scripted multi-turn conversation (see [Multi-turn cases](#multi-turn-cases))
3. Optionally capture real agent output in the `output` field for offline evaluation

## How Scoring Works

- **Check-scored criteria** — a criterion that has [`checks`](#checks) in the case is scored `passed` out of `total` checks, with the failed checks named in the reasoning. Checks take precedence over everything below for that criterion in that case: the rubric's `scoring`, its `deterministic` flag and the legacy 1–5 LLM judge are not used, so no legacy judge call is made. Only a [`judge` check](#the-judge-check) in the list calls the judge model, once per check.
- **Deterministic criteria** — criteria without checks that the rubric marks `deterministic` are scored by the legacy heuristics (code checks on the agent's output, such as structural completeness)
- **LLM-judged criteria** — criteria without checks that are not deterministic are scored by an LLM evaluator using the rubric description (requires output and a configured judge)
- **Cost criteria** — tracked automatically from token usage

A criterion without checks is scored exactly as it was before checks existed, so adding checks to one criterion never changes how the others are scored. The exception is a rubric that sets [`checks_only`](#checks_only-rubrics).

### `checks_only` rubrics

A rubric with `checks_only: true` scores every case from its checks alone:

- **Unchecked criteria are not scored.** A non-cost criterion that a case has no checks for gets no entry in that case's scores (it is absent from `scores`, not recorded as `0` or skipped). It adds nothing to the case score, the case maximum, the agent score or the agent maximum, so it can neither raise nor lower a result.
- **No legacy scoring runs.** The legacy 1–5 LLM judge is never called and the keyword heuristic never runs for such a criterion, whether or not it is marked `deterministic`. The rubric's `scoring` and `deterministic` fields are ignored; a checked criterion's maximum is its number of checks, as before.
- **A criterion adds nothing to its maximum in a case it is not scored in.** Because an unchecked criterion has no entry in a case's scores, it contributes neither points nor maximum for that case. The criterion's pooled maximum in the [verdict's `criteria`](#agent-verdict) is therefore its number of checks across the cases that checked it, not a fixed figure per case. For example, with `file_written` checked twice in one case and once in another, and `report_marker` checked only in the second, `file_written` has a maximum of 3 and `report_marker` a maximum of 1.
- **`judge` checks remain allowed.** A [`judge` check](#the-judge-check) is a check, so it is scored `passed`/`total` like the others and still makes its one yes/no judge call. `checks_only` removes the legacy judge, not the `judge` check type.
- **A case without checks is rejected.** Every case of an agent whose rubric sets `checks_only` must have at least one check. A check-less case would have nothing to score, so it is a fatal load error that names the case file and wraps the same invalid-checks error as a malformed `checks` block, for example `…: no-checks.yaml: rubric for agent selftest-checks sets checks_only but the case has no checks`. A `kairon eval` run over several agents checks every `checks_only` agent up front, so the rejection happens **before any case runs**, including cases of agents that come earlier in the run. A single-case run and `--list` hit the same error when the cases load.
- **No output still scores zero.** A checked criterion is not rescued when the agent produced no output: it records `0/N` (not skipped) with the reasoning `agent produced no output; checks not run`, exactly as described in [When the agent failed](#when-the-agent-failed), and the run fails its threshold.
- **Other rubrics are unchanged.** A rubric without the field (or with `checks_only: false`) is scored exactly as before.

### Skipped Criteria

A criterion that cannot be measured is **skipped**: a non-deterministic criterion when no LLM judge is configured or the judge call fails, or any criterion when the agent produced no output. A skipped criterion scores `0` and its maximum stays in the denominator. It counts against the case and the agent exactly like a criterion the agent failed, so a missing or failing measurement can never raise a score.

The `skipped: true` flag is kept in the results as information: it records *why* the criterion scored 0, and does not change the arithmetic.

Skipped criteria appear in results as:
```json
{
  "name": "task_decomposition",
  "score": 0,
  "max_score": 5,
  "skipped": true,
  "reasoning": "LLM judge not configured — criterion skipped"
}
```

The counting rule, applied to every non-cost criterion of every case:

- `case score` = sum of `score`, `case max` = sum of `max_score` (skipped criteria contribute `0` and their `max_score`).
- `agent score` = sum of the case scores / sum of the case maxima.
- Cost criteria are tracked as cost and are not part of either sum.

For example, a case with one criterion scored `5/5` and one skipped criterion `0/5` totals `5/10` (50%), not `5/5`. A failed case in a printed run lists its skipped criteria as `0/N` in the below-threshold breakdown.

`kairon eval diff` is the one place that still leaves skipped criteria out: its per-criterion averages are unchanged so that historical runs diff exactly as before (see [Comparing Runs](#comparing-runs)).

Results are written to `<evals-dir>/results/<timestamp>-<git-hash>/` (default `.kairon/evals/results/...`) enabling before/after comparison when prompts change.

## Pass/Fail Verdict and Exit Status

`kairon eval` judges the scores as well as reporting them. Every evaluated agent gets a verdict, and the process exit status reflects it.

### Thresholds

Thresholds are percentages in `(0, 100]`. A case's threshold is resolved in this order, first match wins:

1. the case's `min_score`,
2. the rubric's `pass_threshold`,
3. the default, `95`.

The agent's threshold is always its rubric's `pass_threshold` (default `95`); a case's `min_score` affects only that case.

### Case verdict

A case passes when it produced output and `case score / case max * 100 >= threshold`. A case with no output cannot pass. The comparison is exact at the boundary: 19 out of 20 passes a threshold of 95. A case with a zero denominator never passes. Each case in `<agent>.json` records its `threshold` and `passed`.

### Agent verdict

An agent passes when its aggregate score is at least its threshold (and, if the rubric sets [`min_criterion_score`](#per-criterion-scores-and-the-floor), no criterion is below that floor):

```
score = sum of case scores / sum of case maxima * 100   (all non-cost criteria of all cases)
passed = score >= threshold
```

- Skipped, errored and no-output criteria count as 0 in the aggregate (see [Skipped Criteria](#skipped-criteria)).
- **With `--repeat`, `score` is the mean over the repeats.** The sums run over every run of every case; each repeat has the same cases and the same maxima, so this equals the arithmetic mean of the per-repeat scores. `passed` is decided on that mean: a single repeat above the threshold does not rescue a mean below it. The spread is reported next to it in `score_min`, `score_max` and `repeat_scores`, and the number of repeats in `repeat` (see [Repeat runs](#repeat-runs---repeat)). Without `--repeat` the run is one repeat and `score_min == score == score_max`.
- `cases_total` is the number of distinct cases and `cases_failed` the number that did not pass their own threshold. A case with several runs counts as failed when its score pooled over its runs is below its threshold. `cases_failed` is informational: it does not fail the agent by itself, the aggregate score does.
- **Attempts.** `attempts_total`, `attempts_passed` and `success_rate` report how often a whole attempt succeeded, which the pooled `score` hides. An attempt is one case result: every case run counts, so with `--repeat N` each case is N attempts. `attempts_total` is the number of case results, `attempts_passed` the number of those that passed their own threshold (the `passed` recorded on the case, recomputed for results written before `passed` existed; a result with no output is a failed attempt), and `success_rate = attempts_passed * 100 / attempts_total` as a **percent (0-100)**, `0` when there are no attempts. These differ from `cases_total` and `cases_failed`, which count distinct cases and pool the repeats of a case before judging it. After `--repeat 2` of an agent with 21 cases, `cases_total` is 21 while `attempts_total` is 42. The three fields do not depend on the agent's threshold or the criterion floor: an agent can pass at a score of 95% with a `success_rate` of 67%, and a floor failure leaves the counts unchanged. They are always written, so `success_rate: 0` (every attempt failed) is distinguishable from a missing field. `success_rate` is not rounded in `summary.json`; only the printed line rounds it to one decimal. For a single-case run the counts cover only the cases that ran. Trends over several runs: [Attempt trend](#attempt-trend-kairon-eval-trend).
- **Fail closed.** If nothing was scored (the denominator is 0, for example a rubric with only cost criteria), the agent gets `score`, `score_min` and `score_max` of 0, `passed: false`, and no `agent_scores` entry.

#### Per-criterion scores and the floor

The agent `score` pools points across every check of every case, so a criterion with few checks counts for little: one criterion at 0 out of 1 beside 19 passing checks still gives an agent 95%. To make that visible, every verdict carries a `criteria` array with one entry per criterion, and a rubric can turn it into a gate with [`min_criterion_score`](#rubric-format).

- **Per-criterion totals.** For each non-cost criterion the verdict pools `score` and `max_score` (points, not percent) over **every case and every repeat** of the agent, and reports `percent = score * 100 / max_score`. The criteria that are listed are summed to the same totals as the agent `score` (a skipped criterion counts `0` and keeps its maximum), so the sum of the listed `score`s and of the listed `max_score`s equals the numerator and denominator of `score`. `criteria` is always present and is `[]` when nothing was scored. Entries are in first-seen order, which for a normal rubric is rubric order. Cost criteria are never listed.
- **Unscored criteria are not listed.** A criterion that no case scored has nothing measured (a maximum of 0), so it has no entry and is never judged against the floor. This happens for a [`checks_only`](#checks_only-rubrics) criterion that none of the cases has checks for. A criterion scored in only some cases is listed, and its maximum counts only those cases (see [How Scoring Works](#how-scoring-works)).
- **The floor rule.** When the rubric sets `min_criterion_score`, the agent **fails if any listed criterion's pooled percent is below it**, even when the overall `score` meets the threshold:

  ```
  criteria_below_floor = criteria whose percent < min_criterion_score
  passed = score >= threshold && criteria_below_floor is empty
  ```

  The comparison is exact, with the same boundary behaviour as `pass_threshold`: a criterion exactly at the floor passes, and 49 out of 100 fails a floor of 50. `score`, `threshold`, `cases_total` and `cases_failed` are computed exactly as before; the floor only decides `passed`. A floor failure does not make any case fail by itself, so `cases_failed` can be `0` for a failing agent.
- **Repeats.** With `--repeat N` the floor is judged on the **pooled** percent, which is the mean over the repeats (the maximum is N times the single-run maximum and the percent equals the mean of the per-repeat percents). A criterion that is low in one repeat but high in the others is judged on its mean, not on its worst repeat.
- **Single-case runs.** `kairon eval <agent> <case>` (and `--case`) computes the verdict from the one case that ran, so the floor applies to that case's criteria. A criterion the case did not score is not listed and not judged.
- **No floor, no change.** Without `min_criterion_score` nothing is judged against a floor: `passed` is `score >= threshold` as before, and `criteria` is informational. With a floor set and nothing scored, the agent already fails closed and no criterion is named.

### Output and exit status

After an agent finishes, `kairon eval` prints one line, which starts with the verdict word (so `grep '^PASS \|^FAIL '` finds them):

```
PASS selftest: 100.0% (threshold 95.0%), 0/25 cases failed, success rate 100.0% (25/25 attempts)
FAIL selftest-fail: 9.7% (threshold 95.0%), 20/21 cases failed, success rate 4.8% (1/21 attempts)
```

The format is `PASS|FAIL <agent>: <score>% (threshold <threshold>%), <cases_failed>/<cases_total> cases failed, success rate <success_rate>% (<attempts_passed>/<attempts_total> attempts)`, with one decimal for the percentages. The success rate comes right after `cases failed`, in the same position on every line, so `success rate (\d+\.\d)%` matches it whatever follows; an agent with no attempts prints `success rate 0.0% (0/0 attempts)`. See [Agent verdict](#agent-verdict) for what an attempt is. With `--repeat N` (N > 1) `<score>` is the mean and the line ends with the spread, `[repeat N: min X%, max Y%]`; the line still starts with the verdict word and the text before the suffix is unchanged. Each repeat is a separate attempt, so the example below has two attempts for its one case:

```
FAIL selftest-fail: 50.0% (threshold 95.0%), 1/1 cases failed, success rate 50.0% (1/2 attempts) [repeat 2: min 0.0%, max 100.0%]
```

With no `--repeat` (or `--repeat 1`) there is no suffix and the line is exactly as above.

When the verdict has `criteria`, one indented line per criterion follows the verdict line, in `criteria` order: `  <criterion>: <score>/<max_score> (<percent>%)`, with one decimal for the percent. The lines start with spaces, not `PASS ` or `FAIL `, so `grep '^PASS \|^FAIL '` still finds only the verdict lines:

```
PASS selftest: 100.0% (threshold 95.0%), 0/25 cases failed, success rate 100.0% (25/25 attempts)
  structural_completeness: 66/66 (100.0%)
  clarity: 125/125 (100.0%)
```

When the rubric sets `min_criterion_score` and criteria are below it, the `FAIL` line gains a suffix naming each one with its percent and `score/max_score`, after the existing text (including the success rate) and **before** any `[repeat …]` suffix. One criterion reads `; criterion below floor <floor>%: <name> <percent>% (<score>/<max_score>)`; several use `criteria below floor` and are joined with `, ` in `criteria` order:

```
FAIL selftest-floor: 66.7% (threshold 60.0%), 0/1 cases failed, success rate 100.0% (1/1 attempts); criterion below floor 50.0%: report_marker 0.0% (0/1)
  file_written: 2/2 (100.0%)
  report_marker: 0/1 (0.0%)
FAIL demo: 80.4% (threshold 60.0%), 0/2 cases failed, success rate 100.0% (4/4 attempts); criteria below floor 50.0%: a 10.0% (2/20), b 0.0% (0/2) [repeat 2: min 80.4%, max 80.4%]
```

The overall score on those first lines met its threshold; the floor alone failed the agent. Without a floor failure the first line is unchanged. In the second example `--repeat 2` doubled the pooled score and maximum (`2/20` rather than `1/10`) while the percent is the mean over the repeats.

When the agent's cost is incomplete (a call used a model with no configured price), the line also ends with ` [cost incomplete: no price for <models>]`, listing the unpriced model ids, after any repeat suffix. The line still starts with the verdict word:

```
PASS selftest: 100.0% (threshold 95.0%), 0/25 cases failed, success rate 100.0% (25/25 attempts) [cost incomplete: no price for claude-haiku-4.5]
```

See [Unknown cost](#unknown-cost).

| Exit status | Meaning |
|-------------|---------|
| `0` | every evaluated agent passed |
| `1` | at least one evaluated agent is below its threshold, or any other error occurred |

When an agent fails, the results, `summary.json` and the performance report are still written first. Then `kairon eval` exits `1` after printing an error on stderr that names each failing agent:

```
Error: eval failed: 1 of 2 agents below their pass threshold: selftest-fail (4.0% < 95.0%)
```

An agent that failed only because of the floor is named by its criteria instead, so the error never prints a misleading `97.0% < 95.0%` when the overall score met the threshold:

```
Error: eval failed: 1 of 2 agents below their pass threshold: selftest-floor (criterion report_marker 0.0% < floor 50.0%)
```

A threshold-only failure is printed exactly as before. The exit status is `1` in both cases.

Usage text is not printed for this error. The same lines and exit status apply to a single-case run (`kairon eval <agent> <case>` / `--case`) and to `--resume`.

### Percent versus fraction

The verdict uses **percent**, `agent_scores` in `summary.json` uses a **0–1 fraction**. They are not interchangeable:

| Field | Unit | Example |
|-------|------|---------|
| `agent_scores.<agent>` | fraction (0–1), unchanged | `0.96` |
| `agent_verdicts.<agent>.score` | percent (0–100); the mean over repeats | `96` |
| `agent_verdicts.<agent>.score_min`, `.score_max` | percent (0–100) | `92`, `100` |
| `agent_verdicts.<agent>.repeat_scores` | list of percent (0–100) | `[92, 96, 100]` |
| `agent_verdicts.<agent>.threshold` | percent (0–100) | `95` |
| `agent_verdicts.<agent>.min_criterion_score` | percent (0–100) | `50` |
| `agent_verdicts.<agent>.success_rate` | percent (0–100), not a 0–1 fraction | `75` |
| `agent_verdicts.<agent>.attempts_total`, `.attempts_passed` | integer counts of case results (attempts) | `4`, `3` |
| `agent_verdicts.<agent>.criteria[].percent` | percent (0–100) | `66.7` |
| `agent_verdicts.<agent>.criteria[].score`, `.max_score` | **points** (integers), not percent | `2`, `3` |

`agent_scores` keeps its earlier meaning so that `kairon eval diff` reads old and new runs alike. Because it pools every result of the agent, with `--repeat` it is also the mean (as a fraction). See [Run summary](#run-summary-summaryjson) for the full JSON.

## Repeat runs (`--repeat`)

A single run with an LLM judge is noisy: the same prompt can score differently from one run to the next. `--repeat N` runs every case N times so that you read a **mean with its spread** instead of one sample, for example before promoting a prompt that has to reach 95% or more.

```bash
kairon eval architect --repeat 5
kairon eval architect --case decompose-feature --repeat 3   # repeats that one case
```

**How it runs.**

- Execution is case by case: a case runs N times back to back, then the next case. After k cases every repeat therefore covers the same k cases, and a partial `summary.json` already has a meaningful mean, min and max.
- Each run goes through the normal case path, so each gets its own **fresh [case workspace](#case-workspaces)** (nothing a run wrote is visible to the next one). With `--keep-workspaces` every run keeps its own `workspace_dir`.
- Cases are loaded once and the same case values are reused for every run. A stub `judge` list (`stub.judge: [yes, no]`) therefore continues from one run to the next instead of restarting, which is how the self-test produces two different scores (see [Self-Test](#self-test)).
- Progressive saving still happens after each case, and writes all N runs of that case together.

**What is kept.** Every run is its own entry in `cases` of `<agent>.json`: the list stays flat, so a case name appears N times, tagged with `run` (1-based; `1` for a plain run). The agent result records `repeat`. A result file without `run` or `repeat` (written before this feature) is read as one run of one repeat.

**How the numbers are computed.** For repeat k, the repeat score is the sum of the run-k case scores over the sum of the run-k case maxima, as a percent, over the same non-cost criteria as always (skipped, errored and no-output criteria count as 0 and stay in the maximum, see [Skipped Criteria](#skipped-criteria)).

| `agent_verdicts.<agent>` field | Meaning with `--repeat` |
|--------------------------------|-------------------------|
| `repeat` | Number of repeats the result holds (`1` for a plain run). |
| `score` | The **mean** over the repeats. This is what `passed` is judged on. |
| `score_min`, `score_max` | The lowest and highest repeat score. |
| `repeat_scores` | The repeat scores in run order, one per completed repeat. |
| `passed` | `score >= threshold`, on the mean. Repeats of 100% and 80% against a threshold of 95 give a mean of 90 and a FAIL even though the best repeat is above the threshold. |

All four new fields are always written, including a legitimate `score_min` of `0`. A plain run writes `repeat: 1`, `score_min == score_max == score` and `repeat_scores: [score]`. Nothing scored fails closed with all of them `0`. `cases_total` counts distinct case names; see [Agent verdict](#agent-verdict) for `cases_failed`. `attempts_total` counts every repeat, so it is N times the single-run count. These fields are the stable contract that promotion tooling can read from `summary.json`.

**Cost** is the cost of all repeats: `cost` and `total_cost` sum every run, so `--repeat 3` costs about three times a single run. If any repeat had an unknown cost the sum is marked `cost_incomplete` (see [Unknown cost](#unknown-cost)).

**Output.** A plain run prints exactly what it always did. With `--repeat N` (N > 1) each case line is followed by one `run k/N` line per run with its usual status, and the agent line ends with the `[repeat N: min X%, max Y%]` suffix described in [Output and exit status](#output-and-exit-status).

**Validation.**

- `--repeat` below `1` is rejected before anything runs, with `--repeat must be at least 1 (got 0)` and a non-zero exit.
- `--repeat` above `1` together with `--list`, `--cleanup` or `--perf` is rejected (`❌ repeat 2 cannot be combined with --list: it does not run any cases`), because those modes run no cases.
- `--case` / `<agent> <testcase>` repeats just that case.

**Resume.** `--resume` works with repeats. A case counts as completed only when all N of its runs are saved, so an interrupted case re-runs from its first run. The resumed run must use the same `--repeat` as the interrupted one; otherwise it stops with `cannot resume: repeat changed since the interrupted run of <agent> (recorded repeat=N; now M) — resume with --repeat N or start a fresh run`, so one `<agent>.json` never mixes repeat counts. A legacy result file counts as `repeat` 1. See also [Resume refusal](#resume-refusal).

**`eval diff`** works across runs with different repeat counts. Per-criterion averages and costs cover every result (means and totals), and the output comparison uses run 1 of each case. See [Comparing Runs](#comparing-runs).

## Inference Backends

Every "send a prompt to a model, get text back" call made by the harness (the agent under test and the LLM judge) goes through an `inference.Backend` defined in `internal/inference`. The package is stdlib-only and does not import `internal/eval`, so other packages can reuse it without import cycles.

| Backend | Behaviour |
|---------|-----------|
| `kiro-cli` (default) | Shells out to `kiro-cli`. Agent (native run): `kiro-cli chat --agent <agent> --no-interactive --trust-all-tools [--model <model>]`, with `--resume` added directly after `--no-interactive` for turns 2..n of a [multi-turn case](#multi-turn-cases) (`Request.Turn > 0`); under `--sandbox` the agent call uses `--trust-tools=<per-agent set>` instead of `--trust-all-tools` (see [Tool trust](#tool-trust)); judge: `kiro-cli chat --no-interactive [--model <model>]`; prompt on stdin. `--model` is appended when the request carries a model, which is always the case in a normal `kairon eval` run (see [Model Pinning and Run Provenance](#model-pinning-and-run-provenance)). Usage is always **estimated**. Requires `kiro-cli` on `PATH`. |
| `stub` | Deterministic and in-process. Never starts a process or touches the network, and does not require `kiro-cli`. The agent's output comes from the case's `stub.turns`; every judge call returns score 5 (the maximum of the judge scale) with `pass: true`, except yes/no judge calls made by [`judge` checks](#the-judge-check), which are answered from the case's `stub.judge` (see [Stub Case Fields](#stub-case-fields)). |

```bash
kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
```

Notes:
- The `kiro-cli` startup probe (`kiro-cli --version`) and the PATH availability check are performed by the selected backend; the stub reports zero startup overhead and is always available.
- Every backend can run under `--sandbox`, including `--backend stub`. The container is only a transport: the backend chosen with `--backend` runs inside it, the prompt is delivered on stdin, and the request, cost accounting and call record are the same as a native run. See [Backends in the Container](#backends-in-the-container).
- Under `--sandbox` the agent model is pinned and passed exactly as in a native run: `evals.agent_model` is honoured and reaches `kiro-cli` as `--model` (see [`--sandbox` and `evals.agent_model`](#--sandbox-and-evalsagent_model)).

### Stub Case Fields

The stub backend reads its script from the test case:

```yaml
stub:
  turns:
    - response: |          # required: text returned as the agent's output
        ## Self-test
        ### Usage
      model: stub-model    # optional: recorded as the model in results
      usage:               # optional: scripted, "reported" token usage
        input_tokens: 123
        output_tokens: 45
      commands:            # optional: shell commands run in the case workspace before responding
        - "echo hi > marker.txt"
      tool_calls:          # optional: scripted tool calls, gated by the tool trust set (see Tool trust)
        - tool: fs_write
          command: "echo x > tool-marker.txt"
  judge: [yes, no]         # optional: answers to yes/no `judge` checks (a single value is allowed: `judge: yes`)
  routes: [f1, none]       # optional, simulated_user cases only: which fact each simulated reply answers (a fact id or `none`)
```

| Field | Required | Description |
|-------|----------|-------------|
| `stub.turns[].response` | yes | Text returned as the agent output. |
| `stub.turns[].model` | no | Model name recorded in `agent_cost.model` and on the agent call record. It is also the model the call is **priced** under (see [Pricing](#pricing)): a model with no configured price (such as `stub-model`) gives an unknown cost. When omitted, the recorded model is empty, but the call is **priced under the run's pinned agent model** (the served model falls back to the pinned `evals.agent_model`, usually a Sonnet id with a known price) — so omitting the model does *not* by itself make the cost unknown. Set an unpriced model id such as `stub-model` to exercise the unknown-cost path. |
| `stub.turns[].usage.input_tokens` / `output_tokens` | no | If present, these counts are used verbatim and marked `reported`. If absent, usage is estimated from text length. |
| `stub.turns[].commands` | no | Shell commands, each run with `sh -c` in the case workspace (the request's `WorkDir`), in order, before the response is returned. This lets a stub case simulate an agent that edits files. In a multi-turn case the workspace is shared, so a later turn's commands see the files of earlier turns. They are scripted environment actions, **not** tool calls, so the trust gate never applies to them. |
| `stub.turns[].tool_calls[]` | no | Scripted tool calls, each `{tool, command}`. They run after `commands`, in order. Each runs `sh -c <command>` in the workspace only if the tool is in the request's trust set; otherwise the command is skipped and a denial is recorded (see [Tool trust](#tool-trust)). With no trust set (every native run) every tool call runs. |
| `stub.judge` | no | Scripted answers to the yes/no calls made by [`judge` checks](#the-judge-check): a single value (`judge: yes`) or a list (`judge: [yes, no]`). See below. |
| `stub.routes` | no | Scripted routing decisions of a [simulated-user case](#stub-backend-stubroutes): entry *N* answers the *N*th (0-based) simulated reply and is a fact `id` or `none`. Only valid with `simulated_user`. |

The stub answers a request with `stub.turns[Request.Turn]` (0-based). In a single-turn (`input`) case only `turns[0]` is used; the other entries are ignored and not validated. In a `turns` case every user turn consumes its own entry: turn *k* (1-based) uses `stub.turns[k-1]`, including its `response`, `commands` and `tool_calls`, and the number of entries must equal the number of turns (otherwise the case fails to load). A case with no `stub`, a missing entry for the turn, or an empty `response` fails with `case has no stub.turns[<index>].response` (0-based index) rather than silently producing empty output. The `kiro-cli` backend ignores `stub`. See [Multi-turn cases](#multi-turn-cases).

In a [simulated-user case](#simulated-user-elicitation-cases) the number of agent turns is not fixed, so the count rule is a range: `stub.turns` must have at least `1 + len(turns)` and at most `max_turns + len(turns)` entries (the simulated phase runs 1 to `max_turns` agent turns, then the scripted turns). `stub.routes` is indexed by the routing decision number, never consumed as a cursor, and has at most `max_turns - 1` entries; a decision beyond the list fails the case with `case has no stub.routes[N]`. The stub answers routing requests with no model call.

How `stub.judge` behaves:
- It answers only the yes/no calls of [`judge` checks](#the-judge-check). The legacy 1-5 judge calls are unchanged and still always return score 5.
- It is a single value (`judge: yes`, the same as a one-element list) or a list (`judge: [yes, no]`). `yes` and `no` are matched case-insensitively with surrounding whitespace ignored and are rendered as a well-formed judge reply whose reasoning is `stub judge: yes` or `stub judge: no`. Any other value is returned **verbatim**, so `judge: garbage` exercises the `judge parse error` path.
- Answers are consumed **in order**, one per judge call, in the order the judge checks are evaluated (see [Evaluation order](#evaluation-order)), and **cycled**: with `[yes, no]` the calls get `yes`, `no`, `yes`, `no`, and so on.
- The position lives on the loaded case, so it **continues across repeats** of the same loaded case instead of restarting. A script of three answers against two judge checks per run gives `[yes, no]`, then `[yes, yes]`, then `[no, yes]`. Loading the cases again starts a fresh position, and each case has its own.
- `stub.judge` must not be empty and must not contain a blank entry; either is a load error naming the case.
- A case with a `judge` check but no `stub.judge` fails that check with `judge call failed: case has no stub.judge`. The check is never skipped and never passes by default.
- The `kiro-cli` backend ignores `stub.judge`.

How `commands` behave:
- They run in the case workspace: natively the host workspace directory, under `--sandbox` the container's workspace path, as the `sandbox` user. Commands with no workspace directory are an error and run nothing, so a test cannot write into the repository root by accident.
- A command that exits non-zero fails the call with an error carrying the command and its stderr; the scripted response is not returned and later commands do not run.
- All commands of a turn share one deadline: the request timeout (the case [`timeout`](#case-timeout), else the default). On expiry the command's whole process group is killed and the call fails with a timeout error (`stub timeout after <duration>`) that wraps `inference.ErrTimeout`, so it is recorded exactly like a `kiro-cli` timeout.
- `commands` and the trusted `tool_calls` of a turn run under that same deadline, `commands` first. A failing command stops the turn like any other command, and denials already recorded for the turn are kept on the failed call.

### Reported vs Estimated Usage

Each `agent_cost` and `judge_cost` in a result file carries `model` (when known) and `usage_source`:

| `usage_source` | Meaning |
|----------------|---------|
| `reported` | Token counts were supplied by the backend (for the stub: `stub.turns[].usage`). |
| `estimated` | Token counts were estimated as roughly 4 characters per token (`len/4`) of the prompt and the output. |

- `kiro-cli` exposes neither the served model nor token counts, so its usage is always `estimated`. The `model` it records is the model the process was *launched with* (the value passed as `--model`, i.e. the pinned model), not a model confirmed by the service. It is empty (omitted from JSON) only for unpinned calls, such as code paths that do not go through `kairon eval`.
- `estimated_usd` is computed from the token counts at the **configured price of the model that served the call** (see [Pricing](#pricing)), whether the counts were reported or estimated. There is no single fixed rate: a call on `claude-haiku-4.5` is never priced as Sonnet.
- **A model with no configured price has an unknown cost.** Its call contributes `0` to `estimated_usd`, which is *not* a real zero, and the cost is flagged `cost_incomplete` with the model named in `unpriced_models` (see [Unknown cost](#unknown-cost)). A call that spent no tokens at all costs a known `0`: nothing was spent, so nothing is unknown.
- When several judge calls are accumulated into `judge_cost`, the merged `usage_source` is `reported` only if every contributing call was `reported`; otherwise it is `estimated`. The stub judge always produces estimated usage; its model is the pinned judge model when the request is pinned (as in every `kairon eval` run, so it is priced like the real judge) and `stub` otherwise.
- `summary.json` totals `tokens_in`, `tokens_out` and `estimated_usd`, plus `cost_incomplete` and `unpriced_models` when a contributing cost was unknown (and the provenance, execution-mode and containment fields described below).
- The individual calls behind `agent_cost` and `judge_cost` are listed in each case's `calls` array (see [Per-call records](#per-call-records-calls)).

Example from the self-test `stub-usage` case:

```json
"agent_cost": {
  "tokens_in": 123,
  "tokens_out": 45,
  "estimated_usd": 0,
  "model": "stub-model",
  "usage_source": "reported",
  "cost_incomplete": true,
  "unpriced_models": ["stub-model"]
}
```

`stub-model` has no configured price, so the tokens are recorded but the cost is unknown. The self-test case `stub-price-sonnet` (model `claude-sonnet-5.5`, 1000 input / 500 output tokens) is priced: `1000 * 3 / 1e6 + 500 * 15 / 1e6` = `0.0105`, with no `cost_incomplete`. `stub-price-unknown` (model `claude-haiku-4.5`, same tokens) has an unknown cost.

### Pricing

Cost is `tokens x price` for the model that served each call. Prices come from a top-level `pricing:` block in `.kairon/config.yaml` (not under `evals:`, so production code that shares the `inference.CallRecord` format can use the same block). The arithmetic lives in `internal/inference` (`PriceTable.Cost`); `internal/eval` only looks prices up and holds no price constants.

```yaml
pricing:
  claude-sonnet-5.5:
    input_usd_per_mtok: 3
    output_usd_per_mtok: 15
  claude-sonnet-5:
    input_usd_per_mtok: 3
    output_usd_per_mtok: 15
  # ... claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4 at the same 3 / 15
```

| Rule | Behaviour |
|------|-----------|
| Keys | **Exact** model ids, matched as written: no trimming and no case folding (`Claude-Sonnet-5.5` does not match `claude-sonnet-5.5`). An empty key or one with leading or trailing whitespace is rejected when the config is loaded. |
| Unit | USD **per million tokens**, `input_usd_per_mtok` and `output_usd_per_mtok`. Both are required for an entry; negative, `NaN` and infinite values are rejected. The error names the model id. |
| Formula | `input_tokens * input_usd_per_mtok / 1e6 + output_tokens * output_usd_per_mtok / 1e6`. |
| Unpriced model | No entry means an **unknown** cost (see [Unknown cost](#unknown-cost)). The harness never falls back to another model's price. |
| Defaults | When the `pricing` key is **absent** (or the config file is missing) the built-in defaults apply: the five Sonnet ids in `evals.allowed_models` (`claude-sonnet-5.5`, `claude-sonnet-5`, `claude-sonnet-4.6`, `claude-sonnet-4.5`, `claude-sonnet-4`) at 3 / 15. `kairon init` writes the same block, active, into new projects, and `.kairon/config.yaml` carries it too. |
| Replace, not merge | When the `pricing` key is **present** it replaces the defaults entirely. Remove an entry and that model's cost becomes unknown; an empty mapping (`pricing: {}`) prices nothing. |
| Haiku | `claude-haiku-4.5` is allowed to run (it is in `evals.allowed_models`) but is **intentionally unpriced**: add an entry to price it. |

`kairon eval` reads the block once, in the pre-flight, and pins it for the run. A config that cannot be read or parsed, or a `pricing` entry that is invalid, refuses the run with `cannot load pricing: ...` before any case starts and before any model call, like an invalid `evals` block.

#### Unknown cost

When a call used tokens on a model with no price, its cost cannot be computed. The harness records that fact instead of a plausible-looking number:

- **Call record** (`calls[]`): `cost_usd` is `0` and `cost_unknown` is `true`. A reader must **not** sum a `cost_usd` of `0` with `cost_unknown: true` as a real price. `cost_unknown` is omitted when the cost is known, so records written before this field existed read as known, which is what they were.
- **Cost objects** (`agent_cost`, `judge_cost`, `agent_verdicts.<agent>.cost`, `summary.total_cost`): `estimated_usd` holds the sum of the *known* costs, `cost_incomplete` is `true`, and `unpriced_models` lists the model ids that had no price (sorted, de-duplicated; empty ids are left out). Both fields are omitted when every contributing cost was known. The flags propagate upward: one unpriced call makes its case's cost, its agent's verdict cost and the run's `total_cost` incomplete, so `total_cost.estimated_usd` is then a **lower bound**, not a total.
- **Call record model and priced model are the same value.** The model of a call is the model the backend reports, else the model the request was pinned to; the price lookup and the `model` in the record both use it, so a record cannot say "model X" while its cost was priced under another id.

```json
"calls": [
  { "role": "agent", "model": "claude-haiku-4.5", "input_tokens": 1000, "output_tokens": 500,
    "cost_usd": 0, "cost_unknown": true, "estimated": false }
],
"agent_cost": {
  "tokens_in": 1000, "tokens_out": 500, "estimated_usd": 0,
  "model": "claude-haiku-4.5", "usage_source": "reported",
  "cost_incomplete": true, "unpriced_models": ["claude-haiku-4.5"]
}
```

Where the incompleteness is shown to a human:

- **Verdict line.** The `PASS`/`FAIL` line of an agent whose cost is incomplete ends with `[cost incomplete: no price for <models>]`, for example `PASS selftest: 100.0% (threshold 95.0%), 0/25 cases failed, success rate 100.0% (25/25 attempts) [cost incomplete: no price for claude-haiku-4.5]`. The line still starts with `PASS ` or `FAIL `, so `grep '^PASS \|^FAIL '` keeps working (see [Output and exit status](#output-and-exit-status)).
- **`eval diff`.** If either run's `total_cost` is incomplete, the `Cost:` delta line is suffixed ` (incomplete: <side(s)>)` naming which run(s) are incomplete, and the quality-per-dollar lines are suffixed ` (cost incomplete)`, because a ratio over a lower bound overstates quality per dollar (see [Comparing Runs](#comparing-runs)).

**Failed calls stay outside `judge_cost`.** This is unchanged by pricing. A judge call that failed, or whose reply could not be parsed, still has its `calls[]` record (with its tokens, and its `cost_usd` / `cost_unknown`), but adds nothing to the case's `judge_cost`, so `judge_cost` can be lower than the sum of the `calls[]` costs. Likewise a failed agent call is in `calls[]` only. Pricing does not change which calls are counted, only how each counted call is priced.

**Out of scope.** `kiro-cli` reports neither tokens nor the served model, so its usage stays *estimated* (about 4 characters per token) and its model is the one it was launched with. Pricing makes the cost correct for whatever usage a backend reports and visible when it is unknown; it does not make a `kiro-cli` token count exact.

## Model Pinning and Run Provenance

Every agent call and judge call in a `kairon eval` run is pinned to an explicit model, and every result records which models and which prompt produced it. A run cannot silently fall back to the account default (`auto`), which may be a frontier model.

### The `evals` block

Configure pinning in the `evals` block of `.kairon/config.yaml`. The block is optional: when it is absent, or only partly present, the defaults below apply. Only the `evals` key is read, so eval runs do not need `repo:` to be set, and a missing `.kairon/config.yaml` simply means "all defaults". A config file that cannot be read or parsed refuses the run instead of guessing.

```yaml
evals:
  agent_model: ""                  # optional override for the agent under test
  judge_model: "claude-sonnet-5.5"
  allowed_models:
    - "claude-sonnet-5.5"
    - "claude-sonnet-5"
    - "claude-sonnet-4.6"
    - "claude-sonnet-4.5"
    - "claude-sonnet-4"
    - "claude-haiku-4.5"
  # trust_tools:                   # optional: agent name -> tools trusted in --sandbox runs (see Tool trust)
  #   builder: [read, write, shell]
```

| Key | Default | Description |
|-----|---------|-------------|
| `evals.agent_model` | empty (not set) | Model for the agent under test. When set, it is used for every agent in the run and **overrides** the `model` in the agent's config. When empty, each agent runs on the `model` from its own agent config (`<evals-dir>/agents/<agent>.json` if present, else `.kiro/agents/<agent>.json`). |
| `evals.judge_model` | `claude-sonnet-5.5` | Model for every LLM-judge call. An explicit empty value stays empty and is refused. |
| `evals.allowed_models` | the six models listed above, in that order | Allowlist. The effective agent model and the judge model must each be exactly one of these entries. |
| `evals.trust_tools` | none | Map of agent name to the tool names that agent is trusted to use in a `--sandbox` run. It **overrides** the agent config's `allowedTools` for that agent; an empty list (`builder: []`) trusts nothing. `*` and empty entries are rejected. Has no effect on native runs. See [Tool trust](#tool-trust). |

Notes:
- Prices are configured separately, in the top-level `pricing` block (see [Pricing](#pricing)), not under `evals`.
- A user-supplied `allowed_models` list **replaces** the default list; it is not appended to it. Omit the key to keep the defaults.
- All values are whitespace-trimmed, and empty `allowed_models` entries are dropped. For `trust_tools`, agent and tool names are trimmed and entries with an empty agent name are dropped.
- Use `agent_model` to run on a model that is available on your machine without editing agent configs (for example `agent_model: claude-sonnet-4.5` when `claude-sonnet-5.5` is not available). The override must itself be in `allowed_models`.
- Agent configs are never modified; the model is passed to `kiro-cli` with `--model`, both natively and under `--sandbox`.

### Allowlist refusal

Before the first case starts, `kairon eval` resolves the judge model and the effective model of every agent in scope (the named agent, or every agent that has a rubric) and checks each against `allowed_models`. A model is **rejected** when it is:

- empty (for example `judge_model: ""`, or an agent config with no `model` and no `evals.agent_model` override),
- `auto` (case-insensitive), or
- not exactly one of the `allowed_models` entries (including the case where `allowed_models` is empty, which permits nothing).

The check runs in the pre-flight step of the run, so a refused run:

- starts no case and makes no `kiro-cli` call (not even the `--version` startup probe),
- creates no results directory, and
- exits with an error.

It applies to both backends (the stub backend is refused the same way) and to single-test-case and `--resume` runs. `--list` and `--cleanup` are not blocked.

**All violations are reported together**, judge first, then agents, so one run shows everything that needs fixing. Each line names the setting or agent, the rejected value (or says it is empty), where an agent's model came from (`evals.agent_model` or the agent config path), and the full allowlist, followed by a hint to edit the `evals` block:

```
❌ eval model pinning refused the run before any case started:
  - evals.judge_model: model "auto" is not permitted; allowed_models: [claude-sonnet-5.5, claude-sonnet-5, claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4, claude-haiku-4.5]
  - agent "architect" (model from .kiro/agents/architect.json "model"): model is empty; allowed_models: [claude-sonnet-5.5, claude-sonnet-5, claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4, claude-haiku-4.5]
Edit the evals block in .kairon/config.yaml (agent_model, judge_model, allowed_models)
```

An agent whose config cannot be found or parsed, or whose `prompt: file://...` file is missing, is reported in the same list (the error names both config paths that were tried). If `allowed_models` is empty the message says `allowed_models is empty: no model can be used`.

When the pre-flight passes, one line summarises what the run is pinned to:

```
🔒 Models: judge=claude-sonnet-5.5, architect=claude-sonnet-5.5 (prompt sha256 1a2b3c4d…)
```

### `--sandbox` and `evals.agent_model`

**`--sandbox` honours `evals.agent_model`.** A sandboxed run builds the same agent request as a native run, so the pinned model is sent with the call. With the `kiro-cli` backend the container runs `kiro-cli chat --agent <agent> --no-interactive --trust-tools=<per-agent set> --model <model>`: the same arguments as the native backend except that `--trust-all-tools` is replaced by `--trust-tools=…` (see [Tool trust](#tool-trust)). The effective model is the same as without `--sandbox`: `evals.agent_model` when set, otherwise the `model` in the agent config. It is validated against `allowed_models` in the pre-flight, recorded as `agent_model`, and recorded on the agent call. No warning is printed.

`evals.judge_model` is honoured in the same way: judge calls execute on the host through the inference backend, pinned with `--model <judge_model>`, and are subject to the same allowlist check.

The `<evals-dir>/agents/` overlay is staged into the case workspace's `.kiro/agents/` on the host and reaches the container through the read-only `.kiro` mount (see [Staged `.kiro`](#staged-kiro-and-precedence)), so the agent itself can see it. The model-pinning provenance for `--backend kiro-cli --sandbox` is nevertheless computed from `.kiro/agents/<agent>.json` only (`prompt_sha256`, `resources_present`, and the config model used when `evals.agent_model` is unset); that part of the pre-flight is unchanged. With `--backend stub --sandbox` no agent config is read inside the container, so the overlay applies exactly as in a native run; this is what lets the self-test, whose agent config lives only in `internal/eval/testdata/evals/agents/`, pass the pre-flight under `--sandbox`.

### Recorded provenance

#### Per-call records (`calls`)

Each case in `<agent>.json` carries a `calls` array with one record per agent call and one per judge call, in execution order. The record shape is the shared `inference.CallRecord`:

| Field | Description |
|-------|-------------|
| `role` | `agent` or `judge`. |
| `model` | The model that served the call when the backend reports one, otherwise the pinned (requested) model. `kiro-cli` cannot report the served model, so its records carry the pinned model. The stub agent records the scripted `stub.turns[].model` (e.g. `stub-model`, possibly empty); the stub judge records the pinned judge model, else `stub`. The price lookup uses this same value. |
| `agent` | Agent name (agent calls only). |
| `turn` | 1-based user turn of the call. Present only on agent calls of a `turns` or simulated-user case (see [Multi-turn cases](#multi-turn-cases)); omitted for single-turn cases and judge calls. |
| `criterion` | Rubric criterion being judged (judge calls only). |
| `purpose` | Why a judge call was made when it is not a criterion check. Currently only `simulated_user`, the routing call of a [simulated-user case](#routing). Omitted otherwise. |
| `input_tokens` / `output_tokens` | Token counts for the call. |
| `cost_usd` | Cost of the call: tokens times the configured price of `model` (see [Pricing](#pricing)), the same price used for `agent_cost` / `judge_cost`. `0` when the model has no configured price; see `cost_unknown`. |
| `cost_unknown` | `true` when `cost_usd` is `0` only because `model` has no configured price (the call used tokens). Omitted when the cost is known. Do not sum such a `cost_usd` as a real price. See [Unknown cost](#unknown-cost). |
| `estimated` | `true` unless the usage was `reported` by the backend. |
| `duration_ms` | Call duration in milliseconds: the backend's measurement, or wall-clock time when the backend reports none (the stub). |
| `prompt_sha256` | Hash of the agent's prompt inputs (agent calls only). It is the **case's own** `prompt_sha256` (see [Per-case fields](#per-case-fields-in-agentjson)), computed from that case's staged workspace. It equals the file-level `prompt_sha256` unless the case's fixture overrides `.kiro` content. |
| `trusted_tools` | The whole-tool trust set the call ran with, in `kiro-cli --trust-tools` spelling (for example `["fs_read","fs_write"]`). Present only on `--sandbox` agent calls; omitted for native calls, which run with `--trust-all-tools`. An empty list `[]` means "restricted to nothing", which is different from absent. |
| `tool_denials` | Tool calls the trust gate refused: `{tool, command, reason}` with `reason` `tool not trusted`. Omitted when empty. Populated by the stub backend only (see [Limits](#limits)). |
| `error` | Set when the call failed. A failed call is still recorded. |

Details:
- `agent_cost` and `judge_cost` are unchanged; `calls` is the per-call breakdown behind them.
- No agent record is written when prompt assembly failed, because no call was made. A `turns` case writes one agent record per turn that ran (the failed turn included), each with its `turn`.
- A judge call is recorded even when its output could not be parsed (the tokens were spent). Its cost appears in that call's `calls[]` record but **not** in the case's `judge_cost`, which keeps a zero cost for a failed or unparseable judge call — so for such a case the sum of `calls[].cost_usd` can exceed `judge_cost`. Failed judge calls remain outside `judge_cost` whatever the model's price (see [Unknown cost](#unknown-cost)).
- A `--sandbox` run builds its agent record exactly like a native one, from the same request and the same completion logic: the cost comes from the backend's reported or estimated usage, `model` is the served model when the backend reports one (the stub) or the pinned model (`kiro-cli`), and `estimated` is `true` unless the usage was `reported`. For the same case the sandboxed and native `output`, `agent_cost`, call record `model` and `prompt_sha256` match. The sandboxed agent record additionally carries `trusted_tools`; the only other differences between a native and a container run are the run-level `sandbox` and `containment` fields (see [Execution mode and containment](#execution-mode-and-containment) and [Parity between native and container runs](#parity-between-native-and-container-runs)).

The per-call records (field values below are illustrative):

```json
{
  "case_name": "stub-usage",
  "calls": [
    {
      "role": "agent",
      "model": "stub-model",
      "agent": "selftest",
      "input_tokens": 123,
      "output_tokens": 45,
      "cost_usd": 0,
      "cost_unknown": true,
      "estimated": false,
      "duration_ms": 0,
      "prompt_sha256": "9f2c…"
    },
    {
      "role": "judge",
      "model": "claude-sonnet-5.5",
      "criterion": "clarity",
      "input_tokens": 210,
      "output_tokens": 18,
      "cost_usd": 0.0009,
      "estimated": true,
      "duration_ms": 0
    }
  ]
}
```

#### Run-level fields in `<agent>.json`

The per-agent result file always carries:

| Field | Description |
|-------|-------------|
| `agent_model` | The effective model the agent ran on (`evals.agent_model`, else the agent config's `model`), the same with and without `--sandbox`. |
| `judge_model` | The model used for judge calls. |
| `prompt_sha256` | Agent-level (baseline) hash of everything that shapes the agent's prompt, computed over a staged workspace with no fixture overrides (see [How `prompt_sha256` is computed](#how-prompt_sha256-is-computed)). With `--prompt-file`, the prompt part of the hash is computed from the candidate file's bytes. Individual cases carry their own value (see [Per-case fields](#per-case-fields-in-agentjson)). |
| `prompt_file` | The `--prompt-file` path of a [candidate prompt run](#evaluating-a-candidate-prompt---prompt-file), as given on the command line and normalised with `filepath.Clean` and `filepath.ToSlash` (a relative path stays relative, an absolute path stays absolute). Omitted (the key is absent) for runs without `--prompt-file`. It is the path text only, not a content identity: key on `prompt_sha256` for that. |
| `resources_present` | The agent-config `resources` entries that existed in that baseline staged workspace when the hash was computed. Always written; an empty list serialises as `[]`. A project `*-conventions` skill is never staged, so it is never listed here. |
| `sandbox` | Execution mode as a **boolean**: `true` for a `--sandbox` (container) run, `false` for a native run. Written by every run, including single-case (`--testcase`) runs. Absent in files written before mode tracking. Note that `summary.json` spells the same fact as a string (`"native"` / `"container"`); see below. |
| `containment` | What contained this agent's cases (see [Execution mode and containment](#execution-mode-and-containment)). Present for container runs only; absent for native runs. |
| `repeat` | Number of times every case was run (`--repeat`; see [Repeat runs](#repeat-runs---repeat)). Written as `1` for a plain run; omitted when `0`, and absent in files written before repeats existed, which count as `1`. Each entry of `cases` then carries `run` (1-based). |
| `suite_sha256` | Identity of the agent's eval suite: a 64-character lowercase hex SHA-256 over its rubric, case files and the fixtures those cases reference (see [Run coverage](#run-coverage-suite_sha256-complete-and-case_filter)). Absent in files written before coverage tracking. |
| `complete` | `true` only when every case of the agent has a result for every repeat **and** `case_filter` is empty. `false` is written explicitly. Absent in files written before coverage tracking. |
| `case_filter` | The `--case` value (`kairon eval <agent> <case>`), `""` for a full run. `""` is written explicitly. Absent in files written before coverage tracking. |

```json
{
  "agent": "selftest",
  "git_hash": "a1b2c3d",
  "agent_model": "claude-sonnet-5.5",
  "judge_model": "claude-sonnet-5.5",
  "prompt_sha256": "9f2c…",
  "sandbox": false,
  "resources_present": [],
  "repeat": 1,
  "suite_sha256": "5c0e…",
  "complete": true,
  "case_filter": "",
  "cases": [ … ]
}
```

A `--prompt-file` run additionally carries `"prompt_file": "<path>"` (and a `prompt_sha256` computed from the candidate), for example `"prompt_file": ".kairon/iterations/selftest/v2.md"`.

##### Per-case fields in `<agent>.json`

A pinned run (every normal `kairon eval` run, see [The `evals` block](#the-evals-block)) also records, on **each entry of `cases`**, what that case's own agent saw:

| Field | Description |
|-------|-------------|
| `prompt_sha256` | The hash described in [How `prompt_sha256` is computed](#how-prompt_sha256-is-computed), taken over **the case's staged workspace** after its fixture, the `<evals-dir>/agents/` overlay and any `--prompt-file` candidate were applied. |
| `resources_present` | The agent-config `resources` entries that existed in that workspace. Written whenever the case's `prompt_sha256` is, even when empty (`[]`). |

Relation to the agent-level fields:

- The agent-level `prompt_sha256` / `resources_present` (in `<agent>.json`, `summary.json` and the resume check) are the **baseline**: the hash of the workspace a case gets when its fixture overrides nothing. They never include a fixture, so they do not vary from case to case.
- A case's value **equals the baseline** unless its fixture overrides `.kiro` content that the agent loads: a `skills/<name>-conventions/` skill, the prompt file, the agent config or a resource. A case whose fixture supplies a `*-conventions` skill that the agent lists in `resources` has that entry in its own `resources_present` and a `prompt_sha256` that differs from the baseline. Cases without such a fixture repeat the baseline.
- The case value is what explains that case's score. Use it to confirm which prompt, resources and override hooks a given result was produced under; use the agent-level value to decide whether two runs are comparable.
- Each agent `calls[]` record of the case carries the same `prompt_sha256` as the case.
- Both fields are **omitted** from a case in an unpinned run and in result files written before they existed, so those keep their earlier shape. `--resume`, `eval diff` and [`verify-log`](#iteration-logs-and-verify-log) keep comparing the agent-level values only.

```json
{
  "case_name": "stub-conventions-fixture",
  "prompt_sha256": "c41a…",
  "resources_present": ["skill://.kiro/skills/selftest-conventions/SKILL.md"],
  "calls": [ { "role": "agent", "agent": "selftest", "prompt_sha256": "c41a…", "…": "…" } ]
}
```

In the self-test, `stub-conventions-fixture` shows this: its workspace fixture supplies `selftest-conventions`, so the case lists it in `resources_present` with a `prompt_sha256` that differs from `stub-basic`'s, while the agent-level `resources_present` stays `[]`.

#### Run summary (`summary.json`)

`summary.json` always carries `judge_model` and an `agents` map with `agent_model`, `prompt_sha256` and `resources_present` per agent (plus `prompt_file` for a [candidate prompt run](#evaluating-a-candidate-prompt---prompt-file), and the run-coverage fields `suite_sha256`, `complete` and `case_filter`, see [Run coverage](#run-coverage-suite_sha256-complete-and-case_filter)). These are the agent-level baseline values; per-case values live only in `<agent>.json` (see [Per-case fields](#per-case-fields-in-agentjson)). It also records the run's execution mode in `sandbox` and, for container runs, a per-agent `containment` map. The example below is a `--sandbox` (container) run (the `tool_trust` values are illustrative; they follow each agent's trust set, see [Tool trust](#tool-trust)):

```json
{
  "git_hash": "a1b2c3d",
  "total_cost": { "tokens_in": 4210, "tokens_out": 980, "estimated_usd": 0.0273, "cost_incomplete": true, "unpriced_models": ["claude-haiku-4.5"] },
  "agent_scores": { "architect": 0.85, "builder": 0.8 },
  "agent_verdicts": {
    "architect": { "score": 85, "threshold": 95, "passed": false, "cases_total": 4, "cases_failed": 1, "attempts_total": 4, "attempts_passed": 3, "success_rate": 75, "cost": { "tokens_in": 2100, "tokens_out": 500, "estimated_usd": 0.0138 }, "repeat": 1, "score_min": 85, "score_max": 85, "repeat_scores": [85],
                    "criteria": [ { "name": "task_decomposition", "score": 40, "max_score": 50, "percent": 80 }, { "name": "completeness", "score": 45, "max_score": 50, "percent": 90 } ] },
    "builder":   { "score": 80, "threshold": 95, "passed": false, "cases_total": 3, "cases_failed": 1, "attempts_total": 3, "attempts_passed": 2, "success_rate": 66.67, "cost": { "tokens_in": 2110, "tokens_out": 480, "estimated_usd": 0.0135 }, "repeat": 1, "score_min": 80, "score_max": 80, "repeat_scores": [80],
                    "criteria": [ { "name": "code_correctness", "score": 40, "max_score": 50, "percent": 80 }, { "name": "spec_adherence", "score": 40, "max_score": 50, "percent": 80 } ] }
  },
  "judge_model": "claude-sonnet-5.5",
  "agents": {
    "architect": { "agent_model": "claude-sonnet-5.5", "prompt_sha256": "…", "resources_present": [], "suite_sha256": "…", "complete": true, "case_filter": "" },
    "builder":   { "agent_model": "claude-sonnet-5.5", "prompt_sha256": "…", "resources_present": [], "suite_sha256": "…", "complete": true, "case_filter": "" }
  },
  "sandbox": "container",
  "containment": {
    "architect": { "tool_trust": ["fs_read", "fs_write", "execute_bash"], "fake_gh": true, "read_only_fs": true, "kairon_binary": true, "network": "unrestricted" },
    "builder":   { "tool_trust": ["fs_read", "fs_write", "execute_bash"], "fake_gh": true, "read_only_fs": true, "kairon_binary": true, "network": "unrestricted" }
  }
}
```

| Field | Description |
|-------|-------------|
| `git_hash`, `total_cost`, `agent_scores` | Run totals. `agent_scores` is score / maximum per agent as a **0–1 fraction**; skipped criteria count as 0 and stay in the maximum (see [Skipped Criteria](#skipped-criteria)). An agent with a zero denominator has no entry. |
| `total_cost` | Sum of the `cost` of every entry in `agent_verdicts` (`tokens_in`, `tokens_out`, `estimated_usd`; `model` and `usage_source` are not aggregated). It accumulates across agents and is recomputed from the verdicts on every save, so repeated progressive saves of the same agent and `--resume` do not double count it. When any contributing call had an unknown cost it also carries `cost_incomplete: true` and the union of `unpriced_models`, and `estimated_usd` is then a lower bound (see [Unknown cost](#unknown-cost)); both keys are omitted otherwise. |
| `agent_verdicts` | Map of agent name to its pass/fail verdict (fields below). One entry per evaluated agent. Absent in a `summary.json` written before verdicts existed. |
| `judge_model`, `agents` | Run-level provenance, as described above. |
| `prompt_file` | The `--prompt-file` path of a candidate prompt run, normalised as in `<agent>.json`. Written at the top level only under the single-agent rule below (a `--prompt-file` run always names one agent, so it is always present for such a run), and per agent as `agents.<agent>.prompt_file`. Omitted entirely when no `--prompt-file` was used. |
| `sandbox` | Execution mode of the whole run: `"native"` or `"container"`. Absent in a `summary.json` written before mode tracking. |
| `containment` | Map of agent name to its containment record (see [Execution mode and containment](#execution-mode-and-containment)). Present for `container` runs; **absent** for `native` runs. |

**`agent_verdicts.<agent>` fields.**

| Field | Type | Description |
|-------|------|-------------|
| `score` | number, **percent** (0–100) | The agent's aggregate score. With `--repeat` it is the **mean** over the repeats. `0` when nothing was scored. |
| `threshold` | number, **percent** (0–100) | The agent's pass threshold (its rubric's `pass_threshold`, default 95). |
| `passed` | boolean | The agent passes when `score >= threshold` on the mean **and**, if `min_criterion_score` is set, no criterion's pooled percent is below that floor. `false` when nothing was scored. |
| `cases_total` | integer | Number of distinct cases evaluated (a case run N times counts once). |
| `cases_failed` | integer | Number of cases that did not pass their own threshold; for a case with several runs, its score pooled over the runs is compared to its threshold. Informational. |
| `attempts_total` | integer | Number of attempts: every case result of the agent, counting each repeat of a case separately (`--repeat 2` of 21 cases gives 42, while `cases_total` stays 21). Always written. |
| `attempts_passed` | integer | Number of those attempts that passed their own threshold. Independent of the agent threshold and the criterion floor. Always written. |
| `success_rate` | number, **percent** (0–100) | `attempts_passed * 100 / attempts_total`; `0` when `attempts_total` is `0`. Not rounded. Always written, including `0`. |
| `cost` | object | The agent's cost (agent and judge calls of every run of every case): `tokens_in`, `tokens_out`, `estimated_usd`, plus `cost_incomplete` and `unpriced_models` when a contributing call had an unknown cost (omitted otherwise; see [Unknown cost](#unknown-cost)). |
| `repeat` | integer | Number of repeats (`--repeat`; `1` for a plain run). Always written. |
| `score_min` | number, **percent** (0–100) | The lowest repeat score. Always written, including `0`; equals `score` for a plain run. |
| `score_max` | number, **percent** (0–100) | The highest repeat score. Always written; equals `score` for a plain run. |
| `repeat_scores` | array of **percent** numbers | The repeat scores in run order, one per completed repeat (`[score]` for a plain run). Always written. |
| `criteria` | array of objects | The agent's non-cost criteria pooled over every case and every repeat, in first-seen order (see [Per-criterion scores and the floor](#per-criterion-scores-and-the-floor)). Always written; `[]` when nothing was scored. Fields below. |
| `criteria[].name` | string | The criterion's name in the rubric. |
| `criteria[].score` | integer, **points** (not percent) | Points the criterion earned, summed over all cases and repeats. A skipped criterion counts `0`. |
| `criteria[].max_score` | integer, **points** (not percent) | The criterion's maximum, summed over the same cases and repeats. With `--repeat N` it is N times the single-run maximum. A criterion a `checks_only` rubric did not score in a case adds nothing to it. Never `0`: such entries are not listed. |
| `criteria[].percent` | number, **percent** (0–100) | `score * 100 / max_score`. With `--repeat` it is the mean over the repeats. This is what `min_criterion_score` is compared with. |
| `min_criterion_score` | number, **percent** (0–100) | The rubric's floor, echoed from the result. Omitted when the rubric sets none. |
| `criteria_below_floor` | array of strings | Names of the criteria whose `percent` is below `min_criterion_score`, in `criteria` order. Omitted when there are none (and always when no floor is set). Any entry makes `passed` `false`. |

`score`, `score_min`, `score_max`, `repeat_scores` and `threshold` are percent, while `agent_scores` is a 0–1 fraction (an `agent_scores` of `0.85` is an `agent_verdicts` score of `85`). See [Pass/Fail Verdict and Exit Status](#passfail-verdict-and-exit-status).

A `--repeat 3` run of one agent records, for example (the three repeats scored 92, 96 and 100, so the mean is 96 and the agent passes):

```json
"agent_verdicts": {
  "architect": { "score": 96, "threshold": 95, "passed": true, "cases_total": 4, "cases_failed": 0, "attempts_total": 12, "attempts_passed": 12, "success_rate": 100, "cost": { … },
                 "repeat": 3, "score_min": 92, "score_max": 100, "repeat_scores": [92, 96, 100] }
}
```

A verdict that failed on the floor alone (overall `66.7` against a threshold of `60`, but `report_marker` at `0%` under a floor of `50`) records:

```json
"agent_verdicts": {
  "selftest-floor": { "score": 66.7, "threshold": 60, "passed": false, "cases_total": 1, "cases_failed": 0, "attempts_total": 1, "attempts_passed": 1, "success_rate": 100, "cost": { … },
                      "repeat": 1, "score_min": 66.7, "score_max": 66.7, "repeat_scores": [66.7],
                      "criteria": [ { "name": "file_written", "score": 2, "max_score": 2, "percent": 100 },
                                    { "name": "report_marker", "score": 0, "max_score": 1, "percent": 0 } ],
                      "min_criterion_score": 50, "criteria_below_floor": ["report_marker"] }
}
```

`score`, `max_score` and `percent` in `criteria` are the pooled totals over every case and repeat, so the verdict `score` stays a percent while a criterion's `score` is points (the listed `score`s and `max_score`s add up to the numerator and denominator of the verdict `score`). `agent_scores` is unchanged.

**New fields in `<agent>.json`.** The agent result carries `threshold` (the agent's pass threshold, percent), `min_criterion_score` (the rubric's per-criterion floor in percent, at the top level; omitted when the rubric sets none, which a result file written before the floor existed also lacks, meaning no floor) and `repeat` (see above). Each case carries `threshold` (the case's resolved threshold, percent), `passed` (whether it was met) and `run` (1-based run number; with `--repeat 3` a case name appears three times, with `run` 1, 2 and 3):

```json
{
  "agent": "selftest",
  "threshold": 95,
  "repeat": 2,
  "cases": [
    { "case_name": "stub-basic", "run": 1, "threshold": 95, "passed": true, … },
    { "case_name": "stub-basic", "run": 2, "threshold": 95, "passed": true, … }
  ]
}
```

All of them are optional on read: a result file written before verdicts existed has no `threshold` or `passed`, and its cases are judged against the agent's threshold (95 when it records none). A file without `repeat` or `run` counts as one repeat, with every case run 1.

**Two spellings of one fact.** `summary.json` records the mode as a string (`"native"` / `"container"`), while each `<agent>.json` keeps `sandbox` as a boolean (`true` = container, `false` = native). Both come from the same run, and `eval diff` and `--resume` read both. A summary never mixes modes: one run is entirely native or entirely container.

**Single-agent rule.** A run can cover several agents (`kairon eval` with no agent), each with its own model and prompt hash, so one top-level value would be ambiguous. The top-level `agent_model`, `prompt_sha256`, `resources_present` and `prompt_file` are therefore written **only when the run covers exactly one agent** (`kairon eval <agent>`, a single-case run, or the self-test) and are omitted otherwise. Use the `agents` map, or the per-agent `<agent>.json`, for multi-agent runs. Single-case runs also write a `summary.json`, with the same provenance fields.

#### Execution mode and containment

Every run records how it was executed, through the same code that records the model pins, so no path (full run, single case, resume) can skip it.

- A **native** run records `sandbox: "native"` in `summary.json` (`sandbox: false` in `<agent>.json`) and **no containment**: nothing is contained, so there is nothing to describe. Native summaries do not carry a `containment` key at all, and an old native summary has the same shape as a new one apart from `sandbox`.
- A **container** run (`--sandbox`) records `sandbox: "container"` (`sandbox: true` in `<agent>.json`) and a containment record per agent, in `summary.json` under `containment.<agent>` and in `<agent>.json` as `containment`. The record is per agent because the tool trust set is per agent.

The containment record has five fields:

| Field | Meaning |
|-------|---------|
| `tool_trust` | The normalised `--trust-tools` names the agent's cases ran with, in `kiro-cli` spelling (for example `["fs_read","fs_write"]`; see [Tool trust](#tool-trust)). It is the same resolved set the call used, so it equals `trusted_tools` on that agent's [call records](#per-call-records-calls). `[]` means "trust nothing" and is always written, never omitted. If the trust set cannot be resolved (no `evals.trust_tools` override and the agent config is unreadable), `[]` is recorded and a warning is printed; the agent call itself fails closed, as described under Tool trust. |
| `fake_gh` | `true`: the fake `gh` is first on `PATH` (see [The fake `gh`](#the-fake-gh)). |
| `read_only_fs` | `true`: the container root filesystem is read-only (see [Read-only root filesystem](#read-only-root-filesystem)). |
| `kairon_binary` | `true`: the linux `kairon` binary is mounted read-only at `/usr/local/bin/kairon`, on `PATH`, for every backend including `kiro-cli` (see [The `kairon` binary](#the-kairon-binary)). Records from before this field existed lack it and read as `false`, which is accurate: `eval diff` reports the difference as `kairon_binary: false → true`. |
| `network` | Always `"unrestricted"`. See below. |

`tool_trust`, `fake_gh`, `read_only_fs` and `kairon_binary` are taken from the same sources that enforce them (the trust resolution used for the call, the fake-`gh` mount, the container host config's read-only root filesystem and the `kairon` binary mount), so the record cannot drift from what was applied.

**`network: "unrestricted"` is not the container's network mode.** The value records that Kairon makes **no network containment guarantee** for the run. It says nothing about the runtime's `NetworkMode`: containers are currently created with `NetworkMode: none`, but, as the [Sandbox Containment](#sandbox-containment) layer table and [Limits](#limits) state, that is a pre-existing setting that is "not a network policy" and is not part of the containment guarantee. Do not read `unrestricted` as "the container has network access", and do not read the absence of a guarantee as "the network is blocked". Side effects over the network remain the eval author's responsibility, handled with mocks; guidance for writing those mocks is planned as a separate follow-up (the mock-guidance issue, see [Limits](#limits)). `NetworkMode` itself is unchanged by this recording.

#### Parity between native and container runs

A native run and a `--sandbox` run of the same case are meant to be directly comparable, and the harness is built so that this holds structurally rather than by convention:

- **Provenance.** `agent_model`, `judge_model`, `prompt_sha256` and `resources_present`, in `<agent>.json` and `summary.json`, and `model` and `prompt_sha256` on every `calls[]` record, come from the same pinning and call-record code on both paths, so they are identical for the same inputs.
- **Scoring.** Both paths score a case through the same function. The score and denominator arithmetic (skipped criteria count as 0 and stay in the denominator) lives in one place, shared by the printed case result, the incremental `summary.json` writer and the summary builder, so a native and a container summary cannot compute `agent_scores` differently.
- **What may differ.** Apart from fields that are inherently per-run (timestamps in directory names, per-case container ids and workspace paths), a container run differs from a native run only in `sandbox`, `containment`, and `trusted_tools` on the agent call records.

The daemon-gated test `TestProvenanceParitySandbox` (run by `task eval:selftest:sandbox`) checks this by running `selftest` and `selftest-fail` natively and with `--sandbox` on the stub backend. It compares the provenance fields in `<agent>.json`, `summary.json` and every call record, checks `native` without containment against `container` with it, compares the **whole** `Summary` after normalising only `sandbox` and `containment`, compares per-case score totals and the threshold outcome, and requires `eval diff` between the two runs to succeed and report the mode difference. Like the other gated tests it skips, without passing, when the gate is unset or no daemon is reachable (see [Self-Test in the Container Sandbox](#self-test-in-the-container-sandbox)).

**The per-agent threshold verdict is part of that parity.** The per-agent pass/fail verdict, the `PASS`/`FAIL` line and the non-zero exit status are computed from the same shared score and denominator code as the printed case result and `agent_scores` (see [Pass/Fail Verdict and Exit Status](#passfail-verdict-and-exit-status)). Because the parity test compares the whole `Summary`, `agent_verdicts` and the accumulated `total_cost` are compared on both paths, and the threshold outcome of each agent is the same natively and under `--sandbox`.

#### Run coverage: `suite_sha256`, `complete` and `case_filter`

`prompt_sha256` says which prompt produced a score. Three more per-agent facts say **what the run measured**, so that [`verify-log`](#iteration-logs-and-verify-log) can prove that the only thing that changed between two runs is the thing an iteration claims to have changed.

| Field | Type | Meaning |
|-------|------|---------|
| `suite_sha256` | string, 64 lowercase hex | Identity of the agent's eval suite (below). |
| `complete` | boolean | `true` only when every case of the agent has a result for every repeat **and** `case_filter` is empty. |
| `case_filter` | string | The `--case` value (`kairon eval <agent> <case>` or `--case`); `""` for a full run. |

They are written to `<agent>.json` and to `summary.json` under `agents.<agent>`, always together. The top-level single-agent mirror fields of `summary.json` (`prompt_sha256`, `agent_model`, ...) are **not** extended: the coverage fields live only in `agents.<agent>` and `<agent>.json`. `complete: false` and `case_filter: ""` are written explicitly, so a recorded `false` or `""` is distinguishable from a result that never recorded the fields (a run made before this tracking existed, or a direct library call that bypasses `kairon eval`), which has none of the three. `verify-log` reads that second case as "not recorded".

**What `complete` and `case_filter` mean.**

- A case counts as covered when it has a saved result for every run `1..repeat` (a result with no `run` counts as run 1). An agent with no cases is never complete.
- **A `--case` run is never complete**, even if the agent has exactly one case: "full run" means "no filter and every case covered". It records `case_filter: <case name>` and `complete: false`.
- Every progressive save records its own coverage. A run interrupted after the first of several cases is left with `complete: false`; the final save of a finished run records `complete: true`. A resumed run recomputes `complete` over the saved and the new results together.
- `verify-log` does not look at the `.progress` marker. `complete` is per agent and is the single signal, so an agent that finished in a multi-agent run whose other agent was interrupted is still usable.

**How `suite_sha256` is computed.** It is a lowercase hex SHA-256 of the agent's eval suite, computed from the active evals directory (so it honours [`--evals-dir`](#evals-directory---evals-dir)) before any case starts. Only the bytes of these inputs are hashed:

1. **The rubric.** Every `rubrics/*.yaml` whose parsed `agent` equals the agent (the same selection the rubric loader makes).
2. **The case files.** Every `cases/<agent>/*.yaml` (non-recursive, `.yaml` only). This covers the whole agent suite whatever `--case` selects, so a filtered run and a full run of the same suite hash alike.
3. **What those cases reference:**
   - `workspace: <name>`: every regular file under `fixtures/workspaces/<name>/`. Symlinks and other non-regular entries are skipped, as when the workspace is built, and a file's execute bit is part of its identity because the workspace builder preserves it.
   - `checks[].inject` entries: `fixtures/hidden/<entry>`.
   - `mocks[].script`: `<evals-dir>/<script>`.
   - `setup[]` entries of `type: file`: the `path`, rebased onto `--evals-dir` like the prompt assembly does (so the default `.kairon/evals/fixtures/selftest-input.md` follows the chosen evals directory).

How the hash is built:

- Each input is one length-framed part (the same framing as [`prompt_sha256`](#how-prompt_sha256-is-computed)), after a leading domain part (`suite` / `v1`) that separates this hash from `prompt_sha256`. The part's kind carries the logical name: `rubric <rel>`, `case <rel>`, `file <rel>`, `file+x <rel>` or `missing <rel>`.
- `<rel>` is slash-separated and relative to the evals directory, so the hash does not depend on where that directory lives. A `setup` file outside the evals directory uses its cleaned path as written.
- Inputs are de-duplicated (a fixture shared by several cases counts once) and sorted by kind and name, so discovery order does not matter.
- Contents are hashed as raw bytes, with no line-ending normalisation.
- A referenced file that does not exist is hashed as `missing <rel>` and is **not** an error: the case reports the problem when it runs, and the suite identity stays stable. Any other read error stops the run before any case starts. An agent whose rubric or cases cannot be loaded is skipped here (it records no coverage), so the existing, more specific error or warning for it is unchanged.

Deliberately **not** part of the suite:

- The agent config, its prompt and its `resources`: that is `prompt_sha256`.
- The models (`agent_model`, `judge_model`) and the sandbox mode, which `verify-log` compares separately.
- The rubrics and cases of other agents.
- Workspace fixtures that no case references, and files in `fixtures/hidden/` that no check injects.

So editing a case, the rubric or a fixture a case uses changes `suite_sha256` and leaves `prompt_sha256` unchanged, and editing the prompt does the reverse.

#### How `prompt_sha256` is computed

`prompt_sha256` is a lowercase hex SHA-256 (64 characters). It is a hash only; the prompt text itself is not stored.

**It is hashed over a staged workspace, not over the repository.** The hash describes the bytes an agent actually sees, so it is computed from a staged `.kiro/` (see [Staged `.kiro` and precedence](#staged-kiro-and-precedence)): the config, the `file://` prompt and the `resources` are all read from inside that staged directory. Two staged workspaces are hashed:

- **The agent-level (baseline) workspace.** The value in `<agent>.json`, `summary.json` and the run pin is computed once per agent from a throwaway workspace staged **without any fixture files**: the project's git-tracked `.kiro/` files without `*-conventions` skills, the `<evals-dir>/agents/` overlay (unless the run is a `kiro-cli` sandbox run, which does not use it), and the `--prompt-file` candidate over both. This is "the workspace a case gets when its fixture overrides nothing". The throwaway directory is removed afterwards.
- **Each case's own workspace.** The value in a case entry (see [Per-case fields](#per-case-fields-in-agentjson)) is computed from that case's real staged workspace, so a fixture that supplies a `*-conventions` skill, a prompt or a resource is reflected.

Because untracked files and project `*-conventions` skills are never staged, creating, editing or deleting one of them does not change the agent-level hash.

The hash is computed over an ordered sequence of parts:

1. **config** — the raw bytes of the agent config file in the staged `.kiro/agents/<agent>.json`. Its source is `<evals-dir>/agents/<agent>.json` when it exists, else the project's `.kiro/agents/<agent>.json` (in a `kiro-cli` sandbox run only the latter). A project config must be git-tracked to be staged (see [Migration note](#migration-note-staged-kiro-and-provenance)).
2. **prompt** — if the config's `prompt` starts with `file://`, the bytes of that file. A relative path resolves against the config file's directory in the staged `.kiro/agents/`; absolute paths are allowed. A missing or unreadable prompt file is an error. An inline prompt is already covered by the config bytes.
3. **resource** — for each entry of the config's `resources` array, in config order, the bytes of every existing matching file in the staged workspace.

Each part is framed as `<kind>\x00<decimal length>\x00<bytes>` (`kind` is `config`, `prompt` or `resource`), so parts cannot run together ambiguously. File paths are not hashed, so the same contents give the same hash on any machine. The framing is unchanged by the staged-workspace computation: for a tree with no `*-conventions` skills and no untracked files the hash is byte-identical to what was recorded before.

**With `--prompt-file`.** The **prompt** part uses the candidate file's bytes instead of the live prompt file's bytes; the live prompt file is not read, so it does not even need to exist. The **config** and **resource** parts are unchanged. Consequences:

- `prompt_sha256` differs from the baseline hash exactly when the candidate's content differs from the live prompt's content.
- A candidate that is byte-identical to the live prompt gets the baseline `prompt_sha256` (a handy no-op check); `prompt_file` is still recorded, so the run is identifiable as a candidate run.
- The candidate's path is not hashed (paths never are), so the same candidate content gives the same hash wherever the file lives.
- The candidate is staged over the prompt file in every workspace before anything is hashed, so the hash is taken from the very bytes the agent will read, including through a resource entry that points at the prompt file. Every agent `calls[]` record carries the hash of its own case (see [Per-case fields](#per-case-fields-in-agentjson)), which includes the candidate bytes. Judge calls carry no prompt hash.

Consequently, editing the prompt file, the config, or any present resource changes the hash, and so does reordering resources. Running the same inputs twice gives an identical hash, which makes a before/after comparison of two runs a check that only the intended thing changed.

Resource handling:
- Only string entries of `resources` are considered; object entries are ignored.
- A `file://` or `skill://` prefix is stripped. Any other scheme is not a local file and counts as not present.
- Relative paths resolve against the **root of the staged workspace** (the directory that holds the staged `.kiro/`), not against the config file and not against the repository. A resource that is not staged, such as a repository file outside `.kiro/` that no fixture provides, is therefore not present. Absolute paths stay absolute.
- An entry containing `*`, `?` or `[` is expanded as a glob (matches sorted). It counts as present if at least one regular file matches, and every match is hashed.
- **A missing resource is normal, not an error.** Resources such as `skill://.kiro/skills/<agent>-conventions/SKILL.md` are optional per-project overrides. Because a project `*-conventions` skill is never staged, that entry is absent from the agent-level `resources_present` in every run, whether or not the file exists in your checkout, and creating or editing it does not change the agent-level hash. It can only be present when a case's workspace fixture supplies the file, and then only in that case's own `resources_present` and `prompt_sha256`. A missing entry is skipped: it does not contribute to the hash and is omitted from `resources_present`.
- `resources_present` lists entries exactly as written in the config (for example `skill://.kiro/skills/sentinel-protocol/SKILL.md`), in config order.

The agent runs in its case workspace, whose `.kiro/` is staged by the harness (see [Case Workspaces](#case-workspaces)), and does not load relative resources from the repository root. The hash is computed over exactly that staged workspace, so it reflects what the agent loads and not what happens to exist in your checkout. This holds whether or not an `<evals-dir>/agents/` overlay is used.

### Resume refusal

`kairon eval --resume` refuses to continue into a result file that was written under a different prompt or different models, so one `<agent>.json` never mixes two versions. If the existing file has a `prompt_sha256` and that value, its `prompt_file`, its `agent_model` or its `judge_model` differs from what the resumed run is now pinned to, the run stops with an error:

```
❌ cannot resume: prompt or models changed since the interrupted run of architect (recorded agent_model=… judge_model=… prompt_sha256=… prompt_file=…; now agent_model=… judge_model=… prompt_sha256=… prompt_file=…)
```

`prompt_file` is shown as `(none)` when the run used the live prompt. A result file without a `prompt_file` compares equal to "no candidate", so resuming a candidate run without `--prompt-file` (or with a different file, even one that happens to hash identically) is refused, while resuming with the same file works as usual. See [Resume and diff](#resume-and-diff).

The comparison uses the agent-level (baseline) values only; per-case `prompt_sha256` values are not compared. Because the baseline is hashed over the staged workspace, a change to something that is not staged (an untracked file, a project `*-conventions` skill) cannot cause this refusal, while a change to a tracked `.kiro` file or to the `<evals-dir>/agents/` overlay can. An interrupted run that was started before the staging change described in the [migration note](#migration-note-staged-kiro-and-provenance) is refused with this message whenever its recorded hash differs from the new one.

Start a fresh run (without `--resume`) after changing a prompt, a resource or `evals`. A result file written before provenance existed (no `prompt_sha256`) is **refused when it already holds saved cases** — resuming would attribute those scores to the current prompt and models, which were unknown when they were produced; an empty legacy file is allowed. An unchanged resume continues as before.

**Eval suite.** Resume also refuses to continue into a result file that holds saved cases recorded under a different eval suite, because the resumed run would stamp the current `suite_sha256` onto cases that ran under the old one (see [Run coverage](#run-coverage-suite_sha256-complete-and-case_filter)). The comparison is against the `suite_sha256` in the existing `<agent>.json`:

```
❌ cannot resume: eval suite changed since the interrupted run of architect (recorded suite_sha256=…; now …) — resume with the same suite or start a fresh run
```

A file that holds cases but records no `suite_sha256` (written before suite tracking existed) is refused too, since the suite it ran under cannot be verified:

```
❌ cannot resume: architect.json predates eval suite tracking and has 3 saved case(s); the suite they ran under cannot be verified (now suite_sha256=…) — start a fresh run (without --resume)
```

An empty result file is never refused, and nothing is checked for an agent that has no pinned suite (its rubric or cases could not be loaded). Edit a case, the rubric or a fixture, or add a case, and the interrupted run can no longer be resumed: start a fresh one.

**Execution mode.** Resume also refuses to switch between native and `--sandbox`, because scoring `requires_sandbox` cases under the other mode would mark them completed under the wrong execution model. The check is made at two levels:

- per agent, against the `sandbox` boolean in the existing `<agent>.json` (a file that holds saved cases but predates mode tracking is refused, since its mode cannot be verified);
- for the run as a whole, against the `sandbox` string in `summary.json`. This catches a multi-agent run that was interrupted before a later agent's file existed, where there is nothing per-agent to compare. The error names both modes:

```
❌ cannot resume: sandbox mode changed since the interrupted run (summary recorded sandbox=native; now container) — resume with the same mode or start a fresh run
```

Resume with the same mode, or start a fresh run. As a last line of defence, the summary writer also refuses to overwrite a recorded mode with a different one, so one `summary.json` never mixes modes. A `summary.json` with no recorded `sandbox` (an old run) is not mode-checked at this level.

### Migration note: staged `.kiro` and provenance

Two things change when you first run with the staged-workspace behaviour described in [Staged `.kiro` and precedence](#staged-kiro-and-precedence) and [How `prompt_sha256` is computed](#how-prompt_sha256-is-computed):

- **The agent-level `prompt_sha256` changes once.** Previously the hash was taken over the repository working directory, so a project `*-conventions` skill (such as Kairon's own `.kiro/skills/builder-conventions/SKILL.md`) was hashed and listed in `resources_present` even though no case workspace contained it. It is no longer staged, so it is no longer hashed. For any agent whose project override file existed, the first run records a different `prompt_sha256` (and a shorter `resources_present`) than earlier runs, with no change to the prompt. For an agent without such a file the hash is unchanged. Consequences:
  - [`eval diff`](#run-provenance) reports the difference in `prompt_sha256` between a run before and a run after the change. That difference is expected.
  - [`verify-log`](#iteration-logs-and-verify-log) checks a `change_type: prompt` iteration by comparing the recorded hashes of its baseline and result runs. Do not use a pair of runs that straddles this change as a prompt iteration, because the hash differs without a prompt edit. Re-run the baseline under the new behaviour and use that run.
  - `--resume` of a run interrupted before the change is refused (`cannot resume: prompt or models changed …`) when the hash differs. Start a fresh run.
  - After the change, editing a project `*-conventions` file no longer changes the hash, so it can no longer be mistaken for a prompt iteration.
- **A new, untracked config or skill is not staged.** Only files that git tracks under `.kiro/` reach a case. A new agent config, prompt or skill that you have created but not added to git is invisible to the eval, and the run fails with `agent config for "<agent>" not found in the staged .kiro`. An untracked prompt file that a tracked config references fails the run with a prompt read error, and an untracked resource is silently absent from `resources_present`. Run `git add <path>` for the new file before running `kairon eval`. `git add -N <path>` (intent to add) is enough, so you do not have to stage the content or commit it; the file is staged with its current working-tree content. The same applies to a file kept deliberately untracked, such as `.kiro/settings/`: it is never staged, by design.

A `*-conventions` skill is still evaluated when a case needs one: put it in the case's workspace fixture (`<evals-dir>/fixtures/workspaces/<fixture>/.kiro/skills/<skill>-conventions/SKILL.md`) and set `workspace: <fixture>` on the case.

## Evals Directory (`--evals-dir`)

`--evals-dir` selects the directory the harness reads rubrics, cases and fixtures from and writes results to. It defaults to `.kairon/evals`. The layout is the same as the default one, plus an optional `agents/` directory:

```
<evals-dir>/
  rubrics/<agent>.yaml
  cases/<agent>/*.yaml
  fixtures/...                 # files referenced from setup[].path
  agents/                      # optional: agent configs for this eval set
    <agent>.json
    <prompt>.md                # anything the config references via file://./
  results/<timestamp>-<git-short-hash>/
```

- **Setup file rebasing** — a `setup[].path` beginning with `.kairon/evals/` is rebased onto the chosen evals dir when `--evals-dir` is not the default. For example `.kairon/evals/fixtures/selftest-input.md` resolves to `<evals-dir>/fixtures/selftest-input.md`. Other paths are left untouched.
- **Results** — written under `<evals-dir>/results/`; `--resume` and `eval diff` look there too. The sandbox's `.kairon/evals/tmp/...` debug artefacts stay at their fixed location.

### Agent configs: `agents/` precedence

If `<evals-dir>/agents/<agent>.json` exists, the agent under test is run with that config instead of the repository's `.kiro/agents/<agent>.json`. If it does not exist, nothing changes and the repo's `.kiro/agents` is used. The precedence is chosen per agent, so an evals dir may override only some agents.

**How the overlay reaches the agent.** `kiro-cli` only discovers local agents under `<cwd>/.kiro/agents/`. Every case runs in its own workspace (see [Case Workspaces](#case-workspaces)), and the harness stages that workspace's `.kiro/` on the host with the precedence *workspace fixture > `<evals-dir>/agents/` > project `.kiro/`*. The whole `agents/` directory is copied, so `file://./x.md` prompts keep resolving. Consequences:

- The agent's working directory is the case workspace, **not** the repository root. Tools that read files or run shell commands relative to the cwd see the workspace (the project's `.kiro/` plus whatever the case's fixture provides), not the repo. The repository's own `.kiro/` is only read, never modified.
- Only the agent-under-test call is affected; judge calls always run in the normal working directory.
- The `stub` backend does not run an agent, so it ignores `agents/` (it still runs `stub.turns[].commands` in the workspace).
- The temporary-directory overlay in the `kiro-cli` backend (copy `agents/` to `<tmp>/.kiro/agents/` and run there) is now only used by callers that provide no workspace directory; `kairon eval` always provides one.

## Evaluating a candidate prompt (`--prompt-file`)

Iterating on a prompt used to mean editing `.kiro/agents/<agent>-prompt.md`. That edit is live: krew-lead spawns the builder and validator from the worktree's `.kiro/agents/`, so a mid-iteration edit changes the pipeline that is building your PR, and an edited planner prompt degrades the planner you use day to day. `--prompt-file` scores a candidate prompt while the live agent stays untouched.

```bash
kairon eval architect --prompt-file .kairon/iterations/architect/v2.md
```

The candidate is injected **only into the per-case workspaces** the harness already builds for every case (see [Case Workspaces](#case-workspaces)). Nothing in the repository is written: the live `.kiro/agents/` files (config and prompt) are never modified, and neither is the `<evals-dir>/agents/` overlay.

### Where to put candidates

By convention candidates live under `.kairon/iterations/<agent>/` (for example `.kairon/iterations/architect/v2.md`). This is a convention only: `--prompt-file` does not create, require or scan that directory, and any readable file path works. A relative path is resolved against the process working directory (the repository root for a normal run).

The same directory also holds the agent's [iteration logs](#iteration-logs-and-verify-log) (`iteration-NN.md`). `kairon eval verify-log` reads **only** files named `iteration-NN.md` there; candidate prompts such as `v2.md` and any other file coexist with the logs and are ignored.

### What is substituted, and what is not

| | |
|---|---|
| **Substituted** | Only the one file that the agent config's `prompt` field points at (`"prompt": "file://./architect-prompt.md"`), and only in the workspace copy: `<workspace>/.kiro/agents/<ref>`. |
| **Not substituted** | Everything else about the agent is the live agent: the config (model, tools, `allowedTools`, `toolsSettings`), `resources` and skills, and MCP settings. A candidate cannot change them. |

Details of how the workspace copy is written:

- The agent config is located exactly as for [`prompt_sha256`](#how-prompt_sha256-is-computed): `<evals-dir>/agents/<agent>.json` if it exists, else `.kiro/agents/<agent>.json`. The `prompt` reference is resolved relative to the config's directory, which in the workspace is always `<workspace>/.kiro/agents/`. The same helper computes the destination for the hash pre-flight and for staging, so the two cannot disagree.
- The candidate is written after the project `.kiro/` is copied and the `<evals-dir>/agents/` overlay is applied, so it **wins over both and over a workspace fixture** that provides a file at the same path.
- Any existing file, symlink or hard link at the destination is removed first, then the candidate is written as a new file. A link there is replaced, never written through to its target.
- The staged file is then given the same permissions as the rest of the staged `.kiro/` (not group- or other-writable).
- The staged config is not rewritten; only the prompt file's contents change.

The candidate is used for every path that builds case workspaces: a full run, a single case (`--case` or `<agent> <testcase>`), `--resume`, native and `--sandbox`, and every backend. The `stub` backend does not run an agent, so with `--backend stub` the observable effects are the staged file, `prompt_file` and `prompt_sha256` (the pieces you can verify without a model).

### What is recorded

- `prompt_file` is recorded in `<agent>.json`, in `summary.json` under `agents.<agent>`, and at the top level of `summary.json` (a `--prompt-file` run always covers exactly one agent, so the [single-agent rule](#run-summary-summaryjson) is always met). The value is the path as given on the command line, normalised with `filepath.Clean` and `filepath.ToSlash`: `./.kairon/iterations/a/../a/v2.md` is recorded as `.kairon/iterations/a/v2.md`, a relative path stays relative and an absolute path stays absolute. The key is omitted for runs without `--prompt-file`, so their result files are unchanged.
- `prompt_sha256` is computed with the candidate's bytes as the **prompt** part (see [How `prompt_sha256` is computed](#how-prompt_sha256-is-computed)). It differs from the baseline when the candidate's content differs from the live prompt, and equals it for byte-identical content.
- Each agent call record in `calls[]` carries the candidate-based `prompt_sha256` (the shared call-record type has no `prompt_file` field).
- The `🔒 Models:` pre-flight line shows the candidate-based hash prefix.

`prompt_file` is the path text, not a content identity. Two different files can hold the same bytes and one path can hold different bytes over time; compare runs by `prompt_sha256`.

### Rejected combinations

`--prompt-file` is validated before anything else happens: before the backend and evals directory are configured, before the model-pinning pre-flight, before any case runs, and without creating a results directory. A rejected invocation exits non-zero.

| Condition | Error |
|-----------|-------|
| No agent given (`kairon eval --prompt-file x.md`) | `--prompt-file: an agent is required (usage: kairon eval <agent> --prompt-file <path>)` |
| Combined with `--list`, `--perf` or `--cleanup` | `--prompt-file cannot be combined with --list` (or `--perf` / `--cleanup`). Those modes never build a case workspace, so the flag would be silently ignored. |
| File missing or unreadable | `--prompt-file <path>: <OS error>`, for example `no such file or directory` |
| Path is a directory | `--prompt-file <path>: is a directory, want a file` |
| File is empty | `--prompt-file <path>: file is empty` (an empty prompt is almost certainly a mistake) |

Two further conditions depend on the agent's config, so they are found in the pre-flight (together with the model-pinning checks, listed as `agent "<name>": …` in the same `❌ eval model pinning refused the run before any case started` message), still before any case runs:

| Agent config | Error |
|--------------|-------|
| Inline `prompt` (no `file://`) | `--prompt-file: agent config <config> has an inline prompt; only a file:// prompt can be replaced by a candidate` |
| `file://` with nothing after it | `--prompt-file: agent config <config> has an empty file:// prompt reference` |
| Absolute `file://` path | `--prompt-file: agent config <config> references an absolute prompt path "<ref>"; only a path relative to the config can be replaced by a candidate`. It would not resolve inside a workspace or container. |
| Reference that escapes `.kiro/` once cleaned (for example `file://../../x.md`) | `--prompt-file: prompt reference "<ref>" in <config> escapes .kiro/; only a prompt inside .kiro/ can be replaced by a candidate` |
| Reference that does not name a file | `--prompt-file: prompt reference "<ref>" in <config> does not name a file` |

A reference such as `file://../skills/x.md` stays inside `.kiro/` and is accepted.

### Resume and diff

- **`--resume`** refuses a changed `prompt_file` as well as a changed `prompt_sha256`, so a candidate run is never continued without the candidate (or with a different file that happens to hash the same). Resuming with the same file works. A result file without `prompt_file` counts as "no candidate", so a normal resume of an old or non-candidate run is unchanged. See [Resume refusal](#resume-refusal).
- **`eval diff`** shows `prompt_file` differences in the Run Provenance block next to `prompt_sha256`, for example `architect prompt_file: (not recorded) → .kairon/iterations/architect/v2.md`. It only reports; it never fails the diff, and runs written before this field existed show `(not recorded)`. `prompt_file` is read from `summary.json`'s `agents.<agent>`, falling back to `<agent>.json`. See [Run Provenance](#run-provenance).

### Verifying with `--keep-workspaces`

Keep the workspaces and look at the prompt the agent was given:

```bash
kairon eval --backend stub --evals-dir internal/eval/testdata/evals --keep-workspaces \
  selftest --prompt-file /tmp/candidate.md

# workspace_dir is printed on each case line and recorded in selftest.json
diff <workspace_dir>/.kiro/agents/selftest-prompt.md /tmp/candidate.md   # no output: identical
```

Every case's `<workspace_dir>/.kiro/agents/<prompt-ref>` holds the candidate's bytes, while the live `.kiro/agents/` (and `<evals-dir>/agents/`) files are unchanged. To confirm that the live files are untouched, compare `git status --porcelain .kiro/agents` and a checksum of the files before and after the run. Remove the kept workspaces yourself when you are done.

For a no-op check, evaluate a copy of the live prompt: `prompt_sha256` equals the baseline and `prompt_file` is still recorded.

## Iteration Logs and `verify-log`

An iteration log documents one measured change to an agent: baseline, hypothesis, change, results and reasoning. Logs live at `.kairon/iterations/<agent>/iteration-NN.md` (`NN` zero-padded, starting at `00`) and must reference eval runs committed under `<evals-dir>/results/`. `kairon eval verify-log` checks that every claim in a log set matches the recorded results, so a log cannot assert a score nobody measured.

The log format (front-matter keys `iteration`, `date`, `change_type`, `baseline_run`, `result_run`, `baseline_score`, `result_score`; headings `Baseline`, `Hypothesis`, `Change`, `Results`, `Reasoning`) and a complete example are defined in [`.kairon/iterations/README.md`](../.kairon/iterations/README.md).

```bash
kairon eval verify-log <agent> [--iterations-dir DIR] [--min-iterations N] [--evals-dir DIR]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--iterations-dir` | `.kairon/iterations` | Directory holding `<agent>/iteration-NN.md`. |
| `--min-iterations` | `2` | Minimum number of iteration logs. `0` disables the minimum; a negative value is rejected. |
| `--evals-dir` | `.kairon/evals` | Evals directory whose `results/` holds the referenced runs. This is the persistent `kairon eval` flag, inherited by `verify-log` (see [Evals Directory](#evals-directory---evals-dir)). |

The command takes exactly one argument, the agent name (a single path element: no `/`, `\` or `..`). It does not call a model, `kiro-cli` or a container, and it reads only the iterations directory and `<evals-dir>/results/`.

### Verification rules

`verify-log` collects **all** violations instead of stopping at the first. Each is reported as `<file>: <rule>: <message>`, where `<file>` is the log path as joined from `--iterations-dir` (or the agent directory for set-level rules).

| Rule ID | Fails when |
|---------|------------|
| `no-logs` | The agent directory is missing or contains no `iteration-NN.md`. Reported whatever `--min-iterations` is. |
| `front-matter` | Missing or malformed YAML front-matter; a required key is missing; `change_type` is not `prompt` or `eval`; `date` is not a real `YYYY-MM-DD` date; a score is not percent with exactly one decimal in `0.0`-`100.0`; a run name contains `/`, `\` or `..`. |
| `headings` | A required heading (`Baseline`, `Hypothesis`, `Change`, `Results`, `Reasoning`) is missing, duplicated, out of order, or has no content. Headings inside fenced code blocks do not count. |
| `sequence` | File numbers are not exactly `00..N-1` (gap, duplicate, not starting at `00`), the front-matter `iteration` differs from the file's `NN`, or a file such as `iteration-1.md` starts with `iteration-` and ends in `.md` but does not match `iteration-NN.md`. |
| `chain` | For iteration `k > 0`, `baseline_run` is not iteration `k-1`'s `result_run`. |
| `chronology` | Iteration `k`'s `date` is earlier than iteration `k-1`'s. |
| `run-exists` | `baseline_run` or `result_run` is not a directory under `<evals-dir>/results/`. Runs are matched by exact directory name; unlike `eval diff`, a hash-only name is not resolved. |
| `run-data` | A referenced run records no score for the agent, or does not record `prompt_sha256`, `suite_sha256` or `complete` for the agent; one violation per run lists every missing field, for both change types. Fails closed: an unverifiable change is a violation. A run recorded before run coverage existed lacks `suite_sha256` and `complete` and fails here (see the [migration note](#migration-note-run-coverage-and-verify-log)). |
| `run-complete` | A referenced run records `complete: false` for the agent: it is a `--case` run or an interrupted run, and only full runs can be compared. The message names the run, `complete=false` and the recorded `case_filter`. Reported for each such run. A run that records no `complete` at all is `run-data`, not this rule. |
| `run-config` | Both runs exist and differ in `agent_model` or `sandbox` (`native` or `container`), or, for `change_type: prompt` only, in `judge_model`. The message names the field and both values. A fact recorded by one run and not by the other counts as a difference; a fact recorded by neither is equal. A `change_type: eval` iteration may change the judge, so its `judge_model` is not compared here. |
| `score-match` | A claimed score differs from the run's recorded score by more than 0.1 percentage points (exactly 0.1 passes). The message names the field, the claimed value, the recorded value and the run. The recorded score is not rounded before the comparison. |
| `prompt-change` | The prompt relation does not match `change_type`, when both runs record `prompt_sha256` for the agent. `prompt`: the two runs record the same `prompt_sha256`. `eval`: the two runs record different `prompt_sha256` (the prompt must stay the same). |
| `suite-change` | The suite relation does not match `change_type`, when both runs record `suite_sha256` for the agent. `prompt`: the two runs record different `suite_sha256` (the eval suite must stay the same). `eval`: the two runs record the same `suite_sha256` **and** the same `judge_model` (both recorded identical, or both not recorded), so neither the suite nor the judge changed. |
| `min-iterations` | There are fewer iteration logs than `--min-iterations` (default `2`). |

### Where the recorded score comes from

The score recorded for a run is the first of these that is available (all values are compared as **percent**):

1. `summary.json` → `agent_verdicts.<agent>.score`: percent, written since [per-agent pass/fail verdicts](#passfail-verdict-and-exit-status).
2. `summary.json` → `agent_scores.<agent>`: a 0-1 fraction, multiplied by 100. This is the fallback for older runs without verdicts (see [Percent versus fraction](#percent-versus-fraction)).
3. Recomputed from `<agent>.json` (the case scores, using the same arithmetic as the verdict). This is the fallback for runs that have neither summary field.

If none of them is available the run has no recorded score for the agent, which is a `run-data` violation.

`prompt_sha256` is read from `<agent>.json` and, if absent there, from `summary.json` → `agents.<agent>.prompt_sha256` (see [How `prompt_sha256` is computed](#how-prompt_sha256-is-computed)). A `--prompt-file` run hashes the candidate's bytes, so evaluating a changed candidate yields a different hash than the baseline.

The other facts `verify-log` compares are read the same way, field by field: `<agent>.json` first, then `summary.json`. A fact that neither file records is "not recorded".

| Fact | `<agent>.json` | `summary.json` |
|------|----------------|----------------|
| `agent_model` | `agent_model` | `agents.<agent>.agent_model` |
| `judge_model` | `judge_model` | `judge_model` |
| `sandbox` | `sandbox` (boolean: `true` is `container`, `false` is `native`) | `sandbox` (string: `native` or `container`) |
| `suite_sha256`, `complete`, `case_filter` | the same names | `agents.<agent>.<name>` |

### What `change_type` requires

`verify-log` enforces the meaning of `change_type` against the two runs of each iteration. Every iteration needs both runs to be complete full runs (`run-complete`) with the same `agent_model` and `sandbox` (`run-config`). Beyond that:

| `change_type` | `prompt_sha256` | `suite_sha256` | `judge_model` |
|---------------|-----------------|----------------|---------------|
| `prompt` | must differ (`prompt-change`) | must be the same (`suite-change`) | must be the same (`run-config`) |
| `eval` | must be the same (`prompt-change`) | the suite **or** the judge must change: the same `suite_sha256` with the same `judge_model` fails (`suite-change`) | may change; it is how an `eval` iteration changes the judge |

So a `prompt` iteration proves the prompt changed while the suite, the judge, the models and the sandbox mode did not, and an `eval` iteration proves the prompt did not change while the suite or the judge did. A pairwise rule runs whenever both runs record the facts it needs; a missing fact is reported once by `run-data` instead of cascading into the other rules.

### Exit status

| Exit status | Meaning |
|-------------|---------|
| `0` | No violations. Prints `OK <agent>: N iterations verified`. |
| `1` | At least one violation (all are printed after `Error:` on stderr), or an invalid argument such as an agent name containing a path separator. |

Usage text is not printed for a verification failure.

### Migration note: run coverage and `verify-log`

Runs now record `suite_sha256`, `complete` and `case_filter` (see [Run coverage](#run-coverage-suite_sha256-complete-and-case_filter)), and `verify-log` requires them. Consequences for runs and logs that already exist:

- **A run recorded before this change fails `run-data`.** It has no `suite_sha256` and no `complete`, so `verify-log` cannot prove that its suite was full and unchanged. This is deliberate: an unverifiable run is a violation, not a pass. Re-record it by running `kairon eval <agent>` again on the commit you want to measure, commit the new run under `<evals-dir>/results/`, and point `baseline_run` / `result_run` (and the claimed scores) of the iteration logs at the re-recorded runs. Because each iteration's `baseline_run` must equal the previous iteration's `result_run`, re-recording a run that sits in the middle of a chain means updating both logs that reference it.
- **An `eval` iteration now needs `prompt_sha256` on both runs.** It has to prove that the prompt did not change, so a run without a prompt hash fails `run-data` for either `change_type`. Before this change only `change_type: prompt` required it.
- **A `--case` run or an interrupted run cannot be used.** It records `complete: false` and fails `run-complete`. Use a full run.
- **A resume of a result file written before this change is refused** when it holds saved cases (see [Resume refusal](#resume-refusal)). Start a fresh run.

### Verifying the verifier

The verifier ships with fixtures under `internal/eval/testdata/verifylog/`: shared result runs in `evals/results/` and one log set per rule. Every run records the coverage fields except two that are legacy-shaped on purpose: `260630-100000-fffffff` (no coverage fields and no models) and `260705-170000-8888888` (a prompt hash but no `suite_sha256` and no `complete`). Run the passing set with:

```bash
go run ./cmd/kairon eval verify-log selftest \
  --iterations-dir internal/eval/testdata/verifylog/valid \
  --evals-dir internal/eval/testdata/verifylog/evals
```

This exits 0 and prints `OK selftest: 3 iterations verified`. In the `valid` set, iteration 02 is a `change_type: eval` iteration whose runs record the same `prompt_sha256` and different `suite_sha256`. The other sets each violate one rule and exit non-zero, naming the file and the rule. The eight sets for run coverage each hold `iteration-00.md` (a valid `prompt` iteration) and an `iteration-01.md` that is the only one with a violation, so they pass the default `--min-iterations 2`:

| Set | Rule reported |
|-----|---------------|
| `missing-run` | `run-exists` |
| `score-mismatch` | `score-match` |
| `gap` | `sequence` |
| `broken-chain` | `chain` |
| `no-prompt-change` | `prompt-change` |
| `partial-run` | `run-complete`: `prompt` iteration whose result run records `complete: false` and `case_filter: stub-basic` |
| `model-mismatch` | `run-config`: the result run's `agent_model` differs; the message names `agent_model` |
| `sandbox-mismatch` | `run-config`: the result run was `container`, the baseline `native`; the message names `sandbox` |
| `judge-mismatch` | `run-config`: `prompt` iteration whose runs record different `judge_model`; the message names `judge_model` |
| `suite-changed` | `suite-change`: `prompt` iteration whose runs record different `suite_sha256` |
| `eval-prompt-changed` | `prompt-change`: `eval` iteration whose runs record different `prompt_sha256` |
| `eval-nothing-changed` | `suite-change`: `eval` iteration whose runs record the same `prompt_sha256`, `suite_sha256` and `judge_model` |
| `legacy-run` | `run-data`: the result run records no `suite_sha256` and no `complete`; the message names both |

```bash
go run ./cmd/kairon eval verify-log selftest --min-iterations 5 \
  --iterations-dir internal/eval/testdata/verifylog/valid \
  --evals-dir internal/eval/testdata/verifylog/evals   # fails: min-iterations
```

The unit tests are `go test ./internal/eval/ -run VerifyLog` and `go test ./cmd/kairon/cmd/ -run VerifyLog`.

## Prompt Audit (`kairon eval audit`)

`kairon eval audit` is a static check of an agent's prompt, for the maturity-model Stage 3 requirement that prompts are structured and that most of their instructions earn their place. It checks four things:

1. **RTCC structure.** The prompt has the level-2 headings `## Role`, `## Task`, `## Context` and `## Constraints`, in that order.
2. **Load-bearing coverage.** At least **90%** of the prompt's instructions are *load-bearing*: each is mapped, in a hand-written table, to a criterion of the agent's rubric. The Evidence column of that table must name, as `case:<case-name>`, a case of the agent that has a check on the row's criterion, and the audit verifies it. The table format is defined in [`.kairon/iterations/README.md`](../.kairon/iterations/README.md#load-bearing-table).
3. **No duplicates.** An instruction that repeats an earlier one is dead weight and fails the audit, whatever the coverage.
4. **Distinct criteria.** The agent's rubric has at least **3** non-cost criteria, each is backed by at least one check in the agent's cases, and no two of them carry identical checks (see [Distinct criteria](#distinct-criteria)).

The audit is deterministic. It makes no model call, starts no `kiro-cli` and no container, and reads only the agent config, the prompt, the rubric, the agent's cases and the table. It does not edit anything.

```bash
kairon eval audit <agent> [--prompt-file PATH] [--iterations-dir DIR] [--evals-dir DIR]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--prompt-file` | none | Audit this candidate prompt file instead of the agent's own prompt. The table and the rubric still come from the agent. Local to `audit`. |
| `--iterations-dir` | `.kairon/iterations` | Directory holding `<agent>/load-bearing.md`. Local to `audit`. |
| `--evals-dir` | `.kairon/evals` | Evals directory holding `rubrics/<agent>.yaml`, `cases/<agent>/*.yaml` and the optional `agents/<agent>.json` overlay. This is the persistent `kairon eval` flag, inherited by `audit` (see [Evals Directory](#evals-directory---evals-dir)). |

The command takes exactly one argument, the agent name (a single path element: no `/`, `\` or `..`).

### What is audited

- **The prompt.** The file that the agent config's `prompt: file://<path>` points at, resolved relative to the config. The config is `<evals-dir>/agents/<agent>.json` if it exists, else `.kiro/agents/<agent>.json`. An inline prompt or an absolute `file://` path cannot be audited and is an error. With `--prompt-file`, the candidate is audited instead, and the report line reads `Prompt: <path> (candidate)`. The candidate must be a non-empty regular file, as for `kairon eval --prompt-file`.
- **Engine skills.** Every `skill://.kiro/skills/<name>/SKILL.md` entry in the config's `resources` whose name does **not** end in `-conventions` is an engine skill, such as `sentinel-protocol`. Its lines are audited together with the prompt and count toward the 90%. The skill is read from the copy shipped inside the `kairon` binary (`cmd/kairon/templates/kiro/skills/`), so the audit scores what users actually receive. A skill that is not shipped there cannot be audited and fails with the `resource` rule.
- **Override hooks.** A resource whose skill name ends in `-conventions` (for example `builder-conventions`) is a per-project override hook. It is listed in the report, never read and never scored.
- Resources that are not `skill://` strings are listed as ignored and not scored. A skill that appears twice is processed once.
- **The rubric.** `<evals-dir>/rubrics/<agent>.yaml` is read for each criterion's `name` and `type`. The `type` is what tells a cost criterion (`type: cost`) from a non-cost one, which the [Distinct criteria](#distinct-criteria) rules need. Only the audit reads it this way: the eval runner's own rubric loading is unchanged, so a rubric with fewer than three criteria still runs under `kairon eval`.
- **The agent's cases.** `<evals-dir>/cases/<agent>/*.yaml` (non-recursive, `.yaml` only) are read to verify the Evidence column and the Distinct criteria rules. A case is identified by its `name:` field and counts as a check on a criterion when one of its `checks:` entries has that `criterion`. Besides `name:` and each check's `criterion`, the audit also reads the rest of every check (its `type` and all its other fields) so it can compare checks for equality. It ignores every other field of the case, runs nothing and does not use the eval runner's case loader. A missing `cases/<agent>/` directory means the agent has no cases. A case file that is not valid YAML or has no `name:` is a plain error, like a bad rubric.

### The RTCC rule

The prompt, but not the skills, must contain the four headings `## Role`, `## Task`, `## Context` and `## Constraints`. Names are compared exactly and case-sensitively, so `## role` and `### Role` do not count. A heading inside a fenced code block does not count. Other headings may appear between the four. If the first occurrences are not in the order Role, Task, Context, Constraints, the prompt fails.

### What counts as an instruction

Every physical line of the prompt and of each audited skill that is non-blank after trimming, outside a fenced code block (backtick or tilde fences), not an ATX heading and not a table separator row. A leading YAML front-matter block is skipped, so a `SKILL.md` does not need rows for its metadata. Each occurrence counts separately in `total` and `mapped`; a repeat is additionally a `duplicate` violation (see below). An instruction is mapped when its trimmed text equals an `Instruction` cell of the table exactly, and the row's `Criterion` is a criterion `name` in `<evals-dir>/rubrics/<agent>.yaml`. The full rules, including escaped pipes, are in [`.kairon/iterations/README.md`](../.kairon/iterations/README.md#load-bearing-table).

The score is `mapped / total` over the prompt plus the audited engine skills. The audit passes when `mapped * 100 >= 90 * total`, computed in integers. Exactly 90% passes, 89% fails, and 899 of 1000 (shown as `89.9%`) fails. A percentage is shown truncated to one decimal and never rounded up. If there are no instructions at all, the audit fails.

### The Evidence column

Every data row of the table (one with a non-empty `Instruction` and `Criterion`) must name, in its `Evidence` cell, at least one case of the agent as `case:<case-name>`. Other text in the cell is allowed. Each named case is verified:

- it exists: a file in `<evals-dir>/cases/<agent>/` has `name: <case-name>`, and
- it has at least one check (any type) whose `criterion` equals the row's `Criterion`.

The token must start the cell or follow a character that is not a letter, digit or `_` (so `showcase:x` does not count). A name is letters, digits, `_`, `.` and `-`, matched exactly and case-sensitively, and a trailing `.` or `-` is dropped (`case:basic.` names `basic`). A cell may name several cases and each is verified once. A criterion that is scored only by the LLM judge, with no entry under `checks:`, does not prove an instruction. When the row's criterion is not in the rubric the "no check" test is skipped, because the row is already an `unknown-criterion` problem. The format and examples are in [`.kairon/iterations/README.md`](../.kairon/iterations/README.md#evidence-format).

### Duplicates

An instruction whose trimmed text equals an earlier instruction is dead weight (Stage 3 requires no dead weight). "Earlier" means earlier in the prompt, then in the audited engine skills in the order they appear in the report. Each repeat is a `duplicate` violation at the repeat's `file:line` that names the **first** occurrence, so three copies give two violations that both name the first. The check is on the exact trimmed text and works across files (prompt against skill, skill against skill). It also flags short repeated lines such as `---` outside front-matter; reword or drop them. Duplicates are always reported, even at 100% coverage. Counting is unchanged (`total` and `mapped` include every occurrence); the number of duplicate violations is printed as `duplicates=` in the report.

### Distinct criteria

Maturity-model Stage 3 asks for at least three distinct evaluation criteria, each backed by a substantive check. The audit enforces that on the agent's rubric and cases with three rules. Like `duplicate`, they are always reported, whatever the coverage, and they do not change `total`, `mapped` or the coverage percentage.

**Non-cost criteria.** A criterion is *non-cost* when its `type` is not `cost`. Only non-cost criteria count: a `type: cost` criterion is not counted toward the minimum and is never reported as `unchecked` or `identical`. Criteria are de-duplicated by `name` (the first occurrence wins) and a criterion with an empty name is ignored. A check on a criterion that is not in the rubric, or on a `type: cost` criterion, satisfies none of these rules.

**A check on a criterion.** An entry in a case's `checks:` list whose `criterion` equals the criterion's name. The audit counts the checks, and the distinct cases that carry at least one, per criterion (the `checks=` and `cases=` values of the [report](#report-format)).

The rules:

- **`rubric`: at least 3 non-cost criteria.** A rubric with fewer than three distinct non-cost criteria fails. The violation is attributed to the rubric file (`<evals-dir>/rubrics/<agent>.yaml`, no line number) and states how many were found and which. Two non-cost criteria plus a `type: cost` criterion is still a violation.
- **`unchecked`: every non-cost criterion has a check.** A non-cost criterion with no check in any of the agent's cases fails, once per criterion. The violation names the criterion and the cases directory that was searched. An agent with no cases directory therefore has every non-cost criterion `unchecked`. A criterion scored only by the LLM judge, with no entry under `checks:`, counts as unchecked.
- **`identical`: no two criteria carry the same checks.** Two non-cost criteria fail when, in **every** case of the agent, they have the same checks. Both must have at least one check: two criteria with no checks are `unchecked`, not `identical`. A group of more than two criteria gives one violation per criterion after the first, each naming itself and the first criterion of the group (as `duplicate` names the first occurrence), so a pair gives exactly one violation that names both.

**What "the same checks" means.** For criteria A and B, and for every case of the agent (cases are keyed by `name:`; two files with the same name are merged), the multiset of A's checks equals the multiset of B's checks. A check is compared by its full contents **except the `criterion` field**: the same `type` and the same other fields with the same values. Consequences:

- The order of the YAML fields in a check does not matter, and neither does the order of the checks inside a case. `allow: [README.md]` and `allow: ["README.md"]` are equal.
- A check listed twice for one criterion and once for the other differs (multiset, not set).
- Criteria checked in a different set of cases are not identical, even if every check body is equal. A criterion that has checks in a case where the other has none is also not identical.
- No defaults are applied. `expect_exit: 0` written explicitly and `expect_exit` omitted are different checks.

**No semantic judging.** "Distinct" is structural. The audit compares the text of the checks, never their meaning: two criteria whose check bodies differ in any field pass, even if they test the same thing in practice. Whether a check is substantive is for the author and reviewer to judge.

**Plain error versus violation.** A rubric with **no** criteria at all still stops the audit with a plain error (`rubric <path> has no criteria`), as before. A rubric with one or two non-cost criteria is not an error: it is reported as a `rubric` violation together with the other violations of the run.

### Rule IDs

Every violation is printed as `<file>[:<line>]: <rule>: <message>`. The audit collects all of them instead of stopping at the first.

| Rule ID | Fails when |
|---------|------------|
| `structure` | A required RTCC heading is missing (one violation per missing heading, for example `missing required heading "## Context"`), or the headings are out of order (`required headings out of order: want Role, Task, Context, Constraints; found …`). |
| `resource` | An engine skill listed in `resources` is not shipped under `cmd/kairon/templates/kiro/skills/`, or a `skill://` resource does not name a single skill directory. |
| `table` | `<iterations-dir>/<agent>/load-bearing.md` is missing or unreadable, has no `\| Instruction \| Criterion \| Evidence \|` header row, has a row without exactly three cells or with an empty `Instruction` or `Criterion` cell, or maps one instruction to two different criteria. Also, for every row with an `Instruction` and a `Criterion`, when the `Evidence` cell: names no `case:<case-name>` (`row <N>: Evidence must name at least one case of agent "<agent>" as case:<case-name>`); names a case that is not in `<evals-dir>/cases/<agent>/` (`row <N>: evidence names case "<name>" which is not a case of agent "<agent>" (looked in <cases-dir>)`); or names a case with no check on the row's criterion (`row <N>: case "<name>" has no check on criterion "<criterion>"`). Reported at the table line. |
| `duplicate` | An instruction repeats an earlier one in the prompt or the audited skills (`duplicate of <first file>:<first line>: "<text>"`). Reported at the repeat's `<file>:<line>`; every repeat names the first occurrence. |
| `unmapped` | An instruction (reported at `<file>:<line>`) is not in the table. |
| `unknown-criterion` | An instruction's row names a criterion that is not in the rubric. The instruction does not count as mapped. |
| `coverage` | Fewer than 90% of the instructions are mapped, or there are no instructions to audit. |
| `rubric` | The rubric has fewer than 3 distinct non-cost criteria (`rubric <path> has <N> distinct non-cost criteria (<names>); need at least 3`). A `type: cost` criterion is not counted. Reported at the rubric file, with no line. |
| `unchecked` | A non-cost criterion has no check in any of the agent's cases (`criterion "<name>" has no check in any case (looked in <cases-dir>)`). Reported at the rubric file, once per criterion; cost criteria are never reported. |
| `identical` | Two non-cost criteria have the same checks, ignoring the `criterion` field, in every case of the agent (`criterion "<name>" has the same checks as criterion "<first>" in every case (looked in <cases-dir>)`). Reported at the rubric file; each criterion after the first of a group gets one violation naming the first. See [Distinct criteria](#distinct-criteria). |

`unmapped` and `unknown-criterion` are the per-line detail of a coverage failure, so they are reported only when coverage is below 90%. A prompt at or above the threshold passes even if some of its lines are unmapped. The `structure`, `resource`, `table` (including the Evidence problems), `duplicate`, `rubric`, `unchecked` and `identical` violations are always reported.

Problems that stop the audit from running at all are plain errors, not rule violations: an invalid agent name, a missing agent config, an unreadable or empty `--prompt-file`, a rubric that is missing, invalid or has no criteria at all, or a case file in `cases/<agent>/` that is not valid YAML or has no `name:`. A rubric with only one or two non-cost criteria is **not** a plain error: it is a `rubric` violation.

### Report format

The report goes to stdout, whether the audit passes or fails:

```
Audit: <agent>
Prompt: <prompt path>[ (candidate)]
Audited resources:
  skill <name> (<N> instructions)
Override hooks (excluded from score):
  skill <name>
Ignored resources:
  <resource>
Instructions: total=<T> mapped=<M> duplicates=<D> coverage=<P>% (threshold 90%)
Criteria: non-cost=<N> (need at least 3)
  <criterion>: checks=<C> cases=<K>
```

The `Audited resources:`, `Override hooks (excluded from score):` and `Ignored resources:` blocks appear only when they have entries. `duplicates=<D>` is the number of `duplicate` violations, that is, the occurrences after the first of each repeated instruction; it is `0` for a clean prompt.

The `Criteria:` block follows the `Instructions:` line and is printed whether the audit passes or fails. `non-cost=<N>` is the number of distinct non-cost criteria of the rubric, and there is one indented line per such criterion, in rubric order, with `checks=<C>` (the number of checks on it across all of the agent's cases) and `cases=<K>` (the number of distinct cases with at least one check on it). `checks` and `cases` differ when a case carries several checks on one criterion. Cost criteria are not listed. On success one more line follows:

```
OK <agent>: <T> instructions, <P>% load-bearing, RTCC structure present
```

On failure the violations are returned as the error, so they are printed on stderr after `Error:`, one per line, ordered by file (prompt, then skills, then the table, then the rubric, then the agent config), then by line, then by rule. Usage text is not printed.

The `Error:` block is printed **twice** on stderr, one copy after the other. This is not specific to the audit: the command framework prints a returned error and `main` prints it again. Run through `go run`, as in the examples below, `go run` then adds its own `exit status 1` line, so the last stderr line of a failing run is `exit status 1`. That line comes from `go run`, not from `kairon`; the built `kairon` binary does not print it. The examples below show the captured output as it is, stdout first and then stderr.

### Exit status

| Exit status | Meaning |
|-------------|---------|
| `0` | No violations. The last line on stdout is `OK <agent>: …`. |
| `1` | At least one violation, or any other error (invalid argument, missing config or rubric, unreadable `--prompt-file`). |

### Running it on the real agents

Prompts are not edited by the audit feature and no `load-bearing.md` ships yet, so the real agents are expected to **fail** until their tables are written. The shipped rubrics and cases are not changed by the Distinct criteria rules either, so a real agent can also fail those: the shipped `builder` cases carry no `checks:` at all. For example `kairon eval audit builder` lists the engine skill and the hook, reports every non-cost criterion with `checks=0 cases=0`, and then fails:

```
Audit: builder
Prompt: .kiro/agents/builder-prompt.md
Audited resources:
  skill sentinel-protocol (36 instructions)
Override hooks (excluded from score):
  skill builder-conventions
Instructions: total=63 mapped=0 duplicates=0 coverage=0.0% (threshold 90%)
Criteria: non-cost=4 (need at least 3)
  code_correctness: checks=0 cases=0
  spec_adherence: checks=0 cases=0
  code_quality: checks=0 cases=0
  test_coverage: checks=0 cases=0
```

followed on stderr (the `Error:` block, printed twice, then `go run`'s `exit status 1`) by 4 `structure` violations (`## Role`, `## Task`, `## Context`, `## Constraints` missing), 27 `unmapped` violations for the prompt, 36 for the engine skill, `.kairon/iterations/builder/load-bearing.md: table: load-bearing table not found`, `.kairon/iterations/builder/load-bearing.md: coverage: 0.0% of instructions are load-bearing; need at least 90%` and one `.kairon/evals/rubrics/builder.yaml: unchecked: criterion "<name>" has no check in any case (looked in .kairon/evals/cases/builder)` line per criterion above. The planner prompt has none of the RTCC headings, so `kairon eval audit planner` exits `1` and names `## Role`, `## Task`, `## Context` and `## Constraints` as missing.

### Verifying the audit

The audit ships with fixtures under `internal/eval/testdata/audit/`. Most case directories hold an agent `demo` (config and prompt in `evals/agents/`, rubric in `evals/rubrics/demo.yaml` with the criteria `alpha`, `beta` and `gamma`, the case `basic` in `evals/cases/demo/basic.yaml` with one check on each criterion, each with a different `allow` list so the criteria are distinct) and its table in `iterations/demo/load-bearing.md`, whose Evidence cells are all `case:basic`. The table maps instructions to `alpha` and `beta` only; `gamma` is there for the Distinct criteria rules. The fixture prompts have no engine skills, so the commands do not depend on the shipped templates.

**Exactly 90% passes.** The `coverage-90` prompt has 10 instructions and the table maps 9 of them:

```bash
go run ./cmd/kairon eval audit demo \
  --evals-dir internal/eval/testdata/audit/coverage-90/evals \
  --iterations-dir internal/eval/testdata/audit/coverage-90/iterations; echo "exit=$?"
```

```
Audit: demo
Prompt: internal/eval/testdata/audit/coverage-90/evals/agents/demo-prompt.md
Instructions: total=10 mapped=9 duplicates=0 coverage=90.0% (threshold 90%)
Criteria: non-cost=3 (need at least 3)
  alpha: checks=1 cases=1
  beta: checks=1 cases=1
  gamma: checks=1 cases=1
OK demo: 10 instructions, 90.0% load-bearing, RTCC structure present
exit=0
```

**89% fails.** The `coverage-89` prompt has 100 instructions and the table maps 89 of them (`Instruction 001.` to `Instruction 089.`):

```bash
go run ./cmd/kairon eval audit demo \
  --evals-dir internal/eval/testdata/audit/coverage-89/evals \
  --iterations-dir internal/eval/testdata/audit/coverage-89/iterations; echo "exit=$?"
```

```
Audit: demo
Prompt: internal/eval/testdata/audit/coverage-89/evals/agents/demo-prompt.md
Instructions: total=100 mapped=89 duplicates=0 coverage=89.0% (threshold 90%)
Criteria: non-cost=3 (need at least 3)
  alpha: checks=1 cases=1
  beta: checks=1 cases=1
  gamma: checks=1 cases=1
Error: internal/eval/testdata/audit/coverage-89/evals/agents/demo-prompt.md:94: unmapped: instruction not in load-bearing table: "Instruction 090."
internal/eval/testdata/audit/coverage-89/evals/agents/demo-prompt.md:95: unmapped: instruction not in load-bearing table: "Instruction 091."
…
internal/eval/testdata/audit/coverage-89/evals/agents/demo-prompt.md:104: unmapped: instruction not in load-bearing table: "Instruction 100."
internal/eval/testdata/audit/coverage-89/iterations/demo/load-bearing.md: coverage: 89.0% of instructions are load-bearing; need at least 90%
Error: internal/eval/testdata/audit/coverage-89/evals/agents/demo-prompt.md:94: unmapped: instruction not in load-bearing table: "Instruction 090."
internal/eval/testdata/audit/coverage-89/evals/agents/demo-prompt.md:95: unmapped: instruction not in load-bearing table: "Instruction 091."
…
internal/eval/testdata/audit/coverage-89/evals/agents/demo-prompt.md:104: unmapped: instruction not in load-bearing table: "Instruction 100."
internal/eval/testdata/audit/coverage-89/iterations/demo/load-bearing.md: coverage: 89.0% of instructions are load-bearing; need at least 90%
exit status 1
exit=1
```

(The `…` stands for the lines for `Instruction 092.` to `Instruction 099.`, which follow the same form, in each copy of the `Error:` block. The `Error:` blocks go to stderr and the report to stdout. See the note under [Report format](#report-format) for why the block appears twice and where `exit status 1` comes from.)

**Evidence and duplicate fixtures.** Four more fixtures use a **flat** layout, so one directory is both the evals directory and the iterations directory. Each holds the agent `selftest` (`agents/selftest.json` and `agents/selftest-prompt.md`), its rubric (`rubrics/selftest.yaml`, criteria `alpha`, `beta` and `gamma`), its cases (`cases/selftest/`) and its table (`selftest/load-bearing.md`). Each must fail, and each is run the same way:

```bash
go run ./cmd/kairon eval audit selftest \
  --iterations-dir internal/eval/testdata/audit/<fixture> \
  --evals-dir internal/eval/testdata/audit/<fixture>; echo "exit=$?"
```

| Fixture | What is wrong | Violation (rule `table` or `duplicate`) |
|---------|---------------|------------------------------------------|
| `evidence-missing` | Row 3 has an empty Evidence cell and row 4 has text with no `case:` token | `…/selftest/load-bearing.md:3: table: row 3: Evidence must name at least one case of agent "selftest" as case:<case-name>` and the same for line 4 |
| `evidence-unknown-case` | Row 4 names `case:ghost`, which does not exist | `…/selftest/load-bearing.md:4: table: row 4: evidence names case "ghost" which is not a case of agent "selftest" (looked in internal/eval/testdata/audit/evidence-unknown-case/cases/selftest)` |
| `evidence-unchecked-criterion` | Row 5 maps to criterion `beta` but names `case:alpha-only`, which only has a check on `alpha` | `…/selftest/load-bearing.md:5: table: row 5: case "alpha-only" has no check on criterion "beta"` |
| `duplicate-line` | The prompt repeats `Never skip validation.` on lines 8 and 9 | `…/agents/selftest-prompt.md:9: duplicate: duplicate of …/agents/selftest-prompt.md:8: "Never skip validation."` |

For example `duplicate-line` prints the report on stdout and the violation on stderr, and exits `1`:

```
Audit: selftest
Prompt: internal/eval/testdata/audit/duplicate-line/agents/selftest-prompt.md
Instructions: total=5 mapped=5 duplicates=1 coverage=100.0% (threshold 90%)
Criteria: non-cost=3 (need at least 3)
  alpha: checks=1 cases=1
  beta: checks=1 cases=1
  gamma: checks=1 cases=1
Error: internal/eval/testdata/audit/duplicate-line/agents/selftest-prompt.md:9: duplicate: duplicate of internal/eval/testdata/audit/duplicate-line/agents/selftest-prompt.md:8: "Never skip validation."
Error: internal/eval/testdata/audit/duplicate-line/agents/selftest-prompt.md:9: duplicate: duplicate of internal/eval/testdata/audit/duplicate-line/agents/selftest-prompt.md:8: "Never skip validation."
exit status 1
exit=1
```

The Evidence fixtures also reach 100% coverage (`total=4 mapped=4 duplicates=0 coverage=100.0%`), so they fail only because of the `table` violations, which are always reported. `evidence-unchecked-criterion` has a second case, `alpha-only`, so its `Criteria:` block reports `alpha: checks=2 cases=2`. The `fence-and-table` fixture is the passing counterpart of the README example and exits `0`:

```bash
go run ./cmd/kairon eval audit demo \
  --evals-dir internal/eval/testdata/audit/fence-and-table/evals \
  --iterations-dir internal/eval/testdata/audit/fence-and-table/iterations; echo "exit=$?"
```

```
Audit: demo
Prompt: internal/eval/testdata/audit/fence-and-table/evals/agents/demo-prompt.md
Instructions: total=5 mapped=5 duplicates=0 coverage=100.0% (threshold 90%)
Criteria: non-cost=3 (need at least 3)
  alpha: checks=1 cases=1
  beta: checks=1 cases=1
  gamma: checks=1 cases=1
OK demo: 5 instructions, 100.0% load-bearing, RTCC structure present
exit=0
```

**Distinct-criteria fixtures.** Four more flat-layout fixtures (agent `selftest`, same layout as above) exercise the [Distinct criteria](#distinct-criteria) rules. Each prompt has four instructions and its table maps all four, so coverage is 100% (`total=4 mapped=4 duplicates=0`) and the only failure is the rule under test. Each is run the same way:

```bash
go run ./cmd/kairon eval audit selftest \
  --iterations-dir internal/eval/testdata/audit/<fixture> \
  --evals-dir internal/eval/testdata/audit/<fixture>; echo $?
```

| Fixture | What it holds | Result |
|---------|---------------|--------|
| `two-criteria` | Rubric `alpha`, `beta` and a `type: cost` criterion `cost`; case `basic` checks `alpha` and `beta` | exit `1`; `…/two-criteria/rubrics/selftest.yaml: rubric: rubric …/two-criteria/rubrics/selftest.yaml has 2 distinct non-cost criteria (alpha, beta); need at least 3` |
| `unchecked-criterion` | Rubric `alpha`, `beta`, `gamma`; case `basic` checks `alpha` and `beta` only | exit `1`; `…/unchecked-criterion/rubrics/selftest.yaml: unchecked: criterion "gamma" has no check in any case (looked in …/unchecked-criterion/cases/selftest)` |
| `identical-criteria` | Rubric `alpha`, `beta`, `gamma`; case `basic` gives `beta` and `gamma` the same check (`type: changed_files`, `allow: [shared.md]`) and `alpha` a different one | exit `1`; `…/identical-criteria/rubrics/selftest.yaml: identical: criterion "gamma" has the same checks as criterion "beta" in every case (looked in …/identical-criteria/cases/selftest)` |
| `three-criteria-ok` | Rubric `alpha`, `beta`, `gamma` and a `type: cost` criterion `cost`; case `basic` has two checks on `alpha` and one each on `beta` and `gamma`; case `second` has one more check on `alpha` | exit `0`; the `Criteria:` block lists `alpha: checks=3 cases=2`, `beta: checks=1 cases=1`, `gamma: checks=1 cases=1` and no line for `cost` |

**Fewer than three non-cost criteria** (`two-criteria`). The `cost` criterion is not counted, so the rubric has two. The cost criterion has no checks and is not reported as `unchecked`:

```bash
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/two-criteria --evals-dir internal/eval/testdata/audit/two-criteria; echo $?
```

```
Audit: selftest
Prompt: internal/eval/testdata/audit/two-criteria/agents/selftest-prompt.md
Instructions: total=4 mapped=4 duplicates=0 coverage=100.0% (threshold 90%)
Criteria: non-cost=2 (need at least 3)
  alpha: checks=1 cases=1
  beta: checks=1 cases=1
Error: internal/eval/testdata/audit/two-criteria/rubrics/selftest.yaml: rubric: rubric internal/eval/testdata/audit/two-criteria/rubrics/selftest.yaml has 2 distinct non-cost criteria (alpha, beta); need at least 3
Error: internal/eval/testdata/audit/two-criteria/rubrics/selftest.yaml: rubric: rubric internal/eval/testdata/audit/two-criteria/rubrics/selftest.yaml has 2 distinct non-cost criteria (alpha, beta); need at least 3
exit status 1
1
```

**A criterion without a check** (`unchecked-criterion`):

```bash
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/unchecked-criterion --evals-dir internal/eval/testdata/audit/unchecked-criterion; echo $?
```

```
Audit: selftest
Prompt: internal/eval/testdata/audit/unchecked-criterion/agents/selftest-prompt.md
Instructions: total=4 mapped=4 duplicates=0 coverage=100.0% (threshold 90%)
Criteria: non-cost=3 (need at least 3)
  alpha: checks=1 cases=1
  beta: checks=1 cases=1
  gamma: checks=0 cases=0
Error: internal/eval/testdata/audit/unchecked-criterion/rubrics/selftest.yaml: unchecked: criterion "gamma" has no check in any case (looked in internal/eval/testdata/audit/unchecked-criterion/cases/selftest)
Error: internal/eval/testdata/audit/unchecked-criterion/rubrics/selftest.yaml: unchecked: criterion "gamma" has no check in any case (looked in internal/eval/testdata/audit/unchecked-criterion/cases/selftest)
exit status 1
1
```

**Two criteria with identical checks** (`identical-criteria`). The violation is reported on the later criterion and names the first one of the pair:

```bash
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/identical-criteria --evals-dir internal/eval/testdata/audit/identical-criteria; echo $?
```

```
Audit: selftest
Prompt: internal/eval/testdata/audit/identical-criteria/agents/selftest-prompt.md
Instructions: total=4 mapped=4 duplicates=0 coverage=100.0% (threshold 90%)
Criteria: non-cost=3 (need at least 3)
  alpha: checks=1 cases=1
  beta: checks=1 cases=1
  gamma: checks=1 cases=1
Error: internal/eval/testdata/audit/identical-criteria/rubrics/selftest.yaml: identical: criterion "gamma" has the same checks as criterion "beta" in every case (looked in internal/eval/testdata/audit/identical-criteria/cases/selftest)
Error: internal/eval/testdata/audit/identical-criteria/rubrics/selftest.yaml: identical: criterion "gamma" has the same checks as criterion "beta" in every case (looked in internal/eval/testdata/audit/identical-criteria/cases/selftest)
exit status 1
1
```

**Three distinct, checked criteria** (`three-criteria-ok`). `alpha` has three checks spread over two cases, which shows that `checks` and `cases` count different things. The `cost` criterion is not listed:

```bash
go run ./cmd/kairon eval audit selftest --iterations-dir internal/eval/testdata/audit/three-criteria-ok --evals-dir internal/eval/testdata/audit/three-criteria-ok; echo $?
```

```
Audit: selftest
Prompt: internal/eval/testdata/audit/three-criteria-ok/agents/selftest-prompt.md
Instructions: total=4 mapped=4 duplicates=0 coverage=100.0% (threshold 90%)
Criteria: non-cost=3 (need at least 3)
  alpha: checks=3 cases=2
  beta: checks=1 cases=1
  gamma: checks=1 cases=1
OK selftest: 4 instructions, 100.0% load-bearing, RTCC structure present
0
```

The other fixture directories each isolate one rule: `no-rtcc`, `partial` and `wrong-order` (the `structure` rule), `unmapped` (both `unmapped` and `unknown-criterion`), `ok-hooks` (an engine skill audited and a hook excluded, library tests only, because it needs its own skills directory), `candidate` (`--prompt-file`) and `fence-and-table` (extraction and escaped pipes). The unit tests are `go test ./internal/eval/ -run Audit` and `go test ./cmd/kairon/cmd/ -run Audit`.

## Case Workspaces

Every case runs in its own **workspace**, built on the host by the harness. The native and `--sandbox` paths share the same builder, so a case sees the same layout either way. Nothing runs in the repository root, and the live repository is never modified by a case.

A workspace is a temporary directory laid out like this:

```
<workspace-parent>/     # private (0700) parent, removed with the workspace
  bin/gh                # the fake gh (harness-owned; mounted read-only at /opt/kairon/bin under --sandbox)
  bin/<command>         # one executable per case `mocks:` entry, staged next to the fake gh (same mount)
  ws/                   # the workspace proper (<workspace> below)
    <fixture files>     # contents of fixtures/workspaces/<name>/ (empty by default)
    .kiro/              # staged agent and skill configuration (harness-owned)
    .eval/              # outputs directory (harness-owned, created empty)
```

The tree below `ws/` is the git repo. `bin/` sits beside it, outside the repo and outside the agent's writable mounts, so an agent cannot modify the fake `gh` or a case's mocks. `bin/` holds the fake `gh` and any mocks the case declares with `mocks:` (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking)).

How it is built, in order:

1. A private (`0700`) parent directory is created under `$KAIRON_EVAL_WORKSPACE_ROOT` or, when unset, the OS temp directory, and symlinks in the path are resolved (macOS `/var` is really `/private/var`, and Docker Desktop and Podman machine only share real paths). Set `KAIRON_EVAL_WORKSPACE_ROOT` when your `TMPDIR` is not on a path your container runtime can mount.
2. The case's fixture, `<evals-dir>/fixtures/workspaces/<name>/`, is copied in (regular files and directories only; symlinks and `.git` are skipped). A case without `workspace:` gets an empty workspace.
3. `git init -b main` and a single commit of the fixture (`--allow-empty` for the default workspace). The commit is hermetic: fixed identity and date, no hooks, no signing, and no user or system git config. **That commit is the only commit and its tree is the fixture**, so `git status --porcelain` afterwards shows exactly what the agent (or the stub's `commands`) changed.
4. `.eval/` and `.kiro/` are added to `.git/info/exclude` (not to a tracked file), so harness-owned paths never appear in `git status`. Files the fixture itself tracks under `.kiro/` stay tracked.
5. `.kiro/` is staged (below) and an empty `.eval/` is created. If the case defines `gh_issue`, the host also renders `.eval/gh-issue.json` and `.eval/gh-issue.txt` for the fake `gh` (see [The fake `gh`](#the-fake-gh)); a case without `gh_issue` gets neither file. The fake `gh` script itself is written to `<workspace-parent>/bin/gh` (mode `0755`), next to the workspace rather than inside it; each of the case's `mocks:` is written beside it as `<workspace-parent>/bin/<command>` (mode `0755`), right after the fake `gh`.
6. Permissions are opened so the unprivileged container user can use the tree whatever its host owner: `a+rwX` on everything including `.git`, and `a+rX` (read-only) on `.kiro/`.

### Staged `.kiro` and precedence

The workspace's `.kiro/` is assembled on the host, never copied into a running container. Precedence, highest first:

1. a file provided by the workspace fixture itself,
2. `<evals-dir>/agents/*` (copied to `.kiro/agents/`),
3. the project's own `.kiro/` (agents, skills, MCP config, as set up by `kairon init`), **limited as described below**.

The project's `.kiro/` is only read. The live repository's `.kiro/` is never exposed to the agent. The two filters below apply to the project layer only; the `<evals-dir>/agents/` overlay and the fixture layer are copied as they are, so a fixture can still provide or override any file.

- **Only git-tracked files are staged.** The harness asks git which files under `.kiro/` are tracked (`git ls-files --cached -- .kiro`, run in the working directory) and stages only those. Untracked local state, such as the gitignored `.kiro/settings/`, never reaches a case, so a result does not depend on what happens to sit on the machine that ran it. A tracked file is staged with its **current working-tree content**, so uncommitted edits to tracked files are still staged. A file added with `git add` (or `git add -N`) counts as tracked; a tracked file that was deleted from the working tree is skipped because it is not on disk. If the working directory is not inside a git repository there are no untracked files by definition, so the whole project `.kiro/` is staged (the conventions filter below still applies). Any other git failure is an error, so the filter never silently degrades.
- **`*-conventions` skills are staged only from a case workspace fixture.** A project skill directory `.kiro/skills/<name>-conventions/` (for example Kairon's own `builder-conventions`, `planner-conventions` and `validator-conventions`) is a per-project override hook. It is never staged from the project, so an agent is evaluated with its override hooks **empty**. A case whose fixture provides `.kiro/skills/<name>-conventions/...` does get it, with exactly the fixture's files: the project's files in that directory are never merged in. This keeps an agent certified against its own prompt and engine skills instead of this repository's local overrides.

Because the staged workspace is the single source of truth, [`prompt_sha256` and `resources_present`](#how-prompt_sha256-is-computed) are computed from it as well, never from the repository working directory.

### Workspace fixtures

Fixtures live in `<evals-dir>/fixtures/workspaces/<name>/` and are referenced by a case's `workspace:` field:

```yaml
name: stub-seeded-workspace
agent: selftest
workspace: seeded            # => fixtures/workspaces/seeded/
input: |
  Append a line to the seeded README and report that you did.
stub:
  turns:
    - commands:
        - "echo 'appended by the stub' >> README.md"
      response: |
        ## Seeded workspace
        ### Details
```

The name must match `^[A-Za-z0-9._-]+$` and not be `.` or `..`, and the directory must exist. Both are checked when cases are loaded. After this case runs, `git status --porcelain` in the kept workspace is ` M README.md`.

Fixtures under `fixtures/workspaces/` and `fixtures/hidden/` are excluded from the `task sync:check` comparison of `.kairon/evals` against the shipped templates.

### Keeping workspaces and `workspace_dir`

By default a workspace is deleted once its case has been scored. With `--keep-workspaces` it is kept so you can inspect the result (`git -C <workspace> status`, `ls <workspace>/.eval`). Either way, each case in `<agent>.json` records the path:

```json
{
  "case_name": "stub-marker",
  "workspace_dir": "/private/var/folders/…/kairon-eval-ws-123456789/ws",
  …
}
```

`workspace_dir` is omitted when empty (for example when workspace creation failed, which is recorded as a failed case) and older result files without it still load. The path is set before the case is scored and the workspace is removed only after scoring, so scoring reads the host workspace: its file tree, `git status` and `.eval/` contents. Without `--keep-workspaces` the recorded directory no longer exists once the run is over. Removal is best effort: a failure prints a warning with the path and never fails the run.

The single-case (`kairon eval <agent> <case>`) and `--resume` paths go through the same code, so `workspace_dir` and `--keep-workspaces` behave identically there.

> Workspace scoring reads the workspace on the host. The [`file_exists`, `changed_files` and the other check types](#checks) read `workspace_dir` (as the agent left it, compared with the fixture commit) and need no container-specific code, so they behave identically natively and under `--sandbox`. `command` checks also run on the host, even under `--sandbox` (see [Command checks run on the host](#command-checks-run-on-the-host)).

### Outputs: `.eval/`

`.eval/` is the outputs directory. Under `--sandbox` it is bind-mounted read-write into the container, but it physically lives inside the host workspace, so whatever the process writes there is on the host the moment it is written. No "copy outputs out" step exists, and the layout is identical to a native run. Under `--sandbox` the fake `gh` also uses it: it appends every call to `.eval/gh.log` and copies body files to `.eval/gh-body-<n>.md` (see [The fake `gh`](#the-fake-gh)). A case mock built from `fixtures/mock-cli.sh` logs to `.eval/mock-<command>.log` the same way.

### Case Timeout

A case's `timeout` takes precedence over the default for that case:

| Where | Without `timeout` | With `timeout` |
|-------|-------------------|----------------|
| Native | `KAIRON_EVAL_TIMEOUT`, else 2 minutes | the case `timeout` |
| `--sandbox` | the sandbox limit (`--resource-limit timeout=`, `sandbox.timeout`; default 5 minutes) | the case `timeout` |

The same value is sent to the backend as the request timeout, so a host-side and an in-container deadline agree. For the `kiro-cli` backend the container exec deadline equals the timeout. For helper-based backends such as `stub` the host deadline is the timeout plus 10 seconds, so the in-container backend normally reports the timeout itself and the host deadline is only a backstop. A timeout is recorded as an empty `actual_output`, an `error_context.stderr` beginning `timeout after <duration>` (the effective duration, for example `1s`, not the sandbox default), a call record with an `error`, and a failing score. The container error message names both the case `timeout` and `--resource-limit timeout=`.

## Self-Test

The harness has a self-test that exercises the full pipeline (rubric and case loading, prompt assembly, agent call, deterministic and judged scoring, cost accounting, result files) with no model calls and no `kiro-cli`:

```bash
task eval:selftest
# equivalent to:
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest
```

Fixtures live in `internal/eval/testdata/evals/`:

```
internal/eval/testdata/evals/
  agents/selftest.json              # minimal agent config (model: claude-sonnet-5.5; prompt: file://./selftest-prompt.md;
                                    #   resources: a non-existent selftest-conventions skill, so resources_present is [])
  agents/selftest-prompt.md
  agents/selftest-fail.json         # separate agent whose cases are expected to FAIL (see below)
  agents/selftest-fail-prompt.md
  rubrics/selftest.yaml             # structural_completeness (deterministic), clarity (LLM-judged), cost_efficiency (cost)
  rubrics/selftest-fail.yaml        # structural_completeness (deterministic), cost_efficiency (cost)
  cases/selftest/stub-basic.yaml    # no stub usage -> estimated; setup file exercises path rebasing
  cases/selftest/stub-usage.yaml    # stub model + usage 123/45 -> reported; stub-model has no price, so the cost is unknown
  cases/selftest/stub-price-sonnet.yaml   # model claude-sonnet-5.5, usage 1000/500 -> priced, cost 0.0105
  cases/selftest/stub-price-unknown.yaml  # model claude-haiku-4.5, usage 1000/500 -> unpriced, cost unknown (cost_unknown, cost_incomplete)
  cases/selftest/stub-quoted-input.yaml  # input with quotes, newlines, $(...) and backticks; must reach the backend verbatim
  cases/selftest/stub-marker.yaml   # stub turn command 'echo hi > marker.txt' leaves marker.txt in the case workspace
  cases/selftest/stub-seeded-workspace.yaml  # workspace: seeded; the stub appends to README.md -> ' M README.md'
  cases/selftest/stub-conventions-fixture.yaml  # workspace: conventions; the fixture supplies selftest-conventions -> per-case resources_present lists it
  cases/selftest/check-*.yaml       # 14 passing check cases: one per deterministic check type plus 3 check-judge-* (see below)
  cases/selftest/multi-turn-gh-log.yaml      # 3 turns, one scripted stub turn each; turn-scoped gh_log / output checks plus default-last checks
  cases/selftest/multi-turn-workspace.yaml   # 2 turns plus a setup text entry; turn 2 needs the file turn 1 wrote (shared workspace)
  cases/selftest/multi-turn-judge.yaml       # 2 turns, two judge checks (turn: 1 and unscoped); the --debug prompts show the transcript scoping
  cases/selftest/sim-routes.yaml             # simulated user with facts f1/f2, routed by stub.routes (fact_asked / fact_not_asked)
  cases/selftest/sim-stop-when.yaml          # simulated phase ended early by stop_when, then two scripted turns
  cases/selftest/sim-judge.yaml              # simulated user plus one judge check; the prompt carries the messages actually sent
  cases/selftest-fail/stub-timeout.yaml      # timeout: 1s, stub turn command 'sleep 3' -> timeout failure
  cases/selftest-fail/stub-empty-response.yaml  # empty stub response -> no output; every criterion scores 0 and is counted
  cases/selftest-fail/check-*.yaml  # 10 per-type failing check cases, 6 check-judge-* cases and check-partial (see below)
  cases/selftest-fail/multi-turn-wrong-turn.yaml  # turn-1 gh_log_contains for a call made in turn 3 (expected to FAIL)
  agents/selftest-sandbox.json      # containment agent, allowedTools [read, write]; its cases are all requires_sandbox
  agents/selftest-sandbox-prompt.md
  agents/selftest-sandbox-ro.json   # containment agent, allowedTools [read]
  agents/selftest-sandbox-ro-prompt.md
  rubrics/selftest-sandbox.yaml
  rubrics/selftest-sandbox-ro.yaml
  cases/selftest-sandbox/stub-gh-fake.yaml            # fake gh: logged call, copied --body-file, gh issue view from gh_issue
  cases/selftest-sandbox/stub-workspace-write.yaml    # './marker.txt' in the workspace is writable
  cases/selftest-sandbox/stub-tool-allowed.yaml       # fs_write tool call runs (fs_write is trusted)
  cases/selftest-sandbox/stub-mock-cli.yaml           # mocks: aws -> fixtures/mock-cli.sh; call logged in .eval/mock-aws.log, answered from the workspace
  cases/selftest-sandbox/stub-kairon-binary.yaml      # 'kairon --version' runs from PATH; 'command -v kairon' is /usr/local/bin/kairon
  cases/selftest-sandbox/stub-kairon-binary-readonly.yaml  # writing to / chmod-ing /usr/local/bin/kairon is refused (Read-only file system / Permission denied); the binary still runs
  cases/selftest-sandbox-ro/stub-write-outside-mounts.yaml  # write to /etc fails with 'Read-only file system' (expected to FAIL)
  cases/selftest-sandbox-ro/stub-tool-denied.yaml     # fs_write tool call is denied and recorded (only fs_read is trusted)
  agents/selftest-checks.json       # checks_only agent whose cases pass (see below)
  agents/selftest-checks-prompt.md
  agents/selftest-checks-fail.json  # checks_only agent whose case is expected to FAIL
  agents/selftest-checks-fail-prompt.md
  agents/selftest-floor.json        # checks_only agent with a min_criterion_score floor; expected to FAIL (see below)
  agents/selftest-floor-prompt.md
  rubrics/selftest-checks.yaml      # checks_only: true; file_written, report_marker (both 1-5, neither deterministic), cost_efficiency (cost)
  rubrics/selftest-checks-fail.yaml # checks_only: true; same criteria
  rubrics/selftest-floor.yaml       # checks_only: true; pass_threshold 60, min_criterion_score 50; file_written, report_marker (1-5), cost_efficiency (cost)
  cases/selftest-checks/check-one-criterion.yaml   # checks on file_written only: report_marker is not scored
  cases/selftest-checks/check-both-criteria.yaml   # checks on both criteria, all passing
  cases/selftest-checks-fail/stub-empty-response.yaml  # empty stub response: each checked criterion scores 0/N (expected to FAIL)
  cases/selftest-floor/floor-one-criterion-zero.yaml   # file_written 2/2, report_marker 0/1: 66.7% overall passes 60%, but report_marker is below the 50% floor (expected to FAIL)
  fixtures/selftest-input.md        # referenced as .kairon/evals/fixtures/selftest-input.md
  fixtures/mock-cli.sh              # byte-identical copy of .kairon/evals/fixtures/mock-cli.sh (this evals dir resolves case scripts here)
  fixtures/hidden/hidden_test.go    # file injected by the check-command-inject case (see Checks); the agent never sees it
  fixtures/workspaces/seeded/       # README.md plus docs/notes.txt: the seeded workspace fixture
  fixtures/workspaces/conventions/  # .kiro/skills/selftest-conventions/SKILL.md: a fixture-supplied *-conventions override hook
  fixtures/workspaces/mock-aws/     # report.txt plus .mocks/aws/s3-cp.out: canned reply for the mocked aws
```

`selftest.json` declares a `model` so the self-test passes the model-pinning pre-flight, and lists a `skill://.kiro/skills/selftest-conventions/SKILL.md` resource that no project file provides. It exercises the "missing resources are normal" rule: the run succeeds and the agent-level `resources_present` is `[]`, even if you create `.kiro/skills/selftest-conventions/SKILL.md` in the repository, because a project `*-conventions` skill is never staged. Only the `stub-conventions-fixture` case, whose workspace fixture supplies that file, lists it in its own per-case `resources_present` (see [Per-case fields](#per-case-fields-in-agentjson)).

The `selftest` agent has twenty-eight cases and all of them pass (`task eval:selftest`): eight `stub-*` cases — the five original ones (`stub-basic`, `stub-marker`, `stub-quoted-input`, `stub-seeded-workspace`, `stub-usage`), which carry no `checks` and keep their pinned scores, `stub-conventions-fixture`, and the two pricing cases `stub-price-sonnet` and `stub-price-unknown` (see [Pricing](#pricing)); fourteen `check-*` cases that exercise [checks](#checks) (ten per-type cases, `check-command-inject`, and three `judge` cases: `check-judge-yes`, `check-judge-files` and `check-judge-mixed`); three [multi-turn](#multi-turn-cases) cases (`multi-turn-gh-log`, `multi-turn-judge`, `multi-turn-workspace`); and three [simulated-user](#simulated-user-elicitation-cases) cases (`sim-routes`, `sim-stop-when`, `sim-judge`). The `selftest-fail` agent is deliberately separate, so `selftest` keeps meaning "everything passes". It has twenty-one cases. Twenty are **expected to fail** on a plain run: sixteen `check-*` cases (ten per-type cases and five of the six `check-judge-*` cases) plus `check-partial`, all built so that checks fail. The sixth, `check-judge-repeat`, is the exception: its `stub.judge: [yes, no]` answers `yes` on the first run, so a plain run (no `--repeat`) scores it `1/1` and the agent reports `20/21 cases failed, success rate 4.8% (1/21 attempts)`; only under [`--repeat 2`](#repeat-runs---repeat) does it show the `1/1` then `0/1` spread that pools to a failing 50% (see the `judge` paragraph below). The agent still ends below threshold either way. The rest are `stub-timeout`, which records a timeout failure (empty `actual_output`, `error_context.stderr` containing `timeout after 1s`) in about a second; `stub-empty-response`, whose stub returns an empty response, so the case has no output, every criterion scores 0 out of its maximum, and it still counts toward the agent's score; `multi-turn-wrong-turn`; and `sim-fact-not-asked`, both described below. Run it with:

```bash
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail
```

This command exits **non-zero**: `selftest-fail` ends below its pass threshold, so it prints a `FAIL selftest-fail: …` line and exits `1` (see [Pass/Fail Verdict and Exit Status](#passfail-verdict-and-exit-status)). The results and `summary.json` are still written, and each failed check is printed on its own `✗` line. `task eval:selftest` runs `selftest`, which prints `PASS selftest: …` and exits 0.

All check cases score the `structural_completeness` criterion (present in both rubrics, so the rubrics and the five original cases are unchanged), use only POSIX stub commands, and have no model or `kiro-cli` dependency.

| Check type | `selftest` case (passes) | `selftest-fail` case (fails) |
|------------|--------------------------|------------------------------|
| `command` | `check-command`: `test -f built.txt` exits 0, plus `exit 3` with `expect_exit: 3` | `check-command`: `exit 3` with the default `expect_exit: 0` |
| `file_exists` | `check-file-exists` | `check-file-exists`: `missing.txt` |
| `file_absent` | `check-file-absent` | `check-file-absent`: `marker.txt` exists |
| `file_contains` | `check-file-contains` | `check-file-contains`: pattern not in the file |
| `file_not_contains` | `check-file-not-contains` | `check-file-not-contains`: pattern is in the file |
| `changed_files` | `check-changed-files`: README.md changes, `.eval/note.txt` is ignored, `allow: [README.md]` | `check-changed-files`: deletes `docs/notes.txt` and adds `extra.txt`; the reasoning names both |
| `output_contains` | `check-output-contains` | `check-output-contains` |
| `output_not_contains` | `check-output-not-contains` | `check-output-not-contains` |
| `gh_log_contains` | `check-gh-log-contains`: the stub writes a `gh issue create` line to `.eval/gh.log` | `check-gh-log-contains`: the log is empty |
| `gh_log_not_contains` | `check-gh-log-not-contains` | `check-gh-log-not-contains`: the stub writes a `gh pr merge 1` line |
| `judge` | `check-judge-yes`: `stub.judge: yes`; `multi-turn-judge` and `sim-judge` for `turn` and the conversation | `check-judge-no`: `stub.judge: no` fails with `judge answered no: stub judge: no` |
| `fact_asked` | `sim-routes` (`fact_asked: f1`) | |
| `fact_not_asked` | `sim-routes` (`fact_not_asked: f2`) | `sim-fact-not-asked`: the stub routes the first question to `f1`, so `fact_not_asked: f1` fails |

The `judge` check has further cases. In `selftest`: `check-judge-files` has the stub turn write `.eval/issue-body.md` and the check list it in `files`, so the whole file path runs end to end; `check-judge-mixed` puts a `file_exists` check and a `judge` check on one criterion and scores `2/2`. In `selftest-fail`: `check-judge-garbage` (`stub.judge: garbage`) fails with `judge parse error`; `check-judge-no-script` (no `stub.judge`) fails with `judge call failed: case has no stub.judge`; `check-judge-missing-file` (`files: [.eval/absent.md]`) fails with `file does not exist` and makes no judge call; `check-judge-list` (`stub.judge: [yes, no]`, two `judge` checks) passes the first and fails the second, so it records exactly `1/2`; `check-judge-repeat` (`stub.judge: [yes, no]`, one `judge` check) is meant for [`--repeat`](#repeat-runs---repeat): the judge answers `yes` on run 1 and `no` on run 2, so it scores `1/1` and then `0/1`. None of them is skipped.

To see the repeat statistics end to end, run the whole `selftest` agent three times and the `check-judge-repeat` case twice:

```bash
# Every case runs 3 times: 3 results per case in selftest.json, repeat 3 in summary.json
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest --repeat 3

# Two different scores: run 1 scores 1/1, run 2 scores 0/1. Prints
#   FAIL selftest-fail: 50.0% (threshold 95.0%), 1/1 cases failed, success rate 50.0% (1/2 attempts) [repeat 2: min 0.0%, max 100.0%]
# and exits 1 (the mean, 50, is below the threshold)
go run ./cmd/kairon eval --backend stub --no-sandbox --evals-dir internal/eval/testdata/evals selftest-fail check-judge-repeat --repeat 2
```

Then inspect the newest run directory under `internal/eval/testdata/evals/results/`:

```bash
# selftest: every case name appears exactly 3 times -> [3]
jq '[.cases[].case_name] | group_by(.) | map(length) | unique' <run>/selftest.json
jq '.agent_verdicts.selftest | {repeat, score, score_min, score_max, repeat_scores}' <run>/summary.json

# selftest-fail / check-judge-repeat -> repeat 2, score 50, score_min 0, score_max 100, repeat_scores [100, 0]
jq '.agent_verdicts."selftest-fail" | {repeat, score, score_min, score_max, repeat_scores}' <run>/summary.json
```

`--repeat 0` (or a negative value) is rejected: `go run ./cmd/kairon eval --repeat 0 selftest` prints `--repeat must be at least 1 (got 0)` and exits non-zero.

Further `selftest` / `selftest-fail` cases pin behavior you may rely on:

- `selftest/check-command-inject` shows that an injected file stays hidden. The stub writes the workspace listing to `.eval/seen.txt` before the checks run; the `command` check then injects `fixtures/hidden/hidden_test.go` and passes only if the file is present; `file_not_contains` on `.eval/seen.txt` proves the agent never saw it, and `file_absent` on `hidden_test.go` proves it was removed afterwards.
- `selftest-fail/check-partial` has one passing and one failing `file_exists` check on one criterion, so it records exactly `1/2` with the reasoning `1/2 checks passed; failed: #2 file_exists path=missing.txt (file does not exist)`.
- `selftest/multi-turn-gh-log` has three `turns` and one scripted stub turn each, with distinct markers (`TURN-ONE-MARKER`, `TURN-TWO-MARKER`, `TURN-THREE-MARKER`). Only turn 3's stub turn appends `gh issue create --title t` to `.eval/gh.log`. Its checks show the `turn` field: `gh_log_not_contains 'issue create'` at turn 1 and turn 2, `gh_log_contains 'issue create'` at turn 3 (and, with no `turn`, defaulting to the last turn), `output_contains` of each marker at its own turn, and an unscoped `output_not_contains TURN-ONE-MARKER` that passes because it reads the last turn. Its result has `turn_outputs` with the three scripted responses and three agent `calls[]` numbered `turn` 1..3.
- `selftest/multi-turn-workspace` has two `turns` and a `setup` text entry. Turn 2's stub command is `test -f turn1.txt && echo second > turn2.txt`, so the case fails if the workspace were not shared between turns; `file_exists` and `file_contains` workspace checks (which take no `turn`) confirm both files.
- `selftest-fail/multi-turn-wrong-turn` has the same shape as `multi-turn-gh-log` but asserts `gh_log_contains 'issue create'` with `turn: 1` for a call made in turn 3. It fails (the reasoning names the check with `turn=1`), which proves turn scoping is real: the check would pass if it saw the final log.

The native self-test has no real `gh`, so the `gh_log_*` cases, `multi-turn-gh-log` and `multi-turn-wrong-turn` write `.eval/gh.log` from a stub command (see the [native caveat](#checks)).

The simulated-user cases use the stub backend only, so they make no model call and need no `kiro-cli`:

- `selftest/multi-turn-judge` has two `turns` carrying distinct markers (`USER-ONE-MARKER`, `USER-TWO-MARKER` in the user messages; `REPLY-ONE-MARKER`, `REPLY-TWO-MARKER` in the stub replies) and two `judge` checks on `structural_completeness`, one with `turn: 1` and one without; `stub.judge: yes` answers both, so the case scores `2/2`. Run it with `--debug` to see the two judge prompts: the `turn: 1` prompt holds `USER-ONE-MARKER` and `REPLY-ONE-MARKER` in `TURN 1 USER MESSAGE` / `TURN 1 AGENT REPLY` blocks and neither turn-2 marker, while the unscoped prompt holds all four, in order, through `TURN 2 AGENT REPLY`. A `turn` beyond the case's turns cannot be a `selftest-fail` case file, because the loader rejects it for the whole agent (`turn 9 is beyond the case's 2 turn(s)`); that load error and the run-time failure (`turn 9 is out of range: the case has 2 turn(s)`) are covered by Go tests.
- `selftest/sim-judge` has the shape of `sim-routes` (one fact `f1`, `max_turns: 3`, `stub.routes: [f1, none]`, three stub turns with `SIM-TURN-ONE-MARKER`, `SIM-TURN-TWO-MARKER` and `SIM-TURN-THREE-MARKER`) plus one unscoped `judge` check. Its `--debug` prompt shows the conversation as it happened: the opening message, the verbatim `f1` answer and the `default_reply` as the user messages of turns 1 to 3, each followed by the stub reply to it.
- `selftest/sim-routes` has facts `f1` (`discoverable: false`) and `f2` (`discoverable: true`), `max_turns: 3`, `stub.routes: [f1, none]` and three stub turns with distinct markers. The agent's first question is routed to `f1` and answered verbatim, the second to `none` and answered with `default_reply`, and the third turn ends the phase unanswered. Its result has `user_turns` `[input, <f1 answer>, <default_reply>]` (three entries) and three agent `calls[]` plus two routing calls with `purpose: simulated_user`. With `--keep-workspaces`, `.eval/sim-log.jsonl` has exactly two lines: turn 1 → `f1` (`discoverable: false`) and turn 2 → `none`. Its checks are `fact_asked: f1`, `fact_not_asked: f2` and `output_contains` of each marker at its turn.
- `selftest/sim-stop-when` has `stop_when: 'PLAN-READY'`, `max_turns: 5`, `stub.routes: [f1]` and two scripted `turns` (`Please summarise.`, `Thanks.`). The second stub response contains `PLAN-READY`, so the simulated phase ends after 2 agent turns (only the first output is routed) and the scripted turns run as turns 3 and 4: `turn_outputs` has 4 entries, `user_turns` is `[input, <f1 answer>, "Please summarise.", "Thanks."]`, and the sim log has one line.
- `selftest-fail/sim-fact-not-asked` has the shape of `sim-routes` with `fact_not_asked: f1` while the stub routes `f1`. It fails and the reasoning names `fact_not_asked fact=f1`.

The `selftest-sandbox` and `selftest-sandbox-ro` agents exercise [Sandbox Containment](#sandbox-containment) with the stub backend (`selftest-sandbox` includes `stub-mock-cli`, the working example of [the mock pattern](#preventing-production-side-effects-containment-and-mocking)). Every one of their cases is `requires_sandbox: true`, so a native run records each as failed with `requires --sandbox` and starts nothing. `stub-kairon-binary` and `stub-kairon-binary-readonly` cover the [`kairon` binary mount](#the-kairon-binary): the first runs `kairon --version > .eval/kairon-version.txt` and `command -v kairon > kairon-path.txt`, the second tries to write to and `chmod` `/usr/local/bin/kairon`, records the refusals and runs `kairon --version` again; both are expected to **pass**. `stub-write-outside-mounts` is, like `selftest-fail`, **expected to fail** under `--sandbox`: that failure is the proof that the root filesystem is read-only, and it also means the `selftest-sandbox-ro` run exits non-zero (its agent ends below the pass threshold). Run them under the sandbox with:

```bash
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest-sandbox
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest-sandbox-ro
```

The `selftest-checks` and `selftest-checks-fail` agents exercise [`checks_only` rubrics](#checks_only-rubrics) with the stub backend. Their fixtures use no `judge` checks and no `simulated_user`, so a run makes no judge call and its `calls[]` holds no record with `role: judge`.

- `selftest-checks` **passes**. `check-one-criterion` has checks on `file_written` only, so its result records a score for `file_written` and none for `report_marker`, and the case total equals the number of checks. `check-both-criteria` has checks on both criteria and scores both in full.
- `selftest-checks-fail` is **expected to fail**. `stub-empty-response` has an empty stub response, so the agent produced no output: each checked criterion records `0/N` (not skipped), no check runs, and the agent ends below its threshold, so the run exits non-zero.

```bash
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks        # PASS, exit 0
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-checks-fail   # FAIL, exit 1
```

`internal/eval/testdata/checks-only-invalid/` is a separate, deliberately **invalid** evals directory: its `rubrics/selftest-checks.yaml` sets `checks_only: true`, but its only case, `cases/selftest-checks/no-checks.yaml`, has no `checks:` block. Running it is rejected while the cases load, before any case runs and without writing a result file, with a non-zero exit and an error that names `no-checks.yaml`:

```bash
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/checks-only-invalid selftest-checks   # error naming no-checks.yaml, exit 1
```

The `selftest-floor` agent exercises [`min_criterion_score`](#per-criterion-scores-and-the-floor). Its rubric (`rubrics/selftest-floor.yaml`) is `checks_only` with `pass_threshold: 60` and `min_criterion_score: 50`, and its one case, `cases/selftest-floor/floor-one-criterion-zero.yaml`, runs `echo hi > marker.txt` and responds with text that does not contain `output marker alpha`. `file_written` has two passing checks (`file_exists marker.txt` and `file_contains marker.txt`), so it scores `2/2` (100%); `report_marker` has one `output_contains "output marker alpha"` check, which fails, so it scores `0/1` (0%). The overall score is 2/3 = 66.7%, **above** the 60% threshold, and the case itself passes (`cases_failed` is `0`), but `report_marker` is below the 50% floor. `selftest-floor` is therefore **expected to FAIL** (exit `1`): the failure is the proof that the floor, not the overall score, decides the verdict.

```bash
go run ./cmd/kairon eval --backend stub --evals-dir internal/eval/testdata/evals selftest-floor   # FAIL, exit 1
```

It prints `FAIL selftest-floor: 66.7% (threshold 60.0%), 0/1 cases failed, success rate 100.0% (1/1 attempts); criterion below floor 50.0%: report_marker 0.0% (0/1)` followed by the two criterion lines, and the saved `summary.json` verdict has `passed: false`, `min_criterion_score: 50` and `criteria_below_floor: ["report_marker"]`. Removing the `min_criterion_score` line from a copy of the rubric makes the same run pass with the same score.

`selftest-fail`, `selftest-checks-fail` and `selftest-floor` are expected-to-fail agents, and `checks-only-invalid` is an expected-to-be-rejected fixture; none of them is a regression when it fails.

The self-test runs with the stub backend. Its agent calls record the scripted `stub.turns[].model` (for example `stub-model`) rather than the pinned name, and its judge calls record the pinned judge model (so they are priced like the real judge). The pinned models also appear in the run-level `agent_model` and `judge_model` fields. Because `stub-model` has no configured price, the `stub-usage` case has an unknown cost, so a full `selftest` run reports `cost_incomplete`; the single-case runs `selftest stub-price-sonnet` (priced, `0.0105`) and `selftest stub-price-unknown` (unpriced `claude-haiku-4.5`) show the two outcomes without that noise (see [Pricing](#pricing)).

To list the self-test cases: `kairon eval --evals-dir internal/eval/testdata/evals --list selftest`.

Results go to `internal/eval/testdata/evals/results/`, which is git-ignored (`.gitignore` entry `internal/eval/testdata/evals/results/`), so running the self-test leaves the working tree clean. These fixtures live under `testdata`, outside the template-synced `.kairon/evals/`, so they do not affect `task sync:check`.

### Self-Test in the Container Sandbox

The same self-test can run hermetically inside a container sandbox. It needs a Podman or Docker daemon and network access for the image build (see [Backends in the Container](#backends-in-the-container)):

```bash
task eval:selftest:sandbox
# runs the daemon-gated tests with KAIRON_EVAL_SANDBOX_SELFTEST=1:
#   internal/eval/sandbox: TestBaseImage_ToolsOnlyNoMounts, TestEnsureBaseImage_ReusesExistingImage
#   internal/eval:         TestSelftestSandbox, TestSandboxWorkspace, TestContainmentSandbox, TestContainmentContainerGH, TestProvenanceParitySandbox

# or run the sandboxed self-test directly:
go run ./cmd/kairon eval --backend stub --sandbox --evals-dir internal/eval/testdata/evals selftest
```

`TestSelftestSandbox` runs `selftest` natively and then again with `--sandbox --backend stub`, and requires the sandboxed run to match the native one for every case: non-empty and identical `actual_output`, identical `agent_cost`, and the same model on the recorded agent call. It also applies the same self-test expectations to the sandboxed results. For the two multi-turn cases, which run in one container per case under `--sandbox`, it additionally requires identical `turn_outputs`, identical agent call `turn` numbers and identical criterion scores. The `stub-quoted-input` case checks that shell metacharacters in the input arrive intact.

The other gated tests cover the sandbox layering:
- `TestProvenanceParitySandbox` runs `selftest` and `selftest-fail` natively and with `--sandbox` and checks provenance and scoring parity and the `eval diff` mode report (see [Parity between native and container runs](#parity-between-native-and-container-runs)).
- `TestBaseImage_ToolsOnlyNoMounts` starts a container from the base image with no mounts and checks it runs as `sandbox` (uid 1000), has `kiro-cli`, `gh`, `git` and `sh` on `PATH`, and has an empty `/workspace` with no `/workspace/.kiro` and no project or agent content anywhere.
- `TestEnsureBaseImage_ReusesExistingImage` calls `EnsureBaseImage` twice and requires the second call to report no build with the same tag and image ID.
- `TestSandboxWorkspace` runs `stub-marker`, `stub-seeded-workspace` and the `selftest-fail` timeout case both natively and under `--sandbox` with kept workspaces and compares them: no `Permission denied` anywhere, `marker.txt` containing `hi` on the host, identical host trees (excluding `.git` and `.kiro`) and `git status --porcelain`, `.eval/` present in both, an unchanged repository-root `git status`, keep/no-keep behaviour of `workspace_dir`, the sandboxed timeout recorded as `timeout after 1s`, and base-image reuse after editing an agent config and a case.

A plain `go test ./...` also runs `TestSelfTestWorkspaceCasesNative`, which needs no daemon: it runs the same new cases natively and checks the workspace behaviour above. It likewise runs `TestContainmentFixtures`, `TestContainmentMockCLIFixtureParity` and `TestContainmentNativeRefusal`, which need no daemon either: the first checks that the containment cases load and are all `requires_sandbox` (including `stub-kairon-binary` and `stub-kairon-binary-readonly`, and that `stub-mock-cli` declares the `aws` mock), the second that the self-test copy of `mock-cli.sh` is byte-identical to the live `.kairon/evals/fixtures/mock-cli.sh`, the third that a native run fails every one of them with `requires --sandbox`, makes no agent call and creates no workspace. The mock script itself is covered by hermetic tests that run it with `sh` on the host (`mockcli_test.go`) and the `mocks:` validation and staging by `mocks_test.go`.

The containment tests are daemon-gated like the rest:
- `TestContainmentSandbox` runs the containment agents under `--sandbox --backend stub --keep-workspaces` and checks: `.eval/gh.log` holds the `gh issue create` call and `.eval/gh-body-1.md` equals `b.md`; `view.json` holds the configured `gh_issue` title and body; `stub-mock-cli` (subtest `mocked cli`) leaves `.eval/mock-aws.log` with exactly the two `aws` calls, `aws-path.txt` holding `/opt/kairon/bin/aws`, `aws-out.txt` equal to the canned `.mocks/aws/s3-cp.out`, `aws-unsimulated.txt` containing `is not simulated`, and only those three `aws-*.txt` files in `git status --porcelain`; `stub-write-outside-mounts` fails with `Read-only file system` in `error_context.stderr`; `marker.txt` exists on the host after `stub-workspace-write`; `stub-tool-denied` leaves no `tool-marker.txt` and records an `fs_write` denial with `trusted_tools` `["fs_read"]`; `stub-tool-allowed` creates `tool-marker.txt` with no denials; `stub-kairon-binary` passes its `file_contains` checks on `.eval/kairon-version.txt` (the `kairon --version` output) and on `kairon-path.txt` (`/usr/local/bin/kairon`), and the `selftest-sandbox` results carry `containment.kairon_binary: true` in both `<agent>.json` and `summary.json`; `stub-kairon-binary-readonly` passes, with its `file_contains` check on `write-refused.txt` matching `Read-only file system` or `Permission denied` (the binary is owned by the host user, so the kernel may refuse with either error; the plain write is refused earlier with `Text file busy` because the harness is executing the binary, so the case also `chmod`s it, which the kernel refuses at the read-only mount check) and a second `kairon --version` (into `after.txt`) still succeeding, which shows the read-only mount holds and the binary stays runnable.
- `TestContainmentContainerGH` starts a container with the case's mounts, environment and host config and checks that `command -v gh` is `/opt/kairon/bin/gh`, and that `/usr/local/bin/gh auth status` (the real `gh`, by absolute path) exits non-zero with a "not logged in" message.

Skip behaviour (the test never builds an image by accident, so `task test` and `go test ./...` are unaffected):

- **No container daemon.** When neither Podman nor Docker is reachable the tests are skipped, not failed, with a message such as `no container daemon reachable (tried Podman and Docker); start Podman or Docker to run the sandbox self-test`. `task eval:selftest:sandbox` then exits 0.
- **Gate not set.** Without `KAIRON_EVAL_SANDBOX_SELFTEST=1` (for example a plain `go test ./internal/eval`) the tests are skipped with `sandbox self-test is opt-in: set KAIRON_EVAL_SANDBOX_SELFTEST=1 or run task eval:selftest:sandbox (needs Podman or Docker)` and no container is started.

The direct `go run ... --sandbox` command has no skip: without a reachable daemon it fails up front with the daemon-unavailable error.

## Adding a New Backend

1. Add a file in `internal/inference` (for example `mybackend.go`) with a type implementing `inference.Backend`:
   - `Name() string` — the registry name, also the `--backend` value.
   - `Available() error` — nil when the backend can be used in this environment.
   - `StartupProbe() time.Duration` — start-up overhead, or `0` if there is none.
   - `Invoke(ctx, Request) (Response, error)` — handle `RoleAgent` and `RoleJudge`.
2. Add a name constant and register a constructor in the `registry` map in `internal/inference/inference.go`. `inference.Names()` feeds both the `--backend` help text and the unknown-backend error, so no CLI change is needed.
3. Follow the contract:
   - Fill `Response.Usage` with `Source` set to `inference.UsageReported` when the backend supplies real token counts (for the stub, from the case's `stub.turns[].usage`); otherwise use `inference.EstimateUsage`.
   - Set `Response.Model` when known.
   - Populate `Command`, `Stderr`, `ExitCode` and `Duration` even when returning an error, since the harness builds `error_context` from them.
   - Wrap `inference.ErrTimeout` on timeouts so `errors.Is(err, inference.ErrTimeout)` holds.
   - Honour `Request.Turn`, the 0-based position of this user message in the conversation. `Prompt` carries **only that turn's** user message, and the harness passes no resume flag or transcript, so a backend must continue the conversation itself when `Turn > 0` (`kiro-cli` adds `--resume`, the stub answers `Stub.Turns[Turn]`, a direct-API backend keeps a message list keyed by `WorkDir`, starting a new one at `Turn == 0`). A backend with no notion of conversation can ignore it, but then a `turns` case cannot work on it. See [Backend contract for turns](#backend-contract-for-turns), including its limit under `--sandbox`.
   - A judge response must contain `===JSON_START===` ... `===JSON_END===` with `{"score": <1-5>, "reasoning": "...", "pass": <bool>}`.
   - A routing request (`Request.Route`, a `RoleJudge` request made by a [simulated-user case](#routing)) needs nothing special from a real backend: the prompt carries the whole question and the reply is parsed from `===JSON_START==={"reasoning": "...", "fact": "<id>" | "none"}===JSON_END===`, so answer it as an ordinary judge prompt. `Request.RouteSeq`, the 0-based number of the routing decision in the case, only exists so the stub can index `stub.routes`; ignore both fields. `Route` and `YesNo` are never set together.
4. Keep the package stdlib-only and do not import `internal/eval`.
5. Add unit tests next to it (see `stub_test.go` and `kirocli_test.go`), and verify with `go test ./internal/inference/...`.

## Agent Coverage

All six shipped agents have rubrics and test cases:

| Agent | Rubric | Key Criteria |
|-------|--------|--------------|
| `architect` | `rubrics/architect.yaml` | task_decomposition, acceptance_criteria_testability, file_reference_accuracy, completeness |
| `builder` | `rubrics/builder.yaml` | code_correctness, spec_adherence, code_quality, test_coverage |
| `documenter` | `rubrics/documenter.yaml` | documentation_completeness, accuracy, file_naming_convention, practical_usage_guidance |
| `krew-lead` | `rubrics/krew-lead.yaml` | workflow_adherence, delegation_quality, retry_policy_compliance, error_handling |
| `planner` | `rubrics/planner.yaml` | requirement_clarity, scope_appropriateness, acceptance_criteria_quality, constraint_identification |
| `validator` | `rubrics/validator.yaml` | issue_coverage, test_execution, defect_detection, actionable_feedback |

## Evaluation Workflow

The evaluation framework serves as unit testing for prompt engineering. Follow this workflow when modifying agent prompts or configurations.

**Iterate on a candidate, not on the live prompt.** Do not edit `.kiro/agents/<agent>-prompt.md` to try an idea. That file is live: krew-lead spawns the builder and validator from the worktree's `.kiro/agents/`, so an edit made mid-iteration changes the pipeline that is building your PR (and an edited planner prompt changes the planner you use every day). Write each candidate to its own file, conventionally `.kairon/iterations/<agent>/vN.md`, and score it with `--prompt-file` (see [Evaluating a candidate prompt](#evaluating-a-candidate-prompt---prompt-file)). The candidate is used only in the per-case eval workspaces, so the live agent is never modified. Copy the winning candidate over the live prompt only once, when you are done iterating.

### Before Making Changes (Baseline)

**Required**: Run `kairon eval` before making any prompt changes to establish a baseline:

```bash
# Capture current performance
kairon eval
```

This creates a results snapshot at `.kairon/evals/results/<timestamp>-<git-hash>/` for comparison.

### After Making Changes (Verification)

**Required**: Run `kairon eval` after prompt changes to verify improvements. For a candidate prompt, run the agent with `--prompt-file`; for any other change, run `kairon eval` as before:

```bash
# Test a candidate prompt for one agent (the live prompt is not touched)
kairon eval architect --prompt-file .kairon/iterations/architect/v2.md

# Test other modified behavior
kairon eval

# Compare with baseline
kairon eval diff <baseline-hash> <current-hash>
```

`eval diff` lists the differing `prompt_sha256` and `prompt_file` in its Run Provenance block, so you can confirm which prompt each run used.

### Creating Test Cases for Behavioral Changes

When making specific behavioral changes, create targeted test cases:

1. **Identify the behavior** — What specific agent behavior are you changing?
2. **Create test case** — Add a case in `.kairon/evals/cases/<agent>/` that exercises this behavior
3. **Verify coverage** — Ensure existing rubric criteria measure the desired change
4. **Test iteratively** — Run evaluations as you refine the prompt

Example workflow for improving architect task decomposition:
```bash
# 1. Baseline
kairon eval architect

# 2. Add test case for complex decomposition scenario
# Edit .kairon/evals/cases/architect/complex-decomposition.yaml

# 3. Write a candidate prompt (a copy of .kiro/agents/architect-prompt.md with your
#    changes). Do not edit the live prompt while iterating.
mkdir -p .kairon/iterations/architect
cp .kiro/agents/architect-prompt.md .kairon/iterations/architect/v2.md
# Edit .kairon/iterations/architect/v2.md

# 4. Verify improvement with the candidate
kairon eval architect --prompt-file .kairon/iterations/architect/v2.md
kairon eval diff <baseline> <current>

# 5. Iterate on v3.md, v4.md, ... the same way; the live agent stays untouched
```

### Evaluation as Unit Testing

Treat evaluations like unit tests:
- **Red-Green-Refactor**: Baseline (red) → Change (green) → Optimize (refactor)
- **Regression prevention**: Catch unintended behavior changes
- **Performance tracking**: Monitor cost and quality over time
- **Documentation**: Results serve as behavioral specifications

## Container Sandboxing

The evaluation framework includes container sandboxing for isolated agent testing, using Podman or Docker.

### Using the --sandbox Flag

Run agent evaluations in containers for isolation:

```bash
# Run all agents in sandbox containers
kairon eval --sandbox

# Run specific agent in sandbox
kairon eval --sandbox architect

# List available test cases for an agent
kairon eval --sandbox --list architect

# Keep each case's host-side workspace for inspection
kairon eval --sandbox --keep-workspaces architect
```

The `--sandbox` flag:
- Obtains one cached, **tools-only base image** (built on first use, reused afterwards; see [Sandbox Layering](#sandbox-layering)).
- Builds each case's workspace on the host (see [Case Workspaces](#case-workspaces)) and bind-mounts it into a fresh container with resource limits, no network and a read-only root filesystem.
- Runs the selected backend inside the container as the unprivileged `sandbox` user, with a fake `gh` first on `PATH` and a per-agent tool trust set instead of `--trust-all-tools` (see [Sandbox Containment](#sandbox-containment)).

Nothing is copied into a running container: no project files, no `.kiro`, no `kairon` binary, no fake `gh`. Everything the agent sees arrives through bind mounts.

> **Model pinning in sandbox runs:** `--sandbox` honours `evals.agent_model` (it is passed to `kiro-cli` as `--model`, exactly as in a native run) and `evals.judge_model` for judge calls. See [`--sandbox` and `evals.agent_model`](#--sandbox-and-evalsagent_model).

### Sandbox Layering

The sandbox is split strictly by stability. Stable tools are baked once into an image at build time. Everything that varies per project or per run is supplied from the host through bind mounts.

```
build time, as root, cached by content hash        run time, host-side only, mounted
┌───────────────────────────────────────┐     ┌──────────────────────────────────────────────┐
│ kairon-eval-base:<platform>-<hash>    │     │ <ws>/.kiro   ro  staged project .kiro        │
│  alpine, git, bash/sh, ca-certs,      │  +  │ <ws>         rw  git repo, fixture commit    │
│  gh (pinned), kiro-cli (pinned),      │     │ <ws>/.eval   rw  outputs (inside <ws>)       │
│  user sandbox (uid 1000), /workspace  │     │ /opt/kairon/bin ro  fake gh + case mocks     │
└───────────────────────────────────────┘     │ /usr/local/bin/kairon ro  linux kairon (all) │
                                              └──────────────────────────────────────────────┘
```

The root filesystem of the running container is read-only; only the mounts above (the rw ones) and a few small tmpfs directories are writable (see [Read-only root filesystem](#read-only-root-filesystem)).

#### Base image contents

The image is defined by `internal/eval/dockerfile/base.Dockerfile` (embedded in the binary). It contains:

- Alpine 3.19 with `git`, `bash` and CA certificates (a POSIX `sh` is always present),
- `kiro-cli` at a pinned version, in `/usr/local/bin`,
- `gh` (GitHub CLI) at a pinned version, in `/usr/local/bin`,
- the unprivileged user `sandbox` (uid 1000) and an empty `/workspace` it owns,
- a build-time smoke test (`kiro-cli --version && gh --version && git --version`).

Deliberately **absent**: project toolchains (Go, Node.js, Python, Rust, Java, Task), any `COPY`/`ADD` of project content, `.kiro`, agents, skills, cases and the `kairon` binary. The sandbox no longer detects the project type or installs toolchains; if your evals need a toolchain, add it to the image (see [bumping a baked tool](#when-the-base-image-is-rebuilt)).

The baked `gh` is the **real** GitHub CLI and is **unauthenticated**: the container environment carries no GitHub token, `$HOME` is a fresh tmpfs with no `~/.config/gh/hosts.yml`, and the network is disabled. It is not what an agent reaches by default: a fake `gh` is first on `PATH` and answers instead (see [The fake `gh`](#the-fake-gh)). The real binary is only reached by absolute path (`/usr/local/bin/gh`), where `gh auth status` reports "not logged in". `ContainerConfig.MockGitHub` no longer exists. `kiro-cli` authentication inside the container is also not provided: a real `kiro-cli` sandbox run still needs credentials supplied through the container environment.

#### Mounts

Every container gets exactly these mounts (all are `bind` mounts of host paths that must exist; a missing path is an error, not an auto-created root-owned directory):

| Host | Container | Mode |
|------|-----------|------|
| `<workspace>/.kiro` | `<workspace_dir>/.kiro` | read-only |
| `<workspace>` | `<workspace_dir>` | read-write |
| `<workspace>/.eval` | `<workspace_dir>/.eval` | read-write |
| `<workspace-parent>/bin` (holds the fake `gh` and any case mocks) | `/opt/kairon/bin` | read-only |
| the linux `kairon` binary (every backend, including `kiro-cli`) | `/usr/local/bin/kairon` | read-only |

`<workspace_dir>` is `sandbox.workspace_dir` from `.kairon/config.yaml` (default `/workspace`). There is no `tmpfs` at the workspace path. On top of these bind mounts the container has three small `tmpfs` mounts (`/tmp`, `/var/tmp`, `/home/sandbox`) and a read-only root filesystem; see [Read-only root filesystem](#read-only-root-filesystem). The container runs as `sandbox` with `WorkingDir` set to the workspace, `HOME=/home/sandbox`, `PATH` starting with `/opt/kairon/bin` (`/usr/local/bin`, which holds the mounted `kairon`, is also on it), and a git `safe.directory=*` setting passed as environment (`GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_0`/`GIT_CONFIG_VALUE_0`), because the mounted repo is owned by a different uid than `sandbox` and git would otherwise refuse it as "dubious ownership".

#### Ownership and cleanup

Mounted directories are owned by the host user while the container runs as uid 1000. To make that work on rootful Docker, rootless Podman and the Docker Desktop / Podman machine VMs on macOS:

- The host opens the workspace to everyone (`a+rwX`) before the container starts. The temporary parent directory is `0700`, so other local users cannot traverse into it.
- Every command the harness executes in the container is wrapped as `sh -c 'umask 000; exec "$@"' kairon-exec <command…>`, so files the agent creates are world-accessible and the host user can read them for scoring and delete them afterwards, even when the container uid maps to a foreign host uid. The wrapper does not change the command's arguments, and the prompt is still delivered on stdin only.
- Removing the workspace never fails a run (a warning with the path is printed instead).

While a run is in flight the workspace contents are world-writable.

#### Platform notes

- **macOS (Docker Desktop, Podman machine):** the workspace path must be shared with the VM. The harness resolves symlinks (`/var` → `/private/var`), but if your `TMPDIR` is not a shared mount, set `KAIRON_EVAL_WORKSPACE_ROOT` to a directory under your home directory.
- **Rootless Podman:** supported through the ownership rules above; the open umask wrapper is what normally prevents files that the host cannot delete.
- **SELinux:** when the host reports SELinux as enforcing (`/sys/fs/selinux/enforce` is `1`), the container is created with `label=disable` so the user's directories are not relabeled with `:z`/`:Z`. This is only done on enforcing hosts.

#### When the base image is rebuilt

The image tag is `kairon-eval-base:<platform>-<12 hex>` (platform `/` becomes `-`), where the hash covers **only** the Dockerfile bytes, the platform and the tool pins (`sandbox.ToolSet`). It does not depend on the evals directory, agents, skills, cases, the working directory or the environment.

| Change | Result |
|--------|--------|
| Edit an agent config, a prompt, a skill, a rubric or a case; add a fixture; change `.kiro` | Image **reused** (`✅ Base image reused: <tag>`) |
| Change `base.Dockerfile`, a tool pin, or the platform | New tag, **rebuilt once** (`🔨 Base image built: <tag>`) |

The image is persistent: it is not removed at the end of a run, and the old per-run evaluation images are gone. A bumped pin yields a new tag, and the previous image stays until you remove it yourself (for example with `docker image prune` or `podman image prune`).

To bump a baked tool, change the version in `DefaultToolSet` in `internal/eval/sandbox/baseimage.go` (`KiroCLIVersion`, `GHVersion`). Both pins are versioned download URLs (the kiro-cli zip at `…/<version>/kirocli-<arch>-linux-musl.zip`, `gh` from the GitHub release `v<version>`), so the pin names a real, immutable artifact and nothing resolves to `latest`. To add another tool or toolchain, edit `base.Dockerfile`; either way the next run builds once.

Building the image needs network access and a container daemon, even for `--backend stub`. Container *execution* has no network; `NetworkMode: none` applies to containers, not to the build.

### Backends in the Container

The container is a transport: whichever backend `--backend` selects runs inside it, using the same request and the same cost and call-record logic as a native run. The prompt is always delivered on **stdin**; it is never placed on the command line, so argument-length limits and shell quoting cannot affect it (quotes, newlines, `$(...)` and backticks in a case input arrive verbatim, and large prompts do not deadlock against output).

| Backend | What runs in the container | Needs |
|---------|----------------------------|-------|
| `kiro-cli` | `kiro-cli` directly: `kiro-cli chat --agent <agent> --no-interactive --trust-tools=<per-agent set> [--model <model>]`, prompt on stdin (turns 2..n of a multi-turn case add `--resume` after `--no-interactive`, in the same container). The argument list and the output handling (ANSI stripped, usage estimated, model = the pinned `--model`) are the same code the native backend uses; the only difference is that the native backend passes `--trust-all-tools` where the container passes `--trust-tools=…` (see [Tool trust](#tool-trust)). | `kiro-cli` in the image (baked at image build). The harness only verifies it is present (`ValidateKiroCLI`); it installs nothing. Also the static linux `kairon` binary (see below), so a sandboxed agent can call `kairon`, for example to start a `kairon` MCP server. |
| `stub` (and any other non-`kiro-cli` backend) | The backend runs in-process in the container through a hidden helper command, `kairon inference-exec --backend <name>`. The host mounts a linux `kairon` binary read-only at `/usr/local/bin/kairon` (the same mount every backend gets, see [The `kairon` binary](#the-kairon-binary)), sends the request as one JSON document on stdin, and reads one JSON result on stdout. The stub reads its script (`stub.turns`, including `commands` and `tool_calls`) from that request, and `WorkDir` is the container workspace path. The request carries the trust set and the result carries any tool denials, so the stub's trust gate runs inside the container. | A static linux `kairon` binary for the container's platform (see below), as for every backend. `kiro-cli` is not validated. |

`kairon inference-exec` is an internal protocol between the harness and its own binary; it is hidden from `kairon --help` and is not meant to be run by hand.

#### The `kairon` binary

The container image has no `kairon`, so every `--sandbox` run needs a static linux `kairon` binary matching the container's platform (`linux/amd64` or `linux/arm64`, the host's architecture), whichever backend is selected. The harness bind-mounts it **read-only** at `/usr/local/bin/kairon` for **all** backends, including `kiro-cli`. `/usr/local/bin` is already on the container `PATH` (after the fake-`gh` directory, so the precedence of the fake `gh` and of case mocks is unchanged), which means an agent just runs `kairon`. A sandboxed `kiro-cli` agent can therefore start a `kairon` MCP server, and the stub backend's `kairon inference-exec` helper uses the same path.

The mount is read-only, the file is owned by the host user and the agent runs as uid 1000, so the agent cannot modify or replace the binary. A write to `/usr/local/bin/kairon` is refused with `Read-only file system` or `Permission denied` (a plain write to the running binary reports `Text file busy` first, so the self-test also uses `chmod`); the `stub-kairon-binary-readonly` self-test case proves it (see [Self-Test in the Container Sandbox](#self-test-in-the-container-sandbox)). The binary is mounted, never baked into the image, so the base image tag is unchanged. A case cannot shadow it: `kairon` is a reserved mock command name.

The file must be readable and executable by the `sandbox` user (mode `0755`); otherwise the run fails with an error naming the file. It is resolved in this order:

1. `KAIRON_SANDBOX_BINARY` — path to a prebuilt static linux `kairon` binary. Use this when Go or the source tree is not available, for example `KAIRON_SANDBOX_BINARY=dist/release/kairon-linux-arm64` (see `task build:linux:arm64` / `task build:linux:amd64`).
2. The running executable, when it is itself a linux binary of the container's architecture.
3. Cross-compilation: `CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build -trimpath ./cmd/kairon`, run from the kairon module root (found by walking up from the working directory to a `go.mod` for `github.com/jbrinkman/kairon`). The result is cached under the user cache directory in `kairon/sandbox/`.

Case 3 requires `go` on `PATH` and a kairon source checkout, which is the case for `task eval:selftest:sandbox`.

**A missing binary fails the run before any case starts.** The binary is resolved in the shared pre-flight that runs before the base image is obtained (for a full run, a single-case run and `--resume` alike), so if none of the sources above is available no case runs and nothing is written for one. The error has the form `preparing the kairon binary for the sandbox (<platform>): …` and names `KAIRON_SANDBOX_BINARY`. This applies to the `kiro-cli` backend too: it needs the binary just like `stub`. Native (non-`--sandbox`) runs never need it.

**Verifying the mount for `kiro-cli` by hand.** The automated mount check is a unit test of the mount list; a real `kiro-cli` run needs credentials, so confirm it on a live container like this. Use a case that fails, because `--debug` keeps the container only for a **failed** case (a passing case's container is removed):

```bash
kairon eval <agent> --sandbox --backend kiro-cli --debug --keep-workspaces
docker ps -a --filter name=kairon      # or: podman ps -a …; find the preserved container of the failed case
docker inspect <container> --format '{{range .Mounts}}{{.Destination}} rw={{.RW}}{{"\n"}}{{end}}'
# /usr/local/bin/kairon must be listed with rw=false
```

**Network.** Container *execution* has no network by default, but building the base image does: it downloads `kiro-cli` and `gh`, so the first build needs network access (and a container daemon) even for `--backend stub`. Later runs reuse the cached image (see [When the base image is rebuilt](#when-the-base-image-is-rebuilt)).

**Debugging.** With `--debug` the harness prints `🔧 Debug: container invoke backend=<backend> agent=<agent> model=<model>` before each in-container call, showing which backend and model were sent. The in-container call is bounded by the effective timeout (the case's [`timeout`](#case-timeout), else `--resource-limit timeout=` or the sandbox config), which is also passed to the backend as the request timeout.

### Resource Limits

Containers run with strict resource limits to prevent runaway processes:

| Resource | Default Limit | Environment Variable |
|----------|---------------|----------------------|
| CPU | 1.0 core (1,000,000 μs) | `KAIRON_EVAL_CPU_QUOTA` |
| Memory | 512MB | `KAIRON_EVAL_MEMORY_LIMIT` |
| Timeout | 5 minutes | `KAIRON_EVAL_TIMEOUT` |
| Network | Disabled | N/A |

Configure resource limits via environment variables:

```bash
# Restrict to 0.5 CPU cores and 256MB memory
KAIRON_EVAL_CPU_QUOTA=500000 \
KAIRON_EVAL_MEMORY_LIMIT=268435456 \
kairon eval --sandbox architect

# Set 30-second timeout for quick tests
KAIRON_EVAL_TIMEOUT=30s \
kairon eval --sandbox builder
```

### Sandbox Containment

A `--sandbox` run contains an arbitrary agent at the layers Kairon itself controls. It does not assume a fixed tool set. Three layers are enforced by mechanism, plus a pre-existing network setting that is **not** part of the containment guarantee (see [Limits](#limits)):

| Layer | Mechanism | Enforced by |
|-------|-----------|-------------|
| `gh` | A fake `gh` is bind-mounted read-only and is first on `PATH`. It logs every call, copies `--body-file` bodies and answers `gh issue view` from the case's `gh_issue`. The baked, real `gh` stays reachable by absolute path but is unauthenticated. | Kernel mount and environment |
| Filesystem | Read-only root filesystem. Only the workspace, `.eval/` and a few small tmpfs directories are writable. | Container runtime |
| Tool trust | `--trust-all-tools` is replaced in the container by `--trust-tools=<per-agent set>`. Trust is per tool, not per argument. | `kiro-cli` |
| Network (not a guarantee) | Containers are created with `NetworkMode: none`. This is left exactly as it was and is not a network policy. | Container runtime |

Native runs (without `--sandbox`) are **not** contained: they keep `--trust-all-tools`, use the real `gh` on your machine and write wherever the agent can. For a case that is only safe inside the sandbox, set `requires_sandbox: true` (see [Test Case Format](#test-case-format)).

What a run applied is recorded in its results: `sandbox` and a per-agent `containment` record in `summary.json`, and a native run records no containment. See [Execution mode and containment](#execution-mode-and-containment) (including why the recorded `network` value is `unrestricted`).

For the whole model on one page (layers, owners, limits and extension seams), see [Containment model](#containment-model).

What this does and does not cover, and how to keep a case away from real AWS, `npm publish` and arbitrary HTTP, is the subject of [Preventing Production Side Effects (Containment and Mocking)](#preventing-production-side-effects-containment-and-mocking) below.

#### The fake `gh`

**Delivery.** The fake is a POSIX `sh` script embedded in the `kairon` binary. For each case the host writes it to `<workspace-parent>/bin/gh` (mode `0755`) and bind-mounts that directory **read-only** at `/opt/kairon/bin`. The container `PATH` is `/opt/kairon/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, so a bare `gh` resolves to the fake. Nothing is copied into the running container and the base image is unchanged, so its cache key is too. The script needs no `jq`.

**Files.** The fake finds the outputs directory through `KAIRON_EVAL_DIR`, which is set to `<workspace_dir>/.eval` (so a non-default `sandbox.workspace_dir` works).

| File in `.eval/` | Written by | Content |
|------------------|------------|---------|
| `gh.log` | the fake, on every call | One line per call: `gh <args>`. Args are space-joined with no timestamp (deterministic). An argument is single-quoted only if it is empty or contains a character outside `[A-Za-z0-9_./:=@%+,-]` (for example `gh issue create --title 'Add widget'`); newlines inside an argument are folded to spaces, so a call is always one line. Every invocation is logged first, including ones that then fail. |
| `gh-body-<n>.md` | the fake | A copy of the body passed with `--body-file <path>`, `-F <path>` or `--body-file=<path>`. `n` starts at 1 and takes the next free number (claimed atomically), so a second call writes `gh-body-2.md`. `--body-file -` copies stdin. A missing file fails like real `gh` (`open <path>: no such file or directory`, exit 1) and is still logged. Bodies are copied for every call that carries the flag, including commands the fake does not simulate. `-F` is always read as `--body-file`, so `gh api -F key=value` fails with an open error. |
| `gh-issue.json`, `gh-issue.txt` | the host, before the container starts | The case's `gh_issue` rendered as JSON (one top-level field per line) and as the human `gh issue view` text. Written only when the case defines `gh_issue`. |

Because `.eval/` lives in the host workspace, `gh.log` and the `gh-body-<n>.md` files are on the host the moment they are written. With `--keep-workspaces` you can read them after the run (`cat <workspace_dir>/.eval/gh.log`).

**Behaviour.**

| Call | Result |
|------|--------|
| `gh issue create …` | Prints `https://github.com/fake-owner/fake-repo/issues/<seq>`, exit 0. `<seq>` is a sequence number counting only `create` calls, shared across `gh issue create` and `gh pr create` (so the first create in a case is `1`, the next `2`, …, regardless of other `gh` calls logged in between). It is claimed atomically, so concurrent creates never collide. |
| `gh pr create …` | Prints `https://github.com/fake-owner/fake-repo/pull/<seq>`, exit 0. `<seq>` shares the same create counter as `gh issue create`. |
| `gh issue view [N\|url]` | Answers from the case's `gh_issue`; see below. |
| `gh issue list\|edit\|comment\|close\|reopen`, `gh pr list\|view\|comment\|edit` | Simulated: exit 0 with no output. |
| `gh auth status` | Prints a fake logged-in state and exits 0, so an agent's pre-checks pass. This is the *fake*; it never consults the real binary. |
| `gh --version` | Prints `gh version 2.0.0 (kairon fake gh)`. |
| anything else | Logged, then stderr `kairon fake gh: "<args>" is not simulated (call logged only)` and exit 1. An honest failure rather than a silent success: the call is visible in `gh.log` and you can extend the script. |

**`gh issue view` and `gh_issue`.** A case that wants `gh issue view` to return something sets `gh_issue` (and `requires_sandbox: true`, which loading enforces):

```yaml
requires_sandbox: true
gh_issue:
  number: 42            # default 1
  title: "Add widget"   # required
  body: "..."
  state: OPEN           # default OPEN
  author: octocat       # default fake-user
  labels: [bug]
```

- Without `--json`, `gh issue view` prints `gh-issue.txt`.
- `--json a,b` prints a JSON object holding just those fields, in the order requested. The available fields are `author` (`{"login": …}`), `body`, `labels` (`[{"name": …}]`), `number`, `state`, `title` and `url`. An unknown field prints `Unknown JSON field: "x"` and the available fields to stderr and exits 1.
- A number or URL that differs from the configured `number` exits 1 with a `could not resolve to an issue with the number N` message. If the case has no `gh_issue`, the call exits 1 with `kairon fake gh: no issue configured for this case (gh_issue)`.
- **`--jq`, `-q`, `--template` and `-t` are not supported** (the image has no `jq`). `gh issue view` exits 1 with a message telling the agent to use `--json` and parse the output itself, and the call is still logged. This is a documented limit of the fake, not something to work around by editing the image.

**The real `gh` is unauthenticated.** The baked `gh` is still at `/usr/local/bin/gh`, so an agent that calls it by absolute path reaches the real binary. It cannot authenticate: the container environment is built from scratch and additionally drops `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN` and `GH_HOST` even if they appear in `ContainerConfig.Environment`; `GH_PROMPT_DISABLED=1` and `GH_NO_UPDATE_NOTIFIER=1` are set; and `$HOME` is a fresh tmpfs, so there is no `~/.config/gh/hosts.yml`. `/usr/local/bin/gh auth status` therefore reports "not logged in" and exits non-zero. This does not rely on the network being disabled.

#### Read-only root filesystem

Every container is created with `ReadonlyRootfs: true`. It is always on; there is no opt-out. Only these paths are writable:

| Path | Kind | Notes |
|------|------|-------|
| `<workspace_dir>` (default `/workspace`) | bind mount, read-write | The case's git workspace. |
| `<workspace_dir>/.eval` | bind mount, read-write | Outputs, `gh.log` and `gh-body-<n>.md`. |
| `/tmp` | tmpfs, `rw,nosuid,nodev,mode=1777,size=256m` | Scratch. |
| `/var/tmp` | tmpfs, `rw,nosuid,nodev,mode=1777,size=64m` | Scratch. |
| `/home/sandbox` | tmpfs, `rw,nosuid,nodev,mode=0777,size=256m` | `$HOME`. `kiro-cli` keeps state here, so it must be writable. Ephemeral: it is gone when the container is. No `uid=`/`gid=` option (see the notes below). |

Everything else is read-only, including `<workspace_dir>/.kiro`, `/opt/kairon/bin`, `/usr/local/bin/kairon` and the rest of the image. A write outside the writable paths fails with `Read-only file system` (EROFS), which is distinguishable from the `Permission denied` the unprivileged `sandbox` user already got for places like `/etc`.

Notes:
- `/home/sandbox` is a deliberate hole in "read-only". It is ephemeral and size-capped, and is the reason a real `kiro-cli` run works at all.
- The `/home/sandbox` tmpfs deliberately has **no `uid=` or `gid=` option**. Docker accepts them, but Podman's Docker-compatible API rejects them at container create (`unknown mount option "uid=1000": invalid mount option`), which would make every sandbox container fail on Podman hosts. The mount uses `mode=0777` instead, so it is writable by the unprivileged `sandbox` user (uid 1000) through the "other" permission bits even though the runtime owns the directory as `root:root`. World-writable is acceptable here because the tmpfs is private to one container, ephemeral, size-capped and used by a single unprivileged user. The same options work on Docker and Podman.
- `noexec` is not set on the tmpfs mounts: `kiro-cli` and tools may execute from temporary locations.
- tmpfs memory counts against the container memory limit (see [Resource Limits](#resource-limits)).
- Not added, and possible further hardening: dropping Linux capabilities, `no-new-privileges` and a pids limit.

#### Tool trust

Tool trust applies to **container runs only**. A native run keeps `kiro-cli chat … --trust-all-tools`; under `--sandbox` the agent call gets a single argument `--trust-tools=<csv>` in its place (`--trust-tools=` when the set is empty, which trusts no tools). The trust set is per tool, not per argument.

**Resolution.** For each agent, the first match wins:

1. `evals.trust_tools[<agent>]` in `.kairon/config.yaml`: an explicit override. It may be an empty list, which trusts nothing.
2. The agent config's `allowedTools`, read from the same file the model pinning uses: `<evals-dir>/agents/<agent>.json` if it exists, else `.kiro/agents/<agent>.json`.
3. Neither: the empty set (`--trust-tools=`). The default **fails closed**. An agent config without an `allowedTools` key trusts nothing.

If there is no override and the agent config cannot be found or read, the call fails with a `tool trust: …` error that lists the paths tried; the agent is not run, and the failed call is still recorded.

`*` and empty entries in `evals.trust_tools` are rejected when the config is loaded (the error names the agent), so trusting all tools cannot be restored by accident. `toolsSettings` in an agent config (for example `shell.autoAllowReadonly` or `write.allowedPaths`) stay owned by the agent author and pass through in the staged `.kiro` unchanged; Kairon generates none.

```yaml
evals:
  trust_tools:
    builder: [read, write]   # narrower than the shipped builder allowedTools
    validator: []            # trust nothing
```

**Name normalization.** The short names used in agent configs are rewritten to the spelling `kiro-cli --trust-tools` expects. The table lives in one place (`internal/inference/trust.go`). Duplicates are dropped and the first-seen order is kept; surrounding whitespace is trimmed.

| In `allowedTools` / `trust_tools` | Passed to `--trust-tools` |
|-----------------------------------|---------------------------|
| `read` | `fs_read` |
| `write` | `fs_write` |
| `shell` | `execute_bash` |
| `aws` | `use_aws` |
| `fs_read`, `fs_write`, `execute_bash`, `use_aws` | unchanged |
| any other name (`web_search`, `web_fetch`, `subagent`, `todo_list`, `@server/tool`, …) | unchanged |

**Default per-agent trust sets.** With no `trust_tools` override, the trust set is the agent's own `allowedTools`. For the agents that `kairon init` ships:

| Agent | `allowedTools` | Passed to `--trust-tools` |
|-------|----------------|---------------------------|
| `architect` | `read`, `write`, `shell` | `fs_read,fs_write,execute_bash` |
| `builder` | `read`, `write`, `shell` | `fs_read,fs_write,execute_bash` |
| `validator` | `read`, `write`, `shell` | `fs_read,fs_write,execute_bash` |
| `documenter` | `read`, `write` | `fs_read,fs_write` |
| `planner` | `read`, `write`, `shell`, `web_search`, `web_fetch` | `fs_read,fs_write,execute_bash,web_search,web_fetch` |
| `krew-lead` | `read`, `shell`, `subagent`, `todo_list` | `fs_read,execute_bash,subagent,todo_list` |

If you edit an agent's `allowedTools` (or add MCP entries), the default follows it; entries are passed through as written. The self-test agents use `allowedTools` too: `selftest-sandbox` trusts `fs_read,fs_write` and `selftest-sandbox-ro` trusts `fs_read`.

**What is recorded.** Each container agent call carries `trusted_tools` in its [call record](#per-call-records-calls): the normalized set it ran with, `[]` when restricted to nothing, absent for native calls.

**The stub backend models the plumbing.** With `--backend stub`, a turn's `tool_calls` run only if the tool is in the trust set; a denied call is skipped (its command does not run) and added to the call's `tool_denials` with reason `tool not trusted`. For example, with `selftest-sandbox-ro` (trust set `fs_read`) an `fs_write` tool call leaves no file behind and records a denial, while the same call under `selftest-sandbox` (trust set `fs_read,fs_write`) runs. This is a deterministic, network-free model of the harness plumbing; the real enforcement is `kiro-cli`'s.

#### Limits

Containment here is deliberately narrow. These are the boundaries it does **not** provide:

- **Trust is whole-tool, not per-argument.** Trusting `execute_bash` trusts every shell command the agent runs; trusting `fs_write` trusts every path it can write. Kairon does not inspect or constrain arguments, and per-argument interception is not built. It is an extension seam for later work. `toolsSettings` an agent author wrote (for example `allowedPaths`) are passed through as written; Kairon neither generates nor verifies them.
- **The network is not an enforced boundary.** None of the layers above severs or gateways network access. Containers are currently created with `NetworkMode: none`, which this work leaves exactly as it was; it is not a policy to rely on, and it is the reason a real `kiro-cli` run in the container cannot reach a model endpoint today. **Side effects over the network are the eval author's responsibility, handled with mocks** (for example a fake of the service a case would otherwise call). The contract, the risk vectors, the working mock pattern and its limits are in [Preventing Production Side Effects (Containment and Mocking)](#preventing-production-side-effects-containment-and-mocking). Mocks are a per-case convention, not an enforced boundary: nothing in the harness stops a case that is given credentials and a network, and that calls a real service it did not mock, from reaching it.
- **`kiro-cli` denials are not detectable.** Refused tool calls cannot be reliably detected from `kiro-cli`'s output, so `tool_denials` is populated by the stub backend only. For a real `kiro-cli` run the record states which tools were **trusted** (`trusted_tools`), and the raw output and stderr are kept as before; it does not state which were denied.
- **Tool names are not verified end to end.** `fs_read` and `fs_write` come from `kiro-cli chat --help` and `execute_bash` from its tool table. That `read`/`write`/`shell` map to those names, and that `web_search`, `web_fetch`, `subagent` and `todo_list` pass through unchanged, is an assumption that CI cannot check (it cannot run a real model); the argument list is unit-tested only. If `kiro-cli` rejects a bare `@server`, list MCP tools as `@server/tool`. How `kiro-cli` combines `--trust-tools` with an agent's own `allowedTools` is not documented: an override wider than `allowedTools` may still be limited by `kiro-cli` itself.
- **The fake `gh` is a fake.** It supports a fixed set of commands, does not support `--jq`/`--template`, and is bypassed by an agent that calls `/usr/local/bin/gh` (which is unauthenticated, by design).
- **Native runs are not contained** (see above).

### Preventing Production Side Effects (Containment and Mocking)

An eval case runs an arbitrary agent with tools. Some of what that agent can do reaches beyond the container: delete cloud resources, publish a package, call a real API. This section states what Kairon prevents, what it leaves to you, and gives a working pattern for the part that is yours.

#### The containment contract

| Concern | Who is responsible | How |
|---------|--------------------|-----|
| Filesystem | **Kairon** (enforced) | Read-only root filesystem; only the workspace, `.eval/` and small tmpfs directories are writable; the live repository is never mounted (see [Read-only root filesystem](#read-only-root-filesystem)). |
| GitHub (`gh`) | **Kairon** (enforced) | A fake `gh` is first on `PATH`; the real `gh` is unauthenticated (see [The fake `gh`](#the-fake-gh)). |
| Which tools run | **Kairon** (whole-tool, via `kiro-cli`) | `--trust-tools=<per-agent set>` instead of `--trust-all-tools` (see [Tool trust](#tool-trust)). |
| Network and every other production side effect | **The eval author** | Write cases against mocks, as described below. |

Stated plainly: Kairon enforces the filesystem boundary and the fake `gh`. The agent is the LLM client and keeps network access for model access. Preventing any other production side effect is the eval author's responsibility, done by writing the case against mocks. (Containers are currently created with `NetworkMode: none`. That is a plain setting, not a network policy, and not something to rely on; see [Limits](#limits).)

#### Risk vectors

These are the ways a case with credentials and a network can leave a mark outside the sandbox:

- **Real AWS / cloud SDK calls.** An agent that runs `aws s3 rm`, `aws iam delete-user`, `gcloud …`, `az …` or `kubectl delete …` acts on whatever account the environment's credentials reach. Mock the CLI (`mocks:` with `command: aws`, `gcloud`, `kubectl`, …).
- **`npm publish`** (and the other publish and registry CLIs: `cargo publish`, `twine upload`, `docker push`, …). A publish is public and often irreversible. Mock the CLI, or point the tool at a registry that cannot be reached with project config in the workspace fixture (see [HTTP endpoints](#http-endpoints-and-in-process-sdks)).
- **Arbitrary HTTP to real services.** `curl`, `wget`, a script's HTTP client, a webhook post. Mock the client CLI, or point the tool's endpoint at a stand-in through workspace config.

**Why Kairon does not network-gateway these.** The obvious alternative is an egress allowlist or proxy. That is an application-level gateway: it has to classify every destination, and it still cannot separate a legitimate model call from an exfiltration to the same host, because the agent is itself the model client and must be allowed to talk to the model endpoint. The complexity would buy a filter that looks like a boundary and is not. Kairon enforces the layers it can enforce cheaply and leaves the rest to mocks, which keep a case away from a real service by construction instead of by filtering.

#### The mock pattern

A mock is a script placed first on the container `PATH`, so a bare `aws`, `npm` or `curl` runs the script instead of a real tool. It reuses the mechanism of the fake `gh`: the host stages the script into `<workspace-parent>/bin/<command>` (mode `0755`), that directory is bind-mounted read-only at `/opt/kairon/bin`, and `/opt/kairon/bin` comes first on `PATH`. The agent cannot modify the script. `PATH` applies to every process in the container, so it reaches the commands a real `kiro-cli` agent runs through its shell tool as well as the stub backend's `commands`.

Three pieces, all following the fake-`gh` precedent:

| Piece | Where | Role |
|-------|-------|------|
| The shim | [`fixtures/mock-cli.sh`](../.kairon/evals/fixtures/mock-cli.sh) in your evals directory (shipped by `kairon init`) | One POSIX `sh` script that can stand in for **any** CLI. It behaves according to the name it is installed as. |
| Canned replies | `fixtures/workspaces/<name>/.mocks/<command>/` | Backing data, committed with the workspace fixture so `git status` stays clean. |
| Case wiring | the case's `mocks:` field | Declares which commands are mocked and which script implements each. |

**Fixture layout** (copy this tree; `mock-aws` is the name used by the self-test):

```
.kairon/evals/
  fixtures/
    mock-cli.sh                     # the shim (copied by `kairon init`; do not edit it per case)
    workspaces/
      mock-aws/                     # referenced by `workspace: mock-aws`
        report.txt                  # whatever the case works on
        .mocks/
          aws/                      # data directory for the command "aws"
            s3-cp.out               # reply for `aws s3 cp …`
            default.out             # optional: reply for any other aws call
  cases/
    builder/
      upload-report.yaml
```

**Case YAML:**

```yaml
name: upload-report-uses-mocked-aws
description: "The agent uploads the report with the aws CLI; the call must hit the mock, not a real account"
agent: builder
requires_sandbox: true        # required whenever mocks is set
workspace: mock-aws           # fixtures/workspaces/mock-aws/ holds the canned replies
mocks:
  - command: aws                      # bare command name placed first on PATH
    script: fixtures/mock-cli.sh      # path relative to the evals directory
input: |
  Upload report.txt to s3://prod-bucket/ with the aws CLI and report the result.
```

`mocks[].command` and `mocks[].script` are validated when the cases are loaded (see [Test Case Format](#test-case-format)): `requires_sandbox: true` is mandatory (a native run has no mock directory on `PATH` and would call the real tool), `command` must be a bare name, unique in the case and not `gh`, and `script` must be an existing regular file inside the evals directory.

**What the shim does**, per call:

1. Appends one line, `<command> <args>`, to `.eval/mock-<command>.log` **before** anything else, so a failing or unsimulated call is logged too. Quoting matches the fake `gh`'s `gh.log`: arguments are space-joined with no timestamp, an argument is single-quoted only if it is empty or contains a character outside `[A-Za-z0-9_./:=@%+,-]`, and newlines are folded to spaces.
2. Looks for a canned reply in the data directory: `$KAIRON_MOCK_DATA` if set, else `.mocks/<command>/` in the workspace. It takes the first two arguments that do not start with `-` (`a1`, `a2`), keeps only `[A-Za-z0-9_]` in each, and tries `<a1>-<a2>.out`, then `<a1>.out`, then `default.out`. The first file that exists is printed to stdout verbatim.
3. Exits with the integer in the sibling `<name>.rc` if there is one (for example `255` to simulate `AccessDenied`), else `0`.
4. If nothing matches, prints `kairon mock <command>: "<args>" is not simulated (call logged only)` to stderr and exits `1`. An unscripted destructive call is recorded and never executed.

The shim never runs another binary (so a real `aws` elsewhere on `PATH` is never invoked) and never opens a connection. It needs `KAIRON_EVAL_DIR`, which the sandbox sets to `<workspace_dir>/.eval`.

For the case above, with `.mocks/aws/s3-cp.out` containing `upload: ./report.txt to s3://prod-bucket/report.txt (kairon mock aws)`:

| The agent runs | The shim prints | `.eval/mock-aws.log` gains |
|----------------|-----------------|----------------------------|
| `aws s3 cp report.txt s3://prod-bucket/report.txt` | `upload: ./report.txt to s3://prod-bucket/report.txt (kairon mock aws)`, exit 0 | `aws s3 cp report.txt s3://prod-bucket/report.txt` |
| `aws iam delete-user --user-name prod-admin` | stderr `kairon mock aws: "iam delete-user --user-name prod-admin" is not simulated (call logged only)`, exit 1 | `aws iam delete-user --user-name prod-admin` |

The working example is the self-test case [`stub-mock-cli`](../internal/eval/testdata/evals/cases/selftest-sandbox/stub-mock-cli.yaml) (agent `selftest-sandbox`, workspace fixture `mock-aws`); the self-test's own evals directory holds a byte-identical copy of the shim, because case scripts resolve against the evals directory in use. Run it with:

```bash
go run ./cmd/kairon eval --backend stub --sandbox --keep-workspaces --evals-dir internal/eval/testdata/evals selftest-sandbox
cat <workspace_dir>/.eval/mock-aws.log     # workspace_dir is printed on the case line and recorded in the results
```

**Adapting it to another tool.** The same script mocks any CLI; change `command:` and ship the replies under `.mocks/<command>/`:

```yaml
mocks:
  - command: npm
    script: fixtures/mock-cli.sh      # .mocks/npm/publish.out answers `npm publish`; no match -> logged, exit 1
  - command: curl
    script: fixtures/mock-cli.sh      # .mocks/curl/default.out answers every curl call
```

For `curl` and `wget` use `default.out`: the first non-flag argument is a URL, and removing every character outside `[A-Za-z0-9_]` from it gives a name nobody wants to type. A flag's *value* is not skipped when choosing `a1` and `a2` (write `aws s3 cp --profile x`, or provide `default.out`). If you need richer behaviour (different replies by argument, stdin handling), copy `mock-cli.sh` under a new name in `fixtures/` and edit the dispatch at the bottom; its header comment documents the contract. Keep it POSIX `sh`: the image uses busybox `ash` and has no `jq`.

##### HTTP endpoints and in-process SDKs

Kairon does not run a mock HTTP server: the base image has none, and a case that needs one ships it in its own image or workspace. Without one, there are two ways to keep HTTP traffic off a real service, both of which work today:

- **Shim the client CLI** (`curl`, `wget`, `aws`, `npm`, `gcloud`) with `mock-cli.sh`, as above. This is what `mocks:` is for.
- **Point the tool at a stand-in through project config that lives in the workspace fixture.** The agent's working directory is the workspace, so such files are honoured. For example a `.npmrc` containing `registry=http://127.0.0.1:4873` (an address nothing answers on, so a publish fails instead of reaching the public registry), or a tool's endpoint override in its own config file.

An SDK called in-process by code the agent writes or runs (a Python script using `boto3`, a Node script using `fetch`) does not look up a command on `PATH`, so a `PATH` mock never sees it. It is covered only by the config route (endpoint overrides the SDK honours), by not running such code in the case, or by trusting fewer tools (below).

**Inspecting and scoring.** Everything the mock recorded is on the host the moment it is written, because `.eval/` lives in the host workspace. Run with `--keep-workspaces` and read `<workspace_dir>/.eval/mock-<command>.log`. A case can score the recorded interaction with [`checks`](#checks): `file_contains` / `file_not_contains` on `.eval/mock-<command>.log` read the same host file (for example `type: file_contains`, `path: .eval/mock-aws.log`, `pattern: 's3 cp'`). The daemon-gated `TestContainmentSandbox` (subtest `mocked cli`) also reads the kept workspace and asserts the exact log, the resolution of `aws` to `/opt/kairon/bin/aws`, the canned reply, the `is not simulated` message and the `git status` of the workspace.

#### Mocks and tool trust

[Tool trust](#tool-trust) and mocks are complementary and solve different halves of the problem:

- **Trust limits which tools run.** Each tool removed from `evals.trust_tools[<agent>]` (or from the agent's `allowedTools`; see the per-agent default trust table under [Tool trust](#tool-trust)) is one less surface a case has to mock.
- **Mocks make the tools that do run safe.** A trusted `execute_bash` can run any command. The shim decides what a bare `aws`, `npm` or `curl` does when it does.

The caution: the `PATH` shim intercepts commands resolved through `PATH` by a trusted **shell** tool. Built-in and MCP tools do their own I/O and are **not** intercepted by it: `use_aws` (the `aws` alias), `web_fetch`, `web_search` and MCP tools (`@server/tool`). A case that relies on a mock for AWS or HTTP should leave those out of the trust set, because a mock cannot cover them. The shipped `planner` agent, for instance, trusts `web_search` and `web_fetch` by default. For a builder-style agent that you want to keep off real AWS and HTTP:

```yaml
# .kairon/config.yaml
evals:
  trust_tools:
    builder: [read, write, shell]     # no aws / use_aws, web_fetch, web_search or MCP entries
```

This is a statement about the `PATH` shim, not about `kiro-cli` internals. How `kiro-cli` combines `--trust-tools` with an agent's own `allowedTools` is not documented (see [Limits](#limits)), so check the recorded `trusted_tools` on the call if it matters.

#### Limits of the mock approach

- **A convention, not an enforced boundary.** Mocks protect a case only if its author writes them. A case that calls a real service without a mock is not prevented by Kairon: with credentials in reach and a network, nothing in the harness stops it.
- **Bypasses.** A mock covers calls resolved through `PATH`. An absolute path (`/usr/local/bin/aws`), an in-process SDK, or a built-in or MCP tool (`use_aws`, `web_fetch`, …) does not go through it.
- **Not tamper-proof on the workspace side.** The shim file is read-only (it lives in the read-only `/opt/kairon/bin`), but the canned replies (`.mocks/`) and the log (`.eval/`) are in the agent-writable workspace. The log is a record for the eval author, not an audit trail.
- **The self-test shows wiring, not avoidance.** The base image contains no `aws` or `npm`, so `stub-mock-cli` can show which binary a bare `aws` resolves to, what is recorded and that an unsimulated destructive call is not executed. It cannot show a real `aws` being avoided. In a project whose image does contain `aws`, the same wiring is what keeps the call off the real service.
- **Trade-off against an app gateway.** A gateway would be a mechanical boundary that does not depend on the author remembering to mock. The price is a destination classifier that cannot tell model traffic from exfiltration and that someone has to maintain per environment. Kairon accepts the weaker, simpler convention, and this section is how it is made usable.

### Containment Model and Extension Seams

This section is the one-page map of how a `--sandbox` run is contained: which layers exist, who owns each, whether it is enforced or a convention, where it stops, and where a consuming project can extend it. It links to the detailed sections rather than restating them; if the two ever disagree, the detailed section wins.

#### Why a container, not a convention

Kairon's earlier plan for Stage 3 isolation (item E4 in `.kairon/specs/maturity-model/gap-analysis.md`) was convention-based: run each case in a temporary workspace, put a `gh` shim first on `PATH` and point `GH_CONFIG_DIR` at an empty directory. **That design is superseded by the container sandbox.** A convention only holds for an agent that cooperates. Kairon runs arbitrary, extensible third-party agents (MCP servers, built-in tools that do their own I/O, scripts that call binaries by absolute path such as `/usr/local/bin/gh`, writes outside the current directory), and a working-directory change or a `PATH` shim cannot stop any of them. The container gives a real mount-level boundary (a read-only root filesystem and a short list of bind mounts), and everything the boundary cannot cover is placed in a named layer with a named owner, below. What E4 contributed beyond isolation (per-case git workspaces, staged `.kiro/`, `.eval/`, timeouts, `--keep-workspaces`) is kept and is shared by native and sandbox runs (see [Case Workspaces](#case-workspaces)).

#### Containment model

| Layer | Owner | Mechanism | Enforced? | Honest limit |
|-------|-------|-----------|-----------|--------------|
| Filesystem | Kairon | Read-only root filesystem plus bind mounts: staged `.kiro/` read-only, workspace and `.eval/` read-write, `/opt/kairon/bin` and the `kairon` binary at `/usr/local/bin/kairon` read-only. The live repository is never mounted. | **Enforced** (container runtime) | Writable holes are the workspace, `.eval/` and the tmpfs `/tmp`, `/var/tmp` and `/home/sandbox`. The workspace is world-writable on the host while a run is in flight. Linux capabilities are not dropped and there is no pids limit. |
| `gh` | Kairon | A fake `gh` is first on `PATH` in a read-only mount; the real `gh` at `/usr/local/bin/gh` is unauthenticated (no token variables, fresh `$HOME`). | **Enforced** (mount and environment) | The fake supports a fixed command set and no `--jq`/`--template`. An agent can still run `/usr/local/bin/gh`, which reports "not logged in". |
| Tool trust | Kairon chooses the set; `kiro-cli` enforces it | `--trust-tools=<per-agent set>` instead of `--trust-all-tools`. | **Enforced, whole-tool only** | There is no per-argument or per-call tool hook in this `kiro-cli` build: trusting `execute_bash` trusts every shell command, and trusting `fs_write` trusts every writable path. `kiro-cli` denials are not detectable (`tool_denials` is populated by the stub backend only). How `--trust-tools` combines with an agent's `allowedTools` is undocumented, and the tool-name mapping is not verified end to end. |
| Network side effects (AWS, `npm publish`, HTTP, any other CLI) | **Eval author** | Per-case mocks (`mocks:` with `fixtures/mock-cli.sh`) and endpoint config in the workspace fixture. | **Not enforced** (convention) | There is no network gateway. `NetworkMode: none` is a pre-existing setting, not a network policy, and is not part of the guarantee. Network side effects are the eval author's responsibility via mocks. A `PATH` mock misses built-in and MCP tools, absolute paths and in-process SDKs. |
| Native runs (no `--sandbox`) | Eval author / operator | None. `requires_sandbox: true` makes a case refuse to run natively. | **Not contained** | `--trust-all-tools`, the real `gh`, writes wherever the agent can. |
| Recording | Kairon | `sandbox` plus a per-agent `containment` record (`tool_trust`, `fake_gh`, `read_only_fs`, `kairon_binary`, `network`) in the results; `eval diff` reports a mode difference. | **Enforced by code** | It records what was applied, not whether it was sufficient. `network: "unrestricted"` records the absence of a guarantee, not the container's network mode. |

Reading the table: **enforced** means a mechanism stops the action whatever the agent does; **convention** means the protection exists only if the eval author wrote it (for example the mock). The model is deliberately not airtight, and the rows marked "not enforced" or "not contained" are the ones to review when you add a case.

##### What the model does not defend against

- Code that runs in-process and talks to the network without going through `PATH` (an SDK called from a script the agent writes or runs).
- Built-in and MCP tools that are trusted: `use_aws`, `web_fetch`, `web_search` and `@server/tool` do their own I/O, so the `PATH` shim does not intercept them and their network side effects are uncontrolled. Their filesystem writes are still subject to the read-only root filesystem and bind mounts like any other process in the container.
- Credentials that a consuming project passes into the container environment. Kairon drops the GitHub credential variables; it does not drop others.
- A trusted `execute_bash` doing anything the container can do inside its writable mounts.
- Network access the agent itself needs. The agent is its own model client, so a destination filter could not tell model traffic from exfiltration (see [Why Kairon does not network-gateway these](#risk-vectors)).

#### Extension seams

Each seam is what you edit, what it does and where it stops.

- **Add a mock for a CLI (`PATH` shim).** Edit the case YAML (`mocks: [{command, script}]` with `requires_sandbox: true`), use `fixtures/mock-cli.sh`, and ship canned replies under `fixtures/workspaces/<name>/.mocks/<command>/`. For logic the shim cannot express, copy it under a new name in `fixtures/`. The script is staged into `/opt/kairon/bin` (read-only) and is first on `PATH`. It stops at what resolves through `PATH`. See [The mock pattern](#the-mock-pattern).
- **Point a tool at a stand-in endpoint (workspace config).** Put project config in the workspace fixture, for example a `.npmrc` with `registry=http://127.0.0.1:4873` or a tool's own endpoint override file. The agent's working directory is the workspace, so the file is honoured. It stops at in-process SDKs that ignore config. See [HTTP endpoints and in-process SDKs](#http-endpoints-and-in-process-sdks).
- **Set per-agent tool trust.** Set `evals.trust_tools.<agent>` in `.kairon/config.yaml` (an explicit override, `[]` trusts nothing) or edit the agent's `allowedTools` (the default). With neither, the set is empty, so the default fails closed; `*` and empty entries are rejected at load. The resolved set is recorded as `trusted_tools` on each call and as `containment.tool_trust` per agent. Leave `use_aws`, `web_fetch`, `web_search` and `@server/tool` out of a case that relies on a `PATH` mock. It stops at whole-tool granularity. See [Tool trust](#tool-trust) and [Mocks and tool trust](#mocks-and-tool-trust).

  ```yaml
  evals:
    trust_tools:
      builder: [read, write]   # narrower than the shipped builder allowedTools
      validator: []            # trust nothing
  ```

- **Extend the fake `gh`.** The fake is a script embedded in the `kairon` binary. A `gh` command it does not simulate is logged and exits 1; that is the signal that the fake needs extending. This is a code change to Kairon, not a case-level setting. See [The fake `gh`](#the-fake-gh).
- **Future: a per-tool-call hook (the preferred fine-grained layer).** **This does not exist in this build.** The `kiro-cli` used here exposes only whole-tool `--trust-tools`, and Kairon has no code for a per-tool-call or per-argument hook; its interface is undefined, and no schema, config key or timeline is committed. If `kiro-cli` gains one, it would plug in next to whole-tool trust: trust is resolved in one place (`resolveTrustSet` in `internal/eval/trust.go`, producing an `inference.ToolTrust`) and applied in the `kiro-cli` argument list built for the container call, and the `containment` record (a field alongside `tool_trust`) is where it would be reported. It would be the preferred way to constrain `execute_bash`, `fs_write` or `use_aws` by argument, and it would reduce, not remove, the reliance on mocks. It would not replace the filesystem layer.
- **Future: a network gateway.** Also not built. It is not preferred, for the reason given under [Risk vectors](#risk-vectors); it would be the second seam if a deployment needs a mechanical network boundary.

#### Where each concern is documented in detail

- [Sandbox Containment](#sandbox-containment): the layer summary and [Limits](#limits)
- [The fake `gh`](#the-fake-gh)
- [Read-only root filesystem](#read-only-root-filesystem)
- [Tool trust](#tool-trust)
- [Preventing Production Side Effects (Containment and Mocking)](#preventing-production-side-effects-containment-and-mocking), including [The mock pattern](#the-mock-pattern)
- [Execution mode and containment](#execution-mode-and-containment): what a run records
- [Security Considerations](#security-considerations)

### Container Lifecycle

Each evaluation follows this lifecycle:

1. **Base image** - once per run, `EnsureBaseImage` reuses the cached tools-only image or builds it (see [When the base image is rebuilt](#when-the-base-image-is-rebuilt)). Nothing is built or removed per case.
2. **Workspace** - on the host, build the case's git workspace, staged `.kiro/`, `.eval/` and the `bin/` directory with the fake `gh` and any case mocks (see [Case Workspaces](#case-workspaces)).
3. **Create** - create the container from the base image with resource limits, no network, a read-only root filesystem with small tmpfs mounts, the `sandbox` user, the [mounts](#mounts) (including the read-only directory at `/opt/kairon/bin` that holds the fake `gh` and any case mocks) and an environment whose `PATH` starts with it.
4. **Execute** - run the selected backend inside the container, wrapped for the open umask, with the prompt on stdin (see [Backends in the Container](#backends-in-the-container)). For `kiro-cli` the harness first checks (read-only) that `kiro-cli` is present, and the agent is started with `--trust-tools=<per-agent set>` (see [Tool trust](#tool-trust)). A [multi-turn case](#multi-turn-cases) repeats this step once per turn **in the same container** (turns 2..n add `--resume`); the container is created and validated once, when the first turn runs.
5. **Score** - score the case while the host workspace still exists.
6. **Cleanup** - stop and remove the container, then delete the workspace unless `--keep-workspaces` is set. A multi-turn case has one container, removed after its last turn.

### Troubleshooting Container Issues

**Container daemon not running:**
```bash
# Ensure the Docker daemon is running
sudo systemctl start docker   # Linux
open -a Docker               # macOS

# Or start the Podman machine
podman machine start
```

**Permission denied:**
```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
newgrp docker
```

Permission errors *inside* the workspace (`mkdir: can't create directory … Permission denied`) should not occur: the workspace and `.eval/` are mounted read-write and opened to all users, and `.kiro/` is intentionally read-only. If you see one, check that you are not writing to `.kiro/` and that `KAIRON_EVAL_WORKSPACE_ROOT` points at a path your runtime shares with its VM.

**`Read-only file system`:**
The container's root filesystem is read-only. Only the workspace, `.eval/`, `/tmp`, `/var/tmp` and `/home/sandbox` are writable (see [Read-only root filesystem](#read-only-root-filesystem)). A tool that writes elsewhere (a cache under `/var/cache`, a file in `/usr/local`, …) fails with this error; point it at `/tmp` or the workspace instead.

**Workspace mount fails (macOS):**
```bash
# Put workspaces under a directory the VM shares
KAIRON_EVAL_WORKSPACE_ROOT="$HOME/.cache/kairon-eval" kairon eval --sandbox
```

**Out of memory:**
```bash
# Check container resource usage
docker stats

# Increase memory limit
KAIRON_EVAL_MEMORY_LIMIT=1073741824 kairon eval --sandbox
```

**Timeout errors:**
```bash
# Increase timeout for complex evaluations
KAIRON_EVAL_TIMEOUT=10m kairon eval --sandbox

# Or set it for one case in its YAML:  timeout: 10m
```

**Build failures:**
```bash
# The first run builds the base image and needs network access (kiro-cli and gh are downloaded)
docker images | grep kairon-eval-base

# Inspect a kept workspace and a preserved debug container
kairon eval --sandbox --debug --keep-workspaces
```

**Network connectivity:**
Containers are created with `NetworkMode: none`. That is a plain setting, not a network policy, and not something to rely on for containment (see [Limits](#limits)). If a case needs a service, give it a mock rather than a network (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking)).

### Security Considerations

Container sandboxing provides multiple security layers:

- **Process isolation** - Containers run in separate namespaces
- **Resource limits** - CPU and memory usage restricted
- **Network isolation** - No external network access by default (not a containment guarantee; see [Limits](#limits))
- **User isolation** - Runs as the non-root `sandbox` user (uid 1000)
- **Read-only root filesystem** - only the workspace, `.eval/` and small tmpfs directories (`/tmp`, `/var/tmp`, `/home/sandbox`) are writable
- **Read-only agent configuration** - the staged `.kiro/` is mounted read-only, and the live repository is never mounted
- **Fake `gh`, no real GitHub access** - a fake `gh` is first on `PATH` and logs every call; the real `gh` is unauthenticated and GitHub credential variables never reach the container
- **Case mocks** - a case can place its own mock of any other command (`aws`, `npm`, `curl`, …) in the same read-only directory; this is opt-in per case, not enforced (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking))
- **Whole-tool trust** - `kiro-cli` runs with `--trust-tools=<per-agent set>` instead of `--trust-all-tools`
- **Temporary containers** - Automatically cleaned up after evaluation

For the layers, owners and extension seams on one page, see [Containment model](#containment-model).

Limits of this layer: tool trust is per tool, not per argument; the network is not an enforced boundary, so network side effects are the eval author's responsibility via mocks (see [Preventing Production Side Effects](#preventing-production-side-effects-containment-and-mocking); mocks are a per-case convention, not an enforcement); `kiro-cli`'s own tool denials cannot be detected; Linux capabilities are not dropped and there is no pids limit; and the workspace is world-writable on the host while a run is in flight. See [Limits](#limits) for details.

## Comparing Runs

```bash
kairon eval diff <runA> <runB>
```

The `eval diff` command shows, in this order:
- A **Run Provenance** block (below)
- Per-criterion score deltas per agent
- Token and cost deltas
- Quality-per-dollar assessment

**Incomplete costs.** If either run's `total_cost` is `cost_incomplete` (a call used a model with no configured price, see [Unknown cost](#unknown-cost)), the diff says so instead of comparing two lower bounds as if they were totals: the `Cost:` delta line is suffixed ` (incomplete: <side(s)>)`, naming which run(s) are incomplete, and each quality-per-dollar line is suffixed ` (cost incomplete)`. Runs written before cost marking existed have no `cost_incomplete` and are read as complete, so they diff exactly as before.

Runs made with [`--repeat`](#repeat-runs---repeat) diff like any other. The per-criterion averages and the cost cover every result of the run (so they are means and totals over the repeats), and the case output comparison uses run 1 of each case, so it does not depend on how many repeats either run used. The spread (`score_min`, `score_max`, `repeat_scores`) is not shown by `eval diff`; read it from `summary.json`.

### Run Provenance

Scores are only comparable when the runs were produced under comparable conditions, so the diff starts with a `Run Provenance` block, printed before any deltas. It always shows each run's execution mode, then lists every difference between the two runs in:

- `sandbox` (execution mode: `native` or `container`),
- `containment`, per agent (`tool_trust`, `fake_gh`, `read_only_fs`, `kairon_binary`, `network`; a run with a containment record against one without is shown as `record: none → …`),
- `judge_model`,
- `agent_model`, per agent,
- `prompt_sha256`, per agent,
- `prompt_file`, per agent (the `--prompt-file` candidate path; see [Evaluating a candidate prompt](#evaluating-a-candidate-prompt---prompt-file)).

Trust sets are compared ignoring order. A value a run did not record is shown as `(not recorded)`. When nothing differs the block says `No provenance differences.`

Example, a native run against a `--sandbox` run:

```
Run Provenance:
  260620-200919-e369501: mode native
  260621-160207-8a19eb2: mode container
  sandbox: native → container
  selftest containment.record: none → tool_trust=[execute_bash,fs_read,fs_write] fake_gh=true read_only_fs=true kairon_binary=true network=unrestricted
  ⚠ Runs used different execution modes (native vs container); scores are not directly comparable.
```

(The run names and values are illustrative.)

**Mode-difference warning.** When both runs have a known execution mode and the modes differ, the block ends with a `⚠` line stating that the runs used different execution modes and that their scores are not directly comparable. A native run is not contained and a container run is, so a score difference may come from the mode rather than from the change under test. Compare like with like where you can.

**Old runs show mode `unknown`.** The mode is read from `summary.json`'s `sandbox`; when that is absent, from the `sandbox` boolean in the agent files, provided every agent file records it and they all agree; otherwise it is shown as `unknown (predates sandbox-mode tracking)`. If either run's mode is unknown the `⚠` line is replaced by a note that comparability cannot be verified, because nothing can be said about whether the modes differ.

**The provenance block only reports.** It never fails the diff and never changes the deltas that follow. All the provenance and mode fields are optional, so **result directories written before this tracking existed still load**: a missing field shows as `(not recorded)` and the mode as `unknown`, and a malformed agent file is skipped as before.

**Legacy results still load.** Result directories written before pass/fail verdicts existed (no `agent_verdicts`, no `threshold` or `passed` on agents and cases) load and diff exactly as before. The agent-level score in the diff comes from `agent_scores`, a 0–1 fraction, which is unchanged. The per-criterion averages in `eval diff` still leave skipped criteria out, so historical runs diff as they always did; only runs produced after the counting change score skipped criteria as 0 in `agent_scores`, so a diff across that boundary can show a lower agent score that reflects the new counting rule rather than a change in the agent.

## Attempt trend (`kairon eval trend`)

```bash
kairon eval trend <agent> [--json] [--evals-dir <dir>]
```

`eval trend` lists one agent's results across every run under `<evals-dir>/results/`, oldest first, so you can read the score and the [attempt success rate](#agent-verdict) over time. It only reads saved results: it calls no model and no container, and it writes nothing. Production workflow metrics (what the watcher saw on real issues) are out of scope; the trend covers eval runs only.

| Flag | Description |
|------|-------------|
| `<agent>` | Required, exactly one argument. A single path element: a name containing `/` or `..` is rejected. |
| `--json` | Print a JSON array instead of text lines. |
| `--evals-dir <dir>` | The [evals directory](#evals-directory---evals-dir) whose `results/` is read. Inherited from `kairon eval`; the default is the same as for the other `eval` commands. |

**Which runs are listed.** Every directory directly under `<evals-dir>/results/` that has a score for the agent. The score is found the same way as for [`verify-log`](#where-the-recorded-score-comes-from): the `agent_verdicts` score in `summary.json`, else `agent_scores`, else the scoreable cases of `<agent>.json`. Files, directories with no data for the agent, and runs of other agents are skipped without a message.

**Order and date.** Oldest first. The date is read from the run directory name (`YYMMDD-HHMMSS-<hash>`, UTC), so `260701-100000-aaaaaaa` is `2026-07-01T10:00:00Z`. It is the time the run started, not when it finished, so an interrupted or resumed run shows its start time. Runs are sorted by that date, then by directory name. A run whose name has no timestamp (the old hash-only format) has no date, so it is listed first with an empty date. File modification times are not used, because they change on checkout.

**Text format.** One line per run, no header (so `wc -l` is the number of runs). Fields are separated by two spaces, and a value that is not recorded is `-`:

```
<run>  <date>  score <score>%  success <success_rate>%  agent_model <agent_model>  judge_model <judge_model>  prompt <prompt_sha256>  <completeness>
```

`score` and `success` have one decimal. `prompt` is the first 8 characters of the agent's prompt SHA-256. For the committed fixture:

```
$ kairon eval trend selftest --evals-dir internal/eval/testdata/trend
260701-100000-aaaaaaa  2026-07-01T10:00:00Z  score 80.0%  success 75.0%  agent_model agent-m  judge_model judge-m  prompt aaaaaaaa  complete
260702-100000-bbbbbbb  2026-07-02T10:00:00Z  score 100.0%  success 100.0%  agent_model agent-m  judge_model judge-m  prompt bbbbbbbb  complete
260703-100000-ccccccc  2026-07-03T10:00:00Z  score 100.0%  success 100.0%  agent_model agent-m  judge_model judge-m  prompt cccccccc  INCOMPLETE
260704-100000-ddddddd  2026-07-04T10:00:00Z  score 87.5%  success 75.0%  agent_model agent-m2  judge_model judge-m2  prompt dddddddd  complete
```

**Completeness.** The last field says whether the run covered the whole suite (the `complete` flag of [run coverage](#run-coverage-suite_sha256-complete-and-case_filter)):

| Value | Meaning |
|-------|---------|
| `complete` | The run evaluated every case of the agent. |
| `INCOMPLETE` | The run did not: it was limited to one case (`--case`, or `kairon eval <agent> <case>`) or was interrupted. Its score and success rate cover only the cases that ran, so do not compare it with a full run. The row is listed, not dropped or treated as an error. |
| `completeness-unknown` | The run was written before coverage tracking existed and does not record `complete`. |

**Success rate for older runs.** The `success` value is, in order: the `success_rate` of the summary verdict when its `attempts_total` is greater than 0; otherwise the rate recomputed from the case results in `<agent>.json` (each result's recorded `passed`), which gives runs written before `attempts_total` existed a rate (in the fixture, `260701-100000-aaaaaaa` has 3 passing results out of 4, so `75.0%`); otherwise unknown (`-` in text, `null` in JSON), for example when the agent file has no case results. Unlike the verdict, which reports `0` for zero attempts, the trend does not invent a rate for a run with no attempts.

**JSON format.** `--json` prints an indented array with one object per run, the same runs in the same order:

```json
[
  {
    "run": "260703-100000-ccccccc",
    "date": "2026-07-03T10:00:00Z",
    "score": 100,
    "success_rate": 100,
    "agent_model": "agent-m",
    "judge_model": "judge-m",
    "prompt_sha256": "cccccccc",
    "complete": false
  }
]
```

| Key | Type | Description |
|-----|------|-------------|
| `run` | string | The run directory name. |
| `date` | string | RFC 3339 UTC time from the run name; `""` when the name has no timestamp. |
| `score` | number, **percent** (0–100) | The agent's score, not rounded. |
| `success_rate` | number, **percent** (0–100), or `null` | The attempt success rate, not rounded; `null` when it cannot be determined (see above). |
| `agent_model` | string | The agent model; `""` when not recorded. |
| `judge_model` | string | The judge model; `""` when not recorded. |
| `prompt_sha256` | string | The **first 8 characters** of the prompt SHA-256, not the full hash; `""` when not recorded. |
| `complete` | boolean or `null` | `true`, `false` (the `INCOMPLETE` case), or `null` when the run does not record it (`completeness-unknown`). |

**No results.** If no run has a result for the agent, including a missing or empty `results/` directory, `eval trend` prints nothing on stdout, prints `Error: no eval results for agent "<agent>" under <evals-dir>/results` on stderr (without usage text) and exits `1` (the CLI's error handling currently prints that line twice; scripts should rely on the exit status):

```
$ kairon eval trend nope --evals-dir internal/eval/testdata/trend
Error: no eval results for agent "nope" under internal/eval/testdata/trend/results
$ echo $?
1
```

The same exit status applies to an invalid agent name and to a results directory that cannot be read. A successful listing exits `0`, including when some runs are `INCOMPLETE`.
