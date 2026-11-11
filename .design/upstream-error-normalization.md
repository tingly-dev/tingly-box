# Upstream error normalization

Tracking issue: tingly-dev/tingly-box#2245.

## Problem

The OpenAI SDK (`libs/openai-go/internal/requestconfig`, around
`StatusCode >= 400`) decodes the `error` value of a failed response straight
into `apierror.Error`. When that value is a JSON string (`{"error":"text"}`)
the decode fails with `json: cannot unmarshal string into Go value of type
apierror.Error` and the HTTP status is lost. Measured on the bare SDKs against
12 body shapes (`error` object, `error` string, `detail` string/list,
`message`, `error:null`, JSON string body, plain text, HTML, empty): the
OpenAI SDK fails only on the `error`-string shapes; the Anthropic SDK handles
all of them (right status, body kept in the message). Sources of
`{"error":"text"}`: upstreams, proxies and TB's own gateway.

## Decisions

1. **The gateway answers in the envelope.** `contextMiddleware` refusals and the
   two marshal errors in `anthropic_message.go` were `{"error":"text"}`; they
   are `{"error":{"message","type"}}` now (`success:false` is kept on the
   scenario refusals). This removes TB's own source of the error.
2. **`team:<id>` is a team scope, not a profile.** The Team page probes under the
   scenario `team:<id>` (`db.TeamScenario`). `contextMiddleware` used to look the
   suffix up as a profile and answer 400 `unknown profile`, which is what Test
   all on a non-default team ran into; it now checks `Config.HasTeam`. Sharing
   keys are unaffected: they may only use the bare `team` scenario
   (`sharingKeyCanAccessModelSurface`).
3. **A 20-line transport covers third-party upstreams.** `stringErrorTransport`
   (`internal/client/error_envelope_transport.go`) sits on the OpenAI client and
   rewrites only `{"error":"text"}` to `{"error":{"message":"text"}}` on a
   status >= 400. Everything else passes through byte for byte. It is not
   mounted on Anthropic clients, which need nothing. The alternative is a
   3-line fallback in the openai-go fork; it was not taken to avoid a fork
   commit plus a submodule bump.
4. **The probe endpoint shows the upstream's words.** `POST /api/v2/probe`
   reports an SDK API error through `protocol.UpstreamMessage`
   (`Upstream error: 401 Unauthorized: invalid api key`) instead of the SDK's
   bare `OpenAI API error: 401 Unauthorized`. Transport/internal errors keep
   their raw text.
5. **The Codex client reports an in-stream content-policy block as an upstream
   400 (#2417).** `codexEventDecoder` (`internal/client/codex_stream_errors.go`)
   turns the backend's in-stream `error` event into the `*openai.Error` that an
   HTTP 400 with the same body produces, so status mapping, client message and
   server log treat it as an upstream 400. Unlike `stringErrorTransport`, it
   rewrites no bytes. Details: `.design/codex.md` §6.

## Tests

- `internal/client/error_envelope_transport_test.go`: 12 body shapes x OpenAI
  (+stream) / Anthropic never produce a decode error and keep the status; string
  errors keep their text.
- `internal/protocolserver/routes_middleware_test.go`: team scope accepted,
  unknown team refused, refusals are envelopes.
- `internal/servertest/team_probe_test.go`: probe through a Team rule whose
  upstream answers `{"error":"..."}` against the real gateway, via the prober
  and via `/api/v2/probe`.

## Not covered

`detail` / `message` / plain-text bodies keep only status + status text on the
OpenAI SDK (no decode error). Mid-stream SSE `error` events and the Google
(genai) client are untouched, except content-policy blocks from a Codex OAuth
upstream (decision 5). The same block relayed by another Responses upstream
goes through the generic OpenAI client and is not recognized.
