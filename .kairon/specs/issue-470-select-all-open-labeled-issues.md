# Design Spec: Select every open labeled issue, oldest first, without --search

Closes #470

## Scope Coverage

| # | Item (quoted from the issue) | Delivered by |
|---|------------------------------|--------------|
| S1 | In Scope: "`internal/github`: fetch every open labeled issue (paginate, or list numbers and labels first and fetch comments only for the candidates), in oldest-first order (ascending issue number)." | `fetch-open-issues`, `wire-select-issues` |
| S2 | In Scope: "`internal/github`: no `gh` call made by `selectIssues` uses `--search`. Oldest-first order comes from the API's own ordering (GraphQL `orderBy: {field: CREATED_AT, direction: ASC}` or REST `sort=created&direction=asc`) or from sorting by issue number in Go after all pages are fetched." | `fetch-open-issues`, `wire-select-issues` |
| S3 | In Scope: "`internal/github`: if fetching any page fails, `selectIssues` returns an error and no `Selection`. It never returns the issues from the pages that succeeded. The watcher's existing error path (`checkIssues` leaves `tracked` untouched on error) then keeps every tracked issue. `untrackAbsent` itself is unchanged: an issue absent from a complete result (closed, or labeled done or failed) is still untracked." | `fetch-open-issues`, `wire-select-issues` |
| AC1 | "With more open labeled issues than one page holds, a single `selectIssues` call selects every eligible issue, oldest first." | `wire-select-issues` (`TestSelectIssues_MultiPageSelectsAllOldestFirst`) |
| AC2 | "A failed page never yields a partial selection." | `fetch-open-issues` (`TestFetchOpenIssues_LaterPageErrorReturnsNothing`), `wire-select-issues` (`TestSelectIssues_SecondPageFailureReturnsErrorAndEmptySelection`), `validate-all` |
| AC3 | "Selection does not use the search index, so a just-labeled issue is seen on the next poll." | `wire-select-issues` (`TestSelectIssues_NeverUsesSearch`) |
| AC4 | "Order is oldest first, whatever order the API returns issues in." | `wire-select-issues` (`TestSelectIssues_OutOfOrderPagesComeBackAscending`) |
| AC5 | "The gate passes." | `validate-all` |

## Deferred work

none

## Solution Approach

### What is wrong today

`selectIssues` (`internal/github/adapter.go`) makes one call:

```
gh issue list --repo R --label L --state open --json number,title,body,labels,comments --limit 50 --search "sort:created-asc" [--assignee A]
```

- `--limit 50` (`listIssueLimit`) hides every issue past the 50th.
- `--search` reads GitHub's search index. It lags label changes and has its own rate limit.
- `gh issue list` takes no page or cursor argument, so Kairon cannot page through it, and a `--limit` large enough to cover "everything" is still a cap.

### Chosen design: cursor-paginated GraphQL, one `gh api graphql` call per page

Kairon calls `gh api graphql` once per page and follows `pageInfo.endCursor` until `hasNextPage` is false. Paging in Kairon code (not inside `gh --paginate`) is required so a test with a fake `gh` can make page 2 fail (AC2) and so no output of a half-finished fetch can leak out.

Why GraphQL and not the alternatives:

| Option | Verdict |
|--------|---------|
| `gh issue list --limit <huge>` | Rejected. It is still a cap, `gh` pages internally so the page-2-fails case cannot be tested, and ordering needs `--search`. |
| REST `GET /repos/{r}/issues?labels=L&state=open&sort=created&direction=asc&per_page=100&page=N` | Rejected. It returns pull requests too (need filtering), returns no comments (one extra call per issue every poll), and page-number paging can skip an issue when another issue closes between page fetches. |
| List numbers and labels first, then fetch comments for candidates only | Rejected. It costs one extra call per candidate issue, per poll. GraphQL returns labels and comments in the same page, so the extra calls buy nothing. |
| GraphQL `issues` connection, cursor paging, `orderBy CREATED_AT ASC` | Chosen. One call per page, labels and comments in the same response, cursor positions stay valid when issues close or open mid-fetch (new issues append at the end of an ascending order), and the connection reads the database, not the search index. |

The query shape was run against the live API for `jbrinkman/kairon` (read-only) and works, including a null `$assignee` and the failure mode for a missing repository (exit 1, `errors` in the body, `repository: null`).

```graphql
query($owner: String!, $name: String!, $label: String!, $assignee: String, $after: String) {
  repository(owner: $owner, name: $name) {
    issues(first: <listPageSize>, after: $after, states: [OPEN], labels: [$label],
           filterBy: {assignee: $assignee},
           orderBy: {field: CREATED_AT, direction: ASC}) {
      pageInfo { hasNextPage endCursor }
      nodes {
        number title body
        labels(first: 100) { nodes { name } }
        comments(first: <listCommentCap>) { nodes { author { login } body createdAt } }
      }
    }
  }
}
```

Call shape (an argument list, never a shell):

```
gh api graphql -f query=<query> -f owner=<owner> -f name=<name> -f label=<label> [-f assignee=<a>] [-f after=<endCursor>]
```

- `<listPageSize>` and `<listCommentCap>` are integer constants formatted into the query text. The repo, label, assignee and cursor are untrusted or opaque values and travel only as `-f` variables, never inside the query text.
- Use `-f` (string), not `-F`, so a value starting with `@` is not read as a file.
- `comments(first: listCommentCap)` keeps today's behavior: an issue whose comments reach `listCommentCap` is re-read in full through `CLI.State`. That branch in `selectIssues` is unchanged.
- `labels(first: 100)` matches the cap `gh issue list --json labels` applies today.
- The query never uses the GraphQL `search(...)` field, and no argument is `--search`.

### New code: `fetchOpenIssues`

New file `internal/github/issuelist.go`:

```go
// fetchOpenIssues returns every open issue labeled label (and assigned to
// assignee when non-empty), ascending by issue number. Any failure returns
// (nil, err): the caller never sees the pages that did succeed.
func fetchOpenIssues(run ghFunc, repo, label, assignee string) ([]rawIssue, error)
```

Behavior:

1. Split `repo` on the first `/`. If owner or name is empty, return an error before any `gh` call.
2. Loop: build args, call `run`, decode the page, append its nodes to a local slice, stop when `hasNextPage` is false.
3. Error on any of: `run` returns an error (map `isRateLimited(err)` to `ErrRateLimited`, otherwise wrap with the page number); invalid JSON; a non-empty `errors` array; `data.repository` is null; `hasNextPage` true with an empty `endCursor`; an `endCursor` already seen in this fetch (a cursor that does not advance would loop forever). Every one of these returns `(nil, err)`.
4. After the last page, `sort.SliceStable` by `Number` ascending, then return. The API order (`CREATED_AT ASC`) keeps cursor paging stable. The Go sort guarantees the ascending-number contract whatever order the API or a fake returns.
5. Convert each GraphQL node (`labels.nodes`, `comments.nodes`) into the existing `rawIssue` / `Label` / `rawComment` values so the rest of `selectIssues` keeps working on the types it uses now. A null comment `author` decodes to an empty login.

### Change to `selectIssues`

In `internal/github/adapter.go`:

- Replace the `run("", "issue", "list", ... "--search" ...)` call, the `json.Unmarshal` into `[]rawIssue` and the error mapping with `issues, err := fetchOpenIssues(run, repo, label, assignee)`; on error return `Selection{}, err` (rate limit already mapped to `ErrRateLimited`).
- Delete `listIssueLimit`. Keep `listCommentCap`. Add `listPageSize = 50` (documented: page size, not a cap on the total).
- Leave the classification loop, the viewer lookup, the events lookup and the `CLI.State` re-read exactly as they are. Their per-issue fail-closed `continue` behavior is unchanged and stays pinned by `TestSelectIssues_EventsLookupErrorFailsClosed`, `TestSelectIssues_ViewerLookupErrorFailsClosed` and `TestSelectIssues_CommentCapRereadFailsClosed`.
- `Selection` returned on any error is the zero value (`Selection{}`).

Because `fetchOpenIssues` sorts, `Eligible` and `TerminalPending` are each in ascending issue number.

### Not touched

`internal/watcher` (including `untrackAbsent`, `checkIssues` and their tests), `internal/github/githubtest/fake.go`, `ListIssues`/`listIssues` signatures and `SelectIssues`' signature. `internal/github/client.go:86` (`gh pr list --search head:spec/issue-N-`) belongs to a different function that does not run during `selectIssues`, and stays as is.

## Relevant Files

| File | Change |
|------|--------|
| `internal/github/issuelist.go` | New. `fetchOpenIssues`, the query constant, the page decode types. |
| `internal/github/issuelist_test.go` | New. Tests for `fetchOpenIssues` (paging, cursor passing, failure, guards, args). |
| `internal/github/adapter.go` | `selectIssues` calls `fetchOpenIssues`; remove `listIssueLimit`; add `listPageSize`. |
| `internal/github/adapter_test.go` | `fakeGH` serves `api graphql` pages; `issueJSON` and `listOf` emit GraphQL node and page shape; rewrite `TestSelectIssues_ListArgs`; add the four `selectIssues` tests named in the coverage table. |
| `README.md` | "GitHub Integration" list: replace the `gh issue list` bullet and describe the paginated poll. |
| `internal/watcher/*`, `internal/github/githubtest/fake.go` | Read only. Must have no diff. |

`internal/github/adapter_test.go` references to `fakeGH.list`/`listErr`/`issueJSON`/`listOf` are used by about twenty existing tests. Keep those field and helper names so that those tests need no edits beyond what the new shape forces.

## Test design (TDD, per AGENTS.md)

Write the tests first, run them, and confirm each fails for the expected reason (for the new tests: the current code issues `issue list ... --search`, so the fake sees no `api graphql` call, or `selectIssues` returns only the first 50 issues). A compile error is not an acceptable first failure: stub `fetchOpenIssues` with a signature-only version returning `errors.New("not implemented")` first so the failures are behavioral. Commit tests and implementation together. The commit message names the test observed failing.

### `fakeGH` changes (`adapter_test.go`)

- `list string` stays and now holds the output of the single `api graphql` call. Add `pages []fakePage` (`out string`, `err error`) served in call order when non-empty; otherwise the single `list`/`listErr` pair answers every `api graphql` call. Dispatch on `strings.HasPrefix(j, "api graphql")`, replacing the `issue list` case.
- `issueJSON(n, labels, comments...)` emits `{"number":n,"title":..,"body":..,"labels":{"nodes":[{"name":..}]},"comments":{"nodes":[..]}}`.
- `listOf(issues...)` returns a complete single page: `{"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[..]}}}}`.
- Add `pageOf(endCursor string, issues ...string)` (`hasNextPage` true when `endCursor` is non-empty).
- Add a helper that returns the `after=` value of a recorded call.

### Tests

| Test | File | What it pins |
|------|------|--------------|
| `TestSelectIssues_MultiPageSelectsAllOldestFirst` | `adapter_test.go` | AC1. Three pages (for example 50, 50, 7 issues, generated from `listPageSize`), one issue with `kairon-done`, one with a trusted marker (terminal pending). `Eligible` holds every other issue, ascending; the count exceeds `listPageSize`; exactly three `api graphql` calls. |
| `TestSelectIssues_SecondPageFailureReturnsErrorAndEmptySelection` | `adapter_test.go` | AC2. Page 1 succeeds with eligible issues, page 2 returns an error. `err != nil`, and `sel.Eligible` and `sel.TerminalPending` are both empty (`reflect.DeepEqual(sel, Selection{})`). Also a variant where page 2 is invalid JSON. |
| `TestFetchOpenIssues_LaterPageErrorReturnsNothing` | `issuelist_test.go` | AC2 at the fetch layer. Returned slice is nil on a page-2 error, and `errors.Is(err, ErrRateLimited)` holds when the page-2 error is a rate-limit `*exec.ExitError`. |
| `TestSelectIssues_NeverUsesSearch` | `adapter_test.go` | AC3. Run a scenario that triggers every sub-call (two pages, a marker issue with viewer and events lookups, a 100-comment issue that triggers the `issue view` re-read). For every recorded call, no argument contains `--search`, and the query argument does not contain `search(`. |
| `TestSelectIssues_OutOfOrderPagesComeBackAscending` | `adapter_test.go` | AC4. Page 1 holds `9, 3, 7`, page 2 holds `5, 1`, and one terminal-pending issue sits among them. `Eligible` and `TerminalPending` are each ascending. |
| `TestFetchOpenIssues_PassesCursorAndVariables` | `issuelist_test.go` | Page 1 call has no `after=`; page 2 call has `after=` equal to page 1's `endCursor`. `owner`, `name`, `label` are separate `-f` args; `assignee` appears only when non-empty; the label and repo values never appear inside the query argument. |
| `TestFetchOpenIssues_RejectsBadPageState` | `issuelist_test.go` | Table: `hasNextPage` true with empty cursor; repeated cursor; non-empty `errors` array; `repository: null`; repo without `/`. Each returns `(nil, err)`; the repo case makes no `gh` call. |
| `TestSelectIssues_PageArgs` (replaces `TestSelectIssues_ListArgs`) | `adapter_test.go` | Exact args of the first call for label `my-label`, assignee `alice`: `api graphql`, the query, owner, name, label, assignee, and no `--search`, `--limit` or `issue list`. |
| existing `TestSelectIssues_*`, `TestListIssues_ReturnsEligibleOnly` | `adapter_test.go` | Unchanged assertions. `TestSelectIssues_ListErrors` and `TestSelectIssues_RateLimitIsMapped` keep working through the new fake dispatch. |
| `TestCheckIssues_PollErrorLeavesTrackedUnchanged`, `TestUntrackAbsent_DropsExcludedKeepsEligible` | `internal/watcher/watcher_reconcile_test.go` | Unchanged, run as part of `go test ./...`. |

## Team Orchestration

- `fetch-open-issues` (builder) runs first. It owns `issuelist.go` and `issuelist_test.go`, which no other task touches.
- `wire-select-issues` (builder) depends on it. It owns `adapter.go` and `adapter_test.go`.
- `update-readme` (documenter) touches only `README.md` and can run in parallel with `wire-select-issues` once `fetch-open-issues` fixes the call shape.
- `validate-all` (validator, read-only) runs last.

Builders must stage only their own files. The working tree already has uncommitted changes in `.kiro/agents/*.json` that are not part of this issue; do not add them to a commit. Do not push.

## Step-by-Step Task Breakdown

### Task 1: `fetch-open-issues`
Create `fetchOpenIssues` and its tests in `issuelist.go` / `issuelist_test.go` following the sections above. Start with a signature-only stub so the tests fail on behavior, show the failures, then implement. The tests use their own small fake `ghFunc`; they do not depend on `fakeGH` changes.
Dependencies: none.

### Task 2: `wire-select-issues`
Update `fakeGH` and the helpers, write the four AC tests and `TestSelectIssues_PageArgs`, confirm they fail against the current `selectIssues` (it still calls `issue list --search`), then switch `selectIssues` to `fetchOpenIssues`, remove `listIssueLimit`, add `listPageSize`. Run the whole `internal/github` package and `./internal/watcher`.
Dependencies: Task 1.

### Task 3: `update-readme`
In `README.md` "GitHub Integration", replace "`gh issue list` — poll for labeled issues" with a bullet for `gh api graphql` that says the watcher pages through all open labeled issues (50 per page, oldest first) without `--search`, and that a failed page fails the whole poll so the watcher keeps its tracked issues. Check `README.md` and `docs/` for any other statement about the 50-issue limit or `gh issue list` polling (a search for `issue list`, `--limit`, `oldest`, `50 issues` found only the README line when this spec was written).
Dependencies: Task 1.

### Task 4: `validate-all`
Read-only verification of every acceptance criterion, the scope table, and that `internal/watcher` is untouched.
Dependencies: Tasks 2 and 3.

## Validation Commands

```bash
go build ./...
go vet ./internal/github/...
test -z "$(gofmt -l internal/github)"
go test ./internal/github/... -run 'FetchOpenIssues|SelectIssues|ListIssues' -count=1
go test ./internal/watcher/... -run 'TestCheckIssues_PollErrorLeavesTrackedUnchanged|TestUntrackAbsent_DropsExcludedKeepsEligible' -count=1
! grep -n '"--search"' internal/github/adapter.go internal/github/issuelist.go
git diff --exit-code 4d213fd -- internal/watcher internal/github/githubtest
task test
task lint
task fmt:check
```

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "fetch-open-issues"
    agent: "builder"
    description: "Add internal/github/issuelist.go with fetchOpenIssues(run, repo, label, assignee) ([]rawIssue, error): one `gh api graphql` call per page using cursor pagination (issues connection, states OPEN, labels filter, optional assignee filter, orderBy CREATED_AT ASC, labels and comments in the page), all untrusted values passed as -f variables, errors on any failed page, invalid JSON, GraphQL errors, null repository, missing or repeated cursor, and a repo without owner/name. Return (nil, err) on any failure, sort by issue number ascending after the last page, convert nodes to rawIssue. Write issuelist_test.go first (signature-only stub, see the tests fail on behavior), then implement. Commit tests and implementation together."
    dependencies: []
    acceptance_criteria:
      - "fetchOpenIssues exists in internal/github/issuelist.go with the documented signature and no gh call uses the string \"--search\" or the GraphQL search( field"
      - "TestFetchOpenIssues_LaterPageErrorReturnsNothing passes: a page-2 error returns a nil slice and an error, and a rate-limit page error satisfies errors.Is(err, ErrRateLimited)"
      - "TestFetchOpenIssues_PassesCursorAndVariables passes: page 2 call carries after=<page 1 endCursor>, repo/label/assignee are separate -f args and never inside the query text"
      - "TestFetchOpenIssues_RejectsBadPageState passes for: hasNextPage with empty cursor, repeated cursor, non-empty errors array, null repository, repo without a slash (no gh call for the last)"
      - "The returned issues are ascending by number even when the fake returns pages out of order"
      - "The tests were seen failing for a behavioral reason before the implementation, and that test name is in the commit message"
    validation_commands:
      - "go build ./internal/github/..."
      - "go vet ./internal/github/..."
      - "test -z \"$(gofmt -l internal/github)\""
      - "go test ./internal/github/... -run 'FetchOpenIssues' -count=1"
      - "! grep -n '\"--search\"' internal/github/issuelist.go"

  - id: "wire-select-issues"
    agent: "builder"
    description: "Switch selectIssues in internal/github/adapter.go to fetchOpenIssues, return Selection{} and the error on any fetch error (rate limit stays ErrRateLimited), delete listIssueLimit, add listPageSize = 50, leave the classification loop and per-issue fail-closed lookups unchanged. In adapter_test.go update fakeGH to serve `api graphql` pages (list/listErr kept, new pages field), make issueJSON and listOf emit GraphQL node and page shape, add pageOf, replace TestSelectIssues_ListArgs with TestSelectIssues_PageArgs, and add TestSelectIssues_MultiPageSelectsAllOldestFirst, TestSelectIssues_SecondPageFailureReturnsErrorAndEmptySelection, TestSelectIssues_NeverUsesSearch and TestSelectIssues_OutOfOrderPagesComeBackAscending. Write the new tests first and see them fail against the current issue-list code. Do not edit internal/watcher. Commit tests and implementation together."
    dependencies: ["fetch-open-issues"]
    acceptance_criteria:
      - "selectIssues makes no `issue list` call and no argument of any gh call it makes contains --search (asserted over every recorded call in a scenario that triggers viewer, events and issue-view sub-calls)"
      - "TestSelectIssues_MultiPageSelectsAllOldestFirst passes: more issues than listPageSize across three pages, all eligible ones selected in ascending number order, done and terminal-pending issues excluded from Eligible"
      - "TestSelectIssues_SecondPageFailureReturnsErrorAndEmptySelection passes: page 1 ok, page 2 fails, err is non-nil and the Selection equals Selection{}"
      - "TestSelectIssues_OutOfOrderPagesComeBackAscending passes: Eligible and TerminalPending are each ascending although pages and in-page order are shuffled"
      - "All pre-existing tests in internal/github pass with no change to their assertions, including the fail-closed tests and TestSelectIssues_RateLimitIsMapped"
      - "listIssueLimit is gone and listPageSize is documented as a page size, not a total cap"
      - "No file under internal/watcher or internal/github/githubtest has a diff"
      - "The new tests were seen failing for a behavioral reason before the change, and that test name is in the commit message"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/github/..."
      - "test -z \"$(gofmt -l internal/github)\""
      - "go test ./internal/github/... -count=1"
      - "go test ./internal/watcher/... -run 'TestCheckIssues_PollErrorLeavesTrackedUnchanged|TestUntrackAbsent_DropsExcludedKeepsEligible' -count=1"
      - "! grep -n '\"--search\"' internal/github/adapter.go internal/github/issuelist.go"
      - "git diff --exit-code 4d213fd -- internal/watcher internal/github/githubtest"

  - id: "update-readme"
    agent: "documenter"
    description: "Update README.md 'GitHub Integration' so it no longer says the watcher polls with `gh issue list`: describe the `gh api graphql` paginated poll (50 issues per page, all open labeled issues, oldest first, no --search, a failed page fails the whole poll and the watcher keeps its tracked issues). Check README.md and docs/ for any other statement of a 50-issue limit or `gh issue list` polling and correct it. Do not edit .kairon/specs/, CHANGELOG.md or internal/watcher."
    dependencies: ["fetch-open-issues"]
    acceptance_criteria:
      - "README.md no longer lists `gh issue list` as the poll call and describes the paginated GraphQL poll accurately"
      - "No other live doc under README.md or docs/ claims a 50-issue cap or search-based ordering for the poll"
      - "No file under .kairon/specs/ or CHANGELOG.md is modified"
    validation_commands:
      - "! grep -n 'gh issue list. — poll' README.md"
      - "grep -n 'graphql' README.md"
      - "git diff --exit-code 4d213fd -- .kairon/specs CHANGELOG.md"

  - id: "validate-all"
    agent: "validator"
    description: "Verify every acceptance criterion of issue #470 and the Scope Coverage table: run the full gate, confirm the four AC tests exist and assert what the spec says, confirm no gh call made by selectIssues uses --search, confirm the second-page-failure test checks an empty Selection and an error, confirm the two named watcher tests pass unchanged, and confirm internal/watcher and internal/github/githubtest have no diff."
    dependencies: ["wire-select-issues", "update-readme"]
    acceptance_criteria:
      - "task test exits 0"
      - "task lint and task fmt:check exit 0"
      - "TestSelectIssues_MultiPageSelectsAllOldestFirst, TestSelectIssues_SecondPageFailureReturnsErrorAndEmptySelection, TestSelectIssues_NeverUsesSearch and TestSelectIssues_OutOfOrderPagesComeBackAscending exist and pass"
      - "TestCheckIssues_PollErrorLeavesTrackedUnchanged and TestUntrackAbsent_DropsExcludedKeepsEligible pass and their files are unmodified"
      - "git diff against 4d213fd shows no change under internal/watcher or internal/github/githubtest"
      - "The PR description will state 'Deferred work: none' and a 'Spec deviations' section (listing each deviation, or 'none')"
    validation_commands:
      - "task test"
      - "task lint"
      - "task fmt:check"
      - "go test ./internal/github/... ./internal/watcher/... -count=1"
      - "git diff --exit-code 4d213fd -- internal/watcher internal/github/githubtest"
      - "! grep -rn '\"--search\"' internal/github/adapter.go internal/github/issuelist.go"
```
