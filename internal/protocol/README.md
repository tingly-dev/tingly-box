# `internal/protocol` — request & response translation

This package owns the **wire-level translation layer** between the API
styles tingly-box accepts from clients and the API styles it forwards to
upstream providers. Every request that crosses the gateway flows through
something in here.

Two ideas drive the design:

- **APIStyle** — what a *provider* speaks. One of `openai`, `anthropic`,
  `google`. Discovered from provider config.
- **APIType** — the canonical *protocol shape* we normalize a request to
  before forwarding. One of:
  - `TypeOpenAIChat` (`/v1/chat/completions`)
  - `TypeOpenAIResponses` (`/v1/responses`)
  - `TypeAnthropicV1` (Anthropic Messages, the "stable" SDK type)
  - `TypeAnthropicBeta` (Anthropic Messages Beta, structural superset of v1)
  - `TypeGoogle` (Gemini `generateContent`)

A client speaks one APIType in, the dispatcher picks a target APIType
based on the resolved provider's APIStyle, the transform chain converts
the request, and a matching response-side converter turns the upstream
reply back into the client's APIType.

## Subpackage layout

| Path | Purpose |
| --- | --- |
| `types.go`, `anthropic.go`, `openai.go`, `google.go` | shared types, helpers, and re-exports (the public canonical types live in `ai/`) |
| `request/` | **request-side** converters: `<source>_to_<target>.go` files that turn a parsed request of one APIType into another |
| `transform/` | the **transform chain** that wires request converters, consistency rules, and vendor quirks together; see below |
| `stream/` | **streaming response** converters — SSE in, SSE out, one file per `(upstream type → client type)` pair |
| `nonstream/` | **non-streaming response** converters — same pairing, JSON in, JSON out |
| `assembler/` | builders that turn a captured stream of upstream events into a single non-stream response object (used for recording, replay, and the "assemble then forward" path used by some Codex-style providers) |
| `sse/` | low-level SSE framing helpers shared by `stream/` |
| `token/` | token usage accounting helpers shared across converters |
| `test/` | shared test fixtures and golden data |

`internal/server/protocol_dispatch.go` is the entrypoint that walks the
`(SourceAPI, TargetAPI)` matrix and picks the right converter pair.

## The transform chain

`transform.NewTransformChain` runs three transforms in order
(see `internal/server/chain_builder.go`):

1. **BaseTransform** (`transform/base.go`) — protocol conversion.
   Switches on `targetType` and calls the right `request/` converter to
   reshape `ctx.Request` from the source APIType into the target APIType.
   This is the only step that changes the request's Go type.
2. **ConsistencyTransform** (`transform/consistency.go`) —
   cross-provider normalization that applies to every provider of a
   given target type: tool schema cleanups (`type: "object"`, properties
   shape), tool_call_id truncation, scenario flag application
   (disable-stream-usage, thinking mode), bounds checks.
3. **VendorTransform** (`transform/vendor.go`) — provider-specific
   quirks keyed on the provider URL: DeepSeek's
   `x_thinking → reasoning_content` rewrite, Moonshot's reasoning shape,
   Codex's session-id handling, etc.

Optional pre/post recording transforms wrap the chain when scenario
recording is enabled.

## Supported `(source → target)` matrix

`request/` and the response converters cover the following pairs.
Anything not listed is intentionally unsupported and returns an
`unsupported request type` error from `BaseTransform`.

| source ↓  /  target → | `openai_chat` | `openai_responses` | `anthropic_v1` | `anthropic_beta` | `google`  |
| --------------------- | :-----------: | :----------------: | :------------: | :--------------: | :-------: |
| `openai_chat`         |   ✓ (pass)    |         ✓          |       —        |        ✓         |     ✓     |
| `openai_responses`    |       ✓       |      ✓ (pass)      |       —        |        ✓         |     —     |
| `anthropic_v1`        |       ✓       |         ✓          |    ✓ (pass)    |        —         |     ✓     |
| `anthropic_beta`      |       ✓       |         ✓          |       —        |     ✓ (pass)     |     ✓     |
| `google`              |       —       |         —          |       —        |        —         | ✓ (pass)  |

- `✓ (pass)` means same-type passthrough — `BaseTransform` is a no-op
  and the request is forwarded as-is.
- The `—` cells in the `anthropic_v1` target column are the subject of
  the design concern below.
- The harness validation matrix (`internal/protocoltest.DefaultPairs`)
  lists every supported pair explicitly. New rows here should be added
  there too so they get end-to-end coverage.

## Design concern: Anthropic Beta is the *single* normalization target for non-Anthropic sources

Anthropic publishes two SDK shapes for the Messages API: stable v1
(`anthropic.MessageNewParams`) and beta
(`anthropic.BetaMessageNewParams`). Beta is a **structural superset** of
v1 — every v1 field exists on beta, and beta adds extras (extended
thinking variants, more tool block types, MCP/server-tool blocks, etc.).

We used to normalize toward both targets, depending on the source:

- `OpenAIChat → Anthropic` picked `TypeAnthropicBeta`.
- `OpenAIResponses → Anthropic` picked `TypeAnthropicV1`.

That asymmetry forced the codebase to carry **two parallel converter
families** for the same destination (a v1 family and a beta family of
`Convert*ToAnthropic*Request` helpers, plus a Beta→V1 projection helper
that did nothing but downshape Beta back to v1 so the asymmetric path
could compile). The behavior was identical at the wire — Anthropic
providers accept both — so the asymmetry was pure busywork.

The current rule is:

> **Non-Anthropic source → Anthropic provider always normalizes to
> `TypeAnthropicBeta`.** `TypeAnthropicV1` as a target exists only for
> Anthropic V1 → Anthropic V1 passthrough.

Concretely:

- `convertToAnthropicV1` in `transform/base.go` accepts only v1 input
  (passthrough) and v1-from-beta is rejected as incompatible. Anything
  else returns `unsupported request type ... non-Anthropic sources must
  target Anthropic beta`.
- `convertToAnthropicBeta` is the single funnel for OpenAI Chat,
  OpenAI Responses, and (eventually) Google sources targeting an
  Anthropic provider.
- The dispatch switch in `internal/server/protocol_dispatch.go` mirrors
  this: the `TypeAnthropicV1` arm is a one-liner passthrough; the
  `TypeAnthropicBeta` arm fans out by `SourceAPI` to the right
  cross-format handler.

### What this rules in and out

- **In**: a single conversion path per `(non-Anthropic source, Anthropic
  target)` pair. Adding a new field handling means editing one
  converter, not two.
- **In**: Beta-typed response shaping
  (`buildResponsesPayloadFromAnthropicBeta`, the Beta stream handler in
  `stream/anthropic_beta_to_openai_responses.go`) is the only Anthropic
  → non-Anthropic response converter we maintain.
- **Out**: there is no path for "force a non-Anthropic request through
  v1 specifically". If a provider rejects a Beta-only field we'd need
  to either strip the field in `ConsistencyTransform` or add a
  Beta→Beta normalization step — *not* re-introduce a parallel v1
  pipeline.

## Adding a new conversion

1. Pick a source and target APIType. Add a converter under `request/`
   named `<source>_to_<target>.go` and wire it into the corresponding
   `convertTo<Target>` switch in `transform/base.go`.
2. Add the response-side converter(s):
   - non-streaming under `nonstream/`,
   - streaming under `stream/`.
3. If the dispatch matrix changes, update the inner switch in
   `internal/server/protocol_dispatch.go` for the new
   `(SourceAPI, TargetAPI)` case.
4. Add a row to the matrix above and an entry to the e2e test in
   `transform/e2e_test.go`. Unsupported pairs should hit the default
   error branch in `BaseTransform` whose message matches the test's
   `"unsupported request type"` predicate, so the test will report them
   as `NOT SUPPORTED` automatically.
5. Run `go test ./internal/protocol/...` and
   `./harness matrix` (built from `cli/harness`) to confirm the
   conversion matrix is still green.
