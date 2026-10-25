# Analysis: Go LLM Frameworks for a Direct-API Inference Backend

Status: research / decision input. Not a design spec.
Audience: Kairon maintainers evaluating how to call LLM APIs directly instead of shelling out to a CLI.

## 1. Why this document exists

Kairon today runs every model turn by shelling out to the `kiro-cli` binary. That binary, not Kairon,
owns the whole agent loop: the tool-call cycle, skills, sub-agents, conversation continuity (`--resume`),
and tool-trust gating. Kairon's seam is deliberately narrow, in `internal/inference`:

```go
type Backend interface {
    Name() string
    Available() error
    StartupProbe() time.Duration
    Invoke(ctx context.Context, req Request) (Response, error)
}
```

`Invoke` is "one prompt in, text out" (`Request.Prompt` -> `Response.Text`), and the doc comment on
`Request.Turn` already anticipates "a future direct-API backend" that keeps its own message list keyed by
the conversation. Two backends ship: `kiro-cli` (process) and `stub` (deterministic, in-process).

The goal of this research is to pick what fills a third, `direct` backend so Kairon can call the major LLM
APIs itself and stop depending on a CLI that may carry license restrictions on using it to drive inference.

### The load-bearing fact

Because `kiro-cli` owns the agent loop, a direct-API backend cannot be "just an HTTP client". To preserve
Kairon's current behaviour it must itself own, inside the Go process:

1. Multi-provider model calls (Claude first, since Kairon's agents and judges are Claude-class; OpenAI,
   Gemini and Bedrock desirable for provider neutrality and license flexibility).
2. The tool-call loop (model requests a tool, host runs it, result is fed back, repeat until a final answer).
3. Tool/skill exposure. Kairon's agents use skills (`.kiro/skills/*/SKILL.md`) and tools; a faithful
   replacement must surface those. MCP is the natural wire for this, and it matters that Kairon already
   speaks ACP/MCP-adjacent protocols (`internal/acp`).
4. Conversation continuity across turns (the `--resume` equivalent) keyed by `Request.WorkDir`/conversation.
5. Tool-trust gating equivalent to `--trust-all-tools` vs `--trust-tools=<csv>` (`Request.ToolTrust`), so the
   eval sandbox's containment guarantees still hold.
6. Token usage that is *reported* (not estimated). A direct SDK is actually an upgrade here: `kiro-cli`
   reports none, so Kairon estimates at 4 chars/token. Real APIs return exact input/output token counts,
   which feeds `CostInfo` and pricing directly.

Items 2 to 5 are exactly what separates a *framework* from a *provider SDK*. That split drives the whole
analysis below, so each option is scored on how much of the list it hands you versus how much you build.

### Evaluation criteria

- **Maintenance**: is it actively maintained, and by whom? (An unmaintained dependency in the model path is
  a production liability, per Kairon's own "don't leave broken/flaky things" culture.)
- **Provider coverage**: Claude (required), plus OpenAI / Gemini / Bedrock / Ollama (provider neutrality and
  license flexibility are the entire point of the exercise).
- **Agent loop ownership**: tool-call loop, multi-turn sessions, sub-agents, interrupt/resume.
- **Tool & skill model**: typed tools, and crucially MCP client support (for Kairon's skills/tools).
- **Reported token usage**: for cost accounting.
- **Fit to the existing seam**: how cleanly it sits behind `inference.Backend.Invoke` without dragging an
  opinionated runtime (HTTP servers, Dev UIs, GCP session stores) into a TUI/eval binary.
- **Dependency weight & Go version floor**: Kairon is a small static binary; see `go.mod`.

Two layers are evaluated separately because they answer different questions:

- **Layer A - Agent frameworks**: own the loop for you (Genkit Go, Eino, Google ADK Go, tRPC-Agent-Go,
  LangChainGo).
- **Layer B - Provider SDKs + MCP SDK**: you own a small loop, they own transport and schema (official
  Anthropic / OpenAI Go SDKs, the official MCP Go SDK, and thin multi-provider shims).

---

## 2. Layer A: Agent frameworks

### 2.1 Genkit Go (`github.com/genkit-ai/genkit/go`)

Official, first-party Google SDK; reached 1.0 in 2025, roughly monthly releases. Three abstraction levels in
one SDK (vanilla generation, typed flows, agents), a middleware layer (retry, fallback, tool approval,
skills), and a local Dev UI with an OpenTelemetry trace explorer.

Strengths
- First-party maintenance with a steady release cadence. Lowest "abandonment" risk of the independent-ish
  options.
- Provider-neutral in one line: Gemini (Google AI + Vertex), Anthropic, Ollama, and an OpenAI-compatible
  family (OpenAI, xAI, DeepSeek, OpenRouter, Kimi, DashScope, Z.ai). Anthropic being first-class matters
  for Kairon.
- The tool loop runs inside `Generate`; tools are typed (schema inferred from Go structs). MCP client and
  server plugins exist, which is the clean path to exposing Kairon's skills/tools.
- Reports real token usage and traces every call. This directly upgrades Kairon's estimated-usage story.
- `tool approval` middleware maps conceptually onto Kairon's `ToolTrust` gate.

Weaknesses
- The **agents** package (sessions, multi-agent delegation, the `Skills` middleware) is still marked
  experimental and gated behind `genkit.WithExperimental()`. Multi-turn sessions and sub-agents, which
  Kairon needs to match `kiro-cli`, are exactly the part that is least settled.
- Opinionated runtime surface (flows as `net/http` handlers, Dev UI, dotprompt). Kairon only needs the
  model-call + tool-loop core; the rest is weight to ignore.
- Go 1.25+ floor. Check against Kairon's `go.mod` before committing.
- Fewer vector stores than LangChainGo (irrelevant to Kairon today; no RAG requirement).

Fit to Kairon: **High for the model-call core, medium for the full agent loop** (because the session/subagent
layer is experimental). Good candidate if the direct backend starts with single-turn + tool-loop and adopts
sessions as that package stabilizes.

### 2.2 Eino (`github.com/cloudwego/eino`)

ByteDance / CloudWeGo. The most-starred independent Go LLM framework (~13k) and the most active of the
independents; frequent 0.x releases. Split into `compose` (Chain/Graph/Workflow orchestration with automatic
streaming) and `adk` (agents, runners, events). Components live in a separate `eino-ext` repo.

Strengths
- Typed graph orchestration with automatic stream handling; every node gets `Invoke` and `Stream` for free.
  Overkill for `Invoke`-one-prompt, but a strong base if Kairon ever models its krew-lead/architect/builder
  pipeline as a graph.
- ADK-style agents plus a roadmap explicitly centered on agents: DeepAgent (planning + sub-agents),
  interrupt/resume for human-in-the-loop, skills. Interrupt/resume is a direct analogue to what Kairon's
  gated pipeline wants.
- Broad providers in `eino-ext`: OpenAI, **Claude**, Gemini, DeepSeek, Qwen, Ollama, OpenRouter, Ark,
  Qianfan. MCP is a first-class tool component.
- Runs on Go 1.18+, the lowest floor in Layer A: least likely to force a toolchain bump.
- Callback aspects (`OnStart/OnEnd/OnError` + streaming variants) give a clean instrumentation seam for
  Kairon's structured logging.

Weaknesses
- Still 0.x with frequent alphas; expect API churn. For a core model path this is the main risk.
- Verbose: config structs and explicit error checks everywhere. More ceremony than the narrow `Invoke` seam.
- Depends on ByteDance's `sonic` JSON library, which prints a startup warning and falls back to
  `encoding/json` on the newest Go toolchains. Noisy but harmless; still an extra transitive dependency in a
  small binary.
- ByteDance-flavoured corners (Ark, VikingDB, CozeLoop) and a Chinese-first documentation history.
- You create the provider client yourself and hand it to the component (not a drawback per se, but it means
  Eino does not abstract provider auth the way Genkit does).

Fit to Kairon: **Medium.** Best if orchestration (the multi-agent pipeline) becomes the hard problem. For a
plain direct-API backend it is more machinery than the seam needs, and the 0.x churn is a real cost on the
model path.

### 2.3 Google ADK Go (`google.golang.org/adk/v2`)

Official Google agent framework, 1.0 in 2026, highest commit velocity of the five. Agent-only: no vanilla
generation and no flow primitive; you always build an agent, give it to a runner, and iterate events (via a
Go 1.23 `iter.Seq2` loop, the most idiomatic event loop of the group).

Strengths
- First-party Google support and the highest commit velocity; lowest abandonment risk alongside Genkit.
- First-class agent machinery Kairon would otherwise build by hand: sessions, memory, artifacts, sequential
  / parallel / loop workflow agents, A2A, MCP toolsets, tool confirmation, embedded web UI.
- Idiomatic Go: generic function tools, iterator event loops.
- Same mental model as ADK Python/Java/TS (useful if the team is cross-language).

Weaknesses
- **Gemini-first.** Other providers only through an OpenAI-compatible endpoint (`openaimodel`). Claude is not
  a first-class provider; you would route Claude through an OpenAI-compatible shim or Bedrock's
  OpenAI-compatible surface. For a Claude-centric project this is the decisive drawback.
- **Strong pull toward Vertex AI / GCP**: the persistent session and memory services, evaluation tooling and
  telemetry defaults assume Google Cloud. Kairon is a local TUI + eval harness; that gravity is wrong.
- Go 1.26.6+ floor, the highest of the five.
- Agent-only shape means even a one-shot judge call must be wrapped in an agent + runner, which fights the
  single-shot `Invoke` seam and the deterministic judge path.
- Config structs get long; no vector-store components.

Fit to Kairon: **Low.** The Gemini/GCP gravity and the lack of first-class Claude are disqualifying for the
stated goal (Claude-class agents and judges, local binary, provider neutrality).

### 2.4 tRPC-Agent-Go (`trpc.group/trpc-go/trpc-agent-go`)

Tencent. ADK-shaped (`llmagent`, `runner`, `chainagent`, `parallelagent`, `cycleagent`, sessions, memory),
1.x since late 2025, smaller community (~1.8k stars) but actively committed. Its standout is **GraphAgent**, a
faithful LangGraph-in-Go (state schemas, conditional edges, checkpoints, interrupts, time travel) that plugs
into the same runner and telemetry as the LLM agents.

Strengths
- Broadest first-class provider set of the agent frameworks for Kairon's needs: OpenAI-compatible,
  **Anthropic**, Gemini, **Bedrock**, Ollama, Hugging Face, Hunyuan. Claude and Bedrock both first-class,
  which no other Layer-A option offers together.
- ADK mental model without the GCP gravity: sessions, memory, workflow agents, all runnable locally.
- Ships the pieces Kairon's gated pipeline wants: **Agent Skills with sandboxed execution** (close to
  Kairon's skills + sandbox story), A2A, MCP, an evaluation package, **model failover and hedging**, prompt
  caching, and OpenTelemetry traces/metrics.
- GraphAgent is the most complete workflow engine of the five, with checkpoints and interrupts for
  human-in-the-loop, which maps onto Kairon's approval gates.
- Go 1.21+ floor (moderate).

Weaknesses
- Smallest community of the five; more key-person / single-vendor risk than Genkit or ADK.
- No Dev UI; observability is OpenTelemetry + Langfuse.
- Large option surface (`llmagent` alone has dozens of options) and OpenAI-shaped event structs that are
  verbose to consume (`Choices[0].Message` / `Delta`).
- Tencent-flavoured corners (Hunyuan, tRPC integration) and partly Chinese-first docs.

Fit to Kairon: **High on capability, medium on risk.** It is the only Layer-A framework that is simultaneously
Claude-first-class, Bedrock-first-class, local (no cloud gravity), and ships sandboxed skills + eval +
failover. The caution is the smaller community for a core dependency.

### 2.5 LangChainGo (`github.com/tmc/langchaingo`)

The original (2023). Largest catalog on paper (17 providers, 15 vector stores) and LangChain-familiar
vocabulary. **Effectively unmaintained**: last release Oct 2025, last commit to main Jan 2026, still 0.1.x
after three and a half years, with a large backlog of unmerged issues/PRs.

Strengths
- Familiar names for anyone from LangChain Python; many tutorials exist.
- Largest provider/vector-store catalog by count (OpenAI, Anthropic, Google AI, Vertex, Bedrock, Cohere,
  Mistral, Ollama, ...).

Weaknesses
- **Unmaintained**, and the rot is concrete: default Gemini model retired, the Gemini provider sits on a
  dead `google/generative-ai-go` SDK, streaming broken on Gemini. Inheriting this on the model path is the
  exact production liability Kairon's engineering culture says to avoid.
- No middleware, retry, fallback, or OpenTelemetry.
- Tools are untyped `Call(ctx, string) (string, error)`: no schema, no typed arguments. The agent is a
  prompt-driven ReAct loop, weaker than native tool-calling.
- Every vector store needs an external service.

Fit to Kairon: **Do not adopt for new work.** Listed only because it is the name people reach for first and
because it may appear in reference code. If anything here uses it, pin models and plan removal.

---

## 3. Layer B: Provider SDKs + the MCP SDK (the "own a small loop" path)

This layer matters because Kairon's seam is narrow and the agent loop it needs is specific (tool-trust
gating, conversation continuity keyed by `WorkDir`, deterministic judge path). Owning a ~200-line tool-call
loop over first-party SDKs can be *less* total complexity than bending an opinionated agent framework to fit,
and it removes the framework-churn risk from the core model path.

### 3.1 Official Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`)

First-party, actively maintained. Wraps the Messages API with typed request/response structs, retries and
streaming, token counting and batch management, and **native tool use** (including server-side tools: web
search, web fetch, bash, text editor) plus thinking-mode and beta capabilities. Lets you point at any
compatible base URL (so Bedrock / Vertex Anthropic endpoints are reachable).

Strengths
- First-party and current; the lowest-risk way to call the models Kairon actually uses.
- Native tool-use blocks and **reported token usage** out of the box, feeding `CostInfo` exactly.
- Minimal, unopinionated: it is an API client, so it drops cleanly behind `inference.Backend.Invoke` with no
  runtime to ignore.
- Base-URL override means one SDK can reach Anthropic direct, Bedrock, and Vertex.

Weaknesses
- Single-provider. OpenAI / Gemini / Ollama need their own SDKs or a shim (see 3.4).
- **You write the tool-call loop, sub-agent handling, and conversation-continuity store yourself.** That is
  the work the Layer-A frameworks do for you. For Kairon this is bounded and testable, but it is real code.
- No built-in MCP wiring; pair with the MCP Go SDK (3.3).

### 3.2 Official OpenAI Go SDK (`github.com/openai/openai-go`)

First-party, actively maintained, typed, with function-calling / tool support and streaming. Relevant because
many models (including non-OpenAI ones via OpenAI-compatible gateways, Bedrock's OpenAI-compatible surface,
OpenRouter, local Ollama) speak the OpenAI wire format, so one OpenAI-shaped client reaches a large share of
providers.

Strengths
- First-party and current; typed function-calling; reported usage.
- The OpenAI wire format is the lingua franca, so this plus base-URL override covers OpenAI-compatible
  providers without extra SDKs.

Weaknesses
- Claude via an OpenAI-compatible shim is second-class versus the native Anthropic SDK (tool-use and
  thinking-mode fidelity differ). For a Claude-first project, prefer 3.1 for Claude and keep this for the
  OpenAI-compatible family.
- Same "you own the loop" caveat as 3.1.

### 3.3 Official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)

Official, maintained in collaboration with Google. Server and client primitives (`mcp.NewClient`,
`mcp.NewServer`, `mcp.AddTool`) over stdio / command / in-memory / streamable-HTTP transports, with protocol
version negotiation and backward compatibility.

Why it matters for Kairon specifically
- Kairon's skills/tools and sub-agent story is the hard part of replacing `kiro-cli`. MCP is the standard
  wire for exposing tools to a model-driven loop, and Kairon already has protocol machinery (`internal/acp`).
- With the MCP client you connect to the same MCP tool servers an agent would use, enumerate their tools,
  advertise them to the model (via 3.1 / 3.2 tool schemas), and run the tool-call loop yourself, applying
  `Request.ToolTrust` at the point where you decide whether to execute a tool. That is the cleanest place to
  reimplement the `--trust-all-tools` / `--trust-tools=<csv>` gate with a *reported*, inspectable decision
  (unlike kiro-cli, where denials are "not reliably detectable", per the code comment).

Weakness
- It is plumbing, not an agent loop: it gives you tool transport and schema, not the model call or the loop.
  It is a companion to 3.1 / 3.2, not a substitute for a framework.

### 3.4 Multi-provider shims (Jetify `ai`, `goai`, OmniLLM, LLMHub, hoangvvo `llm-sdk`)

A cluster of libraries offering "one interface, many providers", several explicitly modelled on the Vercel AI
SDK (Jetify `ai`, `goai`). Some advertise MCP support and stdlib-only footprints.

Strengths
- One `LanguageModel`-style interface across OpenAI, Anthropic, Gemini, Ollama, etc.; a single swap point
  that matches the spirit of `inference.Backend`.
- The Vercel-AI-shaped ones tend to be light and generics-first, closer to Kairon's taste than a full
  framework.

Weaknesses
- **Maintenance and longevity vary widely**; most are single-maintainer community projects without the
  first-party guarantee of 3.1/3.2 or the velocity of Genkit/ADK. On the core model path, that is the same
  class of risk that sank LangChainGo.
- Abstraction leaks: provider-specific features (Claude thinking mode, server-side tools, prompt caching)
  are often lowest-common-denominator'd away, which undercuts the reason to call Claude directly.
- Still "you own the agent loop" unless the shim also ships an agent layer (a few do, immaturely).

Fit: usable as a thin convenience over the official SDKs, but **do not put an unvetted single-maintainer shim
on the core model path**. If multi-provider ergonomics are wanted, prefer Genkit's provider plugins (Layer A,
first-party) or write a tiny internal adapter over 3.1 + 3.2.

---

## 3bis. Layer C: Non-Go frameworks integrated across a process boundary

A framework does not have to be written in Go to be usable by Kairon. `kiro-cli` is itself a non-Go process
that Kairon drives over stdin/stdout, and `internal/inference/exec.go` already defines a JSON wire envelope
(`ServeExec` / `DecodeExecResult`) for running a backend out of process. So "integrate a non-Go framework"
means exactly what Kairon does today: a `direct`-style backend shells out to (or long-runs and talks JSON-RPC
to) an external harness, and maps its result onto `Response`. The integration seam already exists; the
question per candidate is whether it offers a clean, scriptable, non-interactive surface (print/JSON or
RPC), a license that permits using it to drive inference, and reported usage.

Two integration shapes are available and both fit the current seam:
- **One-shot subprocess** (what `kiro-cli` uses): spawn per turn, prompt on stdin, parse stdout. Simplest;
  conversation continuity needs a `--resume`-style flag or a session id the harness persists.
- **Long-running RPC child**: start the harness once, speak JSON-RPC over a pipe/socket for many turns. Lower
  per-turn latency, natural multi-turn sessions, and a tool-call channel the host can gate. This matches
  Kairon's `internal/acp` client, which already drives an agent over JSON-RPC.

The cost that is common to every Layer-C option: a **runtime dependency outside the Go toolchain** (Node.js
or Python) must be present wherever Kairon runs, including the eval sandbox image and CI. That is the same
class of dependency `kiro-cli` already imposes, so it is not new in kind, but it is weight the Go-native and
SDK paths avoid, and it complicates the static-binary story.

### 3bis.1 Pi (`@earendil-works/pi`, the pi-mono toolkit) - the strongest non-Go fit

Pi (by Mario Zechner / badlogic, now under the earendil-works namespace) is a TypeScript/Node monorepo, **MIT
licensed**, structured as a toolkit rather than a monolith. The relevant packages:
- `@earendil-works/pi-ai`: unified multi-provider LLM API, ~31 provider IDs including **OpenAI, Anthropic,
  Google, Mistral, Bedrock, Fireworks**, with automatic model discovery, **token/cost tracking**, and
  cross-provider handoffs mid-session.
- `@earendil-works/pi-agent-core`: a stateful agent runtime with **tool execution, event streaming, a
  transport abstraction, and state management**. This is the agent loop Kairon would otherwise build.
- `@earendil-works/pi-coding-agent`: the coding-agent CLI, with session management plus a skills / extension
  / prompt-template system.

Why it is the best Layer-C match for Kairon specifically:
- **It runs in exactly the modes Kairon needs.** Pi ships four modes: interactive TUI, **print/JSON output**,
  **RPC for process integration**, and an **SDK for embedding**. The print/JSON mode maps onto the one-shot
  subprocess shape; the RPC mode maps onto the long-running JSON-RPC shape and onto Kairon's existing
  `internal/acp` client. Either drops behind `inference.Backend.Invoke` with the wire plumbing Kairon already
  has in `exec.go`.
- **Architecturally it is a like-for-like replacement for `kiro-cli`.** Both are minimal coding-agent
  harnesses with four core tools (read/write/edit/bash), a skills/extension system, and session management.
  The conceptual mapping is close to one-to-one: Pi skills <-> Kairon's `.kiro/skills/*/SKILL.md`, Pi tools
  <-> trust-gated tools, Pi sessions <-> `--resume`/`Request.Turn` continuity. This minimizes the behaviour
  gap the eval suite has to close.
- **Provider neutrality and the license motivation are both satisfied.** MIT license and a 31-provider
  unified API (Anthropic and Bedrock first-class) is precisely the "call LLMs directly, free of CLI license
  restrictions" goal, without Kairon owning the provider layer.
- **Reported token/cost tracking** is built in, upgrading Kairon's estimated-usage story.
- Extensible in TypeScript without forking the core, so a Kairon-specific adapter (output contract, trust
  mapping) can live as an extension rather than a patch.

Weaknesses / integration risks
- **Node.js runtime dependency** everywhere Kairon runs, including the sandbox image and CI. This is the
  biggest single cost and must be weighed against the Go-native and official-SDK paths that avoid it.
- **Young and fast-moving.** Pre-1.0 (0.74.x), high commit volume, large open issue/PR count, and a recent
  namespace/ownership move (to earendil-works). Pin a version; expect churn on the RPC/JSON contract.
- **Single-originator project.** Strong vision but concentrated maintenance, the same key-person risk flagged
  for the smaller Go frameworks, with the extra fragility of a young project.
- **Tool execution happens inside Pi's process**, so Kairon's `ToolTrust` gate and sandbox containment must
  be enforced either by configuring Pi's tool set and running it inside Kairon's existing container sandbox,
  or by routing tool calls back to the host over the RPC channel and gating them there. The RPC mode is the
  one that makes host-side gating clean; the print/JSON one-shot mode leaves gating to Pi's own config, which
  is a weaker guarantee. Verify which the RPC surface actually supports before relying on it.
- The output/JSON contract is not a stability-promised API the way an official SDK's types are; treat it as a
  versioned external contract and cover it with the stub/self-test harness.

Fit to Kairon: **High, and the best of the non-Go options.** It is the closest architectural analogue to
`kiro-cli`, it is MIT-licensed, it is multi-provider with reported usage, and it exposes a process/RPC
integration surface that matches the seam Kairon already has. The decisive trade-off is accepting a Node.js
runtime dependency and a pre-1.0 contract in exchange for not writing the agent loop in Go.

### 3bis.2 OpenAI Agents SDK + Codex CLI (Python / TypeScript)

The OpenAI Agents SDK (Python and TS) provides agents, tool-calling, handoffs and sessions; the Codex CLI
exposes an `mcp-server` mode and a Codex SDK for automation/CI. Integration is either as a subprocess CLI or
by standing up an MCP/HTTP server the Go host calls.

Strengths: first-party OpenAI maintenance; mature tool-calling and multi-agent handoffs; Codex's MCP server
is a clean process boundary.
Weaknesses: OpenAI-centric (other providers are second-class or via compatible endpoints), so it fights the
Claude-first requirement; Python/Node runtime dependency; heavier than Pi for Kairon's narrow need. Also a
model-provider's own agent runtime carries the same category of license consideration the project is trying
to move away from. Fit: **Low-medium**, mainly if Kairon standardized on OpenAI models.

### 3bis.3 LangGraph (Python)

The most capable graph/state-machine agent runtime, with checkpoints, interrupts and multi-agent
orchestration; broad provider coverage via LangChain. Integrated through a FastAPI/uvicorn server (the
common `langgraph-agent-toolkit` pattern) that the Go host calls over HTTP.

Strengths: best-in-class orchestration and human-in-the-loop interrupts (maps onto Kairon's gates); large
ecosystem and provider list; production patterns are well documented.
Weaknesses: Python runtime plus a web-server process to supervise is the heaviest integration here; the
LangChain dependency surface is large and has its own churn; it is far more framework than a one-prompt
`Invoke` seam needs. Fit: **Low** unless Kairon's pipeline orchestration (not just model calls) is what you
want to outsource, in which case it competes with Eino/tRPC-Agent-Go but across a process boundary.

### 3bis.4 Pydantic AI / `clai` (Python)

Pydantic AI is a typed Python agent framework with multi-provider support, tool-calling and structured
output; it ships a CLI (`clai`) and can run as a uvicorn server. Clean, well-typed, actively maintained.

Strengths: strong typing and structured output; multi-provider including Anthropic; both CLI and server
surfaces for integration.
Weaknesses: Python runtime dependency; the CLI is aimed at interactive chat rather than a documented
machine-to-machine contract, so a server wrapper is the realistic integration and that is extra surface to
own. Fit: **Low-medium**; a reasonable Python counterpart to Pi, but with less of a drop-in
process-integration story than Pi's explicit RPC mode.

### Layer C summary

If a non-Go framework is on the table, **Pi is the recommended candidate**: MIT-licensed, multi-provider with
reported usage, architecturally a near-twin of `kiro-cli`, and shipping an explicit RPC/process-integration
mode that fits Kairon's existing `exec.go` / `internal/acp` seam. The other three are heavier or
provider-skewed and only win if Kairon wants to outsource orchestration (LangGraph) or has standardized on
one provider (OpenAI Agents SDK). Every Layer-C option trades the Go static-binary story for a Node.js or
Python runtime dependency in the sandbox and CI; that trade is the crux of the decision, and it is the same
kind of external-runtime dependency `kiro-cli` already imposes today.

## 4. Side-by-side

| Option | Maintainer | Maint. risk | Claude first-class | Other providers | Owns agent loop? | MCP | Reported usage | Go floor | Runtime weight |
|---|---|---|---|---|---|---|---|---|---|
| Genkit Go | Google (1st-party) | Low | Yes (plugin) | Gemini, OpenAI-compat family, Ollama | Partial (agents experimental) | Yes | Yes | 1.25+ | Medium (Dev UI, flows) |
| Eino | ByteDance | Medium (0.x churn) | Yes | OpenAI, Gemini, DeepSeek, Qwen, Ollama, ... | Yes (adk, roadmap) | Yes | Yes | 1.18+ | Medium-high |
| Google ADK Go | Google (1st-party) | Low | No (OpenAI-compat only) | Gemini-first, Apigee | Yes | Yes | Yes | 1.26.6+ | High (GCP/Vertex gravity) |
| tRPC-Agent-Go | Tencent | Medium (small community) | Yes | Bedrock, Gemini, Ollama, OpenAI-compat, ... | Yes (+ GraphAgent, skills, failover) | Yes | Yes | 1.21+ | Medium |
| LangChainGo | community | High (unmaintained) | Yes (on paper) | 17 providers | Weak (prompt ReAct) | partial | no middleware | 1.24+ | Low |
| Anthropic Go SDK | Anthropic (1st-party) | Low | Yes (native) | Claude only (+ Bedrock/Vertex via base URL) | No (you own it) | No (pair w/ MCP SDK) | Yes | current | Minimal |
| OpenAI Go SDK | OpenAI (1st-party) | Low | via compat shim | OpenAI + OpenAI-compatible | No (you own it) | No (pair w/ MCP SDK) | Yes | current | Minimal |
| MCP Go SDK | MCP + Google | Low | n/a (tools) | n/a | No (plumbing) | Is MCP | n/a | current | Minimal |
| Pi (pi-mono) | earendil-works | Medium-high (pre-1.0) | Yes (native) | OpenAI, Google, Mistral, Bedrock, ... (~31) | Yes (pi-agent-core) | Yes | Yes | n/a (Node.js) | External runtime (Node) |

"Owns agent loop" = provides the tool-call loop, multi-turn sessions and sub-agent orchestration so Kairon
does not write them. Pi is non-Go and integrates across a process boundary (print/JSON or RPC), so its "Go
floor" is n/a but it adds a Node.js runtime dependency; the other Layer-C options (OpenAI Agents SDK,
LangGraph, Pydantic AI) are omitted from the table because they are heavier or provider-skewed, see section
3bis. Provider/version facts are as of the 2026 comparison sources cited in section 6 and should be
re-verified against each module's current release before implementation.

---

## 5. Recommendation for Kairon

Framed against the actual seam, three coherent paths emerge. They are not mutually exclusive: Path B can grow
into Path A later, and Path C is the option when a ready-made harness beats both building and embedding.

### Path A - adopt one agent framework (less code, more dependency)
If Kairon wants the tool loop, sessions and sub-agents handed to it:

- **tRPC-Agent-Go** is the best capability fit: Claude and Bedrock both first-class, local (no GCP gravity),
  and it ships sandboxed Agent Skills, an eval package, MCP, and model failover, all of which map onto things
  Kairon already does by hand. The cost is a smaller community for a core dependency.
- **Genkit Go** is the best maintenance bet (first-party Google, Anthropic plugin, best observability) *if*
  you can accept that its session/sub-agent layer is still experimental and start with single-turn + tool
  loop.
- **Eino** is the choice only if multi-agent *orchestration* (modelling the krew-lead/architect/builder
  pipeline as a typed graph) becomes the primary problem; otherwise it is more machinery and more 0.x churn
  than the seam needs.
- **Google ADK Go** and **LangChainGo** are not recommended: ADK is Gemini/GCP-centric with no first-class
  Claude; LangChainGo is unmaintained.

### Path B - official provider SDKs + MCP SDK, own a thin loop (more code, least risk)
Add a `direct` backend built on the **official Anthropic Go SDK** (Claude, native tool use, reported usage)
and the **official OpenAI Go SDK** (OpenAI-compatible family), with the **official MCP Go SDK** exposing
Kairon's skills/tools. Kairon writes a small, well-tested tool-call loop and conversation store, and
implements `ToolTrust` at the tool-dispatch point (a strict upgrade: the decision becomes reported and
inspectable, unlike kiro-cli's undetectable denials).

This path has the lowest dependency and abandonment risk (three first-party SDKs), keeps the binary small,
and sits most naturally behind `inference.Backend.Invoke`, which is itself a one-call seam. The cost is that
Kairon owns ~200 lines of loop and a session store that a framework would otherwise provide. Given that
Kairon already owns a pluggable-backend abstraction, a deterministic stub, a sandbox, and a tool-trust model,
that loop is a small, aligned increment rather than new surface area.

### Path C - integrate a ready-made non-Go harness across a process boundary (least Go code, external runtime)
Instead of building the loop (Path B) or embedding a Go framework (Path A), drive an existing non-Go harness
the way Kairon already drives `kiro-cli`. **Pi (`@earendil-works/pi`)** is the recommended candidate: MIT
licensed, ~31 providers (Claude and Bedrock native), reported usage, and an explicit RPC/process-integration
mode plus a print/JSON mode that fit Kairon's `exec.go` wire envelope and `internal/acp` JSON-RPC client. It
is the closest architectural twin to `kiro-cli`, so the behaviour gap the eval suite must close is smallest.

The trade is a **Node.js runtime dependency** in the sandbox image and CI, and a **pre-1.0 external contract**
to pin and cover with the self-test harness. Tool-trust gating must be enforced either by running Pi inside
Kairon's container sandbox with a constrained tool set, or by routing tool calls back to the host over the
RPC channel and gating there (verify Pi's RPC surface supports host-side tool dispatch before relying on it).
Path C swaps one external-process inference dependency (`kiro-cli`, with its license concern) for another
(Pi, MIT), which directly answers the license motivation while keeping Kairon from owning the agent loop at
all.

### Suggested starting point
The paths answer different appetites, so pick by how much of the agent loop Kairon should own:

- If the priority is **lowest risk and a small static binary**, prototype **Path B** behind a new `inference`
  backend named `direct`, Claude-only first (Anthropic SDK + single-turn, no tools) to validate parity of
  `Response.Text`, reported `Usage`, timeout and error-string behaviour against the `kiro-cli` backend using
  the existing stub/self-test harness. Then layer in the MCP tool loop and `ToolTrust`, then multi-turn
  (`Request.Turn`), then OpenAI-compatible providers.
- If the priority is **fastest replacement of `kiro-cli` with the least new Go code**, prototype **Path C**
  with Pi behind a `direct`/`pi` backend over its RPC mode, reusing the `exec.go` envelope and `internal/acp`
  patterns, and accept the Node.js dependency. This is the shortest route to dropping the CLI license
  concern because Pi is a near-drop-in architectural replacement.
- Re-evaluate **Path A** (adopting **tRPC-Agent-Go** or **Genkit Go** in-process) only if the
  sub-agent/orchestration surface Kairon needs grows past what a thin loop or an external harness should own.

In all cases, keep `kiro-cli` as the default backend until the new backend reaches behavioural parity under
the eval suite. My recommendation: evaluate Path B and Path C in parallel as `direct` and `pi` backends,
since both reuse the existing seam and the stub harness makes parity measurable, then choose based on whether
the team would rather carry ~200 lines of Go loop (B) or a Node.js runtime and an external contract (C).

This keeps the license-driven motivation satisfied (Kairon calls the APIs itself or via an MIT harness, no
proprietary-CLI inference dependency), preserves the narrow seam, and defers the larger in-process framework
bet until the requirements that would justify it are concrete.

---

## 6. Sources

Framework comparison, maintenance snapshots, provider tables and code shapes:
- "Top Gen AI Frameworks for Go in 2026: A Hands-On Comparison", xavidop (dev.to), covering Genkit Go, Eino,
  Google ADK Go, tRPC-Agent-Go, LangChainGo with versions and last-commit dates.
- "Top 7 Best Golang AI Agent Frameworks ..." (reliasoftware.com) and "Go Generative AI Frameworks & SDKs"
  (itechguides.com) for the broader option set.

Project/SDK primary sources:
- Genkit Go: `github.com/genkit-ai/genkit` (go module).
- Eino: `github.com/cloudwego/eino` and `eino-ext`.
- Google ADK Go: `google.golang.org/adk/v2`.
- tRPC-Agent-Go: `trpc.group/trpc-go/trpc-agent-go`.
- LangChainGo: `github.com/tmc/langchaingo`.
- Anthropic Go SDK (official): `github.com/anthropics/anthropic-sdk-go` and platform.claude.com Go docs
  (native tool use, token counting, base-URL override).
- OpenAI Go SDK (official): `github.com/openai/openai-go`.
- MCP Go SDK (official, with Google): `github.com/modelcontextprotocol/go-sdk`.
- Multi-provider shims: Jetify `github.com/jetify-com/ai`, `github.com/zendev-sh/goai`,
  `github.com/plexusone/omnillm-core`, `github.com/gotoailab/llmhub`, `github.com/hoangvvo/llm-sdk`.

Non-Go (Layer C) frameworks integrated across a process boundary:
- Pi: `github.com/earendil-works/pi` (pi-mono), README and `packages/ai`, `packages/agent`,
  `packages/coding-agent` docs; `pi-mono-docs` architecture overview (four run modes: TUI, print/JSON, RPC,
  SDK; ~31 providers; MIT license; v0.74.x).
- OpenAI Agents SDK + Codex CLI: developers.openai.com cookbook "Building Consistent Workflows with Codex CLI
  & Agents SDK" (Codex `mcp-server` and Codex SDK).
- LangGraph: `langgraph-agent-toolkit` (PyPI) for the FastAPI/uvicorn server integration pattern.
- Pydantic AI `clai`: pydantic-ai docs "Command Line Interface (CLI)" (CLI + uvicorn server surfaces).

Kairon internal grounding:
- `internal/inference/inference.go` (the `Backend` interface, `Request`/`Response`, `Turn` continuity
  contract, `ToolTrust`, usage source).
- `internal/inference/kirocli.go`, `internal/inference/exec.go` (current CLI behaviour being replaced).
- `.kairon/specs/issue-283-pluggable-inference-backend.md` (the abstraction that anticipates this backend).

Provider, version and maintenance facts above are drawn from 2026-dated comparison sources; re-verify each
module's current release, Go version floor and provider list at implementation time.
