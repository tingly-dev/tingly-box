# Harness Matrix

> For contributors working with `internal/protocoltest/`, `cli/harness/`,
> or adding new protocol conversion paths / scenarios.
>
> Related: per-rule **flag** behavior is tested by a separate, registry-driven
> suite — see [`rule-flag-testing.md`](./rule-flag-testing.md). The matrix
> itself stays flag-free (it exercises protocol conversion, not rule flags).

---

## 1. What the matrix tests

The gateway converts between 4 protocol families (Anthropic V1, Anthropic
Beta, OpenAI Chat, OpenAI Responses) in both streaming and non-streaming
modes. The matrix validates that every supported conversion path preserves
content, role, tool calls, usage, and finish reason.

Three levels of validation, each more demanding:

| Level | What it proves | Entry point |
|-------|----------------|-------------|
| **Single-hop** | A→B preserves semantics | `Matrix.Run(t)` / `Matrix.ExecuteAll()` |
| **Two-hop (transitive)** | A→B→C preserves semantics across chained conversions | `Matrix.RunTransitive(t)` / `Matrix.ExecuteAllTransitive()` |
| **Idempotent (round-trip)** | `g(f(A)) == A` — converting A→B then B→A recovers the original | `Matrix.RunIdempotent(t)` / `Matrix.ExecuteAllIdempotent()` |

Two-hop and idempotence are **different** validations and must not be
conflated: two-hop checks semantic preservation across a *chain of distinct*
conversions (A→B→C), while idempotence checks that a *round-trip* recovers the
original (A→B→A with `g(f(A)) == A` assertions). The transitive run does emit
some chains where C happens to equal A, but those only run the scenario's
normal assertions after two hops — they do **not** assert idempotence. True
idempotence lives in its own path (`idempotent.go`).

---

## 2. Architecture

```
Scenario (mock responses in 4 formats)
    ↓
Matrix (pairs × scenarios × streaming modes)
    ↓
TestEnv (real gateway server + VirtualServer mock provider)
    ↓
SendAs / SendAsCLI → RoundTripResult (parsed semantics)
    ↓
Assertions + SemanticEquivalence checks
```

### Key types

- **`Scenario`** — named test case. Carries `MockResponses` per format,
  `Assertions` to check, and `SkipTransitive` flag for scenarios that
  produce no comparable output (e.g. error responses).

- **`ProtocolPair`** — a single `(Source → Target)` conversion path.
  `DefaultPairs()` lists exactly the 12 pairs the dispatch graph supports
  (not the Cartesian product — many cells would map to the same handler).

- **`TransitiveChain`** — two pairs `A→B` + `B→C` joined where
  `First.Target == Second.Source`. Built automatically from `DefaultPairs()`.

- **`Matrix`** — the orchestrator. Holds `Pairs`, `Scenarios`, `Streaming`
  modes, and filter/config methods (`OnlyScenarios`, `OnlySources`, etc.).

- **`RoundTripResult`** — normalized output: `Content`, `Role`,
  `FinishReason`, `ToolCalls`, `Usage`, `StreamEvents`, etc.

### Shared helpers

| Helper | Purpose |
|--------|---------|
| `streamMode(bool)` | Returns `"stream"` or `"nonstream"` for test names |
| `streamingSkipReason(Scenario, bool)` | Checks streaming compatibility, returns skip reason |
| `semanticEquivalenceErrors(label, r1, r2)` | Compares two results field-by-field, returns `[]AssertionError` |
| `assertSemanticEquivalence(t, label, r1, r2)` | Delegates to above, calls `t.Errorf` per error |

---

## 3. How to run

### go test (requires `-tags e2e`)

```bash
# Everything — single-hop + two-hop, both modes
go test -tags e2e ./internal/protocoltest/... -run TestHarness

# Single-hop only
go test -tags e2e ./internal/protocoltest/... -run TestHarness/single_hop

# Two-hop only
go test -tags e2e ./internal/protocoltest/... -run TestHarness/two_hop

# Streaming only
go test -tags e2e ./internal/protocoltest/... -run TestHarness_Streaming

# Idempotent round-trips
go test -tags e2e ./internal/protocoltest/... -run TestIdempotent
```

### CLI (`cli/harness`)

`--mode` selects which sections run. Each section has a `testing.T`-free
executor (`ExecuteAll*`) so the CLI can run it directly — including idempotence
and the rule-flag suite, which would otherwise be go-test-only.

| `--mode` | single (A→B) | transitive (A→B→C) | idempotent (`g(f(A))==A`) | flags (per-rule) |
|----------|:---:|:---:|:---:|:---:|
| `default` *(no flag)* | ✅ | — | ✅ | — |
| `all` | ✅ | ✅ | ✅ | ✅ |
| `single` | ✅ | — | — | — |
| `transitive` | — | ✅ | — | — |
| `idempotent` | — | — | ✅ | — |
| `flags` | — | — | — | ✅ |

```bash
# Default: single-hop + idempotent round-trips. Two-hop and flags are OFF by
# default (two-hop is the slowest and overlaps single-hop; flags are an
# orthogonal axis).
go run ./cli/harness matrix

# Everything
go run ./cli/harness matrix --mode=all

# A single section
go run ./cli/harness matrix --mode=single
go run ./cli/harness matrix --mode=transitive
go run ./cli/harness matrix --mode=idempotent
go run ./cli/harness matrix --mode=flags     # per-rule flag behavior

# Filter by scenario / source / target
go run ./cli/harness matrix --scenario text --source anthropic_v1

# JSON for CI
go run ./cli/harness matrix --json
```

The `flags` section is documented in detail in
[`rule-flag-testing.md`](./rule-flag-testing.md); `ExecuteAllFlags` reports one
`TestResult` per flag (`Name: "flags/<key>"`, `Scenario: <key>`).

---

## 4. Adding a new scenario

1. Define a `FooScenario() Scenario` function in `scenarios.go` with
   `MockResponses` for all 4 formats (nonstream + stream).

2. Add it to `AllScenarios()`.

3. Write `Assertions` that validate the converted result (HTTP status,
   content, tool calls, etc.).

4. Set `SkipTransitive: true` if the scenario produces no output worth
   comparing across hops (e.g. error responses).

5. Run `go test -tags e2e ./internal/protocoltest/... -run TestHarness`
   to verify it passes across all pairs and modes.

### Example

```go
func IncompleteScenario() Scenario {
    return Scenario{
        Name:        "incomplete",
        Description: "max_output_tokens truncation with partial output + usage",
        Tags:        []string{"incomplete"},
        MockResponses: map[ResponseFormat]MockResponseBuilder{
            FormatOpenAIChat:      openAIIncompleteResponse(),
            FormatOpenAIResponses: openAIResponsesIncompleteResponse(),
            FormatAnthropic:       anthropicIncompleteResponse(),
            FormatGoogle:          googleIncompleteResponse(),
        },
        Assertions: []Assertion{
            AssertHTTPStatus(200),
            AssertContentContains("Paris"),
            AssertUsageNonZero(),
        },
    }
}
```

---

## 5. Adding a new protocol pair

1. Add the pair to `DefaultPairs()` in `matrix.go`.

2. Ensure the mock provider (VirtualServer) can serve that target format.

3. Two-hop chains are derived automatically — any chain where
   `First.Target == Second.Source` is included.

4. If the pair has known limitations, add a skip entry to
   `skipSourceScenarios` in `matrix.go`.

---

## 6. Test naming conventions

Tests are structured as nested subtests for `-run` filtering:

```
TestHarness/
  single_hop/
    {scenario}/
      {source}/
        {target}/
          stream|nonstream

  two_hop/
    {scenario}/
      {A}→{B}→{C}/
        stream|nonstream
```

CLI `TestResult.Name` follows the same `scenario/path/mode` pattern.

---

## 7. Design decisions

**Why explicit pairs, not Cartesian product?**
Many cells of the full product map to the same dispatch handler (e.g.
`anthropic_v1 → anthropic_beta` and `anthropic_v1 → anthropic_v1` both
hit the Anthropic passthrough). Listing pairs keeps the matrix in sync
with the actual dispatch graph. See `internal/protocol/README.md`.

**Why `SkipTransitive` on Scenario instead of a name check?**
A hardcoded `name == "error"` check is fragile — future error-shaped
scenarios would silently skip without appearing in the iteration. The
boolean makes the opt-out explicit and discoverable.

**Why `--mode` enum instead of `--transitive`/`--single-hop` booleans?**
Two mutually-exclusive booleans require a manual conflict check and
would need a third boolean for any future hop level. The enum encodes
the constraint in the flag parser (Kong validates it).

**Why `semanticEquivalenceErrors` returns `[]AssertionError`?**
This is the shared core used by both the `testing.T` path
(`assertSemanticEquivalence`) and the CLI path
(`executeTransitiveChain`). The `testing.T` version just loops and
calls `t.Errorf` per entry — no duplicated field checks.

---

## 8. Error scenarios (pre-content vs mid-stream)

Error scenarios are modeled by `ErrorInjection.Stage`, and the two stages have
very different observable shapes — conflating them hides real gateway bugs.

| Stage | Real-world shape | Mock | Scenarios |
|-------|------------------|------|-----------|
| **PreContent** | upstream rejects at the HTTP status line, before any SSE frame | fails with the HTTP status — for streaming too, via `MockResponseBuilder.StreamHTTPError` | `error` (429), `error-500`, `error-auth-401` |
| **MidStream** | upstream starts a normal 200 stream, emits partial content, then the connection drops | `buildMidStreamTruncated`: 200 + partial content, terminal frames omitted (`[DONE]` / `message_stop` / `response.completed`) | `error-midstream-close` |

`BuildErrorFromSpec` routes on the stage. The earlier (now-fixed) behavior
served *all* streaming errors as `200 + an SSE error line`, which no real
provider does and which the gateway cannot surface as an HTTP status.

These scenarios lock two gateway behaviors:

- **Upstream status propagation** — a forwarding failure returns the upstream
  HTTP status (401/429/4xx), not a flat 500. `protocol.UpstreamStatus(err,
  fallback)` extracts it from the vendor SDK error types; the non-stream
  handlers and the streaming pre-frame helpers (`SendStreamingError`,
  `SendForwardingError`) all use it.
- **Truncated-stream termination** — a Responses stream that ends without
  `response.completed` must still terminate: the Responses→Chat converter emits
  a fallback terminal chunk and sets `completed=true` so it does not re-emit
  forever (an unbounded-flush OOM otherwise).

All error scenarios set `SkipTransitive: true` — a wrapped error shape is not
worth comparing across hops (and idempotence skips `error` for the same reason).

---

## 9. Inspecting the forwarded request (capture & flags)

The matrix asserts on the parsed *response*, but some checks need the *request
the gateway actually forwarded upstream*. The `VirtualServer` mock records it:

| Helper | Purpose |
|--------|---------|
| `VirtualServer.LastRequest(kind)` | the forwarded request (method, path, headers, body) for a provider endpoint — assert field rewrites, header overrides, stripped tools, folded messages, … |
| `VirtualServer.EndpointHits(kind)` | how many requests hit each provider endpoint — assert which endpoint the gateway chose |
| `TestEnv.SetupRouteWithFlags(src, tgt, scenario, flags)` | wires a route with `rule.Flags` set, so the request traverses the real flag-resolution + transform path |

These power the per-rule **flag** behavior suite, which is documented
separately: [`rule-flag-testing.md`](./rule-flag-testing.md). Keep the matrix
itself flag-free — flags are an orthogonal axis and live in their own suite.
