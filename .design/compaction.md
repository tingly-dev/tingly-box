# Native Responses compaction

`POST /tingly/:scenario[/v1]/responses/compact` forwards the standalone OpenAI
Responses compact operation. This increment is native forwarding only.

## Attempt the operation, independent of inference protocol

The route uses normal model authentication, team scope, model alias/rule
resolution and one selected service. It resolves the provider's OpenAI dual URL
when available (respecting the existing explicit style override), then actually
attempts `responses/compact`. A Chat inference preference or Anthropic API style
is not evidence that the provider lacks this endpoint. No preliminary inference,
capability probe, summary prompt or conversion is performed.

OpenAI-compatible providers use their usual SDK transport/auth; Codex OAuth uses
`/backend-api/codex/responses/compact`, account/session headers and the existing
`:fast` model mapping. Anthropic-style providers use the Anthropic SDK's existing
authentication and proxy transport to attempt `v1/responses/compact`. Google API-key
HTTP bases are attempted with `x-goog-api-key`. Cloud/issuer-specific transports
retain their own supported operations; this does not add compact implementations
to proprietary provider APIs.

Compact is JSON request/response, not the Responses-create SSE operation. Only
the model alias is changed on the native payload. Do not inject `stream=true`,
`store=false`, generated input item IDs, or interpretation of opaque ciphertext.
The returned output window and unknown fields remain intact. A malformed 200
response (missing output array or containing an error) is a 502, never a success.

## Client error contract

Only after an actual upstream failure, a missing endpoint (plain 404), 405, 501,
or an explicit unsupported-operation code on 400/404 is normalized to:

```json
{"error":{"type":"invalid_request_error","code":"unsupported_compaction","message":"The selected upstream does not support native Responses compaction"}}
```

The HTTP status is 400. Structured `model_not_found` remains 404; authentication,
permission, quota, protection and transient failures keep their upstream status
and machine-readable code. These errors never trigger a summary inference or a
retry against the same upstream. Network errors remain transport failures.

Codex 0.169.0 maps HTTP 400 to a terminal request error. Earlier real-CLI probes
observed `willRetry:false` for `unsupported_compaction` on ordinary `/responses`.
That CLI version uses ordinary `/responses` for local/remote-V2 compaction; it does
not exercise the standalone endpoint. This distinction is deliberate: these
standalone endpoint tests do not claim real Codex ciphertext acceptance.

## Continuation and scope

The client can replay the returned output window to a compatible native Responses
inference route. Parsing and per-attempt cloning preserve unknown fields on native
compaction/reasoning items without a float64 round trip. A continuation routed to
Chat/Anthropic conversion fails explicitly instead of dropping native state.
Native ciphertext remains bound to its original compatible provider/backend.

This increment adds no summary codec, Chat/Anthropic prompt adapter, remote V2
emulation, configuration-default change, capability cache or priority failover
for standalone compact. Ordinary Responses failover behavior is otherwise intact.

## Reproduce

```sh
go test ./internal/client ./internal/protocolserver ./internal/protocoltest -run 'Compact|OpaqueReplay' -count=1
go build -o harness ./cli/harness
./harness matrix --mode=native_compact --json
```

The CLI and `TestNativeCompactAcceptance` share 18 real-HTTP gateway cases: native
compact-to-continue with exact opaque state, successful compact on Chat/Anthropic
configured providers, unsupported responses only after dispatch, provider auth,
malformed success, and terminal/nonterminal upstream errors. The section runs in
`--mode=all`. Additional SDK/router tests cover Codex JSON path/auth, cancellation,
auth/team middleware, model aliasing, raw replay fields, and response validation.
These are controlled upstream contracts, not live provider acceptance or summary
quality tests.
