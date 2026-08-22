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
  └─► visionproxy.NewServiceFromPool(pool, resolver, logger)
        └─► Service{ Processor: &VisionProxyProcessor{
              Client:   NewPoolVisionClient(pool, resolver, logger),
              Resolver: resolver,
            }}

per request (internal/server/vision_proxy.go → applyVisionProxy)
  Service.Apply(ctx, cfg, scenarioType, rule, typedRequest)
        │
        ├─ Resolve(cfg, scenarioType, rule) → *loadbalance.Service
        │    rule.Flags.VisionProxyService wins over
        │    cfg.Scenarios[...].Extensions["vision_proxy_service"]
        │    nil  ⇒  neither scope configured a service → no-op
        │
        └─ Processor.Process(ctx, typedRequest, []*loadbalance.Service{svc})
             mutates typedRequest in place (see below)
```

`Service.Apply` is called directly from the request handlers
(`openai_chat.go`, `openai_responses.go`, `anthropic_message.go`) before
service selection — it is not a smart-routing op. An earlier version
registered `VisionProxyProcessor` into `internal/smart_routing`'s processor
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
   message of `req.Messages` is sent to the vision upstream; the
   description is spliced in as a text block. This is the actual cost
   center.
2. **Strip historical images.** Every image in messages BEFORE the last
   one is replaced with a fixed text marker (`[image: (omitted from
   history)]`) — no vision call is made. The image is gone from the
   request so the text-only downstream still accepts it.

### Process pipeline

Processing is two-phase: a **collect** walk that strips historical images
in place and gathers the latest message's images as `imageRef`s (source +
splice-back callback), then a **describe** fan-out that resolves each ref
via the vision upstream — concurrently, with `describeConcurrency` (4)
bounding both live goroutines and in-flight upstream calls (the semaphore
is acquired before each goroutine spawns). Each ref splices into its own
distinct block slot, so the concurrent writes need no locking. A panic in
the describe path is recovered per-image and collapses to the fail-strip
marker — the goroutines run outside the HTTP handler's recovery
middleware, so containment lives here.

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
       │ Phase 1 — collect<Protocol>(req):
       │   for each message i < lastIdx:
       │     replace OfImage blocks with
       │       { OfText: "[image: (omitted from history)]" }
       │     (no Describe call — no upstream cost for historical images)
       │   for each OfImage in messages[lastIdx]:
       │     extractImageSource → (mediaType, b64Data, remoteURL)
       │       - Beta:   img.Source.OfBase64 | img.Source.OfURL
       │       - V1:     img.Source.OfBase64 | img.Source.OfURL
       │       - OpenAI: ParseImageURLToAnthropicSource(image_url.url)
       │     collect imageRef{source, splice}
       │
       │ pickUsableService(services)          (skipped when no refs)
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
fail-strip does not apply; they always receive the omitted marker.

```
                          ┌──────────────────────────────────────────────┐
                          │ describe outcome                  → replacement│
                          ├──────────────────────────────────┬───────────┤
  no usable service       │ usable == nil                    │  unavail   │
  vision client nil       │ p.Client == nil                  │  unavail   │
  Describe() error        │ err != nil                       │  unavail   │
  empty response          │ strings.TrimSpace(desc) == ""    │  unavail   │
  Describe() panics       │ recovered in safeDescribe        │  unavail   │
  success                 │ desc non-empty                   │  [image: …]│
                          ├──────────────────────────────────┴───────────┤
  historical image        │ messages[i] where i < lastIdx    │  historic │
                          │ (no Describe call)               │            │
                          └──────────────────────────────────┴───────────┘
  unavail  = "[image: (description unavailable)]"
  historic = "[image: (omitted from history)]"
```

### Protocol coverage

| Request shape                              | Image block source                             | Notes                                  |
|--------------------------------------------|--------------------------------------------------|----------------------------------------|
| `*anthropic.BetaMessageNewParams`          | `BetaImageBlockParam.Source` (Base64 \| URL)   | last message described; older stripped |
| `*anthropic.MessageNewParams`              | `ImageBlockParam.Source` (Base64 \| URL)       | last message described; older stripped |
| `*openai.ChatCompletionNewParams`          | `user.content[].OfImageURL.ImageURL.URL`       | last message described; older stripped |
| `*responses.ResponseNewParams`             | `input[].content[].OfInputImage`               | last item described; older stripped    |

Images nested inside `tool_result` content blocks are also walked (Beta and
v1 shapes) — tool-returning agents (screenshot / read-image / MCP tools)
deliver images this way. Unknown request shapes are left alone (no-op).

## Testing

- `visionproxytest/` — shared test doubles (`StubVisionClient`,
  `StubResolver`, fixture builders) reused by this package's own tests and
  by `internal/server` tests that exercise `Service.Apply` through the real
  handler call order (see `internal/server/openai_responses_vision_test.go`).
- `vision_proxy_e2e_test.go` (build tag `e2e`) drives a real deployment;
  requires `TINGLY_API_KEY`, see the file header for details.

## Out of scope (today)

- Caching describe results across requests (each image is described once
  per request, even if the identical image appears in multiple requests).
- Deduplicating identical images within one request (each occurrence gets
  its own describe call).
