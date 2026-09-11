# visionproxy

The vision proxy plugin: when the downstream model is text-only, describe
image content via a vision-capable upstream and splice the description in as
text, so image-bearing requests still work.

See `.design/vision-proxy.md` for the product-level design (scopes,
configuration matrix, data model). This README covers the implementation —
how `Service` resolves the upstream and how `VisionProxyProcessor` rewrites
the request.

## Wiring

```
boot (internal/server/server.go)
  └─► visionproxy.NewServiceFromPool(pool, resolver)
        └─► Service{ Processor: &VisionProxyProcessor{
              Client:   NewPoolVisionClient(pool, resolver),
              Resolver: resolver,
            }}

per request (internal/protocolserver/protocol_handler.go → applyVisionProxy)
  Service.Apply(ctx, cfg, scenarioType, rule, typedRequest, sessionID)
        │
        ├─ Resolve(cfg, scenarioType, rule) → *loadbalance.Service
        │    rule.Flags.VisionProxyService wins over
        │    cfg.Scenarios[...].Extensions["vision_proxy_service"]
        │    nil  ⇒  neither scope configured a service → no-op
        │
        └─ Processor.Process(ctx, typedRequest, []*loadbalance.Service{svc}, sessionID)
             mutates typedRequest in place (see below)
```

`Service.Apply` is called directly from the request handlers
(`openai_chat.go`, `openai_responses.go`, `anthropic_message.go`) before
service selection — it is not a smart-routing op. An earlier version
registered `VisionProxyProcessor` into `internal/routing/smartrouting`'s processor
registry so a matching rule could bypass routing with `{Position:
proxy_vision, Operation: enabled}`; that path was removed in favor of the
rule/scenario flags above, which are simpler to configure and don't require
a second rule.

## VisionProxyProcessor

Replaces every image content block in the request with a text block.
Enabling vision proxy implies the fallback (downstream) model does not
support images, so EVERY image block must be removed from the serialized
request. But describing every image in the conversation history through
the vision upstream would be wasteful — older images are rarely the
subject of the current question. The processor therefore has two distinct
responsibilities:

1. **Describe the latest message's images.** Each image in the LAST
   message of `req.Messages` is sent to the vision upstream (unless the
   describe cache already has an answer for it — see below); the
   description is spliced in as a text block. This is the actual cost
   center.
2. **Strip historical images.** Every image in messages BEFORE the last
   one is replaced with a fixed text marker (`[image: (omitted from
   history)]`) — no vision call is made — **unless** the describe cache
   already has a real description for it (e.g. it was the latest message
   last turn), in which case the real description is used instead.

### Describe cache

`describe_cache.go` is a two-tier cache from
`(session, provider, model, image-content-hash)` to the already-formatted
replacement text:

- a fixed-capacity in-memory LRU (2000 entries) — the hot tier every lookup
  hits first;
- `describe_store.go` — a durable tier in tingly's shared SQLite database
  (`vision_descriptions` table), consulted on a memory miss and written
  through on every put. A store hit is promoted into memory so the same
  historical image never touches the database again on later turns.

Every image occurrence — latest or historical — checks the cache first
(`spliceOrCollect`): a hit splices the cached text immediately, no upstream
call either way; a miss falls through to the two behaviors above. Only
successful describe calls are written back; fail-strip results never are.
Session is part of the key so a description is only ever reused within the
same conversation; the session component is `source:value`, deliberately
without the client-IP backup so a network change mid-conversation keeps
hitting. See `.design/vision-proxy.md` §10 for the full rationale.

The durable tier is what keeps the downstream prompt prefix stable across
a gateway restart: a conversation carrying a dozen screenshots would
otherwise re-describe all of them — with a dozen freshly worded texts — on
its first request after the restart. There is no age limit — a paid-for
description is kept for as long as the table is under its ceiling of
100000 rows, beyond which the least recently used rows go first; the check
runs at boot and then at most hourly from the write path. The
store is wired at server boot from the StoreManager's connection; if that
is unavailable the cache silently degrades to memory-only.

### Process pipeline

Processing is two-phase: a **collect** walk that, for every image, checks
the cache first — a hit splices the cached text immediately regardless of
position; a historical miss strips with the fixed marker; a latest-message
miss is gathered as an `imageRef` (source + splice-back callback) — then a
**describe** fan-out that resolves each gathered ref via the vision
upstream — concurrently, with `describeConcurrency` (4) bounding both live
goroutines and in-flight upstream calls (the semaphore is acquired before
each goroutine spawns). Each ref splices into its own distinct block slot,
so the concurrent writes need no locking. A panic in the describe path is
recovered per-image and collapses to the fail-strip marker — the goroutines
run outside the HTTP handler's recovery middleware, so containment lives
here. A successful describe result is written to the cache before splicing.

```
req : *anthropic.BetaMessageNewParams (or v1 / OpenAI / Responses)

  messages: [
    { role: user,
      content: [ "earlier turn", <OfImage A> ] },           ◄── historical
    { role: assistant, content: [ "previous reply" ] },
    { role: user,
      content: [
        { OfText:  "What's in this picture?" },
        { OfImage: B }                                       ◄── latest target
      ] } ]
       │
       │ Phase 1 — collect<Protocol>(req, session, usable, cache):
       │   for each image block (any message index):
       │     key := newVisionCacheKey(session, usable, mediaType, b64, remoteURL)
       │     if cache.get(key) hits:
       │       splice the cached text in immediately — done, no ref, no call
       │     else if i < lastIdx (historical):
       │       replace OfImage blocks with
       │         { OfText: "[image: (omitted from history)]" }
       │       (no Describe call — no upstream cost for historical images)
       │     else (latest message):
       │       collect imageRef{source, cacheKey: key, splice}
       │   extractImageSource → (mediaType, b64Data, remoteURL)
       │     - Beta:   img.Source.OfBase64 | img.Source.OfURL
       │     - V1:     img.Source.OfBase64 | img.Source.OfURL
       │     - OpenAI: ParseImageURLToAnthropicSource(image_url.url)
       │
       │ pickUsableService(services)          (resolved once, up front)
       │   skip nil / inactive / unresolvable-provider svcs
       │
       │ Phase 2 — describeAll(refs): concurrent, ≤ describeConcurrency
       │   describe(ctx, service, mediaType, b64, url):
       │     visionClient.Describe(...)
       │       poolVisionClient (production adapter)
       │         dispatches by provider.APIStyle and ALWAYS uses streaming
       │         (most providers require it for vision); events are folded
       │         back into a non-streaming message via the shared
       │         internal/protocol/assembler package:
       │           "anthropic" → BetaMessagesNewStreaming →
       │                         assembler.NewAnthropicBetaSDKAssembler →
       │                         read text blocks from *BetaMessage
       │           "openai"    → ChatCompletionsNewStreaming →
       │                         assembler.NewOpenAIStreamAssembler →
       │                         read Choice.Message.Content from *ChatCompletion
       │           other       → error → fail-strip marker
       │
       │   desc = "a red apple on a white plate"   (success)
       │        = ""                                (empty   → fail-strip)
       │        = err                               (error   → fail-strip)
       │
       │   on success: cache.put(key, replacement text)
       │   replace OfImage with OfText("[image: <desc>]" or fail-strip)
       ▼
  messages: [
    { role: user,
      content: [ "earlier turn",
                 { OfText: "[image: (omitted from history)]" } ] },
    { role: assistant, content: [ "previous reply" ] },
    { role: user,
      content: [
        { OfText: "What's in this picture?" },
        { OfText: "[image: a red apple on a white plate]" } ] } ]

  Service.Apply returns; the (now text-only) typed request continues
  through the normal service-selection + forwarding path.
```

### Fail-strip semantics

For images in the LAST message the block is removed **regardless of
outcome** — success, error, or empty response — so the downstream
text-only model never receives unsupported content. Historical images
follow a separate path: they are never sent to the vision upstream, so
fail-strip does not apply; they receive the omitted marker unless the
describe cache already has a real description for them.

```
                          ┌──────────────────────────────────────────────┐
                          │ describe outcome                  → replacement│
                          ├──────────────────────────────────┬───────────┤
  cache hit (any position)│ describeCache.get(key) ok         │ [image: …]│
                          ├──────────────────────────────────┼───────────┤
  no usable service       │ usable == nil                    │  unavail   │
  vision client nil       │ p.Client == nil                  │  unavail   │
  Describe() error        │ err != nil                       │  unavail   │
  empty response          │ strings.TrimSpace(desc) == ""    │  unavail   │
  Describe() panics       │ recovered in safeDescribe        │  unavail   │
  success                 │ desc non-empty (→ cached)         │  [image: …]│
                          ├──────────────────────────────────┴───────────┤
  historical image, miss  │ messages[i] where i < lastIdx    │  historic │
                          │ (lastIdx = latest image-bearing) │            │
                          │ (no Describe call)               │            │
                          └──────────────────────────────────┴───────────┘
  unavail  = "[image error: the vision proxy failed to describe this image, …]"
             (explicit proxy-side error report — see imageUnavailableText)
  historic = "[image: (omitted from history)]"
```

### Protocol coverage

| Request shape                              | Image block source                             | Notes                                  |
|--------------------------------------------|--------------------------------------------------|----------------------------------------|
| `*anthropic.BetaMessageNewParams`          | `BetaImageBlockParam.Source` (Base64 \| URL)   | latest user message described; older stripped |
| `*anthropic.MessageNewParams`              | `ImageBlockParam.Source` (Base64 \| URL)       | latest user message described; older stripped |
| `*openai.ChatCompletionNewParams`          | `user.content[].OfImageURL.ImageURL.URL` (`OfUser` and `OfTool` messages) | latest user/tool message described; older stripped |
| `*responses.ResponseNewParams`             | `input[].content[].OfInputImage`               | latest user item described; older stripped    |

`lastIdx` is the index of the latest message that can carry an image from the
caller — **not** simply `len(messages)-1`. Clients append trailing messages
that are not part of the turn: Claude Code emits a `<system-reminder>` as its
own system-role message after every tool result, so the request answering a
tool call arrives as `user / system / assistant(tool_use) /
user(tool_result+image) / system`. Anchoring on the final message would make
the image of the turn in flight test as history and replace it with the
`omitted from history` marker, so the caller would answer about an image no
model ever saw. See `latestImageAnchor`.

Images nested inside `tool_result` content blocks are also walked (Beta and
v1 shapes) — tool-returning agents (screenshot / read-image / MCP tools)
deliver images this way. Unknown request shapes are left alone (no-op).

## Testing

- `stub.go` — shared test doubles (`StubVisionClient`, `StubResolver`,
  `NewProcessor`, fixture builders) reused by this package's own tests and by
  other packages' tests that exercise `Service.Apply` through the real
  handler call order.
- `describe_cache_test.go` — LRU mechanics in isolation (eviction, update,
  key isolation across service/session, nil/zero-capacity no-op behavior).
- `describe_store_test.go` — the durable tier: round-trip and upsert,
  session/service isolation, survive-a-restart (fresh cache over the same
  database hits with zero vision calls and byte-identical text), memory
  eviction falling back to the store, no-age-limit and size-ceiling
  pruning, `last_used_at` touch throttling, and the IP-independent session
  scope.
- `vision_proxy_test.go` / `vision_proxy_regression_test.go` — the
  processor contract, including the `TestVisionProxy_Cache_*` cases for
  session/model isolation, historical-hit-uses-real-description, and
  failed-describe-not-cached.
- `vision_proxy_e2e_test.go` (build tag `e2e`) drives a real deployment;
  requires `TINGLY_API_KEY`, see the file header for details.

## Out of scope (today)

- Deduplicating identical images within one request (each occurrence still
  gets its own describe call the first time it's seen — the cache only
  helps across separate `Process` calls, not within one).
- Cross-instance cache sharing (the durable tier lives in each gateway's
  own `tingly.db`; two gateway instances do not see each other's
  descriptions — see `.design/vision-proxy.md` §10).
- Coalescing two concurrent requests that carry the same new image. This
  is deliberate, not a gap: both describe, each gets its own independent
  description (keeping the vision model's output diversity), and the later
  write is what later turns of the session settle on.
