# Codex image generation: GIF / SVG output (research notes)

Date: 2026-11-05. Feeds `.design/image-output-formats.md` and TEP #2432.
Method: read the `openai/codex` source on `main` through `gh api`, plus web search of OpenAI's published API reference. `developers.openai.com` and `github.com` page fetches were blocked in this environment, so the OpenAI doc statements below come from search-result snippets of the API reference, not from reading the pages. Nothing here was run against a real ChatGPT/Codex account.

## 1. What the Codex built-in image tool is now

| Fact | Where (openai/codex, `codex-rs/`) | Confidence |
|---|---|---|
| Built-in tool is an *extension* (`image_gen.imagegen`), not the Responses hosted `image_generation` tool. | `ext/image-generation/src/{tool,extension}.rs` | read the source |
| It POSTs `images/generations` and `images/edits` to the active model provider's base URL, JSON bodies, response `{created, data:[{b64_json, generation_id}], background?, quality?, size?}`. | `codex-api/src/endpoint/images.rs`, `codex-api/src/images.rs`, `ext/image-generation/src/backend.rs` | read the source |
| Model is hardcoded `gpt-image-2`. Codex Desktop advertises Images 2.5 (2026-09-08) but the tool still pins `gpt-image-2`; no model selector. | `tool.rs` (`IMAGE_MODEL`), issue openai/codex#43965 | source + issue |
| Tool args the model sees: `prompt`, `gif`, `transparent_background`, `referenced_image_paths` (max 5), `num_last_images_to_include` (1-5). No size/quality/format args. | `tool.rs` (`ImagegenArgs`), issue #19175 | read the source |
| Request header `x-codex-image-turn-id`; response header `x-codex-imagegen-request-id`. | `backend.rs`, `endpoint/images.rs` | read the source |
| Availability: only when the provider `is_openai()`, `requires_openai_auth`, or uses OpenAI actor authorization. | `extension.rs` | read the source; not tested |
| Max decoded image the tool accepts: 32 MiB. | `tool.rs` (`MAX_EXECUTOR_GENERATED_IMAGE_BYTES`) | read the source |
| Images are saved to `$CODEX_HOME/generated_images/<session>/<call_id>.{png,gif}`; the file is detected from the base64 header, not requested. | `artifact.rs` | read the source |

## 2. GIF

- Added by openai/codex PR #59073 "Add optional animated GIF output to image generation", merged 2026-11-03. Gated by `features.image_generation_animation`, stage `UnderDevelopment`, default off (`features/src/lib.rs`).
- Wire format (`codex-api/src/images.rs`): the Images request carries `output_format: "gif"` plus sibling `frame_count` (2-16, default 9) and `frame_duration_ms` (20-2000, default 200). `output_format` is serialised only for GIF; PNG requests omit it. Applies to both `images/generations` and `images/edits`.
- Result detection: base64 starting `R0lGODdh` / `R0lGODlh` means GIF, anything else is treated as PNG. Saved with `.gif`, replayed as `data:image/gif;base64,...`; the model receives a still preview.
- Model guidance (`imagegen_gif_description.md`): simple looping GIFs only, 2-16 frames; to edit one, build a spritesheet of equal-size cells and pass that through `referenced_image_paths`; do not pass the GIF itself as the edit input.
- Not verified: whether `api.openai.com/v1/images/generations` accepts `output_format: "gif"`. The public reference lists `output_format` as `png | jpeg | webp` and says it is supported for GPT image models only. Whether the ChatGPT/Codex backend renders true animation or assembles frames server-side is also unknown (the spritesheet advice suggests the latter).

## 3. SVG

- No `svg` string in any file of the imagegen extension (`tool.rs`, `artifact.rs`, `extension.rs`, `backend.rs`), and `GeneratedImageFormat` has only `Png` and `Gif`.
- `skills/src/assets/samples/imagegen/SKILL.md`: imagegen is for raster images; "Do not use when the task is better handled by editing existing SVG/vector/code-native assets"; SVG stand-ins only "when the user explicitly asked for vector output".
- Public API docs: `output_format` is png/jpeg/webp only. An SVG from a model is text written by a language model, not an image-API result.
- Conclusion: SVG is never an imagegen output. It can still reach Tingly Box as a file an agent wrote and published with `send_file`.

## 4. Tingly Box facts that matter (verified by reading or running)

- `codex` scenario transports are `[TransportOpenAI]` only (`internal/typ/scenario_registry.go:~120`) and both image handlers return 400 unless the scenario has `TransportImageGen` (`internal/protocolserver/openai_image.go:48`, `openai_image_edit.go:62`). So the Codex CLI pointed at `/tingly/codex` cannot reach `images/generations` or `images/edits` today. (Read, not run.)
- Tingly Box's generated Codex config sets `requires_openai_auth = true` (`.design/codex-auth.md`, `ai/agent/apply_support.go:1052-1060`), which satisfies the extension's availability check.
- Run in a scratch test (deleted afterwards): `json.Unmarshal` of `{"output_format":"gif","frame_count":5,"frame_duration_ms":100}` into `openai.ImageGenerateParams` keeps `output_format="gif"` (the SDK type does not validate the enum) but **drops** `frame_count` and `frame_duration_ms` (`ExtraFields()` is empty and re-marshal omits them).
- Codex *edit* already uses the native `POST {base}/codex/images/edits` JSON endpoint (`internal/client/codex_images.go`); *generation* still uses the Responses `image_generation` tool, a deliberate deferral recorded in `.design/image-edit.md` §3.4. `codex_round_tripper.go:433-438` already treats `/backend-api/codex/images/` paths as plain JSON.
- Telegram adapter sends kind `image` through `SendPhoto` with no extension or MIME check (`imbot/platform/telegram/telegram.go:552-561`); `go-telegram/bot` v1.23.0 has `SendAnimation`. `imchannel.SendFile` picks kind `image` for any `image/*` MIME, including `image/svg+xml` (`remote/channel/imchannel/imchannel.go:~52`).
- `projects` threads are Claude Code sessions (`agent: llm|claude-code`); no Codex-agent thread type, and nothing in `projects/` calls the image endpoints.

## 5. Sources

- https://github.com/openai/codex/pull/59073
- https://github.com/openai/codex/tree/main/codex-rs/ext/image-generation
- https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/images.rs
- https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/endpoint/images.rs
- https://github.com/openai/codex/blob/main/codex-rs/features/src/lib.rs
- https://github.com/openai/codex/blob/main/codex-rs/skills/src/assets/samples/imagegen/SKILL.md
- https://github.com/openai/codex/issues/43965, /19175, /50838
- https://developers.openai.com/api/reference/resources/images/methods/generate (png/jpeg/webp, from search snippet)
- https://developers.openai.com/api/reference/resources/responses/methods/create (`image_generation` tool `output_format`, from search snippet)
