# Harness Matrix

> For contributors working with `internal/protocoltest/`, `cli/harness/`,
> or adding new protocol conversion paths / scenarios.
>
> Related: per-rule **flag** behavior is tested by a separate, registry-driven
> suite — see [`rule-flag-testing.md`](./rule-flag-testing.md). The matrix
> itself stays flag-free (it exercises protocol conversion, not rule flags).
>
> Related: request **content-shape** regressions (a client sending array-of-
> text-blocks content instead of a plain string on tool/system/assistant
> messages) are covered by a separate suite in `content_shapes.go` — see
> §9.1. The matrix itself always sends the same fixed single-turn prompt and
> only varies the mocked *response* shape, so it cannot catch bugs in how
> unusual *request* shapes are forwarded upstream.

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

### Section drivers (`section.go`)

Every section contributes only its combos and per-combo execution; the shared
lifecycle lives once in `internal/protocoltest/section.go`:

| Driver | Used by | Purpose |
|--------|---------|---------|
| `Matrix.executePerScenario(skip, combosFor)` | `ExecuteAll` / `ExecuteAllTransitive` / `ExecuteAllIdempotent` | CLI-side: one env per scenario, setup-failure fan-out (env boot failure is reported per-combination), sequential combo execution, env close |
| `Matrix.runPerScenario(t, skip, run)` | `RunTransitive` / `RunIdempotent` | testing.T-side: one env per scenario subtest, scenarios in parallel, combos sequential |
| `runRecorderCase(name, scenario, run)` | `ExecuteAllFlags` / `ExecuteAllContentShapes` | Runs one `flagTB`-style case body under the recording shim, returns a `TestResult` |

A `scenarioCombo` is `{meta TestResult, run func(*TestEnv) TestResult}` — the
`meta` doubles as the setup-failure result when the scenario's env cannot be
created. (Single-hop `Matrix.Run(t)` keeps its own deeper subtest nesting —
`scenario/source/target/mode` with one env per leaf — because its test-name
contract and per-leaf parallelism differ from the per-scenario sections.)

### CLI parallelism & env-boot cost

The CLI drivers run their independent units — scenarios in
`executePerScenario`, cases in `runRecorderCases` — concurrently up to
`GOMAXPROCS`, mirroring the `t.Parallel()` the go-test entry points always
had. Combos *within* a scenario still run sequentially against the shared
env. Results are written into index-addressed slots, so output order (and
therefore `--json` output) is identical to a sequential run. Two run modes
force sequential execution (`sectionParallelism()`): `--batch` measures
per-request timing that parallel load would skew, and `--record-dir`
captures traffic for replay that concurrent runs would interleave.

Env boots themselves are also cheap now: the single most expensive step of a
config boot was generating the enterprise-context RSA-2048 key pair
(~100-600ms of prime search) into every fresh temp config dir. The harness
pre-seeds each env's key slots from a process-level cached pair
(`serverconfig.PreseedEnterpriseContextKeys`), generated once per process —
sharing one throwaway key across harness envs is deliberate, and production
configs are untouched (they generate once and reuse from disk). Together
these took the default CLI run from ~15s to ~4s and `--mode=all` from ~41s
to ~10s on a 4-core machine; the e2e go-test path (already parallel, so
keygen CPU was its bottleneck) dropped from ~60s to ~17s.

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

# Request content-shape regression suite (see §9.1)
go test -tags e2e ./internal/protocoltest/... -run TestContentShapes
```

### CLI (`cli/harness`)

`--mode` selects which sections run. Each section has a `testing.T`-free
executor (`ExecuteAll*`) so the CLI can run it directly — including idempotence
and the rule-flag suite, which would otherwise be go-test-only.

| `--mode` | single (A→B) | transitive (A→B→C) | idempotent (`g(f(A))==A`) | flags (per-rule) | content_shapes (§9.1) |
|----------|:---:|:---:|:---:|:---:|:---:|
| `default` *(no flag)* | ✅ | — | ✅ | — | — |
| `all` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `single` | ✅ | — | — | — | — |
| `transitive` | — | ✅ | — | — | — |
| `idempotent` | — | — | ✅ | — | — |
| `flags` | — | — | — | ✅ | — |
| `content_shapes` | — | — | — | — | ✅ |

This mode → section mapping is declared in one place: the `matrixSections`
registry in `cli/harness/matrix.go`. Each entry names the section, lists the
`--mode` values that include it, marks whether it is http-only (`flags` /
`content_shapes` drive raw requests directly), and points at its
`ExecuteAll*` executor. Adding a section = one registry entry + extending the
`--mode` enum.

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
go run ./cli/harness matrix --mode=content_shapes  # request content-shape regression

# Filter by scenario / source / target
go run ./cli/harness matrix --scenario text --source anthropic_v1

# JSON for CI
go run ./cli/harness matrix --json
```

The `flags` section is documented in detail in
[`rule-flag-testing.md`](./rule-flag-testing.md); `ExecuteAllFlags` reports one
`TestResult` per flag (`Name: "flags/<key>"`, `Scenario: <key>`).

The `content_shapes` section is documented in §9.1 below; `ExecuteAllContentShapes`
reports one `TestResult` per case (`Name: "content_shapes/<case name>"`).

### Client drivers (`--client`)

By default the matrix sends hand-crafted JSON over Go's `net/http`. That
validates conversion *semantics* but not real-client *wire behavior*: official
SDKs dispatch SSE frames on the `event:` line, validate every response field
strictly (pydantic), and accumulate streams with protocol-enforcing state
machines. `--client` swaps the sending stack while reusing the same matrix,
scenarios, and assertions:

| `--client` | Stack | CI leg (`harness-matrix.yml`) |
|------------|-------|-------------------------------|
| `http` *(default)* | raw JSON over `net/http` (`client_http.go`) | `matrix-single` / `matrix-transitive` / `matrix-idempotent` / `matrix-flags` |
| `gosdk` | official `anthropic-sdk-go` + `openai-go`, in-process (`client_gosdk.go`) | `matrix-single-gosdk` / `matrix-idempotent-gosdk` |
| `python` | real `anthropic` + `openai` Python SDKs via subprocess driver | `matrix-single-python` |
| `node` | real `@anthropic-ai/sdk` + `openai` Node SDKs via subprocess driver | `matrix-single-node` |
| `aisdk` | AI SDK by Vercel (`ai` + `@ai-sdk/anthropic` + `@ai-sdk/openai`) via subprocess driver — the strictest client: zod-validates every response and stream event | `matrix-single-aisdk` |

Every client mode runs as its own leg in the harness-matrix workflow; the
subprocess legs install their toolchain (setup-python / setup-node +
driver dependencies) before running.

```bash
go run ./cli/harness matrix --mode=single --client=gosdk

# Python/Node need their driver deps once:
pip install -r tests/clients/python/requirements.txt
go run ./cli/harness matrix --mode=single --client=python

npm install --prefix tests/clients/node
go run ./cli/harness matrix --mode=single --client=node

npm install --prefix tests/clients/aisdk
go run ./cli/harness matrix --mode=single --client=aisdk
```

**The seam.** `Client` (`client.go`) is the driver interface:
`Send(env, SendSpec) (*RoundTripResult, error)`. `SendSpec` carries
`{Source, RequestModel, Streaming, GatewayURL, APIKey}` — request bodies are
scenario-independent, so a driver varies only by (source protocol × streaming).
All matrix paths (single / transitive / idempotent) funnel through
`TestEnv.sendModel`, which delegates to the configured client
(`Matrix.WithClient` / `NewTestEnvOptionWithClient`). The `flags` suite drives
raw requests with custom headers and stays http-only.

Drivers report gateway/API errors **in the result** (`HTTPStatus`, `RawBody`),
not as a `Send` error — error-scenario assertions must still run. A non-nil
`Send` error means the driver itself broke.

**Subprocess contract** (`client_subprocess.go` ⇄
`tests/clients/{python,node}/driver.*`): one JSON object on stdin, one on
stdout; non-zero exit = broken driver, API errors go in-band:

```jsonc
// stdin
{"version":1,"source":"anthropic_v1","base_url":"http://127.0.0.1:PORT",
 "api_key":"tb-...","model":"pv-...","stream":true,"scenario":"text",
 "prompt":"...","timeout_ms":30000}
// stdout
{"http_status":200,"role":"assistant","content":"...","model":"...",
 "finish_reason":"end_turn","thinking":"","tool_calls":[...],
 "usage":{"input_tokens":10,"output_tokens":8},
 "stream_event_count":7,"stream_completed":true,
 "stream_error":null,"raw_body":"...",
 "error":{"status":429,"type":"...","message":"..."}}
```

`stream_error` is distinct from `error`: the latter is an HTTP/API failure,
while `stream_error` means headers were already committed (typically HTTP 200)
but the turn did not reach a normal terminal event. Drivers must set
`stream_completed` only after observing a real protocol completion marker; they
must never synthesize completion for a truncated stream.

The contract itself is covered on every PR by a stub shell driver
(`client_subprocess_test.go` + `tests/clients/testdata/stub_driver.sh`), so
Python/Node are only needed where those SDKs actually run.

**Known incompatibilities** go in `clientSkipScenarios`
(`matrix.go`, key `client|source|scenario`) as *visible skips* with a reason —
never silent failures. Drivers must not be weakened to paper over a gateway
bug; the strictness is the point. The gosdk/python bring-up alone surfaced
four real gateway bugs (missing `event:` lines on the v1 stream path, empty
"passthrough" frames for thinking deltas, `message_delta` without `usage`,
and Responses string-`input` dropped in responses→chat conversion).

---

## 4. Adding a new scenario

1. Define a `FooScenario() Scenario` function in
   `vmodel/benchmark/scenario/builtins.go` with `MockResponses` for all 4
   formats (nonstream + stream).

2. Add it to `AllScenarios()`.

3. Write `Assertions` that validate the converted result (HTTP status,
   content, tool calls, etc.), plus the upstream-independent `Structural`
   tier (status/shape/counts) used when the response is not test-controlled
   (replay's vmodel/real upstreams).

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

### 9.1 Request content-shape regression suite (`content_shapes.go`)

**Why this exists.** Every matrix request is built by `buildRequest`
(`testenv.go`) from the fixed `harnessPrompt` — a single-turn plain-string user
message, identical across every scenario and every client driver
(`client.go`: "the request itself varies only by (Source, RequestModel,
Streaming)"). `Scenario.MockResponses` only controls the mocked upstream
*response* shape. That split means the matrix, by construction, cannot catch
a bug in how the gateway forwards an unusual *request* content shape upstream
— which is exactly how issue #1427 shipped: a `role: "tool"` message whose
`content` was an array of text blocks (`[{"type":"text","text":"..."}]`,
valid per the OpenAI spec and emitted by several agent frameworks instead of
a plain string) was silently forwarded upstream as an empty string, and no
scenario or assertion in the matrix could have caught it — the matrix never
sends a tool/assistant/system message in that shape in the first place.

Reworking `buildRequest` to vary per scenario would mean threading bespoke
request bodies through every client driver, including the Python/Node/AI SDK
subprocess drivers that wrap real vendor SDKs — those SDKs may not expose an
array-of-text-blocks tool result through their high-level API at all, and the
matrix's client-driver architecture deliberately assumes a shape-invariant
request so results stay comparable across drivers. That redesign is out of
proportion to the bug class.

Instead, `content_shapes.go` follows the same "separate, registry-driven
suite" pattern as `flags.go`: a `contentShapeCase{name, run}` list, driven
through the real gateway with a bespoke JSON body per case (built directly,
bypassing `buildRequest`), asserting on the request the gateway actually
forwarded upstream via `VirtualServer.LastRequest` — not the parsed response.
Cases reuse `flagTB`/`flagRecorder`/`flagAbort` from `flags.go` so they run
under both `*testing.T` (`TestContentShapes`) and the CLI
(`--mode=content_shapes`) without duplicating that plumbing.

See `contentShapeCases()` in `content_shapes.go` for current coverage. Extend
that list, not the matrix's `Scenario`/`buildRequest`, when a future bug is
"the gateway dropped an unusual request shape while forwarding it."
