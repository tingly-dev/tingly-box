# vmodel transport — dispatch virtual providers over HTTP, not in-memory

> Status: **implemented** (in-memory listener). The unix-socket option (step 5
> below) is not built yet.

## Problem

A provider with `auth_type = vmodel` is the only provider kind that does **not**
go through an HTTP client. `ClientPool.GetOpenAIClient` / `GetAnthropicClient`
(`internal/client/pool.go`) return a pair of process-wide singletons from
`vmodel/client` that call the registry directly:

```
gateway → rule → vmodel provider → ClientPool.IsVirtual() short-circuit
        → vmodel/client (Go function call) → registry.Get(model).Handle*()
```

Every other provider looks like this:

```
gateway → rule → provider → ClientPool → official SDK → transport chain
        → http.Transport → TCP → upstream
```

The in-memory path was introduced so virtual providers could "traverse the same
dispatch path as real providers" (the SDK-level interfaces). In practice it is a
second implementation of the upstream and has drifted into a maintenance and
fidelity problem:

| Divergence | Where | Effect |
| --- | --- | --- |
| Error status is not reproduced. `injectedPreContentError` collapses any injected status (429 / 503 / 401 …) into a generic Go error that maps to 500 | `vmodel/client/inject.go` | Failover, health monitor, quota and retry logic cannot be exercised against the real status codes the virtualserver would send |
| Responses are rebuilt by hand (JSON round-trips, token estimates, tool-call shapes, SSE event sequencing) instead of coming off the wire | `vmodel/client/openai.go`, `vmodel/client/anthropic.go` (~830 lines) | Every protocol change in `virtualserver` must be re-implemented a second time; the two paths have already diverged (reasoning shape fix in `86aad15` touched only one side) |
| `Client()` returns `nil` | both clients | Any caller that reaches for the underlying SDK client (probes, model listers, future features) panics or silently no-ops for vmodel providers |
| Transport chain is skipped entirely | `pool.go` | `ruleFlagTransport` (custom UA, extra headers), logging round-tripper, probe headers, request timeout, proxy settings are all bypassed. A rule flag that works on a real provider is invisible on a vmodel provider |
| `APIBase = "vmodel://local"` is a fake value stored on the provider row | `vmodel/virtualserver/seed.go` | The UI shows an alias instead of a concrete, dialable value (UX principle 5); the probe has to special-case it (`internal/probe/e2e_probe.go`) |
| Provider identity is a dummy | `internal/server/server.go` (`vmodel-internal`) | `GetProvider()` on the client does not return the provider the rule resolved, so per-provider stats / logging keyed on the client's provider are wrong |

The root cause is architectural: **vmodel is a provider, and providers are
reached over HTTP.** Anything else is a fork of the dispatch pipeline that has to
be kept in sync forever.

## Decision

Make a vmodel provider indistinguishable from a real provider at the dispatch
layer: delete the `IsVirtual()` short-circuit and the `vmodel/client` package,
and reach the virtualserver through the **same SDK + transport chain** as every
other provider. The only vmodel-specific piece left is *where the bytes go*.

```
gateway → rule → vmodel provider → ClientPool → official SDK → transport chain
        → http.Transport{DialContext: vmodelDial} → virtualserver http.Server
```

Two things change and nothing else does:

1. **A real `http.Server` for the virtualserver.** `virtualserver.Service` already
   exposes gin routes and is already served over a real listener by
   `vmodel/benchmark.LocalServer`. The main server starts one such server at
   boot on a **local, unroutable listener** (below) — separate from the public
   gin engine, with no auth middleware, because the listener itself is the
   access control.
2. **A dialer for the `vmodel` provider kind.** The transport pool hands out an
   `*http.Transport` whose `DialContext` connects to that listener. Everything
   above the dialer (SDK, retries-off, timeout, rule flags, logging) is the
   standard chain.

### Listener choice

| Option | Wire | Needs a port | Works in tests without `Start()` | Reachable from outside the process |
| --- | --- | :---: | :---: | :---: |
| **A. In-memory `net.Listener`** (`net.Pipe`-backed, bufconn-style) | real HTTP/1.1 over an in-memory duplex stream | no | yes | no |
| B. Unix domain socket (`<configDir>/tingly-vmodel.sock`) | real HTTP/1.1 | no | yes | same host, filesystem-permission gated |
| C. Loopback TCP (`127.0.0.1:0`, port recorded like the runtime port file) | real HTTP/1.1 | yes | yes | same host, any user |
| D. `http.RoundTripper` calling `engine.ServeHTTP` directly | none (no serialization) | no | yes | no |

**Pick A as the default, with B as an opt-in for external access.**

- A is "the same as a real provider" at every layer that matters: bytes are
  serialized, SSE frames are flushed and parsed by the SDK's `ssestream`, status
  codes and headers travel as HTTP. There is no socket to bind, no port
  collision, nothing for a firewall or another user to hit, and nothing to clean
  up on crash. The cost is one goroutine per connection copying through a pipe,
  which is negligible next to the mock latency profiles vmodel already
  simulates.
- B is worth keeping as an option because it is the one that lets a **separate
  process** (a CLI harness, a subprocess agent under test, an external client)
  reach the exact same virtual provider without going through the gateway.
  Windows named-pipe support exists in Go but is not first class; keep it
  opt-in (`--vmodel-socket`) rather than default.
- C is rejected as the default: it exposes an unauthenticated endpoint on the
  host, and it couples vmodel to port-file bookkeeping for no benefit — the
  authenticated `/virtual/*` routes on the main engine already cover external
  HTTP access.
- D is rejected: it skips serialization, so it re-creates the fidelity gap we are
  removing (a `gin.ResponseWriter` without a real connection does not behave
  like one under streaming, cancellation, or `Content-Length`). If a test needs
  zero-copy speed it can keep constructing `httptest.Server` as it does today.

### Provider record

`APIBase` becomes a real, concrete URL scheme that the dialer recognizes:

```
vmodel://openai        →  virtualserver /openai/v1/...
vmodel://anthropic     →  virtualserver /anthropic/v1/...
```

The `vmodel://` scheme is what tells the transport pool to use `vmodelDial`;
the host part selects the protocol root (mirrors today's `/virtual/openai` and
`/virtual/anthropic` split so `/models` returns only dispatchable IDs). Before
the SDK sees it the client constructor rewrites the URL to
`http://vmodel.internal/<host>/v1` (`vmodelclient.HTTPBase`), so the SDKs
receive an ordinary http base URL; the transport's dialer ignores the
placeholder host and connects to the listener. Nothing else about the provider row changes:
`AuthType = vmodel`, `VModelDetail.Models`, builtin seeding and the UI stay as
they are. `VModelSentinelToken` is still sent as the bearer/x-api-key (the
virtualserver ignores it), so the SDK's non-empty-key check is satisfied
without a code path.

For opt-in B the same record works: `vmodel://openai` resolves to the unix
socket if configured, in-memory otherwise. The provider never needs to know.

### Auth

None on the private listener. It is reachable only by whoever holds the
`net.Listener` (in-memory) or can open the socket file (unix, `0600`). This is
the same trust boundary as the in-memory call it replaces. The public
`/virtual/*` routes on the main engine keep the model-auth middleware exactly as
today; they are unchanged by this design.

## What gets deleted

- The old `vmodel/client/` contents (`openai.go`, `anthropic.go`,
  `inject.go`, tests): hand-written SDK-interface implementations. The package
  now holds only the client side of the HTTP path (scheme, base URL,
  transport); the protocol work is done by the real SDKs.
- `ClientPool.SetVirtualClients` and the singleton short-circuit in
  `GetOpenAIClient` / `GetAnthropicClient`; the pool now dispatches
  `IsVirtual()` to a dedicated constructor like every other provider kind.
- The `vmodel-internal` dummy provider in `internal/server/server.go`.
- `resolveVModelLoopbackTarget` in `internal/probe/e2e_probe.go` — the probe can
  now dial a vmodel provider like any other provider (UX principle 7: the
  diagnostic traverses the real path instead of a rerouted one).
- `vmodelAPIBaseSentinel = "vmodel://local"` and the three test fixtures that
  copy it (`internal/protocoltest/failover.go`).

`IsVirtual()` itself stays: model-list resolution (`config/model_resolve.go`),
the provider API (`module/provider/handler.go`) and the UI still need to know a
provider is virtual. Only the *dispatch* special case goes away.

## Wiring

```
internal/server/server.go (NewServer)
  virtualModelService = virtualserver.NewService()          // unchanged
  vmodelServer = virtualserver.Serve(virtualModelService)   // server side: private in-memory listener
  vmodelclient.Connect(vmodelServer.DialContext)            // client side: Transport() now dials it
  virtualModelService.EnsureBuiltinProviders(store)         // seeds vmodel://openai|anthropic
  (Server.Stop disconnects and closes vmodelServer)

vmodel/virtualserver                                        // server side
  listener.go  memListener (net.Pipe-backed net.Listener)
  serve.go     Serve, Server.DialContext, Server.Close

vmodel/client                                               // client side
  apibase.go   Scheme, APIBase(style), IsAPIBase, HTTPBase   (vmodel://openai → http://vmodel.internal/openai/v1)
  transport.go Connect(dialer), Transport()                  (fails fast when not connected)

internal/client/pool.go                                     // dispatch, like Codex / Azure / Bedrock
  provider.IsVirtual() → NewVModelOpenAIClient / NewVModelAnthropicClient

internal/client/vmodel_client.go                            // wrap vmodel/client on the generic clients
  newOpenAIClientWithTransport / newAnthropicClientWithTransport with
  WithBaseURL(HTTPBase(...)) and the generic chain (rule flags → advisor
  loopback → logging) over Transport(); no pooled network transport is created
```

`virtualserver.Serve` is a small addition to the existing package, modeled on
`benchmark.LocalServer`: build the per-protocol router (`/openai/v1/*`,
`/anthropic/v1/*`), start `http.Server.Serve(listener)`, expose
`DialContext` and `Close`. The in-memory listener is ~60 lines
(`Accept` hands out one end of `net.Pipe`, `Dial` returns the other, closing
the listener unblocks `Accept`); no new dependency is needed.

Shutdown: `vmodelSrv.Close()` joins the existing server shutdown path next to
the PID/port-file cleanup.

## Consequences

**Wins**

- One implementation of the virtual upstream. Fixes land in `virtualserver` and
  are automatically what the gateway sees.
- Real status codes, real SSE framing, real timeouts — failover, quota, health
  and rule-flag tests against vmodel now test the production code path.
- Rule flags, probe headers, logging and recording work on vmodel providers with
  no extra code, because there is no special path left to forget them in.
- The UI shows a concrete, meaningful base URL instead of `vmodel://local`.
- Tests that construct `server.NewServer` without calling `Start()` (all of
  `protocoltest`, `servertest`) keep working: the in-memory listener does not
  depend on the public port.

**Costs**

- One extra HTTP serialize/deserialize per vmodel request. Measured against the
  benchmark package's own loopback numbers this is microseconds; vmodel latency
  profiles are milliseconds.
- `vmodel/client` was the only consumer of `protocol.OpenAIChatCompletionRequest`
  built from SDK params on the client side; that adapter goes with it.
- Tests that asserted on in-memory specifics (`client_test.go` in
  `vmodel/client`) are deleted, not migrated — the equivalent coverage already
  exists as `virtualserver/*_test.go` plus the gateway-level protocoltest
  matrix, which after this change exercises the same bytes.

## Migration

1. Add `virtualserver.Serve` + in-memory listener; unit-test it with the real
   OpenAI and Anthropic Go SDKs pointed at `DialContext` (non-stream, stream,
   injected 429).
2. Teach `TransportPool` the `vmodel://` scheme and add the base-URL rewrite in
   the two client constructors. Keep `SetVirtualClients` for one commit so both
   paths coexist.
3. Flip the seed to `vmodel://openai` / `vmodel://anthropic`. Existing rows with
   `vmodel://local` are rewritten by `EnsureBuiltinProviders` on next start
   (already idempotent and already refreshes the row); user-created vmodel
   providers with `vmodel://local` get the same one-line normalization in the
   provider-store load path.
4. Remove the short-circuit, `vmodel/client`, the dummy provider and the probe
   reroute. Run `protocoltest` failover / flag matrices — they should pass with
   the fake-status workaround gone.
5. Optional: `--vmodel-socket <path>` to additionally serve on a unix socket
   for out-of-process harnesses (`cli/harness --upstream=vmodel` from a
   subprocess).

## Endpoints vmodel does not simulate

`/embeddings`, `/images/*` and `/messages/count_tokens` are reachable through
the SDKs but have no virtual implementation. The virtualserver answers them
with an explicit 501 and a protocol-shaped "not supported by vmodel" error, so
the gateway propagates a clear status instead of a bare router 404.

## Non-goals

- Changing what vmodel *simulates* (models, sequences, latency profiles,
  error injection). Only the transport to reach it changes.
- Replacing the public `/virtual/*` routes. They remain the authenticated,
  externally reachable entrypoint.
