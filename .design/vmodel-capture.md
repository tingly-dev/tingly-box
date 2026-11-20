# VModel capture — catching what a real client sends

Status: **proposal, nothing implemented.** Written after the Claude Code 2.1.328
wire analysis (`.research/claude-code/2.1.328.md` §9, branch
`tp/analyze-latest-claude-code-reque-954264`), which had to hand-write a Python
`fake_api.py` because `tb vmodel` could not do the job. Index:
[`vmodel.md`](./vmodel.md). Related: [`vmodel-definitions.md`](./vmodel-definitions.md),
[`recording.md`](./recording.md), [`harness-agent-testing.md`](./harness-agent-testing.md).

## Problem

Every Claude Code (CC) release changes the wire: `anthropic-beta` order, identity
headers, the billing header and `cch`, body fields. The only trustworthy source is
the **real binary talking to a fake API**, and that is a recurring job (each CC
upgrade, `.design/claude-code-oauth-compat.md` §B6). vmodel is the project's
virtual upstream, but today it can only *answer* requests; it cannot *keep them*,
and it only answers for model ids it was configured with.

The scenario to support, concretely (from the 2.1.328 run):

1. start a local fake API;
2. run the real `claude -p` under `env -i`, an isolated `HOME` and a fake token,
   across a matrix (OAuth / API key × proxy / direct × Sonnet / Haiku / Opus[1m]);
3. keep, per request: header values **as sent**, the body **byte for byte**
   (`cch` is computed over the exact bytes), and the arrival order;
4. let the CLI finish a normal round so it does not retry or error;
5. hand the captures to whoever analyses them (diffing is out of scope here).

## 1. Current state (read from the code, not run)

| Question | Today | Where |
| --- | --- | --- |
| Standalone listener? | **Yes.** `tb vmodel [--config f]… [--host --port] [--json]` binds `127.0.0.1:0`, prints `url` / `anthropic_base_url` / model ids, no DB, no gateway. Routes: `/v1/messages`, `/v1/chat/completions`, `/v1/responses`, `/v1beta/models/*`, `/v1/decisions`, `/healthz`, `/v1|/openai|/anthropic/.../models`. | `internal/command/vmodel.go`, `vmodel/spec/runtime.go` |
| Records requests? | **No** in `tb vmodel`. Two partial things exist elsewhere: (a) `vmodel/benchmark` capture middleware keeps only the **last** request per endpoint kind / per path (`Method, Path, Headers http.Header, Body []byte`), no response, no order, in-process test API only; (b) gateway `obs` recording (`.design/recording.md`) records *gateway* traffic at pipeline points. | `vmodel/benchmark/capture.go`, `internal/obs` |
| Raw fidelity of what exists? | (a) body bytes are exact but one slot per path; headers are Go-canonicalized and unordered. (b) `obs.RecordRequest` is `Headers map[string]string` + `Body map[string]interface{}`: duplicates, order, key order and bytes are gone, so it **cannot** reproduce `cch` or prove beta/header order. `recording.md` P6 already notes no layer records the true wire request; its Phase 3 `recordTransport` is the gateway-side analogue. | `internal/obs/sink.go` |
| Answers by protocol? | **Yes** for Anthropic Messages (JSON + SSE), OpenAI Chat, Responses, Google. Content/tool calls/usage/latency via `mock`; exact SSE via `recording` (`raw_stream`); status sequences via `sequence`. | `vmodel/spec` |
| Any model name? | **No.** Lookup is exact on `model`; unknown → 404 (`Runtime.ServeHTTP`; `GenericRegistry.Get` has no fallback). `claude -p --model claude-sonnet-4-6` against `tb vmodel` gets 404 unless a definition with that exact id was supplied (and `[1m]` is stripped on the wire, so the id differs from the CLI flag). | `vmodel/spec/runtime.go` |
| Typed body parse? | `Handler.Messages` binds the body into the SDK-typed `AnthropicBetaMessagesRequest` (`?beta=true`) before choosing a model. Unknown/new union values in a new CC body (`thinking:{type:"adaptive"}`, `cache_control.scope`, `diagnostics`) could 400 there. **Unverified** — needs a run with a real body. | `vmodel/virtualserver/handler.go` |
| Auxiliary endpoints? | Anything unrouted → protocol-shaped 501 (`NotSupported`). `count_tokens`, `/api/oauth/*`, `/api/event_logging/*`, `/v1/messages/cache_touch` all land there; no stubs. | `vmodel/virtualserver/handler.go` |
| Response headers CC consumes? | None set: no `request-id` (CC stores it as the next request's `cc_prev_req`, direct mode only), no `anthropic-ratelimit-*`. | grep over `vmodel/` |
| Multi-turn scripts? | `sequence` steps are `{status, content, repeat}` — no tool calls. A "tool_use turn, then end_turn" script (needed to see `cc_prev_req`, `cc_turn_index > 1`, tool-result blocks) is not expressible except via a Go `builtin`. | `vmodel/sequence.go` |
| Real CLI as the client? | Only through the gateway: `harness agent --mock` runs `claude` → in-process **gateway** → VirtualServer, with `cmd.Env = os.Environ()` (host env leaks: the `CLAUDE_CODE_ENTRYPOINT` / real-token pitfall of the 2.1.328 notes) and a generated `--settings`. Good for "does tingly work with CC"; wrong for "what does CC send". | `cli/harness/agent.go` |
| OAuth token endpoint? | Not served. And stock CC cannot be pointed at a local one: token URL is `platform.claude.com/v1/oauth/token`, `CLAUDE_CODE_CUSTOM_OAUTH_URL` is allow-list only (2.1.328 §4.1; whether localhost is admitted is **unverified**). Inference with `CLAUDE_CODE_OAUTH_TOKEN=<fake>` needs no token endpoint. | research §4 |
| Proxy vs direct? | A *client-side* switch (`_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1` makes CC behave as if talking to api.anthropic.com). The server needs nothing; the matrix runner sets it. | research §9 |

## 2. Gaps for the capture scenario

| # | Gap | Severity for the 2.1.328 job |
| --- | --- | --- |
| G1 | No capture at all in `tb vmodel` (no per-request log, no raw body, no order). | blocker |
| G2 | Unknown model id → 404; real CLIs send ids vmodel never heard of. | blocker |
| G3 | Header names/order/casing are lost by `net/http` (`x-app`, `anthropic-version` arrive as `X-App`, `Anthropic-Version`). Value order *inside* `anthropic-beta` is kept; header-name casing/order is not. Matters for fingerprinting claims. | medium; opt-in raw tee solves it |
| G4 | Typed body binding may reject new CC bodies before anything is recorded or answered. | unknown → must be tested; capture must be outside it |
| G5 | Non-messages traffic (telemetry, oauth, count_tokens) gets a bare 501 and is not recorded; we cannot even list which endpoints a CC version calls. | medium |
| G6 | No `request-id` / ratelimit response headers; `cc_prev_req` and quota behaviour cannot be observed. | medium |
| G7 | No scripted tool-use turns → multi-request sessions (`cc_prev_*`, turn index) cannot be captured. | later (needs a definition-contract change) |
| G8 | No standard way to launch the client safely (`env -i`, isolated HOME, onboarding seed, `-p` flags live in a prose snippet). | low: a documented recipe is enough; no script owned by us |
| G9 | Diff / summary tooling. | **dropped**: not our job; captures are the deliverable |
| G10 | Capture format is ad hoc (`req.json`, `req*.raw`). | medium: one raw-first format |
| G11 | Real OAuth login/refresh endpoint capture; real-server response capture (forwarding). | out of scope here (see non-goals) |

Not gaps: serving Anthropic/OpenAI/Responses protocols, standalone lifecycle,
`--json` readiness, strict definition loading.


## 3. Decision

**Raw first.** The capture's ground truth is the exact bytes the client put on
the TCP connection. Everything else (parsed headers, a JSON index) is a
convenience derived from, and never a replacement for, those bytes. Nothing is
decoded, canonicalized, re-ordered, redacted or pretty-printed in the capture
files.

One flag on the **existing** command (keeps the "no extra `serve` command" rule
of `vmodel-definitions.md`):

```sh
tb vmodel --capture ./cap/oauth-direct-sonnet [--config models.yaml] [--json]
```

Scope is deliberately small: a capture tap, an unknown-model fallback so the
client gets a normal answer, and a recipe. No diffing, summarizing, report
generation or committed-evidence format.

### 3.1 The tap

New package `vmodel/capture` (imports nothing from `internal/`), wired in
`internal/command/vmodel.go` only:

- A `net.Listener` wrapper whose `Accept` returns a `net.Conn` that tees every
  `Read` into `raw/conn-NNNN.in` (append-only, flushed per read). This sits
  *below* `net/http`, so header-name casing, header order, duplicate headers,
  the exact request line, chunked framing and `Expect: 100-continue` are all
  preserved; nothing the typed body binding or the router does can lose or
  reject a capture (G1, G3, G4, G5). Plain HTTP only (clients are pointed at
  `http://127.0.0.1`); TLS interception is a non-goal.
- An `http.Handler` wrapper (outermost) that, per request, appends one line to
  `index.jsonl` pointing into the raw files, and tees the response.

Why a TCP tee rather than re-serializing in the handler: Go's `net/http`
canonicalizes header names and gives an unordered map; re-serializing would
fabricate a request that never existed, which is exactly what the capture must
not do. The tee needs no HTTP parsing, is correct for keep-alive and chunked
bodies, and cannot affect the response path.

### 3.2 Layout

```
<dir>/
  meta.json            # tb version, argv, listen URL, start time (what the runner knows is added by the caller)
  raw/conn-0001.in     # verbatim client→server bytes of TCP connection 1 (ground truth)
  raw/conn-0001.out    # verbatim server→client bytes (what we answered; for stream/retry analysis)
  index.jsonl          # one line per request, arrival order
```

An `index.jsonl` line only locates things; it carries no information that is
not already in `raw/`:

```json
{"seq":1,"t":"2026-11-20T10:00:00.123Z","conn":1,"in":[0,90311],"out":[0,1204],
 "method":"POST","target":"/v1/messages?beta=true","status":200}
```

- `in` / `out` are byte ranges in `conn-N.in` / `conn-N.out`. A request starts
  where the previous exchange on that connection ended (HTTP/1.1 clients do not
  pipeline, so the offset sampled when the previous response completes is the
  start of the next request); the last range ends at EOF. `meta`-like fields
  (`method`, `target`, `status`) are for `grep`, nothing more.
- `seq` is dense and 1-based in request-arrival order. A request whose response
  never completes still gets a line (`status` absent, `out` open-ended), so
  aborted/retried calls are visible.
- Splitting a range into head and body is the reader's job (it is `\r\n\r\n`
  plus `Content-Length` / chunked); a tiny `capture.Read(dir)` helper in Go may
  do it for tests (e.g. the existing `LiveCaptures` check), but the format does
  not depend on it.

### 3.3 No redaction in the files

"Most raw" and redaction conflict, and raw wins: tokens, `device_id`,
`session_id` and local paths stay in the files. The mitigation is operational,
not in the format: the capture directory is scratch (default under the
gitignored `.tmp/`, never committed), and the documented recipe only ever uses
a fake token and an isolated `HOME`. As a guard, `tb vmodel --capture` prints a
warning (does not alter anything) when it sees an `Authorization` / `x-api-key`
value that does not carry an obvious fake marker. Sharing a capture is then a
manual, deliberate step by whoever owns it.

### 3.4 What the server answers (so the client finishes a normal round)

Capture exists to observe the client, so the answer only has to be good enough
that the client does not retry or abort:

- **Unknown model ids** (G2): in capture mode a request for an id nobody
  defined is served by a built-in `capture-default` mock (short fixed text,
  filled `usage`) instead of 404. Explicit definitions (`--config`) still
  match first. Implemented as an option on `spec.Runtime` that only `--capture`
  sets, so the production `/virtual/v1/*` and the "unknown model → 404"
  contract of `vmodel-definitions.md` are unchanged.
- **`request-id` response header** on Anthropic responses in capture mode
  (`req_vmodel_<seq>`), because CC feeds it back as the next request's
  `cc_prev_req` (direct mode) (G6).
- **Auxiliary endpoints** (G5): always captured (the tap is outermost). Answered
  from a tiny table so the CLI proceeds — `POST /v1/messages/count_tokens` →
  `{"input_tokens": <n>}`; telemetry/event-logging/bootstrap → `200 {}`;
  anything else keeps the protocol-shaped 501. `/api/oauth/*` is captured if it
  ever arrives, not simulated.
- **Typed binding** (G4): the answer path still binds `/v1/messages` into the
  SDK type. If a real 2.1.328 body 400s there, the capture is already safe; the
  follow-up is to let capture mode answer without binding (peek `stream`, serve
  the default mock). Decided from evidence in Phase 1, not guessed.

### 3.5 Recipe (documentation, not a tool)

Documented in `.design/claude-code-oauth-compat.md` §B2.4 (replacing the Python
`fake_api.py`) and in `vmodel/README.md`; the existing rules stay the same:

```sh
tb vmodel --capture "$OUT" --json            # prints {"url": "...", ...}
env -i PATH="$PATH" TERM=xterm HOME="$ISO_HOME" \
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_TELEMETRY=1 \
    DISABLE_ERROR_REPORTING=1 DISABLE_AUTOUPDATER=1 \
    ANTHROPIC_BASE_URL="$URL" CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-fake \
    [_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1] \
    claude -p "say hi" --model claude-sonnet-4-6 < /dev/null
```

`env -i`, an isolated `HOME` (with `{"hasCompletedOnboarding":true}`) and a fake
token are mandatory (each one bit the 2.1.328 run). One `tb vmodel` process per
matrix cell keeps captures separate. Whether to wrap this in a script is left to
whoever runs the matrix; we do not own one.

## 4. Relationship to harness / replay

`harness agent --mock` (CLI → gateway → vmodel) answers "does the gateway work
with this CLI"; capture (CLI → vmodel) answers "what does the CLI send". Different
topology, not merged. Because raw requests are plain HTTP bytes, a captured
request body can be fed to `harness replay` as a fixture body by hand; automating
that, or putting the tap behind the gateway's in-process VirtualServer, belongs
with the deferred "user definition files for agent --mock / replay" item
(`cli-harness-refactor.md` §5.4) and is not part of this work.

## 5. Non-goals

- Diffing, summarizing or reporting on captures; a committed evidence format.
- OAuth login/refresh capture: stock CC talks TLS to `platform.claude.com`; a
  plain-HTTP local server cannot stand in and the URL override is allow-listed.
  Needs a TLS-intercepting proxy; different tool. The bearer-token *inference*
  request is fully covered with `CLAUDE_CODE_OAUTH_TOKEN=<fake>`.
- Forward/record mode (proxying to the real API to keep real response headers):
  needs real credentials and outbound traffic; separate proposal if ever wanted.
- Interactive (TTY) capture, scripted multi-turn tool-use responses (a change
  to the `sequence` definition contract), TLS, response-body decoding.

## 6. Phased plan

| Phase | Content | Touches | Risk |
| --- | --- | --- | --- |
| **1 Raw tap** | `vmodel/capture`: conn-tee listener, handler wrapper, `index.jsonl`, `meta.json`; `tb vmodel --capture`; token-marker warning; tests (`httptest` + raw `net.Dial` client proving header case/order, duplicates, chunked body, keep-alive offsets, aborted request). | `vmodel/capture` (new), `internal/command/vmodel.go` | low |
| **2 Answer well enough** | `capture-default` fallback for unknown models, `request-id`, `count_tokens`/telemetry stubs; try a real-shaped `/v1/messages?beta=true` body to settle G4. | `vmodel/spec/runtime.go` (option), `vmodel/virtualserver` | low–medium |
| **3 Recipe + docs** | Replace the Python snippet in `claude-code-oauth-compat.md` §B2.4; `vmodel/README.md`; an optional Go helper `capture.Read(dir)` and point the `LiveCaptures` test at it. | docs, `internal/client` test | low |
| later | Scripted multi-turn responses; tap behind the gateway for `harness`. | contract change / `internal/protocoltest` | medium |

Recommended go-ahead: Phases 1–2 (3 is small docs). No `internal/` backend logic
is touched; the only `internal/` file is the CLI command wiring.

## 7. Open questions

1. `request-id` on all vmodel Anthropic responses or capture mode only
   (proposed: capture only).
2. `raw/conn-N.out` always, or behind a second flag? (Proposed: always; it is
   what lets you see exactly what the client was answered when it retried.)
3. Should `--capture` imply a fixed default dir under `.tmp/` when given no
   value, or always require a path? (Proposed: require a path.)
4. G4 is unverified until Phase 2 sees a real 2.1.328 body (the original
   captures sit in the other thread's scratch directory, not on its branch).
