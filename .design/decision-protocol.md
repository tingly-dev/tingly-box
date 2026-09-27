# Decision protocol — the Jev micro-model as a first-class protocol family

> For contributors touching `internal/protocolserver/decision.go`,
> `internal/typ/scenario_registry.go`, `internal/forwarding`, the protocoltest
> harness (`internal/protocoltest`, `vmodel/benchmark`), or the frontend
> decision scenario pages.
>
> Design principle: **harness-first**. A protocol that the test harness cannot
> exercise end-to-end is not a protocol — it is an endpoint. Decision is
> designed as the fifth protocol family next to OpenAI Chat, OpenAI Responses,
> Anthropic V1/Beta, and Google, with the same layers: type system, serving
> surface, dispatch pipeline, validation matrix, mock upstream, and matrix
> coverage.

## 1. Context

Jev is not a chat-completion model. It is a micro-model whose native
`POST {base}/api/v1/decisions` surface accepts a model identifier, shared
state, and a map of typed questions (`choice`, `score`, `noul`, …) and returns
structured answers with calibrated probabilities. Treating that payload as
chat would lose the schema and push callers into parsing generated prose —
the exact failure mode the native protocol exists to avoid.

The first implementation of this integration was endpoint-first, not
harness-first, and failed on every layer at once:

- `HandleDecision` hand-rolled a raw `http.Client`, bypassing the production
  dispatch pipeline (no tracking/usage plumbing, no house error
  shape, always-zero usage records).
- Nothing validated provider `api_style` against the scenario, so a decision
  rule could be bound to an OpenAI provider and only explode at request time.
- The harness had no notion of decision at all: no mock endpoint, no fixture,
  no matrix pair — the flow was untestable end-to-end.
- The frontend hand-rolled a page instead of the shared scenario skeleton and
  broke i18n parity.

This document is the corrected design. Everything listed in §3–§7 is
implemented on this branch; §8 lists deliberately deferred work.

## 2. Capability, not style — the decision endpoint fork

Decision is **not a provider style users must pick**. It is a dedicated
client (`internal/client.DecisionClient`) speaking an endpoint fork that
OpenAI/Anthropic upstreams do not natively have. A provider gains decision
capability in one of two ways, and both dispatch through the same client:

1. **Jev-native**: `api_style = decision`; `APIBase` is the decisions surface.
2. **Fork endpoint**: any openai/anthropic-style provider sets the optional
   `api_base_decision` URL (same "independent optional URL" pattern as the
   dual chat URLs).

The user-facing consequence: when configuring a provider you never care
whether it speaks O or A — if the model supports Deciding, fill the decision
endpoint and bind it to a `decision` rule. The protocol layers still exist
internally (`APIStyleDecision`, `TypeDecision`, `TransportDecision`,
`ScenarioDecision`), but they describe the dispatch surface, not a
configuration choice.

| Layer | OpenAI chat | Decision |
|---|---|---|
| Provider family (`ai.APIStyle`) | `openai` | `decision`, or any style + fork URL |
| Request target (`ai.APIType`) | `openai_chat` | `decision` |
| Scenario surface (`typ.ScenarioTransport`) | `openai` / `embed` / `imagegen` | `decision` |
| Scenario | `openai`, `embed`, `imagegen`, … | `decision` |
| Client | OpenAI/Anthropic SDK wrappers | `client.DecisionClient` (dedicated) |

`internal/protocol.APIStyle/APIType` are aliases of the public `ai` package
types; `TypeDecision`/`APIStyleDecision` are registered in both.

Upstream wire contract (unchanged from Jev):

```text
POST {DecisionBase()}/decisions         (Bearer = provider access token;
                                        APIBaseDecision fork URL or APIBase for Jev-native)
{ "model": "...", "state": {...}, "questions": { "q1": { "type": "choice", ... } } }
→ 200 { "answers": {...}, "probabilities": {...}, [optional "usage": {...}] }
```

The gateway's only body edit is the routed model name (`req["model"] =
service.Model`). Everything else stays opaque JSON so new Jev fields remain
forward-compatible.

## 3. Serving surface & pipeline

Routes (both mounted via `SetupMixinEndpoints`, so scenario-scoped and
profile-scoped paths work like every other endpoint):

```text
POST /tingly/:scenario/v1/decisions
POST /tingly/:scenario/decisions
```

`HandleDecision` follows the embeddings-handler shape (the closest structural
sibling: structured, non-streaming, native-protocol-passthrough), not a
private shortcut:

1. `IsValidRuleScenario` + `ScenarioSupportsTransport(scenario, TransportDecision)`.
2. Size-limited body read; validate `model` + `questions` exist as objects
   (internals stay opaque for forward compatibility).
3. `determineRuleWithScenario` → routing stage pipeline (health → smart
   routing → affinity → tactic) via `selectService`.
4. `SetTrackingContext` + session-ID resolution (usage records, request
   timeline, and LB affinity all key off these).
5. **Dispatch** through the dedicated client
   (`ClientPool.GetDecisionClient` → `forwarding.ForwardDecision`): model
   rewrite, forward, propagate upstream status/body verbatim, hop-by-hop
   headers stripped.
6. Usage: `trackUsageWithTokenUsage` with real token usage when the upstream
   reports an optional `usage` object (OpenAI- or Anthropic-style field
   spellings both accepted), zero otherwise — never a hardcoded zero on
   success. Errors go through the house `{"error":{message,type}}` shape via
   `SendErrorResponse`/`failForward`, which also propagate the upstream HTTP
   status.

**No failover, by design.** A decision is a single-shot advisory micro-model
call, not a retryable stream: there is no failover gate, no candidate
rotation, no breaker feedback loop. (Chat handlers fail over because every
attempt re-transforms the request for the next candidate's style; decision
has neither transforms nor a stream worth retrying.)

No protocol recorder: recording exists to replay protocol *conversions*, and
decision performs none (native → native). Access log, usage records, and the
`/api/v1/requests` timeline all still traverse (they are fed by the tracking
context and routing log lines, not the recorder). No guardrails: those
evaluate chat message content; a decision question is not prose.

## 4. Validation (capability-gated at binding time)

The incompatibility is refused where it is created — when a rule is saved
(`Config.validateRuleServices` → `typ.ProviderSupportsScenario`):

- **Decision surfaces are capability-gated, not style-gated**: a rule bound
  to `decision` accepts any provider with `HasDecisionEndpoint()` — a
  Jev-native provider, or an openai/anthropic provider with a decision fork
  URL. A plain chat provider without the fork is rejected with a message
  naming the missing endpoint.
- **Chat surfaces remain style-gated** (chat conversion families + dual-URL
  awareness). A Jev-native provider cannot bind to chat scenarios: its
  APIBase is the decisions endpoint, it has no chat surface.
- `api_base_decision` is api_key-only (the decision client authenticates with
  a plain Bearer token), enforced in the provider create/patch handlers.

**Request-time** stays as defense in depth (`requireDecisionEndpoint` → house
error shape).

**TUI/CLI**: provider add offers the "Decision (Jev)" style (Jev-native
entry) and an optional "Decision endpoint URL" input for chat-style
providers; the base URL must parse and the `/decisions` suffix is normalized
by the gateway (`client.DecisionEndpointURL`), accepting `…/api/v1`,
`…/api/v1/`, or `…/api/v1/decisions`.

**Web UI**: the provider form has a third, optional Decision slot next to the
OpenAI/Anthropic protocol slots — enabling it never changes the provider's
chat style, mirroring the capability model exactly.

## 5. Harness integration

The vmodel scenario responder (what protocoltest actually drives) gains the
mock decision endpoint; the capture layer gains a matching endpoint kind; the
matrix gains decision pairs.

- `vmodel/benchmark/scenario`: `FormatDecision ResponseFormat = "decision"`;
  scenarios declare `NonStream`-only builders (decisions are request/response
  JSON; `Stream == nil` already skips streaming modes in the matrix).
- `vmodel/benchmark/scenario_responder.go`: `mux.HandleFunc("/decisions", …)`
  — scenario detection works unchanged because the decision body carries
  `model`.
- `vmodel/benchmark/capture.go` + `internal/protocoltest/virtual_server.go`:
  `EndpointDecision` so `EndpointHits` / `LastRequest` can target the
  forwarded upstream request.
- `internal/protocoltest/testenv.go`: `buildRequest` decision case (posts to
  `/tingly/decision/v1/decisions`), `targetToAPIStyle`,
  `sourceToRuleScenario` → `typ.ScenarioDecision`, `sourceToStyle`,
  `parseFromJSON` decision branch. The decision route's provider is built on
  the primary UX path — an `openai`-style provider with the fork URL set to
  the bare virtual URL (the gateway appends `/decisions`) — so the harness
  exercises capability-gated dispatch, not a Jev-native special case.
- `internal/protocoltest/matrix.go`: `decision → decision` passthrough pair.
  Decision is a **leaf family**: no pairs into chat formats, because the
  transform layer has no decision↔chat conversion (§8) — and the matrix must
  mirror the dispatch graph, not aspirations.
- Assertions: status/body passthrough checks on `RoundTripResult.RawBody`
  plus upstream-capture checks (`LastRequest(EndpointDecision)` carries the
  routed model) — the `content_shapes` pattern for structured protocols.
- Negative coverage: a decision scenario rule pointing at an `openai`-style
  provider is rejected at binding time; a decision request that reaches the
  gateway with a mismatched provider still fails with the house error shape.

`cli/harness matrix` needs no changes: `--source/--target/--scenario` are
string filters and the section registry stays protocol-agnostic.

## 6. Frontend UX

- `decision` is a registry scenario (`scenarioRegistry.tsx`, Psychology icon,
  hideable, `/agent/decision`), shown in the SDK-tools nav group next to
  Embedding.
- `UseDecisionPage` uses the shared `ScenarioPage` skeleton (endpoint card via
  `ProviderConfigCard`, rules card via `TemplatePage`) — the same shape as
  `UseEmbedPage`, not a hand-rolled page. Endpoint: `<base>/tingly/decision`.
- `ApiStyleBadge` renders the `decision` style (Decision mark, violet tint).
- i18n parity in en/zh/ru (`layout.nav.useDecision`,
  `scenarioOverview.descriptions.decision`); zh intentionally falls back to
  English for descriptions per the locale's own rule.

## 7. What "usable end-to-end" means here

1. Create a decision provider (TUI/CLI/API; UI support in §8).
2. Bind a `decision` rule to it — rejected up front if the provider speaks
   chat instead of decision.
3. `POST /tingly/decision/v1/decisions` with a Jev-shaped body → routed to
   the bound service, response passed through, usage/requests timeline show
   the row.
4. A failing decision upstream surfaces its own status and error body
   verbatim — no failover, by design (§3).
5. `go run ./cli/harness matrix` covers decision against the mock upstream on
   the real gateway path.

## 8. Deliberately deferred (designed, not built here)

- **decision → openai_chat / anthropic conversion.** Serving decisions from
  chat-only models means mapping typed questions onto a schema-constrained
  prompt and parsing structured answers back — doable, but the returned
  "probabilities" would no longer be calibrated, which silently changes the
  product contract for callers. The capability gate (§4) is the honest
  behavior until that work exists; the harness's pair list and the
  `APIType`/`APIStyle` plumbing already have the leaf slots for it. Note this
  is about *models without a decision endpoint at all* — providers with a
  decision fork are already first-class citizens.
- **Jev workflow presets / MCP tools.** Application-level conveniences on top
  of the native protocol; they change nothing about this wire contract.
- **Decision probe.** The provider form's Verify button probes chat
  endpoints; a decision-endpoint probe (a cheap well-formed request) would
  close the loop on "is this fork URL actually alive?"
