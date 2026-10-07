# Decisions — an OpenAI-native endpoint, handled specially

> For contributors touching `internal/protocolserver/openai_decisions.go`,
> `OpenAIClient.DecisionsNew`, or `forwarding.ForwardOpenAIDecisions`.

## Position

OpenAI's Decisions API (announced at DevDay, 2026-09-29; limited preview) picks
one answer from options the caller defines. Its request/response schema is not
public. Jev exposes a compatible `POST {base}/decisions` surface.

**Decisions are an OpenAI endpoint, not a protocol family.** There is no new
provider style, no new provider field, no new scenario, and no UI: any
`openai`-style provider already carries everything needed (base URL, token,
proxy). The endpoint is `{APIBase}/decisions`, so a Jev provider is just an
OpenAI-style provider with base `https://www.jevai.org/api/v1`, and OpenAI's is
`https://api.openai.com/v1`.

Earlier iterations modeled Jev as a fifth protocol family (style/type/transport/
scenario, a fork URL field, a harness matrix pair, a frontend page). That was
~50 files for one passthrough endpoint, and it forced unrelated tests to skip
the new family. It was removed in favor of this design.

## The one special case: opaque body

The schema is unpublished and differs between vendors, so the body is opaque
JSON. The gateway reads and rewrites exactly one field:

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

## vmodel: a working decisions service

`vmodel/decision` ships real, user-facing virtual models — not test fixtures —
so decisions can be tried without any upstream (onboarding, dry-runs, routing
demos). They answer from the options in the opaque body, in two shapes:

```text
{"model","options":["a","b"]}                          -> {"answer","probabilities":{...},"usage"}
{"model","questions":{"q":{"options":[...]}}}          -> {"answers":{q:..},"probabilities":{q:{..}},"usage"}
```

Options are strings or objects named by `id`/`label`/`value`/`name`. Models:
`decision-first` (always the first option, confidence 1) and `decision-stable`
(SHA-256 of the request picks the option, confidence 0.8: reproducible but
input-dependent). A request without usable options is a 400 in the OpenAI error
envelope; an unknown model is a 404.

These shapes are this service's own simulation, derived from the Jev surface
and third-party write-ups — not OpenAI's schema, which is unpublished. Revisit
with the real schema.

Wiring: `virtualserver.Service` owns a decision registry, mounts
`POST .../decisions` next to chat/responses (so the in-process private server,
`/virtual` endpoint, and the benchmark servers all serve it), lists the models
in the OpenAI model list, and seeds them into the builtin OpenAI vmodel
provider. vmodel providers reach it through the standard SDK + transport chain,
i.e. `OpenAIClient.DecisionsNew` like any other upstream.

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
