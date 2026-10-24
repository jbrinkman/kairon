# Design Spec: Make the watcher ignore dependency declarations inside fenced code

Closes #401

## Problem

`DependencyParser.extractIssueNumbers` (`internal/watcher/dependencies.go`) runs four regexes
(`dependsOnIssueRe`, `blockedByRe`, `dependsOnLinkRe`, `dependenciesRe`) over the raw issue body. There is no
fence tracking, so an example such as `Dependencies: #397` inside a fenced code block is scheduled as a real
dependency. The planner-side guard (`internal/planner/deps_guard.go` `checkDependencies`) and the structural guard
(`internal/guard/structural.go` `scanLines`) both treat fenced lines as examples. The watcher must agree.

## Findings from the code

- `ParseDependencies` is a one-line wrapper around `extractIssueNumbers`. It has two production callers, both inside
  `internal/watcher/dependencies.go`: `ValidateIssue` (called from `watcher.go:187`) and
  `checkCircularDependencies`. `internal/provenance/chain_test.go` also calls it (no fences; must keep passing).
  Fixing `extractIssueNumbers` therefore covers every parse path.
- `checkCircularDependencies` fetches dependency bodies with `github.GetIssueDetails` directly, not the injectable
  `dv.getIssueDetails`. It cannot be unit-tested hermetically for AC4, and a naive test would pass vacuously before the
  fix (real `gh` fails, the loop `continue`s, nothing is returned). It must be switched to `dv.getIssueDetails`.
  Production behavior is identical, because `NewDependencyValidator` sets that field to `github.GetIssueDetails`.
- Both existing guards detect fences the same way: opening line matches `^ {0,3}(` + "`{3,}|~{3,}" + `)`; a closing line
  must use the same character, be at least as long as the opener, and carry nothing but whitespace after the run; an
  unclosed fence runs to the end of the body; the delimiter lines are themselves "in fence".

## Solution approach

Add one unexported helper in `internal/watcher/dependencies.go` and call it first in `extractIssueNumbers`:

~~~go
// fenceRE matches the opening or closing line of a fenced code block.
var fenceRE = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")

// stripFencedCode returns text with every line inside a fenced code block
// (delimiters included) replaced by an empty line.
func stripFencedCode(text string) string
~~~

Behavior:

1. Split `text` on `"\n"` only (do not normalize `\r\n`, so unfenced bodies are byte-identical after the join; the
   fence regex and closing-line `strings.TrimSpace` tolerate a trailing `\r`).
2. Track the opening run exactly as `scanLines` does (same char, length >= opener, only whitespace after the run
   closes it; an unclosed fence runs to end of body).
3. Replace each in-fence line (delimiters included) with `""`. Keeping the line count and `\n` separators means the
   existing regexes behave exactly as before on the remaining text, and a match cannot span a fence because `\s+`
   only crosses blank lines, not fenced content.
4. Fast path: if no line matches `fenceRE`, return `text` unchanged. This guarantees no change in behavior for
   bodies with no fenced code (constraint) and avoids allocation on the common path.

`extractIssueNumbers` becomes `text = stripFencedCode(text)` followed by the unchanged matching logic. The set of
recognised syntaxes and result ordering are untouched.

Why not import the guard's scanner: `scanLines` and `fenceRE` are unexported in `internal/guard`, and `internal/guard`
and `internal/planner` are out of scope and must not change. The helper is about 20 lines; a comment cross-references
`internal/planner/deps_guard.go` and `internal/guard/structural.go` so the three stay aligned.

Then change `checkCircularDependencies` to call `dv.getIssueDetails(repo, dep)` instead of `github.GetIssueDetails`
so AC4 is testable with an injected lookup. No other change to that function.

## Relevant files

| File | Change |
|------|--------|
| `internal/watcher/dependencies.go` | Add `fenceRE`, `stripFencedCode`; call it in `extractIssueNumbers`; use `dv.getIssueDetails` in `checkCircularDependencies` |
| `internal/watcher/dependencies_test.go` | New tests (see below); existing tests untouched |
| `README.md` | One sentence in "Issue Dependencies": declarations inside fenced code blocks are ignored |
| `internal/planner/deps_guard.go`, `internal/guard/*`, planner-shared skill | Not modified (out of scope) |
| `.kairon/specs/` (other files) | Historical; not modified |

## Test plan (TDD, per AGENTS.md)

Write tests first, run them, and confirm they fail for the expected reason (a fenced dependency is returned or a
cycle is reported), not because of a compile error. Then implement and commit tests with implementation together.
In the commit message or PR description, name the test seen failing first.

`TestParseDependencies` cases (add to the table or a new `TestParseDependenciesFencedCode`), using `ParseDependencies`:

| Case | Input shape | Expected |
|------|-------------|----------|
| AC1 | `Dependencies: none` outside, backtick fence containing `Dependencies: #397` | `nil`/empty |
| AC2 | `Dependencies: #380, #379` outside any fence | `[380, 379]` |
| AC3a | backtick fence containing `depends on issue #397` | empty |
| AC3b | backtick fence containing `blocked by: #392` | empty |
| AC3c | same two declarations outside a fence | `[397, 392]` order as returned by current extraction (`depends on` regexes first, then `blocked by`) |
| mixed | real `Depends on Issue #5` before a fence and a fenced `Dependencies: #397` after | `[5]` |
| tilde | tilde fence containing `Dependencies: #397` | empty |
| indented | fence indented 1-3 spaces containing a dependency line | empty; 4-space indent is not a fence and is parsed |
| longer closer | opener of 4 backticks, inner 3-backtick line does not close, dependency inside | empty |
| mismatched char | backtick opener closed by tilde line does not close | dependency after it still ignored |
| unclosed | opener with no closer, dependency after it | empty |
| markdown-link form | fenced `Depends on [Issue #88]` | empty |
| CRLF | fenced block in a `\r\n` body | empty |
| no fence | every existing table case | unchanged (existing tests pass unmodified) |

Add direct unit tests for `stripFencedCode` (fast-path identity when no fence; line count preserved).

Circular-dependency test (AC4), new test using `NewDependencyValidator()` with `dv.getIssueDetails` replaced by the
existing `fakeIssueLookup` helper:

- Body `"Dependencies: none\n\n<fence>\nDependencies: #10\n<fence>"` for issue 10, with the lookup returning
  `{10: {Body: "Dependencies: #10"}}`. Assert `checkCircularDependencies("owner/repo", 10, body, map[int]bool{})`
  returns empty and the lookup was never called (no edge).
- Control: the same declaration outside a fence still produces a cycle (non-empty result), proving the test can fail.
- `ValidateIssue` with a fenced-only dependency returns `IsValid == true` and performs zero lookups.

Build the test bodies with string concatenation or `strings.Join` of lines so the fence characters appear as
ordinary Go string content.

## Team orchestration

- `fence-aware-parsing` (builder): tests-first, then `stripFencedCode` and the `extractIssueNumbers` call. Includes
  the AC1-3 and constraint tests. Single commit with tests and implementation.
- `circular-detection-fence-aware` (builder, after `fence-aware-parsing`): swap to `dv.getIssueDetails` and add the
  AC4 tests. Same file as task 1, so it is sequenced rather than parallel. See the TDD note below.
- `update-readme` (builder, after `fence-aware-parsing`, parallel with the circular task): one-sentence doc update.
  Docs are exempt from test-first.
- `validate-all` (validator): runs the full verification, checks scope (no edits to out-of-scope paths).

Note on task 2 TDD: because `extractIssueNumbers` is already fixed by task 1, the AC4 fenced test would pass once
the lookup is injectable. To show the test can fail, the builder verifies the control case (unfenced self-reference
yields a cycle) and, as a one-off local check, temporarily bypasses `stripFencedCode` to see the fenced test fail,
then restores it. Do not commit the bypass.

## Validation commands

~~~bash
go build ./...
go vet ./internal/watcher/
go test ./internal/watcher/
go test -race ./internal/watcher/
go test ./internal/provenance/ ./internal/planner/ ./internal/guard/
task test
task lint
git diff --name-only origin/main...HEAD   # only dependencies.go, dependencies_test.go, README.md, this spec
~~~

If the repo's CI gate also runs `task sync:check`, it is unaffected: no template-synchronized file is edited.

## Risks

- A fence-like line inside a non-code context (for example a line starting with three backticks mid-paragraph) is
  treated as a fence, identical to the guards. Accepted; matches the authoritative reading rule.
- An issue whose real dependency was written inside a fence will now be scheduled immediately. This is the intended
  behavior and matches what the planner guard already enforces.

## Machine-readable execution plan

```kiro-plan
version: "1.0"
tasks:
  - id: "fence-aware-parsing"
    agent: "builder"
    description: "Test-first: add failing watcher tests for fenced dependency declarations (AC1-3 plus tilde, indented, longer-closer, mismatched-char, unclosed, CRLF, 4-space-indent, mixed cases and stripFencedCode unit tests), confirm they fail because fenced dependencies are returned, then add fenceRE and stripFencedCode to internal/watcher/dependencies.go and call it at the top of extractIssueNumbers. Commit tests and implementation together, naming the first-failing test in the commit message."
    dependencies: []
    acceptance_criteria:
      - "Body with Dependencies: none outside a fence and Dependencies: #397 inside a backtick fence parses to zero dependencies via DependencyParser.ParseDependencies"
      - "Body with Dependencies: #380, #379 outside any fence parses to [380, 379]"
      - "Fenced 'depends on issue #397' and fenced 'blocked by: #392' are ignored, and the same declarations outside a fence are still parsed (four cases covered)"
      - "A tilde fence containing Dependencies: #397 parses to zero dependencies; fences indented 1-3 spaces are honoured and 4 spaces is not a fence"
      - "Fence closing rules match internal/planner/deps_guard.go: same character, run length >= opener, only whitespace after the run; unclosed fence runs to end of body"
      - "Bodies with no fence line are returned unchanged by stripFencedCode (fast path) and all pre-existing tests in internal/watcher pass without modification"
      - "No edits outside internal/watcher/dependencies.go and internal/watcher/dependencies_test.go"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/watcher/"
      - "go test ./internal/watcher/"
      - "go test -race ./internal/watcher/"
      - "go test ./internal/provenance/"

  - id: "circular-detection-fence-aware"
    agent: "builder"
    description: "Change checkCircularDependencies in internal/watcher/dependencies.go to fetch dependency bodies through dv.getIssueDetails instead of github.GetIssueDetails so it is hermetically testable, then add tests asserting a fenced dependency line yields no edge (no lookup, no cycle) while an unfenced self-reference still reports a cycle, and that ValidateIssue with a fenced-only dependency is valid with zero lookups."
    dependencies: ["fence-aware-parsing"]
    acceptance_criteria:
      - "checkCircularDependencies uses dv.getIssueDetails; NewDependencyValidator still defaults it to github.GetIssueDetails so production behavior is unchanged"
      - "Test: a body whose only self-reference is inside a fence makes checkCircularDependencies return empty and performs zero lookups"
      - "Control test: the same self-reference outside a fence makes checkCircularDependencies return a non-empty cycle"
      - "Test: ValidateIssue with a fenced-only dependency returns IsValid true and performs zero lookups"
      - "Existing TestValidateIssue and TestValidateIssueCyclicDependencyDoesNotFetchChain pass unchanged"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/watcher/"
      - "go test -race ./internal/watcher/"

  - id: "update-readme"
    agent: "builder"
    description: "In README.md under 'Issue Dependencies', add one sentence stating that dependency declarations inside fenced code blocks (backtick or tilde, including indented up to three spaces) are ignored, matching the planner dependencies guard. Do not change the recognised syntaxes table."
    dependencies: ["fence-aware-parsing"]
    acceptance_criteria:
      - "README.md Issue Dependencies section states that declarations inside fenced code blocks are ignored"
      - "The table of recognised formats and the single-line Dependencies: rule are unchanged"
      - "No other files are modified"
    validation_commands:
      - "grep -n -i 'fenced' README.md"
      - "git diff --stat -- README.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion and constraint of issue #401: fence-aware parsing for ParseDependencies and the circular-dependency path, tilde and indented fences, no behavior change for unfenced bodies, race-clean tests, and that out-of-scope paths (internal/planner, internal/guard, planner-shared skill, historical specs) were not modified."
    dependencies: ["circular-detection-fence-aware", "update-readme"]
    acceptance_criteria:
      - "go test ./internal/watcher/ and go test -race ./internal/watcher/ exit 0"
      - "Tests exist for all four AC3 cases, the tilde fence case, and the fenced circular-dependency case"
      - "git diff against the base branch touches only internal/watcher/dependencies.go, internal/watcher/dependencies_test.go, README.md and the issue-401 spec"
      - "task test and task lint pass"
    validation_commands:
      - "go build ./..."
      - "go test ./internal/watcher/"
      - "go test -race ./internal/watcher/"
      - "go test ./internal/provenance/ ./internal/planner/ ./internal/guard/"
      - "task test"
      - "task lint"
```
