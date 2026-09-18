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
