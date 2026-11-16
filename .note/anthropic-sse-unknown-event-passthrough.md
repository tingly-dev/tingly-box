# anthropic-sdk-go: unknown SSE event names are now yielded

Source: anthropic-sdk-go `3a8f9bb` (landed between v1.80.0 and v1.84.0, bumped
2026-10-21). Previously the SDK's SSE decoder silently dropped event names it
didn't recognize; now it yields them to the consumer instead.

## Why this matters for tingly-box

tingly-box has ~20 files that `switch` on Anthropic stream event `.Type`
(assemblers, converters, bridges, recorder, forwarding) — see
`internal/protocol/assembler/`, `internal/protocol/stream/`,
`internal/protocol/stage/{anthropicbridge,openaibridge,responsesbridge,
sdkstream,toolround,upstream}/`, `internal/recording/`,
`internal/forwarding/anthropic.go`, `internal/toolengine/
generic_stream_interceptor.go`. If Anthropic ships a new event type in the
future, these switches will now see it where before the SDK would have eaten
it silently.

Spot-checked `internal/protocol/assembler/anthropic_assembler.go`: its
`switch event.Type` blocks have no `default:` panic — unmatched types
no-op. That's the safe shape, but it was only spot-checked; the other ~19
call sites were not audited.

## Action item (not yet done)

Audit all Anthropic event-type switches for a safe (no-op or explicit log,
never panic) default path. Low urgency — Anthropic hasn't actually shipped a
genuinely new event type yet, this just means we'd now see it if they did.
