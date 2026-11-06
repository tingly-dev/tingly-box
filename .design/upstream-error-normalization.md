# Upstream error normalization

Tracking issue: tingly-dev/tingly-box#2245.

## Problem

The vendored OpenAI SDK (`libs/openai-go/internal/requestconfig`, around
`StatusCode >= 400`) takes `gjson.Get(body, "error")` and decodes it straight
into `apierror.Error`; the Anthropic SDK decodes the whole body into its own
`apierror.Error`. Any other shape fails with a Go JSON error
(`json: cannot unmarshal string into Go value of type apierror.Error`) and the
real status and message are lost. Seen bodies: `{"error":"text"}`,
`{"detail":"text"}` / `{"detail":[{"msg":..}]}`, `{"message":"text"}`, plain
text, HTML error pages, empty bodies. The sources are upstreams, proxies and
TB's own gateway (`/tingly/:scenario`, which a probe goes through).

## Decisions

1. **Normalise at the transport, not in the SDK forks.** `errorEnvelopeTransport`
   (`internal/client/error_envelope_transport.go`) is the outermost layer of
   every OpenAI and Anthropic client built by `newOpenAIClientWithTransport` /
   `newAnthropicClientWithTransport`. For a status >= 400 it reads the body and
   leaves it alone only if the SDK's own error type decodes it from an `error`
   object; otherwise it rewrites it to the envelope the SDK expects
   (`{"error":{"message","type"}}`, or `{"type":"error","error":{...}}` for
   Anthropic). The message comes from `error` / `message` / `detail` / `msg` /
   `error_description` / `errors` (strings, objects, lists), else the stripped
   text body, else the HTTP status text. 2xx responses, content-encoded bodies
   and bodies over 1 MiB pass through. Doing it in tingly-box avoids bumping a
   fork submodule for every fix.
2. **The gateway answers refusals in the envelope too.** `contextMiddleware`
   scenario/profile refusals and the two `anthropic_message.go` marshal errors
   were `{"error":"text"}`; they are `{"error":{"message","type"}}` now
   (`success:false` is kept on the scenario refusals).
3. **`team:<id>` is a team scope, not a profile.** The Team page probes under the
   scenario `team:<id>` (`db.TeamScenario`). `contextMiddleware` used to look the
   suffix up as a profile and answer 400 `unknown profile`; it now checks
   `Config.HasTeam`. Sharing keys are unaffected: they may only use the bare
   `team` scenario (`sharingKeyCanAccessModelSurface`), so only the global model
   token can address `/tingly/team:<id>`.
4. **The probe endpoint shows the upstream's words.** `POST /api/v2/probe`
   reports an SDK API error through `protocol.UpstreamMessage`
   (`Upstream error: 401 Unauthorized: invalid api key`) instead of the SDK's
   bare `OpenAI API error: 401 Unauthorized`. Transport/internal errors keep
   their raw text, which is what diagnoses them.

## Tests

- `internal/client/error_envelope_transport_test.go`: every body shape x
  OpenAI (+stream) / Anthropic keeps status and message.
- `internal/protocolserver/routes_middleware_test.go`: team scope accepted,
  unknown team refused, refusals are envelopes.
- `internal/servertest/team_probe_test.go`: probe through a Team rule whose
  upstream answers `{"error":"..."}` against the real gateway, via the prober
  and via `/api/v2/probe`.

## Not covered

Mid-stream SSE `error` events and the Google (genai) client are untouched.
