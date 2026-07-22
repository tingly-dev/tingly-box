# Harness Codex client driver (`--client=codex`)

> Companion to [`harness-matrix.md`](./harness-matrix.md) §"Client drivers".
> Adds a **fifth** matrix client driver that reproduces the real
> [OpenAI Codex](https://github.com/openai/codex) CLI's Responses-API wire
> behavior, so the gateway's `openai_responses` **source** path is exercised by
> a request shaped exactly like Codex's — not the generic `{model, input}`
> Responses call the `gosdk`/`python`/`node` drivers send.

---

## 1. Why a Codex driver, and why a subprocess (TS/Node) driver

The task the driver answers: *does the gateway correctly accept and convert the
request the real Codex client actually sends, and correctly re-emit a Responses
stream that a Codex-style consumer can accumulate?*

The existing Responses drivers (`gosdk`, `python`, `node`) all send the SDK's
**minimal** Responses request — `client.responses.create({model, input:"<prompt>"})`
— which touches **none** of the fields that make Codex traffic distinctive:
`instructions`, `reasoning` (effort/summary),
`include:["reasoning.encrypted_content"]`, `store:false`, `prompt_cache_key`,
`tool_choice:"auto"`, `parallel_tool_calls`, `text.verbosity`,
freeform/**custom** tools (the `apply_patch` lark grammar), and Codex's identity
headers (`OpenAI-Beta: responses=experimental`, `conversation_id`, `session_id`,
`originator: codex_cli_rs`, the `codex_cli_rs/<ver>` User-Agent). Those are
precisely the paths a Codex-facing gateway has to get right, and they were
previously untested at the matrix layer.

**Client approach decision — reconstruct Codex's wire shape in a subprocess
driver under `tests/clients/`, the same home as the other real-client drivers
(`node`, `python`, `aisdk`).** Alternatives considered:

| Approach | Verdict |
|----------|---------|
| **Subprocess running the real Codex binary** | Rejected. Codex is an *interactive agent*, not a one-shot "send one request / read one response" tool, so it does not fit the subprocess driver's stdin/stdout contract. Real-binary compatibility is already covered by **Tier C** (`harness agent codex`), which spawns the actual Codex CLI against an in-process gateway. |
| **In-process Go replica** (a `client_codex.go` alongside `client_gosdk.go`) | Rejected. It would duplicate request-shaping logic *outside* the established `tests/clients/*` "real client driver" convention, in a different language from every other foreign-client driver. The `--client` seam exists specifically to drive the gateway from **foreign client stacks**; a hand-rolled Go body is the least foreign option. |
| **A TS/Node subprocess driver that reconstructs Codex's exact request + a Codex-style SSE accumulator** | **Chosen.** Lives at `tests/clients/codex/driver.mjs`, next to `node`/`aisdk`, and speaks the same JSON-over-stdin/stdout contract. Codex itself hand-rolls raw HTTP (no SDK), so the faithful analog is **raw `fetch` + a manual SSE state machine** — a TS port of `codex-rs` `core/src/client.rs`. Because it uses Node's built-in `fetch`, it needs **no npm dependencies** (unlike `node`/`aisdk`), so the CI leg is just `node` on PATH. |

> Note on Codex's implementation language: today's `openai/codex` is **Rust**
> (`codex-rs`); an older revision was TypeScript. The driver does not *run*
> Codex either way — it reconstructs Codex's wire contract — so a TS/Node driver
> is purely a fit-the-harness-convention choice, independent of Codex's own
> language.

`codexClient.Supports()` returns true **only** for `openai_responses` (the
`subprocessClient.sources` restriction), so `--client=codex` visibly skips every
Anthropic / OpenAI-Chat source cell (the matrix's standard
`client %q does not support source` skip) and runs the Responses-source cells
with a Codex-shaped request.

## 2. Wire reference (source of truth)

Distilled from `openai/codex` at tag **`rust-v0.46.0`** — the last release whose
client layer is the clean, self-contained `core/src/{client,client_common,
chat_completions}.rs`; the JSON on the wire is materially identical in
`rust-v0.145.0` (where it moved to the `codex-api`/`codex-client` crates plus
OpenAI-internal-only headers a gateway ignores).

**Request body** — `ResponsesApiRequest` (`core/src/client_common.rs`), populated
in `stream_responses` (`core/src/client.rs`). Wire field names equal the Rust
field names:

```jsonc
{
  "model": "<request model>",
  "instructions": "<full base system prompt>",   // NOT the user turn
  "input": [ /* Vec<ResponseItem>, see below */ ],
  "tools": [ /* function + custom(freeform) tools */ ],
  "tool_choice": "auto",                          // hardcoded
  "parallel_tool_calls": false,
  "reasoning": { "effort": "medium", "summary": "auto" }, // Some only for reasoning models
  "store": false,                                 // true ONLY for Azure Responses
  "stream": true,                                 // always true in real Codex
  "include": ["reasoning.encrypted_content"],     // present only when reasoning is Some
  "prompt_cache_key": "<conversation uuid>",      // == conversation_id/session_id headers
  "text": { "verbosity": "medium" }               // gpt-5 family only
}
```

**`input` items** — `ResponseItem` (`protocol/src/models.rs`), a
`#[serde(tag="type", rename_all="snake_case")]` enum. `type` tags: `message`,
`reasoning`, `local_shell_call`, `function_call`, `function_call_output`,
`custom_tool_call`, `custom_tool_call_output`, `web_search_call`. Most `id`
fields are `#[serde(skip_serializing)]`, so Codex echoes items back **without**
the server `id` but **keeps** `call_id` and (on reasoning) `encrypted_content`.
`ContentItem` content types: `input_text`, `input_image`, `output_text`.
`Prompt::get_formatted_input()` prepends up to two synthetic `user`/`input_text`
messages (environment context, user instructions) ahead of the real turn.

**Tools** — `ToolSpec` (`client_common.rs`), `#[serde(tag="type")]`. Variants:
`function` (`ResponsesApiTool{name,description,strict,parameters}`), `local_shell`,
`web_search`, and `custom` — the Rust variant is `Freeform(FreeformTool)` but it
**serializes as `"type":"custom"`** with
`format:{type:"grammar",syntax:"lark",definition:"<grammar>"}`. `apply_patch`
ships either as this custom/grammar tool or as a plain `function` tool with one
`input` string param, per model family.

**Headers** (`client.rs` + `default_client.rs` + `model_provider_info.rs`):
`Authorization: Bearer <token>`, `OpenAI-Beta: responses=experimental`,
`conversation_id: <uuid>`, `session_id: <uuid>`, `Accept: text/event-stream`,
`originator: codex_cli_rs`, `User-Agent: codex_cli_rs/<ver> (<os>; <arch>)`,
`version: <ver>`, and `chatgpt-account-id` only under ChatGPT OAuth.

**SSE / response** — `ResponseEvent` (`client_common.rs`); dispatch in
`client.rs` matches the event `type`. The real content arrives as **whole items**
on `response.output_item.done`; text/reasoning deltas
(`response.output_text.delta`, `response.reasoning_summary_text.delta`,
`response.reasoning_text.delta`) are for live UI; `response.completed` carries the
final `id` + token usage (emitted after the stream ends); `response.failed` maps
to an error (context-window vs. generic, with `retry-after`). Many event types
are explicit no-ops (`response.output_text.done`, `response.content_part.done`,
`response.function_call_arguments.delta`, …).

**Chat path** — when `provider.wire_api == "chat"`, `chat_completions.rs`
sends `{model, messages, stream:true, tools}` and `AggregatedChatStream`
reshapes chat deltas back into a Responses-like `OutputItemDone`+`Completed`
sequence. The built-in `openai` provider uses `wire_api:"responses"`, which is
what this driver models. (Codex's Chat path is not modeled by the driver; the
gateway's Chat handling is already covered by the other Responses/Chat cells.)

## 3. What the Node driver reproduces (`tests/clients/codex/driver.mjs`)

- **Request:** the `ResponsesApiRequest` shape above — `instructions`, two
  synthetic `input` messages (env context + the shared harness prompt) as
  `input_text`, the `shell` **function** tool + the `apply_patch` **custom**
  (lark grammar) tool, `tool_choice:"auto"`, `parallel_tool_calls:false`,
  `reasoning{effort,summary}`, `store:false`, `include:[reasoning.encrypted_content]`,
  `prompt_cache_key`, and `text.verbosity`.
- **Headers:** the full Codex identity set (§2), via raw `fetch` (no SDK).
- **Streaming vs. not:** real Codex is always `stream:true`. The matrix drives a
  streaming *axis*, so the driver honors the request's `stream` flag: streaming
  sends `stream:true` + `Accept: text/event-stream` and accumulates the SSE with
  a Codex-style `CodexAccumulator` (mirrors the `ResponseEvent` loop);
  non-streaming sends `stream:false` and parses the JSON body. The non-streaming
  path is the one documented deviation from real Codex, kept so the driver
  covers the matrix's non-streaming Responses cells.
- **Response accumulation** (`CodexAccumulator`): prefers whole items from
  `response.output_item.done` **and** the terminal
  `response.completed`/`incomplete` `output[]` array for message text and
  `function_call` tool calls, falls back to `output_text.delta` accumulation,
  gathers `reasoning_*` deltas as thinking, and reads status + usage + model from
  the terminal event. `response.failed` is surfaced as an in-band error.
- **Errors:** gateway 4xx/5xx are reported in the driver's `error` envelope
  (`http_status`/`raw_body`), never as a non-zero exit — so error-scenario
  assertions still run (the subprocess driver contract).

The Go side (`internal/protocoltest/client_subprocess.go`, `NewCodexClient`)
wires the driver into the matrix and restricts `Supports()` to
`openai_responses` via the new `subprocessClient.sources` field.

## 4. Running

```bash
go run ./cli/harness matrix --mode=single --client=codex
go run ./cli/harness matrix --mode=single --client=codex --scenario text --streaming

# e2e (go test): drives the real Node driver through the full matrix
go test -tags e2e ./internal/protocoltest/... -run TestHarness_Codex
```

Needs only `node` on PATH (built-in `fetch`, no `npm install`). CI: a
`matrix-single-codex` leg in `.github/workflows/harness-matrix.yml` (setup-node,
no dependency install step).

## 5. Known-defect interaction

The Responses-**source** `tool_use` path is already on the matrix skip list
(`protocoltest.skipSourceScenarios["openai_responses|tool_use"]`), so the Codex
driver inherits that skip for the tool_use cells until the gateway's
Responses-source tool_call conversion is completed — the driver does not paper
over it. This is a pre-existing, separately-tracked gateway defect; closing it
is out of scope for the client driver itself.
