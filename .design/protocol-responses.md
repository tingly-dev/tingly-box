# Responses Protocol Contract

What tingly-box guarantees on the OpenAI Responses API wire when the
upstream speaks a different protocol. Client-specific behaviour (Codex) is
in `codex.md`; provider findings (DeepSeek) in `deepseek.md`; when a
Responses request is downgraded to Chat at all is decided by
`openai-endpoint-routing.md`.

## 1. Tool-call repair

`RepairResponsesToolCalls` (`internal/protocol/request/responses_tool_call_repair.go`)
normalizes a Responses API input item list before it is converted to Chat
Completions or Anthropic Messages. Both converters call it first.

### Why

Codex speaks the Responses API. Its history only correlates `function_call`
and `function_call_output` by `call_id`: the API has no adjacency rule, does
not require every call to be answered, and OpenAI accepts a standalone
`function_call_output`. Codex relies on all three:

- the user interrupts a turn while a tool runs → `function_call` with no
  output, followed by a new user message (single or parallel calls);
- automations, heartbeat ticks and cross-thread delegation inject a
  `function_call_output` with no `call_id` and no paired call
  (openai/codex #42088, #45318, #45450, #45914).

Chat Completions and Anthropic enforce a stricter shape, adjacency plus a
one-to-one pairing:

| Provider | Rule | Error text |
|---|---|---|
| DeepSeek / OpenAI chat | every `tool_calls` id answered by a `tool` message right after | `An assistant message with 'tool_calls' must be followed by tool messages responding to each 'tool_call_id'. (insufficient tool messages following tool_calls message)` |
| DeepSeek / OpenAI chat | a `tool` message must follow a `tool_calls` message | `Messages with role 'tool' must be a response to a preceding message with 'tool_calls'` |
| Anthropic | every `tool_use` answered by a `tool_result` in the next user message | `tool_use ids were found without tool_result blocks immediately after` |
| Anthropic | a `tool_result` must reference a `tool_use` in the previous message | `unexpected tool_use_id found in tool_result blocks` |

Once a thread's history is dirty every later turn fails the same way, which
is what users see as "Codex → tingly-box → DeepSeek fails sometimes".

### What it does

1. Outputs are indexed by `call_id` and re-emitted right after the call group
   (consecutive `function_call` items) that requested them, whatever their
   position in the input.
2. A call with no output gets a synthesized output carrying
   `[tool call aborted: no output was recorded for this call]`.
3. An output with no call (missing `call_id`, call trimmed from history, or
   a duplicate answer) is rewritten as a user message
   `[tool output: <name or call_id>]\n<output>`; images are kept as
   `input_image` parts. Plain user text has no cross-reference constraint in
   any protocol, so this is safer than synthesizing a fake tool call whose
   tool may not be declared. LiteLLM drops such outputs instead; we keep the
   content.
4. Everything else passes through unchanged and in order.

Item-level converters (`openai_responses_to_chat.go`,
`openai_responses_to_anthropic.go`) then only preserve order: consecutive
calls fold into one assistant message, outputs into tool messages / one user
message of `tool_result` blocks.

### Live verification

DeepSeek was probed with both the pre-repair and the repaired shapes, and
with the real converter output for the three Codex histories: pre-repair
shapes return 400, repaired shapes return 200. The full table is in
`.design/deepseek.md`. Anthropic rules are taken from the official tool-use
documentation; no live Anthropic probe was run.

### Related

- The same class of bug in other projects: LiteLLM #32992 / message
  sanitization (`modify_params`), cc-switch #7074 / #7400, opencodex #4870
  ("adjacency repair"), gptme #3846 (merge dropped parallel results).

## 2. Ids minted by the gateway

When the upstream is not a Responses API, every Responses object the
client sees is synthesized here, including its ids. Clients such as Codex
persist them and replay them verbatim, possibly into a native Responses
upstream, where OpenAI validates replayed `input[i].id`: type prefix
(`fc_`, `msg_`, `rs_`; "Expected an ID that begins with 'rs_'"), charset
`[A-Za-z0-9_-]`, at most 64 characters, and no duplicates within one
request ("Duplicate item found with id"). Under `store: false` nobody
resolves them, so well-formed and unique is the whole requirement. The
model never sees item ids; it does see the `call_id`, which is the
correlation key and is passed through from the upstream unchanged.

Rules (`internal/protocol/ids`):

| Object | Id | Source |
|---|---|---|
| response | `resp_<32 hex>` | minted |
| message item | `msg_<32 hex>` | minted |
| function_call item | `fc_<32 hex>` | minted |
| function_call `call_id` | upstream tool call id (`call_00_…`, `toolu_…`) | passthrough; `call_<32 hex>` only if the upstream gave none |
| Anthropic `message_start.id` | `msg_<32 hex>` | minted, same helper |

The suffix is a random uuid v4. This is the canonical OpenAI shape, it is
what OpenAI itself and vLLM issue, and it meets every replay constraint
without post-processing. Rejected alternatives:

- **timestamps** (previous behaviour): two responses in one second collide,
  and `item_`/`call_`/`toolu_` prefixes were leaking into item ids on some
  paths;
- **derived from upstream ids** (`fc_` + tool call id, `msg_` + response
  id): leaks the provider's shape into the gateway's wire form and has no
  length bound (LiteLLM #41534: Gemini thought-signatures pushed such an
  id past 64 chars);
- **content hash**: deterministic and canonical, but nobody in the field
  does it and the determinism is only useful in tests and recording
  replays, which inject a fixed generator instead.

Invariants the code keeps:

- an id is minted **once per object**; the same id appears in
  `output_item.added`, `output_item.done` and the final
  `response.completed` output (vLLM shipped a bug where these disagreed and
  Codex could not pair them);
- ids on replayed **input** items are never rewritten. Preprocessing only
  adds `type` and flattens `output_text`; the chat and Anthropic
  converters drop item ids entirely and keep `call_id`; the native
  Responses passthrough forwards input untouched.
