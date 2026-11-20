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
5. turn the result into a version-to-version diff.

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
| G7 | No scripted tool-use turns → multi-request sessions (`cc_prev_*`, turn index, tool results) cannot be captured. §7 of the CC notes lists these as still open. | medium |
| G8 | No standard runner: `env -i`, isolated HOME, `hasCompletedOnboarding` seed, matrix, per-cell outputs and the `-p` flags live in a prose snippet. | high (the recurring cost) |
| G9 | No diff/summary tooling; §9 was hand-assembled from `dict(headers)` dumps. | high (the recurring cost) |
| G10 | Capture format is ad hoc (`req.json`, `req*.raw` for the Go `LiveCaptures` test); nothing connects it to recording/replay fixtures. | low–medium |
| G11 | Real OAuth login/refresh endpoint capture; real-server response capture (forwarding). | out of scope here (see non-goals) |

Not gaps: serving Anthropic/OpenAI/Responses protocols, standalone lifecycle,
`--json` readiness, strict definition loading.

## 3. Decision

Add capture to the **existing** `tb vmodel` as one flag, keeping the "no extra
`serve` command" rule of `vmodel-definitions.md`:

```sh
tb vmodel --capture ./cap/oauth-direct-sonnet [--config models.yaml] [--json]
```

Three small parts, each independently useful:

1. **A capture tap** (`vmodel/capture`, a new package that imports nothing from
   `internal/`): an `http.Handler` wrapper placed **outermost**, so it sees
   every request before routing or body binding, including 404/501s and bodies
   the typed parser would reject (G1, G4, G5).
2. **A capture-mode fallback**: with `--capture`, a model id nobody defined is
   served by a built-in default mock instead of 404 (G2). Explicit definitions
   still match first, so `--config` can script specific models.
3. **A standard runner + summarizer** under `script/cc-capture/` (G8, G9).

Why not extend what exists:

- `obs.Record` is the gateway's pipeline record, shaped for the UI/JSONL
  exporters (decoded body map, `map[string]string` headers). Making it
  byte-exact would change a model other code relies on and still would not record
  *non-gateway* requests. Capture is a different observation point (the server
  side of a wire), so it gets its own small type. The convergence point is
  `recording.md` Phase 3 (wire `recordTransport`): when that lands, it can emit
  the same `Exchange` lines. Not coupled now.
- `benchmark`'s recorder is last-per-path by design (assertion helper). It can
  later be rebuilt on the append-only tap; not required here.

### 3.1 Capture directory (the format)

```
<dir>/
  meta.json          # tool version, argv, listen URL, definition ids, start time;
                     # the runner adds label, client argv, client version, env allowlist
  requests.jsonl     # one Exchange per line, arrival order (seq is dense, 1-based)
  bodies/0001.req    # request body, verbatim bytes (never decoded/rewritten)
  bodies/0001.res    # response body as served (SSE stays SSE)
  raw/conn-0003.in   # only with --capture-raw: every inbound byte of that TCP connection
```

An `Exchange` line:

```json
{"seq":1,"t":"2026-11-20T10:00:00.123Z","conn":3,
 "method":"POST","target":"/v1/messages?beta=true","proto":"HTTP/1.1","host":"127.0.0.1:18091",
 "req_headers":[["User-Agent","claude-cli/2.1.328 (external, sdk-cli)"],["Authorization","Bearer sk-ant-oat01-…[redacted len=24]"]],
 "req_body":{"file":"bodies/0001.req","bytes":90112,"sha256":"…"},
 "route":{"format":"anthropic","model":"claude-sonnet-4-6","stream":true},
 "served_by":"capture-default",
 "res":{"status":200,"headers":[["Content-Type","text/event-stream"],["request-id","req_vmodel_0001"]],
        "body":{"file":"bodies/0001.res","bytes":812},"ttfb_ms":3,"dur_ms":9}}
```

Choices and why:

- **JSONL + side files**, not one JSON per request: arrival order is the file
  order, `grep`/`jq -c` work, and the 90 KB bodies stay out of the index. Bodies
  are plain files so `sha256sum`, `diff` and the existing
  `TINGLY_CC_CAPTURE_DIR` Go test (`req*.raw`) can read them with a trivial
  rename/glob change.
- **Headers as an ordered pair list**, not a map: duplicates and the order Go saw
  are kept. Names are Go-canonicalized in this layer (G3); exact casing/order
  is what `--capture-raw` is for.
- **`--capture-raw` is a plain TCP tee** (wrap `net.Conn.Read`, append to
  `conn-N.in`): no HTTP re-parsing, correct for chunked bodies and keep-alive,
  and it is the only way to prove header-name casing/order as sent. Opt-in
  because it also stores credentials unredacted. Plain HTTP only (clients are
  pointed at `http://127.0.0.1`); TLS interception is out of scope.
- **`route` is best-effort** (peeks `model` and `stream` with a lenient JSON
  decode); `null` when the body is not JSON. It never influences serving.
- **Response side matches the recording shape.** `res.body` + status + headers
  is what `spec.Definition` kind `recording` already stores (`raw_body` /
  `raw_stream`). A later `tb vmodel --from-capture <dir> --export rec.yaml` is a
  mechanical conversion (not part of the first phases): captured request →
  harness replay fixture body, captured response → recording definition. That is
  the "unified with recording/replay" link; no new definition kind.
- **Concurrency**: `seq` is assigned when the request head is accepted under a
  mutex; the index line is written when the response completes (so streams that
  never finish still get a line with `res.error`). Writes are append + flush per
  exchange; the tap adds no behavior to the response path (it tees the writer).

### 3.2 Redaction

Principle: **never alter bytes you will analyze; keep secrets out of what gets
committed.**

- `requests.jsonl` header values for `Authorization`, `x-api-key`, `Cookie`,
  `Proxy-Authorization` are replaced by `<scheme/prefix up to 14 chars>…[redacted
  len=N]`. Prefix and length are enough to see the token kind (`sk-ant-oat01-`
  vs `sk-ant-api03-`). `--capture-unredacted` disables it (needed only if the
  value itself is under test).
- Bodies and `raw/` are verbatim, because `cch`, fingerprints and byte-exact
  checks depend on them. They contain a local path, `device_id`, `session_id`.
  So capture dirs are **scratch**, kept outside the repo (default under
  `$TMPDIR`/`.tmp/`, gitignored), and the standard runner only ever uses a fake
  token and an isolated `HOME`.
- What gets committed is the **sanitized summary** (3.4), produced by the
  summarizer, never the raw dir. The research doc previously said the same
  ("raw captures are in the thread scratch dir, not committed").

### 3.3 What the server answers in capture mode

- **Default mock for unknown models** (G2): a built-in definition
  `capture-default` (`kind: mock`, short fixed text, `usage` filled) matched for
  any model id on the chat/messages/responses/google routes when no explicit
  definition matches. Implemented as a fallback in `spec.Runtime` guarded by an
  option that only `--capture` sets, so the production `/virtual/v1/*` and the
  strict "unknown model → 404" test contracts (`vmodel-definitions.md`) are
  untouched.
- **`request-id` response header** on Anthropic responses in capture mode
  (`req_vmodel_<seq>`), so CC's direct mode can emit `cc_prev_req` on turn 2
  (G6). Whether to add it to all vmodel Anthropic responses is a separate,
  small call (real Anthropic always sends it); not forced here.
- **Auxiliary endpoints** (G5): recorded always; answered from a small
  built-in table so CC proceeds — `POST /v1/messages/count_tokens` →
  `{"input_tokens": <len/4>}`, telemetry/event-logging/bootstrap → `200 {}`,
  everything else keeps the protocol-shaped 501. `/api/oauth/*` is *recorded*
  but not simulated (non-goal below).
- **Tool-use scripting** (G7): extend `sequence` steps to optionally reference a
  mock (`step.mock: {tool_calls: […]}`) — turn 1 returns `tool_use`, turn 2
  `end_turn`. This is a change to the shared definition contract, so it is
  its own phase and needs the user's go-ahead; the first captures do not need it.
- **Typed binding (G4)** stays as is for the answer path. If a real 2.1.328 body
  is rejected, the exchange is still captured (tap is outside) and the CLI sees
  400. The fix would then be to loosen binding for capture mode (answer without
  binding: peek `stream` and serve the default mock) — decided from evidence in
  Phase 1, not guessed now.

### 3.4 Standard capture procedure for a new CC release

`script/cc-capture/` (the repo has no `script/` yet; `tests/` and `.research/`
hold the nearest precedents — directory name follows the user's convention):

| Script | Job |
| --- | --- |
| `run.sh --bin <claude> --out <dir> [--cells cells.txt] [--prompt "say hi"]` | For each cell, in a fresh isolated `HOME` (seeded with `{"hasCompletedOnboarding":true}`): start `tb vmodel --capture <out>/<cell> --json`, read the URL, run the CLI under `env -i PATH=… TERM=xterm HOME=… CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_TELEMETRY=1 DISABLE_ERROR_REPORTING=1 DISABLE_AUTOUPDATER=1 ANTHROPIC_BASE_URL=<url> <auth env> [_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1] claude -p … --model <m> < /dev/null`, stop vmodel, write `meta.json` (CLI `--version`, cell, exact env list). `--dry-run` prints the plan. One vmodel process per cell keeps cells separate without URL tricks that would change the wire path. |
| `cells.txt` | Default matrix: `{oauth,apikey} × {proxy,direct} × {sonnet-4-6}` plus `{haiku-4-5, opus-4-8[1m]}` on direct OAuth — the 6 cells of 2.1.328 §9, one line each. Auth is a fake token (`sk-ant-oat01-fake…` / `sk-ant-api03-fake…`). |
| `summarize.py <out> [--prev <prev-summary.json>]` | Reads `requests.jsonl`+bodies of `POST /v1/messages` per cell and emits (a) `summary.json`: identity headers, ordered `anthropic-beta` list, direct-only headers, billing-header fields in order, body top-level keys, `system` block shapes (lengths, cache_control), tool names, `metadata.user_id` key shapes, endpoints seen, with tokens/paths/uuids masked; (b) the §9-style markdown tables; (c) with `--prev`, a diff (added/removed/reordered betas and headers) — the "what changed since last version" section. |

Output lands where the research notes already live:
`.research/claude-code/<ver>.md` §9 is generated/pasted from (b), and
`.research/claude-code/<ver>.capture.json` (the sanitized `summary.json`, tens
of KB at most) is the committed evidence and the `--prev` input next time. Raw
dirs stay in scratch. The Go check stays as it is: `TINGLY_CC_CAPTURE_DIR=<cell
dir> go test ./internal/client/ -run LiveCaptures`, with the test adjusted to
read `bodies/*.req` of `/v1/messages` exchanges (or the runner exports
`req*.raw` copies; pick in Phase 3).

Hard rules baked into the runner (each one bit the 2.1.328 run): always
`env -i`; always isolated `HOME`; refuse a real-looking token (`sk-ant-…` not
ending in the fake marker) unless `--allow-real-token`; never touch `~/.claude`
or `~/.tingly-box`.

## 4. Relationship to harness / replay

- `harness agent --mock` answers "does the gateway work with this CLI"
  (CLI → gateway → vmodel). It stays as is. Capture answers "what does the CLI
  send" (CLI → vmodel). Different questions, different topology; do not merge.
- If someone wants "what does *tingly* send upstream for this CLI", the in-process
  VirtualServer behind the gateway can wear the same tap: expose
  `capture.Wrap(handler, dir)` and let `harness agent --mock --capture <dir>` (and
  `replay --upstream vmodel`) use it. That is the seam to the deferred item
  "user definition files for agent --mock / replay" (`cli-harness-refactor.md`
  §5.4, open): both need harness to accept a vmodel definition/option, so they
  should be decided together. Not in the first phases; needs `internal/protocoltest`
  changes while another thread is reviewing `internal/`.
- Captured **requests** become replay fixtures (body bytes as-is), captured
  **responses** become `recording` definitions: the two conversions in 3.1.

## 5. Non-goals (and the cheaper alternative)

- **OAuth login/refresh capture** (G11). Stock CC talks TLS to
  `platform.claude.com`; a local plain-HTTP server cannot stand in, and URL
  override is allow-listed. Needs a TLS-intercepting proxy with a trusted CA
  (`NODE_EXTRA_CA_CERTS`), a different tool. The bearer-token **inference**
  request shape — the part that matters for tingly's OAuth upstream — is fully
  covered with `CLAUDE_CODE_OAUTH_TOKEN=<fake>`.
- **Forward/record mode** (reverse-proxy to the real API to record real response
  headers such as `request-id`, `anthropic-ratelimit-*`). Valuable, but it needs
  real credentials and sends traffic out; deliberately deferred, to be proposed
  separately with its own consent/redaction story. Until then the
  response-header facts come from bundle analysis.
- **Interactive (TTY) mode** capture (`cc_entrypoint=cli`, `redact-thinking`): needs a
  pty driver; the runner is `-p` only for now.
- A new capture definition `kind` or a second file contract: none added.

## 6. Phased plan

Each phase is independently shippable, vet/test green, and ends with the
runner (when it exists) still working.

| Phase | Content | Touches | Risk |
| --- | --- | --- | --- |
| **1 Tap + fallback** | `vmodel/capture` (Exchange, JSONL/bodies writer, header redaction, tests with `httptest`); `tb vmodel --capture <dir>`; unknown-model fallback to `capture-default`; `request-id`; `count_tokens`/telemetry stubs; record-before-bind. Verify with a hand-fed real-shaped `/v1/messages?beta=true` body (resolves G4) and a scripted client. | `vmodel/capture` (new), `vmodel/spec/runtime.go`, `internal/command/vmodel.go` | low–medium (Runtime option; no `internal/` backend logic) |
| **2 Runner + summarizer** | `script/cc-capture/{run.sh,cells.txt,summarize.py}`, `--dry-run`, safety rules; re-run the 2.1.328 six cells and diff against §9 as the acceptance test; commit `<ver>.capture.json`; update `.research/claude-code/README.md` method + `claude-code-oauth-compat.md` §B6 step 3 to call the runner. Requires running the real `claude` binary in an isolated env. | `script/`, docs | low |
| **3 Raw tee + Go test bridge** | `--capture-raw` TCP tee; `LiveCaptures` reads capture dirs; `--capture-unredacted`. | `vmodel/capture`, `internal/client` test | low |
| **4 Multi-turn scripts** | `sequence` steps with mocks / tool calls; second-request capture (`cc_prev_req`, turn index, tool results) cell in the runner. Changes the definition contract → design review + update `vmodel-definitions.md`/`vmodel-sequence.md`. | `vmodel/sequence.go`, `vmodel/spec` | medium |
| **5 Conversions + harness seam** | `--from-capture … --export` (request → fixture, response → recording); `capture.Wrap` for `harness agent --mock/replay`, together with the deferred definition-file item. | `vmodel/spec`, `cli/harness`, `internal/protocoltest` | medium |

Recommended scope for the first go-ahead: **Phases 1–2** — they remove the
hand-written `fake_api.py` and make the next CC bump a one-command capture +
diff. Phases 3–5 only on demand.

## 7. Open questions

1. `request-id` on all vmodel Anthropic responses, or capture mode only? (Default
   proposed: capture mode only; it is cheap to widen later.)
2. Where should the committed summary live: `.research/claude-code/<ver>.capture.json`
   (proposed, next to the analysis) or under `.design/`?
3. Directory name for scripts: `script/cc-capture/` (user convention; no `script/`
   exists in this repo yet) vs extending `tests/`.
4. Is the unverified typed-binding risk (G4) acceptable to resolve in Phase 1,
   or should it be probed first with a captured 2.1.328 body from the other
   thread (it is in that thread's scratch dir, not on the branch)?
5. Whether `CLAUDE_CODE_CUSTOM_OAUTH_URL` admits `localhost` (would shrink the
   OAuth non-goal) — a one-line probe once the runner exists.
