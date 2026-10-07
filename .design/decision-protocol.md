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

## Status / follow-ups

- As of early October 2026 `POST /v1/decisions` is feature-gated (403 for
  ordinary keys). Until a key is enabled, verify against a Jev endpoint.
- Revisit when OpenAI publishes the schema: typed request/response, usage
  field names, provider-form Verify probe, harness mock endpoint.
- Not built: decision ↔ chat conversion (would lose calibrated probabilities),
  frontend scenario page.

Tests: `internal/client/decision_test.go` (wire path, auth, body fidelity,
error status), `internal/protocolserver/openai_decisions_test.go` (helpers +
end-to-end route through a dual provider).
