# processor

Op-level processors for `smart_routing`. A processor is a side-effect handler
bound to a `(SmartOpPosition, SmartOpOperation)` tuple. When the smart-routing
stage matches a rule whose ops carry a registered processor, the stage runs
the processor(s) and returns `(nil, false)` — the **LoadBalancer** (the
global fallback) then picks an upstream from the parent rule's top-level
`Services` with the **mutated** request. The bypass is strictly one-shot:
the mutated request is not re-evaluated against smart-routing rules,
keeping post-processor behavior predictable.

The matched rule's `Services` are treated as the **processor's upstream
candidate pool**, not the routing destination.

## Wiring

```
boot (server.go)
  └─► processor.RegisterAll(pool, resolver, logger)
        ├─► VisionProxyProcessor{
        │     Client:   NewPoolVisionClient(pool, resolver, logger),
        │     Resolver: resolver,
        │   }
        └─► smartrouting.RegisterProcessor(
              PositionProxyVision,
              OpProxyVisionEnabled,
              visionProc)
                  │
                  ▼
            registry["proxy_vision|enabled"] = visionProc

per request (internal/server/routing/stage_smart_routing.go)

  router.Evaluate(reqCtx)
        │
        ├── no match  ──► (nil, false) → next stage
        │
        └── match rule R
              │
              ├── R already in ctx.BypassedSmartRules
              │     ──► (nil, false) → LoadBalancer
              │
              ├── R has no processors
              │     ──► terminal selection
              │         (intersect, filter active, return result)
              │
              └── R has processors
                    run each Process(pctx) in op order
                    (each processor mutates ctx.Request in place)
                    ctx.BypassedSmartRules[R] = struct{}{}
                    ──► (nil, false) → LoadBalancer
                        (with mutated request)
```

## VisionProxyProcessor

Replaces every image content block in the request with a text block.
Enables **text-only downstream models to accept image-bearing requests**.

Enabling proxy_vision implies the fallback (downstream) model does not
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

```
pctx.Request : *anthropic.BetaMessageNewParams (or v1 / OpenAI)

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
       │ pickUsableService(pctx.Services)
       │   skip nil / inactive / unresolvable-provider svcs
       │
       │ for each message i < lastIdx:
       │   replace OfImage blocks with
       │     { OfText: "[image: (omitted from history)]" }
       │   (no Describe call — no upstream cost for historical images)
       │
       │ for each OfImage in messages[lastIdx]:
       │   extractImageSource → (mediaType, b64Data, remoteURL)
       │     - Beta:   img.Source.OfBase64 | img.Source.OfURL
       │     - V1:     img.Source.OfBase64 | img.Source.OfURL
       │     - OpenAI: ParseImageURLToAnthropicSource(image_url.url)
       │
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

  smart_routing stage returns (nil, false);
  LoadBalancer picks main service;
  forwarder serializes the now-text-only typed request downstream.
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
|--------------------------------------------|------------------------------------------------|----------------------------------------|
| `*anthropic.BetaMessageNewParams`          | `BetaImageBlockParam.Source` (Base64 \| URL)   | last message described; older stripped |
| `*anthropic.MessageNewParams`              | `ImageBlockParam.Source` (Base64 \| URL)       | last message described; older stripped |
| `*openai.ChatCompletionNewParams`          | `user.content[].OfImageURL.ImageURL.URL`       | last message described; older stripped |

Unknown request shapes are left alone (no-op).

## Adding a new processor

1. Implement `smartrouting.OpProcessor`:
   ```go
   type MyProc struct { /* deps */ }
   func (p *MyProc) Process(pctx *smartrouting.ProcessorContext) error { … }
   ```
2. Register it in `processor.RegisterAll`:
   ```go
   smartrouting.RegisterProcessor(
       smartrouting.PositionXxx,
       smartrouting.OpXxx,
       &MyProc{…})
   ```
3. Add a `SmartOp` entry in `internal/smart_routing/op.go` and handle the op
   in the appropriate `evaluateXxxOp` function so rules can declare it.

The matched rule's `Services` are passed in `pctx.Services` for processors
that need an upstream pool — `pickUsableService`-style selection is the
processor's responsibility.

## Out of scope (today)

- Concurrent image description (sequential, one call per image).
- Caching describe results across requests.
