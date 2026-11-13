# Native standalone Responses compaction

Tracking: [TEP-2160](https://github.com/tingly-dev/tingly-box/issues/2160).
Ordinary `/responses` native compaction is covered by #2166. This later change,
#2162, adds the separate `/responses/compact` JSON operation.

## Standalone forwarding


`HandleResponsesCompact` uses normal model authentication, team scope, rule
resolution and content-free service selection. It resolves the OpenAI dual URL,
rewrites only the model, sets non-streaming tracking, and uses
`ForwardOpenAIResponsesCompact` and `OpenAIClient.ResponsesCompact`. This is a
native gateway route, like images/decisions, not an admin/frontend SDK endpoint.

The request travels as raw JSON through the SDK's authenticated transport.
Typed input-union reserialization could invent IDs or lose fields on opaque
history. Do not run chat preprocessing, vision translation or Responses-create
defaults. TB does not reject or rewrite a caller-supplied `stream` field;
parameter acceptance is left to the upstream. The published compact response
contract remains JSON. Forwarding retains request cancellation and provider
timeouts. Cache, reasoning and output usage are normalized with the shared Responses usage helper.
Malformed successful upstream replies without an output array are gateway errors,
not fabricated successful compactions. Non-2xx upstream status is retained.

Codex OAuth rewrites the endpoint to
`/backend-api/codex/responses/compact`, maps the gateway's `:fast` model suffix,
and applies account/session affinity headers. It classifies the endpoint as JSON,
without forcing `stream=true`, adding `store=false`, or expecting SSE.
Images and ordinary Responses retain their existing protocol behavior.

Non-native API styles and explicitly chat-only providers return an explicit
`501 unsupported_operation`. Unknown OpenAI-compatible providers are allowed
to answer for their native endpoint; the gateway does not infer endpoint support
from a provider name or silently convert the request into a summary. The selected
service must support the operation; this endpoint does not add cross-provider
priority failover or advertise V2 globally.

## Verification and deferred work

Client and gateway tests cover request/window fidelity, authenticated routing,
Codex path/JSON handling, malformed replies, upstream errors and unsupported
backends. These use mock providers; live OAuth/ciphertext acceptance remains
unverified. The shared standalone compact-to-continue harness remains deferred.
Generic summary adapters, cross-protocol conversion and default V2 configuration
are separate future work.

## Held V2 configuration rollout

The separate #2164 draft defaults generated gateway/hybrid Codex configs to V2
and preserves explicit capability overrides. Keep that rollout on hold until
mixed-provider support exists; the native-only first wave is insufficient.
