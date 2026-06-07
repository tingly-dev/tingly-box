# Probe Subsystem

## Overview

The probe subsystem performs SDK-level end-to-end connectivity tests for providers and rules. There are two probe strategies:

- **Lightweight** (`internal/probe/lightweight.go`): HTTP-level checks (OPTIONS, `/models`, `/chat/completions`) with no SDK. Used during provider onboarding to validate credentials quickly.
- **E2E** (`internal/probe/e2e.go`): Full SDK round-trip using the same client methods as production traffic (ChatCompletionsNew, ResponsesNew, MessagesNew, GenerateContent). This catches provider quirks that only show up under the real code path.

## E2E Target Types

An `E2ERequest` has three `target_type` values:

| `target_type`     | What it tests                                                                |
|-------------------|------------------------------------------------------------------------------|
| `provider`        | A saved provider record by UUID, pinned to a specific model                  |
| `rule`            | A rule by UUID — exercises all TB middleware for that rule's scenario         |
| `provider_config` | An inline provider config (name, api_base, api_style, token) — used during onboarding before the provider is saved |

### Direct vs Through-TB (provider probes)

Provider probes have two modes controlled by `E2ERequest.Direct`:

| Mode             | `direct` field | What it does                                                        | Use case                                               |
|------------------|----------------|---------------------------------------------------------------------|--------------------------------------------------------|
| Through-TB (default) | `false`    | Routes through `http://localhost:{port}/tingly/{scenario}` loopback | Tests the full TB pipeline — flags, routing, middleware |
| Direct           | `true`         | Calls the upstream SDK without any loopback                         | Isolates whether a failure is upstream vs TB-internal  |

When a through-TB probe fails and a direct probe succeeds, the problem is in TB's middleware stack. When both fail, the upstream itself is the cause. This is the primary diagnostic value of the distinction.

### Test Modes

`test_mode` controls the shape of the probe request:

| Mode        | Description                                         |
|-------------|-----------------------------------------------------|
| `simple`    | Single non-streaming completion                     |
| `streaming` | Streaming completion (SSE)                          |
| `tool`      | Completion with a tool definition + auto tool choice |

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

## Routing Trace (response headers → ProbeResult)

When `X-Tingly-Debug-Routing: 1` is present, the routing decision is emitted across two chokepoints.

**Selection stage** — `SimpleSelector.SelectService` (routing/simple.go):

| Header                          | Content                                      |
|---------------------------------|----------------------------------------------|
| `X-Tingly-Selected-Provider`    | Provider name                                |
| `X-Tingly-Selected-Provider-UUID` | Provider UUID                              |
| `X-Tingly-Selected-Model`       | Model name actually used                     |
| `X-Tingly-Routing-Source`       | `affinity`, `smart_routing`, `load_balancer`, or `probe_pin` |
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

Neither round tripper is installed on production clients. `ProbeProviderWithSDK` calls `ApplyProbeHeadersToClient` and `ApplyRoutingCaptureToClient` only when `GetProbeHeaders(ctx)` returns true.

## Code layout

```
internal/probe/
  types.go        — E2ERequest (incl. Direct field) / E2EData / E2EMode / E2ETarget, ScenarioEndpoint()
  result.go       — ProbeResult (incl. routing trace fields)
  e2e.go          — E2EService: resolveTargetToProviderModel, loopbackAPIBase,
                    ProbeProviderWithSDK, applyRoutingCapture
  sdkprobe.go     — SDK dispatch helpers: probeOpenAIChat, probeAnthropicMessages, probeGoogleGenerate, …
  lightweight.go  — LightweightProbeService (HTTP-level, no SDK)
  probetools.go   — Tool definitions used by E2EModeTool

internal/client/
  http.go         — probeHeadersKey, WithProbeHeaders, GetProbeHeaders,
                    probeHeaderRoundTripper, ApplyProbeHeadersToClient
                    RoutingCapture, captureRoutingRoundTripper, ApplyRoutingCaptureToClient

internal/server/
  handlers.go     — determineRuleWithScenario: X-Tingly-Probe-Rule / X-Tingly-Probe-Service handling
  routing/
    simple.go     — SelectService: X-Tingly-Probe-Service pin + X-Tingly-Debug-Routing response headers
```

## Trade-offs and constraints

- **Google probes go direct**: The TB loopback only exposes `/tingly/openai` and `/tingly/anthropic` endpoints. Google uses its own SDK and has no matching loopback route, so `resolveProviderTarget` returns the original provider record for Google.
- **Rule probe requires a running server**: `resolveRuleTarget` fails fast if `ServerPort == 0`. There is no direct fallback for rule probes because the whole point is to exercise TB middleware.
- **Probe headers are not authenticated**: Any caller that can reach the TB HTTP port can send `X-Tingly-Probe-Service` and bypass load balancing. This is intentional — probe endpoints are admin-only behind TB's own auth layer.
- **`probe-synthetic` rule UUID**: The synthetic rule created from `X-Tingly-Probe-Service` (when no probe rule header is present) carries `UUID: "probe-synthetic"`. This is a sentinel value, not a persisted rule; it exists only for the duration of the request.
- **Routing trace is empty for direct and provider_config probes**: Only loopback probes emit `X-Tingly-Selected-*` headers. Direct probes and `provider_config` probes have no routing pipeline, so those fields stay empty.
