# OpenCode Zen session header

OpenCode Zen (`opencode.ai/zen/…`, both the pay-as-you-go `/zen/v1` and the Go
subscription `/zen/go/v1`) rejects any request that arrives without a
conversation identifier:

```json
{
  "type": "MissingSessionID",
  "message": "Error from provider (Console Go): Request is missing x-opencode-session and cannot be routed efficiently."
}
```

Every request through Tingly-Box hit this (#1713). The client's own
`x-opencode-session` never reaches the upstream — the SDK builds a fresh
outbound request from the transformed params, so inbound headers are not
forwarded — which is why the failure was total rather than
client-dependent.

## Where the header is stamped

`OpenCodeClient` (`internal/client/opencode_client.go`), dispatched from
`ClientPool.GetOpenAIClient` next to the Codex, Kimi and Azure branches — one
vendor, one client, the same shape every other vendor handshake follows. Its
`openCodeRoundTripper` carries the single wire concern. Zen's Anthropic-style
base (`/zen/go` alongside `/zen/go/v1`) needs the same header, so
`GetAnthropicClient` dispatches to `NewOpenCodeAnthropicClient`, which reuses
the same layer; nothing else about the generic Anthropic client changes, so it
is not wrapped in a vendor type.

Dispatch is on the provider's API base (`IsOpenCodeZen`: host + `/zen` path
prefix through `ops.SplitProviderHostPath`) — the same host-match discipline
the request-side vendor transforms use, so a relay that merely mentions the
host in its path is not mistaken for Zen.

One difference from the OAuth vendor clients: Codex and Kimi replace the
transport outright, which deliberately drops the rule-flag layer so their
pinned handshake UA stays decisive. Zen providers are plain `api_key`
providers, where `extra_headers` and the UA precedence are supported features,
so `openCodeTransport` splices the vendor layer in *underneath* the generic
provider chain instead of replacing it. The layer also fills an **absent**
header only, so a session pinned through the rule's `extra_headers` wins on
both counts.

## What the value is

The hashed form of the request's resolved session
(`typ.GetSessionID`: Tingly header > native client header >
Anthropic `metadata.user_id` > client IP), which is the per-conversation
identity the gateway already uses for provider affinity.

Two properties matter, and Zen's own docs are explicit that this is what the
header is for (backend affinity, and with it prompt-cache warmth):

- **Stable per conversation.** One constant for all conversations would clear
  the 400 while collapsing every conversation onto one affinity scope; a fresh
  value per request would clear it while defeating cache reuse entirely.
- **Opaque.** The resolved session falls back to the client IP, and can be a
  user id carried in Anthropic metadata — neither is something to hand an
  upstream verbatim, hence the SHA-256.

A request with no resolved session (nothing put one in the context) gets a
random value rather than a shared constant: it still reaches the upstream, and
unrelated conversations stay off one scope. Every dispatch path sets the
session, so this is the probe/diagnostic case, not the request path.

## Diagnostics

The probe path goes through `ClientPool`, so probes get the header like any
other request. `probe.BuildCurl`'s **direct** command deliberately does not:
a direct call failing where the through-TB one succeeds is the diagnostic —
it is what shows the user the gateway is the part supplying the session.

## Verified against the live upstream

`internal/client/opencode_e2e_test.go` (skipped unless `OPENCODE_API_KEY` is
set) drives the real Go-subscription endpoint through `ClientPool`. All three
confirmed on 2026-09-08 against `https://opencode.ai/zen/go`:

| path | without the header | with it |
| --- | --- | --- |
| Chat Completions | 400 `MissingSessionID` | 200 |
| Chat Completions, streaming | — | 200, SSE chunks |
| Anthropic `/v1/messages` | 400 `MissingSessionID` | 200 |

So the Anthropic shape needing the header is measured, not assumed. What is
*not* measured is the pay-as-you-go `/zen/v1`: the gate covers it because the
header is harmless where it is not required, while missing it is a hard 400.

Two things the e2e test's own setup documents, both independent of this fix:

- The transport pool never inherits `HTTP(S)_PROXY` from the environment (see
  `NewOpenAIClient`), so the test takes `OPENCODE_PROXY_URL` and sets it as
  the provider's `proxy_url`, the way a user would.
- Not every Zen model answers on the Anthropic shape — `glm-5.3-flash` 500s
  there with or without the session header — hence
  `OPENCODE_ANTHROPIC_MODEL`.

## Not covered

The quota fetcher's `GET /zen/go/v1/usage` (`ai/quota/fetcher/opencode.go`)
uses its own HTTP client and needs no session — usage is account-scoped, not
conversation-scoped.
