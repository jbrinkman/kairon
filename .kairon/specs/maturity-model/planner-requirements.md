# Planner Requirements — Stage Output Contract and Intake Design

**Drafted:** 2026-10-18
**Rewritten:** 2026-10-20 (full replacement; see §1)
**Status:** Requirements for the planner rebuild (Stage 3), and the stage-output contract the whole pipeline is
moving toward (Stages 4 and 5). Supersedes both the Step 1 approach in `gap-analysis.md` §S3.6 (which iterated on
the existing planner prompt) and the 2026-10-18 draft of this document (which kept the planner writing to GitHub).
**Audience:** the Eval-Driven Development (EDD) rebuild of the planner, and anyone designing the stage boundaries
(guards) that connect pipeline steps. It defines *what each stage must produce and guarantee* and *who commits side
effects*; it does not prescribe prompt text, rubric wording, or eval-harness mechanics.

> This is a design record. Later code may legitimately diverge; this document captures the decisions reached about
> the stage-output contract and the planner's role on the dates above.

---

## 1. Why this is a full rewrite, not an edit

The 2026-10-18 draft rebuilt the issue schema from first principles (that work survives, in §5–§8 below). But it
left one assumption from the original design in place: **the planner writes its output directly into the live GitHub
issue**, embedding a content hash as it goes, and the pipeline then re-reads and re-classifies the issue to decide
whether planning was done.

Building on that assumption produced a class of defects that are symptoms, not root causes:

- The planner rewrote the issue body mid-run, so a valid hash could go live *before* the planner finished. A later
  poll saw a "valid" body while the run was still in flight and spawned the implementation agent on a half-planned
  issue. (PRs #372, #373, #374 were the hash scheme, the eval spec, and watcher race-guards for exactly this
  window. All three are reverted/closed; the guards defend a design we are removing.)
- Provenance had to be *tamper-evident* because the agent wrote to a public, editable surface and the pipeline could
  not trust its own output. The hash existed to detect that the agent's own write had been corrupted.
- The planner's eval had to assert against a side effect on GitHub ("did the agent eventually write the right thing
  to a live issue"), which is not deterministically testable and forces a mock-heavy, flaky suite.

The fix is architectural, and it is the direction the maturity model already points (Stage 4 §2: *guardrails sit
**between** steps, not inside them*). **A stage produces a typed output document and nothing else. A deterministic
guard validates that document at the boundary and is the only thing allowed to commit a side effect** (write the
GitHub issue, compute and attach the hash, spawn the next stage). An agent that cannot write to GitHub cannot
produce a half-written issue, cannot corrupt its own provenance, and is evaluated purely on the document it returns.

This rewrite therefore changes the spine of the document from "what the planner writes to GitHub" to "what document
the planner returns and how the guard validates and commits it," and generalizes that contract to every stage.

---

## 2. Core principles

Principles 1–8 govern **issue content** (unchanged in spirit from the 2026-10-18 draft); principles 9–13 govern the
**stage-output contract** (new, and the reason for this rewrite). First, §2.1 defines the execution model those
contracts live in (workflows, nodes, and the two transport regimes), because it determines *which* transport a given
boundary may use.

### 2.1 Workflows, nodes, and shared state (the execution model)

A precise vocabulary, because the whole boundary model depends on it:

- An **agent** is a defined role (planner, architect, krew-lead, builder, validator, documenter). It is a definition,
  not a running thing.
- A **node** is a running *instance* of an agent: one invocation doing one unit of work.
- A **workflow** is a graph of one or more nodes that together accomplish one phase of the pipeline. A workflow runs
  **entirely on a single machine**: all of its nodes execute in one place.

Today there are **two workflows**, and a third is planned:

1. **Planning workflow:** single node, single agent (the planner).
2. **Implementation workflow:** multi-node, an orchestrator (krew-lead) plus the agents it drives (builder,
   validator, documenter, and the deterministic gates between them).
3. **PR-review-and-correction workflow** (future): reviews an implementation's PR and drives corrections.

From this follow two distinct transport regimes, and **the regime a boundary spans decides what it may assume**:

- **Intra-workflow transport (between nodes of the same workflow).** Because a workflow runs on one machine, nodes
  share local resources: the filesystem, a working directory, in-process values, the git worktree. A node may hand
  the next node an in-memory object or a local file directly. Some of these artifacts may additionally be
  **committed to the repository for historical audit** (e.g. the architect's spec file), but that is for the record,
  not because the next node could not otherwise reach them. **This is the one case where "hand the next stage the
  artifact the previous stage produced" is literally true.**
- **Inter-workflow transport (between different workflows).** Two workflows are **not guaranteed to run on the same
  machine**: planning, implementation, and review may each run on a different host, owned by a different person.
  They share no filesystem, no process, no worktree. They can communicate only through a **shared state mechanism**
  that is durable and reachable from any machine. **Today that shared state is GitHub:**
  - **GitHub Issues** are the channel between the **planning** and **implementation** workflows. The planning
    workflow commits the canonical issue; the implementation workflow reads it on a possibly-different machine.
  - **Pull requests** are the channel between the **implementation** and **review** workflows. The implementation
    workflow opens the PR; the review workflow reads it, again possibly elsewhere.

So the stage-output contract (§3) is realized by **two different mechanisms depending on the boundary**: inside a
workflow, the output document can travel as a local object/file (and optionally be committed for audit); across
workflows, the document must be **serialized into the shared state (the GitHub issue or PR) and verified back out of
it** (§8, §11.1). The design error the 2026-10-18 model made was treating every boundary as if it were intra-workflow,
assuming the consuming node could be handed the producing node's artifact, when the planning→implementation and
implementation→review boundaries are inter-workflow and can only pass through GitHub.

Two consequences this document carries forward:

- The provenance hash (§8) exists **because of inter-workflow transport**: when the only thing crossing is the
  serialized GitHub body, the consuming workflow needs a deterministic way to confirm the body it reads is exactly
  what the producing workflow validated, and was not edited in between. For an intra-workflow handoff the hash is
  optional belt-and-suspenders; for an inter-workflow handoff it is load-bearing.
- A single shared-state record carries only what is serialized into it. The GitHub issue body carries the body and
  the hash, not the full metadata envelope (§3.3); preserving envelope metadata across an inter-workflow boundary is
  a deliberate design choice (§11.1), not something the transport gives for free.

### Issue-content principles

1. **An issue is a requirements document, not a design or an implementation.** It states what is needed, why, the
   boundaries of the change, and how we will know it is done. The architect's spec is the design and task list.
2. **"Done" must be mechanically verifiable.** Every acceptance criterion maps to an observable outcome — a command
   exit code, a file, an API response — that the validation step can check without judgment.
3. **Front-load every decision; the agent cannot ask.** Any open choice becomes an unaccountable guess. If the agent
   is meant to decide, the issue says so and gives the decision criteria.
4. **Separate must from nice.** Binding decisions go in Constraints (with a reason and a verification). Non-binding
   background goes in Context.
5. **References must be resolvable.** Exact repo-relative paths, symbol names, issue/PR numbers, commit SHAs, and
   fetchable links.
6. **Scope boundaries are defensive.** An explicit Out of Scope / do-not-touch list is the main defense against a
   diff that balloons past what is reviewable.
7. **Right-size it.** Every sentence must change what the agent builds or how it is verified.
8. **The issue body is a specification, not a prompt.** The pipeline treats issue content as untrusted data.

### Stage-output-contract principles

9. **A stage output is a value, not a side effect.** Each stage returns a structured document (§3). It does not
   mutate shared mutable state (the GitHub issue, the repo, the labels). The thing a guard validates is the returned
   document, not a re-read of whatever the agent did to the world.
10. **The guard owns the commit, not the stage.** Deterministic code at the stage boundary validates the output
    document and is the only component that performs the side effect: writing the GitHub issue, computing and
    attaching provenance, creating a branch, spawning the next stage. "Provenance valid" stops meaning "an agent
    claimed it" and starts meaning "the guard validated this output and stamped it."
11. **Guards sit between steps, not inside them** (Stage 4 §2). A check an agent performs on itself mid-run is not a
    guard. The guard is separate, deterministic, and positioned at the boundary where one stage's output becomes the
    next stage's input.
12. **Stage outputs are structured, extensible, and encoded as YAML.** The output document is **YAML** (the single
    required format for every stage) carrying the content body plus a metadata envelope, parsed under a strict
    schema (unknown keys rejected), so metadata can be added over time without breaking existing stages (§3). The
    way strict parsing and additive metadata coexist (version-scoped strictness, not relaxed parsing) is defined in
    §3.4. YAML
    is required (not "YAML or JSON") so stages never diverge on format: it is the encoding the architect's existing
    `kiro-plan` contract already uses (`internal/plan/parser.go`, `gopkg.in/yaml.v3` with `KnownFields(true)`), it is
    already a `go.mod` dependency with a working strict-parse pattern to reuse, and it embeds multi-line body prose
    in a fenced block far more readably than JSON's escaped strings. (YAML is a superset of JSON, so a JSON-shaped
    payload remains valid YAML if a stage ever needs it; the required, canonical format is still YAML.)
13. **Stages are pure and re-runnable where possible.** A stage that returns a document can be re-run and its output
    diffed; a stage that mutated GitHub mid-run cannot be cleanly retried. Idempotent, inspectable stages are what
    make Stage 4's audit trail and Stage 5's dispatch-only coordination possible.

---

## 3. The stage-output contract

Every pipeline stage (planner, architect, implementation/krew-lead, builder, validator, documenter) conforms to the
same shape: it consumes a typed input document and returns a typed output document. The agent never writes to GitHub,
the repo, or any shared resource as a means of "producing" its result; its result **is** the returned document. How
that document reaches the next stage depends on the boundary (§2.1): within a workflow it can travel as a local
object or file; across workflows it is serialized into shared state (a GitHub issue or PR) and verified back out
(§8, §11.1).

### 3.1 Document shape

A stage output is a structured artifact with two parts:

- **A metadata envelope** — a **YAML** object carrying stage identity, schema version, the input it was derived
  from, and extensible fields (see §3.3). Parsed under a strict schema so an unknown or misspelled key is an error,
  not a silent drop.
- **A body** — the stage's substantive content (for the planner, the canonical issue text per §5–§8; for the
  architect, the design and `kiro-plan` task list; and so on). The body is carried *inside* the structured document,
  so metadata can be attached without smuggling it into prose.

The required encoding is **YAML** (not free prose with markers scattered through it), precisely so that metadata is
a first-class, additive field rather than something a reader has to recover from the body text. YAML is the single
format for every stage (see principle 12 for why). The existing `kiro-plan` block is the precedent: a fenced YAML
document with a defined schema, parsed strictly.

#### 3.1.1 Canonical envelope shape

So two implementers cannot build incompatible documents while both following the prose, the canonical wire shape is
a single top-level YAML mapping with exactly these keys:

```yaml
# A stage-output document: a YAML mapping carrying a metadata envelope plus a body.
schema_version: 1                 # integer, REQUIRED. The output-contract version; ties to the §3.4 rule.
stage: planner                    # string enum, REQUIRED. One of:
                                  #   planner | architect | krew-lead | builder | validator | documenter
source:                           # mapping, REQUIRED. Identifies the input this output was derived from.
  kind: github_issue              #   string enum, REQUIRED: github_issue | document
  ref: "142"                      #   string, REQUIRED: the issue number, or the upstream document's identity
generated: 2026-10-20T20:00:00Z   # string, REQUIRED. ISO-8601 / RFC 3339 timestamp, UTC.
body: |                           # string, REQUIRED. The stage's substantive content as a block scalar.
  ## Problem
  ...the canonical issue text per §5-§8...
```

Rules for this shape:

- **Required keys:** `schema_version`, `stage`, `source` (with its required `kind` and `ref`), `generated`, and
  `body`. All five must be present; a missing one is a structural failure the guard rejects (§4.1).
- **Field name is `schema_version`, not `schema`.** There is one version field, an integer, and no bare `schema`
  alternative. (This resolves the earlier `schema` / `schema_version` ambiguity.)
- **`body` is a single YAML block scalar** (literal `|`), carrying the substantive content verbatim; it is not
  parsed as further YAML. This is what lets multi-line prose with colons, quotes, and `#` travel without escaping.
- **Unknown top-level keys are rejected within the declared `schema_version`** (`KnownFields(true)`), and keys are
  added only by incrementing the version per §3.4. The provenance hash is **not** one of these keys: it is not part
  of the agent's output and is added by the guard on commit (§4.3, §8).

### 3.2 Why structured rather than a marked-up body

The 2026-10-18 design put a provenance marker as an HTML comment on the last line of the issue body and hashed the
rest. That conflated two things the reader needs to keep apart: the *content* and the *metadata about the content*.
A structured envelope separates them cleanly:

- Metadata (provenance, source input, schema version, stage, generation time, and future additions) lives in the
  envelope, never in the body.
- The body is pure content, so the guard can validate it and the deterministic layer can render/write it however the
  destination requires (a GitHub issue body, a spec file, a PR description).
- Adding a new metadata field later is a schema addition, not a change to how the body is parsed.

### 3.3 Metadata envelope (minimum fields)

| Field | Meaning |
|-------|---------|
| `schema_version` | The output-contract version (integer), so the format can evolve without misclassifying old artifacts (§3.4) |
| `stage` | Which stage produced this document (planner, architect, …) |
| `source` | The input artifact this output was derived from (e.g. the raw issue number, or the upstream document's identity) |
| `generated` | ISO-8601 generation timestamp |
| *(extensible)* | Future metadata (model, token/cost accounting for Stage 4 §5 audit, classification, etc.) added as new keys |

Provenance (the hash) is **not** written by the agent into this envelope. The deterministic guard computes it over
the validated body and attaches it when it commits the output (§4, §8). The agent returns the body and the
non-provenance metadata; the guard stamps provenance.

### 3.4 How strict parsing and extensibility coexist

Principle 12 requires two things that are in tension unless the rule below is stated: unknown keys are rejected
(strict parsing), yet metadata keys (for example `model` or `cost`) are expected to be added over time. They coexist
through the envelope's `schema_version`, not by relaxing strict parsing:

- **Strictness is scoped to a declared `schema_version`.** Within a given version, the set of permitted keys is
  closed, and an unknown or misspelled key is an error (`KnownFields(true)`). Strict parsing is never relaxed to let
  arbitrary unknown keys through.
- **Adding a metadata key is a `schema_version` increment, not a silent in-place addition.** A producer that emits a
  new key declares the higher version that defines it.
- **A consumer parses against the version the envelope declares, and reacts explicitly to one it does not know.** An
  older consumer meeting a newer `schema_version` does not silently ignore the new key and does not blindly reject a
  single field; it detects that the document's version is newer than it understands and routes that as an
  unprocessable input (a punch-out per §14, not a silent drop and not a hard parse crash on the specific key).
- **Backward-compatible evolution is a version-negotiation decision, not a `KnownFields` exception.** If a later
  version is defined to be safely readable by an earlier consumer (the new keys are optional and ignorable), that is
  recorded as an explicit forward-compatibility allowance tied to those versions, so the compatibility set is a
  stated contract rather than an accident of lenient parsing.

The net effect: the schema can grow without ever misclassifying an old artifact (the version on the artifact says
exactly which rules apply) and without ever letting an unrecognized key pass unexamined inside a version it does not
belong to.

---

## 4. The guards (deterministic boundary)

Between every pair of stages sits a **boundary**, and a boundary is not one monolithic check. It is an ordered
**chain of guards**, each a small piece of deterministic Go (not an agent), that inspect the upstream stage's output
before the pipeline advances. This matches what Stages 4 and 5 envision: Stage 4 §2 lists some *common* kinds of
guardrail (deterministic validation checks, hooks and sentinel files, adversarial review agents) and Stage 5 §3 adds
adversarial review in its own context, but that list is illustrative, not a required or exhaustive checklist.
**Which guards a boundary runs is our determination for this system:** we pick the checks that are appropriate for
Kairon, we are not obligated to implement every type Stage 4 names, and we may add types that appear on no such list.
The fixed commitment is the shape (a chain of uniform guards sharing the two-method interface of §4.1), not a
particular catalog. The boundary
is designed as "many guards," not "a guard," from the start.

> **Terminology.** Elsewhere this document writes "the guard" as shorthand for **the deterministic boundary layer**
> as a whole: the guard chain (§4.1–§4.2) plus the terminal commit step (§4.3). Where a sentence says "the guard
> validates and commits," read it as that layer: the chain of guards admits the output, and the layer then commits
> once. No single guard both checks and commits; §4.3 is explicit that committing is not a guard.

### 4.1 A guard is a uniform two-method check

Every guard implements the **same interface**, which has **two methods**, so the boundary can compose guards without
knowing what each one does:

- **An admission method.**
  - **Input:** the upstream stage's output document (§3).
  - **Output:** a boolean, `pass` or `fail`. Nothing else. This method does not mutate the document, does not write
    to GitHub, and does not itself take failure action; it only answers "does this output satisfy my check." A
    `pass` admits the output to the next guard; a `fail` triggers the guard's own failure method.
- **A failure method.**
  - **Input:** a typed `FailureContext` (defined below) describing the failure the admission method found.
  - **Action:** the guard takes the appropriate action for its own check, deciding between a **retry** of the
    upstream stage (recoverable, §14.1) and a **punch-out** (terminal, §14.2). The guard owns this decision because
    only it knows whether its check is retry-recoverable. The deterministic layer invokes this method and supplies
    the context; it does **not** decide the handling. This method is the "well-known failure endpoint" referred to
    elsewhere: it is part of the guard, not an external component.

The split keeps admission (a pure pass/fail judgment) separate from handling (the action on failure) while both live
on the same guard. The admission method stays side-effect-free and cheaply testable; the failure method carries the
one guard-specific policy decision.

**`FailureContext`** is the typed value the admission failure produces and the failure method consumes. It carries at
minimum:

| Field | Meaning |
|-------|---------|
| `stage` | Which stage produced the rejected output |
| `guard` | Which guard's admission method failed |
| `attempt` | The current attempt number and the max for this boundary (§14.1) |
| `expected` | What the admission method required |
| `actual` | What it observed: the failing field, the gate output, or the reason |

`FailureContext` is produced **only on a fail** and flows only into the failing guard's failure method. It is also
exactly the information the terminal punch-out comment must contain (§14.2 step 1), so when the failure method
punches out, the comment is rendered from this typed context rather than reconstructed from a bare boolean.

Because the contract is uniform, guards are independent and order-composable. The following are **illustrative**
examples of distinct guards a boundary *might* run; none is mandatory, the set we actually run is our choice per
boundary, and it grows over time:

- **Structural guard:** the document parses under the strict schema, required sections are present, acceptance
  criteria carry `Verify:` lines, dependencies are in the single-line form (§7), size limits respected (the §5–§8
  issue standard).
- **Security guard:** no secrets, injected content, or disallowed instructions smuggled into the body (the
  body is untrusted data, principle 8).
- **Correctness / adversarial guard:** a check (deterministic, or an adversarial reviewer in its own context per
  Stage 5 §3) that challenges whether the output actually satisfies its stated intent.
- **…and others not yet designed.** The interface is fixed; the catalog of guards is open.

### 4.2 How the chain runs

The boundary runs the guards in order:

1. Run the next guard's admission method against the stage output.
2. **On `pass`:** move on and run the next guard. When every guard in the chain has passed, the boundary proceeds to
   commit (§4.3).
3. **On `fail`:** the admission method produces a `FailureContext` (§4.1) and the deterministic layer invokes **that
   guard's own failure method** with it. The failure method takes the appropriate action, deciding between a
   **retry** of the upstream stage (recoverable failures, §14.1) and the **punch-out / failure response** (terminal
   failures, §14.2). The guard owns that decision because only it knows whether its check is retry-recoverable, and
   it enacts the decision in its failure method rather than handing it to an external decider. The chain does not
   advance to the next guard or to commit while a failure is being handled.

A guard whose admission method answers only pass/fail, with handling behind its own failure method, is what keeps the
checks composable: adding a guard is adding those two methods, with no change to the chain runner or the other
guards.

### 4.3 Commit is a terminal boundary step, not a guard

Committing the side effect (for the planner boundary: the single authoritative GitHub write plus the provenance hash)
is **not** one of the guards. It is the terminal action the boundary takes **after the whole guard chain passes**:

1. **Compute provenance.** Canonicalize the validated body and compute the hash (§8), using the same code path that
   later classifies issues, so the writer and the classifier can never disagree. The body being canonicalized and
   hashed is the final body, with any dependency-number substitution (§7) already applied, so the hash covers the
   exact bytes that get committed.
2. **Commit, once.** Perform the single authoritative GitHub write: create or update the issue body, embed the
   computed provenance, and only then mark the issue ready for the next stage. Because the agent never wrote to
   GitHub, there is no window in which a half-planned body is live, and no concurrent-writer race to guard against
   inside the watcher.
3. **Advance.** Spawn the next stage with the committed, validated document as input.

This separation (guards decide admit/reject; the boundary commits once, after all pass) is what makes the watcher
race-guards from PR #374 unnecessary rather than merely patched: the `plannerInFlight` ordering, the backoff
recheck, and the `ProvenanceValid` in-flight check all existed to defend against the planner writing a valid body
mid-run. With the agent out of the write path, that failure mode does not exist.

### 4.4 Configuration: hard-coded first, configurable later

For the initial implementation, the guards on each boundary and their order are **hard-coded** in Go. That is
deliberate: it keeps the first cut simple and the chain fully deterministic. The longer-term target, which the
uniform interface is chosen to enable, is for the guard chain on a boundary to be **configurable**: adding or
removing checks between steps without rewriting the chain runner. The pass/fail-plus-failure-callback interface is
the seam that makes that later configurability possible, so it is worth getting right even while the first set of
guards is hard-coded.

---

## 5. The three issue types (planner body content)

The planner's body content classifies the request into one of three types, because there are only three genuinely
different *shapes* of acceptance criteria:

| Type | Contract | Acceptance-criteria shape |
|------|----------|---------------------------|
| **Feature** | Additive — behavior that does not exist should exist | Forward spec of new observable behavior |
| **Bug** | Corrective — observable behavior is wrong; here is the gap and a reproduction | Current-vs-expected plus a deterministic reproduction |
| **Refactor** | Structural — internal structure should change; observable behavior must *not* | Behavior-preservation guarantee |

Everything else folds into one of these three as a **domain costume**, with extra detail carried in an optional
section (§6):

| Former type | Folds into | Why |
|-------------|-----------|-----|
| Documentation | Feature (additive doc) or Bug (wrong doc) | The observable "behavior" is the rendered doc |
| Performance | Feature + optional *Baseline & Target* | Additive measurable behavior with a quantified AC |
| Security fix | Bug + optional *Security* | A vulnerability is wrong behavior with a repro (the exploit) |
| Chore / dependency upgrade | Refactor + optional *Migration* | Structure changes, observable behavior preserved |
| Infrastructure / IaC | Feature or Refactor + optional *Operational* | Same three shapes; env/rollback are optional detail |
| Spike / investigation | **Excluded** | Deliverable is knowledge, not a PR — out of an auto-implement pipeline |

Getting the classification slightly wrong costs at most an extra or missing optional block, never a broken contract.

---

## 6. The shared spine (all three types)

The planner's body carries these sections, in this order. (This is the content the guard validates and the
deterministic layer renders into the GitHub issue body; it is not the agent's GitHub write.)

1. **Title** — one line, imperative, scoped, ≤70 characters, one PR's worth of change.
2. **Problem Statement** — 2–5 sentences: what is true today, the evidence (file:line, command output, observed
   behavior), why it is a problem, and what should be true after. Stated separately from any proposed solution.
3. **Context** — why this matters, who is affected, resolvable references. Non-binding only.
4. **Scope — In and Out** — the highest-leverage section. In Scope lists packages/components (existing, or marked
   `(new)`). Out of Scope has at least one entry: what not to touch.
5. **Acceptance Criteria** — the contract the validation step checks. Each is an independently verifiable,
   observable-outcome statement with a black-box `Verify:` line.
6. **Constraints** *(optional; binding)* — hard rules the implementation must obey, each with a reason and a
   verification. Separate from Context.
7. **Dependencies** — issues that must land first, in the machine-parseable form the scheduler reads (§7).

Absent by design: no User Story section (fold the *why* into the Problem Statement), no Verification / Test Plan
section (§6.1), no Definition-of-Done-as-commands section (pipeline-level concern).

### 6.1 Optional sections (planner attaches by classification)

| Section | When attached | Carries |
|---------|---------------|---------|
| **Functional Requirements** | Feature | Numbered, each testable: inputs, outputs, state changes |
| **Interface / Contract** | Feature with an external surface | Exact signatures, request/response schemas, status codes, error shapes; for UI, component states and a design reference |
| **Edge Cases & Error Handling** | Feature/Bug with non-trivial failure modes | Enumerated failure modes; the agent handles exactly the cases named |
| **Example Interactions** | Feature | Concrete input → expected output pairs that double as acceptance tests |
| **Reproduction** | Bug | Numbered deterministic steps, minimal triggering input, environment, and exact evidence pasted verbatim |
| **Current / Target State** | Refactor | The structure today and the desired structure, concretely |
| **Behavior-Preservation Contract** | Refactor | The explicit no-functional-change rule; the acceptance test is "existing tests still pass unchanged" plus any characterization tests first |
| **Baseline & Target** | Performance | A measured baseline, a quantified target, and the correctness guard |
| **Security** | Security fix | Vulnerability class, threat model, affected surface, a test proving the exploit is closed |
| **Migration** | Chore / dependency upgrade | Pinned target versions, the migration to follow, breaking-change handling |
| **Operational** | Infrastructure / IaC | Target state, prod-in-scope flag, idempotency, rollback plan, blast-radius limits |
| **Data / Migration** | Any schema change | Schema changes, backfills, feature flags |

### 6.2 Why there is no Verification / Test Plan section

A per-issue Verification / Test Plan leaks a solution-space concern into a problem-space document and invites hollow
or wrong plans. The load splits three ways: verifiability stays in Acceptance Criteria as observable outcomes; test
mechanics become the implementation agent's job; uniform guarantees (full suite, lint, type-check, build, security
scan) move to pipeline-level deterministic gates applied identically to every issue. The one exception: for bug
reports a *reproduction* is problem-space (the author describing the problem), so reproduction steps stay and the
agent turns them into a regression test.

---

## 7. Dependencies format

The scheduler reads issue-number dependencies in a narrow set of formats (see `internal/watcher/dependencies.go`).
The planner body **must** express dependencies in the single-line form the scheduler parses:

```
Dependencies: #12, #15, #18
```

A `## Dependencies` header followed by a bullet list parses to **zero** dependencies and releases dependents early.
A dependency is satisfied iff the referenced issue is closed. When the planner proposes a chain of issues, the
deterministic layer captures each created issue number as it commits each issue, and substitutes it into later
issues' `Dependencies:` line **before those later issues enter their own guard chain and provenance step** (§4.3).
Substitution is part of preparing a later issue's body for validation, not a post-hash edit: the body the guard
chain validates and the commit step hashes is already the final, number-substituted body, so the provenance hash
always covers exactly the committed bytes. (Equivalently: never substitute after hashing; if a number is
substituted, the affected issue is re-validated and re-hashed over the substituted body.) The agent proposes the
chain; the guard resolves the numbers.

---

## 8. Provenance — computed and written by the guard

The pipeline must distinguish a planner-canonical issue from a raw human-written one, so it can trust the structure
of the first and route the second through the planner. Under the new contract this is a property the **deterministic
guard** establishes, not something the agent writes.

### 8.1 What it proves

That a given issue body is a committed planner output, unmodified since the guard wrote it. A static label is
forgeable and cannot detect a post-commit human edit; a content hash catches both the unmarked (manual) case and the
marked-but-edited case. This matters specifically because the planning→implementation boundary is **inter-workflow**
(§2.1): the issue is the only thing that crosses to the (possibly different) implementation machine, it is a public
editable surface, and the consuming workflow has no other way to confirm the body it reads is the one the producing
workflow validated. Within a single workflow, where nodes share local state, the same guarantee would be
cheap-to-free; across workflows the hash is what carries it.

### 8.2 Who computes it

The **guard** canonicalizes the validated body and computes the hash in deterministic Go, as step 3 of §4, using the
same function the classifier uses on read. The agent never computes or writes the hash. This removes the entire
reason the 2026-10-18 scheme needed to be tamper-evident against its own writer: the writer is now trusted code.

### 8.3 Scheme

- **Algorithm:** SHA-256 over the canonicalized issue body (marker excluded from its own input, line endings
  normalized, trailing whitespace removed).
- **Strength:** tamper-*evident*, not tamper-*proof*. A GitHub issue body is public and editable. The marker's only
  job is to let the pipeline classify an issue as canonical / manual / edited and route accordingly. **No HMAC, no
  secret**: an unmarked or mismatched issue is not acted on with elevated privilege — it is re-routed through the
  planner for canonicalization, which makes a best-effort fill and either produces a valid hashed body or punches out
  for human review on data it cannot reasonably infer (§10.2). The marker gates *trust in the structure*, not access.
- **Versioning:** the scheme carries a version (`v1`) so canonicalization rules can evolve.
- **Marker form:** the deterministic layer writes a single HTML comment on the last line of the rendered issue body,
  excluded from its own hash input:

  ```
  <!-- planner-provenance: v1 sha256=<hex> generated=<ISO-8601> -->
  ```

  Note this is the *rendered GitHub representation*. The planner's own output document (§3) carries no hash; the
  guard adds the marker when it renders and commits the body.

### 8.4 Read states and pipeline action

On read, the deterministic layer recomputes the hash over the canonicalized body and compares:

| State | Meaning | Pipeline action |
|-------|---------|-----------------|
| **Valid hash** | Guard-committed planner output, unmodified | Process with strict assumptions (the schema is trusted) |
| **No marker** | Manually created | Route through the planner (automated intake, §10.2); the guard canonicalizes, hashes, and commits if the planner can fill the required slots (best-effort inference included), or posts a punch-out comment and halts if a required slot cannot be reasonably inferred |
| **Hash mismatch** | Committed planner issue, edited afterward | Treat as untrusted; re-route through the planner (same best-effort-then-punch-out outcome as "No marker") |

Two of three states converge on one routing action, keeping the logic a single branch:

> **If the hash is not valid, planner-process the issue; otherwise proceed.**

"Planner-process" is the route; its automated outcome is either a guard-committed canonical body or a punch-out that
posts a comment and halts the issue (§10.2).

Downstream invariant: **anything the implementation pipeline consumes carries a guard-written valid hash.**

---

## 9. Manual-issue intake (auto-routing)

Manually created issues are automatically routed through the planner for classification. Routing is the action for a
missing or mismatched marker; the *outcome* of that routing depends on whether a human is in the loop (§10), because
the planner runs in two different modes.

- The planner is the **intake** for the whole pipeline — the one stage whose *input* may be a raw human-written
  GitHub issue. Every other stage receives an already-validated, guard-committed document as input.
- **Automated intake has no human to interview.** When the watcher picks up an unmarked or mismatched issue, it runs
  the planner non-interactively. The planner cannot ask questions or offer defaults for approval. It makes a
  **best-effort fill** of open slots by reasonable inference from the issue and the repository (§10.2 step 3,
  recording the evidence for each inference), and then either (a) produces a complete, canonical output document,
  which the guard validates and commits; or (b) finds a required slot it **cannot reasonably infer** and
  **punches out for human review** (§10.2). A punch-out is a terminal failure for that issue: the deterministic
  layer posts a comment listing exactly what must be added to the issue description, and **all further processing of
  that issue halts** until a human edits the issue and the comment is resolved. The planner fills what it can
  reasonably ground and punches out for the rest; it never guesses among materially different options or fabricates
  a requirement.
- **Interactive intake has a human.** When a person runs the planner directly, the elicitation flow (§10.1) applies:
  the planner interviews the user one decision at a time and fills gaps with confirmed answers. This mode never runs
  from the watcher.
- In both modes the planner's **output document** is the same structured artifact (§3); the guard validates and
  commits it identically. The difference is only in how gaps are resolved: by interview (interactive), or by
  best-effort inference and a punch-out for what cannot be inferred (automated).

---

## 10. Planner behavior (two modes: interactive and automated intake)

The planner runs in two modes with the **same output contract** (§3) but different gap-resolution rules:

- **Interactive** (a human runs `kiro-cli chat --agent planner`): the planner's goal is to **capture every
  requirement the user holds, with zero silent assumptions, in as few turns as the request allows**, then **return a
  structured output document** whose body is a canonical issue. Gaps are resolved by interview (§10.1).
- **Automated intake** (the watcher routes an unmarked or mismatched issue, non-interactively): there is **no human
  to interview**. The planner canonicalizes from the issue body and the repository, making a best-effort inference
  for gaps it can reasonably ground (§10.2 step 3). For any required slot it cannot reasonably infer, it
  **punches out for human review** (§10.2); it never interviews, and never guesses among materially different
  options.

In both modes the planner does **not** create the GitHub issue or write the hash — it returns the document (or a
punch-out result) and the guard acts on it (§4).

### 10.1 Interactive elicitation flow (human present only)

This flow applies **only** in interactive mode. It never runs under the watcher.

1. **Investigate before asking.** Read the relevant code, docs, config, related issues. Anything the repository
   answers is stated with a file reference, never asked.
2. **Classify.** Decide Feature / Bug / Refactor and which optional sections (§6.1) apply.
3. **Coverage map.** Track these slots, each filled from code, from the user, by an accepted default, or still open:
   problem and evidence · desired behavior · scope boundary and out of scope · acceptance criteria and their
   verification · edge and error behavior · constraints · dependencies. Finalize the output only when no slot is
   open.
4. **Interview, one decision per turn**, starting with the highest-impact open slot. Offer a) b) c) Other as the
   choices and **recommend exactly one of them** with one line on why it matters, subject to two rules:
   - **Each option is a single, unambiguous commitment.** An option may bundle multiple items joined by *and*
     ("a) add validation **and** the error response", where picking a) means both are in scope) but **never** by
     *or* ("a) add validation **or** the error response"). A disjunctive option is forbidden: if the user picks it,
     it is not clear which thing they agreed to, so the decision — and the acceptance criteria derived from it — is
     not bounded. When items are genuine alternatives, split them into separate options (a) = x, b) = y) so the
     user's single selection resolves to exactly one outcome. When they genuinely go together, state them as a
     conjunction so picking the option commits to all of them.
   - **The recommendation is a single option.** Recommend exactly one of the listed options, never a branching or
     conditional suggestion ("a) if X, else b)", "a) or b) would work"). If the agent genuinely cannot pick one, the
     choice is material and belongs to the user: present the options without a recommendation rather than hedging.

   Together these guarantee the user knows exactly what they are agreeing to when they select an option, and the
   acceptance criteria are bounded by a single, fully-resolved decision. A bug whose root cause may differ from the
   symptom gets a symptom / root cause / both choice (here "both" is a conjunction, so it is a valid single option).
5. **Constraints are asked, never invented.** Investigation finding an established pattern, or a request phrased as
   design, triggers a "should this be required?" choice: a) mandate as a Constraint, b) note as non-binding Context,
   c) leave to the architect — with a single recommended option (per step 4's singular-recommendation rule). "No
   preference" means no constraint.
6. **Escape hatch.** If the user says to use the recommendations, every remaining non-blocking slot takes its
   recommended default.
7. **Confirm defaults.** Before finalizing, one turn lists every defaulted slot plus the proposed edge/error behavior
   and asks a single question. The only turn allowed to contain a list. Skipped if nothing was defaulted.
8. **Draft review.** Show the issue body (per §6–§7), then a coverage summary *outside* the issue, then one approval
   question. Revisions return here.
9. **Return the output document.** On approval, ask the label question as a separate turn, then **return the
   structured output document** (§3) — body plus metadata envelope — to the guard. The guard validates, computes
   provenance, and commits the GitHub issue (§4). The planner itself performs no `gh issue create` and writes no
   hash.
10. **Split oversized requests.** If the request needs more than about 6 blocking questions, or the body would exceed
    the size limits (≤6 In Scope entries, ≤8 acceptance criteria), propose a split — remaining issues as one-line
    summaries in dependency order — and continue with only the first. Each proposed issue is a separate output
    document; the guard commits them in dependency order and resolves the `Dependencies:` numbers (§7).

### 10.2 Automated-intake behavior (watcher, no human)

When the watcher routes an unmarked or mismatched issue, the planner runs non-interactively and the §10.1 flow does
not apply (no interview, no per-turn approval, no defaults-for-confirmation). The behavior is:

1. **Investigate.** Read the issue body and the repository (code, docs, config, related issues) exactly as in §10.1
   step 1. Anything the repository answers is resolved from the repository.
2. **Classify and map coverage.** Decide Feature / Bug / Refactor and fill the coverage-map slots (§10.1 step 3)
   from the issue and the repository.
3. **Best-effort fill.** For each still-open slot, make a reasonable inference from the evidence actually present —
   the issue text, the repository's established patterns and conventions, related issues, and the project's
   defaults. A slot filled this way is filled from grounded evidence, not invented: the planner records *what* it
   inferred and *the evidence it inferred from*, so the inference is reviewable. "Reasonable" means a competent
   engineer reading the same issue and repository would reach the same conclusion; a coin-flip among materially
   different options is not reasonable and is not filled.
4. **Decide: canonicalize or punch-out.**
   - If every **required** slot is now filled — directly from the issue/repository, or by a reasonable best-effort
     inference (step 3) — produce the canonical output document (§3) and return it. The guard validates, hashes, and
     commits it, and processing continues.
   - If any required slot **cannot be reasonably inferred** (a genuine requirement the issue does not state, the
     repository cannot settle, and no grounded inference resolves — e.g. a product decision, an external contract,
     an ambiguous scope with materially different plausible readings), **punch-out for human review**. The planner
     returns a punch-out result naming each unresolvable item and why it could not be inferred; it does **not**
     fabricate the data, guess among materially different options, or emit a partial canonical document.
5. **Punch-out is terminal and forces human intervention.** On a punch-out, the deterministic layer executes the
   shared punch-out contract (§14): it posts a comment on the issue stating exactly what must be added to the issue
   description to proceed, applies the fail-state label, and thereby **halts all further processing of that issue**
   — no implementation agent is spawned, and the issue is not re-routed on later polls — until a human reviews the
   comment, addresses it, and clears the fail-state label to restart. (This is the automated counterpart of the
   interactive flow's interview: where a human present would be asked, an automated run asks via a comment and waits.
   Best-effort inference reduces how often this happens, but never substitutes a guess for a decision only a human
   can make.)

### 10.3 Hard bars (both modes)

- A planner output contains **no ungrounded assumptions**. Every statement traces to the code, the user (interactive
  only), a default the user accepted (interactive only), or — in automated intake — a reasonable inference whose
  evidence the planner recorded (§10.2 step 3). A required slot that cannot be grounded is a punch-out (§10.2),
  never a guess.
- **Options are unambiguous; recommendations are singular.** Every option the planner offers is a single commitment —
  a lone item or an *and*-conjunction of items — **never** an *or*-disjunction, so a user's selection resolves to
  exactly one bounded outcome (genuine alternatives are split into separate options). And when the planner
  recommends, it recommends exactly one concrete option, never a branching or conditional suggestion; if it cannot
  pick one, it presents the options un-recommended. Both rules exist so the acceptance criteria are bounded by a
  single, fully-resolved decision (§10.1 step 4).
- No vague terms in acceptance criteria ("properly", "appropriate", "gracefully", "fast", "robust", "as needed",
  "etc.", "should work") unless quantified.
- No design outside Constraints; no Constraint the user did not state or confirm.
- Don't re-derive what the repository already states: resolve it from the repository directly (don't ask in
  interactive mode, don't treat it as an open inference in automated mode).
- The planner returns a well-formed output document (§3), or in automated intake a punch-out result (§10.2), and
  nothing else; it does not touch GitHub. Provenance and the punch-out comment are the guard's responsibility,
  enforced at the boundary (§4, §8), not bars the agent satisfies by writing to GitHub itself.

---

## 11. Division of labor across the pipeline

Each stage owns a document and a guard sits on each boundary. No stage writes to a shared resource as its result; the
guard commits.

| Stage | Produces (document) | Guard at its output boundary commits | Does not own |
|-------|---------------------|--------------------------------------|--------------|
| **Planner** | Canonical issue document (problem, observable done, scope, binding constraints) | GitHub issue write + provenance hash | Solution design, structure, test mechanics, task lists, its own GitHub write |
| **Architect** | Spec + `kiro-plan` task list (structured) | Spec artifact write; plan validated (tasks reference registered agents only) | Re-deciding requirements |
| **Implementation (krew-lead)** | Code + tests document / branch artifact derived from acceptance criteria | Branch/PR creation | Expanding scope, generic quality gates |
| **Deterministic gates** | Pass/fail results (suite, lint, type-check, build, security scan) | Gate verdict that admits or blocks advancement | Per-issue judgment |
| **Validation** | Per-criterion verdict against each `Verify:` line | Final accept/reject | Writing tests the implementer already wrote |
| **Documenter** | Documentation output document | Doc write | Changing behavior |

### 11.1 Stage *inputs* are documents too (current gap)

The contract (§3) applies to both ends of a boundary: a stage consumes a typed input document and returns a typed
output document. The watcher→implementation boundary must meet the input half of this contract, but the way it meets
it is fixed by §2.1: **this is an inter-workflow boundary** (the planning workflow to the implementation workflow),
so the two ends are not guaranteed to run on the same machine, and the only thing that can cross between them is the
**shared state — the GitHub issue**.

**The inter-workflow reality.** The planning workflow and the implementation workflow do not generally run on the
same host, so the planner node's in-memory output document cannot be handed to the krew-lead node as an object — that
move is only available *within* a workflow (§2.1). Consider the canonical case:

1. **User A** runs the planner locally. The planner's output document (§3) exists only on A's machine, in A's process.
2. The guard **on A's machine** validates that document and commits it: it renders the body into the GitHub issue and
   stamps the provenance hash (§4). GitHub now holds the *serialized* result — the issue body plus the hash — but
   **not** the YAML envelope object. At this instant A's machine and GitHub are in sync.
3. Someone (a human, or an automated assignment) assigns the issue to **User B**, who runs Kairon on a **different
   machine**. B's instance picks the issue up to implement it.
4. **B's machine has never seen A's document.** It has only what GitHub carries: the issue body and the hash. There
   is no shared filesystem, process, or artifact store between A and B through which the structured document could
   travel.

So **GitHub is the serialization and transport medium between stages that live on different machines**, and the issue
body as committed by the guard **is** the serialized stage-output document. The hash is what makes this safe: it lets
B's guard verify, deterministically, that the body B reads is byte-for-byte the planner-validated content A's guard
committed, before admitting it to implementation. A re-read of the GitHub issue is therefore not the loosely-defined
"re-fetch" defect at this boundary — across a machine boundary it is the *only* possible transport, and the hash
turns it from an untrusted read into a verified document load.

**Current behavior** (`internal/agent/manager.go`, `Spawn`): the watcher spawns
`kiro-cli chat --agent krew-lead --no-interactive --trust-all-tools` with a **free-text sentence** as the prompt —

```
Process issue #N from repo owner/name. Worktree name: issue-N-PID. You are already in
the worktree directory — all file operations happen here. Skip worktree creation (step 2).
```

— plus a few environment variables (`ISSUE_NUMBER`, `REPO`, `KAIRON_WATCHER_PID`, `WORKTREE_PATH`). krew-lead parses
that sentence and re-fetches the issue from GitHub itself. The genuine defect here is **not** that B reads from
GitHub (it must), but that B reads an **unverified** body and treats a free-text prompt as its contract: nothing on
B's side checks the hash before implementation, so B cannot distinguish a planner-validated issue from one a human
edited after the planner committed it.

**Target behavior:** the boundary is a *load-and-verify*, not an object handoff:

- The guard on the consuming machine (B's) **re-reads the issue from GitHub** — the only cross-machine transport —
  and **verifies the provenance hash** over the canonicalized body (§4, §8) before admitting the issue to
  implementation. A body whose hash does not validate is not implemented; it is routed back through intake or punched
  out (§10.2, §14), because it is either un-planned or was edited after planning.
- The verified body is then reconstructed into the typed input document krew-lead consumes (the same schema §3
  defines for the planner's output), so krew-lead implements a validated document rather than parsing a sentence and
  trusting a raw read.
- Operational parameters (worktree path, repo, issue number) remain environment/CLI inputs; what changes is that the
  *requirements content* is a hash-verified document loaded from GitHub, not an unchecked live read triggered by a
  prompt.

This keeps the §3 contract intact across a machine boundary without pretending machines share memory: the document is
*serialized to GitHub by the producing guard* and *verified from GitHub by the consuming guard*. It is also what makes
the Stage 4 audit trail (§13) reproducible on the consuming side — the input krew-lead ran on is exactly the bytes the
hash covers, retrievable from the issue itself, not an in-process object that never left A's machine.

> **Open design point for the rebuild.** Whether GitHub should carry *more* than the rendered body — e.g. the full
> YAML envelope persisted in a machine-readable block or an attached artifact — is a real decision, because today the
> envelope's non-provenance metadata (`stage`, `source`, `generated`, model/cost) does **not** survive the trip to B.
> The minimum that must cross is the body and a verifiable hash; preserving the rest of the envelope across machines
> is a separate choice to make deliberately, not to assume.

---

## 12. What the EDD rebuild inherits from this document

For the planner rebuild, this document is the problem-space definition. The planner eval specification and the issue
standard it certifies against must encode:

- The three-type classification and the shared spine (§5–§6).
- The optional-section catalog and attach-by-classification rule (§6.1).
- The removal of any per-issue test plan; verifiability lives in observable acceptance criteria with `Verify:` lines
  (§6.2).
- The single-line `Dependencies:` format (§7).
- **The execution model (§2.1): agents vs nodes vs workflows, and the two transport regimes.** A workflow runs on one
  machine (intra-workflow handoffs may use local objects/files, optionally committed for audit); different workflows
  are not guaranteed to co-locate (inter-workflow handoffs cross only through shared state: today GitHub issues
  between planning and implementation, PRs between implementation and review). This determines which transport each
  boundary may assume and is why provenance is load-bearing at the inter-workflow boundaries.
- **The stage-output contract (§3): the planner returns a structured YAML document with a body and a metadata
  envelope, parsed under a strict schema — it does not write to GitHub.**
- **Provenance as a guard responsibility (§4, §8): the deterministic layer computes and writes the hash; the planner
  never does.** The planner eval therefore asserts against the returned document, not against a GitHub side effect,
  which is what makes it deterministically testable.
- Auto-routing of manual issues, making the planner the pipeline's intake (§9).
- **Two planner modes (§10):** interactive elicitation (human present, §10.1) and automated intake (watcher, no
  human, §10.2). The automated path cannot interview; it makes a best-effort fill from the issue and repository
  (recording the evidence for each inference) and **punches out for human review** on any required slot it cannot
  reasonably infer — posting a comment naming the missing data and halting the issue until a human resolves it. It
  never guesses among materially different options or fabricates a requirement.
- The coverage map and the shared hard bars (§10.3).
- **Stage inputs are documents too, across machines (§11.1):** Kairon is distributed, so the planner (machine A) and
  implementation (machine B) need not share a host. GitHub is the serialization and transport medium between them:
  the producing guard writes the validated body plus hash to the issue, and the consuming guard re-reads the issue
  and verifies the hash before admitting it to implementation. The defect to remove is not the GitHub read (across a
  machine boundary it is the only transport) but implementing an **unverified** body parsed from a free-text prompt;
  the target is a hash-verified document load.
- **Failure handling is two-tier (§14):** recoverable failures retry up to a bounded max (`max_retries`,
  `max_qa_retries`) to absorb the system's non-determinism; only when retries are exhausted or the failure is
  inherently unrecoverable does the issue punch out — posting an explanatory comment and applying the fail-state
  label, which halts it until a human clears the label to restart with a fresh budget. The planner's punch-out is an
  immediately-terminal (Tier 2) case because no retry can supply a human-only decision.

The planner rubric criteria (`conversation_protocol`, `elicitation_completeness`, `gate_compliance`, `issue_quality`,
`requirements_boundary`) still apply. This rewrite changes what counts as a correct output (`issue_quality`,
`requirements_boundary`), and reframes `gate_compliance` from "the agent wrote a valid marker" to "the agent returned
a well-formed output document the guard could validate and commit."

---

## 13. Impact on Stages 4 and 5

This contract is the template the later stages require, not a planner-only concern:

- **Stage 4 (deterministic workflow with guardrails between steps).** Guards between steps (§4, §11) are the literal
  Stage 4 requirement: a deterministic check positioned between stages, never an agent checking itself mid-run.
  Typed stage outputs (§3) are what make Stage 4 §5's per-step audit trail (model, token/cost, failure-origin
  tracing) possible — a side effect on a shared issue is not traceable to a step; a returned document is.
- **Stage 5 (dispatch-only coordination).** A coordinator that only dispatches works only if each stage is already a
  scoped agent with a typed, guard-validated output. Getting the planner boundary right now is load-bearing for
  Stage 5; every stage converted to the §3 contract is one the coordinator can compose without knowing its internals.

The planner is the first stage converted to this contract because it is the pipeline's intake and the stage whose
old write-to-GitHub design produced the defects in §1. The architect's `kiro-plan` block already demonstrates the
target shape. The remaining stages (§11) are converted to the same contract as the pipeline matures toward Stages 4
and 5.

---

## 14. Failure handling: retry, then punch-out (all stages)

This is a distributed, non-deterministic system. An agent run can fail for a transient or recoverable reason (a flaky
tool call, a validation rejection the next attempt fixes, a timeout) as readily as for a true, unrecoverable one (the
requirement genuinely cannot be met, the data genuinely is not there). The failure model has **two tiers**, and the
boundary between them is a bounded retry budget:

- **Tier 1 — bounded retry (recover where we reasonably can).** A recoverable failure at a stage boundary is retried
  up to a configured maximum. Retries are how the pipeline absorbs non-determinism without human intervention.
- **Tier 2 — punch-out (true failure).** Only when the retry budget is exhausted, or the failure is inherently
  unrecoverable (no retry could fix it — e.g. the planner cannot infer a product decision, §10.2), does the issue
  enter the fail state and hand off to a human.

A failure is never swallowed and never silently advances the issue. But it is also not an immediate punch-out:
punch-out is the *terminal* outcome after recovery has been given its bounded number of chances.

### 14.1 Tier 1 — bounded retry

Each retryable boundary has a maximum attempt count (today: `max_retries` for the implementation run, `max_qa_retries`
for the builder↔validator QA loop, both defaulting to 3 in `internal/config/config.go`). On a recoverable failure the
deterministic layer re-runs the stage and increments the attempt counter, up to the max. For example: the builder
writes code, the validator rejects it; the system re-runs the builder (with the validator's feedback) up to
`max_qa_retries` times before the failure becomes terminal. Retries happen **within** the fail state's absence — the
issue is still being actively worked, no fail-state label is applied, and no human action is required while attempts
remain.

What is retryable versus immediately terminal is a per-boundary decision the design must state: a transient tool error
or a validator rejection is retryable; a planner punch-out for a decision only a human can make (§10.2) is not — no
number of retries produces the missing information, so it goes straight to Tier 2.

### 14.2 Tier 2 — punch-out (the terminal failure contract)

When Tier 1 is exhausted (retry budget spent) or the failure is unrecoverable by nature, the deterministic layer (not
the agent) performs all three of the following, together:

1. **Post a comment** on the issue stating *why* it failed, in enough detail for a human to act: which stage failed,
   how many attempts were made, what was expected, what actually happened (the missing requirement, the failing gate
   and its output, the validator's standing rejection, the error), and what the human must change to let the issue
   proceed. These are exactly the `FailureContext` fields the guard's failure method already holds (§4.1), so the
   comment is rendered from that typed context, not reconstructed. The comment is the human-readable record of the
   failure, not a generic "failed" note.
2. **Apply the fail-state label** that removes the issue from the watcher's polling set, so no stage picks it up
   again while it is in the fail state. (Today that label is `<trigger-label>-failed`, e.g. `kairon-failed`;
   `internal/github/client.go` `ListIssues` already excludes any issue carrying it, alongside the `-done` label.)
3. **Halt the issue.** No next stage is spawned and the issue is not re-routed on later polls, by virtue of the
   fail-state label.

### 14.3 How a human restarts a punched-out issue

The fail-state label is a one-way stop that only a human clears. To restart, a human reviews the comment, makes the
change it asks for (edits the issue body, resolves a dependency, fixes an environment problem), and **removes the
fail-state label**. Clearing the label returns the issue to the polling set, and the pipeline picks it up again on the
next poll with a fresh retry budget. Clearing the label *is* the restart; there is no separate "retry" action at the
terminal tier, and no automatic re-entry while the label is present. (This is distinct from Tier 1 automatic retries,
which happen before the label is ever applied.)

### 14.4 Why this is uniform

Consistency across stages is the point. Two failures of the same tier — in the planner, the implementation agent, a
gate, or the validator — should be indistinguishable in *shape* to the human and to the pipeline: bounded automatic
retry while recovery is plausible, then a comment that explains the terminal failure and a label that gates it. This
keeps the watcher's selection logic a single rule ("skip issues carrying the fail-state or done label") regardless of
which stage failed, lets the Stage 4 audit trail attribute every halt (and every retry) to its originating stage, and
gives the human one consistent place (the issue) and one consistent action (clear the label) to resume any truly
failed issue. The planner's punch-out (§10.2) is one instance of the Tier 2 contract, not a mechanism of its own.

### 14.5 Current state and the gap

Parts of this already exist: `internal/agent/manager.go` retries the implementation run up to `max_retries` and only
applies `<label>-failed` once retries are exhausted (the two-tier shape is already there for that boundary). What is
**not** yet uniform:

- The terminal path applies the label but does **not** reliably post a comment explaining *why* it failed or how many
  attempts were made, so a human sees a `kairon-failed` issue with no stated reason. The contract requires the
  explanatory comment on every terminal failure.
- The planner punch-out path (§10.2) is new and must use this same comment + label mechanism, and must be treated as
  an immediately-terminal (Tier 2) failure since no retry can supply a human-only decision.
- Gate and validation failures must route through the same two-tier contract (bounded retry where recoverable, then
  the terminal comment + label) rather than failing silently or only logging.

Converting all stages to the §14 contract is part of the same work as the §3 stage-output contract and the §11.1
input-document contract.
