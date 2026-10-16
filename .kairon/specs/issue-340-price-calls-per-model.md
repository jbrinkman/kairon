# Design Spec: Evals: price calls per model and mark unknown costs

Closes #340

## Problem Summary

`costFromUsage` (`internal/eval/types.go`) prices every call at a hard-coded $3 input / $15 output per million tokens, whatever the model. A `claude-haiku-4.5` call is therefore reported at Sonnet cost. The calculation also lives in `internal/eval`, so production workflow audit records (which share the `inference.CallRecord` format, #294) cannot reuse it.

kiro-cli reports no token counts or model, so its calls stay estimates (out of scope). This issue makes cost correct for whatever usage a backend reports, and visible when it is unknown.

## Current State (verified by reading the code)

- `internal/eval/types.go`: `costFromUsage(model, usage) CostInfo`, hard-coded $3/$15. `CostInfo{TokensIn, TokensOut, EstimatedUSD, Model, UsageSource}` with `Add`.
- Callers of `costFromUsage`: `runner.go` (`completeAgentCall`, `scoreLLMJudge` x2), `judge_check.go`, `simulated_user.go`.
- `newCallRecord` (`runner.go`) copies `cost.EstimatedUSD` into `inference.CallRecord.CostUSD`; the record's `Model` is `resp.Model`, falling back to `req.Model`. `CostInfo.Model` is only `resp.Model` (no fallback), so the two can differ today (stub turn with no `model`).
- Cost totals are summed by hand in three places that do not use `CostInfo.Add`: `agentCostTotals` and `setAgentVerdict` (`verdict.go`), `calculateAgentJudgeCosts` (`diff.go`). `Summary.TotalCost` is recomputed from `AgentVerdicts[*].Cost`.
- `internal/config/evals.go`: `LoadEvals()` reads only the `evals` key of `./.kairon/config.yaml`; missing file yields defaults. `config.Load()` reads the whole file into `Config`.
- `pinRun` (`provenance.go`) calls `config.LoadEvals()` and stores the result in `cfg.pins` (`runPins`, nil-safe accessor methods).
- Stub backend (`internal/inference/stub.go`): judge responses always report `Model: "stub"`; agent responses report `turn.Model` (may be empty). kiro-cli (`kirocli.go`/`exec.go`) echoes `req.Model` as `resp.Model`.
- `inference` imports only stdlib + `yaml.v3`; it must not import `internal/eval`. `internal/config` may import `inference` (no cycle: `inference` does not import `config`).
- Container runs: pricing happens host-side in `completeAgentCall` from the transported `inference.Response`, so native and `--sandbox` runs price identically with no extra work.
- Legacy result files: all new JSON fields below are `omitempty`/additive, so old files still decode; `eval diff` only reads existing fields.

## Solution Approach

### 1. Pricing mechanism in `internal/inference` (new `pricing.go`)

```go
type Price struct {
    InputUSDPerMTok  float64 `yaml:"input_usd_per_mtok"  json:"input_usd_per_mtok"`
    OutputUSDPerMTok float64 `yaml:"output_usd_per_mtok" json:"output_usd_per_mtok"`
}
type PriceTable map[string]Price   // keyed by EXACT model id (no trimming, no case folding)

type Cost struct {
    USD   float64
    Known bool
}

func (t PriceTable) Cost(model string, u Usage) Cost
func (t PriceTable) Validate() error
func (p *Price) UnmarshalYAML(node *yaml.Node) error // both fields required
```

Rules for `Cost`:
- No tokens at all (`InputTokens == 0 && OutputTokens == 0`) -> `{USD: 0, Known: true}` (nothing was spent, so nothing is unknown).
- Model present in the table -> `Known: true`, `USD = float64(in)*p.Input/1_000_000 + float64(out)*p.Output/1_000_000` (same expression shape as today, so existing sonnet numbers are bit-identical).
- Model missing from the table (including model `""`) with tokens -> `{USD: 0, Known: false}`. Never fall back to another model's price.
- A nil/empty table prices nothing (every call with tokens is unknown).

`Price.UnmarshalYAML`: decode into a struct of `*float64`; error if either field is missing (`pricing entry needs input_usd_per_mtok and output_usd_per_mtok`); reject negative, NaN and Inf. `Validate` rejects empty keys and keys with leading/trailing whitespace (error names the key).

Unknown representation on the shared call record (`record.go`):

```go
CostUnknown bool `json:"cost_unknown,omitempty"`   // CostUSD is 0 because the model has no configured price
```

Decision: keep `CostUSD float64` (type unchanged) and add an `omitempty` flag instead of making `CostUSD` a pointer/null. Reason: changing the type would break every existing reader and the shared-format agreement from #294, and `record_test.go` pins the 11-field shape of a full record; an `omitempty` bool leaves that shape untouched for known costs, and a pre-change file (flag absent) reads as "known", which is exactly what it was. When `CostUnknown` is true, `CostUSD` is 0 and must not be summed as if it were a real price; docs say so.

Stub backend change (needed so the self-test discriminates): the stub judge (`Invoke` default judge, `stubYesNoJudge`, `stubRouteJudge`) reports `Model: req.Model` when the request is pinned, else `NameStub` (`"stub"`). This mirrors kiro-cli (which echoes the pinned model). Without it every judged self-test case would carry an unpriced `stub` judge call and every run would be "incomplete", making the AC 4 signal meaningless. Stub *agent* behaviour is unchanged (`turn.Model`, possibly empty).

### 2. Pricing configuration in `internal/config`

- New top-level `pricing:` key (not under `evals:`) so production code and eval share one block:

```yaml
pricing:
  claude-sonnet-5.5:
    input_usd_per_mtok: 3
    output_usd_per_mtok: 15
```

- `Config.Pricing inference.PriceTable \`yaml:"pricing"\``; `DefaultPricing() inference.PriceTable` returns a freshly allocated table with the five Sonnet ids in `DefaultEvalsConfig().AllowedModels` (`claude-sonnet-5.5`, `claude-sonnet-5`, `claude-sonnet-4.6`, `claude-sonnet-4.5`, `claude-sonnet-4`) at 3 / 15. `claude-haiku-4.5` is deliberately absent: it is allowed to run but has no price.
- Semantics: key absent -> defaults; key present -> it **replaces** the defaults entirely (so an operator can remove a price and get "unknown"). Implementation: do not pre-populate `Config.Pricing` before `yaml.Unmarshal` (yaml would merge into an existing map); after unmarshal, `nil` -> `DefaultPricing()`; then `Validate()`. Validation errors are returned from `Load()` as `invalid pricing config: ...`.
- `LoadPricing() (inference.PriceTable, error)` mirrors `LoadEvals`: reads only the `pricing` key of `./.kairon/config.yaml`; missing file -> defaults; read/parse/validate error -> returned.

### 3. Wiring in `internal/eval` (uses, does not contain, the pricing)

- `runPins` gains `Prices inference.PriceTable`, filled by `pinRun` from `config.LoadPricing()` (error -> `❌ cannot load pricing: ...`, refuse run before any case starts). Add nil-safe `(*runPins).prices()` returning nil for an unpinned run.
- `types.go`:
  - `costFromUsage(model string, u inference.Usage) CostInfo` stays as the single eval-side adapter but delegates to `cfg.pins.prices().Cost(model, u)`; the hard-coded $3/$15 is deleted. When the cost is unknown: `EstimatedUSD = 0`, `CostIncomplete = true`, `UnpricedModels = [model]` (omitted if model is `""`).
  - `CostInfo` gains `CostIncomplete bool \`json:"cost_incomplete,omitempty"\`` and `UnpricedModels []string \`json:"unpriced_models,omitempty"\`` (sorted, de-duplicated).
  - `CostInfo.Add` ORs `CostIncomplete` and unions `UnpricedModels`. New unexported `addTotals(other)` (tokens, USD, incomplete flag, unpriced union; does not touch Model/UsageSource) replaces the hand-rolled sums in `agentCostTotals`, `setAgentVerdict` (keep sorted-agent order for deterministic floats) and `calculateAgentJudgeCosts`.
- Model used for pricing and recording: one helper `servedModel(req, resp)` = `resp.Model`, else `req.Model`. Use it for the price lookup, for `CostInfo.Model`, and in `newCallRecord`, so a record can never say "model X" while the cost was looked up under "". All four call sites (`completeAgentCall`, `scoreLLMJudge` x2, `judge_check.go`, `simulated_user.go`) pass it.
- `newCallRecord` takes the `CostInfo`; it sets `CostUSD = cost.EstimatedUSD` and `CostUnknown = cost.CostIncomplete` (a single call's CostInfo is incomplete iff that call's cost is unknown).
- Failed calls keep today's behaviour (the returned `CostInfo` is zero and the tokens appear only in `calls[]`); this issue does not change that. Documented in the docs update.
- Human-visible markers:
  - `printAgentVerdict` appends ` [cost incomplete: no price for <models>]` after the existing text when `v.Cost.CostIncomplete` (line still starts with `PASS `/`FAIL `).
  - `eval diff`: if either run's `total_cost` is incomplete, the `Cost:` delta line is suffixed ` (incomplete: <side(s)>)` and the quality/$ lines are suffixed ` (cost incomplete)`.
- Legacy compatibility: no field is renamed or retyped; `diff.go` loaders are untouched apart from the aggregation helper, so `go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2` keeps working (AC 5), and a unit test pins this with a legacy JSON literal.

### 4. Self-test cases (`internal/eval/testdata/evals/cases/selftest/`)

Add two stub cases next to `stub-usage.yaml` (same shape, agent `selftest`):
- `stub-price-sonnet.yaml`: turn `model: claude-sonnet-5.5`, usage 1000 in / 500 out. Expected agent call `cost_usd` and `agent_cost.estimated_usd` = 1000*3/1e6 + 500*15/1e6 = **0.0105**, `cost_unknown` absent, `agent_cost.cost_incomplete` absent.
- `stub-price-unknown.yaml`: turn `model: claude-haiku-4.5`, usage 1000 in / 500 out. Expected agent call `cost_unknown: true`, `cost_usd: 0`; `agent_cost.cost_incomplete: true` with `unpriced_models: ["claude-haiku-4.5"]`.

Existing `stub-usage` (model `stub-model`, 123/45) becomes an unknown-cost case; update its assertions (tokens/model/usage_source unchanged, cost unknown) rather than changing its model, because several tests assert `stub-model`.

Because the full `selftest` agent run now always contains unpriced cases (`stub-usage`, haiku), the AC 3/4 summary check runs the single case: `kairon eval --backend stub --evals-dir <copy> selftest stub-price-unknown` -> `summary.json` `total_cost.cost_incomplete == true`, `agent_verdicts.selftest.cost.cost_incomplete == true`; the same for `stub-price-sonnet` -> not incomplete (works because the stub judge now reports the priced pinned judge model). Adding cases changes case counts; update any test that enumerates the self-test cases.

### 5. Config files and docs

- `.kairon/config.yaml` and `cmd/kairon/templates/kairon/config.yaml`: add an **active** (uncommented) `pricing:` block with a comment explaining exact-id keys, USD per million tokens, "no entry = unknown cost", replace-not-merge semantics, and a note that `claude-haiku-4.5` is intentionally unpriced. Entries: the five Sonnet ids at 3 / 15. (These two files are not in the `task sync:check` mapping; they are kept in step by hand and by a test.)
- `docs/evaluation.md`: update "Reported vs Estimated Usage" (remove "fixed $3 / $15"; describe per-model pricing, unknown cost, `cost_incomplete`/`unpriced_models`, summary/verdict/diff markers), the `calls[]` table (`cost_usd`, new `cost_unknown`), the summary/verdict field tables (`total_cost`, `cost`), the stub judge model note (pinned model when pinned, else `stub`), the `stub.turns[].model` row, and add a "Pricing" subsection documenting the config block. Also `README.md` config table is unaffected (pricing is an eval/runtime key); no change required.

## Relevant Files

Create:
- `internal/inference/pricing.go`, `internal/inference/pricing_test.go`
- `internal/eval/testdata/evals/cases/selftest/stub-price-sonnet.yaml`
- `internal/eval/testdata/evals/cases/selftest/stub-price-unknown.yaml`
- `internal/eval/pricing_cost_test.go` (eval-side tests)

Modify:
- `internal/inference/record.go` (+ `record_test.go`), `internal/inference/stub.go` (+ stub judge tests)
- `internal/config/config.go`, `internal/config/evals.go` (or new `pricing.go`), `internal/config/*_test.go`
- `internal/eval/types.go`, `provenance.go`, `runner.go`, `judge_check.go`, `simulated_user.go`, `verdict.go`, `diff.go`
- Existing eval tests that assume the fixed price or `stub`/`served-model` costs: `config_test.go` (lines ~267-327), `pinning_test.go` (~350), `judge_check_test.go` (~143-167), `selftest_test.go`, `container_agent_test.go`, `verdict_test.go`, `repeat_exec_test.go`, `exit_status_test.go`, `architecture_compatibility_test.go`
- `.kairon/config.yaml`, `cmd/kairon/templates/kairon/config.yaml`, `docs/evaluation.md`

Do NOT edit (AGENTS.md historical artifacts): `.kairon/specs/` other than this file, `.kairon/evals/results/`, `CHANGELOG.md`, `.kairon/validation-results.md`.

## Team Orchestration

- `inference-pricing` first (pure package, no dependencies).
- `config-pricing` depends on it (imports `inference.PriceTable`).
- `eval-cost-wiring` depends on both and is the large integration task.
- `config-files-docs` depends only on `config-pricing` and can run in parallel with `eval-cost-wiring`; it owns the two YAML files plus the test that parses them.
- `selftest-cases` depends on `eval-cost-wiring`.
- `validate-all` (validator) last.
- Template sync: the changed template file (`cmd/kairon/templates/kairon/config.yaml`) is not in the `sync:check` mapping, but `task sync:check` must still pass; no synchronized live files change. The new selftest cases live under `internal/eval/testdata/` (not `.kairon/evals/cases`), so no template copy is needed.

## Step-by-Step Task Breakdown (TDD per AGENTS.md: failing test first, observed for the right reason, then minimal code, test + code in one commit)

1. **inference-pricing** - tests first: `PriceTable.Cost` known/unknown/zero-token/nil-table/empty-model; exact-id (no case folding); `UnmarshalYAML` missing field, negative, NaN; `Validate` bad keys; `CallRecord` `cost_unknown` omitted when false, round-trips, old JSON (no flag) decodes as known; stub judge reports the pinned model when `Request.Model` is set, `stub` otherwise (default, yes/no, route paths). Then implement `pricing.go`, `CostUnknown`, stub change.
2. **config-pricing** - tests first: `DefaultPricing` contents (five Sonnet ids at 3/15, no haiku, fresh allocation); `Load` with no `pricing` key -> defaults; with key -> replaces defaults; invalid entries rejected with the model named; `LoadPricing` missing file -> defaults, bad YAML -> error. Then implement.
3. **eval-cost-wiring** - tests first (in `pricing_cost_test.go`): `costFromUsage` priced vs unpriced under a pinned table; `CostInfo.Add`/`addTotals` propagate incomplete + unpriced union; `newCallRecord` sets `CostUnknown`; `agentVerdict`/`setAgentVerdict`/`buildSummary` totals incomplete when any case is; `printAgentVerdict` suffix; `eval diff` suffixes; legacy summary/agent JSON literal without new fields still loads and diffs; `servedModel` fallback keeps record model and cost model identical. Then implement and fix the existing tests listed above (use a priced model or set `cfg.pins.Prices` where a test needs a non-zero cost).
4. **config-files-docs** - test first (`internal/config`): parse `../../.kairon/config.yaml` and `../../cmd/kairon/templates/kairon/config.yaml` with `LoadPricing`-style decoding and assert every Sonnet id in `DefaultEvalsConfig().AllowedModels` is present at exactly 3 / 15 (covers AC 1). Then edit both YAML files and `docs/evaluation.md`.
5. **selftest-cases** - tests first (in `internal/eval`, copying fixtures with `copyFixturesTo`): AC 2 (`stub-price-sonnet` -> 0.0105 on the agent call and `agent_cost`), AC 3 (`stub-price-unknown` -> `cost_unknown`, `cost_usd` 0, `agent_cost.cost_incomplete`, `unpriced_models`), AC 4 (single-case run summary incomplete; sonnet single-case run not incomplete), plus the container/parity tests that enumerate selftest cases. Then add the two YAML cases, update case-count/`stub-usage` assertions. Run `task eval:selftest`.
6. **validate-all** - run the validation commands below; confirm AC 1-5 and the constraint (eval uses `inference.PriceTable`; no price constants remain in `internal/eval`).

## Validation Commands

```bash
go build ./...
go vet ./...
go test ./internal/inference/... ./internal/config/... ./internal/eval/... -count=1
task test
task lint
task sync:check
task eval:selftest
go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2
# Constraint: no hard-coded price left in the eval package
! grep -nE '3\.0 */ *1_000_000|15\.0 */ *1_000_000' internal/eval/*.go
grep -n "pricing:" .kairon/config.yaml cmd/kairon/templates/kairon/config.yaml
```

## Acceptance Criteria Traceability

| AC | Covered by |
|----|-----------|
| 1 pricing block in both config files, Sonnet @ 3/15 | task `config-files-docs` (+ test parsing both files); `Config.Pricing`/`LoadPricing` |
| 2 cost = tokens x model price | `PriceTable.Cost`, `costFromUsage` delegation, case `stub-price-sonnet` (0.0105) |
| 3 unknown model -> unknown cost | `Cost.Known == false` -> `cost_unknown`, case `stub-price-unknown` |
| 4 totals state incompleteness | `CostInfo.CostIncomplete`/`unpriced_models` on case, agent verdict, `summary.total_cost`; verdict-line and diff suffixes |
| 5 legacy results load | additive `omitempty` fields only; legacy-literal unit test; `eval diff` command |
| Constraint | pricing/cost logic in `internal/inference/pricing.go`; eval only delegates |

## Risks and Notes

- Existing users whose `config.yaml` has no `pricing:` key get the built-in Sonnet defaults (same numbers as before for Sonnet); any other model, including haiku, now shows as unknown rather than silently priced as Sonnet. That is the intended behaviour change.
- An unpinned/unknown model never gets a price; `cost_usd: 0` with `cost_unknown: true` must not be summed by external readers as a real zero. Documented.
- Stub judge model change touches stub tests that asserted `stub` for pinned requests; update them to the pinned model.
- Out of scope (unchanged): kiro-cli usage estimation, a direct-API backend, failed-call judge cost accounting.

## Machine-Readable Execution Plan

```kiro-plan
version: "1.0"
tasks:
  - id: "inference-pricing"
    agent: "builder"
    description: "Add internal/inference/pricing.go (Price, PriceTable, Cost, Validate, Price.UnmarshalYAML), add CallRecord.CostUnknown (cost_unknown, omitempty), and make the stub judge report the pinned request model (else 'stub'). Write failing tests first and observe them fail for the expected reason."
    dependencies: []
    acceptance_criteria:
      - "PriceTable.Cost returns Known=true with USD = in*inputPrice/1e6 + out*outputPrice/1e6 for a listed model, using exact-id lookup only"
      - "PriceTable.Cost returns Known=false, USD=0 for a model with no entry (including empty model) when tokens > 0, and Known=true USD=0 when both token counts are 0"
      - "Price YAML decoding requires both input_usd_per_mtok and output_usd_per_mtok and rejects negative, NaN and Inf; Validate rejects empty and whitespace-padded keys"
      - "CallRecord has CostUnknown with json cost_unknown,omitempty; a full known record still marshals to the existing 11 fields; JSON without the flag decodes as known"
      - "Stub judge responses (default, yes/no, route) report Request.Model when set, else 'stub'; stub agent behaviour unchanged"
      - "internal/inference does not import internal/eval or internal/config"
    validation_commands:
      - "go build ./internal/inference/..."
      - "go test ./internal/inference/... -count=1"
      - "go vet ./internal/inference/..."

  - id: "config-pricing"
    agent: "builder"
    description: "Add the top-level pricing block to internal/config: Config.Pricing, DefaultPricing() (five Sonnet ids at 3/15, no haiku), LoadPricing(), validation in Load(). Absent key -> defaults; present key replaces defaults. Tests first."
    dependencies: ["inference-pricing"]
    acceptance_criteria:
      - "DefaultPricing returns a fresh table with claude-sonnet-5.5, claude-sonnet-5, claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4 at input 3 and output 15, and no claude-haiku-4.5 entry"
      - "Load() and LoadPricing() return defaults when the pricing key is absent (and LoadPricing when the file is missing), and exactly the configured entries (no merge with defaults) when the key is present"
      - "An entry with a missing field, a negative price or a bad key is rejected with an error that names the model id"
      - "LoadPricing reads only the pricing key and does not require repo"
    validation_commands:
      - "go build ./internal/config/..."
      - "go test ./internal/config/... -count=1"
      - "go vet ./internal/config/..."

  - id: "eval-cost-wiring"
    agent: "builder"
    description: "Replace the hard-coded $3/$15 in internal/eval with the inference pricing: runPins.Prices loaded by pinRun via config.LoadPricing, costFromUsage delegating to PriceTable.Cost, servedModel helper (resp.Model else req.Model) used for pricing, CostInfo.Model and call records, CostInfo.CostIncomplete/UnpricedModels with Add/addTotals, CallRecord.CostUnknown set in newCallRecord, incomplete propagation through agentCostTotals/setAgentVerdict/buildSummary/calculateAgentJudgeCosts, verdict-line and eval diff markers. Tests first; then update the existing tests that assumed the fixed price."
    dependencies: ["inference-pricing", "config-pricing"]
    acceptance_criteria:
      - "No price constant or pricing table remains in internal/eval; costs come from inference.PriceTable.Cost"
      - "A priced call has cost tokens x price and no incomplete marker; an unpriced call has EstimatedUSD 0, CostIncomplete true, UnpricedModels containing the model, and its call record has cost_unknown true and cost_usd 0"
      - "CaseResult agent_cost/judge_cost, AgentVerdict.Cost and Summary.TotalCost are marked cost_incomplete (with unpriced_models) when any contributing call had an unknown cost, and are unmarked otherwise"
      - "printAgentVerdict appends '[cost incomplete: no price for <models>]' only when incomplete and the line still starts with PASS/FAIL; eval diff annotates the Cost line and quality/$ lines when either run is incomplete"
      - "A call record's model and the model its cost was priced under are always the same value"
      - "Result and summary JSON written before this change (no new fields) still unmarshal and 'eval diff' on them works; pinned by a unit test with a legacy literal"
      - "pinRun refuses the run before any case starts when pricing config is invalid"
      - "All previously passing eval tests pass (updated where they assumed the fixed price or the 'stub' judge model)"
    validation_commands:
      - "go build ./..."
      - "go vet ./internal/eval/..."
      - "go test ./internal/eval/... -count=1"
      - "go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2"
      - "! grep -nE '3\\.0 */ *1_000_000|15\\.0 */ *1_000_000' internal/eval/*.go"

  - id: "config-files-docs"
    agent: "builder"
    description: "Add an active, documented pricing block (five Sonnet ids at 3/15, comments covering exact-id keys, USD per million tokens, unpriced = unknown cost, replace-not-merge, haiku intentionally unpriced) to .kairon/config.yaml and cmd/kairon/templates/kairon/config.yaml, with a test in internal/config that parses both files; update docs/evaluation.md for per-model pricing, unknown cost, cost_incomplete/unpriced_models, cost_unknown and the stub judge model."
    dependencies: ["config-pricing"]
    acceptance_criteria:
      - "A test parses both config files and asserts every Sonnet model in DefaultEvalsConfig().AllowedModels has input_usd_per_mtok 3 and output_usd_per_mtok 15, and that both files remain loadable"
      - "Both config files visibly contain a pricing: block with explanatory comments"
      - "docs/evaluation.md no longer states a fixed $3 / $15 estimate; it documents the pricing block, unknown cost semantics (cost_unknown, cost_incomplete, unpriced_models), the verdict-line and eval diff markers, and that failed judge calls remain outside judge_cost"
      - "Historical artifacts (.kairon/specs other than this issue, results, CHANGELOG.md) are not modified"
    validation_commands:
      - "go test ./internal/config/... -count=1"
      - "grep -n 'pricing:' .kairon/config.yaml cmd/kairon/templates/kairon/config.yaml"
      - "! grep -n 'fixed \\$3' docs/evaluation.md"
      - "task sync:check"

  - id: "selftest-cases"
    agent: "builder"
    description: "Add self-test cases stub-price-sonnet (claude-sonnet-5.5, 1000/500 tokens -> 0.0105) and stub-price-unknown (claude-haiku-4.5, 1000/500 tokens -> unknown cost) under internal/eval/testdata/evals/cases/selftest/, with tests (written first) covering AC 2, 3 and 4 via single-case stub runs, and update tests that enumerate self-test cases or assert stub-usage cost."
    dependencies: ["eval-cost-wiring"]
    acceptance_criteria:
      - "Running selftest stub-price-sonnet records the agent call cost_usd 0.0105 and agent_cost.estimated_usd 0.0105 with no cost_unknown and no cost_incomplete, and summary total_cost is not incomplete"
      - "Running selftest stub-price-unknown records the agent call with cost_unknown true and cost_usd 0, agent_cost.cost_incomplete true with unpriced_models [claude-haiku-4.5], and summary.json total_cost.cost_incomplete true"
      - "task eval:selftest passes end to end"
      - "Container/parity tests that enumerate selftest cases still pass or skip cleanly when no container daemon is available"
    validation_commands:
      - "go test ./internal/eval/... -count=1"
      - "task eval:selftest"
      - "go test ./internal/eval -run 'Selftest|Pricing|Cost' -count=1 -v"

  - id: "validate-all"
    agent: "validator"
    description: "Read-only verification of all five acceptance criteria and the pricing-outside-eval constraint, plus build, vet, tests, lint and template sync."
    dependencies: ["eval-cost-wiring", "config-files-docs", "selftest-cases"]
    acceptance_criteria:
      - "AC 1: both config files contain a pricing block with the five Sonnet ids at 3 input / 15 output"
      - "AC 2: self-test case with model claude-sonnet-5.5 records configured price times tokens"
      - "AC 3: self-test case with model claude-haiku-4.5 records an unknown cost"
      - "AC 4: summary of the haiku run marks total cost incomplete; case, agent verdict and run totals all carry the marker"
      - "AC 5: eval diff 260620-200919-e369501 260621-160207-8a19eb2 succeeds"
      - "Constraint: internal/eval contains no price constants and uses inference.PriceTable; internal/inference does not import internal/eval"
      - "task test, task lint and task sync:check pass"
    validation_commands:
      - "go build ./..."
      - "go vet ./..."
      - "task test"
      - "task lint"
      - "task sync:check"
      - "task eval:selftest"
      - "go run ./cmd/kairon eval diff 260620-200919-e369501 260621-160207-8a19eb2"
      - "grep -n 'pricing:' .kairon/config.yaml cmd/kairon/templates/kairon/config.yaml"
      - "! grep -nE '3\\.0 */ *1_000_000|15\\.0 */ *1_000_000' internal/eval/*.go"
      - "! grep -rn 'internal/eval' internal/inference/*.go | grep -v _test | grep -v '//'"
```
