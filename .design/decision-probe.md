# Decision rule probes

A Decisions rule probe is an `/api/v2/probe` request with `operation` set to
`decisions`, `target_type` set to `rule`, and the rule's scenario and UUID.
It uses the existing loopback authentication and normal model routing to call
`/tingly/{scenario}/decisions`. It is not cached, streamed, or sent as chat.
Rule quick tests use a fixed choice example; the request can instead supply a
`decision_request` containing an OpenAI Decisions or System One body. The
selected rule provides the model, and a wildcard rule requires an explicit
concrete `model`. The selected rule must be active. Routing remains natural:
a more specific matching rule can win, so the debug response includes the
matched rule and selected provider/model when available.

The probe returns the upstream answer in `decision_response` and `content`,
plus actual latency, reported token usage and routing trace. An HTTP success
without matching answer types is a probe error. Stream, tool, thinking, vision
and raw chat controls do not apply. The cURL preview is built from the same
body and uses a gateway-token placeholder rather than exposing a credential.

The Decisions scenario's quick test runs this operation. Unknown upstreams keep
the production gateway's existing passthrough behavior. The feature depends on
`.design/decision-protocol.md`; it does not add a provider-direct test or a
new gateway protocol.
