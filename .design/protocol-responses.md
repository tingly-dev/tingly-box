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

### Not a correlation key

Only `call_id` carries cross-request meaning; a later turn's
`function_call_output` must reference it, and it is passed through
unchanged. The minted ids above do not: OpenAI's own Responses API only
makes `response.id`/item ids resolvable via `previous_response_id` or
`GET /v1/responses/{id}`, and tingly-box implements neither (`store` is
not honoured; `GET` returns 404). So a minted id needs no relationship to
the call that produced it — randomizing it is safe, and it closes a minor
leak rather than opening one: the previous scheme (`response.id =
resp.ID`, `"msg_" + resp.ID`) exposed the upstream's own id, and by
extension which provider served the request, to the client.

## 3. Cache-shape invariant on converted requests

**A converted input item's serialized shape must depend only on its content —
never on whether a prompt-cache breakpoint happens to sit on it this turn.**

Anthropic expresses cache boundaries as `cache_control` on a content block;
both OpenAI shapes express them as `prompt_cache_breakpoint` on a content
*part*. A part list is therefore the only shape that can carry a breakpoint,
and the converters used to switch between the two representations per item —
the same bug, independently, on both the Responses and the Chat Completions
side:

| Converter | Item | No breakpoint | With breakpoint |
| --- | --- | --- | --- |
| Anthropic→Responses | user / assistant message | `"content": "text"` | `"content": [{"type":"input_text",…}]` |
| Anthropic→Responses | tool result | `"output": "text"` | `"output": [{"type":"input_text",…}]` |
| Anthropic→Chat | system / user / assistant message | `"content": "text"` | `"content": [{"type":"text",…}]` |
| Anthropic→Chat | tool result | `"content": "text"` | `"content": [{"type":"text",…}]` |

(The Chat side was found by the `cache_prefix` harness section — see
`harness-matrix.md` §10.4 — after the Responses side had been fixed by hand.
Responses→Chat collapses text to the compact string form unconditionally, so
it was never affected.)

That looked like a harmless compaction and was not. A client carries a small,
fixed number of ephemeral breakpoints and **rolls them forward** as the
conversation grows — Claude Code has four — so a block that owned one on turn N
usually does not own it on turn N+1. Under the switch, every turn silently
rewrote conversation history the upstream cache had already been keyed on: the
prefix matched up to the oldest moved breakpoint and missed from there on. The
symptom is a hit rate that oscillates and, when it lands short, reports a small
`cached_tokens` against a large input — the prefix matched the system prompt and
the tools and then diverged. Nothing in the logs attributes it to caching.

So the part-list shape is emitted unconditionally, and a breakpoint only ever
*adds a field* to a part that would have been there anyway. Two consequences
worth stating:

- For the ChatGPT Codex backend, which does not accept breakpoints at all and
  has them stripped at the provider boundary, breakpoint placement now has
  **exactly zero** effect on the dispatched body. `TestCodexBodyIsStable-
  AcrossBreakpointRotation` asserts byte equality across every placement.
- The array form is what travels *through* the gateway, not necessarily what
  leaves it. It is the richer of the two Chat wire forms, and an
  OpenAI-compatible vendor that only accepts a string for system, assistant or
  tool content would reject it. So a second, deliberately independent vendor
  allowlist — `acceptsChatArrayTextContent`, sitting next to
  `supportsExplicitPromptCache` — decides the wire form, and
  `compactOpenAIChatTextContent` collapses all-text content back to a plain
  string for everyone off it. Nothing is lost, because the breakpoints are
  already stripped by then, and both branches stay shape-stable: an
  off-allowlist vendor always sees strings, an allowlisted one always sees
  parts. The two allowlists hold the same single entry today and are still kept
  apart on purpose: "accepts the prompt-cache fields" and "accepts array text
  content" are different questions, and folding them together would silently
  flip a vendor's entire text wire format the day it is added for the cache
  fields alone. The `vendor` harness section (`harness-matrix.md` §10.3) carries
  the two expectations as independent fixture fields for the same reason.
- For native Responses and allowlisted Chat providers the breakpoints still
  ship, unchanged; they are now purely additive.
  `TestResponsesShapeIsStableAcrossBreakpointRotation` asserts that stripping
  the cache directives leaves every placement identical, and the `cache_prefix`
  harness section asserts the same end-to-end through the real gateway for
  every client shape × target protocol.

### The system prompt

A system breakpoint flips a coarser switch — `instructions` (a plain string) vs.
a leading `role: "system"` input message whose parts can carry the breakpoint —
and that one is kept: it is what lets a system-level boundary survive a
Responses→Anthropic round trip, and `applyFirstResponsesCacheBreakpoint`
deliberately relocates `instructions` the same way so a tool-definition boundary
lands on the system prefix rather than after the first user message.

Codex would otherwise see that flip too, since it rejects `role: "system"` input
and `normalizeCodexSystemMessagesJSON` lifts the text back. The lift is
therefore the **exact inverse** of the converters' own join — parts concatenated
verbatim, no trimming, no separator, matching `ConvertBetaTextBlocksToString` —
so both representations reduce to a byte-identical `instructions`. It used to
trim each part and join with `\n\n`, which meant the two paths produced
different instruction strings and a flip invalidated the entire prefix, not just
its tail.

### Assistant content needs `output_text`, not `input_text`

Making the part-list form unconditional (above) turned a latent bug into a
guaranteed one. `responsesTextParts`/`responsesInputTextPart` were written for
*input*-side content — user and system messages — and hardcode
`type: "input_text"`. Before the unconditional form, an assistant message only
took the part-list path when a cache breakpoint happened to sit on it, which
was rare (Claude Code's rolling breakpoints seldom land on assistant turns),
so the mismatch stayed latent. Once every assistant message with text went
through the same builder regardless of breakpoints, so did the mismatch — and
Codex's Responses API rejects `input_text` on assistant-authored content
outright:

```
Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'.
param: input[N].content[0]
```

Fixed by giving assistant text its own path in both directions:

- `responsesOutputTextParts` (`cache_control.go`) is the assistant-side
  counterpart to `responsesTextParts`: it emits `output_text` parts inside an
  `output_message` item (`ResponseInputItemParamOfOutputMessage`), not a
  `message` item. `ResponseOutputTextParam` has no `prompt_cache_breakpoint`
  field at all, so a breakpoint on an assistant text block is dropped here
  rather than mismatched — this is an inherent Responses API limitation
  (breakpoints only exist on input-side `input_text`/`input_image`), not a
  gateway gap, and the round trip back does not recover it.
- The reverse converters (`openai_responses_to_anthropic.go`,
  `openai_responses_to_chat.go`) had no case for `OfOutputMessage` at all —
  it fell through their item-type switch silently, **dropping the whole
  assistant turn**. Added `convertResponsesOutputMessageToAnthropicBeta` /
  `outputMessageText`.
- The Chat→Responses path (`convertChatAssistantMessageToResponses`) had the
  same latent bug, behind its own cache-breakpoint condition, and emitted a
  *different* valid shape besides (`EasyInputMessage` plain-string
  shorthand) even once fixed. It now also emits `output_message` for
  assistant text, matching the Anthropic→Responses shape — two different but
  both-valid wire forms for "assistant history item" is exactly the kind of
  inconsistency that let this bug class happen twice, independently, and
  would bite a future shared helper that assumed one of them.

**Coverage gap this exposed:** the `cache_prefix` harness section (§10.4)
validates that two consecutive requests serialize to the same *shape* — it
does not validate that the shape is *legal* per the role it's attached to. A
regression that mis-tags assistant content as `input_text` again would
serialize identically turn over turn (stable, just wrong) and `cache_prefix`
would report success while the real Responses API 400s on it. Today only the
unit tests in `internal/protocol/request` (`cache_control_family_test.go`,
`openai_chat_to_openai_responses_test.go`) catch this class of bug; extending
the harness to assert role-appropriate content types is unstarted.

