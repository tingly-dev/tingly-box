# Claude Code — stream-json 参考资料目录

本目录存放 Claude Code `--output-format stream-json` 输出格式的参考资料，用于驱动
`agentboot/claude` 的解析/校验逻辑与 `stream-json.schema.json` 的维护。

## 文件清单

| 文件 | 说明 | 用途 |
| --- | --- | --- |
| [`stream-json.schema.json`](./stream-json.schema.json) | NDJSON 流消息的 JSON Schema（draft-07）。顶层 `oneOf` 覆盖 `system` / `assistant` / `user` / `tool_use` / `tool_result` / `result` / `stream_event` 七类 envelope，内部 `$defs` 定义 message / content block / delta / usage 等。 | 解析器与测试校验的权威 schema |
| [`stream-json.md`](./stream-json.md) | 一次真实 `claude -p` 调用的采集日志（含 `init`→`assistant(tool_use)`→`user`→`assistant(text)`→`result` 完整流，已用 Z.AI 提供的 `glm-4.7` 后端）。 | 真实样本，验证字段与时序 |
| [`assistant.json`](./assistant.json) | 单条 `assistant` 消息的完整结构样本（来自 **Go Anthropic SDK** 的结构体序列化）。 | 暴露 SDK 内部 tagged-union 字段与 usage 细节 |

## schema 信息来源

`stream-json.schema.json` 的字段以**多方交叉验证**为准。重要前提：Anthropic **未正式文档化**
完整 stream-json schema（见 [issue #24612](https://github.com/anthropics/claude-code/issues/24612)），
因此字段属「观察性」、可能随 CLI/SDK 版本变化。schema 全程采用 forward-compatible 设计
（`additionalProperties: true`、subtype 用宽松字符串），避免新字段导致校验失败。

### 一手/官方
- **GitHub Issue #24612** — [`claude -p --output-format stream-json`](https://github.com/anthropics/claude-code/issues/24612)
  消息类型与字段最集中的社区汇总，明确指出文档缺口。
- **Claude Code CLI Reference** — [`code.claude.com/docs/en/cli-reference`](https://code.claude.com/docs/en/cli-reference)
  `--output-format` 选项（`text` / `json` / `stream-json`）。
- **Agent SDK reference (Python/TS)** — [`code.claude.com/docs/en/agent-sdk`](https://code.claude.com/docs/en/agent-sdk/python)
  流式事件、result 消息结构、subagent `parent_tool_use_id`。
- **Anthropic Messages API — Create a Message (streaming)** — [`docs.claude.com/.../streaming`](https://platform.claude.com/docs/en/build-with-claude/streaming)
  底层 SSE 事件名与字段：`message_start` / `content_block_*` / `message_delta` / `message_stop` /
  `ping` / `error`，以及 delta 类型 `text_delta` / `input_json_delta` / `thinking_delta` /
  `signature_delta`，`stop_reason` 取值（含 `pause_turn`、`refusal`）。

### 本地采集样本（本目录，优先级最高）
- `assistant.json` 实测可见的、schema 当前以 forward-compatible 方式覆盖（未严格建模）的字段：
  - `message.container`（`id` / `expires_at` / `skills`）
  - `message.context_management`（`applied_edits`）
  - `usage.cache_creation.ephemeral_1h_input_tokens` / `ephemeral_5m_input_tokens`
  - `usage.inference_geo`
  - `usage.server_tool_use.web_fetch_requests`（schema 已覆盖 `web_search_requests`）
  - content block 的 SDK tagged-union 容器（`OfBetaMCPToolResultBlockContent` / `OfString` / `OfContent` …）

### 社区/第三方
- **takopi stream-json cheatsheet** — [`takopi.dev/.../stream-json-cheatsheet`](https://takopi.dev/reference/runners/claude/stream-json-cheatsheet/)
  逐消息类型字段清单，含 `result` 的 `result_reason` / `duration_api_ms` / `permission_denials`。
- **Go genai provider 常量**（`github.com/maruel/genai/providers/anthropic`）—
  类型化的 delta 与 stop_reason 常量，用于交叉核对枚举值。

## 维护约定

1. **新增字段默认可选**。仅在多个来源确认取值集合时，才用 `enum` 收紧；否则保持 forward-compatible。
2. **改动后用 `stream-json.md` / `assistant.json` 回归校验**，确保真实样本仍通过 schema。
3. **优先以本地采集样本为准**，其次官方 API 文档，最后社区资料——三者冲突时以样本为准。
4. 更新时在本文件「信息来源」补充新增引用链接。
