# Test Infrastructure

Overview of the test packages, their responsibilities, and how they relate.

## Package Hierarchy

```
vmodel/vmodeltest/          ← HTTP test client + response parser (no test-framework deps)
    ↑ embedded by
internal/protocoltest/      ← Protocol transform validation (VirtualServer + TestEnv + Matrix)
    ↑ imported by
cli/harness/                ← CLI entry points for matrix, agent, replay harnesses

internal/servertest/        ← Gateway integration tests (independent, shares nothing above)
```

## Packages

### `vmodel/vmodeltest`

Standalone HTTP test client for vmodel endpoints.

- **`Client`** — sends model-parameterized requests (`SendOpenAIChatModel`, `SendAnthropicV1Model`, `SendAnthropicBetaModel`)
- **`ParsedResponse`** — HTTP status + raw body + SSE events + parsed semantics (`sse.ParsedResult`)
- **Parser helpers** — `ParsedResultFromJSON`, `ParsedResultFromStream` (delegates to `internal/protocol/sse`)

No dependency on `protocoltest` or `servertest`. Used by `vmodel/virtualserver/` tests.

### `internal/protocoltest`

End-to-end protocol transform validation framework. Tests that the gateway correctly converts between provider formats (OpenAI ↔ Anthropic ↔ Google).

**Mock provider layer** (from former `server_validate`):
- **`VirtualServer`** — scenario-driven mock HTTP provider speaking OpenAI/Anthropic/Google formats at provider-native routes (`/v1/chat/completions`, `/v1/messages`, etc.)
- **`VirtualClient`** — embeds `vmodeltest.Client`, adds scenario-based send methods (`SendOpenAIChat`, `SendAnthropicV1`, `SendGoogle`) with auto-registration on a bound `VirtualServer`

**Test framework layer** (from former `protocol_validate`):
- **`Scenario`** — named test case with `MockResponses` per `ResponseFormat` + `Assertions`. Implements `vmodel.VirtualModel` for registry storage.
- **`TestEnv`** — wires a real gateway server to a VirtualServer, manages routing rules. Provides `SetupRoute()` + `SendAs()` for full round-trip testing.
- **`AgentTestEnv`** — variant for agent CLI testing (Claude Code, Codex, OpenCode).
- **`Matrix`** — combinatorial executor: sources × targets × scenarios × streaming modes.
- **Assertions** — `AssertHTTPStatus`, `AssertContentContains`, `AssertHasToolCalls`, `AssertHasThinking`, etc.
- **Failover helpers** — two-tier routing rules with vmodel-backed error injection.
- **Real model config** — `LoadProvidersConfig` for testing against real upstream providers.

Built-in scenarios: `text`, `tool_use`, `tool_result`, `thinking`, `multi_turn`, `streaming_text`, `streaming_tool_use`, `error`.

### `internal/servertest`

Gateway-level integration tests. Tests server features (auth, routing, load balancing) using a simpler mock approach.

- **`MockProviderServer`** — endpoint-level mock (set response per endpoint, track call counts). Simpler than VirtualServer — no scenario registry, no multi-format awareness.
- **`TestServer`** — wraps a real gateway server + config. Helpers: `AddTestProviders`, `AddTestRule`, `EnsureLoadBalancingRule`.
- **Request/response helpers** — `CreateTestChatRequest`, `CreateJSONBody`, `AssertJSONResponse`.

Uses `//go:build e2e` tag for some tests. Not imported by any other package.

## When to Use What

| I want to... | Use |
|---|---|
| Test a vmodel endpoint handler | `vmodeltest.Client` |
| Test protocol transforms (OpenAI→Anthropic, etc.) | `protocoltest.TestEnv` + `Matrix` |
| Test gateway routing / load balancing / auth | `servertest.TestServer` + `MockProviderServer` |
| Test agent CLI integration | `protocoltest.AgentTestEnv` |
| Run the full validation matrix from CLI | `cli/harness/` (imports `protocoltest`) |
| Validate the gateway against real SDK clients (Go in-process, Python/Node subprocess) | `cli/harness matrix --client=...` — see "Client drivers" in [`harness-matrix.md`](./harness-matrix.md) |

## Future direction

The three mock layers above (`virtualserver`, `protocoltest.VirtualServer`,
`servertest.MockProviderServer`) are being unified onto a single observable
reference bench under `vmodel/benchmark` — shared transport, request capture,
and a reusable check-logic layer (`check/`) with pluggable response generation.
See [`vmodel-benchmark.md`](./vmodel-benchmark.md) for the design and migration
phases. This table will be updated as Phases 2–3 land.
