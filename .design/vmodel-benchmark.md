# vmodel as a shared real-world benchmark

`vmodel` already ships production-grade, protocol-compliant synthetic LLM
behavior (it backs the public `/virtual/v1/*` endpoint). This document proposes
elevating it into the single **reference test-bench** — a "real-world
benchmark" — that `internal/servertest`, `internal/protocoltest`, future
`*test` packages, and outside Go projects all build on, including a reusable
*preset check-logic* layer.

Status: **design-first.** This doc defines the architecture and a foundation
package; the foundation implementation (Phase 1) and consumer migrations
(Phases 2–3) land under separate approvals.

## Motivation

Three separate mock-provider implementations exist today, with
overlapping-but-inconsistent capabilities:

| | `vmodel/virtualserver.Service` (prod) | `protocoltest.VirtualServer` | `servertest.MockProviderServer` |
|---|---|---|---|
| Protocol-correct responses | real vmodel models | `MockResponseBuilder` fixtures | hardcoded JSON maps + `fmt.Printf` spam |
| Formats | OpenAI chat + Responses + Anthropic | + Google (4) | OpenAI chat + Anthropic (crude) |
| Request capture / hit counts | none | yes | partial |
| Scenario registry | vmodel registry | `GenericRegistry[Scenario]` | none |
| Error injection (pre/mid-stream) | yes | yes | status + delay only |
| Real TCP listener | via `benchmark.LocalServer` | httptest only | httptest only |

The costs: fixture drift between the three; `servertest`'s mock emits hand-rolled
JSON that is not guaranteed wire-correct and is littered with debug `fmt.Printf`;
no shared assertion vocabulary; and nothing an outside project can `go get`.

## Positioning

The bench is a **business-first** extension of `vmodel`, consistent with
`vmodel/README.md` "Positioning & registration discipline": it ships alongside
the production `/virtual/v1/*` surface and is the single source of truth for
synthetic, protocol-compliant model behavior. Test packages remain **secondary
consumers** that reuse the same primitives.

### Resolving the "benchmark" name overload

`vmodel/benchmark` today hosts a **load generator** (`client.go` — pooled HTTP
throughput/latency driver) plus `LocalServer` (a TCP load target wrapping
`virtualserver.Service`). We keep that package and grow it to host **two
complementary roles**:

| Role | Entry points | Purpose |
|------|--------------|---------|
| Load generator | `BenchmarkClient`, `BenchmarkOptions`, `BenchmarkResult` | throughput / latency measurement (unchanged) |
| Reference bench | `Server`, `check/`, `scenario/` | observable mock-provider for tests + external projects |

The README must state this split explicitly so "benchmark" is never ambiguous.

## Architecture

### Central insight: observability and transport are shared; response generation is pluggable

Request capture, per-endpoint hit counts, total call count, and the in-process
(`httptest`) vs. real-TCP-listener choice are **orthogonal** to how a request
becomes a response. The foundation makes the first three shared and the last
pluggable:

```
bench.Server = observing middleware (capture + EndpointKind hits + counts)
               wrapped around ANY inner provider http.Handler
   ├── NewModelServer()  → inner = virtualserver.Service routes (real vmodel models)
   │                            used by servertest (opt-in), external projects, load tests
   └── NewScenarioServer(reg) → inner = scenario/MockResponseBuilder mux (4 formats)
                                used by protocoltest's transform matrix and servertest's byte-exact mocks
```

### What is shared vs. pluggable

| Layer | Concern | Shared / Pluggable | Where it lives | Source today |
|---|---|---|---|---|
| Transport | in-process `httptest` **or** real TCP listener (`Listen(addr)`) | **shared** | `benchmark/bench.go` | `benchmark/server.go` (TCP) + httptest |
| Observability | request capture, `EndpointKind` hit counts, total call count, `LastRequest` | **shared** | `benchmark/capture.go` | elevated from `protocoltest.VirtualServer` |
| Error simulation | pre-content + mid-stream injection, delay | **shared** | reuse `vmodel.ErrorInjection` | `vmodel/error_injection.go` |
| Check logic | `Assertion` + `RoundTripResult` (`AssertContentContains`, `AssertHasToolCalls`, `AssertHTTPStatus`, …) | **shared** | `benchmark/check/` | elevated from `protocoltest/assertions.go` + `types.go` |
| Response generation | how a request becomes a response body | **pluggable** | inner `http.Handler` | two responders ↓ |
| ↳ Model responder | real vmodel models → protocol-correct bytes | plug A | `NewModelServer()` | `vmodel/virtualserver.Service` |
| ↳ Scenario responder | `MockResponseBuilder` fixtures across 4 formats | plug B | `NewScenarioServer(reg)` | `protocoltest/scenarios.go` |

### Package layout (extend `vmodel/benchmark`)

```
vmodel/benchmark/
├── client.go            (existing) load generator — unchanged
├── server.go            (existing) LocalServer load target — kept; thin alias over
│                        bench.NewModelServer().Listen()
├── bench.go             NEW  Server: observing wrapper; InProcess() (httptest) + Listen(addr) (TCP);
│                             NewModelServer() + NewScenarioServer(reg) constructors
├── capture.go           NEW  CapturedRequest, EndpointKind, recorder — elevated from
│                             protocoltest.VirtualServer (capture / recordHit / EndpointHits / LastRequest)
├── check/               NEW subpkg — reusable preset check logic
│   ├── result.go        RoundTripResult, ToolCallResult, TokenUsage (from protocoltest/types.go)
│   └── assert.go        Assertion + AssertContentContains / AssertHasToolCalls / AssertHTTPStatus / …
├── scenario/            NEW subpkg — reusable fixtures
│   ├── scenario.go      Scenario, ResponseFormat, MockResponseBuilder (from protocoltest/scenarios.go)
│   └── builtins.go      text / tool_use / thinking / streaming_* / error presets
└── examples/server/     (existing) simple runnable example — keep; point at bench.Server
```

**Import-cycle safety (verified).** `vmodel/*` already imports
`internal/protocol` (e.g. `vmodel/virtualserver/handler.go`,
`vmodel/vmodeltest/client.go`); non-test `internal/protocol` does **not** import
`vmodel`. So `check/` may import `internal/protocol` for `protocol.APIType`, and
`benchmark` may import `internal/protocol/sse`, with no cycle.

**No `*testing.T` in the core.** `bench.Server`, `check/`, and `scenario/` take a
`*testing.T` nowhere, so external (non-`go test`) consumers can import them.
`t.Cleanup` ergonomics stay in the consumer wrappers.

## Reusable preset check logic

`check.Assertion` is `{Name string; Check func(*RoundTripResult) error}`. A
`RoundTripResult` is the protocol-neutral, post-parse view of one round trip
(HTTP status, raw body, SSE events, plus extracted `Content`/`Role`/`Model`/
`FinishReason`/`ToolCalls`/`ThinkingContent`/`Usage`). The existing assertion
library (`AssertContentContains`, `AssertHasToolCalls`, `AssertFinishReasonOneOf`,
`AssertHTTPStatus`, `AssertUsageNonZero`, …) moves into `check/` verbatim. Any
`*test` package — or an outside project — can then assert on a round trip with
one shared vocabulary instead of re-deriving checks per package.

## External-consumption story

- **Primary — in-process Go import.** An external test suite imports
  `vmodel/benchmark` (+ `check`, `scenario`), spins a `Server` with
  `InProcess()`, sends with `vmodeltest.Client`, and asserts with `check`. The
  bridge between the two is `(*vmodeltest.ParsedResponse).ToRoundTrip()`, which
  produces the `check.RoundTripResult` the assertion library consumes — so the
  reusable check layer ships with its *producer*, not just the assertions. This
  is exactly how `protocoltest` uses `VirtualServer` today, generalized.
- **Secondary — simple runnable server.** `vmodel/benchmark/examples/server` is
  the canonical example: a `main` that calls `NewModelServer().Listen(addr)`
  and serves the model responder (real vmodel models) over loopback for
  non-Go drivers — i.e. it demonstrates the foundation itself. No new CLI surface
  beyond this example.

### Two production-backed servers, one router

`NewModelServer()` (observable: capture + endpoint hits) and `LocalServer`
(the capture-free load target used by the load generator) both serve the same
`virtualserver.Service` and share their route wiring via the package-private
`modelRouter()` helper — so there is one place that mounts
`/v1`,`/openai/v1`,`/anthropic/v1`. `LocalServer` deliberately omits the capture
middleware to keep the load hot-path overhead-free. A server that needs *custom*
models (rather than the defaults) builds its own router over a registered
`virtualserver.Service` and wraps it with `NewServer(router)` — the same generic
seam — so no dedicated `Service()` accessor is shipped until a caller needs one.

## Migration phases (foundation-first)

1. **Phase 1 — foundation (✅ landed).** Built `bench.go`, `capture.go`,
   `check/`, `scenario/` with tests; the load generator (`client.go`,
   `LocalServer`) and examples are untouched. `protocoltest` now re-exports the
   relocated `check`/`scenario` symbols via `aliases.go`, so its existing suite
   doubles as the regression net for the elevated code. `servertest` is
   unchanged.
2. **Phase 2 — protocoltest (✅ landed).** `protocoltest.VirtualServer` is now a
   thin wrapper over `benchmark.NewScenarioServer` — the scenario-serving
   handlers, request capture, and endpoint-hit counting live in
   `vmodel/benchmark` (`scenario_responder.go` + `capture.go`). `EndpointKind`,
   its constants, and `CapturedRequest` are aliased to the benchmark types; the
   protocoltest-facing API (`Client()`, `RegisterScenario`, `EndpointHits`,
   `LastRequest`, …) is unchanged, so the matrix/flags/agent suites and
   `cli/harness` keep working. Validated by the full harness matrix
   (`--mode=all` + gosdk/python/node/aisdk drivers), 0 failures.
3. **Phase 3 — servertest (✅ landed).** `servertest.MockProviderServer` keeps
   its endpoint-keyed, byte-exact responder but wraps it with
   `benchmark.NewServer(inner)`. Benchmark owns transport, request capture,
   endpoint hit counts, and reset; the adapter owns arbitrary response, error,
   delay, and SSE configuration.

## Phase 3 — servertest: shared foundation, local responder

**Current state (2026-07-23):** Phase 3 now uses the generic
`NewServer(inner)` seam. `benchmark.Server.ServeHTTP` exposes the same observable
handler path used by managed transports, so direct tests, request capture, hit
counts, and reset all share the benchmark implementation. The byte-exact echo
responder remains local to `servertest`; using `NewModelServer` or
`NewScenarioServer` would change its response semantics.

### Historical decision context

**Decision: do not migrate `servertest` onto the foundation, and leave it
untouched in this PR.** The analysis that led here (grounded in actual usage,
not assumed):

- **Lower benefit than Phase 2.** `servertest.MockProviderServer`'s response model
  is fundamentally different from the scenario one:

  | Axis | `protocoltest.VirtualServer` (Phase 2) | `servertest.MockProviderServer` |
  |---|---|---|
  | Keyed by | scenario (from request `model`) | **endpoint** (`/v1/chat/completions`, `/v1/messages`) |
  | Response source | `MockResponseBuilder` per format | arbitrary `Body`/`Error`/`StatusCode` set at runtime |
  | When unset | 500 "no mock for scenario" | **sensible default** body (tests rely on this) |
  | Extra knobs | `StreamHTTPError` | **`Delay`** (timeout), per-endpoint call counts |

  servertest *deliberately* wants a dumb echo with byte-exact control — it does
  **not** want the foundation's headline value (wire-correct generated responses),
  so migrating would not improve its behavior.
- **Higher cost than Phase 2.** Phase 2 was a delegation (parity by construction).
  Phase 3 would be **net-new code** — a third `EndpointMock` responder
  (`SetResponse`/`Delay`/defaults/error-envelope/streaming, ~150 lines) whose
  *only* consumer is servertest. The "duplication" removed is just the small
  generic httptest+capture plumbing; the bulk (echo/default logic) would merely
  move, not shrink — and it sits under gateway-level tests (LB / auth /
  concurrency), so the swap carries risk for little gain.
- **Already "supported".** The original goal — a foundation servertest *can*
  reuse — is met: the foundation exists and is proven across five client
  drivers. Support ≠ forced absorption. Migrating now is speculative
  gold-plating (violates "done ≠ locked", "reduce noise", avoid speculative
  abstraction).

**State at the time of that decision:** `servertest` was unchanged.

**Follow-up outcomes:**

1. **Cleanup (independent, low-risk).** `mock_provider.go` carries ~250 lines of
   dead scaffolding (`MockProviderTestSuite` + `RunMockProviderTests`, never
   invoked) and 13 `fmt.Printf` debug lines in the live handlers. Deleting the
   dead suite and stripping the spam (607 → ~322 lines) is a clear quality win,
   but it is unrelated to the benchmark foundation and belongs in its own small,
   obvious-to-review PR — not bundled here. It needs no benchmark coupling.
2. **Adoption (now landed).** `servertest` now adopts the generic
   `benchmark.NewServer(inner)` seam while keeping the echo responder local.

---

The implementation follows the same thesis — **observability + transport are
shared; response generation is pluggable** — and leans on the existing
`NewServer(inner)` seam rather than bending servertest onto the scenario one.

### Step 1 — keep the echo *handler* local (no new constructor)

The generic `NewServer(inner http.Handler)` already wraps any handler with the
capture + transport layer, so a dumb-echo needs **no bespoke constructor** — only
an `http.Handler`. (Contrast `NewModelServer`/`NewScenarioServer`, which earn
their constructors by bundling real wiring — a gin router + service, or a
registry wired into the Server for `RegisterScenario`. An echo bundles nothing
of the sort.)

```go
srv := benchmark.NewServer(endpointHandler) // capture + transports for free
```

- The local responder serves `/v1/chat/completions` (+ `/chat/completions`),
  `/v1/messages` (+ `/messages`); honors `Delay` (sleep), `Error` (envelope),
  and the `stream` flag by emitting the exact configured SSE events.
- Default bodies (`CreateMockChatCompletionResponse` plus the Anthropic default)
  stay local, preserving the "when unset" behavior without expanding the public
  benchmark API.
- Per-endpoint counts/last-request come from the shared recorder's exact-path
  indexes; kind-level observation remains available independently.

### Step 2 — make `servertest.MockProviderServer` a thin adapter

Keep its exact public signatures (`NewMockProviderServer`, `SetResponse`,
`SetStreamingResponse`, `GetURL`, `GetCallCount`, `GetLastRequest`, `Reset`,
`Close`) but back them with `benchmark.NewServer(inner)`. `MockResponse` and
`MockStreamingResponse` stay local because they describe the local responder.

### Risks / decisions

- **`Delay` lives on the echo `MockResponse`, not on `scenario.MockResponseBuilder`** —
  the two response strategies stay separate; no change to the scenario path.
- **Counter keying**: benchmark records both `EndpointKind` aggregates and exact
  URL paths. The servertest compatibility methods use exact paths, preserving
  the distinction between `/v1/chat/completions` and `/chat/completions`.
- **`GetLastRequest` returns `map[string]interface{}`** today; the benchmark
  `CapturedRequest.JSON()` returns the same shape — adapter calls `.JSON()`.
- Validation: `go test ./internal/servertest/...` (unchanged); no matrix
  involvement (servertest is gateway-level).

## Parity check — does the foundation serve both with the same effect?

Verified against current usage, not assumed:

- **protocoltest — parity by construction.** The scenario responder + `capture.go`
  + `check/` *are* today's `VirtualServer` and `assertions.go` elevated verbatim;
  Phase 2 is a thin re-export wrapper, so nothing can drift.
- **servertest — shared foundation with a local responder.** Its mock surface
  delegates transport and observability to `benchmark.Server`
  (`SetResponse`/`SetStreamingResponse`/`GetCallCount`/`GetLastRequest`/`GetURL`/
  `Reset`/`Close` over the shared `capture.go`). But servertest *intentionally*
  wants a **dumb echo** (arbitrary bytes to exercise gateway forwarding), **not**
  generated protocol-correct responses — so the foundation's headline value does
  not apply. The response generator stays local while capture and transport are
  shared.

This is the payoff of the pluggable split: the model responder serves
protocol-correct paths, the scenario responder serves the transform matrix, and
the local endpoint responder serves servertest's byte-exact needs — one
shared observability + transport layer underneath, response generation pluggable
on top. No single responder is forced on a consumer it doesn't fit.

## Risks & non-goals

- **No change to `/virtual/v1/*`.** The production endpoint keeps using
  `virtualserver.Service`; the bench wraps, never forks, it.
- **No forced single response strategy.** Generative models and fixture builders
  coexist behind the same observability layer by design.
- **Load-generator API untouched.** `BenchmarkClient`/`BenchmarkResult` stay
  as-is; only additive growth of the package.
- **Migrations were gated.** The foundation was validated before the Phase 2 and
  Phase 3 consumers moved.

## See also

- `vmodel/README.md` — positioning & registration discipline.
- `.design/test-infrastructure.md` — current package hierarchy (to be updated
  once Phases 2–3 land).
- `.design/harness-matrix.md` — CLI harness + client drivers that ride on
  `protocoltest`.
