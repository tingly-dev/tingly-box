# Codex as a Client

What tingly-box has to guarantee when OpenAI Codex (CLI or desktop) is the
client, plus — in §5 — what the ChatGPT Codex backend expects when it is the
*upstream*. Codex setup and auth are covered by `codex-config.md` and
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

### Opaque reasoning history and remote compaction

Codex's `ResponseItem::Reasoning` permits an absent `id`, whereas the Go SDK
models the field as a required string and marshals absence as `"id":""`.
At the Codex provider boundary, an encrypted reasoning item must survive ID
sanitization: omit an invalid ID with `param.Omit` in the typed pass, and delete
only the ID in the JSON pass. Preserve the ciphertext, summary, existing SDK extra
fields, and item ordering. Reasoning without ciphertext retains the existing malformed-ID
handling. Do not invent IDs or decode, summarize, or remove opaque history.

The ID-preservation fix is a request-fidelity fix, not a demonstrated cause of
the protection error. The audited Codex request formatter clones history without
unconditionally clearing reasoning IDs, so a normal request containing valid
reasoning IDs does not exercise the bug.

### Native Responses compaction contract

The first compaction compatibility increment covers Codex OAuth and native
Responses upstreams on ordinary `POST /responses`:

- Forward `context_management`, including its compaction threshold, and
  Codex's `compaction_trigger` without interpreting them as summary requests.
- Keep native `compaction` items and their opaque `encrypted_content` in
  request history and in streaming/non-streaming output.
- Forward progress, output-item and completion events in upstream order,
  retaining extra event/output fields. Codex JSON responses are assembled from
  the upstream SSE completion as usual.
- Replay the returned native state to the same compatible backend on the next
  request. Upstream failures remain failures; no replacement summary is created.

These paths already use the SDK's native types and raw response/event JSON.
Regression tests cover the client, request parser and authenticated gateway.
This increment adds no summary codec, cross-protocol adapter, standalone compact
route or V2 configuration default. Live provider acceptance remains unverified.

### Compaction branch audit: Codex CLI 0.163.0

Audited upstream tag `rust-v0.163.0`, commit
`83086188ae0614bd882818ff1aa23f3194962012` (published 2026-10-15).
Desktop applications can bundle a different core version.

- `model-provider/src/capabilities.rs` defaults native OpenAI and Azure Responses
  providers to remote V2, and other configured providers to local compaction.
  `is_openai()` compares the provider name exactly with `OpenAI`; requiring
  OpenAI auth does not select the native provider's compaction capabilities.
- TB's generated provider is named `OpenAI using Tingly Box`. This held rollout
  draft adds an explicit V2 capability by default, preserving explicit overrides.
  It must wait for mixed-provider compatibility before deployment.
- `core/src/session/turn.rs::run_auto_compact` chooses the branch. Both branches
  use `POST /responses` in this release. Local compaction asks the model for a
  textual summary; remote V2 appends `compaction_trigger` and expects a completed
  response containing exactly one `compaction` output item.
- `core/src/compact.rs` keeps the history but builds a Prompt with default empty
  tools. V2 instead uses the current tool router's declarations, unless Lite
  declarations already live in `additional_tools` history. Hosted search history
  without a matching declaration is therefore a concrete compatibility boundary
  to test. This source comparison does not prove that this shape causes OpenAI's
  protection failure; the local reconstruction test injects the failure.
- Responses Lite still uses `/responses`; `client.rs` switches its header and
  tool representation. It is not a separate `/responses/lite` URL in this release.
- V2 foreground stream retries are capped at two per transport. Switching from
  WebSocket to HTTPS is a transport fallback, not a switch to local compaction.
  Local compaction has its own retry loop (default provider stream budget: five).
  Failed pre-sampling compaction stops the current turn after preserving input.
- The old `features.remote_compaction_v2` switch is removed and ignored. The
  supported custom-provider override is
  `[model_providers.tingly-box.capabilities] remote_compaction = "v2"`.
  Use it only when every routed backend supports V2. This draft preserves explicit
  capability overrides when Quick Config is reapplied, including `unsupported`
  for local summarization. Generic summary/translation support is deferred, so
  native-only acceptance does not justify enabling V2 for mixed providers.

The standalone `POST /responses/compact` JSON endpoint remains a supported public
OpenAI contract and is separate from either audited CLI branch. The gateway
exposes it natively; see `compaction.md`. Its availability does not establish
the cause of an upstream protection error during a different compaction mode.

The exact prefix `received error while streaming:` is emitted by TB's vendored
Go SDK when it detects an SSE payload's `error` object. It can be reproduced
with an upstream HTTP **200** and that in-stream payload; it does not establish
an upstream HTTP 502. TB may then return a JSON failure before downstream SSE
starts, or an SSE error after streaming has started. The client-visible JSON
and the upstream HTTP status are different observations.

Upstream sources (paths relative to `codex-rs/`):
[capability selection](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/model-provider/src/capabilities.rs#L30),
[local request builder](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/core/src/compact.rs#L288),
[V2 request builder](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/core/src/compact_remote_v2_attempt.rs#L167),
[Responses endpoint](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/codex-api/src/endpoint/responses.rs#L133).

`response protection is unavailable` is also reported with direct ChatGPT
authentication (openai/codex #52016). An upstream message identifies where an
error was produced, not whether the gateway contributed to its trigger. The
encrypted-history regression tests prove local request fidelity only, not that
this upstream failure is repaired. Keep HTTP errors and in-stream errors visible;
do not replace protection failures with successful or fabricated compaction.

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

## 5. Prompt-cache affinity on the Codex upstream

The ChatGPT Codex backend caches on the request prefix like any Responses
endpoint, but it does not decide *where* a request lands from the prefix alone.
Two affinity hints exist, and a request that carries neither can miss a cache it
would otherwise have hit — the same conversation, byte-identical prefix,
oscillating between a ~99% hit and a full re-bill depending on which shard it
reached.

What the Codex CLI does, read from `codex-rs/core/src/client.rs` (openai/codex,
Sep 2026):

| Hint | Value the CLI sends | Notes |
| --- | --- | --- |
| `prompt_cache_key` (body) | `prompt_cache_key_override`, else `"{session_source}:{parent_thread_id}"` for internal sessions, else the **session id** | Conversation-scoped and stable across turns, never per-turn |
| `session-id` (header) | the Responses session id, canonical uuid | The CLI's own comment: *"ChatGPT derives cache affinity from the Responses session-id header."* Hyphenated; the legacy underscore `session_id` / `conversation_id` headers were dropped outside the compact path (openai/codex #11732) |
| `thread-id`, `x-client-request-id`, `originator`, `x-codex-window-id` | thread id / attribution | Attribution, not cache affinity |

So: **the body field is the platform-API hint; the `session-id` header is the
one ChatGPT itself keys affinity on.** The CLI sends both, which is why a
native Codex session holds a high hit rate across a long thread.

Traffic converted from another client used to send neither. Anthropic has no
equivalent of `prompt_cache_key` — its cache is addressed purely by prefix — so
nothing survived the conversion, and a Claude Code session against a Codex
provider ran with no affinity at all.

The gateway now derives one identifier and uses it for both:

- `request.openAIPromptCacheKey` reads the session id out of Claude Code's
  `metadata.user_id` (see `metaid.ParseMetadataUserID`) and sets it as
  `prompt_cache_key` on the converted Responses request. Only the session id is
  forwarded — never the device id or account uuid. A `user_id` in some other
  shape is hashed, so an unrecognized format still yields a stable key without
  leaking its contents upstream.
- `applyCodexSessionAffinityHeader` mirrors that key into the `session-id`
  header at the Codex boundary, so no extra plumbing is needed to carry the
  session identity down to the round tripper. A client that sent its own
  `session-id` keeps it. A key that is not a canonical uuid (the hashed
  fallback) stays in the body only: the header expects the uuid form and a
  rejected request would cost more than a missed hint.

`thread-id` and the attribution headers are deliberately not synthesized — they
identify a Codex client we are not, and they do not affect caching.

The `cache_prefix` harness section (`harness-matrix.md` §10.4) drives each
client shape through a provider wired with a Codex OAuth identity, so these
rules — breakpoints stripped, system lifted into `instructions`, `session-id`
present and agreeing with `prompt_cache_key` — are asserted against the request
that actually leaves the Codex RoundTripper, not against a hand-built body.
