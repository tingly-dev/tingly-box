# Image Effort Control

Playground 生成卡片新增可选 Effort 控件（`默认 / low / medium / high`）。

- **默认不发送** `reasoning_effort`，行为与现状一致；用户主动选择才传。
- **OpenAI 直连**：作为 `images/generations|edits` 的文档化参数 `reasoning_effort` 透传上游（gpt-image-1/-mini 支持；gpt-image-2 不允许，报错原样透出给用户）。
- **Codex (ChatGPT OAuth)**：网关将其映射为 Responses API 的 `Reasoning.Effort`（`buildImageGenerationResponsesRequest` / `buildImageEditResponsesRequest`）；不改动 `:fast` / `applyCodexDefaultsToParams` 逻辑（cf. 132f6d6fd）。原生 JSON edit 路径（`codex_images.go`）上游无此参数，不传。
- 控件始终显示，v1 不做 per-model 能力标记（gpt-image-2 的隐藏/禁用留作 v2）。

Spec 详见 `.sdlc/docs/imagegen-effort-control-20261025.spec.md`。
