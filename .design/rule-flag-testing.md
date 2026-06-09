# Rule-Flag Behavior Testing

> For contributors adding or changing a per-rule flag (`typ.RuleFlags`) or
> working in `internal/protocoltest/flag_test.go`.
>
> Sibling of the protocol [`harness-matrix.md`](./harness-matrix.md). The matrix
> tests **protocol conversion** (A→B fidelity) and stays flag-free; this suite
> tests **rule flags** (per-rule request/response behavior). Keeping them
> separate avoids an `pairs × scenarios × flags` blow-up and keeps each
> concern's intent legible.
>
> It is wired into the harness as the **`flags`** section: run it via
> `harness matrix --mode=flags` (also included in `--mode=all`), or as
> `go test -run TestRuleFlags`. See the mode table in
> [`harness-matrix.md`](./harness-matrix.md#3-how-to-run).

---

## 1. What it tests, and why a separate suite

`typ.RuleFlags` are per-rule toggles that change how a request is transformed or
routed (rewrite a field, strip a header, fold a message, pin a session, …).
They are catalogued in `typ.RuleFlagRegistry()` — the single source of truth the
UI and this suite both read.

Before this suite the protocol matrix built every rule with **zero flags**, so
none of these behaviors had gateway coverage — even as flag-driven behavior grew
(e.g. Claude Code / Desktop default-on rules) and a flag-loss regression shipped
because nothing locked the contract.

The suite drives **one request per flag through the real gateway** with
`rule.Flags` set, and asserts the observable effect on either:

- the **upstream request** the mock provider received (`VirtualServer.LastRequest`), or
- the **client response** / chosen endpoint.

It does not re-test protocol conversion — it sets one flag and checks one effect.

---

## 2. Architecture

```
flagScenario()  ── one shared multi-turn mock (all 4 formats, advertises usage)
flagBaseRequest ── one representative multi-turn request that bakes in the
                   material flags act on (rich content, tools, system blocks,
                   max_tokens)
       ↓
SetupRouteWithFlags(src, tgt, flagScenario(), flags)  ── stamps rule.Flags
       ↓
TestEnv.dispatch → real gateway → VirtualServer (mock provider, captures request)
       ↓
assert on VirtualServer.LastRequest(kind)  /  client RoundTripResult  /  EndpointHits
```

Key pieces (all in `internal/protocoltest/`):

| Piece | Role |
|-------|------|
| `flagCase{key, run}` | one flag's full setup→send→assert, keyed by its registry key |
| `flagTB` | the `testing.TB` subset the cases use, so the same bodies run under `go test` (`*testing.T`) and the CLI (a recording shim) |
| `flagScenario()` | the single shared scenario (built from `MultiTurnScenario`'s mocks) |
| `flagBaseRequest(src, model, streaming)` | the unified multi-turn request material |
| `sendFlag(...)` | sends `flagBaseRequest` (+ optional body mutate / headers) through `dispatch` |
| `VirtualServer.LastRequest(kind)` | the forwarded upstream request, for assertions |
| `TestEnv.SetupRouteWithFlags(...)` | wires a route with `rule.Flags` set |
| `Matrix.ExecuteAllFlags()` | CLI executor — runs every case with the recording shim, returns `[]TestResult` |

The cases live in `flags.go` (a non-test file) so the CLI build can see them;
`flag_test.go` is just the two go-test entry points (`TestRuleFlags`,
`TestRuleFlagRegistry_FullyCovered`). Cases assert via the `flagTB` interface,
not `*testing.T` directly, which is what lets `ExecuteAllFlags` reuse the exact
same case bodies from the CLI (its shim records `Errorf`/`Fatalf` into
`AssertionError`s and runs `Cleanup`s).

One unified fixture is intentional: rather than each case crafting a bespoke
request, the multi-turn `flagBaseRequest` already carries the material, so a
case usually just sets its flag and asserts its slice. Only flags whose test is
inherently a field/shape *swap* (`use_max_tokens`, `claude_code_compat`) pass a
small per-case `mutate`.

---

## 3. Coverage matrix

Every flag runs through the gateway with the flag set; the assertion site is
either the **upstream** request the provider received, the **client** response,
or the chosen **endpoint**.

| Flag (registry key) | Route (src→tgt) | Flag value | Asserted effect | Site |
|---------------------|-----------------|------------|-----------------|------|
| `custom_user_agent` | openai_chat→openai_chat *(stream)* | `"HarnessFlagUA/9.9"` | `User-Agent` header overridden | upstream header |
| `use_max_completion_tokens` | openai_chat→openai_chat | `true` | `max_tokens` rewritten to `max_completion_tokens` | upstream body |
| `use_max_tokens` | openai_chat→openai_chat | `true` | `max_completion_tokens` rewritten to `max_tokens` | upstream body |
| `block_tools` | openai_chat→openai_chat | `"web_search"` | `web_search` removed, `keep_me` kept | upstream body |
| `skip_usage` | openai_chat→openai_chat | `true` | no `usage` block in response | client response |
| `thinking_effort` | anthropic_v1→anthropic_beta | `high` | `thinking.type == "enabled"` | upstream body |
| `claude_code_compat` | anthropic_v1→anthropic_beta | `true` | mid-convo `system`-role message folded away | upstream body |
| `clean_header` | anthropic_v1→anthropic_beta | `true` | `x-anthropic-billing-header` block stripped | upstream body |
| `cursor_compat` | openai_chat→openai_chat | `true` | array content flattened to a string | upstream body |
| `cursor_compat_auto` | openai_chat→openai_chat | `true` + `User-Agent: Cursor/...` | flattened (auto-detected by header) | upstream body |
| `openai_endpoint_override` | openai_chat→openai_responses *(provider mode=both)* | `"responses"` | forwarded to `/v1/responses` | endpoint hits |
| `session_affinity` | one rule, **two** upstreams | `3600` + `X-Tingly-Session-ID` | all N requests pin to the first-chosen upstream | upstream hits |
| `vision_proxy_service` | openai_chat→openai_chat + describer | `{describer, vision-model}` | image block described + replaced; describer called; text spliced upstream | upstream body + describer hits |

Notes on the two non-trivial fixtures:

- **`session_affinity`** uses two distinguishable counting upstreams behind one
  rule; pinning is proven by *all* hits landing on one server, none on the other.
- **`vision_proxy_service`** sends a real `image_url` block and configures a
  describer service. The describer mock must serve an **SSE stream** — the
  vision adapter (`describeViaOpenAI`) always uses the streaming endpoint, so a
  non-streaming mock yields an empty description and the proxy falls back to its
  fail-strip path.

---

## 4. The completeness guard

`TestRuleFlagRegistry_FullyCovered` cross-checks the suite against the registry:

- every `typ.RuleFlagRegistry()` key must have a `flagCase`, and
- no `flagCase` may reference a key that is not in the registry.

This is the point of the suite's structure: **a new flag cannot ship without a
behavior test.** Add a flag to the registry without a case here and CI fails.
This closes the silent-omission class of bug (a flag added but never exercised /
silently dropped on a code path).

---

## 5. Adding a new flag

1. Add the flag to `typ.RuleFlags` and `typ.RuleFlagRegistry()` (and wire its
   transform / handler behavior).
2. Add a `flagCase` to `ruleFlagCases()` in `flag_test.go`:
   - set the flag via `SetupRouteWithFlags`,
   - if the unified `flagBaseRequest` doesn't already carry the material your
     flag needs, either extend `flagBaseRequest` (if broadly useful) or pass a
     small per-case `mutate` to `sendFlag`,
   - assert the observable effect on `LastRequest(kind)`, the client response,
     or `EndpointHits`.
3. Run `go test ./internal/protocoltest/ -run TestRuleFlag`. The
   `..._FullyCovered` guard will also confirm the registry and the suite agree.

### Choosing where to assert

- **Request-mutating flags** (field rewrites, header/tool/message changes,
  vision) → assert on `VirtualServer.LastRequest(kind).JSON()` / `.Headers` /
  `.Body`. Pick `kind` from the target: chat→`EndpointChat`,
  responses→`EndpointResponses`, anthropic→`EndpointAnthropic`.
- **Response-affecting flags** (`skip_usage`) → assert on the client
  `RoundTripResult`.
- **Routing flags** (`openai_endpoint_override`, `session_affinity`) → assert on
  `EndpointHits` or distinguishable upstreams.

---

## 6. How to run

```bash
# CLI — the flags section of the matrix
go run ./cli/harness matrix --mode=flags
go run ./cli/harness matrix --mode=flags --json     # one row per flag

# go test — whole suite + the completeness guard (no e2e tag)
go test ./internal/protocoltest/ -run TestRuleFlag

# A single flag
go test ./internal/protocoltest/ -run 'TestRuleFlags/block_tools' -v
```

The suite spins up the full gateway per case (`NewTestEnv` under `go test`,
`NewTestEnvForCLI` under the CLI), so it runs in the ordinary `go test` path —
unlike the e2e-tagged protocol matrix — which keeps the registry guard cheap to
run on every change. In the CLI table each row's `Scenario` column is the flag
key.

---

## 7. Design decisions

**Why separate from the protocol matrix?** Flags are an orthogonal axis. Folding
them into the matrix would multiply `pairs × scenarios × flags` and conflate
"does conversion preserve semantics" with "does this flag do its one thing." The
matrix stays flag-free; flags get one request each here.

**Why one unified fixture instead of per-case requests?** A single multi-turn
`flagBaseRequest` + `flagScenario` makes each case set just its flag and assert
its slice, instead of 13 bespoke request builders. It also keeps the fixture
realistic (multi-turn, rich content, tools, system blocks) rather than a trivial
single-turn text exchange.

**Why a registry-driven guard?** The failure mode we're defending against is a
flag that exists but is never exercised (and silently regresses). Driving the
guard off `typ.RuleFlagRegistry()` makes "untested flag" a build failure rather
than a thing a reviewer has to notice.
