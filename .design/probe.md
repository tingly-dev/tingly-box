# Probe Subsystem

## Overview

The probe subsystem provides two diagnostics for different user questions:

- **Lightweight** (`../internal/probe/light_probe.go`): the Connect AI **Test Connection** path for an inline, not-yet-saved provider config. It calls the upstream directly and returns an advisory endpoint matrix: OPTIONS, models, and (for OpenAI-style providers) non-streaming Chat and Responses checks. OPTIONS is raw HTTP; models and completion checks reuse the production client/SDK methods. It does not enter `E2EProber`, TB loopback, routing, or rule evaluation.
- **E2E** (`../internal/probe/e2e_probe.go`): the Probe dialog / troubleshooting path. It performs a full SDK round-trip for a saved provider or rule, normally through TB's loopback and production routing path. This catches provider quirks and TB middleware/routing failures that a direct connectivity check cannot distinguish.

## Product entry points

| User surface | Frontend call | HTTP endpoint | Backend strategy | Question answered |
|--------------|---------------|---------------|------------------|-------------------|
| Connect AI → Test Connection | `runProviderProbe` → `api.probeProviderLightweight` | `POST /api/v2/probe/lightweight` | `LightProber` | Are these credentials/endpoints reachable enough to continue? |
| Probe dialog / Troubleshoot | `runProbe` | `POST /api/v2/probe` | `E2EProber` | Does a real request work, how did it route, and what came back? |
| Probe dialog → cURL section | `buildProbeCurl` | `POST /api/v2/probe/curl` | `E2EProber.BuildCurl` | What exact curl reproduces this probe? |

The similarly named frontend helper `api.probeProvider` targets the E2E
`provider_config` API, but Connect AI does not call it. Keep the two paths
distinct: onboarding connectivity is advisory and direct; troubleshooting must
exercise the real TB path.

## E2E Target Types

An `E2ERequest` has three `target_type` values:

| `target_type`     | What it tests                                                                |
|-------------------|------------------------------------------------------------------------------|
| `provider`        | A saved provider record by UUID, pinned to a specific model                  |
| `rule`            | A rule by UUID — exercises all TB middleware for that rule's scenario         |
| `provider_config` | An inline provider config for direct E2E API callers; it does not represent Connect AI's Test Connection path |

### Direct vs Through-TB (provider probes)

Provider probes have two modes controlled by `E2ERequest.Direct`:

| Mode             | `direct` field | What it does                                                        | Use case                                               |
|------------------|----------------|---------------------------------------------------------------------|--------------------------------------------------------|
| Through-TB (default) | `false`    | Routes through `http://localhost:{port}/tingly/{scenario}` loopback | Tests the full TB pipeline — flags, routing, middleware |
| Direct           | `true`         | Calls the upstream SDK without any loopback                         | Isolates whether a failure is upstream vs TB-internal  |

When a through-TB probe fails and a direct probe succeeds, the problem is in TB's middleware stack. When both fail, the upstream itself is the cause. This is the primary diagnostic value of the distinction.

### Test axes

The request shape is described by orthogonal fields; the legacy `test_mode` enum (`simple`/`streaming`/`tool`) mixed two axes and is kept only as a compatibility spelling (it wins when present): `simple` → off/off, `streaming` → on/off, `tool` → off/on (tool mode historically takes the non-stream path so structured `tool_calls` come back).

| Axis | Field | Notes |
|------|-------|-------|
| Shape (stream) | `stream bool` | SSE vs single response |
| Tool | `tool bool` | attaches probe tools; composes with both stream values (non-stream lifts structured `tool_calls`; stream keeps raw chunks) |
| Thinking | `thinking` | `none`/`low`/`medium`/`high`, unchanged |
| Vision | `vision` | `none`/`user`/`tool` — attaches the canonical image fixture (`internal/protocol/vision`, a 256×256 red PNG + "what color?" prompt) in the user message or as a synthetic tool-result turn: the two channels of issue #1606. A vision-capable route answers "red"; anything else reveals a drop or corruption. Drops the echo instruction (it would echo the prompt instead of answering). Not supported for Google targets. |
| Protocol | `protocol` | `openai_chat` / `openai_responses` / `anthropic_v1` — no "auto"; empty = target's primary (provider APIStyle, Codex OAuth → Responses). Replaces the OpenAI-only legacy `endpoint` field (still accepted; `protocol` wins). Not allowed for rule targets (scenario fixes it). |
| Scope | `direct bool` | unchanged |

Resolution helpers live on `E2ERequest` (`ResolveAxes`, `ResolveClientStyle`, `ResolveOpenAIEndpointOverride`); the SDK helpers read flat `probeParams{Stream, Tool, Thinking}` booleans and never branch on the wire enum.

Protocol override behavior: for **direct** probes a dual-base provider's matching URL is selected via `Provider.ResolveStyle`; for **through-TB** probes the loopback speaks the requested protocol (loopback scenario = the protocol family's canonical one when overridden) and TB's transform pipeline converts to the upstream exactly as production traffic does.

### cURL generation

`POST /api/v2/probe/curl` takes the same body as `/api/v2/probe` and returns `probe.CurlData` (`command`, `method`, `url`, `headers`, `body`, `key_env_var`) **without executing anything**. It resolves the target through the same `resolveTargetToProviderModel` path and marshals the *same* param builders the SDK helpers use (`buildOpenAIChatParams` / `buildOpenAIResponsesParams` / `buildAnthropicMessageParams` in `helper.go`) — the constructed body cannot drift from a real probe. The `stream: true` member the SDKs inject via `WithJSONSet` is added by `marshalStreamAware`. Secrets are never embedded: `$TB_API_KEY` for through-TB curls (loopback URL + gateway-key header), `$UPSTREAM_API_KEY` for direct ones. Google targets are rejected explicitly.

The rendered command puts the **URL first**, then headers, then the body; the payload is pretty-printed and **single-quoted** (`shellQuote`, embedded quotes escaped as `'\''`) so the JSON reads verbatim. `CurlData.Body` stays compact for programmatic use.

## TB Loopback Pattern

Provider (non-direct) and rule probes route through TB's own HTTP endpoint (`http://localhost:{port}/tingly/{scenario}`) rather than going directly to the upstream API. This ensures that rule flags (`openai_endpoint_override`, thinking effort, etc.), smart routing, and load balancing all execute exactly as they would for production traffic.

```
Probe code
  → SDK client (probeHeaderRoundTripper + captureRoutingRoundTripper)
    → TB loopback /tingly/{scenario}/chat/completions (or /messages)
      → determineRuleWithScenario (reads X-Tingly-Probe-* headers)
        → SimpleSelector.SelectService (pins service or runs normal pipeline)
          → responds with X-Tingly-Selected-* headers
        → upstream provider
    ← response headers captured → ProbeResult.RoutingTrace fields
```

### URL conventions

`loopbackAPIBase(port, scenario)` delegates to `ScenarioEndpoint(scenario)` for the canonical `/tingly/{scenario}` path — no `/v1` suffix. TB registers both `/tingly/:scenario` and `/tingly/:scenario/v1` with identical handlers, so each SDK appends its own operation path (`/chat/completions`, `/messages`) without needing the prefix to carry a version segment.

`resolveRuleTarget` prefers the request's `scenario` (the page's scenario, which may carry a profile suffix like `claude_code:p1`), falling back to `rule.Scenario` then OpenAI, and passes it to `loopbackAPIBase` — so the loopback hits the exact `/tingly/{scenario}` endpoint including any profile. `ScenarioEndpoint` keeps the full scenario in the path but resolves the api-style from the *base* scenario (via `ParseScenarioProfile`), so `claude_code:p1` still maps to the Anthropic SDK. If `ServerPort == 0` (unknown), it returns an error rather than falling back to direct (rule probes have no meaningful fallback).

`resolveProviderTarget` calls `defaultScenarioForAPIStyle(provider.APIStyle)` to get the canonical scenario for the provider, then passes it to `loopbackAPIBase`. Google providers and the `port == 0` case fall back to direct SDK calls.

Virtual model providers (`provider.IsVirtual()`) are also resolved to the TB loopback via `resolveVModelLoopbackTarget`, sharing the same `loopbackAPIBase` helper.

## Probe Headers (outgoing)

Two request headers let the probe subsystem control TB routing without modifying the stored rule or provider configuration.

### `X-Tingly-Probe-Service: {provider_uuid}:{model}`

Injected by `resolveProviderTarget` on the SDK client transport. Two TB layers consume it:

1. **`determineRuleWithScenario`** (handlers.go): If no `X-Tingly-Probe-Rule` header is present, builds a minimal synthetic `typ.Rule` wrapping the pinned service so the handler has a rule to work with.
2. **`SimpleSelector.SelectService`** (routing/simple.go): Bypasses the affinity → smart routing → load balancer pipeline and returns the pinned provider+model directly.

### `X-Tingly-Probe-Rule: {rule_uuid}`

Optionally injected by callers that want to apply a specific rule's flags while overriding service selection via `X-Tingly-Probe-Service`. `determineRuleWithScenario` loads the named rule and returns it; the `SelectService` probe pin still applies.

### `X-Tingly-Debug-Routing: 1`

Always injected by loopback probes (both provider and rule). Causes `SimpleSelector.SelectService` to append routing-decision headers to the response (see below).

### Request shaping (`system`, `messages`, `client`)

- `E2ERequest.System` / `Messages` replace the fixture prompt / single message in the
  three param builders; empty keeps today's behaviour. A mid-conversation `system` turn is
  sent verbatim (it is the claude_code_compat test subject); the last turn must be `user`.
- `E2ERequest.Client = "claude_code"` sends the loopback request **as Claude Code**: the
  probe builds its SDK client with `client.NewClaudeClient` — the same constructor and
  request guard TB uses to impersonate Claude Code toward Anthropic — pointed at the
  loopback, so headers (user-agent, anthropic-beta, x-app, stainless, `?beta=true`),
  the `X-Claude-Code-Session-Id` header, the disabled-thinking default and tool-name
  conventions are whatever that client emits; nothing is listed twice. The probe adds
  what the real CLI puts in the body and TB's upstream client does not: the identity
  block with its cache breakpoint, the billing block (`x-anthropic-billing-header:`,
  which clean_header strips) and a `metadata.user_id` session key (which the client
  guard requires and session affinity buckets on). Through-TB and Anthropic only.
  `BuildCurl` renders it by capturing the request from that client via an SDK middleware
  instead of re-composing headers.

## Routing Trace (response headers → ProbeResult)

When `X-Tingly-Debug-Routing: 1` is present, the routing decision is emitted across two chokepoints.

**Selection stage** — `SimpleSelector.SelectService` (routing/simple.go):

| Header                          | Content                                      |
|---------------------------------|----------------------------------------------|
| `X-Tingly-Selected-Provider`    | Provider name                                |
| `X-Tingly-Selected-Provider-UUID` | Provider UUID                              |
| `X-Tingly-Selected-Model`       | Model name actually used                     |
| `X-Tingly-Routing-Source`       | `affinity`, `smartrouting`, `load_balancer`, or `probe_pin` |
| `X-Tingly-Matched-Smart-Rule`   | Index of matched smart rule (omitted if none) |

**Dispatch stage** — `setProbeUpstreamHeaders` in `dispatchChainResult` (protocol_dispatch.go), the single point where the resolved upstream API + matched rule + applied flags are all known, before any response byte is written:

| Header                        | Content                                                        |
|-------------------------------|---------------------------------------------------------------|
| `X-Tingly-Upstream-API`       | Resolved upstream API type (`openai_chat`, `openai_responses`, `anthropic_v1`, …) — answers chat-vs-responses |
| `X-Tingly-Upstream-URL`       | Real upstream endpoint TB forwarded to (`provider.APIBase` + path) |
| `X-Tingly-Matched-Rule`       | Matched rule UUID (omitted for synthetic provider probes)     |
| `X-Tingly-Matched-Rule-Desc`  | Matched rule description, percent-encoded (decoded probe-side) |
| `X-Tingly-Applied-Flags`      | Compact non-default flags, e.g. `endpoint=responses, thinking=high` |

`captureRoutingRoundTripper` (`client.ApplyRoutingCaptureToClient`) is layered on the probe client transport. After the SDK call completes, `applyRoutingCapture` copies these into `ProbeResult`:

```go
ProbeResult.SelectedProvider     // provider name
ProbeResult.SelectedProviderUUID // provider UUID
ProbeResult.SelectedModel        // model
ProbeResult.RoutingSource        // how the service was selected
ProbeResult.MatchedSmartRule     // smart rule index (-1 = none)
ProbeResult.UpstreamAPI          // resolved upstream API type
ProbeResult.UpstreamURL          // real upstream endpoint
ProbeResult.MatchedRule          // matched rule UUID
ProbeResult.MatchedRuleDesc      // matched rule description (decoded)
ProbeResult.AppliedFlags         // compact applied-flags string
```

The frontend dialog renders these as the "请求旅程" (request journey): Rule → Flags → Routing → Provider→Model → Endpoint → Upstream URL.

Direct probes (`req.Direct = true`) skip the loopback entirely, so these fields are empty.

## Transport wiring

Probe headers are stored in the `context.Context` via `client.WithProbeHeaders(ctx, headers)`. `probeHeaderRoundTripper` reads the context on every `RoundTrip` and injects the headers into outgoing requests.

`captureRoutingRoundTripper` wraps the same transport chain and reads routing headers from each response.

Neither round tripper is installed on production clients. `probeProviderWithSDK` calls `ApplyProbeHeadersToClient` and `ApplyRoutingCaptureToClient` only when `GetProbeHeaders(ctx)` returns true.

## Code layout

```
internal/probe/
  types.go          — Result (= E2EData): Success/Content/Usage/ToolCalls + routing trace;
                      E2ERequest (Stream/Tool/Protocol axes + legacy test_mode/endpoint),
                      E2ETarget, ProbeProtocol, ValidateE2ERequest, ResolveAxes *,
                      ScenarioEndpoint(), toProbeResult
  e2e_probe.go      — E2EProber: resolveTargetToProviderModel, loopbackAPIBase,
                      probeProviderWithSDK, applyRoutingCapture, e2eMessage
  helper.go         — probeParams + shared param builders (buildOpenAIChatParams /
                      buildOpenAIResponsesParams / buildAnthropicMessageParams) and the
                      SDK dispatch helpers (probeOpenAIChat, probeOpenAIResponses,
                      probeAnthropicMessages, probeGoogleGenerate, probeOptions) with
                      per-provider usage extraction (via internal/protocol/usage)
  curl.go           — CurlData, E2EProber.BuildCurl, marshalStreamAware, shellQuote,
                      renderCurl — construct-only cURL generation sharing the param
                      builders above
  light_probe.go    — LightProber (direct advisory connectivity matrix; reuses
                      SDK clients for models/chat/responses)
  probetools.go     — Tool definitions used by tool-axis probes
  endpoint_probe_cache.go — narrow direct-endpoint capability cache

internal/protocol/usage/
  extract.go        — FromOpenAIChatCompletion / FromOpenAIResponses / FromAnthropicMessage:
                      the canonical TokenUsage extractors reused by the SDK probes

internal/client/
  http.go           — probeHeadersKey, WithProbeHeaders, GetProbeHeaders,
                      probeHeaderRoundTripper, ApplyProbeHeadersToClient
                      RoutingCapture, captureRoutingRoundTripper, ApplyRoutingCaptureToClient

internal/protocolserver/
  protocol_handler.go   — determineRuleWithScenario: X-Tingly-Probe-Rule /
                          X-Tingly-Probe-Service handling

internal/routing/
  simple.go             — SelectService: X-Tingly-Probe-Service pin +
                          X-Tingly-Debug-Routing response headers

internal/server/module/probe/
  handler.go / routes.go — HTTP endpoints: /api/v2/probe, /api/v2/probe/curl,
                          /api/v2/probe/lightweight (+ swagger registration)

frontend/src/components/probe/
  probeConfig.ts    — ProbeAxes model, open-time association chain (props →
                      initialResult.stream → persisted per-target-type config →
                      defaults), per-target protocol availability
  ProbeControls.tsx — control rail (primary axes + Advanced expander)
  ProbeDialog.tsx   — rail + results + full-width cURL section, CopyBlock
  runProbe.ts       — runProbe / buildProbeCurl envelopes
```

## Result fields

`Result` (returned as `E2EData`) carries, for one SDK round-trip:

- `success`, `content` (raw marshaled upstream response — object for non-stream,
  chunk array for stream), `latency_ms` (pure upstream call time, measured by the
  SDK probe — the HTTP handler does not overwrite it), `error_message`.
- `stream` — true for streaming probes (explicit; the caller's `stream` axis is
  the other source of truth).
- `usage` — normalized `*protocol.TokenUsage` parsed via `internal/protocol/usage`
  for OpenAI Chat / Responses and Anthropic (non-stream always; stream when the
  provider emits a final usage block). Uses the canonical `protocol.TokenUsage`
  shape (`input_tokens` / `output_tokens` / `cache_read_tokens` /
  `cache_write_tokens` / `reasoning_tokens`) — the same vocabulary the rest of
  TB emits and the frontend renders; there are no parallel flat token fields.
  `nil` for Google (out of scope) and cache hits.
- OpenAI Chat requests set `stream_options.include_usage` only on the streaming
  branch. It is a stream-only parameter and must not be sent by non-streaming or
  lightweight Chat checks because strict OpenAI-compatible providers may reject it.
- Tool handling depends on the stream axis: **non-stream** tool probes lift
  structured `tool_calls` out of the response; **stream** tool probes keep the
  raw chunk array as the diagnostic artifact and do not assemble a second
  representation.
- Routing trace fields (see below) — populated for TB-loopback probes only.

## Trade-offs and constraints

- **Google probes go direct**: The TB loopback only exposes `/tingly/openai` and `/tingly/anthropic` endpoints. Google uses its own SDK and has no matching loopback route, so `resolveProviderTarget` returns the original provider record for Google.
- **Rule probe requires a running server**: `resolveRuleTarget` fails fast if `ServerPort == 0`. There is no direct fallback for rule probes because the whole point is to exercise TB middleware.
- **Probe headers are not authenticated**: Any caller that can reach the TB HTTP port can send `X-Tingly-Probe-Service` and bypass load balancing. This is intentional — probe endpoints are admin-only behind TB's own auth layer.
- **`probe-synthetic` rule UUID**: The synthetic rule created from `X-Tingly-Probe-Service` (when no probe rule header is present) carries `UUID: "probe-synthetic"`. This is a sentinel value, not a persisted rule; it exists only for the duration of the request.
- **Routing trace is empty for direct and provider_config probes**: Only loopback probes emit `X-Tingly-Selected-*` headers. Direct probes and `provider_config` probes have no routing pipeline, so those fields stay empty.
