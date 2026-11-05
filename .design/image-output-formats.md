# Image output formats: GIF through the gateway, SVG kept inert

> Status: **phases 1, 2 and 4 implemented, phase 0 (live capture) and phase 3 live checks still open** — see §11 for what was built and where it differs from the options below. Tracks TEP #2432. Capability research and sources: [`../.research/codex-imagegen-gif.md`](../.research/codex-imagegen-gif.md).
> Read first: [`image-edit.md`](./image-edit.md) (Codex native images protocol, §3.4 deferred native generation), [`projects-published-files.md`](./projects-published-files.md) (send_file and IM delivery), [`projects/.design/library-tree.md`](../projects/.design/library-tree.md) (Library preview allowlist), [`codex-auth.md`](./codex-auth.md), [`ux-principles.md`](./ux-principles.md).

## 1. Problem and scope

Codex's built-in image tool can now return an animated **GIF** (feature-flagged, openai/codex#59073). It never returns **SVG**. Every image path in Tingly Box so far assumes the result is a PNG: the Images handler saves `.png`, the playground builds `data:image/png`, the Codex provider ignores the requested format. A GIF would be mislabelled, and one request field set (`frame_count`, `frame_duration_ms`) is dropped by the request decoder.

In scope:
- Carry `output_format` (`png|jpeg|webp|gif`) and the GIF options end to end through `/images/generations` and `/images/edits`, including the Codex provider and the Codex CLI as a client.
- Make every consumer decide format from the **bytes**, not from a constant or the request.
- Deliver a generated GIF through Projects (`send_file`, Library) and IM.
- Keep SVG inert: never rendered, never inline-served, never sent as an IM image.

Not in scope: generating SVG, server-side rasterizing SVG, transcoding between formats, video, GIF as an edit *input*, the Responses-to-Anthropic image conversion (listed in §9).

## 2. Surfaces and what happens today

| # | Surface | Entry | Today |
|---|---|---|---|
| S1 | Codex CLI built-in `image_gen` | `POST {base}/images/generations` and `/images/edits`, where `{base}` = `http://host:12580/tingly/codex` (`codex-auth.md`) | **Rejected with 400**: the `codex` scenario has only `TransportOpenAI` (`internal/typ/scenario_registry.go:120`), the image handlers require `TransportImageGen` (`openai_image.go:48`, `openai_image_edit.go:62`). Also needs a rule that matches model `gpt-image-2`, which Quick Config does not create (unverified). |
| S2 | Images API clients: OpenAI SDK, Tingly Box Image page, `imagegen` scenario | `POST /tingly/imagegen/v1/images/{generations,edits}` | Works for PNG. Codex provider: generation = Responses `image_generation` tool, edit = native `codex/images/edits`. |
| S3 | Responses `image_generation` hosted tool | `POST /responses` | Passthrough relies on SDK union types; `response.image_generation_call.*` is dropped when converting to Anthropic. Not a GIF surface (the tool's `output_format` is png/jpeg/webp). |
| S4 | Projects | Claude Code thread -> any tool -> file in cwd -> `send_file` | No format policy anywhere in `PublishFile`; IM kind picked from MIME prefix. |

Findings that shape the design (details and line numbers in the research notes):
1. `output_format:"gif"` survives decoding into `openai.ImageGenerateParams` (no enum validation) but `frame_count` / `frame_duration_ms` are **dropped** (verified in a scratch test).
2. `codex_client.go:431-441` builds the Responses tool's `output_format` from `req.ResponseFormat` (`b64_json`/`url`), not from `OutputFormat`; the real value is ignored.
3. `persistImageGeneration` hardcodes `.png` (`openai_image.go:281-283`).
4. The Responses assembler appends `partial_image` chunks although each is a complete image (`openai_responses_assembler.go:185`); the Codex image path already handles this correctly (`codex_client.go` keeps only the latest partial).
5. `imchannel.SendFile` maps any `image/*` MIME to kind `image` (`imchannel.go:52`), so `image/svg+xml` becomes an image; Telegram sends kind `image` with `SendPhoto` (`telegram.go:556`) regardless of extension.

## 3. Design decisions

### 3.1 The bytes are the truth (one sniffing helper)

New small package `internal/vision/imagefmt` (no dependencies):

```go
type Format string // png, jpeg, webp, gif
func Sniff(b []byte) (Format, bool)  // PNG sig, FF D8 FF, GIF87a/GIF89a, RIFF....WEBP
func (f Format) MIME() string; func (f Format) Ext() string
func SniffBase64(s string) (Format, bool) // decodes only the first few bytes
```

Rules:
- Response `output_format` is set from the sniffed bytes when the upstream left it empty or disagreed. A request for `jpeg` that returns PNG is reported as `png`.
- Persisted file extension comes from the sniffed format (`.gif` for GIF). Unknown bytes are not persisted (§4).
- No decoding of image data in the gateway. We read the first bytes only, so there is no decompression-bomb surface in the gateway.

### 3.2 Request validation and per-vendor capability

Add to `internal/vision/imagegen` (next to `DetectVendor`) a static capability table, until model metadata (#1774) exists:

| Vendor | Accepts `output_format` | GIF |
|---|---|---|
| Codex (native `codex/images/*`) | png (default), gif | **yes**, if the account has the feature (upstream decides) |
| Codex (Responses tool, today's generation path) | png, jpeg, webp | no |
| OpenAI-compatible | passthrough (png/jpeg/webp; gif unverified) | passthrough, unverified |
| DashScope, MiniMax, xAI, Qianfan | ignored by adapters | no |

Handler behaviour:
- `output_format` outside `{png,jpeg,webp,gif}` -> 400 `invalid_request_error` naming the allowed values.
- `gif` against a vendor whose row says no -> 400 `output_format gif is not supported by provider <name>`. This is strict because a silent still image would lose the user's intent, and there is no existing client relying on it.
- `png|jpeg|webp` against vendors that ignore the field stays lenient (today's behaviour); the response simply reports the sniffed truth.
- `frame_count` outside 2..16 or `frame_duration_ms` outside 20..2000, or either set without `gif` -> 400. Ranges are Codex's (`GifOptions`); they live in one constants block so a change upstream is a one-line edit.
- Failover (`failover_dispatch`) must skip providers that fail the capability check for the request instead of failing the whole request (§5.1).

### 3.3 Carrying the GIF fields

The request decoder drops unknown fields, so extras must be parsed explicitly. Options:

| Option | How | Pros | Cons |
|---|---|---|---|
| A. Side struct | Handler decodes the already-read body bytes a second time into `ImageGenExtras{FrameCount, FrameDurationMS *int}`, validates, passes it with the params to the forwarder | explicit, typed, testable, no SDK coupling | one more parameter through `ForwardOpenAIImageGeneration` and the client interface |
| B. SDK extra fields | `req.SetExtraFields(...)` so the SDK re-marshals them | no signature change; reaches OpenAI-compatible upstreams unchanged | invisible coupling; Codex client must read them back out of `ExtraFields()`; easy to lose in a copy |
| C. Raw body passthrough for Codex | forward the original JSON to `codex/images/generations` | zero translation | skips validation, model rewrite, `n` fan-out, quality mapping |

**Recommend A, plus B only inside the OpenAI-compatible client** so a public-API GIF (if it exists) reaches upstream. Codex client reads the typed extras. Same extras struct is parsed by the edit handler (multipart form fields and JSON mirror). *As built:* the parsed options travel in the request context instead of a new parameter (see §11), which keeps the `OpenAIClientInterface` signatures unchanged.

### 3.4 Codex provider: which upstream path

| Option | Description | Pros | Cons |
|---|---|---|---|
| P1 | Keep Responses tool for generation; reject `gif` | no new upstream protocol; nothing to verify | no GIF, defeats the TEP |
| P2 | Switch all Codex generation to native `codex/images/generations` (what Codex itself does) | one protocol for generate and edit, matches the CLI | `image-edit.md` §3.4 deferred this on purpose; code comment says the backend "does not support" it (may be stale); changes behaviour for every existing user; needs live validation on subscriptions |
| P3 | **Hybrid:** Responses tool by default; native `codex/images/generations` only when `output_format=gif` | smallest blast radius; existing PNG users untouched; native-generation path is exercised only by a feature that cannot work otherwise | two generation paths to maintain for a while; revisit unification after P3 is proven |

**Recommend P3.** Native request body mirrors `codex-api/src/images.rs`: `prompt`, `model`, `output_format:"gif"`, `frame_count`, `frame_duration_ms`, optional `background`/`quality`/`size`, header `x-codex-image-turn-id`. The edit path (`codex_images.go`) is already native and gets the same three GIF fields added to `codexImageEditRequest`; GIF edits accept a PNG spritesheet as input (the GIF itself is not a valid input, per Codex's own guidance), and a GIF *input* is rejected with a clear 400 rather than forwarded. The round tripper already treats `/backend-api/codex/images/` as plain JSON (`codex_round_tripper.go:433`). Upstream errors such as "feature not enabled for this account" pass through verbatim.

`n > 1`: reuse `fanOutCodexImages` (one GIF per call). GIF calls are slow and costly, so cap `n` for GIF at 4 (constant) and return 400 above it.

### 3.5 Making the Codex CLI (S1) work

Two coupled gaps: the scenario gate, and the model rule.

| Option | Description | Pros | Cons |
|---|---|---|---|
| S-a | Add `TransportImageGen` to the `codex` scenario; Quick Config / Codex setup also creates a `gpt-image-2` rule to the Codex provider when one is connected | image tool works after the normal Codex setup; usage and rules stay attributable to `codex` | touches Codex setup code; users with a non-Codex provider need a rule choice |
| S-b | Route `/tingly/codex/**/images/*` to the `imagegen` scenario's rules | user configures images once (matches "smart defaults over toggles") | scenario `codex` stops meaning one thing; usage attribution splits; harder to explain |
| S-c | Document manual setup only | no code | fails the UX-first bar: the headline feature silently 400s |

**Recommend S-a**, with the auto-created rule limited to the Codex OAuth provider. If the user has no Codex provider the endpoint returns a 400 that says which rule is missing. Open question Q2. *Decided:* S-a without the auto-created rule; the user adds the `gpt-image-2` rule by hand (`codex-config.md`, last section).

## 4. Gateway response and persistence

- `persistImageGeneration` / `persistImages` pick the extension from `imagefmt.SniffBase64`, write the sidecar `.txt` with the same stem (current code does `strings.Replace(filename, ".png", ".txt", 1)`, which breaks for other extensions; derive the stem instead).
- Persist only sniffed raster formats. Anything else (HTML, SVG, empty) is logged at warn with the first bytes' type and **not written**; the JSON response still goes back to the caller unchanged.
- File names are generated (timestamp + index); nothing from the upstream response is used in a path.
- Recording / usage logs must not embed the base64 body. Check what `trackUsage...` and the request recorder keep for image responses and truncate; unverified today (§10 Q8).
- Size: no new cap on the upstream response below Codex's own limit. Define `MaxGeneratedImageBytes = 32 MiB` (decoded) to match Codex's tool limit; above it the file is not persisted and a warn is logged. Playground and `send_file` limits are listed in §6.

## 5. Routing, failover, capability, playground

### 5.1 Failover and capability

`gif` is a *routing constraint*, not just a validation. If a rule has several providers, the selector must filter by the capability table (§3.2) before picking, and return the 400 only when no provider in the rule qualifies. The same table feeds the `/models` capability metadata once #1774 lands (`output_formats: ["png","gif"]`), so clients and the UI can stop guessing.

### 5.2 Image playground (frontend)

Today PNG is assumed in `frontend/src/pages/image/components/imageGenSession.ts:32,37,65` (data URL, `base64ToBlob`), `ImageGenLightbox.tsx:311,329` (download name), thumbnails converted to webp (`imageThumbnails.ts:66`), and the upload accept list excludes GIF.

- A generated result carries `{ b64, mime }`, taken from the response `output_format` (which the gateway now sets from the bytes). Data URLs, blobs and download names use it.
- Thumbnails of a GIF keep the original bytes or a first-frame still; they must not be re-encoded through webp (that flattens the animation). The lightbox shows the animated original.
- GIF stays out of the reference-image accept list (Codex: edit via spritesheet, not via a GIF).
- Control (Phase 4, after the backend is proven): no mode picker. A single **Format** choice in the existing options (Image / Animated GIF) appears only when the selected model's provider reports `gif` in its capabilities; frame count and duration live under Advanced with Codex's defaults (9 frames, 200 ms). Per the UX principles: smart default (still image), show the concrete values, hide what the provider cannot do.

## 6. Projects and IM delivery, preview and security rules

### 6.1 How a GIF reaches a Projects thread

Threads are Claude Code sessions, not Codex sessions, and nothing in `projects/` calls the image endpoints. A GIF therefore reaches a thread when the agent (or the user's script/skill) calls the gateway, writes the bytes under the thread's cwd or `library/outputs/<slug>/`, and calls `send_file`. The gateway's own output directory (`configDir/image/YYYYMMDD/`) is outside both roots, so `PublishFile` rightly refuses it.

| Option | Description | Verdict |
|---|---|---|
| D1 | Nothing new: agent saves the decoded bytes itself, then `send_file`. Document a snippet. | **Now** |
| D2 | Add a Projects tool `generate_image` (calls the gateway, writes into the thread's outputs dir, publishes) | Separate TEP; needs an auth/identity story for Projects -> gateway |
| D3 | Let `send_file` read the gateway image dir | **Rejected**: widens the allowed roots of a security-checked tool |

### 6.2 One classification function for outgoing media

Problem: `imchannel` uses the MIME prefix, `sniffType` uses the extension first, adapters add their own extension lists. Result: SVG is kind `image` at the channel and only some adapters downgrade it.

Add `core.ClassifyMedia(path, mime) MediaKind` in `imbot/core` (imbot already declares media types incl. `gif`; `imchannel` and adapters both depend on it). It reads the first 512 bytes and returns:

| Result | When | 
|---|---|
| `image` | magic bytes are PNG, JPEG, WebP, BMP, or GIF (non-animated decision is not made here) |
| `animation` | GIF (a distinct kind so adapters can choose animation APIs) |
| `document` | everything else, **including SVG** (`<svg`/XML prefix, `.svg`/`.svgz`, `image/svg+xml`) and anything whose extension disagrees with its bytes in an unsafe direction (for example `.png` that sniffs as HTML) |

`imchannel.SendFile` calls it and stops trusting `mediaType` for the kind decision. Adapter extension lists stay as a second filter. This also fixes the existing Telegram gap for SVG (`SendPhoto`).

### 6.3 Per-platform GIF handling

| Platform | GIF plan | Evidence level |
|---|---|---|
| Telegram | *Decided:* GIF goes as a document like everywhere else; `SendAnimation` (exists in `go-telegram/bot` v1.23.0) is a later option once it can be tried live | method exists; behaviour unverified live |
| Feishu / Lark | `image` (GIF is in the adapter's image list, `feishu/media.go:17-27`) | list verified in code; animation behaviour unverified |
| DingTalk | `image` (`.gif` in list, `dingtalk/media.go:46-48`), 20 MB cap from #2401 | unverified live |
| WeChat | `image` (`weixin.go:392-394`) | unverified live |
| WeCom | `image` (`wecom.go:354`) | unverified live |
| Slack / Discord / WhatsApp | unsupported (unchanged) | n/a |

If a platform is found to flatten GIFs to a still, that platform sends GIF as a `document` instead (a one-line table change in its adapter). The live results go into the status table of `projects-published-files.md`. Per-platform size caps other than DingTalk's are **not known** here and must be collected during the live pass; the adapter or platform error is surfaced as today.

### 6.4 Preview and security rules (normative)

1. **SVG is never rendered by Tingly Box**: not in the Library preview (unchanged, #2429), not inline in chat, not as an IM `image`. It stays download-only, delivered with `Content-Disposition: attachment` (what `c.FileAttachment` does today) and `X-Content-Type-Options: nosniff`. Do not add SVG to the imagegen format allowlist, and do not convert or rasterize it server-side (external references and parser attack surface; separate TEP if ever wanted).
2. **GIF preview uses the existing path only**: `<img src=blob:>`, extension in the allowlist, magic bytes `GIF87a`/`GIF89a` checked before an object URL is created, 25 MB cap, decode failure -> "download instead" (`library-tree.md`). A `.gif` that is not a GIF is not shown.
3. **Bytes beat names**: any code that decides *kind* (IM, gateway persistence, playground) uses sniffed bytes; extension only breaks ties for text types.
4. **Generated names only**: persisted file names are generated, never taken from upstream content or a response field.
5. **No decoding in the gateway**: only header sniffing, so a hostile GIF cannot cost CPU or memory beyond the base64 body itself.
6. **Library access stays behind `os.Root`** (#2430); no new route serves files from the gateway image directory.
7. **Motion**: the Library preview shows the GIF as-is. Respecting `prefers-reduced-motion` is an open UX question (Q7), not a security rule.

### 6.5 Limits at a glance

| Limit | Value | Source |
|---|---|---|
| Codex tool decoded image | 32 MiB | `ext/image-generation/src/tool.rs` |
| GIF frames / duration | 2-16 / 20-2000 ms | `codex-api/src/images.rs` |
| Reference images per edit | 5 | `codex_images.go` |
| `send_file` | 200 MB | `published_file.go` |
| Library image preview | 25 MB | `library-tree.md` |
| DingTalk upload | 20 MB | #2401 |
| `n` for GIF | 4 (gateway choice) | this design |

## 7. Error cases

| Case | Behaviour |
|---|---|
| Unknown `output_format` | 400, lists allowed values |
| `gif` and no capable provider in the rule | 400, names the provider(s) tried |
| `frame_count` / `frame_duration_ms` out of range, or set without `gif` | 400 with the valid range |
| GIF used as an edit input image | 400: "pass a PNG spritesheet instead" |
| `n` above the GIF cap | 400 |
| Upstream feature not enabled / plan lacks it | upstream status and message passed through |
| Upstream returns bytes that sniff as non-raster | response forwarded; nothing persisted; warn log |
| Upstream returns PNG for a `gif` request | response `output_format` says `png`; warn log; no relabel |
| Upstream timeout (Codex says a few minutes) | existing timeout handling; test with a slow fake |
| IM upload fails | unchanged: error returned and shown in the thread; Library copy stays |

## 8. Tests

Unit:
- `imagefmt.Sniff` table: PNG, JPEG, GIF87a, GIF89a, WebP, SVG, HTML, empty, truncated; base64 variant with `R0lGOD` prefixes.
- Extras parsing: multipart form, JSON, ranges, "set without gif".
- Capability table x request matrix; failover skips incapable providers.
- `core.ClassifyMedia`: `.png` that is GIF, `.png` that is HTML, `.svg`, `image/svg+xml` with a `.png` name, GIF, WebP.
- Persistence: extension from sniff, sidecar stem for `.gif`, non-raster not written.

Component (fake servers, no real accounts):
- Codex client against an `httptest` backend: native generation request has `output_format:"gif"`, `frame_count`, `frame_duration_ms`; edit request carries the same; PNG request still uses the Responses tool and sends the correct `output_format` (regression for `codex_client.go:431`).
- Handler: `/tingly/codex/v1/images/generations` accepted after S-a and routed via the auto-created rule; 400s from §7.
- Assembler: partial images replace, final result wins.
- Telegram adapter with a fake Bot API: GIF -> `sendAnimation`, SVG -> `sendDocument`, PNG -> `sendPhoto`.
- `imchannel.SendFile` end to end with the tingly test channel for GIF and SVG.

Frontend (vitest): generated GIF round trip in `imageGenSession` (mime, download name, thumbnail not re-encoded); Library preview still refuses SVG and mismatched magic.

Manual / live (cannot be done by CI): one captured real Codex GIF request and response; each IM platform sending a GIF; Codex CLI with `features.image_generation_animation=true` through `/tingly/codex`.

## 9. Recommended phased plan

| Phase | Content | Why this order |
|---|---|---|
| 0. Spike (no product code) | Capture a real Codex GIF exchange (needs a ChatGPT account with the flag); check whether `api.openai.com` accepts `gif`; confirm the base path Codex uses for `images/*` through Tingly Box; confirm recorder behaviour for base64 bodies | Every later phase depends on facts we have only from source reading |
| 1. Format correctness | `imagefmt`; sniff-based extension and `output_format`; fix `codex_client.go` format mix-up; output_format validation (no GIF yet); `ClassifyMedia` + Telegram SVG guard; playground mime generalization. The assembler partial-image fix was deferred (§11). | Pure fixes, valuable without GIF, and they de-risk phase 2 |
| 2. GIF through the gateway | Extras struct; capability table + failover filter; hybrid Codex path P3; edit path fields; `codex` scenario `TransportImageGen` + auto rule (S-a) | The feature itself |
| 3. Projects / IM | Telegram `SendAnimation`; per-platform live matrix; `projects-published-files.md` table; D1 doc snippet | Needs phase 1 classification and a real GIF from phase 2 |
| 4. UI and follow-ups | Playground Format control; `/models` capability metadata (#1774); decide on `generate_image` Projects tool (D2) and Responses-to-Anthropic image handling as separate TEPs | Depends on proven backend |

Deferred on purpose: Responses `image_generation_call` conversion to Anthropic (Anthropic assistant messages cannot carry image blocks, so it needs its own decision), SVG rasterizing, gateway-side GIF assembly from a spritesheet using Go's `image/gif` (works for every vendor and needs no Codex flag, but it is a product feature of its own: prompting for aligned frames, quality, cost).

## 10. Open questions

1. **Priority surface.** Is the target the Codex CLI through `/tingly/codex` (S1), the Tingly Box Image page (S2), or both? It decides whether S-a (scenario change) is in phase 2 or can wait.
2. **Codex rule.** Should Quick Config create a `gpt-image-2` rule automatically when a Codex OAuth provider is connected (S-a), or route images to the `imagegen` scenario's rules (S-b)?
3. **Who runs the phase 0 capture?** It needs a ChatGPT account with `image_generation_animation` enabled. Without it, GIF stays unverified end to end.
4. **Public API.** If `api.openai.com` does not accept `gif`, is Codex-OAuth-only GIF acceptable, with a clear 400 for other OpenAI providers?
5. **GIF in IM by default:** image message, file, or animation, per platform? Needs the live pass and an owner.
6. **Gateway GIF assembly** (spritesheet -> GIF for any vendor) as an alternative or complement. Worth a TEP?
7. **Motion and accessibility:** autoplay in the Library preview vs `prefers-reduced-motion`.
8. **Recording and logs:** do the request recorder and usage logs keep full base64 image bodies today? If so, truncation is a prerequisite (not verified).
9. **Retention:** `configDir/image/` grows with 32 MiB GIFs; is there a cleanup policy (`image-library.md` §7 mentions the idea, not an implementation)?

## 11. As built (2026-11-05)

Three stacked commits on top of this design: format detection from bytes, GIF through the gateway, and the Image page selector. Go and frontend tests for each were written first against faked upstreams; nothing was run against a real Codex account (phase 0 is still open), so **GIF is experimental and Codex-only** in the UI and in the error messages.

### Decisions that differ from, or settle, the options above

| Topic | Decision |
|---|---|
| Carrying `frame_count` / `frame_duration_ms` (§3.3) | The handler parses them next to the typed SDK request (`imagegen.ParseOutputOptionsJSON` / `...Form`) and puts the result in the request context (`imagegen.WithOutputOptions`); clients read it with `OutputOptionsFrom`. The OpenAI-compatible client turns it into SDK extra fields. No interface change. |
| Codex path (§3.4) | Hybrid (P3): GIF -> native `POST codex/images/generations` (`imagesGenerateNative`); everything else stays on the Responses tool. The tool now sends the request's `output_format` (jpeg/webp, else png); it used to send `response_format`. Edit already used the native endpoint and now also carries the GIF fields; a GIF input image and GIF output with a mask are refused with a clear message. |
| `codex` scenario (§3.5) | `TransportImageGen` added, so `/tingly/codex/images/*` and `/tingly/codex/v1/images/*` pass the gate. **No `gpt-image-2` rule is auto-created**: the Codex CLI's tool hardcodes that model, so the user adds a rule (request model `gpt-image-2`, or `gpt-image-*`, scenario Codex, pointing at a Codex provider). Without one the CLI gets "not configured". Documented in `codex-config.md`. |
| Capability (§3.2) | Only GIF is gated: Codex (generate and edit) yes; OpenAI-compatible generation yes (experimental pass-through, so a Tingly Box in front of a Codex-backed gateway keeps working), OpenAI-compatible edit no (multipart cannot carry the frame fields); DashScope, MiniMax, xAI, Qianfan and unknown no. A refused GIF is a 400 naming the provider. png/jpeg/webp stay lenient; the response reports the sniffed truth. |
| Validation | Unknown `output_format` (incl. svg) -> 400; frame fields without `gif` -> 400; ranges 2-16 and 20-2000; `n` at most 4 for GIF. Both generation and edit (JSON and multipart). |
| Response and persistence | `output_format` in the response and the saved extension come from the first bytes. Bytes that are not png/jpeg/webp/gif are not written to disk. The sidecar `.txt` shares the image stem. |
| IM delivery | `core.ClassifyMedia` (imbot) decides from bytes: PNG/JPEG/WebP/BMP -> image; **GIF and SVG -> document** on every platform (Feishu, DingTalk, WeCom already honour the type; WeChat uses it first). Telegram also checks local files itself, so SVG/GIF/unknown never go through `sendPhoto`. `SendAnimation` is not used. |
| Projects | Attachment MIME comes from the bytes (raster, or an SVG hiding under any name); the chat chips and the composer draw only png/jpeg/gif/webp/bmp, so an SVG is a download chip. The Library preview (#2429) and the download headers (`attachment`, `nosniff`) already matched §6.4 and were not changed. |
| Image page | Format select (png default, jpeg, webp, gif) plus frames and duration under gif, en/zh/ru, experimental hint. The request is unchanged for png. Results carry `{b64, mime}` from the response `output_format` or the sniffed header; GIF thumbnails are a lossless first-frame still, the lightbox shows the animation. **Limitation:** edits with reference images go out as multipart through the SDK, which cannot carry the format fields, so a run with references returns the default format and the panel says so. |
| Assembler partial images | Not changed. `openai_responses_assembler.go` still concatenates `partial_image` chunks, but its tests assert that behaviour while `codex_client.go` and its tests treat each partial as a standalone image. Which is right needs a decision (and a real stream); left as a follow-up. |

### Not done
- Phase 0: no real Codex GIF request or response was captured; the native generations body follows `codex-rs/codex-api/src/images.rs`. If the live backend differs, `codexImageGenerateRequest` is the one place to adjust.
- Live IM checks for GIF/SVG on each platform and per-platform size caps.
- Image page: GIF edits, and a capability-aware hide of the GIF option (needs #1774).
- Recorder and usage logs were not checked for full base64 bodies (Q8).
- Retention of `configDir/image/` (Q9).

### Tests added
`internal/vision/imagefmt`, `internal/vision/imagegen/output_test.go`, `internal/client/codex_images_gif_test.go` and `openai_images_gif_test.go` (fake Codex and OpenAI-compatible upstreams), `internal/protocolserver/image_format_test.go` and `image_output_test.go` (persistence, scenario gate, validation, GIF pass-through and refusal through the real route chain), `imbot/core/media_classify_test.go`, `imbot/platform/telegram/media_kind_test.go`, `remote/channel/imchannel` (kind from bytes on five platforms), `projects/internal/project/attachment_test.go`, `frontend/packages/projects` attachment chips, and the Image page `imageOutput.test.ts` / `imageGenSession.test.ts`.
