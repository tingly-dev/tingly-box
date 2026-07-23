# Test Infrastructure

Overview of the test packages, their responsibilities, and how they relate.

## Package Hierarchy

```
vmodel/benchmark/           ← reference test-bench foundation:
  ├ check/                    reusable RoundTripResult + Assertion library (no test-framework deps)
  ├ scenario/                 reusable mock-provider fixtures (Scenario, MockResponseBuilder)
  └ Server                    observable mock provider (capture + hits) over pluggable responders
    ↑ wrapped / re-exported by
vmodel/vmodeltest/          ← HTTP test client + response parser (no test-framework deps)
    ↑ embedded by
internal/protocoltest/      ← Protocol transform validation; VirtualServer wraps benchmark.Server,
                              re-exports check/scenario via aliases.go (TestEnv + Matrix on top)
    ↑ imported by
cli/harness/                ← CLI entry points for matrix, agent, replay harnesses

internal/servertest/        ← Gateway integration tests; endpoint-keyed dumb-echo responder
                              wrapped by benchmark.Server transport + capture
```

## Packages

### `vmodel/benchmark`

The shared **reference test-bench** (and load generator). It is the single source
of reusable mock-provider building blocks; protocoltest consumes them.

- **`check/`** — protocol-neutral `RoundTripResult` + the named `Assertion`
  library (`AssertContentContains`, `AssertHasToolCalls`, …). No test-framework
  dependency, so any `*test` package or external Go project can reuse it.
- **`scenario/`** — reusable fixtures: `Scenario` (implements `vmodel.VirtualModel`),
  `MockResponseBuilder` per `ResponseFormat`, and the built-in scenario set.
- **`Server`** — observable mock provider: capture / per-endpoint hit counts over
  pluggable response generation. `NewProductionServer()` serves real vmodel
  models; `NewScenarioServer()` serves scenario fixtures. In-process (`InProcess`)
  or real-TCP (`Listen`) transport.
- **Load generator** — `BenchmarkClient` / `LocalServer` (throughput/latency),
  the package's original role.

See [`vmodel-benchmark.md`](./vmodel-benchmark.md).

### `vmodel/vmodeltest`

Standalone HTTP test client for vmodel endpoints.

- **`Client`** — sends model-parameterized requests (`SendOpenAIChatModel`, `SendAnthropicV1Model`, `SendAnthropicBetaModel`)
- **`ParsedResponse`** — HTTP status + raw body + SSE events + parsed semantics (`sse.ParsedResult`)
- **Parser helpers** — `ParsedResultFromJSON`, `ParsedResultFromStream` (delegates to `internal/protocol/sse`)

No dependency on `protocoltest` or `servertest`. Used by `vmodel/virtualserver/` tests.

### `internal/protocoltest`

End-to-end protocol transform validation framework. Tests that the gateway correctly converts between provider formats (OpenAI ↔ Anthropic ↔ Google).

**Mock provider layer** (from former `server_validate`):
- **`VirtualServer`** — a thin wrapper over `benchmark.NewScenarioServer` (the scenario-serving handlers, request capture, and endpoint-hit counting live in `vmodel/benchmark`). Speaks OpenAI/Anthropic/Google formats at provider-native routes. `EndpointKind` / `CapturedRequest` are aliased to the benchmark types.
- **`VirtualClient`** — embeds `vmodeltest.Client`, adds scenario-based send methods (`SendOpenAIChat`, `SendAnthropicV1`, `SendGoogle`) with auto-registration on a bound `VirtualServer`

**Test framework layer** (from former `protocol_validate`):
- **`Scenario`** / **`Assertion`** / **`RoundTripResult`** — re-exported from `vmodel/benchmark/{scenario,check}` via `aliases.go`; `protocoltest` is a thin consumer of the foundation.
- **`TestEnv`** — wires a real gateway server to a VirtualServer, manages routing rules. Provides `SetupRoute()` + `SendAs()` for full round-trip testing.
- **`AgentTestEnv`** — variant for agent CLI testing (Claude Code, Codex, OpenCode).
- **`Matrix`** — combinatorial executor: sources × targets × scenarios × streaming modes.
- **Assertions** — `AssertHTTPStatus`, `AssertContentContains`, `AssertHasToolCalls`, `AssertHasThinking`, etc.
- **Failover helpers** — two-tier routing rules with vmodel-backed error injection.
- **Real model config** — `LoadProvidersConfig` for testing against real upstream providers.

Built-in scenarios: `text`, `tool_use`, `tool_result`, `thinking`, `multi_turn`, `streaming_text`, `streaming_tool_use`, `error`.

### `internal/servertest`

Gateway-level integration tests. Tests server features (auth, routing, load balancing) using a deliberately simple mock approach.

- **`MockProviderServer`** — a thin adapter over `benchmark.NewServer`: its local
  endpoint-keyed responder keeps byte-exact `Body` / `Error` / `Delay` / SSE
  control, while benchmark owns HTTP transport, request capture, endpoint hit
  counts, and reset behavior.
- **`TestServer`** — wraps a real gateway server + config. Helpers: `AddTestProviders`, `AddTestRule`, `EnsureLoadBalancingRule`.
- **Request helpers** — `CreateTestChatRequest`, `CreateJSONBody`.

Uses `//go:build e2e` tag for some tests. Not imported by any other package.

Every test creates a fresh configuration under `t.TempDir()` and populates it
with explicit fixtures. Tests must never discover, read, copy, migrate, or write
the developer's default configuration directory, provider database, credentials,
or runtime data. No-argument config constructors are therefore forbidden in
tests; pass `config.WithConfigDir(t.TempDir())` instead. Local mock providers use
a two-second timeout, and the e2e suite must run with an explicit Go test timeout.

## When to Use What

| I want to... | Use |
|---|---|
| Test a vmodel endpoint handler | `vmodeltest.Client` |
| Test protocol transforms (OpenAI→Anthropic, etc.) | `protocoltest.TestEnv` + `Matrix` |
| Test gateway routing / load balancing / auth | `servertest.TestServer` + `MockProviderServer` |
| Test agent CLI integration | `protocoltest.AgentTestEnv` |
| Run the full validation matrix from CLI | `cli/harness/` (imports `protocoltest`) |
| Validate the gateway against real SDK clients (Go in-process, Python/Node subprocess) | `cli/harness matrix --client=...` — see "Client drivers" in [`harness-matrix.md`](./harness-matrix.md) |

## Benchmark unification — current state

The reusable mock-provider foundation now lives in `vmodel/benchmark` (shared
transport + request capture, a reusable `check/` assertion layer, and pluggable
response generation). Status:

- **Phase 1 (done)** — foundation built: `check/`, `scenario/`, observable `Server`.
- **Phase 2 (done)** — `protocoltest.VirtualServer` delegates to
  `benchmark.NewScenarioServer`; `check`/`scenario` re-exported via aliases.
- **Phase 3 (done)** — `servertest.MockProviderServer` delegates transport and
  observability to `benchmark.NewServer(inner)`. Its endpoint responder remains
  local because byte-exact gateway fixtures and protocol-correct vmodel scenarios
  are separate response strategies.

See [`vmodel-benchmark.md`](./vmodel-benchmark.md) for the full design and rationale.
