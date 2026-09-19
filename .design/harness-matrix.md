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
> §10.1. The matrix itself always sends the same fixed single-turn prompt and
> only varies the mocked *response* shape, so it cannot catch bugs in how
> unusual *request* shapes are forwarded upstream.
>
> Related: prompt-cache request metadata is covered by `cache_controls.go` —
> see §10.2. It validates cache/no-cache over every single hop and every ABA
> idempotent chain.
>
> Related: whether two *consecutive* requests of one conversation still share a
> prefix by the time they reach the upstream is covered by `cache_prefix.go` —
> see §10.4. Every section above validates one request at a time, which is
> blind to the failure that actually costs money: a gateway that re-serializes
> history differently from turn to turn, invalidating the upstream cache while
> every individual request stays valid.

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

Two further suites round out coverage on orthogonal axes and are documented
in their own sections below: per-rule **flag** behavior (§10, detailed in
[`rule-flag-testing.md`](./rule-flag-testing.md)) and request **content-shape**
regressions (§10.1).

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
  modes, and filter/config methods (`OnlyScenarios`, `OnlySources`, etc.),
  plus CLI-only knobs (`BatchCount`, `RecordDir`, `Client`) set via `With*`
  methods.

- **`RoundTripResult`** — normalized output: `Content`, `Role`,
  `FinishReason`, `ToolCalls`, `Usage`, `StreamEvents`, etc.

- **`TestResult`** — the outcome of one combination (`Passed`, `Skipped`,
  `Errors`, `Duration`, plus `Batch*` fields when `BatchCount > 1`). Every
  `Execute*` entry point returns `[]TestResult` uniformly, so the CLI's
  table/JSON printer and the `testing.T` bridge (`reportTestResult`) share
  one shape instead of each section inventing its own.

### Shared helpers

| Helper | Purpose |
|--------|---------|
| `streamMode(bool)` | Returns `"stream"` or `"nonstream"` for test names |
| `streamingSkipReason(Scenario, bool)` | Checks streaming compatibility, returns skip reason |
| `semanticEquivalenceErrors(label, r1, r2)` | Compares two results field-by-field, returns `[]AssertionError` |
| `assertSemanticEquivalence(t, label, r1, r2)` | Delegates to above, calls `t.Errorf` per error |

### Section drivers (`section.go`)

Every section (single-hop, transitive, idempotent, flags, content_shapes,
cache_controls) contributes only its combos and per-combo execution; the env-per-scenario
lifecycle, setup-failure fan-out, and `testing.T` scaffolding live once in
`internal/protocoltest/section.go`:

| Driver | Used by | Purpose |
|--------|---------|---------|
| `Matrix.executePerScenario(skip, combosFor)` | `ExecuteAll` / `ExecuteAllTransitive` / `ExecuteAllIdempotent` | CLI-side: one env per scenario, scenarios run concurrently (§3.1), combos within a scenario run sequentially against the shared env |
| `Matrix.runPerScenario(t, skip, run)` | `RunTransitive` / `RunIdempotent` | `testing.T` counterpart: one env per scenario subtest, `t.Parallel()` scenarios, combos sequential |
| `Matrix.runRecorderCases(cases)` | `ExecuteAllFlags` / `ExecuteAllContentShapes` / `ExecuteAllCacheControls` | CLI-side: one env per case, cases run concurrently (§3.1) |
| `runRecorderCase(case)` | `runRecorderCases` | Runs one `flagTB`-style case body under the recording shim (`flagRecorder`), returns its `TestResult` |
| `setupFailureResult(base, err)` | `executeScenarioCombos` / `runRecorderCase` | Shared "env failed to boot" `TestResult` construction |

A `scenarioCombo` is `{meta TestResult, run func(*TestEnv) TestResult}` — the
`meta` doubles as the setup-failure result when the scenario's env cannot be
created. A `recorderCase` is the equivalent for the
flags/content_shapes/cache_controls suites: it carries result metadata plus
`run func(flagTB, *TestEnv)`. (Single-hop
`Matrix.Run(t)` keeps its own deeper subtest nesting —
`scenario/source/target/mode` with one env per leaf — because its test-name
contract and per-leaf parallelism differ from the per-scenario sections.)

---

## 3. CLI execution model

Two runtime properties of `cli/harness matrix` sit outside the test
*definitions* above but matter for anyone running or profiling it.

### 3.1 Parallelism

Independent env-backed units — scenarios in `executePerScenario`, cases in
`runRecorderCases` — run concurrently via `runIndexed` (an
`golang.org/x/sync/errgroup.Group` with `SetLimit`), mirroring the
`t.Parallel()` the go-test entry points always had. Combos *within* a
scenario still run sequentially against the shared env — routes are keyed by
`(source, target, scenario)`, so this is purely about overlapping I/O wait,
not a correctness requirement. Results land in index-addressed slots, so
output order — and therefore `--json` output — is identical to a sequential
run regardless of goroutine completion order.

The concurrency cap (`Matrix.sectionParallelism()`) is
`min(GOMAXPROCS, maxSectionParallelism)` — currently 8 — not raw core count
(see §8 for why). It drops to 1 (fully sequential) whenever `--batch` or
`--record-dir` is set: `--batch` measures per-request timing that parallel
load would skew, and `--record-dir` captures request/response traffic for
replay that concurrent runs would interleave.

### 3.2 Env-boot cost

Each env boots a full `httptest.Server` + SQLite-backed `AppConfig` in a
fresh temp dir. The single most expensive step of that boot is
`internal/server/config`'s enterprise-context RSA-2048 key generation
(~100-600ms of prime search) — a one-time cost for a real install, but every
harness env is a fresh install by construction, and one CLI run boots dozens
of them.

`internal/protocoltest/testenv.go`'s `preseedEnterpriseContextKeys` amortizes
this: one key pair is generated per process (`sync.OnceValues`) and written
into every env's key slots before `config.NewAppConfig` runs, so
`ensureEnterpriseContextRS256KeyPair` finds the files already present and
skips generating its own. `internal/server/config/enterprise.go` only
exports the generic primitives this uses — `GenerateEnterpriseContextPEMs`,
`WriteEnterpriseContextKeys`, `EnterpriseContextKeyPaths` — the same ones
`ensureEnterpriseContextRS256KeyPair` already used internally; see §8 for why
the caching *policy* itself lives in the harness, not the production config
package.

Together, §3.1 and §3.2 took (4-core machine):

| | before | after |
|---|---|---|
| `matrix` (default) | ~15s | ~3s |
| `matrix --mode=all` | ~41s | ~8s |
| go test `./internal/protocoltest/...` (full package) | ~60s | ~17s |

The go-test path was already parallel via `t.Parallel()`, so keygen CPU was
its only bottleneck — it benefits from §3.2 alone.

---

## 4. How to run

### go test

Broad (pair × scenario × streaming) single-hop/two-hop/idempotent coverage no
longer runs under `go test` at all — it lives *only* in the CLI (below). There
is no `TestHarness` entry point and no `e2e` build tag; `matrix_config_test.go`
and `roundtrip_test.go` keep only two things: config-level guards on the
matrix definition itself (pair/scenario/chain shape) and focused round-trip
cases the CLI matrix doesn't cover (see those files' own doc comments). The
suites that *do* still run as regular `go test` targets, with no build tag,
are:

```bash
# Request content-shape regression suite (see §10.1)
go test ./internal/protocoltest/... -run TestContentShapes

# Prompt-cache request suite (see §10.2)
go test ./internal/protocoltest/... -run TestCacheControls

# Cross-request prompt-cache prefix suite (see §10.4)
go test ./internal/protocoltest/... -run TestCachePrefix

# Vendor-dispatch suite (see §10.3)
go test ./internal/protocoltest/... -run TestVendorTransforms

# Everything in the package (includes duo/routing/failover/content_shapes/...)
go test ./internal/protocoltest/...
```

### CLI (`cli/harness`)

`--mode` selects which sections run. Each section has a `testing.T`-free
executor (`ExecuteAll*`) so the CLI can run it directly — including idempotence
and the rule-flag suite, which would otherwise be go-test-only.

| `--mode` | single (A→B) | transitive (A→B→C) | idempotent (`g(f(A))==A`) | flags (per-rule) | content_shapes (§10.1) | cache_controls (§10.2) | vendor (§10.3) | cache_prefix (§10.4) |
|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| `default` *(no flag)* | ✅ | — | ✅ | — | — | — | — | — |
| `all` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `single` | ✅ | — | — | — | — | — | — | — |
| `transitive` | — | ✅ | — | — | — | — | — | — |
| `idempotent` | — | — | ✅ | — | — | — | — | — |
| `flags` | — | — | — | ✅ | — | — | — | — |
| `content_shapes` | — | — | — | — | ✅ | — | — | — |
| `cache_controls` | — | — | — | — | — | ✅ | — | — |
| `vendor` | — | — | — | — | — | — | ✅ | — |
| `cache_prefix` | — | — | — | — | — | — | — | ✅ |

This mode → section mapping is declared in one place: the `matrixSections`
registry in `cli/harness/matrix.go`. Each entry names the section, lists the
`--mode` values that include it, marks whether it is http-only (`flags`,
`content_shapes`, `cache_controls`, `cache_prefix`, and `vendor` drive raw
requests directly),
and points at its `ExecuteAll*` executor. Adding a section = one registry
entry + extending the `--mode` enum (see §8 for why this replaced a
hand-maintained if-chain).

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
go run ./cli/harness matrix --mode=cache_controls  # single-hop + ABA cache/no-cache
go run ./cli/harness matrix --mode=cache_prefix    # cross-request prefix stability, per client shape
go run ./cli/harness matrix --mode=vendor          # vendor-dispatch (ApplyProviderTransforms) against real vendor APIBase

# Filter by scenario / source / target
go run ./cli/harness matrix --scenario text --source anthropic_v1

# JSON for CI
go run ./cli/harness matrix --json
```

The `flags` section is documented in detail in
[`rule-flag-testing.md`](./rule-flag-testing.md); `ExecuteAllFlags` reports one
`TestResult` per flag (`Name: "flags/<key>"`, `Scenario: <key>`).

The `content_shapes` section is documented in §10.1 below; `ExecuteAllContentShapes`
reports one `TestResult` per case (`Name: "content_shapes/<case name>"`).

The `cache_controls` section is documented in §10.2 below;
`ExecuteAllCacheControls` reports one result per protocol path and stream mode.
Each result verifies both the positive cache request and the negative no-cache
request against the final provider capture.

The `vendor` section is documented in §10.3 below; `ExecuteAllVendorTransforms`
reports one result per vendor fixture and stream mode.

Other CLI flags (`--batch`, `--record-dir`, `--mcp`, `-v`) are documented in
[`cli/harness/README.md`](../cli/harness/README.md); `--batch` and
`--record-dir` additionally force the sections above to run sequentially
(§3.1).

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
(`matrix.go`, key `client|source|scenario`, or the more precise
`client|source|scenario|target|mode` when only one target/streaming
combination is affected — e.g. `aisdk`'s Responses provider can't satisfy
`responses_item_ids_canonical` in streaming mode because `streamText()`
never exposes the raw provider body for that provider, but only when
converting to a non-Responses target) as *visible skips* with a reason —
never silent failures. Drivers must not be weakened to paper over a gateway
bug; the strictness is the point. The gosdk/python bring-up alone surfaced
four real gateway bugs (missing `event:` lines on the v1 stream path, empty
"passthrough" frames for thinking deltas, `message_delta` without `usage`,
and Responses string-`input` dropped in responses→chat conversion).

---

## 5. Adding a new scenario

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

5. Run `go run ./cli/harness matrix --mode=all` to verify it passes across
   all pairs and modes (this is the only entry point that exercises the
   full pair × scenario × mode cross-product — see §4).

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

## 6. Adding a new protocol pair

1. Add the pair to `DefaultPairs()` in `matrix.go`.

2. Ensure the mock provider (VirtualServer) can serve that target format.

3. Two-hop chains are derived automatically — any chain where
   `First.Target == Second.Source` is included.

4. If the pair has known limitations, add a skip entry to
   `skipSourceScenarios` in `matrix.go`.

---

## 7. Test naming conventions

The pair × scenario × mode cross-product runs only through the CLI now (§4);
`TestResult.Name` follows a `scenario/path/mode` pattern there:

```
{scenario}/{source}/{target}/stream|nonstream            # single-hop
{scenario}/{A}→{B}→{C}/stream|nonstream                  # two-hop (transitive)
```

`Matrix.Run(t)` — the single-hop `testing.T` entry point nested as
`TestHarness/single_hop/{scenario}/{source}/{target}/stream|nonstream` — no
longer exists; it and its two-hop/idempotent counterparts were retired when
broad matrix execution moved to the CLI exclusively (§4). The focused
`go test` suites that remain (content_shapes, cache_controls, cache_prefix,
vendor, plus the individual `TestRoundTrip_*` cases) each use their own flat
subtest names — see §10 and `roundtrip_test.go`.

---

## 8. Design decisions

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

**Why a `matrixSections` registry instead of an if-chain on `--mode`?**
Before, CLI dispatch was five near-identical `if m.Mode == ...` blocks
plus a hand-maintained comment describing the same mapping — the two
routinely drifted. A data table is the mapping: adding a section is one
entry, and this doc's mode table (§4) can point straight at the source
of truth instead of re-deriving it.

**Why does the harness cap concurrency at `min(GOMAXPROCS, 8)` instead of raw `GOMAXPROCS`?**
Each concurrent unit boots a full `httptest.Server` + SQLite-backed
config — an I/O/fd/memory cost, not a CPU one. Core count doesn't bound
that cost, so scaling purely with `GOMAXPROCS` would over-commit fd and
memory limits on high-core CI runners for no throughput benefit; a small
fixed ceiling caps it regardless of machine size.

**Why does the RSA key pre-seed (§3.2) live in `internal/protocoltest`, not `internal/server/config`?**
`internal/server/config` ships in the production server binary; the
process-cache-and-reuse *policy* has zero production callers and exists
purely to amortize a cost the harness's "fresh config dir per env"
pattern creates. `enterprise.go` only exports the generic
generate/write/path primitives `ensureEnterpriseContextRS256KeyPair`
already used internally — the caching decision is harness-only
knowledge and belongs in the package that's structurally a test
harness. Production configs are unaffected: they still generate once
per install and reuse the key from disk.

---

## 9. Error scenarios (pre-content vs mid-stream)

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

## 10. Inspecting the forwarded request (capture & flags)

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

### 10.1 Request content-shape regression suite (`content_shapes.go`)

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

The image cases (`*_image_content`) are the multimodal half of this suite,
added for issue #1606 (tool-returned `image_url` parts corrupted in
`role:"tool"` messages). The full catalog of where multimodal content can
appear per protocol, and the conversion contract those cases enforce, lives
in [`multimodal-content.md`](./multimodal-content.md).

### 10.2 Prompt-cache request suite (`cache_controls.go`)

Prompt-cache metadata is also request-shaped and therefore cannot be exercised
by the matrix's fixed request. `cache_controls.go` uses the same raw-request and
provider-capture approach as `content_shapes`, across two path classes:

- **single-hop**: every pair in `DefaultPairs()` (A→B);
- **ABA**: every case in `DefaultIdempotentCases()` plus Anthropic Beta
  variants (A→B→A).

ABA cases reuse `setupChainHopRoute`: the A→B provider points back at the
gateway's B endpoint, so the same request genuinely re-enters the HTTP gateway
and executes B→A before reaching `VirtualServer`. This is distinct from the
`transitive` section, whose two hops are separate requests.

For every path and stream mode, the suite sends:

1. a cached request with two stable-prefix markers and verifies both markers
   plus OpenAI explicit mode at the final provider;
2. the matching no-cache request and verifies that no marker or explicit mode
   was synthesized;
3. an automatic-cache request and verifies Anthropic's top-level
   `cache_control` ↔ OpenAI's `prompt_cache_options.mode="implicit"` through
   both the JSON re-entry boundary and the final provider request.

Run it with:

```bash
go test ./internal/protocoltest -run TestCacheControls -count=1
go run ./cli/harness matrix --mode=cache_controls
```

Every provider `cache_controls.go` dispatches to is a generic httptest URL —
right for protocol-shape fidelity, but invisible to `ApplyProviderTransforms`
(`internal/protocol/ops/request_openai_extensions.go`), which keys its
behavior off the destination's *real* `APIBase` (`api.openai.com`,
`api.deepseek.com`, ...). Concretely: OpenAI's gpt-5.6+ explicit
`prompt_cache_options` / `prompt_cache_breakpoint` fields are not part of the
Chat Completions schema most OpenAI-compatible vendors cloned, and several —
including Azure OpenAI — reject them outright when their schema validation is
strict. `ApplyProviderTransforms` therefore strips those fields by default and
only re-adds them for an allowlisted vendor (`supportsExplicitPromptCache`,
currently just `api.openai.com`). `cache_controls.go`'s generic destination is
correctly off that allowlist, so it can only prove the fields are stripped for
an unrecognized vendor — never that they survive for a recognized one. §10.3
closes that gap.

### 10.3 Vendor-dispatch suite (`vendor_transforms.go`)

This section builds virtual providers whose `APIBase` genuinely matches a
vendor discriminator (`vendorFixture.apiBase`, e.g. `api.openai.com`), and
dispatches to them through `newVendorHostProxy` — a local forward proxy that
relays whatever `Provider.ProxyURL` sends it to the real `VirtualServer`
(preserving method/path/query/headers/streaming). Pairing the fake-but-real
`APIBase` with a `ProxyURL` that resolves it locally means the *same*
`Provider.APIBase` string production dials reaches `ApplyProviderTransforms`'s
matcher, while the bytes never leave the process — no production code changes,
no real network egress to the vendor's actual host.

Each fixture in `vendorFixtures` declares `wantsExplicitPromptCache`; the case
sends a cached and a no-cache Anthropic v1 → OpenAI Chat request and asserts
the final provider capture matches (reusing `assertCapturedCacheState` from
§10.2 — same assertion, different provider identity). Today the fixture table
only exercises the prompt-cache allowlist (`openai_official`,
`generic_openai_compatible`, `deepseek`, `nvidia_nim`); extending it to other
vendor-specific transforms (DeepSeek's `reasoning_content` flip, Gemini's
`thinking_config` mapping, tool-schema filtering, ...) means adding fields to
`vendorFixture` and assertions in `runVendorTransformCase` — the
routing/relay plumbing is already generic.

**Provider UUID must be unique per (fixture, streaming mode).**
`GetGlobalTransportPool` is a process-global cache keyed on provider UUID +
model, not `APIBase`/`ProxyURL`. Two concurrently-running cases that share a
UUID can race on the pool's invalidate-on-`AddProvider` step and end up
dialing through each other's (possibly already-closed) relay — this is what
"final provider received no chat request" flakes under `--mode=all`/`--mode=vendor`
mean if you see them after adding a fixture. `setupVendorRoute` stamps
`streamMode(streaming)` onto the UUID for exactly this reason; keep doing that
for any new fixture dimension.

Run it with:

```bash
go test ./internal/protocoltest -run TestVendorTransforms -count=1
go run ./cli/harness matrix --mode=vendor
```

Alongside the allowlist itself, the section asserts the **wire shape of text
content** per vendor: the compact string for everyone off
`acceptsChatArrayTextContent`, the content-part array for those on it
(`assertCapturedChatTextShape`). The converters emit the array unconditionally
so a moving cache breakpoint cannot change an item's shape (§10.4), which makes
the array the gateway's internal form — `compactOpenAIChatTextContent` picks the
compatible wire form per vendor on the way out. Both branches are fixed per
vendor; neither depends on where a breakpoint sat.

`vendorFixture` declares `wantsArrayTextContent` separately from
`wantsExplicitPromptCache` even though the two production allowlists agree
today. Deriving one expectation from the other would re-encode the coupling the
production split exists to avoid, leaving the suite unable to notice if the two
were ever rejoined.

### 10.4 Cross-request prompt-cache suite (`cache_prefix.go`)

§10.2 asks "does a cache marker survive one conversion?" — a property of a
single request. This section asks the question the token bill answers: **do two
consecutive requests of the same conversation still share a prefix by the time
they reach the upstream?**

Every prompt cache — OpenAI's, ChatGPT's Codex backend, Anthropic's — keys on
the request prefix. A gateway that re-serializes history even slightly
differently from one turn to the next invalidates the cache from the first
differing byte, and nothing in the response says so: the request succeeds, the
answer is correct, and the user is silently re-billed for the whole
conversation. That is exactly how the Codex collapse (#1718) slipped past every
other section — each individual request it sent was valid.

#### Client shapes

A prefix bug can hide in one client's wire shape and not another's, so the
section drives real ones rather than a single synthetic fixture:

| Fixture | Source | Rotates breakpoints | Carries session identity |
|---|---|:---:|---|
| `claude_code` | Anthropic Beta | ✅ (4 rolling ephemeral breakpoints, cached tool definition, two-block system prompt) | `metadata.user_id` |
| `anthropic_sdk` | Anthropic V1 | ✅ (content breakpoints only, no Claude-Code scaffolding) | — |
| `codex_cli` | OpenAI Responses | — (its backend rejects breakpoints) | `prompt_cache_key` |

`codex_cli` also replays `reasoning` items carrying ids in the shape
`internal/protocol/ids` mints, because that is what Codex sends back to us.

#### Properties

Each fixture runs against every distinct target protocol, in both stream modes:

1. **rotation** — the same history with the client's breakpoint parked on
   different blocks. A client's fixed pool of breakpoints rolls forward every
   turn, so this is what naturally happens between two consecutive requests.
   The dispatched request must be identical modulo cache directives.
2. **growth** — turn N+1 appends one exchange. Every item turn N sent must
   arrive byte-identical, and the failure message names the first item position
   that diverged — the position an upstream cache would stop matching at.
3. **affinity** — `prompt_cache_key` must reach the upstream, stay stable
   across turns of one conversation, and differ across conversations. Anthropic
   targets have no such field (their cache is addressed purely by prefix), and
   the suite asserts the gateway does not invent one there.

Comparison strips every prompt-cache directive in either protocol family's
spelling (`cache_control`, `prompt_cache_breakpoint`, `prompt_cache_options`,
`prompt_cache_retention`, `prompt_cache_key`) before comparing: those are hints
*about* the prefix, not part of the content being cached.

#### The Codex boundary

Each fixture additionally runs against a provider wired with a Codex OAuth
identity via `SetupCodexAssemblyRoute`, so the request passes through the real
Codex `RoundTripper` on its way out. There the suite also asserts what the
ChatGPT backend requires — all of which the cache depends on:

- no `prompt_cache_breakpoint` / `prompt_cache_options` / `prompt_cache_retention`
  survives (Codex rejects them);
- no `role: "system"` input message survives — the system prompt must arrive in
  `instructions`, identically however the converter represented it upstream of
  this boundary;
- the `session-id` header is present, stable across turns, and agrees with
  `prompt_cache_key`. ChatGPT derives Responses cache affinity from that header
  (see `codex.md` §5), so a mismatch means one conversation with two affinity
  scopes.

Run it with:

```bash
go test ./internal/protocoltest -run TestCachePrefix -count=1
go run ./cli/harness matrix --mode=cache_prefix
```

#### Known gap: shape stability, not wire-schema legality

This suite proves a converted body's *shape* does not drift between
consecutive requests — it does not prove that shape is *legal* for the role
it's attached to. The mock Responses endpoint behind the matrix binds only
`{Model, Stream, Input}` and never checks a content part's `type` against its
item's `role`, so a regression that tags assistant-authored content
`input_text` instead of `output_text` (see `protocol-responses.md`'s
"Assistant content needs `output_text`, not `input_text`") would serialize
identically turn over turn — stable, just wrong — and this suite would report
success while the real Responses API 400s. Only the unit tests in
`internal/protocol/request` catch that class of bug today.


