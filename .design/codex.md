# Codex as a Client

What tingly-box has to guarantee when OpenAI Codex (CLI or desktop) is the
client. Codex setup and auth are covered by `codex-config.md` and
`codex-auth.md`; this note is about the wire behaviour once requests flow.
The Responses-side contract itself lives in `protocol-responses.md`.

## 1. What Codex sends

- Codex speaks the **Responses API** only (`wire_api = "responses"`), with
  `store: false`: every request carries the full thread history as an
  `input` item list. There is no `previous_response_id`.
- History items are exactly what earlier responses returned (`message`,
  `reasoning`, `function_call`, `custom_tool_call`, …) plus what Codex
  adds locally (`function_call_output`, `custom_tool_call_output`,
  developer/user messages). Codex persists the `id` of every output item we
  return and replays it **verbatim** on later turns.
- The Responses API correlates calls and outputs only by `call_id`. It has
  no adjacency rule, does not require every call to be answered, and
  OpenAI accepts a standalone `function_call_output`. Codex relies on all
  three:
  - interrupting a turn while a tool runs leaves a `function_call` (or the
    tail of a parallel batch) with no output, followed by a new user
    message;
  - automations, heartbeat ticks and cross-thread delegation inject a
    `function_call_output` with no `call_id` and no paired call
    (openai/codex #42088, #45318, #45450, #45914).
- Codex compacts history, so item positions are not stable across turns.

## 2. Downgrading to Chat Completions or Anthropic

Chat Completions (DeepSeek, OpenAI chat) and Anthropic Messages enforce a
stricter shape than the source: adjacency plus a one-to-one pairing of
calls and results. The gateway repairs the history once, at the Responses
item level, before either conversion (`RepairResponsesToolCalls`, see
`protocol-responses.md` §1). The repair is a deterministic function of the
history, so repeated turns produce identical prefixes and provider prompt
caches are not disturbed.

`custom_tool_call` / `custom_tool_call_output` (Codex's freeform
`apply_patch`) are currently dropped by both converters. Tracked
separately.

## 3. Ids Codex replays

Two facts about the ids on the objects the gateway synthesizes:

1. Codex replays our ids verbatim, possibly into a **native** Responses
   upstream after a mid-thread provider switch, where OpenAI validates
   them (type prefix, charset, length, uniqueness). Codex's own replay
   validation only checks that *some* prefix exists (openai/codex #38855),
   so a wrong prefix reaches the upstream.
2. Ids are never model-visible and never resolved by anyone under
   `store: false`; they only have to be well-formed and unique. What the
   model does see is the `call_id`, passed through from the upstream.

The minting rules and rejected alternatives are in `protocol-responses.md`
§2. Two invariants follow: an id is minted once per object and is identical
in `output_item.added`, `output_item.done` and the final output; ids on
replayed input items are never rewritten.

## 4. DeepSeek specifics

DeepSeek's chat endpoint validates the tool-call shape strictly and is the
provider that surfaced the repair; its Responses endpoint is lenient on ids
and only insists on `call_id`. Probe tables are in `deepseek.md`.
