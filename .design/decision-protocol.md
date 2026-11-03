# Decisions — an OpenAI-native endpoint, handled specially

> For contributors touching `internal/protocolserver/openai_decisions.go`,
> `OpenAIClient.DecisionsNew`, or `forwarding.ForwardOpenAIDecisions`.

## Current implementation (2026-11-02)

Both `POST /tingly/:scenario[/v1]/decisions` and `.../systemone` share the
same authentication, team scope, rule routing, usage accounting and handler.
Both identify OpenAI Decisions (`input`, question array) and System One
(`state`, question object) independently of the inbound URL. Mixed shapes are
rejected. Legacy requests with neither identifying field remain opaque.

### Upstream selection

`internal/decision.ResolveTarget` is the single registry for the
known upstream endpoint and dialect. Exact host matching uses the resolved
OpenAI-compatible provider base, not a model or provider display name:

| Host | Upstream endpoint | Dialect |
| --- | --- | --- |
| `api.typesafe.ai` | `/v1/systemone` | System One |
| `openrouter.ai` | `/api/alpha/decisions` | System One |
| `api.openai.com` | `{base}/decisions` | OpenAI |
| `ai-gateway.vercel.sh` | `{base}/decisions` | OpenAI |

Unknown hosts and unrecognized OpenAI-compatible path prefixes keep `{base}/decisions` and opaque forwarding. A virtual provider
uses System One. The virtual server also converts incoming OpenAI bodies when
called directly. There is no UI override or automatic endpoint probing.

### Conversion

`internal/decision` performs request conversion only when both formats
are known and differ. A per-request context carries question order, original
names, choice values and score labels to convert the response. The model field
is replaced by the routed model upstream and echoed back as the caller's model.
Same-format and unknown-format requests continue as opaque passthrough.

The supported common subset is plain text, named or unnamed predicates/noul,
string choices with string descriptions, and scores with 2–10 string labels.
OpenAI score descriptions, boolean choices, structured input/instructions,
image assets, noul criteria, cache controls and other extra fields are rejected
before contacting an upstream. Question counts and choice limits are checked.
No probabilities, confidence or answer values are fabricated; incompatible,
refused, incomplete or unexpected upstream answers produce 502 after usage
accounting. Original model usage and provider metadata are retained, with
known token spelling aliases normalized and total calculated only when both
counts exist. Providers that omit token breakdowns cannot supply all OpenAI
usage-detail fields; the gateway does not invent them.

Decisions remain non-streaming, use content-free service selection, and skip
the chat transform chain. The endpoint does not perform automatic failover.
Conversion is based on the published OpenAI schema and TypeSafe examples;
provider-specific extensions and live credentials require separate verification.

References: [OpenAI Decisions](https://developers.openai.com/api/docs/guides/decisions)
and [TypeSafe OpenAPI](https://api.typesafe.ai/openapi.json). Both define score
as the expected value of zero-based ordered levels.


## Historical design record

The following original sections are retained as a historical record. They
describe the implementation and evidence available at the time they were
written; statements about unpublished schemas, passthrough, supported endpoints,
verification status, and unfinished work may be outdated. Where they differ,
the current implementation above is authoritative.

## Position

> Update: a dedicated `decisions` scenario (`TransportDecisions`, decisions-only,
> page at `/agent/decisions`) was added as a convenience entry point, like
> `embed`. It adds no new provider style or protocol family; `openai`, `team`
> and every other `TransportOpenAI` scenario keep serving `/decisions`
> unchanged.

OpenAI's Decisions API (announced at DevDay, 2026-09-29; limited preview) picks
one answer from options the caller defines. Its request/response schema is not
public. Jev (TypeSafe AI) uses the same body shape, but natively at
`POST https://api.typesafe.ai/v1/systemone`, not `/decisions`.

**Decisions are an OpenAI endpoint, not a protocol family.** There is no new
provider style, no new provider field, no new scenario, and no UI: any
`openai`-style provider already carries everything needed (base URL, token,
proxy). The endpoint is `{APIBase}/decisions`, so OpenAI's base is
`https://api.openai.com/v1` and Vercel AI Gateway's is
`https://ai-gateway.vercel.sh/v1` (it serves Jev behind `/decisions`). Two
vendors host decisions elsewhere, so `decisionsTarget` (internal/client/openai.go)
rewrites the upstream URL by host, body unchanged: TypeSafe
`api.typesafe.ai` → `/v1/systemone`, OpenRouter `openrouter.ai` →
`/api/alpha/decisions`. Both are in the catalog (`typesafe-ai`, `openrouter-ai`).
Together/Fireworks: no decisions endpoint found.

Earlier iterations modeled Jev as a fifth protocol family (style/type/transport/
scenario, a fork URL field, a harness matrix pair, a frontend page). That was
~50 files for one passthrough endpoint, and it forced unrelated tests to skip
the new family. It was removed in favor of this design.

## The one special case: opaque body

OpenAI's schema is unpublished and vendors differ, so the body is opaque JSON. The gateway reads and rewrites exactly one field:

- `model` is read for rule routing and replaced by the routed service model on
  the way up; the caller's model is echoed back on the way down (only when the
  response carries a differing top-level `model`; other bytes are untouched).

Everything else — including new vendor fields — passes through byte-for-byte.
Optional `usage` is lifted for accounting (`prompt_tokens`/`input_tokens`,
`completion_tokens`/`output_tokens`); absent means zero.

## Wiring

```text
POST /tingly/:scenario/v1/decisions   (and /tingly/:scenario/decisions)
  -> HandleOpenAIDecisions              scenario must declare TransportOpenAI
     rule by model -> selectServiceForEmbeddings (no content-based smart routing)
     provider.ResolveStyle(openai)      honors the dual OpenAI URL
     ForwardOpenAIDecisions -> OpenAIClient.DecisionsNew
        c.client.Post("decisions", rawBody)   SDK auth/proxy/error handling
```

Like embeddings: non-streaming, no chat transform chain, no guardrails (a
decision payload is not prose), no failover (single-shot call). Upstream errors
come back as `*openai.Error` and keep their HTTP status via
`stream.SendForwardingError`. `DecisionsNew` is on `OpenAIClientInterface`;
Kimi/Codex/xAI/OpenCode inherit it from the embedded `*OpenAIClient`.

## Jev compatibility

Jev is the only published decisions API, so it is the concrete wire reference
(from its public docs as quoted by third-party guides; the doc sites themselves
were not reachable when this was checked):

```text
POST {base}/decisions        Authorization: Bearer <key>
{ "model": "typesafe/jev-1.13",
  "state": "<string | object | array>",
  "questions": { "<id>": { "type": "choice|score|noul", "instructions": "...", "criteria": ... } } }
  choice: criteria = {"option": "description", ...}
  score:  criteria = ["lowest level", ..., "highest"]   (2-10 levels)
  noul:   instructions only (optional true/false criteria)
-> { "model": "jev-1.13.0",
     "answers": { "<id>": { "type":"choice","choice":"billing","probabilities":{...},"confidence":0.81 }
                  | { "type":"score","score":2.4,"legend":{"0":"..."},"probabilities":{"0":0.01,...},"confidence":0.57 }
                  | { "type":"noul","noul":0.94 } },
     "usage": { "input_tokens": 318, "output_tokens": 52 } }
```

How the gateway supports it: a Jev-serving provider is an `openai`-style
provider whose base ends so that `{base}/decisions` is valid, e.g. Vercel AI
Gateway `https://ai-gateway.vercel.sh/v1` (the SDK appends `decisions` and sends the
bearer token). The body is opaque, so `state`/`questions` pass through
untouched; only `model` is rewritten. Jev's `usage.input_tokens/output_tokens`
are lifted for accounting (`usage.cost` is ignored). The response `model` is
rewritten to the caller's model, so `jev-1.13.0` becomes what the client asked
for. Not verified against the live service (no key): error body shape, rate
limits, and whether extra fields exist beyond the examples above.

## vmodel: a working decisions service

`vmodel/decision` ships real, user-facing virtual models — not test fixtures —
so decisions can be tried without any upstream (onboarding, dry-runs, routing
demos). They speak the Jev shape above for `choice`, `score`, and `noul`
questions (`state` and a non-empty `questions` are required; a bad question is
a 400 in the OpenAI error envelope, an unknown model a 404).

- `decision-first`: first choice option, lowest score level, `noul` = 1.
  Predictable.
- `decision-stable`: SHA-256 of the compacted `state` plus the question id picks
  the option (0.8 mass on the pick, rest split evenly; `noul` 0.8 or 0.2) —
  reproducible, input-dependent, and independent of key spacing.

Answers are self-consistent: probabilities sum to 1, `confidence` is
1 − normalized entropy, and `score` is the probability-weighted level.

Wiring: `virtualserver.Service` owns a decision registry, mounts
`POST .../decisions` next to chat/responses (so the in-process private server,
`/virtual` endpoint, and the benchmark servers all serve it), lists the models
in the OpenAI model list, and seeds them into the builtin OpenAI vmodel
provider (they share the provider's base URL, like the real endpoint). vmodel
providers reach it through the standard SDK + transport chain, i.e.
`OpenAIClient.DecisionsNew` like any other upstream.

Harness: `TestEnv.SetupDecisionRoute` / `SendDecision`
(`internal/protocoltest/decision.go`) drive the real gateway against the real
vmodel service. Decisions are deliberately not a matrix pair (no source×target
conversion exists), so no existing matrix, cache-control, or golden test is
touched.

## Status / follow-ups

- As of early October 2026 `POST /v1/decisions` is feature-gated (403 for
  ordinary keys). Until a key is enabled, verify against a Jev endpoint.
- Revisit when OpenAI publishes the schema: typed request/response, usage
  field names, provider-form Verify probe, harness mock endpoint.
- Not built: decision ↔ chat conversion (would lose calibrated probabilities),
  frontend scenario page.

Tests: `vmodel/decision`, `vmodel/virtualserver/decisions_test.go`,
`internal/protocoltest/decision_test.go`, `internal/client/decision_test.go` (wire path, auth, body fidelity,
error status), `internal/protocolserver/openai_decisions_test.go` (helpers +
end-to-end route through a dual provider).
