# Multimodal content — protocol catalog & conversion contract

Issue [#1606](https://github.com/tingly-dev/tingly-box/issues/1606) exposed a
class of bugs where image content survived some conversion paths but was
corrupted or silently dropped on others (most visibly: `image_url` parts in
`role:"tool"` messages, which upstreams rejected with 400 on *every*
subsequent request carrying that history). This document is the catalog the
fix was driven from: every place multimodal content can legally appear in
each protocol, what it converts to in each target, and the harness coverage
that keeps it working.

## 1. Where multimodal content lives, per protocol

### OpenAI Chat Completions (`TypeOpenAIChat`)

| Location | Part types | Notes |
| --- | --- | --- |
| `messages[role=user].content[]` | `text`, `image_url`, `input_audio`, `file` | classic multimodal user turn |
| `messages[role=tool].content[]` | `text`, `image_url` (ecosystem) | how agent frameworks return tool screenshots. The upstream SDK types this as text-only; our `openai-go` fork widens `ChatCompletionToolMessageParamContentUnion.OfArrayOfContentParts` to the full content-part union — see §3 |
| `messages[role=system/developer].content[]` | `text` only | never multimodal |
| `messages[role=assistant].content[]` | `text`, `refusal` | never image-bearing on request side |

### OpenAI Responses (`TypeOpenAIResponses`)

| Location | Part types | Notes |
| --- | --- | --- |
| `input[type=message].content[]` | `input_text`, `input_image`, `input_file` | user/system message parts |
| `input[type=function_call_output].output[]` | `input_text`, `input_image`, `input_file` | `output` is a string OR an item list; images ride the item list |

### Anthropic Messages v1 / Beta (`TypeAnthropicV1` / `TypeAnthropicBeta`)

| Location | Block types | Notes |
| --- | --- | --- |
| `messages[].content[]` (user) | `text`, `image`, `document`, `search_result` | image source: `base64` or `url` |
| `tool_result.content[]` | `text`, `image`, `document`, `search_result`, `tool_reference` | how Claude-family tools return screenshots |
| `system[]` | `text` only | |

### Google Gemini (`TypeGoogle`)

| Location | Part types | Notes |
| --- | --- | --- |
| `contents[].parts[]` | `text`, `inline_data`, `file_data` | user images as inline blobs |
| `contents[].parts[].function_response.parts[]` | `inline_data` blobs | tool-returned media |

## 2. Conversion contract

The gateway's rule (issue #1606): **an image must survive every supported
`(source → target)` request conversion in both the user channel and the tool
channel** — either natively or, when a scenario routes to a text-only model,
via the vision-proxy rewrite (`.design/vision-proxy.md`), never by silent
corruption.

Canonical mappings:

| From | To OpenAI Chat | To Responses | To Anthropic | To Google |
| --- | --- | --- | --- | --- |
| user image | `image_url` part (data: URL for base64) | `input_image` | `image` block | `inline_data` |
| tool image | `image_url` part in `role:"tool"` content | `input_image` in `function_call_output.output[]` | `image` in `tool_result.content[]` | `inline_data` in `function_response.parts[]` |

Shared helpers:

- `request.ParseImageURLToAnthropicSource` — splits an OpenAI `image_url.url`
  into (mediaType, data) for data: URLs, or a remote URL.
- `request.anthropicImageToOpenAIURL` / `imageBlockToOpenAIURL` /
  `betaImageBlockToOpenAIURL` — Anthropic image source → OpenAI URL string.
- `request.openAIImageURLToAnthropicBetaBlock` — the reverse.
- `request.openAIToolMessageFromAnthropicToolResult` — tool_result (view) →
  OpenAI tool message, preserving image entries.

String-vs-array rule for tool results: text-only content keeps collapsing to
the compact plain-string form (`content: "…"` / `output: "…"`); the array
form is used exactly when the content carries images or cache breakpoints.
This keeps wire output byte-stable for the overwhelmingly common text case.

## 3. The openai-go fork extension

Upstream `openai-go` types tool message content as
`[]ChatCompletionContentPartTextParam` (text-only, matching OpenAI's
published spec). Real agent traffic — and OpenAI-compatible upstreams such as
Qwen/vLLM — use `image_url` parts inside tool messages. With the text-only
union, the ingress parse itself corrupted the part
(`{"type":"image_url","text":""}`), which is precisely the 400 in #1606.

The fork (`libs/openai-go`) therefore widens
`ChatCompletionToolMessageParamContentUnion.OfArrayOfContentParts` to
`[]ChatCompletionContentPartUnionParam` (the same union user messages use)
and teaches `openai.ToolMessage` to accept the union slice. Wire output for
text-only content is unchanged. When syncing the fork with upstream, keep
this extension — `chatcompletion_toolmessage_patch_test.go` (in the fork) locks it.

## 4. Harness coverage (vmodel-driven)

The canonical image fixture — a 256×256 red PNG plus the "what color?" prompt and
the tool-channel turn script — lives in `internal/vision/visionproxy` (the
`thinking`-package pattern) so every consumer sends the exact same shapes:
the probe subsystem's `vision` axis, the content-shape harness cases below,
and future vision health checks.

Per the TDD strategy on #1606, coverage lives at three levels:

- **Probe (user-facing runtime)**: the Probe dialog's `vision` axis
  (`none`/`user`/`tool` — see `.design/probe.md` § Test axes) sends the
  fixture through the TB loopback and the real transform pipeline, with cURL
  export via the shared param builders. `internal/probe/vision_test.go` pins
  the builder shapes.

- **Unit** (`internal/protocol/request/image_conversions_test.go` — user
  channel; `image_tool_conversions_test.go` — tool channel;
  `internal/protocol/multimodal_roundtrip_test.go` — ingress round trip;
  `internal/protocol/transform/consistency_multimodal_test.go` — orphan-tool
  alignment).
- **End-to-end** (`internal/protocoltest/content_shapes.go`, image cases):
  bespoke requests through the real gateway, asserting on the request the
  vmodel-backed VirtualServer actually captured upstream
  (`VirtualServer.LastRequest`), for:
  `chat→chat` (tool + user), `chat→responses` (tool), `chat→anthropic`
  (tool), `anthropic→chat` (user + tool_result), `anthropic→responses`
  (tool_result). Runs under both `go test ./internal/protocoltest` and
  `harness matrix --mode=content_shapes`.

## 5. Known gaps (out of scope of the #1606 fix, catalogued for follow-up)

- **Audio / file / document parts**: `input_audio` and `file` Chat parts,
  Responses `input_file`, and Anthropic `document` / `search_result` blocks
  are passed through same-protocol but not yet mapped cross-protocol.
- **Google targets**: user images convert (`inline_data`); tool images now
  forward as `function_response` inline blobs on the OpenAI→Google path, but
  the Anthropic→Google path still extracts text only from `tool_result`.
- **Remote (http) image URLs → Google** are stubbed as `[Image: <url>]` text
  because Gemini needs fetched bytes or `file_data`.
- **Vision health probes**: the E2E probe now has the `vision` axis (§4), but
  the periodic health monitor does not yet send an image-bearing check;
  #1606's comment showed a health probe hitting the corrupted-part 400. When
  added, it should reuse the probe param builders with
  `probeParams{Vision: ...}` for both channels.
